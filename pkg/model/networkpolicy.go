package model

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

const dbLabelPrefix = "backstage-psql"

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

func isDbScoped(np *networkingv1.NetworkPolicy) bool {
	val, ok := np.Spec.PodSelector.MatchLabels[BackstageAppLabel]
	return ok && strings.HasPrefix(val, dbLabelPrefix)
}

func replaceLabel(labels map[string]string, backstageName string) {
	val, ok := labels[BackstageAppLabel]
	if !ok {
		return
	}
	if strings.HasPrefix(val, dbLabelPrefix) {
		labels[BackstageAppLabel] = utils.BackstageDbAppLabelValue(backstageName)
	} else {
		labels[BackstageAppLabel] = utils.BackstageAppLabelValue(backstageName)
	}
}

func (b *BackstageNetworkPolicy) filterForExternalDb() {
	filtered := make([]client.Object, 0, len(b.networkPolicies.Items))
	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)
		if isDbScoped(np) {
			continue
		}
		for i := range np.Spec.Egress {
			for j := range np.Spec.Egress[i].To {
				if np.Spec.Egress[i].To[j].PodSelector != nil {
					if val, ok := np.Spec.Egress[i].To[j].PodSelector.MatchLabels[BackstageAppLabel]; ok && strings.HasPrefix(val, dbLabelPrefix) {
						np.Spec.Egress[i].To = nil
					}
				}
			}
		}
		filtered = append(filtered, item)
	}
	b.networkPolicies.Items = filtered
}

func (b *BackstageNetworkPolicy) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	for _, item := range b.networkPolicies.Items {
		np := item.(*networkingv1.NetworkPolicy)

		replaceLabel(np.Spec.PodSelector.MatchLabels, backstage.Name)

		for i := range np.Spec.Egress {
			for j := range np.Spec.Egress[i].To {
				if np.Spec.Egress[i].To[j].PodSelector != nil {
					replaceLabel(np.Spec.Egress[i].To[j].PodSelector.MatchLabels, backstage.Name)
				}
			}
		}

		for i := range np.Spec.Ingress {
			for j := range np.Spec.Ingress[i].From {
				if np.Spec.Ingress[i].From[j].PodSelector != nil {
					replaceLabel(np.Spec.Ingress[i].From[j].PodSelector.MatchLabels, backstage.Name)
				}
			}
		}

		namePrefix := "netpol"
		objectLabel := utils.BackstageAppLabelValue(backstage.Name)
		if isDbScoped(np) {
			namePrefix = "db-netpol"
			objectLabel = utils.BackstageDbAppLabelValue(backstage.Name)
		}

		utils.GenerateLabel(&np.Labels, BackstageAppLabel, objectLabel)
		utils.AddAnnotation(item, ConfiguredNameAnnotation, item.GetName())
		item.SetName(DefaultMultiObjectName(namePrefix, backstage.Name, item.GetName()))
		setMetaInfo(item, backstage, scheme)
	}
}
