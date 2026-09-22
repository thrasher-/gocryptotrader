package binance

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

const (
	coinWebsocketAPI = "Coin-Websocket-API"
	usdtWebsocketAPI = "USDT-Websocket-API"
)

func (e *Exchange) futuresAPIConnectionSetup(a asset.Item) *websocket.ConnectionSetup {
	setup := &websocket.ConnectionSetup{
		Handler:                  e.wsHandleSpotAPIData,
		ResponseCheckTimeout:     e.Config.WebsocketResponseCheckTimeout,
		ResponseMaxLimit:         e.Config.WebsocketResponseMaxLimit,
		RateLimit:                request.NewWeightedRateLimitByDuration(250 * time.Millisecond),
		SubscriptionsNotRequired: true,
	}
	switch a {
	case asset.CoinMarginedFutures:
		setup.MessageFilter, setup.URL = coinWebsocketAPI, "wss://ws-dapi.binance.com/ws-dapi/v1"
	case asset.USDTMarginedFutures:
		setup.MessageFilter, setup.URL = usdtWebsocketAPI, "wss://ws-fapi.binance.com/ws-fapi/v1"
	}
	setup.Connector = func(ctx context.Context, conn websocket.Connection) error {
		if err := e.CurrencyPairs.IsAssetEnabled(a); err != nil {
			return err
		}
		if err := conn.Dial(ctx, &gws.Dialer{HandshakeTimeout: e.Config.HTTPTimeout, Proxy: http.ProxyFromEnvironment}, http.Header{}, nil); err != nil {
			return err
		}
		conn.SetupPingHandler(request.UnAuth, websocket.PingHandler{UseGorillaHandler: true, MessageType: gws.PongMessage, Delay: pingDelay})
		return nil
	}
	return setup
}

func (e *Exchange) sendFuturesWSRequest(ctx context.Context, a asset.Item, method string, params map[string]any, result any) error {
	var filter string
	switch a {
	case asset.CoinMarginedFutures:
		filter = coinWebsocketAPI
	case asset.USDTMarginedFutures:
		filter = usdtWebsocketAPI
	default:
		return fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	if params == nil {
		params = make(map[string]any)
	}
	switch {
	case strings.HasPrefix(method, "userDataStream."):
		creds, err := e.GetCredentials(ctx)
		if err != nil {
			return err
		}
		params["apiKey"] = creds.Key
	case method != cnlDepth && method != "ticker.book" && method != "ticker.price":
		params["timestamp"] = time.Now().UnixMilli()
		_, signature, err := e.signRequest(ctx, params)
		if err != nil {
			return err
		}
		params["signature"] = signature
	}
	conn, err := e.Websocket.GetConnection(filter)
	if err != nil {
		return err
	}
	return e.sendWsRequest(ctx, conn, method, params, result)
}

// WsCFuturesGetAccountRequest contains the documented parameters for account.status.
type WsCFuturesGetAccountRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// WsCFuturesGetAccount calls account.status on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesGetAccount(ctx context.Context, arg *WsCFuturesGetAccountRequest) (*FuturesAccountInformation, error) {
	if arg == nil {
		arg = new(WsCFuturesGetAccountRequest)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	var response *FuturesAccountInformation
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "account.status", params, &response)
}

// WsCFuturesGetBalancesRequest contains the documented parameters for account.balance.
type WsCFuturesGetBalancesRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// WsCFuturesGetBalances calls account.balance on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesGetBalances(ctx context.Context, arg *WsCFuturesGetBalancesRequest) ([]*FuturesAccountBalanceData, error) {
	if arg == nil {
		arg = new(WsCFuturesGetBalancesRequest)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	var response []*FuturesAccountBalanceData
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "account.balance", params, &response)
}

// WsCFuturesCancelOrderRequest contains the documented parameters for order.cancel.
type WsCFuturesCancelOrderRequest struct {
	Symbol                currency.Pair `json:"symbol"`
	OrderID               uint64        `json:"orderId,omitempty"`
	OriginalClientOrderID string        `json:"origClientOrderId,omitempty"`
	RecvWindow            uint64        `json:"recvWindow,omitempty"`
}

