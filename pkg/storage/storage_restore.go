package storage

import (
	"context"
	"fmt"
	"path"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
)

type StorageRestore struct {
	Handlers  handlers.Interface
	RestoreId string
	Ctx       context.Context
	Cancel    context.CancelFunc

	Backup      *sysv1.Backup
	Snapshot    *sysv1.Snapshot
	Restore     *sysv1.Restore
	RestoreType *handlers.RestoreType

	Params *RestoreParameters

	LastProgressPercent int
	LastProgressTime    time.Time
}

type RestoreParameters struct {
	BackupId string
	Password string
	Path     string
	Location map[string]string
}

func (s *StorageRestore) RunRestore() error {
	if err := s.checkRestoreExists(); err != nil {
		return errors.WithStack(err)
	}

	var f = func() error {
		var e error
		if e = s.prepareRestoreParams(); e != nil {
			return errors.WithStack(e)
		}

		if e = s.prepareForRun(); e != nil {
			return errors.WithStack(e)
		}
		return nil
	}

	if err := f(); err != nil {
		if err := s.updateRestoreResult(nil, err); err != nil {
			return err
		}
		return nil
	}

	restoreResult, restoreErr := s.execute()

	if restoreErr != nil {
		log.Errorf("Restore %s error: %v", s.RestoreId, restoreErr)
	} else {
		log.Infof("Restore %s success, result: %s", s.RestoreId, util.ToJSON(restoreResult))
	}

	if err := s.updateRestoreResult(restoreResult, restoreErr); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (s *StorageRestore) checkRestoreExists() error {
	restore, err := s.Handlers.GetRestoreHandler().GetById(s.Ctx, s.RestoreId)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("get restore %s error: %v", s.RestoreId, err)
	}

	if restore == nil {
		return fmt.Errorf("restore %s not exists", s.RestoreId)
	}

	restoreType, err := handlers.ParseRestoreType(restore)
	if err != nil {
		return fmt.Errorf("restore %s type %v invalid", s.RestoreId, restore.Spec.RestoreType)
	}

	log.Infof("restore %s, data: %s", s.RestoreId, util.ToJSON(restoreType))
	if restoreType.Type == constant.RestoreTypeSnapshot {
		snapshot, err := s.Handlers.GetSnapshotHandler().GetById(s.Ctx, restoreType.SnapshotId)
		if err != nil {
			return fmt.Errorf("snapshot not found: %v", err)
		}
		backup, err := s.Handlers.GetBackupHandler().GetById(s.Ctx, snapshot.Spec.BackupId)
		if err != nil {
			return fmt.Errorf("backup not found: %v", err)
		}

		s.Backup = backup
		s.Snapshot = snapshot
	}

	s.Restore = restore
	s.RestoreType = restoreType

	return nil
}

func (s *StorageRestore) prepareRestoreParams() error {
	var backupId string
	var password string
	var locationConfig = make(map[string]string)
	var err error
	var external bool

	if s.RestoreType.Type == constant.RestoreTypeSnapshot {
		backupId = s.Backup.Name

		password, err = handlers.GetBackupPassword(s.Ctx, s.Backup.Spec.Owner, s.Backup.Spec.Name)
		if err != nil {
			return fmt.Errorf("Restore %s get password error: %v", s.RestoreId, err)
		}

		userspacePath, err := handlers.GetUserspacePvc(s.Backup.Spec.Owner)
		if err != nil {
			return fmt.Errorf("Restore %s, get userspace pvc error: %v", s.RestoreId, err)
		}

		locationConfig, err = handlers.GetBackupLocationConfig(s.Backup)
		if err != nil {
			return fmt.Errorf("Restore %s get location config error: %v", s.RestoreId, err)
		}
		if locationConfig == nil {
			return fmt.Errorf("Restore %s location config not found", s.RestoreId)
		}

		location := locationConfig["location"]
		if location == constant.BackupLocationFileSystem.String() {
			locPath := locationConfig["path"]
			external, locPath = handlers.TrimPathPrefix(locPath)
			if external {
				locationConfig["path"] = path.Join(constant.ExternalPath, locPath)
			} else {
				locationConfig["path"] = path.Join(userspacePath, locPath)
			}

		}
	} else {
		// backupUrl
		log.Infof("restore from backupUrl, ready to get integration token, owner: %s, location: %s", s.RestoreType.Owner, s.RestoreType.Location)
		integrationName, err := integration.IntegrationManager().GetIntegrationNameByLocation(s.Ctx, s.RestoreType.Owner, s.RestoreType.Location)
		if err != nil {
			log.Errorf("get restore integration name error: %v", err)
			return err
		}

		locationConfig["location"] = s.RestoreType.Location
		locationConfig["name"] = integrationName
		locationConfig["cloudName"] = s.RestoreType.BackupUrl.CloudName
		locationConfig["regionId"] = s.RestoreType.BackupUrl.RegionId
		locationConfig["clusterId"] = s.RestoreType.ClusterId
		locationConfig["suffix"] = s.RestoreType.BackupUrl.TerminusSuffix
		locationConfig["path"] = s.RestoreType.BackupUrl.FilesystemPath

		p, _ := util.Base64decode(s.RestoreType.Password)
		password = string(p)

		backupId = s.RestoreType.BackupUrl.BackupId
	}

	userspacePvc, err := handlers.GetUserspacePvc(s.Restore.Spec.Owner)
	if err != nil {
		return err
	}

	var restorePath string
	var tmpRestoreExternal, tmpRestorePath = handlers.TrimPathPrefix(s.RestoreType.Path)
	if tmpRestoreExternal {
		restorePath = path.Join(constant.ExternalPath, tmpRestorePath)
	} else {
		restorePath = path.Join(userspacePvc, tmpRestorePath)
	}

	log.Infof("restore: %s, locationConfig: %v", s.RestoreId, util.ToJSON(locationConfig))
	s.Params = &RestoreParameters{
		BackupId: backupId,
		Path:     restorePath,
		Password: password,
		Location: locationConfig,
	}

	return nil
}

