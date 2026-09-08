package hyperliquid

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

// Supply an account address for address-scoped live tests. Public tests do not
// need credentials; signing and transfer tests use isolated offline fixtures.
var apiCredentials = &accounts.Credentials{
	Key: "",
}

var e *Exchange

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Hyperliquid setup error: %s", err)
	}
	if apiCredentials.Key != "" {
		e.API.AuthenticatedSupport = true
		e.SetCredentials(apiCredentials)
	}
	code := m.Run()
	if err := e.Shutdown(); err != nil {
		log.Printf("Hyperliquid shutdown error: %s", err)
		code = 1
	}
	os.Exit(code)
}

func TestLivePerpetualMetadata(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	metadata, err := e.GetPerpetualMetadata(t.Context())
	require.NoError(t, err, "GetPerpetualMetadata must not error")
	require.NotNil(t, metadata, "metadata must not be nil")
	assert.NotEmpty(t, metadata.Universe, "metadata.Universe should contain markets")

	contexts, err := e.GetPerpetualMetadataAndAssetContexts(t.Context())
	require.NoError(t, err, "GetPerpetualMetadataAndAssetContexts must not error")
	require.NotNil(t, contexts, "contexts must not be nil")
	assert.Len(t, contexts.AssetContexts, len(contexts.Metadata.Universe), "contexts.AssetContexts should align with contexts.Metadata.Universe")

	dexes, err := e.GetPerpetualDEXs(t.Context())
	require.NoError(t, err, "GetPerpetualDEXs must not error")
	require.NotEmpty(t, dexes, "dexes must contain the default DEX")
	assert.Nil(t, dexes[0], "dexes[0] should represent the default DEX")
	for _, dex := range dexes[1:] {
		if dex == nil {
			continue
		}
		t.Run(dex.Name, func(t *testing.T) {
			metadata, err := e.GetPerpetualMetadataForDEX(t.Context(), dex.Name)
			require.NoError(t, err, "GetPerpetualMetadataForDEX must not error")
			assert.NotNil(t, metadata, "metadata should not be nil for a builder DEX")
			contexts, err := e.GetPerpetualMetadataAndAssetContextsForDEX(t.Context(), dex.Name)
			require.NoError(t, err, "GetPerpetualMetadataAndAssetContextsForDEX must not error")
			require.NotNil(t, contexts, "contexts must not be nil for a builder DEX")
			assert.Len(t, contexts.AssetContexts, len(contexts.Metadata.Universe), "contexts.AssetContexts should align with contexts.Metadata.Universe for a builder DEX")
			mids, err := e.GetAllMids(t.Context(), dex.Name)
			require.NoError(t, err, "GetAllMids must not error")
			assert.NotNil(t, mids, "mids should not be nil for a builder DEX")
		})
		break // One registered builder DEX exercises the route without querying every builder.
	}
}

func TestLiveSpotMetadata(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	metadata, err := e.GetSpotMetadata(t.Context())
	require.NoError(t, err, "GetSpotMetadata must not error")
	require.NotNil(t, metadata, "metadata must not be nil")
	assert.NotEmpty(t, metadata.Universe, "metadata.Universe should contain markets")
	assert.NotEmpty(t, metadata.Tokens, "metadata.Tokens should contain tokens")

	contexts, err := e.GetSpotMetadataAndAssetContexts(t.Context())
	require.NoError(t, err, "GetSpotMetadataAndAssetContexts must not error")
	require.NotNil(t, contexts, "contexts must not be nil")
	require.NotEmpty(t, contexts.AssetContexts, "contexts.AssetContexts must contain spot market data")
	byCoin := make(map[string]SpotAssetContext, len(contexts.AssetContexts))
	for _, market := range contexts.AssetContexts {
		byCoin[market.Coin] = market
	}
	// The endpoint also returns contexts for coins outside the spot universe.
	for _, market := range contexts.Metadata.Universe {
		assert.Contains(t, byCoin, market.Name, "byCoin should contain each spot market in contexts.Metadata.Universe")
	}
}

func TestLiveGetAllMids(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	mids, err := e.GetAllMids(t.Context(), "")
	require.NoError(t, err, "GetAllMids must not error")
	assert.NotEmpty(t, mids, "mids should contain active markets")
}

func TestLiveMarketData(t *testing.T) {
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
			coin, err := e.getCoin(t.Context(), market.pair, market.asset)
			require.NoError(t, err, "getCoin must not error")
			t.Run("Orderbook", func(t *testing.T) {
				book, err := e.GetL2Book(t.Context(), market.asset, &L2BookRequest{Coin: coin})
				require.NoError(t, err, "GetL2Book must not error")
				require.NotNil(t, book, "book must not be nil")
				assert.Equal(t, coin, book.Coin, "book.Coin should identify the requested market")
				require.Len(t, book.Levels, 2, "book.Levels must contain both sides")
				assert.NotEmpty(t, book.Levels[0], "book.Levels[0] should contain bids")
				assert.NotEmpty(t, book.Levels[1], "book.Levels[1] should contain asks")
			})
			t.Run("Trades", func(t *testing.T) {
				trades, err := e.GetRecentTradesForCoin(t.Context(), coin, market.asset)
				require.NoError(t, err, "GetRecentTradesForCoin must not error")
				require.NotEmpty(t, trades, "trades must not be empty for an active market")
				for _, trade := range trades {
					assert.Equal(t, coin, trade.Coin, "trade.Coin should identify the requested market")
				}
			})
			t.Run("Candles", func(t *testing.T) {
				end := time.Now().UTC().Truncate(time.Minute)
				candles, err := e.GetCandles(t.Context(), market.asset, &CandleRequest{
					Coin: coin, Interval: kline.OneMin, StartTime: end.Add(-time.Hour), EndTime: end,
				})
				require.NoError(t, err, "GetCandles must not error")
				require.NotEmpty(t, candles, "candles must not be empty for an active market")
				for _, candle := range candles {
					assert.Equal(t, coin, candle.Symbol, "candle.Symbol should identify the requested market")
					assert.Equal(t, "1m", candle.Interval, "candle.Interval should match the requested interval")
				}
			})
		})
	}
}

func TestLiveGetFundingHistory(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	end := time.Now().UTC()
	rates, err := e.GetFundingHistory(t.Context(), currency.BTC.String(), end.Add(-24*time.Hour), end)
	require.NoError(t, err, "GetFundingHistory must not error")
	require.NotEmpty(t, rates, "rates must not be empty for an active perpetual market")
	for _, rate := range rates {
		assert.Equal(t, currency.BTC.String(), rate.Coin, "rate.Coin should identify the requested market")
	}
}
