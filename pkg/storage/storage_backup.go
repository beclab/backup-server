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
	"bytetrade.io/web3os/backup-server/pkg/notify"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	backupssdk "bytetrade.io/web3os/backups-sdk"
	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
	backupssdkmodel "bytetrade.io/web3os/backups-sdk/pkg/storage/model"
	"github.com/pkg/errors"
)

type StorageBackup struct {
	Handlers   handlers.Interface
	SnapshotId string
	Ctx        context.Context
	Cancel     context.CancelFunc

	Backup       *sysv1.Backup
	Snapshot     *sysv1.Snapshot
	Params       *BackupParameters
	SnapshotType *int

	LastProgressPercent int
	LastProgressTime    time.Time
}

type BackupParameters struct {
	Password     string
	Path         string
	Location     map[string]string
	SnapshotType string
}

func (s *StorageBackup) RunBackup() error {
	if err := s.checkBackupExists(); err != nil {
		return errors.WithStack(err)
	}

	var backupName = s.Backup.Spec.Name
	var snapshotId = s.Snapshot.Name

	var f = func() error {
		var e error
		if e = s.validateSnapshotPreconditions(); e != nil {
			return errors.WithStack(e)
		}

		if e = s.checkSnapshotType(); e != nil {
			return errors.WithStack(e)
		}

		if e = s.prepareBackupParams(); e != nil {
			return errors.WithStack(e)
		}

		if e = s.prepareForRun(); e != nil {
			return errors.WithStack(e)
		}

		return nil
	}
	if err := f(); err != nil {
		// if e := s.notifyBackupResult(nil, nil, err); e != nil {
		// 	log.Errorf("Backup %s,%s, notify backup terminate error: %v", backupName, snapshotId, err)
		// } else {
		// 	log.Infof("Backup %s,%s, notify backup terminate success", backupName, snapshotId)
		// }
		log.Errorf("Backup %s,%s, prepare for run error: %v", backupName, snapshotId, err)
		if e := s.updateBackupResult(nil, nil, 0, err); e != nil {
			return errors.WithStack(e)
		}

		return nil
	}

	log.Infof("Backup %s,%s, locationConfig: %s", backupName, snapshotId, util.ToJSON(s.Params.Location))
	backupResult, backupStorageObj, backupTotalSize, backupErr := s.execute()
	if backupErr != nil {
		log.Errorf("Backup %s,%s, error: %v", backupName, snapshotId, backupErr)
	} else {
		log.Infof("Backup %s,%s, success", backupName, snapshotId)
	}

	// if err := s.notifyBackupResult(backupResult, backupStorageObj, backupErr); err != nil {
	// 	log.Errorf("Backup %s,%s notify backup result error: %v", backupName, snapshotId, err)
	// } else {
	// 	log.Infof("Backup %s,%s notify backup result success", backupName, snapshotId)
	// }
	if err := s.updateBackupResult(backupResult, backupStorageObj, backupTotalSize, backupErr); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (s *StorageBackup) checkBackupExists() error {
	snapshot, err := s.Handlers.GetSnapshotHandler().GetById(s.Ctx, s.SnapshotId)
	if err != nil {
		return fmt.Errorf("snapshot not found: %v", err)
	}
	backup, err := s.Handlers.GetBackupHandler().GetById(s.Ctx, snapshot.Spec.BackupId)
	if err != nil {
		return fmt.Errorf("backup not found: %v", err)
	}

	s.Backup = backup
	s.Snapshot = snapshot

	return nil
}

func (s *StorageBackup) validateSnapshotPreconditions() error {
	var backupName = s.Backup.Spec.Name
	var snapshotId = s.Snapshot.Name
	var phase = *s.Snapshot.Spec.Phase
	if phase != constant.Pending.String() { // other phase ?
		return fmt.Errorf("Backup %s,%s, snapshot phase %s invalid", backupName, snapshotId, phase)
	}
	return nil
}

func (s *StorageBackup) checkSnapshotType() error {
	snapshotType, err := s.Handlers.GetSnapshotHandler().GetSnapshotType(s.Ctx, s.Backup.Name)
	if err != nil {
		return fmt.Errorf("Backup %s,%s, get snapshot type error: %v", s.Backup.Spec.Name, s.Snapshot.Name, err)
	}

	s.SnapshotType = handlers.ParseSnapshotType(snapshotType)
	return nil
}

func (s *StorageBackup) prepareBackupParams() error {
	var backupName = s.Backup.Spec.Name
	var snapshotId = s.Snapshot.Name
	password, err := handlers.GetBackupPassword(s.Ctx, s.Backup.Spec.Owner, s.Backup.Spec.Name)
	if err != nil {
		return fmt.Errorf("Backup %s,%s, get password error: %v", backupName, snapshotId, err)
	}

	userspacePath, err := handlers.GetUserspacePvc(s.Backup.Spec.Owner)
	if err != nil {
		return fmt.Errorf("Backup %s,%s, get userspace pvc error: %v", backupName, snapshotId, err)
	}

	location, err := handlers.GetBackupLocationConfig(s.Backup)
	if err != nil {
		return fmt.Errorf("Backup %s,%s, get location config error: %v", backupName, snapshotId, err)
	}

	if location == nil {
		return fmt.Errorf("Backup %s,%s, location config not exists", backupName, snapshotId)
	}

	loc := location["location"]
	if loc == constant.BackupLocationFileSystem.String() {
		locPath := location["path"]
		locPath = handlers.TrimPathPrefix(locPath)
		location["path"] = path.Join(userspacePath, locPath)
	}

	var backupPath = path.Join(userspacePath, handlers.TrimPathPrefix(handlers.GetBackupPath(s.Backup)))

	s.Params = &BackupParameters{
		Path:     backupPath,
		Password: password,
		Location: location,
	}

	return nil
}

func (s *StorageBackup) prepareForRun() error {
	return s.Handlers.GetSnapshotHandler().UpdatePhase(s.Ctx, s.Snapshot.Name, constant.Running.String())
}

func (s *StorageBackup) progressCallback(percentDone float64) {

	select {
	case <-s.Ctx.Done():
		return
	default:
	}

	var percent = int(percentDone * progressDone)

	if percent == progressDone {
		percent = progressDone - 1
		s.Handlers.GetSnapshotHandler().UpdateProgress(s.Ctx, s.SnapshotId, percent)
		return
	}

	if time.Since(s.LastProgressTime) >= progressInterval*time.Second && s.LastProgressPercent != percent {
		s.Handlers.GetSnapshotHandler().UpdateProgress(s.Ctx, s.SnapshotId, percent)
		s.LastProgressPercent = percent
		s.LastProgressTime = time.Now()
	}
}

func (s *StorageBackup) execute() (backupOutput *backupssdkrestic.SummaryOutput,
	backupStorageObj *backupssdkmodel.StorageInfo, backupTotalSize uint64, backupError error) {
	var isSpaceBackup bool
	var logger = log.GetLogger()
	var backupId = s.Backup.Name
	var backupName = s.Backup.Spec.Name
	var snapshotId = s.Snapshot.Name
	var location = s.Params.Location["location"]

	log.Infof("Backup %s,%s, location: %s, prepare", backupName, snapshotId, location)

	var backupService *backupssdkstorage.BackupService
	var options backupssdkoptions.Option

	switch location {
	case constant.BackupLocationSpace.String():
		isSpaceBackup = true
		backupOutput, backupStorageObj, backupTotalSize, backupError = s.backupToSpace()
	case constant.BackupLocationAwsS3.String():
		token, err := s.getIntegrationCloud()
		if err != nil {
			backupError = fmt.Errorf("get %s token error: %v", token.Type, err)
			return
		}
		options = &backupssdkoptions.AwsBackupOption{
			RepoName:        backupId,
			Path:            s.Params.Path,
			Endpoint:        token.Endpoint,
			AccessKey:       token.AccessKey,
			SecretAccessKey: token.SecretKey,
		}
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: s.Params.Password,
			Operator: constant.StorageOperatorApp,
			Ctx:      s.Ctx,
			Logger:   logger,
			Aws:      options.(*backupssdkoptions.AwsBackupOption),
		})
	case constant.BackupLocationTencentCloud.String():
		token, err := s.getIntegrationCloud()
		if err != nil {
			backupError = fmt.Errorf("get %s token error: %v", token.Type, err)
			return
		}
		options = &backupssdkoptions.TencentCloudBackupOption{
			RepoName:        backupId,
			Path:            s.Params.Path,
			Endpoint:        token.Endpoint,
			AccessKey:       token.AccessKey,
			SecretAccessKey: token.SecretKey,
		}
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password:     s.Params.Password,
			Operator:     constant.StorageOperatorApp,
			Ctx:          s.Ctx,
			Logger:       logger,
			TencentCloud: options.(*backupssdkoptions.TencentCloudBackupOption),
		})
	case constant.BackupLocationFileSystem.String():
		options = &backupssdkoptions.FilesystemBackupOption{
			RepoName: backupId,
			Endpoint: s.Params.Location["path"],
			Path:     s.Params.Path,
		}
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password:   s.Params.Password,
			Operator:   constant.StorageOperatorApp,
			Ctx:        s.Ctx,
			Logger:     logger,
			Filesystem: options.(*backupssdkoptions.FilesystemBackupOption),
		})
	}

	if !isSpaceBackup {
		backupOutput, backupStorageObj, backupError = backupService.Backup(s.progressCallback)
		if backupError == nil {
			stats, err := s.getStats(options)
			if err != nil {
				log.Errorf("Backup %s,%s, get stats error: %v", backupName, snapshotId, err)
			} else {
				log.Infof("Backup %s,%s, get stats: %s", backupName, snapshotId, util.ToJSON(stats))
				backupTotalSize = stats.TotalSize
			}
		}
	}

	return
}

