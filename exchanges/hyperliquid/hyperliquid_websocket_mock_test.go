package hyperliquid

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	ws "github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fill"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
)

var errWebsocketFixture = errors.New("websocket fixture failure")

type websocketConnectionFixture struct {
	ws.Connection

	mu       sync.Mutex
	dialErr  error
	sendErr  error
	sendHook func(websocketRequest, error)
	messages []ws.Response
	sent     []websocketRequest
	ping     ws.PingHandler
	url      string

	dialCalls     atomic.Int32
	readCalls     atomic.Int32
	pingCalls     atomic.Int32
	shutdownCalls atomic.Int32
}

func (f *websocketConnectionFixture) Dial(context.Context, *gws.Dialer, http.Header, url.Values) error {
	f.dialCalls.Add(1)
	return f.dialErr
}

func (f *websocketConnectionFixture) ReadMessage() ws.Response {
	f.readCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) == 0 {
		return ws.Response{}
	}
	message := f.messages[0]
	f.messages = f.messages[1:]
	return message
}

func (f *websocketConnectionFixture) SetupPingHandler(_ request.EndpointLimit, handler ws.PingHandler) {
	f.pingCalls.Add(1)
	f.mu.Lock()
	f.ping = handler
	f.mu.Unlock()
}

func (f *websocketConnectionFixture) SendJSONMessage(_ context.Context, _ request.EndpointLimit, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	wsRequest, ok := payload.(websocketRequest)
	if !ok {
		return common.GetTypeAssertError("websocketRequest", payload)
	}
	if f.sendHook != nil {
		f.sendHook(wsRequest, f.sendErr)
	}
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, wsRequest)
	return nil
}

func (f *websocketConnectionFixture) SetURL(value string) {
	f.mu.Lock()
	f.url = value
	f.mu.Unlock()
}

func (f *websocketConnectionFixture) GetURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.url
}

func (f *websocketConnectionFixture) Shutdown() error {
	f.shutdownCalls.Add(1)
	return nil
}

func (f *websocketConnectionFixture) sentRequests() []websocketRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]websocketRequest(nil), f.sent...)
}

func waitForHyperliquidWebsocketReaders(t *testing.T, group *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("group must finish waiting for websocket readers before the timeout")
	}
}

func newWebsocketConnectTestExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := new(Exchange)
	ex.SetDefaults()
	cfg, err := ex.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for websocket test config")
	cfg.Features = &config.FeaturesConfig{Enabled: config.FeaturesEnabledConfig{Websocket: true}}
	require.NoError(t, ex.Setup(cfg), "Setup must not error for websocket test exchange")
	return ex
}

func newWebsocketHandlerTestExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := newTradingTestExchange(t, nil, nil)
	ex.Name += "-" + strings.ReplaceAll(t.Name(), "/", "-")
	require.NoError(t, ex.UpdatePairs(currency.Pairs{testPerpetualPair}, asset.PerpetualContract, false), "UpdatePairs: updating available perpetual test pairs must not error")
	require.NoError(t, ex.UpdatePairs(currency.Pairs{testPerpetualPair}, asset.PerpetualContract, true), "UpdatePairs: updating enabled perpetual test pairs must not error")
	require.NoError(t, ex.UpdatePairs(currency.Pairs{testSpotPair}, asset.Spot, false), "UpdatePairs: updating available spot test pairs must not error")
	require.NoError(t, ex.UpdatePairs(currency.Pairs{testSpotPair}, asset.Spot, true), "UpdatePairs: updating enabled spot test pairs must not error")
	ex.Websocket.Trade.Setup(true, ex.Websocket.DataHandler)
	ex.SetFillsFeedStatus(true)
	ex.Websocket.Fills.Setup(true, ex.Websocket.DataHandler)
	return ex
}

func websocketAcknowledgement(t *testing.T, req *websocketRequest) []byte {
	t.Helper()
	response, err := json.Marshal(struct {
		Channel string                        `json:"channel"`
		Data    websocketSubscriptionResponse `json:"data"`
	}{
		Channel: wsChannelSubscriptionResponse,
		Data:    websocketSubscriptionResponse(*req),
	})
	require.NoError(t, err, "Marshal must not error for websocket acknowledgement")
	return response
}

func installWebsocketAcknowledgements(t *testing.T, ex *Exchange, connection *websocketConnectionFixture, authenticated bool) {
	t.Helper()
	connection.sendHook = func(req websocketRequest, sendErr error) {
		if sendErr != nil {
			return
		}
		require.NoError(t, ex.websocketHandleDataForConnection(t.Context(), websocketAcknowledgement(t, &req), authenticated), "websocketHandleDataForConnection must not error for handling websocket acknowledgement")
	}
}

func receiveWebsocketData(t *testing.T, ex *Exchange) any {
	t.Helper()
	select {
	case payload := <-ex.Websocket.DataHandler.C:
		return payload.Data
	case <-time.After(time.Second):
		t.Fatal("receiveWebsocketData must receive websocket data before the timeout")
		return nil
	}
}

func TestConnectWebsocket(t *testing.T) {
	ex := new(Exchange)
	require.ErrorIs(t, ex.connectWebsocket(t.Context(), nil), common.ErrNilPointer, "connectWebsocket must return the expected error for nil websocket connection")

	failed := &websocketConnectionFixture{dialErr: errWebsocketFixture}
	require.ErrorIs(t, ex.connectWebsocket(t.Context(), failed), errWebsocketFixture, "connectWebsocket must return websocket dial failure")
	assert.Zero(t, failed.pingCalls.Load(), "failed.pingCalls: failed websocket connection should not install a ping handler")

	connection := new(websocketConnectionFixture)
	require.NoError(t, ex.connectWebsocket(t.Context(), connection), "connectWebsocket must not error for connecting a websocket fixture")
	assert.Equal(t, int32(1), connection.dialCalls.Load(), "connection.dialCalls: websocket should dial once")
	assert.Equal(t, int32(1), connection.pingCalls.Load(), "connection.pingCalls: websocket should install one ping handler")
	assert.JSONEq(t, `{"method":"ping"}`, string(connection.ping.Message), "connection.ping.Message: ping handler should send the application ping")
	assert.Equal(t, websocketPingInterval, connection.ping.Delay, "connection.ping.Delay: ping handler should use the documented interval")
}

