package integration_tests

import (
	"context"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

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

	It("creates Backstage object (on Openshift)", func() {

		if !isOpenshiftCluster() {
			Skip("Skipped for non-Openshift cluster")
		}

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			Application: &bsv1.Application{
				Route: &bsv1.Route{
					//Host:      "localhost",
					//Enabled:   ptr.To(true),
					Subdomain: "test",
				},
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
			By("creating Route")
			route := &openshift.Route{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.RouteName(backstageName)}, route)
			g.Expect(err).To(Not(HaveOccurred()), controllerMessage())

			g.Expect(route.Status.Ingress).To(HaveLen(1))
			g.Expect(route.Status.Ingress[0].Host).To(Not(BeEmpty()))

		}, 5*time.Minute, time.Second).Should(Succeed())

	})
})
