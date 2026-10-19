package integration_tests

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/redhat-developer/rhdh-operator/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// RHDHBUGS-3084: the operator mounted ConfigMap files one subPath per key by ranging over
// the ConfigMap data, so every reconcile produced a different pod template and the
// workload rolled forever. utils.SortedKeys fixed it. The assertions are on the rendered
// workload, not on SortedKeys, so a regression anywhere on the extraFiles ConfigMap mount
// path is caught, not only one inside SortedKeys. app-config ConfigMaps are single-entry now, a Secret under extraFiles is
// mounted as one key or a whole directory, and extraEnvs without a key render a single
// envFrom, so none of them depends on key order and they are not used here.
var _ = When("reconciling an unchanged Backstage again", func() {

	// on top of the one createAndReconcileBackstage runs
	const extraReconciles = 3

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

	DescribeTable("keeps the workload unchanged", func(kind string) {
		if useExistingController {
			Skip("Skipped for real controller: the test drives every reconcile itself")
		}

		cmFile := generateConfigMap(ctx, k8sClient, "cm-file", ns, manyKeys(), nil, nil)

		bs := api.BackstageSpec{
			Deployment: &api.BackstageDeployment{Kind: kind},
			Application: &api.Application{
				ExtraFiles: &api.ExtraFiles{
					ConfigMaps: []api.FileObjectRef{{Name: cmFile}},
				},
			},
		}
		backstageName := createAndReconcileBackstage(ctx, ns, bs, "")

		generation, template := renderedWorkload(ctx, ns, backstageName)

		for i := range extraReconciles {
			_, err := NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			gotGeneration, gotTemplate := renderedWorkload(ctx, ns, backstageName)
			Expect(gotTemplate).To(Equal(template), "pod template changed on extra reconcile %d", i+1)
			Expect(gotGeneration).To(Equal(generation), "%s generation advanced on extra reconcile %d", kind, i+1)
		}
	},
		Entry("for a Deployment", "Deployment"),
		Entry("for a StatefulSet", "StatefulSet"),
	)
})

// manyKeys returns ConfigMap data with entries named file00, file01, ...
func manyKeys() map[string]string {
	// Go randomizes map iteration order, but only among a few starting points for a small
	// map. With this many keys an unsorted rendering almost always differs between
	// reconciles.
	const keys = 12
	data := map[string]string{}
	for i := range keys {
		data[fmt.Sprintf("file%02d", i)] = "content"
	}
	return data
}

// renderedWorkload returns the Backstage workload's generation and its pod template
// serialized to JSON, so two renderings compare byte for byte.
func renderedWorkload(ctx context.Context, ns, backstageName string) (int64, string) {
	deploy, err := backstageDeployment(ctx, k8sClient, ns, backstageName)
	Expect(err).NotTo(HaveOccurred())
	template, err := json.Marshal(struct {
		Metadata any `json:"metadata"`
		Spec     any `json:"spec"`
	}{deploy.PodObjectMeta(), deploy.PodSpec()})
	Expect(err).NotTo(HaveOccurred())
	return deploy.GetObject().GetGeneration(), string(template)
}
