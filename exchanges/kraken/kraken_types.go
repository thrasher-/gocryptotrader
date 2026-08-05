package kraken

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
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

var assetTranslator assetTranslatorStore

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

// Asset holds asset information
type Asset struct {
	Altname         string       `json:"altname"`
	AclassBase      string       `json:"aclass_base"`
	AssetClass      string       `json:"aclass"`
	Decimals        int          `json:"decimals"`
	DisplayDecimals int          `json:"display_decimals"`
	CollateralValue types.Number `json:"collateral_value"`
	MarginRate      types.Number `json:"margin_rate"`
	Status          string       `json:"status"`
}

// AssetPairs holds asset pair information
type AssetPairs struct {
	Altname            string       `json:"altname"`
	Wsname             string       `json:"wsname"`
	AclassBase         string       `json:"aclass_base"`
	Base               string       `json:"base"`
	AclassQuote        string       `json:"aclass_quote"`
	Quote              string       `json:"quote"`
	ExecutionVenue     string       `json:"execution_venue"`
	Lot                string       `json:"lot"`
	CostDecimals       int          `json:"cost_decimals"`
	PairDecimals       int          `json:"pair_decimals"`
	LotDecimals        int          `json:"lot_decimals"`
	LotMultiplier      int          `json:"lot_multiplier"`
	LeverageBuy        []int        `json:"leverage_buy"`
	LeverageSell       []int        `json:"leverage_sell"`
	Fees               [][]float64  `json:"fees"`
	FeesMaker          [][]float64  `json:"fees_maker"`
	FeeVolumeCurrency  string       `json:"fee_volume_currency"`
	MarginCall         int          `json:"margin_call"`
	MarginStop         int          `json:"margin_stop"`
	OrderMinimum       types.Number `json:"ordermin"`
	CostMinimum        types.Number `json:"costmin"`
	TickSize           types.Number `json:"tick_size"`
	Status             string       `json:"status"`
	LongPositionLimit  types.Number `json:"long_position_limit"`
	ShortPositionLimit types.Number `json:"short_position_limit"`
}

// TickerResponse holds Kraken Spot ticker information.
type TickerResponse struct {
	Ask                        [3]types.Number `json:"a"`
	Bid                        [3]types.Number `json:"b"`
	Last                       [2]types.Number `json:"c"`
	Volume                     [2]types.Number `json:"v"`
	VolumeWeightedAveragePrice [2]types.Number `json:"p"`
	Trades                     [2]int64        `json:"t"`
	Low                        [2]types.Number `json:"l"`
	High                       [2]types.Number `json:"h"`
	Open                       types.Number    `json:"o"`
}

// RecentTradesResponse holds recent trade data
type RecentTradesResponse struct {
	Trades map[string][]RecentTradeResponseItem
	Last   string
}

// UnmarshalJSON unmarshals the recent trades response
func (r *RecentTradesResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	response := RecentTradesResponse{Trades: make(map[string][]RecentTradeResponseItem, len(raw))}
	for key, value := range raw {
		if key == "last" {
			if err := json.Unmarshal(value, &response.Last); err != nil {
				return err
			}
		} else {
			var trades []RecentTradeResponseItem
			if err := json.Unmarshal(value, &trades); err != nil {
				return err
			}
			response.Trades[key] = trades
		}
	}
	*r = response
	return nil
}

// RecentTradeResponseItem holds a single recent trade response item
type RecentTradeResponseItem struct {
	Price         types.Number
	Volume        types.Number
	Time          types.Time
	BuyOrSell     string
	MarketOrLimit string
	Miscellaneous any
	TradeID       types.Number
}

// UnmarshalJSON unmarshals the recent trade response item
func (r *RecentTradeResponseItem) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 7 {
		return fmt.Errorf("expected 7 trade fields, got %d", len(fields))
	}
	decoded := RecentTradeResponseItem{}
	if err := json.Unmarshal(data, &[7]any{
		&decoded.Price,
		&decoded.Volume,
		&decoded.Time,
		&decoded.BuyOrSell,
		&decoded.MarketOrLimit,
		&decoded.Miscellaneous,
		&decoded.TradeID,
	}); err != nil {
		return err
	}
	*r = decoded
	return nil
}

