package store

import (
	"nofx/crypto"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	PaymentProviderInfini = "infini"
)

type PaymentProviderConfigStore struct {
	db *gorm.DB
}

type PaymentProviderConfig struct {
	ID            string                 `gorm:"column:id;primaryKey" json:"id"`
	Provider      string                 `gorm:"column:provider;not null;index:idx_payment_provider_config_provider" json:"provider"`
	Environment   string                 `gorm:"column:environment;not null;default:'production'" json:"environment"`
	DisplayName   string                 `gorm:"column:display_name;not null;default:''" json:"display_name"`
	BaseURL       string                 `gorm:"column:base_url;not null;default:''" json:"base_url"`
	KeyID         string                 `gorm:"column:key_id;not null;default:''" json:"key_id"`
	SecretKey     crypto.EncryptedString `gorm:"column:secret_key;type:text;not null;default:''" json:"secret_key"`
	WebhookSecret crypto.EncryptedString `gorm:"column:webhook_secret;type:text;not null;default:''" json:"webhook_secret"`
	Enabled       bool                   `gorm:"column:enabled;not null;default:true;index" json:"enabled"`
	IsDefault     bool                   `gorm:"column:is_default;not null;default:false;index" json:"is_default"`
	Version       int                    `gorm:"column:version;not null;default:1" json:"version"`
	CreatedAt     time.Time              `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time              `gorm:"column:updated_at" json:"updated_at"`
}

func (PaymentProviderConfig) TableName() string { return "payment_provider_configs" }

func NewPaymentProviderConfigStore(db *gorm.DB) *PaymentProviderConfigStore {
	return &PaymentProviderConfigStore{db: db}
}

func (s *PaymentProviderConfigStore) initTables() error {
	if err := s.db.AutoMigrate(&PaymentProviderConfig{}); err != nil {
		return err
	}
	s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_provider_config_provider_env_ver ON payment_provider_configs(provider, environment, version)`)
	return nil
}

func normalizeProvider(provider string) string {
	value := strings.ToLower(strings.TrimSpace(provider))
	if value == "" {
		return PaymentProviderInfini
	}
	return value
}

func normalizeEnvironment(env string) string {
	value := strings.ToLower(strings.TrimSpace(env))
	if value == "" {
		return "production"
	}
	return value
}

func (s *PaymentProviderConfigStore) ListByProvider(provider string) ([]*PaymentProviderConfig, error) {
	var items []*PaymentProviderConfig
	err := s.db.Where("provider = ?", normalizeProvider(provider)).
		Order("is_default DESC, enabled DESC, version DESC, updated_at DESC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *PaymentProviderConfigStore) GetByID(id string) (*PaymentProviderConfig, error) {
	var item PaymentProviderConfig
	err := s.db.Where("id = ?", strings.TrimSpace(id)).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *PaymentProviderConfigStore) GetActive(provider string) (*PaymentProviderConfig, error) {
	var item PaymentProviderConfig
	err := s.db.Where("provider = ? AND enabled = ?", normalizeProvider(provider), true).
		Order("is_default DESC, version DESC, updated_at DESC").
		First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *PaymentProviderConfigStore) Upsert(input *PaymentProviderConfig) (*PaymentProviderConfig, error) {
	if input == nil {
		return nil, gorm.ErrInvalidData
	}
	input.Provider = normalizeProvider(input.Provider)
	input.Environment = normalizeEnvironment(input.Environment)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.KeyID = strings.TrimSpace(input.KeyID)

	if input.ID == "" {
		input.ID = uuid.New().String()
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if input.Version <= 0 {
			var maxVersion int
			if qErr := tx.Model(&PaymentProviderConfig{}).
				Where("provider = ? AND environment = ?", input.Provider, input.Environment).
				Select("COALESCE(MAX(version), 0)").
				Scan(&maxVersion).Error; qErr != nil {
				return qErr
			}
			input.Version = maxVersion + 1
		}

		if input.IsDefault && input.Enabled {
			if uErr := tx.Model(&PaymentProviderConfig{}).
				Where("provider = ? AND id <> ?", input.Provider, input.ID).
				Update("is_default", false).Error; uErr != nil {
				return uErr
			}
		}

		return tx.Save(input).Error
	})
	if err != nil {
		return nil, err
	}
	return input, nil
}
