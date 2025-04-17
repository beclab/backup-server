package handlers

import (
	"fmt"
	"net/url"
	"strings"

	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"github.com/pkg/errors"
)

type SnapshotNotifyState struct {
	Prepare  bool `json:"prepare"`
	Progress bool `json:"progress"`
	Result   bool `json:"result"`
}

type RestoreType struct {
	Owner            string                  `json:"owner"`
	Type             string                  `json:"type"` // snapshot or url
	Path             string                  `json:"path"` // restore target path
	BackupName       string                  `json:"backupName"`
	BackupPath       string                  `json:"backupPath"` // from backupUrl
	Password         string                  `json:"p"`
	SnapshotId       string                  `json:"snapshotId"`
	SnapshotTime     string                  `json:"snapshotTime"` // from backupUrl
	ResticSnapshotId string                  `json:"resticSnapshotId"`
	ClusterId        string                  `json:"clusterId"`
	Location         string                  `json:"location"`
	BackupUrl        *RestoreBackupUrlDetail `json:"backupUrl"`
}

type RestoreBackupUrlDetail struct {
	BackupId       string `json:"backupId"`  // space(backupId) / custom repoName
	CloudName      string `json:"cloudName"` // awss3 tencentcloud filesystem
	RegionId       string `json:"regionId"`
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix"`
	TerminusSuffix string `json:"suffix"` // only used for space, prev backup did-suffix
	FilesystemPath string `json:"fsPath"` // only used for filesystem
}

type proxyRequest struct {
	Op       string      `json:"op"`
	DataType string      `json:"datatype"`
	Version  string      `json:"version"`
	Group    string      `json:"group"`
	Param    interface{} `json:"param,omitempty"`
	Data     string      `json:"data,omitempty"`
	Token    string
}

type passwordResponse struct {
	response.Header
	Data *passwordResponseData `json:"data,omitempty"`
}

type passwordResponseData struct {
	Env   string `json:"env"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type BackupUrlType struct {
	Schema               string
	Host                 string
	Path                 string
	Values               url.Values
	Location             string
	IsBackupToSpace      bool
	IsBackupToFilesystem bool
}

func (u *BackupUrlType) GetStorage() (*RestoreBackupUrlDetail, error) {
	backupName, err := u.getBackupName()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	regionId, err := u.getRegionId()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bucket, prefix, suffix, err := u.getBucketAndPrefix()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	fsBackupPath, err := u.getFsBackupPath()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	location := u.getLocation()
	if location == "" {
		return nil, errors.WithStack(fmt.Errorf("location invalid, host: %s, schema: %s", u.Host, u.Schema))
	}

	return &RestoreBackupUrlDetail{
		BackupId:       backupName,
		CloudName:      location,
		RegionId:       regionId,
		Bucket:         bucket,
		Prefix:         prefix,
		TerminusSuffix: suffix,
		FilesystemPath: fsBackupPath,
	}, nil
}

func (u *BackupUrlType) getLocation() string {
	switch {
	case strings.Contains(u.Host, "cos."):
		return constant.BackupLocationTencentCloud.String()
	case strings.Contains(u.Host, "s3."):
		return constant.BackupLocationAwsS3.String()
	case u.Schema == "fs":
		return constant.BackupLocationFileSystem.String()
	}

	return ""
}

func (u *BackupUrlType) getFsBackupPath() (p string, err error) {
	if u.Location != constant.BackupLocationFileSystem.String() {
		return
	}

	if u.Path == "" {
		err = errors.New("path is empty")
		return
	}

	if u.Path[0] != '/' {
		p = "/" + u.Path
	}

	return
}

func (u *BackupUrlType) getBucketAndPrefix() (bucket string, prefix string, suffix string, err error) {
	paths := strings.Split(u.Path, "/")

	if u.IsBackupToSpace {
		if len(paths) != 4 {
			err = fmt.Errorf("path invalid, path: %s", u.Path)
			return
		}
		suffix, err = util.GetSuffix(paths[1], "-")
		if err != nil {
			return
		}
		bucket = paths[0]
		prefix = paths[1]
		return
	} else {
		if len(paths) < 2 {
			err = fmt.Errorf("path invalid, path: %s", u.Path)
			return
		}
		bucket = paths[0]
		prefix = strings.Join(paths[1:len(paths)-1], "/")
		return
	}
}

func (u *BackupUrlType) getBackupName() (string, error) {
	paths := strings.Split(u.Path, "/")

	if u.IsBackupToSpace {
		if len(paths) != 4 {
			return "", fmt.Errorf("path invalid, path: %s", u.Path)
		}
	} else {
		if len(paths) < 2 {
			return "", fmt.Errorf("path invalid, path: %s", u.Path)
		}
	}

	return paths[len(paths)-1], nil
}

func (u *BackupUrlType) getRegionId() (string, error) {
	var h = strings.Split(u.Host, ".")
	if len(h) != 4 {
		return "", fmt.Errorf("region invalid invalid, host: %s", u.Host)
	}

	return h[1], nil
}
