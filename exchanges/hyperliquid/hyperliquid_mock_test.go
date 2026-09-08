package hyperliquid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
)

const (
	testBuilderDEXName    = "xyz"
	perpetualMetadataJSON = `{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":40,"marginTableId":56}],"collateralToken":0,"marginTables":[]}`
	spotMetadataJSON      = `{"universe":[{"tokens":[150,0],"name":"@107","index":107,"isCanonical":true}],"tokens":[{"name":"USDC","szDecimals":8,"weiDecimals":8,"index":0,"tokenId":"0x0","isCanonical":true,"evmContract":null,"fullName":"USD Coin","deployerTradingFeeShare":"0"},{"name":"HYPE","szDecimals":2,"weiDecimals":8,"index":150,"tokenId":"0x96","isCanonical":true,"evmContract":{"address":"0x1"},"fullName":"Hyperliquid","deployerTradingFeeShare":"0"}]}`
	perpetualContextsJSON = `[` + perpetualMetadataJSON + `,[{"funding":"0.0001","openInterest":"10","prevDayPx":"99","dayNtlVlm":"1000","premium":"0.001","oraclePx":"100","markPx":"101","midPx":"100.5","impactPxs":["100","101"],"dayBaseVlm":"10"}]]`
	spotContextsJSON      = `[` + spotMetadataJSON + `,[{"prevDayPx":"9","dayNtlVlm":"100","markPx":"10","midPx":"9.5","circulatingSupply":"1000","totalSupply":"1000","coin":"@107","dayBaseVlm":"10"}]]`
	bookJSON              = `{"coin":"BTC","levels":[[{"px":"100","sz":"2","n":1}],[{"px":"101","sz":"3","n":2}]],"time":1700000000000}`
	tradesJSON            = `[{"coin":"BTC","side":"A","px":"100","sz":"2","time":1700000000000,"hash":"0x1","tid":7,"users":["0x2"]}]`
	candlesJSON           = `[{"t":1700000000000,"T":1700000059999,"s":"BTC","i":"1m","o":"100","c":"101","h":"102","l":"99","v":"5","n":3}]`
)

func newHTTPTestExchange(t *testing.T, handler http.Handler) *Exchange {
	t.Helper()
	ex := new(Exchange)
	ex.SetDefaults()
	cfg, err := ex.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for the standard config")
	require.NoError(t, ex.Setup(cfg), "Setup must not error for the test exchange")
	require.NoError(t, ex.DisableRateLimiter(), "DisableRateLimiter must not error for the test rate limiter")

	server := httptest.NewServer(handler)
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL must not error for the spot test URL")
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestFutures.String(), server.URL), "SetRunningURL must not error for the futures test URL")
	t.Cleanup(func() {
		server.Close()
		assert.NoError(t, ex.Shutdown(), "Shutdown should not error for the test exchange")
	})
	return ex
}

func newStaticInfoExchange(t *testing.T, responses map[string]string) *Exchange {
	t.Helper()
	return newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("r.Method should be POST: %s", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/info" {
			t.Errorf("r.URL.Path should be /info: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		var payload infoRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("Decode should not error for the request body: %v", err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		response, ok := responses[payload.Type]
		if !ok && payload.Type == infoTypePerpetualDEXs {
			response = `[null]`
			ok = true
		}
		if !ok {
			t.Errorf("payload.Type should have a configured response: %s", payload.Type)
			http.Error(w, "unexpected request type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(response)); err != nil {
			t.Errorf("Write should not error for the response: %v", err)
		}
	}))
}

func TestGetRESTEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name     string
		asset    asset.Item
		expected exchange.URL
		errorIs  error
	}{
		{name: "spot", asset: asset.Spot, expected: exchange.RestSpot},
		{name: "perpetual", asset: asset.PerpetualContract, expected: exchange.RestFutures},
		{name: "unsupported", asset: asset.Options, errorIs: asset.ErrNotSupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := getRESTEndpoint(tc.asset)
			require.ErrorIs(t, err, tc.errorIs, "getRESTEndpoint must return the expected error for a REST endpoint")
			assert.Equal(t, tc.expected, endpoint, "endpoint: REST endpoint should match the asset type")
		})
	}
}

