package integration_tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	bsv1alpha3 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	It("refreshes pod for mounts with subPath", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		if !useExistingController {
			Skip("Skipped for not real controller")
		}

		appConfig1 := "app-config1"
		secretEnv1 := "secret-env1"

		backstageName := generateRandName("")

		conf := `
organization:
  name: "my org"
`

		generateConfigMap(ctx, k8sClient, appConfig1, ns, map[string]string{"appconfig11": conf}, nil, nil)
		generateSecret(ctx, k8sClient, secretEnv1, ns, map[string]string{"sec11": "val11"}, nil, nil)

		bs := bsv1.BackstageSpec{
			Application: &bsv1.Application{
				AppConfig: &bsv1.AppConfig{
					MountPath: "/my/mount/path",
					ConfigMaps: []bsv1.FileObjectRef{
						{Name: appConfig1},
					},
				},
				ExtraEnvs: &bsv1.ExtraEnvs{
					Secrets: []bsv1.EnvObjectRef{
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
			out, _, err := executeRemoteCommand(ctx, ns, podName, backstageContainerName(deploy), "cat /my/mount/path/appconfig11")
			g.Expect(err).ShouldNot(HaveOccurred())
			out = strings.Replace(out, "\r", "", -1)
			g.Expect(out).To(Equal(conf))

			out, _, err = executeRemoteCommand(ctx, ns, podName, backstageContainerName(deploy), "echo $sec11")
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect("val11\r\n").To(Equal(out))

		}, 5*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

		cm := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: appConfig1}, cm)
		Expect(err).ShouldNot(HaveOccurred())

		// update appconfig11
		newData := `
organization:
  name: "another org"
`
		cm.Data = map[string]string{"appconfig11": newData}
		err = k8sClient.Update(ctx, cm)
		Expect(err).ShouldNot(HaveOccurred())

		sec := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretEnv1}, sec)
		Expect(err).ShouldNot(HaveOccurred())
		newEnv := "val22"
		sec.StringData = map[string]string{"sec11": newEnv}
		err = k8sClient.Update(ctx, sec)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: appConfig1}, cm)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(cm.Data["appconfig11"]).To(Equal(newData))

			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			// Pod replaced so have to re-ask
			podList := &corev1.PodList{}
			err = k8sClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{model.BackstageAppLabel: utils.BackstageAppLabelValue(backstageName)})
			g.Expect(err).ShouldNot(HaveOccurred())

			podName := podList.Items[0].Name
			out, _, err := executeRemoteCommand(ctx, ns, podName, backstageContainerName(deploy), "cat /my/mount/path/appconfig11")
			g.Expect(err).ShouldNot(HaveOccurred())
			// TODO nicer method to compare file content with added '\r'
			// NOTE: it does not work well on envtest, real controller needed
			g.Expect(strings.ReplaceAll(out, "\r", "")).To(Equal(newData))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretEnv1}, sec)
			g.Expect(err).ShouldNot(HaveOccurred())

			out2, _, err := executeRemoteCommand(ctx, ns, podName, backstageContainerName(deploy), "echo $sec11")
			g.Expect(err).ShouldNot(HaveOccurred())
			// NOTE: it does not work well on envtest, real controller needed
			g.Expect(fmt.Sprintf("%s%s", newEnv, "\r\n")).To(Equal(out2))

		}, 10*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

	})

	It("refreshes mounts without subPath", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		//if !useExistingController {
		//	Skip("Skipped for not real controller")
		//}

		appConfig1 := "app-config1"
		secretFile1 := "secret1"

		backstageName := generateRandName("")

		conf := `
organization:
  name: "my org"
`

		generateConfigMap(ctx, k8sClient, appConfig1, ns, map[string]string{"appconfig11": conf}, nil, nil)
		generateSecret(ctx, k8sClient, secretFile1, ns, map[string]string{"sec11": "val11"}, nil, nil)

		bs := bsv1.BackstageSpec{
			Application: &bsv1.Application{
				AppConfig: &bsv1.AppConfig{
					ConfigMaps: []bsv1.FileObjectRef{
						{Name: appConfig1, MountPath: "/my/appconfig"},
					},
				},
				ExtraFiles: &bsv1.ExtraFiles{
					MountPath: "/my",
					Secrets: []bsv1.FileObjectRef{
						{Name: secretFile1, MountPath: "secret"},
					},
				},
			},
		}

		createAndReconcileBackstage(ctx, ns, bs, backstageName)

		var podName string
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			podList := &corev1.PodList{}
			err = k8sClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{model.BackstageAppLabel: utils.BackstageAppLabelValue(backstageName)})
			g.Expect(err).ShouldNot(HaveOccurred())

			g.Expect(len(podList.Items)).To(Equal(1))
			podName = podList.Items[0].Name

			out, _, err := executeRemoteCommand(ctx, ns, podName, backstageContainerName(deploy), "cat /my/appconfig/appconfig11")
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(strings.ReplaceAll(out, "\r", "")).To(Equal(conf))

			_, _, err = executeRemoteCommand(ctx, ns, podName, backstageContainerName(deploy), "cat /my/secret/sec11")
			g.Expect(err).ShouldNot(HaveOccurred())

		}, 4*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

		cm := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: appConfig1}, cm)
		Expect(err).ShouldNot(HaveOccurred())
		// update appconfig11
		newData := `
organization:
  name: "another org"
`
		cm.Data = map[string]string{"appconfig11": newData}
		err = k8sClient.Update(ctx, cm)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {

			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			// no need to re-ask pod name, it is not recreated, just use what we've got
			out, _, err := executeRemoteCommand(ctx, ns, podName, backstageContainerName(deploy), "cat /my/appconfig/appconfig11")
			g.Expect(err).ShouldNot(HaveOccurred())
			// let's check, just in case (it is k8s job to refresh it :)
			g.Expect(strings.ReplaceAll(out, "\r", "")).To(Equal(newData))

		}, 4*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

	})

	It("verifies v1alpha3 and v1alpha4 API compatibility", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		if !useExistingController {
			Skip("Skipped for not real controller")
		}

		// test v1alpha3 backward compatibility
		By("creating a Backstage resource using v1alpha3 API")
		backstageNameV3 := generateRandName("bs-v1alpha3-")

		// create ConfigMap for v1alpha3 test
		generateConfigMap(ctx, k8sClient, "default-app-config", ns,
			map[string]string{
				"app-config.yaml": `app:
					title: Test App v1alpha3
					baseUrl: http://localhost:3000`,
			}, nil, nil)

		bsV1Alpha3 := &bsv1alpha3.Backstage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backstageNameV3,
				Namespace: ns,
			},
			Spec: bsv1alpha3.BackstageSpec{
				Application: &bsv1alpha3.Application{
					AppConfig: &bsv1alpha3.AppConfig{
						ConfigMaps: []bsv1alpha3.FileObjectRef{
							{Name: "default-app-config"},
						},
					},
				},
			},
		}

		err := k8sClient.Create(ctx, bsV1Alpha3)
		Expect(err).ShouldNot(HaveOccurred())

		By("verifying the operator reconciles the v1alpha3 resource")
		Eventually(func(g Gomega) {
			fetched := &bsv1alpha3.Backstage{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageNameV3, Namespace: ns}, fetched)
			g.Expect(err).ShouldNot(HaveOccurred())
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("verifying v1alpha3 deployment is created and running")
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: ns,
				Name:      model.DeploymentName(backstageNameV3),
			}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy.Status.ReadyReplicas).To(BeNumerically(">", 0))
		}, 3*time.Minute, 10*time.Second).Should(Succeed())

		// test v1alpha4 compatibility
		By("creating a Backstage resource using v1alpha4 API")
		backstageNameV4 := generateRandName("bs-v1alpha4-")

		// create ConfigMap for v1alpha4 test
		generateConfigMap(ctx, k8sClient, "default-app-config-v4", ns,
			map[string]string{
				"app-config.yaml": `app:
					title: Test App v1alpha4
					baseUrl: http://localhost:3000`,
			}, nil, nil)

		bsV1Alpha4 := &bsv1.Backstage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backstageNameV4,
				Namespace: ns,
			},
			Spec: bsv1.BackstageSpec{
				Application: &bsv1.Application{
					AppConfig: &bsv1.AppConfig{
						ConfigMaps: []bsv1.FileObjectRef{
							{Name: "default-app-config-v4"},
						},
					},
				},
			},
		}

		err = k8sClient.Create(ctx, bsV1Alpha4)
		Expect(err).ShouldNot(HaveOccurred())

		By("verifying the operator reconciles the v1alpha4 resource")
		Eventually(func(g Gomega) {
			fetched := &bsv1.Backstage{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageNameV4, Namespace: ns}, fetched)
			g.Expect(err).ShouldNot(HaveOccurred())
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("verifying v1alpha4 deployment is created and running")
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: ns,
				Name:      model.DeploymentName(backstageNameV4),
			}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy.Status.ReadyReplicas).To(BeNumerically(">", 0))
		}, 3*time.Minute, 10*time.Second).Should(Succeed())

		By("verifying both v1alpha3 and v1alpha4 resources coexist")
		// verify both deployments are running simultaneously
		Eventually(func(g Gomega) {
			deployV3 := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: ns,
				Name:      model.DeploymentName(backstageNameV3),
			}, deployV3)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deployV3.Status.ReadyReplicas).To(BeNumerically(">", 0))

			deployV4 := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Namespace: ns,
				Name:      model.DeploymentName(backstageNameV4),
			}, deployV4)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deployV4.Status.ReadyReplicas).To(BeNumerically(">", 0))
		}, 4*time.Minute, 15*time.Second).Should(Succeed())

		// clean up test resources
		By("cleaning up v1alpha3 test resource")
		err = k8sClient.Delete(ctx, bsV1Alpha3)
		Expect(err).ShouldNot(HaveOccurred())

		By("cleaning up v1alpha4 test resource")
		err = k8sClient.Delete(ctx, bsV1Alpha4)
		Expect(err).ShouldNot(HaveOccurred())
	})
})