// WsCFuturesCancelOrder calls order.cancel on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesCancelOrder(ctx context.Context, arg *WsCFuturesCancelOrderRequest) (*FuturesOrderData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.OrderID == 0 && arg.OriginalClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response *FuturesOrderData
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "order.cancel", params, &response)
}

// WsCFuturesModifyOrderRequest contains the documented parameters for order.modify.
type WsCFuturesModifyOrderRequest struct {
	Symbol                currency.Pair `json:"symbol"`
	Side                  order.Side    `json:"side"`
	Quantity              float64       `json:"quantity"`
	Price                 float64       `json:"price"`
	OrderID               uint64        `json:"orderId,omitempty"`
	OriginalClientOrderID string        `json:"origClientOrderId,omitempty"`
	PriceMatch            string        `json:"priceMatch,omitempty"`
	ModifyID              uint64        `json:"modifyId,omitempty"`
	RecvWindow            uint64        `json:"recvWindow,omitempty"`
}

// WsCFuturesModifyOrder calls order.modify on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesModifyOrder(ctx context.Context, arg *WsCFuturesModifyOrderRequest) (*FuturesOrderData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side != order.Buy && arg.Side != order.Sell {
		return nil, order.ErrSideIsInvalid
	}
	if arg.Quantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.Price <= 0 && arg.PriceMatch == "" {
		return nil, limits.ErrPriceBelowMin
	}
	if arg.OrderID == 0 && arg.OriginalClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	if arg.Price != 0 && arg.PriceMatch != "" {
		return nil, errInvalidOrderQueryCombination
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response *FuturesOrderData
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "order.modify", params, &response)
}

// WsCFuturesNewOrderRequest contains the documented parameters for order.place.
type WsCFuturesNewOrderRequest struct {
	Symbol                  currency.Pair     `json:"symbol"`
	Side                    order.Side        `json:"side"`
	OrderType               string            `json:"type"`
	PositionSide            string            `json:"positionSide,omitempty"`
	TimeInForce             order.TimeInForce `json:"timeInForce,omitempty"`
	Quantity                float64           `json:"quantity,omitempty"`
	ReduceOnly              string            `json:"reduceOnly,omitempty"`
	Price                   float64           `json:"price,omitempty"`
	NewClientOrderID        string            `json:"newClientOrderId,omitempty"`
	StopPrice               float64           `json:"stopPrice,omitempty"`
	ClosePosition           string            `json:"closePosition,omitempty"`
	ActivationPrice         float64           `json:"activationPrice,omitempty"`
	CallbackRate            float64           `json:"callbackRate,omitempty"`
	WorkingType             string            `json:"workingType,omitempty"`
	PriceProtect            string            `json:"priceProtect,omitempty"`
	NewOrderRespType        string            `json:"newOrderRespType,omitempty"`
	PriceMatch              string            `json:"priceMatch,omitempty"`
	SelfTradePreventionMode string            `json:"selfTradePreventionMode,omitempty"`
	RecvWindow              uint64            `json:"recvWindow,omitempty"`
}

// WsCFuturesNewOrder calls order.place on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesNewOrder(ctx context.Context, arg *WsCFuturesNewOrderRequest) (*FuturesOrderPlaceData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side != order.Buy && arg.Side != order.Sell {
		return nil, order.ErrSideIsInvalid
	}
	if arg.OrderType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	if arg.Price != 0 && arg.PriceMatch != "" {
		return nil, errInvalidOrderQueryCombination
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response *FuturesOrderPlaceData
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "order.place", params, &response)
}

// WsCFuturesGetPositionsRequest contains the documented parameters for account.position.
type WsCFuturesGetPositionsRequest struct {
	MarginAsset currency.Code `json:"marginAsset,omitzero"`
	Pair        currency.Pair `json:"pair,omitzero"`
	RecvWindow  uint64        `json:"recvWindow,omitempty"`
}

