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
	owner string
	// backupPool  *TaskPool
	// restorePool *TaskPool
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
		// backupPool: &TaskPool{
		// 	taskType: BackupTaskType,
		// 	pool:     newPool(constant.BackupQueueSize),
		// 	owner:    owner,
		// },
		// restorePool: &TaskPool{
		// 	taskType: RestoreTaskType,
		// 	pool:     newPool(constant.RestoreQueueSize),
		// 	owner:    owner,
		// },
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
	if err := w.ExistsTask(owner, backupId); err != nil {
		return err
	}
	return w.addTask(owner, backupId, snapshotId, "")
}

func (w *WorkerPool) AddRestoreTask(owner, restoreId string) error {
	return w.addTask(owner, "", "", restoreId)
}

func (w *WorkerPool) addTask(owner, backupId, snapshotId, restoreId string) error {
	userPool := w.GetOrCreateUserPool(owner)
	taskPool := userPool.executePool

	if (backupId == "" || snapshotId == "") && restoreId == "" {
		return fmt.Errorf("task parms invalid, backupId: %s, snapshotId: %s, restoreId: %s", backupId, snapshotId, restoreId)
	}

	var ctxTask, cancelTask = context.WithCancel(w.ctx)
	var ctxBase context.Context

	var taskType TaskType

	var taskId string
	if restoreId != "" {
		taskId = restoreId
		taskType = RestoreTaskType
		ctxBase = context.WithValue(ctxTask, constant.TraceId, restoreId)
	} else {
		taskId = fmt.Sprintf("%s_%s", backupId, snapshotId)
		taskType = BackupTaskType
		ctxBase = context.WithValue(ctxTask, constant.TraceId, snapshotId)
	}

	var executeTask *ExecuteTask

	var newTask = &BaseTask{
		ctx:      ctxBase,
		cancel:   cancelTask,
		owner:    owner,
		id:       taskId,
		taskType: taskType,
	}

	switch taskType {
	case BackupTaskType:
		var backup = &storage.StorageBackup{
			Ctx:              ctxBase,
			Handlers:         w.handlers,
			SnapshotId:       snapshotId,
			LastProgressTime: time.Now(),
		}
		executeTask = &ExecuteTask{
			BaseTask:   newTask,
			backupId:   backupId,
			snapshotId: snapshotId,
			backup:     backup,
		}
	case RestoreTaskType:
		var restore = &storage.StorageRestore{
			Ctx:              ctxBase,
			Handlers:         w.handlers,
			RestoreId:        restoreId,
			LastProgressTime: time.Now(),
		}
		executeTask = &ExecuteTask{
			BaseTask:  newTask,
			restoreId: restoreId,
			restore:   restore,
		}
	}

	taskPool.tasks.Store(taskId, executeTask)

	taskFn := func() {
		defer func() {
			taskPool.tasks.Delete(taskId)
			taskPool.setActiveTask(nil)

			if r := recover(); r != nil {
				log.Errorf("[worker] task panic: %v, owner: %s, taskId: %s, taskType: %s", r, owner, taskId, taskType)
			}
		}()

		if executeTask.IsCanceled() {
			return
		}

		log.Infof("[worker] task start, owner: %s, taskId: %s, taskType: %s", owner, taskId, taskType)
		taskPool.setActiveTask(executeTask)

		if taskType == BackupTaskType {
			executeTask.backup.RunBackup()
		} else {
			executeTask.restore.RunRestore()
		}
	}

	_, ok := taskPool.pool.TrySubmit(taskFn)
	if !ok {
		cancelTask()
		taskPool.tasks.Delete(taskId)
		return fmt.Errorf("[worker] task try sumit failed, owner: %s, queuesize: %d, waitings: %d", owner, taskPool.pool.QueueSize(), taskPool.pool.WaitingTasks())
	}

	return nil
}

func (w *WorkerPool) ExistsTask(owner string, backupId string) error {
	poolObj, ok := w.userPools.Load(owner)
	if !ok {
		log.Warnf("[worker] no backup tasks found for owner: %s", owner)
		return nil
	}

	userPool := poolObj.(*UserPool)
	taskPool := userPool.executePool

	var backupExists bool

	taskPool.tasks.Range(func(key, value interface{}) bool {
		task := value.(*ExecuteTask)
		if task.backupId == backupId {
			backupExists = true
		}
		return true
	})

	if backupExists {
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
			backupTask.BaseTask.cancel()
			w.sendBackupCanceledEvent(backupTask.owner, backupTask.backupId, backupTask.snapshotId)
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
			backupTask.BaseTask.cancel()
			w.sendBackupCanceledEvent(backupTask.owner, backupTask.backupId, backupTask.snapshotId)
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
			restoreTask.BaseTask.cancel()
			w.sendRestoreCanceledEvent(restoreTask.owner, restoreTask.restoreId)
		}
	}
}

