package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	utilstring "bytetrade.io/web3os/backup-server/pkg/util/string"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
)

// TODO debug
func getBackupPassword(owner string, backupName string) (string, error) {
	return "123", nil

	settingsUrl := fmt.Sprintf("http://settings-service.user-space-%s/api/backup/password", owner)
	client := resty.New().SetTimeout(2 * time.Second).SetDebug(true)

	req := &proxyRequest{
		Op:       "getAccount",
		DataType: "backupPassword",
		Version:  "v1",
		Group:    "service.settings",
		Data:     backupName,
	}

	terminusNonce, err := util.GenTerminusNonce("")
	if err != nil {
		log.Error("generate nonce error, ", err)
		return "", err
	}

	log.Info("fetch password from settings, ", settingsUrl)
	resp, err := client.R().
		SetHeader(restful.HEADER_ContentType, restful.MIME_JSON).
		SetHeader("Terminus-Nonce", terminusNonce).
		SetBody(req).
		SetResult(&passwordResponse{}).
		Post(settingsUrl)

	if err != nil {
		log.Error("request settings password api error, ", err)
		return "", err
	}

	if resp.StatusCode() != http.StatusOK {
		log.Error("request settings password api response not ok, ", resp.StatusCode())
		err = errors.New(string(resp.Body()))
		return "", err
	}

	pwdResp := resp.Result().(*passwordResponse)
	log.Infof("settings password api response, %+v", pwdResp)
	if pwdResp.Code != 0 {
		log.Error("request settings password api response error, ", pwdResp.Code, ", ", pwdResp.Message)
		err = errors.New(pwdResp.Message)
		return "", err
	}

	if pwdResp.Data == nil {
		log.Error("request settings password api response data is nil, ", pwdResp.Code, ", ", pwdResp.Message)
		err = errors.New("request settings password api response data is nil")
		return "", err
	}

	return pwdResp.Data.Value, nil
}

func GetBackupIntegrationName(location string, locationConfig map[string]string) string {
	name, ok := locationConfig[location]
	if !ok {
		return ""
	}
	var config map[string]string
	if err := json.Unmarshal([]byte(name), &config); err != nil {
		return ""
	}
	return config["name"]
}

func ParseSnapshotTypeText(snapshotType *int) string {
	var t = *snapshotType
	switch t {
	case 0:
		return constant.FullyBackup
	case 1:
		return constant.IncrementalBackup
	default:
		return constant.UnKnownBackup
	}
}

func ParseSnapshotType(snapshotType string) *int {
	var r = constant.UnKnownBackupId
	switch snapshotType {
	case constant.FullyBackup:
		r = constant.FullyBackupId
	case constant.IncrementalBackup:
		r = constant.IncrementalBackupId
	}
	return &r
}

func ParseSnapshotTypeTitle(snapshotType *int) string {
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

func GetBackupPath(backup *sysv1.Backup) string {
	var p string
	for k, v := range backup.Spec.BackupType {
		if k != "file" {
			continue
		}
		var m = make(map[string]string)
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			log.Errorf("unmarshal backup type error: %v, value: %s", err, v)
			continue
		}
		p = m["path"]
	}

	return p
}

func GetRestorePath(restore *sysv1.Restore) string {
	var p string
	for k, v := range restore.Spec.RestoreType {
		if k != "file" {
			continue
		}
		var m = make(map[string]string)
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			log.Errorf("unmarshal backup type error: %v, value: %s", err, v)
			continue
		}
		p = m["path"]
	}

	return p
}

func GetBackupLocationConfig(backup *sysv1.Backup) (map[string]string, error) {
	var locationConfig map[string]string
	var err error
	for k, v := range backup.Spec.Location {
		if err = json.Unmarshal([]byte(v), &locationConfig); err != nil {
			return nil, err
		}
		_, ok := locationConfig["name"]
		if util.ListContains([]string{
			constant.BackupLocationSpace.String(),
			constant.BackupLocationAwsS3.String(),
			constant.BackupLocationTencentCloud.String(),
		}, k) && !ok {
			return nil, fmt.Errorf("location %s config name not exsits, config: %s", k, v)
		}

		_, ok = locationConfig["path"]
		if k == constant.BackupLocationFileSystem.String() && !ok {
			return nil, fmt.Errorf("location %s config path not exsits, config: %s", k, v)
		}

		locationConfig["location"] = k
		break
	}

	return locationConfig, nil
}

func ParseBackupSnapshotFrequency(str string) string {
	str = strings.ReplaceAll(str, "@", "")
	return utilstring.Title(str)
}

func ParseLocationConfig(locationConfig map[string]string) string {
	var location string
	if locationConfig == nil {
		return ParseBackupLocation(location)
	}

	for l, _ := range locationConfig {
		location = l
	}
	return ParseBackupLocation(location)
}

func ParseBackupLocation(l string) string {
	switch l {
	case constant.BackupLocationSpace.String():
		return constant.BackupLocationSpaceAlias.String()
	case constant.BackupLocationAwsS3.String():
		return constant.BackupLocationAwsS3Alias.String()
	case constant.BackupLocationTencentCloud.String():
		return constant.BackupLocationCosAlias.String()
	case constant.BackupLocationFileSystem.String():
		return constant.BackupLocationFileSystemAlias.String()
	default:
		return constant.BackupLocationUnKnownAlias.String()
	}
}

func ParseSnapshotSize(size *uint64) string {
	if size == nil {
		return ""
	}

	return fmt.Sprintf("%d", *size)
}

func ParseBackupTypePath(backupType map[string]string) string {
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

func GetNextBackupTime(bp sysv1.BackupPolicy) *int64 {
	var res int64
	var n = time.Now().Local()
	var prefix int64 = util.ParseToInt64(bp.TimesOfDay) / 1000
	var incr = util.ParseToNextUnixTime(bp.SnapshotFrequency, bp.TimesOfDay, bp.DayOfWeek)

	switch bp.SnapshotFrequency {
	case "@weekly":
		var midweek = util.GetFirstDayOfWeek(n).AddDate(0, 0, bp.DayOfWeek)
		res = midweek.Unix() + incr + prefix
	default:
		var midnight = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
		res = midnight.Unix() + incr + prefix
	}
	return &res
}

func ParseSnapshotName(startAt int64) string {
	t := time.UnixMilli(startAt)
	return t.Format("2006-01-02 15:04")
}
