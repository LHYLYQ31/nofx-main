package backtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"nofx/logger"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nofx/kernel"
	"nofx/market"
	"nofx/mcp"
	"nofx/store"
)

var (
	errBacktestCompleted = errors.New("backtest completed")
	errLiquidated        = errors.New("account liquidated")
)

const (
	metricsWriteInterval            = 5 * time.Second
	aiDecisionMaxRetries            = 3
	backtestRecentTradeDetailsLimit = 20
	backtestRecentStatsWindow30     = 30
	backtestRecentStatsWindow50     = 50
	// Guardrails for dynamic grid recentering (adjust_grid).
	gridRecenterCooldownCycles = 3
	gridRecenterMinShiftRatio  = 0.15
)

// gridPendingOrder stores a simulated grid limit order in backtest mode.
// It is intentionally isolated from non-grid strategies.
type gridPendingOrder struct {
	ID         string
	Symbol     string
	Side       string // "buy" or "sell"
	Intent     string // "open" or "close"
	CloseSide  string // "long" or "short" when Intent=close
	Price      float64
	Quantity   float64
	LevelIndex int
	Reasoning  string
	CreatedAt  int64
}

type gridRegimeState struct {
	Mode      string // "range" or "trend"
	Direction string // "long", "short", "neutral"
}

// gridStaticRange keeps a stable grid range for a symbol during one backtest run.
// This prevents boundaries from drifting every cycle and chasing price.
type gridStaticRange struct {
	Upper   float64
	Lower   float64
	Spacing float64
}

type manualClosePositionRequest struct {
	Symbol string
	Side   string
	Resp   chan error
}

type positionBracket struct {
	StopLoss   float64
	TakeProfit float64
}

// Runner encapsulates the lifecycle of a single backtest run.
type Runner struct {
	cfg            BacktestConfig
	feed           *DataFeed
	account        *BacktestAccount
	strategyEngine *kernel.StrategyEngine

	decisionLogDir string
	mcpClient      mcp.AIClient

	statusMu sync.RWMutex
	status   RunState

	stateMu sync.RWMutex
	state   *BacktestState

	pauseCh    chan struct{}
	resumeCh   chan struct{}
	stopCh     chan struct{}
	closeAllCh chan chan error
	closePosCh chan manualClosePositionRequest
	doneCh     chan struct{}

	err              error
	errMu            sync.RWMutex
	lastError        string
	lastCheckpoint   time.Time
	createdAt        time.Time
	lastMetricsWrite time.Time

	aiCache   *AICache
	cachePath string

	lockInfo     *RunLockInfo
	lockStop     chan struct{}
	lockStopOnce sync.Once // Ensures lockStop is closed only once

	discordNotifier *discordNotifier

	// Grid backtest state (only used when strategy_type=grid_trading).
	// Kept separate to ensure classic ai_trading behavior is unchanged.
	gridPaused            bool
	gridOrderSeq          int64
	gridOrders            map[string]*gridPendingOrder
	gridRanges            map[string]gridStaticRange
	gridLastRecenterCycle map[string]int
	gridRegimes           map[string]gridRegimeState

	closedTradeHistory []TradeEvent
	positionBrackets   map[string]positionBracket
}

// NewRunner constructs a backtest runner.
func NewRunner(cfg BacktestConfig, mcpClient mcp.AIClient) (*Runner, error) {
	if err := ensureRunDir(cfg.RunID); err != nil {
		return nil, err
	}

	// Create strategy engine from backtest config for unified prompt generation.
	// DataFeed also uses this config to apply fixed context windows per timeframe.
	strategyConfig := cfg.ToStrategyConfig()
	strategyEngine := kernel.NewStrategyEngine(strategyConfig)

	client, err := configureMCPClient(cfg, mcpClient)
	if err != nil {
		return nil, err
	}

	feed, err := NewDataFeed(cfg, strategyConfig)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(decisionLogDir(cfg.RunID), 0o755); err != nil {
		return nil, err
	}

	dLogDir := decisionLogDir(cfg.RunID)
	account := NewBacktestAccount(cfg.InitialBalance, cfg.FeeBps, cfg.SlippageBps)

	createdAt := time.Now().UTC()
	state := &BacktestState{
		Positions:      make(map[string]PositionSnapshot),
		Cash:           account.Cash(),
		Equity:         cfg.InitialBalance,
		UnrealizedPnL:  0,
		RealizedPnL:    0,
		MaxEquity:      cfg.InitialBalance,
		MinEquity:      cfg.InitialBalance,
		MaxDrawdownPct: 0,
		LastUpdate:     createdAt,
	}

	var (
		aiCache   *AICache
		cachePath string
	)
	if cfg.CacheAI || cfg.ReplayOnly || cfg.SharedAICachePath != "" {
		cachePath = cfg.SharedAICachePath
		if cachePath == "" {
			cachePath = filepath.Join(runDir(cfg.RunID), "ai_cache.json")
		}
		cache, err := LoadAICache(cachePath)
		if err != nil {
			return nil, fmt.Errorf("load ai cache: %w", err)
		}
		aiCache = cache
	}

	closedTrades, err := loadClosedTradeHistory(cfg.RunID)
	if err != nil {
		logger.Warnf("backtest %s failed to load trade history for AI context: %v", cfg.RunID, err)
		closedTrades = []TradeEvent{}
	}

	r := &Runner{
		cfg:                   cfg,
		feed:                  feed,
		account:               account,
		strategyEngine:        strategyEngine,
		decisionLogDir:        dLogDir,
		mcpClient:             client,
		status:                RunStateCreated,
		state:                 state,
		pauseCh:               make(chan struct{}, 1),
		resumeCh:              make(chan struct{}, 1),
		stopCh:                make(chan struct{}, 1),
		closeAllCh:            make(chan chan error, 1),
		closePosCh:            make(chan manualClosePositionRequest, 1),
		doneCh:                make(chan struct{}),
		createdAt:             createdAt,
		aiCache:               aiCache,
		cachePath:             cachePath,
		gridOrders:            make(map[string]*gridPendingOrder),
		gridRanges:            make(map[string]gridStaticRange),
		gridLastRecenterCycle: make(map[string]int),
		gridRegimes:           make(map[string]gridRegimeState),
		closedTradeHistory:    closedTrades,
		positionBrackets:      make(map[string]positionBracket),
	}
	r.discordNotifier = buildBacktestDiscordNotifier(cfg)

	if err := r.initLock(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Runner) initLock() error {
	if r.cfg.RunID == "" {
		return fmt.Errorf("run_id required for lock")
	}
	info, err := acquireRunLock(r.cfg.RunID)
	if err != nil {
		return err
	}
	r.lockInfo = info
	r.lockStop = make(chan struct{})
	go r.lockHeartbeatLoop()
	return nil
}

func (r *Runner) lockHeartbeatLoop() {
	ticker := time.NewTicker(lockHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := updateRunLockHeartbeat(r.lockInfo); err != nil {
				logger.Infof("failed to update lock heartbeat for %s: %v", r.cfg.RunID, err)
			}
		case <-r.lockStop:
			return
		}
	}
}

func (r *Runner) releaseLock() {
	// Use sync.Once to ensure channel is closed exactly once, preventing panic on double-close
	r.lockStopOnce.Do(func() {
		if r.lockStop != nil {
			close(r.lockStop)
		}
	})
	if err := deleteRunLock(r.cfg.RunID); err != nil {
		logger.Infof("failed to release lock for %s: %v", r.cfg.RunID, err)
	}
	r.lockInfo = nil
}

// Start launches the backtest loop.
func (r *Runner) Start(ctx context.Context) error {
	r.statusMu.Lock()
	if r.status != RunStateCreated && r.status != RunStatePaused {
		r.statusMu.Unlock()
		return fmt.Errorf("cannot start runner in state %s", r.status)
	}
	r.status = RunStateRunning
	r.statusMu.Unlock()

	go r.loop(ctx)
	return nil
}

// PersistMetadata writes the current snapshot to run.json.
func (r *Runner) PersistMetadata() {
	r.persistMetadata()
}

func (r *Runner) setLastError(err error) {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	if err == nil {
		r.lastError = ""
		return
	}
	r.lastError = err.Error()
}

func (r *Runner) lastErrorString() string {
	r.errMu.RLock()
	defer r.errMu.RUnlock()
	return r.lastError
}

// CurrentMetadata returns the metadata corresponding to the current in-memory state.
func (r *Runner) CurrentMetadata() *RunMetadata {
	state := r.snapshotState()
	meta := r.buildMetadata(state, r.Status())
	meta.CreatedAt = r.createdAt
	meta.UpdatedAt = state.LastUpdate
	return meta
}

func (r *Runner) loop(ctx context.Context) {
	defer close(r.doneCh)

	for {
		select {
		case <-ctx.Done():
			r.handleStop(fmt.Errorf("context canceled: %w", ctx.Err()))
			return
		case <-r.stopCh:
			r.handleStop(nil)
			return
		case <-r.pauseCh:
			r.handlePause()
			<-r.resumeCh
			r.resumeFromPause()
		case resp := <-r.closeAllCh:
			err := r.closeAllPositions("manual close on backtest run")
			if err == nil {
				r.persistMetadata()
				r.persistMetrics(false)
			}
			resp <- err
		case req := <-r.closePosCh:
			err := r.closeSinglePosition(req.Symbol, req.Side, "manual close specific position")
			if err == nil {
				r.persistMetadata()
				r.persistMetrics(false)
			}
			req.Resp <- err
		default:
		}

		err := r.stepOnce()
		if errors.Is(err, errBacktestCompleted) {
			if r.cfg.ClosePositionsAtEnd {
				if settleErr := r.forceCloseAllAtBacktestEnd(); settleErr != nil {
					r.handleFailure(settleErr)
					return
				}
			}
			r.handleCompletion()
			return
		}
		if errors.Is(err, errLiquidated) {
			r.handleLiquidation()
			return
		}
		if err != nil {
			r.handleFailure(err)
			return
		}
	}
}

func (r *Runner) stepOnce() error {
	state := r.snapshotState()
	if state.BarIndex >= r.feed.DecisionBarCount() {
		return errBacktestCompleted
	}

	ts := r.feed.DecisionTimestamp(state.BarIndex)

	marketData, multiTF, err := r.feed.BuildMarketData(ts)
	if err != nil {
		return err
	}

	priceMap := make(map[string]float64, len(marketData))
	for symbol, data := range marketData {
		priceMap[symbol] = data.CurrentPrice
	}

	callCount := state.DecisionCycle + 1
	shouldDecide := r.shouldTriggerDecision(state.BarIndex)

	var (
		record          *store.DecisionRecord
		decisionActions []store.DecisionAction
		tradeEvents     = make([]TradeEvent, 0)
		execLog         []string
		hadError        bool
	)

	decisionAttempted := shouldDecide

	// In grid mode, previously placed limit orders are matched against the current bar
	// before generating new decisions at bar close.
	if r.isGridBacktestStrategy() {
		fills, fillLogs, fillErr := r.matchGridOrders(ts, state.DecisionCycle)
		if fillErr != nil {
			hadError = true
			execLog = append(execLog, fmt.Sprintf("grid fill error: %v", fillErr))
		}
		if len(fills) > 0 {
			tradeEvents = append(tradeEvents, fills...)
		}
		if len(fillLogs) > 0 {
			execLog = append(execLog, fillLogs...)
		}
	}

	// [CODE ENFORCED] Trigger persisted stop-loss / take-profit levels before new AI decisions.
	bracketEvents, bracketLogs, bracketErr := r.checkStopTakeProfitTriggers(ts, state.DecisionCycle)
	if bracketErr != nil {
		return bracketErr
	}
	if len(bracketEvents) > 0 {
		tradeEvents = append(tradeEvents, bracketEvents...)
	}
	if len(bracketLogs) > 0 {
		execLog = append(execLog, bracketLogs...)
	}

	// [CODE ENFORCED] ATR volatility stop-loss check before new AI decisions.
	atrStopEvents, atrStopLogs, atrStopErr := r.checkATRVolatilityStops(ts, marketData, priceMap, state.DecisionCycle)
	if atrStopErr != nil {
		return atrStopErr
	}
	if len(atrStopEvents) > 0 {
		tradeEvents = append(tradeEvents, atrStopEvents...)
	}
	if len(atrStopLogs) > 0 {
		execLog = append(execLog, atrStopLogs...)
	}

	if shouldDecide {
		ctx, rec, err := r.buildDecisionContext(ts, marketData, multiTF, priceMap, callCount)
		if err != nil {
			// Defensive nil check to prevent panic if buildDecisionContext returns error with nil record
			if rec != nil {
				rec.Success = false
				rec.ErrorMessage = fmt.Sprintf("failed to build trading context: %v", err)
				_ = r.logDecision(rec)
			}
			return err
		}
		record = rec

		var (
			fullDecision *kernel.FullDecision
			fromCache    bool
			cacheKey     string
		)
		if r.aiCache != nil {
			if key, err := computeCacheKey(ctx, r.cacheVariantKey(), ts); err == nil {
				cacheKey = key
				if cached, ok := r.aiCache.Get(cacheKey); ok {
					// Guardrail: grid backtest must not reuse non-grid cached decisions.
					if r.isGridBacktestStrategy() && !isGridDecisionCache(cached) {
						fromCache = false
					} else {
						fullDecision = cached
						fromCache = true
					}
				} else if r.cfg.ReplayOnly {
					decisionErr := fmt.Errorf("replay_only enabled but cache miss at %d", ts)
					record.Success = false
					record.ErrorMessage = fmt.Sprintf("cached decision not found for ts=%d", ts)
					_ = r.logDecision(record)
					return decisionErr
				}
			} else {
				logger.Infof("failed to compute ai cache key: %v", err)
			}
		}

		if !fromCache {
			fd, err := r.invokeAIWithRetry(ctx, ts, marketData, priceMap)
			if err != nil {
				decisionAttempted = true
				hadError = true
				record.Success = false
				record.ErrorMessage = fmt.Sprintf("AI decision failed: %v", err)
				execLog = append(execLog, fmt.Sprintf("⚠️ AI decision failed: %v", err))
				if requestID := extractUpstreamRequestIDFromError(err); requestID != "" {
					logger.Warnf("backtest AI request failed: run=%s cycle=%d ts=%d request_id=%s err=%v",
						r.cfg.RunID, callCount, ts, requestID, err)
				} else {
					logger.Warnf("backtest AI request failed: run=%s cycle=%d ts=%d err=%v",
						r.cfg.RunID, callCount, ts, err)
				}
				r.setLastError(err)
			} else {
				fullDecision = fd
				if r.cfg.CacheAI && r.aiCache != nil && cacheKey != "" {
					if shouldPersistAICacheDecision(fullDecision) {
						if err := r.aiCache.Put(cacheKey, r.cacheVariantKey(), ts, fullDecision); err != nil {
							logger.Infof("failed to persist ai cache for %s: %v", r.cfg.RunID, err)
						}
					} else {
						logger.Infof("skip ai cache persist for %s: fallback/invalid AI response", r.cfg.RunID)
					}
				}
			}
		}

		if fullDecision != nil {
			r.fillDecisionRecord(record, fullDecision)

			sorted := sortDecisionsByPriority(fullDecision.Decisions)

			prevLogs := execLog
			decisionActions = make([]store.DecisionAction, 0, len(sorted))
			execLog = make([]string, 0, len(sorted)+len(prevLogs))
			if len(prevLogs) > 0 {
				execLog = append(execLog, prevLogs...)
			}

			for _, dec := range sorted {
				actionRecord, trades, logEntry, execErr := r.executeDecision(dec, priceMap, ts, callCount)
				if execErr != nil {
					actionRecord.Success = false
					actionRecord.Error = execErr.Error()
					hadError = true
					execLog = append(execLog, fmt.Sprintf("❌ %s %s: %v", dec.Symbol, dec.Action, execErr))
				} else {
					actionRecord.Success = true
					execLog = append(execLog, fmt.Sprintf("✓ %s %s", dec.Symbol, dec.Action))
				}
				if len(trades) > 0 {
					tradeEvents = append(tradeEvents, trades...)
				}
				if logEntry != "" {
					execLog = append(execLog, logEntry)
				}
				decisionActions = append(decisionActions, actionRecord)
			}
		}
	}

	cycleForLog := state.DecisionCycle
	if decisionAttempted {
		cycleForLog = callCount
	}

	liquidationEvents, liquidationNote, err := r.checkLiquidation(ts, priceMap, cycleForLog)
	if err != nil {
		if record != nil {
			record.Success = false
			record.ErrorMessage = err.Error()
			_ = r.logDecision(record)
		}
		return err
	}
	if len(liquidationEvents) > 0 {
		hadError = true
		tradeEvents = append(tradeEvents, liquidationEvents...)
		if record != nil {
			execLog = append(execLog, fmt.Sprintf("⚠️ Forced liquidation: %s", liquidationNote))
		}
	}

	if record != nil {
		record.Decisions = decisionActions
		record.ExecutionLog = execLog
		record.Success = !hadError && liquidationNote == ""
		if liquidationNote != "" {
			record.ErrorMessage = liquidationNote
		}
	}

	equity, unrealized, _ := r.account.TotalEquity(priceMap)
	marginUsed := r.totalMarginUsed()

	r.updateState(ts, equity, unrealized, marginUsed, priceMap, decisionAttempted)

	snapshot := r.snapshotState()
	drawdownPct := 0.0
	if snapshot.MaxEquity > 0 {
		drawdownPct = ((snapshot.MaxEquity - snapshot.Equity) / snapshot.MaxEquity) * 100
	}

	equityPoint := EquityPoint{
		Timestamp:   ts,
		Equity:      snapshot.Equity,
		Available:   snapshot.Cash,
		PnL:         snapshot.Equity - r.account.InitialBalance(),
		PnLPct:      ((snapshot.Equity - r.account.InitialBalance()) / r.account.InitialBalance()) * 100,
		DrawdownPct: drawdownPct,
		Cycle:       snapshot.DecisionCycle,
	}

	if err := appendEquityPoint(r.cfg.RunID, equityPoint); err != nil {
		return err
	}

	for _, evt := range tradeEvents {
		if err := appendTradeEvent(r.cfg.RunID, evt); err != nil {
			return err
		}
		r.recordTradeEventForContext(evt)
		r.notifyTradeSignal(evt, snapshot.Equity)
	}

	if record != nil {
		if err := r.logDecision(record); err != nil {
			return err
		}
	}

	if err := saveProgress(r.cfg.RunID, &snapshot, &r.cfg); err != nil {
		return err
	}

	if err := r.maybeCheckpoint(); err != nil {
		return err
	}

	r.persistMetadata()
	r.persistMetrics(false)

	if !hadError && liquidationNote == "" {
		r.setLastError(nil)
	}

	if snapshot.Liquidated {
		return errLiquidated
	}

	return nil
}

