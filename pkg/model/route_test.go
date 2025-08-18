package model

import (
	"bytes"
	"context"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	openshift "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRoute(t *testing.T) {
	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				Route: &bsv1.Route{},
			},
		},
	}
	assert.True(t, bs.Spec.IsRouteEnabled())

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("route.yaml", "raw-route.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)

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
	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				Route: &bsv1.Route{
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
	model, err := InitObjects(context.TODO(), bs, testObjNoDef.externalConfig, platform.OpenShift, testObjNoDef.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// check if what we have is what we specified in bs
	assert.Equal(t, RouteName(bs.Name), model.route.route.Name)
	assert.Equal(t, bs.Spec.Application.Route.Host, model.route.route.Spec.Host)

	// Test with default route configured
	testObjWithDef := testObjNoDef.addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err = InitObjects(context.TODO(), bs, testObjWithDef.externalConfig, platform.OpenShift, testObjWithDef.scheme)

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
	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				Route: &bsv1.Route{
					Enabled: ptr.To(false),
					Host:    "TestSpecifiedRoute",
					//TLS:     nil,
				},
			},
		},
	}

	// With def route config
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.Nil(t, model.route)

	// W/o def route config
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.Nil(t, model.route)

}

func TestExcludedRoute(t *testing.T) {
	// No route configured
	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		//Spec: bsv1.BackstageSpec{ //	//Application: &bsv1.Application{},
		//},
	}

	// With def route config - create default route
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// W/o def route config - do not create route
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.Nil(t, model.route)
}

func TestEnabledRoute(t *testing.T) {
	// Route is enabled by default if configured
	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSpecifiedRoute",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				Route: &bsv1.Route{},
			},
		},
	}

	// With def route config
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// W/o def route config
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

}

func Test_buildBaseUrl(t *testing.T) {
	type args struct {
		model     *BackstageModel
		backstage bsv1.Backstage
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "should not return anything on a non-OpenShift platform",
			args: args{
				model: &BackstageModel{
					isOpenshift: false,
				},
			},
			want: "",
		},
		{
			name: "should not return anything if route is disabled in the CR",
			args: args{
				model: &BackstageModel{
					isOpenshift: true,
				},
				backstage: bsv1.Backstage{
					Spec: bsv1.BackstageSpec{
						Application: &bsv1.Application{
							Route: &bsv1.Route{
								Enabled: ptr.To(false),
							},
						},
					},
				},
			},
			want: "",
		},
		{
			name: "should not return anything if there is no cluster ingress domain set",
			args: args{
				model: &BackstageModel{
					isOpenshift: true,
				},
				backstage: bsv1.Backstage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-backstage",
						Namespace: "my-ns",
					},
				},
			},
			want: "",
		},
		{
			name: "should return the default route domain if no route spec",
			args: args{
				model: &BackstageModel{
					isOpenshift: true,
					ExternalConfig: ExternalConfig{
						OpenShiftIngressDomain: "my-ocp-apps.example.com",
					},
				},
				backstage: bsv1.Backstage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-backstage",
						Namespace: "my-ns",
					},
				},
			},
			want: "https://backstage-my-backstage-my-ns.my-ocp-apps.example.com",
		},
		{
			name: "should return the route with the sub-domain if set in the route spec",
			args: args{
				model: &BackstageModel{
					isOpenshift: true,
					ExternalConfig: ExternalConfig{
						OpenShiftIngressDomain: "my-ocp-apps.example.com",
					},
				},
				backstage: bsv1.Backstage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-backstage",
						Namespace: "my-ns",
					},
					Spec: bsv1.BackstageSpec{
						Application: &bsv1.Application{
							Route: &bsv1.Route{
								Enabled:   ptr.To(true),
								Subdomain: "my-backstage.subdomain",
							},
						},
					},
				},
			},
			want: "https://my-backstage.subdomain.my-ocp-apps.example.com",
		},
		{
			name: "should return the route host if set in the route spec even with a subdomain",
			args: args{
				model: &BackstageModel{
					isOpenshift: true,
					ExternalConfig: ExternalConfig{
						OpenShiftIngressDomain: "my-ocp-apps.example.com",
					},
				},
				backstage: bsv1.Backstage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-backstage",
						Namespace: "my-ns",
					},
					Spec: bsv1.BackstageSpec{
						Application: &bsv1.Application{
							Route: &bsv1.Route{
								Enabled:   ptr.To(true),
								Host:      "my-awesome-backstage.idp.example.com",
								Subdomain: "my-backstage.subdomain",
							},
						},
					},
				},
			},
			want: "https://my-awesome-backstage.idp.example.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(
				t,
				tt.want,
				buildBaseUrl(tt.args.model, tt.args.backstage),
				"buildOpenShiftBaseUrl(%v, %v)",
				tt.args.model, tt.args.backstage,
			)
		})
	}
}

