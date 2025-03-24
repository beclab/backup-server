package storage

import (
	"context"

	"bytetrade.io/web3os/backup-server/pkg/client"
	integration "bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
)

type StorageInterface interface {
	Backup(ctx context.Context, opt options.Option, token integration.IntegrationToken, tokenService *integration.Integration) (backupOutput *backupssdkrestic.SummaryOutput, backupRepo string, backupError error)
	Restore(ctx context.Context, opt options.Option) (restoreOutput *backupssdkrestic.RestoreSummaryOutput, restoreError error)

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
