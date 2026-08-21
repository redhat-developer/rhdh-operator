package model

import (
	openshift "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
)

type OkpRouteFactory struct{}

func (f OkpRouteFactory) newBackstageObject() RuntimeObject {
	return &OkpRoute{}
}

// OkpRoute is the standalone OpenShift Route exposing the OKP Service. Route is inherently
// OpenShift-only, so it is applied only when the lightspeed flavour is enabled AND the
// platform is OpenShift.
type OkpRoute struct {
	route *openshift.Route
	model *BackstageModel
}

func init() {
	registerConfig(OkpRouteKey, OkpRouteFactory{}, false, mergeOkpObject)
}

// implementation of RuntimeObject interface
func (o *OkpRoute) Object() runtime.Object {
	if o.route == nil {
		return nil
	}
	return o.route
}

// implementation of RuntimeObject interface
func (o *OkpRoute) GetKey() string {
	return OkpRouteKey
}

// implementation of RuntimeObject interface
func (o *OkpRoute) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	o.model = model
	if config != nil {
		o.route = config.(*openshift.Route)
	}

	// OKP is OpenShift-only; drop the Route on vanilla K8s.
	if !model.isOpenshift {
		o.route = nil
	}

	// Always add the wrapper (placeholder pattern); Object() returns nil when not applicable.
	model.setRuntimeObject(o)

	if o.route != nil {
		o.setMetaInfo(backstage, scheme)
	}
	return nil
}

// implementation of RuntimeObject interface
func (o *OkpRoute) updateAndValidate(_ api.Backstage, _ *runtime.Scheme) error {
	return nil
}

func (o *OkpRoute) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	name := OkpName(backstage.Name)
	o.route.SetName(name)
	o.route.SetLabels(okpLabels(backstage.Name))
	// Point the Route at the OKP Service (same deterministic name).
	o.route.Spec.To.Name = name
	setMetaInfo(o.route, backstage, scheme)
}
