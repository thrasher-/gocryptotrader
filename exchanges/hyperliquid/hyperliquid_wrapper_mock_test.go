package hyperliquid

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var (
	testPerpetualPair = currency.NewPairWithDelimiter("BTC", "USDC", "-")
	testSpotPair      = currency.NewPairWithDelimiter("HYPE", "USDC", "-")
)

func setTestPair(t *testing.T, ex *Exchange, a asset.Item, pair currency.Pair, coin string) {
	t.Helper()
	ex.setPairMappings(a, []pairMapping{{pair: pair, coin: coin}})
	require.NoError(t, ex.UpdatePairs(currency.Pairs{pair}, a, false), "UpdatePairs: updating available test pairs must not error")
	require.NoError(t, ex.UpdatePairs(currency.Pairs{pair}, a, true), "UpdatePairs: updating enabled test pairs must not error")
}

func TestLogDefaultError(t *testing.T) {
	assert.NotPanics(t, func() { logDefaultError(nil) }, "logDefaultError should not panic for a nil error")
	assert.NotPanics(t, func() { logDefaultError(assert.AnError) }, "logDefaultError should not panic for a non-nil error")
}

func TestSetDefaults(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	assert.Equal(t, "Hyperliquid", ex.Name, "ex.Name: exchange name should be configured")
	assert.True(t, ex.Enabled, "ex.Enabled: exchange should be enabled by default")
	assert.True(t, ex.Features.Supports.REST, "ex.Features.Supports.REST should be supported")
	assert.True(t, ex.Features.Supports.Websocket, "ex.Features.Supports.Websocket should be supported")
	assert.True(t, ex.Features.Supports.RESTCapabilities.AuthenticatedEndpoints, "ex.Features.Supports.RESTCapabilities.AuthenticatedEndpoints: authenticated REST endpoints should be supported")
	assert.True(t, ex.Features.Supports.RESTCapabilities.SubmitOrder, "ex.Features.Supports.RESTCapabilities.SubmitOrder: REST order submission should be supported")
	assert.True(t, ex.Features.Supports.RESTCapabilities.TradeFee, "ex.Features.Supports.RESTCapabilities.TradeFee: account-specific trade fees should be supported")
	assert.True(t, ex.Features.Supports.RESTCapabilities.CryptoWithdrawal, "ex.Features.Supports.RESTCapabilities.CryptoWithdrawal: USDC bridge withdrawals should be supported")
	assert.True(t, ex.Features.Supports.RESTCapabilities.DepositHistory, "ex.Features.Supports.RESTCapabilities.DepositHistory: account deposit history should be supported")
	assert.True(t, ex.Features.Supports.RESTCapabilities.WithdrawalHistory, "ex.Features.Supports.RESTCapabilities.WithdrawalHistory: bridge withdrawal history should be supported")
	assert.True(t, ex.HasAssetTypeAccountSegregation(), "HasAssetTypeAccountSegregation: separate spot and perpetual balance pools should report asset type account segregation")
	assert.Equal(t, exchange.AutoWithdrawCryptoWithSetup, ex.Features.Supports.WithdrawPermissions, "ex.Features.Supports.WithdrawPermissions: withdrawal permissions should require master-wallet setup")
	assert.True(t, ex.Features.Supports.WebsocketCapabilities.AuthenticatedEndpoints, "ex.Features.Supports.WebsocketCapabilities.AuthenticatedEndpoints: address-scoped websocket feeds should be supported")
	assert.True(t, ex.Features.Supports.RESTCapabilities.TickerBatching, "ex.Features.Supports.RESTCapabilities.TickerBatching should be supported")
	assert.True(t, ex.Features.Supports.RESTCapabilities.AutoPairUpdates, "ex.Features.Supports.RESTCapabilities.AutoPairUpdates: automatic pair updates should be supported")
	assert.True(t, ex.Features.Supports.FuturesCapabilities.FundingRates, "ex.Features.Supports.FuturesCapabilities.FundingRates: perpetual funding rates should be supported")
	assert.True(t, ex.Features.Supports.FuturesCapabilities.Leverage, "ex.Features.Supports.FuturesCapabilities.Leverage: perpetual leverage readback should be supported")
	assert.True(t, ex.Features.Supports.FuturesCapabilities.OpenInterest.Supported, "ex.Features.Supports.FuturesCapabilities.OpenInterest.Supported: perpetual open interest should be supported")
	assert.Equal(t, uint64(maximumCandleCount), ex.Features.Enabled.Kline.GlobalResultLimit, "ex.Features.Enabled.Kline.GlobalResultLimit: candle result limit should match Hyperliquid retention")
	assert.NotNil(t, ex.Requester, "ex.Requester: REST requester should be initialised")
	assert.NotNil(t, ex.Websocket, "ex.Websocket: websocket manager should be initialised")
	assert.NotNil(t, ex.pairMappings, "ex.pairMappings should be initialised")
	assert.NotNil(t, ex.pairMappingMisses, "ex.pairMappingMisses: pair mapping miss cache should be initialised")
	spotURL, err := ex.API.Endpoints.GetURL(exchange.RestSpot)
	require.NoError(t, err, "GetURL must not error for the default spot URL")
	futuresURL, err := ex.API.Endpoints.GetURL(exchange.RestFutures)
	require.NoError(t, err, "GetURL must not error for the default futures URL")
	assert.Equal(t, hyperliquidAPIURL, spotURL, "spotURL should use the Hyperliquid API")
	assert.Equal(t, spotURL, futuresURL, "futuresURL: spot and futures should share the Hyperliquid API URL")
	websocketURL, err := ex.API.Endpoints.GetURL(exchange.WebsocketSpot)
	require.NoError(t, err, "GetURL must not error for the default websocket URL")
	assert.Equal(t, hyperliquidWebsocketURL, websocketURL, "websocketURL should use the Hyperliquid stream")
	for _, a := range []asset.Item{asset.Spot, asset.PerpetualContract} {
		format, err := ex.GetPairFormat(a, true)
		require.NoError(t, err, "GetPairFormat: getting the request pair format must not error")
		assert.True(t, format.Uppercase, "format.Uppercase: request pair format should be uppercase")
		assert.Equal(t, currency.DashDelimiter, format.Delimiter, "format.Delimiter: request pair format should use a dash")
	}
	require.NoError(t, ex.Shutdown(), "Shutdown must not error for the default exchange")
}

func TestConfigSetup(t *testing.T) {
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error for Hyperliquid from configtest")
	assert.True(t, ex.SupportsAsset(asset.Spot), "SupportsAsset: configtest should enable spot markets")
	assert.True(t, ex.SupportsAsset(asset.PerpetualContract), "SupportsAsset: configtest should enable perpetual markets")
	require.NoError(t, ex.Shutdown(), "Shutdown must not error for the configured exchange")
}

func TestSetPairMappings(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	mappings := []pairMapping{{pair: testSpotPair, coin: "@107"}}
	ex.setPairMappings(asset.Spot, mappings)
	mappings[0].coin = "changed"
	coin, err := ex.getCoin(t.Context(), testSpotPair, asset.Spot)
	require.NoError(t, err, "getCoin must not error for a cloned pair mapping")
	assert.Equal(t, "@107", coin, "coin: stored pair mapping in an initialised map should not alias the source slice")

	ex.pairMappings = nil
	mappings[0].coin = "@108"
	ex.setPairMappings(asset.Spot, mappings)
	mappings[0].coin = "changed again"
	coin, err = ex.getCoin(t.Context(), testSpotPair, asset.Spot)
	require.NoError(t, err, "getCoin must not error for a cloned pair mapping from a newly initialised map")
	assert.Equal(t, "@108", coin, "coin: stored pair mapping in a newly initialised map should not alias the source slice")
	require.NoError(t, ex.Shutdown(), "Shutdown must not error for the exchange")
}

func TestLookupPairMapping(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, ok := ex.lookupPairMapping(testSpotPair, asset.Spot)
	assert.False(t, ok, "ok: missing pair mapping should not be found")

	expected := pairMapping{pair: testSpotPair, coin: "@107"}
	ex.setPairMappings(asset.Spot, []pairMapping{expected})
	result, ok := ex.lookupPairMapping(testSpotPair, asset.Spot)
	require.True(t, ok, "ok: cached pair mapping must be found")
	assert.Equal(t, expected, result, "result: cached pair mapping should match")
	require.NoError(t, ex.Shutdown(), "Shutdown must not error for the lookup exchange")
}

func TestFetchPairMapping(t *testing.T) {
	cached := new(Exchange)
	cached.SetDefaults()
	expected := pairMapping{pair: testPerpetualPair, coin: "BTC"}
	cached.setPairMappings(asset.PerpetualContract, []pairMapping{expected})
	result, err := cached.fetchPairMapping(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "fetchPairMapping must not error for a concurrently cached pair mapping")
	assert.Equal(t, expected, result, "result: concurrently cached pair mapping should be returned")
	require.NoError(t, cached.Shutdown(), "Shutdown must not error for the cached exchange")

	var fetches atomic.Int32
	refreshed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload), "Decode should not error for pair metadata request") {
			return
		}
		fetches.Add(1)
		switch payload.Type {
		case infoTypePerpetualDEXs:
			_, err := w.Write([]byte(`[null]`))
			assert.NoError(t, err, "Write should not error for perpetual DEX registry")
		case infoTypeMetadata:
			_, err := w.Write([]byte(perpetualMetadataJSON))
			assert.NoError(t, err, "Write should not error for perpetual metadata")
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	refreshed.setPairMappings(asset.PerpetualContract, nil)
	result, err = refreshed.fetchPairMapping(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "fetchPairMapping must not error for a pair mapping")
	assert.Equal(t, "BTC", result.coin, "result.coin: refreshed pair mapping should be returned")
	refreshed.pairMappingMisses = nil
	missingPair := currency.NewPair(currency.ETH, currency.USDC)
	_, err = refreshed.fetchPairMapping(t.Context(), missingPair, asset.PerpetualContract)
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMapping must return the expected error for missing refreshed pair mapping")
	fetchCount := fetches.Load()
	_, err = refreshed.fetchPairMapping(t.Context(), missingPair, asset.PerpetualContract)
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMapping must return the expected error for cached missing pair mapping")
	assert.Equal(t, fetchCount, fetches.Load(), "fetches: cached missing pair mapping should not refetch metadata")
	cacheKey := "pair:" + asset.PerpetualContract.String() + ":" + strings.ToLower(missingPair.String())
	refreshed.pairMappingMisses[cacheKey] = time.Now().Add(-time.Second)
	_, err = refreshed.fetchPairMapping(t.Context(), missingPair, asset.PerpetualContract)
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMapping must return the expected error for expired missing pair mapping")
	assert.Greater(t, fetches.Load(), fetchCount, "fetches: expired missing pair mapping should refresh metadata")

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = failed.fetchPairMapping(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.Error(t, err, "fetchPairMapping must return pair mapping refresh failure")
}

func TestLookupPairMappingByCoin(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, _, err := ex.lookupPairMappingByCoin("BTC")
	require.ErrorIs(t, err, errPairMappingNotFound, "lookupPairMappingByCoin must return the expected error for missing coin mapping")

	expected := pairMapping{pair: testPerpetualPair, coin: "BTC"}
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{expected})
	result, a, err := ex.lookupPairMappingByCoin("BTC")
	require.NoError(t, err, "lookupPairMappingByCoin must not error for unique coin mapping")
	assert.Equal(t, expected, result, "result: unique coin mapping should match")
	assert.Equal(t, asset.PerpetualContract, a, "a: unique coin mapping should return its asset")

	ex.setPairMappings(asset.Spot, []pairMapping{{pair: testSpotPair, coin: "BTC"}})
	_, _, err = ex.lookupPairMappingByCoin("BTC")
	require.ErrorIs(t, err, errAmbiguousCoinMapping, "lookupPairMappingByCoin must return the expected error for ambiguous coin mapping")
	require.NoError(t, ex.Shutdown(), "Shutdown must not error for the coin lookup exchange")
}

