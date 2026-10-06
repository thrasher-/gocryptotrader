package hyperliquid

import (
	"fmt"
	"maps"
	"math"
	"net/http"
	"slices"
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
	"github.com/thrasher-corp/gocryptotrader/exchanges/futures"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/protocol"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
	"github.com/thrasher-corp/gocryptotrader/types"
	"github.com/thrasher-corp/gocryptotrader/types/decimal"
)

// testWithdrawalAddress receives the mock withdrawals and transfers
const testWithdrawalAddress = "0x0000000000000000000000000000000000000014"

var (
	// dashFormat is the configured pair format, which enabled pairs carry
	dashFormat      = currency.PairFormat{Uppercase: true, Delimiter: currency.DashDelimiter}
	testBuilderPair = currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.USDC)
	// The mappings of the mock responses' BTC, HYPE spot and xyz:XYZ100 markets
	testPerpetualMapping = pairMapping{pair: perpetualPair, coin: "BTC", sizeDecimals: 5, maxLeverage: 40}
	testSpotMapping      = pairMapping{pair: spotPair, coin: "@107", assetID: 10107, sizeDecimals: 2}
	testBuilderMapping   = pairMapping{pair: testBuilderPair, coin: "xyz:XYZ100", dex: "xyz", assetID: 110000, sizeDecimals: 4, maxLeverage: 30}
)

// newInfoServerExchange returns an exchange signing as the mock signing account, whose info requests receive the
// response keyed by their type and DEX as "type:dex", else by their type; the perpetual DEX registry and user roles
// default to only the default DEX and a user, and any other info request fails. Exchange actions receive the response
// actions returns for the decoded action, and fail when it returns nothing
func newInfoServerExchange(t *testing.T, responses map[string]string, actions func(action map[string]any) string) *Exchange {
	t.Helper()
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response string
		switch r.URL.Path {
		case "/info":
			var body InfoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
				return
			}
			var ok bool
			if response, ok = responses[body.Type+":"+body.DEX]; ok {
				break
			}
			if response, ok = responses[body.Type]; ok {
				break
			}
			switch body.Type {
			case "perpDexs":
				response = `[null]`
			case "userRole":
				response = `{"role":"user"}`
			default:
				http.Error(w, "unexpected info request "+body.Type, http.StatusBadRequest)
				return
			}
		case "/exchange":
			var body struct {
				Action map[string]any `json:"action"`
			}
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the exchange request should not error") {
				return
			}
			if actions != nil {
				response = actions(body.Action)
			}
			if response == "" {
				http.Error(w, "unexpected exchange action", http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	ex.Name = t.Name()
	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey})
	return ex
}

// newUnavailableServerExchange returns an exchange signing as the mock signing account whose requests all fail
func newUnavailableServerExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	ex.Name = t.Name()
	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey})
	return ex
}

// clearBalanceTimes checks the times Save stamps on stored balances are current, then clears them so balances compare by
// value
func clearBalanceTimes(t *testing.T, subAccounts accounts.SubAccounts) {
	t.Helper()
	for _, subAccount := range subAccounts {
		for code, balance := range subAccount.Balances {
			assert.WithinDurationf(t, time.Now(), balance.UpdatedAt, time.Minute, "Save should stamp the %s balance with the current time", code)
			balance.UpdatedAt = time.Time{}
			subAccount.Balances[code] = balance
		}
	}
}

// statusesResponse returns an exchange action response holding the statuses, which are JSON values
func statusesResponse(responseType string, statuses ...string) string {
	return `{"status":"ok","response":{"type":"` + responseType + `","data":{"statuses":[` + strings.Join(statuses, ",") + `]}}}`
}

// setTestPairs caches the mappings and makes their pairs the asset's available and enabled pairs
func setTestPairs(t *testing.T, ex *Exchange, a asset.Item, mappings ...pairMapping) {
	t.Helper()
	ex.setPairMappings(a, mappings)
	pairs := make(currency.Pairs, len(mappings))
	for i := range mappings {
		pairs[i] = mappings[i].pair
	}
	require.NoError(t, ex.UpdatePairs(pairs, a, false), "UpdatePairs must not error for available pairs")
	require.NoError(t, ex.UpdatePairs(pairs, a, true), "UpdatePairs must not error for enabled pairs")
}

func TestLogDefaultError(t *testing.T) {
	t.Parallel()
	assert.NotPanics(t, func() { logDefaultError(nil) }, "logDefaultError should not panic for a nil error")
	assert.NotPanics(t, func() { logDefaultError(assert.AnError) }, "logDefaultError should not panic for an error")
}

func TestSetDefaults(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	ex.SetDefaults()
	assert.Equal(t, "Hyperliquid", ex.Name, "SetDefaults should name the exchange")
	assert.True(t, ex.Enabled, "SetDefaults should enable the exchange")
	assert.Equal(t, currency.Currencies{currency.USDC}, ex.BaseCurrencies, "SetDefaults should use USDC as the base currency")
	assert.True(t, ex.API.CredentialsValidator.RequiresKey, "SetDefaults should require the account address as the key")
	for _, a := range []asset.Item{asset.Spot, asset.PerpetualContract} {
		format, err := ex.GetPairFormat(a, true)
		require.NoErrorf(t, err, "GetPairFormat must not error for %s", a)
		assert.Equalf(t, dashFormat, format, "SetDefaults should set the %s request format", a)
	}
	expREST := protocol.Features{
		TickerBatching:                 true,
		TickerFetching:                 true,
		KlineFetching:                  true,
		TradeFetching:                  true,
		OrderbookFetching:              true,
		AutoPairUpdates:                true,
		AccountBalance:                 true,
		CryptoWithdrawal:               true,
		DepositHistory:                 true,
		WithdrawalHistory:              true,
		GetOrder:                       true,
		GetOrders:                      true,
		CancelOrders:                   true,
		CancelOrder:                    true,
		SubmitOrder:                    true,
		ModifyOrder:                    true,
		TradeFee:                       true,
		AuthenticatedEndpoints:         true,
		HasAssetTypeAccountSegregation: true,
	}
	assert.Equal(t, expREST, ex.Features.Supports.RESTCapabilities, "SetDefaults should set the REST capabilities")
	expWebsocket := protocol.Features{
		TickerFetching:         true,
		KlineFetching:          true,
		TradeFetching:          true,
		OrderbookFetching:      true,
		Subscribe:              true,
		Unsubscribe:            true,
		GetOrders:              true,
		AuthenticatedEndpoints: true,
	}
	assert.Equal(t, expWebsocket, ex.Features.Supports.WebsocketCapabilities, "SetDefaults should set the websocket capabilities")
	expFutures := exchange.FuturesCapabilities{
		FundingRates:                    true,
		FundingRateBatching:             map[asset.Item]bool{asset.PerpetualContract: true},
		SupportedFundingRateFrequencies: map[kline.Interval]bool{kline.OneHour: true},
		Leverage:                        true,
		OpenInterest:                    exchange.OpenInterestSupport{Supported: true, SupportsRestBatch: true},
	}
	assert.Equal(t, expFutures, ex.Features.Supports.FuturesCapabilities, "SetDefaults should set the futures capabilities")
	assert.Equal(t, exchange.AutoWithdrawCryptoWithSetup, ex.Features.Supports.WithdrawPermissions, "SetDefaults should require withdrawal setup")
	assert.Equal(t, uint64(maximumCandleCount), ex.Features.Enabled.Kline.GlobalResultLimit, "SetDefaults should limit candles to Hyperliquid's retention")
	for kind, exp := range map[exchange.URL]string{exchange.RestSpot: apiURL, exchange.WebsocketSpot: websocketURL} {
		endpoint, err := ex.API.Endpoints.GetURL(kind)
		require.NoErrorf(t, err, "GetURL must not error for %s", kind)
		assert.Equalf(t, exp, endpoint, "SetDefaults should set the %s URL", kind)
	}
	_, err := ex.API.Endpoints.GetURL(exchange.RestFutures)
	assert.Error(t, err, "SetDefaults should not set a futures URL, as one host serves every market")
	assert.NotNil(t, ex.Requester, "SetDefaults should set the requester")
	assert.NotNil(t, ex.Websocket, "SetDefaults should set the websocket manager")
	assert.NotNil(t, ex.pairMappings, "SetDefaults should initialise the pair mappings")
	assert.NotNil(t, ex.pairMappingMisses, "SetDefaults should initialise the pair mapping misses")
	assert.NotNil(t, ex.websocketPending, "SetDefaults should initialise the pending websocket operations")
}

func TestSetup(t *testing.T) {
	t.Parallel()
	newConfig := func(t *testing.T) (*Exchange, *config.Exchange) {
		t.Helper()
		ex := new(Exchange)
		ex.SetDefaults()
		cfg, err := ex.GetStandardConfig()
		require.NoError(t, err, "GetStandardConfig must not error")
		return ex, cfg
	}

	ex, cfg := newConfig(t)
	require.Error(t, ex.Setup(nil), "Setup must reject a nil configuration")
	cfg.Enabled = false
	require.NoError(t, ex.Setup(cfg), "Setup must not error for a disabled exchange")
	assert.False(t, ex.IsEnabled(), "Setup should leave a disabled exchange disabled")

	ex, cfg = newConfig(t)
	cfg.API.AuthenticatedSupport = true
	cfg.API.Credentials.Key = testAccountAddress
	require.NoError(t, ex.Setup(cfg), "Setup must not error")
	assert.True(t, ex.IsEnabled(), "Setup should enable the exchange")
	credentials, err := ex.GetCredentials(t.Context())
	require.NoError(t, err, "GetCredentials must not error")
	assert.Equal(t, testAccountAddress, credentials.Key, "Setup should load the configured account address")
	assert.True(t, ex.isMainnetEnvironment(), "Setup should sign for mainnet by default")

	for _, tc := range []struct {
		name      string
		endpoints map[string]string
		exp       map[exchange.URL]string
	}{
		{
			name: "default endpoints",
			exp:  map[exchange.URL]string{exchange.RestSpot: testnetAPIURL, exchange.WebsocketSpot: testnetWebsocketURL},
		},
		{
			name:      "default endpoints with trailing slashes",
			endpoints: map[string]string{exchange.RestSpot.String(): apiURL + "/", exchange.WebsocketSpot.String(): websocketURL + "/"},
			exp:       map[exchange.URL]string{exchange.RestSpot: testnetAPIURL, exchange.WebsocketSpot: testnetWebsocketURL},
		},
		{
			name:      "custom gateway",
			endpoints: map[string]string{exchange.RestSpot.String(): "https://hyperliquid.internal.example", exchange.WebsocketSpot.String(): "wss://hyperliquid.internal.example/ws"},
			exp:       map[exchange.URL]string{exchange.RestSpot: "https://hyperliquid.internal.example", exchange.WebsocketSpot: "wss://hyperliquid.internal.example/ws"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sandbox, sandboxConfig := newConfig(t)
			sandboxConfig.UseSandbox = true
			sandboxConfig.API.Endpoints = tc.endpoints
			require.NoError(t, sandbox.Setup(sandboxConfig), "Setup must not error in sandbox mode")
			assert.False(t, sandbox.isMainnetEnvironment(), "Setup should sign for testnet in sandbox mode")
			for kind, exp := range tc.exp {
				endpoint, err := sandbox.API.Endpoints.GetURL(kind)
				require.NoErrorf(t, err, "GetURL must not error for %s", kind)
				assert.Equalf(t, exp, endpoint, "Setup should select the %s URL", kind)
			}
		})
	}

	ex, cfg = newConfig(t)
	cfg.API.Endpoints = map[string]string{exchange.RestSpot.String(): testnetAPIURL, exchange.WebsocketSpot.String(): testnetWebsocketURL}
	require.ErrorIs(t, ex.Setup(cfg), errEndpointEnvironment, "Setup must reject testnet URLs outside sandbox mode")

	ex, cfg = newConfig(t)
	cfg.API.Endpoints = map[string]string{"invalid": "https://example.com"}
	require.Error(t, ex.Setup(cfg), "Setup must reject an unknown endpoint")

	ex, cfg = newConfig(t)
	ex.API.Endpoints = ex.NewEndpoints()
	require.Error(t, ex.Setup(cfg), "Setup must error without endpoints")

	ex, cfg = newConfig(t)
	cfg.WebsocketTrafficTimeout = time.Millisecond
	require.Error(t, ex.Setup(cfg), "Setup must reject an invalid websocket traffic timeout")

	ex, cfg = newConfig(t)
	ex.Websocket.TrafficAlert = nil
	require.Error(t, ex.Setup(cfg), "Setup must error when the websocket connection cannot be set up")
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTradablePairs(t.Context(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "FetchTradablePairs must reject an unsupported asset")
	_, err = new(Exchange).FetchTradablePairs(t.Context(), asset.Spot)
	require.ErrorIs(t, err, asset.ErrNotSupported, "FetchTradablePairs must reject an asset without pair storage")

	pairs, err := e.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "FetchTradablePairs must not error for perpetuals")
	if mockTests {
		exp := currency.Pairs{
			perpetualPair,
			currency.NewPair(currency.ETH, currency.USDC),
			currency.NewPair(currency.NewCode("HPOS"), currency.USDC),
			testBuilderPair,
			currency.NewPair(currency.NewCode("xyz:TSLA"), currency.USDC),
			currency.NewPair(currency.NewCode("flx:TSLA"), currency.NewCode("USDH")),
		}
		assert.Equal(t, exp, pairs, "FetchTradablePairs should list every active market of every perpetual DEX")
	} else {
		assert.Contains(t, pairs, perpetualPair, "FetchTradablePairs should list BTC-USDC")
	}

	pairs, err = e.FetchTradablePairs(t.Context(), asset.Spot)
	require.NoError(t, err, "FetchTradablePairs must not error for spot")
	if mockTests {
		exp := currency.Pairs{currency.NewPair(currency.NewCode("PURR"), currency.USDC), spotPair, currency.NewPair(currency.NewCode("USDH"), currency.USDC)}
		assert.Equal(t, exp, pairs, "FetchTradablePairs should list every spot market")
	} else {
		assert.Contains(t, pairs, spotPair, "FetchTradablePairs should list HYPE-USDC")
	}

	ambiguous := newInfoServerExchange(t, map[string]string{
		"meta":     `{"universe":[{"name":"BTC","szDecimals":5},{"name":"BTC","szDecimals":5},{"name":"ETH","szDecimals":4}]}`,
		"spotMeta": `{"universe":[{"tokens":[150,0],"name":"@107","index":107},{"tokens":[151,0],"name":"@108","index":108},{"tokens":[152,0],"name":"@109","index":109}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"HYPE","index":151},{"name":"PURR","index":152}]}`,
	}, nil)
	pairs, err = ambiguous.FetchTradablePairs(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "FetchTradablePairs must not error for an ambiguous perpetual display pair")
	assert.Equal(t, currency.Pairs{currency.NewPair(currency.ETH, currency.USDC)}, pairs, "FetchTradablePairs should skip an ambiguous perpetual display pair")
	_, ok := ambiguous.lookupPairMapping(perpetualPair, asset.PerpetualContract)
	assert.False(t, ok, "FetchTradablePairs should not map an ambiguous perpetual display pair")
	pairs, err = ambiguous.FetchTradablePairs(t.Context(), asset.Spot)
	require.NoError(t, err, "FetchTradablePairs must not error for an ambiguous spot display pair")
	assert.Equal(t, currency.Pairs{currency.NewPair(currency.NewCode("PURR"), currency.USDC)}, pairs, "FetchTradablePairs should skip an ambiguous spot display pair")
	mapping, ok := ambiguous.lookupPairMapping(pairs[0], asset.Spot)
	require.True(t, ok, "FetchTradablePairs must map an unambiguous spot pair")
	assert.Equal(t, pairMapping{pair: pairs[0], coin: "@109", assetID: 10109}, mapping, "FetchTradablePairs should map the unambiguous spot market")

	failed := newUnavailableServerExchange(t)
	for _, a := range []asset.Item{asset.Spot, asset.PerpetualContract} {
		_, err = failed.FetchTradablePairs(t.Context(), a)
		assert.Errorf(t, err, "FetchTradablePairs should return a %s metadata failure", a)
	}
}

func TestFetchPerpetualPairMappings(t *testing.T) {
	t.Parallel()
	mappings, err := e.fetchPerpetualPairMappings(t.Context())
	require.NoError(t, err, "fetchPerpetualPairMappings must not error")
	if mockTests {
		exp := []pairMapping{
			testPerpetualMapping,
			{pair: currency.NewPair(currency.ETH, currency.USDC), coin: "ETH", assetID: 1, sizeDecimals: 4, maxLeverage: 25},
			{pair: currency.NewPair(currency.NewCode("HPOS"), currency.USDC), coin: "HPOS", assetID: 3, maxLeverage: 3, onlyIsolated: true},
			testBuilderMapping,
			{pair: currency.NewPair(currency.NewCode("xyz:TSLA"), currency.USDC), coin: "xyz:TSLA", dex: "xyz", assetID: 110001, sizeDecimals: 3, maxLeverage: 20, onlyIsolated: true},
			{pair: currency.NewPair(currency.NewCode("flx:TSLA"), currency.NewCode("USDH")), coin: "flx:TSLA", dex: "flx", assetID: 120000, sizeDecimals: 2, maxLeverage: 10},
		}
		assert.Equal(t, exp, mappings, "fetchPerpetualPairMappings should map every active market with its DEX's asset ID offset")
	} else {
		assert.Contains(t, mappings, testPerpetualMapping, "fetchPerpetualPairMappings should map BTC")
	}

	ex := newInfoServerExchange(t, map[string]string{
		"perpDexs": `[null,{"name":"xyz"}]`,
		"meta:":    `{"universe":[{"name":"BAD COIN"},{"name":"ETH","szDecimals":4,"maxLeverage":25}]}`,
		"meta:xyz": `{"universe":[{"name":"xyz:XYZ100","szDecimals":4,"maxLeverage":30},{"name":"xyz:OLD","isDelisted":true}],"collateralToken":150}`,
		"spotMeta": `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150}]}`,
	}, nil)
	mappings, err = ex.fetchPerpetualPairMappings(t.Context())
	require.NoError(t, err, "fetchPerpetualPairMappings must not error for a builder DEX with spot collateral")
	exp := []pairMapping{
		{pair: currency.NewPair(currency.ETH, currency.USDC), coin: "ETH", assetID: 1, sizeDecimals: 4, maxLeverage: 25},
		{pair: currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.HYPE), coin: "xyz:XYZ100", dex: "xyz", assetID: 110000, sizeDecimals: 4, maxLeverage: 30},
	}
	assert.Equal(t, exp, mappings, "fetchPerpetualPairMappings should skip invalid names and quote builder markets in their collateral")

	for _, tc := range []struct {
		name      string
		responses map[string]string
		err       error
	}{
		{name: "missing builder entry", responses: map[string]string{"perpDexs": `[null,null]`}, err: errInvalidPerpetualDEX},
		{name: "blank builder name", responses: map[string]string{"perpDexs": `[null,{"name":" "}]`}, err: errInvalidPerpetualDEX},
		{name: "duplicate builder name", responses: map[string]string{"perpDexs": `[null,{"name":"xyz"},{"name":"xyz"}]`}, err: errInvalidPerpetualDEX},
		{name: "unscoped builder market", responses: map[string]string{"perpDexs": `[null,{"name":"xyz"}]`, "meta": `{"universe":[{"name":"XYZ100"}]}`}, err: errInvalidPerpetualDEX},
		{
			name:      "too many builder markets",
			responses: map[string]string{"perpDexs": `[null,{"name":"xyz"}]`, "meta": `{"universe":[` + strings.Repeat(`{"name":"xyz:X"},`, builderPerpetualDEXAssetStride) + `{"name":"xyz:X"}]}`},
			err:       errInvalidPerpetualDEX,
		},
		{
			name:      "missing collateral token",
			responses: map[string]string{"meta": `{"universe":[],"collateralToken":150}`, "spotMeta": `{"tokens":[{"name":"USDC","index":0}]}`},
			err:       errSpotTokenNotFound,
		},
		{
			name:      "later DEX missing collateral token",
			responses: map[string]string{"perpDexs": `[null,{"name":"xyz"},{"name":"flx"}]`, "meta:": `{"universe":[]}`, "meta:xyz": `{"universe":[],"collateralToken":150}`, "meta:flx": `{"universe":[],"collateralToken":151}`, "spotMeta": `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150}]}`},
			err:       errSpotTokenNotFound,
		},
		{name: "invalid registry", responses: map[string]string{"perpDexs": `[]`}, err: errUnexpectedResponseLength},
		{name: "metadata failure", responses: map[string]string{"meta": `{`}},
		{name: "spot metadata failure", responses: map[string]string{"meta": `{"universe":[],"collateralToken":150}`, "spotMeta": `{`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newInfoServerExchange(t, tc.responses, nil).fetchPerpetualPairMappings(t.Context())
			if tc.err == nil {
				assert.Error(t, err, "fetchPerpetualPairMappings should return a malformed response error")
				return
			}
			assert.ErrorIs(t, err, tc.err, "fetchPerpetualPairMappings should return the expected error")
		})
	}
}

func TestGetSpotTokenNames(t *testing.T) {
	t.Parallel()
	names, err := e.getSpotTokenNames(t.Context())
	require.NoError(t, err, "getSpotTokenNames must not error")
	if mockTests {
		exp := map[uint64]currency.Code{0: currency.USDC, 1: currency.NewCode("PURR"), 150: currency.HYPE, 360: currency.NewCode("USDH")}
		assert.Equal(t, exp, names, "getSpotTokenNames should map every token index to its name")
	} else {
		assert.Equal(t, currency.USDC, names[0], "getSpotTokenNames should map index 0 to USDC")
	}

	for _, tc := range []struct {
		name     string
		spotMeta string
		err      error
	}{
		{name: "duplicate index", spotMeta: `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":0}]}`, err: errUnexpectedResponseLength},
		{name: "blank name", spotMeta: `{"tokens":[{"name":"USDC","index":0},{"name":"","index":150}]}`, err: errSpotTokenNotFound},
		{name: "non-USDC index 0", spotMeta: `{"tokens":[{"name":"USDT","index":0}]}`, err: errSpotTokenNotFound},
	} {
		_, err := newInfoServerExchange(t, map[string]string{"spotMeta": tc.spotMeta}, nil).getSpotTokenNames(t.Context())
		assert.ErrorIsf(t, err, tc.err, "getSpotTokenNames should reject spot metadata with a %s", tc.name)
	}
	_, err = newUnavailableServerExchange(t).getSpotTokenNames(t.Context())
	assert.Error(t, err, "getSpotTokenNames should return a spot metadata failure")
}

