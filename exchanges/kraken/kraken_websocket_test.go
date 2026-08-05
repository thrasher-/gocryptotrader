package kraken

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

type mockAuthSubConnection struct {
	websocket.Connection
	responses  [][]byte
	response   []byte
	err        error
	beforeSend func()
	expected   int
	inspector  websocket.Inspector
	dialErr    error
}

type mockPingConnection struct {
	websocket.Connection
	endpoint request.EndpointLimit
	handler  websocket.PingHandler
}

func (*mockAuthSubConnection) Shutdown() error {
	return nil
}

func (m *mockAuthSubConnection) Dial(context.Context, *gws.Dialer, http.Header, url.Values) error {
	return m.dialErr
}

func (*mockAuthSubConnection) ReadMessage() websocket.Response {
	return websocket.Response{}
}

func (*mockAuthSubConnection) SetupPingHandler(request.EndpointLimit, websocket.PingHandler) {}

func (m *mockPingConnection) SetupPingHandler(endpoint request.EndpointLimit, handler websocket.PingHandler) {
	m.endpoint = endpoint
	m.handler = handler
}

func (m *mockAuthSubConnection) SendMessageReturnResponses(_ context.Context, _ request.EndpointLimit, _, _ any, expected int) ([][]byte, error) {
	m.expected = expected
	if m.beforeSend != nil {
		m.beforeSend()
	}
	return m.responses, m.err
}

func (m *mockAuthSubConnection) SendMessageReturnResponsesWithInspector(_ context.Context, _ request.EndpointLimit, _, _ any, expected int, inspector websocket.Inspector) ([][]byte, error) {
	m.expected = expected
	m.inspector = inspector
	if m.beforeSend != nil {
		m.beforeSend()
	}
	return m.responses, m.err
}

func (m *mockAuthSubConnection) SendMessageReturnResponse(_ context.Context, _ request.EndpointLimit, _, _ any) ([]byte, error) {
	if m.beforeSend != nil {
		m.beforeSend()
	}
	return m.response, m.err
}

func TestStartWsPingHandler(t *testing.T) {
	t.Parallel()

	conn := new(mockPingConnection)
	new(Exchange).startWsPingHandler(conn)
	assert.Equal(t, request.Unset, conn.endpoint, "conn.endpoint should be unset")
	assert.Equal(t, websocket.PingHandler{
		MessageType: gws.TextMessage,
		Message:     []byte(`{"method":"ping"}`),
		Delay:       wsPingDelay,
	}, conn.handler, "conn.handler should match Kraken's websocket requirements")
}

func TestCancelOrderResponseInspector(t *testing.T) {
	t.Parallel()

	inspector := cancelOrderResponseInspector{}
	for _, tc := range []struct {
		name     string
		response string
		final    bool
	}{
		{name: "Acknowledgement", response: `{"success":true,"result":{"order_id":"ORDER-1"}}`},
		{name: "Per-order rejection", response: `{"success":false,"error":"not found","result":{"order_id":"ORDER-1"}}`},
		{name: "Whole-request rejection", response: `{"success":false,"error":"invalid request"}`, final: true},
		{name: "Malformed", response: `{`, final: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.final, inspector.IsFinal([]byte(tc.response)), "IsFinal result should match response scope")
		})
	}
}

