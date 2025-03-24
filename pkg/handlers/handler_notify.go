package handlers

import (
	"fmt"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/util/http"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/emicklei/go-restful/v3"
	"github.com/pkg/errors"
)

const (
	SendBackupUrl   = "/v1/resource/backup/save"
	SendSnapshotUrl = "/v1/resource/snapshot/save"
)

type Backup struct {
	UserId         string // did
	Token          string // access token
	BackupId       string
	Name           string
	BackupPath     string // backup path
	BackupLocation string // location  space / awss3 / tencentcloud / ...
	Status         string // snapshot phase
}

type Snapshot struct {
	UserId       string // did
	BackupId     string //
	SnapshotId   string // restic snapshotId
	Size         uint64 // snapshot size
	Uint         string // "byte"
	SnapshotTime int64  // StartAt
	Status       string // snapshot phase
	Type         string // fully / incremental
	Url          string // repo URL
	CloudName    string // awss3 / tencentcloud(space）； awss3 / tencentcloud / filesystem
	RegionId     string // regionId(space); extract from aws/cos URL
	Bucket       string // bucket(space); extract from aws/cos URL
	Prefix       string // prefix(space); extract from aws/cos URL
	Message      string // message
}

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type NotifyHandler struct {
	factory  client.Factory
	handlers Interface
}

func NewNotifyHandler(f client.Factory, handlers Interface) *NotifyHandler {
	return &NotifyHandler{
		factory:  f,
		handlers: handlers,
	}
}

func (o *NotifyHandler) UpdateState() {

}

func (o *NotifyHandler) NotifySnapshotFailed(err error, backup *sysv1.Backup, snapshot *sysv1.Snapshot, f func(err error, backup *sysv1.Backup, snapshot *sysv1.Snapshot) error) error {
	var tokenService = &integration.Integration{
		Factory: o.factory,
		Owner:   backup.Spec.Owner,
	}
	token, e := tokenService.GetIntegrationSpaceToken()
	if e != nil {
		return f(errors.WithMessage(fmt.Errorf("get space token error %v", e), err.Error()), backup, snapshot)
	}

	return f(err, backup, snapshot)
}

func (o *NotifyHandler) updateBackup(cloudApiUrl string, token integration.IntegrationToken, backup *sysv1.Backup, snapshot *sysv1.Snapshot, state string) error {
	var backupPath = getBackupPath(backup)
	var location, _, _ = getBackupLocationConfig(backup)
	var url = fmt.Sprintf("%s%s", cloudApiUrl, SendBackupUrl)
	var headers = make(map[string]string)
	var t = token.(*integration.IntegrationSpace)
	headers[restful.HEADER_ContentType] = "application/x-www-form-urlencoded"
	var data = fmt.Sprintf("userid=%s&token=%s&backupId=%s&name=%s&backupPath=%s&backupLocation=%s&status=%s",
		t.OlaresDid, t.AccessToken, backup.Name, backup.Spec.Name, backupPath, location, state)

	log.Infof("send backup data: %s", data)

	result, err := http.Post[Response](url, headers, data)
	if err != nil {
		return err
	}

	if result.Code != 200 {
		return fmt.Errorf("update backup record failed: %d, url: %s", result.Code, url)
	}
	return nil
}

func (o *NotifyHandler) updateSnapshot(cloudApiUrl string, token integration.IntegrationToken, backup *sysv1.Backup, snapshot *sysv1.Snapshot, state string) {
	var t = token.(*integration.IntegrationSpace)
	var url = fmt.Sprintf("%s%s", cloudApiUrl, SendSnapshotUrl)
	var headers = make(map[string]string)
	headers[restful.HEADER_ContentType] = "application/x-www-form-urlencoded"

	var data = fmt.Sprintf("userid=%s&backupId=%s&snapshotId=%s&size=%d&unit=%s&snapshotTime=%d&status=%s&type=%s&url=%s&cloud=%s&region=%s&bucket=%s&prefix=%s&message=%s", t.OlaresDid, backup.Name,
		snapshot.Name, *snapshot.Spec.Size, "byte",
		snapshot.Spec.StartAt, snapshot.Status, *snapshot.Spec.SnapshotType,
		snapshot.Url, snapshot.CloudName, snapshot.RegionId,
		snapshot.Bucket, snapshot.Prefix, snapshot.Message)

	log.Infof("send snapshot data: %s", data)

	result, err := utils.Post[Response](url, headers, data)
	if err != nil {
		return err
	}

	if result.Code != 200 {
		return fmt.Errorf("send new snapshot record failed %s", result.Message)
	}
	return nil
}
