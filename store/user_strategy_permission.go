package store

import (
	"time"

	"gorm.io/gorm"
)

// UserStrategyPermission maps a user to available strategies.
type UserStrategyPermission struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	UserID     string     `gorm:"column:user_id;not null;index:idx_user_strategy,priority:1" json:"user_id"`
	StrategyID string     `gorm:"column:strategy_id;not null;index:idx_user_strategy,priority:2" json:"strategy_id"`
	ExpiresAt  *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (UserStrategyPermission) TableName() string { return "user_strategy_permissions" }

type UserStrategyPermissionStore struct {
	db *gorm.DB
}

func NewUserStrategyPermissionStore(db *gorm.DB) *UserStrategyPermissionStore {
	return &UserStrategyPermissionStore{db: db}
}

func (s *UserStrategyPermissionStore) initTables() error {
	return s.db.AutoMigrate(&UserStrategyPermission{})
}

func (s *UserStrategyPermissionStore) ListStrategyIDsByUser(userID string) ([]string, error) {
	var strategyIDs []string
	now := time.Now().UTC()
	err := s.db.Model(&UserStrategyPermission{}).
		Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, now).
		Order("created_at DESC").
		Pluck("strategy_id", &strategyIDs).Error
	return strategyIDs, err
}

func (s *UserStrategyPermissionStore) HasAccess(userID, strategyID string) (bool, error) {
	now := time.Now().UTC()
	var count int64
	err := s.db.Model(&UserStrategyPermission{}).
		Where("user_id = ? AND strategy_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, strategyID, now).
		Count(&count).Error
	return count > 0, err
}

func (s *UserStrategyPermissionStore) ReplaceUserStrategies(userID string, strategyIDs []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&UserStrategyPermission{}).Error; err != nil {
			return err
		}
		for _, strategyID := range strategyIDs {
			if strategyID == "" {
				continue
			}
			entry := UserStrategyPermission{
				ID:         userID + "_" + strategyID,
				UserID:     userID,
				StrategyID: strategyID,
			}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