// WsCFuturesGetPositions calls account.position on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesGetPositions(ctx context.Context, arg *WsCFuturesGetPositionsRequest) ([]*FuturesPositionInformation, error) {
	if arg == nil {
		arg = new(WsCFuturesGetPositionsRequest)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Pair.IsEmpty() {
		params["pair"] = arg.Pair.Format(currency.PairFormat{Uppercase: true}).String()
	} else {
		delete(params, "pair")
	}
	var response []*FuturesPositionInformation
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "account.position", params, &response)
}

// WsCFuturesGetOrderRequest contains the documented parameters for order.status.
type WsCFuturesGetOrderRequest struct {
	Symbol                currency.Pair `json:"symbol"`
	OrderID               uint64        `json:"orderId,omitempty"`
	OriginalClientOrderID string        `json:"origClientOrderId,omitempty"`
	RecvWindow            uint64        `json:"recvWindow,omitempty"`
}

// WsCFuturesGetOrder calls order.status on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesGetOrder(ctx context.Context, arg *WsCFuturesGetOrderRequest) (*FuturesOrderGetData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.OrderID == 0 && arg.OriginalClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response *FuturesOrderGetData
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "order.status", params, &response)
}

// WsCFuturesCloseUserDataStream calls userDataStream.stop on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesCloseUserDataStream(ctx context.Context) error {
	var params map[string]any
	return e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "userDataStream.stop", params, nil)
}

// WsCFuturesKeepUserDataStreamAlive calls userDataStream.ping on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesKeepUserDataStreamAlive(ctx context.Context) (*ListenKeyResponse, error) {
	var params map[string]any
	var response *ListenKeyResponse
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "userDataStream.ping", params, &response)
}

// WsCFuturesStartUserDataStream calls userDataStream.start on the COIN-M WebSocket API.
func (e *Exchange) WsCFuturesStartUserDataStream(ctx context.Context) (*ListenKeyResponse, error) {
	var params map[string]any
	var response *ListenKeyResponse
	return response, e.sendFuturesWSRequest(ctx, asset.CoinMarginedFutures, "userDataStream.start", params, &response)
}

// WsUFuturesGetAccountRequest contains the documented parameters for account.status.
type WsUFuturesGetAccountRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// WsUFuturesGetAccount calls account.status on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetAccount(ctx context.Context, arg *WsUFuturesGetAccountRequest) (*UAccountInformationV2Data, error) {
	if arg == nil {
		arg = new(WsUFuturesGetAccountRequest)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	var response *UAccountInformationV2Data
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "account.status", params, &response)
}

// WsUFuturesGetAccountV2Request contains the documented parameters for v2/account.status.
type WsUFuturesGetAccountV2Request struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// WsUFuturesGetAccountV2 calls v2/account.status on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetAccountV2(ctx context.Context, arg *WsUFuturesGetAccountV2Request) (*UFuturesAccountV3, error) {
	if arg == nil {
		arg = new(WsUFuturesGetAccountV2Request)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	var response *UFuturesAccountV3
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "v2/account.status", params, &response)
}

// WsUFuturesGetBalancesRequest contains the documented parameters for account.balance.
type WsUFuturesGetBalancesRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// WsUFuturesGetBalances calls account.balance on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetBalances(ctx context.Context, arg *WsUFuturesGetBalancesRequest) ([]*UFuturesAccountBalance, error) {
	if arg == nil {
		arg = new(WsUFuturesGetBalancesRequest)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	var response []*UFuturesAccountBalance
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "account.balance", params, &response)
}

// WsUFuturesGetBalancesV2Request contains the documented parameters for v2/account.balance.
type WsUFuturesGetBalancesV2Request struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// WsUFuturesGetBalancesV2 calls v2/account.balance on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetBalancesV2(ctx context.Context, arg *WsUFuturesGetBalancesV2Request) ([]*UFuturesAccountBalance, error) {
	if arg == nil {
		arg = new(WsUFuturesGetBalancesV2Request)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	var response []*UFuturesAccountBalance
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "v2/account.balance", params, &response)
}

// WsUFuturesGetOrderbookRequest contains the documented parameters for depth.
type WsUFuturesGetOrderbookRequest struct {
	Symbol currency.Pair `json:"symbol"`
	Limit  uint64        `json:"limit,omitempty"`
}

