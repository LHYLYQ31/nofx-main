package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"nofx/config"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"nofx/backtest"
	"nofx/logger"
	"nofx/market"
	"nofx/provider/nofxos"
	"nofx/store"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (s *Server) registerBacktestRoutes(router *gin.RouterGroup) {
	router.POST("/start", s.handleBacktestStart)
	router.POST("/pause", s.handleBacktestPause)
	router.POST("/resume", s.handleBacktestResume)
	router.POST("/stop", s.handleBacktestStop)
	router.POST("/close-all", s.handleBacktestCloseAll)
	router.POST("/close-position", s.handleBacktestClosePosition)
	router.POST("/label", s.handleBacktestLabel)
	router.POST("/delete", s.handleBacktestDelete)
	router.GET("/status", s.handleBacktestStatus)
	router.GET("/runs", s.handleBacktestRuns)
	router.GET("/showcase/runs", s.handleBacktestShowcaseRuns)
	router.GET("/showcase/strategies", s.handleBacktestShowcaseStrategies)
	router.GET("/correction/permission", s.handleBacktestCorrectionPermission)
	router.POST("/correction", s.handleBacktestCorrection)
	router.GET("/equity", s.handleBacktestEquity)
	router.GET("/trades", s.handleBacktestTrades)
	router.GET("/metrics", s.handleBacktestMetrics)
	router.GET("/trace", s.handleBacktestTrace)
	router.GET("/decisions", s.handleBacktestDecisions)
	router.GET("/export", s.handleBacktestExport)
	router.GET("/klines", s.handleBacktestKlines)
}

