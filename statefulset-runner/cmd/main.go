package main

import (
	"flag"
	"fmt"
	"os"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	statefulsetcontrollers "code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/tools"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var cpKubeConfig string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&cpKubeConfig, "cp-kube-config", "", "Path to the KUBECONFIG for the Korifi cluster.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	logger, _, err := tools.NewZapLogger(zapcore.InfoLevel)
	if err != nil {
		panic(fmt.Sprintf("error creating new zap logger: %v", err))
	}

	ctrl.SetLogger(logger)
	klog.SetLogger(ctrl.Log)

	remoteConf, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&clientcmd.ClientConfigLoadingRules{ExplicitPath: cpKubeConfig}, nil).ClientConfig()
	if err != nil {
		setupLog.Error(err, "unable to initialize control-plane cluster config")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(remoteConf, ctrl.Options{
		Scheme: scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "13c200bs.cloudfoundry.org",
	})
	if err != nil {
		setupLog.Error(err, "unable to initialize manager")
		os.Exit(1)
	}

	localConf := ctrl.GetConfigOrDie()
	setupLog.Info("creating control-plane cluster object", "server", localConf.String())
	localCluster, err := cluster.New(localConf, func(o *cluster.Options) {
		o.Scheme = scheme
		o.Logger = ctrl.Log.WithName("local-cluster")
	})
	if err != nil {
		setupLog.Error(err, "unable to initialize control-plane cluster")
		os.Exit(1)
	}

	localCache := localCluster.GetCache()
	localClient := localCluster.GetClient()
	localRESTMapper := localCluster.GetRESTMapper()

	mgr.Add(localCluster)

	controllersLog := ctrl.Log.WithName("controllers")
	if err = statefulsetcontrollers.NewAppWorkloadReconciler(
		mgr.GetClient(),
		localClient,
		localCache,
		localRESTMapper,
		mgr.GetScheme(),
		statefulsetcontrollers.NewAppWorkloadToStatefulsetConverter(
			scheme,
			false,
		),
		statefulsetcontrollers.NewPDBUpdater(localClient),
		controllersLog,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AppWorkload")
		os.Exit(1)
	}

	if err = statefulsetcontrollers.NewRunnerInfoReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		controllersLog,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RunnerInfo")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
