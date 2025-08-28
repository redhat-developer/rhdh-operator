package model

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"

	"golang.org/x/exp/maps"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

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
	registerConfig(SecretFilesObjectKey, SecretFilesFactory{}, true)
}

func (p *SecretFiles) addExternalConfig(spec bsv1.BackstageSpec) error {

	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.Secrets == nil {
		return nil
	}

	for _, specSec := range spec.Application.ExtraFiles.Secrets {

		if specSec.MountPath == "" && specSec.Key == "" {
			return fmt.Errorf("key is required if defaultMountPath is not specified for secret %s", specSec.Name)
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
func (p *SecretFiles) EmptyObject() client.Object {
	return &corev1.Secret{}
}

// implementation of RuntimeObject interface
func (p *SecretFiles) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	p.model = model
	if p.secrets != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) updateAndValidate(_ bsv1.Backstage) error {

	for _, item := range p.secrets.Items {
		secret, ok := item.(*corev1.Secret)
		if !ok {
			return fmt.Errorf("payload is not Secret kind: %T", item)
		}

		keys := append(maps.Keys(secret.Data), maps.Keys(secret.StringData)...)
		mountPath, subPath := p.model.backstageDeployment.getDefConfigMountPath(item)
		//containers, err := p.model.backstageDeployment.filterContainerNames(utils.ParseCommaSeparated(item.GetAnnotations()[ContainersAnnotation]))
		err := p.model.backstageDeployment.mountFilesFrom(containersFilter{annotation: item.GetAnnotations()[ContainersAnnotation]}, SecretObjectKind,
			item.GetName(), mountPath, "", subPath != "", keys)
		if err != nil {
			return fmt.Errorf("failed to add files from secret %s: %w", item.GetName(), err)
		}
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {

	for _, item := range p.secrets.Items {
		secret := item.(*corev1.Secret)
		if len(p.secrets.Items) == 1 {
			// keep for backward compatibility
			secret.Name = utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files")
		} else {
			utils.AddAnnotation(secret, ConfiguredNameAnnotation, item.GetName())
			secret.Name = fmt.Sprintf("%s-%s", utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"), secret.Name)
		}
		setMetaInfo(secret, backstage, scheme)
	}
}
