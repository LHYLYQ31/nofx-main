package backtest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	discordColorGreen = 0x00C853
	discordColorRed   = 0xD50000
)

type discordNotifier struct {
	webhookURL string
	username   string
	client     *http.Client
}

type discordWebhookPayload struct {
	Username string         `json:"username,omitempty"`
	Embeds   []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Color       int                 `json:"color"`
	Footer      *discordEmbedFooter `json:"footer,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type discordEmbedFooter struct {
	Text string `json:"text"`
}

func newDiscordNotifier(webhookURL, username string) *discordNotifier {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "newmoneyclub"
	}
	return &discordNotifier{
		webhookURL: webhookURL,
		username:   username,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (n *discordNotifier) notifyTrade(runID, userEmail string, evt TradeEvent, equity float64) error {
	if n == nil {
		return nil
	}

	payload := discordWebhookPayload{
		Username: n.username,
		Embeds:   []discordEmbed{buildBacktestTradeEmbed(runID, userEmail, evt, equity)},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("discord webhook status=%d", resp.StatusCode)
	}
	return nil
}

func buildBacktestTradeEmbed(runID, userEmail string, evt TradeEvent, equity float64) discordEmbed {
	_ = runID
	_ = userEmail
	_ = equity
	title, color := signalTitle(evt)
	ts := time.UnixMilli(evt.Timestamp).UTC()

	return discordEmbed{
		Title:       title,
		Description: buildSignalDescription(evt),
		Color:       color,
		Footer:      &discordEmbedFooter{Text: "New Money AI • Strategy Signal"},
		Timestamp:   ts.Format(time.RFC3339),
	}
}

func buildSignalDescription(evt TradeEvent) string {
	action := strings.ToLower(strings.TrimSpace(evt.Action))
	if action == "open_long" || action == "open_short" {
		return buildOpenSignalDescription(evt)
	}
	return buildCloseSignalDescription(evt)
}

func buildOpenSignalDescription(evt TradeEvent) string {
	logicText := logicFromReasoning(evt.Reasoning)
	entryLow, entryHigh := signalEntryRange(evt)
	stopLoss := signalStopLossText(evt)
	tp1, tp2, tp3 := signalTargets(evt)
	leverageText := signalLeverageText(evt.Leverage)

	return fmt.Sprintf(`交易逻辑 (The Why):
%s

执行参数 (Execution):
入场区间 (Entry): %s - %s（分批建仓，不要一次性打满）
止损 (Hard SL): %s（跌破关键结构位绝对平仓，不要扛单！）

止盈目标 (Targets):
TP1: %s（到达后推保护性止损至开仓价，锁定本金）
TP2: %s（减仓 50%%）
TP3: %s（尾仓格局）

风控建议 (Risk Mgt):
杠杆建议：%s
仓位限制：单笔亏损严格控制在总资金的 2%% 以内。

New Money AI 辅助决策系统`, logicText, entryLow, entryHigh, stopLoss, tp1, tp2, tp3, leverageText)
}

func buildCloseSignalDescription(evt TradeEvent) string {
	logicText := logicFromReasoning(evt.Reasoning)
	return fmt.Sprintf(`平仓执行 (Execution):
交易动作：%s
方向：%s
数量：%s
成交价：%s
已实现盈亏：%s

说明 (Notes):
%s

New Money AI 辅助决策系统`,
		fallbackText(evt.Action, "待补充"),
		fallbackText(evt.Side, "待补充"),
		trimFloat(evt.Quantity, 6),
		fmtPrice(evt.Price),
		trimFloat(evt.RealizedPnL, 4),
		logicText,
	)
}

func signalTitle(evt TradeEvent) (string, int) {
	pair := formatSymbolPair(evt.Symbol)
	action := strings.ToLower(strings.TrimSpace(evt.Action))
	switch action {
	case "open_long":
		return fmt.Sprintf("#做多信号 | $%s", pair), discordColorGreen
	case "open_short":
		return fmt.Sprintf("#做空信号 | $%s", pair), discordColorRed
	case "close_short":
		return fmt.Sprintf("#平仓信号 | $%s", pair), discordColorGreen
	case "close_long":
		return fmt.Sprintf("#平仓信号 | $%s", pair), discordColorRed
	case "liquidated":
		return fmt.Sprintf("#风控信号 | $%s", pair), discordColorRed
	default:
		if evt.RealizedPnL >= 0 {
			return fmt.Sprintf("#交易信号 | $%s", pair), discordColorGreen
		}
		return fmt.Sprintf("#交易信号 | $%s", pair), discordColorRed
	}
}

func fmtPrice(v float64) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "待补充"
	}
	if v >= 1000 {
		return trimFloat(v, 2)
	}
	if v >= 1 {
		return trimFloat(v, 4)
	}
	return trimFloat(v, 6)
}

func signalLeverageText(v int) string {
	if v <= 0 {
		return "最高 5x - 10x"
	}
	if v <= 5 {
		return fmt.Sprintf("最高 %dx", v)
	}
	return fmt.Sprintf("最高 5x - %dx", v)
}

func signalEntryRange(evt TradeEvent) (string, string) {
	price := referencePrice(evt)
	if price <= 0 {
		return "待补充", "待补充"
	}
	low := price * 0.985
	high := price * 1.015
	return fmtPrice(low), fmtPrice(high)
}

func signalTargets(evt TradeEvent) (string, string, string) {
	side := strings.ToLower(strings.TrimSpace(evt.Side))
	price := referencePrice(evt)
	if price <= 0 {
		return "待补充", "待补充", "待补充"
	}

	if evt.TakeProfit > 0 {
		tp1 := evt.TakeProfit
		if side == "short" {
			tp2 := tp1 * 0.97
			tp3 := tp1 * 0.94
			return fmtPrice(tp1), fmtPrice(tp2), fmtPrice(tp3)
		}
		tp2 := tp1 * 1.03
		tp3 := tp1 * 1.06
		return fmtPrice(tp1), fmtPrice(tp2), fmtPrice(tp3)
	}

	if side == "short" {
		return fmtPrice(price * 0.97), fmtPrice(price * 0.94), fmtPrice(price * 0.90)
	}
	return fmtPrice(price * 1.03), fmtPrice(price * 1.06), fmtPrice(price * 1.10)
}

func signalStopLossText(evt TradeEvent) string {
	stopLoss := resolveStopLoss(evt)
	if stopLoss <= 0 {
		return "待补充"
	}
	return fmtPrice(stopLoss)
}

func resolveStopLoss(evt TradeEvent) float64 {
	if evt.StopLoss > 0 {
		return evt.StopLoss
	}
	price := referencePrice(evt)
	if price <= 0 {
		return 0
	}
	if strings.ToLower(strings.TrimSpace(evt.Side)) == "short" {
		return price * 1.03
	}
	return price * 0.97
}

func referencePrice(evt TradeEvent) float64 {
	if evt.Price > 0 {
		return evt.Price
	}
	if evt.OrderValue > 0 && evt.Quantity > 0 {
		return evt.OrderValue / evt.Quantity
	}

	side := strings.ToLower(strings.TrimSpace(evt.Side))
	if evt.TakeProfit > 0 {
		if side == "short" {
			return evt.TakeProfit / 0.97
		}
		return evt.TakeProfit / 1.03
	}
	if evt.StopLoss > 0 {
		if side == "short" {
			return evt.StopLoss / 1.03
		}
		return evt.StopLoss / 0.97
	}
	return 0
}

func logicFromReasoning(reasoning string) string {
	parts := splitLogic(reasoning)
	if len(parts) == 0 {
		return strings.Join([]string{
			"当前信号由 AI 基于历史数据与结构触发。",
			"多周期条件满足，具备执行参考价值。",
			"请结合实时盘面与风险承受能力再确认。",
		}, "\n")
	}
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, "\n")
}

func splitLogic(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	re := regexp.MustCompile(`[。；;！!？?\n]+`)
	raw := re.Split(text, -1)
	out := make([]string, 0, 4)
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line+"。")
	}
	return out
}

func formatSymbolPair(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return "N/A"
	}
	if strings.HasSuffix(symbol, "USDT") && len(symbol) > 4 {
		return fmt.Sprintf("%s/USDT", strings.TrimSuffix(symbol, "USDT"))
	}
	re := regexp.MustCompile(`^([A-Z0-9]+?)(USDC|USD|BTC|ETH|USDT)$`)
	m := re.FindStringSubmatch(symbol)
	if len(m) == 3 {
		return fmt.Sprintf("%s/%s", m[1], m[2])
	}
	return symbol
}

func trimFloat(v float64, precision int) string {
	s := fmt.Sprintf("%.*f", precision, v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func fallbackText(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}
