package kraken

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func spotResult(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"error":[],"result":`+result+`}`)
}

func futuresResult(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, result)
}

func krakenMockRESTHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/0/public/SystemStatus"):
		spotResult(w, `{"status":"online","timestamp":"2025-01-01T00:00:00Z"}`)
		return
	case strings.HasPrefix(r.URL.Path, "/0/public/GroupedBook"):
		spotResult(w, `{"pair":"XBT/USD","grouping":1,"bids":[],"asks":[]}`)
		return
	case strings.HasPrefix(r.URL.Path, "/0/public/PreTrade"):
		spotResult(w, `{"symbol":"XBT/USD","description":"test","base_asset":"XBT","base_notation":"XBT","quote_asset":"USD","quote_notation":"USD","venue":"test","system":"test","bids":[],"asks":[]}`)
		return
	case strings.HasPrefix(r.URL.Path, "/0/public/PostTrade"):
		spotResult(w, `{"last_ts":"2025-01-01T00:00:00Z","count":0,"trades":[]}`)
		return
	case strings.HasPrefix(r.URL.Path, "/0/private/"):
		method := strings.TrimPrefix(r.URL.Path, "/0/private/")
		if method == "RetrieveExport" {
			_, _ = io.WriteString(w, "id,field\n1,test\n")
			return
		}
		switch method {
		case "Level3":
			spotResult(w, `{"pair":"XBT/USD","bids":[],"asks":[]}`)
		case "Balance":
			spotResult(w, `{"XXBT":"0.1"}`)
		case "BalanceEx":
			spotResult(w, `{"XXBT":{"balance":"0.1","hold_trade":"0.01"}}`)
		case "WithdrawInfo":
			spotResult(w, `{}`)
		case "Withdraw":
			spotResult(w, `{"refid":"ref-123"}`)
		case "DepositMethods":
			spotResult(w, `[{"method":"Bitcoin","limit":false,"fee":"0.0001","address-setup-fee":"0"}]`)
		case "DepositAddresses":
			spotResult(w, `[{"address":"bc1qexample","expiretm":"0","tag":"","new":false}]`)
		case "TradeBalance":
			spotResult(w, `{}`)
		case "OpenOrders":
			spotResult(w, `{"open":{}}`)
		case "ClosedOrders":
			spotResult(w, `{}`)
		case "QueryOrders":
			spotResult(w, `{}`)
		case "TradesHistory":
			spotResult(w, `{}`)
		case "QueryTrades":
			spotResult(w, `{}`)
		case "OpenPositions":
			spotResult(w, `{}`)
		case "Ledgers":
			spotResult(w, `{}`)
		case "QueryLedgers":
			spotResult(w, `{}`)
		case "TradeVolume":
			spotResult(w, `{}`)
		case "CreditLines":
			spotResult(w, `{}`)
		case "OrderAmends":
			spotResult(w, `{}`)
		case "AddExport":
			spotResult(w, `{"id":"export-id"}`)
		case "ExportStatus":
			spotResult(w, `[]`)
		case "RemoveExport":
			spotResult(w, `{"delete":true,"cancel":false}`)
		case "AmendOrder":
			spotResult(w, `{"amend_id":"amend-id"}`)
		case "CancelAll":
			spotResult(w, `{"count":1,"pending":false}`)
		case "CancelAllOrdersAfter":
			spotResult(w, `{"currentTime":"now","triggerTime":"later"}`)
		case "AddOrderBatch":
			spotResult(w, `{"orders":[]}`)
		case "CancelOrderBatch":
			spotResult(w, `{"count":1}`)
		case "EditOrder":
			spotResult(w, `{"status":"ok"}`)
		case "DepositStatus":
			spotResult(w, `[]`)
		case "WithdrawStatus":
			spotResult(w, `[{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-1","txid":"tx-1","info":"ok","amount":"1","fee":"0.1","time":1700000000,"status":"Success"}]`)
		case "WithdrawCancel":
			spotResult(w, `true`)
		case "WithdrawMethods":
			spotResult(w, `[]`)
		case "WithdrawAddresses":
			spotResult(w, `[]`)
		case "WalletTransfer":
			spotResult(w, `{"refid":"wallet-ref"}`)
		case "CreateSubaccount":
			spotResult(w, `true`)
		case "AccountTransfer":
			spotResult(w, `{"transfer_id":"transfer-id","status":"ok"}`)
		case "Earn/Allocate":
			spotResult(w, `true`)
		case "Earn/Deallocate":
			spotResult(w, `true`)
		case "Earn/AllocateStatus":
			spotResult(w, `{"pending":false}`)
		case "Earn/DeallocateStatus":
			spotResult(w, `{"pending":false}`)
		case "Earn/Strategies":
			spotResult(w, `{"items":[],"next_cursor":""}`)
		case "Earn/Allocations":
			spotResult(w, `{"converted_asset":"USD","total_allocated":"0","total_rewarded":"0","items":[]}`)
		case "GetWebSocketsToken":
			spotResult(w, `{"token":"mock-token","expires":600}`)
		default:
			http.NotFound(w, r)
		}
		return
	case strings.HasPrefix(r.URL.Path, "/api/v3/"),
		strings.HasPrefix(r.URL.Path, "/charts/v1/"),
		strings.HasPrefix(r.URL.Path, "/history/v2/market/"):
		futuresResult(w, `{"result":"success"}`)
		return
	default:
		http.NotFound(w, r)
	}
}

