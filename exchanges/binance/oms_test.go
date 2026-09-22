package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

func TestDocumentedOMSRequests(t *testing.T) {
	raw, err := os.ReadFile("testdata/documented_oms.json")
	require.NoError(t, err, "OMS fixtures must load")
	var fixtures map[string]*documentedResponse
	require.NoError(t, json.Unmarshal(raw, &fixtures), "OMS fixtures must decode")
	start, end := time.UnixMilli(1750500000000), time.UnixMilli(1750500060000)
	for _, tc := range []struct {
		name   string
		params url.Values
		call   func(*Exchange) (any, error)
	}{
		{"GET /sapi/v1/apiReferral/userCustomization", url.Values{"apiAgentCode": {"broker"}}, func(e *Exchange) (any, error) { return e.GetSpotUsersCustomisedID(t.Context(), "broker") }},
		{"POST /sapi/v1/apiReferral/userCustomization", url.Values{"customerId": {"customer"}, "apiAgentCode": {"broker"}}, func(e *Exchange) (any, error) { return e.CustomiseSpotOwnClientID(t.Context(), "customer", "broker") }},
		{"GET /sapi/v1/apiReferral/customization", url.Values{"customerId": {"customer"}}, func(e *Exchange) (any, error) { return e.GetSpotClientEmailCustomisedID(t.Context(), "customer", "") }},
		{"POST /sapi/v1/apiReferral/customization", url.Values{"customerId": {"customer"}, "email": {"test@example.com"}}, func(e *Exchange) (any, error) {
			return e.CustomiseSpotPartnerClientID(t.Context(), "customer", "test@example.com")
		}},
		{"GET /sapi/v1/apiReferral/ifNewUser", url.Values{"apiAgentCode": {"broker"}}, func(e *Exchange) (any, error) { return e.GetSpotInfoAboutIfUserIsNew(t.Context(), "broker") }},
		{"GET /sapi/v1/apiReferral/kickback/recentRecord", url.Values{"startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "limit": {"5"}}, func(e *Exchange) (any, error) { return e.GetSpotOwnRebateRecentRecords(t.Context(), start, end, 5) }},
		{"GET /sapi/v1/apiReferral/rebate/recentRecord", url.Values{"startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "limit": {"5"}, "customerId": {"customer"}}, func(e *Exchange) (any, error) {
			return e.GetSpotOthersRebateRecentRecord(t.Context(), "customer", start, end, 5)
		}},
		{"GET /papi/v1/apiReferral/userCustomization", url.Values{"brokerId": {"broker"}}, func(e *Exchange) (any, error) { return e.GetUsersCustomiseIDs(t.Context(), "broker") }},
		{"POST /papi/v1/apiReferral/userCustomization", url.Values{"customerId": {"customer"}, "brokerId": {"broker"}}, func(e *Exchange) (any, error) {
			return e.CustomiseIDForClientToReferredUser(t.Context(), "customer", "broker")
		}},
		{"GET /fapi/v1/apiReferral/userCustomization", url.Values{"brokerId": {"broker"}}, func(e *Exchange) (any, error) { return e.GetFuturesUsersCustomisedID(t.Context(), "broker") }},
		{"POST /fapi/v1/apiReferral/userCustomization", url.Values{"customerId": {"customer"}, "brokerId": {"broker"}}, func(e *Exchange) (any, error) {
			return e.CustomiseFuturesOwnClientID(t.Context(), "customer", "broker")
		}},
		{"GET /fapi/v1/apiReferral/customization", url.Values{"customerId": {"customer"}, "page": {"2"}, "limit": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetFuturesClientEmailCustomisedID(t.Context(), "customer", "", &ReferralCustomersRequest{Page: 2, Limit: 5})
		}},
		{"POST /fapi/v1/apiReferral/customization", url.Values{"customerId": {"customer"}, "email": {"test@example.com"}}, func(e *Exchange) (any, error) {
			return e.CustomiseFuturesPartnerClientID(t.Context(), "customer", "test@example.com")
		}},
		{"GET /fapi/v1/income", url.Values{"symbol": {"BTCUSDT"}, "incomeType": {"TRANSFER"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "limit": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetFuturesUserIncomeHistory(t.Context(), &GetFuturesUserIncomeHistoryRequest{Symbol: currency.NewBTCUSDT(), IncomeType: "TRANSFER", StartTime: start, EndTime: end, Limit: 5})
		}},
		{"GET /fapi/v1/apiReferral/overview", url.Values{"type": {"2"}}, func(e *Exchange) (any, error) { return e.GetFuturesRebateDataOverview(t.Context(), true) }},
		{"GET /fapi/v1/apiReferral/rebateVol", url.Values{"type": {"2"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "limit": {"5"}}, func(e *Exchange) (any, error) { return e.GetRebateVolume(t.Context(), true, start, end, 5) }},
		{"GET /fapi/v1/apiReferral/traderSummary", url.Values{"customerId": {"customer"}, "type": {"2"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "limit": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetTraderDetail(t.Context(), &GetTraderDetailRequest{CustomerID: "customer", CoinMargined: true, StartTime: start, EndTime: end, Limit: 5})
		}},
		{"GET /fapi/v1/apiReferral/traderNum", url.Values{"type": {"2"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "limit": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetFuturesReferredTradersNumber(t.Context(), true, start, end, 5)
		}},
		{"GET /fapi/v1/apiReferral/tradeVol", url.Values{"type": {"2"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "limit": {"5"}}, func(e *Exchange) (any, error) { return e.GetUserTradeVolume(t.Context(), true, start, end, 5) }},
		{"GET /papi/v1/apiReferral/ifNewUser", url.Values{"brokerId": {"broker"}, "type": {"2"}}, func(e *Exchange) (any, error) { return e.GetFuturesClientifNewUser(t.Context(), "broker", true) }},
		{"GET /fapi/v1/apiReferral/ifNewUser", url.Values{"brokerId": {"broker"}, "type": {"2"}}, func(e *Exchange) (any, error) { return e.GetFuturesClientIfNewUser(t.Context(), "broker", 2) }},
		{"POST /v1/api-key/create", url.Values{"apiName": {"test"}, "enableTrade": {"false"}, "enableFutureTrade": {"false"}, "publicKey": {"test-public-key"}, "enableMargin": {"false"}, "enableEuropeanOptions": {"false"}, "status": {"2"}, "ipAddress": {"192.0.2.1"}, "thirdPartyName": {"test-partner"}, "apiKeyPublicKey": {"test-signing-key"}}, func(e *Exchange) (any, error) {
			return e.CreateAPIKey(t.Context(), &BrokerAPIKeyRequest{AccessToken: "test-oauth-token", APIName: "test", PublicKey: "test-public-key", APIKeyPublicKey: "test-signing-key", Status: "2", IPAddress: "192.0.2.1", ThirdPartyName: "test-partner"})
		}},
		{"GET /v1/api-key/user-status", url.Values{}, func(e *Exchange) (any, error) {
			return e.GetFastAPIUserStatus(t.Context(), &FastAPIRequest{AccessToken: "test-oauth-token"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := fixtures[tc.name]
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, fixture.Method, r.Method, "HTTP method should follow documentation")
				assert.Equal(t, fixture.Path, r.URL.Path, "path should follow documentation")
				params := r.URL.Query()
				if strings.HasPrefix(fixture.Path, "/v1/api-key/") {
					assert.Equal(t, "Bearer test-oauth-token", r.Header.Get("Authorization"), "Fast API should use the explicitly supplied OAuth token")
					assert.Empty(t, r.Header.Get("X-MBX-APIKEY"), "Fast API should carry no unrelated exchange API key")
					assert.Empty(t, params, "OAuth requests should have no signed query")
					if r.Method == http.MethodPost {
						assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"), "Fast API should use the documented form transport")
						if !assert.NoError(t, r.ParseForm(), "form should decode") {
							return
						}
						params = r.PostForm
					}
				} else {
					sig := params.Get("signature")
					params.Del("signature")
					mac := hmac.New(sha256.New, []byte("test-secret"))
					_, _ = mac.Write([]byte(params.Encode()))
					assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), sig, "signature should cover every parameter")
					assert.NotEmpty(t, params.Get("timestamp"), "signed request should include a timestamp")
					params.Del("timestamp")
					assert.Equal(t, "5000", params.Get("recvWindow"), "default receive window should be preserved")
					params.Del("recvWindow")
				}
				assert.Equal(t, tc.params, params, "all documented business fields should use the correct spelling and values")
				_, err := w.Write(fixture.Data)
				assert.NoError(t, err, "documented response should write")
			})
			response, err := tc.call(local)
			require.NoError(t, err, "documented response must decode through its real request builder")
			assertResponseFields(t, fixture.Data, reflect.TypeOf(response), tc.name)
		})
	}
}

func TestOMSValidation(t *testing.T) {
	local := new(Exchange)
	_, err := local.GetFastAPIUserStatus(t.Context())
	assert.ErrorIs(t, err, errAccessTokenRequired, "OAuth status should require an explicit token")
	_, err = local.CreateAPIKey(t.Context(), &BrokerAPIKeyRequest{APIName: "test", PublicKey: "test-public-key"})
	assert.ErrorIs(t, err, errAccessTokenRequired, "key creation should require an explicit token")
	_, err = local.GetSpotClientEmailCustomisedID(t.Context(), "customer", "test@example.com")
	assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "Spot customer ID and email filters should be mutually exclusive")
	_, err = local.GetFuturesClientEmailCustomisedID(t.Context(), "customer", "test@example.com")
	assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "Futures customer ID and email filters should be mutually exclusive")
}