func TestWsConnect(t *testing.T) {
	disabledWebsocket := new(Exchange)
	disabledWebsocket.SetDefaults()
	require.ErrorIs(t, disabledWebsocket.WsConnect(), ws.ErrWebsocketNotEnabled, "WsConnect must return the expected error for disabled websocket")

	disabledExchange := newWebsocketConnectTestExchange(t)
	disabledExchange.SetEnabled(false)
	require.ErrorIs(t, disabledExchange.WsConnect(), ws.ErrWebsocketNotEnabled, "WsConnect must return the expected websocket error for disabled exchange")

	publicFailure := newWebsocketConnectTestExchange(t)
	publicFailure.Websocket.Conn = &websocketConnectionFixture{dialErr: errWebsocketFixture}
	require.ErrorIs(t, publicFailure.WsConnect(), errWebsocketFixture, "WsConnect must return public websocket dial failure")

	publicOnly := newWebsocketConnectTestExchange(t)
	publicOnly.API.AuthenticatedWebsocketSupport = false
	publicConnection := new(websocketConnectionFixture)
	publicOnly.Websocket.Conn = publicConnection
	require.NoError(t, publicOnly.WsConnect(), "WsConnect must not error for public-only websocket connection")
	waitForHyperliquidWebsocketReaders(t, &publicOnly.Websocket.Wg)
	assert.Equal(t, int32(1), publicConnection.readCalls.Load(), "publicConnection.readCalls: public websocket reader should start")

	missingCredentials := newWebsocketConnectTestExchange(t)
	missingCredentials.API.AuthenticatedWebsocketSupport = true
	missingCredentials.Websocket.SetCanUseAuthenticatedEndpoints(true)
	missingPublic := new(websocketConnectionFixture)
	missingAuth := new(websocketConnectionFixture)
	missingCredentials.Websocket.Conn = missingPublic
	missingCredentials.Websocket.AuthConn = missingAuth
	require.NoError(t, missingCredentials.WsConnect(), "WsConnect must degrade to public websocket only for missing address")
	waitForHyperliquidWebsocketReaders(t, &missingCredentials.Websocket.Wg)
	assert.False(t, missingCredentials.Websocket.CanUseAuthenticatedEndpoints(), "CanUseAuthenticatedEndpoints: missing address should disable account-scoped feeds")
	assert.Zero(t, missingAuth.dialCalls.Load(), "missingAuth.dialCalls: missing address should not dial the account connection")

	authFailure := newWebsocketConnectTestExchange(t)
	authFailure.API.AuthenticatedWebsocketSupport = true
	setTestCredentials(authFailure, &accounts.Credentials{Key: officialSigningAddress})
	authFailure.Websocket.SetCanUseAuthenticatedEndpoints(true)
	authFailure.Websocket.Conn = new(websocketConnectionFixture)
	failedAuthConnection := &websocketConnectionFixture{dialErr: errWebsocketFixture}
	authFailure.Websocket.AuthConn = failedAuthConnection
	require.NoError(t, authFailure.WsConnect(), "WsConnect must retain the public stream for account websocket dial failure")
	waitForHyperliquidWebsocketReaders(t, &authFailure.Websocket.Wg)
	assert.False(t, authFailure.Websocket.CanUseAuthenticatedEndpoints(), "CanUseAuthenticatedEndpoints: account dial failure should disable account-scoped feeds")
	assert.Zero(t, failedAuthConnection.readCalls.Load(), "failedAuthConnection.readCalls: failed account connection should not start a reader")

	authenticated := newWebsocketConnectTestExchange(t)
	authenticated.API.AuthenticatedWebsocketSupport = true
	setTestCredentials(authenticated, &accounts.Credentials{Key: officialSigningAddress})
	authenticated.Websocket.Conn = new(websocketConnectionFixture)
	authenticatedConnection := new(websocketConnectionFixture)
	authenticated.Websocket.AuthConn = authenticatedConnection
	require.NoError(t, authenticated.WsConnect(), "WsConnect must not error for public and account websocket connections")
	waitForHyperliquidWebsocketReaders(t, &authenticated.Websocket.Wg)
	assert.True(t, authenticated.Websocket.CanUseAuthenticatedEndpoints(), "CanUseAuthenticatedEndpoints: configured address should enable account-scoped feeds")
	assert.Equal(t, int32(1), authenticatedConnection.readCalls.Load(), "authenticatedConnection.readCalls: account websocket reader should start")
}

func TestWebsocketReadLoop(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	connection := &websocketConnectionFixture{messages: []ws.Response{{Raw: []byte(`invalid`)}, {}}}
	ex.Websocket.Wg.Add(1)
	ex.websocketReadLoop(t.Context(), connection, false)
	waitForHyperliquidWebsocketReaders(t, &ex.Websocket.Wg)
	relayed, ok := receiveWebsocketData(t, ex).(error)
	require.True(t, ok, "ok: read-loop message must be an error")
	assert.Error(t, relayed, "relayed: read-loop handler error should be relayed")
	assert.Equal(t, int32(2), connection.readCalls.Load(), "connection.readCalls: read loop should stop on an empty message")

	fullRelay := newWebsocketHandlerTestExchange(t)
	fullRelay.Websocket.DataHandler = stream.NewRelay(1)
	require.NoError(t, fullRelay.Websocket.DataHandler.Send(t.Context(), "filler"), "Send must not error for filling the websocket relay")
	fullConnection := &websocketConnectionFixture{messages: []ws.Response{{Raw: []byte(`invalid`)}, {}}}
	fullRelay.Websocket.Wg.Add(1)
	fullRelay.websocketReadLoop(t.Context(), fullConnection, false)
	waitForHyperliquidWebsocketReaders(t, &fullRelay.Websocket.Wg)
	assert.Equal(t, int32(2), fullConnection.readCalls.Load(), "fullConnection.readCalls: read loop should continue after a full data relay")
}

func TestWebsocketHandleDataRouting(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	require.Error(t, ex.websocketHandleData(t.Context(), []byte(`invalid`)), "websocketHandleData must error for invalid websocket envelope")
	require.NoError(t, ex.websocketHandleData(t.Context(), []byte(websocketConnectionEstablished)), "websocketHandleData must be ignored for connection acknowledgement")
	require.ErrorIs(t, ex.websocketHandleData(t.Context(), []byte(`{"channel":"subscriptionResponse","data":{}}`)), errWebsocketSubscription, "websocketHandleData must error for malformed subscription response")
	require.NoError(t, ex.websocketHandleData(t.Context(), []byte(`{"channel":"pong","data":{}}`)), "websocketHandleData must be ignored for pong")
	err := ex.websocketHandleData(t.Context(), []byte(`{"channel":"error","data":"bad subscription"}`))
	require.ErrorIs(t, err, errWebsocketServer, "websocketHandleData must return the expected error for websocket error channel")
	assert.ErrorContains(t, err, "bad subscription", "websocketHandleData should retain the server message for websocket error")
	err = ex.websocketHandleData(t.Context(), []byte(`{"channel":"error","data":{"message":"bad request"}}`))
	require.ErrorIs(t, err, errWebsocketServer, "websocketHandleData must return the expected error for structured websocket error")
	assert.ErrorContains(t, err, `"message":"bad request"`, "websocketHandleData should retain the server payload for structured websocket error")
	err = ex.websocketHandleData(t.Context(), []byte(`{"channel":"error","data":null}`))
	require.ErrorIs(t, err, errWebsocketServer, "websocketHandleData must return the expected error for empty websocket error")
	assert.ErrorContains(t, err, "unspecified error", "websocketHandleData should use a safe fallback for empty websocket error")

	for _, channel := range []string{
		wsChannelActiveAssetContext,
		wsChannelActiveSpotAssetContext,
		wsChannelOrderbook,
		wsChannelTrades,
		wsChannelCandle,
		wsChannelOrderUpdates,
		wsChannelUserFills,
	} {
		err := ex.websocketHandleData(t.Context(), []byte(`{"channel":"`+channel+`","data":"invalid"}`))
		require.Error(t, err, "websocketHandleData must error for invalid routed channel data")
	}

	require.NoError(t, ex.websocketHandleData(t.Context(), []byte(`{"channel":"futureChannel","data":{}}`)), "websocketHandleData must be relayed as a warning for unhandled websocket message")
	_, ok := receiveWebsocketData(t, ex).(ws.UnhandledMessageWarning)
	assert.True(t, ok, "ok: unhandled channel should relay the expected warning type")
}

