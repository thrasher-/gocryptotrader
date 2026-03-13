package kraken

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func spotResult(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"error":[],"result":`+result+`}`)
}

func futuresResult(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, result)
}

type restMockRoute struct {
	pathPrefix string
	handler    http.HandlerFunc
}

func mockRESTRoute(pathPrefix string, handler http.HandlerFunc) restMockRoute {
	return restMockRoute{
		pathPrefix: pathPrefix,
		handler:    handler,
	}
}

func mockPublicResult(method, result string) restMockRoute {
	return mockRESTRoute("/0/public/"+method, func(w http.ResponseWriter, _ *http.Request) {
		spotResult(w, result)
	})
}

func mockPrivateResult(method, result string) restMockRoute {
	return mockRESTRoute("/0/private/"+method, func(w http.ResponseWriter, _ *http.Request) {
		spotResult(w, result)
	})
}

func mockRawResult(pathPrefix, contentType, body string) restMockRoute {
	return mockRESTRoute(pathPrefix, func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = io.WriteString(w, body)
	})
}

func mockFuturesResponse(pathPrefix string) restMockRoute {
	return mockRESTRoute(pathPrefix, func(w http.ResponseWriter, _ *http.Request) {
		futuresResult(w, `{"result":"success"}`)
	})
}

func restMockHandler(routes []restMockRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for i := range routes {
			if strings.HasPrefix(r.URL.Path, routes[i].pathPrefix) {
				routes[i].handler(w, r)
				return
			}
		}
		http.NotFound(w, r)
	}
}

func newMockedKraken(t *testing.T, routes ...restMockRoute) *Exchange {
	t.Helper()
	return newMockedKrakenWithRESTHandler(t, restMockHandler(routes))
}

func newMockedKrakenWithRESTHandler(t *testing.T, handler http.HandlerFunc) *Exchange {
	t.Helper()

	k := new(Exchange)
	require.NoError(t, testexch.Setup(k), "Setup must not error")

	k.SkipAuthCheck = true
	k.SetCredentials("test", "c2VjcmV0", "", "", "", "")

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	require.NoError(t, k.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL rest spot must not error")
	require.NoError(t, k.API.Endpoints.SetRunningURL(exchange.RestFutures.String(), server.URL), "SetRunningURL rest futures must not error")
	require.NoError(t, k.API.Endpoints.SetRunningURL(exchange.RestFuturesSupplementary.String(), server.URL+"/"), "SetRunningURL rest futures supplementary must not error")

	return k
}

func copyValues(values url.Values) url.Values {
	copied := make(url.Values, len(values))
	for key, value := range values {
		copied[key] = slices.Clone(value)
	}
	return copied
}

func decodeBodyValues(t *testing.T, r *http.Request) url.Values {
	t.Helper()

	payload, err := io.ReadAll(r.Body)
	require.NoError(t, err, "ReadAll must not error")

	values, err := url.ParseQuery(string(payload))
	require.NoError(t, err, "ParseQuery must not error")
	return values
}

func capturePublicQueryValues(
	t *testing.T,
	pathPrefix string,
	result string,
	invoke func(*Exchange) error,
) url.Values {
	t.Helper()

	captured := make(chan url.Values, 1)
	k := newMockedKraken(t, mockRESTRoute(pathPrefix, func(w http.ResponseWriter, r *http.Request) {
		captured <- copyValues(r.URL.Query())
		spotResult(w, result)
	}))

	require.NoError(t, invoke(k), "invoke must not error")
	return <-captured
}

func capturePrivateBodyValues(
	t *testing.T,
	pathPrefix string,
	result string,
	invoke func(*Exchange) error,
) url.Values {
	t.Helper()

	captured := make(chan url.Values, 1)
	k := newMockedKraken(t, mockRESTRoute(pathPrefix, func(w http.ResponseWriter, r *http.Request) {
		captured <- copyValues(decodeBodyValues(t, r))
		spotResult(w, result)
	}))

	require.NoError(t, invoke(k), "invoke must not error")
	return <-captured
}

func mockedFuturesPair() currency.Pair {
	return currency.NewPairWithDelimiter("PF", "XBTUSD", "_")
}

func TestMockedGetSystemStatus(t *testing.T) {
	k := newMockedKraken(t, mockPublicResult("SystemStatus", `{"status":"online","timestamp":"2025-01-01T00:00:00Z"}`))
	_, err := k.GetSystemStatus(t.Context())
	require.NoError(t, err)
}

func TestMockedGetGroupedOrderBook(t *testing.T) {
	k := newMockedKraken(t, mockPublicResult("GroupedBook", `{"pair":"XBT/USD","grouping":1,"bids":[],"asks":[]}`))
	_, err := k.GetGroupedOrderBook(t.Context(), &GroupedOrderBookRequest{Pair: spotTestPair, Depth: 10, Grouping: 1})
	require.NoError(t, err)
}

func TestMockedQueryLevel3OrderBook(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("Level3", `{"pair":"XBT/USD","bids":[{"price":"100.1","qty":"1.5","order_id":"bid-1","timestamp":1704067200000000000}],"asks":[{"price":"100.2","qty":"2.5","order_id":"ask-1","timestamp":1704067201000000000}]}`))
	resp, err := k.QueryLevel3OrderBook(t.Context(), &QueryLevel3OrderBookRequest{Pair: spotTestPair, Depth: 5})
	require.NoError(t, err)
	assert.Equal(t, &QueryLevel3OrderBookResponse{
		Pair: "XBT/USD",
		Bids: []Level3OrderBookEntry{{
			Price:     types.Number(100.1),
			Quantity:  types.Number(1.5),
			OrderID:   "bid-1",
			Timestamp: types.Time(time.Unix(0, 1704067200000000000)),
		}},
		Asks: []Level3OrderBookEntry{{
			Price:     types.Number(100.2),
			Quantity:  types.Number(2.5),
			OrderID:   "ask-1",
			Timestamp: types.Time(time.Unix(0, 1704067201000000000)),
		}},
	}, resp)
}

func TestMockedGetAccountBalance(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("Balance", `{"XXBT":"0.1"}`))
	_, err := k.GetAccountBalance(t.Context(), &GetAccountBalanceRequest{})
	require.NoError(t, err)
}

func TestMockedGetExtendedBalance(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("BalanceEx", `{"XXBT":{"balance":"0.1","hold_trade":"0.01"}}`))
	_, err := k.GetExtendedBalance(t.Context(), &GetExtendedBalanceRequest{})
	require.NoError(t, err)
}

func TestMockedGetBalance(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("BalanceEx", `{"XXBT":{"balance":"0.1","hold_trade":"0.01"}}`))
	_, err := k.GetBalance(t.Context())
	require.NoError(t, err)
}

func TestGetAssetsRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePublicQueryValues(
		t,
		"/0/public/Assets",
		`{"XXBT":{"altname":"XBT","aclass_base":"currency","decimals":10,"display_decimals":5}}`,
		func(k *Exchange) error {
			_, err := k.GetAssets(t.Context(), &GetAssetsRequest{
				Asset:  "XBT",
				Aclass: "currency",
			})
			return err
		},
	)

	assert.Equal(t, "XBT", values.Get("asset"), "asset should be forwarded")
	assert.Equal(t, "currency", values.Get("aclass"), "aclass should be forwarded")
}

func TestGetAssetPairsRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePublicQueryValues(
		t,
		"/0/public/AssetPairs",
		`{"XXBTZUSD":{"altname":"XBTUSD","wsname":"XBT/USD","aclass_base":"currency","base":"XXBT","aclass_quote":"currency","quote":"ZUSD","lot":"unit","pair_decimals":1,"lot_decimals":8,"lot_multiplier":1,"leverage_buy":[],"leverage_sell":[],"fees":[],"fees_maker":[],"fee_volume_currency":"ZUSD","margin_call":80,"margin_stop":40,"ordermin":"0.0001","tick_size":"0.1","status":"online"}}`,
		func(k *Exchange) error {
			_, err := k.GetAssetPairs(t.Context(), &GetAssetPairsRequest{
				AssetPairs:     []string{"XXBTZUSD"},
				Info:           "info",
				AssetClassBase: "currency",
				CountryCode:    "US",
			})
			return err
		},
	)

	assert.Equal(t, "XXBTZUSD", values.Get("pair"), "pair should be forwarded")
	assert.Equal(t, "info", values.Get("info"), "info should be forwarded")
	assert.Equal(t, "currency", values.Get("aclass_base"), "aclass_base should be forwarded")
	assert.Equal(t, "US", values.Get("country_code"), "country_code should be forwarded")
}

func TestGetTickerRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePublicQueryValues(
		t,
		"/0/public/Ticker",
		`{"XXBTZUSD":{"a":["1","1","1"],"b":["1","1","1"],"c":["1","1"],"v":["1","1"],"p":["1","1"],"t":[1,1],"l":["1","1"],"h":["1","1"],"o":"1"}}`,
		func(k *Exchange) error {
			_, err := k.GetTicker(t.Context(), &GetTickerRequest{
				Pair:       spotTestPair,
				AssetClass: "currency",
			})
			return err
		},
	)

	assert.Equal(t, "currency", values.Get("asset_class"), "asset_class should be forwarded")
	assert.NotEmpty(t, values.Get("pair"), "pair should be forwarded")
}

func TestGetTickersRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePublicQueryValues(
		t,
		"/0/public/Ticker",
		`{"XXBTZUSD":{"a":["1","1","1"],"b":["1","1","1"],"c":["1","1"],"v":["1","1"],"p":["1","1"],"t":[1,1],"l":["1","1"],"h":["1","1"],"o":"1"}}`,
		func(k *Exchange) error {
			_, err := k.GetTickers(t.Context(), &GetTickersRequest{
				PairList:   "XXBTZUSD",
				AssetClass: "currency",
			})
			return err
		},
	)

	assert.Equal(t, "currency", values.Get("asset_class"), "asset_class should be forwarded")
	assert.Equal(t, "XXBTZUSD", values.Get("pair"), "pair should be forwarded")
}

func TestGetOHLCRequestParams(t *testing.T) {
	t.Parallel()

	since := time.Unix(1700000000, 0)
	values := capturePublicQueryValues(
		t,
		"/0/public/OHLC",
		`{"XBTUSD":[[1700000000,"1","2","0.5","1.5","1.2","5",10]],"XXBTZUSD":[[1700000000,"1","2","0.5","1.5","1.2","5",10]],"last":1700000000}`,
		func(k *Exchange) error {
			_, err := k.GetOHLC(t.Context(), &GetOHLCRequest{
				Pair:       spotTestPair,
				Interval:   "1",
				Since:      since,
				AssetClass: "currency",
			})
			return err
		},
	)

	assert.Equal(t, strconv.FormatInt(since.Unix(), 10), values.Get("since"), "since should be forwarded")
	assert.Equal(t, "currency", values.Get("asset_class"), "asset_class should be forwarded")
	assert.Equal(t, "1", values.Get("interval"), "interval should be forwarded")
}

func TestGetDepthRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePublicQueryValues(
		t,
		"/0/public/Depth",
		`{"XXBTZUSD":{"bids":[["1","1",1700000000]],"asks":[["2","1",1700000000]]}}`,
		func(k *Exchange) error {
			_, err := k.GetDepth(t.Context(), &GetDepthRequest{
				Pair:       spotTestPair,
				Count:      10,
				AssetClass: "currency",
			})
			return err
		},
	)

	assert.Equal(t, "10", values.Get("count"), "count should be forwarded")
	assert.Equal(t, "currency", values.Get("asset_class"), "asset_class should be forwarded")
}

func TestGetTradesRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePublicQueryValues(
		t,
		"/0/public/Trades",
		`{"XXBTZUSD":[["1","1",1700000000,"b","l","",1]],"last":1700000000}`,
		func(k *Exchange) error {
			_, err := k.GetTrades(t.Context(), &GetTradesRequest{
				Pair:       spotTestPair,
				Since:      time.Unix(1700000000, 0),
				Count:      25,
				AssetClass: "currency",
			})
			return err
		},
	)

	assert.Equal(t, "currency", values.Get("asset_class"), "asset_class should be forwarded")
	assert.Equal(t, "25", values.Get("count"), "count should be forwarded")
	assert.NotEmpty(t, values.Get("since"), "since should be forwarded")
}

func TestGetSpreadRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePublicQueryValues(
		t,
		"/0/public/Spread",
		`{"XXBTZUSD":[[1700000000,"1","2"]],"last":1700000000}`,
		func(k *Exchange) error {
			_, err := k.GetSpread(t.Context(), &GetSpreadRequest{
				Pair:       spotTestPair,
				Since:      time.Unix(1700000000, 0),
				AssetClass: "currency",
			})
			return err
		},
	)

	assert.Equal(t, "currency", values.Get("asset_class"), "asset_class should be forwarded")
	assert.NotEmpty(t, values.Get("since"), "since should be forwarded")
}

func TestGetAccountBalanceRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/Balance",
		`{"XXBT":"0.1"}`,
		func(k *Exchange) error {
			_, err := k.GetAccountBalance(t.Context(), &GetAccountBalanceRequest{
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestGetExtendedBalanceRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/BalanceEx",
		`{"XXBT":{"balance":"0.1","hold_trade":"0"}}`,
		func(k *Exchange) error {
			_, err := k.GetExtendedBalance(t.Context(), &GetExtendedBalanceRequest{
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedGetWithdrawInfo(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("WithdrawInfo", `{}`))
	_, err := k.GetWithdrawInfo(t.Context(), "XBT", "wallet", 1)
	require.NoError(t, err)
}

func TestMockedWithdraw(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("Withdraw", `{"refid":"ref-123"}`))
	refID, err := k.Withdraw(t.Context(), &WithdrawRequest{
		Asset:  "XBT",
		Key:    "wallet",
		Amount: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "ref-123", refID)
}

func TestWithdrawRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/Withdraw",
		`{"refid":"ref-123"}`,
		func(k *Exchange) error {
			_, err := k.Withdraw(t.Context(), &WithdrawRequest{
				Asset:            "XBT",
				Key:              "wallet",
				Amount:           1,
				AssetClass:       "currency",
				Address:          "bc1qexample",
				MaxFee:           "0.01",
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "currency", values.Get("aclass"), "aclass should be forwarded")
	assert.Equal(t, "bc1qexample", values.Get("address"), "address should be forwarded")
	assert.Equal(t, "0.01", values.Get("max_fee"), "max_fee should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedGetDepositMethods(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("DepositMethods", `[{"method":"Bitcoin","limit":false,"fee":"0.0001","address-setup-fee":"0"}]`))
	resp, err := k.GetDepositMethods(t.Context(), &GetDepositMethodsRequest{
		Asset: "XBT",
	})
	require.NoError(t, err)
	assert.Equal(t, []DepositMethods{{
		Method:          "Bitcoin",
		Limit:           false,
		Fee:             0.0001,
		AddressSetupFee: 0,
	}}, resp)
}

func TestGetDepositMethodsRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/DepositMethods",
		`[{"method":"Bitcoin","limit":false,"fee":"0.0001","address-setup-fee":"0"}]`,
		func(k *Exchange) error {
			_, err := k.GetDepositMethods(t.Context(), &GetDepositMethodsRequest{
				Asset:            "XBT",
				AssetClass:       "currency",
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "XBT", values.Get("asset"), "asset should be forwarded")
	assert.Equal(t, "currency", values.Get("aclass"), "aclass should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedGetCryptoDepositAddress(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("DepositAddresses", `[{"address":"bc1qexample","expiretm":"0","tag":"","new":false}]`))
	_, err := k.GetCryptoDepositAddress(t.Context(), &GetCryptoDepositAddressRequest{
		Asset:  "XBT",
		Method: "Bitcoin",
	})
	require.NoError(t, err)
}

func TestGetCryptoDepositAddressRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/DepositAddresses",
		`[{"address":"bc1qexample","expiretm":"0","tag":"","new":false}]`,
		func(k *Exchange) error {
			_, err := k.GetCryptoDepositAddress(t.Context(), &GetCryptoDepositAddressRequest{
				Asset:      "XBT",
				Method:     "Bitcoin",
				CreateNew:  true,
				AssetClass: "currency",
				Amount:     "1.2",
			})
			return err
		},
	)

	assert.Equal(t, "XBT", values.Get("asset"), "asset should be forwarded")
	assert.Equal(t, "Bitcoin", values.Get("method"), "method should be forwarded")
	assert.Equal(t, "true", values.Get("new"), "new should be forwarded")
	assert.Equal(t, "currency", values.Get("aclass"), "aclass should be forwarded")
	assert.Equal(t, "1.2", values.Get("amount"), "amount should be forwarded")
}

func TestMockedGetTradeBalance(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("TradeBalance", `{}`))
	_, err := k.GetTradeBalance(t.Context(), &TradeBalanceOptions{Asset: "XBT"})
	require.NoError(t, err)
}

func TestGetTradeBalanceRequestParams(t *testing.T) {
	t.Parallel()

	captured := make(chan url.Values, 1)
	k := newMockedKraken(t, mockRESTRoute("/0/private/TradeBalance", func(w http.ResponseWriter, r *http.Request) {
		captured <- copyValues(decodeBodyValues(t, r))
		spotResult(w, `{}`)
	}))

	_, err := k.GetTradeBalance(t.Context(), &TradeBalanceOptions{
		Asset:            "XBT",
		RebaseMultiplier: "1",
	})
	require.NoError(t, err)

	form := <-captured
	assert.Equal(t, "XBT", form.Get("asset"), "asset should be forwarded")
	assert.Equal(t, "1", form.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
	assert.Empty(t, form.Get("aclass"), "aclass should not be forwarded")
}

func TestGetOpenOrdersRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/OpenOrders",
		`{"open":{},"count":0}`,
		func(k *Exchange) error {
			_, err := k.GetOpenOrders(t.Context(), OrderInfoOptions{
				Trades:           true,
				UserRef:          5,
				ClientOrderID:    "client-id",
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "true", values.Get("trades"), "trades should be forwarded")
	assert.Equal(t, "5", values.Get("userref"), "userref should be forwarded")
	assert.Equal(t, "client-id", values.Get("cl_ord_id"), "cl_ord_id should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestCancelExistingOrderRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/CancelOrder",
		`{"count":1,"pending":false}`,
		func(k *Exchange) error {
			_, err := k.CancelExistingOrder(t.Context(), "", "client-id")
			return err
		},
	)

	assert.Equal(t, "client-id", values.Get("cl_ord_id"), "cl_ord_id should be forwarded")
}

func TestMockedGetClosedOrders(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("ClosedOrders", `{"closed":{},"count":0}`))
	_, err := k.GetClosedOrders(t.Context(), &GetClosedOrdersOptions{Trades: true})
	require.NoError(t, err)
}

func TestGetClosedOrdersRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/ClosedOrders",
		`{"closed":{},"count":0}`,
		func(k *Exchange) error {
			_, err := k.GetClosedOrders(t.Context(), &GetClosedOrdersOptions{
				Trades:           true,
				UserRef:          12,
				ClientOrderID:    "client-id",
				Start:            "1",
				End:              "2",
				Ofs:              3,
				CloseTime:        "close",
				ConsolidateTaker: true,
				WithoutCount:     true,
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "true", values.Get("trades"), "trades should be forwarded")
	assert.Equal(t, "12", values.Get("userref"), "userref should be forwarded")
	assert.Equal(t, "client-id", values.Get("cl_ord_id"), "cl_ord_id should be forwarded")
	assert.Equal(t, "close", values.Get("closetime"), "closetime should be forwarded")
	assert.Equal(t, "true", values.Get("consolidate_taker"), "consolidate_taker should be forwarded")
	assert.Equal(t, "true", values.Get("without_count"), "without_count should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedQueryOrdersInfo(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("QueryOrders", `{}`))
	_, err := k.QueryOrdersInfo(t.Context(), OrderInfoOptions{Trades: true}, "order-1")
	require.NoError(t, err)
}

func TestQueryOrdersInfoRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/QueryOrders",
		`{}`,
		func(k *Exchange) error {
			_, err := k.QueryOrdersInfo(t.Context(), OrderInfoOptions{
				Trades:           true,
				UserRef:          42,
				ConsolidateTaker: true,
				RebaseMultiplier: "1",
			}, "order-1", "order-2")
			return err
		},
	)

	assert.Equal(t, "true", values.Get("trades"), "trades should be forwarded")
	assert.Equal(t, "42", values.Get("userref"), "userref should be forwarded")
	assert.Equal(t, "order-1,order-2", values.Get("txid"), "txid should be forwarded")
	assert.Equal(t, "true", values.Get("consolidate_taker"), "consolidate_taker should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedGetTradesHistory(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("TradesHistory", `{"trades":{},"count":0}`))
	_, err := k.GetTradesHistory(t.Context(), &GetTradesHistoryOptions{Trades: true})
	require.NoError(t, err)
}

func TestGetTradesHistoryRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/TradesHistory",
		`{"trades":{},"count":0}`,
		func(k *Exchange) error {
			_, err := k.GetTradesHistory(t.Context(), &GetTradesHistoryOptions{
				Type:             "all",
				Trades:           true,
				Start:            "1",
				End:              "2",
				Ofs:              3,
				WithoutCount:     true,
				ConsolidateTaker: true,
				Ledgers:          true,
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "all", values.Get("type"), "type should be forwarded")
	assert.Equal(t, "true", values.Get("without_count"), "without_count should be forwarded")
	assert.Equal(t, "true", values.Get("consolidate_taker"), "consolidate_taker should be forwarded")
	assert.Equal(t, "true", values.Get("ledgers"), "ledgers should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedQueryTrades(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("QueryTrades", `{}`))
	_, err := k.QueryTrades(t.Context(), &QueryTradesRequest{
		Trades:        true,
		TransactionID: "trade-1",
	})
	require.NoError(t, err)
}

func TestQueryTradesRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/QueryTrades",
		`{}`,
		func(k *Exchange) error {
			_, err := k.QueryTrades(t.Context(), &QueryTradesRequest{
				Trades:           true,
				TransactionID:    "trade-1",
				TransactionIDs:   []string{"trade-2", "trade-3"},
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "true", values.Get("trades"), "trades should be forwarded")
	assert.Equal(t, "trade-1,trade-2,trade-3", values.Get("txid"), "txid should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestQueryTradesMissingTransactionID(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("QueryTrades", `{}`))
	_, err := k.QueryTrades(t.Context(), &QueryTradesRequest{})
	require.ErrorIs(t, err, errTransactionIDRequired)
}

func TestMockedOpenPositions(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("OpenPositions", `{}`))
	_, err := k.OpenPositions(t.Context(), &OpenPositionsRequest{
		DoCalculations: true,
	})
	require.NoError(t, err)
}

func TestOpenPositionsRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/OpenPositions",
		`{}`,
		func(k *Exchange) error {
			_, err := k.OpenPositions(t.Context(), &OpenPositionsRequest{
				TransactionIDList: []string{"pos-1", "pos-2"},
				DoCalculations:    true,
				Consolidation:     "market",
				RebaseMultiplier:  "1",
			})
			return err
		},
	)

	assert.Equal(t, "pos-1,pos-2", values.Get("txid"), "txid should be forwarded")
	assert.Equal(t, "true", values.Get("docalcs"), "docalcs should be forwarded")
	assert.Equal(t, "market", values.Get("consolidation"), "consolidation should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedGetLedgers(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("Ledgers", `{}`))
	_, err := k.GetLedgers(t.Context(), &GetLedgersOptions{})
	require.NoError(t, err)
}

func TestGetLedgersRequestParams(t *testing.T) {
	t.Parallel()

	captured := make(chan url.Values, 1)
	k := newMockedKrakenWithRESTHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/0/private/Ledgers") {
			captured <- copyValues(decodeBodyValues(t, r))
			spotResult(w, `{"ledger":{},"count":0}`)
			return
		}
		http.NotFound(w, r)
	})

	_, err := k.GetLedgers(t.Context(), &GetLedgersOptions{
		Aclass:           "currency",
		Asset:            "XBT",
		Type:             "all",
		Start:            "1",
		End:              "2",
		Ofs:              3,
		WithoutCount:     true,
		RebaseMultiplier: "1",
	})
	require.NoError(t, err)

	form := <-captured
	assert.Equal(t, "currency", form.Get("aclass"), "aclass should be forwarded")
	assert.Equal(t, "XBT", form.Get("asset"), "asset should be forwarded")
	assert.Equal(t, "all", form.Get("type"), "type should be forwarded")
	assert.Equal(t, "1", form.Get("start"), "start should be forwarded")
	assert.Equal(t, "2", form.Get("end"), "end should be forwarded")
	assert.Equal(t, "3", form.Get("ofs"), "ofs should be forwarded")
	assert.Equal(t, "true", form.Get("without_count"), "without_count should be forwarded")
	assert.Equal(t, "1", form.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedQueryLedgers(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("QueryLedgers", `{}`))
	_, err := k.QueryLedgers(t.Context(), &QueryLedgersRequest{
		ID: "ledger-1",
	})
	require.NoError(t, err)
}

func TestQueryLedgersRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/QueryLedgers",
		`{}`,
		func(k *Exchange) error {
			_, err := k.QueryLedgers(t.Context(), &QueryLedgersRequest{
				ID:               "ledger-1",
				IDs:              []string{"ledger-2", "ledger-3"},
				Trades:           true,
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "ledger-1,ledger-2,ledger-3", values.Get("id"), "id list should be forwarded")
	assert.Equal(t, "true", values.Get("trades"), "trades should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestQueryLedgersMissingID(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("QueryLedgers", `{}`))
	_, err := k.QueryLedgers(t.Context(), &QueryLedgersRequest{})
	require.ErrorIs(t, err, errIDRequired)
}

func TestMockedGetTradeVolume(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("TradeVolume", `{}`))
	_, err := k.GetTradeVolume(t.Context(), &GetTradeVolumeRequest{
		Pairs: []currency.Pair{spotTestPair},
	})
	require.NoError(t, err)
}

func TestGetTradeVolumeRequestParams(t *testing.T) {
	t.Parallel()

	captured := make(chan url.Values, 1)
	k := newMockedKrakenWithRESTHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/0/private/TradeVolume") {
			captured <- copyValues(decodeBodyValues(t, r))
			spotResult(w, `{}`)
			return
		}
		http.NotFound(w, r)
	})

	_, err := k.GetTradeVolume(t.Context(), &GetTradeVolumeRequest{
		Pairs:            []currency.Pair{spotTestPair},
		RebaseMultiplier: "1",
	})
	require.NoError(t, err)

	form := <-captured
	assert.NotEmpty(t, form.Get("pair"), "pair should be forwarded")
	assert.Empty(t, form.Get("fee-info"), "fee-info should not be sent")
	assert.Equal(t, "1", form.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedWithdrawStatus(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("WithdrawStatus", `[{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-1","txid":"tx-1","info":"ok","amount":"1","fee":"0.1","time":1700000000,"status":"Success"}]`))
	_, err := k.WithdrawStatus(t.Context(), &WithdrawStatusRequest{
		Asset:  currency.XBT,
		Method: "Bitcoin",
	})
	require.NoError(t, err)
}

func TestWithdrawStatusRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/WithdrawStatus",
		`[{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-1","txid":"tx-1","info":"ok","amount":"1","fee":"0.1","time":1700000000,"status":"Success"}]`,
		func(k *Exchange) error {
			_, err := k.WithdrawStatus(t.Context(), &WithdrawStatusRequest{
				Asset:            currency.XBT,
				Method:           "Bitcoin",
				AssetClass:       "currency",
				Start:            "1",
				End:              "2",
				Cursor:           "cursor",
				Limit:            25,
				RebaseMultiplier: "1",
			})
			return err
		},
	)

	assert.Equal(t, "currency", values.Get("aclass"), "aclass should be forwarded")
	assert.Equal(t, "1", values.Get("start"), "start should be forwarded")
	assert.Equal(t, "2", values.Get("end"), "end should be forwarded")
	assert.Equal(t, "cursor", values.Get("cursor"), "cursor should be forwarded")
	assert.Equal(t, "25", values.Get("limit"), "limit should be forwarded")
	assert.Equal(t, "1", values.Get("rebase_multiplier"), "rebase_multiplier should be forwarded")
}

