package kraken

import (
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
)

type spotLiveAccess struct {
	requiresCredentials bool
	mutationAllowed     bool
	mutationOptIn       string
}

var (
	spotLivePublic  = spotLiveAccess{}
	spotLivePrivate = spotLiveAccess{requiresCredentials: true}

	spotLiveOrderAmendment = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canAmendRealSpotOrder,
		mutationOptIn:       "canAmendRealSpotOrder",
	}
	spotLiveCancelAllOrders = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canCancelAllRealSpotOrders,
		mutationOptIn:       "canCancelAllRealSpotOrders",
	}
	spotLiveDeadMansSwitch = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canArmRealSpotDeadMansSwitch,
		mutationOptIn:       "canArmRealSpotDeadMansSwitch",
	}
	spotLiveOrderBatchValidation = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canValidateRealSpotOrderBatch,
		mutationOptIn:       "canValidateRealSpotOrderBatch",
	}
	spotLiveOrderBatchCancellation = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canCancelRealSpotOrderBatch,
		mutationOptIn:       "canCancelRealSpotOrderBatch",
	}
	spotLiveOrderValidation = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canValidateRealSpotOrder,
		mutationOptIn:       "canValidateRealSpotOrder",
	}
	spotLiveOrderCancellation = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canCancelRealSpotOrder,
		mutationOptIn:       "canCancelRealSpotOrder",
	}
	spotLiveWithdrawal = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canWithdrawRealSpotFunds,
		mutationOptIn:       "canWithdrawRealSpotFunds",
	}
	spotLiveWithdrawalCancellation = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canCancelRealSpotWithdrawal,
		mutationOptIn:       "canCancelRealSpotWithdrawal",
	}
	spotLiveWalletTransfer = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canTransferRealSpotWalletFunds,
		mutationOptIn:       "canTransferRealSpotWalletFunds",
	}
	spotLiveEarnAllocation = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canAllocateRealSpotEarnFunds,
		mutationOptIn:       "canAllocateRealSpotEarnFunds",
	}
	spotLiveEarnDeallocation = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canDeallocateRealSpotEarnFunds,
		mutationOptIn:       "canDeallocateRealSpotEarnFunds",
	}
	spotLiveExportRequest = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canRequestRealSpotExportReport,
		mutationOptIn:       "canRequestRealSpotExportReport",
	}
	spotLiveExportDeletion = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canDeleteRealSpotExportReport,
		mutationOptIn:       "canDeleteRealSpotExportReport",
	}
	spotLiveSubaccountCreation = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canCreateRealSpotSubaccount,
		mutationOptIn:       "canCreateRealSpotSubaccount",
	}
	spotLiveSubaccountTransfer = spotLiveAccess{
		requiresCredentials: true,
		mutationAllowed:     canTransferRealSpotSubaccountFunds,
		mutationOptIn:       "canTransferRealSpotSubaccountFunds",
	}
)

type spotHTTPResponse struct {
	contentType string
	body        string
}

type spotFixtureSet struct {
	results   map[string]string
	responses map[string]spotHTTPResponse
}

var spotTestPair = currency.NewPair(currency.XBT, currency.USD)

var spotSharedFixtures = spotFixtureSet{
	results: map[string]string{
		"/0/private/GetWebSocketsToken": `{"token":"TOKEN","expires":900}`,
	},
	responses: map[string]spotHTTPResponse{
		"/0/private/RetrieveExportError": {contentType: "application/json", body: `{"error":["EGeneral:boom"],"result":null}`},
		"/0/private/TypedJSON":           {contentType: "application/json", body: `{"error":[],"result":{"value":"VALUE"}}`},
		"/0/private/RawJSON":             {contentType: "application/json", body: `{"error":[],"result":{}}`},
		"/0/private/RawScalar":           {body: `123`},
		"/0/private/RawObject":           {body: `{"report":"data"}`},
		"/0/private/NormalError":         {contentType: "application/json", body: `{"error":["EGeneral:boom"],"result":null}`},
		"/0/private/SemanticError":       {contentType: "application/json", body: `{"error":[1],"result":null}`},
		"/0/private/Warning":             {contentType: "application/json", body: `{"error":["WGeneral:warning"],"result":{"value":"VALUE"}}`},
		"/0/private/Malformed":           {contentType: "application/json", body: `{`},
	},
}

var allSpotFixtures = []spotFixtureSet{
	spotSharedFixtures,
	spotMarketFixtures,
	spotAccountFixtures,
	spotTradingFixtures,
	spotFundingFixtures,
	spotEarnFixtures,
	spotSubaccountsFixtures,
}