func (s *Server) handleBacktestShowcaseStrategies(c *gin.Context) {
	items, err := s.store.StrategyShowcase().List()
	if err != nil {
		SafeInternalError(c, "List showcase strategies", err)
		return
	}
	all, err := s.store.Strategy().ListAll()
	if err != nil {
		SafeInternalError(c, "List strategies", err)
		return
	}
	nameByID := make(map[string]string, len(all))
	for _, st := range all {
		if st == nil {
			continue
		}
		nameByID[st.ID] = strings.TrimSpace(st.Name)
	}

	resp := make([]gin.H, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		resp = append(resp, gin.H{
			"strategy_id":   it.StrategyID,
			"strategy_name": nameByID[it.StrategyID],
			"sort_order":    it.SortOrder,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": resp})
}

type backtestStartRequest struct {
	Config backtest.BacktestConfig `json:"config"`
}

type runIDRequest struct {
	RunID string `json:"run_id"`
}

type runPositionRequest struct {
	RunID  string `json:"run_id"`
	Symbol string `json:"symbol"`
	Side   string `json:"side"`
}

type labelRequest struct {
	RunID string `json:"run_id"`
	Label string `json:"label"`
}

type backtestCorrectionMetaPatch struct {
	Label           *string  `json:"label"`
	State           *string  `json:"state"`
	LastError       *string  `json:"last_error"`
	SymbolCount     *int     `json:"symbol_count"`
	DecisionTF      *string  `json:"decision_tf"`
	ProcessedBars   *int     `json:"processed_bars"`
	ProgressPct     *float64 `json:"progress_pct"`
	EquityLast      *float64 `json:"equity_last"`
	MaxDrawdownPct  *float64 `json:"max_drawdown_pct"`
	Liquidated      *bool    `json:"liquidated"`
	LiquidationNote *string  `json:"liquidation_note"`
}

type backtestCorrectionMetricsPatch struct {
	TotalReturnPct *float64 `json:"total_return_pct"`
	MaxDrawdownPct *float64 `json:"max_drawdown_pct"`
	SharpeRatio    *float64 `json:"sharpe_ratio"`
	ProfitFactor   *float64 `json:"profit_factor"`
	WinRate        *float64 `json:"win_rate"`
	Trades         *int     `json:"trades"`
	AvgWin         *float64 `json:"avg_win"`
	AvgLoss        *float64 `json:"avg_loss"`
	BestSymbol     *string  `json:"best_symbol"`
	WorstSymbol    *string  `json:"worst_symbol"`
	Liquidated     *bool    `json:"liquidated"`
}

type backtestCorrectionTradePatch struct {
	TradeID         int64    `json:"trade_id" binding:"required"`
	Timestamp       *int64   `json:"ts"`
	Symbol          *string  `json:"symbol"`
	Action          *string  `json:"action"`
	Side            *string  `json:"side"`
	Quantity        *float64 `json:"qty"`
	Price           *float64 `json:"price"`
	EntryPrice      *float64 `json:"entry_price"`
	Fee             *float64 `json:"fee"`
	Slippage        *float64 `json:"slippage"`
	OrderValue      *float64 `json:"order_value"`
	RealizedPnL     *float64 `json:"realized_pnl"`
	Leverage        *int     `json:"leverage"`
	Cycle           *int     `json:"cycle"`
	PositionAfter   *float64 `json:"position_after"`
	LiquidationFlag *bool    `json:"liquidation"`
	Note            *string  `json:"note"`
}

type backtestCorrectionRequest struct {
	RunID        string                          `json:"run_id" binding:"required"`
	Reason       string                          `json:"reason"`
	Meta         *backtestCorrectionMetaPatch    `json:"meta"`
	Metrics      *backtestCorrectionMetricsPatch `json:"metrics"`
	TradeUpdates []backtestCorrectionTradePatch  `json:"trade_updates"`
}

type backtestRunListItem struct {
	*backtest.RunMetadata
	StrategyID   string   `json:"strategy_id,omitempty"`
	StrategyName string   `json:"strategy_name,omitempty"`
	Symbols      []string `json:"symbols,omitempty"`
}

func (s *Server) handleBacktestStart(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}

	var req backtestStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	cfg := req.Config
	if cfg.RunID == "" {
		cfg.RunID = "bt_" + time.Now().UTC().Format("20060102_150405")
	}
	cfg.CustomPrompt = strings.TrimSpace(cfg.CustomPrompt)
	cfg.UserID = normalizeUserID(c.GetString("user_id"))
	cfg.UserEmail = normalizeEmail(c.GetString("email"))

	logger.Infof("📊 Backtest request - symbols from request: %v (count=%d), strategyID: %s",
		cfg.Symbols, len(cfg.Symbols), cfg.StrategyID)

	// Load strategy config if strategy_id is provided
	if cfg.StrategyID != "" {
		var strategy *store.Strategy
		var err error
		if isAdminRole(c) {
			strategy, err = s.store.Strategy().GetByID(cfg.StrategyID)
		} else {
			strategy, err = s.store.Strategy().GetAccessible(cfg.UserID, cfg.StrategyID)
		}
		if err != nil {
			SafeBadRequest(c, "Failed to load strategy")
			return
		}
		if strategy == nil {
			SafeBadRequest(c, "Strategy not found")
			return
		}
		var strategyConfig store.StrategyConfig
		if err := json.Unmarshal([]byte(strategy.Config), &strategyConfig); err != nil {
			SafeBadRequest(c, "Failed to parse strategy config")
			return
		}
		cfg.SetLoadedStrategy(&strategyConfig)
		logger.Infof("📊 Backtest using saved strategy: %s (%s)", strategy.Name, strategy.ID)
		logger.Infof("📊 Strategy coin source: type=%s, use_ai500=%v, use_oi_top=%v, static_coins=%v",
			strategyConfig.CoinSource.SourceType,
			strategyConfig.CoinSource.UseAI500,
			strategyConfig.CoinSource.UseOITop,
			strategyConfig.CoinSource.StaticCoins)

		// If no symbols provided, fetch from strategy's coin source
		if len(cfg.Symbols) == 0 {
			symbols, err := s.resolveStrategyCoins(&strategyConfig)
			if err != nil {
				SafeBadRequest(c, fmt.Sprintf("Failed to resolve coins from strategy: %v", err))
				return
			}
			cfg.Symbols = symbols
			logger.Infof("📊 Resolved %d coins from strategy: %v", len(symbols), symbols)
		}
	}

	if err := s.hydrateBacktestAIConfig(&cfg); err != nil {
		SafeBadRequest(c, "Failed to configure AI model")
		return
	}

	logger.Infof("📊 Starting backtest with final config: runID=%s, symbols=%v (count=%d), strategyID=%s",
		cfg.RunID, cfg.Symbols, len(cfg.Symbols), cfg.StrategyID)

	runner, err := s.backtestManager.Start(context.Background(), cfg)
	if err != nil {
		SafeError(c, http.StatusBadRequest, "Failed to start backtest", err)
		return
	}

	meta := runner.CurrentMetadata()
	c.JSON(http.StatusOK, meta)
}

func (s *Server) handleBacktestPause(c *gin.Context) {
	s.handleBacktestControl(c, s.backtestManager.Pause)
}

func (s *Server) handleBacktestResume(c *gin.Context) {
	s.handleBacktestControl(c, s.backtestManager.Resume)
}

func (s *Server) handleBacktestStop(c *gin.Context) {
	s.handleBacktestControl(c, s.backtestManager.Stop)
}

func (s *Server) handleBacktestCloseAll(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))

	var req runIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	if strings.TrimSpace(req.RunID) == "" {
		SafeBadRequest(c, "run_id is required")
		return
	}
	if _, err := s.ensureBacktestRunOwnership(req.RunID, userID); writeBacktestAccessError(c, err) {
		return
	}
	if err := s.backtestManager.CloseAllPositions(req.RunID); err != nil {
		SafeBadRequest(c, SanitizeError(err, "Failed to close all positions"))
		return
	}
	meta, err := s.backtestManager.LoadMetadata(req.RunID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (s *Server) handleBacktestClosePosition(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))

	var req runPositionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	req.RunID = strings.TrimSpace(req.RunID)
	req.Symbol = strings.TrimSpace(req.Symbol)
	req.Side = strings.ToLower(strings.TrimSpace(req.Side))
	if req.RunID == "" || req.Symbol == "" || (req.Side != "long" && req.Side != "short") {
		SafeBadRequest(c, "run_id, symbol and side(long/short) are required")
		return
	}
	if _, err := s.ensureBacktestRunOwnership(req.RunID, userID); writeBacktestAccessError(c, err) {
		return
	}
	if err := s.backtestManager.ClosePosition(req.RunID, req.Symbol, req.Side); err != nil {
		SafeBadRequest(c, SanitizeError(err, "Failed to close backtest position"))
		return
	}
	meta, err := s.backtestManager.LoadMetadata(req.RunID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (s *Server) handleBacktestControl(c *gin.Context, fn func(string) error) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))

	var req runIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	if req.RunID == "" {
		SafeBadRequest(c, "run_id is required")
		return
	}

	if _, err := s.ensureBacktestRunOwnership(req.RunID, userID); writeBacktestAccessError(c, err) {
		return
	}

	if err := fn(req.RunID); err != nil {
		SafeError(c, http.StatusBadRequest, "Failed to execute backtest operation", err)
		return
	}

	meta, err := s.backtestManager.LoadMetadata(req.RunID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (s *Server) handleBacktestLabel(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	var req labelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	if strings.TrimSpace(req.RunID) == "" {
		SafeBadRequest(c, "run_id is required")
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))
	if _, err := s.ensureBacktestRunOwnership(req.RunID, userID); writeBacktestAccessError(c, err) {
		return
	}
	meta, err := s.backtestManager.UpdateLabel(req.RunID, req.Label)
	if err != nil {
		SafeInternalError(c, "Update backtest label", err)
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (s *Server) handleBacktestDelete(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	var req runIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	if strings.TrimSpace(req.RunID) == "" {
		SafeBadRequest(c, "run_id is required")
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))
	if _, err := s.ensureBacktestRunOwnership(req.RunID, userID); writeBacktestAccessError(c, err) {
		return
	}
	if err := s.backtestManager.Delete(req.RunID); err != nil {
		SafeInternalError(c, "Delete backtest run", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (s *Server) handleBacktestStatus(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}

	userID := normalizeUserID(c.GetString("user_id"))

	runID := c.Query("run_id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required"})
		return
	}

	meta, err := s.ensureBacktestRunReadAccess(runID, userID)
	if writeBacktestAccessError(c, err) {
		return
	}

	status := s.backtestManager.Status(runID)
	if status != nil && (status.State == backtest.RunStateRunning || status.State == backtest.RunStatePaused) {
		c.JSON(http.StatusOK, status)
		return
	}

	payload := backtest.StatusPayload{
		RunID:          meta.RunID,
		State:          meta.State,
		ProgressPct:    meta.Summary.ProgressPct,
		ProcessedBars:  meta.Summary.ProcessedBars,
		CurrentTime:    0,
		DecisionCycle:  meta.Summary.ProcessedBars,
		Equity:         meta.Summary.EquityLast,
		UnrealizedPnL:  0,
		RealizedPnL:    0,
		Note:           meta.Summary.LiquidationNote,
		LastUpdatedIso: meta.UpdatedAt.Format(time.RFC3339),
	}
	c.JSON(http.StatusOK, payload)
}

func (s *Server) handleBacktestRuns(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	rawUserID := strings.TrimSpace(c.GetString("user_id"))
	userID := normalizeUserID(rawUserID)
	filterByUser := rawUserID != "" && rawUserID != "admin"

	metas, err := s.backtestManager.ListRuns()
	if err != nil {
		SafeInternalError(c, "List backtest runs", err)
		return
	}
	stateFilter := strings.ToLower(strings.TrimSpace(c.Query("state")))
	search := strings.ToLower(strings.TrimSpace(c.Query("search")))
	limit := queryInt(c, "limit", 50)
	offset := queryInt(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	filtered := make([]*backtest.RunMetadata, 0, len(metas))
	for _, meta := range metas {
		if stateFilter != "" && !strings.EqualFold(string(meta.State), stateFilter) {
			continue
		}
		if search != "" {
			target := strings.ToLower(meta.RunID + " " + meta.Summary.DecisionTF + " " + meta.Label + " " + meta.LastError)
			if !strings.Contains(target, search) {
				continue
			}
		}
		if filterByUser {
			owner := strings.TrimSpace(meta.UserID)
			if owner != "" && owner != userID {
				continue
			}
		}
		filtered = append(filtered, meta)
	}

	total := len(filtered)
	start := offset
	if start > total {
		start = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := filtered[start:end]
	items := s.decorateBacktestRunListItems(page)

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"items": items,
	})
}

func (s *Server) handleBacktestShowcaseRuns(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	showcaseOwners, err := s.backtestShowcaseOwnerUserIDSet()
	if err != nil {
		SafeInternalError(c, "Resolve backtest showcase owners", err)
		return
	}
	if len(showcaseOwners) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"total": 0,
			"items": []*backtest.RunMetadata{},
		})
		return
	}

	metas, err := s.backtestManager.ListRuns()
	if err != nil {
		SafeInternalError(c, "List backtest showcase runs", err)
		return
	}

	stateFilter := strings.ToLower(strings.TrimSpace(c.Query("state")))
	search := strings.ToLower(strings.TrimSpace(c.Query("search")))
	limit := queryInt(c, "limit", 50)
	offset := queryInt(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	filtered := make([]*backtest.RunMetadata, 0)
	for _, meta := range metas {
		owner := normalizeUserID(strings.TrimSpace(meta.UserID))
		if _, ok := showcaseOwners[owner]; !ok {
			continue
		}
		if stateFilter != "" && !strings.EqualFold(string(meta.State), stateFilter) {
			continue
		}
		if search != "" {
			target := strings.ToLower(meta.RunID + " " + meta.Summary.DecisionTF + " " + meta.Label + " " + meta.LastError)
			if !strings.Contains(target, search) {
				continue
			}
		}
		filtered = append(filtered, meta)
	}

	total := len(filtered)
	start := offset
	if start > total {
		start = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := filtered[start:end]
	items := s.decorateBacktestRunListItems(page)

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"items": items,
	})
}

