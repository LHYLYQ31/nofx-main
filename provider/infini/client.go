package infini

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://openapi.infini.money"
)

type Client struct {
	KeyID     string
	SecretKey string
	BaseURL   string
	Client    *http.Client
}

type APIError struct {
	StatusCode int
	Code       int
	Message    string
	Detail     string
	RawBody    string
}

func (e *APIError) Error() string {
	if e == nil {
		return "infini api error"
	}
	if e.Code != 0 {
		return fmt.Sprintf("infini api error(status=%d, code=%d): %s", e.StatusCode, e.Code, strings.TrimSpace(e.Message))
	}
	return fmt.Sprintf("infini api error(status=%d): %s", e.StatusCode, strings.TrimSpace(e.Message))
}

type responseEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Detail  string          `json:"detail"`
	Data    json.RawMessage `json:"data"`
}

type CreateOrderRequest struct {
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	RequestID       string `json:"request_id"`
	ClientReference string `json:"client_reference,omitempty"`
	OrderDesc       string `json:"order_desc,omitempty"`
	ExpiresIn       int64  `json:"expires_in,omitempty"`
	MerchantAlias   string `json:"merchant_alias,omitempty"`
	SuccessURL      string `json:"success_url,omitempty"`
	FailureURL      string `json:"failure_url,omitempty"`
	PayMethods      []int  `json:"pay_methods,omitempty"`
}

type CreateOrderResponse struct {
	OrderID         string `json:"order_id"`
	RequestID       string `json:"request_id"`
	CheckoutURL     string `json:"checkout_url"`
	ClientReference string `json:"client_reference"`
}