// WsUFuturesGetOrderbook calls depth on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetOrderbook(ctx context.Context, arg *WsUFuturesGetOrderbookRequest) (*OrderBook, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Limit != 0 {
		if err := e.CheckLimit(arg.Limit); err != nil {
			return nil, err
		}
		if arg.Limit > 1000 {
			return nil, errLimitNumberRequired
		}
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response *OrderBook
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "depth", params, &response)
}

// WsUFuturesGetBookTickerRequest contains the documented parameters for ticker.book.
type WsUFuturesGetBookTickerRequest struct {
	Symbol currency.Pair `json:"symbol,omitzero"`
}

// WsUFuturesGetBookTicker calls ticker.book on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetBookTicker(ctx context.Context, arg *WsUFuturesGetBookTickerRequest) ([]*USymbolOrderbookTicker, error) {
	if arg == nil {
		arg = new(WsUFuturesGetBookTickerRequest)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	if !arg.Symbol.IsEmpty() {
		var response *USymbolOrderbookTicker
		err := e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "ticker.book", params, &response)
		if err != nil {
			return nil, err
		}
		return []*USymbolOrderbookTicker{response}, nil
	}
	var response []*USymbolOrderbookTicker
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "ticker.book", params, &response)
}

// WsUFuturesGetPriceTickerRequest contains the documented parameters for ticker.price.
type WsUFuturesGetPriceTickerRequest struct {
	Symbol currency.Pair `json:"symbol,omitzero"`
}

// WsUFuturesGetPriceTicker calls ticker.price on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetPriceTicker(ctx context.Context, arg *WsUFuturesGetPriceTickerRequest) ([]*USymbolPriceTicker, error) {
	if arg == nil {
		arg = new(WsUFuturesGetPriceTickerRequest)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	if !arg.Symbol.IsEmpty() {
		var response *USymbolPriceTicker
		err := e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "ticker.price", params, &response)
		if err != nil {
			return nil, err
		}
		return []*USymbolPriceTicker{response}, nil
	}
	var response []*USymbolPriceTicker
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "ticker.price", params, &response)
}

// WsUFuturesCancelAlgoOrderRequest contains the documented parameters for algoOrder.cancel.
type WsUFuturesCancelAlgoOrderRequest struct {
	AlgoID       uint64 `json:"algoId,omitempty"`
	ClientAlgoID string `json:"clientAlgoId,omitempty"`
	RecvWindow   uint64 `json:"recvWindow,omitempty"`
}

// WsUFuturesCancelAlgoOrder calls algoOrder.cancel on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesCancelAlgoOrder(ctx context.Context, arg *WsUFuturesCancelAlgoOrderRequest) (*UFuturesAlgoOrderCancelResponse, error) {
	if arg == nil {
		arg = new(WsUFuturesCancelAlgoOrderRequest)
	}
	if arg.AlgoID == 0 && arg.ClientAlgoID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	var response *UFuturesAlgoOrderCancelResponse
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "algoOrder.cancel", params, &response)
}

// WsUFuturesCancelOrderRequest contains the documented parameters for order.cancel.
type WsUFuturesCancelOrderRequest struct {
	Symbol                currency.Pair `json:"symbol"`
	OrderID               uint64        `json:"orderId,omitempty"`
	OriginalClientOrderID string        `json:"origClientOrderId,omitempty"`
	RecvWindow            uint64        `json:"recvWindow,omitempty"`
}

// WsUFuturesCancelOrder calls order.cancel on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesCancelOrder(ctx context.Context, arg *WsUFuturesCancelOrderRequest) (*UOrderData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.OrderID == 0 && arg.OriginalClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response *UOrderData
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "order.cancel", params, &response)
}

