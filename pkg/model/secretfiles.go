package model

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type SecretFilesFactory struct{}

func (f SecretFilesFactory) newBackstageObject() RuntimeObject {
	return &SecretFiles{MountPath: defaultMountDir}
}

type SecretFiles struct {
	Secret    *corev1.Secret
	MountPath string
	Key       string
}

func init() {
	registerConfig("secret-files.yaml", SecretFilesFactory{}, false)
}

func addSecretFiles(spec bsv1.BackstageSpec, deployment *appsv1.Deployment) error {

	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.Secrets == nil {
		return nil
	}
	mp := defaultMountDir
	if spec.Application.ExtraFiles.MountPath != "" {
		mp = spec.Application.ExtraFiles.MountPath
	}

	for _, sec := range spec.Application.ExtraFiles.Secrets {
		if sec.Key == "" {
			return fmt.Errorf("key is required to mount extra file with secret %s", sec.Name)
		}
		sf := SecretFiles{
			Secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: sec.Name},
				// TODO it is not correct, there may not be such a secret key
				// it is done for 0.1.0 compatibility only
				StringData: map[string]string{sec.Key: ""},
			},
			MountPath: mp,
			Key:       sec.Key,
		}
		sf.updatePod(deployment)
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) Object() runtime.Object {
	return p.Secret
}

func (p *SecretFiles) setObject(obj runtime.Object) {
	p.Secret = nil
	if obj != nil {
		p.Secret = obj.(*corev1.Secret)
	}
}

// implementation of RuntimeObject interface
func (p *SecretFiles) EmptyObject() client.Object {
	return &corev1.Secret{}
}

// implementation of RuntimeObject interface
func (p *SecretFiles) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	if p.Secret != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) validate(_ *BackstageModel, _ bsv1.Backstage) error {
	return nil
}

func (p *SecretFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.Secret.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"))
	setMetaInfo(p.Secret, backstage, scheme)
}

// implementation of BackstagePodContributor interface
func (p *SecretFiles) updatePod(depoyment *appsv1.Deployment) {

	utils.MountFilesFrom(&depoyment.Spec.Template.Spec, &depoyment.Spec.Template.Spec.Containers[0], utils.SecretObjectKind,
		p.Secret.Name, p.MountPath, p.Key, p.Secret.StringData)
}
