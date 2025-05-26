package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/go-resty/resty/v2"
)

const (
	BackupAppHost = "rss-svc.os-system:3010"
)

const (
	BackupAppStartPath  = "{app}/backup/start"
	BackupAppStatusPath = "{app}/backup/status"
	BackupAppResultPath = "{app}/backup/result"
)

type AppHandler struct {
	name  string
	owner string
}

func NewAppHandler(appName, owner string) *AppHandler {
	return &AppHandler{
		name:  appName,
		owner: owner,
	}
}

func (app *AppHandler) StartAppBackup(parentCtx context.Context, backupId, snapshotId string) error {
	var ctx, cancel = context.WithTimeout(parentCtx, 15*time.Second)
	defer cancel()
	var appUrl = strings.ReplaceAll(BackupAppStartPath, "{app}", app.name)
	var url = fmt.Sprintf("http://%s/%s", BackupAppHost, appUrl)
	var headers = map[string]string{
		"X-BFL-USER": app.owner,
	}

	var result *BackupAppResponse

	client := resty.New().SetTimeout(15 * time.Second).SetDebug(true)

	data := map[string]string{
		"backup_id":   backupId,
		"snapshot_id": snapshotId,
	}

	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headers).
		SetFormData(data).
		SetResult(&result).
		Put(url)

	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("start app %s backup error, status code: %d, msg: %s", app.name, resp.StatusCode(), string(resp.Body()))
	}

	if result.Code != 0 {
		return fmt.Errorf("start app %s backup error, code: %d, msg: %s", app.name, result.Code, result.Message)
	}

	log.Infof("start app %s backup, backupId: %s, snapshotId: %s, msg: %s", app.name, backupId, snapshotId, string(resp.Body()))

	return nil
}

func (app *AppHandler) GetAppBackupStatus(parentCtx context.Context, backupId, snapshotId string) (*BackupAppStatus, error) {
	var ctx, cancel = context.WithTimeout(parentCtx, 15*time.Second)
	defer cancel()

	var result *BackupAppStatus
	var appUrl = strings.ReplaceAll(BackupAppStatusPath, "{app}", app.name)
	var url = fmt.Sprintf("http://%s/%s?backup_id=%s&snapshot_id=%s", BackupAppHost, appUrl, backupId, snapshotId)
	var headers = map[string]string{
		"X-BFL-USER": app.owner,
	}

	client := resty.New().SetTimeout(15 * time.Second).SetDebug(true)

	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headers).
		SetResult(&result).
		Get(url)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get app %s backup error, status code: %d, msg: %s", app.name, resp.StatusCode(), string(resp.Body()))
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("get app %s backup error, code: %d, msg: %s", app.name, result.Code, result.Message)
	}

	log.Infof("start app %s backup, backupId: %s, snapshotId: %s, msg: %s", app.name, backupId, snapshotId, string(resp.Body()))

	return result, nil
}

func (app *AppHandler) SendBackupResult(backupId, snapshotId, status, message string) error {
	return nil
}