// SpreadItem holds the spread between trades
type SpreadItem struct {
	Time types.Time
	Bid  types.Number
	Ask  types.Number
}

// UnmarshalJSON unmarshals the spread item
func (s *SpreadItem) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 3 {
		return fmt.Errorf("expected 3 spread fields, got %d", len(fields))
	}
	decoded := SpreadItem{}
	if err := json.Unmarshal(data, &[3]any{&decoded.Time, &decoded.Bid, &decoded.Ask}); err != nil {
		return err
	}
	*s = decoded
	return nil
}

// SpreadResponse holds the spread response data
type SpreadResponse struct {
	Spreads map[string][]SpreadItem
	Last    types.Time
}

// UnmarshalJSON unmarshals the spread response
func (s *SpreadResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	response := SpreadResponse{Spreads: make(map[string][]SpreadItem, len(raw))}
	for key, value := range raw {
		if key == "last" {
			if err := json.Unmarshal(value, &response.Last); err != nil {
				return err
			}
		} else {
			var spreads []SpreadItem
			if err := json.Unmarshal(value, &spreads); err != nil {
				return err
			}
			response.Spreads[key] = spreads
		}
	}
	*s = response
	return nil
}

// TradeBalanceInfo type
type TradeBalanceInfo struct {
	EquivalentBalance types.Number `json:"eb"` // combined balance of all currencies
	TradeBalance      types.Number `json:"tb"` // combined balance of all equity currencies
	MarginAmount      types.Number `json:"m"`  // margin amount of open positions
	Net               types.Number `json:"n"`  // unrealized net profit/loss of open positions
	CostBasis         types.Number `json:"c"`
	CurrentValuation  types.Number `json:"v"`
	Equity            types.Number `json:"e"`  // trade balance + unrealized net profit/loss
	FreeMargin        types.Number `json:"mf"` // equity - initial margin (maximum margin available to open new positions)
	FreeMarginOrders  types.Number `json:"mfo"`
	MarginLevel       types.Number `json:"ml"` // (equity / initial margin) * 100
	UnexecutedValue   types.Number `json:"uv"`
}