func (s *Server) decorateBacktestRunListItems(runs []*backtest.RunMetadata) []*backtestRunListItem {
	if len(runs) == 0 {
		return []*backtestRunListItem{}
	}

	items := make([]*backtestRunListItem, 0, len(runs))
	strategyNameCache := make(map[string]string)
	for _, meta := range runs {
		if meta == nil {
			continue
		}

		item := &backtestRunListItem{
			RunMetadata: meta,
		}

		cfg, err := backtest.LoadConfig(meta.RunID)
		if err != nil || cfg == nil {
			items = append(items, item)
			continue
		}

		item.Symbols = append([]string(nil), cfg.Symbols...)
		item.StrategyID = strings.TrimSpace(cfg.StrategyID)
		if item.StrategyID == "" {
			items = append(items, item)
			continue
		}

		if name, ok := strategyNameCache[item.StrategyID]; ok {
			item.StrategyName = name
			items = append(items, item)
			continue
		}

		if st, getErr := s.store.Strategy().GetByID(item.StrategyID); getErr == nil && st != nil {
			item.StrategyName = strings.TrimSpace(st.Name)
		}
		strategyNameCache[item.StrategyID] = item.StrategyName
		items = append(items, item)
	}
	return items
}

func (s *Server) handleBacktestCorrectionPermission(c *gin.Context) {
	email := normalizeEmail(c.GetString("email"))
	showcaseEnabled, err := s.isBacktestShowcaseEnabled()
	if err != nil {
		SafeInternalError(c, "Load backtest showcase permission", err)
		return
	}
	canEdit := false
	if showcaseEnabled {
		canEdit, err = s.canEditBacktestCorrection(email)
		if err != nil {
			SafeInternalError(c, "Load backtest showcase editor permission", err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"can_edit":         canEdit,
		"showcase_enabled": showcaseEnabled,
	})
}

func (s *Server) handleBacktestCorrection(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}

	userID := normalizeUserID(c.GetString("user_id"))
	email := normalizeEmail(c.GetString("email"))
	canEdit, err := s.canEditBacktestCorrection(email)
	if err != nil {
		SafeInternalError(c, "Load backtest showcase editor permission", err)
		return
	}
	if !canEdit {
		SafeForbidden(c, "Only showcase provider account can edit backtest results")
		return
	}

	var req backtestCorrectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		SafeBadRequest(c, "run_id is required")
		return
	}

	if runner, ok := s.backtestManager.GetRunner(runID); ok {
		state := runner.Status()
		if state == backtest.RunStateRunning || state == backtest.RunStatePaused {
			SafeBadRequest(c, "Cannot correct a running or paused backtest")
			return
		}
	}

	meta, err := backtest.LoadRunMetadata(runID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows) {
			SafeNotFound(c, "Backtest task")
			return
		}
		SafeInternalError(c, "Load backtest metadata", err)
		return
	}

	ownerUserID := normalizeUserID(meta.UserID)
	showcaseOwners, err := s.backtestShowcaseOwnerUserIDSet()
	if err != nil {
		SafeInternalError(c, "Resolve backtest showcase owners", err)
		return
	}
	if _, ok := showcaseOwners[ownerUserID]; !ok {
		SafeForbidden(c, "Only showcase provider account runs can be corrected")
		return
	}

	if len(req.TradeUpdates) == 0 {
		SafeBadRequest(c, "trade_updates is required")
		return
	}
	if req.Meta != nil || req.Metrics != nil {
		SafeBadRequest(c, "Only trade_updates is supported now; summary and metrics are recalculated automatically")
		return
	}

	updatedMeta := false

	var metrics *backtest.Metrics
	updatedMetrics := false
	manualRealizedOverrides := map[int64]float64{}
	currentEvents, err := backtest.LoadTradeEvents(runID)
	if err != nil {
		SafeInternalError(c, "Load trade events for correction", err)
		return
	}
	eventByID := make(map[int64]backtest.TradeEvent, len(currentEvents))
	for _, evt := range currentEvents {
		if evt.ID > 0 {
			eventByID[evt.ID] = evt
		}
	}

	for _, tradePatch := range req.TradeUpdates {
		if tradePatch.TradeID <= 0 {
			SafeBadRequest(c, "trade_updates.trade_id must be greater than 0")
			return
		}
		updates := map[string]interface{}{}
		if tradePatch.Timestamp != nil {
			updates["ts"] = *tradePatch.Timestamp
		}
		if tradePatch.Symbol != nil {
			updates["symbol"] = strings.TrimSpace(*tradePatch.Symbol)
		}
		if tradePatch.Action != nil {
			updates["action"] = strings.TrimSpace(*tradePatch.Action)
		}
		if tradePatch.Side != nil {
			updates["side"] = strings.TrimSpace(*tradePatch.Side)
		}
		if tradePatch.Quantity != nil {
			updates["qty"] = *tradePatch.Quantity
		}
		if tradePatch.Price != nil {
			updates["price"] = *tradePatch.Price
		}
		if tradePatch.Fee != nil {
			updates["fee"] = *tradePatch.Fee
		}
		if tradePatch.Slippage != nil {
			updates["slippage"] = *tradePatch.Slippage
		}
		if tradePatch.OrderValue != nil {
			updates["order_value"] = *tradePatch.OrderValue
		}
		if tradePatch.RealizedPnL != nil {
			updates["realized_pnl"] = *tradePatch.RealizedPnL
			manualRealizedOverrides[tradePatch.TradeID] = *tradePatch.RealizedPnL
		}
		if tradePatch.Leverage != nil {
			updates["leverage"] = *tradePatch.Leverage
		}
		if tradePatch.Cycle != nil {
			updates["cycle"] = *tradePatch.Cycle
		}
		if tradePatch.PositionAfter != nil {
			updates["position_after"] = *tradePatch.PositionAfter
		}
		if tradePatch.LiquidationFlag != nil {
			updates["liquidation"] = *tradePatch.LiquidationFlag
		}
		if tradePatch.Note != nil {
			updates["note"] = strings.TrimSpace(*tradePatch.Note)
		}

		if tradePatch.EntryPrice != nil {
			if *tradePatch.EntryPrice <= 0 {
				SafeBadRequest(c, "trade_updates.entry_price must be greater than 0")
				return
			}
			baseEvt, ok := eventByID[tradePatch.TradeID]
			if !ok {
				SafeBadRequest(c, fmt.Sprintf("trade id %d not found for run", tradePatch.TradeID))
				return
			}
			action := baseEvt.Action
			if tradePatch.Action != nil {
				action = strings.TrimSpace(*tradePatch.Action)
			}
			sideRaw := baseEvt.Side
			if tradePatch.Side != nil {
				sideRaw = strings.TrimSpace(*tradePatch.Side)
			}
			qty := baseEvt.Quantity
			if tradePatch.Quantity != nil {
				qty = *tradePatch.Quantity
			}
			price := baseEvt.Price
			if tradePatch.Price != nil {
				price = *tradePatch.Price
			}
			fee := baseEvt.Fee
			if tradePatch.Fee != nil {
				fee = *tradePatch.Fee
			}
			liquidation := baseEvt.LiquidationFlag
			if tradePatch.LiquidationFlag != nil {
				liquidation = *tradePatch.LiquidationFlag
			}
			side := inferTradeSide(backtest.TradeEvent{
				Action:          action,
				Side:            sideRaw,
				LiquidationFlag: liquidation,
			})
			if side == "" {
				SafeBadRequest(c, "Cannot infer side for entry_price correction")
				return
			}
			isOpen, isClose := inferTradeIntent(action, side, liquidation)
			if !isClose || isOpen {
				SafeBadRequest(c, "entry_price is only valid for close trades")
				return
			}
			closeQty := math.Abs(qty)
			if closeQty <= 0 {
				SafeBadRequest(c, "trade_updates.qty must be greater than 0 for entry_price correction")
				return
			}
			grossPnL := (price - *tradePatch.EntryPrice) * closeQty
			if side == "short" {
				grossPnL = (*tradePatch.EntryPrice - price) * closeQty
			}
			manualPnL := grossPnL - fee
			updates["realized_pnl"] = manualPnL
			manualRealizedOverrides[tradePatch.TradeID] = manualPnL
		}

		if len(updates) == 0 {
			continue
		}
		if err := s.store.Backtest().UpdateTradeEvent(runID, tradePatch.TradeID, updates); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				SafeBadRequest(c, fmt.Sprintf("trade id %d not found for run", tradePatch.TradeID))
				return
			}
			SafeInternalError(c, "Save corrected trades", err)
			return
		}
	}
	if len(req.TradeUpdates) > 0 {
		if err := s.recalculateTradeRealizedPnL(runID); err != nil {
			SafeInternalError(c, "Recalculate trade realized PnL", err)
			return
		}
		for tradeID, manualPnL := range manualRealizedOverrides {
			if err := s.store.Backtest().UpdateTradeEvent(runID, tradeID, map[string]interface{}{"realized_pnl": manualPnL}); err != nil {
				SafeInternalError(c, "Reapply manual realized PnL override", err)
				return
			}
		}
		recalculatedMetrics, recalculatedPoints, lastEquity, err := recomputeCorrectionMetrics(runID, meta.Summary.Liquidated)
		if err != nil {
			SafeInternalError(c, "Recompute corrected backtest metrics", err)
			return
		}
		if err := backtest.PersistEquityPoints(runID, recalculatedPoints); err != nil {
			SafeInternalError(c, "Persist recalculated equity curve", err)
			return
		}
		metrics = recalculatedMetrics
		updatedMetrics = true
		meta.Summary.EquityLast = lastEquity
		meta.Summary.MaxDrawdownPct = recalculatedMetrics.MaxDrawdownPct
		meta.Summary.Liquidated = recalculatedMetrics.Liquidated
		updatedMeta = true
	}

	if updatedMeta {
		if err := backtest.SaveRunMetadata(meta); err != nil {
			SafeInternalError(c, "Save corrected run metadata", err)
			return
		}
	}
	if updatedMetrics && metrics != nil {
		payload, err := json.Marshal(metrics)
		if err != nil {
			SafeInternalError(c, "Serialize corrected metrics", err)
			return
		}
		if err := s.store.Backtest().SaveMetrics(runID, payload); err != nil {
			SafeInternalError(c, "Save corrected metrics", err)
			return
		}
	}

	patchPayload, _ := json.Marshal(req)
	if err := s.store.Backtest().SaveCorrectionLog(runID, userID, strings.TrimSpace(req.Reason), patchPayload); err != nil {
		SafeInternalError(c, "Save correction audit log", err)
		return
	}

	updatedRun, err := s.backtestManager.LoadMetadata(runID)
	if err != nil {
		SafeInternalError(c, "Reload corrected metadata", err)
		return
	}
	updatedMetricsObj, _ := backtest.LoadMetrics(runID)
	c.JSON(http.StatusOK, gin.H{
		"message": "Backtest result corrected successfully",
		"run":     updatedRun,
		"metrics": updatedMetricsObj,
	})
}

