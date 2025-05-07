package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bytetrade.io/web3os/backup-server/pkg/util"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Backup struct {
	Id           uint64            `gorm:"primaryKey;autoIncrement"`
	Owner        string            `gorm:"type:varchar(100);not null;index:idx_backup_owner"`
	BackupId     string            `gorm:"type:varchar(36);not null;index:idx_backup_id"`
	BackupName   string            `gorm:"type:varchar(100);not null;index:idx_backup_name"`
	Location     datatypes.JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	BackupType   datatypes.JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	BackupPolicy datatypes.JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	Size         uint64            `gorm:"type:bigint;not null;default:0"`
	Notified     bool              `gorm:"not null;default:false"`
	Deleted      bool              `gorm:"not null;default:false"`
	Extra        datatypes.JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	CreateTime   time.Time         `gorm:"not null;type:timestamptz"`
	UpdateTime   time.Time         `gorm:"not null;type:timestamptz;autoUpdateTime"`
}

type BackupPolicy struct {
	Enabled           bool   `json:"enabled"`
	SnapshotFrequency string `json:"snapshotFrequency"`
	TimesOfDay        string `json:"timesOfDay"`
	TimespanOfDay     string `json:"timespanOfDay"`
	DayOfWeek         int    `json:"dayOfWeek"`
	DateOfMonth       int    `json:"dateOfMonth"`
}

func (p *BackupPolicy) ToJson() map[string]interface{} {
	return map[string]interface{}{
		"enabled":           p.Enabled,
		"snapshotFrequency": p.SnapshotFrequency,
		"timesOfDay":        p.TimesOfDay,
		"timespanOfDay":     p.TimespanOfDay,
		"dayOfWeek":         p.DayOfWeek,
		"dateOfMonth":       p.DateOfMonth,
	}
}

func ParseBackupPolicy(data datatypes.JSONMap) (*BackupPolicy, error) {
	var policy *BackupPolicy
	j, err := data.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal backup policy: %w", err)
	}

	if err = json.Unmarshal(j, &policy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal backup policy: %w", err)
	}

	return policy, nil
}

func CreateBackup(owner, backupId, backupName string,
	location, backupType map[string]string,
	backupPolicy *BackupPolicy, createAt time.Time) (*Backup, error) {

	if DBServer == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	// 创建备份记录
	backup := &Backup{
		Owner:        owner,
		BackupId:     backupId,
		BackupName:   backupName,
		Location:     util.ToMap(location),
		BackupType:   util.ToMap(backupType),
		BackupPolicy: backupPolicy.ToJson(),
		Size:         0,
		Notified:     false,
		Deleted:      false,
		Extra:        datatypes.JSONMap{},
		CreateTime:   createAt,
		UpdateTime:   createAt,
	}

	result := DBServer.Create(backup)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to create backup record: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("no rows affected, backup record not created")
	}

	return backup, nil
}

func GetBackupById(ctx context.Context, backupId string) (*Backup, error) {
	if DBServer == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	var backup Backup
	result := DBServer.Where("backup_id = ?", backupId).Limit(1).WithContext(ctx)
	if result.Error != nil {
		if result.Error != gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find backup with ID %s: %w", backupId, result.Error)
	}

	return &backup, nil
}

// UpdateBackupSize 更新备份大小
func UpdateBackupSize(backupId string, size uint64) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Backup{}).
		Where("backup_id = ?", backupId).
		Update("size", size)

	if result.Error != nil {
		return fmt.Errorf("failed to update backup size: %w", result.Error)
	}

	return nil
}

// MarkBackupAsNotified 将备份标记为已通知
func MarkBackupAsNotified(backupId string) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Backup{}).
		Where("backup_id = ?", backupId).
		Update("notified", true)

	if result.Error != nil {
		return fmt.Errorf("failed to mark backup as notified: %w", result.Error)
	}

	return nil
}

// SoftDeleteBackup 软删除备份记录
func SoftDeleteBackup(backupId string) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Backup{}).
		Where("backup_id = ?", backupId).
		Update("deleted", true)

	if result.Error != nil {
		return fmt.Errorf("failed to soft delete backup: %w", result.Error)
	}

	return nil
}

// UpdateBackupExtra 更新备份的额外信息
func UpdateBackupExtra(backupId string, extra map[string]interface{}) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	result := DBServer.Model(&Backup{}).
		Where("backup_id = ?", backupId).
		Update("extra", extra)

	if result.Error != nil {
		return fmt.Errorf("failed to update backup extra info: %w", result.Error)
	}

	return nil
}

// query
func GetBackupLists(ctx context.Context, owner string, limit, offset int64) error {
	if DBServer == nil {
		return fmt.Errorf("database connection not initialized")
	}

	return nil
}
