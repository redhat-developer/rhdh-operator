package integration_tests

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/redhat-developer/rhdh-operator/pkg/model"
	appsv1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/util/yaml"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func generateConfigMap(ctx context.Context, k8sClient client.Client, name string, namespace string, data, labels map[string]string, annotations map[string]string) string {
	Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Data: data,
	})).To(Not(HaveOccurred()))

	return name
}

func generateSecret(ctx context.Context, k8sClient client.Client, name, namespace string, data, labels, annotations map[string]string) string {
	Expect(k8sClient.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		StringData: data,
	})).To(Not(HaveOccurred()))

	return name
}

func readTestYamlFile(name string) string {

	b, err := os.ReadFile(filepath.Join("testdata", name)) // #nosec G304, path is constructed internally
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

func executeRemoteCommand(ctx context.Context, podNamespace, podName, container, command string) (string, string, error) {
	kubeCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	restCfg, err := kubeCfg.ClientConfig()
	if err != nil {
		return "", "", err
	}
	coreClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", "", err
	}

	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	request := coreClient.CoreV1().RESTClient().
		Post().
		Namespace(podNamespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command:   []string{"/bin/sh", "-c", command},
			Container: container,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(restCfg, "POST", request.URL())
	if err != nil {
		return "", "", fmt.Errorf("%w failed creating executor  %s on %v/%v", err, command, podNamespace, podName)
	}
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: errBuf,
	})
	if err != nil {
		return "", "", fmt.Errorf("%w Failed executing command %s on %v/%v", err, command, podNamespace, podName)
	}

	return buf.String(), errBuf.String(), nil
}

func ReadYaml(manifest []byte, object interface{}) error {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 1000)
	if err := dec.Decode(object); err != nil {
		return fmt.Errorf("failed to decode YAML: %w", err)
	}
	return nil
}

func ReadYamlFile(path string, object interface{}) error {
	fpath := filepath.Clean(path)
	if _, err := os.Stat(fpath); err != nil {
		return err
	}
	b, err := os.ReadFile(fpath)
	if err != nil {
		return fmt.Errorf("failed to read YAML file: %w", err)
	}
	return ReadYaml(b, object)
}

func backstageContainerName(deploy *appsv1.Deployment) string {
	// backstage-backend
	cIndex := model.BackstageContainerIndex(deploy)
	return deploy.Spec.Template.Spec.Containers[cIndex].Name
}
