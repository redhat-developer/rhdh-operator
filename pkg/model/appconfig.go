package model

import (
	"path/filepath"

	"golang.org/x/exp/maps"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
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
}

func init() {
	registerConfig("app-config.yaml", AppConfigFactory{}, false)
}

func AppConfigDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-appconfig")
}

func addAppConfigsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {

	if spec.Application == nil || spec.Application.AppConfig == nil || spec.Application.AppConfig.ConfigMaps == nil {
		return nil
	}

	for _, specCm := range spec.Application.AppConfig.ConfigMaps {
		mp, wSubpath := model.backstageDeployment.mountPath(specCm.MountPath, specCm.Key, spec.Application.AppConfig.MountPath)
		updatePodWithAppConfig(model.backstageDeployment, model.backstageDeployment.container(), specCm.Name,
			mp, specCm.Key, wSubpath, model.ExternalConfig.AppConfigKeys[specCm.Name])
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
func (b *AppConfig) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	if b.ConfigMap != nil {
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	updatePodWithAppConfig(m.backstageDeployment, m.backstageDeployment.container(), b.ConfigMap.Name, m.backstageDeployment.defaultMountPath(), "", true, maps.Keys(b.ConfigMap.Data))
	return nil
}

func (b *AppConfig) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.ConfigMap.SetName(AppConfigDefaultName(backstage.Name))
	setMetaInfo(b.ConfigMap, backstage, scheme)
}

// updatePodWithAppConfig contrubutes to Volumes, container.VolumeMounts and contaiter.Args
func updatePodWithAppConfig(bsd *BackstageDeployment, container *corev1.Container, cmName, mountPath, key string, withSubPath bool, cmData []string) {
	bsd.mountFilesFrom([]string{container.Name}, ConfigMapObjectKind,
		cmName, mountPath, key, withSubPath, cmData)

	for _, file := range cmData {
		if key == "" || key == file {
			container.Args = append(container.Args, []string{"--config", filepath.Join(mountPath, file)}...)
		}
	}
}
