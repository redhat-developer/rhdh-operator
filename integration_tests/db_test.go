package integration_tests

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"

	appsv1 "k8s.io/api/apps/v1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

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

	It("creates default Backstage and then update CR to not to use local DB", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{}, "")

		Eventually(func(g Gomega) {
			By("creating Deployment with database.enableLocalDb=true by default")

			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, &appsv1.StatefulSet{})
			g.Expect(err).To(Not(HaveOccurred()))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, &corev1.Service{})
			g.Expect(err).To(Not(HaveOccurred()))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-secret-%s", backstageName)}, &corev1.Secret{})
			g.Expect(err).To(Not(HaveOccurred()))

		}, time.Minute, time.Second).Should(Succeed())

		By("updating Backstage")
		update := &bsv1.Backstage{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, update)
		Expect(err).To(Not(HaveOccurred()))
		update.Spec.Database = &bsv1.Database{}
		update.Spec.Database.EnableLocalDb = ptr.To(false)
		err = k8sClient.Update(ctx, update)
		Expect(err).To(Not(HaveOccurred()))
		_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		Eventually(func(g Gomega) {
			By("deleting Local Db StatefulSet, Service and Secret")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, &appsv1.StatefulSet{})
			g.Expect(err).To(HaveOccurred())
			g.Expect(errors.IsNotFound(err))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, &corev1.Service{})
			g.Expect(err).To(HaveOccurred())
			g.Expect(errors.IsNotFound(err))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-secret-%s", backstageName)}, &corev1.Secret{})
			g.Expect(err).To(HaveOccurred())
			g.Expect(errors.IsNotFound(err))
		}, time.Minute, time.Second).Should(Succeed())

	})

	It("creates Backstage with disabled local DB and secret", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			Database: &bsv1.Database{
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
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, &appsv1.Deployment{})
			g.Expect(err).Should(Not(HaveOccurred()))
		}, time.Minute, time.Second).Should(Succeed())
	})

	It("creates Backstage with disabled local DB no secret", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			Database: &bsv1.Database{
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
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, &appsv1.Deployment{})
			g.Expect(err).Should(Not(HaveOccurred()))
		}, time.Minute, time.Second).Should(Succeed())
	})

})
