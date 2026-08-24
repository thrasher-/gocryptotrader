package bybit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	testutils "github.com/thrasher-corp/gocryptotrader/internal/testing/utils"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
	"github.com/thrasher-corp/gocryptotrader/types"
)

const (
	rejectedWebsocketOrderID = "reject-order"
	websocketTestOrderID     = "order-id"
	websocketTestLastPrice   = "LastPrice"
	websocketTestMarkPrice   = "MarkPrice"
	websocketTestPartial     = "Partial"
	websocketTestNoError     = "EC_NoError"
	websocketTestNew         = "New"
	websocketTestCancelled   = "Cancelled"
)

type websocketTradeRequestFixture struct {
	websocket.Connection
	confirmation             []byte
	sendErr                  error
	match                    <-chan websocket.MatchedResponse
	matchErr                 error
	waitForMatchCancellation bool
	matchCancelled           chan struct{}
	matchID                  any
	matchCount               int
	sentLimit                request.EndpointLimit
	sentID                   any
	sentRequest              any
	sent                     chan struct{}
	recorder                 *websocketTradeRequestRecorder
}

type websocketTradeRequestRecorder struct {
	calls []string
}

func (f *websocketTradeRequestFixture) SendMessageReturnResponse(_ context.Context, limit request.EndpointLimit, id, payload any) ([]byte, error) {
	f.recorder.calls = append(f.recorder.calls, "send")
	f.sentLimit = limit
	f.sentID = id
	f.sentRequest = payload
	if f.sent != nil {
		close(f.sent)
	}
	return f.confirmation, f.sendErr
}

func (f *websocketTradeRequestFixture) MatchReturnResponses(ctx context.Context, id any, count int) (<-chan websocket.MatchedResponse, error) {
	f.recorder.calls = append(f.recorder.calls, "match")
	f.matchID = id
	f.matchCount = count
	if f.waitForMatchCancellation {
		responses := make(chan websocket.MatchedResponse, 1)
		go func() {
			<-ctx.Done()
			close(f.matchCancelled)
			responses <- websocket.MatchedResponse{Err: ctx.Err()}
			close(responses)
		}()
		return responses, f.matchErr
	}
	return f.match, f.matchErr
}

func websocketMatchedResponse(response websocket.MatchedResponse) <-chan websocket.MatchedResponse {
	responses := make(chan websocket.MatchedResponse, 1)
	responses <- response
	close(responses)
	return responses
}

func TestWebsocketTradeRequestFixture(t *testing.T) {
	t.Parallel()

	recorder := new(websocketTradeRequestRecorder)
	expectedErr := errors.New("forced fixture error")
	fixture := &websocketTradeRequestFixture{
		confirmation: []byte(`{"retCode":0}`),
		sendErr:      expectedErr,
		match:        websocketMatchedResponse(websocket.MatchedResponse{Err: expectedErr}),
		matchErr:     expectedErr,
		recorder:     recorder,
	}
	payload := struct{}{}
	confirmation, err := fixture.SendMessageReturnResponse(t.Context(), wsOrderSpotEPL, "request-id", payload)
	require.ErrorIs(t, err, expectedErr, "fixture send must return its configured error")
	assert.JSONEq(t, `{"retCode":0}`, string(confirmation), "fixture confirmation should match")
	assert.Equal(t, wsOrderSpotEPL, fixture.sentLimit, "fixture rate limit should match")
	assert.Equal(t, "request-id", fixture.sentID, "fixture request ID should match")
	assert.Equal(t, payload, fixture.sentRequest, "fixture request should match")

	responses, err := fixture.MatchReturnResponses(t.Context(), "match-id", 2)
	require.ErrorIs(t, err, expectedErr, "fixture matcher must return its configured error")
	assert.Equal(t, "match-id", fixture.matchID, "fixture match ID should match")
	assert.Equal(t, 2, fixture.matchCount, "fixture match count should match")
	require.ErrorIs(t, (<-responses).Err, expectedErr, "fixture matched response must match")
	assert.Equal(t, []string{"send", "match"}, recorder.calls, "fixture calls should be recorded")

	fixture.matchErr = nil
	fixture.waitForMatchCancellation = true
	fixture.matchCancelled = make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	responses, err = fixture.MatchReturnResponses(ctx, "cancelled", 1)
	require.NoError(t, err, "cancellation-aware fixture matcher must not error")
	cancel()
	require.ErrorIs(t, (<-responses).Err, context.Canceled, "cancellation-aware fixture must return context cancellation")
	<-fixture.matchCancelled
}

func TestWebsocketOrderResponse(t *testing.T) {
	t.Parallel()
	const response = `{"topic":"order","id":"74199870_22004_157776604406","creationTime":1786689290245,"data":[{"category":"spot","symbol":"FLOWUSDT","orderId":"2281309546159014400","orderLinkId":"019ffefa-ffb4-738f-9c29-ad4fe85302e5","blockTradeId":"","side":"Sell","positionIdx":0,"orderStatus":"Filled","cancelType":"UNKNOWN","rejectReason":"EC_NoError","timeInForce":"FOK","isLeverage":"0","price":"0.03154","qty":"194.10","avgPrice":"0.03154","leavesQty":"0","leavesValue":"0.0000000","cumExecQty":"194.1","cumExecValue":"6.1219140","cumExecFee":"0.0048975312","orderType":"Limit","stopOrderType":"","orderIv":"","triggerPrice":"0.00000","takeProfit":"0.00000","stopLoss":"0.00000","triggerBy":"","tpTriggerBy":"","slTriggerBy":"","triggerDirection":0,"placeType":"","lastPriceOnCreated":"0.03157","closeOnTrigger":false,"reduceOnly":false,"smpGroup":"0","smpType":"None","smpOrderId":"","slLimitPrice":"0.00000","tpLimitPrice":"0.00000","marketUnit":"","createdTime":"1786689290243","updatedTime":"1786689290244","feeCurrency":"USDT","slippageTolerance":"","slippageToleranceType":"UNKNOWN","cumFeeDetail":{"USDT":"0.0048975312"},"rpiTakerAccess":false,"rpiMatchedQty":"0"}]}`

	var result WebsocketOrderResponse
	require.NoError(t, json.Unmarshal([]byte(response), &result), "order response must decode")
	require.Len(t, result.OrderDetails, 1, "response must contain one order")
	require.Equal(t, "74199870_22004_157776604406", result.ID, "response ID must match")
	require.Equal(t, "order", result.Topic, "response topic must match")
	require.Equal(t, "FLOWUSDT", result.OrderDetails[0].Symbol, "order symbol must match")
	require.Equal(t, "2281309546159014400", result.OrderDetails[0].OrderID, "order ID must match")
	require.Equal(t, "Filled", result.OrderDetails[0].OrderStatus, "order status must match")
	require.Equal(t, types.Time(time.UnixMilli(1786689290245)), result.CreationTime, "creation time must match")
	require.Equal(t, types.Time(time.UnixMilli(1786689290243)), result.OrderDetails[0].CreatedTime, "created time must match")
	require.Equal(t, types.Time(time.UnixMilli(1786689290244)), result.OrderDetails[0].UpdatedTime, "updated time must match")
	require.Equal(t, order.Sell, result.OrderDetails[0].Side, "order side must match")
	require.Equal(t, "UNKNOWN", result.OrderDetails[0].SlippageToleranceType, "slippage tolerance type must match")
	require.Equal(t, types.Number(0.0048975312), result.OrderDetails[0].CumulativeFeeDetail["USDT"], "cumulative fee must match")
}

func TestGetWebsocketTradeConnections(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("forced connection error")
	outbound := &websocketTradeRequestFixture{}
	inbound := &websocketTradeRequestFixture{}
	for _, tc := range []struct {
		name           string
		requireInbound bool
		outboundErr    error
		inboundErr     error
		expectedErr    error
		expectedCalls  []any
	}{
		{name: "outbound error", outboundErr: expectedErr, expectedErr: expectedErr, expectedCalls: []any{OutboundTradeConnection}},
		{name: "outbound only", expectedCalls: []any{OutboundTradeConnection}},
		{name: "inbound error", requireInbound: true, inboundErr: expectedErr, expectedErr: expectedErr, expectedCalls: []any{OutboundTradeConnection, InboundPrivateConnection}},
		{name: "both connections", requireInbound: true, expectedCalls: []any{OutboundTradeConnection, InboundPrivateConnection}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := make([]any, 0, 2)
			getter := func(filter any) (websocket.Connection, error) {
				calls = append(calls, filter)
				switch filter {
				case OutboundTradeConnection:
					return outbound, tc.outboundErr
				case InboundPrivateConnection:
					return inbound, tc.inboundErr
				default:
					return nil, fmt.Errorf("unexpected connection filter: %v", filter)
				}
			}
			gotOutbound, gotInbound, err := getWebsocketTradeConnections(getter, tc.requireInbound)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr, "connection lookup must return the expected error")
				assert.Nil(t, gotOutbound, "failed connection lookup should not return an outbound connection")
				assert.Nil(t, gotInbound, "failed connection lookup should not return an inbound connection")
			} else {
				require.NoError(t, err, "connection lookup must not error")
				assert.Same(t, outbound, gotOutbound, "outbound connection should match")
				if tc.requireInbound {
					assert.Same(t, inbound, gotInbound, "inbound connection should match")
				} else {
					assert.Nil(t, gotInbound, "outbound-only lookup should not return an inbound connection")
				}
			}
			assert.Equal(t, tc.expectedCalls, calls, "connection filters should match")
		})
	}
}

