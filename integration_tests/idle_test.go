package integration_tests

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("backstage idle annotation", func() {

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

	It("idles and wakes the instance", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, api.BackstageSpec{}, "")

		Eventually(func(g Gomega) {
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy).NotTo(BeNil())
		}, time.Minute, time.Second).Should(Succeed())

		By("setting idle annotation to true")
		bs := &api.Backstage{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, bs)).To(Succeed())
		if bs.Annotations == nil {
			bs.Annotations = map[string]string{}
		}
		bs.Annotations[model.IdleAnnotation] = "true"
		Expect(k8sClient.Update(ctx, bs)).To(Succeed())

		_, err := NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		By("verifying deployment replicas=0")
		Eventually(func(g Gomega) {
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy.SpecReplicas()).To(HaveValue(BeEquivalentTo(0)))
		}, time.Minute, time.Second).Should(Succeed())

		By("verifying DB StatefulSet replicas=0")
		Eventually(func(g Gomega) {
			ss := &appsv1.StatefulSet{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, ss)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(ss.Spec.Replicas).To(HaveValue(BeEquivalentTo(0)))
		}, time.Minute, time.Second).Should(Succeed())

		By("verifying status condition is Idled")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, bs)).To(Succeed())
			g.Expect(bs.Status.Conditions).To(HaveLen(1))
			g.Expect(bs.Status.Conditions[0].Reason).To(Equal("Idled"))
			g.Expect(bs.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		}, time.Minute, time.Second).Should(Succeed())

		By("removing idle annotation (wake)")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, bs)).To(Succeed())
		delete(bs.Annotations, model.IdleAnnotation)
		Expect(k8sClient.Update(ctx, bs)).To(Succeed())

		_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		By("verifying deployment replicas restored to 1")
		Eventually(func(g Gomega) {
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy.SpecReplicas()).To(HaveValue(BeEquivalentTo(1)))
		}, time.Minute, time.Second).Should(Succeed())

		By("verifying DB StatefulSet replicas restored to 1")
		Eventually(func(g Gomega) {
			ss := &appsv1.StatefulSet{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, ss)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(ss.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)))
		}, time.Minute, time.Second).Should(Succeed())

		By("verifying status condition is no longer Idled")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, bs)).To(Succeed())
			g.Expect(bs.Status.Conditions[0].Reason).NotTo(Equal("Idled"))
		}, time.Minute, time.Second).Should(Succeed())
	})

})
