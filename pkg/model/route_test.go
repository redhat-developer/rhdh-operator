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
	"testing"

	openshift "github.com/openshift/api/route/v1"

	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRoute(t *testing.T) {
	bs := bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		Spec: bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				Route: &bsv1alpha1.Route{},
			},
		},
	}
	assert.True(t, bs.Spec.IsRouteEnabled())

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("route.yaml", "raw-route.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, true, testObj.scheme)

	assert.NoError(t, err)

	assert.NotNil(t, model.route)

	assert.Equal(t, RouteName(bs.Name), model.route.route.Name)
	assert.Equal(t, model.backstageService.service.Name, model.route.route.Spec.To.Name)
	// from spec
	assert.Equal(t, "/default", model.route.route.Spec.Path)
	// from default
	assert.NotNil(t, model.route.route.Spec.TLS)
	assert.NotEmpty(t, model.route.route.Spec.TLS.Termination)

	//	assert.Empty(t, model.route.route.Spec.Host)
}

func TestSpecifiedRoute(t *testing.T) {
	bs := bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		Spec: bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				Route: &bsv1alpha1.Route{
					Enabled: ptr.To(true),
					Host:    "TestSpecifiedRoute",
					//TLS:     nil,
				},
			},
		},
	}

	assert.True(t, bs.Spec.IsRouteEnabled())

	// Test w/o default route configured
	testObjNoDef := createBackstageTest(bs).withDefaultConfig(true)
	model, err := InitObjects(context.TODO(), bs, testObjNoDef.externalConfig, true, true, testObjNoDef.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// check if what we have is what we specified in bs
	assert.Equal(t, RouteName(bs.Name), model.route.route.Name)
	assert.Equal(t, bs.Spec.Application.Route.Host, model.route.route.Spec.Host)

	// Test with default route configured
	testObjWithDef := testObjNoDef.addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err = InitObjects(context.TODO(), bs, testObjWithDef.externalConfig, true, true, testObjWithDef.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// check if what we have is default route merged with fields defined in bs
	assert.Equal(t, RouteName(bs.Name), model.route.route.Name)
	assert.Equal(t, bs.Spec.Application.Route.Host, model.route.route.Spec.Host)
	assert.NotNil(t, model.route.route.Spec.TLS)
	assert.Equal(t, openshift.TLSTerminationEdge, model.route.route.Spec.TLS.Termination)
}

func TestDisabledRoute(t *testing.T) {

	// Route.Enabled = false
	bs := bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		Spec: bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				Route: &bsv1alpha1.Route{
					Enabled: ptr.To(false),
					Host:    "TestSpecifiedRoute",
					//TLS:     nil,
				},
			},
		},
	}

	// With def route config
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, true, testObj.scheme)
	assert.NoError(t, err)
	assert.Nil(t, model.route)

	// W/o def route config
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, true, true, testObj.scheme)
	assert.NoError(t, err)
	assert.Nil(t, model.route)

}

func TestExcludedRoute(t *testing.T) {
	// No route configured
	bs := bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		//Spec: bsv1alpha1.BackstageSpec{ //	//Application: &bsv1alpha1.Application{},
		//},
	}

	// With def route config - create default route
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, true, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// W/o def route config - do not create route
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, true, true, testObj.scheme)
	assert.NoError(t, err)
	assert.Nil(t, model.route)
}

func TestEnabledRoute(t *testing.T) {
	// Route is enabled by default if configured
	bs := bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		Spec: bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				Route: &bsv1alpha1.Route{},
			},
		},
	}

	// With def route config
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, true, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// W/o def route config
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, true, true, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

}
