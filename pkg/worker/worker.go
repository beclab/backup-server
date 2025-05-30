package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	pond "github.com/alitto/pond/v2"
)

var workerPool *WorkerPool

type TaskType string

const (
	BackupTaskType  TaskType = "backup"
	RestoreTaskType TaskType = "restore"
	ExecuteTaskType TaskType = "execute"
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
	executePool *TaskPool
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

type ExecuteTask struct {
	*BaseTask
	backupId   string
	snapshotId string
	backup     *storage.StorageBackup

	restoreId string
	restore   *storage.StorageRestore
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

func newPool() pond.Pool {
	return pond.NewPool(constant.MaxConcurrency, pond.WithContext(context.Background()), pond.WithNonBlocking(constant.NonBlocking))
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
		executePool: &TaskPool{
			taskType: ExecuteTaskType,
			pool:     newPool(),
			owner:    owner,
		},
	}

	w.userPools.Store(owner, userPool)

	return userPool
}

func (w *WorkerPool) AddBackupTask(owner, backupId, snapshotId string) error {
	if err := w.ExistsTask(owner, backupId, snapshotId); err != nil {
		return err
	}

	if backupId == "" || snapshotId == "" {
		return fmt.Errorf("backup task parms invalid, backupId: %s, snapshotId: %s", backupId, snapshotId)
	}

	var taskType = BackupTaskType
	var taskId = fmt.Sprintf("%s_%s", backupId, snapshotId)
	var traceId = snapshotId

	var ctxTask, cancelTask = context.WithCancel(w.ctx)
	var ctxBase context.Context = context.WithValue(ctxTask, constant.TraceId, traceId)

	var newTask = &BaseTask{
		ctx:      ctxBase,
		cancel:   cancelTask,
		owner:    owner,
		id:       taskId,
		taskType: taskType,
	}

	var backup = &storage.StorageBackup{
		Ctx:              ctxBase,
		Handlers:         w.handlers,
		SnapshotId:       snapshotId,
		LastProgressTime: time.Now(),
	}

	var executeTask = &ExecuteTask{
		BaseTask:   newTask,
		backupId:   backupId,
		snapshotId: snapshotId,
		backup:     backup,
	}

	return w.addTask(owner, taskId, taskType, executeTask)
}

func (w *WorkerPool) AddRestoreTask(owner, restoreId string) error {
	if restoreId == "" {
		return fmt.Errorf("restore task parms invalid, restoreId: %s", restoreId)
	}

	var taskType = RestoreTaskType
	var taskId = restoreId
	var traceId = restoreId

	var ctxTask, cancelTask = context.WithCancel(w.ctx)
	var ctxBase context.Context = context.WithValue(ctxTask, constant.TraceId, traceId)

	var newTask = &BaseTask{
		ctx:      ctxBase,
		cancel:   cancelTask,
		owner:    owner,
		id:       taskId,
		taskType: taskType,
	}

	var restore = &storage.StorageRestore{
		Ctx:              ctxBase,
		Handlers:         w.handlers,
		RestoreId:        restoreId,
		LastProgressTime: time.Now(),
	}

	var executeTask = &ExecuteTask{
		BaseTask:  newTask,
		restoreId: restoreId,
		restore:   restore,
	}

	return w.addTask(owner, taskId, taskType, executeTask)
}

func (w *WorkerPool) addTask(owner, taskId string, taskType TaskType, task *ExecuteTask) error {
	userPool := w.GetOrCreateUserPool(owner)
	taskPool := userPool.executePool

	taskPool.tasks.Store(taskId, task)

	taskFn := func() {
		defer func() {
			taskPool.tasks.Delete(taskId)
			taskPool.setActiveTask(nil)

			if r := recover(); r != nil {
				log.Errorf("[worker] task panic: %v, owner: %s, taskId: %s, taskType: %s", r, owner, taskId, taskType)
			}
		}()

		if task.IsCanceled() {
			return
		}

		log.Infof("[worker] task start, owner: %s, taskId: %s, taskType: %s", owner, taskId, taskType)
		taskPool.setActiveTask(task)

		if taskType == BackupTaskType {
			task.backup.RunBackup()
		} else {
			task.restore.RunRestore()
		}
	}

	_, ok := taskPool.pool.TrySubmit(taskFn)
	if !ok {
		task.BaseTask.cancel()
		taskPool.tasks.Delete(taskId)
		return fmt.Errorf("[worker] task try sumit failed, owner: %s, queuesize: %d, waitings: %d", owner, taskPool.pool.QueueSize(), taskPool.pool.WaitingTasks())
	}

	return nil
}

func (w *WorkerPool) ExistsTask(owner string, backupId, snapshotId string) error {
	poolObj, ok := w.userPools.Load(owner)
	if !ok {
		log.Warnf("[worker] no backup tasks found for owner: %s", owner)
		return nil
	}

	userPool := poolObj.(*UserPool)
	taskPool := userPool.executePool

	var snapshotExists bool

	taskPool.tasks.Range(func(key, value interface{}) bool {
		task := value.(*ExecuteTask)
		if task.backupId == backupId && !task.IsCanceled() {
			snapshotExists = true
			return false
		}
		return true
	})

	if snapshotExists {
		return fmt.Errorf("the current snapshot task is still running or queued. the system will pause adding new tasks and automatically resume scheduling once the task is completed.")
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
	taskPool := userPool.executePool

	taskPool.tasks.Range(func(key, value interface{}) bool {
		task := value.(*ExecuteTask)
		if task.backupId == backupId {
			task.BaseTask.canceled = true
		}
		return true
	})

	activeTask := taskPool.activeTask.Load()
	if activeTask != nil {
		backupTask := activeTask.(*ExecuteTask)
		if backupTask.backupId == backupId {
			w.sendBackupCanceledEvent(backupTask.owner, backupTask.backupId, backupTask.snapshotId)
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
	taskPool := userPool.executePool

	taskPool.tasks.Range(func(key, value interface{}) bool {
		task := value.(*ExecuteTask)
		if task.snapshotId == snapshotId {
			task.BaseTask.canceled = true
		}
		return true
	})

	activeTask := taskPool.activeTask.Load()
	if activeTask != nil {
		backupTask := activeTask.(*ExecuteTask)
		if backupTask.snapshotId == snapshotId {
			w.sendBackupCanceledEvent(backupTask.owner, backupTask.backupId, backupTask.snapshotId)
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
	taskPool := userPool.executePool

	taskPool.tasks.Range(func(key, value interface{}) bool {
		task := value.(*ExecuteTask)
		if task.restoreId == restoreId {
			task.BaseTask.canceled = true
		}
		return true
	})

	activeTask := taskPool.activeTask.Load()
	if activeTask != nil {
		restoreTask := activeTask.(*ExecuteTask)
		if restoreTask.restoreId == restoreId {
			w.sendRestoreCanceledEvent(restoreTask.owner, restoreTask.restoreId)
			restoreTask.BaseTask.cancel()
		}
	}
}

func (w *WorkerPool) sendBackupCanceledEvent(owner string, backupId string, snapshotId string) {
	w.handlers.GetNotification().Send(w.ctx, constant.EventBackup, owner, "backup canceled", map[string]interface{}{
		"id":       snapshotId,
		"backupId": backupId,
		"endat":    time.Now().Unix(),
		"status":   constant.Canceled.String(),
		"message":  "",
	})
}

func (w *WorkerPool) sendRestoreCanceledEvent(owner string, restoreId string) {
	w.handlers.GetNotification().Send(w.ctx, constant.EventRestore, owner, "restore canceled", map[string]interface{}{
		"id":      restoreId,
		"endat":   time.Now().Unix(),
		"status":  constant.Canceled.String(),
		"message": "",
	})
}
