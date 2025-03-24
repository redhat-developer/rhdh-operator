package model

import (
	"fmt"
	"path/filepath"

	"golang.org/x/exp/maps"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

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
func (b *AppConfig) updateAndValidate(m *BackstageModel, backstage bsv1.Backstage) error {
	b.updateDefaultBaseUrls(m, backstage)
	updatePodWithAppConfig(m.backstageDeployment, m.backstageDeployment.container(), b.ConfigMap.Name, m.backstageDeployment.defaultMountPath(), "", true, maps.Keys(b.ConfigMap.Data))
	return nil
}

// updateDefaultBaseUrls tries to set the baseUrl in the default app-config.
// Note that this is purposely done on a best effort basis. So it is not considered an issue if the cluster ingress domain
// could not be determined, since the user can always set it explicitly in their custom app-config.
func (b *AppConfig) updateDefaultBaseUrls(m *BackstageModel, backstage bsv1.Backstage) {
	baseUrl := buildOpenShiftBaseUrl(m, backstage)
	if baseUrl == "" {
		return
	}

	for k, v := range b.ConfigMap.Data {
		updated, err := getAppConfigContentWithBaseUrlsUpdated(v, baseUrl)
		if err != nil {
			klog.V(1).Infof("[warn] could not update base url in default app-config %q for backstage %s: %v",
				k, backstage.Name, err)
			continue
		}
		b.ConfigMap.Data[k] = updated
	}
}

func getAppConfigContentWithBaseUrlsUpdated(content string, baseUrl string) (string, error) {
	var appConfigData map[string]any
	err := yaml.Unmarshal([]byte(content), &appConfigData)
	if err != nil {
		return "", fmt.Errorf("failed to decode app-config YAML: %w", err)
	}
	app, ok := appConfigData["app"].(map[string]any)
	if !ok {
		app = make(map[string]any)
		appConfigData["app"] = app
	}
	app["baseUrl"] = baseUrl

	backend, ok := appConfigData["backend"].(map[string]any)
	if !ok {
		backend = make(map[string]any)
		appConfigData["backend"] = backend
	}
	backend["baseUrl"] = baseUrl

	backendCors, ok := backend["cors"].(map[string]any)
	if !ok {
		backendCors = make(map[string]any)
		backend["cors"] = backendCors
	}
	backendCors["origin"] = baseUrl

	updated, err := yaml.Marshal(&appConfigData)
	if err != nil {
		return "", fmt.Errorf("failed to serialize updated app-config YAML: %w", err)
	}
	return string(updated), nil
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
