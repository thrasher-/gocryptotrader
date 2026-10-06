package hyperliquid

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fill"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	"github.com/thrasher-corp/gocryptotrader/log"
)

const (
	// websocketPingInterval keeps quiet connections inside the server's 60 second idle timeout
	websocketPingInterval = 25 * time.Second
	// websocketMessageInterval spaces requests within the limit of 2000 messages a minute
	websocketMessageInterval = 30 * time.Millisecond
	// maximumWebsocketSubscriptions reserves two of the 1000 subscriptions allowed per IP for the account feeds
	maximumWebsocketSubscriptions = 998
	// maximumFastAssetContextsSize bounds a decompressed fastAssetCtxs message; a full snapshot is about 82 KB
	maximumFastAssetContextsSize = 16 << 20

	wsMethodSubscribe   = "subscribe"
	wsMethodUnsubscribe = "unsubscribe"
	wsMethodPost        = "post"
	wsPostTypeInfo      = "info"
	wsPostTypeError     = "error"

	// Subscription types, which name their data channels except userEvents
	wsChannelAllMids                     = "allMids"
	wsChannelNotification                = "notification"
	wsChannelWebData3                    = "webData3"
	wsChannelTWAPStates                  = "twapStates"
	wsChannelClearinghouseState          = "clearinghouseState"
	wsChannelOpenOrders                  = "openOrders"
	wsChannelCandle                      = "candle"
	wsChannelOrderbook                   = "l2Book"
	wsChannelTrades                      = "trades"
	wsChannelOrderUpdates                = "orderUpdates"
	wsSubscriptionUserEvents             = "userEvents"
	wsChannelUserFills                   = "userFills"
	wsChannelUserFundings                = "userFundings"
	wsChannelUserNonFundingLedgerUpdates = "userNonFundingLedgerUpdates"
	wsChannelActiveAssetContext          = "activeAssetCtx"
	wsChannelActiveAssetData             = "activeAssetData"
	wsChannelUserTWAPSliceFills          = "userTwapSliceFills"
	wsChannelUserTWAPHistory             = "userTwapHistory"
	wsChannelBestBidOffer                = "bbo"
	wsChannelSpotState                   = "spotState"
	wsChannelAllDEXsClearinghouseState   = "allDexsClearinghouseState"
	wsChannelAllDEXsAssetContexts        = "allDexsAssetCtxs"
	wsChannelOutcomeMetaUpdates          = "outcomeMetaUpdates"
	wsChannelFastAssetContexts           = "fastAssetCtxs"

	// Data channels that do not share their subscription's name
	wsChannelUserEvents             = "user"
	wsChannelActiveSpotAssetContext = "activeSpotAssetCtx"

	wsChannelSubscriptionResponse = "subscriptionResponse"
	wsChannelPost                 = "post"
	wsChannelPong                 = "pong"
	wsChannelError                = "error"
	// websocketConnectionEstablished is a plain text greeting the server once sent on connection
	websocketConnectionEstablished = "Websocket connection established."

	// Error message prefixes that identify the request they answer
	wsErrorAlreadySubscribed   = "Already subscribed: "
	wsErrorAlreadyUnsubscribed = "Already unsubscribed: "
	wsErrorInvalidSubscription = "Invalid subscription "
	wsErrorParseFailure        = "Error parsing JSON into valid websocket request: "
)

var (
	errWebsocketAssetMismatch = errors.New("websocket market asset mismatch")
	errWebsocketParameter     = errors.New("invalid websocket subscription parameter")
	errWebsocketPost          = errors.New("websocket post request failed")
	errWebsocketServer        = errors.New("websocket server error")
	errWebsocketSubscription  = errors.New("websocket subscription acknowledgement error")
)

var defaultSubscriptions = subscription.List{
	{Enabled: true, Asset: asset.All, Channel: subscription.TickerChannel},
	{Enabled: true, Asset: asset.All, Channel: subscription.OrderbookChannel},
	{Enabled: true, Asset: asset.All, Channel: subscription.AllTradesChannel},
	{Enabled: true, Asset: asset.All, Channel: subscription.CandlesChannel, Interval: kline.OneMin},
	{Enabled: true, Channel: subscription.MyOrdersChannel, Authenticated: true},
	{Enabled: true, Channel: subscription.MyTradesChannel, Authenticated: true},
}

// websocketFeed describes the parameters a Hyperliquid subscription type takes
type websocketFeed struct {
	// coin feeds are per market, taking the subscription's single pair and its asset
	coin bool
	// user feeds are per account, taking the configured account address, and use the authenticated connection
	user bool
	// dex feeds take an optional perpetual DEX name from the subscription's dex parameter
	dex bool
}

// websocketFeeds lists every Hyperliquid subscription type, which a subscription may name as its channel
var websocketFeeds = map[string]websocketFeed{
	wsChannelAllMids:                     {dex: true},
	wsChannelNotification:                {user: true},
	wsChannelWebData3:                    {user: true},
	wsChannelTWAPStates:                  {user: true, dex: true},
	wsChannelClearinghouseState:          {user: true, dex: true},
	wsChannelOpenOrders:                  {user: true, dex: true},
	wsChannelCandle:                      {coin: true},
	wsChannelOrderbook:                   {coin: true},
	wsChannelTrades:                      {coin: true},
	wsChannelOrderUpdates:                {user: true},
	wsSubscriptionUserEvents:             {user: true},
	wsChannelUserFills:                   {user: true},
	wsChannelUserFundings:                {user: true},
	wsChannelUserNonFundingLedgerUpdates: {user: true},
	wsChannelActiveAssetContext:          {coin: true},
	wsChannelActiveAssetData:             {coin: true, user: true},
	wsChannelUserTWAPSliceFills:          {user: true},
	wsChannelUserTWAPHistory:             {user: true},
	wsChannelBestBidOffer:                {coin: true},
	wsChannelSpotState:                   {user: true},
	wsChannelAllDEXsClearinghouseState:   {user: true},
	wsChannelAllDEXsAssetContexts:        {},
	wsChannelOutcomeMetaUpdates:          {},
	wsChannelFastAssetContexts:           {},
}

