package store

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// UserStore user storage
type UserStore struct {
	db *gorm.DB
}

// User user model
type User struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Email        string    `gorm:"uniqueIndex:idx_users_email;not null" json:"email"`
	Role         string    `gorm:"column:role;not null;default:'USER';index" json:"role"`
	PasswordHash string    `gorm:"column:password_hash;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string { return "users" }

// NewUserStore creates a new UserStore
func NewUserStore(db *gorm.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) initTables() error {
	// For PostgreSQL with existing table, skip AutoMigrate to avoid index conflicts
	if s.db.Dialector.Name() == "postgres" {
		var tableExists int64
		s.db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'users'`).Scan(&tableExists)

		if tableExists > 0 {
			// Table exists - manually ensure all columns exist
			// Core columns (should already exist)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'USER'`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)

			// Ensure unique index exists on email (don't care about the name)
			var indexExists int64
			s.db.Raw(`
				SELECT COUNT(*) FROM pg_indexes
				WHERE tablename = 'users' AND indexdef LIKE '%email%' AND indexdef LIKE '%UNIQUE%'
			`).Scan(&indexExists)

			if indexExists == 0 {
				s.db.Exec("CREATE UNIQUE INDEX idx_users_email ON users(email)")
			}

			return nil
		}
	}
	return s.db.AutoMigrate(&User{})
}

func normalizeRole(role string) string {
	if role == "ADMIN" {
		return "ADMIN"
	}
	return "USER"
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Create creates user
func (s *UserStore) Create(user *User) error {
	user.Email = normalizeEmail(user.Email)
	user.Role = normalizeRole(user.Role)
	return s.db.Create(user).Error
}

// GetByEmail gets user by email
func (s *UserStore) GetByEmail(email string) (*User, error) {
	var user User
	err := s.db.Where("email = ?", normalizeEmail(email)).First(&user).Error
	if err != nil {
		return nil, err
	}
	user.Role = normalizeRole(user.Role)
	return &user, nil
}

// GetByID gets user by ID
func (s *UserStore) GetByID(userID string) (*User, error) {
	var user User
	err := s.db.Where("id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	user.Role = normalizeRole(user.Role)
	return &user, nil
}

// Count returns the total number of users
func (s *UserStore) Count() (int, error) {
	var count int64
	err := s.db.Model(&User{}).Count(&count).Error
	return int(count), err
}

// CountByRole returns the number of users by role.
func (s *UserStore) CountByRole(role string) (int, error) {
	var count int64
	err := s.db.Model(&User{}).Where("role = ?", normalizeRole(role)).Count(&count).Error
	return int(count), err
}

// GetAllIDs gets all user IDs
func (s *UserStore) GetAllIDs() ([]string, error) {
	var userIDs []string
	err := s.db.Model(&User{}).Order("id").Pluck("id", &userIDs).Error
	return userIDs, err
}

// List gets all users.
func (s *UserStore) List() ([]*User, error) {
	var users []*User
	err := s.db.Order("created_at DESC").Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

// UpdatePassword updates password
func (s *UserStore) UpdatePassword(userID, passwordHash string) error {
	return s.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"password_hash": passwordHash,
		"updated_at":    time.Now().UTC(),
	}).Error
}

// UpdateRole updates a user's role.
func (s *UserStore) UpdateRole(userID, role string) error {
	return s.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"role":       normalizeRole(role),
		"updated_at": time.Now().UTC(),
	}).Error
}

// EnsureAdmin ensures admin user exists
func (s *UserStore) EnsureAdmin() error {
	return s.EnsureBootstrapAdmin("admin@example.com", "", func() string { return "admin" })
}

// EnsureBootstrapAdmin ensures the bootstrap admin account exists and has admin role.
func (s *UserStore) EnsureBootstrapAdmin(email, passwordHash string, idFactory func() string) error {
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		normalizedEmail = "admin@example.com"
	}

	var existing User
	err := s.db.Where("email = ?", normalizedEmail).First(&existing).Error
	if err == nil {
		updates := map[string]interface{}{
			"updated_at": time.Now().UTC(),
		}
		needUpdate := false

		if normalizeRole(existing.Role) != "ADMIN" {
			updates["role"] = "ADMIN"
			needUpdate = true
		}

		// If bootstrap password is provided, always sync it on startup.
		// This ensures NOFX_SUPER_ADMIN_PASSWORD takes effect on every deployment.
		if strings.TrimSpace(passwordHash) != "" && existing.PasswordHash != passwordHash {
			updates["password_hash"] = passwordHash
			needUpdate = true
		}

		if !needUpdate {
			return nil
		}

		return s.db.Model(&User{}).Where("id = ?", existing.ID).Updates(updates).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}

	userID := "admin"
	if idFactory != nil {
		if generated := strings.TrimSpace(idFactory()); generated != "" {
			userID = generated
		}
	}

	return s.Create(&User{
		ID:           userID,
		Email:        normalizedEmail,
		Role:         "ADMIN",
		PasswordHash: passwordHash,
	})
}
