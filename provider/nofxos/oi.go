package nofxos

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// OIPosition represents open interest data for a single coin.
type OIPosition struct {
	Symbol            string  `json:"symbol"`
	Rank              int     `json:"rank"`
	Price             float64 `json:"price"`
	CurrentOI         float64 `json:"current_oi"`
	OIDelta           float64 `json:"oi_delta"`
	OIDeltaPercent    float64 `json:"oi_delta_percent"`    // x100
	OIDeltaValue      float64 `json:"oi_delta_value"`      // USDT value
	PriceDeltaPercent float64 `json:"price_delta_percent"` // x100
	NetLong           float64 `json:"net_long"`
	NetShort          float64 `json:"net_short"`
}

// OIRankingData contains both top and low OI rankings.
type OIRankingData struct {
	TimeRange    string       `json:"time_range"`
	Duration     string       `json:"duration"`
	TopPositions []OIPosition `json:"top_positions"`
	LowPositions []OIPosition `json:"low_positions"`
	FetchedAt    time.Time    `json:"fetched_at"`
}

// GetOIRanking retrieves OI ranking data from public exchange endpoints.
func (c *Client) GetOIRanking(duration string, limit int) (*OIRankingData, error) {
	duration = normalizeRankingDuration(duration)
	if duration == "" {
		duration = "1h"
	}
	if limit <= 0 {
		limit = 20
	}

	tickers, err := c.fetchBinanceAllTickers24h()
	if err != nil {
		return nil, fmt.Errorf("fetch binance ticker universe failed: %w", err)
	}
	universe := buildRankingUniverseFromTickers(tickers, rankingUniverseSize(limit))
	period := durationToBinancePeriod(duration)

	result := &OIRankingData{
		Duration:  duration,
		TimeRange: duration,
		FetchedAt: time.Now(),
	}

	positions := make([]OIPosition, 0, len(universe))
	for _, snap := range universe {
		prev, curr, err := c.fetchBinanceOIHistPair(snap.Symbol, period)
		if err != nil {
			continue
		}

		delta := curr.oi - prev.oi
		deltaValue := curr.oiValue - prev.oiValue
		if deltaValue == 0 {
			deltaValue = delta * snap.Price
		}
		deltaPct := 0.0
		if prev.oi != 0 {
			deltaPct = (delta / prev.oi) * 100
		}

		positions = append(positions, OIPosition{
			Symbol:            snap.Symbol,
			Price:             snap.Price,
			CurrentOI:         curr.oi,
			OIDelta:           delta,
			OIDeltaPercent:    deltaPct,
			OIDeltaValue:      deltaValue,
			PriceDeltaPercent: snap.PriceDelta24h * 100,
		})
	}

	if len(positions) > 0 {
		top := append([]OIPosition(nil), positions...)
		low := append([]OIPosition(nil), positions...)

		sort.Slice(top, func(i, j int) bool { return top[i].OIDeltaValue > top[j].OIDeltaValue })
		sort.Slice(low, func(i, j int) bool { return low[i].OIDeltaValue < low[j].OIDeltaValue })

		result.TopPositions = trimAndRankOIPositions(top, limit)
		result.LowPositions = trimAndRankOIPositions(low, limit)
	}

	log.Printf("Fetched OI ranking data: %d top, %d low (duration: %s)",
		len(result.TopPositions), len(result.LowPositions), duration)

	return result, nil
}

// GetOITopPositions retrieves top OI increase positions.
func (c *Client) GetOITopPositions() ([]OIPosition, error) {
	data, err := c.GetOIRanking("1h", 20)
	if err != nil {
		return nil, err
	}
	return data.TopPositions, nil
}

// GetOITopSymbols retrieves OI top coin symbol list.
func (c *Client) GetOITopSymbols() ([]string, error) {
	positions, err := c.GetOITopPositions()
	if err != nil {
		return nil, err
	}

	symbols := make([]string, 0, len(positions))
	for _, pos := range positions {
		symbols = append(symbols, NormalizeSymbol(pos.Symbol))
	}
	return symbols, nil
}

