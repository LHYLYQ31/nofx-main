package backtest

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"nofx/market"
	"nofx/store"
)

type timeframeSeries struct {
	klines     []market.Kline
	closeTimes []int64
}

type symbolSeries struct {
	byTF map[string]*timeframeSeries
}

// DataFeed manages historical kline data and provides time-progressive snapshots for backtesting.
type DataFeed struct {
	cfg           BacktestConfig
	symbols       []string
	timeframes    []string
	symbolSeries  map[string]*symbolSeries
	decisionTimes []int64
	primaryTF     string
	longerTF      string
	tfWindows     map[string]int
}

const (
	defaultPrimaryWindow = 60
	defaultLongerWindow  = 30
)

func NewDataFeed(cfg BacktestConfig, strategyCfg *store.StrategyConfig) (*DataFeed, error) {
	df := &DataFeed{
		cfg:          cfg,
		symbols:      make([]string, len(cfg.Symbols)),
		timeframes:   append([]string(nil), cfg.Timeframes...),
		symbolSeries: make(map[string]*symbolSeries),
		primaryTF:    cfg.DecisionTimeframe,
		tfWindows:    buildTimeframeWindows(cfg, strategyCfg),
	}
	copy(df.symbols, cfg.Symbols)

	if err := df.loadAll(); err != nil {
		return nil, err
	}

	return df, nil
}

func (df *DataFeed) loadAll() error {
	start := time.Unix(df.cfg.StartTS, 0)
	end := time.Unix(df.cfg.EndTS, 0)

	// longest timeframe used for auxiliary indicators
	var longestDur time.Duration
	for _, tf := range df.timeframes {
		dur, err := market.TFDuration(tf)
		if err != nil {
			return err
		}
		if dur > longestDur {
			longestDur = dur
			df.longerTF = tf
		}
	}

	for _, symbol := range df.symbols {
		ss := &symbolSeries{byTF: make(map[string]*timeframeSeries)}
		for _, tf := range df.timeframes {
			dur, _ := market.TFDuration(tf)
			// Preload enough bars so fixed-window slicing keeps indicator warmup stable.
			bufferBars := df.windowSize(tf)
			if bufferBars < 200 {
				bufferBars = 200
			}
			buffer := dur * time.Duration(bufferBars)
			fetchStart := start.Add(-buffer)
			if fetchStart.Before(time.Unix(0, 0)) {
				fetchStart = time.Unix(0, 0)
			}
			fetchEnd := end.Add(dur)

			klines, err := market.GetKlinesRange(symbol, tf, fetchStart, fetchEnd)
			if err != nil {
				return fmt.Errorf("fetch klines for %s %s: %w", symbol, tf, err)
			}
			if len(klines) == 0 {
				return fmt.Errorf("no klines for %s %s", symbol, tf)
			}

			series := &timeframeSeries{
				klines:     klines,
				closeTimes: make([]int64, len(klines)),
			}
			for i, k := range klines {
				series.closeTimes[i] = k.CloseTime
			}
			ss.byTF[tf] = series
		}
		df.symbolSeries[symbol] = ss
	}

	// Generate backtest progress timeline using the primary timeframe of the first symbol
	firstSymbol := df.symbols[0]
	primarySeries := df.symbolSeries[firstSymbol].byTF[df.primaryTF]
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()
	for _, ts := range primarySeries.closeTimes {
		if ts < startMs {
			continue
		}
		if ts > endMs {
			break
		}
		df.decisionTimes = append(df.decisionTimes, ts)
		// Align other symbols; report error early if data is missing
		for _, symbol := range df.symbols[1:] {
			if _, ok := df.symbolSeries[symbol].byTF[df.primaryTF]; !ok {
				return fmt.Errorf("symbol %s missing timeframe %s", symbol, df.primaryTF)
			}
		}
	}
	if len(df.decisionTimes) == 0 {
		return fmt.Errorf("no decision bars in range")
	}
	return nil
}

func (df *DataFeed) DecisionBarCount() int {
	return len(df.decisionTimes)
}

func (df *DataFeed) DecisionTimestamp(index int) int64 {
	// Bounds check to prevent panic
	if index < 0 || index >= len(df.decisionTimes) {
		return 0
	}
	return df.decisionTimes[index]
}

func (df *DataFeed) sliceUpTo(symbol, tf string, ts int64) []market.Kline {
	// Nil checks to prevent panic
	ss, ok := df.symbolSeries[symbol]
	if !ok || ss == nil {
		return nil
	}
	series, ok := ss.byTF[tf]
	if !ok || series == nil {
		return nil
	}
	idx := sort.Search(len(series.closeTimes), func(i int) bool {
		return series.closeTimes[i] > ts
	})
	if idx <= 0 {
		return nil
	}
	window := df.windowSize(tf)
	if window > 0 && idx > window {
		return series.klines[idx-window : idx]
	}
	return series.klines[:idx]
}

