package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/util/repo"
	utilstring "bytetrade.io/web3os/backup-server/pkg/util/string"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

func CheckSnapshotNotifyState(snapshot *sysv1.Snapshot, field string) (bool, error) {
	if snapshot.Spec.Extra == nil {
		return false, fmt.Errorf("snapshot %s extra is nil", snapshot.Name)
	}

	notifyState, ok := snapshot.Spec.Extra["push"]
	if !ok {
		return false, fmt.Errorf("snapshot %s extra push is nil", snapshot.Name)
	}

	var s *SnapshotNotifyState
	if err := json.Unmarshal([]byte(notifyState), &s); err != nil {
		return false, err
	}

	switch field {
	case "progress":
		return s.Progress, nil
	case "result":
		return s.Result, nil
	case "prepare":
		return s.Prepare, nil
	}

	return false, fmt.Errorf("field not found")
}

func GetBackupPassword(ctx context.Context, owner string, backupName string) (string, error) {
	d := os.Getenv("PASSWORD_DEBUG")
	if d != "" {
		return "123", nil
	}

	settingsUrl := fmt.Sprintf("http://settings-service.user-space-%s/api/backup/password", owner)
	client := resty.New().SetTimeout(5 * time.Second).SetDebug(true)

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
	resp, err := client.R().SetContext(ctx).
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
		return "0"
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

	timeParts := strings.Split(bp.TimesOfDay, ":")
	if len(timeParts) != 2 {
		return nil
	}

	hours, errHour := strconv.Atoi(timeParts[0])
	minutes, errMin := strconv.Atoi(timeParts[1])
	if errHour != nil || errMin != nil {
		return nil
	}

	switch bp.SnapshotFrequency {
	case "@hourly":
		res = getNextBackupTimeByHourly(minutes).Unix()
	case "@weekly":
		res = getNextBackupTimeByWeekly(hours, minutes, bp.DayOfWeek).Unix()
	case "@monthly":
		res = getNextBackupTimeByMonthly(hours, minutes, bp.DateOfMonth).Unix()
	default:
		res = getNextBackupTimeByDaily(hours, minutes).Unix()
	}
	return &res
}

func getNextBackupTimeByDaily(hours, minutes int) time.Time {
	var n = time.Now()

	today := time.Date(n.Year(), n.Month(), n.Day(), hours, minutes, 0, 0, n.Location())

	if today.Before(n) {
		return today.AddDate(0, 0, 1)
	}

	return today
}

func getNextBackupTimeByMonthly(hours, minutes int, day int) time.Time {

	var n = time.Now()
	firstDayOfMonth := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, n.Location())

	backupDay := firstDayOfMonth.AddDate(0, 0, day-1)

	backupTime := time.Date(backupDay.Year(), backupDay.Month(), backupDay.Day(),
		hours, minutes, 0, 0, n.Location())

	if backupTime.Before(n) {
		backupTime = backupTime.AddDate(0, 1, 0)
	}

	if backupTime.Day() != day {
		nextMonth := backupTime.AddDate(0, 1, 0)
		firstDayOfNextMonth := time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, n.Location())
		backupTime = firstDayOfNextMonth.AddDate(0, 0, day-1)

		for backupTime.Day() != day {
			nextMonth = backupTime.AddDate(0, 1, 0)
			firstDayOfNextMonth = time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, n.Location())
			backupTime = firstDayOfNextMonth.AddDate(0, 0, day-1)
		}
	}

	return backupTime
}

func getNextBackupTimeByWeekly(hours, minutes int, weekly int) time.Time {
	weekly = weekly - 1
	var n = time.Now()
	firstDayOfWeek := util.GetFirstDayOfWeek(n)

	backupDay := firstDayOfWeek.AddDate(0, 0, weekly)

	backupTime := time.Date(backupDay.Year(), backupDay.Month(), backupDay.Day(),
		hours, minutes, 0, 0, n.Location())

	if backupTime.Before(n) {
		backupTime = backupTime.AddDate(0, 0, 7)
	}

	return backupTime
}

func getNextBackupTimeByHourly(minutes int) time.Time {
	now := time.Now()

	currentMinute := now.Minute()
	nextMinute := currentMinute

	remainder := currentMinute % minutes
	if remainder == 0 && now.Second() == 0 && now.Nanosecond() == 0 {
		nextMinute = currentMinute + minutes
	} else {
		nextMinute = currentMinute + (minutes - remainder)
	}

	minutesToAdd := nextMinute - currentMinute

	nextTime := now.Add(time.Duration(minutesToAdd) * time.Minute).
		Truncate(time.Minute)

	return nextTime
}

func ParseRestoreType(restore *sysv1.Restore) (*RestoreType, error) {
	var m *RestoreType
	var data = restore.Spec.RestoreType
	v, ok := data[constant.BackupTypeFile]
	if !ok {
		return nil, errors.WithStack(fmt.Errorf("restore file type data not found"))
	}

	if err := json.Unmarshal([]byte(v), &m); err != nil {
		return nil, errors.WithStack(err)
	}
	return m, nil
}

