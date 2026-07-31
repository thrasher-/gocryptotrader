package gateio

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func TestDurationFromSeconds(t *testing.T) {
	t.Parallel()
	duration, err := durationFromSeconds(60)
	require.NoError(t, err)
	assert.Equal(t, time.Minute, duration)

	_, err = durationFromSeconds(math.MaxUint64)
	require.ErrorIs(t, err, common.ErrMalformedData)
}

func TestDecimalIncrementFromPrecision(t *testing.T) {
	t.Parallel()
	increment, err := decimalIncrementFromPrecision(8)
	require.NoError(t, err)
	assert.Equal(t, 1e-8, increment)

	_, err = decimalIncrementFromPrecision(324)
	require.ErrorIs(t, err, common.ErrMalformedData)
}

func newHTTPTestExchange(t *testing.T, handler http.Handler) *Exchange {
	t.Helper()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	require.NoError(t, ex.SetHTTPClient(server.Client()), "SetHTTPClient must not error")
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL must not error")
	return ex
}

func newAuthenticatedHTTPTestExchange(t *testing.T, handler http.Handler) *Exchange {
	t.Helper()
	ex := newHTTPTestExchange(t, handler)
	ex.API.AuthenticatedSupport = true
	ex.SetCredentials(&accounts.Credentials{Key: "key", Secret: "secret"})
	return ex
}

func newHTTPRouteTestExchange(t *testing.T, method, requestURI, response string) *Exchange {
	t.Helper()
	return newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, method, r.Method, "request method should match")
		assert.Equal(t, requestURI, r.URL.RequestURI(), "request URI should match")
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing response should not error")
	}))
}

func newAuthenticatedHTTPRouteTestExchange(t *testing.T, method, requestURI, response string) *Exchange {
	t.Helper()
	return newAuthenticatedHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, method, r.Method, "request method should match")
		assert.Equal(t, requestURI, r.URL.RequestURI(), "request URI should match")
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing response should not error")
	}))
}

func newAuthenticatedJSONRouteTestExchange(t *testing.T, method, requestURI, expectedBody, response string) *Exchange {
	t.Helper()
	return newAuthenticatedHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, method, r.Method, "request method should match")
		assert.Equal(t, requestURI, r.URL.RequestURI(), "request URI should match")
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err, "reading request body should not error") {
			return
		}
		assert.JSONEq(t, expectedBody, string(body), "request body should match")
		_, err = fmt.Fprint(w, response)
		assert.NoError(t, err, "writing response should not error")
	}))
}

func TestCancelAllOrders(t *testing.T) {
	t.Parallel()

	_, err := e.CancelAllOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrCancelOrderIsNil)

	_, err = e.CancelAllOrders(t.Context(), &order.Cancel{Pair: currency.EMPTYPAIR, AssetType: 1336})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.CancelAllOrders(t.Context(), &order.Cancel{Pair: currency.NewBTCUSDT(), AssetType: 1336})
	require.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.CancelAllOrders(t.Context(), &order.Cancel{
		Pair:      currency.NewBTCUSDT(),
		AssetType: asset.Options,
		Side:      order.ClosePosition,
	})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	_, err = e.CancelAllOrders(t.Context(), &order.Cancel{
		Pair:      currency.NewPair(currency.BTC, currency.EMPTYCODE),
		AssetType: asset.USDTMarginedFutures,
		Side:      order.Long,
	})
	require.ErrorIs(t, err, errInvalidSettlementQuote)

	_, err = e.CancelAllOrders(t.Context(), &order.Cancel{
		Pair:      currency.NewPair(currency.BTC, currency.EMPTYCODE),
		AssetType: asset.USDTMarginedFutures,
		Side:      order.Short,
	})
	require.ErrorIs(t, err, errInvalidSettlementQuote)

	_, err = e.CancelAllOrders(t.Context(), &order.Cancel{
		Pair:      currency.NewPair(currency.BTC, currency.EMPTYCODE),
		AssetType: asset.USDTMarginedFutures,
		Side:      order.AnySide,
	})
	require.ErrorIs(t, err, errInvalidSettlementQuote)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}

	for _, a := range e.GetAssetTypes(false) {
		t.Run(a.String(), func(t *testing.T) {
			t.Parallel()
			ex := e
			if mockTests {
				ex = newAuthenticatedHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodDelete, r.Method, "cancel-all method should match")
					assert.NotEmpty(t, r.URL.Path, "cancel-all path should be populated")
					_, err := fmt.Fprint(w, `[]`)
					assert.NoError(t, err, "writing response should not error")
				}))
			}
			r := &order.Cancel{
				OrderID:   "1",
				AccountID: "1",
				AssetType: a,
				Pair:      currency.EMPTYPAIR,
			}
			_, err := ex.CancelAllOrders(t.Context(), r)
			assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

			r.Pair = getPair(t, a)
			_, err = ex.CancelAllOrders(t.Context(), r)
			assert.NoError(t, err)
		})
	}
}

