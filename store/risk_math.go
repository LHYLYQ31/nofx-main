package store

import "strings"

// CalculateRiskReward computes reward/risk ratio at a concrete entry price.
// Side accepts values like: long/short/open_long/open_short.
func CalculateRiskReward(side string, entryPrice, stopLoss, takeProfit float64) (ratio, riskPct, rewardPct float64, ok bool) {
	if entryPrice <= 0 || stopLoss <= 0 || takeProfit <= 0 {
		return 0, 0, 0, false
	}
	s := strings.ToLower(strings.TrimSpace(side))
	isLong := s == "long" || s == "open_long"
	isShort := s == "short" || s == "open_short"
	if !isLong && !isShort {
		return 0, 0, 0, false
	}

	if isLong {
		if !(stopLoss < entryPrice && entryPrice < takeProfit) {
			return 0, 0, 0, false
		}
		riskPct = (entryPrice - stopLoss) / entryPrice * 100
		rewardPct = (takeProfit - entryPrice) / entryPrice * 100
	} else {
		if !(takeProfit < entryPrice && entryPrice < stopLoss) {
			return 0, 0, 0, false
		}
		riskPct = (stopLoss - entryPrice) / entryPrice * 100
		rewardPct = (entryPrice - takeProfit) / entryPrice * 100
	}
	if riskPct <= 0 {
		return 0, riskPct, rewardPct, false
	}
	return rewardPct / riskPct, riskPct, rewardPct, true
}

// AdjustTakeProfitForMinRR returns the take-profit price needed to satisfy minRR.
// Side accepts values like: long/short/open_long/open_short.
func AdjustTakeProfitForMinRR(side string, entryPrice, stopLoss, minRR float64) (float64, bool) {
	if entryPrice <= 0 || stopLoss <= 0 || minRR <= 0 {
		return 0, false
	}
	s := strings.ToLower(strings.TrimSpace(side))
	isLong := s == "long" || s == "open_long"
	isShort := s == "short" || s == "open_short"
	if !isLong && !isShort {
		return 0, false
	}
	if isLong {
		risk := entryPrice - stopLoss
		if risk <= 0 {
			return 0, false
		}
		return entryPrice + risk*minRR, true
	}
	risk := stopLoss - entryPrice
	if risk <= 0 {
		return 0, false
	}
	return entryPrice - risk*minRR, true
}