func ParseBackupNameFromRestore(restore *sysv1.Restore) string {
	if restore == nil || restore.Spec.RestoreType == nil {
		return ""
	}

	data, ok := restore.Spec.RestoreType[constant.BackupTypeFile]
	if !ok {
		return ""
	}

	var r *RestoreType
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return ""
	}
	return r.BackupName
}

func ParseRestoreBackupUrlDetail(owner, u string) (storage *RestoreBackupUrlDetail, backupName, backupId string, resticSnapshotId string, snapshotTime string, backupPath string, location string, err error) {
	if u == "" {
		err = fmt.Errorf("backupUrl is empty")
		return
	}

	u = strings.TrimPrefix(u, "s3:")
	u = strings.TrimRight(u, "/")
	backupUrlType, e := ParseBackupUrl(owner, u)
	if e != nil {
		err = errors.WithMessage(e, fmt.Sprintf("parse backupUrl failed, backupUrl: %s", u))
		return
	}

	log.Infof("backup url type: %s", util.ToJSON(backupUrlType))

	if backupName = backupUrlType.BackupName; backupName == "" {
		err = errors.WithStack(fmt.Errorf("backupName is empty, backupUrl: %s", u))
		return
	}

	if backupId = backupUrlType.BackupId; backupId == "" {
		err = errors.WithStack(fmt.Errorf("backupId is empty, backupUrl: %s", u))
		return
	}

	if resticSnapshotId = backupUrlType.Values.Get("snapshotId"); resticSnapshotId == "" {
		err = errors.WithStack(fmt.Errorf("snapshotId is empty, backupUrl: %s", u))
		return
	}

	if snapshotTime = backupUrlType.Values.Get("snapshotTime"); snapshotTime == "" {
		err = errors.WithStack(fmt.Errorf("snapshotTime is empty, backupUrl: %s", u))
		return
	}

	if backupPath = backupUrlType.Values.Get("backupPath"); backupPath == "" {
		err = errors.WithStack(fmt.Errorf("backupPath is empty, backupUrl: %s", u))
		return
	} else {
		backupPathBytes, _ := util.Base64decode(backupPath)
		backupPath = string(backupPathBytes)
	}

	location = backupUrlType.Location
	storage, err = backupUrlType.GetStorage()
	if err != nil {
		return
	}

	return
}

func IsBackupLocationSpace(u string) bool {
	return strings.Contains(u, constant.LocationTypeSpaceTag)
}

func ParseBackupUrl(owner, s string) (*BackupUrlType, error) {
	userspacePath, err := GetUserspacePvc(owner)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var location string
	if u.Scheme == "" && u.Path[:1] == "/" {
		location = constant.BackupLocationFileSystem.String()
	} else if strings.Contains(u.Path, constant.LocationTypeSpaceTag) {
		location = constant.BackupLocationSpace.String()
	} else if strings.Contains(u.Host, constant.LocationTypeAwsS3Tag) {
		location = constant.BackupLocationAwsS3.String()
	} else if strings.Contains(u.Host, constant.LocationTypeTencentCloudTag) {
		location = constant.BackupLocationTencentCloud.String()
	}

	if location == "" {
		return nil, fmt.Errorf("location is empty, url: %s", s)
	}

	if strings.TrimPrefix(u.Path, "/") == "" {
		return nil, fmt.Errorf("url invalid, path: %s", u.Path)
	}

	idx := strings.Index(u.Path, constant.DefaultStoragePrefix)
	if idx == -1 {
		return nil, fmt.Errorf("url invalid, url: %s", u)
	}

	pathSuffix := u.Path[idx+len(constant.DefaultStoragePrefix):]

	backupName, backupId, err := utilstring.SplitPath(pathSuffix)
	if err != nil {
		return nil, fmt.Errorf("split path errror: %v, url: %s", err, s)
	}

	repoInfo, err := FormatEndpoint(location, u.Scheme, u.Host, u.Path, userspacePath)
	if err != nil {
		return nil, err
	}

	if location == constant.BackupLocationFileSystem.String() {
		if !util.IsExist(repoInfo.Endpoint) {
			return nil, fmt.Errorf("backup dir not exists, path: %s", repoInfo.Endpoint)
		}
	}

	var cloudName string
	if location == constant.BackupLocationSpace.String() {
		if strings.Contains(u.Host, constant.LocationTypeAwsS3Tag) {
			cloudName = "aws"
		} else if strings.Contains(u.Host, constant.LocationTypeTencentCloudTag) {
			cloudName = constant.BackupLocationTencentCloud.String()
		}
	}

	var res = &BackupUrlType{
		Schema:     u.Scheme,
		Host:       u.Host,
		Path:       u.Path,
		Values:     u.Query(),
		Location:   location,
		Endpoint:   repoInfo.Endpoint,
		BackupId:   backupId,
		BackupName: backupName,
		PvcPath:    userspacePath,

		CloudName:      cloudName,
		Region:         repoInfo.Region,
		Bucket:         repoInfo.Bucket,
		Prefix:         repoInfo.Prefix,
		TerminusSuffix: repoInfo.Suffix,
	}

	if location == constant.BackupLocationFileSystem.String() {
		res.FilesystemPath = repoInfo.Endpoint
	}
	return res, nil
}

