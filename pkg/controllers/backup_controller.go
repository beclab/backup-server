package controllers

import (
	"context"
	"reflect"
	"time"

	sysapiv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	bclient "bytetrade.io/web3os/backup-server/pkg/client"
	sysv1 "bytetrade.io/web3os/backup-server/pkg/generated/clientset/versioned"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
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
type BackupReconciler struct {
	client.Client
	factory bclient.Factory
	manager velero.Manager
	scheme  *runtime.Scheme

	sc sysv1.Interface
	vc veleroclientset.Interface
}

func NewBackupController(c client.Client, factory bclient.Factory, bcm velero.Manager, schema *runtime.Scheme) *BackupReconciler {
	b := &BackupReconciler{Client: c,
		factory: factory,
		manager: bcm,
		scheme:  schema,
	}

	sc, err := factory.Sysv1Client()
	if err != nil {
		panic(err)
	}

	vc, err := factory.Client()
	if err != nil {
		panic(err)
	}

	b.sc = sc
	b.vc = vc
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
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *BackupReconciler) handleUpdateSysBackup(sb *sysapiv1.Backup) {
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
	if sb.Spec.MiddleWarePhase == nil ||
		!util.ListContains([]string{velero.Succeed, velero.Success}, *sb.Spec.MiddleWarePhase) {
		log.Infof("velero or middleware backup not ready")
		return
	}

	log.Debugf("starting async to backup %q osdata", name)

	go r.manager.AsyncOsDataBackup(name)
}

func (r *BackupReconciler) waitingAvailable(ctx context.Context) error {
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

func (r *BackupReconciler) handleDeleteBackup(name string, sb *sysapiv1.Backup) {
	var (
		ctx = context.Background()
	)

	var backupType string

	log.Debugf("deleting backup %q, type: %s", name, backupType)

	// delete backup
	if err := r.manager.DeleteBackup(ctx, name, sb); err != nil && !apierrors.IsNotFound(err) {
		log.Errorf("failed to delete backup %q: %v", name, err)
		return
	}

	log.Debugf("successfully to delete backup %q", name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&sysapiv1.Backup{}, builder.WithPredicates(predicate.Funcs{
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return false },
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				oldObj, newObj := updateEvent.ObjectOld, updateEvent.ObjectNew
				a, ok1 := r.isSysBackup(oldObj)
				b, ok2 := r.isSysBackup(newObj)
				if !(ok1 && ok2) || reflect.DeepEqual(a.Spec, b.Spec) {
					return false
				}
				resticPhase := b.Spec.ResticPhase
				if resticPhase != nil {
					// restic backup running
					return false
				}

				oldPhase, newPhase := a.Spec.Phase, b.Spec.Phase
				oldMWPhase, newMWPhase := a.Spec.MiddleWarePhase, b.Spec.MiddleWarePhase

				if (oldPhase != nil && newPhase != nil && *oldPhase == *newPhase) &&
					(oldMWPhase != nil && newMWPhase != nil && *oldMWPhase == *newMWPhase) {
					return false
				}

				if velero.VeleroBackupCompleted == *newPhase {
					log.Debugf("backup %q received %q event", b.Name, *newPhase)
					r.handleUpdateSysBackup(b)
				}
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				sb, ok := r.isSysBackup(e.Object)
				if !ok {
					return false
				}
				r.handleDeleteBackup(sb.Name, sb)
				return false
			},
		})).Build(r)
	if err != nil {
		return err
	}

	return nil
}

func (r *BackupReconciler) isSysBackup(obj client.Object) (*sysapiv1.Backup, bool) {
	b, ok := obj.(*sysapiv1.Backup)
	if !ok || b == nil {
		return nil, false
	}
	if b.Namespace != velero.DefaultVeleroNamespace {
		return nil, false
	}
	if b.Spec.Owner == nil || b.Spec.Phase == nil || b.Spec.Extra == nil {
		return nil, false
	}
	if _, ok = b.Spec.Extra[velero.ExtraBackupType]; !ok {
		return nil, false
	}
	if _, ok = b.Spec.Extra[velero.ExtraBackupStorageLocation]; !ok {
		return nil, false
	}
	if _, ok = b.Spec.Extra[velero.ExtraRetainDays]; !ok {
		return nil, false
	}

	return b, true
}
