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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"

	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create backstage with CR configured", func() {

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

	It("creates Backstage with configuration ", func() {

		appConfig1 := generateConfigMap(ctx, k8sClient, "app-config1", ns, map[string]string{"key11": "app:", "key12": "app:"})
		appConfig2 := generateConfigMap(ctx, k8sClient, "app-config2", ns, map[string]string{"key21": "app:", "key22": "app:"})

		cmFile1 := generateConfigMap(ctx, k8sClient, "cm-file1", ns, map[string]string{"cm11": "11", "cm12": "12"})
		cmFile2 := generateConfigMap(ctx, k8sClient, "cm-file2", ns, map[string]string{"cm21": "21", "cm22": "22"})

		secretFile1 := generateSecret(ctx, k8sClient, "secret-file1", ns, []string{"sec11", "sec12"})
		secretFile2 := generateSecret(ctx, k8sClient, "secret-file2", ns, []string{"sec21", "sec22"})

		cmEnv1 := generateConfigMap(ctx, k8sClient, "cm-env1", ns, map[string]string{"cm11": "11", "cm12": "12"})
		cmEnv2 := generateConfigMap(ctx, k8sClient, "cm-env2", ns, map[string]string{"cm21": "21", "cm22": "22"})

		secretEnv1 := generateSecret(ctx, k8sClient, "secret-env1", ns, []string{"sec11", "sec12"})
		_ = generateSecret(ctx, k8sClient, "secret-env2", ns, []string{"sec21", "sec22"})

		bs := bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				AppConfig: &bsv1alpha1.AppConfig{
					MountPath: "/my/mount/path",
					ConfigMaps: []bsv1alpha1.ObjectKeyRef{
						{Name: appConfig1},
						{Name: appConfig2, Key: "key21"},
					},
				},
				//DynamicPluginsConfigMapName: "",
				ExtraFiles: &bsv1alpha1.ExtraFiles{
					MountPath: "/my/file/path",
					ConfigMaps: []bsv1alpha1.ObjectKeyRef{
						{Name: cmFile1},
						{Name: cmFile2, Key: "cm21"},
					},
					Secrets: []bsv1alpha1.ObjectKeyRef{
						{Name: secretFile1, Key: "sec11"},
						{Name: secretFile2, Key: "sec21"},
					},
				},
				ExtraEnvs: &bsv1alpha1.ExtraEnvs{
					ConfigMaps: []bsv1alpha1.ObjectKeyRef{
						{Name: cmEnv1},
						{Name: cmEnv2, Key: "cm21"},
					},
					Secrets: []bsv1alpha1.ObjectKeyRef{
						{Name: secretEnv1, Key: "sec11"},
					},
					Envs: []bsv1alpha1.Env{
						{Name: "env1", Value: "val1"},
					},
				},
			},
		}
		backstageName := createAndReconcileBackstage(ctx, ns, bs)

		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			podSpec := deploy.Spec.Template.Spec
			c := podSpec.Containers[0]

			By("checking if app-config volumes are added to PodSpec")
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(appConfig1)).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(appConfig2)).To(BeAddedAsVolumeToPodSpec(podSpec))

			By("checking if app-config volumes are mounted to the Backstage container")
			g.Expect("/my/mount/path/key11").To(BeMountedToContainer(c))
			g.Expect("/my/mount/path/key12").To(BeMountedToContainer(c))
			g.Expect("/my/mount/path/key21").To(BeMountedToContainer(c))
			g.Expect("/my/mount/path/key22").NotTo(BeMountedToContainer(c))

			By("checking if app-config args are added to the Backstage container")
			g.Expect("/my/mount/path/key11").To(BeAddedAsArgToContainer(c))
			g.Expect("/my/mount/path/key12").To(BeAddedAsArgToContainer(c))
			g.Expect("/my/mount/path/key21").To(BeAddedAsArgToContainer(c))
			g.Expect("/my/mount/path/key22").NotTo(BeAddedAsArgToContainer(c))

			By("checking if extra-cm-file volumes are added to PodSpec")
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(cmFile1)).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(cmFile2)).To(BeAddedAsVolumeToPodSpec(podSpec))

			By("checking if extra-cm-file volumes are mounted to the Backstage container")
			g.Expect("/my/file/path/cm11").To(BeMountedToContainer(c))
			g.Expect("/my/file/path/cm12").To(BeMountedToContainer(c))
			g.Expect("/my/file/path/cm21").To(BeMountedToContainer(c))
			g.Expect("/my/file/path/cm22").NotTo(BeMountedToContainer(c))

			By("checking if extra-secret-file volumes are added to PodSpec")
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret("secret-file1")).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret("secret-file2")).To(BeAddedAsVolumeToPodSpec(podSpec))

			By("checking if extra-secret-file volumes are mounted to the Backstage container")
			g.Expect("/my/file/path/sec11").To(BeMountedToContainer(c))
			g.Expect("/my/file/path/sec12").NotTo(BeMountedToContainer(c))
			g.Expect("/my/file/path/sec21").To(BeMountedToContainer(c))
			g.Expect("/my/file/path/sec22").NotTo(BeMountedToContainer(c))

			By("checking if extra-envvars are injected to the Backstage container as EnvFrom")
			g.Expect("cm-env1").To(BeEnvFromForContainer(c))

			By("checking if environment variables are injected to the Backstage container as EnvVar")
			g.Expect("env1").To(BeEnvVarForContainer(c))
			g.Expect("cm21").To(BeEnvVarForContainer(c))
			g.Expect("sec11").To(BeEnvVarForContainer(c))

			for _, cond := range deploy.Status.Conditions {
				if cond.Type == "Available" {
					g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
				}
			}
		}, 5*time.Minute, time.Second).Should(Succeed(), controllerMessage())
	})

	It("creates default Backstage and then update CR ", func() {

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1alpha1.BackstageSpec{})

		Eventually(func(g Gomega) {
			By("creating Deployment with replicas=1 by default")
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).To(Not(HaveOccurred()))
			g.Expect(deploy.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)))
			g.Expect(deploy.Spec.Template.Spec.ImagePullSecrets).To(HaveLen(0))

		}, time.Minute, time.Second).Should(Succeed())

		By("updating Backstage")
		imageName := "quay.io/my-org/my-awesome-image:1.2.3"
		ips := []string{"some-image-pull-secret-1", "some-image-pull-secret-2"}
		update := &bsv1alpha1.Backstage{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, update)
		Expect(err).To(Not(HaveOccurred()))
		update.Spec.Application = &bsv1alpha1.Application{}
		update.Spec.Application.Replicas = ptr.To(int32(2))
		update.Spec.Application.Image = ptr.To(imageName)
		update.Spec.Application.ImagePullSecrets = ips

		err = k8sClient.Update(ctx, update)
		Expect(err).To(Not(HaveOccurred()))
		_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		Eventually(func(g Gomega) {

			deploy := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).To(Not(HaveOccurred()))
			g.Expect(deploy.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)))
			g.Expect(deploy.Spec.Template.Spec.ImagePullSecrets).To(HaveLen(2))
			g.Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(HaveValue(BeEquivalentTo(imageName)))

		}, time.Minute, time.Second).Should(Succeed())

	})

	It("creates default Backstage and then update CR to not to use local DB", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, bsv1alpha1.BackstageSpec{})

		Eventually(func(g Gomega) {
			By("creating Deployment with database.enableLocalDb=true by default")

			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DbStatefulSetName(backstageName)}, &appsv1.StatefulSet{})
			g.Expect(err).To(Not(HaveOccurred()))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DbStatefulSetName(backstageName)}, &corev1.Service{})
			g.Expect(err).To(Not(HaveOccurred()))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DbStatefulSetName(backstageName)}, &corev1.Secret{})
			g.Expect(err).To(Not(HaveOccurred()))

		}, time.Minute, time.Second).Should(Succeed())

		By("updating Backstage")
		update := &bsv1alpha1.Backstage{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, update)
		Expect(err).To(Not(HaveOccurred()))
		update.Spec.Database = &bsv1alpha1.Database{}
		update.Spec.Database.EnableLocalDb = ptr.To(false)
		err = k8sClient.Update(ctx, update)
		Expect(err).To(Not(HaveOccurred()))
		_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		Eventually(func(g Gomega) {
			By("deleting Local Db StatefulSet, Service and Secret")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DbStatefulSetName(backstageName)}, &appsv1.StatefulSet{})
			g.Expect(err).To(HaveOccurred())
			g.Expect(errors.IsNotFound(err))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DbServiceName(backstageName)}, &corev1.Service{})
			g.Expect(err).To(HaveOccurred())
			g.Expect(errors.IsNotFound(err))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DbSecretDefaultName(backstageName)}, &corev1.Secret{})
			g.Expect(err).To(HaveOccurred())
			g.Expect(errors.IsNotFound(err))
		}, time.Minute, time.Second).Should(Succeed())

	})

	It("creates Backstage with disabled local DB and secret", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, bsv1alpha1.BackstageSpec{
			Database: &bsv1alpha1.Database{
				EnableLocalDb:  ptr.To(false),
				AuthSecretName: "existing-secret",
			},
		})

		Eventually(func(g Gomega) {
			By("not creating a StatefulSet for the Database")
			err := k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: model.DbStatefulSetName(backstageName)},
				&appsv1.StatefulSet{})
			g.Expect(err).Should(HaveOccurred())
			g.Expect(errors.IsNotFound(err))

			By("Checking if Deployment was successfully created in the reconciliation")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, &appsv1.Deployment{})
			g.Expect(err).Should(Not(HaveOccurred()))
		}, time.Minute, time.Second).Should(Succeed())
	})

	It("creates Backstage with disabled local DB no secret", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, bsv1alpha1.BackstageSpec{
			Database: &bsv1alpha1.Database{
				EnableLocalDb: ptr.To(false),
			},
		})

		Eventually(func(g Gomega) {
			By("not creating a StatefulSet for the Database")
			err := k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: model.DbStatefulSetName(backstageName)},
				&appsv1.StatefulSet{})
			g.Expect(err).Should(HaveOccurred())
			g.Expect(errors.IsNotFound(err))

			By("Checking if Deployment was successfully created in the reconciliation")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, &appsv1.Deployment{})
			g.Expect(err).Should(Not(HaveOccurred()))
		}, time.Minute, time.Second).Should(Succeed())
	})
})

// Duplicated files in different CMs
// Message: "Deployment.apps \"test-backstage-ro86g-deployment\" is invalid: spec.template.spec.containers[0].volumeMounts[4].mountPath: Invalid value: \"/my/mount/path/key12\": must be unique",

// No CM configured
//failed to preprocess backstage spec app-configs failed to get configMap app-config3: configmaps "app-config3" not found

// If no such a key - no reaction, it is just not included

// mounting/injecting secret by key only

// TODO test for Raw Config https://github.com/janus-idp/operator/issues/202
