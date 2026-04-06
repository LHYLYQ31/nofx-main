package store

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ProfitSettlementStatusPending  = "pending"
	ProfitSettlementStatusDue      = "due"
	ProfitSettlementStatusPaid     = "paid"
	ProfitSettlementStatusCanceled = "canceled"
)

type ProfitSettlementStore struct {
	db *gorm.DB
}

type ProfitSettlement struct {
	ID               string    `gorm:"column:id;primaryKey" json:"id"`
	UserID           string    `gorm:"column:user_id;not null;index" json:"user_id"`
	MembershipID     string    `gorm:"column:membership_id;not null;default:'';index" json:"membership_id"`
	PlanCode         string    `gorm:"column:plan_code;not null;default:'';index" json:"plan_code"`
	PeriodStartAt    time.Time `gorm:"column:period_start_at;not null;index" json:"period_start_at"`
	PeriodEndAt      time.Time `gorm:"column:period_end_at;not null;index" json:"period_end_at"`
	ProfitBaseCents  int64     `gorm:"column:profit_base_cents;not null;default:0" json:"profit_base_cents"`
	HWMBeforeCents   int64     `gorm:"column:hwm_before_cents;not null;default:0" json:"hwm_before_cents"`
	HWMAfterCents    int64     `gorm:"column:hwm_after_cents;not null;default:0" json:"hwm_after_cents"`
	ShareBps         int       `gorm:"column:share_bps;not null;default:0" json:"share_bps"`
	ShareAmountCents int64     `gorm:"column:share_amount_cents;not null;default:0" json:"share_amount_cents"`
	Status           string    `gorm:"column:status;not null;default:'pending';index" json:"status"`
	PaymentOrderID   string    `gorm:"column:payment_order_id;not null;default:'';index" json:"payment_order_id"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (ProfitSettlement) TableName() string { return "profit_settlements" }

func NewProfitSettlementStore(db *gorm.DB) *ProfitSettlementStore {
	return &ProfitSettlementStore{db: db}
}

func (s *ProfitSettlementStore) initTables() error {
	return s.db.AutoMigrate(&ProfitSettlement{})
}

func normalizeSettlementStatus(status string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	switch value {
	case ProfitSettlementStatusPending, ProfitSettlementStatusDue, ProfitSettlementStatusPaid, ProfitSettlementStatusCanceled:
		return value
	default:
		return ProfitSettlementStatusPending
	}
}

func (s *ProfitSettlementStore) Create(item *ProfitSettlement) error {
	if item == nil {
		return gorm.ErrInvalidData
	}
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	item.UserID = strings.TrimSpace(item.UserID)
	item.MembershipID = strings.TrimSpace(item.MembershipID)
	item.PlanCode = normalizePlanCode(item.PlanCode)
	item.Status = normalizeSettlementStatus(item.Status)
	return s.db.Create(item).Error
}

func (s *ProfitSettlementStore) ListByUser(userID string, limit int) ([]*ProfitSettlement, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var items []*ProfitSettlement
	err := s.db.Where("user_id = ?", strings.TrimSpace(userID)).
		Order("created_at DESC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}
