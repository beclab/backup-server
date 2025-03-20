package storage

import (
	"fmt"
	"strings"

	"bytetrade.io/web3os/backup-server/pkg/options"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	backupssdk "bytetrade.io/web3os/backups-sdk"
	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
	"github.com/pkg/errors"
)

func (s *storage) Backup(opt options.Option) (backupOutput *backupssdkrestic.SummaryOutput, backupRepo string, backupError error) {
	var isSpaceBackup bool

	var backupService *backupssdkstorage.BackupService

	switch opt.(type) {
	case *options.SpaceBackupOptions:
		isSpaceBackup = true
		backupOutput, backupRepo, backupError = s.backupToSpace(opt)
	case *options.AwsBackupOptions:
		o := opt.(*options.AwsBackupOptions)
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   log.GetLogger(),
			Aws: &backupssdkoptions.AwsBackupOption{
				RepoName:        o.RepoName,
				Path:            o.Path,
				Endpoint:        o.Endpoint,
				AccessKey:       o.AccessKey,
				SecretAccessKey: o.SecretKey,
				LimitUploadRate: "200",
			},
		})
	case *options.TencentCloudBackupOptions:
		o := opt.(*options.TencentCloudBackupOptions)
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   log.GetLogger(),
			TencentCloud: &backupssdkoptions.TencentCloudBackupOption{
				RepoName:        o.RepoName,
				Path:            o.Path,
				Endpoint:        o.Endpoint,
				AccessKey:       o.AccessKey,
				SecretAccessKey: o.SecretKey,
			},
		})
	case *options.FilesystemBackupOptions:
		o := opt.(*options.FilesystemBackupOptions)
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   log.GetLogger(),
			Filesystem: &backupssdkoptions.FilesystemBackupOption{
				RepoName: o.RepoName,
				Endpoint: o.Endpoint,
				Path:     o.Path,
			},
		})
	}

	if !isSpaceBackup {
		backupOutput, backupRepo, backupError = backupService.Backup()
	}

	if backupError != nil {
		return
	}

	return
}

func (s *storage) backupToSpace(opt options.Option) (output *backupssdkrestic.SummaryOutput, repo string, err error) {
	var o = opt.(*options.SpaceBackupOptions)

	for {
		var olaresDid, olaresAccessToken string
		var expired int64
		olaresDid, olaresAccessToken, expired, err = s.GetUserToken()
		if err != nil {
			break
		}

		if util.IsTimestampExpired(expired) {
			err = errors.WithStack(fmt.Errorf("olares access token expired %d", olaresAccessToken))
			break
		}

		var spaceBackupOption = &backupssdkoptions.SpaceBackupOption{
			RepoName:       o.RepoName,
			Path:           o.Path,
			OlaresDid:      olaresDid,
			AccessToken:    olaresAccessToken,
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

		output, repo, err = backupService.Backup()

		if err != nil {
			if strings.Contains(err.Error(), "get sts token invalid") {
				continue
			} else {
				err = errors.WithStack(fmt.Errorf("backup-server backup to space error: %v", err))
				break
			}
		}
		log.Infof("backup-server backup to space %s success", o.RepoName)
		break
	}

	return
}
