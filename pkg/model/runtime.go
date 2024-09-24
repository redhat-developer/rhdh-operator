//
// Copyright (c) 2023 Red Hat, Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"redhat-developer/red-hat-developer-hub-operator/pkg/model/multiobject"
	"reflect"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/log"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"

	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"
)

const BackstageAppLabel = "rhdh.redhat.com/app"

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

	route *BackstageRoute

	RuntimeObjects []RuntimeObject

	ExternalConfig ExternalConfig
}

func (m *BackstageModel) setRuntimeObject(object RuntimeObject) {
	for i, obj := range m.RuntimeObjects {
		if reflect.TypeOf(obj) == reflect.TypeOf(object) {
			m.RuntimeObjects[i] = object
			return
		}
	}
	m.RuntimeObjects = append(m.RuntimeObjects, object)
}

// sort objects so DbStatefulSet and BackstageDeployment become the last in the list
func (m *BackstageModel) sortRuntimeObjects() {

	slices.SortFunc(m.RuntimeObjects,
		func(a, b RuntimeObject) int {
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
	runtimeConfig = append(runtimeConfig, ObjectConfig{Key: key, ObjectFactory: factory, Multiple: multiple})
}

// InitObjects performs a main loop for configuring and making the array of objects to reconcile
func InitObjects(ctx context.Context, backstage bsv1.Backstage, externalConfig ExternalConfig, ownsRuntime bool, isOpenshift bool, scheme *runtime.Scheme) (*BackstageModel, error) {

	// 3 phases of Backstage configuration:
	// 1- load from Operator defaults, modify metadata (labels, selectors..) and namespace as needed
	// 2- overlay some/all objects with Backstage.spec.rawRuntimeConfig CM
	// 3- override some parameters defined in Backstage.spec.application
	// At the end there should be an array of runtime RuntimeObjects to apply (order optimized)

	lg := log.FromContext(ctx)
	lg.V(1)

	model := &BackstageModel{RuntimeObjects: make([]RuntimeObject, 0), ExternalConfig: externalConfig, localDbEnabled: backstage.Spec.IsLocalDbEnabled(), isOpenshift: isOpenshift}

	// looping through the registered runtimeConfig objects initializing the model
	for _, conf := range runtimeConfig {

		// creating the instance of backstageObject
		backstageObject := conf.ObjectFactory.newBackstageObject()

		var templ = backstageObject.EmptyObject()
		if objs, err := utils.ReadYamlFiles(utils.DefFile(conf.Key), templ, *scheme); err != nil {
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
			if objs, err := utils.ReadYamls([]byte(overlay), templ, *scheme); err != nil {
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
			//setMetaInfo(backstageObject, backstage, ownsRuntime, scheme)
			backstageObject.setMetaInfo(backstage, scheme)
		}
	}

	// set generic metainfo and validate all
	for _, v := range model.RuntimeObjects {
		err := v.validate(model, backstage)
		if err != nil {
			return nil, fmt.Errorf("failed object validation, reason: %s", err)
		}
	}

	// sort for reconciliation number optimization
	model.sortRuntimeObjects()

	return model, nil
}

// Every RuntimeObject.setMetaInfo should as minimum call this
func setMetaInfo(clientObj client.Object, backstage bsv1.Backstage, scheme *runtime.Scheme) {

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