func TestWsConnect(t *testing.T) {
	t.Run("Disabled", func(t *testing.T) {
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		ex.SetEnabled(false)
		require.ErrorIs(t, ex.WsConnect(), websocket.ErrWebsocketNotEnabled, "WsConnect must reject a disabled exchange")
	})

	t.Run("PublicDialError", func(t *testing.T) {
		dialErr := errors.New("dial failed")
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		ex.Websocket.Conn = &mockAuthSubConnection{dialErr: dialErr}
		require.ErrorIs(t, ex.WsConnect(), dialErr, "WsConnect must return the public connection error")
	})

	t.Run("AuthenticationError", func(t *testing.T) {
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		ex.Websocket.Conn = new(mockAuthSubConnection)
		ex.API.AuthenticatedWebsocketSupport = true
		require.NoError(t, ex.WsConnect(), "WsConnect must retain the public connection when authentication fails")
		ex.Websocket.Wg.Wait()
		assert.False(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "WsConnect should disable authenticated endpoints after a token error")
	})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/0/private/GetWebSocketsToken", r.URL.Path, "WsConnect should request a current WebSocket token")
		_, err := w.Write([]byte(`{"error":[],"result":{"token":"test-token"}}`))
		assert.NoError(t, err, "WsConnect token response writing should not error")
	}))
	defer tokenServer.Close()

	t.Run("AuthenticatedDialError", func(t *testing.T) {
		dialErr := errors.New("authenticated dial failed")
		ex := newAuthenticatedSpotExchange(t, tokenServer.URL)
		ex.API.AuthenticatedWebsocketSupport = true
		ex.Websocket.Conn = new(mockAuthSubConnection)
		ex.Websocket.AuthConn = &mockAuthSubConnection{dialErr: dialErr}
		require.NoError(t, ex.WsConnect(), "WsConnect must retain the public connection when authenticated dialing fails")
		ex.Websocket.Wg.Wait()
		assert.False(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "WsConnect should disable authenticated endpoints after an authenticated dial error")
	})

	t.Run("Authenticated", func(t *testing.T) {
		ex := newAuthenticatedSpotExchange(t, tokenServer.URL)
		ex.API.AuthenticatedWebsocketSupport = true
		ex.Websocket.Conn = new(mockAuthSubConnection)
		ex.Websocket.AuthConn = new(mockAuthSubConnection)
		ex.executionSequence = 99
		ex.executionResubPending = true
		require.NoError(t, ex.WsConnect(), "WsConnect must connect public and authenticated WebSockets")
		ex.Websocket.Wg.Wait()
		assert.True(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "WsConnect should enable authenticated endpoints")
		assert.Equal(t, "test-token", ex.websocketAuthToken(), "WsConnect should retain the authenticated token")
		assert.Zero(t, ex.executionSequence, "WsConnect should reset execution sequence tracking")
		assert.False(t, ex.executionResubPending, "WsConnect should reset execution resubscription tracking")
	})
}

func TestGetSubscriptionTemplate(t *testing.T) {
	t.Parallel()

	tmpl, err := new(Exchange).GetSubscriptionTemplate(nil)
	require.NoError(t, err, "GetSubscriptionTemplate must parse the subscription template")
	assert.NotNil(t, tmpl, "GetSubscriptionTemplate should return a template")
}

func TestValidateSubscriptions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		subs      subscription.List
		expectErr bool
	}{
		{name: "empty"},
		{name: "nil subscription", subs: subscription.List{nil}},
		{
			name: "non-book and non-Spot ignored",
			subs: subscription.List{
				{Channel: subscription.TickerChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewBTCUSD()}},
				{Channel: subscription.OrderbookChannel, Asset: asset.Futures, Pairs: currency.Pairs{currency.NewBTCUSD()}, Levels: 10},
			},
		},
		{
			name: "distinct pairs and depths",
			subs: subscription.List{
				{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewBTCUSD()}, Levels: 10},
				{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewPair(currency.ETH, currency.USD)}, Levels: 1000},
			},
		},
		{
			name: "default and explicit depth equivalent",
			subs: subscription.List{
				{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewPair(currency.XBT, currency.USD)}},
				{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewBTCUSD()}, Levels: 10},
			},
		},
		{
			name: "conflicting depths",
			subs: subscription.List{
				{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewPair(currency.XBT, currency.USD)}, Levels: 10},
				{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewBTCUSD()}, Levels: 1000},
			},
			expectErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := new(Exchange).ValidateSubscriptions(tc.subs)
			if tc.expectErr {
				require.ErrorIs(t, err, subscription.ErrExclusiveSubscription, "ValidateSubscriptions must reject conflicting book depths")
				assert.ErrorContains(t, err, "BTC/USD", "ValidateSubscriptions should identify the conflicting pair")
				return
			}
			require.NoError(t, err, "ValidateSubscriptions must accept unambiguous subscriptions")
		})
	}
}

