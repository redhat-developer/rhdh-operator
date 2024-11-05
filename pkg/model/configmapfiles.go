package model

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapFilesFactory struct{}

func (f ConfigMapFilesFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapFiles{withSubPath: true}
}

type ConfigMapFiles struct {
	ConfigMap   *corev1.ConfigMap
	MountPath   string
	Key         string
	withSubPath bool
}

func init() {
	registerConfig("configmap-files.yaml", ConfigMapFilesFactory{}, false)
}

func addConfigMapFiles(spec bsv1.BackstageSpec, model *BackstageModel) error {

	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.ConfigMaps == nil {
		return nil
	}

	for _, configMap := range spec.Application.ExtraFiles.ConfigMaps {

		mp, wSubpath := model.backstageDeployment.mountPath(configMap, spec.Application.ExtraFiles.MountPath)
		cm := model.ExternalConfig.ExtraFileConfigMaps[configMap.Name]
		cmf := ConfigMapFiles{
			ConfigMap:   &cm,
			MountPath:   mp,
			Key:         configMap.Key,
			withSubPath: wSubpath,
		}
		cmf.updatePod(model.backstageDeployment.deployment)
	}

	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) Object() runtime.Object {
	return p.ConfigMap
}

func (p *ConfigMapFiles) setObject(obj runtime.Object) {
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}

}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) EmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	p.MountPath = m.backstageDeployment.defaultMountPath()
	p.updatePod(m.backstageDeployment.deployment)

	return nil
}

func (p *ConfigMapFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}

func (p *ConfigMapFiles) updatePod(deployment *appsv1.Deployment) {

	utils.MountFilesFrom(&deployment.Spec.Template.Spec, &deployment.Spec.Template.Spec.Containers[0], utils.ConfigMapObjectKind,
		p.ConfigMap.Name, p.MountPath, p.Key, p.withSubPath, p.ConfigMap.Data)

}
