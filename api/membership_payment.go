package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"nofx/crypto"
	"nofx/provider/infini"
	"nofx/store"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const paymentConfigCacheTTL = 45 * time.Second

type paymentConfigCacheEntry struct {
	config    *store.PaymentProviderConfig
	expiresAt time.Time
}

type infiniWebhookPayload struct {
	Event           string   `json:"event"`
	OrderID         string   `json:"order_id"`
	ClientReference string   `json:"client_reference"`
	Status          string   `json:"status"`
	ExceptionTags   []string `json:"exception_tags"`
}

func (s *Server) handleMembershipPlans(c *gin.Context) {
	items, err := s.store.MembershipPlan().List(true)
	if err != nil {
		SafeInternalError(c, "List membership plans", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) handleCurrentMembership(c *gin.Context) {
	userID := c.GetString("user_id")
	now := time.Now().UTC()
	active, err := s.store.UserMembership().GetActiveByUser(userID, now)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		SafeInternalError(c, "Get membership", err)
		return
	}
	if active == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{
			"membership": nil,
			"is_active":  false,
			"tier":       "free",
		})
		return
	}

	plan, planErr := s.store.MembershipPlan().GetByCode(active.PlanCode)
	if planErr != nil && !errors.Is(planErr, gorm.ErrRecordNotFound) {
		SafeInternalError(c, "Get membership plan", planErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"membership": active,
		"is_active":  active.EndAt.After(now) && active.Status == store.MembershipStatusActive,
		"tier":       active.PlanCode,
		"plan":       plan,
	})
}

