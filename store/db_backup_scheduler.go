package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nofx/logger"
)

const (
	dbBackupFilePrefix = "nofx-db-backup-"
	dbBackupFileSuffix = ".db"
)

// DatabaseBackupConfig controls scheduled database backups.
type DatabaseBackupConfig struct {
	Enabled       bool
	Interval      time.Duration
	BackupDir     string
	RetentionDays int
	RunOnStartup  bool
}

// StartDatabaseBackupScheduler starts a background scheduler and returns a stop function.
// Backup is currently supported for SQLite only. Unsupported DBs are skipped with a warning.
func StartDatabaseBackupScheduler(st *Store, dbPath string, cfg DatabaseBackupConfig) func() {
	if st == nil || !cfg.Enabled {
		return func() {}
	}

	if cfg.Interval <= 0 {
		cfg.Interval = time.Hour
	}

	backupDir := strings.TrimSpace(cfg.BackupDir)
	if backupDir == "" {
		backupDir = filepath.Join("data", "backups")
	}
	backupDir = filepath.Clean(backupDir)

	if st.DBType() != DBTypeSQLite {
		logger.Warnf("Database backup enabled but DB_TYPE=%s is not supported by scheduler yet; skipping", st.DBType())
		return func() {}
	}

	if strings.TrimSpace(dbPath) == "" {
		logger.Warn("Database backup enabled but DB path is empty; skipping")
		return func() {}
	}

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		logger.Errorf("Failed to create backup directory %s: %v", backupDir, err)
		return func() {}
	}

	logger.Infof(
		"Database backup scheduler started (interval=%s, dir=%s, retention_days=%d, run_on_startup=%t)",
		cfg.Interval,
		backupDir,
		cfg.RetentionDays,
		cfg.RunOnStartup,
	)

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	var runMu sync.Mutex
	var stopOnce sync.Once

	runOnce := func(trigger string) {
		runMu.Lock()
		defer runMu.Unlock()

		startedAt := time.Now()
		path, err := backupSQLiteDatabase(st, dbPath, backupDir)
		if err != nil {
			logger.Errorf("Database backup failed (%s): %v", trigger, err)
			return
		}

		removed, cleanupErr := cleanupOldBackupFiles(backupDir, cfg.RetentionDays)
		if cleanupErr != nil {
			logger.Warnf("Backup cleanup finished with errors: %v", cleanupErr)
		}
		logger.Infof(
			"Database backup completed (%s): file=%s elapsed=%s removed_old=%d",
			trigger,
			path,
			time.Since(startedAt).Round(time.Millisecond),
			removed,
		)
	}

	go func() {
		defer close(doneCh)
		if cfg.RunOnStartup {
			runOnce("startup")
		}

		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				runOnce("scheduled")
			}
		}
	}()

	return func() {
		stopOnce.Do(func() {
			close(stopCh)
		})

		select {
		case <-doneCh:
			logger.Info("Database backup scheduler stopped")
		case <-time.After(5 * time.Second):
			logger.Warn("Timed out while stopping database backup scheduler")
		}
	}
}

func backupSQLiteDatabase(st *Store, dbPath, backupDir string) (string, error) {
	if st == nil || st.GormDB() == nil {
		return "", fmt.Errorf("database connection is nil")
	}

	srcPath := filepath.Clean(dbPath)
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("source db not found: %w", err)
	}

	ts := time.Now().UTC().Format("20060102-150405")
	dstPath := filepath.Join(backupDir, dbBackupFilePrefix+ts+dbBackupFileSuffix)
	// VACUUM INTO requires destination file does not exist.
	_ = os.Remove(dstPath)

	_ = st.GormDB().Exec("PRAGMA wal_checkpoint(FULL)").Error

	sql := fmt.Sprintf("VACUUM INTO '%s';", escapeSQLiteStringLiteral(filepath.Clean(dstPath)))
	if err := st.GormDB().Exec(sql).Error; err != nil {
		return "", err
	}

	return dstPath, nil
}

func cleanupOldBackupFiles(backupDir string, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	removed := 0
	var firstErr error

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, dbBackupFilePrefix) || !strings.HasSuffix(name, dbBackupFileSuffix) {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			if firstErr == nil {
				firstErr = infoErr
			}
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}

		path := filepath.Join(backupDir, name)
		if removeErr := os.Remove(path); removeErr != nil {
			if firstErr == nil {
				firstErr = removeErr
			}
			continue
		}
		removed++
	}

	return removed, firstErr
}

func escapeSQLiteStringLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