func TestMockedWithdrawCancel(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("WithdrawCancel", `true`))
	_, err := k.WithdrawCancel(t.Context(), currency.XBT, "ref-1")
	require.NoError(t, err)
}

func TestMockedGetCreditLines(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("CreditLines", `{}`))
		_, err := k.GetCreditLines(t.Context(), &GetCreditLinesRequest{})
		require.NoError(t, err)
	})

	t.Run("with rebase multiplier", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("CreditLines", `{}`))
		_, err := k.GetCreditLines(t.Context(), &GetCreditLinesRequest{RebaseMultiplier: "1"})
		require.NoError(t, err)
	})
}

func TestMockedGetOrderAmends(t *testing.T) {
	const response = `{"count":1,"amends":[{"amend_id":"amend-1","amend_type":"original","order_qty":"1.25","display_qty":"0.50","remaining_qty":"1.00","limit_price":"65000.1","trigger_price":"64000.1","reason":"created","post_only":true,"timestamp":1704067200}]}`

	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("OrderAmends", response))
		resp, err := k.GetOrderAmends(t.Context(), &GetOrderAmendsRequest{OrderID: "order-id"})
		require.NoError(t, err)
		assert.Equal(t, &GetOrderAmendsResponse{
			Count: 1,
			Amends: []OrderAmend{{
				AmendID:       "amend-1",
				AmendType:     "original",
				OrderQuantity: types.Number(1.25),
				DisplayVolume: types.Number(0.5),
				RemainingQty:  types.Number(1),
				LimitPrice:    types.Number(65000.1),
				TriggerPrice:  types.Number(64000.1),
				Reason:        "created",
				PostOnly:      true,
				Timestamp:     types.Time(time.Unix(1704067200, 0)),
			}},
		}, resp)
	})

	t.Run("missing order id", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("OrderAmends", response))
		_, err := k.GetOrderAmends(t.Context(), &GetOrderAmendsRequest{})
		require.ErrorIs(t, err, errOrderIDRequired)
	})

	t.Run("with rebase multiplier", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("OrderAmends", response))
		_, err := k.GetOrderAmends(t.Context(), &GetOrderAmendsRequest{OrderID: "order-id", RebaseMultiplier: "1"})
		require.NoError(t, err)
	})
}