func newMockedKraken(t *testing.T) *Exchange {
	t.Helper()

	k := new(Exchange)
	require.NoError(t, testexch.Setup(k), "Setup must not error")

	k.SkipAuthCheck = true
	k.SetCredentials("test", "secret", "", "", "", "")

	server := httptest.NewServer(http.HandlerFunc(krakenMockRESTHandler))
	t.Cleanup(server.Close)

	require.NoError(t, k.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL rest spot must not error")
	require.NoError(t, k.API.Endpoints.SetRunningURL(exchange.RestFutures.String(), server.URL), "SetRunningURL rest futures must not error")
	require.NoError(t, k.API.Endpoints.SetRunningURL(exchange.RestFuturesSupplementary.String(), server.URL+"/"), "SetRunningURL rest futures supplementary must not error")

	return k
}

func newMockedKrakenWithRESTHandler(t *testing.T, handler http.HandlerFunc) *Exchange {
	t.Helper()

	k := new(Exchange)
	require.NoError(t, testexch.Setup(k), "Setup must not error")

	k.SkipAuthCheck = true
	k.SetCredentials("test", "secret", "", "", "", "")

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
		copied[key] = append([]string(nil), value...)
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
	k := newMockedKrakenWithRESTHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, pathPrefix) {
			captured <- copyValues(r.URL.Query())
			spotResult(w, result)
			return
		}
		http.NotFound(w, r)
	})

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
	k := newMockedKrakenWithRESTHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, pathPrefix) {
			captured <- copyValues(decodeBodyValues(t, r))
			spotResult(w, result)
			return
		}
		http.NotFound(w, r)
	})

	require.NoError(t, invoke(k), "invoke must not error")
	return <-captured
}

func mockedFuturesPair() currency.Pair {
	return currency.NewPairWithDelimiter("PF", "XBTUSD", "_")
}

func TestMockedGetSystemStatus(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetSystemStatus(t.Context())
	require.NoError(t, err)
}

func TestMockedGetGroupedOrderBook(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetGroupedOrderBook(t.Context(), &GroupedOrderBookRequest{Pair: spotTestPair, Depth: 10, Grouping: 1})
	require.NoError(t, err)
}

func TestMockedGetGroupedOrderBookNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetGroupedOrderBook(t.Context(), &GroupedOrderBookRequest{})
	require.NoError(t, err)
}

func TestMockedQueryLevel3OrderBook(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.QueryLevel3OrderBook(t.Context(), &QueryLevel3OrderBookRequest{Pair: spotTestPair, Depth: 5})
	require.NoError(t, err)
}

func TestMockedQueryLevel3OrderBookNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.QueryLevel3OrderBook(t.Context(), &QueryLevel3OrderBookRequest{})
	require.NoError(t, err)
}

func TestMockedGetAccountBalance(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetAccountBalance(t.Context(), &GetAccountBalanceRequest{})
	require.NoError(t, err)
}

func TestMockedGetExtendedBalance(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetExtendedBalance(t.Context(), &GetExtendedBalanceRequest{})
	require.NoError(t, err)
}

func TestMockedGetBalance(t *testing.T) {
	k := newMockedKraken(t)
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
		`{"XBTUSD":[[1700000000,"1","2","0.5","1.5","1.2","5",10]],"last":1700000000}`,
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
	k := newMockedKraken(t)
	_, err := k.GetWithdrawInfo(t.Context(), "XBT", "wallet", 1)
	require.NoError(t, err)
}

func TestMockedWithdraw(t *testing.T) {
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
	_, err := k.GetDepositMethods(t.Context(), &GetDepositMethodsRequest{
		Asset: "XBT",
	})
	require.NoError(t, err)
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
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
	_, err := k.GetTradeBalance(t.Context(), &TradeBalanceOptions{Asset: "XBT"})
	require.NoError(t, err)
}