// websocketStandardChannels maps standard channels to the Hyperliquid subscription types that serve them; userEvents
// serves myTrades because it carries TWAP slice fills, which userFills omits, and sends no snapshot to replay
var websocketStandardChannels = map[string]string{
	subscription.TickerChannel:    wsChannelActiveAssetContext,
	subscription.OrderbookChannel: wsChannelOrderbook,
	subscription.AllTradesChannel: wsChannelTrades,
	subscription.CandlesChannel:   wsChannelCandle,
	subscription.MyOrdersChannel:  wsChannelOrderUpdates,
	subscription.MyTradesChannel:  wsSubscriptionUserEvents,
}

// WsConnect connects the public stream and, when an account address is configured, a stream for its account feeds
func (e *Exchange) WsConnect() error {
	ctx := context.TODO()
	if !e.Websocket.IsEnabled() || !e.IsEnabled() {
		return websocket.ErrWebsocketNotEnabled
	}
	if err := e.connectWebsocket(ctx, e.Websocket.Conn); err != nil {
		return err
	}
	e.Websocket.Wg.Add(1)
	go e.wsReadData(ctx, e.Websocket.Conn, false)

	if !e.IsWebsocketAuthenticationSupported() {
		return nil
	}
	if _, err := e.getWatchAddress(ctx); err != nil {
		e.Websocket.SetCanUseAuthenticatedEndpoints(false)
		log.Warnf(log.WebsocketMgr, "%s account websocket feeds disabled: %s", e.Name, err)
		return nil
	}
	if err := e.connectWebsocket(ctx, e.Websocket.AuthConn); err != nil {
		e.Websocket.SetCanUseAuthenticatedEndpoints(false)
		log.Warnf(log.WebsocketMgr, "%s account websocket connection failed: %s", e.Name, err)
		return nil
	}
	// Hyperliquid's account feeds are public and keyed by address, so authenticated here only means an address is set
	e.Websocket.SetCanUseAuthenticatedEndpoints(true)
	e.Websocket.Wg.Add(1)
	go e.wsReadData(ctx, e.Websocket.AuthConn, true)
	return nil
}

func (e *Exchange) connectWebsocket(ctx context.Context, connection websocket.Connection) error {
	if connection == nil {
		return common.ErrNilPointer
	}
	if err := connection.Dial(ctx, &gws.Dialer{}, http.Header{}, nil); err != nil {
		return err
	}
	connection.SetupPingHandler(request.Unset, websocket.PingHandler{
		MessageType: gws.TextMessage,
		Message:     []byte(`{"method":"ping"}`),
		Delay:       websocketPingInterval,
	})
	return nil
}

// wsReadData relays a connection's messages until it closes, then fails its outstanding subscription operations
func (e *Exchange) wsReadData(ctx context.Context, connection websocket.Connection, authenticated bool) {
	defer e.Websocket.Wg.Done()
	for {
		message := connection.ReadMessage()
		if message.Raw == nil {
			_ = e.failWebsocketPending(authenticated, websocket.ErrNotConnected)
			return
		}
		if err := e.wsHandleData(ctx, message.Raw, authenticated); err != nil {
			if sendErr := e.Websocket.DataHandler.Send(ctx, err); sendErr != nil {
				log.Errorf(log.WebsocketMgr, "%s websocket data handler: %s; source error: %s", e.Name, sendErr, err)
			}
		}
	}
}

// wsHandleData handles a message, settling subscription operations on the connection that sent them
func (e *Exchange) wsHandleData(ctx context.Context, raw []byte, authenticated bool) error {
	if string(raw) == websocketConnectionEstablished {
		return nil
	}
	var envelope WsResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	switch envelope.Channel {
	case wsChannelSubscriptionResponse:
		return e.wsHandleSubscriptionResponse(envelope.Data, authenticated)
	case wsChannelPost:
		return e.wsHandlePostResponse(envelope.Data)
	case wsChannelPong:
		return nil
	case wsChannelError:
		return e.wsHandleError(envelope.Data, authenticated)
	case wsChannelActiveAssetContext:
		return e.wsHandlePerpetualTicker(ctx, envelope.Data)
	case wsChannelActiveSpotAssetContext:
		return e.wsHandleSpotTicker(ctx, envelope.Data)
	case wsChannelOrderbook:
		return e.wsHandleOrderbook(ctx, envelope.Data)
	case wsChannelTrades:
		return e.wsHandleTrades(envelope.Data)
	case wsChannelCandle:
		return e.wsHandleCandle(ctx, envelope.Data)
	case wsChannelOrderUpdates:
		return e.wsHandleOrderUpdates(ctx, envelope.Data)
	case wsChannelUserEvents:
		return e.wsHandleUserEvent(ctx, envelope.Data)
	case wsChannelUserFills:
		return e.wsHandleUserFills(envelope.Data)
	case wsChannelUserTWAPSliceFills:
		return e.wsHandleUserTWAPSliceFills(envelope.Data)
	case wsChannelUserFundings:
		return e.wsHandleUserFundings(ctx, envelope.Data)
	case wsChannelUserNonFundingLedgerUpdates:
		return e.wsHandleUserNonFundingLedgerUpdates(ctx, envelope.Data)
	case wsChannelUserTWAPHistory:
		return e.wsHandleUserTWAPHistory(ctx, envelope.Data)
	case wsChannelOutcomeMetaUpdates:
		return e.wsHandleOutcomeMetaUpdates(ctx, envelope.Data)
	case wsChannelFastAssetContexts:
		return e.wsHandleFastAssetContexts(ctx, envelope.Data)
	case wsChannelAllMids:
		return wsRelay[WsAllMids](ctx, e, envelope.Data)
	case wsChannelNotification:
		return wsRelay[WsNotification](ctx, e, envelope.Data)
	case wsChannelWebData3:
		return wsRelay[WsWebData3](ctx, e, envelope.Data)
	case wsChannelTWAPStates:
		return wsRelay[WsTWAPStates](ctx, e, envelope.Data)
	case wsChannelClearinghouseState:
		return wsRelay[WsClearinghouseState](ctx, e, envelope.Data)
	case wsChannelOpenOrders:
		return wsRelay[WsOpenOrders](ctx, e, envelope.Data)
	case wsChannelActiveAssetData:
		return wsRelay[ActiveAssetDataResponse](ctx, e, envelope.Data)
	case wsChannelBestBidOffer:
		return wsRelay[WsBestBidOffer](ctx, e, envelope.Data)
	case wsChannelSpotState:
		return wsRelay[WsSpotState](ctx, e, envelope.Data)
	case wsChannelAllDEXsClearinghouseState:
		return wsRelay[WsAllDEXsClearinghouseState](ctx, e, envelope.Data)
	case wsChannelAllDEXsAssetContexts:
		return wsRelay[WsAllDEXsAssetContexts](ctx, e, envelope.Data)
	default:
		return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{
			Message: e.Name + websocket.UnhandledMessage + string(raw),
		})
	}
}

