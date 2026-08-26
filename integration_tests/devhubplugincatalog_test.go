package integration_tests

import (
	"context"
	"os"
	"time"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/api/v1alpha5"
	"github.com/redhat-developer/rhdh-operator/internal/controller"
	"github.com/redhat-developer/rhdh-operator/pkg/catalog"
	"github.com/redhat-developer/rhdh-operator/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	defaultCatalogIndexImage = "oci://quay.io/rhdh/plugin-catalog-index:next"
)

func getCatalogIndexImage() string {
	if img := os.Getenv("CATALOG_INDEX_IMAGE"); img != "" {
		return img
	}
	return defaultCatalogIndexImage
}

var _ = When("create DevHubPluginCatalog", func() {

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

	It("fetches catalog and patches ConfigMap with dynamic plugins", func() {
		catalogName := "test-catalog-" + randString(5)
		configMapName := controller.DefaultConfigMapName

		// Create the default-config ConfigMap that will be patched
		defaultConfigCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: ns,
			},
			Data: map[string]string{
				"placeholder": "initial",
			},
		}
		err := k8sClient.Create(ctx, defaultConfigCM)
		Expect(err).ShouldNot(HaveOccurred())

		// Create DevHubPluginCatalog pointing to the catalog index
		dhpc := &api.DevHubPluginCatalog{
			ObjectMeta: metav1.ObjectMeta{
				Name:      catalogName,
				Namespace: ns,
			},
			Spec: api.DevHubPluginCatalogSpec{
				Source: api.CatalogSource{
					Ref: getCatalogIndexImage(),
				},
			},
		}

		err = k8sClient.Create(ctx, dhpc)
		Expect(err).ShouldNot(HaveOccurred())

		// Create and run the reconciler
		reconciler := &controller.DevHubPluginCatalogReconciler{
			Client:            k8sClient,
			Scheme:            k8sClient.Scheme(),
			OperatorNamespace: ns,
			Processor:         catalog.NewProcessor(),
		}

		// Reconcile - this should fetch the catalog and patch the ConfigMap
		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: catalogName},
		})
		Expect(err).ShouldNot(HaveOccurred())

		// Verify the ConfigMap was patched with dynamic plugins content
		Eventually(func(g Gomega) {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: ns}, cm)
			g.Expect(err).ShouldNot(HaveOccurred())

			// Should have dynamic-plugins.yaml with ConfigMap YAML content
			g.Expect(cm.Data).To(HaveKey("dynamic-plugins.yaml"))
			dpContent := cm.Data["dynamic-plugins.yaml"]

			// Content must be a ConfigMap YAML (with apiVersion, kind, metadata, data)
			var innerCM map[string]interface{}
			err = yaml.Unmarshal([]byte(dpContent), &innerCM)
			g.Expect(err).ShouldNot(HaveOccurred(), "dynamic-plugins.yaml should be valid YAML")
			g.Expect(innerCM).To(HaveKey("apiVersion"))
			g.Expect(innerCM).To(HaveKey("kind"))
			g.Expect(innerCM["kind"]).To(Equal("ConfigMap"))

			// Extract the inner data and parse as DynaPluginsConfig
			innerData, ok := innerCM["data"].(map[interface{}]interface{})
			g.Expect(ok).To(BeTrue(), "ConfigMap should have data field")
			innerDPContent, ok := innerData["dynamic-plugins.yaml"].(string)
			g.Expect(ok).To(BeTrue(), "inner ConfigMap should have dynamic-plugins.yaml key")

			var config model.DynaPluginsConfig
			err = yaml.Unmarshal([]byte(innerDPContent), &config)
			g.Expect(err).ShouldNot(HaveOccurred(), "inner content should be parseable as DynaPluginsConfig")
			g.Expect(config.Plugins).ShouldNot(BeEmpty(), "should have at least one plugin")
			g.Expect(config.Plugins[0].Package).To(ContainSubstring("oci://"))
		}, time.Second*30, time.Second).Should(Succeed())

		// Verify the catalog has Ready=True condition
		Eventually(func(g Gomega) {
			updatedCatalog := &api.DevHubPluginCatalog{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: catalogName, Namespace: ns}, updatedCatalog)
			g.Expect(err).ShouldNot(HaveOccurred())

			var readyCondition *metav1.Condition
			for i := range updatedCatalog.Status.Conditions {
				if updatedCatalog.Status.Conditions[i].Type == v1alpha5.ConditionTypeReady {
					readyCondition = &updatedCatalog.Status.Conditions[i]
					break
				}
			}
			g.Expect(readyCondition).ShouldNot(BeNil(), "should have Ready condition")
			g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(readyCondition.Reason).To(Equal(v1alpha5.ConditionReasonSucceeded))
		}, time.Second*10, time.Second).Should(Succeed())
	})
})
