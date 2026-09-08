package hyperliquid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

func TestLiveWebsocket(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	t.Cleanup(func() {
		assert.NoError(t, ex.Shutdown(), "Shutdown should not error")
		assert.False(t, ex.Websocket.IsConnected(), "Websocket.IsConnected should be false after Shutdown")
	})
	require.NoError(t, ex.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must not error")
	ex.Features.Subscriptions = subscription.List{}
	ex.Websocket.SetSubscriptionsNotRequired()
	if ex.Websocket.IsEnabled() {
		require.NoError(t, ex.Websocket.Connect(t.Context()), "Websocket.Connect must not error")
	} else {
		require.NoError(t, ex.Websocket.Enable(t.Context()), "Websocket.Enable must not error")
	}

	subs := subscription.List{
		{Channel: subscription.TickerChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewPair(currency.NewCode("HYPE"), currency.USDC)}},
		{Channel: subscription.TickerChannel, Asset: asset.PerpetualContract, Pairs: currency.Pairs{currency.NewPair(currency.BTC, currency.USDC)}},
	}
	require.NoError(t, ex.Subscribe(subs), "Subscribe must receive acknowledgements")
	require.Len(t, ex.Websocket.GetSubscriptions(), len(subs), "Websocket.GetSubscriptions must contain both subscriptions")
	for _, sub := range ex.Websocket.GetSubscriptions() {
		assert.Equal(t, subscription.SubscribedState, sub.State(), "sub.State() should be SubscribedState after acknowledgement")
	}

	received := make(map[asset.Item]bool)
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for len(received) < len(subs) {
		select {
		case payload := <-ex.Websocket.DataHandler.C:
			switch data := payload.Data.(type) {
			case error:
				require.NoError(t, data, "data must not contain a websocket error")
			case *ticker.Price:
				for _, sub := range subs {
					if data.AssetType == sub.Asset && data.Pair.Equal(sub.Pairs[0]) {
						assert.Positive(t, data.Last, "data.Last should be positive for an active market")
						received[data.AssetType] = true
					}
				}
			}
		case <-timer.C:
			t.Fatal("Websocket.DataHandler must deliver spot and perpetual tickers before the deadline")
		}
	}
	require.NoError(t, ex.Unsubscribe(subs), "Unsubscribe must receive acknowledgements")
	assert.Empty(t, ex.Websocket.GetSubscriptions(), "Websocket.GetSubscriptions should be empty after unsubscribe")
}
