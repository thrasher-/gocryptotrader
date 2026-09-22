package binance

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

// FuturesOrderModificationRequest identifies a LIMIT order and its replacement values.
// COIN-M accepts either quantity or price; portfolio margin requires quantity and
// price (or priceMatch). An explicit price and priceMatch cannot be combined.
type FuturesOrderModificationRequest struct {
	Symbol                currency.Pair `json:"symbol"`
	Side                  order.Side    `json:"side"`
	OrderID               uint64        `json:"orderId,omitempty"`
	OriginalClientOrderID string        `json:"origClientOrderId,omitempty"`
	Quantity              float64       `json:"quantity,omitempty"`
	Price                 float64       `json:"price,omitempty"`
	PriceMatch            string        `json:"priceMatch,omitempty"`
	ModifyID              uint64        `json:"modifyId,omitempty"`
	RecvWindow            uint64        `json:"recvWindow,omitempty"`
}

func validateFuturesOrderModification(arg *FuturesOrderModificationRequest, portfolio bool) error {
	if err := common.NilGuard(arg); err != nil {
		return err
	}
	if arg.Symbol.IsEmpty() {
		return currency.ErrCurrencyPairEmpty
	}
	if arg.OrderID == 0 && arg.OriginalClientOrderID == "" {
		return order.ErrOrderIDNotSet
	}
	if arg.Side != order.Buy && arg.Side != order.Sell {
		return order.ErrSideIsInvalid
	}
	if arg.Quantity < 0 || (portfolio && arg.Quantity == 0) {
		return limits.ErrAmountBelowMin
	}
	if arg.Price < 0 || (portfolio && arg.Price == 0 && arg.PriceMatch == "") {
		return limits.ErrPriceBelowMin
	}
	if arg.Price != 0 && arg.PriceMatch != "" {
		return errInvalidOrderQueryCombination
	}
	if arg.Quantity == 0 && arg.Price == 0 && arg.PriceMatch == "" {
		return common.ErrEmptyParams
	}
	return nil
}

// ModifyCFuturesOrder updates a COIN-M order; the exchange moves it to the back of its price queue.
func (e *Exchange) ModifyCFuturesOrder(ctx context.Context, arg *FuturesOrderModificationRequest) (*FuturesOrderData, error) {
	if err := validateFuturesOrderModification(arg, false); err != nil {
		return nil, err
	}
	symbol, err := e.FormatSymbol(arg.Symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	var response *FuturesOrderData
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPut, "/dapi/v1/order", url.Values{symbolParam: {symbol}}, cFuturesOrdersDefaultRate, arg, &response)
}

// CFuturesOrderModificationsRequest sets a request-wide receive window for a batch.
type CFuturesOrderModificationsRequest struct {
	BatchOrders []*FuturesOrderModificationRequest `json:"batchOrders"`
	RecvWindow  uint64                             `json:"recvWindow,omitempty"`
}

// ModifyCFuturesOrders modifies up to five COIN-M orders in one request.
func (e *Exchange) ModifyCFuturesOrders(ctx context.Context, arg *CFuturesOrderModificationsRequest) ([]*FuturesOrderData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if len(arg.BatchOrders) == 0 {
		return nil, common.ErrEmptyParams
	}
	if len(arg.BatchOrders) > 5 {
		return nil, errLimitNumberRequired
	}
	batch := make([]FuturesOrderModificationRequest, len(arg.BatchOrders))
	for i, item := range arg.BatchOrders {
		if err := validateFuturesOrderModification(item, false); err != nil {
			return nil, err
		}
		batch[i] = *item
		var err error
		batch[i].Symbol, err = e.FormatExchangeCurrency(item.Symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}
	params := url.Values{"batchOrders": {string(encoded)}}
	if arg.RecvWindow != 0 {
		params.Set("recvWindow", strconv.FormatUint(arg.RecvWindow, 10))
	}
	var response []*FuturesOrderData
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPut, "/dapi/v1/batchOrders", params, cFuturesBatchOrdersRate, nil, &response)
}

// FuturesPositionModeRequest selects hedge mode (true) or one-way mode (false).
type FuturesPositionModeRequest struct {
	DualSidePosition bool   `json:"dualSidePosition"`
	RecvWindow       uint64 `json:"recvWindow,omitempty"`
}

// ChangeCFuturesPositionMode changes position mode across all COIN-M symbols.
func (e *Exchange) ChangeCFuturesPositionMode(ctx context.Context, arg *FuturesPositionModeRequest) (*SuccessResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	var response *SuccessResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPost, "/dapi/v1/positionSide/dual", nil, cFuturesDefaultRate, arg, &response)
}

// ModifyPMCMOrder modifies a COIN-M LIMIT order in a portfolio-margin account.
func (e *Exchange) ModifyPMCMOrder(ctx context.Context, arg *FuturesOrderModificationRequest) (*UMCMOrder, error) {
	if err := validateFuturesOrderModification(arg, true); err != nil {
		return nil, err
	}
	symbol, err := e.FormatSymbol(arg.Symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	var response *UMCMOrder
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPut, "/papi/v1/cm/order", url.Values{symbolParam: {symbol}}, pmDefaultRate, arg, &response)
}

// ModifyPMUMOrder modifies a USD-M LIMIT order in a portfolio-margin account.
func (e *Exchange) ModifyPMUMOrder(ctx context.Context, arg *FuturesOrderModificationRequest) (*UMCMOrder, error) {
	if err := validateFuturesOrderModification(arg, true); err != nil {
		return nil, err
	}
	symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
	if err != nil {
		return nil, err
	}
	var response *UMCMOrder
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestFuturesSupplementary, http.MethodPut, "/papi/v1/um/order", url.Values{"symbol": {symbol}}, pmDefaultRate, arg, &response)
}
