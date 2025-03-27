package controllers

import (
	"context"
	"fmt"
	"reflect"
	"time"

	sysapiv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	v1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	k8sclient "bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/notify"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/worker"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// BackupReconciler reconciles a BackupConfig object
type SnapshotReconciler struct {
	client.Client
	factory k8sclient.Factory
	scheme  *runtime.Scheme
	handler handlers.Interface
}

func NewSnapshotController(c client.Client, factory k8sclient.Factory, schema *runtime.Scheme, handler handlers.Interface) *SnapshotReconciler {
	return &SnapshotReconciler{Client: c,
		factory: factory,
		scheme:  schema,
		handler: handler,
	}
}

//+kubebuilder:rbac:groups=sys.bytetrade.i,resources=snapshot,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=snapshot/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=snapshot/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Snapshot object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *SnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *SnapshotReconciler) handleDeleteBackup(name string, sb *sysapiv1.Backup) {
	var (
	// ctx = context.Background()
	)

	var backupType string

	log.Debugf("deleting backup %q, type: %s", name, backupType)

	// delete backup
	// if err := r.manager.DeleteBackup(ctx, name, sb); err != nil && !apierrors.IsNotFound(err) {
	// 	log.Errorf("failed to delete backup %q: %v", name, err)
	// 	return
	// }

	log.Debugf("successfully to delete backup %q", name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&sysapiv1.Snapshot{}, builder.WithPredicates(predicate.Funcs{
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				log.Info("hit snapshot create event")
				// Pending,Running Failed Complete

				snapshot, ok := r.isSysSnapshot(e.Object)
				if !ok {
					log.Debugf("not a snapshot resource")
					return false
				}

				log.Infof("hit snapshot create event %s, %s", snapshot.Name, *snapshot.Spec.Phase)
				backup, err := r.getBackup(snapshot.Spec.BackupId)
				if err != nil {
					log.Errorf("get backup error %v, backupId: %s", err, snapshot.Spec.BackupId)
					return false
				}

				// If the computer restarts and causes the snapshot to remain incomplete, its status will be changed from "Running" to "Failed."
				// This prevents a large number of "Running" tasks from triggering backup actions after the computer restarts.

				var phase = *snapshot.Spec.Phase

				switch phase {
				case constant.Completed.String(), constant.Failed.String(), constant.Canceled.String():
					return false
				case constant.Running.String():
					// It is necessary to check whether the restic snapshot was successful and fix the CRD data.
					// The snapshotID from the CRD needs to be passed to the restic backend and associated with the restic snapshot information.
					if err := r.handler.GetSnapshotHandler().UpdatePhase(context.Background(), snapshot.Name, constant.Failed.String()); err != nil {
						log.Errorf("update backup %s snapshot %s phase Failed error %v", backup.Spec.Name, snapshot.Name, err)
					}
					if err := r.notifySnapshot(backup, snapshot, constant.Failed.String()); err != nil {
						log.Errorf("notify backup %s snapshot %s Failed error: %v", backup.Spec.Name, snapshot.Name, err)
					} else {
						log.Infof("notify backup %s snapshot %s Failed", backup.Spec.Name, snapshot.Name)
					}
				case constant.Pending.String():
					if err := r.notifySnapshot(backup, snapshot, constant.Pending.String()); err != nil {
						log.Errorf("notify backup %s snapshot %s New error: %v", backup.Spec.Name, snapshot.Name, err)
					} else {
						log.Infof("notify backup %s snapshot %s New", backup.Spec.Name, snapshot.Name)
					}
					log.Infof("add to backup worker %s", snapshot.Name)
					worker.Worker.AppendBackupTask(fmt.Sprintf("%s_%s_%s", backup.Spec.Owner, snapshot.Spec.BackupId, snapshot.Name))
				}

				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				log.Info("hit snapshot update event")

				// todo need update backup.spec.Size

				oldObj, newObj := updateEvent.ObjectOld, updateEvent.ObjectNew
				oldSnapshot, ok1 := r.isSysSnapshot(oldObj)
				newSnapshot, ok2 := r.isSysSnapshot(newObj)

				if !(ok1 && ok2) || reflect.DeepEqual(oldSnapshot.Spec, newSnapshot.Spec) {
					return false
				}

				backup, err := r.getBackup(newSnapshot.Spec.BackupId)
				if err != nil {
					log.Errorf("get backup error %v", err)
					return false
				}

				log.Infof("snapshot update event, id: %s, phase: %s", newSnapshot.Name, *newSnapshot.Spec.Phase)
				log.Infof("snapshot update event old: %s, new: %s", util.ToJSON(oldSnapshot), util.ToJSON(newSnapshot))

				if constant.Failed.String() == *newSnapshot.Spec.Phase {
					return false
				}

				var notifyState string
				if r.isRunning(oldSnapshot, newSnapshot) {
					notifyState = constant.Running.String()
				} else if r.isCanceled(newSnapshot) {
					notifyState = constant.Canceled.String()
				}

				if notifyState == "" {
					log.Infof("snapshot state is %s, skip notify", *newSnapshot.Spec.Phase)
					return false
				}

				log.Infof("snapshot notify state changed: %s", notifyState)
				if err := r.notifySnapshot(backup, newSnapshot, notifyState); err != nil {
					log.Errorf("notify backup %s snapshot %s %s error: %v", backup.Spec.Name, newSnapshot.Name, notifyState, err)
				} else {
					log.Infof("notify backup %s snapshot %s %s", backup.Spec.Name, newSnapshot.Name, notifyState)
				}

				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				log.Info("hit snapshot delete event")
				return false
			},
		})).Build(r)
	if err != nil {
		return err
	}

	return nil
}

func (r *SnapshotReconciler) isSysSnapshot(obj client.Object) (*sysapiv1.Snapshot, bool) {
	b, ok := obj.(*sysapiv1.Snapshot)
	if !ok || b == nil {
		return nil, false
	}

	return b, true
}

func (r *SnapshotReconciler) getBackup(backupId string) (*v1.Backup, error) {
	var ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	backup, err := r.handler.GetBackupHandler().GetById(ctx, backupId)
	if err != nil {
		return nil, err
	}

	return backup, nil
}

func (r *SnapshotReconciler) isRunning(oldSnapshot *v1.Snapshot, newSnapshot *v1.Snapshot) bool {
	if *oldSnapshot.Spec.Phase == constant.Pending.String() && *newSnapshot.Spec.Phase == constant.Running.String() {
		return true
	}
	return false
}

func (r *SnapshotReconciler) isCanceled(newSnapshot *v1.Snapshot) bool {
	newPhase := *newSnapshot.Spec.Phase
	return newPhase == constant.Canceled.String()
}

func (r *SnapshotReconciler) notifySnapshot(backup *v1.Backup, snapshot *v1.Snapshot, status string) error {
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

	var snapshotRecord = &notify.Snapshot{
		UserId:       olaresSpaceToken.OlaresDid,
		BackupId:     backup.Name,
		SnapshotId:   snapshot.Name,
		Size:         0,
		Unit:         constant.DefaultSnapshotSizeUnit,
		SnapshotTime: snapshot.Spec.StartAt,
		Status:       status,
		Type:         handlers.ParseSnapshotTypeText(snapshot.Spec.SnapshotType),
	}

	if err := notify.NotifySnapshot(ctx, constant.DefaultSyncServerURL, snapshotRecord); err != nil {
		return err
	}

	return nil
}