func (r *Runner) buildDecisionContext(ts int64, marketData map[string]*market.Data, multiTF map[string]map[string]*market.Data, priceMap map[string]float64, callCount int) (*kernel.Context, *store.DecisionRecord, error) {
	equity, unrealized, _ := r.account.TotalEquity(priceMap)
	available := r.account.Cash()
	marginUsed := r.totalMarginUsed()
	marginPct := 0.0
	if equity > 0 {
		marginPct = (marginUsed / equity) * 100
	}

	accountInfo := kernel.AccountInfo{
		TotalEquity:      equity,
		AvailableBalance: available,
		TotalPnL:         equity - r.account.InitialBalance(),
		TotalPnLPct:      ((equity - r.account.InitialBalance()) / r.account.InitialBalance()) * 100,
		MarginUsed:       marginUsed,
		MarginUsedPct:    marginPct,
		PositionCount:    len(r.account.Positions()),
	}

	positions := r.convertPositions(priceMap)

	// Get candidate coins from strategy engine (includes source info)
	candidateCoins, err := r.strategyEngine.GetCandidateCoins()
	if err != nil {
		// Fallback to simple list if strategy engine fails
		candidateCoins = make([]kernel.CandidateCoin, 0, len(r.cfg.Symbols))
		for _, sym := range r.cfg.Symbols {
			candidateCoins = append(candidateCoins, kernel.CandidateCoin{Symbol: sym, Sources: []string{"backtest"}})
		}
	}

	runtime := int((ts - int64(r.cfg.StartTS*1000)) / 60000)
	ctx := &kernel.Context{
		CurrentTime:     time.UnixMilli(ts).UTC().Format("2006-01-02 15:04:05 UTC"),
		RuntimeMinutes:  runtime,
		CallCount:       callCount,
		Account:         accountInfo,
		Positions:       positions,
		CandidateCoins:  candidateCoins,
		PromptVariant:   r.cfg.PromptVariant,
		MarketDataMap:   marketData,
		MultiTFMarket:   multiTF,
		BTCETHLeverage:  r.cfg.Leverage.BTCETHLeverage,
		AltcoinLeverage: r.cfg.Leverage.AltcoinLeverage,
		Timeframes:      r.cfg.Timeframes,
	}
	r.populateTradeContext(ctx)

	// Fetch quantitative data if enabled in strategy (uses current data as approximation)
	strategyConfig := r.strategyEngine.GetConfig()
	if strategyConfig.Indicators.EnableQuantData {
		// Collect symbols to query (candidate coins + position coins)
		symbolSet := make(map[string]bool)
		for _, sym := range r.cfg.Symbols {
			symbolSet[sym] = true
		}
		for _, pos := range positions {
			symbolSet[pos.Symbol] = true
		}
		symbols := make([]string, 0, len(symbolSet))
		for sym := range symbolSet {
			symbols = append(symbols, sym)
		}
		ctx.QuantDataMap = r.strategyEngine.FetchQuantDataBatch(symbols)
		if len(ctx.QuantDataMap) > 0 {
			logger.Infof("📊 Backtest: fetched quant data for %d symbols", len(ctx.QuantDataMap))
		}
	}

	// Fetch OI ranking data if enabled in strategy (uses current data as approximation)
	if strategyConfig.Indicators.EnableOIRanking {
		ctx.OIRankingData = r.strategyEngine.FetchOIRankingData()
		if ctx.OIRankingData != nil {
			logger.Infof("📊 Backtest: OI ranking data ready: %d top, %d low positions",
				len(ctx.OIRankingData.TopPositions), len(ctx.OIRankingData.LowPositions))
		}
	}

	// Fetch NetFlow ranking data if enabled in strategy
	if strategyConfig.Indicators.EnableNetFlowRanking {
		ctx.NetFlowRankingData = r.strategyEngine.FetchNetFlowRankingData()
		if ctx.NetFlowRankingData != nil {
			logger.Infof("💰 Backtest: NetFlow ranking data ready: inst_in=%d, inst_out=%d",
				len(ctx.NetFlowRankingData.InstitutionFutureTop), len(ctx.NetFlowRankingData.InstitutionFutureLow))
		}
	}

	// Fetch Price ranking data if enabled in strategy
	if strategyConfig.Indicators.EnablePriceRanking {
		ctx.PriceRankingData = r.strategyEngine.FetchPriceRankingData()
		if ctx.PriceRankingData != nil {
			logger.Infof("📈 Backtest: Price ranking data ready for %d durations",
				len(ctx.PriceRankingData.Durations))
		}
	}

	record := &store.DecisionRecord{
		CycleNumber: callCount,
		AccountState: store.AccountSnapshot{
			TotalBalance:          accountInfo.TotalEquity,
			AvailableBalance:      accountInfo.AvailableBalance,
			TotalUnrealizedProfit: unrealized,
			PositionCount:         accountInfo.PositionCount,
			MarginUsedPct:         accountInfo.MarginUsedPct,
		},
		CandidateCoins: make([]string, 0, len(candidateCoins)),
		Positions:      r.snapshotPositions(priceMap),
	}
	for _, coin := range candidateCoins {
		record.CandidateCoins = append(record.CandidateCoins, coin.Symbol)
	}
	record.Timestamp = time.UnixMilli(ts).UTC()

	return ctx, record, nil
}

func (r *Runner) populateTradeContext(ctx *kernel.Context) {
	if ctx == nil {
		return
	}
	if len(r.closedTradeHistory) == 0 {
		return
	}

	ctx.RecentOrders = buildRecentOrdersFromEvents(r.closedTradeHistory, backtestRecentTradeDetailsLimit)

	maxDrawdown := 0.0
	if snapshot := r.snapshotState(); snapshot.MaxDrawdownPct > 0 {
		maxDrawdown = snapshot.MaxDrawdownPct
	}
	ctx.TradingStats = buildTradingStatsFromEvents(r.closedTradeHistory, maxDrawdown)
	ctx.RecentTradingStats30 = buildRecentTradingStatsFromEvents(r.closedTradeHistory, backtestRecentStatsWindow30)
	ctx.RecentTradingStats50 = buildRecentTradingStatsFromEvents(r.closedTradeHistory, backtestRecentStatsWindow50)
}

func (r *Runner) recordTradeEventForContext(evt TradeEvent) {
	if !isTradeEventForAIHistory(evt) {
		return
	}
	r.closedTradeHistory = append(r.closedTradeHistory, evt)
}

func loadClosedTradeHistory(runID string) ([]TradeEvent, error) {
	events, err := LoadTradeEvents(runID)
	if err != nil {
		return nil, err
	}
	filtered := make([]TradeEvent, 0, len(events))
	for _, evt := range events {
		if isTradeEventForAIHistory(evt) {
			filtered = append(filtered, evt)
		}
	}
	return filtered, nil
}

func isTradeEventForAIHistory(evt TradeEvent) bool {
	if evt.LiquidationFlag {
		return true
	}
	action := strings.ToLower(strings.TrimSpace(evt.Action))
	if strings.HasPrefix(action, "close") {
		return true
	}
	return math.Abs(evt.RealizedPnL) > 1e-9
}

func tradeEventsTail(events []TradeEvent, n int) []TradeEvent {
	if n <= 0 || len(events) == 0 {
		return nil
	}
	if len(events) <= n {
		return events
	}
	return events[len(events)-n:]
}

func buildRecentOrdersFromEvents(events []TradeEvent, limit int) []kernel.RecentOrder {
	window := tradeEventsTail(events, limit)
	if len(window) == 0 {
		return nil
	}
	orders := make([]kernel.RecentOrder, 0, len(window))
	for _, evt := range window {
		entryPrice := estimateEntryPriceFromTrade(evt)
		pnlPct := estimatePnLPct(evt, entryPrice)
		orders = append(orders, kernel.RecentOrder{
			Symbol:       evt.Symbol,
			Side:         evt.Side,
			EntryPrice:   entryPrice,
			ExitPrice:    evt.Price,
			RealizedPnL:  evt.RealizedPnL,
			PnLPct:       pnlPct,
			EntryTime:    "n/a",
			ExitTime:     time.UnixMilli(evt.Timestamp).UTC().Format("01-02 15:04 UTC"),
			HoldDuration: "n/a",
		})
	}
	return orders
}

func buildTradingStatsFromEvents(events []TradeEvent, maxDrawdown float64) *kernel.TradingStats {
	if len(events) == 0 {
		return nil
	}

	var (
		totalPnL   float64
		totalWin   float64
		totalLoss  float64
		winTrades  int
		lossTrades int
	)
	for _, evt := range events {
		totalPnL += evt.RealizedPnL
		if evt.RealizedPnL > 0 {
			winTrades++
			totalWin += evt.RealizedPnL
		} else if evt.RealizedPnL < 0 {
			lossTrades++
			totalLoss += -evt.RealizedPnL
		}
	}

	totalTrades := len(events)
	winRate := 0.0
	if totalTrades > 0 {
		winRate = float64(winTrades) / float64(totalTrades) * 100
	}

	avgWin := 0.0
	if winTrades > 0 {
		avgWin = totalWin / float64(winTrades)
	}

	avgLoss := 0.0
	if lossTrades > 0 {
		avgLoss = totalLoss / float64(lossTrades)
	}

	profitFactor := 0.0
	if totalLoss > 0 {
		profitFactor = totalWin / totalLoss
	} else if totalWin > 0 {
		profitFactor = 100.0
	}

	return &kernel.TradingStats{
		TotalTrades:    totalTrades,
		WinRate:        winRate,
		ProfitFactor:   profitFactor,
		TotalPnL:       totalPnL,
		AvgWin:         avgWin,
		AvgLoss:        avgLoss,
		MaxDrawdownPct: maxDrawdown,
	}
}

func buildRecentTradingStatsFromEvents(events []TradeEvent, window int) *kernel.RecentTradingStats {
	if window <= 0 {
		return nil
	}
	windowEvents := tradeEventsTail(events, window)
	if len(windowEvents) == 0 {
		return nil
	}
	base := buildTradingStatsFromEvents(windowEvents, 0)
	if base == nil {
		return nil
	}
	return &kernel.RecentTradingStats{
		WindowSize:   window,
		TotalTrades:  base.TotalTrades,
		WinRate:      base.WinRate,
		ProfitFactor: base.ProfitFactor,
		TotalPnL:     base.TotalPnL,
		AvgWin:       base.AvgWin,
		AvgLoss:      base.AvgLoss,
	}
}

func estimateEntryPriceFromTrade(evt TradeEvent) float64 {
	if evt.Quantity <= 0 {
		return evt.Price
	}

	side := strings.ToLower(strings.TrimSpace(evt.Side))
	switch side {
	case "short":
		entry := evt.Price + (evt.RealizedPnL / evt.Quantity)
		if entry > 0 {
			return entry
		}
	default:
		entry := evt.Price - (evt.RealizedPnL / evt.Quantity)
		if entry > 0 {
			return entry
		}
	}
	return evt.Price
}

func estimatePnLPct(evt TradeEvent, entryPrice float64) float64 {
	if entryPrice <= 0 {
		return 0
	}
	side := strings.ToLower(strings.TrimSpace(evt.Side))
	switch side {
	case "short":
		return (entryPrice - evt.Price) / entryPrice * 100
	default:
		return (evt.Price - entryPrice) / entryPrice * 100
	}
}