// wsRelay decodes a message into its exported type and relays it
func wsRelay[T any](ctx context.Context, e *Exchange, raw []byte) error {
	update := new(T)
	if err := json.Unmarshal(raw, update); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, update)
}

// wsHandleSubscriptionResponse completes the subscription operation an acknowledgement matches
func (e *Exchange) wsHandleSubscriptionResponse(raw []byte, authenticated bool) error {
	var response WsSubscriptionResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return fmt.Errorf("%w: %w", errWebsocketSubscription, err)
	}
	if (response.Method != wsMethodSubscribe && response.Method != wsMethodUnsubscribe) || strings.TrimSpace(response.Subscription.Type) == "" {
		return fmt.Errorf("%w: invalid response method %q or subscription", errWebsocketSubscription, response.Method)
	}
	_, err := e.completeWebsocketPending(authenticated, response.Method, &response.Subscription)
	return err
}

// completeWebsocketPending applies the server's completion of a pending operation to the subscription store; it reports
// whether an operation was pending, as a late or unknown completion is ignored
func (e *Exchange) completeWebsocketPending(authenticated bool, method string, sub *WsSubscription) (bool, error) {
	key := websocketPendingKey{authenticated: authenticated, subscription: *sub}
	e.websocketPendingMu.Lock()
	pending := e.websocketPending[key]
	if pending == nil {
		e.websocketPendingMu.Unlock()
		return false, nil
	}
	if pending.method != method {
		e.websocketPendingMu.Unlock()
		return true, fmt.Errorf("%w: expected %s, got %s", errWebsocketSubscription, pending.method, method)
	}
	delete(e.websocketPending, key)
	e.websocketPendingMu.Unlock()

	var err error
	switch method {
	case wsMethodSubscribe:
		err = pending.subscription.SetState(subscription.SubscribedState)
	case wsMethodUnsubscribe:
		err = e.Websocket.RemoveSubscriptions(pending.connection, pending.subscription)
	}
	if err != nil {
		err = common.AppendError(err, e.rollbackWebsocketPending(pending))
	}
	pending.done <- err
	return true, err
}

// failWebsocketPendingOperation fails and rolls back one pending operation; it reports whether the operation was pending
func (e *Exchange) failWebsocketPendingOperation(authenticated bool, method string, sub *WsSubscription, cause error) bool {
	key := websocketPendingKey{authenticated: authenticated, subscription: *sub}
	e.websocketPendingMu.Lock()
	pending := e.websocketPending[key]
	if pending == nil || pending.method != method {
		e.websocketPendingMu.Unlock()
		return false
	}
	delete(e.websocketPending, key)
	e.websocketPendingMu.Unlock()
	pending.done <- common.AppendError(cause, e.rollbackWebsocketPending(pending))
	return true
}

// rollbackWebsocketPending restores the local subscription state of an operation the server did not complete
func (e *Exchange) rollbackWebsocketPending(pending *websocketPendingOperation) error {
	if pending == nil || pending.connection == nil || pending.subscription == nil {
		return common.ErrNilPointer
	}
	switch pending.method {
	case wsMethodSubscribe:
		return e.Websocket.RemoveSubscriptions(pending.connection, pending.subscription)
	case wsMethodUnsubscribe:
		if pending.subscription.State() == pending.previousState {
			return nil
		}
		return pending.subscription.SetState(pending.previousState)
	default:
		return fmt.Errorf("%w: %s", common.ErrNotYetImplemented, pending.method)
	}
}

// failWebsocketPending fails and rolls back every operation outstanding on a connection
func (e *Exchange) failWebsocketPending(authenticated bool, cause error) error {
	e.websocketPendingMu.Lock()
	pending := make([]*websocketPendingOperation, 0, len(e.websocketPending))
	for key, operation := range e.websocketPending {
		if key.authenticated == authenticated {
			delete(e.websocketPending, key)
			pending = append(pending, operation)
		}
	}
	e.websocketPendingMu.Unlock()

	result := cause
	for i := range pending {
		rollbackErr := e.rollbackWebsocketPending(pending[i])
		pending[i].done <- common.AppendError(cause, rollbackErr)
		result = common.AppendError(result, rollbackErr)
	}
	return result
}

