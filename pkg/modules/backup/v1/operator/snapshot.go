package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/options"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	"bytetrade.io/web3os/backups-sdk/pkg/utils"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

type backupJob struct {
	name string
	f    func()
}

func (b backupJob) Run() { b.f() }

type SnapshotOperator struct {
	factory client.Factory
	cron    *cron.Cron
}

func NewSnapshotOperator(f client.Factory) *SnapshotOperator {
	c := cron.New()
	c.Start()

	return &SnapshotOperator{
		factory: f,
		cron:    c,
	}
}

func (o *SnapshotOperator) ListSnapshots(ctx context.Context, limit int64, labelSelector string, fieldSelector string) (*sysv1.SnapshotList, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	var listOptions = metav1.ListOptions{
		Limit: limit,
	}

	if labelSelector != "" {
		listOptions.LabelSelector = labelSelector
	}
	if fieldSelector != "" {
		listOptions.FieldSelector = fieldSelector
	}

	l, err := c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).List(ctx, listOptions)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if l == nil || l.Items == nil || len(l.Items) == 0 {
		return nil, fmt.Errorf("snapshots not exists")
	}

	sort.Slice(l.Items, func(i, j int) bool {
		return !l.Items[i].ObjectMeta.CreationTimestamp.Before(&l.Items[j].ObjectMeta.CreationTimestamp)
	})
	return l, nil
}

func (o *SnapshotOperator) CreateSnapshotSchedule(ctx context.Context, backup *sysv1.Backup, schedule string, paused bool) error {

	log.Infof("create snapshot schedule, name: %s, schedule: %s", backup.Name, schedule)

	entries := o.cron.Entries()
	for _, e := range entries {
		if e.Job.(backupJob).name == backup.Name {
			log.Info("remove prev cron job to apply new one")
			o.cron.Remove(e.ID)
		}
	}

	_, err := o.cron.AddJob(schedule, backupJob{
		name: backup.Name,
		f: func() {
			log.Infof("prepare to create backup task, name: %s", backup.Name)
			flag, err := o.getRunningSnapshot(ctx, backup.Spec.Id) // avoided running multiple snapshots simultaneously
			if err != nil {
				log.Errorf("get running snapshot error %v", err)
				return
			}
			if flag {
				log.Infof("snapshot is already running")
				return
			}

			flag, err = o.getFullySnapshot(ctx, backup.Spec.Id)
			if err != nil {
				log.Errorf("get complete snapshot error %v", err)
				return
			}
			var snapshotType = constant.FullyBackup
			if flag {
				snapshotType = constant.IncrementalBackup
			}

			var location string
			for k := range backup.Spec.Location {
				location = k
				break
			}

			log.Info("start to create backup snapshot task")

			_, err = o.CreateSnapshot(ctx, backup, snapshotType, location)
			if err != nil {
				log.Error("create backup task error, ", err)
			}
		},
	})

	if err != nil {
		log.Error("add backup schedule error, ", err)
	}

	return err
}

func (o *SnapshotOperator) CreateSnapshot(ctx context.Context, backup *sysv1.Backup, snapshotType string, location string) (*sysv1.Snapshot, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}
	var startAt = time.Now().UnixMilli()
	var name = fmt.Sprintf("%s-%d", backup.Name, startAt)
	var phase = constant.SnapshotPhasePending.String()
	var parseSnapshotType = o.parseSnapshotType(snapshotType)

	var snapshot = &sysv1.Snapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       constant.KindSnapshot,
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constant.DefaultOsSystemNamespace,
			Labels: map[string]string{
				"backup-id":     backup.Spec.Id,
				"snapshot-type": fmt.Sprintf("%d", *parseSnapshotType),
			},
		},
		Spec: sysv1.SnapshotSpec{
			Id:           utils.NewUUID(),
			BackupId:     backup.Spec.Id,
			Location:     location,
			SnapshotType: parseSnapshotType,
			StartAt:      startAt,
			Phase:        &phase,
			Extra:        map[string]string{},
		},
	}

	created, err := c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).Create(ctx, snapshot, metav1.CreateOptions{FieldManager: constant.SnapshotController})
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (o *SnapshotOperator) getRunningSnapshot(ctx context.Context, backupId string) (bool, error) {
	// check exists
	var labelSelector = fmt.Sprintf("backup-id=%s", backupId)
	snapshots, err := o.ListSnapshots(ctx, 1, labelSelector, "") // find all snapshots by backupName
	if err != nil {
		return false, err
	}

	for _, snapshot := range snapshots.Items {
		if util.ListContains([]string{constant.SnapshotPhaseRunning.String()}, *snapshot.Spec.Phase) {
			return true, nil
		}
	}

	return false, nil
}