func (r *Runner) fillDecisionRecord(record *store.DecisionRecord, full *kernel.FullDecision) {
	record.InputPrompt = full.UserPrompt
	record.CoTTrace = full.CoTTrace
	if len(full.Decisions) > 0 {
		if data, err := json.MarshalIndent(full.Decisions, "", "  "); err == nil {
			record.DecisionJSON = string(data)
		}
	}
}

func (r *Runner) invokeAIWithRetry(ctx *kernel.Context, ts int64, marketData map[string]*market.Data, priceMap map[string]float64) (*kernel.FullDecision, error) {
	var lastErr error
	for attempt := 0; attempt < aiDecisionMaxRetries; attempt++ {
		fd, err := r.invokeDecision(ctx, ts, marketData, priceMap)
		if err == nil {
			return fd, nil
		}
		lastErr = err
		delay := time.Duration(attempt+1) * 500 * time.Millisecond
		time.Sleep(delay)
	}
	return nil, lastErr
}

func (r *Runner) invokeDecision(ctx *kernel.Context, ts int64, marketData map[string]*market.Data, priceMap map[string]float64) (*kernel.FullDecision, error) {
	// Grid strategy backtest is isolated in this branch to avoid impacting existing non-grid behavior.
	cfg := r.strategyEngine.GetConfig()
	if cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.StrategyType), "grid_trading") && cfg.GridConfig != nil {
		gridCtx, gridCfg, err := r.buildGridDecisionContext(ts, marketData, priceMap, cfg)
		if err != nil {
			return nil, err
		}
		lang := strings.TrimSpace(cfg.Language)
		if lang == "" {
			lang = "en"
		}
		fd, err := kernel.GetGridDecisions(gridCtx, r.mcpClient, gridCfg, lang)
		if err != nil {
			return nil, err
		}
		r.normalizeGridDecisionsForBacktest(fd, gridCtx.CurrentPrice, gridCfg)
		return fd, nil
	}

	// Default path remains unchanged for ai_trading strategies.
	return kernel.GetFullDecisionWithStrategy(
		ctx,
		r.mcpClient,
		r.strategyEngine,
		r.cfg.PromptVariant,
	)
}

func (r *Runner) buildGridDecisionContext(ts int64, marketData map[string]*market.Data, priceMap map[string]float64, strategyCfg *store.StrategyConfig) (*kernel.GridContext, *store.GridStrategyConfig, error) {
	if strategyCfg == nil || strategyCfg.GridConfig == nil {
		return nil, nil, fmt.Errorf("grid strategy config missing")
	}
	if len(marketData) == 0 {
		return nil, nil, fmt.Errorf("market data is empty")
	}

	gridCfgCopy := *strategyCfg.GridConfig
	symbol := strings.ToUpper(strings.TrimSpace(gridCfgCopy.Symbol))
	if symbol == "" || strings.EqualFold(symbol, "MULTI") {
		for _, sym := range r.cfg.Symbols {
			if _, ok := marketData[sym]; ok {
				symbol = sym
				break
			}
		}
	}
	if symbol == "" {
		for sym := range marketData {
			symbol = sym
			break
		}
	}
	mktData, ok := marketData[symbol]
	if !ok || mktData == nil {
		return nil, nil, fmt.Errorf("grid symbol market data not found: %s", symbol)
	}

	gridCfgCopy.Symbol = symbol
	tfFallbacks := make([]string, 0, len(strategyCfg.Indicators.Klines.SelectedTimeframes)+len(r.cfg.Timeframes))
	tfSeen := make(map[string]struct{}, len(strategyCfg.Indicators.Klines.SelectedTimeframes)+len(r.cfg.Timeframes))
	appendTF := func(tf string) {
		n := strings.ToLower(strings.TrimSpace(tf))
		if n == "" {
			return
		}
		if _, ok := tfSeen[n]; ok {
			return
		}
		tfSeen[n] = struct{}{}
		tfFallbacks = append(tfFallbacks, n)
	}
	for _, tf := range strategyCfg.Indicators.Klines.SelectedTimeframes {
		appendTF(tf)
	}
	for _, tf := range r.cfg.Timeframes {
		appendTF(tf)
	}
	gridCtx := kernel.BuildGridContextFromMarketData(mktData, &gridCfgCopy, r.cfg.DecisionTimeframe, tfFallbacks)
	gridCtx.CurrentTime = time.UnixMilli(ts).UTC().Format("2006-01-02 15:04:05 UTC")

	equity, unrealized, _ := r.account.TotalEquity(priceMap)
	gridCtx.TotalEquity = equity
	gridCtx.AvailableBalance = r.account.Cash()
	gridCtx.UnrealizedPnL = unrealized
	gridCtx.CurrentPosition = r.netPositionForSymbol(symbol)

	snapshot := r.snapshotState()
	gridCtx.TotalProfit = snapshot.RealizedPnL
	gridCtx.MaxDrawdown = snapshot.MaxDrawdownPct
	// Keep a stable grid range per symbol in backtest to avoid boundary drift.
	r.applyStableGridRange(symbol, gridCtx, &gridCfgCopy)
	// Inject actual pending-order snapshot so AI can see existing grid state.
	r.injectGridRuntimeState(symbol, gridCtx, &gridCfgCopy)
	r.gridRegimes[symbol] = inferGridRegime(gridCtx)

	return gridCtx, &gridCfgCopy, nil
}

func inferGridRegime(ctx *kernel.GridContext) gridRegimeState {
	regime := gridRegimeState{
		Mode:      "range",
		Direction: "neutral",
	}
	if ctx == nil {
		return regime
	}

	// Align with grid prompt semantics:
	// - trending when Bollinger width > 4% or EMA distance > 2%
	if ctx.BollingerWidth >= 4 || math.Abs(ctx.EMADistance) >= 2 {
		regime.Mode = "trend"
		if ctx.EMA20 > ctx.EMA50 {
			regime.Direction = "long"
		} else if ctx.EMA20 < ctx.EMA50 {
			regime.Direction = "short"
		} else if ctx.CurrentPrice >= ctx.BollingerMiddle {
			regime.Direction = "long"
		} else {
			regime.Direction = "short"
		}
	}
	return regime
}

func (r *Runner) applyStableGridRange(symbol string, gridCtx *kernel.GridContext, cfg *store.GridStrategyConfig) {
	if gridCtx == nil || cfg == nil {
		return
	}
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if sym == "" {
		sym = strings.ToUpper(strings.TrimSpace(cfg.Symbol))
	}
	if sym == "" {
		return
	}

	// Priority:
	// 1) explicit config bounds
	// 2) previously initialized run-local bounds
	// 3) derive once from ATR/current price then keep fixed.
	if cfg.UpperPrice > 0 && cfg.LowerPrice > 0 && cfg.UpperPrice > cfg.LowerPrice {
		gridCtx.UpperPrice = cfg.UpperPrice
		gridCtx.LowerPrice = cfg.LowerPrice
	} else if fixed, ok := r.gridRanges[sym]; ok && fixed.Upper > fixed.Lower {
		gridCtx.UpperPrice = fixed.Upper
		gridCtx.LowerPrice = fixed.Lower
		gridCtx.GridSpacing = fixed.Spacing
		return
	} else if !(gridCtx.UpperPrice > 0 && gridCtx.LowerPrice > 0 && gridCtx.UpperPrice > gridCtx.LowerPrice) {
		atr := gridCtx.ATR14
		if atr <= 0 {
			atr = gridCtx.CurrentPrice * 0.01
		}
		multiplier := cfg.ATRMultiplier
		if multiplier <= 0 {
			multiplier = 1
		}
		span := atr * multiplier
		if span <= 0 {
			span = gridCtx.CurrentPrice * 0.02
		}
		gridCtx.UpperPrice = gridCtx.CurrentPrice + span
		gridCtx.LowerPrice = gridCtx.CurrentPrice - span
	}

	if cfg.GridCount > 1 {
		gridCtx.GridSpacing = (gridCtx.UpperPrice - gridCtx.LowerPrice) / float64(cfg.GridCount-1)
	}
	r.gridRanges[sym] = gridStaticRange{
		Upper:   gridCtx.UpperPrice,
		Lower:   gridCtx.LowerPrice,
		Spacing: gridCtx.GridSpacing,
	}
}

func (r *Runner) injectGridRuntimeState(symbol string, gridCtx *kernel.GridContext, cfg *store.GridStrategyConfig) {
	if gridCtx == nil || cfg == nil {
		return
	}
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if sym == "" {
		sym = strings.ToUpper(strings.TrimSpace(cfg.Symbol))
	}
	gridCtx.IsPaused = r.gridPaused

	levels := cfg.GridCount
	if levels <= 0 {
		levels = 3
	}
	gridLevels := make([]kernel.GridLevelInfo, 0, levels)
	allocated := 0.0
	if levels > 0 && cfg.TotalInvestment > 0 {
		allocated = cfg.TotalInvestment / float64(levels)
	}
	for i := 0; i < levels; i++ {
		price := gridCtx.LowerPrice + float64(i)*gridCtx.GridSpacing
		side := "buy"
		if price >= gridCtx.CurrentPrice {
			side = "sell"
		}
		gridLevels = append(gridLevels, kernel.GridLevelInfo{
			Index:        i,
			Price:        price,
			State:        "empty",
			Side:         side,
			AllocatedUSD: allocated,
		})
	}

	active := 0
	for _, ord := range r.gridOrders {
		if ord == nil || !strings.EqualFold(ord.Symbol, sym) {
			continue
		}
		active++
		idx := ord.LevelIndex
		if idx < 0 || idx >= len(gridLevels) {
			idx = r.inferGridLevelIndex(sym, ord.Price, cfg)
		}
		if idx < 0 || idx >= len(gridLevels) {
			continue
		}
		lvl := &gridLevels[idx]
		lvl.State = "pending"
		lvl.Side = ord.Side
		lvl.OrderID = ord.ID
		lvl.OrderQuantity = ord.Quantity
		if ord.Price > 0 {
			lvl.Price = ord.Price
		}
	}

	gridCtx.Levels = gridLevels
	gridCtx.ActiveOrderCount = active
	// Filled level count uses current non-zero position as coarse proxy.
	if math.Abs(gridCtx.CurrentPosition) > 0 {
		gridCtx.FilledLevelCount = 1
	} else {
		gridCtx.FilledLevelCount = 0
	}
}

func (r *Runner) inferGridLevelIndex(symbol string, price float64, cfg *store.GridStrategyConfig) int {
	if cfg == nil || price <= 0 {
		return -1
	}
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	fixed, ok := r.gridRanges[sym]
	if !ok || fixed.Upper <= fixed.Lower {
		return -1
	}
	levels := cfg.GridCount
	if levels <= 0 {
		levels = 3
	}
	if levels == 1 || fixed.Spacing <= 0 {
		return 0
	}
	idx := int(math.Round((price - fixed.Lower) / fixed.Spacing))
	if idx < 0 {
		return 0
	}
	if idx >= levels {
		return levels - 1
	}
	return idx
}

func (r *Runner) normalizeGridDecisionsForBacktest(full *kernel.FullDecision, fallbackPrice float64, gridCfg *store.GridStrategyConfig) {
	if full == nil {
		return
	}
	for i := range full.Decisions {
		dec := &full.Decisions[i]
		dec.Action = strings.ToLower(strings.TrimSpace(dec.Action))
		if dec.Symbol == "" && gridCfg != nil {
			dec.Symbol = strings.ToUpper(strings.TrimSpace(gridCfg.Symbol))
		}

		switch dec.Action {
		case "place_buy_limit", "place_sell_limit", "open_long", "open_short":
			r.enrichGridPositionSizing(dec, fallbackPrice, gridCfg)
		case "cancel_order", "cancel_all_orders", "pause_grid", "resume_grid", "adjust_grid", "close_long", "close_short", "hold", "wait":
			// Supported as-is.
		default:
			dec.Action = "hold"
			if dec.Reasoning == "" {
				dec.Reasoning = "unsupported grid action converted to hold in backtest"
			}
		}

		if dec.Leverage <= 0 && gridCfg != nil && gridCfg.Leverage > 0 {
			dec.Leverage = gridCfg.Leverage
		}
	}
}

func (r *Runner) enrichGridPositionSizing(dec *kernel.Decision, fallbackPrice float64, gridCfg *store.GridStrategyConfig) {
	if dec == nil || dec.PositionSizeUSD > 0 {
		return
	}
	price := fallbackPrice
	if dec.Price > 0 {
		price = dec.Price
	}
	if dec.Quantity > 0 && price > 0 {
		dec.PositionSizeUSD = dec.Quantity * price
	}
	if dec.PositionSizeUSD > 0 {
		return
	}
	if gridCfg != nil && gridCfg.TotalInvestment > 0 {
		levels := gridCfg.GridCount
		if levels <= 0 {
			levels = 3
		}
		alloc := gridCfg.TotalInvestment / float64(levels)
		if alloc < MinPositionSizeUSD {
			alloc = MinPositionSizeUSD
		}
		dec.PositionSizeUSD = alloc
	}
}

func (r *Runner) netPositionForSymbol(symbol string) float64 {
	target := strings.ToUpper(strings.TrimSpace(symbol))
	if target == "" {
		return 0
	}
	net := 0.0
	for _, pos := range r.account.Positions() {
		if pos == nil || strings.ToUpper(pos.Symbol) != target {
			continue
		}
		if pos.Side == "long" {
			net += pos.Quantity
		} else if pos.Side == "short" {
			net -= pos.Quantity
		}
	}
	return net
}

func (r *Runner) placeGridOrder(dec kernel.Decision, priceMap map[string]float64) (*gridPendingOrder, string, error) {
	if dec.Symbol == "" {
		return nil, "", fmt.Errorf("grid order symbol cannot be empty")
	}
	symbol := strings.ToUpper(strings.TrimSpace(dec.Symbol))
	price := dec.Price
	if price <= 0 {
		if priceMap == nil {
			return nil, "", fmt.Errorf("grid order requires price or market data")
		}
		marketPrice, ok := priceMap[symbol]
		if !ok || marketPrice <= 0 {
			return nil, "", fmt.Errorf("grid order price unavailable for %s", symbol)
		}
		price = marketPrice
	}
	qty := dec.Quantity
	if qty <= 0 {
		// Fallback to position_size_usd when model omits explicit quantity.
		if dec.PositionSizeUSD > 0 {
			qty = dec.PositionSizeUSD / price
		}
	}
	if qty <= 0 {
		return nil, "", fmt.Errorf("grid order quantity must be > 0")
	}
	levelIndex := dec.LevelIndex
	if levelIndex < 0 {
		var gridCfg *store.GridStrategyConfig
		if cfg := r.strategyEngine.GetConfig(); cfg != nil {
			gridCfg = cfg.GridConfig
		}
		levelIndex = r.inferGridLevelIndex(symbol, price, gridCfg)
	}
	side, intent, closeSide, skipReason := r.planGridOrder(symbol, dec.Action)
	if skipReason != "" {
		return nil, skipReason, nil
	}

	order := &gridPendingOrder{
		ID:         r.gridOrderID(dec, symbol, levelIndex),
		Symbol:     symbol,
		Side:       side,
		Intent:     intent,
		CloseSide:  closeSide,
		Price:      price,
		Quantity:   qty,
		LevelIndex: levelIndex,
		Reasoning:  strings.TrimSpace(dec.Reasoning),
		CreatedAt:  time.Now().UTC().UnixMilli(),
	}
	// Keep existing pending order by same ID to avoid churn/overwriting every cycle.
	// Grid updates should be explicit via cancel_* then place_*.
	if existing, ok := r.gridOrders[order.ID]; ok && existing != nil {
		return existing, "", nil
	}
	r.gridOrders[order.ID] = order
	return order, "", nil
}

