package nofxos

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// NetFlowPosition represents fund flow data for a single coin.
type NetFlowPosition struct {
	Rank   int     `json:"rank"`
	Symbol string  `json:"symbol"`
	Amount float64 `json:"amount"` // positive=inflow, negative=outflow
	Price  float64 `json:"price"`
}

// NetFlowRankingData contains institution and personal fund flow rankings.
type NetFlowRankingData struct {
	Duration             string            `json:"duration"`
	TimeRange            string            `json:"time_range"`
	InstitutionFutureTop []NetFlowPosition `json:"institution_future_top"`
	InstitutionFutureLow []NetFlowPosition `json:"institution_future_low"`
	PersonalFutureTop    []NetFlowPosition `json:"personal_future_top"`
	PersonalFutureLow    []NetFlowPosition `json:"personal_future_low"`
	FetchedAt            time.Time         `json:"fetched_at"`
}

// GetNetFlowRanking retrieves NetFlow ranking data from public exchange endpoints.
// Note: this uses a proxy metric from taker buy/sell imbalance.
func (c *Client) GetNetFlowRanking(duration string, limit int) (*NetFlowRankingData, error) {
	duration = normalizeRankingDuration(duration)
	if duration == "" {
		duration = "1h"
	}
	if limit <= 0 {
		limit = 10
	}

	tickers, err := c.fetchBinanceAllTickers24h()
	if err != nil {
		return nil, fmt.Errorf("fetch binance ticker universe failed: %w", err)
	}
	universe := buildRankingUniverseFromTickers(tickers, rankingUniverseSize(limit))
	period := durationToBinancePeriod(duration)

	result := &NetFlowRankingData{
		Duration:  duration,
		TimeRange: duration,
		FetchedAt: time.Now(),
	}

	positions := make([]NetFlowPosition, 0, len(universe))
	for _, snap := range universe {
		buy, sell, err := c.fetchBinanceTakerFlow(snap.Symbol, period)
		if err != nil {
			continue
		}
		positions = append(positions, NetFlowPosition{
			Symbol: snap.Symbol,
			Amount: buy - sell,
			Price:  snap.Price,
		})
	}

	if len(positions) > 0 {
		top := append([]NetFlowPosition(nil), positions...)
		low := append([]NetFlowPosition(nil), positions...)

		sort.Slice(top, func(i, j int) bool { return top[i].Amount > top[j].Amount })
		sort.Slice(low, func(i, j int) bool { return low[i].Amount < low[j].Amount })

		result.InstitutionFutureTop = trimAndRankNetFlowPositions(top, limit)
		result.InstitutionFutureLow = trimAndRankNetFlowPositions(low, limit)
	}

	log.Printf("Fetched NetFlow ranking data: inst_in=%d, inst_out=%d, retail_in=%d, retail_out=%d (duration: %s)",
		len(result.InstitutionFutureTop), len(result.InstitutionFutureLow),
		len(result.PersonalFutureTop), len(result.PersonalFutureLow), duration)

	return result, nil
}

// FormatNetFlowRankingForAI formats NetFlow ranking data for AI consumption.
func FormatNetFlowRankingForAI(data *NetFlowRankingData, lang Language) string {
	if data == nil {
		return ""
	}

	if lang == LangChinese {
		return formatNetFlowRankingZH(data)
	}
	return formatNetFlowRankingEN(data)
}

func formatNetFlowRankingZH(data *NetFlowRankingData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## 资金流向排行 (%s)\n\n", data.Duration))

	if len(data.InstitutionFutureTop) > 0 {
		sb.WriteString("### 机构资金流入榜 (代理指标)\n")
		sb.WriteString("| 排名 | 币种 | 资金差额(USDT) | 价格 |\n")
		sb.WriteString("|------|------|----------------|------|\n")
		for _, pos := range data.InstitutionFutureTop {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | $%.4f |\n", pos.Rank, pos.Symbol, formatValue(pos.Amount), pos.Price))
		}
		sb.WriteString("\n")
	}

	if len(data.InstitutionFutureLow) > 0 {
		sb.WriteString("### 机构资金流出榜 (代理指标)\n")
		sb.WriteString("| 排名 | 币种 | 资金差额(USDT) | 价格 |\n")
		sb.WriteString("|------|------|----------------|------|\n")
		for _, pos := range data.InstitutionFutureLow {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | $%.4f |\n", pos.Rank, pos.Symbol, formatValue(pos.Amount), pos.Price))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**解读**: 基于 taker 买卖差的代理指标，不等同于真实机构席位数据。\n\n")
	return sb.String()
}

func formatNetFlowRankingEN(data *NetFlowRankingData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Fund Flow Ranking (%s)\n\n", data.Duration))

	if len(data.InstitutionFutureTop) > 0 {
		sb.WriteString("### Institution Inflow (proxy)\n")
		sb.WriteString("| Rank | Symbol | Flow Delta (USDT) | Price |\n")
		sb.WriteString("|------|--------|-------------------|-------|\n")
		for _, pos := range data.InstitutionFutureTop {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | $%.4f |\n", pos.Rank, pos.Symbol, formatValue(pos.Amount), pos.Price))
		}
		sb.WriteString("\n")
	}

	if len(data.InstitutionFutureLow) > 0 {
		sb.WriteString("### Institution Outflow (proxy)\n")
		sb.WriteString("| Rank | Symbol | Flow Delta (USDT) | Price |\n")
		sb.WriteString("|------|--------|-------------------|-------|\n")
		for _, pos := range data.InstitutionFutureLow {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | $%.4f |\n", pos.Rank, pos.Symbol, formatValue(pos.Amount), pos.Price))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Key**: Proxy metric from taker buy/sell imbalance, not true institution account breakdown.\n\n")
	return sb.String()
}

func trimAndRankNetFlowPositions(items []NetFlowPosition, limit int) []NetFlowPosition {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]NetFlowPosition, len(items))
	copy(out, items)
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}
