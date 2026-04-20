package backtest

import (
	"math"
	"testing"
)

func TestBacktestAccountTotalEquityFallsBackToEntryPriceWhenMarkMissing(t *testing.T) {
	acc := NewBacktestAccount(1000, 0, 0)
	if _, _, _, err := acc.Open("DOGEUSDT", "long", 100, 2, 10, 0); err != nil {
		t.Fatalf("open position failed: %v", err)
	}

	equity, unrealized, _ := acc.TotalEquity(map[string]float64{})
	if math.Abs(equity-1000) > 1e-9 {
		t.Fatalf("expected equity=1000, got %.8f", equity)
	}
	if math.Abs(unrealized) > 1e-9 {
		t.Fatalf("expected unrealized=0, got %.8f", unrealized)
	}
}

func TestBacktestAccountTotalEquityUsesProvidedMarkPrice(t *testing.T) {
	acc := NewBacktestAccount(1000, 0, 0)
	if _, _, _, err := acc.Open("DOGEUSDT", "long", 100, 2, 10, 0); err != nil {
		t.Fatalf("open position failed: %v", err)
	}

	equity, unrealized, _ := acc.TotalEquity(map[string]float64{"DOGEUSDT": 8})
	if math.Abs(equity-800) > 1e-9 {
		t.Fatalf("expected equity=800, got %.8f", equity)
	}
	if math.Abs(unrealized+200) > 1e-9 {
		t.Fatalf("expected unrealized=-200, got %.8f", unrealized)
	}
}
