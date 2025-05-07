package postgres

import (
	"context"
	"fmt"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/util"
	backupssdkrestic "bytetrade.io/web3os/backups-sdk/pkg/restic"
	backupssdkmodel "bytetrade.io/web3os/backups-sdk/pkg/storage/model"

	"gorm.io/datatypes"
)

type Snapshot struct {
	Id               uint64            `gorm:"primaryKey;autoIncrement"`
	Owner            string            `gorm:"type:varchar(100);not null;index:idx_snapshot_owner"`
	SnapshotId       string            `gorm:"type:varchar(36);not null;index:idx_snapshot_id"`
	BackupId         string            `gorm:"type:varchar(36);not null;index:idx_snapshot_backup_id"`
	Location         string            `gorm:"type:varchar(100);not null;index:idx_snapshot_location"`
	SnapshotType     int               `gorm:"not null;default:0;index:idx_snapshot_type"`
	ResticSnapshotId string            `gorm:"type:varchar(100);not null;default '';index:idx_snapshot_restic_snapshot_id"`
	Size             uint64            `gorm:"type:bigint;not null;default:0"`
	CreateAt         time.Time         `gorm:"not null;type:timestamptz"`
	StartAt          time.Time         `gorm:"not null;type:timestamptz"`
	EndAt            time.Time         `gorm:"not null;type:timestamptz"`
	Progress         int               `gorm:"not null;default:0"`
	Phase            string            `gorm:"type:varchar(20);not null;default:'Pending'"`
	Message          string            `gorm:"type:varchar(2000);not null;default:''"`
	ResticPhase      string            `gorm:"type:varchar(20);not null;default:'Pending'"`
	ResticMessage    string            `gorm:"type:varchar(2000);not null;default:''"`
	StorageMessage   string            `gorm:"type:varchar(2000);not null;default:''"`
	Extra            datatypes.JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	CreateTime       time.Time         `gorm:"not null;type:timestamptz"`
	UpdateTime       time.Time         `gorm:"not null;type:timestamptz;autoUpdateTime"`
}

func CreateSnapshot(owner, snapshotId, backupId, location, phase string, snapshotType int) (*Snapshot, error) {

	if DBServer == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	var createAt = time.Now()
	snapshot := &Snapshot{
		Owner:        owner,
		SnapshotId:   snapshotId,
		BackupId:     backupId,
		Location:     location,
		SnapshotType: snapshotType,
		CreateAt:     createAt,
		StartAt:      createAt,
		EndAt:        createAt,
		Phase:        phase,
		Extra:        datatypes.JSONMap{},
	}

	result := DBServer.Create(snapshot)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to create snapshot record: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("no rows affected, snapshot record not created")
	}

	return snapshot, nil
}

func UpdateSnapshotFailed(ctx context.Context, snapshotId string, phase string, message string) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Snapshot{}).
		Where("snapshot_id = ?", snapshotId).
		Update("end_at", time.Now()).
		Update("phase", phase).
		Update("message", message).WithContext(ctx)

	if result.Error != nil {
		return fmt.Errorf("failed to update snapshot %s phase %s, error: %v", snapshotId, phase, result.Error)
	}

	return nil
}

func UpdateSnapshotPhase(ctx context.Context, snapshotId string, phase string, message string) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Snapshot{}).
		Where("snapshot_id = ?", snapshotId).
		Update("phase", phase).
		Update("message", message).WithContext(ctx)

	if result.Error != nil {
		return fmt.Errorf("failed to update snapshot %s phase %s, error: %v", snapshotId, phase, result.Error)
	}

	return nil
}

func UpdateSnapshotCompleted(ctx context.Context, backupId, snapshotId string, snapshotType int, progressDone int, backupOutput *backupssdkrestic.SummaryOutput,
	backupStorageObj *backupssdkmodel.StorageInfo, backupTotalSize uint64) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	tx := DBServer.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	endAt := time.Now()

	snapshotResult := tx.Model(&Snapshot{}).
		Where("snapshot_id = ?", snapshotId).
		Updates(map[string]interface{}{
			"snapshot_type":      snapshotType,
			"restic_snapshot_id": backupOutput.SnapshotID,
			"size":               backupOutput.TotalBytesProcessed,
			"end_at":             endAt,
			"progress":           progressDone,
			"phase":              constant.Completed.String(),
			"message":            constant.Completed.String(),
			"restic_phase":       constant.Completed.String(),
			"restic_message":     util.ToJSON(backupOutput),
			"storage_message":    util.ToJSON(backupStorageObj),
		})

	if snapshotResult.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update snapshot %s: %w", snapshotId, snapshotResult.Error)
	}

	backupResult := tx.Model(&Backup{}).
		Where("backup_id = ?", backupId).
		Update("size", backupTotalSize)

	if backupResult.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update backup %s size: %w", backupId, backupResult.Error)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func GetSnapshotById(ctx context.Context, snapshotId string) (*Snapshot, error) {
	if DBServer == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	var snapshot Snapshot
	result := DBServer.Where("snapshot_id = ?", snapshotId).First(&snapshot).WithContext(ctx)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to find snapshot with Id %s: %w", snapshotId, result.Error)
	}

	return &snapshot, nil
}

func GetSnapshotsByBackupId(ctx context.Context, backupId string) ([]*Snapshot, error) {
	if DBServer == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}
	var snapshots []*Snapshot
	result := DBServer.Where("backup_id = ?", backupId).Find(&snapshots).WithContext(ctx)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to find snapshots with backupId %s: %w", backupId, result.Error)
	}
	return snapshots, nil
}