func (r *Runner) planGridOrder(symbol, action string) (side, intent, closeSide, skipReason string) {
	side = mapGridOrderSide(action)
	intent = "open"
	closeSide = ""

	regime, ok := r.gridRegimes[strings.ToUpper(strings.TrimSpace(symbol))]
	if !ok || regime.Mode != "trend" {
		return side, intent, closeSide, ""
	}

	switch regime.Direction {
	case "long":
		// Trend-long mode: buy can open/add long; sell can only close long.
		if side == "buy" {
			return "buy", "open", "", ""
		}
		if r.currentSideQuantity(symbol, "long") <= 0 {
			return "", "", "", "trend-long: skip sell limit (no long position to close)"
		}
		return "sell", "close", "long", ""
	case "short":
		// Trend-short mode: sell can open/add short; buy can only close short.
		if side == "sell" {
			return "sell", "open", "", ""
		}
		if r.currentSideQuantity(symbol, "short") <= 0 {
			return "", "", "", "trend-short: skip buy limit (no short position to close)"
		}
		return "buy", "close", "short", ""
	default:
		return side, intent, closeSide, ""
	}
}

func (r *Runner) gridOrderID(dec kernel.Decision, symbol string, level int) string {
	if strings.TrimSpace(dec.OrderID) != "" {
		return strings.TrimSpace(dec.OrderID)
	}
	side := mapGridOrderSide(dec.Action)
	// Use a stable key per symbol-side-level so newer AI decisions update prior pending orders.
	if level >= 0 {
		return fmt.Sprintf("%s:%s:L%d", strings.ToUpper(strings.TrimSpace(symbol)), side, level)
	}
	r.gridOrderSeq++
	return fmt.Sprintf("%s:%s:auto:%d", strings.ToUpper(strings.TrimSpace(symbol)), side, r.gridOrderSeq)
}

func mapGridOrderSide(action string) string {
	if strings.EqualFold(strings.TrimSpace(action), "place_sell_limit") {
		return "sell"
	}
	return "buy"
}

func (r *Runner) cancelGridOrder(dec kernel.Decision) bool {
	if strings.TrimSpace(dec.OrderID) != "" {
		id := strings.TrimSpace(dec.OrderID)
		if _, ok := r.gridOrders[id]; ok {
			delete(r.gridOrders, id)
			return true
		}
		return false
	}

	side := mapGridOrderSide(dec.Action)
	if dec.Action == "cancel_order" && dec.LevelIndex >= 0 {
		id := fmt.Sprintf("%s:%s:L%d", strings.ToUpper(strings.TrimSpace(dec.Symbol)), side, dec.LevelIndex)
		if _, ok := r.gridOrders[id]; ok {
			delete(r.gridOrders, id)
			return true
		}
	}
	return false
}

func (r *Runner) cancelAllGridOrders(symbol string) int {
	target := strings.ToUpper(strings.TrimSpace(symbol))
	removed := 0
	for id, ord := range r.gridOrders {
		if ord == nil {
			delete(r.gridOrders, id)
			continue
		}
		if target == "" || strings.EqualFold(target, "ALL") || ord.Symbol == target {
			delete(r.gridOrders, id)
			removed++
		}
	}
	return removed
}

// recenterGridRange applies dynamic grid recentering in backtest:
// 1) cancel pending orders of the symbol
// 2) keep existing positions unchanged
// 3) rebuild boundaries around a new center (usually current price)
// with threshold + cooldown guardrails to avoid frequent rebuild churn.
func (r *Runner) recenterGridRange(dec kernel.Decision, priceMap map[string]float64, cycle int) (bool, string, error) {
	symbol := strings.ToUpper(strings.TrimSpace(dec.Symbol))
	if symbol == "" {
		return false, "", fmt.Errorf("adjust_grid requires symbol")
	}
	if priceMap == nil {
		return false, "", fmt.Errorf("adjust_grid requires market price map")
	}
	currentPrice, ok := priceMap[symbol]
	if !ok || currentPrice <= 0 {
		return false, "", fmt.Errorf("adjust_grid price unavailable for %s", symbol)
	}

	rng, ok := r.gridRanges[symbol]
	if !ok || rng.Upper <= rng.Lower {
		return false, "adjust_grid skipped: grid range not initialized", nil
	}
	width := rng.Upper - rng.Lower
	if width <= 0 {
		return false, "adjust_grid skipped: invalid range width", nil
	}

	targetCenter := currentPrice
	// Optional: if AI provides price on adjust_grid, treat it as desired center.
	if dec.Price > 0 {
		targetCenter = dec.Price
	}
	oldCenter := (rng.Upper + rng.Lower) / 2
	shift := math.Abs(targetCenter - oldCenter)

	minShift := math.Max(rng.Spacing*0.75, width*gridRecenterMinShiftRatio)
	if minShift <= 0 {
		minShift = currentPrice * 0.0015
	}
	if shift < minShift {
		return false, fmt.Sprintf("adjust_grid skipped: shift %.4f < threshold %.4f", shift, minShift), nil
	}

	if last, ok := r.gridLastRecenterCycle[symbol]; ok {
		if cycle-last < gridRecenterCooldownCycles {
			return false, fmt.Sprintf("adjust_grid cooldown: %d/%d cycles", cycle-last, gridRecenterCooldownCycles), nil
		}
	}

	half := width / 2
	newLower := targetCenter - half
	newUpper := targetCenter + half
	if newUpper <= newLower {
		return false, "adjust_grid skipped: computed invalid range", nil
	}

	levels := 3
	if cfg := r.strategyEngine.GetConfig(); cfg != nil && cfg.GridConfig != nil && cfg.GridConfig.GridCount > 0 {
		levels = cfg.GridConfig.GridCount
	}
	newSpacing := rng.Spacing
	if levels > 1 {
		newSpacing = (newUpper - newLower) / float64(levels-1)
	}

	cancelled := r.cancelAllGridOrders(symbol)
	r.gridRanges[symbol] = gridStaticRange{
		Upper:   newUpper,
		Lower:   newLower,
		Spacing: newSpacing,
	}
	r.gridLastRecenterCycle[symbol] = cycle

	return true, fmt.Sprintf(
		"grid recentered %.4f-%.4f -> %.4f-%.4f; cancelled %d pending",
		rng.Lower, rng.Upper, newLower, newUpper, cancelled,
	), nil
}

func (r *Runner) matchGridOrders(ts int64, cycle int) ([]TradeEvent, []string, error) {
	if len(r.gridOrders) == 0 || r.gridPaused {
		return nil, nil, nil
	}

	events := make([]TradeEvent, 0)
	logs := make([]string, 0)

	for id, ord := range r.gridOrders {
		if ord == nil {
			delete(r.gridOrders, id)
			continue
		}
		curr, _ := r.feed.decisionBarSnapshot(ord.Symbol, ts)
		if curr == nil {
			continue
		}

		triggered := false
		if ord.Side == "buy" && curr.Low <= ord.Price {
			triggered = true
		}
		if ord.Side == "sell" && curr.High >= ord.Price {
			triggered = true
		}
		if !triggered {
			continue
		}

		delete(r.gridOrders, id)
		if strings.EqualFold(ord.Intent, "close") {
			closeSide := strings.ToLower(strings.TrimSpace(ord.CloseSide))
			if closeSide == "" {
				if ord.Side == "sell" {
					closeSide = "long"
				} else {
					closeSide = "short"
				}
			}
			available := r.currentSideQuantity(ord.Symbol, closeSide)
			qty := ord.Quantity
			if qty > available {
				qty = available
			}
			if qty <= 0 {
				logs = append(logs, fmt.Sprintf("grid order %s skipped close: no %s position", id, closeSide))
				continue
			}

			realized, fee, execPrice, err := r.account.Close(ord.Symbol, closeSide, qty, ord.Price)
			if err != nil {
				logs = append(logs, fmt.Sprintf("grid order %s rejected on close fill: %v", id, err))
				continue
			}

			action := "close_long"
			if closeSide == "short" {
				action = "close_short"
			}
			positionAfter := r.currentSideQuantity(ord.Symbol, closeSide)
			slippage := execPrice - ord.Price
			if ord.Side == "buy" {
				slippage = ord.Price - execPrice
			}

			evt := TradeEvent{
				Timestamp:     ts,
				Symbol:        ord.Symbol,
				Action:        action,
				Side:          closeSide,
				Reasoning:     ord.Reasoning,
				Quantity:      qty,
				Price:         execPrice,
				Fee:           fee,
				Slippage:      slippage,
				OrderValue:    execPrice * qty,
				RealizedPnL:   realized,
				Leverage:      r.gridLeverageForSymbol(ord.Symbol),
				Cycle:         cycle,
				PositionAfter: positionAfter,
				Note:          fmt.Sprintf("filled grid close %s limit @ %.4f", ord.Side, ord.Price),
			}
			events = append(events, evt)
			logs = append(logs, fmt.Sprintf("grid order %s filled close_%s %.4f @ %.4f", id, closeSide, qty, execPrice))
			continue
		}

		side := "long"
		action := "open_long"
		if ord.Side == "sell" {
			side = "short"
			action = "open_short"
		}
		leverage := r.gridLeverageForSymbol(ord.Symbol)
		pos, fee, execPrice, err := r.account.Open(ord.Symbol, side, ord.Quantity, leverage, ord.Price, ts)
		if err != nil {
			logs = append(logs, fmt.Sprintf("grid order %s rejected on fill: %v", id, err))
			continue
		}

		slippage := execPrice - ord.Price
		if side == "short" {
			slippage = ord.Price - execPrice
		}

		evt := TradeEvent{
			Timestamp:     ts,
			Symbol:        ord.Symbol,
			Action:        action,
			Side:          side,
			Reasoning:     ord.Reasoning,
			Quantity:      ord.Quantity,
			Price:         execPrice,
			Fee:           fee,
			Slippage:      slippage,
			OrderValue:    execPrice * ord.Quantity,
			RealizedPnL:   0,
			Leverage:      pos.Leverage,
			Cycle:         cycle,
			PositionAfter: pos.Quantity,
			Note:          fmt.Sprintf("filled grid %s limit @ %.4f", ord.Side, ord.Price),
		}
		events = append(events, evt)
		logs = append(logs, fmt.Sprintf("grid order %s filled %s %.4f @ %.4f", id, side, ord.Quantity, execPrice))
	}

	return events, logs, nil
}

func (r *Runner) gridLeverageForSymbol(symbol string) int {
	cfg := r.strategyEngine.GetConfig()
	if cfg != nil && cfg.GridConfig != nil && cfg.GridConfig.Leverage > 0 {
		return r.resolveLeverage(cfg.GridConfig.Leverage, symbol)
	}
	return r.resolveLeverage(0, symbol)
}

func decisionRequiresSymbol(action string) bool {
	switch action {
	case "hold", "wait", "pause_grid", "resume_grid", "cancel_all_orders":
		return false
	default:
		return true
	}
}

func isOpenAction(action string) bool {
	switch action {
	case "open_long", "open_short":
		return true
	default:
		return false
	}
}

func (r *Runner) minConfidenceThreshold() int {
	cfg := r.strategyEngine.GetConfig()
	if cfg == nil {
		return 0
	}
	minConf := cfg.RiskControl.MinConfidence
	if minConf < 0 {
		return 0
	}
	if minConf > 100 {
		return 100
	}
	return minConf
}

func (r *Runner) riskControlConfig() store.RiskControlConfig {
	cfg := r.strategyEngine.GetConfig()
	if cfg != nil {
		return cfg.RiskControl
	}
	return store.GetDefaultStrategyConfig("en").RiskControl
}

func shouldSkipByPriceDeviation(expectedEntry, marketPrice, limitPct float64) bool {
	if expectedEntry <= 0 || marketPrice <= 0 || limitPct <= 0 {
		return false
	}
	deviationPct := math.Abs(marketPrice-expectedEntry) / expectedEntry * 100
	return deviationPct > limitPct
}

func effectiveMinRiskRewardRatio(v float64) float64 {
	if v <= 0 {
		return 3.0
	}
	return v
}

func (r *Runner) setPositionBracket(symbol, side string, stopLoss, takeProfit float64) {
	if r == nil {
		return
	}
	key := positionKey(strings.ToUpper(strings.TrimSpace(symbol)), strings.ToLower(strings.TrimSpace(side)))
	if stopLoss <= 0 && takeProfit <= 0 {
		delete(r.positionBrackets, key)
		return
	}
	r.positionBrackets[key] = positionBracket{
		StopLoss:   stopLoss,
		TakeProfit: takeProfit,
	}
}

func (r *Runner) getPositionBracket(symbol, side string) (positionBracket, bool) {
	if r == nil {
		return positionBracket{}, false
	}
	key := positionKey(strings.ToUpper(strings.TrimSpace(symbol)), strings.ToLower(strings.TrimSpace(side)))
	bracket, ok := r.positionBrackets[key]
	return bracket, ok
}

func (r *Runner) clearPositionBracket(symbol, side string) {
	if r == nil {
		return
	}
	key := positionKey(strings.ToUpper(strings.TrimSpace(symbol)), strings.ToLower(strings.TrimSpace(side)))
	delete(r.positionBrackets, key)
}