func (s *Server) handleBacktestEquity(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}

	userID := normalizeUserID(c.GetString("user_id"))

	runID := c.Query("run_id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required"})
		return
	}
	if _, err := s.ensureBacktestRunReadAccess(runID, userID); writeBacktestAccessError(c, err) {
		return
	}
	timeframe := c.Query("tf")
	limit := queryInt(c, "limit", 1000)

	points, err := s.backtestManager.LoadEquity(runID, timeframe, limit)
	if err != nil {
		SafeError(c, http.StatusBadRequest, "Failed to load equity data", err)
		return
	}
	c.JSON(http.StatusOK, points)
}

func (s *Server) handleBacktestTrades(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}

	userID := normalizeUserID(c.GetString("user_id"))

	runID := c.Query("run_id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required"})
		return
	}
	if _, err := s.ensureBacktestRunReadAccess(runID, userID); writeBacktestAccessError(c, err) {
		return
	}
	limit := queryInt(c, "limit", 1000)

	events, err := s.backtestManager.LoadTrades(runID, limit)
	if err != nil {
		SafeError(c, http.StatusBadRequest, "Failed to load trades", err)
		return
	}
	c.JSON(http.StatusOK, events)
}

func (s *Server) handleBacktestMetrics(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}

	userID := normalizeUserID(c.GetString("user_id"))

	runID := c.Query("run_id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required"})
		return
	}
	if _, err := s.ensureBacktestRunReadAccess(runID, userID); writeBacktestAccessError(c, err) {
		return
	}

	metrics, err := s.backtestManager.GetMetrics(runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusAccepted, gin.H{"error": "metrics not ready yet"})
			return
		}
		SafeError(c, http.StatusBadRequest, "Failed to load metrics", err)
		return
	}
	c.JSON(http.StatusOK, metrics)
}

