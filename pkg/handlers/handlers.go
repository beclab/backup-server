package handlers

import (
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/interfaces"
)

var _ Interface = &handlers{}

type Interface interface {
	interfaces.HandlerInterface
	GetBackupHandler() *BackupHandler
	GetSnapshotHandler() *SnapshotHandler
	GetRestoreHandler() *RestoreHandler
}

type handlers struct {
	BackupHandler   *BackupHandler
	SnapshotHandler *SnapshotHandler
	RestoreHandler  *RestoreHandler
}

func NewHandler(factory client.Factory) Interface {
	return &handlers{
		BackupHandler:   NewBackupHandler(factory),
		SnapshotHandler: NewSnapshotHandler(factory),
		RestoreHandler:  NewRestoreHandler(factory),
	}
}

func (h *handlers) GetBackupHandler() *BackupHandler {
	return h.BackupHandler
}

func (h *handlers) GetSnapshotHandler() *SnapshotHandler {
	return h.SnapshotHandler
}

func (h *handlers) GetRestoreHandler() *RestoreHandler {
	return h.RestoreHandler
}
