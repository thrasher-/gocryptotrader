package binance

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Binance's listen-token specification includes parameters absent from the SDK:
// https://developers.binance.com/en/docs/products/margin-trading/listen-token-data-stream
func TestMarginListenToken(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request *MarginListenTokenRequest
		query   url.Values
	}{
		{"defaults", &MarginListenTokenRequest{}, url.Values{}},
		{"cross", &MarginListenTokenRequest{IsIsolated: new(false), Validity: time.Hour}, url.Values{"isIsolated": {"false"}, "validity": {"3600000"}}},
		{"isolated", &MarginListenTokenRequest{Symbol: currency.NewBTCUSDT(), IsIsolated: new(true), Validity: 24 * time.Hour}, url.Values{"symbol": {"BTCUSDT"}, "isIsolated": {"true"}, "validity": {"86400000"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method, "token should use POST")
				assert.Equal(t, "/sapi/v1/userListenToken", r.URL.Path, "token should use the documented path")
				params := r.URL.Query()
				assert.NotEmpty(t, params.Get("signature"), "token request should be signed")
				assert.NotEmpty(t, params.Get("timestamp"), "token request should include a timestamp")
				for _, key := range []string{"signature", "timestamp", "recvWindow"} {
					params.Del(key)
				}
				assert.Equal(t, tc.query, params, "token request should retain all documented parameters")
				_, err := w.Write([]byte(`{"token":"test-listen-token","expirationTime":1758792204196}`))
				assert.NoError(t, err, "mock token response should write")
			})
			response, err := local.CreateMarginListenToken(t.Context(), tc.request)
			require.NoError(t, err, "documented token response must decode")
			assert.Equal(t, "test-listen-token", response.Token, "response should preserve the token")
			assert.EqualValues(t, 1758792204196, response.ExpirationTime.Time().UnixMilli(), "response should preserve millisecond expiry")
		})
	}
	local := new(Exchange)
	_, err := local.CreateMarginListenToken(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should retain its sentinel")
	_, err = local.CreateMarginListenToken(t.Context(), &MarginListenTokenRequest{IsIsolated: new(true)})
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "isolated account without a symbol should retain its sentinel")
	for _, validity := range []time.Duration{-time.Millisecond, time.Nanosecond, 24*time.Hour + time.Millisecond} {
		_, err = local.CreateMarginListenToken(t.Context(), &MarginListenTokenRequest{Validity: validity})
		assert.ErrorIs(t, err, errInvalidListenTokenValidity, "invalid validity should fail before transport")
	}
	_, err = local.WsSubscribeMarginListenToken("")
	assert.ErrorIs(t, err, errListenTokenRequired, "empty token should fail before transport")
}

func TestMarginListenTokenWebsocket(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "subscribed", true: "rejected"}[rejected], func(t *testing.T) {
			local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
				tb.Helper()
				var request WsAPIRequest
				require.NoError(tb, json.Unmarshal(data, &request), "subscription request must decode")
				assert.Equal(tb, "userDataStream.subscribe.listenToken", request.Method, "margin token should select the documented method")
				raw, err := json.Marshal(request.Params)
				require.NoError(tb, err, "subscription parameters must encode")
				assert.JSONEq(tb, `{"listenToken":"test-listen-token"}`, string(raw), "unauthenticated session should send only its token")
				if rejected {
					return conn.WriteJSON(map[string]any{"id": request.ID, "status": 400, "error": map[string]any{"code": -1209, "msg": "Invalid listen token."}})
				}
				return conn.WriteJSON(map[string]any{"id": request.ID, "status": 200, "result": map[string]any{"subscriptionId": 0, "expirationTime": 1749094553955907}})
			})
			response, err := local.WsSubscribeMarginListenToken("test-listen-token")
			if rejected {
				assert.ErrorIs(t, err, errAPIResponse, "invalid token should retain the API error sentinel")
				return
			}
			require.NoError(t, err, "margin subscription must decode without session authentication")
			assert.Zero(t, response.SubscriptionID, "subscription ID zero should remain valid")
			assert.EqualValues(t, 1749094553955907, response.ExpirationTime.Time().UnixMicro(), "documented microsecond expiry should retain its precision")
		})
	}
}

// Synthetic values cover every field in Binance's two risk-event schemas.
// https://developers.binance.com/en/docs/products/margin-trading/risk-data-stream
func TestMarginRiskStreamFields(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	local.Websocket.DataHandler = stream.NewRelay(4)
	for _, raw := range []string{
		`{"e":"MARGIN_LEVEL_STATUS_CHANGE","E":1750515742303,"l":"1.25","s":"MARGIN_CALL"}`,
		`{"e":"USER_LIABILITY_CHANGE","E":1750515742303,"a":"BTC","t":"BORROW","p":"1.25","i":"0.005"}`,
	} {
		require.NoError(t, local.WsHandleMarginRiskData(t.Context(), nil, []byte(raw)), "risk event must decode")
		select {
		case value := <-local.Websocket.DataHandler.C:
			assertResponseFields(t, []byte(raw), reflect.TypeOf(value.Data), "margin risk")
		default:
			t.Fatal("risk event must be emitted")
		}
	}
	assert.ErrorIs(t, local.WsHandleMarginRiskData(t.Context(), nil, []byte(`{"e":"unknown"}`)), errUnsupportedChannel, "unknown risk event should retain its sentinel")
	for _, body := range []string{`{}`, `{"code":-1125,"msg":"invalid listen key"}`} {
		mock := mockBinanceHTTP(t, func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(body))
			assert.NoError(t, err, "mock response should write")
		})
		want := common.ErrNoResponse
		if body != `{}` {
			want = errAPIResponse
		}
		assert.ErrorIs(t, mock.WsMarginRiskConnect(t.Context(), nil), want, "failed margin key acquisition should retain its sentinel")
		assert.ErrorIs(t, mock.WsPortfolioMarginProConnect(t.Context(), nil), want, "failed PM Pro key acquisition should retain its sentinel")
	}
}

func TestMarginExecutionReportIDs(t *testing.T) {
	for _, value := range []json.RawMessage{[]byte(`"9007199254740993"`), []byte(`9007199254740993`)} {
		payload := map[string]json.RawMessage{}
		for _, key := range []string{"d", "j", "J", "v", "u", "U", "a"} {
			payload[key] = value
		}
		raw, err := json.Marshal(payload)
		require.NoError(t, err, "margin execution report must encode")
		var response WsOrderUpdateData
		require.NoError(t, json.Unmarshal(raw, &response), "quoted and bare IDs must decode")
		for _, id := range []types.PreciseNumber{response.TrailingDelta, response.StrategyID, response.StrategyType, response.PreventedMatchID, response.TradeGroupID, response.CounterOrderID, response.AllocationID} {
			assert.Equal(t, "9007199254740993", id.String(), "margin and Spot IDs should retain integer precision")
		}
	}
}
