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
	Username string              `json:"username,omitempty"`
	Embeds   []traderSignalEmbed `json:"embeds"`
}

type traderSignalEmbed struct {
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	Color       int                      `json:"color"`
	Footer      *traderSignalEmbedFooter `json:"footer,omitempty"`
	Timestamp   string                   `json:"timestamp,omitempty"`
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
	// Discord signal notifications are sent when notifications are enabled
	// (alert_only mode, or live_and_alert mode that combines live trading with alerts).
	if !at.notificationsEnabled() {
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
		Footer:      &traderSignalEmbedFooter{Text: fmt.Sprintf("New Money AI | trader:%s", shortID(traderID))},
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
	case "hold", "wait":
		return fmt.Sprintf("🦅 #观望信号 | $%s", pair), 0x94A3B8
	default:
		return fmt.Sprintf("🦅 #交易信号 | $%s", pair), 0x3B82F6
	}
}

func traderSignalDescription(traderName string, decision *kernel.Decision, action *store.DecisionAction) string {
	actionName := strings.ToLower(strings.TrimSpace(decision.Action))
	logic := traderReasoning(decision.Reasoning)
	priceText := traderPriceText(action.Price)

	execStatus := "\u2705 SUCCESS"
	if !action.Success {
		execStatus = "\u274C FAILED"
	}

	errorLine := ""
	if msg := strings.TrimSpace(action.Error); msg != "" {
		errorLine = fmt.Sprintf("- Error: %s\n", msg)
	}

	if actionName == "open_long" || actionName == "open_short" {
		leverageText := "N/A"
		if decision.Leverage > 0 {
			leverageText = fmt.Sprintf("%dx", decision.Leverage)
		}
		tp1 := traderPriceText(decision.TakeProfit)
		tp2, tp3 := "N/A", "N/A"
		if decision.TakeProfit > 0 {
			if actionName == "open_short" {
				tp2 = traderPriceText(decision.TakeProfit * 0.97)
				tp3 = traderPriceText(decision.TakeProfit * 0.94)
			} else {
				tp2 = traderPriceText(decision.TakeProfit * 1.03)
				tp3 = traderPriceText(decision.TakeProfit * 1.06)
			}
		}

		return fmt.Sprintf(`Trader: %s

📊 交易逻辑 (The Why):
%s

⚔️ 执行参数 (Execution):
🟠 入场价 (Entry): %s
🔴 止损 (Hard SL): %s
🎯 止盈目标 (Targets):
🥇 TP1: %s
🥈 TP2: %s
🥉 TP3: %s

🧭 风险建议 (Risk Mgt):
杠杆建议: %s
仓位规模: %s USDT
信心分: %d
风险金额: %s USDT

🤖 New Money AI 辅助决策系统
状态: %s | 执行价: %s
%s`,
			traderFallback(traderName, "-"),
			logic,
			priceText,
			traderPriceText(decision.StopLoss),
			tp1,
			tp2,
			tp3,
			leverageText,
			traderPriceText(decision.PositionSizeUSD),
			decision.Confidence,
			traderPriceText(decision.RiskUSD),
			execStatus,
			priceText,
			errorLine,
		)
	}

	return fmt.Sprintf(`Trader: %s

⚔️ 执行参数 (Execution):
动作: %s
信号价: %s

📊 交易逻辑 (The Why):
%s

🤖 New Money AI 辅助决策系统
状态: %s | 执行价: %s
%s`,
		traderFallback(traderName, "-"),
		traderFallback(decision.Action, "N/A"),
		priceText,
		logic,
		execStatus,
		priceText,
		errorLine,
	)
}
func traderReasoning(reason string) string {
	v := strings.TrimSpace(reason)
	if v == "" {
		return "Signal generated by AI from multi-timeframe context. Please verify with live market conditions."
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

func traderPriceText(v float64) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "N/A"
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
