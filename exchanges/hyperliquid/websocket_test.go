package hyperliquid

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
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
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fill"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var errWebsocketFixture = errors.New("websocket fixture failure")

// websocketConnectionFixture stands in for a websocket connection: it records each request it sends, has the exchange
// handle the frames reply answers a request with as if they were read from the connection, and serves queued messages
// to a reader
type websocketConnectionFixture struct {
	websocket.Connection

	exchange      *Exchange
	authenticated bool

	mu      sync.Mutex
	dialErr error
	sendErr error
	reply   func(request []byte) []string
	sent    []string
	// handleErrs holds the exchange's result of handling each reply frame
	handleErrs []error
	messages   []websocket.Response
	ping       websocket.PingHandler

	dialCalls atomic.Int32
	readCalls atomic.Int32
	pingCalls atomic.Int32
}

func (f *websocketConnectionFixture) Dial(context.Context, *gws.Dialer, http.Header, url.Values) error {
	f.dialCalls.Add(1)
	return f.dialErr
}

func (f *websocketConnectionFixture) ReadMessage() websocket.Response {
	f.readCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) == 0 {
		return websocket.Response{}
	}
	message := f.messages[0]
	f.messages = f.messages[1:]
	return message
}

func (f *websocketConnectionFixture) SetupPingHandler(_ request.EndpointLimit, handler websocket.PingHandler) {
	f.pingCalls.Add(1)
	f.mu.Lock()
	f.ping = handler
	f.mu.Unlock()
}

// SendJSONMessage records the request and handles its reply frames before returning sendErr, so a reply can settle an
// operation whose send then reports a failure
func (f *websocketConnectionFixture) SendJSONMessage(ctx context.Context, _ request.EndpointLimit, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.sent = append(f.sent, string(encoded))
	reply, sendErr := f.reply, f.sendErr
	f.mu.Unlock()
	if reply != nil {
		for _, frame := range reply(encoded) {
			handleErr := f.exchange.wsHandleData(ctx, []byte(frame), f.authenticated)
			f.mu.Lock()
			f.handleErrs = append(f.handleErrs, handleErr)
			f.mu.Unlock()
		}
	}
	return sendErr
}

// SendMessageReturnResponse sends a request and returns the response the match system received while its reply was
// handled, or times out at once, as the reply has then already been handled
func (f *websocketConnectionFixture) SendMessageReturnResponse(ctx context.Context, epl request.EndpointLimit, signature, payload any) ([]byte, error) {
	responses, err := f.exchange.Websocket.Match.Set(signature, 1)
	if err != nil {
		return nil, err
	}
	if err := f.SendJSONMessage(ctx, epl, payload); err != nil {
		f.exchange.Websocket.Match.RemoveSignature(signature)
		return nil, err
	}
	select {
	case response := <-responses:
		return response, nil
	default:
		f.exchange.Websocket.Match.RemoveSignature(signature)
		return nil, websocket.ErrSignatureTimeout
	}
}

func (f *websocketConnectionFixture) sentRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

func (f *websocketConnectionFixture) handled() []error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]error(nil), f.handleErrs...)
}

// websocketCanonicalDefaults contains the options Hyperliquid adds to the subscriptions it echoes, by subscription type
var websocketCanonicalDefaults = map[string]map[string]any{
	wsChannelOrderbook:          {"nSigFigs": nil, "mantissa": nil, "fast": false},
	wsChannelUserFills:          {"aggregateByTime": false},
	wsChannelSpotState:          {"ignorePortfolioMargin": false},
	wsChannelClearinghouseState: {"dex": ""},
	wsChannelOpenOrders:         {"dex": ""},
	wsChannelTWAPStates:         {"dex": ""},
}

// acknowledge answers a subscription request as Hyperliquid does, echoing its subscription in the canonical form, with
// the user lower case and the defaults of absent options present; it does not answer other requests
func acknowledge(sent []byte) []string {
	var req struct {
		Method       string         `json:"method"`
		Subscription map[string]any `json:"subscription"`
	}
	if err := json.Unmarshal(sent, &req); err != nil || req.Subscription == nil {
		return nil
	}
	canonical := req.Subscription
	if subscriptionType, ok := canonical["type"].(string); ok {
		for option, value := range websocketCanonicalDefaults[subscriptionType] {
			if _, ok := canonical[option]; !ok {
				canonical[option] = value
			}
		}
	}
	if user, ok := canonical["user"].(string); ok {
		canonical["user"] = strings.ToLower(user)
	}
	frame, err := json.Marshal(map[string]any{
		"channel": wsChannelSubscriptionResponse,
		"data":    map[string]any{"method": req.Method, "subscription": canonical},
	})
	if err != nil {
		return nil
	}
	return []string{string(frame)}
}

// newWebsocketTestExchange returns a trading server exchange with its markets enabled, the trade and fills feeds
// enabled, account feeds usable, and fixture connections that acknowledge every subscription request
func newWebsocketTestExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := newTradingServerExchange(t, nil, nil)
	setTestPairs(t, ex, asset.PerpetualContract, testPerpetualMapping, testBuilderMapping)
	setTestPairs(t, ex, asset.Spot, testSpotMapping)
	ex.Websocket.Trade.Setup(true, ex.Websocket.DataHandler)
	ex.SetFillsFeedStatus(true)
	ex.Websocket.Fills.Setup(true, ex.Websocket.DataHandler)
	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
	ex.Websocket.Conn = &websocketConnectionFixture{exchange: ex, reply: acknowledge}
	ex.Websocket.AuthConn = &websocketConnectionFixture{exchange: ex, authenticated: true, reply: acknowledge}
	return ex
}

// websocketFixture returns a connection's fixture
func websocketFixture(t *testing.T, connection websocket.Connection) *websocketConnectionFixture {
	t.Helper()
	fixture, ok := connection.(*websocketConnectionFixture)
	require.True(t, ok, "connection must be a websocketConnectionFixture")
	return fixture
}

// receiveWebsocketData returns the next relayed message; handlers relay synchronously, so it must already be waiting
func receiveWebsocketData(t *testing.T, ex *Exchange) any {
	t.Helper()
	select {
	case payload := <-ex.Websocket.DataHandler.C:
		return payload.Data
	default:
		require.FailNow(t, "a relayed message must be waiting")
		return nil
	}
}

// assertNoWebsocketData checks nothing more was relayed
func assertNoWebsocketData(t *testing.T, ex *Exchange) {
	t.Helper()
	select {
	case payload := <-ex.Websocket.DataHandler.C:
		assert.Failf(t, "nothing should be relayed", "relayed %T: %v", payload.Data, payload.Data)
	default:
	}
}

// addWebsocketPending prepares a subscription as manageWebsocketSubscription does before sending its request, and
// registers the operation awaiting its acknowledgement under key
func addWebsocketPending(t *testing.T, ex *Exchange, method string, sub *subscription.Subscription, key *websocketPendingKey) *websocketPendingOperation {
	t.Helper()
	connection := ex.Websocket.Conn
	if key.authenticated {
		connection = ex.Websocket.AuthConn
	}
	previousState := sub.State()
	if method == wsMethodSubscribe {
		require.NoError(t, ex.Websocket.AddSubscriptions(connection, sub), "AddSubscriptions must not error")
	} else {
		require.NoError(t, sub.SetState(subscription.UnsubscribingState), "SetState must not error")
	}
	pending := &websocketPendingOperation{
		method:        method,
		connection:    connection,
		subscription:  sub,
		previousState: previousState,
		done:          make(chan error, 1),
	}
	registerWebsocketPending(ex, key, pending)
	return pending
}

// registerWebsocketPending registers an operation under key without preparing its subscription
func registerWebsocketPending(ex *Exchange, key *websocketPendingKey, pending *websocketPendingOperation) {
	ex.websocketPendingMu.Lock()
	if ex.websocketPending == nil {
		ex.websocketPending = make(map[websocketPendingKey]*websocketPendingOperation)
	}
	ex.websocketPending[*key] = pending
	ex.websocketPendingMu.Unlock()
}

// pendingWebsocketOperation returns the operation pending under key, if any
func pendingWebsocketOperation(ex *Exchange, key *websocketPendingKey) *websocketPendingOperation {
	ex.websocketPendingMu.Lock()
	defer ex.websocketPendingMu.Unlock()
	return ex.websocketPending[*key]
}

// newSubscribedWebsocketSubscription returns a subscription stored as subscribed on the public connection
func newSubscribedWebsocketSubscription(t *testing.T, ex *Exchange, sub *subscription.Subscription) *subscription.Subscription {
	t.Helper()
	require.NoError(t, ex.Websocket.AddSuccessfulSubscriptions(ex.Websocket.Conn, sub), "AddSuccessfulSubscriptions must not error")
	return sub
}

// deflateBase64 compresses data with raw DEFLATE and encodes it in base64, as the fastAssetCtxs feed sends it
func deflateBase64(t *testing.T, data []byte) string {
	t.Helper()
	var compressed bytes.Buffer
	writer, err := flate.NewWriter(&compressed, flate.BestCompression)
	require.NoError(t, err, "flate.NewWriter must not error")
	_, err = writer.Write(data)
	require.NoError(t, err, "Write must not error")
	require.NoError(t, writer.Close(), "Close must not error")
	return base64.StdEncoding.EncodeToString(compressed.Bytes())
}

// waitForWebsocketReaders waits for the read loops, which return once their connection has no messages left
func waitForWebsocketReaders(t *testing.T, ex *Exchange) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		ex.Websocket.Wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the websocket readers must exit")
	}
}

// newWebsocketConnectTestExchange returns an exchange with the websocket enabled, the credentials set when given, and
// fixture connections
func newWebsocketConnectTestExchange(t *testing.T, credentials *accounts.Credentials) (ex *Exchange, public, authenticated *websocketConnectionFixture) {
	t.Helper()
	ex = new(Exchange)
	ex.SetDefaults()
	cfg, err := ex.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error")
	cfg.Features = &config.FeaturesConfig{Enabled: config.FeaturesEnabledConfig{Websocket: true}}
	require.NoError(t, ex.Setup(cfg), "Setup must not error")
	if credentials != nil {
		setTestCredentials(ex, credentials)
	}
	public = &websocketConnectionFixture{exchange: ex}
	authenticated = &websocketConnectionFixture{exchange: ex, authenticated: true}
	ex.Websocket.Conn = public
	ex.Websocket.AuthConn = authenticated
	return ex, public, authenticated
}

func TestWsConnect(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	ex.SetDefaults()
	require.ErrorIs(t, ex.WsConnect(), websocket.ErrWebsocketNotEnabled, "WsConnect must reject a disabled websocket")

	ex, _, _ = newWebsocketConnectTestExchange(t, nil)
	ex.SetEnabled(false)
	require.ErrorIs(t, ex.WsConnect(), websocket.ErrWebsocketNotEnabled, "WsConnect must reject a disabled exchange")

	ex, public, _ := newWebsocketConnectTestExchange(t, nil)
	public.dialErr = errWebsocketFixture
	require.ErrorIs(t, ex.WsConnect(), errWebsocketFixture, "WsConnect must return the public dial error")

	ex, public, authenticated := newWebsocketConnectTestExchange(t, &accounts.Credentials{Key: testAccountAddress})
	ex.API.AuthenticatedWebsocketSupport = false
	require.NoError(t, ex.WsConnect(), "WsConnect must not error without account feed support")
	waitForWebsocketReaders(t, ex)
	assert.Equal(t, int32(1), public.readCalls.Load(), "WsConnect should start the public reader")
	assert.Zero(t, authenticated.dialCalls.Load(), "WsConnect should not dial the account connection without account feed support")

	ex, _, authenticated = newWebsocketConnectTestExchange(t, nil)
	ex.API.AuthenticatedWebsocketSupport = true
	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
	require.NoError(t, ex.WsConnect(), "WsConnect must not error without an account address")
	waitForWebsocketReaders(t, ex)
	assert.False(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "WsConnect should disable account feeds without an account address")
	assert.Zero(t, authenticated.dialCalls.Load(), "WsConnect should not dial the account connection without an account address")

	ex, public, authenticated = newWebsocketConnectTestExchange(t, &accounts.Credentials{Key: testAccountAddress})
	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
	authenticated.dialErr = errWebsocketFixture
	require.NoError(t, ex.WsConnect(), "WsConnect must keep the public connection when the account connection fails")
	waitForWebsocketReaders(t, ex)
	assert.False(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "WsConnect should disable account feeds when the account connection fails")
	assert.Equal(t, int32(1), public.readCalls.Load(), "WsConnect should start the public reader")
	assert.Zero(t, authenticated.readCalls.Load(), "WsConnect should not read a failed account connection")

	ex, public, authenticated = newWebsocketConnectTestExchange(t, &accounts.Credentials{Key: testAccountAddress})
	require.NoError(t, ex.WsConnect(), "WsConnect must not error")
	waitForWebsocketReaders(t, ex)
	assert.True(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "WsConnect should enable account feeds")
	assert.Equal(t, int32(1), public.readCalls.Load(), "WsConnect should start the public reader")
	assert.Equal(t, int32(1), authenticated.readCalls.Load(), "WsConnect should start the account reader")
}

func TestConnectWebsocket(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	require.ErrorIs(t, ex.connectWebsocket(t.Context(), nil), common.ErrNilPointer, "connectWebsocket must reject a nil connection")

	failed := &websocketConnectionFixture{dialErr: errWebsocketFixture}
	require.ErrorIs(t, ex.connectWebsocket(t.Context(), failed), errWebsocketFixture, "connectWebsocket must return the dial error")
	assert.Zero(t, failed.pingCalls.Load(), "connectWebsocket should not set up pings after a failed dial")

	connection := new(websocketConnectionFixture)
	require.NoError(t, ex.connectWebsocket(t.Context(), connection), "connectWebsocket must not error")
	assert.Equal(t, int32(1), connection.dialCalls.Load(), "connectWebsocket should dial once")
	exp := websocket.PingHandler{MessageType: gws.TextMessage, Message: []byte(`{"method":"ping"}`), Delay: websocketPingInterval}
	assert.Equal(t, exp, connection.ping, "connectWebsocket should send application pings")
}

func TestWsReadData(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	publicSub := &subscription.Subscription{Channel: wsChannelAllMids, QualifiedChannel: wsChannelAllMids}
	publicPending := addWebsocketPending(t, ex, wsMethodSubscribe, publicSub, &websocketPendingKey{subscription: WsSubscription{Type: wsChannelAllMids}})
	authenticatedKey := websocketPendingKey{authenticated: true, subscription: WsSubscription{Type: wsChannelNotification, User: testAccountAddress}}
	authenticatedPending := addWebsocketPending(t, ex, wsMethodSubscribe, &subscription.Subscription{Channel: wsChannelNotification, Authenticated: true, QualifiedChannel: wsChannelNotification}, &authenticatedKey)

	connection := &websocketConnectionFixture{exchange: ex, messages: []websocket.Response{{Raw: []byte(`{"channel":"pong"}`)}, {Raw: []byte(`invalid`)}, {}}}
	ex.Websocket.Wg.Add(1)
	ex.wsReadData(t.Context(), connection, false)
	assert.Equal(t, int32(3), connection.readCalls.Load(), "wsReadData should read until the connection closes")
	relayed, ok := receiveWebsocketData(t, ex).(error)
	require.True(t, ok, "wsReadData must relay the handler error")
	assert.Error(t, relayed, "wsReadData should relay the handler error")
	assertNoWebsocketData(t, ex)
	require.ErrorIs(t, <-publicPending.done, websocket.ErrNotConnected, "wsReadData must fail the connection's pending operations when it closes")
	assert.Nil(t, ex.Websocket.GetSubscription(publicSub), "wsReadData should roll back the connection's pending subscribe")
	assert.Same(t, authenticatedPending, pendingWebsocketOperation(ex, &authenticatedKey), "wsReadData should leave the other connection's pending operations")

	ex.Websocket.DataHandler = stream.NewRelay(1)
	require.NoError(t, ex.Websocket.DataHandler.Send(t.Context(), "filler"), "Send must not error")
	connection = &websocketConnectionFixture{exchange: ex, messages: []websocket.Response{{Raw: []byte(`invalid`)}, {Raw: []byte(`invalid`)}, {}}}
	ex.Websocket.Wg.Add(1)
	ex.wsReadData(t.Context(), connection, false)
	assert.Equal(t, int32(3), connection.readCalls.Load(), "wsReadData should keep reading when the data handler is full")
}