func TestGetTradeBalanceRequestParams(t *testing.T) {
	t.Parallel()

	captured := make(chan url.Values, 1)
	k := newMockedKrakenWithRESTHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/0/private/TradeBalance") {
			captured <- copyValues(decodeBodyValues(t, r))
			spotResult(w, `{}`)
			return
		}
		http.NotFound(w, r)
	})

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
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
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

func TestQueryTradesNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.QueryTrades(t.Context(), &QueryTradesRequest{})
	require.ErrorContains(t, err, "transaction id is required")
}

func TestQueryTradesMissingTransactionID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.QueryTrades(t.Context(), &QueryTradesRequest{})
	require.ErrorContains(t, err, "transaction id is required")
}

func TestMockedOpenPositions(t *testing.T) {
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
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

func TestQueryLedgersNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.QueryLedgers(t.Context(), &QueryLedgersRequest{})
	require.ErrorContains(t, err, "id is required")
}

func TestQueryLedgersMissingID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.QueryLedgers(t.Context(), &QueryLedgersRequest{})
	require.ErrorContains(t, err, "id is required")
}

func TestMockedGetTradeVolume(t *testing.T) {
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
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
	k := newMockedKraken(t)
	_, err := k.WithdrawCancel(t.Context(), currency.XBT, "ref-1")
	require.NoError(t, err)
}

func TestMockedGetCreditLines(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetCreditLines(t.Context(), &GetCreditLinesRequest{})
	require.NoError(t, err)
}

func TestMockedGetCreditLinesNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetCreditLines(t.Context(), &GetCreditLinesRequest{})
	require.NoError(t, err)
}

func TestMockedGetCreditLinesWithRebaseMultiplier(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetCreditLines(t.Context(), &GetCreditLinesRequest{RebaseMultiplier: "1"})
	require.NoError(t, err)
}

func TestMockedGetOrderAmends(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetOrderAmends(t.Context(), &GetOrderAmendsRequest{OrderID: "order-id"})
	require.NoError(t, err)
}

func TestMockedGetOrderAmendsNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetOrderAmends(t.Context(), &GetOrderAmendsRequest{})
	require.ErrorContains(t, err, "order id is required")
}

func TestMockedGetOrderAmendsMissingOrderID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetOrderAmends(t.Context(), &GetOrderAmendsRequest{})
	require.ErrorContains(t, err, "order id is required")
}

func TestMockedGetOrderAmendsWithRebaseMultiplier(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetOrderAmends(t.Context(), &GetOrderAmendsRequest{OrderID: "order-id", RebaseMultiplier: "1"})
	require.NoError(t, err)
}

func TestMockedRequestExportReport(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.RequestExportReport(t.Context(), &RequestExportReportRequest{Report: "trades", Format: "CSV"})
	require.NoError(t, err)
}

func TestMockedRequestExportReportNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.RequestExportReport(t.Context(), &RequestExportReportRequest{})
	require.ErrorContains(t, err, "report is required")
}

func TestMockedRequestExportReportMissingReport(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.RequestExportReport(t.Context(), &RequestExportReportRequest{Format: "CSV"})
	require.ErrorContains(t, err, "report is required")
}

func TestMockedRequestExportReportMissingFormat(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.RequestExportReport(t.Context(), &RequestExportReportRequest{Report: "trades"})
	require.ErrorContains(t, err, "format is required")
}

func TestMockedRequestExportReportWithOptionalFields(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.RequestExportReport(t.Context(), &RequestExportReportRequest{
		Report:      "ledgers",
		Format:      "CSV",
		Description: "test export",
		Fields:      "refid,time",
		StartTime:   1700000000,
		EndTime:     1700003600,
	})
	require.NoError(t, err)
}

func TestMockedGetExportReportStatus(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetExportReportStatus(t.Context(), &GetExportReportStatusRequest{Report: "trades"})
	require.NoError(t, err)
}

func TestMockedGetExportReportStatusNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetExportReportStatus(t.Context(), &GetExportReportStatusRequest{})
	require.ErrorContains(t, err, "report is required")
}

func TestMockedGetExportReportStatusMissingReport(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetExportReportStatus(t.Context(), &GetExportReportStatusRequest{})
	require.ErrorContains(t, err, "report is required")
}

func TestMockedRetrieveDataExport(t *testing.T) {
	k := newMockedKraken(t)
	data, err := k.RetrieveDataExport(t.Context(), &RetrieveDataExportRequest{ID: "export-id"})
	require.NoError(t, err)
	assert.Contains(t, string(data), "id,field")
}

func TestMockedRetrieveDataExportNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.RetrieveDataExport(t.Context(), &RetrieveDataExportRequest{})
	require.ErrorContains(t, err, "id is required")
}

func TestMockedRetrieveDataExportMissingID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.RetrieveDataExport(t.Context(), &RetrieveDataExportRequest{})
	require.ErrorContains(t, err, "id is required")
}

func TestMockedDeleteExportReport(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.DeleteExportReport(t.Context(), &DeleteExportReportRequest{ID: "export-id", Type: "delete"})
	require.NoError(t, err)
}

func TestMockedDeleteExportReportNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.DeleteExportReport(t.Context(), &DeleteExportReportRequest{})
	require.ErrorContains(t, err, "id is required")
}

func TestMockedDeleteExportReportMissingID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.DeleteExportReport(t.Context(), &DeleteExportReportRequest{})
	require.ErrorContains(t, err, "id is required")
}

func TestMockedDeleteExportReportMissingType(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.DeleteExportReport(t.Context(), &DeleteExportReportRequest{ID: "export-id"})
	require.ErrorContains(t, err, "type is required")
}

func TestMockedAmendOrder(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AmendOrder(t.Context(), &AmendOrderRequest{TransactionID: "txid"})
	require.NoError(t, err)
}

func TestMockedAmendOrderNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AmendOrder(t.Context(), &AmendOrderRequest{})
	require.NoError(t, err)
}

func TestMockedAmendOrderWithOptionalFields(t *testing.T) {
	k := newMockedKraken(t)
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
}

func TestMockedCancelAllOrdersREST(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CancelAllOrdersREST(t.Context())
	require.NoError(t, err)
}

func TestMockedCancelAllOrdersAfter(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CancelAllOrdersAfter(t.Context(), &CancelAllOrdersAfterRequest{Timeout: 10})
	require.NoError(t, err)
}

func TestMockedCancelAllOrdersAfterNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CancelAllOrdersAfter(t.Context(), &CancelAllOrdersAfterRequest{})
	require.ErrorContains(t, err, "timeout must be greater than zero")
}

func TestMockedCancelAllOrdersAfterZeroTimeout(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CancelAllOrdersAfter(t.Context(), &CancelAllOrdersAfterRequest{})
	require.ErrorContains(t, err, "timeout must be greater than zero")
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
	k := newMockedKraken(t)
	_, err := k.AddOrderBatch(t.Context(), &AddOrderBatchRequest{Orders: []AddOrderBatchOrderRequest{{OrderType: "limit", OrderSide: "buy", Volume: "1"}}})
	require.NoError(t, err)
}

func TestMockedAddOrderBatchNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AddOrderBatch(t.Context(), &AddOrderBatchRequest{})
	require.ErrorContains(t, err, "orders are required")
}

func TestMockedAddOrderBatchMissingOrders(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AddOrderBatch(t.Context(), &AddOrderBatchRequest{})
	require.ErrorContains(t, err, "orders are required")
}

func TestMockedAddOrderBatchWithOptionalFields(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AddOrderBatch(t.Context(), &AddOrderBatchRequest{
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
	})
	require.NoError(t, err)
}

func TestMockedCancelOrderBatch(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CancelOrderBatch(t.Context(), &CancelOrderBatchRequest{Orders: []CancelOrderBatchOrderRequest{{TransactionID: "txid"}}})
	require.NoError(t, err)
}

func TestMockedCancelOrderBatchNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CancelOrderBatch(t.Context(), &CancelOrderBatchRequest{})
	require.ErrorContains(t, err, "orders or client orders are required")
}

func TestMockedCancelOrderBatchMissingOrders(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CancelOrderBatch(t.Context(), &CancelOrderBatchRequest{})
	require.ErrorContains(t, err, "orders or client orders are required")
}

func TestMockedCancelOrderBatchWithClientOrders(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CancelOrderBatch(t.Context(), &CancelOrderBatchRequest{
		ClientOrder: []CancelOrderBatchClientOrderIDItem{{ClientOrderID: "client-order-id"}},
	})
	require.NoError(t, err)
}

func TestMockedEditOrder(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.EditOrder(t.Context(), &EditOrderRequest{TransactionID: "txid"})
	require.NoError(t, err)
}

func TestMockedEditOrderNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.EditOrder(t.Context(), &EditOrderRequest{})
	require.ErrorContains(t, err, "transaction id is required")
}

func TestMockedEditOrderMissingTransactionID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.EditOrder(t.Context(), &EditOrderRequest{})
	require.ErrorContains(t, err, "transaction id is required")
}