func TestApplyBookDepthDefaults(t *testing.T) {
	t.Parallel()
	assert.NotPanics(t, func() {
		applyBookDepthDefaults(nil)
	}, "applyBookDepthDefaults should accept an empty list")
	assert.NotPanics(t, func() {
		applyBookDepthDefaults(subscription.List{nil})
	}, "applyBookDepthDefaults should accept a nil subscription")

	for _, tc := range []struct {
		name     string
		sub      *subscription.Subscription
		expected int
	}{
		{name: "default Spot book depth", sub: &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.Spot}, expected: wsDefaultBookDepth},
		{name: "explicit Spot book depth", sub: &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Levels: 1000}, expected: 1000},
		{name: "Spot ticker", sub: &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.Spot}},
		{name: "Futures book", sub: &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.Futures}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			originalDepth := tc.sub.Levels
			result := applyBookDepthDefaults(subscription.List{tc.sub})
			require.Len(t, result, 1, "applyBookDepthDefaults result must contain one subscription")
			assert.Equal(t, tc.expected, result[0].Levels, "applyBookDepthDefaults should set only omitted Spot book depths")
			assert.Equal(t, originalDepth, tc.sub.Levels, "applyBookDepthDefaults should not mutate the caller's subscription")
		})
	}
}

func TestSubscriptionConnection(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	publicConn := new(mockAuthSubConnection)
	authConn := new(mockAuthSubConnection)
	ex.Websocket.Conn = publicConn
	ex.Websocket.AuthConn = authConn

	assert.Same(t, publicConn, ex.subscriptionConnection(nil), "subscriptionConnection should use the public connection for a nil subscription")
	assert.Same(t, publicConn, ex.subscriptionConnection(new(subscription.Subscription)), "subscriptionConnection should use the public connection for a public subscription")
	assert.Same(t, authConn, ex.subscriptionConnection(&subscription.Subscription{Authenticated: true}), "subscriptionConnection should use the authenticated connection for an authenticated subscription")
}

func TestAuthenticatedSubscriptionLifecycle(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	publicConn := &mockAuthSubConnection{err: errors.New("public connection used")}
	authConn := &mockAuthSubConnection{
		responses: [][]byte{[]byte(`{"method":"subscribe","result":{"channel":"executions","snap_orders":true,"snap_trades":true},"success":true}`)},
	}
	ex.Websocket.Conn = publicConn
	ex.Websocket.AuthConn = authConn
	sub := &subscription.Subscription{
		Channel:          subscription.MyAccountChannel,
		QualifiedChannel: wsExecutions,
		Authenticated:    true,
	}
	authConn.beforeSend = func() {
		ex.wsProcessSubStatus(authConn.responses[0])
	}
	require.NoError(t, ex.Subscribe(subscription.List{sub}), "Subscribe must use the authenticated connection")
	assert.Same(t, sub, ex.Websocket.GetSubscription(sub), "GetSubscription should retain the authenticated subscription after acknowledgement")

	authConn.responses[0] = []byte(`{"method":"unsubscribe","result":{"channel":"executions"},"success":true}`)
	require.NoError(t, ex.Unsubscribe(subscription.List{sub}), "Unsubscribe must use the authenticated connection")
	assert.Nil(t, ex.Websocket.GetSubscription(sub), "GetSubscription should not return the unsubscribed authenticated subscription")
	assert.Zero(t, publicConn.expected, "publicConn.expected should remain zero for the authenticated lifecycle")
}