func TestWebsocketHandleSubscriptionAcknowledgement(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	connection := new(websocketConnectionFixture)
	ex.Websocket.Conn = connection

	require.ErrorIs(
		t,
		ex.websocketHandleDataForConnection(t.Context(), []byte(`{"channel":"subscriptionResponse","data":"invalid"}`), false),
		errWebsocketSubscription,
		"websocketHandleDataForConnection must return the expected error for invalid acknowledgement payload",
	)
	require.ErrorIs(
		t,
		ex.websocketHandleDataForConnection(t.Context(), websocketAcknowledgement(t, &websocketRequest{
			Method:       "invalid",
			Subscription: websocketSubscription{Type: wsChannelTrades, Coin: "BTC"},
		}), false),
		errWebsocketSubscription,
		"websocketHandleDataForConnection must return the expected error for invalid acknowledgement method",
	)
	require.ErrorIs(
		t,
		ex.websocketHandleDataForConnection(t.Context(), websocketAcknowledgement(t, &websocketRequest{Method: wsMethodSubscribe}), false),
		errWebsocketSubscription,
		"websocketHandleDataForConnection must return the expected error for acknowledgement without a subscription type",
	)

	unknown := websocketRequest{
		Method:       wsMethodSubscribe,
		Subscription: websocketSubscription{Type: wsChannelTrades, Coin: "UNKNOWN"},
	}
	require.NoError(t, ex.websocketHandleDataForConnection(t.Context(), websocketAcknowledgement(t, &unknown), false), "websocketHandleDataForConnection must be ignored for unknown or late acknowledgement")

	wrongMethodSub := &subscription.Subscription{Channel: subscription.AllTradesChannel}
	require.NoError(t, wrongMethodSub.SetState(subscription.SubscribingState), "SetState: preparing wrong-method subscription state must not error")
	wrongMethodPayload := websocketSubscription{Type: wsChannelTrades, Coin: "BTC"}
	wrongMethodKey := websocketPendingKey{subscription: wrongMethodPayload}
	wrongMethodPending := &websocketPendingOperation{
		method:        wsMethodSubscribe,
		connection:    connection,
		subscription:  wrongMethodSub,
		previousState: subscription.InactiveState,
		done:          make(chan error, 1),
	}
	ex.websocketPending[wrongMethodKey] = wrongMethodPending
	err := ex.websocketHandleDataForConnection(t.Context(), websocketAcknowledgement(t, &websocketRequest{
		Method:       wsMethodUnsubscribe,
		Subscription: wrongMethodPayload,
	}), false)
	require.ErrorIs(t, err, errWebsocketSubscription, "websocketHandleDataForConnection must fail closed for crossed acknowledgement method")
	assert.Same(t, wrongMethodPending, ex.websocketPending[wrongMethodKey], "ex.websocketPending[wrongMethodKey]: crossed acknowledgement should not remove the pending operation")
	delete(ex.websocketPending, wrongMethodKey)

	stateConflictSub := &subscription.Subscription{
		Channel:          subscription.OrderbookChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "ack-state-conflict",
	}
	require.NoError(t, ex.Websocket.AddSuccessfulSubscriptions(connection, stateConflictSub), "AddSuccessfulSubscriptions: preparing subscribed acknowledgement fixture must not error")
	stateConflictPayload := websocketSubscription{Type: wsChannelOrderbook, Coin: "BTC"}
	stateConflictPending := &websocketPendingOperation{
		method:        wsMethodSubscribe,
		connection:    connection,
		subscription:  stateConflictSub,
		previousState: subscription.InactiveState,
		done:          make(chan error, 1),
	}
	ex.websocketPending[websocketPendingKey{subscription: stateConflictPayload}] = stateConflictPending
	err = ex.websocketHandleDataForConnection(t.Context(), websocketAcknowledgement(t, &websocketRequest{
		Method:       wsMethodSubscribe,
		Subscription: stateConflictPayload,
	}), false)
	require.ErrorIs(t, err, subscription.ErrInStateAlready, "websocketHandleDataForConnection must return conflicting subscribe acknowledgement state")
	require.ErrorIs(t, <-stateConflictPending.done, subscription.ErrInStateAlready, "Subscribe waiter must receive the state conflict")
	assert.Nil(t, ex.Websocket.GetSubscription(stateConflictSub), "GetSubscription: conflicting subscribe acknowledgement should roll back the stored subscription")

	missingUnsubscribeSub := &subscription.Subscription{Channel: subscription.CandlesChannel}
	require.NoError(t, missingUnsubscribeSub.SetState(subscription.SubscribedState), "SetState: preparing unsubscribe fixture state must not error")
	require.NoError(t, missingUnsubscribeSub.SetState(subscription.UnsubscribingState), "SetState: preparing unsubscribe transition must not error")
	missingUnsubscribePayload := websocketSubscription{Type: wsChannelCandle, Coin: "BTC", Interval: "1m"}
	missingUnsubscribePending := &websocketPendingOperation{
		method:        wsMethodUnsubscribe,
		connection:    connection,
		subscription:  missingUnsubscribeSub,
		previousState: subscription.SubscribedState,
		done:          make(chan error, 1),
	}
	ex.websocketPending[websocketPendingKey{subscription: missingUnsubscribePayload}] = missingUnsubscribePending
	err = ex.websocketHandleDataForConnection(t.Context(), websocketAcknowledgement(t, &websocketRequest{
		Method:       wsMethodUnsubscribe,
		Subscription: missingUnsubscribePayload,
	}), false)
	require.ErrorIs(t, err, subscription.ErrNotFound, "websocketHandleDataForConnection must return the store error for acknowledged unknown unsubscribe")
	require.ErrorIs(t, <-missingUnsubscribePending.done, subscription.ErrNotFound, "Unsubscribe waiter must receive the store error")
	assert.Equal(t, subscription.SubscribedState, missingUnsubscribeSub.State(), "State: failed unsubscribe acknowledgement should restore the prior state")

	authSub := &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true}
	require.NoError(t, authSub.SetState(subscription.SubscribingState), "SetState: preparing authenticated subscription state must not error")
	authPayload := websocketSubscription{Type: wsChannelOrderUpdates, User: officialSigningAddress}
	authKey := websocketPendingKey{authenticated: true, subscription: authPayload}
	authPending := &websocketPendingOperation{
		method:        wsMethodSubscribe,
		connection:    connection,
		subscription:  authSub,
		previousState: subscription.InactiveState,
		done:          make(chan error, 1),
	}
	ex.websocketPending[authKey] = authPending
	authAck := websocketAcknowledgement(t, &websocketRequest{Method: wsMethodSubscribe, Subscription: authPayload})
	require.NoError(t, ex.websocketHandleDataForConnection(t.Context(), authAck, false), "websocketHandleDataForConnection must be ignored for acknowledgement on the wrong connection")
	assert.Same(t, authPending, ex.websocketPending[authKey], "ex.websocketPending[authKey]: wrong-connection acknowledgement should not mutate authenticated state")
	require.NoError(t, ex.websocketHandleDataForConnection(t.Context(), authAck, true), "websocketHandleDataForConnection must complete its exact operation for authenticated acknowledgement")
	require.NoError(t, <-authPending.done, "authPending.done: authenticated subscription waiter must receive success")
	assert.Equal(t, subscription.SubscribedState, authSub.State(), "State: authenticated acknowledgement should mark the subscription active")
	require.NoError(t, ex.websocketHandleDataForConnection(t.Context(), authAck, true), "websocketHandleDataForConnection must be ignored for duplicate late acknowledgement")
}

