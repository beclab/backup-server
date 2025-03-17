package storage

import (
	"bytetrade.io/web3os/backup-server/pkg/options"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	backupssdk "bytetrade.io/web3os/backups-sdk"
	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
)

func (s *storage) Backup(opt options.Option) error {
	backupssdk.NewBackupCommands()

	var backupService *backupssdkstorage.BackupService
	switch opt.(type) {
	case *options.SpaceBackupOptions:
		o := opt.(*options.SpaceBackupOptions)
		backupService = backupssdkstorage.NewBackupService(&backupssdkstorage.BackupOption{
			Password: o.Password,
			Logger:   log.GetLogger(),
			Space: &backupssdkoptions.SpaceBackupOption{
				RepoName:       o.RepoName,
				Path:           o.Path,
				OlaresDid:      o.OlaresDid,
				AccessToken:    o.AccessToken,
				ClusterId:      o.ClusterId,
				CloudName:      o.CloudName,
				RegionId:       o.RegionId,
				CloudApiMirror: o.CloudApiMirror,
			},
		})
	case options.S3BackupOptions:
		// todo
	}

	backupService.Backup()

	return nil
}
