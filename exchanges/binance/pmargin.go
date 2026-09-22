package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Definitions and Terminology
// Portfolio Margin is an advanced trading mode offered by Binance, designed for experienced traders who seek
// increased leverage and flexibility across various trading products. It incorporates a unique approach to margin
// calculations and risk management to offer a more comprehensive assessment of the trader's overall exposure.

// - Terminology
// Margin refers to Cross Margin
// UM refers to USD-M Futures
// CM refers to Coin-M Futures

// NewUMOrder send in a new USDT margined order/orders.
func (e *Exchange) NewUMOrder(ctx context.Context, arg *UMOrderRequest) (*UMCMOrder, error) {
	return e.newUMCMOrder(ctx, arg, "/papi/v1/um/order")
}

// NewCMOrder send in a new Coin margined order/orders.
func (e *Exchange) NewCMOrder(ctx context.Context, arg *UMOrderRequest) (*UMCMOrder, error) {
	return e.newUMCMOrder(ctx, arg, "/papi/v1/cm/order")
}

func (e *Exchange) newUMCMOrder(ctx context.Context, arg *UMOrderRequest, path string) (*UMCMOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if path == "/papi/v1/cm/order" && (!arg.GoodTillDate.IsZero() || arg.GoodTillDateTimestamp != 0 || arg.SelfTradePreventionMode != "") {
		return nil, fmt.Errorf("%w: goodTillDate and selfTradePreventionMode are UM-only", errUnsupportedParameter)
	}
	if !arg.GoodTillDate.IsZero() {
		arg.GoodTillDateTimestamp = arg.GoodTillDate.UnixMilli()
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.OrderType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	arg.OrderType = strings.ToUpper(arg.OrderType)
	switch arg.OrderType {
	case order.Limit.String():
		if arg.TimeInForce == "" {
			return nil, order.ErrInvalidTimeInForce
		}
		if arg.Quantity <= 0 {
			return nil, limits.ErrAmountBelowMin
		}
		if arg.Price <= 0 && arg.PriceMatch == "" {
			return nil, limits.ErrPriceBelowMin
		}
	case "MARKET":
		if arg.Quantity <= 0 {
			return nil, limits.ErrAmountBelowMin
		}
	default:
		return nil, order.ErrUnsupportedOrderType
	}
	var resp *UMCMOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, path, nil, pmDefaultRate, arg, &resp)
}

// NewMarginOrder places a new cross margin order
func (e *Exchange) NewMarginOrder(ctx context.Context, arg *MarginOrderRequest) (*PortfolioMarginOrderResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.OrderType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	var resp *PortfolioMarginOrderResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/margin/order", nil, pmDefaultRate, arg, &resp)
}

// MarginAccountBorrow apply for margin loan
func (e *Exchange) MarginAccountBorrow(ctx context.Context, ccy currency.Code, amount float64) (string, error) {
	return e.marginAccountBorrowRepay(ctx, ccy, amount, "/papi/v1/marginLoan")
}

// MarginAccountRepay repay for margin loan
func (e *Exchange) MarginAccountRepay(ctx context.Context, ccy currency.Code, amount float64) (string, error) {
	return e.marginAccountBorrowRepay(ctx, ccy, amount, "/papi/v1/repayLoan")
}

func (e *Exchange) marginAccountBorrowRepay(ctx context.Context, ccy currency.Code, amount float64, path string) (string, error) {
	if ccy.IsEmpty() {
		return "", currency.ErrCurrencyCodeEmpty
	}
	if amount <= 0 {
		return "", limits.ErrAmountBelowMin
	}
	params := url.Values{}
	params.Set("asset", ccy.String())
	params.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	var resp struct {
		TransactionID types.PreciseNumber `json:"tranId"`
	}
	err := e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, path, params, pmMarginAccountLoanAndRepayRate, nil, &resp)
	return resp.TransactionID.String(), err
}

// MarginAccountNewOCO sends a new OCO order for a margin account.
func (e *Exchange) MarginAccountNewOCO(ctx context.Context, arg *OCOOrderRequest) (*OCOOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.LimitStrategyID != "" || arg.LimitStrategyType != "" || arg.StopStrategyID != 0 || arg.StopStrategyType != 0 || arg.SelfTradePreventionMode != "" || arg.TrailingDelta != 0 {
		return nil, fmt.Errorf("%w: strategy IDs, strategy types, selfTradePreventionMode and trailingDelta are not accepted by portfolio margin OCO", errUnsupportedParameter)
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.Amount <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.Price <= 0 {
		return nil, limits.ErrPriceBelowMin
	}
	if arg.StopPrice <= 0 {
		return nil, fmt.Errorf("%w, stopPrice is required", limits.ErrPriceBelowMin)
	}
	var resp *OCOOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/margin/order/oco", nil, pmDefaultRate, arg, &resp)
}

// NewUMConditionalOrder places a new conditional USDT margined order
//
// Deprecated: Binance deprecated UM conditional endpoints on 2026-04-28. Use the corresponding UM algo order APIs.
func (e *Exchange) NewUMConditionalOrder(ctx context.Context, arg *ConditionalOrderRequest) (*ConditionalOrder, error) {
	return e.placeConditionalOrder(ctx, arg, "/papi/v1/um/conditional/order")
}

// NewCMConditionalOrder posts a new coin margined futures conditional order.
func (e *Exchange) NewCMConditionalOrder(ctx context.Context, arg *ConditionalOrderRequest) (*ConditionalOrder, error) {
	return e.placeConditionalOrder(ctx, arg, "/papi/v1/cm/conditional/order")
}

func (e *Exchange) placeConditionalOrder(ctx context.Context, arg *ConditionalOrderRequest, path string) (*ConditionalOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if path == "/papi/v1/cm/conditional/order" && (!arg.GoodTillDate.IsZero() || arg.GoodTillDateTimestamp != 0 || arg.SelfTradePreventionMode != "") {
		return nil, fmt.Errorf("%w: goodTillDate and selfTradePreventionMode are UM-only", errUnsupportedParameter)
	}
	if !arg.GoodTillDate.IsZero() {
		arg.GoodTillDateTimestamp = arg.GoodTillDate.UnixMilli()
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.StrategyType == "" {
		return nil, errStrategyTypeRequired
	}
	var resp *ConditionalOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, path, nil, pmDefaultRate, arg, &resp)
}

// -------------------------------------------- Cancel Order Endpoints  ----------------------------------------------------

// CancelCMOrder cancels an active Coin Margined Futures limit order.
func (e *Exchange) CancelCMOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, orderID string) (*UMCMOrder, error) {
	return e.cancelOrder(ctx, symbol, origClientOrderID, "/papi/v1/cm/order", orderID)
}

// CancelUMOrder cancels an active USDT Margined Futures limit order.
func (e *Exchange) CancelUMOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, orderID string) (*UMCMOrder, error) {
	return e.cancelOrder(ctx, symbol, origClientOrderID, "/papi/v1/um/order", orderID)
}

func (e *Exchange) cancelOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, path, orderID string) (*UMCMOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if orderID == "" && origClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp *UMCMOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, path, params, pmDefaultRate, nil, &resp)
}

// CancelAllUMOrders cancels all active USDT Margined Futures limit orders on specific symbol
func (e *Exchange) CancelAllUMOrders(ctx context.Context, symbol currency.Pair) (*SuccessResponse, error) {
	return e.cancelAllUMCMOrders(ctx, symbol, "/papi/v1/um/allOpenOrders")
}

// CancelAllCMOrders cancels all active Coin Margined Futures limit orders on specific symbol
func (e *Exchange) CancelAllCMOrders(ctx context.Context, symbol currency.Pair) (*SuccessResponse, error) {
	return e.cancelAllUMCMOrders(ctx, symbol, "/papi/v1/cm/allOpenOrders")
}

func (e *Exchange) cancelAllUMCMOrders(ctx context.Context, symbol currency.Pair, path string) (*SuccessResponse, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	var resp *SuccessResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, path, params, pmDefaultRate, nil, &resp)
}