func TestOpenInterestFromStats(t *testing.T) {
	t.Parallel()

	_, err := openInterestFromStats(nil)
	require.ErrorIs(t, err, errNoValidResponseFromServer)

	openInterest, err := openInterestFromStats([]*ContractStat{
		{Time: types.Time(time.Unix(100, 0)), OpenInterest: types.Number(2)},
		{Time: types.Time(time.Unix(300, 0)), OpenInterest: types.Number(4)},
		{Time: types.Time(time.Unix(200, 0)), OpenInterest: types.Number(3)},
	})
	require.NoError(t, err)
	assert.Equal(t, 4.0, openInterest)
}

func TestUseOpenInterestStats(t *testing.T) {
	t.Parallel()

	assert.False(t, useOpenInterestStats(nil, asset.USDTMarginedFutures))
	assert.False(t, useOpenInterestStats([]key.PairAsset{{Asset: asset.CoinMarginedFutures}, {Asset: asset.CoinMarginedFutures}}, asset.CoinMarginedFutures))
	assert.False(t, useOpenInterestStats([]key.PairAsset{{Asset: asset.CoinMarginedFutures}}, asset.USDTMarginedFutures))
	assert.False(t, useOpenInterestStats([]key.PairAsset{{Asset: asset.DeliveryFutures}}, asset.DeliveryFutures))
	assert.True(t, useOpenInterestStats([]key.PairAsset{{Asset: asset.CoinMarginedFutures}}, asset.CoinMarginedFutures))
	assert.True(t, useOpenInterestStats([]key.PairAsset{{Asset: asset.USDTMarginedFutures}}, asset.USDTMarginedFutures))
}

func TestGetCrossMarginMinimums(t *testing.T) {
	t.Parallel()

	minimums, err := e.getCrossMarginMinimums(t.Context())
	require.NoError(t, err, "getCrossMarginMinimums must not error")
	require.NotEmpty(t, minimums, "getCrossMarginMinimums must return loanable currencies")
	for ccy, minimum := range minimums {
		assert.Falsef(t, ccy.IsEmpty(), "currency should not be empty for minimum %f", minimum)
		assert.Positivef(t, minimum, "minimum should be positive for %s", ccy)
	}
}

func TestUpdateOrderExecutionLimitsUsesProductBorrowMinimums(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	ex.Name = "GateIOProductBorrowMinimums"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "request method should be GET")
		switch r.URL.Path {
		case "/api/v4/spot/currency_pairs":
			_, err := fmt.Fprint(w, `[{"id":"BTC_USDT","base":"BTC","quote":"USDT","min_base_amount":"0.001","min_quote_amount":"1","amount_precision":3,"precision":2,"trade_status":"tradable"}]`)
			assert.NoError(t, err, "writing spot currency pairs should not error")
		case "/api/v4/margin/currency_pairs":
			_, err := fmt.Fprint(w, `[{"id":"BTC_USDT","base":"BTC","quote":"USDT","min_base_amount":"0.01","min_quote_amount":"2","status":1}]`)
			assert.NoError(t, err, "writing margin currency pairs should not error")
		case "/api/v4/margin/cross/currencies":
			_, err := fmt.Fprint(w, `[{"name":"BTC","min_borrow_amount":"0.03","loanable":true,"status":1},{"name":"USDT","min_borrow_amount":"4","loanable":true,"status":1}]`)
			assert.NoError(t, err, "writing cross-margin currencies should not error")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	require.NoError(t, ex.SetHTTPClient(server.Client()), "SetHTTPClient must not error")
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL must not error")

	pair := currency.NewBTCUSDT()
	require.NoError(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.Margin), "UpdateOrderExecutionLimits must not error for margin")
	isolatedLimits, err := ex.GetOrderExecutionLimits(asset.Margin, pair)
	require.NoError(t, err, "GetOrderExecutionLimits must not error for margin")
	assert.Equal(t, 0.01, isolatedLimits.MinimumBorrowAmountBase, "margin base borrow minimum should use the isolated pair value")
	assert.Equal(t, 2.0, isolatedLimits.MinimumBorrowAmountQuote, "margin quote borrow minimum should use the isolated pair value")

	require.NoError(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.CrossMargin), "UpdateOrderExecutionLimits must not error for cross margin")
	crossLimits, err := ex.GetOrderExecutionLimits(asset.CrossMargin, pair)
	require.NoError(t, err, "GetOrderExecutionLimits must not error for cross margin")
	assert.Equal(t, 0.03, crossLimits.MinimumBorrowAmountBase, "cross-margin base borrow minimum should use the currency value")
	assert.Equal(t, 4.0, crossLimits.MinimumBorrowAmountQuote, "cross-margin quote borrow minimum should use the currency value")
}

