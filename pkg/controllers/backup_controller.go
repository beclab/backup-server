package controllers

import (
	"context"
	"reflect"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	k8sclient "bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/gengo/examples/set-gen/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// BackupReconciler reconciles a Backup object
type BackupReconciler struct {
	client.Client
	manager   velero.Manager
	factory   k8sclient.Factory
	bcManager velero.Manager
	scheme    *runtime.Scheme
	handler   handlers.Interface
}

func NewBackupController(c client.Client, factory k8sclient.Factory, bcm velero.Manager, schema *runtime.Scheme, handler handlers.Interface) *BackupReconciler {
	return &BackupReconciler{
		Client:    c,
		manager:   bcm,
		factory:   factory,
		bcManager: bcm,
		scheme:    schema,
		handler:   handler,
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

	backup, err := c.SysV1().Backups(req.Namespace).
		Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		return ctrl.Result{Requeue: true, RequeueAfter: 2 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, errors.WithStack(err)
	} else if backup.Spec.BackupPolicy.Enabled {
		err = r.reconcileBackupPolicies(backup)
		if err != nil {
			return ctrl.Result{}, errors.WithStack(err)
		}
	} else {
		return ctrl.Result{}, nil
	}

	// todo restore

	// ignore when restore in progress
	// vc, err := r.factory.Client()
	// if err != nil {
	// 	return ctrl.Result{}, errors.WithStack(err)
	// } else {
	// 	restores, err := vc.VeleroV1().Restores(req.Namespace).
	// 		List(ctx, metav1.ListOptions{})
	// 	if err != nil {
	// 		return ctrl.Result{}, errors.WithStack(err)
	// 	}

	// 	for _, restore := range restores.Items {
	// 		if restore.Status.Phase == velerov1api.RestorePhaseInProgress {
	// 			log.Warn("restore in progress, requeue after 5 minutes")
	// 			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Minute}, nil
	// 		}
	// 	}
	// }

	// if err = r.apply(ctx, req.Namespace, &bc.Spec); err != nil {
	// 	return ctrl.Result{}, errors.WithStack(err)
	// }

	return ctrl.Result{}, nil
}

func (r *BackupReconciler) reconcileBackupPolicies(backup *sysv1.Backup) error {
	ctx := context.Background()
	if backup.Spec.BackupPolicy != nil {
		cron, _ := util.ParseToCron(backup.Spec.BackupPolicy.SnapshotFrequency, backup.Spec.BackupPolicy.TimesOfDay, backup.Spec.BackupPolicy.DayOfWeek)
		err := r.handler.GetSnapshotHandler().CreateSnapshotSchedule(ctx, backup, cron, !backup.Spec.BackupPolicy.Enabled)
		if err != nil {
			return err
		}
		log.Debugf("schedule %q created: %q", backup.Spec.Name, cron)
	}
	return nil
}

func (r *BackupReconciler) apply(ctx context.Context, namespace string, bSpec *sysv1.BackupSpec) error {
	log.Info("prepare to apply velero resources")

	// kc, err := r.factory.KubeClient()
	// if err != nil {
	// 	return err
	// }

	// v := r.bcManager

	// // credentials secret
	// applySecret := velero.BuildSecretApplyConfiguration(namespace, velero.DefaultVeleroSecretName, v.NewCredentials(bSpec))
	// secret, err := kc.CoreV1().Secrets(namespace).
	// 	Apply(ctx, applySecret, metav1.ApplyOptions{Force: true, FieldManager: velero.ApplyPatchFieldManager})
	// if err != nil {
	// 	return pkgerrors.Errorf("unable to apply secret: %v", err)
	// }
	// log.Infof("applied %q secret: %s", secret.Name, util.PrettyJSON(secret))

	// // backupStorageLocation
	// bsl, err := v.ApplyBackupStorageLocation(ctx, bSpec)
	// if err != nil {
	// 	return err
	// }
	// log.Infof("applied %q backupStorageLocation: %s", bsl.Name, util.PrettyJSON(bsl))

	// // deployment
	// applyDeployment := velero.DeploymentApplyConfiguration(namespace, bSpec)
	// deploy, err := kc.AppsV1().
	// 	Deployments(namespace).
	// 	Apply(ctx, applyDeployment, metav1.ApplyOptions{Force: true, FieldManager: velero.ApplyPatchFieldManager})
	// if err != nil {
	// 	return pkgerrors.Errorf("apply deployment: %v", err)
	// }
	// log.Infof("applied %q deployment: %s", deploy.Name, util.PrettyJSON(deploy.Spec))

	return nil
}

func (r *BackupReconciler) deleteBackupConfig(name string, namespace string) {
	// sc, err := r.factory.Sysv1Client()
	// if err != nil {
	// 	log.Warnf("failed to new sys client: %v", err)
	// 	return
	// }

	// log.Infof("delete backup tasks of config %s", name)
	// var labelSelector = fmt.Sprintf("%s=%s", velero.LabelBackupConfig, name)

	// ctx := context.Background()

	// if backups, err := sc.SysV1().Backups(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector}); err == nil {
	// 	if len(backups.Items) > 0 {
	// 		for i := len(backups.Items) - 1; i >= 0; i-- {
	// 			b := backups.Items[i]
	// 			// if config, ok := b.Annotations[velero.LabelBackupConfig]; ok && config == name {
	// 			log.Infof("deleteing backup [%s] of config, %s", b.Name, b.Spec.Extra[velero.ExtraBackupType])
	// 			if err = sc.SysV1().Backups(namespace).Delete(ctx, b.Name, metav1.DeleteOptions{}); err != nil {
	// 				log.Errorf("delete backups of config error, %+v, %s", err, b.Name)
	// 			}
	// 			// }
	// 		}
	// 	}

	// 	if err = r.manager.DeleteVeleroBackup(ctx, namespace, name); err != nil {
	// 		log.Warnf("delete velero backup error, %+v, %s", err, name)
	// 	}
	// 	log.Infof("successfully delete bc %q", name)
	// } else {
	// 	log.Errorf("get backups of config error, %+v, %s", err, name)
	// }
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
			DeleteFunc: func(e event.DeleteEvent) bool {
				log.Info("hit backup delete event")
				// if !isTrue(e.Object) {
				// 	return false
				// }
				// r.deleteBackupConfig(e.Object.GetName(), e.Object.GetNamespace())
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				log.Info("hit backup update event")
				if !isTrue(e.ObjectOld, e.ObjectNew) {
					return false
				}
				bc1, ok1 := e.ObjectOld.(*sysv1.Backup)
				bc2, ok2 := e.ObjectNew.(*sysv1.Backup)
				if !(ok1 || ok2) || reflect.DeepEqual(bc1.Spec, bc2.Spec) {
					log.Info("backup not changed")
					return false
				}
				return true
			}})).
		Build(r)
	if err != nil {
		return err
	}

	return nil

	// return c.Watch(&source.Kind{Type: &appsv1.Deployment{}},
	// 	handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
	// 		return []reconcile.Request{{NamespacedName: types.NamespacedName{
	// 			Namespace: o.GetNamespace(),
	// 			Name:      o.GetName()}},
	// 		}
	// 	}), newDeleteOnlyPredicate(func(e event.DeleteEvent) bool {
	// 		log.Info("hit velero deployment delete event")
	// 		return isTrue(e.Object) && e.Object.GetName() == velero.DefaultVeleroDeploymentName &&
	// 			r.backupConfigExist()
	// 	}))
}

func isTrue(objects ...client.Object) bool {
	fs := sets.NewByte()

	for _, o := range objects {
		_, ok := o.GetLabels()["component"]
		if o.GetNamespace() == velero.DefaultVeleroNamespace &&
			ok {
			fs.Insert('y')
		} else {
			fs.Insert('n')
		}
	}
	return fs.HasAll('y')
}