// abortWebsocketPending fails an operation that could not be sent or timed out; if the acknowledgement handler already
// took the operation, its outcome is returned instead
func (e *Exchange) abortWebsocketPending(key *websocketPendingKey, pending *websocketPendingOperation, cause error) error {
	e.websocketPendingMu.Lock()
	if e.websocketPending[*key] != pending {
		e.websocketPendingMu.Unlock()
		return <-pending.done
	}
	delete(e.websocketPending, *key)
	e.websocketPendingMu.Unlock()
	return common.AppendError(cause, e.rollbackWebsocketPending(pending))
}

// wsHandleError settles the operation an error message answers, which the message identifies by quoting its
// subscription or request; any other error fails every operation pending on the connection
func (e *Exchange) wsHandleError(raw []byte, authenticated bool) error {
	var message string
	if err := json.Unmarshal(raw, &message); err != nil {
		message = strings.TrimSpace(string(raw))
	}
	if message == "" || message == "null" {
		message = "unspecified error"
	}
	cause := fmt.Errorf("%w: %s", errWebsocketServer, message)
	if quoted, ok := strings.CutPrefix(message, wsErrorAlreadySubscribed); ok {
		// The feed is active, so the subscribe succeeded, but without replaying recent trades
		if sub, err := decodeWebsocketSubscription(quoted); err == nil {
			if settled, err := e.completeWebsocketPending(authenticated, wsMethodSubscribe, &sub); settled {
				if sub.Type == wsChannelTrades {
					e.clearTradeReplay(sub.Coin)
				}
				return err
			}
		}
		return cause
	}
	if quoted, ok := strings.CutPrefix(message, wsErrorAlreadyUnsubscribed); ok {
		// The feed is inactive, so the unsubscribe succeeded
		if sub, err := decodeWebsocketSubscription(quoted); err == nil {
			if settled, err := e.completeWebsocketPending(authenticated, wsMethodUnsubscribe, &sub); settled {
				return err
			}
		}
		return cause
	}
	if quoted, ok := strings.CutPrefix(message, wsErrorInvalidSubscription); ok {
		if sub, err := decodeWebsocketSubscription(quoted); err == nil && e.failWebsocketPendingOperation(authenticated, wsMethodSubscribe, &sub, cause) {
			return cause
		}
		return e.failWebsocketPending(authenticated, cause)
	}
	if quoted, ok := strings.CutPrefix(message, wsErrorParseFailure); ok {
		var requestMethod struct {
			Method string `json:"method"`
			ID     uint64 `json:"id"`
		}
		if err := json.Unmarshal([]byte(quoted), &requestMethod); err == nil {
			switch requestMethod.Method {
			case wsMethodPost:
				response, err := json.Marshal(&WsPostResponse{ID: requestMethod.ID, Response: WsPostResponseBody{Type: wsPostTypeError, Payload: raw}})
				if err == nil && e.Websocket.Match.IncomingWithData(requestMethod.ID, response) {
					return nil
				}
				return cause
			case wsMethodSubscribe, wsMethodUnsubscribe:
				var subscriptionRequest WsSubscriptionRequest
				if json.Unmarshal([]byte(quoted), &subscriptionRequest) == nil && e.failWebsocketPendingOperation(authenticated, subscriptionRequest.Method, &subscriptionRequest.Subscription, cause) {
					return cause
				}
			}
		}
	}
	return e.failWebsocketPending(authenticated, cause)
}

// decodeWebsocketSubscription decodes the canonical subscription an error message quotes
func decodeWebsocketSubscription(quoted string) (WsSubscription, error) {
	var sub WsSubscription
	if err := json.Unmarshal([]byte(quoted), &sub); err != nil {
		return WsSubscription{}, err
	}
	return sub, nil
}

// wsHandlePostResponse delivers a post response to the request awaiting its ID
func (e *Exchange) wsHandlePostResponse(raw []byte) error {
	var response WsPostResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return err
	}
	return e.Websocket.Match.RequireMatchWithData(response.ID, raw)
}

func (e *Exchange) wsHandlePerpetualTicker(ctx context.Context, raw []byte) error {
	var update WsPerpetualAssetContext
	if err := json.Unmarshal(raw, &update); err != nil {
		return err
	}
	mapping, a, err := e.lookupPairMappingByCoin(update.Coin)
	if err != nil {
		return err
	}
	if a != asset.PerpetualContract {
		return fmt.Errorf("%w: expected %s, got %s for %s", errWebsocketAssetMismatch, asset.PerpetualContract, a, update.Coin)
	}
	price := e.perpetualTickerPrice(mapping.pair, &update.Context)
	if err := ticker.ProcessTicker(price); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, price)
}

func (e *Exchange) wsHandleSpotTicker(ctx context.Context, raw []byte) error {
	var update WsSpotAssetContext
	if err := json.Unmarshal(raw, &update); err != nil {
		return err
	}
	mapping, a, err := e.lookupPairMappingByCoin(update.Coin)
	if err != nil {
		return err
	}
	if a != asset.Spot {
		return fmt.Errorf("%w: expected %s, got %s for %s", errWebsocketAssetMismatch, asset.Spot, a, update.Coin)
	}
	price := e.spotTickerPrice(mapping.pair, &update.Context)
	if err := ticker.ProcessTicker(price); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, price)
}

// wsHandleOrderbook loads a book snapshot; the feed pushes full snapshots rather than incremental updates
func (e *Exchange) wsHandleOrderbook(ctx context.Context, raw []byte) error {
	var update WsL2Book
	if err := json.Unmarshal(raw, &update); err != nil {
		return err
	}
	mapping, a, err := e.lookupPairMappingByCoin(update.Coin)
	if err != nil {
		return err
	}
	book, err := e.convertL2Book(&update.L2Book, mapping.pair, a)
	if err != nil {
		return err
	}
	return e.Websocket.Orderbook.LoadSnapshot(ctx, book)
}

