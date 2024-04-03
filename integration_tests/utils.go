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

package integration_tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func generateConfigMap(ctx context.Context, k8sClient client.Client, name, namespace string, data map[string]string) string {
	Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	})).To(Not(HaveOccurred()))

	return name
}

func generateSecret(ctx context.Context, k8sClient client.Client, name, namespace string, keys []string) string {
	data := map[string]string{}
	for _, v := range keys {
		data[v] = fmt.Sprintf("value-%s", v)
	}
	Expect(k8sClient.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
