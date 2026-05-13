package trader

import (
	"encoding/json"
	"fmt"
	"math"
	"nofx/experience"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	"nofx/store"
	"nofx/trader/aster"
	"nofx/trader/binance"
	"nofx/trader/bitget"
	"nofx/trader/bybit"
	"nofx/trader/gate"
	"nofx/trader/hyperliquid"
	"nofx/trader/indodax"
	"nofx/trader/kucoin"
	"nofx/trader/lighter"
	"nofx/trader/okx"
	"strings"
	"sync"
	"time"
)

// AutoTraderConfig auto trading configuration (simplified version - AI makes all decisions)
type AutoTraderConfig struct {
	// Trader identification
	ID      string // Trader unique identifier (for log directory, etc.)
	Name    string // Trader display name
	AIModel string // AI model: "qwen" or "deepseek"

	// Trading platform selection
	Exchange   string // Exchange type: "binance", "bybit", "okx", "bitget", "gate", "hyperliquid", "aster" or "lighter"
	ExchangeID string // Exchange account UUID (for multi-account support)

	// Binance API configuration
	BinanceAPIKey    string
	BinanceSecretKey string

	// Bybit API configuration
	BybitAPIKey    string
	BybitSecretKey string

	// OKX API configuration
	OKXAPIKey     string
	OKXSecretKey  string
	OKXPassphrase string

	// Bitget API configuration
	BitgetAPIKey     string
	BitgetSecretKey  string
	BitgetPassphrase string

	// Gate API configuration
	GateAPIKey    string
	GateSecretKey string

	// KuCoin API configuration
	KuCoinAPIKey     string
	KuCoinSecretKey  string
	KuCoinPassphrase string

	// Indodax API configuration
	IndodaxAPIKey    string
	IndodaxSecretKey string

	// Hyperliquid configuration
	HyperliquidPrivateKey  string
	HyperliquidWalletAddr  string
	HyperliquidTestnet     bool
	HyperliquidUnifiedAcct bool // Unified Account mode: Spot USDC as Perp collateral

	// Aster configuration
	AsterUser       string // Aster main wallet address
	AsterSigner     string // Aster API wallet address
	AsterPrivateKey string // Aster API wallet private key

	// LIGHTER configuration
	LighterWalletAddr       string // LIGHTER wallet address (L1 wallet)
	LighterPrivateKey       string // LIGHTER L1 private key (for account identification)
	LighterAPIKeyPrivateKey string // LIGHTER API Key private key (40 bytes, for transaction signing)
	LighterAPIKeyIndex      int    // LIGHTER API Key index (0-255)
	LighterTestnet          bool   // Whether to use testnet

	// AI configuration
	UseQwen     bool
	DeepSeekKey string
	QwenKey     string

	// Custom AI API configuration
	CustomAPIURL    string
	CustomAPIKey    string
	CustomModelName string

	// Scan configuration
	ScanInterval  time.Duration // Scan interval (recommended 3 minutes)
	ExecutionMode string        // Execution mode: "live" or "alert_only"

	// Account configuration
	InitialBalance float64 // Initial balance (for P&L calculation, must be set manually)

	// Risk control (only as hints, AI can make autonomous decisions)
	MaxDailyLoss    float64       // Maximum daily loss percentage (hint)
	MaxDrawdown     float64       // Maximum drawdown percentage (hint)
	StopTradingTime time.Duration // Pause duration after risk control triggers

	// Position mode
	IsCrossMargin bool // true=cross margin mode, false=isolated margin mode

	// Competition visibility
	ShowInCompetition bool // Whether to show in competition page

	// Strategy configuration (use complete strategy config)
	StrategyConfig *store.StrategyConfig // Strategy configuration (includes coin sources, indicators, risk control, prompts, etc.)
	StrategyID     string
}

// AutoTrader automatic trader
type AutoTrader struct {
	id                    string // Trader unique identifier
	name                  string // Trader display name
	aiModel               string // AI model name
	exchange              string // Trading platform type (binance/bybit/etc)
	exchangeID            string // Exchange account UUID
	showInCompetition     bool   // Whether to show in competition page
	config                AutoTraderConfig
	trader                Trader // Use Trader interface (supports multiple platforms)
	mcpClient             mcp.AIClient
	store                 *store.Store           // Data storage (decision records, etc.)
	strategyEngine        *kernel.StrategyEngine // Strategy engine (uses strategy configuration)
	cycleNumber           int                    // Current cycle number
	initialBalance        float64
	dailyPnL              float64
	customPrompt          string // Custom trading strategy prompt
	overrideBasePrompt    bool   // Whether to override base prompt
	lastResetTime         time.Time
	stopUntil             time.Time
	isRunning             bool
	isRunningMutex        sync.RWMutex       // Mutex to protect isRunning flag
	startTime             time.Time          // System start time
	callCount             int                // AI call count
	positionFirstSeenTime map[string]int64   // Position first seen time (symbol_side -> timestamp in milliseconds)
	stopMonitorCh         chan struct{}      // Used to stop monitoring goroutine
	monitorWg             sync.WaitGroup     // Used to wait for monitoring goroutine to finish
	peakPnLCache          map[string]float64 // Peak profit cache (symbol -> peak P&L percentage)
	peakPnLCacheMutex     sync.RWMutex       // Cache read-write lock
	lastBalanceSyncTime   time.Time          // Last balance sync time
	userID                string             // User ID
	userEmail             string             // User email (for signal routing)
	strategyID            string             // Bound strategy ID (for signal routing)
	lastStrategyUpdatedAt time.Time          // Last loaded strategy update time (for hot-reload)
	gridState             *GridState         // Grid trading state (only used when StrategyType == "grid_trading")
}

