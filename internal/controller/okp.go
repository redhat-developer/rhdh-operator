package controller

import (
	"context"
	"fmt"

	openshift "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

// okpComponentName is the shared name/label value for OKP resources.
const okpComponentName = "lightspeed-okp"

func okpName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, okpComponentName)
}

func okpLabels(backstageName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      okpComponentName,
		"app.kubernetes.io/instance":  backstageName,
		"app.kubernetes.io/component": okpComponentName,
	}
}

func okpSelectorLabels(backstageName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     okpComponentName,
		"app.kubernetes.io/instance": backstageName,
	}
}

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
func (r *BackstageReconciler) prepareOkpEnvVar(backstage *api.Backstage, bsModel *model.BackstageModel) {
	if !model.IsFlavourEnabled(backstage.Spec, "lightspeed") || !r.Platform.IsOpenshift() {
		return
	}

	name := okpName(backstage.Name)
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

func (r *BackstageReconciler) applyOkpResources(ctx context.Context, backstage *api.Backstage, bsModel *model.BackstageModel) error {
	lg := log.FromContext(ctx).WithValues("Backstage", backstage.Name)

	if !model.IsFlavourEnabled(backstage.Spec, "lightspeed") || !r.Platform.IsOpenshift() {
		return nil
	}

	name := okpName(backstage.Name)
	ns := backstage.Namespace
	labels := okpLabels(backstage.Name)
	selectorLabels := okpSelectorLabels(backstage.Name)

	// OKP Deployment
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selectorLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "okp",
							Image:           "registry.redhat.io/offline-knowledge-portal/rhokp-rhel9:1.2.10-1786628394",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{Name: "httpd", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
								{Name: "solr", ContainerPort: 8983, Protocol: corev1.ProtocolTCP},
							},
							Env: []corev1.EnvVar{
								{Name: "SOLR_JAVA_MEM", Value: "-Xms1g -Xmx1g"},
								{Name: "SOLR_HOST_BIND", Value: "0.0.0.0"},
								{Name: "HTTPD_SERVER_NAME", Value: "localhost"},
								{Name: "HTTPD_COMPRESSED", Value: "true"},
								{Name: "HTTPD_ENCRYPT", Value: "false"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(backstage, deployment, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on OKP Deployment: %w", err)
	}
	if err := r.Patch(ctx, deployment, client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil { //nolint:staticcheck
		return fmt.Errorf("failed to apply OKP Deployment: %w", err)
	}
	lg.Info("OKP Deployment applied", "name", name)

	// OKP Service
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{
				{Name: "httpd", Port: 8080, TargetPort: intstr.FromString("httpd"), Protocol: corev1.ProtocolTCP},
				{Name: "solr", Port: 8983, TargetPort: intstr.FromString("solr"), Protocol: corev1.ProtocolTCP},
			},
		},
	}

	if err := controllerutil.SetControllerReference(backstage, service, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on OKP Service: %w", err)
	}
	if err := r.Patch(ctx, service, client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil { //nolint:staticcheck
		return fmt.Errorf("failed to apply OKP Service: %w", err)
	}
	lg.Info("OKP Service applied", "name", name)

	// OKP Route (OpenShift only)
	if r.Platform.IsOpenshift() {
		route := &openshift.Route{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "route.openshift.io/v1",
				Kind:       "Route",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    labels,
			},
			Spec: openshift.RouteSpec{
				To: openshift.RouteTargetReference{
					Kind: "Service",
					Name: name,
				},
				Port: &openshift.RoutePort{
					TargetPort: intstr.FromString("httpd"),
				},
				TLS: &openshift.TLSConfig{
					Termination:                   openshift.TLSTerminationEdge,
					InsecureEdgeTerminationPolicy: openshift.InsecureEdgeTerminationPolicyAllow,
				},
			},
		}

		if err := controllerutil.SetControllerReference(backstage, route, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference on OKP Route: %w", err)
		}
		if err := r.Patch(ctx, route, client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil { //nolint:staticcheck
			return fmt.Errorf("failed to apply OKP Route: %w", err)
		}
		lg.Info("OKP Route applied", "name", name)
	}

	return nil
}
