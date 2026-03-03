package model

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type BackstageServiceFactory struct{}

func (f BackstageServiceFactory) newBackstageObject() RuntimeObject {
	return &BackstageService{}
}

type BackstageService struct {
	service *corev1.Service
	model   *BackstageModel
}

func init() {
	registerConfig("service.yaml", BackstageServiceFactory{}, false, FlavourMergePolicyNoFlavour)
}

func ServiceName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage")
}

// implementation of RuntimeObject interface
func (b *BackstageService) Object() runtime.Object {
	return b.service
}

func (b *BackstageService) setObject(obj runtime.Object) {
	b.service = nil
	if obj != nil {
		b.service = obj.(*corev1.Service)
	}
}

// implementation of RuntimeObject interface
func (b *BackstageService) addToModel(model *BackstageModel, _ api.Backstage) (bool, error) {
	b.model = model
	if b.service == nil {
		return false, fmt.Errorf("backstage Service is not initialized, make sure there is service.yaml in default or raw configuration")
	}
	model.backstageService = b
	model.setRuntimeObject(b)

	return true, nil

}

// implementation of RuntimeObject interface
func (b *BackstageService) updateAndValidate(_ api.Backstage) error {
	return nil
}

func (b *BackstageService) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	b.service.SetName(ServiceName(backstage.Name))
	utils.GenerateLabel(&b.service.Spec.Selector, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	setMetaInfo(b.service, backstage, scheme)
}
