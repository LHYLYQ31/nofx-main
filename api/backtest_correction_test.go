package api

import (
	"testing"

	"nofx/backtest"
)

func TestInferTradeSideSupportsLegacyBuySell(t *testing.T) {
	tests := []struct {
		name string
		evt  backtest.TradeEvent
		want string
	}{
		{
			name: "open buy treated as long",
			evt: backtest.TradeEvent{
				Action: "open",
				Side:   "buy",
			},
			want: "long",
		},
		{
			name: "close buy treated as short close",
			evt: backtest.TradeEvent{
				Action: "close",
				Side:   "buy",
			},
			want: "short",
		},
		{
			name: "open sell treated as short",
			evt: backtest.TradeEvent{
				Action: "open",
				Side:   "sell",
			},
			want: "short",
		},
		{
			name: "close sell treated as long close",
			evt: backtest.TradeEvent{
				Action: "close",
				Side:   "sell",
			},
			want: "long",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inferTradeSide(tc.evt)
			if got != tc.want {
				t.Fatalf("inferTradeSide() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFillTradeStatsTreatsUppercaseCloseAsTrade(t *testing.T) {
	metrics := &backtest.Metrics{
		SymbolStats: map[string]backtest.SymbolMetrics{},
	}
	events := []backtest.TradeEvent{
		{
			Symbol:      "BTCUSDT",
			Action:      "CLOSE_SHORT",
			RealizedPnL: 0,
		},
	}

	fillTradeStats(metrics, events)

	if metrics.Trades != 1 {
		t.Fatalf("metrics.Trades = %d, want 1", metrics.Trades)
	}
	if stats, ok := metrics.SymbolStats["BTCUSDT"]; !ok || stats.TotalTrades != 1 {
		t.Fatalf("symbol stats not counted for uppercase close event")
	}
}

