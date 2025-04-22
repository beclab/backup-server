package constant

import (
	scheme "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	EnvSpaceUrl          string = "OLARES_SPACE_URL"
	DefaultSyncServerURL string = "https://cloud-api.bttcdn.com"

	DefaultSnapshotSizeUnit = "byte"
	BflUserKey              = "X-BFL-USER"

	DefaultOsSystemNamespace = "os-system"

	FullyBackup         string = "fully"
	IncrementalBackup   string = "incremental"
	UnKnownBackup       string = "unknown"
	FullyBackupId       int    = 0
	IncrementalBackupId int    = 1
	UnKnownBackupId     int    = 2

	KindSnapshot string = "Snapshot"
	KindBackup   string = "Backup"
	KindRestore  string = "Restore"

	SnapshotController string = "snapshot-controller"
	BackupController   string = "backup-controller"
	RestoreController  string = "restore-controller"

	BackupTypeFile string = "file"

	BackupPause  string = "pause"
	BackupResume string = "resume"
	BackupCancel string = "cancel"

	RestoreTypeSnapshot string = "snapshot"
	RestoreTypeUrl      string = "url"

	StorageOperatorCli string = "cli"
	StorageOperatorApp string = "app"

	EventBackup  string = "backup-state-event"
	EventRestore string = "restore-state-event"
)

type BackupLocation string

func (b BackupLocation) String() string {
	return string(b)
}

const (
	BackupLocationSpace        BackupLocation = "space"
	BackupLocationAwsS3        BackupLocation = "awss3"
	BackupLocationTencentCloud BackupLocation = "tencentcloud"
	BackupLocationFileSystem   BackupLocation = "filesystem"

	BackupLocationSpaceAlias      BackupLocation = "Olares Space"
	BackupLocationAwsS3Alias      BackupLocation = "AWS S3"
	BackupLocationCosAlias        BackupLocation = "TencentCloud COS"
	BackupLocationFileSystemAlias BackupLocation = "Local"
	BackupLocationUnKnownAlias    BackupLocation = "Unknown"
)

type BackupSnapshotFrequency string

func (b BackupSnapshotFrequency) String() string {
	return string(b)
}

const (
	BackupSnapshotFrequencyHourly  BackupSnapshotFrequency = "@hourly"
	BackupSnapshotFrequencyDaily   BackupSnapshotFrequency = "@daily"
	BackupSnapshotFrequencyWeekly  BackupSnapshotFrequency = "@weekly"
	BackupSnapshotFrequencyMonthly BackupSnapshotFrequency = "@monthly"
)

var (
	BackupGVR = schema.GroupVersionResource{
		Group:    scheme.SchemeGroupVersion.Group,
		Version:  scheme.SchemeGroupVersion.Version,
		Resource: "backups",
	}

	SnapshotGVR = schema.GroupVersionResource{
		Group:    scheme.SchemeGroupVersion.Group,
		Version:  scheme.SchemeGroupVersion.Version,
		Resource: "snapshots",
	}

	RestoreGVR = schema.GroupVersionResource{
		Group:    scheme.SchemeGroupVersion.Group,
		Version:  scheme.SchemeGroupVersion.Version,
		Resource: "restores",
	}

	TerminusGVR = schema.GroupVersionResource{
		Group:    "sys.bytetrade.io",
		Version:  "v1alpha1",
		Resource: "terminus",
	}

	UsersGVR = schema.GroupVersionResource{
		Group:    "iam.kubesphere.io",
		Version:  "v1alpha2",
		Resource: "users",
	}
)

type Phase string

func (s Phase) String() string {
	return string(s)
}

const (
	New       Phase = "New"
	Pending   Phase = "Pending"
	Running   Phase = "Running"
	Failed    Phase = "Failed"
	Completed Phase = "Completed"
	Canceled  Phase = "Canceled"
	Rejected  Phase = "Rejected"
)

const (
	MaxConcurrency   = 1
	BackupQueueSize  = 5
	RestoreQueueSize = 5
	NonBlocking      = true
)
