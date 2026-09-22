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
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

type publicWSRecording struct {
	ObservedAt time.Time                  `json:"observedAt"`
	Responses  map[string]json.RawMessage `json:"responses"`
}

// TestPublicWebsocketAPILive exercises the actual GCT public request methods against Binance.
func TestPublicWebsocketAPILive(t *testing.T) {
	if os.Getenv("BINANCE_LIVE_PUBLIC") != "1" {
		t.Skip("set BINANCE_LIVE_PUBLIC=1 to check production public WebSocket APIs")
	}
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "public exchange must initialise")
	local.API.AuthenticatedSupport = false
	local.API.AuthenticatedWebsocketSupport = false
	local.Websocket = websocket.NewManager()
	local.Websocket.DataHandler = stream.NewRelay(64)
	require.NoError(t, local.Websocket.Setup(&websocket.ManagerSetup{ExchangeConfig: local.Config, Features: &local.Features.Supports.WebsocketCapabilities, UseMultiConnectionManagement: true}), "public manager must initialise")
	setup := local.spotAPIConnectionSetup()
	setup.Authenticate = nil
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
	require.NoError(t, local.Websocket.SetupNewConnection(setup), "public API connection must register")
	require.NoError(t, local.Websocket.Connect(t.Context()), "public API must connect")
	t.Cleanup(func() {
		if local.Websocket.IsEnabled() {
			assert.NoError(t, local.Websocket.Disable(), "public manager should disable")
		}
		if local.Websocket.IsConnected() {
			assert.NoError(t, local.Websocket.Shutdown(), "public manager should shut down")
		}
		if path := os.Getenv("BINANCE_RECORD_PUBLIC_WS"); path != "" {
			mu.Lock()
			defer mu.Unlock()
			data, err := json.MarshalIndent(recording, "", " ")
			require.NoError(t, err, "public WebSocket responses must serialise")
			require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600), "public WebSocket recording must save")
		}
	})
	runPublicWSChecks(t, local, &current, func(name string) json.RawMessage {
		mu.Lock()
		defer mu.Unlock()
		return recording.Responses[name]
	})
}

// TestPublicWebsocketAPIRecorded replays the live public responses through mockws and GCT request builders.
func TestPublicWebsocketAPIRecorded(t *testing.T) {
	data, err := os.ReadFile("testdata/public_api/websocket_api.json")
	require.NoError(t, err, "public WebSocket recording must load")
	var recording publicWSRecording
	require.NoError(t, json.Unmarshal(data, &recording), "public recording must decode")
	var current atomic.Pointer[string]
	local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
		tb.Helper()
		var request struct {
			ID     json.RawMessage            `json:"id"`
			Params map[string]json.RawMessage `json:"params"`
		}
		require.NoError(tb, json.Unmarshal(data, &request), "public request must decode")
		assert.NotContains(tb, request.Params, "apiKey", "public requests should omit credentials")
		assert.NotContains(tb, request.Params, "signature", "public requests should omit signatures")
		var response map[string]json.RawMessage
		require.NoError(tb, json.Unmarshal(recording.Responses[*current.Load()], &response), "recorded response must decode")
		response["id"] = request.ID
		return conn.WriteJSON(response)
	})
	runPublicWSChecks(t, local, &current, func(name string) json.RawMessage { return recording.Responses[name] })
}

func runPublicWSChecks(t *testing.T, local *Exchange, current *atomic.Pointer[string], response func(string) json.RawMessage) {
	t.Helper()
	pair := currency.NewBTCUSDT()
	pairs := currency.Pairs{currency.NewPairWithDelimiter("BTC", "USDT", "-"), currency.NewPairWithDelimiter("ETH", "USDT", "-")}
	for _, tc := range []struct {
		name string
		call func() (any, error)
	}{
		{"ping", func() (any, error) { return &struct{}{}, local.WsPing() }},
		{"time", func() (any, error) { return local.GetWsServerTime() }},
		{"exchangeInfo", func() (any, error) { return local.GetWsExchangeInfo(&GetExchangeInfoRequest{Symbol: pair}) }},
		{"executionRules", func() (any, error) { return local.GetWsExecutionRules(&WsMarketSymbolsRequest{Symbol: pair}) }},
		{"depth", func() (any, error) { return local.GetWsOrderbook(&OrderBookDataRequest{Symbol: pair, Limit: 5}) }},
		{"trades.recent", func() (any, error) { return local.GetWsMostRecentTrades(&RecentTradeRequest{Symbol: pair, Limit: 5}) }},
		{"trades.historical", func() (any, error) { return local.GetWsHistoricalTrades(pair, 0, 5) }},
		{"trades.aggregate", func() (any, error) {
			return local.GetWsAggregatedTrades(&WsAggregateTradeRequest{Symbol: pair, Limit: 5})
		}},
		{"blockTrades.historical", func() (any, error) { return local.GetWsHistoricalBlockTrades(pair, 0, 5) }},
		{"klines", func() (any, error) {
			return local.GetWsCandlestick(&KlinesRequest{Symbol: pair, Interval: "1m", Limit: 5})
		}},
		{"uiKlines", func() (any, error) {
			return local.GetWsOptimizedCandlestick(&KlinesRequest{Symbol: pair, Interval: "1m", Limit: 5})
		}},
		{"avgPrice", func() (any, error) { return local.GetWsCurrenctAveragePrice(pair) }},
		{"referencePrice", func() (any, error) { return local.GetWsReferencePrice(pair) }},
		{"referencePrice.calculation", func() (any, error) { return local.GetWsReferencePriceCalculation(pair, "") }},
		{"ticker", func() (any, error) {
			return local.GetWsRollingWindowPriceChanges(&WsRollingWindowPriceRequest{Symbols: pairs, WindowSizeDuration: time.Hour, SymbolStatus: "TRADING"})
		}},
		{"ticker.24hr", func() (any, error) {
			return local.GetWs24HourPriceChanges(&PriceChangeRequest{Symbols: pairs, SymbolStatus: "TRADING"})
		}},
		{"ticker.tradingDay", func() (any, error) {
			return local.GetWsTradingDayTickers(&PriceChangeRequest{Symbols: pairs, Timezone: "0", SymbolStatus: "TRADING"})
		}},
		{"ticker.price/single", func() (any, error) { return local.GetSymbolPriceTicker(pair) }},
		{"ticker.price/multiple", func() (any, error) {
			return local.GetSymbolPriceTicker(currency.EMPTYPAIR, &WsMarketSymbolsRequest{Symbols: pairs, SymbolStatus: "TRADING"})
		}},
		{"ticker.price/all", func() (any, error) { return local.GetSymbolPriceTicker(currency.EMPTYPAIR) }},
		{"ticker.book/single", func() (any, error) { return local.GetWsSymbolOrderbookTicker(currency.Pairs{pair}) }},
		{"ticker.book/multiple", func() (any, error) { return local.GetWsSymbolOrderbookTicker(pairs, "TRADING") }},
		{"ticker.book/all", func() (any, error) { return local.GetWsSymbolOrderbookTicker(nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current.Store(&tc.name)
			result, err := tc.call()
			require.NoError(t, err, "public request must return a decodable response")
			var envelope WsAPIResponse
			require.NoError(t, json.Unmarshal(response(tc.name), &envelope), "observed envelope must decode")
			assertResponseFields(t, envelope.Result, reflect.TypeOf(result), tc.name)
			if clock, ok := result.(*WsServerTimeResponse); ok {
				assert.False(t, clock.ServerTime.Time().IsZero(), "server time should be populated")
			}
		})
	}
}
