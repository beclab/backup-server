package db

import (
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/pkg/errors"
)

var (
	initDbScript []InitScript
)

type InitScript struct {
	Info string
	SQL  string
}

func AddToDbInit(sqls []InitScript) {
	initDbScript = append(initDbScript, sqls...)
}

func InitDB(operator *Operator) (err error) {
	for _, s := range initDbScript {
		log.Infof("init or update db: %s", s.Info)
		_, err = operator.DB.Exec(s.SQL)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return
}
