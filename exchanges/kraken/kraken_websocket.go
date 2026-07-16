package kraken

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/buger/jsonparser"
	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// List of all websocket channels to subscribe to
const (
	krakenWSURL              = "wss://ws.kraken.com/v2"
	krakenAuthWSURL          = "wss://ws-auth.kraken.com/v2"
	krakenWSSandboxURL       = "wss://sandbox.kraken.com"
	krakenWSSupportedVersion = "v2"

	// Websocket Channels
	krakenWsHeartbeat            = "heartbeat"
	krakenWsSystemStatus         = "systemStatus"
	krakenWsSubscribe            = "subscribe"
	krakenWsUnsubscribe          = "unsubscribe"
	krakenWsSubscribed           = "subscribed"
	krakenWsUnsubscribed         = "unsubscribed"
	krakenWsSubscriptionStatus   = "subscriptionStatus"
	krakenWsTicker               = "ticker"
	krakenWsOHLC                 = "ohlc"
	krakenWsTrade                = "trade"
	krakenWsSpread               = "spread"
	krakenWsOrderbook            = "book"
	krakenWsOwnTrades            = "ownTrades"
	krakenWsOpenOrders           = "openOrders"
	krakenWsAddOrder             = "addOrder"
	krakenWsCancelOrder          = "cancelOrder"
	krakenWsCancelAll            = "cancelAll"
	krakenWsAddOrderStatus       = "addOrderStatus"
	krakenWsCancelOrderStatus    = "cancelOrderStatus"
	krakenWsCancelAllOrderStatus = "cancelAllStatus"
	krakenWsPong                 = "pong"
	krakenWsPingDelay            = time.Second * 27

	krakenWsV2Status      = "status"
	krakenWsV2Executions  = "executions"
	krakenWsV2AddOrder    = "add_order"
	krakenWsV2CancelOrder = "cancel_order"
	krakenWsV2CancelAll   = "cancel_all"
)

var channelNames = map[string]string{
	subscription.TickerChannel:    krakenWsTicker,
	subscription.OrderbookChannel: krakenWsOrderbook,
	subscription.CandlesChannel:   krakenWsOHLC,
	subscription.AllTradesChannel: krakenWsTrade,
	subscription.MyAccountChannel: krakenWsV2Executions,
}
var reverseChannelNames = map[string]string{}

func init() {
	for k, v := range channelNames {
		reverseChannelNames[v] = k
	}
}

var (
	errCancellingOrder        = errors.New("error cancelling order")
	errSubPairMissing         = errors.New("pair missing from subscription response")
	errInvalidChecksum        = errors.New("invalid checksum")
	errExpectedOneSubResponse = errors.New("expected 1 subscription response")
	errV2TrackingValueNotSet  = errors.New("tracking value not set for a Kraken trailing-stop order")
	errV2TriggerPriceNotSet   = errors.New("trigger price not set for a Kraken triggered order")
)

func krakenV2CurrencyToExchange(c currency.Code) currency.Code {
	switch {
	case c.Equal(currency.XBT), c.Equal(currency.XXBT):
		return currency.BTC
	case c.Equal(currency.XDG), c.Equal(currency.XXDG):
		return currency.DOGE
	default:
		return c
	}
}

func krakenV2CurrencyFromExchange(c currency.Code) currency.Code {
	switch {
	case c.Equal(currency.BTC):
		return currency.XBT
	case c.Equal(currency.DOGE):
		return currency.XDG
	default:
		return c
	}
}

func krakenV2PairToExchange(p currency.Pair) currency.Pair {
	p.Base = krakenV2CurrencyToExchange(p.Base)
	p.Quote = krakenV2CurrencyToExchange(p.Quote)
	return p
}

func krakenV2PairFromExchange(symbol string) (currency.Pair, error) {
	p, err := currency.NewPairDelimiter(symbol, "/")
	if err != nil {
		return currency.EMPTYPAIR, err
	}
	p.Base = krakenV2CurrencyFromExchange(p.Base)
	p.Quote = krakenV2CurrencyFromExchange(p.Quote)
	return p, nil
}

func krakenV2PairsToExchange(pairs currency.Pairs) currency.Pairs {
	translated := make(currency.Pairs, len(pairs))
	for i := range pairs {
		translated[i] = krakenV2PairToExchange(pairs[i])
	}
	return translated
}

var defaultSubscriptions = subscription.List{
	{Enabled: true, Asset: asset.Spot, Channel: subscription.TickerChannel},
	{Enabled: true, Asset: asset.Spot, Channel: subscription.AllTradesChannel},
	{Enabled: true, Asset: asset.Spot, Channel: subscription.CandlesChannel, Interval: kline.OneMin},
	{Enabled: true, Asset: asset.Spot, Channel: subscription.OrderbookChannel, Levels: 1000},
	{Enabled: true, Channel: subscription.MyAccountChannel, Authenticated: true},
}

// WsConnect initiates a websocket connection
func (e *Exchange) WsConnect() error {
	ctx := context.TODO()
	if !e.Websocket.IsEnabled() || !e.IsEnabled() {
		return websocket.ErrWebsocketNotEnabled
	}

	var dialer gws.Dialer
	err := e.Websocket.Conn.Dial(ctx, &dialer, http.Header{}, nil)
	if err != nil {
		return err
	}

	e.Websocket.Wg.Add(1)
	go e.wsReadData(ctx, e.Websocket.Conn)

	if e.IsWebsocketAuthenticationSupported() {
		if authToken, err := e.GetWebsocketToken(ctx); err != nil {
			e.Websocket.SetCanUseAuthenticatedEndpoints(false)
			log.Errorf(log.ExchangeSys, "%s - authentication failed: %v\n", e.Name, err)
		} else {
			if err := e.Websocket.AuthConn.Dial(ctx, &dialer, http.Header{}, nil); err != nil {
				e.Websocket.SetCanUseAuthenticatedEndpoints(false)
				log.Errorf(log.ExchangeSys, "%s - failed to connect to authenticated endpoint: %v\n", e.Name, err)
			} else {
				e.setWebsocketAuthToken(authToken)
				e.Websocket.SetCanUseAuthenticatedEndpoints(true)
				e.Websocket.Wg.Add(1)
				go e.wsReadData(ctx, e.Websocket.AuthConn)
				e.startWsPingHandler(e.Websocket.AuthConn)
			}
		}
	}

	e.startWsPingHandler(e.Websocket.Conn)

	return nil
}

// wsReadData funnels both auth and public ws data into one manageable place
func (e *Exchange) wsReadData(ctx context.Context, ws websocket.Connection) {
	defer e.Websocket.Wg.Done()
	for {
		resp := ws.ReadMessage()
		if resp.Raw == nil {
			return
		}
		if err := e.wsHandleData(ctx, resp.Raw); err != nil {
			if errSend := e.Websocket.DataHandler.Send(ctx, err); errSend != nil {
				log.Errorf(log.WebsocketMgr, "%s %s: %s %s", e.Name, ws.GetURL(), errSend, err)
			}
		}
	}
}

