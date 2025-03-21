package worker

import (
	"context"
	"sync"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/handlers"

	"bytetrade.io/web3os/backup-server/pkg/util/log"
)

var Worker *WorkerManage

type WorkerManage struct {
	ctx      context.Context
	handlers handlers.Interface

	backupQueue  []string
	restoreQueue []string

	activeBackup  *activeBackup
	activeRestore *activeRestore

	queueMutex sync.Mutex
}

type activeBackup struct {
	ctx      context.Context
	cancel   context.CancelFunc
	snapshot *sysv1.Snapshot
}

type activeRestore struct {
	ctx      context.Context
	cancel   context.CancelFunc
	snapshot *sysv1.Snapshot
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
	// TODO
}

func (w *WorkerManage) RunBackup(ctx context.Context) {
	if w.activeBackup != nil {
		log.Infof("[worker] active backup %s snapshot %s is running", w.activeBackup.snapshot.Spec.BackupId, w.activeBackup.snapshot.Name)
		return
	}

	snapshotId, ok := w.getBackupQueue()
	if !ok {
		return
	}

	log.Infof("[worker] backup queue - snapshot %s", snapshotId)

	// move to backup handler

	snapshot, err := w.handlers.GetSnapshotHandler().GetSnapshot(context.Background(), snapshotId)
	if err != nil {
		log.Errorf("[worker] get snapshot %s error: %v", snapshotId, err)
		return
	}

	backup, err := w.handlers.GetBackupHandler().GetBackupById(ctx, snapshot.Spec.BackupId)
	if err != nil {
		log.Errorf("[worker] get backup %s error: %v", snapshot.Spec.BackupId, err)
		return
	}

	var ctxTask, cancelTask = context.WithCancel(ctx)
	w.activeBackup = &activeBackup{
		ctx:      ctxTask,
		cancel:   cancelTask,
		snapshot: snapshot,
	}
	log.Infof("[worker] run backup %s snapshot %s", backup.Spec.Name, snapshotId)

	// ~ run backup
	if err := w.handlers.GetSnapshotHandler().Backup(w.ctx, backup, snapshot); err != nil {
		log.Errorf("[worker] backup %s snapshot %s error: %v", backup.Spec.Name, snapshotId, err)
	}

	w.clearActiveBackup()
}

func (w *WorkerManage) AppendBackupTask(snapshot string) {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	w.backupQueue = append(w.backupQueue, snapshot)
}

func (w *WorkerManage) AppendRestoreTask(snapshot string) {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	w.restoreQueue = append(w.restoreQueue, snapshot)
}

func (w *WorkerManage) StopBackup(snapshotId string) {
	if w.activeBackup.snapshot.Name != snapshotId {
		log.Warnf("active backup %s not equal %s", w.activeBackup.snapshot.Name, snapshotId)
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

func (w *WorkerManage) getBackupQueue() (string, bool) {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	if len(w.backupQueue) == 0 {
		return "", false
	}

	first := w.backupQueue[0]
	w.backupQueue = w.backupQueue[1:]

	return first, true
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