func TestManageSubs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name             string
		channel          string
		qualifiedChannel string
		response         []byte
		responseCount    int
		errIs            error
		errContains      string
	}{
		{name: "executions", channel: subscription.MyAccountChannel, qualifiedChannel: wsExecutions, response: []byte(`{"method":"subscribe","result":{"channel":"executions","snap_orders":true,"snap_trades":true},"success":true,"req_id":3}`), responseCount: 1},
		{name: "requires single response", channel: subscription.MyAccountChannel, qualifiedChannel: wsExecutions, responseCount: 0, errIs: errExpectedOneSubResponse, errContains: "got 0; Channel: myAccount"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ex := new(Exchange)
			require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")

			conn := &mockAuthSubConnection{expected: -1}
			if tc.responseCount > 0 {
				conn.responses = [][]byte{tc.response}
			}
			ex.Websocket.AuthConn = conn

			err := ex.manageSubs(t.Context(), wsSubscribe, subscription.List{{
				Channel:          tc.channel,
				QualifiedChannel: tc.qualifiedChannel,
				Authenticated:    true,
			}})
			if tc.errIs != nil {
				require.ErrorIs(t, err, tc.errIs, "manageSubs must return the expected sentinel error")
				require.ErrorContains(t, err, tc.errContains, "manageSubs must return the expected error")
			} else {
				require.NoError(t, err, "manageSubs must accept an authenticated subscription without pairs")
			}
			assert.Equal(t, 1, conn.expected, "manageSubs should wait for one response when an authenticated subscription has no pairs")
		})
	}

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	require.ErrorIs(t, ex.manageSubs(t.Context(), wsSubscribe, nil), subscription.ErrBatchingNotSupported, "manageSubs must reject a batch without exactly one subscription")

	sendErr := errors.New("send failed")
	ex.Websocket.Conn = &mockAuthSubConnection{err: sendErr}
	err := ex.manageSubs(t.Context(), wsSubscribe, subscription.List{{
		Channel:          subscription.TickerChannel,
		QualifiedChannel: wsTicker,
		Pairs:            currency.Pairs{currency.NewBTCUSD()},
	}})
	require.ErrorIs(t, err, sendErr, "manageSubs must return a non-timeout connection error")
}

