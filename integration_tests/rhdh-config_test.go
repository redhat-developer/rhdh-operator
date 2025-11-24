package integration_tests

import (
	"context"
	"fmt"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create default rhdh", func() {

	It("tests rhdh config", func() {

		if !isProfile("rhdh") {
			Skip("Skipped for non rhdh config")
		}

		ctx := context.Background()
		ns := createNamespace(ctx)
		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{}, "")

		Eventually(func(g Gomega) {
			//deploy := &appsv1.Deployment{}
			//err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			//bpod, err := getBackstagePod(ctx, k8sClient, ns, backstageName)
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred(), controllerMessage())

			ss := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DbStatefulSetName(backstageName)}, ss)
			g.Expect(err).ShouldNot(HaveOccurred(), controllerMessage())

			serv := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.ServiceName(backstageName)}, serv)
			g.Expect(err).ShouldNot(HaveOccurred(), controllerMessage())

			dpCm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DynamicPluginsDefaultName(backstageName)}, dpCm)
			g.Expect(err).ShouldNot(HaveOccurred())

			var appConfigCm corev1.ConfigMap
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.AppConfigDefaultName(backstageName)}, &appConfigCm)
			g.Expect(err).ShouldNot(HaveOccurred())

			g.Expect(deploy.PodSpec().InitContainers).To(HaveLen(1))
			_, initCont := model.DynamicPluginsInitContainer(deploy.PodSpec().InitContainers)

			initContainerExpectedVolumeMounts := []corev1.VolumeMount{
				{
					Name:      "dynamic-plugins-root",
					MountPath: "/dynamic-plugins-root",
					SubPath:   "",
				},
				{
					Name:      utils.GenerateRuntimeObjectName(backstageName, "backstage-files"),
					MountPath: "/opt/app-root/src/.npmrc.dynamic-plugins",
					SubPath:   "",
				},
				// TODO - configure this when have defined dynamic plugins OCI registry
				{
					Name:      "dynamic-plugins-registry-auth",
					MountPath: "/opt/app-root/src/.config/containers",
					SubPath:   "",
				},
				{
					Name:      "npmcacache",
					MountPath: "/opt/app-root/src/.npm/_cacache",
					SubPath:   "",
				},
				{
					Name:      utils.GenerateVolumeNameFromCmOrSecret(model.DynamicPluginsDefaultName(backstageName)),
					MountPath: "/opt/app-root/src/dynamic-plugins.yaml",
					SubPath:   "dynamic-plugins.yaml",
				},
				{
					Name:      "temp",
					MountPath: "/tmp",
					SubPath:   "",
				},
			}

			g.Expect(initCont.VolumeMounts).To(HaveLen(len(initContainerExpectedVolumeMounts)))

			for _, evm := range initContainerExpectedVolumeMounts {
				found := false
				for _, vm := range initCont.VolumeMounts {
					if vm.Name == evm.Name {
						found = true
						g.Expect(vm.MountPath).To(Equal(evm.MountPath))
						g.Expect(vm.SubPath).To(Equal(evm.SubPath))
					}
				}
				g.Expect(found).To(BeTrue())
			}

			g.Expect(initCont.Env[0].Name).To(Equal("NPM_CONFIG_USERCONFIG"))
			g.Expect(initCont.Env[0].Value).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins/.npmrc"))

			g.Expect(deploy.PodSpec().Volumes).To(HaveLen(7))
			g.Expect(deploy.PodSpec().Containers).To(HaveLen(1))
			mainCont := backstageContainer2(*deploy.PodSpec())
			g.Expect(mainCont.Args).To(HaveLen(4))
			g.Expect(mainCont.Args[0]).To(Equal("--config"))
			g.Expect(mainCont.Args[1]).To(Equal("dynamic-plugins-root/app-config.dynamic-plugins.yaml"))
			g.Expect(mainCont.Args[2]).To(Equal("--config"))
			g.Expect(mainCont.Args[3]).To(Equal("/opt/app-root/src/default.app-config.yaml"))

			mainContainerExpectedVolumeMounts := []corev1.VolumeMount{
				{
					MountPath: "/opt/app-root/src/dynamic-plugins-root",
					SubPath:   "",
				},
				{
					MountPath: "/opt/app-root/src/default.app-config.yaml",
					SubPath:   "default.app-config.yaml",
				},
				{
					MountPath: "/tmp",
					SubPath:   "",
				},
			}

			g.Expect(mainCont.VolumeMounts).To(HaveLen(len(mainContainerExpectedVolumeMounts)))

			for _, evm := range mainContainerExpectedVolumeMounts {
				found := false
				for _, vm := range mainCont.VolumeMounts {
					if evm.MountPath == vm.MountPath {
						found = true
						g.Expect(vm.MountPath).To(Equal(evm.MountPath))
						g.Expect(vm.SubPath).To(Equal(evm.SubPath))
					}
				}
				g.Expect(found).To(BeTrue())
			}

			if currentPlatform.IsOpenshift() {
				// no patch, so default
				By("not applying any platform-specific patches", func() {
					g.Expect(deploy.PodSpec().SecurityContext.FSGroup).To(BeNil())
					g.Expect(ss.Spec.Template.Spec.SecurityContext.FSGroup).To(BeNil())
					g.Expect(serv.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				})

				By("updating the baseUrls in the default app-config CM (RHIDP-6192)", func() {
					g.Expect(appConfigCm).To(
						HaveAppConfigBaseUrl(MatchRegexp(fmt.Sprintf(`^https://%s-%s.+`, model.RouteName(backstageName), ns))))
				})
			} else {
				// k8s (patched)
				By("applying k8s-specific patches for security context", func() {
					g.Expect(deploy.PodSpec().SecurityContext.FSGroup).To(Not(BeNil()))
					g.Expect(ss.Spec.Template.Spec.SecurityContext.FSGroup).To(Not(BeNil()))
					g.Expect(serv.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
				})

				By("not updating the baseUrls in the default app-config CM", func() {
					g.Expect(appConfigCm).To(HaveAppConfigBaseUrl(BeEmpty()))
				})
			}

		}, 20*time.Second, time.Second).Should(Succeed())

		deleteNamespace(ctx, ns)
	})

	It("replaces dynamic-plugins-root volume", func() {

		// This test relies on the fact that RHDH default config for deployment contains
		//       volumes:
		//        - ephemeral:
		//          name: dynamic-plugins-root
		// and check if it replaced with one defined in spec.deployment

		ctx := context.Background()
		ns := createNamespace(ctx)
		bs2 := &bsv1.Backstage{}

		err := readYamlFile("testdata/rhdh-replace-dynaplugin-root.yaml", bs2)
		Expect(err).To(Not(HaveOccurred()))

		backstageName := createAndReconcileBackstage(ctx, ns, bs2.Spec, "")

		Eventually(func(g Gomega) {
			By("getting the Pod ")
			//deploy := &appsv1.Deployment{}
			//err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			//bpod, err := getBackstagePod(ctx, k8sClient, ns, backstageName)
			g.Expect(err).To(Not(HaveOccurred()))

			var bsvolume *corev1.Volume
			for _, v := range deploy.PodSpec().Volumes {

				if v.Name == "dynamic-plugins-root" {
					bsvolume = &v
					break
				}
			}

			g.Expect(bsvolume).NotTo(BeNil())
			g.Expect(bsvolume.Ephemeral).To(BeNil())
			g.Expect(bsvolume.PersistentVolumeClaim).NotTo(BeNil())

		}, 10*time.Second, time.Second).Should(Succeed())

	})

	It("replaces .npmrc", func() {

		// This test relies on the fact that RHDH default config contains secret-files.yaml for .nmrc with:
		//  name: dynamic-plugins-npmrc
		//  annotations:
		//    rhdh.redhat.com/mount-path: /opt/app-root/src/.npmrc.dynamic-plugins
		//    rhdh.redhat.com/containers: install-dynamic-plugins
		// and check if it replaced with one defined in spec.deployment

		if !isProfile("rhdh") {
			Skip("Skipped for non rhdh config")
		}

		ctx := context.Background()
		ns := createNamespace(ctx)
		npmrcSecret := generateSecret(ctx, k8sClient, "my-dynamic-plugins-npmrc", ns, map[string]string{".npmrc": "new-npmrc"}, nil, nil)

		bsSpec := bsv1.BackstageSpec{
			Application: &bsv1.Application{
				ExtraFiles: &bsv1.ExtraFiles{
					Secrets: []bsv1.FileObjectRef{
						{
							Name:      npmrcSecret,
							MountPath: "/opt/app-root/src/.npmrc.dynamic-plugins",
							Containers: []string{
								"install-dynamic-plugins",
							},
						},
					},
				},
			},
		}

		backstageName := createAndReconcileBackstage(ctx, ns, bsSpec, "")

		Eventually(func(g Gomega) {
			By("getting the Deployment ")
			//deploy := &appsv1.Deployment{}
			//err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			//bpod, err := getBackstagePod(ctx, k8sClient, ns, backstageName)
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).To(Not(HaveOccurred()))

			g.Expect(len(deploy.PodSpec().InitContainers)).To(Equal(1))
			initCont := deploy.PodSpec().InitContainers[0]
			g.Expect(initCont.Name).To(Equal("install-dynamic-plugins"))
			found := 0
			for i := range initCont.VolumeMounts {
				if initCont.VolumeMounts[i].MountPath == "/opt/app-root/src/.npmrc.dynamic-plugins" {
					g.Expect(initCont.VolumeMounts[i].Name).To(Equal(npmrcSecret))
					found++
				}
			}
			g.Expect(found).To(Equal(1))

		}, 10*time.Second, time.Second).Should(Succeed())

	})
})
