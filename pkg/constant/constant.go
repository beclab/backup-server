package constant

import (
	scheme "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	EnvSpaceUrl          string = "OLARES_SPACE_URL"
	DefaultSyncServerURL string = "https://cloud-api.bttcdn.com"

	DefaultOwnerHeaderKey    = "X-Backup-Owner"
	DefaultOsSystemNamespace = "os-system"

	FullyBackup         string = "fully"
	FullyBackupId       int    = 0
	IncrementalBackup   string = "incremental"
	IncrementalBackupId int    = 1
	UnKnownBackup       string = "unknown"
	UnKnownBackupId     int    = 2

	KindSnapshot string = "Snapshot"
	KindBackup   string = "Backup"
	KindRestore  string = "Restore"

	SnapshotController string = "snapshot-controller"
	BackupController   string = "backup-controller"
	RestoreController  string = "restore-controller"

	BackupTypeFile string = "file"
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
	Pending   Phase = "Pending"
	Running   Phase = "Running"
	Failed    Phase = "Failed"
	Completed Phase = "Completed"
	Canceled  Phase = "Canceled"
)
