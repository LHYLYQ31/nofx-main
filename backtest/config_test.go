package backtest

import (
	"reflect"
	"testing"
)

func TestNormalizeBacktestSymbols(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "split space separated",
			input: []string{"SOLUSDT SUIUSDT DOGEUSDT 1000PEPEUSDT"},
			want:  []string{"SOLUSDT", "SUIUSDT", "DOGEUSDT", "1000PEPEUSDT"},
		},
		{
			name:  "split mixed separators and dedupe",
			input: []string{"BTCUSDT,ETHUSDT", "ETHUSDT|SOLUSDT", "  BTCUSDT  "},
			want:  []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"},
		},
		{
			name:  "normalize non usdt suffix",
			input: []string{"BTC ETH"},
			want:  []string{"BTCUSDT", "ETHUSDT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBacktestSymbols(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeBacktestSymbols(%v)=%v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
