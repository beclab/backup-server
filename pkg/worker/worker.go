package worker

import (
	"context"
	"sync"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/modules/backup/v1/operator"

	"bytetrade.io/web3os/backup-server/pkg/util/log"
)

var Worker *WorkerManage

type WorkerManage struct {
	ctx              context.Context
	backupOperator   *operator.BackupOperator
	snapshotOperator *operator.SnapshotOperator

	snapshotQueue []string
	restoreQueue  []string

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
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWorkerManage(ctx context.Context, backupOperator *operator.BackupOperator, snapshotOperator *operator.SnapshotOperator) *WorkerManage {
	Worker = &WorkerManage{
		backupOperator:   backupOperator,
		snapshotOperator: snapshotOperator,
		ctx:              ctx,
	}
	return Worker
}

func (w *WorkerManage) StartBackup() {
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

func (w *WorkerManage) RunBackup(ctx context.Context) {
	if w.activeBackup != nil {
		log.Infof("[worker] active backup %s snapshot %s is running", w.activeBackup.snapshot.Spec.BackupId, w.activeBackup.snapshot.Name)
		return
	}

	snapshotId, ok := w.getSnapshotQueue()
	if !ok {
		return
	}

	log.Infof("[worker] backup queue - snapshot %s", snapshotId)

	snapshot, err := w.snapshotOperator.GetSnapshot(context.Background(), snapshotId)
	if err != nil {
		log.Errorf("[worker] get snapshot %s error %v", snapshotId, err)
		return
	}

	backup, err := w.backupOperator.GetBackupById(ctx, snapshot.Spec.BackupId)
	if err != nil {
		log.Errorf("[worker] get backup %s error %v", snapshot.Spec.BackupId, err)
		return
	}

	var ctxTask, cancelTask = context.WithCancel(ctx)
	w.activeBackup = &activeBackup{
		ctx:      ctxTask,
		cancel:   cancelTask,
		snapshot: snapshot,
	}
	log.Infof("[worker] run backup %s snapshot %s", backup.Spec.Name, snapshot.Name)
	if err := w.snapshotOperator.Backup(w.ctx, backup, snapshot); err != nil {
		log.Errorf("[worker] backup %s snapshot error %v", backup.Name, err)
	}

	w.clearActiveBackup()
}

func (w *WorkerManage) AppendSnapshotTask(snapshot string) {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	w.snapshotQueue = append(w.snapshotQueue, snapshot)
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

func (w *WorkerManage) getSnapshotQueue() (string, bool) {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	if len(w.snapshotQueue) == 0 {
		return "", false
	}

	first := w.snapshotQueue[0]
	w.snapshotQueue = w.snapshotQueue[1:]

	return first, true
}

func (w *WorkerManage) isSnapshotQueueEmpty() bool {
	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()
	return len(w.snapshotQueue) == 0
}