// wsHandleTrades relays the trades that convert and reports the rest, except the batch of recent trades Hyperliquid
// replays on subscribing
func (e *Exchange) wsHandleTrades(raw []byte) error {
	var updates []RecentTrade
	if err := json.Unmarshal(raw, &updates); err != nil {
		return err
	}
	if len(updates) == 0 || e.consumeTradeReplay(updates[0].Coin) {
		return nil
	}
	trades := make([]trade.Data, 0, len(updates))
	var errs error
	for i := range updates {
		mapping, a, err := e.lookupPairMappingByCoin(updates[i].Coin)
		if err != nil {
			errs = common.AppendError(errs, fmt.Errorf("trade %d: %w", i, err))
			continue
		}
		converted, err := e.convertTrade(&updates[i], mapping.pair, a)
		if err != nil {
			errs = common.AppendError(errs, fmt.Errorf("trade %d: %w", i, err))
			continue
		}
		trades = append(trades, converted)
	}
	if len(trades) > 0 {
		errs = common.AppendError(errs, e.Websocket.Trade.Update(e.IsSaveTradeDataEnabled(), trades...))
	}
	return errs
}

// expectTradeReplay marks a coin whose next trades message replays recent trades
func (e *Exchange) expectTradeReplay(coin string) {
	e.websocketPendingMu.Lock()
	if e.websocketTradeReplays == nil {
		e.websocketTradeReplays = make(map[string]struct{})
	}
	e.websocketTradeReplays[coin] = struct{}{}
	e.websocketPendingMu.Unlock()
}

// consumeTradeReplay reports whether a coin's trades message is the replay its subscription expects, clearing the mark
func (e *Exchange) consumeTradeReplay(coin string) bool {
	e.websocketPendingMu.Lock()
	defer e.websocketPendingMu.Unlock()
	if _, ok := e.websocketTradeReplays[coin]; !ok {
		return false
	}
	delete(e.websocketTradeReplays, coin)
	return true
}

// clearTradeReplay removes a coin's replay mark when its subscription is not sent or is removed
func (e *Exchange) clearTradeReplay(coin string) {
	e.websocketPendingMu.Lock()
	delete(e.websocketTradeReplays, coin)
	e.websocketPendingMu.Unlock()
}

func (e *Exchange) wsHandleCandle(ctx context.Context, raw []byte) error {
	var update Candle
	if err := json.Unmarshal(raw, &update); err != nil {
		return err
	}
	mapping, a, err := e.lookupPairMappingByCoin(update.Symbol)
	if err != nil {
		return err
	}
	interval, err := parseInterval(update.Interval)
	if err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, kline.Item{
		Exchange: e.Name,
		Asset:    a,
		Pair:     mapping.pair,
		Interval: interval,
		Candles:  []kline.Candle{convertCandle(&update)},
	})
}

// wsHandleOrderUpdates relays the updates that convert and reports the rest
func (e *Exchange) wsHandleOrderUpdates(ctx context.Context, raw []byte) error {
	var updates []WsOrder
	if err := json.Unmarshal(raw, &updates); err != nil {
		return err
	}
	orders := make([]order.Detail, 0, len(updates))
	var errs error
	for i := range updates {
		detail, err := e.convertWsOrder(&updates[i])
		if err != nil {
			errs = common.AppendError(errs, fmt.Errorf("order update %d: %w", i, err))
			continue
		}
		orders = append(orders, detail)
	}
	if len(orders) > 0 {
		errs = common.AppendError(errs, e.Websocket.DataHandler.Send(ctx, orders))
	}
	return errs
}

// wsHandleUserEvent relays a user event: fills and TWAP slice fills as fills, and the other events as themselves
func (e *Exchange) wsHandleUserEvent(ctx context.Context, raw []byte) error {
	var event WsUserEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return err
	}
	switch {
	case len(event.Fills) > 0:
		return e.relayFills(event.Fills, nil)
	case len(event.TWAPSliceFills) > 0:
		return e.relayFills(nil, event.TWAPSliceFills)
	case event.Funding != nil:
		return e.Websocket.DataHandler.Send(ctx, event.Funding)
	case event.Liquidation != nil:
		return e.Websocket.DataHandler.Send(ctx, event.Liquidation)
	case len(event.NonUserCancel) > 0:
		return e.Websocket.DataHandler.Send(ctx, event.NonUserCancel)
	default:
		return nil
	}
}

// wsHandleUserFills relays the fills that convert and reports the rest
// The snapshot sent on each subscription is dropped, so reconnecting does not replay fills already relayed
func (e *Exchange) wsHandleUserFills(raw []byte) error {
	var update WsUserFills
	if err := json.Unmarshal(raw, &update); err != nil {
		return err
	}
	if update.IsSnapshot {
		return nil
	}
	return e.relayFills(update.Fills, nil)
}

// wsHandleUserTWAPSliceFills relays TWAP slice fills as fills, dropping the snapshot sent on each subscription
func (e *Exchange) wsHandleUserTWAPSliceFills(raw []byte) error {
	var update WsUserTWAPSliceFills
	if err := json.Unmarshal(raw, &update); err != nil {
		return err
	}
	if update.IsSnapshot {
		return nil
	}
	return e.relayFills(nil, update.TWAPSliceFills)
}

