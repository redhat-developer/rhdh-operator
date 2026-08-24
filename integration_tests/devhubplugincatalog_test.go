package integration_tests

import (
	"context"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/internal/controller"
	"github.com/redhat-developer/rhdh-operator/pkg/catalog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	defaultCatalogIndexImage = "quay.io/rhdh/plugin-catalog-index:next"
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
				Name: catalogName,
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

			// Should have the catalogs-ready marker
			g.Expect(cm.Data).To(HaveKey(catalog.CatalogsReadyKey))
			g.Expect(cm.Data[catalog.CatalogsReadyKey]).To(Equal("true"))

			// Should have dynamic-plugins.yaml with content
			g.Expect(cm.Data).To(HaveKey("dynamic-plugins.yaml"))
			dpContent := cm.Data["dynamic-plugins.yaml"]
			g.Expect(dpContent).To(ContainSubstring("plugins:"))
			g.Expect(dpContent).To(ContainSubstring("oci://"))
		}, time.Second*30, time.Second).Should(Succeed())
	})
})
