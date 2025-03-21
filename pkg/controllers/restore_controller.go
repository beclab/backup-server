package controllers

import (
	"context"

	sysapiv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	bclient "bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
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

type RestoreReconciler struct {
	client.Client
	factory   bclient.Factory
	bcManager velero.Manager
	scheme    *runtime.Scheme
	handler   handlers.Interface
}

func NewRestoreController(c client.Client, factory bclient.Factory, bcm velero.Manager, schema *runtime.Scheme, handler handlers.Interface) *RestoreReconciler {
	return &RestoreReconciler{Client: c,
		factory:   factory,
		bcManager: bcm,
		scheme:    schema,
		handler:   handler,
	}
}

//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=restore,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=restore/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=restore/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Restore object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *RestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&sysapiv1.Restore{}, builder.WithPredicates(predicate.Funcs{
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				log.Info("hit restore create event")

				// if phase != Pending (restart) ??

				restore, ok := r.isSysRestore(e.Object)
				if !ok {
					log.Debugf("not a restore resource")
					return false
				}

				log.Infof("hit restore create event %s %s", restore.Name, *restore.Spec.Phase)

				var phase = *restore.Spec.Phase
				switch phase {
				case constant.Pending.String():
					log.Infof("add to restore worker %s", restore.Name)
					worker.Worker.AppendRestoreTask(restore.Name)
				default:
					// todo update Failed
					return false
				}

				return false

				// check snapshot --> backup exists ?

				// add to worker
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				log.Info("hit restore update event")

				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				log.Info("hit restore delete event")
				return false
			},
		})).Build(r)
	if err != nil {
		return err
	}

	return nil
}

func (r *RestoreReconciler) isSysRestore(obj client.Object) (*sysapiv1.Restore, bool) {
	b, ok := obj.(*sysapiv1.Restore)
	if !ok || b == nil {
		return nil, false
	}

	return b, true
}