func TestFetchSpotPairMappings(t *testing.T) {
	t.Parallel()
	mappings, err := e.fetchSpotPairMappings(t.Context())
	require.NoError(t, err, "fetchSpotPairMappings must not error")
	if mockTests {
		exp := []pairMapping{
			{pair: currency.NewPair(currency.NewCode("PURR"), currency.USDC), coin: "PURR/USDC", assetID: 10000},
			testSpotMapping,
			{pair: currency.NewPair(currency.NewCode("USDH"), currency.USDC), coin: "@230", assetID: 10230, sizeDecimals: 2},
		}
		assert.Equal(t, exp, mappings, "fetchSpotPairMappings should map every spot market")
	} else {
		assert.Contains(t, mappings, testSpotMapping, "fetchSpotPairMappings should map HYPE-USDC")
	}

	purr := pairMapping{pair: currency.NewPair(currency.NewCode("PURR"), currency.USDC), coin: "@2", assetID: 10002}
	for _, tc := range []struct {
		name     string
		spotMeta string
	}{
		{name: "one token", spotMeta: `{"universe":[{"tokens":[1],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"PURR","index":2}]}`},
		{name: "missing base token", spotMeta: `{"universe":[{"tokens":[1,0],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"PURR","index":2}]}`},
		{name: "missing quote token", spotMeta: `{"universe":[{"tokens":[1,9],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"TOKEN","index":1},{"name":"PURR","index":2}]}`},
		{name: "blank token name", spotMeta: `{"universe":[{"tokens":[1,0],"name":"@1","index":1},{"tokens":[2,0],"name":"@2","index":2}],"tokens":[{"name":"USDC","index":0},{"name":"","index":1},{"name":"PURR","index":2}]}`},
	} {
		mappings, err := newInfoServerExchange(t, map[string]string{"spotMeta": tc.spotMeta}, nil).fetchSpotPairMappings(t.Context())
		require.NoErrorf(t, err, "fetchSpotPairMappings must not error for a market with a %s", tc.name)
		assert.Equalf(t, []pairMapping{purr}, mappings, "fetchSpotPairMappings should skip a market with a %s", tc.name)
	}

	for _, tc := range []struct {
		name     string
		spotMeta string
	}{
		{name: "token index", spotMeta: `{"universe":[{"tokens":[150,0],"name":"@107","index":107}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"PURR","index":150}]}`},
		{name: "market index", spotMeta: `{"universe":[{"tokens":[150,0],"name":"@107","index":107},{"tokens":[151,0],"name":"@108","index":107}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"PURR","index":151}]}`},
		{name: "market name", spotMeta: `{"universe":[{"tokens":[150,0],"name":"@107","index":107},{"tokens":[151,0],"name":"@107","index":108}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150},{"name":"PURR","index":151}]}`},
	} {
		_, err := newInfoServerExchange(t, map[string]string{"spotMeta": tc.spotMeta}, nil).fetchSpotPairMappings(t.Context())
		assert.ErrorIsf(t, err, errUnexpectedResponseLength, "fetchSpotPairMappings should reject a duplicate %s", tc.name)
	}
	_, err = newUnavailableServerExchange(t).fetchSpotPairMappings(t.Context())
	assert.Error(t, err, "fetchSpotPairMappings should return a spot metadata failure")
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	require.NoError(t, e.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must not error")
	for a, exp := range map[asset.Item]currency.Pair{asset.PerpetualContract: perpetualPair, asset.Spot: spotPair} {
		pairs, err := e.GetAvailablePairs(a)
		require.NoErrorf(t, err, "GetAvailablePairs must not error for %s", a)
		assert.Truef(t, pairs.Contains(exp, true), "UpdateTradablePairs should make %s available", exp)
	}

	require.Error(t, newUnavailableServerExchange(t).UpdateTradablePairs(t.Context()), "UpdateTradablePairs must return a metadata failure")
	empty := newInfoServerExchange(t, map[string]string{"meta": `{"universe":[]}`, "spotMeta": `{"universe":[],"tokens":[]}`}, nil)
	require.Error(t, empty.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must error when no pair can be enabled")
}

func TestGetPerpetualDEXNames(t *testing.T) {
	t.Parallel()
	names, err := e.getPerpetualDEXNames(t.Context())
	require.NoError(t, err, "getPerpetualDEXNames must not error")
	if mockTests {
		assert.Equal(t, []string{"", "xyz", "flx"}, names, "getPerpetualDEXNames should list every DEX in registry order")
	} else {
		assert.Empty(t, names[0], "getPerpetualDEXNames should list the default DEX first")
	}

	names, err = newInfoServerExchange(t, map[string]string{"perpDexs": `[null,{"name":" xyz "},{"name":"hyna"}]`}, nil).getPerpetualDEXNames(t.Context())
	require.NoError(t, err, "getPerpetualDEXNames must not error for padded names")
	assert.Equal(t, []string{"", "xyz", "hyna"}, names, "getPerpetualDEXNames should trim DEX names")

	for _, tc := range []struct {
		registry string
		err      error
	}{
		{registry: `[]`, err: errUnexpectedResponseLength},
		{registry: `[{"name":"default"}]`, err: errUnexpectedResponseLength},
		{registry: `[null,null]`, err: errInvalidPerpetualDEX},
		{registry: `[null,{"name":" "}]`, err: errInvalidPerpetualDEX},
		{registry: `[null,{"name":"xyz"},{"name":" xyz "}]`, err: errInvalidPerpetualDEX},
	} {
		_, err := newInfoServerExchange(t, map[string]string{"perpDexs": tc.registry}, nil).getPerpetualDEXNames(t.Context())
		assert.ErrorIsf(t, err, tc.err, "getPerpetualDEXNames should reject registry %s", tc.registry)
	}
	_, err = newUnavailableServerExchange(t).getPerpetualDEXNames(t.Context())
	assert.Error(t, err, "getPerpetualDEXNames should return a registry failure")
}

func TestSetPairMappings(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	mappings := []pairMapping{testSpotMapping}
	ex.setPairMappings(asset.Spot, mappings)
	mappings[0].coin = "changed"
	mapping, ok := ex.lookupPairMapping(spotPair, asset.Spot)
	require.True(t, ok, "lookupPairMapping must find the stored mapping")
	assert.Equal(t, testSpotMapping, mapping, "setPairMappings should store a copy of the mappings")
}

func TestGetCoin(t *testing.T) {
	t.Parallel()
	coin, err := e.getCoin(t.Context(), perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "getCoin must not error")
	assert.Equal(t, "BTC", coin, "getCoin should return the market's coin")
	coin, err = e.getCoin(t.Context(), spotPair, asset.Spot)
	require.NoError(t, err, "getCoin must not error for spot")
	assert.Equal(t, "@107", coin, "getCoin should return the spot market's coin")
	_, err = e.getCoin(t.Context(), currency.EMPTYPAIR, asset.Spot)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "getCoin should reject an empty pair")
}

func TestGetPairMapping(t *testing.T) {
	t.Parallel()
	_, err := e.getPairMapping(t.Context(), perpetualPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "getPairMapping must reject an unsupported asset")
	_, err = e.getPairMapping(t.Context(), currency.EMPTYPAIR, asset.PerpetualContract)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "getPairMapping must reject an empty pair")

	mapping, err := e.getPairMapping(t.Context(), testBuilderPair, asset.PerpetualContract)
	require.NoError(t, err, "getPairMapping must not error for a builder DEX market")
	if mockTests {
		assert.Equal(t, testBuilderMapping, mapping, "getPairMapping should fetch the builder DEX market's mapping")
	} else {
		assert.Equal(t, "xyz", mapping.dex, "getPairMapping should return the builder DEX market's DEX")
	}

	ex := new(Exchange)
	ex.SetDefaults()
	ex.setPairMappings(asset.Spot, []pairMapping{testSpotMapping})
	mapping, err = ex.getPairMapping(t.Context(), spotPair, asset.Spot)
	require.NoError(t, err, "getPairMapping must not error for a cached mapping")
	assert.Equal(t, testSpotMapping, mapping, "getPairMapping should return a cached mapping without fetching")
}

func TestLookupPairMapping(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	_, ok := ex.lookupPairMapping(spotPair, asset.Spot)
	assert.False(t, ok, "lookupPairMapping should not find an uncached mapping")
	ex.setPairMappings(asset.Spot, []pairMapping{testSpotMapping})
	mapping, ok := ex.lookupPairMapping(spotPair, asset.Spot)
	require.True(t, ok, "lookupPairMapping must find a cached mapping")
	assert.Equal(t, testSpotMapping, mapping, "lookupPairMapping should return the cached mapping")
	_, ok = ex.lookupPairMapping(spotPair, asset.PerpetualContract)
	assert.False(t, ok, "lookupPairMapping should not find a mapping of another asset")
}

func TestFetchPairMapping(t *testing.T) {
	t.Parallel()
	var fetches atomic.Int32
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body InfoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
			return
		}
		fetches.Add(1)
		response := `{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":40}]}`
		if body.Type == "perpDexs" {
			response = `[null]`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	mapping, err := ex.fetchPairMapping(t.Context(), perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "fetchPairMapping must not error for a mapping cached while waiting")
	assert.Equal(t, testPerpetualMapping, mapping, "fetchPairMapping should return a mapping cached while waiting")
	assert.Zero(t, fetches.Load(), "fetchPairMapping should not fetch a cached mapping")

	ex.setPairMappings(asset.PerpetualContract, nil)
	mapping, err = ex.fetchPairMapping(t.Context(), perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "fetchPairMapping must not error")
	assert.Equal(t, testPerpetualMapping, mapping, "fetchPairMapping should fetch the market's mapping")

	missing := currency.NewPair(currency.ETH, currency.USDC)
	_, err = ex.fetchPairMapping(t.Context(), missing, asset.PerpetualContract)
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMapping must report a missing market")
	fetched := fetches.Load()
	_, err = ex.fetchPairMapping(t.Context(), missing, asset.PerpetualContract)
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMapping must report a cached missing market")
	assert.Equal(t, fetched, fetches.Load(), "fetchPairMapping should not refetch a recently missing market")
	ex.pairMappingMisses["pair:"+asset.PerpetualContract.String()+":"+strings.ToLower(missing.String())] = time.Now().Add(-time.Second)
	_, err = ex.fetchPairMapping(t.Context(), missing, asset.PerpetualContract)
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMapping must report a missing market after its miss expires")
	assert.Greater(t, fetches.Load(), fetched, "fetchPairMapping should refetch once a miss expires")

	_, err = newUnavailableServerExchange(t).fetchPairMapping(t.Context(), perpetualPair, asset.PerpetualContract)
	assert.Error(t, err, "fetchPairMapping should return a metadata failure")
}

func TestGetPairMappingByCoin(t *testing.T) {
	t.Parallel()
	_, _, err := e.getPairMappingByCoin(t.Context(), " ")
	require.ErrorIs(t, err, errCoinRequired, "getPairMappingByCoin must require a coin")

	mapping, a, err := e.getPairMappingByCoin(t.Context(), "@107")
	require.NoError(t, err, "getPairMappingByCoin must not error")
	assert.Equal(t, asset.Spot, a, "getPairMappingByCoin should return the coin's asset")
	if mockTests {
		assert.Equal(t, testSpotMapping, mapping, "getPairMappingByCoin should return the coin's mapping")
	} else {
		assert.Equal(t, spotPair, mapping.pair, "getPairMappingByCoin should return the coin's pair")
	}

	ex := new(Exchange)
	ex.SetDefaults()
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	ex.setPairMappings(asset.Spot, []pairMapping{{pair: spotPair, coin: "BTC"}})
	_, _, err = ex.getPairMappingByCoin(t.Context(), "BTC")
	assert.ErrorIs(t, err, errAmbiguousCoinMapping, "getPairMappingByCoin should reject a cached ambiguous coin without fetching")
}

func TestLookupPairMappingByCoin(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	_, _, err := ex.lookupPairMappingByCoin("BTC")
	require.ErrorIs(t, err, errPairMappingNotFound, "lookupPairMappingByCoin must report an uncached coin")
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	mapping, a, err := ex.lookupPairMappingByCoin("BTC")
	require.NoError(t, err, "lookupPairMappingByCoin must not error")
	assert.Equal(t, testPerpetualMapping, mapping, "lookupPairMappingByCoin should return the coin's mapping")
	assert.Equal(t, asset.PerpetualContract, a, "lookupPairMappingByCoin should return the coin's asset")
	ex.setPairMappings(asset.Spot, []pairMapping{{pair: spotPair, coin: "BTC"}})
	_, _, err = ex.lookupPairMappingByCoin("BTC")
	assert.ErrorIs(t, err, errAmbiguousCoinMapping, "lookupPairMappingByCoin should reject a coin mapped by both assets")
}

func TestFetchPairMappingByCoin(t *testing.T) {
	t.Parallel()
	cached := new(Exchange)
	cached.SetDefaults()
	cached.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	mapping, a, err := cached.fetchPairMappingByCoin(t.Context(), "BTC")
	require.NoError(t, err, "fetchPairMappingByCoin must not error for a coin cached while waiting")
	assert.Equal(t, testPerpetualMapping, mapping, "fetchPairMappingByCoin should return a coin cached while waiting")
	assert.Equal(t, asset.PerpetualContract, a, "fetchPairMappingByCoin should return the cached coin's asset")

	_, _, err = new(Exchange).fetchPairMappingByCoin(t.Context(), "BTC")
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMappingByCoin must report a coin when no asset is supported")

	var fetches atomic.Int32
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body InfoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
			return
		}
		fetches.Add(1)
		var response string
		switch body.Type {
		case "perpDexs":
			response = `[null]`
		case "meta":
			response = `{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":40}]}`
		case "spotMeta":
			response = `{"universe":[{"tokens":[150,0],"name":"@107","index":107}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150,"szDecimals":2}]}`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	mapping, a, err = ex.fetchPairMappingByCoin(t.Context(), "BTC")
	require.NoError(t, err, "fetchPairMappingByCoin must not error")
	assert.Equal(t, testPerpetualMapping, mapping, "fetchPairMappingByCoin should fetch the coin's mapping")
	assert.Equal(t, asset.PerpetualContract, a, "fetchPairMappingByCoin should return the fetched coin's asset")

	_, _, err = ex.fetchPairMappingByCoin(t.Context(), "MISSING")
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMappingByCoin must report a missing coin")
	fetched := fetches.Load()
	_, _, err = ex.fetchPairMappingByCoin(t.Context(), "MISSING")
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMappingByCoin must report a cached missing coin")
	assert.Equal(t, fetched, fetches.Load(), "fetchPairMappingByCoin should not refetch a recently missing coin")
	ex.pairMappingMisses["coin:missing"] = time.Now().Add(-time.Second)
	_, _, err = ex.fetchPairMappingByCoin(t.Context(), "MISSING")
	require.ErrorIs(t, err, errPairMappingNotFound, "fetchPairMappingByCoin must report a missing coin after its miss expires")
	assert.Greater(t, fetches.Load(), fetched, "fetchPairMappingByCoin should refetch once a miss expires")

	_, _, err = newUnavailableServerExchange(t).fetchPairMappingByCoin(t.Context(), "BTC")
	assert.Error(t, err, "fetchPairMappingByCoin should return a metadata failure")
}

func TestCachePairMappingMiss(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	assert.False(t, ex.isCachedPairMappingMiss("pair:spot:missing"), "isCachedPairMappingMiss should report an unknown key as uncached")
	ex.cachePairMappingMiss("pair:spot:missing")
	assert.True(t, ex.isCachedPairMappingMiss("pair:spot:missing"), "isCachedPairMappingMiss should report a recent miss as cached")
	ex.pairMappingMisses["pair:spot:missing"] = time.Now().Add(-time.Second)
	assert.False(t, ex.isCachedPairMappingMiss("pair:spot:missing"), "isCachedPairMappingMiss should report an expired miss as uncached")
	assert.NotContains(t, ex.pairMappingMisses, "pair:spot:missing", "isCachedPairMappingMiss should drop an expired miss")
}

func TestGetPerpetualPairMappings(t *testing.T) {
	t.Parallel()
	mappings, err := e.getPerpetualPairMappings(t.Context())
	require.NoError(t, err, "getPerpetualPairMappings must not error")
	assert.Contains(t, mappings, testPerpetualMapping, "getPerpetualPairMappings should include BTC")

	ex := new(Exchange)
	ex.SetDefaults()
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{testBuilderMapping})
	mappings, err = ex.getPerpetualPairMappings(t.Context())
	require.NoError(t, err, "getPerpetualPairMappings must not error for cached mappings")
	assert.Equal(t, []pairMapping{testBuilderMapping}, mappings, "getPerpetualPairMappings should return cached mappings without fetching")

	_, err = newUnavailableServerExchange(t).getPerpetualPairMappings(t.Context())
	assert.Error(t, err, "getPerpetualPairMappings should return a metadata failure")
}

