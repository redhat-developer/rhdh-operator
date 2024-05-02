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
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	appsv1 "k8s.io/api/apps/v1"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create backstage with external configuration", func() {

	var (
		ctx context.Context
		ns  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		ns = createNamespace(ctx)
	})

	AfterEach(func() {
		deleteNamespace(ctx, ns)
	})

	It("refresh config", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		appConfig1 := "app-config1"
		secretEnv1 := "secret-env1"

		backstageName := generateRandName()

		generateConfigMap(ctx, k8sClient, appConfig1, ns, map[string]string{"key11": "app:", "key12": "app:"}, nil, nil)
		//map[string]string{model.ExtConfigSyncLabel: "true"}, map[string]string{model.BackstageNameAnnotation: backstageName})
		generateSecret(ctx, k8sClient, secretEnv1, ns, map[string]string{"sec11": "val11"}, nil, nil)
		//map[string]string{model.ExtConfigSyncLabel: "true"}, map[string]string{model.BackstageNameAnnotation: backstageName})

		bs := bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				AppConfig: &bsv1alpha1.AppConfig{
					MountPath: "/my/mount/path",
					ConfigMaps: []bsv1alpha1.ObjectKeyRef{
						{Name: appConfig1},
					},
				},
				ExtraEnvs: &bsv1alpha1.ExtraEnvs{
					Secrets: []bsv1alpha1.ObjectKeyRef{
						{Name: secretEnv1, Key: "sec11"},
					},
				},
			},
		}

		createAndReconcileBackstage(ctx, ns, bs, backstageName)

		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			podList := &corev1.PodList{}
			err = k8sClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{model.BackstageAppLabel: utils.BackstageAppLabelValue(backstageName)})
			g.Expect(err).ShouldNot(HaveOccurred())

			g.Expect(len(podList.Items)).To(Equal(1))
			podName := podList.Items[0].Name
			out, _, err := executeRemoteCommand(ctx, ns, podName, "backstage-backend", "cat /my/mount/path/key11")
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(out).To(Equal("app:"))

			out, _, err = executeRemoteCommand(ctx, ns, podName, "backstage-backend", "echo $sec11")
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect("val11\r\n").To(Equal(out))

		}, 10*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

		cm := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: appConfig1}, cm)
		Expect(err).ShouldNot(HaveOccurred())

		newData := "app:\n  backend:"
		cm.Data = map[string]string{"key11": newData}
		err = k8sClient.Update(ctx, cm)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: appConfig1}, cm)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(cm.Data["key11"]).To(Equal(newData))

			// Pod replaced so have to re-ask
			podList := &corev1.PodList{}
			err = k8sClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{model.BackstageAppLabel: utils.BackstageAppLabelValue(backstageName)})
			g.Expect(err).ShouldNot(HaveOccurred())

			podName := podList.Items[0].Name
			out, _, err := executeRemoteCommand(ctx, ns, podName, "backstage-backend", "cat /my/mount/path/key11")
			g.Expect(err).ShouldNot(HaveOccurred())
			// TODO nicer method to compare file content with added '\r'
			g.Expect(strings.ReplaceAll(out, "\r", "")).To(Equal(newData))

			_, _, err = executeRemoteCommand(ctx, ns, podName, "backstage-backend", "cat /my/mount/path/key12")
			g.Expect(err).Should(HaveOccurred())

		}, 10*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

		sec := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretEnv1}, sec)
		Expect(err).ShouldNot(HaveOccurred())
		newEnv := "val22"
		sec.StringData = map[string]string{"sec11": newEnv}
		err = k8sClient.Update(ctx, sec)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {

			// Pod replaced so have to re-ask
			podList := &corev1.PodList{}
			err = k8sClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{model.BackstageAppLabel: utils.BackstageAppLabelValue(backstageName)})
			g.Expect(err).ShouldNot(HaveOccurred())

			podName := podList.Items[0].Name

			out, _, err := executeRemoteCommand(ctx, ns, podName, "backstage-backend", "echo $sec11")
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(fmt.Sprintf("%s%s", newEnv, "\r\n")).To(Equal(out))

		}, 10*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

	})

})
