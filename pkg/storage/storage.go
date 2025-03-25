package storage

import (
	"context"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkmodel "bytetrade.io/web3os/backups-sdk/pkg/storage/model"
)

type StorageInterface interface {
	Backup(ctx context.Context, backup *sysv1.Backup, snapshot *sysv1.Snapshot, opt options.Option) (backupOutput *backupssdkrestic.SummaryOutput, storageInfo *backupssdkmodel.StorageInfo, backupError error)
	Restore(ctx context.Context, backup *sysv1.Backup, snapshot *sysv1.Snapshot, restore *sysv1.Restore, opt options.Option) (restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error)

	GetRegions(ctx context.Context, olaresId string) (string, error)
}

type StorageBackupHandler interface {
	Init()
	Prepare()
	Backup()
	Verify(ctx context.Context) error
	GetResult()
}

type StorageBackup struct {
	Factory  client.Factory
	Owner    string
	Backup   *sysv1.Backup
	Snapshot *sysv1.Snapshot
}

// ---

type storage struct {
	factory client.Factory
	owner   string
}

func NewStorage(f client.Factory, owner string) StorageInterface {
	return &storage{
		factory: f,
		owner:   owner,
	}
}
