package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	integration "bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	backupssdk "bytetrade.io/web3os/backups-sdk"
	"github.com/pkg/errors"

	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
)

type StorageRestore struct {
	Handlers  handlers.Interface
	RestoreId string
	Ctx       context.Context
	Cancel    context.CancelFunc

	Backup   *sysv1.Backup
	Snapshot *sysv1.Snapshot
	Restore  *sysv1.Restore

	Params *RestoreParameters
}

type RestoreParameters struct {
	Password string
	Path     string
	Location map[string]string
}

func (s *StorageRestore) RunRestore() error {
	if err := s.checkRestoreExists(); err != nil {
		return errors.WithStack(err)
	}

	// todo update failed phase
	if err := s.prepareRestoreParams(); err != nil {
		return errors.WithStack(err)
	}

	if err := s.prepareForRun(); err != nil {
		return errors.WithStack(err)
	}

	var backupName = s.Backup.Spec.Name
	var snapshotId = s.Snapshot.Name

	restoreResult, restoreErr := s.execute()
	if restoreErr != nil {
		log.Errorf("Restore %s-%s error %v", backupName, snapshotId, restoreErr)
	} else {
		log.Infof("Restore %s-%s success", backupName, snapshotId)
	}

	if err := s.updateRestoreResult(restoreResult, restoreErr); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (s *StorageRestore) checkRestoreExists() error {
	restore, err := s.Handlers.GetRestoreHandler().GetRestore(s.Ctx, s.RestoreId)
	if err != nil {
		return fmt.Errorf("restore not found: %v", err)
	}

	snapshot, err := s.Handlers.GetSnapshotHandler().GetById(s.Ctx, restore.Spec.SnapshotId)
	if err != nil {
		return fmt.Errorf("snapshot not found: %v", err)
	}
	backup, err := s.Handlers.GetBackupHandler().GetById(s.Ctx, snapshot.Spec.BackupId)
	if err != nil {
		return fmt.Errorf("backup not found: %v", err)
	}

	s.Backup = backup
	s.Snapshot = snapshot
	s.Restore = restore

	return nil
}

func (s *StorageRestore) prepareRestoreParams() error {
	var backupName = s.Backup.Spec.Name
	var snapshotId = s.Snapshot.Name
	password, err := s.Handlers.GetBackupHandler().GetBackupPassword(s.Ctx, s.Backup)
	if err != nil {
		return fmt.Errorf("Restore %s-%s get password error %v", s.Backup.Spec.Name, s.Snapshot.Name, err)
	}

	locationConfig, err := handlers.GetBackupLocationConfig(s.Backup) // TODO
	if err != nil {
		return fmt.Errorf("Backup %s-%s get location config error %v", backupName, snapshotId, err)
	}
	if locationConfig == nil {
		return fmt.Errorf("Backup %s-%s location config not found", backupName, snapshotId)
	}

	s.Params = &RestoreParameters{
		Path:     handlers.GetRestorePath(s.Restore),
		Password: password,
		Location: locationConfig,
	}

	return nil
}

func (s *StorageRestore) prepareForRun() error {
	return s.Handlers.GetRestoreHandler().UpdatePhase(s.Ctx, s.Restore.Name, constant.Running.String())
}

// --
func (s *StorageRestore) execute() (restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error) {
	var isSpaceRestore bool
	var logger = log.GetLogger()
	var backupName = s.Backup.Spec.Name
	var snapshotId = s.Restore.Spec.SnapshotId
	var resticSnapshotId = *s.Snapshot.Spec.SnapshotId
	var location = s.Params.Location["location"] // todo review

	log.Infof("Restore %s-%s prepare: %s", backupName, snapshotId)

	var restoreService *backupssdkstorage.RestoreService

	switch location {
	case "space":
		isSpaceRestore = true
		restoreOutput, restoreError = s.restoreFromSpace()
	case "awss3":
		token, err := s.getIntegrationCloud()
		if err != nil {
			restoreError = fmt.Errorf("get %s token error %v", token.Type, err)
			return
		}
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: s.Params.Password,
			Logger:   logger,
			Aws: &backupssdkoptions.AwsRestoreOption{
				RepoName:        s.Backup.Name,
				SnapshotId:      resticSnapshotId,
				Path:            s.Params.Path,
				Endpoint:        token.Endpoint,
				AccessKey:       token.AccessKey,
				SecretAccessKey: token.SecretKey,
			},
		})
	case "tencentcloud":
		token, err := s.getIntegrationCloud()
		if err != nil {
			restoreError = fmt.Errorf("get %s token error %v", token.Type, err)
			return
		}
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: s.Params.Password,
			Logger:   logger,
			TencentCloud: &backupssdkoptions.TencentCloudRestoreOption{
				RepoName:        s.Backup.Name,
				SnapshotId:      resticSnapshotId,
				Path:            s.Params.Path,
				Endpoint:        token.Endpoint,
				AccessKey:       token.AccessKey,
				SecretAccessKey: token.SecretKey,
			},
		})
	case "filesystem":
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: s.Params.Password,
			Logger:   logger,
			Filesystem: &backupssdkoptions.FilesystemRestoreOption{
				RepoName:   s.Backup.Name,
				SnapshotId: resticSnapshotId,
				Endpoint:   "", // TODO
				Path:       s.Params.Path,
			},
		})
	}

	if !isSpaceRestore {
		restoreOutput, restoreError = restoreService.Restore()
	}

	return
}