// relayFills relays fills and TWAP slice fills to the fills feed, reporting those whose coin or side cannot convert
func (e *Exchange) relayFills(fills []Fill, sliceFills []TWAPSliceFill) error {
	if !e.IsFillsFeedEnabled() {
		return nil
	}
	data := make([]fill.Data, 0, len(fills)+len(sliceFills))
	var errs error
	for i := range fills {
		converted, err := e.convertFill(&fills[i])
		if err != nil {
			errs = common.AppendError(errs, fmt.Errorf("fill %d: %w", i, err))
			continue
		}
		data = append(data, converted)
	}
	for i := range sliceFills {
		converted, err := e.convertFill(&sliceFills[i].Fill)
		if err != nil {
			errs = common.AppendError(errs, fmt.Errorf("TWAP %d slice fill %d: %w", sliceFills[i].TWAPID, i, err))
			continue
		}
		data = append(data, converted)
	}
	if len(data) > 0 {
		errs = common.AppendError(errs, e.Websocket.Fills.Update(data...))
	}
	return errs
}

// convertFill converts a fill, identified by its trade ID
func (e *Exchange) convertFill(source *Fill) (fill.Data, error) {
	mapping, a, err := e.lookupPairMappingByCoin(source.Coin)
	if err != nil {
		return fill.Data{}, err
	}
	side, err := parseSide(source.Side)
	if err != nil {
		return fill.Data{}, err
	}
	tradeID := strconv.FormatUint(source.TradeID, 10)
	return fill.Data{
		ID:            tradeID,
		Timestamp:     source.Time.Time().UTC(),
		Exchange:      e.Name,
		AssetType:     a,
		CurrencyPair:  mapping.pair,
		Side:          side,
		OrderID:       strconv.FormatUint(source.OrderID, 10),
		ClientOrderID: source.ClientOrderID,
		TradeID:       tradeID,
		Price:         source.Price.Float64(),
		Amount:        source.Size.Float64(),
	}, nil
}

// wsHandleUserFundings relays funding payments; Hyperliquid can send a message without payments, which is dropped
func (e *Exchange) wsHandleUserFundings(ctx context.Context, raw []byte) error {
	update := new(WsUserFundings)
	if err := json.Unmarshal(raw, update); err != nil {
		return err
	}
	if len(update.Fundings) == 0 {
		return nil
	}
	return e.Websocket.DataHandler.Send(ctx, update)
}

// wsHandleUserNonFundingLedgerUpdates relays ledger updates; Hyperliquid sends a message without updates at each
// funding payment, which is dropped
func (e *Exchange) wsHandleUserNonFundingLedgerUpdates(ctx context.Context, raw []byte) error {
	update := new(WsUserNonFundingLedgerUpdates)
	if err := json.Unmarshal(raw, update); err != nil {
		return err
	}
	if len(update.NonFundingLedgerUpdates) == 0 {
		return nil
	}
	return e.Websocket.DataHandler.Send(ctx, update)
}

// wsHandleUserTWAPHistory relays TWAP status changes, dropping a message without changes
func (e *Exchange) wsHandleUserTWAPHistory(ctx context.Context, raw []byte) error {
	update := new(WsUserTWAPHistory)
	if err := json.Unmarshal(raw, update); err != nil {
		return err
	}
	if len(update.History) == 0 {
		return nil
	}
	return e.Websocket.DataHandler.Send(ctx, update)
}

// wsHandleOutcomeMetaUpdates relays outcome and question changes, dropping a message without changes
func (e *Exchange) wsHandleOutcomeMetaUpdates(ctx context.Context, raw []byte) error {
	var updates []WsOutcomeMetaUpdate
	if err := json.Unmarshal(raw, &updates); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}
	return e.Websocket.DataHandler.Send(ctx, updates)
}

// wsHandleFastAssetContexts relays the mark and mid prices of every coin, which arrive as a base64 string of raw
// DEFLATE compressed JSON; the first message after subscribing holds every coin and later messages only the changes
func (e *Exchange) wsHandleFastAssetContexts(ctx context.Context, raw []byte) error {
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return err
	}
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	reader := flate.NewReader(bytes.NewReader(compressed))
	defer reader.Close()
	decompressed, err := io.ReadAll(io.LimitReader(reader, maximumFastAssetContextsSize+1))
	if err != nil {
		return err
	}
	if len(decompressed) > maximumFastAssetContextsSize {
		return fmt.Errorf("%w: fastAssetCtxs message exceeds %d bytes", errWebsocketServer, maximumFastAssetContextsSize)
	}
	var contexts map[string]WsFastAssetContext
	if err := json.Unmarshal(decompressed, &contexts); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, contexts)
}

// WsPostInfo sends an info request body, such as an InfoRequest, over the public websocket connection and decodes the
// info endpoint's response into result
func (e *Exchange) WsPostInfo(ctx context.Context, payload, result any) error {
	if payload == nil || result == nil {
		return common.ErrNilPointer
	}
	response, err := e.sendWebsocketPost(ctx, payload)
	if err != nil {
		return err
	}
	var info WsPostInfoPayload
	if err := json.Unmarshal(response, &info); err != nil {
		return err
	}
	return json.Unmarshal(info.Data, result)
}

// sendWebsocketPost posts an info request and returns its response payload; a rejected request returns the server's
// error
func (e *Exchange) sendWebsocketPost(ctx context.Context, payload any) (json.RawMessage, error) {
	connection := e.Websocket.Conn
	if connection == nil {
		return nil, websocket.ErrNotConnected
	}
	id := e.websocketPostID.Add(1)
	raw, err := connection.SendMessageReturnResponse(ctx, request.Unset, id, &WsPostRequest{
		Method:  wsMethodPost,
		ID:      id,
		Request: WsPostRequestBody{Type: wsPostTypeInfo, Payload: payload},
	})
	if err != nil {
		return nil, err
	}
	var response WsPostResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	switch response.Response.Type {
	case wsPostTypeInfo:
		return response.Response.Payload, nil
	case wsPostTypeError:
		var message string
		if err := json.Unmarshal(response.Response.Payload, &message); err != nil {
			message = string(response.Response.Payload)
		}
		return nil, fmt.Errorf("%w: %s", errWebsocketPost, message)
	default:
		return nil, fmt.Errorf("%w: unexpected response type %q", errWebsocketPost, response.Response.Type)
	}
}

