package options

import (
	"strings"

	"bytetrade.io/web3os/backup-server/pkg/apiserver"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	_ "bytetrade.io/web3os/backup-server/pkg/apiserver/runtime"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/spf13/pflag"
)

type ServerRunOptions struct {
	LogLevel string

	SkipKubeClient bool

	ListenAddr string

	APIRoutePrefix string
}

func NewServerRunOptions() *ServerRunOptions {
	return &ServerRunOptions{}
}

func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.LogLevel, "log-level", "debug", "logging level")
	fs.StringVar(&s.ListenAddr, "listen-address", ":8082", "server listen address")
	fs.StringVar(&s.APIRoutePrefix, "api-route-prefix", "/apis", "server route api path prefix")

	fs.Int64Var(&velero.DefaultBackupTTL, "backup-retain-days", velero.DefaultBackupTTL, "backup ttl, retain for days")

	// for osdata backup
	fs.StringVar(&velero.DefaultOSDataPath, "terminus-rootfs-path",
		util.EnvOrDefault("TERMINUS_ROOTFS_PATH", velero.DefaultOSDataPath), "terminus os data rootfs path")
	fs.StringVar(&velero.DefaultBackupTempStoragePath, "backup-temp-path",
		util.EnvOrDefault("BACKUP_TEMP_PATH", velero.DefaultBackupTempStoragePath), "backup temp storage path")

	fs.StringVar(&velero.DefaultBackupBucket, "backup-bucket",
		util.EnvOrDefault("VELERO_BACKUP_BUCKET", velero.DefaultBackupBucket), "terminus backup bucket")
	fs.StringVar(&velero.DefaultBackupKeyPrefix, "backup-key-prefix",
		util.EnvOrDefault("VELERO_BACKUP_KEY_PREFIX", velero.DefaultBackupKeyPrefix), "terminus backup key prefix")

	fs.StringVarP(&velero.EnableMiddleWareBackup, "enable-middleware-backup", "",
		util.EnvOrDefault("ENABLE_MIDDLEWARE_BACKUP", velero.EnableMiddleWareBackup), "whether enable middleware backup check")
}

func (s *ServerRunOptions) NewAPIServer() (*apiserver.APIServer, error) {
	log.InitLog(s.LogLevel)

	// apiserver config options
	cfg := config.Config{
		ListenAddr:     s.ListenAddr,
		APIRoutePrefix: s.APIRoutePrefix,
	}

	server, err := apiserver.New(&cfg)
	if err != nil {
		return nil, err
	}

	return server, err
}

func (s *ServerRunOptions) Validate() (err error) {
	return
}

func (s *ServerRunOptions) Complete() (err error) {
	if s.APIRoutePrefix == "" {
		s.APIRoutePrefix = "/"
		return
	}
	if s.APIRoutePrefix == "/" {
		return
	}
	s.APIRoutePrefix = strings.TrimRight(s.APIRoutePrefix, "/")
	return
}
