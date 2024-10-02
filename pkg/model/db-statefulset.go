package model

import (
	"fmt"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
)

const LocalDbImageEnvVar = "RELATED_IMAGE_postgresql"

type DbStatefulSetFactory struct{}

func (f DbStatefulSetFactory) newBackstageObject() RuntimeObject {
	return &DbStatefulSet{}
}

type DbStatefulSet struct {
	statefulSet *appsv1.StatefulSet
}

func init() {
	registerConfig("db-statefulset.yaml", DbStatefulSetFactory{}, false)
}

func DbStatefulSetName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql")
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) Object() runtime.Object {
	return b.statefulSet
}

func (b *DbStatefulSet) setObject(obj runtime.Object) {
	b.statefulSet = nil
	if obj != nil {
		b.statefulSet = obj.(*appsv1.StatefulSet)
	}
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	if b.statefulSet == nil {
		if model.localDbEnabled {
			return false, fmt.Errorf("LocalDb StatefulSet not configured, make sure there is db-statefulset.yaml.yaml in default or raw configuration")
		}
		return false, nil
	} else {
		if !model.localDbEnabled {
			return false, nil
		}
	}

	model.localDbStatefulSet = b
	model.setRuntimeObject(b)

	// override image with env var
	// [GA] Do we really need this feature?
	if os.Getenv(LocalDbImageEnvVar) != "" {
		b.container().Image = os.Getenv(LocalDbImageEnvVar)
	}

	return true, nil
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) EmptyObject() client.Object {
	return &appsv1.StatefulSet{}
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) validate(model *BackstageModel, backstage bsv1.Backstage) error {

	// point ServiceName to localDb
	b.statefulSet.Spec.ServiceName = model.LocalDbService.service.Name

	if backstage.Spec.Application != nil && backstage.Spec.Application.ImagePullSecrets != nil {
		utils.SetImagePullSecrets(b.podSpec(), backstage.Spec.Application.ImagePullSecrets)
	}

	if backstage.Spec.IsAuthSecretSpecified() {
		utils.SetDbSecretEnvVar(b.container(), backstage.Spec.Database.AuthSecretName)
	} else if model.LocalDbSecret != nil {
		utils.SetDbSecretEnvVar(b.container(), model.LocalDbSecret.secret.Name)
	}
	return nil
}

func (b *DbStatefulSet) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.statefulSet.SetName(DbStatefulSetName(backstage.Name))
	utils.GenerateLabel(&b.statefulSet.Spec.Template.ObjectMeta.Labels, BackstageAppLabel, utils.BackstageDbAppLabelValue(backstage.Name))
	utils.GenerateLabel(&b.statefulSet.Spec.Selector.MatchLabels, BackstageAppLabel, utils.BackstageDbAppLabelValue(backstage.Name))
	setMetaInfo(b.statefulSet, backstage, scheme)
}

// returns DB container
func (b *DbStatefulSet) container() *corev1.Container {
	return &b.podSpec().Containers[0]
}

// returns DB pod
func (b *DbStatefulSet) podSpec() *corev1.PodSpec {
	return &b.statefulSet.Spec.Template.Spec
}
