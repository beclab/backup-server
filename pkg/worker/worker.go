package worker

import (
	"context"
	"errors"
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
	restoreQueue []string // restoreId

	activeBackup  *activeBackup
	activeRestore *activeRestore

	sync.Mutex
}

type activeBackup struct {
	ctx        context.Context
	cancel     context.CancelFunc
	backupId   string
	snapshotId string
	backup     *storage.StorageBackup
}

type activeRestore struct {
	ctx       context.Context
	cancel    context.CancelFunc
	restoreId string
	restore   *storage.StorageRestore
	progress  float64
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
	var restoreId string
	var ok bool
	var setupErr error

	func() {
		w.Lock()
		defer w.Unlock()

		if w.activeRestore != nil {
			log.Infof("[worker] active restore %s is running, skip", w.activeRestore.restoreId)
			setupErr = fmt.Errorf("[worker] active restore %s is running, skip", w.activeRestore.restoreId)
			return
		}

		restoreId, ok = w.getRestoreQueue()
		if !ok {
			setupErr = fmt.Errorf("[worker] restoreQueue is empty")
			return
		}

		var ctxTask, cancelTask = context.WithCancel(ctx)

		var storageRestore = &storage.StorageRestore{
			Ctx:       ctxTask,
			Handlers:  w.handlers,
			RestoreId: restoreId,
		}

		w.activeRestore = &activeRestore{
			ctx:       ctx,
			cancel:    cancelTask,
			restoreId: restoreId,
			restore:   storageRestore,
			progress:  0.0,
		}
	}()

	if setupErr != nil {
		return
	}

	log.Infof("[worker] run restore %s", restoreId)

	if err := w.activeRestore.restore.RunRestore(w.callbackRestoreProgress); err != nil {
		log.Errorf("[worker] restore %s error: %v", restoreId, err)
	}

	w.clearActiveRestore(restoreId)
}

func (w *WorkerManage) callbackRestoreProgress(percentDone float64) {
	if w.activeRestore == nil {
		return
	}

	w.activeRestore.progress = percentDone
}

func (w *WorkerManage) RunBackup(ctx context.Context) {
	var backupId string
	var snapshotId string
	var ok bool
	var setupErr error

	func() {
		w.Lock()
		defer w.Unlock()

		if w.activeBackup != nil {
			log.Infof("[worker] active snapshot %s is running, skip", w.activeBackup.snapshotId)
			setupErr = fmt.Errorf("[worker] active snapshot %s is running, skip", w.activeBackup.snapshotId)
			return
		}

		_, backupId, snapshotId, ok = w.getBackupQueue()
		if !ok {
			setupErr = fmt.Errorf("[worker] backupQueue is empty")
			return
		}

		var ctxTask, cancelTask = context.WithCancel(ctx)
		var storageBackup = &storage.StorageBackup{
			Ctx:        ctxTask,
			Handlers:   w.handlers,
			SnapshotId: snapshotId,
		}

		w.activeBackup = &activeBackup{
			ctx:        ctx,
			cancel:     cancelTask,
			backupId:   backupId,
			snapshotId: snapshotId,
			backup:     storageBackup,
		}
	}()

	if setupErr != nil {
		return
	}

	log.Infof("[worker] run backup: %s, snapshot: %s", backupId, snapshotId)

	if err := w.activeBackup.backup.RunBackup(); err != nil {
		log.Errorf("[worker] backup: %s, error: %v", snapshotId, err)
	}

	w.clearActiveBackup(backupId, snapshotId)
}

func (w *WorkerManage) CancelBackup(backupId string) error {
	w.Lock()
	defer w.Unlock()

	log.Infof("[worker] cancel backup: %s", backupId)

	w.removeBackupSnapshotsFromBackupQueue(backupId)

	if w.activeBackup != nil && w.activeBackup.backupId == backupId {
		w.activeBackup.cancel()

		w.activeBackup = nil
	}

	return nil
}

func (w *WorkerManage) CancelSnapshot(snapshotId string) error {
	w.Lock()
	defer w.Unlock()

	log.Infof("[worker] cancel snapshot: %s", snapshotId)

	w.removeSnapshotIdFromBackupQueue(snapshotId)

	if w.activeBackup != nil && w.activeBackup.snapshotId == snapshotId {
		w.activeBackup.cancel()

		w.activeBackup = nil
	}

	return nil
}

func (w *WorkerManage) CancelRestore(restoreId string) error {
	w.Lock()
	defer w.Unlock()

	log.Infof("[worker] cancel restore: %s", restoreId)

	w.removeRestoreIdFromRestoreQueue(restoreId)

	if w.activeRestore != nil && w.activeRestore.restoreId == restoreId {
		w.activeRestore.cancel()

		w.activeRestore = nil
	}

	return nil
}

func (w *WorkerManage) AppendBackupTask(id string) {
	w.Lock()
	defer w.Unlock()

	w.backupQueue = append(w.backupQueue, id)
}

func (w *WorkerManage) AppendRestoreTask(restoreId string) {
	w.Lock()
	defer w.Unlock()

	w.restoreQueue = append(w.restoreQueue, restoreId)
}

func (w *WorkerManage) GetRestoreProgress(restoreId string) (float64, error) {
	if w.activeRestore == nil {
		return 0.0, errors.New("[worker] no active restore")
	}
	if w.activeRestore.restoreId != restoreId {
		return 0.0, fmt.Errorf("[worker] restore id not match, runId: %s", w.activeRestore.restoreId)
	}

	return w.activeRestore.progress, nil
}

func (w *WorkerManage) clearActiveBackup(backupId, snapshotId string) {
	w.Lock()
	defer w.Unlock()
	log.Infof("[worker] clear active backup: %s, snapshot: %s", backupId, snapshotId)
	w.activeBackup = nil
}

func (w *WorkerManage) clearActiveRestore(restoreId string) {
	w.Lock()
	defer w.Unlock()
	log.Infof("[worker] clear active restore: %s", restoreId)
	w.activeRestore = nil
}

func (w *WorkerManage) getBackupQueue() (owner string, backupId string, snapshotId string, flag bool) {
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
	if len(w.restoreQueue) == 0 {
		return "", false
	}

	first := w.restoreQueue[0]
	w.restoreQueue = w.restoreQueue[1:]

	return first, true
}

func (w *WorkerManage) removeBackupSnapshotsFromBackupQueue(backupId string) bool {
	if len(w.backupQueue) == 0 {
		return false
	}

	var slice []string
	for _, v := range w.backupQueue {
		vs := strings.Split(v, "_")
		if len(vs) != 3 {
			continue
		}

		if vs[1] != backupId {
			slice = append(slice, v)
		}
	}

	w.backupQueue = slice
	return true
}

func (w *WorkerManage) removeSnapshotIdFromBackupQueue(snapshotId string) bool {
	if len(w.backupQueue) == 0 {
		return false
	}

	var found bool
	var slice []string
	for i, v := range w.backupQueue {
		vs := strings.Split(v, "_")
		if len(vs) != 3 {
			continue
		}
		if vs[2] == snapshotId {
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