func TestGetPerpetualAssetContexts(t *testing.T) {
	t.Parallel()
	contexts, err := e.getPerpetualAssetContexts(t.Context(), []pairMapping{testPerpetualMapping, testBuilderMapping})
	require.NoError(t, err, "getPerpetualAssetContexts must not error")
	require.Len(t, contexts, 2, "getPerpetualAssetContexts must return a context per mapping")
	if mockTests {
		exp := []PerpetualAssetContext{
			{Funding: 0.0000125, OpenInterest: 39310.43958, PreviousDayPrice: 86248, DayNotionalVolume: 1820651593.0427696705, Premium: -0.0002691175, OraclePrice: 85836.1, MarkPrice: 85810.5, MidPrice: 85812.5, ImpactPrices: []types.Number{85812, 85813}, DayBaseVolume: 21219.11844},
			{Funding: 0.00000625, OpenInterest: 7148.95, PreviousDayPrice: 30759, DayNotionalVolume: 275526916.1308999658, Premium: 0.0000645887, OraclePrice: 31120, MarkPrice: 31123, MidPrice: 31122.5, ImpactPrices: []types.Number{31121.02, 31123}, DayBaseVolume: 8893.0929},
		}
		assert.Equal(t, exp, contexts, "getPerpetualAssetContexts should return each market's context from its DEX")
	} else {
		assert.Positive(t, contexts[0].MarkPrice.Float64(), "getPerpetualAssetContexts should return BTC's mark price")
	}

	var requested []string
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body InfoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
			return
		}
		requested = append(requested, body.DEX)
		response := `[{"universe":[{"name":"BTC"},{"name":"OLD","isDelisted":true}]},[{"funding":"0.0001"},{"funding":"0.0002"}]]`
		if body.DEX == "xyz" {
			response = `[{"universe":[{"name":"xyz:XYZ100"}]},[{"funding":"0.0003"}]]`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	contexts, err = ex.getPerpetualAssetContexts(t.Context(), []pairMapping{testPerpetualMapping, testBuilderMapping, testPerpetualMapping})
	require.NoError(t, err, "getPerpetualAssetContexts must not error for repeated DEXs")
	assert.Equal(t, []PerpetualAssetContext{{Funding: 0.0001}, {Funding: 0.0003}, {Funding: 0.0001}}, contexts, "getPerpetualAssetContexts should return each market's context")
	assert.Equal(t, []string{"", "xyz"}, requested, "getPerpetualAssetContexts should fetch each DEX once")
	_, err = ex.getPerpetualAssetContexts(t.Context(), []pairMapping{{pair: perpetualPair, coin: "OLD"}})
	assert.ErrorIs(t, err, errAssetContextNotFound, "getPerpetualAssetContexts should not return a delisted market's context")
	_, err = newUnavailableServerExchange(t).getPerpetualAssetContexts(t.Context(), []pairMapping{testPerpetualMapping})
	assert.Error(t, err, "getPerpetualAssetContexts should return a context failure")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.UpdateTickers(t.Context(), asset.Options), asset.ErrNotSupported, "UpdateTickers must reject an unsupported asset")
	require.ErrorIs(t, new(Exchange).UpdateTickers(t.Context(), asset.Spot), asset.ErrNotSupported, "UpdateTickers must reject an asset without pair storage")

	for _, a := range e.GetAssetTypes(false) {
		require.NoErrorf(t, e.UpdateTickers(t.Context(), a), "UpdateTickers must not error for %s", a)
	}
	perpetual, err := ticker.GetTicker(e.Name, perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetTicker must not error for the perpetual ticker")
	spot, err := ticker.GetTicker(e.Name, spotPair, asset.Spot)
	require.NoError(t, err, "GetTicker must not error for the spot ticker")
	if mockTests {
		exp := &ticker.Price{
			Last:         85812.5,
			Open:         86248,
			BaseVolume:   21219.11844,
			QuoteVolume:  1820651593.0427696705,
			OpenInterest: 39310.43958,
			MarkPrice:    85810.5,
			IndexPrice:   85836.1,
			Pair:         perpetualPair.Format(dashFormat),
			ExchangeName: e.Name,
			AssetType:    asset.PerpetualContract,
			LastUpdated:  perpetual.LastUpdated,
		}
		assert.Equal(t, exp, perpetual, "UpdateTickers should store the perpetual ticker from its context")
		exp = &ticker.Price{
			Last:         93.1465,
			Open:         93.102,
			BaseVolume:   677650.0300000001,
			QuoteVolume:  63321924.1126599833,
			MarkPrice:    93.146,
			Pair:         spotPair.Format(dashFormat),
			ExchangeName: e.Name,
			AssetType:    asset.Spot,
			LastUpdated:  spot.LastUpdated,
		}
		assert.Equal(t, exp, spot, "UpdateTickers should store the spot ticker from its context")
	} else {
		assert.Positive(t, perpetual.MarkPrice, "UpdateTickers should store the perpetual mark price")
		assert.Positive(t, spot.MarkPrice, "UpdateTickers should store the spot mark price")
	}
	assert.WithinDuration(t, time.Now(), perpetual.LastUpdated, time.Minute, "UpdateTickers should time the perpetual ticker when it is fetched")

	var requested []string
	builder := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body InfoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
			return
		}
		requested = append(requested, body.DEX)
		response := `[{"universe":[{"name":"BTC"}]},[{"openInterest":"10","prevDayPx":"99","dayNtlVlm":"1000","oraclePx":"100","markPx":"101","midPx":null,"dayBaseVlm":"10"}]]`
		if body.DEX == "xyz" {
			response = `[{"universe":[{"name":"xyz:XYZ100"}]},[{"openInterest":"20","prevDayPx":"49","dayNtlVlm":"2000","oraclePx":"50","markPx":"51","midPx":"50.5","dayBaseVlm":"40"}]]`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	builder.Name = t.Name()
	setTestPairs(t, builder, asset.PerpetualContract, testPerpetualMapping, testBuilderMapping)
	require.NoError(t, builder.UpdateTickers(t.Context(), asset.PerpetualContract), "UpdateTickers must not error across DEXs")
	assert.Equal(t, []string{"", "xyz"}, requested, "UpdateTickers should fetch each DEX's contexts once")
	for _, exp := range []*ticker.Price{
		{Last: 101, Open: 99, BaseVolume: 10, QuoteVolume: 1000, OpenInterest: 10, MarkPrice: 101, IndexPrice: 100, Pair: perpetualPair.Format(dashFormat), ExchangeName: builder.Name, AssetType: asset.PerpetualContract},
		{Last: 50.5, Open: 49, BaseVolume: 40, QuoteVolume: 2000, OpenInterest: 20, MarkPrice: 51, IndexPrice: 50, Pair: testBuilderPair.Format(dashFormat), ExchangeName: builder.Name, AssetType: asset.PerpetualContract},
	} {
		price, err := ticker.GetTicker(builder.Name, exp.Pair, asset.PerpetualContract)
		require.NoErrorf(t, err, "GetTicker must not error for %s", exp.Pair)
		exp.LastUpdated = price.LastUpdated
		assert.Equalf(t, exp, price, "UpdateTickers should store %s, using the mark price without a mid price", exp.Pair)
	}

	spotContexts := `[{"universe":[],"tokens":[]},[{"prevDayPx":"9","dayNtlVlm":"100","markPx":"10","midPx":null,"coin":"@107","dayBaseVlm":"10"},{"coin":"#10","markPx":"0.5"}]]`
	spotExchange := newInfoServerExchange(t, map[string]string{"spotMetaAndAssetCtxs": spotContexts}, nil)
	setTestPairs(t, spotExchange, asset.Spot, testSpotMapping)
	require.NoError(t, spotExchange.UpdateTickers(t.Context(), asset.Spot), "UpdateTickers must not error for spot contexts outside the universe")
	price, err := ticker.GetTicker(spotExchange.Name, spotPair, asset.Spot)
	require.NoError(t, err, "GetTicker must not error for the spot ticker")
	exp := &ticker.Price{Last: 10, Open: 9, BaseVolume: 10, QuoteVolume: 100, MarkPrice: 10, Pair: spotPair.Format(dashFormat), ExchangeName: spotExchange.Name, AssetType: asset.Spot, LastUpdated: price.LastUpdated}
	assert.Equal(t, exp, price, "UpdateTickers should match spot contexts by coin and use the mark price without a mid price")

	perpetualContexts := `[{"universe":[{"name":"BTC"}]},[{"markPx":"101"}]]`
	for _, tc := range []struct {
		name      string
		a         asset.Item
		responses map[string]string
		mappings  []pairMapping
		err       error
	}{
		{name: "mismatched perpetual contexts", a: asset.PerpetualContract, responses: map[string]string{"metaAndAssetCtxs": `[{"universe":[{"name":"BTC"}]},[]]`}, mappings: []pairMapping{testPerpetualMapping}, err: errUnexpectedResponseLength},
		{name: "missing perpetual context", a: asset.PerpetualContract, responses: map[string]string{"metaAndAssetCtxs": perpetualContexts}, mappings: []pairMapping{{pair: perpetualPair, coin: "ETH"}}, err: errAssetContextNotFound},
		{name: "missing spot context", a: asset.Spot, responses: map[string]string{"spotMetaAndAssetCtxs": spotContexts}, mappings: []pairMapping{{pair: spotPair, coin: "@108"}}, err: errAssetContextNotFound},
	} {
		ex := newInfoServerExchange(t, tc.responses, nil)
		setTestPairs(t, ex, tc.a, tc.mappings...)
		assert.ErrorIsf(t, ex.UpdateTickers(t.Context(), tc.a), tc.err, "UpdateTickers should report a %s", tc.name)
	}

	for a, responses := range map[asset.Item]map[string]string{
		asset.PerpetualContract: {"metaAndAssetCtxs": perpetualContexts, "meta": `{"universe":[{"name":"ETH"}]}`},
		asset.Spot:              {"spotMetaAndAssetCtxs": spotContexts, "spotMeta": `{"universe":[],"tokens":[]}`},
	} {
		ex := newInfoServerExchange(t, responses, nil)
		assert.ErrorIsf(t, ex.UpdateTickers(t.Context(), a), errPairMappingNotFound, "UpdateTickers should report a %s pair without a market", a)
	}

	mixed := newInfoServerExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContexts}, nil)
	ethPair := currency.NewPair(currency.ETH, currency.USDC)
	setTestPairs(t, mixed, asset.PerpetualContract, testPerpetualMapping, pairMapping{pair: ethPair, coin: "ETH"})
	assert.ErrorIs(t, mixed.UpdateTickers(t.Context(), asset.PerpetualContract), errAssetContextNotFound, "UpdateTickers should report a missing context in a batch")
	price, err = ticker.GetTicker(mixed.Name, perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetTicker must not error for the batch's valid ticker")
	assert.Equal(t, 101.0, price.Last, "UpdateTickers should store the batch's valid tickers")

	for _, a := range []asset.Item{asset.Spot, asset.PerpetualContract} {
		unnamed := newInfoServerExchange(t, map[string]string{"metaAndAssetCtxs": perpetualContexts, "spotMetaAndAssetCtxs": spotContexts}, nil)
		setTestPairs(t, unnamed, asset.Spot, testSpotMapping)
		setTestPairs(t, unnamed, asset.PerpetualContract, testPerpetualMapping)
		unnamed.Name = ""
		assert.ErrorIsf(t, unnamed.UpdateTickers(t.Context(), a), common.ErrExchangeNameNotSet, "UpdateTickers should return a %s ticker processing error", a)

		failed := newUnavailableServerExchange(t)
		setTestPairs(t, failed, a, map[asset.Item]pairMapping{asset.Spot: testSpotMapping, asset.PerpetualContract: testPerpetualMapping}[a])
		assert.Errorf(t, failed.UpdateTickers(t.Context(), a), "UpdateTickers should return a %s context failure", a)
	}

	disabled := newInfoServerExchange(t, nil, nil)
	require.NoError(t, disabled.CurrencyPairs.SetAssetEnabled(asset.Spot, false), "SetAssetEnabled must not error")
	assert.Error(t, disabled.UpdateTickers(t.Context(), asset.Spot), "UpdateTickers should error for a disabled asset")
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	price, err := e.UpdateTicker(t.Context(), spotPair, asset.Spot)
	require.NoError(t, err, "UpdateTicker must not error")
	if mockTests {
		assert.Equal(t, 93.1465, price.Last, "UpdateTicker should return the updated ticker")
	} else {
		assert.Positive(t, price.Last, "UpdateTicker should return the updated ticker")
	}
	_, err = e.UpdateTicker(t.Context(), currency.NewPair(currency.ETH, currency.USDC), asset.Spot)
	assert.ErrorIs(t, err, ticker.ErrTickerNotFound, "UpdateTicker should not find a ticker for a pair that is not enabled")
	_, err = e.UpdateTicker(t.Context(), spotPair, asset.Options)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "UpdateTicker should reject an unsupported asset")
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateOrderbook(t.Context(), perpetualPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateOrderbook must reject an unsupported asset")

	for _, tc := range []struct {
		pair        currency.Pair
		a           asset.Item
		bids, asks  orderbook.Levels
		lastUpdated int64
	}{
		{
			pair:        perpetualPair,
			a:           asset.PerpetualContract,
			bids:        orderbook.Levels{{Price: 85812, Amount: 0.41043, OrderCount: 4}, {Price: 85811, Amount: 1.62719, OrderCount: 7}},
			asks:        orderbook.Levels{{Price: 85813, Amount: 4.71207, OrderCount: 19}, {Price: 85814, Amount: 0.0035, OrderCount: 1}},
			lastUpdated: 1791275808695,
		},
		{
			pair:        spotPair,
			a:           asset.Spot,
			bids:        orderbook.Levels{{Price: 93.144, Amount: 12.04, OrderCount: 2}},
			asks:        orderbook.Levels{{Price: 93.149, Amount: 48.31, OrderCount: 3}},
			lastUpdated: 1791275812441,
		},
		{
			pair:        testBuilderPair,
			a:           asset.PerpetualContract,
			bids:        orderbook.Levels{{Price: 31122, Amount: 0.215, OrderCount: 3}},
			asks:        orderbook.Levels{{Price: 31123, Amount: 0.8411, OrderCount: 5}},
			lastUpdated: 1791275813002,
		},
	} {
		book, err := e.UpdateOrderbook(t.Context(), tc.pair, tc.a)
		require.NoErrorf(t, err, "UpdateOrderbook must not error for %s %s", tc.a, tc.pair)
		if mockTests {
			assert.Equalf(t, tc.bids, book.Bids, "UpdateOrderbook should convert the %s bids", tc.pair)
			assert.Equalf(t, tc.asks, book.Asks, "UpdateOrderbook should convert the %s asks", tc.pair)
			assert.Equalf(t, time.UnixMilli(tc.lastUpdated).UTC(), book.LastUpdated, "UpdateOrderbook should time the %s book from its snapshot", tc.pair)
		} else {
			assert.NotEmptyf(t, book.Bids, "UpdateOrderbook should return %s bids", tc.pair)
		}
	}

	invalidBook := `{"coin":"BTC","levels":[[{"px":"100","sz":"-2","n":1}],[{"px":"101","sz":"3","n":2}]],"time":1791275808695}`
	ex := newTradingServerExchange(t, map[string]string{"l2Book": invalidBook}, nil)
	_, err = ex.UpdateOrderbook(t.Context(), perpetualPair, asset.PerpetualContract)
	assert.Error(t, err, "UpdateOrderbook should return a validation failure")
	ex.ValidateOrderbook = false
	book, err := ex.UpdateOrderbook(t.Context(), perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "UpdateOrderbook must not error with validation disabled")
	assert.False(t, book.ValidateOrderbook, "UpdateOrderbook should keep the exchange's validation setting")

	_, err = newTradingServerExchange(t, map[string]string{"l2Book": `{"coin":"BTC","levels":[[]],"time":1791275808695}`}, nil).UpdateOrderbook(t.Context(), perpetualPair, asset.PerpetualContract)
	assert.ErrorIs(t, err, errInvalidBookLevelCount, "UpdateOrderbook should reject a book without both sides")
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = failed.UpdateOrderbook(t.Context(), perpetualPair, asset.PerpetualContract)
	assert.Error(t, err, "UpdateOrderbook should return a book failure")
}

func TestConvertL2Book(t *testing.T) {
	t.Parallel()
	_, err := e.convertL2Book(&L2Book{Levels: [][]L2Level{{}}}, perpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, errInvalidBookLevelCount, "convertL2Book must reject a book without both sides")

	book, err := e.convertL2Book(&L2Book{
		Coin:   "BTC",
		Time:   milli(1791275808695),
		Levels: [][]L2Level{{{Price: 100, Size: 2, OrderCount: 3}}, {{Price: 101, Size: 4, OrderCount: 5}}},
	}, perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "convertL2Book must not error")
	exp := &orderbook.Book{
		Exchange:          e.Name,
		Pair:              perpetualPair,
		Asset:             asset.PerpetualContract,
		LastUpdated:       time.UnixMilli(1791275808695).UTC(),
		ValidateOrderbook: e.ValidateOrderbook,
		Bids:              orderbook.Levels{{Price: 100, Amount: 2, OrderCount: 3}},
		Asks:              orderbook.Levels{{Price: 101, Amount: 4, OrderCount: 5}},
	}
	assert.Equal(t, exp, book, "convertL2Book should convert both sides of the book")
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetRecentTrades(t.Context(), perpetualPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetRecentTrades must reject an unsupported asset")

	trades, err := e.GetRecentTrades(t.Context(), perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetRecentTrades must not error")
	if mockTests {
		exp := []trade.Data{
			{TID: "284092582946010", Exchange: e.Name, CurrencyPair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Sell, Price: 85812, Amount: 0.00347, Timestamp: time.UnixMilli(1791275814004).UTC()},
			{TID: "744489298526402", Exchange: e.Name, CurrencyPair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Buy, Price: 85813, Amount: 0.01757, Timestamp: time.UnixMilli(1791275814806).UTC()},
		}
		assert.Equal(t, exp, trades, "GetRecentTrades should convert every trade in time order")
	} else {
		assert.NotEmpty(t, trades, "GetRecentTrades should return trades")
	}

	unordered := newInfoServerExchange(t, map[string]string{"recentTrades": `[{"coin":"BTC","side":"B","px":"101","sz":"3","time":1791275814806,"tid":8},{"coin":"BTC","side":"A","px":"100","sz":"2","time":1791275814004,"tid":7}]`}, nil)
	unordered.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	trades, err = unordered.GetRecentTrades(t.Context(), perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetRecentTrades must not error for unordered trades")
	require.Len(t, trades, 2, "GetRecentTrades must return every trade")
	assert.Equal(t, "7", trades[0].TID, "GetRecentTrades should sort trades oldest first")

	invalid := newInfoServerExchange(t, map[string]string{"recentTrades": `[{"coin":"BTC","side":"X","px":"100","sz":"2","time":1791275814004,"tid":7}]`}, nil)
	invalid.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = invalid.GetRecentTrades(t.Context(), perpetualPair, asset.PerpetualContract)
	assert.ErrorIs(t, err, order.ErrSideIsInvalid, "GetRecentTrades should reject an invalid trade side")
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = failed.GetRecentTrades(t.Context(), perpetualPair, asset.PerpetualContract)
	assert.Error(t, err, "GetRecentTrades should return a trades failure")
}

func TestConvertTrade(t *testing.T) {
	t.Parallel()
	_, err := e.convertTrade(&RecentTrade{Side: "X"}, perpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, order.ErrSideIsInvalid, "convertTrade must reject an invalid side")
	result, err := e.convertTrade(&RecentTrade{Coin: "BTC", Side: "B", Price: 100, Size: 2, Time: milli(1791275814004), Hash: "0x1", TradeID: 7}, perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "convertTrade must not error")
	exp := trade.Data{TID: "7", Exchange: e.Name, CurrencyPair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Buy, Price: 100, Amount: 2, Timestamp: time.UnixMilli(1791275814004).UTC()}
	assert.Equal(t, exp, result, "convertTrade should convert the trade")
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	start, end := getTime()
	_, err := e.GetHistoricCandles(t.Context(), perpetualPair, asset.Options, kline.OneHour, start, end)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetHistoricCandles must reject an unsupported asset")
	_, err = e.GetHistoricCandles(t.Context(), currency.EMPTYPAIR, asset.PerpetualContract, kline.OneHour, start, end)
	require.Error(t, err, "GetHistoricCandles must reject an empty pair")
	_, err = e.GetHistoricCandles(t.Context(), perpetualPair, asset.PerpetualContract, kline.OneMin, end.Add(-(maximumCandleCount+1)*time.Minute), end)
	require.ErrorIs(t, err, kline.ErrRequestExceedsExchangeLimits, "GetHistoricCandles must reject a range beyond Hyperliquid's retention")

	result, err := e.GetHistoricCandles(t.Context(), perpetualPair, asset.PerpetualContract, kline.OneHour, start, end)
	require.NoError(t, err, "GetHistoricCandles must not error")
	assert.Equal(t, kline.OneHour, result.Interval, "GetHistoricCandles should return the requested interval")
	if mockTests {
		exp := []kline.Candle{
			{Time: time.UnixMilli(1791252000000).UTC(), Open: 85641, High: 85739, Low: 85433, Close: 85513, Volume: 526.02537},
			{Time: time.UnixMilli(1791255600000).UTC(), Open: 85513, High: 85702, Low: 85390, Close: 85690, Volume: 611.8724},
		}
		for hour := 2; hour < 6; hour++ {
			exp = append(exp, kline.Candle{Time: start.Add(time.Duration(hour) * time.Hour).UTC()})
		}
		assert.Equal(t, exp, result.Candles, "GetHistoricCandles should convert every candle and pad the range's missing hours")
	} else {
		assert.NotEmpty(t, result.Candles, "GetHistoricCandles should return candles")
	}

	ex := newInfoServerExchange(t, map[string]string{"candleSnapshot": `[]`, "meta": `{"universe":[]}`}, nil)
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = ex.GetHistoricCandles(t.Context(), perpetualPair, asset.PerpetualContract, kline.OneHour, start, end)
	assert.ErrorIs(t, err, kline.ErrNoTimeSeriesDataToConvert, "GetHistoricCandles should report an empty response")
	ex.setPairMappings(asset.PerpetualContract, nil)
	_, err = ex.GetHistoricCandles(t.Context(), perpetualPair, asset.PerpetualContract, kline.OneHour, start, end)
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetHistoricCandles should report a pair without a market")
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = failed.GetHistoricCandles(t.Context(), perpetualPair, asset.PerpetualContract, kline.OneHour, start, end)
	assert.Error(t, err, "GetHistoricCandles should return a candle failure")
}

func TestConvertCandle(t *testing.T) {
	t.Parallel()
	exp := kline.Candle{Time: time.UnixMilli(1791252000000).UTC(), Open: 1, High: 4, Low: 0.5, Close: 2, Volume: 3}
	assert.Equal(t, exp, convertCandle(&Candle{OpenTime: milli(1791252000000), CloseTime: milli(1791255599999), Open: 1, Close: 2, High: 4, Low: 0.5, Volume: 3, TradeCount: 5}), "convertCandle should convert the candle")
}

func TestUnsupportedMethods(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	_, err := ex.GetHistoricTrades(t.Context(), spotPair, asset.Spot, time.Time{}, time.Time{})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricTrades should not be supported")
	_, err = ex.GetHistoricCandlesExtended(t.Context(), spotPair, asset.Spot, kline.OneMin, time.Time{}, time.Time{})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricCandlesExtended should not be supported")
	_, err = ex.GetServerTime(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetServerTime should not be supported")
	_, err = ex.GetDepositAddress(t.Context(), currency.USDC, "", "")
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetDepositAddress should not be supported")
	_, err = ex.WithdrawFiatFunds(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "WithdrawFiatFunds should not be supported")
	_, err = ex.WithdrawFiatFundsToInternationalBank(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "WithdrawFiatFundsToInternationalBank should not be supported")
	_, err = ex.GetFuturesContractDetails(t.Context(), asset.PerpetualContract)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetFuturesContractDetails should not be supported")
	_, err = ex.GetCurrencyTradeURL(t.Context(), asset.Spot, spotPair)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetCurrencyTradeURL should not be supported")
	assert.ErrorIs(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.Spot), common.ErrNotYetImplemented, "UpdateOrderExecutionLimits should not be implemented")
}

func TestUpdateAccountBalances(t *testing.T) {
	t.Parallel()
	unconfigured := new(Exchange)
	unconfigured.SetDefaults()
	_, err := unconfigured.UpdateAccountBalances(t.Context(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateAccountBalances must reject an unsupported asset before checking credentials")
	_, err = unconfigured.UpdateAccountBalances(t.Context(), asset.Spot)
	require.Error(t, err, "UpdateAccountBalances must require an account address")

	for _, mode := range []AccountAbstraction{AccountAbstractionUnified, AccountAbstractionPortfolio} {
		ex := newInfoServerExchange(t, map[string]string{
			"spotClearinghouseState": `{"balances":[{"coin":"USDC","token":0,"total":"30","hold":"4"},{"coin":"HYPE","token":150,"total":"5","hold":"1"}]}`,
			"userAbstraction":        `"` + string(mode) + `"`,
			"perpDexs":               `[null,{"name":"xyz"}]`,
		}, nil)
		stale := accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress)
		stale.Balances.Set(currency.USDC, accounts.Balance{Total: 30})
		staleBuilder := accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress+":xyz")
		staleBuilder.Balances.Set(currency.HYPE, accounts.Balance{Total: 5})
		require.NoError(t, ex.Accounts.Save(t.Context(), accounts.SubAccounts{stale, staleBuilder}, true), "Save must not error")
		_, err := ex.UpdateAccountBalances(t.Context(), asset.Spot)
		require.NoErrorf(t, err, "UpdateAccountBalances must not error for %s spot balances", mode)
		result, err := ex.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
		require.NoErrorf(t, err, "UpdateAccountBalances must not error for %s perpetual balances", mode)
		exp := accounts.SubAccounts{
			accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress),
			accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress+":xyz"),
		}
		assert.Equalf(t, exp, result, "UpdateAccountBalances should clear every DEX's separate balance for %s, which holds them in spot", mode)
		balances, err := ex.Accounts.CurrencyBalances(nil, asset.All)
		require.NoError(t, err, "CurrencyBalances must not error")
		assert.Equalf(t, 30.0, balances[currency.USDC].Total, "UpdateAccountBalances should count %s USDC once", mode)
		assert.Equalf(t, 5.0, balances[currency.HYPE].Total, "UpdateAccountBalances should count %s HYPE once", mode)
	}

	builder := newInfoServerExchange(t, map[string]string{
		"userAbstraction":        `"default"`,
		"perpDexs":               `[null,{"name":"xyz"}]`,
		"spotMeta":               `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150}]}`,
		"meta:":                  `{"universe":[],"collateralToken":0}`,
		"meta:xyz":               `{"universe":[],"collateralToken":150}`,
		"clearinghouseState:":    `{"marginSummary":{"accountValue":"20","totalMarginUsed":"3"},"withdrawable":"16"}`,
		"clearinghouseState:xyz": `{"marginSummary":{"accountValue":"7","totalMarginUsed":"2"},"withdrawable":"4"}`,
	}, nil)
	result, err := builder.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "UpdateAccountBalances must not error for a builder DEX with spot collateral")
	clearBalanceTimes(t, result)
	defaultDEX := accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress)
	defaultDEX.Balances.Set(currency.USDC, accounts.Balance{Total: 20, Hold: 3, Free: 16})
	builderDEX := accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress+":xyz")
	builderDEX.Balances.Set(currency.HYPE, accounts.Balance{Total: 7, Hold: 2, Free: 4})
	assert.Equal(t, accounts.SubAccounts{defaultDEX, builderDEX}, result, "UpdateAccountBalances should hold each DEX's collateral in its own subaccount")

	for _, tc := range []struct {
		name      string
		responses map[string]string
		err       error
	}{
		{name: "abstraction failure", responses: map[string]string{"userAbstraction": `"unknown"`}, err: errAccountAbstractionInvalid},
		{name: "registry failure", responses: map[string]string{"userAbstraction": `"default"`, "perpDexs": `{`}},
		{name: "spot metadata failure", responses: map[string]string{"userAbstraction": `"default"`, "spotMeta": `{`}},
		{name: "duplicate token index", responses: map[string]string{"userAbstraction": `"default"`, "spotMeta": `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":0}]}`}, err: errUnexpectedResponseLength},
		{name: "metadata failure", responses: map[string]string{"userAbstraction": `"default"`, "spotMeta": `{"tokens":[{"name":"USDC","index":0}]}`, "meta": `{`}},
		{name: "missing collateral token", responses: map[string]string{"userAbstraction": `"default"`, "spotMeta": `{"tokens":[{"name":"USDC","index":0}]}`, "meta": `{"collateralToken":150}`}, err: errSpotTokenNotFound},
		{name: "clearinghouse failure", responses: map[string]string{"userAbstraction": `"default"`, "spotMeta": `{"tokens":[{"name":"USDC","index":0}]}`, "meta": `{"collateralToken":0}`, "clearinghouseState": `{`}},
	} {
		_, err := newInfoServerExchange(t, tc.responses, nil).UpdateAccountBalances(t.Context(), asset.PerpetualContract)
		if tc.err == nil {
			assert.Errorf(t, err, "UpdateAccountBalances should return a %s", tc.name)
			continue
		}
		assert.ErrorIsf(t, err, tc.err, "UpdateAccountBalances should return a %s", tc.name)
	}
	for _, a := range []asset.Item{asset.Spot, asset.PerpetualContract} {
		_, err = newUnavailableServerExchange(t).UpdateAccountBalances(t.Context(), a)
		assert.Errorf(t, err, "UpdateAccountBalances should return a %s balance failure", a)
	}

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	spot, err := e.UpdateAccountBalances(t.Context(), asset.Spot)
	require.NoError(t, err, "UpdateAccountBalances must not error for spot")
	perpetual, err := e.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "UpdateAccountBalances must not error for perpetuals")
	if !mockTests {
		assert.NotEmpty(t, perpetual, "UpdateAccountBalances should return a subaccount per perpetual DEX")
		return
	}
	clearBalanceTimes(t, spot)
	clearBalanceTimes(t, perpetual)
	expSpot := accounts.NewSubAccount(asset.Spot, testAccountAddress)
	expSpot.Balances.Set(currency.USDC, accounts.Balance{Total: 1520.25, Hold: 120.5, Free: 1399.75})
	expSpot.Balances.Set(currency.HYPE, accounts.Balance{Total: 12.5, Hold: 2.5, Free: 10})
	assert.Equal(t, accounts.SubAccounts{expSpot}, spot, "UpdateAccountBalances should return every spot balance")
	expDefault := accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress)
	expDefault.Balances.Set(currency.USDC, accounts.Balance{Total: 2500.5, Hold: 42.90625, Free: 2457.59375})
	expXYZ := accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress+":xyz")
	expXYZ.Balances.Set(currency.USDC, accounts.Balance{Total: 300, Free: 300})
	expFLX := accounts.NewSubAccount(asset.PerpetualContract, testAccountAddress+":flx")
	expFLX.Balances.Set(currency.NewCode("USDH"), accounts.Balance{Total: 120, Hold: 3.955, Free: 116.045})
	assert.Equal(t, accounts.SubAccounts{expDefault, expXYZ, expFLX}, perpetual, "UpdateAccountBalances should return each perpetual DEX's collateral balance")
}

