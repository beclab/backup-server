package velero

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func (v *velero) ListBackupX() {

}

func (v *velero) ExistsRunningBackupX() (bool, error) {
	return false, nil
}

func (v *velero) CreateBackupX(ctx context.Context,
	namespace string, backup *v1.Backup) error {
	vc, err := v.factory.Client()
	if err != nil {
		return err
	}

	_, err = vc.VeleroV1().Backups(namespace).Create(ctx, backup, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (v *velero) UpdateSysBackupPhase(ctx context.Context, backup *v1.Backup) error {
	var jobTime = backup.Annotations[AnnotationBackupJobTime]
	var jobName = fmt.Sprintf("%s-%s", backup.Name, jobTime)
	sysBackup, err := v.GetSysBackup(ctx, jobName)
	if err != nil {
		return err
	}

	sysBackup.Spec.Phase = (*string)(&backup.Status.Phase)

	return v.UpdateSysBackup(ctx, sysBackup)
}

func (v *velero) UpdateSysBackup(ctx context.Context, sysBackup *sysv1.Backup) error {
	client, err := v.factory.Sysv1Client()
	if err != nil {
		return err
	}

	_, err = client.SysV1().Backups(sysBackup.Namespace).Update(ctx, sysBackup, metav1.UpdateOptions{})
	return err
}

func (v *velero) DeleteBackupX(ctx context.Context, namespace string, name string) error {
	vc, err := v.factory.Client()
	if err != nil {
		return err
	}

	vb, _ := v.GetVeleroBackup(ctx, name)
	if vb == nil {
		return nil
	}

	if err = v.removeExistsBackupDeleteRequest(namespace, name); err != nil {
		return err
	}

	var req = &v1.DeleteBackupRequest{}
	req.Name = name
	req.Namespace = namespace
	req.Spec.BackupName = name

	_, err = vc.VeleroV1().DeleteBackupRequests(namespace).Create(ctx, req, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("delete request error %v", err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	a1 := informers.NewDeleteBackupRequestInformer(vc, namespace, 0, nil)
	var delEventhandler = cache.ResourceEventHandlerFuncs{}

	delEventhandler.UpdateFunc = func(oldObj, newObj interface{}) {
		b := newObj.(*v1.DeleteBackupRequest)
		if b.Status.Phase == v1.DeleteBackupRequestPhaseProcessed {
			stopCh <- struct{}{}
		}
	}

	a1.AddEventHandler(delEventhandler)
	go a1.Run(stopCh)

	select {
	case <-stopCh:
		return nil
	}
}

func (v *velero) removeExistsBackupDeleteRequest(namespace string, name string) error {
	vc, err := v.factory.Client()
	if err != nil {
		return err
	}

	vc.VeleroV1().DeleteBackupRequests(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	return nil
}

func (v *velero) CanDeleteBackup(phase v1.BackupPhase) bool {
	switch phase {
	case v1.BackupPhaseCompleted, v1.BackupPhaseFailed,
		v1.BackupPhaseFailedValidation,
		v1.BackupPhaseFinalizingPartiallyFailed,
		v1.BackupPhasePartiallyFailed, v1.BackupPhase(""):
		return true
	default:
		return false
	}
}

func (v *velero) CanCreateBackup(phase v1.BackupPhase) bool {
	switch phase {
	// case v1.BackupPhaseCompleted,
	// 	v1.BackupPhaseFailedValidation,
	// 	v1.BackupPhaseFinalizingPartiallyFailed,
	// 	v1.BackupPhasePartiallyFailed: // , v1.BackupPhaseFinalizing, v1.BackupPhaseInProgress, v1.BackupPhaseNew
	case Started:
		return true
	default:
		return false
	}
}

func (v *velero) GetBackupName(name string) (string, error) {
	var pos = strings.LastIndex(name, "-")
	if pos <= 0 {
		return "", fmt.Errorf("backup name invalid")
	}
	return name[:pos], nil
}

func (v *velero) GetBackupJobLastTime(name string) uint64 {
	var pos = strings.LastIndex(name, "-")
	if pos <= 0 {
		return 0
	}

	str := name[pos+1:]
	res, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return 0
	}
	return res

}

func (v *velero) SetBackupConfigPBMName(ctx context.Context, backupConfig *sysv1.BackupConfig, pbmName string) error {
	client, err := v.factory.Sysv1Client()
	if err != nil {
		return err
	}

	if backupConfig.Annotations == nil {
		backupConfig.Annotations = make(map[string]string)
	}
	backupConfig.Annotations[AnnotationPerconaMongoClusterLastBackupPBMName] = pbmName

	_, err = client.SysV1().BackupConfigs(backupConfig.Namespace).Update(ctx, backupConfig, metav1.UpdateOptions{})

	return err
}
