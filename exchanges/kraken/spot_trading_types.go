package kraken

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
)

var (
	errBatchCancelOrderCount   = errors.New("batch cancellation must contain between 1 and 50 order identifiers")
	errBatchCloseTypeInvalid   = errors.New("batch conditional close order type is invalid")
	errBatchIdentityConflict   = errors.New("batch order user reference and client order identifier are mutually exclusive")
	errBatchOrderCount         = errors.New("batch order request must contain between 2 and 15 orders")
	errBatchOrderFields        = errors.New("batch orders require order type and side")
	errBatchOrderTypeInvalid   = errors.New("batch order type is invalid")
	errBatchSelfTradeInvalid   = errors.New("batch order self-trade policy is invalid")
	errBatchSideInvalid        = errors.New("batch order side is invalid")
	errBatchTimeInForceInvalid = errors.New("batch order time-in-force is invalid")
	errBatchTriggerInvalid     = errors.New("batch order trigger is invalid")
	errDeadlineInvalid         = errors.New("deadline must be between 2 and 60 seconds in the future")
	errOrderFlagInvalid        = errors.New("order flag is invalid")
	errOrderIdentityConflict   = errors.New("order identity fields are mutually exclusive")
	errOrderIdentityRequired   = errors.New("transaction or client order identifier is required")
	errOrderPriceConflict      = errors.New("absolute and relative order prices are mutually exclusive")
	errOrderPriceInvalid       = errors.New("order price is invalid")
	errOrderSideInvalid        = errors.New("order side must be buy or sell")
	errOrderTypeInvalid        = errors.New("order type is invalid")
	errScheduledTimeConflict   = errors.New("absolute and relative scheduled times are mutually exclusive")
	errScheduledTimeInvalid    = errors.New("scheduled time is invalid")
	errSelfTradeInvalid        = errors.New("self-trade policy is invalid")
	errTimeInForceInvalid      = errors.New("time-in-force is invalid")
	errTimeoutInvalid          = errors.New("cancel-all timeout must be a non-negative whole number of seconds")
	errTimeoutTooLarge         = errors.New("cancel-all timeout must be less than 86400 seconds")
	errTriggerInvalid          = errors.New("order trigger must be index or last")
	errVolumeInvalid           = errors.New("volume must not be negative")
)

// OrderType identifies a Kraken Spot order execution model.
type OrderType string

// Kraken Spot order types.
const (
	OrderTypeMarket            OrderType = "market"
	OrderTypeLimit             OrderType = "limit"
	OrderTypeIceberg           OrderType = "iceberg"
	OrderTypeStopLoss          OrderType = "stop-loss"
	OrderTypeTakeProfit        OrderType = "take-profit"
	OrderTypeStopLossLimit     OrderType = "stop-loss-limit"
	OrderTypeTakeProfitLimit   OrderType = "take-profit-limit"
	OrderTypeTrailingStop      OrderType = "trailing-stop"
	OrderTypeTrailingStopLimit OrderType = "trailing-stop-limit"
	OrderTypeSettlePosition    OrderType = "settle-position"
)

// OrderSide identifies a Kraken order direction.
type OrderSide string

// Kraken order sides.
const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

// OrderTrigger identifies the price signal used to trigger an order.
type OrderTrigger string

// Kraken order trigger sources.
const (
	OrderTriggerIndex OrderTrigger = "index"
	OrderTriggerLast  OrderTrigger = "last"
)

// SelfTradePolicy identifies Kraken self-trade prevention behaviour.
type SelfTradePolicy string

// Kraken self-trade prevention policies.
const (
	SelfTradePolicyCancelNewest SelfTradePolicy = "cancel-newest"
	SelfTradePolicyCancelOldest SelfTradePolicy = "cancel-oldest"
	SelfTradePolicyCancelBoth   SelfTradePolicy = "cancel-both"
)

// OrderTimeInForce identifies a Kraken order lifetime policy.
type OrderTimeInForce string