func (r *Runner) checkStopTakeProfitTriggers(ts int64, cycle int) ([]TradeEvent, []string, error) {
	positions := append([]*position(nil), r.account.Positions()...)
	events := make([]TradeEvent, 0)
	logs := make([]string, 0)

	for _, pos := range positions {
		if pos == nil || pos.Quantity <= 0 {
			continue
		}
		bracket, ok := r.getPositionBracket(pos.Symbol, pos.Side)
		if !ok {
			continue
		}

		curr, _ := r.feed.decisionBarSnapshot(pos.Symbol, ts)
		if curr == nil {
			continue
		}

		triggerReason := ""
		triggerPrice := 0.0
		// Gap-aware fill resolution:
		//  - If the bar opens already past a bracket level, the order fills at Open
		//    (worst-case for stop-loss, realized at Open for take-profit), because
		//    the first trade of the bar is at/near the Open and the trigger price
		//    never actually traded. Using the bracket price understates gap losses
		//    and overstates gap-through take-profit gains.
		//  - Otherwise fall back to the bracket price for intrabar touches.
		//  - Conservative tie-break: if both sides are touched and there is no
		//    gap at Open, treat as stop-loss (unchanged behavior).
		if pos.Side == "long" {
			gapStop := bracket.StopLoss > 0 && curr.Open > 0 && curr.Open <= bracket.StopLoss
			gapTP := bracket.TakeProfit > 0 && curr.Open > 0 && curr.Open >= bracket.TakeProfit
			stopHit := bracket.StopLoss > 0 && curr.Low > 0 && curr.Low <= bracket.StopLoss
			tpHit := bracket.TakeProfit > 0 && curr.High > 0 && curr.High >= bracket.TakeProfit
			switch {
			case gapStop:
				triggerReason = "stop_loss"
				triggerPrice = curr.Open
			case gapTP:
				triggerReason = "take_profit"
				triggerPrice = curr.Open
			case stopHit:
				triggerReason = "stop_loss"
				triggerPrice = bracket.StopLoss
			case tpHit:
				triggerReason = "take_profit"
				triggerPrice = bracket.TakeProfit
			}
		} else {
			gapStop := bracket.StopLoss > 0 && curr.Open > 0 && curr.Open >= bracket.StopLoss
			gapTP := bracket.TakeProfit > 0 && curr.Open > 0 && curr.Open <= bracket.TakeProfit
			stopHit := bracket.StopLoss > 0 && curr.High > 0 && curr.High >= bracket.StopLoss
			tpHit := bracket.TakeProfit > 0 && curr.Low > 0 && curr.Low <= bracket.TakeProfit
			switch {
			case gapStop:
				triggerReason = "stop_loss"
				triggerPrice = curr.Open
			case gapTP:
				triggerReason = "take_profit"
				triggerPrice = curr.Open
			case stopHit:
				triggerReason = "stop_loss"
				triggerPrice = bracket.StopLoss
			case tpHit:
				triggerReason = "take_profit"
				triggerPrice = bracket.TakeProfit
			}
		}

		if triggerReason == "" || triggerPrice <= 0 {
			continue
		}

		closeQty := pos.Quantity
		realized, fee, execPrice, err := r.account.Close(pos.Symbol, pos.Side, closeQty, triggerPrice)
		if err != nil {
			return nil, nil, fmt.Errorf("protection close %s %s failed: %w", pos.Symbol, pos.Side, err)
		}

		action := "close_long"
		slippage := triggerPrice - execPrice
		if pos.Side == "short" {
			action = "close_short"
			slippage = execPrice - triggerPrice
		}

		events = append(events, TradeEvent{
			Timestamp:     ts,
			Symbol:        pos.Symbol,
			Action:        action,
			Side:          pos.Side,
			Reasoning:     "triggered by protective order",
			Quantity:      closeQty,
			Price:         execPrice,
			StopLoss:      bracket.StopLoss,
			TakeProfit:    bracket.TakeProfit,
			Fee:           fee,
			Slippage:      slippage,
			OrderValue:    execPrice * closeQty,
			RealizedPnL:   realized - fee,
			Leverage:      pos.Leverage,
			Cycle:         cycle,
			PositionAfter: 0,
			Note:          triggerReason,
		})
		logs = append(logs, fmt.Sprintf("Protection trigger: %s %s %s @ %.4f (bar H/L: %.4f / %.4f)",
			pos.Symbol, pos.Side, triggerReason, execPrice, curr.High, curr.Low))
		r.clearPositionBracket(pos.Symbol, pos.Side)
	}

	return events, logs, nil
}

func (r *Runner) executeDecision(dec kernel.Decision, priceMap map[string]float64, ts int64, cycle int) (store.DecisionAction, []TradeEvent, string, error) {
	action := strings.ToLower(strings.TrimSpace(dec.Action))
	symbol := strings.ToUpper(strings.TrimSpace(dec.Symbol))
	reasoning := strings.TrimSpace(dec.Reasoning)

	if decisionRequiresSymbol(action) && symbol == "" {
		return store.DecisionAction{}, nil, "", fmt.Errorf("empty symbol in decision")
	}

	dec.Action = action
	dec.Symbol = symbol
	dec.Reasoning = reasoning

	usedLeverage := r.resolveLeverage(dec.Leverage, symbol)
	actionRecord := store.DecisionAction{
		Action:          dec.Action,
		Symbol:          symbol,
		Leverage:        usedLeverage,
		EntryPrice:      dec.EntryPrice,
		PositionSizeUSD: dec.PositionSizeUSD,
		StopLoss:        dec.StopLoss,
		TakeProfit:      dec.TakeProfit,
		Confidence:      dec.Confidence,
		Reasoning:       dec.Reasoning,
		Timestamp:       time.UnixMilli(ts).UTC(),
	}

	if isOpenAction(dec.Action) {
		minConf := r.minConfidenceThreshold()
		if minConf > 0 && dec.Confidence < minConf {
			return actionRecord, nil, fmt.Sprintf("skip %s: confidence %d < min %d", dec.Action, dec.Confidence, minConf), nil
		}
	}

	// Safe fallback decision may return Symbol=ALL + Action=wait.
	// hold/wait should not depend on symbol price availability.
	switch dec.Action {
	case "hold", "wait":
		return actionRecord, nil, fmt.Sprintf("hold position: %s", dec.Action), nil
	case "pause_grid":
		r.gridPaused = true
		return actionRecord, nil, "grid paused", nil
	case "resume_grid":
		r.gridPaused = false
		return actionRecord, nil, "grid resumed", nil
	case "cancel_all_orders":
		cancelled := r.cancelAllGridOrders(symbol)
		return actionRecord, nil, fmt.Sprintf("cancelled %d grid orders", cancelled), nil
	case "cancel_order":
		removed := r.cancelGridOrder(dec)
		if removed {
			return actionRecord, nil, "grid order cancelled", nil
		}
		return actionRecord, nil, "grid order not found", nil
	case "adjust_grid":
		_, msg, err := r.recenterGridRange(dec, priceMap, cycle)
		if err != nil {
			return actionRecord, nil, "", err
		}
		return actionRecord, nil, msg, nil
	case "place_buy_limit", "place_sell_limit":
		if r.gridPaused {
			return actionRecord, nil, "grid paused; skip placing order", nil
		}
		order, skipReason, err := r.placeGridOrder(dec, priceMap)
		if err != nil {
			return actionRecord, nil, "", err
		}
		if order == nil {
			if skipReason == "" {
				skipReason = "grid order skipped"
			}
			return actionRecord, nil, skipReason, nil
		}
		actionRecord.Quantity = order.Quantity
		actionRecord.Price = order.Price
		return actionRecord, nil, fmt.Sprintf("grid %s limit order placed", order.Intent), nil
	}

	if priceMap == nil {
		return actionRecord, nil, "", fmt.Errorf("priceMap is nil")
	}

	basePrice, ok := priceMap[symbol]
	if !ok || basePrice <= 0 {
		return actionRecord, nil, "", fmt.Errorf("price unavailable for %s (found=%v, price=%.4f)", symbol, ok, basePrice)
	}
	fillPrice := r.executionPrice(symbol, basePrice, ts)
	riskControl := r.riskControlConfig()
	isGridMode := r.isGridBacktestStrategy()

	switch dec.Action {
	case "open_long":
		if shouldSkipByPriceDeviation(dec.EntryPrice, fillPrice, riskControl.EffectivePriceDeviationLimitPct()) {
			msg := fmt.Sprintf("skip open_long: entry deviation too high (ai=%.6f fill=%.6f limit=%.2f%%)",
				dec.EntryPrice, fillPrice, riskControl.EffectivePriceDeviationLimitPct())
			return actionRecord, nil, msg, nil
		}
		qty := r.determineOpenQuantity(dec, "long", basePrice, isGridMode)
		if qty <= 0 {
			if isGridMode {
				return actionRecord, nil, "grid long target already satisfied or insufficient free margin", nil
			}
			return actionRecord, nil, "", fmt.Errorf("invalid qty")
		}
		pos, fee, execPrice, err := r.account.Open(symbol, "long", qty, usedLeverage, fillPrice, ts)
		if err != nil {
			return actionRecord, nil, "", err
		}
		actionRecord.Quantity = qty
		actionRecord.Price = execPrice
		actionRecord.Leverage = pos.Leverage
		trade := TradeEvent{
			Timestamp:     ts,
			Symbol:        symbol,
			Action:        dec.Action,
			Side:          "long",
			Reasoning:     strings.TrimSpace(dec.Reasoning),
			Quantity:      qty,
			Price:         execPrice,
			StopLoss:      dec.StopLoss,
			TakeProfit:    dec.TakeProfit,
			Fee:           fee,
			Slippage:      execPrice - basePrice,
			OrderValue:    execPrice * qty,
			RealizedPnL:   0,
			Leverage:      pos.Leverage,
			Cycle:         cycle,
			PositionAfter: pos.Quantity,
		}
		trades := []TradeEvent{trade}
		logEntry := ""

		if riskControl.EffectivePostFillRRRecheckEnabled() {
			minRR := effectiveMinRiskRewardRatio(riskControl.MinRiskRewardRatio)
			threshold := minRR - riskControl.EffectivePostFillRRTolerance()
			if threshold < 0 {
				threshold = 0
			}
			rr, riskPct, rewardPct, ok := store.CalculateRiskReward(dec.Action, execPrice, dec.StopLoss, dec.TakeProfit)
			if !ok || rr < threshold {
				onFail := riskControl.EffectivePostFillRROnFail()
				logEntry = fmt.Sprintf("post-fill RR check failed: rr=%.2f threshold=%.2f risk=%.2f%% reward=%.2f%% action=%s",
					rr, threshold, riskPct, rewardPct, onFail)
				switch onFail {
				case store.PostFillRROnFailAdjustTP:
					if newTP, adjusted := store.AdjustTakeProfitForMinRR(dec.Action, execPrice, dec.StopLoss, minRR); adjusted && newTP > 0 {
						dec.TakeProfit = newTP
						actionRecord.TakeProfit = newTP
						trades[0].TakeProfit = newTP
						logEntry = logEntry + fmt.Sprintf("; adjusted tp=%.6f", newTP)
					}
				case store.PostFillRROnFailCloseImmediately:
					realized, closeFee, closePrice, closeErr := r.account.Close(symbol, "long", qty, execPrice)
					if closeErr != nil {
						return actionRecord, trades, "", closeErr
					}
					closeTrade := TradeEvent{
						Timestamp:     ts,
						Symbol:        symbol,
						Action:        "close_long",
						Side:          "long",
						Reasoning:     "post-fill RR guard close_immediately",
						Quantity:      qty,
						Price:         closePrice,
						StopLoss:      dec.StopLoss,
						TakeProfit:    dec.TakeProfit,
						Fee:           closeFee,
						Slippage:      execPrice - closePrice,
						OrderValue:    closePrice * qty,
						RealizedPnL:   realized - closeFee,
						Leverage:      pos.Leverage,
						Cycle:         cycle,
						PositionAfter: r.remainingPosition(symbol, "long"),
					}
					trades = append(trades, closeTrade)
					logEntry = logEntry + "; closed immediately"
					r.clearPositionBracket(symbol, "long")
				case store.PostFillRROnFailAlertOnly:
					logEntry = logEntry + "; alert_only"
				}
			}
		}

		if r.remainingPosition(symbol, "long") > 0 {
			r.setPositionBracket(symbol, "long", dec.StopLoss, dec.TakeProfit)
		}
		return actionRecord, trades, logEntry, nil

	case "open_short":
		if shouldSkipByPriceDeviation(dec.EntryPrice, fillPrice, riskControl.EffectivePriceDeviationLimitPct()) {
			msg := fmt.Sprintf("skip open_short: entry deviation too high (ai=%.6f fill=%.6f limit=%.2f%%)",
				dec.EntryPrice, fillPrice, riskControl.EffectivePriceDeviationLimitPct())
			return actionRecord, nil, msg, nil
		}
		qty := r.determineOpenQuantity(dec, "short", basePrice, isGridMode)
		if qty <= 0 {
			if isGridMode {
				return actionRecord, nil, "grid short target already satisfied or insufficient free margin", nil
			}
			return actionRecord, nil, "", fmt.Errorf("invalid qty")
		}
		pos, fee, execPrice, err := r.account.Open(symbol, "short", qty, usedLeverage, fillPrice, ts)
		if err != nil {
			return actionRecord, nil, "", err
		}
		actionRecord.Quantity = qty
		actionRecord.Price = execPrice
		actionRecord.Leverage = pos.Leverage
		trade := TradeEvent{
			Timestamp:     ts,
			Symbol:        symbol,
			Action:        dec.Action,
			Side:          "short",
			Reasoning:     strings.TrimSpace(dec.Reasoning),
			Quantity:      qty,
			Price:         execPrice,
			StopLoss:      dec.StopLoss,
			TakeProfit:    dec.TakeProfit,
			Fee:           fee,
			Slippage:      basePrice - execPrice,
			OrderValue:    execPrice * qty,
			RealizedPnL:   0,
			Leverage:      pos.Leverage,
			Cycle:         cycle,
			PositionAfter: pos.Quantity,
		}
		trades := []TradeEvent{trade}
		logEntry := ""

		if riskControl.EffectivePostFillRRRecheckEnabled() {
			minRR := effectiveMinRiskRewardRatio(riskControl.MinRiskRewardRatio)
			threshold := minRR - riskControl.EffectivePostFillRRTolerance()
			if threshold < 0 {
				threshold = 0
			}
			rr, riskPct, rewardPct, ok := store.CalculateRiskReward(dec.Action, execPrice, dec.StopLoss, dec.TakeProfit)
			if !ok || rr < threshold {
				onFail := riskControl.EffectivePostFillRROnFail()
				logEntry = fmt.Sprintf("post-fill RR check failed: rr=%.2f threshold=%.2f risk=%.2f%% reward=%.2f%% action=%s",
					rr, threshold, riskPct, rewardPct, onFail)
				switch onFail {
				case store.PostFillRROnFailAdjustTP:
					if newTP, adjusted := store.AdjustTakeProfitForMinRR(dec.Action, execPrice, dec.StopLoss, minRR); adjusted && newTP > 0 {
						dec.TakeProfit = newTP
						actionRecord.TakeProfit = newTP
						trades[0].TakeProfit = newTP
						logEntry = logEntry + fmt.Sprintf("; adjusted tp=%.6f", newTP)
					}
				case store.PostFillRROnFailCloseImmediately:
					realized, closeFee, closePrice, closeErr := r.account.Close(symbol, "short", qty, execPrice)
					if closeErr != nil {
						return actionRecord, trades, "", closeErr
					}
					closeTrade := TradeEvent{
						Timestamp:     ts,
						Symbol:        symbol,
						Action:        "close_short",
						Side:          "short",
						Reasoning:     "post-fill RR guard close_immediately",
						Quantity:      qty,
						Price:         closePrice,
						StopLoss:      dec.StopLoss,
						TakeProfit:    dec.TakeProfit,
						Fee:           closeFee,
						Slippage:      closePrice - execPrice,
						OrderValue:    closePrice * qty,
						RealizedPnL:   realized - closeFee,
						Leverage:      pos.Leverage,
						Cycle:         cycle,
						PositionAfter: r.remainingPosition(symbol, "short"),
					}
					trades = append(trades, closeTrade)
					logEntry = logEntry + "; closed immediately"
					r.clearPositionBracket(symbol, "short")
				case store.PostFillRROnFailAlertOnly:
					logEntry = logEntry + "; alert_only"
				}
			}
		}

		if r.remainingPosition(symbol, "short") > 0 {
			r.setPositionBracket(symbol, "short", dec.StopLoss, dec.TakeProfit)
		}
		return actionRecord, trades, logEntry, nil

	case "close_long":
		qty := r.determineCloseQuantity(symbol, "long", dec)
		if qty <= 0 {
			return actionRecord, nil, "", fmt.Errorf("invalid close qty")
		}
		posLev := r.account.positionLeverage(symbol, "long")
		realized, fee, execPrice, err := r.account.Close(symbol, "long", qty, fillPrice)
		if err != nil {
			return actionRecord, nil, "", err
		}
		actionRecord.Quantity = qty
		actionRecord.Price = execPrice
		actionRecord.Leverage = posLev
		trade := TradeEvent{
			Timestamp:     ts,
			Symbol:        symbol,
			Action:        dec.Action,
			Side:          "long",
			Reasoning:     strings.TrimSpace(dec.Reasoning),
			Quantity:      qty,
			Price:         execPrice,
			StopLoss:      dec.StopLoss,
			TakeProfit:    dec.TakeProfit,
			Fee:           fee,
			Slippage:      basePrice - execPrice,
			OrderValue:    execPrice * qty,
			RealizedPnL:   realized - fee,
			Leverage:      posLev,
			Cycle:         cycle,
			PositionAfter: r.remainingPosition(symbol, "long"),
		}
		if r.remainingPosition(symbol, "long") <= 0 {
			r.clearPositionBracket(symbol, "long")
		}
		return actionRecord, []TradeEvent{trade}, "", nil

	case "close_short":
		qty := r.determineCloseQuantity(symbol, "short", dec)
		if qty <= 0 {
			return actionRecord, nil, "", fmt.Errorf("invalid close qty")
		}
		posLev := r.account.positionLeverage(symbol, "short")
		realized, fee, execPrice, err := r.account.Close(symbol, "short", qty, fillPrice)
		if err != nil {
			return actionRecord, nil, "", err
		}
		actionRecord.Quantity = qty
		actionRecord.Price = execPrice
		actionRecord.Leverage = posLev
		trade := TradeEvent{
			Timestamp:     ts,
			Symbol:        symbol,
			Action:        dec.Action,
			Side:          "short",
			Reasoning:     strings.TrimSpace(dec.Reasoning),
			Quantity:      qty,
			Price:         execPrice,
			StopLoss:      dec.StopLoss,
			TakeProfit:    dec.TakeProfit,
			Fee:           fee,
			Slippage:      execPrice - basePrice,
			OrderValue:    execPrice * qty,
			RealizedPnL:   realized - fee,
			Leverage:      posLev,
			Cycle:         cycle,
			PositionAfter: r.remainingPosition(symbol, "short"),
		}
		if r.remainingPosition(symbol, "short") <= 0 {
			r.clearPositionBracket(symbol, "short")
		}
		return actionRecord, []TradeEvent{trade}, "", nil

	default:
		return actionRecord, nil, "", fmt.Errorf("unsupported action %s", dec.Action)
	}
}