func (s *StorageBackup) backupToSpace() (backupOutput *backupssdkrestic.SummaryOutput, backupStorageObj *backupssdkmodel.StorageInfo, totalSize uint64, err error) {
	var backupId = s.Backup.Name
	var location = s.Params.Location
	var olaresId = location["name"]

	var spaceToken *integration.SpaceToken
	var spaceBackupOption backupssdkoptions.Option

	for {
		// TODO loop forever?
		spaceToken, err = integration.IntegrationManager().GetIntegrationSpaceToken(s.Ctx, s.Backup.Spec.Owner, olaresId)
		if err != nil {
			err = fmt.Errorf("get space token error: %v", err)
			break
		}
		if util.IsTimestampNearingExpiration(spaceToken.ExpiresAt) {
			err = fmt.Errorf("space access token expired %d(%s)", spaceToken.ExpiresAt, util.ParseUnixMilliToDate(spaceToken.ExpiresAt))
			break
		}

		spaceBackupOption = &backupssdkoptions.SpaceBackupOption{
			RepoName:       backupId,
			Path:           s.Params.Path,
			OlaresDid:      spaceToken.OlaresDid,
			AccessToken:    spaceToken.AccessToken,
			ClusterId:      location["clusterId"],
			CloudName:      location["cloudName"],
			RegionId:       location["regionId"],
			CloudApiMirror: constant.DefaultSyncServerURL,
		}

		var backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: s.Params.Password,
			Operator: constant.StorageOperatorApp,
			Ctx:      s.Ctx,
			Logger:   log.GetLogger(),
			Space:    spaceBackupOption.(*backupssdkoptions.SpaceBackupOption),
		})

		backupOutput, backupStorageObj, err = backupService.Backup(s.progressCallback)

		if err != nil {
			if strings.Contains(err.Error(), "refresh-token error") {
				continue
			} else {
				err = fmt.Errorf("space backup error: %v", err)
				break
			}
		}
		break
	}
	if err == nil {
		stats, err := s.getStats(spaceBackupOption)
		if err != nil {

			log.Errorf("Backup %s,%s, get stats error: %v", s.Backup.Spec.Name, s.SnapshotId, err)
		} else {
			totalSize = stats.TotalSize
		}
	}

	return
}