func TestDEXSubAccountID(t *testing.T) {
	t.Parallel()
	assert.Equal(t, testAccountAddress, dexSubAccountID(testAccountAddress, ""), "dexSubAccountID should not scope the default DEX")
	assert.Equal(t, testAccountAddress+":xyz", dexSubAccountID(testAccountAddress, "xyz"), "dexSubAccountID should scope a builder DEX")
}

func TestGetAccountFundingHistory(t *testing.T) {
	t.Parallel()
	_, err := new(Exchange).GetAccountFundingHistory(t.Context())
	require.Error(t, err, "GetAccountFundingHistory must require an account address")
	_, err = newUnavailableServerExchange(t).GetAccountFundingHistory(t.Context())
	require.Error(t, err, "GetAccountFundingHistory must return a ledger failure")
	_, err = newInfoServerExchange(t, map[string]string{"userNonFundingLedgerUpdates": `[{"time":1791200000000,"hash":"0x1","delta":{"type":""}}]`}, nil).GetAccountFundingHistory(t.Context())
	require.ErrorIs(t, err, errUnexpectedResponseLength, "GetAccountFundingHistory must reject a ledger update without a type")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err := e.GetAccountFundingHistory(t.Context())
	require.NoError(t, err, "GetAccountFundingHistory must not error")
	if !mockTests {
		return
	}
	exp := []exchange.FundingHistory{
		{
			ExchangeName: e.Name,
			Status:       "processed",
			TransferID:   "0x1f6d2c9e3a8b4d5f60718293a4b5c6d7e8f9011223344556677889900aabbcc",
			Description:  "deposit",
			Timestamp:    time.UnixMilli(1791200000000).UTC(),
			Currency:     "USDC",
			Amount:       3000,
			TransferType: "deposit",
			CryptoTxID:   "0x1f6d2c9e3a8b4d5f60718293a4b5c6d7e8f9011223344556677889900aabbcc",
		},
		{
			ExchangeName: e.Name,
			Status:       "processed",
			TransferID:   "0x2a7e3d0f4b9c5e6071829304b5c6d7e8f90a1122334455667788990011aabbdd",
			Description:  "perpetual to spot",
			Timestamp:    time.UnixMilli(1791210000000).UTC(),
			Currency:     "USDC",
			Amount:       1500,
			TransferType: "accountClassTransfer",
			CryptoTxID:   "0x2a7e3d0f4b9c5e6071829304b5c6d7e8f90a1122334455667788990011aabbdd",
		},
		{
			ExchangeName:      e.Name,
			Status:            "processed",
			TransferID:        "0x3b8f4e105cad6f718293a415c6d7e8f90a1b22334455667788990011aabbccee",
			Description:       "spotTransfer",
			Timestamp:         time.UnixMilli(1791220000000).UTC(),
			Currency:          "HYPE",
			Amount:            2.5,
			TransferType:      "spotTransfer",
			CryptoToAddress:   testWithdrawalAddress,
			CryptoFromAddress: testAccountAddress,
			CryptoTxID:        "0x3b8f4e105cad6f718293a415c6d7e8f90a1b22334455667788990011aabbccee",
		},
		{
			ExchangeName: e.Name,
			Status:       "processed",
			TransferID:   "0x4c90a5216dbe7082a3b4c526d7e8f90a1b2c33445566778899001122aabbccff",
			Description:  "withdraw",
			Timestamp:    time.UnixMilli(1791230000000).UTC(),
			Currency:     "USDC",
			Amount:       -100,
			Fee:          1,
			TransferType: "withdraw",
			CryptoTxID:   "0x4c90a5216dbe7082a3b4c526d7e8f90a1b2c33445566778899001122aabbccff",
		},
	}
	assert.Equal(t, exp, result, "GetAccountFundingHistory should convert every ledger update in time order")
}

func TestGetWithdrawalsHistory(t *testing.T) {
	t.Parallel()
	result, err := new(Exchange).GetWithdrawalsHistory(t.Context(), currency.BTC, asset.Empty)
	require.NoError(t, err, "GetWithdrawalsHistory must not error for a currency the bridge does not withdraw")
	assert.Empty(t, result, "GetWithdrawalsHistory should return no withdrawals of a currency the bridge does not withdraw")
	_, err = new(Exchange).GetWithdrawalsHistory(t.Context(), currency.USDC, asset.Empty)
	require.Error(t, err, "GetWithdrawalsHistory must require an account address")
	_, err = newUnavailableServerExchange(t).GetWithdrawalsHistory(t.Context(), currency.USDC, asset.Empty)
	require.Error(t, err, "GetWithdrawalsHistory must return a ledger failure")

	sandbox := newInfoServerExchange(t, map[string]string{"userNonFundingLedgerUpdates": `[{"time":1791200000000,"hash":"0xdeposit","delta":{"type":"deposit","usdc":"3"}},{"time":1791200000001,"hash":"0xwithdraw","delta":{"type":"withdraw","usdc":"-2","fee":"1","nonce":7}}]`}, nil)
	sandbox.Config.UseSandbox = true
	result, err = sandbox.GetWithdrawalsHistory(t.Context(), currency.EMPTYCODE, asset.Empty)
	require.NoError(t, err, "GetWithdrawalsHistory must not error in sandbox mode")
	exp := []exchange.WithdrawalHistory{{
		Status:       "processed",
		TransferID:   "0xwithdraw",
		Description:  "Hyperliquid bridge withdrawal",
		Timestamp:    time.UnixMilli(1791200000001).UTC(),
		Currency:     "USDC",
		Amount:       2,
		Fee:          1,
		TransferType: "withdrawal",
		CryptoTxID:   "0xwithdraw",
		CryptoChain:  "Arbitrum Sepolia",
	}}
	assert.Equal(t, exp, result, "GetWithdrawalsHistory should return only bridge withdrawals, on the testnet bridge chain")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err = e.GetWithdrawalsHistory(t.Context(), currency.USDC, asset.Spot)
	require.NoError(t, err, "GetWithdrawalsHistory must not error")
	if mockTests {
		exp = []exchange.WithdrawalHistory{{
			Status:       "processed",
			TransferID:   "0x4c90a5216dbe7082a3b4c526d7e8f90a1b2c33445566778899001122aabbccff",
			Description:  "Hyperliquid bridge withdrawal",
			Timestamp:    time.UnixMilli(1791230000000).UTC(),
			Currency:     "USDC",
			Amount:       100,
			Fee:          1,
			TransferType: "withdrawal",
			CryptoTxID:   "0x4c90a5216dbe7082a3b4c526d7e8f90a1b2c33445566778899001122aabbccff",
			CryptoChain:  "Arbitrum",
		}}
		assert.Equal(t, exp, result, "GetWithdrawalsHistory should convert the bridge withdrawal")
	}
}

func TestGetUserNonFundingLedgerUpdatesPaginated(t *testing.T) {
	t.Parallel()
	start := time.UnixMilli(1791200000000)
	// ledgerServerExchange serves pages of deposits; page receives the request's start time and the page number, and
	// returns each record's time and hash
	ledgerServerExchange := func(t *testing.T, page func(startTime int64, number int32) (times []int64, hashes []string)) *Exchange {
		t.Helper()
		var pages atomic.Int32
		return newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body InfoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
				return
			}
			times, hashes := page(body.StartTime, pages.Add(1))
			updates := make([]string, len(times))
			for i := range times {
				updates[i] = fmt.Sprintf(`{"time":%d,"hash":%q,"delta":{"type":"deposit","usdc":"1"}}`, times[i], hashes[i])
			}
			_, err := w.Write([]byte("[" + strings.Join(updates, ",") + "]"))
			assert.NoError(t, err, "Writing the ledger page should not error")
		}))
	}
	fullPage := func(startTime int64, prefix string) (times []int64, hashes []string) {
		for i := range int64(maximumUserLedgerHistoryCount) {
			times = append(times, startTime+i)
			hashes = append(hashes, prefix+strconv.FormatInt(i, 10))
		}
		return times, hashes
	}

	var starts []int64
	paged := ledgerServerExchange(t, func(startTime int64, number int32) ([]int64, []string) {
		starts = append(starts, startTime)
		if number == 1 {
			return fullPage(startTime, "0xa")
		}
		return []int64{startTime}, []string{"0xb"}
	})
	result, err := paged.getUserNonFundingLedgerUpdatesPaginated(t.Context(), testAccountAddress, start)
	require.NoError(t, err, "getUserNonFundingLedgerUpdatesPaginated must not error")
	assert.Len(t, result, maximumUserLedgerHistoryCount+1, "getUserNonFundingLedgerUpdatesPaginated should return every page's records")
	assert.Equal(t, []int64{start.UnixMilli(), start.UnixMilli() + maximumUserLedgerHistoryCount - 1}, starts, "getUserNonFundingLedgerUpdatesPaginated should start each page at the previous page's last time")

	repeated := ledgerServerExchange(t, func(startTime int64, number int32) ([]int64, []string) {
		if number == 1 {
			return fullPage(startTime, "0xa")
		}
		return []int64{startTime}, []string{"0xa" + strconv.Itoa(maximumUserLedgerHistoryCount-1)}
	})
	result, err = repeated.getUserNonFundingLedgerUpdatesPaginated(t.Context(), testAccountAddress, start)
	require.NoError(t, err, "getUserNonFundingLedgerUpdatesPaginated must not error for a repeated boundary record")
	assert.Len(t, result, maximumUserLedgerHistoryCount, "getUserNonFundingLedgerUpdatesPaginated should drop a boundary record repeated on the next page")

	for _, tc := range []struct {
		name string
		page func(startTime int64, number int32) ([]int64, []string)
	}{
		{name: "oversized page", page: func(startTime int64, _ int32) ([]int64, []string) {
			times, hashes := fullPage(startTime, "0xa")
			return append(times, startTime+maximumUserLedgerHistoryCount), append(hashes, "0xb")
		}},
		{name: "record before the cursor", page: func(startTime int64, _ int32) ([]int64, []string) {
			return []int64{startTime - 1}, []string{"0xa"}
		}},
		{name: "decreasing times", page: func(startTime int64, _ int32) ([]int64, []string) {
			return []int64{startTime + 2, startTime + 1}, []string{"0xa", "0xb"}
		}},
		{name: "stalled cursor", page: func(startTime int64, _ int32) ([]int64, []string) {
			times, hashes := fullPage(startTime, "0xa")
			for i := range times {
				times[i] = startTime
			}
			return times, hashes
		}},
	} {
		_, err := ledgerServerExchange(t, tc.page).getUserNonFundingLedgerUpdatesPaginated(t.Context(), testAccountAddress, start)
		assert.ErrorIsf(t, err, errUnexpectedResponseLength, "getUserNonFundingLedgerUpdatesPaginated should reject a %s", tc.name)
	}
	_, err = newUnavailableServerExchange(t).getUserNonFundingLedgerUpdatesPaginated(t.Context(), testAccountAddress, start)
	assert.Error(t, err, "getUserNonFundingLedgerUpdatesPaginated should return a ledger failure")
}

func TestConvertUserLedgerUpdate(t *testing.T) {
	t.Parallel()
	_, err := e.convertUserLedgerUpdate(&UserLedgerUpdate{Delta: UserLedgerDelta{Type: " "}})
	require.ErrorIs(t, err, errUnexpectedResponseLength, "convertUserLedgerUpdate must reject an update without a type")

	for _, tc := range []struct {
		delta       UserLedgerDelta
		currency    string
		amount      float64
		description string
	}{
		{delta: UserLedgerDelta{Type: "deposit", USDC: 10}, currency: "USDC", amount: 10, description: "deposit"},
		{delta: UserLedgerDelta{Type: "spotTransfer", Token: currency.HYPE, Amount: 2}, currency: "HYPE", amount: 2, description: "spotTransfer"},
		{delta: UserLedgerDelta{Type: "spotGenesis", Amount: 3}, currency: "USDC", amount: 3, description: "spotGenesis"},
		{delta: UserLedgerDelta{Type: "send", Token: currency.USDC, Amount: 4, SourceDEX: "spot", DestinationDEX: "xyz"}, currency: "USDC", amount: 4, description: "send: spot to xyz"},
		{delta: UserLedgerDelta{Type: "rewardsClaim", Amount: 5}, currency: "USDC", amount: 5, description: "rewardsClaim"},
		{delta: UserLedgerDelta{Type: "vaultWithdraw", NetWithdrawnUSD: 6}, currency: "USDC", amount: 6, description: "vaultWithdraw"},
		{delta: UserLedgerDelta{Type: "accountClassTransfer", USDC: 7}, currency: "USDC", amount: 7, description: "perpetual to spot"},
		{delta: UserLedgerDelta{Type: "accountClassTransfer", USDC: 8, ToPerp: true}, currency: "USDC", amount: 8, description: "spot to perpetual"},
	} {
		tc.delta.Fee = 0.5
		tc.delta.User = testAccountAddress
		tc.delta.Destination = testWithdrawalAddress
		result, err := e.convertUserLedgerUpdate(&UserLedgerUpdate{Delta: tc.delta, Hash: "0x1", Time: milli(1791200000000)})
		require.NoErrorf(t, err, "convertUserLedgerUpdate must not error for %s", tc.description)
		exp := exchange.FundingHistory{
			ExchangeName:      e.Name,
			Status:            "processed",
			TransferID:        "0x1",
			Description:       tc.description,
			Timestamp:         time.UnixMilli(1791200000000).UTC(),
			Currency:          tc.currency,
			Amount:            tc.amount,
			Fee:               0.5,
			TransferType:      tc.delta.Type,
			CryptoToAddress:   testWithdrawalAddress,
			CryptoFromAddress: testAccountAddress,
			CryptoTxID:        "0x1",
		}
		assert.Equalf(t, exp, result, "convertUserLedgerUpdate should convert a %s update", tc.description)
	}
}

// newTradingServerExchange returns an info server exchange with the BTC, xyz:XYZ100 and HYPE spot markets mapped; its
// metadata lists the same markets, so a refetch keeps the mappings, and the xyz DEX has no open orders, unless responses
// replaces them
func newTradingServerExchange(t *testing.T, responses map[string]string, actions func(action map[string]any) string) *Exchange {
	t.Helper()
	merged := map[string]string{
		"perpDexs":               `[null,{"name":"xyz"}]`,
		"meta:":                  `{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":40}]}`,
		"meta:xyz":               `{"universe":[{"name":"xyz:XYZ100","szDecimals":4,"maxLeverage":30}]}`,
		"spotMeta":               `{"universe":[{"tokens":[150,0],"name":"@107","index":107}],"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150,"szDecimals":2}]}`,
		"frontendOpenOrders:xyz": `[]`,
		"openOrders:xyz":         `[]`,
	}
	maps.Copy(merged, responses)
	ex := newInfoServerExchange(t, merged, actions)
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping, testBuilderMapping})
	ex.setPairMappings(asset.Spot, []pairMapping{testSpotMapping})
	return ex
}

// respondWith returns an exchange action handler that answers every action with the body
func respondWith(body string) func(map[string]any) string {
	return func(map[string]any) string { return body }
}