type capturedSpotRequest struct {
	path   string
	values url.Values
}

const invalidSpotValue = "invalid"

var errSpotTransport = errors.New("spot REST transport failure")

type spotTestRoundTripper func(*http.Request) (*http.Response, error)

func (r spotTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

func TestSpotTestRoundTripper(t *testing.T) {
	t.Parallel()
	expectedRequest := httptest.NewRequest(http.MethodGet, "https://kraken.test", http.NoBody)
	expectedResponse := &http.Response{Body: http.NoBody}
	roundTripper := spotTestRoundTripper(func(req *http.Request) (*http.Response, error) {
		require.Same(t, expectedRequest, req, "RoundTrip must pass through the request")
		return expectedResponse, errSpotTransport
	})
	response, err := roundTripper.RoundTrip(expectedRequest)
	require.ErrorIs(t, err, errSpotTransport, "RoundTrip must return the transport error")
	require.Same(t, expectedResponse, response, "RoundTrip must return the transport response")
	require.NoError(t, response.Body.Close(), "RoundTrip response body must close")
}

func TestFormatSpotFloat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		value         float64
		expected      string
		expectedError error
	}{
		{name: "zero", value: 0, expected: "0"},
		{name: "negative", value: -1.25, expected: "-1.25"},
		{name: "small decimal", value: 0.000000001, expected: "0.000000001"},
		{name: "NaN", value: math.NaN(), expectedError: errNumericValueInvalid},
		{name: "positive infinity", value: math.Inf(1), expectedError: errNumericValueInvalid},
		{name: "negative infinity", value: math.Inf(-1), expectedError: errNumericValueInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatSpotFloat(tc.value)
			require.ErrorIs(t, err, tc.expectedError, "formatSpotFloat must return the expected error")
			assert.Equal(t, tc.expected, result, "formatSpotFloat should return the expected wire value")
		})
	}
}

func TestGetWebsocketToken(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotSharedFixtures)
	response, err := ex.GetWebsocketToken(t.Context())
	require.NoError(t, err, "GetWebsocketToken must not error")
	assert.Equal(t, "TOKEN", response.Token, "GetWebsocketToken should decode the token")
	assert.Equal(t, int64(900), response.Expires, "GetWebsocketToken should decode the expiry")
	requireSpotRequest(t, requests, "/0/private/GetWebSocketsToken")
	response, err = newSpotNullResultExchange(t).GetWebsocketToken(t.Context())
	require.NoError(t, err, "GetWebsocketToken must accept a null REST result")
	require.Nil(t, response, "GetWebsocketToken must return nil for a null REST result")
	response, err = newSpotErrorExchange(t).GetWebsocketToken(t.Context())
	require.ErrorIs(t, err, errSpotTransport, "GetWebsocketToken must surface request errors")
	assert.Nil(t, response, "GetWebsocketToken response should remain nil after a request error")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetWebsocketToken(t.Context())
		require.NoError(t, err, "GetWebsocketToken must not error against the live API")
		require.NotNil(t, response, "GetWebsocketToken must return a response from the live API")
		require.NotEmpty(t, response.Token, "GetWebsocketToken live response must include a token")
		require.Positive(t, response.Expires, "GetWebsocketToken live response must include a positive expiry")
	})
}

func skipSpotLiveTest(tb testing.TB, access spotLiveAccess) {
	tb.Helper()
	skipSpotLiveTestWithState(tb, access, mockTests, spotLiveExchange)
}

func skipSpotLiveTestWithState(tb testing.TB, access spotLiveAccess, mock bool, ex *Exchange) {
	tb.Helper()
	if mock {
		tb.Skip("live testing disabled; run with -tags=mock_test_off to enable")
	}
	if access.requiresCredentials {
		sharedtestvalues.SkipTestIfCredentialsUnset(tb, ex)
	}
	if access.mutationOptIn != "" && !access.mutationAllowed {
		tb.Skipf("live mutation disabled; set %s to true after reviewing the endpoint payload", access.mutationOptIn)
	}
}

func TestSkipSpotLiveTest(t *testing.T) {
	reachedEndpoint := false
	require.True(t, t.Run("configured build", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePublic)
		reachedEndpoint = true
	}), "skipSpotLiveTest configured-build subtest must not fail")
	assert.Equal(t, !mockTests, reachedEndpoint, "skipSpotLiveTest should follow the configured build mode")
}