func (s *StorageBackup) getStats(opt backupssdkoptions.Option) (*backupssdkrestic.StatsContainer, error) {
	var options = &backupssdkstorage.SnapshotsOption{
		Password: s.Params.Password,
		Logger:   log.GetLogger(),
	}

	switch opt.(type) {
	case *backupssdkoptions.SpaceBackupOption:
		var location = s.Params.Location
		var olaresId = location["name"]
		spaceToken, err := integration.IntegrationManager().GetIntegrationSpaceToken(s.Ctx, s.Backup.Spec.Owner, olaresId)
		if err != nil {
			err = fmt.Errorf("get space token error: %v", err)
			break
		}
		if util.IsTimestampNearingExpiration(spaceToken.ExpiresAt) {
			err = fmt.Errorf("space access token expired %d(%s)", spaceToken.ExpiresAt, util.ParseUnixMilliToDate(spaceToken.ExpiresAt))
			break
		}
		o := opt.(*backupssdkoptions.SpaceBackupOption)
		options.Space = &backupssdkoptions.SpaceSnapshotsOption{
			RepoName:       o.RepoName,
			OlaresDid:      spaceToken.OlaresDid,
			AccessToken:    spaceToken.AccessToken,
			ClusterId:      location["clusterId"],
			CloudName:      location["cloudName"],
			RegionId:       location["regionId"],
			CloudApiMirror: constant.DefaultSyncServerURL,
		}
	case *backupssdkoptions.AwsBackupOption:
		token, err := s.getIntegrationCloud()
		if err != nil {
			err = fmt.Errorf("get %s token error: %v", token.Type, err)
			break
		}
		o := opt.(*backupssdkoptions.AwsBackupOption)
		options.Aws = &backupssdkoptions.AwsSnapshotsOption{
			RepoName:        o.RepoName,
			Endpoint:        o.Endpoint,
			AccessKey:       token.AccessKey,
			SecretAccessKey: token.SecretKey,
		}
	case *backupssdkoptions.TencentCloudBackupOption:
		token, err := s.getIntegrationCloud()
		if err != nil {
			err = fmt.Errorf("get %s token error: %v", token.Type, err)
			break
		}
		o := opt.(*backupssdkoptions.TencentCloudBackupOption)
		options.TencentCloud = &backupssdkoptions.TencentCloudSnapshotsOption{
			RepoName:        o.RepoName,
			Endpoint:        o.Endpoint,
			AccessKey:       token.AccessKey,
			SecretAccessKey: token.SecretKey,
		}
	case *backupssdkoptions.FilesystemBackupOption:
		o := opt.(*backupssdkoptions.FilesystemBackupOption)
		options.Filesystem = &backupssdkoptions.FilesystemSnapshotsOption{
			RepoName: o.RepoName,
			Endpoint: o.Endpoint,
		}
	}

	statsService := backupssdk.NewStatsService(options)
	return statsService.Stats()
}

