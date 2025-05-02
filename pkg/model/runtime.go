package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

const BackstageAppLabel = "rhdh.redhat.com/app"
const ConfiguredNameAnnotation = "rhdh.redhat.com/configured-name"
const DefaultMountPathAnnotation = "rhdh.redhat.com/mount-path"
const ContainersAnnotation = "rhdh.redhat.com/containers"

// Backstage configuration scaffolding with empty BackstageObjects.
// There are all possible objects for configuration
var runtimeConfig []ObjectConfig

// BackstageModel represents internal object model
type BackstageModel struct {
	localDbEnabled bool
	isOpenshift    bool

	backstageDeployment *BackstageDeployment
	backstageService    *BackstageService

	localDbStatefulSet *DbStatefulSet
	LocalDbService     *DbService
	LocalDbSecret      *DbSecret

	route     *BackstageRoute
	appConfig *AppConfig

	RuntimeObjects []RuntimeObject

	ExternalConfig ExternalConfig
}

func (m *BackstageModel) setRuntimeObject(object RuntimeObject) {
	__sealights__.TraceFunc("f2064c2e8a23ea1ea8")
	for i, obj := range m.RuntimeObjects {
		if reflect.TypeOf(obj) == reflect.TypeOf(object) {
			m.RuntimeObjects[i] = object
			return
		}
	}
	m.RuntimeObjects = append(m.RuntimeObjects, object)
}

func (m *BackstageModel) getRuntimeObjectByType(object RuntimeObject) RuntimeObject {
	__sealights__.TraceFunc("5dd18a6235ac63b792")
	for _, obj := range m.RuntimeObjects {
		if reflect.TypeOf(obj) == reflect.TypeOf(object) {
			return obj
		}
	}
	return nil
}

// sort objects so DbStatefulSet and BackstageDeployment become the last in the list
func (m *BackstageModel) sortRuntimeObjects() {
	__sealights__.TraceFunc("dc24c33fb378f0a82f")

	slices.SortFunc(m.RuntimeObjects,
		func(a, b RuntimeObject) int {
			__sealights__.TraceFunc("8fe409301c738134a1")
			_, ok1 := b.(*DbStatefulSet)
			_, ok2 := b.(*BackstageDeployment)
			if ok1 || ok2 {
				return -1
			}
			return 1
		})
}

// Registers config object
func registerConfig(key string, factory ObjectFactory, multiple bool) {
	__sealights__.TraceFunc("87e70c3bedb19e87ab")
	runtimeConfig = append(runtimeConfig, ObjectConfig{Key: key, ObjectFactory: factory, Multiple: multiple})
}

// InitObjects performs a main loop for configuring and making the array of objects to reconcile
func InitObjects(ctx context.Context, backstage bsv1.Backstage, externalConfig ExternalConfig, platform platform.Platform, scheme *runtime.Scheme) (*BackstageModel, error) {
	__sealights__.TraceFunc("5b679d4ee1ff18b1d6")

	// 3 phases of Backstage configuration:
	// 1- load from Operator defaults, modify metadata (labels, selectors..) and namespace as needed
	// 2- overlay some/all objects with Backstage.spec.rawRuntimeConfig CM
	// 3- override some parameters defined in Backstage.spec.application
	// At the end there should be an array of runtime RuntimeObjects to apply (order optimized)

	lg := log.FromContext(ctx)
	lg.V(1)

	model := &BackstageModel{RuntimeObjects: make([]RuntimeObject, 0), ExternalConfig: externalConfig, localDbEnabled: backstage.Spec.IsLocalDbEnabled(), isOpenshift: platform.IsOpenshift()}

	// looping through the registered runtimeConfig objects initializing the model
	for _, conf := range runtimeConfig {

		// creating the instance of backstageObject
		backstageObject := conf.ObjectFactory.newBackstageObject()

		var templ = backstageObject.EmptyObject()
		if objs, err := utils.ReadYamlFiles(utils.DefFile(conf.Key), templ, *scheme, platform.Extension); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("failed to read default value for the key %s, reason: %s", conf.Key, err)
			}
		} else {
			if obj, err := adjustObject(conf, objs); err != nil {
				return nil, fmt.Errorf("failed to initialize object: %w", err)
			} else {
				backstageObject.setObject(obj)
			}
		}

		// read configuration defined in BackstageCR.Spec.RawConfigContent ConfigMap
		// if present, backstageObject's default configuration will be overridden
		overlay, overlayExist := externalConfig.RawConfig[conf.Key]
		if overlayExist {
			// new object to replace default, not merge
			templ = backstageObject.EmptyObject()
			if objs, err := utils.ReadYamls([]byte(overlay), nil, templ, *scheme); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return nil, fmt.Errorf("failed to read default value for the key %s, reason: %s", conf.Key, err)
				}
			} else {
				if obj, err := adjustObject(conf, objs); err != nil {
					return nil, fmt.Errorf("failed to initialize object: %w", err)
				} else {
					backstageObject.setObject(obj)
				}
			}
		}

		// apply spec and add the object to the model and list
		if added, err := backstageObject.addToModel(model, backstage); err != nil {
			return nil, fmt.Errorf("failed to initialize backstage, reason: %s", err)
		} else if added {
			backstageObject.setMetaInfo(backstage, scheme)
		}
	}

	// set generic metainfo and updateAndValidate all
	for _, v := range model.RuntimeObjects {
		err := v.updateAndValidate(model, backstage)
		if err != nil {
			return nil, fmt.Errorf("failed object validation, reason: %s", err)
		}
	}

	// Add objects specified in Backstage CR
	if err := addFromSpec(backstage.Spec, model); err != nil {
		return nil, err
	}

	// sort for reconciliation number optimization
	model.sortRuntimeObjects()

	return model, nil
}

func addFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {
	__sealights__.TraceFunc("bb1ee8bb05a3069aed")

	if err := addAppConfigsFromSpec(spec, model); err != nil {
		return err
	}

	if err := addConfigMapFilesFromSpec(spec, model); err != nil {
		return err
	}

	addConfigMapEnvsFromSpec(spec, model)
	if err := addDynamicPluginsFromSpec(spec, model); err != nil {
		return err
	}
	if err := addSecretFilesFromSpec(spec, model); err != nil {
		return err
	}
	if err := addSecretEnvsFromSpec(spec, model); err != nil {
		return err
	}
	addPvcsFromSpec(spec, model)
	return nil
}

// Every RuntimeObject.setMetaInfo should as minimum call this
func setMetaInfo(clientObj client.Object, backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("2e11bee2f154dea76a")

	clientObj.SetNamespace(backstage.Namespace)
	clientObj.SetLabels(utils.SetKubeLabels(clientObj.GetLabels(), backstage.Name))

	if err := controllerutil.SetControllerReference(&backstage, clientObj, scheme); err != nil {
		//error should never have happened,
		//otherwise the Operator has invalid (not a runtime.Object) or non-registered type.
		//In both cases it will fail before this place
		panic(err)
	}
}

func adjustObject(objectConfig ObjectConfig, objects []client.Object) (runtime.Object, error) {
	__sealights__.TraceFunc("0167efc3388da53710")
	if len(objects) == 0 {
		return nil, nil
	}
	if !objectConfig.Multiple {
		if len(objects) > 1 {
			return nil, fmt.Errorf("multiple objects not expected for: %s", objectConfig.Key)
		}
		return objects[0], nil
	}

	return &multiobject.MultiObject{
		Items: objects,
		// any object is ok as GVK is the same
		ObjectKind: objects[0].GetObjectKind(),
	}, nil

}
