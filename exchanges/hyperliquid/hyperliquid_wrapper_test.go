package hyperliquid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

func TestLiveUpdateTradablePairs(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	require.NoError(t, e.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must not error")
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.GetAvailablePairs(a)
		require.NoErrorf(t, err, "GetAvailablePairs for %s must not error", a)
		assert.NotEmptyf(t, pairs, "pairs for %s should not be empty", a)
	}
}

func TestLiveUpdateTicker(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	testexch.UpdatePairsOnce(t, e)
	for _, tc := range liveMarkets {
		t.Run(tc.asset.String(), func(t *testing.T) {
			tick, err := e.UpdateTicker(t.Context(), tc.pair, tc.asset)
			require.NoError(t, err, "UpdateTicker must not error")
			require.NotNil(t, tick, "tick must not be nil")
			assert.True(t, tc.pair.Equal(tick.Pair), "tick.Pair should identify the requested pair")
			assert.Equal(t, tc.asset, tick.AssetType, "tick.AssetType should identify the requested asset")
			assert.Positive(t, tick.Last, "tick.Last should be positive for an active market")
		})
	}
}

func TestLiveUpdateOrderbook(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	testexch.UpdatePairsOnce(t, e)
	for _, tc := range liveMarkets {
		t.Run(tc.asset.String(), func(t *testing.T) {
			book, err := e.UpdateOrderbook(t.Context(), tc.pair, tc.asset)
			require.NoError(t, err, "UpdateOrderbook must not error")
			require.NotNil(t, book, "book must not be nil")
			assert.True(t, tc.pair.Equal(book.Pair), "book.Pair should identify the requested pair")
			assert.Equal(t, tc.asset, book.Asset, "book.Asset should identify the requested asset")
			assert.NotEmpty(t, book.Bids, "book.Bids should contain bids")
			assert.NotEmpty(t, book.Asks, "book.Asks should contain asks")
		})
	}
}

func TestLiveGetRecentTrades(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	testexch.UpdatePairsOnce(t, e)
	for _, tc := range liveMarkets {
		t.Run(tc.asset.String(), func(t *testing.T) {
			trades, err := e.GetRecentTrades(t.Context(), tc.pair, tc.asset)
			require.NoError(t, err, "GetRecentTrades must not error")
			assert.NotEmpty(t, trades, "trades should not be empty for an active market")
		})
	}
}

func TestLiveGetHistoricCandles(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	testexch.UpdatePairsOnce(t, e)
	for _, tc := range liveMarkets {
		t.Run(tc.asset.String(), func(t *testing.T) {
			end := time.Now().UTC().Truncate(time.Minute)
			candles, err := e.GetHistoricCandles(t.Context(), tc.pair, tc.asset, kline.OneMin, end.Add(-time.Hour), end)
			require.NoError(t, err, "GetHistoricCandles must not error")
			require.NotNil(t, candles, "candles must not be nil")
			assert.NotEmpty(t, candles.Candles, "candles.Candles should not be empty for an active market")
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