func TestMockedRequestExportReport(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("AddExport", `{"id":"export-id"}`))
		_, err := k.RequestExportReport(t.Context(), &RequestExportReportRequest{Report: "trades", Format: "CSV"})
		require.NoError(t, err)
	})

	t.Run("with optional fields", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("AddExport", `{"id":"export-id"}`))
		_, err := k.RequestExportReport(t.Context(), &RequestExportReportRequest{
			Report:      "ledgers",
			Format:      "CSV",
			Description: "test export",
			Fields:      "refid,time",
			StartTime:   time.Unix(1700000000, 0),
			EndTime:     time.Unix(1700003600, 0),
		})
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name        string
		req         *RequestExportReportRequest
		expectedErr error
	}{
		{
			name:        "missing report",
			req:         &RequestExportReportRequest{Format: "CSV"},
			expectedErr: errReportRequired,
		},
		{
			name:        "missing format",
			req:         &RequestExportReportRequest{Report: "trades"},
			expectedErr: errFormatRequired,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("AddExport", `{"id":"export-id"}`))
			_, err := k.RequestExportReport(t.Context(), tc.req)
			require.ErrorIs(t, err, tc.expectedErr, "RequestExportReport must return the expected validation error")
		})
	}
}

func TestMockedGetExportReportStatus(t *testing.T) {
	const response = `[{"id":"export-id","descr":"test export","format":"CSV","report":"trades","subtype":"","status":"Processed","flags":"","fields":"refid,time","createdtm":"1704067200","expiretm":"1704153600","starttm":"1704067260","completedtm":"1704067320","datastarttm":"1704060000","dataendtm":"1704063600","aclass":"currency","asset":"XBT"}]`

	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("ExportStatus", response))
		resp, err := k.GetExportReportStatus(t.Context(), &GetExportReportStatusRequest{Report: "trades"})
		require.NoError(t, err)
		assert.Equal(t, []ExportReportStatusResponse{{
			ID:            "export-id",
			Description:   "test export",
			Format:        "CSV",
			Report:        "trades",
			Subtype:       "",
			Status:        "Processed",
			Flags:         "",
			Fields:        "refid,time",
			CreatedTime:   types.Time(time.Unix(1704067200, 0)),
			ExpiryTime:    types.Time(time.Unix(1704153600, 0)),
			StartTime:     types.Time(time.Unix(1704067260, 0)),
			CompletedTime: types.Time(time.Unix(1704067320, 0)),
			DataStartTime: types.Time(time.Unix(1704060000, 0)),
			DataEndTime:   types.Time(time.Unix(1704063600, 0)),
			AssetClass:    "currency",
			Asset:         "XBT",
		}}, resp)
	})

	t.Run("missing report", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("ExportStatus", response))
		_, err := k.GetExportReportStatus(t.Context(), &GetExportReportStatusRequest{})
		require.ErrorIs(t, err, errReportRequired)
	})
}

