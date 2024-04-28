package db

import "time"

type SyncBackup struct {
	Id         int       `db:"id" json:"id"`
	UId        string    `db:"uid" json:"UId"`
	Name       string    `db:"name" json:"name"`
	Owner      string    `db:"owner" json:"owner"`
	Phase      string    `db:"phase" json:"phase"`
	Synced     int       `db:"synced" json:"synced"`
	CreateTime time.Time `db:"create_time" json:"createTime,omitempty"`
}
