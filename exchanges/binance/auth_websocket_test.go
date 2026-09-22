package binance

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
)

// mockDialConnection preserves the production connector while routing its dial
// through mockws's in-memory transport.
type mockDialConnection struct {
	websocket.Connection
	dialer *gws.Dialer
}

func (c *mockDialConnection) Dial(ctx context.Context, _ *gws.Dialer, headers http.Header, values url.Values) error {
	return c.Connection.Dial(ctx, c.dialer, headers, values)
}

func mockSpotAPI(t *testing.T, authenticated bool, handler mockws.WsMockFunc) *Exchange {
	t.Helper()
	server, dialer := mockws.NewTestServer(t, mockws.CurryWsMockUpgrader(t, handler))
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	local.SkipAuthCheck = true
	local.API.AuthenticatedWebsocketSupport = authenticated
	local.API.CredentialsValidator.RequiresSecret = true
	local.SetCredentials(&accounts.Credentials{Key: "test-key", Secret: "test-secret"})
	local.Websocket = websocket.NewManager()
	local.Websocket.DataHandler = stream.NewRelay(64)
	require.NoError(t, local.Websocket.Setup(&websocket.ManagerSetup{
		ExchangeConfig:               local.Config,
		Features:                     &local.Features.Supports.WebsocketCapabilities,
		UseMultiConnectionManagement: true,
	}), "mock manager must initialise")
	local.Websocket.SetCanUseAuthenticatedEndpoints(authenticated)
	setup := local.spotAPIConnectionSetup()
	setup.URL = "ws" + strings.TrimPrefix(server.URL, "http")
	setup.ResponseMaxLimit = 2 * time.Second
	setup.Connector = func(ctx context.Context, conn websocket.Connection) error {
		return local.WsConnectAPI(ctx, &mockDialConnection{Connection: conn, dialer: dialer})
	}
	require.NoError(t, local.Websocket.SetupNewConnection(setup), "production API setup must register")
	t.Cleanup(func() {
		if local.Websocket.IsEnabled() {
			assert.NoError(t, local.Websocket.Disable(), "mock websocket should disable")
		}
		if local.Websocket.IsConnected() {
			assert.NoError(t, local.Websocket.Shutdown(), "mock websocket should shut down")
		}
	})
	require.NoError(t, local.Websocket.Connect(t.Context()), "mock websocket must connect through the manager")
	return local
}

func assertHMACParams(tb testing.TB, raw json.RawMessage) map[string]json.RawMessage {
	tb.Helper()
	var params map[string]json.RawMessage
	require.NoError(tb, json.Unmarshal(raw, &params), "request params must decode")
	var signature, key string
	require.NoError(tb, json.Unmarshal(params["signature"], &signature), "signature must be present")
	require.NoError(tb, json.Unmarshal(params["apiKey"], &key), "API key must be present")
	assert.Equal(tb, "test-key", key, "mock request should use only the test key")
	values := make(map[string]any, len(params))
	for k, v := range params {
		if k == "signature" {
			continue
		}
		value := string(v)
		if len(v) > 0 && v[0] == '"' {
			require.NoError(tb, json.Unmarshal(v, &value), "string param must decode")
		}
		values[k] = value
	}
	var payload strings.Builder
	for i, k := range sortMapKeys(values) {
		if i > 0 {
			payload.WriteByte('&')
		}
		payload.WriteString(k + "=" + testRequestAs[string](tb, values[k]))
	}
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write([]byte(payload.String()))
	assert.Equal(tb, hex.EncodeToString(mac.Sum(nil)), signature, "signature should cover the exact transmitted parameters")
	return params
}

