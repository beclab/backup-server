package notify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/util/http"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/emicklei/go-restful/v3"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

const (
	SendBackupUrl       = "/v1/resource/backup/save"
	SendSnapshotUrl     = "/v1/resource/snapshot/save"
	SendDeleteBackupUrl = "/v1/resource/backup/delete"
)

type Backup struct {
	UserId         string `json:"user_id"` // did
	Token          string `json:"token"`   // access token
	BackupId       string `json:"backup_id"`
	Name           string `json:"name"`
	BackupTime     int64  `json:"backup_time"`
	BackupPath     string `json:"backup_path"`     // backup path
	BackupLocation string `json:"backup_location"` // location  space / awss3 / tencentcloud / ...
}

type Snapshot struct {
	UserId           string `json:"user_id"`     // did
	BackupId         string `json:"backup_id"`   //
	SnapshotId       string `json:"snapshot_id"` // restic snapshotId
	ResticSnapshotId string `json:"restic_snapshot_id"`
	Size             uint64 `json:"size"`          // snapshot size
	Unit             string `json:"unit"`          // "byte"
	SnapshotTime     int64  `json:"snapshot_time"` // createAt
	Status           string `json:"status"`        // snapshot phase
	Type             string `json:"type"`          // fully / incremental
	Url              string `json:"url"`           // repo URL
	CloudName        string `json:"cloud_name"`    // awss3 / tencentcloud(space）； awss3 / tencentcloud / filesystem
	RegionId         string `json:"region_id"`     // regionId(space); extract from aws/cos URL
	Bucket           string `json:"bucket"`        // bucket(space); extract from aws/cos URL
	Prefix           string `json:"prefix"`        // prefix(space); extract from aws/cos URL
	Message          string `json:"message"`       // message
}

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NotifyBackup(ctx context.Context, cloudApiUrl string, backup *Backup) error {
	var backoff = wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.5,
		Steps:    5,
	}

	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		var url = fmt.Sprintf("%s%s", cloudApiUrl, SendBackupUrl)
		var headers = make(map[string]string)
		headers[restful.HEADER_ContentType] = "application/x-www-form-urlencoded"
		var data = fmt.Sprintf("userid=%s&token=%s&backupId=%s&name=%s&backupTime=%d&backupPath=%s&backupLocation=%s",
			backup.UserId, backup.Token, backup.BackupId, backup.Name, backup.BackupTime, backup.BackupPath, backup.BackupLocation)

		log.Infof("[notify] backup data: %s", data)

		result, err := http.Post[Response](ctx, url, headers, data, false)
		if err != nil {
			return err
		}

		if result.Code != 200 {
			return fmt.Errorf("[notify] send new backup record failed: %d, url: %s", result.Code, url)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func NotifySnapshot(ctx context.Context, cloudApiUrl string, snapshot *Snapshot) error {
	var backoff = wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    5,
	}

	var data = fmt.Sprintf("userid=%s&backupId=%s&snapshotId=%s&resticSnapshotId=%s&size=%d&unit=%s&snapshotTime=%d&status=%s&type=%s&url=%s&cloud=%s&region=%s&bucket=%s&prefix=%s&message=%s", snapshot.UserId, snapshot.BackupId,
		snapshot.SnapshotId, snapshot.ResticSnapshotId, snapshot.Size, snapshot.Unit,
		snapshot.SnapshotTime, snapshot.Status, snapshot.Type,
		strings.TrimPrefix(snapshot.Url, "s3:"), snapshot.CloudName, snapshot.RegionId, snapshot.Bucket, snapshot.Prefix,
		snapshot.Message)

	log.Infof("[notify] snapshot data: %s", data)

	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		var url = fmt.Sprintf("%s%s", cloudApiUrl, SendSnapshotUrl)
		var headers = make(map[string]string)
		headers[restful.HEADER_ContentType] = "application/x-www-form-urlencoded"

		result, err := http.Post[Response](ctx, url, headers, data, true)
		if err != nil {
			return err
		}

		if result.Code != 200 {
			return fmt.Errorf("[notify] snapshot record failed, code: %d, message: %s", result.Code, result.Message)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func NotifyDeleteBackup(ctx context.Context, cloudApiUrl string, userId, token, backupId string) error {
	var backoff = wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    3,
	}

	var data = fmt.Sprintf("userid=%s&token=%s&backupId=%s", userId, token, backupId)

	log.Infof("[notify] delete backup data: %s", data)

	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		var url = fmt.Sprintf("%s%s", cloudApiUrl, SendDeleteBackupUrl)
		var headers = make(map[string]string)
		headers[restful.HEADER_ContentType] = "application/x-www-form-urlencoded"

		result, err := http.Post[Response](ctx, url, headers, data, true)
		if err != nil {
			log.Errorf("[notify] delete backup record failed: %v", err)
			return err
		}

		if result.Code != 200 {
			return fmt.Errorf("[notify] delete backup record failed, code: %d, msg: %s", result.Code, result.Message)
		}
		return nil
	}); err != nil {
		log.Errorf(err.Error())
		return err
	}

	return nil
}
