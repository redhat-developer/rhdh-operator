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
	"k8s.io/utils/pointer"
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
		list := findElementsByPredicate(envVars, func(envVar corev1.EnvVar) bool {
			return envVar.Name == key
		})
		if len(list) == 0 {
			return corev1.EnvVar{}, false
		}
		return list[0], true
	}

	findEnvVarFrom := func(envVars []corev1.EnvFromSource, key string) (corev1.EnvFromSource, bool) {
		list := findElementsByPredicate(envVars, func(envVar corev1.EnvFromSource) bool {
			var n string
			switch {
			case envVar.ConfigMapRef != nil:
				n = envVar.ConfigMapRef.Name
			case envVar.SecretRef != nil:
				n = envVar.SecretRef.Name
			}
			return n == key
		})
		if len(list) == 0 {
			return corev1.EnvFromSource{}, false
		}
		return list[0], true
	}

	findVolume := func(vols []corev1.Volume, name string) (corev1.Volume, bool) {
		list := findElementsByPredicate(vols, func(vol corev1.Volume) bool {
			return vol.Name == name
		})
		if len(list) == 0 {
			return corev1.Volume{}, false
		}
		return list[0], true
	}

	findVolumeMounts := func(mounts []corev1.VolumeMount, name string) []corev1.VolumeMount {
		return findElementsByPredicate(mounts, func(mount corev1.VolumeMount) bool {
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

			By("creating a StatefulSet for the Database")
			Eventually(func(g Gomega) {
				found := &appsv1.StatefulSet{}
				name := fmt.Sprintf("backstage-psql-%s", backstage.Name)
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, found)
				g.Expect(err).ShouldNot(HaveOccurred())
			}, time.Minute, time.Second).Should(Succeed())

			backendAuthConfigName := fmt.Sprintf("%s-auth-app-config", backstageName)
			By("Creating a ConfigMap for default backend auth key", func() {
				Eventually(func(g Gomega) {
					found := &corev1.ConfigMap{}
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: backendAuthConfigName}, found)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(found.Data).ToNot(BeEmpty(), "backend auth secret should contain non-empty data")
				}, time.Minute, time.Second).Should(Succeed())
			})

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

			By("checking the number of replicas")
			Expect(found.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)))

			By("Checking the Volumes in the Backstage Deployment", func() {
				Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(4))

				_, ok := findVolume(found.Spec.Template.Spec.Volumes, "dynamic-plugins-root")
				Expect(ok).To(BeTrue(), "No volume found with name: dynamic-plugins-root")

				_, ok = findVolume(found.Spec.Template.Spec.Volumes, "dynamic-plugins-npmrc")
				Expect(ok).To(BeTrue(), "No volume found with name: dynamic-plugins-npmrc")

				dynamicPluginsConfigVol, ok := findVolume(found.Spec.Template.Spec.Volumes, dynamicPluginsConfigName)
				Expect(ok).To(BeTrue(), "No volume found with name: %s", dynamicPluginsConfigName)
				Expect(dynamicPluginsConfigVol.VolumeSource.Secret).To(BeNil())
				Expect(dynamicPluginsConfigVol.VolumeSource.ConfigMap.DefaultMode).To(HaveValue(Equal(int32(420))))
				Expect(dynamicPluginsConfigVol.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(dynamicPluginsConfigName))

				backendAuthAppConfigVol, ok := findVolume(found.Spec.Template.Spec.Volumes, backendAuthConfigName)
				Expect(ok).To(BeTrue(), "No volume found with name: %s", backendAuthConfigName)
				Expect(backendAuthAppConfigVol.VolumeSource.Secret).To(BeNil())
				Expect(backendAuthAppConfigVol.VolumeSource.ConfigMap.DefaultMode).To(HaveValue(Equal(int32(420))))
				Expect(backendAuthAppConfigVol.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(backendAuthConfigName))
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

				dpRoot := findVolumeMounts(initCont.VolumeMounts, "dynamic-plugins-root")
				Expect(dpRoot).To(HaveLen(1), "No volume mount found with name: dynamic-plugins-root")
				Expect(dpRoot[0].MountPath).To(Equal("/dynamic-plugins-root"))
				Expect(dpRoot[0].ReadOnly).To(BeFalse())
				Expect(dpRoot[0].SubPath).To(BeEmpty())

				dpNpmrc := findVolumeMounts(initCont.VolumeMounts, "dynamic-plugins-npmrc")
				Expect(dpNpmrc).To(HaveLen(1), "No volume mount found with name: dynamic-plugins-npmrc")
				Expect(dpNpmrc[0].MountPath).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins"))
				Expect(dpNpmrc[0].ReadOnly).To(BeTrue())
				Expect(dpNpmrc[0].SubPath).To(Equal(".npmrc"))

				dp := findVolumeMounts(initCont.VolumeMounts, dynamicPluginsConfigName)
				Expect(dp).To(HaveLen(1), "No volume mount found with name: %s", dynamicPluginsConfigName)
				Expect(dp[0].MountPath).To(Equal("/opt/app-root/src/dynamic-plugins.yaml"))
				Expect(dp[0].SubPath).To(Equal("dynamic-plugins.yaml"))
				Expect(dp[0].ReadOnly).To(BeTrue())
			})

			By("Checking the Number of main containers in the Backstage Deployment")
			Expect(found.Spec.Template.Spec.Containers).To(HaveLen(1))
			mainCont := found.Spec.Template.Spec.Containers[0]

			By("Checking the main container Args in the Backstage Deployment", func() {
				Expect(mainCont.Args).To(HaveLen(4))
				Expect(mainCont.Args[0]).To(Equal("--config"))
				Expect(mainCont.Args[1]).To(Equal("dynamic-plugins-root/app-config.dynamic-plugins.yaml"))
				Expect(mainCont.Args[2]).To(Equal("--config"))
				Expect(mainCont.Args[3]).To(Equal("/opt/app-root/src/app-config.backend-auth.default.yaml"))
			})

			By("Checking the main container Volume Mounts in the Backstage Deployment", func() {
				Expect(mainCont.VolumeMounts).To(HaveLen(2))

				dpRoot := findVolumeMounts(mainCont.VolumeMounts, "dynamic-plugins-root")
				Expect(dpRoot).To(HaveLen(1), "No volume mount found with name: dynamic-plugins-root")
				Expect(dpRoot[0].MountPath).To(Equal("/opt/app-root/src/dynamic-plugins-root"))
				Expect(dpRoot[0].SubPath).To(BeEmpty())

				bsAuth := findVolumeMounts(mainCont.VolumeMounts, backendAuthConfigName)
				Expect(bsAuth).To(HaveLen(1), "No volume mount found with name: %s", backendAuthConfigName)
				Expect(bsAuth[0].MountPath).To(Equal("/opt/app-root/src/app-config.backend-auth.default.yaml"))
				Expect(bsAuth[0].SubPath).To(Equal("app-config.backend-auth.default.yaml"))
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
		When("referencing non-existing ConfigMap as app-config", func() {
			var backstage *bsv1alpha1.Backstage

			BeforeEach(func() {
				backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
					Application: &bsv1alpha1.Application{
						AppConfig: &bsv1alpha1.AppConfig{
							ConfigMaps: []bsv1alpha1.ObjectKeyRef{
								{Name: "a-non-existing-cm"},
							},
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

		for _, mountPath := range []string{"", "/some/path/for/app-config"} {
			mountPath := mountPath
			for _, key := range []string{"", "my-app-config-12.yaml"} {
				key := key
				When(fmt.Sprintf("referencing ConfigMaps for app-configs (mountPath=%q, key=%q) and dynamic plugins config ConfigMap", mountPath, key),
					func() {
						const (
							appConfig1CmName         = "my-app-config-1-cm"
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

							dynamicPluginsCm := buildConfigMap(dynamicPluginsConfigName, map[string]string{
								"dynamic-plugins.yaml": `
# dynamic-plugins.yaml (configmap)
includes: [dynamic-plugins.default.yaml]
plugins: []
`,
							})
							err = k8sClient.Create(ctx, dynamicPluginsCm)
							Expect(err).To(Not(HaveOccurred()))

							backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
								Application: &bsv1alpha1.Application{
									AppConfig: &bsv1alpha1.AppConfig{
										MountPath: mountPath,
										ConfigMaps: []bsv1alpha1.ObjectKeyRef{
											{
												Name: appConfig1CmName,
												Key:  key,
											},
										},
									},
									DynamicPluginsConfigMapName: dynamicPluginsConfigName,
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
								Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(4))

								_, ok := findVolume(found.Spec.Template.Spec.Volumes, "dynamic-plugins-root")
								Expect(ok).To(BeTrue(), "No volume found with name: dynamic-plugins-root")

								_, ok = findVolume(found.Spec.Template.Spec.Volumes, "dynamic-plugins-npmrc")
								Expect(ok).To(BeTrue(), "No volume found with name: dynamic-plugins-npmrc")

								appConfig1CmVol, ok := findVolume(found.Spec.Template.Spec.Volumes, appConfig1CmName)
								Expect(ok).To(BeTrue(), "No volume found with name: %s", appConfig1CmName)
								Expect(appConfig1CmVol.VolumeSource.Secret).To(BeNil())
								Expect(appConfig1CmVol.VolumeSource.ConfigMap.DefaultMode).To(HaveValue(Equal(int32(420))))
								Expect(appConfig1CmVol.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(appConfig1CmName))

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

								dpRoot := findVolumeMounts(initCont.VolumeMounts, "dynamic-plugins-root")
								Expect(dpRoot).To(HaveLen(1),
									"No volume mount found with name: dynamic-plugins-root")
								Expect(dpRoot[0].MountPath).To(Equal("/dynamic-plugins-root"))
								Expect(dpRoot[0].ReadOnly).To(BeFalse())
								Expect(dpRoot[0].SubPath).To(BeEmpty())

								dpNpmrc := findVolumeMounts(initCont.VolumeMounts, "dynamic-plugins-npmrc")
								Expect(dpNpmrc).To(HaveLen(1),
									"No volume mount found with name: dynamic-plugins-npmrc")
								Expect(dpNpmrc[0].MountPath).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins"))
								Expect(dpNpmrc[0].ReadOnly).To(BeTrue())
								Expect(dpNpmrc[0].SubPath).To(Equal(".npmrc"))

								dp := findVolumeMounts(initCont.VolumeMounts, dynamicPluginsConfigName)
								Expect(dp).To(HaveLen(1), "No volume mount found with name: %s", dynamicPluginsConfigName)
								Expect(dp[0].MountPath).To(Equal("/opt/app-root/src/dynamic-plugins.yaml"))
								Expect(dp[0].SubPath).To(Equal("dynamic-plugins.yaml"))
								Expect(dp[0].ReadOnly).To(BeTrue())
							})

							By("Checking the Number of main containers in the Backstage Deployment")
							Expect(found.Spec.Template.Spec.Containers).To(HaveLen(1))
							mainCont := found.Spec.Template.Spec.Containers[0]

							expectedMountPath := mountPath
							if expectedMountPath == "" {
								expectedMountPath = "/opt/app-root/src"
							}

							By("Checking the main container Args in the Backstage Deployment", func() {
								nbArgs := 6
								if key != "" {
									nbArgs = 4
								}
								Expect(mainCont.Args).To(HaveLen(nbArgs))
								Expect(mainCont.Args[1]).To(Equal("dynamic-plugins-root/app-config.dynamic-plugins.yaml"))
								for i := 0; i <= nbArgs-2; i += 2 {
									Expect(mainCont.Args[i]).To(Equal("--config"))
								}
								if key == "" {
									//TODO(rm3l): the order of the rest of the --config args should be the same as the order in
									// which the keys are listed in the ConfigMap/Secrets
									// But as this is returned as a map, Go does not provide any guarantee on the iteration order.
									Expect(mainCont.Args[3]).To(SatisfyAny(
										Equal(expectedMountPath+"/my-app-config-11.yaml"),
										Equal(expectedMountPath+"/my-app-config-12.yaml"),
									))
									Expect(mainCont.Args[5]).To(SatisfyAny(
										Equal(expectedMountPath+"/my-app-config-11.yaml"),
										Equal(expectedMountPath+"/my-app-config-12.yaml"),
									))
									Expect(mainCont.Args[3]).To(Not(Equal(mainCont.Args[5])))
								} else {
									Expect(mainCont.Args[3]).To(Equal(fmt.Sprintf("%s/%s", expectedMountPath, key)))
								}
							})

							By("Checking the main container Volume Mounts in the Backstage Deployment", func() {
								nbMounts := 3
								if key != "" {
									nbMounts = 2
								}
								Expect(mainCont.VolumeMounts).To(HaveLen(nbMounts))

								dpRoot := findVolumeMounts(mainCont.VolumeMounts, "dynamic-plugins-root")
								Expect(dpRoot).To(HaveLen(1), "No volume mount found with name: dynamic-plugins-root")
								Expect(dpRoot[0].MountPath).To(Equal("/opt/app-root/src/dynamic-plugins-root"))
								Expect(dpRoot[0].SubPath).To(BeEmpty())

								appConfig1CmMounts := findVolumeMounts(mainCont.VolumeMounts, appConfig1CmName)
								Expect(appConfig1CmMounts).To(HaveLen(nbMounts-1), "Wrong number of volume mounts found with name: %s", appConfig1CmName)
								if key != "" {
									Expect(appConfig1CmMounts).To(HaveLen(1), "Wrong number of volume mounts found with name: %s", appConfig1CmName)
									Expect(appConfig1CmMounts[0].MountPath).To(Equal(fmt.Sprintf("%s/%s", expectedMountPath, key)))
									Expect(appConfig1CmMounts[0].SubPath).To(Equal(key))
								} else {
									Expect(appConfig1CmMounts).To(HaveLen(2), "Wrong number of volume mounts found with name: %s", appConfig1CmName)
									Expect(appConfig1CmMounts[0].MountPath).ToNot(Equal(appConfig1CmMounts[1].MountPath))
									for i := 0; i <= 1; i++ {
										Expect(appConfig1CmMounts[i].MountPath).To(
											SatisfyAny(
												Equal(expectedMountPath+"/my-app-config-11.yaml"),
												Equal(expectedMountPath+"/my-app-config-12.yaml")))
										Expect(appConfig1CmMounts[i].SubPath).To(
											SatisfyAny(
												Equal("my-app-config-11.yaml"),
												Equal("my-app-config-12.yaml")))
									}
								}
							})

							By("Checking the latest Status added to the Backstage instance")
							verifyBackstageInstance(ctx)

						})
					})
			}
		}
	})

	Context("Extra Files", func() {
		for _, kind := range []string{"ConfigMap", "Secret"} {
			kind := kind
			When(fmt.Sprintf("referencing non-existing %s as extra-file", kind), func() {
				var backstage *bsv1alpha1.Backstage

				BeforeEach(func() {
					var (
						cmExtraFiles  []bsv1alpha1.ObjectKeyRef
						secExtraFiles []bsv1alpha1.ObjectKeyRef
					)
					name := "a-non-existing-" + strings.ToLower(kind)
					switch kind {
					case "ConfigMap":
						cmExtraFiles = append(cmExtraFiles, bsv1alpha1.ObjectKeyRef{Name: name})
					case "Secret":
						secExtraFiles = append(secExtraFiles, bsv1alpha1.ObjectKeyRef{Name: name})
					}
					backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
						Application: &bsv1alpha1.Application{
							ExtraFiles: &bsv1alpha1.ExtraFiles{
								ConfigMaps: cmExtraFiles,
								Secrets:    secExtraFiles,
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

		for _, mountPath := range []string{"", "/some/path/for/extra/config"} {
			mountPath := mountPath
			When("referencing ConfigMaps and Secrets for extra files - mountPath="+mountPath, func() {
				const (
					extraConfig1CmNameAll        = "my-extra-config-1-cm-all"
					extraConfig2SecretNameAll    = "my-extra-config-2-secret-all"
					extraConfig1CmNameSingle     = "my-extra-config-1-cm-single"
					extraConfig2SecretNameSingle = "my-extra-config-2-secret-single"
				)

				var backstage *bsv1alpha1.Backstage

				BeforeEach(func() {
					extraConfig1CmAll := buildConfigMap(extraConfig1CmNameAll, map[string]string{
						"my-extra-config-11.yaml": `
# my-extra-config-11.yaml
`,
						"my-extra-config-12.yaml": `
# my-extra-config-12.yaml
`,
					})
					err := k8sClient.Create(ctx, extraConfig1CmAll)
					Expect(err).To(Not(HaveOccurred()))

					extraConfig2SecretAll := buildSecret(extraConfig2SecretNameAll, map[string][]byte{
						"my-extra-config-21.yaml": []byte(`
# my-extra-config-21.yaml
`),
						"my-extra-config-22.yaml": []byte(`
# my-extra-config-22.yaml
`),
					})
					err = k8sClient.Create(ctx, extraConfig2SecretAll)
					Expect(err).To(Not(HaveOccurred()))

					extraConfig1CmSingle := buildConfigMap(extraConfig1CmNameSingle, map[string]string{
						"my-extra-file-11-single.yaml": `
# my-extra-file-11-single.yaml
`,
						"my-extra-file-12-single.yaml": `
# my-extra-file-12-single.yaml
`,
					})
					err = k8sClient.Create(ctx, extraConfig1CmSingle)
					Expect(err).To(Not(HaveOccurred()))

					extraConfig2SecretSingle := buildSecret(extraConfig2SecretNameSingle, map[string][]byte{
						"my-extra-file-21-single.yaml": []byte(`
# my-extra-file-21-single.yaml
`),
						"my-extra-file-22-single.yaml": []byte(`
# my-extra-file-22-single.yaml
`),
					})
					err = k8sClient.Create(ctx, extraConfig2SecretSingle)
					Expect(err).To(Not(HaveOccurred()))

					backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
						Application: &bsv1alpha1.Application{
							ExtraFiles: &bsv1alpha1.ExtraFiles{
								MountPath: mountPath,
								ConfigMaps: []bsv1alpha1.ObjectKeyRef{
									{Name: extraConfig1CmNameAll},
									{Name: extraConfig1CmNameSingle, Key: "my-extra-file-12-single.yaml"},
								},
								Secrets: []bsv1alpha1.ObjectKeyRef{
									{Name: extraConfig2SecretNameAll},
									{Name: extraConfig2SecretNameSingle, Key: "my-extra-file-22-single.yaml"},
								},
							},
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
						Expect(found.Spec.Template.Spec.Volumes).To(HaveLen(7))

						extraConfig1CmVol, ok := findVolume(found.Spec.Template.Spec.Volumes, extraConfig1CmNameAll)
						Expect(ok).To(BeTrue(), "No volume found with name: %s", extraConfig1CmNameAll)
						Expect(extraConfig1CmVol.VolumeSource.Secret).To(BeNil())
						Expect(extraConfig1CmVol.VolumeSource.ConfigMap.DefaultMode).To(HaveValue(Equal(int32(420))))
						Expect(extraConfig1CmVol.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(extraConfig1CmNameAll))

						extraConfig2SecretVol, ok := findVolume(found.Spec.Template.Spec.Volumes, extraConfig2SecretNameAll)
						Expect(ok).To(BeTrue(), "No volume found with name: %s", extraConfig2SecretNameAll)
						Expect(extraConfig2SecretVol.VolumeSource.ConfigMap).To(BeNil())
						Expect(extraConfig2SecretVol.VolumeSource.Secret.DefaultMode).To(HaveValue(Equal(int32(420))))
						Expect(extraConfig2SecretVol.VolumeSource.Secret.SecretName).To(Equal(extraConfig2SecretNameAll))

						extraConfig1SingleCmVol, ok := findVolume(found.Spec.Template.Spec.Volumes, extraConfig1CmNameSingle)
						Expect(ok).To(BeTrue(), "No volume found with name: %s", extraConfig1CmNameSingle)
						Expect(extraConfig1SingleCmVol.VolumeSource.Secret).To(BeNil())
						Expect(extraConfig1SingleCmVol.VolumeSource.ConfigMap.DefaultMode).To(HaveValue(Equal(int32(420))))
						Expect(extraConfig1SingleCmVol.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(extraConfig1CmNameSingle))

						extraConfig2SingleSecretVol, ok := findVolume(found.Spec.Template.Spec.Volumes, extraConfig2SecretNameSingle)
						Expect(ok).To(BeTrue(), "No volume found with name: %s", extraConfig2SecretNameSingle)
						Expect(extraConfig2SingleSecretVol.VolumeSource.ConfigMap).To(BeNil())
						Expect(extraConfig2SingleSecretVol.VolumeSource.Secret.DefaultMode).To(HaveValue(Equal(int32(420))))
						Expect(extraConfig2SingleSecretVol.VolumeSource.Secret.SecretName).To(Equal(extraConfig2SecretNameSingle))
					})

					initCont := found.Spec.Template.Spec.InitContainers[0]
					By("Checking the Init Container Volume Mounts in the Backstage Deployment", func() {
						Expect(initCont.VolumeMounts).To(HaveLen(3))

						// Extra config mounted in the main container
						Expect(findVolumeMounts(initCont.VolumeMounts, extraConfig1CmNameAll)).Should(HaveLen(0))
						Expect(findVolumeMounts(initCont.VolumeMounts, extraConfig2SecretNameAll)).Should(HaveLen(0))
					})

					mainCont := found.Spec.Template.Spec.Containers[0]

					By("Checking the main container Volume Mounts in the Backstage Deployment", func() {
						Expect(mainCont.VolumeMounts).To(HaveLen(7))

						expectedMountPath := mountPath
						if expectedMountPath == "" {
							expectedMountPath = "/opt/app-root/src"
						}

						extraConfig1CmMounts := findVolumeMounts(mainCont.VolumeMounts, extraConfig1CmNameAll)
						Expect(extraConfig1CmMounts).To(HaveLen(2), "No volume mounts found with name: %s", extraConfig1CmNameAll)
						Expect(extraConfig1CmMounts[0].MountPath).ToNot(Equal(extraConfig1CmMounts[1].MountPath))
						for i := 0; i <= 1; i++ {
							Expect(extraConfig1CmMounts[i].MountPath).To(
								SatisfyAny(
									Equal(expectedMountPath+"/my-extra-config-11.yaml"),
									Equal(expectedMountPath+"/my-extra-config-12.yaml")))
							Expect(extraConfig1CmMounts[i].SubPath).To(
								SatisfyAny(
									Equal("my-extra-config-11.yaml"),
									Equal("my-extra-config-12.yaml")))
						}

						extraConfig2SecretMounts := findVolumeMounts(mainCont.VolumeMounts, extraConfig2SecretNameAll)
						Expect(extraConfig2SecretMounts).To(HaveLen(2), "No volume mounts found with name: %s", extraConfig2SecretNameAll)
						Expect(extraConfig2SecretMounts[0].MountPath).ToNot(Equal(extraConfig2SecretMounts[1].MountPath))
						for i := 0; i <= 1; i++ {
							Expect(extraConfig2SecretMounts[i].MountPath).To(
								SatisfyAny(
									Equal(expectedMountPath+"/my-extra-config-21.yaml"),
									Equal(expectedMountPath+"/my-extra-config-22.yaml")))
							Expect(extraConfig2SecretMounts[i].SubPath).To(
								SatisfyAny(
									Equal("my-extra-config-21.yaml"),
									Equal("my-extra-config-22.yaml")))
						}

						extraConfig1CmSingleMounts := findVolumeMounts(mainCont.VolumeMounts, extraConfig1CmNameSingle)
						Expect(extraConfig1CmSingleMounts).To(HaveLen(1), "No volume mounts found with name: %s", extraConfig1CmNameSingle)
						Expect(extraConfig1CmSingleMounts[0].MountPath).To(Equal(expectedMountPath + "/my-extra-file-12-single.yaml"))
						Expect(extraConfig1CmSingleMounts[0].SubPath).To(Equal("my-extra-file-12-single.yaml"))

						extraConfig2SecretSingleMounts := findVolumeMounts(mainCont.VolumeMounts, extraConfig2SecretNameSingle)
						Expect(extraConfig2SecretSingleMounts).To(HaveLen(1), "No volume mounts found with name: %s", extraConfig2SecretNameSingle)
						Expect(extraConfig2SecretSingleMounts[0].MountPath).To(Equal(expectedMountPath + "/my-extra-file-22-single.yaml"))
						Expect(extraConfig2SecretSingleMounts[0].SubPath).To(Equal("my-extra-file-22-single.yaml"))
					})

					By("Checking the latest Status added to the Backstage instance")
					verifyBackstageInstance(ctx)
				})
			})
		}
	})

	Context("Extra Env Vars", func() {
		When("setting environment variables either directly or via references to ConfigMap or Secret", func() {
			const (
				envConfig1CmNameAll        = "my-env-config-1-cm-all"
				envConfig2SecretNameAll    = "my-env-config-2-secret-all"
				envConfig1CmNameSingle     = "my-env-config-1-cm-single"
				envConfig2SecretNameSingle = "my-env-config-2-secret-single"
			)

			var backstage *bsv1alpha1.Backstage

			BeforeEach(func() {
				envConfig1Cm := buildConfigMap(envConfig1CmNameAll, map[string]string{
					"MY_ENV_VAR_1_FROM_CM": "value 11",
					"MY_ENV_VAR_2_FROM_CM": "value 12",
				})
				err := k8sClient.Create(ctx, envConfig1Cm)
				Expect(err).To(Not(HaveOccurred()))

				envConfig2Secret := buildSecret(envConfig2SecretNameAll, map[string][]byte{
					"MY_ENV_VAR_1_FROM_SECRET": []byte("value 21"),
					"MY_ENV_VAR_2_FROM_SECRET": []byte("value 22"),
				})
				err = k8sClient.Create(ctx, envConfig2Secret)
				Expect(err).To(Not(HaveOccurred()))

				envConfig1CmSingle := buildConfigMap(envConfig1CmNameSingle, map[string]string{
					"MY_ENV_VAR_1_FROM_CM_SINGLE": "value 11 single",
					"MY_ENV_VAR_2_FROM_CM_SINGLE": "value 12 single",
				})
				err = k8sClient.Create(ctx, envConfig1CmSingle)
				Expect(err).To(Not(HaveOccurred()))

				envConfig2SecretSingle := buildSecret(envConfig2SecretNameSingle, map[string][]byte{
					"MY_ENV_VAR_1_FROM_SECRET_SINGLE": []byte("value 21 single"),
					"MY_ENV_VAR_2_FROM_SECRET_SINGLE": []byte("value 22 single"),
				})
				err = k8sClient.Create(ctx, envConfig2SecretSingle)
				Expect(err).To(Not(HaveOccurred()))

				backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
					Application: &bsv1alpha1.Application{
						ExtraEnvs: &bsv1alpha1.ExtraEnvs{
							Envs: []bsv1alpha1.Env{
								{Name: "MY_ENV_VAR_1", Value: "value 10"},
								{Name: "MY_ENV_VAR_2", Value: "value 20"},
							},
							ConfigMaps: []bsv1alpha1.ObjectKeyRef{
								{Name: envConfig1CmNameAll},
								{Name: envConfig1CmNameSingle, Key: "MY_ENV_VAR_2_FROM_CM_SINGLE"},
							},
							Secrets: []bsv1alpha1.ObjectKeyRef{
								{Name: envConfig2SecretNameAll},
								{Name: envConfig2SecretNameSingle, Key: "MY_ENV_VAR_2_FROM_SECRET_SINGLE"},
							},
						},
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

				mainCont := found.Spec.Template.Spec.Containers[0]
				By(fmt.Sprintf("Checking Env in the Backstage Deployment - container: %q", mainCont.Name), func() {
					Expect(len(mainCont.Env)).To(BeNumerically(">=", 4),
						"Expected at least 4 items in Env for container %q, fot %d", mainCont.Name, len(mainCont.Env))

					envVar, ok := findEnvVar(mainCont.Env, "MY_ENV_VAR_1")
					Expect(ok).To(BeTrue(), "No env var with name MY_ENV_VAR_1 in main container")
					Expect(envVar.Value).Should(Equal("value 10"))

					envVar, ok = findEnvVar(mainCont.Env, "MY_ENV_VAR_2")
					Expect(ok).To(BeTrue(), "No env var with name MY_ENV_VAR_2 in main container")
					Expect(envVar.Value).Should(Equal("value 20"))

					envVar, ok = findEnvVar(mainCont.Env, "MY_ENV_VAR_2_FROM_CM_SINGLE")
					Expect(ok).To(BeTrue(), "No env var with name MY_ENV_VAR_2_FROM_CM_SINGLE in main container")
					Expect(envVar.Value).Should(BeEmpty())
					Expect(envVar.ValueFrom).ShouldNot(BeNil())
					Expect(envVar.ValueFrom.FieldRef).Should(BeNil())
					Expect(envVar.ValueFrom.ResourceFieldRef).Should(BeNil())
					Expect(envVar.ValueFrom.SecretKeyRef).Should(BeNil())
					Expect(envVar.ValueFrom.ConfigMapKeyRef).ShouldNot(BeNil())
					Expect(envVar.ValueFrom.ConfigMapKeyRef.Key).Should(Equal("MY_ENV_VAR_2_FROM_CM_SINGLE"))
					Expect(envVar.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name).Should(Equal(envConfig1CmNameSingle))

					envVar, ok = findEnvVar(mainCont.Env, "MY_ENV_VAR_2_FROM_SECRET_SINGLE")
					Expect(ok).To(BeTrue(), "No env var with name MY_ENV_VAR_2_FROM_SECRET_SINGLE in main container")
					Expect(envVar.Value).Should(BeEmpty())
					Expect(envVar.ValueFrom).ShouldNot(BeNil())
					Expect(envVar.ValueFrom.FieldRef).Should(BeNil())
					Expect(envVar.ValueFrom.ResourceFieldRef).Should(BeNil())
					Expect(envVar.ValueFrom.ConfigMapKeyRef).Should(BeNil())
					Expect(envVar.ValueFrom.SecretKeyRef).ShouldNot(BeNil())
					Expect(envVar.ValueFrom.SecretKeyRef.Key).Should(Equal("MY_ENV_VAR_2_FROM_SECRET_SINGLE"))
					Expect(envVar.ValueFrom.SecretKeyRef.LocalObjectReference.Name).Should(Equal(envConfig2SecretNameSingle))
				})
				By(fmt.Sprintf("Checking EnvFrom in the Backstage Deployment - container: %q", mainCont.Name), func() {
					Expect(len(mainCont.EnvFrom)).To(BeNumerically(">=", 2),
						"Expected at least 2 items in EnvFrom for container %q, fot %d", mainCont.Name, len(mainCont.EnvFrom))

					envVar, ok := findEnvVarFrom(mainCont.EnvFrom, envConfig1CmNameAll)
					Expect(ok).To(BeTrue(), "No ConfigMap-backed envFrom in main container: %s", envConfig1CmNameAll)
					Expect(envVar.SecretRef).Should(BeNil())
					Expect(envVar.ConfigMapRef).ShouldNot(BeNil())

					envVar, ok = findEnvVarFrom(mainCont.EnvFrom, envConfig2SecretNameAll)
					Expect(ok).To(BeTrue(), "No Secret-backed envFrom in main container: %s", envConfig2SecretNameAll)
					Expect(envVar.ConfigMapRef).Should(BeNil())
					Expect(envVar.SecretRef).ShouldNot(BeNil())
				})

				initCont := found.Spec.Template.Spec.InitContainers[0]
				By("not injecting Env set in CR into the Backstage Deployment Init Container", func() {
					_, ok := findEnvVar(initCont.Env, "MY_ENV_VAR_1")
					Expect(ok).To(BeFalse(), "Env var with name MY_ENV_VAR_1 should not be injected into init container")
					_, ok = findEnvVar(initCont.Env, "MY_ENV_VAR_2")
					Expect(ok).To(BeFalse(), "Env var with name MY_ENV_VAR_2 should not be injected into  init container")
					_, ok = findEnvVar(initCont.Env, "MY_ENV_VAR_2_FROM_CM_SINGLE")
					Expect(ok).To(BeFalse(), "Env var with name MY_ENV_VAR_2_FROM_CM_SINGLE should not be injected into  init container")
					_, ok = findEnvVar(initCont.Env, "MY_ENV_VAR_2_FROM_SECRET_SINGLE")
					Expect(ok).To(BeFalse(), "Env var with name MY_ENV_VAR_2_FROM_SECRET_SINGLE should not be injected into  init container")
				})
				By("not injecting EnvFrom set in CR into the Backstage Deployment Init Container", func() {
					_, ok := findEnvVarFrom(initCont.EnvFrom, envConfig1CmNameAll)
					Expect(ok).To(BeFalse(), "ConfigMap-backed envFrom should not be added to init container: %s", envConfig1CmNameAll)
					_, ok = findEnvVarFrom(initCont.EnvFrom, envConfig2SecretNameAll)
					Expect(ok).To(BeFalse(), "Secret-backed envFrom should not be added to init container: %s", envConfig2SecretNameAll)
				})

				By("Checking the latest Status added to the Backstage instance")
				verifyBackstageInstance(ctx)
			})
		})
	})

	When("setting image", func() {
		var imageName = "quay.io/my-org/my-awesome-image:1.2.3"

		var backstage *bsv1alpha1.Backstage

		BeforeEach(func() {
			backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
				Application: &bsv1alpha1.Application{
					Image: &imageName,
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

			By("Checking that the Deployment was successfully created in the reconciliation")
			found := &appsv1.Deployment{}
			Eventually(func(g Gomega) {
				// TODO to get name from default
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, found)
				g.Expect(err).To(Not(HaveOccurred()))
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking that the image was set on all containers in the Pod Spec")
			visitContainers(&found.Spec.Template, func(container *corev1.Container) {
				By(fmt.Sprintf("Checking Image in the Backstage Deployment - container: %q", container.Name), func() {
					Expect(container.Image).Should(Equal(imageName))
				})
			})

			By("Checking the latest Status added to the Backstage instance")
			verifyBackstageInstance(ctx)
		})
	})

	When("setting image pull secrets", func() {
		const (
			ips1 = "some-image-pull-secret-1"
			ips2 = "some-image-pull-secret-2"
		)

		var backstage *bsv1alpha1.Backstage

		BeforeEach(func() {
			backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
				Application: &bsv1alpha1.Application{
					ImagePullSecrets: []string{ips1, ips2},
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

			By("Checking that the Deployment was successfully created in the reconciliation")
			found := &appsv1.Deployment{}
			Eventually(func(g Gomega) {
				// TODO to get name from default
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, found)
				g.Expect(err).To(Not(HaveOccurred()))
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking the image pull secrets are included in the pod spec of Backstage", func() {
				var list []string
				for _, v := range found.Spec.Template.Spec.ImagePullSecrets {
					list = append(list, v.Name)
				}
				Expect(list).Should(HaveExactElements(ips1, ips2))
			})

			By("Checking the latest Status added to the Backstage instance")
			verifyBackstageInstance(ctx)
		})
	})

	When("setting the number of replicas", func() {
		var nbReplicas int32 = 5

		var backstage *bsv1alpha1.Backstage

		BeforeEach(func() {
			backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
				Application: &bsv1alpha1.Application{
					Replicas: &nbReplicas,
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

			By("Checking that the Deployment was successfully created in the reconciliation")
			found := &appsv1.Deployment{}
			Eventually(func(g Gomega) {
				// TODO to get name from default
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, found)
				g.Expect(err).To(Not(HaveOccurred()))
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking the number of replicas of the Backstage Instance")
			Expect(found.Spec.Replicas).Should(HaveValue(BeEquivalentTo(nbReplicas)))

			By("Checking the latest Status added to the Backstage instance")
			verifyBackstageInstance(ctx)
		})
	})

	Context("PostgreSQL", func() {
		// Other cases covered in the tests above

		When("disabling PostgreSQL in the CR", func() {
			var backstage *bsv1alpha1.Backstage
			BeforeEach(func() {
				backstage = buildBackstageCR(bsv1alpha1.BackstageSpec{
					EnableLocalDb: pointer.Bool(false),
				})
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

				By("not creating a StatefulSet for the Database")
				Consistently(func(g Gomega) {
					err := k8sClient.Get(ctx,
						types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstage.Name)},
						&appsv1.StatefulSet{})
					g.Expect(err).Should(HaveOccurred())
					g.Expect(errors.IsNotFound(err)).Should(BeTrue(), "Expected error to be a not-found one, but got %v", err)
				}, 10*time.Second, time.Second).Should(Succeed())

				By("Checking if Deployment was successfully created in the reconciliation")
				Eventually(func() error {
					// TODO to get name from default
					return k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "backstage"}, &appsv1.Deployment{})
				}, time.Minute, time.Second).Should(Succeed())

				By("Checking the latest Status added to the Backstage instance")
				verifyBackstageInstance(ctx)
			})
		})
	})
})

func findElementsByPredicate[T any](l []T, predicate func(t T) bool) (result []T) {
	for _, v := range l {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}
