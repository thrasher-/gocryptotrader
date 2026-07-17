package kraken

import (
	"context"
	"testing"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

type mockAuthSubConnection struct {
	websocket.Connection
	responses [][]byte
	expected  int
}

type mockPingConnection struct {
	websocket.Connection
	endpoint request.EndpointLimit
	handler  websocket.PingHandler
}

func (m *mockPingConnection) SetupPingHandler(endpoint request.EndpointLimit, handler websocket.PingHandler) {
	m.endpoint = endpoint
	m.handler = handler
}

func (m *mockAuthSubConnection) SendMessageReturnResponses(_ context.Context, _ request.EndpointLimit, _, _ any, expected int) ([][]byte, error) {
	m.expected = expected
	return m.responses, nil
}

func TestStartWsPingHandler(t *testing.T) {
	t.Parallel()

	conn := new(mockPingConnection)
	new(Exchange).startWsPingHandler(conn)
	assert.Equal(t, request.Unset, conn.endpoint, "ping handler endpoint should be unset")
	assert.Equal(t, websocket.PingHandler{
		MessageType: gws.TextMessage,
		Message:     []byte(`{"method":"ping"}`),
		Delay:       wsPingDelay,
	}, conn.handler, "ping handler should match Kraken's websocket requirements")
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
			require.NoError(t, testexch.Setup(ex), "Setup Instance must not error")

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
				require.ErrorIs(t, err, tc.errIs)
				require.ErrorContains(t, err, tc.errContains)
			} else {
				require.NoError(t, err, "auth subscription without pairs must not error")
			}
			assert.Equal(t, 1, conn.expected, "auth subscription without pairs waits for one response")
		})
	}
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
		assert.NoError(t, err, "exchange-formatted response should match the internal subscription pair")
	})
	t.Run("missing response", func(t *testing.T) {
		t.Parallel()
		err := new(Exchange).handleSubResps(sub, nil, wsSubscribe)
		require.ErrorIs(t, err, errSubPairMissing, "missing response must return errSubPairMissing")
		assert.ErrorContains(t, err, "XBT/USD", "missing response should retain exchange pair formatting")
	})
}

func TestWsProcessSubStatusInvalidPair(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup Instance must not error")
	s := &subscription.Subscription{
		Channel: subscription.TickerChannel,
		Pairs:   currency.Pairs{currency.NewBTCUSD()},
	}
	require.NoError(t, ex.Websocket.AddSubscriptions(nil, s), "subscription must be added in subscribing state")

	ex.wsProcessSubStatus([]byte(`{"method":"subscribe","result":{"channel":"ticker","symbol":"not-a-pair"},"success":true,"req_id":3}`))
	assert.Equal(t, subscription.SubscribingState, s.State(), "invalid websocket subscription pair should leave the subscription state unchanged")
}
