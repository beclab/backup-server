package controllers

import (
	"context"
	"fmt"
	"reflect"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	k8sclient "bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/notify"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/worker"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// BackupReconciler reconciles a Backup object
type BackupReconciler struct {
	client.Client
	factory k8sclient.Factory
	scheme  *runtime.Scheme
	handler handlers.Interface
}

func NewBackupController(c client.Client, factory k8sclient.Factory, schema *runtime.Scheme, handler handlers.Interface) *BackupReconciler {
	return &BackupReconciler{
		Client:  c,
		factory: factory,
		scheme:  schema,
		handler: handler,
	}
}

//+kubebuilder:rbac:groups=sys.bytetrade.i,resources=backup,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=backup/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=backup/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Backup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Infof("received backup request, namespace: %q, name: %q", req.Namespace, req.Name)

	c, err := r.factory.Sysv1Client()
	if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 3 * time.Second}, errors.WithStack(err)
	}

	backup, err := c.SysV1().Backups(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		return ctrl.Result{Requeue: true, RequeueAfter: 2 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, errors.WithStack(err)
	}

	if !r.isNotified(backup) {
		err = r.notify(backup)
		if err != nil {
			log.Errorf("notify backupName: %s, backupId: %s, error: %v", backup.Spec.Name, backup.Name, err)
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}
		log.Infof("notify backupName: %s, backupId: %s, record success", backup.Spec.Name, backup.Name)
	}

	if !backup.Spec.Deleted && backup.Spec.BackupPolicy.Enabled {
		err = r.reconcileBackupPolicies(backup)
		if err != nil {
			return ctrl.Result{}, errors.WithStack(err)
		}
	} else {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&sysv1.Backup{}, builder.WithPredicates(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				log.Info("hit backup create event")
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				log.Info("hit backup update event")

				bc1, ok1 := e.ObjectOld.(*sysv1.Backup)
				bc2, ok2 := e.ObjectNew.(*sysv1.Backup)
				if !(ok1 || ok2) || reflect.DeepEqual(bc1.Spec, bc2.Spec) {
					log.Info("backup not changed")
					return false
				}

				if isNotifiedStateChanged(bc1, bc2) {
					log.Info("backup notify state changed: Notified")
					return false
				}

				if isDeleted(bc2) {
					return false
				}

				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				log.Info("hit backup delete event")

				backup, ok := e.Object.(*sysv1.Backup)
				if !ok {
					log.Errorf("delete backup error, not a backup resource")
					return false
				}

				if err := worker.Worker.CancelBackup(backup.Name, ""); err != nil {
					log.Errorf("cancel backup %s error: %v", backup.Name, err)
				}

				return false
			}})).
		Build(r)
	if err != nil {
		return err
	}

	return nil
}

func (r *BackupReconciler) isNotified(backup *sysv1.Backup) bool {
	return backup.Spec.Notified
}

func isNotifiedStateChanged(oldBackup *sysv1.Backup, newBackup *sysv1.Backup) bool {
	if oldBackup.Spec.Notified != newBackup.Spec.Notified {
		return true
	}
	return false
}

func isDeleted(newBackup *sysv1.Backup) bool {
	return newBackup.Spec.Deleted
}

func (r *BackupReconciler) reconcileBackupPolicies(backup *sysv1.Backup) error {
	ctx := context.Background()
	if backup.Spec.BackupPolicy != nil {
		cron, _ := util.ParseToCron(backup.Spec.BackupPolicy.SnapshotFrequency, backup.Spec.BackupPolicy.TimesOfDay, backup.Spec.BackupPolicy.DayOfWeek)
		err := r.handler.GetSnapshotHandler().CreateSchedule(ctx, backup, cron, !backup.Spec.BackupPolicy.Enabled)
		if err != nil {
			return err
		}
		log.Debugf("schedule %q created: %q", backup.Spec.Name, cron)
	}
	return nil
}

func (r *BackupReconciler) notify(backup *sysv1.Backup) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	integrationName := handlers.GetBackupIntegrationName(constant.BackupLocationSpace.String(), backup.Spec.Location)
	if integrationName == "" {
		return fmt.Errorf("space integrationName not exists, config: %s", util.ToJSON(backup.Spec.Location))
	}
	olaresSpaceToken, err := integration.IntegrationManager().GetIntegrationSpaceToken(ctx, backup.Spec.Owner, integrationName)
	if err != nil {
		return err
	}

	locationConfig, err := handlers.GetBackupLocationConfig(backup)
	if err != nil {
		return fmt.Errorf("get backup location config error: %v", err)
	}
	if locationConfig == nil {
		return fmt.Errorf("backup location config not exists")
	}
	var location = locationConfig["location"]

	var notifyBackupObj = &notify.Backup{
		UserId:         olaresSpaceToken.OlaresDid,
		Token:          olaresSpaceToken.AccessToken,
		BackupId:       backup.Name,
		Name:           backup.Spec.Name,
		BackupPath:     handlers.GetBackupPath(backup),
		BackupLocation: location,
	}

	if err := notify.NotifyBackup(ctx, constant.DefaultSyncServerURL, notifyBackupObj); err != nil {
		return fmt.Errorf("notify backup obj error: %v", err)
	}

	return r.handler.GetBackupHandler().UpdateNotifyState(ctx, backup.Name, true)
}
