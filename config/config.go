package config

import (
	"nofx/experience"
	"nofx/mcp"
	"os"
	"strconv"
	"strings"
)

// Global configuration instance
var global *Config

// Config is the global configuration (loaded from .env)
// Only contains truly global config, trading related config is at trader/strategy level
type Config struct {
	// Service configuration
	APIServerPort       int
	JWTSecret           string
	RegistrationEnabled bool
	MaxUsers            int // Maximum number of users allowed (0 = unlimited, default = 10)

	// Database configuration
	DBType     string // sqlite or postgres
	DBPath     string // SQLite database file path
	DBHost     string // PostgreSQL host
	DBPort     int    // PostgreSQL port
	DBUser     string // PostgreSQL user
	DBPassword string // PostgreSQL password
	DBName     string // PostgreSQL database name
	DBSSLMode  string // PostgreSQL SSL mode
	// Database backup configuration
	DBBackupEnabled         bool
	DBBackupIntervalMinutes int
	DBBackupDir             string
	DBBackupRetentionDays   int
	DBBackupOnStartup       bool

	// Security configuration
	// TransportEncryption enables browser-side encryption for API keys
	// Requires HTTPS or localhost. Set to false for HTTP access via IP.
	TransportEncryption bool

	// Experience improvement (anonymous usage statistics)
	// Helps us understand product usage and improve the experience
	// Set EXPERIENCE_IMPROVEMENT=false to disable
	ExperienceImprovement bool

	// Market data provider API keys
	AlpacaAPIKey    string // Alpaca API key for US stocks
	AlpacaSecretKey string // Alpaca secret key
	TwelveDataKey   string // TwelveData API key for forex & metals

	// Backtest showcase owner email. When set, all authenticated users can view this account's backtest runs.
	BacktestShowcaseEmail string
	// Deprecated fallback: showcase owner user_id.
	BacktestShowcaseUserID string
	// Backtest Discord webhook. When configured, selected users' backtest trades can be pushed to Discord.
	BacktestDiscordWebhookURL string
	// Comma-separated emails allowed to send backtest trade notifications.
	BacktestDiscordNotifyEmails string
	// Optional sender name shown in Discord webhook messages.
	BacktestDiscordUsername string

	// SMTP config (for password reset verification email)
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
}

