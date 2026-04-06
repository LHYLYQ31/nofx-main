package store

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MembershipPlanBronze  = "bronze"
	MembershipPlanDiamond = "diamond"
)

type MembershipPlanStore struct {
	db *gorm.DB
}

type MembershipPlan struct {
	Code            string    `gorm:"column:code;primaryKey" json:"code"`
	Name            string    `gorm:"column:name;not null;default:''" json:"name"`
	Description     string    `gorm:"column:description;not null;default:''" json:"description"`
	PriceCents      int64     `gorm:"column:price_cents;not null;default:0" json:"price_cents"`
	Currency        string    `gorm:"column:currency;not null;default:'USD'" json:"currency"`
	BillingCycle    string    `gorm:"column:billing_cycle;not null;default:'monthly'" json:"billing_cycle"`
	RevenueShareBps int       `gorm:"column:revenue_share_bps;not null;default:0" json:"revenue_share_bps"`
	SeatLimit       int       `gorm:"column:seat_limit;not null;default:0" json:"seat_limit"`
	Enabled         bool      `gorm:"column:enabled;not null;default:true;index" json:"enabled"`
	SortOrder       int       `gorm:"column:sort_order;not null;default:0" json:"sort_order"`
	Entitlements    string    `gorm:"column:entitlements;type:text;not null;default:'{}'" json:"entitlements"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (MembershipPlan) TableName() string { return "membership_plans" }

func NewMembershipPlanStore(db *gorm.DB) *MembershipPlanStore {
	return &MembershipPlanStore{db: db}
}

func (s *MembershipPlanStore) initTables() error {
	return s.db.AutoMigrate(&MembershipPlan{})
}

func (s *MembershipPlanStore) initDefaultData() error {
	defaultPlans := []*MembershipPlan{
		{
			Code:            MembershipPlanBronze,
			Name:            "Bronze",
			Description:     "Signal following and backtest insights",
			PriceCents:      9900,
			Currency:        "USD",
			BillingCycle:    "monthly",
			RevenueShareBps: 0,
			SeatLimit:       0,
			Enabled:         true,
			SortOrder:       10,
			Entitlements:    `{"signals":true,"backtest_view":true,"copy_trade":true,"managed_api":false,"priority_support":false}`,
		},
		{
			Code:            MembershipPlanDiamond,
			Name:            "Diamond",
			Description:     "API managed trading with risk controls and support",
			PriceCents:      49900,
			Currency:        "USD",
			BillingCycle:    "monthly",
			RevenueShareBps: 2000,
			SeatLimit:       50,
			Enabled:         true,
			SortOrder:       20,
			Entitlements:    `{"signals":true,"backtest_view":true,"copy_trade":true,"managed_api":true,"priority_support":true}`,
		},
	}

	for _, plan := range defaultPlans {
		if err := s.Upsert(plan); err != nil {
			return err
		}
	}
	return nil
}

func normalizePlanCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func (s *MembershipPlanStore) Upsert(plan *MembershipPlan) error {
	if plan == nil {
		return fmt.Errorf("membership plan is nil")
	}
	plan.Code = normalizePlanCode(plan.Code)
	plan.Currency = strings.ToUpper(strings.TrimSpace(plan.Currency))
	plan.BillingCycle = strings.ToLower(strings.TrimSpace(plan.BillingCycle))
	if plan.Code == "" {
		return fmt.Errorf("membership plan code is required")
	}
	if plan.Currency == "" {
		plan.Currency = "USD"
	}
	if plan.BillingCycle == "" {
		plan.BillingCycle = "monthly"
	}
	if strings.TrimSpace(plan.Entitlements) == "" {
		plan.Entitlements = "{}"
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "code"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "description", "price_cents", "currency", "billing_cycle",
			"revenue_share_bps", "seat_limit", "enabled", "sort_order", "entitlements", "updated_at",
		}),
	}).Create(plan).Error
}

func (s *MembershipPlanStore) List(enabledOnly bool) ([]*MembershipPlan, error) {
	var items []*MembershipPlan
	query := s.db.Model(&MembershipPlan{})
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("sort_order ASC, price_cents ASC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *MembershipPlanStore) GetByCode(code string) (*MembershipPlan, error) {
	var item MembershipPlan
	err := s.db.Where("code = ?", normalizePlanCode(code)).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}
