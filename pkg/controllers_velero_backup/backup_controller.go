package controllersvelerobackup

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	sysapiv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	bclient "bytetrade.io/web3os/backup-server/pkg/client"
	sysv1 "bytetrade.io/web3os/backup-server/pkg/generated/clientset/versioned"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type VeleroBackupPhase struct {
	Time  uint64
	Phase v1.BackupPhase
}

type BackupReconciler struct {
	client.Client
	factory bclient.Factory
	manager velero.Manager
	scheme  *runtime.Scheme

	sc sysv1.Interface
	vc veleroclientset.Interface

	startup struct {
		backups map[string]*VeleroBackupPhase
		sync.Mutex
	}
}

func NewVeleroBackupController(c client.Client, factory bclient.Factory, bcm velero.Manager, schema *runtime.Scheme) *BackupReconciler {
	b := &BackupReconciler{Client: c,
		factory: factory,
		manager: bcm,
		scheme:  schema,
		startup: struct {
			backups map[string]*VeleroBackupPhase
			sync.Mutex
		}{
			backups: map[string]*VeleroBackupPhase{},
		},
	}

	b.getAllVeleroBackups()

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

func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var jobTime = r.manager.GetBackupJobLastTime(req.Name)
	var backupName, _ = r.manager.GetBackupName(req.Name)

	sysBackup, err := r.manager.GetSysBackup(ctx, req.Name)
	sysBackupConfig, err := r.manager.GetBackupConfig(ctx, backupName)

	if sysBackupConfig == nil {
		log.Errorf("Backup - backup config not found %s", req.Name)
		return ctrl.Result{}, nil
	}

	if sysBackup != nil {
		if sysBackup.Spec.MiddleWarePhase == nil || *sysBackup.Spec.MiddleWarePhase == velero.Running {
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}
	}

	if err != nil && !apierrors.IsNotFound(err) {
		log.Errorf("Backup - reconcile get sys backup error %s(%d) %v", req.Name, jobTime, err)
		return ctrl.Result{}, nil
	}

	if sysBackup == nil {
		log.Errorf("Backup - reconcile sys backup not found %s(%d)", req.Name, jobTime)
		return ctrl.Result{}, nil
	}

	if retry := r.createVeleroBackup(ctx, sysBackup, sysBackupConfig); retry {
		log.Warnf("Backup - create backup retry %s(%d)", req.Name, jobTime)
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *BackupReconciler) createVeleroBackup(ctx context.Context, sysBackup *sysapiv1.Backup, sysBackupConfig *sysapiv1.BackupConfig) bool {
	pbmName := sysBackup.Annotations[velero.AnnotationPerconaMongoClusterLastBackupPBMName]
	if pbmName == "" {
		log.Error("Backup - mongo last backup pbmname not found")
		return false
	}

	config := sysBackup.Labels[velero.LabelBackupConfig]
	if config == "" {
		log.Error("Backup - backup config not found")
		return false
	}

	ctx1, cancel1 := context.WithTimeout(ctx, 30*time.Second)
	defer cancel1()

	jobTime := r.manager.GetBackupJobLastTime(sysBackup.Name)
	backupType := sysBackup.Spec.Extra[velero.ExtraBackupType]
	retainStr := sysBackup.Spec.Extra[velero.ExtraRetainDays]

	sysBackupConfig, err := r.manager.GetBackupConfig(ctx1, config)
	if err != nil {
		log.Errorf("Backup - get backup config error %s(%d) %v", config, jobTime, err)
		return false
	}

	ok, err := r.isBackupInProgress(ctx1, config)
	if err != nil {
		log.Errorf("Backup - create backup errored %s(%d) %v", config, jobTime, err)
		return false
	}

	if !ok {
		return true
	}

	if err = r.manager.DeleteBackupX(ctx1, sysBackup.Namespace, config); err != nil {
		log.Errorf("Backup - delete old backup error %s(%d) %v", config, jobTime, err)
		return false
	}

	retainDays, err := strconv.ParseInt(retainStr, 10, 64)
	if err != nil {
		log.Errorf("Backup - build backup retainDays error %s(%d) %v", config, jobTime, err)
		return false
	}

	if err = r.manager.SetBackupConfigPBMName(ctx1, sysBackupConfig, pbmName); err != nil {
		log.Errorf("Backup - set backupconfig pbmname %s error %s(%d) %v", pbmName, config, jobTime, err)
		return true
	}

	buildVeleroBackup := r.manager.BuildVeleroBackupX(ctx1, config, fmt.Sprintf("%d", jobTime), *sysBackup.Spec.Owner, backupType, retainDays)

	log.Infof("Backup - create backup prepared %s(%d) storage: %s", config, jobTime, sysBackupConfig.Spec.StorageLocation)
	if err = r.manager.CreateBackupX(ctx1, sysBackup.Namespace, buildVeleroBackup); err != nil {
		log.Errorf("Backup - create backup error %s(%d) %v", config, jobTime, err)
		return true
	}
	log.Infof("Backup - create backup start %s(%d)", config, jobTime)
	return false
}

func (r *BackupReconciler) isBackupInProgress(ctx context.Context, name string) (bool, error) {
	backup, err := r.manager.GetVeleroBackup(ctx, name)
	if err != nil {
		return false, fmt.Errorf("get error %v", err)
	}

	if backup == nil {
		return true, nil
	}

	return r.manager.CanDeleteBackup(backup.Status.Phase), nil
}

// TODO not implemented
func (r *BackupReconciler) deleteVeleroBackup(ctx context.Context, name string, namespace string) (bool, error) {
	// todo Reconciler
	// ok, err := r.deleteVeleroBackup(ctx, req.Name, req.Namespace)
	// if err != nil {
	// 	return ctrl.Result{}, nil
	// }
	// if ok {
	// 	return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	// }
	// return ctrl.Result{}, nil

	ctx1, cancel1 := context.WithTimeout(ctx, 30*time.Second)
	defer cancel1()

	var phase v1.BackupPhase

	backup, err := r.manager.GetVeleroBackup(ctx1, name)
	if err != nil {
		log.Errorf("Backup - get velero backup error %s %v", name, err)
	}
	if backup == nil {
		phase = v1.BackupPhase("")
	} else {
		phase = backup.Status.Phase
	}

	log.Infof("Backup - delete sys backup %s phase: %s", name, phase)
	if r.manager.CanDeleteBackup(phase) {
		if err = r.manager.DeleteBackupX(ctx1, namespace, name); err != nil {
			log.Errorf("Backup - delete sys backup error %s %v", name, err)
			return true, err
		}
		log.Infof("Backup - delete sys backup succeed %s", name)
		return false, nil
	}
	log.Warnf("Backup - delete sys backup failed, phase invalid %s %s, continue", name, phase)
	return true, nil
}

func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&sysapiv1.Backup{}, builder.WithPredicates(predicate.Funcs{
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(ue event.UpdateEvent) bool {
				var sysBackup, ok = ue.ObjectNew.(*sysapiv1.Backup)
				if !ok {
					return false
				}
				oldSysBackup, ok := ue.ObjectOld.(*sysapiv1.Backup)
				if !ok {
					return false
				}

				if *oldSysBackup.Spec.Phase == *sysBackup.Spec.Phase {
					return false
				}

				// sys backup phase changed
				var canCreateBackupFlag = r.manager.CanCreateBackup(v1.BackupPhase(*sysBackup.Spec.Phase))
				var isExpiredFlag, needFix = r.isExpiredBackupJob(sysBackup)

				if needFix {
					r.fixSysBackupPhase(sysBackup)
				}

				if canCreateBackupFlag {
					log.Infof("Backup - Create - %s %s canCreate: %v, isExpired: %v, needFix: %v",
						sysBackup.Name, *sysBackup.Spec.Phase, canCreateBackupFlag, isExpiredFlag, needFix)
					return !isExpiredFlag
				}

				return false
			},
			DeleteFunc: func(de event.DeleteEvent) bool {
				return false // TODO
			},
		})).Build(r)
	if err != nil {
		return err
	}
	return nil
}