func TestRollbackWebsocketPending(t *testing.T) {
	var nilExchange *Exchange
	require.ErrorIs(t, nilExchange.rollbackWebsocketPending(nil), common.ErrNilPointer, "rollbackWebsocketPending must return the expected error for nil rollback inputs")

	ex := newWebsocketHandlerTestExchange(t)
	connection := new(websocketConnectionFixture)
	subscribeSub := &subscription.Subscription{Channel: subscription.AllTradesChannel, QualifiedChannel: "rollback-subscribe"}
	require.NoError(t, ex.Websocket.AddSubscriptions(connection, subscribeSub), "AddSubscriptions: preparing subscribe rollback must not error")
	require.NoError(t, ex.rollbackWebsocketPending(&websocketPendingOperation{
		method:       wsMethodSubscribe,
		connection:   connection,
		subscription: subscribeSub,
	}), "rollbackWebsocketPending must not error for rolling back a subscribe")
	assert.Nil(t, ex.Websocket.GetSubscription(subscribeSub), "Subscribe rollback should remove the subscription")

	unsubscribeSub := &subscription.Subscription{Channel: subscription.AllTradesChannel}
	require.NoError(t, unsubscribeSub.SetState(subscription.SubscribedState), "SetState: preparing unsubscribe rollback state must not error")
	require.NoError(t, ex.rollbackWebsocketPending(&websocketPendingOperation{
		method:        wsMethodUnsubscribe,
		connection:    connection,
		subscription:  unsubscribeSub,
		previousState: subscription.SubscribedState,
	}), "rollbackWebsocketPending must be a no-op for rollback already in the previous unsubscribe state")
	require.NoError(t, unsubscribeSub.SetState(subscription.UnsubscribingState), "SetState: preparing active unsubscribe rollback must not error")
	require.NoError(t, ex.rollbackWebsocketPending(&websocketPendingOperation{
		method:        wsMethodUnsubscribe,
		connection:    connection,
		subscription:  unsubscribeSub,
		previousState: subscription.SubscribedState,
	}), "rollbackWebsocketPending must restore its previous state for rolling back an unsubscribe")
	assert.Equal(t, subscription.SubscribedState, unsubscribeSub.State(), "Unsubscribe rollback should restore the previous state")

	require.ErrorIs(t, ex.rollbackWebsocketPending(&websocketPendingOperation{
		method:       "invalid",
		connection:   connection,
		subscription: unsubscribeSub,
	}), common.ErrNotYetImplemented, "rollbackWebsocketPending must return the expected error for unknown rollback method")
}

func TestFailWebsocketPending(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	require.ErrorIs(t, ex.failWebsocketPending(false, nil), common.ErrNilPointer, "failWebsocketPending must return the expected error for nil pending failure cause")

	connection := new(websocketConnectionFixture)
	publicSub := &subscription.Subscription{Channel: subscription.AllTradesChannel, QualifiedChannel: "fail-public"}
	authSub := &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true, QualifiedChannel: "fail-auth"}
	require.NoError(t, ex.Websocket.AddSubscriptions(connection, publicSub, authSub), "AddSubscriptions: preparing pending failure subscriptions must not error")
	publicPending := &websocketPendingOperation{
		method:       wsMethodSubscribe,
		connection:   connection,
		subscription: publicSub,
		done:         make(chan error, 1),
	}
	authPending := &websocketPendingOperation{
		method:       wsMethodSubscribe,
		connection:   connection,
		subscription: authSub,
		done:         make(chan error, 1),
	}
	invalidPending := &websocketPendingOperation{
		method:       "invalid",
		connection:   connection,
		subscription: &subscription.Subscription{},
		done:         make(chan error, 1),
	}
	publicKey := websocketPendingKey{subscription: websocketSubscription{Type: wsChannelTrades, Coin: "BTC"}}
	authKey := websocketPendingKey{authenticated: true, subscription: websocketSubscription{Type: wsChannelOrderUpdates, User: officialSigningAddress}}
	invalidKey := websocketPendingKey{subscription: websocketSubscription{Type: wsChannelCandle, Coin: "BTC", Interval: "1m"}}
	ex.websocketPending[publicKey] = publicPending
	ex.websocketPending[authKey] = authPending
	ex.websocketPending[invalidKey] = invalidPending

	err := ex.failWebsocketPending(false, errWebsocketFixture)
	require.ErrorIs(t, err, errWebsocketFixture, "failWebsocketPending must retain the connection cause for pending failure")
	require.ErrorIs(t, err, common.ErrNotYetImplemented, "failWebsocketPending must retain rollback errors for pending failure")
	require.ErrorIs(t, <-publicPending.done, errWebsocketFixture, "publicPending.done: public pending waiter must receive the connection cause")
	require.ErrorIs(t, <-invalidPending.done, common.ErrNotYetImplemented, "invalidPending.done: invalid pending waiter must receive its rollback error")
	assert.Nil(t, ex.Websocket.GetSubscription(publicSub), "GetSubscription: failed public subscribe should be rolled back")
	assert.Same(t, authPending, ex.websocketPending[authKey], "ex.websocketPending[authKey]: public failure should not consume authenticated pending operations")

	err = ex.failWebsocketPending(true, errWebsocketServer)
	require.ErrorIs(t, err, errWebsocketServer, "failWebsocketPending must retain its cause for authenticated pending failure")
	require.ErrorIs(t, <-authPending.done, errWebsocketServer, "authPending.done: authenticated pending waiter must receive the connection cause")
	assert.Empty(t, ex.websocketPending, "ex.websocketPending: all matching pending operations should be removed")
	assert.Nil(t, ex.Websocket.GetSubscription(authSub), "GetSubscription: failed authenticated subscribe should be rolled back")
}

func TestAbortWebsocketPending(t *testing.T) {
	var nilExchange *Exchange
	require.ErrorIs(t, nilExchange.abortWebsocketPending(nil, nil, nil), common.ErrNilPointer, "abortWebsocketPending must return the expected error for nil abort inputs")

	ex := newWebsocketHandlerTestExchange(t)
	connection := new(websocketConnectionFixture)
	sub := &subscription.Subscription{Channel: subscription.AllTradesChannel, QualifiedChannel: "abort-owned"}
	require.NoError(t, ex.Websocket.AddSubscriptions(connection, sub), "AddSubscriptions: preparing owned pending abort must not error")
	key := websocketPendingKey{subscription: websocketSubscription{Type: wsChannelTrades, Coin: "BTC"}}
	pending := &websocketPendingOperation{
		method:       wsMethodSubscribe,
		connection:   connection,
		subscription: sub,
		done:         make(chan error, 1),
	}
	ex.websocketPending[key] = pending
	err := ex.abortWebsocketPending(&key, pending, errWebsocketFixture)
	require.ErrorIs(t, err, errWebsocketFixture, "abortWebsocketPending must retain its cause for owned pending abort")
	assert.NotContains(t, ex.websocketPending, key, "ex.websocketPending: owned pending abort should remove the operation")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "GetSubscription: owned pending abort should roll back the subscription")

	completed := &websocketPendingOperation{done: make(chan error, 1)}
	completed.done <- errWebsocketServer
	err = ex.abortWebsocketPending(&key, completed, errWebsocketFixture)
	require.ErrorIs(t, err, errWebsocketServer, "abortWebsocketPending must return its authoritative concurrent result for completed pending abort")
	assert.NotErrorIs(t, err, errWebsocketFixture, "abortWebsocketPending: completed pending abort should discard a losing local cause")
}