// WsUFuturesModifyOrderRequest contains the documented parameters for order.modify.
type WsUFuturesModifyOrderRequest struct {
	Symbol                currency.Pair `json:"symbol"`
	Side                  order.Side    `json:"side"`
	Quantity              float64       `json:"quantity"`
	Price                 float64       `json:"price"`
	OrderID               uint64        `json:"orderId,omitempty"`
	OriginalClientOrderID string        `json:"origClientOrderId,omitempty"`
	PriceMatch            string        `json:"priceMatch,omitempty"`
	ModifyID              uint64        `json:"modifyId,omitempty"`
	ReduceOnly            string        `json:"reduceOnly,omitempty"`
	RecvWindow            uint64        `json:"recvWindow,omitempty"`
}

// WsUFuturesModifyOrder calls order.modify on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesModifyOrder(ctx context.Context, arg *WsUFuturesModifyOrderRequest) (*UOrderData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side != order.Buy && arg.Side != order.Sell {
		return nil, order.ErrSideIsInvalid
	}
	if arg.Quantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.Price <= 0 && arg.PriceMatch == "" {
		return nil, limits.ErrPriceBelowMin
	}
	if arg.OrderID == 0 && arg.OriginalClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	if arg.Price != 0 && arg.PriceMatch != "" {
		return nil, errInvalidOrderQueryCombination
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response *UOrderData
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "order.modify", params, &response)
}

// WsUFuturesNewAlgoOrderRequest contains the documented parameters for algoOrder.place.
type WsUFuturesNewAlgoOrderRequest struct {
	AlgoType                string            `json:"algoType"`
	Symbol                  currency.Pair     `json:"symbol"`
	Side                    order.Side        `json:"side"`
	OrderType               string            `json:"type"`
	PositionSide            string            `json:"positionSide,omitempty"`
	TimeInForce             order.TimeInForce `json:"timeInForce,omitempty"`
	Quantity                float64           `json:"quantity,omitempty"`
	Price                   float64           `json:"price,omitempty"`
	TriggerPrice            float64           `json:"triggerPrice,omitempty"`
	WorkingType             string            `json:"workingType,omitempty"`
	PriceMatch              string            `json:"priceMatch,omitempty"`
	ClosePosition           string            `json:"closePosition,omitempty"`
	PriceProtect            string            `json:"priceProtect,omitempty"`
	ReduceOnly              string            `json:"reduceOnly,omitempty"`
	ActivatePrice           float64           `json:"activatePrice,omitempty"`
	CallbackRate            float64           `json:"callbackRate,omitempty"`
	ClientAlgoID            string            `json:"clientAlgoId,omitempty"`
	NewOrderRespType        string            `json:"newOrderRespType,omitempty"`
	SelfTradePreventionMode string            `json:"selfTradePreventionMode,omitempty"`
	GoodTillDate            time.Time         `json:"-"`
	RecvWindow              uint64            `json:"recvWindow,omitempty"`
}

// WsUFuturesNewAlgoOrder calls algoOrder.place on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesNewAlgoOrder(ctx context.Context, arg *WsUFuturesNewAlgoOrderRequest) (*UFuturesAlgoOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.AlgoType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side != order.Buy && arg.Side != order.Sell {
		return nil, order.ErrSideIsInvalid
	}
	if arg.OrderType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	if arg.Price != 0 && arg.PriceMatch != "" {
		return nil, errInvalidOrderQueryCombination
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	if !arg.GoodTillDate.IsZero() {
		params["goodTillDate"] = arg.GoodTillDate.UnixMilli()
	}
	var response *UFuturesAlgoOrder
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "algoOrder.place", params, &response)
}

// WsUFuturesNewOrderRequest contains the documented parameters for order.place.
type WsUFuturesNewOrderRequest struct {
	Symbol                  currency.Pair     `json:"symbol"`
	Side                    order.Side        `json:"side"`
	OrderType               string            `json:"type"`
	PositionSide            string            `json:"positionSide,omitempty"`
	TimeInForce             order.TimeInForce `json:"timeInForce,omitempty"`
	ReduceOnly              string            `json:"reduceOnly,omitempty"`
	Quantity                float64           `json:"quantity,omitempty"`
	Price                   float64           `json:"price,omitempty"`
	NewClientOrderID        string            `json:"newClientOrderId,omitempty"`
	NewOrderRespType        string            `json:"newOrderRespType,omitempty"`
	PriceMatch              string            `json:"priceMatch,omitempty"`
	SelfTradePreventionMode string            `json:"selfTradePreventionMode,omitempty"`
	GoodTillDate            time.Time         `json:"-"`
	RecvWindow              uint64            `json:"recvWindow,omitempty"`
}

