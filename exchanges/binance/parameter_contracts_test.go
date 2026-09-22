package binance

import (
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

func TestUnsupportedRequestParameters(t *testing.T) {
	local := mockBinanceHTTP(t, func(http.ResponseWriter, *http.Request) {
		t.Error("unsupported parameters should fail before sending a request")
	})
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"option open-order limit", func() error {
			_, err := local.GetCurrentOpenOptionsOrders(t.Context(), &GetCurrentOpenOptionsOrdersRequest{Limit: 10})
			return err
		}},
		{"margin interest transaction ID", func() error {
			_, err := local.GetMarginBorrowOrLoanInterestHistory(t.Context(), &GetMarginBorrowOrLoanInterestHistoryRequest{TransactionID: 1})
			return err
		}},
		{"margin prevented-match limit", func() error {
			_, err := local.GetMarginPreventedMatches(t.Context(), &GetMarginPreventedMatchesRequest{Limit: 10})
			return err
		}},
		{"spot OCO margin borrowing", func() error {
			_, err := local.NewOCOOrder(t.Context(), &OCOOrderRequest{SideEffectType: "MARGIN_BUY"})
			return err
		}},
		{"gift-card discount", func() error {
			_, err := local.CreateDualTokenGiftCard(t.Context(), currency.USDT, currency.BNB, 10, 0.1)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, tc.call(), errUnsupportedParameter, "unsupported argument should retain its sentinel")
		})
	}
	for _, value := range []bool{false, true} {
		_, err := local.NewOrderUsingSOR(t.Context(), &SOROrderRequest{ComputeCommissionRates: new(value)})
		assert.ErrorIs(t, err, errUnsupportedParameter, "test-only parameter should fail for either explicit boolean value")
	}
	for _, arg := range []*OCOOrderRequest{
		{LimitStrategyID: "1"},
		{LimitStrategyType: "1000000"},
		{StopStrategyID: 1},
		{StopStrategyType: 1000000},
		{TrailingDelta: 10},
		{SelfTradePreventionMode: "EXPIRE_MAKER"},
	} {
		_, err := local.MarginAccountNewOCO(t.Context(), arg)
		assert.ErrorIs(t, err, errUnsupportedParameter, "spot-only OCO parameter should fail for portfolio margin")
	}
	for _, arg := range []*UMOrderRequest{
		{GoodTillDate: time.UnixMilli(1770000000000)},
		{GoodTillDateTimestamp: 1770000000000},
		{SelfTradePreventionMode: "EXPIRE_MAKER"},
	} {
		_, err := local.NewCMOrder(t.Context(), arg)
		assert.ErrorIs(t, err, errUnsupportedParameter, "UM-only order parameter should fail for COIN-M")
	}
	for _, arg := range []*ConditionalOrderRequest{
		{GoodTillDate: time.UnixMilli(1770000000000)},
		{GoodTillDateTimestamp: 1770000000000},
		{SelfTradePreventionMode: "EXPIRE_MAKER"},
	} {
		_, err := local.NewCMConditionalOrder(t.Context(), arg)
		assert.ErrorIs(t, err, errUnsupportedParameter, "UM-only conditional parameter should fail for COIN-M")
	}
	for _, arg := range []*UFuturesNewOrderRequest{
		{ActivationPrice: 750},
		{CallbackRate: 0.3},
		{ClosePosition: "false"},
		{PriceProtect: "FALSE"},
		{StopPrice: 750},
		{WorkingType: "MARK_PRICE"},
	} {
		arg.Symbol = currency.NewBTCUSDT()
		arg.OrderType = "LIMIT"
		_, err := local.UFuturesNewOrder(t.Context(), arg)
		assert.ErrorIs(t, err, errUnsupportedParameter, "algo parameter should fail at the ordinary USD-M order endpoint")
	}
	_, err := local.UFuturesNewOrder(t.Context(), &UFuturesNewOrderRequest{Symbol: currency.NewBTCUSDT(), OrderType: "STOP_MARKET"})
	assert.ErrorIs(t, err, order.ErrUnsupportedOrderType, "conditional order should require the algo endpoint")
}

