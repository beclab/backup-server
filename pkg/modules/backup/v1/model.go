package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	utilstring "bytetrade.io/web3os/backup-server/pkg/util/string"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"k8s.io/klog/v2"
)

type LocationConfig struct {
	CloudName string `json:"cloudName,omitempty"`
	RegionId  string `json:"regionId,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

type BackupCreate struct {
	Name string `json:"name"`

	Path string `json:"path"`

	Location string `json:"location"` // space or s3

	LocationConfig *LocationConfig `json:"locationConfig,omitempty"`

	BackupPolicies *sysv1.BackupPolicy `json:"backupPolicies,omitempty"`

	Password string `json:"password,omitempty"`

	ConfirmPassword string `json:"confirmPassword,omitempty"`
}

type Snapshot struct {
	Name string `json:"name,omitempty"`

	CreationTimestamp   int64  `json:"creationTimestamp,omitempty"`
	NextBackupTimestamp *int64 `json:"nextBackupTimestamp,omitempty"`

	Size *int64 `json:"size,omitempty"`

	Phase *string `json:"phase"`

	FailedMessage string `json:"failedMessage,omitempty"`
}

type ResponseBackupList struct {
	Id                string `json:"id"`
	Name              string `json:"name"`
	SnapshotFrequency string `json:"snapshotFrequency"`
	Location          string `json:"location"`
	SnapshotId        string `json:"snapshotId"`
	Status            string `json:"status"`
	Size              string `json:"size"`
	Path              string `json:"path"`
}

type ResponseBackupDetail struct {
	Id             string              `json:"id"`
	Name           string              `json:"name"`
	Path           string              `json:"path"`
	BackupPolicies *sysv1.BackupPolicy `json:"backupPolicies,omitempty"`
	Size           string              `json:"size"`
}

type ResponseSnapshotList struct {
	Id       string `json:"id"`
	CreateAt int64  `json:"createAt"`
	Size     string `json:"size"`
	Status   string `json:"status"`
}

type ResponseSnapshotDetail struct {
	Id           string `json:"id"`
	Size         string `json:"size"`
	SnapshotType string `json:"snapshotType"`
	Status       string `json:"status"`
	Message      string `json:"message"`
}

type SnapshotDetails struct {
	Name string `json:"name"`

	CreationTimestamp int64 `json:"creationTimestamp"`

	Size *int64 `json:"size"`

	Phase *string `json:"phase"`

	// Config *Config `json:"config,omitempty"`

	FailedMessage string `json:"failedMessage"`

	Owner *string `json:"owner"`

	BackupType string `json:"backupType"`

	SnapshotId string `json:"snapshotId"`

	RepositoryPasswordHash string `json:"repositoryPasswordHash"`

	BackupConfigName string `json:"backupConfigName"`
}

type ListBackupsDetails struct {
	Name string `json:"name"`

	Size *int64 `json:"size,omitempty"`

	SnapshotName string `json:"snapshotName"`

	SnapshotFrequency string `json:"snapshotFrequency"`

	CreationTimestamp int64 `json:"creationTimestamp,omitempty"`

	NextBackupTimestamp *int64 `json:"nextBackupTimestamp,omitempty"`

	Phase string `json:"phase,omitempty"`

	FailedMessage string `json:"failedMessage,omitempty"`
}

type ResponseDescribeBackup struct {
	Name string `json:"name"`

	Path string `json:"path"`

	Size *uint64 `json:"totalSize,omitempty"`

	BackupPolicies *sysv1.BackupPolicy `json:"backupPolicies"`

	Snapshots []Snapshot `json:"snapshots,omitempty"`

	// new list api fields
	SnapshotName string `json:"latestSnapshotName"`

	CreationTimestamp int64 `json:"creationTimestamp,omitempty"`

	NextBackupTimestamp *int64 `json:"nextBackupTimestamp,omitempty"`

	Phase string `json:"phase,omitempty"`

	FailedMessage string `json:"failedMessage,omitempty"`
}

type PostBackupSchedule struct {
	Schedule string `json:"schedule"`

	Paused bool `json:"paused"`
}

type SyncBackup struct {
	UID string `json:"uid"`

	CreationTimestamp int64 `json:"creationTimestamp"`

	Name string `json:"name"`

	Namespace string `json:"namespace,omitempty"`

	BackupConfigName string `json:"backupConfigName"`

	Size *int64 `json:"size"`

	// S3Config *Config `json:"s3Config"`

	Phase *string `json:"phase"`

	FailedMessage string `json:"failedMessage"`

	Owner *string `json:"owner"`

	TerminusVersion *string `json:"terminusVersion"`

	Expiration *int64 `json:"expiration"`

	CompletionTimestamp *int64 `json:"completionTimestamp"`

	BackupType string `json:"backupType"`

	SnapshotId string `json:"snapshotId"`

	S3Repository string `json:"s3Repository"`

	RepositoryPasswordHash string `json:"repositoryPasswordHash"`

	RefFullyBackupUid *string `json:"-"`

	RefFullyBackupName *string `json:"-"`
}

type SyncBackupList []*SyncBackup

func parseBackup(ctx context.Context, m velero.Manager, bc *sysv1.BackupConfig, sb *sysv1.Backup) (b *SyncBackup) {
	b = &SyncBackup{
		UID:               string(sb.UID),
		CreationTimestamp: sb.CreationTimestamp.Unix(),
		Name:              sb.Name,
		Namespace:         sb.Namespace,
		// Phase:             sb.Spec.ResticPhase,
		// Owner:             sb.Spec.Owner,
		// TerminusVersion:   sb.Spec.TerminusVersion,
		// Size:              sb.Spec.Size,
		BackupConfigName: bc.Name,
		// S3Config: &Config{
		// 	Provider: bc.Spec.Provider,
		// 	Region:   bc.Spec.Region,
		// 	Bucket:   bc.Spec.Bucket,
		// 	Prefix:   strings.Split(bc.Spec.Prefix, "/")[0],
		// 	S3Url:    bc.Spec.S3Url,
		// },
	}

	if sb.Spec.Extra != nil {
		extra := sb.Spec.Extra
		if backupType, ok := extra[velero.ExtraBackupType]; ok {
			b.BackupType = backupType
		}
		if snapshotId, ok := extra[velero.ExtraSnapshotId]; ok {
			b.SnapshotId = snapshotId
		}
		if repo, ok := extra[velero.ExtraS3Repository]; ok {
			b.S3Repository = repo
		}
		if hashSum, ok := extra[velero.ExtraRepositoryPasswordHash]; ok {
			b.RepositoryPasswordHash = hashSum
		}
		if refUid, ok := extra[velero.ExtraRefFullyBackupUid]; ok {
			b.RefFullyBackupUid = pointer.String(refUid)
		}
		if refName, ok := extra[velero.ExtraRefFullyBackupName]; ok {
			b.RefFullyBackupName = pointer.String(refName)
		}
	}

	// if sb.Spec.FailedMessage != nil && *sb.Spec.FailedMessage != "" {
	// 	b.FailedMessage = *sb.Spec.FailedMessage
	// 	return
	// }

	// vb, err := m.GetVeleroBackup(ctx, bc.Name)
	// if err == nil && vb != nil {
	// 	if vb.Status.Expiration != nil {
	// 		b.Expiration = pointer.Int64(vb.Status.Expiration.Unix())
	// 	}

	// 	if vb.Status.CompletionTimestamp != nil {
	// 		b.CompletionTimestamp = pointer.Int64(vb.Status.CompletionTimestamp.Unix())
	// 	}
	// }

	// var ok bool

	// ok, phase, err := m.BackupStatus(ctx, sb.Name)
	// if err != nil {
	// 	b.FailedMessage = err.Error()
	// }
	// if ok {
	// 	b.Phase = pointer.String(velero.Succeed)
	// } else if phase != "" {
	// 	b.Phase = pointer.String(phase)
	// }

	return
}

func parseResponseSnapshotList(snapshots *sysv1.SnapshotList) map[string]interface{} {
	var data = make(map[string]interface{})

	if snapshots == nil || len(snapshots.Items) == 0 {
		return data
	}

	data["totalPage"] = 0
	data["currentPage"] = 1
	var ss []*ResponseSnapshotList
	for _, snapshot := range snapshots.Items {
		var item = &ResponseSnapshotList{
			Id:       snapshot.Name,
			CreateAt: snapshot.Spec.StartAt,
			Size:     parseSnapshotSize(snapshot.Spec.Size),
			Status:   parseMessage(snapshot.Spec.Phase),
		}
		ss = append(ss, item)
	}

	data["snapshots"] = ss

	return data
}

func parseResponseSnapshotDetail(snapshot *sysv1.Snapshot) *ResponseSnapshotDetail {
	return &ResponseSnapshotDetail{
		Id:           snapshot.Name,
		Size:         parseSnapshotSize(snapshot.Spec.Size),
		SnapshotType: parseSnapshotType(snapshot.Spec.SnapshotType),
		Status:       parseMessage(snapshot.Spec.Phase),
		Message:      parseMessage(snapshot.Spec.Message),
	}
}

func parseResponseBackupDetail(backup *sysv1.Backup) *ResponseBackupDetail {
	return &ResponseBackupDetail{
		Id:             backup.Name,
		Name:           backup.Spec.Name,
		BackupPolicies: backup.Spec.BackupPolicy,
		Path:           parseBackupTypePath(backup.Spec.BackupType),
		Size:           parseSnapshotSize(backup.Spec.Size),
	}
}

func parseResponseBackupList(data *sysv1.BackupList, snapshots *sysv1.SnapshotList) []*ResponseBackupList {
	if data == nil || data.Items == nil || len(data.Items) == 0 {
		return nil
	}

	var bs = make(map[string]*sysv1.Snapshot)
	var res []*ResponseBackupList
	if snapshots != nil {
		for _, snapshot := range snapshots.Items {
			if _, ok := bs[snapshot.Spec.BackupId]; !ok {
				bs[snapshot.Spec.BackupId] = &snapshot
				continue
			}
		}
	}

	for _, backup := range data.Items {
		var r = &ResponseBackupList{
			Id:                backup.Name,
			Name:              backup.Spec.Name,
			SnapshotFrequency: parseBackupSnapshotFrequency(backup.Spec.BackupPolicy.SnapshotFrequency),
			Location:          parseLocationConfig(backup.Spec.Location),
			Path:              parseBackupTypePath(backup.Spec.BackupType),
		}

		if s, ok := bs[backup.Name]; ok {
			r.SnapshotId = s.Name
			r.Size = parseSnapshotSize(s.Spec.Size)
			r.Status = *s.Spec.Phase
		}

		res = append(res, r)
	}

	return res
}

func parseBackupSnapshotDetail(b *SyncBackup) *SnapshotDetails {
	return &SnapshotDetails{
		Name:                   b.Name,
		CreationTimestamp:      b.CreationTimestamp,
		Size:                   b.Size,
		Phase:                  b.Phase,
		FailedMessage:          b.FailedMessage,
		Owner:                  b.Owner,
		BackupType:             b.BackupType,
		SnapshotId:             b.SnapshotId,
		RepositoryPasswordHash: b.RepositoryPasswordHash,
		BackupConfigName:       b.BackupConfigName,
		//RefFullyBackupUid:            b.RefFullyBackupUid,
		//RefFullyBackupName:           b.RefFullyBackupName,
	}
}

func (s *SyncBackup) FormData() (map[string]string, error) {
	// s3config, err := json.Marshal(s.S3Config)
	// if err != nil {
	// 	klog.Error("parse s3 config error, ", err)
	// 	return nil, err
	// }

	formdata := make(map[string]string)
	formdata["backupConfigName"] = s.BackupConfigName
	formdata["completionTimestamp"] = toString(s.CompletionTimestamp)
	formdata["creationTimestamp"] = toString(s.CreationTimestamp)
	formdata["expiration"] = toString(s.Expiration)
	formdata["name"] = toString(s.Name)
	formdata["phase"] = toString(s.Phase)
	formdata["uid"] = toString(s.UID)
	formdata["size"] = toString(s.Size)
	// formdata["s3Config"] = string(s3config)
	formdata["terminusVersion"] = toString(s.TerminusVersion)
	formdata["owner"] = toString(s.Owner)
	formdata["backupType"] = toString(s.BackupType)
	formdata["s3Repository"] = toString(s.S3Repository)
	formdata["snapshotId"] = toString(s.SnapshotId)
	return formdata, nil
}

func toString(v interface{}) string {
	int64ToStr := func(n int64) string {
		s := strconv.FormatInt(n, 10)
		return s
	}

	switch s := v.(type) {
	case int64:
		return int64ToStr(s)
	case *int64:
		if s == nil {
			return ""
		}
		return int64ToStr(*s)
	case *string:
		if s == nil {
			return ""
		}
		return *s
	case string:
		return s
	}

	klog.Error("unknown field type")
	return ""
}

func parseBackupTypePath(backupType map[string]string) string {
	if backupType == nil {
		return ""
	}
	var backupTypeValue map[string]string
	for k, v := range backupType {
		if k != "file" {
			continue
		}
		if err := json.Unmarshal([]byte(v), &backupTypeValue); err != nil {
			return ""
		}
		return backupTypeValue["path"]
	}
	return ""
}

func parseLocationConfig(locationConfig map[string]string) string {
	var location string
	if locationConfig == nil {
		return parseBackupLocation(location)
	}

	for l, _ := range locationConfig {
		location = l
	}
	return parseBackupLocation(location)
}

func parseSnapshotSize(size *uint64) string {
	if size == nil {
		return ""
	}

	return fmt.Sprintf("%d", *size)
}

func parseSnapshotType(snapshotType *int) string {
	var t = constant.UnKnownBackup

	if snapshotType == nil || (*snapshotType < 0 || *snapshotType > 1) {
		return utilstring.Title(t)
	}
	if *snapshotType == 0 {
		t = constant.FullyBackup
	} else {
		t = constant.IncrementalBackup
	}

	return utilstring.Title(t)
}

func parseMessage(msg *string) string {
	if msg == nil {
		return ""
	}
	return *msg
}

func parseBackupSnapshotFrequency(str string) string {
	str = strings.ReplaceAll(str, "@", "")
	return utilstring.Title(str)
}

func parseBackupLocation(l string) string {
	switch l {
	case constant.BackupLocationSpace.String():
		return constant.BackupLocationSpaceAlias.String()
	case constant.BackupLocationAws.String():
		return constant.BackupLocationAwsAlias.String()
	case constant.BackupLocationTencentCloud.String():
		return constant.BackupLocationCosAlias.String()
	case constant.BackupLocationFileSystem.String():
		return constant.BackupLocationFileSystemAlias.String()
	default:
		return constant.BackupLocationUnKnownAlias.String()
	}
}
