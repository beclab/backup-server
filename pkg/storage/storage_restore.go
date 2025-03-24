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
	"github.com/pkg/errors"

	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
)

func (s *storage) Restore(ctx context.Context, opt options.Option) (restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error) {
	var isSpaceRestore bool
	var logger = log.GetLogger()

	var tokenService = &integration.Integration{
		Factory:  s.factory,
		Owner:    s.owner,
		Location: opt.GetLocation(),
		Name:     opt.GetLocationConfigName(),
	}

	var token, err = tokenService.GetIntegrationToken()
	if err != nil {
		restoreError = errors.WithStack(fmt.Errorf("get location %s user %s token error %v", opt.GetLocation(), opt.GetLocationConfigName(), err))
		return
	}

	var restoreService *backupssdkstorage.RestoreService

	switch opt.(type) {
	case *options.SpaceRestoreOptions:
		isSpaceRestore = true
		restoreOutput, restoreError = s.restoreFromSpace(ctx, opt, token, tokenService)
	case *options.AwsS3RestoreOptions:
		o := opt.(*options.AwsS3RestoreOptions)
		t := token.(*integration.IntegrationCloud)
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: o.Password,
			Logger:   logger,
			Aws: &backupssdkoptions.AwsRestoreOption{
				RepoName:          o.RepoName,
				Path:              o.Path,
				Endpoint:          t.Endpoint,
				AccessKey:         t.AccessKey,
				SecretAccessKey:   t.SecretKey,
				LimitDownloadRate: o.LimitDownloadRate,
			},
		})
	case *options.TencentCloudRestoreOptions:
		o := opt.(*options.TencentCloudRestoreOptions)
		t := token.(*integration.IntegrationCloud)
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: o.Password,
			Logger:   logger,
			TencentCloud: &backupssdkoptions.TencentCloudRestoreOption{
				RepoName:        o.RepoName,
				Path:            o.Path,
				Endpoint:        t.Endpoint,
				AccessKey:       t.AccessKey,
				SecretAccessKey: t.SecretKey,
			},
		})
	case *options.FilesystemRestoreOptions:
		o := opt.(*options.FilesystemRestoreOptions)
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: o.Password,
			Logger:   logger,
			Filesystem: &backupssdkoptions.FilesystemRestoreOption{
				RepoName: o.RepoName,
				Endpoint: o.Endpoint,
				Path:     o.Path,
			},
		})
	}

	if !isSpaceRestore {
		restoreOutput, restoreError = restoreService.Restore()
	}

	if restoreError != nil {
		return
	}

	return
}

func (s *storage) restoreFromSpace(ctx context.Context, opt options.Option, token integration.IntegrationToken, tokenService integration.IntegrationInterface) (restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error) {
	var o = opt.(*options.SpaceRestoreOptions)

	log.Infof("backup-server restore from space backup %s snapshot %s", o.RepoName, o.SnapshotId)

	for {
		var spaceToken = token.(*integration.IntegrationSpace)
		if util.IsTimestampExpired(spaceToken.ExpiresAt) {
			restoreError = fmt.Errorf("olares space access token expired %d", spaceToken.ExpiresAt)
			break
		}

		var spaceRestoreOption = &backupssdkoptions.SpaceRestoreOption{
			RepoName:          o.RepoName,
			SnapshotId:        o.SnapshotId,
			Path:              o.Path,
			OlaresDid:         spaceToken.OlaresDid,
			AccessToken:       spaceToken.AccessToken,
			ClusterId:         o.ClusterId,
			CloudName:         o.CloudName,
			RegionId:          o.RegionId,
			LimitDownloadRate: o.LimitDownloadRate,
			CloudApiMirror:    o.CloudApiMirror,
		}

		var restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: o.Password,
			Logger:   log.GetLogger(),
			Space:    spaceRestoreOption,
		})

		restoreOutput, restoreError = restoreService.Restore()

		if restoreError != nil {
			if strings.Contains(restoreError.Error(), "refresh-token error") {
				token, restoreError = tokenService.GetIntegrationToken()
				if restoreError != nil {
					restoreError = fmt.Errorf("backup-server restore from space get integration token error: %v", restoreError)
					break
				}
				continue
			} else {
				restoreError = errors.WithStack(fmt.Errorf("backup-server restore from space error: %v", restoreError))
				break
			}
		}

		log.Infof("backup-server restore from space backup %s snapshot %s success", o.RepoName, o.SnapshotId)

		break
	}

	return
}