func TestFetchPairMappingByCoin(t *testing.T) {
	cached := new(Exchange)
	cached.SetDefaults()
	expected := pairMapping{pair: testPerpetualPair, coin: "BTC"}
	cached.setPairMappings(asset.PerpetualContract, []pairMapping{expected})
	result, a, err := cached.fetchPairMappingByCoin(t.Context(), "BTC")
	require.NoError(t, err, "fetchPairMappingByCoin must not error for a concurrently cached coin mapping")
	assert.Equal(t, expected, result, "result: concurrently cached coin mapping should be returned")
	assert.Equal(t, asset.PerpetualContract, a, "a: concurrently cached coin mapping should retain its asset")

	cached.setPairMappings(asset.Spot, []pairMapping{{pair: testSpotPair, coin: "BTC"}})
	_, _, err = cached.fetchPairMappingByCoin(t.Context(), "BTC")
	require.ErrorIs(t, err, errAmbiguousCoinMapping, "fetchPairMappingByCoin must return the expected error for concurrently cached ambiguous mapping")
	require.NoError(t, cached.Shutdown(), "Shutdown must not error for the cached coin exchange")

	unsupported := new(Exchange)
	_, _, err = unsupported.fetchPairMappingByCoin(t.Context(), "BTC")
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMappingByCoin must return a missing coin mapping for exchange without supported assets")

	var fetches atomic.Int32
	refreshed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload), "Decode should not error for coin metadata request") {
			return
		}
		fetches.Add(1)
		switch payload.Type {
		case infoTypePerpetualDEXs:
			_, err := w.Write([]byte(`[null]`))
			assert.NoError(t, err, "Write should not error for perpetual DEX registry")
		case "meta":
			_, err := w.Write([]byte(perpetualMetadataJSON))
			assert.NoError(t, err, "Write should not error for perpetual metadata")
		case "spotMeta":
			_, err := w.Write([]byte(spotMetadataJSON))
			assert.NoError(t, err, "Write should not error for spot metadata")
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	refreshed.setPairMappings(asset.Spot, nil)
	refreshed.setPairMappings(asset.PerpetualContract, nil)
	result, a, err = refreshed.fetchPairMappingByCoin(t.Context(), "BTC")
	require.NoError(t, err, "fetchPairMappingByCoin must not error for a coin mapping")
	assert.Equal(t, "BTC", result.coin, "result.coin: refreshed coin mapping should be returned")
	assert.Equal(t, asset.PerpetualContract, a, "a: refreshed coin mapping should return its asset")
	_, _, err = refreshed.fetchPairMappingByCoin(t.Context(), "MISSING")
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMappingByCoin must return the expected error for missing refreshed coin mapping")
	fetchCount := fetches.Load()
	_, _, err = refreshed.fetchPairMappingByCoin(t.Context(), "MISSING")
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMappingByCoin must return the expected error for cached missing coin mapping")
	assert.Equal(t, fetchCount, fetches.Load(), "fetches: cached missing coin mapping should not refetch metadata")
	refreshed.pairMappingMisses["coin:missing"] = time.Now().Add(-time.Second)
	_, _, err = refreshed.fetchPairMappingByCoin(t.Context(), "MISSING")
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMappingByCoin must return the expected error for expired missing coin mapping")
	assert.Greater(t, fetches.Load(), fetchCount, "fetches: expired missing coin mapping should refresh metadata")

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, _, err = failed.fetchPairMappingByCoin(t.Context(), "BTC")
	require.Error(t, err, "fetchPairMappingByCoin must return coin mapping refresh failure")
}

func TestGetPairMappingByCoin(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"meta": perpetualMetadataJSON, "spotMeta": spotMetadataJSON})
	_, _, err := ex.getPairMappingByCoin(t.Context(), "")
	require.ErrorIs(t, err, errCoinRequired, "getPairMappingByCoin must return the expected error for empty API coin")

	ex.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: testPerpetualPair, coin: "BTC"}})
	result, a, err := ex.getPairMappingByCoin(t.Context(), "BTC")
	require.NoError(t, err, "getPairMappingByCoin must not error for a cached coin mapping")
	assert.Equal(t, testPerpetualPair, result.pair, "result.pair: cached coin mapping should be returned")
	assert.Equal(t, asset.PerpetualContract, a, "a: cached coin mapping should return its asset")

	ex.setPairMappings(asset.Spot, []pairMapping{{pair: testSpotPair, coin: "BTC"}})
	_, _, err = ex.getPairMappingByCoin(t.Context(), "BTC")
	require.ErrorIs(t, err, errAmbiguousCoinMapping, "getPairMappingByCoin must return the expected error for an ambiguous cached coin mapping")

	ex.setPairMappings(asset.Spot, nil)
	ex.setPairMappings(asset.PerpetualContract, nil)
	result, a, err = ex.getPairMappingByCoin(t.Context(), "@107")
	require.NoError(t, err, "getPairMappingByCoin must not error for a refreshed coin mapping")
	assert.True(t, result.pair.Equal(testSpotPair), "Equal: refreshed coin mapping should be returned")
	assert.Equal(t, asset.Spot, a, "a: refreshed coin mapping should return its asset")
}

func TestGetCoin(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"spotMeta": spotMetadataJSON})
	_, err := ex.getCoin(t.Context(), testSpotPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "getCoin must return the expected error for unsupported asset")
	_, err = ex.getCoin(t.Context(), currency.EMPTYPAIR, asset.Spot)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "getCoin must return the expected error for empty pair")

	ex.setPairMappings(asset.Spot, []pairMapping{{pair: testSpotPair, coin: "@cached"}})
	coin, err := ex.getCoin(t.Context(), testSpotPair, asset.Spot)
	require.NoError(t, err, "getCoin must not error for a cached coin mapping")
	assert.Equal(t, "@cached", coin, "coin: cached coin mapping should be returned")

	ex.setPairMappings(asset.Spot, nil)
	coin, err = ex.getCoin(t.Context(), testSpotPair, asset.Spot)
	require.NoError(t, err, "getCoin must not error for a coin mapping")
	assert.Equal(t, "@107", coin, "coin: refreshed coin mapping should be returned")

	_, err = ex.getCoin(t.Context(), currency.NewPairWithDelimiter("MISSING", "USDC", "-"), asset.Spot)
	require.ErrorIs(t, err, errPairMappingNotFound, "getCoin must return the expected error for missing refreshed mapping")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.getCoin(t.Context(), testSpotPair, asset.Spot)
	require.Error(t, err, "getCoin must error for a mapping from a failing server")
}

func TestFetchTradablePairs(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		"meta":     `{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":40,"onlyIsolated":true},{"name":"OLD","isDelisted":true}]}`,
		"spotMeta": `{"universe":[{"tokens":[150,0],"name":"@107"}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150}]}`,
	})
	_, err := ex.FetchTradablePairs(t.Context(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "FetchTradablePairs must return the expected error for unsupported asset")
	_, err = new(Exchange).FetchTradablePairs(t.Context(), asset.Spot)
	require.ErrorIs(t, err, asset.ErrNotSupported, "FetchTradablePairs must return the expected error for unconfigured spot support")

	perpetualPairs, err := ex.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "FetchTradablePairs must not error for perpetual pairs")
	require.Len(t, perpetualPairs, 1, "perpetualPairs: delisted perpetual pairs must be excluded")
	assert.True(t, perpetualPairs[0].Equal(testPerpetualPair), "Equal: perpetual pair should use USDC as quote")
	coin, err := ex.getCoin(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "getCoin must not error for the fetched perpetual mapping")
	assert.Equal(t, "BTC", coin, "coin: perpetual mapping should use the API coin")
	mapping, err := ex.getPairMapping(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "getPairMapping must not error for fetched perpetual metadata")
	assert.Equal(t, uint64(40), mapping.maxLeverage, "mapping.maxLeverage: perpetual mapping should retain the market leverage limit")
	assert.True(t, mapping.onlyIsolated, "mapping.onlyIsolated: perpetual mapping should retain the isolated-only restriction")

	hip3 := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for HIP-3 discovery request") {
			return
		}
		var response string
		switch request.Type {
		case infoTypePerpetualDEXs:
			response = `[null,{"name":"xyz"},{"name":"flx"}]`
		case "meta":
			switch request.DEX {
			case "":
				response = `{"universe":[{"name":"BTC","szDecimals":5}]}`
			case testBuilderDEXName:
				response = `{"universe":[{"name":"xyz:XYZ100","szDecimals":2},{"name":"xyz:OLD","isDelisted":true}]}`
			case "flx":
				response = `{"universe":[{"name":"flx:TSLA","szDecimals":3}]}`
			default:
				http.Error(w, "unexpected DEX", http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Write should not error for HIP-3 discovery response")
	}))
	hip3Pairs, err := hip3.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "FetchTradablePairs must not error for default and HIP-3 markets")
	require.Len(t, hip3Pairs, 3, "FetchTradablePairs must retain active markets from every perpetual DEX")
	for _, tc := range []struct {
		pair    currency.Pair
		coin    string
		dex     string
		assetID uint64
	}{
		{pair: testPerpetualPair, coin: "BTC", assetID: 0},
		{pair: currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.USDC), coin: "xyz:XYZ100", dex: testBuilderDEXName, assetID: 110000},
		{pair: currency.NewPair(currency.NewCode("flx:TSLA"), currency.USDC), coin: "flx:TSLA", dex: "flx", assetID: 120000},
	} {
		mapping, err := hip3.getPairMapping(t.Context(), tc.pair, asset.PerpetualContract)
		require.NoError(t, err, "getPairMapping must not error for discovered DEX mapping")
		assert.Equal(t, tc.coin, mapping.coin, "mapping.coin: DEX mapping should retain the API coin")
		assert.Equal(t, tc.dex, mapping.dex, "mapping.dex: DEX mapping should retain its DEX scope")
		assert.Equal(t, tc.assetID, mapping.assetID, "mapping.assetID: DEX mapping should apply the official asset-ID offset")
	}

	variableCollateral := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for variable-collateral discovery request") {
			return
		}
		var response string
		switch request.Type {
		case infoTypePerpetualDEXs:
			response = `[null,{"name":"xyz"}]`
		case infoTypeMetadata:
			if request.DEX == testBuilderDEXName {
				response = `{"collateralToken":150,"universe":[{"name":"xyz:XYZ100","szDecimals":2}]}`
			} else {
				response = `{"collateralToken":0,"universe":[{"name":"BTC","szDecimals":5}]}`
			}
		case "spotMeta":
			response = `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150}]}`
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Write should not error for variable-collateral discovery response")
	}))
	variablePairs, err := variableCollateral.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "FetchTradablePairs must not error for a HIP-3 market with non-USDC collateral")
	assert.Contains(t, variablePairs, currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.NewCode("HYPE")), "variablePairs: HIP-3 pair quote should use the DEX collateral token")

	for _, tc := range []struct {
		name         string
		registry     string
		builderMeta  string
		secondMeta   string
		spotMetadata string
		spotFailure  bool
		expectedIs   error
	}{
		{
			name:        "spot metadata failure",
			registry:    `[null,{"name":"xyz"}]`,
			builderMeta: `{"collateralToken":150,"universe":[{"name":"xyz:XYZ100"}]}`,
			spotFailure: true,
		},
		{
			name:         "duplicate collateral token",
			registry:     `[null,{"name":"xyz"}]`,
			builderMeta:  `{"collateralToken":150,"universe":[{"name":"xyz:XYZ100"}]}`,
			spotMetadata: `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"HYPE2","index":150}]}`,
			expectedIs:   errUnexpectedResponseLength,
		},
		{
			name:         "blank collateral token",
			registry:     `[null,{"name":"xyz"}]`,
			builderMeta:  `{"collateralToken":150,"universe":[{"name":"xyz:XYZ100"}]}`,
			spotMetadata: `{"tokens":[{"name":"USDC","index":0},{"name":" ","index":150}]}`,
			expectedIs:   errSpotTokenNotFound,
		},
		{
			name:         "invalid canonical collateral",
			registry:     `[null,{"name":"xyz"}]`,
			builderMeta:  `{"collateralToken":150,"universe":[{"name":"xyz:XYZ100"}]}`,
			spotMetadata: `{"tokens":[{"name":"USDT","index":0},{"name":"HYPE","index":150}]}`,
			expectedIs:   errSpotTokenNotFound,
		},
		{
			name:         "missing collateral token",
			registry:     `[null,{"name":"xyz"}]`,
			builderMeta:  `{"collateralToken":150,"universe":[{"name":"xyz:XYZ100"}]}`,
			spotMetadata: `{"tokens":[{"name":"USDC","index":0}]}`,
			expectedIs:   errSpotTokenNotFound,
		},
		{
			name:         "later DEX missing collateral token",
			registry:     `[null,{"name":"xyz"},{"name":"flx"}]`,
			builderMeta:  `{"collateralToken":150,"universe":[{"name":"xyz:XYZ100"}]}`,
			secondMeta:   `{"collateralToken":151,"universe":[{"name":"flx:TSLA"}]}`,
			spotMetadata: `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150}]}`,
			expectedIs:   errSpotTokenNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request infoRequest
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for invalid collateral discovery request") {
					return
				}
				switch request.Type {
				case infoTypePerpetualDEXs:
					_, err := w.Write([]byte(tc.registry))
					assert.NoError(t, err, "Write should not error for invalid collateral registry")
				case infoTypeMetadata:
					response := `{"collateralToken":0,"universe":[{"name":"BTC"}]}`
					switch request.DEX {
					case testBuilderDEXName:
						response = tc.builderMeta
					case "flx":
						response = tc.secondMeta
					}
					_, err := w.Write([]byte(response))
					assert.NoError(t, err, "Write should not error for invalid collateral metadata")
				case "spotMeta":
					if tc.spotFailure {
						http.Error(w, "spot metadata unavailable", http.StatusServiceUnavailable)
						return
					}
					_, err := w.Write([]byte(tc.spotMetadata))
					assert.NoError(t, err, "Write should not error for invalid spot metadata")
				default:
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			_, err := invalid.FetchTradablePairs(t.Context(), asset.PerpetualContract)
			require.Error(t, err, "FetchTradablePairs must return an error for invalid collateral discovery")
			if tc.expectedIs != nil {
				require.ErrorIs(t, err, tc.expectedIs, "FetchTradablePairs must return the expected error for invalid collateral discovery")
			}
		})
	}

	spotPairs, err := ex.FetchTradablePairs(t.Context(), asset.Spot)
	require.NoError(t, err, "FetchTradablePairs must not error for spot pairs")
	require.Len(t, spotPairs, 1, "spotPairs: spot metadata must produce one pair")
	assert.True(t, spotPairs[0].Equal(testSpotPair), "Equal: spot pair should use token metadata names")
	coin, err = ex.getCoin(t.Context(), testSpotPair, asset.Spot)
	require.NoError(t, err, "getCoin must not error for the fetched spot mapping")
	assert.Equal(t, "@107", coin, "coin: spot mapping should retain the API market identifier")

	duplicatePerpetual := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[{"name":"BTC"},{"name":"BTC"},{"name":"ETH"}]}`})
	perpetualPairs, err = duplicatePerpetual.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "FetchTradablePairs must not error for perpetual markets with an ambiguous display pair")
	require.Len(t, perpetualPairs, 1, "perpetualPairs: ambiguous perpetual display pairs must be skipped")
	assert.True(t, perpetualPairs[0].Equal(currency.NewPair(currency.ETH, currency.USDC)), "Equal: unambiguous perpetual pairs should be retained")

	duplicateSpot := newStaticInfoExchange(t, map[string]string{
		"spotMeta": `{"universe":[{"tokens":[150,0],"name":"@107","index":107},{"tokens":[151,0],"name":"@108","index":108},{"tokens":[152,0],"name":"@109","index":109}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"HYPE","index":151},{"name":"PURR","index":152}]}`,
	})
	spotPairs, err = duplicateSpot.FetchTradablePairs(t.Context(), asset.Spot)
	require.NoError(t, err, "FetchTradablePairs must not error for spot markets with an ambiguous display pair")
	require.Len(t, spotPairs, 1, "spotPairs: ambiguous spot display pairs must be skipped")
	assert.True(t, spotPairs[0].Equal(currency.NewPair(currency.NewCode("PURR"), currency.USDC)), "Equal: unambiguous spot pairs should be retained")
	coin, err = duplicateSpot.getCoin(t.Context(), spotPairs[0], asset.Spot)
	require.NoError(t, err, "getCoin must not error for an unambiguous spot mapping")
	assert.Equal(t, "@109", coin, "coin: unambiguous spot mapping should retain the API market identifier")

	invalidPerpetual := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[{"name":"BAD COIN"},{"name":"ETH"}]}`})
	perpetualPairs, err = invalidPerpetual.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "FetchTradablePairs must not error for a perpetual market with an invalid currency name")
	require.Len(t, perpetualPairs, 1, "perpetualPairs: invalid perpetual names must be skipped")
	assert.True(t, perpetualPairs[0].Equal(currency.NewPair(currency.ETH, currency.USDC)), "Equal: valid perpetual markets should remain available")

	for _, tc := range []struct {
		name       string
		registry   string
		metadata   string
		expectedIs error
	}{
		{name: "missing builder entry", registry: `[null,null]`, metadata: perpetualMetadataJSON, expectedIs: errInvalidPerpetualDEX},
		{name: "blank builder name", registry: `[null,{"name":" "}]`, metadata: perpetualMetadataJSON, expectedIs: errInvalidPerpetualDEX},
		{name: "duplicate builder name", registry: `[null,{"name":"xyz"},{"name":"xyz"}]`, metadata: `{"universe":[{"name":"xyz:XYZ100"}]}`, expectedIs: errInvalidPerpetualDEX},
		{name: "unscoped builder market", registry: `[null,{"name":"xyz"}]`, metadata: `{"universe":[{"name":"XYZ100"}]}`, expectedIs: errInvalidPerpetualDEX},
		{
			name:       "too many builder markets",
			registry:   `[null,{"name":"xyz"}]`,
			metadata:   `{"universe":[` + strings.Repeat(`{"name":"xyz:X"},`, builderPerpetualDEXAssetStride) + `{"name":"xyz:X"}]}`,
			expectedIs: errInvalidPerpetualDEX,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request infoRequest
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for invalid DEX request") {
					return
				}
				response := tc.metadata
				if request.Type == infoTypePerpetualDEXs {
					response = tc.registry
				}
				_, err := w.Write([]byte(response))
				assert.NoError(t, err, "Write should not error for invalid DEX response")
			}))
			_, err := invalid.FetchTradablePairs(t.Context(), asset.PerpetualContract)
			require.ErrorIs(t, err, tc.expectedIs, "FetchTradablePairs must return the expected error for invalid DEX discovery")
		})
	}

	metadataFailure := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for failing DEX request") {
			return
		}
		if request.Type == infoTypePerpetualDEXs {
			_, err := w.Write([]byte(`[null,{"name":"xyz"}]`))
			assert.NoError(t, err, "Write should not error for perpetual DEX registry")
			return
		}
		http.Error(w, "metadata unavailable", http.StatusServiceUnavailable)
	}))
	_, err = metadataFailure.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.Error(t, err, "FetchTradablePairs must return builder metadata failure")

	for _, tc := range []struct {
		name     string
		response string
	}{
		{
			name:     "invalid token count",
			response: `{"universe":[{"tokens":[1],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"PURR","index":2}]}`,
		},
		{
			name:     "missing base token",
			response: `{"universe":[{"tokens":[1,0],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"PURR","index":2}]}`,
		},
		{
			name:     "missing quote token",
			response: `{"universe":[{"tokens":[1,9],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"TOKEN","index":1},{"name":"PURR","index":2}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalidExchange := newStaticInfoExchange(t, map[string]string{"spotMeta": tc.response})
			pairs, err := invalidExchange.FetchTradablePairs(t.Context(), asset.Spot)
			require.NoError(t, err, "FetchTradablePairs must not error for structurally invalid spot metadata")
			require.Len(t, pairs, 1, "pairs: structurally invalid spot markets must be skipped")
			assert.True(t, pairs[0].Equal(currency.NewPair(currency.NewCode("PURR"), currency.USDC)), "Equal: valid spot markets should remain available")
		})
	}
	for _, tc := range []struct {
		name     string
		response string
	}{
		{
			name:     "empty token name",
			response: `{"universe":[{"tokens":[1,0],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"","index":1},{"name":"PURR","index":2}]}`,
		},
		{
			name:     "invalid token spacing",
			response: `{"universe":[{"tokens":[1,0],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"BAD COIN","index":1},{"name":"PURR","index":2}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalidExchange := newStaticInfoExchange(t, map[string]string{"spotMeta": tc.response})
			pairs, err := invalidExchange.FetchTradablePairs(t.Context(), asset.Spot)
			require.NoError(t, err, "FetchTradablePairs must not error for spot metadata with an invalid token name")
			require.Len(t, pairs, 1, "pairs: invalid spot token names must be skipped")
			assert.True(t, pairs[0].Equal(currency.NewPair(currency.NewCode("PURR"), currency.USDC)), "Equal: valid spot markets should remain available")
		})
	}

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.FetchTradablePairs(t.Context(), asset.Spot)
	require.Error(t, err, "FetchTradablePairs must error for spot pairs from a failing server")
	_, err = errorExchange.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.Error(t, err, "FetchTradablePairs must error for perpetual pairs from a failing server")
}