func TestSpotAPIAuthenticationLifecycle(t *testing.T) {
	requests := make(chan string, 8)
	local := mockSpotAPI(t, true, func(tb testing.TB, data []byte, conn *gws.Conn) error {
		tb.Helper()
		var request struct {
			ID     string          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		require.NoError(tb, json.Unmarshal(data, &request), "request must decode")
		requests <- request.Method
		switch request.Method {
		case "userDataStream.subscribe.signature":
			assertHMACParams(tb, request.Params)
			if err := conn.WriteJSON(map[string]any{"id": request.ID, "status": 200, "result": map[string]any{"subscriptionId": 0}}); err != nil {
				return err
			}
			// Binance's documented balanceUpdate envelope, including valid subscription 0.
			return conn.WriteMessage(gws.TextMessage, []byte(`{"subscriptionId":0,"event":{"e":"balanceUpdate","E":1573200697110,"a":"BTC","d":"100.00000000","T":1573200697068}}`))
		case "userDataStream.unsubscribe":
			assert.JSONEq(tb, `{"subscriptionId":0}`, string(request.Params), "unsubscribe should retain subscription ID zero")
			return conn.WriteJSON(map[string]any{"id": request.ID, "status": 200, "result": map[string]any{}})
		default:
			tb.Errorf("unexpected method %s; subscription should use the authenticated callback", request.Method)
			return nil
		}
	})
	assert.True(t, local.IsAPIStreamConnected(), "authenticated connection should be registered")
	assert.Equal(t, "userDataStream.subscribe.signature", <-requests, "startup should subscribe exactly once")
	select {
	case message := <-local.Websocket.DataHandler.C:
		balance, ok := message.Data.(WsBalanceUpdateData)
		require.True(t, ok, "user event must reach the balance handler")
		assert.Equal(t, currency.BTC, balance.Asset, "balance event should preserve its asset")
	case <-time.After(2 * time.Second):
		t.Fatal("documented user event must reach the data handler")
	}
	require.NoError(t, local.WsUnsubscribeUserDataStream(0), "subscription zero must be removable")
	assert.Equal(t, "userDataStream.unsubscribe", <-requests, "unsubscribe should use the correct method")
}

func TestSpotAPIOfflineErrors(t *testing.T) {
	local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
		tb.Helper()
		var request WsAPIRequest
		require.NoError(tb, json.Unmarshal(data, &request), "request must decode")
		return conn.WriteJSON(map[string]any{"id": request.ID, "status": 400, "error": map[string]any{"code": -1121, "msg": "Invalid symbol."}})
	})
	_, err := local.GetWsOrderbook(&OrderBookDataRequest{Symbol: currency.NewBTCUSDT(), Limit: 5})
	assert.ErrorIs(t, err, errAPIResponse, "API rejection should retain its sentinel")
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "known rejection should retain its mapped sentinel")
	_, err = local.WsPlaceOCOOrder(&PlaceOCOOrderRequest{Symbol: currency.NewBTCUSDT(), Side: order.Buy.String(), Quantity: 1, TrailingDelta: -1})
	assert.ErrorIs(t, err, errInvalidTrailingDelta, "negative trailing delta should fail before transport")
	_, err = local.WsQueryOCOOrder("", 0, 0)
	assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "missing list identifiers should fail before transport")
	_, err = local.WsSessionLogon()
	assert.ErrorIs(t, err, errEd25519KeyRequired, "HMAC credentials should not send an invalid session logon")
	_, _, err = local.SignRequest(map[string]any{})
	assert.ErrorIs(t, err, errTimestampInfoRequired, "signature without a timestamp should fail offline")
	_, _, err = local.SignRequest(map[string]any{"timestamp": "1e12"})
	assert.ErrorIs(t, err, errTimestampInfoRequired, "exponent timestamp should fail offline")
}

func TestSpotOrderListCancelRequests(t *testing.T) {
	for _, id := range []string{"", "9007199254740993"} {
		t.Run("orderListId="+id, func(t *testing.T) {
			local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
				tb.Helper()
				var request struct {
					ID     string          `json:"id"`
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				require.NoError(tb, json.Unmarshal(data, &request), "cancel request must decode")
				assert.Equal(tb, "orderList.cancel", request.Method, "cancellation should select the current method")
				params := assertHMACParams(tb, request.Params)
				assert.Equal(tb, `"BTCUSDT"`, string(params["symbol"]), "cancellation should send its required symbol")
				assert.Equal(tb, `"test-list"`, string(params["listClientOrderId"]), "cancellation should support the client ID alone")
				assert.Equal(tb, `"test-cancel"`, string(params["newClientOrderId"]), "cancellation should retain its new client ID")
				assert.Equal(tb, "5000.123", string(params["recvWindow"]), "cancellation should retain fractional receive windows")
				assert.Equal(tb, id, string(params["orderListId"]), "cancellation should preserve or omit the numeric list ID")
				return conn.WriteJSON(map[string]any{"id": request.ID, "status": 200, "result": map[string]any{"orderListId": 9007199254740993, "contingencyType": "OCO", "listStatusType": "ALL_DONE", "listOrderStatus": "ALL_DONE", "listClientOrderId": "test-list", "transactionTime": 1758792204196, "symbol": "BTCUSDT", "orders": []any{}, "orderReports": []any{}}})
			})
			_, err := local.WsCancelOCOOrder(currency.NewBTCUSDT(), id, "test-list", "test-cancel", 5000.123)
			require.NoError(t, err, "documented cancellation must decode through mockws")
		})
	}
	_, err := new(Exchange).WsCancelOCOOrder(currency.NewBTCUSDT(), "invalid", "", "")
	assert.ErrorIs(t, err, errInvalidOrderListID, "non-numeric list ID should retain its validation sentinel")
}