func TestSetup(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	cfg, err := ex.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for the standard config")

	disabled := *cfg
	disabled.Enabled = false
	require.NoError(t, ex.Setup(&disabled), "Setup must not error for a disabled exchange")
	assert.False(t, ex.IsEnabled(), "IsEnabled: disabled exchange should remain disabled")

	cfg.Enabled = true
	cfg.API.AuthenticatedSupport = true
	cfg.API.AuthenticatedWebsocketSupport = true
	cfg.API.Credentials.Key = officialSigningAddress
	require.NoError(t, ex.Setup(cfg), "Setup must not error for an enabled exchange")
	assert.True(t, ex.IsEnabled(), "IsEnabled: enabled exchange should remain enabled")
	assert.True(t, ex.API.AuthenticatedSupport, "ex.API.AuthenticatedSupport: configured authenticated REST support should remain enabled")
	assert.True(t, ex.API.AuthenticatedWebsocketSupport, "ex.API.AuthenticatedWebsocketSupport: configured account websocket support should remain enabled")
	credentials, err := ex.GetCredentials(t.Context())
	require.NoError(t, err, "GetCredentials must not error for configured credentials")
	assert.Equal(t, officialSigningAddress, credentials.Key, "credentials.Key: setup should load the configured watch address")
	assert.True(t, ex.isMainnetEnvironment(), "isMainnetEnvironment: the default configuration should use the mainnet signing environment")

	sandbox := new(Exchange)
	sandbox.SetDefaults()
	sandboxConfig, err := sandbox.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for a sandbox config")
	sandboxConfig.UseSandbox = true
	require.NoError(t, sandbox.Setup(sandboxConfig), "Setup must not error for the official sandbox")
	assert.False(t, sandbox.isMainnetEnvironment(), "isMainnetEnvironment: sandbox configuration should use the testnet signing environment")
	for _, endpoint := range []struct {
		kind     exchange.URL
		expected string
	}{
		{kind: exchange.RestSpot, expected: hyperliquidTestnetAPIURL},
		{kind: exchange.RestFutures, expected: hyperliquidTestnetAPIURL},
		{kind: exchange.WebsocketSpot, expected: hyperliquidTestnetWebsocketURL},
	} {
		runningURL, err := sandbox.API.Endpoints.GetURL(endpoint.kind)
		require.NoError(t, err, "GetURL must not error for an official sandbox endpoint")
		assert.Equal(t, endpoint.expected, runningURL, "runningURL: sandbox should replace the matching production endpoint")
	}
	require.NoError(t, sandbox.Shutdown(), "Shutdown must not error for the sandbox exchange")

	trailingSlashSandbox := new(Exchange)
	trailingSlashSandbox.SetDefaults()
	trailingSlashConfig, err := trailingSlashSandbox.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for a trailing-slash sandbox config")
	trailingSlashConfig.UseSandbox = true
	trailingSlashConfig.API.Endpoints = map[string]string{
		exchange.RestSpot.String():      hyperliquidAPIURL + "/",
		exchange.RestFutures.String():   hyperliquidAPIURL + "/",
		exchange.WebsocketSpot.String(): hyperliquidWebsocketURL + "/",
	}
	require.NoError(t, trailingSlashSandbox.Setup(trailingSlashConfig), "Setup must not error for official production endpoints with trailing slashes")
	for _, endpoint := range []struct {
		kind     exchange.URL
		expected string
	}{
		{kind: exchange.RestSpot, expected: hyperliquidTestnetAPIURL},
		{kind: exchange.RestFutures, expected: hyperliquidTestnetAPIURL},
		{kind: exchange.WebsocketSpot, expected: hyperliquidTestnetWebsocketURL},
	} {
		runningURL, err := trailingSlashSandbox.API.Endpoints.GetURL(endpoint.kind)
		require.NoError(t, err, "GetURL must not error for a trailing-slash sandbox endpoint")
		assert.Equal(t, endpoint.expected, runningURL, "runningURL: sandbox should normalise the production endpoint before selecting testnet")
	}
	require.NoError(t, trailingSlashSandbox.Shutdown(), "Shutdown must not error for the trailing-slash sandbox exchange")

	customSandbox := new(Exchange)
	customSandbox.SetDefaults()
	customConfig, err := customSandbox.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for a custom sandbox config")
	customConfig.UseSandbox = true
	customConfig.API.Endpoints = map[string]string{
		exchange.RestSpot.String():      "https://hyperliquid-testnet.internal.example",
		exchange.RestFutures.String():   "https://hyperliquid-testnet.internal.example",
		exchange.WebsocketSpot.String(): "wss://hyperliquid-testnet.internal.example/ws",
	}
	require.NoError(t, customSandbox.Setup(customConfig), "Setup must not error for a custom testnet gateway")
	customRESTURL, err := customSandbox.API.Endpoints.GetURL(exchange.RestSpot)
	require.NoError(t, err, "GetURL must not error for a custom REST endpoint")
	assert.Equal(t, customConfig.API.Endpoints[exchange.RestSpot.String()], customRESTURL, "customRESTURL: sandbox setup should preserve a custom gateway")
	assert.False(t, customSandbox.isMainnetEnvironment(), "isMainnetEnvironment: a custom gateway should use the explicitly configured sandbox environment")
	require.NoError(t, customSandbox.Shutdown(), "Shutdown must not error for the custom sandbox exchange")

	mismatchedEnvironment := new(Exchange)
	mismatchedEnvironment.SetDefaults()
	mismatchedConfig, err := mismatchedEnvironment.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for an environment-mismatch config")
	mismatchedConfig.API.Endpoints = map[string]string{
		exchange.RestSpot.String():      hyperliquidTestnetAPIURL,
		exchange.RestFutures.String():   hyperliquidTestnetAPIURL,
		exchange.WebsocketSpot.String(): hyperliquidTestnetWebsocketURL,
	}
	require.ErrorIs(t, mismatchedEnvironment.Setup(mismatchedConfig), errEndpointEnvironment, "Setup must fail closed for official testnet endpoints without sandbox mode")
	require.NoError(t, mismatchedEnvironment.Shutdown(), "Shutdown must not error for the environment-mismatch exchange")

	missingEndpoints := new(Exchange)
	missingEndpoints.SetDefaults()
	missingConfig, err := missingEndpoints.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for a missing-endpoint config")
	missingConfig.UseSandbox = true
	missingEndpoints.API.Endpoints = missingEndpoints.NewEndpoints()
	require.Error(t, missingEndpoints.Setup(missingConfig), "Setup must error for sandbox setup without endpoints")
	require.NoError(t, missingEndpoints.Shutdown(), "Shutdown must not error for the missing-endpoint exchange")

	invalidEndpoint := new(Exchange)
	invalidEndpoint.SetDefaults()
	invalidEndpointConfig, err := invalidEndpoint.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for an invalid-endpoint config")
	invalidEndpointConfig.API.Endpoints = map[string]string{"invalid": "https://example.com"}
	require.Error(t, invalidEndpoint.Setup(invalidEndpointConfig), "Setup with an invalid configured endpoint must error")
	require.NoError(t, invalidEndpoint.Shutdown(), "Shutdown must not error for the invalid-endpoint exchange")

	missingWebsocketEndpoint := new(Exchange)
	missingWebsocketEndpoint.SetDefaults()
	missingWebsocketConfig, err := missingWebsocketEndpoint.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for a missing-websocket config")
	missingWebsocketEndpoint.API.Endpoints = missingWebsocketEndpoint.NewEndpoints()
	require.Error(t, missingWebsocketEndpoint.Setup(missingWebsocketConfig), "Setup without a websocket endpoint must error")
	require.NoError(t, missingWebsocketEndpoint.Shutdown(), "Shutdown must not error for the missing-websocket exchange")

	invalidTrafficTimeout := new(Exchange)
	invalidTrafficTimeout.SetDefaults()
	invalidTrafficConfig, err := invalidTrafficTimeout.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for an invalid-traffic config")
	invalidTrafficConfig.WebsocketTrafficTimeout = time.Millisecond
	require.Error(t, invalidTrafficTimeout.Setup(invalidTrafficConfig), "Setup with an invalid websocket traffic timeout must error")
	require.NoError(t, invalidTrafficTimeout.Shutdown(), "Shutdown must not error for the invalid-traffic exchange")

	connectionFailure := new(Exchange)
	connectionFailure.SetDefaults()
	connectionFailureConfig, err := connectionFailure.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error for a connection-failure config")
	connectionFailure.Websocket.TrafficAlert = nil
	require.Error(t, connectionFailure.Setup(connectionFailureConfig), "Setup with invalid websocket connection state must error")
	require.NoError(t, connectionFailure.Shutdown(), "Shutdown must not error for the connection-failure exchange")

	require.Error(t, ex.Setup(nil), "Setup must error with a nil config")
	require.NoError(t, ex.Shutdown(), "Shutdown must not error for the exchange")
}

