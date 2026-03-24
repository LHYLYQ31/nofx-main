package store

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StrategyWebhookStore struct {
	db *gorm.DB
}

type StrategyWebhook struct {
	StrategyID string    `gorm:"column:strategy_id;primaryKey" json:"strategy_id"`
	WebhookURL string    `gorm:"column:webhook_url;not null;default:''" json:"webhook_url"`
	Enabled    bool      `gorm:"column:enabled;not null;default:true;index" json:"enabled"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (StrategyWebhook) TableName() string { return "strategy_discord_webhooks" }

func NewStrategyWebhookStore(db *gorm.DB) *StrategyWebhookStore {
	return &StrategyWebhookStore{db: db}
}

func (s *StrategyWebhookStore) initTables() error {
	return s.db.AutoMigrate(&StrategyWebhook{})
}

func normalizeWebhookURL(url string) string {
	return strings.TrimSpace(url)
}

func (s *StrategyWebhookStore) Upsert(strategyID, webhookURL string, enabled bool) error {
	record := &StrategyWebhook{
		StrategyID: strings.TrimSpace(strategyID),
		WebhookURL: normalizeWebhookURL(webhookURL),
		Enabled:    enabled,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "strategy_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"webhook_url", "enabled", "updated_at"}),
	}).Create(record).Error
}

func (s *StrategyWebhookStore) Delete(strategyID string) error {
	return s.db.Where("strategy_id = ?", strings.TrimSpace(strategyID)).Delete(&StrategyWebhook{}).Error
}

func (s *StrategyWebhookStore) GetByStrategyID(strategyID string) (*StrategyWebhook, error) {
	var item StrategyWebhook
	err := s.db.Where("strategy_id = ?", strings.TrimSpace(strategyID)).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *StrategyWebhookStore) List() ([]*StrategyWebhook, error) {
	var items []*StrategyWebhook
	err := s.db.Order("updated_at DESC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

type SignalNotifyUserStore struct {
	db *gorm.DB
}

type SignalNotifyUser struct {
	Email     string    `gorm:"column:email;primaryKey" json:"email"`
	Enabled   bool      `gorm:"column:enabled;not null;default:true;index" json:"enabled"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (SignalNotifyUser) TableName() string { return "signal_notify_users" }

func NewSignalNotifyUserStore(db *gorm.DB) *SignalNotifyUserStore {
	return &SignalNotifyUserStore{db: db}
}

func (s *SignalNotifyUserStore) initTables() error {
	return s.db.AutoMigrate(&SignalNotifyUser{})
}

func normalizeSignalEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *SignalNotifyUserStore) Upsert(email string, enabled bool) error {
	record := &SignalNotifyUser{
		Email:   normalizeSignalEmail(email),
		Enabled: enabled,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(record).Error
}

func (s *SignalNotifyUserStore) Delete(email string) error {
	return s.db.Where("email = ?", normalizeSignalEmail(email)).Delete(&SignalNotifyUser{}).Error
}

func (s *SignalNotifyUserStore) IsAllowed(email string) (bool, error) {
	var count int64
	err := s.db.Model(&SignalNotifyUser{}).
		Where("email = ? AND enabled = ?", normalizeSignalEmail(email), true).
		Count(&count).Error
	return count > 0, err
}

func (s *SignalNotifyUserStore) List() ([]*SignalNotifyUser, error) {
	var items []*SignalNotifyUser
	err := s.db.Order("updated_at DESC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

// ResolveSignalWebhook resolves the final webhook URL for a strategy signal by
// checking both notify-user allowlist and strategy webhook mapping.
func (s *Store) ResolveSignalWebhook(strategyID, userEmail string) (string, bool, error) {
	if s == nil {
		return "", false, nil
	}
	strategyID = strings.TrimSpace(strategyID)
	userEmail = normalizeSignalEmail(userEmail)
	if strategyID == "" || userEmail == "" {
		return "", false, nil
	}

	allowed, err := s.SignalNotifyUser().IsAllowed(userEmail)
	if err != nil || !allowed {
		return "", false, err
	}

	item, err := s.StrategyWebhook().GetByStrategyID(strategyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if item == nil || !item.Enabled || strings.TrimSpace(item.WebhookURL) == "" {
		return "", false, nil
	}
	return strings.TrimSpace(item.WebhookURL), true, nil
}