// websocketChannelName returns the Hyperliquid subscription type a subscription's channel refers to: a standard
// channel's type, or the type the channel names
func websocketChannelName(sub *subscription.Subscription) (string, error) {
	if sub == nil {
		return "", common.ErrNilPointer
	}
	if name, ok := websocketStandardChannels[sub.Channel]; ok {
		return name, nil
	}
	if _, ok := websocketFeeds[sub.Channel]; ok {
		return sub.Channel, nil
	}
	return "", fmt.Errorf("%w: %s", subscription.ErrNotSupported, sub.Channel)
}

// GetSubscriptionTemplate returns the channel qualification template
func (e *Exchange) GetSubscriptionTemplate(_ *subscription.Subscription) (*template.Template, error) {
	return template.New("master.tmpl").Funcs(template.FuncMap{
		"channelName": websocketChannelName,
		"perPair":     func(name string) bool { return websocketFeeds[name].coin },
		"interval": func(value kline.Interval) string {
			formatted, _ := formatInterval(value) // Subscription validation rejects an interval Hyperliquid does not serve
			return formatted
		},
	}).Parse(websocketSubscriptionTemplate)
}

func (e *Exchange) generateSubscriptions() (subscription.List, error) {
	return e.Features.Subscriptions.ExpandTemplates(e)
}

// Subscribe subscribes to feeds, completing only once the server acknowledges each one
func (e *Exchange) Subscribe(subscriptions subscription.List) error {
	ctx := context.TODO()
	if slices.Contains(subscriptions, nil) {
		return common.ErrNilPointer
	}
	expanded, errs := subscriptions.ExpandTemplates(e)
	for _, sub := range expanded {
		errs = common.AppendError(errs, e.manageWebsocketSubscription(ctx, wsMethodSubscribe, sub))
	}
	return errs
}

// Unsubscribe unsubscribes from feeds, completing only once the server acknowledges each one
func (e *Exchange) Unsubscribe(subscriptions subscription.List) error {
	ctx := context.TODO()
	if slices.Contains(subscriptions, nil) {
		return common.ErrNilPointer
	}
	expanded, errs := subscriptions.ExpandTemplates(e)
	for _, key := range expanded {
		sub := e.Websocket.GetSubscription(key)
		if sub == nil {
			errs = common.AppendError(errs, fmt.Errorf("%w: %s", subscription.ErrNotFound, key))
			continue
		}
		errs = common.AppendError(errs, e.manageWebsocketSubscription(ctx, wsMethodUnsubscribe, sub))
	}
	return errs
}

// manageWebsocketSubscription sends a subscription operation and waits for its acknowledgement, rolling the local
// subscription state back if the server rejects it, the connection drops or it times out
func (e *Exchange) manageWebsocketSubscription(ctx context.Context, method string, sub *subscription.Subscription) error {
	if sub == nil {
		return common.ErrNilPointer
	}
	if method != wsMethodSubscribe && method != wsMethodUnsubscribe {
		return fmt.Errorf("%w: %s", common.ErrNotYetImplemented, method)
	}
	payload, err := e.websocketSubscriptionPayload(ctx, sub)
	if err != nil {
		return err
	}
	connection := e.Websocket.Conn
	if sub.Authenticated {
		connection = e.Websocket.AuthConn
	}
	if connection == nil {
		return common.ErrNilPointer
	}
	previousState := sub.State()
	if method == wsMethodSubscribe {
		if err := e.Websocket.AddSubscriptions(connection, sub); err != nil {
			return err
		}
	} else if err := sub.SetState(subscription.UnsubscribingState); err != nil {
		return err
	}
	key := websocketPendingKey{authenticated: sub.Authenticated, subscription: payload}
	pending := &websocketPendingOperation{
		method:        method,
		connection:    connection,
		subscription:  sub,
		previousState: previousState,
		done:          make(chan error, 1),
	}
	e.websocketPendingMu.Lock()
	if e.websocketPending == nil {
		e.websocketPending = make(map[websocketPendingKey]*websocketPendingOperation)
	}
	if e.websocketPending[key] != nil {
		e.websocketPendingMu.Unlock()
		return common.AppendError(fmt.Errorf("%w: operation already pending for %s", errWebsocketSubscription, sub), e.rollbackWebsocketPending(pending))
	}
	e.websocketPending[key] = pending
	e.websocketPendingMu.Unlock()

	// A trades subscription opens with recent trades, which must not be relayed as new trades
	replays := payload.Type == wsChannelTrades && method == wsMethodSubscribe
	if replays {
		e.expectTradeReplay(payload.Coin)
	}
	err = e.awaitWebsocketPending(ctx, connection, &key, pending, &WsSubscriptionRequest{Method: method, Subscription: payload})
	if payload.Type == wsChannelTrades && (err != nil || !replays) {
		e.clearTradeReplay(payload.Coin)
	}
	return err
}

// awaitWebsocketPending sends a subscription request and waits for the server to settle its pending operation
func (e *Exchange) awaitWebsocketPending(ctx context.Context, connection websocket.Connection, key *websocketPendingKey, pending *websocketPendingOperation, req *WsSubscriptionRequest) error {
	if err := connection.SendJSONMessage(ctx, request.Unset, req); err != nil {
		return e.abortWebsocketPending(key, pending, err)
	}
	timeout := e.WebsocketResponseMaxLimit
	if timeout <= 0 {
		timeout = exchange.DefaultWebsocketResponseMaxLimit
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-pending.done:
		return err
	case <-ctx.Done():
		return e.abortWebsocketPending(key, pending, ctx.Err())
	case <-timer.C:
		return e.abortWebsocketPending(key, pending, fmt.Errorf("%w: %s", websocket.ErrSignatureTimeout, pending.subscription))
	}
}