func (s *Server) handleBacktestTrace(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))
	runID := c.Query("run_id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required"})
		return
	}
	if _, err := s.ensureBacktestRunReadAccess(runID, userID); writeBacktestAccessError(c, err) {
		return
	}
	cycle := queryInt(c, "cycle", 0)
	record, err := s.backtestManager.GetTrace(runID, cycle)
	if err != nil {
		SafeNotFound(c, "Trace record")
		return
	}
	c.JSON(http.StatusOK, record)
}

func (s *Server) handleBacktestDecisions(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))
	runID := c.Query("run_id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required"})
		return
	}
	if _, err := s.ensureBacktestRunReadAccess(runID, userID); writeBacktestAccessError(c, err) {
		return
	}
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	records, err := backtest.LoadDecisionRecords(runID, limit, offset)
	if err != nil {
		SafeInternalError(c, "Load decision records", err)
		return
	}
	c.JSON(http.StatusOK, records)
}

func (s *Server) handleBacktestExport(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))
	runID := c.Query("run_id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required"})
		return
	}
	if _, err := s.ensureBacktestRunOwnership(runID, userID); writeBacktestAccessError(c, err) {
		return
	}
	path, err := s.backtestManager.ExportRun(runID)
	if err != nil {
		SafeError(c, http.StatusBadRequest, "Failed to export backtest", err)
		return
	}
	defer os.Remove(path)
	filename := fmt.Sprintf("%s_export.zip", runID)
	c.FileAttachment(path, filename)
}

func (s *Server) handleBacktestKlines(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}
	userID := normalizeUserID(c.GetString("user_id"))
	runID := c.Query("run_id")
	symbol := c.Query("symbol")
	timeframe := c.Query("timeframe")

	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id is required"})
		return
	}
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol is required"})
		return
	}

	meta, err := s.ensureBacktestRunReadAccess(runID, userID)
	if writeBacktestAccessError(c, err) {
		return
	}

	// Load config to get time range
	cfg, err := backtest.LoadConfig(runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to load backtest config"})
		return
	}

	// Use decision timeframe if not specified
	if timeframe == "" {
		timeframe = cfg.DecisionTimeframe
		if timeframe == "" {
			timeframe = "15m"
		}
	}

	// Fetch klines for the backtest time range
	startTime := time.Unix(cfg.StartTS, 0)
	endTime := time.Unix(cfg.EndTS, 0)

	klines, err := market.GetKlinesRange(symbol, timeframe, startTime, endTime)
	if err != nil {
		SafeInternalError(c, "Fetch klines", err)
		return
	}

	// Convert to response format
	type KlineResponse struct {
		Time   int64   `json:"time"`
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume float64 `json:"volume"`
	}

	result := make([]KlineResponse, len(klines))
	for i, k := range klines {
		result[i] = KlineResponse{
			Time:   k.OpenTime / 1000, // Convert to seconds for lightweight-charts
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"symbol":    symbol,
		"timeframe": timeframe,
		"start_ts":  cfg.StartTS,
		"end_ts":    cfg.EndTS,
		"count":     len(result),
		"klines":    result,
		"run_id":    meta.RunID,
	})
}

func queryInt(c *gin.Context, name string, fallback int) int {
	if value := c.Query(name); value != "" {
		if v, err := strconv.Atoi(value); err == nil {
			return v
		}
	}
	return fallback
}

type tradeReplayPosition struct {
	Qty     float64
	Avg     float64
	OpenFee float64
}

func inferTradeIntent(action string, side string, liquidation bool) (isOpen bool, isClose bool) {
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	isOpen = strings.Contains(normalizedAction, "open")
	isClose = strings.Contains(normalizedAction, "close") || liquidation
	if isOpen || isClose {
		return isOpen, isClose
	}

	// Backward compatibility: some historical trades used buy/sell as action.
	switch normalizedAction {
	case "buy":
		if side == "long" {
			return true, false
		}
		if side == "short" {
			return false, true
		}
	case "sell":
		if side == "short" {
			return true, false
		}
		if side == "long" {
			return false, true
		}
	}
	return false, liquidation
}

