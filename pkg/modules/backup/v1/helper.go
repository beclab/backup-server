package v1

import "bytetrade.io/web3os/backup-server/pkg/constant"

func GetBackupType(backupTypeParam string) string {
	if backupTypeParam == constant.BackupTypeApp {
		return constant.BackupTypeApp
	}
	return constant.BackupTypeFile
}
