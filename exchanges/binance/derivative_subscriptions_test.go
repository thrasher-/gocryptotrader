package binance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func TestConfiguredDerivativeSubscriptions(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "saved configuration must load")
	for _, tc := range []struct {
		asset            asset.Item
		pair             currency.Pair
		filter, expected string
	}{
		{asset.USDTMarginedFutures, currency.NewBTCUSDT(), usdtmPublicFilter, "btcusdt@depth10@100ms"},
		{asset.CoinMarginedFutures, currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "", "btcusd_perp@depth10@100ms"},
		{asset.Options, currency.NewPairWithDelimiter("BTC", "260921-81000-C", "-"), optionsPublicFilter, "btc-260921-81000-c@depth10@100ms"},
	} {
		require.NoError(t, local.CurrencyPairs.SetAssetEnabled(tc.asset, true), "subscription asset must enable")
		require.NoError(t, local.CurrencyPairs.StorePairs(tc.asset, currency.Pairs{tc.pair}, false), "selected pair must be available")
		require.NoError(t, local.CurrencyPairs.StorePairs(tc.asset, currency.Pairs{tc.pair}, true), "selected pair must enable")
		for _, scope := range []asset.Item{tc.asset, asset.All} {
			for _, enabled := range []bool{false, true} {
				saved := &subscription.Subscription{Asset: scope, Channel: subscription.OrderbookChannel, Enabled: enabled, Levels: 10, Interval: kline.HundredMilliseconds}
				local.Features.Subscriptions = subscription.List{saved}
				var subs subscription.List
				var err error
				switch tc.asset {
				case asset.USDTMarginedFutures:
					subs, err = local.GenerateUFuturesDefaultSubscriptions(tc.filter)
				case asset.CoinMarginedFutures:
					subs, err = local.GenerateDefaultCFuturesSubscriptions()
				case asset.Options:
					subs, err = local.GenerateEOptionsDefaultSubscriptions(tc.filter)
				}
				require.NoError(t, err, "configured subscriptions must generate")
				if enabled {
					require.Len(t, subs, 1, "explicit product configuration must replace default channels")
					if tc.asset == asset.Options {
						params, err := optionsSubscriptionParams(subs)
						require.NoError(t, err, "Options subscription must format")
						assert.Equal(t, []string{tc.expected}, params, "Options should honour levels and interval")
					} else {
						assert.Equal(t, tc.expected, subs[0].QualifiedChannel, "futures should honour the configured contract, levels and interval")
					}
				} else {
					assert.Empty(t, subs, "explicitly disabled subscriptions should suppress product defaults")
				}
				assert.Equal(t, scope, saved.Asset, "routing should preserve saved asset scope")
				assert.Nil(t, saved.Params, "Options depth parameters should not mutate saved configuration")
			}
		}
	}
	local.Features.Subscriptions = subscription.List{{Asset: asset.CoinMarginedFutures, Enabled: true, Channel: subscription.TickerChannel}}
	subs, err := local.generateSubscriptions()
	require.NoError(t, err, "Spot generator must filter derivative configuration")
	assert.Empty(t, subs, "COIN-M channels should not be sent to the Spot connection")
}

func TestDerivativeSubscriptionValidation(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	for _, tc := range []struct {
		sub  *subscription.Subscription
		want error
	}{
		{&subscription.Subscription{Channel: "unsupported"}, subscription.ErrNotSupported},
		{&subscription.Subscription{Channel: "!unsupported"}, subscription.ErrNotSupported},
		{&subscription.Subscription{Channel: subscription.OrderbookChannel, Levels: 3}, subscription.ErrInvalidLevel},
		{&subscription.Subscription{Channel: subscription.OrderbookChannel, Interval: kline.OneMin}, subscription.ErrInvalidInterval},
		{&subscription.Subscription{Channel: subscription.CandlesChannel, Interval: kline.HundredMilliseconds}, kline.ErrInvalidInterval},
		{&subscription.Subscription{Channel: "continuousKline", Pairs: currency.Pairs{currency.NewPairWithDelimiter("BTCUSD", "261225", "_")}}, errContractTypeIsRequired},
		{&subscription.Subscription{Channel: "ticker", Pairs: currency.Pairs{currency.EMPTYPAIR}}, currency.ErrCurrencyPairEmpty},
	} {
		tc.sub.Enabled = true
		tc.sub.Asset = asset.CoinMarginedFutures
		if tc.sub.Pairs == nil {
			tc.sub.Pairs = currency.Pairs{currency.NewPairWithDelimiter("BTCUSD", "PERP", "_")}
		}
		local.Features.Subscriptions = subscription.List{tc.sub}
		_, _, err := local.configuredDerivativeSubscriptions(asset.CoinMarginedFutures, "")
		assert.ErrorIs(t, err, tc.want, "invalid subscription should retain its validation sentinel")
	}
	_, err := local.GenerateUFuturesDefaultSubscriptions("invalid")
	assert.ErrorIs(t, err, errUnsupportedSubscription, "invalid connection filter should fail offline")
}