func (s *Server) recalculateTradeRealizedPnL(runID string) error {
	events, err := backtest.LoadTradeEvents(runID)
	if err != nil {
		return err
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp == events[j].Timestamp {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp < events[j].Timestamp
	})

	positions := make(map[string]*tradeReplayPosition)
	for i := range events {
		evt := &events[i]
		side := inferTradeSide(*evt)
		if side == "" {
			continue
		}
		key := evt.Symbol + "|" + side
		pos := positions[key]
		if pos == nil {
			pos = &tradeReplayPosition{}
			positions[key] = pos
		}

		isOpen, isClose := inferTradeIntent(evt.Action, side, evt.LiquidationFlag)
		qty := evt.Quantity
		if qty < 0 {
			qty = -qty
		}
		if qty <= 0 {
			continue
		}

		if isOpen {
			newQty := pos.Qty + qty
			if newQty > 0 {
				pos.Avg = (pos.Avg*pos.Qty + evt.Price*qty) / newQty
			}
			pos.Qty = newQty
			pos.OpenFee += evt.Fee
			desiredPnL := 0.0
			if math.Abs(evt.RealizedPnL-desiredPnL) > 1e-9 && evt.ID > 0 {
				if err := s.store.Backtest().UpdateTradeEvent(runID, evt.ID, map[string]interface{}{"realized_pnl": desiredPnL}); err != nil {
					return err
				}
			}
			continue
		}

		if !isClose {
			continue
		}

		closeQty := qty
		if pos.Qty > 0 && closeQty > pos.Qty {
			closeQty = pos.Qty
		}
		if closeQty <= 0 {
			continue
		}

		grossPnL := (evt.Price - pos.Avg) * closeQty
		if side == "short" {
			grossPnL = (pos.Avg - evt.Price) * closeQty
		}

		openFeeShare := 0.0
		if pos.Qty > 0 && pos.OpenFee != 0 {
			openFeeShare = pos.OpenFee * (closeQty / pos.Qty)
		}
		desiredPnL := grossPnL - evt.Fee - openFeeShare

		if math.Abs(evt.RealizedPnL-desiredPnL) > 1e-9 && evt.ID > 0 {
			if err := s.store.Backtest().UpdateTradeEvent(runID, evt.ID, map[string]interface{}{"realized_pnl": desiredPnL}); err != nil {
				return err
			}
		}

		if pos.Qty > 0 {
			pos.Qty -= closeQty
			pos.OpenFee -= openFeeShare
			if pos.Qty < 1e-12 {
				pos.Qty = 0
				pos.Avg = 0
				pos.OpenFee = 0
			}
		}
	}
	return nil
}

func inferTradeSide(evt backtest.TradeEvent) string {
	side := strings.ToLower(strings.TrimSpace(evt.Side))
	if side == "long" || side == "short" {
		return side
	}

	action := strings.ToLower(strings.TrimSpace(evt.Action))
	if strings.Contains(action, "long") {
		return "long"
	}
	if strings.Contains(action, "short") {
		return "short"
	}

	// Backward compatibility: some historical rows may use buy/sell in side.
	// Use action semantics to map to position side.
	if side == "buy" || side == "sell" {
		isOpen := strings.Contains(action, "open")
		isClose := strings.Contains(action, "close") || evt.LiquidationFlag
		if side == "buy" {
			if isOpen {
				return "long"
			}
			if isClose {
				return "short"
			}
		}
		if side == "sell" {
			if isOpen {
				return "short"
			}
			if isClose {
				return "long"
			}
		}
	}

	return ""
}

func recomputeCorrectionMetrics(runID string, liquidatedHint bool) (*backtest.Metrics, []backtest.EquityPoint, float64, error) {
	cfg, err := backtest.LoadConfig(runID)
	if err != nil {
		return nil, nil, 0, err
	}
	events, err := backtest.LoadTradeEvents(runID)
	if err != nil {
		return nil, nil, 0, err
	}

	initialBalance := cfg.InitialBalance
	if initialBalance <= 0 {
		initialBalance = 1
	}
	lastEquity := initialBalance
	points := make([]backtest.EquityPoint, 0, len(events))
	for _, evt := range events {
		lastEquity += evt.RealizedPnL
		points = append(points, backtest.EquityPoint{
			Timestamp: evt.Timestamp,
			Equity:    lastEquity,
			Available: lastEquity,
			PnL:       lastEquity - initialBalance,
			PnLPct:    ((lastEquity - initialBalance) / initialBalance) * 100,
			Cycle:     evt.Cycle,
		})
	}
	applyDrawdownPct(points)

	metrics := &backtest.Metrics{
		SymbolStats: make(map[string]backtest.SymbolMetrics),
		Liquidated:  liquidatedHint,
	}
	for _, evt := range events {
		if evt.LiquidationFlag {
			metrics.Liquidated = true
			break
		}
	}
	metrics.TotalReturnPct = ((lastEquity - initialBalance) / initialBalance) * 100
	metrics.MaxDrawdownPct = maxDrawdownFromPoints(points)
	metrics.SharpeRatio = sharpeRatioFromPoints(points)
	fillTradeStats(metrics, events)

	return metrics, points, lastEquity, nil
}

func applyDrawdownPct(points []backtest.EquityPoint) {
	if len(points) == 0 {
		return
	}
	peak := points[0].Equity
	if peak <= 0 {
		peak = 1
	}
	for i := range points {
		if points[i].Equity > peak {
			peak = points[i].Equity
		}
		if peak > 0 {
			points[i].DrawdownPct = (peak - points[i].Equity) / peak * 100
		}
	}
}

func maxDrawdownFromPoints(points []backtest.EquityPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	peak := points[0].Equity
	if peak <= 0 {
		peak = 1
	}
	maxDD := 0.0
	for _, pt := range points {
		if pt.Equity > peak {
			peak = pt.Equity
		}
		if peak <= 0 {
			continue
		}
		dd := (peak - pt.Equity) / peak * 100
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

func sharpeRatioFromPoints(points []backtest.EquityPoint) float64 {
	const minDataPoints = 10
	if len(points) < minDataPoints {
		return 0
	}
	returns := make([]float64, 0, len(points)-1)
	prev := points[0].Equity
	for i := 1; i < len(points); i++ {
		curr := points[i].Equity
		if prev <= 0 {
			prev = curr
			continue
		}
		returns = append(returns, (curr-prev)/prev)
		prev = curr
	}
	if len(returns) < minDataPoints-1 {
		return 0
	}

	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	variance := 0.0
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	if len(returns) > 1 {
		variance /= float64(len(returns) - 1)
	}
	std := math.Sqrt(variance)
	if std < 1e-10 {
		return 0
	}
	return (mean / std) * math.Sqrt(252.0)
}

func fillTradeStats(metrics *backtest.Metrics, events []backtest.TradeEvent) {
	if metrics == nil {
		return
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp == events[j].Timestamp {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp < events[j].Timestamp
	})

	totalTrades := 0
	winTrades := 0
	lossTrades := 0
	totalWinAmount := 0.0
	totalLossAmount := 0.0
	for _, evt := range events {
		action := strings.ToLower(strings.TrimSpace(evt.Action))
		include := evt.LiquidationFlag || strings.HasPrefix(action, "close")
		if evt.RealizedPnL != 0 {
			include = true
		}
		if !include {
			continue
		}

		totalTrades++
		stats := metrics.SymbolStats[evt.Symbol]
		stats.TotalTrades++
		stats.TotalPnL += evt.RealizedPnL
		if evt.RealizedPnL > 0 {
			winTrades++
			totalWinAmount += evt.RealizedPnL
			stats.WinningTrades++
		} else if evt.RealizedPnL < 0 {
			lossTrades++
			totalLossAmount += -evt.RealizedPnL
			stats.LosingTrades++
		}
		metrics.SymbolStats[evt.Symbol] = stats
	}

	metrics.Trades = totalTrades
	if totalTrades > 0 {
		metrics.WinRate = (float64(winTrades) / float64(totalTrades)) * 100
	}
	if winTrades > 0 {
		metrics.AvgWin = totalWinAmount / float64(winTrades)
	}
	if lossTrades > 0 {
		metrics.AvgLoss = -(totalLossAmount / float64(lossTrades))
	}
	if totalLossAmount > 0 {
		metrics.ProfitFactor = totalWinAmount / totalLossAmount
	} else if totalWinAmount > 0 {
		metrics.ProfitFactor = 100.0
	}

	bestSymbol := ""
	bestPnL := math.Inf(-1)
	worstSymbol := ""
	worstPnL := math.Inf(1)
	for symbol, stats := range metrics.SymbolStats {
		if stats.TotalTrades > 0 {
			if stats.TotalPnL > bestPnL {
				bestPnL = stats.TotalPnL
				bestSymbol = symbol
			}
			if stats.TotalPnL < worstPnL {
				worstPnL = stats.TotalPnL
				worstSymbol = symbol
			}
			stats.AvgPnL = stats.TotalPnL / float64(stats.TotalTrades)
			stats.WinRate = (float64(stats.WinningTrades) / float64(stats.TotalTrades)) * 100
			metrics.SymbolStats[symbol] = stats
		}
	}
	if !math.IsInf(bestPnL, -1) {
		metrics.BestSymbol = bestSymbol
	}
	if !math.IsInf(worstPnL, 1) {
		metrics.WorstSymbol = worstSymbol
	}
}

var errBacktestForbidden = errors.New("backtest run forbidden")

func normalizeUserID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "default"
	}
	return id
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Server) ensureBacktestRunOwnership(runID, userID string) (*backtest.RunMetadata, error) {
	return s.ensureBacktestRunAccess(runID, userID, false)
}

