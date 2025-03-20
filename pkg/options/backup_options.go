package options

var _ Option = &SpaceBackupOptions{}

type SpaceBackupOptions struct {
	RepoName       string
	ClusterId      string
	OlaresId       string
	CloudName      string
	RegionId       string
	CloudApiMirror string
	Path           string
	Password       string
}

type AwsBackupOptions struct {
	RepoName           string
	LocationConfigName string
	// Endpoint           string
	// AccessKey          string
	// SecretKey          string
	Path     string
	Password string
}

type TencentCloudBackupOptions struct {
	RepoName           string
	LocationConfigName string
	// Endpoint           string
	// AccessKey          string
	// SecretKey          string
	Path     string
	Password string
}

type FilesystemBackupOptions struct {
	RepoName string
	Endpoint string
	Path     string
	Password string
}