func TestMockedRetrieveDataExport(t *testing.T) {
	const response = "id,field\n1,test\n"

	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockRawResult("/0/private/RetrieveExport", "", response))
		data, err := k.RetrieveDataExport(t.Context(), &RetrieveDataExportRequest{ID: "export-id"})
		require.NoError(t, err)
		assert.Contains(t, string(data), "id,field")
	})

	t.Run("missing id", func(t *testing.T) {
		k := newMockedKraken(t, mockRawResult("/0/private/RetrieveExport", "", response))
		_, err := k.RetrieveDataExport(t.Context(), &RetrieveDataExportRequest{})
		require.ErrorIs(t, err, errIDRequired)
	})
}

func TestMockedDeleteExportReport(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("RemoveExport", `{"delete":true,"cancel":false}`))
		_, err := k.DeleteExportReport(t.Context(), &DeleteExportReportRequest{ID: "export-id", Type: "delete"})
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name        string
		req         *DeleteExportReportRequest
		expectedErr error
	}{
		{
			name:        "missing id",
			req:         &DeleteExportReportRequest{},
			expectedErr: errIDRequired,
		},
		{
			name:        "missing type",
			req:         &DeleteExportReportRequest{ID: "export-id"},
			expectedErr: errTypeRequired,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("RemoveExport", `{"delete":true,"cancel":false}`))
			_, err := k.DeleteExportReport(t.Context(), tc.req)
			require.ErrorIs(t, err, tc.expectedErr, "DeleteExportReport must return the expected validation error")
		})
	}
}