func (s *Server) ensureBacktestRunReadAccess(runID, userID string) (*backtest.RunMetadata, error) {
	return s.ensureBacktestRunAccess(runID, userID, true)
}

func (s *Server) ensureBacktestRunAccess(runID, userID string, allowShowcaseRead bool) (*backtest.RunMetadata, error) {
	if s.backtestManager == nil {
		return nil, fmt.Errorf("backtest manager unavailable")
	}
	meta, err := s.backtestManager.LoadMetadata(runID)
	if err != nil {
		return nil, err
	}
	if userID == "" || userID == "admin" {
		return meta, nil
	}
	owner := strings.TrimSpace(meta.UserID)
	if owner == "" {
		return meta, nil
	}
	if owner != userID {
		if allowShowcaseRead {
			owners, err := s.backtestShowcaseOwnerUserIDSet()
			if err != nil {
				return nil, fmt.Errorf("resolve backtest showcase owners: %w", err)
			}
			if _, ok := owners[owner]; ok {
				return meta, nil
			}
		}
		return nil, errBacktestForbidden
	}
	return meta, nil
}

func (s *Server) backtestShowcaseOwnerUserIDSet() (map[string]struct{}, error) {
	owners, err := s.store.BacktestShowcaseUser().ResolveEnabledUserIDs(s.store.User())
	if err != nil {
		return nil, err
	}
	legacyOwner := s.backtestShowcaseLegacyOwnerUserID()
	if legacyOwner != "" {
		owners[legacyOwner] = struct{}{}
	}
	return owners, nil
}

func (s *Server) isBacktestShowcaseEnabled() (bool, error) {
	owners, err := s.backtestShowcaseOwnerUserIDSet()
	if err != nil {
		return false, err
	}
	return len(owners) > 0, nil
}

func (s *Server) backtestShowcaseLegacyOwnerUserID() string {
	showcaseEmail := normalizeEmail(config.Get().BacktestShowcaseEmail)
	if showcaseEmail != "" {
		u, err := s.store.User().GetByEmail(showcaseEmail)
		if err == nil && u != nil {
			return normalizeUserID(u.ID)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Errorf("Resolve legacy showcase email %s failed: %v", showcaseEmail, err)
		}
		return ""
	}
	rawUserID := strings.TrimSpace(config.Get().BacktestShowcaseUserID)
	if rawUserID == "" {
		return ""
	}
	return normalizeUserID(rawUserID)
}

func (s *Server) canEditBacktestCorrection(email string) (bool, error) {
	normalized := normalizeEmail(email)
	if normalized == "" {
		return false, nil
	}
	allowed, err := s.store.BacktestShowcaseUser().IsAllowed(normalized)
	if err != nil {
		return false, err
	}
	if allowed {
		return true, nil
	}
	legacyShowcaseEmail := normalizeEmail(config.Get().BacktestShowcaseEmail)
	if legacyShowcaseEmail == "" {
		return false, nil
	}
	return normalized == legacyShowcaseEmail, nil
}

func writeBacktestAccessError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, errBacktestForbidden):
		SafeForbidden(c, "No permission to access this backtest task")
	case errors.Is(err, os.ErrNotExist), errors.Is(err, sql.ErrNoRows):
		SafeNotFound(c, "Backtest task")
	default:
		SafeInternalError(c, "Access backtest", err)
	}
	return true
}