// TODO review
func (s *StorageBackup) updateBackupResult(backupOutput *backupssdkrestic.SummaryOutput,
	backupStorageObj *backupssdkmodel.StorageInfo, backupTotalSize uint64, backupError error) error {

	backup, err := s.Handlers.GetBackupHandler().GetById(s.Ctx, s.Backup.Name)
	if err != nil {
		return err
	}

	snapshot, err := s.Handlers.GetSnapshotHandler().GetById(s.Ctx, s.Snapshot.Name)
	if err != nil {
		return err
	}

	var phase constant.Phase = constant.Completed

	if backupError != nil {
		if strings.Contains(backupError.Error(), strings.ToLower(constant.Canceled.String())) {
			phase = constant.Canceled
		} else {
			phase = constant.Failed
		}
		snapshot.Spec.Phase = pointer.String(phase.String())
		snapshot.Spec.Message = pointer.String(backupError.Error())
		snapshot.Spec.ResticPhase = pointer.String(phase.String())
		if s.SnapshotType != nil {
			snapshot.Spec.SnapshotType = s.SnapshotType
		}
	} else {
		snapshot.Spec.SnapshotType = s.SnapshotType
		snapshot.Spec.SnapshotId = pointer.String(backupOutput.SnapshotID)
		snapshot.Spec.Size = pointer.UInt64Ptr(backupOutput.TotalBytesProcessed)
		snapshot.Spec.Progress = progressDone
		snapshot.Spec.Phase = pointer.String(phase.String())
		snapshot.Spec.Message = pointer.String(phase.String())
		snapshot.Spec.ResticPhase = pointer.String(phase.String())
		snapshot.Spec.ResticMessage = pointer.String(util.ToJSON(backupOutput))
	}

	snapshot.Spec.EndAt = pointer.Time()

	if backupStorageObj != nil {
		var extra = snapshot.Spec.Extra
		if extra == nil {
			extra = make(map[string]string)
		}
		extra["storage"] = util.ToJSON(backupStorageObj)
		snapshot.Spec.Extra = extra
	}

	if backupOutput != nil {
		if err := s.Handlers.GetBackupHandler().UpdateTotalSize(s.Ctx, backup, backupTotalSize); err != nil {
			log.Errorf("Backup %s,%s, update backup total size error: %v", backup.Spec.Name, s.Snapshot.Name, err)
		}
	}

	return s.Handlers.GetSnapshotHandler().UpdateBackupResult(s.Ctx, snapshot)
}

