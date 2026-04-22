package model

import (
	"os"

	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
)

const LocalDbImageEnvVar = "RELATED_IMAGE_postgresql"

type DbStatefulSetFactory struct{}

func (f DbStatefulSetFactory) newBackstageObject() RuntimeObject {
	return &DbStatefulSet{}
}

type DbStatefulSet struct {
	statefulSet *appsv1.StatefulSet
	model       *BackstageModel
}

func init() {
	registerConfig(DbStatefulSetKey, DbStatefulSetFactory{}, false, nil)
}

func DbStatefulSetName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql")
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) Object() runtime.Object {
	if b.statefulSet == nil {
		return nil
	}
	return b.statefulSet
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	b.model = model

	// Only set statefulSet if localDb is enabled
	if model.localDbEnabled && config != nil {
		b.statefulSet = config.(*appsv1.StatefulSet)
	}

	// Always add wrapper to model (unconditional)
	model.setRuntimeObject(DbStatefulSetKey, b)

	// Only set metadata if underlying object exists
	if b.statefulSet != nil {
		// override image with env var
		// [GA] Do we really need this feature?
		if os.Getenv(LocalDbImageEnvVar) != "" {
			b.container().Image = os.Getenv(LocalDbImageEnvVar)
		}
		b.setMetaInfo(backstage, scheme)
	}

	return nil
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
	if b.statefulSet == nil {
		return nil
	}

	// point ServiceName to localDb
	if dbService := b.model.GetRuntimeObject(DbServiceKey); dbService != nil {
		service := dbService.(*DbService).service
		if service != nil {
			b.statefulSet.Spec.ServiceName = service.Name
		}
	}

	if backstage.Spec.IsAuthSecretSpecified() {
		b.setDbSecretEnvVar(b.container(), backstage.Spec.Database.AuthSecretName)
	} else if dbSecret := b.model.GetRuntimeObject(DbSecretKey); dbSecret != nil {
		secret := dbSecret.(*DbSecret).secret
		if secret != nil {
			b.setDbSecretEnvVar(b.container(), secret.Name)
		}
	}
	return nil
}

func (b *DbStatefulSet) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	b.statefulSet.SetName(DbStatefulSetName(backstage.Name))
	utils.GenerateLabel(&b.statefulSet.Spec.Template.Labels, BackstageAppLabel, utils.BackstageDbAppLabelValue(backstage.Name))
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

func (b *DbStatefulSet) setDbSecretEnvVar(container *corev1.Container, secretName string) {
	//AddEnvVarsFrom(container, SecretObjectKind, secretName, "")
	envFromSrc := corev1.EnvFromSource{}
	envFromSrc.SecretRef = &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: secretName}}
	container.EnvFrom = append(container.EnvFrom, envFromSrc)
}
