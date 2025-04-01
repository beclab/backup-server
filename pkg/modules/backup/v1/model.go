package v1

import (
	"strconv"
	"strings"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	"k8s.io/klog/v2"
)

type LocationConfig struct {
	Name      string `json:"name"`                // olaresId or integrationCloudAccessKey
	CloudName string `json:"cloudName,omitempty"` // olares space cloudName
	RegionId  string `json:"regionId,omitempty"`  // olares space regionId
	Path      string `json:"path"`                // filesystem target path
}

type BackupCreate struct {
	Name            string              `json:"name"`
	Path            string              `json:"path"`
	Location        string              `json:"location"` // space or s3
	LocationConfig  *LocationConfig     `json:"locationConfig,omitempty"`
	BackupPolicies  *sysv1.BackupPolicy `json:"backupPolicies,omitempty"`
	Password        string              `json:"password,omitempty"`
	ConfirmPassword string              `json:"confirmPassword,omitempty"`
}

type BackupEnabled struct {
	Event string `json:"event"`
}

type Snapshot struct {
	Name string `json:"name,omitempty"`

	CreationTimestamp   int64  `json:"creationTimestamp,omitempty"`
	NextBackupTimestamp *int64 `json:"nextBackupTimestamp,omitempty"`

	Size *int64 `json:"size,omitempty"`

	Phase *string `json:"phase"`

	FailedMessage string `json:"failedMessage,omitempty"`
}

type SnapshotCancel struct {
	Event string `json:"event"`
}

type CreateSnapshot struct {
	Event string `json:"event"`
}

type RestoreCreate struct {
	BackupUrl  string `json:"backupUrl"`
	Password   string `json:"password"`
	SnapshotId string `json:"snapshotId"`
	Path       string `json:"path"`
}

func (r *RestoreCreate) verify() bool {
	var path = strings.TrimSpace(r.Path)
	var backupUrl = strings.TrimSpace(r.BackupUrl)
	var snapshotId = strings.TrimSpace(r.SnapshotId)
	var password = strings.TrimSpace(r.Password)

	if path == "" {
		return false
	}

	if (backupUrl == "") == (snapshotId == "") {
		return false
	}

	if backupUrl != "" && password == "" {
		return false
	}

	return true
}

type RestoreCancel struct {
	Event string `json:"event"`
}

type ResponseBackupList struct {
	Id                  string `json:"id"`
	Name                string `json:"name"`
	SnapshotFrequency   string `json:"snapshotFrequency"`
	Location            string `json:"location"`           // space, awss3, tencentcloud ...
	LocationConfigName  string `json:"locationConfigName"` // olaresDid / cloudAccessKey
	SnapshotId          string `json:"snapshotId"`
	NextBackupTimestamp *int64 `json:"nextBackupTimestamp,omitempty"`
	Status              string `json:"status"`
	Size                string `json:"size"`
	Path                string `json:"path"`
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

type ResponseRestoreDetail struct {
	BackupName   string  `json:"backupName"`
	BackupPath   string  `json:"backupPath"`
	SnapshotName string  `json:"snapshotName"`
	RestorePath  string  `json:"restorePath"`
	Status       string  `json:"status"`
	Message      string  `json:"message"`
	Progress     float64 `json:"progress,omitempty"`
}

type ResponseRestoreList struct {
	Id         string  `json:"id"`
	BackupName string  `json:"backupName"`
	Path       string  `json:"path"`
	CreateAt   int64   `json:"createAt"`
	EndAt      int64   `json:"endAt"`
	Status     string  `json:"status"`
	Progress   float64 `json:"progress,omitempty"`
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
			Size:     handlers.ParseSnapshotSize(snapshot.Spec.Size),
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
		Size:         handlers.ParseSnapshotSize(snapshot.Spec.Size),
		SnapshotType: handlers.ParseSnapshotTypeTitle(snapshot.Spec.SnapshotType),
		Status:       parseMessage(snapshot.Spec.Phase),
		Message:      parseMessage(snapshot.Spec.Message),
	}
}

func parseResponseBackupDetail(backup *sysv1.Backup) *ResponseBackupDetail {
	return &ResponseBackupDetail{
		Id:             backup.Name,
		Name:           backup.Spec.Name,
		BackupPolicies: backup.Spec.BackupPolicy,
		Path:           handlers.ParseBackupTypePath(backup.Spec.BackupType),
		Size:           handlers.ParseSnapshotSize(backup.Spec.Size),
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
		locationConfig, err := handlers.GetBackupLocationConfig(&backup)
		if err != nil || locationConfig == nil {
			continue
		}
		if locationConfig["location"] == "filesystem" { // TODO
			continue
		}
		location := locationConfig["location"]
		locationConfigName := locationConfig["name"]
		var r = &ResponseBackupList{
			Id:                  backup.Name,
			Name:                backup.Spec.Name,
			SnapshotFrequency:   handlers.ParseBackupSnapshotFrequency(backup.Spec.BackupPolicy.SnapshotFrequency),
			NextBackupTimestamp: handlers.GetNextBackupTime(*backup.Spec.BackupPolicy),
			Location:            location,
			LocationConfigName:  locationConfigName, // maybe is empty
			Path:                handlers.ParseBackupTypePath(backup.Spec.BackupType),
		}

		if s, ok := bs[backup.Name]; ok {
			r.SnapshotId = s.Name
			r.Size = handlers.ParseSnapshotSize(s.Spec.Size)
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

func parseResponseRestoreDetail(backup *sysv1.Backup, snapshot *sysv1.Snapshot, restore *sysv1.Restore, progress float64) *ResponseRestoreDetail {
	return &ResponseRestoreDetail{
		BackupName:   backup.Spec.Name,
		BackupPath:   handlers.GetBackupPath(backup),
		SnapshotName: handlers.ParseSnapshotName(snapshot.Spec.StartAt),
		RestorePath:  handlers.GetRestorePath(restore),
		Status:       *restore.Spec.Phase,
		Message:      *restore.Spec.Message,
		Progress:     progress,
	}
}

func parseResponseRestoreList(data *sysv1.RestoreList) []*ResponseRestoreList {
	if data == nil || data.Items == nil || len(data.Items) == 0 {
		return nil
	}

	var result []*ResponseRestoreList
	for _, restore := range data.Items {
		d, err := handlers.ParseRestoreType(&restore)
		if err != nil {
			continue
		}
		var r = &ResponseRestoreList{
			Id:         restore.Name,
			BackupName: d.BackupName,
			Path:       d.Path,
			CreateAt:   restore.Spec.CreateAt,
			EndAt:      restore.Spec.EndAt,
			Status:     *restore.Spec.Phase,
		}
		result = append(result, r)
	}

	return result

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

func parseMessage(msg *string) string {
	if msg == nil {
		return ""
	}
	return *msg
}
