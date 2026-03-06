package model

import (
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openshift "github.com/openshift/api/route/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"k8s.io/utils/ptr"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
)

// testBackstageObject it is a helper object to simplify testing model component allowing to customize and isolate testing configuration
// usual sequence of creating testBackstageObject contains such a steps:
// createBackstageTest(api.Backstage).
// withDefaultConfig(useDef bool)
// addToDefaultConfig(key, fileName)
type testBackstageObject struct {
	backstage      api.Backstage
	externalConfig ExternalConfig
	scheme         *runtime.Scheme
}

// initialises testBackstageObject object
func createBackstageTest(bs api.Backstage) *testBackstageObject {
	ec := ExternalConfig{
		RawConfig: map[string]string{},
	}
	b := &testBackstageObject{backstage: bs, externalConfig: ec, scheme: runtime.NewScheme()}
	utilruntime.Must(api.AddToScheme(b.scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(b.scheme))
	utilruntime.Must(openshift.Install(b.scheme))
	return b
}

// enables LocalDB
func (b *testBackstageObject) withLocalDb(enabled bool) *testBackstageObject {
	if b.backstage.Spec.Database == nil {
		b.backstage.Spec.Database = &api.Database{}
	}
	b.backstage.Spec.Database.EnableLocalDb = ptr.To(enabled)
	return b
}

// tells if object should use default Backstage Deployment/Service configuration from ./testdata/default-config or not
func (b *testBackstageObject) withDefaultConfig(useDef bool) *testBackstageObject {
	if useDef {
		// here we have default-config folder
		_ = os.Setenv("LOCALBIN", "./testdata")
	} else {
		_ = os.Setenv("LOCALBIN", ".")
	}
	return b
}

// sets custom config path by pointing LOCALBIN to the specified path
func (b *testBackstageObject) withConfigPath(path string) *testBackstageObject {
	_ = os.Setenv("LOCALBIN", path)
	return b
}

// adds particular part of configuration pointing to configuration key
// where key is configuration key (such as "deployment.yaml" and fileName is a name of additional conf file in ./testdata
func (b *testBackstageObject) addToDefaultConfig(key string, fileName string) *testBackstageObject {

	yaml, err := readTestYamlFile(fileName)
	if err != nil {
		panic(err)
	}

	b.externalConfig.RawConfig[key] = string(yaml)

	return b
}

// reads file from ./testdata
func readTestYamlFile(name string) ([]byte, error) {

	b, err := os.ReadFile(filepath.Join("testdata", name)) // #nosec G304, path is constructed internally
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}
	return b, nil
}

func checkIfContainVolumes(volumes []corev1.Volume, name string) bool {
	for _, volume := range volumes {
		if volume.Name == name {
			return true
		}
	}
	return false
}

// Helper functions for test assertions

func findConfigMapByName(items []client.Object, name string) client.Object {
	for _, item := range items {
		cm := item.(*corev1.ConfigMap)
		if cm.Name == name {
			return item
		}
	}
	return nil
}

func findConfigMapBySource(items []client.Object, source string) client.Object {
	for _, item := range items {
		cm := item.(*corev1.ConfigMap)
		if cm.Annotations != nil && cm.Annotations[SourceAnnotation] == source {
			return item
		}
	}
	return nil
}

func findPluginByPackage(plugins []DynaPlugin, packageName string) *DynaPlugin {
	for _, plugin := range plugins {
		if plugin.Package == packageName {
			return &plugin
		}
	}
	return nil
}

func findEnvVar(envVars []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range envVars {
		if envVars[i].Name == name {
			return &envVars[i]
		}
	}
	return nil
}