func TestSendHTTPRequest(t *testing.T) {
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method, "r.Method: request method should be POST")
		assert.Equal(t, "/info", r.URL.Path, "r.URL.Path: request path should target the info endpoint")
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"), "Get: request content type should be JSON")
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for the request body") {
			return
		}
		_, err := w.Write([]byte(`{"BTC":"100"}`))
		assert.NoError(t, err, "Write should not error for the response")
	}))
	var result map[string]string
	require.NoError(t, ex.SendHTTPRequest(t.Context(), exchange.RestSpot, infoLightEPL, &infoRequest{Type: "allMids"}, &result), "SendHTTPRequest must not error for a valid info request")
	assert.Equal(t, "allMids", got.Type, "got.Type: request type should be serialised")
	assert.Equal(t, "100", result["BTC"], "result[\"BTC\"]: response should be decoded")

	uninitialised := new(Exchange)
	require.ErrorIs(t, uninitialised.SendHTTPRequest(t.Context(), exchange.RestSpot, infoLightEPL, &infoRequest{Type: "allMids"}, &result), common.ErrNilPointer, "SendHTTPRequest must return the expected error without endpoints")
	var nilExchange *Exchange
	require.ErrorIs(t, nilExchange.SendHTTPRequest(t.Context(), exchange.RestSpot, infoLightEPL, &infoRequest{Type: "allMids"}, &result), common.ErrNilPointer, "SendHTTPRequest must return the expected error with a nil exchange")
	nilEndpoints := new(Exchange)
	nilEndpoints.SetDefaults()
	nilEndpoints.API.Endpoints = nil
	require.ErrorIs(t, nilEndpoints.SendHTTPRequest(t.Context(), exchange.RestSpot, infoLightEPL, &infoRequest{Type: "allMids"}, &result), common.ErrNilPointer, "SendHTTPRequest must return the expected error without an endpoint store")
	require.NoError(t, nilEndpoints.Shutdown(), "Shutdown must not error for the nil-endpoints exchange")
	missingEndpoint := new(Exchange)
	missingEndpoint.SetDefaults()
	missingEndpoint.API.Endpoints = missingEndpoint.NewEndpoints()
	require.Error(t, missingEndpoint.SendHTTPRequest(t.Context(), exchange.RestSpot, infoLightEPL, &infoRequest{Type: "allMids"}, &result), "SendHTTPRequest must error without a configured spot endpoint")
	require.NoError(t, missingEndpoint.Shutdown(), "Shutdown must not error for the missing-endpoint exchange")
	require.Error(t, ex.SendHTTPRequest(t.Context(), exchange.RestSpot, infoLightEPL, make(chan int), &result), "SendHTTPRequest must error for an unsupported JSON value")

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, ex.SendHTTPRequest(cancelled, exchange.RestSpot, infoLightEPL, &infoRequest{Type: "allMids"}, &result), context.Canceled, "SendHTTPRequest must return its cancellation with a cancelled context")
}

