package controllers

import (
	"context"
	"fmt"
	"time"

	bclient "bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/pkg/errors"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	clientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	"go4.org/sort"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type BackupRestoreReconciler struct {
	client.Client
	factory   bclient.Factory
	bcManager velero.Manager
	scheme    *runtime.Scheme
}

func NewBackupRestoreController(c client.Client, factory bclient.Factory, bcm velero.Manager, schema *runtime.Scheme) *BackupRestoreReconciler {
	return &BackupRestoreReconciler{Client: c, factory: factory, bcManager: bcm, scheme: schema}
}

//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=backupconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=backupconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=sys.bytetrade.io,resources=backupconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BackupConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *BackupRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Infof("received br request, namespace: %q, name: %q", req.Namespace, req.Name)

	vc, err := r.factory.Client()
	if err != nil {
		return ctrl.Result{}, errors.WithStack(err)
	}

	restores, err := vc.VeleroV1().Restores(req.Namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Errorf("%+v", err)
		return ctrl.Result{}, nil
	}
	sort.Slice(restores.Items, func(i, j int) bool {
		return !restores.Items[i].CreationTimestamp.Before(&restores.Items[j].CreationTimestamp)
	})

	if len(restores.Items) < 1 {
		log.Info("no restores found, wait retry")
		return ctrl.Result{}, errors.New("no restores found, be problem")
	} else {
		// waiting for bfl ready
		if err = r.isReady(ctx); err != nil {
			log.Warn("bfl not ready, requeue after 5 seconds")
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}

		log.Info("download and restore osdata ...")
		latestRestore := restores.Items[0]
		log.Infof("got latest restore %q: %s", latestRestore.Name, util.PrettyJSON(latestRestore))

		if _, ok := latestRestore.Annotations[velero.AnnotationOSDataBackupRestored]; ok {
			log.Info("osdata is already restored")
			return ctrl.Result{}, nil
		}

		err = r.updateRestore(ctx, vc, req.Namespace, latestRestore.Name)
		if err != nil {
			log.Errorf("%+v", err)
			return ctrl.Result{}, nil
		}
		log.Info("osdata restore successfully")
	}

	return ctrl.Result{}, nil
}

func (r *BackupRestoreReconciler) isReady(ctx context.Context) error {
	kc, err := r.factory.KubeClient()
	if err != nil {
		return err
	}

	sts, err := kc.AppsV1().StatefulSets("").
		List(ctx, metav1.ListOptions{LabelSelector: "tier=bfl"})
	if err != nil && apierrors.IsNotFound(err) {
		return errors.New("bfl sts not found")
	} else if err != nil {
		return errors.WithStack(err)
	}

	if len(sts.Items) == 0 {
		return errors.New("no bfl sts with all users")
	}

	for _, st := range sts.Items {
		if !r.stsAvailable(st.Status) {
			return errors.Errorf("bfl sts %q not ready yet", st.Namespace)
		}

		if err = r.pvReady(ctx, &st, "userspace_pv"); err != nil {
			return err
		}
	}

	appService, err := kc.AppsV1().StatefulSets("os-system").
		Get(ctx, "app-service", metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		return errors.New("sts app-service not found")
	} else if err != nil {
		return errors.WithStack(err)
	}

	if !r.stsAvailable(appService.Status) {
		return errors.New("sts os-system/app-service not ready")
	}

	return r.pvReady(ctx, appService, "charts_pv")
}

func (r *BackupRestoreReconciler) stsAvailable(status appsv1.StatefulSetStatus) bool {
	return status.AvailableReplicas == status.ReadyReplicas && status.ReadyReplicas == 1
}

func (r *BackupRestoreReconciler) pvReady(ctx context.Context, st *appsv1.StatefulSet, pvAnnotationName string) error {
	kc, err := r.factory.KubeClient()
	if err != nil {
		return err
	}

	pvName, ok := st.Annotations[pvAnnotationName]
	if !ok {
		return errors.Errorf("sts %s/%s, no pv annotation %q", st.Namespace, st.Name, pvAnnotationName)
	}

	pv, err := kc.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		return errors.Errorf("pv %q not created", pvName)
	} else if err != nil {
		return errors.WithStack(err)
	}
	if pv.Status.Phase != corev1.VolumeBound {
		return errors.Errorf("pv %q not ready, phase %q", pvName, pv.Status.Phase)
	}

	return nil
}

func (r *BackupRestoreReconciler) updateRestore(ctx context.Context, vc clientset.Interface, namespace, restoreName string) error {
	patchAnnotation := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"true"}}}`, velero.AnnotationOSDataBackupRestored)

	updatedRestore, err := vc.VeleroV1().Restores(namespace).
		Patch(ctx, restoreName, types.MergePatchType, []byte(patchAnnotation), metav1.PatchOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	log.Infof("patched %q restore: %s", updatedRestore.Name, util.PrettyJSON(updatedRestore))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&velerov1api.Restore{},
			builder.WithPredicates(newCreateOnlyPredicate(func(e event.CreateEvent) bool {
				log.Info("hit backup restore create event")

				v, ok := e.Object.(*velerov1api.Restore)
				if !ok {
					return false
				}
				log.Infof("restore backup name: %s", v.Spec.BackupName)
				return v.Namespace == velero.DefaultVeleroNamespace

			}))).
		Complete(r)
}
