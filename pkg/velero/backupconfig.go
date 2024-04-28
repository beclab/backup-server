package velero

import (
	"path/filepath"
	"strings"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validateBackupConfig(c *sysv1.BackupConfigSpec) error {
	if c.StorageLocation == "" {
		return errors.New("missing 'storageLocation'")
	}

	if c.Provider == Minio {
		if c.S3Url == "" {
			return errors.New("minio 'S3Url' not allowed empty")
		}

		if c.AccessKey == "" || c.SecretKey == "" {
			return errors.New("minio must specific access_key, and secret_key")
		}
	}

	return nil
}

func defaultConfigMinio(c *sysv1.BackupConfigSpec) {
	c.Bucket = minioDefaultBucket
	c.Region = Minio
	// c.Plugins = []string{"velero/velero-plugin-for-aws:v1.2.1"}

	c.Prefix = k8sBackupPrefix
}

func defaultConfigAWS(c *sysv1.BackupConfigSpec) {
	// default terminus backup bucket, region
	if c.Bucket == "" && c.Region == "" && DefaultBackupBucket != "" {
		c.Bucket = DefaultBackupBucket
		sl := strings.Split(DefaultBackupBucket, "-")
		c.Region = strings.Join(sl[1:], "-")
	}
	if DefaultBackupKeyPrefix != "" {
		c.Prefix = filepath.Join(DefaultBackupKeyPrefix, k8sBackupPrefix)
		// c.OSDataPrefix = filepath.Join(DefaultBackupKeyPrefix, osDataBackupPrefix)
	}
	if c.Prefix == "" {
		c.Prefix = k8sBackupPrefix
		// c.OSDataPrefix = osDataBackupPrefix
	} else {
		if !strings.Contains(c.Prefix, "/") {
			prefix := c.Prefix
			c.Prefix = filepath.Join(prefix, k8sBackupPrefix)
			// c.OSDataPrefix = filepath.Join(prefix, osDataBackupPrefix)
		}
	}
	// c.Plugins = []string{"velero/velero-plugin-for-aws:v1.7.0"}

	// if c.AccessKey == "" && c.SecretKey == "" {
	// 	c.NoSecret = true
	// }
}

func defaultTerminusCloud(c *sysv1.BackupConfigSpec) {
	c.StorageLocation = c.Location
	c.Bucket = c.Location
	c.Region = c.Location
	c.Plugins = []string{}
}

func SetDefaultBackupConfigSpec(c *sysv1.BackupConfigSpec) {
	switch c.Provider {
	case Minio:
		defaultConfigMinio(c)
	case AWS:
		defaultConfigAWS(c)
	case TerminusCloud:
		defaultTerminusCloud(c)
	}

}

func BuildBackupConfig(namespace, name string, bcSpec *sysv1.BackupConfigSpec) *sysv1.BackupConfig {
	return &sysv1.BackupConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"component": Velero,
			},
			Annotations: map[string]string{
				AnnotationPerconaMongoClusterLastBackupPBMName: "",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "BackupConfig",
			APIVersion: sysv1.SchemeGroupVersion.String(),
		},
		Spec: *bcSpec,
	}
}
