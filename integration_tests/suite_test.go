//
// Copyright (c) 2023 Red Hat, Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration_tests

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/utils/ptr"

	openshift "github.com/openshift/api/route/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"

	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controller "redhat-developer/red-hat-developer-hub-operator/controllers"

	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/rand"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"

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
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
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
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = bsv1.AddToScheme(scheme.Scheme)
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
	var (
		isOpenshift bool
		err         error
	)
	isOpenshift = isOpenshiftCluster()
	if *testEnv.UseExistingCluster {
		Expect(err).To(Not(HaveOccurred()))
		if isOpenshift {
			utilruntime.Must(openshift.Install(sch))
		}
	} else {
		isOpenshift = false
	}

	return &TestBackstageReconciler{rec: controller.BackstageReconciler{
		Client:      k8sClient,
		Scheme:      sch,
		OwnsRuntime: true,
		// let's set it explicitly to avoid misunderstanding
		IsOpenShift: isOpenshift,
	}, namespace: namespace}
}

//func (t *TestBackstageReconciler) ReconcileLocalCluster(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//
//	if !*testEnv.UseExistingCluster {
//		// Ignore requests for other namespaces, if specified.
//		// To overcome a limitation of EnvTest about namespace deletion.
//		// More details on https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
//		if t.namespace != "" && req.Namespace != t.namespace {
//			return ctrl.Result{}, nil
//		}
//		return t.Reconcile(ctx, req)
//	} else {
//		return ctrl.Result{}, nil
//	}
//}

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

func isOpenshiftCluster() bool {

	if *testEnv.UseExistingCluster {
		isOs, err := utils.IsOpenshift()
		Expect(err).To(Not(HaveOccurred()))
		return isOs
	} else {
		return false
	}
}

func controllerMessage() string {
	if useExistingController == true {
		return "USE_EXISTING_CONTROLLER=true configured. Make sure Controller manager is up and running."
	}
	return ""
}
