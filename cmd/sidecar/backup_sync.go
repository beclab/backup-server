package main

import (
	"net/http"
	"os"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/apiserver/response"
	backupv1 "bytetrade.io/web3os/backup-server/pkg/modules/backup/v1"
	syncbackupv1 "bytetrade.io/web3os/backup-server/pkg/sidecar/syncbackup/v1"
	"bytetrade.io/web3os/backup-server/pkg/sidecar/syncbackup/v1/db"
	"bytetrade.io/web3os/backup-server/pkg/signals"
	"bytetrade.io/web3os/backup-server/pkg/util"
	httputil "bytetrade.io/web3os/backup-server/pkg/util/http"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	logLevel string

	syncInterval uint

	syncServerURL string

	syncServerToken string

	backupServer string

	defaultClient *http.Client
)

var syncManager *syncbackupv1.SyncManager

func available() error {
	if syncServerToken == "" {
		return errors.New("backup sync secret not found")
	}

	var res response.Response

	url := backupServer + "/apis/backup/v1/available"
	_, err := httputil.RequestJSON("GET", url, nil, nil, &res)
	if err != nil {
		return err
	}

	if res.Code != 0 {
		return errors.New(res.Message)
	}
	return nil
}

func syncBackup() {
	log.Info("cache and syncing backups")

	var err error

	if err = available(); err != nil {
		log.Warnf("check available: %v", err)
		return
	}

	// backup server list
	backupPlanListUrl := backupServer + "/apis/backup/v1/plans"
	var plansResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Totals int                                `json:"totals"`
			Items  []*backupv1.ResponseDescribeBackup `json:"items"`
		} `json:"data"`
	}

	_, err = httputil.RequestJSON("GET", backupPlanListUrl, nil, nil, &plansResp)
	if err != nil {
		log.Errorf("list backup plan error: %+v", err)
		return
	}

	if plansResp.Code != 0 {
		log.Errorf("list backup plan: %q", plansResp.Message)
		return
	}

	for _, p := range plansResp.Data.Items {
		backupListUrl := backupServer + "/apis/backup/v1/plans/" + p.Name + "/backups"

		var r struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Totals int                    `json:"totals"`
				Items  []*backupv1.SyncBackup `json:"items"`
			} `json:"data"`
		}

		_, err = httputil.RequestJSON("GET", backupListUrl, nil, nil, &r)
		if err != nil {
			log.Errorf("list backup error: %+v", err)
			return
		}

		if r.Code != 0 {
			log.Errorf("list backup: %s", r.Message)
			return
		}

		log.Debugf("got backup totals: %d, result: %v", r.Data.Totals, util.ToJSON(r))

		for _, backup := range r.Data.Items {
			log.Infof("syncing backup %q ...", backup.Name)
			err = syncManager.StoreAndSync(backup)
			if err != nil {
				log.Warnf("backup %q, %v", backup.Name, err)
				continue
			}
			log.Infof("successfully cache and synced %q", backup.Name)
		}

	}

}

func main() {
	pflag.StringVarP(&logLevel, "log-level", "", "debug", "log level")
	pflag.StringVarP(&backupServer, "backup-server", "a",
		util.EnvOrDefault("BACKUP_SERVER", "http://127.0.0.1:8082"), "backup api server")
	pflag.UintVarP(&syncInterval, "sync-interval", "i",
		5, "sync backup interval seconds")
	pflag.StringVarP(&syncServerURL, "sync-server-url", "",
		util.EnvOrDefault("BACKUP_SYNC_SERVER_URL", "https://cloud-api.bttcdn.com/v1/resource/backup"), "sync server api token")
	pflag.StringVarP(&syncServerToken, "sync-server-token", "",
		util.EnvOrDefault("BACKUP_SECRET", ""), "sync server api token")
	pflag.Parse()

	log.InitLog(logLevel)

	ctx := signals.SetupSignalContext()
	dbOperator, err := db.NewDbOperator()
	if err != nil {
		log.Errorf("new db operator error: %+v", err)
		os.Exit(1)
	}
	syncManager = syncbackupv1.NewSyncManager(ctx, syncServerURL, syncServerToken, defaultClient, dbOperator)

	log.Debugf("startup sync backup, with intervals %d", syncInterval)
	go wait.Until(syncBackup, time.Duration(syncInterval)*time.Second, ctx.Done())

	<-ctx.Done()
	log.Info("exiting...")
	dbOperator.Close()
}