// func (w *WorkerPool) AddBackupTask(owner, backupId, snapshotId string) error {
// 	userPool := w.GetOrCreateUserPool(owner)
// 	taskPool := userPool.backupPool

// 	var ctxTask, cancelTask = context.WithCancel(w.ctx)
// 	var c = context.WithValue(ctxTask, constant.TraceId, snapshotId)

// 	taskId := fmt.Sprintf("%s_%s", backupId, snapshotId)

// 	baseTask := &BaseTask{
// 		ctx:      c,
// 		cancel:   cancelTask,
// 		owner:    owner,
// 		id:       taskId,
// 		taskType: BackupTaskType,
// 	}

// 	backup := &storage.StorageBackup{
// 		Ctx:              c,
// 		Handlers:         w.handlers,
// 		SnapshotId:       snapshotId,
// 		LastProgressTime: time.Now(),
// 	}

// 	var backupTask = &BackupTask{
// 		BaseTask:   baseTask,
// 		backupId:   backupId,
// 		snapshotId: snapshotId,
// 		backup:     backup,
// 	}

// 	taskPool.tasks.Store(taskId, backupTask)

// 	taskFn := func() {
// 		defer func() {
// 			taskPool.tasks.Delete(taskId)
// 			taskPool.setActiveTask(nil)

// 			if r := recover(); r != nil {
// 				log.Errorf("[worker] backup task panic: %v, owner: %s, backupId: %s, snapshotId: %s", r, owner, backupId, snapshotId)
// 			}
// 		}()

// 		if backupTask.IsCanceled() {
// 			return
// 		}

// 		log.Infof("[worker] backup task start, owner: %s, backupId: %s, snapshotId: %s", owner, backupId, snapshotId)
// 		taskPool.setActiveTask(backupTask)

// 		if err := backupTask.backup.RunBackup(); err != nil {
// 			log.Errorf("[worker] backup task failed, owner: %s, backupId: %s, snapshotId: %s, err: %s", owner, backupId, snapshotId, err.Error())
// 		} else {
// 			log.Infof("[worker] backup task success, owner: %s, backupId: %s, snapshotId: %s", owner, backupId, snapshotId)
// 		}
// 	}

// 	_, ok := taskPool.pool.TrySubmit(taskFn)
// 	if !ok {
// 		cancelTask()
// 		taskPool.tasks.Delete(taskId)
// 		return fmt.Errorf("[worker] backup task queue is full for owner: %s, queuesize: %d, waitings: %d", owner, taskPool.pool.QueueSize(), taskPool.pool.WaitingTasks())
// 	}

// 	return nil
// }

// func (w *WorkerPool) AddRestoreTask(owner, restoreId string) error {

// 	userPool := w.GetOrCreateUserPool(owner)
// 	taskPool := userPool.restorePool

// 	taskCtx, cancelTask := context.WithCancel(w.ctx)

// 	taskId := restoreId

// 	baseTask := &BaseTask{
// 		ctx:      taskCtx,
// 		cancel:   cancelTask,
// 		owner:    owner,
// 		id:       taskId,
// 		taskType: RestoreTaskType,
// 	}

// 	restore := &storage.StorageRestore{
// 		Ctx:              taskCtx,
// 		Handlers:         w.handlers,
// 		RestoreId:        restoreId,
// 		LastProgressTime: time.Now(),
// 	}

// 	var restoreTask = &RestoreTask{
// 		BaseTask:  baseTask,
// 		restoreId: restoreId,
// 		restore:   restore,
// 	}

// 	taskPool.tasks.Store(taskId, restoreTask)

// 	taskFn := func() {
// 		defer func() {
// 			taskPool.tasks.Delete(taskId)
// 			taskPool.setActiveTask(nil)

// 			if r := recover(); r != nil {
// 				log.Errorf("[worker] restore task panic: %v, owner: %s, restoreId: %s", r, owner, restoreId)
// 			}
// 		}()

