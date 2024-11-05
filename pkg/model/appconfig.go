package model

import (
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	appsv1 "k8s.io/api/apps/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type AppConfigFactory struct{}

// factory method to create App Config object
func (f AppConfigFactory) newBackstageObject() RuntimeObject {
	return &AppConfig{withSubPath: true}
}

// structure containing ConfigMap where keys are Backstage ConfigApp file names and vaues are contents of the files
// Mount path is a patch to the follder to place the files to
type AppConfig struct {
	ConfigMap   *corev1.ConfigMap
	MountPath   string
	Key         string
	withSubPath bool
}

func init() {
	registerConfig("app-config.yaml", AppConfigFactory{}, false)
}

func AppConfigDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-appconfig")
}

func addAppConfigs(spec bsv1.BackstageSpec, model *BackstageModel) {

	if spec.Application == nil || spec.Application.AppConfig == nil || spec.Application.AppConfig.ConfigMaps == nil {
		return
	}

	for _, configMap := range spec.Application.AppConfig.ConfigMaps {
		cm := model.ExternalConfig.AppConfigs[configMap.Name]
		mp, wSubpath := model.backstageDeployment.mountPath(configMap, spec.Application.AppConfig.MountPath)
		ac := AppConfig{
			ConfigMap:   &cm,
			MountPath:   mp,
			Key:         configMap.Key,
			withSubPath: wSubpath,
		}
		ac.updatePod(model.backstageDeployment.deployment)
	}
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
func (b *AppConfig) updateAndValidate(m *BackstageModel, backstage bsv1.Backstage) error {
	b.MountPath = m.backstageDeployment.defaultMountPath()
	b.updatePod(m.backstageDeployment.deployment)
	return nil
}

func (b *AppConfig) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.ConfigMap.SetName(AppConfigDefaultName(backstage.Name))
	setMetaInfo(b.ConfigMap, backstage, scheme)
}

// it contrubutes to Volumes, container.VolumeMounts and contaiter.Args
func (b *AppConfig) updatePod(deployment *appsv1.Deployment) {

	utils.MountFilesFrom(&deployment.Spec.Template.Spec, &deployment.Spec.Template.Spec.Containers[0], utils.ConfigMapObjectKind,
		b.ConfigMap.Name, b.MountPath, b.Key, b.withSubPath, b.ConfigMap.Data)

	fileDir := b.MountPath
	for file := range b.ConfigMap.Data {
		if b.Key == "" || b.Key == file {
			deployment.Spec.Template.Spec.Containers[0].Args =
				append(deployment.Spec.Template.Spec.Containers[0].Args, []string{"--config", filepath.Join(fileDir, file)}...)
		}
	}
}
