package options

var _ Option = &SpaceBackupOptions{}

type SpaceBackupOptions struct {
	RepoName       string
	Location       string
	ClusterId      string
	OlaresId       string
	CloudName      string
	RegionId       string
	CloudApiMirror string
	Path           string
	Password       string
}

func (o *SpaceBackupOptions) GetLocation() string {
	return o.Location
}

func (o *SpaceBackupOptions) GetLocationConfigName() string {
	return o.OlaresId
}

type AwsS3BackupOptions struct {
	RepoName           string
	Location           string
	LocationConfigName string
	Path               string
	Password           string
}

func (o *AwsS3BackupOptions) GetLocation() string {
	return o.Location
}

func (o *AwsS3BackupOptions) GetLocationConfigName() string {
	return o.LocationConfigName
}

type TencentCloudBackupOptions struct {
	RepoName           string
	Location           string
	LocationConfigName string
	Path               string
	Password           string
}

func (o *TencentCloudBackupOptions) GetLocation() string {
	return o.Location
}

func (o *TencentCloudBackupOptions) GetLocationConfigName() string {
	return o.LocationConfigName
}

type FilesystemBackupOptions struct {
	RepoName string
	Location string
	Endpoint string
	Path     string
	Password string
}

func (o *FilesystemBackupOptions) GetLocation() string {
	return o.Location
}

func (o *FilesystemBackupOptions) GetLocationConfigName() string {
	return ""
}
