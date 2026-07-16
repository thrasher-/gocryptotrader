package kraken

import (
	"context"
	"testing"

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

func (m *mockAuthSubConnection) SendMessageReturnResponses(_ context.Context, _ request.EndpointLimit, _, _ any, expected int) ([][]byte, error) {
	m.expected = expected
	return m.responses, nil
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
		{name: "executions", channel: subscription.MyAccountChannel, qualifiedChannel: krakenWsV2Executions, response: []byte(`{"method":"subscribe","result":{"channel":"executions","snap_orders":true,"snap_trades":true},"success":true,"req_id":3}`), responseCount: 1},
		{name: "requires single response", channel: subscription.MyAccountChannel, qualifiedChannel: krakenWsV2Executions, responseCount: 0, errIs: errExpectedOneSubResponse, errContains: "got 0; Channel: myAccount"},
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

			err := ex.manageSubs(t.Context(), krakenWsSubscribe, subscription.List{{
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
