package model

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
)

type OkpServiceFactory struct{}

func (f OkpServiceFactory) newBackstageObject() RuntimeObject {
	return &OkpService{}
}

// OkpService is the standalone ClusterIP Service fronting the OKP Deployment.
// Applied only when the lightspeed flavour is enabled AND the platform is OpenShift.
type OkpService struct {
	service *corev1.Service
	model   *BackstageModel
}

func init() {
	registerConfig(OkpServiceKey, OkpServiceFactory{}, false, mergeOkpObject)
}

// implementation of RuntimeObject interface
func (o *OkpService) Object() runtime.Object {
	if o.service == nil {
		return nil
	}
	return o.service
}

// implementation of RuntimeObject interface
func (o *OkpService) GetKey() string {
	return OkpServiceKey
}

// implementation of RuntimeObject interface
func (o *OkpService) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	o.model = model
	if config != nil {
		o.service = config.(*corev1.Service)
	}

	// OKP is OpenShift-only; drop the object on vanilla K8s.
	if !model.isOpenshift {
		o.service = nil
	}

	// Always add the wrapper (placeholder pattern); Object() returns nil when not applicable.
	model.setRuntimeObject(o)

	if o.service != nil {
		o.setMetaInfo(backstage, scheme)
	}
	return nil
}

// implementation of RuntimeObject interface
func (o *OkpService) updateAndValidate(_ api.Backstage, _ *runtime.Scheme) error {
	return nil
}

func (o *OkpService) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	o.service.SetName(OkpName(backstage.Name))
	o.service.SetLabels(okpLabels(backstage.Name))
	o.service.Spec.Selector = okpSelectorLabels(backstage.Name)
	setMetaInfo(o.service, backstage, scheme)
}