func TestWsHandleData(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	assert.NoError(t, ex.wsHandleData(t.Context(), []byte(websocketConnectionEstablished), false), "wsHandleData should ignore the connection greeting")
	assert.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"pong"}`), false), "wsHandleData should ignore a pong")
	assert.Error(t, ex.wsHandleData(t.Context(), []byte(`invalid`), false), "wsHandleData should reject an invalid message")
	for _, channel := range []string{
		wsChannelSubscriptionResponse, wsChannelPost, wsChannelError, wsChannelActiveAssetContext, wsChannelActiveSpotAssetContext,
		wsChannelOrderbook, wsChannelTrades, wsChannelCandle, wsChannelOrderUpdates, wsChannelUserEvents, wsChannelUserFills,
		wsChannelUserTWAPSliceFills, wsChannelUserFundings, wsChannelUserNonFundingLedgerUpdates, wsChannelUserTWAPHistory,
		wsChannelOutcomeMetaUpdates, wsChannelFastAssetContexts, wsChannelAllMids, wsChannelNotification, wsChannelWebData3,
		wsChannelTWAPStates, wsChannelClearinghouseState, wsChannelOpenOrders, wsChannelActiveAssetData, wsChannelBestBidOffer,
		wsChannelSpotState, wsChannelAllDEXsClearinghouseState, wsChannelAllDEXsAssetContexts,
	} {
		assert.Errorf(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"`+channel+`","data":"invalid!"}`), false), "wsHandleData should reject invalid %s data", channel)
	}
	assertNoWebsocketData(t, ex)

	unhandled := `{"channel":"futureChannel","data":{}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(unhandled), false), "wsHandleData must not error for an unhandled channel")
	exp := websocket.UnhandledMessageWarning{Message: ex.Name + websocket.UnhandledMessage + unhandled}
	assert.Equal(t, exp, receiveWebsocketData(t, ex), "wsHandleData should relay a warning for an unhandled channel")
}

// websocketClearinghouseStateJSON is the clearinghouse state of the websocket fixtures' vault on the default DEX
const websocketClearinghouseStateJSON = `{"marginSummary":{"accountValue":"3155203.7660619998","totalNtlPos":"4345291.8850560002","totalRawUsd":"3182148.2545679999","totalMarginUsed":"217264.594197"},"crossMarginSummary":{"accountValue":"3155203.7660619998","totalNtlPos":"4345291.8850560002","totalRawUsd":"3182148.2545679999","totalMarginUsed":"217264.594197"},"crossMaintenanceMarginUsed":"43452.918775","withdrawable":"2720674.5776","assetPositions":[{"type":"oneWay","position":{"coin":"SAGA","szi":"-103774.5","leverage":{"type":"cross","value":20},"entryPx":"0.01662","positionValue":"2527.94682","unrealizedPnl":"-802.226337","returnOnEquity":"-9.2972917098","liquidationPx":"29.713170189","marginUsed":"126.397341","maxLeverage":50,"cumFunding":{"allTime":"-3524.80395","sinceOpen":"81.316838","sinceChange":"114.927323"}}},{"type":"oneWay","position":{"coin":"SOL","szi":"-212.78","leverage":{"type":"isolated","value":10,"rawUsd":"27985.6"},"entryPx":"119.51","positionValue":"25429.33","unrealizedPnl":"-0.21","returnOnEquity":"-0.0000826","liquidationPx":"130.2","marginUsed":"2542.93","maxLeverage":20,"cumFunding":{"allTime":"-12.75","sinceOpen":"-0.32","sinceChange":"-0.32"}}}],"time":1791279140401}`

// websocketEmptyClearinghouseStateJSON is the clearinghouse state of the websocket fixtures' vault on the xyz DEX
const websocketEmptyClearinghouseStateJSON = `{"marginSummary":{"accountValue":"0.0","totalNtlPos":"0.0","totalRawUsd":"0.0","totalMarginUsed":"0.0"},"crossMarginSummary":{"accountValue":"0.0","totalNtlPos":"0.0","totalRawUsd":"0.0","totalMarginUsed":"0.0"},"crossMaintenanceMarginUsed":"0.0","withdrawable":"0.0","assetPositions":[],"time":1791280776159}`

// testWebsocketClearinghouseState is websocketClearinghouseStateJSON decoded
func testWebsocketClearinghouseState() ClearinghouseStateResponse {
	summary := MarginSummary{AccountValue: 3155203.7660619998, TotalNotionalPosition: 4345291.8850560002, TotalRawUSD: 3182148.2545679999, TotalMarginUsed: 217264.594197}
	return ClearinghouseStateResponse{
		MarginSummary:              summary,
		CrossMarginSummary:         summary,
		CrossMaintenanceMarginUsed: 43452.918775,
		Withdrawable:               2720674.5776,
		AssetPositions: []AssetPosition{
			{Type: "oneWay", Position: Position{
				Coin:              "SAGA",
				SignedSize:        -103774.5,
				Leverage:          Leverage{Type: "cross", Value: 20},
				EntryPrice:        0.01662,
				PositionValue:     2527.94682,
				UnrealisedPNL:     -802.226337,
				ReturnOnEquity:    -9.2972917098,
				LiquidationPrice:  29.713170189,
				MarginUsed:        126.397341,
				MaxLeverage:       50,
				CumulativeFunding: CumulativeFunding{AllTime: -3524.80395, SinceOpen: 81.316838, SinceChange: 114.927323},
			}},
			{Type: "oneWay", Position: Position{
				Coin:              "SOL",
				SignedSize:        -212.78,
				Leverage:          Leverage{Type: "isolated", Value: 10, RawUSD: 27985.6},
				EntryPrice:        119.51,
				PositionValue:     25429.33,
				UnrealisedPNL:     -0.21,
				ReturnOnEquity:    -0.0000826,
				LiquidationPrice:  130.2,
				MarginUsed:        2542.93,
				MaxLeverage:       20,
				CumulativeFunding: CumulativeFunding{AllTime: -12.75, SinceOpen: -0.32, SinceChange: -0.32},
			}},
		},
		Time: milli(1791279140401),
	}
}

// testWebsocketEmptyClearinghouseState is websocketEmptyClearinghouseStateJSON decoded
func testWebsocketEmptyClearinghouseState() ClearinghouseStateResponse {
	return ClearinghouseStateResponse{AssetPositions: []AssetPosition{}, Time: milli(1791280776159)}
}

func TestWsRelay(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	for _, tc := range []struct {
		name  string
		frame string
		exp   any
	}{
		{
			name:  "allMids",
			frame: `{"channel":"allMids","data":{"mids":{"BTC":"85801.5","PURR/USDC":"0.14988","@107":"92.9315","#14880":"0.111485"}}}`,
			exp:   &WsAllMids{Mids: map[string]types.Number{"BTC": 85801.5, "PURR/USDC": 0.14988, "@107": 92.9315, "#14880": 0.111485}},
		},
		{
			name:  "builder DEX allMids",
			frame: `{"channel":"allMids","data":{"dex":"xyz","mids":{"xyz:XYZ100":"31169.5"}}}`,
			exp:   &WsAllMids{DEX: "xyz", Mids: map[string]types.Number{"xyz:XYZ100": 31169.5}},
		},
		{
			name:  "notification",
			frame: `{"channel":"notification","data":{"notification":"Resting order filled: Bought 1017.6 ARB at $0.20138"}}`,
			exp:   &WsNotification{Notification: "Resting order filled: Bought 1017.6 ARB at $0.20138"},
		},
		{
			name:  "webData3",
			frame: `{"channel":"webData3","data":{"userState":{"agentAddress":"0x0000000000000000000000000000000000000008","agentValidUntil":1792311392771,"cumLedger":"-2686.97","serverTime":1791280777681,"isVault":false,"user":"0x0000000000000000000000000000000000000003","dexAbstractionEnabled":true,"abstraction":"unifiedAccount","optOutOfSpotDusting":true},"perpDexStates":[{"totalVaultEquity":"1250000.5","perpsAtOpenInterestCap":["CANTO","FTM"]},{"totalVaultEquity":"15.25","perpsAtOpenInterestCap":["xyz:XYZ100"],"leadingVaults":[{"address":"0x0000000000000000000000000000000000000009","name":"Hyperliquidity Provider (HLP)"}]}]}}`,
			exp: &WsWebData3{
				UserState: WsWebData3UserState{
					AgentAddress:          "0x0000000000000000000000000000000000000008",
					AgentValidUntil:       milli(1792311392771),
					CumulativeLedger:      -2686.97,
					ServerTime:            milli(1791280777681),
					User:                  "0x0000000000000000000000000000000000000003",
					OptOutOfSpotDusting:   true,
					DEXAbstractionEnabled: true,
					Abstraction:           AccountAbstractionUnified,
				},
				PerpetualDEXStates: []WsPerpetualDEXState{
					{TotalVaultEquity: 1250000.5, PerpetualsAtOpenInterestCap: []string{"CANTO", "FTM"}},
					{
						TotalVaultEquity:            15.25,
						PerpetualsAtOpenInterestCap: []string{"xyz:XYZ100"},
						LeadingVaults:               []WsLeadingVault{{Address: "0x0000000000000000000000000000000000000009", Name: "Hyperliquidity Provider (HLP)"}},
					},
				},
			},
		},
		{
			name:  "vault webData3",
			frame: `{"channel":"webData3","data":{"userState":{"agentAddress":null,"agentValidUntil":null,"cumLedger":"-1190433.52","serverTime":1791280775688,"isVault":true,"user":"0x0000000000000000000000000000000000000001"},"perpDexStates":[{"totalVaultEquity":"0.0","perpsAtOpenInterestCap":["CANTO","FTM"]},{"totalVaultEquity":"0.0"}]}}`,
			exp: &WsWebData3{
				UserState: WsWebData3UserState{
					CumulativeLedger: -1190433.52,
					ServerTime:       milli(1791280775688),
					IsVault:          true,
					User:             "0x0000000000000000000000000000000000000001",
				},
				PerpetualDEXStates: []WsPerpetualDEXState{{PerpetualsAtOpenInterestCap: []string{"CANTO", "FTM"}}, {}},
			},
		},
		{
			name:  "twapStates",
			frame: `{"channel":"twapStates","data":{"dex":"","user":"0x0000000000000000000000000000000000000006","states":[[2285948,{"coin":"@107","user":"0x0000000000000000000000000000000000000006","side":"A","sz":"1000.0","executedSz":"333.29","executedNtl":"31171.44245","minutes":4320,"reduceOnly":true,"randomize":true,"timestamp":1791192756084,"trigger":{"px":"90.0","above":true},"stopPx":"92.0"}]]}}`,
			exp: &WsTWAPStates{
				User: "0x0000000000000000000000000000000000000006",
				States: []TWAPStateEntry{{
					TWAPID: 2285948,
					State: TWAPState{
						Coin:             "@107",
						User:             "0x0000000000000000000000000000000000000006",
						Side:             "A",
						Size:             1000,
						ExecutedSize:     333.29,
						ExecutedNotional: 31171.44245,
						Minutes:          4320,
						ReduceOnly:       true,
						Randomise:        true,
						Timestamp:        milli(1791192756084),
						Trigger:          &TWAPTrigger{Price: 90, Above: true},
						StopPrice:        92,
					},
				}},
			},
		},
		{
			name:  "builder DEX twapStates",
			frame: `{"channel":"twapStates","data":{"dex":"xyz","user":"0x0000000000000000000000000000000000000003","states":[]}}`,
			exp:   &WsTWAPStates{DEX: "xyz", User: "0x0000000000000000000000000000000000000003", States: []TWAPStateEntry{}},
		},
		{
			name:  "clearinghouseState",
			frame: `{"channel":"clearinghouseState","data":{"dex":"","user":"0x0000000000000000000000000000000000000001","clearinghouseState":` + websocketClearinghouseStateJSON + `}}`,
			exp:   &WsClearinghouseState{User: "0x0000000000000000000000000000000000000001", ClearinghouseState: testWebsocketClearinghouseState()},
		},
		{
			name:  "builder DEX clearinghouseState",
			frame: `{"channel":"clearinghouseState","data":{"dex":"xyz","user":"0x0000000000000000000000000000000000000001","clearinghouseState":` + websocketEmptyClearinghouseStateJSON + `}}`,
			exp:   &WsClearinghouseState{DEX: "xyz", User: "0x0000000000000000000000000000000000000001", ClearinghouseState: testWebsocketEmptyClearinghouseState()},
		},
		{
			name:  "openOrders",
			frame: `{"channel":"openOrders","data":{"dex":"xyz","user":"0x000000000000000000000000000000000000000a","orders":[{"coin":"xyz:XYZ100","side":"A","limitPx":"31175.0","sz":"0.0296","oid":566617880209,"timestamp":1791280999484,"triggerCondition":"N/A","isTrigger":false,"triggerPx":"0.0","children":[],"isPositionTpsl":false,"reduceOnly":false,"orderType":"Limit","origSz":"0.032","tif":"Alo","cloid":"0x000000000000000000065d291ba7c026"},{"coin":"xyz:XYZ100","side":"B","limitPx":"31180.0","sz":"0.0296","oid":566617880210,"timestamp":1791280999484,"triggerCondition":"Price above 31180","isTrigger":true,"triggerPx":"31180.0","children":[{"coin":"xyz:XYZ100","side":"B","limitPx":"30000.0","sz":"0.0296","oid":566617880211,"timestamp":1791280999484,"triggerCondition":"Price below 30000","isTrigger":true,"triggerPx":"30000.0","children":[],"isPositionTpsl":true,"reduceOnly":true,"orderType":"Stop Market","origSz":"0.032","tif":"FrontendMarket","cloid":"0x000000000000000000065d291ba7c028"}],"isPositionTpsl":true,"reduceOnly":true,"orderType":"Take Profit Market","origSz":"0.032","tif":"FrontendMarket","cloid":"0x000000000000000000065d291ba7c027"}]}}`,
			exp: &WsOpenOrders{
				DEX:  "xyz",
				User: "0x000000000000000000000000000000000000000a",
				Orders: []FrontendOpenOrder{
					{
						Coin:             "xyz:XYZ100",
						Side:             "A",
						LimitPrice:       31175,
						Size:             0.0296,
						OrderID:          566617880209,
						Timestamp:        milli(1791280999484),
						OriginalSize:     0.032,
						ClientOrderID:    "0x000000000000000000065d291ba7c026",
						TriggerCondition: "N/A",
						Children:         []FrontendOpenOrder{},
						OrderType:        "Limit",
						TimeInForce:      "Alo",
					},
					{
						Coin:             "xyz:XYZ100",
						Side:             "B",
						LimitPrice:       31180,
						Size:             0.0296,
						OrderID:          566617880210,
						Timestamp:        milli(1791280999484),
						OriginalSize:     0.032,
						ClientOrderID:    "0x000000000000000000065d291ba7c027",
						TriggerCondition: "Price above 31180",
						IsTrigger:        true,
						TriggerPrice:     31180,
						Children: []FrontendOpenOrder{{
							Coin:             "xyz:XYZ100",
							Side:             "B",
							LimitPrice:       30000,
							Size:             0.0296,
							OrderID:          566617880211,
							Timestamp:        milli(1791280999484),
							OriginalSize:     0.032,
							ClientOrderID:    "0x000000000000000000065d291ba7c028",
							TriggerCondition: "Price below 30000",
							IsTrigger:        true,
							TriggerPrice:     30000,
							Children:         []FrontendOpenOrder{},
							IsPositionTPSL:   true,
							ReduceOnly:       true,
							OrderType:        "Stop Market",
							TimeInForce:      "FrontendMarket",
						}},
						IsPositionTPSL: true,
						ReduceOnly:     true,
						OrderType:      "Take Profit Market",
						TimeInForce:    "FrontendMarket",
					},
				},
			},
		},
		{
			name:  "activeAssetData",
			frame: `{"channel":"activeAssetData","data":{"user":"0x0000000000000000000000000000000000000001","coin":"BTC","leverage":{"type":"isolated","value":10,"rawUsd":"-4999.5"},"maxTradeSzs":["683.30131","684.41969"],"availableToTrade":["2937170.6810349999","2941978.0374650001"],"markPx":"85970.0"}}`,
			exp: &ActiveAssetDataResponse{
				User:             "0x0000000000000000000000000000000000000001",
				Coin:             "BTC",
				Leverage:         Leverage{Type: "isolated", Value: 10, RawUSD: -4999.5},
				MaxTradeSizes:    [2]types.Number{683.30131, 684.41969},
				AvailableToTrade: [2]types.Number{2937170.6810349999, 2941978.0374650001},
				MarkPrice:        85970,
			},
		},
		{
			name:  "bbo",
			frame: `{"channel":"bbo","data":{"coin":"BTC","time":1791278884013,"bbo":[{"px":"85811.0","sz":"9.48256","n":43},{"px":"85812.0","sz":"4.09277","n":1}]}}`,
			exp: &WsBestBidOffer{
				Coin:         "BTC",
				Time:         milli(1791278884013),
				BestBidOffer: [2]*L2Level{{Price: 85811, Size: 9.48256, OrderCount: 43}, {Price: 85812, Size: 4.09277, OrderCount: 1}},
			},
		},
		{
			name:  "bbo without bids",
			frame: `{"channel":"bbo","data":{"coin":"@107","time":1791279271233,"bbo":[null,{"px":"92.638","sz":"12.89","n":1}]}}`,
			exp:   &WsBestBidOffer{Coin: "@107", Time: milli(1791279271233), BestBidOffer: [2]*L2Level{nil, {Price: 92.638, Size: 12.89, OrderCount: 1}}},
		},
		{
			name:  "spotState",
			frame: `{"channel":"spotState","data":{"user":"0x0000000000000000000000000000000000000002","spotState":{"balances":[{"coin":"USDC","token":0,"total":"100508.11272667","hold":"49999.89","entryNtl":"1.0"},{"coin":"VAULT","token":106,"total":"0.95437326","hold":"0.5","entryNtl":"0.00095437"}],"tokenToAvailableAfterMaintenance":[[0,"100508.11272667"]]}}}`,
			exp: &WsSpotState{
				User: "0x0000000000000000000000000000000000000002",
				SpotState: SpotClearinghouseStateResponse{
					Balances: []SpotBalance{
						{Coin: currency.USDC, Total: 100508.11272667, Hold: 49999.89, EntryNotional: 1},
						{Coin: currency.NewCode("VAULT"), Token: 106, Total: 0.95437326, Hold: 0.5, EntryNotional: 0.00095437},
					},
					TokenToAvailableAfterMaintenance: []TokenAmount{{Amount: 100508.11272667}},
				},
			},
		},
		{
			name:  "allDexsClearinghouseState",
			frame: `{"channel":"allDexsClearinghouseState","data":{"user":"0x0000000000000000000000000000000000000001","clearinghouseStates":[["",` + websocketClearinghouseStateJSON + `],["xyz",` + websocketEmptyClearinghouseStateJSON + `]]}}`,
			exp: &WsAllDEXsClearinghouseState{
				User: "0x0000000000000000000000000000000000000001",
				ClearinghouseStates: []DEXClearinghouseState{
					{ClearinghouseState: testWebsocketClearinghouseState()},
					{DEX: "xyz", ClearinghouseState: testWebsocketEmptyClearinghouseState()},
				},
			},
		},
		{
			name:  "allDexsAssetCtxs",
			frame: `{"channel":"allDexsAssetCtxs","data":{"ctxs":[["",[{"funding":"0.0000125","openInterest":"39988.35486","prevDayPx":"85840.0","dayNtlVlm":"1821731901.6448168755","premium":"-0.0000932097","oraclePx":"85828.0","markPx":"85814.0","midPx":"85818.0","impactPxs":["85816.0","85820.0"],"dayBaseVlm":"21234.78753"}]],["xyz",[{"funding":"0.00000625","openInterest":"7177.89","prevDayPx":"30751.0","dayNtlVlm":"276887160.4848001003","premium":"0.0000855208","oraclePx":"31162.0","markPx":"31164.0","midPx":"31164.5","impactPxs":["31164.0","31165.33"],"dayBaseVlm":"8935.3045"}]]]}}`,
			exp: &WsAllDEXsAssetContexts{Contexts: []DEXAssetContexts{
				{AssetContexts: []PerpetualAssetContext{{
					Funding:           0.0000125,
					OpenInterest:      39988.35486,
					PreviousDayPrice:  85840,
					DayNotionalVolume: 1821731901.6448168755,
					Premium:           -0.0000932097,
					OraclePrice:       85828,
					MarkPrice:         85814,
					MidPrice:          85818,
					ImpactPrices:      []types.Number{85816, 85820},
					DayBaseVolume:     21234.78753,
				}}},
				{DEX: "xyz", AssetContexts: []PerpetualAssetContext{{
					Funding:           0.00000625,
					OpenInterest:      7177.89,
					PreviousDayPrice:  30751,
					DayNotionalVolume: 276887160.4848001003,
					Premium:           0.0000855208,
					OraclePrice:       31162,
					MarkPrice:         31164,
					MidPrice:          31164.5,
					ImpactPrices:      []types.Number{31164, 31165.33},
					DayBaseVolume:     8935.3045,
				}}},
			}},
		},
		{
			name:  "allDexsAssetCtxs without a book",
			frame: `{"channel":"allDexsAssetCtxs","data":{"ctxs":[["",[{"funding":"0.0000125","openInterest":"39988.35486","prevDayPx":"85840.0","dayNtlVlm":"1821731901.6448168755","premium":null,"oraclePx":"85828.0","markPx":"85814.0","midPx":null,"impactPxs":null,"dayBaseVlm":"21234.78753"}]]]}}`,
			exp: &WsAllDEXsAssetContexts{Contexts: []DEXAssetContexts{{AssetContexts: []PerpetualAssetContext{{
				Funding:           0.0000125,
				OpenInterest:      39988.35486,
				PreviousDayPrice:  85840,
				DayNotionalVolume: 1821731901.6448168755,
				OraclePrice:       85828,
				MarkPrice:         85814,
				DayBaseVolume:     21234.78753,
			}}}}},
		},
	} {
		require.NoErrorf(t, ex.wsHandleData(t.Context(), []byte(tc.frame), false), "wsHandleData must not error for %s", tc.name)
		assert.Equalf(t, tc.exp, receiveWebsocketData(t, ex), "wsHandleData should relay the decoded %s message", tc.name)
	}
	assertNoWebsocketData(t, ex)
}

func TestWebsocketMessageUnmarshal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		data   string
		target any
		exp    any
	}{
		{
			name:   "fast l2Book",
			data:   `{"coin":"SOL","time":1791279270407,"levels":[[{"px":"119.19","sz":"896.3","n":7},{"px":"119.18","sz":"157.98","n":5}],[{"px":"119.2","sz":"684.82","n":11},{"px":"119.21","sz":"862.57","n":17}]],"fast":true}`,
			target: new(WsL2Book),
			exp: &WsL2Book{
				Coin: "SOL",
				Time: milli(1791279270407),
				Levels: [][]L2Level{
					{{Price: 119.19, Size: 896.3, OrderCount: 7}, {Price: 119.18, Size: 157.98, OrderCount: 5}},
					{{Price: 119.2, Size: 684.82, OrderCount: 11}, {Price: 119.21, Size: 862.57, OrderCount: 17}},
				},
				Fast: true,
			},
		},
		{
			name:   "aggregated l2Book",
			data:   `{"coin":"BTC","time":1791279269350,"levels":[[{"px":"85874.0","sz":"17.86492","n":63}],[{"px":"85876.0","sz":"1.22481","n":6}]],"spread":"1.0"}`,
			target: new(WsL2Book),
			exp: &WsL2Book{
				Coin:   "BTC",
				Time:   milli(1791279269350),
				Levels: [][]L2Level{{{Price: 85874, Size: 17.86492, OrderCount: 63}}, {{Price: 85876, Size: 1.22481, OrderCount: 6}}},
				Spread: 1,
			},
		},
		{
			name:   "perpetual activeAssetCtx",
			data:   `{"coin":"BTC","ctx":{"funding":"0.0000125","openInterest":"39980.76392","prevDayPx":"85840.0","dayNtlVlm":"1821282705.4506571293","premium":"-0.0001864194","oraclePx":"85828.0","markPx":"85814.0","midPx":"85811.5","impactPxs":["85811.0","85812.0"],"dayBaseVlm":"21229.55909"}}`,
			target: new(WsPerpetualAssetContext),
			exp: &WsPerpetualAssetContext{Coin: "BTC", Context: PerpetualAssetContext{
				Funding:           0.0000125,
				OpenInterest:      39980.76392,
				PreviousDayPrice:  85840,
				DayNotionalVolume: 1821282705.4506571293,
				Premium:           -0.0001864194,
				OraclePrice:       85828,
				MarkPrice:         85814,
				MidPrice:          85811.5,
				ImpactPrices:      []types.Number{85811, 85812},
				DayBaseVolume:     21229.55909,
			}},
		},
		{
			name:   "spot activeAssetCtx",
			data:   `{"coin":"@107","ctx":{"prevDayPx":"93.124","dayNtlVlm":"64552049.6973299459","markPx":"92.949","midPx":"92.945","circulatingSupply":"298462370.4079165459","coin":"@107","totalSupply":"998882916.6303099394","dayBaseVlm":"690824.1400000008"}}`,
			target: new(WsSpotAssetContext),
			exp: &WsSpotAssetContext{Coin: "@107", Context: SpotAssetContext{
				PreviousDayPrice:  93.124,
				DayNotionalVolume: 64552049.6973299459,
				MarkPrice:         92.949,
				MidPrice:          92.945,
				CirculatingSupply: 298462370.4079165459,
				Coin:              "@107",
				TotalSupply:       998882916.6303099394,
				DayBaseVolume:     690824.1400000008,
			}},
		},
		{
			name:   "userFills",
			data:   `{"isSnapshot":true,"user":"0x000000000000000000000000000000000000000a","fills":[` + websocketFillJSON + `]}`,
			target: new(WsUserFills),
			exp:    &WsUserFills{IsSnapshot: true, User: "0x000000000000000000000000000000000000000a", Fills: []Fill{testWebsocketFill()}},
		},
		{
			name:   "userTwapSliceFills",
			data:   `{"isSnapshot":true,"user":"0x0000000000000000000000000000000000000006","twapSliceFills":[{"fill":` + websocketFillJSON + `,"twapId":2288616}]}`,
			target: new(WsUserTWAPSliceFills),
			exp: &WsUserTWAPSliceFills{
				IsSnapshot:     true,
				User:           "0x0000000000000000000000000000000000000006",
				TWAPSliceFills: []TWAPSliceFill{{Fill: testWebsocketFill(), TWAPID: 2288616}},
			},
		},
		{
			name:   "userEvents",
			data:   `{"fills":[` + websocketFillJSON + `],"funding":{"time":1791280800037,"coin":"BTC","usdc":"-0.601087","szi":"0.55919","fundingRate":"0.0000125","nSamples":24},"liquidation":{"lid":123456,"liquidator":"0x0000000000000000000000000000000000000009","liquidated_user":"0x000000000000000000000000000000000000000b","liquidated_ntl_pos":"356.80015","liquidated_account_value":"2.956494"},"nonUserCancel":[{"coin":"PNUT","oid":566615769247}],"twapSliceFills":[{"fill":` + websocketFillJSON + `,"twapId":2288616}]}`,
			target: new(WsUserEvent),
			exp: &WsUserEvent{
				Fills:          []Fill{testWebsocketFill()},
				Funding:        &WsUserFunding{Time: milli(1791280800037), Coin: "BTC", USDC: -0.601087, SignedSize: 0.55919, FundingRate: 0.0000125, NumberOfSamples: 24},
				Liquidation:    &WsLiquidation{LiquidationID: 123456, Liquidator: "0x0000000000000000000000000000000000000009", LiquidatedUser: "0x000000000000000000000000000000000000000b", LiquidatedNotionalPosition: 356.80015, LiquidatedAccountValue: 2.956494},
				NonUserCancel:  []WsNonUserCancel{{Coin: "PNUT", OrderID: 566615769247}},
				TWAPSliceFills: []TWAPSliceFill{{Fill: testWebsocketFill(), TWAPID: 2288616}},
			},
		},
	} {
		require.NoErrorf(t, json.Unmarshal([]byte(tc.data), tc.target), "Unmarshal must not error for %s", tc.name)
		assert.Equalf(t, tc.exp, tc.target, "Unmarshal should decode every %s field", tc.name)
	}
}

// websocketFillJSON is a liquidation fill from the websocket fixtures, with every field set
const websocketFillJSON = `{"coin":"BTC","px":"86006.0","sz":"0.07905","side":"B","time":1791280942711,"startPosition":"-0.14136","dir":"Close Short","closedPnl":"-2.600745","hash":"0x0da3f51a7423740a0f1d0445f5707e02017400000f2692dcb16ca06d33274df4","oid":566617233807,"crossed":true,"fee":"-0.067987","tid":669583051366496,"cloid":"0x000000000000000000065d2917ff6969","feeToken":"USDC","twapId":2288616,"builderFee":"0.0001","liquidation":{"liquidatedUser":"0x000000000000000000000000000000000000000b","markPx":"86010.0","method":"backstop"}}`

// websocketSpotFillJSON is a TWAP slice fill of the HYPE spot market from the websocket fixtures
const websocketSpotFillJSON = `{"coin":"@107","px":"92.7","sz":"0.41","side":"A","time":1791279150013,"startPosition":"54377.71000602","dir":"Sell","closedPnl":"5.11885001","hash":"0x0000000000000000000000000000000000000000000000000000000000000000","oid":566598918312,"crossed":true,"fee":"0.01787849","tid":499584618036466,"feeToken":"USDC","twapId":null}`

// testWebsocketFill is websocketFillJSON decoded
func testWebsocketFill() Fill {
	return Fill{
		Coin:          "BTC",
		Price:         86006,
		Size:          0.07905,
		Side:          "B",
		Time:          milli(1791280942711),
		StartPosition: -0.14136,
		Direction:     "Close Short",
		ClosedPNL:     -2.600745,
		Hash:          "0x0da3f51a7423740a0f1d0445f5707e02017400000f2692dcb16ca06d33274df4",
		OrderID:       566617233807,
		Crossed:       true,
		Fee:           -0.067987,
		TradeID:       669583051366496,
		ClientOrderID: "0x000000000000000000065d2917ff6969",
		Liquidation:   &FillLiquidation{LiquidatedUser: "0x000000000000000000000000000000000000000b", MarkPrice: 86010, Method: "backstop"},
		FeeToken:      currency.USDC,
		BuilderFee:    0.0001,
		TWAPID:        2288616,
	}
}

// testWebsocketFillData is websocketFillJSON converted to a fill
func testWebsocketFillData(exchangeName string) fill.Data {
	return fill.Data{
		ID:            "669583051366496",
		Timestamp:     time.UnixMilli(1791280942711).UTC(),
		Exchange:      exchangeName,
		AssetType:     asset.PerpetualContract,
		CurrencyPair:  perpetualPair,
		Side:          order.Buy,
		OrderID:       "566617233807",
		ClientOrderID: "0x000000000000000000065d2917ff6969",
		TradeID:       "669583051366496",
		Price:         86006,
		Amount:        0.07905,
	}
}

// testWebsocketSpotFillData is websocketSpotFillJSON converted to a fill
func testWebsocketSpotFillData(exchangeName string) fill.Data {
	return fill.Data{
		ID:           "499584618036466",
		Timestamp:    time.UnixMilli(1791279150013).UTC(),
		Exchange:     exchangeName,
		AssetType:    asset.Spot,
		CurrencyPair: spotPair,
		Side:         order.Sell,
		OrderID:      "566598918312",
		TradeID:      "499584618036466",
		Price:        92.7,
		Amount:       0.41,
	}
}

// receiveTicker returns the relayed ticker, checking and then clearing its current time stamp
func receiveTicker(t *testing.T, ex *Exchange) *ticker.Price {
	t.Helper()
	price, ok := receiveWebsocketData(t, ex).(*ticker.Price)
	require.True(t, ok, "the relayed message must be a ticker")
	assert.WithinDuration(t, time.Now(), price.LastUpdated, time.Minute, "the ticker should be stamped with the current time")
	price.LastUpdated = time.Time{}
	return price
}

func TestWsHandlePerpetualTicker(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"activeAssetCtx","data":{"coin":"BTC","ctx":{"funding":"0.0000125","openInterest":"39980.76392","prevDayPx":"85840.0","dayNtlVlm":"1821282705.4506571293","premium":"-0.0001864194","oraclePx":"85828.0","markPx":"85814.0","midPx":"85811.5","impactPxs":["85811.0","85812.0"],"dayBaseVlm":"21229.55909"}}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), false), "wsHandleData must not error")
	exp := &ticker.Price{
		Last:         85811.5,
		Open:         85840,
		BaseVolume:   21229.55909,
		QuoteVolume:  1821282705.4506571293,
		OpenInterest: 39980.76392,
		MarkPrice:    85814,
		IndexPrice:   85828,
		Pair:         perpetualPair,
		ExchangeName: ex.Name,
		AssetType:    asset.PerpetualContract,
	}
	assert.Equal(t, exp, receiveTicker(t, ex), "wsHandlePerpetualTicker should relay the converted ticker")
	stored, err := ticker.GetTicker(ex.Name, perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetTicker must not error")
	assert.Equal(t, 85811.5, stored.Last, "wsHandlePerpetualTicker should store the ticker")

	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"activeAssetCtx","data":{"coin":"@107","ctx":{}}}`), false)
	assert.ErrorIs(t, err, errWebsocketAssetMismatch, "wsHandlePerpetualTicker should reject a spot coin")
	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"activeAssetCtx","data":{"coin":"ETH","ctx":{}}}`), false)
	assert.ErrorIs(t, err, errPairMappingNotFound, "wsHandlePerpetualTicker should reject an unknown coin")
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{{coin: "UNPAIRED"}})
	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"activeAssetCtx","data":{"coin":"UNPAIRED","ctx":{"markPx":"1.0"}}}`), false)
	assert.Error(t, err, "wsHandlePerpetualTicker should return the error of a ticker that cannot be stored")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleSpotTicker(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"activeSpotAssetCtx","data":{"coin":"@107","ctx":{"prevDayPx":"93.124","dayNtlVlm":"64552049.6973299459","markPx":"92.949","midPx":"92.945","circulatingSupply":"298462370.4079165459","coin":"@107","totalSupply":"998882916.6303099394","dayBaseVlm":"690824.1400000008"}}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), false), "wsHandleData must not error")
	exp := &ticker.Price{
		Last:         92.945,
		Open:         93.124,
		BaseVolume:   690824.1400000008,
		QuoteVolume:  64552049.6973299459,
		MarkPrice:    92.949,
		Pair:         spotPair,
		ExchangeName: ex.Name,
		AssetType:    asset.Spot,
	}
	assert.Equal(t, exp, receiveTicker(t, ex), "wsHandleSpotTicker should relay the converted ticker")
	stored, err := ticker.GetTicker(ex.Name, spotPair, asset.Spot)
	require.NoError(t, err, "GetTicker must not error")
	assert.Equal(t, 92.945, stored.Last, "wsHandleSpotTicker should store the ticker")

	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"activeSpotAssetCtx","data":{"coin":"BTC","ctx":{}}}`), false)
	assert.ErrorIs(t, err, errWebsocketAssetMismatch, "wsHandleSpotTicker should reject a perpetual coin")
	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"activeSpotAssetCtx","data":{"coin":"@1","ctx":{}}}`), false)
	assert.ErrorIs(t, err, errPairMappingNotFound, "wsHandleSpotTicker should reject an unknown coin")
	ex.setPairMappings(asset.Spot, []pairMapping{{coin: "@999"}})
	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"activeSpotAssetCtx","data":{"coin":"@999","ctx":{"markPx":"1.0"}}}`), false)
	assert.Error(t, err, "wsHandleSpotTicker should return the error of a ticker that cannot be stored")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleOrderbook(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"l2Book","data":{"coin":"BTC","time":1791278882097,"levels":[[{"px":"85821.0","sz":"0.96592","n":4},{"px":"85818.0","sz":"0.80629","n":1}],[{"px":"85822.0","sz":"26.98831","n":71},{"px":"85823.0","sz":"4.42262","n":4}]]}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), false), "wsHandleData must not error")
	_, ok := receiveWebsocketData(t, ex).(*orderbook.Depth)
	require.True(t, ok, "wsHandleOrderbook must relay the orderbook depth")
	book, err := ex.Websocket.Orderbook.GetOrderbook(perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetOrderbook must not error")
	assert.Equal(t, orderbook.Levels{{Price: 85821, Amount: 0.96592, OrderCount: 4}, {Price: 85818, Amount: 0.80629, OrderCount: 1}}, book.Bids, "wsHandleOrderbook should load the bids")
	assert.Equal(t, orderbook.Levels{{Price: 85822, Amount: 26.98831, OrderCount: 71}, {Price: 85823, Amount: 4.42262, OrderCount: 4}}, book.Asks, "wsHandleOrderbook should load the asks")
	assert.True(t, book.LastUpdated.Equal(time.UnixMilli(1791278882097)), "wsHandleOrderbook should load the book time")

	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"l2Book","data":{"coin":"BTC","time":1791278882097,"levels":[[]]}}`), false)
	assert.ErrorIs(t, err, errInvalidBookLevelCount, "wsHandleOrderbook should reject a book without two sides")
	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"l2Book","data":{"coin":"ETH","time":1791278882097,"levels":[[],[]]}}`), false)
	assert.ErrorIs(t, err, errPairMappingNotFound, "wsHandleOrderbook should reject an unknown coin")
	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"l2Book","data":{"coin":"@107","time":1791278882097,"levels":[[{"px":"0","sz":"1","n":1}],[{"px":"92.7","sz":"1","n":1}]]}}`), false)
	assert.ErrorIs(t, err, orderbook.ErrPriceZero, "wsHandleOrderbook should reject an invalid book")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleTrades(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"trades","data":[{"coin":"BTC","side":"B","px":"85827.0","sz":"0.02721","time":1791278878126,"hash":"0xe7a30278f69a68bee91c0445f4fe140206c5005e919d87908b6badcbb59e42a9","tid":188095306343783,"users":["0x0000000000000000000000000000000000000004","0x0000000000000000000000000000000000000005"]},{"coin":"BTC","side":"A","px":"85826.0","sz":"0.00897","time":1791278878126,"hash":"0x7e3eb751c3147c9c7fb80445f4fe140206cd00375e179b6e220762a482185687","tid":250009747508244,"users":["0x0000000000000000000000000000000000000004","0x0000000000000000000000000000000000000005"]}]}`
	ex.expectTradeReplay("BTC")
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), false), "wsHandleData must not error for the replayed trades")
	assertNoWebsocketData(t, ex)

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), false), "wsHandleData must not error")
	exp := []trade.Data{
		{TID: "188095306343783", Exchange: ex.Name, CurrencyPair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Buy, Price: 85827, Amount: 0.02721, Timestamp: time.UnixMilli(1791278878126).UTC()},
		{TID: "250009747508244", Exchange: ex.Name, CurrencyPair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Sell, Price: 85826, Amount: 0.00897, Timestamp: time.UnixMilli(1791278878126).UTC()},
	}
	assert.Equal(t, exp, receiveWebsocketData(t, ex), "wsHandleTrades should relay the trades after the replay")

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"trades","data":[]}`), false), "wsHandleData must not error for an empty batch")
	assertNoWebsocketData(t, ex)

	mixed := `{"channel":"trades","data":[{"coin":"BTC","side":"X","px":"85827.0","sz":"1","time":1791278878126,"tid":1},{"coin":"BTC","side":"B","px":"85827.0","sz":"0.02721","time":1791278878126,"tid":188095306343783}]}`
	assert.ErrorIs(t, ex.wsHandleData(t.Context(), []byte(mixed), false), order.ErrSideIsInvalid, "wsHandleTrades should report a trade that cannot convert")
	assert.Equal(t, exp[:1], receiveWebsocketData(t, ex), "wsHandleTrades should relay the trades that convert")

	err := ex.wsHandleData(t.Context(), []byte(`{"channel":"trades","data":[{"coin":"ETH","side":"B","px":"2710.8","sz":"1","time":1791278878126,"tid":1}]}`), false)
	assert.ErrorIs(t, err, errPairMappingNotFound, "wsHandleTrades should reject an unknown coin")
	assertNoWebsocketData(t, ex)
}

