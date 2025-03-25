package storage

import (
	"context"
	"fmt"
	"strings"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	integration "bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/options"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	backupssdk "bytetrade.io/web3os/backups-sdk"
	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
	backupssdkmodel "bytetrade.io/web3os/backups-sdk/pkg/storage/model"
	"github.com/pkg/errors"
)

func (s *storage) Backup(ctx context.Context, backup *sysv1.Backup, snapshot *sysv1.Snapshot, opt options.Option) (backupOutput *backupssdkrestic.SummaryOutput, storageInfo *backupssdkmodel.StorageInfo, backupError error) {
	var isSpaceBackup bool
	var logger = log.GetLogger()
	var backupName = backup.Spec.Name
	var snapshotId = snapshot.Name

	log.Infof("Backup %s-%s location %s prepare: %s", backupName, snapshotId, opt.GetLocation(), opt.String())

	var tokenService = &integration.Integration{
		Factory:  s.factory,
		Owner:    s.owner,
		Location: opt.GetLocation(),
		Name:     opt.GetLocationConfigName(),
	}
	token, err := tokenService.GetIntegrationToken()
	if err != nil {
		backupError = fmt.Errorf("get token error %v", err)
		return
	}
	var backupService *backupssdkstorage.BackupService

	switch opt.(type) {
	case *options.SpaceBackupOptions:
		isSpaceBackup = true
		backupOutput, storageInfo, backupError = s.backupToSpace(ctx, backup, snapshot, opt, token, tokenService)
	case *options.AwsS3BackupOptions:
		o := opt.(*options.AwsS3BackupOptions)
		t := token.(*integration.IntegrationCloud)
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   logger,
			Aws: &backupssdkoptions.AwsBackupOption{
				RepoName:        backup.Name,
				Path:            o.Path,
				Endpoint:        t.Endpoint,
				AccessKey:       t.AccessKey,
				SecretAccessKey: t.SecretKey,
			},
		})
	case *options.TencentCloudBackupOptions:
		o := opt.(*options.TencentCloudBackupOptions)
		t := token.(*integration.IntegrationCloud)
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   logger,
			TencentCloud: &backupssdkoptions.TencentCloudBackupOption{
				RepoName:        backup.Name,
				Path:            o.Path,
				Endpoint:        t.Endpoint,
				AccessKey:       t.AccessKey,
				SecretAccessKey: t.SecretKey,
			},
		})
	case *options.FilesystemBackupOptions:
		o := opt.(*options.FilesystemBackupOptions)
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   logger,
			Filesystem: &backupssdkoptions.FilesystemBackupOption{
				RepoName: backup.Name,
				Endpoint: o.Endpoint,
				Path:     o.Path,
			},
		})
	}

	if !isSpaceBackup {
		backupOutput, storageInfo, backupError = backupService.Backup()
		if backupError != nil {
			log.Errorf("Backup %s-%s cloud error: %v", backupName, snapshotId, backupError)
		} else {
			log.Infof("Backup %s-%s cloud success", backupName, snapshotId)
		}
	}

	return
}

func (s *storage) backupToSpace(ctx context.Context, backup *sysv1.Backup,
	snapshot *sysv1.Snapshot,
	opt options.Option, token integration.IntegrationToken, tokenService integration.IntegrationInterface) (output *backupssdkrestic.SummaryOutput, storageInfo *backupssdkmodel.StorageInfo, err error) {
	_ = ctx
	var o = opt.(*options.SpaceBackupOptions)
	var backupName = backup.Spec.Name
	var snapshotId = snapshot.Name

	for {
		var spaceToken = token.(*integration.IntegrationSpace)
		if util.IsTimestampNearingExpiration(spaceToken.ExpiresAt) {
			err = fmt.Errorf("Backup %s-%s space access token expired %d(%s)",
				backupName, snapshotId, spaceToken.ExpiresAt, util.ParseUnixMilliToDate(spaceToken.ExpiresAt))
			break
		}

		var spaceBackupOption = &backupssdkoptions.SpaceBackupOption{
			RepoName:       backup.Name,
			Path:           o.Path,
			OlaresDid:      spaceToken.OlaresDid,
			AccessToken:    spaceToken.AccessToken,
			ClusterId:      o.ClusterId,
			CloudName:      o.CloudName,
			RegionId:       o.RegionId,
			CloudApiMirror: o.CloudApiMirror,
		}

		var backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   log.GetLogger(),
			Space:    spaceBackupOption,
		})

		output, storageInfo, err = backupService.Backup() // todo ctx

		if err != nil {
			if strings.Contains(err.Error(), "refresh-token error") {
				token, err = tokenService.GetIntegrationToken()
				if err != nil {
					err = fmt.Errorf("Backup %s-%s space get integration token error: %v", backupName, snapshotId, err)
					break
				}
				continue
			} else {
				err = errors.WithStack(fmt.Errorf("Backup %s-%s space error: %v", backupName, snapshotId, err))
				break
			}
		}
		log.Infof("Backup %s-%s space success", backupName, snapshotId)
		break
	}

	return
}