func (s *StorageRestore) restoreFromSpace() (restoreOutput *backupssdkrestic.RestoreSummaryOutput, err error) {
	var backupId = s.Backup.Name
	var resticSnapshotId = *s.Snapshot.Spec.SnapshotId
	var location = s.Params.Location

	for {
		var spaceToken, err = integration.IntegrationManager().GetIntegrationSpaceToken(s.Ctx, s.Backup.Spec.Owner, location["name"])
		if err != nil {
			err = fmt.Errorf("get space token error %v", err)
			break
		}

		if util.IsTimestampNearingExpiration(spaceToken.ExpiresAt) {
			err = fmt.Errorf("space access token expired %d(%s)", spaceToken.ExpiresAt, util.ParseUnixMilliToDate(spaceToken.ExpiresAt))
			break
		}

		var spaceRestoreOption = &backupssdkoptions.SpaceRestoreOption{
			RepoName:       backupId,
			SnapshotId:     resticSnapshotId,
			Path:           s.Params.Path,
			OlaresDid:      spaceToken.OlaresDid,
			AccessToken:    spaceToken.AccessToken,
			ClusterId:      location["clusterId"],
			CloudName:      location["cloudName"],
			RegionId:       location["regionId"],
			CloudApiMirror: constant.DefaultSyncServerURL,
		}

		var restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: s.Params.Password,
			Ctx:      s.Ctx,
			Logger:   log.GetLogger(),
			Space:    spaceRestoreOption,
		})

		restoreOutput, err = restoreService.Restore()

		if err != nil {
			if strings.Contains(err.Error(), "refresh-token error") {
				spaceToken, err = integration.IntegrationManager().GetIntegrationSpaceToken(s.Ctx, s.Backup.Spec.Owner, location["name"])
				if err != nil {
					err = fmt.Errorf("get space token error %v", err)
					break
				}
				continue
			} else {
				err = fmt.Errorf("space backup error %v", err)
				break
			}
		}
		break
	}

	return
}

func (s *StorageRestore) updateRestoreResult(restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error) error {
	restore, err := s.Handlers.GetRestoreHandler().GetRestore(s.Ctx, s.RestoreId)
	if err != nil {
		return err
	}

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

	return s.Handlers.GetRestoreHandler().Update(s.Ctx, s.RestoreId, &restore.Spec)
}

func (s *StorageRestore) getIntegrationCloud() (*integration.IntegrationToken, error) {
	var l = s.Params.Location
	var location = l["location"]
	var locationIntegrationName = l["name"]
	return integration.IntegrationManager().GetIntegrationCloudToken(s.Ctx, s.Backup.Spec.Owner, location, locationIntegrationName)
}
