package binance

import (
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// WsServerTimeResponse contains the Spot API server's clock.
type WsServerTimeResponse struct {
	ServerTime types.Time `json:"serverTime"`
}

// WsMarketSymbolsRequest selects one, several or all Spot symbols.
type WsMarketSymbolsRequest struct {
	Symbol       currency.Pair  `json:"symbol,omitzero"`
	Symbols      currency.Pairs `json:"symbols,omitempty"`
	SymbolStatus string         `json:"symbolStatus,omitempty"`
}

func spotSymbolParams(symbol currency.Pair, symbols currency.Pairs, status string) map[string]any {
	params := make(map[string]any)
	format := currency.PairFormat{Uppercase: true}
	if !symbol.IsEmpty() {
		params["symbol"] = symbol.Format(format).String()
	}
	if len(symbols) != 0 {
		params["symbols"] = symbols.Format(format).Strings()
	}
	if status != "" {
		params["symbolStatus"] = status
	}
	return params
}

// Exchange information and execution rules accept a symbol selection or filters.
func validateSpotSymbolSelection(symbol currency.Pair, symbols currency.Pairs, status string, permissions []string) error {
	if (!symbol.IsEmpty() && len(symbols) != 0) ||
		((!symbol.IsEmpty() || len(symbols) != 0) && (status != "" || len(permissions) != 0)) {
		return errInvalidOrderQueryCombination
	}
	return nil
}

// WsPing checks the Spot WebSocket API connection.
func (e *Exchange) WsPing() error { return e.SendWsRequest("ping", nil, &struct{}{}) }

// GetWsServerTime returns the Spot server clock over WebSocket.
func (e *Exchange) GetWsServerTime() (*WsServerTimeResponse, error) {
	var response *WsServerTimeResponse
	return response, e.SendWsRequest("time", nil, &response)
}

// GetWsExchangeInfo returns symbol rules and permissions over WebSocket.
func (e *Exchange) GetWsExchangeInfo(arg *GetExchangeInfoRequest) (*ExchangeInfo, error) {
	params := make(map[string]any)
	if arg != nil {
		if err := validateSpotSymbolSelection(arg.Symbol, arg.Symbols, arg.SymbolStatus, arg.Permissions); err != nil {
			return nil, err
		}
		params = spotSymbolParams(arg.Symbol, arg.Symbols, arg.SymbolStatus)
		if len(arg.Permissions) != 0 {
			params["permissions"] = arg.Permissions
		}
		if arg.ShowPermissionSets != nil {
			params["showPermissionSets"] = *arg.ShowPermissionSets
		}
	}
	var response *ExchangeInfo
	return response, e.SendWsRequest("exchangeInfo", params, &response)
}

// GetWsExecutionRules returns the execution rules for the selected symbols.
func (e *Exchange) GetWsExecutionRules(arg *WsMarketSymbolsRequest) (*SymbolExecutionRules, error) {
	params := make(map[string]any)
	if arg != nil {
		if err := validateSpotSymbolSelection(arg.Symbol, arg.Symbols, arg.SymbolStatus, nil); err != nil {
			return nil, err
		}
		params = spotSymbolParams(arg.Symbol, arg.Symbols, arg.SymbolStatus)
	}
	var response *SymbolExecutionRules
	return response, e.SendWsRequest("executionRules", params, &response)
}

// GetWsHistoricalTrades returns trades from the given ID, or the most recent historical trades.
func (e *Exchange) GetWsHistoricalTrades(symbol currency.Pair, fromID, limit uint64) ([]*HistoricalTrade, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := spotSymbolParams(symbol, nil, "")
	if fromID != 0 {
		params["fromId"] = fromID
	}
	if limit != 0 {
		params["limit"] = limit
	}
	var response []*HistoricalTrade
	return response, e.SendWsRequest("trades.historical", params, &response)
}

// GetWsHistoricalBlockTrades returns Spot block trades from the supplied ID.
func (e *Exchange) GetWsHistoricalBlockTrades(symbol currency.Pair, fromID, limit uint64) ([]*HistoricalBlockTrade, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := spotSymbolParams(symbol, nil, "")
	params["fromId"] = fromID
	if limit != 0 {
		params["limit"] = limit
	}
	var response []*HistoricalBlockTrade
	return response, e.SendWsRequest("blockTrades.historical", params, &response)
}

// GetWsReferencePrice returns the price used by a symbol's execution rules.
func (e *Exchange) GetWsReferencePrice(symbol currency.Pair) (*ReferencePrice, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	var response *ReferencePrice
	return response, e.SendWsRequest("referencePrice", spotSymbolParams(symbol, nil, ""), &response)
}

// GetWsReferencePriceCalculation returns how the symbol's reference price is calculated.
func (e *Exchange) GetWsReferencePriceCalculation(symbol currency.Pair, symbolStatus string) (*ReferencePriceCalculation, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	var response *ReferencePriceCalculation
	return response, e.SendWsRequest("referencePrice.calculation", spotSymbolParams(symbol, nil, symbolStatus), &response)
}

// GetWsAccountSymbolFilters returns account-specific symbol filters.
func (e *Exchange) GetWsAccountSymbolFilters(symbol currency.Pair, recvWindow float64) (*AccountFiltersResponse, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := spotSymbolParams(symbol, nil, "")
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	var response *AccountFiltersResponse
	return response, e.sendSignedWsRequest("myFilters", params, &response)
}

// GetWsOrderAmendments returns an order's amendments, retaining exact execution IDs.
func (e *Exchange) GetWsOrderAmendments(arg *GetOrderAmendmentsRequest, recvWindow float64) ([]*OrderAmendment, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.OrderID == 0 {
		return nil, order.ErrOrderIDNotSet
	}
	params := spotSymbolParams(arg.Symbol, nil, "")
	params["orderId"] = arg.OrderID
	if arg.FromExecutionID != 0 {
		params["fromExecutionId"] = arg.FromExecutionID
	}
	if arg.Limit != 0 {
		params["limit"] = arg.Limit
	}
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	var response []*OrderAmendment
	return response, e.sendSignedWsRequest("order.amendments", params, &response)
}

// WsAmendOrderKeepPriority reduces an order's quantity without losing its priority.
func (e *Exchange) WsAmendOrderKeepPriority(arg *AmendKeepPriorityRequest, recvWindow float64) (*AmendKeepPriorityResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.OrderID == 0 && arg.OrigClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	if arg.NewQuantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	params["symbol"] = arg.Symbol.Format(currency.PairFormat{Uppercase: true}).String()
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	var response *AmendKeepPriorityResponse
	return response, e.sendSignedWsRequest("order.amend.keepPriority", params, &response)
}

// WsNewOCOOrderList places an OCO using the current order-list API.
func (e *Exchange) WsNewOCOOrderList(arg *OCOOrderListRequest, recvWindow float64) (*OCOListOrderResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Quantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.Side == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.AboveType == "" || arg.BelowType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	params["symbol"] = arg.Symbol.Format(currency.PairFormat{Uppercase: true}).String()
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	var response *OCOListOrderResponse
	return response, e.sendSignedWsRequest("orderList.place.oco", params, &response)
}

// WsNewOTOOrder places a working order and a pending order activated by its fill.
func (e *Exchange) WsNewOTOOrder(arg *OTOOrderRequest, recvWindow float64) (*OCOOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.WorkingQuantity <= 0 || arg.PendingQuantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.WorkingSide == "" || arg.PendingSide == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.WorkingType == "" || arg.PendingType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	params["symbol"] = arg.Symbol.Format(currency.PairFormat{Uppercase: true}).String()
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	var response *OCOOrder
	return response, e.sendSignedWsRequest("orderList.place.oto", params, &response)
}

// WsNewOTOCOOrder places a working order that activates a pending OCO pair.
func (e *Exchange) WsNewOTOCOOrder(arg *OTOCOOrderRequest, recvWindow float64) (*OCOOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.WorkingQuantity <= 0 || arg.PendingQuantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.WorkingSide == "" || arg.PendingSide == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.WorkingType == "" || arg.PendingAboveType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	params["symbol"] = arg.Symbol.Format(currency.PairFormat{Uppercase: true}).String()
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	var response *OCOOrder
	return response, e.sendSignedWsRequest("orderList.place.otoco", params, &response)
}
