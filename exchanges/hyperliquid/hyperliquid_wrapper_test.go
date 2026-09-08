package hyperliquid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

func TestLiveUpdateTradablePairs(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.GetAvailablePairs(a)
		require.NoErrorf(t, err, "GetAvailablePairs for %s must not error", a)
		assert.NotEmptyf(t, pairs, "pairs for %s should not be empty", a)
	}
}

func TestLiveMarketDataWrappers(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	testexch.UpdatePairsOnce(t, e)
	for _, market := range []struct {
		asset asset.Item
		pair  currency.Pair
	}{
		{asset.Spot, currency.NewPair(currency.NewCode("HYPE"), currency.USDC)},
		{asset.PerpetualContract, currency.NewPair(currency.BTC, currency.USDC)},
	} {
		t.Run(market.asset.String(), func(t *testing.T) {
			t.Run("Ticker", func(t *testing.T) {
				tick, err := e.UpdateTicker(t.Context(), market.pair, market.asset)
				require.NoError(t, err, "UpdateTicker must not error")
				require.NotNil(t, tick, "tick must not be nil")
				assert.True(t, market.pair.Equal(tick.Pair), "tick.Pair should identify the requested pair")
				assert.Equal(t, market.asset, tick.AssetType, "tick.AssetType should identify the requested asset")
				assert.Positive(t, tick.Last, "tick.Last should be positive for an active market")
			})
			t.Run("Orderbook", func(t *testing.T) {
				book, err := e.UpdateOrderbook(t.Context(), market.pair, market.asset)
				require.NoError(t, err, "UpdateOrderbook must not error")
				require.NotNil(t, book, "book must not be nil")
				assert.True(t, market.pair.Equal(book.Pair), "book.Pair should identify the requested pair")
				assert.Equal(t, market.asset, book.Asset, "book.Asset should identify the requested asset")
				assert.NotEmpty(t, book.Bids, "book.Bids should contain bids")
				assert.NotEmpty(t, book.Asks, "book.Asks should contain asks")
			})
			t.Run("Trades", func(t *testing.T) {
				trades, err := e.GetRecentTrades(t.Context(), market.pair, market.asset)
				require.NoError(t, err, "GetRecentTrades must not error")
				assert.NotEmpty(t, trades, "trades should not be empty for an active market")
			})
			t.Run("Candles", func(t *testing.T) {
				end := time.Now().UTC().Truncate(time.Minute)
				candles, err := e.GetHistoricCandles(t.Context(), market.pair, market.asset, kline.OneMin, end.Add(-time.Hour), end)
				require.NoError(t, err, "GetHistoricCandles must not error")
				require.NotNil(t, candles, "candles must not be nil")
				assert.NotEmpty(t, candles.Candles, "candles.Candles should not be empty for an active market")
			})
		})
	}
}

func TestLiveUpdateAccountBalances(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		t.Run(a.String(), func(t *testing.T) {
			_, err := e.UpdateAccountBalances(t.Context(), a)
			assert.NoError(t, err, "UpdateAccountBalances should succeed for accounts with or without funds")
		})
	}
}