// Kraken Spot order time-in-force values.
const (
	OrderTimeInForceGTC OrderTimeInForce = "GTC"
	OrderTimeInForceIOC OrderTimeInForce = "IOC"
	OrderTimeInForceGTD OrderTimeInForce = "GTD"
	OrderTimeInForceFOK OrderTimeInForce = "FOK"
)

// OrderFlag identifies an optional Kraken order flag.
type OrderFlag string

// Kraken Spot order flags.
const (
	OrderFlagPostOnly        OrderFlag = "post"
	OrderFlagFeeInBase       OrderFlag = "fcib"
	OrderFlagFeeInQuote      OrderFlag = "fciq"
	OrderFlagVolumeInQuote   OrderFlag = "viqc"
	OrderFlagNoMarketProtect OrderFlag = "nompp"
)

// OrderPrice represents either an ordinary numeric price or a Kraken relative-price expression.
type OrderPrice struct {
	Value      float64
	Expression string
}

// AddOrderRequest defines all current standard-order parameters.
type AddOrderRequest struct {
	UserReference   *int32
	ClientOrderID   string
	OrderType       OrderType
	Side            OrderSide
	Volume          float64
	DisplayVolume   *float64
	Pair            currency.Pair
	AssetClass      AssetClass
	Price           *OrderPrice
	SecondaryPrice  *OrderPrice
	Trigger         OrderTrigger
	Leverage        uint8
	ReduceOnly      bool
	SelfTradePolicy SelfTradePolicy
	OrderFlags      []OrderFlag
	TimeInForce     OrderTimeInForce
	StartTime       time.Time
	StartDelay      time.Duration
	ExpireTime      time.Time
	ExpireAfter     time.Duration
	Close           *AddOrderCloseRequest
	Deadline        time.Time
	Validate        bool
	Broker          string
}

// AddOrderCloseRequest defines a conditional close attached to a standard order.
type AddOrderCloseRequest struct {
	OrderType      OrderType
	Price          *OrderPrice
	SecondaryPrice *OrderPrice
}

// CancelOrderRequest defines current order-cancellation identities.
type CancelOrderRequest struct {
	TransactionID string
	UserReference *int32
	ClientOrderID string
}

// AmendOrderRequest defines parameters for amending an open order.
type AmendOrderRequest struct {
	TransactionID   string
	ClientOrderID   string
	OrderQuantity   *float64
	DisplayQuantity *float64
	LimitPrice      *OrderPrice
	TriggerPrice    *OrderPrice
	Pair            currency.Pair
	PostOnly        bool
	Deadline        time.Time
}

// AmendOrderResponse defines an order amend identifier.
type AmendOrderResponse struct {
	AmendID string `json:"amend_id"`
}

// CancelAllOrdersAfterRequest defines parameters for Kraken's Spot dead-man switch.
type CancelAllOrdersAfterRequest struct {
	Timeout time.Duration
}

// CancelAllOrdersAfterResponse defines cancel-all trigger timing.
type CancelAllOrdersAfterResponse struct {
	CurrentTime time.Time `json:"currentTime"`
	TriggerTime time.Time `json:"triggerTime"`
}

// AddOrderBatchRequest defines parameters for batch order placement.
type AddOrderBatchRequest struct {
	Orders     []AddOrderBatchOrderRequest
	Pair       currency.Pair
	AssetClass AssetClass
	Deadline   time.Time
	Validate   bool
	Broker     string
}

// AddOrderBatchOrderRequest defines one order in a batch placement request.
type AddOrderBatchOrderRequest struct {
	UserReference   *int32
	ClientOrderID   string
	OrderType       OrderType
	OrderSide       OrderSide
	Volume          float64
	DisplayVolume   *float64
	Price           *OrderPrice
	SecondaryPrice  *OrderPrice
	Trigger         OrderTrigger
	Leverage        uint8
	ReduceOnly      bool
	SelfTradePolicy SelfTradePolicy
	OrderFlags      []OrderFlag
	TimeInForce     OrderTimeInForce
	StartTime       time.Time
	StartDelay      time.Duration
	ExpireTime      time.Time
	ExpireAfter     time.Duration
	Close           *AddOrderBatchCloseRequest
}