func (s *StorageRestore) prepareForRun() error {
	s.Handlers.GetNotification().Send(s.Ctx, constant.EventRestore, s.RestoreType.Owner, "restore running", map[string]interface{}{
		"id":       s.Restore.Name,
		"progress": 0,
		"status":   constant.Running.String(),
		"message":  "",
	})

	return s.Handlers.GetRestoreHandler().UpdatePhase(s.Ctx, s.Restore.Name, constant.Running.String())
}

func (s *StorageRestore) progressCallback(percentDone float64) {

	select {
	case <-s.Ctx.Done():
		return
	default:
	}

	var percent = int(percentDone * progressDone)

	if time.Since(s.LastProgressTime) >= progressInterval*time.Second && s.LastProgressPercent != percent {
		s.LastProgressPercent = percent
		s.LastProgressTime = time.Now()

		s.Handlers.GetRestoreHandler().UpdateProgress(s.Ctx, s.RestoreId, percent)

		s.Handlers.GetNotification().Send(s.Ctx, constant.EventRestore, s.RestoreType.Owner, "restore running", map[string]interface{}{
			"id":       s.RestoreId,
			"progress": percent,
			"status":   constant.Running.String(),
			"message":  "",
		})
	}
}

func (s *StorageRestore) execute() (restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error) {
	var isSpaceRestore bool
	var logger = log.GetLogger()
	var resticSnapshotId = s.RestoreType.ResticSnapshotId
	var location = s.Params.Location["location"]

	log.Infof("Restore %s prepare: %s, resticSnapshotId: %s, location: %s", s.RestoreId, s.RestoreType.Type, resticSnapshotId, location)

	var restoreService *backupssdkstorage.RestoreService

	switch location {
	case constant.BackupLocationSpace.String():
		isSpaceRestore = true
		restoreOutput, restoreError = s.restoreFromSpace()
	case constant.BackupLocationAwsS3.String():
		token, err := s.getIntegrationCloud() // s3
		if err != nil {
			restoreError = fmt.Errorf("get %s token error: %v", token.Type, err)
			return
		}
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: s.Params.Password,
			Operator: constant.StorageOperatorApp,
			Ctx:      s.Ctx,
			Logger:   logger,
			Aws: &backupssdkoptions.AwsRestoreOption{
				RepoName:        s.Params.BackupId,
				SnapshotId:      resticSnapshotId,
				Path:            s.Params.Path,
				Endpoint:        token.Endpoint,
				AccessKey:       token.AccessKey,
				SecretAccessKey: token.SecretKey,
			},
		})
	case constant.BackupLocationTencentCloud.String():
		token, err := s.getIntegrationCloud() // cos
		if err != nil {
			restoreError = fmt.Errorf("get %s token error: %v", token.Type, err)
			return
		}
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: s.Params.Password,
			Operator: constant.StorageOperatorApp,
			Ctx:      s.Ctx,
			Logger:   logger,
			TencentCloud: &backupssdkoptions.TencentCloudRestoreOption{
				RepoName:        s.Params.BackupId,
				SnapshotId:      resticSnapshotId,
				Path:            s.Params.Path,
				Endpoint:        token.Endpoint,
				AccessKey:       token.AccessKey,
				SecretAccessKey: token.SecretKey,
			},
		})
	case constant.BackupLocationFileSystem.String():
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: s.Params.Password,
			Operator: constant.StorageOperatorApp,
			Ctx:      s.Ctx,
			Logger:   logger,
			Filesystem: &backupssdkoptions.FilesystemRestoreOption{
				RepoName:   fmt.Sprintf("olares-backup-%s", s.RestoreType.BackupName), //s.Params.BackupId,
				SnapshotId: resticSnapshotId,
				Endpoint:   s.Params.Location["path"],
				Path:       s.Params.Path,
			},
		})
	}

	if !isSpaceRestore {
		restoreOutput, restoreError = restoreService.Restore(s.progressCallback)
	}

	return
}

