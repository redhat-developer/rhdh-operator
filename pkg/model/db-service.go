package model

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type DbServiceFactory struct{}

func (f DbServiceFactory) newBackstageObject() RuntimeObject {
	return &DbService{}
}

type DbService struct {
	service *corev1.Service
	model   *BackstageModel
}

func init() {
	registerConfig(DbServiceKey, DbServiceFactory{}, false, nil)
}

func DbServiceName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql")
}

// implementation of RuntimeObject interface
func (b *DbService) Object() runtime.Object {
	if b.service == nil {
		return nil
	}
	return b.service
}

// implementation of RuntimeObject interface
func (b *DbService) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	b.model = model

	// Only set service if localDb is enabled
	if model.localDbEnabled && config != nil {
		b.service = config.(*corev1.Service)
	}

	// Always add wrapper to model (unconditional)
	model.setRuntimeObject(DbServiceKey, b)

	// Only set metadata if underlying object exists
	if b.service != nil {
		// force this service to be headless even if it is not set in the original config
		b.service.Spec.ClusterIP = corev1.ClusterIPNone
		b.setMetaInfo(backstage, scheme)
	}

	return nil
}

// implementation of RuntimeObject interface
func (b *DbService) updateAndValidate(_ api.Backstage, _ *runtime.Scheme) error {
	return nil
}

func (b *DbService) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	b.service.SetName(DbServiceName(backstage.Name))
	utils.GenerateLabel(&b.service.Spec.Selector, BackstageAppLabel, utils.BackstageDbAppLabelValue(backstage.Name))
	setMetaInfo(b.service, backstage, scheme)
}