func TestFetchTradablePairsUsesMarginProductSources(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	ex.Name = "GateIOTradableMarginPairs"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "request method should be GET")
		switch r.URL.Path {
		case "/api/v4/spot/currency_pairs":
			_, err := fmt.Fprint(w, `[{"id":"BTC_USDT","base":"BTC","quote":"USDT","trade_status":"tradable"},{"id":"ETH_USDT","base":"ETH","quote":"USDT","trade_status":"tradable"},{"id":"DOGE_USDT","base":"DOGE","quote":"USDT","trade_status":"untradable"}]`)
			assert.NoError(t, err, "writing spot currency pairs should not error")
		case "/api/v4/margin/currency_pairs":
			_, err := fmt.Fprint(w, `[{"id":"BTC_USDT","base":"BTC","quote":"USDT","min_base_amount":"0.01","status":0},{"id":"ETH_USDT","base":"ETH","quote":"USDT","min_base_amount":"0.02","status":1}]`)
			assert.NoError(t, err, "writing margin currency pairs should not error")
		case "/api/v4/margin/cross/currencies":
			_, err := fmt.Fprint(w, `[{"name":"BTC","min_borrow_amount":"0.03","loanable":true,"status":1},{"name":"USDT","min_borrow_amount":"4","loanable":true,"status":1},{"name":"ETH","min_borrow_amount":"0.05","loanable":true,"status":0},{"name":"DOGE","min_borrow_amount":"1","loanable":true,"status":1}]`)
			assert.NoError(t, err, "writing cross-margin currencies should not error")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	require.NoError(t, ex.SetHTTPClient(server.Client()), "SetHTTPClient must not error")
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL must not error")

	marginPairs, err := ex.FetchTradablePairs(t.Context(), asset.Margin)
	require.NoError(t, err, "FetchTradablePairs must not error for margin")
	require.Len(t, marginPairs, 1, "margin must return one active isolated pair")
	assert.True(t, marginPairs[0].Equal(currency.NewPair(currency.ETH, currency.USDT)), "margin should use the isolated-margin pair endpoint")

	crossPairs, err := ex.FetchTradablePairs(t.Context(), asset.CrossMargin)
	require.NoError(t, err, "FetchTradablePairs must not error for cross margin")
	require.Len(t, crossPairs, 1, "cross margin must return one enabled tradable pair")
	assert.True(t, crossPairs[0].Equal(currency.NewBTCUSDT()), "cross margin should use enabled currencies and tradable spot pairs")
}

