package model

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
	corev1 "k8s.io/api/core/v1"
)

const SecretFilesObjectKey = "secret-files.yaml"

type SecretFilesFactory struct{}

func (f SecretFilesFactory) newBackstageObject() RuntimeObject {
	return &SecretFiles{}
}

type SecretFiles struct {
	secrets *multiobject.MultiObject
	model   *BackstageModel
}

func init() {
	registerConfig(SecretFilesObjectKey, SecretFilesFactory{}, true, nil)
}

func (p *SecretFiles) addExternalConfig(spec api.BackstageSpec) error {

	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.Secrets == nil {
		return nil
	}

	for _, specSec := range spec.Application.ExtraFiles.Secrets {

		if specSec.MountPath == "" && specSec.Key == "" {
			return fmt.Errorf("key or mountPath has to be specified for secret %s", specSec.Name)
		}
		mp, wSubpath := p.model.backstageDeployment.mountPath(specSec.MountPath, specSec.Key, spec.Application.ExtraFiles.MountPath)
		keys := p.model.ExternalConfig.ExtraFileSecretKeys[specSec.Name].All()
		err := p.model.backstageDeployment.mountFilesFrom(containersFilter{names: specSec.Containers}, SecretObjectKind,
			specSec.Name, mp, specSec.Key, wSubpath, keys)
		if err != nil {
			return fmt.Errorf("failed to mount files on secret %s: %w", specSec.Name, err)
		}
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) Object() runtime.Object {
	return p.secrets
}

// implementation of RuntimeObject interface
func (p *SecretFiles) setObject(obj runtime.Object) {
	p.secrets = nil
	if obj != nil {
		p.secrets = obj.(*multiobject.MultiObject)
	}
}

// implementation of RuntimeObject interface
func (p *SecretFiles) addToModel(model *BackstageModel, _ api.Backstage) (bool, error) {
	p.model = model
	if p.secrets != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) updateAndValidate(_ api.Backstage) error {

	for _, item := range p.secrets.Items {
		secret, ok := item.(*corev1.Secret)
		if !ok {
			return fmt.Errorf("payload is not Secret kind: %T", item)
		}

		keys := append(utils.SortedKeys(secret.Data), utils.SortedKeys(secret.StringData)...)
		mountPath, subPath, fileName := p.model.backstageDeployment.getDefConfigMountPath(item)
		err := p.model.backstageDeployment.mountFilesFrom(containersFilter{annotation: item.GetAnnotations()[ContainersAnnotation]}, SecretObjectKind,
			item.GetName(), mountPath, fileName, subPath != "", keys)
		if err != nil {
			return fmt.Errorf("failed to add files from secret %s: %w", item.GetName(), err)
		}
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	setMultiObjectConfigMetaInfo(p.secrets, "files", backstage, scheme)
}
