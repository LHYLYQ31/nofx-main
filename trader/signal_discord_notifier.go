package trader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"nofx/kernel"
	"nofx/logger"
	"nofx/store"
)

type traderDiscordNotifier struct {
	webhookURL string
	username   string
	client     *http.Client
}

type traderDiscordWebhookPayload struct {
	Username string             `json:"username,omitempty"`
	Embeds   []traderSignalEmbed `json:"embeds"`
}

type traderSignalEmbed struct {
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Color       int                    `json:"color"`
	Footer      *traderSignalEmbedFooter `json:"footer,omitempty"`
	Timestamp   string                 `json:"timestamp,omitempty"`
}

type traderSignalEmbedFooter struct {
	Text string `json:"text"`
}

func newTraderDiscordNotifier(webhookURL, username string) *traderDiscordNotifier {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "newmoneyclub"
	}
	return &traderDiscordNotifier{
		webhookURL: webhookURL,
		username:   username,
		client: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

func (at *AutoTrader) notifyStrategySignal(decision *kernel.Decision, actionRecord *store.DecisionAction) {
	if at == nil || at.store == nil || decision == nil || actionRecord == nil {
		return
	}
	if strings.TrimSpace(at.strategyID) == "" || strings.TrimSpace(at.userEmail) == "" {
		return
	}

	webhookURL, ok, err := at.store.ResolveSignalWebhook(at.strategyID, at.userEmail)
	if err != nil {
		logger.Warnf("resolve strategy webhook failed trader=%s strategy=%s email=%s: %v",
			at.id, at.strategyID, at.userEmail, err)
		return
	}
	if !ok {
		return
	}

	notifier := newTraderDiscordNotifier(webhookURL, "newmoneyclub")
	if notifier == nil {
		return
	}

	decisionCopy := *decision
	actionCopy := *actionRecord
	go func() {
		if err := notifier.sendDecisionSignal(at.id, at.name, &decisionCopy, &actionCopy); err != nil {
			logger.Warnf("strategy signal discord notify failed trader=%s strategy=%s symbol=%s action=%s: %v",
				at.id, at.strategyID, decisionCopy.Symbol, decisionCopy.Action, err)
		}
	}()
}

func (n *traderDiscordNotifier) sendDecisionSignal(traderID, traderName string, decision *kernel.Decision, action *store.DecisionAction) error {
	if n == nil || decision == nil || action == nil {
		return nil
	}
	payload := traderDiscordWebhookPayload{
		Username: n.username,
		Embeds: []traderSignalEmbed{
			buildTraderSignalEmbed(traderID, traderName, decision, action),
		},
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

func buildTraderSignalEmbed(traderID, traderName string, decision *kernel.Decision, action *store.DecisionAction) traderSignalEmbed {
	actionName := strings.ToLower(strings.TrimSpace(decision.Action))
	title, color := traderSignalTitle(actionName, decision.Symbol)
	desc := traderSignalDescription(traderName, decision, action)
	return traderSignalEmbed{
		Title:       title,
		Description: desc,
		Color:       color,
		Footer:      &traderSignalEmbedFooter{Text: fmt.Sprintf("newmoneyclub • trader:%s", shortID(traderID))},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func shortID(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 8 {
		return v
	}
	return v[:8]
}

func traderSignalTitle(action, symbol string) (string, int) {
	pair := formatTraderPair(symbol)
	switch action {
	case "open_long":
		return fmt.Sprintf("🦅 #做多信号 | $%s", pair), 0x00C853
	case "open_short":
		return fmt.Sprintf("🦅 #做空信号 | $%s", pair), 0xD50000
	case "close_long", "close_short":
		return fmt.Sprintf("🦅 #平仓信号 | $%s", pair), 0xF0B90B
	default:
		return fmt.Sprintf("🦅 #交易信号 | $%s", pair), 0x3B82F6
	}
}

func traderSignalDescription(traderName string, decision *kernel.Decision, action *store.DecisionAction) string {
	actionName := strings.ToLower(strings.TrimSpace(decision.Action))
	logic := traderReasoning(decision.Reasoning)
	price := action.Price
	if price <= 0 {
		price = 0
	}
	if actionName == "open_long" || actionName == "open_short" {
		entryLow, entryHigh := traderEntryRange(price)
		sl := traderPriceText(decision.StopLoss)
		tp1 := traderPriceText(decision.TakeProfit)
		tp2, tp3 := "待补充", "待补充"
		if decision.TakeProfit > 0 {
			if actionName == "open_short" {
				tp2 = traderPriceText(decision.TakeProfit * 0.97)
				tp3 = traderPriceText(decision.TakeProfit * 0.94)
			} else {
				tp2 = traderPriceText(decision.TakeProfit * 1.03)
				tp3 = traderPriceText(decision.TakeProfit * 1.06)
			}
		}
		leverage := "最高 3x"
		if decision.Leverage > 0 {
			leverage = fmt.Sprintf("最高 %dx", decision.Leverage)
		}
		return fmt.Sprintf(`交易员: %s
📊 **交易逻辑 (The Why):**
%s

⚔️ **执行参数 (Execution):**
👉 **入场区间 (Entry):** %s - %s（分批建仓，不要一次性打满）
🛑 **止损 (Hard SL):** %s（跌破关键结构位绝对平仓，不要扛单！）

🎯 **止盈目标 (Targets):**
🥇 **TP1:** %s（到达后推保护性止损至开仓价，锁定本金）
🥈 **TP2:** %s（减仓 50%%）
🥉 **TP3:** %s（尾仓格局）

⚙️ **风控建议 (Risk Mgt):**
杠杆建议：%s
仓位限制：单笔亏损严格控制在总资金的 2%% 以内。

🤖 **New Money AI 辅助决策系统**`, traderFallback(traderName, "-"), logic, entryLow, entryHigh, sl, tp1, tp2, tp3, leverage)
	}

	return fmt.Sprintf(`交易员: %s
⚔️ **执行参数 (Execution):**
交易动作: %s
数量: %s
价格: %s

📝 **说明 (Notes):**
%s

🤖 **New Money AI 辅助决策系统**`,
		traderFallback(traderName, "-"),
		traderFallback(decision.Action, "待补充"),
		traderTrimFloat(action.Quantity, 6),
		traderPriceText(price),
		logic)
}

func traderReasoning(reason string) string {
	v := strings.TrimSpace(reason)
	if v == "" {
		return "当前信号由 AI 基于历史数据与结构触发。\n多周期条件满足，具备执行参考价值。\n请结合实时盘面与风险承受能力再确认。"
	}
	lines := splitTraderLines(v)
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.Join(lines, "\n")
}

func splitTraderLines(v string) []string {
	raw := strings.Split(v, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		lines = append(lines, clean)
	}
	return lines
}

var traderSymbolRe = regexp.MustCompile(`^([A-Z0-9]+?)(USDT|USD|USDC|BUSD)$`)

func formatTraderPair(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if s == "" {
		return "N/A"
	}
	m := traderSymbolRe.FindStringSubmatch(s)
	if len(m) == 3 {
		return fmt.Sprintf("%s/%s", m[1], m[2])
	}
	return s
}

func traderEntryRange(price float64) (string, string) {
	if price <= 0 {
		return "待补充", "待补充"
	}
	return traderPriceText(price * 0.985), traderPriceText(price * 1.015)
}

func traderPriceText(v float64) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "待补充"
	}
	if v >= 1000 {
		return traderTrimFloat(v, 2)
	}
	if v >= 1 {
		return traderTrimFloat(v, 4)
	}
	return traderTrimFloat(v, 6)
}

func traderTrimFloat(v float64, precision int) string {
	format := fmt.Sprintf("%%.%df", precision)
	s := fmt.Sprintf(format, v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func traderFallback(v, fb string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fb
	}
	return v
}
