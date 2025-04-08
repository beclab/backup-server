package worker

import (
	"context"
	"fmt"
	"sync"

	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	pond "github.com/alitto/pond/v2"
)

const (
	maxConcurrency   = 1
	backupQueueSize  = 20
	restoreQueueSize = 20
	nonBlocking      = true
)

var workerPool *WorkerPool

type WorkerPool struct {
	ctx      context.Context
	cancel   context.CancelFunc
	handlers handlers.Interface

	activeBackupTask  *BackupTask
	activeRestoreTask *RestoreTask

	backupTasks  sync.Map
	restoreTasks sync.Map

	backupPool  pond.Pool
	restorePool pond.Pool

	sync.Mutex
}

type BackupTask struct {
	ctx        context.Context
	cancel     context.CancelFunc
	task       pond.Task
	owner      string
	backupId   string
	snapshotId string
	backup     *storage.StorageBackup
	canceled   bool
}

type RestoreTask struct {
	ctx       context.Context
	cancel    context.CancelFunc
	task      pond.Task
	owner     string
	restoreId string
	restore   *storage.StorageRestore
	canceled  bool
}

func NewWorkerPool(ctx context.Context, handlers handlers.Interface) {
	workerPool = &WorkerPool{
		ctx:         ctx,
		backupPool:  newPool(backupQueueSize),
		restorePool: newPool(restoreQueueSize),
		handlers:    handlers,
	}
}

func GetWorkerPool() *WorkerPool {
	return workerPool
}

func newPool(queueSize int) pond.Pool {
	return pond.NewPool(maxConcurrency, pond.WithContext(context.Background()), pond.WithQueueSize(queueSize), pond.WithNonBlocking(nonBlocking))
}

func (w *WorkerPool) AddBackupTask(owner string, backupId string, snapshotId string) {

	log.Infof("[worker] backup task added, snapshotId: %s, waitings: %d, runnings: %d", snapshotId, w.backupPool.WaitingTasks(), w.backupPool.RunningWorkers())

	var ctxTask, cancelTask = context.WithCancel(w.ctx)

	var backup = &storage.StorageBackup{
		Ctx:        ctxTask,
		Handlers:   w.handlers,
		SnapshotId: snapshotId,
	}

	var backupTask = &BackupTask{
		ctx:        ctxTask,
		cancel:     cancelTask,
		owner:      owner,
		backupId:   backupId,
		snapshotId: snapshotId,
		backup:     backup,
	}

	wrappedFn := func() {
		defer func() {
			w.Lock()
			w.activeBackupTask = nil
			w.backupTasks.Delete(fmt.Sprintf("%s_%s_%s", owner, backupId, snapshotId))
			w.Unlock()

			backupTask = nil
		}()

		if backupTask.canceled {
			log.Infof("[worker] backup task canceled: %s_%s_%s", owner, backupId, snapshotId)
			return
		}

		log.Infof("[worker] backup task start, owner: %s, backupId: %s, snapshotId: %s", owner, backupId, snapshotId)

		w.Lock()
		w.backupTasks.Store(fmt.Sprintf("%s_%s_%s", owner, backupId, snapshotId), backupTask)
		w.activeBackupTask = backupTask
		w.Unlock()

		if err := backupTask.backup.RunBackup(); err != nil {
			log.Errorf("[worker] backup task failed, owner: %s, backupId: %s, snapshotId: %s, err: %s", owner, backupId, snapshotId, err.Error())
		} else {
			log.Infof("[worker] backup task success, owner: %s, backupId: %s, snapshotId: %s", owner, backupId, snapshotId)
		}
	}

	_, ok := w.backupPool.TrySubmit(wrappedFn)

	if !ok {
		log.Warn("[worker] backup task submission failed because the queue is full")
		cancelTask()
		backupTask = nil
	}
}

func (w *WorkerPool) AddRestoreTask(owner string, restoreId string) {

	log.Infof("[worker] restore task added, restoreId: %s, waitings: %d, runnings: %d", restoreId, w.restorePool.WaitingTasks(), w.restorePool.RunningWorkers())

	var ctxTask, cancelTask = context.WithCancel(w.ctx)

	var restore = &storage.StorageRestore{
		Ctx:       ctxTask,
		Handlers:  w.handlers,
		RestoreId: restoreId,
	}

	var restoreTask = &RestoreTask{
		ctx:       ctxTask,
		cancel:    cancelTask,
		owner:     owner,
		restoreId: restoreId,
		restore:   restore,
	}

	wrappedFn := func() {
		defer func() {
			w.Lock()
			w.activeRestoreTask = nil
			w.restoreTasks.Delete(fmt.Sprintf("%s_%s", owner, restoreId))
			w.Unlock()
		}()

		if restoreTask.canceled {
			log.Infof("[worker] restore task canceled: %s_%s", owner, restoreId)
			return
		}

		log.Infof("[worker] restore task start, owner: %s, restoreId: %s", owner, restoreId)

		w.Lock()
		w.restoreTasks.Store(fmt.Sprintf("%s_%s", owner, restoreId), restoreTask)
		w.activeRestoreTask = restoreTask
		w.Unlock()

		if err := restoreTask.restore.RunRestore(); err != nil {
			log.Errorf("[worker] restore task failed, owner: %s, restoreId: %s, err: %s", owner, restoreId, err.Error())
		} else {
			log.Infof("[worker] restore task success, owner: %s, restoreId: %s", owner, restoreId)
		}
	}

	_, ok := w.restorePool.TrySubmit(wrappedFn)
	if !ok {
		log.Warn("[worker] restore task submission failed because the queue is full")
		cancelTask()
	}
}

func (w *WorkerPool) CancelBackup(backupId string) {
	w.Lock()
	defer w.Unlock()

	log.Infof("[worker] cancel backup: %s", backupId)

	w.backupTasks.Range(func(key, value interface{}) bool {
		task := value.(*BackupTask)
		if task.backupId == backupId {
			task.canceled = true
		}
		return true
	})

	if w.activeBackupTask != nil {
		task := w.activeBackupTask
		if task.backupId == backupId {
			task.cancel()
		}
	}
}

func (w *WorkerPool) CancelSnapshot(snapshotId string) {
	w.Lock()
	defer w.Unlock()

	log.Infof("[worker] cancel snapshot: %s", snapshotId)

	w.backupTasks.Range(func(key, value interface{}) bool {
		task := value.(*BackupTask)
		if task.snapshotId == snapshotId {
			task.canceled = true
		}
		return true
	})

	if w.activeBackupTask != nil {
		task := w.activeBackupTask
		if task.snapshotId == snapshotId {
			task.cancel()
		}
	}
}

func (w *WorkerPool) CancelRestore(restoreId string) {
	w.Lock()
	defer w.Unlock()

	log.Infof("[worker] cancel restore: %s", restoreId)

	w.restoreTasks.Range(func(key, value interface{}) bool {
		task := value.(*RestoreTask)
		if task.restoreId == restoreId {
			task.canceled = true
		}
		return true
	})

	if w.activeRestoreTask != nil {
		task := w.activeRestoreTask
		if task.restoreId == restoreId {
			task.cancel()
		}
	}
}
