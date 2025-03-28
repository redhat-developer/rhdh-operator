package model

import (
	"fmt"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	openshift "github.com/openshift/api/route/v1"
)

type BackstageRouteFactory struct{}

func (f BackstageRouteFactory) newBackstageObject() RuntimeObject {
	return &BackstageRoute{}
}

type BackstageRoute struct {
	route *openshift.Route
}

func RouteName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage")
}

func (b *BackstageRoute) setRoute(specified *bsv1.Route) {

	if len(specified.Host) > 0 {
		b.route.Spec.Host = specified.Host
	}
	if len(specified.Subdomain) > 0 {
		b.route.Spec.Subdomain = specified.Subdomain
	}
	if specified.TLS == nil {
		return
	}
	if b.route.Spec.TLS == nil {
		b.route.Spec.TLS = &openshift.TLSConfig{
			Termination:                   openshift.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: openshift.InsecureEdgeTerminationPolicyRedirect,
			Certificate:                   specified.TLS.Certificate,
			Key:                           specified.TLS.Key,
			CACertificate:                 specified.TLS.CACertificate,
			ExternalCertificate: &openshift.LocalObjectReference{
				Name: specified.TLS.ExternalCertificateSecretName,
			},
		}
		return
	}
	if len(specified.TLS.Certificate) > 0 {
		b.route.Spec.TLS.Certificate = specified.TLS.Certificate
	}
	if len(specified.TLS.Key) > 0 {
		b.route.Spec.TLS.Key = specified.TLS.Key
	}
	if len(specified.TLS.Certificate) > 0 {
		b.route.Spec.TLS.Certificate = specified.TLS.Certificate
	}
	if len(specified.TLS.CACertificate) > 0 {
		b.route.Spec.TLS.CACertificate = specified.TLS.CACertificate
	}
	if len(specified.TLS.ExternalCertificateSecretName) > 0 {
		b.route.Spec.TLS.ExternalCertificate = &openshift.LocalObjectReference{
			Name: specified.TLS.ExternalCertificateSecretName,
		}
	}
}

func init() {
	registerConfig("route.yaml", BackstageRouteFactory{}, false)
}

// implementation of RuntimeObject interface
func (b *BackstageRoute) Object() runtime.Object {
	return b.route
}

func (b *BackstageRoute) setObject(obj runtime.Object) {
	b.route = nil
	if obj != nil {
		b.route = obj.(*openshift.Route)
	}
}

// implementation of RuntimeObject interface
func (b *BackstageRoute) EmptyObject() client.Object {
	return &openshift.Route{}
}

// implementation of RuntimeObject interface
func (b *BackstageRoute) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {

	// not Openshift
	if !model.isOpenshift {
		return false, nil
	}

	// route explicitly disabled
	if !backstage.Spec.IsRouteEnabled() {
		return false, nil
	}

	specDefined := backstage.Spec.Application != nil && backstage.Spec.Application.Route != nil

	// no default route and not defined
	if b.route == nil && !specDefined {
		return false, nil
	}

	// no default route but defined in the spec -> create default
	if b.route == nil {
		b.route = &openshift.Route{}
	}

	// merge with specified (pieces) if any
	if specDefined {
		b.setRoute(backstage.Spec.Application.Route)
	}

	model.route = b
	model.setRuntimeObject(b)

	return true, nil
}

// implementation of RuntimeObject interface
func (b *BackstageRoute) updateAndValidate(model *BackstageModel, _ bsv1.Backstage) error {
	b.route.Spec.To.Name = model.backstageService.service.Name
	return nil
}

func (b *BackstageRoute) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.route.SetName(RouteName(backstage.Name))
	setMetaInfo(b.route, backstage, scheme)
}

// buildOpenShiftBaseUrl returns the base URL that should be considered as default on OpenShift,
// per the cluster ingress domain and the Route spec.
func buildOpenShiftBaseUrl(model *BackstageModel, backstage bsv1.Backstage) string {
	if !model.isOpenshift {
		return ""
	}
	if !backstage.Spec.IsRouteEnabled() {
		return ""
	}
	host := fmt.Sprintf("%s-%s", RouteName(backstage.Name), backstage.Namespace)
	appendIngressDomain := true
	if backstage.Spec.Application != nil && backstage.Spec.Application.Route != nil {
		// Per the Route spec, if a user specifies both the host and subdomain, the host takes precedence.
		if backstage.Spec.Application.Route.Host != "" {
			host = backstage.Spec.Application.Route.Host
			appendIngressDomain = false
		} else if backstage.Spec.Application.Route.Subdomain != "" {
			host = backstage.Spec.Application.Route.Subdomain
		}
	}
	if appendIngressDomain {
		d := model.ExternalConfig.OpenShiftIngressDomain
		if d == "" {
			return ""
		}
		host = fmt.Sprintf("%s.%s", host, d)
	}
	return fmt.Sprintf("https://%s", host)
}