func TestFetchTradablePairsRejectsDuplicateSpotIdentities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
	}{
		{
			name:     "token index",
			response: `{"universe":[{"tokens":[150,0],"name":"@107","index":107}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"PURR","index":150}]}`,
		},
		{
			name:     "market index",
			response: `{"universe":[{"tokens":[150,0],"name":"@107","index":107},{"tokens":[151,0],"name":"@108","index":107}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"PURR","index":151}]}`,
		},
		{
			name:     "market name",
			response: `{"universe":[{"tokens":[150,0],"name":"@107","index":107},{"tokens":[151,0],"name":"@107","index":108}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"PURR","index":151}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := newStaticInfoExchange(t, map[string]string{"spotMeta": tc.response})
			_, err := ex.FetchTradablePairs(t.Context(), asset.Spot)
			require.ErrorIs(t, err, errUnexpectedResponseLength, "FetchTradablePairs must return the expected error for duplicate spot metadata identity")
			assert.Empty(t, ex.pairMappings[asset.Spot], "ex.pairMappings[asset.Spot]: rejected spot metadata should not install pair mappings")
		})
	}
}

func TestGetPerpetualDEXNames(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		infoTypePerpetualDEXs: `[null,{"name":" xyz "},{"name":"hyna"}]`,
	})
	names, err := ex.getPerpetualDEXNames(t.Context())
	require.NoError(t, err, "getPerpetualDEXNames must not error for valid perpetual DEX names")
	assert.Equal(t, []string{"", "xyz", "hyna"}, names, "names: perpetual DEX names should retain registry order and normalise whitespace")

	for _, tc := range []struct {
		name       string
		registry   string
		expectedIs error
	}{
		{name: "empty", registry: `[]`, expectedIs: errUnexpectedResponseLength},
		{name: "non-default first entry", registry: `[{"name":"default"}]`, expectedIs: errUnexpectedResponseLength},
		{name: "missing builder entry", registry: `[null,null]`, expectedIs: errInvalidPerpetualDEX},
		{name: "blank builder name", registry: `[null,{"name":" "}]`, expectedIs: errInvalidPerpetualDEX},
		{name: "duplicate builder name", registry: `[null,{"name":"xyz"},{"name":" xyz "}]`, expectedIs: errInvalidPerpetualDEX},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := newStaticInfoExchange(t, map[string]string{infoTypePerpetualDEXs: tc.registry})
			_, err := invalid.getPerpetualDEXNames(t.Context())
			require.ErrorIs(t, err, tc.expectedIs, "getPerpetualDEXNames must return the expected error for invalid perpetual DEX registry")
		})
	}

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = failed.getPerpetualDEXNames(t.Context())
	require.Error(t, err, "getPerpetualDEXNames must return perpetual DEX registry HTTP failure")
}

func TestUpdateTradablePairs(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"meta": perpetualMetadataJSON, "spotMeta": spotMetadataJSON})
	require.NoError(t, ex.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must not error for tradable pairs")
	spot, err := ex.GetAvailablePairs(asset.Spot)
	require.NoError(t, err, "GetAvailablePairs must not error for updated spot pairs")
	assert.Contains(t, spot, testSpotPair, "spot: updated spot pairs should contain HYPE-USDC")
	perpetual, err := ex.GetAvailablePairs(asset.PerpetualContract)
	require.NoError(t, err, "GetAvailablePairs must not error for updated perpetual pairs")
	assert.Contains(t, perpetual, testPerpetualPair, "perpetual: updated perpetual pairs should contain BTC-USDC")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	require.Error(t, errorExchange.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must error for tradable pairs from a failing server")

	emptyResponseExchange := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[]}`, "spotMeta": `{"universe":[],"tokens":[]}`})
	require.Error(t, emptyResponseExchange.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must error for tradable pairs from empty metadata")

	noAssetExchange := newStaticInfoExchange(t, map[string]string{})
	noAssetExchange.CurrencyPairs.Pairs = make(map[asset.Item]*currency.PairStore)
	require.Error(t, noAssetExchange.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must error for tradable pairs without configured assets")
}

func TestUpdatePerpetualTickers(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContextsJSON})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")
	require.NoError(t, ex.UpdateTickers(t.Context(), asset.PerpetualContract), "UpdateTickers must not error for perpetual tickers")
	price, err := ticker.GetTicker(ex.Name, testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetTicker: getting the updated perpetual ticker must not error")
	assert.Equal(t, 100.5, price.Last, "price.Last: perpetual ticker should use the current midpoint")
	assert.Equal(t, 10.0, price.Volume, "price.Volume: perpetual ticker should include base volume")
	assert.Equal(t, 1000.0, price.QuoteVolume, "price.QuoteVolume: perpetual ticker should include quote volume")
	assert.Equal(t, 99.0, price.Open, "price.Open: perpetual ticker should include the previous-day price")
	assert.Zero(t, price.Close, "price.Close: perpetual ticker should not label a midpoint or mark price as the close")
	assert.Zero(t, price.Bid, "price.Bid: perpetual ticker should not treat impact prices as the best bid")
	assert.Zero(t, price.Ask, "price.Ask: perpetual ticker should not treat impact prices as the best ask")
	assert.Equal(t, 101.0, price.MarkPrice, "price.MarkPrice: perpetual mark price should be populated")
	assert.Equal(t, 100.0, price.IndexPrice, "price.IndexPrice: perpetual index price should be populated")
	assert.WithinDuration(t, time.Now().UTC(), price.LastUpdated, time.Second, "price.LastUpdated: perpetual ticker update time should be current")

	hip3Pair := currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.USDC)
	var requestedDEXes []string
	hip3 := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for scoped ticker request") {
			return
		}
		requestedDEXes = append(requestedDEXes, request.DEX)
		response := perpetualContextsJSON
		if request.DEX == "xyz" {
			response = `[{"universe":[{"name":"xyz:XYZ100"}]},[{"openInterest":"20","prevDayPx":"49","dayNtlVlm":"2000","oraclePx":"50","markPx":"51","midPx":"50.5","dayBaseVlm":"40"}]]`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Write should not error for scoped ticker response")
	}))
	hip3.setPairMappings(asset.PerpetualContract, []pairMapping{
		{pair: testPerpetualPair, coin: "BTC"},
		{pair: hip3Pair, coin: "xyz:XYZ100", dex: "xyz", assetID: 110000},
	})
	require.NoError(t, hip3.UpdatePairs(currency.Pairs{testPerpetualPair, hip3Pair}, asset.PerpetualContract, false), "UpdatePairs: updating scoped ticker pairs must not error")
	require.NoError(t, hip3.UpdatePairs(currency.Pairs{testPerpetualPair, hip3Pair}, asset.PerpetualContract, true), "UpdatePairs: enabling scoped ticker pairs must not error")
	require.NoError(t, hip3.UpdateTickers(t.Context(), asset.PerpetualContract), "UpdateTickers must not error for default and HIP-3 tickers")
	assert.Equal(t, []string{"", "xyz"}, requestedDEXes, "requestedDEXes: ticker batch should query each required DEX once")
	hip3Price, err := ticker.GetTicker(hip3.Name, hip3Pair, asset.PerpetualContract)
	require.NoError(t, err, "GetTicker: getting updated HIP-3 ticker must not error")
	assert.Equal(t, 50.5, hip3Price.Last, "hip3Price.Last: HIP-3 ticker should use its DEX-scoped midpoint")
	assert.Equal(t, 20.0, hip3Price.OpenInterest, "hip3Price.OpenInterest: HIP-3 ticker should use its DEX-scoped open interest")

	fallbackJSON := `[` + perpetualMetadataJSON + `,[{"openInterest":"10","prevDayPx":"99","dayNtlVlm":"1000","oraclePx":"100","markPx":"101","midPx":"0","dayBaseVlm":"10"}]]`
	fallbackExchange := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": fallbackJSON})
	setTestPair(t, fallbackExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	require.NoError(t, fallbackExchange.UpdateTickers(t.Context(), asset.PerpetualContract), "UpdateTickers must not error for a perpetual ticker without a midpoint")
	price, err = ticker.GetTicker(fallbackExchange.Name, testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetTicker: getting the fallback perpetual ticker must not error")
	assert.Equal(t, 101.0, price.Last, "price.Last: perpetual ticker should fall back to the mark price when the midpoint is unavailable")

	lengthExchange := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": `[` + perpetualMetadataJSON + `,[]]`})
	setTestPair(t, lengthExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	require.ErrorIs(t, lengthExchange.UpdateTickers(t.Context(), asset.PerpetualContract), errUnexpectedResponseLength, "UpdateTickers must return the expected error for mismatched perpetual contexts")

	missingExchange := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContextsJSON})
	setTestPair(t, missingExchange, asset.PerpetualContract, testPerpetualPair, "ETH")
	require.ErrorIs(t, missingExchange.UpdateTickers(t.Context(), asset.PerpetualContract), errAssetContextNotFound, "UpdateTickers must return the expected error for missing perpetual context")

	processExchange := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContextsJSON})
	setTestPair(t, processExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	processExchange.Name = ""
	require.ErrorIs(t, processExchange.UpdateTickers(t.Context(), asset.PerpetualContract), common.ErrExchangeNameNotSet, "UpdateTickers must return its processing error for invalid perpetual ticker")

	mappingExchange := newStaticInfoExchange(t, map[string]string{
		"metaAndAssetCtxs": perpetualContextsJSON,
		"meta":             `{"universe":[{"name":"ETH"}]}`,
	})
	setTestPair(t, mappingExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	mappingExchange.setPairMappings(asset.PerpetualContract, nil)
	require.ErrorIs(t, mappingExchange.UpdateTickers(t.Context(), asset.PerpetualContract), errPairMappingNotFound, "UpdateTickers must return the expected error for missing perpetual pair mapping")

	mixedExchange := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContextsJSON})
	missingPair := currency.NewPair(currency.ETH, currency.USDC)
	mixedExchange.setPairMappings(asset.PerpetualContract, []pairMapping{
		{pair: testPerpetualPair, coin: "BTC"},
		{pair: missingPair, coin: "ETH"},
	})
	require.NoError(t, mixedExchange.UpdatePairs(currency.Pairs{testPerpetualPair, missingPair}, asset.PerpetualContract, false), "UpdatePairs: updating mixed available pairs must not error")
	require.NoError(t, mixedExchange.UpdatePairs(currency.Pairs{testPerpetualPair, missingPair}, asset.PerpetualContract, true), "UpdatePairs: updating mixed enabled pairs must not error")
	require.ErrorIs(t, mixedExchange.UpdateTickers(t.Context(), asset.PerpetualContract), errAssetContextNotFound, "UpdateTickers must report the missing context for mixed ticker update")
	price, err = ticker.GetTicker(mixedExchange.Name, testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetTicker: getting a valid ticker from a partial batch must not error")
	assert.Equal(t, 100.5, price.Last, "price.Last: partial batch should retain valid ticker updates")
}

