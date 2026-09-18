package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	demov1alpha1 "controller/api/v1alpha1"
	"controller/internal/controller"
	"controller/internal/provisioner"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(demov1alpha1.AddToScheme(scheme))
}

func main() {
	provisionerURL := flag.String("provisioner-url", "http://localhost:8080", "base URL of the provisioning API")
	flag.Parse()

	ctrl.SetLogger(zap.New())

	shutdownTimeout := 35 * time.Second
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		GracefulShutdownTimeout: &shutdownTimeout,
	})
	if err != nil {
		setupLog.Error(err, "failed to start manager")
		os.Exit(1)
	}

	if err := (&controller.ManagedDatabaseReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Provisioner: provisioner.NewClient(*provisionerURL),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "failed to create controller")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "manager exited with error")
		os.Exit(1)
	}
}
