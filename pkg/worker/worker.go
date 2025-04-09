package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	pond "github.com/alitto/pond/v2"
)

const (
	maxConcurrency   = 1
	backupQueueSize  = 5
	restoreQueueSize = 5
	nonBlocking      = true
)

var workerPool *WorkerPool

type TaskType string

const (
	BackupTaskType  TaskType = "backup"
	RestoreTaskType TaskType = "restore"
)

type WorkerPool struct {
	ctx      context.Context
	cancel   context.CancelFunc
	handlers handlers.Interface

	userPools sync.Map

	sync.RWMutex
}

type TaskPool struct {
	taskType   TaskType
	pool       pond.Pool
	tasks      sync.Map
	activeTask atomic.Value
	owner      string
	sync.RWMutex
}

func (tp *TaskPool) setActiveTask(task interface{}) {
	tp.activeTask.Store(task)
}

type UserPool struct {
	owner       string
	backupPool  *TaskPool
	restorePool *TaskPool
	sync.RWMutex
}

type BaseTask struct {
	ctx      context.Context
	cancel   context.CancelFunc
	id       string
	owner    string
	taskType TaskType
	canceled bool
	mutex    sync.RWMutex
}

func (t *BaseTask) IsCanceled() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.canceled
}

type BackupTask struct {
	*BaseTask
	backupId   string
	snapshotId string
	backup     *storage.StorageBackup
}

type RestoreTask struct {
	*BaseTask
	restoreId string
	restore   *storage.StorageRestore
}

func newPool(queueSize int) pond.Pool {
	return pond.NewPool(maxConcurrency, pond.WithContext(context.Background()), pond.WithQueueSize(queueSize), pond.WithNonBlocking(nonBlocking))
}

func GetWorkerPool() *WorkerPool {
	return workerPool
}

func NewWorkerPool(ctx context.Context, handlers handlers.Interface) {
	ctx, cancel := context.WithCancel(ctx)

	workerPool = &WorkerPool{
		ctx:      ctx,
		cancel:   cancel,
		handlers: handlers,
	}
}

func (w *WorkerPool) GetOrCreateUserPool(owner string) *UserPool {
	if pool, ok := w.userPools.Load(owner); ok {
		userPool := pool.(*UserPool)
		return userPool
	}

	userPool := &UserPool{
		owner: owner,
		backupPool: &TaskPool{
			taskType: BackupTaskType,
			pool:     newPool(backupQueueSize),
			owner:    owner,
		},
		restorePool: &TaskPool{
			taskType: RestoreTaskType,
			pool:     newPool(restoreQueueSize),
			owner:    owner,
		},
	}

	w.userPools.Store(owner, userPool)

	return userPool
}

func (w *WorkerPool) AddBackupTask(owner, backupId, snapshotId string) error {
	userPool := w.GetOrCreateUserPool(owner)
	taskPool := userPool.backupPool

	var ctxTask, cancelTask = context.WithCancel(w.ctx)

	taskId := fmt.Sprintf("%s_%s", backupId, snapshotId)

	baseTask := &BaseTask{
		ctx:      ctxTask,
		cancel:   cancelTask,
		owner:    owner,
		id:       taskId,
		taskType: BackupTaskType,
	}

	backup := &storage.StorageBackup{
		Ctx:              ctxTask,
		Handlers:         w.handlers,
		SnapshotId:       snapshotId,
		LastProgressTime: time.Now(),
	}

	var backupTask = &BackupTask{
		BaseTask:   baseTask,
		backupId:   backupId,
		snapshotId: snapshotId,
		backup:     backup,
	}

	taskPool.tasks.Store(taskId, backupTask)

	taskFn := func() {
		defer func() {
			taskPool.tasks.Delete(taskId)
			taskPool.setActiveTask(nil)

			if r := recover(); r != nil {
				log.Errorf("[worker] backup task panic: %v, owner: %s, backupId: %s, snapshotId: %s", r, owner, backupId, snapshotId)
			}
		}()

		if backupTask.IsCanceled() {
			return
		}

		log.Infof("[worker] backup task start, owner: %s, backupId: %s, snapshotId: %s", owner, backupId, snapshotId)
		taskPool.setActiveTask(backupTask)

		if err := backupTask.backup.RunBackup(); err != nil {
			log.Errorf("[worker] backup task failed, owner: %s, backupId: %s, snapshotId: %s, err: %s", owner, backupId, snapshotId, err.Error())
		} else {
			log.Infof("[worker] backup task success, owner: %s, backupId: %s, snapshotId: %s", owner, backupId, snapshotId)
		}
	}

	_, ok := taskPool.pool.TrySubmit(taskFn)
	if !ok {
		cancelTask()
		taskPool.tasks.Delete(taskId)
		return fmt.Errorf("[worker] backup task queue is full for owner: %s", owner)
	}

	return nil
}

