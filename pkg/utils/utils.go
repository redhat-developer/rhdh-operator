//
// Copyright (c) 2023 Red Hat, Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/util/yaml"
)

const maxK8sResourceNameLength = 63

func SetKubeLabels(labels map[string]string, backstageName string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	labels["app.kubernetes.io/name"] = "backstage"
	labels["app.kubernetes.io/instance"] = backstageName

	return labels
}

// GenerateLabel generates backstage-{Id} for labels or selectors
func GenerateLabel(labels *map[string]string, name string, value string) {
	if *labels == nil {
		*labels = map[string]string{}
	}
	(*labels)[name] = value
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
func ReadYamls(manifest []byte, templ runtime.Object, scheme runtime.Scheme) ([]client.Object, error) {

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
		objects = append(objects, obj)
	}

	return objects, nil
}

func ReadYamlFiles(path string, templ runtime.Object, scheme runtime.Scheme) ([]client.Object, error) {
	fpath := filepath.Clean(path)
	if _, err := os.Stat(fpath); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(fpath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}
	return ReadYamls(b, templ, scheme)
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

// Automatically detects if the cluster the operator running on is OpenShift
func IsOpenshift() (bool, error) {
	restConfig := ctrl.GetConfigOrDie()
	dcl, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return false, err
	}

	apiList, err := dcl.ServerGroups()
	if err != nil {
		return false, err
	}

	apiGroups := apiList.Groups
	for i := 0; i < len(apiGroups); i++ {
		if apiGroups[i].Name == "route.openshift.io" {
			return true, nil
		}
	}

	return false, nil
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