// captureAction returns an exchange action handler that stores each action as JSON and answers it with the body
func captureAction(t *testing.T, captured *string, body string) func(map[string]any) string {
	t.Helper()
	return func(action map[string]any) string {
		encoded, err := json.Marshal(action)
		assert.NoError(t, err, "Encoding the action should not error")
		*captured = string(encoded)
		return body
	}
}

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	_, err := e.SubmitOrder(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrSubmissionIsNil, "SubmitOrder must reject a nil submission")

	limit := &order.Submit{
		Exchange:      e.Name,
		Type:          order.Limit,
		Side:          order.Buy,
		Pair:          perpetualPair,
		AssetType:     asset.PerpetualContract,
		TimeInForce:   order.GoodTillCancel,
		Amount:        0.001,
		Price:         85000,
		ClientOrderID: testClientOrderID,
	}
	watchOnly := newTradingServerExchange(t, nil, nil)
	setTestCredentials(watchOnly, &accounts.Credentials{Key: testAccountAddress})
	_, err = watchOnly.SubmitOrder(t.Context(), limit)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SubmitOrder must require a private key")

	invalid := *limit
	invalid.Amount = 0.000001
	_, err = newTradingServerExchange(t, nil, nil).SubmitOrder(t.Context(), &invalid)
	require.ErrorIs(t, err, errSizePrecision, "SubmitOrder must reject a size beyond the market's precision")
	invalid = *limit
	invalid.TriggerPrice = 90000
	_, err = newTradingServerExchange(t, nil, nil).SubmitOrder(t.Context(), &invalid)
	require.ErrorIs(t, err, errRiskManagementUnsupported, "SubmitOrder must reject a trigger price on a limit order")

	stopMarket := &order.Submit{
		Exchange:          e.Name,
		Type:              order.StopMarket,
		Side:              order.Sell,
		Pair:              perpetualPair,
		AssetType:         asset.PerpetualContract,
		Amount:            0.1,
		Price:             80,
		TriggerPrice:      90,
		TriggerPriceType:  order.MarkPrice,
		ReduceOnly:        true,
		SlippageTolerance: 0.1,
	}
	var action string
	trigger := newTradingServerExchange(t, nil, captureAction(t, &action, statusesResponse("order", `{"resting":{"oid":9}}`)))
	result, err := trigger.SubmitOrder(t.Context(), stopMarket)
	require.NoError(t, err, "SubmitOrder must not error for a stop market order")
	assert.JSONEq(t, `{"type":"order","orders":[{"a":0,"b":false,"p":"80","s":"0.1","r":true,"t":{"trigger":{"isMarket":true,"triggerPx":"90","tpsl":"sl"}}}],"grouping":"na"}`, action, "SubmitOrder should send the stop market order")
	exp := &order.SubmitResponse{
		Exchange:     stopMarket.Exchange,
		Type:         order.StopMarket,
		Side:         order.Sell,
		Pair:         perpetualPair,
		AssetType:    asset.PerpetualContract,
		TimeInForce:  order.UnknownTIF,
		ReduceOnly:   true,
		Price:        80,
		Amount:       0.1,
		TriggerPrice: 90,
		Status:       order.New,
		OrderID:      "9",
		Date:         result.Date,
		LastUpdated:  result.LastUpdated,
	}
	exp.RemainingAmount = 0.1
	assert.Equal(t, exp, result, "SubmitOrder should return the resting trigger order")

	bracket := *limit
	bracket.Exchange = trigger.Name
	bracket.RiskManagementModes = order.RiskManagementModes{
		TakeProfit: order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 90000},
		StopLoss:   order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 80000, LimitPrice: 79000, OrderType: order.Limit},
	}
	grouped := newTradingServerExchange(t, nil, captureAction(t, &action, statusesResponse("order", `{"resting":{"oid":10}}`, `"waitingForFill"`, `"waitingForFill"`)))
	result, err = grouped.SubmitOrder(t.Context(), &bracket)
	require.NoError(t, err, "SubmitOrder must not error for a bracket order")
	assert.JSONEq(t, `{"type":"order","orders":[`+
		`{"a":0,"b":true,"p":"85000","s":"0.001","r":false,"t":{"limit":{"tif":"Gtc"}},"c":"`+testClientOrderID+`"},`+
		`{"a":0,"b":false,"p":"81000","s":"0.001","r":true,"t":{"trigger":{"isMarket":true,"triggerPx":"90000","tpsl":"tp"}}},`+
		`{"a":0,"b":false,"p":"79000","s":"0.001","r":true,"t":{"trigger":{"isMarket":false,"triggerPx":"80000","tpsl":"sl"}}}],"grouping":"normalTpsl"}`,
		action, "SubmitOrder should send the parent with its take-profit and stop-loss children")
	assert.Equal(t, "10", result.OrderID, "SubmitOrder should return the parent's order ID")
	assert.NoError(t, result.SubmissionError, "SubmitOrder should accept children that wait for their parent's fill")

	for _, tc := range []struct {
		name       string
		submit     *order.Submit
		mids       string
		statuses   []string
		status     order.Status
		remaining  float64
		submission error
		err        error
	}{
		{name: "partial GTC fill", submit: limit, statuses: []string{`{"filled":{"oid":7,"totalSz":"0.0004","avgPx":"85000"}}`}, status: order.PartiallyFilled, remaining: 0.0006},
		{name: "rejected child", submit: &bracket, statuses: []string{`{"resting":{"oid":11}}`, `{"error":"bad TP"}`, `"waitingForFill"`}, status: order.New, remaining: 0.001, submission: errGroupedOrderChildFailure},
		{name: "partial ALO fill", submit: func() *order.Submit { s := *limit; s.TimeInForce = order.PostOnly; return &s }(), statuses: []string{`{"filled":{"oid":7,"totalSz":"0.0004","avgPx":"85000"}}`}, err: errActionStatusMalformed},
		{name: "partial trigger fill", submit: stopMarket, statuses: []string{`{"filled":{"oid":9,"totalSz":"0.04","avgPx":"90"}}`}, err: errActionStatusMalformed},
		{name: "rejected batch", submit: &bracket, statuses: []string{`{"error":"batch rejected"}`}, err: order.ErrUnableToPlaceOrder},
		{name: "waiting parent", submit: &bracket, statuses: []string{`"waitingForFill"`, `"waitingForFill"`, `"waitingForFill"`}, err: errActionStatusMalformed},
		{name: "over-reported fill", submit: limit, statuses: []string{`{"filled":{"oid":7,"totalSz":"0.002","avgPx":"85000"}}`}, err: errInvalidFilledSize},
		{name: "fill beyond the market's precision", submit: limit, statuses: []string{`{"filled":{"oid":7,"totalSz":"0.000400001","avgPx":"85000"}}`}, err: errInvalidFilledSize},
		{name: "missing status", submit: limit, err: errActionStatusCount},
	} {
		ex := newTradingServerExchange(t, map[string]string{"allMids": `{"BTC":"85000"}`}, respondWith(statusesResponse("order", tc.statuses...)))
		submitted, err := ex.SubmitOrder(t.Context(), tc.submit)
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "SubmitOrder should reject a %s", tc.name)
			continue
		}
		require.NoErrorf(t, err, "SubmitOrder must not error for a %s", tc.name)
		assert.Equalf(t, tc.status, submitted.Status, "SubmitOrder should return the status of a %s", tc.name)
		assert.InDeltaf(t, tc.remaining, submitted.RemainingAmount, 1e-12, "SubmitOrder should return the remaining amount of a %s", tc.name)
		assert.ErrorIsf(t, submitted.SubmissionError, tc.submission, "SubmitOrder should return the submission error of a %s", tc.name)
	}
	_, err = newTradingServerExchange(t, nil, respondWith(`{"status":"err","response":"Insufficient margin to place order."}`)).SubmitOrder(t.Context(), limit)
	assert.ErrorIs(t, err, errActionResponse, "SubmitOrder should return a rejected action")
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = failed.SubmitOrder(t.Context(), limit)
	assert.Error(t, err, "SubmitOrder should return an action failure")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	result, err = e.SubmitOrder(t.Context(), limit)
	require.NoError(t, err, "SubmitOrder must not error for a limit order")
	if mockTests {
		exp = &order.SubmitResponse{
			Exchange:        e.Name,
			Type:            order.Limit,
			Side:            order.Buy,
			Pair:            perpetualPair,
			AssetType:       asset.PerpetualContract,
			TimeInForce:     order.GoodTillCancel,
			Price:           85000,
			Amount:          0.001,
			RemainingAmount: 0.001,
			ClientOrderID:   testClientOrderID,
			Status:          order.New,
			OrderID:         strconv.FormatUint(testOrderID, 10),
			Date:            result.Date,
			LastUpdated:     result.LastUpdated,
		}
		assert.Equal(t, exp, result, "SubmitOrder should return the resting limit order")
	}

	market := &order.Submit{Exchange: e.Name, Type: order.Market, Side: order.Sell, Pair: spotPair, AssetType: asset.Spot, Amount: 1.5, SlippageTolerance: 0.01}
	result, err = e.SubmitOrder(t.Context(), market)
	require.NoError(t, err, "SubmitOrder must not error for a market order")
	if mockTests {
		exp = &order.SubmitResponse{
			Exchange:             e.Name,
			Type:                 order.Market,
			Side:                 order.Sell,
			Pair:                 spotPair,
			AssetType:            asset.Spot,
			TimeInForce:          order.ImmediateOrCancel,
			Price:                92.21,
			Amount:               1.5,
			AverageExecutedPrice: 93.14,
			Status:               order.Filled,
			OrderID:              "566563303542",
			Date:                 result.Date,
			LastUpdated:          result.LastUpdated,
		}
		assert.Equal(t, exp, result, "SubmitOrder should return the filled market order, bounded below the mid price by the slippage tolerance")
	}
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	_, err := e.ModifyOrder(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrModifyOrderIsNil, "ModifyOrder must reject a nil modification")

	const openOrderStatus = `{"status":"order","order":{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1791275926947,"isTrigger":false,"triggerPx":"0","reduceOnly":true,"orderType":"Limit","tif":"Gtc","cloid":"0x00000000000000000000000000000001"},"status":"open","statusTimestamp":1791275930650}}`
	watchOnly := newTradingServerExchange(t, nil, nil)
	setTestCredentials(watchOnly, &accounts.Credentials{Key: testAccountAddress})
	_, err = watchOnly.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "ModifyOrder must require a private key")

	invalid := newTradingServerExchange(t, map[string]string{"orderStatus": openOrderStatus}, nil)
	for _, tc := range []struct {
		modify *order.Modify
		err    error
	}{
		{modify: &order.Modify{OrderID: "bad", Pair: perpetualPair, AssetType: asset.PerpetualContract}, err: order.ErrOrderIDNotSet},
		{modify: &order.Modify{OrderID: "0", Pair: perpetualPair, AssetType: asset.PerpetualContract}, err: order.ErrOrderIDNotSet},
		{modify: &order.Modify{ClientOrderID: "invalid", Pair: perpetualPair, AssetType: asset.PerpetualContract}, err: errClientOrderIDInvalid},
		{modify: &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract, Type: order.Market}, err: order.ErrUnsupportedOrderType},
		{modify: &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract, TimeInForce: order.ImmediateOrCancel}, err: order.ErrUnsupportedTimeInForce},
		{modify: &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract, TriggerPrice: 90}, err: errRiskManagementUnsupported},
		{modify: &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract, Amount: 0.000001}, err: errSizePrecision},
	} {
		_, err := invalid.ModifyOrder(t.Context(), tc.modify)
		assert.ErrorIsf(t, err, tc.err, "ModifyOrder should reject %+v", tc.modify)
	}
	triggerOrder := newTradingServerExchange(t, map[string]string{"orderStatus": `{"status":"order","order":{"order":{"coin":"BTC","side":"A","limitPx":"80","sz":"1","origSz":"1","oid":12,"timestamp":1791275926947,"isTrigger":true,"triggerPx":"90","reduceOnly":true,"orderType":"Stop Market","tif":null},"status":"open","statusTimestamp":1791275930650}}`}, nil)
	_, err = triggerOrder.ModifyOrder(t.Context(), &order.Modify{OrderID: "12", Pair: perpetualPair, AssetType: asset.PerpetualContract})
	assert.ErrorIs(t, err, errModifyOrderTypeUnsupported, "ModifyOrder should reject a trigger order, which only always-place modifies replace")
	_, err = newTradingServerExchange(t, map[string]string{"orderStatus": `{"status":"unknownOid"}`}, nil).ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract})
	assert.ErrorIs(t, err, order.ErrOrderNotFound, "ModifyOrder should report an unknown order")
	filledOrder := strings.Replace(strings.Replace(openOrderStatus, `"sz":"1"`, `"sz":"0"`, 1), `"status":"open"`, `"status":"filled"`, 1)
	_, err = newTradingServerExchange(t, map[string]string{"orderStatus": filledOrder}, nil).ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract})
	assert.ErrorIs(t, err, errOrderNotModifiable, "ModifyOrder should reject a filled order")

	var action string
	explicit := newTradingServerExchange(t, map[string]string{"orderStatus": openOrderStatus}, captureAction(t, &action, statusesResponse("order", `{"resting":{"oid":8}}`)))
	result, err := explicit.ModifyOrder(t.Context(), &order.Modify{
		OrderID:          "7",
		Pair:             perpetualPair,
		AssetType:        asset.PerpetualContract,
		Price:            101,
		Amount:           1.5,
		Side:             order.Sell,
		Type:             order.Limit,
		TimeInForce:      order.PostOnly,
		NewClientOrderID: "0x00000000000000000000000000000002",
	})
	require.NoError(t, err, "ModifyOrder must not error")
	assert.JSONEq(t, `{"type":"batchModify","modifies":[{"oid":7,"order":{"a":0,"b":false,"p":"101","s":"1.5","r":true,"t":{"limit":{"tif":"Alo"}},"c":"0x00000000000000000000000000000002"}}]}`, action, "ModifyOrder should send the replacement")
	exp := &order.ModifyResponse{
		Exchange:        explicit.Name,
		OrderID:         "8",
		ClientOrderID:   "0x00000000000000000000000000000002",
		Pair:            perpetualPair,
		Type:            order.Limit,
		Side:            order.Sell,
		Status:          order.New,
		AssetType:       asset.PerpetualContract,
		TimeInForce:     order.PostOnly,
		Price:           101,
		Amount:          1.5,
		RemainingAmount: 1.5,
		Date:            time.UnixMilli(1791275926947).UTC(),
		LastUpdated:     result.LastUpdated,
	}
	assert.Equal(t, exp, result, "ModifyOrder should return the resting replacement")

	inherited := newTradingServerExchange(t, map[string]string{"orderStatus": openOrderStatus}, captureAction(t, &action, statusesResponse("order", `{"resting":{"oid":7}}`)))
	result, err = inherited.ModifyOrder(t.Context(), &order.Modify{ClientOrderID: "0X00000000000000000000000000000001", Pair: perpetualPair, AssetType: asset.PerpetualContract})
	require.NoError(t, err, "ModifyOrder must not error by client order ID")
	assert.JSONEq(t, `{"type":"batchModify","modifies":[{"oid":"0x00000000000000000000000000000001","order":{"a":0,"b":true,"p":"100","s":"1","r":true,"t":{"limit":{"tif":"Gtc"}},"c":"0x00000000000000000000000000000001"}}]}`, action, "ModifyOrder should replace the order with its own remaining size and fields")
	assert.Equal(t, "7", result.OrderID, "ModifyOrder should return the order ID")

	for _, tc := range []struct {
		status    string
		exp       order.Status
		remaining float64
		err       error
	}{
		{status: `{"filled":{"oid":7,"totalSz":"1","avgPx":"100"}}`, exp: order.Filled},
		{status: `{"filled":{"oid":7,"totalSz":"0.4","avgPx":"100"}}`, exp: order.PartiallyFilled, remaining: 0.6},
		{status: `{"filled":{"oid":7,"totalSz":"2","avgPx":"100"}}`, err: errInvalidFilledSize},
		{status: `{"filled":{"oid":7,"totalSz":"0.400001","avgPx":"100"}}`, err: errInvalidFilledSize},
		{status: `{"error":"Cannot modify canceled or filled order"}`, err: order.ErrUnableToPlaceOrder},
	} {
		ex := newTradingServerExchange(t, map[string]string{"orderStatus": openOrderStatus}, respondWith(statusesResponse("order", tc.status)))
		modified, err := ex.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract})
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "ModifyOrder should reject status %s", tc.status)
			continue
		}
		require.NoErrorf(t, err, "ModifyOrder must not error for status %s", tc.status)
		assert.Equalf(t, tc.exp, modified.Status, "ModifyOrder should derive the status of %s", tc.status)
		assert.InDeltaf(t, tc.remaining, modified.RemainingAmount, 1e-12, "ModifyOrder should derive the remaining amount of %s", tc.status)
	}
	_, err = newTradingServerExchange(t, map[string]string{"orderStatus": openOrderStatus}, respondWith(statusesResponse("order"))).ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract})
	assert.ErrorIs(t, err, errActionStatusCount, "ModifyOrder should reject a response without a status")
	_, err = newTradingServerExchange(t, map[string]string{"orderStatus": openOrderStatus}, respondWith(`{"status":"err","response":"modify failed"}`)).ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract})
	assert.ErrorIs(t, err, errActionResponse, "ModifyOrder should return a rejected action")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	result, err = e.ModifyOrder(t.Context(), &order.Modify{OrderID: strconv.FormatUint(testOrderID, 10), Pair: perpetualPair, AssetType: asset.PerpetualContract, Price: 84000})
	require.NoError(t, err, "ModifyOrder must not error")
	if mockTests {
		exp = &order.ModifyResponse{
			Exchange:        e.Name,
			OrderID:         "566563303537",
			ClientOrderID:   testClientOrderID,
			Pair:            perpetualPair,
			Type:            order.Limit,
			Side:            order.Buy,
			Status:          order.New,
			AssetType:       asset.PerpetualContract,
			TimeInForce:     order.PostOnly,
			Price:           84000,
			Amount:          0.001,
			RemainingAmount: 0.001,
			Date:            time.UnixMilli(1791275926947).UTC(),
			LastUpdated:     result.LastUpdated,
		}
		assert.Equal(t, exp, result, "ModifyOrder should return the replacement order")
	}
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.CancelOrder(t.Context(), nil), order.ErrCancelOrderIsNil, "CancelOrder must reject a nil cancel")
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	for _, cancel := range []*order.Cancel{
		{OrderID: strconv.FormatUint(testOrderID, 10), Pair: perpetualPair, AssetType: asset.PerpetualContract},
		{ClientOrderID: testClientOrderID, Pair: perpetualPair, AssetType: asset.PerpetualContract},
	} {
		assert.NoErrorf(t, e.CancelOrder(t.Context(), cancel), "CancelOrder should not error for %+v", cancel)
	}
}

func TestCancelBatchOrders(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	result, err := e.CancelBatchOrders(t.Context(), []order.Cancel{
		{OrderID: strconv.FormatUint(testOrderID, 10), Pair: perpetualPair, AssetType: asset.PerpetualContract},
		{ClientOrderID: testClientOrderID, Pair: perpetualPair, AssetType: asset.PerpetualContract},
	})
	require.NoError(t, err, "CancelBatchOrders must not error")
	exp := &order.CancelBatchResponse{Status: map[string]string{strconv.FormatUint(testOrderID, 10): "success", testClientOrderID: "success"}}
	assert.Equal(t, exp, result, "CancelBatchOrders should return each cancel's status")
}

func TestCancelOrders(t *testing.T) {
	t.Parallel()
	var actions []string
	ex := newTradingServerExchange(t, nil, func(action map[string]any) string {
		encoded, err := json.Marshal(action)
		assert.NoError(t, err, "Encoding the action should not error")
		actions = append(actions, string(encoded))
		cancels, _ := action["cancels"].([]any)
		return statusesResponse("cancel", slices.Repeat([]string{`"success"`}, len(cancels))...)
	})
	statuses, err := ex.cancelOrders(t.Context(), nil)
	require.NoError(t, err, "cancelOrders must not error for no cancels")
	assert.Empty(t, statuses, "cancelOrders should return no statuses for no cancels")
	_, err = ex.cancelOrders(t.Context(), make([]order.Cancel, maximumActionBatchSize+1))
	require.ErrorIs(t, err, errActionBatchTooLarge, "cancelOrders must reject an oversized batch")

	for _, tc := range []struct {
		cancel order.Cancel
		err    error
	}{
		{cancel: order.Cancel{OrderID: "1", AssetType: asset.PerpetualContract}, err: order.ErrPairIsEmpty},
		{cancel: order.Cancel{OrderID: "1", Pair: perpetualPair}, err: order.ErrAssetNotSet},
		{cancel: order.Cancel{OrderID: "1", Pair: currency.NewPair(currency.ETH, currency.USDC), AssetType: asset.PerpetualContract}, err: errPairMappingNotFound},
		{cancel: order.Cancel{OrderID: "bad", Pair: perpetualPair, AssetType: asset.PerpetualContract}, err: order.ErrOrderIDNotSet},
		{cancel: order.Cancel{OrderID: "0", Pair: perpetualPair, AssetType: asset.PerpetualContract}, err: order.ErrOrderIDNotSet},
		{cancel: order.Cancel{ClientOrderID: "invalid", Pair: perpetualPair, AssetType: asset.PerpetualContract}, err: errClientOrderIDInvalid},
		{cancel: order.Cancel{Pair: perpetualPair, AssetType: asset.PerpetualContract}, err: order.ErrOrderIDNotSet},
	} {
		_, err := ex.cancelOrders(t.Context(), []order.Cancel{tc.cancel})
		assert.ErrorIsf(t, err, tc.err, "cancelOrders should reject %+v", tc.cancel)
	}
	assert.Empty(t, actions, "cancelOrders should not send an invalid batch")

	upperClientOrderID := strings.ToUpper(testClientOrderID[2:])
	statuses, err = ex.cancelOrders(t.Context(), []order.Cancel{
		{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract},
		{ClientOrderID: "0x" + upperClientOrderID, Pair: spotPair, AssetType: asset.Spot},
		{OrderID: "8", Pair: testBuilderPair, AssetType: asset.PerpetualContract},
	})
	require.NoError(t, err, "cancelOrders must not error")
	assert.Equal(t, map[string]string{"7": "success", "0x" + upperClientOrderID: "success", "8": "success"}, statuses, "cancelOrders should key each status by the caller's identifier")
	assert.Equal(t, []string{
		`{"cancels":[{"a":0,"o":7},{"a":110000,"o":8}],"type":"cancel"}`,
		`{"cancels":[{"asset":10107,"cloid":"` + testClientOrderID + `"}],"type":"cancelByCloid"}`,
	}, actions, "cancelOrders should cancel order IDs and client order IDs in one action each")

	statuses, err = newTradingServerExchange(t, nil, respondWith(`{"status":"err","response":"cancel failed"}`)).cancelOrders(t.Context(), []order.Cancel{
		{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract},
		{ClientOrderID: testClientOrderID, Pair: perpetualPair, AssetType: asset.PerpetualContract},
	})
	assert.ErrorIs(t, err, errActionResponse, "cancelOrders should return a rejected action")
	assert.Empty(t, statuses, "cancelOrders should not report a rejected action's cancels as successful")
	_, err = newTradingServerExchange(t, nil, respondWith(statusesResponse("cancel"))).cancelOrders(t.Context(), []order.Cancel{{OrderID: "7", Pair: perpetualPair, AssetType: asset.PerpetualContract}})
	assert.ErrorIs(t, err, errActionStatusCount, "cancelOrders should reject a response without a status")
}

func TestRecordCancelStatuses(t *testing.T) {
	t.Parallel()
	statuses := make(map[string]string)
	err := recordCancelStatuses(statuses, []string{"1", "2"}, []CancelActionStatus{{}, {Error: "Order was never placed, already canceled, or filled."}})
	assert.ErrorIs(t, err, errActionResponse, "recordCancelStatuses should return each failed cancel")
	assert.Equal(t, map[string]string{"1": "success", "2": "Order was never placed, already canceled, or filled."}, statuses, "recordCancelStatuses should record each cancel's outcome")
}

func TestCancelAllOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrCancelOrderIsNil, "CancelAllOrders must reject a nil cancel")
	_, err = e.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Options})
	require.ErrorIs(t, err, asset.ErrNotSupported, "CancelAllOrders must reject an unsupported asset")
	_, err = new(Exchange).CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.Error(t, err, "CancelAllOrders must require credentials")

	successes := func(action map[string]any) string {
		cancels, _ := action["cancels"].([]any)
		return statusesResponse("cancel", slices.Repeat([]string{`"success"`}, len(cancels))...)
	}
	openOrders := `[{"coin":"BTC","oid":7},{"coin":"@107","oid":8},{"coin":"MISSING","oid":9}]`
	ex := newTradingServerExchange(t, map[string]string{"openOrders": openOrders}, successes)
	result, err := ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	assert.ErrorIs(t, err, errPairMappingNotFound, "CancelAllOrders should report an order without a market")
	assert.Equal(t, order.CancelAllResponse{Status: map[string]string{"7": "success"}}, result, "CancelAllOrders should cancel the asset's orders")
	result, err = ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract, Pair: currency.NewPair(currency.ETH, currency.USDC)})
	assert.ErrorIs(t, err, errPairMappingNotFound, "CancelAllOrders should report an order without a market")
	assert.Empty(t, result.Status, "CancelAllOrders should cancel only the requested pair's orders")

	orders := make([]string, maximumActionBatchSize+1)
	for i := range orders {
		orders[i] = `{"coin":"BTC","oid":` + strconv.Itoa(i+1) + `}`
	}
	chunked := newTradingServerExchange(t, map[string]string{"openOrders": `[` + strings.Join(orders, ",") + `]`}, successes)
	result, err = chunked.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.NoError(t, err, "CancelAllOrders must not error beyond one action's batch")
	assert.Len(t, result.Status, maximumActionBatchSize+1, "CancelAllOrders should cancel every order across batches")
	_, err = newUnavailableServerExchange(t).CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	assert.Error(t, err, "CancelAllOrders should return an open orders failure")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	result, err = e.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.NoError(t, err, "CancelAllOrders must not error for perpetuals")
	if mockTests {
		assert.Equal(t, order.CancelAllResponse{Status: map[string]string{strconv.FormatUint(testOrderID, 10): "success", "566563303539": "success"}}, result, "CancelAllOrders should cancel every DEX's perpetual orders")
		result, err = e.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Spot})
		assert.ErrorIs(t, err, errActionResponse, "CancelAllOrders should report a failed cancel")
		assert.Equal(t, order.CancelAllResponse{Status: map[string]string{"566563303538": "Order was never placed, already canceled, or filled. asset=10107"}}, result, "CancelAllOrders should return a failed cancel's reason")
	}
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderInfo(t.Context(), "", perpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet, "GetOrderInfo must require an order ID")
	_, err = e.GetOrderInfo(t.Context(), "7", perpetualPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetOrderInfo must reject an unsupported asset")
	_, err = new(Exchange).GetOrderInfo(t.Context(), "7", perpetualPair, asset.Empty)
	require.Error(t, err, "GetOrderInfo must require an account address")

	const openOrderStatus = `{"status":"order","order":{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1791275926947,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1791275930650}}`
	ex := newTradingServerExchange(t, map[string]string{"orderStatus": openOrderStatus}, nil)
	_, err = ex.GetOrderInfo(t.Context(), "invalid", currency.EMPTYPAIR, asset.Empty)
	assert.ErrorIs(t, err, errClientOrderIDInvalid, "GetOrderInfo should reject an identifier that is neither an order ID nor a client order ID")
	_, err = ex.GetOrderInfo(t.Context(), "7", spotPair, asset.Empty)
	assert.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo should not return an order of another pair")
	_, err = ex.GetOrderInfo(t.Context(), "7", currency.EMPTYPAIR, asset.Spot)
	assert.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo should not return an order of another asset")
	for _, response := range []string{`{"status":"unknownOid"}`, `{"status":"order","order":null}`} {
		_, err = newTradingServerExchange(t, map[string]string{"orderStatus": response}, nil).GetOrderInfo(t.Context(), "7", currency.EMPTYPAIR, asset.Empty)
		assert.ErrorIsf(t, err, order.ErrOrderNotFound, "GetOrderInfo should report %s as not found", response)
	}
	_, err = newTradingServerExchange(t, map[string]string{"orderStatus": strings.Replace(openOrderStatus, `"BTC"`, `"MISSING"`, 1)}, nil).GetOrderInfo(t.Context(), "7", currency.EMPTYPAIR, asset.Empty)
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetOrderInfo should report an order without a market")
	_, err = newUnavailableServerExchange(t).GetOrderInfo(t.Context(), "7", currency.EMPTYPAIR, asset.Empty)
	assert.Error(t, err, "GetOrderInfo should return an order status failure")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	for _, tc := range []struct {
		orderID string
		pair    currency.Pair
		a       asset.Item
	}{
		{orderID: strconv.FormatUint(testOrderID, 10), pair: perpetualPair, a: asset.PerpetualContract},
		{orderID: testClientOrderID},
	} {
		result, err := e.GetOrderInfo(t.Context(), tc.orderID, tc.pair, tc.a)
		if !mockTests {
			if err != nil {
				assert.ErrorIsf(t, err, order.ErrOrderNotFound, "GetOrderInfo should only report %s as not found", tc.orderID)
			}
			continue
		}
		require.NoErrorf(t, err, "GetOrderInfo must not error for %s", tc.orderID)
		exp := &order.Detail{
			TimeInForce:     order.GoodTillCancel,
			Price:           85000,
			Amount:          0.002,
			ExecutedAmount:  0.001,
			RemainingAmount: 0.001,
			Exchange:        e.Name,
			OrderID:         strconv.FormatUint(testOrderID, 10),
			ClientOrderID:   testClientOrderID,
			Type:            order.Limit,
			Side:            order.Buy,
			Status:          order.Open,
			AssetType:       asset.PerpetualContract,
			Date:            time.UnixMilli(1791275926947).UTC(),
			LastUpdated:     time.UnixMilli(1791275926947).UTC(),
			Pair:            perpetualPair,
		}
		assert.Equalf(t, exp, result, "GetOrderInfo should convert the order for %s", tc.orderID)
	}
	_, err = e.GetOrderInfo(t.Context(), "1", perpetualPair, asset.PerpetualContract)
	assert.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo should report an unknown order as not found")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetActiveOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrGetOrdersRequestIsNil, "GetActiveOrders must reject a nil request")
	_, err = e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{AssetType: asset.Options, Side: order.AnySide, Type: order.AnyType})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetActiveOrders must reject an unsupported asset")
	perpetualRequest := &order.MultiOrderRequest{AssetType: asset.PerpetualContract, Side: order.AnySide, Type: order.AnyType}
	_, err = new(Exchange).GetActiveOrders(t.Context(), perpetualRequest)
	require.Error(t, err, "GetActiveOrders must require an account address")
	_, err = newUnavailableServerExchange(t).GetActiveOrders(t.Context(), perpetualRequest)
	require.Error(t, err, "GetActiveOrders must return an open orders failure")

	mixed := newTradingServerExchange(t, map[string]string{
		"frontendOpenOrders": `[{"coin":"MISSING","side":"B","limitPx":"1","sz":"1","origSz":"1","oid":7,"timestamp":1791275926947,"orderType":"Limit","tif":"Gtc"},{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":8,"timestamp":1791275926947,"orderType":"Limit","tif":"Gtc"}]`,
	}, nil)
	orders, err := mixed.GetActiveOrders(t.Context(), perpetualRequest)
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetActiveOrders should report an order without a market")
	require.Len(t, orders, 1, "GetActiveOrders must return the convertible orders")
	assert.Equal(t, "8", orders[0].OrderID, "GetActiveOrders should return the convertible order")

	frontendOrders := make([]string, maximumFrontendOpenOrders)
	basicOrders := make([]string, maximumFrontendOpenOrders+2)
	for i := range maximumFrontendOpenOrders + 1 {
		basicOrder := `{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"1","oid":` + strconv.Itoa(i+1) + `,"timestamp":1791275926947`
		if i < maximumFrontendOpenOrders {
			frontendOrders[i] = basicOrder + `,"orderType":"Limit","tif":"Alo"}`
		}
		basicOrders[i] = basicOrder + `}`
	}
	basicOrders[maximumFrontendOpenOrders+1] = `{"coin":"MISSING","side":"B","oid":102,"timestamp":1791275926947}`
	capped := newTradingServerExchange(t, map[string]string{
		"frontendOpenOrders": `[` + strings.Join(frontendOrders, ",") + `]`,
		"openOrders":         `[` + strings.Join(basicOrders, ",") + `]`,
	}, nil)
	orders, err = capped.GetActiveOrders(t.Context(), perpetualRequest)
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetActiveOrders should report an older order without a market")
	require.Len(t, orders, maximumFrontendOpenOrders+1, "GetActiveOrders must add the orders beyond the frontendOpenOrders limit")
	placed := time.UnixMilli(1791275926947).UTC()
	exp := order.Detail{Price: 100, Amount: 1, RemainingAmount: 1, Exchange: capped.Name, OrderID: "1", Type: order.Limit, TimeInForce: order.PostOnly, Side: order.Buy, Status: order.Open, AssetType: asset.PerpetualContract, Date: placed, LastUpdated: placed, Pair: perpetualPair}
	assert.Equal(t, exp, orders[0], "GetActiveOrders should keep the frontend fields of the orders within the limit")
	exp = order.Detail{Price: 100, Amount: 1, RemainingAmount: 1, Exchange: capped.Name, OrderID: "101", Side: order.Buy, Status: order.Open, AssetType: asset.PerpetualContract, Date: placed, LastUpdated: placed, Pair: perpetualPair}
	assert.Equal(t, exp, orders[maximumFrontendOpenOrders], "GetActiveOrders should add an order beyond the limit without its order type or time in force")
	unavailable := newTradingServerExchange(t, map[string]string{"frontendOpenOrders": `[` + strings.Join(frontendOrders, ",") + `]`, "openOrders": `{`}, nil)
	_, err = unavailable.GetActiveOrders(t.Context(), perpetualRequest)
	assert.Error(t, err, "GetActiveOrders should return an openOrders failure")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	orders, err = e.GetActiveOrders(t.Context(), perpetualRequest)
	require.NoError(t, err, "GetActiveOrders must not error for perpetuals")
	spotOrders, err := e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{AssetType: asset.Spot, Side: order.AnySide, Type: order.AnyType})
	require.NoError(t, err, "GetActiveOrders must not error for spot")
	if !mockTests {
		return
	}
	expOrders := order.FilteredOrders{
		{TimeInForce: order.GoodTillCancel, Price: 85000, Amount: 0.002, ExecutedAmount: 0.001, RemainingAmount: 0.001, Exchange: e.Name, OrderID: strconv.FormatUint(testOrderID, 10), ClientOrderID: testClientOrderID, Type: order.Limit, Side: order.Buy, Status: order.Open, AssetType: asset.PerpetualContract, Date: placed, LastUpdated: placed, Pair: perpetualPair},
		{TimeInForce: order.GoodTillCancel, ReduceOnly: true, Price: 27000, Amount: 0.05, TriggerPrice: 30000, RemainingAmount: 0.05, Exchange: e.Name, OrderID: "566563303539", Type: order.StopMarket, Side: order.Sell, Status: order.Open, AssetType: asset.PerpetualContract, Date: placed, LastUpdated: placed, Pair: testBuilderPair},
	}
	assert.Equal(t, expOrders, orders, "GetActiveOrders should convert every DEX's perpetual orders")
	expOrders = order.FilteredOrders{
		{TimeInForce: order.PostOnly, Price: 95.5, Amount: 1.5, RemainingAmount: 1.5, Exchange: e.Name, OrderID: "566563303538", Type: order.Limit, Side: order.Sell, Status: order.Open, AssetType: asset.Spot, Date: placed, LastUpdated: placed, Pair: spotPair},
	}
	assert.Equal(t, expOrders, spotOrders, "GetActiveOrders should convert the spot orders")
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderHistory(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrGetOrdersRequestIsNil, "GetOrderHistory must reject a nil request")
	_, err = e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{AssetType: asset.Options, Side: order.AnySide, Type: order.AnyType})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetOrderHistory must reject an unsupported asset")
	perpetualRequest := &order.MultiOrderRequest{AssetType: asset.PerpetualContract, Side: order.AnySide, Type: order.AnyType}
	_, err = new(Exchange).GetOrderHistory(t.Context(), perpetualRequest)
	require.Error(t, err, "GetOrderHistory must require an account address")
	_, err = newUnavailableServerExchange(t).GetOrderHistory(t.Context(), perpetualRequest)
	require.Error(t, err, "GetOrderHistory must return a history failure")

	mixed := newTradingServerExchange(t, map[string]string{
		"historicalOrders": `[{"order":{"coin":"MISSING","side":"B","limitPx":"1","sz":"1","origSz":"1","oid":7,"timestamp":1791275926947,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1791275930650},{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"0","origSz":"2","oid":8,"timestamp":1791275926947,"orderType":"Limit","tif":"Gtc"},"status":"filled","statusTimestamp":1791275930650}]`,
	}, nil)
	orders, err := mixed.GetOrderHistory(t.Context(), perpetualRequest)
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetOrderHistory should report an order without a market")
	require.Len(t, orders, 1, "GetOrderHistory must return the convertible orders")
	assert.Equal(t, order.Filled, orders[0].Status, "GetOrderHistory should convert the order's status")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	orders, err = e.GetOrderHistory(t.Context(), perpetualRequest)
	require.NoError(t, err, "GetOrderHistory must not error for perpetuals")
	spotOrders, err := e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{AssetType: asset.Spot, Side: order.AnySide, Type: order.AnyType})
	require.NoError(t, err, "GetOrderHistory must not error for spot")
	if !mockTests {
		return
	}
	placed := time.UnixMilli(1791275926947).UTC()
	exp := order.FilteredOrders{
		{TimeInForce: order.GoodTillCancel, Price: 85000, Amount: 0.002, ExecutedAmount: 0.001, RemainingAmount: 0.001, Exchange: e.Name, OrderID: strconv.FormatUint(testOrderID, 10), ClientOrderID: testClientOrderID, Type: order.Limit, Side: order.Buy, Status: order.Open, AssetType: asset.PerpetualContract, Date: placed, LastUpdated: placed, Pair: perpetualPair},
		{TimeInForce: order.ImmediateOrCancel, Price: 86500, Amount: 0.01, ExecutedAmount: 0.01, Exchange: e.Name, OrderID: "566563303540", Type: order.Limit, Side: order.Sell, Status: order.Filled, AssetType: asset.PerpetualContract, Date: placed, LastUpdated: time.UnixMilli(1791275930650).UTC(), Pair: perpetualPair},
	}
	assert.Equal(t, exp, orders, "GetOrderHistory should convert the perpetual orders")
	exp = order.FilteredOrders{
		{TimeInForce: order.GoodTillCancel, Price: 90, Amount: 2, RemainingAmount: 2, Exchange: e.Name, OrderID: "566563303541", Type: order.Limit, Side: order.Buy, Status: order.Cancelled, AssetType: asset.Spot, Date: placed, LastUpdated: time.UnixMilli(1791275931000).UTC(), Pair: spotPair},
	}
	assert.Equal(t, exp, spotOrders, "GetOrderHistory should convert the spot orders")
}

func TestOpenOrderDEXes(t *testing.T) {
	t.Parallel()
	ex := newTradingServerExchange(t, nil, nil)
	_, err := ex.openOrderDEXes(t.Context(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "openOrderDEXes must reject an unsupported asset")
	dexes, err := ex.openOrderDEXes(t.Context(), asset.Spot)
	require.NoError(t, err, "openOrderDEXes must not error for spot")
	assert.Equal(t, []string{""}, dexes, "openOrderDEXes should return the first DEX, which lists spot orders, for spot")
	dexes, err = ex.openOrderDEXes(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "openOrderDEXes must not error for perpetuals")
	assert.Equal(t, []string{"", "xyz"}, dexes, "openOrderDEXes should return every DEX for perpetuals")
	_, err = newUnavailableServerExchange(t).openOrderDEXes(t.Context(), asset.PerpetualContract)
	assert.Error(t, err, "openOrderDEXes should return a registry failure")
}

func TestConvertOpenOrder(t *testing.T) {
	t.Parallel()
	ex := newTradingServerExchange(t, nil, nil)
	_, err := ex.convertOpenOrder(t.Context(), &BasicOrder{Coin: "MISSING", Side: "B"})
	assert.ErrorIs(t, err, errPairMappingNotFound, "convertOpenOrder should reject an order without a market")
	detail, err := ex.convertOpenOrder(t.Context(), &BasicOrder{Coin: "@107", Side: "A", LimitPrice: 95.5, Size: 1, OrderID: 9, Timestamp: milli(1791275926947), OriginalSize: 1.5, ClientOrderID: testClientOrderID})
	require.NoError(t, err, "convertOpenOrder must not error")
	placed := time.UnixMilli(1791275926947).UTC()
	exp := order.Detail{Price: 95.5, Amount: 1.5, ExecutedAmount: 0.5, RemainingAmount: 1, Exchange: ex.Name, OrderID: "9", ClientOrderID: testClientOrderID, Side: order.Sell, Status: order.Open, AssetType: asset.Spot, Date: placed, LastUpdated: placed, Pair: spotPair}
	assert.Equal(t, exp, detail, "convertOpenOrder should convert the order without its order type or time in force")
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	_, err := e.GetAvailableTransferChains(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "GetAvailableTransferChains must require a currency")
	_, err = e.GetAvailableTransferChains(t.Context(), currency.BTC)
	require.ErrorIs(t, err, errTransferCurrencyInvalid, "GetAvailableTransferChains must reject a currency the bridge does not transfer")
	chains, err := e.GetAvailableTransferChains(t.Context(), currency.USDC)
	require.NoError(t, err, "GetAvailableTransferChains must not error")
	assert.Equal(t, []string{"Arbitrum"}, chains, "GetAvailableTransferChains should return the mainnet bridge chain")
	sandbox := &Exchange{}
	sandbox.Config = &config.Exchange{UseSandbox: true}
	chains, err = sandbox.GetAvailableTransferChains(t.Context(), currency.USDC)
	require.NoError(t, err, "GetAvailableTransferChains must not error in sandbox mode")
	assert.Equal(t, []string{"Arbitrum Sepolia"}, chains, "GetAvailableTransferChains should return the testnet bridge chain")
}

func TestWithdrawCryptocurrencyFunds(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawCryptocurrencyFunds(t.Context(), nil)
	require.ErrorIs(t, err, withdraw.ErrRequestCannotBeNil, "WithdrawCryptocurrencyFunds must reject a nil request")
	newRequest := func() *withdraw.Request {
		return &withdraw.Request{Exchange: e.Name, Currency: currency.USDC, Amount: 25, Type: withdraw.Crypto, Crypto: withdraw.CryptoRequest{Address: testWithdrawalAddress}}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*withdraw.Request)
		err    error
	}{
		{name: "currency", mutate: func(r *withdraw.Request) { r.Currency = currency.BTC }, err: errTransferCurrencyInvalid},
		{name: "address tag", mutate: func(r *withdraw.Request) { r.Crypto.AddressTag = "tag" }, err: errWithdrawalAddressTag},
		{name: "fee", mutate: func(r *withdraw.Request) { r.Crypto.FeeAmount = 1 }, err: errWithdrawalFeeInput},
		{name: "chain", mutate: func(r *withdraw.Request) { r.Crypto.Chain = "Ethereum" }, err: errBridgeChainInvalid},
		{name: "address", mutate: func(r *withdraw.Request) { r.Crypto.Address = "invalid" }, err: errInvalidAddress},
	} {
		withdrawal := newRequest()
		tc.mutate(withdrawal)
		_, err := e.WithdrawCryptocurrencyFunds(t.Context(), withdrawal)
		assert.ErrorIsf(t, err, tc.err, "WithdrawCryptocurrencyFunds should reject an invalid %s", tc.name)
	}
	_, err = new(Exchange).WithdrawCryptocurrencyFunds(t.Context(), newRequest())
	require.Error(t, err, "WithdrawCryptocurrencyFunds must require credentials")
	_, err = newUnavailableServerExchange(t).WithdrawCryptocurrencyFunds(t.Context(), newRequest())
	require.Error(t, err, "WithdrawCryptocurrencyFunds must return an action failure")

	skipUnlessMockTesting(t)
	for _, internal := range []bool{false, true} {
		withdrawal := newRequest()
		withdrawal.InternalTransfer = internal
		withdrawal.Crypto.Chain = "arbitrum"
		ex := newUserSignedTestExchange(t)
		result, err := ex.WithdrawCryptocurrencyFunds(t.Context(), withdrawal)
		require.NoErrorf(t, err, "WithdrawCryptocurrencyFunds must not error for an internal transfer %t", internal)
		if mockTests {
			exp := &withdraw.ExchangeResponse{Name: ex.Name, ID: strconv.FormatUint(mockUserSignedNonce, 10), Status: "submitted"}
			assert.Equalf(t, exp, result, "WithdrawCryptocurrencyFunds should return the action's nonce for an internal transfer %t", internal)
		}
	}
}

func TestGetFeeByType(t *testing.T) {
	t.Parallel()
	_, err := e.GetFeeByType(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetFeeByType must reject a nil fee builder")
	_, err = e.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.InternationalBankWithdrawalFee, Pair: perpetualPair})
	require.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetFeeByType must reject an unsupported fee type")
	_, err = e.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.OfflineTradeFee})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetFeeByType must require a pair")
	for _, notional := range [][2]float64{{-1, 1}, {1, -1}, {math.NaN(), 1}, {1, math.NaN()}, {math.Inf(1), 1}, {1, math.Inf(1)}} {
		_, err = e.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.OfflineTradeFee, Pair: perpetualPair, PurchasePrice: notional[0], Amount: notional[1]})
		assert.ErrorIsf(t, err, order.ErrAmountIsInvalid, "GetFeeByType should reject price %v and amount %v", notional[0], notional[1])
	}

	offline := new(Exchange)
	require.NoError(t, testexch.Setup(offline), "Setup must not error")
	for _, tc := range []struct {
		pair    currency.Pair
		isMaker bool
		exp     float64
	}{
		{pair: perpetualPair, exp: 100 * 2 * perpetualTakerBaseFeeRate},
		{pair: perpetualPair, isMaker: true, exp: 100 * 2 * perpetualMakerBaseFeeRate},
		{pair: spotPair, exp: 100 * 2 * spotTakerBaseFeeRate},
		{pair: spotPair, isMaker: true, exp: 100 * 2 * spotMakerBaseFeeRate},
	} {
		builder := &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, Pair: tc.pair, IsMaker: tc.isMaker, PurchasePrice: 100, Amount: 2}
		fee, err := offline.GetFeeByType(t.Context(), builder)
		require.NoErrorf(t, err, "GetFeeByType must not error for %s maker %t without credentials", tc.pair, tc.isMaker)
		assert.InDeltaf(t, tc.exp, fee, 1e-12, "GetFeeByType should use the base %s rate for maker %t", tc.pair, tc.isMaker)
		assert.Equalf(t, exchange.OfflineTradeFee, builder.FeeType, "GetFeeByType should estimate offline without credentials for %s", tc.pair)
	}

	unmapped := newInfoServerExchange(t, nil, nil)
	unmapped.CurrencyPairs.Pairs = make(map[asset.Item]*currency.PairStore)
	_, err = unmapped.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.OfflineTradeFee, Pair: perpetualPair})
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetFeeByType should reject a pair of neither asset")
	unmapped.setPairMappings(asset.Spot, []pairMapping{{pair: perpetualPair, coin: "@1"}})
	unmapped.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = unmapped.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.OfflineTradeFee, Pair: perpetualPair})
	assert.ErrorIs(t, err, errAmbiguousCoinMapping, "GetFeeByType should reject a pair traded by both assets")
	unmapped.setPairMappings(asset.Spot, nil)
	fee, err := unmapped.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.OfflineTradeFee, Pair: perpetualPair, PurchasePrice: 100, Amount: 2})
	require.NoError(t, err, "GetFeeByType must not error for a mapped pair that is not enabled")
	assert.InDelta(t, 100*2*perpetualTakerBaseFeeRate, fee, 1e-12, "GetFeeByType should identify the asset of a mapped pair")

	online := newInfoServerExchange(t, map[string]string{"userFees": `{"userCrossRate":"0.000315","userAddRate":"0.000105","userSpotCrossRate":"0.00049","userSpotAddRate":"0.00028"}`}, nil)
	for _, tc := range []struct {
		pair    currency.Pair
		isMaker bool
		rate    float64
	}{
		{pair: perpetualPair, rate: 0.000315},
		{pair: perpetualPair, isMaker: true, rate: 0.000105},
		{pair: spotPair, rate: 0.00049},
		{pair: spotPair, isMaker: true, rate: 0.00028},
	} {
		accountFee, err := online.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, Pair: tc.pair, IsMaker: tc.isMaker, PurchasePrice: 100, Amount: 2})
		require.NoErrorf(t, err, "GetFeeByType must not error for %s maker %t", tc.pair, tc.isMaker)
		assert.InDeltaf(t, 100*2*tc.rate, accountFee, 1e-12, "GetFeeByType should use the account's %s rate for maker %t", tc.pair, tc.isMaker)
	}
	_, err = newUnavailableServerExchange(t).GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, Pair: perpetualPair})
	assert.Error(t, err, "GetFeeByType should return a fee failure")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	fee, err = e.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, Pair: perpetualPair, PurchasePrice: 100, Amount: 2})
	require.NoError(t, err, "GetFeeByType must not error")
	assert.Positive(t, fee, "GetFeeByType should return the fee")
}

func TestValidateAPICredentials(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.ValidateAPICredentials(t.Context(), asset.Options), asset.ErrNotSupported, "ValidateAPICredentials must reject an unsupported asset")
	require.ErrorIs(t, new(Exchange).ValidateAPICredentials(t.Context(), asset.Empty), request.ErrAuthRequestFailed, "ValidateAPICredentials must require credentials")
	missing := newInfoServerExchange(t, map[string]string{"userRole": `{"role":"missing"}`}, nil)
	err := missing.ValidateAPICredentials(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, errConfiguredAccountMissing, "ValidateAPICredentials should report a missing account")
	assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "ValidateAPICredentials should report a missing account as an authentication failure")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	assert.NoError(t, e.ValidateAPICredentials(t.Context(), asset.PerpetualContract), "ValidateAPICredentials should not error for the configured account")
}

func TestSetLeverage(t *testing.T) {
	t.Parallel()
	ethPair := currency.NewPair(currency.ETH, currency.USDC)
	isolatedPair := currency.NewPair(currency.NewCode("HPOS"), currency.USDC)
	var action string
	ex := newTradingServerExchange(t, map[string]string{
		"meta:": `{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":40},{"name":"ETH"},{"name":"OLD","isDelisted":true},{"name":"HPOS","maxLeverage":3,"onlyIsolated":true}]}`,
	}, captureAction(t, &action, `{"status":"ok","response":{"type":"default"}}`))
	for _, tc := range []struct {
		a          asset.Item
		pair       currency.Pair
		marginType margin.Type
		amount     float64
		err        error
	}{
		{a: asset.Spot, pair: spotPair, marginType: margin.Multi, amount: 1, err: asset.ErrNotSupported},
		{a: asset.PerpetualContract, pair: currency.NewPair(currency.SOL, currency.USDC), marginType: margin.Multi, amount: 1, err: errPairMappingNotFound},
		{a: asset.PerpetualContract, pair: perpetualPair, marginType: margin.Multi, amount: 0, err: errInvalidLeverage},
		{a: asset.PerpetualContract, pair: perpetualPair, marginType: margin.Multi, amount: -1, err: errInvalidLeverage},
		{a: asset.PerpetualContract, pair: perpetualPair, marginType: margin.Multi, amount: 1.5, err: errInvalidLeverage},
		{a: asset.PerpetualContract, pair: perpetualPair, marginType: margin.Multi, amount: math.NaN(), err: errInvalidLeverage},
		{a: asset.PerpetualContract, pair: perpetualPair, marginType: margin.Multi, amount: math.Inf(1), err: errInvalidLeverage},
		{a: asset.PerpetualContract, pair: perpetualPair, marginType: margin.Multi, amount: 41, err: errInvalidLeverage},
		{a: asset.PerpetualContract, pair: ethPair, marginType: margin.Multi, amount: 1, err: errInvalidLeverage},
		{a: asset.PerpetualContract, pair: perpetualPair, marginType: margin.NoMargin, amount: 1, err: margin.ErrMarginTypeUnsupported},
		{a: asset.PerpetualContract, pair: isolatedPair, marginType: margin.Multi, amount: 1, err: errCrossMarginUnavailable},
	} {
		err := ex.SetLeverage(t.Context(), tc.a, tc.pair, tc.marginType, tc.amount, order.UnknownSide)
		assert.ErrorIsf(t, err, tc.err, "SetLeverage should reject %s %s %s leverage %v", tc.a, tc.pair, tc.marginType, tc.amount)
	}

	for _, tc := range []struct {
		pair       currency.Pair
		marginType margin.Type
		amount     float64
		exp        string
	}{
		{pair: perpetualPair, marginType: margin.Unset, amount: 10, exp: `{"type":"updateLeverage","asset":0,"isCross":true,"leverage":10}`},
		{pair: perpetualPair, marginType: margin.Multi, amount: 20, exp: `{"type":"updateLeverage","asset":0,"isCross":true,"leverage":20}`},
		{pair: perpetualPair, marginType: margin.Isolated, amount: 40, exp: `{"type":"updateLeverage","asset":0,"isCross":false,"leverage":40}`},
		{pair: isolatedPair, marginType: margin.Isolated, amount: 3, exp: `{"type":"updateLeverage","asset":3,"isCross":false,"leverage":3}`},
		{pair: testBuilderPair, marginType: margin.Multi, amount: 30, exp: `{"type":"updateLeverage","asset":110000,"isCross":true,"leverage":30}`},
	} {
		require.NoErrorf(t, ex.SetLeverage(t.Context(), asset.PerpetualContract, tc.pair, tc.marginType, tc.amount, order.UnknownSide), "SetLeverage must not error for %s %s", tc.pair, tc.marginType)
		assert.JSONEqf(t, tc.exp, action, "SetLeverage should send the leverage of %s %s", tc.pair, tc.marginType)
	}
	err := newTradingServerExchange(t, nil, respondWith(`{"status":"err","response":"leverage update failed"}`)).SetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, margin.Multi, 1, order.UnknownSide)
	assert.ErrorIs(t, err, errActionResponse, "SetLeverage should return a rejected action")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	assert.NoError(t, e.SetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, margin.Multi, 20, order.UnknownSide), "SetLeverage should not error")
}

