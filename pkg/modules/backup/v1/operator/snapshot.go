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
	"bytetrade.io/web3os/backups-sdk/pkg/utils"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			// todo ExistRunningBackup
			log.Infof("prepare to create backup task, name: %s", backup.Name)
			flag, err := o.getRunningSnapshot(ctx, backup.Spec.Owner, backup.Name)
			if err != nil {

			}
			if flag { // todo
				log.Infof("snapshot is already running")
				return
			}

			var backupType = constant.FullyBackup
			if _, ok := backup.Spec.Extra["fully"]; ok {
				backupType = constant.IncrementalBackup
			}

			var location string
			for k := range backup.Spec.Location {
				location = k
				break
			}

			log.Info("start to create backup task")

			_, err = o.CreateSnapshot(ctx, backup, backupType, location)
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

func (o *SnapshotOperator) CreateSnapshot(ctx context.Context, backup *sysv1.Backup, backupType string, location string) (*sysv1.Snapshot, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}
	var startAt = time.Now().UnixMilli()
	var name = fmt.Sprintf("%s-%d", backup.Name, startAt)
	var phase = "pending"
	var parseBackupType = o.parseBackupType(backupType)

	var snapshot = &sysv1.Snapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Snapshot",
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constant.DefaultOsSystemNamespace,
			Labels: map[string]string{
				"backup-name": backup.Name,
				"backup-type": fmt.Sprintf("%d", &parseBackupType),
			},
		},
		Spec: sysv1.SnapshotSpec{
			Id:         utils.NewUUID(),
			BackupId:   backup.Spec.Id,
			Location:   location,
			BackupType: parseBackupType,
			StartAt:    startAt,
			Phase:      &phase,
		},
	}

	created, err := c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).Create(ctx, snapshot, metav1.CreateOptions{FieldManager: "snapshot-controller"})
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (o *SnapshotOperator) getRunningSnapshot(ctx context.Context, owner string, backupName string) (bool, error) {
	// check exists
	var labelSelector = fmt.Sprintf("backup-name=%s", backupName)
	var fieldSelector = fmt.Sprintf("spec.owner=" + owner)
	snapshots, err := o.ListSnapshots(ctx, 1, labelSelector, fieldSelector) // find all snapshots by backupName
	if err != nil {
		return false, err
	}

	for _, snapshot := range snapshots.Items {
		if util.ListContains([]string{"Running"}, *snapshot.Spec.Phase) {
			return true, nil
		}
	}

	return false, nil
}

func (o *SnapshotOperator) parseBackupType(backupType string) *int {
	var r int
	if backupType == constant.FullyBackup {
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

func (o *SnapshotOperator) RunBackup(backup *sysv1.Backup, snapshot *sysv1.Snapshot) error {
	// password

	// password, err := o.getPassword(backup)
	// if err != nil {
	// 	return err
	// }
	password := "123"

	olaresId, err := o.getOlaresId(backup.Spec.Owner)
	if err != nil {
		return err
	}

	olaresDid, accessToken, err := o.getUserToken(backup.Spec.Owner, olaresId)
	if err != nil {
		return err
	}

	var location string
	var locationConfig map[string]string
	for k, v := range backup.Spec.Location {
		location = k
		if err := json.Unmarshal([]byte(v), &locationConfig); err != nil {
			return err
		}
	}

	var opt options.Option
	switch location {
	case "space":
		opt = &options.SpaceBackupOptions{
			OlaresDid:      olaresDid,
			AccessToken:    accessToken,
			ClusterId:      locationConfig["clusterId"],
			CloudName:      locationConfig["cloudName"],
			RegionId:       locationConfig["regionId"],
			Path:           "",
			CloudApiMirror: constant.DefaultCloudApiMirror,
			Password:       password,
		}
	case "s3", "cos":
	}

	var storage = storage.NewStorage(o.factory)
	storage.Backup(opt)

	return nil
}