func TestFetchTradablePairsExcludesUnavailableFuturesContracts(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "request method should be GET")
		switch r.URL.Path {
		case "/api/v4/futures/usdt/contracts":
			_, err := fmt.Fprint(w, `[
				{"name":"BTC_USDT","type":"direct","status":"trading"},
				{"name":"DELISTING_USDT","type":"direct","status":"trading","in_delisting":true},
				{"name":"DELISTING_STATUS_USDT","type":"direct","status":"delisting"},
				{"name":"DELISTED_STATUS_USDT","type":"direct","status":"delisted"},
				{"name":"DELISTED_TIME_USDT","type":"direct","status":"trading","delisted_time":1700000000},
				{"name":"INVERSE_USDT","type":"inverse","status":"trading"},
				{"name":"","type":"direct","status":"trading"}
			]`)
			assert.NoError(t, err, "writing USDT contracts should not error")
		case "/api/v4/futures/btc/contracts":
			_, err := fmt.Fprint(w, `[
				{"name":"BTC_USD","type":"inverse","status":"trading"},
				{"name":"DIRECT_USD","type":"direct","status":"trading"}
			]`)
			assert.NoError(t, err, "writing BTC contracts should not error")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	require.NoError(t, ex.SetHTTPClient(server.Client()), "SetHTTPClient must not error")
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL must not error")

	usdtPairs, err := ex.FetchTradablePairs(t.Context(), asset.USDTMarginedFutures)
	require.NoError(t, err, "FetchTradablePairs must not error for USDT futures")
	require.Len(t, usdtPairs, 1, "USDT futures must only return active direct contracts")
	assert.True(t, usdtPairs[0].Equal(currency.NewBTCUSDT()), "USDT futures must retain the active contract")

	coinPairs, err := ex.FetchTradablePairs(t.Context(), asset.CoinMarginedFutures)
	require.NoError(t, err, "FetchTradablePairs must not error for coin futures")
	require.Len(t, coinPairs, 1, "coin futures must only return inverse contracts")
	assert.True(t, coinPairs[0].Equal(currency.NewBTCUSD()), "coin futures must retain the active inverse contract")
}

func TestGetActiveOrdersUsesFuturesOrderPrice(t *testing.T) {
	t.Parallel()

	ex := newAuthenticatedHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "request method should be GET")
		assert.Equal(t, "/api/v4/futures/usdt/orders", r.URL.Path, "request path should use USDT futures orders")
		assert.Equal(t, statusOpen, r.URL.Query().Get("status"), "request should select open orders")
		_, err := fmt.Fprint(w, `[{"id":123,"contract":"BTC_USDT","create_time":1700000000,"size":"2","left":"1","price":"123.45","fill_price":"120.25","tif":"gtc","status":"open"}]`)
		assert.NoError(t, err, "writing futures orders should not error")
	}))

	orders, err := ex.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		AssetType: asset.USDTMarginedFutures,
	})
	require.NoError(t, err, "GetActiveOrders must not error")
	require.Len(t, orders, 1, "GetActiveOrders must return one order")
	assert.Equal(t, 123.45, orders[0].Price, "order price must use the submitted order price")
	assert.Equal(t, 120.25, orders[0].AverageExecutedPrice, "average price must use the fill price")
}

func TestGetActiveOrdersUsesOptionsFillPrice(t *testing.T) {
	t.Parallel()

	ex := newAuthenticatedHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "request method should be GET")
		assert.Equal(t, "/api/v4/options/orders", r.URL.Path, "request path should use options orders")
		assert.Equal(t, statusOpen, r.URL.Query().Get("status"), "request should select open orders")
		_, err := fmt.Fprint(w, `[{"id":123,"contract":"BTC_USDT-20211130-50000-C","size":"2","left":"1","price":"123.45","fill_price":"120.25","status":"open"}]`)
		assert.NoError(t, err, "writing options orders should not error")
	}))

	orders, err := ex.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		AssetType: asset.Options,
	})
	require.NoError(t, err, "GetActiveOrders must not error")
	require.Len(t, orders, 1, "GetActiveOrders must return one order")
	assert.Equal(t, 123.45, orders[0].Price, "order price must use the submitted order price")
	assert.Equal(t, 120.25, orders[0].AverageExecutedPrice, "average price must use the fill price")
	assert.Zero(t, orders[0].QuoteAmount, "limit options orders must not invent a quote amount")
}

func TestGetOrderHistoryUsesCrossMarginAccount(t *testing.T) {
	t.Parallel()

	ex := newAuthenticatedHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "request method should be GET")
		assert.Equal(t, "/api/v4/spot/my_trades", r.URL.Path, "request path should use spot trade history")
		assert.Equal(t, asset.CrossMargin.String(), r.URL.Query().Get("account"), "cross-margin history must select the cross-margin account")
		_, err := fmt.Fprint(w, `[]`)
		assert.NoError(t, err, "writing spot trade history should not error")
	}))

	orders, err := ex.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		AssetType: asset.CrossMargin,
	})
	require.NoError(t, err, "GetOrderHistory must not error")
	assert.Empty(t, orders, "empty server history must return no orders")
}

