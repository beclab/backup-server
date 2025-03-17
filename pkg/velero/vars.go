package velero

import (
	"path/filepath"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Minio         = "minio"
	AWS           = "AWS"
	TerminusCloud = "terminus"
)

var SupportedProviders = []string{AWS}

var (
	Velero = "velero"

	DefaultVeleroNamespace = Velero

	DefaultVeleroServiceAccountName = Velero

	DefaultVeleroDeploymentName = Velero

	DefaultVeleroImage = "beclab/velero:v1.11.0"

	DefaultVeleroSecretName = "cloud-credentials"

	DefaultBackupStorageLocationName = "default"

	DefaultBackupConfigName = "default"

	DefaultBackupScheduleName = "system-default"

	DefaultBackupIncrementScheduleName = "autoincrement-default"

	ApplyPatchFieldManager = Velero
)

var (
	DefaultOSDataPath = "/rootfs"

	TerminusMetaDataFilePath = filepath.Join(DefaultOSDataPath, "terminus-metadata.json")

	DefaultBackupTempStoragePath = "/tmp/backups"

	DefaultBackupTTL int64 = 3650

	DefaultBackupBucket string

	DefaultBackupKeyPrefix string

	minioDefaultBucket = "terminus"

	k8sBackupPrefix = "k8s"

	osDataBackupPrefix = "osdata"

	BackupOwnerHeaderKey = "X-Backup-Owner"

	LabelStorageLocation = "velero.io/storage-location"

	LabelBackupConfig = "backup.bytetrade.io/config-name"

	AnnotationBackupOwner = "backup.bytetrade.io/owner"

	AnnotationBackupType = "backup.bytetrade.io/type"

	AnnotationBackupRetainDays = "backup.bytetrade.io/retain-days"

	AnnotationBackupJobTime = "backup.bytetrade.io/job-time"

	AnnotationBackupCanceled = "backup.bytetrade.io/canceled"

	// osdata
	AnnotationOSDataBackupRestored = "osdata.bytetrade.io/restored"

	// percona psmdb-last-backup-pbmname
	AnnotationPerconaMongoClusterLastBackupPBMName = "percona/psmdb-last-backup-pbmname"

	// middleware backup
	EnableMiddleWareBackup = "false"

	// backup subdirs of /rootfs
	BackupSourceDirFiles = []string{
		"charts",
		"userspace",
		"usertemplate",
		"terminus-metadata.json",
		"auth",
		"search",
		"middleware-backup",
	}

	ExcludeOsDataDirOrFiles = []string{
		".trash", ".config", ".accesslog", ".stats",
	}
)

var BackupConfigGVR = schema.GroupVersionResource{
	Group:    sysv1.SchemeGroupVersion.Group,
	Version:  sysv1.SchemeGroupVersion.Version,
	Resource: "backupconfigs",
}

var UsersGVR = schema.GroupVersionResource{
	Group:    "iam.kubesphere.io",
	Version:  "v1alpha2",
	Resource: "users",
}

var TerminusGVR = schema.GroupVersionResource{
	Group:    "sys.bytetrade.io",
	Version:  "v1alpha1",
	Resource: "terminus",
}
