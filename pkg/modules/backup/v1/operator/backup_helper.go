package operator

import (
	"fmt"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
)

func (o *BackupOperator) GetBackupIdForLabels(backups *sysv1.BackupList) []string {
	var labels []string

	for _, backup := range backups.Items {
		labels = append(labels, fmt.Sprintf("backup-id=%s", backup.Spec.Id))
	}
	return labels
}