// 		if restoreTask.IsCanceled() {
// 			return
// 		}

// 		log.Infof("[worker] restore task start, owner: %s, restoreId: %s", owner, restoreId)
// 		taskPool.setActiveTask(restoreTask)

// 		if err := restoreTask.restore.RunRestore(); err != nil {
// 			log.Errorf("[worker] restore task failed, owner: %s, restoreId: %s, err: %s", owner, restoreId, err.Error())
// 		} else {
// 			log.Infof("[worker] restore task success, owner: %s, restoreId: %s", owner, restoreId)
// 		}
// 	}

// 	_, ok := taskPool.pool.TrySubmit(taskFn)
// 	if !ok {
// 		cancelTask()
// 		taskPool.tasks.Delete(taskId)
// 		return fmt.Errorf("[worker] restore task queue is full for owner: %s, queuesize: %d, waitings: %d", owner, taskPool.pool.QueueSize(), taskPool.pool.WaitingTasks())
// 	}

// 	return nil
// }

// func (w *WorkerPool) CancelBackup(owner, backupId string) {

// 	log.Infof("[worker] cancel backup, owner: %s, backupId: %s", owner, backupId)

// 	poolObj, ok := w.userPools.Load(owner)
// 	if !ok {
// 		log.Warnf("[worker] no backup tasks found for owner: %s", owner)
// 		return
// 	}

// 	userPool := poolObj.(*UserPool)
// 	taskPool := userPool.backupPool

// 	taskPool.tasks.Range(func(key, value interface{}) bool {
// 		task := value.(*BackupTask)
// 		if task.backupId == backupId {
// 			task.BaseTask.canceled = true
// 		}
// 		return true
// 	})

// 	activeTask := taskPool.activeTask.Load()
// 	if activeTask != nil {
// 		backupTask := activeTask.(*BackupTask)
// 		if backupTask.backupId == backupId {
// 			backupTask.BaseTask.cancel()
// 			w.sendBackupCanceledEvent(backupTask.owner, backupTask.backupId, backupTask.snapshotId)
// 		}
// 	}
// }

// func (w *WorkerPool) CancelSnapshot(owner, snapshotId string) {
// 	log.Infof("[worker] cancel snapshot, owner: %s, snapshotId: %s", owner, snapshotId)

// 	poolObj, ok := w.userPools.Load(owner)
// 	if !ok {
// 		log.Warnf("[worker] no backup tasks found for owner: %s", owner)
// 		return
// 	}

// 	userPool := poolObj.(*UserPool)
// 	taskPool := userPool.backupPool

// 	taskPool.tasks.Range(func(key, value interface{}) bool {
// 		task := value.(*BackupTask)
// 		if task.snapshotId == snapshotId {
// 			task.BaseTask.canceled = true
// 		}
// 		return true
// 	})

// 	activeTask := taskPool.activeTask.Load()
// 	if activeTask != nil {
// 		backupTask := activeTask.(*BackupTask)
// 		if backupTask.snapshotId == snapshotId {
// 			backupTask.BaseTask.cancel()
// 			w.sendBackupCanceledEvent(backupTask.owner, backupTask.backupId, backupTask.snapshotId)
// 		}
// 	}
// }

// func (w *WorkerPool) CancelRestore(owner, restoreId string) {

// 	log.Infof("[worker] cancel restore, owner: %s, restoreId: %s", owner, restoreId)

// 	poolObj, ok := w.userPools.Load(owner)
// 	if !ok {
// 		log.Warn("[worker] no restore tasks found for owner: %s", owner)
// 		return
// 	}

// 	userPool := poolObj.(*UserPool)
// 	taskPool := userPool.restorePool

// 	taskPool.tasks.Range(func(key, value interface{}) bool {
// 		task := value.(*RestoreTask)
// 		if task.restoreId == restoreId {
// 			task.BaseTask.canceled = true
// 		}
// 		return true
// 	})

// 	activeTask := taskPool.activeTask.Load()
// 	if activeTask != nil {
// 		restoreTask := activeTask.(*RestoreTask)
// 		if restoreTask.restoreId == restoreId {
// 			restoreTask.BaseTask.cancel()
// 			w.sendRestoreCanceledEvent(restoreTask.owner, restoreTask.restoreId)
// 		}
// 	}
// }

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