func TestHandleSubResps(t *testing.T) {
	t.Parallel()

	sub := &subscription.Subscription{
		Channel: subscription.TickerChannel,
		Pairs:   currency.Pairs{currency.NewPair(currency.XBT, currency.USD)},
	}
	t.Run("exchange formatted response", func(t *testing.T) {
		t.Parallel()
		err := new(Exchange).handleSubResps(sub, [][]byte{
			[]byte(`{"method":"subscribe","result":{"channel":"ticker","symbol":"BTC/USD"},"success":true}`),
		}, wsSubscribe)
		assert.NoError(t, err, "handleSubResps should match an exchange-formatted response to the internal pair")
	})
	t.Run("missing response", func(t *testing.T) {
		t.Parallel()
		err := new(Exchange).handleSubResps(sub, nil, wsSubscribe)
		require.ErrorIs(t, err, errSubPairMissing, "handleSubResps must return errSubPairMissing for a missing response")
		assert.ErrorContains(t, err, "XBT/USD", "handleSubResps should retain exchange pair formatting for a missing response")
	})
	t.Run("channel response error", func(t *testing.T) {
		t.Parallel()
		err := new(Exchange).handleSubResps(&subscription.Subscription{Channel: subscription.MyAccountChannel}, [][]byte{
			[]byte(`{"method":"subscribe","error":"permission denied","success":false}`),
		}, wsSubscribe)
		require.ErrorContains(t, err, "permission denied", "handleSubResps must return a channel response error")
	})
	t.Run("malformed response", func(t *testing.T) {
		t.Parallel()
		err := new(Exchange).handleSubResps(sub, [][]byte{[]byte(`{`)}, wsSubscribe)
		require.ErrorContains(t, err, "parsing WS response", "handleSubResps must reject a malformed response")
		require.ErrorIs(t, err, common.ErrInvalidResponse, "handleSubResps must return common.ErrInvalidResponse for malformed data")
	})
	t.Run("missing response pair", func(t *testing.T) {
		t.Parallel()
		err := new(Exchange).handleSubResps(sub, [][]byte{
			[]byte(`{"method":"subscribe","success":true}`),
		}, wsSubscribe)
		require.ErrorContains(t, err, "parsing WS symbol", "handleSubResps must reject a response without a symbol")
		require.ErrorIs(t, err, common.ErrInvalidResponse, "handleSubResps must return common.ErrInvalidResponse without a symbol")
		require.ErrorIs(t, err, errSubPairMissing, "handleSubResps must retain errSubPairMissing without a symbol")
	})
	t.Run("invalid response pair", func(t *testing.T) {
		t.Parallel()
		err := new(Exchange).handleSubResps(sub, [][]byte{
			[]byte(`{"method":"subscribe","result":{"symbol":"invalid"},"success":true}`),
		}, wsSubscribe)
		require.ErrorContains(t, err, "parsing WS pair", "handleSubResps must reject an invalid response pair")
	})
	t.Run("verbose response", func(t *testing.T) {
		t.Parallel()
		ex := &Exchange{}
		ex.Verbose = true
		err := ex.handleSubResps(sub, [][]byte{
			[]byte(`{"method":"subscribe","result":{"symbol":"BTC/USD"},"success":true}`),
		}, wsSubscribe)
		require.NoError(t, err, "handleSubResps must accept a verbose successful response")
	})
}

func TestParseWebsocketResponse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		response    string
		errIs       error
		errContains string
		invalid     bool
	}{
		{name: "malformed", response: `{`, errContains: "error parsing WS response", invalid: true},
		{name: "missing success", response: `{}`, errContains: "error parsing WS success", invalid: true},
		{name: "success", response: `{"success":true}`},
		{name: "unknown error", response: `{"success":false}`, errIs: common.ErrUnknownError, invalid: true},
		{name: "server error", response: `{"success":false,"error":"permission denied"}`, errContains: "permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseWebsocketResponse([]byte(tc.response))
			if tc.errIs != nil {
				require.ErrorIs(t, err, tc.errIs, "parseWebsocketResponse must return the expected sentinel error")
			}
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains, "parseWebsocketResponse must return the expected error")
			}
			if tc.invalid {
				require.ErrorIs(t, err, common.ErrInvalidResponse, "parseWebsocketResponse must identify an invalid response")
			}
			if tc.errIs == nil && tc.errContains == "" {
				require.NoError(t, err, "parseWebsocketResponse must accept a successful response")
			}
		})
	}
}

func TestGetSubRespErr(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.ErrorContains(t, ex.getSubRespErr([]byte(`{`), wsSubscribe), "error parsing WS response", "getSubRespErr must reject malformed responses")
	require.ErrorContains(t, ex.getSubRespErr([]byte(`{"method":"unsubscribe","success":true}`), wsSubscribe), "wrong WS method", "getSubRespErr must reject the wrong method")
	require.NoError(t, ex.getSubRespErr([]byte(`{"method":"subscribe","success":true}`), wsSubscribe), "getSubRespErr must accept the requested method")
}

func TestSubscribeCleanupError(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	ex.Websocket.Conn = &mockAuthSubConnection{
		beforeSend: func() {
			subs := ex.Websocket.GetSubscriptions()
			if len(subs) == 1 {
				subs[0].SetKey("changed-after-add")
			}
		},
	}

	err := ex.Subscribe(subscription.List{{
		Asset:   asset.Spot,
		Channel: subscription.TickerChannel,
		Pairs:   currency.Pairs{currency.NewPair(currency.XBT, currency.USD)},
	}})
	require.ErrorIs(t, err, subscription.ErrNotFound, "Subscribe must report cleanup failure when a subscription key changes concurrently")
}

