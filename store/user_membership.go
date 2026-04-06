package store

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	MembershipStatusActive   = "active"
	MembershipStatusExpired  = "expired"
	MembershipStatusCanceled = "canceled"
)

type UserMembershipStore struct {
	db *gorm.DB
}

type UserMembership struct {
	ID                 string    `gorm:"column:id;primaryKey" json:"id"`
	UserID             string    `gorm:"column:user_id;not null;index:idx_user_memberships_user_status" json:"user_id"`
	PlanCode           string    `gorm:"column:plan_code;not null;index" json:"plan_code"`
	Status             string    `gorm:"column:status;not null;default:'active';index:idx_user_memberships_user_status" json:"status"`
	StartAt            time.Time `gorm:"column:start_at;not null" json:"start_at"`
	EndAt              time.Time `gorm:"column:end_at;not null;index" json:"end_at"`
	AutoRenew          bool      `gorm:"column:auto_renew;not null;default:true" json:"auto_renew"`
	HighWatermark      float64   `gorm:"column:high_watermark;not null;default:0" json:"high_watermark"`
	LastPaymentOrderID string    `gorm:"column:last_payment_order_id;not null;default:''" json:"last_payment_order_id"`
	CreatedAt          time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (UserMembership) TableName() string { return "user_memberships" }

func NewUserMembershipStore(db *gorm.DB) *UserMembershipStore {
	return &UserMembershipStore{db: db}
}

func (s *UserMembershipStore) initTables() error {
	return s.db.AutoMigrate(&UserMembership{})
}

func normalizeMembershipStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case MembershipStatusActive, MembershipStatusExpired, MembershipStatusCanceled:
		return status
	default:
		return MembershipStatusActive
	}
}

func (s *UserMembershipStore) GetActiveByUser(userID string, at time.Time) (*UserMembership, error) {
	var item UserMembership
	err := s.db.Where("user_id = ? AND status = ? AND end_at > ?", strings.TrimSpace(userID), MembershipStatusActive, at.UTC()).
		Order("end_at DESC").First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *UserMembershipStore) GetLatestByUser(userID string) (*UserMembership, error) {
	var item UserMembership
	err := s.db.Where("user_id = ?", strings.TrimSpace(userID)).Order("end_at DESC, updated_at DESC").First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *UserMembershipStore) ListByUser(userID string, limit int) ([]*UserMembership, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var items []*UserMembership
	err := s.db.Where("user_id = ?", strings.TrimSpace(userID)).
		Order("created_at DESC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *UserMembershipStore) Save(item *UserMembership) error {
	if item == nil {
		return gorm.ErrInvalidData
	}
	item.UserID = strings.TrimSpace(item.UserID)
	item.PlanCode = normalizePlanCode(item.PlanCode)
	item.Status = normalizeMembershipStatus(item.Status)
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	return s.db.Save(item).Error
}

func (s *UserMembershipStore) ActivateByPayment(tx *gorm.DB, userID, planCode, orderID, billingCycle string, now time.Time) (*UserMembership, error) {
	db := s.db
	if tx != nil {
		db = tx
	}
	userID = strings.TrimSpace(userID)
	planCode = normalizePlanCode(planCode)
	if userID == "" || planCode == "" {
		return nil, gorm.ErrInvalidData
	}
	now = now.UTC()

	var active UserMembership
	err := db.Where("user_id = ? AND status = ? AND end_at > ?", userID, MembershipStatusActive, now).
		Order("end_at DESC").
		First(&active).Error

	if err == nil {
		startAt := active.StartAt
		endAt := active.EndAt
		if planCode != active.PlanCode {
			startAt = now
			endAt = now
		}
		endAt = nextCycleEnd(endAt, billingCycle)
		active.PlanCode = planCode
		active.StartAt = startAt
		active.EndAt = endAt
		active.Status = MembershipStatusActive
		active.AutoRenew = true
		active.LastPaymentOrderID = strings.TrimSpace(orderID)
		if updateErr := db.Save(&active).Error; updateErr != nil {
			return nil, updateErr
		}
		return &active, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	created := &UserMembership{
		ID:                 uuid.New().String(),
		UserID:             userID,
		PlanCode:           planCode,
		Status:             MembershipStatusActive,
		StartAt:            now,
		EndAt:              nextCycleEnd(now, billingCycle),
		AutoRenew:          true,
		LastPaymentOrderID: strings.TrimSpace(orderID),
	}
	if createErr := db.Create(created).Error; createErr != nil {
		return nil, createErr
	}
	return created, nil
}

func nextCycleEnd(from time.Time, billingCycle string) time.Time {
	switch strings.ToLower(strings.TrimSpace(billingCycle)) {
	case "yearly":
		return from.AddDate(1, 0, 0)
	case "weekly":
		return from.AddDate(0, 0, 7)
	default:
		return from.AddDate(0, 1, 0)
	}
}
