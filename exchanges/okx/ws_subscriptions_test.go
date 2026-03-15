package okx

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
)

func TestGetSpotMarginEvaluator(t *testing.T) {
	t.Parallel()
	eval := e.getSpotMarginEvaluator(nil)
	require.Empty(t, eval)

	spotBTCUSDTSub := &subscription.Subscription{Asset: asset.Spot, Pairs: []currency.Pair{currency.NewBTCUSDT()}, Channel: "trades"}
	futuresBTCUSDTSub := &subscription.Subscription{Asset: asset.USDTMarginedFutures, Pairs: []currency.Pair{currency.NewBTCUSDT()}, Channel: "trades"}
	marginBTCUSDTSub := &subscription.Subscription{Asset: asset.Margin, Pairs: []currency.Pair{currency.NewBTCUSDT()}, Channel: "trades"}

	subs := []*subscription.Subscription{spotBTCUSDTSub, futuresBTCUSDTSub, marginBTCUSDTSub}
	eval = e.getSpotMarginEvaluator(subs)
	require.True(t, eval.exists(currency.NewBTCUSDT(), "trades", asset.Spot))
	require.False(t, eval.exists(currency.NewBTCUSDT(), "trades", asset.USDTMarginedFutures))
	require.True(t, eval.exists(currency.NewBTCUSDT(), "trades", asset.Margin))

	needed, err := eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Spot)
	require.NoError(t, err)
	require.True(t, needed, "must be needed as no spot or margin subscription exists")
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.True(t, needed, "must be needed due to being a futures subscription")
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Margin)
	require.NoError(t, err)
	require.False(t, needed, "must not be needed as spot subscription will be used")

	subs = []*subscription.Subscription{spotBTCUSDTSub, futuresBTCUSDTSub}
	eval = e.getSpotMarginEvaluator(subs)
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Spot)
	require.NoError(t, err)
	require.True(t, needed, "must be needed as no margin subscription exists")
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.True(t, needed, "must be needed due to being a futures subscription")

	subs = []*subscription.Subscription{spotBTCUSDTSub, futuresBTCUSDTSub}
	err = e.Websocket.AddSuccessfulSubscriptions(nil, marginBTCUSDTSub)
	require.NoError(t, err)
	eval = e.getSpotMarginEvaluator(subs)
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Spot)
	require.NoError(t, err)
	require.False(t, needed, "must not be needed as margin subscription exists")
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.True(t, needed, "must be needed due to being a futures subscription")

	subs = []*subscription.Subscription{spotBTCUSDTSub, futuresBTCUSDTSub}
	eval = e.getSpotMarginEvaluator(subs)
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Spot)
	require.NoError(t, err)
	require.False(t, needed, "must not be needed as margin subscription exists and only the spot sub is being removed")
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.True(t, needed, "must be needed due to being a futures subscription")

	subs = []*subscription.Subscription{spotBTCUSDTSub, futuresBTCUSDTSub}
	err = e.Websocket.RemoveSubscriptions(nil, marginBTCUSDTSub)
	require.NoError(t, err)
	eval = e.getSpotMarginEvaluator(subs)
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Spot)
	require.NoError(t, err)
	require.True(t, needed, "must be needed as margin subscription does not exist and the subscription is no longer required")
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.True(t, needed, "must be needed due to being a futures subscription")

	subs = []*subscription.Subscription{marginBTCUSDTSub, futuresBTCUSDTSub}
	eval = e.getSpotMarginEvaluator(subs)
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Margin)
	require.NoError(t, err)
	require.True(t, needed, "must be needed as spot subscription does not exist and the subscription is no longer required")
	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.True(t, needed, "must be needed due to being a futures subscription")
}

func TestNeedsOutboundSubscription(t *testing.T) {
	t.Parallel()
	eval := make(spotMarginEvaluator)
	eval.add(currency.NewBTCUSDT(), "trades", asset.Spot, true)
	require.True(t, eval.exists(currency.NewBTCUSDT(), "trades", asset.Spot), "subscription must exist")
	needed, err := eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Spot)
	require.NoError(t, err)
	require.True(t, needed, "subscription must be needed")

	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.True(t, needed, "subscription must be needed")

	needed, err = eval.NeedsOutboundSubscription(currency.NewBTCUSDT(), "trades", asset.Margin)
	require.ErrorIs(t, err, subscription.ErrNotFound)
	require.False(t, needed, "subscription must not be needed")
}

func TestSubscribeSkipsEquivalentSpotMarginOutboundRequest(t *testing.T) {
	t.Parallel()

	ex := &Exchange{}
	ex.Websocket = websocket.NewManager()

	initialConn := &subscriptionRecorderConnection{}
	laterConn := &subscriptionRecorderConnection{}
	pair := currency.NewPairWithDelimiter("BTC", "USDT", "-")
	marginSub := &subscription.Subscription{
		Asset:            asset.Margin,
		Channel:          subscription.TickerChannel,
		Pairs:            currency.Pairs{pair},
		QualifiedChannel: `{"channel":"tickers","instID":"BTC-USDT"}`,
	}
	spotSub := &subscription.Subscription{
		Asset:            asset.Spot,
		Channel:          subscription.TickerChannel,
		Pairs:            currency.Pairs{pair},
		QualifiedChannel: marginSub.QualifiedChannel,
	}

	err := ex.Subscribe(t.Context(), initialConn, subscription.List{marginSub})
	require.NoError(t, err, "initial equivalent margin subscribe must not error")
	require.Len(t, initialConn.requests, 1, "initial equivalent margin subscribe must emit one outbound physical subscribe request")
	require.Len(t, initialConn.requests[0].Arguments, 1, "initial equivalent margin subscribe must contain one physical subscribe argument")
	require.Equal(t, channelTickers, initialConn.requests[0].Arguments[0].Channel, "initial equivalent margin subscribe must target the OKX ticker channel")

	err = ex.Subscribe(t.Context(), laterConn, subscription.List{spotSub})
	require.NoError(t, err, "later equivalent spot subscribe must not error")
	require.Empty(t, laterConn.requests, "later equivalent spot subscribe on a later connection pass must not emit another outbound physical subscribe request")
	require.Len(t, ex.Websocket.GetSubscriptions(), 2, "equivalent spot and margin subscriptions must both be tracked logically")
	require.NotNil(t, ex.Websocket.GetSubscription(marginSub), "earlier margin subscription must remain tracked")
	require.NotNil(t, ex.Websocket.GetSubscription(spotSub), "later spot subscription must be tracked without a second physical request")
}

type subscriptionRecorderConnection struct {
	websocket.Connection
	requests []WSSubscriptionInformationList
}

func (c *subscriptionRecorderConnection) SendJSONMessage(_ context.Context, _ request.EndpointLimit, payload any) error {
	req, ok := payload.(WSSubscriptionInformationList)
	if !ok {
		return fmt.Errorf("unexpected websocket payload type %T", payload)
	}
	c.requests = append(c.requests, req)
	return nil
}
