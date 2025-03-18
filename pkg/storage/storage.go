package storage

import (
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/options"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
)

type StorageInterface interface {
	Backup(opt options.Option) (backupOutput *backupssdkrestic.SummaryOutput, backupRepo string, err error)
	Restore()
	GetUserToken() (olaresDid, olaresAccessToken string, expired int64, err error)
}

type storage struct {
	factory  client.Factory
	owner    string
	olaresId string
}

func NewStorage(f client.Factory, owner string, olaresId string) StorageInterface {
	return &storage{
		factory:  f,
		owner:    owner,
		olaresId: olaresId,
	}
}