func TestUpdateSpotTickers(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"spotMetaAndAssetCtxs": spotContextsJSON})
	setTestPair(t, ex, asset.Spot, testSpotPair, "@107")
	require.NoError(t, ex.UpdateTickers(t.Context(), asset.Spot), "UpdateTickers must not error for spot tickers")
	price, err := ticker.GetTicker(ex.Name, testSpotPair, asset.Spot)
	require.NoError(t, err, "GetTicker: getting the updated spot ticker must not error")
	assert.Equal(t, 9.5, price.Last, "price.Last: spot ticker should use the current midpoint")
	assert.Equal(t, 10.0, price.Volume, "price.Volume: spot ticker should include base volume")
	assert.Equal(t, 100.0, price.QuoteVolume, "price.QuoteVolume: spot ticker should include quote volume")
	assert.Equal(t, 9.0, price.Open, "price.Open: spot ticker should include the previous-day price")
	assert.Zero(t, price.Close, "price.Close: spot ticker should not label a midpoint or mark price as the close")
	assert.Equal(t, 10.0, price.MarkPrice, "price.MarkPrice: spot mark price should be populated")
	assert.WithinDuration(t, time.Now().UTC(), price.LastUpdated, time.Second, "price.LastUpdated: spot ticker update time should be current")

	fallbackJSON := `[` + spotMetadataJSON + `,[{"prevDayPx":"9","dayNtlVlm":"100","markPx":"10","midPx":"0","coin":"@107","dayBaseVlm":"10"}]]`
	fallbackExchange := newStaticInfoExchange(t, map[string]string{"spotMetaAndAssetCtxs": fallbackJSON})
	setTestPair(t, fallbackExchange, asset.Spot, testSpotPair, "@107")
	require.NoError(t, fallbackExchange.UpdateTickers(t.Context(), asset.Spot), "UpdateTickers must not error for a spot ticker without a midpoint")
	price, err = ticker.GetTicker(fallbackExchange.Name, testSpotPair, asset.Spot)
	require.NoError(t, err, "GetTicker: getting the fallback spot ticker must not error")
	assert.Equal(t, 10.0, price.Last, "price.Last: spot ticker should fall back to the mark price when the midpoint is unavailable")

	positionalJSON := `[` + spotMetadataJSON + `,[{"prevDayPx":"9","dayNtlVlm":"100","markPx":"10","midPx":"9.5","dayBaseVlm":"10"}]]`
	positionalExchange := newStaticInfoExchange(t, map[string]string{"spotMetaAndAssetCtxs": positionalJSON})
	setTestPair(t, positionalExchange, asset.Spot, testSpotPair, "@107")
	require.NoError(t, positionalExchange.UpdateTickers(t.Context(), asset.Spot), "UpdateTickers must not error for aligned positional spot contexts")
	price, err = ticker.GetTicker(positionalExchange.Name, testSpotPair, asset.Spot)
	require.NoError(t, err, "GetTicker: getting the positional spot ticker must not error")
	assert.Equal(t, 9.5, price.Last, "price.Last: aligned positional spot context should populate the midpoint")

	mixedContextJSON := `[{"universe":[{"name":"@1"},{"name":"@2"}],"tokens":[]},[{"coin":"@2"},{}]]`
	mixedContextExchange := newStaticInfoExchange(t, map[string]string{"spotMetaAndAssetCtxs": mixedContextJSON})
	require.ErrorIs(t, mixedContextExchange.UpdateTickers(t.Context(), asset.Spot), errAssetContextNotFound, "UpdateTickers must fail closed for mixed explicit and positional spot contexts")

	unmappedContextJSON := `[{"universe":[],"tokens":[]},[{"markPx":"10"}]]`
	unmappedContextExchange := newStaticInfoExchange(t, map[string]string{"spotMetaAndAssetCtxs": unmappedContextJSON})
	require.ErrorIs(t, unmappedContextExchange.UpdateTickers(t.Context(), asset.Spot), errAssetContextNotFound, "UpdateTickers must return the expected error for unidentified spot context")

	missingExchange := newStaticInfoExchange(t, map[string]string{"spotMetaAndAssetCtxs": spotContextsJSON})
	setTestPair(t, missingExchange, asset.Spot, testSpotPair, "@missing")
	require.ErrorIs(t, missingExchange.UpdateTickers(t.Context(), asset.Spot), errAssetContextNotFound, "UpdateTickers must return the expected error for missing spot context")

	processExchange := newStaticInfoExchange(t, map[string]string{"spotMetaAndAssetCtxs": spotContextsJSON})
	setTestPair(t, processExchange, asset.Spot, testSpotPair, "@107")
	processExchange.Name = ""
	require.ErrorIs(t, processExchange.UpdateTickers(t.Context(), asset.Spot), common.ErrExchangeNameNotSet, "UpdateTickers must return its processing error for invalid spot ticker")

	mappingExchange := newStaticInfoExchange(t, map[string]string{
		"spotMetaAndAssetCtxs": spotContextsJSON,
		"spotMeta":             `{"universe":[],"tokens":[]}`,
	})
	setTestPair(t, mappingExchange, asset.Spot, testSpotPair, "@107")
	mappingExchange.setPairMappings(asset.Spot, nil)
	require.ErrorIs(t, mappingExchange.UpdateTickers(t.Context(), asset.Spot), errPairMappingNotFound, "UpdateTickers must return the expected error for missing spot pair mapping")
}

func TestUpdateTickersErrors(t *testing.T) {
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	require.ErrorIs(t, ex.UpdateTickers(t.Context(), asset.Options), asset.ErrNotSupported, "UpdateTickers must return the expected error for unsupported ticker asset")
	require.ErrorIs(t, new(Exchange).UpdateTickers(t.Context(), asset.Spot), asset.ErrNotSupported, "UpdateTickers must return the expected error for unconfigured ticker asset")
	require.NoError(t, ex.CurrencyPairs.SetAssetEnabled(asset.Spot, false), "SetAssetEnabled must not error for the spot asset")
	require.Error(t, ex.UpdateTickers(t.Context(), asset.Spot), "UpdateTickers must error for tickers for a disabled asset")
	require.NoError(t, ex.CurrencyPairs.SetAssetEnabled(asset.Spot, true), "SetAssetEnabled must not error for re-enabling the spot asset")
	setTestPair(t, ex, asset.Spot, testSpotPair, "@107")
	require.Error(t, ex.UpdateTickers(t.Context(), asset.Spot), "UpdateTickers must error for spot tickers from a failing server")
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")
	require.Error(t, ex.UpdateTickers(t.Context(), asset.PerpetualContract), "UpdateTickers must error for perpetual tickers from a failing server")
}

func TestUpdateTicker(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"spotMetaAndAssetCtxs": spotContextsJSON})
	setTestPair(t, ex, asset.Spot, testSpotPair, "@107")
	price, err := ex.UpdateTicker(t.Context(), testSpotPair, asset.Spot)
	require.NoError(t, err, "UpdateTicker must not error for one ticker")
	assert.Equal(t, 10.0, price.MarkPrice, "price.MarkPrice: updated ticker should be returned from the cache")
	assert.Equal(t, 9.5, price.Last, "price.Last: updated ticker should include the current midpoint")
	_, err = ex.UpdateTicker(t.Context(), testPerpetualPair, asset.Spot)
	require.ErrorIs(t, err, ticker.ErrTickerNotFound, "UpdateTicker must return the cache lookup error for a non-enabled pair")
	_, err = ex.UpdateTicker(t.Context(), testSpotPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateTicker must return the expected error for one unsupported ticker")
}

