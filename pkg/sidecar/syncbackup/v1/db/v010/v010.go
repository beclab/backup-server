package v010

import (
	"bytetrade.io/web3os/backup-server/pkg/sidecar/syncbackup/v1/db"
)

func init() {
	db.AddToDbInit([]db.InitScript{
		createSyncBackupTable,
	})
}

var (
	createSyncBackupTable = db.InitScript{
		Info: "initialize sync_backups table",
		SQL: `
create table if not exists sync_backups (
	id integer primary key autoincrement,
	uid char(36) not null,
	name varchar(256) not null,
 	owner varchar(128) not null,
	phase varchar(16) not null,
	size INTEGER not null default 0,
	synced tinyint(1) not null default 0,
	create_time datetime default CURRENT_TIMESTAMP
);
create index if not exists idx_uid on sync_backups(uid);
create index if not exists idx_name on sync_backups(name);
create index if not exists idx_create_time on sync_backups(create_time);
`}
)
