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


type BackstageNetworkPolicyFactory struct{}

func (f BackstageNetworkPolicyFactory) newBackstageObject() RuntimeObject {
	return &BackstageNetworkPolicy{}
}

type BackstageNetworkPolicy struct {
	networkPolicies *multiobject.MultiObject
	model           *BackstageModel
}

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

func (b *BackstageNetworkPolicy) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	b.model = model

	if config != nil {
		b.networkPolicies = config.(*multiobject.MultiObject)
	}

	if b.networkPolicies != nil && !model.localDbEnabled {
		b.filterForExternalDb()
	}

	if b.networkPolicies != nil && len(b.networkPolicies.Items) > 0 {
		b.setMetaInfo(backstage, scheme)
	}

	model.setRuntimeObject(b)
	return nil
}

func (b *BackstageNetworkPolicy) updateAndValidate(backstage api.Backstage, _ *runtime.Scheme) error {
	if b.networkPolicies == nil {
		return nil
	}
	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)
		replaceRuleLabels(np, backstage.Name)
		if b.model.isOpenshift && np.GetAnnotations()[ConfiguredNameAnnotation] == "allow-router-ingress" {
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
	}
	return nil
}

func isDbScoped(np *networkingv1.NetworkPolicy) bool {
	val, ok := np.Spec.PodSelector.MatchLabels[BackstageAppLabel]
	return ok && val == utils.BackstageDBAppName
}

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

func replacePeerLabels(peers []networkingv1.NetworkPolicyPeer, backstageName string) {
	for i := range peers {
		if peers[i].PodSelector != nil {
			replaceLabel(peers[i].PodSelector.MatchLabels, backstageName)
		}
	}
}

func replaceRuleLabels(np *networkingv1.NetworkPolicy, backstageName string) {
	for i := range np.Spec.Egress {
		replacePeerLabels(np.Spec.Egress[i].To, backstageName)
	}
	for i := range np.Spec.Ingress {
		replacePeerLabels(np.Spec.Ingress[i].From, backstageName)
	}
}

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
