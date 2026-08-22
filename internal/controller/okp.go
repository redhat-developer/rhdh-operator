package controller

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
)

// okpServiceURL builds the URL the lightspeed-core sidecar uses to reach OKP.
// When an OpenShift ingress domain is known, the Route hostname is used;
// otherwise the in-cluster Service DNS name is used.
func okpServiceURL(name, namespace, ingressDomain string) string {
	if ingressDomain != "" {
		return fmt.Sprintf("http://%s-%s.%s", name, namespace, ingressDomain)
	}
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", name, namespace)
}

// prepareOkpEnvVar injects OKP_SERVICE_URL into the lightspeed-core container
// in the model's deployment spec BEFORE applyObjects, avoiding a second rollout.
// OKP is only deployed on OpenShift; vanilla K8s uses the no-okp config.
//
// The OKP Deployment/Service/Route themselves are declared as flavour YAML and applied
// via the runtime model (see pkg/model/okp-*.go); this only wires the sidecar to them.
func (r *BackstageReconciler) prepareOkpEnvVar(backstage *api.Backstage, bsModel *model.BackstageModel) {
	if !model.IsFlavourEnabled(backstage.Spec, "lightspeed") || !r.Platform.IsOpenshift() {
		return
	}

	name := model.OkpName(backstage.Name)
	url := okpServiceURL(name, backstage.Namespace, bsModel.ExternalConfig.OpenShiftIngressDomain)

	_ = bsModel.InjectContainerEnvVar("lightspeed-core", "OKP_SERVICE_URL", url)
}

// prepareOkpConfig selects the appropriate lightspeed-stack config based on platform.
// On OpenShift, the full config (with rag/okp sections) is used.
// On vanilla K8s, the no-okp variant is swapped in to prevent LCORE from crashing
// when Solr is unreachable.
func (r *BackstageReconciler) prepareOkpConfig(backstage *api.Backstage, bsModel *model.BackstageModel) {
	if !model.IsFlavourEnabled(backstage.Spec, "lightspeed") {
		return
	}

	if !r.Platform.IsOpenshift() {
		bsModel.SwapConfigMapDataKey("lightspeed-stack.yaml", "lightspeed-stack-no-okp.yaml")
	} else {
		bsModel.RemoveConfigMapDataKey("lightspeed-stack-no-okp.yaml")
	}
}
