package nofxos

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type binanceTicker24hItem struct {
	Symbol             string `json:"symbol"`
	LastPrice          string `json:"lastPrice"`
	PriceChangePercent string `json:"priceChangePercent"`
	QuoteVolume        string `json:"quoteVolume"`
}

type publicSymbolSnapshot struct {
	Symbol        string
	Price         float64
	PriceDelta24h float64 // decimal (0.01 = 1%)
	QuoteVolume   float64
}

var rankingSymbolPattern = regexp.MustCompile(`^[A-Z0-9]{2,20}USDT$`)

func normalizeRankingDuration(duration string) string {
	switch strings.ToLower(strings.TrimSpace(duration)) {
	case "1h":
		return "1h"
	case "4h":
		return "4h"
	case "24h", "1d":
		return "24h"
	default:
		return ""
	}
}

func parseRankingDurations(raw string) []string {
	seen := make(map[string]bool)
	ordered := make([]string, 0, 3)
	for _, part := range strings.Split(raw, ",") {
		d := normalizeRankingDuration(part)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		ordered = append(ordered, d)
	}
	if len(ordered) == 0 {
		return []string{"1h"}
	}
	return ordered
}

func durationToBinancePeriod(duration string) string {
	switch normalizeRankingDuration(duration) {
	case "1h":
		return "1h"
	case "4h":
		return "4h"
	case "24h":
		return "1d"
	default:
		return "1h"
	}
}

func durationToKlineInterval(duration string) string {
	switch normalizeRankingDuration(duration) {
	case "1h":
		return "1h"
	case "4h":
		return "4h"
	case "24h":
		return "1d"
	default:
		return "1h"
	}
}

func rankingUniverseSize(limit int) int {
	if limit <= 0 {
		limit = 10
	}
	size := limit + 6
	if size < 12 {
		size = 12
	}
	if size > 24 {
		size = 24
	}
	return size
}

func (c *Client) fetchBinanceAllTickers24h() ([]binanceTicker24hItem, error) {
	body, err := c.doPublicRequest(BinanceBaseURL, "/fapi/v1/ticker/24hr", nil)
	if err != nil {
		return nil, err
	}

	var items []binanceTicker24hItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parse binance all tickers 24h: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("empty binance ticker24h list")
	}
	return items, nil
}

func buildRankingUniverseFromTickers(tickers []binanceTicker24hItem, universeSize int) []publicSymbolSnapshot {
	snapshots := make([]publicSymbolSnapshot, 0, len(tickers))
	for _, item := range tickers {
		symbol := NormalizeSymbol(item.Symbol)
		if !strings.HasSuffix(symbol, "USDT") {
			continue
		}
		// Filter malformed/unexpected symbols to avoid polluting prompt rankings.
		if !rankingSymbolPattern.MatchString(symbol) {
			continue
		}

		price, err := parseStringFloat(item.LastPrice)
		if err != nil || price <= 0 {
			continue
		}

		quoteVolume := 0.0
		if qv, err := parseStringFloat(item.QuoteVolume); err == nil {
			quoteVolume = qv
		}

		priceDelta24h := 0.0
		if pct, err := parseStringFloat(item.PriceChangePercent); err == nil {
			priceDelta24h = pct / 100
		}

		snapshots = append(snapshots, publicSymbolSnapshot{
			Symbol:        symbol,
			Price:         price,
			PriceDelta24h: priceDelta24h,
			QuoteVolume:   quoteVolume,
		})
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].QuoteVolume > snapshots[j].QuoteVolume
	})

	if universeSize > 0 && len(snapshots) > universeSize {
		snapshots = snapshots[:universeSize]
	}
	return snapshots
}

func (c *Client) fetchBinanceKlineDelta(symbol, interval string) (float64, float64, error) {
	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("interval", interval)
	query.Set("limit", "2")

	body, err := c.doPublicRequest(BinanceBaseURL, "/fapi/v1/klines", query)
	if err != nil {
		return 0, 0, err
	}

	var rows [][]interface{}
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, 0, fmt.Errorf("parse binance klines: %w", err)
	}
	if len(rows) < 2 {
		return 0, 0, fmt.Errorf("insufficient kline rows")
	}

	prevClose, err := parseKlineNumeric(rows[len(rows)-2], 4)
	if err != nil {
		return 0, 0, err
	}
	lastClose, err := parseKlineNumeric(rows[len(rows)-1], 4)
	if err != nil {
		return 0, 0, err
	}
	if prevClose == 0 {
		return 0, 0, fmt.Errorf("invalid previous close 0")
	}
	return (lastClose - prevClose) / prevClose, lastClose, nil
}

func parseKlineNumeric(row []interface{}, index int) (float64, error) {
	if len(row) <= index {
		return 0, fmt.Errorf("kline index out of range")
	}

	switch v := row[index].(type) {
	case string:
		return parseStringFloat(v)
	case float64:
		return v, nil
	case json.Number:
		return v.Float64()
	default:
		return parseStringFloat(fmt.Sprintf("%v", v))
	}
}
