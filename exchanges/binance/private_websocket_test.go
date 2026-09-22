package binance

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
)

func TestDocumentedPrivateStreamEvents(t *testing.T) {
	data, err := os.ReadFile("testdata/documented_private_streams.json")
	require.NoError(t, err, "private stream fixtures must load")
	var fixtures []struct {
		Asset asset.Item      `json:"asset"`
		Model string          `json:"model"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(data, &fixtures), "private stream fixtures must decode")
	for _, fixture := range fixtures {
		t.Run(fixture.Asset.String()+"/"+fixture.Model, func(t *testing.T) {
			local := new(Exchange)
			require.NoError(t, testexch.Setup(local), "test exchange must initialise")
			local.SkipAuthCheck = true
			local.SetCredentials(&accounts.Credentials{Key: "test-key", Secret: "test-secret"})
			local.Websocket.DataHandler = stream.NewRelay(64)
			var model any
			switch fixture.Model {
			case "AccountUpdate":
				if fixture.Asset == asset.Options {
					model = new(OptionsAccountUpdate)
				} else {
					model = new(WSBalanceAndPositionUpdate)
				}
			case "BalancePositionUpdate":
				model = new(OptionsBalancePositionUpdate)
			case "GreekUpdate":
				model = new(OptionsGreekUpdate)
			case "RiskLevelChange":
				model = new(OptionsRiskLevelChange)
			case "AccountConfigUpdate":
				model = new(FuturesAccountConfigUpdate)
			case "AlgoUpdate":
				model = new(FuturesAlgoUpdate)
			case "ConditionalOrderTriggerReject":
				model = new(FuturesConditionalOrderReject)
			case "GridUpdate":
				model = new(FuturesGridUpdate)
			case "ListenKeyExpired":
				model = new(FuturesListenKeyExpired)
			case "MarginCall":
				model = new(FuturesMarginCall)
			case "OrderTradeUpdate":
				model = new(FuturesOrderTradeUpdate)
			case "StrategyUpdate":
				model = new(FuturesStrategyUpdate)
			case "TradeLite":
				model = new(FuturesTradeLite)
			default:
				t.Fatalf("fixture model %s must have a typed decoder", fixture.Model)
			}
			require.NoError(t, json.Unmarshal(fixture.Data, model), "all documented event fields must decode")
			assertResponseFields(t, fixture.Data, reflect.TypeOf(model), fixture.Model)
			if update, ok := model.(*FuturesOrderTradeUpdate); ok {
				pair := currency.NewBTCUSDT()
				switch fixture.Asset {
				case asset.CoinMarginedFutures:
					pair = currency.NewPairWithDelimiter("BTCUSD", "PERP", currency.UnderscoreDelimiter)
				case asset.Options:
					pair = currency.Pair{Base: currency.BTC, Quote: currency.NewCode("270101-100000-C"), Delimiter: currency.DashDelimiter}
				}
				require.NoError(t, local.CurrencyPairs.StorePairs(fixture.Asset, currency.Pairs{pair}, false), "documented order symbol must be available")
				assert.Equal(t, uint64(9007199254740993), update.Order.OrderID, "large integer order IDs should remain exact")
			}
			setup := local.privateStreamSetup(fixture.Asset)
			require.NoError(t, setup.Handler(t.Context(), nil, fixture.Data), "private event must pass through the production handler")
			select {
			case event := <-local.Websocket.DataHandler.C:
				if fixture.Model == "OrderTradeUpdate" {
					detail, ok := event.Data.(*order.Detail)
					require.True(t, ok, "trade event must become an order detail")
					assert.Equal(t, "9007199254740993", detail.OrderID, "order detail should retain the exact ID")
					assert.Equal(t, fixture.Asset, detail.AssetType, "order detail should retain its product")
				} else if fixture.Model != "AccountUpdate" || fixture.Asset == asset.Options {
					assert.IsType(t, model, event.Data, "handler should emit the documented event type")
				}
			case <-time.After(time.Second):
				t.Fatal("private event must be emitted")
			}
		})
	}
}

func TestPrivateStreamLifecycle(t *testing.T) {
	for _, tc := range []struct {
		asset                         asset.Item
		restPath, wsPath, event, kind string
	}{
		{asset.USDTMarginedFutures, "/fapi/v1/listenKey", "/private/ws/test-listen-key", `{"e":"ACCOUNT_CONFIG_UPDATE","E":1611646737479,"T":1611646737476,"ac":{"s":"BTCUSDT","l":25}}`, ""},
		{asset.CoinMarginedFutures, "/dapi/v1/listenKey", "/ws/test-listen-key", `{"e":"ACCOUNT_CONFIG_UPDATE","E":1611646737479,"T":1611646737476,"ac":{"s":"BTCUSD_PERP","l":25}}`, ""},
		{asset.Options, "/eapi/v1/listenKey", "/private/ws/test-listen-key", `{"e":"ACCOUNT_UPDATE","E":1750515742303,"T":1750515742297,"eq":"1000.25","aeq":"999.25","b":"1000.00","m":"0.25","u":"0.25","i":"10.25","M":"1.25"}`, ""},
		{asset.Margin, "/papi/v1/listenKey", "/pm/ws/test-listen-key", `{"e":"ACCOUNT_CONFIG_UPDATE","fs":"UM","E":1611646737479,"T":1611646737476,"ac":{"s":"BTCUSDT","l":25}}`, "portfolio"},
		{asset.USDTMarginedFutures, "/fapi/v1/listenKey", "/pm-classic/ws/test-listen-key", `{"e":"PM_PRO_ACCOUNT_UPDATE","E":1750515742303,"u":"1.25","eq":"1000","ae":"999","im":"10","mm":"5","avb":"900","vmw":"800"}`, "portfolio-pro"},
		{asset.Margin, "/sapi/v1/margin/listen-key", "/ws/test-listen-key", `{"e":"MARGIN_LEVEL_STATUS_CHANGE","E":1750515742303,"l":"1.25","s":"MARGIN_CALL"}`, "margin-risk"},
	} {
		t.Run(tc.asset.String()+"/"+tc.kind, func(t *testing.T) {
			requests := make(chan string, 16)
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.restPath, r.URL.Path, "listen key should use the product REST endpoint")
				if tc.kind == "margin-risk" && r.Method == http.MethodPut {
					assert.Equal(t, "listenKey=test-listen-key", r.URL.RawQuery, "margin risk renewal should retain the key")
				} else {
					assert.Empty(t, r.URL.RawQuery, "listen key should use API-key authentication without signed parameters")
				}
				assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "listen key should use the test API key")
				requests <- r.Method
				if r.Method == http.MethodPost {
					_, _ = w.Write([]byte(`{"listenKey":"test-listen-key"}`))
				} else {
					_, _ = w.Write([]byte(`{}`))
				}
			})
			require.NoError(t, local.CurrencyPairs.SetAssetEnabled(tc.asset, true), "stream asset must be enabled")
			serverConnections := make(chan *gws.Conn, 1)
			serverClosed := make(chan struct{})
			server, dialer := mockws.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.wsPath, r.URL.Path, "private connection should include the correct product path and listen key")
				upgrader := gws.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				if !assert.NoError(t, err, "private mock stream should upgrade") {
					return
				}
				defer conn.Close()
				defer close(serverClosed)
				if !assert.NoError(t, conn.WriteMessage(gws.TextMessage, []byte(tc.event)), "documented event should be sent without a subscription request") {
					return
				}
				serverConnections <- conn
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
			}))
			local.Websocket = websocket.NewManager()
			local.SkipAuthCheck = true
			local.SetCredentials(&accounts.Credentials{Key: "test-key", Secret: "test-secret"})
			local.Websocket.DataHandler = stream.NewRelay(64)
			require.NoError(t, local.Websocket.Setup(&websocket.ManagerSetup{ExchangeConfig: local.Config, Features: &local.Features.Supports.WebsocketCapabilities, UseMultiConnectionManagement: true}), "private manager must initialise")
			setup := local.privateStreamSetup(tc.asset)
			switch tc.kind {
			case "portfolio":
				setup = local.PortfolioMarginStreamSetup()
			case "portfolio-pro":
				setup = local.PortfolioMarginProStreamSetup()
			case "margin-risk":
				setup = local.MarginRiskStreamSetup()
			}
			setup.URL = "ws" + strings.TrimPrefix(server.URL, "http") + strings.TrimSuffix(tc.wsPath, "/ws/test-listen-key")
			connected := make(chan websocket.Connection, 1)
			connector := setup.Connector
			setup.Connector = func(ctx context.Context, conn websocket.Connection) error {
				mockConnection := &mockDialConnection{Connection: conn, dialer: dialer}
				if err := connector(ctx, mockConnection); err != nil {
					return err
				}
				connected <- conn
				return nil
			}
			require.NoError(t, local.Websocket.SetupNewConnection(setup), "private connection must register")
			t.Cleanup(func() {
				if local.Websocket.IsEnabled() {
					assert.NoError(t, local.Websocket.Disable(), "private manager should disable")
				}
				if local.Websocket.IsConnected() {
					assert.NoError(t, local.Websocket.Shutdown(), "private manager should shut down")
				}
			})
			require.NoError(t, local.Websocket.Connect(t.Context()), "private stream must connect through the manager")
			assert.Equal(t, http.MethodPost, <-requests, "startup should create a listen key")
			select {
			case event := <-local.Websocket.DataHandler.C:
				switch {
				case tc.kind == "portfolio-pro":
					assert.IsType(t, new(PortfolioMarginProAccountUpdate), event.Data, "PM Pro event should decode")
				case tc.kind == "margin-risk":
					assert.IsType(t, new(MarginLevelStatusChange), event.Data, "margin risk event should decode")
				case tc.asset == asset.Options:
					assert.IsType(t, new(OptionsAccountUpdate), event.Data, "options event should decode")
				default:
					assert.IsType(t, new(FuturesAccountConfigUpdate), event.Data, "futures event should decode")
				}
			case <-time.After(time.Second):
				t.Fatal("private stream must emit its event")
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch tc.kind {
			case "margin-risk":
				local.maintainListenKey(ctx, <-connected, "margin risk", time.Millisecond, func(ctx context.Context) error {
					return local.KeepMarginListenKeyAlive(ctx, "test-listen-key")
				})
			case "portfolio":
				local.maintainListenKey(ctx, <-connected, "portfolio margin", time.Millisecond, func(ctx context.Context) error {
					_, err := local.KeepPortfolioMarginListenKeyAlive(ctx)
					return err
				})
			default:
				local.maintainPrivateStream(ctx, <-connected, tc.asset, time.Millisecond)
			}
			select {
			case method := <-requests:
				assert.Equal(t, http.MethodPut, method, "renewal should keep the correct product key alive")
			case <-time.After(time.Second):
				t.Fatal("private stream must renew its key")
			}
			cancel()
			serverConn := <-serverConnections
			require.NoError(t, serverConn.WriteMessage(gws.TextMessage, []byte(`{"e":"listenKeyExpired","E":1750515742303,"listenKey":"test-listen-key"}`)), "expiry event must send")
			select {
			case <-serverClosed:
			case <-time.After(time.Second):
				t.Fatal("expired private stream must close so the manager can reconnect")
			}
		})
	}
}

func TestPrivateStreamUnknownEvent(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	for _, a := range []asset.Item{asset.USDTMarginedFutures, asset.CoinMarginedFutures, asset.Options} {
		var err error
		if a == asset.Options {
			err = local.wsHandleOptionsUserData(t.Context(), "unknown", []byte(`{"e":"unknown"}`))
		} else {
			err = local.privateStreamSetup(a).Handler(t.Context(), nil, []byte(`{"e":"unknown"}`))
		}
		assert.ErrorIs(t, err, errUnsupportedChannel, "unknown private event should retain its sentinel")
	}
}