func (s *StorageBackup) notifyBackupResult(backupOutput *backupssdkrestic.SummaryOutput,
	backupStorageObj *backupssdkmodel.StorageInfo, backupError error) error {
	spaceToken, err := integration.IntegrationManager().GetIntegrationSpaceToken(s.Ctx, s.Backup.Spec.Owner, s.Params.Location["name"])
	if err != nil {
		return fmt.Errorf("get space token error: %v", err)
	}

	var status constant.Phase = constant.Completed

	var snapshotRecord = &notify.Snapshot{
		UserId:       spaceToken.OlaresDid,
		BackupId:     s.Backup.Name,
		SnapshotId:   s.Snapshot.Name,
		Unit:         constant.DefaultSnapshotSizeUnit,
		SnapshotTime: s.Snapshot.Spec.StartAt.UnixMilli(),
		Type:         handlers.ParseSnapshotTypeText(s.SnapshotType),
	}

	if backupStorageObj != nil {
		snapshotRecord.Url = backupStorageObj.Url
		snapshotRecord.CloudName = backupStorageObj.CloudName
		snapshotRecord.RegionId = backupStorageObj.RegionId
		snapshotRecord.Bucket = backupStorageObj.Bucket
		snapshotRecord.Prefix = backupStorageObj.Prefix
	}

	if backupError != nil {
		if strings.Contains(backupError.Error(), strings.ToLower(constant.Canceled.String())) {
			status = constant.Canceled
		} else {
			status = constant.Failed
		}
		snapshotRecord.Status = status.String()
		snapshotRecord.Message = backupError.Error()
	} else {
		snapshotRecord.ResticSnapshotId = backupOutput.SnapshotID
		snapshotRecord.Size = backupOutput.TotalBytesProcessed
		snapshotRecord.Status = status.String()
		snapshotRecord.Message = util.ToJSON(backupOutput)
	}

	log.Infof("notify backup result: %s", util.ToJSON(snapshotRecord))
	if err := notify.NotifySnapshot(s.Ctx, constant.DefaultSyncServerURL, snapshotRecord); err != nil { // finished
		return err
	}

	return nil
}

func (s *StorageBackup) getIntegrationCloud() (*integration.IntegrationToken, error) {
	var l = s.Params.Location
	var location = l["location"]
	var locationIntegrationName = l["name"]
	return integration.IntegrationManager().GetIntegrationCloudToken(s.Ctx, s.Backup.Spec.Owner, location, locationIntegrationName)
}
