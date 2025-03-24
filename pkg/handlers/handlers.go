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
	GetNotifyHandler() *NotifyHandler
}

type handlers struct {
	BackupHandler   *BackupHandler
	SnapshotHandler *SnapshotHandler
	RestoreHandler  *RestoreHandler
	NotifyHandler   *NotifyHandler
}

func NewHandler(factory client.Factory) Interface {
	var handlers = &handlers{}

	handlers.BackupHandler = NewBackupHandler(factory, handlers)
	handlers.SnapshotHandler = NewSnapshotHandler(factory, handlers)
	handlers.RestoreHandler = NewRestoreHandler(factory, handlers)
	handlers.NotifyHandler = NewNotifyHandler(factory, handlers)

	return handlers
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

func (h *handlers) GetNotifyHandler() *NotifyHandler {
	return h.NotifyHandler
}
