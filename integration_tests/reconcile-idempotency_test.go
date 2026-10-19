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

// Go randomizes map iteration order, but only among a few starting points for a small
// map. Several objects with this many keys each make an unsorted rendering differ
// between reconciles in practice every time.
const keysPerObject = 12

// RHDHBUGS-3084: the operator rendered mounts and envs by ranging over ConfigMap and
// Secret data, so every reconcile produced a different pod template and the workload
// rolled forever. utils.SortedKeys fixed it. The assertions are on the rendered
// workload, not on SortedKeys, so a new call site that ranges over a map directly is
// caught too. (app-config ConfigMaps are single-entry now, so they are not used here.)
var _ = When("reconciling an unchanged Backstage again", func() {

	const reconciles = 3

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
		cmFile := generateConfigMap(ctx, k8sClient, "cm-file", ns, manyKeys("file", "content"), nil, nil)
		cmEnv := generateConfigMap(ctx, k8sClient, "cm-env", ns, manyKeys("ENV", "value"), nil, nil)
		secretEnv := generateSecret(ctx, k8sClient, "secret-env", ns, manyKeys("SECENV", "value"), nil, nil)

		bs := api.BackstageSpec{
			Deployment: &api.BackstageDeployment{Kind: kind},
			Application: &api.Application{
				ExtraFiles: &api.ExtraFiles{
					ConfigMaps: []api.FileObjectRef{{Name: cmFile}},
				},
				ExtraEnvs: &api.ExtraEnvs{
					ConfigMaps: []api.EnvObjectRef{{Name: cmEnv}},
					Secrets:    []api.EnvObjectRef{{Name: secretEnv}},
				},
			},
		}
		backstageName := createAndReconcileBackstage(ctx, ns, bs, "")

		generation, template := renderedWorkload(ctx, ns, backstageName)

		for i := range reconciles {
			_, err := NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			gotGeneration, gotTemplate := renderedWorkload(ctx, ns, backstageName)
			Expect(gotTemplate).To(Equal(template), "pod template changed on reconcile %d", i+2)
			Expect(gotGeneration).To(Equal(generation), "%s generation advanced on reconcile %d", kind, i+2)
		}
	},
		Entry("for a Deployment", "Deployment"),
		Entry("for a StatefulSet", "StatefulSet"),
	)
})

// manyKeys returns keysPerObject entries named <prefix>00, <prefix>01, ...
func manyKeys(prefix, value string) map[string]string {
	data := map[string]string{}
	for i := range keysPerObject {
		data[fmt.Sprintf("%s%02d", prefix, i)] = value
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
