package velero

type User struct {
	Name string `json:"name"`

	Email string `json:"email"`

	Did string `json:"did"`

	Zone string `json:"zone"`

	OwnerRole string `json:"ownerRole"`

	Annotations map[string]string `json:"annotations"`

	PersistentVolumes []PersistentVolume `json:"persistentVolumes"`
}

type PersistentVolume struct {
	Name string `json:"name"`

	Storage string `json:"storage"`

	StorageClass string `json:"storageClass"`

	HostPath string `json:"hostPath"`
}

type Version struct {
	Terminus string `json:"terminus"`
}

type MetaData struct {
	Nodes int `json:"nodes"`

	Pods int `json:"pods"`

	Version Version `json:"version"`

	PersistentVolumes []PersistentVolume `json:"persistentVolumes"`

	Users []User `json:"users"`
}

const (
	Pending = "Pending"

	Started = "Started"

	Running = "Running"

	Canceled = "Canceled"

	Succeed = "Succeed"

	Success = "Success"

	Failed = "Failed"

	InProgress = "InProgress"

	VeleroBackupCompleted = "Completed"

	FinalizingPartiallyFailed = "FinalizingPartiallyFailed"
	PartiallyFailed           = "PartiallyFailed"
	FailedValidation          = "FailedValidation"
)

type CacheBackupResult struct {
	Phase string
	Err   error
}

const (
	FullyBackup = "fully"

	IncrementalBackup = "incremental"

	ExtraBackupStorageLocation = "backupStorageLocation"

	ExtraBackupType = "backupType"

	ExtraS3Repository = "s3Repository"

	ExtraS3RepoPrefix = "s3RepoPrefix"

	ExtraSnapshotId = "snapshotId"

	ExtraRefFullyBackupUid = "refFullyBackupUid"

	ExtraRefFullyBackupName = "refFullyBackupName"

	ExtraRetainDays = "retainDays"

	ExtraRepositoryPassword = "extraRepositoryPassword"

	ExtraRepositoryPasswordHash = "extraRepositoryPasswordHash"
)

type ResticMessage struct {
	MessageType *string `json:"message_type"`

	DataAdded *int64 `json:"data_added"`

	DataBlobs *int64 `json:"data_blobs"`

	DirsChanged *int64 `json:"dirs_changed"`

	DirsNew *int64 `json:"dirs_new"`

	DirsUnmodified *int64 `json:"dirs_unmodified"`

	FilesChanged *int64 `json:"files_changed"`

	FilesNew *int64 `json:"files_new"`

	FilesUnmodified *int64 `json:"files_unmodified"`

	SnapshotID *string `json:"snapshot_id"`

	TotalBytesProcessed *int64 `json:"total_bytes_processed"`

	TotalDuration *float64 `json:"total_duration"`

	TotalFilesProcessed *int64 `json:"total_files_processed"`

	TreeBlobs *int64 `json:"tree_blobs"`
}
