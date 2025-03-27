package handlers

import (
	"context"
	"fmt"
	"sort"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	"bytetrade.io/web3os/backup-server/pkg/util/uuid"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

type SnapshotHandler struct {
	factory  client.Factory
	cron     *cron.Cron
	handlers Interface
}

type backupJob struct {
	name string
	f    func()
}

func (b backupJob) Run() { b.f() }

func NewSnapshotHandler(f client.Factory, handlers Interface) *SnapshotHandler {
	c := cron.New()
	c.Start()

	return &SnapshotHandler{
		factory:  f,
		cron:     c,
		handlers: handlers,
	}
}

func (o *SnapshotHandler) UpdatePhase(ctx context.Context, snapshotId string, phase string) error {
	snapshot, err := o.GetById(ctx, snapshotId)
	if err != nil || !apierrors.IsNotFound(err) {
		return err
	}

	snapshot.Spec.Phase = pointer.String(phase)

	return o.update(ctx, snapshot) // updatePhase
}

func (o *SnapshotHandler) ListSnapshots(ctx context.Context, limit int64, labelSelector string, fieldSelector string) (*sysv1.SnapshotList, error) {
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
		return nil, nil
	}

	sort.Slice(l.Items, func(i, j int) bool {
		return !l.Items[i].ObjectMeta.CreationTimestamp.Before(&l.Items[j].ObjectMeta.CreationTimestamp)
	})
	return l, nil
}

func (o *SnapshotHandler) CreateSchedule(ctx context.Context, backup *sysv1.Backup, schedule string, paused bool) error {
	log.Infof("create snapshot schedule, name: %s, frequency: %s, schedule: %s", backup.Spec.Name, backup.Spec.BackupPolicy.SnapshotFrequency, schedule)

	entries := o.cron.Entries()
	for _, e := range entries {
		if e.Job.(backupJob).name == backup.Spec.Name {
			log.Info("remove prev cron job to apply new one")
			o.cron.Remove(e.ID)
		}
	}

	_, err := o.cron.AddJob(schedule, backupJob{
		name: backup.Spec.Name,
		f: func() {
			log.Infof("prepare to create snapshot task, name: %s", backup.Spec.Name)

			var location string
			for k := range backup.Spec.Location {
				location = k
				break
			}

			_, err := o.Create(ctx, backup, location)
			if err != nil {
				log.Error("create snapshot task error, ", err)
			}
		},
	})

	if err != nil {
		log.Error("add snapshot schedule error, ", err)
	}

	return err
}

func (o *SnapshotHandler) Create(ctx context.Context, backup *sysv1.Backup, location string) (*sysv1.Snapshot, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}
	var startAt = time.Now().UnixMilli()
	var name = uuid.NewUUID()
	var phase = constant.Pending.String()
	var parseSnapshotType = ParseSnapshotType(constant.UnKnownBackup)

	var snapshot = &sysv1.Snapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       constant.KindSnapshot,
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constant.DefaultOsSystemNamespace,
			Labels: map[string]string{
				"backup-id":     backup.Name,
				"snapshot-type": fmt.Sprintf("%d", *parseSnapshotType),
			},
		},
		Spec: sysv1.SnapshotSpec{
			BackupId:     backup.Name,
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

func (o *SnapshotHandler) GetById(ctx context.Context, snapshotId string) (*sysv1.Snapshot, error) {
	var getCtx, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	snapshot, err := c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).Get(getCtx, snapshotId, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	if snapshot == nil {
		return nil, apierrors.NewNotFound(sysv1.Resource("Snapshot"), snapshotId)
	}

	return snapshot, nil
}

func (o *SnapshotHandler) GetRunningSnapshot(ctx context.Context, backupId string) (bool, error) {
	// check exists
	var labelSelector = fmt.Sprintf("backup-id=%s", backupId)
	snapshots, err := o.ListSnapshots(ctx, 1, labelSelector, "") // find all snapshots by backupName
	if err != nil {
		return false, err
	}

	if snapshots == nil || len(snapshots.Items) == 0 {
		return false, nil
	}

	for _, snapshot := range snapshots.Items {
		if util.ListContains([]string{constant.Running.String()}, *snapshot.Spec.Phase) {
			return true, nil
		}
	}

	return false, nil
}

func (o *SnapshotHandler) GetSnapshotType(ctx context.Context, backupId string) (snapshotType string, err error) {
	var snapshots *sysv1.SnapshotList
	var labelSelector = fmt.Sprintf("backup-id=%s", backupId)
	snapshotType = constant.FullyBackup

	snapshots, err = o.ListSnapshots(ctx, 0, labelSelector, "") // find all snapshots by backupName
	if err != nil {
		return
	}

	if snapshots == nil || len(snapshots.Items) == 0 {
		return
	}

	for _, snapshot := range snapshots.Items {
		if util.ListContains([]string{constant.Completed.String()}, *snapshot.Spec.Phase) {
			snapshotType = constant.IncrementalBackup
			return
		}
	}

	return
}

func (o *SnapshotHandler) UpdateBackupResult(ctx context.Context, snapshot *sysv1.Snapshot) error {
	return o.update(ctx, snapshot)
}

func (o *SnapshotHandler) update(ctx context.Context, snapshot *sysv1.Snapshot) error {
	sc, err := o.factory.Sysv1Client()
	if err != nil {
		return err
	}

	var getCtx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

RETRY:
	_, err = sc.SysV1().Snapshots(constant.DefaultOsSystemNamespace).Update(getCtx, snapshot, metav1.UpdateOptions{
		FieldManager: constant.SnapshotController,
	})

	if err != nil && apierrors.IsConflict(err) {
		log.Warnf("update snapshot %s spec retry", snapshot.Name)
		goto RETRY
	} else if err != nil {
		return errors.WithStack(fmt.Errorf("update snapshot error: %v", err))
	}

	return nil
}

// --

func (o *SnapshotHandler) GetOlaresId(owner string) (string, error) {
	dynamicClient, err := o.factory.DynamicClient()
	if err != nil {
		return "", errors.WithStack(fmt.Errorf("get dynamic client error %v", err))
	}

	var backoff = wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    5,
	}

	var olaresName string
	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		unstructuredUser, err := dynamicClient.Resource(constant.UsersGVR).Get(ctx, owner, metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(fmt.Errorf("get user error %v", err))
		}
		obj := unstructuredUser.UnstructuredContent()
		olaresName, _, err = unstructured.NestedString(obj, "spec", "email")
		if err != nil {
			return errors.WithStack(fmt.Errorf("get user nested string error %v", err))
		}
		return nil
	}); err != nil {
		return "", errors.WithStack(err)
	}

	return olaresName, nil
}
