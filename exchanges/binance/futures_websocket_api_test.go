package binance

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
)

func mockFuturesAPI(t *testing.T, a asset.Item, handler mockws.WsMockFunc) *Exchange {
	t.Helper()
	server, dialer := mockws.NewTestServer(t, mockws.CurryWsMockUpgrader(t, handler))
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	local.SkipAuthCheck = true
	local.SetCredentials(&accounts.Credentials{Key: "test-key", Secret: "test-secret"})
	require.NoError(t, local.CurrencyPairs.SetAssetEnabled(a, true), "Futures asset must enable")
	local.Websocket = websocket.NewManager()
	local.Websocket.DataHandler = stream.NewRelay(64)
	require.NoError(t, local.Websocket.Setup(&websocket.ManagerSetup{ExchangeConfig: local.Config, Features: &local.Features.Supports.WebsocketCapabilities, UseMultiConnectionManagement: true}), "mock manager must initialise")
	setup := local.futuresAPIConnectionSetup(a)
	setup.URL = "ws" + strings.TrimPrefix(server.URL, "http")
	setup.ResponseMaxLimit = 2 * time.Second
	connector := setup.Connector
	setup.Connector = func(ctx context.Context, conn websocket.Connection) error {
		return connector(ctx, &mockDialConnection{Connection: conn, dialer: dialer})
	}
	require.NoError(t, local.Websocket.SetupNewConnection(setup), "production API setup must register")
	t.Cleanup(func() {
		if local.Websocket.IsEnabled() {
			assert.NoError(t, local.Websocket.Disable(), "mock manager should disable")
		}
		if local.Websocket.IsConnected() {
			assert.NoError(t, local.Websocket.Shutdown(), "mock connection should close")
		}
	})
	require.NoError(t, local.Websocket.Connect(t.Context()), "mock API must connect through the production connector")
	return local
}

