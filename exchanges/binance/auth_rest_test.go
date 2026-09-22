package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func mockBinanceHTTP(t *testing.T, handler http.HandlerFunc) *Exchange {
	t.Helper()
	server := httptest.NewTestServer(t, handler)
	client := server.Client()
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	require.NoError(t, local.DisableRateLimiter(), "in-memory HTTP tests must bypass production rate delays")
	local.SkipAuthCheck = true
	local.SetCredentials(&accounts.Credentials{Key: "test-key", Secret: "test-secret"})
	require.NoError(t, local.SetHTTPClient(client), "mock client must initialise")
	for _, endpoint := range []exchange.URL{exchange.RestSpot, exchange.RestUSDTMargined, exchange.RestCoinMargined, exchange.RestFuturesSupplementary, exchange.RestOptions} {
		require.NoError(t, local.API.Endpoints.SetRunningURL(endpoint.String(), server.URL), "endpoint must use its mock server")
	}
	return local
}

func TestOfflineAPIResponseErrors(t *testing.T) {
	for _, mode := range []string{"public", "api-key", "signed"} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name, body string
				status     int
				sentinel   error
			}{
				{"mapped HTTP rejection", `{"code":-1121,"msg":"Invalid symbol."}`, 400, currency.ErrCurrencyPairEmpty},
				{"mapped successful status rejection", `{"code":-1002,"msg":"Not authorised."}`, 200, request.ErrAuthRequestFailed},
				{"quoted error code", `{"code":"-1121","msg":"Invalid symbol."}`, 200, currency.ErrCurrencyPairEmpty},
				{"invalid API key ID", `{"code":-2008,"msg":"Invalid Api-Key ID."}`, 400, request.ErrAuthRequestFailed},
				{"invalid API key format", `{"code":-2014,"msg":"API-key format invalid."}`, 200, request.ErrAuthRequestFailed},
				{"rejected API key permissions", `{"code":-2015,"msg":"Invalid API-key, IP, or permissions for action."}`, 401, request.ErrAuthRequestFailed},
				{"unknown error code", `{"code":-9999,"msg":"Failure"}`, 400, errAPIResponse},
				{"explicit failed operation", `{"success":false,"message":"Failure"}`, 200, errAPIResponse},
				{"message on success", `{"msg":"success"}`, 200, nil},
				{"quoted success code", `{"code":"200","msg":"success"}`, 200, nil},
				{"SAPI success code", `{"code":"000000","success":true}`, 200, nil},
				{"null", `null`, 200, nil},
				{"array", `[]`, 200, nil},
			} {
				t.Run(tc.name, func(t *testing.T) {
					local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
						if mode != "public" {
							assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "authenticated request should use a test key")
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(tc.status)
						_, err := w.Write([]byte(tc.body))
						assert.NoError(t, err, "mock response should write")
					})
					var response json.RawMessage
					var err error
					switch mode {
					case "public":
						err = local.SendHTTPRequest(t.Context(), exchange.RestSpot, "/api/v3/ping", request.UnAuth, &response)
					case "api-key":
						err = local.SendAPIKeyHTTPRequest(t.Context(), exchange.RestSpot, http.MethodPost, "/sapi/v1/margin/listen-key", request.UnAuth, &response)
					case "signed":
						err = local.SendAuthHTTPRequest(t.Context(), exchange.RestSpot, http.MethodGet, "/api/v3/account", nil, request.UnAuth, nil, &response)
					}
					if tc.sentinel != nil {
						assert.ErrorIs(t, err, tc.sentinel, "documented rejection should retain its sentinel")
						assert.ErrorIs(t, err, errAPIResponse, "all API failures should retain the API response sentinel")
					} else {
						assert.NoError(t, err, "successful envelopes should not be rejected")
					}
				})
			}
		})
	}
}

