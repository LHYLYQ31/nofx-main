package nofxos

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// QuantData represents quantitative data for a single coin
type QuantData struct {
	Symbol      string             `json:"symbol"`
	Price       float64            `json:"price"`
	Netflow     *NetflowData       `json:"netflow,omitempty"`
	OI          map[string]*OIData `json:"oi,omitempty"`           // keyed by exchange: "binance", "bybit"
	PriceChange map[string]float64 `json:"price_change,omitempty"` // keyed by duration: "1h", "4h", etc.
}

// NetflowData contains fund flow data
type NetflowData struct {
	Institution *FlowTypeData `json:"institution,omitempty"`
	Personal    *FlowTypeData `json:"personal,omitempty"`
}

// FlowTypeData contains flow data by trade type
type FlowTypeData struct {
	Future map[string]float64 `json:"future,omitempty"` // keyed by duration
	Spot   map[string]float64 `json:"spot,omitempty"`   // keyed by duration
}

// OIData contains open interest data for an exchange
type OIData struct {
	CurrentOI float64                 `json:"current_oi"`
	NetLong   float64                 `json:"net_long"`
	NetShort  float64                 `json:"net_short"`
	Delta     map[string]*OIDeltaData `json:"delta,omitempty"` // keyed by duration
}

// OIDeltaData contains OI change data
type OIDeltaData struct {
	OIDelta        float64 `json:"oi_delta"`
	OIDeltaValue   float64 `json:"oi_delta_value"`
	OIDeltaPercent float64 `json:"oi_delta_percent"` // x100
}

type quantIncludeFlags struct {
	price   bool
	oi      bool
	netflow bool
}

// CoinResponse is the legacy NofxOS API response structure.
type CoinResponse struct {
	Success bool       `json:"success"`
	Code    int        `json:"code"`
	Data    *QuantData `json:"data"`
}

type binanceTicker24hResponse struct {
	LastPrice          string `json:"lastPrice"`
	PriceChangePercent string `json:"priceChangePercent"`
}

type binanceOpenInterestResponse struct {
	OpenInterest string `json:"openInterest"`
}

type binanceOpenInterestHistItem struct {
	SumOpenInterest      string `json:"sumOpenInterest"`
	SumOpenInterestValue string `json:"sumOpenInterestValue"`
	Timestamp            int64  `json:"timestamp"`
}

type binanceTakerRatioItem struct {
	BuyVol    string `json:"buyVol"`
	SellVol   string `json:"sellVol"`
	Timestamp int64  `json:"timestamp"`
}

type bybitTickerResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List []struct {
			LastPrice    string `json:"lastPrice"`
			Price24hPcnt string `json:"price24hPcnt"`
			OpenInterest string `json:"openInterest"`
		} `json:"list"`
	} `json:"result"`
}

type bybitOpenInterestResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List []struct {
			OpenInterest string `json:"openInterest"`
			Timestamp    string `json:"timestamp"`
		} `json:"list"`
	} `json:"result"`
}

var (
	binanceOIPeriodByDuration = map[string]string{
		"1h":  "1h",
		"4h":  "4h",
		"24h": "1d",
	}
	binanceTakerPeriodByDuration = map[string]string{
		"1h":  "1h",
		"4h":  "4h",
		"24h": "1d",
	}
	bybitOIPeriodByDuration = map[string]string{
		"1h":  "1h",
		"4h":  "4h",
		"24h": "1d",
	}
)

// GetCoinData retrieves quantitative data for a single coin from public exchange APIs.
func (c *Client) GetCoinData(symbol string, include string) (*QuantData, error) {
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	flags := parseQuantIncludeFlags(include)
	normalized := NormalizeSymbol(symbol)

	var errs []error

	if data, err := c.getCoinDataFromBinance(normalized, flags); err == nil && data != nil {
		return data, nil
	} else if err != nil {
		errs = append(errs, fmt.Errorf("binance: %w", err))
	}

	if data, err := c.getCoinDataFromBybit(normalized, flags); err == nil && data != nil {
		return data, nil
	} else if err != nil {
		errs = append(errs, fmt.Errorf("bybit: %w", err))
	}

	// Quant data now uses public exchange APIs only.
	return nil, combineQuantErrors("failed to fetch quant data from public providers", errs...)
}

