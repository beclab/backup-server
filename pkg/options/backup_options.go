package options

import "bytetrade.io/web3os/backup-server/pkg/util"

var _ Option = &SpaceBackupOptions{}

type SpaceBackupOptions struct {
	Location       string `json:"location"`
	ClusterId      string `json:"cluster_id"`
	OlaresId       string `json:"olares_id"`
	CloudName      string `json:"cloud_name"`
	RegionId       string `json:"region_id"`
	CloudApiMirror string `json:"cloud_api_mirror"`
	Path           string `json:"path"`
	Password       string `json:"-"`
}

func (o *SpaceBackupOptions) GetLocation() string {
	return o.Location
}

func (o *SpaceBackupOptions) GetLocationConfigName() string {
	return o.OlaresId
}

func (o *SpaceBackupOptions) String() string {
	return util.ToJSON(o)
}

var _ Option = &AwsS3BackupOptions{}

type AwsS3BackupOptions struct {
	Location           string `json:"location"`
	LocationConfigName string `json:"location_config_name"`
	Path               string `json:"path"`
	Password           string `json:"-"`
}

func (o *AwsS3BackupOptions) GetLocation() string {
	return o.Location
}

func (o *AwsS3BackupOptions) GetLocationConfigName() string {
	return o.LocationConfigName
}

func (o *AwsS3BackupOptions) String() string {
	return util.ToJSON(o)
}

var _ Option = &TencentCloudBackupOptions{}

type TencentCloudBackupOptions struct {
	Location           string `json:"location"`
	LocationConfigName string `json:"location_config_name"`
	Path               string `json:"path"`
	Password           string `json:"-"`
}

func (o *TencentCloudBackupOptions) GetLocation() string {
	return o.Location
}

func (o *TencentCloudBackupOptions) GetLocationConfigName() string {
	return o.LocationConfigName
}

func (o *TencentCloudBackupOptions) String() string {
	return util.ToJSON(o)
}

var _ Option = &FilesystemBackupOptions{}

type FilesystemBackupOptions struct {
	Location string `json:"location"`
	Endpoint string `json:"endpoint"`
	Path     string `json:"path"`
	Password string `json:"-"`
}

func (o *FilesystemBackupOptions) GetLocation() string {
	return o.Location
}

func (o *FilesystemBackupOptions) GetLocationConfigName() string {
	return ""
}

func (o *FilesystemBackupOptions) String() string {
	return util.ToJSON(o)
}
