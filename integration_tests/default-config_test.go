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
	"time"

	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create default backstage", func() {

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

	It("creates runtime objects", func() {

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{}, "")

		Eventually(func(g Gomega) {
			By("creating a secret for accessing the Database")
			secret := &corev1.Secret{}
			secretName := fmt.Sprintf("backstage-psql-secret-%s", backstageName)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretName}, secret)
			g.Expect(err).ShouldNot(HaveOccurred(), controllerMessage())
			g.Expect(len(secret.Data)).To(Equal(5))
			g.Expect(secret.Data).To(HaveKeyWithValue("POSTGRES_USER", []uint8("postgres")))

			By("creating a StatefulSet for the Database")
			ss := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, ss)
			g.Expect(err).ShouldNot(HaveOccurred())

			By("injecting default DB Secret as an env var for Db container")
			g.Expect(model.DbSecretDefaultName(backstageName)).To(BeEnvFromForContainer(ss.Spec.Template.Spec.Containers[0]))
			g.Expect(ss.GetOwnerReferences()).To(HaveLen(1))

			By("creating a Service for the Database")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("backstage-psql-%s", backstageName), Namespace: ns}, &corev1.Service{})
			g.Expect(err).To(Not(HaveOccurred()))

			By("creating Deployment")
			deploy := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())
			Expect(deploy.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)))

			By("creating OwnerReference to all the runtime objects")
			or := deploy.GetOwnerReferences()
			g.Expect(or).To(HaveLen(1))
			g.Expect(or[0].Name).To(Equal(backstageName))

			By("creating default app-config")
			appConfig := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.AppConfigDefaultName(backstageName)}, appConfig)
			g.Expect(err).ShouldNot(HaveOccurred())

			By("mounting Volume defined in default app-config")
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(model.AppConfigDefaultName(backstageName))).
				To(BeAddedAsVolumeToPodSpec(deploy.Spec.Template.Spec))

		}, 5*time.Minute, time.Second).Should(Succeed())

		if *testEnv.UseExistingCluster {
			By("setting Backstage status (real cluster only)")
			Eventually(func(g Gomega) {

				bs := &bsv1.Backstage{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: backstageName}, bs)
				g.Expect(err).ShouldNot(HaveOccurred())

				deploy := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
				g.Expect(err).ShouldNot(HaveOccurred())

				// TODO better matcher for Conditions
				g.Expect(bs.Status.Conditions[0].Reason).To(Equal("Deployed"))

				for _, cond := range deploy.Status.Conditions {
					if cond.Type == "Available" {
						g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
					}
				}
			}, 5*time.Minute, time.Second).Should(Succeed())
		}
	})

	It("creates runtime object using raw configuration ", func() {

		bsConf := map[string]string{"deployment.yaml": readTestYamlFile("raw-deployment.yaml")}
		dbConf := map[string]string{"db-statefulset.yaml": readTestYamlFile("raw-statefulset.yaml")}

		bsRaw := generateConfigMap(ctx, k8sClient, "bsraw", ns, bsConf, nil, nil)
		dbRaw := generateConfigMap(ctx, k8sClient, "dbraw", ns, dbConf, nil, nil)

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			RawRuntimeConfig: &bsv1.RuntimeConfig{
				BackstageConfigName: bsRaw,
				LocalDbConfigName:   dbRaw,
			},
		}, "")

		Eventually(func(g Gomega) {
			By("creating Deployment")
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)))
			g.Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			g.Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal("busybox"))

			By("creating StatefulSet")
			ss := &appsv1.StatefulSet{}
			name := fmt.Sprintf("backstage-psql-%s", backstageName)
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, ss)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))
			g.Expect(ss.Spec.Template.Spec.Containers[0].Image).To(Equal("busybox"))
		}, time.Minute, time.Second).Should(Succeed())

	})

	It("creates runtime object using raw configuration then updates StatefulSet to replace some immutable fields", func() {
		if !*testEnv.UseExistingCluster {
			Skip("Real cluster required to assert actual deletion and replacement of resources")
		}

		rawStatefulSetYamlContent := readTestYamlFile("raw-statefulset.yaml")
		dbConf := map[string]string{"db-statefulset.yaml": rawStatefulSetYamlContent}

		dbRaw := generateConfigMap(ctx, k8sClient, "dbraw", ns, dbConf, nil, nil)

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			RawRuntimeConfig: &bsv1.RuntimeConfig{
				LocalDbConfigName: dbRaw,
			},
		}, "")

		Eventually(func(g Gomega) {
			By("creating Deployment")
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			By("creating StatefulSet")
			dbStatefulSet := &appsv1.StatefulSet{}
			name := fmt.Sprintf("backstage-psql-%s", backstageName)
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, dbStatefulSet)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(dbStatefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
			g.Expect(dbStatefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal("busybox"))
			g.Expect(dbStatefulSet.Spec.PodManagementPolicy).To(Equal(appsv1.ParallelPodManagement))
		}, time.Minute, time.Second).Should(Succeed())

		By("updating CR to default config")
		update := &bsv1.Backstage{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, update)
		Expect(err).To(Not(HaveOccurred()))
		update.Spec.RawRuntimeConfig = nil
		err = k8sClient.Update(ctx, update)
		Expect(err).To(Not(HaveOccurred()))

		_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		// Patching StatefulSets is done by the reconciler in two passes: first deleting the StatefulSet, then recreating it in the next reconcilation.
		// to make next reconciliation happen (forcing ReconcileAny is not working on a real cluster)
		update.SetAnnotations(map[string]string{"name": "value"})
		err = k8sClient.Update(ctx, update)
		Expect(err).To(Not(HaveOccurred()))

		_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		Eventually(func(g Gomega) {
			By("replacing StatefulSet")
			dbStatefulSet := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, dbStatefulSet)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(dbStatefulSet.Spec.PodManagementPolicy).To(Equal(appsv1.OrderedReadyPodManagement))
		}, time.Minute, time.Second).Should(Succeed())
	})

})
