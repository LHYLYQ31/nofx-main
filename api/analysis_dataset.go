package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"nofx/backtest"
	"nofx/store"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type analysisDatasetResponse struct {
	GeneratedAt string                      `json:"generated_at"`
	Params      analysisDatasetParams       `json:"params"`
	Backtests   []analysisBacktestDataset   `json:"backtests"`
	Traders     []analysisTraderLiveDataset `json:"traders"`
	Warnings    []string                    `json:"warnings,omitempty"`
}

type analysisDatasetParams struct {
	RunLimit            int      `json:"run_limit"`
	RunTradeLimit       int      `json:"run_trade_limit"`
	RunEquityLimit      int      `json:"run_equity_limit"`
	RunDecisionLimit    int      `json:"run_decision_limit"`
	TraderLimit         int      `json:"trader_limit"`
	TraderTradeLimit    int      `json:"trader_trade_limit"`
	TraderEquityLimit   int      `json:"trader_equity_limit"`
	TraderDecisionLimit int      `json:"trader_decision_limit"`
	IncludeDecisions    bool     `json:"include_decisions"`
	RunIDs              []string `json:"run_ids,omitempty"`
	TraderIDs           []string `json:"trader_ids,omitempty"`
}

type analysisBacktestDataset struct {
	Metadata  *backtest.RunMetadata       `json:"metadata"`
	Status    *backtest.StatusPayload     `json:"status,omitempty"`
	Config    *analysisBacktestConfigView `json:"config,omitempty"`
	Metrics   *backtest.Metrics           `json:"metrics,omitempty"`
	Equity    []backtest.EquityPoint      `json:"equity,omitempty"`
	Trades    []backtest.TradeEvent       `json:"trades,omitempty"`
	Decisions []*store.DecisionRecord     `json:"decisions,omitempty"`
	Error     string                      `json:"error,omitempty"`
}

type analysisBacktestConfigView struct {
	RunID             string                  `json:"run_id"`
	StrategyID        string                  `json:"strategy_id,omitempty"`
	Symbols           []string                `json:"symbols,omitempty"`
	Timeframes        []string                `json:"timeframes,omitempty"`
	DecisionTimeframe string                  `json:"decision_timeframe,omitempty"`
	StartTS           int64                   `json:"start_ts"`
	EndTS             int64                   `json:"end_ts"`
	InitialBalance    float64                 `json:"initial_balance"`
	FeeBps            float64                 `json:"fee_bps"`
	SlippageBps       float64                 `json:"slippage_bps"`
	Leverage          backtest.LeverageConfig `json:"leverage"`
}

type analysisTraderLiveDataset struct {
	Trader        analysisTraderProfile   `json:"trader"`
	StrategyName  string                  `json:"strategy_name,omitempty"`
	Stats         *store.TraderStats      `json:"stats,omitempty"`
	DecisionStats *store.Statistics       `json:"decision_stats,omitempty"`
	RecentTrades  []store.RecentTrade     `json:"recent_trades,omitempty"`
	EquityHistory []analysisEquityPoint   `json:"equity_history,omitempty"`
	Decisions     []*store.DecisionRecord `json:"decisions,omitempty"`
	Error         string                  `json:"error,omitempty"`
}