// PMCancelMarginAccountOrder cancels margin account order
func (e *Exchange) PMCancelMarginAccountOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, orderID string, options ...*PMCancelMarginAccountOrderRequest) (*PortfolioMarginOrderResponse, error) {
	option := new(PMCancelMarginAccountOrderRequest)
	if len(options) != 0 && options[0] != nil {
		option = options[0]
	}

	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if orderID == "" && origClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp *PortfolioMarginOrderResponse
	if option.NewClientOrderID != "" {
		params.Set("newClientOrderId", option.NewClientOrderID)
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, "/papi/v1/margin/order", params, pmMarginOrderDeleteRate, nil, &resp)
}

// CancelAllMarginOpenOrdersBySymbol cancels all open margin account orders of a specific symbol.
func (e *Exchange) CancelAllMarginOpenOrdersBySymbol(ctx context.Context, symbol currency.Pair) (MarginAccOrdersList, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	var resp MarginAccOrdersList
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, "/papi/v1/margin/allOpenOrders", params, pmCancelMarginAccountOpenOrdersOnSymbolRate, nil, &resp)
}

// CancelMarginAccountOCOOrders cancels margin account OCO orders.
func (e *Exchange) CancelMarginAccountOCOOrders(ctx context.Context, symbol currency.Pair, listClientOrderID, newClientOrderID string, orderListID int64) (*OCOOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if listClientOrderID != "" {
		params.Set("listClientOrderId", listClientOrderID)
	}
	if newClientOrderID != "" {
		params.Set("newClientOrderId", newClientOrderID)
	}
	if orderListID > 0 {
		params.Set("orderListId", strconv.FormatInt(orderListID, 10))
	}
	var resp *OCOOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, "/papi/v1/margin/orderList", params, pmCancelMarginAccountOCORate, nil, &resp)
}

// CancelUMConditionalOrder cancels a USDT margind futures conditional order
//
// Deprecated: Binance deprecated UM conditional endpoints on 2026-04-28. Use the corresponding UM algo order APIs.
func (e *Exchange) CancelUMConditionalOrder(ctx context.Context, symbol currency.Pair, newClientStrategyID string, strategyID int64) (*ConditionalOrder, error) {
	return e.cancelUMCMConditionalOrder(ctx, symbol, newClientStrategyID, "/papi/v1/um/conditional/order", strategyID)
}

// CancelCMConditionalOrder cancels a Coin margined futures conditional order
func (e *Exchange) CancelCMConditionalOrder(ctx context.Context, symbol currency.Pair, newClientStrategyID string, strategyID int64) (*ConditionalOrder, error) {
	return e.cancelUMCMConditionalOrder(ctx, symbol, newClientStrategyID, "/papi/v1/cm/conditional/order", strategyID)
}

func (e *Exchange) cancelUMCMConditionalOrder(ctx context.Context, symbol currency.Pair, newClientStrategyID, path string, strategyID int64) (*ConditionalOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if strategyID == 0 && newClientStrategyID == "" {
		return nil, fmt.Errorf("%w, either strategyId or newClientStrategyId is required", order.ErrOrderIDNotSet)
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if strategyID > 0 {
		params.Set("strategyId", strconv.FormatInt(strategyID, 10))
	}
	if newClientStrategyID != "" {
		params.Set("newClientStrategyId", newClientStrategyID)
	}
	var resp *ConditionalOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, path, params, pmDefaultRate, nil, &resp)
}

// CancelAllUMOpenConditionalOrders cancels all open conditional USDT margined orders
//
// Deprecated: Binance deprecated UM conditional endpoints on 2026-04-28. Use the corresponding UM algo order APIs.
func (e *Exchange) CancelAllUMOpenConditionalOrders(ctx context.Context, symbol currency.Pair) (*SuccessResponse, error) {
	return e.cancelAllUMCMOpenConditionalOrders(ctx, symbol, "/papi/v1/um/conditional/allOpenOrders")
}

// CancelAllCMOpenConditionalOrders cancels all open conditional Coin margined orders
func (e *Exchange) CancelAllCMOpenConditionalOrders(ctx context.Context, symbol currency.Pair) (*SuccessResponse, error) {
	return e.cancelAllUMCMOpenConditionalOrders(ctx, symbol, "/papi/v1/cm/conditional/allOpenOrders")
}

func (e *Exchange) cancelAllUMCMOpenConditionalOrders(ctx context.Context, symbol currency.Pair, path string) (*SuccessResponse, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	var resp *SuccessResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, path, params, pmDefaultRate, nil, &resp)
}

// --------------------------------------------------------   Query Order Endpoints  --------------------------------------------------------

// GetUMOrder check an USDT Margined order's status
// Orders can not be found if the order status is CANCELLED or EXPIRED
func (e *Exchange) GetUMOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, orderID string) (*UMCMOrder, error) {
	return e.getUMCMOrder(ctx, symbol, origClientOrderID, "/papi/v1/um/order", orderID)
}

// GetUMOpenOrder get current UM open order
func (e *Exchange) GetUMOpenOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, orderID string) (*UMCMOrder, error) {
	return e.getUMCMOrder(ctx, symbol, origClientOrderID, "/papi/v1/um/openOrder", orderID)
}

func (e *Exchange) getUMCMOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, path, orderID string) (*UMCMOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if orderID == "" && origClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp *UMCMOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, pmDefaultRate, nil, &resp)
}

// GetAllUMOpenOrders retrieves all open USDT margined orders.
// If no symbol is provided, it will load all open USDT orders, taking more ratelimit weight than the ordinary endpoints.
func (e *Exchange) GetAllUMOpenOrders(ctx context.Context, symbol currency.Pair) ([]*UMCMOrder, error) {
	endpointLimit := pmDefaultRate
	if symbol.IsEmpty() {
		endpointLimit = pmRetrieveAllUMOpenOrdersForAllSymbolRate
	}
	return e.getUMOrders(ctx, symbol, time.Time{}, time.Time{}, "/papi/v1/um/openOrders", "", 0, endpointLimit)
}

// GetAllUMOrders retrieves all USDT margined orders except for
// 1. CANCELLED or EXPIRED orders.
// 2. Orders with not fill.
//  3. Order Created later earlier than three days from now.
//
// If orderId is set, it will get orders >= that orderId. Otherwise most recent orders are returned.
// The query time period must be less then 7 days.
func (e *Exchange) GetAllUMOrders(ctx context.Context, arg *GetAllUMOrdersRequest) ([]*UMCMOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, startTime, endTime := arg.Symbol, arg.StartTime, arg.EndTime
	startingOrderID, limit := arg.StartingOrderID, arg.Limit
	return e.getUMOrders(ctx, symbol, startTime, endTime, "/papi/v1/um/allOrders", startingOrderID, limit, pmGetAllUMOrdersRate)
}

func (e *Exchange) getUMOrders(ctx context.Context, symbol currency.Pair, startTime, endTime time.Time, path, startingOrderID string, limit int64, endpointLimit request.EndpointLimit) ([]*UMCMOrder, error) {
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if startingOrderID != "" {
		params.Set("orderId", startingOrderID)
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*UMCMOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, endpointLimit, nil, &resp)
}

// GetCMOrder retrieves Coin Margined order instance.
func (e *Exchange) GetCMOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, orderID string) (*UMCMOrder, error) {
	return e.getUMCMOrder(ctx, symbol, origClientOrderID, "/papi/v1/cm/order", orderID)
}

// GetCMOpenOrder retrieves Coin Margined open order instance.
func (e *Exchange) GetCMOpenOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, orderID string) ([]*UMCMOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if orderID == "" && origClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp []*UMCMOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/cm/openOrder", params, pmDefaultRate, nil, &resp)
}

// GetAllCMOpenOrders retrieves all open Coin Margined futures orders on a symbol.
func (e *Exchange) GetAllCMOpenOrders(ctx context.Context, symbol currency.Pair, pair string) ([]*UMCMOrder, error) {
	endpointLimit := pmDefaultRate
	if symbol.IsEmpty() {
		endpointLimit = pmRetrieveAllCMOpenOrdersForAllSymbolRate
	}
	return e.getCMOrders(ctx, symbol, pair, "/papi/v1/cm/openOrders", "", time.Time{}, time.Time{}, 0, endpointLimit)
}

