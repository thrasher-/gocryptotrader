package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

func TestDocumentedFuturesModificationRequests(t *testing.T) {
	raw, err := os.ReadFile("testdata/documented_responses.json")
	require.NoError(t, err, "documented fixtures must load")
	var fixtures map[string]*documentedResponse
	require.NoError(t, json.Unmarshal(raw, &fixtures), "documented fixtures must decode")
	for _, tc := range []struct {
		fixture, symbol string
		call            func(*Exchange, *FuturesOrderModificationRequest) (any, error)
	}{
		{"derivatives_trading_coin_futures/modify_order", "BTCUSD_PERP", func(e *Exchange, arg *FuturesOrderModificationRequest) (any, error) {
			return e.ModifyCFuturesOrder(t.Context(), arg)
		}},
		{"derivatives_trading_portfolio_margin/modify_cm_order", "BTCUSD_PERP", func(e *Exchange, arg *FuturesOrderModificationRequest) (any, error) {
			return e.ModifyPMCMOrder(t.Context(), arg)
		}},
		{"derivatives_trading_portfolio_margin/modify_um_order", "BTCUSDT", func(e *Exchange, arg *FuturesOrderModificationRequest) (any, error) {
			return e.ModifyPMUMOrder(t.Context(), arg)
		}},
		{"derivatives_trading_coin_futures/modify_multiple_orders", "BTCUSD_PERP", func(e *Exchange, arg *FuturesOrderModificationRequest) (any, error) {
			return e.ModifyCFuturesOrders(t.Context(), &CFuturesOrderModificationsRequest{BatchOrders: []*FuturesOrderModificationRequest{arg}, RecvWindow: 7000})
		}},
	} {
		for _, priceMatch := range []bool{false, true} {
			t.Run(tc.fixture+"/"+map[bool]string{false: "price", true: "priceMatch"}[priceMatch], func(t *testing.T) {
				fixture := fixtures[tc.fixture]
				require.NotNil(t, fixture, "endpoint fixture must exist")
				arg := &FuturesOrderModificationRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), Side: order.Buy, OrderID: 9007199254740993, OriginalClientOrderID: "client-order", Quantity: 2, Price: 80000, ModifyID: 9007199254740995, RecvWindow: 6000}
				if tc.symbol == "BTCUSDT" {
					arg.Symbol = currency.NewBTCUSDT()
				}
				want := url.Values{"symbol": {tc.symbol}, "side": {"BUY"}, "orderId": {"9007199254740993"}, "origClientOrderId": {"client-order"}, "quantity": {"2"}, "price": {"80000"}, "modifyId": {"9007199254740995"}, "recvWindow": {"6000"}}
				if priceMatch {
					arg.Price = 0
					arg.PriceMatch = "OPPONENT"
					want.Del("price")
					want.Set("priceMatch", "OPPONENT")
				}
				before := *arg
				local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, fixture.Method, r.Method, "method should follow documentation")
					assert.Equal(t, fixture.Path, r.URL.Path, "path should follow documentation")
					params := r.URL.Query()
					sig := params.Get("signature")
					params.Del("signature")
					mac := hmac.New(sha256.New, []byte("test-secret"))
					_, _ = mac.Write([]byte(params.Encode()))
					assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), sig, "signature should cover every parameter")
					assert.NotEmpty(t, params.Get("timestamp"), "signed request should include its timestamp")
					params.Del("timestamp")
					if fixture.Path == "/dapi/v1/batchOrders" {
						assert.Equal(t, "7000", params.Get("recvWindow"), "batch receive window should be independent of item parameters")
						var orders []map[string]json.RawMessage
						if !assert.NoError(t, json.Unmarshal([]byte(params.Get("batchOrders")), &orders), "batch JSON should decode") {
							return
						}
						if !assert.Len(t, orders, 1, "batch should contain one order") {
							return
						}
						actual := url.Values{}
						for key, raw := range orders[0] {
							value := string(raw)
							if raw[0] == '"' {
								if !assert.NoError(t, json.Unmarshal(raw, &value), "string should decode") {
									return
								}
							}
							actual.Set(key, value)
						}
						assert.Equal(t, want, actual, "batch should retain all documented item parameters")
						params.Del("batchOrders")
						params.Del("recvWindow")
						assert.Empty(t, params, "batch should contain no unintended parameters")
					} else {
						assert.Equal(t, want, params, "all amendment parameters should match documentation")
					}
					_, err := w.Write(fixture.Data)
					assert.NoError(t, err, "documented response should write")
				})
				response, err := tc.call(local, arg)
				require.NoError(t, err, "documented response must decode")
				assert.Equal(t, before, *arg, "formatting should preserve caller input")
				assertResponseFields(t, fixture.Data, reflect.TypeOf(response), tc.fixture)
			})
		}
	}
	for _, enabled := range []bool{false, true} {
		local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method, "position mode should use POST")
			assert.Equal(t, "/dapi/v1/positionSide/dual", r.URL.Path, "position mode should use the COIN-M route")
			assert.Equal(t, map[bool]string{false: "false", true: "true"}[enabled], r.URL.Query().Get("dualSidePosition"), "both position modes should be explicit")
			assert.Equal(t, "6000", r.URL.Query().Get("recvWindow"), "position mode should retain its receive window")
			_, err := w.Write(fixtures["derivatives_trading_coin_futures/change_position_mode"].Data)
			assert.NoError(t, err, "fixture should write")
		})
		response, err := local.ChangeCFuturesPositionMode(t.Context(), &FuturesPositionModeRequest{DualSidePosition: enabled, RecvWindow: 6000})
		require.NoError(t, err, "position-mode response must decode")
		assert.EqualValues(t, 200, response.Code, "position-mode response should indicate success")
	}
}