func TestMockedGetAPIKeyInfo(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("GetApiKeyInfo", `{"apiKeyName":"my-api-key","apiKey":"4/SDrDBcOOPnm3nPlNfEMMJDeRcIVqPz+QhRxIodyZbI9po/aVRiHsgX","nonce":"1772627060997","nonceWindow":0,"permissions":["query-funds","withdraw-funds","query-open-trades","modify-trades"],"iban":"AA88 N84G WOAK NMOI","validUntil":"0","queryFrom":"0","queryTo":"0","createdTime":"1772542900","modifiedTime":"1772543095","ipAllowlist":[],"lastUsed":"1772627061"}`))

		resp, err := k.GetAPIKeyInfo(t.Context(), &GetAPIKeyInfoRequest{})
		require.NoError(t, err, "GetAPIKeyInfo must not error")
		require.NotNil(t, resp, "GetAPIKeyInfo response must not be nil")

		assert.Equal(t, "my-api-key", resp.APIKeyName, "APIKeyName should decode")
		assert.Equal(t, "4/SDrDBcOOPnm3nPlNfEMMJDeRcIVqPz+QhRxIodyZbI9po/aVRiHsgX", resp.APIKey, "APIKey should decode")
		assert.Equal(t, "1772627060997", resp.Nonce, "Nonce should decode")
		assert.Equal(t, uint64(0), resp.NonceWindow, "NonceWindow should decode")
		assert.Equal(t, []string{"query-funds", "withdraw-funds", "query-open-trades", "modify-trades"}, resp.Permissions, "Permissions should decode")
		assert.Equal(t, "AA88 N84G WOAK NMOI", resp.IBAN, "IBAN should decode")
		assert.True(t, resp.ValidUntil.Time().IsZero(), "ValidUntil should decode zero timestamps")
		assert.True(t, resp.QueryFrom.Time().IsZero(), "QueryFrom should decode zero timestamps")
		assert.True(t, resp.QueryTo.Time().IsZero(), "QueryTo should decode zero timestamps")
		assert.Equal(t, time.Unix(1772542900, 0), resp.CreatedTime.Time(), "CreatedTime should decode")
		assert.Equal(t, time.Unix(1772543095, 0), resp.ModifiedTime.Time(), "ModifiedTime should decode")
		assert.Empty(t, resp.IPAllowlist, "IPAllowlist should decode")
		require.NotNil(t, resp.LastUsed, "LastUsed must not be nil when Kraken returns a timestamp")
		assert.Equal(t, time.Unix(1772627061, 0), resp.LastUsed.Time(), "LastUsed should decode")
	})

	t.Run("null last used", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("GetApiKeyInfo", `{"apiKeyName":"my-api-key","apiKey":"4/SDrDBcOOPnm3nPlNfEMMJDeRcIVqPz+QhRxIodyZbI9po/aVRiHsgX","nonce":"1772627060997","nonceWindow":0,"permissions":["query-funds"],"iban":"AA88 N84G WOAK NMOI","validUntil":"0","queryFrom":"0","queryTo":"0","createdTime":"1772542900","modifiedTime":"1772543095","ipAllowlist":[],"lastUsed":null}`))

		resp, err := k.GetAPIKeyInfo(t.Context(), &GetAPIKeyInfoRequest{})
		require.NoError(t, err, "GetAPIKeyInfo must not error when lastUsed is null")
		require.NotNil(t, resp, "GetAPIKeyInfo response must not be nil")
		assert.Nil(t, resp.LastUsed, "LastUsed should be nil when Kraken returns null")
	})

	t.Run("error response", func(t *testing.T) {
		k := newMockedKrakenWithRESTHandler(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/0/private/GetApiKeyInfo") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"error":["EGeneral:Invalid arguments"],"result":{}}`)
				return
			}
			http.NotFound(w, r)
		})

		_, err := k.GetAPIKeyInfo(t.Context(), &GetAPIKeyInfoRequest{})
		require.ErrorIs(t, err, request.ErrAuthRequestFailed, "GetAPIKeyInfo must return an authenticated request error")
		assert.ErrorContains(t, err, "EGeneral:Invalid arguments", "GetAPIKeyInfo should surface Kraken error messages")
	})
}

func TestGetAPIKeyInfoRequestParams(t *testing.T) {
	t.Parallel()

	values := capturePrivateBodyValues(
		t,
		"/0/private/GetApiKeyInfo",
		`{"apiKeyName":"my-api-key","apiKey":"4/SDrDBcOOPnm3nPlNfEMMJDeRcIVqPz+QhRxIodyZbI9po/aVRiHsgX","nonce":"1772627060997","nonceWindow":0,"permissions":[],"iban":"","validUntil":"0","queryFrom":"0","queryTo":"0","createdTime":"1772542900","modifiedTime":"1772543095","ipAllowlist":[],"lastUsed":null}`,
		func(k *Exchange) error {
			_, err := k.GetAPIKeyInfo(t.Context(), &GetAPIKeyInfoRequest{})
			return err
		},
	)

	require.Len(t, values, 1, "GetAPIKeyInfo request values must only contain nonce")
	assert.NotEmpty(t, values.Get("nonce"), "GetAPIKeyInfo nonce should be populated")
}

func TestMockedAmendOrder(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("AmendOrder", `{"amend_id":"amend-id"}`))
		_, err := k.AmendOrder(t.Context(), &AmendOrderRequest{TransactionID: "txid"})
		require.NoError(t, err)
	})

	t.Run("with optional fields", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("AmendOrder", `{"amend_id":"amend-id"}`))
		_, err := k.AmendOrder(t.Context(), &AmendOrderRequest{
			TransactionID:   "txid",
			ClientOrderID:   "client-order-id",
			OrderQuantity:   "1",
			DisplayQuantity: "0.5",
			LimitPrice:      "100",
			TriggerPrice:    "90",
			Pair:            "XBT/USD",
			PostOnly:        true,
			Deadline:        "2026-01-01T00:00:00Z",
		})
		require.NoError(t, err)
	})
}

func TestMockedCancelAllOrdersREST(t *testing.T) {
	k := newMockedKraken(t, mockPrivateResult("CancelAll", `{"count":1,"pending":false}`))
	_, err := k.CancelAllOrdersREST(t.Context())
	require.NoError(t, err)
}

func TestMockedCancelAllOrdersAfter(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("CancelAllOrdersAfter", `{"currentTime":"2025-01-01T00:00:00Z","triggerTime":"2025-01-01T00:00:10Z"}`))
		_, err := k.CancelAllOrdersAfter(t.Context(), &CancelAllOrdersAfterRequest{Timeout: 10})
		require.NoError(t, err)
	})

	t.Run("validation missing timeout", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("CancelAllOrdersAfter", `{"currentTime":"2025-01-01T00:00:00Z","triggerTime":"2025-01-01T00:00:10Z"}`))
		_, err := k.CancelAllOrdersAfter(t.Context(), &CancelAllOrdersAfterRequest{})
		require.ErrorIs(t, err, errTimeoutMustBeGreaterThanZero)
	})
}

func TestAddOrderRequestParams(t *testing.T) {
	t.Parallel()

	captured := make(chan url.Values, 1)
	k := newMockedKrakenWithRESTHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/0/private/AddOrder") {
			captured <- copyValues(decodeBodyValues(t, r))
			spotResult(w, `{"descr":{"close":"","order":"buy 1 XBTUSD @ limit"},"txid":["txid-1"]}`)
			return
		}
		http.NotFound(w, r)
	})

	_, err := k.AddOrder(t.Context(), spotTestPair, "buy", order.Limit.Lower(), 1, 100, 90, 2, &AddOrderOptions{
		UserRef:         123,
		ClientOrderID:   "client-id",
		AssetClass:      "currency",
		DisplayVolume:   0.5,
		Trigger:         "last",
		ReduceOnly:      true,
		SelfTradePolicy: "cancel-newest",
		OrderFlags:      "post",
		StartTm:         "1",
		ExpireTm:        "2",
		CloseOrderType:  "limit",
		ClosePrice:      80,
		ClosePrice2:     70,
		Validate:        true,
		TimeInForce:     "IOC",
		Deadline:        "2026-01-01T00:00:00Z",
	})
	require.NoError(t, err)

	form := <-captured
	assert.Equal(t, "123", form.Get("userref"), "userref should be forwarded")
	assert.Equal(t, "client-id", form.Get("cl_ord_id"), "cl_ord_id should be forwarded")
	assert.Equal(t, "currency", form.Get("asset_class"), "asset_class should be forwarded")
	assert.Equal(t, "0.5", form.Get("displayvol"), "displayvol should be forwarded")
	assert.Equal(t, "last", form.Get("trigger"), "trigger should be forwarded")
	assert.Equal(t, "true", form.Get("reduce_only"), "reduce_only should be forwarded")
	assert.Equal(t, "cancel-newest", form.Get("stptype"), "stptype should be forwarded")
	assert.Equal(t, "limit", form.Get("close[ordertype]"), "close[ordertype] should use close order type")
	assert.Equal(t, "2026-01-01T00:00:00Z", form.Get("deadline"), "deadline should be forwarded")
}

func TestAddOrderNilOptions(t *testing.T) {
	t.Parallel()

	k := newMockedKrakenWithRESTHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/0/private/AddOrder") {
			spotResult(w, `{"descr":{"close":"","order":"buy 1 XBTUSD @ limit"},"txid":["txid-1"]}`)
			return
		}
		http.NotFound(w, r)
	})
	_, err := k.AddOrder(t.Context(), spotTestPair, "buy", order.Limit.Lower(), 1, 100, 0, 0, nil)
	require.NoError(t, err)
}