// NewAutoTrader creates an automatic trader
// st parameter is used to store decision records to database
func NewAutoTrader(config AutoTraderConfig, st *store.Store, userID string) (*AutoTrader, error) {
	// Set default values
	if config.ID == "" {
		config.ID = "default_trader"
	}
	if config.Name == "" {
		config.Name = "Default Trader"
	}
	if config.AIModel == "" {
		if config.UseQwen {
			config.AIModel = "qwen"
		} else {
			config.AIModel = "deepseek"
		}
	}
	config.ExecutionMode = store.NormalizeTraderExecutionMode(config.ExecutionMode)

	// Initialize AI client based on provider
	var mcpClient mcp.AIClient
	aiModel := config.AIModel
	if config.UseQwen && aiModel == "" {
		aiModel = "qwen"
	}

	switch aiModel {
	case "claude":
		mcpClient = mcp.NewClaudeClient()
		mcpClient.SetAPIKey(config.CustomAPIKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using Claude AI", config.Name)

	case "kimi":
		mcpClient = mcp.NewKimiClient()
		mcpClient.SetAPIKey(config.CustomAPIKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using Kimi (Moonshot) AI", config.Name)

	case "gemini":
		mcpClient = mcp.NewGeminiClient()
		mcpClient.SetAPIKey(config.CustomAPIKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using Google Gemini AI", config.Name)

	case "grok":
		mcpClient = mcp.NewGrokClient()
		mcpClient.SetAPIKey(config.CustomAPIKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using xAI Grok AI", config.Name)

	case "openai":
		mcpClient = mcp.NewOpenAIClient()
		mcpClient.SetAPIKey(config.CustomAPIKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using OpenAI", config.Name)

	case "minimax":
		mcpClient = mcp.NewMiniMaxClient()
		mcpClient.SetAPIKey(config.CustomAPIKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using MiniMax AI", config.Name)

	case "blockrun-base":
		mcpClient = mcp.NewBlockRunBaseClient()
		mcpClient.SetAPIKey(config.CustomAPIKey, "", config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using BlockRun (Base Wallet) AI", config.Name)

	case "blockrun-sol":
		mcpClient = mcp.NewBlockRunSolClient()
		mcpClient.SetAPIKey(config.CustomAPIKey, "", config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using BlockRun (Solana Wallet) AI", config.Name)

	case "qwen":
		mcpClient = mcp.NewQwenClient()
		apiKey := config.QwenKey
		if apiKey == "" {
			apiKey = config.CustomAPIKey
		}
		mcpClient.SetAPIKey(apiKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using Alibaba Cloud Qwen AI", config.Name)

	case "custom":
		mcpClient = mcp.New()
		mcpClient.SetAPIKey(config.CustomAPIKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using custom AI API: %s (model: %s)", config.Name, config.CustomAPIURL, config.CustomModelName)

	default: // deepseek or empty
		mcpClient = mcp.NewDeepSeekClient()
		apiKey := config.DeepSeekKey
		if apiKey == "" {
			apiKey = config.CustomAPIKey
		}
		mcpClient.SetAPIKey(apiKey, config.CustomAPIURL, config.CustomModelName)
		logger.Infof("濡絽鍠氬Ο?[%s] Using DeepSeek AI", config.Name)
	}

	if config.CustomAPIURL != "" || config.CustomModelName != "" {
		logger.Infof("濡絽鍟弳?[%s] Custom config - URL: %s, Model: %s", config.Name, config.CustomAPIURL, config.CustomModelName)
	}

	// Set default trading platform
	if config.Exchange == "" {
		config.Exchange = "binance"
	}

	// Create corresponding trader based on configuration
	var trader Trader
	var err error

	// Record position mode (general)
	marginModeStr := "Cross Margin"
	if !config.IsCrossMargin {
		marginModeStr = "Isolated Margin"
	}
	logger.Infof("濡絽鍟幆?[%s] Position mode: %s", config.Name, marginModeStr)

	switch config.Exchange {
	case "binance":
		logger.Infof("濡絽鍟紞?[%s] Using Binance Futures trading", config.Name)
		trader = binance.NewFuturesTrader(config.BinanceAPIKey, config.BinanceSecretKey, userID)
	case "bybit":
		logger.Infof("濡絽鍟紞?[%s] Using Bybit Futures trading", config.Name)
		trader = bybit.NewBybitTrader(config.BybitAPIKey, config.BybitSecretKey)
	case "okx":
		logger.Infof("濡絽鍟紞?[%s] Using OKX Futures trading", config.Name)
		trader = okx.NewOKXTrader(config.OKXAPIKey, config.OKXSecretKey, config.OKXPassphrase)
	case "bitget":
		logger.Infof("濡絽鍟紞?[%s] Using Bitget Futures trading", config.Name)
		trader = bitget.NewBitgetTrader(config.BitgetAPIKey, config.BitgetSecretKey, config.BitgetPassphrase)
	case "gate":
		logger.Infof("濡絽鍟紞?[%s] Using Gate.io Futures trading", config.Name)
		trader = gate.NewGateTrader(config.GateAPIKey, config.GateSecretKey)
	case "kucoin":
		logger.Infof("濡絽鍟紞?[%s] Using KuCoin Futures trading", config.Name)
		trader = kucoin.NewKuCoinTrader(config.KuCoinAPIKey, config.KuCoinSecretKey, config.KuCoinPassphrase)
	case "hyperliquid":
		logger.Infof("濡絽鍟紞?[%s] Using Hyperliquid trading", config.Name)
		trader, err = hyperliquid.NewHyperliquidTrader(config.HyperliquidPrivateKey, config.HyperliquidWalletAddr, config.HyperliquidTestnet, config.HyperliquidUnifiedAcct)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Hyperliquid trader: %w", err)
		}
	case "aster":
		logger.Infof("濡絽鍟紞?[%s] Using Aster trading", config.Name)
		trader, err = aster.NewAsterTrader(config.AsterUser, config.AsterSigner, config.AsterPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Aster trader: %w", err)
		}
	case "lighter":
		logger.Infof("濡絽鍟紞?[%s] Using LIGHTER trading", config.Name)

		if config.LighterWalletAddr == "" || config.LighterAPIKeyPrivateKey == "" {
			return nil, fmt.Errorf("Lighter requires wallet address and API Key private key")
		}

		// Lighter only supports mainnet (testnet disabled)
		trader, err = lighter.NewLighterTraderV2(
			config.LighterWalletAddr,
			config.LighterAPIKeyPrivateKey,
			config.LighterAPIKeyIndex,
			false, // Always use mainnet for Lighter
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize LIGHTER trader: %w", err)
		}
		logger.Infof("闂?LIGHTER trader initialized successfully")
	case "indodax":
		logger.Infof("濡絽鍟紞?[%s] Using Indodax Spot trading", config.Name)
		trader = indodax.NewIndodaxTrader(config.IndodaxAPIKey, config.IndodaxSecretKey)
	default:
		return nil, fmt.Errorf("unsupported trading platform: %s", config.Exchange)
	}

	// Validate initial balance configuration, auto-fetch from exchange if 0
	if config.InitialBalance <= 0 {
		logger.Infof("濡絽鍟幆?[%s] Initial balance not set, attempting to fetch current balance from exchange...", config.Name)
		account, err := trader.GetBalance()
		if err != nil {
			return nil, fmt.Errorf("initial balance not set and unable to fetch balance from exchange: %w", err)
		}
		// Try multiple balance field names (different exchanges return different formats)
		balanceKeys := []string{"total_equity", "totalWalletBalance", "wallet_balance", "totalEq", "balance"}
		var foundBalance float64
		for _, key := range balanceKeys {
			if balance, ok := account[key].(float64); ok && balance > 0 {
				foundBalance = balance
				break
			}
		}
		if foundBalance > 0 {
			config.InitialBalance = foundBalance
			logger.Infof("闂?[%s] Auto-fetched initial balance: %.2f USDT", config.Name, foundBalance)
			// Save to database so it persists across restarts
			if st != nil {
				if err := st.Trader().UpdateInitialBalance(userID, config.ID, foundBalance); err != nil {
					logger.Infof("闂佸疇娉曟刊瀵哥箔? [%s] Failed to save initial balance to database: %v", config.Name, err)
				} else {
					logger.Infof("闂?[%s] Initial balance saved to database", config.Name)
				}
			}
		} else {
			return nil, fmt.Errorf("initial balance must be greater than 0, please set InitialBalance in config or ensure exchange account has balance")
		}
	}

	// Get last cycle number (for recovery)
	var cycleNumber int
	if st != nil {
		cycleNumber, _ = st.Decision().GetLastCycleNumber(config.ID)
		logger.Infof("濡絽鍟幆?[%s] Decision records will be stored to database", config.Name)
	}

	// Create strategy engine (must have strategy config)
	if config.StrategyConfig == nil {
		return nil, fmt.Errorf("[%s] strategy not configured", config.Name)
	}
	strategyEngine := kernel.NewStrategyEngine(config.StrategyConfig)
	logger.Infof("闂?[%s] Using strategy engine (strategy configuration loaded)", config.Name)

	at := &AutoTrader{
		id:                    config.ID,
		name:                  config.Name,
		aiModel:               config.AIModel,
		exchange:              config.Exchange,
		exchangeID:            config.ExchangeID,
		showInCompetition:     config.ShowInCompetition,
		config:                config,
		trader:                trader,
		mcpClient:             mcpClient,
		store:                 st,
		strategyEngine:        strategyEngine,
		cycleNumber:           cycleNumber,
		initialBalance:        config.InitialBalance,
		lastResetTime:         time.Now(),
		startTime:             time.Now(),
		callCount:             0,
		isRunning:             false,
		positionFirstSeenTime: make(map[string]int64),
		stopMonitorCh:         make(chan struct{}),
		monitorWg:             sync.WaitGroup{},
		peakPnLCache:          make(map[string]float64),
		peakPnLCacheMutex:     sync.RWMutex{},
		lastBalanceSyncTime:   time.Now(),
		userID:                userID,
		userEmail:             "",
		strategyID:            strings.TrimSpace(config.StrategyID),
	}

	if st != nil && strings.TrimSpace(userID) != "" {
		if user, err := st.User().GetByID(userID); err == nil && user != nil {
			at.userEmail = strings.ToLower(strings.TrimSpace(user.Email))
		}
	}

	return at, nil
}

// Run runs the automatic trading main loop
func (at *AutoTrader) Run() error {
	at.isRunningMutex.Lock()
	at.isRunning = true
	at.isRunningMutex.Unlock()

	at.stopMonitorCh = make(chan struct{})
	at.startTime = time.Now()

	logger.Info("濡絽鍟悾?AI-driven automatic trading system started")
	logger.Infof("濡絽鍟亸?Initial balance: %.2f USDT", at.initialBalance)
	logger.Infof("闂佽櫕瀵ч悷杈╃箔? Scan interval: %v", at.config.ScanInterval)
	if at.isAlertOnlyMode() {
		logger.Infof("濡絽鍟幉?Execution mode: alert_only (signal notification only, no order placement)")
	}
	logger.Info("濡絽鍠氬Ο?AI will make full decisions on leverage, position size, stop loss/take profit, etc.")
	at.monitorWg.Add(1)
	defer at.monitorWg.Done()

	// Start drawdown monitoring
	at.startDrawdownMonitor()

	// Start Lighter order sync if using Lighter exchange
	if at.exchange == "lighter" {
		if lighterTrader, ok := at.trader.(*lighter.LighterTraderV2); ok && at.store != nil {
			lighterTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] Lighter order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Hyperliquid order sync if using Hyperliquid exchange
	if at.exchange == "hyperliquid" {
		if hyperliquidTrader, ok := at.trader.(*hyperliquid.HyperliquidTrader); ok && at.store != nil {
			hyperliquidTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] Hyperliquid order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Bybit order sync if using Bybit exchange
	if at.exchange == "bybit" {
		if bybitTrader, ok := at.trader.(*bybit.BybitTrader); ok && at.store != nil {
			bybitTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] Bybit order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start OKX order sync if using OKX exchange
	if at.exchange == "okx" {
		if okxTrader, ok := at.trader.(*okx.OKXTrader); ok && at.store != nil {
			okxTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] OKX order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Bitget order sync if using Bitget exchange
	if at.exchange == "bitget" {
		if bitgetTrader, ok := at.trader.(*bitget.BitgetTrader); ok && at.store != nil {
			bitgetTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] Bitget order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Aster order sync if using Aster exchange
	if at.exchange == "aster" {
		if asterTrader, ok := at.trader.(*aster.AsterTrader); ok && at.store != nil {
			asterTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] Aster order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Binance order sync if using Binance exchange
	if at.exchange == "binance" {
		if binanceTrader, ok := at.trader.(*binance.FuturesTrader); ok && at.store != nil {
			binanceTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] Binance order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Gate order sync if using Gate exchange
	if at.exchange == "gate" {
		if gateTrader, ok := at.trader.(*gate.GateTrader); ok && at.store != nil {
			gateTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] Gate order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start KuCoin order sync if using KuCoin exchange
	if at.exchange == "kucoin" {
		if kucoinTrader, ok := at.trader.(*kucoin.KuCoinTrader); ok && at.store != nil {
			kucoinTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("濡絽鍟弫?[%s] KuCoin order+position sync enabled (every 30s)", at.name)
		}
	}

	ticker := time.NewTicker(at.config.ScanInterval)
	defer ticker.Stop()

	// Check if this is a grid trading strategy
	isGridStrategy := at.IsGridStrategy()
	if isGridStrategy {
		logger.Infof("濡絽鍟弳?[%s] Grid trading strategy detected, initializing grid...", at.name)
		if err := at.InitializeGrid(); err != nil {
			logger.Errorf("闂?[%s] Failed to initialize grid: %v", at.name, err)
			return fmt.Errorf("grid initialization failed: %w", err)
		}
	}

	// Execute immediately on first run
	if isGridStrategy {
		if err := at.RunGridCycle(); err != nil {
			logger.Infof("闂?Grid execution failed: %v", err)
		}
	} else {
		if err := at.runCycle(); err != nil {
			logger.Infof("闂?Execution failed: %v", err)
		}
	}

	for {
		at.isRunningMutex.RLock()
		running := at.isRunning
		at.isRunningMutex.RUnlock()

		if !running {
			break
		}

		select {
		case <-ticker.C:
			if isGridStrategy {
				if err := at.RunGridCycle(); err != nil {
					logger.Infof("闂?Grid execution failed: %v", err)
				}
			} else {
				if err := at.runCycle(); err != nil {
					logger.Infof("闂?Execution failed: %v", err)
				}
			}
		case <-at.stopMonitorCh:
			logger.Infof("[%s] 闂?Stop signal received, exiting automatic trading main loop", at.name)
			return nil
		}
	}

	return nil
}

// Stop stops the automatic trading
func (at *AutoTrader) Stop() {
	at.isRunningMutex.Lock()
	if !at.isRunning {
		at.isRunningMutex.Unlock()
		return
	}
	at.isRunning = false
	at.isRunningMutex.Unlock()

	close(at.stopMonitorCh) // Notify monitoring goroutine to stop
	at.monitorWg.Wait()     // Wait for monitoring goroutine to finish
	logger.Info("闂?Automatic trading system stopped")
}

func (at *AutoTrader) isAlertOnlyMode() bool {
	return store.NormalizeTraderExecutionMode(at.config.ExecutionMode) == store.TraderExecutionModeAlertOnly
}

func (at *AutoTrader) notificationsEnabled() bool {
	mode := store.NormalizeTraderExecutionMode(at.config.ExecutionMode)
	return mode == store.TraderExecutionModeAlertOnly || mode == store.TraderExecutionModeLiveAndAlert
}

func (at *AutoTrader) estimateCloseQuantity(symbol, side string) float64 {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return 0
	}
	targetSymbol := market.Normalize(symbol)
	targetSide := strings.ToLower(strings.TrimSpace(side))
	for _, pos := range positions {
		rawSymbol, _ := pos["symbol"].(string)
		rawSide, _ := pos["side"].(string)
		if market.Normalize(rawSymbol) != targetSymbol || strings.ToLower(strings.TrimSpace(rawSide)) != targetSide {
			continue
		}
		amt, ok := pos["positionAmt"].(float64)
		if !ok {
			return 0
		}
		if amt < 0 {
			amt = -amt
		}
		return amt
	}
	return 0
}

func (at *AutoTrader) handleAlertOnlyDecision(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	action := strings.ToLower(strings.TrimSpace(decision.Action))

	switch action {
	case "open_long", "open_short":
		if marketData, err := market.GetWithExchange(decision.Symbol, at.exchange); err == nil {
			actionRecord.Price = marketData.CurrentPrice
		}

		// Apply risk validation before sending any open signal notifications.
		if err := at.validateAlertOnlyOpenSignal(decision, actionRecord); err != nil {
			return err
		}
		if isActionSkipped(actionRecord) {
			actionRecord.Success = true
			return nil
		}

		if actionRecord.Price > 0 && decision.PositionSizeUSD > 0 {
			actionRecord.Quantity = decision.PositionSizeUSD / actionRecord.Price
		}

		actionRecord.Success = true
		at.notifyStrategySignal(decision, actionRecord)
		return nil

	case "close_long", "close_short":
		if marketData, err := market.GetWithExchange(decision.Symbol, at.exchange); err == nil {
			actionRecord.Price = marketData.CurrentPrice
		}
		if action == "close_long" {
			actionRecord.Quantity = at.estimateCloseQuantity(decision.Symbol, "long")
		}
		if action == "close_short" {
			actionRecord.Quantity = at.estimateCloseQuantity(decision.Symbol, "short")
		}

		// Do not send close signal if there is no corresponding position.
		if actionRecord.Quantity <= 0 {
			markActionSkipped(actionRecord, fmt.Sprintf("skip %s %s: no matching open position", action, decision.Symbol))
			actionRecord.Success = true
			return nil
		}

		actionRecord.Success = true
		at.notifyStrategySignal(decision, actionRecord)
		return nil
	case "hold", "wait":
		actionRecord.Success = true
		return nil
	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}
}

func (at *AutoTrader) validateAlertOnlyOpenSignal(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	riskControl := at.riskControlConfig()
	marketPrice := actionRecord.Price
	if marketPrice <= 0 {
		return fmt.Errorf("failed to get market price for %s", decision.Symbol)
	}

	// 1) Entry deviation guard.
	if shouldSkipByPriceDeviation(decision.EntryPrice, marketPrice, riskControl.EffectivePriceDeviationLimitPct()) {
		reason := fmt.Sprintf("skip %s %s: entry deviation too high (ai=%.6f market=%.6f limit=%.2f%%)",
			decision.Action, decision.Symbol, decision.EntryPrice, marketPrice, riskControl.EffectivePriceDeviationLimitPct())
		markActionSkipped(actionRecord, reason)
		return nil
	}

	// 2) RR guard with current market price as estimated entry in alert mode.
	minRR := effectiveMinRiskRewardRatio(riskControl.MinRiskRewardRatio)
	rr, riskPct, rewardPct, ok := store.CalculateRiskReward(decision.Action, marketPrice, decision.StopLoss, decision.TakeProfit)
	if !ok {
		markActionSkipped(actionRecord, fmt.Sprintf("skip %s %s: invalid RR inputs at market entry %.6f", decision.Action, decision.Symbol, marketPrice))
		return nil
	}

	threshold := minRR
	if riskControl.EffectivePostFillRRRecheckEnabled() {
		threshold = minRR - riskControl.EffectivePostFillRRTolerance()
		if threshold < 0 {
			threshold = 0
		}
	}

	if rr >= threshold {
		return nil
	}

	onFail := riskControl.EffectivePostFillRROnFail()
	msg := fmt.Sprintf("RR check failed [%s %s]: rr=%.2f threshold=%.2f risk=%.2f%% reward=%.2f%% action=%s",
		decision.Action, decision.Symbol, rr, threshold, riskPct, rewardPct, onFail)

	switch onFail {
	case store.PostFillRROnFailAdjustTP:
		newTP, adjusted := store.AdjustTakeProfitForMinRR(decision.Action, marketPrice, decision.StopLoss, minRR)
		if !adjusted || newTP <= 0 {
			markActionSkipped(actionRecord, msg+"; adjust_tp failed")
			return nil
		}
		decision.TakeProfit = newTP
		actionRecord.TakeProfit = newTP
		appendActionWarning(actionRecord, msg+fmt.Sprintf("; adjusted tp=%.6f", newTP))
		return nil
	case store.PostFillRROnFailCloseImmediately:
		// In alert-only mode there is no newly opened position to close; skip signal.
		markActionSkipped(actionRecord, msg+"; close_immediately mapped to skip in alert_only mode")
		return nil
	case store.PostFillRROnFailAlertOnly:
		appendActionWarning(actionRecord, msg)
		return nil
	default:
		markActionSkipped(actionRecord, msg)
		return nil
	}
}

// runCycle runs one trading cycle (using AI full decision-making)
func (at *AutoTrader) runCycle() error {
	at.callCount++

	logger.Info("\n" + strings.Repeat("=", 70) + "\n")
	logger.Infof("闂?%s - AI decision cycle #%d", time.Now().Format("2006-01-02 15:04:05"), at.callCount)
	logger.Info(strings.Repeat("=", 70))

	// 0. Check if trader is stopped (early exit to prevent trades after Stop() is called)
	at.isRunningMutex.RLock()
	running := at.isRunning
	at.isRunningMutex.RUnlock()
	if !running {
		logger.Infof("闂?Trader is stopped, aborting cycle #%d", at.callCount)
		return nil
	}

	// Hot-reload strategy config from DB when Strategy Studio changes are detected.
	at.refreshStrategyConfigIfNeeded()

	// Create decision record
	record := &store.DecisionRecord{
		ExecutionLog: []string{},
		Success:      true,
	}

	// 1. Check if trading needs to be stopped
	if time.Now().Before(at.stopUntil) {
		remaining := at.stopUntil.Sub(time.Now())
		logger.Infof("闂?Risk control: Trading paused, remaining %.0f minutes", remaining.Minutes())
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Risk control paused, remaining %.0f minutes", remaining.Minutes())
		at.saveDecision(record)
		return nil
	}

	// 2. Reset daily P&L (reset every day)
	if time.Since(at.lastResetTime) > 24*time.Hour {
		at.dailyPnL = 0
		at.lastResetTime = time.Now()
		logger.Info("濡絽鍟幆?Daily P&L reset")
	}

	// 4. Collect trading context
	ctx, err := at.buildTradingContext()
	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Failed to build trading context: %v", err)
		at.saveDecision(record)
		return fmt.Errorf("failed to build trading context: %w", err)
	}

	// Save equity snapshot independently (decoupled from AI decision, used for drawing profit curve)
	// NOTE: Must be called BEFORE candidate coins check to ensure equity is always recorded
	at.saveEquitySnapshot(ctx)

	// If no candidate coins, skip this cycle but still persist account snapshot.
	if len(ctx.CandidateCoins) == 0 {
		logger.Infof("No candidate coins available, skipping this cycle")
		record.Success = true
		record.ExecutionLog = append(record.ExecutionLog, "No candidate coins available, cycle skipped")
		record.AccountState = store.AccountSnapshot{
			TotalBalance:          ctx.Account.TotalEquity,
			AvailableBalance:      ctx.Account.AvailableBalance,
			TotalUnrealizedProfit: ctx.Account.UnrealizedPnL,
			PositionCount:         ctx.Account.PositionCount,
			InitialBalance:        at.initialBalance,
		}
		at.saveDecision(record)
		return nil
	}

	logger.Info(strings.Repeat("=", 70))
	for _, coin := range ctx.CandidateCoins {
		record.CandidateCoins = append(record.CandidateCoins, coin.Symbol)
	}

	logger.Infof("濡絽鍟幆?Account equity: %.2f USDT | Available: %.2f USDT | Positions: %d",
		ctx.Account.TotalEquity, ctx.Account.AvailableBalance, ctx.Account.PositionCount)

	// 5. Use strategy engine to call AI for decision
	logger.Infof("濡絽鍠氬Ο?Requesting AI analysis and decision... [Strategy Engine]")
	aiDecision, err := kernel.GetFullDecisionWithStrategy(ctx, at.mcpClient, at.strategyEngine, "balanced")

	if aiDecision != nil && aiDecision.AIRequestDurationMs > 0 {
		record.AIRequestDurationMs = aiDecision.AIRequestDurationMs
		logger.Infof("闂佸啿鐡ㄩ崬鑽ょ箔?AI call duration: %.2f seconds", float64(record.AIRequestDurationMs)/1000)
		record.ExecutionLog = append(record.ExecutionLog,
			fmt.Sprintf("AI call duration: %d ms", record.AIRequestDurationMs))
	}

	// Save chain of thought, decisions, and input prompt even if there's an error (for debugging)
	if aiDecision != nil {
		record.SystemPrompt = aiDecision.SystemPrompt // Save system prompt
		record.InputPrompt = aiDecision.UserPrompt
		record.CoTTrace = aiDecision.CoTTrace
		record.RawResponse = aiDecision.RawResponse // Save raw AI response for debugging
		if len(aiDecision.Decisions) > 0 {
			decisionJSON, _ := json.MarshalIndent(aiDecision.Decisions, "", "  ")
			record.DecisionJSON = string(decisionJSON)
		}
	}

	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Failed to get AI decision: %v", err)

		// Print system prompt and AI chain of thought (output even with errors for debugging)
		if aiDecision != nil {
			logger.Info("\n" + strings.Repeat("=", 70) + "\n")
			logger.Infof("濡絽鍟幆?System prompt (error case)")
			logger.Info(strings.Repeat("=", 70))
			logger.Info(aiDecision.SystemPrompt)
			logger.Info(strings.Repeat("=", 70))

			if aiDecision.CoTTrace != "" {
				logger.Info("\n" + strings.Repeat("-", 70) + "\n")
				logger.Info("濡絽鍟亸?AI chain of thought analysis (error case):")
				logger.Info(strings.Repeat("-", 70))
				logger.Info(aiDecision.CoTTrace)
				logger.Info(strings.Repeat("-", 70))
			}
		}

		at.saveDecision(record)
		return fmt.Errorf("failed to get AI decision: %w", err)
	}

	// // 5. Print system prompt
	// logger.Infof("\n" + strings.Repeat("=", 70))
	// logger.Infof("濡絽鍟幆?System prompt [template: %s]", at.systemPromptTemplate)
	// logger.Info(strings.Repeat("=", 70))
	// logger.Info(decision.SystemPrompt)
	// logger.Infof(strings.Repeat("=", 70) + "\n")

	// 6. Print AI chain of thought
	// logger.Infof("\n" + strings.Repeat("-", 70))
	// logger.Info("濡絽鍟亸?AI chain of thought analysis:")
	// logger.Info(strings.Repeat("-", 70))
	// logger.Info(decision.CoTTrace)
	// logger.Infof(strings.Repeat("-", 70) + "\n")

	// 7. Print AI decisions
	// logger.Infof("濡絽鍟幆?AI decision list (%d items):\n", len(kernel.Decisions))
	// for i, d := range kernel.Decisions {
	//     logger.Infof("  [%d] %s: %s - %s", i+1, d.Symbol, d.Action, d.Reasoning)
	//     if d.Action == "open_long" || d.Action == "open_short" {
	//        logger.Infof("      Leverage: %dx | Position: %.2f USDT | Stop loss: %.4f | Take profit: %.4f",
	//           d.Leverage, d.PositionSizeUSD, d.StopLoss, d.TakeProfit)
	//     }
	// }
	logger.Info()
	logger.Info(strings.Repeat("-", 70))
	// 8. Sort decisions: ensure close positions first, then open positions (prevent position stacking overflow)
	logger.Info(strings.Repeat("-", 70))

	// 8. Sort decisions: ensure close positions first, then open positions (prevent position stacking overflow)
	sortedDecisions := sortDecisionsByPriority(aiDecision.Decisions)

	logger.Info("濡絽鍟弫?Execution order (optimized): Close positions first 闂?Open positions later")
	for i, d := range sortedDecisions {
		logger.Infof("  [%d] %s %s", i+1, d.Symbol, d.Action)
	}
	logger.Info()

	// Check if trader is stopped before executing any decisions (prevent trades after Stop())
	at.isRunningMutex.RLock()
	running = at.isRunning
	at.isRunningMutex.RUnlock()
	if !running {
		logger.Infof("闂?Trader stopped before decision execution, aborting cycle #%d", at.callCount)
		return nil
	}

	alertOnlyMode := at.isAlertOnlyMode()
	if alertOnlyMode {
		logger.Infof("濡絽鍟幉?[%s] Alert-only mode enabled: decisions will be sent as signals without placing orders", at.name)
	}

	// Execute decisions and record results
	for _, d := range sortedDecisions {
		// Check if trader is stopped before each decision (allow immediate stop during execution)
		at.isRunningMutex.RLock()
		running = at.isRunning
		at.isRunningMutex.RUnlock()
		if !running {
			logger.Infof("闂?Trader stopped during decision execution, aborting remaining decisions")
			break
		}

		actionRecord := store.DecisionAction{
			Action:          d.Action,
			Symbol:          d.Symbol,
			Quantity:        0,
			Leverage:        d.Leverage,
			Price:           0,
			EntryPrice:      d.EntryPrice,
			PositionSizeUSD: d.PositionSizeUSD,
			StopLoss:        d.StopLoss,
			TakeProfit:      d.TakeProfit,
			Confidence:      d.Confidence,
			Reasoning:       d.Reasoning,
			Timestamp:       time.Now().UTC(),
			Success:         false,
		}

		if isOpenDecisionAction(d.Action) {
			minConf := at.minConfidenceThreshold()
			if minConf > 0 && d.Confidence < minConf {
				actionRecord.Success = true
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("[SKIP] %s %s skipped: confidence %d < min %d", d.Symbol, d.Action, d.Confidence, minConf))
				record.Decisions = append(record.Decisions, actionRecord)
				continue
			}
		}

		if alertOnlyMode {
			if err := at.handleAlertOnlyDecision(&d, &actionRecord); err != nil {
				logger.Infof("闂?Failed to process signal (%s %s): %v", d.Symbol, d.Action, err)
				actionRecord.Error = err.Error()
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("[ERROR] %s %s signal failed: %v", d.Symbol, d.Action, err))
			} else if reason, skipped := actionSkipReason(&actionRecord); skipped {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("[SKIP] %s %s skipped: %s", d.Symbol, d.Action, reason))
			} else if actionRecord.Action == "open_long" || actionRecord.Action == "open_short" || actionRecord.Action == "close_long" || actionRecord.Action == "close_short" {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("[SIGNAL] %s %s signal sent", d.Symbol, d.Action))
			} else {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("[WAIT] %s %s", d.Symbol, d.Action))
			}
			record.Decisions = append(record.Decisions, actionRecord)
			continue
		}

		if err := at.executeDecisionWithRecord(&d, &actionRecord); err != nil {
			logger.Infof("闂?Failed to execute decision (%s %s): %v", d.Symbol, d.Action, err)
			actionRecord.Error = err.Error()
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("[ERROR] %s %s failed: %v", d.Symbol, d.Action, err))
		} else {
			actionRecord.Success = true
			if reason, skipped := actionSkipReason(&actionRecord); skipped {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("[SKIP] %s %s skipped: %s", d.Symbol, d.Action, reason))
			} else {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("[OK] %s %s succeeded", d.Symbol, d.Action))
				// Brief delay after successful execution
				time.Sleep(1 * time.Second)
			}
		}

		record.Decisions = append(record.Decisions, actionRecord)
	}

	// 9. Save decision record
	if err := at.saveDecision(record); err != nil {
		logger.Infof("闂?Failed to save decision record: %v", err)
	}

	return nil
}

// buildTradingContext builds trading context
func (at *AutoTrader) buildTradingContext() (*kernel.Context, error) {
	// 1. Get account information
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get account balance: %w", err)
	}

	// Get account fields
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0
	totalEquity := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Use totalEquity directly if provided by trader (more accurate)
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		totalEquity = eq
	} else {
		// Fallback: Total Equity = Wallet balance + Unrealized profit
		totalEquity = totalWalletBalance + totalUnrealizedProfit
	}

	// 2. Get position information
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	var positionInfos []kernel.PositionInfo
	totalMarginUsed := 0.0

	// Current position key set (for cleaning up closed position records)
	currentPositionKeys := make(map[string]bool)

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // Short position quantity is negative, convert to positive
		}

		// Skip closed positions (quantity = 0), prevent "ghost positions" from being passed to AI
		if quantity == 0 {
			continue
		}

		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		// Calculate margin used (estimated)
		leverage := 10 // Default value, should actually be fetched from position info
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed

		// Calculate P&L percentage (based on margin, considering leverage)
		pnlPct := calculatePnLPercentage(unrealizedPnl, marginUsed)

		// Get position open time from exchange (preferred) or fallback to local tracking
		posKey := symbol + "_" + side
		currentPositionKeys[posKey] = true

		var updateTime int64
		// Priority 1: Get from database (trader_positions table) - most accurate
		if at.store != nil {
			if dbPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, symbol, side); err == nil && dbPos != nil {
				if dbPos.EntryTime > 0 {
					updateTime = dbPos.EntryTime
				}
			}
		}
		// Priority 2: Get from exchange API (Bybit: createdTime, OKX: createdTime)
		if updateTime == 0 {
			if createdTime, ok := pos["createdTime"].(int64); ok && createdTime > 0 {
				updateTime = createdTime
			}
		}
		// Priority 3: Fallback to local tracking
		if updateTime == 0 {
			if _, exists := at.positionFirstSeenTime[posKey]; !exists {
				at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()
			}
			updateTime = at.positionFirstSeenTime[posKey]
		}

		// Get peak profit rate for this position
		at.peakPnLCacheMutex.RLock()
		peakPnlPct := at.peakPnLCache[posKey]
		at.peakPnLCacheMutex.RUnlock()

		positionInfos = append(positionInfos, kernel.PositionInfo{
			Symbol:           symbol,
			Side:             side,
			EntryPrice:       entryPrice,
			MarkPrice:        markPrice,
			Quantity:         quantity,
			Leverage:         leverage,
			UnrealizedPnL:    unrealizedPnl,
			UnrealizedPnLPct: pnlPct,
			PeakPnLPct:       peakPnlPct,
			LiquidationPrice: liquidationPrice,
			MarginUsed:       marginUsed,
			UpdateTime:       updateTime,
		})
	}

	// Clean up closed position records
	for key := range at.positionFirstSeenTime {
		if !currentPositionKeys[key] {
			delete(at.positionFirstSeenTime, key)
		}
	}

	// 3. Use strategy engine to get candidate coins (must have strategy engine)
	var candidateCoins []kernel.CandidateCoin
	if at.strategyEngine == nil {
		logger.Infof("闂佸疇娉曟刊瀵哥箔?[%s] No strategy engine configured, skipping candidate coins", at.name)
	} else {
		coins, err := at.strategyEngine.GetCandidateCoins()
		if err != nil {
			// Log warning but don't fail - equity snapshot should still be saved
			logger.Infof("闂佸疇娉曟刊瀵哥箔?[%s] Failed to get candidate coins: %v (will use empty list)", at.name, err)
		} else {
			candidateCoins = coins
			logger.Infof("濡絽鍟幆?[%s] Strategy engine fetched candidate coins: %d", at.name, len(candidateCoins))
		}
	}

	// 4. Calculate total P&L
	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	// 5. Get leverage from strategy config
	strategyConfig := at.strategyEngine.GetConfig()
	btcEthLeverage := strategyConfig.RiskControl.BTCETHMaxLeverage
	altcoinLeverage := strategyConfig.RiskControl.AltcoinMaxLeverage
	logger.Infof("濡絽鍟幆?[%s] Strategy leverage config: BTC/ETH=%dx, Altcoin=%dx", at.name, btcEthLeverage, altcoinLeverage)

	// 6. Build context
	ctx := &kernel.Context{
		CurrentTime:     time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		RuntimeMinutes:  int(time.Since(at.startTime).Minutes()),
		CallCount:       at.callCount,
		BTCETHLeverage:  btcEthLeverage,
		AltcoinLeverage: altcoinLeverage,
		Account: kernel.AccountInfo{
			TotalEquity:      totalEquity,
			AvailableBalance: availableBalance,
			UnrealizedPnL:    totalUnrealizedProfit,
			TotalPnL:         totalPnL,
			TotalPnLPct:      totalPnLPct,
			MarginUsed:       totalMarginUsed,
			MarginUsedPct:    marginUsedPct,
			PositionCount:    len(positionInfos),
		},
		Positions:      positionInfos,
		CandidateCoins: candidateCoins,
	}

	// 7. Add recent closed trades (if store is available)
	if at.store != nil {
		// Get recent 10 closed trades for AI context
		recentTrades, err := at.store.Position().GetRecentTrades(at.id, 10)
		if err != nil {
			logger.Infof("闂佸疇娉曟刊瀵哥箔?[%s] Failed to get recent trades: %v", at.name, err)
		} else {
			logger.Infof("濡絽鍟幆?[%s] Found %d recent closed trades for AI context", at.name, len(recentTrades))
			for _, trade := range recentTrades {
				// Convert Unix timestamps to formatted strings for AI readability
				entryTimeStr := ""
				if trade.EntryTime > 0 {
					entryTimeStr = time.Unix(trade.EntryTime, 0).UTC().Format("01-02 15:04 UTC")
				}
				exitTimeStr := ""
				if trade.ExitTime > 0 {
					exitTimeStr = time.Unix(trade.ExitTime, 0).UTC().Format("01-02 15:04 UTC")
				}

				ctx.RecentOrders = append(ctx.RecentOrders, kernel.RecentOrder{
					Symbol:       trade.Symbol,
					Side:         trade.Side,
					EntryPrice:   trade.EntryPrice,
					ExitPrice:    trade.ExitPrice,
					RealizedPnL:  trade.RealizedPnL,
					PnLPct:       trade.PnLPct,
					EntryTime:    entryTimeStr,
					ExitTime:     exitTimeStr,
					HoldDuration: trade.HoldDuration,
				})
			}
		}
		// Get trading statistics for AI context
		stats, err := at.store.Position().GetFullStats(at.id)
		if err != nil {
			logger.Infof("闂佸疇娉曟刊瀵哥箔?[%s] Failed to get trading stats: %v", at.name, err)
		} else if stats == nil {
			logger.Infof("闂佸疇娉曟刊瀵哥箔?[%s] GetFullStats returned nil", at.name)
		} else if stats.TotalTrades == 0 {
			logger.Infof("闂佸疇娉曟刊瀵哥箔?[%s] GetFullStats returned 0 trades (traderID=%s)", at.name, at.id)
		} else {
			ctx.TradingStats = &kernel.TradingStats{
				TotalTrades:    stats.TotalTrades,
				WinRate:        stats.WinRate,
				ProfitFactor:   stats.ProfitFactor,
				SharpeRatio:    stats.SharpeRatio,
				TotalPnL:       stats.TotalPnL,
				AvgWin:         stats.AvgWin,
				AvgLoss:        stats.AvgLoss,
				MaxDrawdownPct: stats.MaxDrawdownPct,
			}
			logger.Infof("濡絽鍟幆?[%s] Trading stats: %d trades, %.1f%% win rate, PF=%.2f, Sharpe=%.2f, DD=%.1f%%",
				at.name, stats.TotalTrades, stats.WinRate, stats.ProfitFactor, stats.SharpeRatio, stats.MaxDrawdownPct)
		}
	} else {
		logger.Infof("闂佸疇娉曟刊瀵哥箔?[%s] Store is nil, cannot get recent trades", at.name)
	}

	// 8. Get quantitative data (if enabled in strategy config)
	if strategyConfig.Indicators.EnableQuantData {
		// Collect symbols to query (candidate coins + position coins)
		symbolsToQuery := make(map[string]bool)
		for _, coin := range candidateCoins {
			symbolsToQuery[coin.Symbol] = true
		}
		for _, pos := range positionInfos {
			symbolsToQuery[pos.Symbol] = true
		}

		symbols := make([]string, 0, len(symbolsToQuery))
		for sym := range symbolsToQuery {
			symbols = append(symbols, sym)
		}

		logger.Infof("濡絽鍟幆?[%s] Fetching quantitative data for %d symbols...", at.name, len(symbols))
		ctx.QuantDataMap = at.strategyEngine.FetchQuantDataBatch(symbols)
		logger.Infof("濡絽鍟幆?[%s] Successfully fetched quantitative data for %d symbols", at.name, len(ctx.QuantDataMap))
	}

	// 9. Get OI ranking data (market-wide position changes)
	if strategyConfig.Indicators.EnableOIRanking {
		logger.Infof("濡絽鍟幆?[%s] Fetching OI ranking data...", at.name)
		ctx.OIRankingData = at.strategyEngine.FetchOIRankingData()
		if ctx.OIRankingData != nil {
			logger.Infof("濡絽鍟幆?[%s] OI ranking data ready: %d top, %d low positions",
				at.name, len(ctx.OIRankingData.TopPositions), len(ctx.OIRankingData.LowPositions))
		}
	}

	// 10. Get NetFlow ranking data (market-wide fund flow)
	if strategyConfig.Indicators.EnableNetFlowRanking {
		logger.Infof("濡絽鍟亸?[%s] Fetching NetFlow ranking data...", at.name)
		ctx.NetFlowRankingData = at.strategyEngine.FetchNetFlowRankingData()
		if ctx.NetFlowRankingData != nil {
			logger.Infof("濡絽鍟亸?[%s] NetFlow ranking data ready: inst_in=%d, inst_out=%d",
				at.name, len(ctx.NetFlowRankingData.InstitutionFutureTop), len(ctx.NetFlowRankingData.InstitutionFutureLow))
		}
	}

	// 11. Get Price ranking data (market-wide gainers/losers)
	if strategyConfig.Indicators.EnablePriceRanking {
		logger.Infof("濡絽鍟幆?[%s] Fetching Price ranking data...", at.name)
		ctx.PriceRankingData = at.strategyEngine.FetchPriceRankingData()
		if ctx.PriceRankingData != nil {
			logger.Infof("濡絽鍟幆?[%s] Price ranking data ready for %d durations",
				at.name, len(ctx.PriceRankingData.Durations))
		}
	}

	return ctx, nil
}

// executeDecisionWithRecord executes AI decision and records detailed information
func (at *AutoTrader) executeDecisionWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	switch decision.Action {
	case "open_long":
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "open_short":
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "close_long":
		return at.executeCloseLongWithRecord(decision, actionRecord)
	case "close_short":
		return at.executeCloseShortWithRecord(decision, actionRecord)
	case "hold", "wait":
		// No execution needed, just record
		return nil
	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}
}