func TestSkipSpotLiveTestWithState(t *testing.T) {
	t.Run("mock build", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			skipSpotLiveTestWithState(t, spotLivePublic, true, nil)
			reachedEndpoint = true
		}), "skipSpotLiveTestWithState mock subtest must not fail")
		assert.False(t, reachedEndpoint, "skipSpotLiveTestWithState should skip live calls in a mock build")
	})

	t.Run("public live", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("continue", func(t *testing.T) {
			skipSpotLiveTestWithState(t, spotLivePublic, false, nil)
			reachedEndpoint = true
		}), "skipSpotLiveTestWithState public subtest must not fail")
		assert.True(t, reachedEndpoint, "skipSpotLiveTestWithState should continue public live calls")
	})

	t.Run("private without credentials", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			skipSpotLiveTestWithState(t, spotLivePrivate, false, new(Exchange))
			reachedEndpoint = true
		}), "skipSpotLiveTestWithState private subtest must not fail")
		assert.False(t, reachedEndpoint, "skipSpotLiveTestWithState should skip private live calls without credentials")
	})

	t.Run("private with credentials", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("continue", func(t *testing.T) {
			skipSpotLiveTestWithState(t, spotLivePrivate, false, newAuthenticatedSpotExchange(t, "https://kraken.test"))
			reachedEndpoint = true
		}), "skipSpotLiveTestWithState private subtest must not fail")
		assert.True(t, reachedEndpoint, "skipSpotLiveTestWithState should continue private live calls with credentials")
	})

	t.Run("mutation disabled", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			skipSpotLiveTestWithState(t, spotLiveAccess{requiresCredentials: true, mutationOptIn: "canMutate"}, false, newAuthenticatedSpotExchange(t, "https://kraken.test"))
			reachedEndpoint = true
		}), "skipSpotLiveTestWithState mutation subtest must not fail")
		assert.False(t, reachedEndpoint, "skipSpotLiveTestWithState should skip mutating live calls without explicit opt-in")
	})

	t.Run("mutation enabled", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("continue", func(t *testing.T) {
			skipSpotLiveTestWithState(t, spotLiveAccess{requiresCredentials: true, mutationAllowed: true, mutationOptIn: "canMutate"}, false, newAuthenticatedSpotExchange(t, "https://kraken.test"))
			reachedEndpoint = true
		}), "skipSpotLiveTestWithState mutation-enabled subtest must not fail")
		assert.True(t, reachedEndpoint, "skipSpotLiveTestWithState should continue mutating live calls with explicit opt-in")
	})
}

func spotLiveTestValue(tb testing.TB, name string) string {
	tb.Helper()
	value := os.Getenv(name)
	if value == "" {
		tb.Skipf("live test parameter %s is unset", name)
	}
	return value
}

func TestSpotLiveTestValue(t *testing.T) {
	const name = "GCT_KRAKEN_SPOT_LIVE_TEST_VALUE"
	t.Run("set", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("continue", func(t *testing.T) {
			t.Setenv(name, "VALUE")
			assert.Equal(t, "VALUE", spotLiveTestValue(t, name), "spotLiveTestValue should return the configured value")
			reachedEndpoint = true
		}), "spotLiveTestValue set subtest must not fail")
		assert.True(t, reachedEndpoint, "spotLiveTestValue should continue when the value is set")
	})

	t.Run("unset", func(t *testing.T) {
		t.Setenv(name, "")
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			spotLiveTestValue(t, name)
			reachedEndpoint = true
		}), "spotLiveTestValue unset subtest must not fail")
		assert.False(t, reachedEndpoint, "spotLiveTestValue should skip when the value is unset")
	})
}

func parseSpotLiveTestPositiveFloat(value string) (float64, error) {
	amount, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, errAmountInvalid
	}
	return amount, nil
}

func TestParseSpotLiveTestPositiveFloat(t *testing.T) {
	for _, tc := range []struct {
		name          string
		value         string
		expected      float64
		expectedError error
	}{
		{name: "valid", value: "0.001", expected: 0.001},
		{name: "invalid", value: "invalid", expectedError: strconv.ErrSyntax},
		{name: "zero", value: "0", expectedError: errAmountInvalid},
		{name: "negative", value: "-1", expectedError: errAmountInvalid},
		{name: "NaN", value: "NaN", expectedError: errAmountInvalid},
		{name: "infinity", value: "+Inf", expectedError: errAmountInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			amount, err := parseSpotLiveTestPositiveFloat(tc.value)
			require.ErrorIs(t, err, tc.expectedError, "parseSpotLiveTestPositiveFloat must return the expected error")
			assert.Equal(t, tc.expected, amount, "parseSpotLiveTestPositiveFloat should return the expected value")
		})
	}
}

func cloneSpotValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, value := range values {
		cloned[key] = slices.Clone(value)
	}
	return cloned
}