func TestMockedAddOrderBatch(t *testing.T) {
	for _, tc := range []struct {
		name        string
		req         *AddOrderBatchRequest
		expectedErr error
	}{
		{
			name: "happy path",
			req: &AddOrderBatchRequest{
				Orders: []AddOrderBatchOrderRequest{{OrderType: "limit", OrderSide: "buy", Volume: "1"}},
			},
		},
		{
			name:        "validation missing orders",
			req:         &AddOrderBatchRequest{},
			expectedErr: errOrdersRequired,
		},
		{
			name: "with optional fields",
			req: &AddOrderBatchRequest{
				Orders: []AddOrderBatchOrderRequest{{
					OrderType:   "limit",
					OrderSide:   "buy",
					Volume:      "1",
					Price:       "100",
					TimeInForce: "IOC",
				}},
				Pair:       "XBT/USD",
				AssetClass: "currency",
				Deadline:   "2026-01-01T00:00:00Z",
				Validate:   true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("AddOrderBatch", `{"orders":[]}`))
			_, err := k.AddOrderBatch(t.Context(), tc.req)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMockedCancelOrderBatch(t *testing.T) {
	for _, tc := range []struct {
		name        string
		req         *CancelOrderBatchRequest
		expectedErr error
	}{
		{
			name: "happy path",
			req: &CancelOrderBatchRequest{
				Orders: []CancelOrderBatchOrderRequest{{TransactionID: "txid"}},
			},
		},
		{
			name:        "validation missing orders",
			req:         &CancelOrderBatchRequest{},
			expectedErr: errOrdersOrClientOrdersRequired,
		},
		{
			name: "with client orders",
			req: &CancelOrderBatchRequest{
				ClientOrder: []CancelOrderBatchClientOrderIDItem{{ClientOrderID: "client-order-id"}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("CancelOrderBatch", `{"count":1}`))
			_, err := k.CancelOrderBatch(t.Context(), tc.req)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMockedEditOrder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		req         *EditOrderRequest
		expectedErr error
	}{
		{
			name: "happy path",
			req:  &EditOrderRequest{TransactionID: "txid"},
		},
		{
			name:        "validation missing transaction id",
			req:         &EditOrderRequest{},
			expectedErr: errTransactionIDRequired,
		},
		{
			name: "with optional fields",
			req: &EditOrderRequest{
				UserReference:  123,
				TransactionID:  "txid",
				Volume:         "1",
				DisplayVolume:  "0.5",
				Pair:           "XBT/USD",
				AssetClass:     "currency",
				Price:          "100",
				SecondaryPrice: "90",
				OrderFlags:     "post",
				Deadline:       "2026-01-01T00:00:00Z",
				CancelResponse: true,
				Validate:       true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("EditOrder", `{"status":"ok"}`))
			_, err := k.EditOrder(t.Context(), tc.req)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMockedGetRecentDepositsStatus(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("DepositStatus", `[]`))
		_, err := k.GetRecentDepositsStatus(t.Context(), &GetRecentDepositsStatusRequest{Asset: "XBT"})
		require.NoError(t, err)
	})

	t.Run("with optional fields", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("DepositStatus", `[]`))
		_, err := k.GetRecentDepositsStatus(t.Context(), &GetRecentDepositsStatusRequest{
			Asset:            "XBT",
			AssetClass:       "currency",
			Method:           "Bitcoin",
			Start:            "1",
			End:              "2",
			Cursor:           "cursor",
			Limit:            2,
			RebaseMultiplier: "1",
		})
		require.NoError(t, err)
	})
}

func TestMockedGetWithdrawalMethods(t *testing.T) {
	for _, tc := range []struct {
		name        string
		req         *GetWithdrawalMethodsRequest
		expectedErr error
	}{
		{
			name: "happy path",
			req:  &GetWithdrawalMethodsRequest{Asset: "XBT"},
		},
		{
			name:        "validation missing asset",
			req:         &GetWithdrawalMethodsRequest{},
			expectedErr: errAssetRequired,
		},
		{
			name: "with optional fields",
			req: &GetWithdrawalMethodsRequest{
				Asset:            "XBT",
				AssetClass:       "currency",
				Network:          "Bitcoin",
				RebaseMultiplier: "1",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("WithdrawMethods", `[]`))
			_, err := k.GetWithdrawalMethods(t.Context(), tc.req)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMockedGetWithdrawalAddresses(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("WithdrawAddresses", `[]`))
		verified := true
		_, err := k.GetWithdrawalAddresses(t.Context(), &GetWithdrawalAddressesRequest{Asset: "XBT", Verified: &verified})
		require.NoError(t, err)
	})

	t.Run("validation missing asset", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("WithdrawAddresses", `[]`))
		_, err := k.GetWithdrawalAddresses(t.Context(), &GetWithdrawalAddressesRequest{})
		require.ErrorIs(t, err, errAssetRequired)
	})

	t.Run("with optional fields", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("WithdrawAddresses", `[]`))
		verified := false
		_, err := k.GetWithdrawalAddresses(t.Context(), &GetWithdrawalAddressesRequest{
			Asset:      "XBT",
			AssetClass: "currency",
			Method:     "Bitcoin",
			Key:        "primary",
			Verified:   &verified,
		})
		require.NoError(t, err)
	})
}

func TestMockedWalletTransfer(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("WalletTransfer", `{"refid":"wallet-ref"}`))
		_, err := k.WalletTransfer(t.Context(), &WalletTransferRequest{Asset: "XBT", From: "spot", To: "margin", Amount: "1"})
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name        string
		req         *WalletTransferRequest
		expectedErr error
	}{
		{
			name:        "missing asset",
			req:         &WalletTransferRequest{},
			expectedErr: errAssetRequired,
		},
		{
			name:        "missing from",
			req:         &WalletTransferRequest{Asset: "XBT", To: "margin", Amount: "1"},
			expectedErr: errFromRequired,
		},
		{
			name:        "missing to",
			req:         &WalletTransferRequest{Asset: "XBT", From: "spot", Amount: "1"},
			expectedErr: errToRequired,
		},
		{
			name:        "missing amount",
			req:         &WalletTransferRequest{Asset: "XBT", From: "spot", To: "margin"},
			expectedErr: errAmountRequired,
		},
	} {
		t.Run("validation "+tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("WalletTransfer", `{"refid":"wallet-ref"}`))
			_, err := k.WalletTransfer(t.Context(), tc.req)
			require.ErrorIs(t, err, tc.expectedErr, "WalletTransfer must return the expected validation error")
		})
	}
}

func TestMockedCreateSubaccount(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("CreateSubaccount", `true`))
		_, err := k.CreateSubaccount(t.Context(), &CreateSubaccountRequest{Username: "alice", Email: "alice@example.com"})
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name        string
		req         *CreateSubaccountRequest
		expectedErr error
	}{
		{
			name:        "missing username",
			req:         &CreateSubaccountRequest{},
			expectedErr: errUsernameRequired,
		},
		{
			name:        "missing email",
			req:         &CreateSubaccountRequest{Username: "alice"},
			expectedErr: errEmailRequired,
		},
	} {
		t.Run("validation "+tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("CreateSubaccount", `true`))
			_, err := k.CreateSubaccount(t.Context(), tc.req)
			require.ErrorIs(t, err, tc.expectedErr, "CreateSubaccount must return the expected validation error")
		})
	}
}

func TestMockedAccountTransfer(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("AccountTransfer", `{"transfer_id":"transfer-id","status":"ok"}`))
		_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{Asset: "XBT", Amount: "1", From: "main", To: "sub"})
		require.NoError(t, err)
	})

	t.Run("with asset class", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("AccountTransfer", `{"transfer_id":"transfer-id","status":"ok"}`))
		_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{
			Asset:      "XBT",
			AssetClass: "currency",
			Amount:     "1",
			From:       "main",
			To:         "sub",
		})
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name        string
		req         *AccountTransferRequest
		expectedErr error
	}{
		{
			name:        "missing asset",
			req:         &AccountTransferRequest{},
			expectedErr: errAssetRequired,
		},
		{
			name:        "missing amount",
			req:         &AccountTransferRequest{Asset: "XBT", From: "main", To: "sub"},
			expectedErr: errAmountRequired,
		},
		{
			name:        "missing from",
			req:         &AccountTransferRequest{Asset: "XBT", Amount: "1", To: "sub"},
			expectedErr: errFromRequired,
		},
		{
			name:        "missing to",
			req:         &AccountTransferRequest{Asset: "XBT", Amount: "1", From: "main"},
			expectedErr: errToRequired,
		},
	} {
		t.Run("validation "+tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("AccountTransfer", `{"transfer_id":"transfer-id","status":"ok"}`))
			_, err := k.AccountTransfer(t.Context(), tc.req)
			require.ErrorIs(t, err, tc.expectedErr, "AccountTransfer must return the expected validation error")
		})
	}
}

func TestMockedAllocateEarnFunds(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/Allocate", `true`))
		_, err := k.AllocateEarnFunds(t.Context(), &AllocateEarnFundsRequest{Amount: "1", StrategyID: "strategy-id"})
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name        string
		req         *AllocateEarnFundsRequest
		expectedErr error
	}{
		{
			name:        "missing amount",
			req:         &AllocateEarnFundsRequest{},
			expectedErr: errAmountRequired,
		},
		{
			name:        "missing strategy id",
			req:         &AllocateEarnFundsRequest{Amount: "1"},
			expectedErr: errStrategyIDRequired,
		},
	} {
		t.Run("validation "+tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("Earn/Allocate", `true`))
			_, err := k.AllocateEarnFunds(t.Context(), tc.req)
			require.ErrorIs(t, err, tc.expectedErr, "AllocateEarnFunds must return the expected validation error")
		})
	}
}

