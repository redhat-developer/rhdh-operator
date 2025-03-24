package integration_tests

import (
	"errors"
	"fmt"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/gcustom"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
)

// Matcher for Container VolumeMounts
func BeMountedToContainer(c corev1.Container) types.GomegaMatcher {
	return &BeMountedToContainerMatcher{container: c}
}

type BeMountedToContainerMatcher struct {
	container corev1.Container
}

func (matcher *BeMountedToContainerMatcher) Match(actual interface{}) (bool, error) {
	mountPath, ok := actual.(string)
	if !ok {
		return false, fmt.Errorf("BeMountedToContainer must be passed string. Got\n%s", format.Object(actual, 1))
	}

	for _, vm := range matcher.container.VolumeMounts {
		if vm.MountPath == mountPath {
			return true, nil
		}
	}
	return false, nil
}
func (matcher *BeMountedToContainerMatcher) FailureMessage(actual interface{}) string {
	mountPath, _ := actual.(string)
	return fmt.Sprintf("Expected container to contain VolumeMount with mountPath %s", mountPath)
}
func (matcher *BeMountedToContainerMatcher) NegatedFailureMessage(actual interface{}) string {
	mountPath, _ := actual.(string)
	return fmt.Sprintf("Expected container not to contain VolumeMount with mountPath %s", mountPath)
}

// Matcher for PodSpec Volumes
func BeAddedAsVolumeToPodSpec(ps corev1.PodSpec) types.GomegaMatcher {
	return &BeAddedAsVolumeToPodSpecMatcher{podSpec: ps}
}

type BeAddedAsVolumeToPodSpecMatcher struct {
	podSpec corev1.PodSpec
}

func (matcher *BeAddedAsVolumeToPodSpecMatcher) Match(actual interface{}) (bool, error) {
	volumeName, ok := actual.(string)
	if !ok {
		return false, fmt.Errorf("BeMountedToContainer must be passed string. Got\n%s", format.Object(actual, 1))
	}

	for _, v := range matcher.podSpec.Volumes {
		if v.Name == volumeName {
			return true, nil
		}
	}
	return false, nil
}
func (matcher *BeAddedAsVolumeToPodSpecMatcher) FailureMessage(actual interface{}) string {
	volumeName, _ := actual.(string)
	return fmt.Sprintf("Expected PodSpec to contain Volume %s", volumeName)
}
func (matcher *BeAddedAsVolumeToPodSpecMatcher) NegatedFailureMessage(actual interface{}) string {
	volumeName, _ := actual.(string)
	return fmt.Sprintf("Expected PodSpec not to contain Volume %s", volumeName)
}

// Matcher for container Args
func BeAddedAsArgToContainer(c corev1.Container) types.GomegaMatcher {
	return &BeMountedToContainerMatcher{container: c}
}

type BeAddedAsArgToContainerMatcher struct {
	container corev1.Container
}

func (matcher *BeAddedAsArgToContainerMatcher) Match(actual interface{}) (bool, error) {
	arg, ok := actual.(string)
	if !ok {
		return false, fmt.Errorf("BeAddedAsArgToContainer must be passed string. Got\n%s", format.Object(actual, 1))
	}

	for _, a := range matcher.container.Args {
		if arg == a {
			return true, nil
		}
	}
	return false, nil
}
func (matcher *BeAddedAsArgToContainerMatcher) FailureMessage(actual interface{}) string {
	arg, _ := actual.(string)
	return fmt.Sprintf("Expected container to contain Arg %s", arg)
}
func (matcher *BeAddedAsArgToContainerMatcher) NegatedFailureMessage(actual interface{}) string {
	arg, _ := actual.(string)
	return fmt.Sprintf("Expected container not to contain Arg %s", arg)
}

// Matcher for Container EnvFrom
func BeEnvFromForContainer(c corev1.Container) types.GomegaMatcher {
	return &BeEnvFromForContainerMatcher{container: c}
}

type BeEnvFromForContainerMatcher struct {
	container corev1.Container
}

func (matcher *BeEnvFromForContainerMatcher) Match(actual interface{}) (bool, error) {
	objectName, ok := actual.(string)
	if !ok {
		return false, fmt.Errorf("BeEnvFromForContainer must be passed string. Got\n%s", format.Object(actual, 1))
	}

	for _, ef := range matcher.container.EnvFrom {
		if ef.SecretRef != nil && ef.SecretRef.Name == objectName {
			return true, nil
		}
		if ef.ConfigMapRef != nil && ef.ConfigMapRef.Name == objectName {
			return true, nil
		}
	}
	return false, nil
}

func (matcher *BeEnvFromForContainerMatcher) FailureMessage(actual interface{}) string {
	objectName, _ := actual.(string)
	return fmt.Sprintf("Expected container to contain EnvFrom %s", objectName)
}

func (matcher *BeEnvFromForContainerMatcher) NegatedFailureMessage(actual interface{}) string {
	objectName, _ := actual.(string)
	return fmt.Sprintf("Expected container not to contain EnvFrom %s", objectName)
}

// Matcher for Container Env Var
func BeEnvVarForContainer(c corev1.Container) types.GomegaMatcher {
	return &BeEnvVarForContainerMatcher{container: c}
}

type BeEnvVarForContainerMatcher struct {
	container corev1.Container
}

func (matcher *BeEnvVarForContainerMatcher) Match(actual interface{}) (bool, error) {
	objectName, ok := actual.(string)
	if !ok {
		return false, fmt.Errorf("BeEnvVarForContainer must be passed string. Got\n%s", format.Object(actual, 1))
	}

	for _, ev := range matcher.container.Env {
		if ev.Name == objectName {
			return true, nil
		}
	}
	return false, nil
}

func (matcher *BeEnvVarForContainerMatcher) FailureMessage(actual interface{}) string {
	objectName, _ := actual.(string)
	return fmt.Sprintf("Expected container to contain EnvVar %s", objectName)
}

func (matcher *BeEnvVarForContainerMatcher) NegatedFailureMessage(actual interface{}) string {
	objectName, _ := actual.(string)
	return fmt.Sprintf("Expected container not to contain EnvVar %s", objectName)
}

type AppConfigData struct {
	App struct {
		BaseUrl string `yaml:"baseUrl"`
	} `yaml:"app"`
	Backend struct {
		BaseUrl string `yaml:"baseUrl"`
		Cors    struct {
			Origin string `yaml:"origin"`
		} `yaml:"cors"`
	} `yaml:"backend"`
}

func HaveAppConfigBaseUrl(expectedPattern string) types.GomegaMatcher {
	return gcustom.MakeMatcher(func(actual corev1.ConfigMap) (bool, error) {
		y, ok := actual.Data["default.app-config.yaml"]
		if !ok {
			return false, errors.New("expected to find `default.app-config.yaml` in the default app-config ConfigMap")
		}
		var appConfig AppConfigData
		err := ReadYaml([]byte(y), &appConfig)
		if err != nil {
			return false, fmt.Errorf("could not parse `default.app-config.yaml` into an AppConfig struct: %w", err)
		}
		return HaveEach(MatchRegexp(expectedPattern)).Match(
			[]string{
				appConfig.App.BaseUrl,
				appConfig.Backend.BaseUrl,
				appConfig.Backend.Cors.Origin,
			})
	}).WithTemplate(
		"Expected the default app-config ConfigMap:\n{{.FormattedActual}}\n{{.To}} have the default baseUrls and CORS origin set to:\n{{format .Data 1}}",
		expectedPattern)
}