// GetCoinDataBatch retrieves quantitative data for multiple coins.
func (c *Client) GetCoinDataBatch(symbols []string, include string) map[string]*QuantData {
	result := make(map[string]*QuantData)

	for _, symbol := range symbols {
		data, err := c.GetCoinData(symbol, include)
		if err != nil {
			log.Printf("⚠️ Failed to fetch coin data for %s: %v", symbol, err)
			continue
		}
		if data != nil {
			result[NormalizeSymbol(symbol)] = data
		}
	}

	return result
}

func (c *Client) getCoinDataFromLegacyNofx(symbol string, include string) (*QuantData, error) {
	if include == "" {
		include = "netflow,oi,price"
	}

	endpoint := fmt.Sprintf("/api/coin/%s?include=%s", symbol, include)
	body, err := c.doRequest(endpoint)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	var response CoinResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("json parsing failed: %w", err)
	}
	if !response.Success && response.Code != 0 {
		return nil, fmt.Errorf("api returned error code: %d", response.Code)
	}
	return response.Data, nil
}

func (c *Client) getCoinDataFromBinance(symbol string, flags quantIncludeFlags) (*QuantData, error) {
	result := &QuantData{Symbol: symbol, PriceChange: make(map[string]float64)}
	collected := 0

	if flags.price {
		ticker, err := c.fetchBinanceTicker24h(symbol)
		if err != nil {
			return nil, err
		}
		if p, err := parseStringFloat(ticker.LastPrice); err == nil && p > 0 {
			result.Price = p
			collected++
		}
		if pct, err := parseStringFloat(ticker.PriceChangePercent); err == nil {
			result.PriceChange["24h"] = pct / 100
			collected++
		}
	}

	if flags.oi {
		oiData := &OIData{Delta: make(map[string]*OIDeltaData)}
		if current, err := c.fetchBinanceOpenInterest(symbol); err == nil && current > 0 {
			oiData.CurrentOI = current
			collected++
		}

		for duration, period := range binanceOIPeriodByDuration {
			prev, curr, err := c.fetchBinanceOIHistPair(symbol, period)
			if err != nil {
				continue
			}
			delta := curr.oi - prev.oi
			pct := 0.0
			if prev.oi != 0 {
				pct = (delta / prev.oi) * 100
			}
			oiData.Delta[duration] = &OIDeltaData{
				OIDelta:        delta,
				OIDeltaValue:   curr.oiValue - prev.oiValue,
				OIDeltaPercent: pct,
			}
			if oiData.CurrentOI == 0 && curr.oi > 0 {
				oiData.CurrentOI = curr.oi
			}
			collected++
		}

		if len(oiData.Delta) > 0 || oiData.CurrentOI > 0 {
			result.OI = map[string]*OIData{"binance": oiData}
		}
	}

	if flags.netflow {
		flow := make(map[string]float64)
		for duration, period := range binanceTakerPeriodByDuration {
			buy, sell, err := c.fetchBinanceTakerFlow(symbol, period)
			if err != nil {
				continue
			}
			flow[duration] = buy - sell
			collected++
		}
		if len(flow) > 0 {
			result.Netflow = &NetflowData{
				// Proxy metric: taker buy/sell imbalance.
				Institution: &FlowTypeData{Future: flow},
			}
		}
	}

	if result.Price == 0 && flags.price {
		if p, err := c.fetchBinanceLastPrice(symbol); err == nil && p > 0 {
			result.Price = p
			collected++
		}
	}

	if collected == 0 {
		return nil, fmt.Errorf("no usable quant fields from binance")
	}
	return result, nil
}

