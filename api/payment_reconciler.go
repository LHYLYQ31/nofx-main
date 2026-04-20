package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"nofx/logger"
	"nofx/store"
)

func (s *Server) StartPaymentOrderReconciler(enabled bool, interval, staleAge time.Duration, batchSize int) func() {
	if s == nil || s.store == nil || !enabled {
		return func() {}
	}
	if interval <= 0 {
		interval = time.Minute
	}
	if staleAge <= 0 {
		staleAge = 2 * time.Minute
	}
	if batchSize <= 0 || batchSize > 500 {
		batchSize = 50
	}

	logger.Infof("Payment order reconciler started (interval=%s, stale_age=%s, batch_size=%d)", interval, staleAge, batchSize)

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	var once sync.Once

	go func() {
		defer close(doneCh)

		s.reconcileInfiniUnsettledOrders(staleAge, batchSize)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				s.reconcileInfiniUnsettledOrders(staleAge, batchSize)
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(stopCh)
		})
		select {
		case <-doneCh:
			logger.Info("Payment order reconciler stopped")
		case <-time.After(5 * time.Second):
			logger.Warn("Timed out while stopping payment order reconciler")
		}
	}
}

func (s *Server) reconcileInfiniUnsettledOrders(staleAge time.Duration, batchSize int) {
	cutoff := time.Now().UTC().Add(-staleAge)
	orders, err := s.store.PaymentOrder().ListUnsettledForReconcile(store.PaymentProviderInfini, cutoff, batchSize)
	if err != nil {
		logger.Warnf("Payment reconcile list failed: %v", err)
		return
	}
	if len(orders) == 0 {
		return
	}

	for _, order := range orders {
		if order == nil {
			continue
		}
		s.reconcileSingleInfiniOrder(order)
	}
}

func (s *Server) reconcileSingleInfiniOrder(order *store.PaymentOrder) {
	if order == nil {
		return
	}
	now := time.Now().UTC()
	if strings.TrimSpace(order.ProviderOrderID) == "" {
		if order.ExpiresAt != nil && now.After(order.ExpiresAt.UTC()) {
			_ = s.store.PaymentOrder().UpdateStatusWithExpiresAt(order.ID, store.PaymentOrderStatusExpired, "reconcile: missing provider order id and local expires_at reached", order.ExpiresAt)
		}
		return
	}

	cfg, err := s.getPaymentProviderConfigForOrder(order)
	if err != nil {
		logger.Warnf("Payment reconcile skipped order=%s: load provider config failed: %v", order.ID, err)
		return
	}
	client := newInfiniClientFromConfig(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	queryResp, raw, err := client.QueryOrder(ctx, order.ProviderOrderID)
	if err != nil {
		if order.ExpiresAt != nil && now.After(order.ExpiresAt.UTC()) {
			_ = s.store.PaymentOrder().UpdateStatusWithExpiresAt(order.ID, store.PaymentOrderStatusExpired, "reconcile fallback: local expires_at reached", order.ExpiresAt)
		}
		logger.Warnf("Payment reconcile query failed order=%s provider_order_id=%s: %v", order.ID, order.ProviderOrderID, err)
		return
	}

	nextStatus := mapInfiniOrderStatus(queryResp.Status, "")
	expiresAt := unixTimestampToUTCPtr(queryResp.ExpiresAt)
	if expiresAt == nil {
		expiresAt = order.ExpiresAt
	}
	if nextStatus == store.PaymentOrderStatusPending && expiresAt != nil && now.After(expiresAt.UTC()) {
		nextStatus = store.PaymentOrderStatusExpired
	}

	if err := s.store.PaymentOrder().UpdateStatusWithExpiresAt(order.ID, nextStatus, raw, expiresAt); err != nil {
		logger.Warnf("Payment reconcile update failed order=%s: %v", order.ID, err)
		return
	}

	if nextStatus == store.PaymentOrderStatusPaid {
		if err := s.activateMembershipByOrder(context.Background(), order.ID); err != nil {
			logger.Warnf("Payment reconcile activate membership failed order=%s: %v", order.ID, err)
		}
	}
}

func unixTimestampToUTCPtr(ts int64) *time.Time {
	if ts <= 0 {
		return nil
	}
	var t time.Time
	// Some upstream APIs return milliseconds, others return seconds.
	if ts >= 1_000_000_000_000 {
		t = time.UnixMilli(ts)
	} else {
		t = time.Unix(ts, 0)
	}
	utc := t.UTC()
	return &utc
}
