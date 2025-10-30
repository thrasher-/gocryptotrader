package hyperliquid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
)

type pendingWSRequest struct {
	method       string
	subscription *subscription.Subscription
}

// WsConnect creates a new websocket connection.
func (e *Exchange) WsConnect() error {
	if !e.Websocket.IsEnabled() || !e.IsEnabled() {
		return websocket.ErrWebsocketNotEnabled
	}
	timeout := exchange.DefaultHTTPTimeout
	if e.Config != nil && e.Config.HTTPTimeout > 0 {
		timeout = e.Config.HTTPTimeout
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	dialer := gws.Dialer{
		HandshakeTimeout: e.Config.HTTPTimeout,
		Proxy:            http.ProxyFromEnvironment,
	}

	if err := e.Websocket.Conn.Dial(dialCtx, &dialer, http.Header{}); err != nil {
		return fmt.Errorf("%v - Unable to connect to Websocket. Error: %s", e.Name, err)
	}

	e.Websocket.Wg.Add(1)
	go e.wsReadData(context.Background())
	return nil
}

func (e *Exchange) enqueuePendingWS(method string, payload *wsSubscriptionPayload, sub *subscription.Subscription) {
	if sub == nil {
		return
	}
	method = strings.ToLower(method)
	e.wsPendingMu.Lock()
	if e.wsPending == nil {
		e.wsPending = make(map[string][]pendingWSRequest)
	}
	key := payloadKeyFromPayload(payload)
	e.wsPending[key] = append(e.wsPending[key], pendingWSRequest{
		method:       method,
		subscription: sub,
	})
	e.wsPendingMu.Unlock()
}

func (e *Exchange) dequeuePendingWS(method string, payload *wsSubscriptionPayload) *subscription.Subscription {
	method = strings.ToLower(method)
	key := payloadKeyFromPayload(payload)
	e.wsPendingMu.Lock()
	defer e.wsPendingMu.Unlock()
	if e.wsPending == nil {
		return nil
	}
	items := e.wsPending[key]
	for i, item := range items {
		if item.method == method {
			sub := item.subscription
			e.wsPending[key] = append(items[:i], items[i+1:]...)
			if len(e.wsPending[key]) == 0 {
				delete(e.wsPending, key)
			}
			return sub
		}
	}
	return nil
}

// Subscribe sends websocket messages to receive data for a list of channels.
func (e *Exchange) Subscribe(subs subscription.List) error {
	ctx := context.Background()
	var errs error
	for _, sub := range subs {
		channel, identifier, err := parseQualifiedChannel(sub.QualifiedChannel)
		if err != nil {
			errs = common.AppendError(errs, err)
			continue
		}
		payload, err := e.buildSubscriptionPayload(sub, channel, identifier)
		if err != nil {
			errs = common.AppendError(errs, err)
			continue
		}
		req := wsSubscriptionRequest{
			Method:       "subscribe",
			Subscription: payload,
		}
		if err := e.Websocket.Conn.SendJSONMessage(ctx, request.Unset, req); err != nil {
			errs = common.AppendError(errs, err)
			continue
		}
		e.enqueuePendingWS("subscribe", &payload, sub)
	}
	return errs
}

// Unsubscribe sends websocket messages to stop receiving data for a list of channels.
func (e *Exchange) Unsubscribe(subs subscription.List) error {
	ctx := context.Background()
	var errs error
	for _, sub := range subs {
		channel, identifier, err := parseQualifiedChannel(sub.QualifiedChannel)
		if err != nil {
			errs = common.AppendError(errs, err)
			continue
		}
		payload, err := e.buildSubscriptionPayload(sub, channel, identifier)
		if err != nil {
			errs = common.AppendError(errs, err)
			continue
		}
		req := wsSubscriptionRequest{
			Method:       "unsubscribe",
			Subscription: payload,
		}
		if err := e.Websocket.Conn.SendJSONMessage(ctx, request.Unset, req); err != nil {
			errs = common.AppendError(errs, err)
			continue
		}
		e.enqueuePendingWS("unsubscribe", &payload, sub)
	}
	return errs
}

// wsReadData receives and passes on websocket messages for processing.
func (e *Exchange) wsReadData(ctx context.Context) {
	defer e.Websocket.Wg.Done()
	for {
		resp := e.Websocket.Conn.ReadMessage()
		if resp.Raw == nil {
			return
		}
		if err := e.wsHandleData(ctx, resp.Raw); err != nil {
			e.Websocket.DataHandler <- err
		}
	}
}

// wsHandleData processes a websocket incoming data.
func (e *Exchange) wsHandleData(ctx context.Context, respData []byte) error {
	if len(respData) == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	trimmed := bytes.TrimSpace(respData)
	if len(trimmed) == 0 {
		return nil
	}
	if bytes.Equal(trimmed, []byte("Websocket connection established.")) {
		return nil
	}

	var payload wsMessage
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return fmt.Errorf("hyperliquid: decode websocket message: %w", err)
	}

	switch payload.Channel {
	case websocketChannelSubscriptionResponse:
		if len(payload.Data) == 0 {
			return nil
		}
		var ack wsSubscriptionAck
		if err := json.Unmarshal(payload.Data, &ack); err != nil {
			return fmt.Errorf("hyperliquid: decode subscription response: %w", err)
		}
		return e.handleSubscriptionAck(ack)
	case "pong":
		return nil
	case websocketChannelError:
		var message string
		if len(payload.Data) != 0 {
			if err := json.Unmarshal(payload.Data, &message); err != nil {
				message = string(bytes.TrimSpace(payload.Data))
			}
		}
		if message == "" {
			message = "unknown websocket error"
		}
		return fmt.Errorf("hyperliquid websocket error: %s", message)
	case websocketChannelAllMids:
		var mids wsAllMidsData
		if err := json.Unmarshal(payload.Data, &mids); err != nil {
			return fmt.Errorf("hyperliquid: decode all mids payload: %w", err)
		}
		return e.handleAllMids(mids)
	case websocketChannelL2Book:
		var book wsL2BookData
		if err := json.Unmarshal(payload.Data, &book); err != nil {
			return fmt.Errorf("hyperliquid: decode l2 book payload: %w", err)
		}
		return e.handleL2Book(book)
	case websocketChannelBbo:
		data := new(wsBBOData)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode bbo payload: %w", err)
		}
		return e.handleBBO(data)
	case websocketChannelTrades:
		var trades wsTradesData
		if err := json.Unmarshal(payload.Data, &trades); err != nil {
			return fmt.Errorf("hyperliquid: decode trades payload: %w", err)
		}
		return e.handleTrades(trades)
	case websocketChannelCandle:
		data := new(wsCandleData)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode candle payload: %w", err)
		}
		return e.handleCandle(data)
	case websocketChannelActiveAssetData:
		data := new(wsActiveAssetDataPayload)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode active asset data payload: %w", err)
		}
		return e.handleActiveAssetData(data)
	case websocketChannelActiveAssetCtx:
		data := new(wsActiveAssetContextPayload)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode active asset context payload: %w", err)
		}
		return e.handleActiveAssetContext(data)
	case websocketChannelActiveSpotAssetCtx:
		data := new(wsActiveSpotAssetContextPayload)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode active spot asset context payload: %w", err)
		}
		return e.handleActiveSpotAssetContext(data)
	case websocketChannelUserEvents:
		data := new(wsUserEventsData)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode user events payload: %w", err)
		}
		return e.handleUserEvents(data)
	case websocketChannelUserFills:
		data := new(wsUserFillsPayload)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode user fills payload: %w", err)
		}
		return e.handleUserFills(data)
	case websocketChannelOrderUpdates:
		data := new(wsOrderUpdatesPayload)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode order updates payload: %w", err)
		}
		return e.handleOrderUpdates(data)
	case websocketChannelUserFundings:
		data := new(wsUserFundingPayload)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode user funding payload: %w", err)
		}
		return e.handleUserFunding(data)
	case websocketChannelUserLedgerUpdates:
		data := new(wsUserNonFundingLedgerPayload)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode user ledger payload: %w", err)
		}
		return e.handleUserLedgerUpdates(data)
	case websocketChannelWebData2:
		data := new(wsWebData2Payload)
		if err := json.Unmarshal(payload.Data, data); err != nil {
			return fmt.Errorf("hyperliquid: decode webData2 payload: %w", err)
		}
		return e.handleWebData2(data)
	default:
		e.Websocket.DataHandler <- websocket.UnhandledMessageWarning{
			Message: e.Name + websocket.UnhandledMessage + string(trimmed),
		}
		return nil
	}
}

