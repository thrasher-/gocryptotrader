package kraken

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

const (
	// Futures
	futuresTickers      = "/api/v3/tickers"
	futuresOrderbook    = "/api/v3/orderbook"
	futuresInstruments  = "/api/v3/instruments"
	futuresTradeHistory = "/api/v3/history"
	futuresCandles      = "charts/v1/"
	futuresPublicTrades = "history/v2/market/"

	futuresSendOrder         = "/api/v3/sendorder"
	futuresCancelOrder       = "/api/v3/cancelorder"
	futuresOrderFills        = "/api/v3/fills"
	futuresTransfer          = "/api/v3/transfer"
	futuresOpenPositions     = "/api/v3/openpositions"
	futuresBatchOrder        = "/api/v3/batchorder"
	futuresNotifications     = "/api/v3/notifications"
	futuresAccountData       = "/api/v3/accounts"
	futuresCancelAllOrders   = "/api/v3/cancelallorders"
	futuresCancelOrdersAfter = "/api/v3/cancelallordersafter"
	futuresOpenOrders        = "/api/v3/openorders"
	futuresRecentOrders      = "/api/v3/recentorders"
	futuresWithdraw          = "/api/v3/withdrawal"
	futuresTransfers         = "/api/v3/transfers"
	futuresEditOrder         = "/api/v3/editorder"

	// Rate limit consts
	krakenRateInterval = time.Second
	krakenRequestRate  = 1

	// Status consts
	statusOpen = "open"
)

// GenericResponse stores general response data for functions that only return success
type GenericResponse struct {
	Timestamp string `json:"timestamp"`
	Result    string `json:"result"`
}

type genericFuturesResponse struct {
	Result     string    `json:"result"`
	ServerTime time.Time `json:"serverTime"`
	Error      string    `json:"error"`
	Errors     []string  `json:"errors"`
}

// WebsocketRequest defines the common Spot WebSocket request envelope.
type WebsocketRequest[T any] struct {
	Method    string `json:"method"`
	Params    T      `json:"params"`
	RequestID int64  `json:"req_id,omitempty"`
}

// WebsocketSubscriptionParams defines Spot WebSocket subscription parameters.
type WebsocketSubscriptionParams struct {
	Channel    string   `json:"channel"`
	Symbols    []string `json:"symbol,omitempty"`
	Interval   int      `json:"interval,omitempty"`
	Depth      int      `json:"depth,omitempty"`
	Token      string   `json:"token,omitempty"`
	SnapOrders bool     `json:"snap_orders,omitempty"`
	SnapTrades bool     `json:"snap_trades,omitempty"`
}

type websocketResponse struct {
	Method    string                  `json:"method"`
	RequestID int64                   `json:"req_id,omitempty"`
	Success   *bool                   `json:"success,omitempty"`
	Error     string                  `json:"error,omitempty"`
	Symbol    string                  `json:"symbol,omitempty"`
	Result    websocketResponseResult `json:"result"`
}

type websocketResponseResult struct {
	Channel    string `json:"channel"`
	Symbol     string `json:"symbol"`
	Interval   int    `json:"interval,omitempty"`
	Depth      int    `json:"depth,omitempty"`
	OrderID    string `json:"order_id,omitempty"`
	Count      int64  `json:"count,omitempty"`
	SnapOrders bool   `json:"snap_orders,omitempty"`
	SnapTrades bool   `json:"snap_trades,omitempty"`
}

type websocketMessage struct {
	Channel  string            `json:"channel"`
	Data     []json.RawMessage `json:"data"`
	Sequence uint64            `json:"sequence,omitempty"`
	Type     string            `json:"type"`
}

type websocketStatus struct {
	APIVersion   string `json:"api_version"`
	ConnectionID uint64 `json:"connection_id"`
	System       string `json:"system"`
	Version      string `json:"version"`
}

type websocketTicker struct {
	Ask       float64   `json:"ask"`
	AskQty    float64   `json:"ask_qty"`
	Bid       float64   `json:"bid"`
	BidQty    float64   `json:"bid_qty"`
	Change    float64   `json:"change"`
	High      float64   `json:"high"`
	Last      float64   `json:"last"`
	Low       float64   `json:"low"`
	Symbol    string    `json:"symbol"`
	Timestamp time.Time `json:"timestamp"`
	Volume    float64   `json:"volume"`
	VWAP      float64   `json:"vwap"`
}

