package hyperliquid

import (
	"context"
	"time"

	"github.com/thrasher-corp/gocryptotrader/types"
)

// GetUserState retrieves perpetual clearinghouse state for a user.
func (e *Exchange) GetUserState(ctx context.Context, user, dex string) (*UserStateResponse, error) {
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
	resp := new(UserStateResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSpotUserState retrieves spot clearinghouse information for a user.
func (e *Exchange) GetSpotUserState(ctx context.Context, user string) (*SpotUserStateResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "spotClearinghouseState",
		User: user,
	}
	resp := new(SpotUserStateResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetOpenOrders retrieves open orders for a user.
func (e *Exchange) GetOpenOrders(ctx context.Context, user, dex string) ([]OpenOrderResponse, error) {
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
	var resp []OpenOrderResponse
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetFrontendOpenOrders retrieves frontend open orders for a user.
func (e *Exchange) GetFrontendOpenOrders(ctx context.Context, user, dex string) ([]OpenOrderResponse, error) {
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
	var resp []OpenOrderResponse
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetAllMids retrieves mid prices for all coins.
func (e *Exchange) GetAllMids(ctx context.Context, dex string) (map[string]types.Number, error) {
	req := struct {
		Type string `json:"type"`
		Dex  string `json:"dex,omitempty"`
	}{
		Type: "allMids",
	}
	if dex != "" {
		req.Dex = dex
	}
	var resp map[string]types.Number
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetRecentPublicTrades retrieves recent public trades for a coin.
func (e *Exchange) GetRecentPublicTrades(ctx context.Context, coin string, limit *int, startTime, endTime *time.Time) ([]RecentTrade, error) {
	var startMs, endMs *int64
	if startTime != nil {
		ms := startTime.UTC().UnixMilli()
		startMs = &ms
	}
	if endTime != nil {
		ms := endTime.UTC().UnixMilli()
		endMs = &ms
	}
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
		StartTime: startMs,
		EndTime:   endMs,
	}
	var resp []RecentTrade
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserFills retrieves recent fills for a user.
func (e *Exchange) GetUserFills(ctx context.Context, user string) ([]UserFill, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userFills",
		User: user,
	}
	var resp []UserFill
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserFillsByTime retrieves user fills within a timeframe.
func (e *Exchange) GetUserFillsByTime(ctx context.Context, user string, startTime time.Time, endTime *time.Time, aggregate bool) ([]UserFill, error) {
	var endMs *int64
	if endTime != nil {
		ms := endTime.UTC().UnixMilli()
		endMs = &ms
	}
	req := struct {
		Type            string `json:"type"`
		User            string `json:"user"`
		StartTime       int64  `json:"startTime"`
		EndTime         *int64 `json:"endTime,omitempty"`
		AggregateByTime bool   `json:"aggregateByTime"`
	}{
		Type:            "userFillsByTime",
		User:            user,
		StartTime:       startTime.UTC().UnixMilli(),
		EndTime:         endMs,
		AggregateByTime: aggregate,
	}
	var resp []UserFill
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetMeta retrieves futures metadata.
func (e *Exchange) GetMeta(ctx context.Context, dex string) (*MetaResponse, error) {
	req := struct {
		Type string `json:"type"`
		Dex  string `json:"dex,omitempty"`
	}{
		Type: "meta",
	}
	if dex != "" {
		req.Dex = dex
	}
	resp := new(MetaResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetMetaAndAssetContexts retrieves futures metadata and asset contexts.
func (e *Exchange) GetMetaAndAssetContexts(ctx context.Context) (*MetaAndAssetContextsResponse, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "metaAndAssetCtxs",
	}
	resp := new(MetaAndAssetContextsResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPerpDexs retrieves perpetual DEX metadata.
func (e *Exchange) GetPerpDexs(ctx context.Context) ([]PerpDex, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "perpDexs",
	}
	var resp []PerpDex
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSpotMeta retrieves spot metadata.
func (e *Exchange) GetSpotMeta(ctx context.Context) (*SpotMetaResponse, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "spotMeta",
	}
	resp := new(SpotMetaResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSpotMetaAndAssetContexts retrieves spot metadata and asset contexts.
func (e *Exchange) GetSpotMetaAndAssetContexts(ctx context.Context) (*SpotMetaAndAssetContextsResponse, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "spotMetaAndAssetCtxs",
	}
	resp := new(SpotMetaAndAssetContextsResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetFundingHistory retrieves funding history for a coin.
func (e *Exchange) GetFundingHistory(ctx context.Context, coin string, startTime time.Time, endTime *time.Time) ([]FundingHistoryEntry, error) {
	var endMs *int64
	if endTime != nil {
		ms := endTime.UTC().UnixMilli()
		endMs = &ms
	}
	req := struct {
		Type      string `json:"type"`
		Coin      string `json:"coin"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{
		Type:      "fundingHistory",
		Coin:      coin,
		StartTime: startTime.UTC().UnixMilli(),
		EndTime:   endMs,
	}
	var resp []FundingHistoryEntry
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserFundingHistory retrieves a user's funding history.
func (e *Exchange) GetUserFundingHistory(ctx context.Context, user string, startTime time.Time, endTime *time.Time) ([]UserFundingHistoryEntry, error) {
	var endMs *int64
	if endTime != nil {
		ms := endTime.UTC().UnixMilli()
		endMs = &ms
	}
	req := struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{
		Type:      "userFunding",
		User:      user,
		StartTime: startTime.UTC().UnixMilli(),
		EndTime:   endMs,
	}
	var resp []UserFundingHistoryEntry
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserNonFundingLedgerUpdates retrieves non-funding ledger updates for a user.
func (e *Exchange) GetUserNonFundingLedgerUpdates(ctx context.Context, user string, startTime time.Time, endTime *time.Time) ([]UserNonFundingLedgerEntry, error) {
	var endMs *int64
	if endTime != nil {
		ms := endTime.UTC().UnixMilli()
		endMs = &ms
	}
	req := struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{
		Type:      "userNonFundingLedgerUpdates",
		User:      user,
		StartTime: startTime.UTC().UnixMilli(),
		EndTime:   endMs,
	}
	var resp []UserNonFundingLedgerEntry
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetL2Snapshot retrieves an orderbook snapshot.
func (e *Exchange) GetL2Snapshot(ctx context.Context, coin string) (*OrderbookSnapshot, error) {
	req := struct {
		Type string `json:"type"`
		Coin string `json:"coin"`
	}{
		Type: "l2Book",
		Coin: coin,
	}
	resp := new(OrderbookSnapshot)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetCandleSnapshot retrieves candle data for a coin.
func (e *Exchange) GetCandleSnapshot(ctx context.Context, coin, interval string, startTime, endTime time.Time) ([]CandleSnapshot, error) {
	req := struct {
		Type string         `json:"type"`
		Req  map[string]any `json:"req"`
	}{
		Type: "candleSnapshot",
		Req: map[string]any{
			"coin":      coin,
			"interval":  interval,
			"startTime": startTime.UTC().UnixMilli(),
			"endTime":   endTime.UTC().UnixMilli(),
		},
	}
	var resp []CandleSnapshot
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserFees retrieves user fee information.
func (e *Exchange) GetUserFees(ctx context.Context, user string) (*UserFeesResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userFees",
		User: user,
	}
	resp := new(UserFeesResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserStakingSummary retrieves staking summary.
func (e *Exchange) GetUserStakingSummary(ctx context.Context, user string) (*UserStakingSummaryResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "delegatorSummary",
		User: user,
	}
	resp := new(UserStakingSummaryResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserStakingDelegations retrieves staking delegations.
func (e *Exchange) GetUserStakingDelegations(ctx context.Context, user string) ([]UserStakingDelegation, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "delegations",
		User: user,
	}
	var resp []UserStakingDelegation
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserStakingRewards retrieves staking rewards.
func (e *Exchange) GetUserStakingRewards(ctx context.Context, user string) ([]UserStakingReward, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "delegatorRewards",
		User: user,
	}
	var resp []UserStakingReward
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetDelegatorHistory retrieves delegator history.
func (e *Exchange) GetDelegatorHistory(ctx context.Context, user string) ([]DelegatorHistoryEntry, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "delegatorHistory",
		User: user,
	}
	var resp []DelegatorHistoryEntry
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetOrderStatusByOID retrieves order status by order ID.
func (e *Exchange) GetOrderStatusByOID(ctx context.Context, user string, oid int64) (*OrderStatusResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
		OID  int64  `json:"oid"`
	}{
		Type: "orderStatus",
		User: user,
		OID:  oid,
	}
	resp := new(OrderStatusResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetOrderStatusByCloid retrieves order status by client order ID hex string.
func (e *Exchange) GetOrderStatusByCloid(ctx context.Context, user, cloid string) (*OrderStatusResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
		OID  string `json:"oid"`
	}{
		Type: "orderStatus",
		User: user,
		OID:  cloid,
	}
	resp := new(OrderStatusResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetReferralState retrieves referral state information.
func (e *Exchange) GetReferralState(ctx context.Context, user string) (*ReferralStateResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "referral",
		User: user,
	}
	resp := new(ReferralStateResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSubAccounts retrieves sub-account data.
func (e *Exchange) GetSubAccounts(ctx context.Context, user string) ([]SubAccount, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "subAccounts",
		User: user,
	}
	var resp []SubAccount
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserToMultiSigSigners retrieves multi-sig signers for a user.
func (e *Exchange) GetUserToMultiSigSigners(ctx context.Context, user string) ([]UserMultiSigSigner, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userToMultiSigSigners",
		User: user,
	}
	var resp []UserMultiSigSigner
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPerpDeployAuctionStatus retrieves perpetual deploy auction status.
func (e *Exchange) GetPerpDeployAuctionStatus(ctx context.Context) (*PerpDeployAuctionStatusResponse, error) {
	req := struct {
		Type string `json:"type"`
	}{
		Type: "perpDeployAuctionStatus",
	}
	resp := new(PerpDeployAuctionStatusResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserDexAbstraction retrieves a user's DEX abstraction state.
func (e *Exchange) GetUserDexAbstraction(ctx context.Context, user string) (*UserDexAbstractionResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userDexAbstraction",
		User: user,
	}
	resp := new(UserDexAbstractionResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetHistoricalOrders retrieves historical orders for a user.
func (e *Exchange) GetHistoricalOrders(ctx context.Context, user string) ([]HistoricalOrderEntry, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "historicalOrders",
		User: user,
	}
	var resp []HistoricalOrderEntry
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserPortfolio retrieves a user's portfolio data.
func (e *Exchange) GetUserPortfolio(ctx context.Context, user string) ([]PortfolioPeriod, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "portfolio",
		User: user,
	}
	var resp []PortfolioPeriod
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserTwapSliceFills retrieves TWAP slice fills.
func (e *Exchange) GetUserTwapSliceFills(ctx context.Context, user string) ([]UserFill, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userTwapSliceFills",
		User: user,
	}
	var resp []UserFill
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserVaultEquities retrieves vault equity data.
func (e *Exchange) GetUserVaultEquities(ctx context.Context, user string) ([]UserVaultEquity, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userVaultEquities",
		User: user,
	}
	var resp []UserVaultEquity
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserRole retrieves a user's role information.
func (e *Exchange) GetUserRole(ctx context.Context, user string) (*UserRoleResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userRole",
		User: user,
	}
	resp := new(UserRoleResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetUserRateLimit retrieves API rate limit details.
func (e *Exchange) GetUserRateLimit(ctx context.Context, user string) (*UserRateLimitResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "userRateLimit",
		User: user,
	}
	resp := new(UserRateLimitResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSpotDeployState retrieves spot deploy state.
func (e *Exchange) GetSpotDeployState(ctx context.Context, user string) (*SpotDeployStateResponse, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "spotDeployState",
		User: user,
	}
	resp := new(SpotDeployStateResponse)
	if err := e.sendInfo(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetExtraAgents retrieves extra agent information.
func (e *Exchange) GetExtraAgents(ctx context.Context, user string) ([]ExtraAgent, error) {
	req := struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{
		Type: "extraAgents",
		User: user,
	}
	var resp []ExtraAgent
	if err := e.sendInfo(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
