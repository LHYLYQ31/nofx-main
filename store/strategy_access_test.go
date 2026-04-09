package store

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStrategyStore_GetAccessible_AdminCanAccessOtherAdminsStrategy(t *testing.T) {
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: "file:strategy_access_admin?mode=memory&cache=shared"}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	userStore := NewUserStore(db)
	if err := userStore.initTables(); err != nil {
		t.Fatalf("init users: %v", err)
	}
	strategyStore := NewStrategyStore(db)
	if err := strategyStore.initTables(); err != nil {
		t.Fatalf("init strategies: %v", err)
	}
	permStore := NewUserStrategyPermissionStore(db)
	if err := permStore.initTables(); err != nil {
		t.Fatalf("init permissions: %v", err)
	}

	if err := db.Create(&User{ID: "admin_a", Email: "a@example.com", Role: "ADMIN", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("create admin_a: %v", err)
	}
	if err := db.Create(&User{ID: "admin_b", Email: "b@example.com", Role: "ADMIN", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("create admin_b: %v", err)
	}
	if err := db.Create(&Strategy{
		ID:          "s_b",
		UserID:      "admin_b",
		Name:        "B Strategy",
		Description: "",
		Config:      "{}",
	}).Error; err != nil {
		t.Fatalf("create strategy: %v", err)
	}

	got, err := strategyStore.GetAccessible("admin_a", "s_b")
	if err != nil {
		t.Fatalf("GetAccessible failed for admin user: %v", err)
	}
	if got == nil || got.ID != "s_b" {
		t.Fatalf("unexpected strategy result: %+v", got)
	}
}

func TestStrategyStore_GetAccessible_NonAdminCannotAccessWithoutGrant(t *testing.T) {
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: "file:strategy_access_nonadmin?mode=memory&cache=shared"}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	userStore := NewUserStore(db)
	if err := userStore.initTables(); err != nil {
		t.Fatalf("init users: %v", err)
	}
	strategyStore := NewStrategyStore(db)
	if err := strategyStore.initTables(); err != nil {
		t.Fatalf("init strategies: %v", err)
	}
	permStore := NewUserStrategyPermissionStore(db)
	if err := permStore.initTables(); err != nil {
		t.Fatalf("init permissions: %v", err)
	}

	if err := db.Create(&User{ID: "user_a", Email: "user@example.com", Role: "USER", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("create user_a: %v", err)
	}
	if err := db.Create(&User{ID: "admin_b", Email: "b2@example.com", Role: "ADMIN", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("create admin_b: %v", err)
	}
	if err := db.Create(&Strategy{
		ID:          "s_b2",
		UserID:      "admin_b",
		Name:        "B Strategy 2",
		Description: "",
		Config:      "{}",
	}).Error; err != nil {
		t.Fatalf("create strategy: %v", err)
	}

	got, err := strategyStore.GetAccessible("user_a", "s_b2")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got strategy=%+v err=%v", got, err)
	}
}