// websocketSubscriptionPayload builds the feed a subscription refers to, validating every coin, DEX and option first,
// because Hyperliquid closes the connection, dropping every feed on it, for a subscription it cannot validate
func (e *Exchange) websocketSubscriptionPayload(ctx context.Context, sub *subscription.Subscription) (WsSubscription, error) {
	name, err := websocketChannelName(sub)
	if err != nil {
		return WsSubscription{}, err
	}
	feed := websocketFeeds[name]
	if sub.Authenticated != feed.user {
		return WsSubscription{}, fmt.Errorf("%w: %s must set authenticated to %t", subscription.ErrNotSupported, name, feed.user)
	}
	payload := WsSubscription{Type: name}
	if feed.user {
		if payload.User, err = e.getWatchAddress(ctx); err != nil {
			return WsSubscription{}, err
		}
	}
	switch {
	case feed.coin:
		if len(sub.Pairs) != 1 {
			return WsSubscription{}, subscription.ErrNotSinglePair
		}
		if name == wsChannelActiveAssetData && sub.Asset != asset.PerpetualContract {
			return WsSubscription{}, fmt.Errorf("%w: %s serves perpetual contracts only", asset.ErrNotSupported, name)
		}
		mapping, err := e.getPairMapping(ctx, sub.Pairs[0], sub.Asset)
		if err != nil {
			return WsSubscription{}, err
		}
		payload.Coin = mapping.coin
	case sub.Asset != asset.Empty:
		return WsSubscription{}, fmt.Errorf("%w: %s does not take an asset", subscription.ErrNotSupported, name)
	}
	if feed.dex {
		if payload.DEX, err = e.websocketDEXParameter(ctx, sub.Params); err != nil {
			return WsSubscription{}, err
		}
	}
	switch name {
	case wsChannelCandle:
		if payload.Interval, err = formatInterval(sub.Interval); err != nil {
			return WsSubscription{}, err
		}
	case wsChannelOrderbook:
		if err := websocketOrderbookOptions(sub, &payload); err != nil {
			return WsSubscription{}, err
		}
	case wsChannelUserFills:
		if payload.AggregateByTime, err = websocketBoolParameter(sub.Params, "aggregateByTime"); err != nil {
			return WsSubscription{}, err
		}
	case wsChannelSpotState:
		if payload.IgnorePortfolioMargin, err = websocketBoolParameter(sub.Params, "ignorePortfolioMargin"); err != nil {
			return WsSubscription{}, err
		}
	}
	return payload, nil
}

// websocketOrderbookOptions sets an l2Book's options: 5 levels select the fast book, and the nSigFigs and mantissa
// parameters aggregate it
func websocketOrderbookOptions(sub *subscription.Subscription, payload *WsSubscription) error {
	switch sub.Levels {
	case 0, 20:
	case 5:
		payload.Fast = true
	default:
		return fmt.Errorf("%w: %d, as l2Book sends 20 levels or 5 fast levels", subscription.ErrInvalidLevel, sub.Levels)
	}
	var err error
	if payload.SignificantFigures, err = websocketUintParameter(sub.Params, "nSigFigs"); err != nil {
		return err
	}
	if payload.Mantissa, err = websocketUintParameter(sub.Params, "mantissa"); err != nil {
		return err
	}
	return validateBookAggregation(payload.SignificantFigures, payload.Mantissa)
}

// websocketDEXParameter returns a subscription's dex parameter, which must name a registered perpetual DEX
func (e *Exchange) websocketDEXParameter(ctx context.Context, params map[string]any) (string, error) {
	value, ok := params["dex"]
	if !ok || value == nil {
		return "", nil
	}
	dex, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%w: dex %v", errWebsocketParameter, value)
	}
	if dex == "" {
		return "", nil
	}
	names, err := e.getPerpetualDEXNames(ctx)
	if err != nil {
		return "", err
	}
	if !slices.Contains(names[1:], dex) {
		return "", fmt.Errorf("%w: %q", errInvalidPerpetualDEX, dex)
	}
	return dex, nil
}

// websocketUintParameter returns a non-negative whole number subscription parameter, which configuration supplies as a
// JSON number; an absent parameter is zero
func websocketUintParameter(params map[string]any, name string) (uint64, error) {
	switch value := params[name].(type) {
	case nil:
		return 0, nil
	case uint64:
		return value, nil
	case int:
		if value >= 0 {
			return uint64(value), nil
		}
	case float64:
		if value >= 0 && value <= math.MaxUint32 && value == math.Trunc(value) {
			return uint64(value), nil
		}
	}
	return 0, fmt.Errorf("%w: %s %v", errWebsocketParameter, name, params[name])
}

// websocketBoolParameter returns a boolean subscription parameter; an absent parameter is false
func websocketBoolParameter(params map[string]any, name string) (bool, error) {
	switch value := params[name].(type) {
	case nil:
		return false, nil
	case bool:
		return value, nil
	default:
		return false, fmt.Errorf("%w: %s %v", errWebsocketParameter, name, value)
	}
}

// websocketSubscriptionTemplate qualifies a subscription per pair for market feeds and once otherwise
const websocketSubscriptionTemplate = `
{{- $name := channelName $.S }}
{{- if $.S.Asset }}
	{{- range $asset, $pairs := $.AssetPairs }}
		{{- if perPair $name }}
			{{- range $pair := $pairs }}
				{{- $name -}} : {{- $asset -}} : {{- $pair -}}
				{{- if $.S.Interval -}} : {{- interval $.S.Interval -}}{{- end -}}
				{{ $.PairSeparator }}
			{{- end }}
		{{- else }}
			{{- $name -}} : {{- $asset }}
		{{- end }}
		{{ $.AssetSeparator }}
	{{- end }}
{{- else -}}
	{{ $name }}
{{- end }}
`
