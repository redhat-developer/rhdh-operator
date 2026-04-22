package model

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"golang.org/x/exp/maps"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-developer/rhdh-operator/api"
	corev1 "k8s.io/api/core/v1"
)

type SecretFilesFactory struct{}

func (f SecretFilesFactory) newBackstageObject() RuntimeObject {
	return &SecretFiles{}
}

type SecretFiles struct {
	secrets *multiobject.MultiObject
	model   *BackstageModel
}

func init() {
	registerConfig(SecretFilesKey, SecretFilesFactory{}, true, nil)
}

// implementation of RuntimeObject interface
func (p *SecretFiles) Object() runtime.Object {
	if p.secrets != nil && len(p.secrets.Items) > 0 {
		return p.secrets
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	p.model = model
	if config != nil {
		p.secrets = config.(*multiobject.MultiObject)
	} else {
		// Create empty secrets - might be populated later from spec
		p.secrets = &multiobject.MultiObject{Items: []client.Object{}}
	}

	// Always add to model so updateAndValidate is called (may process spec secrets)
	model.setRuntimeObject(SecretFilesKey, p)
	if p.secrets != nil && len(p.secrets.Items) > 0 {
		p.setMetaInfo(backstage, scheme)
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
	deployment := p.model.getDeployment()
	if deployment == nil {
		return fmt.Errorf("backstage deployment not found in model")
	}

	// Process secrets from config files
	if p.secrets != nil {
		for _, item := range p.secrets.Items {
			secret, ok := item.(*corev1.Secret)
			if !ok {
				return fmt.Errorf("payload is not Secret kind: %T", item)
			}

			keys := append(maps.Keys(secret.Data), maps.Keys(secret.StringData)...)
			mountPath, subPath, fileName := deployment.getDefConfigMountPath(item)
			err := deployment.mountFilesFrom(containersFilter{annotation: item.GetAnnotations()[ContainersAnnotation]}, SecretObjectKind,
				item.GetName(), mountPath, fileName, subPath != "", keys)
			if err != nil {
				return fmt.Errorf("failed to add files from secret %s: %w", item.GetName(), err)
			}
		}
	}

	// Process secrets from CR spec (formerly addExternalConfig)
	if backstage.Spec.Application != nil && backstage.Spec.Application.ExtraFiles != nil && backstage.Spec.Application.ExtraFiles.Secrets != nil {
		for _, specSec := range backstage.Spec.Application.ExtraFiles.Secrets {

			if specSec.MountPath == "" && specSec.Key == "" {
				return fmt.Errorf("key or mountPath has to be specified for secret %s", specSec.Name)
			}
			mp, wSubpath := deployment.mountPath(specSec.MountPath, specSec.Key, backstage.Spec.Application.ExtraFiles.MountPath)
			keys := p.model.ExternalConfig.ExtraFileSecretKeys[specSec.Name].All()
			err := deployment.mountFilesFrom(containersFilter{names: specSec.Containers}, SecretObjectKind,
				specSec.Name, mp, specSec.Key, wSubpath, keys)
			if err != nil {
				return fmt.Errorf("failed to mount files on secret %s: %w", specSec.Name, err)
			}
		}
	}

	return nil
}

// implementation of RuntimeObject interface
func (p *SecretFiles) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	setMultiObjectConfigMetaInfo(p.secrets, "files", backstage, scheme)
}