func TestMockedEditOrderWithOptionalFields(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.EditOrder(t.Context(), &EditOrderRequest{
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
	})
	require.NoError(t, err)
}

func TestMockedGetRecentDepositsStatus(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetRecentDepositsStatus(t.Context(), &GetRecentDepositsStatusRequest{Asset: "XBT"})
	require.NoError(t, err)
}

func TestMockedGetRecentDepositsStatusNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetRecentDepositsStatus(t.Context(), &GetRecentDepositsStatusRequest{})
	require.NoError(t, err)
}

func TestMockedGetRecentDepositsStatusWithOptionalFields(t *testing.T) {
	k := newMockedKraken(t)
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
}

func TestMockedGetWithdrawalMethods(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetWithdrawalMethods(t.Context(), &GetWithdrawalMethodsRequest{Asset: "XBT"})
	require.NoError(t, err)
}

func TestMockedGetWithdrawalMethodsNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetWithdrawalMethods(t.Context(), &GetWithdrawalMethodsRequest{})
	require.ErrorContains(t, err, "asset is required")
}

func TestMockedGetWithdrawalMethodsMissingAsset(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetWithdrawalMethods(t.Context(), &GetWithdrawalMethodsRequest{})
	require.ErrorContains(t, err, "asset is required")
}

func TestMockedGetWithdrawalMethodsWithOptionalFields(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetWithdrawalMethods(t.Context(), &GetWithdrawalMethodsRequest{
		Asset:            "XBT",
		AssetClass:       "currency",
		Network:          "Bitcoin",
		RebaseMultiplier: "1",
	})
	require.NoError(t, err)
}

func TestMockedGetWithdrawalAddresses(t *testing.T) {
	k := newMockedKraken(t)
	verified := true
	_, err := k.GetWithdrawalAddresses(t.Context(), &GetWithdrawalAddressesRequest{Asset: "XBT", Verified: &verified})
	require.NoError(t, err)
}

func TestMockedGetWithdrawalAddressesNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetWithdrawalAddresses(t.Context(), &GetWithdrawalAddressesRequest{})
	require.ErrorContains(t, err, "asset is required")
}

func TestMockedGetWithdrawalAddressesMissingAsset(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetWithdrawalAddresses(t.Context(), &GetWithdrawalAddressesRequest{})
	require.ErrorContains(t, err, "asset is required")
}

func TestMockedGetWithdrawalAddressesWithOptionalFields(t *testing.T) {
	k := newMockedKraken(t)
	verified := false
	_, err := k.GetWithdrawalAddresses(t.Context(), &GetWithdrawalAddressesRequest{
		Asset:      "XBT",
		AssetClass: "currency",
		Method:     "Bitcoin",
		Key:        "primary",
		Verified:   &verified,
	})
	require.NoError(t, err)
}

func TestMockedWalletTransfer(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.WalletTransfer(t.Context(), &WalletTransferRequest{Asset: "XBT", From: "spot", To: "margin", Amount: "1"})
	require.NoError(t, err)
}

func TestMockedWalletTransferNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.WalletTransfer(t.Context(), &WalletTransferRequest{})
	require.ErrorContains(t, err, "asset is required")
}

func TestMockedWalletTransferMissingAsset(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.WalletTransfer(t.Context(), &WalletTransferRequest{})
	require.ErrorContains(t, err, "asset is required")
}

func TestMockedWalletTransferMissingFrom(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.WalletTransfer(t.Context(), &WalletTransferRequest{Asset: "XBT", To: "margin", Amount: "1"})
	require.ErrorContains(t, err, "from is required")
}

func TestMockedWalletTransferMissingTo(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.WalletTransfer(t.Context(), &WalletTransferRequest{Asset: "XBT", From: "spot", Amount: "1"})
	require.ErrorContains(t, err, "to is required")
}

func TestMockedWalletTransferMissingAmount(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.WalletTransfer(t.Context(), &WalletTransferRequest{Asset: "XBT", From: "spot", To: "margin"})
	require.ErrorContains(t, err, "amount is required")
}

func TestMockedCreateSubaccount(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CreateSubaccount(t.Context(), &CreateSubaccountRequest{Username: "alice", Email: "alice@example.com"})
	require.NoError(t, err)
}

func TestMockedCreateSubaccountNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CreateSubaccount(t.Context(), &CreateSubaccountRequest{})
	require.ErrorContains(t, err, "username is required")
}

func TestMockedCreateSubaccountMissingUsername(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CreateSubaccount(t.Context(), &CreateSubaccountRequest{})
	require.ErrorContains(t, err, "username is required")
}

