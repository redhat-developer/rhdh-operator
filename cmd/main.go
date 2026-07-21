package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"
	"path/filepath"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha5"

	"github.com/redhat-developer/rhdh-operator/internal/controller"

	configv1 "github.com/openshift/api/config/v1"
	openshift "github.com/openshift/api/route/v1"
	tlspkg "github.com/openshift/controller-runtime-common/pkg/tls"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(bsv1.AddToScheme(scheme))

	utilruntime.Must(openshift.Install(scheme))

	utilruntime.Must(monitoringv1.AddToScheme(scheme))

	utilruntime.Must(configv1.Install(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableCacheLabelFilter bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS and requires authentication and authorization. "+
			"Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt",
		"The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableCacheLabelFilter, "enable-cache-label-filter", os.Getenv("ENABLE_CACHE_LABEL_FILTER") == "true",
		"If set, the cache will only store Secrets and ConfigMaps with the label 'rhdh.redhat.com/external-config=true'. This reduces memory consumption. Can also be set via ENABLE_CACHE_LABEL_FILTER env var.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Cancel this context when the TLS profile changes so the pod restarts with the new config.
	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())
	defer cancel()

	if metricsAddr != "0" && !secureMetrics {
		setupLog.Info("Metrics are served over plaintext HTTP. This is only intended for local development.")
	}

	restConfig := ctrl.GetConfigOrDie()

	// Fetch the TLS profile from apiservers.config.openshift.io/cluster.
	// Fall back to Intermediate on non-OpenShift clusters (or if the fetch fails).
	tlsSecurityProfileSpec, err := tlspkg.GetTLSProfileSpec(nil)
	if err != nil {
		setupLog.Error(err, "unable to get default TLS profile")
		os.Exit(1)
	}
	tlsAdherence := configv1.TLSAdherencePolicyNoOpinion

	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Info("unable to create client for TLS profile fetch, using Intermediate fallback", "error", err)
	} else {
		if profile, fetchErr := tlspkg.FetchAPIServerTLSProfile(ctx, k8sClient); fetchErr != nil {
			setupLog.Info("unable to get TLS profile from API server, using Intermediate fallback", "error", fetchErr)
		} else {
			tlsSecurityProfileSpec = profile
		}
		if adherence, fetchErr := tlspkg.FetchAPIServerTLSAdherencePolicy(ctx, k8sClient); fetchErr != nil {
			setupLog.Info("unable to get TLS adherence policy from API server", "error", fetchErr)
		} else {
			tlsAdherence = adherence
		}
	}

	tlsConfig, unsupportedCiphers := tlspkg.NewTLSConfigFromProfile(tlsSecurityProfileSpec)
	if len(unsupportedCiphers) > 0 {
		setupLog.Info("TLS profile contains ciphers unsupported by Go that will be ignored", "ciphers", unsupportedCiphers)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){tlsConfig}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(webhookCertPath, webhookCertName),
			filepath.Join(webhookCertPath, webhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: webhookTLSOpts,
	})

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize metrics certificate watcher")
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	mgrOpts := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "06bdbdd5.rhdh.redhat.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	}

	// Strip useless managedFields from all cached objects to reduce memory usage.
	// With many resources, managedFields can consume 70%+ of the informer cache heap.
	mgrOpts.Cache.DefaultTransform = cache.TransformStripManagedFields()

	// Configure cache to only watch labeled Secrets and ConfigMaps if flag is enabled
	if enableCacheLabelFilter {
		setupLog.Info("Enabling cache label filter for Secrets and ConfigMaps")
		labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{
				"rhdh.redhat.com/external-config": "true",
			},
		})
		if err != nil {
			setupLog.Error(err, "failed to create label selector")
			os.Exit(1)
		}

		mgrOpts.Cache.ByObject = map[client.Object]cache.ByObject{
			&corev1.Secret{}: {
				Label: labelSelector,
			},
			&corev1.ConfigMap{}: {
				Label: labelSelector,
			},
		}
	}

	mgr, err := ctrl.NewManager(restConfig, mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	plf, err := controller.DetectPlatform()
	if err != nil {
		setupLog.Error(err, "unable to detect platform. Make sure your cluster is running and accessible")
		os.Exit(1)
	}

	if err = (&controller.BackstageReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Platform: plf,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Backstage")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	// Watch for TLS profile changes and restart so the new config is applied.
	if plf.IsOpenshift() {
		if err := (&tlspkg.SecurityProfileWatcher{
			Client:                    mgr.GetClient(),
			InitialTLSProfileSpec:     tlsSecurityProfileSpec,
			InitialTLSAdherencePolicy: tlsAdherence,
			OnProfileChange: func(_ context.Context, oldTLSProfileSpec, newTLSProfileSpec configv1.TLSProfileSpec) {
				setupLog.Info("TLS profile changed, shutting down to reload",
					"old", oldTLSProfileSpec, "new", newTLSProfileSpec)
				cancel()
			},
			OnAdherencePolicyChange: func(_ context.Context, oldTLSAdherencePolicy, newTLSAdherencePolicy configv1.TLSAdherencePolicy) {
				setupLog.Info("TLS adherence policy changed, shutting down to reload",
					"old", oldTLSAdherencePolicy, "new", newTLSAdherencePolicy)
				cancel()
			},
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create TLS security profile watcher")
			os.Exit(1)
		}
	} else {
		setupLog.Info("TLS profile watcher is not supported on non-OpenShift clusters")
	}

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager with parameters: ",
		"env.LOCALBIN", os.Getenv("LOCALBIN"),
		"platform", plf.Name,
	)
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
