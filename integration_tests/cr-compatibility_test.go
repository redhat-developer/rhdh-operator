package integration_tests

import (
	"context"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	appsv1 "k8s.io/api/apps/v1"

	bsv1prev "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha5"

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

	It("verifies prev and current version API compatibility", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		if !useExistingController {
			Skip("Skipped for not real controller")
		}

		// test prev version backward compatibility
		By("creating a Backstage resource using v1alpha3 API")
		backstageNameV3 := generateRandName("bs-v1alpha3")

		// create ConfigMap for prev version test
		generateConfigMap(ctx, k8sClient, "default-app-config", ns,
			map[string]string{
				"app-config.yaml": `app:
					title: Test App v1alpha3
					baseUrl: http://localhost:3000`,
			}, nil, nil)

		bsV1Prev := &bsv1prev.Backstage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backstageNameV3,
				Namespace: ns,
			},
			Spec: bsv1prev.BackstageSpec{
				Application: &bsv1prev.Application{
					AppConfig: &bsv1prev.AppConfig{
						ConfigMaps: []bsv1prev.FileObjectRef{
							{Name: "default-app-config"},
						},
					},
				},
			},
		}

		err := k8sClient.Create(ctx, bsV1Prev)
		Expect(err).ShouldNot(HaveOccurred())

		By("verifying the operator reconciles the prev version resource")
		Eventually(func(g Gomega) {
			fetched := &bsv1prev.Backstage{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageNameV3, Namespace: ns}, fetched)
			g.Expect(err).ShouldNot(HaveOccurred())
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("verifying prev version deployment is created")
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: ns,
				Name:      model.DeploymentName(backstageNameV3),
			}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy.Spec.Replicas).ToNot(BeNil())
			g.Expect(*deploy.Spec.Replicas).To(Equal(int32(1)))
		}, 30*time.Second, 5*time.Second).Should(Succeed())

		// test current version compatibility
		By("creating a Backstage resource using current version API")
		backstageNameV4 := generateRandName("bs-current")

		// create ConfigMap for current version test
		generateConfigMap(ctx, k8sClient, "default-app-config-v4", ns,
			map[string]string{
				"app-config.yaml": `app:
					title: Test App current version
					baseUrl: http://localhost:3000`,
			}, nil, nil)

		bsV1Alpha5 := &bsv1.Backstage{
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

		err = k8sClient.Create(ctx, bsV1Alpha5)
		Expect(err).ShouldNot(HaveOccurred())

		By("verifying the operator reconciles the current version resource")
		Eventually(func(g Gomega) {
			fetched := &bsv1.Backstage{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageNameV4, Namespace: ns}, fetched)
			g.Expect(err).ShouldNot(HaveOccurred())
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("verifying current version deployment is created")
		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: ns,
				Name:      model.DeploymentName(backstageNameV4),
			}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy.Spec.Replicas).ToNot(BeNil())
			g.Expect(*deploy.Spec.Replicas).To(Equal(int32(1)))
		}, 30*time.Second, 5*time.Second).Should(Succeed())

		By("verifying both prev and current version resources coexist")
		// verify both deployments exist and have correct specs
		Eventually(func(g Gomega) {
			deployPrev := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: ns,
				Name:      model.DeploymentName(backstageNameV3),
			}, deployPrev)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deployPrev.Spec.Replicas).ToNot(BeNil())
			g.Expect(*deployPrev.Spec.Replicas).To(Equal(int32(1)))

			deployCurr := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Namespace: ns,
				Name:      model.DeploymentName(backstageNameV4),
			}, deployCurr)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deployCurr.Spec.Replicas).ToNot(BeNil())
			g.Expect(*deployCurr.Spec.Replicas).To(Equal(int32(1)))
		}, 30*time.Second, 5*time.Second).Should(Succeed())

		// clean up test resources
		By("cleaning up prev version test resource")
		err = k8sClient.Delete(ctx, bsV1Prev)
		Expect(err).ShouldNot(HaveOccurred())

		By("cleaning up current version test resource")
		err = k8sClient.Delete(ctx, bsV1Alpha5)
		Expect(err).ShouldNot(HaveOccurred())
	})
})
