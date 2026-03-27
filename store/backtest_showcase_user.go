package store

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BacktestShowcaseUserStore struct {
	db *gorm.DB
}

type BacktestShowcaseUser struct {
	Email     string    `gorm:"column:email;primaryKey" json:"email"`
	Enabled   bool      `gorm:"column:enabled;not null;default:true;index" json:"enabled"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (BacktestShowcaseUser) TableName() string { return "backtest_showcase_users" }

func NewBacktestShowcaseUserStore(db *gorm.DB) *BacktestShowcaseUserStore {
	return &BacktestShowcaseUserStore{db: db}
}

func (s *BacktestShowcaseUserStore) initTables() error {
	return s.db.AutoMigrate(&BacktestShowcaseUser{})
}

func normalizeBacktestShowcaseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *BacktestShowcaseUserStore) Upsert(email string, enabled bool) error {
	record := &BacktestShowcaseUser{
		Email:   normalizeBacktestShowcaseEmail(email),
		Enabled: enabled,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(record).Error
}

func (s *BacktestShowcaseUserStore) Delete(email string) error {
	return s.db.Where("email = ?", normalizeBacktestShowcaseEmail(email)).Delete(&BacktestShowcaseUser{}).Error
}

func (s *BacktestShowcaseUserStore) IsAllowed(email string) (bool, error) {
	var count int64
	err := s.db.Model(&BacktestShowcaseUser{}).
		Where("email = ? AND enabled = ?", normalizeBacktestShowcaseEmail(email), true).
		Count(&count).Error
	return count > 0, err
}

func (s *BacktestShowcaseUserStore) List() ([]*BacktestShowcaseUser, error) {
	var items []*BacktestShowcaseUser
	err := s.db.Order("updated_at DESC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *BacktestShowcaseUserStore) ListEnabled() ([]*BacktestShowcaseUser, error) {
	var items []*BacktestShowcaseUser
	err := s.db.Where("enabled = ?", true).Order("updated_at DESC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *BacktestShowcaseUserStore) GetByEmail(email string) (*BacktestShowcaseUser, error) {
	var item BacktestShowcaseUser
	err := s.db.Where("email = ?", normalizeBacktestShowcaseEmail(email)).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *BacktestShowcaseUserStore) ResolveEnabledUserIDs(userStore *UserStore) (map[string]struct{}, error) {
	if userStore == nil {
		return map[string]struct{}{}, nil
	}
	items, err := s.ListEnabled()
	if err != nil {
		return nil, err
	}
	owners := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		email := normalizeBacktestShowcaseEmail(item.Email)
		if email == "" {
			continue
		}
		u, getErr := userStore.GetByEmail(email)
		if getErr != nil {
			if errors.Is(getErr, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, getErr
		}
		if u == nil {
			continue
		}
		uid := strings.TrimSpace(u.ID)
		if uid == "" {
			continue
		}
		owners[uid] = struct{}{}
	}
	return owners, nil
}
