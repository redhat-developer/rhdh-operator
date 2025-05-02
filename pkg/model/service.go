package model

import (
	"fmt"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type BackstageServiceFactory struct{}

func (f BackstageServiceFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("7d0db9e0ecc975456f")
	return &BackstageService{}
}

type BackstageService struct {
	service *corev1.Service
}

func init() {
	__sealights__.TraceFunc("2623b0ad73eb0a121a")
	registerConfig("service.yaml", BackstageServiceFactory{}, false)
}

func ServiceName(backstageName string) string {
	__sealights__.TraceFunc("b83b9f3d5b61827964")
	return utils.GenerateRuntimeObjectName(backstageName, "backstage")
}

// implementation of RuntimeObject interface
func (b *BackstageService) Object() runtime.Object {
	__sealights__.TraceFunc("7a69c9d16e582ce9e1")
	return b.service
}

func (b *BackstageService) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("c6204989b7c0527f9f")
	b.service = nil
	if obj != nil {
		b.service = obj.(*corev1.Service)
	}
}

// implementation of RuntimeObject interface
func (b *BackstageService) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("f54ff1a70d7505f1d5")
	if b.service == nil {
		return false, fmt.Errorf("Backstage Service is not initialized, make sure there is service.yaml in default or raw configuration")
	}
	model.backstageService = b
	model.setRuntimeObject(b)

	return true, nil

}

// implementation of RuntimeObject interface
func (b *BackstageService) EmptyObject() client.Object {
	__sealights__.TraceFunc("be4b02e5e8a2370a20")
	return &corev1.Service{}
}

// implementation of RuntimeObject interface
func (b *BackstageService) updateAndValidate(_ *BackstageModel, _ bsv1.Backstage) error {
	__sealights__.TraceFunc("46a285002a5c8cbd2a")
	return nil
}

func (b *BackstageService) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("7a7d80415b7e8760df")
	b.service.SetName(ServiceName(backstage.Name))
	utils.GenerateLabel(&b.service.Spec.Selector, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	setMetaInfo(b.service, backstage, scheme)
}
