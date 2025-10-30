package hyperliquid

import (
	"context"
	"encoding/json"
)

// GetUserState retrieves perpetual clearinghouse state for a user.
func (e *Exchange) GetUserState(ctx context.Context, user, dex string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
		Dex  string `json:"dex,omitempty"`
	}{
		Type: "clearinghouseState",
		User: user,
	}
	if dex != "" {
		req.Dex = dex
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetSpotUserState retrieves spot clearinghouse information for a user.
func (e *Exchange) GetSpotUserState(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "spotClearinghouseState",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetOpenOrders retrieves open orders for a user.
func (e *Exchange) GetOpenOrders(ctx context.Context, user, dex string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
		Dex  string `json:"dex,omitempty"`
	}{
		Type: "openOrders",
		User: user,
	}
	if dex != "" {
		req.Dex = dex
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetFrontendOpenOrders retrieves frontend open orders for a user.
func (e *Exchange) GetFrontendOpenOrders(ctx context.Context, user, dex string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
		Dex  string `json:"dex,omitempty"`
	}{
		Type: "frontendOpenOrders",
		User: user,
	}
	if dex != "" {
		req.Dex = dex
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetAllMids retrieves mid prices for all coins.
func (e *Exchange) GetAllMids(ctx context.Context, dex string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		Dex  string `json:"dex,omitempty"`
	}{
		Type: "allMids",
	}
	if dex != "" {
		req.Dex = dex
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetRecentPublicTrades retrieves recent public trades for a coin.
func (e *Exchange) GetRecentPublicTrades(ctx context.Context, coin string, limit *int, startTime, endTime *int64) (json.RawMessage, error) {
	req := struct {
		Type      string `json:"type"`
		Coin      string `json:"coin"`
		Limit     *int   `json:"n,omitempty"`
		StartTime *int64 `json:"startTime,omitempty"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{
		Type:      "recentTrades",
		Coin:      coin,
		Limit:     limit,
		StartTime: startTime,
		EndTime:   endTime,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserFills retrieves recent fills for a user.
func (e *Exchange) GetUserFills(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userFills",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserFillsByTime retrieves user fills within a timeframe.
func (e *Exchange) GetUserFillsByTime(ctx context.Context, user string, startTime int64, endTime *int64, aggregate bool) (json.RawMessage, error) {
	req := struct {
		Type            string `json:"type"`
		User            string `json:"user"`
		StartTime       int64  `json:"startTime"`
		EndTime         *int64 `json:"endTime,omitempty"`
		AggregateByTime bool   `json:"aggregateByTime"`
	}{
		Type:            "userFillsByTime",
		User:            user,
		StartTime:       startTime,
		EndTime:         endTime,
		AggregateByTime: aggregate,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetMeta retrieves futures metadata.
func (e *Exchange) GetMeta(ctx context.Context, dex string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		Dex  string `json:"dex,omitempty"`
	}{
		Type: "meta",
	}
	if dex != "" {
		req.Dex = dex
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetMetaAndAssetContexts retrieves futures metadata and asset contexts.
func (e *Exchange) GetMetaAndAssetContexts(ctx context.Context) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "metaAndAssetCtxs",
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetPerpDexs retrieves perpetual DEX metadata.
func (e *Exchange) GetPerpDexs(ctx context.Context) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "perpDexs",
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetSpotMeta retrieves spot metadata.
func (e *Exchange) GetSpotMeta(ctx context.Context) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "spotMeta",
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetSpotMetaAndAssetContexts retrieves spot metadata and asset contexts.
func (e *Exchange) GetSpotMetaAndAssetContexts(ctx context.Context) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "spotMetaAndAssetCtxs",
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetFundingHistory retrieves funding history for a coin.
func (e *Exchange) GetFundingHistory(ctx context.Context, coin string, startTime int64, endTime *int64) (json.RawMessage, error) {
	req := struct {
		Type      string `json:"type"`
		Coin      string `json:"coin"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{
		Type:      "fundingHistory",
		Coin:      coin,
		StartTime: startTime,
		EndTime:   endTime,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserFundingHistory retrieves a user's funding history.
func (e *Exchange) GetUserFundingHistory(ctx context.Context, user string, startTime int64, endTime *int64) (json.RawMessage, error) {
	req := struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{
		Type:      "userFunding",
		User:      user,
		StartTime: startTime,
		EndTime:   endTime,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserNonFundingLedgerUpdates retrieves non-funding ledger updates for a user.
func (e *Exchange) GetUserNonFundingLedgerUpdates(ctx context.Context, user string, startTime int64, endTime *int64) (json.RawMessage, error) {
	req := struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{
		Type:      "userNonFundingLedgerUpdates",
		User:      user,
		StartTime: startTime,
		EndTime:   endTime,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetL2Snapshot retrieves an orderbook snapshot.
func (e *Exchange) GetL2Snapshot(ctx context.Context, coin string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		Coin string `json:"coin"`
	}{
		Type: "l2Book",
		Coin: coin,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetCandleSnapshot retrieves candle data for a coin.
func (e *Exchange) GetCandleSnapshot(ctx context.Context, coin, interval string, startTime, endTime int64) (json.RawMessage, error) {
	req := struct {
		Type string                 `json:"type"`
		Req  map[string]interface{} `json:"req"`
	}{
		Type: "candleSnapshot",
		Req: map[string]interface{}{
			"coin":      coin,
			"interval":  interval,
			"startTime": startTime,
			"endTime":   endTime,
		},
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserFees retrieves user fee information.
func (e *Exchange) GetUserFees(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userFees",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserStakingSummary retrieves staking summary.
func (e *Exchange) GetUserStakingSummary(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "delegatorSummary",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserStakingDelegations retrieves staking delegations.
func (e *Exchange) GetUserStakingDelegations(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "delegations",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserStakingRewards retrieves staking rewards.
func (e *Exchange) GetUserStakingRewards(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "delegatorRewards",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetDelegatorHistory retrieves delegator history.
func (e *Exchange) GetDelegatorHistory(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "delegatorHistory",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetOrderStatusByOID retrieves order status by order ID.
func (e *Exchange) GetOrderStatusByOID(ctx context.Context, user string, oid int64) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
		OID  int64  `json:"oid"`
	}{
		Type: "orderStatus",
		User: user,
		OID:  oid,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetOrderStatusByCloid retrieves order status by client order ID hex string.
func (e *Exchange) GetOrderStatusByCloid(ctx context.Context, user, cloid string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
		OID  string `json:"oid"`
	}{
		Type: "orderStatus",
		User: user,
		OID:  cloid,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetReferralState retrieves referral state information.
func (e *Exchange) GetReferralState(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "referral",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetSubAccounts retrieves sub-account data.
func (e *Exchange) GetSubAccounts(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "subAccounts",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserToMultiSigSigners retrieves multi-sig signers for a user.
func (e *Exchange) GetUserToMultiSigSigners(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userToMultiSigSigners",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetPerpDeployAuctionStatus retrieves perpetual deploy auction status.
func (e *Exchange) GetPerpDeployAuctionStatus(ctx context.Context) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "perpDeployAuctionStatus",
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserDexAbstraction retrieves a user's DEX abstraction state.
func (e *Exchange) GetUserDexAbstraction(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userDexAbstraction",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetHistoricalOrders retrieves historical orders for a user.
func (e *Exchange) GetHistoricalOrders(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "historicalOrders",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserPortfolio retrieves a user's portfolio data.
func (e *Exchange) GetUserPortfolio(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "portfolio",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserTwapSliceFills retrieves TWAP slice fills.
func (e *Exchange) GetUserTwapSliceFills(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userTwapSliceFills",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserVaultEquities retrieves vault equity data.
func (e *Exchange) GetUserVaultEquities(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userVaultEquities",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserRole retrieves a user's role information.
func (e *Exchange) GetUserRole(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userRole",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetUserRateLimit retrieves API rate limit details.
func (e *Exchange) GetUserRateLimit(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userRateLimit",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetSpotDeployState retrieves spot deploy state.
func (e *Exchange) GetSpotDeployState(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "spotDeployState",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}

// GetExtraAgents retrieves extra agent information.
func (e *Exchange) GetExtraAgents(ctx context.Context, user string) (json.RawMessage, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "extraAgents",
		User: user,
	}
	var resp json.RawMessage
	return resp, e.sendInfo(ctx, req, &resp)
}
