package integration_tests

import (
	"context"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	appsv1 "k8s.io/api/apps/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("test plugin deps", func() {

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
		_ = os.Unsetenv("PLUGIN_DEPS_DIR_backstage")
	})

	It("creates plugin dependencies", func() {
		if !isProfile("rhdh") {
			Skip("Skipped for non rhdh config")
		}

		if useExistingController {
			Skip("Skipped for real controller")
		}

		err := os.Setenv("PLUGIN_DEPS_DIR_backstage", "testdata/plugin-deps1")
		Expect(err).NotTo(HaveOccurred())

		dynapluginCm := map[string]string{"dynamic-plugins.yaml": readTestYamlFile("raw-dynaplugins-with-deps.yaml")}

		bsRaw := generateConfigMap(ctx, k8sClient, "dynaplugins", ns, dynapluginCm, nil, nil)

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			RawRuntimeConfig: &bsv1.RuntimeConfig{
				BackstageConfigName: bsRaw,
			},
		}, "")

		Eventually(func(g Gomega) {

			deploy := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "test-dependency"}, cm)
			g.Expect(err).ShouldNot(HaveOccurred())

			sec := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "test-dependency2"}, sec)
			g.Expect(err).ShouldNot(HaveOccurred())

			// disabled
			cm = &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "test-dependency3"}, cm)
			g.Expect(err).Should(HaveOccurred())

			// not listed
			cm = &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "test-dependency4"}, cm)
			g.Expect(err).Should(HaveOccurred())

		}, 5*time.Minute, time.Second).Should(Succeed())

	})
})