// GetOILowPositions retrieves OI decrease positions.
func (c *Client) GetOILowPositions() ([]OIPosition, error) {
	data, err := c.GetOIRanking("1h", 20)
	if err != nil {
		return nil, err
	}
	return data.LowPositions, nil
}

// GetOILowSymbols retrieves OI low coin symbol list.
func (c *Client) GetOILowSymbols() ([]string, error) {
	positions, err := c.GetOILowPositions()
	if err != nil {
		return nil, err
	}

	symbols := make([]string, 0, len(positions))
	for _, pos := range positions {
		symbols = append(symbols, NormalizeSymbol(pos.Symbol))
	}
	return symbols, nil
}

// FormatOIRankingForAI formats OI ranking data for AI consumption.
func FormatOIRankingForAI(data *OIRankingData, lang Language) string {
	if data == nil {
		return ""
	}
	if lang == LangChinese {
		return formatOIRankingZH(data)
	}
	return formatOIRankingEN(data)
}

func formatOIRankingZH(data *OIRankingData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## 持仓量变化排行 (%s)\n\n", data.Duration))

	if len(data.TopPositions) > 0 {
		sb.WriteString("### 持仓增加榜\n")
		sb.WriteString("| 排名 | 币种 | OI变化(USDT) | OI变化% | 价格变化% |\n")
		sb.WriteString("|------|------|--------------|---------|-----------|\n")
		for _, pos := range data.TopPositions {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %+.2f%% | %+.2f%% |\n",
				pos.Rank, pos.Symbol, formatValue(pos.OIDeltaValue), pos.OIDeltaPercent, pos.PriceDeltaPercent))
		}
		sb.WriteString("\n")
	}

	if len(data.LowPositions) > 0 {
		sb.WriteString("### 持仓减少榜\n")
		sb.WriteString("| 排名 | 币种 | OI变化(USDT) | OI变化% | 价格变化% |\n")
		sb.WriteString("|------|------|--------------|---------|-----------|\n")
		for _, pos := range data.LowPositions {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %+.2f%% | %+.2f%% |\n",
				pos.Rank, pos.Symbol, formatValue(pos.OIDeltaValue), pos.OIDeltaPercent, pos.PriceDeltaPercent))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**解读**: OI增+价涨=多头主导 | OI增+价跌=空头主导 | OI减+价涨=空头平仓 | OI减+价跌=多头平仓\n\n")
	return sb.String()
}

func formatOIRankingEN(data *OIRankingData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Open Interest Changes (%s)\n\n", data.Duration))

	if len(data.TopPositions) > 0 {
		sb.WriteString("### OI Increase Ranking\n")
		sb.WriteString("| Rank | Symbol | OI Change (USDT) | OI Change % | Price Change % |\n")
		sb.WriteString("|------|--------|------------------|-------------|----------------|\n")
		for _, pos := range data.TopPositions {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %+.2f%% | %+.2f%% |\n",
				pos.Rank, pos.Symbol, formatValue(pos.OIDeltaValue), pos.OIDeltaPercent, pos.PriceDeltaPercent))
		}
		sb.WriteString("\n")
	}

	if len(data.LowPositions) > 0 {
		sb.WriteString("### OI Decrease Ranking\n")
		sb.WriteString("| Rank | Symbol | OI Change (USDT) | OI Change % | Price Change % |\n")
		sb.WriteString("|------|--------|------------------|-------------|----------------|\n")
		for _, pos := range data.LowPositions {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %+.2f%% | %+.2f%% |\n",
				pos.Rank, pos.Symbol, formatValue(pos.OIDeltaValue), pos.OIDeltaPercent, pos.PriceDeltaPercent))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Key**: OI up + Price up = Bulls dominant | OI up + Price down = Bears dominant | OI down + Price up = Short covering | OI down + Price down = Long liquidation\n\n")
	return sb.String()
}

func trimAndRankOIPositions(items []OIPosition, limit int) []OIPosition {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]OIPosition, len(items))
	copy(out, items)
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}
