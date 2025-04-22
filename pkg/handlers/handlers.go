package handlers

import (
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/interfaces"
	"bytetrade.io/web3os/backup-server/pkg/watchers"
)

var _ Interface = &handlers{}

type Interface interface {
	interfaces.HandlerInterface
	GetBackupHandler() *BackupHandler
	GetSnapshotHandler() *SnapshotHandler
	GetRestoreHandler() *RestoreHandler
	GetNotification() *watchers.Notification
}

type handlers struct {
	BackupHandler   *BackupHandler
	SnapshotHandler *SnapshotHandler
	RestoreHandler  *RestoreHandler
	Notification    *watchers.Notification
}

func NewHandler(factory client.Factory, n *watchers.Notification) Interface {
	var handlers = &handlers{
		Notification: n,
	}

	handlers.BackupHandler = NewBackupHandler(factory, handlers)
	handlers.SnapshotHandler = NewSnapshotHandler(factory, handlers)
	handlers.RestoreHandler = NewRestoreHandler(factory, handlers)

	return handlers
}

func (h *handlers) GetNotification() *watchers.Notification {
	return h.Notification
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
