package hyperliquid

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket/buffer"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func setupWebsocketExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := new(Exchange)
	ex.SetDefaults()
	ex.Verbose = false
	ex.Features.Enabled.TradeFeed = true
	ex.Websocket.DataHandler = make(chan any, 8)
	ex.Websocket.TrafficAlert = make(chan struct{}, 1)
	ex.Websocket.ReadMessageErrors = make(chan error, 1)
	ex.Websocket.ShutdownC = make(chan struct{})

	cfg := &config.Exchange{
		Name: ex.Name,
		Orderbook: config.Orderbook{
			WebsocketBufferEnabled: false,
			WebsocketBufferLimit:   5,
		},
	}
	require.NoError(t, ex.Websocket.Orderbook.Setup(cfg, &buffer.Config{}, ex.Websocket.DataHandler))

	return ex
}

func TestBuildSubscriptionPayloadActiveAssetData(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)

	sub := &subscription.Subscription{Channel: hyperliquidActiveAssetDataChannel, Params: map[string]any{"user": "0xabc"}}
	payload, err := ex.buildSubscriptionPayload(sub, hyperliquidActiveAssetDataChannel, "BTC")
	require.NoError(t, err)
	assert.Equal(t, websocketChannelActiveAssetData, payload.Type)
	assert.Equal(t, "BTC", payload.Coin)
	assert.Equal(t, "0xabc", payload.User)

	_, err = ex.buildSubscriptionPayload(&subscription.Subscription{Channel: hyperliquidActiveAssetDataChannel}, hyperliquidActiveAssetDataChannel, "BTC")
	require.Error(t, err)
	_, err = ex.buildSubscriptionPayload(sub, hyperliquidActiveAssetDataChannel, "")
	require.Error(t, err)
}

func TestHandleActiveAssetData(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)

	perpPair := currency.NewBTCUSDC()
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, currency.Pairs{perpPair}, false))
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, currency.Pairs{perpPair}, true))

	payload := &wsActiveAssetDataPayload{
		User:             "0xabc",
		Coin:             "BTC",
		Leverage:         &wsLeverage{Type: "cross", Value: types.Number(5)},
		MaxTradeSizes:    []types.Number{types.Number(10), types.Number(1000)},
		AvailableToTrade: []types.Number{types.Number(4), types.Number(400)},
		MarkPrice:        types.Number(30000),
	}

	ex.now = func() time.Time { return time.UnixMilli(1700000000000) }
	require.NoError(t, ex.handleActiveAssetData(payload))

	select {
	case resp := <-ex.Websocket.DataHandler:
		update, ok := resp.(ActiveAssetDataUpdate)
		require.True(t, ok)
		assert.Equal(t, "0xabc", update.Address)
		assert.InDelta(t, 30000, update.MarkPrice, 1e-9)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for active asset data event")
	}

	cached, err := ex.GetLatestActiveAssetData(perpPair, asset.PerpetualContract)
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.InDelta(t, 30000, cached.MarkPrice, 1e-9)
}

func TestGenerateSubscriptionsActiveAssetData(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)
	perpPairs := currency.Pairs{currency.NewBTCUSDC()}
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, perpPairs, false))
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, perpPairs, true))
	spotPairs := currency.Pairs{currency.NewBTCUSDC()}
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.Spot, spotPairs, false))
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.Spot, spotPairs, true))

	ex.Features.Subscriptions = subscription.List{
		{Enabled: true, Asset: asset.PerpetualContract, Channel: hyperliquidActiveAssetDataChannel},
	}

	subs, err := ex.generateSubscriptions()
	require.NoError(t, err)

	found := false
	for _, sub := range subs {
		if sub.Channel == hyperliquidActiveAssetDataChannel {
			found = true
			break
		}
	}
	assert.False(t, found, "active asset data subscription should require credentials")
}

func TestGenerateSubscriptionsActiveAssetDataWithCredentials(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)
	ex.SetCredentials(testAddress, testPrivateKey, "", testVault, "", "")

	perpPairs := currency.Pairs{currency.NewBTCUSDC()}
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, perpPairs, false))
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, perpPairs, true))

	ex.Features.Subscriptions = subscription.List{
		{Enabled: true, Asset: asset.PerpetualContract, Channel: hyperliquidActiveAssetDataChannel},
	}

	addr, err := ex.accountAddressLower()
	require.NoError(t, err)
	assert.Equal(t, strings.ToLower(testAddress), addr)

	subs, err := ex.generateSubscriptions()
	require.NoError(t, err)
	require.NotEmpty(t, subs)

	found := false
	for _, sub := range subs {
		if sub.Channel != hyperliquidActiveAssetDataChannel {
			continue
		}
		found = true
		require.NotNil(t, sub.Params)
		assert.Equal(t, strings.ToLower(testAddress), sub.Params["user"])
		assert.Len(t, sub.Pairs, 1)
		break
	}
	assert.True(t, found, "expected active asset data subscription when credentials set")
}

