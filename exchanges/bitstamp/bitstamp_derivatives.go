package bitstamp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

// GetMarginTiers returns the margin tiers of all derivatives markets
func (e *Exchange) GetMarginTiers(ctx context.Context) ([]MarginTiersResponse, error) {
	var resp []MarginTiersResponse
	return resp, e.SendHTTPRequest(ctx, exchange.RestSpot, "/v2/margin_tiers/", nil, &resp)
}

// GetMarketHours returns the reference index publishing schedules of all derivatives markets for the next 30 days
func (e *Exchange) GetMarketHours(ctx context.Context) ([]MarketHoursResponse, error) {
	var resp []MarketHoursResponse
	return resp, e.SendHTTPRequest(ctx, exchange.RestSpot, "/v2/derivatives/market_hours/", nil, &resp)
}

// GetMarketHoursForMarket returns the reference index publishing schedule of a derivatives market for the next 30 days
// Markets which do not follow a publishing schedule are not found
func (e *Exchange) GetMarketHoursForMarket(ctx context.Context, pair currency.Pair) (*MarketHoursResponse, error) {
	if pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	var resp *MarketHoursResponse
	return resp, e.SendHTTPRequest(ctx, exchange.RestSpot, "/v2/derivatives/market_hours/"+formatMarketSymbol(pair)+"/", nil, &resp)
}

// GetOpenPositions returns the account's open derivatives positions
// pair is optional and restricts the results to a single market
func (e *Exchange) GetOpenPositions(ctx context.Context, pair currency.Pair) ([]PositionResponse, error) {
	path := "/v2/open_positions/"
	if !pair.IsEmpty() {
		path += formatMarketSymbol(pair) + "/"
	}
	var resp []PositionResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, path, nil, nil, &resp)
}

// GetPositionStatus returns the status of an open or closed derivatives position
func (e *Exchange) GetPositionStatus(ctx context.Context, positionID string) (*PositionStatusResponse, error) {
	if positionID == "" {
		return nil, errPositionIDRequired
	}
	var resp *PositionStatusResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/v2/position_status/"+url.PathEscape(positionID)+"/", nil, nil, &resp)
}

// GetPositionHistory returns the account's historical derivatives positions
func (e *Exchange) GetPositionHistory(ctx context.Context, req *PositionHistoryRequest) ([]PositionHistoryResponse, error) {
	if err := common.NilGuard(req); err != nil {
		return nil, err
	}
	if req.Limit > maxResultLimit {
		return nil, fmt.Errorf("%w: %d must not exceed %d", errInvalidLimit, req.Limit, maxResultLimit)
	}
	params := url.Values{}
	if req.Offset != 0 {
		params.Set("offset", strconv.FormatUint(req.Offset, 10))
	}
	if req.Limit != 0 {
		params.Set("limit", strconv.FormatUint(req.Limit, 10))
	}
	if req.SinceID != "" {
		params.Set("since_id", req.SinceID)
	}
	if req.Sort != "" {
		params.Set("sort", req.Sort)
	}
	path := "/v2/position_history/"
	if !req.Pair.IsEmpty() {
		path += formatMarketSymbol(req.Pair) + "/"
	}
	var resp []PositionHistoryResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, path, params, nil, &resp)
}

// ClosePositions closes the account's derivatives positions with market orders
func (e *Exchange) ClosePositions(ctx context.Context, req *ClosePositionsRequest) (*ClosePositionsResponse, error) {
	if err := common.NilGuard(req); err != nil {
		return nil, err
	}
	body := &closePositionsBody{
		MarginMode: req.MarginMode,
		OrderType:  OrderSubtypeMarket,
	}
	if !req.Pair.IsEmpty() {
		body.Market = formatMarketName(req.Pair)
	}
	var resp *ClosePositionsResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/v2/close_positions/", nil, body, &resp)
}

// ClosePosition closes a derivatives position with a market order
func (e *Exchange) ClosePosition(ctx context.Context, positionID string) (*ClosedPositionResponse, error) {
	if positionID == "" {
		return nil, errPositionIDRequired
	}
	var resp *ClosedPositionResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/v2/close_position/", nil, &closePositionBody{PositionID: positionID}, &resp)
}