func TestTradeReplay(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	assert.False(t, ex.consumeTradeReplay("BTC"), "consumeTradeReplay should report no replay before one is expected")
	ex.expectTradeReplay("BTC")
	assert.False(t, ex.consumeTradeReplay("ETH"), "consumeTradeReplay should report no replay for another coin")
	assert.True(t, ex.consumeTradeReplay("BTC"), "consumeTradeReplay should report the expected replay")
	assert.False(t, ex.consumeTradeReplay("BTC"), "consumeTradeReplay should report one replay per subscription")
	ex.expectTradeReplay("BTC")
	ex.clearTradeReplay("BTC")
	assert.False(t, ex.consumeTradeReplay("BTC"), "consumeTradeReplay should report no replay once it is cleared")
}

func TestWsHandleCandle(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	for _, tc := range []struct {
		frame string
		exp   kline.Item
	}{
		{
			frame: `{"channel":"candle","data":{"t":1791278880000,"T":1791278939999,"s":"BTC","i":"1m","o":"85827.0","c":"85812.0","h":"85827.0","l":"85801.0","v":"0.73617","n":20}}`,
			exp: kline.Item{
				Exchange: ex.Name,
				Pair:     perpetualPair,
				Asset:    asset.PerpetualContract,
				Interval: kline.OneMin,
				Candles:  []kline.Candle{{Time: time.UnixMilli(1791278880000).UTC(), Open: 85827, High: 85827, Low: 85801, Close: 85812, Volume: 0.73617}},
			},
		},
		{
			frame: `{"channel":"candle","data":{"t":1791279240000,"T":1791279299999,"s":"@107","i":"1m","o":"92.652","c":"92.621","h":"92.653","l":"92.581","v":"2115.03","n":57}}`,
			exp: kline.Item{
				Exchange: ex.Name,
				Pair:     spotPair,
				Asset:    asset.Spot,
				Interval: kline.OneMin,
				Candles:  []kline.Candle{{Time: time.UnixMilli(1791279240000).UTC(), Open: 92.652, High: 92.653, Low: 92.581, Close: 92.621, Volume: 2115.03}},
			},
		},
	} {
		require.NoError(t, ex.wsHandleData(t.Context(), []byte(tc.frame), false), "wsHandleData must not error")
		assert.Equalf(t, tc.exp, receiveWebsocketData(t, ex), "wsHandleCandle should relay the %s %s candle", tc.exp.Asset, tc.exp.Pair)
	}
	err := ex.wsHandleData(t.Context(), []byte(`{"channel":"candle","data":{"s":"ETH","i":"1m"}}`), false)
	assert.ErrorIs(t, err, errPairMappingNotFound, "wsHandleCandle should reject an unknown coin")
	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"candle","data":{"s":"BTC","i":"7m"}}`), false)
	assert.ErrorIs(t, err, kline.ErrUnsupportedInterval, "wsHandleCandle should reject an unsupported interval")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleOrderUpdates(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"orderUpdates","data":[{"order":{"coin":"@107","side":"B","limitPx":"93.218","sz":"3.28","oid":566617964508,"timestamp":1791281008463,"origSz":"3.28","cloid":"0x000000000000000000065d291c305ae0"},"status":"open","statusTimestamp":1791281008463},{"order":{"coin":"@107","side":"B","limitPx":"93.218","sz":"0.0","oid":566617964508,"timestamp":1791281008463,"origSz":"3.28","cloid":"0x000000000000000000065d291c305ae0"},"status":"filled","statusTimestamp":1791281008463},{"order":{"coin":"xyz:XYZ100","side":"A","limitPx":"31169.5","sz":"0.1","oid":566617840215,"timestamp":1791280994709,"origSz":"0.1","cloid":"0x000000000000000000065d291b5eb630"},"status":"canceled","statusTimestamp":1791281001374}]}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), true), "wsHandleData must not error")
	placed := time.UnixMilli(1791281008463).UTC()
	exp := []order.Detail{
		{
			Price:           93.218,
			Amount:          3.28,
			RemainingAmount: 3.28,
			Exchange:        ex.Name,
			OrderID:         "566617964508",
			ClientOrderID:   "0x000000000000000000065d291c305ae0",
			Side:            order.Buy,
			Status:          order.Open,
			AssetType:       asset.Spot,
			Date:            placed,
			LastUpdated:     placed,
			Pair:            spotPair,
		},
		{
			Price:          93.218,
			Amount:         3.28,
			ExecutedAmount: 3.28,
			Exchange:       ex.Name,
			OrderID:        "566617964508",
			ClientOrderID:  "0x000000000000000000065d291c305ae0",
			Side:           order.Buy,
			Status:         order.Filled,
			AssetType:      asset.Spot,
			Date:           placed,
			LastUpdated:    placed,
			Pair:           spotPair,
		},
		{
			Price:           31169.5,
			Amount:          0.1,
			RemainingAmount: 0.1,
			Exchange:        ex.Name,
			OrderID:         "566617840215",
			ClientOrderID:   "0x000000000000000000065d291b5eb630",
			Side:            order.Sell,
			Status:          order.Cancelled,
			AssetType:       asset.PerpetualContract,
			Date:            time.UnixMilli(1791280994709).UTC(),
			LastUpdated:     time.UnixMilli(1791281001374).UTC(),
			Pair:            testBuilderPair,
		},
	}
	assert.Equal(t, exp, receiveWebsocketData(t, ex), "wsHandleOrderUpdates should relay the converted orders")

	mixed := `{"channel":"orderUpdates","data":[{"order":{"coin":"ETH","side":"B","oid":1,"timestamp":1791281008463},"status":"open","statusTimestamp":1791281008463},{"order":{"coin":"BTC","side":"B","oid":2,"timestamp":1791281008463},"status":"unknownStatus","statusTimestamp":1791281008463},{"order":{"coin":"@107","side":"B","limitPx":"93.218","sz":"3.28","oid":566617964508,"timestamp":1791281008463,"origSz":"3.28","cloid":"0x000000000000000000065d291c305ae0"},"status":"open","statusTimestamp":1791281008463}]}`
	err := ex.wsHandleData(t.Context(), []byte(mixed), true)
	assert.ErrorIs(t, err, errPairMappingNotFound, "wsHandleOrderUpdates should report an order of an unknown coin")
	assert.ErrorIs(t, err, errUnsupportedOrderStatus, "wsHandleOrderUpdates should report an order with an unknown status")
	assert.Equal(t, exp[:1], receiveWebsocketData(t, ex), "wsHandleOrderUpdates should relay the orders that convert")

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"orderUpdates","data":[]}`), true), "wsHandleData must not error for an empty batch")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleUserEvent(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"user","data":{"fills":[`+websocketFillJSON+`]}}`), true), "wsHandleData must not error for fills")
	assert.Equal(t, []fill.Data{testWebsocketFillData(ex.Name)}, receiveWebsocketData(t, ex), "wsHandleUserEvent should relay fills to the fills feed")

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"user","data":{"twapSliceFills":[{"fill":`+websocketSpotFillJSON+`,"twapId":2278664}]}}`), true), "wsHandleData must not error for TWAP slice fills")
	assert.Equal(t, []fill.Data{testWebsocketSpotFillData(ex.Name)}, receiveWebsocketData(t, ex), "wsHandleUserEvent should relay TWAP slice fills to the fills feed")

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"user","data":{"funding":{"time":1791280800037,"coin":"BTC","usdc":"-0.601087","szi":"0.55919","fundingRate":"0.0000125","nSamples":null}}}`), true), "wsHandleData must not error for a funding payment")
	assert.Equal(t, &WsUserFunding{Time: milli(1791280800037), Coin: "BTC", USDC: -0.601087, SignedSize: 0.55919, FundingRate: 0.0000125}, receiveWebsocketData(t, ex), "wsHandleUserEvent should relay a funding payment")

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"user","data":{"liquidation":{"lid":123456,"liquidator":"0x0000000000000000000000000000000000000009","liquidated_user":"0x000000000000000000000000000000000000000b","liquidated_ntl_pos":"356.80015","liquidated_account_value":"2.956494"}}}`), true), "wsHandleData must not error for a liquidation")
	assert.Equal(t, &WsLiquidation{LiquidationID: 123456, Liquidator: "0x0000000000000000000000000000000000000009", LiquidatedUser: "0x000000000000000000000000000000000000000b", LiquidatedNotionalPosition: 356.80015, LiquidatedAccountValue: 2.956494}, receiveWebsocketData(t, ex), "wsHandleUserEvent should relay a liquidation")

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"user","data":{"nonUserCancel":[{"coin":"PNUT","oid":566615769247}]}}`), true), "wsHandleData must not error for cancels")
	assert.Equal(t, []WsNonUserCancel{{Coin: "PNUT", OrderID: 566615769247}}, receiveWebsocketData(t, ex), "wsHandleUserEvent should relay the orders the exchange cancelled")

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"user","data":{}}`), true), "wsHandleData must not error for an empty event")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleUserFills(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"userFills","data":{"isSnapshot":true,"user":"0x000000000000000000000000000000000000000a","fills":[`+websocketFillJSON+`]}}`), true), "wsHandleData must not error for the snapshot")
	assertNoWebsocketData(t, ex)

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"userFills","data":{"user":"0x000000000000000000000000000000000000000a","fills":[`+websocketFillJSON+`]}}`), true), "wsHandleData must not error")
	assert.Equal(t, []fill.Data{testWebsocketFillData(ex.Name)}, receiveWebsocketData(t, ex), "wsHandleUserFills should relay new fills")
}

