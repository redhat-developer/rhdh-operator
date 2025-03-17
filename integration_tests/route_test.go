package integration_tests

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/redhat-developer/rhdh-operator/pkg/model"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	openshift "github.com/openshift/api/route/v1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

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

	for _, tt := range []struct {
		name              string
		desiredRoute      bsv1.Route
		expectedBaseUrlFn func(ingressDomain string) string
	}{
		{
			name: "route disabled",
			desiredRoute: bsv1.Route{
				Enabled: ptr.To(false),
			},
			expectedBaseUrlFn: func(ingressDomain string) string {
				return "http://localhost:7007"
			},
		},
		{
			name: "route with subdomain",
			desiredRoute: bsv1.Route{
				//Host:      "localhost",
				//Enabled:   ptr.To(true),
				Subdomain: "test",
			},
			expectedBaseUrlFn: func(ingressDomain string) string {
				return fmt.Sprintf("https://test.%s", ingressDomain)
			},
		},
		{
			name: "route with host",
			desiredRoute: bsv1.Route{
				Host: "host.example.com",
			},
			expectedBaseUrlFn: func(ingressDomain string) string {
				return "https://host.example.com"
			},
		},
	} {
		tt := tt
		It("creates Backstage object (on Openshift) - "+tt.name, func() {

			if !isOpenshiftCluster() {
				Skip("Skipped for non-Openshift cluster")
			}

			backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
				Application: &bsv1.Application{
					Route: &tt.desiredRoute,
				},
			}, "")

			Eventually(func() error {
				found := &bsv1.Backstage{}
				return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
			}, time.Minute, time.Second).Should(Succeed())

			_, err := NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
			})
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func(g Gomega) {
				if ptr.Deref(tt.desiredRoute.Enabled, true) {
					By("creating Route")
					route := &openshift.Route{}
					err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.RouteName(backstageName)}, route)
					g.Expect(err).To(Not(HaveOccurred()), controllerMessage())

					g.Expect(route.Status.Ingress).To(HaveLen(1))
					g.Expect(route.Status.Ingress[0].Host).To(Not(BeEmpty()))
				}

				By("updating the baseUrls in the default app-config CM, per the desired route settings (RHIDP-6192)")
				var appConfigCm corev1.ConfigMap
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.AppConfigDefaultName(backstageName)}, &appConfigCm)
				g.Expect(err).ShouldNot(HaveOccurred())
				domain, err := utils.GetOCPIngressDomain()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(appConfigCm).To(
					HaveAppConfigBaseUrl(tt.expectedBaseUrlFn(domain)))
			}, 5*time.Minute, time.Second).Should(Succeed())

		})
	}
})
