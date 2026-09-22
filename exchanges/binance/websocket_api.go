package binance

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

const (
	binanceWebsocketAPIURL = "wss://ws-api.binance.com:443/ws-api/v3"
)

const spotWebsocketAPI = "Spot-Websocket-API"

// WsConnectAPI creates a new websocket connection to API server
func (e *Exchange) WsConnectAPI(ctx context.Context, conn websocket.Connection) (err error) {
	if err := e.CurrencyPairs.IsAssetEnabled(asset.Spot); err != nil {
		return err
	}

	// The connection is not being established within the default 15 second timeout. Therefore, we needed to extend it to double the default timeout.
	extendedTimeout := e.Config.HTTPTimeout * 2

	dialer := gws.Dialer{
		HandshakeTimeout: extendedTimeout,
		Proxy:            http.ProxyFromEnvironment,
	}
	if err = conn.Dial(ctx, &dialer, http.Header{}, nil); err != nil {
		return fmt.Errorf("%s - unable to connect to websocket: %w", e.Name, err)
	}

	conn.SetupPingHandler(request.UnAuth, websocket.PingHandler{
		UseGorillaHandler: true,
		MessageType:       gws.PongMessage,
		Delay:             pingDelay,
	})

	return nil
}

// IsAPIStreamConnected checks if the API stream connection is established
func (e *Exchange) IsAPIStreamConnected() bool {
	_, err := e.Websocket.GetConnection(spotWebsocketAPI)
	return err == nil
}

// wsAuthenticateSpotAPI runs after the manager registers the connection and starts
// its reader, so the signed subscription can receive its acknowledgement.
func (e *Exchange) wsAuthenticateSpotAPI(ctx context.Context, conn websocket.Connection) error {
	params := map[string]any{"timestamp": time.Now().UnixMilli()}
	_, signature, err := e.SignRequest(params)
	if err != nil {
		return err
	}
	params["signature"] = signature
	var response *UserDataStreamSubscriptionResponse
	return e.sendWsRequest(ctx, conn, "userDataStream.subscribe.signature", params, &response)
}