func TestWsHandleUserTWAPSliceFills(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"userTwapSliceFills","data":{"isSnapshot":true,"user":"0x0000000000000000000000000000000000000007","twapSliceFills":[{"fill":`+websocketSpotFillJSON+`,"twapId":2278664}]}}`), true), "wsHandleData must not error for the snapshot")
	assertNoWebsocketData(t, ex)

	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"userTwapSliceFills","data":{"user":"0x0000000000000000000000000000000000000007","twapSliceFills":[{"fill":`+websocketSpotFillJSON+`,"twapId":2278664}]}}`), true), "wsHandleData must not error")
	assert.Equal(t, []fill.Data{testWebsocketSpotFillData(ex.Name)}, receiveWebsocketData(t, ex), "wsHandleUserTWAPSliceFills should relay new TWAP slice fills")
}

func TestRelayFills(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	fills := []Fill{{Coin: "ETH", Side: "B"}, testWebsocketFill()}
	sliceFills := []TWAPSliceFill{{Fill: Fill{Coin: "BTC", Side: "X"}, TWAPID: 7}, {Fill: testWebsocketFill(), TWAPID: 2288616}}
	err := ex.relayFills(fills, sliceFills)
	assert.ErrorIs(t, err, errPairMappingNotFound, "relayFills should report a fill of an unknown coin")
	assert.ErrorIs(t, err, order.ErrSideIsInvalid, "relayFills should report a TWAP slice fill with an invalid side")
	assert.ErrorContains(t, err, "TWAP 7 slice fill 0", "relayFills should identify the TWAP slice fill that failed")
	exp := []fill.Data{testWebsocketFillData(ex.Name), testWebsocketFillData(ex.Name)}
	assert.Equal(t, exp, receiveWebsocketData(t, ex), "relayFills should relay the fills that convert")

	assert.ErrorIs(t, ex.relayFills([]Fill{{Coin: "ETH", Side: "B"}}, nil), errPairMappingNotFound, "relayFills should report a batch that does not convert")
	assert.NoError(t, ex.relayFills(nil, nil), "relayFills should not error without fills")
	ex.SetFillsFeedStatus(false)
	assert.NoError(t, ex.relayFills(fills, nil), "relayFills should ignore fills while the fills feed is disabled")
	assertNoWebsocketData(t, ex)
}