func newSpotEndpointExchange(t *testing.T, fixtureSets ...spotFixtureSet) (ex *Exchange, capturedRequests <-chan capturedSpotRequest) {
	t.Helper()
	requests := make(chan capturedSpotRequest, 128)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := cloneSpotValues(r.URL.Query())
		payload, _ := io.ReadAll(r.Body)
		bodyValues, _ := url.ParseQuery(string(payload))
		for key, value := range bodyValues {
			values[key] = slices.Clone(value)
		}
		requests <- capturedSpotRequest{path: r.URL.Path, values: values}
		var result string
		var response spotHTTPResponse
		var found, special bool
		for _, fixtures := range fixtureSets {
			response, special = fixtures.responses[r.URL.Path]
			if special {
				found = true
				break
			}
			result, found = fixtures.results[r.URL.Path]
			if found {
				break
			}
		}
		if !found {
			http.Error(w, "unexpected Spot REST route", http.StatusNotFound)
			return
		}
		if special {
			if response.contentType != "" {
				w.Header().Set("Content-Type", response.contentType)
			}
			_, _ = io.WriteString(w, response.body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"error":[],"result":`+result+`}`)
	}))
	t.Cleanup(server.Close)
	ex = newAuthenticatedSpotExchange(t, server.URL)
	require.NoError(t, ex.Requester.DisableRateLimiter(), "DisableRateLimiter must disable rate limiting for endpoint tests")
	return ex, requests
}

func TestNewSpotEndpointExchangeUnknownRoute(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	_, err := ex.GetAccountBalance(t.Context(), &GetAccountBalanceRequest{})
	require.Error(t, err, "newSpotEndpointExchange must reject an unknown fixture route")
	requireSpotRequest(t, requests, "/0/private/Balance")
	var raw request.RawResponse
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RawJSON", url.Values{}, &raw)
	require.Error(t, err, "newSpotEndpointExchange must reject an undeclared special route")
	requireSpotRequest(t, requests, "/0/private/RawJSON")
}

func newSpotErrorExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := newAuthenticatedSpotExchange(t, "https://kraken.invalid")
	err := ex.Requester.SetHTTPClient(&http.Client{Transport: spotTestRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errSpotTransport
	})})
	require.NoError(t, err, "SetHTTPClient must install the endpoint error transport")
	require.NoError(t, ex.Requester.DisableRateLimiter(), "DisableRateLimiter must disable rate limiting for endpoint error tests")
	return ex
}

func newSpotNullResultExchange(t *testing.T) *Exchange {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"error":[],"result":null}`)
	}))
	t.Cleanup(server.Close)
	ex := newAuthenticatedSpotExchange(t, server.URL)
	require.NoError(t, ex.Requester.DisableRateLimiter(), "DisableRateLimiter must disable rate limiting for nil-result tests")
	return ex
}

func requireSpotRequest(t *testing.T, requests <-chan capturedSpotRequest, path string) url.Values {
	t.Helper()
	select {
	case capturedRequest := <-requests:
		require.Equal(t, path, capturedRequest.path, "Spot REST request must use the expected path")
		return capturedRequest.values
	case <-time.After(time.Second):
		require.FailNow(t, "Spot REST request must reach the mock server")
		return nil
	}
}

func TestIsValidSpotEnum(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		value    string
		expected bool
	}{
		{name: "empty optional value", expected: true},
		{name: "allowed value", value: "allowed", expected: true},
		{name: "invalid value", value: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, isValidSpotEnum(tc.value, "allowed"), "isValidSpotEnum should validate documented values")
		})
	}
}

func TestErrorResponseUnmarshalJSONMalformed(t *testing.T) {
	t.Parallel()
	var response errorResponse
	require.Error(t, response.UnmarshalJSON([]byte("{")), "UnmarshalJSON must reject malformed JSON")
}

