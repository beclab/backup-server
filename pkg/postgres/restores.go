package postgres

import (
	"context"
	"fmt"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"

	"gorm.io/datatypes"
)

type Restore struct {
	Id            uint64            `gorm:"primaryKey;autoIncrement"`
	Owner         string            `gorm:"type:varchar(100);not null;index:idx_restore_owner"`
	RestoreId     string            `gorm:"type:varchar(36);not null;index:idx_restore_id"`
	RestoreType   datatypes.JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	CreateAt      time.Time         `gorm:"not null;type:timestamptz"`
	StartAt       time.Time         `gorm:"not null;type:timestamptz"`
	EndAt         time.Time         `gorm:"not null;type:timestamptz"`
	Progress      int               `gorm:"not null;default:0"`
	Phase         string            `gorm:"type:varchar(20);not null;default:'Pending'"`
	Message       string            `gorm:"type:varchar(2000);not null;default:''"`
	ResticPhase   string            `gorm:"type:varchar(20);not null;default:'Pending'"`
	ResticMessage string            `gorm:"type:varchar(2000);not null;default:''"`
	Extra         datatypes.JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	CreateTime    time.Time         `gorm:"not null;type:timestamptz"`
	UpdateTime    time.Time         `gorm:"not null;type:timestamptz;autoUpdateTime"`
}

func GetRestoreById(ctx context.Context, restoreId string) (*Restore, error) {
	if DBServer == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	var restore Restore
	result := DBServer.Where("restore_id = ?", restoreId).First(&restore).WithContext(ctx)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to find restore with Id %s: %w", restoreId, result.Error)
	}

	return &restore, nil
}

func UpdateRestoreFailed(ctx context.Context, restoreId string, phase string, message string) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Restore{}).
		Where("restore_id = ?", restoreId).
		Update("end_at", time.Now()).
		Update("phase", phase).
		Update("message", message).WithContext(ctx)

	if result.Error != nil {
		return fmt.Errorf("failed to update restore %s phase %s, error: %v", restoreId, phase, result.Error)
	}

	return nil
}

func UpdateRestorePhase(ctx context.Context, restoreId string, phase string, message string) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Restore{}).
		Where("restore_id = ?", restoreId).
		Update("phase", phase).
		Update("message", message).WithContext(ctx)

	if result.Error != nil {
		return fmt.Errorf("failed to update restore %s phase %s, error: %v", restoreId, phase, result.Error)
	}

	return nil
}

func UpdateRestoreCompleted(ctx context.Context, restoreId string, progressDone int, restoreOutput *backupssdkrestic.RestoreSummaryOutput) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	var phase = constant.Completed.String()

	result := DBServer.Model(&Restore{}).
		Where("restore_id = ?", restoreId).
		Update("end_at", time.Now()).
		Update("progress", progressDone).
		Update("phase", phase).
		Update("message", phase).
		Update("restic_phase", phase).
		Update("restic_message", util.ToJSON(restoreOutput)).WithContext(ctx)

	if result.Error != nil {
		return fmt.Errorf("failed to update restore %s phase %s, error: %v", restoreId, phase, result.Error)
	}

	return nil
}
