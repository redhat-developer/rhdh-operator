package model

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
)

type OkpDeploymentFactory struct{}

func (f OkpDeploymentFactory) newBackstageObject() RuntimeObject {
	return &OkpDeployment{}
}

// OkpDeployment is the standalone OKP Deployment (NOT merged into the main Backstage pod).
// It is applied only when the lightspeed flavour is enabled (flavour gating is implicit:
// the config is sourced only from the lightspeed flavour dir) AND the platform is OpenShift.
type OkpDeployment struct {
	deployment *appsv1.Deployment
	model      *BackstageModel
}

func init() {
	registerConfig(OkpDeploymentKey, OkpDeploymentFactory{}, false, mergeOkpObject)
}

// implementation of RuntimeObject interface
func (o *OkpDeployment) Object() runtime.Object {
	if o.deployment == nil {
		return nil
	}
	return o.deployment
}

// implementation of RuntimeObject interface
func (o *OkpDeployment) GetKey() string {
	return OkpDeploymentKey
}

// implementation of RuntimeObject interface
func (o *OkpDeployment) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	o.model = model
	if config != nil {
		o.deployment = config.(*appsv1.Deployment)
	}

	// OKP is OpenShift-only; drop the object on vanilla K8s.
	if !model.isOpenshift {
		o.deployment = nil
	}

	// Always add the wrapper (placeholder pattern); Object() returns nil when not applicable.
	model.setRuntimeObject(o)

	if o.deployment != nil {
		o.setMetaInfo(backstage, scheme)
	}
	return nil
}

// implementation of RuntimeObject interface
func (o *OkpDeployment) updateAndValidate(_ api.Backstage, _ *runtime.Scheme) error {
	return nil
}

func (o *OkpDeployment) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	o.deployment.SetName(OkpName(backstage.Name))
	o.deployment.SetLabels(okpLabels(backstage.Name))
	if o.deployment.Spec.Selector == nil {
		o.deployment.Spec.Selector = &metav1.LabelSelector{}
	}
	o.deployment.Spec.Selector.MatchLabels = okpSelectorLabels(backstage.Name)
	o.deployment.Spec.Template.Labels = okpSelectorLabels(backstage.Name)
	setMetaInfo(o.deployment, backstage, scheme)
}