func GenericPager[T runtime.Object](limit int64, offset int64, resourceList T) (T, int64, int64) {
	if limit <= 0 {
		limit = 5
	}
	if offset < 0 {
		offset = 0
	}

	listValue := reflect.ValueOf(resourceList)
	if listValue.Kind() == reflect.Ptr {
		listValue = listValue.Elem()
	}

	itemsField := listValue.FieldByName("Items")
	if !itemsField.IsValid() || itemsField.Kind() != reflect.Slice {
		return resourceList, 1, 1
	}

	total := int64(itemsField.Len())
	resultList := reflect.New(reflect.TypeOf(resourceList).Elem()).Elem()

	if typeMetaField := resultList.FieldByName("TypeMeta"); typeMetaField.IsValid() {
		originalTypeMetaField := listValue.FieldByName("TypeMeta")
		if originalTypeMetaField.IsValid() {
			typeMetaField.Set(originalTypeMetaField)
		}
	}

	if listMetaField := resultList.FieldByName("ListMeta"); listMetaField.IsValid() {
		originalListMetaField := listValue.FieldByName("ListMeta")
		if originalListMetaField.IsValid() {
			listMetaField.Set(originalListMetaField)
		}
	}

	startIndex := offset
	endIndex := offset + limit

	if startIndex >= total {
		emptySlice := reflect.MakeSlice(itemsField.Type(), 0, 0)
		resultList.FieldByName("Items").Set(emptySlice)
	} else {
		if endIndex > total {
			endIndex = total
		}

		newItemsSlice := reflect.MakeSlice(itemsField.Type(), int(endIndex-startIndex), int(endIndex-startIndex))
		for i := startIndex; i < endIndex; i++ {
			newItemsSlice.Index(int(i - startIndex)).Set(itemsField.Index(int(i)))
		}
		resultList.FieldByName("Items").Set(newItemsSlice)
	}

	currentPage := offset/limit + 1
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	return resultList.Addr().Interface().(T), currentPage, totalPages
}

func GetUserspacePvc(owner string) (string, error) {
	f, err := client.NewFactory()
	if err != nil {
		return "", errors.WithStack(err)
	}

	c, err := f.KubeClient()
	if err != nil {
		return "", errors.WithStack(err)
	}

	res, err := c.AppsV1().StatefulSets("user-space-"+owner).Get(context.TODO(), "bfl", metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("get bfl failed, owner: %s", owner))
	}

	userspacePvc, ok := res.Annotations["userspace_pvc"]
	if !ok {
		return "", fmt.Errorf("bfl userspace_pvc not found, owner: %s", owner)
	}

	var p = path.Join("/", "rootfs", "userspace", userspacePvc)

	return p, nil
}

func TrimPathPrefix(p string) (bool, string) {
	if strings.HasPrefix(p, "/Files/External") {
		return true, strings.TrimPrefix(p, "/Files/External")
	} else if strings.HasPrefix(p, "/Files") {
		return false, strings.TrimPrefix(p, "/Files")
	} else {
		return false, p
	}
}

func FormatEndpoint(location, schema, host, urlPath, pvc string) (*repo.RepositoryInfo, error) {
	var p = utilstring.TrimSuffix(urlPath, constant.DefaultStoragePrefix)

	switch location {
	case constant.BackupLocationSpace.String():
		return repo.FormatSpace(schema, host, p)
	case constant.BackupLocationTencentCloud.String():
		return repo.FormatCos(schema, host, p)
	case constant.BackupLocationAwsS3.String():
		var rawUrl = fmt.Sprintf("%s://%s%s", schema, host, p)
		result, err := repo.FormatS3(rawUrl)
		if err != nil {
			return nil, err
		}
		result.Endpoint = strings.TrimPrefix(result.Endpoint, "s3:")
		return result, nil
	case constant.BackupLocationFileSystem.String():
		external, relativePath := TrimPathPrefix(p)
		var endpoint string
		if external {
			endpoint = path.Join(constant.ExternalPath, relativePath)
		} else {
			endpoint = path.Join(pvc, relativePath)
		}
		return &repo.RepositoryInfo{
			Endpoint: endpoint,
		}, nil
	default:
		return nil, fmt.Errorf("location invalid: %s", location)
	}
}