// AddOrderBatchCloseRequest defines a conditional close attached to a batch order.
type AddOrderBatchCloseRequest struct {
	OrderType      OrderType
	Price          *OrderPrice
	SecondaryPrice *OrderPrice
}

// AddOrderBatchResponse defines batch placement results.
type AddOrderBatchResponse struct {
	Orders []AddOrderBatchOrderResponse `json:"orders"`
}

// AddOrderBatchOrderResponse defines one batch placement result.
type AddOrderBatchOrderResponse struct {
	Description OrderDescription `json:"descr"`
	Error       string           `json:"error"`
	Transaction string           `json:"txid"`
}

// CancelOrderBatchRequest defines order identifiers for batch cancellation.
type CancelOrderBatchRequest struct {
	TransactionIDs []string
	UserReferences []int32
	ClientOrderIDs []string
}

// CancelOrderBatchResponse defines the number of cancelled orders.
type CancelOrderBatchResponse struct {
	Count uint64 `json:"count"`
}

func formatOrderPrice(price *OrderPrice) (string, error) {
	if price == nil {
		return "", nil
	}
	if price.Value != 0 && price.Expression != "" {
		return "", errOrderPriceConflict
	}
	if price.Expression == "" {
		if price.Value < 0 || math.IsNaN(price.Value) || math.IsInf(price.Value, 0) {
			return "", errOrderPriceInvalid
		}
		return strconv.FormatFloat(price.Value, 'f', -1, 64), nil
	}
	if !strings.ContainsAny(price.Expression[:1], "+-#") {
		return "", errOrderPriceInvalid
	}
	number := strings.TrimSuffix(price.Expression[1:], "%")
	value, err := strconv.ParseFloat(number, 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return "", errOrderPriceInvalid
	}
	return price.Expression, nil
}

func formatScheduledTime(absolute time.Time, relative, minimumRelative time.Duration) (string, error) {
	if !absolute.IsZero() && relative != 0 {
		return "", errScheduledTimeConflict
	}
	if !absolute.IsZero() {
		if absolute.Unix() < 0 {
			return "", errScheduledTimeInvalid
		}
		return strconv.FormatInt(absolute.Unix(), 10), nil
	}
	if relative == 0 {
		return "", nil
	}
	if relative < minimumRelative || relative < 0 || relative%time.Second != 0 {
		return "", errScheduledTimeInvalid
	}
	return "+" + strconv.FormatInt(int64(relative/time.Second), 10), nil
}

func formatDeadline(deadline, now time.Time) (string, error) {
	if deadline.IsZero() {
		return "", nil
	}
	minimum := now.Add(2 * time.Second)
	maximum := now.Add(time.Minute)
	if deadline.Before(minimum) || deadline.After(maximum) {
		return "", errDeadlineInvalid
	}
	return deadline.UTC().Format(time.RFC3339Nano), nil
}

func formatOrderFlags(flags []OrderFlag) (string, error) {
	values := make([]string, len(flags))
	for i := range flags {
		if !isValidSpotEnum(flags[i], "post", "fcib", "fciq", "viqc", "nompp") || flags[i] == "" {
			return "", errOrderFlagInvalid
		}
		values[i] = string(flags[i])
	}
	return strings.Join(values, ","), nil
}

// AddOrderResponse type
type AddOrderResponse struct {
	Description    OrderDescription `json:"descr"`
	TransactionIDs []string         `json:"txid"`
}

// OrderDescription represents an orders description
type OrderDescription struct {
	Close string `json:"close"`
	Order string `json:"order"`
}

// CancelOrderResponse type
type CancelOrderResponse struct {
	Count   int64 `json:"count"`
	Pending any   `json:"pending"`
}
