package options

import (
	"strings"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/pkg/errors"
)

type Option struct {
	Provider  string `json:"provider"`
	Region    string `json:"region"`
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`

	DownloadKeyName   string `json:"downloadKeyName"`
	DownloadStorePath string `json:"downloadStorePath"`
}

func New() *Option {
	return &Option{}
}

func (o *Option) Complete() error {
	if o.DownloadKeyName == "" {
		return errors.New("download object has missing 'key' name")
	}

	if strings.HasPrefix(o.DownloadKeyName, "/") {
		o.DownloadKeyName = strings.TrimLeft(o.DownloadKeyName, "/")
	}
	return nil
}

func (o *Option) BackupConfigSpec() *sysv1.BackupConfigSpec {
	log.Debugf("flag options: %s", util.PrettyJSON(o))

	bc := sysv1.BackupConfigSpec{
		Provider:  o.Provider,
		Region:    o.Region,
		Bucket:    o.Bucket,
		S3Url:     o.Endpoint,
		AccessKey: o.AccessKey,
		SecretKey: o.SecretKey,
	}
	velero.SetDefaultBackupConfigSpec(&bc)

	return &bc
}