func TestWebsocketHandleTicker(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	require.Error(t, ex.websocketHandleTicker(t.Context(), []byte(`invalid`), asset.Spot), "websocketHandleTicker must error for invalid ticker update")

	perpetual := `{"coin":"BTC","ctx":{"openInterest":"10","prevDayPx":"90","dayNtlVlm":"1000","oraclePx":"99","markPx":"101","midPx":"100","dayBaseVlm":"5"}}`
	require.NoError(t, ex.websocketHandleTicker(t.Context(), []byte(perpetual), asset.PerpetualContract), "websocketHandleTicker must not error for valid perpetual ticker")
	perpetualTicker, ok := receiveWebsocketData(t, ex).(*ticker.Price)
	require.True(t, ok, "ok: perpetual ticker must relay the expected type")
	assert.Equal(t, 100.0, perpetualTicker.Last, "perpetualTicker.Last: perpetual midpoint should be used as last")
	assert.Equal(t, 10.0, perpetualTicker.OpenInterest, "perpetualTicker.OpenInterest: perpetual open interest should be decoded")
	assert.Equal(t, 99.0, perpetualTicker.IndexPrice, "perpetualTicker.IndexPrice: perpetual oracle price should be used as index")

	spot := `{"coin":"@107","ctx":{"prevDayPx":"9","dayNtlVlm":"100","markPx":"10","midPx":"0","dayBaseVlm":"5"}}`
	require.NoError(t, ex.websocketHandleTicker(t.Context(), []byte(spot), asset.Spot), "websocketHandleTicker must not error for valid spot ticker")
	spotTicker, ok := receiveWebsocketData(t, ex).(*ticker.Price)
	require.True(t, ok, "ok: spot ticker must relay the expected type")
	assert.Equal(t, 10.0, spotTicker.Last, "spotTicker.Last: spot mark price should be the zero-midpoint fallback")
	assert.Equal(t, 5.0, spotTicker.Volume, "spotTicker.Volume: spot base volume should be decoded")

	require.ErrorIs(t, ex.websocketHandleTicker(t.Context(), []byte(perpetual), asset.Spot), errWebsocketAssetMismatch, "websocketHandleTicker must return the expected error for ticker channel asset mismatch")
	require.Error(t, ex.websocketHandleTicker(t.Context(), []byte(`{"coin":"BTC","ctx":"bad"}`), asset.PerpetualContract), "websocketHandleTicker must error for invalid perpetual ticker context")
	require.Error(t, ex.websocketHandleTicker(t.Context(), []byte(`{"coin":"@107","ctx":"bad"}`), asset.Spot), "websocketHandleTicker must error for invalid spot ticker context")
	require.ErrorIs(t, ex.websocketHandleTicker(t.Context(), []byte(`{"coin":"MISSING","ctx":{}}`), asset.Spot), errPairMappingNotFound, "websocketHandleTicker must return the expected error for unknown ticker market")

	perpetualFallback := `{"coin":"BTC","ctx":{"prevDayPx":"99","dayNtlVlm":"1000","oraclePx":"100","markPx":"101","midPx":"0","dayBaseVlm":"10"}}`
	require.NoError(t, ex.websocketHandleTicker(t.Context(), []byte(perpetualFallback), asset.PerpetualContract), "websocketHandleTicker must not error for perpetual ticker without a midpoint")
	fallbackTicker, ok := receiveWebsocketData(t, ex).(*ticker.Price)
	require.True(t, ok, "ok: fallback ticker must relay the expected type")
	assert.Equal(t, 101.0, fallbackTicker.Last, "fallbackTicker.Last: perpetual ticker should fall back to the mark price")
}