func TestUnsubscribeStateError(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	s := &subscription.Subscription{
		Asset:   asset.Spot,
		Channel: subscription.TickerChannel,
		Pairs:   currency.Pairs{currency.NewPair(currency.XBT, currency.USD)},
	}
	require.NoError(t, ex.Websocket.AddSuccessfulSubscriptions(ex.Websocket.Conn, s), "AddSuccessfulSubscriptions must add the subscription")
	require.NoError(t, s.SetState(subscription.UnsubscribingState), "s.SetState must enter unsubscribing state")
	require.ErrorIs(t, ex.Unsubscribe(subscription.List{s}), subscription.ErrInStateAlready, "Unsubscribe must return a state transition error")
}

func TestUnsubscribeDoesNotMutateKeys(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	conn := &mockAuthSubConnection{responses: [][]byte{
		[]byte(`{"method":"unsubscribe","result":{"channel":"book","symbol":"BTC/USD","depth":10},"success":true}`),
	}}
	ex.Websocket.Conn = conn
	stored := &subscription.Subscription{
		Asset:   asset.Spot,
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
		Levels:  wsDefaultBookDepth,
	}
	require.NoError(t, ex.Websocket.AddSuccessfulSubscriptions(conn, stored), "AddSuccessfulSubscriptions must add the orderbook subscription")
	key := &subscription.Subscription{
		Asset:   asset.Spot,
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
	}
	require.NoError(t, ex.Unsubscribe(subscription.List{key}), "Unsubscribe must match a default-depth orderbook")
	assert.Zero(t, key.Levels, "Unsubscribe should not apply defaults to the caller's key")
}

func TestWsProcessSubStatusFailures(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	ex.wsProcessSubStatus([]byte(`{"method":"subscribe","result":{"channel":"ticker","symbol":"BTC/USD"},"error":"rejected","success":false}`))
	ex.wsProcessSubStatus([]byte(`{"method":"subscribe","result":{"channel":"unknown","symbol":"BTC/USD"},"success":true}`))
	ex.wsProcessSubStatus([]byte(`{"method":"subscribe","result":{"channel":"ticker","symbol":"BTC/USD"},"success":true}`))
}

