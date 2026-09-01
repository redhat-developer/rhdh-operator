package model

import (
	"k8s.io/apimachinery/pkg/runtime"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

// BackstageNetworkPolicyFactory creates BackstageNetworkPolicy instances for the RuntimeObject registry.
type BackstageNetworkPolicyFactory struct{}

func (f BackstageNetworkPolicyFactory) newBackstageObject() RuntimeObject {
	return &BackstageNetworkPolicy{}
}

// BackstageNetworkPolicy manages both backend and DB NetworkPolicies as a single MultiObject.
// DB-scoped policies (podSelector value "backstage-psql") are filtered out when local DB is disabled.
type BackstageNetworkPolicy struct {
	networkPolicies *multiobject.MultiObject
	model           *BackstageModel
}

var _ RuntimeObject = (*BackstageNetworkPolicy)(nil)
var _ ObjectFactory = BackstageNetworkPolicyFactory{}

func init() {
	registerConfig(NetworkPolicyKey, BackstageNetworkPolicyFactory{}, true, mergeMultiObjectConfigs)
}

func (b *BackstageNetworkPolicy) Object() runtime.Object {
	if b.networkPolicies != nil && len(b.networkPolicies.Items) > 0 {
		return b.networkPolicies
	}
	return nil
}

func (b *BackstageNetworkPolicy) GetKey() string {
	return NetworkPolicyKey
}

// addToModel loads the NP config and filters out DB-scoped policies when local DB is disabled.
func (b *BackstageNetworkPolicy) addToModel(model *BackstageModel, _ api.Backstage, config runtime.Object, _ *runtime.Scheme) error {
	b.model = model

	if config != nil {
		b.networkPolicies = config.(*multiobject.MultiObject)
	}

	if b.networkPolicies != nil && !model.localDbEnabled {
		b.filterForExternalDb()
	}

	model.setRuntimeObject(b)
	return nil
}

// updateAndValidate sets metadata, replaces placeholder labels, and applies platform-specific adjustments.
func (b *BackstageNetworkPolicy) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
	if b.networkPolicies == nil || len(b.networkPolicies.Items) == 0 {
		return nil
	}
	b.setMetaInfo(backstage, scheme)
	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)
		replaceRuleLabels(np, backstage.Name)
		if b.model.isOpenshift && np.GetAnnotations()[ConfiguredNameAnnotation] == "allow-router-ingress" {
			setOpenShiftIngressSelector(np)
		}
	}
	return nil
}

// setMetaInfo sets names, labels, annotations, namespace, and owner references on each NP.
func (b *BackstageNetworkPolicy) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)

		namePrefix := "netpol"
		objectLabel := utils.BackstageAppLabelValue(backstage.Name)
		if isDbScoped(np) {
			namePrefix = "db-netpol"
			objectLabel = utils.BackstageDbAppLabelValue(backstage.Name)
		}

		replaceLabel(np.Spec.PodSelector.MatchLabels, backstage.Name)

		utils.GenerateLabel(&np.Labels, BackstageAppLabel, objectLabel)
		utils.AddAnnotation(item, ConfiguredNameAnnotation, item.GetName())
		item.SetName(DefaultMultiObjectName(namePrefix, backstage.Name, item.GetName()))
		setMetaInfo(item, backstage, scheme)
	}
}

// isDbScoped returns true if the NP targets DB pods (podSelector placeholder is "backstage-psql").
func isDbScoped(np *networkingv1.NetworkPolicy) bool {
	val, ok := np.Spec.PodSelector.MatchLabels[BackstageAppLabel]
	return ok && val == utils.BackstageDBAppName
}

// replaceLabel substitutes a placeholder label value with the CR-specific value.
func replaceLabel(labels map[string]string, backstageName string) {
	val, ok := labels[BackstageAppLabel]
	if !ok {
		return
	}
	switch val {
	case utils.BackstageDBAppName:
		labels[BackstageAppLabel] = utils.BackstageDbAppLabelValue(backstageName)
	case utils.BackstageAppName:
		labels[BackstageAppLabel] = utils.BackstageAppLabelValue(backstageName)
	}
}

// filterForExternalDb drops DB-scoped NPs and broadens psql egress for external DB connectivity.
func (b *BackstageNetworkPolicy) filterForExternalDb() {
	filtered := make([]client.Object, 0, len(b.networkPolicies.Items))
	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)
		if isDbScoped(np) {
			continue
		}
		clearDbEgressTargets(np)
		filtered = append(filtered, item)
	}
	b.networkPolicies.Items = filtered
}

// clearDbEgressTargets removes the DB pod selector from egress rules, broadening them for external DB.
func clearDbEgressTargets(np *networkingv1.NetworkPolicy) {
	for i := range np.Spec.Egress {
		for j := range np.Spec.Egress[i].To {
			peer := &np.Spec.Egress[i].To[j]
			if peer.PodSelector == nil {
				continue
			}
			if val, ok := peer.PodSelector.MatchLabels[BackstageAppLabel]; ok && val == utils.BackstageDBAppName {
				np.Spec.Egress[i].To = nil
				return
			}
		}
	}
}

// replaceRuleLabels replaces placeholder labels in all egress and ingress peer selectors.
func replaceRuleLabels(np *networkingv1.NetworkPolicy, backstageName string) {
	for i := range np.Spec.Egress {
		replacePeerLabels(np.Spec.Egress[i].To, backstageName)
	}
	for i := range np.Spec.Ingress {
		replacePeerLabels(np.Spec.Ingress[i].From, backstageName)
	}
}

// replacePeerLabels replaces placeholder labels in a list of NetworkPolicy peers.
func replacePeerLabels(peers []networkingv1.NetworkPolicyPeer, backstageName string) {
	for i := range peers {
		if peers[i].PodSelector != nil {
			replaceLabel(peers[i].PodSelector.MatchLabels, backstageName)
		}
	}
}

// setOpenShiftIngressSelector narrows the router ingress from "any namespace" to OCP ingress-labeled namespaces.
func setOpenShiftIngressSelector(np *networkingv1.NetworkPolicy) {
	for i := range np.Spec.Ingress {
		for j := range np.Spec.Ingress[i].From {
			if np.Spec.Ingress[i].From[j].NamespaceSelector != nil {
				np.Spec.Ingress[i].From[j].NamespaceSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"policy-group.network.openshift.io/ingress": "",
					},
				}
			}
		}
	}
}
