package runtime

import (
	"os"

	"bytetrade.io/web3os/backup-server/pkg/util/log"
)

func Must(err error) {
	if err != nil {
		log.Errorf("%+v\n", err)
		os.Exit(1)
	}
}
