package kraken

import (
	"strings"
	"testing"

	"github.com/buger/jsonparser"
	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

func mockWsServer(tb testing.TB, msg []byte, w *gws.Conn) error {
	tb.Helper()
	method, err := jsonparser.GetUnsafeString(msg, "method")
	if err != nil {
		return err
	}
	switch method {
	case wsCancelOrder:
		return mockWsCancelOrders(tb, msg, w)
	case wsAddOrder:
		return mockWsAddOrder(tb, msg, w)
	case wsCancelAll:
		return mockWsCancelAllOrders(tb, msg, w)
	}
	return nil
}

func mockWsCancelAllOrders(tb testing.TB, msg []byte, w *gws.Conn) error {
	tb.Helper()
	var req WebsocketRequest[WebsocketCancelAllParams]
	if err := json.Unmarshal(msg, &req); err != nil {
		return err
	}
	success := true
	resp := websocketResponse{
		Method:    wsCancelAll,
		RequestID: req.RequestID,
		Success:   &success,
		Result:    websocketResponseResult{Count: 3},
	}
	response, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return w.WriteMessage(gws.TextMessage, response)
}

func mockWsCancelOrders(tb testing.TB, msg []byte, w *gws.Conn) error {
	tb.Helper()
	var req WebsocketRequest[WebsocketCancelOrderParams]
	if err := json.Unmarshal(msg, &req); err != nil {
		return err
	}
	if len(req.Params.OrderIDs) > 0 && req.Params.OrderIDs[0] == "GLOBAL" {
		success := false
		response, err := json.Marshal(websocketResponse{
			Method:    wsCancelOrder,
			RequestID: req.RequestID,
			Success:   &success,
			Error:     "EGeneral:Invalid arguments",
		})
		if err != nil {
			return err
		}
		return w.WriteMessage(gws.TextMessage, response)
	}
	for _, orderID := range req.Params.OrderIDs {
		success := !strings.Contains(orderID, "FISH")
		resp := websocketResponse{
			Method:    wsCancelOrder,
			RequestID: req.RequestID,
			Success:   &success,
			Result:    websocketResponseResult{OrderID: orderID},
		}
		if !success {
			resp.Error = "EOrder:Unknown order"
		}
		response, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if err := w.WriteMessage(gws.TextMessage, response); err != nil {
			return err
		}
	}
	return nil
}

func mockWsAddOrder(tb testing.TB, msg []byte, w *gws.Conn) error {
	tb.Helper()
	var req WebsocketRequest[WebsocketAddOrderParams]
	if err := json.Unmarshal(msg, &req); err != nil {
		return err
	}

	assert.Equal(tb, "buy", req.Params.Side, "req.Params.Side should match")
	assert.Equal(tb, "limit", req.Params.OrderType, "req.Params.OrderType should match")
	assert.Equal(tb, "BTC/USD", req.Params.Symbol, "req.Params.Symbol should match")
	if assert.NotNil(tb, req.Params.LimitPrice, "req.Params.LimitPrice should be present") {
		assert.Equal(tb, 80000.0, *req.Params.LimitPrice, "req.Params.LimitPrice should match")
	}

	success := true
	resp := websocketResponse{
		Method:    wsAddOrder,
		RequestID: req.RequestID,
		Success:   &success,
		Result:    websocketResponseResult{OrderID: "ONPNXH-KMKMU-F4MR5V"},
	}
	response, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return w.WriteMessage(gws.TextMessage, response)
}
