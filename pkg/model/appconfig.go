package model

import (
	"path/filepath"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type AppConfigFactory struct{}

// factory method to create App Config object
func (f AppConfigFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("fc3ec67e20f713c300")
	return &AppConfig{}
}

// structure containing ConfigMap where keys are Backstage ConfigApp file names and vaues are contents of the files
// Mount path is a patch to the follder to place the files to
type AppConfig struct {
	ConfigMap *corev1.ConfigMap
}

func init() {
	__sealights__.TraceFunc("675c9fef974656db23")
	registerConfig("app-config.yaml", AppConfigFactory{}, false)
}

func AppConfigDefaultName(backstageName string) string {
	__sealights__.TraceFunc("79a8a0e0f9d3e52ee6")
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-appconfig")
}

func addAppConfigsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {
	__sealights__.TraceFunc("2ea71fb97b633fad3c")

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
	__sealights__.TraceFunc("ceb6cda3c7298405bc")
	return b.ConfigMap
}

// implementation of RuntimeObject interface
func (b *AppConfig) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("b7640955baf7d95e40")
	b.ConfigMap = nil
	if obj != nil {
		b.ConfigMap = obj.(*corev1.ConfigMap)
	}
}

// implementation of RuntimeObject interface
func (b *AppConfig) EmptyObject() client.Object {
	__sealights__.TraceFunc("de33c717e5345d45eb")
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (b *AppConfig) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("5a8167b1735b3991bf")
	if b.ConfigMap != nil {
		model.appConfig = b
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) updateAndValidate(m *BackstageModel, backstage bsv1.Backstage) error {
	__sealights__.TraceFunc("04c0cd10423ad4b4ad")
	updatePodWithAppConfig(m.backstageDeployment, m.backstageDeployment.container(), b.ConfigMap.Name, m.backstageDeployment.defaultMountPath(), "", true, maps.Keys(b.ConfigMap.Data))
	return nil
}

func (b *AppConfig) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("8225359746cb3e2eea")
	b.ConfigMap.SetName(AppConfigDefaultName(backstage.Name))
	setMetaInfo(b.ConfigMap, backstage, scheme)
}

// updatePodWithAppConfig contributes to Volumes, container.VolumeMounts and container.Args
func updatePodWithAppConfig(bsd *BackstageDeployment, container *corev1.Container, cmName, mountPath, key string, withSubPath bool, cmData []string) {
	__sealights__.TraceFunc("714ab3178166d36f70")
	bsd.mountFilesFrom([]string{container.Name}, ConfigMapObjectKind,
		cmName, mountPath, key, withSubPath, cmData)

	for _, file := range cmData {
		if key == "" || key == file {
			container.Args = append(container.Args, []string{"--config", filepath.Join(mountPath, file)}...)
		}
	}
}