func TestUpdateOrderbook(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"l2Book": bookJSON})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")
	book, err := ex.UpdateOrderbook(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "UpdateOrderbook must not error for an L2 orderbook")
	require.Len(t, book.Bids, 1, "book.Bids: updated orderbook must contain one bid")
	require.Len(t, book.Asks, 1, "book.Asks: updated orderbook must contain one ask")
	assert.Equal(t, 100.0, book.Bids[0].Price, "book.Bids[0].Price: bid price should be converted")
	assert.Equal(t, 2.0, book.Bids[0].Amount, "book.Bids[0].Amount: bid amount should be converted")
	assert.Equal(t, 101.0, book.Asks[0].Price, "book.Asks[0].Price: ask price should be converted")
	assert.Equal(t, 3.0, book.Asks[0].Amount, "book.Asks[0].Amount: ask amount should be converted")

	_, err = ex.UpdateOrderbook(t.Context(), testPerpetualPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateOrderbook must return the expected error for an unsupported orderbook")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestPair(t, errorExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = errorExchange.UpdateOrderbook(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.Error(t, err, "UpdateOrderbook must error for an orderbook from a failing server")

	invalidBook := `{"coin":"BTC","levels":[[{"px":"100","sz":"-2","n":1}],[{"px":"101","sz":"3","n":2}]],"time":1700000000000}`
	processExchange := newStaticInfoExchange(t, map[string]string{"l2Book": invalidBook})
	setTestPair(t, processExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = processExchange.UpdateOrderbook(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.Error(t, err, "UpdateOrderbook must error for an invalid orderbook")
}

func TestGetRecentTrades(t *testing.T) {
	response := `[{"coin":"BTC","side":"B","px":"101","sz":"3","time":1700000001000,"tid":8},{"coin":"BTC","side":"A","px":"100","sz":"2","time":1700000000000,"tid":7}]`
	ex := newStaticInfoExchange(t, map[string]string{"recentTrades": response})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")
	trades, err := ex.GetRecentTrades(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetRecentTrades must not error for converted recent trades")
	require.Len(t, trades, 2, "trades: converted recent trades must contain both results")
	assert.Equal(t, order.Sell, trades[0].Side, "trades[0].Side: ask-side trade should convert to sell")
	assert.Equal(t, order.Buy, trades[1].Side, "trades[1].Side: bid-side trade should convert to buy")
	assert.Equal(t, "7", trades[0].TID, "trades[0].TID: recent trades should be sorted chronologically")
	assert.Equal(t, "8", trades[1].TID, "trades[1].TID: trade ID should be converted to text")

	_, err = ex.GetRecentTrades(t.Context(), testPerpetualPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetRecentTrades must return the expected error for trades for an unsupported asset")

	invalidExchange := newStaticInfoExchange(t, map[string]string{"recentTrades": `[{"coin":"BTC","side":"X","px":"100","sz":"2","time":1700000000000,"tid":7}]`})
	setTestPair(t, invalidExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = invalidExchange.GetRecentTrades(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, order.ErrSideIsInvalid, "GetRecentTrades must return the expected error for invalid trade side")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestPair(t, errorExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = errorExchange.GetRecentTrades(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.Error(t, err, "GetRecentTrades must error for trades from a failing server")
}

func TestGetHistoricCandles(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	end := start.Add(2 * time.Minute)
	response := fmt.Sprintf(`[{"t":%d,"T":%d,"s":"BTC","i":"1m","o":"90","c":"91","h":"92","l":"89","v":"1","n":1},{"t":%d,"T":%d,"s":"BTC","i":"1m","o":"100","c":"101","h":"102","l":"99","v":"5","n":3},{"t":%d,"T":%d,"s":"BTC","i":"1m","o":"110","c":"111","h":"112","l":"109","v":"2","n":1}]`,
		start.Add(-time.Minute).UnixMilli(), start.Add(-time.Millisecond).UnixMilli(),
		start.UnixMilli(), start.Add(time.Minute-time.Millisecond).UnixMilli(),
		end.Add(time.Minute).UnixMilli(), end.Add(2*time.Minute-time.Millisecond).UnixMilli())
	ex := newStaticInfoExchange(t, map[string]string{"candleSnapshot": response})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")
	item, err := ex.GetHistoricCandles(t.Context(), testPerpetualPair, asset.PerpetualContract, kline.OneMin, start, end)
	require.NoError(t, err, "GetHistoricCandles must not error for historic candles")
	require.NotEmpty(t, item.Candles, "item.Candles: historic candle result must contain data")
	assert.Equal(t, start, item.Candles[0].Time, "item.Candles[0].Time: candles before the requested range should be filtered")
	assert.Equal(t, 100.0, item.Candles[0].Open, "item.Candles[0].Open: candle open should be converted")
	_, err = ex.GetHistoricCandles(t.Context(), testPerpetualPair, asset.PerpetualContract, kline.OneMin, start.Add(-(maximumCandleCount+1)*time.Minute), start)
	require.ErrorIs(t, err, kline.ErrRequestExceedsExchangeLimits, "GetHistoricCandles must fail closed for candle ranges outside Hyperliquid retention")

	_, err = ex.GetHistoricCandles(t.Context(), currency.EMPTYPAIR, asset.PerpetualContract, kline.OneMin, start, end)
	require.Error(t, err, "GetHistoricCandles must error for candles with an empty pair")
	_, err = ex.GetHistoricCandles(t.Context(), testPerpetualPair, asset.Options, kline.OneMin, start, end)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetHistoricCandles must return the expected error for candles for an unsupported asset")

	emptyExchange := newStaticInfoExchange(t, map[string]string{"candleSnapshot": `[]`})
	setTestPair(t, emptyExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = emptyExchange.GetHistoricCandles(t.Context(), testPerpetualPair, asset.PerpetualContract, kline.OneMin, start, end)
	require.ErrorIs(t, err, kline.ErrNoTimeSeriesDataToConvert, "GetHistoricCandles must return the expected error for empty candle response")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestPair(t, errorExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = errorExchange.GetHistoricCandles(t.Context(), testPerpetualPair, asset.PerpetualContract, kline.OneMin, start, end)
	require.Error(t, err, "GetHistoricCandles must error for candles from a failing server")

	mappingExchange := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[]}`})
	setTestPair(t, mappingExchange, asset.PerpetualContract, testPerpetualPair, "BTC")
	mappingExchange.setPairMappings(asset.PerpetualContract, nil)
	_, err = mappingExchange.GetHistoricCandles(t.Context(), testPerpetualPair, asset.PerpetualContract, kline.OneMin, start, end)
	require.ErrorIs(t, err, errPairMappingNotFound, "GetHistoricCandles must return the expected error for missing candle pair mapping")
}

func TestGetFeeByType(t *testing.T) {
	feesJSON := `{"userCrossRate":"0.0003","userAddRate":"0.0001","userSpotCrossRate":"0.0005","userSpotAddRate":"0.0002"}`
	ex := newStaticInfoExchange(t, map[string]string{"userFees": feesJSON})
	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")

	_, err := ex.GetFeeByType(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetFeeByType must return the expected error for nil fee builder")
	_, err = ex.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.BankFee, Pair: testPerpetualPair})
	require.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetFeeByType must return the expected error for unsupported fee type")
	_, err = ex.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetFeeByType must return the expected error for empty fee pair")

	for _, tc := range []struct {
		name   string
		price  float64
		amount float64
	}{
		{name: "negative price", price: -1, amount: 1},
		{name: "negative amount", price: 1, amount: -1},
		{name: "nan price", price: math.NaN(), amount: 1},
		{name: "nan amount", price: 1, amount: math.NaN()},
		{name: "infinite price", price: math.Inf(1), amount: 1},
		{name: "infinite amount", price: 1, amount: math.Inf(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.GetFeeByType(t.Context(), &exchange.FeeBuilder{
				FeeType:       exchange.CryptocurrencyTradeFee,
				Pair:          testPerpetualPair,
				PurchasePrice: tc.price,
				Amount:        tc.amount,
			})
			require.ErrorIs(t, err, order.ErrAmountIsInvalid, "GetFeeByType must return the expected error for invalid fee notional")
		})
	}

	taker, err := ex.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.CryptocurrencyTradeFee,
		Pair:          testPerpetualPair,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err, "GetFeeByType must not error for perpetual taker fee")
	assert.InDelta(t, 0.06, taker, 1e-12, "taker: perpetual taker fee should use the effective account rate")
	maker, err := ex.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.OfflineTradeFee,
		Pair:          testPerpetualPair,
		IsMaker:       true,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err, "GetFeeByType must not error for perpetual maker fee")
	assert.InDelta(t, 0.03, maker, 1e-12, "maker: offline perpetual maker fee should use the published base rate")

	spot := newStaticInfoExchange(t, map[string]string{"userFees": feesJSON})
	setTestCredentials(spot, &accounts.Credentials{Key: officialSigningAddress})
	setTestPair(t, spot, asset.Spot, testSpotPair, "@107")
	spotFee, err := spot.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.CryptocurrencyTradeFee,
		Pair:          testSpotPair,
		IsMaker:       true,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err, "GetFeeByType must not error for spot maker fee")
	assert.InDelta(t, 0.04, spotFee, 1e-12, "spotFee: spot maker fee should use the effective account rate")

	missing := newStaticInfoExchange(t, nil)
	setTestCredentials(missing, &accounts.Credentials{Key: officialSigningAddress})
	_, err = missing.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, Pair: testPerpetualPair})
	require.ErrorIs(t, err, errPairMappingNotFound, "GetFeeByType must return the expected error for unmapped fee pair")

	ambiguous := newStaticInfoExchange(t, nil)
	setTestCredentials(ambiguous, &accounts.Credentials{Key: officialSigningAddress})
	setTestPair(t, ambiguous, asset.Spot, testPerpetualPair, "@1")
	setTestPair(t, ambiguous, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = ambiguous.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, Pair: testPerpetualPair})
	require.ErrorIs(t, err, errAmbiguousCoinMapping, "GetFeeByType must fail closed for asset-ambiguous fee pair")

	dualListedSpot := newStaticInfoExchange(t, nil)
	setTestPair(t, dualListedSpot, asset.Spot, testPerpetualPair, "@1")
	dualListedSpot.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: testPerpetualPair, coin: "BTC"}})
	dualListedFee, err := dualListedSpot.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.OfflineTradeFee,
		Pair:          testPerpetualPair,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err, "GetFeeByType must not be ambiguous for spot-enabled dual-listed fee pair")
	assert.InDelta(t, 0.14, dualListedFee, 1e-12, "dualListedFee: spot-enabled dual-listed fee pair should use the spot taker rate")

	dualListedPerpetual := newStaticInfoExchange(t, nil)
	setTestPair(t, dualListedPerpetual, asset.PerpetualContract, testPerpetualPair, "BTC")
	dualListedPerpetual.setPairMappings(asset.Spot, []pairMapping{{pair: testPerpetualPair, coin: "@1"}})
	dualListedFee, err = dualListedPerpetual.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.OfflineTradeFee,
		Pair:          testPerpetualPair,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err, "GetFeeByType must not be ambiguous for perpetual-enabled dual-listed fee pair")
	assert.InDelta(t, 0.09, dualListedFee, 1e-12, "dualListedFee: perpetual-enabled dual-listed fee pair should use the perpetual taker rate")

	offlinePerpetual := newStaticInfoExchange(t, nil)
	setTestPair(t, offlinePerpetual, asset.PerpetualContract, testPerpetualPair, "BTC")
	offlineFee, err := offlinePerpetual.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.OfflineTradeFee,
		Pair:          testPerpetualPair,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err, "GetFeeByType must not require credentials or network access for offline perpetual fee")
	assert.InDelta(t, 0.09, offlineFee, 1e-12, "offlineFee: offline perpetual taker fee should use the published base rate")

	credentiallessBuilder := &exchange.FeeBuilder{
		FeeType:       exchange.CryptocurrencyTradeFee,
		Pair:          testPerpetualPair,
		PurchasePrice: 100,
		Amount:        2,
	}
	offlineFee, err = offlinePerpetual.GetFeeByType(t.Context(), credentiallessBuilder)
	require.NoError(t, err, "GetFeeByType must downgrade to an offline estimate for credentialless cryptocurrency fee")
	assert.Equal(t, exchange.OfflineTradeFee, credentiallessBuilder.FeeType, "credentiallessBuilder.FeeType: credentialless fee request should be classified as offline")
	assert.InDelta(t, 0.09, offlineFee, 1e-12, "offlineFee: credentialless perpetual taker fee should use the published base rate")

	offlineSpot := newStaticInfoExchange(t, nil)
	setTestPair(t, offlineSpot, asset.Spot, testSpotPair, "@107")
	offlineFee, err = offlineSpot.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.OfflineTradeFee,
		Pair:          testSpotPair,
		IsMaker:       true,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err, "GetFeeByType must not require credentials or network access for offline spot fee")
	assert.InDelta(t, 0.08, offlineFee, 1e-12, "offlineFee: offline spot maker fee should use the published base rate")
	offlineFee, err = offlineSpot.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.OfflineTradeFee,
		Pair:          testSpotPair,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err, "GetFeeByType must not require credentials or network access for offline spot taker fee")
	assert.InDelta(t, 0.14, offlineFee, 1e-12, "offlineFee: offline spot taker fee should use the published base rate")

	for _, tc := range []struct {
		name     string
		asset    asset.Item
		pair     currency.Pair
		expected float64
	}{
		{name: "perpetual", asset: asset.PerpetualContract, pair: testPerpetualPair, expected: 0.09},
		{name: "spot", asset: asset.Spot, pair: testSpotPair, expected: 0.14},
	} {
		t.Run("cold configured "+tc.name, func(t *testing.T) {
			cold := newStaticInfoExchange(t, nil)
			require.NoError(t, cold.UpdatePairs(currency.Pairs{tc.pair}, tc.asset, false), "UpdatePairs: updating cold available fee pair must not error")
			require.NoError(t, cold.UpdatePairs(currency.Pairs{tc.pair}, tc.asset, true), "UpdatePairs: updating cold enabled fee pair must not error")
			fee, err := cold.GetFeeByType(t.Context(), &exchange.FeeBuilder{
				FeeType:       exchange.OfflineTradeFee,
				Pair:          tc.pair,
				PurchasePrice: 100,
				Amount:        2,
			})
			require.NoError(t, err, "GetFeeByType must not require discovered API mappings for cold configured offline fee")
			assert.InDelta(t, tc.expected, fee, 1e-12, "fee: cold configured offline fee should use its asset base rate")
		})
	}

	invalidAddress := newStaticInfoExchange(t, nil)
	setTestCredentials(invalidAddress, &accounts.Credentials{Key: "not-an-address", Secret: officialSigningTestKey})
	setTestPair(t, invalidAddress, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = invalidAddress.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		FeeType:       exchange.CryptocurrencyTradeFee,
		Pair:          testPerpetualPair,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.ErrorIs(t, err, errInvalidAddress, "GetFeeByType must fail closed for online fee lookup with an invalid account address")

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failed, &accounts.Credentials{Key: officialSigningAddress})
	setTestPair(t, failed, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = failed.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, Pair: testPerpetualPair})
	require.Error(t, err, "GetFeeByType must return fee lookup HTTP failure")
}

func TestGetLeverage(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		"activeAssetData": `{"user":"` + officialSigningAddress + `","coin":"BTC","leverage":{"type":"cross","value":20}}`,
	})
	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")

	_, err := ex.GetLeverage(t.Context(), asset.Spot, testSpotPair, margin.Unset, order.UnknownSide)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetLeverage must return the expected error for spot leverage")
	_, err = ex.GetLeverage(t.Context(), asset.PerpetualContract, currency.EMPTYPAIR, margin.Unset, order.UnknownSide)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetLeverage must return the expected error for empty leverage pair")
	value, err := ex.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Unset, order.UnknownSide)
	require.NoError(t, err, "GetLeverage must not error for cross leverage without a mode filter")
	assert.Equal(t, 20.0, value, "value: cross leverage value should be returned")
	value, err = ex.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Multi, order.Buy)
	require.NoError(t, err, "GetLeverage must not error for cross leverage with a cross filter")
	assert.Equal(t, 20.0, value, "value: filtered cross leverage value should be returned")
	_, err = ex.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Isolated, order.UnknownSide)
	require.ErrorIs(t, err, margin.ErrMarginTypeUnsupported, "GetLeverage must fail closed for cross leverage queried as isolated")

	isolated := newStaticInfoExchange(t, map[string]string{
		"activeAssetData": `{"leverage":{"type":"isolated","value":5}}`,
	})
	setTestCredentials(isolated, &accounts.Credentials{Key: officialSigningAddress})
	setTestPair(t, isolated, asset.PerpetualContract, testPerpetualPair, "BTC")
	value, err = isolated.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Isolated, order.UnknownSide)
	require.NoError(t, err, "GetLeverage must not error for isolated leverage with an isolated filter")
	assert.Equal(t, 5.0, value, "value: isolated leverage value should be returned")
	_, err = isolated.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Multi, order.UnknownSide)
	require.ErrorIs(t, err, margin.ErrMarginTypeUnsupported, "GetLeverage must fail closed for isolated leverage queried as cross")

	for _, raw := range []string{
		`{"leverage":{"type":"portfolio","value":5}}`,
		`{"leverage":{"type":"cross","value":0}}`,
	} {
		invalid := newStaticInfoExchange(t, map[string]string{"activeAssetData": raw})
		setTestCredentials(invalid, &accounts.Credentials{Key: officialSigningAddress})
		setTestPair(t, invalid, asset.PerpetualContract, testPerpetualPair, "BTC")
		_, err = invalid.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Unset, order.UnknownSide)
		require.Error(t, err, "GetLeverage must error for invalid leverage response")
	}

	missingMapping := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[]}`})
	setTestCredentials(missingMapping, &accounts.Credentials{Key: officialSigningAddress})
	_, err = missingMapping.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Unset, order.UnknownSide)
	require.ErrorIs(t, err, errPairMappingNotFound, "GetLeverage must return the expected error for missing leverage pair mapping")
	missingAddress := newStaticInfoExchange(t, nil)
	setTestPair(t, missingAddress, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = missingAddress.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Unset, order.UnknownSide)
	require.Error(t, err, "GetLeverage must error for leverage lookup without an account address")
	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failed, &accounts.Credentials{Key: officialSigningAddress})
	setTestPair(t, failed, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = failed.GetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Unset, order.UnknownSide)
	require.Error(t, err, "GetLeverage must return leverage HTTP failure")
}

func TestGetLatestFundingRates(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContextsJSON})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err := ex.GetLatestFundingRates(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetLatestFundingRates must return the expected error for nil latest funding request")
	_, err = ex.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.Spot})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetLatestFundingRates must return the expected error for spot latest funding request")
	_, err = ex.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract, IncludePredictedRate: true})
	require.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetLatestFundingRates must return the expected error for predicted funding request")

	rates, err := ex.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract, Pair: testPerpetualPair})
	require.NoError(t, err, "GetLatestFundingRates must not error for one latest funding rate")
	require.Len(t, rates, 1, "GetLatestFundingRates must return one requested market")
	assert.Equal(t, "0.0001", rates[0].LatestRate.Rate.String(), "rates[0].LatestRate.Rate: latest funding rate should be converted exactly")
	assert.Equal(t, testPerpetualPair, rates[0].Pair, "rates[0].Pair: latest funding pair should be returned")
	assert.Equal(t, time.Hour, rates[0].TimeOfNextRate.Sub(rates[0].LatestRate.Time), "Sub: latest funding window should be hourly")
	rates, err = ex.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.NoError(t, err, "GetLatestFundingRates must not error for all latest funding rates")
	require.Len(t, rates, 1, "GetLatestFundingRates must include the configured market")

	hip3Pair := currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.USDC)
	var requestedDEXes []string
	hip3 := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for scoped funding request") {
			return
		}
		requestedDEXes = append(requestedDEXes, request.DEX)
		response := perpetualContextsJSON
		if request.DEX == "xyz" {
			response = `[{"universe":[{"name":"xyz:XYZ100"}]},[{"funding":"0.0002"}]]`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Write should not error for scoped funding response")
	}))
	hip3.setPairMappings(asset.PerpetualContract, []pairMapping{
		{pair: testPerpetualPair, coin: "BTC"},
		{pair: hip3Pair, coin: "xyz:XYZ100", dex: "xyz", assetID: 110000},
	})
	rates, err = hip3.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.NoError(t, err, "GetLatestFundingRates must not error for default and HIP-3 latest funding rates")
	require.Len(t, rates, 2, "GetLatestFundingRates must include both scoped DEXes")
	assert.Equal(t, []string{"", "xyz"}, requestedDEXes, "requestedDEXes: latest funding should query each required DEX once")
	assert.Equal(t, "0.0002", rates[1].LatestRate.Rate.String(), "rates[1].LatestRate.Rate: HIP-3 funding should use its scoped context")

	cold := newStaticInfoExchange(t, map[string]string{
		"meta":             perpetualMetadataJSON,
		"metaAndAssetCtxs": perpetualContextsJSON,
	})
	rates, err = cold.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.NoError(t, err, "GetLatestFundingRates must discover active markets for cold latest funding lookup")
	require.Len(t, rates, 1, "rates: cold latest funding lookup must include the discovered market")

	empty := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[],"collateralToken":0}`})
	_, err = empty.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.ErrorIs(t, err, fundingrate.ErrNoFundingRatesFound, "GetLatestFundingRates must return the expected funding error for no active markets")
	length := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": `[` + perpetualMetadataJSON + `,[]]`})
	setTestPair(t, length, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = length.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.ErrorIs(t, err, errUnexpectedResponseLength, "GetLatestFundingRates must return the expected error for mismatched funding contexts")
	missing := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContextsJSON})
	setTestPair(t, missing, asset.PerpetualContract, testPerpetualPair, "ETH")
	_, err = missing.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.ErrorIs(t, err, errAssetContextNotFound, "GetLatestFundingRates must return the expected error for missing latest funding context")
	missingMapping := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[]}`})
	_, err = missingMapping.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract, Pair: testPerpetualPair})
	require.ErrorIs(t, err, errPairMappingNotFound, "GetLatestFundingRates must return the expected error for missing latest funding mapping")
	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestPair(t, failed, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = failed.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.Error(t, err, "GetLatestFundingRates must return latest funding HTTP failure")
	coldFailure := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = coldFailure.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.Error(t, err, "GetLatestFundingRates must return cold latest funding discovery failure")
}

