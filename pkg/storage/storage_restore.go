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
	"github.com/pkg/errors"

	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
)

func (s *storage) Restore(ctx context.Context,
	backup *sysv1.Backup, snapshot *sysv1.Snapshot, restore *sysv1.Restore, opt options.Option) (restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error) {
	var isSpaceRestore bool
	var logger = log.GetLogger()
	var backupName = backup.Spec.Name
	var snapshotId = restore.Spec.SnapshotId
	var resticSnapshotId = *snapshot.Spec.SnapshotId

	log.Infof("Restore %s-%s location %s prepare: %s", backupName, snapshotId, opt.GetLocation(), opt.String())

	var tokenService = &integration.Integration{
		Factory:  s.factory,
		Owner:    s.owner,
		Location: opt.GetLocation(),
		Name:     opt.GetLocationConfigName(),
	}

	var token, err = tokenService.GetIntegrationToken()
	if err != nil {
		restoreError = errors.WithStack(fmt.Errorf("get token error %v", opt.GetLocation(), err))
		return
	}

	var restoreService *backupssdkstorage.RestoreService

	switch opt.(type) {
	case *options.SpaceRestoreOptions:
		isSpaceRestore = true
		restoreOutput, restoreError = s.restoreFromSpace(ctx, backup, snapshot, restore, opt, token, tokenService)
	case *options.AwsS3RestoreOptions:
		o := opt.(*options.AwsS3RestoreOptions)
		t := token.(*integration.IntegrationCloud)
		restoreService = backupssdk.NewRestoreService(&backupssdkstorage.RestoreOption{
			Password: o.Password,
			Logger:   logger,
			Aws: &backupssdkoptions.AwsRestoreOption{
				RepoName:          backup.Name,
				SnapshotId:        resticSnapshotId,
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
				RepoName:        backup.Name,
				SnapshotId:      resticSnapshotId,
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
				RepoName:   backup.Name,
				SnapshotId: resticSnapshotId,
				Endpoint:   o.Endpoint,
				Path:       o.Path,
			},
		})
	}

	if !isSpaceRestore {
		restoreOutput, restoreError = restoreService.Restore()
		if restoreError != nil {
			log.Errorf("Restore %s-%s cloud error: %v", backupName, snapshotId, restoreError)
		} else {
			log.Infof("Restore %s-%s cloud success", backupName, snapshotId)
		}
	}

	if restoreError != nil {
		return
	}

	return
}

func (s *storage) restoreFromSpace(ctx context.Context, backup *sysv1.Backup, snapshot *sysv1.Snapshot, restore *sysv1.Restore, opt options.Option, token integration.IntegrationToken, tokenService integration.IntegrationInterface) (restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error) {
	var o = opt.(*options.SpaceRestoreOptions)
	var backupName = backup.Spec.Name
	var snapshotId = snapshot.Name

	for {
		var spaceToken = token.(*integration.IntegrationSpace)
		if util.IsTimestampNearingExpiration(spaceToken.ExpiresAt) {
			restoreError = fmt.Errorf("Restore %s-%s space access token expired %d(%s)", backupName, snapshotId, spaceToken.ExpiresAt, util.ParseUnixMilliToDate(spaceToken.ExpiresAt))
			break
		}

		var spaceRestoreOption = &backupssdkoptions.SpaceRestoreOption{
			RepoName:          backup.Name,
			SnapshotId:        *snapshot.Spec.SnapshotId,
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
					restoreError = fmt.Errorf("Restore %s-%s space get integration token error: %v", backupName, snapshotId, restoreError)
					break
				}
				continue
			} else {
				restoreError = errors.WithStack(fmt.Errorf("Restore %s-%s space error: %v", backupName, snapshotId, restoreError))
				break
			}
		}

		log.Infof("Restore %s-%s space success", backupName, snapshotId)

		break
	}

	return
}
