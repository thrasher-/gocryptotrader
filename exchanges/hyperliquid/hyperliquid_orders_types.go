package hyperliquid

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/types"
)

type limitOrderTypeWire struct {
	TimeInForce string `json:"tif" msgpack:"tif"`
}

type triggerOrderTypeWire struct {
	IsMarket           bool   `json:"isMarket"  msgpack:"isMarket"`
	TriggerPrice       string `json:"triggerPx" msgpack:"triggerPx"`
	TakeProfitStopLoss string `json:"tpsl"      msgpack:"tpsl"`
}

type orderTypeWire struct {
	Limit   *limitOrderTypeWire   `json:"limit,omitempty"   msgpack:"limit,omitempty"`
	Trigger *triggerOrderTypeWire `json:"trigger,omitempty" msgpack:"trigger,omitempty"`
}

type orderWire struct {
	AssetID       uint64        `json:"a"           msgpack:"a"`
	IsBuy         bool          `json:"b"           msgpack:"b"`
	Price         string        `json:"p"           msgpack:"p"`
	Size          string        `json:"s"           msgpack:"s"`
	ReduceOnly    bool          `json:"r"           msgpack:"r"`
	Type          orderTypeWire `json:"t"           msgpack:"t"`
	ClientOrderID string        `json:"c,omitempty" msgpack:"c,omitempty"`
}

type orderAction struct {
	Type     string      `json:"type"     msgpack:"type"`
	Orders   []orderWire `json:"orders"   msgpack:"orders"`
	Grouping string      `json:"grouping" msgpack:"grouping"`
}

type modifyWire struct {
	OrderID any       `json:"oid"   msgpack:"oid"`
	Order   orderWire `json:"order" msgpack:"order"`
}

type batchModifyAction struct {
	Type     string       `json:"type"     msgpack:"type"`
	Modifies []modifyWire `json:"modifies" msgpack:"modifies"`
}

type cancelWire struct {
	AssetID uint64 `json:"a" msgpack:"a"`
	OrderID uint64 `json:"o" msgpack:"o"`
}

type cancelAction struct {
	Type    string       `json:"type"    msgpack:"type"`
	Cancels []cancelWire `json:"cancels" msgpack:"cancels"`
}

type cancelByClientOrderIDWire struct {
	AssetID       uint64 `json:"asset" msgpack:"asset"`
	ClientOrderID string `json:"cloid" msgpack:"cloid"`
}

type cancelByClientOrderIDAction struct {
	Type    string                      `json:"type"    msgpack:"type"`
	Cancels []cancelByClientOrderIDWire `json:"cancels" msgpack:"cancels"`
}

type updateLeverageAction struct {
	Type     string `json:"type"     msgpack:"type"`
	AssetID  uint64 `json:"asset"    msgpack:"asset"`
	IsCross  bool   `json:"isCross"  msgpack:"isCross"`
	Leverage uint64 `json:"leverage" msgpack:"leverage"`
}

type orderConversionRequest struct {
	Source          *OpenOrder
	Status          string
	StatusTimestamp time.Time
	Mapping         *pairMapping
	AssetType       asset.Item
}

// OpenOrdersRequest selects an address and its perpetual DEX. An empty DEX
// includes spot orders and orders on the default perpetual DEX.
type OpenOrdersRequest struct {
	User string
	DEX  string
}

// OpenOrder contains the common order fields returned by account info and websocket endpoints.
type OpenOrder struct {
	Coin             string       `json:"coin"`
	Side             string       `json:"side"`
	LimitPrice       types.Number `json:"limitPx"`
	Size             types.Number `json:"sz"`
	OriginalSize     types.Number `json:"origSz"`
	OrderID          uint64       `json:"oid"`
	Timestamp        types.Time   `json:"timestamp"`
	TriggerCondition string       `json:"triggerCondition"`
	IsTrigger        bool         `json:"isTrigger"`
	TriggerPrice     types.Number `json:"triggerPx"`
	IsPositionTPSL   bool         `json:"isPositionTpsl"`
	ReduceOnly       bool         `json:"reduceOnly"`
	OrderType        string       `json:"orderType"`
	TimeInForce      string       `json:"tif"`
	ClientOrderID    *string      `json:"cloid"`
}

// HistoricalOrder contains an order and its latest status.
type HistoricalOrder struct {
	Order           OpenOrder  `json:"order"`
	Status          string     `json:"status"`
	StatusTimestamp types.Time `json:"statusTimestamp"`
}

// OrderStatusRequest selects an order for an address using exactly one of
// OrderID or ClientOrderID.
type OrderStatusRequest struct {
	User          string
	OrderID       uint64
	ClientOrderID string
}

// OrderStatusResponse contains either an order result or an unknown-order status.
type OrderStatusResponse struct {
	Status string           `json:"status"`
	Order  *HistoricalOrder `json:"order"`
}

type exchangeActionData struct {
	Type string                 `json:"type"`
	Data exchangeActionStatuses `json:"data"`
}

type exchangeActionStatuses struct {
	Statuses []json.RawMessage `json:"statuses"`
}

type actionErrorResponse struct {
	Error string `json:"error"`
}

type restingOrderStatus struct {
	OrderID uint64 `json:"oid"`
}

type filledOrderStatus struct {
	TotalSize    types.Number `json:"totalSz"`
	AveragePrice types.Number `json:"avgPx"`
	OrderID      uint64       `json:"oid"`
}

type orderActionStatus struct {
	Resting  *restingOrderStatus `json:"resting"`
	Filled   *filledOrderStatus  `json:"filled"`
	Error    string              `json:"error"`
	Deferred string              `json:"-"`
}
