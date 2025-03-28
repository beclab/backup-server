package handlers

import "bytetrade.io/web3os/backup-server/pkg/apiserver/response"

type RestoreType struct {
	Owner            string                  `json:"owner"`
	Type             string                  `json:"type"` // snapshot or url
	Path             string                  `json:"path"`
	BackupUrl        *RestoreBackupUrlDetail `json:"backupUrl"`
	Password         string                  `json:"p"`
	SnapshotId       string                  `json:"snapshotId"`
	ResticSnapshotId string                  `json:"resticSnapshotId"`
	ClusterId        string                  `json:"clusterId"`
	Location         string                  `json:"location"`
}

type RestoreBackupUrlDetail struct {
	BackupId       string `json:"backupId"`  // space(backupId) / custom repoName
	CloudName      string `json:"cloudName"` // awss3 tencentcloud filesystem
	RegionId       string `json:"regionId"`
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix"`
	TerminusSuffix string `json:"suffix"` // only used for space, prev backup did-suffix
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
