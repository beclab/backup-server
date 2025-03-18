package options

var _ Option = &SpaceBackupOptions{}

type SpaceBackupOptions struct {
	RepoName       string
	ClusterId      string
	CloudName      string
	RegionId       string
	CloudApiMirror string
	Path           string
	Password       string
}

type S3BackupOptions struct {
	RepoName  string
	Endpoint  string
	AccessKey string
	SecretKey string
	Path      string
	Password  string
}

type CosBackupOptions struct {
	RepoName  string
	Endpoint  string
	AccessKey string
	SecretKey string
	Path      string
	Password  string
}

type FilesystemBackupOptions struct {
	RepoName string
	Endpoint string
	Path     string
	Password string
}
