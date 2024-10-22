package integration_tests

import (
	"context"
	"time"

	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha3"

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
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred(), controllerMessage())

			//By("creating /opt/app-root/src/dynamic-plugins.xml ")
			appConfig := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DynamicPluginsDefaultName(backstageName)}, appConfig)
			g.Expect(err).ShouldNot(HaveOccurred())

			g.Expect(deploy.Spec.Template.Spec.InitContainers).To(HaveLen(1))
			_, initCont := model.DynamicPluginsInitContainer(deploy.Spec.Template.Spec.InitContainers)
			//deploy.Spec.Template.Spec.InitContainers[0]
			g.Expect(initCont.VolumeMounts).To(HaveLen(5))
			g.Expect(initCont.VolumeMounts[0].MountPath).To(Equal("/dynamic-plugins-root"))
			g.Expect(initCont.VolumeMounts[0].SubPath).To(BeEmpty())
			g.Expect(initCont.VolumeMounts[1].MountPath).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins"))
			g.Expect(initCont.VolumeMounts[1].SubPath).To(Equal(".npmrc"))
			g.Expect(initCont.VolumeMounts[2].MountPath).To(Equal("/opt/app-root/src/.config/containers"))
			g.Expect(initCont.VolumeMounts[3].MountPath).To(Equal("/opt/app-root/src/.npm/_cacache"))
			g.Expect(initCont.VolumeMounts[3].SubPath).To(BeEmpty())
			g.Expect(initCont.VolumeMounts[4].MountPath).To(Equal("/opt/app-root/src/dynamic-plugins.yaml"))
			g.Expect(initCont.VolumeMounts[4].SubPath).To(Equal("dynamic-plugins.yaml"))
			g.Expect(initCont.VolumeMounts[4].Name).
				To(Equal(utils.GenerateVolumeNameFromCmOrSecret(model.DynamicPluginsDefaultName(backstageName))))
			g.Expect(initCont.VolumeMounts[4].SubPath).To(Equal(model.DynamicPluginsFile))

			g.Expect(initCont.Env[0].Name).To(Equal("NPM_CONFIG_USERCONFIG"))
			g.Expect(initCont.Env[0].Value).To(Equal("/opt/app-root/src/.npmrc.dynamic-plugins"))

			g.Expect(deploy.Spec.Template.Spec.Volumes).To(HaveLen(7))
			g.Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			mainCont := deploy.Spec.Template.Spec.Containers[0]
			g.Expect(mainCont.Args).To(HaveLen(4))
			g.Expect(mainCont.Args[0]).To(Equal("--config"))
			g.Expect(mainCont.Args[1]).To(Equal("dynamic-plugins-root/app-config.dynamic-plugins.yaml"))
			g.Expect(mainCont.Args[2]).To(Equal("--config"))
			g.Expect(mainCont.Args[3]).To(Equal("/opt/app-root/src/default.app-config.yaml"))

			g.Expect(mainCont.VolumeMounts).To(HaveLen(3))
			g.Expect(mainCont.VolumeMounts[0].MountPath).To(Equal("/opt/app-root/src/dynamic-plugins-root"))
			g.Expect(mainCont.VolumeMounts[0].SubPath).To(BeEmpty())
			g.Expect(mainCont.VolumeMounts[1].MountPath).To(Equal("/opt/app-root/src/default.app-config.yaml"))
			g.Expect(mainCont.VolumeMounts[1].SubPath).To(Equal("default.app-config.yaml"))

			g.Expect(mainCont.VolumeMounts[2].MountPath).To(Equal("/var/log/redhat-developer-hub/audit"))
			//g.Expect(mainCont.VolumeMounts[3].MountPath).To(Equal(fmt.Sprintf("/opt/app-root/src/backstage-%s-dynamic-plugins", backstageName)))

		}, 10*time.Second, time.Second).Should(Succeed())

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

		err := ReadYamlFile("testdata/rhdh-replace-dynaplugin-root.yaml", bs2)
		Expect(err).To(Not(HaveOccurred()))

		backstageName := createAndReconcileBackstage(ctx, ns, bs2.Spec, "")

		Eventually(func(g Gomega) {
			By("getting the Deployment ")
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).To(Not(HaveOccurred()))

			var bsvolume *corev1.Volume
			for _, v := range deploy.Spec.Template.Spec.Volumes {

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
})
