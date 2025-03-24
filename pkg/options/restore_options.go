package options

var _ Option = &SpaceRestoreOptions{}

type SpaceRestoreOptions struct {
	RepoName          string
	SnapshotId        string
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

func (o *SpaceRestoreOptions) GetRepoName() string {
	return o.RepoName
}

func (o *SpaceRestoreOptions) GetLocation() string {
	return o.Location
}

func (o *SpaceRestoreOptions) GetLocationConfigName() string {
	return o.OlaresId
}

var _ Option = &AwsS3RestoreOptions{}

type AwsS3RestoreOptions struct {
	RepoName           string
	Location           string
	LocationConfigName string
	SnapshotId         string
	Path               string
	Password           string
	LimitDownloadRate  string
}

func (o *AwsS3RestoreOptions) GetRepoName() string {
	return o.RepoName
}

func (o *AwsS3RestoreOptions) GetLocation() string {
	return o.Location
}

func (o *AwsS3RestoreOptions) GetLocationConfigName() string {
	return o.LocationConfigName
}

var _ Option = &TencentCloudRestoreOptions{}

type TencentCloudRestoreOptions struct {
	RepoName           string
	Location           string
	LocationConfigName string
	SnapshotId         string
	Path               string
	Password           string
	LimitDownloadRate  string
}

func (o *TencentCloudRestoreOptions) GetRepoName() string {
	return o.RepoName
}

func (o *TencentCloudRestoreOptions) GetLocation() string {
	return o.Location
}

func (o *TencentCloudRestoreOptions) GetLocationConfigName() string {
	return o.LocationConfigName
}

var _ Option = &FilesystemRestoreOptions{}

type FilesystemRestoreOptions struct {
	RepoName   string
	Location   string
	SnapshotId string
	Endpoint   string
	Path       string
	Password   string
}

func (o *FilesystemRestoreOptions) GetRepoName() string {
	return o.RepoName
}

func (o *FilesystemRestoreOptions) GetLocation() string {
	return o.Location
}

func (o *FilesystemRestoreOptions) GetLocationConfigName() string {
	return "filesystem"
}
