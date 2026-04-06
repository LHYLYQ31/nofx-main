package store

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	PaymentWebhookStatusPending   = "pending"
	PaymentWebhookStatusProcessed = "processed"
	PaymentWebhookStatusIgnored   = "ignored"
	PaymentWebhookStatusFailed    = "failed"
)

type PaymentWebhookEventStore struct {
	db *gorm.DB
}

type PaymentWebhookEvent struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Provider    string     `gorm:"column:provider;not null;index:idx_payment_webhook_provider_event,priority:1" json:"provider"`
	EventID     string     `gorm:"column:event_id;not null;index:idx_payment_webhook_provider_event,priority:2" json:"event_id"`
	EventType   string     `gorm:"column:event_type;not null;default:'';index" json:"event_type"`
	Signature   string     `gorm:"column:signature;not null;default:''" json:"signature"`
	Payload     string     `gorm:"column:payload;type:text;not null;default:''" json:"payload"`
	Status      string     `gorm:"column:status;not null;default:'pending';index" json:"status"`
	Result      string     `gorm:"column:result;type:text;not null;default:''" json:"result"`
	ProcessedAt *time.Time `gorm:"column:processed_at" json:"processed_at"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (PaymentWebhookEvent) TableName() string { return "payment_webhook_events" }

func NewPaymentWebhookEventStore(db *gorm.DB) *PaymentWebhookEventStore {
	return &PaymentWebhookEventStore{db: db}
}

func (s *PaymentWebhookEventStore) initTables() error {
	if err := s.db.AutoMigrate(&PaymentWebhookEvent{}); err != nil {
		return err
	}
	s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_webhook_provider_event_unique ON payment_webhook_events(provider, event_id)`)
	return nil
}

func normalizeWebhookStatus(status string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	switch value {
	case PaymentWebhookStatusPending, PaymentWebhookStatusProcessed, PaymentWebhookStatusIgnored, PaymentWebhookStatusFailed:
		return value
	default:
		return PaymentWebhookStatusPending
	}
}

func (s *PaymentWebhookEventStore) Create(event *PaymentWebhookEvent) error {
	if event == nil {
		return gorm.ErrInvalidData
	}
	event.Provider = normalizeProvider(event.Provider)
	event.EventID = strings.TrimSpace(event.EventID)
	event.Status = normalizeWebhookStatus(event.Status)
	if event.Status == "" {
		event.Status = PaymentWebhookStatusPending
	}
	return s.db.Create(event).Error
}

func (s *PaymentWebhookEventStore) GetByProviderEventID(provider, eventID string) (*PaymentWebhookEvent, error) {
	var item PaymentWebhookEvent
	err := s.db.Where("provider = ? AND event_id = ?", normalizeProvider(provider), strings.TrimSpace(eventID)).
		First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *PaymentWebhookEventStore) MarkProcessed(id int64, status, result string) error {
	now := time.Now().UTC()
	return s.db.Model(&PaymentWebhookEvent{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       normalizeWebhookStatus(status),
		"result":       strings.TrimSpace(result),
		"processed_at": now,
		"updated_at":   now,
	}).Error
}
