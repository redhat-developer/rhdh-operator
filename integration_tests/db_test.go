package integration_tests

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/utils/ptr"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/redhat-developer/rhdh-operator/api"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create backstage with CR configured", func() {

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

	It("creates Backstage with disabled local DB and secret", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, api.BackstageSpec{
			Database: &api.Database{
				EnableLocalDb:  ptr.To(false),
				AuthSecretName: "existing-secret",
			},
		}, "")

		Eventually(func(g Gomega) {
			By("not creating a StatefulSet for the Database")
			err := k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)},
				&appsv1.StatefulSet{})
			g.Expect(err).Should(HaveOccurred())
			g.Expect(errors.IsNotFound(err))

			By("Checking if Deployment was successfully created in the reconciliation")
			_, err = backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).Should(Not(HaveOccurred()))
		}, time.Minute, time.Second).Should(Succeed())
	})

	It("creates Backstage with disabled local DB no secret", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, api.BackstageSpec{
			Database: &api.Database{
				EnableLocalDb: ptr.To(false),
			},
		}, "")

		Eventually(func(g Gomega) {
			By("not creating a StatefulSet for the Database")
			err := k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)},
				&appsv1.StatefulSet{})
			g.Expect(err).Should(HaveOccurred())
			g.Expect(errors.IsNotFound(err))

			By("Checking if Deployment was successfully created in the reconciliation")
			_, err = backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).Should(Not(HaveOccurred()))
		}, time.Minute, time.Second).Should(Succeed())
	})

})
