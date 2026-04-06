package store

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	PaymentBizMembership = "membership"
)

const (
	PaymentOrderStatusPending  = "pending"
	PaymentOrderStatusCreated  = "created"
	PaymentOrderStatusPaid     = "paid"
	PaymentOrderStatusExpired  = "expired"
	PaymentOrderStatusFailed   = "failed"
	PaymentOrderStatusCanceled = "canceled"
	PaymentOrderStatusRefunded = "refunded"
	PaymentOrderStatusPartial  = "partial_paid"
	PaymentOrderStatusUnknown  = "unknown"
)

type PaymentOrderStore struct {
	db *gorm.DB
}

type PaymentOrder struct {
	ID                    string     `gorm:"column:id;primaryKey" json:"id"`
	UserID                string     `gorm:"column:user_id;not null;index" json:"user_id"`
	Provider              string     `gorm:"column:provider;not null;index" json:"provider"`
	ProviderConfigID      string     `gorm:"column:provider_config_id;not null;default:'';index" json:"provider_config_id"`
	ProviderConfigVersion int        `gorm:"column:provider_config_version;not null;default:0" json:"provider_config_version"`
	RequestID             string     `gorm:"column:request_id;not null;uniqueIndex" json:"request_id"`
	MerchantOrderID       string     `gorm:"column:merchant_order_id;not null;uniqueIndex" json:"merchant_order_id"`
	ProviderOrderID       string     `gorm:"column:provider_order_id;not null;default:'';index" json:"provider_order_id"`
	BizType               string     `gorm:"column:biz_type;not null;default:'membership';index" json:"biz_type"`
	PlanCode              string     `gorm:"column:plan_code;not null;default:'';index" json:"plan_code"`
	AmountCents           int64      `gorm:"column:amount_cents;not null;default:0" json:"amount_cents"`
	Currency              string     `gorm:"column:currency;not null;default:'USD'" json:"currency"`
	Status                string     `gorm:"column:status;not null;default:'pending';index" json:"status"`
	CheckoutURL           string     `gorm:"column:checkout_url;not null;default:''" json:"checkout_url"`
	ClientReturnURL       string     `gorm:"column:client_return_url;not null;default:''" json:"client_return_url"`
	RawCreateResponse     string     `gorm:"column:raw_create_response;type:text;not null;default:''" json:"raw_create_response"`
	RawLastQuery          string     `gorm:"column:raw_last_query;type:text;not null;default:''" json:"raw_last_query"`
	PaidAt                *time.Time `gorm:"column:paid_at" json:"paid_at"`
	ExpiresAt             *time.Time `gorm:"column:expires_at" json:"expires_at"`
	CreatedAt             time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (PaymentOrder) TableName() string { return "payment_orders" }

func NewPaymentOrderStore(db *gorm.DB) *PaymentOrderStore {
	return &PaymentOrderStore{db: db}
}

func (s *PaymentOrderStore) initTables() error {
	if err := s.db.AutoMigrate(&PaymentOrder{}); err != nil {
		return err
	}
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_payment_orders_user_created ON payment_orders(user_id, created_at DESC)`)
	return nil
}

func normalizePaymentStatus(status string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	switch value {
	case PaymentOrderStatusPending, PaymentOrderStatusCreated, PaymentOrderStatusPaid, PaymentOrderStatusExpired,
		PaymentOrderStatusFailed, PaymentOrderStatusCanceled, PaymentOrderStatusRefunded, PaymentOrderStatusPartial:
		return value
	default:
		return PaymentOrderStatusUnknown
	}
}

func (s *PaymentOrderStore) Create(order *PaymentOrder) error {
	if order == nil {
		return gorm.ErrInvalidData
	}
	if order.ID == "" {
		order.ID = uuid.New().String()
	}
	order.Provider = normalizeProvider(order.Provider)
	order.RequestID = strings.TrimSpace(order.RequestID)
	order.MerchantOrderID = strings.TrimSpace(order.MerchantOrderID)
	order.Currency = strings.ToUpper(strings.TrimSpace(order.Currency))
	order.BizType = strings.ToLower(strings.TrimSpace(order.BizType))
	order.PlanCode = normalizePlanCode(order.PlanCode)
	order.Status = normalizePaymentStatus(order.Status)
	if order.Currency == "" {
		order.Currency = "USD"
	}
	if order.BizType == "" {
		order.BizType = PaymentBizMembership
	}
	if order.Status == PaymentOrderStatusUnknown {
		order.Status = PaymentOrderStatusPending
	}
	return s.db.Create(order).Error
}

func (s *PaymentOrderStore) GetByID(orderID string) (*PaymentOrder, error) {
	var item PaymentOrder
	err := s.db.Where("id = ?", strings.TrimSpace(orderID)).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *PaymentOrderStore) GetByIDAndUser(orderID, userID string) (*PaymentOrder, error) {
	var item PaymentOrder
	err := s.db.Where("id = ? AND user_id = ?", strings.TrimSpace(orderID), strings.TrimSpace(userID)).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *PaymentOrderStore) GetByMerchantOrderID(merchantOrderID string) (*PaymentOrder, error) {
	var item PaymentOrder
	err := s.db.Where("merchant_order_id = ?", strings.TrimSpace(merchantOrderID)).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *PaymentOrderStore) GetByProviderOrderID(provider, providerOrderID string) (*PaymentOrder, error) {
	var item PaymentOrder
	err := s.db.Where("provider = ? AND provider_order_id = ?", normalizeProvider(provider), strings.TrimSpace(providerOrderID)).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *PaymentOrderStore) ListByUser(userID string, limit int) ([]*PaymentOrder, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var items []*PaymentOrder
	err := s.db.Where("user_id = ?", strings.TrimSpace(userID)).
		Order("created_at DESC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *PaymentOrderStore) UpdateCreateResponse(orderID, providerOrderID, checkoutURL, status, rawResponse string, expiresAt *time.Time) error {
	updates := map[string]interface{}{
		"provider_order_id":   strings.TrimSpace(providerOrderID),
		"checkout_url":        strings.TrimSpace(checkoutURL),
		"status":              normalizePaymentStatus(status),
		"raw_create_response": strings.TrimSpace(rawResponse),
		"updated_at":          time.Now().UTC(),
	}
	if expiresAt != nil {
		updates["expires_at"] = expiresAt.UTC()
	}
	return s.db.Model(&PaymentOrder{}).Where("id = ?", strings.TrimSpace(orderID)).Updates(updates).Error
}

func (s *PaymentOrderStore) UpdateStatus(orderID, status, rawLastQuery string) error {
	updates := map[string]interface{}{
		"status":         normalizePaymentStatus(status),
		"raw_last_query": strings.TrimSpace(rawLastQuery),
		"updated_at":     time.Now().UTC(),
	}
	if normalizePaymentStatus(status) == PaymentOrderStatusPaid {
		now := time.Now().UTC()
		updates["paid_at"] = now
	}
	return s.db.Model(&PaymentOrder{}).Where("id = ?", strings.TrimSpace(orderID)).Updates(updates).Error
}