func TestErrorResponse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		json             string
		expectedError    string
		expectedAPIError string
		expectedWarning  string
	}{
		{
			name: "no errors or warnings",
			json: `{"error":[],"result":{"unixtime":1721884425,"rfc1123":"Thu, 25 Jul 24 05:13:45 +0000"}}`,
		},
		{
			name:          "invalid array entry",
			json:          `{"error":[69420],"result":{}}`,
			expectedError: "unable to convert 69420 to string",
		},
		{
			name:          "unsupported error container",
			json:          `{"error":124,"result":{}}`,
			expectedError: "unhandled error response type float64",
		},
		{
			name:             "error string in array",
			json:             `{"error":["EQuery:Unknown asset pair"],"result":{}}`,
			expectedAPIError: "EQuery:Unknown asset pair",
		},
		{
			name:             "single error string",
			json:             `{"error":"EService:Unavailable","result":{}}`,
			expectedAPIError: "EService:Unavailable",
		},
		{
			name:            "warning string in array",
			json:            `{"error":["WGeneral:Danger"],"result":{}}`,
			expectedWarning: "WGeneral:Danger",
		},
		{
			name:            "single warning string",
			json:            `{"error":"WGeneral:Unknown warning","result":{}}`,
			expectedWarning: "WGeneral:Unknown warning",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var response genericRESTResponse
			err := json.Unmarshal([]byte(tc.json), &response)
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError, "genericRESTResponse must return the expected decoding error")
				return
			}
			require.NoError(t, err, "genericRESTResponse must decode the response")
			if tc.expectedAPIError == "" {
				assert.NoError(t, response.Error.Errors(), "genericRESTResponse errors should remain empty")
			} else {
				assert.ErrorContains(t, response.Error.Errors(), tc.expectedAPIError, "genericRESTResponse errors should contain the API error")
			}
			if tc.expectedWarning == "" {
				assert.Empty(t, response.Error.Warnings(), "genericRESTResponse warnings should remain empty")
			} else {
				assert.Contains(t, response.Error.Warnings(), tc.expectedWarning, "genericRESTResponse warnings should contain the API warning")
			}
		})
	}
}

func TestSendAuthenticatedHTTPRequestRawResult(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotSharedFixtures, spotAccountFixtures)

	var nilRaw *request.RawResponse
	err := ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RetrieveExport", url.Values{}, nilRaw)
	require.ErrorIs(t, err, common.ErrNilPointer, "SendAuthenticatedHTTPRequest must reject a nil raw result pointer")

	var raw request.RawResponse
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RetrieveExport", url.Values{"id": {"REPORT"}}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must not error for a raw response")
	assert.Equal(t, "PK\x03\x04export", string(raw), "SendAuthenticatedHTTPRequest should preserve raw bytes")
	requireSpotRequest(t, requests, "/0/private/RetrieveExport")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RetrieveExportError", url.Values{}, &raw)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must surface JSON errors returned by a raw endpoint")
	requireSpotRequest(t, requests, "/0/private/RetrieveExportError")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RawJSON", url.Values{}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must accept an error-free JSON response from a raw endpoint")
	requireSpotRequest(t, requests, "/0/private/RawJSON")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RawScalar", url.Values{}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must preserve a scalar JSON export body")
	assert.Equal(t, "123", string(raw), "SendAuthenticatedHTTPRequest should preserve a scalar JSON export body")
	requireSpotRequest(t, requests, "/0/private/RawScalar")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RawObject", url.Values{}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must preserve a JSON object without an API error envelope")
	assert.JSONEq(t, `{"report":"data"}`, string(raw), "SendAuthenticatedHTTPRequest should preserve a JSON object without an API error envelope")
	requireSpotRequest(t, requests, "/0/private/RawObject")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "SemanticError", url.Values{}, &raw)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must reject semantically invalid JSON from a raw endpoint")
	requireSpotRequest(t, requests, "/0/private/SemanticError")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "Warning", url.Values{}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must allow warnings returned by a raw endpoint")
	requireSpotRequest(t, requests, "/0/private/Warning")

	var result struct {
		Value string `json:"value"`
	}
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "TypedJSON", url.Values{}, &result)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must not error for a JSON response")
	assert.Equal(t, "VALUE", result.Value, "SendAuthenticatedHTTPRequest should decode JSON results")
	requireSpotRequest(t, requests, "/0/private/TypedJSON")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "NormalError", url.Values{}, &result)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must surface JSON API errors")
	requireSpotRequest(t, requests, "/0/private/NormalError")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "SemanticError", url.Values{}, &result)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must reject semantically invalid JSON")
	requireSpotRequest(t, requests, "/0/private/SemanticError")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "Malformed", url.Values{}, &result)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must reject malformed JSON")
	requireSpotRequest(t, requests, "/0/private/Malformed")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "Warning", url.Values{}, &result)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must allow warning-only responses")
	requireSpotRequest(t, requests, "/0/private/Warning")

	err = new(Exchange).SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "AmendOrder", url.Values{}, &result)
	require.Error(t, err, "SendAuthenticatedHTTPRequest must reject unavailable credentials")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.URL(255), "AmendOrder", url.Values{}, &result)
	require.Error(t, err, "SendAuthenticatedHTTPRequest must reject an invalid endpoint type")

	err = newSpotErrorExchange(t).SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "AmendOrder", url.Values{}, &result)
	require.ErrorIs(t, err, errSpotTransport, "SendAuthenticatedHTTPRequest must surface transport errors")
}