func TestGetHistoricalFundingRates(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Hour).Add(-600 * time.Hour)
	end := start.Add(501 * time.Hour)
	var requests atomic.Int64
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for historical funding request") {
			return
		}
		page := requests.Add(1)
		var response strings.Builder
		response.WriteByte('[')
		count := 1
		firstTime := start.Add(500 * time.Hour)
		if page == 1 {
			count = maximumFundingHistoryCount
			firstTime = start
		}
		for i := range count {
			if i != 0 {
				response.WriteByte(',')
			}
			_, _ = fmt.Fprintf(&response, `{"coin":"BTC","fundingRate":"0.0001","premium":"0","time":%d}`, firstTime.Add(time.Duration(i)*time.Hour).UnixMilli())
		}
		response.WriteByte(']')
		_, err := w.Write([]byte(response.String()))
		assert.NoError(t, err, "Write should not error for historical funding response")
	}))
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")

	_, err := ex.GetHistoricalFundingRates(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetHistoricalFundingRates must return the expected error for nil historical funding request")
	for _, tc := range []struct {
		name       string
		request    fundingrate.HistoricalRatesRequest
		expectedIs error
	}{
		{name: "unsupported asset", request: fundingrate.HistoricalRatesRequest{Asset: asset.Spot}, expectedIs: asset.ErrNotSupported},
		{name: "empty pair", request: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract}, expectedIs: currency.ErrCurrencyPairEmpty},
		{name: "predicted", request: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: testPerpetualPair, IncludePredictedRate: true}, expectedIs: common.ErrFunctionNotSupported},
		{name: "payments", request: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: testPerpetualPair, IncludePayments: true}, expectedIs: common.ErrFunctionNotSupported},
		{name: "unsupported payment currency", request: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: testPerpetualPair, PaymentCurrency: currency.BTC, StartDate: start, EndDate: end}, expectedIs: asset.ErrNotSupported},
		{name: "invalid range", request: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: testPerpetualPair, StartDate: end, EndDate: start}, expectedIs: common.ErrStartAfterEnd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.GetHistoricalFundingRates(t.Context(), &tc.request)
			require.ErrorIs(t, err, tc.expectedIs, "GetHistoricalFundingRates must return the expected error for invalid historical funding request")
		})
	}

	result, err := ex.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.PerpetualContract,
		Pair:            testPerpetualPair,
		PaymentCurrency: currency.USDC,
		StartDate:       start,
		EndDate:         end,
	})
	require.NoError(t, err, "GetHistoricalFundingRates must not error for paginated historical funding")
	require.Len(t, result.FundingRates, 501, "result.FundingRates: historical funding must include both pages")
	assert.Equal(t, int64(2), requests.Load(), "requests: historical funding should request two pages")
	assert.Equal(t, result.FundingRates[500], result.LatestRate, "result.LatestRate: historical funding should expose its latest rate")
	assert.Equal(t, currency.USDC, result.PaymentCurrency, "result.PaymentCurrency: historical funding should report USDC payments")

	hip3Pair := currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.NewCode("HYPE"))
	hip3 := newStaticInfoExchange(t, map[string]string{
		"fundingHistory": `[{"coin":"xyz:XYZ100","fundingRate":"0.0002","time":` + strconv.FormatInt(start.UnixMilli(), 10) + `}]`,
	})
	setTestPair(t, hip3, asset.PerpetualContract, hip3Pair, "xyz:XYZ100")
	hip3Result, err := hip3.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.PerpetualContract,
		Pair:            hip3Pair,
		PaymentCurrency: currency.NewCode("HYPE"),
		StartDate:       start,
		EndDate:         end,
	})
	require.NoError(t, err, "GetHistoricalFundingRates must not error for non-USDC HIP-3 historical funding")
	assert.Equal(t, currency.NewCode("HYPE"), hip3Result.PaymentCurrency, "hip3Result.PaymentCurrency: HIP-3 funding should report its DEX collateral currency")
	_, err = hip3.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.PerpetualContract,
		Pair:            hip3Pair,
		PaymentCurrency: currency.USDC,
		StartDate:       start,
		EndDate:         end,
	})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetHistoricalFundingRates must fail closed for mismatched HIP-3 funding currency")

	empty := newStaticInfoExchange(t, map[string]string{"fundingHistory": `[]`})
	setTestPair(t, empty, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = empty.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset: asset.PerpetualContract, Pair: testPerpetualPair, StartDate: start, EndDate: end,
	})
	require.ErrorIs(t, err, fundingrate.ErrNoFundingRatesFound, "GetHistoricalFundingRates must return the expected error for empty historical funding")
	record := `{"coin":"BTC","fundingRate":"0.0001","premium":"0","time":` + strconv.FormatInt(start.UnixMilli(), 10) + `}`
	oversized := newStaticInfoExchange(t, map[string]string{
		"fundingHistory": `[` + strings.Repeat(record+",", maximumFundingHistoryCount) + record + `]`,
	})
	setTestPair(t, oversized, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = oversized.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset: asset.PerpetualContract, Pair: testPerpetualPair, StartDate: start, EndDate: end,
	})
	require.ErrorIs(t, err, errUnexpectedResponseLength, "GetHistoricalFundingRates must fail closed for oversized historical funding page")
	malformed := newStaticInfoExchange(t, map[string]string{
		"fundingHistory": `[{"coin":"ETH","fundingRate":"0.1","time":` + strconv.FormatInt(start.UnixMilli(), 10) + `}]`,
	})
	setTestPair(t, malformed, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = malformed.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset: asset.PerpetualContract, Pair: testPerpetualPair, StartDate: start, EndDate: end,
	})
	require.ErrorIs(t, err, errUnexpectedResponseLength, "GetHistoricalFundingRates must fail closed for mismatched historical funding coin")
	missingMapping := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[]}`})
	_, err = missingMapping.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset: asset.PerpetualContract, Pair: testPerpetualPair, StartDate: start, EndDate: end,
	})
	require.ErrorIs(t, err, errPairMappingNotFound, "GetHistoricalFundingRates must return the expected error for missing historical funding mapping")
	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestPair(t, failed, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = failed.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset: asset.PerpetualContract, Pair: testPerpetualPair, StartDate: start, EndDate: end,
	})
	require.Error(t, err, "GetHistoricalFundingRates must return historical funding HTTP failure")
}

func TestGetOpenInterest(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContextsJSON})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")
	requested := key.PairAsset{Base: testPerpetualPair.Base.Item, Quote: testPerpetualPair.Quote.Item, Asset: asset.PerpetualContract}
	result, err := ex.GetOpenInterest(t.Context(), requested)
	require.NoError(t, err, "GetOpenInterest must not error for requested open interest")
	require.Len(t, result, 1, "GetOpenInterest must contain one requested market")
	assert.Equal(t, 10.0, result[0].OpenInterest, "result[0].OpenInterest should be converted")
	assert.True(t, result[0].Key.MatchesPairAsset(testPerpetualPair, asset.PerpetualContract), "MatchesPairAsset: open-interest key should identify the market")
	result, err = ex.GetOpenInterest(t.Context())
	require.NoError(t, err, "GetOpenInterest must not error for all open interest")
	require.Len(t, result, 1, "GetOpenInterest must include the configured market")

	hip3Pair := currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.USDC)
	var requestedDEXes []string
	hip3 := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for scoped open-interest request") {
			return
		}
		requestedDEXes = append(requestedDEXes, request.DEX)
		response := perpetualContextsJSON
		if request.DEX == "xyz" {
			response = `[{"universe":[{"name":"xyz:XYZ100"}]},[{"openInterest":"20"}]]`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Write should not error for scoped open-interest response")
	}))
	hip3.setPairMappings(asset.PerpetualContract, []pairMapping{
		{pair: testPerpetualPair, coin: "BTC"},
		{pair: hip3Pair, coin: "xyz:XYZ100", dex: "xyz", assetID: 110000},
	})
	result, err = hip3.GetOpenInterest(t.Context())
	require.NoError(t, err, "GetOpenInterest must not error for default and HIP-3 open interest")
	require.Len(t, result, 2, "GetOpenInterest must include both scoped DEXes")
	assert.Equal(t, []string{"", "xyz"}, requestedDEXes, "requestedDEXes: open interest should query each required DEX once")
	assert.Equal(t, 20.0, result[1].OpenInterest, "result[1].OpenInterest: HIP-3 open interest should use its scoped context")

	_, err = ex.GetOpenInterest(t.Context(), key.PairAsset{Base: testSpotPair.Base.Item, Quote: testSpotPair.Quote.Item, Asset: asset.Spot})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetOpenInterest must return the expected error for spot open interest")
	missingMapping := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[]}`})
	_, err = missingMapping.GetOpenInterest(t.Context(), requested)
	require.ErrorIs(t, err, errPairMappingNotFound, "GetOpenInterest must return the expected error for missing open-interest mapping")
	length := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": `[` + perpetualMetadataJSON + `,[]]`})
	setTestPair(t, length, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = length.GetOpenInterest(t.Context())
	require.ErrorIs(t, err, errUnexpectedResponseLength, "GetOpenInterest must return the expected error for mismatched open-interest contexts")
	missing := newStaticInfoExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContextsJSON})
	setTestPair(t, missing, asset.PerpetualContract, testPerpetualPair, "ETH")
	_, err = missing.GetOpenInterest(t.Context())
	require.ErrorIs(t, err, errAssetContextNotFound, "GetOpenInterest must return the expected error for missing open-interest context")
	cold := newStaticInfoExchange(t, map[string]string{
		"meta":             perpetualMetadataJSON,
		"metaAndAssetCtxs": perpetualContextsJSON,
	})
	result, err = cold.GetOpenInterest(t.Context())
	require.NoError(t, err, "GetOpenInterest must discover active markets for cold open-interest lookup")
	require.Len(t, result, 1, "result: cold open-interest lookup must include the discovered market")
	empty := newStaticInfoExchange(t, map[string]string{"meta": `{"universe":[],"collateralToken":0}`})
	result, err = empty.GetOpenInterest(t.Context())
	require.NoError(t, err, "GetOpenInterest must not error for open interest without active markets")
	assert.Empty(t, result, "result: open interest without active markets should be empty")
	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestPair(t, failed, asset.PerpetualContract, testPerpetualPair, "BTC")
	_, err = failed.GetOpenInterest(t.Context())
	require.Error(t, err, "GetOpenInterest must return open-interest HTTP failure")
	coldFailure := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = coldFailure.GetOpenInterest(t.Context())
	require.Error(t, err, "GetOpenInterest must return cold open-interest discovery failure")
}