func (r *BackupReconciler) SetupBackupsInformer() *BackupReconciler {
	informer, err := r.manager.NewInformerFactory()
	if err != nil {
		return r
	}

	backupsInformer := informer.Velero().V1().Backups().Informer()

	// TODO when updating, should we update the intermediate or failed states as well?
	backupsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newBackup, ok := obj.(*v1.Backup)
			if !ok {
				return
			}
			var jobTime = newBackup.Annotations[velero.AnnotationBackupJobTime]
			log.Infof("Backup - informer new backup %s(%s) %s", newBackup.Name, jobTime, newBackup.Status.Phase)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldBackup, ok := oldObj.(*v1.Backup)
			if !ok {
				return
			}

			newBackup, ok := newObj.(*v1.Backup)
			if !ok {
				return
			}
			oldPhase := oldBackup.Status.Phase
			newPhase := newBackup.Status.Phase

			if oldPhase == newPhase {
				return
			}

			if newPhase == v1.BackupPhaseDeleting {
				return
			}
			var jobTime = newBackup.Annotations[velero.AnnotationBackupJobTime]
			if err := r.manager.UpdateSysBackupPhase(context.TODO(), newBackup); err != nil {
				// TODO If a sys backup is deleted, how should it be handled here
				log.Errorf("Backup - informer update backup phase error %s(%s) %s: %v", newBackup.Name, jobTime, newPhase, err)
				return
			}
			log.Infof("Backup - informer update backup phase %s(%s) %s", newBackup.Name, jobTime, newPhase)
		},
	})

	return r
}

