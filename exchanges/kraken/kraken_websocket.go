package kraken

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

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
	krakenWSURL        = "wss://ws.kraken.com/v2"
	krakenAuthWSURL    = "wss://ws-auth.kraken.com/v2"
	wsSupportedVersion = "v2"

	// Websocket Channels
	wsAddOrder    = "add_order"
	wsCancelAll   = "cancel_all"
	wsCancelOrder = "cancel_order"
	wsExecutions  = "executions"
	wsHeartbeat   = "heartbeat"
	wsOHLC        = "ohlc"
	wsOrderbook   = "book"
	wsPong        = "pong"
	wsStatus      = "status"
	wsSubscribe   = "subscribe"
	wsTicker      = "ticker"
	wsTrade       = "trade"
	wsUnsubscribe = "unsubscribe"

	wsDefaultBookDepth = 10
	wsPingDelay        = time.Second * 27
)

var channelNames = map[string]string{
	subscription.TickerChannel:    wsTicker,
	subscription.OrderbookChannel: wsOrderbook,
	subscription.CandlesChannel:   wsOHLC,
	subscription.AllTradesChannel: wsTrade,
	subscription.MyAccountChannel: wsExecutions,
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
	errTrackingValueNotSet    = errors.New("tracking value not set for a trailing-stop order")
	errTriggerPriceNotSet     = errors.New("trigger price not set for a triggered order")
	errEndTimeNotSet          = errors.New("end time not set for a good-till-date order")
	errEndTimeOutOfRange      = errors.New("end time must be in the future and no more than one month away")
	errExecutionSequence      = errors.New("invalid executions sequence")
)

func currencyToExchange(c currency.Code) currency.Code {
	switch {
	case c.Equal(currency.XBT), c.Equal(currency.XXBT):
		return currency.BTC
	case c.Equal(currency.XDG), c.Equal(currency.XXDG):
		return currency.DOGE
	default:
		return c
	}
}

func currencyFromExchange(c currency.Code) currency.Code {
	switch {
	case c.Equal(currency.BTC):
		return currency.XBT
	case c.Equal(currency.DOGE):
		return currency.XDG
	default:
		return c
	}
}

func pairToExchange(p currency.Pair) currency.Pair {
	p.Base = currencyToExchange(p.Base)
	p.Quote = currencyToExchange(p.Quote)
	return p
}

func pairFromExchange(symbol string) (currency.Pair, error) {
	p, err := currency.NewPairDelimiter(symbol, "/")
	if err != nil {
		return currency.EMPTYPAIR, err
	}
	p.Base = currencyFromExchange(p.Base)
	p.Quote = currencyFromExchange(p.Quote)
	p.Delimiter = ""
	return p, nil
}

type pairChannelKey struct {
	*subscription.Subscription
}

func (k pairChannelKey) Match(eachKey subscription.MatchableKey) bool {
	if k.Subscription == nil || eachKey == nil || len(k.Pairs) != 1 {
		return false
	}
	eachSubscription := eachKey.GetSubscription()
	return eachSubscription != nil &&
		eachSubscription.Channel == k.Channel &&
		eachSubscription.Asset == k.Asset &&
		eachSubscription.Pairs.Contains(k.Pairs[0], true)
}

func (k pairChannelKey) GetSubscription() *subscription.Subscription {
	return k.Subscription
}

func (k pairChannelKey) String() string {
	if k.Subscription == nil {
		return "Uninitialised pairChannelKey"
	}
	return k.Subscription.String()
}