// GetPositionSettlementTransactions returns the account's position settlement transactions from up to 30 days ago
func (e *Exchange) GetPositionSettlementTransactions(ctx context.Context, req *SettlementTransactionsRequest) ([]SettlementTransactionResponse, error) {
	if err := common.NilGuard(req); err != nil {
		return nil, err
	}
	if req.Limit > maxResultLimit {
		return nil, fmt.Errorf("%w: %d must not exceed %d", errInvalidLimit, req.Limit, maxResultLimit)
	}
	if !req.Since.IsZero() && !req.Until.IsZero() && req.Since.After(req.Until) {
		return nil, common.ErrStartAfterEnd
	}
	params := url.Values{}
	if req.Offset != 0 {
		params.Set("offset", strconv.FormatUint(req.Offset, 10))
	}
	if req.Limit != 0 {
		params.Set("limit", strconv.FormatUint(req.Limit, 10))
	}
	if req.SinceID != "" {
		params.Set("since_id", req.SinceID)
	}
	if req.Sort != "" {
		params.Set("sort", req.Sort)
	}
	if !req.Since.IsZero() {
		params.Set("since_timestamp", strconv.FormatInt(req.Since.Unix(), 10))
	}
	if !req.Until.IsZero() {
		params.Set("until_timestamp", strconv.FormatInt(req.Until.Unix(), 10))
	}
	path := "/v2/position_settlement_transactions/"
	if req.TransactionID != "" {
		path += url.PathEscape(req.TransactionID) + "/"
	}
	var resp []SettlementTransactionResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, path, params, nil, &resp)
}

// GetDerivativesTradeHistory returns the account's derivatives trades for a market or order
func (e *Exchange) GetDerivativesTradeHistory(ctx context.Context, req *DerivativesTradeHistoryRequest) ([]DerivativesTradeResponse, error) {
	if err := common.NilGuard(req); err != nil {
		return nil, err
	}
	if req.Pair.IsEmpty() && req.OrderID == 0 {
		return nil, errPairOrOrderIDRequired
	}
	if req.Limit > maxResultLimit {
		return nil, fmt.Errorf("%w: %d must not exceed %d", errInvalidLimit, req.Limit, maxResultLimit)
	}
	if !req.Since.IsZero() && !req.Until.IsZero() && req.Since.After(req.Until) {
		return nil, common.ErrStartAfterEnd
	}
	params := url.Values{}
	if req.Limit != 0 {
		params.Set("limit", strconv.FormatUint(req.Limit, 10))
	}
	if req.Sort != "" {
		params.Set("sort", req.Sort)
	}
	if req.OrderID != 0 {
		params.Set("order_id", strconv.FormatUint(req.OrderID, 10))
	}
	if !req.Since.IsZero() {
		params.Set("since_timestamp", strconv.FormatInt(req.Since.Unix(), 10))
	}
	if !req.Until.IsZero() {
		params.Set("until_timestamp", strconv.FormatInt(req.Until.Unix(), 10))
	}
	if req.AfterID != "" {
		params.Set("after_id", req.AfterID)
	}
	path := "/v2/trade_history/"
	if !req.Pair.IsEmpty() {
		path += formatMarketSymbol(req.Pair) + "/"
	}
	var resp []DerivativesTradeResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, path, params, nil, &resp)
}

// GetMarginInfo returns the account's derivatives margin information
func (e *Exchange) GetMarginInfo(ctx context.Context) (*MarginInfoResponse, error) {
	var resp *MarginInfoResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/v2/margin_info/", nil, nil, &resp)
}