// refreshStrategyConfigIfNeeded hot-reloads strategy config when DB version changes.
func (at *AutoTrader) refreshStrategyConfigIfNeeded() {
	if at.store == nil || strings.TrimSpace(at.strategyID) == "" || strings.TrimSpace(at.userID) == "" {
		return
	}

	st, err := at.store.Strategy().GetAccessible(at.userID, at.strategyID)
	if err != nil || st == nil {
		if err != nil {
			logger.Infof("閳跨媴绗?[%s] Strategy hot-reload skipped (load failed): %v", at.name, err)
		}
		return
	}

	if !at.lastStrategyUpdatedAt.IsZero() && !st.UpdatedAt.After(at.lastStrategyUpdatedAt) {
		return
	}

	cfg, err := st.ParseConfig()
	if err != nil {
		logger.Infof("閳跨媴绗?[%s] Strategy hot-reload skipped (parse failed): %v", at.name, err)
		return
	}

	at.config.StrategyConfig = cfg
	at.strategyEngine = kernel.NewStrategyEngine(cfg)
	at.lastStrategyUpdatedAt = st.UpdatedAt

	logger.Infof("棣冩敡 [%s] Strategy hot-reloaded: id=%s updated_at=%s",
		at.name, at.strategyID, st.UpdatedAt.UTC().Format(time.RFC3339))
}

