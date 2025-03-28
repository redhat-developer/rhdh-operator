package model

import (
	"context"
	"testing"

	openshift "github.com/openshift/api/route/v1"

	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

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

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, testObj.scheme)

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
	model, err := InitObjects(context.TODO(), bs, testObjNoDef.externalConfig, true, testObjNoDef.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// check if what we have is what we specified in bs
	assert.Equal(t, RouteName(bs.Name), model.route.route.Name)
	assert.Equal(t, bs.Spec.Application.Route.Host, model.route.route.Spec.Host)

	// Test with default route configured
	testObjWithDef := testObjNoDef.addToDefaultConfig("route.yaml", "raw-route.yaml")
	model, err = InitObjects(context.TODO(), bs, testObjWithDef.externalConfig, true, testObjWithDef.scheme)

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
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, testObj.scheme)
	assert.NoError(t, err)
	assert.Nil(t, model.route)

	// W/o def route config
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, true, testObj.scheme)
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
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// W/o def route config - do not create route
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, true, testObj.scheme)
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
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

	// W/o def route config
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, true, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.route)

}

func Test_buildOpenShiftBaseUrl(t *testing.T) {
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
				buildOpenShiftBaseUrl(tt.args.model, tt.args.backstage),
				"buildOpenShiftBaseUrl(%v, %v)",
				tt.args.model, tt.args.backstage,
			)
		})
	}
}