func TestSignedWebsocketReceiveWindows(t *testing.T) {
	local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
		tb.Helper()
		var request struct {
			ID     string          `json:"id"`
			Params json.RawMessage `json:"params"`
		}
		require.NoError(tb, json.Unmarshal(data, &request), "signed request must decode")
		params := assertHMACParams(tb, request.Params)
		assert.Equal(tb, "5000.123", string(params["recvWindow"]), "signed methods should preserve fractional receive windows")
		return conn.WriteJSON(map[string]any{"id": request.ID, "status": 200, "result": map[string]any{}})
	})
	_, err := local.WsSubscribeUserDataStreamWithSignature(5000.123)
	require.NoError(t, err, "subscription receive window must reach mockws")
	_, err = local.WsTestNewOrder(&TradeOrderRequest{Symbol: currency.NewBTCUSDT(), Side: "BUY", OrderType: "MARKET", Quantity: 1, RecvWindow: 5000.123}, false)
	require.NoError(t, err, "test-order receive window must reach mockws")
}

func TestWebsocketReferencePriceAlternatives(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		wantError    bool
	}{
		{"unset", `{"symbol":"BAZUSD","referencePrice":null,"timestamp":1770946889251}`, false},
		{"never set", `{"code":-2043,"msg":"This symbol doesn't have a reference price."}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
				tb.Helper()
				var request WsAPIRequest
				require.NoError(tb, json.Unmarshal(data, &request), "request must decode")
				return conn.WriteJSON(map[string]any{"id": request.ID, "status": 200, "result": json.RawMessage(tc.result)})
			})
			response, err := local.GetWsReferencePrice(currency.NewBTCUSDT())
			if tc.wantError {
				assert.ErrorIs(t, err, errAPIResponse, "an error in result should retain its sentinel")
				return
			}
			require.NoError(t, err, "a null reference price must decode")
			assert.Equal(t, "BAZUSD", response.Symbol, "an unset price should retain its symbol")
			assert.EqualValues(t, 1770946889251, response.Timestamp.Time().UnixMilli(), "an unset price should retain its timestamp")
		})
	}
}

func testEd25519Key(t *testing.T) (privateKey ed25519.PrivateKey, encoded string) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err, "test private key must marshal")
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestAuthenticatedWebsocketRequests(t *testing.T) {
	data, err := os.ReadFile("testdata/documented_websocket_responses.json")
	require.NoError(t, err, "documented WebSocket fixtures must load")
	var fixtures map[string]struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(data, &fixtures), "fixtures must decode")
	observed := make(chan map[string]json.RawMessage, 8)
	local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
		tb.Helper()
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		require.NoError(tb, json.Unmarshal(data, &request), "signed request must decode")
		params := assertHMACParams(tb, request.Params)
		params["method"], _ = json.Marshal(request.Method)
		observed <- params
		fixture, ok := fixtures[request.Method]
		require.True(tb, ok, "authenticated method must have a documented response")
		var response map[string]json.RawMessage
		require.NoError(tb, json.Unmarshal(fixture.Data, &response), "documented envelope must decode")
		response["id"] = request.ID
		return conn.WriteJSON(response)
	})
	arg := &TradeOrderRequest{Symbol: currency.NewBTCUSDT(), Side: "BUY", OrderType: "LIMIT", Quantity: 1, Price: 100, TimeInForce: "GTC", NewClientOrderID: "client-order", StrategyID: 9007199254740993, StrategyType: 1000000, NewOrderRespType: "FULL", RecvWindow: 5000.123}
	for range 2 {
		response, err := local.WsPlaceNewOrder(arg)
		require.NoError(t, err, "reused signed order request must succeed against its documented response")
		require.NotNil(t, response, "FULL order response must be returned")
		require.NotEmpty(t, response.Fills, "FULL response must preserve fills")
		params := <-observed
		assert.Equal(t, "9007199254740993", string(params["strategyId"]), "strategy ID should remain an exact JSON integer")
		assert.Equal(t, "5000.123", string(params["recvWindow"]), "order receive window should preserve its precision")
		assert.Equal(t, `"order.place"`, string(params["method"]), "order should use the documented placement method")
	}
	testOrder, err := local.WsTestNewOrder(arg, true)
	require.NoError(t, err, "test order must return documented commission estimates")
	require.NotNil(t, testOrder.Discount, "test order must retain discount information")
	params := <-observed
	assert.Equal(t, "true", string(params["computeCommissionRates"]), "test-order signature should include the commission flag")
	sor, err := local.WsTestNewOrderUsingSOR(&WsOSRPlaceOrderRequest{Symbol: currency.NewBTCUSDT(), Side: "BUY", OrderType: "LIMIT", Quantity: 1, Price: 100, TimeInForce: "GTC"}, true)
	require.NoError(t, err, "SOR test order must decode its commission response")
	require.NotNil(t, sor.Discount, "SOR test order must retain discount information")
	params = <-observed
	assert.Equal(t, `"sor.order.test"`, string(params["method"]), "SOR validation should call the test endpoint")
	require.Equal(t, "true", string(params["computeCommissionRates"]), "commission flag must be sent")
	account, err := local.GetWsAccountInfo(5000, false)
	require.NoError(t, err, "account request must decode its response")
	require.NotNil(t, account, "account response must be returned")
	params = <-observed
	assert.Equal(t, "false", string(params["omitZeroBalances"]), "explicit false should be retained")
	_, err = local.WsQueryOCOOrder("", 123, 5000)
	require.NoError(t, err, "list ID alone must be accepted")
	params = <-observed
	assert.Equal(t, "123", string(params["orderListId"]), "order list ID should be sent")
	assert.NotContains(t, params, "origClientOrderId", "absent client order ID should be omitted")
	_, err = local.WsPlaceOCOOrder(&PlaceOCOOrderRequest{Symbol: currency.NewBTCUSDT(), Side: "SELL", Quantity: 1, Price: 110, StopPrice: 90})
	require.NoError(t, err, "stop-price OCO must not require trailing delta")
	<-observed
}

func TestEd25519SessionLogon(t *testing.T) {
	privateKey, encoded := testEd25519Key(t)
	local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
		tb.Helper()
		var request struct {
			ID     string                     `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		require.NoError(tb, json.Unmarshal(data, &request), "session logon must decode")
		assert.Equal(tb, "session.logon", request.Method, "Ed25519 session should use logon")
		var signature string
		require.NoError(tb, json.Unmarshal(request.Params["signature"], &signature), "signature must decode")
		decoded, err := base64.StdEncoding.DecodeString(signature)
		require.NoError(tb, err, "Ed25519 signature must be base64")
		assert.Equal(tb, "5000.123", string(request.Params["recvWindow"]), "session receive window should retain its precision")
		message := "apiKey=test-key&recvWindow=5000.123&timestamp=" + string(request.Params["timestamp"])
		assert.True(tb, ed25519.Verify(ed25519.PublicKey(privateKey[ed25519.SeedSize:]), []byte(message), decoded), "signature should verify with the configured public key")
		return conn.WriteJSON(map[string]any{"id": request.ID, "status": 200, "result": map[string]any{"apiKey": "test-key", "authorizedSince": 1649729878532, "connectedSince": 1649729873021, "returnRateLimits": false, "serverTime": 1649729878630, "userDataStream": false}})
	})
	local.SkipAuthCheck = false
	local.API.AuthenticatedWebsocketSupport = true
	local.SetCredentials(&accounts.Credentials{Key: "test-key", Secret: encoded})
	response, err := local.WsSessionLogon(5000.123)
	require.NoError(t, err, "Ed25519 credentials must authenticate without bypassing credential checks")
	require.NotNil(t, response.UserDataStream, "session response must preserve the nullable subscription state")
	assert.False(t, *response.UserDataStream, "new session should report no user-data subscription")
}

