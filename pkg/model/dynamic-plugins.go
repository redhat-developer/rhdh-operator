package model

import (
	"fmt"
	"os"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

//it relies on implementation where dynamic-plugin initContainer
//uses specified ConfigMap for producing app-config with dynamic-plugins
//For this implementation:
//- backstage contaier and dynamic-plugin initContainer must share a volume
//  where initContainer writes and backstage container reads produced app-config
//- app-config path should be set as a --config parameter of backstage container
//in the deployment manifest

//it creates a volume with dynamic-plugins ConfigMap (there should be a key named "dynamic-plugins.yaml")
//and mount it to the dynamic-plugin initContainer's WorkingDir (what if not specified?)

const dynamicPluginInitContainerName = "install-dynamic-plugins"
const DynamicPluginsFile = "dynamic-plugins.yaml"

type DynamicPluginsFactory struct{}

func (f DynamicPluginsFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("54c88faa43ba8c5ded")
	return &DynamicPlugins{}
}

type DynamicPlugins struct {
	ConfigMap *corev1.ConfigMap
}

func init() {
	__sealights__.TraceFunc("b092b1470f5c435ab1")
	registerConfig("dynamic-plugins.yaml", DynamicPluginsFactory{}, false)
}

func DynamicPluginsDefaultName(backstageName string) string {
	__sealights__.TraceFunc("769d34ae79574715b7")
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-dynamic-plugins")
}

func addDynamicPluginsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {
	__sealights__.TraceFunc("69c9548fb3f3a33186")

	if spec.Application == nil || spec.Application.DynamicPluginsConfigMapName == "" {
		return nil
	}

	_, ic := DynamicPluginsInitContainer(model.backstageDeployment.deployment.Spec.Template.Spec.InitContainers)
	if ic == nil {
		return fmt.Errorf("validation failed, dynamic plugin name configured but no InitContainer %s defined", dynamicPluginInitContainerName)
	}

	dp := model.ExternalConfig.DynamicPlugins
	if dp.Data == nil || len(dp.Data) != 1 || dp.Data[DynamicPluginsFile] == "" {
		return fmt.Errorf("dynamic plugin configMap expects exactly one Data key named '%s' ", DynamicPluginsFile)
	}

	model.backstageDeployment.mountFilesFrom([]string{dynamicPluginInitContainerName}, ConfigMapObjectKind,
		dp.Name, ic.WorkingDir, DynamicPluginsFile, true, maps.Keys(dp.Data))

	return nil

}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) Object() runtime.Object {
	__sealights__.TraceFunc("7cd7466d7b5bc8cbf2")
	return p.ConfigMap
}

func (p *DynamicPlugins) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("4fb709f65a2e99d504")
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}

}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) EmptyObject() client.Object {
	__sealights__.TraceFunc("fdfc0367f0dfc73a9d")
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("fa518985307dd2d259")

	if p.ConfigMap == nil || (backstage.Spec.Application != nil && backstage.Spec.Application.DynamicPluginsConfigMapName != "") {
		return false, nil
	}
	model.setRuntimeObject(p)
	return true, nil
}

// implementation of RuntimeObject interface
// ConfigMap name must be the same as (deployment.yaml).spec.template.spec.volumes.name.dynamic-plugins-conf.ConfigMap.name
func (p *DynamicPlugins) updateAndValidate(model *BackstageModel, _ bsv1.Backstage) error {
	__sealights__.TraceFunc("85995d4ae0bf3e5b20")

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

	model.backstageDeployment.mountFilesFrom([]string{dynamicPluginInitContainerName}, ConfigMapObjectKind,
		p.ConfigMap.Name, initContainer.WorkingDir, DynamicPluginsFile, true, maps.Keys(p.ConfigMap.Data))

	return nil
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("70f235114d4b030098")
	p.ConfigMap.SetName(DynamicPluginsDefaultName(backstage.Name))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}

// returns initContainer supposed to initialize DynamicPlugins
// TODO consider to use a label to identify instead
func DynamicPluginsInitContainer(initContainers []corev1.Container) (int, *corev1.Container) {
	__sealights__.TraceFunc("c51c63a727e4caa6c5")
	for i, ic := range initContainers {
		if ic.Name == dynamicPluginInitContainerName {
			return i, &ic
		}
	}
	return -1, nil
}