func TestSendWebsocketTradeAcknowledgement(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	expectedErr := errors.New("forced websocket error")
	requestPayload := &PlaceOrderRequest{Category: cSpot, Symbol: currency.NewBTCUSDT()}
	for _, tc := range []struct {
		name             string
		sendErr          error
		confirmation     []byte
		expectedAnyError bool
		expectedErr      error
		expectedContains string
	}{
		{name: "send error", sendErr: expectedErr, expectedErr: expectedErr},
		{name: "invalid confirmation", confirmation: []byte(`{`), expectedAnyError: true},
		{name: "rejected confirmation", confirmation: []byte(`{"retCode":10404,"retMsg":"bad request"}`), expectedContains: "code:10404"},
		{name: "success", confirmation: []byte(`{"reqId":"request-id","retCode":0,"retMsg":"OK","data":{"orderId":"order-id","orderLinkId":"order-link-id"}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := new(websocketTradeRequestRecorder)
			outbound := &websocketTradeRequestFixture{confirmation: tc.confirmation, sendErr: tc.sendErr, recorder: recorder}
			got, err := ex.sendWebsocketTradeAcknowledgement(t.Context(), outbound, "order.create", requestPayload, wsOrderSpotEPL)
			switch {
			case tc.expectedAnyError:
				require.Error(t, err, "sendWebsocketTradeAcknowledgement must return a decoding error")
				return
			case tc.expectedErr != nil:
				require.ErrorIs(t, err, tc.expectedErr, "sendWebsocketTradeAcknowledgement must return the expected error")
				return
			case tc.expectedContains != "":
				require.ErrorContains(t, err, tc.expectedContains, "sendWebsocketTradeAcknowledgement must return the expected error")
				return
			}

			require.NoError(t, err, "sendWebsocketTradeAcknowledgement must not error")
			require.NotNil(t, got, "sendWebsocketTradeAcknowledgement must return a confirmation")
			assert.Equal(t, websocketTestOrderID, got.RequestAcknowledgement.OrderID, "acknowledged order ID should match")
			assert.Equal(t, wsOrderSpotEPL, outbound.sentLimit, "rate limit should match")
			wirePayload, ok := outbound.sentRequest.(WebsocketGeneralPayload)
			require.True(t, ok, "outbound payload must use WebsocketGeneralPayload")
			assert.NotEmpty(t, wirePayload.RequestID, "request ID should be populated")
			assert.Equal(t, outbound.sentID, wirePayload.RequestID, "request ID should match the response signature")
			assert.Equal(t, "order.create", wirePayload.Operation, "operation should match")
			assert.NotEmpty(t, wirePayload.Header["X-BAPI-TIMESTAMP"], "timestamp header should be populated")
			require.Len(t, wirePayload.Arguments, 1, "arguments must contain the request")
			assert.Same(t, requestPayload, wirePayload.Arguments[0], "request argument should match")
		})
	}
}

func TestWebsocketOrderUpdateStore(t *testing.T) {
	t.Parallel()

	var store websocketOrderUpdateStore
	store.publish(nil)
	_, _, err := store.subscribe("")
	require.ErrorIs(t, err, errOrderLinkIDMissing, "subscribe must reject an empty match ID")

	listener, unsubscribe, err := store.subscribe("client-id")
	require.NoError(t, err, "subscribe must register a client order ID")
	_, _, err = store.subscribe("client-id")
	require.Error(t, err, "subscribe must reject a duplicate match ID")
	store.publish(&WebsocketOrderDetails{OrderID: websocketTestOrderID, OrderLinkID: "client-id"})
	store.publish(&WebsocketOrderDetails{OrderID: websocketTestOrderID, OrderLinkID: "client-id", OrderStatus: websocketTestNew})
	select {
	case <-listener.notify:
	default:
		require.FailNow(t, "publish must notify the listener")
	}
	listener.mu.Lock()
	require.Len(t, listener.updates, 2, "publish must queue every client-correlated update without blocking")
	assert.Equal(t, websocketTestOrderID, listener.updates[0].OrderID, "publish must prefer and deliver the client-correlated update")
	listener.mu.Unlock()
	unsubscribe()

	listener, unsubscribe, err = store.subscribe(websocketTestOrderID)
	require.NoError(t, err, "subscribe must allow a cleaned-up match ID")
	store.publish(&WebsocketOrderDetails{OrderID: websocketTestOrderID})
	listener.mu.Lock()
	require.Len(t, listener.updates, 1, "publish must queue the exchange-correlated update")
	assert.Equal(t, websocketTestOrderID, listener.updates[0].OrderID, "publish must deliver an exchange-correlated update")
	listener.mu.Unlock()
	store.publish(&WebsocketOrderDetails{OrderID: "unmatched"})

	unsubscribe()
	unsubscribe()

	closedListener := &websocketOrderUpdateListener{notify: make(chan struct{}, 1), closed: true}
	store.listeners["closed-id"] = closedListener
	store.publish(&WebsocketOrderDetails{OrderID: "closed-id"})
	assert.Empty(t, closedListener.updates, "publish must ignore a closed listener")
}

func TestWebsocketAmendOrderConfirmed(t *testing.T) {
	t.Parallel()
	takeProfitPrice := 4.0
	stopLossPrice := 5.0

	amendRequest := &AmendOrderRequest{
		OrderQuantity:          1,
		Price:                  2,
		TriggerPrice:           3,
		TakeProfitPrice:        &takeProfitPrice,
		StopLossPrice:          &stopLossPrice,
		TakeProfitLimitPrice:   6,
		StopLossLimitPrice:     7,
		OrderImpliedVolatility: "8.0",
		TakeProfitTriggerBy:    websocketTestMarkPrice,
		StopLossTriggerBy:      "IndexPrice",
		TriggerPriceType:       websocketTestLastPrice,
		TPSLMode:               websocketTestPartial,
	}
	matching := WebsocketOrderDetails{
		Quantity:               1,
		Price:                  2,
		TriggerPrice:           3,
		TakeProfit:             4,
		StopLoss:               5,
		TakeProfitLimitPrice:   6,
		StopLossLimitPrice:     7,
		OrderImpliedVolatility: 8,
		TakeProfitTriggerBy:    websocketTestMarkPrice,
		StopLossTriggerBy:      "IndexPrice",
		TriggerBy:              websocketTestLastPrice,
		TakeProfitStopLossMode: websocketTestPartial,
	}
	confirmed, err := websocketAmendOrderConfirmed(amendRequest, &matching)
	require.NoError(t, err, "matching amendment fields must not error")
	assert.True(t, confirmed, "matching amendment fields must confirm the operation")

	for _, tc := range []struct {
		name   string
		mutate func(*WebsocketOrderDetails)
	}{
		{name: "quantity", mutate: func(update *WebsocketOrderDetails) { update.Quantity = 9 }},
		{name: "price", mutate: func(update *WebsocketOrderDetails) { update.Price = 9 }},
		{name: "trigger price", mutate: func(update *WebsocketOrderDetails) { update.TriggerPrice = 9 }},
		{name: "take profit", mutate: func(update *WebsocketOrderDetails) { update.TakeProfit = 9 }},
		{name: "stop loss", mutate: func(update *WebsocketOrderDetails) { update.StopLoss = 9 }},
		{name: "take profit limit", mutate: func(update *WebsocketOrderDetails) { update.TakeProfitLimitPrice = 9 }},
		{name: "stop loss limit", mutate: func(update *WebsocketOrderDetails) { update.StopLossLimitPrice = 9 }},
		{name: "implied volatility", mutate: func(update *WebsocketOrderDetails) { update.OrderImpliedVolatility = 9 }},
		{name: "take profit trigger", mutate: func(update *WebsocketOrderDetails) { update.TakeProfitTriggerBy = websocketTestLastPrice }},
		{name: "stop loss trigger", mutate: func(update *WebsocketOrderDetails) { update.StopLossTriggerBy = websocketTestLastPrice }},
		{name: "trigger type", mutate: func(update *WebsocketOrderDetails) { update.TriggerBy = websocketTestMarkPrice }},
		{name: "TPSL mode", mutate: func(update *WebsocketOrderDetails) { update.TakeProfitStopLossMode = "Full" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			update := matching
			tc.mutate(&update)
			gotConfirmed, gotErr := websocketAmendOrderConfirmed(amendRequest, &update)
			require.NoError(t, gotErr, "a non-terminal mismatched update must not error")
			assert.False(t, gotConfirmed, "a mismatched requested field must not confirm the amendment")
		})
	}

	confirmed, err = websocketAmendOrderConfirmed(new(AmendOrderRequest), new(WebsocketOrderDetails))
	require.NoError(t, err, "omitted amendment fields must not error")
	assert.True(t, confirmed, "omitted amendment fields must not require confirmation")
	zero := 0.0
	confirmed, err = websocketAmendOrderConfirmed(&AmendOrderRequest{TakeProfitPrice: &zero, StopLossPrice: &zero}, new(WebsocketOrderDetails))
	require.NoError(t, err, "explicit take-profit and stop-loss cancellation must not error")
	assert.True(t, confirmed, "explicit zero take-profit and stop-loss values must confirm cancellation")
	confirmed, err = websocketAmendOrderConfirmed(&AmendOrderRequest{TakeProfitPrice: &zero}, &WebsocketOrderDetails{TakeProfit: 1})
	require.NoError(t, err, "an uncleared take-profit must not error")
	assert.False(t, confirmed, "an explicit zero take-profit must wait for the cleared value")
	for _, status := range []string{"Filled", websocketTestCancelled, "Rejected", "Deactivated", "PartiallyFilledCanceled"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			confirmed, err := websocketAmendOrderConfirmed(amendRequest, &WebsocketOrderDetails{OrderStatus: status})
			assert.False(t, confirmed, "a terminal order must not confirm an amendment")
			require.ErrorIs(t, err, errWebsocketOrderTerminalState, "a terminal order must fail an amendment immediately")
		})
	}
	_, err = websocketAmendOrderConfirmed(&AmendOrderRequest{OrderImpliedVolatility: "invalid"}, new(WebsocketOrderDetails))
	require.ErrorIs(t, err, errInvalidOrderImpliedVolatility, "invalid implied volatility must fail confirmation")
}

func TestWebsocketAmendStringConfirmed(t *testing.T) {
	t.Parallel()

	assert.True(t, websocketAmendStringConfirmed("", "anything"), "an omitted string must not require confirmation")
	assert.False(t, websocketAmendStringConfirmed("UNKNOWN", "anything"), "an explicit unknown string must require an exact confirmation")
	assert.True(t, websocketAmendStringConfirmed(websocketTestMarkPrice, websocketTestMarkPrice), "equal strings must confirm")
	assert.False(t, websocketAmendStringConfirmed(websocketTestMarkPrice, websocketTestLastPrice), "different requested strings must not confirm")
}

func TestWebsocketCancelOrderConfirmed(t *testing.T) {
	t.Parallel()

	for _, status := range []string{websocketTestCancelled, "Deactivated", "PartiallyFilledCanceled"} {
		confirmed, err := websocketCancelOrderConfirmed(&WebsocketOrderDetails{OrderStatus: status})
		require.NoError(t, err, "a cancellation terminal status must not error")
		assert.True(t, confirmed, "a cancellation terminal status must confirm cancellation")
	}
	for _, status := range []string{"Filled", "Rejected"} {
		confirmed, err := websocketCancelOrderConfirmed(&WebsocketOrderDetails{OrderStatus: status})
		assert.False(t, confirmed, "a conflicting terminal status must not confirm cancellation")
		require.ErrorIs(t, err, errWebsocketOrderTerminalState, "a conflicting terminal status must fail cancellation immediately")
	}
	confirmed, err := websocketCancelOrderConfirmed(&WebsocketOrderDetails{OrderStatus: websocketTestNew})
	require.NoError(t, err, "a non-terminal cancellation update must not error")
	assert.False(t, confirmed, "a non-terminal update must not confirm cancellation")
}

func TestWebsocketOrderRejection(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{"", websocketTestNoError, "EC_PerCancelRequest"} {
		require.NoError(t, websocketOrderRejection(&WebsocketOrderDetails{OrderStatus: websocketTestNew, RejectReason: reason}), "a successful lifecycle reason must not reject the operation")
	}
	require.ErrorIs(t, websocketOrderRejection(&WebsocketOrderDetails{OrderStatus: "Rejected"}), errWebsocketOrderRejected, "a rejected status must fail the operation")
	require.ErrorIs(t, websocketOrderRejection(&WebsocketOrderDetails{OrderStatus: "Filled", RejectReason: "EC_OrigClOrdIDDoesNotExist"}), errWebsocketOrderRejected, "a rejection reason must fail the operation")
	require.ErrorIs(t, websocketOrderRejection(&WebsocketOrderDetails{OrderStatus: websocketTestCancelled, RejectReason: "EC_CancelReplaceOrder"}), errWebsocketOrderRejected, "a cancel-replace reason must not be treated as a successful amendment")
}

func TestSendWebsocketTradeRequestUntil(t *testing.T) {
	t.Parallel()

	validConfirmation := []byte(`{"retCode":0,"retMsg":"OK"}`)
	requestPayload := &CancelOrderRequest{Category: cSpot, Symbol: currency.NewBTCUSDT(), OrderID: websocketTestOrderID}

	t.Run("nil confirmation", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		_, err := ex.sendWebsocketTradeRequestUntil(t.Context(), new(websocketTradeRequestFixture), "order.cancel", websocketTestOrderID, requestPayload, wsOrderSpotEPL, nil)
		require.ErrorIs(t, err, errNilArgument, "sendWebsocketTradeRequestUntil must reject a nil confirmation function")
	})

	t.Run("listener collision", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		_, unsubscribe, err := ex.websocketOrderUpdates.subscribe(websocketTestOrderID)
		require.NoError(t, err, "fixture listener must register")
		defer unsubscribe()
		_, err = ex.sendWebsocketTradeRequestUntil(t.Context(), new(websocketTradeRequestFixture), "order.cancel", websocketTestOrderID, requestPayload, wsOrderSpotEPL, websocketCancelOrderConfirmed)
		require.ErrorIs(t, err, errWebsocketOrderUpdateListenerExists, "sendWebsocketTradeRequestUntil must return listener collisions")
	})

	t.Run("send error removes listener", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("forced send error")
		ex := new(Exchange)
		outbound := &websocketTradeRequestFixture{sendErr: expectedErr, recorder: new(websocketTradeRequestRecorder)}
		_, err := ex.sendWebsocketTradeRequestUntil(t.Context(), outbound, "order.cancel", websocketTestOrderID, requestPayload, wsOrderSpotEPL, websocketCancelOrderConfirmed)
		require.ErrorIs(t, err, expectedErr, "sendWebsocketTradeRequestUntil must return send errors")
		_, unsubscribe, err := ex.websocketOrderUpdates.subscribe(websocketTestOrderID)
		require.NoError(t, err, "send errors must remove the listener")
		unsubscribe()
	})

	t.Run("queues updates then confirms cancellation", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		sent := make(chan struct{})
		outbound := &websocketTradeRequestFixture{confirmation: validConfirmation, sent: sent, recorder: new(websocketTradeRequestRecorder)}
		result := make(chan *WebsocketOrderDetails, 1)
		errs := make(chan error, 1)
		go func() {
			got, err := ex.sendWebsocketTradeRequestUntil(t.Context(), outbound, "order.cancel", websocketTestOrderID, requestPayload, wsOrderSpotEPL, websocketCancelOrderConfirmed)
			result <- got
			errs <- err
		}()
		<-sent
		ex.websocketOrderUpdates.publish(&WebsocketOrderDetails{OrderID: websocketTestOrderID, OrderStatus: websocketTestNew, RejectReason: websocketTestNoError})
		ex.websocketOrderUpdates.publish(&WebsocketOrderDetails{OrderID: websocketTestOrderID, OrderStatus: websocketTestCancelled})
		require.NoError(t, <-errs, "sendWebsocketTradeRequestUntil must inspect queued updates until cancellation is confirmed")
		assert.Equal(t, websocketTestCancelled, (<-result).OrderStatus, "the operation-specific terminal update must be returned")
	})

	t.Run("fill fails cancellation", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		sent := make(chan struct{})
		outbound := &websocketTradeRequestFixture{confirmation: validConfirmation, sent: sent, recorder: new(websocketTradeRequestRecorder)}
		errs := make(chan error, 1)
		go func() {
			_, err := ex.sendWebsocketTradeRequestUntil(t.Context(), outbound, "order.cancel", websocketTestOrderID, requestPayload, wsOrderSpotEPL, websocketCancelOrderConfirmed)
			errs <- err
		}()
		<-sent
		ex.websocketOrderUpdates.publish(&WebsocketOrderDetails{OrderID: websocketTestOrderID, OrderStatus: "Filled", RejectReason: websocketTestNoError})
		require.ErrorIs(t, <-errs, errWebsocketOrderTerminalState, "a fill racing cancellation must fail immediately")
	})

	t.Run("private rejection", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		sent := make(chan struct{})
		outbound := &websocketTradeRequestFixture{confirmation: validConfirmation, sent: sent, recorder: new(websocketTradeRequestRecorder)}
		errs := make(chan error, 1)
		go func() {
			_, err := ex.sendWebsocketTradeRequestUntil(t.Context(), outbound, "order.cancel", websocketTestOrderID, requestPayload, wsOrderSpotEPL, websocketCancelOrderConfirmed)
			errs <- err
		}()
		<-sent
		ex.websocketOrderUpdates.publish(&WebsocketOrderDetails{OrderID: websocketTestOrderID, RejectReason: "EC_OrigClOrdIDDoesNotExist"})
		err := <-errs
		require.ErrorIs(t, err, errWebsocketOrderRejected, "private rejections must fail the operation")
		require.ErrorContains(t, err, "EC_OrigClOrdIDDoesNotExist", "private rejection errors must retain the exchange reason")
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		ex.WebsocketResponseMaxLimit = time.Nanosecond
		outbound := &websocketTradeRequestFixture{confirmation: validConfirmation, recorder: new(websocketTradeRequestRecorder)}
		_, err := ex.sendWebsocketTradeRequestUntil(t.Context(), outbound, "order.cancel", websocketTestOrderID, requestPayload, wsOrderSpotEPL, websocketCancelOrderConfirmed)
		require.ErrorIs(t, err, websocket.ErrSignatureTimeout, "sendWebsocketTradeRequestUntil must bound the private update wait")
	})

	t.Run("context cancellation", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		outbound := &websocketTradeRequestFixture{confirmation: validConfirmation, recorder: new(websocketTradeRequestRecorder)}
		_, err := ex.sendWebsocketTradeRequestUntil(ctx, outbound, "order.cancel", websocketTestOrderID, requestPayload, wsOrderSpotEPL, websocketCancelOrderConfirmed)
		require.ErrorIs(t, err, context.Canceled, "sendWebsocketTradeRequestUntil must return context cancellation")
	})
}

func TestSendWebsocketTradeRequest(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	expectedErr := errors.New("forced websocket error")
	validConfirmation := []byte(`{"retCode":0,"retMsg":"OK"}`)
	validOrderResponse := []byte(`{"data":[{"orderId":"order-id","rejectReason":"EC_NoError"}]}`)
	requestPayload := &PlaceOrderRequest{Category: cSpot, Symbol: currency.NewBTCUSDT()}
	for _, tc := range []struct {
		name                     string
		matchErr                 error
		sendErr                  error
		matchedResponse          websocket.MatchedResponse
		waitForMatchCancellation bool
		expectedAnyError         bool
		expectedErr              error
		expectedContains         string
		expectedOrderID          string
	}{
		{name: "response listener error", matchErr: expectedErr, expectedErr: expectedErr},
		{name: "send error tears down listener", sendErr: expectedErr, waitForMatchCancellation: true, expectedErr: expectedErr},
		{name: "inbound response error", matchedResponse: websocket.MatchedResponse{Err: expectedErr}, expectedErr: expectedErr},
		{name: "unexpected inbound response count", matchedResponse: websocket.MatchedResponse{}, expectedContains: "expected 1 matched response, received 0"},
		{name: "invalid inbound response", matchedResponse: websocket.MatchedResponse{Responses: [][]byte{[]byte(`{`)}}, expectedAnyError: true},
		{name: "unexpected order count", matchedResponse: websocket.MatchedResponse{Responses: [][]byte{[]byte(`{"data":[]}`)}}, expectedContains: "expected 1 order detail, received 0"},
		{name: "rejected order", matchedResponse: websocket.MatchedResponse{Responses: [][]byte{[]byte(`{"data":[{"rejectReason":"EC_PostOnlyWillTakeLiquidity"}]}`)}}, expectedContains: "order rejected"},
		{name: "success", matchedResponse: websocket.MatchedResponse{Responses: [][]byte{validOrderResponse}}, expectedOrderID: websocketTestOrderID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := new(websocketTradeRequestRecorder)
			outbound := &websocketTradeRequestFixture{confirmation: validConfirmation, sendErr: tc.sendErr, recorder: recorder}
			inbound := &websocketTradeRequestFixture{
				match:                    websocketMatchedResponse(tc.matchedResponse),
				matchErr:                 tc.matchErr,
				waitForMatchCancellation: tc.waitForMatchCancellation,
				matchCancelled:           make(chan struct{}),
				recorder:                 recorder,
			}
			got, err := ex.sendWebsocketTradeRequest(t.Context(), outbound, inbound, "order.create", "order-link-id", requestPayload, wsOrderSpotEPL)
			switch {
			case tc.expectedAnyError:
				require.Error(t, err, "sendWebsocketTradeRequest must return a decoding error")
			case tc.expectedErr != nil:
				require.ErrorIs(t, err, tc.expectedErr, "sendWebsocketTradeRequest must return the expected error")
			case tc.expectedContains != "":
				require.ErrorContains(t, err, tc.expectedContains, "sendWebsocketTradeRequest must return the expected error")
			default:
				require.NoError(t, err, "sendWebsocketTradeRequest must not error")
				require.NotNil(t, got, "sendWebsocketTradeRequest must return order details")
				assert.Equal(t, tc.expectedOrderID, got.OrderID, "order ID should match")
			}
			assert.Equal(t, "order-link-id", inbound.matchID, "match signature should match")
			assert.Equal(t, 1, inbound.matchCount, "match count should match")
			if tc.matchErr == nil {
				assert.Equal(t, []string{"match", "send"}, recorder.calls, "response matching should be registered before sending")
			}
			if tc.waitForMatchCancellation {
				select {
				case <-inbound.matchCancelled:
				default:
					require.FailNow(t, "response listener must be cancelled before sendWebsocketTradeRequest returns")
				}
			}
		})
	}
}

func TestWSCreateOrder(t *testing.T) {
	t.Parallel()

	_, err := e.WSCreateOrder(t.Context(), nil)
	require.ErrorIs(t, err, errNilArgument, "WSCreateOrder must reject nil input")
	_, _, err = prepareWSCreateOrder(nil)
	require.ErrorIs(t, err, errNilArgument, "prepareWSCreateOrder must reject nil input")

	arg := &PlaceOrderRequest{}
	_, _, err = prepareWSCreateOrder(arg)
	require.ErrorIs(t, err, errCategoryNotSet)
	require.NotErrorIs(t, err, errUnknownCategory, "prepareWSCreateOrder must return validation errors before rate-limit errors")

	arg.Category = cSpot
	_, _, err = prepareWSCreateOrder(arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = currency.NewBTCUSDT()
	_, _, err = prepareWSCreateOrder(arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "Buy"
	_, _, err = prepareWSCreateOrder(arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = "Limit"
	_, _, err = prepareWSCreateOrder(arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.OrderQuantity = 0.0001
	arg.TriggerDirection = 69
	_, _, err = prepareWSCreateOrder(arg)
	require.ErrorIs(t, err, errInvalidTriggerDirection)

	arg.TriggerDirection = 0
	arg.OrderFilter = "dodgy"
	_, _, err = prepareWSCreateOrder(arg)
	require.ErrorIs(t, err, errInvalidOrderFilter)

	arg.OrderFilter = "Order"
	arg.TriggerPriceType = "dodgy"
	_, _, err = prepareWSCreateOrder(arg)
	require.ErrorIs(t, err, errInvalidTriggerPriceType)

	arg.TriggerPriceType = ""
	arg.EnableBorrow = true
	original := *arg

	_, err = e.WSCreateOrder(t.Context(), arg)
	require.ErrorIs(t, err, websocket.ErrNotConnected, "WSCreateOrder must return a disconnected manager error")
	require.Equal(t, original, *arg, "WSCreateOrder must not mutate the caller request")

	wireArgument, limit, err := prepareWSCreateOrder(arg)
	require.NoError(t, err, "prepareWSCreateOrder must prepare a valid request")
	assert.Equal(t, wsOrderSpotEPL, limit, "WSCreateOrder rate limit should match")
	require.NotEmpty(t, wireArgument.OrderLinkID, "WSCreateOrder must generate an order link ID")
	assert.Equal(t, int64(1), wireArgument.IsLeverage, "WSCreateOrder should apply borrow leverage to the wire request")
	wireJSON, err := json.Marshal(wireArgument)
	require.NoError(t, err, "WSCreateOrder wire request must encode")
	expectedWireJSON := fmt.Sprintf(`{"category":"spot","symbol":"BTCUSDT","side":"Buy","orderType":"Limit","qty":"0.0001","orderLinkId":%q,"isLeverage":1,"orderFilter":"Order"}`, wireArgument.OrderLinkID)
	require.JSONEq(t, expectedWireJSON, string(wireJSON), "WSCreateOrder wire request must match")
	require.Equal(t, original, *arg, "prepareWSCreateOrder must not mutate the caller request")

	providedLinkID := *arg
	providedLinkID.OrderLinkID = "provided-order-link-id"
	wireArgument, _, err = prepareWSCreateOrder(&providedLinkID)
	require.NoError(t, err, "prepareWSCreateOrder must accept a provided order link ID")
	assert.Equal(t, "provided-order-link-id", wireArgument.OrderLinkID, "WSCreateOrder wire request should retain the provided order link ID")
	require.Equal(t, "provided-order-link-id", providedLinkID.OrderLinkID, "prepareWSCreateOrder must not mutate a provided order link ID")

	type tradeRequest struct {
		RequestID string `json:"reqId"`
		Operation string `json:"op"`
		Arguments []struct {
			OrderID     string `json:"orderId"`
			OrderLinkID string `json:"orderLinkId"`
		} `json:"args"`
	}
	requests := make(chan tradeRequest, 1)
	ex := testexch.MockWsInstance[Exchange](t, mockws.CurryWsMockUpgrader(t, func(_ testing.TB, payload []byte, conn *gws.Conn) error {
		var request tradeRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			return err
		}
		if len(request.Arguments) != 1 {
			return fmt.Errorf("expected one websocket trade argument, received %d", len(request.Arguments))
		}
		confirmationResponse := WebsocketConfirmation{
			RequestID: request.RequestID,
			RetMsg:    "OK",
			Operation: request.Operation,
			RequestAcknowledgement: OrderResponse{
				OrderID:     "created-order-id",
				OrderLinkID: request.Arguments[0].OrderLinkID,
			},
		}
		if request.Arguments[0].OrderLinkID == rejectedWebsocketOrderID {
			confirmationResponse.RetCode = 10404
			confirmationResponse.RetMsg = "bad request"
		}
		confirmation, err := json.Marshal(confirmationResponse)
		if err != nil {
			return err
		}
		if err := conn.WriteMessage(gws.TextMessage, confirmation); err != nil {
			return err
		}
		requests <- request
		return nil
	}))
	t.Cleanup(func() {
		if ex.Websocket.IsConnected() {
			assert.NoError(t, ex.Websocket.Shutdown(), "mock websocket manager shutdown should not error")
		}
	})
	type createResult struct {
		details *WebsocketOrderDetails
		err     error
	}
	result := make(chan createResult, 1)
	go func() {
		details, err := ex.WSCreateOrder(t.Context(), arg)
		result <- createResult{details: details, err: err}
	}()
	sentRequest := <-requests
	require.Equal(t, "order.create", sentRequest.Operation, "WSCreateOrder mock operation must match")
	require.Len(t, sentRequest.Arguments, 1, "WSCreateOrder mock request must contain one argument")
	require.NotEmpty(t, sentRequest.Arguments[0].OrderLinkID, "WSCreateOrder mock request must contain the generated client order ID")
	inbound, err := ex.Websocket.GetConnection(InboundPrivateConnection)
	require.NoError(t, err, "mock private websocket connection must be available")
	privateResponse := fmt.Appendf(nil, `{"data":[{"orderId":"created-order-id","orderLinkId":%q,"rejectReason":"EC_NoError","orderStatus":"New","timeInForce":"GTC"}]}`, sentRequest.Arguments[0].OrderLinkID)
	require.True(t, inbound.IncomingWithData(sentRequest.Arguments[0].OrderLinkID, privateResponse), "mock private order update must match the generated client order ID")
	created := <-result
	require.NoError(t, created.err, "WSCreateOrder must complete against connected mock websocket transports")
	require.NotNil(t, created.details, "WSCreateOrder must return mock order details")
	assert.Equal(t, "created-order-id", created.details.OrderID, "WSCreateOrder mock order ID should match")
	require.Equal(t, original, *arg, "WSCreateOrder mock transport must not mutate the caller request")
	rejectedArg := *arg
	rejectedArg.OrderLinkID = rejectedWebsocketOrderID
	_, err = ex.WSCreateOrder(t.Context(), &rejectedArg)
	require.ErrorContains(t, err, "code:10404", "WSCreateOrder must return rejected trade acknowledgements")
	rejectedRequest := <-requests
	assert.Equal(t, rejectedWebsocketOrderID, rejectedRequest.Arguments[0].OrderLinkID, "WSCreateOrder rejected request client order ID should match")

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		if mockTests {
			t.Skip("live testing disabled; run with -tags=mock_test_off to enable")
		}
		value := os.Getenv("GCT_BYBIT_LIVE_CREATE_ORDER")
		if value == "" {
			t.Skip("GCT_BYBIT_LIVE_CREATE_ORDER is unset")
		}
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, true)
		var liveConfig struct {
			DedicatedTestAccount bool              `json:"dedicated_test_account"`
			Order                PlaceOrderRequest `json:"order"`
		}
		require.NoError(t, json.Unmarshal([]byte(value), &liveConfig), "GCT_BYBIT_LIVE_CREATE_ORDER must contain valid JSON")
		require.True(t, liveConfig.DedicatedTestAccount, "GCT_BYBIT_LIVE_CREATE_ORDER must set dedicated_test_account=true")
		require.Equal(t, cSpot, liveConfig.Order.Category, "GCT_BYBIT_LIVE_CREATE_ORDER must use a spot order")
		require.Equal(t, "Limit", liveConfig.Order.OrderType, "GCT_BYBIT_LIVE_CREATE_ORDER must use a limit order")
		require.Equal(t, "PostOnly", liveConfig.Order.TimeInForce, "GCT_BYBIT_LIVE_CREATE_ORDER must use a post-only order")
		require.Positive(t, liveConfig.Order.OrderQuantity, "GCT_BYBIT_LIVE_CREATE_ORDER quantity must be positive")
		require.Positive(t, liveConfig.Order.Price, "GCT_BYBIT_LIVE_CREATE_ORDER price must be positive")
		require.False(t, liveConfig.Order.EnableBorrow, "GCT_BYBIT_LIVE_CREATE_ORDER must not borrow")
		require.Zero(t, liveConfig.Order.IsLeverage, "GCT_BYBIT_LIVE_CREATE_ORDER must not use leverage")
		require.Empty(t, liveConfig.Order.OrderLinkID, "GCT_BYBIT_LIVE_CREATE_ORDER client order ID must remain empty for cleanup correlation")
		liveConfig.Order.OrderLinkID = fmt.Sprintf("gct-%d", time.Now().UnixNano())
		ex := getWebsocketInstance(t)
		cancelRequest := CancelOrderRequest{Category: liveConfig.Order.Category, Symbol: liveConfig.Order.Symbol, OrderLinkID: liveConfig.Order.OrderLinkID, OrderFilter: liveConfig.Order.OrderFilter}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var cleanupErr error
			consecutiveAbsent := 0
			for attempt := range 10 {
				_, cancelErr := ex.WSCancelOrder(ctx, &cancelRequest)
				openOrders, listErr := ex.GetOpenOrders(ctx, liveConfig.Order.Category, liveConfig.Order.Symbol.String(), "", "", "", liveConfig.Order.OrderLinkID, liveConfig.Order.OrderFilter, "", 0, 1)
				if listErr == nil && (openOrders == nil || len(openOrders.List) == 0) {
					consecutiveAbsent++
					if consecutiveAbsent >= 2 {
						return
					}
				} else {
					consecutiveAbsent = 0
				}
				cleanupErr = errors.Join(cleanupErr, listErr, cancelErr)
				if attempt < 9 {
					timer := time.NewTimer(500 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						assert.Failf(t, "WSCreateOrder live cleanup should not time out while reconciling the test-owned order", "cleanup errors: %v", errors.Join(cleanupErr, ctx.Err()))
						return
					case <-timer.C:
					}
				}
			}
			assert.Failf(t, "WSCreateOrder live cleanup should confirm cancellation of the test-owned order", "cleanup errors: %v", cleanupErr)
		})
		got, err := ex.WSCreateOrder(t.Context(), &liveConfig.Order)
		require.NoError(t, err, "WSCreateOrder must not error against the live API")
		require.NotEmpty(t, got, "WSCreateOrder must return live order details")
	})
}

func TestWebsocketSubmitOrder(t *testing.T) {
	t.Parallel()

	// Test quote amount needs to be used due to protocol trade requirements
	s := &order.Submit{
		Exchange:  e.Name,
		Pair:      currency.NewBTCUSDT(),
		AssetType: asset.Spot,
		Side:      order.Buy,
		Type:      order.Market,
		Amount:    0.0001,
	}

	_, err := e.WebsocketSubmitOrder(t.Context(), s)
	require.ErrorIs(t, err, order.ErrAmountMustBeSet)

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		if mockTests {
			t.Skip("live testing disabled; run with -tags=mock_test_off to enable")
		}
		value := os.Getenv("GCT_BYBIT_LIVE_CREATE_ORDER")
		if value == "" {
			t.Skip("GCT_BYBIT_LIVE_CREATE_ORDER is unset")
		}
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, true)
		var liveConfig struct {
			DedicatedTestAccount bool              `json:"dedicated_test_account"`
			Order                PlaceOrderRequest `json:"order"`
		}
		require.NoError(t, json.Unmarshal([]byte(value), &liveConfig), "GCT_BYBIT_LIVE_CREATE_ORDER must contain valid JSON")
		require.True(t, liveConfig.DedicatedTestAccount, "GCT_BYBIT_LIVE_CREATE_ORDER must set dedicated_test_account=true")
		require.Equal(t, cSpot, liveConfig.Order.Category, "GCT_BYBIT_LIVE_CREATE_ORDER must use a spot order")
		require.Equal(t, sideBuy, liveConfig.Order.Side, "GCT_BYBIT_LIVE_CREATE_ORDER must use a buy order")
		require.Equal(t, "Limit", liveConfig.Order.OrderType, "GCT_BYBIT_LIVE_CREATE_ORDER must use a limit order")
		require.Equal(t, "PostOnly", liveConfig.Order.TimeInForce, "GCT_BYBIT_LIVE_CREATE_ORDER must use a post-only order")
		require.Positive(t, liveConfig.Order.OrderQuantity, "GCT_BYBIT_LIVE_CREATE_ORDER quantity must be positive")
		require.Positive(t, liveConfig.Order.Price, "GCT_BYBIT_LIVE_CREATE_ORDER price must be positive")
		require.False(t, liveConfig.Order.EnableBorrow, "GCT_BYBIT_LIVE_CREATE_ORDER must not borrow")
		require.Zero(t, liveConfig.Order.IsLeverage, "GCT_BYBIT_LIVE_CREATE_ORDER must not use leverage")
		require.Empty(t, liveConfig.Order.OrderLinkID, "GCT_BYBIT_LIVE_CREATE_ORDER client order ID must remain empty for cleanup correlation")
		clientOrderID := fmt.Sprintf("gct-%d", time.Now().UnixNano())
		ex := getWebsocketInstance(t)
		cancelRequest := CancelOrderRequest{Category: liveConfig.Order.Category, Symbol: liveConfig.Order.Symbol, OrderLinkID: clientOrderID, OrderFilter: liveConfig.Order.OrderFilter}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var cleanupErr error
			consecutiveAbsent := 0
			for attempt := range 10 {
				_, cancelErr := ex.WSCancelOrder(ctx, &cancelRequest)
				openOrders, listErr := ex.GetOpenOrders(ctx, liveConfig.Order.Category, liveConfig.Order.Symbol.String(), "", "", "", clientOrderID, liveConfig.Order.OrderFilter, "", 0, 1)
				if listErr == nil && (openOrders == nil || len(openOrders.List) == 0) {
					consecutiveAbsent++
					if consecutiveAbsent >= 2 {
						return
					}
				} else {
					consecutiveAbsent = 0
				}
				cleanupErr = errors.Join(cleanupErr, listErr, cancelErr)
				if attempt < 9 {
					timer := time.NewTimer(500 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						assert.Failf(t, "WebsocketSubmitOrder live cleanup should not time out while reconciling the test-owned order", "cleanup errors: %v", errors.Join(cleanupErr, ctx.Err()))
						return
					case <-timer.C:
					}
				}
			}
			assert.Failf(t, "WebsocketSubmitOrder live cleanup should confirm cancellation of the test-owned order", "cleanup errors: %v", cleanupErr)
		})
		got, err := ex.WebsocketSubmitOrder(t.Context(), &order.Submit{
			Exchange:      e.Name,
			Pair:          liveConfig.Order.Symbol,
			AssetType:     asset.Spot,
			Side:          order.Buy,
			Type:          order.Limit,
			TimeInForce:   order.PostOnly,
			Amount:        liveConfig.Order.OrderQuantity,
			Price:         liveConfig.Order.Price,
			ClientOrderID: clientOrderID,
		})
		require.NoError(t, err, "WebsocketSubmitOrder must not error against the live API")
		require.NotEmpty(t, got, "WebsocketSubmitOrder must return live order details")
	})
}

func TestWSAmendOrder(t *testing.T) {
	t.Parallel()
	_, err := e.WSAmendOrder(t.Context(), nil)
	require.ErrorIs(t, err, errNilArgument, "WSAmendOrder must reject nil input")
	_, _, err = prepareWSAmendOrder(nil)
	require.ErrorIs(t, err, errNilArgument, "prepareWSAmendOrder must reject nil input")

	arg := &AmendOrderRequest{}
	_, _, err = prepareWSAmendOrder(arg)
	require.ErrorIs(t, err, errCategoryNotSet)
	require.NotErrorIs(t, err, errUnknownCategory, "prepareWSAmendOrder must return validation errors before rate-limit errors")

	arg.Category = cSpot
	_, _, err = prepareWSAmendOrder(arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = currency.NewBTCUSDT()
	_, _, err = prepareWSAmendOrder(arg)
	require.ErrorIs(t, err, errEitherOrderIDOROrderLinkIDRequired)
	arg.OrderID = websocketTestOrderID
	arg.OrderQuantity = 0.0002
	original := *arg

	_, err = e.WSAmendOrder(t.Context(), arg)
	require.ErrorIs(t, err, websocket.ErrNotConnected, "WSAmendOrder must return a disconnected manager error")
	require.Equal(t, original, *arg, "WSAmendOrder must not mutate the caller request")

	wireArgument, limit, err := prepareWSAmendOrder(arg)
	require.NoError(t, err, "prepareWSAmendOrder must prepare a valid request")
	assert.Equal(t, wsOrderSpotEPL, limit, "WSAmendOrder rate limit should match")
	assert.Empty(t, wireArgument.OrderLinkID, "WSAmendOrder should not fabricate an order link ID")
	wireJSON, err := json.Marshal(wireArgument)
	require.NoError(t, err, "WSAmendOrder wire request must encode")
	require.JSONEq(t, `{"category":"spot","symbol":"BTCUSDT","orderId":"order-id","qty":"0.0002"}`, string(wireJSON), "WSAmendOrder wire request must match")
	require.Equal(t, original, *arg, "prepareWSAmendOrder must not mutate the caller request")
	zero := 0.0
	clearTakeProfit := *arg
	clearTakeProfit.OrderQuantity = 0
	clearTakeProfit.TakeProfitPrice = &zero
	wireArgument, _, err = prepareWSAmendOrder(&clearTakeProfit)
	require.NoError(t, err, "prepareWSAmendOrder must accept an explicit take-profit cancellation")
	wireJSON, err = json.Marshal(wireArgument)
	require.NoError(t, err, "WSAmendOrder take-profit cancellation must encode")
	require.JSONEq(t, `{"category":"spot","symbol":"BTCUSDT","orderId":"order-id","takeProfit":"0"}`, string(wireJSON), "WSAmendOrder must distinguish an explicit take-profit cancellation from omission")

	providedLinkID := *arg
	providedLinkID.OrderID = ""
	providedLinkID.OrderLinkID = "provided-order-link-id"
	wireArgument, _, err = prepareWSAmendOrder(&providedLinkID)
	require.NoError(t, err, "prepareWSAmendOrder must accept a provided order link ID")
	assert.Empty(t, wireArgument.OrderID, "WSAmendOrder provided-ID wire request should omit the order ID")
	assert.Equal(t, "provided-order-link-id", wireArgument.OrderLinkID, "WSAmendOrder wire request should retain the provided order link ID")
	require.Equal(t, "provided-order-link-id", providedLinkID.OrderLinkID, "prepareWSAmendOrder must not mutate a provided order link ID")

	type tradeRequest struct {
		RequestID string `json:"reqId"`
		Operation string `json:"op"`
		Arguments []struct {
			OrderID     string `json:"orderId"`
			OrderLinkID string `json:"orderLinkId"`
		} `json:"args"`
	}
	requests := make(chan tradeRequest, 1)
	ex := testexch.MockWsInstance[Exchange](t, mockws.CurryWsMockUpgrader(t, func(_ testing.TB, payload []byte, conn *gws.Conn) error {
		var request tradeRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			return err
		}
		if len(request.Arguments) != 1 {
			return fmt.Errorf("expected one websocket trade argument, received %d", len(request.Arguments))
		}
		confirmationResponse := WebsocketConfirmation{
			RequestID: request.RequestID,
			RetMsg:    "OK",
			Operation: request.Operation,
			RequestAcknowledgement: OrderResponse{
				OrderID:     "amended-order-id",
				OrderLinkID: request.Arguments[0].OrderLinkID,
			},
		}
		if request.Arguments[0].OrderID == rejectedWebsocketOrderID {
			confirmationResponse.RetCode = 10404
			confirmationResponse.RetMsg = "bad request"
		}
		confirmation, err := json.Marshal(confirmationResponse)
		if err != nil {
			return err
		}
		if err := conn.WriteMessage(gws.TextMessage, confirmation); err != nil {
			return err
		}
		requests <- request
		return nil
	}))
	t.Cleanup(func() {
		if ex.Websocket.IsConnected() {
			assert.NoError(t, ex.Websocket.Shutdown(), "mock websocket manager shutdown should not error")
		}
	})
	type amendResult struct {
		details *WebsocketOrderDetails
		err     error
	}
	result := make(chan amendResult, 1)
	go func() {
		details, err := ex.WSAmendOrder(t.Context(), arg)
		result <- amendResult{details: details, err: err}
	}()
	sentRequest := <-requests
	assert.Equal(t, "order.amend", sentRequest.Operation, "WSAmendOrder mock operation should match")
	ex.websocketOrderUpdates.publish(&WebsocketOrderDetails{
		OrderID:      sentRequest.Arguments[0].OrderID,
		OrderLinkID:  sentRequest.Arguments[0].OrderLinkID,
		RejectReason: websocketTestNoError,
		OrderStatus:  websocketTestNew,
		Quantity:     types.Number(arg.OrderQuantity),
	})
	amended := <-result
	require.NoError(t, amended.err, "WSAmendOrder must complete against the connected mock websocket transports")
	require.NotNil(t, amended.details, "WSAmendOrder must return confirmed mock order details")
	assert.Equal(t, arg.OrderID, amended.details.OrderID, "WSAmendOrder mock order ID should match")
	assert.Equal(t, arg.OrderQuantity, amended.details.Quantity.Float64(), "WSAmendOrder mock quantity should match the confirmed update")
	require.Equal(t, original, *arg, "WSAmendOrder mock transport must not mutate the caller request")
	rejectedArg := *arg
	rejectedArg.OrderID = rejectedWebsocketOrderID
	_, err = ex.WSAmendOrder(t.Context(), &rejectedArg)
	require.ErrorContains(t, err, "code:10404", "WSAmendOrder must return rejected trade acknowledgements")
	<-requests

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		if mockTests {
			t.Skip("live testing disabled; run with -tags=mock_test_off to enable")
		}
		value := os.Getenv("GCT_BYBIT_LIVE_AMEND_ORDER")
		if value == "" {
			t.Skip("GCT_BYBIT_LIVE_AMEND_ORDER is unset")
		}
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, true)
		var liveConfig struct {
			DedicatedTestAccount bool              `json:"dedicated_test_account"`
			Order                PlaceOrderRequest `json:"order"`
			Change               AmendOrderRequest `json:"change"`
		}
		require.NoError(t, json.Unmarshal([]byte(value), &liveConfig), "GCT_BYBIT_LIVE_AMEND_ORDER must contain valid JSON")
		require.True(t, liveConfig.DedicatedTestAccount, "GCT_BYBIT_LIVE_AMEND_ORDER must set dedicated_test_account=true")
		require.Equal(t, cSpot, liveConfig.Order.Category, "GCT_BYBIT_LIVE_AMEND_ORDER must use a spot order")
		require.Equal(t, "Limit", liveConfig.Order.OrderType, "GCT_BYBIT_LIVE_AMEND_ORDER must use a limit order")
		require.Equal(t, "PostOnly", liveConfig.Order.TimeInForce, "GCT_BYBIT_LIVE_AMEND_ORDER must use a post-only order")
		require.Positive(t, liveConfig.Order.OrderQuantity, "GCT_BYBIT_LIVE_AMEND_ORDER quantity must be positive")
		require.Positive(t, liveConfig.Order.Price, "GCT_BYBIT_LIVE_AMEND_ORDER price must be positive")
		require.False(t, liveConfig.Order.EnableBorrow, "GCT_BYBIT_LIVE_AMEND_ORDER must not borrow")
		require.Zero(t, liveConfig.Order.IsLeverage, "GCT_BYBIT_LIVE_AMEND_ORDER must not use leverage")
		require.Empty(t, liveConfig.Order.OrderLinkID, "GCT_BYBIT_LIVE_AMEND_ORDER client order ID must remain empty for cleanup correlation")
		require.NotEqual(t, liveConfig.Change.OrderQuantity != 0, liveConfig.Change.Price != 0, "GCT_BYBIT_LIVE_AMEND_ORDER change must set exactly one of quantity or price")
		liveConfig.Order.OrderLinkID = fmt.Sprintf("gct-%d", time.Now().UnixNano())
		liveConfig.Change.Category = liveConfig.Order.Category
		liveConfig.Change.Symbol = liveConfig.Order.Symbol
		liveConfig.Change.OrderID = ""
		liveConfig.Change.OrderLinkID = liveConfig.Order.OrderLinkID
		ex := getWebsocketInstance(t)
		cancelRequest := CancelOrderRequest{Category: liveConfig.Order.Category, Symbol: liveConfig.Order.Symbol, OrderLinkID: liveConfig.Order.OrderLinkID, OrderFilter: liveConfig.Order.OrderFilter}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var cleanupErr error
			consecutiveAbsent := 0
			for attempt := range 10 {
				_, cancelErr := ex.WSCancelOrder(ctx, &cancelRequest)
				openOrders, listErr := ex.GetOpenOrders(ctx, liveConfig.Order.Category, liveConfig.Order.Symbol.String(), "", "", "", liveConfig.Order.OrderLinkID, liveConfig.Order.OrderFilter, "", 0, 1)
				if listErr == nil && (openOrders == nil || len(openOrders.List) == 0) {
					consecutiveAbsent++
					if consecutiveAbsent >= 2 {
						return
					}
				} else {
					consecutiveAbsent = 0
				}
				cleanupErr = errors.Join(cleanupErr, listErr, cancelErr)
				if attempt < 9 {
					timer := time.NewTimer(500 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						assert.Failf(t, "WSAmendOrder live cleanup should not time out while reconciling the test-owned order", "cleanup errors: %v", errors.Join(cleanupErr, ctx.Err()))
						return
					case <-timer.C:
					}
				}
			}
			assert.Failf(t, "WSAmendOrder live cleanup should confirm cancellation of the test-owned order", "cleanup errors: %v", cleanupErr)
		})
		created, err := ex.WSCreateOrder(t.Context(), &liveConfig.Order)
		require.NoError(t, err, "WSCreateOrder must create the test-owned live amendment fixture")
		require.NotNil(t, created, "WSCreateOrder must return the test-owned live amendment fixture")
		require.NotEmpty(t, created.OrderID, "WSCreateOrder must return the test-owned live amendment order ID")
		got, err := ex.WSAmendOrder(t.Context(), &liveConfig.Change)
		require.NoError(t, err, "WSAmendOrder must amend the test-owned live fixture")
		require.NotNil(t, got, "WSAmendOrder must return the amended test-owned fixture")
		assert.Equal(t, created.OrderID, got.OrderID, "WSAmendOrder acknowledgement should identify the test-owned order")
	})
}

func TestWebsocketModifyOrder(t *testing.T) {
	t.Parallel()
	_, err := e.WebsocketModifyOrder(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrModifyOrderIsNil, "WebsocketModifyOrder must reject nil input")
	_, err = e.WebsocketModifyOrder(t.Context(), new(order.Modify))
	require.ErrorIs(t, err, order.ErrPairIsEmpty, "WebsocketModifyOrder must return non-nil request validation errors")

	mod := &order.Modify{
		Exchange:      e.Name,
		Pair:          currency.NewBTCUSDT(),
		AssetType:     asset.Spot,
		Amount:        0.0001,
		Price:         2,
		OrderID:       "existing-order-id",
		ClientOrderID: "client-order-id",
	}
	_, err = e.WebsocketModifyOrder(t.Context(), mod)
	require.ErrorIs(t, err, websocket.ErrNotConnected, "WebsocketModifyOrder must return a disconnected manager error")

	type tradeRequest struct {
		RequestID string `json:"reqId"`
		Operation string `json:"op"`
		Arguments []struct {
			OrderID     string `json:"orderId"`
			OrderLinkID string `json:"orderLinkId"`
		} `json:"args"`
	}
	requests := make(chan tradeRequest, 1)
	ex := testexch.MockWsInstance[Exchange](t, mockws.CurryWsMockUpgrader(t, func(_ testing.TB, payload []byte, conn *gws.Conn) error {
		var request tradeRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			return err
		}
		if len(request.Arguments) != 1 {
			return fmt.Errorf("expected one websocket trade argument, received %d", len(request.Arguments))
		}
		confirmationResponse := WebsocketConfirmation{
			RequestID: request.RequestID,
			RetMsg:    "OK",
			Operation: request.Operation,
			RequestAcknowledgement: OrderResponse{
				OrderID:     "amended-order-id",
				OrderLinkID: request.Arguments[0].OrderLinkID,
			},
		}
		if request.Arguments[0].OrderID == rejectedWebsocketOrderID {
			confirmationResponse.RetCode = 10404
			confirmationResponse.RetMsg = "bad request"
		}
		confirmation, err := json.Marshal(confirmationResponse)
		if err != nil {
			return err
		}
		if err := conn.WriteMessage(gws.TextMessage, confirmation); err != nil {
			return err
		}
		requests <- request
		return nil
	}))
	t.Cleanup(func() {
		if ex.Websocket.IsConnected() {
			assert.NoError(t, ex.Websocket.Shutdown(), "mock websocket manager shutdown should not error")
		}
	})
	type modifyResult struct {
		response *order.ModifyResponse
		err      error
	}
	result := make(chan modifyResult, 1)
	go func() {
		response, err := ex.WebsocketModifyOrder(t.Context(), mod)
		result <- modifyResult{response: response, err: err}
	}()
	sentRequest := <-requests
	assert.Equal(t, "order.amend", sentRequest.Operation, "WebsocketModifyOrder operation should match")
	require.Len(t, sentRequest.Arguments, 1, "WebsocketModifyOrder request must contain one argument")
	assert.Equal(t, mod.OrderID, sentRequest.Arguments[0].OrderID, "WebsocketModifyOrder order ID wire value should match")
	assert.Equal(t, mod.ClientOrderID, sentRequest.Arguments[0].OrderLinkID, "WebsocketModifyOrder client order ID wire value should match")
	ex.websocketOrderUpdates.publish(&WebsocketOrderDetails{
		OrderID:      sentRequest.Arguments[0].OrderID,
		OrderLinkID:  sentRequest.Arguments[0].OrderLinkID,
		RejectReason: websocketTestNoError,
		OrderStatus:  websocketTestNew,
		Quantity:     types.Number(mod.Amount),
		Price:        types.Number(mod.Price),
	})
	modified := <-result
	require.NoError(t, modified.err, "WebsocketModifyOrder must complete against the connected mock websocket transports")
	require.NotNil(t, modified.response, "WebsocketModifyOrder must return a modify response")
	assert.Equal(t, mod.OrderID, modified.response.OrderID, "WebsocketModifyOrder order ID should match the confirmed update")
	assert.Equal(t, mod.ClientOrderID, modified.response.ClientOrderID, "WebsocketModifyOrder client order ID should match")
	assert.Equal(t, mod.Amount, modified.response.Amount, "WebsocketModifyOrder should preserve the requested amount")
	assert.Equal(t, mod.Price, modified.response.Price, "WebsocketModifyOrder should preserve the requested price")
	rejectedMod := *mod
	rejectedMod.OrderID = rejectedWebsocketOrderID
	_, err = ex.WebsocketModifyOrder(t.Context(), &rejectedMod)
	require.ErrorContains(t, err, "code:10404", "WebsocketModifyOrder must return rejected trade acknowledgements")
	rejectedRequest := <-requests
	assert.Equal(t, rejectedWebsocketOrderID, rejectedRequest.Arguments[0].OrderID, "WebsocketModifyOrder rejected request order ID should match")

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		if mockTests {
			t.Skip("live testing disabled; run with -tags=mock_test_off to enable")
		}
		value := os.Getenv("GCT_BYBIT_LIVE_AMEND_ORDER")
		if value == "" {
			t.Skip("GCT_BYBIT_LIVE_AMEND_ORDER is unset")
		}
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, true)
		var liveConfig struct {
			DedicatedTestAccount bool              `json:"dedicated_test_account"`
			Order                PlaceOrderRequest `json:"order"`
			Change               AmendOrderRequest `json:"change"`
		}
		require.NoError(t, json.Unmarshal([]byte(value), &liveConfig), "GCT_BYBIT_LIVE_AMEND_ORDER must contain valid JSON")
		require.True(t, liveConfig.DedicatedTestAccount, "GCT_BYBIT_LIVE_AMEND_ORDER must set dedicated_test_account=true")
		require.Equal(t, cSpot, liveConfig.Order.Category, "GCT_BYBIT_LIVE_AMEND_ORDER must use a spot order")
		require.Equal(t, "Limit", liveConfig.Order.OrderType, "GCT_BYBIT_LIVE_AMEND_ORDER must use a limit order")
		require.Equal(t, "PostOnly", liveConfig.Order.TimeInForce, "GCT_BYBIT_LIVE_AMEND_ORDER must use a post-only order")
		require.Positive(t, liveConfig.Order.OrderQuantity, "GCT_BYBIT_LIVE_AMEND_ORDER quantity must be positive")
		require.Positive(t, liveConfig.Order.Price, "GCT_BYBIT_LIVE_AMEND_ORDER price must be positive")
		require.False(t, liveConfig.Order.EnableBorrow, "GCT_BYBIT_LIVE_AMEND_ORDER must not borrow")
		require.Zero(t, liveConfig.Order.IsLeverage, "GCT_BYBIT_LIVE_AMEND_ORDER must not use leverage")
		require.Empty(t, liveConfig.Order.OrderLinkID, "GCT_BYBIT_LIVE_AMEND_ORDER client order ID must remain empty for cleanup correlation")
		require.NotEqual(t, liveConfig.Change.OrderQuantity != 0, liveConfig.Change.Price != 0, "GCT_BYBIT_LIVE_AMEND_ORDER change must set exactly one of quantity or price")
		liveConfig.Order.OrderLinkID = fmt.Sprintf("gct-%d", time.Now().UnixNano())
		ex := getWebsocketInstance(t)
		cancelRequest := CancelOrderRequest{Category: liveConfig.Order.Category, Symbol: liveConfig.Order.Symbol, OrderLinkID: liveConfig.Order.OrderLinkID, OrderFilter: liveConfig.Order.OrderFilter}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var cleanupErr error
			consecutiveAbsent := 0
			for attempt := range 10 {
				_, cancelErr := ex.WSCancelOrder(ctx, &cancelRequest)
				openOrders, listErr := ex.GetOpenOrders(ctx, liveConfig.Order.Category, liveConfig.Order.Symbol.String(), "", "", "", liveConfig.Order.OrderLinkID, liveConfig.Order.OrderFilter, "", 0, 1)
				if listErr == nil && (openOrders == nil || len(openOrders.List) == 0) {
					consecutiveAbsent++
					if consecutiveAbsent >= 2 {
						return
					}
				} else {
					consecutiveAbsent = 0
				}
				cleanupErr = errors.Join(cleanupErr, listErr, cancelErr)
				if attempt < 9 {
					timer := time.NewTimer(500 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						assert.Failf(t, "WebsocketModifyOrder live cleanup should not time out while reconciling the test-owned order", "cleanup errors: %v", errors.Join(cleanupErr, ctx.Err()))
						return
					case <-timer.C:
					}
				}
			}
			assert.Failf(t, "WebsocketModifyOrder live cleanup should confirm cancellation of the test-owned order", "cleanup errors: %v", cleanupErr)
		})
		created, err := ex.WSCreateOrder(t.Context(), &liveConfig.Order)
		require.NoError(t, err, "WSCreateOrder must create the test-owned live modify fixture")
		require.NotNil(t, created, "WSCreateOrder must return the test-owned live modify fixture")
		require.NotEmpty(t, created.OrderID, "WSCreateOrder must return the test-owned live modify order ID")
		got, err := ex.WebsocketModifyOrder(t.Context(), &order.Modify{
			Exchange:      ex.Name,
			Pair:          liveConfig.Order.Symbol,
			AssetType:     asset.Spot,
			Amount:        liveConfig.Change.OrderQuantity,
			Price:         liveConfig.Change.Price,
			ClientOrderID: liveConfig.Order.OrderLinkID,
		})
		require.NoError(t, err, "WebsocketModifyOrder must amend the test-owned live fixture")
		require.NotNil(t, got, "WebsocketModifyOrder must return the amended test-owned fixture")
		assert.Equal(t, created.OrderID, got.OrderID, "WebsocketModifyOrder acknowledgement should identify the test-owned order")
	})
}

func TestWSCancelOrder(t *testing.T) {
	t.Parallel()
	_, err := e.WSCancelOrder(t.Context(), nil)
	require.ErrorIs(t, err, errNilArgument, "WSCancelOrder must reject nil input")
	_, _, err = prepareWSCancelOrder(nil)
	require.ErrorIs(t, err, errNilArgument, "prepareWSCancelOrder must reject nil input")

	arg := &CancelOrderRequest{}
	_, _, err = prepareWSCancelOrder(arg)
	require.ErrorIs(t, err, errCategoryNotSet)
	require.NotErrorIs(t, err, errUnknownCategory, "prepareWSCancelOrder must return validation errors before rate-limit errors")

	arg.Category = cSpot
	_, _, err = prepareWSCancelOrder(arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = currency.NewBTCUSDT()
	_, _, err = prepareWSCancelOrder(arg)
	require.ErrorIs(t, err, errEitherOrderIDOROrderLinkIDRequired)

	arg.OrderID = "1793353687809485568" // Replace with a valid order ID

	arg.OrderFilter = "dodgy"
	_, _, err = prepareWSCancelOrder(arg)
	require.ErrorIs(t, err, errInvalidOrderFilter)

	arg.Category = cLinear
	_, _, err = prepareWSCancelOrder(arg)
	require.ErrorIs(t, err, errInvalidCategory)

	arg.Category = cSpot
	arg.OrderFilter = "Order"
	original := *arg

	_, err = e.WSCancelOrder(t.Context(), arg)
	require.ErrorIs(t, err, websocket.ErrNotConnected, "WSCancelOrder must return a disconnected manager error")
	require.Equal(t, original, *arg, "WSCancelOrder must not mutate the caller request")

	wireArgument, limit, err := prepareWSCancelOrder(arg)
	require.NoError(t, err, "prepareWSCancelOrder must prepare a valid request")
	assert.Equal(t, wsOrderSpotEPL, limit, "WSCancelOrder rate limit should match")
	assert.Empty(t, wireArgument.OrderLinkID, "WSCancelOrder should not fabricate an order link ID")
	wireJSON, err := json.Marshal(wireArgument)
	require.NoError(t, err, "WSCancelOrder wire request must encode")
	require.JSONEq(t, `{"category":"spot","symbol":"BTCUSDT","orderId":"1793353687809485568","orderFilter":"Order"}`, string(wireJSON), "WSCancelOrder wire request must match")
	require.Equal(t, original, *arg, "prepareWSCancelOrder must not mutate the caller request")

	providedLinkID := *arg
	providedLinkID.OrderID = ""
	providedLinkID.OrderLinkID = "provided-order-link-id"
	wireArgument, _, err = prepareWSCancelOrder(&providedLinkID)
	require.NoError(t, err, "prepareWSCancelOrder must accept a provided order link ID")
	assert.Empty(t, wireArgument.OrderID, "WSCancelOrder provided-ID wire request should omit the order ID")
	assert.Equal(t, "provided-order-link-id", wireArgument.OrderLinkID, "WSCancelOrder wire request should retain the provided order link ID")
	require.Equal(t, "provided-order-link-id", providedLinkID.OrderLinkID, "prepareWSCancelOrder must not mutate a provided order link ID")

	type tradeRequest struct {
		RequestID string `json:"reqId"`
		Operation string `json:"op"`
		Arguments []struct {
			OrderID     string `json:"orderId"`
			OrderLinkID string `json:"orderLinkId"`
		} `json:"args"`
	}
	requests := make(chan tradeRequest, 1)
	ex := testexch.MockWsInstance[Exchange](t, mockws.CurryWsMockUpgrader(t, func(_ testing.TB, payload []byte, conn *gws.Conn) error {
		var request tradeRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			return err
		}
		if len(request.Arguments) != 1 {
			return fmt.Errorf("expected one websocket trade argument, received %d", len(request.Arguments))
		}
		confirmationResponse := WebsocketConfirmation{
			RequestID: request.RequestID,
			RetMsg:    "OK",
			Operation: request.Operation,
			RequestAcknowledgement: OrderResponse{
				OrderID:     "cancelled-order-id",
				OrderLinkID: request.Arguments[0].OrderLinkID,
			},
		}
		if request.Arguments[0].OrderID == rejectedWebsocketOrderID {
			confirmationResponse.RetCode = 10404
			confirmationResponse.RetMsg = "bad request"
		}
		confirmation, err := json.Marshal(confirmationResponse)
		if err != nil {
			return err
		}
		if err := conn.WriteMessage(gws.TextMessage, confirmation); err != nil {
			return err
		}
		requests <- request
		return nil
	}))
	t.Cleanup(func() {
		if ex.Websocket.IsConnected() {
			assert.NoError(t, ex.Websocket.Shutdown(), "mock websocket manager shutdown should not error")
		}
	})
	type cancelResult struct {
		details *WebsocketOrderDetails
		err     error
	}
	result := make(chan cancelResult, 1)
	go func() {
		details, err := ex.WSCancelOrder(t.Context(), arg)
		result <- cancelResult{details: details, err: err}
	}()
	sentRequest := <-requests
	assert.Equal(t, "order.cancel", sentRequest.Operation, "WSCancelOrder mock operation should match")
	ex.websocketOrderUpdates.publish(&WebsocketOrderDetails{
		OrderID:      sentRequest.Arguments[0].OrderID,
		OrderLinkID:  sentRequest.Arguments[0].OrderLinkID,
		RejectReason: websocketTestNoError,
		OrderStatus:  websocketTestCancelled,
	})
	cancelled := <-result
	require.NoError(t, cancelled.err, "WSCancelOrder must complete against the connected mock websocket transports")
	require.NotNil(t, cancelled.details, "WSCancelOrder must return confirmed mock order details")
	assert.Equal(t, arg.OrderID, cancelled.details.OrderID, "WSCancelOrder mock order ID should match")
	assert.Equal(t, websocketTestCancelled, cancelled.details.OrderStatus, "WSCancelOrder mock status should match the confirmed update")
	require.Equal(t, original, *arg, "WSCancelOrder mock transport must not mutate the caller request")
	rejectedArg := *arg
	rejectedArg.OrderID = rejectedWebsocketOrderID
	_, err = ex.WSCancelOrder(t.Context(), &rejectedArg)
	require.ErrorContains(t, err, "code:10404", "WSCancelOrder must return rejected trade acknowledgements")
	<-requests

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		if mockTests {
			t.Skip("live testing disabled; run with -tags=mock_test_off to enable")
		}
		value := os.Getenv("GCT_BYBIT_LIVE_CANCEL_ORDER")
		if value == "" {
			t.Skip("GCT_BYBIT_LIVE_CANCEL_ORDER is unset")
		}
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, true)
		var liveConfig struct {
			DedicatedTestAccount bool              `json:"dedicated_test_account"`
			Order                PlaceOrderRequest `json:"order"`
		}
		require.NoError(t, json.Unmarshal([]byte(value), &liveConfig), "GCT_BYBIT_LIVE_CANCEL_ORDER must contain valid JSON")
		require.True(t, liveConfig.DedicatedTestAccount, "GCT_BYBIT_LIVE_CANCEL_ORDER must set dedicated_test_account=true")
		require.Equal(t, cSpot, liveConfig.Order.Category, "GCT_BYBIT_LIVE_CANCEL_ORDER must use a spot order")
		require.Equal(t, "Limit", liveConfig.Order.OrderType, "GCT_BYBIT_LIVE_CANCEL_ORDER must use a limit order")
		require.Equal(t, "PostOnly", liveConfig.Order.TimeInForce, "GCT_BYBIT_LIVE_CANCEL_ORDER must use a post-only order")
		require.Positive(t, liveConfig.Order.OrderQuantity, "GCT_BYBIT_LIVE_CANCEL_ORDER quantity must be positive")
		require.Positive(t, liveConfig.Order.Price, "GCT_BYBIT_LIVE_CANCEL_ORDER price must be positive")
		require.False(t, liveConfig.Order.EnableBorrow, "GCT_BYBIT_LIVE_CANCEL_ORDER must not borrow")
		require.Zero(t, liveConfig.Order.IsLeverage, "GCT_BYBIT_LIVE_CANCEL_ORDER must not use leverage")
		require.Empty(t, liveConfig.Order.OrderLinkID, "GCT_BYBIT_LIVE_CANCEL_ORDER client order ID must remain empty for cleanup correlation")
		liveConfig.Order.OrderLinkID = fmt.Sprintf("gct-%d", time.Now().UnixNano())
		ex := getWebsocketInstance(t)
		cancelRequest := CancelOrderRequest{Category: liveConfig.Order.Category, Symbol: liveConfig.Order.Symbol, OrderLinkID: liveConfig.Order.OrderLinkID, OrderFilter: liveConfig.Order.OrderFilter}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var cleanupErr error
			consecutiveAbsent := 0
			for attempt := range 10 {
				_, cancelErr := ex.WSCancelOrder(ctx, &cancelRequest)
				openOrders, listErr := ex.GetOpenOrders(ctx, liveConfig.Order.Category, liveConfig.Order.Symbol.String(), "", "", "", liveConfig.Order.OrderLinkID, liveConfig.Order.OrderFilter, "", 0, 1)
				if listErr == nil && (openOrders == nil || len(openOrders.List) == 0) {
					consecutiveAbsent++
					if consecutiveAbsent >= 2 {
						return
					}
				} else {
					consecutiveAbsent = 0
				}
				cleanupErr = errors.Join(cleanupErr, listErr, cancelErr)
				if attempt < 9 {
					timer := time.NewTimer(500 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						assert.Failf(t, "WSCancelOrder live cleanup should not time out while reconciling the test-owned order", "cleanup errors: %v", errors.Join(cleanupErr, ctx.Err()))
						return
					case <-timer.C:
					}
				}
			}
			assert.Failf(t, "WSCancelOrder live cleanup should confirm cancellation of the test-owned order", "cleanup errors: %v", cleanupErr)
		})
		created, err := ex.WSCreateOrder(t.Context(), &liveConfig.Order)
		require.NoError(t, err, "WSCreateOrder must create the test-owned live cancellation fixture")
		require.NotNil(t, created, "WSCreateOrder must return the test-owned live cancellation fixture")
		require.NotEmpty(t, created.OrderID, "WSCreateOrder must return the test-owned live cancellation order ID")
		got, err := ex.WSCancelOrder(t.Context(), &cancelRequest)
		require.NoError(t, err, "WSCancelOrder must cancel the test-owned live fixture")
		require.NotNil(t, got, "WSCancelOrder must return the cancelled test-owned fixture")
		assert.Equal(t, created.OrderID, got.OrderID, "WSCancelOrder acknowledgement should identify the test-owned order")
	})
}

func TestWebsocketCancelOrder(t *testing.T) {
	t.Parallel()
	cancel := &order.Cancel{
		OrderID:   "1793388409122024192", // Replace with a valid order ID
		Pair:      currency.NewBTCUSDT(),
		AssetType: asset.Spot,
	}

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	e := getWebsocketInstance(t)

	err := e.WebsocketCancelOrder(t.Context(), cancel)
	require.NoError(t, err)
}

// getWebsocketInstance returns a websocket instance copy for live bi-directional testing
func getWebsocketInstance(t *testing.T) *Exchange {
	t.Helper()
	cfg := &config.Config{}
	root, err := testutils.RootPathFromCWD()
	require.NoError(t, err)

	err = cfg.LoadConfig(filepath.Join(root, "testdata", "configtest.json"), true)
	require.NoError(t, err)

	pairs := &e.CurrencyPairs
	e := new(Exchange)
	e.SetDefaults()
	bConf, err := cfg.GetExchangeConfig("Bybit")
	require.NoError(t, err)
	bConf.API.AuthenticatedSupport = true
	bConf.API.AuthenticatedWebsocketSupport = true
	bConf.API.Credentials.Key = apiCredentials.Key
	bConf.API.Credentials.Secret = apiCredentials.Secret

	require.NoError(t, e.Setup(bConf), "Setup must not error")
	e.CurrencyPairs.Load(pairs)
	require.NoError(t, e.Websocket.Connect(t.Context()))
	return e
}
