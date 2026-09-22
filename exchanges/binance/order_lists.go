package binance

import (
	"context"
	"net/http"
	"net/url"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

// OPOOrderRequest contains the parameters for a OPO order list.
type OPOOrderRequest struct {
	Symbol                  currency.Pair `json:"symbol"`
	WorkingType             string        `json:"workingType"`
	WorkingSide             string        `json:"workingSide"`
	WorkingPrice            float64       `json:"workingPrice"`
	WorkingQuantity         float64       `json:"workingQuantity"`
	PendingType             string        `json:"pendingType"`
	PendingSide             string        `json:"pendingSide"`
	ListClientOrderID       string        `json:"listClientOrderId,omitempty"`
	NewOrderRespType        string        `json:"newOrderRespType,omitempty"`
	SelfTradePreventionMode string        `json:"selfTradePreventionMode,omitempty"`
	WorkingClientOrderID    string        `json:"workingClientOrderId,omitempty"`
	WorkingIcebergQuantity  float64       `json:"workingIcebergQty,omitempty"`
	WorkingTimeInForce      string        `json:"workingTimeInForce,omitempty"`
	WorkingStrategyID       uint64        `json:"workingStrategyId,omitempty"`
	WorkingStrategyType     uint64        `json:"workingStrategyType,omitempty"`
	WorkingPegPriceType     string        `json:"workingPegPriceType,omitempty"`
	WorkingPegOffsetType    string        `json:"workingPegOffsetType,omitempty"`
	WorkingPegOffsetValue   uint64        `json:"workingPegOffsetValue,omitempty"`
	PendingClientOrderID    string        `json:"pendingClientOrderId,omitempty"`
	PendingPrice            float64       `json:"pendingPrice,omitempty"`
	PendingStopPrice        float64       `json:"pendingStopPrice,omitempty"`
	PendingTrailingDelta    float64       `json:"pendingTrailingDelta,omitempty"`
	PendingIcebergQuantity  float64       `json:"pendingIcebergQty,omitempty"`
	PendingTimeInForce      string        `json:"pendingTimeInForce,omitempty"`
	PendingStrategyID       uint64        `json:"pendingStrategyId,omitempty"`
	PendingStrategyType     uint64        `json:"pendingStrategyType,omitempty"`
	PendingPegPriceType     string        `json:"pendingPegPriceType,omitempty"`
	PendingPegOffsetType    string        `json:"pendingPegOffsetType,omitempty"`
	PendingPegOffsetValue   uint64        `json:"pendingPegOffsetValue,omitempty"`
	RecvWindow              float64       `json:"recvWindow,omitempty"`
}

// NewOPOOrderList places a working order with pending orders activated by partial fills.
func (e *Exchange) NewOPOOrderList(ctx context.Context, arg *OPOOrderRequest) (*OCOOrder, error) {
	if err := validateOPOOrder(arg); err != nil {
		return nil, err
	}
	var response *OCOOrder
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/api/v3/orderList/opo", url.Values{symbolParam: {arg.Symbol.Format(currency.PairFormat{Uppercase: true}).String()}}, spotOrderRate, arg, &response)
}

// WsNewOPOOrderList places a OPO order list over the Spot WebSocket API.
func (e *Exchange) WsNewOPOOrderList(arg *OPOOrderRequest) (*OCOOrder, error) {
	if err := validateOPOOrder(arg); err != nil {
		return nil, err
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	params["symbol"] = arg.Symbol.Format(currency.PairFormat{Uppercase: true}).String()
	var response *OCOOrder
	return response, e.sendSignedWsRequest("orderList.place.opo", params, &response)
}

func validateOPOOrder(arg *OPOOrderRequest) error {
	if err := common.NilGuard(arg); err != nil {
		return err
	}
	if arg.Symbol.IsEmpty() {
		return currency.ErrCurrencyPairEmpty
	}
	if arg.WorkingQuantity <= 0 {
		return limits.ErrAmountBelowMin
	}
	if arg.WorkingSide == "" || arg.PendingSide == "" {
		return order.ErrSideIsInvalid
	}
	if arg.WorkingType == "" || arg.PendingType == "" {
		return order.ErrTypeIsInvalid
	}
	return nil
}

// OPOCOOrderRequest contains the parameters for a OPOCO order list.
type OPOCOOrderRequest struct {
	Symbol                      currency.Pair `json:"symbol"`
	WorkingType                 string        `json:"workingType"`
	WorkingSide                 string        `json:"workingSide"`
	WorkingPrice                float64       `json:"workingPrice"`
	WorkingQuantity             float64       `json:"workingQuantity"`
	PendingSide                 string        `json:"pendingSide"`
	PendingAboveType            string        `json:"pendingAboveType"`
	ListClientOrderID           string        `json:"listClientOrderId,omitempty"`
	NewOrderRespType            string        `json:"newOrderRespType,omitempty"`
	SelfTradePreventionMode     string        `json:"selfTradePreventionMode,omitempty"`
	WorkingClientOrderID        string        `json:"workingClientOrderId,omitempty"`
	WorkingIcebergQuantity      float64       `json:"workingIcebergQty,omitempty"`
	WorkingTimeInForce          string        `json:"workingTimeInForce,omitempty"`
	WorkingStrategyID           uint64        `json:"workingStrategyId,omitempty"`
	WorkingStrategyType         uint64        `json:"workingStrategyType,omitempty"`
	WorkingPegPriceType         string        `json:"workingPegPriceType,omitempty"`
	WorkingPegOffsetType        string        `json:"workingPegOffsetType,omitempty"`
	WorkingPegOffsetValue       uint64        `json:"workingPegOffsetValue,omitempty"`
	PendingAboveClientOrderID   string        `json:"pendingAboveClientOrderId,omitempty"`
	PendingAbovePrice           float64       `json:"pendingAbovePrice,omitempty"`
	PendingAboveStopPrice       float64       `json:"pendingAboveStopPrice,omitempty"`
	PendingAboveTrailingDelta   float64       `json:"pendingAboveTrailingDelta,omitempty"`
	PendingAboveIcebergQuantity float64       `json:"pendingAboveIcebergQty,omitempty"`
	PendingAboveTimeInForce     string        `json:"pendingAboveTimeInForce,omitempty"`
	PendingAboveStrategyID      uint64        `json:"pendingAboveStrategyId,omitempty"`
	PendingAboveStrategyType    uint64        `json:"pendingAboveStrategyType,omitempty"`
	PendingAbovePegPriceType    string        `json:"pendingAbovePegPriceType,omitempty"`
	PendingAbovePegOffsetType   string        `json:"pendingAbovePegOffsetType,omitempty"`
	PendingAbovePegOffsetValue  uint64        `json:"pendingAbovePegOffsetValue,omitempty"`
	PendingBelowType            string        `json:"pendingBelowType,omitempty"`
	PendingBelowClientOrderID   string        `json:"pendingBelowClientOrderId,omitempty"`
	PendingBelowPrice           float64       `json:"pendingBelowPrice,omitempty"`
	PendingBelowStopPrice       float64       `json:"pendingBelowStopPrice,omitempty"`
	PendingBelowTrailingDelta   float64       `json:"pendingBelowTrailingDelta,omitempty"`
	PendingBelowIcebergQuantity float64       `json:"pendingBelowIcebergQty,omitempty"`
	PendingBelowTimeInForce     string        `json:"pendingBelowTimeInForce,omitempty"`
	PendingBelowStrategyID      uint64        `json:"pendingBelowStrategyId,omitempty"`
	PendingBelowStrategyType    uint64        `json:"pendingBelowStrategyType,omitempty"`
	PendingBelowPegPriceType    string        `json:"pendingBelowPegPriceType,omitempty"`
	PendingBelowPegOffsetType   string        `json:"pendingBelowPegOffsetType,omitempty"`
	PendingBelowPegOffsetValue  uint64        `json:"pendingBelowPegOffsetValue,omitempty"`
	RecvWindow                  float64       `json:"recvWindow,omitempty"`
}

// NewOPOCOOrderList places a working order with pending orders activated by partial fills.
func (e *Exchange) NewOPOCOOrderList(ctx context.Context, arg *OPOCOOrderRequest) (*OCOOrder, error) {
	if err := validateOPOCOOrder(arg); err != nil {
		return nil, err
	}
	var response *OCOOrder
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/api/v3/orderList/opoco", url.Values{symbolParam: {arg.Symbol.Format(currency.PairFormat{Uppercase: true}).String()}}, spotOrderRate, arg, &response)
}

// WsNewOPOCOOrderList places a OPOCO order list over the Spot WebSocket API.
func (e *Exchange) WsNewOPOCOOrderList(arg *OPOCOOrderRequest) (*OCOOrder, error) {
	if err := validateOPOCOOrder(arg); err != nil {
		return nil, err
	}
	params, err := e.ToMap(arg)
	if err != nil {
		return nil, err
	}
	params["symbol"] = arg.Symbol.Format(currency.PairFormat{Uppercase: true}).String()
	var response *OCOOrder
	return response, e.sendSignedWsRequest("orderList.place.opoco", params, &response)
}

func validateOPOCOOrder(arg *OPOCOOrderRequest) error {
	if err := common.NilGuard(arg); err != nil {
		return err
	}
	if arg.Symbol.IsEmpty() {
		return currency.ErrCurrencyPairEmpty
	}
	if arg.WorkingQuantity <= 0 {
		return limits.ErrAmountBelowMin
	}
	if arg.WorkingSide == "" || arg.PendingSide == "" {
		return order.ErrSideIsInvalid
	}
	if arg.WorkingType == "" || arg.PendingAboveType == "" {
		return order.ErrTypeIsInvalid
	}
	return nil
}