func (s *StorageRestore) restoreFromSpace() (restoreOutput *backupssdkrestic.RestoreSummaryOutput, err error) {
	var backupId string
	var terminusSuffix string
	var owner = s.RestoreType.Owner
	if s.RestoreType.Type == constant.RestoreTypeSnapshot {
		backupId = s.Backup.Name
	} else {
		backupId = s.RestoreType.BackupUrl.BackupId
		terminusSuffix = s.RestoreType.BackupUrl.TerminusSuffix
	}

	var spaceToken *integration.SpaceToken
	var resticSnapshotId = s.RestoreType.ResticSnapshotId
	var location = s.Params.Location

	for {
		spaceToken, err = integration.IntegrationManager().GetIntegrationSpaceToken(s.Ctx, owner, location["name"]) // restoreFromSpace
		if err != nil {
			err = fmt.Errorf("get space token error: %v", err)
			break
		}

		if util.IsTimestampNearingExpiration(spaceToken.ExpiresAt) {
			err = fmt.Errorf("space access token expired %d(%s)", spaceToken.ExpiresAt, util.ParseUnixMilliToDate(spaceToken.ExpiresAt))
			break
		}

		var spaceRestoreOption = &backupssdkoptions.SpaceRestoreOption{
			RepoName:       backupId,
			RepoSuffix:     terminusSuffix, // only used for backupUrl
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
			Operator: constant.StorageOperatorApp,
			Ctx:      s.Ctx,
			Logger:   log.GetLogger(),
			Space:    spaceRestoreOption,
		})

		restoreOutput, err = restoreService.Restore(s.progressCallback)

		if err != nil {
			if strings.Contains(err.Error(), "refresh-token error") {
				spaceToken, err = integration.IntegrationManager().GetIntegrationSpaceToken(s.Ctx, owner, location["name"]) // restoreFromSpace
				if err != nil {
					err = fmt.Errorf("get space token error: %v", err)
					break
				}
				continue
			} else {
				err = fmt.Errorf("space backup error: %v", err)
				break
			}
		}
		break
	}

	return
}

func (s *StorageRestore) updateRestoreResult(restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error) error {
	var msg, phase string
	var notifyProgress int

	restore, err := s.Handlers.GetRestoreHandler().GetRestore(s.Ctx, s.RestoreId)
	if err != nil {
		return err
	}

	if restoreError != nil {
		msg = restoreError.Error()
		phase = constant.Failed.String()
		notifyProgress = restore.Spec.Progress
		restore.Spec.Phase = pointer.String(constant.Failed.String())
		restore.Spec.Message = pointer.String(restoreError.Error())
		restore.Spec.ResticPhase = pointer.String(phase)
	} else {
		msg = constant.Completed.String()
		phase = constant.Completed.String()
		notifyProgress = progressDone
		restore.Spec.Size = pointer.UInt64Ptr(restoreOutput.TotalBytes)
		restore.Spec.Progress = progressDone
		restore.Spec.Phase = pointer.String(phase)
		restore.Spec.Message = pointer.String(phase)
		restore.Spec.ResticPhase = pointer.String(phase)
		restore.Spec.ResticMessage = pointer.String(util.ToJSON(restoreOutput))
	}

	restore.Spec.EndAt = pointer.Time()

	s.Handlers.GetNotification().Send(s.Ctx, constant.EventRestore, s.RestoreType.Owner, "restore running", map[string]interface{}{
		"id":       s.Restore.Name,
		"progress": notifyProgress,
		"endat":    restore.Spec.EndAt.Unix(),
		"status":   phase,
		"message":  msg,
	})

	return s.Handlers.GetRestoreHandler().Update(s.Ctx, s.RestoreId, &restore.Spec)
}

func (s *StorageRestore) getIntegrationCloud() (*integration.IntegrationToken, error) {
	var l = s.Params.Location
	var location = l["location"]
	var locationIntegrationName = l["name"]

	return integration.IntegrationManager().GetIntegrationCloudToken(s.Ctx, s.Restore.Spec.Owner, location, locationIntegrationName)
}