func TestDocumentedUserDataEvents(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	require.NoError(t, local.CurrencyPairs.StorePairs(asset.Spot, currency.Pairs{currency.NewPair(currency.ETH, currency.BTC)}, false), "documented symbol must be available")
	require.NoError(t, local.CurrencyPairs.StorePairs(asset.Spot, currency.Pairs{currency.NewPair(currency.ETH, currency.BTC)}, true), "documented symbol must be enabled")

	local.Websocket.DataHandler = stream.NewRelay(64)
	testexch.FixtureToDataHandler(t, "testdata/documented_user_data.json", func(ctx context.Context, data []byte) error {
		var frame struct {
			Event json.RawMessage `json:"event"`
		}
		require.NoError(t, json.Unmarshal(data, &frame), "event envelope must decode")
		eventType, err := jsonparser.GetString(frame.Event, "e")
		require.NoError(t, err, "event type must decode")
		models := map[string]any{"outboundAccountPosition": new(WsAccountPositionData), "balanceUpdate": new(WsBalanceUpdateData), "executionReport": new(WsOrderUpdateData), "listStatus": new(WsListStatusData), "externalLockUpdate": new(WsExternalLockUpdate), "eventStreamTerminated": new(WsEventStreamTerminated)}
		model, ok := models[eventType]
		require.True(t, ok, "documented event must have a typed model")
		require.NoError(t, json.Unmarshal(frame.Event, model), "all documented fields must decode")
		assertResponseFields(t, frame.Event, reflect.TypeOf(model), eventType)
		err = local.wsHandleSpotAPIData(ctx, nil, data)
		if err != nil {
			return err
		}
		select {
		case <-local.Websocket.DataHandler.C:
		case <-time.After(time.Second):
			t.Fatal("documented event must be emitted")
		}
		return nil
	})
}