func (e *Exchange) wsHandleData(ctx context.Context, respRaw []byte) error {
	if _, err := jsonparser.GetUnsafeString(respRaw, "channel"); err == nil {
		return e.wsHandleV2Data(ctx, respRaw)
	}
	if _, err := jsonparser.GetUnsafeString(respRaw, "method"); err == nil {
		return e.wsHandleV2Response(ctx, respRaw)
	}

	// Retain decoding of recorded v1 fixtures while all runtime endpoints use v2.
	if strings.HasPrefix(string(respRaw), "[") {
		var msg []json.RawMessage
		if err := json.Unmarshal(respRaw, &msg); err != nil {
			return err
		}
		if len(msg) < 3 {
			return fmt.Errorf("data array too short: %s", respRaw)
		}

		// For all types of channel second to last field is the channel Name
		var chanName string
		if err := json.Unmarshal(msg[len(msg)-2], &chanName); err != nil {
			return fmt.Errorf("error unmarshalling channel name: %w", err)
		}

		pair := currency.EMPTYPAIR
		var maybePair string
		if err := json.Unmarshal(msg[len(msg)-1], &maybePair); err == nil {
			p, err := currency.NewPairFromString(maybePair)
			if err != nil {
				return err
			}
			pair = p
		}

		return e.wsReadDataResponse(ctx, chanName, pair, msg)
	}

	event, err := jsonparser.GetString(respRaw, "event")
	if err != nil {
		return fmt.Errorf("%w parsing: %s", err, respRaw)
	}

	if event == krakenWsSubscriptionStatus { // Must happen before IncomingWithData to avoid race
		e.wsProcessSubStatus(respRaw)
	}

	reqID, err := jsonparser.GetInt(respRaw, "reqid")
	if err == nil && reqID != 0 && e.Websocket.Match.IncomingWithData(reqID, respRaw) {
		return nil
	}

	if event == "" {
		return nil
	}

	switch event {
	case krakenWsPong, krakenWsHeartbeat:
		return nil
	case krakenWsCancelOrderStatus, krakenWsCancelAllOrderStatus, krakenWsAddOrderStatus, krakenWsSubscriptionStatus:
		// All of these should have found a listener already
		return fmt.Errorf("%w: %s %v", websocket.ErrSignatureNotMatched, event, reqID)
	case krakenWsSystemStatus:
		return e.wsProcessSystemStatus(respRaw)
	default:
		return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{
			Message: fmt.Sprintf("%s: %s", websocket.UnhandledMessage, respRaw),
		})
	}
}

func (e *Exchange) wsHandleV2Response(ctx context.Context, respRaw []byte) error {
	var resp websocketV2Response
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return fmt.Errorf("error unmarshalling WebSocket v2 response: %w", err)
	}

	if resp.Method == krakenWsSubscribe || resp.Method == krakenWsUnsubscribe {
		e.wsProcessSubStatus(respRaw)
	}

	if resp.RequestID != 0 && e.Websocket.Match.IncomingWithData(resp.RequestID, respRaw) {
		return nil
	}
	if resp.Method == krakenWsPong {
		return nil
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	if resp.Method == krakenWsSubscribe || resp.Method == krakenWsUnsubscribe ||
		resp.Method == krakenWsV2AddOrder || resp.Method == krakenWsV2CancelOrder || resp.Method == krakenWsV2CancelAll {
		return fmt.Errorf("%w: %s %v", websocket.ErrSignatureNotMatched, resp.Method, resp.RequestID)
	}
	return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{
		Message: fmt.Sprintf("%s: %s", websocket.UnhandledMessage, respRaw),
	})
}

func (e *Exchange) wsHandleV2Data(ctx context.Context, respRaw []byte) error {
	var msg websocketV2Message
	if err := json.Unmarshal(respRaw, &msg); err != nil {
		return fmt.Errorf("error unmarshalling WebSocket v2 message: %w", err)
	}

	switch msg.Channel {
	case krakenWsHeartbeat:
		return nil
	case krakenWsV2Status:
		return e.wsProcessV2Status(msg.Data)
	case krakenWsTicker:
		return e.wsProcessV2Tickers(ctx, msg.Data)
	case krakenWsTrade:
		return e.wsProcessV2Trades(ctx, msg.Data)
	case krakenWsOHLC:
		return e.wsProcessV2Candles(ctx, msg.Data)
	case krakenWsOrderbook:
		return e.wsProcessV2Orderbooks(ctx, msg.Type, msg.Data)
	case krakenWsV2Executions:
		return e.wsProcessV2Executions(ctx, msg.Data)
	default:
		return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{
			Message: fmt.Sprintf("%s: %s", websocket.UnhandledMessage, respRaw),
		})
	}
}

func (e *Exchange) wsProcessV2Status(data []json.RawMessage) error {
	if len(data) != 1 {
		return fmt.Errorf("expected one status item, received %d", len(data))
	}
	var status websocketV2Status
	if err := json.Unmarshal(data[0], &status); err != nil {
		return fmt.Errorf("error unmarshalling status data: %w", err)
	}
	if status.System != "online" {
		return fmt.Errorf("system status not online: %s", status.System)
	}
	if status.APIVersion != krakenWSSupportedVersion {
		return fmt.Errorf("unsupported WebSocket API version: %s", status.APIVersion)
	}
	return nil
}