func TestGetLeverage(t *testing.T) {
	t.Parallel()
	_, err := e.GetLeverage(t.Context(), asset.Spot, spotPair, margin.Unset, order.UnknownSide)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetLeverage must reject spot")
	_, err = e.GetLeverage(t.Context(), asset.PerpetualContract, currency.EMPTYPAIR, margin.Unset, order.UnknownSide)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetLeverage must require a pair")
	unconfigured := new(Exchange)
	unconfigured.SetDefaults()
	unconfigured.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = unconfigured.GetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, margin.Unset, order.UnknownSide)
	require.Error(t, err, "GetLeverage must require an account address")

	for _, tc := range []struct {
		leverage   string
		marginType margin.Type
		exp        float64
		err        error
	}{
		{leverage: `{"type":"cross","value":20}`, marginType: margin.Unset, exp: 20},
		{leverage: `{"type":"cross","value":20}`, marginType: margin.Multi, exp: 20},
		{leverage: `{"type":"cross","value":20}`, marginType: margin.Isolated, err: margin.ErrMarginTypeUnsupported},
		{leverage: `{"type":"isolated","value":5,"rawUsd":"-95.06"}`, marginType: margin.Isolated, exp: 5},
		{leverage: `{"type":"isolated","value":5,"rawUsd":"-95.06"}`, marginType: margin.Multi, err: margin.ErrMarginTypeUnsupported},
		{leverage: `{"type":"portfolio","value":5}`, marginType: margin.Unset, err: margin.ErrMarginTypeUnsupported},
		{leverage: `{"type":"cross","value":0}`, marginType: margin.Unset, err: errInvalidLeverage},
	} {
		ex := newTradingServerExchange(t, map[string]string{"activeAssetData": `{"user":"` + testAccountAddress + `","coin":"BTC","leverage":` + tc.leverage + `}`}, nil)
		value, err := ex.GetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, tc.marginType, order.UnknownSide)
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "GetLeverage should reject leverage %s as %s", tc.leverage, tc.marginType)
			continue
		}
		require.NoErrorf(t, err, "GetLeverage must not error for leverage %s as %s", tc.leverage, tc.marginType)
		assert.Equalf(t, tc.exp, value, "GetLeverage should return leverage %s as %s", tc.leverage, tc.marginType)
	}
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = failed.GetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, margin.Unset, order.UnknownSide)
	assert.Error(t, err, "GetLeverage should return an asset data failure")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	value, err := e.GetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, margin.Unset, order.UnknownSide)
	require.NoError(t, err, "GetLeverage must not error")
	if mockTests {
		assert.Equal(t, 20.0, value, "GetLeverage should return the account's leverage")
	} else {
		assert.Positive(t, value, "GetLeverage should return the account's leverage")
	}
}

func TestGetLatestFundingRates(t *testing.T) {
	t.Parallel()
	_, err := e.GetLatestFundingRates(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetLatestFundingRates must reject a nil request")
	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.Spot})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetLatestFundingRates must reject spot")
	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract, IncludePredictedRate: true})
	require.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetLatestFundingRates must reject predicted rates")

	rates, err := e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract, Pair: perpetualPair})
	require.NoError(t, err, "GetLatestFundingRates must not error for a pair")
	require.Len(t, rates, 1, "GetLatestFundingRates must return the pair's rate")
	checked := rates[0].TimeChecked
	assert.WithinDuration(t, time.Now(), checked, time.Minute, "GetLatestFundingRates should time the check when it is made")
	exp := fundingrate.LatestRateResponse{
		Exchange:       e.Name,
		Asset:          asset.PerpetualContract,
		Pair:           perpetualPair,
		LatestRate:     fundingrate.Rate{Time: checked.Truncate(time.Hour), Rate: rates[0].LatestRate.Rate},
		TimeOfNextRate: checked.Truncate(time.Hour).Add(time.Hour),
		TimeChecked:    checked,
	}
	if mockTests {
		exp.LatestRate.Rate = decimal.MustFromFloat(0.0000125)
	}
	assert.Equal(t, exp, rates[0], "GetLatestFundingRates should return the current hour's rate")

	rates, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	require.NoError(t, err, "GetLatestFundingRates must not error for every pair")
	if mockTests {
		exp := map[currency.Pair]decimal.Decimal{
			perpetualPair: decimal.MustFromFloat(0.0000125),
			currency.NewPair(currency.ETH, currency.USDC):             decimal.MustFromFloat(0.0000125),
			currency.NewPair(currency.NewCode("HPOS"), currency.USDC): decimal.MustFromFloat(0.0000125),
			testBuilderPair: decimal.MustFromFloat(0.00000625),
			currency.NewPair(currency.NewCode("xyz:TSLA"), currency.USDC):            decimal.MustFromFloat(0.00000625),
			currency.NewPair(currency.NewCode("flx:TSLA"), currency.NewCode("USDH")): decimal.MustFromFloat(0.0000125),
		}
		result := make(map[currency.Pair]decimal.Decimal, len(rates))
		for i := range rates {
			result[rates[i].Pair] = rates[i].LatestRate.Rate
		}
		assert.Equal(t, exp, result, "GetLatestFundingRates should return every DEX's rates")
	} else {
		assert.NotEmpty(t, rates, "GetLatestFundingRates should return every market's rate")
	}

	empty := newInfoServerExchange(t, map[string]string{"meta": `{"universe":[]}`}, nil)
	_, err = empty.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	assert.ErrorIs(t, err, fundingrate.ErrNoFundingRatesFound, "GetLatestFundingRates should report no markets")
	_, err = empty.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract, Pair: perpetualPair})
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetLatestFundingRates should report a pair without a market")
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = failed.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	assert.Error(t, err, "GetLatestFundingRates should return a context failure")
	_, err = newUnavailableServerExchange(t).GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.PerpetualContract})
	assert.Error(t, err, "GetLatestFundingRates should return a metadata failure")
}

func TestGetHistoricalFundingRates(t *testing.T) {
	t.Parallel()
	start, end := getTime()
	_, err := e.GetHistoricalFundingRates(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetHistoricalFundingRates must reject a nil request")
	for _, tc := range []struct {
		arg fundingrate.HistoricalRatesRequest
		err error
	}{
		{arg: fundingrate.HistoricalRatesRequest{Asset: asset.Spot}, err: asset.ErrNotSupported},
		{arg: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract}, err: currency.ErrCurrencyPairEmpty},
		{arg: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, IncludePredictedRate: true}, err: common.ErrFunctionNotSupported},
		{arg: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, IncludePayments: true}, err: common.ErrFunctionNotSupported},
		{arg: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: end, EndDate: start}, err: common.ErrStartAfterEnd},
		{arg: fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: start, EndDate: end, PaymentCurrency: currency.BTC}, err: asset.ErrNotSupported},
	} {
		_, err := e.GetHistoricalFundingRates(t.Context(), &tc.arg)
		assert.ErrorIsf(t, err, tc.err, "GetHistoricalFundingRates should reject %+v", tc.arg)
	}

	result, err := e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: start, EndDate: end, PaymentCurrency: currency.USDC})
	require.NoError(t, err, "GetHistoricalFundingRates must not error")
	if mockTests {
		rates := []fundingrate.Rate{
			{Time: time.UnixMilli(1791255600057).UTC(), Rate: decimal.MustFromFloat(0.0000125)},
			{Time: time.UnixMilli(1791259200041).UTC(), Rate: decimal.MustFromFloat(-0.0000032)},
		}
		exp := &fundingrate.HistoricalRates{
			Exchange:        e.Name,
			Asset:           asset.PerpetualContract,
			Pair:            perpetualPair,
			StartDate:       start,
			EndDate:         end,
			LatestRate:      rates[1],
			FundingRates:    rates,
			PaymentCurrency: currency.USDC,
		}
		assert.Equal(t, exp, result, "GetHistoricalFundingRates should return every hourly rate in the range")
	} else {
		assert.NotEmpty(t, result.FundingRates, "GetHistoricalFundingRates should return rates")
	}

	pageStart := time.UnixMilli(mockStartTime).Add(-(maximumFundingHistoryCount + 2) * time.Hour)
	pageEnd := pageStart.Add((maximumFundingHistoryCount + 1) * time.Hour)
	var pages atomic.Int32
	paged := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body InfoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
			return
		}
		count := 2
		if pages.Add(1) == 1 {
			count = maximumFundingHistoryCount
		}
		records := make([]string, count)
		for i := range records {
			records[i] = fmt.Sprintf(`{"coin":"BTC","fundingRate":"0.0001","premium":"0.0","time":%d}`, body.StartTime+int64(i)*time.Hour.Milliseconds())
		}
		_, err := w.Write([]byte("[" + strings.Join(records, ",") + "]"))
		assert.NoError(t, err, "Writing the funding page should not error")
	}))
	paged.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	result, err = paged.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: pageStart, EndDate: pageEnd})
	require.NoError(t, err, "GetHistoricalFundingRates must not error across pages")
	assert.Len(t, result.FundingRates, maximumFundingHistoryCount+2, "GetHistoricalFundingRates should return every page's rates")
	assert.Equal(t, int32(2), pages.Load(), "GetHistoricalFundingRates should request the next page after a full page")

	builder := newTradingServerExchange(t, map[string]string{"fundingHistory": `[{"coin":"xyz:XYZ100","fundingRate":"0.0002","premium":"0.0001","time":` + strconv.FormatInt(pageStart.UnixMilli(), 10) + `}]`}, nil)
	builder.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.HYPE), coin: "xyz:XYZ100", dex: "xyz"}})
	builderRequest := &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.HYPE), StartDate: pageStart, EndDate: pageEnd}
	result, err = builder.GetHistoricalFundingRates(t.Context(), builderRequest)
	require.NoError(t, err, "GetHistoricalFundingRates must not error for a builder DEX")
	assert.Equal(t, currency.HYPE, result.PaymentCurrency, "GetHistoricalFundingRates should pay funding in the DEX's collateral")
	builderRequest.PaymentCurrency = currency.USDC
	_, err = builder.GetHistoricalFundingRates(t.Context(), builderRequest)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetHistoricalFundingRates should reject a payment currency other than the DEX's collateral")

	record := `{"coin":"BTC","fundingRate":"0.0001","time":` + strconv.FormatInt(pageStart.UnixMilli(), 10) + `}`
	for _, tc := range []struct {
		name    string
		history string
		err     error
	}{
		{name: "empty history", history: `[]`, err: fundingrate.ErrNoFundingRatesFound},
		{name: "oversized page", history: `[` + strings.Repeat(record+",", maximumFundingHistoryCount) + record + `]`, err: errUnexpectedResponseLength},
		{name: "page of another coin", history: `[{"coin":"ETH","fundingRate":"0.1","time":` + strconv.FormatInt(pageStart.UnixMilli(), 10) + `}]`, err: errUnexpectedResponseLength},
		{name: "repeated time", history: `[` + record + `,` + record + `]`, err: errUnexpectedResponseLength},
		{name: "record outside the range", history: `[{"coin":"BTC","fundingRate":"0.1","time":` + strconv.FormatInt(pageEnd.Add(time.Hour).UnixMilli(), 10) + `}]`, err: errUnexpectedResponseLength},
	} {
		ex := newTradingServerExchange(t, map[string]string{"fundingHistory": tc.history}, nil)
		_, err := ex.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: pageStart, EndDate: pageEnd})
		assert.ErrorIsf(t, err, tc.err, "GetHistoricalFundingRates should reject an %s", tc.name)
	}
	_, err = newInfoServerExchange(t, map[string]string{"meta": `{"universe":[]}`}, nil).GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: pageStart, EndDate: pageEnd})
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetHistoricalFundingRates should report a pair without a market")
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = failed.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: pageStart, EndDate: pageEnd})
	assert.Error(t, err, "GetHistoricalFundingRates should return a history failure")
}

func TestGetOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := e.GetOpenInterest(t.Context(), key.PairAsset{Base: spotPair.Base.Item, Quote: spotPair.Quote.Item, Asset: asset.Spot})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetOpenInterest must reject spot")

	result, err := e.GetOpenInterest(t.Context(), key.PairAsset{Base: perpetualPair.Base.Item, Quote: perpetualPair.Quote.Item, Asset: asset.PerpetualContract})
	require.NoError(t, err, "GetOpenInterest must not error for a pair")
	require.Len(t, result, 1, "GetOpenInterest must return the pair's open interest")
	if mockTests {
		exp := []futures.OpenInterest{{Key: key.NewExchangeAssetPair(e.Name, asset.PerpetualContract, perpetualPair), OpenInterest: 39310.43958}}
		assert.Equal(t, exp, result, "GetOpenInterest should return the pair's open interest")
	} else {
		assert.Positive(t, result[0].OpenInterest, "GetOpenInterest should return the pair's open interest")
	}
	result, err = e.GetOpenInterest(t.Context())
	require.NoError(t, err, "GetOpenInterest must not error for every pair")
	if mockTests {
		assert.Len(t, result, 6, "GetOpenInterest should return every DEX's active markets")
	} else {
		assert.NotEmpty(t, result, "GetOpenInterest should return every market's open interest")
	}

	empty := newInfoServerExchange(t, map[string]string{"meta": `{"universe":[]}`}, nil)
	result, err = empty.GetOpenInterest(t.Context())
	require.NoError(t, err, "GetOpenInterest must not error without markets")
	assert.Empty(t, result, "GetOpenInterest should return nothing without markets")
	_, err = empty.GetOpenInterest(t.Context(), key.PairAsset{Base: perpetualPair.Base.Item, Quote: perpetualPair.Quote.Item, Asset: asset.PerpetualContract})
	assert.ErrorIs(t, err, errPairMappingNotFound, "GetOpenInterest should report a pair without a market")
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, err = failed.GetOpenInterest(t.Context())
	assert.Error(t, err, "GetOpenInterest should return a context failure")
	_, err = newUnavailableServerExchange(t).GetOpenInterest(t.Context())
	assert.Error(t, err, "GetOpenInterest should return a metadata failure")
}

func TestBuildOrderRequest(t *testing.T) {
	t.Parallel()
	ex := newTradingServerExchange(t, map[string]string{"allMids": `{"BTC":"100","@107":"10"}`, "allMids:xyz": `{"xyz:XYZ100":"50"}`}, nil)
	for _, tc := range []struct {
		name         string
		pair         currency.Pair
		a            asset.Item
		orderType    order.Type
		side         order.Side
		timeInForce  order.TimeInForce
		amount       float64
		price        float64
		triggerPrice float64
		slippage     float64
		reduceOnly   bool
		exp          OrderRequest
	}{
		{
			name: "limit", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Limit, side: order.Buy, timeInForce: order.GoodTillCancel, amount: 0.12345, price: 100.5, reduceOnly: true,
			exp: OrderRequest{Asset: 0, IsBuy: true, Price: 100.5, Size: 0.12345, ReduceOnly: true, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}, ClientOrderID: testClientOrderID},
		},
		{
			name: "market buy", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Market, side: order.Buy, amount: 0.1, slippage: 0.01,
			exp: OrderRequest{IsBuy: true, Price: 101, Size: 0.1, Limit: &LimitOrderType{TimeInForce: TimeInForceIOC}, ClientOrderID: testClientOrderID},
		},
		{
			name: "market sell", pair: spotPair, a: asset.Spot, orderType: order.Market, side: order.Sell, amount: 1, slippage: 0.01,
			exp: OrderRequest{Asset: 10107, Price: 9.9, Size: 1, Limit: &LimitOrderType{TimeInForce: TimeInForceIOC}, ClientOrderID: testClientOrderID},
		},
		{
			name: "builder DEX market", pair: testBuilderPair, a: asset.PerpetualContract, orderType: order.Market, side: order.Buy, amount: 1, slippage: 0.01,
			exp: OrderRequest{Asset: 110000, IsBuy: true, Price: 50.5, Size: 1, Limit: &LimitOrderType{TimeInForce: TimeInForceIOC}, ClientOrderID: testClientOrderID},
		},
		{
			name: "stop market", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.StopMarket, side: order.Sell, amount: 0.1, triggerPrice: 90, slippage: 0.1, reduceOnly: true,
			exp: OrderRequest{Price: 81, Size: 0.1, ReduceOnly: true, Trigger: &TriggerOrderType{IsMarket: true, TriggerPrice: 90, TakeProfitStopLoss: TriggerStopLoss}, ClientOrderID: testClientOrderID},
		},
		{
			name: "take profit limit", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.TakeProfit, side: order.Buy, amount: 0.1, price: 111, triggerPrice: 110, reduceOnly: true,
			exp: OrderRequest{IsBuy: true, Price: 111, Size: 0.1, ReduceOnly: true, Trigger: &TriggerOrderType{TriggerPrice: 110, TakeProfitStopLoss: TriggerTakeProfit}, ClientOrderID: testClientOrderID},
		},
	} {
		result, _, err := ex.buildOrderRequest(t.Context(), tc.pair, tc.a, tc.orderType, tc.side, tc.timeInForce, tc.amount, tc.price, tc.triggerPrice, tc.slippage, tc.reduceOnly, strings.ToUpper(testClientOrderID[:2])+testClientOrderID[2:])
		require.NoErrorf(t, err, "buildOrderRequest must not error for a %s order", tc.name)
		assert.Equalf(t, tc.exp, result, "buildOrderRequest should build the %s order", tc.name)
	}

	for _, tc := range []struct {
		name         string
		pair         currency.Pair
		a            asset.Item
		orderType    order.Type
		timeInForce  order.TimeInForce
		amount       float64
		price        float64
		triggerPrice float64
		slippage     float64
		reduceOnly   bool
		clientID     string
		err          error
	}{
		{name: "pair without a market", pair: currency.NewPair(currency.SOL, currency.USDC), a: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: 1, err: errPairMappingNotFound},
		{name: "size beyond the market's precision", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Limit, amount: 0.000001, price: 1, err: errSizePrecision},
		{name: "invalid client order ID", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: 1, clientID: "invalid", err: errClientOrderIDInvalid},
		{name: "price beyond the market's precision", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: 100.55, err: errPricePrecision},
		{name: "unsupported time in force", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Limit, timeInForce: order.FillOrKill, amount: 1, price: 1, err: order.ErrUnsupportedTimeInForce},
		{name: "limit trigger price", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: 1, triggerPrice: 2, err: errRiskManagementUnsupported},
		{name: "market trigger price", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Market, amount: 1, triggerPrice: 2, slippage: 0.1, err: errRiskManagementUnsupported},
		{name: "zero slippage", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Market, amount: 1, err: errSlippageTolerance},
		{name: "whole slippage", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Market, amount: 1, slippage: 1, err: errSlippageTolerance},
		{name: "spot trigger", pair: spotPair, a: asset.Spot, orderType: order.StopMarket, amount: 1, triggerPrice: 10, slippage: 0.1, reduceOnly: true, err: errTriggerOrderReduceOnly},
		{name: "increasing trigger", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, triggerPrice: 10, slippage: 0.1, err: errTriggerOrderReduceOnly},
		{name: "missing trigger price", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, slippage: 0.1, reduceOnly: true, err: errTriggerPriceRequired},
		{name: "trigger price beyond the market's precision", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, triggerPrice: 100.55, slippage: 0.1, reduceOnly: true, err: errPricePrecision},
		{name: "trigger market without slippage", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, triggerPrice: 100, reduceOnly: true, err: errSlippageTolerance},
		{name: "unbounded trigger market price", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, triggerPrice: math.MaxFloat64, slippage: 0.9, reduceOnly: true, err: errInvalidMarketPrice},
		{name: "trigger limit without price", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.StopLimit, amount: 1, triggerPrice: 100, reduceOnly: true, err: errInvalidMarketPrice},
		{name: "unsupported type", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.TrailingStop, amount: 1, price: 1, err: order.ErrTypeIsInvalid},
		{name: "NaN price", pair: perpetualPair, a: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: math.NaN(), err: errInvalidMarketPrice},
	} {
		_, _, err := ex.buildOrderRequest(t.Context(), tc.pair, tc.a, tc.orderType, order.Buy, tc.timeInForce, tc.amount, tc.price, tc.triggerPrice, tc.slippage, tc.reduceOnly, tc.clientID)
		assert.ErrorIsf(t, err, tc.err, "buildOrderRequest should reject a %s", tc.name)
	}

	for _, mids := range []string{`{}`, `{"BTC":"0"}`} {
		_, _, err := newTradingServerExchange(t, map[string]string{"allMids": mids}, nil).buildOrderRequest(t.Context(), perpetualPair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 1, 0, 0, 0.01, false, "")
		assert.ErrorIsf(t, err, errMarketMidPriceNotFound, "buildOrderRequest should reject mid prices %s", mids)
	}
	failed := newUnavailableServerExchange(t)
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	_, _, err := failed.buildOrderRequest(t.Context(), perpetualPair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 1, 0, 0, 0.01, false, "")
	assert.Error(t, err, "buildOrderRequest should return a mid price failure")
	unpriceable := newTradingServerExchange(t, map[string]string{"allMids": `{"BTC":"100"}`}, nil)
	unpriceable.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: perpetualPair, coin: "BTC", sizeDecimals: 7}})
	_, _, err = unpriceable.buildOrderRequest(t.Context(), perpetualPair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 1, 0, 0, 0.01, false, "")
	assert.ErrorIs(t, err, errSizePrecision, "buildOrderRequest should reject a market whose size decimals leave no price decimals")
}

