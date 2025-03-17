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

	FullyBackup       string = "fully"
	IncrementalBackup string = "incremental"
)

type BackupLocation string

func (b BackupLocation) String() string {
	return string(b)
}

const (
	BackupLocationSpace BackupLocation = "space"
	BackupLocationS3    BackupLocation = "s3"
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
	DefaultCloudApiMirror = "https://cloud-dev-api.olares.xyz"

	BackupGVR = schema.GroupVersionResource{
		Group:    scheme.SchemeGroupVersion.Group,
		Version:  scheme.SchemeGroupVersion.Version,
		Resource: "backups",
	}

	SnapshotGVR = schema.GroupVersionResource{
		Group:    scheme.SchemeGroupVersion.Group,
		Version:  scheme.SchemeGroupVersion.Version,
		Resource: "snapshot",
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
