package notification

import (
	"fmt"

	"bytetrade.io/web3os/backups-sdk/pkg/constants"
	"bytetrade.io/web3os/backups-sdk/pkg/logger"
	"bytetrade.io/web3os/backups-sdk/pkg/utils"
	"github.com/emicklei/go-restful/v3"
)

type Backup struct {
	UserId         string
	Token          string
	BackupId       string
	Name           string
	BackupPath     string
	BackupLocation string
	Status         string
}

type Snapshot struct {
	UserId       string
	BackupId     string
	SnapshotId   string
	Size         uint64
	Uint         string
	SnapshotTime int64
	Status       string
	Type         string
	Url          string
	CloudName    string
	RegionId     string
	Bucket       string
	Prefix       string
	Message      string
}

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func SendNewBackup(cloudApiUrl string, backup *Backup) error {
	var url = fmt.Sprintf("%s%s", cloudApiUrl, constants.SendBackupUrl)
	var headers = make(map[string]string)
	headers[restful.HEADER_ContentType] = "application/x-www-form-urlencoded"
	var data = fmt.Sprintf("userid=%s&token=%s&backupId=%s&name=%s&backupPath=%s&backupLocation=%s&status=%s",
		backup.UserId, backup.Token, backup.BackupId, backup.Name, backup.BackupPath, backup.BackupLocation, backup.Status)

	logger.Infof("send backup data: %s", data)

	result, err := utils.Post[Response](url, headers, data)
	if err != nil {
		return err
	}

	if result.Code != 200 {
		return fmt.Errorf("send new backup record failed: %d, url: %s", result.Code, url)
	}
	return nil
}

func SendNewSnapshot(cloudApiUrl string, snapshot *Snapshot) error {
	var url = fmt.Sprintf("%s%s", cloudApiUrl, constants.SendSnapshotUrl)
	var headers = make(map[string]string)
	headers[restful.HEADER_ContentType] = "application/x-www-form-urlencoded"

	var data = fmt.Sprintf("userid=%s&backupId=%s&snapshotId=%s&size=%d&unit=%s&snapshotTime=%d&status=%s&type=%s&url=%s&cloud=%s&region=%s&bucket=%s&prefix=%s&message=%s", snapshot.UserId, snapshot.BackupId,
		snapshot.SnapshotId, snapshot.Size, snapshot.Uint,
		snapshot.SnapshotTime, snapshot.Status, snapshot.Type,
		snapshot.Url, snapshot.CloudName, snapshot.RegionId,
		snapshot.Bucket, snapshot.Prefix, snapshot.Message)

	logger.Infof("send snapshot data: %s", data)

	result, err := utils.Post[Response](url, headers, data)
	if err != nil {
		return err
	}

	if result.Code != 200 {
		return fmt.Errorf("send new snapshot record failed %s", result.Message)
	}
	return nil
}
