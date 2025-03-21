package velero

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
