package handlers

import (
	"context"
	"fmt"
	"sort"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
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

func (o *RestoreHandler) ListRestores(ctx context.Context, owner string, page int64, limit int64) (*sysv1.RestoreList, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	restores, err := c.SysV1().Restores(constant.DefaultOsSystemNamespace).List(ctx, metav1.ListOptions{
		Limit: limit,
	})

	if err != nil {
		return nil, err
	}

	if restores == nil || restores.Items == nil || len(restores.Items) == 0 {
		return nil, fmt.Errorf("restores not exists")
	}

	sort.Slice(restores.Items, func(i, j int) bool {
		return !restores.Items[i].ObjectMeta.CreationTimestamp.Before(&restores.Items[j].ObjectMeta.CreationTimestamp)
	})

	return restores, nil
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
		Path: GetRestorePath(restore),
	}

	params.Password, err = getBackupPassword(backup.Spec.Owner, backup.Spec.Name)
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("get restore password error: %v", err))
	}

	params.Location, params.LocationConfig, err = GetBackupLocationConfig(backup)
	if err != nil {
		return nil, err
	}

	return params, err
}

func (o *RestoreHandler) createRestoreOption(params *RestoreParams) options.Option {
	switch params.Location {
	case constant.BackupLocationSpace.String():
		return &options.SpaceRestoreOptions{
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
			Location:           params.Location,
			LocationConfigName: params.LocationConfig["name"],
			Path:               params.Path,
			Password:           params.Password,
		}
	case constant.BackupLocationTencentCloud.String():
		return &options.TencentCloudRestoreOptions{
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

func (o *RestoreHandler) executeRestore(ctx context.Context,
	backup *sysv1.Backup, snapshot *sysv1.Snapshot, restore *sysv1.Restore,
	opt options.Option, params *RestoreParams) error {
	storage := storage.NewStorage(o.factory, backup.Spec.Owner)
	restoreOutput, restoreErr := storage.Restore(ctx, backup, snapshot, restore, opt)

	if restoreErr != nil {
		log.Errorf("restore %s snapshot %s error: %v", backup.Spec.Name, restore.Spec.SnapshotId, restoreErr)
	} else {
		log.Infof("restore %s snapshot %s completed", backup.Spec.Name, restore.Spec.SnapshotId)
	}

	err := o.updateRestoreFinishedStatus(restoreOutput, restoreErr, restore)

	return err
}

// TODO 是不是需要检查下  snapshot 是否成功完成
func (o *RestoreHandler) Restore(ctx context.Context, restoreId string) error {
	var err error
	restore, err := o.GetRestore(ctx, restoreId)
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

	opt := o.createRestoreOption(params)

	return o.executeRestore(ctx, backup, snapshot, restore, opt, params)
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
	sc, err := o.factory.Sysv1Client()
	if err != nil {
		return err
	}

	r, err := o.GetRestore(ctx, restoreId)
	if err != nil {
		return err
	}
	r.Spec = *restoreSpec

RETRY:
	_, err = sc.SysV1().Restores(constant.DefaultOsSystemNamespace).Update(ctx, r, metav1.UpdateOptions{
		FieldManager: constant.RestoreController,
	})

	if err != nil && apierrors.IsConflict(err) {
		log.Warnf("update restore %s spec retry", restoreId)
		goto RETRY
	} else if err != nil {
		return errors.WithStack(fmt.Errorf("update restore error: %v", err))
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

func (o *RestoreHandler) SetRestorePhase(restoreId string, phase constant.Phase) error {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return err
	}

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    10,
	}

	if err = retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		var ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		r, err := c.SysV1().Restores(constant.DefaultOsSystemNamespace).Get(ctx, restoreId, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("retry")
		}

		r.Spec.Phase = pointer.String(phase.String())
		_, err = c.SysV1().Restores(constant.DefaultOsSystemNamespace).
			Update(ctx, r, metav1.UpdateOptions{})
		if err != nil && apierrors.IsConflict(err) {
			return fmt.Errorf("retry")
		} else if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}