func TestBackstageRoute_updateAppConfigWithBaseUrls(t *testing.T) {
	type args struct {
		model     *BackstageModel
		backstage bsv1.Backstage
	}
	tests := []struct {
		name     string
		args     args
		assertFn func(t *testing.T, res map[string]map[string]any)
	}{
		{
			name: "default app config ConfigMap not initialized",
			args: args{
				model: &BackstageModel{},
			},
			assertFn: func(t *testing.T, res map[string]map[string]any) {
				assert.Empty(t, res)
			},
		},
		{
			name: "empty data in default app config ConfigMap",
			args: args{
				model: &BackstageModel{
					ExternalConfig: ExternalConfig{
						OpenShiftIngressDomain: "my-ocp-apps.example.com",
					},
					appConfig: &AppConfig{
						ConfigMap: &corev1.ConfigMap{
							Data: map[string]string{
								"my-default-app-config.yaml": "",
							},
						},
					},
				},
				backstage: bsv1.Backstage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-backstage-app",
						Namespace: "my-ns",
					},
				},
			},
			assertFn: func(t *testing.T, res map[string]map[string]any) {
				const expected = "https://backstage-my-backstage-app-my-ns.my-ocp-apps.example.com"
				assert.Len(t, res, 1)
				v := res["my-default-app-config.yaml"]
				assert.NotNil(t, v)
				assert.Equal(t, expected, v["app"].(map[string]any)["baseUrl"].(string))
				assert.Equal(t, expected, v["backend"].(map[string]any)["baseUrl"].(string))
				assert.Equal(t, expected, v["backend"].(map[string]any)["cors"].(map[string]any)["origin"].(string))
			},
		},
		{
			name: "multi-file default app-config ConfigMap with other fields defined",
			args: args{
				model: &BackstageModel{
					ExternalConfig: ExternalConfig{
						OpenShiftIngressDomain: "my-ocp-apps.example.com",
					},
					appConfig: &AppConfig{
						ConfigMap: &corev1.ConfigMap{
							Data: map[string]string{
								"my-default-app-config-1.yaml": `---
app:
  title: "My Awesome App"
plugin1:
  config1: [val1, val2]
---
`,
								"my-default-app-config-2.yaml": `backend:
  baseUrl: https://app.example.com
  auth:
    # TODO: once plugins have been migrated we can remove this, but right now it
    # is require for the backend-next to work in this repo
    dangerouslyDisableDefaultAuthPolicy: true
  cors:
    origin: http://localhost:3000
    credentials: true

organization:
  name: My Company
    
`,
							},
						},
					},
				},
				backstage: bsv1.Backstage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-backstage-app",
						Namespace: "my-ns",
					},
				},
			},
			assertFn: func(t *testing.T, res map[string]map[string]any) {
				const expected = "https://backstage-my-backstage-app-my-ns.my-ocp-apps.example.com"
				assert.Len(t, res, 2)
				for _, v := range res {
					assert.NotNil(t, v)
					assert.Equal(t, expected, v["app"].(map[string]any)["baseUrl"].(string))
					assert.Equal(t, expected, v["backend"].(map[string]any)["baseUrl"].(string))
					assert.Equal(t, expected, v["backend"].(map[string]any)["cors"].(map[string]any)["origin"].(string))
				}

				//The other fields defined in the default app-config should still be present
				assert.Equal(t, "My Awesome App",
					res["my-default-app-config-1.yaml"]["app"].(map[string]any)["title"].(string))
				assert.NotNil(t, res["my-default-app-config-1.yaml"]["plugin1"].(map[string]any))

				assert.Equal(t, "My Company",
					res["my-default-app-config-2.yaml"]["organization"].(map[string]any)["name"].(string))
				assert.True(t,
					res["my-default-app-config-2.yaml"]["backend"].(map[string]any)["auth"].(map[string]any)["dangerouslyDisableDefaultAuthPolicy"].(bool))
				assert.True(t,
					res["my-default-app-config-2.yaml"]["backend"].(map[string]any)["cors"].(map[string]any)["credentials"].(bool))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &BackstageRoute{}
			b.updateAppConfigWithBaseUrls(tt.args.model, tt.args.backstage)
			updatedAppConfigMaps := make(map[string]map[string]any)
			if tt.args.model.appConfig != nil && tt.args.model.appConfig.ConfigMap != nil {
				for k, v := range tt.args.model.appConfig.ConfigMap.Data {
					var appConfig map[string]any
					err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(v)), 1000).Decode(&appConfig)
					assert.NoError(t, err)
					updatedAppConfigMaps[k] = appConfig
				}
			}
			tt.assertFn(t, updatedAppConfigMaps)
		})
	}
}