// resolveStrategyCoins fetches coins based on strategy's coin source configuration
func (s *Server) resolveStrategyCoins(strategyConfig *store.StrategyConfig) ([]string, error) {
	if strategyConfig == nil {
		return nil, fmt.Errorf("strategy config is nil")
	}

	coinSource := strategyConfig.CoinSource
	var symbols []string
	symbolSet := make(map[string]bool)
	gridSymbol := ""
	if strategyConfig.GridConfig != nil {
		gridSymbol = strings.TrimSpace(strategyConfig.GridConfig.Symbol)
	}
	hasUsableGridSymbol := gridSymbol != "" && !strings.EqualFold(gridSymbol, "MULTI")

	// Handle empty source_type - check flags for backward compatibility
	sourceType := coinSource.SourceType
	if sourceType == "" {
		if coinSource.UseAI500 && coinSource.UseOITop {
			sourceType = "mixed"
		} else if coinSource.UseAI500 {
			sourceType = "ai500"
		} else if coinSource.UseOITop {
			sourceType = "oi_top"
		} else if len(coinSource.StaticCoins) > 0 {
			sourceType = "static"
		} else if hasUsableGridSymbol {
			// Grid strategy may only set grid_config.symbol.
			sourceType = "static"
		} else {
			return nil, fmt.Errorf("strategy has no coin source configured")
		}
		logger.Infof("📊 Inferred source_type=%s from flags", sourceType)
	}

	switch sourceType {
	case "static":
		if len(coinSource.StaticCoins) == 0 && hasUsableGridSymbol {
			sym := market.Normalize(gridSymbol)
			if sym != "" {
				symbols = append(symbols, sym)
				symbolSet[sym] = true
				break
			}
		}
		if len(coinSource.StaticCoins) == 0 {
			return nil, fmt.Errorf("strategy coin_source is static but static_coins is empty")
		}
		for _, sym := range coinSource.StaticCoins {
			sym = market.Normalize(sym)
			if !symbolSet[sym] {
				symbols = append(symbols, sym)
				symbolSet[sym] = true
			}
		}

	case "ai500":
		limit := coinSource.AI500Limit
		if limit <= 0 {
			limit = 30
		}
		logger.Infof("📊 Fetching AI500 coins with limit=%d", limit)
		coins, err := nofxos.DefaultClient().GetTopRatedCoins(limit)
		if err != nil {
			return nil, fmt.Errorf("failed to get AI500 coins: %w", err)
		}
		logger.Infof("📊 Got %d coins from AI500: %v", len(coins), coins)
		for _, sym := range coins {
			sym = market.Normalize(sym)
			if !symbolSet[sym] {
				symbols = append(symbols, sym)
				symbolSet[sym] = true
			}
		}

	case "oi_top":
		coins, err := nofxos.DefaultClient().GetOITopSymbols()
		if err != nil {
			return nil, fmt.Errorf("failed to get OI Top coins: %w", err)
		}
		limit := coinSource.OITopLimit
		if limit <= 0 || limit > len(coins) {
			limit = len(coins)
		}
		for i, sym := range coins {
			if i >= limit {
				break
			}
			sym = market.Normalize(sym)
			if !symbolSet[sym] {
				symbols = append(symbols, sym)
				symbolSet[sym] = true
			}
		}

	case "mixed":
		// Get from AI500
		if coinSource.UseAI500 {
			limit := coinSource.AI500Limit
			if limit <= 0 {
				limit = 30
			}
			coins, err := nofxos.DefaultClient().GetTopRatedCoins(limit)
			if err != nil {
				logger.Warnf("Failed to get AI500 coins: %v", err)
			} else {
				for _, sym := range coins {
					sym = market.Normalize(sym)
					if !symbolSet[sym] {
						symbols = append(symbols, sym)
						symbolSet[sym] = true
					}
				}
			}
		}

		// Get from OI Top
		if coinSource.UseOITop {
			coins, err := nofxos.DefaultClient().GetOITopSymbols()
			if err != nil {
				logger.Warnf("Failed to get OI Top coins: %v", err)
			} else {
				limit := coinSource.OITopLimit
				if limit <= 0 || limit > len(coins) {
					limit = len(coins)
				}
				for i, sym := range coins {
					if i >= limit {
						break
					}
					sym = market.Normalize(sym)
					if !symbolSet[sym] {
						symbols = append(symbols, sym)
						symbolSet[sym] = true
					}
				}
			}
		}

		// Add static coins
		for _, sym := range coinSource.StaticCoins {
			sym = market.Normalize(sym)
			if !symbolSet[sym] {
				symbols = append(symbols, sym)
				symbolSet[sym] = true
			}
		}

	default:
		return nil, fmt.Errorf("unknown coin source type: %s", sourceType)
	}

	if len(symbols) == 0 {
		return nil, fmt.Errorf("no coins resolved from strategy")
	}

	logger.Infof("📊 Final resolved symbols: %d coins - %v", len(symbols), symbols)
	return symbols, nil
}

func (s *Server) resolveBacktestAIConfig(cfg *backtest.BacktestConfig, userID string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if s.store == nil {
		return fmt.Errorf("System database not ready, cannot load AI model configuration")
	}

	cfg.UserID = normalizeUserID(userID)

	return s.hydrateBacktestAIConfig(cfg)
}

func (s *Server) hydrateBacktestAIConfig(cfg *backtest.BacktestConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if s.store == nil {
		return fmt.Errorf("System database not ready, cannot load AI model configuration")
	}

	cfg.UserID = normalizeUserID(cfg.UserID)
	modelID := strings.TrimSpace(cfg.AIModelID)

	var (
		model *store.AIModel
		err   error
	)

	if modelID != "" {
		model, err = s.store.AIModel().Get(cfg.UserID, modelID)
		if err != nil {
			return fmt.Errorf("Failed to load AI model: %w", err)
		}
	} else {
		model, err = s.store.AIModel().GetDefault(cfg.UserID)
		if err != nil {
			return fmt.Errorf("No available AI model found: %w", err)
		}
		cfg.AIModelID = model.ID
	}

	if !model.Enabled {
		return fmt.Errorf("AI model %s is not enabled yet", model.Name)
	}

	apiKey := strings.TrimSpace(string(model.APIKey))
	if apiKey == "" {
		return fmt.Errorf("AI model %s is missing API Key, please configure it in the system first", model.Name)
	}

	provider := strings.ToLower(strings.TrimSpace(model.Provider))
	// Ensure provider is never empty or "inherit" - infer from model name if needed
	if provider == "" || provider == "inherit" {
		modelNameLower := strings.ToLower(model.Name)
		if strings.Contains(modelNameLower, "claude") || strings.Contains(modelNameLower, "anthropic") {
			provider = "anthropic"
		} else if strings.Contains(modelNameLower, "gpt") || strings.Contains(modelNameLower, "openai") {
			provider = "openai"
		} else if strings.Contains(modelNameLower, "gemini") || strings.Contains(modelNameLower, "google") {
			provider = "google"
		} else if strings.Contains(modelNameLower, "deepseek") {
			provider = "deepseek"
		} else if strings.Contains(modelNameLower, "minimax") {
			provider = "minimax"
		} else if model.CustomAPIURL != "" {
			provider = "custom"
		} else {
			provider = "openai" // default fallback
		}
		logger.Infof("📊 Inferred AI provider '%s' from model name '%s'", provider, model.Name)
	}
	cfg.AICfg.Provider = provider
	cfg.AICfg.APIKey = apiKey
	cfg.AICfg.BaseURL = strings.TrimSpace(model.CustomAPIURL)
	modelName := strings.TrimSpace(model.CustomModelName)
	// If user configured a model name in AI model settings, always prefer it.
	// If not configured, keep incoming value (possibly empty) and let provider default apply.
	if modelName != "" {
		cfg.AICfg.Model = modelName
	}
	cfg.AICfg.Model = strings.TrimSpace(cfg.AICfg.Model)

	if cfg.AICfg.Provider == "custom" {
		if cfg.AICfg.BaseURL == "" {
			return fmt.Errorf("Custom AI model requires API URL configuration")
		}
		if cfg.AICfg.Model == "" {
			return fmt.Errorf("Custom AI model requires model name configuration")
		}
	}

	return nil
}