func TestGetMetadata(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{"spotMeta": spotMetadataJSON})
	futuresServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(perpetualMetadataJSON))
		assert.NoError(t, err, "Write should not error for the perpetual metadata response")
	}))
	t.Cleanup(futuresServer.Close)
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestFutures.String(), futuresServer.URL), "SetRunningURL must not error for a distinct futures URL")
	perpetual, err := ex.GetPerpetualMetadata(t.Context())
	require.NoError(t, err, "GetPerpetualMetadata must not error for perpetual metadata")
	require.Len(t, perpetual.Universe, 1, "perpetual.Universe: perpetual metadata must contain one market")
	assert.Equal(t, "BTC", perpetual.Universe[0].Name, "perpetual.Universe[0].Name: perpetual market name should be decoded")

	spot, err := ex.GetSpotMetadata(t.Context())
	require.NoError(t, err, "GetSpotMetadata must not error for spot metadata")
	require.Len(t, spot.Universe, 1, "spot.Universe: spot metadata must contain one market")
	assert.Equal(t, "@107", spot.Universe[0].Name, "spot.Universe[0].Name: spot market identifier should be decoded")

	nullExchange := newStaticInfoExchange(t, map[string]string{"meta": "null", "spotMeta": "null"})
	_, err = nullExchange.GetPerpetualMetadata(t.Context())
	require.ErrorIs(t, err, common.ErrNilPointer, "GetPerpetualMetadata must return the expected error for null perpetual metadata")
	_, err = nullExchange.GetSpotMetadata(t.Context())
	require.ErrorIs(t, err, common.ErrNilPointer, "GetSpotMetadata must return the expected error for null spot metadata")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetPerpetualMetadata(t.Context())
	require.Error(t, err, "GetPerpetualMetadata must error for perpetual metadata from a failing server")
	_, err = errorExchange.GetSpotMetadata(t.Context())
	require.Error(t, err, "GetSpotMetadata must error for spot metadata from a failing server")
}

