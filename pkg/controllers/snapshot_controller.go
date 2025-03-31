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
	log.Infof("received snapshot request, namespace: %q, name: %q", req.Namespace, req.Name)

	c, err := r.factory.Sysv1Client()
	if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 3 * time.Second}, errors.WithStack(err)
	}

	snapshot, err := c.SysV1().Snapshots(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		return ctrl.Result{Requeue: true, RequeueAfter: 2 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, errors.WithStack(err)
	}

	var phase = *snapshot.Spec.Phase

	switch phase {
	case constant.Pending.String():
		if err := r.addToWorkerManager(snapshot); err != nil {
			return ctrl.Result{}, errors.WithStack(err)
		}
	case constant.Running.String():
		if err := r.updateRunningPhase(snapshot); err != nil {
			return ctrl.Result{}, errors.WithStack(err)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&sysapiv1.Snapshot{}, builder.WithPredicates(predicate.Funcs{
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				log.Info("hit snapshot create event")

				snapshot, ok := r.isSysSnapshot(e.Object)
				if !ok {
					log.Debugf("not a snapshot resource")
					return false
				}

				log.Infof("hit snapshot create event, snapshotId: %s, snapshotPhase: %s", snapshot.Name, *snapshot.Spec.Phase)

				var phase = *snapshot.Spec.Phase

				switch phase {
				case constant.Completed.String(), constant.Failed.String(), constant.Canceled.String():
					return false
				default:
					return true
				}
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				log.Info("hit snapshot update event")

				oldObj, newObj := updateEvent.ObjectOld, updateEvent.ObjectNew
				oldSnapshot, ok1 := r.isSysSnapshot(oldObj)
				newSnapshot, ok2 := r.isSysSnapshot(newObj)

				if !(ok1 && ok2) || reflect.DeepEqual(oldSnapshot.Spec, newSnapshot.Spec) {
					return false
				}

				backup, err := r.getBackup(newSnapshot.Spec.BackupId)
				if err != nil {
					log.Errorf("get backup error: %v, backupId: %s, spanshotId: %s", err, newSnapshot.Spec.BackupId, newSnapshot.Name)
					return false
				}

				log.Infof("snapshot update event, snapshotId: %s, snapshotPhase: %s", newSnapshot.Name, *newSnapshot.Spec.Phase)

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
					log.Errorf("notify backupName: %s, snapshotId: %s, snapshotPhase: %s, error: %v", backup.Spec.Name, newSnapshot.Name, notifyState, err)
				} else {
					log.Infof("notify backupName: %s, snapshotId: %s, snapshotPhase: %s", backup.Spec.Name, newSnapshot.Name, notifyState)
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

func (r *SnapshotReconciler) addToWorkerManager(snapshot *sysapiv1.Snapshot) error {
	backup, err := r.getBackup(snapshot.Spec.BackupId)
	if err != nil {
		log.Errorf("get backup error %v, backupId: %s", err, snapshot.Spec.BackupId)
		return err
	}
	if err := r.notifySnapshot(backup, snapshot, constant.Pending.String()); err != nil {
		log.Errorf("notify backupName: %s, snapshotId: %s, phase: New, error: %v", backup.Spec.Name, snapshot.Name, err)
	} else {
		log.Infof("notify backupName: %s, snapshotId: %s, phase: New", backup.Spec.Name, snapshot.Name)
	}
	log.Infof("add to backup worker, snapshotId: %s", snapshot.Name)
	worker.Worker.AppendBackupTask(fmt.Sprintf("%s_%s_%s", backup.Spec.Owner, snapshot.Spec.BackupId, snapshot.Name))

	return nil
}

func (r *SnapshotReconciler) updateRunningPhase(snapshot *sysapiv1.Snapshot) error {
	backup, err := r.getBackup(snapshot.Spec.BackupId)
	if err != nil {
		log.Errorf("get backup error %v, backupId: %s", err, snapshot.Spec.BackupId)
		return err
	}
	// It is necessary to check whether the restic snapshot was successful and fix the CRD data.
	// The snapshotID from the CRD needs to be passed to the restic backend and associated with the restic snapshot information.
	if err := r.handler.GetSnapshotHandler().UpdatePhase(context.Background(), snapshot.Name, constant.Failed.String()); err != nil {
		log.Errorf("update backupName: %s, snapshotId: %s, phase Failed, error %v", backup.Spec.Name, snapshot.Name, err)
	}
	if err := r.notifySnapshot(backup, snapshot, constant.Failed.String()); err != nil {
		log.Errorf("notify backupName: %s, snapshotId: %s, phase: Failed, error: %v", backup.Spec.Name, snapshot.Name, err)
	} else {
		log.Infof("notify backupName: %s, snapshotId: %s, phase: Failed", backup.Spec.Name, snapshot.Name)
	}

	return nil
}
