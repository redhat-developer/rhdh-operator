package model

import (
	"k8s.io/apimachinery/pkg/runtime"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	model.setRuntimeObject(b)

	if b.networkPolicies != nil && len(b.networkPolicies.Items) > 0 {
		b.setMetaInfo(backstage, scheme)
	}

	return nil
}

func (b *BackstageNetworkPolicy) updateAndValidate(_ api.Backstage, _ *runtime.Scheme) error {
	if b.networkPolicies == nil || !b.model.isOpenshift {
		return nil
	}
	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)
		if np.GetAnnotations()[ConfiguredNameAnnotation] != "allow-router-ingress" {
			continue
		}
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
		break
	}
	return nil
}

func (b *BackstageNetworkPolicy) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	backendLabel := utils.BackstageAppLabelValue(backstage.Name)
	dbLabel := utils.BackstageDbAppLabelValue(backstage.Name)

	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)

		np.Spec.PodSelector.MatchLabels[BackstageAppLabel] = backendLabel

		for i := range np.Spec.Egress {
			for j := range np.Spec.Egress[i].To {
				if np.Spec.Egress[i].To[j].PodSelector != nil {
					if _, ok := np.Spec.Egress[i].To[j].PodSelector.MatchLabels[BackstageAppLabel]; ok {
						np.Spec.Egress[i].To[j].PodSelector.MatchLabels[BackstageAppLabel] = dbLabel
					}
				}
			}
		}

		utils.GenerateLabel(&np.Labels, BackstageAppLabel, backendLabel)
		utils.AddAnnotation(item, ConfiguredNameAnnotation, item.GetName())
		item.SetName(DefaultMultiObjectName("netpol", backstage.Name, item.GetName()))
		setMetaInfo(item, backstage, scheme)
	}
}
