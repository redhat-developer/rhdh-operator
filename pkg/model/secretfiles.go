package model

import (
	"fmt"

	"golang.org/x/exp/maps"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type SecretFilesFactory struct{}

func (f SecretFilesFactory) newBackstageObject() RuntimeObject {
	return &SecretFiles{}
}

type SecretFiles struct {
	Secret *corev1.Secret
}

func init() {
	registerConfig("secret-files.yaml", SecretFilesFactory{}, false)
}

func addSecretFilesFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {

	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.Secrets == nil {
		return nil
	}

	for _, specSec := range spec.Application.ExtraFiles.Secrets {

		if specSec.MountPath == "" && specSec.Key == "" {
			return fmt.Errorf("key is required if defaultMountPath is not specified for secret %s", specSec.Name)
		}
		mp, wSubpath := model.backstageDeployment.mountPath(specSec.MountPath, specSec.Key, spec.Application.ExtraFiles.MountPath)
		keys := model.ExternalConfig.ExtraFileSecretKeys[specSec.Name].All()
		utils.MountFilesFrom(&model.backstageDeployment.deployment.Spec.Template.Spec, model.backstageDeployment.container(), utils.SecretObjectKind,
			specSec.Name, mp, specSec.Key, wSubpath, keys)
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
func (p *SecretFiles) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {

	keys := append(maps.Keys(p.Secret.Data), maps.Keys(p.Secret.StringData)...)
	utils.MountFilesFrom(&m.backstageDeployment.deployment.Spec.Template.Spec, m.backstageDeployment.container(), utils.SecretObjectKind,
		p.Secret.Name, m.backstageDeployment.defaultMountPath(), "", true, keys)
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.Secret.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"))
	setMetaInfo(p.Secret, backstage, scheme)
}