// wsHandleSpotAPIData routes API replies and the documented user-data envelope.
func (e *Exchange) wsHandleSpotAPIData(ctx context.Context, conn websocket.Connection, data []byte) error {
	var envelope struct {
		ID    json.RawMessage `json:"id"`
		Event json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if len(envelope.Event) > 0 && string(envelope.Event) != "null" {
		return e.wsHandleData(ctx, conn, data)
	}
	if len(envelope.ID) > 0 && string(envelope.ID) != "null" {
		return matchSubscriptionResponse(conn, envelope.ID, data)
	}
	return fmt.Errorf("%w: %s", errUnsupportedChannel, data)
}

// SendWsRequest sends a Spot WebSocket API request.
func (e *Exchange) SendWsRequest(method string, params, result any) error {
	conn, err := e.Websocket.GetConnection(spotWebsocketAPI)
	if err != nil {
		return err
	}
	return e.sendWsRequest(context.Background(), conn, method, params, result)
}

func (e *Exchange) sendWsRequest(ctx context.Context, conn websocket.Connection, method string, params, result any) error {
	input := &WsAPIRequest{ID: e.MessageID(), Method: method, Params: params}
	data, err := conn.SendMessageReturnResponse(ctx, request.UnAuth, input.ID, input)
	if err != nil {
		return err
	}
	var response WsAPIResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	if err := apiResponseError(response.Error); err != nil {
		return err
	}
	if response.Status != http.StatusOK {
		return fmt.Errorf("%w: websocket status %d", errAPIResponse, response.Status)
	}
	// Some methods, including referencePrice, return errors in result with status 200.
	if err := apiResponseError(response.Result); err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(response.Result, result)
}

// GetWsOrderbook returns full orderbook information
//
// OrderBookDataRequest contains the following members
// symbol: string of currency pair
// limit: returned limit amount
func (e *Exchange) GetWsOrderbook(obd *OrderBookDataRequest) (*OrderBook, error) {
	if err := common.NilGuard(obd); err != nil {
		return nil, err
	}
	if err := e.CheckLimit(obd.Limit); err != nil {
		return nil, err
	}
	var resp *OrderBook
	return resp, e.SendWsRequest("depth", obd, &resp)
}

// GetWsMostRecentTrades returns recent trade activity through the websocket connection
// limit: Up to 500 results returned
func (e *Exchange) GetWsMostRecentTrades(rtr *RecentTradeRequest) ([]*RecentTrade, error) {
	if err := common.NilGuard(rtr); err != nil {
		return nil, err
	}
	if rtr.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	var resp []*RecentTrade
	return resp, e.SendWsRequest("trades.recent", rtr, &resp)
}

// GetWsAggregatedTrades retrieves aggregated trade activity.
func (e *Exchange) GetWsAggregatedTrades(arg *WsAggregateTradeRequest) ([]*AggregatedTrade, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if !arg.StartTime.IsZero() && !arg.EndTime.IsZero() {
		if err := common.StartEndTimeCheck(arg.StartTime, arg.EndTime); err != nil {
			return nil, err
		}
	}
	if !arg.StartTime.IsZero() {
		arg.StartTimestamp = arg.StartTime.UnixMilli()
	}
	if !arg.EndTime.IsZero() {
		arg.EndTimestamp = arg.EndTime.UnixMilli()
	}
	var resp []*AggregatedTrade
	return resp, e.SendWsRequest("trades.aggregate", arg, &resp)
}

// GetWsCandlestick retrieves spot kline data through the websocket connection.
func (e *Exchange) GetWsCandlestick(arg *KlinesRequest) ([]*CandleStick, error) {
	return e.getWsKlines("klines", arg)
}

// GetWsOptimizedCandlestick retrieves spot candlestick bars through the websocket connection.
func (e *Exchange) GetWsOptimizedCandlestick(arg *KlinesRequest) ([]*CandleStick, error) {
	return e.getWsKlines("uiKlines", arg)
}

// getWsKlines retrieves spot kline data through the websocket connection.
func (e *Exchange) getWsKlines(method string, arg *KlinesRequest) ([]*CandleStick, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Interval == "" {
		return nil, kline.ErrInvalidInterval
	}
	if !arg.StartTime.IsZero() && !arg.EndTime.IsZero() {
		if err := common.StartEndTimeCheck(arg.StartTime, arg.EndTime); err != nil {
			return nil, err
		}
	}
	if !arg.StartTime.IsZero() {
		arg.StartTimestamp = arg.StartTime.UnixMilli()
	}
	if !arg.EndTime.IsZero() {
		arg.EndTimestamp = arg.EndTime.UnixMilli()
	}
	var resp []*CandleStick
	return resp, e.SendWsRequest(method, arg, &resp)
}

// GetWsCurrenctAveragePrice retrieves current average price for a symbol.
func (e *Exchange) GetWsCurrenctAveragePrice(symbol currency.Pair) (*SymbolAveragePrice, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	arg := &struct {
		Symbol currency.Pair `json:"symbol"`
	}{
		Symbol: symbol,
	}
	var resp *SymbolAveragePrice
	return resp, e.SendWsRequest("avgPrice", arg, &resp)
}

// GetWs24HourPriceChanges 24-hour rolling window price changes statistics through the websocket stream.
// 'type': 'FULL' (default) or 'MINI'
// 'timeZone' Default: 0 (UTC)
func (e *Exchange) GetWs24HourPriceChanges(arg *PriceChangeRequest) ([]*PriceChangeStats, error) {
	return e.tickerDataChange("ticker.24hr", arg)
}

// GetWsTradingDayTickers price change statistics for a trading day.
// 'type': 'FULL' (default) or 'MINI'
// 'timeZone' Default: 0 (UTC)
func (e *Exchange) GetWsTradingDayTickers(arg *PriceChangeRequest) ([]*PriceChangeStats, error) {
	return e.tickerDataChange("ticker.tradingDay", arg)
}

// tickerDataChange unifying method to make price change requests through the websocket stream.
func (e *Exchange) tickerDataChange(method string, arg *PriceChangeRequest) ([]*PriceChangeStats, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	params := spotSymbolParams(currency.EMPTYPAIR, arg.Symbols, arg.SymbolStatus)
	if arg.Symbol != "" {
		params["symbol"] = arg.Symbol
	}
	if arg.TickerType != "" {
		params["type"] = arg.TickerType
	}
	if method == "ticker.tradingDay" && arg.Timezone != "" {
		params["timeZone"] = arg.Timezone
	}
	var resp PriceChanges
	return resp, e.SendWsRequest(method, params, &resp)
}

// WindowSizeToString converts a ticker window to Binance's duration notation.

// WindowSizeToString formats a supported rolling-window duration.
func (e *Exchange) WindowSizeToString(windowSize time.Duration) string {
	switch {
	case windowSize/(time.Hour*24) > 0:
		return strconv.FormatInt(int64(windowSize/(time.Hour*24)), 10) + "d"
	case (windowSize / time.Hour) > 0:
		return strconv.FormatInt(int64(windowSize/time.Hour), 10) + "h"
	case (windowSize / time.Minute) > 0:
		return strconv.FormatInt(int64((windowSize/time.Minute)), 10) + "m"
	}
	return ""
}

// GetSymbolPriceTicker represents a symbol ticker item information.
func (e *Exchange) GetSymbolPriceTicker(symbol currency.Pair, options ...*WsMarketSymbolsRequest) ([]*SymbolTickerItem, error) {
	params := spotSymbolParams(symbol, nil, "")
	if len(options) != 0 && options[0] != nil {
		params = spotSymbolParams(symbol, options[0].Symbols, options[0].SymbolStatus)
	}
	var resp SymbolTickers
	return resp, e.SendWsRequest("ticker.price", params, &resp)
}

// GetWsRollingWindowPriceChanges retrieves rolling window price change statistics with a custom window.
// this request is similar to ticker.24hr, but statistics are computed on demand using the arbitrary window you specify
func (e *Exchange) GetWsRollingWindowPriceChanges(arg *WsRollingWindowPriceRequest) ([]*PriceChangeStats, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	params := spotSymbolParams(currency.EMPTYPAIR, arg.Symbols, arg.SymbolStatus)
	if arg.Symbol != "" {
		params["symbol"] = arg.Symbol
	}
	if arg.TickerType != "" {
		params["type"] = arg.TickerType
	}
	window := arg.WindowSize
	if arg.WindowSizeDuration != 0 {
		window = e.WindowSizeToString(arg.WindowSizeDuration)
	}
	if window != "" {
		params["windowSize"] = window
	}
	var resp PriceChanges
	return resp, e.SendWsRequest("ticker", params, &resp)
}

// GetWsSymbolOrderbookTicker retrieves the current best price and quantity on the order book.
func (e *Exchange) GetWsSymbolOrderbookTicker(symbols currency.Pairs, symbolStatus ...string) ([]*WsOrderbookTicker, error) {
	for _, symbol := range symbols {
		if symbol.IsEmpty() {
			return nil, currency.ErrCurrencyPairsEmpty
		}
	}
	status := ""
	if len(symbolStatus) != 0 {
		status = symbolStatus[0]
	}
	params := spotSymbolParams(currency.EMPTYPAIR, symbols, status)
	if len(symbols) == 1 {
		params = spotSymbolParams(symbols[0], nil, status)
	}
	var resp WsOrderbookTickers
	return resp, e.SendWsRequest("ticker.book", params, &resp)
}

func (e *Exchange) getSignature(arg any) (apiKey, signature string, err error) {
	mapValue, err := e.ToMap(arg)
	if err != nil {
		return apiKey, signature, err
	}
	return e.SignRequest(mapValue)
}

// SignRequest creates a signature given params map
func (e *Exchange) SignRequest(params map[string]any) (apiKey, signature string, err error) {
	return e.signRequest(context.Background(), params)
}

func (e *Exchange) signRequest(ctx context.Context, params map[string]any) (apiKey, signature string, err error) {
	creds, err := e.GetCredentials(ctx)
	if err != nil {
		return "", "", err
	}
	timestampInfo, okay := params["timestamp"]
	if !okay {
		return "", "", errTimestampInfoRequired
	}
	// Validate against the rendered form rather than the concrete type: values decoded
	// from JSON retain their number literals, and exponent notation would
	// be signed differently from how it is transmitted.
	if _, err := strconv.ParseInt(signatureValue(timestampInfo), 10, 64); err != nil {
		return "", "", fmt.Errorf("%w: invalid timestamp %v", errTimestampInfoRequired, timestampInfo)
	}
	params["apiKey"] = creds.Key
	// Binance signs every parameter except the signature itself. A struct reused for a
	// second request still carries the previous signature, which would otherwise be
	// signed and then overwritten, so the transmitted payload would not match.
	delete(params, "signature")
	keys := sortMapKeys(params)
	var payload strings.Builder
	for i, k := range keys {
		if i > 0 {
			payload.WriteByte('&')
		}
		payload.WriteString(k)
		payload.WriteByte('=')
		payload.WriteString(signatureValue(params[k]))
	}
	signature, err = signPayload([]byte(payload.String()), creds.Secret)
	return creds.Key, signature, err
}

// sortMapKeys returns the map's keys in ascending order, which is the order Binance
// requires request parameters to be in before signing.
func sortMapKeys(params map[string]any) []string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// GetQuerySessionStatus query the status of the WebSocket connection, inspecting which API key (if any) is used to authorize requests.
func (e *Exchange) GetQuerySessionStatus() (*FuturesAuthenticationResponse, error) {
	var resp *FuturesAuthenticationResponse
	return resp, e.SendWsRequest("session.status", nil, &resp)
}

// GetLogOutOfSession forget the API key previously authenticated. If the connection is not authenticated, this request does nothing.
func (e *Exchange) GetLogOutOfSession() (*FuturesAuthenticationResponse, error) {
	var resp *FuturesAuthenticationResponse
	return resp, e.SendWsRequest("session.logout", nil, &resp)
}

// ----------------------------------------------------------- Trading Requests ----------------------------------------------------

// WsPlaceNewOrder place new order
func (e *Exchange) WsPlaceNewOrder(arg *TradeOrderRequest) (*TradeOrderResponse, error) {
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
	arg.Timestamp = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg.APIKey = apiKey
	arg.Signature = signature
	var resp *TradeOrderResponse
	return resp, e.SendWsRequest("order.place", arg, &resp)
}

// ValidatePlaceNewOrderRequest tests whether the request order is valid or not.
func (e *Exchange) ValidatePlaceNewOrderRequest(arg *TradeOrderRequest) error {
	_, err := e.WsTestNewOrder(arg, false)
	return err
}

// WsTestNewOrder validates an order and optionally returns commission estimates.
func (e *Exchange) WsTestNewOrder(arg *TradeOrderRequest, computeCommissionRates bool) (*TestOrderResponse, error) {
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
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	params["computeCommissionRates"] = computeCommissionRates
	var response *TestOrderResponse
	return response, e.sendSignedWsRequest("order.test", params, &response)
}

// WsQueryOrder to query a trade order
func (e *Exchange) WsQueryOrder(arg *QueryOrderRequest) (*TradeOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.OrderID == 0 && arg.OrigClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	arg.Timestamp = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg.APIKey = apiKey
	arg.Signature = signature
	var resp *TradeOrder
	return resp, e.SendWsRequest("order.status", arg, &resp)
}

// WsCancelOrder cancel an active order.
func (e *Exchange) WsCancelOrder(arg *QueryOrderRequest) (*TradeOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.OrderID == 0 && arg.OrigClientOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	arg.Timestamp = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg.APIKey = apiKey
	arg.Signature = signature
	var resp *TradeOrder
	return resp, e.SendWsRequest("order.cancel", &arg, &resp)
}

// WsCancelAndReplaceTradeOrder cancel an existing order and immediately place a new order instead of the cancelled one.
func (e *Exchange) WsCancelAndReplaceTradeOrder(arg *WsCancelAndReplaceRequest) (*WsCancelAndReplaceTradeOrderResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.CancelReplaceMode == "" {
		return nil, errCancelReplaceModeRequired
	}
	if arg.CancelOrderID == "" {
		return nil, fmt.Errorf("cancelOrderId missing, %w", order.ErrOrderIDNotSet)
	}
	if arg.Side == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.OrderType == "" {
		return nil, order.ErrTypeIsInvalid
	}
	arg.Timestamp = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg.APIKey = apiKey
	arg.Signature = signature
	var resp *WsCancelAndReplaceTradeOrderResponse
	return resp, e.SendWsRequest("order.cancelReplace", &arg, &resp)
}

func (e *Exchange) openOrdersFilter(symbol currency.Pair, recvWindow float64) (map[string]any, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	arg := make(map[string]any)
	if recvWindow != 0 {
		arg["recvWindow"] = recvWindow
	}
	arg["symbol"] = symbol
	arg["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(arg)
	if err != nil {
		return nil, err
	}
	arg["apiKey"] = apiKey
	arg["signature"] = signature
	return arg, nil
}

// WsCurrentOpenOrders retrieves list of open orders.
func (e *Exchange) WsCurrentOpenOrders(symbol currency.Pair, recvWindow float64) ([]*TradeOrder, error) {
	arg, err := e.openOrdersFilter(symbol, recvWindow)
	if err != nil {
		return nil, err
	}
	arg["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg["apiKey"] = apiKey
	arg["signature"] = signature
	var resp []*TradeOrder
	return resp, e.SendWsRequest("openOrders.status", arg, &resp)
}

// WsCancelOpenOrders represents an open orders list
func (e *Exchange) WsCancelOpenOrders(symbol currency.Pair, recvWindow float64) ([]*WsCancelOrder, error) {
	arg, err := e.openOrdersFilter(symbol, recvWindow)
	if err != nil {
		return nil, err
	}
	arg["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg["apiKey"] = apiKey
	arg["signature"] = signature
	var resp []*WsCancelOrder
	return resp, e.SendWsRequest("openOrders.cancelAll", arg, &resp)
}

// WsPlaceOCOOrder send in a new one-cancels-the-other (OCO) pair: LIMIT_MAKER + STOP_LOSS/STOP_LOSS_LIMIT orders (called legs), where activation of one order immediately cancels the other.
// Response format for orderReports is selected using the newOrderRespType parameter. The following example is for RESULT response type. See order.place for more examples.
func (e *Exchange) WsPlaceOCOOrder(arg *PlaceOCOOrderRequest) (*OCOOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.Side == "" {
		return nil, order.ErrSideIsInvalid
	}
	if arg.Quantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.TrailingDelta < 0 {
		return nil, errInvalidTrailingDelta
	}
	if arg.StopPrice <= 0 && arg.TrailingDelta == 0 {
		return nil, fmt.Errorf("stopPrice: %w", limits.ErrPriceBelowMin)
	}
	arg.Timestamp = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg.APIKey = apiKey
	arg.Signature = signature
	var resp *OCOOrder
	return resp, e.SendWsRequest("orderList.place", arg, &resp)
}

// WsQueryOCOOrder execution status of an OCO.
func (e *Exchange) WsQueryOCOOrder(origClientOrderID string, orderListID int64, recvWindow float64) (*OCOOrderInfo, error) {
	if origClientOrderID == "" && orderListID == 0 {
		return nil, order.ErrOrderIDNotSet
	}
	params := map[string]any{}
	if origClientOrderID != "" {
		params["origClientOrderId"] = origClientOrderID
	}
	if orderListID != 0 {
		params["orderListId"] = orderListID
	}
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp *OCOOrderInfo
	return resp, e.SendWsRequest("orderList.status", params, &resp)
}

// WsCancelOCOOrder cancel an active OCO order.
func (e *Exchange) WsCancelOCOOrder(symbol currency.Pair, orderListID, listClientOrderID, newClientOrderID string, recvWindow ...float64) (*OCOOrder, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if orderListID == "" && listClientOrderID == "" {
		return nil, fmt.Errorf("orderListID %w", order.ErrOrderIDNotSet)
	}
	params := spotSymbolParams(symbol, nil, "")
	if listClientOrderID != "" {
		params["listClientOrderId"] = listClientOrderID
	}
	if newClientOrderID != "" {
		params["newClientOrderId"] = newClientOrderID
	}
	if orderListID != "" {
		id, err := strconv.ParseUint(orderListID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidOrderListID, err)
		}
		params["orderListId"] = id
	}
	if len(recvWindow) > 0 && recvWindow[0] != 0 {
		params["recvWindow"] = recvWindow[0]
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp *OCOOrder
	return resp, e.SendWsRequest("orderList.cancel", params, &resp)
}

// WsCurrentOpenOCOOrders query execution status of all open OCOs.
func (e *Exchange) WsCurrentOpenOCOOrders(recvWindow float64) ([]*OCOOrder, error) {
	params := make(map[string]any)
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp []*OCOOrder
	return resp, e.SendWsRequest("openOrderLists.status", params, &resp)
}

// WsPlaceNewSOROrder places an order using smart order routing (SOR).
func (e *Exchange) WsPlaceNewSOROrder(arg *WsOSRPlaceOrderRequest) ([]*OSROrder, error) {
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
	if arg.Quantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	arg.Timestamp = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg.APIKey = apiKey
	arg.Signature = signature
	var resp []*OSROrder
	return resp, e.SendWsRequest("sor.order.place", arg, &resp)
}

// WsTestNewOrderUsingSOR test new order creation and signature/recvWindow using smart order routing (SOR).
// Creates and validates a new order but does not send it into the matching engine.
func (e *Exchange) WsTestNewOrderUsingSOR(arg *WsOSRPlaceOrderRequest, computeCommissionRates ...bool) (*TestOrderResponse, error) {
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
	if arg.Quantity <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	if len(computeCommissionRates) > 0 {
		params["computeCommissionRates"] = computeCommissionRates[0]
	}
	var response *TestOrderResponse
	return response, e.sendSignedWsRequest("sor.order.test", params, &response)
}

// ToMap creates a map out of struct instances
func (e *Exchange) ToMap(input any) (map[string]any, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	resp := make(map[string]any, len(fields))
	for name, value := range fields {
		if len(value) > 0 && value[0] == '"' {
			var decoded string
			if err := json.Unmarshal(value, &decoded); err != nil {
				return nil, err
			}
			resp[name] = decoded
			continue
		}
		// RawMessage preserves the number literal for both signing and encoding.
		resp[name] = value
	}
	return resp, nil
}

// ------------------------------------------- Account Requests --------------------------------

// GetWsAccountInfo query information about your account.
func (e *Exchange) GetWsAccountInfo(recvWindow float64, omitZeroBalances ...bool) (*Account, error) {
	params := map[string]any{}
	if len(omitZeroBalances) > 0 {
		params["omitZeroBalances"] = omitZeroBalances[0]
	}
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signatures, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signatures
	var resp *Account
	return resp, e.SendWsRequest("account.status", params, &resp)
}

// WsQueryAccountOrderRateLimits query your current order rate limit.
func (e *Exchange) WsQueryAccountOrderRateLimits(recvWindow float64) ([]*RateLimitItem, error) {
	params := map[string]any{}
	if recvWindow > 0 {
		params["recvWindow"] = recvWindow
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp []*RateLimitItem
	return resp, e.SendWsRequest("account.rateLimits.orders", params, &resp)
}

// WsQueryAccountOrderHistory query information about all your orders – active, cancelled, filled – filtered by time range.
// Status reports for orders are identical to order.status.
func (e *Exchange) WsQueryAccountOrderHistory(arg *AccountOrderRequest) ([]*TradeOrder, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if !arg.StartTime.IsZero() && !arg.EndTime.IsZero() {
		if err := common.StartEndTimeCheck(arg.StartTime, arg.EndTime); err != nil {
			return nil, err
		}
	}
	if !arg.StartTime.IsZero() {
		arg.StartTimestamp = arg.StartTime.UnixMilli()
	}
	if !arg.EndTime.IsZero() {
		arg.EndTimestamp = arg.EndTime.UnixMilli()
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	arg.Timestamp = time.Now().UnixMilli()
	apiKey, signature, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg.APIKey = apiKey
	arg.Signature = signature
	var resp []*TradeOrder
	return resp, e.SendWsRequest("allOrders", arg, &resp)
}

// WsQueryAccountOCOOrderHistory query information about all your OCOs, filtered by time range.
// Status reports for OCOs are identical to orderList.status.
func (e *Exchange) WsQueryAccountOCOOrderHistory(fromID, limit int64, recvWindow float64, startTime, endTime time.Time) ([]*OCOOrder, error) {
	params := make(map[string]any)
	if fromID != 0 {
		params["fromId"] = fromID
	}
	if limit != 0 {
		params["limit"] = limit
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	if !startTime.IsZero() {
		params["startTime"] = startTime.UnixMilli()
	}
	if !endTime.IsZero() {
		params["endTime"] = endTime.UnixMilli()
	}
	if recvWindow != 0 {
		params["recvWindow"] = recvWindow
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp []*OCOOrder
	return resp, e.SendWsRequest("allOrderLists", params, &resp)
}

// WsAccountTradeHistory query information about all your trades, filtered by time range.
func (e *Exchange) WsAccountTradeHistory(arg *AccountOrderRequest) ([]*TradeHistory, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if !arg.StartTime.IsZero() && !arg.EndTime.IsZero() {
		if err := common.StartEndTimeCheck(arg.StartTime, arg.EndTime); err != nil {
			return nil, err
		}
	}
	if !arg.StartTime.IsZero() {
		arg.StartTimestamp = arg.StartTime.UnixMilli()
	}
	if !arg.EndTime.IsZero() {
		arg.EndTimestamp = arg.EndTime.UnixMilli()
	}
	if arg.Symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	arg.Timestamp = time.Now().UnixMilli()
	apiKey, signatures, err := e.getSignature(arg)
	if err != nil {
		return nil, err
	}
	arg.APIKey = apiKey
	arg.Signature = signatures
	var resp []*TradeHistory
	return resp, e.SendWsRequest("myTrades", arg, &resp)
}

// WsAccountPreventedMatches displays the list of orders that were expired because of STP trigger.
//
// These are the combinations supported:
// symbol + preventedMatchId
// symbol + orderId
// symbol + orderId + fromPreventedMatchId (limit will default to 500)
// symbol + orderId + fromPreventedMatchId + limit
func (e *Exchange) WsAccountPreventedMatches(symbol currency.Pair, preventedMatchID, orderID, fromPreventedMatchID, limit int64, recvWindow float64) ([]*SelfTradePrevention, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if orderID == 0 && preventedMatchID == 0 {
		return nil, fmt.Errorf("%w, either orderID or preventedMatchID is required", order.ErrOrderIDNotSet)
	}
	params := make(map[string]any)
	params["symbol"] = symbol
	if preventedMatchID != 0 {
		params["preventedMatchId"] = preventedMatchID
	}
	if orderID != 0 {
		params["orderId"] = orderID
	}
	if fromPreventedMatchID != 0 {
		params["fromPreventedMatchId"] = fromPreventedMatchID
	}
	if limit > 0 {
		params["limit"] = limit
	}
	if recvWindow > 0 {
		params["recvWindow"] = recvWindow
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp []*SelfTradePrevention
	return resp, e.SendWsRequest("myPreventedMatches", params, &resp)
}

// WsAccountAllocation retrieves allocations resulting from SOR order placement.
func (e *Exchange) WsAccountAllocation(symbol currency.Pair, startTime, endTime time.Time, orderID, fromAllocationID int64, recvWindow float64, limit int64) ([]*SORReplacements, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := map[string]any{
		symbolParam: symbol.String(),
	}
	switch {
	case !startTime.IsZero() && !endTime.IsZero():
		if common.StartEndTimeCheck(startTime, endTime) == nil {
			params["startTime"] = startTime.UnixMilli()
			params["endTime"] = endTime.UnixMilli()
		}
	case !startTime.IsZero():
		params["startTime"] = startTime.UnixMilli()
	case !endTime.IsZero():
		params["endTime"] = endTime.UnixMilli()
	}
	if fromAllocationID != 0 {
		params["fromAllocationId"] = fromAllocationID
	}
	if limit > 0 {
		params["limit"] = limit
	}
	if orderID > 0 {
		params["orderId"] = orderID
	}
	if recvWindow > 0 {
		params["recvWindow"] = recvWindow
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp []*SORReplacements
	return resp, e.SendWsRequest("myAllocations", params, &resp)
}

// WsAccountCommissionRates get current account commission rates.
func (e *Exchange) WsAccountCommissionRates(symbol currency.Pair) (*CommissionRateInto, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := map[string]any{
		symbolParam: symbol.String(),
	}
	params["timestamp"] = time.Now().UnixMilli()
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp *CommissionRateInto
	return resp, e.SendWsRequest("account.commission", params, &resp)
}

// --------------------------------- User Data Stream requests ---------------------------------

// WsSessionLogon authenticates the websocket connection with the configured API key, so that
// subsequent signed requests may omit apiKey and signature. Binance only permits one
// authenticated key per connection; calling this again replaces it.
func (e *Exchange) WsSessionLogon(recvWindow ...float64) (*FuturesAuthenticationResponse, error) {
	creds, err := e.GetCredentials(context.Background())
	if err != nil {
		return nil, err
	}
	key, err := parsePrivateKey(creds.Secret)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errEd25519KeyRequired, err)
	}
	if _, ok := key.(ed25519.PrivateKey); !ok {
		return nil, errEd25519KeyRequired
	}

	params := map[string]any{"timestamp": time.Now().UnixMilli()}
	if len(recvWindow) > 0 && recvWindow[0] != 0 {
		params["recvWindow"] = recvWindow[0]
	}
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp *FuturesAuthenticationResponse
	return resp, e.SendWsRequest("session.logon", params, &resp)
}

// WsSubscribeUserDataStream subscribes to the user data stream on the current authenticated
// connection. This requires a prior WsSessionLogon using Ed25519 keys; for HMAC keys use
// WsSubscribeUserDataStreamWithSignature instead.
//
// This supersedes the listen key flow: Binance retired POST/PUT/DELETE /api/v3/userDataStream
// and the userDataStream.start/ping/stop methods on 2026-02-20.
func (e *Exchange) WsSubscribeUserDataStream() (*UserDataStreamSubscriptionResponse, error) {
	var resp *UserDataStreamSubscriptionResponse
	return resp, e.SendWsRequest("userDataStream.subscribe", nil, &resp)
}

// WsSubscribeUserDataStreamWithSignature subscribes to the user data stream by signing the
// request, which works on any connection whether or not it has been authenticated with
// WsSessionLogon, and works with HMAC as well as Ed25519 keys.
func (e *Exchange) WsSubscribeUserDataStreamWithSignature(recvWindow ...float64) (*UserDataStreamSubscriptionResponse, error) {
	params := map[string]any{"timestamp": time.Now().UnixMilli()}
	if len(recvWindow) > 0 && recvWindow[0] != 0 {
		params["recvWindow"] = recvWindow[0]
	}
	apiKey, signature, err := e.SignRequest(params)
	if err != nil {
		return nil, err
	}
	params["apiKey"] = apiKey
	params["signature"] = signature
	var resp *UserDataStreamSubscriptionResponse
	return resp, e.SendWsRequest("userDataStream.subscribe.signature", params, &resp)
}

// WsUnsubscribeUserDataStream stops listening to the user data stream on this connection.
// Note that session.logout only closes a subscription opened by WsSubscribeUserDataStream,
// not one opened by WsSubscribeUserDataStreamWithSignature.
func (e *Exchange) WsUnsubscribeUserDataStream(subscriptionID ...uint64) error {
	params := map[string]any{}
	if len(subscriptionID) > 0 {
		params["subscriptionId"] = subscriptionID[0]
	}
	return e.SendWsRequest("userDataStream.unsubscribe", params, &struct{}{})
}

// WsUserDataStreamSubscriptions returns all active user-data subscriptions on the connection.
func (e *Exchange) WsUserDataStreamSubscriptions() ([]*UserDataStreamSubscriptionResponse, error) {
	var response []*UserDataStreamSubscriptionResponse
	return response, e.SendWsRequest("session.subscriptions", nil, &response)
}

// signatureValue preserves JSON number and boolean literals without converting
// them through float64 or printing RawMessage's underlying byte slice.
func signatureValue(value any) string {
	if raw, ok := value.(json.RawMessage); ok {
		return string(raw)
	}
	return fmt.Sprint(value)
}

func (e *Exchange) sendSignedWsRequest(method string, params map[string]any, response any) error {
	params["timestamp"] = time.Now().UnixMilli()
	_, signature, err := e.SignRequest(params)
	if err != nil {
		return err
	}
	params["signature"] = signature
	return e.SendWsRequest(method, params, response)
}
