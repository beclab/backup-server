package controllers

import (
	"context"
	"reflect"
	"time"

	sysapiv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	v1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	k8sclient "bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	sysv1 "bytetrade.io/web3os/backup-server/pkg/generated/clientset/versioned"
	"bytetrade.io/web3os/backup-server/pkg/modules/backup/v1/operator"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
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

	backupOperator   *operator.BackupOperator
	snapshotOperator *operator.SnapshotOperator

	sc sysv1.Interface
}

func NewSnapshotController(c client.Client, factory k8sclient.Factory, bcm velero.Manager, schema *runtime.Scheme) *SnapshotReconciler {
	b := &SnapshotReconciler{Client: c,
		factory:          factory,
		manager:          bcm,
		scheme:           schema,
		backupOperator:   operator.NewBackupOperator(factory),
		snapshotOperator: operator.NewSnapshotOperator(factory),
	}

	sc, err := factory.Sysv1Client()
	if err != nil {
		panic(err)
	}

	b.sc = sc
	return b
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BackupConfig object against the actual cluster state, and then
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
		ctx  = context.Background()

		err error
	)

	log.Debugf("waiting for velero is available")
	if err = r.waitingAvailable(ctx); err != nil {
		log.Errorf("waiting for velero server available failed: %v", err)
		return
	}

	log.Infof("waiting for velero and middleware backup completed")
	// if sb.Spec.MiddleWarePhase == nil ||
	// 	!util.ListContains([]string{velero.Succeed, velero.Success}, *sb.Spec.MiddleWarePhase) {
	// 	log.Infof("velero or middleware backup not ready")
	// 	return
	// }

	log.Debugf("starting async to backup %q osdata", name)

	// go r.manager.AsyncOsDataBackup(name)
}

func (r *SnapshotReconciler) waitingAvailable(ctx context.Context) error {
	var observation int

	return wait.PollImmediate(time.Second, 5*time.Minute, func() (bool, error) {
		ok, err := r.manager.Available(ctx)
		if err != nil && apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		observation++

		if observation >= 3 {
			return true, nil
		}
		return false, nil
	})
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

func (r *SnapshotReconciler) setSnapshotPhase(backupName string, snapshot *v1.Snapshot, phase constant.SnapshotPhase) error {
	return r.snapshotOperator.SetSnapshotPhase(backupName, snapshot, phase)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&sysapiv1.Snapshot{}, builder.WithPredicates(predicate.Funcs{
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool { // Pending,Running Failed Complete
				log.Info("hit snapshot create event")
				snapshot, ok := r.isSysSnapshot(e.Object)
				if !ok {
					log.Debugf("not a sys snapshot")
					return false
				}

				backup, err := r.getBackup(snapshot.Spec.BackupId)
				if err != nil {
					log.Errorf("get backup error %v", err)
					return false
				}

				// If the computer restarts and causes the snapshot to remain incomplete, its status will be changed from "Running" to "Failed."
				// This prevents a large number of "Running" tasks from triggering backup actions after the computer restarts.

				var snapshotPhase = constant.SnapshotPhaseFailed
				var phase = *snapshot.Spec.Phase

				switch phase {
				case constant.SnapshotPhaseComplete.String(), constant.SnapshotPhaseFailed.String():
					return false
				case constant.SnapshotPhaseRunning.String():
					// todo
					// It is necessary to check whether the restic snapshot was successful and fix the CRD data.
					// The snapshotID from the CRD needs to be passed to the restic backend and associated with the restic snapshot information.
					snapshotPhase = constant.SnapshotPhaseFailed // todo fix snapshot data
				case constant.SnapshotPhasePending.String():
					snapshotPhase = constant.SnapshotPhaseRunning
				}

				if err := r.snapshotOperator.SetSnapshotPhase(backup.Name, snapshot, snapshotPhase); err != nil {
					log.Errorf("update backup %s snapshot %s phase running error %v", backup.Name, snapshot.Spec.Id, err)
				}

				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				log.Info("hit snapshot update event")

				// todo need update backup.spec.Size

				oldObj, newObj := updateEvent.ObjectOld, updateEvent.ObjectNew
				a, ok1 := r.isSysSnapshot(oldObj)
				b, ok2 := r.isSysSnapshot(newObj)

				if !(ok1 && ok2) || reflect.DeepEqual(a.Spec, b.Spec) {
					return false
				}

				backup, err := r.getBackup(b.Spec.BackupId)
				if err != nil {
					log.Errorf("get backup error %v", err)
					return false
				}

				if util.ListContains([]string{constant.SnapshotPhaseComplete.String(), constant.SnapshotPhaseFailed.String()}, *b.Spec.Phase) {
					log.Infof("backup: %s, snapshot: %s, phase: %s", backup.Name, b.Spec.Id, *b.Spec.Phase)
					return false
				}

				log.Infof("run backup: %s, snapshot: %s", backup.Name, r.snapshotOperator.ParseSnapshotName(b.Spec.StartAt))

				if err := r.snapshotOperator.Backup(backup, b); err != nil {
					log.Errorf("backup %s snapshot error %v", backup.Name, err)
				}

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
	backup, err := r.backupOperator.GetBackupById(ctx, backupId)
	if err != nil {
		return nil, err
	}

	return backup, nil
}