func (c *Client) getCoinDataFromBybit(symbol string, flags quantIncludeFlags) (*QuantData, error) {
	ticker, err := c.fetchBybitTicker(symbol)
	if err != nil {
		return nil, err
	}

	result := &QuantData{Symbol: symbol, PriceChange: make(map[string]float64)}
	collected := 0

	if flags.price {
		if p, err := parseStringFloat(ticker.LastPrice); err == nil && p > 0 {
			result.Price = p
			collected++
		}
		if pct, err := parseStringFloat(ticker.Price24hPcnt); err == nil {
			result.PriceChange["24h"] = pct
			collected++
		}
	}

	if flags.oi {
		oiData := &OIData{Delta: make(map[string]*OIDeltaData)}
		if current, err := parseStringFloat(ticker.OpenInterest); err == nil && current > 0 {
			oiData.CurrentOI = current
			collected++
		}

		for duration, interval := range bybitOIPeriodByDuration {
			prev, curr, err := c.fetchBybitOpenInterestPair(symbol, interval)
			if err != nil {
				continue
			}
			delta := curr - prev
			pct := 0.0
			if prev != 0 {
				pct = (delta / prev) * 100
			}
			oiData.Delta[duration] = &OIDeltaData{
				OIDelta:        delta,
				OIDeltaValue:   0,
				OIDeltaPercent: pct,
			}
			if oiData.CurrentOI == 0 && curr > 0 {
				oiData.CurrentOI = curr
			}
			collected++
		}

		if len(oiData.Delta) > 0 || oiData.CurrentOI > 0 {
			result.OI = map[string]*OIData{"bybit": oiData}
		}
	}

	if collected == 0 {
		return nil, fmt.Errorf("no usable quant fields from bybit")
	}
	return result, nil
}

func parseQuantIncludeFlags(include string) quantIncludeFlags {
	if strings.TrimSpace(include) == "" {
		include = "netflow,oi,price"
	}
	flags := quantIncludeFlags{}
	for _, part := range strings.Split(include, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "price":
			flags.price = true
		case "oi":
			flags.oi = true
		case "netflow":
			flags.netflow = true
		}
	}
	if !flags.price && !flags.oi && !flags.netflow {
		flags.price = true
		flags.oi = true
	}
	return flags
}

func parseStringFloat(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty numeric string")
	}
	return strconv.ParseFloat(raw, 64)
}

func combineQuantErrors(prefix string, errs ...error) error {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		parts = append(parts, err.Error())
	}
	if len(parts) == 0 {
		return errors.New(prefix)
	}
	return fmt.Errorf("%s: %s", prefix, strings.Join(parts, "; "))
}

func (c *Client) fetchBinanceTicker24h(symbol string) (*binanceTicker24hResponse, error) {
	query := url.Values{}
	query.Set("symbol", symbol)
	body, err := c.doPublicRequest(BinanceBaseURL, "/fapi/v1/ticker/24hr", query)
	if err != nil {
		return nil, err
	}
	var resp binanceTicker24hResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse binance ticker24h: %w", err)
	}
	return &resp, nil
}

