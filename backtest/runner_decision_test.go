package backtest

import "testing"

func TestDecisionRequiresSymbol(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{action: "hold", want: false},
		{action: "wait", want: false},
		{action: "pause_grid", want: false},
		{action: "resume_grid", want: false},
		{action: "cancel_all_orders", want: false},
		{action: "open_long", want: true},
		{action: "open_short", want: true},
		{action: "close_long", want: true},
		{action: "close_short", want: true},
		{action: "adjust_grid", want: true},
		{action: "place_buy_limit", want: true},
		{action: "place_sell_limit", want: true},
		{action: "cancel_order", want: true},
	}

	for _, tt := range tests {
		got := decisionRequiresSymbol(tt.action)
		if got != tt.want {
			t.Fatalf("decisionRequiresSymbol(%q)=%v, want %v", tt.action, got, tt.want)
		}
	}
}

func TestIsOpenAction(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{action: "open_long", want: true},
		{action: "open_short", want: true},
		{action: "close_long", want: false},
		{action: "close_short", want: false},
		{action: "wait", want: false},
	}

	for _, tt := range tests {
		got := isOpenAction(tt.action)
		if got != tt.want {
			t.Fatalf("isOpenAction(%q)=%v, want %v", tt.action, got, tt.want)
		}
	}
}