func (r *Runner) isGridBacktestStrategy() bool {
	cfg := r.strategyEngine.GetConfig()
	return cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.StrategyType), "grid_trading") && cfg.GridConfig != nil
}

func (r *Runner) cacheVariantKey() string {
	cfg := r.strategyEngine.GetConfig()
	if cfg == nil {
		return r.cfg.PromptVariant
	}
	scope := strings.ToLower(strings.TrimSpace(cfg.StrategyType))
	if scope == "" {
		scope = "ai_trading"
	}
	gridSymbol := ""
	if cfg.GridConfig != nil {
		gridSymbol = strings.ToUpper(strings.TrimSpace(cfg.GridConfig.Symbol))
	}
	// Add strategy scope to prevent cross-strategy cache pollution.
	return fmt.Sprintf("%s|scope:%s|grid:%s|strategy:%s", r.cfg.PromptVariant, scope, gridSymbol, strings.TrimSpace(r.cfg.StrategyID))
}

func isGridDecisionCache(fd *kernel.FullDecision) bool {
	if fd == nil {
		return false
	}
	sp := strings.ToLower(fd.SystemPrompt)
	if strings.Contains(sp, "grid trading ai") || strings.Contains(sp, "网格交易ai") {
		return true
	}
	for _, d := range fd.Decisions {
		a := strings.ToLower(strings.TrimSpace(d.Action))
		if a == "place_buy_limit" || a == "place_sell_limit" || a == "pause_grid" || a == "resume_grid" {
			return true
		}
	}
	return false
}

func shouldPersistAICacheDecision(fd *kernel.FullDecision) bool {
	if fd == nil {
		return false
	}
	if len(fd.Decisions) == 0 {
		return false
	}
	// Do not cache parser fallback decisions; they may mask transient model errors
	// and should not poison future replay runs.
	if len(fd.Decisions) == 1 {
		d := fd.Decisions[0]
		if strings.EqualFold(strings.TrimSpace(d.Action), "hold") &&
			strings.Contains(strings.ToLower(d.Reasoning), "failed to parse ai response") {
			return false
		}
	}
	return true
}

func extractUpstreamRequestIDFromError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	prefixes := []string{
		"request_id=",
		"requestId=",
		"request-id=",
		"x-request-id=",
	}
	for _, prefix := range prefixes {
		idx := strings.Index(msg, prefix)
		if idx < 0 {
			continue
		}
		start := idx + len(prefix)
		end := start
		for end < len(msg) {
			ch := msg[end]
			if ch == ',' || ch == ')' || ch == ' ' || ch == '\n' || ch == '\r' || ch == ';' || ch == '"' || ch == '\'' {
				break
			}
			end++
		}
		if end > start {
			return strings.Trim(msg[start:end], "\"'")
		}
	}
	return ""
}

func (r *Runner) determineOpenQuantity(dec kernel.Decision, side string, price float64, isGridMode bool) float64 {
	affordable := r.determineQuantity(dec, price)
	if affordable <= 0 {
		return 0
	}
	if !isGridMode {
		return affordable
	}

	// In grid mode we treat AI quantity as target exposure for this side, not perpetual add-on.
	if dec.Quantity > 0 {
		existing := r.currentSideQuantity(dec.Symbol, side)
		delta := dec.Quantity - existing
		if delta <= 0 {
			return 0
		}
		if delta > affordable {
			return affordable
		}
		return delta
	}

	return affordable
}

func (r *Runner) currentSideQuantity(symbol, side string) float64 {
	targetSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	targetSide := strings.ToLower(strings.TrimSpace(side))
	total := 0.0
	for _, pos := range r.account.Positions() {
		if pos == nil {
			continue
		}
		if strings.ToUpper(pos.Symbol) == targetSymbol && strings.ToLower(pos.Side) == targetSide {
			total += pos.Quantity
		}
	}
	return total
}

// MinPositionSizeUSD is the minimum position size in USD to avoid dust positions
const MinPositionSizeUSD = 12.0

func (r *Runner) determineQuantity(dec kernel.Decision, price float64) float64 {
	snapshot := r.snapshotState()
	equity := snapshot.Equity
	if equity <= 0 {
		equity = r.account.InitialBalance()
	}

	symbol := strings.ToUpper(strings.TrimSpace(dec.Symbol))
	isBTCETH := symbol == "BTCUSDT" || symbol == "ETHUSDT"

	// Resolve sizing constraints from strategy risk config.
	minPositionSize := MinPositionSizeUSD
	maxMarginUsage := 0.9
	btcEthPosRatio := 5.0
	altcoinPosRatio := 1.0
	if cfg := r.strategyEngine.GetConfig(); cfg != nil {
		rc := cfg.RiskControl
		if rc.MinPositionSize > 0 {
			minPositionSize = rc.MinPositionSize
		}
		if rc.MaxMarginUsage > 0 && rc.MaxMarginUsage <= 1 {
			maxMarginUsage = rc.MaxMarginUsage
		}
		if rc.BTCETHMaxPositionValueRatio > 0 {
			btcEthPosRatio = rc.BTCETHMaxPositionValueRatio
		}
		if rc.AltcoinMaxPositionValueRatio > 0 {
			altcoinPosRatio = rc.AltcoinMaxPositionValueRatio
		}
	}

	// Get leverage for this symbol.
	leverage := r.resolveLeverage(dec.Leverage, dec.Symbol)
	if leverage <= 0 {
		leverage = 5
	}

	// Calculate margin headroom by configured max margin usage against total equity.
	// This matches the intended semantics: total margin used / equity <= max_margin_usage.
	availableCash := r.account.Cash()
	currentMarginUsed := r.totalMarginUsed()
	maxAllowedMargin := equity * maxMarginUsage
	marginHeadroom := maxAllowedMargin - currentMarginUsed
	if marginHeadroom <= 0 {
		logger.Infof("Backtest: rejecting %s position, no margin headroom (used %.2f / allowed %.2f)",
			dec.Symbol, currentMarginUsed, maxAllowedMargin)
		return 0
	}
	if marginHeadroom > availableCash {
		marginHeadroom = availableCash
	}
	if marginHeadroom <= 0 {
		logger.Infof("Backtest: rejecting %s position, no available cash headroom", dec.Symbol)
		return 0
	}
	maxPositionValue := marginHeadroom * float64(leverage)

	sizeUSD := dec.PositionSizeUSD
	if sizeUSD <= 0 {
		// Default to 5% of equity, then cap by risk and affordability.
		sizeUSD = 0.05 * equity
	}

	// [CODE ENFORCED] Align with live validator: single-position value cap by equity ratio.
	posRatio := altcoinPosRatio
	if isBTCETH {
		posRatio = btcEthPosRatio
	}
	maxByRiskRatio := equity * posRatio
	if maxByRiskRatio > 0 && sizeUSD > maxByRiskRatio {
		logger.Infof("Backtest: capping %s position from %.2f to %.2f by risk ratio %.2fx (equity: %.2f)",
			dec.Symbol, sizeUSD, maxByRiskRatio, posRatio, equity)
		sizeUSD = maxByRiskRatio
	}

	// Cap position size to what we can actually afford.
	if sizeUSD > maxPositionValue {
		logger.Infof("Backtest: capping position from %.2f to %.2f (margin headroom: %.2f, leverage: %dx)",
			sizeUSD, maxPositionValue, marginHeadroom, leverage)
		sizeUSD = maxPositionValue
	}

	// Reject positions below minimum size.
	if sizeUSD < minPositionSize {
		logger.Infof("Backtest: rejecting %s position size %.2f USD (below minimum %.2f USD)",
			dec.Symbol, sizeUSD, minPositionSize)
		return 0
	}

	qty := sizeUSD / price
	if qty < 0 {
		qty = 0
	}
	return qty
}

func (r *Runner) determineCloseQuantity(symbol, side string, dec kernel.Decision) float64 {
	for _, pos := range r.account.Positions() {
		if pos.Symbol == strings.ToUpper(symbol) && pos.Side == side {
			return pos.Quantity
		}
	}
	return 0
}

func (r *Runner) resolveLeverage(requested int, symbol string) int {
	sym := strings.ToUpper(symbol)
	isBTCETH := sym == "BTCUSDT" || sym == "ETHUSDT"

	// Determine configured max leverage for this symbol type
	var maxLeverage int
	if isBTCETH {
		maxLeverage = r.cfg.Leverage.BTCETHLeverage
		if maxLeverage <= 0 {
			maxLeverage = 10 // Default max for BTC/ETH
		}
	} else {
		maxLeverage = r.cfg.Leverage.AltcoinLeverage
		if maxLeverage <= 0 {
			maxLeverage = 5 // Default max for altcoins
		}
	}

	// Use requested leverage if provided, otherwise use max as default
	leverage := requested
	if leverage <= 0 {
		leverage = maxLeverage
	}

	// Enforce max leverage limit
	if leverage > maxLeverage {
		logger.Infof("📊 Backtest: capping leverage from %dx to %dx for %s",
			leverage, maxLeverage, symbol)
		leverage = maxLeverage
	}

	return leverage
}

func (r *Runner) remainingPosition(symbol, side string) float64 {
	for _, pos := range r.account.Positions() {
		if pos.Symbol == strings.ToUpper(symbol) && pos.Side == side {
			return pos.Quantity
		}
	}
	return 0
}

func (r *Runner) snapshotPositions(priceMap map[string]float64) []store.PositionSnapshot {
	positions := r.account.Positions()
	list := make([]store.PositionSnapshot, 0, len(positions))
	for _, pos := range positions {
		price := priceMap[pos.Symbol]
		list = append(list, store.PositionSnapshot{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			PositionAmt:      pos.Quantity,
			EntryPrice:       pos.EntryPrice,
			MarkPrice:        price,
			UnrealizedProfit: unrealizedPnL(pos, price),
			Leverage:         float64(pos.Leverage),
			LiquidationPrice: pos.LiquidationPrice,
		})
	}
	return list
}

func (r *Runner) convertPositions(priceMap map[string]float64) []kernel.PositionInfo {
	positions := r.account.Positions()
	list := make([]kernel.PositionInfo, 0, len(positions))
	for _, pos := range positions {
		price := priceMap[pos.Symbol]
		pnl := unrealizedPnL(pos, price)
		// Calculate P&L percentage based on entry notional (position cost)
		pnlPct := 0.0
		if pos.Notional > 0 {
			pnlPct = (pnl / pos.Notional) * 100
		}
		list = append(list, kernel.PositionInfo{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			EntryPrice:       pos.EntryPrice,
			MarkPrice:        price,
			Quantity:         pos.Quantity,
			Leverage:         pos.Leverage,
			UnrealizedPnL:    pnl,
			UnrealizedPnLPct: pnlPct,
			LiquidationPrice: pos.LiquidationPrice,
			MarginUsed:       pos.Margin,
			UpdateTime:       time.Now().UnixMilli(),
		})
	}
	return list
}

func (r *Runner) executionPrice(symbol string, markPrice float64, ts int64) float64 {
	curr, next := r.feed.decisionBarSnapshot(symbol, ts)
	switch r.cfg.FillPolicy {
	case FillPolicyNextOpen:
		if next != nil && next.Open > 0 {
			return next.Open
		}
	case FillPolicyBarVWAP:
		if curr != nil {
			if vwap := barVWAP(*curr); vwap > 0 {
				return vwap
			}
		}
	case FillPolicyMidPrice:
		if curr != nil && curr.High > 0 && curr.Low > 0 {
			return (curr.High + curr.Low) / 2
		}
	}
	return markPrice
}

func (r *Runner) totalMarginUsed() float64 {
	sum := 0.0
	for _, pos := range r.account.Positions() {
		sum += pos.Margin
	}
	return sum
}