func TestDocumentedAuthenticatedRequests(t *testing.T) {
	data, err := os.ReadFile("testdata/documented_responses.json")
	require.NoError(t, err, "documented responses must load")
	var fixtures map[string]*documentedResponse
	require.NoError(t, json.Unmarshal(data, &fixtures), "documented fixtures must decode")
	type requestCase struct {
		fixture string
		params  url.Values
		call    func(context.Context, *Exchange) (any, error)
	}
	var current atomic.Pointer[requestCase]
	local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		tc := current.Load()
		fixture := fixtures[tc.fixture]
		assert.Equal(t, fixture.Path, r.URL.Path, "method should select its documented path")
		assert.Equal(t, fixture.Method, r.Method, "method should use its documented HTTP verb")
		assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "request should use the test API key")
		params := r.URL.Query()
		signature := params.Get("signature")
		params.Del("signature")
		mac := hmac.New(sha256.New, []byte("test-secret"))
		_, _ = mac.Write([]byte(params.Encode()))
		assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), signature, "signature should cover all transmitted parameters")
		_, err := strconv.ParseInt(params.Get("timestamp"), 10, 64)
		assert.NoError(t, err, "timestamp should be an exact millisecond integer")
		params.Del("timestamp")
		params.Del("recvWindow")
		assert.Equal(t, tc.params, params, "request should contain exactly the documented endpoint parameters")
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(fixture.Data)
		assert.NoError(t, err, "documented response should write")
	})
	for _, tc := range []requestCase{
		{"sub_account/enable_futures_for_sub_account", url.Values{"email": {"test@example.com"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.EnableFuturesSubAccount(ctx, "test@example.com")
		}},
		{"staking/subscribe_eth_staking", url.Values{"amount": {"0.123"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.SubscribeETHStaking(ctx, 0.123)
		}},
		{"margin_trading/margin_account_borrow_repay", url.Values{"asset": {"USDT"}, "amount": {"1"}, "type": {"BORROW"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.MarginAccountBorrowRepay(ctx, &MarginAccountBorrowRepayRequest{AssetName: currency.USDT, Amount: 1, LendingType: "BORROW"})
		}},
		{"derivatives_trading_portfolio_margin/margin_account_borrow", url.Values{"asset": {"USDT"}, "amount": {"1"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.MarginAccountBorrow(ctx, currency.USDT, 1)
		}},
		{"wallet/user_universal_transfer", url.Values{"asset": {"USDT"}, "amount": {"1"}, "type": {"MAIN_UMFUTURE"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.UserUniversalTransfer(ctx, &UserUniversalTransferRequest{Currency: currency.USDT, Amount: 1, TransferType: ttMainUMFuture})
		}},
		{"wallet/toggle_bnb_burn_on_spot_trade_and_margin_interest", url.Values{"spotBnbBurn": {"false"}, "interestBnbBurn": {"false"}}, func(ctx context.Context, e *Exchange) (any, error) { return e.ToggleBNBBurn(ctx, false, false) }},
		{"spot/order_amend_keep_priority", url.Values{"symbol": {"BTCUSDT"}, "orderId": {"9007199254740993"}, "newQty": {"0.5"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.AmendOrderKeepPriority(ctx, &AmendKeepPriorityRequest{Symbol: currency.NewBTCUSDT(), OrderID: 9007199254740993, NewQuantity: 0.5})
		}},
		{"derivatives_trading_portfolio_margin/query_current_um_open_algo_order", url.Values{"algoId": {"9007199254740993"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.GetUMOpenAlgoOrder(ctx, currency.EMPTYPAIR, &GetUMOpenAlgoOrderRequest{AlgoID: 9007199254740993})
		}},
		{"derivatives_trading_portfolio_margin/query_cm_conditional_order_history", url.Values{"symbol": {"BTCUSD"}, "strategyId": {"123445"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.GetAllCMConditionalOrderHistory(ctx, currency.NewBTCUSD(), "", 123445)
		}},
		{"derivatives_trading_portfolio_margin/query_current_cm_open_order", url.Values{"symbol": {"BTCUSD"}, "orderId": {"1917641"}}, func(ctx context.Context, e *Exchange) (any, error) {
			return e.GetCMOpenOrder(ctx, currency.NewBTCUSD(), "", "1917641")
		}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			fixture, ok := fixtures[tc.fixture]
			require.True(t, ok, "request must reference a documented fixture")
			current.Store(&tc)
			response, err := tc.call(t.Context(), local)
			require.NoError(t, err, "documented response must decode through its endpoint")
			require.NotNil(t, response, "endpoint must return its response")
			if id, ok := response.(string); ok {
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(fixture.Data, &fields), "transaction response must decode")
				assert.Equal(t, string(fields["tranId"]), id, "endpoint should return the response transaction ID")
			} else {
				assertResponseFields(t, fixture.Data, reflect.TypeOf(response), tc.fixture)
			}
		})
	}
}

// The 2024-11-21 wallet changelog defines this compatibility response.
// https://developers.binance.com/en/docs/products/wallet/change-log
func TestRetiredBUSDConversionResponse(t *testing.T) {
	local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sapi/v1/asset/convert-transfer", r.URL.Path, "compatibility call should retain its path")
		_, err := w.Write([]byte(`{"tranId":null,"status":"F","response":"No longer supported"}`))
		assert.NoError(t, err, "mock compatibility response should write")
	})
	response, err := local.ConvertBUSD(t.Context(), &ConvertBUSDRequest{ClientTransactionID: "test-conversion", AssetCcy: currency.BUSD, TargetAsset: currency.USDT, Amount: 1})
	require.NoError(t, err, "documented compatibility response must decode")
	assert.Empty(t, response.TransactionID, "retired conversion should retain the null transaction ID")
	assert.Equal(t, "F", response.Status, "retired conversion should preserve failure status")
	assert.Equal(t, "No longer supported", response.Response, "retired conversion should preserve the reason")
}
