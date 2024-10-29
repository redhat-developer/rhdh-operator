package model

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	appsv1 "k8s.io/api/apps/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type SecretFilesFactory struct{}

func (f SecretFilesFactory) newBackstageObject() RuntimeObject {
	return &SecretFiles{MountPath: DefaultMountDir, withSubPath: true}
}

type SecretFiles struct {
	Secret      *corev1.Secret
	MountPath   string
	Key         string
	withSubPath bool
}

func init() {
	registerConfig("secret-files.yaml", SecretFilesFactory{}, false)
}

func addSecretFiles(spec bsv1.BackstageSpec, deployment *appsv1.Deployment, model *BackstageModel) error {

	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.Secrets == nil {
		return nil
	}

	for _, sec := range spec.Application.ExtraFiles.Secrets {

		if sec.MountPath == "" && sec.Key == "" {
			return fmt.Errorf("key is required if mountPath is not specified for secret %s", sec.Name)
		}

		mp, wSubpath := GetMountPath(sec, spec.Application.ExtraFiles.MountPath)

		efs := model.ExternalConfig.ExtraFileSecrets[sec.Name]
		sf := SecretFiles{
			Secret:      &efs,
			MountPath:   mp,
			Key:         sec.Key,
			withSubPath: wSubpath,
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
		p.Secret.Name, p.MountPath, p.Key, p.withSubPath, p.Secret.StringData)
}
