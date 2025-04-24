package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/kyaml/yaml/merge2"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/util/yaml"
)

const maxK8sResourceNameLength = 63

const (
	BackstageAppLabel      = "app.kubernetes.io/name"
	BackstageAppName       = "backstage"
	BackstageInstanceLabel = "app.kubernetes.io/instance"
)

func SetKubeLabels(labels map[string]string, backstageName string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	labels[BackstageAppLabel] = BackstageAppName
	labels[BackstageInstanceLabel] = backstageName

	return labels
}

// GenerateLabel generates backstage-{Id} for labels or selectors
func GenerateLabel(labels *map[string]string, name string, value string) {
	if *labels == nil {
		*labels = map[string]string{}
	}
	(*labels)[name] = value
}

func AddAnnotation(object client.Object, name string, value string) {
	if object.GetAnnotations() == nil {
		object.SetAnnotations(map[string]string{})
	}
	object.GetAnnotations()[name] = value
}

// GenerateRuntimeObjectName generates name using BackstageCR name and objectType which is ConfigObject Key without '.yaml' (like 'deployment')
func GenerateRuntimeObjectName(backstageCRName string, objectType string) string {
	return fmt.Sprintf("%s-%s", objectType, backstageCRName)
}

// GenerateVolumeNameFromCmOrSecret generates volume name for mounting ConfigMap or Secret.
//
// It does so by converting the input name to an RFC 1123-compliant value, which is required by Kubernetes,
// even if the input CM/Secret name can be a valid DNS subdomain.
//
// See https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
func GenerateVolumeNameFromCmOrSecret(cmOrSecretName string) string {
	return ToRFC1123Label(cmOrSecretName)
}

func BackstageAppLabelValue(backstageName string) string {
	return fmt.Sprintf("backstage-%s", backstageName)
}

func BackstageDbAppLabelValue(backstageName string) string {
	return fmt.Sprintf("backstage-psql-%s", backstageName)
}

// ReadYamls reads and unmarshalls yaml with potentially multiple objects of the same type
// manifest - yaml content
// platformPatch - yaml content with platform specific patch, to be merged with manifest if exists
// templ - template object to create new objects
// scheme - runtime.Scheme
func ReadYamls(manifest []byte, platformPatch []byte, templ runtime.Object, scheme runtime.Scheme) ([]client.Object, error) {

	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 1000)

	objects := []client.Object{}
	for {
		// make a new object from template
		obj := reflect.New(reflect.ValueOf(templ).Elem().Type()).Interface().(client.Object)

		err := dec.Decode(obj)

		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}

		if err := checkObjectKind(obj, &scheme); err != nil {
			return nil, err
		}

		// merge platform patch if exists
		if platformPatch != nil {

			merged, err := merge2.MergeStrings(string(platformPatch), string(manifest), false, kyaml.MergeOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to merge platform patch: %w", err)
			}

			err = yaml.Unmarshal([]byte(merged), obj)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal merged YAML: %w", err)
			}
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

func ReadYamlFiles(path string, templ runtime.Object, scheme runtime.Scheme, platformExt string) ([]client.Object, error) {
	fpath := filepath.Clean(path)
	if _, err := os.Stat(fpath); err != nil {
		return nil, err
	}
	conf, err := os.ReadFile(fpath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}

	// Read platform patch if exists
	pp, err := readPlatformPatch(fpath, platformExt)
	if err != nil {
		return nil, fmt.Errorf("failed to read platform patch: %w", err)
	}
	return ReadYamls(conf, pp, templ, scheme)
}

func readPlatformPatch(path string, platformExt string) ([]byte, error) {
	fpath := filepath.Clean(path + "." + platformExt)
	b, err := os.ReadFile(fpath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read platform patch: %w", err)
	}
	return b, nil
}

func checkObjectKind(object client.Object, scheme *runtime.Scheme) error {
	gvks, _, err := scheme.ObjectKinds(object)
	if err != nil {
		return fmt.Errorf("failed to obtain object Kind: %w", err)
	}

	for _, gvk := range gvks {
		if object.GetObjectKind().GroupVersionKind() == gvk {
			return nil
		}
	}

	return fmt.Errorf("GroupVersionKind not match, found: %v, expected: %v", object.GetObjectKind().GroupVersionKind(), gvks)

}

func GetObjectKind(object client.Object, scheme *runtime.Scheme) *schema.GroupVersionKind {
	gvks, _, err := scheme.ObjectKinds(object)
	if err != nil {
		return nil
	}
	return &gvks[0]
}

func DefFile(key string) string {
	return filepath.Join(os.Getenv("LOCALBIN"), "default-config", key)
}

func GeneratePassword(length int) (string, error) {
	buff := make([]byte, length)
	if _, err := rand.Read(buff); err != nil {
		return "", err
	}
	// Encode the password to prevent special characters
	return base64.StdEncoding.EncodeToString(buff), nil
}

// ToRFC1123Label converts the given string into a valid Kubernetes label name (RFC 1123-compliant).
// See https://kubernetes.io/docs/concepts/overview/working-with-objects/names/ for more details about the requirements.
// It will replace any invalid characters with a dash and drop any leading or trailing dashes.
func ToRFC1123Label(str string) string {
	const dash = "-"

	name := strings.ToLower(str)

	// Replace all invalid characters with a dash
	re := regexp.MustCompile(`[^a-z0-9-]`)
	name = re.ReplaceAllString(name, dash)

	// Replace consecutive dashes with a single dash
	reConsecutiveDashes := regexp.MustCompile(`-+`)
	name = reConsecutiveDashes.ReplaceAllString(name, dash)

	// Truncate to maxK8sResourceNameLength characters if necessary
	if len(name) > maxK8sResourceNameLength {
		name = name[:maxK8sResourceNameLength]
	}

	// Continue trimming leading and trailing dashes if necessary
	for strings.HasPrefix(name, dash) || strings.HasSuffix(name, dash) {
		name = strings.Trim(name, dash)
	}

	return name
}

func BoolEnvVar(envvar string, def bool) bool {
	if envValue, ok := os.LookupEnv(envvar); ok {
		if ret, err := strconv.ParseBool(envValue); err == nil {
			return ret
		}
	}
	return def
}

func FilterContainers(allContainers []string, filter string) []string {
	if filter == "*" {
		return allContainers
	} else if filter == "" {
		return nil
	}

	filtered := []string{}
	for _, c := range allContainers {
		for _, cname := range strings.Split(filter, ",") {
			if c == strings.TrimSpace(cname) {
				filtered = append(filtered, c)
			}
		}
	}
	return filtered
}