func (e *Exchange) buildSubscriptionPayload(sub *subscription.Subscription, channel, identifier string) (wsSubscriptionPayload, error) {
	if sub == nil {
		return wsSubscriptionPayload{}, errSubscriptionPayloadRequiresMetadata
	}
	switch channel {
	case subscription.TickerChannel:
		return wsSubscriptionPayload{Type: websocketChannelAllMids}, nil
	case subscription.OrderbookChannel:
		return wsSubscriptionPayload{Type: websocketChannelL2Book, Coin: identifier}, nil
	case subscription.AllTradesChannel:
		return wsSubscriptionPayload{Type: websocketChannelTrades, Coin: identifier}, nil
	case subscription.CandlesChannel:
		if sub.Interval <= 0 {
			return wsSubscriptionPayload{}, fmt.Errorf("hyperliquid: candle subscription for %s must set interval", identifier)
		}
		interval, err := candleIntervalString(sub.Interval)
		if err != nil {
			return wsSubscriptionPayload{}, err
		}
		return wsSubscriptionPayload{Type: websocketChannelCandle, Coin: identifier, Interval: interval}, nil
	case websocketChannelBbo:
		return wsSubscriptionPayload{Type: websocketChannelBbo, Coin: identifier}, nil
	case hyperliquidActiveAssetDataChannel:
		if identifier == "" {
			return wsSubscriptionPayload{}, errActiveAssetDataSubMissingMarket
		}
		if sub.Params == nil {
			return wsSubscriptionPayload{}, errActiveAssetDataSubMissingParams
		}
		userParam, ok := sub.Params["user"].(string)
		if !ok || userParam == "" {
			return wsSubscriptionPayload{}, errActiveAssetDataSubMissingUserParam
		}
		return wsSubscriptionPayload{Type: websocketChannelActiveAssetData, Coin: identifier, User: strings.ToLower(userParam)}, nil
	case websocketChannelActiveAssetCtx, websocketChannelActiveSpotAssetCtx:
		if identifier == "" {
			return wsSubscriptionPayload{}, fmt.Errorf("hyperliquid: %s subscription missing market identifier", channel)
		}
		return wsSubscriptionPayload{Type: channel, Coin: identifier}, nil
	case websocketChannelUserEvents,
		websocketChannelUserFills,
		websocketChannelOrderUpdates,
		websocketChannelUserFundings,
		websocketChannelUserLedgerUpdates,
		websocketChannelWebData2:
		if sub.Params == nil {
			return wsSubscriptionPayload{}, fmt.Errorf("hyperliquid: %s subscription missing parameters", channel)
		}
		userParam, ok := sub.Params["user"].(string)
		if !ok || userParam == "" {
			return wsSubscriptionPayload{}, fmt.Errorf("hyperliquid: %s subscription missing user param", channel)
		}
		return wsSubscriptionPayload{Type: channel, User: strings.ToLower(userParam)}, nil
	default:
		return wsSubscriptionPayload{}, fmt.Errorf("hyperliquid: unsupported subscription channel %s", channel)
	}
}

