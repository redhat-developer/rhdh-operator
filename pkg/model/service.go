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
	registerConfig(ServiceKey, BackstageServiceFactory{}, false, nil)
}

func ServiceName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage")
}

// implementation of RuntimeObject interface
func (b *BackstageService) Object() runtime.Object {
	return b.service
}

// implementation of RuntimeObject interface
func (b *BackstageService) GetKey() string {
	return ServiceKey
}

// implementation of RuntimeObject interface
func (b *BackstageService) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	b.model = model
	if config != nil {
		b.service = config.(*corev1.Service)
	}
	if b.service == nil {
		return fmt.Errorf("backstage Service is not initialized, make sure there is service.yaml in default or raw configuration")
	}
	// Service is required, so always add to model
	model.setRuntimeObject(b)
	b.setMetaInfo(backstage, scheme)
	return nil
}

// implementation of RuntimeObject interface
func (b *BackstageService) updateAndValidate(_ api.Backstage, _ *runtime.Scheme) error {
	return nil
}

func (b *BackstageService) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	b.service.SetName(ServiceName(backstage.Name))
	utils.GenerateLabel(&b.service.Spec.Selector, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	setMetaInfo(b.service, backstage, scheme)
}
