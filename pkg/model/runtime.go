package model

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/redhat-developer/rhdh-operator/api"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

const BackstageAppLabel = "rhdh.redhat.com/app"
const ConfiguredNameAnnotation = "rhdh.redhat.com/configured-name"
const DefaultMountPathAnnotation = "rhdh.redhat.com/mount-path"
const DefaultSubPathAnnotation = "rhdh.redhat.com/sub-path"
const ContainersAnnotation = "rhdh.redhat.com/containers"
const SourceAnnotation = "rhdh.redhat.com/source"

// Runtime object keys used to store and retrieve objects from BackstageModel
const (
	DeploymentKey     = "deployment.yaml"
	ServiceKey        = "service.yaml"
	RouteKey          = "route.yaml"
	AppConfigKey      = "app-config.yaml"
	DynamicPluginsKey = "dynamic-plugins.yaml"
	DbStatefulSetKey  = "db-statefulset.yaml"
	DbServiceKey      = "db-service.yaml"
	DbSecretKey       = "db-secret.yaml"
	SecretEnvsKey     = "secret-envs.yaml"
	SecretFilesKey    = "secret-files.yaml"
	ConfigMapEnvsKey  = "configmap-envs.yaml"
	ConfigMapFilesKey = "configmap-files.yaml"
	PvcsKey           = "pvcs.yaml"
)

// Backstage configuration scaffolding with empty BackstageObjects.
// There are all possible objects for configuration
var runtimeConfig []ObjectConfig

// BackstageModel represents internal object model
type BackstageModel struct {
	localDbEnabled bool
	isOpenshift    bool

	RuntimeObjects []RuntimeObject

	ExternalConfig ExternalConfig
}

// setRuntimeObject adds an object to the model.
func (m *BackstageModel) setRuntimeObject(object RuntimeObject) {
	m.RuntimeObjects = append(m.RuntimeObjects, object)
}

// GetRuntimeObject retrieves an object from the model by key.
// Returns nil if the object doesn't exist or shouldn't be applied (Object() returns nil).
func (m *BackstageModel) GetRuntimeObject(key string) RuntimeObject {
	for _, obj := range m.RuntimeObjects {
		if obj.GetKey() == key && obj.Object() != nil {
			return obj
		}
	}
	return nil
}

// GetRuntimeObjects returns only objects that should be applied (where Object() != nil).
// Objects are returned in the deterministic order they were registered (via init() functions).
func (m *BackstageModel) GetRuntimeObjects() []RuntimeObject {
	result := make([]RuntimeObject, 0)
	for _, obj := range m.RuntimeObjects {
		if obj.Object() != nil {
			result = append(result, obj)
		}
	}
	return result
}

// getDeployment returns the BackstageDeployment from the model.
// Returns nil if deployment doesn't exist in the model.
func (m *BackstageModel) getDeployment() *BackstageDeployment {
	obj := m.GetRuntimeObject(DeploymentKey)
	if obj == nil {
		return nil
	}
	return obj.(*BackstageDeployment)
}

func (m *BackstageModel) GetDeploymentGVK() schema.GroupVersionKind {
	deployment := m.getDeployment()
	return deployment.deployable.GetObject().GetObjectKind().GroupVersionKind()
}

// Registers config object
func registerConfig(key string, factory ObjectFactory, multiple bool, mergeFunc MergeConfigFunc) {
	for _, obj := range runtimeConfig {
		if obj.Key == key {
			panic("duplicate object key " + key)
		}
	}
	runtimeConfig = append(runtimeConfig, ObjectConfig{
		Key:           key,
		ObjectFactory: factory,
		Multiple:      multiple,
		MergeFunc:     mergeFunc,
	})
}

// InitObjects performs a main loop for configuring and making the array of objects to reconcile
func InitObjects(ctx context.Context, backstage api.Backstage, externalConfig ExternalConfig, platform platform.Platform, scheme *runtime.Scheme) (*BackstageModel, error) {

	// 3 phases of Backstage configuration:
	// 1- load from Operator defaults, modify metadata (labels, selectors..) and namespace as needed
	// 2- overlay some/all objects with Backstage.spec.rawRuntimeConfig CM
	// 3- override some parameters defined in Backstage.spec.application
	// At the end there should be an array of runtime RuntimeObjects to apply (order optimized)

	lg := log.FromContext(ctx)
	lg.V(1)

	model := &BackstageModel{
		ExternalConfig: externalConfig,
		localDbEnabled: backstage.Spec.IsLocalDbEnabled(),
		isOpenshift:    platform.IsOpenshift(),
		RuntimeObjects: make([]RuntimeObject, 0),
	}

	// Get enabled flavours once for all configs
	flavours, err := GetEnabledFlavours(backstage.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to determine enabled flavours: %w", err)
	}
	if len(flavours) > 0 {
		for _, flavour := range flavours {
			lg.Info("found enabled flavour", "flavour:", flavour.name)
		}
	}

	// looping through the registered runtimeConfig objects initializing the model
	for _, conf := range runtimeConfig {

		// creating the instance of backstageObject
		backstageObject := conf.ObjectFactory.newBackstageObject()

		// Choose config: overlay OR default (not both)
		var chosenConfig runtime.Object

		// First, try overlay from CR spec
		overlay, overlayExist := externalConfig.RawConfig[conf.Key]
		if overlayExist {
			if objs, err := utils.ReadYamls([]byte(overlay), nil, *scheme); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return nil, fmt.Errorf("failed to read overlay config for the key %s, reason: %w", conf.Key, err)
				}
			} else {
				if obj, err := adjustObject(conf, objs); err != nil {
					return nil, fmt.Errorf("failed to initialize object from overlay: %w", err)
				} else {
					chosenConfig = obj
				}
			}
		}

		// If no overlay, use default config
		if chosenConfig == nil {
			if objs, err := ReadDefaultConfig(conf, flavours, *scheme, platform.Extension); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return nil, fmt.Errorf("failed to read default value for the key %s, reason: %w", conf.Key, err)
				}
			} else if len(objs) > 0 {
				if obj, err := adjustObject(conf, objs); err != nil {
					return nil, fmt.Errorf("failed to initialize object from default: %w", err)
				} else {
					chosenConfig = obj
				}
			}
		}

		// Add object to model (always added, even if config is nil - placeholder pattern)
		if err := backstageObject.addToModel(model, backstage, chosenConfig, scheme); err != nil {
			return nil, fmt.Errorf("failed to add object to model for key %s, reason: %w", conf.Key, err)
		}
	}

	// Phase 2: updateAndValidate all objects
	// All objects are now in model, so cross-references are safe
	// Iterate over RuntimeObjects in their registration order (deterministic)
	for _, obj := range model.RuntimeObjects {
		err := obj.updateAndValidate(backstage, scheme)
		if err != nil {
			return nil, fmt.Errorf("failed object validation, reason: %w", err)
		}
	}

	return model, nil
}

// Every RuntimeObject.setMetaInfo should as minimum call this
func setMetaInfo(clientObj client.Object, backstage api.Backstage, scheme *runtime.Scheme) {

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
	if len(objects) == 0 {
		return nil, nil
	}

	if !objectConfig.Multiple {
		// only one object expected
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
