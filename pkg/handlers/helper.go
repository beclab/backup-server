package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	utilstring "bytetrade.io/web3os/backup-server/pkg/util/string"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
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

func GetClusterId() (string, error) {
	var ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var clusterId string
	factory, err := client.NewFactory()
	if err != nil {
		return clusterId, errors.WithStack(err)
	}

	dynamicClient, err := factory.DynamicClient()
	if err != nil {
		return clusterId, errors.WithStack(err)
	}

	var backoff = wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2,
		Jitter:   0.1,
		Steps:    5,
	}

	if err := retry.OnError(backoff, func(err error) bool {
		return true
	}, func() error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		unstructuredUser, err := dynamicClient.Resource(constant.TerminusGVR).Get(ctx, "terminus", metav1.GetOptions{})
		if err != nil {
			return errors.WithStack(err)
		}
		obj := unstructuredUser.UnstructuredContent()
		clusterId, _, err = unstructured.NestedString(obj, "metadata", "labels", "bytetrade.io/cluster-id")
		if err != nil {
			return errors.WithStack(err)
		}
		if clusterId == "" {
			return errors.WithStack(fmt.Errorf("cluster id not found"))
		}
		return nil
	}); err != nil {
		return clusterId, errors.WithStack(err)
	}

	return clusterId, nil
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

func ParseRestoreType(restore *sysv1.Restore) (*RestoreType, error) {
	var m *RestoreType
	var data = restore.Spec.RestoreType
	v, ok := data[constant.BackupTypeFile]
	if !ok {
		return nil, fmt.Errorf("restore file type data not found")
	}

	if err := json.Unmarshal([]byte(v), &m); err != nil {
		return nil, err
	}
	return m, nil
}

/**
 * extract only cloudName and regionId from Space's BackupURL. Do not parse the user's own BackupURL.
 * The agreed format for BackupURL is:
 * app    space: https://<s3|cos>.<regionId>.amazonaws.com/<bucket>/<prefix>/restic/<backupId>?snapshotId={resticSnapshotId}
 *       prefix: did:key:xxx-yyy
 * app   custom: https://<s3|cos>.<regionId>.amazonaws.com/<bucket>/<prefix>/<repoName>?snapshotId={resticSnapshotId}
 *       prefix: from user's s3|cos endpoint
 *
 * cli    space: https://<s3|cos>.<regionId>.amazonaws.com/<bucket>/<prefix>/restic/<backupId>?snapshotId={resticSnapshotId}
 *       prefix: did:key:xxx-yyy
 * cli   custom: https://<s3|cos>.<regionId>.amazonaws.com/<bucket>/<prefix>/<repoName>?snapshotId={resticSnapshotId}
 *       prefix: from user's s3|cos endpoint
 */
func ParseRestoreBackupUrlDetail(u string) (*RestoreBackupUrlDetail, string, string, error) {
	if u == "" {
		return nil, "", "", fmt.Errorf("backupUrl is empty")
	}

	var err error
	var storage *RestoreBackupUrlDetail

	var location string = constant.BackupLocationSpace.String()
	var resticSnapshotIdPos = strings.Index(u, "?snapshotId=")
	var resticSnapshotId = u[resticSnapshotIdPos+len("?snapshotId="):]
	if resticSnapshotId == "" {
		return nil, "", "", fmt.Errorf("snapshotId is empty")
	}

	url := u[:resticSnapshotIdPos]
	url = strings.ReplaceAll(strings.ReplaceAll(url, "https://", ""), "http://", "")

	if !strings.Contains(url, "did:key") {
		switch {
		case strings.Contains(url, "cos."):
			location = constant.BackupLocationTencentCloud.String()
			storage, err = splitAwsS3Repository(url)
		case strings.Contains(url, "s3."):
			location = constant.BackupLocationAwsS3.String()
			storage, err = splitTencentCloudRepository(url)
		default:
			err = errors.New("url invalid")
		}
	} else {
		storage, err = splitSpaceRepository(url) // space
	}

	if err != nil {
		return nil, "", "", err
	}

	return storage, resticSnapshotId, location, nil
}

