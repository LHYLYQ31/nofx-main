package store

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUserStrategyPermissionStore_HasAccessWithExpiry(t *testing.T) {
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: "file::memory:?cache=shared"}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	st := NewUserStrategyPermissionStore(db)
	if err := st.initTables(); err != nil {
		t.Fatalf("init: %v", err)
	}

	expired := time.Now().UTC().Add(-time.Hour)
	active := time.Now().UTC().Add(time.Hour)
	rows := []UserStrategyPermission{
		{ID: "1", UserID: "u1", StrategyID: "s1", ExpiresAt: &active},
		{ID: "2", UserID: "u1", StrategyID: "s2", ExpiresAt: &expired},
	}
	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("create row: %v", err)
		}
	}

	has, err := st.HasAccess("u1", "s1")
	if err != nil || !has {
		t.Fatalf("expected access to s1, has=%v err=%v", has, err)
	}
	has, err = st.HasAccess("u1", "s2")
	if err != nil {
		t.Fatalf("check expired: %v", err)
	}
	if has {
		t.Fatalf("expected no access to expired strategy")
	}
}
