package model

import (
	"path/filepath"

	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type AppConfigFactory struct{}

// factory method to create App Config object
func (f AppConfigFactory) newBackstageObject() RuntimeObject {
	return &AppConfig{}
}

// structure containing ConfigMap where keys are Backstage ConfigApp file names and vaues are contents of the files
// Mount path is a patch to the follder to place the files to
type AppConfig struct {
	ConfigMap *corev1.ConfigMap
	model     *BackstageModel
}

func init() {
	registerConfig("app-config.yaml", AppConfigFactory{}, false)
}

func AppConfigDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-appconfig")
}

func (b *AppConfig) addExternalConfig(spec bsv1.BackstageSpec) error {

	if spec.Application == nil || spec.Application.AppConfig == nil || spec.Application.AppConfig.ConfigMaps == nil {
		return nil
	}

	for _, specCm := range spec.Application.AppConfig.ConfigMaps {
		mp, wSubpath := b.model.backstageDeployment.mountPath(specCm.MountPath, specCm.Key, spec.Application.AppConfig.MountPath)
		updatePodWithAppConfig(b.model.backstageDeployment, b.model.backstageDeployment.container(), specCm.Name,
			mp, specCm.Key, wSubpath, b.model.ExternalConfig.AppConfigKeys[specCm.Name])
	}
	return nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) Object() runtime.Object {
	return b.ConfigMap
}

// implementation of RuntimeObject interface
func (b *AppConfig) setObject(obj runtime.Object) {
	b.ConfigMap = nil
	if obj != nil {
		b.ConfigMap = obj.(*corev1.ConfigMap)
	}
}

// implementation of RuntimeObject interface
func (b *AppConfig) EmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (b *AppConfig) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {
	b.model = model
	if b.ConfigMap != nil {
		model.appConfig = b
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) updateAndValidate(backstage bsv1.Backstage) error {
	updatePodWithAppConfig(b.model.backstageDeployment, b.model.backstageDeployment.container(), b.ConfigMap.Name,
		b.model.backstageDeployment.defaultMountPath(), "", true, maps.Keys(b.ConfigMap.Data))
	return nil
}

func (b *AppConfig) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.ConfigMap.SetName(AppConfigDefaultName(backstage.Name))
	setMetaInfo(b.ConfigMap, backstage, scheme)
}

// updatePodWithAppConfig contributes to Volumes, container.VolumeMounts and container.Args
func updatePodWithAppConfig(bsd *BackstageDeployment, container *corev1.Container, cmName, mountPath, key string, withSubPath bool, cmData []string) {
	bsd.mountFilesFrom([]string{container.Name}, ConfigMapObjectKind,
		cmName, mountPath, key, withSubPath, cmData)

	for _, file := range cmData {
		if key == "" || key == file {
			container.Args = append(container.Args, []string{"--config", filepath.Join(mountPath, file)}...)
		}
	}
}
