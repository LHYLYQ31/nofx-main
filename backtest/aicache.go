package backtest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"nofx/kernel"
	"nofx/market"
)

type cachedDecision struct {
	Key           string               `json:"key"`
	PromptVariant string               `json:"prompt_variant"`
	Timestamp     int64                `json:"ts"`
	Decision      *kernel.FullDecision `json:"decision"`
}

// AICache persists AI decisions for repeated backtesting or replay.
type AICache struct {
	mu      sync.RWMutex
	path    string
	Entries map[string]cachedDecision `json:"entries"`
}

const aiCacheBackupSuffix = ".bak"

func LoadAICache(path string) (*AICache, error) {
	if path == "" {
		return nil, fmt.Errorf("ai cache path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	cache := &AICache{
		path:    path,
		Entries: make(map[string]cachedDecision),
	}

	if err := loadCacheFromPath(cache, path); err == nil {
		return cache, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		backupPath := cacheBackupPath(path)
		if backupErr := loadCacheFromPath(cache, backupPath); backupErr == nil {
			recovered, marshalErr := json.MarshalIndent(cache, "", "  ")
			if marshalErr != nil {
				return nil, fmt.Errorf("load ai cache from backup failed to marshal recovered cache: %w", marshalErr)
			}
			if recoverErr := writeFileAtomic(path, recovered, 0o644); recoverErr != nil {
				return nil, fmt.Errorf("load ai cache from backup failed to recover primary: %w", recoverErr)
			}
			return cache, nil
		}
		return nil, fmt.Errorf("failed to load ai cache (%s): %w", path, err)
	}

	return cache, nil
}

func (c *AICache) Path() string {
	if c == nil {
		return ""
	}
	return c.path
}

func (c *AICache) Get(key string) (*kernel.FullDecision, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.Entries[key]
	c.mu.RUnlock()
	if !ok || entry.Decision == nil {
		return nil, false
	}
	return cloneDecision(entry.Decision), true
}

func (c *AICache) Put(key string, variant string, ts int64, decision *kernel.FullDecision) error {
	if c == nil || key == "" || decision == nil {
		return nil
	}
	entry := cachedDecision{
		Key:           key,
		PromptVariant: variant,
		Timestamp:     ts,
		Decision:      cloneDecision(decision),
	}
	c.mu.Lock()
	c.Entries[key] = entry
	c.mu.Unlock()
	return c.save()
}

func (c *AICache) save() error {
	if c == nil || c.path == "" {
		return nil
	}
	c.mu.RLock()
	data, err := json.MarshalIndent(c, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := validateAICachePayload(data); err != nil {
		return err
	}
	if err := writeFileAtomic(c.path, data, 0o644); err != nil {
		return err
	}
	// Keep a last-known-good backup to recover from unexpected corruption.
	return writeFileAtomic(cacheBackupPath(c.path), data, 0o644)
}

func cloneDecision(src *kernel.FullDecision) *kernel.FullDecision {
	if src == nil {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return nil
	}
	var dst kernel.FullDecision
	if err := json.Unmarshal(data, &dst); err != nil {
		return nil
	}
	return &dst
}

func computeCacheKey(ctx *kernel.Context, variant string, ts int64) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("context is nil")
	}
	payload := struct {
		Variant        string                 `json:"variant"`
		Timestamp      int64                  `json:"ts"`
		CurrentTime    string                 `json:"current_time"`
		Account        kernel.AccountInfo     `json:"account"`
		Positions      []kernel.PositionInfo  `json:"positions"`
		CandidateCoins []kernel.CandidateCoin `json:"candidate_coins"`
		MarketData     map[string]market.Data `json:"market"`
		MarginUsedPct  float64                `json:"margin_used_pct"`
		Runtime        int                    `json:"runtime_minutes"`
		CallCount      int                    `json:"call_count"`
	}{
		Variant:        variant,
		Timestamp:      ts,
		CurrentTime:    ctx.CurrentTime,
		Account:        ctx.Account,
		Positions:      ctx.Positions,
		CandidateCoins: ctx.CandidateCoins,
		MarginUsedPct:  ctx.Account.MarginUsedPct,
		Runtime:        ctx.RuntimeMinutes,
		CallCount:      ctx.CallCount,
		MarketData:     make(map[string]market.Data, len(ctx.MarketDataMap)),
	}

	for symbol, data := range ctx.MarketDataMap {
		if data == nil {
			continue
		}
		payload.MarketData[symbol] = *data
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}

func cacheBackupPath(path string) string {
	return path + aiCacheBackupSuffix
}

func loadCacheFromPath(cache *AICache, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := validateAICachePayload(data); err != nil {
		return err
	}
	if err := json.Unmarshal(data, cache); err != nil {
		return err
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]cachedDecision)
	}
	return nil
}

func validateAICachePayload(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil
	}
	if !json.Valid(trimmed) {
		return fmt.Errorf("ai cache payload is not valid json")
	}
	var probe struct {
		Entries map[string]json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return fmt.Errorf("ai cache schema probe failed: %w", err)
	}
	return nil
}
