package handlers

import (
	"context"
	"fmt"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/converter"
	"bytetrade.io/web3os/backup-server/pkg/options"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	"bytetrade.io/web3os/backup-server/pkg/util/uuid"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type RestoreHandler struct {
	factory  client.Factory
	handlers Interface
}

func NewRestoreHandler(f client.Factory, handlers Interface) *RestoreHandler {
	return &RestoreHandler{
		factory:  f,
		handlers: handlers,
	}
}

func (o *RestoreHandler) ListRestores() {

}

func (o *RestoreHandler) CreateRestore(ctx context.Context, restoreTypeName string, restoreType map[string]string) (*sysv1.Restore, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	var startAt = time.Now().UnixMilli()
	var phase = constant.Pending.String()

	var restore = &sysv1.Restore{
		TypeMeta: metav1.TypeMeta{
			Kind:       constant.KindRestore,
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewUUID(),
			Namespace: constant.DefaultOsSystemNamespace,
			Labels: map[string]string{
				"snapshot-id": restoreType["snapshotId"],
			},
		},
		Spec: sysv1.RestoreSpec{
			SnapshotId: restoreType["snapshotId"],
			RestoreType: map[string]string{
				restoreTypeName: util.ToJSON(restoreType),
			},
			StartAt: startAt,
			Phase:   &phase,
		},
	}

	created, err := c.SysV1().Restores(constant.DefaultOsSystemNamespace).Create(ctx, restore, metav1.CreateOptions{FieldManager: constant.RestoreController})
	if err != nil {
		return nil, err
	}

	return created, nil
}

// Restore

type RestoreParams struct {
	Path           string
	Password       string
	Location       string
	LocationConfig map[string]string
}

func (o *RestoreHandler) prepareRestoreParams(ctx context.Context, backup *sysv1.Backup, snapshot *sysv1.Snapshot, restore *sysv1.Restore) (*RestoreParams, error) {
	var err error
	params := &RestoreParams{
		Path: getRestorePath(restore),
	}

	params.Password, err = getBackupPassword(backup.Spec.Owner, backup.Spec.Name)
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get restore password error: %v", err))
	}

	params.Location, params.LocationConfig, err = getBackupLocationConfig(backup)
	if err != nil {
		return nil, err
	}

	return params, err
}

func (o *RestoreHandler) createRestoreOption(backupName string, restore *sysv1.Restore, params *RestoreParams) options.Option {
	switch params.Location {
	case constant.BackupLocationSpace.String():
		return &options.SpaceRestoreOptions{
			RepoName:       backupName,
			SnapshotId:     restore.Spec.SnapshotId,
			Location:       params.Location,
			OlaresId:       params.LocationConfig["name"],
			ClusterId:      params.LocationConfig["clusterId"],
			CloudName:      params.LocationConfig["cloudName"],
			RegionId:       params.LocationConfig["regionId"],
			Path:           params.Path,
			Password:       params.Password,
			CloudApiMirror: constant.DefaultSyncServerURL,
		}
	case constant.BackupLocationAwsS3.String():
		return &options.AwsS3RestoreOptions{
			RepoName:           backupName,
			SnapshotId:         restore.Spec.SnapshotId,
			Location:           params.Location,
			LocationConfigName: params.LocationConfig["name"],
			Path:               params.Path,
			Password:           params.Password,
		}
	case constant.BackupLocationTencentCloud.String():
		return &options.TencentCloudRestoreOptions{
			RepoName:           backupName,
			SnapshotId:         restore.Spec.SnapshotId,
			Location:           params.Location,
			LocationConfigName: params.LocationConfig["name"],
			Path:               params.Path,
			Password:           params.Password,
		}
		// todo
		// case constant.BackupLocationFileSystem.String():
	}

	return nil
}

func (o *RestoreHandler) executeRestore(ctx context.Context, backup *sysv1.Backup,
	restore *sysv1.Restore,
	opt options.Option, params *RestoreParams) error {
	storage := storage.NewStorage(o.factory, backup.Spec.Owner)
	restoreOutput, restoreErr := storage.Restore(ctx, opt)

	if restoreErr != nil {
		log.Errorf("restore %s snapshot %s error: %v", backup.Spec.Name, restore.Spec.SnapshotId, restoreErr)
	} else {
		log.Infof("restore %s snapshot %s completed", backup.Spec.Name, restore.Spec.SnapshotId)
	}

	err := o.updateRestoreFinishedStatus(restoreOutput, restoreErr, restore)

	return err
}

