package options

import (
	"strings"

	"bytetrade.io/web3os/backup-server/pkg/apiserver"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	_ "bytetrade.io/web3os/backup-server/pkg/apiserver/runtime"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
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