// ExecuteDecision executes a trading decision from external sources (e.g., debate consensus)
// This is a public method that can be called by other modules
func (at *AutoTrader) ExecuteDecision(d *kernel.Decision) error {
	logger.Infof("[%s] Executing external decision: %s %s", at.name, d.Action, d.Symbol)

	if isOpenDecisionAction(d.Action) {
		minConf := at.minConfidenceThreshold()
		if minConf > 0 && d.Confidence < minConf {
			logger.Infof("[%s] Skip external %s %s: confidence %d < min %d", at.name, d.Action, d.Symbol, d.Confidence, minConf)
			return nil
		}
	}

	// Create a minimal action record for tracking
	actionRecord := &store.DecisionAction{
		Symbol:          d.Symbol,
		Action:          d.Action,
		Leverage:        d.Leverage,
		EntryPrice:      d.EntryPrice,
		PositionSizeUSD: d.PositionSizeUSD,
		StopLoss:        d.StopLoss,
		TakeProfit:      d.TakeProfit,
		Confidence:      d.Confidence,
		Reasoning:       d.Reasoning,
	}

	if at.isAlertOnlyMode() {
		if err := at.handleAlertOnlyDecision(d, actionRecord); err != nil {
			logger.Errorf("[%s] External signal processing failed: %v", at.name, err)
			return err
		}
		if reason, skipped := actionSkipReason(actionRecord); skipped {
			logger.Infof("[%s] External decision skipped in alert_only mode: %s", at.name, reason)
			return nil
		}
		logger.Infof("[%s] External decision converted to signal: %s %s", at.name, d.Action, d.Symbol)
		return nil
	}

	// Execute the decision
	err := at.executeDecisionWithRecord(d, actionRecord)
	if err != nil {
		logger.Errorf("[%s] External decision execution failed: %v", at.name, err)
		return err
	}
	if reason, skipped := actionSkipReason(actionRecord); skipped {
		logger.Infof("[%s] External decision skipped by risk control: %s", at.name, reason)
		return nil
	}

	logger.Infof("[%s] External decision executed successfully: %s %s", at.name, d.Action, d.Symbol)
	return nil
}

