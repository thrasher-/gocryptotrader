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
	case krakenWsV2CancelOrder:
		return mockWsCancelOrders(tb, msg, w)
	case krakenWsV2AddOrder:
		return mockWsAddOrder(tb, msg, w)
	case krakenWsV2CancelAll:
		return mockWsCancelAllOrders(tb, msg, w)
	}
	return nil
}

func mockWsCancelAllOrders(tb testing.TB, msg []byte, w *gws.Conn) error {
	tb.Helper()
	var req WebsocketV2Request[WebsocketV2CancelAllParams]
	if err := json.Unmarshal(msg, &req); err != nil {
		return err
	}
	success := true
	resp := websocketV2Response{
		Method:    krakenWsV2CancelAll,
		RequestID: req.RequestID,
		Success:   &success,
		Result:    websocketV2ResponseResult{Count: 3},
	}
	response, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return w.WriteMessage(gws.TextMessage, response)
}

func mockWsCancelOrders(tb testing.TB, msg []byte, w *gws.Conn) error {
	tb.Helper()
	var req WebsocketV2Request[WebsocketV2CancelOrderParams]
	if err := json.Unmarshal(msg, &req); err != nil {
		return err
	}
	for _, orderID := range req.Params.OrderIDs {
		success := !strings.Contains(orderID, "FISH")
		resp := websocketV2Response{
			Method:    krakenWsV2CancelOrder,
			RequestID: req.RequestID,
			Success:   &success,
			Result:    websocketV2ResponseResult{OrderID: orderID},
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
	var req WebsocketV2Request[WebsocketV2AddOrderParams]
	if err := json.Unmarshal(msg, &req); err != nil {
		return err
	}

	assert.Equal(tb, "buy", req.Params.Side, "Side should be correct")
	assert.Equal(tb, "limit", req.Params.OrderType, "OrderType should be correct")
	assert.Equal(tb, "BTC/USD", req.Params.Symbol, "Symbol should be correct")
	assert.Equal(tb, 80000.0, req.Params.LimitPrice, "LimitPrice should be correct")

	success := true
	resp := websocketV2Response{
		Method:    krakenWsV2AddOrder,
		RequestID: req.RequestID,
		Success:   &success,
		Result:    websocketV2ResponseResult{OrderID: "ONPNXH-KMKMU-F4MR5V"},
	}
	response, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return w.WriteMessage(gws.TextMessage, response)
}
