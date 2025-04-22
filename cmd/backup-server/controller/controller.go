package controller

import (
	"context"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/controllers"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/watchers"
	"bytetrade.io/web3os/backup-server/pkg/worker"
	"github.com/lithammer/dedent"
	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var logLevel string
var metricsAddr string
var enableLeaderElection bool
var probeAddr string

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sysv1.AddToScheme(scheme))

	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	//+kubebuilder:scaffold:scheme
}

func NewControllerCommand() *cobra.Command {
	cmd := cobra.Command{
		Use:   "controller",
		Short: "start controller",
		Long:  dedent.Dedent(`controller for backup and restore`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := Run(); err != nil {
				return err
			}
			return nil
		},
	}

	fs := cmd.PersistentFlags()
	addFlags(fs)

	return &cmd
}

func addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	fs.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	// fs.StringVarP(&constant.DefaultCloudApiMirror, "cloud-api-mirror", "", "https://cloud-dev-api.olares.xyz", "cloud API mirror")
	fs.StringVarP(&logLevel, "log-level", "l", "debug", "log level")

}

func Run() error {
	log.InitLog("debug")

	f, err := client.NewFactory()
	if err != nil {
		return err
	}

	return run(f)
}

func run(factory client.Factory) error {
	c, err := factory.ClientConfig()
	if err != nil {
		return err
	}

	mgr, err := ctrl.NewManager(c, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "326b4914.bytetrade.io",
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
	})
	if err != nil {
		return pkgerrors.Errorf("unable to start manager: %v", err)
	}

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return pkgerrors.Errorf("unable to set up health check: %v", err)
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return pkgerrors.Errorf("unable to setup ready check: %v", err)
	}

	integration.NewIntegrationManager(factory)

	notification := &watchers.Notification{
		Factory: factory,
	}

	var handler = handlers.NewHandler(factory, notification)

	worker.NewWorkerPool(context.TODO(), handler)

	enabledControllers := map[string]struct{}{
		controllers.BackupController:   {},
		controllers.SnapshotController: {},
		controllers.RestoreController:  {},
	}

	if _, ok := enabledControllers[controllers.BackupController]; ok {
		if err = controllers.NewBackupController(mgr.GetClient(), factory, mgr.GetScheme(), handler).
			SetupWithManager(mgr); err != nil {
			return pkgerrors.Errorf("unable to create backup controller: %v", err)
		}
	}

	if _, ok := enabledControllers[controllers.SnapshotController]; ok {
		if err = controllers.NewSnapshotController(mgr.GetClient(), factory, mgr.GetScheme(), handler).
			SetupWithManager(mgr); err != nil {
			return pkgerrors.Errorf("unable to create snapshot controller: %v", err)
		}
	}

	if _, ok := enabledControllers[controllers.RestoreController]; ok {
		if err = controllers.NewRestoreController(mgr.GetClient(), factory, mgr.GetScheme(), handler).
			SetupWithManager(mgr); err != nil {
			return pkgerrors.Errorf("unable to create restore controller: %v", err)
		}
	}

	log.Info("starting manager")

	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return pkgerrors.Errorf("start manager: %v", err)
	}
	return nil
}