// executeOpenLongWithRecord executes open long position and records detailed information
func (at *AutoTrader) executeOpenLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	return at.executeOpenWithRecord(decision, actionRecord, "long")
}

// executeOpenShortWithRecord executes open short position and records detailed information
func (at *AutoTrader) executeOpenShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	return at.executeOpenWithRecord(decision, actionRecord, "short")
}

func (at *AutoTrader) executeOpenWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction, side string) error {
	if side != "long" && side != "short" {
		return fmt.Errorf("unknown open side: %s", side)
	}
	logger.Infof("  Open %s: %s", side, decision.Symbol)

	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("failed to get positions: %w", err)
	}
	if err := at.enforceMaxPositions(len(positions)); err != nil {
		return err
	}
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == side {
			return fmt.Errorf("%s already has %s position, close it first", decision.Symbol, side)
		}
	}

	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	riskControl := at.riskControlConfig()
	if shouldSkipByPriceDeviation(decision.EntryPrice, marketData.CurrentPrice, riskControl.EffectivePriceDeviationLimitPct()) {
		reason := fmt.Sprintf("skip %s %s: entry deviation too high (ai=%.6f market=%.6f limit=%.2f%%)",
			decision.Action, decision.Symbol, decision.EntryPrice, marketData.CurrentPrice, riskControl.EffectivePriceDeviationLimitPct())
		logger.Infof("[RISK CONTROL] %s", reason)
		markActionSkipped(actionRecord, reason)
		return nil
	}

	balance, err := at.trader.GetBalance()
	if err != nil {
		return fmt.Errorf("failed to get account balance: %w", err)
	}
	availableBalance := 0.0
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}
	equity := 0.0
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		equity = eq
	} else if eq, ok := balance["totalWalletBalance"].(float64); ok && eq > 0 {
		equity = eq
	} else {
		equity = availableBalance
	}

	adjustedPositionSize, wasCapped := at.enforcePositionValueRatio(decision.PositionSizeUSD, equity, decision.Symbol)
	if wasCapped {
		decision.PositionSizeUSD = adjustedPositionSize
	}

	marginFactor := 1.01/float64(decision.Leverage) + 0.001
	maxAffordablePositionSize := availableBalance / marginFactor
	actualPositionSize := decision.PositionSizeUSD
	if actualPositionSize > maxAffordablePositionSize {
		adjustedSize := maxAffordablePositionSize * 0.98
		logger.Infof("  Position size %.2f exceeds max affordable %.2f, auto-reducing to %.2f",
			actualPositionSize, maxAffordablePositionSize, adjustedSize)
		actualPositionSize = adjustedSize
		decision.PositionSizeUSD = actualPositionSize
	}
	if err := at.enforceMinPositionSize(decision.PositionSizeUSD); err != nil {
		return err
	}

	quantity := actualPositionSize / marketData.CurrentPrice
	actionRecord.Quantity = quantity
	actionRecord.Price = marketData.CurrentPrice

	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Infof("  Failed to set margin mode: %v", err)
	}

	var (
		order        map[string]interface{}
		positionSide string
		openAction   string
	)
	if side == "long" {
		order, err = at.trader.OpenLong(decision.Symbol, quantity, decision.Leverage)
		positionSide = "LONG"
		openAction = "open_long"
	} else {
		order, err = at.trader.OpenShort(decision.Symbol, quantity, decision.Leverage)
		positionSide = "SHORT"
		openAction = "open_short"
	}
	if err != nil {
		return err
	}
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}
	logger.Infof("  Position opened successfully, order ID: %v, quantity: %.6f", order["orderId"], quantity)

	at.recordAndConfirmOrder(order, decision.Symbol, openAction, quantity, marketData.CurrentPrice, decision.Leverage, 0)

	posKey := decision.Symbol + "_" + side
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	filledEntry := at.resolvePositionEntryPrice(decision.Symbol, side, marketData.CurrentPrice)
	actionRecord.Price = filledEntry
	if err := at.applyPostFillRRGuard(decision, actionRecord, side, filledEntry); err != nil {
		return err
	}
	if isActionSkipped(actionRecord) {
		return nil
	}

	if err := at.applyProtectionOrders(decision, actionRecord, positionSide, side, quantity); err != nil {
		return err
	}

	at.notifyStrategySignal(decision, actionRecord)
	return nil
}

// executeCloseLongWithRecord executes close long position and records detailed information
func (at *AutoTrader) executeCloseLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  濡絽鍟弫?Close long: %s", decision.Symbol)

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	// Normalize symbol for database lookup
	normalizedSymbol := market.Normalize(decision.Symbol)

	// Get entry price and quantity - prioritize local database for accurate quantity
	var entryPrice float64
	var quantity float64

	// First try to get from local database (more accurate for quantity)
	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, "LONG"); err == nil && openPos != nil {
			quantity = openPos.Quantity
			entryPrice = openPos.EntryPrice
			logger.Infof("  濡絽鍟幆?Using local position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
		}
	}

	// Fallback to exchange API if local data not found
	if quantity == 0 {
		positions, err := at.trader.GetPositions()
		if err == nil {
			for _, pos := range positions {
				if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
					if ep, ok := pos["entryPrice"].(float64); ok {
						entryPrice = ep
					}
					if amt, ok := pos["positionAmt"].(float64); ok && amt > 0 {
						quantity = amt
					}
					break
				}
			}
		}
		logger.Infof("  濡絽鍟幆?Using exchange position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
	}

	// Close position
	order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_long", quantity, marketData.CurrentPrice, 0, entryPrice)

	logger.Infof("  闂?Position closed successfully")
	at.notifyStrategySignal(decision, actionRecord)
	return nil
}

// executeCloseShortWithRecord executes close short position and records detailed information
func (at *AutoTrader) executeCloseShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  濡絽鍟弫?Close short: %s", decision.Symbol)

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	// Normalize symbol for database lookup
	normalizedSymbol := market.Normalize(decision.Symbol)

	// Get entry price and quantity - prioritize local database for accurate quantity
	var entryPrice float64
	var quantity float64

	// First try to get from local database (more accurate for quantity)
	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, "SHORT"); err == nil && openPos != nil {
			quantity = openPos.Quantity
			entryPrice = openPos.EntryPrice
			logger.Infof("  濡絽鍟幆?Using local position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
		}
	}

	// Fallback to exchange API if local data not found
	if quantity == 0 {
		positions, err := at.trader.GetPositions()
		if err == nil {
			for _, pos := range positions {
				if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
					if ep, ok := pos["entryPrice"].(float64); ok {
						entryPrice = ep
					}
					if amt, ok := pos["positionAmt"].(float64); ok {
						quantity = -amt // positionAmt is negative for short
					}
					break
				}
			}
		}
		logger.Infof("  濡絽鍟幆?Using exchange position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
	}

	// Close position
	order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_short", quantity, marketData.CurrentPrice, 0, entryPrice)

	logger.Infof("  闂?Position closed successfully")
	at.notifyStrategySignal(decision, actionRecord)
	return nil
}

// GetID gets trader ID
func (at *AutoTrader) GetID() string {
	return at.id
}

// GetUnderlyingTrader returns the underlying Trader interface implementation
// This is used by grid trading and other components that need direct exchange access
func (at *AutoTrader) GetUnderlyingTrader() Trader {
	return at.trader
}

// GetName gets trader name
func (at *AutoTrader) GetName() string {
	return at.name
}

// GetAIModel gets AI model
func (at *AutoTrader) GetAIModel() string {
	return at.aiModel
}

// GetExchange gets exchange
func (at *AutoTrader) GetExchange() string {
	return at.exchange
}

// GetExecutionMode gets trader execution mode
func (at *AutoTrader) GetExecutionMode() string {
	return store.NormalizeTraderExecutionMode(at.config.ExecutionMode)
}

// GetShowInCompetition returns whether trader should be shown in competition
func (at *AutoTrader) GetShowInCompetition() bool {
	return at.showInCompetition
}

// SetShowInCompetition sets whether trader should be shown in competition
func (at *AutoTrader) SetShowInCompetition(show bool) {
	at.showInCompetition = show
}

// SetCustomPrompt sets custom trading strategy prompt
func (at *AutoTrader) SetCustomPrompt(prompt string) {
	at.customPrompt = prompt
}

// SetOverrideBasePrompt sets whether to override base prompt
func (at *AutoTrader) SetOverrideBasePrompt(override bool) {
	at.overrideBasePrompt = override
}

// GetSystemPromptTemplate gets current system prompt template name (from strategy config)
func (at *AutoTrader) GetSystemPromptTemplate() string {
	if at.strategyEngine != nil {
		config := at.strategyEngine.GetConfig()
		if config.CustomPrompt != "" {
			return "custom"
		}
	}
	return "strategy"
}

// saveEquitySnapshot saves equity snapshot independently (for drawing profit curve, decoupled from AI decision)
func (at *AutoTrader) saveEquitySnapshot(ctx *kernel.Context) {
	if at.store == nil || ctx == nil {
		return
	}

	snapshot := &store.EquitySnapshot{
		TraderID:      at.id,
		Timestamp:     time.Now().UTC(),
		TotalEquity:   ctx.Account.TotalEquity,
		Balance:       ctx.Account.TotalEquity - ctx.Account.UnrealizedPnL,
		UnrealizedPnL: ctx.Account.UnrealizedPnL,
		PositionCount: ctx.Account.PositionCount,
		MarginUsedPct: ctx.Account.MarginUsedPct,
	}

	if err := at.store.Equity().Save(snapshot); err != nil {
		logger.Infof("闂佸疇娉曟刊瀵哥箔?Failed to save equity snapshot: %v", err)
	}
}

