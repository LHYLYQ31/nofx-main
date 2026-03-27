package store

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type StrategyShowcaseStore struct {
	db *gorm.DB
}

type StrategyShowcase struct {
	StrategyID string    `gorm:"column:strategy_id;primaryKey" json:"strategy_id"`
	SortOrder  int       `gorm:"column:sort_order;not null;default:0;index" json:"sort_order"`
	Enabled    bool      `gorm:"column:enabled;not null;default:true;index" json:"enabled"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (StrategyShowcase) TableName() string { return "strategy_showcase" }

func NewStrategyShowcaseStore(db *gorm.DB) *StrategyShowcaseStore {
	return &StrategyShowcaseStore{db: db}
}

func (s *StrategyShowcaseStore) initTables() error {
	return s.db.AutoMigrate(&StrategyShowcase{})
}

func (s *StrategyShowcaseStore) List() ([]*StrategyShowcase, error) {
	var items []*StrategyShowcase
	err := s.db.
		Where("enabled = ?", true).
		Order("sort_order ASC, updated_at DESC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *StrategyShowcaseStore) ReplaceStrategyIDs(strategyIDs []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&StrategyShowcase{}).Error; err != nil {
			return err
		}
		order := 0
		seen := map[string]struct{}{}
		for _, raw := range strategyIDs {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			item := &StrategyShowcase{
				StrategyID: id,
				SortOrder:  order,
				Enabled:    true,
			}
			if err := tx.Create(item).Error; err != nil {
				return err
			}
			order++
		}
		return nil
	})
}
