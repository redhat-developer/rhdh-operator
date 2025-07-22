package model

import (
	"fmt"

	"k8s.io/klog/v2"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

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
func (b *BackstageRoute) updateAndValidate(model *BackstageModel, backstage bsv1.Backstage) error {
	b.route.Spec.To.Name = model.backstageService.service.Name
	b.updateAppConfigWithBaseUrls(model, backstage)
	return nil
}

func (b *BackstageRoute) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.route.SetName(RouteName(backstage.Name))
	setMetaInfo(b.route, backstage, scheme)
}

// updateAppConfigWithBaseUrls tries to set the baseUrl in the default app-config.
// Note that this is purposely done on a best effort basis. So it is not considered an issue if the cluster ingress domain
// could not be determined, since the user can always set it explicitly in their custom app-config.
func (b *BackstageRoute) updateAppConfigWithBaseUrls(m *BackstageModel, backstage bsv1.Backstage) {
	if m.appConfig == nil || m.appConfig.ConfigMap == nil {
		klog.V(1).Infof(
			"Default app-config ConfigMap not initialized yet - skipping automatic population of base URLS in the default app-config for Backstage %s",
			backstage.Name)
		return
	}

	baseUrl := buildBaseUrl(m, backstage)
	updateFn := func(content string) (string, error) {
		var appConfigData map[string]any
		err := yaml.Unmarshal([]byte(content), &appConfigData)
		if err != nil {
			return "", fmt.Errorf("failed to decode app-config YAML: %w", err)
		}
		if appConfigData == nil {
			appConfigData = make(map[string]any)
		}
		app, ok := appConfigData["app"].(map[string]any)
		if !ok {
			app = make(map[string]any)
			appConfigData["app"] = app
		}
		app["baseUrl"] = baseUrl

		backend, ok := appConfigData["backend"].(map[string]any)
		if !ok {
			backend = make(map[string]any)
			appConfigData["backend"] = backend
		}
		backend["baseUrl"] = baseUrl

		backendCors, ok := backend["cors"].(map[string]any)
		if !ok {
			backendCors = make(map[string]any)
			backend["cors"] = backendCors
		}
		backendCors["origin"] = baseUrl

		updated, err := yaml.Marshal(&appConfigData)
		if err != nil {
			return "", fmt.Errorf("failed to serialize updated app-config YAML: %w", err)
		}
		return string(updated), nil
	}

	for k, v := range m.appConfig.ConfigMap.Data {
		updated, err := updateFn(v)
		if err != nil {
			klog.V(1).Infof("[warn] could not update base url in default app-config %q for backstage %s: %v",
				k, backstage.Name, err)
			continue
		}
		m.appConfig.ConfigMap.Data[k] = updated
	}
}

// buildBaseUrl returns the base URL that should be considered as default on OpenShift,
// per the cluster ingress domain and the Route spec.
func buildBaseUrl(model *BackstageModel, backstage bsv1.Backstage) string {
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