func TestWebsocketHandleOrderbook(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	require.Error(t, ex.websocketHandleOrderbook(t.Context(), []byte(`invalid`)), "websocketHandleOrderbook must error for invalid orderbook update")
	require.ErrorIs(t, ex.websocketHandleOrderbook(t.Context(), []byte(`{"coin":"BTC","levels":[],"time":1700000000000}`)), errInvalidBookLevelCount, "websocketHandleOrderbook must return the expected error for invalid orderbook side count")
	require.ErrorIs(t, ex.websocketHandleOrderbook(t.Context(), []byte(`{"coin":"MISSING","levels":[[],[]],"time":1700000000000}`)), errPairMappingNotFound, "websocketHandleOrderbook must return the expected error for unknown orderbook market")

	raw := `{"coin":"BTC","levels":[[{"px":"100","sz":"2","n":1}],[{"px":"101","sz":"3","n":1}]],"time":1700000000000}`
	require.NoError(t, ex.websocketHandleOrderbook(t.Context(), []byte(raw)), "websocketHandleOrderbook must not error for valid orderbook snapshot")
	_, ok := receiveWebsocketData(t, ex).(*orderbook.Depth)
	require.True(t, ok, "ok: orderbook snapshot must relay a depth")
	book, err := ex.Websocket.Orderbook.GetOrderbook(testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetOrderbook: getting stored websocket orderbook must not error")
	require.Len(t, book.Bids, 1, "book.Bids: stored orderbook must contain one bid")
	require.Len(t, book.Asks, 1, "book.Asks: stored orderbook must contain one ask")
	assert.Equal(t, 100.0, book.Bids[0].Price, "book.Bids[0].Price: stored bid price should match")

	require.Error(t, ex.websocketHandleOrderbook(t.Context(), []byte(`{"coin":"@107","levels":[[{"px":"0","sz":"1"}],[{"px":"1","sz":"1"}]],"time":1700000000000}`)), "websocketHandleOrderbook must return validation error for invalid orderbook snapshot")
}

func TestWebsocketHandleTrades(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	require.Error(t, ex.websocketHandleTrades(t.Context(), []byte(`invalid`)), "websocketHandleTrades must error for invalid trade update")
	require.ErrorIs(t, ex.websocketHandleTrades(t.Context(), []byte(`[{"coin":"MISSING","side":"B"}]`)), errPairMappingNotFound, "websocketHandleTrades must return the expected error for unknown trade market")
	require.ErrorIs(t, ex.websocketHandleTrades(t.Context(), []byte(`[{"coin":"BTC","side":"X"}]`)), order.ErrSideIsInvalid, "websocketHandleTrades must return the expected error for invalid trade side")
	require.NoError(t, ex.websocketHandleTrades(t.Context(), []byte(`[]`)), "websocketHandleTrades must not error for empty trade update")

	raw := `[{"coin":"BTC","side":"A","px":"100","sz":"2","time":1700000000000,"tid":7},{"coin":"@107","side":"B","px":"10","sz":"3","time":1700000001000,"tid":8}]`
	require.NoError(t, ex.websocketHandleTrades(t.Context(), []byte(raw)), "websocketHandleTrades must not error for valid trade update")
	trades, ok := receiveWebsocketData(t, ex).([]trade.Data)
	require.True(t, ok, "ok: trade update must relay the expected type")
	require.Len(t, trades, 2, "trades: both trades must be relayed")
	assert.Equal(t, order.Sell, trades[0].Side, "trades[0].Side: ask trade should be converted to sell")
	assert.Equal(t, order.Buy, trades[1].Side, "trades[1].Side: bid trade should be converted to buy")
	assert.Equal(t, "8", trades[1].TID, "trades[1].TID: trade ID should be converted")
}

func TestWebsocketHandleCandle(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	require.Error(t, ex.websocketHandleCandle(t.Context(), []byte(`invalid`)), "websocketHandleCandle must error for invalid candle update")
	require.ErrorIs(t, ex.websocketHandleCandle(t.Context(), []byte(`{"s":"MISSING","i":"1m"}`)), errPairMappingNotFound, "websocketHandleCandle must return the expected error for unknown candle market")
	require.ErrorIs(t, ex.websocketHandleCandle(t.Context(), []byte(`{"s":"BTC","i":"bad"}`)), kline.ErrUnsupportedInterval, "websocketHandleCandle must return the expected error for unsupported candle interval")

	raw := `{"t":1700000000000,"T":1700000059999,"s":"BTC","i":"1m","o":"100","c":"101","h":"102","l":"99","v":"5","n":3}`
	require.NoError(t, ex.websocketHandleCandle(t.Context(), []byte(raw)), "websocketHandleCandle must not error for valid candle update")
	item, ok := receiveWebsocketData(t, ex).(kline.Item)
	require.True(t, ok, "ok: candle update must relay the expected type")
	assert.Equal(t, kline.OneMin, item.Interval, "item.Interval: candle interval should be converted")
	require.Len(t, item.Candles, 1, "item.Candles: candle update must contain one candle")
	assert.Equal(t, 101.0, item.Candles[0].Close, "item.Candles[0].Close: candle close should be decoded")
}

func TestWebsocketHandleOrderUpdates(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	require.Error(t, ex.websocketHandleOrderUpdates(t.Context(), []byte(`invalid`)), "websocketHandleOrderUpdates must error for invalid order update")
	require.ErrorIs(t, ex.websocketHandleOrderUpdates(t.Context(), []byte(`[{"order":{"coin":"MISSING","side":"B","timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000}]`)), errPairMappingNotFound, "websocketHandleOrderUpdates must return the expected error for invalid order update conversion")
	require.ErrorIs(t, ex.websocketHandleOrderUpdates(t.Context(), []byte(`[{"order":{"coin":"BTC","side":"X","timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000}]`)), order.ErrSideIsInvalid, "websocketHandleOrderUpdates must return its conversion error for invalid mapped order update")

	raw := `[{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000}]`
	require.NoError(t, ex.websocketHandleOrderUpdates(t.Context(), []byte(raw)), "websocketHandleOrderUpdates must not error for valid order update")
	orders, ok := receiveWebsocketData(t, ex).([]order.Detail)
	require.True(t, ok, "ok: order update must relay the expected type")
	require.Len(t, orders, 1, "orders: one order update must be relayed")
	assert.Equal(t, "7", orders[0].OrderID, "orders[0].OrderID: order update ID should be converted")

	mixed := `[{"order":{"coin":"MISSING","side":"B","timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000},` + raw[1:]
	err := ex.websocketHandleOrderUpdates(t.Context(), []byte(mixed))
	require.ErrorIs(t, err, errPairMappingNotFound, "websocketHandleOrderUpdates must return the invalid entry error for mixed order-update batch")
	orders, ok = receiveWebsocketData(t, ex).([]order.Detail)
	require.True(t, ok, "ok: mixed order-update batch must relay valid entries")
	require.Len(t, orders, 1, "orders: mixed order-update batch must retain its valid entry")
	assert.Equal(t, "7", orders[0].OrderID, "orders[0].OrderID: mixed order-update batch should retain the valid order")
}

func TestWebsocketHandleUserFills(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	ex.SetFillsFeedStatus(false)
	require.NoError(t, ex.websocketHandleUserFills(t.Context(), []byte(`invalid`)), "websocketHandleUserFills must ignore updates for disabled fills feed")

	ex.SetFillsFeedStatus(true)
	ex.Websocket.Fills.Setup(true, ex.Websocket.DataHandler)
	require.Error(t, ex.websocketHandleUserFills(t.Context(), []byte(`invalid`)), "websocketHandleUserFills must error for invalid fill update")
	require.NoError(t, ex.websocketHandleUserFills(t.Context(), []byte(`{"isSnapshot":true,"fills":[]}`)), "websocketHandleUserFills must be ignored for fill snapshot")
	require.ErrorIs(t, ex.websocketHandleUserFills(t.Context(), []byte(`{"fills":[{"coin":"MISSING","side":"B"}]}`)), errPairMappingNotFound, "websocketHandleUserFills must return the expected error for unknown fill market")
	require.ErrorIs(t, ex.websocketHandleUserFills(t.Context(), []byte(`{"fills":[{"coin":"BTC","side":"X"}]}`)), order.ErrSideIsInvalid, "websocketHandleUserFills must return the expected error for invalid fill side")

	raw := `{"isSnapshot":false,"user":"` + officialSigningAddress + `","fills":[{"coin":"BTC","px":"100","sz":"2","side":"A","time":1700000000000,"hash":"0x1","oid":7,"tid":8},{"coin":"@107","px":"10","sz":"3","side":"B","time":1700000001000,"hash":"0x1","oid":9,"tid":10,"cloid":"` + validClientOrderID + `"}]}`
	require.NoError(t, ex.websocketHandleUserFills(t.Context(), []byte(raw)), "websocketHandleUserFills must not error for valid fill update")
	fills, ok := receiveWebsocketData(t, ex).([]fill.Data)
	require.True(t, ok, "ok: fill update must relay the expected type")
	require.Len(t, fills, 2, "fills: both fills must be relayed")
	assert.Equal(t, order.Sell, fills[0].Side, "fills[0].Side: ask fill should be converted to sell")
	assert.Equal(t, order.Buy, fills[1].Side, "fills[1].Side: bid fill should be converted to buy")
	assert.Equal(t, validClientOrderID, fills[1].ClientOrderID, "fills[1].ClientOrderID: fill client order ID should be retained")
	assert.Equal(t, "10", fills[1].TradeID, "fills[1].TradeID: fill trade ID should be converted")
	assert.Equal(t, "8", fills[0].ID, "fills[0].ID: fill ID should use Hyperliquid's unique trade ID")
	assert.Equal(t, "10", fills[1].ID, "fills[1].ID: fills sharing a transaction hash should retain distinct IDs")

	mixed := `{"fills":[{"coin":"BTC","side":"X"},{"coin":"@107","px":"10","sz":"3","side":"B","time":1700000001000,"hash":"0x1","oid":9,"tid":10}]}`
	err := ex.websocketHandleUserFills(t.Context(), []byte(mixed))
	require.ErrorIs(t, err, order.ErrSideIsInvalid, "websocketHandleUserFills must return the invalid entry error for mixed fill batch")
	fills, ok = receiveWebsocketData(t, ex).([]fill.Data)
	require.True(t, ok, "ok: mixed fill batch must relay valid entries")
	require.Len(t, fills, 1, "fills: mixed fill batch must retain its valid entry")
	assert.Equal(t, "10", fills[0].TradeID, "fills[0].TradeID: mixed fill batch should retain the valid fill")

	require.NoError(t, ex.websocketHandleUserFills(t.Context(), []byte(`{"fills":[]}`)), "websocketHandleUserFills must not error for empty fill update")
}

func TestParseWebsocketInterval(t *testing.T) {
	for _, interval := range []kline.Interval{
		kline.OneMin,
		kline.ThreeMin,
		kline.FiveMin,
		kline.FifteenMin,
		kline.ThirtyMin,
		kline.OneHour,
		kline.TwoHour,
		kline.FourHour,
		kline.EightHour,
		kline.TwelveHour,
		kline.OneDay,
		kline.ThreeDay,
		kline.OneWeek,
		kline.OneMonth,
	} {
		formatted, err := formatInterval(interval)
		require.NoError(t, err, "formatInterval must not error for websocket interval fixture")
		result, err := parseWebsocketInterval(formatted)
		require.NoError(t, err, "parseWebsocketInterval must not error for supported websocket interval")
		assert.Equal(t, interval, result, "result: parsed websocket interval should round trip")
	}
	_, err := parseWebsocketInterval("bad")
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "parseWebsocketInterval must return the expected error for unsupported websocket interval")
}

