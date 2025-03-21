package storage

import (
	"context"
	"fmt"
	"strings"

	integration "bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/options"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	backupssdk "bytetrade.io/web3os/backups-sdk"
	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
	"github.com/pkg/errors"
)

func (s *storage) Backup(ctx context.Context, opt options.Option) (backupOutput *backupssdkrestic.SummaryOutput, backupRepo string, backupError error) {
	var isSpaceBackup bool
	var logger = log.GetLogger()

	var tokenService = &integration.Integration{
		Factory:  s.factory,
		Owner:    s.owner,
		Location: opt.GetLocation(),
		Name:     opt.GetLocationConfigName(),
	}

	var token, err = tokenService.GetIntegrationToken()
	if err != nil {
		backupError = errors.WithStack(fmt.Errorf("get location %s user %s token error %v", opt.GetLocation(), opt.GetLocationConfigName(), err))
		return
	}

	var backupService *backupssdkstorage.BackupService

	switch opt.(type) {
	case *options.SpaceBackupOptions:
		isSpaceBackup = true
		backupOutput, backupRepo, backupError = s.backupToSpace(ctx, opt, token, tokenService)
	case *options.AwsS3BackupOptions:
		o := opt.(*options.AwsS3BackupOptions)
		t := token.(*integration.IntegrationCloud)
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   logger,
			Aws: &backupssdkoptions.AwsBackupOption{
				RepoName:        o.RepoName,
				Path:            o.Path,
				Endpoint:        t.Endpoint,
				AccessKey:       t.AccessKey,
				SecretAccessKey: t.SecretKey,
				LimitUploadRate: "200",
			},
		})
	case *options.TencentCloudBackupOptions:
		o := opt.(*options.TencentCloudBackupOptions)
		t := token.(*integration.IntegrationCloud)
		backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   logger,
			TencentCloud: &backupssdkoptions.TencentCloudBackupOption{
				RepoName:        o.RepoName,
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

func (s *storage) backupToSpace(ctx context.Context, opt options.Option, token integration.IntegrationToken, tokenService integration.IntegrationInterface) (output *backupssdkrestic.SummaryOutput, repo string, err error) {
	_ = ctx
	var o = opt.(*options.SpaceBackupOptions)

	for {
		var spaceToken = token.(*integration.IntegrationSpace)
		if util.IsTimestampExpired(spaceToken.ExpiresAt) {
			err = errors.WithStack(fmt.Errorf("olares space access token expired %d", spaceToken.ExpiresAt))
			break
		}

		var spaceBackupOption = &backupssdkoptions.SpaceBackupOption{
			RepoName:        o.RepoName,
			Path:            o.Path,
			OlaresDid:       spaceToken.OlaresDid,
			AccessToken:     spaceToken.AccessToken,
			ClusterId:       o.ClusterId,
			CloudName:       o.CloudName,
			RegionId:        o.RegionId,
			LimitUploadRate: "200",
			CloudApiMirror:  o.CloudApiMirror,
		}

		var backupService = backupssdk.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   log.GetLogger(),
			Space:    spaceBackupOption,
		})

		output, repo, err = backupService.Backup() // todo ctx

		if err != nil {
			if strings.Contains(err.Error(), "refresh-token error") {
				token, err = tokenService.GetIntegrationToken()
				if err != nil {
					break
				}
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
