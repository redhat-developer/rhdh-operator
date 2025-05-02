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

type DbServiceFactory struct{}

func (f DbServiceFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("8c32b269bff2dedbcd")
	return &DbService{}
}

type DbService struct {
	service *corev1.Service
}

func init() {
	__sealights__.TraceFunc("9ba383613a651483ba")
	registerConfig("db-service.yaml", DbServiceFactory{}, false)
}

func DbServiceName(backstageName string) string {
	__sealights__.TraceFunc("961c094392d77e787e")
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql")
}

// implementation of RuntimeObject interface
func (b *DbService) Object() runtime.Object {
	__sealights__.TraceFunc("faf0d3560acff6e3b0")
	return b.service
}

func (b *DbService) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("1425d1390f1dcc15fa")
	b.service = nil
	if obj != nil {
		b.service = obj.(*corev1.Service)
	}
}

// implementation of RuntimeObject interface
func (b *DbService) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("b68bf190c535d2d0cc")
	if b.service == nil {
		if model.localDbEnabled {
			return false, fmt.Errorf("LocalDb Service not initialized, make sure there is db-service.yaml in default or raw configuration")
		}
		return false, nil
	} else {
		if !model.localDbEnabled {
			return false, nil
		}
	}

	// force this service to be headless even if it is not set in the original config
	b.service.Spec.ClusterIP = corev1.ClusterIPNone

	model.LocalDbService = b
	model.setRuntimeObject(b)

	return true, nil
}

// implementation of RuntimeObject interface
func (b *DbService) EmptyObject() client.Object {
	__sealights__.TraceFunc("f98ed5c9020ada829b")
	return &corev1.Service{}
}

// implementation of RuntimeObject interface
func (b *DbService) updateAndValidate(_ *BackstageModel, _ bsv1.Backstage) error {
	__sealights__.TraceFunc("6b8fd81b1fb0d425d6")
	return nil
}

func (b *DbService) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("7bf88af9c2e71a4656")
	b.service.SetName(DbServiceName(backstage.Name))
	utils.GenerateLabel(&b.service.Spec.Selector, BackstageAppLabel, utils.BackstageDbAppLabelValue(backstage.Name))
	setMetaInfo(b.service, backstage, scheme)
}