func TestDocumentedRequestParameterSeparation(t *testing.T) {
	raw, err := os.ReadFile("testdata/documented_responses.json")
	require.NoError(t, err, "documented fixtures must load")
	var fixtures map[string]*documentedResponse
	require.NoError(t, json.Unmarshal(raw, &fixtures), "documented fixtures must decode")
	for _, tc := range []struct {
		fixture string
		want    url.Values
		call    func(*Exchange) (any, error)
	}{
		{"gift_card/create_a_dual_token_gift_card", url.Values{"baseToken": {"USDT"}, "faceToken": {"BNB"}, "baseTokenAmount": {"10"}}, func(e *Exchange) (any, error) {
			return e.CreateDualTokenGiftCard(t.Context(), currency.USDT, currency.BNB, 10, 0)
		}},
		{"mining/hashrate_resale_detail", url.Values{"configId": {"168"}, "pageIndex": {"1"}, "pageSize": {"10"}}, func(e *Exchange) (any, error) {
			return e.GetHashRateRescaleDetail(t.Context(), "168", "", 1, 10)
		}},
		{"spot/sor_order_test", url.Values{"symbol": {"BTCUSDT"}, "side": {"BUY"}, "type": {"LIMIT"}, "quantity": {"1"}, "price": {"750"}, "timeInForce": {"GTC"}, "computeCommissionRates": {"true"}}, func(e *Exchange) (any, error) {
			return e.NewOrderUsingSORTest(t.Context(), &SOROrderRequest{Symbol: currency.NewBTCUSDT(), Side: "BUY", OrderType: "LIMIT", Quantity: 1, Price: 750, TimeInForce: "GTC", ComputeCommissionRates: new(true)})
		}},
		{"derivatives_trading_portfolio_margin/new_um_order", url.Values{"symbol": {"BTCUSDT"}, "side": {"BUY"}, "type": {"LIMIT"}, "quantity": {"1"}, "priceMatch": {"OPPONENT"}, "timeInForce": {"GTC"}}, func(e *Exchange) (any, error) {
			return e.NewUMOrder(t.Context(), &UMOrderRequest{Symbol: currency.NewBTCUSDT(), Side: "BUY", OrderType: "LIMIT", Quantity: 1, PriceMatch: "OPPONENT", TimeInForce: "GTC"})
		}},
		{"derivatives_trading_portfolio_margin/new_cm_order", url.Values{"symbol": {"BTCUSD_PERP"}, "side": {"BUY"}, "type": {"LIMIT"}, "quantity": {"1"}, "priceMatch": {"OPPONENT"}, "timeInForce": {"GTC"}}, func(e *Exchange) (any, error) {
			return e.NewCMOrder(t.Context(), &UMOrderRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), Side: "BUY", OrderType: "LIMIT", Quantity: 1, PriceMatch: "OPPONENT", TimeInForce: "GTC"})
		}},
		{"simple_earn/subscribe_flexible_product", url.Values{"productId": {"BTC001"}, "sourceAccount": {"SPOT"}, "amount": {"1"}, "autoSubscribe": {"false"}}, func(e *Exchange) (any, error) {
			return e.SubscribeToFlexibleProducts(t.Context(), "BTC001", "SPOT", 1, false)
		}},
		{"simple_earn/subscribe_locked_product", url.Values{"projectId": {"project-1"}, "sourceAccount": {"SPOT"}, "amount": {"1"}, "autoSubscribe": {"false"}}, func(e *Exchange) (any, error) {
			return e.SubscribeToLockedProducts(t.Context(), "project-1", "SPOT", 1, false)
		}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			fixture := fixtures[tc.fixture]
			require.NotNil(t, fixture, "documented response must exist")
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, fixture.Method, r.Method, "request method should match the contract")
				assert.Equal(t, fixture.Path, r.URL.Path, "request path should match the contract")
				params := r.URL.Query()
				params.Del("timestamp")
				params.Del("signature")
				params.Del("recvWindow")
				assert.Equal(t, tc.want, params, "request should contain exactly the endpoint's parameters")
				_, err := w.Write(fixture.Data)
				assert.NoError(t, err, "documented response should write")
			})
			_, err := tc.call(local)
			assert.NoError(t, err, "documented request should succeed offline")
		})
	}
}

func TestMarginListenKeyClosureParameters(t *testing.T) {
	local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method, "margin stream closure should use DELETE")
		assert.Equal(t, "/sapi/v1/margin/listen-key", r.URL.Path, "margin stream closure should use its documented route")
		assert.Empty(t, r.URL.RawQuery, "closure should use the API key without query parameters")
		assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "closure should identify the account by its API key")
		w.WriteHeader(http.StatusOK)
	})
	assert.NoError(t, local.CloseMarginListenKey(t.Context()), "no-argument closure should accept the documented empty response")
	assert.NoError(t, local.CloseMarginListenKey(t.Context(), "legacy-key"), "legacy calls should omit the obsolete query parameter")
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"API rejection", `{"code":-2015,"msg":"Invalid API-key"}`, http.StatusOK, request.ErrAuthRequestFailed},
		{"HTTP rejection", "", http.StatusBadRequest, request.ErrBadStatus},
		{"malformed JSON", `{`, http.StatusOK, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, err := w.Write([]byte(tc.body))
				assert.NoError(t, err, "error response should write")
			})
			err := local.CloseMarginListenKey(t.Context())
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want, "empty-response support should preserve API and HTTP errors")
			} else {
				assert.Error(t, err, "malformed nonempty JSON should remain an error")
			}
		})
	}
}

func TestListenKeyEmptyResponses(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		call               func(*Exchange) error
	}{
		{"margin renewal", http.MethodPut, "/sapi/v1/margin/listen-key", func(e *Exchange) error {
			return e.KeepMarginListenKeyAlive(t.Context(), "test-listen-key")
		}},
		{"USD-M closure", http.MethodDelete, "/fapi/v1/listenKey", func(e *Exchange) error {
			return e.CloseUFuturesListenKey(t.Context())
		}},
		{"COIN-M closure", http.MethodDelete, "/dapi/v1/listenKey", func(e *Exchange) error {
			return e.CloseCFuturesListenKey(t.Context())
		}},
		{"portfolio margin closure", http.MethodDelete, "/papi/v1/listenKey", func(e *Exchange) error {
			return e.ClosePortfolioMarginListenKey(t.Context())
		}},
		{"options renewal", http.MethodPut, "/eapi/v1/listenKey", func(e *Exchange) error {
			return e.KeepOptionsListenKeyAlive(t.Context())
		}},
		{"options closure", http.MethodDelete, "/eapi/v1/listenKey", func(e *Exchange) error {
			return e.CloseOptionsListenKey(t.Context())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.method, r.Method, "listen-key method should match the contract")
				assert.Equal(t, tc.path, r.URL.Path, "listen-key route should match the contract")
				assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "listen-key operation should use the test API key")
				w.WriteHeader(http.StatusOK)
			})
			assert.NoError(t, tc.call(local), "void listen-key operation should accept an empty success body")
		})
	}
}