func TestWebsocketSubscriptionTemplates(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	name, err := websocketChannelName(&subscription.Subscription{Channel: subscription.OrderbookChannel})
	require.NoError(t, err, "websocketChannelName must not error for a supported channel name")
	assert.Equal(t, wsChannelOrderbook, name, "name: channel name should map to Hyperliquid")
	_, err = websocketChannelName(nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "websocketChannelName must return the expected error for a nil channel")
	_, err = websocketChannelName(&subscription.Subscription{Channel: "unsupported"})
	require.ErrorIs(t, err, subscription.ErrNotSupported, "websocketChannelName must return the expected error for an unsupported channel")

	template, err := ex.GetSubscriptionTemplate(nil)
	require.NoError(t, err, "GetSubscriptionTemplate must not error for subscription template")
	assert.NotNil(t, template, "template: subscription template should be returned")

	subscriptions, err := ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error for default subscriptions")
	assert.NotEmpty(t, subscriptions, "subscriptions: default subscriptions should expand")
}

func TestWebsocketSubscriptionPayload(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	_, err := ex.websocketSubscriptionPayload(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "websocketSubscriptionPayload must return the expected error for nil subscription")
	_, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: "unsupported"})
	require.ErrorIs(t, err, subscription.ErrNotSupported, "websocketSubscriptionPayload must return the expected error for unsupported subscription channel")

	_, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.TickerChannel, Authenticated: true})
	require.ErrorIs(t, err, subscription.ErrNotSupported, "websocketSubscriptionPayload must return the expected error for authenticated public channel")
	_, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.MyOrdersChannel})
	require.ErrorIs(t, err, subscription.ErrNotSupported, "websocketSubscriptionPayload must return the expected error for unauthenticated account channel")

	payload, err := ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true})
	require.NoError(t, err, "websocketSubscriptionPayload must not error for address-scoped order subscription")
	assert.Equal(t, wsChannelOrderUpdates, payload.Type, "payload.Type: order subscription type should match")
	assert.Equal(t, officialSigningAddress, payload.User, "payload.User: order subscription should include configured address")

	payload, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.MyTradesChannel, Authenticated: true})
	require.NoError(t, err, "websocketSubscriptionPayload must not error for address-scoped fill subscription")
	assert.Equal(t, wsChannelUserFills, payload.Type, "payload.Type: fill subscription type should match")

	missingCredentials := newWebsocketHandlerTestExchange(t)
	missingCredentials.SetCredentials(nil)
	_, err = missingCredentials.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true})
	require.Error(t, err, "websocketSubscriptionPayload must error for address-scoped subscription without credentials")

	_, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract})
	require.ErrorIs(t, err, subscription.ErrNotSinglePair, "websocketSubscriptionPayload must return the expected error for public subscription without one pair")
	_, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{testPerpetualPair, testPerpetualPair}})
	require.ErrorIs(t, err, subscription.ErrNotSinglePair, "websocketSubscriptionPayload must return the expected error for public subscription with multiple pairs")
	_, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{currency.NewPair(currency.ETH, currency.USDC)}})
	require.ErrorIs(t, err, errPairMappingNotFound, "websocketSubscriptionPayload must return the expected error for unknown subscription pair")

	payload, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{testPerpetualPair}})
	require.NoError(t, err, "websocketSubscriptionPayload must not error for perpetual ticker subscription")
	assert.Equal(t, wsChannelActiveAssetContext, payload.Type, "payload.Type: perpetual ticker should use activeAssetCtx")
	assert.Equal(t, "BTC", payload.Coin, "payload.Coin: perpetual ticker should use API coin")

	payload, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.Spot, Pairs: currency.Pairs{testSpotPair}})
	require.NoError(t, err, "websocketSubscriptionPayload must not error for spot ticker subscription")
	assert.Equal(t, wsChannelActiveAssetContext, payload.Type, "payload.Type: spot ticker should subscribe through activeAssetCtx")

	payload, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.CandlesChannel, Asset: asset.Spot, Pairs: currency.Pairs{testSpotPair}, Interval: kline.OneMin})
	require.NoError(t, err, "websocketSubscriptionPayload must not error for candle subscription")
	assert.Equal(t, "1m", payload.Interval, "payload.Interval: candle interval should be formatted")
	_, err = ex.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.CandlesChannel, Asset: asset.Spot, Pairs: currency.Pairs{testSpotPair}, Interval: kline.Interval(42)})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "websocketSubscriptionPayload must return the expected error for unsupported candle subscription interval")
}

