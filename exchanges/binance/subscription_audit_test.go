package binance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func TestSpotSymbolSelection(t *testing.T) {
	local := new(Exchange)
	pair := currency.NewBTCUSDT()
	pairs := currency.Pairs{pair}
	for _, tc := range []struct {
		name    string
		symbol  currency.Pair
		symbols currency.Pairs
		status  string
	}{
		{"single and multiple", pair, pairs, ""},
		{"single and status", pair, nil, "TRADING"},
		{"multiple and status", currency.EMPTYPAIR, pairs, "TRADING"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := local.GetExecutionRules(t.Context(), tc.symbol, tc.symbols, tc.status)
			assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting REST filters should fail offline")
			_, err = local.GetWsExecutionRules(&WsMarketSymbolsRequest{Symbol: tc.symbol, Symbols: tc.symbols, SymbolStatus: tc.status})
			assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting WebSocket filters should fail offline")
			arg := &GetExchangeInfoRequest{Symbol: tc.symbol, Symbols: tc.symbols, SymbolStatus: tc.status}
			_, err = local.GetExchangeInfo(t.Context(), arg)
			assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting REST exchange-info filters should fail offline")
			_, err = local.GetWsExchangeInfo(arg)
			assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting WebSocket exchange-info filters should fail offline")
		})
	}
	for _, symbol := range []currency.Pair{currency.EMPTYPAIR, pair} {
		arg := &GetExchangeInfoRequest{Symbol: symbol, Permissions: []string{"SPOT"}}
		if symbol.IsEmpty() {
			arg.Symbols = pairs
		}
		_, err := local.GetExchangeInfo(t.Context(), arg)
		assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "permission filters should conflict with explicit symbols")
		_, err = local.GetWsExchangeInfo(arg)
		assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "WebSocket permission filters should conflict with explicit symbols")
	}
}

func TestDerivativeSubscriptionsRespectEnabledPairs(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "saved exchange configuration must load")
	for _, tc := range []struct {
		asset    asset.Item
		pair     currency.Pair
		generate func() (subscription.List, error)
		prefix   string
	}{
		{asset.CoinMarginedFutures, currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), local.GenerateDefaultCFuturesSubscriptions, "btcusd_perp@depth"},
		{asset.USDTMarginedFutures, currency.NewBTCUSDT(), func() (subscription.List, error) {
			return local.GenerateUFuturesDefaultSubscriptions(usdtmPublicFilter)
		}, "btcusdt@depth"},
		{asset.Options, currency.NewPairWithDelimiter("BTC", "260921-81000-C", "-"), func() (subscription.List, error) {
			return local.GenerateEOptionsDefaultSubscriptions(optionsPublicFilter)
		}, ""},
	} {
		t.Run(tc.asset.String(), func(t *testing.T) {
			require.NoError(t, local.CurrencyPairs.StorePairs(tc.asset, currency.Pairs{tc.pair}, false), "selected contract must be available")
			require.NoError(t, local.CurrencyPairs.StorePairs(tc.asset, currency.Pairs{tc.pair}, true), "selected contract must be enabled")
			require.NoError(t, local.CurrencyPairs.SetAssetEnabled(tc.asset, true), "asset must be enabled")
			subs, err := tc.generate()
			require.NoError(t, err, "subscriptions must generate without an HTTP request")
			require.NotEmpty(t, subs, "enabled contracts must generate subscriptions")
			if tc.prefix != "" {
				found := false
				for _, s := range subs {
					if strings.HasPrefix(s.QualifiedChannel, tc.prefix) {
						found = true
					}
				}
				assert.True(t, found, "subscriptions should use the saved contract format")
			}
			if tc.asset == asset.Options {
				for _, s := range subs {
					assert.Equal(t, currency.Pairs{tc.pair}, s.Pairs, "Options should use the enabled contract")
				}
			}
			require.NoError(t, local.CurrencyPairs.SetAssetEnabled(tc.asset, false), "asset must disable")
			subs, err = tc.generate()
			require.NoError(t, err, "disabled assets must skip cleanly")
			assert.Empty(t, subs, "disabled assets should generate no connections")
		})
	}
	require.NoError(t, local.CurrencyPairs.SetAssetEnabled(asset.USDTMarginedFutures, true), "USD-M must enable for routing checks")
	public, err := local.GenerateUFuturesDefaultSubscriptions(usdtmPublicFilter)
	require.NoError(t, err, "public subscriptions must generate")
	market, err := local.GenerateUFuturesDefaultSubscriptions(usdtmMarketFilter)
	require.NoError(t, err, "market subscriptions must generate")
	for _, s := range public {
		assert.NotContains(t, s.QualifiedChannel, "aggTrade", "aggregate trades should use the market server")
	}
	found := false
	for _, s := range market {
		if strings.Contains(s.QualifiedChannel, "aggTrade") {
			found = true
		}
	}
	assert.True(t, found, "aggregate trades should be subscribed on the market server")
}
