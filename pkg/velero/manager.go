package velero

import (
	"bytetrade.io/web3os/backup-server/pkg/client"
	"github.com/robfig/cron/v3"
)

type Manager interface {

	// BuildVeleroBackup(name, owner, backupType string, retainDays int64) *velerov1api.Backup

}

type velero struct {
	namespace string
	factory   client.Factory
	cron      *cron.Cron
}

var _ Manager = &velero{}

func NewManager(factory client.Factory) Manager {
	c := cron.New()
	c.Start()

	return &velero{
		namespace: "",
		factory:   factory,
		cron:      c,
	}
}
