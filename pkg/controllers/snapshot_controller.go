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
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
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
	manager velero.Manager
	scheme  *runtime.Scheme
	handler handlers.Interface
}

func NewSnapshotController(c client.Client, factory k8sclient.Factory, bcm velero.Manager, schema *runtime.Scheme, handler handlers.Interface) *SnapshotReconciler {
	return &SnapshotReconciler{Client: c,
		factory: factory,
		manager: bcm,
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

func (r *SnapshotReconciler) handleUpdateSysBackup(sb *sysapiv1.Snapshot) {
	var (
		name = sb.Name
		// ctx  = context.Background()

		// err error
	)

	log.Infof("waiting for velero and middleware backup completed")
	// if sb.Spec.MiddleWarePhase == nil ||
	// 	!util.ListContains([]string{velero.Succeed, velero.Success}, *sb.Spec.MiddleWarePhase) {
	// 	log.Infof("velero or middleware backup not ready")
	// 	return
	// }

	log.Debugf("starting async to backup %q osdata", name)

	// go r.manager.AsyncOsDataBackup(name)
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

func (r *SnapshotReconciler) setSnapshotPhase(backupName string, snapshot *v1.Snapshot, phase constant.Phase) error {
	return r.handler.GetSnapshotHandler().SetSnapshotPhase(backupName, snapshot, phase)
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

				log.Infof("hit snapshot create event %s %s", snapshot.Name, *snapshot.Spec.Phase)
				backup, err := r.getBackup(snapshot.Spec.BackupId)
				if err != nil {
					log.Errorf("get backup error %v, backupId: %s", err, snapshot.Spec.BackupId)
					return false
				}

				// If the computer restarts and causes the snapshot to remain incomplete, its status will be changed from "Running" to "Failed."
				// This prevents a large number of "Running" tasks from triggering backup actions after the computer restarts.

				var phase = *snapshot.Spec.Phase

				switch phase {
				case constant.Completed.String(), constant.Failed.String():
					return false
				case constant.Running.String():
					// It is necessary to check whether the restic snapshot was successful and fix the CRD data.
					// The snapshotID from the CRD needs to be passed to the restic backend and associated with the restic snapshot information.
					if err := r.handler.GetSnapshotHandler().SetSnapshotPhase(backup.Name, snapshot, constant.Failed); err != nil {
						log.Errorf("update backup %s snapshot %s phase running error %v", backup.Name, snapshot.Name, err)
					}
				case constant.Pending.String():
					log.Infof("add to backup worker %s", snapshot.Name)
					worker.Worker.AppendBackupTask(fmt.Sprintf("%s_%s", snapshot.Spec.BackupId, snapshot.Name))
				}

				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				log.Info("hit snapshot update event")

				// todo need update backup.spec.Size

				oldObj, newObj := updateEvent.ObjectOld, updateEvent.ObjectNew
				oldSnapshot, ok1 := r.isSysSnapshot(oldObj)
				newSnapshot, ok2 := r.isSysSnapshot(newObj)

				fmt.Println("---old---", util.ToJSON(oldSnapshot))
				fmt.Println("---new---", util.ToJSON(newSnapshot))
				return false

				if !(ok1 && ok2) || reflect.DeepEqual(oldSnapshot.Spec, newSnapshot.Spec) {
					return false
				}

				backup, err := r.getBackup(newSnapshot.Spec.BackupId)
				if err != nil {
					log.Errorf("get backup error %v", err)
					return false
				}

				var backupName = backup.Spec.Name
				var snapshotCreateAt = r.handler.GetSnapshotHandler().ParseSnapshotName(newSnapshot.Spec.StartAt)

				if util.ListContains([]string{constant.Completed.String(), constant.Failed.String()}, *newSnapshot.Spec.Phase) {
					log.Infof("backup: %s, snapshot: %s, phase: %s", backupName, snapshotCreateAt, *newSnapshot.Spec.Phase)
					return false
				}

				log.Infof("run backup: %s, snapshot: %s", backupName, snapshotCreateAt)

				// worker.Worker.AppendSnapshotTask(newSnapshot.Name)

				// if err := r.snapshotOperator.Backup(backup, newSnapshot); err != nil {
				// 	log.Errorf("backup %s snapshot error %v", backup.Name, err)
				// }

				// resticPhase := b.Spec.ResticPhase
				// if resticPhase != nil {
				// 	// restic backup running
				// 	return false
				// }

				// oldPhase, newPhase := a.Spec.Phase, b.Spec.Phase
				// oldMWPhase, newMWPhase := a.Spec.MiddleWarePhase, b.Spec.MiddleWarePhase

				// if (oldPhase != nil && newPhase != nil && *oldPhase == *newPhase) &&
				// 	(oldMWPhase != nil && newMWPhase != nil && *oldMWPhase == *newMWPhase) {
				// 	return false
				// }

				// if velero.VeleroBackupCompleted == *newPhase {
				// 	log.Debugf("backup %q received %q event", b.Name, *newPhase)
				// 	r.handleUpdateSysBackup(b)
				// }
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				log.Info("hit snapshot delete event")
				// sb, ok := r.isSysBackup(e.Object)
				// if !ok {
				// 	return false
				// }
				// r.handleDeleteBackup(sb.Name, sb)
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
	// if b.Namespace != velero.DefaultVeleroNamespace {
	// 	return nil, false
	// }
	// // if b.Spec.Owner == nil || b.Spec.Phase == nil || b.Spec.Extra == nil {
	// // 	return nil, false
	// // }
	// if _, ok = b.Spec.Extra[velero.ExtraBackupType]; !ok {
	// 	return nil, false
	// }
	// if _, ok = b.Spec.Extra[velero.ExtraBackupStorageLocation]; !ok {
	// 	return nil, false
	// }
	// if _, ok = b.Spec.Extra[velero.ExtraRetainDays]; !ok {
	// 	return nil, false
	// }

	return b, true
}

func (r *SnapshotReconciler) getBackup(backupId string) (*v1.Backup, error) {
	var ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	backup, err := r.handler.GetBackupHandler().GetBackupById(ctx, backupId)
	if err != nil {
		return nil, err
	}

	return backup, nil
}