func (s *Server) handleCreateInfiniOrder(c *gin.Context) {
	userID := strings.TrimSpace(c.GetString("user_id"))
	if userID == "" {
		SafeUnauthorized(c)
		return
	}

	var req struct {
		PlanCode     string `json:"plan_code" binding:"required"`
		SuccessURL   string `json:"success_url"`
		FailureURL   string `json:"failure_url"`
		OrderDesc    string `json:"order_desc"`
		ExpiresInSec int64  `json:"expires_in_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	plan, err := s.store.MembershipPlan().GetByCode(req.PlanCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			SafeBadRequest(c, "Membership plan not found")
			return
		}
		SafeInternalError(c, "Get membership plan", err)
		return
	}
	if !plan.Enabled {
		SafeBadRequest(c, "Membership plan is disabled")
		return
	}

	cfg, err := s.getActivePaymentProviderConfig(store.PaymentProviderInfini)
	if err != nil {
		SafeInternalError(c, "Load payment provider config", err)
		return
	}

	requestID := uuid.New().String()
	merchantOrderID := "mem-" + uuid.New().String()
	order := &store.PaymentOrder{
		UserID:                userID,
		Provider:              store.PaymentProviderInfini,
		ProviderConfigID:      cfg.ID,
		ProviderConfigVersion: cfg.Version,
		RequestID:             requestID,
		MerchantOrderID:       merchantOrderID,
		BizType:               store.PaymentBizMembership,
		PlanCode:              plan.Code,
		AmountCents:           plan.PriceCents,
		Currency:              plan.Currency,
		Status:                store.PaymentOrderStatusPending,
		ClientReturnURL:       strings.TrimSpace(req.SuccessURL),
	}
	if err := s.store.PaymentOrder().Create(order); err != nil {
		SafeInternalError(c, "Create payment order", err)
		return
	}

	client := infini.NewClient(cfg.KeyID, cfg.SecretKey.String(), cfg.BaseURL, nil)
	createReq := infini.CreateOrderRequest{
		Amount:          formatAmountFromCents(plan.PriceCents),
		RequestID:       requestID,
		ClientReference: merchantOrderID,
		OrderDesc:       strings.TrimSpace(req.OrderDesc),
		SuccessURL:      strings.TrimSpace(req.SuccessURL),
		FailureURL:      strings.TrimSpace(req.FailureURL),
		ExpiresIn:       req.ExpiresInSec,
	}

	resp, raw, err := client.CreateOrder(c.Request.Context(), createReq)
	if err != nil {
		_ = s.store.PaymentOrder().UpdateCreateResponse(order.ID, "", "", store.PaymentOrderStatusFailed, raw, nil)
		SafeError(c, http.StatusBadGateway, "Failed to create payment order", err)
		return
	}

	if err := s.store.PaymentOrder().UpdateCreateResponse(order.ID, resp.OrderID, resp.CheckoutURL, store.PaymentOrderStatusCreated, raw, nil); err != nil {
		SafeInternalError(c, "Update payment order", err)
		return
	}

	updated, err := s.store.PaymentOrder().GetByID(order.ID)
	if err != nil {
		SafeInternalError(c, "Get payment order", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"order":        updated,
		"checkout_url": resp.CheckoutURL,
		"provider":     store.PaymentProviderInfini,
	})
}

func (s *Server) handleListPaymentOrders(c *gin.Context) {
	userID := strings.TrimSpace(c.GetString("user_id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := s.store.PaymentOrder().ListByUser(userID, limit)
	if err != nil {
		SafeInternalError(c, "List payment orders", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) handleGetPaymentOrder(c *gin.Context) {
	userID := strings.TrimSpace(c.GetString("user_id"))
	orderID := strings.TrimSpace(c.Param("id"))
	if orderID == "" {
		SafeBadRequest(c, "order id is required")
		return
	}

	order, err := s.store.PaymentOrder().GetByIDAndUser(orderID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			SafeNotFound(c, "Payment order")
			return
		}
		SafeInternalError(c, "Get payment order", err)
		return
	}

	refresh := strings.EqualFold(c.Query("refresh"), "true")
	if refresh && order.Provider == store.PaymentProviderInfini && order.ProviderOrderID != "" && order.Status != store.PaymentOrderStatusPaid {
		cfg, cfgErr := s.getPaymentProviderConfigForOrder(order)
		if cfgErr == nil {
			client := infini.NewClient(cfg.KeyID, cfg.SecretKey.String(), cfg.BaseURL, nil)
			queryResp, raw, queryErr := client.QueryOrder(c.Request.Context(), order.ProviderOrderID)
			if queryErr == nil {
				mapped := mapInfiniOrderStatus(queryResp.Status, "")
				_ = s.store.PaymentOrder().UpdateStatus(order.ID, mapped, raw)
				if mapped == store.PaymentOrderStatusPaid {
					_ = s.activateMembershipByOrder(c.Request.Context(), order.ID)
				}
				order, _ = s.store.PaymentOrder().GetByIDAndUser(orderID, userID)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"order": order})
}

func (s *Server) handleInfiniWebhook(c *gin.Context) {
	timestamp := strings.TrimSpace(c.GetHeader("X-Webhook-Timestamp"))
	eventID := strings.TrimSpace(c.GetHeader("X-Webhook-Event-Id"))
	signature := strings.TrimSpace(c.GetHeader("X-Webhook-Signature"))
	if timestamp == "" || eventID == "" || signature == "" {
		SafeBadRequest(c, "Missing required webhook headers")
		return
	}

	if !infini.IsWebhookTimestampFresh(timestamp, 5*time.Minute) {
		SafeForbidden(c, "Webhook timestamp expired")
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		SafeBadRequest(c, "Invalid webhook payload")
		return
	}
	payload := strings.TrimSpace(string(body))

	cfg, err := s.resolveWebhookConfig(store.PaymentProviderInfini, timestamp, eventID, payload, signature)
	if err != nil {
		SafeInternalError(c, "Load webhook config", err)
		return
	}
	if cfg == nil {
		SafeForbidden(c, "Invalid webhook signature")
		return
	}

	existing, err := s.store.PaymentWebhookEvent().GetByProviderEventID(store.PaymentProviderInfini, eventID)
	if err == nil && existing != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "duplicate": true})
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		SafeInternalError(c, "Check webhook event", err)
		return
	}

	webhookEvent := &store.PaymentWebhookEvent{
		Provider:  store.PaymentProviderInfini,
		EventID:   eventID,
		Signature: signature,
		Payload:   payload,
		Status:    store.PaymentWebhookStatusPending,
	}
	if err := s.store.PaymentWebhookEvent().Create(webhookEvent); err != nil {
		// Concurrent duplicate insert can happen, treat as processed.
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			c.JSON(http.StatusOK, gin.H{"ok": true, "duplicate": true})
			return
		}
		SafeInternalError(c, "Create webhook event", err)
		return
	}

	var msg infiniWebhookPayload
	if err := json.Unmarshal(body, &msg); err != nil {
		_ = s.store.PaymentWebhookEvent().MarkProcessed(webhookEvent.ID, store.PaymentWebhookStatusFailed, "invalid json payload")
		SafeBadRequest(c, "Invalid webhook payload")
		return
	}

	webhookEvent.EventType = strings.TrimSpace(msg.Event)
	_ = s.store.PaymentWebhookEvent().MarkProcessed(webhookEvent.ID, store.PaymentWebhookStatusPending, "accepted")

	if err := s.applyInfiniWebhook(msg, payload); err != nil {
		_ = s.store.PaymentWebhookEvent().MarkProcessed(webhookEvent.ID, store.PaymentWebhookStatusFailed, err.Error())
		SafeInternalError(c, "Process webhook", err)
		return
	}

	_ = s.store.PaymentWebhookEvent().MarkProcessed(webhookEvent.ID, store.PaymentWebhookStatusProcessed, "ok")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) applyInfiniWebhook(msg infiniWebhookPayload, rawPayload string) error {
	event := strings.ToLower(strings.TrimSpace(msg.Event))
	if !strings.HasPrefix(event, "order.") {
		return nil
	}

	providerOrderID := strings.TrimSpace(msg.OrderID)
	if providerOrderID == "" {
		return fmt.Errorf("webhook order_id is required")
	}

	order, err := s.store.PaymentOrder().GetByProviderOrderID(store.PaymentProviderInfini, providerOrderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	nextStatus := mapInfiniOrderStatus(msg.Status, event)
	if err := s.store.PaymentOrder().UpdateStatus(order.ID, nextStatus, rawPayload); err != nil {
		return err
	}

	if nextStatus == store.PaymentOrderStatusPaid {
		return s.activateMembershipByOrder(context.Background(), order.ID)
	}
	return nil
}

func (s *Server) activateMembershipByOrder(ctx context.Context, orderID string) error {
	_ = ctx
	order, err := s.store.PaymentOrder().GetByID(orderID)
	if err != nil {
		return err
	}
	if order.BizType != store.PaymentBizMembership {
		return nil
	}
	if strings.TrimSpace(order.UserID) == "" || strings.TrimSpace(order.PlanCode) == "" {
		return fmt.Errorf("invalid membership order data")
	}
	plan, err := s.store.MembershipPlan().GetByCode(order.PlanCode)
	if err != nil {
		return err
	}

	return s.store.Transaction(func(tx *gorm.DB) error {
		var current store.PaymentOrder
		if err := tx.Where("id = ?", order.ID).First(&current).Error; err != nil {
			return err
		}
		if current.Status != store.PaymentOrderStatusPaid {
			now := time.Now().UTC()
			if err := tx.Model(&store.PaymentOrder{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
				"status":  store.PaymentOrderStatusPaid,
				"paid_at": now,
			}).Error; err != nil {
				return err
			}
		}

		userMembershipStore := store.NewUserMembershipStore(tx)
		_, actErr := userMembershipStore.ActivateByPayment(tx, order.UserID, plan.Code, order.ID, plan.BillingCycle, time.Now().UTC())
		return actErr
	})
}

func (s *Server) handleAdminListMembershipPlans(c *gin.Context) {
	items, err := s.store.MembershipPlan().List(false)
	if err != nil {
		SafeInternalError(c, "List membership plans", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) handleAdminUpsertMembershipPlan(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	if code == "" {
		SafeBadRequest(c, "plan code is required")
		return
	}
	var req struct {
		Name            string `json:"name" binding:"required"`
		Description     string `json:"description"`
		PriceCents      int64  `json:"price_cents" binding:"required"`
		Currency        string `json:"currency"`
		BillingCycle    string `json:"billing_cycle"`
		RevenueShareBps int    `json:"revenue_share_bps"`
		SeatLimit       int    `json:"seat_limit"`
		Enabled         bool   `json:"enabled"`
		SortOrder       int    `json:"sort_order"`
		Entitlements    string `json:"entitlements"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	plan := &store.MembershipPlan{
		Code:            code,
		Name:            req.Name,
		Description:     req.Description,
		PriceCents:      req.PriceCents,
		Currency:        req.Currency,
		BillingCycle:    req.BillingCycle,
		RevenueShareBps: req.RevenueShareBps,
		SeatLimit:       req.SeatLimit,
		Enabled:         req.Enabled,
		SortOrder:       req.SortOrder,
		Entitlements:    req.Entitlements,
	}
	if err := s.store.MembershipPlan().Upsert(plan); err != nil {
		SafeInternalError(c, "Upsert membership plan", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Membership plan updated"})
}

func (s *Server) handleAdminListPaymentProviderConfigs(c *gin.Context) {
	provider := strings.TrimSpace(c.Param("provider"))
	items, err := s.store.PaymentProviderConfig().ListByProvider(provider)
	if err != nil {
		SafeInternalError(c, "List payment provider configs", err)
		return
	}
	resp := make([]gin.H, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		resp = append(resp, gin.H{
			"id":                 item.ID,
			"provider":           item.Provider,
			"environment":        item.Environment,
			"display_name":       item.DisplayName,
			"base_url":           item.BaseURL,
			"key_id":             item.KeyID,
			"enabled":            item.Enabled,
			"is_default":         item.IsDefault,
			"version":            item.Version,
			"has_secret_key":     strings.TrimSpace(item.SecretKey.String()) != "",
			"has_webhook_secret": strings.TrimSpace(item.WebhookSecret.String()) != "",
			"created_at":         item.CreatedAt,
			"updated_at":         item.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": resp})
}

func (s *Server) handleAdminUpsertPaymentProviderConfig(c *gin.Context) {
	provider := strings.TrimSpace(c.Param("provider"))
	var req struct {
		ID            string `json:"id"`
		Environment   string `json:"environment"`
		DisplayName   string `json:"display_name"`
		BaseURL       string `json:"base_url" binding:"required"`
		KeyID         string `json:"key_id" binding:"required"`
		SecretKey     string `json:"secret_key"`
		WebhookSecret string `json:"webhook_secret"`
		Enabled       bool   `json:"enabled"`
		IsDefault     bool   `json:"is_default"`
		Version       int    `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	var existing *store.PaymentProviderConfig
	var err error
	if strings.TrimSpace(req.ID) != "" {
		existing, err = s.store.PaymentProviderConfig().GetByID(req.ID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			SafeInternalError(c, "Get provider config", err)
			return
		}
	}

	cfg := &store.PaymentProviderConfig{
		ID:          strings.TrimSpace(req.ID),
		Provider:    provider,
		Environment: req.Environment,
		DisplayName: req.DisplayName,
		BaseURL:     req.BaseURL,
		KeyID:       req.KeyID,
		Enabled:     req.Enabled,
		IsDefault:   req.IsDefault,
		Version:     req.Version,
	}

	if strings.TrimSpace(req.SecretKey) != "" {
		cfg.SecretKey = crypto.EncryptedString(req.SecretKey)
	} else if existing != nil {
		cfg.SecretKey = existing.SecretKey
	}
	if strings.TrimSpace(req.WebhookSecret) != "" {
		cfg.WebhookSecret = crypto.EncryptedString(req.WebhookSecret)
	} else if existing != nil {
		cfg.WebhookSecret = existing.WebhookSecret
	}
	if strings.TrimSpace(cfg.SecretKey.String()) == "" {
		SafeBadRequest(c, "secret_key is required")
		return
	}
	if strings.TrimSpace(cfg.WebhookSecret.String()) == "" {
		SafeBadRequest(c, "webhook_secret is required")
		return
	}

	updated, err := s.store.PaymentProviderConfig().Upsert(cfg)
	if err != nil {
		SafeInternalError(c, "Upsert payment provider config", err)
		return
	}
	s.invalidatePaymentProviderCache(provider)

	c.JSON(http.StatusOK, gin.H{
		"id":          updated.ID,
		"provider":    updated.Provider,
		"environment": updated.Environment,
		"base_url":    updated.BaseURL,
		"key_id":      updated.KeyID,
		"enabled":     updated.Enabled,
		"is_default":  updated.IsDefault,
		"version":     updated.Version,
	})
}

func (s *Server) getActivePaymentProviderConfig(provider string) (*store.PaymentProviderConfig, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	now := time.Now().UTC()

	s.paymentCfgMu.RLock()
	entry, ok := s.paymentCfgCache[provider]
	s.paymentCfgMu.RUnlock()
	if ok && entry.config != nil && now.Before(entry.expiresAt) {
		cloned := *entry.config
		return &cloned, nil
	}

	cfg, err := s.store.PaymentProviderConfig().GetActive(provider)
	if err != nil {
		return nil, err
	}

	s.paymentCfgMu.Lock()
	s.paymentCfgCache[provider] = paymentConfigCacheEntry{
		config:    cfg,
		expiresAt: now.Add(paymentConfigCacheTTL),
	}
	s.paymentCfgMu.Unlock()

	cloned := *cfg
	return &cloned, nil
}

func (s *Server) getPaymentProviderConfigForOrder(order *store.PaymentOrder) (*store.PaymentProviderConfig, error) {
	if order == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if strings.TrimSpace(order.ProviderConfigID) != "" {
		cfg, err := s.store.PaymentProviderConfig().GetByID(order.ProviderConfigID)
		if err == nil {
			return cfg, nil
		}
	}
	return s.getActivePaymentProviderConfig(order.Provider)
}

func (s *Server) invalidatePaymentProviderCache(provider string) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	s.paymentCfgMu.Lock()
	defer s.paymentCfgMu.Unlock()
	if provider == "" {
		s.paymentCfgCache = make(map[string]paymentConfigCacheEntry)
		return
	}
	delete(s.paymentCfgCache, provider)
}

func (s *Server) resolveWebhookConfig(provider, timestamp, eventID, payload, signature string) (*store.PaymentProviderConfig, error) {
	items, err := s.store.PaymentProviderConfig().ListByProvider(provider)
	if err != nil {
		return nil, err
	}
	for _, cfg := range items {
		if cfg == nil || !cfg.Enabled {
			continue
		}
		if infini.VerifyWebhookSignature(cfg.WebhookSecret.String(), timestamp, eventID, payload, signature) {
			return cfg, nil
		}
	}
	return nil, nil
}

func mapInfiniOrderStatus(status, event string) string {
	event = strings.ToLower(strings.TrimSpace(event))
	status = strings.ToLower(strings.TrimSpace(status))

	if event == "order.completed" {
		return store.PaymentOrderStatusPaid
	}
	if event == "order.expired" && status == "partial_paid" {
		return store.PaymentOrderStatusPartial
	}
	if event == "order.late_payment" {
		return store.PaymentOrderStatusPaid
	}

	switch status {
	case "pending":
		return store.PaymentOrderStatusPending
	case "processing":
		return store.PaymentOrderStatusCreated
	case "paid":
		return store.PaymentOrderStatusPaid
	case "partial_paid":
		return store.PaymentOrderStatusPartial
	case "expired":
		return store.PaymentOrderStatusExpired
	default:
		return store.PaymentOrderStatusUnknown
	}
}

func formatAmountFromCents(cents int64) string {
	amount := float64(cents) / 100.0
	return strconv.FormatFloat(amount, 'f', 2, 64)
}
