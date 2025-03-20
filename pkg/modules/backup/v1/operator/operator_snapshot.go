package operator

import (
	"context"
	"fmt"
	"sort"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/converter"
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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
		return nil, nil
	}

	sort.Slice(l.Items, func(i, j int) bool {
		return !l.Items[i].ObjectMeta.CreationTimestamp.Before(&l.Items[j].ObjectMeta.CreationTimestamp)
	})
	return l, nil
}

func (o *SnapshotOperator) CreateSnapshotSchedule(ctx context.Context, backup *sysv1.Backup, schedule string, paused bool) error {
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

			_, err := o.CreateSnapshot(ctx, backup, location)
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

func (o *SnapshotOperator) CreateSnapshot(ctx context.Context, backup *sysv1.Backup, location string) (*sysv1.Snapshot, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}
	var startAt = time.Now().UnixMilli()
	var name = utils.NewUUID()
	var phase = constant.SnapshotPhasePending.String()
	var parseSnapshotType = parseSnapshotType(constant.UnKnownBackup)

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

func (o *SnapshotOperator) GetSnapshot(ctx context.Context, id string) (*sysv1.Snapshot, error) {
	var ctxTimeout, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}

	snapshot, err := c.SysV1().Snapshots(constant.DefaultOsSystemNamespace).Get(ctxTimeout, id, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	if snapshot == nil {
		return nil, nil
	}

	return snapshot, nil
}

func (o *SnapshotOperator) getRunningSnapshot(ctx context.Context, backupId string) (bool, error) {
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
		if util.ListContains([]string{constant.SnapshotPhaseRunning.String()}, *snapshot.Spec.Phase) {
			return true, nil
		}
	}

	return false, nil
}

func (o *SnapshotOperator) GetSnapshotType(ctx context.Context, backupId string) (snapshotType string, err error) {
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
		if util.ListContains([]string{constant.SnapshotPhaseComplete.String()}, *snapshot.Spec.Phase) {
			snapshotType = constant.IncrementalBackup
			return
		}
	}

	return
}

