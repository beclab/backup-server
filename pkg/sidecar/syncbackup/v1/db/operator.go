package db

import (
	"fmt"

	"bytetrade.io/web3os/backup-server/pkg/util"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

var dbFile = "/data/backup.db"

func init() {
	pflag.StringVarP(&dbFile, "db", "", util.EnvOrDefault("SQLITE_DB_FILE", dbFile),
		"default sync backup db file")
}

type Operator struct {
	DB *sqlx.DB
}

func NewDbOperator() (*Operator, error) {
	source := fmt.Sprintf("file:%s?cache=shared", dbFile)
	db, err := sqlx.Open("sqlite3", source)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	db.SetMaxOpenConns(1)

	if err = db.Ping(); err != nil {
		return nil, errors.WithStack(err)
	}

	operator := &Operator{DB: db}
	if err = InitDB(operator); err != nil {
		return nil, err
	}
	return operator, nil
}

func (db *Operator) Close() error {
	return db.DB.Close()
}