func TestConvertFill(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	_, err := ex.convertFill(&Fill{Coin: "ETH", Side: "B"})
	assert.ErrorIs(t, err, errPairMappingNotFound, "convertFill should reject an unknown coin")
	_, err = ex.convertFill(&Fill{Coin: "BTC", Side: "X"})
	assert.ErrorIs(t, err, order.ErrSideIsInvalid, "convertFill should reject an invalid side")
	source := testWebsocketFill()
	converted, err := ex.convertFill(&source)
	require.NoError(t, err, "convertFill must not error")
	assert.Equal(t, testWebsocketFillData(ex.Name), converted, "convertFill should identify a fill by its trade ID")
}

func TestWsHandleUserFundings(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"userFundings","data":{"isSnapshot":true,"user":"0x0000000000000000000000000000000000000003","fundings":[{"time":1791216000037,"coin":"ENA","usdc":"-2.38878","szi":"777946.0","fundingRate":"0.0000125","nSamples":1},{"time":1791219600012,"coin":"BTC","usdc":"0.238633","szi":"-0.2238","fundingRate":"0.0000125","nSamples":null}]}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), true), "wsHandleData must not error")
	exp := &WsUserFundings{
		IsSnapshot: true,
		User:       "0x0000000000000000000000000000000000000003",
		Fundings: []WsUserFunding{
			{Time: milli(1791216000037), Coin: "ENA", USDC: -2.38878, SignedSize: 777946, FundingRate: 0.0000125, NumberOfSamples: 1},
			{Time: milli(1791219600012), Coin: "BTC", USDC: 0.238633, SignedSize: -0.2238, FundingRate: 0.0000125},
		},
	}
	assert.Equal(t, exp, receiveWebsocketData(t, ex), "wsHandleUserFundings should relay the funding payments")
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"userFundings","data":{"user":"0x0000000000000000000000000000000000000003","fundings":[]}}`), true), "wsHandleData must not error without payments")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleUserNonFundingLedgerUpdates(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"userNonFundingLedgerUpdates","data":{"isSnapshot":true,"user":"0x0000000000000000000000000000000000000001","nonFundingLedgerUpdates":[{"time":1748135719014,"hash":"0x2674629937983fbe452ec47df9c991d29c136ff7c79c3c1630d600d6707524d0","delta":{"type":"deposit","usdc":"200079.847397"}},{"time":1677618623525,"hash":"0x651383a8fda3daaf31cc0308597c0000f7f51388f97acf0df28797ee97ae278a","delta":{"type":"liquidation","liquidatedNtlPos":"356.80015","accountValue":"2.956494","leverageType":"Cross","liquidatedPositions":[{"coin":"ATOM","szi":"29.21"}]}}]}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), true), "wsHandleData must not error")
	exp := &WsUserNonFundingLedgerUpdates{
		IsSnapshot: true,
		User:       "0x0000000000000000000000000000000000000001",
		NonFundingLedgerUpdates: []UserLedgerUpdate{
			{Delta: UserLedgerDelta{Type: "deposit", USDC: 200079.847397}, Hash: "0x2674629937983fbe452ec47df9c991d29c136ff7c79c3c1630d600d6707524d0", Time: milli(1748135719014)},
			{
				Delta: UserLedgerDelta{
					Type:                       "liquidation",
					AccountValue:               2.956494,
					LeverageType:               "Cross",
					LiquidatedNotionalPosition: 356.80015,
					LiquidatedPositions:        []LiquidatedPosition{{Coin: "ATOM", SignedSize: 29.21}},
				},
				Hash: "0x651383a8fda3daaf31cc0308597c0000f7f51388f97acf0df28797ee97ae278a",
				Time: milli(1677618623525),
			},
		},
	}
	assert.Equal(t, exp, receiveWebsocketData(t, ex), "wsHandleUserNonFundingLedgerUpdates should relay the ledger updates")
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"userNonFundingLedgerUpdates","data":{"user":"0x0000000000000000000000000000000000000001","nonFundingLedgerUpdates":[]}}`), true), "wsHandleData must not error without updates")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleUserTWAPHistory(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"userTwapHistory","data":{"isSnapshot":true,"user":"0x0000000000000000000000000000000000000007","history":[{"time":1769165674,"state":{"coin":"@107","user":"0x0000000000000000000000000000000000000007","side":"B","sz":"2000.0","executedSz":"2000.0","executedNtl":"41959.56859","minutes":30,"reduceOnly":true,"randomize":true,"timestamp":1769163872991,"trigger":{"px":"88.0","above":true},"stopPx":"85.5"},"status":{"status":"finished","description":"finished"},"twapId":1530771},{"time":1770013630,"state":{"coin":"@151","user":"0x0000000000000000000000000000000000000007","side":"B","sz":"86.0","executedSz":"49.0413","executedNtl":"107736.21719","minutes":60,"reduceOnly":false,"randomize":false,"timestamp":1770011558151,"trigger":null,"stopPx":null},"status":{"status":"error","description":"Insufficient spot balance"},"twapId":1560286}]}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), true), "wsHandleData must not error")
	exp := &WsUserTWAPHistory{
		IsSnapshot: true,
		User:       "0x0000000000000000000000000000000000000007",
		History: []TWAPHistoryEntry{
			{
				Time: types.Time(time.Unix(1769165674, 0)),
				State: TWAPState{
					Coin:             "@107",
					User:             "0x0000000000000000000000000000000000000007",
					Side:             "B",
					Size:             2000,
					ExecutedSize:     2000,
					ExecutedNotional: 41959.56859,
					Minutes:          30,
					ReduceOnly:       true,
					Randomise:        true,
					Timestamp:        milli(1769163872991),
					Trigger:          &TWAPTrigger{Price: 88, Above: true},
					StopPrice:        85.5,
				},
				Status: TWAPStatus{Status: "finished", Description: "finished"},
				TWAPID: 1530771,
			},
			{
				Time: types.Time(time.Unix(1770013630, 0)),
				State: TWAPState{
					Coin:             "@151",
					User:             "0x0000000000000000000000000000000000000007",
					Side:             "B",
					Size:             86,
					ExecutedSize:     49.0413,
					ExecutedNotional: 107736.21719,
					Minutes:          60,
					Timestamp:        milli(1770011558151),
				},
				Status: TWAPStatus{Status: "error", Description: "Insufficient spot balance"},
				TWAPID: 1560286,
			},
		},
	}
	assert.Equal(t, exp, receiveWebsocketData(t, ex), "wsHandleUserTWAPHistory should relay the TWAP status changes")
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"userTwapHistory","data":{"user":"0x0000000000000000000000000000000000000007","history":[]}}`), true), "wsHandleData must not error without changes")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleOutcomeMetaUpdates(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	frame := `{"channel":"outcomeMetaUpdates","data":[{"outcomeCreated":{"outcome":9115,"name":"template:binaryPrice","description":"perp:HYPE|priceDescription:HYPE-USDC perp mark|seconds:60|threshold:93.058|time:20261006-1015","sideSpecs":[{"name":"template:Yes"},{"name":"template:No"}],"quoteToken":"USDC","venue":"out","deployerFeeScale":"1.0"}},{"outcomeSettled":9112},{"questionUpdated":{"question":400,"name":"template:sportsContestResult","description":"competition:English Premier League|contestType:Match|participantA:Coventry City|participantB:Newcastle United","fallbackOutcome":9068,"namedOutcomes":[9069,9070,9071],"settledNamedOutcomes":[9069]}},{"questionSettled":400}]}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(frame), false), "wsHandleData must not error")
	exp := []WsOutcomeMetaUpdate{
		{OutcomeCreated: &OutcomeSpecification{
			Outcome:            9115,
			Name:               "template:binaryPrice",
			Description:        "perp:HYPE|priceDescription:HYPE-USDC perp mark|seconds:60|threshold:93.058|time:20261006-1015",
			SideSpecifications: []OutcomeSideSpecification{{Name: "template:Yes"}, {Name: "template:No"}},
			QuoteToken:         currency.USDC,
			Venue:              "out",
			DeployerFeeScale:   1,
		}},
		{OutcomeSettled: new(uint64(9112))},
		{QuestionUpdated: &OutcomeQuestion{
			Question:             400,
			Name:                 "template:sportsContestResult",
			Description:          "competition:English Premier League|contestType:Match|participantA:Coventry City|participantB:Newcastle United",
			FallbackOutcome:      9068,
			NamedOutcomes:        []uint64{9069, 9070, 9071},
			SettledNamedOutcomes: []uint64{9069},
		}},
		{QuestionSettled: new(uint64(400))},
	}
	assert.Equal(t, exp, receiveWebsocketData(t, ex), "wsHandleOutcomeMetaUpdates should relay the outcome changes")
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"outcomeMetaUpdates","data":[]}`), false), "wsHandleData must not error without changes")
	assertNoWebsocketData(t, ex)
}

func TestWsHandleFastAssetContexts(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	for _, tc := range []struct {
		name  string
		frame string
		exp   map[string]WsFastAssetContext
	}{
		{
			name:  "snapshot",
			frame: `{"channel":"fastAssetCtxs","data":"q1ZyCnFWsqpWyk0syg6oULJSsjC1MDTRM1DSUcrNTIGLGOqZKtXqKDkYGpijqLY00rM0sURSDBYAq1U2NLGwMEBRbaBnaAgUNUVSDxcC6qiorLKKiIwyNEDVZWxoaGaO4iKgiLkByEW1AA=="}`,
			exp: map[string]WsFastAssetContext{
				"BTC":        {MarkPrice: 85814, MidPrice: 85811.5, MarkPriceSet: true, MidPriceSet: true},
				"@107":       {MarkPrice: 92.949, MidPrice: 92.945, MarkPriceSet: true, MidPriceSet: true},
				"#14880":     {MarkPrice: 0.111485, MidPrice: 0.111485, MarkPriceSet: true, MidPriceSet: true},
				"xyz:XYZ100": {MarkPrice: 31167, MidPrice: 31170.5, MarkPriceSet: true, MidPriceSet: true},
			},
		},
		{
			name:  "update",
			frame: `{"channel":"fastAssetCtxs","data":"q1ZyCnFWsqpWys1MCahQslKyMLUwNNIzVarVUXIwNDAHSyUWZYPlLI30LMEyGZV5iVauIR4oskYmBgZ6lko6MKPySnNyamsB"}`,
			exp: map[string]WsFastAssetContext{
				"BTC":      {MidPrice: 85812.5, MidPriceSet: true},
				"@107":     {MarkPrice: 92.95, MarkPriceSet: true},
				"hyna:ETH": {MarkPrice: 2400.9, MarkPriceSet: true, MidPriceSet: true},
			},
		},
	} {
		require.NoErrorf(t, ex.wsHandleData(t.Context(), []byte(tc.frame), false), "wsHandleData must not error for the %s", tc.name)
		assert.Equalf(t, tc.exp, receiveWebsocketData(t, ex), "wsHandleFastAssetContexts should relay the %s", tc.name)
	}

	for _, tc := range []struct {
		name string
		data string
		err  error
	}{
		{name: "an object", data: `{}`},
		{name: "invalid base64", data: `"!!!"`},
		{name: "invalid DEFLATE data", data: `"` + base64.StdEncoding.EncodeToString([]byte("not compressed")) + `"`},
		{name: "a JSON array", data: `"` + deflateBase64(t, []byte(`[1]`)) + `"`},
		{name: "an oversized message", data: `"` + deflateBase64(t, bytes.Repeat([]byte(" "), maximumFastAssetContextsSize+1)) + `"`, err: errWebsocketServer},
	} {
		err := ex.wsHandleData(t.Context(), []byte(`{"channel":"fastAssetCtxs","data":`+tc.data+`}`), false)
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "wsHandleFastAssetContexts should reject %s", tc.name)
		} else {
			assert.Errorf(t, err, "wsHandleFastAssetContexts should reject %s", tc.name)
		}
	}
	assertNoWebsocketData(t, ex)
}

func TestWsFastAssetContextUnmarshalJSON(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		data string
		exp  WsFastAssetContext
	}{
		{data: `{"markPx":"85814.0","midPx":"85811.5"}`, exp: WsFastAssetContext{MarkPrice: 85814, MidPrice: 85811.5, MarkPriceSet: true, MidPriceSet: true}},
		{data: `{"midPx":"85812.5"}`, exp: WsFastAssetContext{MidPrice: 85812.5, MidPriceSet: true}},
		{data: `{"markPx":"2400.9","midPx":null}`, exp: WsFastAssetContext{MarkPrice: 2400.9, MarkPriceSet: true, MidPriceSet: true}},
		{data: `{}`},
	} {
		var fastContext WsFastAssetContext
		require.NoErrorf(t, json.Unmarshal([]byte(tc.data), &fastContext), "Unmarshal must not error for %s", tc.data)
		assert.Equalf(t, tc.exp, fastContext, "Unmarshal should record which prices %s sets", tc.data)
	}
	for _, data := range []string{`[]`, `{"markPx":true}`, `{"midPx":{}}`} {
		var fastContext WsFastAssetContext
		assert.Errorf(t, json.Unmarshal([]byte(data), &fastContext), "Unmarshal should reject %s", data)
	}
}

// replyToPost returns a reply that answers a post request with the response, echoing the request's ID
func replyToPost(response string) func([]byte) []string {
	return func(request []byte) []string {
		var post WsPostRequest
		if err := json.Unmarshal(request, &post); err != nil {
			return nil
		}
		return []string{`{"channel":"post","data":{"id":` + strconv.FormatUint(post.ID, 10) + `,"response":` + response + `}}`}
	}
}

func TestSendWebsocketPost(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	connection := websocketFixture(t, ex.Websocket.Conn)
	connection.reply = replyToPost(`{"type":"info","payload":{"type":"l2Book","data":null}}`)
	payload, err := ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "l2Book", Coin: "BTC"})
	require.NoError(t, err, "sendWebsocketPost must not error")
	assert.JSONEq(t, `{"type":"l2Book","data":null}`, string(payload), "sendWebsocketPost should return the response payload")

	ex.websocketPostID.Store(math.MaxUint64 - 1)
	_, err = ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "l2Book", Coin: "BTC"})
	require.NoError(t, err, "sendWebsocketPost must match the largest request ID")
	assert.Equal(t, []string{
		`{"method":"post","id":1,"request":{"type":"info","payload":{"type":"l2Book","coin":"BTC"}}}`,
		`{"method":"post","id":18446744073709551615,"request":{"type":"info","payload":{"type":"l2Book","coin":"BTC"}}}`,
	}, connection.sentRequests(), "sendWebsocketPost should send each post with a new ID")

	connection.reply = replyToPost(`{"type":"error","payload":"500 Internal Server Error"}`)
	_, err = ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "l2Book", Coin: "BTC"})
	assert.ErrorIs(t, err, errWebsocketPost, "sendWebsocketPost should return a rejected request's error")
	assert.ErrorContains(t, err, "500 Internal Server Error", "sendWebsocketPost should return the server's message")

	connection.reply = replyToPost(`{"type":"error","payload":{"code":500}}`)
	_, err = ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "l2Book", Coin: "BTC"})
	assert.ErrorIs(t, err, errWebsocketPost, "sendWebsocketPost should return a rejected request's error")
	assert.ErrorContains(t, err, `{"code":500}`, "sendWebsocketPost should return a structured error as it was sent")

	connection.reply = replyToPost(`{"type":"action","payload":{}}`)
	_, err = ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "l2Book", Coin: "BTC"})
	assert.ErrorIs(t, err, errWebsocketPost, "sendWebsocketPost should reject a response of another type")

	connection.reply = func(request []byte) []string {
		message, err := json.Marshal(wsErrorParseFailure + string(request))
		if err != nil {
			return nil
		}
		return []string{`{"channel":"error","data":` + string(message) + `}`}
	}
	_, err = ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "bogusInfoType"})
	assert.ErrorIs(t, err, errWebsocketPost, "sendWebsocketPost should return the error of a request the server cannot parse")
	assert.ErrorContains(t, err, wsErrorParseFailure, "sendWebsocketPost should return the server's parse error")

	connection.reply = nil
	_, err = ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "l2Book", Coin: "BTC"})
	assert.ErrorIs(t, err, websocket.ErrSignatureTimeout, "sendWebsocketPost should time out without a response")

	connection.sendErr = errWebsocketFixture
	_, err = ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "l2Book", Coin: "BTC"})
	assert.ErrorIs(t, err, errWebsocketFixture, "sendWebsocketPost should return a send error")

	ex.Websocket.Conn = nil
	_, err = ex.sendWebsocketPost(t.Context(), &InfoRequest{Type: "l2Book", Coin: "BTC"})
	assert.ErrorIs(t, err, websocket.ErrNotConnected, "sendWebsocketPost should require a connection")
}

func TestWsPostInfo(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	assert.ErrorIs(t, ex.WsPostInfo(t.Context(), nil, new(L2Book)), common.ErrNilPointer, "WsPostInfo should require a payload")
	assert.ErrorIs(t, ex.WsPostInfo(t.Context(), &InfoRequest{Type: "l2Book", Coin: "ETH"}, nil), common.ErrNilPointer, "WsPostInfo should require a result")

	connection := websocketFixture(t, ex.Websocket.Conn)
	connection.reply = replyToPost(`{"type":"info","payload":{"type":"l2Book","data":{"coin":"ETH","time":1791279145054,"levels":[[{"px":"2702.5","sz":"138.1974","n":17}],[{"px":"2702.6","sz":"84.4867","n":17}]],"spread":"0.1"}}}`)
	var book *L2Book
	require.NoError(t, ex.WsPostInfo(t.Context(), &InfoRequest{Type: "l2Book", Coin: "ETH", SignificantFigures: 3}, &book), "WsPostInfo must not error")
	exp := &L2Book{
		Coin:   "ETH",
		Time:   milli(1791279145054),
		Levels: [][]L2Level{{{Price: 2702.5, Size: 138.1974, OrderCount: 17}}, {{Price: 2702.6, Size: 84.4867, OrderCount: 17}}},
		Spread: 0.1,
	}
	assert.Equal(t, exp, book, "WsPostInfo should decode the info response")
	assert.Equal(t, []string{`{"method":"post","id":1,"request":{"type":"info","payload":{"type":"l2Book","coin":"ETH","nSigFigs":3}}}`}, connection.sentRequests(), "WsPostInfo should post the info request")

	connection.reply = replyToPost(`{"type":"info","payload":{"type":"l2Book","data":null}}`)
	book = nil
	require.NoError(t, ex.WsPostInfo(t.Context(), &InfoRequest{Type: "l2Book", Coin: "NOTACOIN"}, &book), "WsPostInfo must not error for a null response")
	assert.Nil(t, book, "WsPostInfo should leave the result unset for a null response")

	connection.reply = replyToPost(`{"type":"info","payload":"invalid"}`)
	assert.Error(t, ex.WsPostInfo(t.Context(), &InfoRequest{Type: "l2Book", Coin: "ETH"}, &book), "WsPostInfo should reject an invalid info payload")
	connection.reply = replyToPost(`{"type":"info","payload":{"type":"l2Book","data":"invalid"}}`)
	assert.Error(t, ex.WsPostInfo(t.Context(), &InfoRequest{Type: "l2Book", Coin: "ETH"}, &book), "WsPostInfo should reject a response that does not fit the result")
	connection.reply = replyToPost(`{"type":"error","payload":"500 Internal Server Error"}`)
	assert.ErrorIs(t, ex.WsPostInfo(t.Context(), &InfoRequest{Type: "l2Book", Coin: "ETH"}, &book), errWebsocketPost, "WsPostInfo should return a rejected request's error")
}

func TestWsHandlePostResponse(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	assert.ErrorIs(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"post","data":{"id":7,"response":{"type":"info","payload":{}}}}`), false), websocket.ErrSignatureNotMatched, "wsHandlePostResponse should reject a response without a request")
	responses, err := ex.Websocket.Match.Set(uint64(7), 1)
	require.NoError(t, err, "Set must not error")
	data := `{"id":7,"response":{"type":"info","payload":{}}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"post","data":`+data+`}`), false), "wsHandleData must not error")
	assert.JSONEq(t, data, string(<-responses), "wsHandlePostResponse should deliver the response to its request")
}