func TestMockedCreateSubaccountMissingEmail(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.CreateSubaccount(t.Context(), &CreateSubaccountRequest{Username: "alice"})
	require.ErrorContains(t, err, "email is required")
}

func TestMockedAccountTransfer(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{Asset: "XBT", Amount: "1", From: "main", To: "sub"})
	require.NoError(t, err)
}

func TestMockedAccountTransferNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{})
	require.ErrorContains(t, err, "asset is required")
}

func TestMockedAccountTransferMissingAsset(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{})
	require.ErrorContains(t, err, "asset is required")
}

func TestMockedAccountTransferMissingAmount(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{Asset: "XBT", From: "main", To: "sub"})
	require.ErrorContains(t, err, "amount is required")
}

func TestMockedAccountTransferMissingFrom(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{Asset: "XBT", Amount: "1", To: "sub"})
	require.ErrorContains(t, err, "from is required")
}

func TestMockedAccountTransferMissingTo(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{Asset: "XBT", Amount: "1", From: "main"})
	require.ErrorContains(t, err, "to is required")
}

func TestMockedAccountTransferWithAssetClass(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AccountTransfer(t.Context(), &AccountTransferRequest{
		Asset:      "XBT",
		AssetClass: "currency",
		Amount:     "1",
		From:       "main",
		To:         "sub",
	})
	require.NoError(t, err)
}

func TestMockedAllocateEarnFunds(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AllocateEarnFunds(t.Context(), &AllocateEarnFundsRequest{Amount: "1", StrategyID: "strategy-id"})
	require.NoError(t, err)
}

func TestMockedAllocateEarnFundsNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AllocateEarnFunds(t.Context(), &AllocateEarnFundsRequest{})
	require.ErrorContains(t, err, "amount is required")
}

func TestMockedAllocateEarnFundsMissingAmount(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AllocateEarnFunds(t.Context(), &AllocateEarnFundsRequest{})
	require.ErrorContains(t, err, "amount is required")
}

func TestMockedAllocateEarnFundsMissingStrategyID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.AllocateEarnFunds(t.Context(), &AllocateEarnFundsRequest{Amount: "1"})
	require.ErrorContains(t, err, "strategy id is required")
}

func TestMockedDeallocateEarnFunds(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.DeallocateEarnFunds(t.Context(), &DeallocateEarnFundsRequest{Amount: "1", StrategyID: "strategy-id"})
	require.NoError(t, err)
}

func TestMockedDeallocateEarnFundsNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.DeallocateEarnFunds(t.Context(), &DeallocateEarnFundsRequest{})
	require.ErrorContains(t, err, "amount is required")
}

func TestMockedDeallocateEarnFundsMissingAmount(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.DeallocateEarnFunds(t.Context(), &DeallocateEarnFundsRequest{})
	require.ErrorContains(t, err, "amount is required")
}

func TestMockedDeallocateEarnFundsMissingStrategyID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.DeallocateEarnFunds(t.Context(), &DeallocateEarnFundsRequest{Amount: "1"})
	require.ErrorContains(t, err, "strategy id is required")
}

func TestMockedGetEarnAllocationStatus(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetEarnAllocationStatus(t.Context(), &EarnOperationStatusRequest{StrategyID: "strategy-id"})
	require.NoError(t, err)
}

func TestMockedGetEarnAllocationStatusNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetEarnAllocationStatus(t.Context(), &EarnOperationStatusRequest{})
	require.ErrorContains(t, err, "strategy id is required")
}

func TestMockedGetEarnAllocationStatusMissingStrategyID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetEarnAllocationStatus(t.Context(), &EarnOperationStatusRequest{})
	require.ErrorContains(t, err, "strategy id is required")
}

func TestMockedGetEarnDeallocationStatus(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetEarnDeallocationStatus(t.Context(), &EarnOperationStatusRequest{StrategyID: "strategy-id"})
	require.NoError(t, err)
}

func TestMockedGetEarnDeallocationStatusNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetEarnDeallocationStatus(t.Context(), &EarnOperationStatusRequest{})
	require.ErrorContains(t, err, "strategy id is required")
}

func TestMockedGetEarnDeallocationStatusMissingStrategyID(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetEarnDeallocationStatus(t.Context(), &EarnOperationStatusRequest{})
	require.ErrorContains(t, err, "strategy id is required")
}

func TestMockedListEarnStrategies(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.ListEarnStrategies(t.Context(), &ListEarnStrategiesRequest{LockType: []string{"bonded"}})
	require.NoError(t, err)
}

func TestMockedListEarnStrategiesNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.ListEarnStrategies(t.Context(), &ListEarnStrategiesRequest{})
	require.NoError(t, err)
}