// saveDecision saves AI decision log to database (only records AI input/output, for debugging)
func (at *AutoTrader) saveDecision(record *store.DecisionRecord) error {
	if at.store == nil {
		return nil
	}

	at.cycleNumber++
	record.CycleNumber = at.cycleNumber
	record.TraderID = at.id

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}

	if err := at.store.Decision().LogDecision(record); err != nil {
		logger.Infof("闂佸疇娉曟刊瀵哥箔?Failed to save decision record: %v", err)
		return err
	}

	logger.Infof("濡絽鍟幉?Decision record saved: trader=%s, cycle=%d", at.id, at.cycleNumber)
	return nil
}

// GetStore gets data store (for external access to decision records, etc.)
func (at *AutoTrader) GetStore() *store.Store {
	return at.store
}

// GetStatus gets system status (for API)
func (at *AutoTrader) GetStatus() map[string]interface{} {
	aiProvider := "DeepSeek"
	if at.config.UseQwen {
		aiProvider = "Qwen"
	}

	at.isRunningMutex.RLock()
	isRunning := at.isRunning
	at.isRunningMutex.RUnlock()

	result := map[string]interface{}{
		"trader_id":       at.id,
		"trader_name":     at.name,
		"ai_model":        at.aiModel,
		"exchange":        at.exchange,
		"is_running":      isRunning,
		"start_time":      at.startTime.Format(time.RFC3339),
		"runtime_minutes": int(time.Since(at.startTime).Minutes()),
		"call_count":      at.callCount,
		"initial_balance": at.initialBalance,
		"scan_interval":   at.config.ScanInterval.String(),
		"stop_until":      at.stopUntil.Format(time.RFC3339),
		"last_reset_time": at.lastResetTime.Format(time.RFC3339),
		"ai_provider":     aiProvider,
		"execution_mode":  at.GetExecutionMode(),
	}

	// Add strategy info
	if at.config.StrategyConfig != nil {
		result["strategy_type"] = at.config.StrategyConfig.StrategyType
		if at.config.StrategyConfig.GridConfig != nil {
			result["grid_symbol"] = at.config.StrategyConfig.GridConfig.Symbol
		}
	}

	return result
}

// GetAccountInfo gets account information (for API)
func (at *AutoTrader) GetAccountInfo() (map[string]interface{}, error) {
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Get account fields
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0
	totalEquity := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Use totalEquity directly if provided by trader (more accurate)
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		totalEquity = eq
	} else {
		// Fallback: Total Equity = Wallet balance + Unrealized profit
		totalEquity = totalWalletBalance + totalUnrealizedProfit
	}

	// Get positions to calculate total margin
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	totalMarginUsed := 0.0
	totalUnrealizedPnLCalculated := 0.0
	for _, pos := range positions {
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		totalUnrealizedPnLCalculated += unrealizedPnl

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed
	}

	// Verify unrealized P&L consistency (API value vs calculated from positions)
	// Note: Lighter API may return 0 for unrealized PnL, this is a known limitation
	diff := math.Abs(totalUnrealizedProfit - totalUnrealizedPnLCalculated)
	if diff > 5.0 { // Only warn if difference is significant (> 5 USDT)
		logger.Infof("闂佸疇娉曟刊瀵哥箔?Unrealized P&L inconsistency (Lighter API limitation): API=%.4f, Calculated=%.4f, Diff=%.4f",
			totalUnrealizedProfit, totalUnrealizedPnLCalculated, diff)
	}

	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	} else {
		logger.Infof("闂佸疇娉曟刊瀵哥箔?Initial Balance abnormal: %.2f, cannot calculate P&L percentage", at.initialBalance)
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	return map[string]interface{}{
		// Core fields
		"total_equity":      totalEquity,           // Account equity = wallet + unrealized
		"wallet_balance":    totalWalletBalance,    // Wallet balance (excluding unrealized P&L)
		"unrealized_profit": totalUnrealizedProfit, // Unrealized P&L (official value from exchange API)
		"available_balance": availableBalance,      // Available balance

		// P&L statistics
		"total_pnl":       totalPnL,          // Total P&L = equity - initial
		"total_pnl_pct":   totalPnLPct,       // Total P&L percentage
		"initial_balance": at.initialBalance, // Initial balance
		"daily_pnl":       at.dailyPnL,       // Daily P&L

		// Position information
		"position_count":  len(positions),  // Position count
		"margin_used":     totalMarginUsed, // Margin used
		"margin_used_pct": marginUsedPct,   // Margin usage rate
	}, nil
}

// GetPositions gets position list (for API)
func (at *AutoTrader) GetPositions() ([]map[string]interface{}, error) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		// Calculate margin used
		marginUsed := (quantity * markPrice) / float64(leverage)

		// Calculate P&L percentage (based on margin)
		pnlPct := calculatePnLPercentage(unrealizedPnl, marginUsed)

		result = append(result, map[string]interface{}{
			"symbol":             symbol,
			"side":               side,
			"entry_price":        entryPrice,
			"mark_price":         markPrice,
			"quantity":           quantity,
			"leverage":           leverage,
			"unrealized_pnl":     unrealizedPnl,
			"unrealized_pnl_pct": pnlPct,
			"liquidation_price":  liquidationPrice,
			"margin_used":        marginUsed,
		})
	}

	return result, nil
}

// calculatePnLPercentage calculates P&L percentage (based on margin, automatically considers leverage)
// Return rate = Unrealized P&L / Margin 闁?100%
func calculatePnLPercentage(unrealizedPnl, marginUsed float64) float64 {
	if marginUsed > 0 {
		return (unrealizedPnl / marginUsed) * 100
	}
	return 0.0
}

// sortDecisionsByPriority sorts decisions: close positions first, then open positions, finally hold/wait
// This avoids position stacking overflow when changing positions
func sortDecisionsByPriority(decisions []kernel.Decision) []kernel.Decision {
	if len(decisions) <= 1 {
		return decisions
	}

	// Define priority
	getActionPriority := func(action string) int {
		switch action {
		case "close_long", "close_short":
			return 1 // Highest priority: close positions first
		case "open_long", "open_short":
			return 2 // Second priority: open positions later
		case "hold", "wait":
			return 3 // Lowest priority: wait
		default:
			return 999 // Unknown actions at the end
		}
	}

	// Copy decision list
	sorted := make([]kernel.Decision, len(decisions))
	copy(sorted, decisions)

	// Sort by priority
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if getActionPriority(sorted[i].Action) > getActionPriority(sorted[j].Action) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// startDrawdownMonitor starts drawdown monitoring
func (at *AutoTrader) startDrawdownMonitor() {
	at.monitorWg.Add(1)
	go func() {
		defer at.monitorWg.Done()

		ticker := time.NewTicker(1 * time.Minute) // Check every minute
		defer ticker.Stop()

		logger.Info("濡絽鍟幆?Started position drawdown monitoring (check every minute)")

		for {
			select {
			case <-ticker.C:
				at.checkPositionDrawdown()
			case <-at.stopMonitorCh:
				logger.Info("闂?Stopped position drawdown monitoring")
				return
			}
		}
	}()
}

func (at *AutoTrader) atrStopConfig() (bool, float64) {
	if at.config.StrategyConfig == nil {
		return false, 0
	}
	rc := at.config.StrategyConfig.RiskControl
	if !rc.ATRStopEnabled {
		return false, 0
	}
	multiplier := rc.ATRStopMultiplier
	if multiplier <= 0 {
		multiplier = 1.5
	}
	return true, multiplier
}

func resolveATR14FromMarketData(data *market.Data) float64 {
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

// checkPositionDrawdown checks position drawdown situation
func (at *AutoTrader) checkPositionDrawdown() {
	// Get current positions
	positions, err := at.trader.GetPositions()
	if err != nil {
		logger.Infof("闂?Drawdown monitoring: failed to get positions: %v", err)
		return
	}

	atrStopEnabled, atrStopMultiplier := at.atrStopConfig()
	atrDataCache := make(map[string]*market.Data)

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := strings.ToLower(pos["side"].(string))
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // Short position quantity is negative, convert to positive
		}
		if quantity <= 0 || entryPrice <= 0 || markPrice <= 0 {
			continue
		}

		// [CODE ENFORCED] ATR volatility stop-loss
		if atrStopEnabled {
			data, exists := atrDataCache[symbol]
			if !exists {
				fetched, fetchErr := market.GetWithExchange(symbol, at.exchange)
				if fetchErr != nil {
					logger.Infof("[ATR STOP] failed to fetch market data for %s: %v", symbol, fetchErr)
				}
				data = fetched
				atrDataCache[symbol] = data
			}

			atr := resolveATR14FromMarketData(data)
			if atr > 0 {
				stopDistance := atr * atrStopMultiplier
				stopPrice := 0.0
				triggered := false
				switch side {
				case "long":
					stopPrice = entryPrice - stopDistance
					triggered = markPrice <= stopPrice
				case "short":
					stopPrice = entryPrice + stopDistance
					triggered = markPrice >= stopPrice
				}
				if triggered {
					logger.Infof("[ATR STOP] trigger: %s %s | mark %.6f entry %.6f atr %.6f k %.2f stop %.6f",
						symbol, side, markPrice, entryPrice, atr, atrStopMultiplier, stopPrice)

					if err := at.emergencyClosePosition(symbol, side); err != nil {
						logger.Infof("[ATR STOP] close failed (%s %s): %v", symbol, side, err)
					} else {
						logger.Infof("[ATR STOP] close succeeded: %s %s", symbol, side)
						at.ClearPeakPnLCache(symbol, side)
					}
					continue
				}
			}
		}

		// Calculate current P&L percentage
		leverage := 10 // Default value
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		var currentPnLPct float64
		if side == "long" {
			currentPnLPct = ((markPrice - entryPrice) / entryPrice) * float64(leverage) * 100
		} else {
			currentPnLPct = ((entryPrice - markPrice) / entryPrice) * float64(leverage) * 100
		}

		// Construct unique position identifier (distinguish long/short)
		posKey := symbol + "_" + side

		// Get historical peak profit for this position
		at.peakPnLCacheMutex.RLock()
		peakPnLPct, exists := at.peakPnLCache[posKey]
		at.peakPnLCacheMutex.RUnlock()

		if !exists {
			// If no historical peak record, use current P&L as initial value
			peakPnLPct = currentPnLPct
			at.UpdatePeakPnL(symbol, side, currentPnLPct)
		} else {
			// Update peak cache
			at.UpdatePeakPnL(symbol, side, currentPnLPct)
		}

		// Calculate drawdown (magnitude of decline from peak)
		var drawdownPct float64
		if peakPnLPct > 0 && currentPnLPct < peakPnLPct {
			drawdownPct = ((peakPnLPct - currentPnLPct) / peakPnLPct) * 100
		}

		// Check close position condition: profit > 5% and drawdown >= 40%
		if currentPnLPct > 5.0 && drawdownPct >= 40.0 {
			logger.Infof("濠碘槅鍋撶徊浠嬪疮椤栫偞鍎?Drawdown close position condition triggered: %s %s | Current profit: %.2f%% | Peak profit: %.2f%% | Drawdown: %.2f%%",
				symbol, side, currentPnLPct, peakPnLPct, drawdownPct)

			// Execute close position
			if err := at.emergencyClosePosition(symbol, side); err != nil {
				logger.Infof("闂?Drawdown close position failed (%s %s): %v", symbol, side, err)
			} else {
				logger.Infof("闂?Drawdown close position succeeded: %s %s", symbol, side)
				// Clear cache for this position after closing
				at.ClearPeakPnLCache(symbol, side)
			}
		} else if currentPnLPct > 5.0 {
			// Record situations close to close position condition (for debugging)
			logger.Infof("濠碘槅鍋撶徊浠嬪疮椤栫偛绠?Drawdown monitoring: %s %s | Profit: %.2f%% | Peak: %.2f%% | Drawdown: %.2f%%",
				symbol, side, currentPnLPct, peakPnLPct, drawdownPct)
		}
	}
}