// Init initializes global configuration (from .env)
func Init() {
	cfg := &Config{
		APIServerPort:         8080,
		RegistrationEnabled:   true,
		MaxUsers:              10,   // Default: 10 users allowed
		ExperienceImprovement: true, // Default: enabled to help improve the product
		// Database defaults
		DBType:    "sqlite",
		DBPath:    "data/data.db",
		DBHost:    "localhost",
		DBPort:    5432,
		DBUser:    "postgres",
		DBName:    "nofx",
		DBSSLMode: "disable",
		// Backup defaults (effective for SQLite; PostgreSQL logs a warning and skips)
		DBBackupEnabled:         true,
		DBBackupIntervalMinutes: 60,
		DBBackupDir:             "data/backups",
		DBBackupRetentionDays:   7,
		DBBackupOnStartup:       true,
		SMTPPort:                587,
	}

	// Load from environment variables
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = strings.TrimSpace(v)
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "default-jwt-secret-change-in-production"
	}

	if v := os.Getenv("REGISTRATION_ENABLED"); v != "" {
		cfg.RegistrationEnabled = strings.ToLower(v) == "true"
	}

	if v := os.Getenv("MAX_USERS"); v != "" {
		if maxUsers, err := strconv.Atoi(v); err == nil && maxUsers >= 0 {
			cfg.MaxUsers = maxUsers
		}
	}

	if v := os.Getenv("API_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.APIServerPort = port
		}
	}

	// Transport encryption: default false for easier deployment
	// Set TRANSPORT_ENCRYPTION=true to enable (requires HTTPS or localhost)
	if v := os.Getenv("TRANSPORT_ENCRYPTION"); v != "" {
		cfg.TransportEncryption = strings.ToLower(v) == "true"
	}

	// Experience improvement: anonymous usage statistics
	// Default enabled, set EXPERIENCE_IMPROVEMENT=false to disable
	if v := os.Getenv("EXPERIENCE_IMPROVEMENT"); v != "" {
		cfg.ExperienceImprovement = strings.ToLower(v) != "false"
	}

	// Market data provider API keys
	cfg.AlpacaAPIKey = os.Getenv("ALPACA_API_KEY")
	cfg.AlpacaSecretKey = os.Getenv("ALPACA_SECRET_KEY")
	cfg.TwelveDataKey = os.Getenv("TWELVEDATA_API_KEY")
	cfg.BacktestShowcaseEmail = strings.ToLower(strings.TrimSpace(os.Getenv("NOFX_BACKTEST_SHOWCASE_EMAIL")))
	cfg.BacktestShowcaseUserID = strings.TrimSpace(os.Getenv("NOFX_BACKTEST_SHOWCASE_USER_ID"))
	cfg.BacktestDiscordWebhookURL = strings.TrimSpace(os.Getenv("NOFX_BACKTEST_DISCORD_WEBHOOK_URL"))
	cfg.BacktestDiscordNotifyEmails = strings.ToLower(strings.TrimSpace(os.Getenv("NOFX_BACKTEST_DISCORD_NOTIFY_EMAILS")))
	cfg.BacktestDiscordUsername = strings.TrimSpace(os.Getenv("NOFX_BACKTEST_DISCORD_USERNAME"))
	if cfg.BacktestDiscordUsername == "" {
		cfg.BacktestDiscordUsername = "newmoneyclub"
	}
	cfg.SMTPHost = strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.SMTPPort = port
		}
	}
	cfg.SMTPUser = strings.TrimSpace(os.Getenv("SMTP_USER"))
	cfg.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	cfg.SMTPFrom = strings.TrimSpace(os.Getenv("SMTP_FROM"))

	// Database configuration
	if v := os.Getenv("DB_TYPE"); v != "" {
		cfg.DBType = strings.ToLower(v)
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.DBHost = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.DBPort = port
		}
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.DBUser = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.DBPassword = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.DBName = v
	}
	if v := os.Getenv("DB_SSLMODE"); v != "" {
		cfg.DBSSLMode = v
	}
	if v := os.Getenv("DB_BACKUP_ENABLED"); v != "" {
		cfg.DBBackupEnabled = strings.ToLower(v) != "false"
	}
	if v := os.Getenv("DB_BACKUP_INTERVAL_MINUTES"); v != "" {
		if mins, err := strconv.Atoi(v); err == nil && mins > 0 {
			cfg.DBBackupIntervalMinutes = mins
		}
	}
	if v := strings.TrimSpace(os.Getenv("DB_BACKUP_DIR")); v != "" {
		cfg.DBBackupDir = v
	}
	if v := os.Getenv("DB_BACKUP_RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days >= 0 {
			cfg.DBBackupRetentionDays = days
		}
	}
	if v := os.Getenv("DB_BACKUP_ON_STARTUP"); v != "" {
		cfg.DBBackupOnStartup = strings.ToLower(v) != "false"
	}

	global = cfg

	// Initialize experience improvement (installation ID will be set after database init)
	experience.Init(cfg.ExperienceImprovement, "")

	// Set up AI token usage tracking callback
	mcp.TokenUsageCallback = func(usage mcp.TokenUsage) {
		experience.TrackAIUsage(experience.AIUsageEvent{
			ModelProvider: usage.Provider,
			ModelName:     usage.Model,
			InputTokens:   usage.PromptTokens,
			OutputTokens:  usage.CompletionTokens,
		})
	}
}

// Get returns the global configuration
func Get() *Config {
	if global == nil {
		Init()
	}
	return global
}

// IsBacktestDiscordEnabled returns true when webhook + notify user list are configured.
func (c *Config) IsBacktestDiscordEnabled() bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.BacktestDiscordWebhookURL) != "" &&
		strings.TrimSpace(c.BacktestDiscordNotifyEmails) != ""
}

// IsBacktestDiscordEmailAllowed checks whether an email is in NOFX_BACKTEST_DISCORD_NOTIFY_EMAILS.
func (c *Config) IsBacktestDiscordEmailAllowed(email string) bool {
	if c == nil {
		return false
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	allowed := strings.ToLower(strings.TrimSpace(c.BacktestDiscordNotifyEmails))
	if allowed == "" {
		return false
	}
	for _, raw := range strings.Split(allowed, ",") {
		if strings.ToLower(strings.TrimSpace(raw)) == email {
			return true
		}
	}
	return false
}
