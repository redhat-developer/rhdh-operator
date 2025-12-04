package integration_tests

import (
	"context"
	"fmt"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	appsv1 "k8s.io/api/apps/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha5"

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
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			Expect(deploy.SpecReplicas()).To(HaveValue(BeEquivalentTo(1)))

			By("creating OwnerReference to all the runtime objects")
			or := deploy.GetObject().GetOwnerReferences()
			g.Expect(or).To(HaveLen(1))
			g.Expect(or[0].Name).To(Equal(backstageName))

			By("creating default app-config")
			appConfig := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.AppConfigDefaultName(backstageName)}, appConfig)
			g.Expect(err).ShouldNot(HaveOccurred())

			By("mounting Volume defined in default app-config")
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(model.AppConfigDefaultName(backstageName))).
				To(BeAddedAsVolumeToPodSpec(*deploy.PodSpec()))

		}, 5*time.Minute, time.Second).Should(Succeed())

		if *testEnv.UseExistingCluster && useExistingController {
			By("setting Backstage status (real cluster only)")
			Eventually(func(g Gomega) {

				bs := &bsv1.Backstage{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: backstageName}, bs)
				g.Expect(err).ShouldNot(HaveOccurred())

				depl, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
				g.Expect(err).ShouldNot(HaveOccurred())

				// TODO better matcher for Conditions
				g.Expect(bs.Status.Conditions[0].Reason).To(Equal("Deployed"))

				g.Expect(depl).NotTo(BeNil())

				switch depl.GetObject().(type) {
				case *appsv1.StatefulSet:
					deploy := depl.GetObject().(*appsv1.StatefulSet)
					for _, cond := range deploy.Status.Conditions {
						if cond.Type == appsv1.StatefulSetConditionType("Ready") {
							g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
						}
					}
				case *appsv1.Deployment:
					deploy := depl.GetObject().(*appsv1.Deployment)
					for _, cond := range deploy.Status.Conditions {
						if cond.Type == appsv1.DeploymentAvailable {
							g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
						}
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
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy.SpecReplicas()).To(HaveValue(BeEquivalentTo(1)))
			g.Expect(deploy.PodSpec().Containers).To(HaveLen(1))

			g.Expect(backstageContainer(*deploy.PodSpec()).Image).To(Equal("busybox"))

			By("creating StatefulSet")
			ss := &appsv1.StatefulSet{}
			name := fmt.Sprintf("backstage-psql-%s", backstageName)
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, ss)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1))
			g.Expect(ss.Spec.Template.Spec.Containers[0].Image).To(Equal("busybox"))
		}, time.Minute, time.Second).Should(Succeed())

	})

	It("creates backstage and checks the status", func() {
		if !*testEnv.UseExistingCluster {
			Skip("Real cluster required")
		}
		if !useExistingController {
			Skip("Real controller required")
		}

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{}, "")

		Eventually(func(g Gomega) {
			bs := &bsv1.Backstage{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: backstageName}, bs)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(bs.Status.Conditions).To(HaveLen(1))
			g.Expect(bs.Status.Conditions[0].Reason).To(Equal("DeployInProgress"))
		}, time.Minute, time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			bs := &bsv1.Backstage{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: backstageName}, bs)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(bs.Status.Conditions).To(HaveLen(1))
			g.Expect(bs.Status.Conditions[0].Reason).To(Equal("Deployed"))
		}, 3*time.Minute, time.Second).Should(Succeed())

	})
})
