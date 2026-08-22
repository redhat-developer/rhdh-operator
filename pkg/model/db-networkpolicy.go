package model

import (
	"k8s.io/apimachinery/pkg/runtime"

	networkingv1 "k8s.io/api/networking/v1"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

type DbNetworkPolicyFactory struct{}

func (f DbNetworkPolicyFactory) newBackstageObject() RuntimeObject {
	return &DbNetworkPolicy{}
}

type DbNetworkPolicy struct {
	networkPolicies *multiobject.MultiObject
	model           *BackstageModel
}

func init() {
	registerConfig(DbNetworkPolicyKey, DbNetworkPolicyFactory{}, true, nil)
}

func (b *DbNetworkPolicy) Object() runtime.Object {
	if b.networkPolicies != nil && len(b.networkPolicies.Items) > 0 {
		return b.networkPolicies
	}
	return nil
}

func (b *DbNetworkPolicy) GetKey() string {
	return DbNetworkPolicyKey
}

func (b *DbNetworkPolicy) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	b.model = model

	if model.localDbEnabled && config != nil {
		b.networkPolicies = config.(*multiobject.MultiObject)
	}

	model.setRuntimeObject(b)

	if b.networkPolicies != nil && len(b.networkPolicies.Items) > 0 {
		b.setMetaInfo(backstage, scheme)
	}

	return nil
}

func (b *DbNetworkPolicy) updateAndValidate(_ api.Backstage, _ *runtime.Scheme) error {
	return nil
}

func (b *DbNetworkPolicy) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	dbLabel := utils.BackstageDbAppLabelValue(backstage.Name)
	backendLabel := utils.BackstageAppLabelValue(backstage.Name)

	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)

		np.Spec.PodSelector.MatchLabels[BackstageAppLabel] = dbLabel

		for i := range np.Spec.Ingress {
			for j := range np.Spec.Ingress[i].From {
				if np.Spec.Ingress[i].From[j].PodSelector != nil {
					if _, ok := np.Spec.Ingress[i].From[j].PodSelector.MatchLabels[BackstageAppLabel]; ok {
						np.Spec.Ingress[i].From[j].PodSelector.MatchLabels[BackstageAppLabel] = backendLabel
					}
				}
			}
		}

		utils.GenerateLabel(&np.Labels, BackstageAppLabel, dbLabel)
		utils.AddAnnotation(item, ConfiguredNameAnnotation, item.GetName())
		item.SetName(DefaultMultiObjectName("db-netpol", backstage.Name, item.GetName()))
		setMetaInfo(item, backstage, scheme)
	}
}