func TestGetUserNonFundingLedgerUpdatesPaginated(t *testing.T) {
	start := time.UnixMilli(1700000000000).UTC()
	end := start.Add(time.Minute)
	var calls atomic.Int32
	var requestedStarts []int64
	success := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for paginated ledger request") {
			return
		}
		requestedStarts = append(requestedStarts, request.StartTime)
		call := calls.Add(1)
		count := maximumUserLedgerHistoryCount
		if call > 1 {
			count = 1
		}
		updates := make([]map[string]any, count)
		for i := range updates {
			updates[i] = map[string]any{
				"time": request.StartTime + int64(i),
				"hash": fmt.Sprintf("0x%d-%d", call, i),
				"delta": map[string]any{
					"type": "deposit",
					"usdc": "1",
				},
			}
		}
		body, err := json.Marshal(updates)
		if !assert.NoError(t, err, "Marshal should not error for paginated ledger response") {
			return
		}
		_, err = w.Write(body)
		assert.NoError(t, err, "Write should not error for paginated ledger response")
	}))
	result, err := success.getUserNonFundingLedgerUpdatesPaginated(t.Context(), officialSigningAddress, start, end)
	require.NoError(t, err, "getUserNonFundingLedgerUpdatesPaginated must not error for multiple ledger pages")
	assert.Len(t, result, maximumUserLedgerHistoryCount+1, "result: every ledger page should be retained")
	assert.Equal(t, int32(2), calls.Load(), "calls: a full ledger page should advance the cursor")
	require.Len(t, requestedStarts, 2, "requestedStarts: ledger fixture must receive two requests")
	assert.Equal(t, start.Add((maximumUserLedgerHistoryCount-1)*time.Millisecond).UnixMilli(), requestedStarts[1], "requestedStarts[1]: ledger pagination should reuse the inclusive terminal timestamp")

	var duplicateCalls atomic.Int32
	exactDuplicate := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for duplicate ledger request") {
			return
		}
		call := duplicateCalls.Add(1)
		count := maximumUserLedgerHistoryCount
		if call > 1 {
			count = 1
		}
		updates := make([]map[string]any, count)
		for i := range updates {
			recordTime := start.Add(time.Duration(i) * time.Millisecond).UnixMilli()
			hash := fmt.Sprintf("0x%d", i)
			if call > 1 {
				recordTime = request.StartTime
				hash = fmt.Sprintf("0x%d", maximumUserLedgerHistoryCount-1)
			}
			updates[i] = map[string]any{
				"time":  recordTime,
				"hash":  hash,
				"delta": map[string]any{"type": "deposit", "usdc": "1"},
			}
		}
		body, err := json.Marshal(updates)
		if !assert.NoError(t, err, "Marshal should not error for duplicate ledger response") {
			return
		}
		_, err = w.Write(body)
		assert.NoError(t, err, "Write should not error for duplicate ledger response")
	}))
	result, err = exactDuplicate.getUserNonFundingLedgerUpdatesPaginated(t.Context(), officialSigningAddress, start, end)
	require.NoError(t, err, "getUserNonFundingLedgerUpdatesPaginated must not error for overlapping exact terminal ledger record")
	assert.Len(t, result, maximumUserLedgerHistoryCount, "result: overlapping exact terminal ledger record should be deduplicated")
	assert.Equal(t, int32(2), duplicateCalls.Load(), "duplicateCalls: exact-terminal deduplication should still request the next inclusive page")

	failing := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = failing.getUserNonFundingLedgerUpdatesPaginated(t.Context(), officialSigningAddress, start, end)
	require.Error(t, err, "getUserNonFundingLedgerUpdatesPaginated must return ledger page request failure")

	for _, tc := range []struct {
		name  string
		times []int64
		count int
	}{
		{name: "oversized page", count: maximumUserLedgerHistoryCount + 1},
		{name: "before cursor", times: []int64{start.Add(-time.Millisecond).UnixMilli()}},
		{name: "after end", times: []int64{end.Add(time.Millisecond).UnixMilli()}},
		{name: "decreasing", times: []int64{start.Add(2 * time.Millisecond).UnixMilli(), start.Add(time.Millisecond).UnixMilli()}},
		{name: "cursor does not advance", count: maximumUserLedgerHistoryCount, times: []int64{start.UnixMilli()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				count := tc.count
				if count == 0 {
					count = len(tc.times)
				}
				updates := make([]map[string]any, count)
				for i := range updates {
					recordTime := start.UnixMilli()
					if len(tc.times) > 1 {
						recordTime = tc.times[i]
					} else if len(tc.times) == 1 {
						recordTime = tc.times[0]
					}
					updates[i] = map[string]any{
						"time":  recordTime,
						"hash":  fmt.Sprintf("0x%d", i),
						"delta": map[string]any{"type": "deposit", "usdc": "1"},
					}
				}
				body, err := json.Marshal(updates)
				if !assert.NoError(t, err, "Marshal should not error for invalid ledger fixture") {
					return
				}
				_, err = w.Write(body)
				assert.NoError(t, err, "Write should not error for invalid ledger fixture")
			}))
			_, err := invalid.getUserNonFundingLedgerUpdatesPaginated(t.Context(), officialSigningAddress, start, end)
			require.ErrorIs(t, err, errUnexpectedResponseLength, "getUserNonFundingLedgerUpdatesPaginated must return the expected error for malformed ledger page")
		})
	}
}

func TestConvertUserLedgerUpdate(t *testing.T) {
	ex := new(Exchange)
	ex.Name = "HyperliquidTest"
	_, err := ex.convertUserLedgerUpdate(nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "convertUserLedgerUpdate must return the expected error for nil ledger update")
	_, err = ex.convertUserLedgerUpdate(&UserLedgerUpdate{})
	require.ErrorIs(t, err, errUnexpectedResponseLength, "convertUserLedgerUpdate must return the expected error for empty ledger type")

	recordTime := time.UnixMilli(1700000000000).UTC()
	for _, tc := range []struct {
		name                string
		delta               UserLedgerDelta
		expectedCurrency    string
		expectedAmount      float64
		expectedDescription string
	}{
		{
			name:                "deposit",
			delta:               UserLedgerDelta{Type: "deposit", USDC: 10},
			expectedCurrency:    "USDC",
			expectedAmount:      10,
			expectedDescription: "deposit",
		},
		{
			name:                "spot transfer",
			delta:               UserLedgerDelta{Type: "spotTransfer", Token: "HYPE:0x96", Amount: 2},
			expectedCurrency:    "HYPE",
			expectedAmount:      2,
			expectedDescription: "spotTransfer",
		},
		{
			name:                "spot genesis without token",
			delta:               UserLedgerDelta{Type: "spotGenesis", Amount: 3},
			expectedCurrency:    "USDC",
			expectedAmount:      3,
			expectedDescription: "spotGenesis",
		},
		{
			name: "send asset",
			delta: UserLedgerDelta{
				Type: "send", Token: "USDC", Amount: 4, SourceDEX: "spot", DestinationDEX: "xyz",
			},
			expectedCurrency:    "USDC",
			expectedAmount:      4,
			expectedDescription: "send: spot to xyz",
		},
		{
			name:                "rewards claim",
			delta:               UserLedgerDelta{Type: "rewardsClaim", Amount: 5},
			expectedCurrency:    "USDC",
			expectedAmount:      5,
			expectedDescription: "rewardsClaim",
		},
		{
			name:                "vault withdrawal",
			delta:               UserLedgerDelta{Type: "vaultWithdraw", NetWithdrawnUSD: 6},
			expectedCurrency:    "USDC",
			expectedAmount:      6,
			expectedDescription: "vaultWithdraw",
		},
		{
			name:                "perpetual to spot",
			delta:               UserLedgerDelta{Type: "accountClassTransfer", USDC: 7},
			expectedCurrency:    "USDC",
			expectedAmount:      7,
			expectedDescription: "perpetual to spot",
		},
		{
			name:                "spot to perpetual",
			delta:               UserLedgerDelta{Type: "accountClassTransfer", USDC: 8, ToPerp: true},
			expectedCurrency:    "USDC",
			expectedAmount:      8,
			expectedDescription: "spot to perpetual",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.delta.Fee = 0.5
			tc.delta.User = officialSigningAddress
			tc.delta.Destination = testOtherAddress
			result, err := ex.convertUserLedgerUpdate(&UserLedgerUpdate{
				Delta: tc.delta,
				Hash:  "0xhash",
				Time:  types.Time(recordTime),
			})
			require.NoError(t, err, "convertUserLedgerUpdate must not error for a valid ledger update")
			assert.Equal(t, "HyperliquidTest", result.ExchangeName, "result.ExchangeName should be retained")
			assert.Equal(t, "processed", result.Status, "result.Status: ledger status should identify a processed L1 update")
			assert.Equal(t, tc.expectedCurrency, result.Currency, "result.Currency: ledger currency should match")
			assert.Equal(t, tc.expectedAmount, result.Amount, "result.Amount: ledger amount should match")
			assert.Equal(t, 0.5, result.Fee, "result.Fee: ledger fee should match")
			assert.Equal(t, tc.expectedDescription, result.Description, "result.Description: ledger description should match")
			assert.Equal(t, officialSigningAddress, result.CryptoFromAddress, "result.CryptoFromAddress: ledger source should match")
			assert.Equal(t, testOtherAddress, result.CryptoToAddress, "result.CryptoToAddress: ledger destination should match")
			assert.Equal(t, "0xhash", result.TransferID, "result.TransferID: ledger transfer ID should use the L1 hash")
			assert.Equal(t, recordTime, result.Timestamp, "result.Timestamp: ledger timestamp should match")
		})
	}
}

