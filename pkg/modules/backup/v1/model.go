package v1

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/util/pointer"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"k8s.io/klog/v2"
)

type Config struct {
	Provider string `json:"provider,omitempty"`

	Region string `json:"region"` // ap-southeast-1

	Bucket string `json:"bucket"` // for aws

	S3Url string `json:"s3Url,omitempty"` // for minio

	Prefix string `json:"prefix"`

	AccessKey string `json:"accessKey,omitempty"`

	SecretKey string `json:"secretKey,omitempty"`
}

type BackupCreate struct {
	Name string `json:"name"`

	Location string `json:"location,omitempty"` // terminus cloud or s3

	Config *Config `json:"config,omitempty"` // if location is s3

	BackupPolicies *sysv1.BackupPolicy `json:"backupPolicies,omitempty"`

	Password string `json:"password,omitempty"`

	ConfirmPassword string `json:"confirmPassword,omitempty"`
}

type Snapshot struct {
	Name string `json:"name"`

	CreationTimestamp int64 `json:"creationTimestamp"`

	Size *int64 `json:"size"`

	Phase *string `json:"phase"`

	FailedMessage string `json:"failedMessage,omitempty"`
}

type SnapshotDetails struct {
	Name string `json:"name"`

	CreationTimestamp int64 `json:"creationTimestamp"`

	Size *int64 `json:"size"`

	Phase *string `json:"phase"`

	Config *Config `json:"config,omitempty"`

	FailedMessage string `json:"failedMessage"`

	Owner *string `json:"owner"`

	BackupType string `json:"backupType"`

	SnapshotId string `json:"snapshotId"`

	RepositoryPasswordHash string `json:"repositoryPasswordHash"`

	BackupConfigName string `json:"backupConfigName"`
}

type ResponseDescribeBackup struct {
	Name string `json:"name"`

	Size *int64 `json:"size,omitempty"`

	BackupPolicies *sysv1.BackupPolicy `json:"backupPolicies"`

	Snapshots []Snapshot `json:"snapshots,omitempty"`
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

	S3Config *Config `json:"s3Config"`

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
		Phase:             sb.Spec.ResticPhase,
		Owner:             sb.Spec.Owner,
		TerminusVersion:   sb.Spec.TerminusVersion,
		Size:              sb.Spec.Size,
		BackupConfigName:  bc.Name,
		S3Config: &Config{
			Provider: bc.Spec.Provider,
			Region:   bc.Spec.Region,
			Bucket:   bc.Spec.Bucket,
			Prefix:   strings.Split(bc.Spec.Prefix, "/")[0],
			S3Url:    bc.Spec.S3Url,
		},
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

	if sb.Spec.FailedMessage != nil && *sb.Spec.FailedMessage != "" {
		b.FailedMessage = *sb.Spec.FailedMessage
		return
	}

	vb, err := m.GetVeleroBackup(ctx, bc.Name)
	if err == nil && vb != nil {
		if vb.Status.Expiration != nil {
			b.Expiration = pointer.Int64(vb.Status.Expiration.Unix())
		}

		if vb.Status.CompletionTimestamp != nil {
			b.CompletionTimestamp = pointer.Int64(vb.Status.CompletionTimestamp.Unix())
		}
	}

	var ok bool

	ok, phase, err := m.BackupStatus(ctx, sb.Name)
	if err != nil {
		b.FailedMessage = err.Error()
	}
	if ok {
		b.Phase = pointer.String(velero.Succeed)
	} else if phase != "" {
		b.Phase = pointer.String(phase)
	}

	return
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
	s3config, err := json.Marshal(s.S3Config)
	if err != nil {
		klog.Error("parse s3 config error, ", err)
		return nil, err
	}

	formdata := make(map[string]string)
	formdata["backupConfigName"] = s.BackupConfigName
	formdata["completionTimestamp"] = toString(s.CompletionTimestamp)
	formdata["creationTimestamp"] = toString(s.CreationTimestamp)
	formdata["expiration"] = toString(s.Expiration)
	formdata["name"] = toString(s.Name)
	formdata["phase"] = toString(s.Phase)
	formdata["uid"] = toString(s.UID)
	formdata["size"] = toString(s.Size)
	formdata["s3Config"] = string(s3config)
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