func pairsToExchange(pairs currency.Pairs) currency.Pairs {
	translated := make(currency.Pairs, len(pairs))
	for i := range pairs {
		translated[i] = pairToExchange(pairs[i])
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
				e.setWebsocketAuthToken(authToken.Token)
				e.executionSequenceMtx.Lock()
				e.executionSequence = 0
				e.executionResubPending = false
				e.executionSequenceMtx.Unlock()
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
	var envelope struct {
		Channel string `json:"channel"`
		Method  string `json:"method"`
	}
	if err := json.Unmarshal(respRaw, &envelope); err != nil {
		return fmt.Errorf("error unmarshalling WebSocket message envelope: %w", err)
	}
	switch {
	case envelope.Channel != "":
		return e.wsHandleMessage(ctx, respRaw)
	case envelope.Method != "":
		return e.wsHandleResponse(ctx, respRaw)
	default:
		return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{
			Message: fmt.Sprintf("%s: %s", websocket.UnhandledMessage, respRaw),
		})
	}
}

func (e *Exchange) wsHandleResponse(ctx context.Context, respRaw []byte) error {
	var resp websocketResponse
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return fmt.Errorf("%w: error unmarshalling WebSocket response: %w", common.ErrInvalidResponse, err)
	}

	if resp.Method == wsSubscribe || resp.Method == wsUnsubscribe {
		e.wsProcessSubStatus(respRaw)
	}

	if resp.RequestID != 0 && e.Websocket.Match.IncomingWithData(resp.RequestID, respRaw) {
		return nil
	}
	if resp.Method == wsPong {
		return nil
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	if resp.Method == wsSubscribe || resp.Method == wsUnsubscribe ||
		resp.Method == wsAddOrder || resp.Method == wsCancelOrder || resp.Method == wsCancelAll {
		return fmt.Errorf("%w: %s %v", websocket.ErrSignatureNotMatched, resp.Method, resp.RequestID)
	}
	return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{
		Message: fmt.Sprintf("%s: %s", websocket.UnhandledMessage, respRaw),
	})
}

func (e *Exchange) wsHandleMessage(ctx context.Context, respRaw []byte) error {
	var msg websocketMessage
	if err := json.Unmarshal(respRaw, &msg); err != nil {
		return fmt.Errorf("error unmarshalling WebSocket message: %w", err)
	}

	switch msg.Channel {
	case wsHeartbeat:
		return nil
	case wsStatus:
		return e.wsProcessStatus(msg.Data)
	case wsTicker:
		return e.wsProcessTickers(ctx, msg.Data)
	case wsTrade:
		return e.wsProcessTrades(ctx, msg.Data)
	case wsOHLC:
		return e.wsProcessCandles(ctx, msg.Data)
	case wsOrderbook:
		return e.wsProcessOrderbooks(ctx, msg.Type, msg.Data)
	case wsExecutions:
		err := e.validateExecutionSequence(msg.Type, msg.Sequence)
		if err == nil {
			err = e.wsProcessExecutions(ctx, msg.Data)
			if err == nil {
				if msg.Type == "snapshot" {
					e.executionSequenceMtx.Lock()
					e.executionResubPending = false
					e.executionSequenceMtx.Unlock()
				}
				return nil
			}
			e.executionSequenceMtx.Lock()
			e.executionSequence = 0
			e.executionSequenceMtx.Unlock()
		}

		e.executionSequenceMtx.Lock()
		if msg.Type == "snapshot" {
			e.executionResubPending = false
		}
		shouldResubscribe := !e.executionResubPending
		e.executionResubPending = true
		e.executionSequenceMtx.Unlock()
		if shouldResubscribe {
			sub := e.Websocket.GetSubscription(subscription.ChannelKey{Subscription: &subscription.Subscription{
				Channel: subscription.MyAccountChannel,
			}})
			if sub != nil {
				go func() {
					resubErr := e.Websocket.ResubscribeToChannel(ctx, e.Websocket.AuthConn, sub)
					if resubErr == nil || errors.Is(resubErr, subscription.ErrInStateAlready) {
						return
					}

					if sub.State() == subscription.ResubscribingState {
						_ = sub.SetState(subscription.SubscribedState)
					}
					e.executionSequenceMtx.Lock()
					e.executionResubPending = false
					e.executionSequenceMtx.Unlock()
					log.Errorf(log.ExchangeSys, "%s executions resubscription failure: %v", e.Name, resubErr)
				}()
			} else {
				e.executionSequenceMtx.Lock()
				e.executionResubPending = false
				e.executionSequenceMtx.Unlock()
			}
		}
		return err
	default:
		return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{
			Message: fmt.Sprintf("%s: %s", websocket.UnhandledMessage, respRaw),
		})
	}
}

