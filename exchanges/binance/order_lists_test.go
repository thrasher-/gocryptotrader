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

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

func TestDocumentedOrderListRequests(t *testing.T) {
	data, err := os.ReadFile("testdata/documented_order_list_requests.json")
	require.NoError(t, err, "documented parameter cases must load")
	var requests map[string]struct {
		Params json.RawMessage `json:"params"`
	}
	require.NoError(t, json.Unmarshal(data, &requests), "documented parameter cases must decode")
	data, err = os.ReadFile("testdata/documented_responses.json")
	require.NoError(t, err, "REST responses must load")
	var responses map[string]*documentedResponse
	require.NoError(t, json.Unmarshal(data, &responses), "REST responses must decode")
	data, err = os.ReadFile("testdata/documented_websocket_responses.json")
	require.NoError(t, err, "WebSocket responses must load")
	var wsResponses map[string]struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(data, &wsResponses), "WebSocket responses must decode")
	for _, tc := range []struct {
		name, fixture string
		arg           any
		rest, ws      func(*Exchange, any) (any, error)
	}{
		{"oco", "spot/order_list_oco", new(OCOOrderListRequest), func(e *Exchange, data any) (any, error) {
			return e.NewOCOOrderList(t.Context(), testRequestAs[*OCOOrderListRequest](t, data))
		}, func(e *Exchange, data any) (any, error) {
			arg := testRequestAs[*OCOOrderListRequest](t, data)
			return e.WsNewOCOOrderList(arg, 0)
		}},
		{"oto", "spot/order_list_oto", new(OTOOrderRequest), func(e *Exchange, data any) (any, error) {
			return e.NewOTOOrderList(t.Context(), testRequestAs[*OTOOrderRequest](t, data))
		}, func(e *Exchange, data any) (any, error) {
			arg := testRequestAs[*OTOOrderRequest](t, data)
			return e.WsNewOTOOrder(arg, 0)
		}},
		{"otoco", "spot/order_list_otoco", new(OTOCOOrderRequest), func(e *Exchange, data any) (any, error) {
			return e.NewOTOCOOrderList(t.Context(), testRequestAs[*OTOCOOrderRequest](t, data))
		}, func(e *Exchange, data any) (any, error) {
			arg := testRequestAs[*OTOCOOrderRequest](t, data)
			return e.WsNewOTOCOOrder(arg, 0)
		}},
		{"opo", "spot/order_list_opo", new(OPOOrderRequest), func(e *Exchange, data any) (any, error) {
			return e.NewOPOOrderList(t.Context(), testRequestAs[*OPOOrderRequest](t, data))
		}, func(e *Exchange, data any) (any, error) {
			arg := testRequestAs[*OPOOrderRequest](t, data)
			return e.WsNewOPOOrderList(arg)
		}},
		{"opoco", "spot/order_list_opoco", new(OPOCOOrderRequest), func(e *Exchange, data any) (any, error) {
			return e.NewOPOCOOrderList(t.Context(), testRequestAs[*OPOCOOrderRequest](t, data))
		}, func(e *Exchange, data any) (any, error) {
			arg := testRequestAs[*OPOCOOrderRequest](t, data)
			return e.WsNewOPOCOOrderList(arg)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, json.Unmarshal(requests[tc.name].Params, tc.arg), "documented parameters must be representable")
			var expected map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(requests[tc.name].Params, &expected), "expected parameters must decode without precision loss")
			expected["symbol"] = json.RawMessage(`"BTCUSDT"`)
			t.Run("REST", func(t *testing.T) {
				fixture, ok := responses[tc.fixture]
				require.True(t, ok, "endpoint must have a documented response")
				local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, fixture.Path, r.URL.Path, "request should use the documented endpoint")
					assert.Equal(t, fixture.Method, r.Method, "request should use the documented verb")
					assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "request should use a placeholder key")
					params := r.URL.Query()
					signature := params.Get("signature")
					params.Del("signature")
					mac := hmac.New(sha256.New, []byte("test-secret"))
					_, _ = mac.Write([]byte(params.Encode()))
					assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), signature, "signature should cover the complete request")
					params.Del("timestamp")
					want := url.Values{}
					for key, value := range expected {
						v := string(value)
						if len(value) > 0 && value[0] == '"' {
							if !assert.NoError(t, json.Unmarshal(value, &v), "string parameter should decode") {
								return
							}
						}
						want.Set(key, v)
					}
					assert.Equal(t, want, params, "all documented parameters should be sent exactly")
					_, err := w.Write(fixture.Data)
					assert.NoError(t, err, "fixture should write")
				})
				response, err := tc.rest(local, tc.arg)
				require.NoError(t, err, "documented REST response must decode through its request method")
				assertResponseFields(t, fixture.Data, reflect.TypeOf(response), tc.name)
			})
			t.Run("WebSocket", func(t *testing.T) {
				method := "orderList.place." + tc.name
				local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
					tb.Helper()
					var request struct {
						ID     json.RawMessage `json:"id"`
						Method string          `json:"method"`
						Params json.RawMessage `json:"params"`
					}
					require.NoError(tb, json.Unmarshal(data, &request), "signed request must decode")
					assert.Equal(tb, method, request.Method, "request should use the documented method")
					params := assertHMACParams(tb, request.Params)
					delete(params, "apiKey")
					delete(params, "signature")
					delete(params, "timestamp")
					assert.Equal(tb, expected, params, "all documented parameters should be signed and sent without precision loss")
					var response map[string]json.RawMessage
					require.NoError(tb, json.Unmarshal(wsResponses[method].Data, &response), "documented response must decode")
					response["id"] = request.ID
					return conn.WriteJSON(response)
				})
				response, err := tc.ws(local, tc.arg)
				require.NoError(t, err, "documented WebSocket response must decode through its request method")
				require.NotNil(t, response, "order-list response must be returned")
			})
		})
	}
}