func (r *Runner) updateState(ts int64, equity, unrealized, marginUsed float64, priceMap map[string]float64, advancedDecision bool) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	if r.state.MaxEquity == 0 || equity > r.state.MaxEquity {
		r.state.MaxEquity = equity
	}
	if r.state.MinEquity == 0 || equity < r.state.MinEquity {
		r.state.MinEquity = equity
	}
	if r.state.MaxEquity > 0 {
		drawdown := ((r.state.MaxEquity - equity) / r.state.MaxEquity) * 100
		if drawdown > r.state.MaxDrawdownPct {
			r.state.MaxDrawdownPct = drawdown
		}
	}

	positions := make(map[string]PositionSnapshot)
	for _, pos := range r.account.Positions() {
		key := fmt.Sprintf("%s:%s", pos.Symbol, pos.Side)
		positions[key] = PositionSnapshot{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			Quantity:         pos.Quantity,
			AvgPrice:         pos.EntryPrice,
			Leverage:         pos.Leverage,
			LiquidationPrice: pos.LiquidationPrice,
			MarginUsed:       pos.Margin,
			OpenTime:         pos.OpenTime,
			AccumulatedFee:   pos.AccumulatedFee,
		}
	}

	r.state.BarTimestamp = ts
	r.state.BarIndex++
	if advancedDecision {
		r.state.DecisionCycle++
	}
	r.state.Cash = r.account.Cash()
	r.state.Equity = equity
	r.state.UnrealizedPnL = unrealized
	r.state.RealizedPnL = r.account.RealizedPnL()
	r.state.Positions = positions
	r.state.LastUpdate = time.Now().UTC()
}

func (r *Runner) maybeCheckpoint() error {
	state := r.snapshotState()
	shouldCheckpoint := false

	if r.cfg.CheckpointIntervalBars > 0 && state.BarIndex > 0 && state.BarIndex%r.cfg.CheckpointIntervalBars == 0 {
		shouldCheckpoint = true
	}

	interval := time.Duration(r.cfg.CheckpointIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if time.Since(r.lastCheckpoint) >= interval {
		shouldCheckpoint = true
	}

	if !shouldCheckpoint {
		return nil
	}

	if err := r.saveCheckpoint(state); err != nil {
		return err
	}

	return nil
}

func (r *Runner) snapshotForCheckpoint(state BacktestState) []PositionSnapshot {
	res := make([]PositionSnapshot, 0, len(state.Positions))
	for _, pos := range state.Positions {
		res = append(res, pos)
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].Symbol == res[j].Symbol {
			return res[i].Side < res[j].Side
		}
		return res[i].Symbol < res[j].Symbol
	})
	return res
}

func (r *Runner) atrStopConfig() (bool, float64) {
	cfg := r.strategyEngine.GetConfig()
	if cfg == nil {
		return false, 0
	}
	rc := cfg.RiskControl
	if !rc.ATRStopEnabled {
		return false, 0
	}
	multiplier := rc.ATRStopMultiplier
	if multiplier <= 0 {
		multiplier = 1.5
	}
	return true, multiplier
}

func resolveBacktestATR14(data *market.Data) float64 {
	if data == nil {
		return 0
	}
	if data.IntradaySeries != nil && data.IntradaySeries.ATR14 > 0 {
		return data.IntradaySeries.ATR14
	}
	if data.LongerTermContext != nil {
		if data.LongerTermContext.ATR14 > 0 {
			return data.LongerTermContext.ATR14
		}
		if data.LongerTermContext.ATR3 > 0 {
			return data.LongerTermContext.ATR3
		}
	}
	return 0
}

func (r *Runner) checkATRVolatilityStops(ts int64, marketData map[string]*market.Data, priceMap map[string]float64, cycle int) ([]TradeEvent, []string, error) {
	enabled, multiplier := r.atrStopConfig()
	if !enabled {
		return nil, nil, nil
	}

	positions := append([]*position(nil), r.account.Positions()...)
	events := make([]TradeEvent, 0)
	logs := make([]string, 0)

	for _, pos := range positions {
		if pos == nil || pos.Quantity <= 0 {
			continue
		}
		price := priceMap[pos.Symbol]
		if price <= 0 {
			price = pos.EntryPrice
		}
		atr := resolveBacktestATR14(marketData[pos.Symbol])
		if atr <= 0 {
			continue
		}

		stopDistance := atr * multiplier
		stopPrice := 0.0
		triggered := false
		if pos.Side == "long" {
			stopPrice = pos.EntryPrice - stopDistance
			triggered = price <= stopPrice
		} else {
			stopPrice = pos.EntryPrice + stopDistance
			triggered = price >= stopPrice
		}
		if !triggered {
			continue
		}

		closeQty := pos.Quantity
		execRefPrice := r.executionPrice(pos.Symbol, price, ts)
		realized, fee, execPrice, err := r.account.Close(pos.Symbol, pos.Side, closeQty, execRefPrice)
		if err != nil {
			return nil, nil, fmt.Errorf("atr stop close %s %s failed: %w", pos.Symbol, pos.Side, err)
		}

		action := "close_long"
		slippage := price - execPrice
		if pos.Side == "short" {
			action = "close_short"
			slippage = execPrice - price
		}

		events = append(events, TradeEvent{
			Timestamp:     ts,
			Symbol:        pos.Symbol,
			Action:        action,
			Side:          pos.Side,
			Reasoning:     "atr volatility stop",
			Quantity:      closeQty,
			Price:         execPrice,
			StopLoss:      stopPrice,
			Fee:           fee,
			Slippage:      slippage,
			OrderValue:    execPrice * closeQty,
			RealizedPnL:   realized - fee,
			Leverage:      pos.Leverage,
			Cycle:         cycle,
			PositionAfter: 0,
			Note:          "atr_stop",
		})
		logs = append(logs, fmt.Sprintf("ATR stop triggered: %s %s @ %.4f (entry %.4f, atr %.4f, k %.2f, stop %.4f)",
			pos.Symbol, pos.Side, execPrice, pos.EntryPrice, atr, multiplier, stopPrice))
		r.clearPositionBracket(pos.Symbol, pos.Side)
	}

	return events, logs, nil
}

func (r *Runner) checkLiquidation(ts int64, priceMap map[string]float64, cycle int) ([]TradeEvent, string, error) {
	positions := append([]*position(nil), r.account.Positions()...)
	events := make([]TradeEvent, 0)
	var noteBuilder strings.Builder

	for _, pos := range positions {
		price := priceMap[pos.Symbol]
		liqPrice := pos.LiquidationPrice
		trigger := false
		execPrice := price
		if pos.Side == "long" {
			if price <= liqPrice && liqPrice > 0 {
				trigger = true
				execPrice = liqPrice
			}
		} else {
			if price >= liqPrice && liqPrice > 0 {
				trigger = true
				execPrice = liqPrice
			}
		}
		if !trigger {
			continue
		}

		realized, fee, finalPrice, err := r.account.Close(pos.Symbol, pos.Side, pos.Quantity, execPrice)
		if err != nil {
			return nil, "", err
		}

		noteBuilder.WriteString(fmt.Sprintf("%s %s @ %.4f; ", pos.Symbol, pos.Side, finalPrice))

		evt := TradeEvent{
			Timestamp:       ts,
			Symbol:          pos.Symbol,
			Action:          "liquidated",
			Side:            pos.Side,
			Quantity:        pos.Quantity,
			Price:           finalPrice,
			Fee:             fee,
			Slippage:        0,
			OrderValue:      finalPrice * pos.Quantity,
			RealizedPnL:     realized - fee,
			Leverage:        pos.Leverage,
			Cycle:           cycle,
			PositionAfter:   0,
			LiquidationFlag: true,
			Note:            fmt.Sprintf("forced liquidation at %.4f", finalPrice),
		}
		events = append(events, evt)
		r.clearPositionBracket(pos.Symbol, pos.Side)
	}

	if len(events) == 0 {
		return events, "", nil
	}

	note := strings.TrimSuffix(noteBuilder.String(), "; ")

	r.stateMu.Lock()
	r.state.Liquidated = true
	r.state.LiquidationNote = note
	r.stateMu.Unlock()

	return events, note, nil
}

func (r *Runner) shouldTriggerDecision(barIndex int) bool {
	if r.cfg.DecisionCadenceNBars <= 1 {
		return true
	}
	if barIndex < 0 {
		return true
	}
	return barIndex%r.cfg.DecisionCadenceNBars == 0
}

// forceCloseAllAtBacktestEnd settles all remaining positions at backtest completion.
// This is opt-in via BacktestConfig.ClosePositionsAtEnd to avoid changing default behavior.
func (r *Runner) forceCloseAllAtBacktestEnd() error {
	return r.closeAllPositions("forced close on backtest completion")
}

func (r *Runner) closeAllPositions(note string) error {
	positions := append([]*position(nil), r.account.Positions()...)
	if len(positions) == 0 {
		return nil
	}

	snapshot := r.snapshotState()
	ts := snapshot.BarTimestamp
	if ts <= 0 && r.feed.DecisionBarCount() > 0 {
		ts = r.feed.DecisionTimestamp(r.feed.DecisionBarCount() - 1)
	}
	cycle := snapshot.DecisionCycle

	priceMap := make(map[string]float64, len(positions))
	if ts > 0 {
		if marketData, _, err := r.feed.BuildMarketData(ts); err == nil {
			for symbol, data := range marketData {
				if data != nil && data.CurrentPrice > 0 {
					priceMap[symbol] = data.CurrentPrice
				}
			}
		}
	}

	events := make([]TradeEvent, 0, len(positions))
	for _, pos := range positions {
		if pos == nil || pos.Quantity <= 0 {
			continue
		}
		mark := priceMap[pos.Symbol]
		if mark <= 0 {
			mark = pos.EntryPrice
		}
		execRefPrice := r.executionPrice(pos.Symbol, mark, ts)
		realized, fee, execPrice, err := r.account.Close(pos.Symbol, pos.Side, pos.Quantity, execRefPrice)
		if err != nil {
			return fmt.Errorf("final settlement close %s %s failed: %w", pos.Symbol, pos.Side, err)
		}

		action := "close_long"
		slippage := mark - execPrice
		if pos.Side == "short" {
			action = "close_short"
			slippage = execPrice - mark
		}

		events = append(events, TradeEvent{
			Timestamp:     ts,
			Symbol:        pos.Symbol,
			Action:        action,
			Side:          pos.Side,
			Reasoning:     "forced settlement at backtest end",
			Quantity:      pos.Quantity,
			Price:         execPrice,
			Fee:           fee,
			Slippage:      slippage,
			OrderValue:    execPrice * pos.Quantity,
			RealizedPnL:   realized - fee,
			Leverage:      pos.Leverage,
			Cycle:         cycle,
			PositionAfter: 0,
			Note:          note,
		})
		r.clearPositionBracket(pos.Symbol, pos.Side)
	}

	if len(events) == 0 {
		return nil
	}

	for _, evt := range events {
		if err := appendTradeEvent(r.cfg.RunID, evt); err != nil {
			return err
		}
		r.recordTradeEventForContext(evt)
		r.notifyTradeSignal(evt, snapshot.Equity)
	}

	equity, unrealized, _ := r.account.TotalEquity(priceMap)
	r.stateMu.Lock()
	r.state.Cash = r.account.Cash()
	r.state.Equity = equity
	r.state.UnrealizedPnL = unrealized
	r.state.RealizedPnL = r.account.RealizedPnL()
	r.state.Positions = make(map[string]PositionSnapshot)
	r.state.LastUpdate = time.Now().UTC()
	r.stateMu.Unlock()

	finalSnapshot := r.snapshotState()
	drawdownPct := 0.0
	if finalSnapshot.MaxEquity > 0 {
		drawdownPct = ((finalSnapshot.MaxEquity - finalSnapshot.Equity) / finalSnapshot.MaxEquity) * 100
	}
	// Write one extra equity point to reflect post-settlement account state.
	if err := appendEquityPoint(r.cfg.RunID, EquityPoint{
		Timestamp:   ts,
		Equity:      finalSnapshot.Equity,
		Available:   finalSnapshot.Cash,
		PnL:         finalSnapshot.Equity - r.account.InitialBalance(),
		PnLPct:      ((finalSnapshot.Equity - r.account.InitialBalance()) / r.account.InitialBalance()) * 100,
		DrawdownPct: drawdownPct,
		Cycle:       finalSnapshot.DecisionCycle,
	}); err != nil {
		return err
	}

	return nil
}

