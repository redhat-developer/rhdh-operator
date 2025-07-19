package integration_tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/utils/ptr"

	openshift "github.com/openshift/api/route/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-developer/rhdh-operator/internal/controller"

	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/rand"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var useExistingController = false
var currentPlatform platform.Platform

type TestBackstageReconciler struct {
	rec       controller.BackstageReconciler
	namespace string
}

func init() {
	rand.Seed(time.Now().UnixNano())
	//testOnExistingCluster, _ = strconv.ParseBool(os.Getenv("TEST_ON_EXISTING_CLUSTER"))
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Integration Test Suite")
}

var _ = BeforeSuite(func() {
	//logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases"), filepath.Join("..", "config", "crd", "external")},
		ErrorIfCRDPathMissing: true,
	}

	testEnv.UseExistingCluster = ptr.To(false)
	if val, ok := os.LookupEnv("USE_EXISTING_CLUSTER"); ok {
		boolValue, err := strconv.ParseBool(val)
		if err == nil {
			testEnv.UseExistingCluster = ptr.To(boolValue)
		}
	}

	if val, ok := os.LookupEnv("USE_EXISTING_CONTROLLER"); ok {
		boolValue, err := strconv.ParseBool(val)
		if err == nil {
			useExistingController = boolValue
		}
	}

	var err error

	if *testEnv.UseExistingCluster {
		currentPlatform, err = controller.DetectPlatform()
		Expect(err).To(Not(HaveOccurred()))
	} else {
		currentPlatform = platform.Default
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = bsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	utilruntime.Must(openshift.Install(scheme.Scheme))

	err = monitoringv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// generateRandName return random name if name is empty or name itself otherwise
func generateRandName(name string) string {
	if name != "" {
		return name
	}
	return "test-backstage-" + randString(5)
}

func createBackstage(ctx context.Context, spec bsv1.BackstageSpec, ns string, name string) string {

	backstageName := generateRandName(name)

	err := k8sClient.Create(ctx, &bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backstageName,
			Namespace: ns,
		},
		Spec: spec,
	})
	Expect(err).To(Not(HaveOccurred()))
	return backstageName
}

func createAndReconcileBackstage(ctx context.Context, ns string, spec bsv1.BackstageSpec, name string) string {
	backstageName := createBackstage(ctx, spec, ns, name)

	Eventually(func() error {
		found := &bsv1.Backstage{}
		return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
	}, time.Minute, time.Second).Should(Succeed())

	_, err := NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
	})

	if err != nil {
		GinkgoWriter.Printf("===> Error detected on Backstage reconcile: %s \n", err.Error())
		if errors.IsAlreadyExists(err) || errors.IsConflict(err) {
			return backstageName
		}
	}

	Expect(err).To(Not(HaveOccurred()))

	return backstageName
}

func getBackstagePod(ctx context.Context, ns, backstageName string) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := k8sClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{model.BackstageAppLabel: utils.BackstageAppLabelValue(backstageName)})
	if err != nil {
		return nil, err
	}
	if len(podList.Items) != 1 {
		return nil, fmt.Errorf("expected only one Pod, but have %v", podList.Items)
	}

	return &podList.Items[0], nil
}

func createNamespace(ctx context.Context) string {
	ns := fmt.Sprintf("ns-%d-%s", GinkgoParallelProcess(), randString(5))
	err := k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	})
	Expect(err).To(Not(HaveOccurred()))
	return ns
}

func deleteNamespace(ctx context.Context, ns string) {
	_ = k8sClient.Delete(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	})
}

func NewTestBackstageReconciler(namespace string) *TestBackstageReconciler {

	sch := k8sClient.Scheme()
	if currentPlatform.IsOpenshift() {
		utilruntime.Must(openshift.Install(sch))
	}

	return &TestBackstageReconciler{rec: controller.BackstageReconciler{
		Client:   k8sClient,
		Scheme:   sch,
		Platform: currentPlatform,
	}, namespace: namespace}
}

func (t *TestBackstageReconciler) ReconcileAny(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Ignore if USE_EXISTING_CLUSTER = true and USE_EXISTING_CONTROLLER=true
	// Ignore requests for other namespaces, if specified.
	// To overcome a limitation of EnvTest about namespace deletion.
	// More details on https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
	if (*testEnv.UseExistingCluster && useExistingController) || (t.namespace != "" && req.Namespace != t.namespace) {
		return ctrl.Result{}, nil
	}
	return t.rec.Reconcile(ctx, req)
}

func isProfile(profile string) bool {
	ev, found := os.LookupEnv("PROFILE")
	if !found {
		// We take "rhdh" profile as default
		ev = "rhdh"
	}
	return profile == ev
}

func controllerMessage() string {
	if useExistingController == true {
		return "USE_EXISTING_CONTROLLER=true configured. Make sure Controller manager is up and running."
	}
	return ""
}