func (e *Exchange) handleSubscriptionAck(ack wsSubscriptionAck) error {
	payload, err := ackSubscriptionMapToPayload(ack.Subscription)
	if err != nil {
		return err
	}
	method := strings.ToLower(ack.Method)
	if method == "" {
		method = "subscribe"
	}
	payloadRef := &payload
	pending := e.dequeuePendingWS(method, payloadRef)
	if ack.Error != nil && *ack.Error != "" {
		return fmt.Errorf("hyperliquid: %s subscription %s failed: %s", method, payloadDescription(payloadRef), *ack.Error)
	}
	if pending == nil {
		switch method {
		case "subscribe":
			sub, err := subscriptionFromPayload(payloadRef)
			if err != nil {
				return err
			}
			pending = sub
		case "unsubscribe":
			pending = e.findSubscriptionByPayload(payloadRef)
		default:
			return fmt.Errorf("hyperliquid: unsupported subscription ack method %s", ack.Method)
		}
	}
	switch method {
	case "subscribe":
		if pending == nil {
			return nil
		}
		if err := e.Websocket.AddSuccessfulSubscriptions(nil, pending); err != nil && !errors.Is(err, subscription.ErrDuplicate) {
			return fmt.Errorf("hyperliquid: register subscription %s: %w", payloadDescription(payloadRef), err)
		}
	case "unsubscribe":
		if pending == nil {
			return nil
		}
		if err := e.Websocket.RemoveSubscriptions(nil, pending); err != nil && !errors.Is(err, subscription.ErrNotFound) {
			return fmt.Errorf("hyperliquid: unregister subscription %s: %w", payloadDescription(payloadRef), err)
		}
	default:
		return fmt.Errorf("hyperliquid: unsupported subscription ack method %s", ack.Method)
	}
	return nil
}