func TestGetPerpetualMetadataForDEX(t *testing.T) {
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for named DEX metadata request") {
			return
		}
		_, err := w.Write([]byte(`{"universe":[{"name":"xyz:XYZ100"}]}`))
		assert.NoError(t, err, "Write should not error for named DEX metadata response")
	}))
	result, err := ex.GetPerpetualMetadataForDEX(t.Context(), "xyz")
	require.NoError(t, err, "GetPerpetualMetadataForDEX must not error for named DEX metadata")
	require.Len(t, result.Universe, 1, "GetPerpetualMetadataForDEX must decode one market")
	assert.Equal(t, "xyz", got.DEX, "got.DEX: named DEX should be included in the metadata request")
}

func TestGetPerpetualDEXs(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		infoTypePerpetualDEXs: `[null,{"name":"xyz","fullName":"XYZ","deployer":"` + officialSigningAddress + `"}]`,
	})
	result, err := ex.GetPerpetualDEXs(t.Context())
	require.NoError(t, err, "GetPerpetualDEXs must not error for valid perpetual DEX registry")
	require.Len(t, result, 2, "GetPerpetualDEXs must retain the default entry")
	require.NotNil(t, result[1], "result[1]: builder DEX registry entry must be decoded")
	assert.Equal(t, "xyz", result[1].Name, "result[1].Name: builder DEX name should be decoded")

	for _, raw := range []string{`[]`, `[{"name":"invalid-default"}]`} {
		invalid := newStaticInfoExchange(t, map[string]string{infoTypePerpetualDEXs: raw})
		_, err = invalid.GetPerpetualDEXs(t.Context())
		require.ErrorIs(t, err, errUnexpectedResponseLength, "GetPerpetualDEXs must return the expected error for invalid perpetual DEX registry")
	}
	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetPerpetualDEXs(t.Context())
	require.Error(t, err, "GetPerpetualDEXs must error for perpetual DEX registry from a failing server")
}

func TestGetMetadataAndAssetContexts(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		"metaAndAssetCtxs":     perpetualContextsJSON,
		"spotMetaAndAssetCtxs": spotContextsJSON,
	})
	perpetual, err := ex.GetPerpetualMetadataAndAssetContexts(t.Context())
	require.NoError(t, err, "GetPerpetualMetadataAndAssetContexts must not error for perpetual metadata and contexts")
	require.Len(t, perpetual.AssetContexts, 1, "perpetual.AssetContexts: perpetual response must contain one context")
	assert.Equal(t, 101.0, perpetual.AssetContexts[0].MarkPrice.Float64(), "Float64: perpetual mark price should be decoded")

	spot, err := ex.GetSpotMetadataAndAssetContexts(t.Context())
	require.NoError(t, err, "GetSpotMetadataAndAssetContexts must not error for spot metadata and contexts")
	require.Len(t, spot.AssetContexts, 1, "spot.AssetContexts: spot response must contain one context")
	assert.Equal(t, "@107", spot.AssetContexts[0].Coin, "spot.AssetContexts[0].Coin: spot context identifier should be decoded")

	lengthExchange := newStaticInfoExchange(t, map[string]string{
		"metaAndAssetCtxs":     `[]`,
		"spotMetaAndAssetCtxs": `[{}]`,
	})
	_, err = lengthExchange.GetPerpetualMetadataAndAssetContexts(t.Context())
	require.ErrorIs(t, err, errUnexpectedResponseLength, "GetPerpetualMetadataAndAssetContexts must return the expected error for unexpected perpetual response length")
	_, err = lengthExchange.GetSpotMetadataAndAssetContexts(t.Context())
	require.ErrorIs(t, err, errUnexpectedResponseLength, "GetSpotMetadataAndAssetContexts must return the expected error for unexpected spot response length")

	decodeExchange := newStaticInfoExchange(t, map[string]string{
		"metaAndAssetCtxs":     `[false,false]`,
		"spotMetaAndAssetCtxs": `[false,false]`,
	})
	_, err = decodeExchange.GetPerpetualMetadataAndAssetContexts(t.Context())
	require.ErrorContains(t, err, "perpetual metadata", "GetPerpetualMetadataAndAssetContexts must return a decoding error for invalid perpetual metadata")
	_, err = decodeExchange.GetSpotMetadataAndAssetContexts(t.Context())
	require.ErrorContains(t, err, "spot metadata", "GetSpotMetadataAndAssetContexts must return a decoding error for invalid spot metadata")

	contextDecodeExchange := newStaticInfoExchange(t, map[string]string{
		"metaAndAssetCtxs":     `[{},false]`,
		"spotMetaAndAssetCtxs": `[{},false]`,
	})
	_, err = contextDecodeExchange.GetPerpetualMetadataAndAssetContexts(t.Context())
	require.ErrorContains(t, err, "perpetual asset contexts", "GetPerpetualMetadataAndAssetContexts must return a decoding error for invalid perpetual contexts")
	_, err = contextDecodeExchange.GetSpotMetadataAndAssetContexts(t.Context())
	require.ErrorContains(t, err, "spot asset contexts", "GetSpotMetadataAndAssetContexts must return a decoding error for invalid spot contexts")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetPerpetualMetadataAndAssetContexts(t.Context())
	require.Error(t, err, "GetPerpetualMetadataAndAssetContexts must error for perpetual contexts from a failing server")
	_, err = errorExchange.GetSpotMetadataAndAssetContexts(t.Context())
	require.Error(t, err, "GetSpotMetadataAndAssetContexts must error for spot contexts from a failing server")
}