func TestGetRequestedOpenInterestPair(t *testing.T) {
	t.Parallel()

	pair := getPair(t, asset.DeliveryFutures)
	requested, err := getRequestedOpenInterestPair(e, []key.PairAsset{{
		Base:  pair.Base.Item,
		Quote: pair.Quote.Item,
		Asset: asset.DeliveryFutures,
	}}, asset.DeliveryFutures)
	require.NoError(t, err)
	assert.Equal(t, pair, requested)

	requested, err = getRequestedOpenInterestPair(e, []key.PairAsset{{
		Base:  pair.Base.Item,
		Quote: pair.Quote.Item,
		Asset: asset.DeliveryFutures,
	}}, asset.CoinMarginedFutures)
	require.NoError(t, err)
	assert.Equal(t, currency.EMPTYPAIR, requested)

	requested, err = getRequestedOpenInterestPair(e, []key.PairAsset{{Asset: asset.DeliveryFutures}, {Asset: asset.DeliveryFutures}}, asset.DeliveryFutures)
	require.NoError(t, err)
	assert.Equal(t, currency.EMPTYPAIR, requested)
}

func TestIsPerpetualFutureCurrency(t *testing.T) {
	t.Parallel()
	for _, a := range []asset.Item{asset.CoinMarginedFutures, asset.USDTMarginedFutures} {
		is, err := e.IsPerpetualFutureCurrency(a, currency.EMPTYPAIR)
		require.NoErrorf(t, err, "IsPerpetualFutureCurrency must not error for %s", a)
		assert.Truef(t, is, "%s should be a perpetual future currency", a)
	}
	for _, a := range []asset.Item{asset.Spot, asset.Margin, asset.CrossMargin, asset.DeliveryFutures, asset.Options} {
		is, err := e.IsPerpetualFutureCurrency(a, currency.EMPTYPAIR)
		require.NoErrorf(t, err, "IsPerpetualFutureCurrency must not error for %s", a)
		assert.Falsef(t, is, "%s should not be a perpetual future currency", a)
	}
}

func TestMessageID(t *testing.T) {
	t.Parallel()
	id := e.MessageID()
	require.Len(t, id, 32, "message ID must be 32 characters long for usage as a request ID")
	got, err := uuid.FromString(id)
	require.NoError(t, err, "ID string must convert back to a UUID")
	require.Equal(t, uuid.V7, got.Version(), "message ID must be a UUID v7")
	require.Len(t, got.String(), 36, "UUID v7 string representation must be 36 characters long")
}

func TestPriceDivisor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		asset  asset.Item
		pair   currency.Pair
		expect float64
		errIs  error
	}{
		{
			name:   "standard pair uses divisor 1",
			asset:  asset.Spot,
			pair:   currency.NewBTCUSDT(),
			expect: 1,
		},
		{
			name:   "special futures pair uses scaled divisor",
			asset:  asset.USDTMarginedFutures,
			pair:   currency.NewPair(divisorCurrency, currency.USDT),
			expect: 1e6,
		},
		{
			name:   "special delivery pair uses scaled divisor",
			asset:  asset.DeliveryFutures,
			pair:   currency.NewPair(divisorCurrency, currency.USDT),
			expect: 1e6,
		},
		{
			name:  "special non futures pair returns unsupported error",
			asset: asset.Spot,
			pair:  currency.NewPair(divisorCurrency, currency.USDT),
			errIs: currency.ErrCurrencyNotSupported,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := priceDivisor(tc.asset, tc.pair)
			if tc.errIs != nil {
				require.ErrorIs(t, err, tc.errIs)
				return
			}

			require.NoError(t, err, "priceDivisor must not error")
			assert.Equal(t, tc.expect, got, "price divisor should match expected value")
		})
	}
}

