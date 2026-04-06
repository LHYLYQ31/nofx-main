package infini

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "test-secret"
	ts := "1763512195"
	eventID := "evt-123"
	payload := `{"event":"order.completed","order_id":"ord-1"}`
	signedContent := fmt.Sprintf("%s.%s.%s", ts, eventID, payload)

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signedContent))
	signature := hex.EncodeToString(mac.Sum(nil))

	if !VerifyWebhookSignature(secret, ts, eventID, payload, signature) {
		t.Fatalf("expected valid signature")
	}
	if !VerifyWebhookSignature(secret, ts, eventID, payload, "sha256="+signature) {
		t.Fatalf("expected valid prefixed signature")
	}
	if VerifyWebhookSignature(secret, ts, eventID, payload, "invalid-signature") {
		t.Fatalf("expected invalid signature")
	}
}

func TestIsWebhookTimestampFresh(t *testing.T) {
	now := time.Now().UTC().Unix()
	if !IsWebhookTimestampFresh(fmt.Sprintf("%d", now), 5*time.Minute) {
		t.Fatalf("expected fresh timestamp")
	}

	old := now - int64((6 * time.Minute).Seconds())
	if IsWebhookTimestampFresh(fmt.Sprintf("%d", old), 5*time.Minute) {
		t.Fatalf("expected stale timestamp")
	}
}