func (e *Exchange) handleAllMids(payload wsAllMidsData) error {
	if len(payload.Mids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for market, price := range payload.Mids {
		assetType, pair, err := parseMarketString(market)
		if err != nil {
			continue
		}
		priceFloat := price.Float64()
		if priceFloat <= 0 {
			continue
		}
		e.Websocket.DataHandler <- &ticker.Price{
			ExchangeName: e.Name,
			Pair:         pair,
			AssetType:    assetType,
			Last:         priceFloat,
			Close:        priceFloat,
			LastUpdated:  now,
		}
	}
	return nil
}

func (e *Exchange) handleL2Book(payload wsL2BookData) error {
	if len(payload.Levels) < 2 {
		return errOrderbookSidesMissing
	}
	assetType, pair, err := parseMarketString(payload.Coin)
	if err != nil {
		return err
	}

	bids := make(orderbook.Levels, 0, len(payload.Levels[0]))
	for _, lvl := range payload.Levels[0] {
		price := lvl.Price.Float64()
		if price == 0 {
			continue
		}
		size := lvl.Size.Float64()
		if size == 0 {
			continue
		}
		bids = append(bids, orderbook.Level{Price: price, Amount: size})
	}

	asks := make(orderbook.Levels, 0, len(payload.Levels[1]))
	for _, lvl := range payload.Levels[1] {
		price := lvl.Price.Float64()
		if price == 0 {
			continue
		}
		size := lvl.Size.Float64()
		if size == 0 {
			continue
		}
		asks = append(asks, orderbook.Level{Price: price, Amount: size})
	}

	if len(bids) == 0 && len(asks) == 0 {
		return nil
	}

	book := &orderbook.Book{
		Exchange:          e.Name,
		Pair:              pair,
		Asset:             assetType,
		Bids:              bids,
		Asks:              asks,
		LastUpdated:       payload.Time.Time().UTC(),
		ValidateOrderbook: e.ValidateOrderbook,
	}
	return e.Websocket.Orderbook.LoadSnapshot(book)
}

func (e *Exchange) handleBBO(payload *wsBBOData) error {
	if payload == nil {
		return errBboPayloadMissing
	}
	assetType, pair, err := parseMarketString(payload.Coin)
	if err != nil {
		return err
	}
	var (
		bidPrice float64
		bidSize  float64
		askPrice float64
		askSize  float64
	)
	if level := payload.BBO[0]; level != nil {
		bidPrice = level.Price.Float64()
		bidSize = level.Size.Float64()
	}
	if level := payload.BBO[1]; level != nil {
		askPrice = level.Price.Float64()
		askSize = level.Size.Float64()
	}
	if bidPrice == 0 && askPrice == 0 {
		return nil
	}
	price := &ticker.Price{
		ExchangeName: e.Name,
		Pair:         pair,
		AssetType:    assetType,
		Bid:          bidPrice,
		BidSize:      bidSize,
		Ask:          askPrice,
		AskSize:      askSize,
		LastUpdated:  payload.Time.Time().UTC(),
	}
	return ticker.ProcessTicker(price)
}

func (e *Exchange) handleTrades(payload wsTradesData) error {
	if len(payload) == 0 {
		return nil
	}
	saveTrades := e.IsSaveTradeDataEnabled()
	tradeFeed := e.IsTradeFeedEnabled()
	if !saveTrades && !tradeFeed {
		return nil
	}
	trades := make([]trade.Data, 0, len(payload))
	for i := range payload {
		entry := payload[i]
		assetType, pair, err := parseMarketString(entry.Coin)
		if err != nil {
			continue
		}
		price := entry.Price.Float64()
		amount := entry.Size.Float64()
		trades = append(trades, trade.Data{
			Exchange:     e.Name,
			CurrencyPair: pair,
			AssetType:    assetType,
			Price:        price,
			Amount:       amount,
			Side:         convertTradeSideToOrder(entry.Side),
			Timestamp:    entry.Time.Time().UTC(),
			TID:          entry.TID,
		})
	}
	if len(trades) == 0 {
		return nil
	}
	if saveTrades {
		if err := trade.AddTradesToBuffer(trades...); err != nil {
			return err
		}
	}
	if tradeFeed {
		for i := range trades {
			e.Websocket.DataHandler <- trades[i]
		}
	}
	return nil
}

func (e *Exchange) handleCandle(payload *wsCandleData) error {
	if payload == nil {
		return errCandlePayloadMissing
	}
	assetType, pair, err := parseMarketString(payload.Symbol)
	if err != nil {
		return err
	}
	interval, err := candleIntervalFromString(payload.Interval)
	if err != nil {
		return err
	}
	klineItem := websocket.KlineData{
		Timestamp:  payload.CloseTime.Time(),
		Pair:       pair,
		AssetType:  assetType,
		Exchange:   e.Name,
		StartTime:  payload.OpenTime.Time(),
		CloseTime:  payload.CloseTime.Time(),
		Interval:   interval.Short(),
		OpenPrice:  payload.Open.Float64(),
		ClosePrice: payload.Close.Float64(),
		HighPrice:  payload.High.Float64(),
		LowPrice:   payload.Low.Float64(),
		Volume:     payload.Volume.Float64(),
	}
	e.Websocket.DataHandler <- klineItem
	return nil
}