func TestBuildOrderRequests(t *testing.T) {
	t.Parallel()
	ex := newTradingServerExchange(t, nil, nil)
	submit := &order.Submit{
		Exchange:      ex.Name,
		Type:          order.Limit,
		Side:          order.Buy,
		Pair:          perpetualPair,
		AssetType:     asset.PerpetualContract,
		TimeInForce:   order.GoodTillCancel,
		Amount:        0.1,
		Price:         100,
		ClientOrderID: testClientOrderID,
	}
	parent := OrderRequest{IsBuy: true, Price: 100, Size: 0.1, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}, ClientOrderID: testClientOrderID}
	orders, mapping, grouping, err := ex.buildOrderRequests(t.Context(), submit)
	require.NoError(t, err, "buildOrderRequests must not error")
	assert.Equal(t, []OrderRequest{parent}, orders, "buildOrderRequests should build the order")
	assert.Equal(t, testPerpetualMapping, mapping, "buildOrderRequests should return the market's mapping")
	assert.Equal(t, GroupingNone, grouping, "buildOrderRequests should not group an order without children")

	trigger := &order.Submit{Exchange: ex.Name, Type: order.StopMarket, Side: order.Sell, Pair: perpetualPair, AssetType: asset.PerpetualContract, Amount: 0.1, TriggerPrice: 90, TriggerPriceType: order.LastPrice, SlippageTolerance: 0.1, ReduceOnly: true}
	orders, _, _, err = ex.buildOrderRequests(t.Context(), trigger)
	require.ErrorIs(t, err, errRiskManagementUnsupported, "buildOrderRequests must reject a trigger on the last price")
	assert.Nil(t, orders, "buildOrderRequests should not build a rejected order")

	grouped := *submit
	grouped.RiskManagementModes = order.RiskManagementModes{
		Mode:       GroupingNormalTPSL,
		TakeProfit: order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110, OrderType: order.Market},
		StopLoss:   order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 90, LimitPrice: 89, OrderType: order.StopLimit},
	}
	orders, _, grouping, err = ex.buildOrderRequests(t.Context(), &grouped)
	require.NoError(t, err, "buildOrderRequests must not error with children")
	exp := []OrderRequest{
		parent,
		{Price: 99, Size: 0.1, ReduceOnly: true, Trigger: &TriggerOrderType{IsMarket: true, TriggerPrice: 110, TakeProfitStopLoss: TriggerTakeProfit}},
		{Price: 89, Size: 0.1, ReduceOnly: true, Trigger: &TriggerOrderType{TriggerPrice: 90, TakeProfitStopLoss: TriggerStopLoss}},
	}
	assert.Equal(t, exp, orders, "buildOrderRequests should add reduce-only children on the opposite side, bounding a market child by the default slippage")
	assert.Equal(t, GroupingNormalTPSL, grouping, "buildOrderRequests should group children with their parent")

	short := grouped
	short.Side = order.Sell
	short.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 90, LimitPrice: 91, OrderType: order.TakeProfit}
	short.RiskManagementModes.StopLoss = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110, OrderType: order.Stop}
	orders, _, _, err = ex.buildOrderRequests(t.Context(), &short)
	require.NoError(t, err, "buildOrderRequests must not error for a short parent")
	exp = []OrderRequest{
		{Price: 100, Size: 0.1, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}, ClientOrderID: testClientOrderID},
		{IsBuy: true, Price: 91, Size: 0.1, ReduceOnly: true, Trigger: &TriggerOrderType{TriggerPrice: 90, TakeProfitStopLoss: TriggerTakeProfit}},
		{IsBuy: true, Price: 121, Size: 0.1, ReduceOnly: true, Trigger: &TriggerOrderType{IsMarket: true, TriggerPrice: 110, TakeProfitStopLoss: TriggerStopLoss}},
	}
	assert.Equal(t, exp, orders, "buildOrderRequests should add buying children to a short parent")

	for _, tc := range []struct {
		name   string
		mutate func(*order.Submit)
		err    error
	}{
		{name: "invalid parent", mutate: func(s *order.Submit) { s.Amount = 0.000001 }, err: errSizePrecision},
		{name: "spot children", mutate: func(s *order.Submit) {
			s.Pair, s.AssetType = spotPair, asset.Spot
			s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, Price: 11}
		}, err: errRiskManagementUnsupported},
		{name: "stop entry", mutate: func(s *order.Submit) { s.RiskManagementModes.StopEntry.Enabled = true }, err: errRiskManagementUnsupported},
		{name: "position grouping", mutate: func(s *order.Submit) {
			s.RiskManagementModes.Mode = GroupingPositionTPSL
			s.RiskManagementModes.TakeProfit.Enabled = true
		}, err: errRiskManagementUnsupported},
		{name: "child without a trigger price", mutate: func(s *order.Submit) { s.RiskManagementModes.TakeProfit.Enabled = true }, err: errTriggerPriceRequired},
		{name: "child on the index price", mutate: func(s *order.Submit) {
			s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, Price: 110, TriggerPriceType: order.IndexPrice}
		}, err: errRiskManagementUnsupported},
		{name: "stop take profit", mutate: func(s *order.Submit) {
			s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110, OrderType: order.Stop}
		}, err: errRiskManagementUnsupported},
		{name: "take profit stop loss", mutate: func(s *order.Submit) {
			s.RiskManagementModes.StopLoss = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 90, OrderType: order.TakeProfit}
		}, err: errRiskManagementUnsupported},
		{name: "child price beyond the market's precision", mutate: func(s *order.Submit) {
			s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110, LimitPrice: 100.55, OrderType: order.Limit}
		}, err: errPricePrecision},
		{name: "whole child slippage", mutate: func(s *order.Submit) {
			s.SlippageTolerance = 1
			s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110}
		}, err: errSlippageTolerance},
	} {
		invalid := *submit
		tc.mutate(&invalid)
		_, _, _, err := ex.buildOrderRequests(t.Context(), &invalid)
		assert.ErrorIsf(t, err, tc.err, "buildOrderRequests should reject a %s", tc.name)
	}
}

func TestRiskManagementOrderType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		orderType  order.Type
		takeProfit bool
		exp        order.Type
		err        error
	}{
		{orderType: order.UnknownType, takeProfit: true, exp: order.TakeProfitMarket},
		{orderType: order.Market, takeProfit: true, exp: order.TakeProfitMarket},
		{orderType: order.TakeProfitMarket, takeProfit: true, exp: order.TakeProfitMarket},
		{orderType: order.Limit, takeProfit: true, exp: order.TakeProfit},
		{orderType: order.TakeProfit, takeProfit: true, exp: order.TakeProfit},
		{orderType: order.Stop, takeProfit: true, err: errRiskManagementUnsupported},
		{orderType: order.UnknownType, exp: order.StopMarket},
		{orderType: order.Market, exp: order.StopMarket},
		{orderType: order.Stop, exp: order.StopMarket},
		{orderType: order.StopMarket, exp: order.StopMarket},
		{orderType: order.Limit, exp: order.StopLimit},
		{orderType: order.StopLimit, exp: order.StopLimit},
		{orderType: order.TakeProfit, err: errRiskManagementUnsupported},
	} {
		result, err := riskManagementOrderType(tc.orderType, tc.takeProfit)
		require.ErrorIsf(t, err, tc.err, "riskManagementOrderType must return the expected error for %s take profit %t", tc.orderType, tc.takeProfit)
		assert.Equalf(t, tc.exp, result, "riskManagementOrderType should map %s take profit %t", tc.orderType, tc.takeProfit)
	}
}

func TestConvertOrder(t *testing.T) {
	t.Parallel()
	ex := newTradingServerExchange(t, nil, nil)
	source := testOpenOrder()
	result, err := ex.convertOrder(t.Context(), &source, "open", time.UnixMilli(1791275930650))
	require.NoError(t, err, "convertOrder must not error")
	exp := order.Detail{
		TimeInForce:     order.GoodTillCancel,
		Price:           85000,
		Amount:          0.002,
		ExecutedAmount:  0.001,
		RemainingAmount: 0.001,
		Exchange:        ex.Name,
		OrderID:         strconv.FormatUint(testOrderID, 10),
		ClientOrderID:   testClientOrderID,
		Type:            order.Limit,
		Side:            order.Buy,
		Status:          order.Open,
		AssetType:       asset.PerpetualContract,
		Date:            time.UnixMilli(1791275926947).UTC(),
		LastUpdated:     time.UnixMilli(1791275930650).UTC(),
		Pair:            perpetualPair,
	}
	assert.Equal(t, exp, result, "convertOrder should convert the order")
	source.Coin = "MISSING"
	_, err = ex.convertOrder(t.Context(), &source, "open", time.Time{})
	assert.ErrorIs(t, err, errPairMappingNotFound, "convertOrder should report an order without a market")
}

func TestConvertOrderFromMapping(t *testing.T) {
	t.Parallel()
	child := testOpenOrder().Children[0]
	result, err := e.convertOrderFromMapping(&child, "triggered", time.Time{}, &testPerpetualMapping, asset.PerpetualContract)
	require.NoError(t, err, "convertOrderFromMapping must not error")
	exp := order.Detail{
		TimeInForce:     order.GoodTillCancel,
		ReduceOnly:      true,
		Price:           90000,
		Amount:          0.001,
		TriggerPrice:    90000,
		RemainingAmount: 0.001,
		Exchange:        e.Name,
		OrderID:         "566563303536",
		Type:            order.TakeProfitMarket,
		Side:            order.Sell,
		Status:          order.Closed,
		AssetType:       asset.PerpetualContract,
		Date:            time.UnixMilli(1791275926947).UTC(),
		LastUpdated:     time.UnixMilli(1791275926947).UTC(),
		Pair:            perpetualPair,
	}
	assert.Equal(t, exp, result, "convertOrderFromMapping should convert the trigger order")

	for _, tc := range []struct {
		mutate func(*FrontendOpenOrder)
		status string
		err    error
	}{
		{mutate: func(o *FrontendOpenOrder) { o.OrderType = "unknown"; o.IsTrigger = false }, status: "open", err: order.ErrTypeIsInvalid},
		{mutate: func(o *FrontendOpenOrder) { o.TimeInForce = "bad" }, status: "open", err: order.ErrInvalidTimeInForce},
		{mutate: func(o *FrontendOpenOrder) { o.Side = "X" }, status: "open", err: order.ErrSideIsInvalid},
		{mutate: func(*FrontendOpenOrder) {}, status: "unknown", err: errUnsupportedOrderStatus},
	} {
		invalid := testOpenOrder()
		tc.mutate(&invalid)
		_, err := e.convertOrderFromMapping(&invalid, tc.status, time.Time{}, &testPerpetualMapping, asset.PerpetualContract)
		assert.ErrorIsf(t, err, tc.err, "convertOrderFromMapping should reject %+v with status %s", invalid, tc.status)
	}
}

func TestConvertWsOrder(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	ex.Name = t.Name()
	update := &WsOrder{Order: testOpenOrder().BasicOrder, Status: "canceled", StatusTimestamp: milli(1791275930650)}
	_, err := ex.convertWsOrder(update)
	require.ErrorIs(t, err, errPairMappingNotFound, "convertWsOrder must report an order without a cached market")
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{testPerpetualMapping})
	result, err := ex.convertWsOrder(update)
	require.NoError(t, err, "convertWsOrder must not error")
	exp := order.Detail{
		Price:           85000,
		Amount:          0.002,
		ExecutedAmount:  0.001,
		RemainingAmount: 0.001,
		Exchange:        ex.Name,
		OrderID:         strconv.FormatUint(testOrderID, 10),
		ClientOrderID:   testClientOrderID,
		Side:            order.Buy,
		Status:          order.Cancelled,
		AssetType:       asset.PerpetualContract,
		Date:            time.UnixMilli(1791275926947).UTC(),
		LastUpdated:     time.UnixMilli(1791275930650).UTC(),
		Pair:            perpetualPair,
	}
	assert.Equal(t, exp, result, "convertWsOrder should convert the update without an order type or time in force")
}

func TestConvertBasicOrder(t *testing.T) {
	t.Parallel()
	source := BasicOrder{Coin: "BTC", Side: "A", LimitPrice: 100, Size: 1, OrderID: 7, Timestamp: milli(1791275926947)}
	result, err := e.convertBasicOrder(&source, "filled", time.Time{}, perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "convertBasicOrder must not error")
	exp := order.Detail{
		Price:           100,
		Amount:          1,
		RemainingAmount: 1,
		Exchange:        e.Name,
		OrderID:         "7",
		Side:            order.Sell,
		Status:          order.Filled,
		AssetType:       asset.PerpetualContract,
		Date:            time.UnixMilli(1791275926947).UTC(),
		LastUpdated:     time.UnixMilli(1791275926947).UTC(),
		Pair:            perpetualPair,
	}
	assert.Equal(t, exp, result, "convertBasicOrder should use the remaining size as the amount without an original size, and the placement time as the update time")
	source.Side = "X"
	_, err = e.convertBasicOrder(&source, "open", time.Time{}, perpetualPair, asset.PerpetualContract)
	assert.ErrorIs(t, err, order.ErrSideIsInvalid, "convertBasicOrder should reject an invalid side")
	source.Side = "B"
	_, err = e.convertBasicOrder(&source, "unknown", time.Time{}, perpetualPair, asset.PerpetualContract)
	assert.ErrorIs(t, err, errUnsupportedOrderStatus, "convertBasicOrder should reject an unknown status")
}

func TestPerpetualTickerPrice(t *testing.T) {
	t.Parallel()
	market := &PerpetualAssetContext{Funding: 0.0001, OpenInterest: 10, PreviousDayPrice: 99, DayNotionalVolume: 1000, OraclePrice: 100, MarkPrice: 101, MidPrice: 100.5, DayBaseVolume: 10}
	result := e.perpetualTickerPrice(perpetualPair, market)
	exp := &ticker.Price{Last: 100.5, Open: 99, BaseVolume: 10, QuoteVolume: 1000, OpenInterest: 10, MarkPrice: 101, IndexPrice: 100, Pair: perpetualPair, ExchangeName: e.Name, AssetType: asset.PerpetualContract, LastUpdated: result.LastUpdated}
	assert.Equal(t, exp, result, "perpetualTickerPrice should use the mid price as the last price")
	market.MidPrice = 0
	assert.Equal(t, 101.0, e.perpetualTickerPrice(perpetualPair, market).Last, "perpetualTickerPrice should use the mark price without a mid price")
}

func TestSpotTickerPrice(t *testing.T) {
	t.Parallel()
	market := &SpotAssetContext{PreviousDayPrice: 9, DayNotionalVolume: 100, MarkPrice: 10, MidPrice: 9.5, Coin: "@107", DayBaseVolume: 10}
	result := e.spotTickerPrice(spotPair, market)
	exp := &ticker.Price{Last: 9.5, Open: 9, BaseVolume: 10, QuoteVolume: 100, MarkPrice: 10, Pair: spotPair, ExchangeName: e.Name, AssetType: asset.Spot, LastUpdated: result.LastUpdated}
	assert.Equal(t, exp, result, "spotTickerPrice should use the mid price as the last price")
	market.MidPrice = 0
	assert.Equal(t, 10.0, e.spotTickerPrice(spotPair, market).Last, "spotTickerPrice should use the mark price without a mid price")
}

func TestParseSide(t *testing.T) {
	t.Parallel()
	for side, exp := range map[string]order.Side{"A": order.Sell, "B": order.Buy} {
		result, err := parseSide(side)
		require.NoErrorf(t, err, "parseSide must not error for %s", side)
		assert.Equalf(t, exp, result, "parseSide should parse %s", side)
	}
	_, err := parseSide("b")
	assert.ErrorIs(t, err, order.ErrSideIsInvalid, "parseSide should reject an unknown side")
}

func TestParseOrderStatus(t *testing.T) {
	t.Parallel()
	for status, exp := range map[string]order.Status{
		"open":                    order.Open,
		"filled":                  order.Filled,
		"triggered":               order.Closed,
		"canceled":                order.Cancelled,
		"scheduledCancel":         order.Cancelled,
		"marginCanceled":          order.Cancelled,
		"liquidatedCanceled":      order.Cancelled,
		"rejected":                order.Rejected,
		"perpMarginRejected":      order.Rejected,
		"perpMaxPositionRejected": order.Rejected,
	} {
		result, err := parseOrderStatus(status)
		require.NoErrorf(t, err, "parseOrderStatus must not error for %s", status)
		assert.Equalf(t, exp, result, "parseOrderStatus should parse %s", status)
	}
	_, err := parseOrderStatus("unknown")
	assert.ErrorIs(t, err, errUnsupportedOrderStatus, "parseOrderStatus should reject an unknown status")
}

func TestParseOrderType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		orderType string
		isTrigger bool
		exp       order.Type
	}{
		{orderType: "Limit", exp: order.Limit},
		{orderType: "Market", exp: order.Market},
		{orderType: "Stop Limit", isTrigger: true, exp: order.StopLimit},
		{orderType: "Stop Market", isTrigger: true, exp: order.StopMarket},
		{orderType: "Stop", isTrigger: true, exp: order.Stop},
		{orderType: "Take Profit Limit", isTrigger: true, exp: order.TakeProfit},
		{orderType: "Take Profit Market", isTrigger: true, exp: order.TakeProfitMarket},
	} {
		result, err := parseOrderType(tc.orderType, tc.isTrigger)
		require.NoErrorf(t, err, "parseOrderType must not error for %s", tc.orderType)
		assert.Equalf(t, tc.exp, result, "parseOrderType should parse %s", tc.orderType)
	}
	_, err := parseOrderType("unknown", false)
	assert.ErrorIs(t, err, order.ErrTypeIsInvalid, "parseOrderType should reject an unknown type")
}

func TestParseTimeInForce(t *testing.T) {
	t.Parallel()
	for timeInForce, exp := range map[string]order.TimeInForce{
		"":               order.GoodTillCancel,
		"Gtc":            order.GoodTillCancel,
		"Alo":            order.PostOnly,
		"Ioc":            order.ImmediateOrCancel,
		"FrontendMarket": order.ImmediateOrCancel,
	} {
		result, err := parseTimeInForce(timeInForce)
		require.NoErrorf(t, err, "parseTimeInForce must not error for %q", timeInForce)
		assert.Equalf(t, exp, result, "parseTimeInForce should parse %q", timeInForce)
	}
	_, err := parseTimeInForce("bad")
	assert.ErrorIs(t, err, order.ErrInvalidTimeInForce, "parseTimeInForce should reject an unknown time in force")
}

func TestFormatOrderTimeInForce(t *testing.T) {
	t.Parallel()
	for timeInForce, exp := range map[order.TimeInForce]string{
		order.UnknownTIF:                      TimeInForceGTC,
		order.GoodTillCancel:                  TimeInForceGTC,
		order.PostOnly:                        TimeInForceALO,
		order.GoodTillCancel | order.PostOnly: TimeInForceALO,
		order.ImmediateOrCancel:               TimeInForceIOC,
	} {
		result, err := formatOrderTimeInForce(timeInForce)
		require.NoErrorf(t, err, "formatOrderTimeInForce must not error for %s", timeInForce)
		assert.Equalf(t, exp, result, "formatOrderTimeInForce should format %s", timeInForce)
	}
	_, err := formatOrderTimeInForce(order.FillOrKill)
	assert.ErrorIs(t, err, order.ErrUnsupportedTimeInForce, "formatOrderTimeInForce should reject fill or kill")
}

func TestFormatOrderSize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		size     float64
		decimals uint64
		exp      string
		err      error
	}{
		{size: 2, exp: "2"},
		{size: 1.23, decimals: 2, exp: "1.23"},
		{size: 0, decimals: 2, err: errSizePrecision},
		{size: -1, decimals: 2, err: errSizePrecision},
		{size: 1, decimals: 9, err: errSizePrecision},
		{size: 1.234, decimals: 2, err: errSizePrecision},
	} {
		result, err := formatOrderSize(tc.size, tc.decimals)
		require.ErrorIsf(t, err, tc.err, "formatOrderSize must return the expected error for %v with %d decimals", tc.size, tc.decimals)
		assert.Equalf(t, tc.exp, result, "formatOrderSize should format %v with %d decimals", tc.size, tc.decimals)
	}
}

func TestDeriveFilledOrderState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		requested, filled float64
		decimals          uint64
		timeInForce       string
		status            order.Status
		remaining         float64
		err               error
	}{
		{requested: 0.1, filled: 0.1, decimals: 5, timeInForce: TimeInForceGTC, status: order.Filled},
		{requested: 0.1, filled: 0.04, decimals: 5, timeInForce: TimeInForceGTC, status: order.PartiallyFilled, remaining: 0.06},
		{requested: 0.1, filled: 0.04, decimals: 5, timeInForce: TimeInForceIOC, status: order.PartiallyFilledCancelled, remaining: 0.06},
		{requested: 0.1, filled: 0.04, decimals: 5, timeInForce: TimeInForceALO, err: errActionStatusMalformed},
		{requested: 0.1, filled: 0.04, decimals: 5, err: errActionStatusMalformed},
		{requested: 0, filled: 0.04, decimals: 5, timeInForce: TimeInForceGTC, err: errInvalidFilledSize},
		{requested: 0.1, filled: 0.0400001, decimals: 5, timeInForce: TimeInForceGTC, err: errInvalidFilledSize},
		{requested: 0.1, filled: 0.04, decimals: 9, timeInForce: TimeInForceGTC, err: errInvalidFilledSize},
		{requested: 0.1, filled: 0.11, decimals: 5, timeInForce: TimeInForceGTC, err: errInvalidFilledSize},
	} {
		status, remaining, err := deriveFilledOrderState(tc.requested, tc.filled, tc.decimals, tc.timeInForce)
		require.ErrorIsf(t, err, tc.err, "deriveFilledOrderState must return the expected error for %v of %v %q", tc.filled, tc.requested, tc.timeInForce)
		assert.Equalf(t, tc.status, status, "deriveFilledOrderState should derive the status of %v of %v %q", tc.filled, tc.requested, tc.timeInForce)
		assert.InDeltaf(t, tc.remaining, remaining, 1e-12, "deriveFilledOrderState should derive the remainder of %v of %v %q", tc.filled, tc.requested, tc.timeInForce)
	}
}

func TestPriceDecimalBase(t *testing.T) {
	t.Parallel()
	for a, exp := range map[asset.Item]uint64{asset.Spot: spotPriceDecimalBase, asset.PerpetualContract: perpetualPriceDecimalBase} {
		result, err := priceDecimalBase(a, 2)
		require.NoErrorf(t, err, "priceDecimalBase must not error for %s", a)
		assert.Equalf(t, exp, result, "priceDecimalBase should return the %s decimal base", a)
	}
	_, err := priceDecimalBase(asset.Options, 0)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "priceDecimalBase should reject an unsupported asset")
	_, err = priceDecimalBase(asset.PerpetualContract, 7)
	assert.ErrorIs(t, err, errSizePrecision, "priceDecimalBase should reject size decimals above the decimal base")
}

func TestValidateLimitPrice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		price    float64
		a        asset.Item
		decimals uint64
		err      error
	}{
		{price: 1234.5, a: asset.PerpetualContract, decimals: 5},
		{price: 0.001234, a: asset.Spot, decimals: 2},
		{price: 123456789, a: asset.PerpetualContract, decimals: 5},
		{a: asset.Spot, err: errInvalidMarketPrice},
		{price: math.NaN(), a: asset.Spot, err: errInvalidMarketPrice},
		{price: math.Inf(1), a: asset.Spot, err: errInvalidMarketPrice},
		{price: 1, a: asset.Options, err: asset.ErrNotSupported},
		{price: 1, a: asset.PerpetualContract, decimals: 7, err: errSizePrecision},
		{price: 100.55, a: asset.PerpetualContract, decimals: 5, err: errPricePrecision},
		{price: 12.3456, a: asset.Spot, decimals: 2, err: errPricePrecision},
		{price: 0.000000001, a: asset.Spot, err: errWireNumberRounding},
	} {
		assert.ErrorIsf(t, validateLimitPrice(tc.price, tc.a, tc.decimals), tc.err, "validateLimitPrice should return the expected error for %v %s with %d size decimals", tc.price, tc.a, tc.decimals)
	}
}

func TestRoundMarketPrice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		price    float64
		a        asset.Item
		decimals uint64
		exp      float64
		err      error
	}{
		{price: 123.456789, a: asset.Spot, decimals: 2, exp: 123.46},
		{price: 123.456789, a: asset.PerpetualContract, decimals: 5, exp: 123.5},
		{a: asset.Spot, err: errInvalidMarketPrice},
		{price: math.NaN(), a: asset.Spot, err: errInvalidMarketPrice},
		{price: math.Inf(1), a: asset.Spot, err: errInvalidMarketPrice},
		{price: 1, a: asset.Options, err: asset.ErrNotSupported},
		{price: 1, a: asset.PerpetualContract, decimals: 7, err: errSizePrecision},
		{price: 0.4, a: asset.PerpetualContract, decimals: 6, err: errInvalidMarketPrice},
	} {
		result, err := roundMarketPrice(tc.price, tc.a, tc.decimals)
		require.ErrorIsf(t, err, tc.err, "roundMarketPrice must return the expected error for %v %s with %d size decimals", tc.price, tc.a, tc.decimals)
		assert.InDeltaf(t, tc.exp, result, 1e-12, "roundMarketPrice should round %v %s with %d size decimals", tc.price, tc.a, tc.decimals)
	}
}

func TestSlippagePrice(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 101.0, slippagePrice(100, 0.01, order.Buy), 1e-12, "slippagePrice should raise a buy's price")
	assert.InDelta(t, 99.0, slippagePrice(100, 0.01, order.Sell), 1e-12, "slippagePrice should lower a sell's price")
}
