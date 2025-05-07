package cron

import (
	"context"
	"strings"
	"sync"

	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"bytetrade.io/web3os/backup-server/pkg/postgres"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/worker"
	"github.com/robfig/cron/v3"
)

type backupJob struct {
	name string
	f    func()
}

func (b backupJob) Run() { b.f() }

var c *CronJob

type CronJob struct {
	cron    *cron.Cron
	handler *handlers.SnapshotHandler
	sync.Mutex
}

func NewCronJob(snapshotHandler *handlers.SnapshotHandler) {
	c = new(CronJob)
	c.cron = cron.New()
	c.cron.Start()
}

func NewSchedule(ctx context.Context, backup *postgres.Backup, schedule string, paused bool) error {
	c.Lock()
	defer c.Unlock()

	var policy, err = postgres.ParseBackupPolicy(backup.BackupPolicy)
	if err != nil {
		return err
	}

	log.Infof("create snapshot schedule, name: %s, frequency: %s, schedule: %s", backup.BackupName, policy.SnapshotFrequency, schedule)

	entries := c.cron.Entries()
	for _, e := range entries {
		if e.Job.(backupJob).name == backup.BackupId {
			log.Info("remove prev cron job to apply new one")
			c.cron.Remove(e.ID)
		}
	}

	_, err = c.cron.AddJob(schedule, backupJob{
		name: backup.BackupId,
		f: func() {
			log.Infof("prepare to create snapshot task, name: %s, id: %s", backup.BackupName, backup.BackupId)

			var location string
			for k := range backup.Location {
				location = k
				break
			}

			newSnapshot, err := c.handler.CreateToSql(ctx, backup, location)
			if err != nil {
				log.Error("create snapshot task error, ", err)
			}

			var phase string = constant.Failed.String()

			err = worker.GetWorkerPool().AddBackupTask(newSnapshot.Owner, newSnapshot.BackupId, newSnapshot.SnapshotId)
			if strings.Contains(err.Error(), "queue is full") {
				phase = constant.Rejected.String()
			}

			if err = postgres.UpdateSnapshotFailed(ctx, newSnapshot.SnapshotId, phase, err.Error()); err != nil {
				log.Error(err)
			}
		},
	})

	if err != nil {
		log.Error("add snapshot schedule error, ", err)
	}

	return err
}
