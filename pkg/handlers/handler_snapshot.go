package handlers

import (
	"context"
	"fmt"
	"sort"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/converter"
	"bytetrade.io/web3os/backup-server/pkg/options"
	"bytetrade.io/web3os/backup-server/pkg/storage"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	"bytetrade.io/web3os/backup-server/pkg/util/uuid"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

type SnapshotHandler struct {
	factory client.Factory
	cron    *cron.Cron
}

type backupJob struct {
	name string
	f    func()
}

func (b backupJob) Run() { b.f() }

func NewSnapshotHandler(f client.Factory) *SnapshotHandler {
	c := cron.New()
	c.Start()

	return &SnapshotHandler{
		factory: f,
		cron:    c,
	}
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

func (o *SnapshotHandler) CreateSnapshotSchedule(ctx context.Context, backup *sysv1.Backup, schedule string, paused bool) error {
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

func (o *SnapshotHandler) CreateSnapshot(ctx context.Context, backup *sysv1.Backup, location string) (*sysv1.Snapshot, error) {
	c, err := o.factory.Sysv1Client()
	if err != nil {
		return nil, err
	}
	var startAt = time.Now().UnixMilli()
	var name = uuid.NewUUID()
	var phase = constant.Pending.String()
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

func (o *SnapshotHandler) GetSnapshot(ctx context.Context, id string) (*sysv1.Snapshot, error) {
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

func (o *SnapshotHandler) getRunningSnapshot(ctx context.Context, backupId string) (bool, error) {
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

func (o *SnapshotHandler) ParseSnapshotName(startAt int64) string {
	t := time.UnixMilli(startAt)
	return t.Format("2006-01-02 15:04")
}

func (o *SnapshotHandler) SetSnapshotPhase(backupName string, snapshot *sysv1.Snapshot, phase constant.Phase) error {
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

func (o *SnapshotHandler) Update(ctx context.Context, snapshotId string, snapshotSpec *sysv1.SnapshotSpec) error {
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
		log.Warnf("update snapshot %s spec retry", snapshotId)
		goto RETRY
	} else if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (o *SnapshotHandler) updateSnapshotFailedStatus(backupError error, snapshot *sysv1.Snapshot) error {
	snapshot.Spec.Phase = pointer.String(constant.Failed.String())
	snapshot.Spec.Message = pointer.String(backupError.Error())
	snapshot.Spec.EndAt = time.Now().UnixMilli()

	return o.Update(context.Background(), snapshot.Name, &snapshot.Spec) // update failed
}

func (o *SnapshotHandler) updateSnapshotFinishedStatus(backupOutput *backupssdkrestic.SummaryOutput,
	backupRepo string, backupError error,
	snapshot *sysv1.Snapshot) error {
	// todo backupRepo
	_ = backupRepo

	if backupError != nil {
		snapshot.Spec.Phase = pointer.String(constant.Failed.String())
		snapshot.Spec.Message = pointer.String(backupError.Error())
		snapshot.Spec.ResticPhase = pointer.String(constant.Failed.String())
	} else {
		snapshot.Spec.Size = pointer.UInt64Ptr(backupOutput.TotalBytesProcessed)
		snapshot.Spec.Phase = pointer.String(constant.Completed.String())
		snapshot.Spec.ResticPhase = pointer.String(constant.Completed.String())
		snapshot.Spec.ResticMessage = pointer.String(util.ToJSON(backupOutput))
	}

	snapshot.Spec.SnapshotId = pointer.String(backupOutput.SnapshotID)
	snapshot.Spec.EndAt = time.Now().UnixMilli()

	return o.Update(context.Background(), snapshot.Name, &snapshot.Spec) // update finished
}

func (o *SnapshotHandler) getPassword(backup *sysv1.Backup) (string, error) {
	// todo
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

func (o *SnapshotHandler) validateBackupPreconditions(backup *sysv1.Backup, snapshot *sysv1.Snapshot) error {
	_ = backup
	if *snapshot.Spec.Phase != constant.Pending.String() { // other phase ?
		return fmt.Errorf("phase %s is not Pending", *snapshot.Spec.Phase)
	}
	return nil
}

func (o *SnapshotHandler) prepareBackupParams(ctx context.Context, backup *sysv1.Backup) (*BackupParams, error) {
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

func (o *SnapshotHandler) updateSnapshotToRunning(ctx context.Context, snapshot *sysv1.Snapshot, params *BackupParams) error {
	snapshot.Spec.Phase = pointer.String(constant.Running.String())
	snapshot.Spec.SnapshotType = parseSnapshotType(params.SnapshotType)
	return o.Update(ctx, snapshot.Name, &snapshot.Spec) // update running
}

func (o *SnapshotHandler) createBackupOption(backupName string, params *BackupParams) options.Option {
	switch params.Location {
	case constant.BackupLocationSpace.String():
		return &options.SpaceBackupOptions{
			RepoName:       backupName,
			Location:       params.Location,
			OlaresId:       params.LocationConfig["name"],
			ClusterId:      params.LocationConfig["clusterId"],
			CloudName:      params.LocationConfig["cloudName"],
			RegionId:       params.LocationConfig["regionId"],
			Path:           params.Path,
			CloudApiMirror: constant.DefaultSyncServerURL,
			Password:       params.Password,
		}
	case constant.BackupLocationAwsS3.String():
		return &options.AwsS3BackupOptions{
			RepoName:           backupName,
			Location:           params.Location,
			LocationConfigName: params.LocationConfig["name"],
			Path:               params.Path,
			Password:           params.Password,
		}
	case constant.BackupLocationTencentCloud.String():
		return &options.TencentCloudBackupOptions{
			RepoName:           backupName,
			Location:           params.Location,
			LocationConfigName: params.LocationConfig["name"],
			Path:               params.Path,
			Password:           params.Password,
		}
	}
	return nil
}

func (o *SnapshotHandler) executeBackup(ctx context.Context,
	backup *sysv1.Backup,
	snapshot *sysv1.Snapshot,
	opt options.Option, params *BackupParams) error {
	_ = params
	storage := storage.NewStorage(o.factory, backup.Spec.Owner)
	backupOutput, backupRepo, backupErr := storage.Backup(ctx, opt) // long time

	if backupErr != nil {
		log.Errorf("backup %s snapshot %s error: %v", backup.Spec.Name, snapshot.Name, backupErr)
	} else {
		log.Infof("backup %s snapshot %s completed, data: %+v", backup.Spec.Name, snapshot.Name, backupOutput)
	}

	err := o.updateSnapshotFinishedStatus(backupOutput, backupRepo, backupErr, snapshot)

	// todo notify

	return err
}

func (o *SnapshotHandler) Backup(ctx context.Context, backup *sysv1.Backup, snapshot *sysv1.Snapshot) error {
	var err error

	if err = o.validateBackupPreconditions(backup, snapshot); err != nil {
		return errors.WithMessage(err, o.updateSnapshotFailedStatus(err, snapshot).Error())
	}

	params, err := o.prepareBackupParams(ctx, backup)
	if err != nil {
		return errors.WithMessage(err, o.updateSnapshotFailedStatus(err, snapshot).Error())
	}

	if err := o.updateSnapshotToRunning(ctx, snapshot, params); err != nil {
		return errors.WithMessage(err, o.updateSnapshotFailedStatus(err, snapshot).Error())
	}

	opt := o.createBackupOption(backup.Name, params)

	return o.executeBackup(ctx, backup, snapshot, opt, params)

}

// --

type ProxyRequest struct {
	Op       string      `json:"op"`
	DataType string      `json:"datatype"`
	Version  string      `json:"version"`
	Group    string      `json:"group"`
	Param    interface{} `json:"param,omitempty"`
	Data     string      `json:"data,omitempty"`
	Token    string
}

type AccountValue struct {
	Email   string `json:"email"`
	Userid  string `json:"userid"`
	Token   string `json:"token"`
	Expired any    `json:"expired"`
}

type PasswordResponse struct {
	response.Header
	Data *PasswordResponseData `json:"data,omitempty"`
}

type PasswordResponseData struct {
	Env   string `json:"env"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// func (o *SnapshotOperator) getPassword(backup *sysv1.Backup) (string, error) {
// 	settingsUrl := fmt.Sprintf("http://settings-service.user-space-%s/api/backup/password", backup.Spec.Owner)
// 	client := resty.New().SetTimeout(2 * time.Second).SetDebug(true)

// 	req := &ProxyRequest{
// 		Op:       "getAccount",
// 		DataType: "backupPassword",
// 		Version:  "v1",
// 		Group:    "service.settings",
// 		Data:     backup.Name,
// 	}

// 	terminusNonce, err := util.GenTerminusNonce("")
// 	if err != nil {
// 		log.Error("generate nonce error, ", err)
// 		return "", err
// 	}

// 	log.Info("fetch password from settings, ", settingsUrl)
// 	resp, err := client.R().
// 		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
// 		SetHeader("Terminus-Nonce", terminusNonce).
// 		SetBody(req).
// 		SetResult(&PasswordResponse{}).
// 		Post(settingsUrl)

// 	if err != nil {
// 		log.Error("request settings password api error, ", err)
// 		return "", err
// 	}

// 	if resp.StatusCode() != http.StatusOK {
// 		log.Error("request settings password api response not ok, ", resp.StatusCode())
// 		err = errors.New(string(resp.Body()))
// 		return "", err
// 	}

// 	pwdResp := resp.Result().(*PasswordResponse)
// 	log.Infof("settings password api response, %+v", pwdResp)
// 	if pwdResp.Code != 0 {
// 		log.Error("request settings password api response error, ", pwdResp.Code, ", ", pwdResp.Message)
// 		err = errors.New(pwdResp.Message)
// 		return "", err
// 	}

// 	if pwdResp.Data == nil {
// 		log.Error("request settings password api response data is nil, ", pwdResp.Code, ", ", pwdResp.Message)
// 		err = errors.New("request settings password api response data is nil")
// 		return "", err
// 	}

// 	return pwdResp.Data.Value, nil
// }

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