// GetAllCMOrders get all account CM orders; active, cancelled, or filled.
//
// Either symbol or pair must be sent.
// If orderId is set, it will get orders >= that orderId. Otherwise most recent orders are returned.
// These orders will not be found:
// - order status is CANCELLED or EXPIRED, AND
// - order has NO filled trade, AND
// - created time + 3 days < current time
func (e *Exchange) GetAllCMOrders(ctx context.Context, arg *GetAllCMOrdersRequest) ([]*UMCMOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, startTime, endTime := arg.Symbol, arg.StartTime, arg.EndTime
	pair, startingOrderID, limit := arg.Pair, arg.StartingOrderID, arg.Limit
	endpointLimit := pmAllCMOrderWithSymbolRate
	if symbol.IsEmpty() {
		endpointLimit = pmAllCMOrderWithoutSymbolRate
	}
	return e.getCMOrders(ctx, symbol, pair, "/papi/v1/cm/allOrders", startingOrderID, startTime, endTime, limit, endpointLimit)
}

func (e *Exchange) getCMOrders(ctx context.Context, symbol currency.Pair, pair, path, startingOrderID string, startTime, endTime time.Time, limit int64, endpointLimit request.EndpointLimit) ([]*UMCMOrder, error) {
	if symbol.IsEmpty() && pair == "" {
		return nil, fmt.Errorf("%w either symbol or pair is required", currency.ErrCurrencyPairEmpty)
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if startingOrderID != "" {
		params.Set("orderId", startingOrderID)
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*UMCMOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, endpointLimit, nil, &resp)
}

// GetOpenUMConditionalOrder retrieves a conditional USDT margined order
//
// Deprecated: Binance deprecated UM conditional endpoints on 2026-04-28. Use the corresponding UM algo order APIs.
func (e *Exchange) GetOpenUMConditionalOrder(ctx context.Context, symbol currency.Pair, newClientStrategyID string, strategyID int64) (*ConditionalOrder, error) {
	return e.getOpenUMCMConditionalOrder(ctx, symbol, newClientStrategyID, "/papi/v1/um/conditional/openOrder", strategyID)
}

func (e *Exchange) getOpenUMCMConditionalOrder(ctx context.Context, symbol currency.Pair, newClientStrategyID, path string, strategyID int64) (*ConditionalOrder, error) {
	if strategyID == 0 && newClientStrategyID == "" {
		return nil, fmt.Errorf("%w, either strategyId or newClientStrategyId is required", order.ErrOrderIDNotSet)
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if strategyID > 0 {
		params.Set("strategyId", strconv.FormatInt(strategyID, 10))
	}
	if newClientStrategyID != "" {
		params.Set("newClientStrategyId", newClientStrategyID)
	}
	var resp *ConditionalOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, pmDefaultRate, nil, &resp)
}

// GetAllUMOpenConditionalOrders retrieves all open conditional orders on a symbol.
//
// Deprecated: Binance deprecated UM conditional endpoints on 2026-04-28. Use the corresponding UM algo order APIs.
func (e *Exchange) GetAllUMOpenConditionalOrders(ctx context.Context, symbol currency.Pair) ([]*ConditionalOrder, error) {
	endpointLimit := pmDefaultRate
	if symbol.IsEmpty() {
		endpointLimit = pmUMOpenConditionalOrdersRate
	}
	return e.getAllUMCMOrders(ctx, symbol, "/papi/v1/um/conditional/openOrders", "", time.Time{}, time.Time{}, 0, 0, endpointLimit)
}

// GetAllUMConditionalOrderHistory retrieves all conditional order history a symbol.
//
// Deprecated: Binance deprecated UM conditional endpoints on 2026-04-28. Use the corresponding UM algo order APIs.
func (e *Exchange) GetAllUMConditionalOrderHistory(ctx context.Context, symbol currency.Pair, newClientStrategyID string, strategyID int64) ([]*ConditionalOrder, error) {
	return e.getAllUMCMOrders(ctx, symbol, "/papi/v1/um/conditional/orderHistory", newClientStrategyID, time.Time{}, time.Time{}, strategyID, 0, pmDefaultRate)
}

// GetAllUMConditionalOrders retrieves conditional orders.
//
// Deprecated: Binance deprecated UM conditional endpoints on 2026-04-28. Use the corresponding UM algo order APIs.
func (e *Exchange) GetAllUMConditionalOrders(ctx context.Context, arg *GetAllUMConditionalOrdersRequest) ([]*ConditionalOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, startTime, endTime := arg.Symbol, arg.StartTime, arg.EndTime
	strategyID, limit := arg.StrategyID, arg.Limit
	endpointLimit := pmDefaultRate
	if symbol.IsEmpty() {
		endpointLimit = pmAllUMConditionalOrdersWithoutSymbolRate
	}
	return e.getAllUMCMOrders(ctx, symbol, "/papi/v1/um/conditional/allOrders", "", startTime, endTime, strategyID, limit, endpointLimit)
}

func (e *Exchange) getAllUMCMOrders(ctx context.Context, symbol currency.Pair, path, newClientStrategyID string, startTime, endTime time.Time, strategyID, limit int64, endpointLimit request.EndpointLimit) ([]*ConditionalOrder, error) {
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	if strategyID > 0 {
		params.Set("strategyId", strconv.FormatInt(strategyID, 10))
	}
	if newClientStrategyID != "" {
		params.Set("newClientStrategyId", newClientStrategyID)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*ConditionalOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, endpointLimit, nil, &resp)
}

// GetOpenCMConditionalOrder get current Coin Margined open conditional order
func (e *Exchange) GetOpenCMConditionalOrder(ctx context.Context, symbol currency.Pair, newClientStrategyID string, strategyID int64) (*ConditionalOrder, error) {
	return e.getOpenUMCMConditionalOrder(ctx, symbol, newClientStrategyID, "/papi/v1/cm/conditional/openOrder", strategyID)
}

// GetAllCMOpenConditionalOrders retrieves all open conditional orders on a symbol.
func (e *Exchange) GetAllCMOpenConditionalOrders(ctx context.Context, symbol currency.Pair) ([]*ConditionalOrder, error) {
	endpointLimit := pmDefaultRate
	if symbol.IsEmpty() {
		endpointLimit = pmAllCMOpenConditionalOrdersWithoutSymbolRate
	}
	return e.getAllUMCMOrders(ctx, symbol, "/papi/v1/cm/conditional/openOrders", "", time.Time{}, time.Time{}, 0, 0, endpointLimit)
}

// GetAllCMConditionalOrderHistory retrieves all conditional order history a symbol.
func (e *Exchange) GetAllCMConditionalOrderHistory(ctx context.Context, symbol currency.Pair, newClientStrategyID string, strategyID int64) (*ConditionalOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if strategyID <= 0 && newClientStrategyID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{symbolParam: {symbol.String()}}
	if strategyID > 0 {
		params.Set("strategyId", strconv.FormatInt(strategyID, 10))
	}
	if newClientStrategyID != "" {
		params.Set("newClientStrategyId", newClientStrategyID)
	}
	var resp *ConditionalOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/cm/conditional/orderHistory", params, pmDefaultRate, nil, &resp)
}

// GetAllCMConditionalOrders retrieves conditional orders.
func (e *Exchange) GetAllCMConditionalOrders(ctx context.Context, arg *GetAllCMConditionalOrdersRequest) ([]*ConditionalOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, startTime, endTime := arg.Symbol, arg.StartTime, arg.EndTime
	strategyID, limit := arg.StrategyID, arg.Limit
	endpointLimit := pmDefaultRate
	if symbol.IsEmpty() {
		endpointLimit = pmAllCMConditionalOrderWithoutSymbolRate
	}
	return e.getAllUMCMOrders(ctx, symbol, "/papi/v1/cm/conditional/allOrders", "", startTime, endTime, strategyID, limit, endpointLimit)
}

// GetMarginAccountOrder retrieves margin account order.
func (e *Exchange) GetMarginAccountOrder(ctx context.Context, symbol currency.Pair, origClientOrderID, orderID string) (*MarginOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if orderID == "" && origClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp *MarginOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/order", params, pmMarginOrderGetRate, nil, &resp)
}

// GetCurrentMarginOpenOrders retrieves an open order.
// If the symbol is not sent, orders for all symbols will be returned in an array.
func (e *Exchange) GetCurrentMarginOpenOrders(ctx context.Context, symbol currency.Pair) ([]*MarginOrder, error) {
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	var resp []*MarginOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/openOrders", params, pmCurrentMarginOpenOrderRate, nil, &resp)
}

// GetAllMarginAccountOrders retrieves all margin account orders
func (e *Exchange) GetAllMarginAccountOrders(ctx context.Context, arg *GetAllMarginAccountOrdersRequest) ([]*MarginOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, startTime, endTime := arg.Symbol, arg.StartTime, arg.EndTime
	orderID, limit := arg.OrderID, arg.Limit
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*MarginOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/allOrders", params, pmAllMarginAccountOrdersRate, nil, &resp)
}

// GetMarginAccountOCO retrieves a specific OCO based on provided optional parameters.
func (e *Exchange) GetMarginAccountOCO(ctx context.Context, orderListID int64, origClientOrderID string) (*OCOOrder, error) {
	params := url.Values{}
	if orderListID > 0 {
		params.Set("orderListId", strconv.FormatInt(orderListID, 10))
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp *OCOOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/orderList", params, pmGetMarginAccountOCORate, nil, &resp)
}

// GetPMMarginAccountAllOCO a portfolio margin method to retrieve all OCO for a specific margin account based on provided optional parameters
func (e *Exchange) GetPMMarginAccountAllOCO(ctx context.Context, startTime, endTime time.Time, fromID, limit int64) ([]*OCOOrder, error) {
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if fromID > 0 {
		params.Set("fromId", strconv.FormatInt(fromID, 10))
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*OCOOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/allOrderList", params, pmGetMarginAccountsAllOCOOrdersRate, nil, &resp)
}

// GetMarginAccountsOpenOCO retrieves a margin account open OCO order
func (e *Exchange) GetMarginAccountsOpenOCO(ctx context.Context) ([]*OCOOrder, error) {
	var resp []*OCOOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/openOrderList", nil, pmGetMarginAccountsOpenOCOOrdersRate, nil, &resp)
}

// GetPMMarginAccountTradeList retrieves margin account trade list
func (e *Exchange) GetPMMarginAccountTradeList(ctx context.Context, arg *GetPMMarginAccountTradeListRequest) ([]*TradeHistory, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, startTime, endTime := arg.Symbol, arg.StartTime, arg.EndTime
	orderID, fromID, limit := arg.OrderID, arg.FromID, arg.Limit
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params, err := ocoOrdersAndTradeParams(symbol, false, startTime, endTime, orderID, fromID, limit)
	if err != nil {
		return nil, err
	}
	var resp []*TradeHistory
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/myTrades", params, pmGetMarginAccountTradeListRate, nil, &resp)
}

//  ---------------------------------------------------  Account Endpoints  -------------------------------------------------------------------------------------

// GetAccountBalance retrieves all account balance information related to an asset/assets(if assetName is not provided).
func (e *Exchange) GetAccountBalance(ctx context.Context, assetName currency.Code) (AccountBalanceResponse, error) {
	params := url.Values{}
	if !assetName.IsEmpty() {
		params.Set("asset", assetName.String())
	}
	var resp AccountBalanceResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/balance", params, pmGetAccountBalancesRate, nil, &resp)
}

// GetPortfolioMarginAccountInformation retrieves an account information
func (e *Exchange) GetPortfolioMarginAccountInformation(ctx context.Context) (*AccountInformation, error) {
	var resp *AccountInformation
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/account", nil, pmGetAccountInformationRate, nil, &resp)
}

// GetPMMarginMaxBorrow holds the maximum borrowable amount limited by the account level.
func (e *Exchange) GetPMMarginMaxBorrow(ctx context.Context, assetName currency.Code) (*MaxBorrow, error) {
	if assetName.IsEmpty() {
		return nil, fmt.Errorf("%w, assetName is required", currency.ErrCurrencyCodeEmpty)
	}
	params := url.Values{}
	params.Set("asset", assetName.String())
	var resp *MaxBorrow
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/maxBorrowable", params, pmMarginMaxBorrowRate, nil, &resp)
}

// GetMarginMaxWithdrawal retrieves the maximum withdrawal amount allowed for margin account.
func (e *Exchange) GetMarginMaxWithdrawal(ctx context.Context, assetName currency.Code) (float64, error) {
	if assetName.IsEmpty() {
		return 0, fmt.Errorf("%w, assetName is required", currency.ErrCurrencyCodeEmpty)
	}
	params := url.Values{}
	params.Set("asset", assetName.String())
	var resp struct {
		Amount types.Number `json:"amount"`
	}
	return resp.Amount.Float64(), e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/maxWithdraw", params, pmGetMarginMaxWithdrawalRate, nil, &resp)
}

// GetUMPositionInformation get current UM position information.
//
// for One-way Mode user, the response will only show the "BOTH" positions
// for Hedge Mode user, the response will show "LONG", and "SHORT" positions.
func (e *Exchange) GetUMPositionInformation(ctx context.Context, symbol currency.Pair) ([]*UMPositionInformation, error) {
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	var resp []*UMPositionInformation
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/positionRisk", params, pmGetUMPositionInformationRate, nil, &resp)
}

// GetCMPositionInformation retrieves current margin position information.
func (e *Exchange) GetCMPositionInformation(ctx context.Context, marginAsset currency.Code, pair string) ([]*CMPositionInformation, error) {
	params := url.Values{}
	if !marginAsset.IsEmpty() {
		params.Set("marginAsset", marginAsset.String())
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	var resp []*CMPositionInformation
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/cm/positionRisk", params, pmDefaultRate, nil, &resp)
}

// ChangeUMInitialLeverage changes user's initial leverage of specific symbol in UM.
func (e *Exchange) ChangeUMInitialLeverage(ctx context.Context, symbol currency.Pair, leverage float64) (*InitialLeverage, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if leverage < 1 || leverage > 125 {
		return nil, fmt.Errorf("%w, leverage must be between 1 and 125", order.ErrSubmitLeverageNotSupported)
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	params.Set("leverage", strconv.FormatFloat(leverage, 'f', -1, 64))
	var resp *InitialLeverage
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/um/leverage", params, pmDefaultRate, nil, &resp)
}

// ChangeCMInitialLeverage change user's initial leverage of specific symbol in CM.
func (e *Exchange) ChangeCMInitialLeverage(ctx context.Context, symbol currency.Pair, leverage float64) (*CMInitialLeverage, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if leverage < 1 || leverage > 125 {
		return nil, fmt.Errorf("%w, leverage must be between 1 and 125", order.ErrSubmitLeverageNotSupported)
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	params.Set("leverage", strconv.FormatFloat(leverage, 'f', -1, 64))
	var resp *CMInitialLeverage
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/cm/leverage", params, pmDefaultRate, nil, &resp)
}

// ChangeUMPositionMode change user's position mode (Hedge Mode or One-way Mode ) on EVERY symbol in UM
func (e *Exchange) ChangeUMPositionMode(ctx context.Context, dualSidePosition bool) (*SuccessResponse, error) {
	return e.changeUMCMPositionMode(ctx, dualSidePosition, "/papi/v1/um/positionSide/dual")
}

// ChangeCMPositionMode change user's position mode (Hedge Mode or One-way Mode ) on EVERY symbol in CM
func (e *Exchange) ChangeCMPositionMode(ctx context.Context, dualSidePosition bool) (*SuccessResponse, error) {
	return e.changeUMCMPositionMode(ctx, dualSidePosition, "/papi/v1/cm/positionSide/dual")
}

func (e *Exchange) changeUMCMPositionMode(ctx context.Context, dualSidePosition bool, path string) (*SuccessResponse, error) {
	params := url.Values{}
	if dualSidePosition {
		params.Set("dualSidePosition", "true")
	} else {
		params.Set("dualSidePosition", "false")
	}
	var resp *SuccessResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, path, params, pmDefaultRate, nil, &resp)
}

// GetUMCurrentPositionMode get user's position mode (Hedge Mode or One-way Mode ) on EVERY symbol in UM
func (e *Exchange) GetUMCurrentPositionMode(ctx context.Context) (*DualPositionMode, error) {
	return e.getPositionMode(ctx, "/papi/v1/um/positionSide/dual", pmGetUMCurrentPositionModeRate)
}

// GetCMCurrentPositionMode get user's position mode (Hedge Mode or One-way Mode ) on EVERY symbol in CM
func (e *Exchange) GetCMCurrentPositionMode(ctx context.Context) (*DualPositionMode, error) {
	return e.getPositionMode(ctx, "/papi/v1/cm/positionSide/dual", pmGetCMCurrentPositionModeRate)
}

func (e *Exchange) getPositionMode(ctx context.Context, path string, endpointLimit request.EndpointLimit) (*DualPositionMode, error) {
	var resp *DualPositionMode
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, nil, endpointLimit, nil, &resp)
}

// GetUMAccountTradeList get trades for a specific account and UM symbol.
func (e *Exchange) GetUMAccountTradeList(ctx context.Context, arg *GetUMAccountTradeListRequest) ([]*UMCMAccountTradeItem, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, startTime, endTime := arg.Symbol, arg.StartTime, arg.EndTime
	fromID, limit := arg.FromID, arg.Limit
	return e.getUMCMAccountTradeList(ctx, symbol, "", "/papi/v1/um/userTrades", startTime, endTime, fromID, limit, pmGetUMAccountTradeListRate)
}

// GetCMAccountTradeList get trades for a specific account and CM symbol.
func (e *Exchange) GetCMAccountTradeList(ctx context.Context, arg *GetCMAccountTradeListRequest) ([]*UMCMAccountTradeItem, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, pair, startTime := arg.Symbol, arg.Pair, arg.StartTime
	endTime, fromID, limit := arg.EndTime, arg.FromID, arg.Limit
	if symbol.IsEmpty() && pair == "" {
		return nil, fmt.Errorf("%w, either symbol or pair is required", currency.ErrCurrencyPairEmpty)
	}
	endpointLimit := pmGetCMAccountTradeListWithPairRate
	if !symbol.IsEmpty() {
		endpointLimit = pmGetCMAccountTradeListWithSymbolRate
	}
	return e.getUMCMAccountTradeList(ctx, symbol, pair, "/papi/v1/cm/userTrades", startTime, endTime, fromID, limit, endpointLimit)
}

// getUMCMAccountTradeList serves both product lines; pair is CM only and is an
// accepted alternative to symbol there, so only require one of the two.
func (e *Exchange) getUMCMAccountTradeList(ctx context.Context, symbol currency.Pair, pair, path string, startTime, endTime time.Time, fromID, limit int64, endpointLimit request.EndpointLimit) ([]*UMCMAccountTradeItem, error) {
	if symbol.IsEmpty() && pair == "" {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if fromID > 0 {
		params.Set("fromId", strconv.FormatInt(fromID, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*UMCMAccountTradeItem
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, endpointLimit, nil, &resp)
}

// GetUMNotionalAndLeverageBrackets query UM notional and leverage brackets
func (e *Exchange) GetUMNotionalAndLeverageBrackets(ctx context.Context, symbol currency.Pair) ([]*NotionalAndLeverage, error) {
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	var resp []*NotionalAndLeverage
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/leverageBracket", params, pmDefaultRate, nil, &resp)
}

// GetCMNotionalAndLeverageBrackets query UM notional and leverage brackets
func (e *Exchange) GetCMNotionalAndLeverageBrackets(ctx context.Context, symbol currency.Pair) ([]*CMNotionalAndLeverage, error) {
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	var resp []*CMNotionalAndLeverage
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/cm/leverageBracket", params, pmDefaultRate, nil, &resp)
}

// GetUsersMarginForceOrders query user's margin force orders
func (e *Exchange) GetUsersMarginForceOrders(ctx context.Context, startTime, endTime time.Time, current, size int64) (*MarginForceOrder, error) {
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if current > 0 {
		params.Set("current", strconv.FormatInt(current, 10))
	}
	if size > 0 {
		params.Set("size", strconv.FormatInt(size, 10))
	}
	var resp *MarginForceOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/forceOrders", params, pmDefaultRate, nil, &resp)
}

// GetUsersUMForceOrders query User's UM Force Orders
func (e *Exchange) GetUsersUMForceOrders(ctx context.Context, arg *GetUsersUMForceOrdersRequest) ([]*ForceOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, autoCloseType, startTime := arg.Symbol, arg.AutoCloseType, arg.StartTime
	endTime, limit := arg.EndTime, arg.Limit
	endpointLimit := pmGetUserUMForceOrdersWithSymbolRate
	if symbol.IsEmpty() {
		endpointLimit = pmGetUserUMForceOrdersWithoutSymbolRate
	}
	return e.getUsersUMCMForceOrders(ctx, symbol, autoCloseType, "/papi/v1/um/forceOrders", startTime, endTime, limit, endpointLimit)
}

// GetUsersCMForceOrders query User's CM Force Orders
func (e *Exchange) GetUsersCMForceOrders(ctx context.Context, arg *GetUsersCMForceOrdersRequest) ([]*ForceOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, autoCloseType, startTime := arg.Symbol, arg.AutoCloseType, arg.StartTime
	endTime, limit := arg.EndTime, arg.Limit
	endpointLimit := pmGetUserCMForceOrdersWithSymbolRate
	if symbol.IsEmpty() {
		endpointLimit = pmGetUserCMForceOrdersWithoutSymbolRate
	}
	return e.getUsersUMCMForceOrders(ctx, symbol, autoCloseType, "/papi/v1/cm/forceOrders", startTime, endTime, limit, endpointLimit)
}

func (e *Exchange) getUsersUMCMForceOrders(ctx context.Context, symbol currency.Pair, autoCloseType, path string, startTime, endTime time.Time, limit int64, endpointLimit request.EndpointLimit) ([]*ForceOrder, error) {
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if autoCloseType != "" {
		params.Set("autoCloseType", autoCloseType)
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*ForceOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, endpointLimit, nil, &resp)
}

// GetPortfolioMarginUMTradingQuantitativeRulesIndicator retrieves rules that regulate general trading based on the quantitative indicators
func (e *Exchange) GetPortfolioMarginUMTradingQuantitativeRulesIndicator(ctx context.Context, symbol currency.Pair) (*TradingQuantitativeRulesIndicators, error) {
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	endpointLimit := pmDefaultRate
	if symbol.IsEmpty() {
		endpointLimit = pmUMTradingQuantitativeRulesIndicatorsRate
	}
	var resp *TradingQuantitativeRulesIndicators
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/apiTradingStatus", params, endpointLimit, nil, &resp)
}

// GetUMUserCommissionRate retrieves usdt margined account user's commission rate
func (e *Exchange) GetUMUserCommissionRate(ctx context.Context, symbol currency.Pair) (*CommissionRate, error) {
	return e.getUserCommissionRate(ctx, symbol, "/papi/v1/um/commissionRate", pmGetUMUserCommissionRate)
}

// GetCMUserCommissionRate retrieves coin margined account user's commission rate
func (e *Exchange) GetCMUserCommissionRate(ctx context.Context, symbol currency.Pair) (*CommissionRate, error) {
	return e.getUserCommissionRate(ctx, symbol, "/papi/v1/cm/commissionRate", pmGetCMUserCommissionRate)
}

func (e *Exchange) getUserCommissionRate(ctx context.Context, symbol currency.Pair, path string, endpointLimit request.EndpointLimit) (*CommissionRate, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	var resp *CommissionRate
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, endpointLimit, nil, &resp)
}

func prepareMarginLoanOrRepayParams(assetName currency.Code, startTime, endTime time.Time, transactionID, current, size int64) (url.Values, error) {
	params := url.Values{}
	if !assetName.IsEmpty() {
		params.Set("asset", assetName.String())
	}
	if transactionID > 0 {
		params.Set("txId", strconv.FormatInt(transactionID, 10))
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if current > 0 {
		params.Set("current", strconv.FormatInt(current, 10))
	}
	if size > 0 {
		params.Set("size", strconv.FormatInt(size, 10))
	}
	return params, nil
}

// GetMarginLoanRecord query margin loan record
func (e *Exchange) GetMarginLoanRecord(ctx context.Context, arg *GetMarginLoanRecordRequest) (*MarginLoanRecord, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	assetName, startTime, endTime := arg.AssetName, arg.StartTime, arg.EndTime
	transactionID, current, size := arg.TransactionID, arg.Current, arg.Size
	if assetName.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	params, err := prepareMarginLoanOrRepayParams(assetName, startTime, endTime, transactionID, current, size)
	if err != nil {
		return nil, err
	}
	var resp *MarginLoanRecord
	if arg.Archived != "" {
		params.Set("archived", arg.Archived)
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/marginLoan", params, pmGetMarginLoanRecordRate, nil, &resp)
}

// GetMarginRepayRecord query margin repay record.
func (e *Exchange) GetMarginRepayRecord(ctx context.Context, arg *GetMarginRepayRecordRequest) (*MarginRepayRecord, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	assetName, startTime, endTime := arg.AssetName, arg.StartTime, arg.EndTime
	transactionID, current, size := arg.TransactionID, arg.Current, arg.Size
	if assetName.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	params, err := prepareMarginLoanOrRepayParams(assetName, startTime, endTime, transactionID, current, size)
	if err != nil {
		return nil, err
	}
	var resp *MarginRepayRecord
	if arg.Archived != "" {
		params.Set("archived", arg.Archived)
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/repayLoan", params, pmGetMarginRepayRecordRate, nil, &resp)
}

// GetMarginBorrowOrLoanInterestHistory retrieves margin borrow loan interest history
func (e *Exchange) GetMarginBorrowOrLoanInterestHistory(ctx context.Context, arg *GetMarginBorrowOrLoanInterestHistoryRequest) (*MarginBorrowOrLoanInterest, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.TransactionID != 0 {
		return nil, fmt.Errorf("%w: txId is not an interest-history filter", errUnsupportedParameter)
	}
	assetName, startTime, endTime := arg.AssetName, arg.StartTime, arg.EndTime
	transactionID, current, size := arg.TransactionID, arg.Current, arg.Size
	params, err := prepareMarginLoanOrRepayParams(assetName, startTime, endTime, transactionID, current, size)
	if err != nil {
		return nil, err
	}
	var resp *MarginBorrowOrLoanInterest
	if arg.Archived != "" {
		params.Set("archived", arg.Archived)
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/margin/marginInterestHistory", params, pmDefaultRate, nil, &resp)
}

// GetPortfolioMarginNegativeBalanceInterestHistory retrieves interest history of negative balance for portfolio margin.
func (e *Exchange) GetPortfolioMarginNegativeBalanceInterestHistory(ctx context.Context, assetName currency.Code, startTime, endTime time.Time, size int64) ([]*PortfolioMarginNegativeBalanceInterest, error) {
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !assetName.IsEmpty() {
		params.Set("asset", assetName.String())
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if size > 0 {
		params.Set("size", strconv.FormatInt(size, 10))
	}
	var resp []*PortfolioMarginNegativeBalanceInterest
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/portfolio/interest-history", params, pmGetPortfolioMarginNegativeBalanceInterestHistoryRate, nil, &resp)
}

// FundAutoCollection fund collection for Portfolio Margin
func (e *Exchange) FundAutoCollection(ctx context.Context) (string, error) {
	var resp struct {
		Message string `json:"msg"`
	}
	return resp.Message, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/auto-collection", nil, pmFundAutoCollectionRate, nil, &resp)
}

// FundCollectionByAsset transfers specific asset from Futures Account to Margin account
// The BNB transfer is not be supported
func (e *Exchange) FundCollectionByAsset(ctx context.Context, assetName currency.Code) (string, error) {
	if assetName.IsEmpty() {
		return "", fmt.Errorf("%w, assetName is required", currency.ErrCurrencyCodeEmpty)
	}
	params := url.Values{}
	params.Set("asset", assetName.String())
	var resp struct {
		Message string `json:"msg"`
	}
	return resp.Message, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/asset-collection", params, pmFundCollectionByAssetRate, nil, &resp)
}

// BNBTransfer Transfer BNB assets
// transferSize: "TO_UM","FROM_UM"
func (e *Exchange) BNBTransfer(ctx context.Context, amount float64, transferSide string) (int64, error) {
	return e.bnbTransfer(ctx, amount, transferSide, "/papi/v1/bnb-transfer", pmBNBTransferRate, exchange.RestFuturesSupplementary)
}

func (e *Exchange) bnbTransfer(ctx context.Context, amount float64, transferSide, path string, endpointLimit request.EndpointLimit, exchangeURL exchange.URL) (int64, error) {
	params := url.Values{}
	if amount > 0 {
		params.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	}
	if transferSide != "" {
		params.Set("transferSide", transferSide)
	}
	var resp struct {
		TransactionID int64 `json:"tranId"`
	}
	err := e.SendAuthHTTPRequest(ctx, exchangeURL, http.MethodPost, path, params, endpointLimit, nil, &resp)
	return resp.TransactionID, err
}

// GetUMIncomeHistory retrieves USDT margined futures income history
// possible incomeType values: TRANSFER, WELCOME_BONUS, REALIZED_PNL, FUNDING_FEE, COMMISSION, INSURANCE_CLEAR, REFERRAL_KICKBACK, COMMISSION_REBATE, API_REBATE, CONTEST_REWARD, CROSS_COLLATERAL_TRANSFER, OPTIONS_PREMIUM_FEE, OPTIONS_SETTLE_PROFIT, INTERNAL_TRANSFER, AUTO_EXCHANGE, DELIVERED_SETTLEMENT, COIN_SWAP_DEPOSIT, COIN_SWAP_WITHDRAW, POSITION_LIMIT_INCREASE_FEE
func (e *Exchange) GetUMIncomeHistory(ctx context.Context, arg *GetUMIncomeHistoryRequest) ([]*IncomeItem, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, incomeType, startTime := arg.Symbol, arg.IncomeType, arg.StartTime
	endTime, limit := arg.EndTime, arg.Limit
	return e.getUMCMIncomeHistory(ctx, symbol, incomeType, "/papi/v1/um/income", startTime, endTime, limit, arg.Page, pmGetUMIncomeHistoryRate)
}

// GetCMIncomeHistory get current UM account asset and position information.
func (e *Exchange) GetCMIncomeHistory(ctx context.Context, arg *GetCMIncomeHistoryRequest) ([]*IncomeItem, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, incomeType, startTime := arg.Symbol, arg.IncomeType, arg.StartTime
	endTime, limit := arg.EndTime, arg.Limit
	return e.getUMCMIncomeHistory(ctx, symbol, incomeType, "/papi/v1/cm/income", startTime, endTime, limit, arg.Page, pmGetCMIncomeHistoryRate)
}

func (e *Exchange) getUMCMIncomeHistory(ctx context.Context, symbol currency.Pair, incomeType, path string, startTime, endTime time.Time, limit int64, page uint64, endpointLimit request.EndpointLimit) ([]*IncomeItem, error) {
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if page != 0 {
		params.Set("page", strconv.FormatUint(page, 10))
	}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	if incomeType != "" {
		params.Set("incomeType", incomeType)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*IncomeItem
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, endpointLimit, nil, &resp)
}

// GetCMAccountDetail gets current CM account asset and position information.
func (e *Exchange) GetCMAccountDetail(ctx context.Context) (*AccountDetail, error) {
	return e.getUMCMAccountDetail(ctx, "/papi/v1/cm/account", pmGetCMAccountDetailRate)
}

func (e *Exchange) getUMCMAccountDetail(ctx context.Context, path string, endpointLimit request.EndpointLimit) (*AccountDetail, error) {
	var resp *AccountDetail
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, nil, endpointLimit, nil, &resp)
}

// ChangeAutoRepayFuturesStatus change Auto-repay-futures Status
func (e *Exchange) ChangeAutoRepayFuturesStatus(ctx context.Context, autoRepay bool) (string, error) {
	return e.changeAutoRepayFuturesStatus(ctx, autoRepay, exchange.RestFuturesSupplementary, "/papi/v1/repay-futures-switch", pmChangeAutoRepayFuturesStatusRate)
}

func (e *Exchange) changeAutoRepayFuturesStatus(ctx context.Context, autoRepay bool, exchURL exchange.URL, path string, epl request.EndpointLimit) (string, error) {
	params := url.Values{}
	if autoRepay {
		params.Set("autoRepay", "true")
	} else {
		params.Set("autoRepay", "false")
	}
	var resp struct {
		Message string `json:"msg"`
	}
	return resp.Message, e.SendAuthHTTPRequest(ctx, exchURL, http.MethodPost, path, params, epl, nil, &resp)
}

// GetAutoRepayFuturesStatus query Auto-repay-futures Status
func (e *Exchange) GetAutoRepayFuturesStatus(ctx context.Context) (*AutoRepayStatus, error) {
	var resp *AutoRepayStatus
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/repay-futures-switch", nil, pmGetAutoRepayFuturesStatusRate, nil, &resp)
}

// RepayFuturesNegativeBalance repay futures Negative Balance
func (e *Exchange) RepayFuturesNegativeBalance(ctx context.Context) (string, error) {
	var resp struct {
		Message string `json:"msg"`
	}
	return resp.Message, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/repay-futures-negative-balance", nil, pmRepayFuturesNegativeBalanceRate, nil, &resp)
}

// GetUMPositionADLQuantileEstimation retrieves ADL Quantile Estimations for a symbol or symbols
//
// Values 0, 1, 2, 3, 4 shows the queue position and possibility of ADL from low to high.
// For positions of the symbol are in One-way Mode or isolated margined in Hedge Mode, "LONG", "SHORT", and "BOTH" will be returned to show the positions' adl quantiles of different position sides.
func (e *Exchange) GetUMPositionADLQuantileEstimation(ctx context.Context, symbol currency.Pair) ([]*ADLQuantileEstimation, error) {
	return e.getUMCMPositionADLQuantileEstimation(ctx, symbol, "/papi/v1/um/adlQuantile", pmGetUMPositionADLQuantileEstimationRate)
}

// GetCMPositionADLQuantileEstimation retrieves Coin Margined Futures position ADL Quantile estimation for symbol or symbols
func (e *Exchange) GetCMPositionADLQuantileEstimation(ctx context.Context, symbol currency.Pair) ([]*ADLQuantileEstimation, error) {
	return e.getUMCMPositionADLQuantileEstimation(ctx, symbol, "/papi/v1/cm/adlQuantile", pmGetCMPositionADLQuantileEstimationRate)
}

func (e *Exchange) getUMCMPositionADLQuantileEstimation(ctx context.Context, symbol currency.Pair, path string, endpointLimit request.EndpointLimit) ([]*ADLQuantileEstimation, error) {
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	var resp []*ADLQuantileEstimation
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, endpointLimit, nil, &resp)
}

// GetUserRateLimits retrieves list of user's account rate-limit information
func (e *Exchange) GetUserRateLimits(ctx context.Context) ([]*RateLimitInfo, error) {
	var resp []*RateLimitInfo
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/rateLimit/order", nil, request.UnAuth, nil, &resp)
}

// PortfolioMarginPing tests connectivity to the portfolio margin REST API.
func (e *Exchange) PortfolioMarginPing(ctx context.Context) error {
	return e.SendHTTPRequest(ctx, exchange.RestFuturesSupplementary, "/papi/v1/ping", pmDefaultRate, &struct{}{})
}

// GetUMAccountDetail returns UM account assets and positions, restricted to symbols with
// positions or open orders. Configuration fields moved to the accountConfig and symbolConfig
// endpoints in this version.
func (e *Exchange) GetUMAccountDetail(ctx context.Context) (*UMAccountDetailV2, error) {
	var resp *UMAccountDetailV2
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v2/um/account", nil, pmGetUMAccountDetailV2Rate, nil, &resp)
}

// NewUMAlgoOrder places a portfolio margin UM algo order.
func (e *Exchange) NewUMAlgoOrder(ctx context.Context, arg *UMAlgoOrderRequest) (*UMAlgoOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.AlgoType == "" {
		return nil, errAlgoTypeRequired
	}
	if arg.OrderType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	symbol, err := e.FormatExchangeCurrency(arg.Symbol, asset.USDTMarginedFutures)
	if err != nil {
		return nil, err
	}
	requestData := *arg
	requestData.Symbol = symbol
	var resp *UMAlgoOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/um/algo/order", nil, pmDefaultRate, &requestData, &resp)
}

// CancelUMAlgoOrder cancels a portfolio margin UM algo order.
func (e *Exchange) CancelUMAlgoOrder(ctx context.Context, algoID uint64, options ...*CancelUMAlgoOrderRequest) (*UMAlgoOrder, error) {
	option := new(CancelUMAlgoOrderRequest)
	if len(options) != 0 && options[0] != nil {
		option = options[0]
	}

	if algoID == 0 && option.ClientAlgoID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{}
	if algoID != 0 {
		params.Set("algoId", strconv.FormatUint(algoID, 10))
	}
	var resp *UMAlgoOrder
	if option.ClientAlgoID != "" {
		params.Set("clientAlgoId", option.ClientAlgoID)
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, "/papi/v1/um/algo/order", params, pmDefaultRate, nil, &resp)
}

// CancelAllUMAlgoOpenOrders cancels every open portfolio margin UM algo order on a symbol.
func (e *Exchange) CancelAllUMAlgoOpenOrders(ctx context.Context, symbol currency.Pair) (*SuccessResponse, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	var resp *SuccessResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodDelete, "/papi/v1/um/algo/allOpenOrders", params, pmDefaultRate, nil, &resp)
}

// GetAllUMOpenAlgoOrders returns the current open portfolio margin UM algo orders.
func (e *Exchange) GetAllUMOpenAlgoOrders(ctx context.Context, symbol currency.Pair, options ...*GetAllUMOpenAlgoOrdersRequest) ([]*UMAlgoOrder, error) {
	option := new(GetAllUMOpenAlgoOrdersRequest)
	if len(options) != 0 && options[0] != nil {
		option = options[0]
	}

	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	var resp []*UMAlgoOrder
	if option.AlgoID != 0 {
		params.Set("algoId", strconv.FormatUint(option.AlgoID, 10))
	}
	if option.AlgoType != "" {
		params.Set("algoType", option.AlgoType)
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/algo/openAlgoOrders", params, pmDefaultRate, nil, &resp)
}

// GetUMAlgoOrderHistory returns historical portfolio margin UM algo orders for a symbol.
func (e *Exchange) GetUMAlgoOrderHistory(ctx context.Context, symbol currency.Pair, options ...*GetUMAlgoOrderHistoryRequest) ([]*UMAlgoOrder, error) {
	option := new(GetUMAlgoOrderHistoryRequest)
	if len(options) != 0 && options[0] != nil {
		option = options[0]
	}

	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	var resp []*UMAlgoOrder
	if option.AlgoID != 0 {
		params.Set("algoId", strconv.FormatUint(option.AlgoID, 10))
	}
	if !option.EndTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(option.EndTime.UTC().UnixMilli(), 10))
	}
	if option.Limit != 0 {
		params.Set("limit", strconv.FormatUint(option.Limit, 10))
	}
	if !option.StartTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(option.StartTime.UTC().UnixMilli(), 10))
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/algo/allAlgoOrders", params, pmUmAlgoAllAlgoOrdersRate, nil, &resp)
}

// GetUMOpenAlgoOrder returns one portfolio margin UM algo order by exchange or client ID.
func (e *Exchange) GetUMOpenAlgoOrder(ctx context.Context, _ currency.Pair, options ...*GetUMOpenAlgoOrderRequest) (*UMAlgoOrder, error) {
	option := new(GetUMOpenAlgoOrderRequest)
	if len(options) != 0 && options[0] != nil {
		option = options[0]
	}

	if option.AlgoID == 0 && option.ClientAlgoID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{}
	var resp *UMAlgoOrder
	if option.AlgoID != 0 {
		params.Set("algoId", strconv.FormatUint(option.AlgoID, 10))
	}
	if option.ClientAlgoID != "" {
		params.Set("clientAlgoId", option.ClientAlgoID)
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/algo/algoOrder", params, pmDefaultRate, nil, &resp)
}

// GetUMFuturesAccountConfig returns the portfolio margin UM account level configuration.
func (e *Exchange) GetUMFuturesAccountConfig(ctx context.Context) (*UMFuturesAccountConfig, error) {
	var resp *UMFuturesAccountConfig
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/accountConfig", nil, pmUmAccountConfigRate, nil, &resp)
}

// GetUMFuturesSymbolConfig returns the per symbol portfolio margin UM configuration.
func (e *Exchange) GetUMFuturesSymbolConfig(ctx context.Context, symbol currency.Pair) ([]*UMFuturesSymbolConfig, error) {
	params := url.Values{}
	if !symbol.IsEmpty() {
		params.Set("symbol", symbol.String())
	}
	var resp []*UMFuturesSymbolConfig
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/symbolConfig", params, pmUmSymbolConfigRate, nil, &resp)
}

// RepayMarginDebt repays margin account debt for an asset.
func (e *Exchange) RepayMarginDebt(ctx context.Context, assetCode currency.Code, amount float64, specifyRepayAssets []string) (*MarginRepayDebtResponse, error) {
	if assetCode.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if amount <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	params := url.Values{}
	params.Set("asset", assetCode.String())
	params.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	if len(specifyRepayAssets) > 0 {
		params.Set("specifyRepayAssets", strings.Join(specifyRepayAssets, ","))
	}
	var resp *MarginRepayDebtResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/margin/repay-debt", params, pmMarginRepayDebtRate, nil, &resp)
}

// GetUMTransactionHistoryDownloadID requests an async download of portfolio margin UM income history.
func (e *Exchange) GetUMTransactionHistoryDownloadID(ctx context.Context, startTime, endTime time.Time) (*UTransactionDownloadID, error) {
	return e.pmDownloadID(ctx, "/papi/v1/um/income/asyn", startTime, endTime)
}

// GetUMOrderHistoryDownloadID requests an async download of portfolio margin UM order history.
func (e *Exchange) GetUMOrderHistoryDownloadID(ctx context.Context, startTime, endTime time.Time) (*UTransactionDownloadID, error) {
	return e.pmDownloadID(ctx, "/papi/v1/um/order/asyn", startTime, endTime)
}

// GetUMTradeHistoryDownloadID requests an async download of portfolio margin UM trade history.
func (e *Exchange) GetUMTradeHistoryDownloadID(ctx context.Context, startTime, endTime time.Time) (*UTransactionDownloadID, error) {
	return e.pmDownloadID(ctx, "/papi/v1/um/trade/asyn", startTime, endTime)
}

func (e *Exchange) pmDownloadID(ctx context.Context, path string, startTime, endTime time.Time) (*UTransactionDownloadID, error) {
	if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	var resp *UTransactionDownloadID
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, pmDefaultRate, nil, &resp)
}

// GetUMTransactionHistoryDownloadLink resolves a portfolio margin UM income download ID to a link.
func (e *Exchange) GetUMTransactionHistoryDownloadLink(ctx context.Context, downloadID string) (*UTransactionHistoryDownloadLink, error) {
	return e.pmDownloadLinkByID(ctx, downloadID, "/papi/v1/um/income/asyn/id")
}

// GetUMOrderHistoryDownloadLink resolves a portfolio margin UM order download ID to a link.
func (e *Exchange) GetUMOrderHistoryDownloadLink(ctx context.Context, downloadID string) (*UTransactionHistoryDownloadLink, error) {
	return e.pmDownloadLinkByID(ctx, downloadID, "/papi/v1/um/order/asyn/id")
}

// GetUMTradeHistoryDownloadLink resolves a portfolio margin UM trade download ID to a link.
func (e *Exchange) GetUMTradeHistoryDownloadLink(ctx context.Context, downloadID string) (*UTransactionHistoryDownloadLink, error) {
	return e.pmDownloadLinkByID(ctx, downloadID, "/papi/v1/um/trade/asyn/id")
}

func (e *Exchange) pmDownloadLinkByID(ctx context.Context, downloadID, path string) (*UTransactionHistoryDownloadLink, error) {
	if downloadID == "" {
		return nil, errDownloadIDRequired
	}
	var resp *UTransactionHistoryDownloadLink
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, url.Values{"downloadId": {downloadID}}, pmDefaultRate, nil, &resp)
}

// GetUMOrderModifyHistory returns the amendment history of portfolio margin UM orders.
func (e *Exchange) GetUMOrderModifyHistory(ctx context.Context, arg *GetUMOrderModifyHistoryRequest) ([]*USDTAmendInfo, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, orderID, origClientOrderID := arg.Symbol, arg.OrderID, arg.OrigClientOrderID
	startTime, endTime, limit := arg.StartTime, arg.EndTime, arg.Limit
	return e.pmOrderModifyHistory(ctx, "/papi/v1/um/orderAmendment", symbol, orderID, origClientOrderID, startTime, endTime, limit)
}

// GetCMOrderModifyHistory returns the amendment history of portfolio margin CM orders.
func (e *Exchange) GetCMOrderModifyHistory(ctx context.Context, arg *GetCMOrderModifyHistoryRequest) ([]*USDTAmendInfo, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, orderID, origClientOrderID := arg.Symbol, arg.OrderID, arg.OrigClientOrderID
	startTime, endTime, limit := arg.StartTime, arg.EndTime, arg.Limit
	return e.pmOrderModifyHistory(ctx, "/papi/v1/cm/orderAmendment", symbol, orderID, origClientOrderID, startTime, endTime, limit)
}

func (e *Exchange) pmOrderModifyHistory(ctx context.Context, path string, symbol currency.Pair, orderID uint64, origClientOrderID string, startTime, endTime time.Time, limit int64) ([]*USDTAmendInfo, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if orderID > 0 {
		params.Set("orderId", strconv.FormatUint(orderID, 10))
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*USDTAmendInfo
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, path, params, pmDefaultRate, nil, &resp)
}

// GetUMFeeBurnStatus reports whether the BNB fee discount is enabled for portfolio margin UM futures.
func (e *Exchange) GetUMFeeBurnStatus(ctx context.Context) (*UFuturesFeeBurnStatus, error) {
	var resp *UFuturesFeeBurnStatus
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/um/feeBurn", nil, pmUmFeeBurnRate, nil, &resp)
}

// SetUMFeeBurn toggles the BNB fee discount for portfolio margin UM futures.
func (e *Exchange) SetUMFeeBurn(ctx context.Context, enabled bool) error {
	params := url.Values{}
	params.Set("feeBurn", strconv.FormatBool(enabled))
	return e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPost, "/papi/v1/um/feeBurn", params, pmDefaultRate, nil, &struct{}{})
}

// GetCMConditionalOpenOrder returns a single open portfolio margin CM conditional order.
func (e *Exchange) GetCMConditionalOpenOrder(ctx context.Context, symbol currency.Pair, strategyID uint64, newClientStrategyID string) (*ConditionalOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if strategyID == 0 && newClientStrategyID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if strategyID != 0 {
		params.Set("strategyId", strconv.FormatUint(strategyID, 10))
	}
	if newClientStrategyID != "" {
		params.Set("newClientStrategyId", newClientStrategyID)
	}
	var resp *ConditionalOrder
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodGet, "/papi/v1/cm/conditional/openOrder", params, pmDefaultRate, nil, &resp)
}