func (r *Runner) closeSinglePosition(symbol, side, note string) error {
	targetSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	targetSide := strings.ToLower(strings.TrimSpace(side))
	if targetSymbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if targetSide != "long" && targetSide != "short" {
		return fmt.Errorf("side must be long or short")
	}

	positions := append([]*position(nil), r.account.Positions()...)
	var target *position
	for _, pos := range positions {
		if pos == nil || pos.Quantity <= 0 {
			continue
		}
		if strings.EqualFold(pos.Symbol, targetSymbol) && strings.EqualFold(pos.Side, targetSide) {
			target = pos
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no active %s position for %s", targetSide, targetSymbol)
	}

	snapshot := r.snapshotState()
	ts := snapshot.BarTimestamp
	if ts <= 0 && r.feed.DecisionBarCount() > 0 {
		ts = r.feed.DecisionTimestamp(r.feed.DecisionBarCount() - 1)
	}
	cycle := snapshot.DecisionCycle

	priceMap := make(map[string]float64, len(snapshot.Positions)+len(r.cfg.Symbols))
	if ts > 0 {
		if marketData, _, err := r.feed.BuildMarketData(ts); err == nil {
			for symbol, data := range marketData {
				if data != nil && data.CurrentPrice > 0 {
					priceMap[symbol] = data.CurrentPrice
				}
			}
		}
	}
	mark := priceMap[targetSymbol]
	if mark <= 0 {
		mark = target.EntryPrice
	}
	closeQty := target.Quantity
	execRefPrice := r.executionPrice(target.Symbol, mark, ts)
	realized, fee, execPrice, err := r.account.Close(target.Symbol, target.Side, closeQty, execRefPrice)
	if err != nil {
		return fmt.Errorf("manual close %s %s failed: %w", target.Symbol, target.Side, err)
	}

	action := "close_long"
	slippage := mark - execPrice
	if target.Side == "short" {
		action = "close_short"
		slippage = execPrice - mark
	}

	evt := TradeEvent{
		Timestamp:     ts,
		Symbol:        target.Symbol,
		Action:        action,
		Side:          target.Side,
		Reasoning:     note,
		Quantity:      closeQty,
		Price:         execPrice,
		Fee:           fee,
		Slippage:      slippage,
		OrderValue:    execPrice * closeQty,
		RealizedPnL:   realized - fee,
		Leverage:      target.Leverage,
		Cycle:         cycle,
		PositionAfter: 0,
		Note:          note,
	}
	r.clearPositionBracket(target.Symbol, target.Side)
	if err := appendTradeEvent(r.cfg.RunID, evt); err != nil {
		return err
	}
	r.recordTradeEventForContext(evt)
	r.notifyTradeSignal(evt, snapshot.Equity)

	// Keep remaining position valuations stable even if market data for some symbol is missing.
	for _, pos := range r.account.Positions() {
		if pos == nil || pos.Quantity <= 0 {
			continue
		}
		if priceMap[pos.Symbol] <= 0 {
			priceMap[pos.Symbol] = pos.EntryPrice
		}
	}

	equity, unrealized, _ := r.account.TotalEquity(priceMap)
	r.stateMu.Lock()
	r.state.Cash = r.account.Cash()
	r.state.Equity = equity
	r.state.UnrealizedPnL = unrealized
	r.state.RealizedPnL = r.account.RealizedPnL()
	positionsMap := make(map[string]PositionSnapshot)
	for _, pos := range r.account.Positions() {
		key := fmt.Sprintf("%s:%s", pos.Symbol, pos.Side)
		positionsMap[key] = PositionSnapshot{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			Quantity:         pos.Quantity,
			AvgPrice:         pos.EntryPrice,
			Leverage:         pos.Leverage,
			LiquidationPrice: pos.LiquidationPrice,
			MarginUsed:       pos.Margin,
			OpenTime:         pos.OpenTime,
			AccumulatedFee:   pos.AccumulatedFee,
		}
	}
	r.state.Positions = positionsMap
	r.state.LastUpdate = time.Now().UTC()
	r.stateMu.Unlock()

	finalSnapshot := r.snapshotState()
	drawdownPct := 0.0
	if finalSnapshot.MaxEquity > 0 {
		drawdownPct = ((finalSnapshot.MaxEquity - finalSnapshot.Equity) / finalSnapshot.MaxEquity) * 100
	}
	return appendEquityPoint(r.cfg.RunID, EquityPoint{
		Timestamp:   ts,
		Equity:      finalSnapshot.Equity,
		Available:   finalSnapshot.Cash,
		PnL:         finalSnapshot.Equity - r.account.InitialBalance(),
		PnLPct:      ((finalSnapshot.Equity - r.account.InitialBalance()) / r.account.InitialBalance()) * 100,
		DrawdownPct: drawdownPct,
		Cycle:       finalSnapshot.DecisionCycle,
	})
}

func (r *Runner) handleStop(reason error) {
	r.forceCheckpoint()
	if reason != nil {
		r.setLastError(reason)
	} else {
		r.setLastError(nil)
	}
	r.statusMu.Lock()
	r.err = reason
	r.status = RunStateStopped
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
	r.releaseLock()
}

func (r *Runner) handlePause() {
	r.forceCheckpoint()
	r.setLastError(nil)
	r.statusMu.Lock()
	r.status = RunStatePaused
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
}

func (r *Runner) resumeFromPause() {
	r.setLastError(nil)
	r.statusMu.Lock()
	r.status = RunStateRunning
	r.statusMu.Unlock()
	r.persistMetadata()
}

func (r *Runner) handleCompletion() {
	r.setLastError(nil)
	r.statusMu.Lock()
	r.status = RunStateCompleted
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
	r.releaseLock()
}

func (r *Runner) handleFailure(err error) {
	r.forceCheckpoint()
	if err != nil {
		r.setLastError(err)
	}
	r.statusMu.Lock()
	r.err = err
	r.status = RunStateFailed
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
	r.releaseLock()
}

func (r *Runner) handleLiquidation() {
	r.forceCheckpoint()
	r.setLastError(errLiquidated)
	r.statusMu.Lock()
	r.err = errLiquidated
	r.status = RunStateLiquidated
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
	r.releaseLock()
}

func (r *Runner) Pause() {
	select {
	case r.pauseCh <- struct{}{}:
	default:
	}
}

func (r *Runner) Resume() {
	select {
	case r.resumeCh <- struct{}{}:
	default:
	}
}

// CloseAllNow requests manual settlement of all current positions.
// The backtest run keeps running after settlement.
func (r *Runner) CloseAllNow() error {
	if r.Status() != RunStateRunning {
		return fmt.Errorf("manual close requires running state")
	}
	resp := make(chan error, 1)
	select {
	case r.closeAllCh <- resp:
	case <-time.After(10 * time.Second):
		return fmt.Errorf("manual close request timeout")
	}
	select {
	case err := <-resp:
		return err
	case <-time.After(180 * time.Second):
		return fmt.Errorf("manual close execution timeout")
	}
}

// ClosePositionNow requests manual settlement for one active position.
func (r *Runner) ClosePositionNow(symbol, side string) error {
	if r.Status() != RunStateRunning {
		return fmt.Errorf("manual close requires running state")
	}
	req := manualClosePositionRequest{
		Symbol: symbol,
		Side:   side,
		Resp:   make(chan error, 1),
	}
	select {
	case r.closePosCh <- req:
	case <-time.After(10 * time.Second):
		return fmt.Errorf("manual close request timeout")
	}
	select {
	case err := <-req.Resp:
		return err
	case <-time.After(180 * time.Second):
		return fmt.Errorf("manual close execution timeout")
	}
}

func (r *Runner) Stop() {
	select {
	case r.stopCh <- struct{}{}:
	default:
	}
}

func (r *Runner) Wait() error {
	<-r.doneCh
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.err
}

// Status returns the current run state.
func (r *Runner) Status() RunState {
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.status
}

// StatusPayload builds the status response for the API.
func (r *Runner) StatusPayload() StatusPayload {
	snapshot := r.snapshotState()
	progress := progressPercent(snapshot, r.cfg)

	// Build position statuses with unrealized P&L
	positions := make([]PositionStatus, 0, len(snapshot.Positions))
	for _, pos := range snapshot.Positions {
		if pos.Quantity <= 0 {
			continue
		}
		// Get mark price from feed if available
		markPrice := pos.AvgPrice // fallback to entry price
		if r.feed != nil && snapshot.BarTimestamp > 0 {
			if md, _, err := r.feed.BuildMarketData(snapshot.BarTimestamp); err == nil {
				if data, ok := md[pos.Symbol]; ok {
					markPrice = data.CurrentPrice
				}
			}
		}

		// Calculate unrealized P&L
		var unrealizedPnL float64
		if pos.Side == "long" {
			unrealizedPnL = (markPrice - pos.AvgPrice) * pos.Quantity
		} else {
			unrealizedPnL = (pos.AvgPrice - markPrice) * pos.Quantity
		}

		// Calculate P&L percentage based on margin
		pnlPct := 0.0
		if pos.MarginUsed > 0 {
			pnlPct = (unrealizedPnL / pos.MarginUsed) * 100
		}

		positions = append(positions, PositionStatus{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			Quantity:         pos.Quantity,
			EntryPrice:       pos.AvgPrice,
			MarkPrice:        markPrice,
			Leverage:         pos.Leverage,
			UnrealizedPnL:    unrealizedPnL,
			UnrealizedPnLPct: pnlPct,
			MarginUsed:       pos.MarginUsed,
		})
	}

	payload := StatusPayload{
		RunID:          r.cfg.RunID,
		State:          r.Status(),
		ProgressPct:    progress,
		ProcessedBars:  snapshot.BarIndex,
		CurrentTime:    snapshot.BarTimestamp,
		DecisionCycle:  snapshot.DecisionCycle,
		Equity:         snapshot.Equity,
		UnrealizedPnL:  snapshot.UnrealizedPnL,
		RealizedPnL:    snapshot.RealizedPnL,
		Positions:      positions,
		Note:           snapshot.LiquidationNote,
		LastError:      r.lastErrorString(),
		LastUpdatedIso: snapshot.LastUpdate.UTC().Format(time.RFC3339),
	}
	return payload
}

func (r *Runner) snapshotState() BacktestState {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()

	copyState := *r.state
	copyState.Positions = make(map[string]PositionSnapshot, len(r.state.Positions))
	for k, v := range r.state.Positions {
		copyState.Positions[k] = v
	}
	return copyState
}

func (r *Runner) persistMetadata() {
	state := r.snapshotState()
	meta := r.buildMetadata(state, r.Status())
	meta.CreatedAt = r.createdAt
	if err := SaveRunMetadata(meta); err != nil {
		logger.Infof("failed to save run metadata for %s: %v", r.cfg.RunID, err)
	} else {
		if err := updateRunIndex(meta, &r.cfg); err != nil {
			logger.Infof("failed to update index for %s: %v", r.cfg.RunID, err)
		}
	}
}

func (r *Runner) logDecision(record *store.DecisionRecord) error {
	if record == nil {
		return nil
	}
	persistDecisionRecord(r.cfg.RunID, record)
	return nil
}

func buildBacktestDiscordNotifier(cfg BacktestConfig) *discordNotifier {
	_ = cfg
	// Backtest should not emit Discord notifications. Notification delivery is
	// reserved for dedicated notify mode in live trader flow.
	return nil
}

func (r *Runner) notifyTradeSignal(evt TradeEvent, equity float64) {
	if r == nil || r.discordNotifier == nil {
		return
	}
	if err := r.discordNotifier.notifyTrade(r.cfg.RunID, r.cfg.UserEmail, evt, equity); err != nil {
		logger.Warnf("backtest discord notify failed run=%s email=%s symbol=%s action=%s: %v",
			r.cfg.RunID, r.cfg.UserEmail, evt.Symbol, evt.Action, err)
	}
}

func (r *Runner) persistMetrics(force bool) {
	if r.cfg.RunID == "" {
		return
	}

	if !force && !r.lastMetricsWrite.IsZero() {
		if time.Since(r.lastMetricsWrite) < metricsWriteInterval {
			return
		}
	}

	state := r.snapshotState()
	metrics, err := CalculateMetrics(r.cfg.RunID, &r.cfg, &state)
	if err != nil {
		logger.Infof("failed to compute metrics for %s: %v", r.cfg.RunID, err)
		return
	}
	if metrics == nil {
		return
	}
	if err := PersistMetrics(r.cfg.RunID, metrics); err != nil {
		logger.Infof("failed to persist metrics for %s: %v", r.cfg.RunID, err)
		return
	}
	r.lastMetricsWrite = time.Now()
}

func (r *Runner) buildMetadata(state BacktestState, runState RunState) *RunMetadata {
	if state.Liquidated && runState != RunStateLiquidated {
		runState = RunStateLiquidated
	}

	progress := progressPercent(state, r.cfg)

	summary := RunSummary{
		SymbolCount:     len(r.cfg.Symbols),
		DecisionTF:      r.cfg.DecisionTimeframe,
		ProcessedBars:   state.BarIndex,
		ProgressPct:     progress,
		EquityLast:      state.Equity,
		MaxDrawdownPct:  state.MaxDrawdownPct,
		Liquidated:      state.Liquidated,
		LiquidationNote: state.LiquidationNote,
	}

	meta := &RunMetadata{
		RunID:     r.cfg.RunID,
		UserID:    r.cfg.UserID,
		State:     runState,
		LastError: r.lastErrorString(),
		Summary:   summary,
	}

	return meta
}

func progressPercent(state BacktestState, cfg BacktestConfig) float64 {
	duration := cfg.Duration()
	if duration <= 0 {
		return 0
	}
	if state.BarTimestamp == 0 {
		return 0
	}

	start := time.Unix(cfg.StartTS, 0)
	end := time.Unix(cfg.EndTS, 0)
	current := time.UnixMilli(state.BarTimestamp)

	if !current.After(start) {
		return 0
	}
	if current.After(end) {
		return 100
	}

	elapsed := current.Sub(start)
	pct := float64(elapsed) / float64(duration) * 100
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}
	return pct
}

func (r *Runner) buildCheckpointFromState(state BacktestState) *Checkpoint {
	return &Checkpoint{
		BarIndex:        state.BarIndex,
		BarTimestamp:    state.BarTimestamp,
		Cash:            state.Cash,
		Equity:          state.Equity,
		UnrealizedPnL:   state.UnrealizedPnL,
		RealizedPnL:     state.RealizedPnL,
		Positions:       r.snapshotForCheckpoint(state),
		DecisionCycle:   state.DecisionCycle,
		Liquidated:      state.Liquidated,
		LiquidationNote: state.LiquidationNote,
		MaxEquity:       state.MaxEquity,
		MinEquity:       state.MinEquity,
		MaxDrawdownPct:  state.MaxDrawdownPct,
		AICacheRef:      r.cachePath,
	}
}

func (r *Runner) saveCheckpoint(state BacktestState) error {
	ckpt := r.buildCheckpointFromState(state)
	if ckpt == nil {
		return nil
	}
	if err := SaveCheckpoint(r.cfg.RunID, ckpt); err != nil {
		return err
	}
	r.lastCheckpoint = time.Now()
	return nil
}

func (r *Runner) forceCheckpoint() {
	state := r.snapshotState()
	if err := r.saveCheckpoint(state); err != nil {
		logger.Infof("failed to save checkpoint for %s: %v", r.cfg.RunID, err)
	}
}

func (r *Runner) RestoreFromCheckpoint() error {
	ckpt, err := LoadCheckpoint(r.cfg.RunID)
	if err != nil {
		return err
	}
	return r.applyCheckpoint(ckpt)
}

func (r *Runner) applyCheckpoint(ckpt *Checkpoint) error {
	if ckpt == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	r.account.RestoreFromSnapshots(ckpt.Cash, ckpt.RealizedPnL, ckpt.Positions)
	r.positionBrackets = make(map[string]positionBracket)
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.state.BarIndex = ckpt.BarIndex
	r.state.BarTimestamp = ckpt.BarTimestamp
	r.state.Cash = ckpt.Cash
	r.state.Equity = ckpt.Equity
	r.state.UnrealizedPnL = ckpt.UnrealizedPnL
	r.state.RealizedPnL = ckpt.RealizedPnL
	r.state.DecisionCycle = ckpt.DecisionCycle
	r.state.Liquidated = ckpt.Liquidated
	r.state.LiquidationNote = ckpt.LiquidationNote
	r.state.MaxEquity = ckpt.MaxEquity
	r.state.MinEquity = ckpt.MinEquity
	r.state.MaxDrawdownPct = ckpt.MaxDrawdownPct
	r.state.Positions = snapshotsToMap(ckpt.Positions)
	r.state.LastUpdate = time.Now().UTC()
	r.lastCheckpoint = time.Now()
	return nil
}

func snapshotsToMap(snaps []PositionSnapshot) map[string]PositionSnapshot {
	positions := make(map[string]PositionSnapshot, len(snaps))
	for _, snap := range snaps {
		key := fmt.Sprintf("%s:%s", snap.Symbol, snap.Side)
		positions[key] = snap
	}
	return positions
}

func sortDecisionsByPriority(decisions []kernel.Decision) []kernel.Decision {
	if len(decisions) <= 1 {
		return decisions
	}

	priority := func(action string) int {
		switch action {
		case "close_long", "close_short":
			return 1
		case "open_long", "open_short":
			return 2
		case "hold", "wait":
			return 3
		default:
			return 99
		}
	}

	result := make([]kernel.Decision, len(decisions))
	copy(result, decisions)

	sort.Slice(result, func(i, j int) bool {
		pi := priority(result[i].Action)
		pj := priority(result[j].Action)
		if pi != pj {
			return pi < pj
		}
		return i < j
	})

	return result
}

func barVWAP(k market.Kline) float64 {
	values := []float64{k.Open, k.High, k.Low, k.Close}
	sum := 0.0
	count := 0.0
	for _, v := range values {
		if v > 0 {
			sum += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / count
}