func (c *Client) fetchBinanceLastPrice(symbol string) (float64, error) {
	query := url.Values{}
	query.Set("symbol", symbol)
	body, err := c.doPublicRequest(BinanceBaseURL, "/fapi/v1/ticker/price", query)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Price string `json:"price"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse binance last price: %w", err)
	}
	return parseStringFloat(resp.Price)
}

func (c *Client) fetchBinanceOpenInterest(symbol string) (float64, error) {
	query := url.Values{}
	query.Set("symbol", symbol)
	body, err := c.doPublicRequest(BinanceBaseURL, "/fapi/v1/openInterest", query)
	if err != nil {
		return 0, err
	}
	var resp binanceOpenInterestResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse binance open interest: %w", err)
	}
	return parseStringFloat(resp.OpenInterest)
}

type oiHistPair struct {
	ts      int64
	oi      float64
	oiValue float64
}

func (c *Client) fetchBinanceOIHistPair(symbol, period string) (oiHistPair, oiHistPair, error) {
	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("period", period)
	query.Set("limit", "2")
	body, err := c.doPublicRequest(BinanceBaseURL, "/futures/data/openInterestHist", query)
	if err != nil {
		return oiHistPair{}, oiHistPair{}, err
	}
	var resp []binanceOpenInterestHistItem
	if err := json.Unmarshal(body, &resp); err != nil {
		return oiHistPair{}, oiHistPair{}, fmt.Errorf("parse binance open interest history: %w", err)
	}
	if len(resp) == 0 {
		return oiHistPair{}, oiHistPair{}, fmt.Errorf("empty open interest history")
	}

	pairs := make([]oiHistPair, 0, len(resp))
	for _, item := range resp {
		oi, err := parseStringFloat(item.SumOpenInterest)
		if err != nil {
			continue
		}
		oiValue := 0.0
		if v, err := parseStringFloat(item.SumOpenInterestValue); err == nil {
			oiValue = v
		}
		pairs = append(pairs, oiHistPair{ts: item.Timestamp, oi: oi, oiValue: oiValue})
	}
	if len(pairs) == 0 {
		return oiHistPair{}, oiHistPair{}, fmt.Errorf("open interest history has no valid rows")
	}

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].ts < pairs[j].ts })
	if len(pairs) == 1 {
		return pairs[0], pairs[0], nil
	}
	return pairs[len(pairs)-2], pairs[len(pairs)-1], nil
}

func (c *Client) fetchBinanceTakerFlow(symbol, period string) (float64, float64, error) {
	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("period", period)
	query.Set("limit", "1")
	body, err := c.doPublicRequest(BinanceBaseURL, "/futures/data/takerlongshortRatio", query)
	if err != nil {
		return 0, 0, err
	}
	var resp []binanceTakerRatioItem
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, 0, fmt.Errorf("parse binance taker flow: %w", err)
	}
	if len(resp) == 0 {
		return 0, 0, fmt.Errorf("empty taker flow")
	}
	last := resp[len(resp)-1]
	buy, err := parseStringFloat(last.BuyVol)
	if err != nil {
		return 0, 0, err
	}
	sell, err := parseStringFloat(last.SellVol)
	if err != nil {
		return 0, 0, err
	}
	if math.IsNaN(buy) || math.IsNaN(sell) || math.IsInf(buy, 0) || math.IsInf(sell, 0) {
		return 0, 0, fmt.Errorf("invalid taker flow values")
	}
	return buy, sell, nil
}

func (c *Client) fetchBybitTicker(symbol string) (*struct {
	LastPrice    string
	Price24hPcnt string
	OpenInterest string
}, error) {
	query := url.Values{}
	query.Set("category", "linear")
	query.Set("symbol", symbol)
	body, err := c.doPublicRequest(BybitBaseURL, "/v5/market/tickers", query)
	if err != nil {
		return nil, err
	}
	var resp bybitTickerResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse bybit ticker: %w", err)
	}
	if resp.RetCode != 0 {
		return nil, fmt.Errorf("bybit ticker error retCode=%d msg=%s", resp.RetCode, resp.RetMsg)
	}
	if len(resp.Result.List) == 0 {
		return nil, fmt.Errorf("bybit ticker empty list")
	}
	last := resp.Result.List[len(resp.Result.List)-1]
	return &struct {
		LastPrice    string
		Price24hPcnt string
		OpenInterest string
	}{
		LastPrice:    last.LastPrice,
		Price24hPcnt: last.Price24hPcnt,
		OpenInterest: last.OpenInterest,
	}, nil
}

func (c *Client) fetchBybitOpenInterestPair(symbol, interval string) (float64, float64, error) {
	query := url.Values{}
	query.Set("category", "linear")
	query.Set("symbol", symbol)
	query.Set("intervalTime", interval)
	query.Set("limit", "2")
	body, err := c.doPublicRequest(BybitBaseURL, "/v5/market/open-interest", query)
	if err != nil {
		return 0, 0, err
	}
	var resp bybitOpenInterestResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, 0, fmt.Errorf("parse bybit open interest: %w", err)
	}
	if resp.RetCode != 0 {
		return 0, 0, fmt.Errorf("bybit open interest error retCode=%d msg=%s", resp.RetCode, resp.RetMsg)
	}
	if len(resp.Result.List) == 0 {
		return 0, 0, fmt.Errorf("bybit open interest empty list")
	}
	type pair struct {
		ts int64
		oi float64
	}
	pairs := make([]pair, 0, len(resp.Result.List))
	for _, item := range resp.Result.List {
		oi, err := parseStringFloat(item.OpenInterest)
		if err != nil {
			continue
		}
		ts, _ := strconv.ParseInt(strings.TrimSpace(item.Timestamp), 10, 64)
		pairs = append(pairs, pair{ts: ts, oi: oi})
	}
	if len(pairs) == 0 {
		return 0, 0, fmt.Errorf("bybit open interest has no valid rows")
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].ts < pairs[j].ts })
	if len(pairs) == 1 {
		return pairs[0].oi, pairs[0].oi, nil
	}
	return pairs[len(pairs)-2].oi, pairs[len(pairs)-1].oi, nil
}

// FormatQuantDataForAI formats single coin quant data for AI consumption.
func FormatQuantDataForAI(symbol string, data *QuantData, lang Language) string {
	if data == nil {
		return ""
	}
	if lang == LangChinese {
		return formatQuantDataZH(symbol, data)
	}
	return formatQuantDataEN(symbol, data)
}

func formatQuantDataZH(symbol string, data *QuantData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("### %s 量化数据\n", symbol))
	sb.WriteString(fmt.Sprintf("价格: $%.4f\n\n", data.Price))

	if len(data.PriceChange) > 0 {
		sb.WriteString("**价格变化**:\n")
		for _, d := range []string{"1h", "4h", "8h", "12h", "24h"} {
			if change, ok := data.PriceChange[d]; ok {
				sb.WriteString(fmt.Sprintf("- %s: %+.2f%%\n", d, change*100))
			}
		}
		sb.WriteString("\n")
	}

	if len(data.OI) > 0 {
		for exchange, oiData := range data.OI {
			if oiData == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("**%s OI**:\n", strings.ToUpper(exchange)))
			sb.WriteString(fmt.Sprintf("- Current OI: %.2f\n", oiData.CurrentOI))
			if oiData.NetLong > 0 || oiData.NetShort > 0 {
				sb.WriteString(fmt.Sprintf("- Net Long: %.2f, Net Short: %.2f\n", oiData.NetLong, oiData.NetShort))
			}
			if oiData.Delta != nil {
				if delta, ok := oiData.Delta["1h"]; ok && delta != nil {
					sb.WriteString(fmt.Sprintf("- 1h Change: %s (%.2f%%)\n", formatValue(delta.OIDeltaValue), delta.OIDeltaPercent))
				}
			}
			sb.WriteString("\n")
		}
	}

	if data.Netflow != nil && data.Netflow.Institution != nil && data.Netflow.Institution.Future != nil {
		sb.WriteString("**资金流向 (代理指标, taker买卖差)**:\n")
		for _, d := range []string{"1h", "4h", "24h"} {
			if flow, ok := data.Netflow.Institution.Future[d]; ok {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", d, formatValue(flow)))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatQuantDataEN(symbol string, data *QuantData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("### %s Quant Data\n", symbol))
	sb.WriteString(fmt.Sprintf("Price: $%.4f\n\n", data.Price))

	if len(data.PriceChange) > 0 {
		sb.WriteString("**Price Change**:\n")
		for _, d := range []string{"1h", "4h", "8h", "12h", "24h"} {
			if change, ok := data.PriceChange[d]; ok {
				sb.WriteString(fmt.Sprintf("- %s: %+.2f%%\n", d, change*100))
			}
		}
		sb.WriteString("\n")
	}

	if len(data.OI) > 0 {
		for exchange, oiData := range data.OI {
			if oiData == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("**%s OI**:\n", strings.ToUpper(exchange)))
			sb.WriteString(fmt.Sprintf("- Current OI: %.2f\n", oiData.CurrentOI))
			if oiData.NetLong > 0 || oiData.NetShort > 0 {
				sb.WriteString(fmt.Sprintf("- Net Long: %.2f, Net Short: %.2f\n", oiData.NetLong, oiData.NetShort))
			}
			if oiData.Delta != nil {
				if delta, ok := oiData.Delta["1h"]; ok && delta != nil {
					sb.WriteString(fmt.Sprintf("- 1h Change: %s (%.2f%%)\n", formatValue(delta.OIDeltaValue), delta.OIDeltaPercent))
				}
			}
			sb.WriteString("\n")
		}
	}

	if data.Netflow != nil && data.Netflow.Institution != nil && data.Netflow.Institution.Future != nil {
		sb.WriteString("**Fund Flow (proxy: taker buy/sell imbalance)**:\n")
		for _, d := range []string{"1h", "4h", "24h"} {
			if flow, ok := data.Netflow.Institution.Future[d]; ok {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", d, formatValue(flow)))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