func TestFuturesOrderModificationValidation(t *testing.T) {
	valid := FuturesOrderModificationRequest{Symbol: currency.NewBTCUSDT(), Side: order.Buy, OrderID: 1, Quantity: 2, Price: 80000}
	for _, portfolio := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			change func(*FuturesOrderModificationRequest)
			want   error
		}{
			{"symbol", func(a *FuturesOrderModificationRequest) { a.Symbol = currency.EMPTYPAIR }, currency.ErrCurrencyPairEmpty},
			{"ID", func(a *FuturesOrderModificationRequest) { a.OrderID = 0 }, order.ErrOrderIDNotSet},
			{"side", func(a *FuturesOrderModificationRequest) { a.Side = order.UnknownSide }, order.ErrSideIsInvalid},
			{"quantity", func(a *FuturesOrderModificationRequest) { a.Quantity = -1 }, limits.ErrAmountBelowMin},
			{"price", func(a *FuturesOrderModificationRequest) { a.Price = -1 }, limits.ErrPriceBelowMin},
			{"price conflict", func(a *FuturesOrderModificationRequest) { a.PriceMatch = "OPPONENT" }, errInvalidOrderQueryCombination},
		} {
			t.Run(tc.name, func(t *testing.T) {
				arg := valid
				tc.change(&arg)
				assert.ErrorIs(t, validateFuturesOrderModification(&arg, portfolio), tc.want, "invalid amendment should retain its sentinel")
			})
		}
		assert.ErrorIs(t, validateFuturesOrderModification(nil, portfolio), common.ErrNilPointer, "nil amendment should fail offline")
	}
	arg := valid
	arg.Quantity = 0
	assert.ErrorIs(t, validateFuturesOrderModification(&arg, true), limits.ErrAmountBelowMin, "portfolio quantity should be required")
	arg = valid
	arg.Price = 0
	assert.ErrorIs(t, validateFuturesOrderModification(&arg, true), limits.ErrPriceBelowMin, "portfolio pricing should be required")
	arg.Quantity = 0
	assert.ErrorIs(t, validateFuturesOrderModification(&arg, false), common.ErrEmptyParams, "empty amendment should fail offline")
	local := new(Exchange)
	_, err := local.ModifyCFuturesOrder(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "single amendment should validate before transport")
	_, err = local.ModifyPMCMOrder(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "CM portfolio amendment should validate before transport")
	_, err = local.ModifyPMUMOrder(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "UM portfolio amendment should validate before transport")
	_, err = local.ChangeCFuturesPositionMode(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "position mode should reject nil")
	_, err = local.ModifyCFuturesOrders(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "batch should reject nil")
	_, err = local.ModifyCFuturesOrders(t.Context(), &CFuturesOrderModificationsRequest{})
	assert.ErrorIs(t, err, common.ErrEmptyParams, "batch should require orders")
	_, err = local.ModifyCFuturesOrders(t.Context(), &CFuturesOrderModificationsRequest{BatchOrders: make([]*FuturesOrderModificationRequest, 6)})
	assert.ErrorIs(t, err, errLimitNumberRequired, "batch should allow at most five orders")
	_, err = local.ModifyCFuturesOrders(t.Context(), &CFuturesOrderModificationsRequest{BatchOrders: []*FuturesOrderModificationRequest{nil}})
	assert.ErrorIs(t, err, common.ErrNilPointer, "batch should validate each order")
}