func (df *DataFeed) BuildMarketData(ts int64) (map[string]*market.Data, map[string]map[string]*market.Data, error) {
	result := make(map[string]*market.Data, len(df.symbols))
	multi := make(map[string]map[string]*market.Data, len(df.symbols))

	for _, symbol := range df.symbols {
		perTF := make(map[string]*market.Data, len(df.timeframes))
		for _, tf := range df.timeframes {
			series := df.sliceUpTo(symbol, tf, ts)
			if len(series) == 0 {
				continue
			}
			var longer []market.Kline
			if df.longerTF != "" && df.longerTF != tf {
				longer = df.sliceUpTo(symbol, df.longerTF, ts)
			}
			data, err := market.BuildDataFromKlines(symbol, series, longer)
			if err != nil {
				return nil, nil, err
			}
			perTF[tf] = data
			if tf == df.primaryTF {
				result[symbol] = data
			}
		}
		if _, ok := perTF[df.primaryTF]; !ok {
			return nil, nil, fmt.Errorf("no primary data for %s at %d", symbol, ts)
		}
		// Backtest-local multi-timeframe wiring:
		// Build explicit TimeframeData so downstream grid prompt can select indicators
		// by decision timeframe / selected timeframe instead of falling back to zeros.
		primaryData := result[symbol]
		if primaryData != nil {
			primaryData.TimeframeData = make(map[string]*market.TimeframeSeriesData, len(df.timeframes))
			for _, tf := range df.timeframes {
				series := df.sliceUpTo(symbol, tf, ts)
				if len(series) == 0 {
					continue
				}
				primaryData.TimeframeData[tf] = market.BuildTimeframeSeriesFromKlines(series, tf, len(series))
			}
		}
		multi[symbol] = perTF
	}
	return result, multi, nil
}

func (df *DataFeed) decisionBarSnapshot(symbol string, ts int64) (*market.Kline, *market.Kline) {
	ss, ok := df.symbolSeries[symbol]
	if !ok {
		return nil, nil
	}
	series, ok := ss.byTF[df.primaryTF]
	if !ok {
		return nil, nil
	}
	idx := sort.Search(len(series.closeTimes), func(i int) bool {
		return series.closeTimes[i] >= ts
	})
	if idx >= len(series.closeTimes) || series.closeTimes[idx] != ts {
		return nil, nil
	}
	curr := &series.klines[idx]
	var next *market.Kline
	if idx+1 < len(series.klines) {
		next = &series.klines[idx+1]
	}
	return curr, next
}

func buildTimeframeWindows(cfg BacktestConfig, strategyCfg *store.StrategyConfig) map[string]int {
	primaryTF := cfg.DecisionTimeframe
	primaryWindow := defaultPrimaryWindow
	longerWindow := defaultLongerWindow

	if strategyCfg != nil {
		kline := strategyCfg.Indicators.Klines
		if normalized, err := market.NormalizeTimeframe(kline.PrimaryTimeframe); err == nil && normalized != "" {
			primaryTF = normalized
		}
		if kline.PrimaryCount > 0 {
			primaryWindow = kline.PrimaryCount
		}
		if kline.LongerCount > 0 {
			longerWindow = kline.LongerCount
		}
	}

	windows := make(map[string]int, len(cfg.Timeframes)+1)
	for _, tf := range cfg.Timeframes {
		norm := normalizeTimeframeOrFallback(tf)
		if norm == primaryTF {
			windows[norm] = primaryWindow
			continue
		}
		windows[norm] = longerWindow
	}
	if _, ok := windows[primaryTF]; !ok {
		windows[primaryTF] = primaryWindow
	}
	return windows
}

func (df *DataFeed) windowSize(tf string) int {
	if df == nil || len(df.tfWindows) == 0 {
		return defaultLongerWindow
	}
	key := normalizeTimeframeOrFallback(tf)
	if v, ok := df.tfWindows[key]; ok && v > 0 {
		return v
	}
	if v, ok := df.tfWindows[df.primaryTF]; ok && v > 0 {
		return v
	}
	return defaultLongerWindow
}

func normalizeTimeframeOrFallback(tf string) string {
	if normalized, err := market.NormalizeTimeframe(strings.TrimSpace(tf)); err == nil && normalized != "" {
		return normalized
	}
	return strings.TrimSpace(tf)
}
