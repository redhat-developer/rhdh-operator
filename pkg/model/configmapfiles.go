package model

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha3"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapFilesFactory struct{}

func (f ConfigMapFilesFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapFiles{MountPath: DefaultMountDir}
}

type ConfigMapFiles struct {
	ConfigMap   *corev1.ConfigMap
	MountPath   string
	Key         string
	withSubPath *bool
}

func init() {
	registerConfig("configmap-files.yaml", ConfigMapFilesFactory{}, false)
}

func addConfigMapFiles(spec bsv1.BackstageSpec, deployment *appsv1.Deployment, model *BackstageModel) error {

	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.ConfigMaps == nil {
		return nil
	}
	mp := DefaultMountDir
	if spec.Application.ExtraFiles.MountPath != "" {
		mp = spec.Application.ExtraFiles.MountPath
	}

	for _, configMap := range spec.Application.ExtraFiles.ConfigMaps {
		if configMap.MountPath != "" {
			mp = configMap.MountPath
		} else if configMap.WithSubPath == ptr.To(false) {
			return fmt.Errorf("mounting without subPath to non-individual MountPath is forbidden, ConfigMap name: %s", configMap.Name)
		}
		cm := model.ExternalConfig.ExtraFileConfigMaps[configMap.Name]
		cmf := ConfigMapFiles{
			ConfigMap:   &cm,
			MountPath:   mp,
			Key:         configMap.Key,
			withSubPath: configMap.WithSubPath,
		}
		cmf.updatePod(deployment)
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
func (p *ConfigMapFiles) validate(_ *BackstageModel, _ bsv1.Backstage) error {
	return nil
}

func (p *ConfigMapFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}

// implementation of BackstagePodContributor interface
func (p *ConfigMapFiles) updatePod(deployment *appsv1.Deployment) {

	utils.MountFilesFrom(&deployment.Spec.Template.Spec, &deployment.Spec.Template.Spec.Containers[0], utils.ConfigMapObjectKind,
		p.ConfigMap.Name, p.MountPath, p.Key, p.withSubPath, p.ConfigMap.Data)

}