func TestDocumentedFuturesWebsocketRequests(t *testing.T) {
	raw, err := os.ReadFile("testdata/documented_futures_websocket.json")
	require.NoError(t, err, "Futures WebSocket fixtures must load")
	var fixtures map[string]struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Data   json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixtures), "Futures WebSocket fixtures must decode")
	for _, tc := range []struct {
		name     string
		asset    asset.Item
		call     func(*testing.T, *Exchange, []byte) (any, error)
		validate func(*testing.T, *Exchange, []byte)
	}{
		{"coin/account.status", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsCFuturesGetAccountRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsCFuturesGetAccount(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"coin/account.balance", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsCFuturesGetBalancesRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsCFuturesGetBalances(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"coin/order.cancel", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsCFuturesCancelOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsCFuturesCancelOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsCFuturesCancelOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsCFuturesCancelOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsCFuturesCancelOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesCancelOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.OrderID = 0
				arg.OriginalClientOrderID = ""
				_, err := e.WsCFuturesCancelOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "missing identifiers should fail offline")
			}
		}},
		{"coin/order.modify", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsCFuturesModifyOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsCFuturesModifyOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsCFuturesModifyOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsCFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsCFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Side = order.UnknownSide
				_, err := e.WsCFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrSideIsInvalid, "missing side should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Quantity = 0
				_, err := e.WsCFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, limits.ErrAmountBelowMin, "missing quantity should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Price = 0
				arg.PriceMatch = ""
				_, err := e.WsCFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, limits.ErrPriceBelowMin, "missing price should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.OrderID = 0
				arg.OriginalClientOrderID = ""
				_, err := e.WsCFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "missing identifiers should fail offline")
			}
			{
				arg := new(WsCFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Price = 1
				_, err := e.WsCFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting pricing modes should fail offline")
			}
		}},
		{"coin/order.place", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsCFuturesNewOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsCFuturesNewOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsCFuturesNewOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsCFuturesNewOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsCFuturesNewOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesNewOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Side = order.UnknownSide
				_, err := e.WsCFuturesNewOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrSideIsInvalid, "missing side should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesNewOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.OrderType = ""
				_, err := e.WsCFuturesNewOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrTypeIsInvalid, "missing type should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesNewOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Price = 1
				_, err := e.WsCFuturesNewOrder(t.Context(), arg)
				assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting pricing modes should fail offline")
			}
		}},
		{"coin/account.position", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsCFuturesGetPositionsRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsCFuturesGetPositions(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"coin/order.status", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsCFuturesGetOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsCFuturesGetOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsCFuturesGetOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsCFuturesGetOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsCFuturesGetOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsCFuturesGetOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.OrderID = 0
				arg.OriginalClientOrderID = ""
				_, err := e.WsCFuturesGetOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "missing identifiers should fail offline")
			}
		}},
		{"coin/userDataStream.stop", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, _ []byte) (any, error) {
			t.Helper()
			return nil, e.WsCFuturesCloseUserDataStream(t.Context())
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"coin/userDataStream.ping", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, _ []byte) (any, error) {
			t.Helper()
			return e.WsCFuturesKeepUserDataStreamAlive(t.Context())
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"coin/userDataStream.start", asset.CoinMarginedFutures, func(t *testing.T, e *Exchange, _ []byte) (any, error) {
			t.Helper()
			return e.WsCFuturesStartUserDataStream(t.Context())
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/account.status", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetAccountRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetAccount(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/v2/account.status", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetAccountV2Request)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetAccountV2(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/account.balance", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetBalancesRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetBalances(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/v2/account.balance", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetBalancesV2Request)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetBalancesV2(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/depth", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetOrderbookRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetOrderbook(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsUFuturesGetOrderbook(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsUFuturesGetOrderbookRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsUFuturesGetOrderbook(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
		}},
		{"usdt/ticker.book", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetBookTickerRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetBookTicker(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/ticker.price", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetPriceTickerRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetPriceTicker(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/algoOrder.cancel", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesCancelAlgoOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesCancelAlgoOrder(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/order.cancel", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesCancelOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesCancelOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsUFuturesCancelOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsUFuturesCancelOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsUFuturesCancelOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesCancelOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.OrderID = 0
				arg.OriginalClientOrderID = ""
				_, err := e.WsUFuturesCancelOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "missing identifiers should fail offline")
			}
		}},
		{"usdt/order.modify", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesModifyOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesModifyOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsUFuturesModifyOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsUFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsUFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Side = order.UnknownSide
				_, err := e.WsUFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrSideIsInvalid, "missing side should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Quantity = 0
				_, err := e.WsUFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, limits.ErrAmountBelowMin, "missing quantity should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Price = 0
				arg.PriceMatch = ""
				_, err := e.WsUFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, limits.ErrPriceBelowMin, "missing price should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.OrderID = 0
				arg.OriginalClientOrderID = ""
				_, err := e.WsUFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "missing identifiers should fail offline")
			}
			{
				arg := new(WsUFuturesModifyOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Price = 1
				_, err := e.WsUFuturesModifyOrder(t.Context(), arg)
				assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting pricing modes should fail offline")
			}
		}},
		{"usdt/algoOrder.place", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesNewAlgoOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			arg.GoodTillDate = time.UnixMilli(1750500000000)
			return e.WsUFuturesNewAlgoOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsUFuturesNewAlgoOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsUFuturesNewAlgoOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.AlgoType = ""
				_, err := e.WsUFuturesNewAlgoOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrTypeIsInvalid, "missing algoType should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesNewAlgoOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsUFuturesNewAlgoOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesNewAlgoOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.Side = order.UnknownSide
				_, err := e.WsUFuturesNewAlgoOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrSideIsInvalid, "missing side should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesNewAlgoOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.OrderType = ""
				_, err := e.WsUFuturesNewAlgoOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrTypeIsInvalid, "missing type should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesNewAlgoOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.Price = 1
				_, err := e.WsUFuturesNewAlgoOrder(t.Context(), arg)
				assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting pricing modes should fail offline")
			}
		}},
		{"usdt/order.place", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesNewOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			arg.GoodTillDate = time.UnixMilli(1750500000000)
			return e.WsUFuturesNewOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsUFuturesNewOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsUFuturesNewOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsUFuturesNewOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesNewOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.Side = order.UnknownSide
				_, err := e.WsUFuturesNewOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrSideIsInvalid, "missing side should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesNewOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.OrderType = ""
				_, err := e.WsUFuturesNewOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrTypeIsInvalid, "missing type should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesNewOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.GoodTillDate = time.UnixMilli(1750500000000)
				arg.Price = 1
				_, err := e.WsUFuturesNewOrder(t.Context(), arg)
				assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "conflicting pricing modes should fail offline")
			}
		}},
		{"usdt/account.position", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetPositionsRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetPositions(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/v2/account.position", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetPositionsV2Request)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetPositionsV2(t.Context(), arg)
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/order.status", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, data []byte) (any, error) {
			t.Helper()
			arg := new(WsUFuturesGetOrderRequest)
			require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
			return e.WsUFuturesGetOrder(t.Context(), arg)
		}, func(t *testing.T, e *Exchange, data []byte) {
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			t.Helper()
			_, err := e.WsUFuturesGetOrder(t.Context(), nil)
			assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before transport")
			{
				arg := new(WsUFuturesGetOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.Symbol = currency.EMPTYPAIR
				_, err := e.WsUFuturesGetOrder(t.Context(), arg)
				assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "missing symbol should retain its validation sentinel")
			}
			{
				arg := new(WsUFuturesGetOrderRequest)
				require.NoError(t, json.Unmarshal(data, arg), "synthetic documented parameters must decode")
				arg.OrderID = 0
				arg.OriginalClientOrderID = ""
				_, err := e.WsUFuturesGetOrder(t.Context(), arg)
				assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "missing identifiers should fail offline")
			}
		}},
		{"usdt/userDataStream.stop", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, _ []byte) (any, error) {
			t.Helper()
			return nil, e.WsUFuturesCloseUserDataStream(t.Context())
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/userDataStream.ping", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, _ []byte) (any, error) {
			t.Helper()
			return e.WsUFuturesKeepUserDataStreamAlive(t.Context())
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
		{"usdt/userDataStream.start", asset.USDTMarginedFutures, func(t *testing.T, e *Exchange, _ []byte) (any, error) {
			t.Helper()
			return e.WsUFuturesStartUserDataStream(t.Context())
		}, func(_ *testing.T, _ *Exchange, _ []byte) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := fixtures[tc.name]
			local := mockFuturesAPI(t, tc.asset, func(tb testing.TB, data []byte, conn *gws.Conn) error {
				tb.Helper()
				var req struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				require.NoError(tb, json.Unmarshal(data, &req), "wire request must decode")
				assert.Equal(tb, fixture.Method, req.Method, "wire method should match its documentation")
				var params map[string]json.RawMessage
				switch {
				case strings.HasPrefix(req.Method, "userDataStream."):
					require.NoError(tb, json.Unmarshal(req.Params, &params), "API-key parameters must decode")
					assert.JSONEq(tb, `"test-key"`, string(params["apiKey"]), "stream request should carry only its API key")
					delete(params, "apiKey")
				case req.Method == "depth" || strings.HasPrefix(req.Method, "ticker."):
					require.NoError(tb, json.Unmarshal(req.Params, &params), "public parameters must decode")
				default:
					params = assertHMACParams(tb, req.Params)
					delete(params, "timestamp")
					delete(params, "signature")
					delete(params, "apiKey")
				}
				expected := make(map[string]json.RawMessage)
				require.NoError(tb, json.Unmarshal(fixture.Params, &expected), "fixture inputs must decode")
				// Optional price zero is omitted when priceMatch selects the price.
				if expected["price"] != nil && string(expected["price"]) == "0" {
					delete(expected, "price")
				}
				if params["price"] != nil && string(params["price"]) == "0" {
					delete(params, "price")
				}
				assert.Equal(tb, expected, params, "every documented input should reach the wire unchanged")
				var response map[string]json.RawMessage
				require.NoError(tb, json.Unmarshal(fixture.Data, &response), "documented envelope must decode")
				response["id"] = req.ID
				return conn.WriteJSON(response)
			})
			var decodeParams map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(fixture.Params, &decodeParams), "input fixture must decode")
			delete(decodeParams, "goodTillDate")
			if tc.asset == asset.USDTMarginedFutures && decodeParams["symbol"] != nil {
				decodeParams["symbol"] = json.RawMessage(`"BTC-USDT"`)
			}
			if decodeParams["pair"] != nil {
				decodeParams["pair"] = json.RawMessage(`"BTC-USD"`)
			}
			encoded, err := json.Marshal(decodeParams)
			require.NoError(t, err, "test inputs must encode")
			result, err := tc.call(t, local, encoded)
			require.NoError(t, err, "documented response must decode through its actual request method")
			var envelope WsAPIResponse
			require.NoError(t, json.Unmarshal(fixture.Data, &envelope), "response fixture must decode")
			if result != nil {
				assertResponseFields(t, envelope.Result, reflect.TypeOf(result), tc.name)
			}
			tc.validate(t, local, encoded)
		})
	}
}
