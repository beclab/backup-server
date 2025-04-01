package options

import "bytetrade.io/web3os/backup-server/pkg/util"

var _ Option = &SpaceRestoreOptions{}

type SpaceRestoreOptions struct {
	Location          string
	ClusterId         string
	OlaresId          string
	CloudName         string
	RegionId          string
	Path              string
	Password          string
	LimitDownloadRate string
	CloudApiMirror    string
}

func (o *SpaceRestoreOptions) GetLocation() string {
	return o.Location
}

func (o *SpaceRestoreOptions) GetLocationConfigName() string {
	return o.OlaresId
}
func (o *SpaceRestoreOptions) String() string {
	return util.ToJSON(o)
}

var _ Option = &AwsS3RestoreOptions{}

type AwsS3RestoreOptions struct {
	Location           string
	LocationConfigName string
	Path               string
	Password           string
	LimitDownloadRate  string
}

func (o *AwsS3RestoreOptions) GetLocation() string {
	return o.Location
}

func (o *AwsS3RestoreOptions) GetLocationConfigName() string {
	return o.LocationConfigName
}

func (o *AwsS3RestoreOptions) String() string {
	return util.ToJSON(o)
}

var _ Option = &TencentCloudRestoreOptions{}

type TencentCloudRestoreOptions struct {
	Location           string
	LocationConfigName string
	Path               string
	Password           string
	LimitDownloadRate  string
}

func (o *TencentCloudRestoreOptions) GetLocation() string {
	return o.Location
}

func (o *TencentCloudRestoreOptions) GetLocationConfigName() string {
	return o.LocationConfigName
}

func (o *TencentCloudRestoreOptions) String() string {
	return util.ToJSON(o)
}

var _ Option = &FilesystemRestoreOptions{}

type FilesystemRestoreOptions struct {
	Location string
	Endpoint string
	Path     string
	Password string
}

func (o *FilesystemRestoreOptions) GetLocation() string {
	return o.Location
}

func (o *FilesystemRestoreOptions) GetLocationConfigName() string {
	return "filesystem" // todo
}

func (o *FilesystemRestoreOptions) String() string {
	return util.ToJSON(o)
}