func TestMockedListEarnStrategiesWithOptionalFields(t *testing.T) {
	k := newMockedKraken(t)
	ascending := true
	_, err := k.ListEarnStrategies(t.Context(), &ListEarnStrategiesRequest{
		Ascending: &ascending,
		Asset:     "XBT",
		Cursor:    "cursor",
		Limit:     10,
		LockType:  []string{"bonded"},
	})
	require.NoError(t, err)
}

func TestMockedListEarnAllocations(t *testing.T) {
	k := newMockedKraken(t)
	ascending := true
	hideZeros := false
	_, err := k.ListEarnAllocations(t.Context(), &ListEarnAllocationsRequest{Ascending: &ascending, HideZeroAllocations: &hideZeros})
	require.NoError(t, err)
}

func TestMockedListEarnAllocationsNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.ListEarnAllocations(t.Context(), &ListEarnAllocationsRequest{})
	require.NoError(t, err)
}

func TestMockedListEarnAllocationsWithConvertedAsset(t *testing.T) {
	k := newMockedKraken(t)
	ascending := false
	hideZeros := true
	_, err := k.ListEarnAllocations(t.Context(), &ListEarnAllocationsRequest{
		Ascending:           &ascending,
		ConvertedAsset:      "USD",
		HideZeroAllocations: &hideZeros,
	})
	require.NoError(t, err)
}

func TestMockedGetPreTradeData(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetPreTradeData(t.Context(), &GetPreTradeDataRequest{Symbol: "XBT/USD"})
	require.NoError(t, err)
}

func TestMockedGetPostTradeData(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetPostTradeData(t.Context(), &GetPostTradeDataRequest{
		Symbol:        "XBT/USD",
		FromTimestamp: time.Now().Add(-time.Hour),
		ToTimestamp:   time.Now(),
		Count:         10,
	})
	require.NoError(t, err)
}

func TestMockedGetPostTradeDataNilRequest(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetPostTradeData(t.Context(), &GetPostTradeDataRequest{})
	require.NoError(t, err)
}

func TestMockedGetFuturesTickerBySymbol(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.GetFuturesTickerBySymbol(t.Context(), "PF_XBTUSD")
	require.NoError(t, err)
}

func TestMockedFuturesEditOrder(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesEditOrder(t.Context(), "order-id", "", 1, 2, 0)
	require.NoError(t, err)
}

func TestMockedFuturesSendOrder(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesSendOrder(t.Context(), order.Limit, mockedFuturesPair(), "buy", "", "", "", order.ImmediateOrCancel, 1, 2, 0)
	require.NoError(t, err)
}

func TestMockedFuturesSendOrderPostOnly(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesSendOrder(t.Context(), order.Limit, mockedFuturesPair(), "buy", "mark", "client-order-id", "true", order.PostOnly, 1, 2, 1.5)
	require.NoError(t, err)
}

func TestMockedFuturesCancelOrder(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesCancelOrder(t.Context(), "order-id", "")
	require.NoError(t, err)
}

