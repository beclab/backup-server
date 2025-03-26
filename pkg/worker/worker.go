package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/storage"

	"bytetrade.io/web3os/backup-server/pkg/util/log"
)

var Worker *WorkerManage

type WorkerManage struct {
	ctx      context.Context
	handlers handlers.Interface

	backupQueue  []string // owner_backupid_snapshotid
	restoreQueue []string // owner_backupid_snapshotid

	activeBackup  *activeBackup
	activeRestore *activeRestore

	queueMutex sync.Mutex
}

type activeBackup struct {
	ctx        context.Context
	cancel     context.CancelFunc
	snapshotId string
	backup     *storage.StorageBackup
}

type activeRestore struct {
	ctx       context.Context
	cancel    context.CancelFunc
	restoreId string
	restore   *storage.StorageRestore
}

func NewWorkerManage(ctx context.Context, handlers handlers.Interface) *WorkerManage {
	Worker = &WorkerManage{
		handlers: handlers,
		ctx:      ctx,
	}
	return Worker
}

func (w *WorkerManage) StartBackupWorker() {
	log.Infof("[worker] run backup worker")
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				w.RunBackup(w.ctx)
			case <-w.ctx.Done():
				return
			}
		}
	}()
}

func (w *WorkerManage) StartRestoreWorker() {
	log.Infof("[worker] run restore worker")
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				w.RunRestore(w.ctx)
			case <-w.ctx.Done():
				return
			}
		}
	}()
}

func (w *WorkerManage) RunRestore(ctx context.Context) {
	if w.activeRestore != nil {
		log.Infof("[worker] active restore %s is running", w.activeRestore.restore)
		return
	}

	restoreId, ok := w.getRestoreQueue()
	if !ok {
		return
	}

	var ctxTask, cancelTask = context.WithCancel(ctx)
	w.activeRestore = &activeRestore{
		ctx:       ctxTask,
		cancel:    cancelTask,
		restoreId: restoreId,
	}
	log.Infof("[worker] run restore %s", restoreId)

	var storageRestore = &storage.StorageRestore{ // TODO ctx cancel
		Handlers:  w.handlers,
		RestoreId: restoreId,
	}
	if err := storageRestore.RunRestore(); err != nil {
		log.Errorf("[worker] restore %s error: %v", restoreId, err)
	}

	w.clearActiveRestore()
}

func (w *WorkerManage) RunBackup(ctx context.Context) {
	if w.activeBackup != nil {
		log.Infof("[worker] active snapshot %s is running", w.activeBackup.snapshotId)
		return
	}

	_, backupId, snapshotId, ok := w.getBackupQueue()
	if !ok {
		return
	}

	var ctxTask, cancelTask = context.WithCancel(ctx)

	var storageBackup = &storage.StorageBackup{ // TODO ctx cancel
		Ctx:        ctx,
		Handlers:   w.handlers,
		SnapshotId: snapshotId,
	}

	w.activeBackup = &activeBackup{
		ctx:        ctxTask,
		cancel:     cancelTask,
		snapshotId: snapshotId,
		backup:     storageBackup,
	}

	log.Infof("[worker] run backup %s snapshot %s", backupId, snapshotId)

	if err := w.activeBackup.backup.RunBackup(); err != nil {
		log.Errorf("[worker] backup %s error: %v", snapshotId, err)
	}

	w.clearActiveBackup()
}

func (w *WorkerManage) CancelBackup(snapshotId string) error {
	if w.activeBackup == nil {
		return fmt.Errorf("no snapshot is running")
	}

	if w.isBackupQueueEmpty() {
		return fmt.Errorf("backupQueue is empty")
	}

	if w.activeBackup.snapshotId != snapshotId {
		if ok := w.removeSnapshotIdFromBackupQueue(snapshotId); !ok {
			log.Infof("snapshot %s not in backupQueue", snapshotId)
		} else {
			log.Infof("snapshot %s removed from backupQueue", snapshotId)
		}
		return nil
	}

	w.activeBackup.cancel()
	// TODO update snapshot Phase
	// TODO Notify
	// TODO if Completed or Failed, skip

	w.clearActiveBackup()

	return nil
}

func (w *WorkerManage) CancelRestore(restoreId string) error {
	if w.activeRestore == nil {
		return fmt.Errorf("no restore is running")
	}

	if w.isRestoreQueueEmpty() {
		return fmt.Errorf("restoreQueue is empty")
	}

	if w.activeRestore.restoreId != restoreId {
		if ok := w.removeRestoreIdFromRestoreQueue(restoreId); !ok {
			log.Infof("restore %s not in restoreQueue", restoreId)
		} else {
			log.Infof("restore %s removed from restoreQueue", restoreId)
		}
		return nil
	}

	w.activeRestore.cancel()
	// TODO update snapshot Phase
	// TODO if Completed or Failed, skip

	w.clearActiveRestore()

	return nil
}

func (w *WorkerManage) AppendBackupTask(id string) {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	w.backupQueue = append(w.backupQueue, id)
}

func (w *WorkerManage) AppendRestoreTask(restoreId string) { // TODO backupid_snapshotid
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	w.restoreQueue = append(w.restoreQueue, restoreId)
}

func (w *WorkerManage) StopBackup(snapshotId string) {
	if w.activeBackup.snapshotId != snapshotId {
		log.Warnf("active snapshot %s is not equal %s", w.activeBackup.snapshotId, snapshotId)
		return
	}

	log.Infof("stop backup %s", snapshotId)
	w.activeBackup.cancel()
	w.activeBackup = nil
}

func (w *WorkerManage) clearActiveBackup() {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()
	w.activeBackup = nil
}

func (w *WorkerManage) clearActiveRestore() {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()
	w.activeRestore = nil
}

func (w *WorkerManage) getBackupQueue() (owner string, backupId string, snapshotId string, flag bool) {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	if len(w.backupQueue) == 0 {
		return
	}

	first := w.backupQueue[0]
	w.backupQueue = w.backupQueue[1:]

	ids := strings.Split(first, "_")
	if len(ids) != 3 {
		return
	}

	owner = ids[0]
	backupId = ids[1]
	snapshotId = ids[2]
	flag = true

	return
}

func (w *WorkerManage) getRestoreQueue() (string, bool) {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	if len(w.restoreQueue) == 0 {
		return "", false
	}

	first := w.restoreQueue[0]
	w.restoreQueue = w.restoreQueue[1:]

	return first, true
}

func (w *WorkerManage) isBackupQueueEmpty() bool {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()
	return len(w.backupQueue) == 0
}

func (w *WorkerManage) isRestoreQueueEmpty() bool {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()
	return len(w.restoreQueue) == 0
}

func (w *WorkerManage) removeSnapshotIdFromBackupQueue(snapshotId string) bool {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	if len(w.backupQueue) == 0 {
		return false
	}

	var found bool
	var slice []string
	for i, v := range w.backupQueue {
		if v == snapshotId {
			slice = w.backupQueue[:i]
			slice = append(slice, w.backupQueue[i+1:]...)
			found = true
			break
		}
	}

	if found {
		w.backupQueue = slice
		return true
	}

	return false
}

func (w *WorkerManage) removeRestoreIdFromRestoreQueue(restoreId string) bool {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	if len(w.restoreQueue) == 0 {
		return false
	}

	var found bool
	var slice []string
	for i, v := range w.restoreQueue {
		if v == restoreId {
			slice = w.restoreQueue[:i]
			slice = append(slice, w.restoreQueue[i+1:]...)
			found = true
			break
		}
	}

	if found {
		w.restoreQueue = slice
		return true
	}

	return false
}