func TestGetAccountFundingHistory(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, err := ex.GetAccountFundingHistory(t.Context())
	require.Error(t, err, "GetAccountFundingHistory must error for account history without credentials")

	failing := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failing, &accounts.Credentials{Key: officialSigningAddress})
	_, err = failing.GetAccountFundingHistory(t.Context())
	require.Error(t, err, "GetAccountFundingHistory must return account ledger request failure")

	invalid := newTransferTestExchange(t, &accounts.Credentials{Key: officialSigningAddress}, map[string]string{
		"userNonFundingLedgerUpdates": `[{"time":1700000000000,"hash":"0x1","delta":{"type":""}}]`,
	}, nil)
	_, err = invalid.GetAccountFundingHistory(t.Context())
	require.ErrorIs(t, err, errUnexpectedResponseLength, "GetAccountFundingHistory must return the expected error for malformed account ledger update")

	success := newTransferTestExchange(t, &accounts.Credentials{Key: officialSigningAddress}, map[string]string{
		"userNonFundingLedgerUpdates": `[
			{"time":1700000000001,"hash":"0x1","delta":{"type":"deposit","usdc":"1"}},
			{"time":1700000000002,"hash":"0x2","delta":{"type":"internalTransfer","usdc":"2","user":"` + officialSigningAddress + `","destination":"` + testOtherAddress + `"}}
		]`,
	}, nil)
	result, err := success.GetAccountFundingHistory(t.Context())
	require.NoError(t, err, "GetAccountFundingHistory must not error for account ledger history")
	require.Len(t, result, 2, "GetAccountFundingHistory must convert every account ledger update")
	assert.Equal(t, "0x1", result[0].TransferID, "result[0].TransferID: account ledger history should be sorted oldest first")
	assert.Equal(t, "0x2", result[1].TransferID, "result[1].TransferID: later account ledger update should sort last")
}

func TestGetWithdrawalsHistory(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	result, err := ex.GetWithdrawalsHistory(t.Context(), currency.BTC, asset.PerpetualContract)
	require.NoError(t, err, "GetWithdrawalsHistory must not error for filtering withdrawal history by an unsupported currency")
	assert.Empty(t, result, "GetWithdrawalsHistory should return no records for an unsupported currency")
	_, err = ex.GetWithdrawalsHistory(t.Context(), currency.USDC, asset.Spot)
	require.Error(t, err, "GetWithdrawalsHistory must error for withdrawal history for an asset-agnostic USDC filter without credentials")
	_, err = ex.GetWithdrawalsHistory(t.Context(), currency.USDC, asset.PerpetualContract)
	require.Error(t, err, "GetWithdrawalsHistory must error for withdrawal history without credentials")

	failing := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failing, &accounts.Credentials{Key: officialSigningAddress})
	_, err = failing.GetWithdrawalsHistory(t.Context(), currency.USDC, asset.PerpetualContract)
	require.Error(t, err, "GetWithdrawalsHistory must return withdrawal ledger request failure")

	success := newTransferTestExchange(t, &accounts.Credentials{Key: officialSigningAddress}, map[string]string{
		"userNonFundingLedgerUpdates": `[
			{"time":1700000000000,"hash":"0xdeposit","delta":{"type":"deposit","usdc":"3"}},
			{"time":1700000000001,"hash":"0xwithdraw","delta":{"type":"withdraw","usdc":"-2","fee":"-1","nonce":7}}
		]`,
	}, nil)
	success.Config.UseSandbox = true
	result, err = success.GetWithdrawalsHistory(t.Context(), currency.EMPTYCODE, asset.Empty)
	require.NoError(t, err, "GetWithdrawalsHistory must not error for bridge withdrawal history")
	require.Len(t, result, 1, "GetWithdrawalsHistory must return only bridge withdrawals")
	assert.Equal(t, "0xwithdraw", result[0].TransferID, "result[0].TransferID: withdrawal ID should use the L1 hash")
	assert.Equal(t, 2.0, result[0].Amount, "result[0].Amount: withdrawal amount should be normalised positive")
	assert.Equal(t, 1.0, result[0].Fee, "result[0].Fee: withdrawal fee should be normalised positive")
	assert.Equal(t, "USDC", result[0].Currency, "result[0].Currency: withdrawal currency should be USDC")
	assert.Equal(t, "Arbitrum Sepolia", result[0].CryptoChain, "result[0].CryptoChain: sandbox withdrawal history should identify Arbitrum Sepolia")
}

func TestGetAvailableTransferChains(t *testing.T) {
	mainnet := new(Exchange)
	mainnet.Config = new(config.Exchange)
	_, err := mainnet.GetAvailableTransferChains(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "GetAvailableTransferChains must return the expected error for empty transfer currency")
	_, err = mainnet.GetAvailableTransferChains(t.Context(), currency.BTC)
	require.ErrorIs(t, err, errTransferCurrencyInvalid, "GetAvailableTransferChains must return the expected error for unsupported transfer currency")
	chains, err := mainnet.GetAvailableTransferChains(t.Context(), currency.USDC)
	require.NoError(t, err, "GetAvailableTransferChains must not error for mainnet transfer chains")
	assert.Equal(t, []string{"Arbitrum"}, chains, "chains: mainnet transfer chain should be Arbitrum")

	sandbox := new(Exchange)
	sandbox.Config = &config.Exchange{UseSandbox: true}
	chains, err = sandbox.GetAvailableTransferChains(t.Context(), currency.USDC)
	require.NoError(t, err, "GetAvailableTransferChains must not error for sandbox transfer chains")
	assert.Equal(t, []string{"Arbitrum Sepolia"}, chains, "chains: sandbox transfer chain should be Arbitrum Sepolia")
}

func TestWithdrawCryptocurrencyFunds(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, err := ex.WithdrawCryptocurrencyFunds(t.Context(), nil)
	require.ErrorIs(t, err, withdraw.ErrRequestCannotBeNil, "WithdrawCryptocurrencyFunds must return the expected error for nil withdrawal")

	request := withdraw.Request{
		Exchange: "Hyperliquid",
		Currency: currency.USDC,
		Amount:   2,
		Type:     withdraw.Crypto,
		Crypto:   withdraw.CryptoRequest{Address: testOtherAddress},
	}
	invalidCurrency := request
	invalidCurrency.Currency = currency.BTC
	_, err = ex.WithdrawCryptocurrencyFunds(t.Context(), &invalidCurrency)
	require.ErrorIs(t, err, errTransferCurrencyInvalid, "WithdrawCryptocurrencyFunds must return the expected error for unsupported withdrawal currency")
	addressTag := request
	addressTag.Crypto.AddressTag = "tag"
	_, err = ex.WithdrawCryptocurrencyFunds(t.Context(), &addressTag)
	require.ErrorIs(t, err, errWithdrawalAddressTag, "WithdrawCryptocurrencyFunds must reject withdrawal address tag")
	fee := request
	fee.Crypto.FeeAmount = 1
	_, err = ex.WithdrawCryptocurrencyFunds(t.Context(), &fee)
	require.ErrorIs(t, err, errWithdrawalFeeInput, "WithdrawCryptocurrencyFunds must reject caller-supplied withdrawal fee")
	invalidChain := request
	invalidChain.Crypto.Chain = "Ethereum"
	_, err = ex.WithdrawCryptocurrencyFunds(t.Context(), &invalidChain)
	require.ErrorIs(t, err, errBridgeChainInvalid, "WithdrawCryptocurrencyFunds must reject wrong bridge chain")

	_, err = ex.WithdrawCryptocurrencyFunds(t.Context(), &request)
	require.Error(t, err, "WithdrawCryptocurrencyFunds must error for withdrawal without credentials")

	var captured signedActionRequest
	success := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey,
	}, nil, &captured)
	request.Crypto.Chain = "arbitrum"
	response, err := success.WithdrawCryptocurrencyFunds(t.Context(), &request)
	require.NoError(t, err, "WithdrawCryptocurrencyFunds must not error for a bridge withdrawal")
	assert.Equal(t, "Hyperliquid", response.Name, "response.Name: withdrawal response exchange should match")
	assert.NotEmpty(t, response.ID, "response.ID: withdrawal response should include the action nonce")
	assert.Equal(t, "submitted", response.Status, "response.Status: withdrawal response should identify submission")
	assert.Equal(t, "withdraw3", getCapturedAction(t, &captured)["type"], "captured: external withdrawal should use the bridge action")

	request.InternalTransfer = true
	response, err = success.WithdrawCryptocurrencyFunds(t.Context(), &request)
	require.NoError(t, err, "WithdrawCryptocurrencyFunds must not error for an internal USDC send")
	assert.NotEmpty(t, response.ID, "response.ID: internal-transfer response should include the action nonce")
	assert.Equal(t, "usdSend", getCapturedAction(t, &captured)["type"], "captured: internal withdrawal should use a Core USDC send")

	actionFailure := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(actionFailure, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	_, err = actionFailure.WithdrawCryptocurrencyFunds(t.Context(), &request)
	require.Error(t, err, "WithdrawCryptocurrencyFunds must return signed withdrawal action failure")
}

func TestUnsupportedMethods(t *testing.T) {
	ex := new(Exchange)
	ctx := t.Context()
	assertUnsupported := func(err error) {
		t.Helper()
		require.ErrorIs(t, err, common.ErrFunctionNotSupported, "err: public-only method must return the expected unsupported error")
	}

	_, err := ex.GetHistoricTrades(ctx, testSpotPair, asset.Spot, time.Time{}, time.Time{})
	assertUnsupported(err)
	_, err = ex.GetServerTime(ctx, asset.Spot)
	assertUnsupported(err)
	_, err = ex.GetDepositAddress(ctx, currency.USDC, "", "")
	assertUnsupported(err)
	_, err = ex.WithdrawFiatFunds(ctx, nil)
	assertUnsupported(err)
	_, err = ex.WithdrawFiatFundsToInternationalBank(ctx, nil)
	assertUnsupported(err)
	_, err = ex.GetHistoricCandlesExtended(ctx, testSpotPair, asset.Spot, kline.OneMin, time.Time{}, time.Time{})
	assertUnsupported(err)
	_, err = ex.GetFuturesContractDetails(ctx, asset.PerpetualContract)
	assertUnsupported(err)
	_, err = ex.GetCurrencyTradeURL(ctx, asset.Spot, testSpotPair)
	assertUnsupported(err)
	require.ErrorIs(t, ex.UpdateOrderExecutionLimits(ctx, asset.Spot), common.ErrNotYetImplemented, "UpdateOrderExecutionLimits must return the expected not-implemented error for execution-limit bootstrap method")
}

func TestUpdateOrderbookUsesValidationSetting(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"l2Book": bookJSON})
	setTestPair(t, ex, asset.PerpetualContract, testPerpetualPair, "BTC")
	ex.Name = "HyperliquidValidationDisabled"
	ex.ValidateOrderbook = false
	book, err := ex.UpdateOrderbook(t.Context(), testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "UpdateOrderbook must not error for an orderbook with validation disabled")
	assert.False(t, book.ValidateOrderbook, "book.ValidateOrderbook: orderbook should inherit the exchange validation setting")

	cached, err := orderbook.Get(ex.Name, testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "Get must not error for the cached orderbook")
	assert.False(t, cached.ValidateOrderbook, "cached.ValidateOrderbook: cached orderbook should retain the exchange validation setting")
}

func TestUnsupportedMethodsIgnoreContext(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	ex := new(Exchange)
	_, err := ex.GetServerTime(cancelled, asset.Spot)
	require.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetServerTime must remain deterministic for a cancelled context for unsupported method")
}