func splitAwsS3Repository(u string) (*RestoreBackupUrlDetail, error) {
	// s3.<region>.amazonaws.com/<bucket>/<prefix>/<repoName>
	var s = strings.Split(u, "/")
	if len(s) < 3 {
		return nil, errors.New("url invalid")
	}

	var domain = s[0]
	var hs = strings.Split(domain, ".")
	if len(hs) != 4 {
		return nil, errors.New("domain invalid")
	}
	var regionId = hs[1]
	var bucket = s[1]
	var prefix string
	var backupId string
	if len(s) == 3 {
		backupId = s[2]
	} else {
		prefix = strings.Join(s[2:len(s)-2], "/")
		backupId = s[len(s)-1]
	}

	return &RestoreBackupUrlDetail{
		BackupId:  backupId,
		CloudName: constant.BackupLocationAwsS3.String(),
		RegionId:  regionId,
		Bucket:    bucket,
		Prefix:    prefix,
	}, nil
}

func splitTencentCloudRepository(u string) (*RestoreBackupUrlDetail, error) {
	// s3.<region>.myqcloud.com/<bucket>/<prefix>/<repoName>
	var s = strings.Split(u, "/")
	if len(s) < 3 {
		return nil, errors.New("url invalid")
	}

	var domain = s[0]
	var hs = strings.Split(domain, ".")
	if len(hs) != 4 {
		return nil, errors.New("domain invalid")
	}
	var regionId = hs[1]
	var bucket = s[1]
	var prefix string
	var backupId string
	if len(s) == 3 {
		backupId = s[2]
	} else {
		prefix = strings.Join(s[2:len(s)-2], "/")
		backupId = s[len(s)-1]
	}

	return &RestoreBackupUrlDetail{
		BackupId:  backupId,
		CloudName: constant.BackupLocationTencentCloud.String(),
		RegionId:  regionId,
		Bucket:    bucket,
		Prefix:    prefix,
	}, nil
}

func splitFileSystemRepository(u string) *RestoreBackupUrlDetail {
	_ = u
	return &RestoreBackupUrlDetail{}
}

func splitSpaceRepository(u string) (*RestoreBackupUrlDetail, error) {
	if strings.Contains(u, "cos.") {
		return splitSpaceTencentCloud(u)
	}
	return splitSpaceAwsS3(u)
}

func splitSpaceAwsS3(u string) (*RestoreBackupUrlDetail, error) {
	// s3.<region>.amazonaws.com/<bucket>/<prefix>/restic/<backupId>
	// prefix = did:xxxx-yyyy
	var s = strings.Split(u, "/")
	if len(s) < 5 {
		return nil, errors.New("url invalid")
	}

	var domain = s[0]
	var hs = strings.Split(domain, ".")
	if len(hs) != 4 {
		return nil, errors.New("domain invalid")
	}

	var regionId = hs[1]
	var bucket = s[1]
	var prefix = s[2]
	var backupId = s[4]
	var suffix, err = util.GetSuffix(prefix, "-")
	if err != nil {
		return nil, errors.New("space prefix invalid")
	}

	return &RestoreBackupUrlDetail{
		BackupId:       backupId,
		CloudName:      constant.BackupLocationAwsS3.String(),
		RegionId:       regionId,
		Bucket:         bucket,
		Prefix:         prefix,
		TerminusSuffix: suffix,
	}, nil
}

func splitSpaceTencentCloud(u string) (*RestoreBackupUrlDetail, error) {
	// cos.<region>.myqcloud.com/<bucket>/<prefix>/restic/<backupId>
	// prefix = did:xxxx-yyyy
	var s = strings.Split(u, "/")
	if len(s) < 5 {
		return nil, errors.New("url invalid")
	}

	var domain = s[0]
	var hs = strings.Split(domain, ".")
	if len(hs) != 4 {
		return nil, errors.New("domain invalid")
	}
	var regionId = hs[1]
	var bucket = s[1]
	var prefix = s[2]
	var backupId = s[4]
	suffix, err := util.GetSuffix(prefix, "-") // did:key:xxx-yyy
	if err != nil {
		return nil, errors.New("space prefix invalid")
	}

	return &RestoreBackupUrlDetail{
		BackupId:       backupId,
		CloudName:      constant.BackupLocationTencentCloud.String(),
		RegionId:       regionId,
		Bucket:         bucket,
		Prefix:         prefix,
		TerminusSuffix: suffix,
	}, nil
}