// GetEstimatedOrderImpact returns the estimated margin impact of placing an order on a derivatives market
func (e *Exchange) GetEstimatedOrderImpact(ctx context.Context, req *OrderImpactRequest) (*OrderImpactResponse, error) {
	if err := common.NilGuard(req); err != nil {
		return nil, err
	}
	if req.Pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if req.OrderType == "" {
		return nil, errOrderTypeRequired
	}
	if req.Amount <= 0 {
		return nil, order.ErrAmountIsInvalid
	}
	side, err := formatOrderSide(req.Side)
	if err != nil {
		return nil, err
	}
	if req.MarginMode == "" {
		return nil, errMarginModeRequired
	}
	if req.Leverage <= 0 {
		return nil, errLeverageRequired
	}
	body := &orderImpactBody{
		Market:               formatMarketName(req.Pair),
		OrderType:            req.OrderType,
		Amount:               req.Amount,
		OrderSide:            strings.ToUpper(side),
		MarginMode:           req.MarginMode,
		Leverage:             req.Leverage,
		Price:                req.Price,
		ReduceOnly:           req.ReduceOnly,
		AdditionalCollateral: formatCollateral(req.AdditionalCollateral),
	}
	var resp *OrderImpactResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/v2/estimated_order_impact/", nil, body, &resp)
}

// GetCollateralChangeImpact returns the estimated impact of changing collateral on open positions
func (e *Exchange) GetCollateralChangeImpact(ctx context.Context, req *CollateralChangeImpactRequest) (*CollateralChangeImpactResponse, error) {
	if err := common.NilGuard(req); err != nil {
		return nil, err
	}
	if req.MarginMode == "" {
		return nil, errMarginModeRequired
	}
	if (len(req.TargetCollateral) == 0) == (len(req.CollateralDeltas) == 0) {
		return nil, errCollateralRequired
	}
	body := &collateralChangeImpactBody{
		MarginMode:       req.MarginMode,
		TargetCollateral: formatCollateral(req.TargetCollateral),
		CollateralDeltas: formatCollateral(req.CollateralDeltas),
	}
	if !req.Pair.IsEmpty() {
		body.Market = formatMarketName(req.Pair)
	}
	var resp *CollateralChangeImpactResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/v2/collateral_change_impact/", nil, body, &resp)
}

// GetCollateralCurrencies returns the currencies which may be used as collateral and their haircuts
func (e *Exchange) GetCollateralCurrencies(ctx context.Context) ([]CollateralCurrencyResponse, error) {
	var resp []CollateralCurrencyResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/v2/collateral_currencies/", nil, nil, &resp)
}

// AdjustPositionCollateral sets the collateral amount of an isolated margin position
func (e *Exchange) AdjustPositionCollateral(ctx context.Context, positionID string, newAmount float64) error {
	if positionID == "" {
		return errPositionIDRequired
	}
	if newAmount <= 0 {
		return order.ErrAmountIsInvalid
	}
	body := &adjustPositionCollateralBody{
		PositionID: positionID,
		NewAmount:  newAmount,
	}
	return e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/v2/adjust_position_collateral/", nil, body, nil)
}

// GetLeverageSettings returns the account's leverage settings for derivatives markets
// marginMode and pair are optional filters
func (e *Exchange) GetLeverageSettings(ctx context.Context, marginMode string, pair currency.Pair) ([]LeverageSettingResponse, error) {
	params := url.Values{}
	if marginMode != "" {
		params.Set("margin_mode", marginMode)
	}
	if !pair.IsEmpty() {
		params.Set("market", formatMarketName(pair))
	}
	var resp []LeverageSettingResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/v2/leverage_settings/", params, nil, &resp)
}

// UpdateLeverageSetting sets the leverage used for a derivatives market and margin mode
func (e *Exchange) UpdateLeverageSetting(ctx context.Context, req *LeverageSettingRequest) (*LeverageSettingResponse, error) {
	if err := common.NilGuard(req); err != nil {
		return nil, err
	}
	if req.Pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if req.MarginMode == "" {
		return nil, errMarginModeRequired
	}
	if req.Leverage <= 0 {
		return nil, errLeverageRequired
	}
	body := &leverageSettingBody{
		MarginMode: req.MarginMode,
		Market:     formatMarketName(req.Pair),
		Leverage:   req.Leverage,
	}
	var resp *LeverageSettingResponse
	return resp, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/v2/leverage_settings/", nil, body, &resp)
}