// emergencyClosePosition emergency close position function
func (at *AutoTrader) emergencyClosePosition(symbol, side string) error {
	switch side {
	case "long":
		order, err := at.trader.CloseLong(symbol, 0) // 0 = close all
		if err != nil {
			return err
		}
		logger.Infof("闂?Emergency close long position succeeded, order ID: %v", order["orderId"])
	case "short":
		order, err := at.trader.CloseShort(symbol, 0) // 0 = close all
		if err != nil {
			return err
		}
		logger.Infof("闂?Emergency close short position succeeded, order ID: %v", order["orderId"])
	default:
		return fmt.Errorf("unknown position direction: %s", side)
	}

	return nil
}

// GetPeakPnLCache gets peak profit cache
func (at *AutoTrader) GetPeakPnLCache() map[string]float64 {
	at.peakPnLCacheMutex.RLock()
	defer at.peakPnLCacheMutex.RUnlock()

	// Return a copy of the cache
	cache := make(map[string]float64)
	for k, v := range at.peakPnLCache {
		cache[k] = v
	}
	return cache
}

// UpdatePeakPnL updates peak profit cache
func (at *AutoTrader) UpdatePeakPnL(symbol, side string, currentPnLPct float64) {
	at.peakPnLCacheMutex.Lock()
	defer at.peakPnLCacheMutex.Unlock()

	posKey := symbol + "_" + side
	if peak, exists := at.peakPnLCache[posKey]; exists {
		// Update peak (if long, take larger value; if short, currentPnLPct is negative, also compare)
		if currentPnLPct > peak {
			at.peakPnLCache[posKey] = currentPnLPct
		}
	} else {
		// First time recording
		at.peakPnLCache[posKey] = currentPnLPct
	}
}

// ClearPeakPnLCache clears peak cache for specified position
func (at *AutoTrader) ClearPeakPnLCache(symbol, side string) {
	at.peakPnLCacheMutex.Lock()
	defer at.peakPnLCacheMutex.Unlock()

	posKey := symbol + "_" + side
	delete(at.peakPnLCache, posKey)
}

// recordAndConfirmOrder polls order status for actual fill data and records position
// action: open_long, open_short, close_long, close_short
// entryPrice: entry price when closing (0 when opening)
func (at *AutoTrader) recordAndConfirmOrder(orderResult map[string]interface{}, symbol, action string, quantity float64, price float64, leverage int, entryPrice float64) {
	if at.store == nil {
		return
	}

	// Get order ID (supports multiple types)
	var orderID string
	switch v := orderResult["orderId"].(type) {
	case int64:
		orderID = fmt.Sprintf("%d", v)
	case float64:
		orderID = fmt.Sprintf("%.0f", v)
	case string:
		orderID = v
	default:
		orderID = fmt.Sprintf("%v", v)
	}

	if orderID == "" || orderID == "0" {
		logger.Infof("  闂佸疇娉曟刊瀵哥箔?Order ID is empty, skipping record")
		return
	}

	// Determine positionSide
	var positionSide string
	switch action {
	case "open_long", "close_long":
		positionSide = "LONG"
	case "open_short", "close_short":
		positionSide = "SHORT"
	}

	var actualPrice = price
	var actualQty = quantity
	var fee float64

	// Exchanges with OrderSync: Skip immediate order recording, let OrderSync handle it
	// This ensures accurate data from GetTrades API and avoids duplicate records
	switch at.exchange {
	case "binance", "lighter", "hyperliquid", "bybit", "okx", "bitget", "aster", "kucoin", "gate":
		logger.Infof("  濡絽鍟幉?Order submitted (id: %s), will be synced by OrderSync", orderID)
		return
	}

	// For exchanges without OrderSync (e.g., Binance): record immediately and poll for fill data
	orderRecord := at.createOrderRecord(orderID, symbol, action, positionSide, quantity, price, leverage)
	if err := at.store.Order().CreateOrder(orderRecord); err != nil {
		logger.Infof("  闂佸疇娉曟刊瀵哥箔?Failed to record order: %v", err)
	} else {
		logger.Infof("  濡絽鍟幉?Order recorded: %s [%s] %s", orderID, action, symbol)
	}

	// Wait for order to be filled and get actual fill data
	time.Sleep(500 * time.Millisecond)
	for i := 0; i < 5; i++ {
		status, err := at.trader.GetOrderStatus(symbol, orderID)
		if err == nil {
			statusStr, _ := status["status"].(string)
			if statusStr == "FILLED" {
				// Get actual fill price
				if avgPrice, ok := status["avgPrice"].(float64); ok && avgPrice > 0 {
					actualPrice = avgPrice
				}
				// Get actual executed quantity
				if execQty, ok := status["executedQty"].(float64); ok && execQty > 0 {
					actualQty = execQty
				}
				// Get commission/fee
				if commission, ok := status["commission"].(float64); ok {
					fee = commission
				}
				logger.Infof("  闂?Order filled: avgPrice=%.6f, qty=%.6f, fee=%.6f", actualPrice, actualQty, fee)

				// Update order status to FILLED
				if err := at.store.Order().UpdateOrderStatus(orderRecord.ID, "FILLED", actualQty, actualPrice, fee); err != nil {
					logger.Infof("  闂佸疇娉曟刊瀵哥箔?Failed to update order status: %v", err)
				}

				// Record fill details
				at.recordOrderFill(orderRecord.ID, orderID, symbol, action, actualPrice, actualQty, fee)
				break
			} else if statusStr == "CANCELED" || statusStr == "EXPIRED" || statusStr == "REJECTED" {
				logger.Infof("  闂佸疇娉曟刊瀵哥箔?Order %s, skipping position record", statusStr)

				// Update order status
				if err := at.store.Order().UpdateOrderStatus(orderRecord.ID, statusStr, 0, 0, 0); err != nil {
					logger.Infof("  闂佸疇娉曟刊瀵哥箔?Failed to update order status: %v", err)
				}
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Normalize symbol for position record consistency
	normalizedSymbolForPosition := market.Normalize(symbol)

	logger.Infof("  濡絽鍟幉?Recording position (ID: %s, action: %s, price: %.6f, qty: %.6f, fee: %.4f)",
		orderID, action, actualPrice, actualQty, fee)

	// Record position change with actual fill data (use normalized symbol)
	at.recordPositionChange(orderID, normalizedSymbolForPosition, positionSide, action, actualQty, actualPrice, leverage, entryPrice, fee)

	// Send anonymous trade statistics for experience improvement (async, non-blocking)
	// This helps us understand overall product usage across all deployments
	experience.TrackTrade(experience.TradeEvent{
		Exchange:  at.exchange,
		TradeType: action,
		Symbol:    symbol,
		AmountUSD: actualPrice * actualQty,
		Leverage:  leverage,
		UserID:    at.userID,
		TraderID:  at.id,
	})
}

// recordPositionChange records position change (create record on open, update record on close)
func (at *AutoTrader) recordPositionChange(orderID, symbol, side, action string, quantity, price float64, leverage int, entryPrice float64, fee float64) {
	if at.store == nil {
		return
	}

	switch action {
	case "open_long", "open_short":
		// Open position: create new position record
		nowMs := time.Now().UTC().UnixMilli()
		pos := &store.TraderPosition{
			TraderID:     at.id,
			ExchangeID:   at.exchangeID, // Exchange account UUID
			ExchangeType: at.exchange,   // Exchange type: binance/bybit/okx/etc
			Symbol:       symbol,
			Side:         side, // LONG or SHORT
			Quantity:     quantity,
			EntryPrice:   price,
			EntryOrderID: orderID,
			EntryTime:    nowMs,
			Leverage:     leverage,
			Status:       "OPEN",
			CreatedAt:    nowMs,
			UpdatedAt:    nowMs,
		}
		if err := at.store.Position().Create(pos); err != nil {
			logger.Infof("  闂佸疇娉曟刊瀵哥箔?Failed to record position: %v", err)
		} else {
			logger.Infof("  濡絽鍟幆?Position recorded [%s] %s %s @ %.4f", at.id[:8], symbol, side, price)
		}

	case "close_long", "close_short":
		// Close position using PositionBuilder for consistent handling
		// PositionBuilder will handle both cases:
		// 1. If open position exists: close it properly
		// 2. If no open position (e.g., table cleared): create a closed position record
		posBuilder := store.NewPositionBuilder(at.store.Position())
		if err := posBuilder.ProcessTrade(
			at.id, at.exchangeID, at.exchange,
			symbol, side, action,
			quantity, price, fee, 0, // realizedPnL will be calculated
			time.Now().UTC().UnixMilli(), orderID,
		); err != nil {
			logger.Infof("  闂佸疇娉曟刊瀵哥箔?Failed to process close position: %v", err)
		} else {
			logger.Infof("  闂?Position closed [%s] %s %s @ %.4f", at.id[:8], symbol, side, price)
		}
	}
}

// createOrderRecord creates an order record struct from order details
func (at *AutoTrader) createOrderRecord(orderID, symbol, action, positionSide string, quantity, price float64, leverage int) *store.TraderOrder {
	// Determine order type (market for auto trader)
	orderType := "MARKET"

	// Determine side (BUY/SELL)
	var side string
	switch action {
	case "open_long", "close_short":
		side = "BUY"
	case "open_short", "close_long":
		side = "SELL"
	}

	// Use action as orderAction directly (keep lowercase format)
	orderAction := action

	// Determine if it's a reduce only order
	reduceOnly := (action == "close_long" || action == "close_short")

	// Normalize symbol for consistency
	normalizedSymbol := market.Normalize(symbol)

	return &store.TraderOrder{
		TraderID:        at.id,
		ExchangeID:      at.exchangeID,
		ExchangeType:    at.exchange,
		ExchangeOrderID: orderID,
		Symbol:          normalizedSymbol,
		Side:            side,
		PositionSide:    positionSide,
		Type:            orderType,
		TimeInForce:     "GTC",
		Quantity:        quantity,
		Price:           price,
		Status:          "NEW",
		FilledQuantity:  0,
		AvgFillPrice:    0,
		Commission:      0,
		CommissionAsset: "USDT",
		Leverage:        leverage,
		ReduceOnly:      reduceOnly,
		ClosePosition:   reduceOnly,
		OrderAction:     orderAction,
		CreatedAt:       time.Now().UTC().UnixMilli(),
		UpdatedAt:       time.Now().UTC().UnixMilli(),
	}
}

// recordOrderFill records order fill/trade details
func (at *AutoTrader) recordOrderFill(orderRecordID int64, exchangeOrderID, symbol, action string, price, quantity, fee float64) {
	if at.store == nil {
		return
	}

	// Determine side (BUY/SELL)
	var side string
	switch action {
	case "open_long", "close_short":
		side = "BUY"
	case "open_short", "close_long":
		side = "SELL"
	}

	// Generate a simple trade ID (exchange doesn't always provide one)
	tradeID := fmt.Sprintf("%s-%d", exchangeOrderID, time.Now().UnixNano())

	// Normalize symbol for consistency
	normalizedSymbol := market.Normalize(symbol)

	fill := &store.TraderFill{
		TraderID:        at.id,
		ExchangeID:      at.exchangeID,
		ExchangeType:    at.exchange,
		OrderID:         orderRecordID,
		ExchangeOrderID: exchangeOrderID,
		ExchangeTradeID: tradeID,
		Symbol:          normalizedSymbol,
		Side:            side,
		Price:           price,
		Quantity:        quantity,
		QuoteQuantity:   price * quantity,
		Commission:      fee,
		CommissionAsset: "USDT",
		RealizedPnL:     0,     // Will be calculated for close orders
		IsMaker:         false, // Market orders are usually taker
		CreatedAt:       time.Now().UTC().UnixMilli(),
	}

	// Calculate realized PnL for close orders
	if action == "close_long" || action == "close_short" {
		// Try to get the entry price from the open position
		var positionSide string
		if action == "close_long" {
			positionSide = "LONG"
		} else {
			positionSide = "SHORT"
		}

		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, symbol, positionSide); err == nil && openPos != nil {
			if positionSide == "LONG" {
				fill.RealizedPnL = (price - openPos.EntryPrice) * quantity
			} else {
				fill.RealizedPnL = (openPos.EntryPrice - price) * quantity
			}
		}
	}

	if err := at.store.Order().CreateFill(fill); err != nil {
		logger.Infof("  闂佸疇娉曟刊瀵哥箔?Failed to record fill: %v", err)
	} else {
		logger.Infof("  濡絽鍟幆?Fill recorded: %.4f @ %.6f, fee: %.4f", quantity, price, fee)
	}
}

// ============================================================================
// Risk Control Helpers
// ============================================================================

// isBTCETH checks if a symbol is BTC or ETH
func isBTCETH(symbol string) bool {
	symbol = strings.ToUpper(symbol)
	return strings.HasPrefix(symbol, "BTC") || strings.HasPrefix(symbol, "ETH")
}

func isOpenDecisionAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "open_long", "open_short":
		return true
	default:
		return false
	}
}