type analysisTraderProfile struct {
	TraderID            string    `json:"trader_id"`
	UserID              string    `json:"user_id"`
	TraderName          string    `json:"trader_name"`
	AIModelID           string    `json:"ai_model_id"`
	ExchangeID          string    `json:"exchange_id"`
	StrategyID          string    `json:"strategy_id,omitempty"`
	ExecutionMode       string    `json:"execution_mode"`
	IsRunning           bool      `json:"is_running"`
	ShowInCompetition   bool      `json:"show_in_competition"`
	InitialBalance      float64   `json:"initial_balance"`
	ScanIntervalMinutes int       `json:"scan_interval_minutes"`
	BTCETHLeverage      int       `json:"btc_eth_leverage,omitempty"`
	AltcoinLeverage     int       `json:"altcoin_leverage,omitempty"`
	TradingSymbols      string    `json:"trading_symbols,omitempty"`
	UseAI500            bool      `json:"use_ai500,omitempty"`
	UseOITop            bool      `json:"use_oi_top,omitempty"`
	IsCrossMargin       bool      `json:"is_cross_margin"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type analysisEquityPoint struct {
	Timestamp        string  `json:"timestamp"`
	TotalEquity      float64 `json:"total_equity"`
	AvailableBalance float64 `json:"available_balance"`
	TotalPnL         float64 `json:"total_pnl"`
	TotalPnLPct      float64 `json:"total_pnl_pct"`
	PositionCount    int     `json:"position_count"`
	MarginUsedPct    float64 `json:"margin_used_pct"`
}

// handlePublicAnalysisDataset exports backtest + live datasets for offline analysis.
// This endpoint is intentionally public (no authentication) as requested.
func (s *Server) handlePublicAnalysisDataset(c *gin.Context) {
	if s.backtestManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest manager unavailable"})
		return
	}

	runLimit := boundedQueryInt(c, "run_limit", 20, 1, 200)
	runTradeLimit := boundedQueryInt(c, "run_trade_limit", 2000, 1, 20000)
	runEquityLimit := boundedQueryInt(c, "run_equity_limit", 5000, 1, 50000)
	runDecisionLimit := boundedQueryInt(c, "run_decision_limit", 200, 1, 5000)
	traderLimit := boundedQueryInt(c, "trader_limit", 50, 1, 500)
	traderTradeLimit := boundedQueryInt(c, "trader_trade_limit", 2000, 1, 20000)
	traderEquityLimit := boundedQueryInt(c, "trader_equity_limit", 5000, 1, 50000)
	traderDecisionLimit := boundedQueryInt(c, "trader_decision_limit", 200, 1, 5000)
	includeDecisions := queryBool(c, "include_decisions", false)
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "zip")))

	runFilter, runIDs := parseCSVSet(c.Query("run_ids"))
	traderFilter, traderIDs := parseCSVSet(c.Query("trader_ids"))

	resp := analysisDatasetResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Params: analysisDatasetParams{
			RunLimit:            runLimit,
			RunTradeLimit:       runTradeLimit,
			RunEquityLimit:      runEquityLimit,
			RunDecisionLimit:    runDecisionLimit,
			TraderLimit:         traderLimit,
			TraderTradeLimit:    traderTradeLimit,
			TraderEquityLimit:   traderEquityLimit,
			TraderDecisionLimit: traderDecisionLimit,
			IncludeDecisions:    includeDecisions,
			RunIDs:              runIDs,
			TraderIDs:           traderIDs,
		},
		Backtests: make([]analysisBacktestDataset, 0),
		Traders:   make([]analysisTraderLiveDataset, 0),
		Warnings:  make([]string, 0),
	}

	// Backtest data
	metas, err := s.backtestManager.ListRuns()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("list backtest runs failed: %v", err)})
		return
	}

	selectedMetas := make([]*backtest.RunMetadata, 0, len(metas))
	for _, meta := range metas {
		if meta == nil {
			continue
		}
		if len(runFilter) > 0 {
			if _, ok := runFilter[meta.RunID]; !ok {
				continue
			}
		}
		selectedMetas = append(selectedMetas, meta)
		if len(runFilter) == 0 && len(selectedMetas) >= runLimit {
			break
		}
	}

	for _, meta := range selectedMetas {
		row := analysisBacktestDataset{
			Metadata: meta,
		}

		if status := s.backtestManager.Status(meta.RunID); status != nil {
			row.Status = status
		}

		if cfg, cfgErr := backtest.LoadConfig(meta.RunID); cfgErr == nil && cfg != nil {
			row.Config = &analysisBacktestConfigView{
				RunID:             cfg.RunID,
				StrategyID:        cfg.StrategyID,
				Symbols:           append([]string(nil), cfg.Symbols...),
				Timeframes:        append([]string(nil), cfg.Timeframes...),
				DecisionTimeframe: cfg.DecisionTimeframe,
				StartTS:           cfg.StartTS,
				EndTS:             cfg.EndTS,
				InitialBalance:    cfg.InitialBalance,
				FeeBps:            cfg.FeeBps,
				SlippageBps:       cfg.SlippageBps,
				Leverage:          cfg.Leverage,
			}
		}

		if metrics, metricsErr := s.backtestManager.GetMetrics(meta.RunID); metricsErr == nil {
			row.Metrics = metrics
		} else {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("backtest %s metrics unavailable: %v", meta.RunID, metricsErr))
		}

		if points, pointsErr := s.backtestManager.LoadEquity(meta.RunID, "15m", runEquityLimit); pointsErr == nil {
			row.Equity = points
		} else {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("backtest %s equity unavailable: %v", meta.RunID, pointsErr))
		}

		if trades, tradesErr := s.backtestManager.LoadTrades(meta.RunID, runTradeLimit); tradesErr == nil {
			row.Trades = trades
		} else {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("backtest %s trades unavailable: %v", meta.RunID, tradesErr))
		}

		if includeDecisions {
			if records, recordErr := backtest.LoadDecisionRecords(meta.RunID, runDecisionLimit, 0); recordErr == nil {
				row.Decisions = records
			} else {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("backtest %s decisions unavailable: %v", meta.RunID, recordErr))
			}
		}

		resp.Backtests = append(resp.Backtests, row)
	}

	// Live trader data
	strategyNameByID := map[string]string{}
	if allStrategies, listErr := s.store.Strategy().ListAll(); listErr == nil {
		for _, st := range allStrategies {
			if st == nil {
				continue
			}
			strategyNameByID[st.ID] = strings.TrimSpace(st.Name)
		}
	}

	allTraders, traderErr := s.store.Trader().ListAll()
	if traderErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("list traders failed: %v", traderErr)})
		return
	}
	selectedTraders := make([]*store.Trader, 0, len(allTraders))
	for _, t := range allTraders {
		if t == nil {
			continue
		}
		if len(traderFilter) > 0 {
			if _, ok := traderFilter[t.ID]; !ok {
				continue
			}
		}
		selectedTraders = append(selectedTraders, t)
		if len(traderFilter) == 0 && len(selectedTraders) >= traderLimit {
			break
		}
	}

	for _, t := range selectedTraders {
		isRunning := t.IsRunning
		if at, getErr := s.traderManager.GetTrader(t.ID); getErr == nil {
			if status := at.GetStatus(); status != nil {
				if running, ok := status["is_running"].(bool); ok {
					isRunning = running
				}
			}
		}

		row := analysisTraderLiveDataset{
			Trader: analysisTraderProfile{
				TraderID:            t.ID,
				UserID:              t.UserID,
				TraderName:          t.Name,
				AIModelID:           t.AIModelID,
				ExchangeID:          t.ExchangeID,
				StrategyID:          t.StrategyID,
				ExecutionMode:       store.NormalizeTraderExecutionMode(t.ExecutionMode),
				IsRunning:           isRunning,
				ShowInCompetition:   t.ShowInCompetition,
				InitialBalance:      t.InitialBalance,
				ScanIntervalMinutes: t.ScanIntervalMinutes,
				BTCETHLeverage:      t.BTCETHLeverage,
				AltcoinLeverage:     t.AltcoinLeverage,
				TradingSymbols:      t.TradingSymbols,
				UseAI500:            t.UseAI500,
				UseOITop:            t.UseOITop,
				IsCrossMargin:       t.IsCrossMargin,
				CreatedAt:           t.CreatedAt,
				UpdatedAt:           t.UpdatedAt,
			},
			StrategyName: strategyNameByID[t.StrategyID],
		}

		if stats, statsErr := s.store.Position().GetFullStats(t.ID); statsErr == nil {
			row.Stats = stats
		} else {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("trader %s stats unavailable: %v", t.ID, statsErr))
		}

		if decStats, dsErr := s.store.Decision().GetStatistics(t.ID); dsErr == nil {
			row.DecisionStats = decStats
		} else {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("trader %s decision stats unavailable: %v", t.ID, dsErr))
		}

		if recentTrades, rtErr := s.store.Position().GetRecentTrades(t.ID, traderTradeLimit); rtErr == nil {
			row.RecentTrades = recentTrades
		} else {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("trader %s recent trades unavailable: %v", t.ID, rtErr))
		}

		if snapshots, eqErr := s.store.Equity().GetLatest(t.ID, traderEquityLimit); eqErr == nil {
			row.EquityHistory = toAnalysisEquityPoints(snapshots)
		} else {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("trader %s equity history unavailable: %v", t.ID, eqErr))
		}

		if includeDecisions {
			if decisions, dErr := s.store.Decision().GetLatestRecords(t.ID, traderDecisionLimit); dErr == nil {
				row.Decisions = decisions
			} else {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("trader %s decisions unavailable: %v", t.ID, dErr))
			}
		}

		resp.Traders = append(resp.Traders, row)
	}

	if format == "json" {
		c.JSON(http.StatusOK, resp)
		return
	}

	payload, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("marshal dataset failed: %v", err)})
		return
	}

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	file, err := zw.Create("dataset.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("zip create failed: %v", err)})
		return
	}
	if _, err := file.Write(payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("zip write failed: %v", err)})
		return
	}
	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("zip finalize failed: %v", err)})
		return
	}

	filename := fmt.Sprintf("analysis_dataset_%s.zip", time.Now().UTC().Format("20060102_150405"))
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func boundedQueryInt(c *gin.Context, name string, fallback, min, max int) int {
	v := queryInt(c, name, fallback)
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func queryBool(c *gin.Context, name string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(c.Query(name)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func parseCSVSet(raw string) (map[string]struct{}, []string) {
	set := map[string]struct{}{}
	items := []string{}
	for _, p := range strings.Split(raw, ",") {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if _, exists := set[v]; exists {
			continue
		}
		set[v] = struct{}{}
		items = append(items, v)
	}
	return set, items
}

func toAnalysisEquityPoints(snapshots []*store.EquitySnapshot) []analysisEquityPoint {
	if len(snapshots) == 0 {
		return []analysisEquityPoint{}
	}

	initialBalance := snapshots[0].Balance
	if initialBalance == 0 {
		initialBalance = 1
	}

	out := make([]analysisEquityPoint, 0, len(snapshots))
	for _, snap := range snapshots {
		totalPnLPct := 0.0
		if initialBalance > 0 {
			totalPnLPct = (snap.UnrealizedPnL / initialBalance) * 100
		}
		out = append(out, analysisEquityPoint{
			Timestamp:        snap.Timestamp.Format("2006-01-02 15:04:05"),
			TotalEquity:      snap.TotalEquity,
			AvailableBalance: snap.Balance,
			TotalPnL:         snap.UnrealizedPnL,
			TotalPnLPct:      totalPnLPct,
			PositionCount:    snap.PositionCount,
			MarginUsedPct:    snap.MarginUsedPct,
		})
	}
	return out
}