// testOrderbookSubscription returns an unstored BTC orderbook subscription and its pending operation key
func testOrderbookSubscription() (*subscription.Subscription, websocketPendingKey) {
	return &subscription.Subscription{
		Channel:          subscription.OrderbookChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{perpetualPair},
		QualifiedChannel: "l2Book:perpetualcontract:BTC-USDC",
	}, websocketPendingKey{subscription: WsSubscription{Type: wsChannelOrderbook, Coin: "BTC"}}
}

// testTradesSubscription returns an unstored BTC trades subscription and its pending operation key
func testTradesSubscription() (*subscription.Subscription, websocketPendingKey) {
	return &subscription.Subscription{
		Channel:          subscription.AllTradesChannel,
		Asset:            asset.PerpetualContract,
		Pairs:            currency.Pairs{perpetualPair},
		QualifiedChannel: "trades:perpetualcontract:BTC-USDC",
	}, websocketPendingKey{subscription: WsSubscription{Type: wsChannelTrades, Coin: "BTC"}}
}

func TestWsHandleSubscriptionResponse(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	for _, data := range []string{
		`"invalid"`,
		`{"method":"post","subscription":{"type":"trades","coin":"BTC"}}`,
		`{"method":"subscribe","subscription":{"type":" "}}`,
	} {
		err := ex.wsHandleData(t.Context(), []byte(`{"channel":"subscriptionResponse","data":`+data+`}`), false)
		assert.ErrorIsf(t, err, errWebsocketSubscription, "wsHandleSubscriptionResponse should reject %s", data)
	}
	assert.NoError(t, ex.wsHandleData(t.Context(), []byte(`{"channel":"subscriptionResponse","data":{"method":"subscribe","subscription":{"type":"trades","coin":"ETH"}}}`), false), "wsHandleSubscriptionResponse should ignore an acknowledgement without an operation")

	sub, key := testOrderbookSubscription()
	pending := addWebsocketPending(t, ex, wsMethodSubscribe, sub, &key)
	ack := []byte(`{"channel":"subscriptionResponse","data":{"method":"subscribe","subscription":{"type":"l2Book","coin":"BTC","nSigFigs":null,"mantissa":null,"fast":false}}}`)
	require.NoError(t, ex.wsHandleData(t.Context(), ack, true), "wsHandleData must not error for an acknowledgement on the other connection")
	assert.Same(t, pending, pendingWebsocketOperation(ex, &key), "wsHandleSubscriptionResponse should not settle another connection's operation")
	require.NoError(t, ex.wsHandleData(t.Context(), ack, false), "wsHandleData must not error")
	require.NoError(t, <-pending.done, "the acknowledged subscribe must succeed")
	assert.Equal(t, subscription.SubscribedState, sub.State(), "wsHandleSubscriptionResponse should mark the subscription subscribed")
	assert.Nil(t, pendingWebsocketOperation(ex, &key), "wsHandleSubscriptionResponse should remove the settled operation")
}

func TestCompleteWebsocketPending(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	sub, key := testOrderbookSubscription()
	settled, err := ex.completeWebsocketPending(false, wsMethodSubscribe, &key.subscription)
	assert.False(t, settled, "completeWebsocketPending should report an unknown operation as not settled")
	assert.NoError(t, err, "completeWebsocketPending should ignore an unknown operation")

	pending := addWebsocketPending(t, ex, wsMethodSubscribe, sub, &key)
	settled, err = ex.completeWebsocketPending(false, wsMethodUnsubscribe, &key.subscription)
	assert.True(t, settled, "completeWebsocketPending should report an operation completed with another method as settled")
	assert.ErrorIs(t, err, errWebsocketSubscription, "completeWebsocketPending should reject a completion with another method")
	assert.Same(t, pending, pendingWebsocketOperation(ex, &key), "completeWebsocketPending should keep an operation completed with another method")

	settled, err = ex.completeWebsocketPending(false, wsMethodSubscribe, &key.subscription)
	assert.True(t, settled, "completeWebsocketPending should report the operation as settled")
	require.NoError(t, err, "completeWebsocketPending must not error")
	require.NoError(t, <-pending.done, "the completed subscribe must succeed")
	assert.Equal(t, subscription.SubscribedState, sub.State(), "completeWebsocketPending should mark the subscription subscribed")

	pending = addWebsocketPending(t, ex, wsMethodUnsubscribe, sub, &key)
	_, err = ex.completeWebsocketPending(false, wsMethodUnsubscribe, &key.subscription)
	require.NoError(t, err, "completeWebsocketPending must not error for an unsubscribe")
	require.NoError(t, <-pending.done, "the completed unsubscribe must succeed")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "completeWebsocketPending should remove the unsubscribed subscription")

	conflict := newSubscribedWebsocketSubscription(t, ex, &subscription.Subscription{Channel: wsChannelAllMids, QualifiedChannel: wsChannelAllMids})
	conflictKey := websocketPendingKey{subscription: WsSubscription{Type: wsChannelAllMids}}
	pending = &websocketPendingOperation{method: wsMethodSubscribe, connection: ex.Websocket.Conn, subscription: conflict, done: make(chan error, 1)}
	registerWebsocketPending(ex, &conflictKey, pending)
	_, err = ex.completeWebsocketPending(false, wsMethodSubscribe, &conflictKey.subscription)
	assert.ErrorIs(t, err, subscription.ErrInStateAlready, "completeWebsocketPending should return a state conflict")
	assert.ErrorIs(t, <-pending.done, subscription.ErrInStateAlready, "completeWebsocketPending should fail the operation with the state conflict")
	assert.Nil(t, ex.Websocket.GetSubscription(conflict), "completeWebsocketPending should roll back a subscribe it cannot complete")

	unstored := &subscription.Subscription{Channel: wsChannelOutcomeMetaUpdates, QualifiedChannel: wsChannelOutcomeMetaUpdates}
	require.NoError(t, unstored.SetState(subscription.SubscribedState), "SetState must not error")
	unstoredKey := websocketPendingKey{subscription: WsSubscription{Type: wsChannelOutcomeMetaUpdates}}
	pending = addWebsocketPending(t, ex, wsMethodUnsubscribe, unstored, &unstoredKey)
	_, err = ex.completeWebsocketPending(false, wsMethodUnsubscribe, &unstoredKey.subscription)
	assert.ErrorIs(t, err, subscription.ErrNotFound, "completeWebsocketPending should return a store error")
	assert.ErrorIs(t, <-pending.done, subscription.ErrNotFound, "completeWebsocketPending should fail the operation with the store error")
	assert.Equal(t, subscription.SubscribedState, unstored.State(), "completeWebsocketPending should restore the state of an unsubscribe it cannot complete")
}

func TestFailWebsocketPendingOperation(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	sub, key := testOrderbookSubscription()
	assert.False(t, ex.failWebsocketPendingOperation(false, wsMethodSubscribe, &key.subscription, errWebsocketFixture), "failWebsocketPendingOperation should report an unknown operation as not failed")
	pending := addWebsocketPending(t, ex, wsMethodSubscribe, sub, &key)
	assert.False(t, ex.failWebsocketPendingOperation(false, wsMethodUnsubscribe, &key.subscription, errWebsocketFixture), "failWebsocketPendingOperation should not fail an operation of another method")
	assert.False(t, ex.failWebsocketPendingOperation(true, wsMethodSubscribe, &key.subscription, errWebsocketFixture), "failWebsocketPendingOperation should not fail another connection's operation")
	assert.Same(t, pending, pendingWebsocketOperation(ex, &key), "failWebsocketPendingOperation should keep an operation it does not fail")
	assert.True(t, ex.failWebsocketPendingOperation(false, wsMethodSubscribe, &key.subscription, errWebsocketFixture), "failWebsocketPendingOperation should fail the operation")
	assert.ErrorIs(t, <-pending.done, errWebsocketFixture, "failWebsocketPendingOperation should fail the operation with the cause")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "failWebsocketPendingOperation should roll back the subscribe")
	assert.Nil(t, pendingWebsocketOperation(ex, &key), "failWebsocketPendingOperation should remove the operation")
}

func TestRollbackWebsocketPending(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	assert.ErrorIs(t, ex.rollbackWebsocketPending(nil), common.ErrNilPointer, "rollbackWebsocketPending should reject a nil operation")
	assert.ErrorIs(t, ex.rollbackWebsocketPending(&websocketPendingOperation{}), common.ErrNilPointer, "rollbackWebsocketPending should reject an operation without a subscription")

	sub, _ := testOrderbookSubscription()
	require.NoError(t, ex.Websocket.AddSubscriptions(ex.Websocket.Conn, sub), "AddSubscriptions must not error")
	require.NoError(t, ex.rollbackWebsocketPending(&websocketPendingOperation{method: wsMethodSubscribe, connection: ex.Websocket.Conn, subscription: sub}), "rollbackWebsocketPending must not error for a subscribe")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "rollbackWebsocketPending should remove a subscribe's subscription")

	unsubscribe := &subscription.Subscription{Channel: wsChannelAllMids}
	require.NoError(t, unsubscribe.SetState(subscription.SubscribedState), "SetState must not error")
	operation := &websocketPendingOperation{method: wsMethodUnsubscribe, connection: ex.Websocket.Conn, subscription: unsubscribe, previousState: subscription.SubscribedState}
	require.NoError(t, ex.rollbackWebsocketPending(operation), "rollbackWebsocketPending must not error for a subscription in its previous state")
	require.NoError(t, unsubscribe.SetState(subscription.UnsubscribingState), "SetState must not error")
	require.NoError(t, ex.rollbackWebsocketPending(operation), "rollbackWebsocketPending must not error for an unsubscribe")
	assert.Equal(t, subscription.SubscribedState, unsubscribe.State(), "rollbackWebsocketPending should restore an unsubscribe's previous state")

	operation.method = "post"
	assert.ErrorIs(t, ex.rollbackWebsocketPending(operation), common.ErrNotYetImplemented, "rollbackWebsocketPending should reject another method")
}

func TestFailWebsocketPending(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	assert.NoError(t, ex.failWebsocketPending(false, nil), "failWebsocketPending should not error without operations or a cause")

	publicSub, publicKey := testOrderbookSubscription()
	publicPending := addWebsocketPending(t, ex, wsMethodSubscribe, publicSub, &publicKey)
	invalidKey := websocketPendingKey{subscription: WsSubscription{Type: wsChannelAllMids}}
	invalidPending := &websocketPendingOperation{method: "post", connection: ex.Websocket.Conn, subscription: &subscription.Subscription{}, done: make(chan error, 1)}
	registerWebsocketPending(ex, &invalidKey, invalidPending)
	authenticatedSub := &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true, QualifiedChannel: wsChannelOrderUpdates}
	authenticatedKey := websocketPendingKey{authenticated: true, subscription: WsSubscription{Type: wsChannelOrderUpdates, User: testAccountAddress}}
	authenticatedPending := addWebsocketPending(t, ex, wsMethodSubscribe, authenticatedSub, &authenticatedKey)

	err := ex.failWebsocketPending(false, errWebsocketFixture)
	assert.ErrorIs(t, err, errWebsocketFixture, "failWebsocketPending should return the cause")
	assert.ErrorIs(t, err, common.ErrNotYetImplemented, "failWebsocketPending should return rollback errors")
	assert.ErrorIs(t, <-publicPending.done, errWebsocketFixture, "failWebsocketPending should fail each operation with the cause")
	assert.ErrorIs(t, <-invalidPending.done, common.ErrNotYetImplemented, "failWebsocketPending should fail an operation with its rollback error")
	assert.Nil(t, ex.Websocket.GetSubscription(publicSub), "failWebsocketPending should roll back the failed subscribe")
	assert.Same(t, authenticatedPending, pendingWebsocketOperation(ex, &authenticatedKey), "failWebsocketPending should keep the other connection's operations")

	assert.ErrorIs(t, ex.failWebsocketPending(true, errWebsocketServer), errWebsocketServer, "failWebsocketPending should return the cause")
	assert.ErrorIs(t, <-authenticatedPending.done, errWebsocketServer, "failWebsocketPending should fail the account connection's operations")
	assert.Nil(t, ex.Websocket.GetSubscription(authenticatedSub), "failWebsocketPending should roll back the account subscribe")
	assert.Empty(t, ex.websocketPending, "failWebsocketPending should remove every failed operation")
}

func TestAbortWebsocketPending(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	sub, key := testOrderbookSubscription()
	pending := addWebsocketPending(t, ex, wsMethodSubscribe, sub, &key)
	assert.ErrorIs(t, ex.abortWebsocketPending(&key, pending, errWebsocketFixture), errWebsocketFixture, "abortWebsocketPending should return the cause")
	assert.Nil(t, pendingWebsocketOperation(ex, &key), "abortWebsocketPending should remove the operation")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "abortWebsocketPending should roll back the operation")

	settled := &websocketPendingOperation{done: make(chan error, 1)}
	settled.done <- errWebsocketServer
	err := ex.abortWebsocketPending(&key, settled, errWebsocketFixture)
	assert.ErrorIs(t, err, errWebsocketServer, "abortWebsocketPending should return the outcome of an operation the server settled")
	assert.NotErrorIs(t, err, errWebsocketFixture, "abortWebsocketPending should discard the cause of an operation the server settled")
}