func TestGetPerpetualMetadataAndAssetContextsForDEX(t *testing.T) {
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for DEX contexts request") {
			return
		}
		_, err := w.Write([]byte(perpetualContextsJSON))
		assert.NoError(t, err, "Write should not error for DEX contexts response")
	}))
	result, err := ex.GetPerpetualMetadataAndAssetContextsForDEX(t.Context(), "xyz")
	require.NoError(t, err, "GetPerpetualMetadataAndAssetContextsForDEX must not error for named DEX contexts")
	require.Len(t, result.AssetContexts, 1, "GetPerpetualMetadataAndAssetContextsForDEX must decode one context")
	assert.Equal(t, "xyz", got.DEX, "got.DEX should be included in the request")
}

func TestGetFundingHistory(t *testing.T) {
	start := time.UnixMilli(1700000000000).UTC()
	end := start.Add(time.Hour)
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for funding history request") {
			return
		}
		_, err := w.Write([]byte(`[{"coin":"BTC","fundingRate":"0.0001","premium":"0.0002","time":1700000000000}]`))
		assert.NoError(t, err, "Write should not error for funding history response")
	}))
	_, err := ex.GetFundingHistory(t.Context(), " ", start, end)
	require.ErrorIs(t, err, errCoinRequired, "GetFundingHistory must return the expected error for blank funding coin")
	_, err = ex.GetFundingHistory(t.Context(), "BTC", end, start)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetFundingHistory must return the expected error for invalid funding range")

	result, err := ex.GetFundingHistory(t.Context(), " BTC ", start, end)
	require.NoError(t, err, "GetFundingHistory must not error for valid funding history")
	require.Len(t, result, 1, "GetFundingHistory must decode one record")
	assert.Equal(t, "BTC", got.Coin, "got.Coin: funding coin should be trimmed")
	assert.Equal(t, start.UnixMilli(), got.StartTime, "got.StartTime: funding start time should be serialised")
	assert.Equal(t, end.UnixMilli(), got.EndTime, "got.EndTime: funding end time should be serialised")
	assert.Equal(t, 0.0001, result[0].FundingRate.Float64(), "Float64: funding rate should be decoded")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetFundingHistory(t.Context(), "BTC", start, end)
	require.Error(t, err, "GetFundingHistory must error for funding history from a failing server")
}

