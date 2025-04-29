package postgres

import (
	"fmt"
	"time"
)

type Snapshot struct {
	Id               uint64                 `gorm:"primaryKey;autoIncrement"`
	Owner            string                 `gorm:"type:varchar(100);not null;index:idx_snapshot_owner"`
	SnapshotId       string                 `gorm:"type:varchar(36);not null;index:idx_snapshot_id"`
	BackupId         string                 `gorm:"type:varchar(36);not null;index:idx_snapshot_backup_id"`
	Location         string                 `gorm:"type:varchar(100);not null;index:idx_snapshot_location"`
	SnapshotType     int                    `gorm:"not null;default:0;index:idx_snapshot_type"`
	ResticSnapshotId string                 `gorm:"type:varchar(100);not null;default '';index:idx_snapshot_restic_snapshot_id"`
	Size             uint64                 `gorm:"type:bigint;not null;default:0"`
	CreateAt         time.Time              `gorm:"not null;type:timestamptz"`
	StartAt          time.Time              `gorm:"not null;type:timestamptz"`
	EndAt            time.Time              `gorm:"not null;type:timestamptz"`
	Progress         int                    `gorm:"not null;default:0"`
	Phase            string                 `gorm:"type:varchar(20);not null;default:'Pending'"`
	Message          string                 `gorm:"type:varchar(2000);not null;default:''"`
	ResticPhase      string                 `gorm:"type:varchar(20);not null;default:'Pending'"`
	ResticMessage    string                 `gorm:"type:varchar(2000);not null;default:''"`
	Extra            map[string]interface{} `gorm:"type:jsonb;not null;default:'{}'"`
	CreateTime       time.Time              `gorm:"not null;type:timestamptz"`
	UpdateTime       time.Time              `gorm:"not null;type:timestamptz;autoUpdateTime"`
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

func UpdateSnapshotPhase(snapshotId string, phase string, message string) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Snapshot{}).
		Where("snapshot_id = ?", snapshotId).
		Update("phase", phase).
		Update("message", message)

	if result.Error != nil {
		return fmt.Errorf("failed to update snapshot %s phase %s, error: %v", snapshotId, phase, result.Error)
	}

	return nil
}
