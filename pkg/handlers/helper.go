package handlers

import (
	"encoding/json"
	"fmt"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
)

func parseSnapshotType(snapshotType string) *int {
	var r = constant.UnKnownBackupId
	switch snapshotType {
	case constant.FullyBackup:
		r = constant.FullyBackupId
	case constant.IncrementalBackup:
		r = constant.IncrementalBackupId
	}
	return &r
}

func getBackupPath(backup *sysv1.Backup) string {
	var p string
	for k, v := range backup.Spec.BackupType {
		if k != "file" {
			continue
		}
		var m = make(map[string]string)
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			log.Errorf("unmarshal backup type error: %v, value: %s", err, v)
			continue
		}
		p = m["path"]
	}

	return p
}

func getBackupLocationConfig(backup *sysv1.Backup) (location string, locationConfig map[string]string, err error) {
	for k, v := range backup.Spec.Location {
		if err = json.Unmarshal([]byte(v), &locationConfig); err != nil {
			return
		}
		_, ok := locationConfig["name"]
		if !ok {
			return location, nil, fmt.Errorf("location %s config name not exsits, config: %s", k, v)
		}

		location = k
		return
	}

	return
}
