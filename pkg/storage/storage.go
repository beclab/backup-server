package storage

import (
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/options"
)

type StorageInterface interface {
	Backup(opt options.Option) error
}

type storage struct {
	factory client.Factory
}

func NewStorage(f client.Factory) StorageInterface {
	return &storage{
		factory: f,
	}
}