func TestSubscriptionFromPayloadActiveAssetData(t *testing.T) {
	t.Parallel()

	payload := wsSubscriptionPayload{Type: websocketChannelActiveAssetData, Coin: "BTC", User: "0xAbC"}
	sub, err := subscriptionFromPayload(&payload)
	require.NoError(t, err)
	require.NotNil(t, sub)
	assert.Equal(t, hyperliquidActiveAssetDataChannel, sub.Channel)
	assert.True(t, sub.Authenticated, "active asset data subscription should be authenticated")
	require.Len(t, sub.Pairs, 1)
	assert.True(t, sub.Pairs[0].Equal(currency.NewBTCUSDC()))
	assert.Equal(t, asset.PerpetualContract, sub.Asset)
	require.NotNil(t, sub.Params)
	assert.Equal(t, strings.ToLower(payload.User), sub.Params["user"])
	expected, err := formatQualifiedChannel(hyperliquidActiveAssetDataChannel, asset.PerpetualContract, "BTC")
	require.NoError(t, err)
	assert.Equal(t, expected, sub.QualifiedChannel)
}

func TestGetLatestActiveAssetDataMissing(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)
	pair := currency.NewBTCUSDC()
	_, err := ex.GetLatestActiveAssetData(pair, asset.PerpetualContract)
	assert.ErrorIs(t, err, errActiveAssetDataNotFound)
}

func TestHandleActiveAssetContext(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)
	ex.now = func() time.Time { return time.UnixMilli(1700000000000).UTC() }

	premium := types.Number(0.001)
	payload := &wsActiveAssetContextPayload{
		Coin: "BTC",
		Ctx: PerpetualAssetContext{
			Funding:        types.Number(0.01),
			OpenInterest:   types.Number(1234),
			PrevDayPrice:   types.Number(26000),
			DayNotionalVol: types.Number(5000000),
			Premium:        &premium,
			OraclePrice:    types.Number(26500),
			MarkPrice:      types.Number(26450),
			MidPrice:       nil,
			ImpactPrices:   []types.Number{types.Number(26300), types.Number(26600)},
			DayBaseVolume:  types.Number(7500),
		},
	}

	require.NoError(t, ex.handleActiveAssetContext(payload))

	select {
	case raw := <-ex.Websocket.DataHandler:
		update, ok := raw.(ActiveAssetContextUpdate)
		require.True(t, ok)
		assert.Equal(t, asset.PerpetualContract, update.Asset)
		assert.True(t, update.Pair.Equal(currency.NewBTCUSDC()))
		assert.Equal(t, payload.Ctx.MarkPrice, update.Context.MarkPrice)
		assert.Equal(t, ex.now().UTC(), update.Timestamp)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for active asset context event")
	}
}

func TestHandleActiveSpotAssetContext(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)
	ex.now = func() time.Time { return time.UnixMilli(1700000000500).UTC() }

	payload := &wsActiveSpotAssetContextPayload{
		Coin: "SOL/USDC",
		Ctx: SpotAssetContext{
			DayNotionalVolume: types.Number(125000),
			MarkPrice:         types.Number(150),
			MidPrice:          nil,
			PrevDayPrice:      types.Number(140),
			CirculatingSupply: types.Number(1000000),
			Coin:              "SOL/USDC",
		},
	}

	require.NoError(t, ex.handleActiveSpotAssetContext(payload))

	select {
	case raw := <-ex.Websocket.DataHandler:
		update, ok := raw.(ActiveSpotAssetContextUpdate)
		require.True(t, ok)
		assert.True(t, update.Pair.Equal(currency.NewPair(currency.NewCode("SOL"), currency.USDC)))
		assert.Equal(t, payload.Ctx.MarkPrice, update.Context.MarkPrice)
		assert.Equal(t, ex.now().UTC(), update.Timestamp)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for active spot asset context event")
	}
}
