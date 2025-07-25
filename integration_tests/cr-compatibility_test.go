package integration_tests

import (
	"context"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	appsv1 "k8s.io/api/apps/v1"

	bsv1alpha3 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("testing API version compatibility", func() {

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

	It("verifies v1alpha3 and v1alpha4 API compatibility", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		if !useExistingController {
			Skip("Skipped for not real controller")
		}

		// test v1alpha3 backward compatibility
		By("creating a Backstage resource using v1alpha3 API")
		backstageNameV3 := generateRandName("bs-v1alpha3")

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
		backstageNameV4 := generateRandName("bs-v1alpha4")

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