// WsUFuturesNewOrder calls order.place on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesNewOrder(ctx context.Context, arg *WsUFuturesNewOrderRequest) (*UOrderData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side != order.Buy && arg.Side != order.Sell {
		return nil, order.ErrSideIsInvalid
	}
	if arg.OrderType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	if arg.Price != 0 && arg.PriceMatch != "" {
		return nil, errInvalidOrderQueryCombination
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	if !arg.GoodTillDate.IsZero() {
		params["goodTillDate"] = arg.GoodTillDate.UnixMilli()
	}
	var response *UOrderData
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "order.place", params, &response)
}

// WsUFuturesGetPositionsRequest contains the documented parameters for account.position.
type WsUFuturesGetPositionsRequest struct {
	Symbol     currency.Pair `json:"symbol,omitzero"`
	RecvWindow uint64        `json:"recvWindow,omitempty"`
}

// WsUFuturesGetPositions calls account.position on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetPositions(ctx context.Context, arg *WsUFuturesGetPositionsRequest) ([]*UPositionInformationV2, error) {
	if arg == nil {
		arg = new(WsUFuturesGetPositionsRequest)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response []*UPositionInformationV2
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "account.position", params, &response)
}

// WsUFuturesGetPositionsV2Request contains the documented parameters for v2/account.position.
type WsUFuturesGetPositionsV2Request struct {
	Symbol     currency.Pair `json:"symbol,omitzero"`
	RecvWindow uint64        `json:"recvWindow,omitempty"`
}

// WsUFuturesGetPositionsV2 calls v2/account.position on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetPositionsV2(ctx context.Context, arg *WsUFuturesGetPositionsV2Request) ([]*UFuturesPositionV3, error) {
	if arg == nil {
		arg = new(WsUFuturesGetPositionsV2Request)
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response []*UFuturesPositionV3
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "v2/account.position", params, &response)
}

// WsUFuturesGetOrderRequest contains the documented parameters for order.status.
type WsUFuturesGetOrderRequest struct {
	Symbol                currency.Pair `json:"symbol"`
	OrderID               uint64        `json:"orderId,omitempty"`
	OriginalClientOrderID string        `json:"origClientOrderId,omitempty"`
	RecvWindow            uint64        `json:"recvWindow,omitempty"`
}

// WsUFuturesGetOrder calls order.status on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesGetOrder(ctx context.Context, arg *WsUFuturesGetOrderRequest) (*UOrderData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.OrderID == 0 && arg.OriginalClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if !arg.Symbol.IsEmpty() {
		symbol, err := e.FormatSymbol(arg.Symbol, asset.USDTMarginedFutures)
		if err != nil {
			return nil, err
		}
		params["symbol"] = symbol
	} else {
		delete(params, "symbol")
	}
	var response *UOrderData
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "order.status", params, &response)
}

// WsUFuturesCloseUserDataStream calls userDataStream.stop on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesCloseUserDataStream(ctx context.Context) error {
	var params map[string]any
	return e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "userDataStream.stop", params, nil)
}

// WsUFuturesKeepUserDataStreamAlive calls userDataStream.ping on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesKeepUserDataStreamAlive(ctx context.Context) (*ListenKeyResponse, error) {
	var params map[string]any
	var response *ListenKeyResponse
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "userDataStream.ping", params, &response)
}

// WsUFuturesStartUserDataStream calls userDataStream.start on the USD-M WebSocket API.
func (e *Exchange) WsUFuturesStartUserDataStream(ctx context.Context) (*ListenKeyResponse, error) {
	var params map[string]any
	var response *ListenKeyResponse
	return response, e.sendFuturesWSRequest(ctx, asset.USDTMarginedFutures, "userDataStream.start", params, &response)
}