func TestWsOrderRequestErrors(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	_, err := ex.wsAddOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "wsAddOrder must reject nil parameters")
	_, err = ex.wsAddOrder(t.Context(), &WebsocketAddOrderParams{Symbol: "invalid"})
	require.ErrorContains(t, err, "invalid add_order symbol", "wsAddOrder must reject an invalid symbol")

	sendErr := errors.New("send failed")
	ex.Websocket.AuthConn = &mockAuthSubConnection{err: sendErr}
	ex.setWebsocketAuthToken("test-token")
	params := &WebsocketAddOrderParams{Symbol: "XBT/USD"}
	_, err = ex.wsAddOrder(t.Context(), params)
	require.ErrorIs(t, err, sendErr, "wsAddOrder must return connection errors")
	assert.Equal(t, "XBT/USD", params.Symbol, "wsAddOrder should not rewrite caller parameters")
	assert.Empty(t, params.Token, "wsAddOrder should not expose the session token through caller parameters")

	ex.Websocket.AuthConn = &mockAuthSubConnection{response: []byte(`{`)}
	_, err = ex.wsAddOrder(t.Context(), &WebsocketAddOrderParams{Symbol: "BTC/USD"})
	require.ErrorContains(t, err, "error parsing WS response", "wsAddOrder must reject malformed responses")

	ex.Websocket.AuthConn = &mockAuthSubConnection{response: []byte(`{"success":false,"error":"rejected"}`)}
	_, err = ex.wsAddOrder(t.Context(), &WebsocketAddOrderParams{Symbol: "BTC/USD"})
	require.ErrorContains(t, err, "add order: rejected", "wsAddOrder must return a server rejection")

	ex.Websocket.AuthConn = &mockAuthSubConnection{response: []byte(`{"success":true,"result":{}}`)}
	_, err = ex.wsAddOrder(t.Context(), &WebsocketAddOrderParams{Symbol: "BTC/USD"})
	require.ErrorIs(t, err, common.ErrInvalidResponse, "wsAddOrder must require an order ID")

	fillDataHandler(t, ex)
	ex.Websocket.AuthConn = &mockAuthSubConnection{response: []byte(`{"success":true,"result":{"order_id":"ORDER-1"}}`)}
	_, err = ex.wsAddOrder(t.Context(), &WebsocketAddOrderParams{Symbol: "BTC/USD"})
	require.Error(t, err, "wsAddOrder must return order delivery errors")

	require.NoError(t, ex.wsCancelOrders(t.Context(), nil), "wsCancelOrders must accept an empty order list")
	ex.Websocket.AuthConn = &mockAuthSubConnection{err: sendErr}
	require.ErrorIs(t, ex.wsCancelOrders(t.Context(), []string{"ORDER-1"}), sendErr, "wsCancelOrders must return connection errors")

	malformedCancelConn := &mockAuthSubConnection{responses: [][]byte{[]byte(`{`)}}
	ex.Websocket.AuthConn = malformedCancelConn
	err = ex.wsCancelOrders(t.Context(), []string{"ORDER-1"})
	require.ErrorContains(t, err, "response", "wsCancelOrders must retain a malformed response")
	assert.NotContains(t, err.Error(), "ORDER-1", "wsCancelOrders should not fabricate an order ID for a malformed response")
	require.NotNil(t, malformedCancelConn.inspector, "wsCancelOrders must inspect batch responses for a whole-request rejection")
	assert.True(t, malformedCancelConn.inspector.IsFinal([]byte(`{"success":false,"error":"invalid request"}`)), "wsCancelOrders inspector should stop on a whole-request rejection")

	ex.Websocket.AuthConn = &mockAuthSubConnection{responses: [][]byte{
		[]byte(`{"success":true,"result":{"order_id":"ORDER-1"}}`),
		[]byte(`{"success":false}`),
	}}
	err = ex.wsCancelOrders(t.Context(), []string{"ORDER-1", "ORDER-2"})
	require.ErrorIs(t, err, common.ErrInvalidResponse, "wsCancelOrders must reject a malformed failure response")
	assert.NotContains(t, err.Error(), "ORDER-1", "wsCancelOrders should not attribute a malformed failure response")
	assert.NotContains(t, err.Error(), "ORDER-2", "wsCancelOrders should not attribute a malformed failure response")

	ex.Websocket.AuthConn = &mockAuthSubConnection{responses: [][]byte{[]byte(`{"success":true,"result":{}}`)}}
	err = ex.wsCancelOrders(t.Context(), []string{"ORDER-1"})
	require.ErrorIs(t, err, common.ErrInvalidResponse, "wsCancelOrders must reject a success response without an order ID")
	assert.NotContains(t, err.Error(), "ORDER-1", "wsCancelOrders should not correlate an invalid success response")

	ex.Websocket.AuthConn = &mockAuthSubConnection{responses: [][]byte{[]byte(`{"success":true,"result":{"order_id":"ORDER-2"}}`)}}
	err = ex.wsCancelOrders(t.Context(), []string{"ORDER-1"})
	require.ErrorIs(t, err, common.ErrInvalidResponse, "wsCancelOrders must reject an acknowledgement for an unrequested order")
	assert.NotContains(t, err.Error(), "ORDER-1", "wsCancelOrders should not correlate an unrequested acknowledgement")

	ex.Websocket.AuthConn = &mockAuthSubConnection{responses: [][]byte{[]byte(`{"success":false,"error":"not found"}`)}}
	err = ex.wsCancelOrders(t.Context(), []string{"ORDER-1"})
	require.ErrorContains(t, err, "not found", "wsCancelOrders must retain a response error without an order ID")
	assert.Contains(t, err.Error(), "ORDER-1", "wsCancelOrders should identify the only unacknowledged order")

	ex.Websocket.AuthConn = &mockAuthSubConnection{responses: [][]byte{
		[]byte(`{"success":false,"error":"not found"}`),
		[]byte(`{"success":true,"result":{"order_id":"ORDER-1"}}`),
	}}
	err = ex.wsCancelOrders(t.Context(), []string{"ORDER-1", "ORDER-2"})
	require.ErrorContains(t, err, "not found", "wsCancelOrders must retain an out-of-order response error")
	assert.NotContains(t, err.Error(), "ORDER-1", "wsCancelOrders should not attribute an out-of-order error to a successful order")
	assert.Contains(t, err.Error(), "ORDER-2", "wsCancelOrders should identify the order missing from successful responses")

	ex.Websocket.AuthConn = &mockAuthSubConnection{responses: [][]byte{
		[]byte(`{"success":false,"error":"already filled"}`),
		[]byte(`{"success":true,"result":{"order_id":"ORDER-2"}}`),
		[]byte(`{"success":false,"error":"not found"}`),
	}}
	err = ex.wsCancelOrders(t.Context(), []string{"ORDER-1", "ORDER-2", "ORDER-3"})
	require.ErrorContains(t, err, "already filled", "wsCancelOrders must retain the first batch error")
	require.ErrorContains(t, err, "not found", "wsCancelOrders must retain the second batch error")
	assert.Contains(t, err.Error(), "ORDER-1", "wsCancelOrders should identify every unacknowledged order")
	assert.NotContains(t, err.Error(), "ORDER-2", "wsCancelOrders should exclude acknowledged orders")
	assert.Contains(t, err.Error(), "ORDER-3", "wsCancelOrders should identify every unacknowledged order")

	ex.Websocket.AuthConn = &mockAuthSubConnection{responses: [][]byte{
		[]byte(`{"success":true,"result":{"order_id":"ORDER-1"}}`),
		[]byte(`{"success":false,"error":"unexpected extra response"}`),
	}}
	err = ex.wsCancelOrders(t.Context(), []string{"ORDER-1"})
	require.ErrorContains(t, err, "unexpected extra response", "wsCancelOrders must retain an excess failure response")
	assert.NotContains(t, err.Error(), "ORDER-1", "wsCancelOrders should not label an acknowledged order as failed")

	ex.Websocket.AuthConn = &mockAuthSubConnection{err: sendErr}
	_, err = ex.wsCancelAllOrders(t.Context())
	require.ErrorIs(t, err, sendErr, "wsCancelAllOrders must return connection errors")

	ex.Websocket.AuthConn = &mockAuthSubConnection{response: []byte(`{"success":false,"error":"rejected"}`)}
	_, err = ex.wsCancelAllOrders(t.Context())
	require.ErrorContains(t, err, "rejected", "wsCancelAllOrders must return a server rejection")
}

func TestWsProcessSubStatusInvalidPair(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	s := &subscription.Subscription{
		Channel: subscription.TickerChannel,
		Pairs:   currency.Pairs{currency.NewBTCUSD()},
	}
	require.NoError(t, ex.Websocket.AddSubscriptions(nil, s), "AddSubscriptions must add the subscription in subscribing state")

	ex.wsProcessSubStatus([]byte(`{"method":"subscribe","result":{"channel":"ticker","symbol":"not-a-pair"},"success":true,"req_id":3}`))
	assert.Equal(t, subscription.SubscribingState, s.State(), "s.State should remain unchanged after an invalid WebSocket pair")
}