func (o *SnapshotOperator) ParseSnapshotName(startAt int64) string {
	t := time.UnixMilli(startAt)
	return t.Format("2006-01-02 15:04")
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

func (o *SnapshotOperator) Update(ctx context.Context, snapshotId string, snapshotSpec *sysv1.SnapshotSpec) error {
	s, err := o.GetSnapshot(ctx, snapshotId)
	if err != nil {
		return err
	}

RETRY:
	var snapshot = &sysv1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: constant.DefaultOsSystemNamespace,
			Labels:    s.Labels,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Snapshot",
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		Spec: *snapshotSpec,
	}

	obj, err := converter.ToUnstructured(snapshot)
	if err != nil {
		return err
	}

	res := unstructured.Unstructured{Object: obj}
	res.SetGroupVersionKind(snapshot.GroupVersionKind())

	dynamicClient, err := o.factory.DynamicClient()
	if err != nil {
		return err
	}

	_, err = dynamicClient.Resource(constant.SnapshotGVR).Namespace(constant.DefaultOsSystemNamespace).Update(ctx, &res, metav1.UpdateOptions{FieldManager: constant.SnapshotController})

	if err != nil && apierrors.IsConflict(err) {
		goto RETRY
	} else if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (o *SnapshotOperator) updateSnapshotFailedStatus(backupError error, snapshot *sysv1.Snapshot) error {
	snapshot.Spec.Phase = pointer.String(constant.SnapshotPhaseFailed.String())
	snapshot.Spec.Message = pointer.String(backupError.Error())
	snapshot.Spec.EndAt = time.Now().UnixMilli()

	return o.Update(context.Background(), snapshot.Name, &snapshot.Spec)
}

func (o *SnapshotOperator) updateSnapshotCompletedStatus(backupOutput *backupssdkrestic.SummaryOutput,
	backupRepo string,
	snapshot *sysv1.Snapshot) error {
	snapshot.Spec.SnapshotId = pointer.String(backupOutput.SnapshotID)
	snapshot.Spec.Phase = pointer.String(constant.SnapshotPhaseComplete.String())
	snapshot.Spec.Size = pointer.UInt64Ptr(backupOutput.TotalBytesProcessed)
	snapshot.Spec.ResticPhase = pointer.String(constant.SnapshotPhaseComplete.String())
	snapshot.Spec.ResticMessage = pointer.String(util.ToJSON(backupOutput))
	snapshot.Spec.EndAt = time.Now().UnixMilli()

	return o.Update(context.Background(), snapshot.Name, &snapshot.Spec)
}

func (o *SnapshotOperator) getPassword(backup *sysv1.Backup) (string, error) {
	if backup.Spec.Extra == nil {
		return "", fmt.Errorf("backup extra not exists")
	}

	p, ok := backup.Spec.Extra["password"]
	if !ok {
		return "", fmt.Errorf("backup extra key not exists")
	}

	key, err := util.Base64decode(p)
	if err != nil {
		return "", fmt.Errorf("base64 decode extra key error %v", err)
	}

	return string(key), nil
}

// backup
type BackupParams struct {
	Path           string
	Password       string
	Location       string
	LocationConfig map[string]string
	SnapshotType   string
}

func (o *SnapshotOperator) validateBackupPreconditions(backup *sysv1.Backup, snapshot *sysv1.Snapshot) error {
	if *snapshot.Spec.Phase != constant.SnapshotPhasePending.String() { // other phase ?
		return fmt.Errorf("backup %s snapshot is not pending, phase: %s", backup.Spec.Name, *snapshot.Spec.Phase)
	}
	return nil
}

func (o *SnapshotOperator) prepareBackupParams(ctx context.Context, backup *sysv1.Backup) (*BackupParams, error) {
	var err error
	params := &BackupParams{
		Path:           getBackupPath(backup),
		LocationConfig: make(map[string]string),
	}

	params.Password, err = o.getPassword(backup)
	if err != nil {
		return nil, err
	}

	params.Location, params.LocationConfig, err = getBackupLocationConfig(backup)
	if err != nil {
		return nil, err
	}

	snapshotType, err := o.GetSnapshotType(ctx, backup.Name)
	if err != nil {
		return nil, fmt.Errorf("get snapshot type error: %v", err)
	}
	params.SnapshotType = snapshotType

	return params, nil
}

func (o *SnapshotOperator) updateSnapshotToRunning(ctx context.Context, snapshot *sysv1.Snapshot, params *BackupParams) error {
	snapshot.Spec.Phase = pointer.String(constant.SnapshotPhaseRunning.String())
	snapshot.Spec.SnapshotType = parseSnapshotType(params.SnapshotType)
	return o.Update(ctx, snapshot.Name, &snapshot.Spec)
}

func (o *SnapshotOperator) createBackupOption(backupName string, params *BackupParams) options.Option {
	switch params.Location {
	case constant.BackupLocationSpace.String():
		return &options.SpaceBackupOptions{
			RepoName:       backupName,
			OlaresId:       params.LocationConfig["name"],
			ClusterId:      params.LocationConfig["clusterId"],
			CloudName:      params.LocationConfig["cloudName"],
			RegionId:       params.LocationConfig["regionId"],
			Path:           params.Path,
			CloudApiMirror: constant.DefaultSyncServerURL,
			Password:       params.Password,
		}
	case constant.BackupLocationAws.String():
		return &options.AwsBackupOptions{
			RepoName:           backupName,
			LocationConfigName: params.LocationConfig["name"],
			Path:               params.Path,
			Password:           params.Password,
		}
	case constant.BackupLocationTencentCloud.String():
		return &options.TencentCloudBackupOptions{
			RepoName:           backupName,
			LocationConfigName: params.LocationConfig["name"],
			Path:               params.Path,
			Password:           params.Password,
		}
	}
	return nil
}

func (o *SnapshotOperator) executeBackup(ctx context.Context, backup *sysv1.Backup, snapshot *sysv1.Snapshot, opt options.Option, params *BackupParams) error {
	storage := storage.NewStorage(o.factory, backup.Spec.Owner)
	backupOutput, backupRepo, backupErr := storage.Backup(opt)

	if backupErr != nil {
		log.Errorf("backup %s snapshot %s error: %v", backup.Spec.Name, snapshot.Name, backupErr)
	} else {
		log.Infof("backup %s snapshot %s completed, data: %+v", backup.Spec.Name, snapshot.Name, backupOutput)
	}

	err := o.updateSnapshotCompletedStatus(backupOutput, backupRepo, snapshot)

	// todo notify

	return err
}

func (o *SnapshotOperator) Backup(ctx context.Context, backup *sysv1.Backup, snapshot *sysv1.Snapshot) error {
	var err error

	if err = o.validateBackupPreconditions(backup, snapshot); err != nil {
		return o.updateSnapshotFailedStatus(err, snapshot)
	}

	params, err := o.prepareBackupParams(ctx, backup)
	if err != nil {
		return o.updateSnapshotFailedStatus(err, snapshot)
	}

	if err := o.updateSnapshotToRunning(ctx, snapshot, params); err != nil {
		return o.updateSnapshotFailedStatus(err, snapshot)
	}

	opt := o.createBackupOption(backup.Name, params)

	return o.executeBackup(ctx, backup, snapshot, opt, params)

}