func (w *WorkerPool) AddRestoreTask(owner, restoreId string) error {

	userPool := w.GetOrCreateUserPool(owner)
	taskPool := userPool.restorePool

	taskCtx, cancelTask := context.WithCancel(w.ctx)

	taskId := restoreId

	baseTask := &BaseTask{
		ctx:      taskCtx,
		cancel:   cancelTask,
		owner:    owner,
		id:       taskId,
		taskType: RestoreTaskType,
	}

	restore := &storage.StorageRestore{
		Ctx:              taskCtx,
		Handlers:         w.handlers,
		RestoreId:        restoreId,
		LastProgressTime: time.Now(),
	}

	var restoreTask = &RestoreTask{
		BaseTask:  baseTask,
		restoreId: restoreId,
		restore:   restore,
	}

	taskPool.tasks.Store(taskId, restoreTask)

	taskFn := func() {
		defer func() {
			taskPool.tasks.Delete(taskId)
			taskPool.setActiveTask(nil)

			if r := recover(); r != nil {
				log.Errorf("[worker] restore task panic: %v, owner: %s, restoreId: %s", r, owner, restoreId)
			}
		}()

		if restoreTask.IsCanceled() {
			return
		}

		log.Infof("[worker] restore task start, owner: %s, restoreId: %s", owner, restoreId)
		taskPool.setActiveTask(restoreTask)

		if err := restoreTask.restore.RunRestore(); err != nil {
			log.Errorf("[worker] restore task failed, owner: %s, restoreId: %s, err: %s", owner, restoreId, err.Error())
		} else {
			log.Infof("[worker] restore task success, owner: %s, restoreId: %s", owner, restoreId)
		}
	}

	_, ok := taskPool.pool.TrySubmit(taskFn)
	if !ok {
		cancelTask()
		taskPool.tasks.Delete(taskId)
		return fmt.Errorf("[worker] restore task queue is full for owner: %s", owner)
	}

	return nil
}

func (w *WorkerPool) CancelBackup(owner, backupId string) {

	log.Infof("[worker] cancel backup, owner: %s, backupId: %s", owner, backupId)

	poolObj, ok := w.userPools.Load(owner)
	if !ok {
		log.Warnf("[worker] no backup tasks found for owner: %s", owner)
		return
	}

	userPool := poolObj.(*UserPool)
	taskPool := userPool.backupPool

	taskPool.tasks.Range(func(key, value interface{}) bool {
		task := value.(*BackupTask)
		if task.backupId == backupId {
			task.BaseTask.canceled = true
		}
		return true
	})

	activeTask := taskPool.activeTask.Load()
	if activeTask != nil {
		backupTask := activeTask.(*BackupTask)
		if backupTask.backupId == backupId {
			backupTask.BaseTask.cancel()
		}
	}
}

func (w *WorkerPool) CancelSnapshot(owner, snapshotId string) {
	log.Infof("[worker] cancel snapshot, owner: %s, snapshotId: %s", owner, snapshotId)

	poolObj, ok := w.userPools.Load(owner)
	if !ok {
		log.Warnf("[worker] no backup tasks found for owner: %s", owner)
		return
	}

	userPool := poolObj.(*UserPool)
	taskPool := userPool.backupPool

	taskPool.tasks.Range(func(key, value interface{}) bool {
		task := value.(*BackupTask)
		if task.snapshotId == snapshotId {
			task.BaseTask.canceled = true
		}
		return true
	})

	activeTask := taskPool.activeTask.Load()
	if activeTask != nil {
		backupTask := activeTask.(*BackupTask)
		if backupTask.snapshotId == snapshotId {
			backupTask.BaseTask.cancel()
		}
	}
}

func (w *WorkerPool) CancelRestore(owner, restoreId string) {

	log.Infof("[worker] cancel restore, owner: %s, restoreId: %s", owner, restoreId)

	poolObj, ok := w.userPools.Load(owner)
	if !ok {
		log.Warn("[worker] no restore tasks found for owner: %s", owner)
		return
	}

	userPool := poolObj.(*UserPool)
	taskPool := userPool.restorePool

	taskPool.tasks.Range(func(key, value interface{}) bool {
		task := value.(*RestoreTask)
		if task.restoreId == restoreId {
			task.BaseTask.canceled = true
		}
		return true
	})

	activeTask := taskPool.activeTask.Load()
	if activeTask != nil {
		restoreTask := activeTask.(*RestoreTask)
		if restoreTask.restoreId == restoreId {
			restoreTask.BaseTask.cancel()
		}
	}
}
