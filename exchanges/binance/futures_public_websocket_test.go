package binance

import (
	"context"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func TestPublicFuturesWebsocketAPILive(t *testing.T) {
	if os.Getenv("BINANCE_LIVE_PUBLIC") != "1" {
		t.Skip("set BINANCE_LIVE_PUBLIC=1 to check production public futures WebSocket APIs")
	}
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "public exchange must initialise")
	local.API.AuthenticatedSupport = false
	local.API.AuthenticatedWebsocketSupport = false
	require.NoError(t, local.CurrencyPairs.SetAssetEnabled(asset.USDTMarginedFutures, true), "USD-M asset must enable")
	local.Websocket = websocket.NewManager()
	local.Websocket.DataHandler = stream.NewRelay(64)
	require.NoError(t, local.Websocket.Setup(&websocket.ManagerSetup{ExchangeConfig: local.Config, Features: &local.Features.Supports.WebsocketCapabilities, UseMultiConnectionManagement: true}), "public manager must initialise")
	setup := local.futuresAPIConnectionSetup(asset.USDTMarginedFutures)
	setup.ResponseMaxLimit = 15 * time.Second
	var current atomic.Pointer[string]
	var mu sync.Mutex
	recording := publicWSRecording{ObservedAt: time.Now().UTC(), Responses: make(map[string]json.RawMessage)}
	handler := setup.Handler
	setup.Handler = func(ctx context.Context, conn websocket.Connection, data []byte) error {
		if name := current.Load(); name != nil {
			mu.Lock()
			recording.Responses[*name] = append(json.RawMessage(nil), data...)
			mu.Unlock()
		}
		return handler(ctx, conn, data)
	}
	require.NoError(t, local.Websocket.SetupNewConnection(setup), "public futures API must register")
	require.NoError(t, local.Websocket.Connect(t.Context()), "public futures API must connect")
	t.Cleanup(func() {
		if local.Websocket.IsEnabled() {
			assert.NoError(t, local.Websocket.Disable(), "public manager should disable")
		}
		if local.Websocket.IsConnected() {
			assert.NoError(t, local.Websocket.Shutdown(), "public manager should shut down")
		}
		if path := os.Getenv("BINANCE_RECORD_PUBLIC_FUTURES_WS"); path != "" {
			mu.Lock()
			defer mu.Unlock()
			data, err := json.MarshalIndent(recording, "", " ")
			require.NoError(t, err, "public responses must serialise")
			require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600), "public recording must save")
		}
	})
	runPublicFuturesWSChecks(t, local, &current, func(name string) json.RawMessage { mu.Lock(); defer mu.Unlock(); return recording.Responses[name] })
}

func TestPublicFuturesWebsocketAPIRecorded(t *testing.T) {
	data, err := os.ReadFile("testdata/public_api/futures_websocket_api.json")
	require.NoError(t, err, "public futures recording must load")
	var recording publicWSRecording
	require.NoError(t, json.Unmarshal(data, &recording), "recording must decode")
	var current atomic.Pointer[string]
	local := mockFuturesAPI(t, asset.USDTMarginedFutures, func(tb testing.TB, data []byte, conn *gws.Conn) error {
		tb.Helper()
		var request struct {
			ID     json.RawMessage            `json:"id"`
			Params map[string]json.RawMessage `json:"params"`
		}
		require.NoError(tb, json.Unmarshal(data, &request), "request must decode")
		assert.NotContains(tb, request.Params, "apiKey", "public requests should omit credentials")
		assert.NotContains(tb, request.Params, "signature", "public requests should omit signatures")
		var response map[string]json.RawMessage
		require.NoError(tb, json.Unmarshal(recording.Responses[*current.Load()], &response), "recorded response must decode")
		response["id"] = request.ID
		return conn.WriteJSON(response)
	})
	runPublicFuturesWSChecks(t, local, &current, func(name string) json.RawMessage { return recording.Responses[name] })
}

func runPublicFuturesWSChecks(t *testing.T, local *Exchange, current *atomic.Pointer[string], response func(string) json.RawMessage) {
	t.Helper()
	pair := currency.NewPairWithDelimiter("BTC", "USDT", "-")
	for _, tc := range []struct {
		name string
		call func() (any, error)
	}{
		{"depth", func() (any, error) {
			return local.WsUFuturesGetOrderbook(t.Context(), &WsUFuturesGetOrderbookRequest{Symbol: pair, Limit: 5})
		}},
		{"ticker.price/single", func() (any, error) {
			return local.WsUFuturesGetPriceTicker(t.Context(), &WsUFuturesGetPriceTickerRequest{Symbol: pair})
		}},
		{"ticker.price/all", func() (any, error) { return local.WsUFuturesGetPriceTicker(t.Context(), nil) }},
		{"ticker.book/single", func() (any, error) {
			return local.WsUFuturesGetBookTicker(t.Context(), &WsUFuturesGetBookTickerRequest{Symbol: pair})
		}},
		{"ticker.book/all", func() (any, error) { return local.WsUFuturesGetBookTicker(t.Context(), nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current.Store(new(tc.name))
			result, err := tc.call()
			require.NoError(t, err, "public API must return a successful response")
			require.NotNil(t, result, "public API must return data")
			var envelope WsAPIResponse
			require.NoError(t, json.Unmarshal(response(tc.name), &envelope), "observed response must decode")
			assertResponseFields(t, envelope.Result, reflect.TypeOf(result), tc.name)
		})
	}
}