func (o *RestoreHandler) Restore(ctx context.Context, snapshotId string) error {
	var err error
	restore, err := o.GetRestore(ctx, snapshotId)
	if err != nil {
		return fmt.Errorf("get restore error: %v", err)
	}

	snapshot, err := o.handlers.GetSnapshotHandler().GetSnapshot(ctx, restore.Spec.SnapshotId)
	if err != nil {
		return fmt.Errorf("get snapshot error: %v", err)
	}

	backup, err := o.handlers.GetBackupHandler().GetBackupById(ctx, snapshot.Spec.BackupId)
	if err != nil {
		return fmt.Errorf("get backup error: %v", snapshot.Spec.BackupId, err)
	}

	params, err := o.prepareRestoreParams(ctx, backup, snapshot, restore)
	if err != nil {
		return errors.WithMessage(err, o.updateRestoreFailedStatus(err, restore).Error())
	}

	if err := o.updateRestoreToRunning(ctx, restore); err != nil {
		return errors.WithMessage(err, o.updateRestoreFailedStatus(err, restore).Error())
	}

	opt := o.createRestoreOption(backup.Spec.Name, restore, params)

	return o.executeRestore(ctx, backup, restore, opt, params)
}

func (o *RestoreHandler) updateRestoreToRunning(ctx context.Context, restore *sysv1.Restore) error {
	restore.Spec.Phase = pointer.String(constant.Running.String())
	return o.Update(ctx, restore.Name, &restore.Spec) // update running
}

func (o *RestoreHandler) updateRestoreFailedStatus(backupError error, restore *sysv1.Restore) error {
	restore.Spec.Phase = pointer.String(constant.Failed.String())
	restore.Spec.Message = pointer.String(backupError.Error())
	restore.Spec.EndAt = time.Now().UnixMilli()

	return o.Update(context.Background(), restore.Name, &restore.Spec) // update failed
}

func (o *RestoreHandler) updateRestoreFinishedStatus(
	restoreOutput *backupssdkrestic.RestoreSummaryOutput,
	restoreError error,
	restore *sysv1.Restore) error {
	// todo backupRepo

	if restoreError != nil {
		restore.Spec.Phase = pointer.String(constant.Failed.String())
		restore.Spec.Message = pointer.String(restoreError.Error())
		restore.Spec.ResticPhase = pointer.String(constant.Failed.String())
	} else {
		restore.Spec.Size = pointer.UInt64Ptr(restoreOutput.TotalBytes)
		restore.Spec.Phase = pointer.String(constant.Completed.String())
		restore.Spec.ResticPhase = pointer.String(constant.Completed.String())
		restore.Spec.ResticMessage = pointer.String(util.ToJSON(restoreOutput))
	}

	restore.Spec.EndAt = time.Now().UnixMilli()

	return o.Update(context.Background(), restore.Name, &restore.Spec) // update finished
}

func (o *RestoreHandler) Update(ctx context.Context, restoreId string, restoreSpec *sysv1.RestoreSpec) error {
	r, err := o.GetRestore(ctx, restoreId)
	if err != nil {
		return err
	}

RETRY:
	var restore = &sysv1.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: constant.DefaultOsSystemNamespace,
			// Labels:    r.Labels,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Restore",
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		Spec: *restoreSpec,
	}

	obj, err := converter.ToUnstructured(restore)
	if err != nil {
		return err
	}

	res := unstructured.Unstructured{Object: obj}
	res.SetGroupVersionKind(restore.GroupVersionKind())

	dynamicClient, err := o.factory.DynamicClient()
	if err != nil {
		return err
	}

	_, err = dynamicClient.Resource(constant.RestoreGVR).Namespace(constant.DefaultOsSystemNamespace).Update(ctx, &res, metav1.UpdateOptions{FieldManager: constant.RestoreController})

	if err != nil && apierrors.IsConflict(err) {
		log.Warnf("update restore %s spec retry", restoreId)
		goto RETRY
	} else if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (o *RestoreHandler) GetRestore(ctx context.Context, restoreId string) (*sysv1.Restore, error) {
	var ctxTimeout, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	restore, err := c.SysV1().Restores(constant.DefaultOsSystemNamespace).Get(ctxTimeout, restoreId, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	if restore == nil {
		return nil, nil
	}

	return restore, nil
}
