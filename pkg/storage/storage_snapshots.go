package storage

import (
	"context"

	"olares.com/backup-server/pkg/constant"
	"olares.com/backup-server/pkg/handlers"
	"olares.com/backup-server/pkg/integration"
	"olares.com/backup-server/pkg/util/log"

	backupssdk "olares.com/backups-sdk"
	backupssdkoptions "olares.com/backups-sdk/pkg/options"
	backupssdkrestic "olares.com/backups-sdk/pkg/restic"
	backupssdkstorage "olares.com/backups-sdk/pkg/storage"
)

type StorageSnapshots struct {
	Handlers handlers.Interface
}

func (s *StorageSnapshots) GetSnapshots(ctx context.Context, password, owner, location, endpoint, backupName, backupId string) (*backupssdkrestic.SnapshotList, error) {
	var snapshotService *backupssdkstorage.SnapshotsService
	var logger = log.GetLogger()

	switch location {
	case constant.BackupLocationAwsS3.String():
		token, err := s.getIntegrationCloud(ctx, owner, location, endpoint)
		if err != nil {
			return nil, err
		}

		snapshotService = backupssdk.NewSnapshotsService(&backupssdkstorage.SnapshotsOption{
			Password: password,
			Logger:   logger,
			Operator: constant.StorageOperatorApp,
			Aws: &backupssdkoptions.AwsSnapshotsOption{
				RepoId:          backupId,
				RepoName:        backupName,
				Endpoint:        endpoint,
				AccessKey:       token.AccessKey,
				SecretAccessKey: token.SecretKey,
			},
		})
	case constant.BackupLocationTencentCloud.String():
		token, err := s.getIntegrationCloud(ctx, owner, location, endpoint)
		if err != nil {
			return nil, err
		}
		snapshotService = backupssdk.NewSnapshotsService(&backupssdkstorage.SnapshotsOption{
			Password: password,
			Logger:   log.GetLogger(),
			Operator: constant.StorageOperatorApp,
			TencentCloud: &backupssdkoptions.TencentCloudSnapshotsOption{
				RepoId:          backupId,
				RepoName:        backupName,
				Endpoint:        endpoint,
				AccessKey:       token.AccessKey,
				SecretAccessKey: token.SecretKey,
			},
		})
	case constant.BackupLocationFileSystem.String():
		snapshotService = backupssdk.NewSnapshotsService(&backupssdkstorage.SnapshotsOption{
			Password: password,
			Logger:   logger,
			Operator: constant.StorageOperatorApp,
			Filesystem: &backupssdkoptions.FilesystemSnapshotsOption{
				RepoId:   backupId,
				RepoName: backupName,
				Endpoint: endpoint,
			},
		})
	}

	return snapshotService.Snapshots()
}

func (s *StorageSnapshots) getIntegrationCloud(ctx context.Context, owner, location, endpoint string) (*integration.IntegrationToken, error) {
	return integration.IntegrationManager().GetIntegrationCloudAccount(ctx, owner, location, endpoint)
}
