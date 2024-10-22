package model

import (
	"fmt"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	appsv1 "k8s.io/api/apps/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

const dynamicPluginInitContainerName = "install-dynamic-plugins"
const DynamicPluginsFile = "dynamic-plugins.yaml"

type DynamicPluginsFactory struct{}

func (f DynamicPluginsFactory) newBackstageObject() RuntimeObject {
	return &DynamicPlugins{}
}

type DynamicPlugins struct {
	ConfigMap *corev1.ConfigMap
}

func init() {
	registerConfig("dynamic-plugins.yaml", DynamicPluginsFactory{}, false)
}

func DynamicPluginsDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-dynamic-plugins")
}

func addDynamicPlugins(spec bsv1.BackstageSpec, deployment *appsv1.Deployment, model *BackstageModel) error {

	if spec.Application == nil || spec.Application.DynamicPluginsConfigMapName == "" {
		return nil
	}

	if _, ic := DynamicPluginsInitContainer(deployment.Spec.Template.Spec.InitContainers); ic == nil {
		return fmt.Errorf("validation failed, dynamic plugin name configured but no InitContainer %s defined", dynamicPluginInitContainerName)
	}

	dp := DynamicPlugins{ConfigMap: &model.ExternalConfig.DynamicPlugins}

	if dp.ConfigMap.Data == nil || len(dp.ConfigMap.Data) != 1 || dp.ConfigMap.Data[DynamicPluginsFile] == "" {
		return fmt.Errorf("dynamic plugin configMap expects exactly one key named '%s' ", DynamicPluginsFile)
	}

	dp.updatePod(deployment)
	return nil

}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) Object() runtime.Object {
	return p.ConfigMap
}

func (p *DynamicPlugins) setObject(obj runtime.Object) {
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}

}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) EmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {

	if p.ConfigMap == nil || (backstage.Spec.Application != nil && backstage.Spec.Application.DynamicPluginsConfigMapName != "") {
		return false, nil
	}
	model.setRuntimeObject(p)
	return true, nil
}

// implementation of BackstagePodContributor interface
func (p *DynamicPlugins) updatePod(deployment *appsv1.Deployment) {

	//it relies on implementation where dynamic-plugin initContainer
	//uses specified ConfigMap for producing app-config with dynamic-plugins
	//For this implementation:
	//- backstage contaier and dynamic-plugin initContainer must share a volume
	//  where initContainer writes and backstage container reads produced app-config
	//- app-config path should be set as a --config parameter of backstage container
	//in the deployment manifest

	//it creates a volume with dynamic-plugins ConfigMap (there should be a key named "dynamic-plugins.yaml")
	//and mount it to the dynamic-plugin initContainer's WorkingDir (what if not specified?)

	_, initContainer := DynamicPluginsInitContainer(deployment.Spec.Template.Spec.InitContainers)
	if initContainer == nil {
		// it will fail on validate
		return
	}

	utils.MountFilesFrom(&deployment.Spec.Template.Spec, &deployment.Spec.Template.Spec.InitContainers[0], utils.ConfigMapObjectKind,
		p.ConfigMap.Name, initContainer.WorkingDir, DynamicPluginsFile, p.ConfigMap.Data)

}

// implementation of RuntimeObject interface
// ConfigMap name must be the same as (deployment.yaml).spec.template.spec.volumes.name.dynamic-plugins-conf.ConfigMap.name
func (p *DynamicPlugins) validate(model *BackstageModel, _ bsv1.Backstage) error {

	_, initContainer := DynamicPluginsInitContainer(model.backstageDeployment.deployment.Spec.Template.Spec.InitContainers)
	if initContainer == nil {
		return fmt.Errorf("failed to find initContainer named %s", dynamicPluginInitContainerName)
	}
	// override image with env var
	// [GA] Do we need this feature?
	if os.Getenv(BackstageImageEnvVar) != "" {
		// TODO workaround for the (janus-idp, rhdh) case where we have
		// exactly the same image for initContainer and want it to be overriden
		// the same way as Backstage's one
		initContainer.Image = os.Getenv(BackstageImageEnvVar)
	}
	return nil
}

func (p *DynamicPlugins) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(DynamicPluginsDefaultName(backstage.Name))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}

// returns initContainer supposed to initialize DynamicPlugins
// TODO consider to use a label to identify instead
func DynamicPluginsInitContainer(initContainers []corev1.Container) (int, *corev1.Container) {
	for i, ic := range initContainers {
		if ic.Name == dynamicPluginInitContainerName {
			return i, &ic
		}
	}
	return -1, nil
}
