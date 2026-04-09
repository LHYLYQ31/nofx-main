package kernel

import "testing"

func TestValidateJSONFormat_AllowsTildeInReasoningString(t *testing.T) {
	jsonContent := `[{"symbol":"BTCUSDT","action":"wait","confidence":0,"reasoning":"区间上沿~71533，下沿~70428"}]`
	if err := validateJSONFormat(jsonContent); err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}

func TestValidateJSONFormat_RejectsThousandSeparator(t *testing.T) {
	jsonContent := `[{"symbol":"BTCUSDT","action":"wait","confidence":0,"reasoning":"ok","position_size_usd":1,234}]`
	if err := validateJSONFormat(jsonContent); err == nil {
		t.Fatal("expected validation error for thousand separator, got nil")
	}
}