func (at *AutoTrader) minConfidenceThreshold() int {
	if at.config.StrategyConfig == nil {
		return 0
	}
	minConf := at.config.StrategyConfig.RiskControl.MinConfidence
	if minConf < 0 {
		return 0
	}
	if minConf > 100 {
		return 100
	}
	return minConf
}

// enforcePositionValueRatio checks and enforces position value ratio limits (CODE ENFORCED)
// Returns the adjusted position size (capped if necessary) and whether the position was capped
// positionSizeUSD: the original position size in USD
// equity: the account equity
// symbol: the trading symbol
func (at *AutoTrader) enforcePositionValueRatio(positionSizeUSD float64, equity float64, symbol string) (float64, bool) {
	if at.config.StrategyConfig == nil {
		return positionSizeUSD, false
	}

	riskControl := at.config.StrategyConfig.RiskControl

	// Get the appropriate position value ratio limit
	var maxPositionValueRatio float64
	if isBTCETH(symbol) {
		maxPositionValueRatio = riskControl.BTCETHMaxPositionValueRatio
		if maxPositionValueRatio <= 0 {
			maxPositionValueRatio = 5.0 // Default: 5x for BTC/ETH
		}
	} else {
		maxPositionValueRatio = riskControl.AltcoinMaxPositionValueRatio
		if maxPositionValueRatio <= 0 {
			maxPositionValueRatio = 1.0 // Default: 1x for altcoins
		}
	}

	// Calculate max allowed position value = equity 闁?ratio
	maxPositionValue := equity * maxPositionValueRatio

	// Check if position size exceeds limit
	if positionSizeUSD > maxPositionValue {
		logger.Infof("  闂佸疇娉曟刊瀵哥箔?[RISK CONTROL] Position %.2f USDT exceeds limit (equity %.2f 闁?%.1fx = %.2f USDT max for %s), capping",
			positionSizeUSD, equity, maxPositionValueRatio, maxPositionValue, symbol)
		return maxPositionValue, true
	}

	return positionSizeUSD, false
}

// enforceMinPositionSize checks minimum position size (CODE ENFORCED)
func (at *AutoTrader) enforceMinPositionSize(positionSizeUSD float64) error {
	if at.config.StrategyConfig == nil {
		return nil
	}

	minSize := at.config.StrategyConfig.RiskControl.MinPositionSize
	if minSize <= 0 {
		minSize = 12 // Default: 12 USDT
	}

	if positionSizeUSD < minSize {
		return fmt.Errorf("闂?[RISK CONTROL] Position %.2f USDT below minimum (%.2f USDT)", positionSizeUSD, minSize)
	}
	return nil
}

// enforceMaxPositions checks maximum positions count (CODE ENFORCED)
func (at *AutoTrader) enforceMaxPositions(currentPositionCount int) error {
	if at.config.StrategyConfig == nil {
		return nil
	}

	maxPositions := at.config.StrategyConfig.RiskControl.MaxPositions
	if maxPositions <= 0 {
		maxPositions = 3 // Default: 3 positions
	}

	if currentPositionCount >= maxPositions {
		return fmt.Errorf("闂?[RISK CONTROL] Already at max positions (%d/%d)", currentPositionCount, maxPositions)
	}
	return nil
}

const actionSkipPrefix = "SKIP: "

func markActionSkipped(actionRecord *store.DecisionAction, reason string) {
	if actionRecord == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "risk control skipped execution"
	}
	actionRecord.Error = actionSkipPrefix + reason
}

func actionSkipReason(actionRecord *store.DecisionAction) (string, bool) {
	if actionRecord == nil {
		return "", false
	}
	raw := strings.TrimSpace(actionRecord.Error)
	if !strings.HasPrefix(raw, actionSkipPrefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(raw, actionSkipPrefix)), true
}

func isActionSkipped(actionRecord *store.DecisionAction) bool {
	_, ok := actionSkipReason(actionRecord)
	return ok
}

func appendActionWarning(actionRecord *store.DecisionAction, warning string) {
	if actionRecord == nil {
		return
	}
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return
	}
	if actionRecord.Error == "" {
		actionRecord.Error = warning
		return
	}
	actionRecord.Error = actionRecord.Error + "; " + warning
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

func (at *AutoTrader) riskControlConfig() store.RiskControlConfig {
	if at.config.StrategyConfig != nil {
		return at.config.StrategyConfig.RiskControl
	}
	return store.GetDefaultStrategyConfig("en").RiskControl
}

func (at *AutoTrader) resolvePositionEntryPrice(symbol, side string, fallback float64) float64 {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fallback
	}
	targetSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	targetSide := strings.ToLower(strings.TrimSpace(side))
	for _, pos := range positions {
		sym, _ := pos["symbol"].(string)
		if strings.ToUpper(strings.TrimSpace(sym)) != targetSymbol {
			continue
		}
		posSide, _ := pos["side"].(string)
		if strings.ToLower(strings.TrimSpace(posSide)) != targetSide {
			continue
		}
		switch v := pos["entryPrice"].(type) {
		case float64:
			if v > 0 {
				return v
			}
		case int:
			if v > 0 {
				return float64(v)
			}
		case int64:
			if v > 0 {
				return float64(v)
			}
		}
	}
	return fallback
}

func (at *AutoTrader) applyPostFillRRGuard(decision *kernel.Decision, actionRecord *store.DecisionAction, side string, entry float64) error {
	riskControl := at.riskControlConfig()
	if !riskControl.EffectivePostFillRRRecheckEnabled() {
		return nil
	}

	minRR := effectiveMinRiskRewardRatio(riskControl.MinRiskRewardRatio)
	threshold := minRR - riskControl.EffectivePostFillRRTolerance()
	if threshold < 0 {
		threshold = 0
	}

	rr, riskPct, rewardPct, ok := store.CalculateRiskReward(decision.Action, entry, decision.StopLoss, decision.TakeProfit)
	if ok && rr >= threshold {
		return nil
	}

	failAction := riskControl.EffectivePostFillRROnFail()
	msg := fmt.Sprintf("post-fill RR recheck failed [%s %s]: rr=%.2f threshold=%.2f entry=%.6f stop=%.6f tp=%.6f risk=%.2f%% reward=%.2f%% action=%s",
		decision.Action, decision.Symbol, rr, threshold, entry, decision.StopLoss, decision.TakeProfit, riskPct, rewardPct, failAction)

	switch failAction {
	case store.PostFillRROnFailAdjustTP:
		newTP, adjusted := store.AdjustTakeProfitForMinRR(decision.Action, entry, decision.StopLoss, minRR)
		if !adjusted || newTP <= 0 {
			if err := at.emergencyClosePosition(decision.Symbol, side); err != nil {
				return fmt.Errorf("%s; adjust_tp failed and emergency close failed: %w", msg, err)
			}
			markActionSkipped(actionRecord, msg+"; adjust_tp failed, position closed")
			return nil
		}
		decision.TakeProfit = newTP
		actionRecord.TakeProfit = newTP
		logger.Infof("[RISK CONTROL] %s; adjusted take_profit to %.6f", msg, newTP)
		return nil
	case store.PostFillRROnFailCloseImmediately:
		if err := at.emergencyClosePosition(decision.Symbol, side); err != nil {
			return fmt.Errorf("%s; close_immediately failed: %w", msg, err)
		}
		markActionSkipped(actionRecord, msg+"; position closed immediately")
		return nil
	case store.PostFillRROnFailAlertOnly:
		logger.Warnf("[RISK CONTROL] %s; kept position due alert_only", msg)
		appendActionWarning(actionRecord, msg)
		return nil
	default:
		return nil
	}
}

func (at *AutoTrader) retryProtectionOrder(kind string, retryCount int, interval time.Duration, place func() error) error {
	attempts := retryCount + 1
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for i := 1; i <= attempts; i++ {
		if err := place(); err == nil {
			return nil
		} else {
			lastErr = err
			logger.Warnf("[RISK CONTROL] %s placement failed (%d/%d): %v", kind, i, attempts, err)
		}
		if i < attempts && interval > 0 {
			time.Sleep(interval)
		}
	}
	return fmt.Errorf("%s placement failed after %d attempts: %w", kind, attempts, lastErr)
}

func (at *AutoTrader) applyProtectionOrders(decision *kernel.Decision, actionRecord *store.DecisionAction, positionSide, side string, quantity float64) error {
	riskControl := at.riskControlConfig()
	retries := riskControl.EffectiveSLTPRetryCount()
	interval := time.Duration(riskControl.EffectiveSLTPRetryIntervalMs()) * time.Millisecond

	var slErr error
	if decision.StopLoss <= 0 {
		slErr = fmt.Errorf("invalid stop loss %.6f", decision.StopLoss)
	} else {
		slErr = at.retryProtectionOrder("stop_loss", retries, interval, func() error {
			return at.trader.SetStopLoss(decision.Symbol, positionSide, quantity, decision.StopLoss)
		})
	}
	if slErr != nil {
		onSLFail := riskControl.EffectiveOnSLFail()
		msg := fmt.Sprintf("stop_loss setup failed [%s %s]: %v (on_sl_fail=%s)", decision.Action, decision.Symbol, slErr, onSLFail)
		switch onSLFail {
		case store.SLTPFailActionCloseImmediately:
			if err := at.emergencyClosePosition(decision.Symbol, side); err != nil {
				return fmt.Errorf("%s; emergency close failed: %w", msg, err)
			}
			return fmt.Errorf("%s; position closed immediately", msg)
		case store.SLTPFailActionAlertOnly:
			logger.Warnf("[RISK CONTROL] %s", msg)
			appendActionWarning(actionRecord, msg)
		default:
			logger.Warnf("[RISK CONTROL] %s", msg)
			appendActionWarning(actionRecord, msg)
		}
	}

	var tpErr error
	if decision.TakeProfit <= 0 {
		tpErr = fmt.Errorf("invalid take profit %.6f", decision.TakeProfit)
	} else {
		tpErr = at.retryProtectionOrder("take_profit", retries, interval, func() error {
			return at.trader.SetTakeProfit(decision.Symbol, positionSide, quantity, decision.TakeProfit)
		})
	}
	if tpErr != nil {
		onTPFail := riskControl.EffectiveOnTPFail()
		msg := fmt.Sprintf("take_profit setup failed [%s %s]: %v (on_tp_fail=%s)", decision.Action, decision.Symbol, tpErr, onTPFail)
		switch onTPFail {
		case store.SLTPFailActionCloseImmediately:
			if err := at.emergencyClosePosition(decision.Symbol, side); err != nil {
				return fmt.Errorf("%s; emergency close failed: %w", msg, err)
			}
			return fmt.Errorf("%s; position closed immediately", msg)
		case store.SLTPFailActionAlertOnly:
			logger.Warnf("[RISK CONTROL] %s", msg)
			appendActionWarning(actionRecord, msg)
		case store.SLTPFailActionKeepWithSLRetry:
			logger.Warnf("[RISK CONTROL] %s; keep position with SL if available", msg)
			appendActionWarning(actionRecord, msg)
		default:
			logger.Warnf("[RISK CONTROL] %s", msg)
			appendActionWarning(actionRecord, msg)
		}
	}

	return nil
}

// getSideFromAction converts order action to side (BUY/SELL)
func getSideFromAction(action string) string {
	switch action {
	case "open_long", "close_short":
		return "BUY"
	case "open_short", "close_long":
		return "SELL"
	default:
		return "BUY"
	}
}

// GetOpenOrders returns open orders (pending SL/TP) from exchange
func (at *AutoTrader) GetOpenOrders(symbol string) ([]OpenOrder, error) {
	return at.trader.GetOpenOrders(symbol)
}
