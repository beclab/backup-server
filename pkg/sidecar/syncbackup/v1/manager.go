package v1

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	backupv1 "bytetrade.io/web3os/backup-server/pkg/modules/backup/v1"
	"bytetrade.io/web3os/backup-server/pkg/sidecar/syncbackup/v1/db"
	_ "bytetrade.io/web3os/backup-server/pkg/sidecar/syncbackup/v1/db/v010"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

type SyncManager struct {
	ctx context.Context

	dbOperator *db.Operator

	httpclient *http.Client

	syncServerURL string

	syncServerToken string
}

func NewSyncManager(ctx context.Context, syncServerURL, syncServerToken string, client *http.Client, operator *db.Operator) *SyncManager {
	return &SyncManager{ctx: ctx,
		syncServerURL:   syncServerURL,
		httpclient:      client,
		syncServerToken: syncServerToken,
		dbOperator:      operator}
}

func (s *SyncManager) queryOne(name, owner, uid string) (*db.SyncBackup, error) {
	stmt := `select id,uid,name,owner,phase,synced from sync_backups where uid = ? and name = ? and owner = ?`
	row := s.dbOperator.DB.QueryRow(stmt, uid, name, owner)
	if row == nil {
		return nil, errors.Errorf("query row: %v", row.Err())
	}

	var b db.SyncBackup

	err := row.Scan(&b.Id, &b.UId, &b.Name, &b.Owner, &b.Phase, &b.Synced)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
	}
	return &b, errors.WithStack(err)
}

func (s *SyncManager) push(b *backupv1.SyncBackup) (err error) {
	var r struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	}

	headers := map[string]string{
		"Authorization": s.syncServerToken,
	}

	formdata, err := b.FormData()
	if err != nil {
		return err
	}

	client := resty.New().SetTimeout(5 * time.Second).SetDebug(true)
	resp, err := client.R().
		SetHeaders(headers).
		SetFormData(formdata).
		SetResult(&r).
		Post(s.syncServerURL)
	if err != nil {
		klog.Error("push to cloud err, ", err)
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return errors.Errorf("push sync response err code: %v, msg %s", resp.StatusCode(), string(resp.Body()))
	}

	if r.Code != http.StatusOK {
		return errors.Errorf("push sync response err: %v", r.Message)
	}

	log.Debugf("pushed backup %q, and response data: %s", b.Name, util.PrettyJSON(r))

	return nil
}

func (s *SyncManager) StoreAndSync(b *backupv1.SyncBackup) error {
	if b == nil || b.Name == "" || b.Owner == nil {
		return errors.New("backup name or owner is empty")
	}

	row, err := s.queryOne(b.Name, *b.Owner, b.UID)
	if err != nil && sql.ErrNoRows != err {
		return err
	}

	log.Debugf("query backup %q, and got row: %s", b.Name, util.ToJSON(row))

	var flag bool
	var result sql.Result

	if row == nil {
		log.Debugf("creating backup %s/%s record", b.Name, *b.Owner)
		result, err = s.dbOperator.DB.
			Exec(`insert into sync_backups(uid,name,owner,phase,synced) values(?, ?, ?, ?, ?)`,
				b.UID, b.Name, b.Owner, b.Phase, 0)
		if err != nil {
			return errors.WithStack(err)
		}
	} else if row != nil && row.Phase != "" {
		if row.Synced == 1 {
			return errors.New("has synced")
		}

		if *b.Phase == velero.Succeed && b.S3Repository != "" {
			flag = true
		}

	}

	// push sync
	if flag {
		log.Debugf("pushing backup %q", b.Name)
		if err = s.push(b); err != nil {
			return err
		}
		log.Debugf("backup %q push synced", b.Name)

		log.Debugf("updating %s/%s(%s) record", b.Name, *b.Owner, b.UID)
		result, err = s.dbOperator.DB.
			Exec(`update sync_backups set phase = ?, synced = ? where id = ?`, b.Phase, 1, row.Id)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if result != nil {
		var rowsAffected int64
		rowsAffected, err = result.RowsAffected()
		if err != nil {
			return errors.WithStack(err)
		}
		log.Debugf("%q affected rows: %v", b.Name, rowsAffected)
	}

	return nil
}

func (s *SyncManager) CloseDB() error {
	return s.dbOperator.Close()
}
