package worker

import (
	"context"
	"strings"
	"sync"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/handlers"

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
	ctx      context.Context
	cancel   context.CancelFunc
	snapshot string
}

type activeRestore struct {
	ctx     context.Context
	cancel  context.CancelFunc
	restore string
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
		// log.Debug("[worker] no restore in queue")
		return
	}

	var ctxTask, cancelTask = context.WithCancel(ctx)
	w.activeRestore = &activeRestore{
		ctx:     ctxTask,
		cancel:  cancelTask,
		restore: restoreId,
	}
	log.Infof("[worker] run restore %s", restoreId)

	// ~ run backup
	if err := w.handlers.GetRestoreHandler().Restore(w.ctx, restoreId); err != nil {
		log.Errorf("[worker] restore %s error: %v", restoreId, err)
	}

	w.clearActiveRestore()
}

func (w *WorkerManage) RunBackup(ctx context.Context) {
	if w.activeBackup != nil {
		log.Infof("[worker] active snapshot %s is running", w.activeBackup.snapshot)
		return
	}

	_, backupId, snapshotId, ok := w.getBackupQueue()
	if !ok {
		// log.Debug("[worker] no snapshot in queue")
		return
	}

	var ctxTask, cancelTask = context.WithCancel(ctx)
	w.activeBackup = &activeBackup{
		ctx:      ctxTask,
		cancel:   cancelTask,
		snapshot: snapshotId,
	}
	log.Infof("[worker] run backup %s snapshot %s", backupId, snapshotId)

	// ~ run backup
	err := w.handlers.GetSnapshotHandler().Backup(w.ctx, backupId, snapshotId)
	if err != nil {
		log.Errorf("[worker] snapshot %s error: %v", snapshotId, err)
	}

	w.clearActiveBackup()
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
	if w.activeBackup.snapshot != snapshotId {
		log.Warnf("active snapshot %s is not equal %s", w.activeBackup.snapshot, snapshotId)
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