func TestEarliestTime(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	pastOldest := time.Unix(1_700_000_000, 0).UTC()
	pastNewer := pastOldest.Add(2 * time.Hour)
	future := now.Add(24 * time.Hour)

	for _, tc := range []struct {
		name   string
		times  []time.Time
		expect time.Time
	}{
		{
			name:   "no times returns zero",
			expect: time.Time{},
		},
		{
			name:   "zero and future times are ignored",
			times:  []time.Time{{}, future},
			expect: time.Time{},
		},
		{
			name:   "time equal to now is ignored",
			times:  []time.Time{now},
			expect: time.Time{},
		},
		{
			name:   "single past time is returned",
			times:  []time.Time{pastNewer},
			expect: pastNewer,
		},
		{
			name:   "oldest past time is returned",
			times:  []time.Time{future, pastNewer, {}, pastOldest},
			expect: pastOldest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := earliestTime(now, tc.times...)
			assert.Equal(t, tc.expect, got, "earliest time should match expected value")
		})
	}
}

// 7610378	       143.3 ns/op	      48 B/op	       2 allocs/op
func BenchmarkMessageID(b *testing.B) {
	for b.Loop() {
		_ = e.MessageID()
	}
}

func TestFetchOrderbook(t *testing.T) {
	t.Parallel()

	availMargin, err := e.GetAvailablePairs(asset.Margin)
	require.NoError(t, err, "GetAvailablePairs must not error")
	require.NotEmpty(t, availMargin, "margin pairs must not be empty")

	enabledMargin, err := e.GetEnabledPairs(asset.Margin)
	require.NoError(t, err, "GetEnabledPairs must not error")

	marginPair := availMargin[0]
	for _, candidate := range enabledMargin {
		if availMargin.Contains(candidate, true) {
			marginPair = candidate
			break
		}
	}

	availOptions, err := e.GetAvailablePairs(asset.Options)
	require.NoError(t, err, "GetAvailablePairs must not error")
	require.NotEmpty(t, availOptions, "options pairs must not be empty")

	enabledOptions, err := e.GetEnabledPairs(asset.Options)
	require.NoError(t, err, "GetEnabledPairs must not error")

	optionsPair := availOptions[0]
	for _, candidate := range enabledOptions {
		if availOptions.Contains(candidate, true) {
			optionsPair = candidate
			break
		}
	}

	availDelivery, err := e.GetAvailablePairs(asset.DeliveryFutures)
	require.NoError(t, err, "GetAvailablePairs must not error")

	deliveryPair, err := availDelivery.GetRandomPair()
	require.NoError(t, err, "GetRandomPair must not error")

	for _, tc := range []struct {
		pair currency.Pair
		a    asset.Item
		err  error
	}{
		{pair: currency.EMPTYPAIR, a: asset.Spot, err: currency.ErrCurrencyPairEmpty},
		{pair: marginPair, a: asset.Binary, err: asset.ErrNotSupported},
		{pair: currency.NewBTCUSDT(), a: asset.Spot},
		{pair: marginPair, a: asset.Margin},
		{pair: currency.NewBTCUSDT(), a: asset.USDTMarginedFutures},
		{pair: deliveryPair, a: asset.DeliveryFutures},
		{pair: optionsPair, a: asset.Options},
	} {
		t.Run(fmt.Sprintf("%s-%s: expected err:%v", tc.pair, tc.a, tc.err), func(t *testing.T) {
			t.Parallel()
			got, err := e.fetchOrderbook(t.Context(), tc.pair, tc.a, 1)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, e.Name, got.Exchange, "Exchange name should be correct")
			assert.True(t, tc.pair.Equal(got.Pair), "Pair should be correct")
			assert.Equal(t, tc.a, got.Asset, "Asset should be correct")
			assert.LessOrEqual(t, len(got.Asks), 5, "Asks count should not exceed limit, but may be empty especially for options")
			assert.LessOrEqual(t, len(got.Bids), 5, "Bids count should not exceed limit, but may be empty especially for options")
			assert.NotZero(t, got.LastUpdated, "Last updated timestamp should be set")
			assert.NotZero(t, got.LastUpdateID, "Last update ID should be set")
			assert.NotZero(t, got.LastPushed, "Last pushed timestamp should be set")
			assert.LessOrEqual(t, got.LastUpdated, got.LastPushed, "Last updated timestamp should be before last pushed timestamp")
		})
	}
}

func TestFetchOrderbookNoSpotInstrument(t *testing.T) {
	t.Parallel()

	fakePair := currency.NewPair(currency.NewCode("ZZFAKE"), currency.USDT)
	_, err := e.fetchOrderbook(t.Context(), fakePair, asset.Margin, 1)
	require.ErrorIs(t, err, errNoSpotInstrument)
}