func TestGetAllMids(t *testing.T) {
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for the all-mids request") {
			return
		}
		_, err := w.Write([]byte(`{"BTC":"100.5"}`))
		assert.NoError(t, err, "Write should not error for the all-mids response")
	}))
	mids, err := ex.GetAllMids(t.Context(), "test-dex")
	require.NoError(t, err, "GetAllMids must not error for all mid prices")
	assert.Equal(t, "test-dex", got.DEX, "got.DEX should be serialised")
	assert.Equal(t, 100.5, mids["BTC"].Float64(), "Float64: mid price should be decoded")

	nullExchange := newStaticInfoExchange(t, map[string]string{"allMids": "null"})
	_, err = nullExchange.GetAllMids(t.Context(), "")
	require.ErrorIs(t, err, common.ErrNilPointer, "GetAllMids must return the expected error for null all-mids response")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetAllMids(t.Context(), "")
	require.Error(t, err, "GetAllMids must error for all mids from a failing server")
}

func TestGetL2Book(t *testing.T) {
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for the L2 book request") {
			return
		}
		_, err := w.Write([]byte(bookJSON))
		assert.NoError(t, err, "Write should not error for the L2 book response")
	}))
	_, err := ex.GetL2Book(t.Context(), asset.PerpetualContract, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetL2Book must return the expected error for nil L2 book request")
	_, err = ex.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{})
	require.ErrorIs(t, err, errCoinRequired, "GetL2Book must return the expected error for empty L2 book coin")

	invalidSignificantFigures := uint64(1)
	_, err = ex.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{Coin: "BTC", SignificantFigures: &invalidSignificantFigures})
	require.ErrorIs(t, err, errInvalidSignificantFigures, "GetL2Book must return the expected error for invalid significant figures")

	mantissa := uint64(1)
	_, err = ex.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{Coin: "BTC", Mantissa: &mantissa})
	require.ErrorIs(t, err, errInvalidMantissa, "GetL2Book must return the expected error for mantissa without significant figures")
	fourSignificantFigures := uint64(4)
	_, err = ex.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{Coin: "BTC", SignificantFigures: &fourSignificantFigures, Mantissa: &mantissa})
	require.ErrorIs(t, err, errInvalidMantissa, "GetL2Book must return the expected error for mantissa with non-five significant figures")
	fiveSignificantFigures := uint64(5)
	invalidMantissa := uint64(3)
	_, err = ex.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{Coin: "BTC", SignificantFigures: &fiveSignificantFigures, Mantissa: &invalidMantissa})
	require.ErrorIs(t, err, errInvalidMantissa, "GetL2Book must return the expected error for invalid mantissa")

	_, err = ex.GetL2Book(t.Context(), asset.Options, &L2BookRequest{Coin: "BTC"})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetL2Book must return the expected error for unsupported L2 book asset")
	book, err := ex.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{Coin: "BTC", SignificantFigures: &fiveSignificantFigures, Mantissa: &mantissa})
	require.NoError(t, err, "GetL2Book must not error for a valid L2 book")
	require.Len(t, book.Levels, 2, "book.Levels: L2 book must contain bid and ask sides")
	assert.Equal(t, "BTC", got.Coin, "got.Coin should be serialised")
	assert.Equal(t, fiveSignificantFigures, *got.NSigFigs, "got.NSigFigs: significant figures should be serialised")
	assert.Equal(t, mantissa, *got.Mantissa, "got.Mantissa should be serialised")

	nullExchange := newStaticInfoExchange(t, map[string]string{"l2Book": "null"})
	_, err = nullExchange.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{Coin: "BTC"})
	require.ErrorIs(t, err, common.ErrNilPointer, "GetL2Book must return the expected error for null L2 book")
	lengthExchange := newStaticInfoExchange(t, map[string]string{"l2Book": `{"coin":"BTC","levels":[],"time":1700000000000}`})
	_, err = lengthExchange.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{Coin: "BTC"})
	require.ErrorIs(t, err, errInvalidBookLevelCount, "GetL2Book must return the expected error for invalid L2 book side count")
	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetL2Book(t.Context(), asset.PerpetualContract, &L2BookRequest{Coin: "BTC"})
	require.Error(t, err, "GetL2Book must error for an L2 book from a failing server")
}

