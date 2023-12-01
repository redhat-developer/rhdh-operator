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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bsv1alpha1 "janus-idp.io/backstage-operator/api/v1alpha1"
)

var _ = Describe("Backstage controller", func() {
	var (
		ctx                 context.Context
		ns                  string
		backstageName       string
		backstageReconciler *BackstageReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		ns = fmt.Sprintf("ns-%d-%s", GinkgoParallelProcess(), randString(5))
		backstageName = "test-backstage-" + randString(5)

		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ns,
				Namespace: ns,
			},
		})
		Expect(err).To(Not(HaveOccurred()))

		backstageReconciler = &BackstageReconciler{
			Client:      k8sClient,
			Scheme:      k8sClient.Scheme(),
			Namespace:   ns,
			OwnsRuntime: true,
		}
	})

	AfterEach(func() {
		// NOTE: Be aware of the current delete namespace limitations.
		// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ns,
				Namespace: ns,
			},
		})
	})

	buildBackstageCR := func(spec bsv1alpha1.BackstageSpec) *bsv1alpha1.Backstage {
		return &bsv1alpha1.Backstage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backstageName,
				Namespace: ns,
			},
			Spec: spec,
		}
	}

	buildConfigMap := func(name string, data map[string]string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
			Data: data,
		}
	}

	buildSecret := func(name string, data map[string][]byte) *corev1.Secret {
		return &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
			Data: data,
		}
	}

	verifyBackstageInstance := func(ctx context.Context) {
		Eventually(func(g Gomega) {
			var backstage bsv1alpha1.Backstage
			err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, &backstage)
			g.Expect(err).NotTo(HaveOccurred())
			//TODO the status is under construction
			g.Expect(len(backstage.Status.Conditions)).To(Equal(2))
		}, time.Minute, time.Second).Should(Succeed())
	}

	findEnvVar := func(envVars []corev1.EnvVar, key string) (corev1.EnvVar, bool) {
		return findElementByPredicate(envVars, func(envVar corev1.EnvVar) bool {
			return envVar.Name == key
		})
	}

	findVolume := func(vols []corev1.Volume, name string) (corev1.Volume, bool) {
		return findElementByPredicate(vols, func(vol corev1.Volume) bool {
			return vol.Name == name
		})
	}

	findVolumeMount := func(mounts []corev1.VolumeMount, name string) (corev1.VolumeMount, bool) {
		return findElementByPredicate(mounts, func(mount corev1.VolumeMount) bool {
			return mount.Name == name
		})
	}

	When("creating default CR with no spec", func() {
		var backstage *bsv1alpha1.Backstage
		BeforeEach(func() {
			backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{})
			err := k8sClient.Create(ctx, backstage)
			Expect(err).To(Not(HaveOccurred()))
		})

		It("should successfully reconcile a custom resource for default Backstage", func() {
			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &bsv1alpha1.Backstage{}
				return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Reconciling the custom resource created")
			_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Generating a value for backend auth secret key")
			Eventually(func(g Gomega) {
				found := &corev1.Secret{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: backstageName + "-auth"}, found)
				g.Expect(err).ShouldNot(HaveOccurred())

				g.Expect(found.Data).To(HaveKey("backend-secret"))
				g.Expect(found.Data["backend-secret"]).To(Not(BeEmpty()),
					"backend auth secret should contain a non-empty 'backend-secret' in its data")
			}, time.Minute, time.Second).Should(Succeed())

			By("Generating a ConfigMap for default config for dynamic plugins")
			dynamicPluginsConfigName := fmt.Sprintf("%s-dynamic-plugins", backstageName)
			Eventually(func(g Gomega) {
				found := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: dynamicPluginsConfigName}, found)
				g.Expect(err).ShouldNot(HaveOccurred())

				g.Expect(found.Data).To(HaveKey("dynamic-plugins.yaml"))
				g.Expect(found.Data["dynamic-plugins.yaml"]).To(Not(BeEmpty()),
					"default ConfigMap for dynamic plugins should contain a non-empty 'dynamic-plugins.yaml' in its data")
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking if Deployment was successfully created in the reconciliation")
			found := &appsv1.Deployment{}
			Eventually(func() error {
				// TODO to get name from default
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking that the Deployment is configured with a random backend auth secret")
			backendSecretEnvVar, ok := findEnvVar(found.Spec.Template.Spec.Containers[0].Env, "BACKEND_SECRET")
			Expect(ok).To(BeTrue(), "env var BACKEND_SECRET not found in main container")
			Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Name).To(
				Not(BeEmpty()), "'name' for backend auth secret ref should not be empty")
			Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Key).To(
				Equal("backend-secret"), "Unexpected secret key ref for backend secret")
			Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Optional).To(HaveValue(BeFalse()),
				"'optional' for backend auth secret ref should be 'false'")

			backendAuthAppConfigEnvVar, ok := findEnvVar(found.Spec.Template.Spec.Containers[0].Env, "APP_CONFIG_backend_auth_keys")
			Expect(ok).To(BeTrue(), "env var APP_CONFIG_backend_auth_keys not found in main container")
			Expect(backendAuthAppConfigEnvVar.Value).To(Equal(`[{"secret": "$(BACKEND_SECRET)"}]`))

			By("Checking the Volumes in the Backstage Deployment", func() {
				Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(3))

				_, ok := findVolume(found.Spec.Template.Spec.Volumes, "dynamic-plugins-root")
				Expect(ok).To(BeTrue(), "No volume found with name: dynamic-plugins-root")

				_, ok = findVolume(found.Spec.Template.Spec.Volumes, "dynamic-plugins-npmrc")
				Expect(ok).To(BeTrue(), "No volume found with name: dynamic-plugins-npmrc")

				dynamicPluginsConfigVol, ok := findVolume(found.Spec.Template.Spec.Volumes, dynamicPluginsConfigName)
				Expect(ok).To(BeTrue(), "No volume found with name: %s", dynamicPluginsConfigName)
				Expect(dynamicPluginsConfigVol.VolumeSource.Secret).To(BeNil())
				Expect(dynamicPluginsConfigVol.VolumeSource.ConfigMap.DefaultMode).To(HaveValue(Equal(int32(420))))
				Expect(dynamicPluginsConfigVol.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(dynamicPluginsConfigName))
			})

			By("Checking the Number of init containers in the Backstage Deployment")
			Expect(found.Spec.Template.Spec.InitContainers).To(HaveLen(1))
			initCont := found.Spec.Template.Spec.InitContainers[0]

			By("Checking the Init Container Env Vars in the Backstage Deployment", func() {
				Expect(initCont.Env).To(HaveLen(1))
				Expect(initCont.Env[0].Name).To(Equal("NPM_CONFIG_USERCONFIG"))
				Expect(initCont.Env[0].Value).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins"))
			})

			By("Checking the Init Container Volume Mounts in the Backstage Deployment", func() {
				Expect(initCont.VolumeMounts).To(HaveLen(3))

				dpRoot, ok := findVolumeMount(initCont.VolumeMounts, "dynamic-plugins-root")
				Expect(ok).To(BeTrue(),
					"No volume mount found with name: dynamic-plugins-root")
				Expect(dpRoot.MountPath).To(Equal("/dynamic-plugins-root"))
				Expect(dpRoot.ReadOnly).To(BeFalse())
				Expect(dpRoot.SubPath).To(BeEmpty())

				dpNpmrc, ok := findVolumeMount(initCont.VolumeMounts, "dynamic-plugins-npmrc")
				Expect(ok).To(BeTrue(),
					"No volume mount found with name: dynamic-plugins-npmrc")
				Expect(dpNpmrc.MountPath).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins"))
				Expect(dpNpmrc.ReadOnly).To(BeTrue())
				Expect(dpNpmrc.SubPath).To(Equal(".npmrc"))

				dp, ok := findVolumeMount(initCont.VolumeMounts, dynamicPluginsConfigName)
				Expect(ok).To(BeTrue(), "No volume mount found with name: %s", dynamicPluginsConfigName)
				Expect(dp.MountPath).To(Equal("/opt/app-root/src/dynamic-plugins.yaml"))
				Expect(dp.SubPath).To(Equal("dynamic-plugins.yaml"))
				Expect(dp.ReadOnly).To(BeTrue())
			})

			By("Checking the Number of main containers in the Backstage Deployment")
			Expect(found.Spec.Template.Spec.Containers).To(HaveLen(1))
			mainCont := found.Spec.Template.Spec.Containers[0]

			By("Checking the main container Args in the Backstage Deployment", func() {
				Expect(mainCont.Args).To(HaveLen(2))
				Expect(mainCont.Args[0]).To(Equal("--config"))
				Expect(mainCont.Args[1]).To(Equal("dynamic-plugins-root/app-config.dynamic-plugins.yaml"))
			})

			By("Checking the main container Volume Mounts in the Backstage Deployment", func() {
				Expect(mainCont.VolumeMounts).To(HaveLen(1))

				dpRoot, ok := findVolumeMount(mainCont.VolumeMounts, "dynamic-plugins-root")
				Expect(ok).To(BeTrue(), "No volume mount found with name: dynamic-plugins-root")
				Expect(dpRoot.MountPath).To(Equal("/opt/app-root/src/dynamic-plugins-root"))
				Expect(dpRoot.SubPath).To(BeEmpty())
			})

			By("Checking the latest Status added to the Backstage instance")
			verifyBackstageInstance(ctx)
		})
	})

	Context("specifying runtime configs", func() {
		When("creating CR with runtime config for Backstage deployment", func() {
			var backstage *bsv1alpha1.Backstage

			BeforeEach(func() {
				backstageConfigMap := buildConfigMap("my-bs-config",
					map[string]string{
						"deployment.yaml": `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bs1-deployment
  labels:
    app: bs1
spec:
  replicas: 1
  selector:
    matchLabels:
      app: bs1
  template:
    metadata:
      labels:
        app: bs1
    spec:
      containers:
        - name: bs1
          image: busybox
`,
					})
				err := k8sClient.Create(ctx, backstageConfigMap)
				Expect(err).To(Not(HaveOccurred()))

				backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
					RawRuntimeConfig: bsv1alpha1.RuntimeConfig{
						BackstageConfigName: backstageConfigMap.Name,
					},
				})

				err = k8sClient.Create(ctx, backstage)
				Expect(err).To(Not(HaveOccurred()))
			})

			It("should create the resources", func() {
				By("Checking if the custom resource was successfully created")
				Eventually(func() error {
					found := &bsv1alpha1.Backstage{}
					return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
				}, time.Minute, time.Second).Should(Succeed())

				By("Reconciling the custom resource created")
				_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
				})
				Expect(err).To(Not(HaveOccurred()))

				By("Checking if Deployment was successfully created in the reconciliation")
				Eventually(func() error {
					found := &appsv1.Deployment{}
					return k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "bs1-deployment"}, found)
				}, time.Minute, time.Second).Should(Succeed())

				By("Checking the latest Status added to the Backstage instance")
				verifyBackstageInstance(ctx)
			})
		})

		When("creating CR with runtime config for the database", func() {
			var backstage *bsv1alpha1.Backstage

			BeforeEach(func() {
				localDbConfigMap := buildConfigMap("my-db-config", map[string]string{
					"db-statefulset.yaml": `
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db-statefulset
spec:
  replicas: 3
  selector:
    matchLabels:
      app: db
  template:
    metadata:
      labels:
        app: db
    spec:
      containers:
        - name: db
          image: busybox
`,
				})
				err := k8sClient.Create(ctx, localDbConfigMap)
				Expect(err).To(Not(HaveOccurred()))

				backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
					RawRuntimeConfig: bsv1alpha1.RuntimeConfig{
						LocalDbConfigName: localDbConfigMap.Name,
					},
				})

				err = k8sClient.Create(ctx, backstage)
				Expect(err).To(Not(HaveOccurred()))
			})

			It("should create the resources", func() {
				By("Checking if the custom resource was successfully created")
				Eventually(func() error {
					found := &bsv1alpha1.Backstage{}
					return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
				}, time.Minute, time.Second).Should(Succeed())

				By("Reconciling the custom resource created")
				_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
				})
				Expect(err).To(Not(HaveOccurred()))

				By("Checking if StatefulSet was successfully created in the reconciliation")
				Eventually(func(g Gomega) {
					found := &appsv1.StatefulSet{}
					name := fmt.Sprintf("backstage-psql-%s", backstage.Name)
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, found)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(found.Spec.Replicas).Should(HaveValue(BeEquivalentTo(3)))
					// Make sure the ownerrefs are correctly set based on backstage CR
					ownerRefs := found.GetOwnerReferences()
					backstageCreated := &bsv1alpha1.Backstage{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, backstageCreated)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(ownerRefs).Should(HaveLen(1))
					g.Expect(ownerRefs[0].APIVersion).Should(Equal(bsv1alpha1.GroupVersion.String()))
					g.Expect(ownerRefs[0].Kind).Should(Equal("Backstage"))
					g.Expect(ownerRefs[0].Name).Should(Equal(backstage.Name))
					g.Expect(ownerRefs[0].UID).Should(Equal(backstageCreated.UID))
				}, time.Minute, time.Second).Should(Succeed())

				By("Checking the latest Status added to the Backstage instance")
				verifyBackstageInstance(ctx)
			})
		})
	})

	Context("App Configs", func() {
		for _, kind := range []string{"ConfigMap", "Secret"} {
			kind := kind
			When(fmt.Sprintf("referencing non-existing %s as app-config", kind), func() {
				var backstage *bsv1alpha1.Backstage

				BeforeEach(func() {
					backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
						AppConfigs: []bsv1alpha1.AppConfigRef{
							{
								Name: "a-non-existing-" + strings.ToLower(kind),
								Kind: kind,
							},
						},
					})
					err := k8sClient.Create(ctx, backstage)
					Expect(err).To(Not(HaveOccurred()))
				})

				It("should fail to reconcile", func() {
					By("Checking if the custom resource was successfully created")
					Eventually(func() error {
						found := &bsv1alpha1.Backstage{}
						return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
					}, time.Minute, time.Second).Should(Succeed())

					By("Not reconciling the custom resource created")
					_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
					})
					Expect(err).To(HaveOccurred())

					By("Not creating a Backstage Deployment")
					Consistently(func() error {
						// TODO to get name from default
						return k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, &appsv1.Deployment{})
					}, 5*time.Second, time.Second).Should(Not(Succeed()))
				})
			})
		}

		for _, dynamicPluginsConfigKind := range []string{"ConfigMap", "Secret"} {
			dynamicPluginsConfigKind := dynamicPluginsConfigKind
			When("referencing ConfigMaps and Secrets for app-configs and dynamic plugins config as "+dynamicPluginsConfigKind, func() {
				const (
					appConfig1CmName         = "my-app-config-1-cm"
					appConfig2SecretName     = "my-app-config-2-secret"
					dynamicPluginsConfigName = "my-dynamic-plugins-config"
				)

				var backstage *bsv1alpha1.Backstage

				BeforeEach(func() {
					appConfig1Cm := buildConfigMap(appConfig1CmName, map[string]string{
						"my-app-config-11.yaml": `
# my-app-config-11.yaml
`,
						"my-app-config-12.yaml": `
# my-app-config-12.yaml
`,
					})
					err := k8sClient.Create(ctx, appConfig1Cm)
					Expect(err).To(Not(HaveOccurred()))

					appConfig2Secret := buildSecret(appConfig2SecretName, map[string][]byte{
						"my-app-config-21.yaml": []byte(`
# my-app-config-21.yaml
`),
						"my-app-config-22.yaml": []byte(`
# my-app-config-22.yaml
`),
					})
					err = k8sClient.Create(ctx, appConfig2Secret)
					Expect(err).To(Not(HaveOccurred()))

					var dynamicPluginsObject client.Object
					switch dynamicPluginsConfigKind {
					case "ConfigMap":
						dynamicPluginsObject = buildConfigMap(dynamicPluginsConfigName, map[string]string{
							"dynamic-plugins.yaml": `
# dynamic-plugins.yaml (configmap)
includes: [dynamic-plugins.default.yaml]
plugins: []
`,
						})
					case "Secret":
						dynamicPluginsObject = buildSecret(dynamicPluginsConfigName, map[string][]byte{
							"dynamic-plugins.yaml": []byte(`
# dynamic-plugins.yaml (secret)
includes: [dynamic-plugins.default.yaml]
plugins: []
`),
						})
					default:
						Fail(fmt.Sprintf("unsupported kind for dynamic plugins object: %q", dynamicPluginsConfigKind))
					}
					err = k8sClient.Create(ctx, dynamicPluginsObject)
					Expect(err).To(Not(HaveOccurred()))

					backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
						AppConfigs: []bsv1alpha1.AppConfigRef{
							{
								Name: appConfig1CmName,
								Kind: "ConfigMap",
							},
							{
								Name: appConfig2SecretName,
								Kind: "Secret",
							},
						},
						DynamicPluginsConfig: &bsv1alpha1.DynamicPluginsConfigRef{
							Name: dynamicPluginsConfigName,
							Kind: dynamicPluginsConfigKind,
						},
					})
					err = k8sClient.Create(ctx, backstage)
					Expect(err).To(Not(HaveOccurred()))
				})

				It("should reconcile", func() {
					By("Checking if the custom resource was successfully created")
					Eventually(func() error {
						found := &bsv1alpha1.Backstage{}
						return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
					}, time.Minute, time.Second).Should(Succeed())

					By("Reconciling the custom resource created")
					_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
					})
					Expect(err).To(Not(HaveOccurred()))

					By("Checking that the Deployment was successfully created in the reconciliation")
					found := &appsv1.Deployment{}
					Eventually(func(g Gomega) {
						// TODO to get name from default
						err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, found)
						g.Expect(err).To(Not(HaveOccurred()))
					}, time.Minute, time.Second).Should(Succeed())

					By("Checking the Volumes in the Backstage Deployment", func() {
						Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(5))

						_, ok := findVolume(found.Spec.Template.Spec.Volumes, "dynamic-plugins-root")
						Expect(ok).To(BeTrue(), "No volume found with name: dynamic-plugins-root")

						_, ok = findVolume(found.Spec.Template.Spec.Volumes, "dynamic-plugins-npmrc")
						Expect(ok).To(BeTrue(), "No volume found with name: dynamic-plugins-npmrc")

						appConfig1CmVol, ok := findVolume(found.Spec.Template.Spec.Volumes, appConfig1CmName)
						Expect(ok).To(BeTrue(), "No volume found with name: %s", appConfig1CmName)
						Expect(appConfig1CmVol.VolumeSource.Secret).To(BeNil())
						Expect(appConfig1CmVol.VolumeSource.ConfigMap.DefaultMode).To(HaveValue(Equal(int32(420))))
						Expect(appConfig1CmVol.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(appConfig1CmName))

						appConfig2SecretVol, ok := findVolume(found.Spec.Template.Spec.Volumes, appConfig2SecretName)
						Expect(ok).To(BeTrue(), "No volume found with name: %s", appConfig2SecretName)
						Expect(appConfig2SecretVol.VolumeSource.ConfigMap).To(BeNil())
						Expect(appConfig2SecretVol.VolumeSource.Secret.DefaultMode).To(HaveValue(Equal(int32(420))))
						Expect(appConfig2SecretVol.VolumeSource.Secret.SecretName).To(Equal(appConfig2SecretName))

						dynamicPluginsConfigVol, ok := findVolume(found.Spec.Template.Spec.Volumes, dynamicPluginsConfigName)
						Expect(ok).To(BeTrue(), "No volume found with name: %s", dynamicPluginsConfigName)
						switch dynamicPluginsConfigKind {
						case "ConfigMap":
							Expect(dynamicPluginsConfigVol.VolumeSource.Secret).To(BeNil())
							Expect(dynamicPluginsConfigVol.VolumeSource.ConfigMap.DefaultMode).To(HaveValue(Equal(int32(420))))
							Expect(dynamicPluginsConfigVol.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(dynamicPluginsConfigName))
						case "Secret":
							Expect(dynamicPluginsConfigVol.VolumeSource.ConfigMap).To(BeNil())
							Expect(dynamicPluginsConfigVol.VolumeSource.Secret.DefaultMode).To(HaveValue(Equal(int32(420))))
							Expect(dynamicPluginsConfigVol.VolumeSource.Secret.SecretName).To(Equal(dynamicPluginsConfigName))
						}
					})

					By("Checking the Number of init containers in the Backstage Deployment")
					Expect(found.Spec.Template.Spec.InitContainers).To(HaveLen(1))
					initCont := found.Spec.Template.Spec.InitContainers[0]

					By("Checking the Init Container Env Vars in the Backstage Deployment", func() {
						Expect(initCont.Env).To(HaveLen(1))
						Expect(initCont.Env[0].Name).To(Equal("NPM_CONFIG_USERCONFIG"))
						Expect(initCont.Env[0].Value).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins"))
					})

					By("Checking the Init Container Volume Mounts in the Backstage Deployment", func() {
						Expect(initCont.VolumeMounts).To(HaveLen(3))

						dpRoot, ok := findVolumeMount(initCont.VolumeMounts, "dynamic-plugins-root")
						Expect(ok).To(BeTrue(),
							"No volume mount found with name: dynamic-plugins-root")
						Expect(dpRoot.MountPath).To(Equal("/dynamic-plugins-root"))
						Expect(dpRoot.ReadOnly).To(BeFalse())
						Expect(dpRoot.SubPath).To(BeEmpty())

						dpNpmrc, ok := findVolumeMount(initCont.VolumeMounts, "dynamic-plugins-npmrc")
						Expect(ok).To(BeTrue(),
							"No volume mount found with name: dynamic-plugins-npmrc")
						Expect(dpNpmrc.MountPath).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins"))
						Expect(dpNpmrc.ReadOnly).To(BeTrue())
						Expect(dpNpmrc.SubPath).To(Equal(".npmrc"))

						dp, ok := findVolumeMount(initCont.VolumeMounts, dynamicPluginsConfigName)
						Expect(ok).To(BeTrue(), "No volume mount found with name: %s", dynamicPluginsConfigName)
						Expect(dp.MountPath).To(Equal("/opt/app-root/src/dynamic-plugins.yaml"))
						Expect(dp.SubPath).To(Equal("dynamic-plugins.yaml"))
						Expect(dp.ReadOnly).To(BeTrue())
					})

					By("Checking the Number of main containers in the Backstage Deployment")
					Expect(found.Spec.Template.Spec.Containers).To(HaveLen(1))
					mainCont := found.Spec.Template.Spec.Containers[0]

					By("Checking the main container Args in the Backstage Deployment", func() {
						Expect(mainCont.Args).To(HaveLen(10))
						Expect(mainCont.Args[1]).To(Equal("dynamic-plugins-root/app-config.dynamic-plugins.yaml"))
						for i := 0; i <= 8; i += 2 {
							Expect(mainCont.Args[i]).To(Equal("--config"))
						}
						//TODO(rm3l): the order of the rest of the --config args should be the same as the order in
						// which the keys are listed in the ConfigMap/Secrets
						// But as this is returned as a map, Go does not provide any guarantee on the iteration order.
						Expect(mainCont.Args[3]).To(SatisfyAny(
							Equal("/opt/app-root/src/my-app-config-1-cm/my-app-config-11.yaml"),
							Equal("/opt/app-root/src/my-app-config-1-cm/my-app-config-12.yaml"),
						))
						Expect(mainCont.Args[5]).To(SatisfyAny(
							Equal("/opt/app-root/src/my-app-config-1-cm/my-app-config-11.yaml"),
							Equal("/opt/app-root/src/my-app-config-1-cm/my-app-config-12.yaml"),
						))
						Expect(mainCont.Args[3]).To(Not(Equal(mainCont.Args[5])))
						Expect(mainCont.Args[7]).To(SatisfyAny(
							Equal("/opt/app-root/src/my-app-config-2-secret/my-app-config-21.yaml"),
							Equal("/opt/app-root/src/my-app-config-2-secret/my-app-config-22.yaml"),
						))
						Expect(mainCont.Args[9]).To(SatisfyAny(
							Equal("/opt/app-root/src/my-app-config-2-secret/my-app-config-21.yaml"),
							Equal("/opt/app-root/src/my-app-config-2-secret/my-app-config-22.yaml"),
						))
						Expect(mainCont.Args[7]).To(Not(Equal(mainCont.Args[9])))
					})

					By("Checking the main container Volume Mounts in the Backstage Deployment", func() {
						Expect(mainCont.VolumeMounts).To(HaveLen(3))

						dpRoot, ok := findVolumeMount(mainCont.VolumeMounts, "dynamic-plugins-root")
						Expect(ok).To(BeTrue(), "No volume mount found with name: dynamic-plugins-root")
						Expect(dpRoot.MountPath).To(Equal("/opt/app-root/src/dynamic-plugins-root"))
						Expect(dpRoot.SubPath).To(BeEmpty())

						appConfig1CmMount, ok := findVolumeMount(mainCont.VolumeMounts, appConfig1CmName)
						Expect(ok).To(BeTrue(), "No volume mount found with name: %s", appConfig1CmName)
						Expect(appConfig1CmMount.MountPath).To(Equal("/opt/app-root/src/my-app-config-1-cm"))
						Expect(appConfig1CmMount.SubPath).To(BeEmpty())

						appConfig2SecretMount, ok := findVolumeMount(mainCont.VolumeMounts, appConfig2SecretName)
						Expect(ok).To(BeTrue(), "No volume mount found with name: %s", appConfig2SecretName)
						Expect(appConfig2SecretMount.MountPath).To(Equal("/opt/app-root/src/my-app-config-2-secret"))
						Expect(appConfig2SecretMount.SubPath).To(BeEmpty())
					})

					By("Checking the latest Status added to the Backstage instance")
					verifyBackstageInstance(ctx)

				})
			})
		}
	})

	Context("Backend Auth Secret", func() {
		for _, key := range []string{"", "some-key"} {
			key := key
			When("creating CR with a non existing backend secret ref and key="+key, func() {
				var backstage *bsv1alpha1.Backstage
				BeforeEach(func() {
					backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
						BackendAuthSecretRef: &bsv1alpha1.BackendAuthSecretRef{
							Name: "non-existing-secret",
							Key:  key,
						},
					})
					err := k8sClient.Create(ctx, backstage)
					Expect(err).To(Not(HaveOccurred()))
				})

				It("should reconcile", func() {
					By("Checking if the custom resource was successfully created")
					Eventually(func() error {
						found := &bsv1alpha1.Backstage{}
						return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
					}, time.Minute, time.Second).Should(Succeed())

					By("Reconciling the custom resource created")
					_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
					})
					Expect(err).To(Not(HaveOccurred()))

					By("Not generating a value for backend auth secret key")
					Consistently(func(g Gomega) {
						found := &corev1.Secret{}
						err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: backstageName + "-auth"}, found)
						g.Expect(err).Should(HaveOccurred())
						g.Expect(errors.IsNotFound(err)).To(BeTrue(),
							fmt.Sprintf("error must be a not-found error, but is %v", err))
					}, 5*time.Second, time.Second).Should(Succeed())

					By("Checking that the Deployment was successfully created in the reconciliation")
					found := &appsv1.Deployment{}
					Eventually(func() error {
						// TODO to get name from default
						return k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, found)
					}, time.Minute, time.Second).Should(Succeed())

					By("Checking that the Deployment is configured with the specified secret", func() {
						expectedKey := key
						if key == "" {
							expectedKey = "backend-secret"
						}
						backendSecretEnvVar, ok := findEnvVar(found.Spec.Template.Spec.Containers[0].Env, "BACKEND_SECRET")
						Expect(ok).To(BeTrue(), "env var BACKEND_SECRET not found in main container")
						Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Name).To(
							Equal("non-existing-secret"), "'name' for backend auth secret ref should not be empty")
						Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Key).To(
							Equal(expectedKey), "Unexpected secret key ref for backend secret")
						Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Optional).To(HaveValue(BeFalse()),
							"'optional' for backend auth secret ref should be 'false'")

						backendAuthAppConfigEnvVar, ok := findEnvVar(found.Spec.Template.Spec.Containers[0].Env, "APP_CONFIG_backend_auth_keys")
						Expect(ok).To(BeTrue(), "env var APP_CONFIG_backend_auth_keys not found in main container")
						Expect(backendAuthAppConfigEnvVar.Value).To(Equal(`[{"secret": "$(BACKEND_SECRET)"}]`))
					})

					By("Checking the latest Status added to the Backstage instance")
					verifyBackstageInstance(ctx)
				})
			})

			When("creating CR with an existing backend secret ref and key="+key, func() {
				const backendAuthSecretName = "my-backend-auth-secret"
				var backstage *bsv1alpha1.Backstage

				BeforeEach(func() {
					d := make(map[string][]byte)
					if key != "" {
						d[key] = []byte("lorem-ipsum-dolor-sit-amet")
					}
					backendAuthSecret := buildSecret(backendAuthSecretName, d)
					err := k8sClient.Create(ctx, backendAuthSecret)
					Expect(err).To(Not(HaveOccurred()))
					backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
						BackendAuthSecretRef: &bsv1alpha1.BackendAuthSecretRef{
							Name: backendAuthSecretName,
							Key:  key,
						},
					})
					err = k8sClient.Create(ctx, backstage)
					Expect(err).To(Not(HaveOccurred()))
				})

				It("should reconcile", func() {
					By("Checking if the custom resource was successfully created")
					Eventually(func() error {
						found := &bsv1alpha1.Backstage{}
						return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
					}, time.Minute, time.Second).Should(Succeed())

					By("Reconciling the custom resource created")
					_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
					})
					Expect(err).To(Not(HaveOccurred()))

					By("Not generating a value for backend auth secret key")
					Consistently(func(g Gomega) {
						found := &corev1.Secret{}
						err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: backstageName + "-auth"}, found)
						g.Expect(err).Should(HaveOccurred())
						g.Expect(errors.IsNotFound(err)).To(BeTrue(),
							fmt.Sprintf("error must be a not-found error, but is %v", err))
					}, 5*time.Second, time.Second).Should(Succeed())

					By("Checking that the Deployment was successfully created in the reconciliation")
					found := &appsv1.Deployment{}
					Eventually(func() error {
						// TODO to get name from default
						return k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, found)
					}, time.Minute, time.Second).Should(Succeed())

					By("Checking that the Deployment is configured with the specified secret", func() {
						expectedKey := key
						if key == "" {
							expectedKey = "backend-secret"
						}
						backendSecretEnvVar, ok := findEnvVar(found.Spec.Template.Spec.Containers[0].Env, "BACKEND_SECRET")
						Expect(ok).To(BeTrue(), "env var BACKEND_SECRET not found in main container")
						Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Name).To(Equal(backendAuthSecretName))
						Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Key).To(
							Equal(expectedKey), "Unexpected secret key ref for backend secret")
						Expect(backendSecretEnvVar.ValueFrom.SecretKeyRef.Optional).To(HaveValue(BeFalse()),
							"'optional' for backend auth secret ref should be 'false'")

						backendAuthAppConfigEnvVar, ok := findEnvVar(found.Spec.Template.Spec.Containers[0].Env, "APP_CONFIG_backend_auth_keys")
						Expect(ok).To(BeTrue(), "env var APP_CONFIG_backend_auth_keys not found in main container")
						Expect(backendAuthAppConfigEnvVar.Value).To(Equal(`[{"secret": "$(BACKEND_SECRET)"}]`))
					})

					By("Checking the latest Status added to the Backstage instance")
					verifyBackstageInstance(ctx)
				})
			})
		}
	})
})

func findElementByPredicate[T any](l []T, predicate func(t T) bool) (T, bool) {
	for _, v := range l {
		if predicate(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}
