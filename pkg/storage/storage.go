package storage

import (
	"context"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
)

type StorageInterface interface {
	Backup(ctx context.Context, opt options.Option) (backupOutput *backupssdkrestic.SummaryOutput, backupRepo string, backupError error)
	Restore(restore *sysv1.Restore)

	GetRegions(ctx context.Context, olaresId string) (string, error)
}

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