func TestManageWebsocketSubscription(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	connection := new(websocketConnectionFixture)
	authConnection := new(websocketConnectionFixture)
	ex.Websocket.Conn = connection
	ex.Websocket.AuthConn = authConnection
	installWebsocketAcknowledgements(t, ex, connection, false)
	installWebsocketAcknowledgements(t, ex, authConnection, true)

	require.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, nil), common.ErrNilPointer, "manageWebsocketSubscription must return the expected error for nil managed subscription")
	sub := &subscription.Subscription{
		Channel:          subscription.OrderbookChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "l2Book:perpetualcontract:BTC-USDC",
	}
	require.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), "invalid", sub), common.ErrNotYetImplemented, "manageWebsocketSubscription must return the expected error for unsupported subscription method")
	require.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, &subscription.Subscription{Channel: "unsupported"}), subscription.ErrNotSupported, "manageWebsocketSubscription must return the expected error for unsupported managed subscription")

	ex.Websocket.Conn = nil
	require.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, sub), common.ErrNilPointer, "manageWebsocketSubscription must return the expected error for nil public connection")
	ex.Websocket.Conn = connection

	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, sub), "manageWebsocketSubscription must not error for subscribing one public channel")
	assert.Equal(t, subscription.SubscribedState, sub.State(), "State: successful subscription should be marked subscribed")
	assert.NotNil(t, ex.Websocket.GetSubscription(sub.Clone()), "GetSubscription: equivalent subscription should match the stored exact key")
	require.Len(t, connection.sentRequests(), 1, "sentRequests: one subscribe request must be sent")
	assert.Equal(t, wsMethodSubscribe, connection.sentRequests()[0].Method, "Subscribe request should use the subscribe method")
	require.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, sub), subscription.ErrDuplicate, "manageWebsocketSubscription must return the expected error for duplicate subscription")

	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, sub), "manageWebsocketSubscription must not error for unsubscribing one public channel")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "GetSubscription: unsubscribed channel should be removed")
	assert.Equal(t, wsMethodUnsubscribe, connection.sentRequests()[1].Method, "Unsubscribe request should use the unsubscribe method")

	acknowledgedThenSendFailed := &subscription.Subscription{
		Channel:          subscription.AllTradesChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "trades:acknowledged-before-send-error",
	}
	connection.sendErr = errWebsocketFixture
	connection.sendHook = func(req websocketRequest, _ error) {
		require.NoError(t, ex.websocketHandleDataForConnection(t.Context(), websocketAcknowledgement(t, &req), false), "websocketHandleDataForConnection must not error for handling a concurrent acknowledgement")
	}
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, acknowledgedThenSendFailed), "manageWebsocketSubscription must remain authoritative for an acknowledgement committed before a send error")
	assert.Equal(t, subscription.SubscribedState, acknowledgedThenSendFailed.State(), "State: acknowledged subscription should remain committed")
	assert.NotNil(t, ex.Websocket.GetSubscription(acknowledgedThenSendFailed), "GetSubscription: acknowledged subscription should remain tracked")
	connection.sendErr = nil
	installWebsocketAcknowledgements(t, ex, connection, false)
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, acknowledgedThenSendFailed), "manageWebsocketSubscription must not error for cleaning up the acknowledged subscription")

	failing := &subscription.Subscription{
		Channel:          subscription.AllTradesChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "trades:perpetualcontract:BTC-USDC",
	}
	connection.sendErr = errWebsocketFixture
	require.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, failing), errWebsocketFixture, "manageWebsocketSubscription must return subscription send failure")
	assert.Nil(t, ex.Websocket.GetSubscription(failing), "GetSubscription: failed subscription should be removed from the store")
	connection.sendErr = nil

	rollbackFailure := &subscription.Subscription{
		Channel:          subscription.AllTradesChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "trades:perpetualcontract:BTC-USDC:rollback-failure",
	}
	var preRollbackErr error
	connection.sendErr = errWebsocketFixture
	connection.sendHook = func(websocketRequest, error) {
		preRollbackErr = ex.Websocket.RemoveSubscriptions(connection, rollbackFailure)
	}
	err := ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, rollbackFailure)
	require.NoError(t, preRollbackErr, "RemoveSubscriptions: concurrent subscription removal must not error")
	require.ErrorIs(t, err, errWebsocketFixture, "manageWebsocketSubscription must remain visible when rollback also fails for subscription send failure")
	require.ErrorIs(t, err, subscription.ErrNotFound, "manageWebsocketSubscription must also be returned for subscription rollback failure")
	connection.sendHook = nil
	connection.sendErr = nil
	installWebsocketAcknowledgements(t, ex, connection, false)

	authSub := &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true, QualifiedChannel: wsChannelOrderUpdates}
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, authSub), "manageWebsocketSubscription must not error for subscribing one address-scoped channel")
	require.Len(t, authConnection.sentRequests(), 1, "sentRequests: address-scoped subscription must use the account connection")

	authConnection.sendErr = errWebsocketFixture
	require.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, authSub), errWebsocketFixture, "manageWebsocketSubscription must return unsubscription send failure")
	assert.NotNil(t, ex.Websocket.GetSubscription(authSub), "GetSubscription: failed unsubscription should remain tracked")
	authConnection.sendErr = nil
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, authSub), "manageWebsocketSubscription must not error for retrying address-scoped unsubscription")

	stateConflict := &subscription.Subscription{
		Channel:          subscription.AllTradesChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "trades:state-conflict",
	}
	require.NoError(t, stateConflict.SetState(subscription.UnsubscribingState), "SetState: preparing unsubscribe state conflict must not error")
	require.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, stateConflict), subscription.ErrInStateAlready, "manageWebsocketSubscription must fail closed for duplicate unsubscribe state transition")

	collision := &subscription.Subscription{
		Channel:          subscription.CandlesChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		Interval:         kline.OneMin,
		QualifiedChannel: "candle:pending-collision",
	}
	collisionPayload, err := ex.websocketSubscriptionPayload(t.Context(), collision)
	require.NoError(t, err, "websocketSubscriptionPayload must not error for collision payload")
	collisionKey := websocketPendingKey{subscription: collisionPayload}
	ex.websocketPending[collisionKey] = &websocketPendingOperation{done: make(chan error, 1)}
	err = ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, collision)
	require.ErrorIs(t, err, errWebsocketSubscription, "manageWebsocketSubscription must fail closed for crossed operation for the same exact subscription")
	assert.Nil(t, ex.Websocket.GetSubscription(collision), "GetSubscription: rejected crossed operation should roll back its local subscription state")
	delete(ex.websocketPending, collisionKey)

	ex.websocketPending = nil
	nilMapSub := &subscription.Subscription{
		Channel:          subscription.TickerChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "ticker:nil-pending-map",
	}
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, nilMapSub), "manageWebsocketSubscription must initialise an absent pending-operation map for subscription")
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, nilMapSub), "manageWebsocketSubscription must support unsubscribe for initialised pending-operation map")

	quietConnection := new(websocketConnectionFixture)
	ex.Websocket.Conn = quietConnection
	ex.WebsocketResponseMaxLimit = time.Millisecond
	timeoutSub := &subscription.Subscription{
		Channel:          subscription.AllTradesChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "trades:timeout",
	}
	err = ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, timeoutSub)
	require.ErrorIs(t, err, ws.ErrSignatureTimeout, "manageWebsocketSubscription must time out for missing subscription acknowledgement")
	assert.Nil(t, ex.Websocket.GetSubscription(timeoutSub), "GetSubscription: timed-out subscription should be rolled back")

	ex.WebsocketResponseMaxLimit = 0
	cancelledContext, cancel := context.WithCancel(t.Context())
	cancel()
	cancelledSub := &subscription.Subscription{
		Channel:          subscription.OrderbookChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "l2Book:cancelled",
	}
	err = ex.manageWebsocketSubscription(cancelledContext, wsMethodSubscribe, cancelledSub)
	require.ErrorIs(t, err, context.Canceled, "manageWebsocketSubscription must return cancelled subscription context")
	assert.Nil(t, ex.Websocket.GetSubscription(cancelledSub), "GetSubscription: cancelled subscription should be rolled back")
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	ex := newWebsocketHandlerTestExchange(t)
	connection := new(websocketConnectionFixture)
	ex.Websocket.Conn = connection
	installWebsocketAcknowledgements(t, ex, connection, false)
	sub := &subscription.Subscription{
		Channel:          subscription.OrderbookChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		QualifiedChannel: "l2Book:perpetualcontract:BTC-USDC",
	}
	require.NoError(t, ex.Subscribe(subscription.List{sub}), "Subscribe must not error for subscribing through wrapper")
	require.NotEmpty(t, connection.sentRequests(), "sentRequests: wrapper subscribe must send a request")
	require.NoError(t, ex.Unsubscribe(subscription.List{sub}), "Unsubscribe must not error for unsubscribing through wrapper")

	missing := &subscription.Subscription{
		Channel:          subscription.CandlesChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{testPerpetualPair},
		Interval:         kline.OneMin,
		QualifiedChannel: "candle:perpetualcontract:BTC-USDC:1m",
	}
	require.ErrorIs(t, ex.Unsubscribe(subscription.List{missing}), subscription.ErrNotFound, "Unsubscribe must return the expected error for unsubscribing an unknown channel")
	require.Error(t, ex.Subscribe(subscription.List{nil}), "Subscribe must error for nil subscription expansion")
	require.ErrorIs(t, ex.Unsubscribe(subscription.List{nil}), common.ErrNilPointer, "Unsubscribe must return the expected error for nil unsubscription")
}
