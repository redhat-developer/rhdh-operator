package main

import (
	"crypto/tls"
	"flag"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth" // Load auth plugins for GCP, Azure, etc.

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/internal/controller"

	openshift "github.com/openshift/api/route/v1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(bsv1.AddToScheme(scheme))
	utilruntime.Must(openshift.Install(scheme))
}

func main() {
	var (
		metricsAddr         string
		enableLeaderElection bool
		probeAddr           string
		secureMetrics       bool
		enableHTTP2         bool
	)

	// CLI flags
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080",
		"Address to bind metrics endpoint. Use 0 to disable metrics.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"Address to bind liveness/readiness probes.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election to ensure a single active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"Serve metrics via HTTPS with authentication and authorization (recommended for production).")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"Enable HTTP/2 (defaults to off due to known vulnerabilities).")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if metricsAddr != "0" && !secureMetrics {
		setupLog.Info("Serving metrics over plaintext HTTP (for development only).")
	}

	// HTTP/2 mitigation
	var tlsOpts []func(*tls.Config)
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			setupLog.Info("Disabling HTTP/2 for security reasons")
			c.NextProtos = []string{"http/1.1"}
		})
	}

	// Configure metrics server
	metricsServerOpts := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		setupLog.Info("Metrics will be served securely with RBAC authorization.")
		metricsServerOpts.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Initialize manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOpts,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "06bdbdd5.rhdh.redhat.com",
	})
	if err != nil {
		setupLog.Error(err, "Unable to start controller manager")
		os.Exit(1)
	}

	// Platform detection
	plf, err := controller.DetectPlatform()
	if err != nil {
		setupLog.Error(err, "Failed to detect platform. Is the cluster running and accessible?")
		os.Exit(1)
	}

	// Setup controller
	if err := (&controller.BackstageReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Platform: plf,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to initialize Backstage controller")
		os.Exit(1)
	}

	// Health & readiness
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to register health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to register readiness check")
		os.Exit(1)
	}

	// Success log
	setupLog.Info("RHDH Operator Manager started",
		"env.LOCALBIN", os.Getenv("LOCALBIN"),
		"platform", plf.Name,
		"leaderElection", enableLeaderElection,
		"metricsSecure", secureMetrics,
	)

	// Start manager
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Controller manager exited with error")
		os.Exit(1)
	}
}
