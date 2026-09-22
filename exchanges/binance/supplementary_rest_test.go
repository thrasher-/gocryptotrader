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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

// The parameter fixtures follow the official parameter tables. Values are synthetic;
// these tests establish serialisation and decoding, not live account acceptance.
func TestDocumentedCurrentRESTRequests(t *testing.T) {
	data, err := os.ReadFile("testdata/documented_current_requests.json")
	require.NoError(t, err, "documented request cases must load")
	var inputs map[string]struct {
		Params json.RawMessage `json:"params"`
	}
	require.NoError(t, json.Unmarshal(data, &inputs), "documented inputs must decode")
	data, err = os.ReadFile("testdata/documented_responses.json")
	require.NoError(t, err, "documented responses must load")
	var responses map[string]*documentedResponse
	require.NoError(t, json.Unmarshal(data, &responses), "documented responses must decode")
	for _, tc := range []struct {
		fixture          string
		signed, required bool
		decode           func(*testing.T, []byte) any
		call             func(*Exchange, any) (any, error)
		argType          reflect.Type
		validation       []struct {
			field string
			err   error
		}
	}{
		{"derivatives_trading_portfolio_margin_pro/delete_margin_call_level", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(DeletePortfolioMarginCallLevelRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.DeletePortfolioMarginCallLevel(t.Context(), testRequestAs[*DeletePortfolioMarginCallLevelRequest](t, arg))
		}, reflect.TypeFor[*DeletePortfolioMarginCallLevelRequest](), []struct {
			field string
			err   error
		}{}},
		{"derivatives_trading_portfolio_margin_pro/get_delta_mode_status", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetPortfolioDeltaModeRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetPortfolioDeltaMode(t.Context(), testRequestAs[*GetPortfolioDeltaModeRequest](t, arg))
		}, reflect.TypeFor[*GetPortfolioDeltaModeRequest](), []struct {
			field string
			err   error
		}{}},
		{"derivatives_trading_portfolio_margin_pro/get_margin_call_level", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetPortfolioMarginCallLevelRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetPortfolioMarginCallLevel(t.Context(), testRequestAs[*GetPortfolioMarginCallLevelRequest](t, arg))
		}, reflect.TypeFor[*GetPortfolioMarginCallLevelRequest](), []struct {
			field string
			err   error
		}{}},
		{"derivatives_trading_portfolio_margin_pro/get_portfolio_margin_pro_account_balance", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetPortfolioMarginProBalancesRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetPortfolioMarginProBalances(t.Context(), testRequestAs[*GetPortfolioMarginProBalancesRequest](t, arg))
		}, reflect.TypeFor[*GetPortfolioMarginProBalancesRequest](), []struct {
			field string
			err   error
		}{}},
		{"derivatives_trading_portfolio_margin_pro/get_portfolio_margin_pro_span_account_info", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetPortfolioMarginProSPANAccountRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetPortfolioMarginProSPANAccount(t.Context(), testRequestAs[*GetPortfolioMarginProSPANAccountRequest](t, arg))
		}, reflect.TypeFor[*GetPortfolioMarginProSPANAccountRequest](), []struct {
			field string
			err   error
		}{}},
		{"derivatives_trading_portfolio_margin_pro/get_transferable_earn_asset_balance_for_portfolio_margin", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetPortfolioTransferableEarnBalanceRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetPortfolioTransferableEarnBalance(t.Context(), testRequestAs[*GetPortfolioTransferableEarnBalanceRequest](t, arg))
		}, reflect.TypeFor[*GetPortfolioTransferableEarnBalanceRequest](), []struct {
			field string
			err   error
		}{{"asset", currency.ErrCurrencyCodeEmpty}, {"transferType", errTransferTypeRequired}}},
		{"derivatives_trading_portfolio_margin_pro/query_portfolio_margin_pro_bankruptcy_loan_repay_history", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetPortfolioMarginProLoanRepaymentsRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			arg.StartTime = time.UnixMilli(1750500000000)
			arg.EndTime = time.UnixMilli(1750500060000)
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetPortfolioMarginProLoanRepayments(t.Context(), testRequestAs[*GetPortfolioMarginProLoanRepaymentsRequest](t, arg))
		}, reflect.TypeFor[*GetPortfolioMarginProLoanRepaymentsRequest](), []struct {
			field string
			err   error
		}{}},
		{"derivatives_trading_portfolio_margin_pro/set_margin_call_level", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(SetPortfolioMarginCallLevelRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.SetPortfolioMarginCallLevel(t.Context(), testRequestAs[*SetPortfolioMarginCallLevelRequest](t, arg))
		}, reflect.TypeFor[*SetPortfolioMarginCallLevelRequest](), []struct {
			field string
			err   error
		}{{"marginCallLevel", errMarginCallValueRequired}}},
		{"derivatives_trading_portfolio_margin_pro/switch_delta_mode", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(SetPortfolioDeltaModeRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.SetPortfolioDeltaMode(t.Context(), testRequestAs[*SetPortfolioDeltaModeRequest](t, arg))
		}, reflect.TypeFor[*SetPortfolioDeltaModeRequest](), []struct {
			field string
			err   error
		}{}},
		{"derivatives_trading_portfolio_margin_pro/transfer_ldusdt_rwusd_for_portfolio_margin", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(TransferPortfolioEarnAssetsRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.TransferPortfolioEarnAssets(t.Context(), testRequestAs[*TransferPortfolioEarnAssetsRequest](t, arg))
		}, reflect.TypeFor[*TransferPortfolioEarnAssetsRequest](), []struct {
			field string
			err   error
		}{{"asset", currency.ErrCurrencyCodeEmpty}, {"transferType", errTransferTypeRequired}, {"amount", limits.ErrAmountBelowMin}}},
		{"derivatives_trading_portfolio_margin_pro/portfolio_margin_pro_tiered_collateral_rate", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetPortfolioTieredCollateralRatesRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetPortfolioTieredCollateralRates(t.Context(), testRequestAs[*GetPortfolioTieredCollateralRatesRequest](t, arg))
		}, reflect.TypeFor[*GetPortfolioTieredCollateralRatesRequest](), []struct {
			field string
			err   error
		}{}},
		{"margin_trading/query_liquidation_loan", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetMarginLiquidationLoanRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetMarginLiquidationLoan(t.Context(), testRequestAs[*GetMarginLiquidationLoanRequest](t, arg))
		}, reflect.TypeFor[*GetMarginLiquidationLoanRequest](), []struct {
			field string
			err   error
		}{}},
		{"wallet/get_spot_asset_tags", false, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetSpotAssetTagsRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetSpotAssetTags(t.Context(), testRequestAs[*GetSpotAssetTagsRequest](t, arg))
		}, reflect.TypeFor[*GetSpotAssetTagsRequest](), []struct {
			field string
			err   error
		}{}},
		{"wallet/broker_withdraw", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(BrokerTravelRuleWithdrawRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.BrokerTravelRuleWithdraw(t.Context(), testRequestAs[*BrokerTravelRuleWithdrawRequest](t, arg))
		}, reflect.TypeFor[*BrokerTravelRuleWithdrawRequest](), []struct {
			field string
			err   error
		}{{"address", errAddressRequired}, {"coin", currency.ErrCurrencyCodeEmpty}, {"amount", limits.ErrAmountBelowMin}, {"withdrawOrderId", order.ErrOrderIDNotSet}, {"questionnaire", errQuestionnaireRequired}, {"originatorPii", errAccountRequired}}},
		{"wallet/deposit_history_v2", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetTravelRuleDepositHistoryV2Request)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			arg.StartTime = time.UnixMilli(1750500000000)
			arg.EndTime = time.UnixMilli(1750500060000)
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetTravelRuleDepositHistoryV2(t.Context(), testRequestAs[*GetTravelRuleDepositHistoryV2Request](t, arg))
		}, reflect.TypeFor[*GetTravelRuleDepositHistoryV2Request](), []struct {
			field string
			err   error
		}{}},
		{"wallet/fetch_address_verification_list", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetAddressVerificationsRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetAddressVerifications(t.Context(), testRequestAs[*GetAddressVerificationsRequest](t, arg))
		}, reflect.TypeFor[*GetAddressVerificationsRequest](), []struct {
			field string
			err   error
		}{}},
		{"wallet/get_country_list", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetTravelRuleCountriesRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetTravelRuleCountries(t.Context(), testRequestAs[*GetTravelRuleCountriesRequest](t, arg))
		}, reflect.TypeFor[*GetTravelRuleCountriesRequest](), []struct {
			field string
			err   error
		}{}},
		{"wallet/get_region_list", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetTravelRuleRegionsRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetTravelRuleRegions(t.Context(), testRequestAs[*GetTravelRuleRegionsRequest](t, arg))
		}, reflect.TypeFor[*GetTravelRuleRegionsRequest](), []struct {
			field string
			err   error
		}{{"countryCode", errCodeRequired}}},
		{"wallet/submit_deposit_questionnaire", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(SubmitBrokerDepositQuestionnaireRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.SubmitBrokerDepositQuestionnaire(t.Context(), testRequestAs[*SubmitBrokerDepositQuestionnaireRequest](t, arg))
		}, reflect.TypeFor[*SubmitBrokerDepositQuestionnaireRequest](), []struct {
			field string
			err   error
		}{{"subAccountId", errSubAccountIDMissing}, {"depositId", errTransactionIDRequired}, {"questionnaire", errQuestionnaireRequired}, {"beneficiaryPii", errAccountRequired}}},
		{"wallet/submit_deposit_questionnaire_v2", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(SubmitDepositQuestionnaireV2Request)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.SubmitDepositQuestionnaireV2(t.Context(), testRequestAs[*SubmitDepositQuestionnaireV2Request](t, arg))
		}, reflect.TypeFor[*SubmitDepositQuestionnaireV2Request](), []struct {
			field string
			err   error
		}{{"depositId", errTransactionIDRequired}, {"questionnaire", errQuestionnaireRequired}}},
		{"wallet/withdraw_history_v1", true, false, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(GetTravelRuleWithdrawalHistoryRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			arg.StartTime = time.UnixMilli(1750500000000)
			arg.EndTime = time.UnixMilli(1750500060000)
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.GetTravelRuleWithdrawalHistory(t.Context(), testRequestAs[*GetTravelRuleWithdrawalHistoryRequest](t, arg))
		}, reflect.TypeFor[*GetTravelRuleWithdrawalHistoryRequest](), []struct {
			field string
			err   error
		}{}},
		{"wallet/withdraw_travel_rule", true, true, func(t *testing.T, data []byte) any {
			t.Helper()
			arg := new(TravelRuleWithdrawRequest)
			require.NoError(t, json.Unmarshal(data, arg), "documented inputs must decode")
			return arg
		}, func(e *Exchange, arg any) (any, error) {
			return e.TravelRuleWithdraw(t.Context(), testRequestAs[*TravelRuleWithdrawRequest](t, arg))
		}, reflect.TypeFor[*TravelRuleWithdrawRequest](), []struct {
			field string
			err   error
		}{{"coin", currency.ErrCurrencyCodeEmpty}, {"address", errAddressRequired}, {"amount", limits.ErrAmountBelowMin}, {"questionnaire", errQuestionnaireRequired}}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			var expected map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(inputs[tc.fixture].Params, &expected), "fixture parameters must decode")
			fixture, ok := responses[tc.fixture]
			require.True(t, ok, "endpoint must have a documented response")
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, fixture.Method, r.Method, "request should use the documented HTTP verb")
				assert.Equal(t, fixture.Path, r.URL.Path, "request should use the documented endpoint")
				params := r.URL.Query()
				switch {
				case tc.signed:
					assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "request should use a placeholder API key")
					sig := params.Get("signature")
					params.Del("signature")
					mac := hmac.New(sha256.New, []byte("test-secret"))
					_, _ = mac.Write([]byte(params.Encode()))
					assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), sig, "signature should cover all query parameters")
					assert.NotEmpty(t, params.Get("timestamp"), "signed request should include a timestamp")
					params.Del("timestamp")
					if _, ok := expected["recvWindow"]; !ok {
						assert.Equal(t, "5000", params.Get("recvWindow"), "omitted receive window should use the default")
						params.Del("recvWindow")
					}
				case tc.fixture == "wallet/get_spot_asset_tags":
					assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "API-key-only market data should carry its key")
				default:
					assert.Empty(t, r.Header.Get("X-MBX-APIKEY"), "public request should carry no credentials")
				}
				want := url.Values{}
				for k, v := range expected {
					value := string(v)
					if len(v) > 0 && v[0] == '"' {
						if !assert.NoError(t, json.Unmarshal(v, &value), "string input should decode") {
							return
						}
					}
					if len(v) > 0 && v[0] == '{' {
						var compact any
						if !assert.NoError(t, json.Unmarshal(v, &compact), "object should decode") {
							return
						}
						body, err := json.Marshal(compact)
						if !assert.NoError(t, err, "object should serialise") {
							return
						}
						value = string(body)
					}
					want.Set(k, value)
				}
				assert.Equal(t, want, params, "all documented endpoint parameters should be transmitted exactly")
				_, err := w.Write(fixture.Data)
				assert.NoError(t, err, "fixture should write")
			})
			data := inputs[tc.fixture].Params
			response, err := tc.call(local, tc.decode(t, data))
			require.NoError(t, err, "documented response must decode through its endpoint")
			assertResponseFields(t, fixture.Data, reflect.TypeOf(response), tc.fixture)
			if tc.required {
				_, err = tc.call(local, reflect.Zero(tc.argType).Interface())
				assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail before HTTP")
			}
			for _, v := range tc.validation {
				t.Run(v.field, func(t *testing.T) {
					var params map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(data, &params), "valid input must decode")
					delete(params, v.field)
					invalid, err := json.Marshal(params)
					require.NoError(t, err, "invalid input must serialise")
					_, err = tc.call(local, tc.decode(t, invalid))
					assert.ErrorIs(t, err, v.err, "missing required input should retain its sentinel")
				})
			}
		})
	}
}