type QueryOrderResponse struct {
	OrderID          string   `json:"order_id"`
	Status           string   `json:"status"`
	Amount           string   `json:"amount"`
	Currency         string   `json:"currency"`
	AmountConfirming string   `json:"amount_confirming"`
	AmountConfirmed  string   `json:"amount_confirmed"`
	ExpiresAt        int64    `json:"expires_at"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
	ExceptionTags    []string `json:"exception_tags"`
	ClientReference  string   `json:"client_reference"`
}

func NewClient(keyID, secretKey, baseURL string, httpClient *http.Client) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		KeyID:     strings.TrimSpace(keyID),
		SecretKey: strings.TrimSpace(secretKey),
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Client:    httpClient,
	}
}

func (c *Client) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, string, error) {
	var out CreateOrderResponse
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/acquiring/order", req, &out)
	if err != nil {
		return nil, raw, err
	}
	return &out, raw, nil
}

func (c *Client) QueryOrder(ctx context.Context, orderID string) (*QueryOrderResponse, string, error) {
	path := "/v1/acquiring/order?order_id=" + url.QueryEscape(strings.TrimSpace(orderID))
	var out QueryOrderResponse
	raw, _, err := c.requestJSON(ctx, http.MethodGet, path, nil, &out)
	if err != nil {
		return nil, raw, err
	}
	return &out, raw, nil
}

func (c *Client) ReissueCheckoutURL(ctx context.Context, orderID string) (*CreateOrderResponse, string, error) {
	payload := map[string]string{"order_id": strings.TrimSpace(orderID)}
	var out CreateOrderResponse
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/acquiring/token/reissue", payload, &out)
	if err != nil {
		return nil, raw, err
	}
	return &out, raw, nil
}

func (c *Client) requestJSON(ctx context.Context, method, path string, payload interface{}, out interface{}) (string, int, error) {
	bodyBytes, err := marshalPayload(payload)
	if err != nil {
		return "", 0, err
	}

	tryLegacyDatePrefix := false
	for attempt := 0; attempt < 2; attempt++ {
		headers, signErr := c.signHeaders(method, path, bodyBytes, tryLegacyDatePrefix)
		if signErr != nil {
			return "", 0, signErr
		}

		var body io.Reader
		if len(bodyBytes) > 0 {
			body = bytes.NewReader(bodyBytes)
		}

		req, reqErr := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
		if reqErr != nil {
			return "", 0, reqErr
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if len(bodyBytes) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, doErr := c.Client.Do(req)
		if doErr != nil {
			return "", 0, doErr
		}

		rawResp, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return "", resp.StatusCode, readErr
		}
		raw := strings.TrimSpace(string(rawResp))

		if resp.StatusCode >= 400 {
			apiErr := parseAPIError(resp.StatusCode, rawResp)
			if !tryLegacyDatePrefix && shouldRetryWithLegacyDatePrefix(resp.StatusCode, raw, apiErr) {
				tryLegacyDatePrefix = true
				continue
			}
			return raw, resp.StatusCode, apiErr
		}

		if out != nil && len(rawResp) > 0 {
			if decodeErr := decodeResponse(rawResp, out); decodeErr != nil {
				return raw, resp.StatusCode, decodeErr
			}
		}
		return raw, resp.StatusCode, nil
	}

	return "", 0, fmt.Errorf("infini request retry flow ended unexpectedly")
}

func parseAPIError(statusCode int, raw []byte) error {
	apiErr := &APIError{StatusCode: statusCode, RawBody: strings.TrimSpace(string(raw))}
	var env responseEnvelope
	if err := json.Unmarshal(raw, &env); err == nil {
		apiErr.Code = env.Code
		apiErr.Message = env.Message
		apiErr.Detail = env.Detail
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(statusCode)
		}
		return apiErr
	}
	apiErr.Message = http.StatusText(statusCode)
	return apiErr
}

func decodeResponse(raw []byte, out interface{}) error {
	var env responseEnvelope
	if err := json.Unmarshal(raw, &env); err == nil {
		if len(env.Data) > 0 {
			return json.Unmarshal(env.Data, out)
		}
		if env.Code == 0 {
			return json.Unmarshal(raw, out)
		}
	}
	return json.Unmarshal(raw, out)
}

func marshalPayload(payload interface{}) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}
	return json.Marshal(payload)
}

func (c *Client) signHeaders(method, path string, body []byte, legacyDatePrefix bool) (map[string]string, error) {
	if strings.TrimSpace(c.KeyID) == "" || strings.TrimSpace(c.SecretKey) == "" {
		return nil, fmt.Errorf("infini key_id/secret_key are required")
	}

	gmtTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	datePrefix := "date:"
	if legacyDatePrefix {
		datePrefix = " date:"
	}
	signingString := fmt.Sprintf(
		"%s\n%s %s\n%s %s\n",
		c.KeyID,
		strings.ToUpper(strings.TrimSpace(method)),
		strings.TrimSpace(path),
		datePrefix,
		gmtTime,
	)

	mac := hmac.New(sha256.New, []byte(c.SecretKey))
	if _, err := mac.Write([]byte(signingString)); err != nil {
		return nil, err
	}
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	authHeader := fmt.Sprintf(`Signature keyId="%s",algorithm="hmac-sha256",headers="@request-target date",signature="%s"`, c.KeyID, signature)

	headers := map[string]string{
		"Date":          gmtTime,
		"Authorization": authHeader,
	}
	if len(body) > 0 {
		digest := sha256.Sum256(body)
		headers["Digest"] = "SHA-256=" + base64.StdEncoding.EncodeToString(digest[:])
	}
	return headers, nil
}

func shouldRetryWithLegacyDatePrefix(statusCode int, raw string, err error) bool {
	if statusCode != http.StatusUnauthorized {
		return false
	}
	lowerRaw := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(lowerRaw, "can't be validated") || strings.Contains(lowerRaw, "invalid signature") {
		return true
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(apiErr.Message))
	detail := strings.ToLower(strings.TrimSpace(apiErr.Detail))
	return strings.Contains(msg, "can't be validated") ||
		strings.Contains(msg, "invalid signature") ||
		strings.Contains(detail, "can't be validated") ||
		strings.Contains(detail, "invalid signature")
}

func VerifyWebhookSignature(webhookSecret, timestamp, eventID, payload, signature string) bool {
	webhookSecret = strings.TrimSpace(webhookSecret)
	timestamp = strings.TrimSpace(timestamp)
	eventID = strings.TrimSpace(eventID)
	signature = normalizeSignature(signature)
	if webhookSecret == "" || timestamp == "" || eventID == "" || signature == "" {
		return false
	}

	signedContent := timestamp + "." + eventID + "." + payload
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	if _, err := mac.Write([]byte(signedContent)); err != nil {
		return false
	}

	expectedHex := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(expectedHex)), []byte(strings.ToLower(signature))) == 1 {
		return true
	}

	// Compatibility fallback: some integrations may send base64 signature.
	mac2 := hmac.New(sha256.New, []byte(webhookSecret))
	if _, err := mac2.Write([]byte(signedContent)); err != nil {
		return false
	}
	expectedBase64 := base64.StdEncoding.EncodeToString(mac2.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expectedBase64), []byte(signature)) == 1
}

func normalizeSignature(signature string) string {
	signature = strings.TrimSpace(signature)
	if idx := strings.Index(signature, "="); idx > 0 {
		prefix := strings.ToLower(strings.TrimSpace(signature[:idx]))
		if prefix == "sha256" {
			signature = strings.TrimSpace(signature[idx+1:])
		}
	}
	return signature
}

func IsWebhookTimestampFresh(timestamp string, maxSkew time.Duration) bool {
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return false
	}
	ts := time.Unix(sec, 0).UTC()
	now := time.Now().UTC()
	diff := now.Sub(ts)
	if diff < 0 {
		diff = -diff
	}
	return diff <= maxSkew
}