func (o *SnapshotOperator) getFullySnapshot(ctx context.Context, backupId string) (bool, error) {
	var labelSelector = fmt.Sprintf("backup-id=%s", backupId)

	snapshots, err := o.ListSnapshots(ctx, 1, labelSelector, "") // find all snapshots by backupName
	if err != nil {
		return false, err
	}

	for _, snapshot := range snapshots.Items {
		if util.ListContains([]string{constant.SnapshotPhaseComplete.String()}, *snapshot.Spec.Phase) {
			return true, nil
		}
	}

	return false, nil
}

func (o *SnapshotOperator) parseSnapshotType(snapshotType string) *int {
	var r int
	if snapshotType == constant.FullyBackup {
		r = 0
	} else {
		r = 1
	}

	return &r
}

func (o *SnapshotOperator) ParseSnapshotName(startAt int64) string {
	t := time.UnixMilli(startAt)
	return t.Format("2006-01-02 15:04")
}

func (o *SnapshotOperator) Backup(backup *sysv1.Backup, snapshot *sysv1.Snapshot) error {
	var err error

	backupPath := o.getPath(backup)
	if backupPath == "" {
		err = fmt.Errorf("backup path is empty")
	}

	// password, err := o.getPassword(backup)
	// if err != nil {
	// 	return err
	// }
	password := "123"

	var olaresId string
	olaresId, err = o.getOlaresId(backup.Spec.Owner)

	var location string
	var locationConfig map[string]string
	for k, v := range backup.Spec.Location {
		location = k

		if e := json.Unmarshal([]byte(v), &locationConfig); e != nil {
			err = fmt.Errorf("unmarshal backup location error %v", err)
		}
	}

	if err != nil {
		err = o.UpdateSnapshot(backup.Name, nil, "", err, snapshot) // before backup
		return err
	}

	var opt options.Option
	switch location {
	case "space":
		opt = &options.SpaceBackupOptions{
			RepoName:       backup.Name,
			ClusterId:      locationConfig["clusterId"],
			CloudName:      locationConfig["cloudName"],
			RegionId:       locationConfig["regionId"],
			Path:           backupPath,
			CloudApiMirror: constant.DefaultSyncServerURL,
			Password:       password,
		}
	case "s3", "cos":
	}

	storage := storage.NewStorage(o.factory, backup.Spec.Owner, olaresId)
	backupOutput, backupRepo, err := storage.Backup(opt)
	if err != nil {
		log.Errorf("backup %s snapshot error %v", backup.Name, err)
	}

	// todo update
	log.Infof("update snapshot phase")
	err = o.UpdateSnapshot(backup.Name, backupOutput, backupRepo, err, snapshot)

	// todo notify

	if err != nil {
		return err
	}

	return nil
}

func (o *SnapshotOperator) SetSnapshotPhase(backupName string, snapshot *sysv1.Snapshot, phase constant.SnapshotPhase) error {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return err
	}

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    10,
	}

	if err = retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		var ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		s, err := c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).Get(ctx, snapshot.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("retry")
		}

		s.Spec.Phase = pointer.String(phase.String())
		_, err = c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).
			Update(ctx, s, metav1.UpdateOptions{})
		if err != nil && apierrors.IsConflict(err) {
			return fmt.Errorf("retry")
		} else if err != nil {
			log.Errorf("update backup %s snapshot %s phase error %v", backupName, s.Name, err)
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (o *SnapshotOperator) UpdateSnapshot(backupName string,
	backupOutput *backupssdkrestic.SummaryOutput,
	backupRepo string,
	err error,
	snapshot *sysv1.Snapshot) (e error) {

	c, err := o.factory.Sysv1Client()
	if err != nil {
		e = err
		return
	}

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    10,
	}

	retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		var ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		s, err := c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).Get(ctx, snapshot.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("retry")
		}

		if err != nil {
			s.Spec.Phase = pointer.String(constant.SnapshotPhaseFailed.String())
			s.Spec.Message = pointer.String(err.Error())
		} else {
			s.Spec.SnapshotId = pointer.String(backupOutput.SnapshotID)
			s.Spec.Phase = pointer.String(constant.SnapshotPhaseComplete.String())
			s.Spec.Size = pointer.UInt64Ptr(backupOutput.TotalBytesProcessed)
			s.Spec.ResticPhase = pointer.String(constant.SnapshotPhaseComplete.String())
			s.Spec.ResticMessage = pointer.String(util.ToJSON(backupOutput))
		}

		s.Spec.EndAt = time.Now().UnixMilli()

		_, err = c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).
			Update(ctx, s, metav1.UpdateOptions{})
		if err != nil && apierrors.IsConflict(err) {
			return fmt.Errorf("retry")
		} else if err != nil {
			// todo, there will remain a fully backup here
			log.Errorf("update backup %s snapshot phase error %v", backupName, err)
			e = err
			return nil
		}
		log.Infof("update backup: %s, snapshot: %s, complete success", backupName, s.Name)
		return nil
	})

	return e

}
