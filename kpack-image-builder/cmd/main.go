package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/registry"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
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
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var cpKubeConfig string
	var builderServiceAccount string
	var cfRootNamespace string
	var clusterBuilderName string
	var containerRepositoryPrefix string
	var buildCacheMb int64
	var memoryMb int64
	var diskMb int64
	var builderReadinessTimeout time.Duration

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&cpKubeConfig, "cp-kube-config", "", "Path to the KUBECONFIG for the Korifi cluster.")
	//TODO: Use a configMap instead of the comman line arguments
	flag.DurationVar(&builderReadinessTimeout, "builder-readiness-timeout", time.Second*30, "Builder readiness timeout")
	flag.StringVar(&builderServiceAccount, "builder-service-account", "kpack-service-account", "Builder Service Account")
	flag.StringVar(&cfRootNamespace, "cf-root-namespace", "cf", "CF root namespace")
	flag.StringVar(&clusterBuilderName, "cluster-builder-name", "cf-kpack-cluster-builder", "ClusterBuilder name")
	flag.StringVar(&containerRepositoryPrefix, "container-repository-prefix", "", "Container repository prefix")
	flag.Int64Var(&buildCacheMb, "build-cache-mb", 2048, "ClusterBuilder name")
	flag.Int64Var(&memoryMb, "memory-mb", 0, "ClusterBuilder name")
	flag.Int64Var(&diskMb, "disk-mb", 0, "ClusterBuilder name")
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
		LeaderElectionID:       "13w500bs.cloudfoundry.org",
	})
	if err != nil {
		setupLog.Error(err, "unable to initialize manager")
		os.Exit(1)
	}

	localConf := ctrl.GetConfigOrDie()
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

	imageClientSet, err := k8sclient.NewForConfig(localConf)
	if err != nil {
		panic(fmt.Sprintf("could not create k8s client: %v", err))
	}

	imageClient := image.NewClient(imageClientSet)
	if err = controllers.NewBuildWorkloadReconciler(
		mgr.GetClient(),
		localClient,
		localCache,
		localRESTMapper,
		mgr.GetScheme(),
		controllersLog,
		builderServiceAccount,
		clusterBuilderName,
		buildCacheMb,
		memoryMb,
		diskMb,
		imageClient,
		containerRepositoryPrefix,
		registry.NewRepositoryCreator(""),
		builderReadinessTimeout,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BuildWorkload")
		os.Exit(1)
	}

	if err = controllers.NewBuilderInfoReconciler(
		mgr.GetClient(),
		localClient,
		localCache,
		localRESTMapper,
		mgr.GetScheme(),
		controllersLog,
		clusterBuilderName,
		cfRootNamespace,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BuilderInfo")
		os.Exit(1)
	}

	// TODO: contribute images cleanup to the kpack directly
	//
	// if err = controllers.NewKpackBuildController(
	// 	mgr.GetClient(),
	// 	controllersLog,
	// 	imageClient,
	// 	controllerConfig.BuilderServiceAccount,
	// ).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create controller", "controller", "KpackBuild")
	// 	os.Exit(1)
	// }

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
