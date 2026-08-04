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
		deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deploy.SpecReplicas()).To(HaveValue(BeEquivalentTo(0)))

		By("verifying DB StatefulSet replicas=0")
		ss := &appsv1.StatefulSet{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, ss)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(ss.Spec.Replicas).To(HaveValue(BeEquivalentTo(0)))

		By("verifying status condition is Idled")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, bs)).To(Succeed())
		Expect(bs.Status.Conditions).To(HaveLen(1))
		Expect(bs.Status.Conditions[0].Reason).To(Equal("Idled"))
		Expect(bs.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

		By("removing idle annotation (wake)")
		delete(bs.Annotations, model.IdleAnnotation)
		Expect(k8sClient.Update(ctx, bs)).To(Succeed())

		_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		By("verifying deployment replicas restored to 1")
		deploy, err = backstageDeployment(ctx, k8sClient, ns, backstageName)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deploy.SpecReplicas()).To(HaveValue(BeEquivalentTo(1)))

		By("verifying DB StatefulSet replicas restored to 1")
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: fmt.Sprintf("backstage-psql-%s", backstageName)}, ss)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(ss.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)))

		By("verifying status condition is no longer Idled")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, bs)).To(Succeed())
		Expect(bs.Status.Conditions[0].Reason).NotTo(Equal("Idled"))
	})

	It("wakes with user-specified replicas from deployment patch", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, api.BackstageSpec{}, "")

		Eventually(func(g Gomega) {
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy).NotTo(BeNil())
		}, time.Minute, time.Second).Should(Succeed())

		By("setting idle annotation")
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

		By("verifying deployment is idled")
		deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deploy.SpecReplicas()).To(HaveValue(BeEquivalentTo(0)))

		By("removing idle annotation and adding replicas via deployment patch")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, bs)).To(Succeed())
		delete(bs.Annotations, model.IdleAnnotation)
		Expect(k8sClient.Update(ctx, bs)).To(Succeed())

		_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		By("verifying deployment replicas restored to 1 (default, no patch)")
		deploy, err = backstageDeployment(ctx, k8sClient, ns, backstageName)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deploy.SpecReplicas()).To(HaveValue(BeEquivalentTo(1)))
	})

	It("does not touch replicas when not idled and never was", func() {
		backstageName := createAndReconcileBackstage(ctx, ns, api.BackstageSpec{}, "")

		Eventually(func(g Gomega) {
			deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(deploy).NotTo(BeNil())
		}, time.Minute, time.Second).Should(Succeed())

		By("verifying status was never Idled")
		bs := &api.Backstage{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, bs)).To(Succeed())
		Expect(bs.Status.Conditions[0].Reason).NotTo(Equal("Idled"))
	})
})