func TestMockedDeallocateEarnFunds(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/Deallocate", `true`))
		_, err := k.DeallocateEarnFunds(t.Context(), &DeallocateEarnFundsRequest{Amount: "1", StrategyID: "strategy-id"})
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name        string
		req         *DeallocateEarnFundsRequest
		expectedErr error
	}{
		{
			name:        "missing amount",
			req:         &DeallocateEarnFundsRequest{},
			expectedErr: errAmountRequired,
		},
		{
			name:        "missing strategy id",
			req:         &DeallocateEarnFundsRequest{Amount: "1"},
			expectedErr: errStrategyIDRequired,
		},
	} {
		t.Run("validation "+tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockPrivateResult("Earn/Deallocate", `true`))
			_, err := k.DeallocateEarnFunds(t.Context(), tc.req)
			require.ErrorIs(t, err, tc.expectedErr, "DeallocateEarnFunds must return the expected validation error")
		})
	}
}

func TestMockedGetEarnAllocationStatus(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/AllocateStatus", `{"pending":false}`))
		_, err := k.GetEarnAllocationStatus(t.Context(), &EarnOperationStatusRequest{StrategyID: "strategy-id"})
		require.NoError(t, err)
	})

	t.Run("validation missing strategy id", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/AllocateStatus", `{"pending":false}`))
		_, err := k.GetEarnAllocationStatus(t.Context(), &EarnOperationStatusRequest{})
		require.ErrorIs(t, err, errStrategyIDRequired)
	})
}

func TestMockedGetEarnDeallocationStatus(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/DeallocateStatus", `{"pending":false}`))
		_, err := k.GetEarnDeallocationStatus(t.Context(), &EarnOperationStatusRequest{StrategyID: "strategy-id"})
		require.NoError(t, err)
	})

	t.Run("validation missing strategy id", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/DeallocateStatus", `{"pending":false}`))
		_, err := k.GetEarnDeallocationStatus(t.Context(), &EarnOperationStatusRequest{})
		require.ErrorIs(t, err, errStrategyIDRequired)
	})
}

func TestMockedListEarnStrategies(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/Strategies", `{"items":[],"next_cursor":""}`))
		_, err := k.ListEarnStrategies(t.Context(), &ListEarnStrategiesRequest{LockType: []string{"bonded"}})
		require.NoError(t, err)
	})

	t.Run("with optional fields", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/Strategies", `{"items":[],"next_cursor":""}`))
		ascending := true
		_, err := k.ListEarnStrategies(t.Context(), &ListEarnStrategiesRequest{
			Ascending: &ascending,
			Asset:     "XBT",
			Cursor:    "cursor",
			Limit:     10,
			LockType:  []string{"bonded"},
		})
		require.NoError(t, err)
	})
}

func TestMockedListEarnAllocations(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/Allocations", `{"converted_asset":"USD","total_allocated":"0","total_rewarded":"0","items":[]}`))
		ascending := true
		hideZeros := false
		_, err := k.ListEarnAllocations(t.Context(), &ListEarnAllocationsRequest{Ascending: &ascending, HideZeroAllocations: &hideZeros})
		require.NoError(t, err)
	})

	t.Run("with converted asset", func(t *testing.T) {
		k := newMockedKraken(t, mockPrivateResult("Earn/Allocations", `{"converted_asset":"USD","total_allocated":"0","total_rewarded":"0","items":[]}`))
		ascending := false
		hideZeros := true
		_, err := k.ListEarnAllocations(t.Context(), &ListEarnAllocationsRequest{
			Ascending:           &ascending,
			ConvertedAsset:      "USD",
			HideZeroAllocations: &hideZeros,
		})
		require.NoError(t, err)
	})
}

func TestMockedGetPreTradeData(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockPublicResult("PreTrade", `{"symbol":"XBT/USD","description":"test","base_asset":"XBT","base_notation":"XBT","quote_asset":"USD","quote_notation":"USD","venue":"test","system":"test","bids":[],"asks":[]}`))
		_, err := k.GetPreTradeData(t.Context(), &GetPreTradeDataRequest{Symbol: "XBT/USD"})
		require.NoError(t, err)
	})

	t.Run("validation missing symbol", func(t *testing.T) {
		k := newMockedKraken(t, mockPublicResult("PreTrade", `{"symbol":"XBT/USD","description":"test","base_asset":"XBT","base_notation":"XBT","quote_asset":"USD","quote_notation":"USD","venue":"test","system":"test","bids":[],"asks":[]}`))
		_, err := k.GetPreTradeData(t.Context(), &GetPreTradeDataRequest{})
		require.ErrorIs(t, err, errSymbolRequired, "GetPreTradeData must error on missing symbol")
	})
}

func TestMockedGetPostTradeData(t *testing.T) {
	k := newMockedKraken(t, mockPublicResult("PostTrade", `{"last_ts":"2025-01-01T00:00:00Z","count":0,"trades":[]}`))
	_, err := k.GetPostTradeData(t.Context(), &GetPostTradeDataRequest{
		Symbol:        "XBT/USD",
		FromTimestamp: time.Now().Add(-time.Hour),
		ToTimestamp:   time.Now(),
		Count:         10,
	})
	require.NoError(t, err)
}

func TestMockedGetFuturesTickerBySymbol(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/tickers"))
	_, err := k.GetFuturesTickerBySymbol(t.Context(), "PF_XBTUSD")
	require.NoError(t, err)
}

func TestMockedFuturesEditOrder(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/editorder"))
	_, err := k.FuturesEditOrder(t.Context(), "order-id", "", 1, 2, 0)
	require.NoError(t, err)
}

func TestMockedFuturesSendOrder(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockFuturesResponse("/api/v3/sendorder"))
		_, err := k.FuturesSendOrder(t.Context(), order.Limit, mockedFuturesPair(), "buy", "", "", "", order.ImmediateOrCancel, 1, 2, 0)
		require.NoError(t, err)
	})

	t.Run("post only", func(t *testing.T) {
		k := newMockedKraken(t, mockFuturesResponse("/api/v3/sendorder"))
		_, err := k.FuturesSendOrder(t.Context(), order.Limit, mockedFuturesPair(), "buy", "mark", "client-order-id", "true", order.PostOnly, 1, 2, 1.5)
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name           string
		orderType      order.Type
		side           string
		triggerSignal  string
		reduceOnly     string
		expectedErrMsg string
	}{
		{
			name:           "invalid order type",
			orderType:      order.UnknownType,
			side:           "buy",
			expectedErrMsg: "invalid orderType",
		},
		{
			name:           "invalid side",
			orderType:      order.Limit,
			side:           "invalid-side",
			expectedErrMsg: "invalid side",
		},
		{
			name:           "invalid trigger signal",
			orderType:      order.Limit,
			side:           "buy",
			triggerSignal:  "invalid-trigger",
			expectedErrMsg: "invalid triggerSignal",
		},
		{
			name:           "invalid reduce only",
			orderType:      order.Limit,
			side:           "buy",
			reduceOnly:     "invalid-reduce",
			expectedErrMsg: "invalid reduceOnly",
		},
	} {
		t.Run("validation "+tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockFuturesResponse("/api/v3/sendorder"))
			_, err := k.FuturesSendOrder(t.Context(), tc.orderType, futuresTestPair, tc.side, tc.triggerSignal, "", tc.reduceOnly, order.GoodTillCancel, 1, 1, 0)
			require.ErrorContains(t, err, tc.expectedErrMsg, "FuturesSendOrder must return the expected validation error")
		})
	}
}

func TestMockedFuturesCancelOrder(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/cancelorder"))
	_, err := k.FuturesCancelOrder(t.Context(), "order-id", "")
	require.NoError(t, err)
}