func (e *Exchange) validateExecutionSequence(messageType string, sequence uint64) error {
	e.executionSequenceMtx.Lock()
	defer e.executionSequenceMtx.Unlock()

	if sequence == 0 {
		e.executionSequence = 0
		return fmt.Errorf("%w: sequence is missing", errExecutionSequence)
	}
	if messageType == "snapshot" {
		e.executionSequence = sequence
		return nil
	}
	if messageType != "update" {
		e.executionSequence = 0
		return fmt.Errorf("%w: unsupported message type %q", errExecutionSequence, messageType)
	}
	if e.executionSequence == 0 {
		return fmt.Errorf("%w: update %d received before a snapshot", errExecutionSequence, sequence)
	}
	expected := e.executionSequence + 1
	if sequence != expected {
		e.executionSequence = 0
		return fmt.Errorf("%w: received %d, expected %d", errExecutionSequence, sequence, expected)
	}
	e.executionSequence = sequence
	return nil
}

func (e *Exchange) wsProcessStatus(data []json.RawMessage) error {
	if len(data) != 1 {
		return fmt.Errorf("expected one status item, received %d", len(data))
	}
	var status websocketStatus
	if err := json.Unmarshal(data[0], &status); err != nil {
		return fmt.Errorf("error unmarshalling status data: %w", err)
	}
	if status.System != "online" {
		return fmt.Errorf("system status not online: %s", status.System)
	}
	if status.APIVersion != wsSupportedVersion {
		return fmt.Errorf("unsupported WebSocket API version: %s", status.APIVersion)
	}
	return nil
}