func (r *BackupReconciler) getAllVeleroBackups() {
	var ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	backups, err := r.manager.ListBackups(ctx)
	if err != nil || backups == nil || len(backups.Items) == 0 {
		return
	}

	for _, item := range backups.Items {
		_, ok := r.startup.backups[item.Name]
		if ok {
			continue
		}
		jobTimeStr := item.Annotations[velero.AnnotationBackupJobTime]
		jobTime, _ := strconv.ParseUint(jobTimeStr, 10, 64)
		var vbp = &VeleroBackupPhase{
			Time:  jobTime,
			Phase: item.Status.Phase,
		}
		r.startup.backups[item.Name] = vbp
	}
}

func (r *BackupReconciler) isExpiredBackupJob(sysBackup *sysapiv1.Backup) (bool, bool) {
	var name = sysBackup.Name
	var jobName = sysBackup.Labels[velero.LabelBackupConfig]
	if jobName == "" {
		return true, false
	}

	var jobTime = r.manager.GetBackupJobLastTime(name)

	r.startup.Lock()
	v, ok := r.startup.backups[jobName]
	if !ok {
		var vbp = &VeleroBackupPhase{
			Time:  jobTime,
			Phase: v1.BackupPhaseNew,
		}
		r.startup.backups[jobName] = vbp
		r.startup.Unlock()
		return false, false
	}

	if v.Time == 0 || jobTime >= v.Time {
		r.startup.backups[jobName].Time = jobTime
	}
	r.startup.Unlock()
	return jobTime < v.Time, v.Phase == v1.BackupPhaseCompleted
}

/**
 * If the velero.Backup is in the "Completed" state,
 * consider checking and repairing the Phase of sys.Backup to improve the appearance of the Phase data for sys.Backup.
 * During the testing process, it is possible for sys.Backup to be in an abnormal state while velero.Backup is in the "Completed" state.
 */
func (r *BackupReconciler) fixSysBackupPhase(sysBackup *sysapiv1.Backup) {
	if *sysBackup.Spec.Phase == string(v1.BackupPhaseCompleted) {
		return
	}

	var jobTime = r.manager.GetBackupJobLastTime(sysBackup.Name)

	var s = string(v1.BackupPhaseCompleted)
	sysBackup.Spec.Phase = &s

	var ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := r.manager.UpdateSysBackup(ctx, sysBackup)
	if err != nil {
		log.Errorf("Backup - fix sys backup phase error %s(%s) %v", sysBackup.Name, jobTime, err)
	}
}
