package nofxos

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// PriceRankingItem represents single coin price ranking data.
type PriceRankingItem struct {
	Pair         string  `json:"pair"`
	Symbol       string  `json:"symbol"`
	PriceDelta   float64 `json:"price_delta"` // decimal: 0.0723 = 7.23%
	Price        float64 `json:"price"`
	FutureFlow   float64 `json:"future_flow"`
	SpotFlow     float64 `json:"spot_flow"`
	OI           float64 `json:"oi"`
	OIDelta      float64 `json:"oi_delta"`
	OIDeltaValue float64 `json:"oi_delta_value"`
}

// PriceRankingDuration contains top gainers and losers for a single duration.
type PriceRankingDuration struct {
	Top []PriceRankingItem `json:"top"`
	Low []PriceRankingItem `json:"low"`
}

// PriceRankingData contains price ranking data for multiple durations.
type PriceRankingData struct {
	Durations map[string]*PriceRankingDuration `json:"durations"`
	FetchedAt time.Time                        `json:"fetched_at"`
}

// GetPriceRanking retrieves price ranking data from public exchange endpoints.
func (c *Client) GetPriceRanking(durations string, limit int) (*PriceRankingData, error) {
	if limit <= 0 {
		limit = 10
	}
	durationList := parseRankingDurations(durations)

	tickers, err := c.fetchBinanceAllTickers24h()
	if err != nil {
		return nil, fmt.Errorf("fetch binance ticker universe failed: %w", err)
	}
	universe := buildRankingUniverseFromTickers(tickers, rankingUniverseSize(limit))

	result := &PriceRankingData{
		Durations: make(map[string]*PriceRankingDuration),
		FetchedAt: time.Now(),
	}

	for _, duration := range durationList {
		items := make([]PriceRankingItem, 0, len(universe))

		if duration == "24h" {
			for _, snap := range universe {
				items = append(items, PriceRankingItem{
					Pair:       snap.Symbol,
					Symbol:     snap.Symbol,
					PriceDelta: snap.PriceDelta24h,
					Price:      snap.Price,
				})
			}
		} else {
			interval := durationToKlineInterval(duration)
			for _, snap := range universe {
				delta, lastPrice, err := c.fetchBinanceKlineDelta(snap.Symbol, interval)
				if err != nil {
					continue
				}
				items = append(items, PriceRankingItem{
					Pair:       snap.Symbol,
					Symbol:     snap.Symbol,
					PriceDelta: delta,
					Price:      lastPrice,
				})
			}
		}

		if len(items) == 0 {
			continue
		}

		top := append([]PriceRankingItem(nil), items...)
		low := append([]PriceRankingItem(nil), items...)
		sort.Slice(top, func(i, j int) bool { return top[i].PriceDelta > top[j].PriceDelta })
		sort.Slice(low, func(i, j int) bool { return low[i].PriceDelta < low[j].PriceDelta })

		result.Durations[duration] = &PriceRankingDuration{
			Top: trimPriceRankingItems(top, limit),
			Low: trimPriceRankingItems(low, limit),
		}
	}

	if len(result.Durations) == 0 {
		return nil, fmt.Errorf("no price ranking data from public providers")
	}

	log.Printf("Fetched Price ranking data for %d durations", len(result.Durations))
	return result, nil
}

// FormatPriceRankingForAI formats Price ranking data for AI consumption.
func FormatPriceRankingForAI(data *PriceRankingData, lang Language) string {
	if data == nil || len(data.Durations) == 0 {
		return ""
	}

	if lang == LangChinese {
		return formatPriceRankingZH(data)
	}
	return formatPriceRankingEN(data)
}

func formatPriceRankingZH(data *PriceRankingData) string {
	var sb strings.Builder

	sb.WriteString("## 涨跌幅排行\n\n")

	durationOrder := []string{"1h", "4h", "24h"}
	for _, duration := range durationOrder {
		durationData, exists := data.Durations[duration]
		if !exists || durationData == nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s 涨跌幅\n\n", duration))

		if len(durationData.Top) > 0 {
			sb.WriteString("**涨幅榜**\n")
			sb.WriteString("| 币种 | 涨幅 | 价格 |\n")
			sb.WriteString("|------|------|------|\n")
			for _, item := range durationData.Top {
				sb.WriteString(fmt.Sprintf("| %s | %+.2f%% | $%.4f |\n", item.Symbol, item.PriceDelta*100, item.Price))
			}
			sb.WriteString("\n")
		}

		if len(durationData.Low) > 0 {
			sb.WriteString("**跌幅榜**\n")
			sb.WriteString("| 币种 | 跌幅 | 价格 |\n")
			sb.WriteString("|------|------|------|\n")
			for _, item := range durationData.Low {
				sb.WriteString(fmt.Sprintf("| %s | %.2f%% | $%.4f |\n", item.Symbol, item.PriceDelta*100, item.Price))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("**说明**: 1h/4h 使用公开K线计算，24h 使用交易所24h统计。\n\n")
	return sb.String()
}

func formatPriceRankingEN(data *PriceRankingData) string {
	var sb strings.Builder

	sb.WriteString("## Price Gainers/Losers\n\n")

	durationOrder := []string{"1h", "4h", "24h"}
	for _, duration := range durationOrder {
		durationData, exists := data.Durations[duration]
		if !exists || durationData == nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s Price Change\n\n", duration))

		if len(durationData.Top) > 0 {
			sb.WriteString("**Top Gainers**\n")
			sb.WriteString("| Symbol | Change | Price |\n")
			sb.WriteString("|--------|--------|-------|\n")
			for _, item := range durationData.Top {
				sb.WriteString(fmt.Sprintf("| %s | %+.2f%% | $%.4f |\n", item.Symbol, item.PriceDelta*100, item.Price))
			}
			sb.WriteString("\n")
		}

		if len(durationData.Low) > 0 {
			sb.WriteString("**Top Losers**\n")
			sb.WriteString("| Symbol | Change | Price |\n")
			sb.WriteString("|--------|--------|-------|\n")
			for _, item := range durationData.Low {
				sb.WriteString(fmt.Sprintf("| %s | %.2f%% | $%.4f |\n", item.Symbol, item.PriceDelta*100, item.Price))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("**Note**: 1h/4h are computed from public klines; 24h uses exchange 24h ticker statistics.\n\n")
	return sb.String()
}

func trimPriceRankingItems(items []PriceRankingItem, limit int) []PriceRankingItem {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]PriceRankingItem, len(items))
	copy(out, items)
	return out
}