// OrderInfo type
type OrderInfo struct {
	RefID         string     `json:"refid"`
	UserRef       int32      `json:"userref"`
	ClientOrderID string     `json:"cl_ord_id"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	OpenTime      types.Time `json:"opentm"`
	CloseTime     types.Time `json:"closetm"`
	StartTime     types.Time `json:"starttm"`
	ExpireTime    types.Time `json:"expiretm"`
	Description   struct {
		Pair       string       `json:"pair"`
		Type       string       `json:"type"`
		OrderType  string       `json:"ordertype"`
		Price      types.Number `json:"price"`
		Price2     types.Number `json:"price2"`
		Leverage   string       `json:"leverage"`
		Order      string       `json:"order"`
		Close      string       `json:"close"`
		AssetClass string       `json:"aclass"`
	} `json:"descr"`
	TimeInForce    string       `json:"time_in_force"`
	Volume         types.Number `json:"vol"`
	VolumeExecuted types.Number `json:"vol_exec"`
	Cost           types.Number `json:"cost"`
	Fee            types.Number `json:"fee"`
	Price          types.Number `json:"price"`
	StopPrice      types.Number `json:"stopprice"`
	LimitPrice     types.Number `json:"limitprice"`
	Trigger        string       `json:"trigger"`
	Margin         bool         `json:"margin"`
	Misc           string       `json:"misc"`
	OrderFlags     string       `json:"oflags"`
	Trades         []string     `json:"trades"`
	SenderSubID    string       `json:"sender_sub_id"`
}

// OpenOrders type
type OpenOrders struct {
	Open  map[string]OrderInfo `json:"open"`
	Count int64                `json:"count"`
}

// ClosedOrders type
type ClosedOrders struct {
	Closed map[string]OrderInfo `json:"closed"`
	Count  int64                `json:"count"`
}

// TradesHistory type
type TradesHistory struct {
	Trades map[string]TradeInfo `json:"trades"`
	Count  int64                `json:"count"`
}

// TradeInfo type
type TradeInfo struct {
	OrderTxID                  string       `json:"ordertxid"`
	PosTxID                    string       `json:"postxid"`
	Pair                       string       `json:"pair"`
	Time                       types.Time   `json:"time"`
	Type                       string       `json:"type"`
	OrderType                  string       `json:"ordertype"`
	Price                      types.Number `json:"price"`
	Cost                       types.Number `json:"cost"`
	Fee                        types.Number `json:"fee"`
	Volume                     types.Number `json:"vol"`
	Margin                     types.Number `json:"margin"`
	Leverage                   string       `json:"leverage"`
	Misc                       string       `json:"misc"`
	ClosedPositionAveragePrice types.Number `json:"cprice"`
	ClosedPositionCost         types.Number `json:"ccost"`
	ClosedPositionFee          types.Number `json:"cfee"`
	ClosedPositionVolume       types.Number `json:"cvol"`
	ClosedPositionMargin       types.Number `json:"cmargin"`
	Net                        types.Number `json:"net"`
	Trades                     []string     `json:"trades"`
	Ledgers                    []string     `json:"ledgers"`
	TradeID                    uint64       `json:"trade_id"`
	Maker                      bool         `json:"maker"`
	AssetClass                 string       `json:"aclass"`
	TradeOrderType             string       `json:"tradeordertype"`
	PosStatus                  string       `json:"posstatus"`
}

// Position holds the opened position
type Position struct {
	Ordertxid      string       `json:"ordertxid"`
	AssetClass     string       `json:"class"`
	Pair           string       `json:"pair"`
	Time           types.Time   `json:"time"`
	Type           string       `json:"type"`
	OrderType      string       `json:"ordertype"`
	Cost           types.Number `json:"cost"`
	Fee            types.Number `json:"fee"`
	Volume         types.Number `json:"vol"`
	VolumeClosed   types.Number `json:"vol_closed"`
	Margin         types.Number `json:"margin"`
	Value          types.Number `json:"value"`
	RolloverTime   int64        `json:"rollovertm,string"`
	Misc           string       `json:"misc"`
	OrderFlags     string       `json:"oflags"`
	PositionStatus string       `json:"posstatus"`
	Net            string       `json:"net"`
	Terms          string       `json:"terms"`
}

// Ledgers type
type Ledgers struct {
	Ledger map[string]LedgerInfo `json:"ledger"`
	Count  int64                 `json:"count"`
}

// LedgerInfo type
type LedgerInfo struct {
	Refid   string       `json:"refid"`
	Time    types.Time   `json:"time"`
	Type    string       `json:"type"`
	Subtype string       `json:"subtype"`
	Aclass  string       `json:"aclass"`
	Asset   string       `json:"asset"`
	Amount  types.Number `json:"amount"`
	Fee     types.Number `json:"fee"`
	Balance types.Number `json:"balance"`
}

// TradeVolumeResponse type
type TradeVolumeResponse struct {
	Currency          string                    `json:"currency"`
	AssetClass        string                    `json:"asset_class"`
	Volume            types.Number              `json:"volume"`
	Inputs            TradeVolumeInputs         `json:"inputs"`
	Fees              map[string]TradeVolumeFee `json:"fees"`
	FeesMaker         map[string]TradeVolumeFee `json:"fees_maker"`
	VolumeSubaccounts []TradeVolumeSubaccount   `json:"volume_subaccounts"`
	Schedules         []TradeVolumeFeeSchedule  `json:"schedules"`
}

// TradeVolumeInputs defines the domain values evaluated against fee tiers.
type TradeVolumeInputs struct {
	SpotVolume30D    types.Number `json:"domain_spot_volume_30d"`
	FuturesVolume30D types.Number `json:"domain_futures_volume_30d"`
	AssetsOnPlatform types.Number `json:"domain_assets_on_platform"`
}

// TradeVolumeSubaccount defines one subaccount volume contribution.
type TradeVolumeSubaccount struct {
	IIBAN  string       `json:"iiban"`
	Volume types.Number `json:"volume"`
}

// TradeVolumeFeeSchedule defines current fee tiers for one pair.
type TradeVolumeFeeSchedule struct {
	Pair       string                       `json:"pair"`
	AssetClass string                       `json:"class"`
	Tiers      []TradeVolumeFeeScheduleTier `json:"tiers"`
}

// TradeVolumeFeeScheduleTier defines one maker/taker fee tier.
type TradeVolumeFeeScheduleTier struct {
	MakerFee                types.Number  `json:"maker_fee"`
	TakerFee                types.Number  `json:"taker_fee"`
	MinimumSpotVolume       *types.Number `json:"min_spot_volume"`
	MinimumFuturesVolume    *types.Number `json:"min_futures_volume"`
	MinimumAssetsOnPlatform *types.Number `json:"min_assets_on_platform"`
	Active                  *bool         `json:"active"`
}

// TradeVolumeFee type
type TradeVolumeFee struct {
	Fee               types.Number  `json:"fee"`
	MinFee            types.Number  `json:"minfee"`
	MaxFee            types.Number  `json:"maxfee"`
	NextFee           *types.Number `json:"nextfee"`
	TierVolume        types.Number  `json:"tiervolume"`
	TierFuturesVolume *types.Number `json:"tierfuturesvolume"`
	NextVolume        *types.Number `json:"nextvolume"`
	NextFuturesVolume *types.Number `json:"nextfuturesvolume"`
	VolumeOffset      *types.Number `json:"volumeoffset"`
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

// DepositFees the large list of predefined deposit fees
// Prone to change
var DepositFees = map[currency.Code]float64{
	currency.XTZ: 0.05,
}

// WithdrawalFees the large list of predefined withdrawal fees
// Prone to change
var WithdrawalFees = map[currency.Code]float64{
	currency.ZUSD: 5,
	currency.ZEUR: 5,
	currency.USD:  5,
	currency.EUR:  5,
	currency.REP:  0.01,
	currency.XXBT: 0.0005,
	currency.BTC:  0.0005,
	currency.XBT:  0.0005,
	currency.BCH:  0.0001,
	currency.ADA:  0.3,
	currency.DASH: 0.005,
	currency.XDG:  2,
	currency.EOS:  0.05,
	currency.ETH:  0.005,
	currency.ETC:  0.005,
	currency.GNO:  0.005,
	currency.ICN:  0.2,
	currency.LTC:  0.001,
	currency.MLN:  0.003,
	currency.XMR:  0.05,
	currency.QTUM: 0.01,
	currency.XRP:  0.02,
	currency.XLM:  0.00002,
	currency.USDT: 5,
	currency.XTZ:  0.05,
	currency.ZEC:  0.0001,
}

// WsTokenResponse holds the WS auth token
type WsTokenResponse struct {
	Expires int64  `json:"expires"`
	Token   string `json:"token"`
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

type genericRESTResponse struct {
	Error  errorResponse `json:"error"`
	Result any           `json:"result"`
}

type errorResponse struct {
	warnings []string
	errors   error
}

func (e *errorResponse) UnmarshalJSON(data []byte) error {
	var errInterface any
	if err := json.Unmarshal(data, &errInterface); err != nil {
		return err
	}

	switch d := errInterface.(type) {
	case string:
		if d[0] == 'E' {
			e.errors = common.AppendError(e.errors, errors.New(d))
		} else {
			e.warnings = append(e.warnings, d)
		}
	case []any:
		for x := range d {
			errStr, ok := d[x].(string)
			if !ok {
				return fmt.Errorf("unable to convert %v to string", d[x])
			}
			if errStr[0] == 'E' {
				e.errors = common.AppendError(e.errors, errors.New(errStr))
			} else {
				e.warnings = append(e.warnings, errStr)
			}
		}
	default:
		return fmt.Errorf("unhandled error response type %T", errInterface)
	}
	return nil
}

// Errors returns one or many errors as an error
func (e errorResponse) Errors() error {
	return e.errors
}

// Warnings returns a string of warnings
func (e errorResponse) Warnings() string {
	return strings.Join(e.warnings, ", ")
}