// websocketErrorFrame returns an error frame holding the message
func websocketErrorFrame(t *testing.T, message string) []byte {
	t.Helper()
	encoded, err := json.Marshal(message)
	require.NoError(t, err, "Marshal must not error")
	return []byte(`{"channel":"error","data":` + string(encoded) + `}`)
}

func TestWsHandleError(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	tradesSub, tradesKey := testTradesSubscription()
	pending := addWebsocketPending(t, ex, wsMethodSubscribe, tradesSub, &tradesKey)
	ex.expectTradeReplay("BTC")
	require.NoError(t, ex.wsHandleData(t.Context(), websocketErrorFrame(t, wsErrorAlreadySubscribed+`{"type":"trades","coin":"BTC"}`), false), "wsHandleData must not error for a duplicate subscribe")
	require.NoError(t, <-pending.done, "a duplicate subscribe must succeed, as the feed is active")
	assert.Equal(t, subscription.SubscribedState, tradesSub.State(), "wsHandleError should mark a duplicate subscribe subscribed")
	assert.False(t, ex.consumeTradeReplay("BTC"), "wsHandleError should not expect a replay for a duplicate trades subscribe")

	pending = addWebsocketPending(t, ex, wsMethodUnsubscribe, tradesSub, &tradesKey)
	require.NoError(t, ex.wsHandleData(t.Context(), websocketErrorFrame(t, wsErrorAlreadyUnsubscribed+`{"type":"trades","coin":"BTC"}`), false), "wsHandleData must not error for a duplicate unsubscribe")
	require.NoError(t, <-pending.done, "a duplicate unsubscribe must succeed, as the feed is inactive")
	assert.Nil(t, ex.Websocket.GetSubscription(tradesSub), "wsHandleError should remove a duplicate unsubscribe's subscription")

	for _, message := range []string{
		wsErrorAlreadySubscribed + `{"type":"allMids"}`,
		wsErrorAlreadySubscribed + `invalid`,
		wsErrorAlreadyUnsubscribed + `{"type":"trades","coin":"ETH"}`,
		wsErrorAlreadyUnsubscribed + `invalid`,
	} {
		err := ex.wsHandleData(t.Context(), websocketErrorFrame(t, message), false)
		assert.ErrorIsf(t, err, errWebsocketServer, "wsHandleError should return %q without an operation", message)
		assert.ErrorContainsf(t, err, message, "wsHandleError should return the server's message %q", message)
	}

	bookSub, bookKey := testOrderbookSubscription()
	bookPending := addWebsocketPending(t, ex, wsMethodSubscribe, bookSub, &bookKey)
	assetDataSub := &subscription.Subscription{Channel: wsChannelActiveAssetData, Authenticated: true, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, QualifiedChannel: "activeAssetData:perpetualcontract:BTC-USDC"}
	assetDataKey := websocketPendingKey{authenticated: true, subscription: WsSubscription{Type: wsChannelActiveAssetData, Coin: "BTC", User: testAccountAddress}}
	assetDataPending := addWebsocketPending(t, ex, wsMethodSubscribe, assetDataSub, &assetDataKey)
	err := ex.wsHandleData(t.Context(), websocketErrorFrame(t, wsErrorInvalidSubscription+`{"type":"activeAssetData","user":"`+testAccountAddress+`","coin":"BTC"}`), true)
	assert.ErrorIs(t, err, errWebsocketServer, "wsHandleError should return an invalid subscription's error")
	assert.ErrorIs(t, <-assetDataPending.done, errWebsocketServer, "wsHandleError should fail the invalid subscription")
	assert.Nil(t, ex.Websocket.GetSubscription(assetDataSub), "wsHandleError should roll back the invalid subscription")
	assert.Same(t, bookPending, pendingWebsocketOperation(ex, &bookKey), "wsHandleError should keep the operations an error does not answer")

	candleSub := &subscription.Subscription{Channel: subscription.CandlesChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, Interval: kline.OneMin, QualifiedChannel: "candle:perpetualcontract:BTC-USDC:1m"}
	candleKey := websocketPendingKey{subscription: WsSubscription{Type: wsChannelCandle, Coin: "BTC", Interval: "1m"}}
	candlePending := addWebsocketPending(t, ex, wsMethodSubscribe, candleSub, &candleKey)
	err = ex.wsHandleData(t.Context(), websocketErrorFrame(t, wsErrorParseFailure+`{"method":"subscribe","subscription":{"type":"candle","coin":"BTC","interval":"1m"}}`), false)
	assert.ErrorIs(t, err, errWebsocketServer, "wsHandleError should return a parse failure's error")
	assert.ErrorIs(t, <-candlePending.done, errWebsocketServer, "wsHandleError should fail the subscription the server could not parse")
	assert.Same(t, bookPending, pendingWebsocketOperation(ex, &bookKey), "wsHandleError should keep the operations a parse failure does not answer")

	responses, err := ex.Websocket.Match.Set(uint64(4), 1)
	require.NoError(t, err, "Set must not error")
	postFailure := wsErrorParseFailure + `{"method":"post","id":4,"request":{"type":"info","payload":{"type":"bogusInfoType"}}}`
	require.NoError(t, ex.wsHandleData(t.Context(), websocketErrorFrame(t, postFailure), false), "wsHandleData must not error for a post the server could not parse")
	var response WsPostResponse
	require.NoError(t, json.Unmarshal(<-responses, &response), "Unmarshal must not error")
	encodedFailure, err := json.Marshal(postFailure)
	require.NoError(t, err, "Marshal must not error")
	assert.Equal(t, WsPostResponse{ID: 4, Response: WsPostResponseBody{Type: wsPostTypeError, Payload: encodedFailure}}, response, "wsHandleError should answer the post with the server's error")
	err = ex.wsHandleData(t.Context(), websocketErrorFrame(t, postFailure), false)
	assert.ErrorIs(t, err, errWebsocketServer, "wsHandleError should return a post parse failure without a request")
	assert.Same(t, bookPending, pendingWebsocketOperation(ex, &bookKey), "wsHandleError should keep the operations a post parse failure does not answer")
	require.True(t, ex.failWebsocketPendingOperation(false, wsMethodSubscribe, &bookKey.subscription, errWebsocketFixture), "failWebsocketPendingOperation must fail the remaining operation")
	require.ErrorIs(t, <-bookPending.done, errWebsocketFixture, "the remaining operation must fail")

	for _, message := range []string{
		wsErrorInvalidSubscription + `{"type":"activeAssetData","user":"` + testAccountAddress + `","coin":"@107"}`,
		wsErrorParseFailure + `{"method":"subscribe","subscription":{"type":"bogus"}}`,
		wsErrorParseFailure + `not json`,
		"Websocket server error",
	} {
		sub, key := testOrderbookSubscription()
		pending := addWebsocketPending(t, ex, wsMethodSubscribe, sub, &key)
		err := ex.wsHandleData(t.Context(), websocketErrorFrame(t, message), false)
		assert.ErrorIsf(t, err, errWebsocketServer, "wsHandleError should return %q", message)
		assert.ErrorIsf(t, <-pending.done, errWebsocketServer, "wsHandleError should fail every operation on the connection for %q", message)
		assert.Nilf(t, ex.Websocket.GetSubscription(sub), "wsHandleError should roll back every operation on the connection for %q", message)
	}

	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"error","data":null}`), false)
	assert.ErrorIs(t, err, errWebsocketServer, "wsHandleError should return an empty error")
	assert.ErrorContains(t, err, "unspecified error", "wsHandleError should describe an empty error")
	err = ex.wsHandleData(t.Context(), []byte(`{"channel":"error","data":{"message":"bad request"}}`), false)
	assert.ErrorContains(t, err, `{"message":"bad request"}`, "wsHandleError should return a structured error as it was sent")
}

func TestDecodeWebsocketSubscription(t *testing.T) {
	t.Parallel()
	sub, err := decodeWebsocketSubscription(`{"type":"l2Book","coin":"BTC","nSigFigs":5,"mantissa":2,"fast":false}`)
	require.NoError(t, err, "decodeWebsocketSubscription must not error")
	assert.Equal(t, WsSubscription{Type: wsChannelOrderbook, Coin: "BTC", SignificantFigures: 5, Mantissa: 2}, sub, "decodeWebsocketSubscription should decode the canonical subscription")
	_, err = decodeWebsocketSubscription("invalid")
	assert.Error(t, err, "decodeWebsocketSubscription should reject an invalid subscription")
}

func TestWebsocketChannelName(t *testing.T) {
	t.Parallel()
	_, err := websocketChannelName(nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "websocketChannelName should reject a nil subscription")
	for channel, exp := range map[string]string{
		subscription.TickerChannel:    wsChannelActiveAssetContext,
		subscription.OrderbookChannel: wsChannelOrderbook,
		subscription.AllTradesChannel: wsChannelTrades,
		subscription.CandlesChannel:   wsChannelCandle,
		subscription.MyOrdersChannel:  wsChannelOrderUpdates,
		subscription.MyTradesChannel:  wsSubscriptionUserEvents,
		wsChannelBestBidOffer:         wsChannelBestBidOffer,
		wsChannelFastAssetContexts:    wsChannelFastAssetContexts,
	} {
		name, err := websocketChannelName(&subscription.Subscription{Channel: channel})
		require.NoErrorf(t, err, "websocketChannelName must not error for %s", channel)
		assert.Equalf(t, exp, name, "websocketChannelName should name the subscription type of %s", channel)
	}
	_, err = websocketChannelName(&subscription.Subscription{Channel: wsChannelUserEvents})
	assert.ErrorIs(t, err, subscription.ErrNotSupported, "websocketChannelName should reject a data channel that is not a subscription type")
}

func TestGenerateSubscriptions(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	subs, err := ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")
	exp := make(subscription.List, 0, 14)
	for _, standard := range []struct {
		channel, name string
		interval      kline.Interval
	}{
		{channel: subscription.TickerChannel, name: wsChannelActiveAssetContext},
		{channel: subscription.OrderbookChannel, name: wsChannelOrderbook},
		{channel: subscription.AllTradesChannel, name: wsChannelTrades},
		{channel: subscription.CandlesChannel, name: wsChannelCandle, interval: kline.OneMin},
	} {
		for _, market := range []struct {
			a    asset.Item
			pair currency.Pair
		}{
			{a: asset.PerpetualContract, pair: perpetualPair},
			{a: asset.PerpetualContract, pair: testBuilderPair},
			{a: asset.Spot, pair: spotPair},
		} {
			pair := market.pair.Format(dashFormat)
			qualifiedChannel := standard.name + ":" + market.a.String() + ":" + pair.String()
			if standard.interval != 0 {
				qualifiedChannel += ":1m"
			}
			exp = append(exp, &subscription.Subscription{
				Enabled:          true,
				Channel:          standard.channel,
				Asset:            market.a,
				Pairs:            currency.Pairs{pair},
				Interval:         standard.interval,
				QualifiedChannel: qualifiedChannel,
			})
		}
	}
	exp = append(
		exp,
		&subscription.Subscription{Enabled: true, Channel: subscription.MyOrdersChannel, Authenticated: true, QualifiedChannel: wsChannelOrderUpdates},
		&subscription.Subscription{Enabled: true, Channel: subscription.MyTradesChannel, Authenticated: true, QualifiedChannel: wsSubscriptionUserEvents},
	)
	testsubs.EqualLists(t, exp, subs)
}

func TestGetSubscriptionTemplate(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	tmpl, err := ex.GetSubscriptionTemplate(nil)
	require.NoError(t, err, "GetSubscriptionTemplate must not error")
	require.NotNil(t, tmpl, "GetSubscriptionTemplate must return a template")
	subs, err := subscription.List{
		{Channel: wsChannelAllMids},
		{Channel: wsChannelBestBidOffer, Asset: asset.Spot},
		{Channel: wsChannelUserFills, Authenticated: true},
	}.ExpandTemplates(ex)
	require.NoError(t, err, "ExpandTemplates must not error")
	spotPairs, err := ex.GetEnabledPairs(asset.Spot)
	require.NoError(t, err, "GetEnabledPairs must not error")
	exp := subscription.List{
		{Channel: wsChannelAllMids, QualifiedChannel: wsChannelAllMids},
		{Channel: wsChannelBestBidOffer, Asset: asset.Spot, Pairs: spotPairs, QualifiedChannel: "bbo:spot:HYPE-USDC"},
		{Channel: wsChannelUserFills, Authenticated: true, QualifiedChannel: wsChannelUserFills},
	}
	testsubs.EqualLists(t, exp, subs)

	_, err = subscription.List{{Channel: "bogus"}}.ExpandTemplates(ex)
	assert.ErrorIs(t, err, subscription.ErrNotSupported, "the template should reject an unsupported channel")
}

func TestSubscribe(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	assert.ErrorIs(t, ex.Subscribe(subscription.List{nil}), common.ErrNilPointer, "Subscribe should reject a nil subscription")
	subs := subscription.List{
		{Channel: subscription.OrderbookChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, Levels: 5, Params: map[string]any{"nSigFigs": 5, "mantissa": 2}},
		{Channel: wsChannelAllMids, Params: map[string]any{"dex": "xyz"}},
		{Channel: wsChannelUserFills, Authenticated: true, Params: map[string]any{"aggregateByTime": true}},
		{Channel: "bogus"},
	}
	assert.ErrorIs(t, ex.Subscribe(subs), subscription.ErrNotSupported, "Subscribe should report a subscription it cannot expand")
	assert.Equal(t, []string{
		`{"method":"subscribe","subscription":{"type":"l2Book","coin":"BTC","nSigFigs":5,"mantissa":2,"fast":true}}`,
		`{"method":"subscribe","subscription":{"type":"allMids","dex":"xyz"}}`,
	}, websocketFixture(t, ex.Websocket.Conn).sentRequests(), "Subscribe should send the public subscriptions on the public connection")
	assert.Equal(t, []string{
		`{"method":"subscribe","subscription":{"type":"userFills","user":"` + testAccountAddress + `","aggregateByTime":true}}`,
	}, websocketFixture(t, ex.Websocket.AuthConn).sentRequests(), "Subscribe should send the account subscriptions on the account connection")
	stored := ex.Websocket.GetSubscriptions()
	require.Len(t, stored, 3, "Subscribe must store each subscription")
	for _, sub := range stored {
		assert.Equalf(t, subscription.SubscribedState, sub.State(), "Subscribe should mark %s subscribed", sub)
	}
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	assert.ErrorIs(t, ex.Unsubscribe(subscription.List{nil}), common.ErrNilPointer, "Unsubscribe should reject a nil subscription")
	sub, _ := testOrderbookSubscription()
	assert.ErrorIs(t, ex.Unsubscribe(subscription.List{sub}), subscription.ErrNotFound, "Unsubscribe should reject a subscription it does not hold")
	newSubscribedWebsocketSubscription(t, ex, sub)
	require.NoError(t, ex.Unsubscribe(subscription.List{sub}), "Unsubscribe must not error")
	assert.Equal(t, []string{
		`{"method":"unsubscribe","subscription":{"type":"l2Book","coin":"BTC"}}`,
	}, websocketFixture(t, ex.Websocket.Conn).sentRequests(), "Unsubscribe should send the unsubscribe request")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "Unsubscribe should remove the subscription")
}

func TestManageWebsocketSubscription(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	connection := websocketFixture(t, ex.Websocket.Conn)
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, nil), common.ErrNilPointer, "manageWebsocketSubscription should reject a nil subscription")
	sub, _ := testOrderbookSubscription()
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), "post", sub), common.ErrNotYetImplemented, "manageWebsocketSubscription should reject another method")
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, &subscription.Subscription{Channel: "bogus"}), subscription.ErrNotSupported, "manageWebsocketSubscription should reject an unsupported channel")

	ex.Websocket.Conn = nil
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, sub), common.ErrNilPointer, "manageWebsocketSubscription should require a connection")
	ex.Websocket.Conn = connection

	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, sub), "manageWebsocketSubscription must not error")
	assert.Equal(t, subscription.SubscribedState, sub.State(), "manageWebsocketSubscription should mark an acknowledged subscribe subscribed")
	assert.NotNil(t, ex.Websocket.GetSubscription(sub.Clone()), "manageWebsocketSubscription should store the subscription")
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, sub), subscription.ErrDuplicate, "manageWebsocketSubscription should reject a duplicate subscribe")
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, sub), "manageWebsocketSubscription must not error for an unsubscribe")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "manageWebsocketSubscription should remove an acknowledged unsubscribe")
	assert.Equal(t, []string{
		`{"method":"subscribe","subscription":{"type":"l2Book","coin":"BTC"}}`,
		`{"method":"unsubscribe","subscription":{"type":"l2Book","coin":"BTC"}}`,
	}, connection.sentRequests(), "manageWebsocketSubscription should send each request")
	for _, err := range connection.handled() {
		assert.NoError(t, err, "the acknowledgements should be handled without error")
	}

	trades, _ := testTradesSubscription()
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, trades), "manageWebsocketSubscription must not error for trades")
	assert.True(t, ex.consumeTradeReplay("BTC"), "manageWebsocketSubscription should expect the trades a subscribe replays")
	ex.expectTradeReplay("BTC")
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, trades), "manageWebsocketSubscription must not error for a trades unsubscribe")
	assert.False(t, ex.consumeTradeReplay("BTC"), "manageWebsocketSubscription should not expect a replay after an unsubscribe")

	connection.sendErr = errWebsocketFixture
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, trades), "manageWebsocketSubscription must keep a subscribe acknowledged before its send failed")
	assert.Equal(t, subscription.SubscribedState, trades.State(), "manageWebsocketSubscription should keep an acknowledged subscribe")
	connection.reply = nil
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, trades), errWebsocketFixture, "manageWebsocketSubscription should return a send error")
	assert.Equal(t, subscription.SubscribedState, trades.State(), "manageWebsocketSubscription should restore the state of an unsubscribe that was not sent")
	connection.sendErr = nil
	connection.reply = acknowledge
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, trades), "manageWebsocketSubscription must not error when retrying an unsubscribe")

	connection.sendErr = errWebsocketFixture
	connection.reply = nil
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, trades), errWebsocketFixture, "manageWebsocketSubscription should return a send error")
	assert.Nil(t, ex.Websocket.GetSubscription(trades), "manageWebsocketSubscription should roll back a subscribe that was not sent")
	assert.False(t, ex.consumeTradeReplay("BTC"), "manageWebsocketSubscription should not expect a replay for a failed subscribe")

	removed, _ := testTradesSubscription()
	connection.reply = func([]byte) []string {
		assert.NoError(t, ex.Websocket.RemoveSubscriptions(connection, removed), "RemoveSubscriptions should not error")
		return nil
	}
	err := ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, removed)
	assert.ErrorIs(t, err, errWebsocketFixture, "manageWebsocketSubscription should return a send error")
	assert.ErrorIs(t, err, subscription.ErrNotFound, "manageWebsocketSubscription should return a rollback error")
	connection.sendErr = nil
	connection.reply = acknowledge

	unsubscribing := &subscription.Subscription{Channel: wsChannelAllMids, QualifiedChannel: wsChannelAllMids}
	require.NoError(t, unsubscribing.SetState(subscription.UnsubscribingState), "SetState must not error")
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, unsubscribing), subscription.ErrInStateAlready, "manageWebsocketSubscription should reject an unsubscribe already in progress")

	collisionKey := websocketPendingKey{subscription: WsSubscription{Type: wsChannelAllMids}}
	collision := &websocketPendingOperation{method: wsMethodUnsubscribe, done: make(chan error, 1)}
	registerWebsocketPending(ex, &collisionKey, collision)
	allMids := &subscription.Subscription{Channel: wsChannelAllMids, QualifiedChannel: wsChannelAllMids}
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, allMids), errWebsocketSubscription, "manageWebsocketSubscription should reject an operation already pending for the feed")
	assert.Nil(t, ex.Websocket.GetSubscription(allMids), "manageWebsocketSubscription should roll back an operation it rejects")
	assert.Same(t, collision, pendingWebsocketOperation(ex, &collisionKey), "manageWebsocketSubscription should keep the pending operation")

	ex.websocketPending = nil
	authenticated := &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true, QualifiedChannel: wsChannelOrderUpdates}
	require.NoError(t, ex.manageWebsocketSubscription(t.Context(), wsMethodSubscribe, authenticated), "manageWebsocketSubscription must not error for an account feed")
	assert.Equal(t, []string{
		`{"method":"subscribe","subscription":{"type":"orderUpdates","user":"` + testAccountAddress + `"}}`,
	}, websocketFixture(t, ex.Websocket.AuthConn).sentRequests(), "manageWebsocketSubscription should send account feeds on the account connection")
	ex.Websocket.AuthConn = nil
	assert.ErrorIs(t, ex.manageWebsocketSubscription(t.Context(), wsMethodUnsubscribe, authenticated), common.ErrNilPointer, "manageWebsocketSubscription should require the account connection")
	assert.Equal(t, subscription.SubscribedState, authenticated.State(), "manageWebsocketSubscription should not change a subscription it cannot send")
}

func TestAwaitWebsocketPending(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	connection := websocketFixture(t, ex.Websocket.Conn)
	connection.reply = nil
	subscribeRequest := &WsSubscriptionRequest{Method: wsMethodSubscribe, Subscription: WsSubscription{Type: wsChannelOrderbook, Coin: "BTC"}}

	sub, key := testOrderbookSubscription()
	pending := addWebsocketPending(t, ex, wsMethodSubscribe, sub, &key)
	pending.done <- errWebsocketServer
	assert.ErrorIs(t, ex.awaitWebsocketPending(t.Context(), connection, &key, pending, subscribeRequest), errWebsocketServer, "awaitWebsocketPending should return the server's outcome")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.ErrorIs(t, ex.awaitWebsocketPending(ctx, connection, &key, pending, subscribeRequest), context.Canceled, "awaitWebsocketPending should stop when its context ends")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "awaitWebsocketPending should roll back an operation whose context ended")

	sub, key = testOrderbookSubscription()
	pending = addWebsocketPending(t, ex, wsMethodSubscribe, sub, &key)
	ex.WebsocketResponseMaxLimit = time.Millisecond
	assert.ErrorIs(t, ex.awaitWebsocketPending(t.Context(), connection, &key, pending, subscribeRequest), websocket.ErrSignatureTimeout, "awaitWebsocketPending should time out without an acknowledgement")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "awaitWebsocketPending should roll back an operation that timed out")
	assert.Nil(t, pendingWebsocketOperation(ex, &key), "awaitWebsocketPending should remove an operation that timed out")
}

func TestWebsocketSubscriptionPayload(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	for _, tc := range []struct {
		name string
		sub  *subscription.Subscription
		exp  WsSubscription
		err  error
	}{
		{name: "nil", err: common.ErrNilPointer},
		{name: "unsupported channel", sub: &subscription.Subscription{Channel: "bogus"}, err: subscription.ErrNotSupported},
		{name: "authenticated market feed", sub: &subscription.Subscription{Channel: subscription.TickerChannel, Authenticated: true}, err: subscription.ErrNotSupported},
		{name: "unauthenticated account feed", sub: &subscription.Subscription{Channel: subscription.MyOrdersChannel}, err: subscription.ErrNotSupported},
		{name: "market feed without a pair", sub: &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract}, err: subscription.ErrNotSinglePair},
		{name: "market feed with two pairs", sub: &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair, testBuilderPair}}, err: subscription.ErrNotSinglePair},
		{name: "unknown pair", sub: &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{currency.NewPair(currency.ETH, currency.USDC)}}, err: errPairMappingNotFound},
		{name: "spot activeAssetData", sub: &subscription.Subscription{Channel: wsChannelActiveAssetData, Authenticated: true, Asset: asset.Spot, Pairs: currency.Pairs{spotPair}}, err: asset.ErrNotSupported},
		{name: "feed with an asset", sub: &subscription.Subscription{Channel: wsChannelAllMids, Asset: asset.Spot}, err: subscription.ErrNotSupported},
		{name: "unknown DEX", sub: &subscription.Subscription{Channel: wsChannelAllMids, Params: map[string]any{"dex": "abc"}}, err: errInvalidPerpetualDEX},
		{name: "unsupported interval", sub: &subscription.Subscription{Channel: subscription.CandlesChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, Interval: kline.TenMin}, err: kline.ErrUnsupportedInterval},
		{name: "unsupported orderbook levels", sub: &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, Levels: 10}, err: subscription.ErrInvalidLevel},
		{name: "invalid aggregateByTime", sub: &subscription.Subscription{Channel: wsChannelUserFills, Authenticated: true, Params: map[string]any{"aggregateByTime": "yes"}}, err: errWebsocketParameter},
		{name: "invalid ignorePortfolioMargin", sub: &subscription.Subscription{Channel: wsChannelSpotState, Authenticated: true, Params: map[string]any{"ignorePortfolioMargin": 1}}, err: errWebsocketParameter},
		{
			name: "perpetual ticker",
			sub:  &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}},
			exp:  WsSubscription{Type: wsChannelActiveAssetContext, Coin: "BTC"},
		},
		{
			name: "spot ticker",
			sub:  &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.Spot, Pairs: currency.Pairs{spotPair}},
			exp:  WsSubscription{Type: wsChannelActiveAssetContext, Coin: "@107"},
		},
		{
			name: "builder DEX trades",
			sub:  &subscription.Subscription{Channel: subscription.AllTradesChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{testBuilderPair}},
			exp:  WsSubscription{Type: wsChannelTrades, Coin: "xyz:XYZ100"},
		},
		{
			name: "candles",
			sub:  &subscription.Subscription{Channel: subscription.CandlesChannel, Asset: asset.Spot, Pairs: currency.Pairs{spotPair}, Interval: kline.OneMonth},
			exp:  WsSubscription{Type: wsChannelCandle, Coin: "@107", Interval: "1M"},
		},
		{
			name: "orderbook",
			sub:  &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, Levels: 20},
			exp:  WsSubscription{Type: wsChannelOrderbook, Coin: "BTC"},
		},
		{
			name: "fast aggregated orderbook",
			sub:  &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, Levels: 5, Params: map[string]any{"nSigFigs": 5.0, "mantissa": 2.0}},
			exp:  WsSubscription{Type: wsChannelOrderbook, Coin: "BTC", SignificantFigures: 5, Mantissa: 2, Fast: true},
		},
		{
			name: "best bid and offer",
			sub:  &subscription.Subscription{Channel: wsChannelBestBidOffer, Asset: asset.Spot, Pairs: currency.Pairs{spotPair}},
			exp:  WsSubscription{Type: wsChannelBestBidOffer, Coin: "@107"},
		},
		{
			name: "activeAssetData",
			sub:  &subscription.Subscription{Channel: wsChannelActiveAssetData, Authenticated: true, Asset: asset.PerpetualContract, Pairs: currency.Pairs{testBuilderPair}},
			exp:  WsSubscription{Type: wsChannelActiveAssetData, Coin: "xyz:XYZ100", User: testAccountAddress},
		},
		{
			name: "allMids",
			sub:  &subscription.Subscription{Channel: wsChannelAllMids, Params: map[string]any{"dex": ""}},
			exp:  WsSubscription{Type: wsChannelAllMids},
		},
		{
			name: "builder DEX allMids",
			sub:  &subscription.Subscription{Channel: wsChannelAllMids, Params: map[string]any{"dex": "xyz"}},
			exp:  WsSubscription{Type: wsChannelAllMids, DEX: "xyz"},
		},
		{
			name: "my orders",
			sub:  &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true},
			exp:  WsSubscription{Type: wsChannelOrderUpdates, User: testAccountAddress},
		},
		{
			name: "my trades",
			sub:  &subscription.Subscription{Channel: subscription.MyTradesChannel, Authenticated: true},
			exp:  WsSubscription{Type: wsSubscriptionUserEvents, User: testAccountAddress},
		},
		{
			name: "builder DEX clearinghouseState",
			sub:  &subscription.Subscription{Channel: wsChannelClearinghouseState, Authenticated: true, Params: map[string]any{"dex": "xyz"}},
			exp:  WsSubscription{Type: wsChannelClearinghouseState, User: testAccountAddress, DEX: "xyz"},
		},
		{
			name: "userFills aggregated by time",
			sub:  &subscription.Subscription{Channel: wsChannelUserFills, Authenticated: true, Params: map[string]any{"aggregateByTime": true}},
			exp:  WsSubscription{Type: wsChannelUserFills, User: testAccountAddress, AggregateByTime: true},
		},
		{
			name: "spotState without portfolio margin",
			sub:  &subscription.Subscription{Channel: wsChannelSpotState, Authenticated: true, Params: map[string]any{"ignorePortfolioMargin": true}},
			exp:  WsSubscription{Type: wsChannelSpotState, User: testAccountAddress, IgnorePortfolioMargin: true},
		},
		{
			name: "fastAssetCtxs",
			sub:  &subscription.Subscription{Channel: wsChannelFastAssetContexts},
			exp:  WsSubscription{Type: wsChannelFastAssetContexts},
		},
	} {
		payload, err := ex.websocketSubscriptionPayload(t.Context(), tc.sub)
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "websocketSubscriptionPayload should reject the %s subscription", tc.name)
			continue
		}
		require.NoErrorf(t, err, "websocketSubscriptionPayload must not error for the %s subscription", tc.name)
		assert.Equalf(t, tc.exp, payload, "websocketSubscriptionPayload should build the %s subscription", tc.name)
	}

	watchless := newWebsocketTestExchange(t)
	watchless.SetCredentials(&accounts.Credentials{})
	_, err := watchless.websocketSubscriptionPayload(t.Context(), &subscription.Subscription{Channel: subscription.MyOrdersChannel, Authenticated: true})
	assert.Error(t, err, "websocketSubscriptionPayload should require an account address for an account feed")
}

func TestWebsocketOrderbookOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		levels int
		params map[string]any
		exp    WsSubscription
		err    error
	}{
		{exp: WsSubscription{}},
		{levels: 20, params: map[string]any{"nSigFigs": 4}, exp: WsSubscription{SignificantFigures: 4}},
		{levels: 5, params: map[string]any{"nSigFigs": uint64(5), "mantissa": 5}, exp: WsSubscription{SignificantFigures: 5, Mantissa: 5, Fast: true}},
		{levels: 10, err: subscription.ErrInvalidLevel},
		{params: map[string]any{"nSigFigs": "5"}, err: errWebsocketParameter},
		{params: map[string]any{"mantissa": -1}, err: errWebsocketParameter},
		{params: map[string]any{"nSigFigs": 7}, err: errInvalidSignificantFigures},
		{params: map[string]any{"nSigFigs": 4, "mantissa": 2}, err: errInvalidMantissa},
	} {
		var payload WsSubscription
		err := websocketOrderbookOptions(&subscription.Subscription{Levels: tc.levels, Params: tc.params}, &payload)
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "websocketOrderbookOptions should reject %d levels with %v", tc.levels, tc.params)
			continue
		}
		require.NoErrorf(t, err, "websocketOrderbookOptions must not error for %d levels with %v", tc.levels, tc.params)
		assert.Equalf(t, tc.exp, payload, "websocketOrderbookOptions should set the options of %d levels with %v", tc.levels, tc.params)
	}
}

func TestWebsocketDEXParameter(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestExchange(t)
	for _, tc := range []struct {
		params map[string]any
		exp    string
		err    error
	}{
		{},
		{params: map[string]any{"dex": nil}},
		{params: map[string]any{"dex": ""}},
		{params: map[string]any{"dex": "xyz"}, exp: "xyz"},
		{params: map[string]any{"dex": "XYZ"}, err: errInvalidPerpetualDEX},
		{params: map[string]any{"dex": 1}, err: errWebsocketParameter},
	} {
		dex, err := ex.websocketDEXParameter(t.Context(), tc.params)
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "websocketDEXParameter should reject %v", tc.params)
			continue
		}
		require.NoErrorf(t, err, "websocketDEXParameter must not error for %v", tc.params)
		assert.Equalf(t, tc.exp, dex, "websocketDEXParameter should return the DEX of %v", tc.params)
	}
	unavailable := newUnavailableServerExchange(t)
	_, err := unavailable.websocketDEXParameter(t.Context(), map[string]any{"dex": "xyz"})
	assert.Error(t, err, "websocketDEXParameter should return a registry error")
}

func TestWebsocketUintParameter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value any
		exp   uint64
		err   error
	}{
		{},
		{value: uint64(5), exp: 5},
		{value: 5, exp: 5},
		{value: 5.0, exp: 5},
		{value: -1, err: errWebsocketParameter},
		{value: -1.0, err: errWebsocketParameter},
		{value: 5.5, err: errWebsocketParameter},
		{value: float64(math.MaxUint32) + 1, err: errWebsocketParameter},
		{value: "5", err: errWebsocketParameter},
	} {
		params := map[string]any{}
		if tc.value != nil {
			params["nSigFigs"] = tc.value
		}
		value, err := websocketUintParameter(params, "nSigFigs")
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "websocketUintParameter should reject %v", tc.value)
			continue
		}
		require.NoErrorf(t, err, "websocketUintParameter must not error for %v", tc.value)
		assert.Equalf(t, tc.exp, value, "websocketUintParameter should return the value of %v", tc.value)
	}
}

func TestWebsocketBoolParameter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value any
		exp   bool
		err   error
	}{
		{},
		{value: true, exp: true},
		{value: false},
		{value: "true", err: errWebsocketParameter},
	} {
		params := map[string]any{}
		if tc.value != nil {
			params["aggregateByTime"] = tc.value
		}
		value, err := websocketBoolParameter(params, "aggregateByTime")
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "websocketBoolParameter should reject %v", tc.value)
			continue
		}
		require.NoErrorf(t, err, "websocketBoolParameter must not error for %v", tc.value)
		assert.Equalf(t, tc.exp, value, "websocketBoolParameter should return the value of %v", tc.value)
	}
}