func TestGetRecentTradesForCoin(t *testing.T) {
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for the recent-trades request") {
			return
		}
		_, err := w.Write([]byte(tradesJSON))
		assert.NoError(t, err, "Write should not error for the recent-trades response")
	}))
	_, err := ex.GetRecentTradesForCoin(t.Context(), " ", asset.PerpetualContract)
	require.ErrorIs(t, err, errCoinRequired, "GetRecentTradesForCoin must return the expected error for empty recent-trades coin")
	_, err = ex.GetRecentTradesForCoin(t.Context(), "BTC", asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetRecentTradesForCoin must return the expected error for unsupported recent-trades asset")
	trades, err := ex.GetRecentTradesForCoin(t.Context(), "BTC", asset.PerpetualContract)
	require.NoError(t, err, "GetRecentTradesForCoin must not error for recent trades")
	require.Len(t, trades, 1, "trades: recent trades must contain one result")
	assert.Equal(t, uint64(7), trades[0].TradeID, "trades[0].TradeID should be decoded")
	assert.Equal(t, "BTC", got.Coin, "got.Coin should be serialised")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetRecentTradesForCoin(t.Context(), "BTC", asset.PerpetualContract)
	require.Error(t, err, "GetRecentTradesForCoin must error for recent trades from a failing server")
}

func TestGetCandles(t *testing.T) {
	start := time.UnixMilli(1700000000000).UTC()
	end := start.Add(time.Minute)
	var got struct {
		Type    string `json:"type"`
		Request struct {
			Coin      string `json:"coin"`
			Interval  string `json:"interval"`
			StartTime int64  `json:"startTime"`
			EndTime   int64  `json:"endTime"`
		} `json:"req"`
	}
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for the candle request") {
			return
		}
		_, err := w.Write([]byte(candlesJSON))
		assert.NoError(t, err, "Write should not error for the candle response")
	}))
	_, err := ex.GetCandles(t.Context(), asset.PerpetualContract, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetCandles must return the expected error for nil candle request")
	_, err = ex.GetCandles(t.Context(), asset.PerpetualContract, &CandleRequest{})
	require.ErrorIs(t, err, errCoinRequired, "GetCandles must return the expected error for empty candle coin")
	_, err = ex.GetCandles(t.Context(), asset.Options, &CandleRequest{Coin: "BTC", Interval: kline.OneMin})
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetCandles must return the expected error for unsupported candle asset")
	_, err = ex.GetCandles(t.Context(), asset.PerpetualContract, &CandleRequest{Coin: "BTC", Interval: kline.Interval(42)})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "GetCandles must return the expected error for unsupported candle interval")
	_, err = ex.GetCandles(t.Context(), asset.PerpetualContract, &CandleRequest{Coin: "BTC", Interval: kline.OneMin, StartTime: end, EndTime: start})
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetCandles must return the expected error for invalid candle times")
	for _, tc := range []struct {
		name      string
		startTime time.Time
		endTime   time.Time
		errorIs   error
	}{
		{name: "start unset", endTime: end, errorIs: common.ErrDateUnset},
		{name: "end unset", startTime: start, errorIs: common.ErrDateUnset},
		{name: "equal times", startTime: start, endTime: start, errorIs: common.ErrStartEqualsEnd},
		{name: "future range", startTime: time.Now().Add(time.Hour), endTime: time.Now().Add(2 * time.Hour), errorIs: common.ErrStartAfterTimeNow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.GetCandles(t.Context(), asset.PerpetualContract, &CandleRequest{Coin: "BTC", Interval: kline.OneMin, StartTime: tc.startTime, EndTime: tc.endTime})
			require.ErrorIs(t, err, tc.errorIs, "GetCandles must return the expected error for invalid candle time range")
		})
	}

	candles, err := ex.GetCandles(t.Context(), asset.PerpetualContract, &CandleRequest{Coin: "BTC", Interval: kline.OneMin, StartTime: start, EndTime: end})
	require.NoError(t, err, "GetCandles must not error for valid candles")
	require.Len(t, candles, 1, "candles: candle response must contain one result")
	assert.Equal(t, "candleSnapshot", got.Type, "got.Type: candle request type should be serialised")
	assert.Equal(t, "BTC", got.Request.Coin, "got.Request.Coin: candle coin should be serialised")
	assert.Equal(t, "1m", got.Request.Interval, "got.Request.Interval: candle interval should be serialised")
	assert.Equal(t, start.UnixMilli(), got.Request.StartTime, "got.Request.StartTime: candle start time should be serialised")
	assert.Equal(t, end.Add(-time.Millisecond).UnixMilli(), got.Request.EndTime, "got.Request.EndTime: candle end time should use an exclusive GCT range")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetCandles(t.Context(), asset.PerpetualContract, &CandleRequest{Coin: "BTC", Interval: kline.OneMin, StartTime: start, EndTime: end})
	require.Error(t, err, "GetCandles must error for candles from a failing server")
}