func TestMockedFuturesGetFills(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesGetFills(t.Context(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
}

func TestMockedFuturesTransfer(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesTransfer(t.Context(), "cash", "futures", "xbt", 1)
	require.NoError(t, err)
}

func TestMockedFuturesGetOpenPositions(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesGetOpenPositions(t.Context())
	require.NoError(t, err)
}

func TestMockedFuturesNotifications(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesNotifications(t.Context())
	require.NoError(t, err)
}

func TestMockedFuturesCancelAllOrders(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesCancelAllOrders(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
}

func TestMockedFuturesCancelAllOrdersWithSymbol(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesCancelAllOrders(t.Context(), mockedFuturesPair())
	require.NoError(t, err)
}

func TestMockedFuturesCancelAllOrdersAfter(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesCancelAllOrdersAfter(t.Context(), 60)
	require.NoError(t, err)
}

func TestMockedFuturesOpenOrders(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesOpenOrders(t.Context())
	require.NoError(t, err)
}

func TestMockedFuturesRecentOrders(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesRecentOrders(t.Context(), mockedFuturesPair())
	require.NoError(t, err)
}

func TestMockedFuturesRecentOrdersEmptySymbol(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesRecentOrders(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
}

func TestMockedFuturesBatchOrder(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesBatchOrder(t.Context(), []PlaceBatchOrderData{{
		PlaceOrderType: "cancel",
		Symbol:         mockedFuturesPair().String(),
		OrderID:        "order-id",
	}})
	require.NoError(t, err)
}

func TestMockedFuturesWithdrawToSpotWallet(t *testing.T) {
	k := newMockedKraken(t)
	_, err := k.FuturesWithdrawToSpotWallet(t.Context(), "xbt", 1)
	require.NoError(t, err)
}

func TestMockedFuturesGetTransfers(t *testing.T) {
	k := newMockedKraken(t)
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

	k := newMockedKraken(t)

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

	k := newMockedKraken(t)

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

func TestMockedFuturesSendOrderInvalidOrderType(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	_, err := k.FuturesSendOrder(t.Context(), order.UnknownType, futuresTestPair, "buy", "", "", "", order.GoodTillCancel, 1, 1, 0)
	require.ErrorContains(t, err, "invalid orderType", "FuturesSendOrder must error on invalid order type")
}

func TestMockedFuturesSendOrderInvalidSide(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	_, err := k.FuturesSendOrder(t.Context(), order.Limit, futuresTestPair, "invalid-side", "", "", "", order.GoodTillCancel, 1, 1, 0)
	require.ErrorContains(t, err, "invalid side", "FuturesSendOrder must error on invalid side")
}

func TestMockedFuturesSendOrderInvalidTriggerSignal(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	_, err := k.FuturesSendOrder(t.Context(), order.Limit, futuresTestPair, "buy", "invalid-trigger", "", "", order.GoodTillCancel, 1, 1, 0)
	require.ErrorContains(t, err, "invalid triggerSignal", "FuturesSendOrder must error on invalid trigger signal")
}

func TestMockedFuturesSendOrderInvalidReduceOnly(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	_, err := k.FuturesSendOrder(t.Context(), order.Limit, futuresTestPair, "buy", "", "", "invalid-reduce", order.GoodTillCancel, 1, 1, 0)
	require.ErrorContains(t, err, "invalid reduceOnly")
}

func TestMockedFuturesBatchOrderValidationErrors(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	_, err := k.FuturesBatchOrder(t.Context(), []PlaceBatchOrderData{{
		PlaceOrderType: "invalid-order-type",
		Symbol:         "PF_XBTUSD",
	}})
	require.ErrorIs(t, err, errInvalidBatchOrderType, "FuturesBatchOrder must error on invalid order type")
}

func TestMockedFuturesBatchOrderInvalidSymbol(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	_, err := k.FuturesBatchOrder(t.Context(), []PlaceBatchOrderData{{
		PlaceOrderType: "cancel",
		Symbol:         "",
	}})
	require.Error(t, err)
}

func TestMockedGetPreTradeDataNilRequest(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	_, err := k.GetPreTradeData(t.Context(), &GetPreTradeDataRequest{})
	require.ErrorContains(t, err, "symbol is required", "GetPreTradeData must error on missing symbol")
}

func TestMockedGetPreTradeDataMissingSymbol(t *testing.T) {
	t.Parallel()

	k := newMockedKraken(t)

	_, err := k.GetPreTradeData(t.Context(), &GetPreTradeDataRequest{})
	require.ErrorContains(t, err, "symbol is required", "GetPreTradeData must error on missing symbol")
}

func TestRecentDepositsStatusResponseUnmarshalList(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`[{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-1","txid":"tx-1","info":"ok","amount":"1","fee":"0.1","time":1700000000,"status":"Success"}]`), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Deposits, 1)
	assert.Equal(t, "ref-1", resp.Deposits[0].ReferenceID)
}

func TestRecentDepositsStatusResponseUnmarshalSingle(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-2","txid":"tx-2","info":"ok","amount":"2","fee":"0.2","time":1700000001,"status":"Success"}`), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Deposits, 1)
	assert.Equal(t, "ref-2", resp.Deposits[0].ReferenceID)
}

func TestRecentDepositsStatusResponseUnmarshalPaginatedList(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`{"deposit":[{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-3","txid":"tx-3","info":"ok","amount":"3","fee":"0.3","time":1700000002,"status":"Success"}],"next_cursor":"cursor-1"}`), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Deposits, 1)
	assert.Equal(t, "cursor-1", resp.NextCursor)
}

func TestRecentDepositsStatusResponseUnmarshalPaginatedSingle(t *testing.T) {
	t.Parallel()

	var resp RecentDepositsStatusResponse
	err := json.Unmarshal([]byte(`{"deposit":{"method":"Bitcoin","aclass":"currency","asset":"XBT","refid":"ref-4","txid":"tx-4","info":"ok","amount":"4","fee":"0.4","time":1700000003,"status":"Success"},"next_cursor":"cursor-2"}`), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Deposits, 1)
	assert.Equal(t, "cursor-2", resp.NextCursor)
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
	assert.Equal(t, "cursor-4", resp.NextCursor)
	assert.Empty(t, resp.Deposits)
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