func (e *Exchange) wsProcessV2Tickers(ctx context.Context, data []json.RawMessage) error {
	for i := range data {
		var item websocketV2Ticker
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling ticker data: %w", err)
		}
		pair, err := krakenV2PairFromExchange(item.Symbol)
		if err != nil {
			return fmt.Errorf("error parsing ticker symbol %q: %w", item.Symbol, err)
		}
		if err := e.Websocket.DataHandler.Send(ctx, &ticker.Price{
			ExchangeName: e.Name,
			Ask:          item.Ask,
			Bid:          item.Bid,
			Close:        item.Last,
			Volume:       item.Volume,
			Low:          item.Low,
			High:         item.High,
			Open:         item.Last - item.Change,
			AssetType:    asset.Spot,
			Pair:         pair,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Exchange) wsProcessV2Trades(ctx context.Context, data []json.RawMessage) error {
	saveTradeData := e.IsSaveTradeDataEnabled()
	tradeFeed := e.IsTradeFeedEnabled()
	if !saveTradeData && !tradeFeed {
		return nil
	}

	trades := make([]trade.Data, 0, len(data))
	for i := range data {
		var item websocketV2Trade
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling trade data: %w", err)
		}
		pair, err := krakenV2PairFromExchange(item.Symbol)
		if err != nil {
			return fmt.Errorf("error parsing trade symbol %q: %w", item.Symbol, err)
		}
		side, err := order.StringToOrderSide(item.Side)
		if err != nil {
			return err
		}
		trades = append(trades, trade.Data{
			AssetType:    asset.Spot,
			CurrencyPair: pair,
			Exchange:     e.Name,
			Price:        item.Price,
			Amount:       item.Quantity,
			Timestamp:    item.Timestamp.UTC(),
			Side:         side,
			TID:          strconv.FormatUint(item.TradeID, 10),
		})
	}
	if tradeFeed {
		for i := range trades {
			if err := e.Websocket.DataHandler.Send(ctx, trades[i]); err != nil {
				return err
			}
		}
	}
	if saveTradeData {
		return trade.AddTradesToBuffer(trades...)
	}
	return nil
}

func (e *Exchange) wsProcessV2Candles(ctx context.Context, data []json.RawMessage) error {
	for i := range data {
		var item websocketV2Candle
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling candle data: %w", err)
		}
		pair, err := krakenV2PairFromExchange(item.Symbol)
		if err != nil {
			return fmt.Errorf("error parsing candle symbol %q: %w", item.Symbol, err)
		}
		if err := e.Websocket.DataHandler.Send(ctx, kline.Item{
			Asset:    asset.Spot,
			Pair:     pair,
			Exchange: e.Name,
			Interval: kline.Interval(time.Minute * time.Duration(item.Interval)),
			Candles: []kline.Candle{{
				Time:        item.IntervalBegin,
				Open:        item.Open,
				High:        item.High,
				Low:         item.Low,
				Close:       item.Close,
				Volume:      item.Volume,
				QuoteVolume: item.Volume * item.VWAP,
			}},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Exchange) wsProcessV2Orderbooks(ctx context.Context, messageType string, data []json.RawMessage) error {
	for i := range data {
		var item websocketV2Book
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling orderbook data: %w", err)
		}
		pair, err := krakenV2PairFromExchange(item.Symbol)
		if err != nil {
			return fmt.Errorf("error parsing orderbook symbol %q: %w", item.Symbol, err)
		}
		sub := e.wsSubscriptionForPair(subscription.OrderbookChannel, pair)
		if sub == nil {
			return fmt.Errorf("%w: %s %s %s", subscription.ErrNotFound, asset.Spot, subscription.OrderbookChannel, pair)
		}
		if sub.State() == subscription.UnsubscribingState {
			continue
		}

		switch messageType {
		case "snapshot":
			err = e.wsProcessV2OrderbookSnapshot(pair, sub.Levels, &item)
		case "update":
			err = e.wsProcessV2OrderbookUpdate(pair, &item)
		default:
			return fmt.Errorf("unsupported orderbook message type %q", messageType)
		}
		if errors.Is(err, errInvalidChecksum) {
			log.Debugf(log.Global, "%s resubscribing to invalid %s orderbook", e.Name, pair)
			go func() {
				if resubErr := e.Websocket.ResubscribeToChannel(ctx, e.Websocket.Conn, sub); resubErr != nil && !errors.Is(resubErr, subscription.ErrInStateAlready) {
					log.Errorf(log.ExchangeSys, "%s resubscription failure for %v: %v", e.Name, pair, resubErr)
				}
			}()
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Exchange) wsSubscriptionForPair(channel string, pair currency.Pair) *subscription.Subscription {
	for _, sub := range e.Websocket.GetSubscriptions() {
		if sub.Channel == channel && sub.Asset == asset.Spot && sub.Pairs.Contains(pair, true) {
			return sub
		}
	}
	return nil
}

func (e *Exchange) wsProcessV2OrderbookSnapshot(pair currency.Pair, levels int, snapshot *websocketV2Book) error {
	updatedAt := snapshot.Timestamp
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	book := orderbook.Book{
		Pair:                   pair,
		Asset:                  asset.Spot,
		Exchange:               e.Name,
		ValidateOrderbook:      e.ValidateOrderbook,
		Bids:                   make(orderbook.Levels, len(snapshot.Bids)),
		Asks:                   make(orderbook.Levels, len(snapshot.Asks)),
		MaxDepth:               levels,
		LastUpdated:            updatedAt,
		ChecksumStringRequired: true,
	}
	for i := range snapshot.Asks {
		book.Asks[i] = orderbook.Level{
			Price:     snapshot.Asks[i].Price,
			Amount:    snapshot.Asks[i].Quantity,
			StrPrice:  snapshot.Asks[i].PriceString,
			StrAmount: snapshot.Asks[i].QtyString,
		}
	}
	for i := range snapshot.Bids {
		book.Bids[i] = orderbook.Level{
			Price:     snapshot.Bids[i].Price,
			Amount:    snapshot.Bids[i].Quantity,
			StrPrice:  snapshot.Bids[i].PriceString,
			StrAmount: snapshot.Bids[i].QtyString,
		}
	}
	if err := e.Websocket.Orderbook.LoadSnapshot(&book); err != nil {
		return err
	}
	stored, err := e.Websocket.Orderbook.GetOrderbook(pair, asset.Spot)
	if err != nil {
		return fmt.Errorf("cannot calculate websocket checksum: book not found for %s %s %w", pair, asset.Spot, err)
	}
	return validateCRC32(stored, snapshot.Checksum)
}

func (e *Exchange) wsProcessV2OrderbookUpdate(pair currency.Pair, update *websocketV2Book) error {
	updatedAt := update.Timestamp
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	bookUpdate := orderbook.Update{
		Asset:      asset.Spot,
		Pair:       pair,
		Bids:       make(orderbook.Levels, len(update.Bids)),
		Asks:       make(orderbook.Levels, len(update.Asks)),
		UpdateTime: updatedAt,
	}
	for i := range update.Asks {
		bookUpdate.Asks[i] = orderbook.Level{
			Price:     update.Asks[i].Price,
			Amount:    update.Asks[i].Quantity,
			StrPrice:  update.Asks[i].PriceString,
			StrAmount: update.Asks[i].QtyString,
		}
	}
	for i := range update.Bids {
		bookUpdate.Bids[i] = orderbook.Level{
			Price:     update.Bids[i].Price,
			Amount:    update.Bids[i].Quantity,
			StrPrice:  update.Bids[i].PriceString,
			StrAmount: update.Bids[i].QtyString,
		}
	}
	if err := e.Websocket.Orderbook.Update(&bookUpdate); err != nil {
		return err
	}
	book, err := e.Websocket.Orderbook.GetOrderbook(pair, asset.Spot)
	if err != nil {
		return fmt.Errorf("cannot calculate websocket checksum: book not found for %s %s %w", pair, asset.Spot, err)
	}
	return validateCRC32(book, update.Checksum)
}

func (e *Exchange) wsProcessV2Executions(ctx context.Context, data []json.RawMessage) error {
	for i := range data {
		var item WebsocketV2Execution
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling execution data: %w", err)
		}
		detail := &order.Detail{
			Exchange:             e.Name,
			OrderID:              item.OrderID,
			Amount:               item.OrderQty,
			ExecutedAmount:       item.CumulativeQty,
			AverageExecutedPrice: item.AveragePrice,
			Price:                item.LimitPrice,
			Date:                 item.Timestamp,
			LastUpdated:          item.Timestamp,
			AssetType:            asset.Spot,
		}
		if item.OrderQty >= item.CumulativeQty {
			detail.RemainingAmount = item.OrderQty - item.CumulativeQty
		}
		var err error
		if item.Symbol != "" {
			detail.Pair, err = krakenV2PairFromExchange(item.Symbol)
			if err != nil {
				return fmt.Errorf("error parsing execution symbol %q: %w", item.Symbol, err)
			}
		}
		if item.Side != "" {
			detail.Side, err = order.StringToOrderSide(item.Side)
			if err != nil {
				return err
			}
		}
		if item.OrderType != "" {
			detail.Type = krakenV2OrderType(item.OrderType)
		}
		if item.OrderStatus != "" {
			detail.Status, err = krakenV2OrderStatus(item.OrderStatus)
			if err != nil {
				return err
			}
		}
		for j := range item.Fees {
			detail.Fee += item.Fees[j].Quantity
		}
		if item.ExecutionType == "trade" {
			tid := item.ExecutionID
			if tid == "" {
				tid = strconv.FormatUint(item.TradeID, 10)
			}
			detail.Trades = []order.TradeHistory{{
				Price:     item.LastPrice,
				Amount:    item.LastQty,
				Fee:       detail.Fee,
				Exchange:  e.Name,
				TID:       tid,
				Type:      detail.Type,
				Side:      detail.Side,
				Timestamp: item.Timestamp,
			}}
		}
		if err := e.Websocket.DataHandler.Send(ctx, detail); err != nil {
			return err
		}
	}
	return nil
}

func krakenV2OrderType(value string) order.Type {
	switch value {
	case "limit", "iceberg":
		return order.Limit
	case "market", "settle-position":
		return order.Market
	case "stop-loss":
		return order.Stop
	case "stop-loss-limit":
		return order.StopLimit
	case "take-profit":
		return order.TakeProfitMarket
	case "take-profit-limit":
		return order.TakeProfit
	case "trailing-stop":
		return order.TrailingStop
	case "trailing-stop-limit":
		return order.TrailingStopLimit
	default:
		return order.UnknownType
	}
}

func krakenV2OrderTypeName(value order.Type) (string, error) {
	switch value {
	case order.Limit:
		return "limit", nil
	case order.Market:
		return "market", nil
	case order.Stop:
		return "stop-loss", nil
	case order.StopLimit:
		return "stop-loss-limit", nil
	case order.TakeProfitMarket:
		return "take-profit", nil
	case order.TakeProfit:
		return "take-profit-limit", nil
	case order.TrailingStop:
		return "trailing-stop", nil
	case order.TrailingStopLimit:
		return "trailing-stop-limit", nil
	default:
		return "", fmt.Errorf("%w: %s", order.ErrTypeIsInvalid, value)
	}
}

func krakenV2OrderStatus(value string) (order.Status, error) {
	if value == "pending_new" {
		return order.Pending, nil
	}
	return order.StringToOrderStatus(value)
}

// startWsPingHandler sets up a websocket ping handler to maintain a connection
func (e *Exchange) startWsPingHandler(conn websocket.Connection) {
	conn.SetupPingHandler(request.Unset, websocket.PingHandler{
		Message:     []byte(`{"method":"ping"}`),
		Delay:       krakenWsPingDelay,
		MessageType: gws.TextMessage,
	})
}

// wsReadDataResponse classifies the WS response and sends to appropriate handler
func (e *Exchange) wsReadDataResponse(ctx context.Context, c string, pair currency.Pair, response []json.RawMessage) error {
	switch c {
	case krakenWsTicker:
		return e.wsProcessTickers(ctx, response[1], pair)
	case krakenWsSpread:
		return e.wsProcessSpread(response[1], pair)
	case krakenWsTrade:
		return e.wsProcessTrades(ctx, response[1], pair)
	case krakenWsOwnTrades:
		return e.wsProcessOwnTrades(ctx, response[0])
	case krakenWsOpenOrders:
		return e.wsProcessOpenOrders(ctx, response[0])
	}

	channelType := strings.TrimRight(c, "-0123456789")
	switch channelType {
	case krakenWsOHLC:
		return e.wsProcessCandle(ctx, c, response[1], pair)
	case krakenWsOrderbook:
		return e.wsProcessOrderBook(ctx, c, response, pair)
	default:
		return fmt.Errorf("received unidentified data for subscription %s: %+v", c, response)
	}
}

func (e *Exchange) wsProcessSystemStatus(respRaw []byte) error {
	var systemStatus wsSystemStatus
	if err := json.Unmarshal(respRaw, &systemStatus); err != nil {
		return fmt.Errorf("%s parsing system status: %s", err, respRaw)
	}
	if systemStatus.Status != "online" {
		return fmt.Errorf("system status not online: %v", systemStatus.Status)
	}
	if systemStatus.Version > krakenWSSupportedVersion {
		log.Warnf(log.ExchangeSys, "%v New version of Websocket API released. Was %v Now %v", e.Name, krakenWSSupportedVersion, systemStatus.Version)
	}
	return nil
}

func (e *Exchange) wsProcessOwnTrades(ctx context.Context, ownOrdersRaw json.RawMessage) error {
	var result []map[string]*WsOwnTrade
	if err := json.Unmarshal(ownOrdersRaw, &result); err != nil {
		return err
	}

	if len(result) == 0 {
		return nil
	}

	for key, val := range result[0] {
		oSide, err := order.StringToOrderSide(val.Type)
		if err != nil {
			return err
		}
		oType, err := order.StringToOrderType(val.OrderType)
		if err != nil {
			return err
		}
		if err := e.Websocket.DataHandler.Send(ctx, &order.Detail{
			Exchange: e.Name,
			OrderID:  val.OrderTransactionID,
			Trades: []order.TradeHistory{
				{
					Price:     val.Price,
					Amount:    val.Vol,
					Fee:       val.Fee,
					Exchange:  e.Name,
					TID:       key,
					Type:      oType,
					Side:      oSide,
					Timestamp: val.Time.Time(),
				},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

// wsProcessOpenOrders processes open orders from the websocket response
func (e *Exchange) wsProcessOpenOrders(ctx context.Context, ownOrdersResp json.RawMessage) error {
	var result []map[string]*WsOpenOrder
	if err := json.Unmarshal(ownOrdersResp, &result); err != nil {
		return err
	}

	for r := range result {
		for key, val := range result[r] {
			d := &order.Detail{
				Exchange:             e.Name,
				OrderID:              key,
				AverageExecutedPrice: val.AveragePrice,
				Amount:               val.Volume,
				LimitPriceUpper:      val.LimitPrice,
				ExecutedAmount:       val.ExecutedVolume,
				Fee:                  val.Fee,
				Date:                 val.OpenTime.Time(),
				LastUpdated:          val.LastUpdated.Time(),
			}

			if val.Status != "" {
				var err error
				if d.Status, err = order.StringToOrderStatus(val.Status); err != nil {
					return err
				}
			}

			if val.Description.Pair != "" {
				var err error
				d.Side = order.Sell
				if !strings.Contains(val.Description.Order, "sell") {
					if d.Side, err = order.StringToOrderSide(val.Description.Type); err != nil {
						return err
					}
				}
				if d.Type, err = order.StringToOrderType(val.Description.OrderType); err != nil {
					return err
				}
				if d.Pair, err = currency.NewPairFromString(val.Description.Pair); err != nil {
					return err
				}
				if d.AssetType, err = e.GetPairAssetType(d.Pair); err != nil {
					return err
				}
			}

			if val.Description.Price > 0 {
				d.Leverage = val.Description.Leverage
				d.Price = val.Description.Price
			}

			if val.Volume > 0 {
				// Note: Volume and ExecutedVolume are only populated when status is open
				d.RemainingAmount = val.Volume - val.ExecutedVolume
			}
			if err := e.Websocket.DataHandler.Send(ctx, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// wsProcessTickers converts ticker data and sends it to the datahandler
func (e *Exchange) wsProcessTickers(ctx context.Context, dataRaw json.RawMessage, pair currency.Pair) error {
	var t wsTicker
	if err := json.Unmarshal(dataRaw, &t); err != nil {
		return fmt.Errorf("error unmarshalling ticker data: %w", err)
	}

	return e.Websocket.DataHandler.Send(ctx, &ticker.Price{
		ExchangeName: e.Name,
		Ask:          t.Ask[0].Float64(),
		Bid:          t.Bid[0].Float64(),
		Close:        t.Last[0].Float64(),
		Volume:       t.Volume[0].Float64(),
		Low:          t.Low[0].Float64(),
		High:         t.High[0].Float64(),
		Open:         t.Open[0].Float64(),
		AssetType:    asset.Spot,
		Pair:         pair,
	})
}

// wsProcessSpread converts spread/orderbook data and sends it to the datahandler
func (e *Exchange) wsProcessSpread(rawData json.RawMessage, pair currency.Pair) error {
	var data wsSpread
	if err := json.Unmarshal(rawData, &data); err != nil {
		return fmt.Errorf("error unmarshalling spread data: %w", err)
	}
	if e.Verbose {
		log.Debugf(log.ExchangeSys, "%s Spread data for %q received. Best bid: '%v' Best ask: '%v' Time: %q, Bid volume: '%v', Ask volume: '%v'",
			e.Name,
			pair,
			data.Bid.Float64(),
			data.Ask.Float64(),
			data.Time.Time(),
			data.BidVolume.Float64(),
			data.AskVolume.Float64())
	}
	return nil
}

// wsProcessTrades converts trade data and sends it to the datahandler
func (e *Exchange) wsProcessTrades(ctx context.Context, respRaw json.RawMessage, pair currency.Pair) error {
	saveTradeData := e.IsSaveTradeDataEnabled()
	tradeFeed := e.IsTradeFeedEnabled()
	if !saveTradeData && !tradeFeed {
		return nil
	}

	var t []wsTrades
	if err := json.Unmarshal(respRaw, &t); err != nil {
		return fmt.Errorf("error unmarshalling trade data: %w", err)
	}

	trades := make([]trade.Data, len(t))
	for i := range trades {
		side := order.Buy
		if t[i].Side == "s" {
			side = order.Sell
		}
		trades[i] = trade.Data{
			AssetType:    asset.Spot,
			CurrencyPair: pair,
			Exchange:     e.Name,
			Price:        t[i].Price.Float64(),
			Amount:       t[i].Volume.Float64(),
			Timestamp:    t[i].Time.Time().UTC(),
			Side:         side,
		}
	}
	if tradeFeed {
		for i := range trades {
			if err := e.Websocket.DataHandler.Send(ctx, trades[i]); err != nil {
				return err
			}
		}
	}
	if saveTradeData {
		return trade.AddTradesToBuffer(trades...)
	}
	return nil
}

func hasKey(raw json.RawMessage, key string) bool {
	_, dataType, _, err := jsonparser.Get(raw, key)
	if err != nil || dataType == jsonparser.NotExist {
		return false
	}
	return true
}

// wsProcessOrderBook handles both partial and full orderbook updates
func (e *Exchange) wsProcessOrderBook(ctx context.Context, c string, response []json.RawMessage, pair currency.Pair) error {
	key := &subscription.Subscription{
		Channel: c,
		Asset:   asset.Spot,
		Pairs:   currency.Pairs{pair},
	}
	if err := fqChannelNameSub(key); err != nil {
		return err
	}
	s := e.Websocket.GetSubscription(key)
	if s == nil {
		return fmt.Errorf("%w: %s %s %s", subscription.ErrNotFound, asset.Spot, c, pair)
	}
	if s.State() == subscription.UnsubscribingState {
		// We only care if it's currently unsubscribing
		return nil
	}

	if isSnapshot := hasKey(response[1], "as") && hasKey(response[1], "bs"); !isSnapshot {
		var update wsUpdate
		if err := json.Unmarshal(response[1], &update); err != nil {
			return fmt.Errorf("error unmarshalling orderbook update: %w", err)
		}
		if len(response) == 5 {
			var update2 wsUpdate
			if err := json.Unmarshal(response[2], &update2); err != nil {
				return fmt.Errorf("error unmarshalling orderbook update: %w", err)
			}
			update.Bids = make([]wsOrderbookItem, len(update2.Bids))
			copy(update.Bids, update2.Bids)
			update.Checksum = update2.Checksum
		}
		err := e.wsProcessOrderBookUpdate(pair, &update)
		if errors.Is(err, errInvalidChecksum) {
			log.Debugf(log.Global, "%s Resubscribing to invalid %s orderbook", e.Name, pair)
			go func() {
				if e2 := e.Websocket.ResubscribeToChannel(ctx, e.Websocket.Conn, s); e2 != nil && !errors.Is(e2, subscription.ErrInStateAlready) {
					log.Errorf(log.ExchangeSys, "%s resubscription failure for %v: %v", e.Name, pair, e2)
				}
			}()
		}
		return err
	}

	var snapshot wsSnapshot
	if err := json.Unmarshal(response[1], &snapshot); err != nil {
		return fmt.Errorf("error unmarshalling orderbook snapshot: %w", err)
	}
	return e.wsProcessOrderBookPartial(pair, &snapshot, key.Levels)
}

// wsProcessOrderBookPartial creates a new orderbook entry for a given currency pair
func (e *Exchange) wsProcessOrderBookPartial(pair currency.Pair, obSnapshot *wsSnapshot, levels int) error {
	base := orderbook.Book{
		Pair:                   pair,
		Asset:                  asset.Spot,
		ValidateOrderbook:      e.ValidateOrderbook,
		Bids:                   make(orderbook.Levels, len(obSnapshot.Bids)),
		Asks:                   make(orderbook.Levels, len(obSnapshot.Asks)),
		MaxDepth:               levels,
		ChecksumStringRequired: true,
	}
	// Kraken ob data is timestamped per price, GCT orderbook data is
	// timestamped per entry using the highest last update time, we can attempt
	// to respect both within a reasonable degree
	var highestLastUpdate time.Time
	for i := range obSnapshot.Asks {
		base.Asks[i].Price = obSnapshot.Asks[i].Price
		base.Asks[i].StrPrice = obSnapshot.Asks[i].PriceRaw
		base.Asks[i].Amount = obSnapshot.Asks[i].Amount
		base.Asks[i].StrAmount = obSnapshot.Asks[i].AmountRaw

		askUpdatedTime := obSnapshot.Asks[i].Time.Time()
		if highestLastUpdate.Before(askUpdatedTime) {
			highestLastUpdate = askUpdatedTime
		}
	}

	for i := range obSnapshot.Bids {
		base.Bids[i].Price = obSnapshot.Bids[i].Price
		base.Bids[i].StrPrice = obSnapshot.Bids[i].PriceRaw
		base.Bids[i].Amount = obSnapshot.Bids[i].Amount
		base.Bids[i].StrAmount = obSnapshot.Bids[i].AmountRaw

		bidUpdateTime := obSnapshot.Bids[i].Time.Time()
		if highestLastUpdate.Before(bidUpdateTime) {
			highestLastUpdate = bidUpdateTime
		}
	}
	base.LastUpdated = highestLastUpdate
	base.Exchange = e.Name
	return e.Websocket.Orderbook.LoadSnapshot(&base)
}

// wsProcessOrderBookUpdate updates an orderbook entry for a given currency pair
func (e *Exchange) wsProcessOrderBookUpdate(pair currency.Pair, wsUpdt *wsUpdate) error {
	obUpdate := orderbook.Update{
		Asset: asset.Spot,
		Pair:  pair,
		Bids:  make(orderbook.Levels, len(wsUpdt.Bids)),
		Asks:  make(orderbook.Levels, len(wsUpdt.Asks)),
	}

	// Calculating checksum requires incoming decimal place checks for both
	// price and amount as there is no set standard between currency pairs. This
	// is calculated per update as opposed to snapshot because changes to
	// decimal amounts could occur at any time.
	var highestLastUpdate time.Time
	// Ask data is not always sent
	for i := range wsUpdt.Asks {
		obUpdate.Asks[i].Price = wsUpdt.Asks[i].Price
		obUpdate.Asks[i].StrPrice = wsUpdt.Asks[i].PriceRaw
		obUpdate.Asks[i].Amount = wsUpdt.Asks[i].Amount
		obUpdate.Asks[i].StrAmount = wsUpdt.Asks[i].AmountRaw

		askUpdatedTime := wsUpdt.Asks[i].Time.Time()
		if highestLastUpdate.Before(askUpdatedTime) {
			highestLastUpdate = askUpdatedTime
		}
	}

	// Bid data is not always sent
	for i := range wsUpdt.Bids {
		obUpdate.Bids[i].Price = wsUpdt.Bids[i].Price
		obUpdate.Bids[i].StrPrice = wsUpdt.Bids[i].PriceRaw
		obUpdate.Bids[i].Amount = wsUpdt.Bids[i].Amount
		obUpdate.Bids[i].StrAmount = wsUpdt.Bids[i].AmountRaw

		bidUpdatedTime := wsUpdt.Bids[i].Time.Time()
		if highestLastUpdate.Before(bidUpdatedTime) {
			highestLastUpdate = bidUpdatedTime
		}
	}
	obUpdate.UpdateTime = highestLastUpdate

	err := e.Websocket.Orderbook.Update(&obUpdate)
	if err != nil {
		return err
	}

	book, err := e.Websocket.Orderbook.GetOrderbook(pair, asset.Spot)
	if err != nil {
		return fmt.Errorf("cannot calculate websocket checksum: book not found for %s %s %w", pair, asset.Spot, err)
	}

	return validateCRC32(book, wsUpdt.Checksum)
}

func validateCRC32(b *orderbook.Book, token uint32) error {
	if b == nil {
		return common.ErrNilPointer
	}
	var checkStr strings.Builder
	for i := 0; i < 10 && i < len(b.Asks); i++ {
		_, err := checkStr.WriteString(trim(b.Asks[i].StrPrice + trim(b.Asks[i].StrAmount)))
		if err != nil {
			return err
		}
	}

	for i := 0; i < 10 && i < len(b.Bids); i++ {
		_, err := checkStr.WriteString(trim(b.Bids[i].StrPrice) + trim(b.Bids[i].StrAmount))
		if err != nil {
			return err
		}
	}

	if check := crc32.ChecksumIEEE([]byte(checkStr.String())); check != token {
		return fmt.Errorf("%s %s %w %d, expected %d", b.Pair, b.Asset, errInvalidChecksum, check, token)
	}
	return nil
}

// trim removes '.' and prefixed '0' from subsequent string
func trim(s string) string {
	s = strings.Replace(s, ".", "", 1)
	s = strings.TrimLeft(s, "0")
	return s
}

// wsProcessCandle converts candle data and sends it to the data handler
func (e *Exchange) wsProcessCandle(ctx context.Context, c string, resp json.RawMessage, pair currency.Pair) error {
	var data wsCandle
	if err := json.Unmarshal(resp, &data); err != nil {
		return fmt.Errorf("error unmarshalling candle data: %w", err)
	}

	// Faster than getting it through the subscription
	parts := strings.Split(c, "-")
	if len(parts) != 2 {
		return errBadChannelSuffix
	}
	intervalMinutes, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("%w: %s", kline.ErrInvalidInterval, c)
	}
	volume := data.Volume.Float64()
	vwap := data.VWAP.Float64()
	return e.Websocket.DataHandler.Send(ctx, kline.Item{
		Asset:    asset.Spot,
		Pair:     pair,
		Exchange: e.Name,
		Interval: kline.Interval(time.Minute * time.Duration(intervalMinutes)),
		Candles: []kline.Candle{{
			Time:        data.LastUpdateTime.Time(),
			Open:        data.Open.Float64(),
			High:        data.High.Float64(),
			Low:         data.Low.Float64(),
			Close:       data.Close.Float64(),
			Volume:      volume,
			QuoteVolume: volume * vwap,
		}},
	})
}

// GetSubscriptionTemplate returns a subscription channel template
func (e *Exchange) GetSubscriptionTemplate(_ *subscription.Subscription) (*template.Template, error) {
	return template.New("master.tmpl").Funcs(template.FuncMap{"channelName": channelName}).Parse(subTplText)
}

func (e *Exchange) generateSubscriptions() (subscription.List, error) {
	return e.Features.Subscriptions.ExpandTemplates(e)
}

// Subscribe adds a channel subscription to the websocket
func (e *Exchange) Subscribe(in subscription.List) error {
	ctx := context.TODO()
	in, errs := in.ExpandTemplates(e)

	// Collect valid new subs and add to websocket in Subscribing state
	subs := subscription.List{}
	for _, s := range in {
		if s.State() != subscription.ResubscribingState {
			if err := e.Websocket.AddSubscriptions(e.Websocket.Conn, s); err != nil {
				errs = common.AppendError(errs, fmt.Errorf("%w; Channel: %s Pairs: %s", err, s.Channel, s.Pairs.Join()))
				continue
			}
		}
		subs = append(subs, s)
	}

	// Merge subs by grouping pairs for request; We make a single request to subscribe to N+ pairs, but get N+ responses back
	groupedSubs := subs.GroupPairs()

	errs = common.AppendError(errs,
		e.ParallelChanOp(ctx, groupedSubs, func(ctx context.Context, s subscription.List) error { return e.manageSubs(ctx, krakenWsSubscribe, s) }, 1),
	)

	for _, s := range subs {
		if s.State() != subscription.SubscribedState {
			_ = s.SetState(subscription.InactiveState)
			if err := e.Websocket.RemoveSubscriptions(e.Websocket.Conn, s); err != nil {
				errs = common.AppendError(errs, fmt.Errorf("error removing failed subscription: %w; Channel: %s Pairs: %s", err, s.Channel, s.Pairs.Join()))
			}
		}
	}

	return errs
}

// Unsubscribe removes a channel subscriptions from the websocket
func (e *Exchange) Unsubscribe(keys subscription.List) error {
	ctx := context.TODO()
	var errs error
	// Make sure we have the concrete subscriptions, since we will change the state
	subs := make(subscription.List, 0, len(keys))
	for _, key := range keys {
		if s := e.Websocket.GetSubscription(key); s == nil {
			errs = common.AppendError(errs, fmt.Errorf("%w; Channel: %s Pairs: %s", subscription.ErrNotFound, key.Channel, key.Pairs.Join()))
		} else {
			if s.State() != subscription.ResubscribingState {
				if err := s.SetState(subscription.UnsubscribingState); err != nil {
					errs = common.AppendError(errs, fmt.Errorf("%w; Channel: %s Pairs: %s", err, s.Channel, s.Pairs.Join()))
					continue
				}
			}
			subs = append(subs, s)
		}
	}

	subs = subs.GroupPairs()

	return common.AppendError(errs,
		e.ParallelChanOp(ctx, subs, func(ctx context.Context, s subscription.List) error { return e.manageSubs(ctx, krakenWsUnsubscribe, s) }, 1),
	)
}

// manageSubs handles both websocket channel subscribe and unsubscribe
func (e *Exchange) manageSubs(ctx context.Context, op string, subs subscription.List) error {
	if len(subs) != 1 {
		return subscription.ErrBatchingNotSupported
	}

	s := subs[0]

	if err := enforceStandardChannelNames(s); err != nil {
		return err
	}

	reqFmt := currency.PairFormat{Uppercase: true, Delimiter: "/"}
	r := &WebsocketV2Request[WebsocketV2SubscriptionParams]{
		Method:    op,
		RequestID: e.MessageSequence(),
		Params: WebsocketV2SubscriptionParams{
			Channel: s.QualifiedChannel,
			Depth:   s.Levels,
			Symbols: krakenV2PairsToExchange(s.Pairs).Format(reqFmt).Strings(),
		},
	}

	if s.Interval != 0 {
		r.Params.Interval = int(time.Duration(s.Interval).Minutes())
	}

	conn := e.Websocket.Conn
	if s.Authenticated {
		r.Params.Token = e.websocketAuthToken()
		if op == krakenWsSubscribe && s.QualifiedChannel == krakenWsV2Executions {
			r.Params.SnapOrders = true
			r.Params.SnapTrades = true
		}
		conn = e.Websocket.AuthConn
	}

	expectedResponses := len(s.Pairs)
	if expectedResponses == 0 {
		expectedResponses = 1
	}
	resps, err := conn.SendMessageReturnResponses(ctx, request.Unset, r.RequestID, r, expectedResponses)

	// Ignore an overall timeout, because we'll track individual subscriptions in handleSubResps
	err = common.ExcludeError(err, websocket.ErrSignatureTimeout)
	if err != nil {
		return fmt.Errorf("%w; Channel: %s Pair: %s", err, s.Channel, s.Pairs)
	}

	return e.handleSubResps(s, resps, op)
}

// handleSubResps takes a collection of subscription responses from Kraken
// We submit a subscription for N+ pairs, and we get N+ individual responses
// Returns an error collection of unique errors and its pairs
func (e *Exchange) handleSubResps(s *subscription.Subscription, resps [][]byte, op string) error {
	reqFmt := currency.PairFormat{Uppercase: true, Delimiter: "/"}

	if len(s.Pairs) == 0 {
		if len(resps) != 1 {
			return fmt.Errorf("%w; got %d; Channel: %s", errExpectedOneSubResponse, len(resps), s.Channel)
		}
		if err := e.getSubRespErr(resps[0], op); err != nil {
			return fmt.Errorf("%w; Channel: %s", err, s.Channel)
		}
		return nil
	}

	errMap := map[string]error{}
	pairErrs := map[currency.Pair]error{}
	for _, p := range s.Pairs {
		pairErrs[p.Format(reqFmt)] = errSubPairMissing
	}

	subPairs := currency.Pairs{}
	for _, resp := range resps {
		var response websocketV2Response
		if err := json.Unmarshal(resp, &response); err != nil {
			return fmt.Errorf("%w parsing WS response from message: %s", err, resp)
		}
		pName := response.Result.Symbol
		if pName == "" {
			pName = response.Symbol
		}
		if pName == "" {
			return fmt.Errorf("%w parsing WS symbol from message: %s", errSubPairMissing, resp)
		}
		pair, err := krakenV2PairFromExchange(pName)
		if err != nil {
			return fmt.Errorf("%w parsing WS pair; Channel: %s Pair: %s", err, s.Channel, pName)
		}
		if err := e.getSubRespErr(resp, op); err != nil {
			// Remove the pair name from the error so we can group errors
			errStr := strings.TrimSpace(strings.TrimSuffix(err.Error(), pName))
			if _, ok := errMap[errStr]; !ok {
				errMap[errStr] = errors.New(errStr)
			}
			pairErrs[pair] = errMap[errStr]
		} else {
			delete(pairErrs, pair)
			if e.Verbose && op == krakenWsSubscribe {
				subPairs = subPairs.Add(pair)
			}
		}
	}

	// 2) Reverse the collection and report a list of pairs with each unique error, and re-add the missing and error pairs for unsubscribe
	errPairs := map[error]currency.Pairs{}
	for pair, err := range pairErrs {
		errPairs[err] = errPairs[err].Add(pair)
	}

	var errs error
	for err, pairs := range errPairs {
		errs = common.AppendError(errs, fmt.Errorf("%w; Channel: %s Pairs: %s", err, s.Channel, pairs.Join()))
	}

	if e.Verbose && len(subPairs) > 0 {
		log.Debugf(log.ExchangeSys, "%s Subscribed to Channel: %s Pairs: %s", e.Name, s.Channel, subPairs.Join())
	}

	return errs
}

// getSubRespErr calls getRespErr and if there's no error from that ensures the status matches the sub operation
func (e *Exchange) getSubRespErr(resp []byte, op string) error {
	if err := e.getRespErr(resp); err != nil {
		return err
	}
	method, err := jsonparser.GetUnsafeString(resp, "method")
	if err != nil {
		return fmt.Errorf("error parsing WS method: %w from message: %s", err, resp)
	}
	if method != op {
		return fmt.Errorf("wrong WS method: %s; expected: %s from message %s", method, op, resp)
	}
	return nil
}

// getRespErr takes a json response string and looks for an error event type
// If found it returns the errorMessage
// It might log parsing errors about the nature of the error
// If the error message is not defined it will return a wrapped errUnknownError
func (e *Exchange) getRespErr(resp []byte) error {
	var response websocketV2Response
	if err := json.Unmarshal(resp, &response); err != nil {
		return fmt.Errorf("error parsing WS response: %w from message: %s", err, resp)
	}
	if response.Success == nil {
		return fmt.Errorf("error parsing WS success from message: %s", resp)
	}
	if *response.Success {
		return nil
	}
	if response.Error == "" {
		return fmt.Errorf("%w: error response did not contain an error: %s", common.ErrUnknownError, resp)
	}
	return errors.New(response.Error)
}

// wsProcessSubStatus handles creating or removing Subscriptions as soon as we receive a message
// It's job is to ensure that subscription state is kept correct sequentially between WS messages
// If this responsibility was moved to Subscribe then we would have a race due to the channel connecting IncomingWithData
func (e *Exchange) wsProcessSubStatus(resp []byte) {
	var response websocketV2Response
	if err := json.Unmarshal(resp, &response); err != nil || response.Result.Channel == "" {
		return
	}
	if err := e.getRespErr(resp); err != nil {
		return
	}

	channel, ok := reverseChannelNames[response.Result.Channel]
	if !ok {
		return
	}
	keySub := &subscription.Subscription{
		Channel:  channel,
		Asset:    asset.Spot,
		Levels:   response.Result.Depth,
		Interval: kline.Interval(time.Minute * time.Duration(response.Result.Interval)),
	}
	lookupKey := any(subscription.ChannelKey{Subscription: keySub})
	if response.Result.Symbol != "" {
		pair, err := krakenV2PairFromExchange(response.Result.Symbol)
		if err != nil {
			log.Errorf(log.ExchangeSys, "%s error parsing websocket subscription pair %q: %s from message: %s", e.Name, response.Result.Symbol, err, resp)
			return
		}
		keySub.Pairs = currency.Pairs{pair}
		lookupKey = &subscription.IgnoringAssetKey{Subscription: keySub}
	} else {
		keySub.Asset = asset.Empty
	}
	s := e.Websocket.GetSubscription(lookupKey)
	if s == nil {
		log.Errorf(log.ExchangeSys, "%s %s Channel: %s Pairs: %s", e.Name, subscription.ErrNotFound, keySub.Channel, keySub.Pairs.Join())
		return
	}

	var err error
	if response.Method == krakenWsSubscribe {
		err = s.SetState(subscription.SubscribedState)
	} else if s.State() != subscription.ResubscribingState { // Do not remove a resubscribing sub which just unsubbed
		conn := e.Websocket.Conn
		if s.Authenticated {
			conn = e.Websocket.AuthConn
		}
		err = e.Websocket.RemoveSubscriptions(conn, s)
		if e2 := s.SetState(subscription.UnsubscribedState); e2 != nil {
			err = common.AppendError(err, e2)
		}
	}

	if err != nil {
		log.Errorf(log.ExchangeSys, "%s %s Channel: %s Pairs: %s", e.Name, err, s.Channel, s.Pairs.Join())
	}
}

// channelName converts a global channel name to kraken bespoke names
func channelName(s *subscription.Subscription) string {
	if n, ok := channelNames[s.Channel]; ok {
		return n
	}
	return s.Channel
}

func enforceStandardChannelNames(s *subscription.Subscription) error {
	if s.Channel == subscription.MyTradesChannel || s.Channel == subscription.MyOrdersChannel {
		return fmt.Errorf("%w: use subscription.MyAccountChannel for Kraken v2 executions", subscription.ErrNotSupported)
	}
	name := strings.Split(s.Channel, "-") // Protect against attempted usage of book-N as a channel name
	if n, ok := reverseChannelNames[name[0]]; ok && n != s.Channel {
		return fmt.Errorf("%w: %s => subscription.%s%sChannel", subscription.ErrUseConstChannelName, s.Channel, bytes.ToUpper([]byte{n[0]}), n[1:])
	}
	return nil
}

// fqChannelNameSub converts an fully qualified channel name into standard name and subscription params
// e.g. book-5 => subscription.OrderbookChannel with Levels: 5
func fqChannelNameSub(s *subscription.Subscription) error {
	parts := strings.Split(s.Channel, "-")
	name := parts[0]
	if stdName, ok := reverseChannelNames[name]; ok {
		name = stdName
	}

	if name == subscription.OrderbookChannel || name == subscription.CandlesChannel {
		if len(parts) != 2 {
			return errBadChannelSuffix
		}
		i, err := strconv.Atoi(parts[1])
		if err != nil {
			return errBadChannelSuffix
		}
		switch name {
		case subscription.OrderbookChannel:
			s.Levels = i
		case subscription.CandlesChannel:
			s.Interval = kline.Interval(time.Minute * time.Duration(i))
		}
	}

	s.Channel = name

	return nil
}

// wsAddOrder creates an order and returns its order ID.
func (e *Exchange) wsAddOrder(ctx context.Context, params *WebsocketV2AddOrderParams) (string, error) {
	if params == nil {
		return "", common.ErrNilPointer
	}
	pair, err := currency.NewPairDelimiter(params.Symbol, "/")
	if err != nil {
		return "", fmt.Errorf("invalid add_order symbol %q: %w", params.Symbol, err)
	}
	params.Symbol = krakenV2PairToExchange(pair).Format(currency.PairFormat{Uppercase: true, Delimiter: "/"}).String()
	params.Token = e.websocketAuthToken()
	req := WebsocketV2Request[WebsocketV2AddOrderParams]{
		Method:    krakenWsV2AddOrder,
		Params:    *params,
		RequestID: e.MessageSequence(),
	}
	jsonResp, err := e.Websocket.AuthConn.SendMessageReturnResponse(ctx, request.Unset, req.RequestID, &req)
	if err != nil {
		return "", err
	}
	var resp websocketV2Response
	if err := json.Unmarshal(jsonResp, &resp); err != nil {
		return "", err
	}
	if err := e.getRespErr(jsonResp); err != nil {
		return "", fmt.Errorf("add order: %w", err)
	}
	if resp.Result.OrderID == "" {
		return "", fmt.Errorf("%w: add_order response did not contain order_id", common.ErrInvalidResponse)
	}
	if err := e.Websocket.DataHandler.Send(ctx, &order.Detail{
		Exchange: e.Name,
		OrderID:  resp.Result.OrderID,
		Status:   order.New,
	}); err != nil {
		return "", err
	}
	return resp.Result.OrderID, nil
}

// wsCancelOrders cancels one or more open orders in a single v2 request.
func (e *Exchange) wsCancelOrders(ctx context.Context, orderIDs []string) error {
	if len(orderIDs) == 0 {
		return nil
	}
	req := WebsocketV2Request[WebsocketV2CancelOrderParams]{
		Method: krakenWsV2CancelOrder,
		Params: WebsocketV2CancelOrderParams{
			OrderIDs: orderIDs,
			Token:    e.websocketAuthToken(),
		},
		RequestID: e.MessageSequence(),
	}
	responses, err := e.Websocket.AuthConn.SendMessageReturnResponses(ctx, request.Unset, req.RequestID, &req, len(orderIDs))
	if err != nil {
		return fmt.Errorf("%w: %w", errCancellingOrder, err)
	}
	var errs error
	for i := range responses {
		var resp websocketV2Response
		if err := json.Unmarshal(responses[i], &resp); err != nil {
			errs = common.AppendError(errs, fmt.Errorf("%w: %w", errCancellingOrder, err))
			continue
		}
		if err := e.getRespErr(responses[i]); err != nil {
			orderID := resp.Result.OrderID
			if orderID == "" && i < len(orderIDs) {
				orderID = orderIDs[i]
			}
			errs = common.AppendError(errs, fmt.Errorf("%w %s: %w", errCancellingOrder, orderID, err))
		}
	}
	return errs
}

// wsCancelAllOrders cancels all opened orders
// Returns number (count param) of affected orders or 0 if no open orders found
func (e *Exchange) wsCancelAllOrders(ctx context.Context) (*WsCancelOrderResponse, error) {
	req := WebsocketV2Request[WebsocketV2CancelAllParams]{
		Method:    krakenWsV2CancelAll,
		Params:    WebsocketV2CancelAllParams{Token: e.websocketAuthToken()},
		RequestID: e.MessageSequence(),
	}

	jsonResp, err := e.Websocket.AuthConn.SendMessageReturnResponse(ctx, request.Unset, req.RequestID, &req)
	if err != nil {
		return nil, err
	}
	if err := e.getRespErr(jsonResp); err != nil {
		return nil, err
	}
	var resp websocketV2Response
	if err := json.Unmarshal(jsonResp, &resp); err != nil {
		return nil, err
	}
	return &WsCancelOrderResponse{Count: resp.Result.Count}, nil
}

/*
One sub per-pair. We don't use one sub with many pairs because:
  - Kraken will fan out in responses anyay
  - resubscribe is messy when our subs don't match their respsonses
  - FlushChannels and GetChannelDiff would incorrectly resub existing subs if we don't generate the same as we've stored
*/
const subTplText = `
{{- if $.S.Asset -}}
	{{ range $asset, $pairs := $.AssetPairs }}
		{{- range $p := $pairs  -}}
			{{- channelName $.S }}
			{{- $.PairSeparator }}
		{{- end -}}
		{{ $.AssetSeparator }}
	{{- end -}}
{{- else -}}
	{{- channelName $.S }}
{{- end }}
`

// websocketAuthToken retrieves the current websocket session's auth token
func (e *Exchange) websocketAuthToken() string {
	e.wsAuthMtx.RLock()
	defer e.wsAuthMtx.RUnlock()
	return e.wsAuthToken
}

func (e *Exchange) setWebsocketAuthToken(token string) {
	e.wsAuthMtx.Lock()
	e.wsAuthToken = token
	e.wsAuthMtx.Unlock()
}