func (e *Exchange) wsProcessTickers(ctx context.Context, data []json.RawMessage) error {
	for i := range data {
		var item websocketTicker
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling ticker data: %w", err)
		}
		pair, err := pairFromExchange(item.Symbol)
		if err != nil {
			return fmt.Errorf("error parsing ticker symbol %q: %w", item.Symbol, err)
		}
		if err := e.Websocket.DataHandler.Send(ctx, &ticker.Price{
			ExchangeName: e.Name,
			Ask:          item.Ask,
			AskSize:      item.AskQty,
			Bid:          item.Bid,
			BidSize:      item.BidQty,
			Close:        item.Last,
			Volume:       item.Volume,
			Low:          item.Low,
			High:         item.High,
			Open:         item.Last - item.Change,
			AssetType:    asset.Spot,
			Pair:         pair,
			LastUpdated:  item.Timestamp,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Exchange) wsProcessTrades(ctx context.Context, data []json.RawMessage) error {
	saveTradeData := e.IsSaveTradeDataEnabled()
	tradeFeed := e.IsTradeFeedEnabled()
	if !saveTradeData && !tradeFeed {
		return nil
	}

	trades := make([]trade.Data, 0, len(data))
	for i := range data {
		var item websocketTrade
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling trade data: %w", err)
		}
		pair, err := pairFromExchange(item.Symbol)
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

func (e *Exchange) wsProcessCandles(ctx context.Context, data []json.RawMessage) error {
	for i := range data {
		var item websocketCandle
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling candle data: %w", err)
		}
		pair, err := pairFromExchange(item.Symbol)
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

func (e *Exchange) wsProcessOrderbooks(ctx context.Context, messageType string, data []json.RawMessage) error {
	for i := range data {
		var item websocketBook
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling orderbook data: %w", err)
		}
		pair, err := pairFromExchange(item.Symbol)
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
			err = e.wsProcessOrderbookSnapshot(pair, sub.Levels, &item)
		case "update":
			err = e.wsProcessOrderbookUpdate(pair, &item)
		default:
			return fmt.Errorf("unsupported orderbook message type %q", messageType)
		}
		if errors.Is(err, errInvalidChecksum) {
			log.Debugf(log.Global, "%s resubscribing to invalid %s orderbook", e.Name, pair)
			go func() {
				if resubErr := e.Websocket.ResubscribeToChannel(ctx, e.Websocket.Conn, sub); resubErr != nil && !errors.Is(resubErr, subscription.ErrInStateAlready) {
					if sub.State() == subscription.ResubscribingState {
						_ = sub.SetState(subscription.SubscribedState)
					}
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
	return e.Websocket.GetSubscription(pairChannelKey{Subscription: &subscription.Subscription{
		Channel: channel,
		Asset:   asset.Spot,
		Pairs:   currency.Pairs{pair},
	}})
}

func (e *Exchange) wsProcessOrderbookSnapshot(pair currency.Pair, levels int, snapshot *websocketBook) error {
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
			Price:     snapshot.Asks[i].Price.Float64(),
			Amount:    snapshot.Asks[i].Quantity.Float64(),
			StrPrice:  snapshot.Asks[i].Price.String(),
			StrAmount: snapshot.Asks[i].Quantity.String(),
		}
	}
	for i := range snapshot.Bids {
		book.Bids[i] = orderbook.Level{
			Price:     snapshot.Bids[i].Price.Float64(),
			Amount:    snapshot.Bids[i].Quantity.Float64(),
			StrPrice:  snapshot.Bids[i].Price.String(),
			StrAmount: snapshot.Bids[i].Quantity.String(),
		}
	}
	if err := e.Websocket.Orderbook.LoadSnapshot(&book); err != nil {
		return err
	}
	err := e.wsValidateOrderbookChecksum(pair, snapshot.Checksum)
	if errors.Is(err, errInvalidChecksum) {
		err = common.AppendError(err, e.Websocket.Orderbook.InvalidateOrderbook(pair, asset.Spot))
	}
	return err
}

func (e *Exchange) wsProcessOrderbookUpdate(pair currency.Pair, update *websocketBook) error {
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
			Price:     update.Asks[i].Price.Float64(),
			Amount:    update.Asks[i].Quantity.Float64(),
			StrPrice:  update.Asks[i].Price.String(),
			StrAmount: update.Asks[i].Quantity.String(),
		}
	}
	for i := range update.Bids {
		bookUpdate.Bids[i] = orderbook.Level{
			Price:     update.Bids[i].Price.Float64(),
			Amount:    update.Bids[i].Quantity.Float64(),
			StrPrice:  update.Bids[i].Price.String(),
			StrAmount: update.Bids[i].Quantity.String(),
		}
	}
	if err := e.Websocket.Orderbook.Update(&bookUpdate); err != nil {
		return err
	}
	err := e.wsValidateOrderbookChecksum(pair, update.Checksum)
	if errors.Is(err, errInvalidChecksum) {
		err = common.AppendError(err, e.Websocket.Orderbook.InvalidateOrderbook(pair, asset.Spot))
	}
	return err
}

func (e *Exchange) wsProcessExecutions(ctx context.Context, data []json.RawMessage) error {
	for i := range data {
		var item WebsocketExecution
		if err := json.Unmarshal(data[i], &item); err != nil {
			return fmt.Errorf("error unmarshalling execution data: %w", err)
		}
		detail := &order.Detail{
			Exchange:             e.Name,
			OrderID:              item.OrderID,
			ClientOrderID:        item.ClientOrderID,
			Amount:               item.OrderQty,
			ExecutedAmount:       item.CumulativeQty,
			AverageExecutedPrice: item.AveragePrice,
			Cost:                 item.CumulativeCost,
			Price:                item.LimitPrice,
			LastUpdated:          item.Timestamp,
			AssetType:            asset.Spot,
			ReduceOnly:           item.ReduceOnly,
		}
		if item.ExecutionType == "new" {
			detail.Date = item.Timestamp
		}
		if item.OrderQty >= item.CumulativeQty {
			detail.RemainingAmount = item.OrderQty - item.CumulativeQty
		}
		var err error
		if item.Symbol != "" {
			detail.Pair, err = pairFromExchange(item.Symbol)
			if err != nil {
				return fmt.Errorf("error parsing execution symbol %q: %w", item.Symbol, err)
			}
			detail.CostAsset = detail.Pair.Quote
		}
		if item.Side != "" {
			detail.Side, err = order.StringToOrderSide(item.Side)
			if err != nil {
				return err
			}
		}
		if item.OrderType != "" {
			detail.Type = wsOrderType(item.OrderType)
		}
		if item.OrderStatus != "" {
			detail.Status, err = wsOrderStatus(item.OrderStatus)
			if err != nil {
				return err
			}
		}
		if item.TimeInForce != "" {
			detail.TimeInForce, err = order.StringToTimeInForce(item.TimeInForce)
			if err != nil {
				return err
			}
		}
		for j := range item.Fees {
			feeAsset := currencyFromExchange(currency.NewCode(item.Fees[j].Asset))
			if !detail.FeeAsset.IsEmpty() && !feeAsset.IsEmpty() && !detail.FeeAsset.Equal(feeAsset) {
				return fmt.Errorf("execution fees use multiple assets: %s and %s", detail.FeeAsset, feeAsset)
			}
			detail.Fee += item.Fees[j].Quantity
			if !feeAsset.IsEmpty() {
				detail.FeeAsset = feeAsset
			}
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
				FeeAsset:  detail.FeeAsset.String(),
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

func wsOrderType(value string) order.Type {
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

func krakenOrderTypeName(value order.Type) (string, error) {
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

func wsOrderStatus(value string) (order.Status, error) {
	if value == "pending_new" {
		return order.Pending, nil
	}
	return order.StringToOrderStatus(value)
}

// startWsPingHandler sets up a websocket ping handler to maintain a connection
func (e *Exchange) startWsPingHandler(conn websocket.Connection) {
	conn.SetupPingHandler(request.Unset, websocket.PingHandler{
		Message:     []byte(`{"method":"ping"}`),
		Delay:       wsPingDelay,
		MessageType: gws.TextMessage,
	})
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

func (e *Exchange) wsValidateOrderbookChecksum(pair currency.Pair, checksum uint32) error {
	book, err := e.Websocket.Orderbook.GetOrderbook(pair, asset.Spot)
	if err != nil {
		return fmt.Errorf("cannot retrieve orderbook for checksum validation: %w", err)
	}
	return validateCRC32(book, checksum)
}

// trim removes '.' and prefixed '0' from subsequent string
func trim(s string) string {
	s = strings.Replace(s, ".", "", 1)
	s = strings.TrimLeft(s, "0")
	return s
}

// GetSubscriptionTemplate returns a subscription channel template
func (e *Exchange) GetSubscriptionTemplate(_ *subscription.Subscription) (*template.Template, error) {
	return template.New("master.tmpl").Funcs(template.FuncMap{"channelName": channelName}).Parse(subTplText)
}

// ValidateSubscriptions implements subscription.ListValidator.
// Spot v2 book messages identify the symbol but not the requested depth, so
// simultaneous depths for one pair cannot be routed without corrupting a book.
func (*Exchange) ValidateSubscriptions(subs subscription.List) error {
	depthByPair := make(map[string]int)
	for _, sub := range subs {
		if sub == nil || sub.Channel != subscription.OrderbookChannel || sub.Asset != asset.Spot {
			continue
		}
		depth := sub.Levels
		if depth == 0 {
			depth = wsDefaultBookDepth
		}
		for i := range sub.Pairs {
			pair := pairToExchange(sub.Pairs[i]).Format(currency.PairFormat{Uppercase: true, Delimiter: "/"})
			existingDepth, exists := depthByPair[pair.String()]
			if exists && existingDepth != depth {
				return fmt.Errorf("%w for %q between depths %d and %d", subscription.ErrExclusiveSubscription, pair, existingDepth, depth)
			}
			depthByPair[pair.String()] = depth
		}
	}
	return nil
}

func applyBookDepthDefaults(subs subscription.List) subscription.List {
	normalised := slices.Clone(subs)
	for i, sub := range normalised {
		if sub != nil && sub.Channel == subscription.OrderbookChannel && sub.Asset == asset.Spot && sub.Levels == 0 {
			normalised[i] = sub.Clone()
			normalised[i].Levels = wsDefaultBookDepth
		}
	}
	return normalised
}

func (e *Exchange) generateSubscriptions() (subscription.List, error) {
	subs, err := e.Features.Subscriptions.ExpandTemplates(e)
	subs = applyBookDepthDefaults(subs)
	return subs, err
}

func (e *Exchange) subscriptionConnection(s *subscription.Subscription) websocket.Connection {
	if s != nil && s.Authenticated {
		return e.Websocket.AuthConn
	}
	return e.Websocket.Conn
}

// Subscribe adds a channel subscription to the websocket
func (e *Exchange) Subscribe(in subscription.List) error {
	ctx := context.TODO()
	in, errs := in.ExpandTemplates(e)
	in = applyBookDepthDefaults(in)

	// Collect valid new subs and add to websocket in Subscribing state
	subs := subscription.List{}
	for _, s := range in {
		if s.State() != subscription.ResubscribingState {
			if err := e.Websocket.AddSubscriptions(e.subscriptionConnection(s), s); err != nil {
				errs = common.AppendError(errs, fmt.Errorf("%w; Channel: %s Pairs: %s", err, s.Channel, s.Pairs.Join()))
				continue
			}
		}
		subs = append(subs, s)
	}

	// Merge subs by grouping pairs for request; We make a single request to subscribe to N+ pairs, but get N+ responses back
	groupedSubs := subs.GroupPairs()

	errs = common.AppendError(errs,
		e.ParallelChanOp(ctx, groupedSubs, func(ctx context.Context, s subscription.List) error { return e.manageSubs(ctx, wsSubscribe, s) }, 1),
	)

	for _, s := range subs {
		if s.State() != subscription.SubscribedState {
			_ = s.SetState(subscription.InactiveState)
			if err := e.Websocket.RemoveSubscriptions(e.subscriptionConnection(s), s); err != nil {
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
	keys = applyBookDepthDefaults(keys)
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
		e.ParallelChanOp(ctx, subs, func(ctx context.Context, s subscription.List) error { return e.manageSubs(ctx, wsUnsubscribe, s) }, 1),
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
	r := &WebsocketRequest[WebsocketSubscriptionParams]{
		Method:    op,
		RequestID: e.MessageSequence(),
		Params: WebsocketSubscriptionParams{
			Channel: s.QualifiedChannel,
			Depth:   s.Levels,
			Symbols: pairsToExchange(s.Pairs).Format(reqFmt).Strings(),
		},
	}

	if s.Interval != 0 {
		r.Params.Interval = int(time.Duration(s.Interval).Minutes())
	}

	conn := e.subscriptionConnection(s)
	if s.Authenticated {
		r.Params.Token = e.websocketAuthToken()
		if op == wsSubscribe && s.QualifiedChannel == wsExecutions {
			r.Params.SnapOrders = true
			r.Params.SnapTrades = true
		}
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
		expectedPair := pairToExchange(p)
		expectedPair.Base = currencyFromExchange(expectedPair.Base)
		expectedPair.Quote = currencyFromExchange(expectedPair.Quote)
		expectedPair.Delimiter = ""
		pairErrs[expectedPair] = errSubPairMissing
	}

	subPairs := currency.Pairs{}
	for _, resp := range resps {
		var response websocketResponse
		if err := json.Unmarshal(resp, &response); err != nil {
			return fmt.Errorf("%w: parsing WS response from message %s: %w", common.ErrInvalidResponse, resp, err)
		}
		pName := response.Result.Symbol
		if pName == "" {
			pName = response.Symbol
		}
		if pName == "" {
			return fmt.Errorf("%w: %w parsing WS symbol from message: %s", common.ErrInvalidResponse, errSubPairMissing, resp)
		}
		pair, err := pairFromExchange(pName)
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
			if e.Verbose && op == wsSubscribe {
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
		errs = common.AppendError(errs, fmt.Errorf("%w; Channel: %s Pairs: %s", err, s.Channel, pairs.Format(reqFmt).Join()))
	}

	if e.Verbose && len(subPairs) > 0 {
		log.Debugf(log.ExchangeSys, "%s Subscribed to Channel: %s Pairs: %s", e.Name, s.Channel, subPairs.Format(reqFmt).Join())
	}

	return errs
}

// getSubRespErr parses a response and ensures its method matches the subscription operation.
func (e *Exchange) getSubRespErr(resp []byte, op string) error {
	response, err := parseWebsocketResponse(resp)
	if err != nil {
		return err
	}
	if response.Method != op {
		return fmt.Errorf("wrong WS method: %s; expected: %s from message %s", response.Method, op, resp)
	}
	return nil
}

func parseWebsocketResponse(resp []byte) (websocketResponse, error) {
	var response websocketResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return response, fmt.Errorf("%w: error parsing WS response: %w from message: %s", common.ErrInvalidResponse, err, resp)
	}
	if response.Success == nil {
		return response, fmt.Errorf("%w: error parsing WS success from message: %s", common.ErrInvalidResponse, resp)
	}
	if *response.Success {
		return response, nil
	}
	if response.Error == "" {
		return response, fmt.Errorf("%w: %w: error response did not contain an error: %s", common.ErrInvalidResponse, common.ErrUnknownError, resp)
	}
	return response, errors.New(response.Error)
}

// wsProcessSubStatus handles creating or removing Subscriptions as soon as we receive a message
// It's job is to ensure that subscription state is kept correct sequentially between WS messages
// If this responsibility was moved to Subscribe then we would have a race due to the channel connecting IncomingWithData
func (e *Exchange) wsProcessSubStatus(resp []byte) {
	var response websocketResponse
	if err := json.Unmarshal(resp, &response); err != nil || response.Result.Channel == "" {
		return
	}
	if _, err := parseWebsocketResponse(resp); err != nil {
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
		pair, err := pairFromExchange(response.Result.Symbol)
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
	if response.Method == wsSubscribe {
		err = s.SetState(subscription.SubscribedState)
	} else if s.State() != subscription.ResubscribingState { // Do not remove a resubscribing sub which just unsubbed
		err = e.Websocket.RemoveSubscriptions(e.subscriptionConnection(s), s)
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
		return fmt.Errorf("%w: use subscription.MyAccountChannel for executions", subscription.ErrNotSupported)
	}
	name := strings.Split(s.Channel, "-") // Protect against attempted usage of book-N as a channel name
	if n, ok := reverseChannelNames[name[0]]; ok && n != s.Channel {
		return fmt.Errorf("%w: %s => subscription.%s%sChannel", subscription.ErrUseConstChannelName, s.Channel, bytes.ToUpper([]byte{n[0]}), n[1:])
	}
	return nil
}

// wsAddOrder creates an order and returns its order ID.
func (e *Exchange) wsAddOrder(ctx context.Context, params *WebsocketAddOrderParams) (string, error) {
	if params == nil {
		return "", common.ErrNilPointer
	}
	requestParams := *params
	pair, err := currency.NewPairDelimiter(requestParams.Symbol, "/")
	if err != nil {
		return "", fmt.Errorf("invalid add_order symbol %q: %w", requestParams.Symbol, err)
	}
	requestParams.Symbol = pairToExchange(pair).Format(currency.PairFormat{Uppercase: true, Delimiter: "/"}).String()
	requestParams.Token = e.websocketAuthToken()
	req := WebsocketRequest[WebsocketAddOrderParams]{
		Method:    wsAddOrder,
		Params:    requestParams,
		RequestID: e.MessageSequence(),
	}
	jsonResp, err := e.Websocket.AuthConn.SendMessageReturnResponse(ctx, request.Unset, req.RequestID, &req)
	if err != nil {
		return "", err
	}
	resp, err := parseWebsocketResponse(jsonResp)
	if err != nil {
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

type cancelOrderResponseInspector struct{}

func (cancelOrderResponseInspector) IsFinal(response []byte) bool {
	var resp websocketResponse
	return json.Unmarshal(response, &resp) != nil || resp.Result.OrderID == ""
}

// wsCancelOrders cancels one or more open orders in a single request.
func (e *Exchange) wsCancelOrders(ctx context.Context, orderIDs []string) error {
	if len(orderIDs) == 0 {
		return nil
	}
	req := WebsocketRequest[WebsocketCancelOrderParams]{
		Method: wsCancelOrder,
		Params: WebsocketCancelOrderParams{
			OrderIDs: orderIDs,
			Token:    e.websocketAuthToken(),
		},
		RequestID: e.MessageSequence(),
	}
	responses, err := e.Websocket.AuthConn.SendMessageReturnResponsesWithInspector(ctx, request.Unset, req.RequestID, &req, len(orderIDs), cancelOrderResponseInspector{})
	if err != nil {
		return fmt.Errorf("%w: %w", errCancellingOrder, err)
	}
	acknowledged := make(map[string]int, len(orderIDs))
	requested := make(map[string]int, len(orderIDs))
	for i := range orderIDs {
		requested[orderIDs[i]]++
	}
	var responseErrs error
	correlatable := true
	for i := range responses {
		resp, err := parseWebsocketResponse(responses[i])
		if err != nil {
			if errors.Is(err, common.ErrInvalidResponse) {
				correlatable = false
			}
			responseErrs = common.AppendError(responseErrs, fmt.Errorf("response %q: %w", responses[i], err))
			continue
		}
		if resp.Result.OrderID == "" || acknowledged[resp.Result.OrderID] >= requested[resp.Result.OrderID] {
			correlatable = false
			responseErrs = common.AppendError(responseErrs, fmt.Errorf("response %q: %w", responses[i], common.ErrInvalidResponse))
			continue
		}
		acknowledged[resp.Result.OrderID]++
	}
	if responseErrs == nil {
		return nil
	}
	if !correlatable {
		return fmt.Errorf("%w: %w", errCancellingOrder, responseErrs)
	}
	unacknowledged := make([]string, 0, len(orderIDs))
	for i := range orderIDs {
		if acknowledged[orderIDs[i]] > 0 {
			acknowledged[orderIDs[i]]--
			continue
		}
		unacknowledged = append(unacknowledged, orderIDs[i])
	}
	if len(unacknowledged) == 0 {
		return fmt.Errorf("%w: %w", errCancellingOrder, responseErrs)
	}
	return fmt.Errorf("%w %s: %w", errCancellingOrder, strings.Join(unacknowledged, ", "), responseErrs)
}

// wsCancelAllOrders cancels all open orders and returns the affected order count.
func (e *Exchange) wsCancelAllOrders(ctx context.Context) (int64, error) {
	req := WebsocketRequest[WebsocketCancelAllParams]{
		Method:    wsCancelAll,
		Params:    WebsocketCancelAllParams{Token: e.websocketAuthToken()},
		RequestID: e.MessageSequence(),
	}

	jsonResp, err := e.Websocket.AuthConn.SendMessageReturnResponse(ctx, request.Unset, req.RequestID, &req)
	if err != nil {
		return 0, err
	}
	resp, err := parseWebsocketResponse(jsonResp)
	if err != nil {
		return 0, err
	}
	return resp.Result.Count, nil
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