func TestMockedFuturesGetFills(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/fills"))
	_, err := k.FuturesGetFills(t.Context(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
}

func TestMockedFuturesTransfer(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/transfer"))
	_, err := k.FuturesTransfer(t.Context(), "cash", "futures", "xbt", 1)
	require.NoError(t, err)
}

func TestMockedFuturesGetOpenPositions(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/openpositions"))
	_, err := k.FuturesGetOpenPositions(t.Context())
	require.NoError(t, err)
}

func TestMockedFuturesNotifications(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/notifications"))
	_, err := k.FuturesNotifications(t.Context())
	require.NoError(t, err)
}

func TestMockedFuturesCancelAllOrders(t *testing.T) {
	t.Run("without symbol", func(t *testing.T) {
		k := newMockedKraken(t, mockFuturesResponse("/api/v3/cancelallorders"))
		_, err := k.FuturesCancelAllOrders(t.Context(), currency.EMPTYPAIR)
		require.NoError(t, err)
	})

	t.Run("with symbol", func(t *testing.T) {
		k := newMockedKraken(t, mockFuturesResponse("/api/v3/cancelallorders"))
		_, err := k.FuturesCancelAllOrders(t.Context(), mockedFuturesPair())
		require.NoError(t, err)
	})
}

func TestMockedFuturesCancelAllOrdersAfter(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/cancelallordersafter"))
	_, err := k.FuturesCancelAllOrdersAfter(t.Context(), 60)
	require.NoError(t, err)
}

func TestMockedFuturesOpenOrders(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/openorders"))
	_, err := k.FuturesOpenOrders(t.Context())
	require.NoError(t, err)
}

func TestMockedFuturesRecentOrders(t *testing.T) {
	t.Run("with symbol", func(t *testing.T) {
		k := newMockedKraken(t, mockFuturesResponse("/api/v3/recentorders"))
		_, err := k.FuturesRecentOrders(t.Context(), mockedFuturesPair())
		require.NoError(t, err)
	})

	t.Run("without symbol", func(t *testing.T) {
		k := newMockedKraken(t, mockFuturesResponse("/api/v3/recentorders"))
		_, err := k.FuturesRecentOrders(t.Context(), currency.EMPTYPAIR)
		require.NoError(t, err)
	})
}

func TestMockedFuturesBatchOrder(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		k := newMockedKraken(t, mockFuturesResponse("/api/v3/batchorder"))
		_, err := k.FuturesBatchOrder(t.Context(), []PlaceBatchOrderData{{
			PlaceOrderType: "cancel",
			Symbol:         mockedFuturesPair().String(),
			OrderID:        "order-id",
		}})
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name        string
		orders      []PlaceBatchOrderData
		expectedErr error
	}{
		{
			name: "invalid order type",
			orders: []PlaceBatchOrderData{{
				PlaceOrderType: "invalid-order-type",
				Symbol:         "PF_XBTUSD",
			}},
			expectedErr: errInvalidBatchOrderType,
		},
		{
			name: "missing symbol",
			orders: []PlaceBatchOrderData{{
				PlaceOrderType: "cancel",
				Symbol:         "",
			}},
		},
	} {
		t.Run("validation "+tc.name, func(t *testing.T) {
			k := newMockedKraken(t, mockFuturesResponse("/api/v3/batchorder"))
			_, err := k.FuturesBatchOrder(t.Context(), tc.orders)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr, "FuturesBatchOrder must return the expected validation error")
				return
			}
			require.Error(t, err, "FuturesBatchOrder must return a validation error")
		})
	}
}

func TestMockedFuturesWithdrawToSpotWallet(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/withdrawal"))
	_, err := k.FuturesWithdrawToSpotWallet(t.Context(), "xbt", 1)
	require.NoError(t, err)
}

func TestMockedFuturesGetTransfers(t *testing.T) {
	k := newMockedKraken(t, mockFuturesResponse("/api/v3/transfers"))
	_, err := k.FuturesGetTransfers(t.Context(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
}

func TestKrakenFuturesSigning(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	sig, err := k.signFuturesRequest("secret", "/api/v3/fills", "123", "a=b")
	require.NoError(t, err, "signFuturesRequest must not error")
	assert.NotEmpty(t, sig, "signFuturesRequest should return a signature")
}

func TestKrakenFuturesAuthRequest(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t, mockFuturesResponse("/api/v3/batchorder"))

	var resp map[string]any
	err := k.SendFuturesAuthRequest(t.Context(),
		http.MethodPost,
		"/api/v3/batchorder",
		url.Values{
			"json": {`{"batchOrder":[{"order":"1"}]}`},
		},
		&resp,
	)
	require.NoError(t, err, "SendFuturesAuthRequest must not error")
}

func TestMockedSendAuthenticatedHTTPRequestGET(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t, mockPrivateResult("CreditLines", `{}`))

	var resp *GetCreditLinesResponse
	err := k.sendAuthenticatedHTTPRequest(t.Context(),
		exchange.RestSpot,
		http.MethodGet,
		"CreditLines",
		url.Values{
			"rebase_multiplier": {"1"},
		},
		&resp,
	)
	require.NoError(t, err)
}

func TestRecentDepositsStatusResponseUnmarshalList(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`[{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-1","txid":"tx-1","info":"ok","amount":"1","fee":"0.1","time":1700000000,"status":"Success"}]`), &resp)
	require.NoError(t, err)
	assert.Equal(t, RecentDepositsStatusResponse{
		Deposits: []RecentDepositStatus{{
			Method:        "Bitcoin",
			AssetClass:    "currency",
			Asset:         "XBT",
			ReferenceID:   "ref-1",
			TransactionID: "tx-1",
			Information:   "ok",
			Amount:        types.Number(1),
			Fee:           types.Number(0.1),
			Time:          types.Time(time.Unix(1700000000, 0)),
			Status:        "Success",
		}},
	}, resp)
}

func TestRecentDepositsStatusResponseUnmarshalSingle(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-2","txid":"tx-2","info":"ok","amount":"2","fee":"0.2","time":1700000001,"status":"Success"}`), &resp)
	require.NoError(t, err)
	assert.Equal(t, RecentDepositsStatusResponse{
		Deposits: []RecentDepositStatus{{
			Method:        "Bitcoin",
			AssetClass:    "currency",
			Asset:         "XBT",
			ReferenceID:   "ref-2",
			TransactionID: "tx-2",
			Information:   "ok",
			Amount:        types.Number(2),
			Fee:           types.Number(0.2),
			Time:          types.Time(time.Unix(1700000001, 0)),
			Status:        "Success",
		}},
	}, resp)
}

func TestRecentDepositsStatusResponseUnmarshalPaginatedList(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`{"deposit":[{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-3","txid":"tx-3","info":"ok","amount":"3","fee":"0.3","time":1700000002,"status":"Success"}],"next_cursor":"cursor-1"}`), &resp)
	require.NoError(t, err)
	assert.Equal(t, RecentDepositsStatusResponse{
		Deposits: []RecentDepositStatus{{
			Method:        "Bitcoin",
			AssetClass:    "currency",
			Asset:         "XBT",
			ReferenceID:   "ref-3",
			TransactionID: "tx-3",
			Information:   "ok",
			Amount:        types.Number(3),
			Fee:           types.Number(0.3),
			Time:          types.Time(time.Unix(1700000002, 0)),
			Status:        "Success",
		}},
		NextCursor: "cursor-1",
	}, resp)
}

func TestRecentDepositsStatusResponseUnmarshalPaginatedSingle(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`{"deposit":{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-4","txid":"tx-4","info":"ok","amount":"4","fee":"0.4","time":1700000003,"status":"Success"},"next_cursor":"cursor-2"}`), &resp)
	require.NoError(t, err)
	assert.Equal(t, RecentDepositsStatusResponse{
		Deposits: []RecentDepositStatus{{
			Method:        "Bitcoin",
			AssetClass:    "currency",
			Asset:         "XBT",
			ReferenceID:   "ref-4",
			TransactionID: "tx-4",
			Information:   "ok",
			Amount:        types.Number(4),
			Fee:           types.Number(0.4),
			Time:          types.Time(time.Unix(1700000003, 0)),
			Status:        "Success",
		}},
		NextCursor: "cursor-2",
	}, resp)
}

func TestRecentDepositsStatusResponseUnmarshalInvalidDeposit(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`{"deposit":"invalid","next_cursor":"cursor-3"}`), &resp)
	require.Error(t, err)
}

func TestRecentDepositsStatusResponseUnmarshalEmptyPaginatedDeposit(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`{"next_cursor":"cursor-4"}`), &resp)
	require.NoError(t, err)
	assert.Equal(t, RecentDepositsStatusResponse{NextCursor: "cursor-4"}, resp)
}

func TestRecentDepositsStatusResponseUnmarshalInvalidPayload(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`123`), &resp)
	require.Error(t, err)
}

func TestCreateSubaccountResponseUnmarshal(t *testing.T) {
	t.Parallel()

	var resp CreateSubaccountResponse
	err := json.Unmarshal([]byte(`true`), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Created)
}

func TestAllocateEarnFundsResponseUnmarshalNull(t *testing.T) {
	t.Parallel()

	var resp AllocateEarnFundsResponse
	err := json.Unmarshal([]byte(`null`), &resp)
	require.NoError(t, err)
	assert.Nil(t, resp.Success)
}

func TestAllocateEarnFundsResponseUnmarshalBoolean(t *testing.T) {
	t.Parallel()

	var resp AllocateEarnFundsResponse
	err := json.Unmarshal([]byte(`true`), &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Success)
	assert.True(t, *resp.Success)
}

func TestAllocateEarnFundsResponseUnmarshalInvalidPayload(t *testing.T) {
	t.Parallel()

	var resp AllocateEarnFundsResponse
	err := json.Unmarshal([]byte(`"invalid"`), &resp)
	require.Error(t, err)
}

func TestDeallocateEarnFundsResponseUnmarshalNull(t *testing.T) {
	t.Parallel()

	var resp DeallocateEarnFundsResponse
	err := json.Unmarshal([]byte(`null`), &resp)
	require.NoError(t, err)
	assert.Nil(t, resp.Success)
}

func TestDeallocateEarnFundsResponseUnmarshalBoolean(t *testing.T) {
	t.Parallel()

	var resp DeallocateEarnFundsResponse
	err := json.Unmarshal([]byte(`true`), &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Success)
	assert.True(t, *resp.Success)
}

func TestDeallocateEarnFundsResponseUnmarshalInvalidPayload(t *testing.T) {
	t.Parallel()

	var resp DeallocateEarnFundsResponse
	err := json.Unmarshal([]byte(`"invalid"`), &resp)
	require.Error(t, err)
}