type websocketTrade struct {
	OrderType string    `json:"ord_type"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"qty"`
	Side      string    `json:"side"`
	Symbol    string    `json:"symbol"`
	Timestamp time.Time `json:"timestamp"`
	TradeID   uint64    `json:"trade_id"`
}

type websocketCandle struct {
	Close         float64   `json:"close"`
	High          float64   `json:"high"`
	Interval      int       `json:"interval"`
	IntervalBegin time.Time `json:"interval_begin"`
	Low           float64   `json:"low"`
	Open          float64   `json:"open"`
	Symbol        string    `json:"symbol"`
	Trades        uint64    `json:"trades"`
	Volume        float64   `json:"volume"`
	VWAP          float64   `json:"vwap"`
}

type websocketBook struct {
	Asks      []websocketBookLevel `json:"asks"`
	Bids      []websocketBookLevel `json:"bids"`
	Checksum  uint32               `json:"checksum"`
	Symbol    string               `json:"symbol"`
	Timestamp time.Time            `json:"timestamp"`
}

type websocketBookLevel struct {
	Price    types.PreciseNumber `json:"price"`
	Quantity types.PreciseNumber `json:"qty"`
}

// WebsocketExecution defines an order status or fill event from the executions channel.
type WebsocketExecution struct {
	AveragePrice   float64                 `json:"avg_price"`
	ClientOrderID  string                  `json:"cl_ord_id"`
	CumulativeCost float64                 `json:"cum_cost"`
	CumulativeQty  float64                 `json:"cum_qty"`
	ExecutionID    string                  `json:"exec_id"`
	ExecutionType  string                  `json:"exec_type"`
	Fees           []WebsocketExecutionFee `json:"fees"`
	LastPrice      float64                 `json:"last_price"`
	LastQty        float64                 `json:"last_qty"`
	LimitPrice     float64                 `json:"limit_price"`
	OrderID        string                  `json:"order_id"`
	OrderQty       float64                 `json:"order_qty"`
	OrderStatus    string                  `json:"order_status"`
	OrderType      string                  `json:"order_type"`
	ReduceOnly     bool                    `json:"reduce_only"`
	Side           string                  `json:"side"`
	Symbol         string                  `json:"symbol"`
	TimeInForce    string                  `json:"time_in_force"`
	Timestamp      time.Time               `json:"timestamp"`
	TradeID        uint64                  `json:"trade_id"`
}

// WebsocketExecutionFee defines a fee charged for an execution.
type WebsocketExecutionFee struct {
	Asset    string  `json:"asset"`
	Quantity float64 `json:"qty"`
}

// WebsocketAddOrderParams defines parameters for a Spot WebSocket add_order request.
type WebsocketAddOrderParams struct {
	ClientOrderID  string                  `json:"cl_ord_id,omitempty"`
	ExpireTime     string                  `json:"expire_time,omitempty"`
	LimitPrice     *float64                `json:"limit_price,omitempty"`
	LimitPriceType string                  `json:"limit_price_type,omitempty"`
	Margin         bool                    `json:"margin,omitempty"`
	OrderQty       float64                 `json:"order_qty"`
	OrderType      string                  `json:"order_type"`
	PostOnly       bool                    `json:"post_only,omitempty"`
	ReduceOnly     bool                    `json:"reduce_only,omitempty"`
	Side           string                  `json:"side"`
	Symbol         string                  `json:"symbol"`
	TimeInForce    string                  `json:"time_in_force,omitempty"`
	Token          string                  `json:"token"`
	Triggers       *WebsocketOrderTriggers `json:"triggers,omitempty"`
}

// WebsocketOrderTriggers defines trigger conditions for a Spot WebSocket order.
type WebsocketOrderTriggers struct {
	Price     float64 `json:"price"`
	PriceType string  `json:"price_type"`
	Reference string  `json:"reference"`
}

// WebsocketCancelOrderParams defines parameters for a Spot WebSocket cancel_order request.
type WebsocketCancelOrderParams struct {
	OrderIDs []string `json:"order_id"`
	Token    string   `json:"token"`
}

// WebsocketCancelAllParams defines parameters for a Spot WebSocket cancel_all request.
type WebsocketCancelAllParams struct {
	Token string `json:"token"`
}
