package model

import (
	"fmt"
	"os"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
)

const LocalDbImageEnvVar = "RELATED_IMAGE_postgresql"

type DbStatefulSetFactory struct{}

func (f DbStatefulSetFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("fb24beb25802d35698")
	return &DbStatefulSet{}
}

type DbStatefulSet struct {
	statefulSet *appsv1.StatefulSet
}

func init() {
	__sealights__.TraceFunc("c42d7652ce6388d0a7")
	registerConfig("db-statefulset.yaml", DbStatefulSetFactory{}, false)
}

func DbStatefulSetName(backstageName string) string {
	__sealights__.TraceFunc("161e469361f81bc844")
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql")
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) Object() runtime.Object {
	__sealights__.TraceFunc("51c40e15b0b521effe")
	return b.statefulSet
}

func (b *DbStatefulSet) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("7d79f5946336c39b26")
	b.statefulSet = nil
	if obj != nil {
		b.statefulSet = obj.(*appsv1.StatefulSet)
	}
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("4b7454c08e08959b0f")
	if b.statefulSet == nil {
		if model.localDbEnabled {
			return false, fmt.Errorf("LocalDb StatefulSet not configured, make sure there is db-statefulset.yaml in default or raw configuration")
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
	__sealights__.TraceFunc("28fa73fdeb8a576445")
	return &appsv1.StatefulSet{}
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) updateAndValidate(model *BackstageModel, backstage bsv1.Backstage) error {
	__sealights__.TraceFunc("7c841e7c7bbbb00fd8")

	// point ServiceName to localDb
	b.statefulSet.Spec.ServiceName = model.LocalDbService.service.Name

	if backstage.Spec.Application != nil && backstage.Spec.Application.ImagePullSecrets != nil {
		utils.SetImagePullSecrets(b.podSpec(), backstage.Spec.Application.ImagePullSecrets)
	}

	if backstage.Spec.IsAuthSecretSpecified() {
		b.setDbSecretEnvVar(b.container(), backstage.Spec.Database.AuthSecretName)
	} else if model.LocalDbSecret != nil {
		b.setDbSecretEnvVar(b.container(), model.LocalDbSecret.secret.Name)
	}
	return nil
}

func (b *DbStatefulSet) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("a263e0500b4dd3cd6d")
	b.statefulSet.SetName(DbStatefulSetName(backstage.Name))
	utils.GenerateLabel(&b.statefulSet.Spec.Template.ObjectMeta.Labels, BackstageAppLabel, utils.BackstageDbAppLabelValue(backstage.Name))
	utils.GenerateLabel(&b.statefulSet.Spec.Selector.MatchLabels, BackstageAppLabel, utils.BackstageDbAppLabelValue(backstage.Name))
	setMetaInfo(b.statefulSet, backstage, scheme)
}

// returns DB container
func (b *DbStatefulSet) container() *corev1.Container {
	__sealights__.TraceFunc("5e641b5d507a0426b1")
	return &b.podSpec().Containers[0]
}

// returns DB pod
func (b *DbStatefulSet) podSpec() *corev1.PodSpec {
	__sealights__.TraceFunc("849eddcd3493bca862")
	return &b.statefulSet.Spec.Template.Spec
}

func (b *DbStatefulSet) setDbSecretEnvVar(container *corev1.Container, secretName string) {
	__sealights__.TraceFunc("d61bae789062824d73")
	//AddEnvVarsFrom(container, SecretObjectKind, secretName, "")
	envFromSrc := corev1.EnvFromSource{}
	envFromSrc.SecretRef = &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: secretName}}
	container.EnvFrom = append(container.EnvFrom, envFromSrc)
}
