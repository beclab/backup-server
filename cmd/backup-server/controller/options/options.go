package options

import (
	"strings"

	"bytetrade.io/web3os/backup-server/pkg/apiserver"
	"bytetrade.io/web3os/backup-server/pkg/apiserver/config"
	"github.com/spf13/pflag"
)

type ControllerServerRunOptions struct {
	ListenAddr     string
	APIRoutePrefix string
}

func NewControllerServerRunOptions() *ControllerServerRunOptions {
	return &ControllerServerRunOptions{}
}

func (s *ControllerServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.ListenAddr, "listen-address", ":8082", "server listen address")
	fs.StringVar(&s.APIRoutePrefix, "api-route-prefix", "/apis", "server route api path prefix")
}

func (s *ControllerServerRunOptions) NewAPIServer() (*apiserver.APIServer, error) {
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

func (s *ControllerServerRunOptions) Validate() (err error) {
	return
}

func (s *ControllerServerRunOptions) Complete() (err error) {
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
