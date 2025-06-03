package kraken

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types"
)

const (
	krakenAPIVersion = "0"
	// All private method constants removed, they are now local to their respective functions in kraken.go

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

	krakenFormat = "2006-01-02T15:04:05.000Z"
)

// SystemStatusResponse defines the response for the system status endpoint
type SystemStatusResponse struct {
	Status    string `json:"status"` // online, maintenance, cancel_only, post_only
	Timestamp string `json:"timestamp"` // Current server time
}

// TimeResponse represents the server time.
type TimeResponse struct {
	Unixtime int64  `json:"unixtime"` // Server time as Unix timestamp
	Rfc1123  string `json:"rfc1123"`  // Server time in RFC1123 format
}

var (
	assetTranslator assetTranslatorStore

	errNoWebsocketOrderbookData = errors.New("no websocket orderbook data")
	errBadChannelSuffix         = errors.New("bad websocket channel suffix")
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

// Asset holds asset information
type Asset struct {
	Altname         string `json:"altname"`
	Aclass          string `json:"aclass"` // Changed from aclass_base
	Decimals        int    `json:"decimals"`
	DisplayDecimals int    `json:"display_decimals"`
}

// AssetPairs holds asset pair information
type AssetPairs struct {
	Altname           string      `json:"altname"`
	Wsname            string      `json:"wsname"`
	AclassBase        string      `json:"aclass_base"`
	Base              string      `json:"base"`
	AclassQuote       string      `json:"aclass_quote"`
	Quote             string      `json:"quote"`
	Lot               string      `json:"lot"`
	PairDecimals      int         `json:"pair_decimals"`
	LotDecimals       int         `json:"lot_decimals"`
	LotMultiplier     int         `json:"lot_multiplier"`
	LeverageBuy       []int       `json:"leverage_buy"`
	LeverageSell      []int       `json:"leverage_sell"`
	Fees              [][]float64 `json:"fees"`
	FeesMaker         [][]float64 `json:"fees_maker"`
	FeeVolumeCurrency string      `json:"fee_volume_currency"`
	MarginCall        int         `json:"margin_call"`
	MarginStop        int         `json:"margin_stop"`
	OrderMinimum      float64     `json:"ordermin,string"`
	TickSize          float64     `json:"tick_size,string"`
	Status            string      `json:"status"`
}

// Ticker is a standard ticker type
type Ticker struct {
	Ask                        float64
	AskSize                    float64
	Bid                        float64
	BidSize                    float64
	Last                       float64
	Volume                     float64
	VolumeWeightedAveragePrice float64
	Trades                     int64
	Low                        float64
	High                       float64
	Open                       float64
}

// Tickers stores a map of tickers
type Tickers map[string]Ticker

// TickerResponse holds ticker information before its put into the Ticker struct
type TickerResponse struct {
	Ask                        [3]types.Number `json:"a"`
	Bid                        [3]types.Number `json:"b"`
	Last                       [2]types.Number `json:"c"`
	Volume                     [2]types.Number `json:"v"`
	VolumeWeightedAveragePrice [2]types.Number `json:"p"`
	Trades                     [2]int64        `json:"t"`
	Low                        [2]types.Number `json:"l"`
	High                       [2]types.Number `json:"h"`
	Open                       [2]types.Number    `json:"o"`
}

// OpenHighLowClose contains ticker event information
type OpenHighLowClose struct {
	Time                       time.Time
	Open                       float64
	High                       float64
	Low                        float64
	Close                      float64
	VolumeWeightedAveragePrice float64
	Volume                     float64
	Count                      int64
}

// RecentTrades holds recent trade data
type RecentTrades struct {
	Price         float64
	Volume        float64
	Time          float64
	BuyOrSell     string
	MarketOrLimit string
	Miscellaneous string // Changed from any
	TradeID       int64
}

// OrderbookBase stores the orderbook price and amount data
type OrderbookBase struct {
	Price     types.Number
	Amount    types.Number
	Timestamp time.Time
}

// Orderbook stores the bids and asks orderbook data
type Orderbook struct {
	Bids []OrderbookBase
	Asks []OrderbookBase
}

// Spread holds the spread between trades
type Spread struct {
	Time time.Time
	Bid  float64
	Ask  float64
}

// Balance represents account asset balances
type Balance struct {
	Total float64 `json:"balance,string"`
	Hold  float64 `json:"hold_trade,string"`
}

// TradeBalanceOptions type
type TradeBalanceOptions struct {
	Asset string // Base asset used to determine balance (default = ZUSD)
}

// TradeBalanceInfo type
type TradeBalanceInfo struct {
	EquivalentBalance float64 `json:"eb,string"` // combined balance of all currencies
	TradeBalance      float64 `json:"tb,string"` // combined balance of all equity currencies
	MarginAmount      float64 `json:"m,string"`  // margin amount of open positions
	Net               float64 `json:"n,string"`  // unrealized net profit/loss of open positions
	CostBasis         float64 `json:"c,string"`  // cost basis of open positions
	FloatingValuation float64 `json:"v,string"`  // current floating valuation of open positions
	Equity            float64 `json:"e,string"`  // trade balance + unrealized net profit/loss
	FreeMargin        float64 `json:"mf,string"` // equity - initial margin (maximum margin available to open new positions)
	MarginLevel       float64 `json:"ml,string"` // (equity / initial margin) * 100
}

// OrderInfo type
type OrderInfo struct {
	RefID       string  `json:"refid"`
	UserRef     int32   `json:"userref"`
	Status      string  `json:"status"`
	OpenTime    int64   `json:"opentm"`
	CloseTime   int64   `json:"closetm"`
	StartTime   int64   `json:"starttm"`
	ExpireTime  int64   `json:"expiretm"`
	Description struct {
		Pair      string  `json:"pair"`
		Type      string  `json:"type"`
		OrderType string  `json:"ordertype"`
		Price     float64 `json:"price,string"`
		Price2    float64 `json:"price2,string"`
		Leverage  string  `json:"leverage"`
		Order     string  `json:"order"`
		Close     string  `json:"close"`
	} `json:"descr"`
	Volume         float64  `json:"vol,string"`
	VolumeExecuted float64  `json:"vol_exec,string"`
	Cost           float64  `json:"cost,string"`
	Fee            float64  `json:"fee,string"`
	Price          float64  `json:"price,string"`
	StopPrice      float64  `json:"stopprice,string"`
	LimitPrice     float64  `json:"limitprice,string"`
	Misc           string   `json:"misc"`
	OrderFlags     string   `json:"oflags"`
	Trades         []string `json:"trades"`
		Reason         string   `json:"reason,omitempty"` // Added
}

// OpenOrders type
type OpenOrders struct {
	Open  map[string]OrderInfo `json:"open"`
}

// ClosedOrders type
type ClosedOrders struct {
	Closed map[string]OrderInfo `json:"closed"`
	Count  int64                `json:"count"`
}

// GetClosedOrdersOptions type
type GetClosedOrdersOptions struct {
	Trades    bool
	UserRef   int32
	Start     string
	End       string
	Ofs       int64
	CloseTime string
}

// OrderInfoOptions type
type OrderInfoOptions struct {
	Trades  bool
	UserRef int32
}

// GetTradesHistoryOptions type
type GetTradesHistoryOptions struct {
	Type             string `json:"type,omitempty"`
	Trades           bool   `json:"trades,omitempty"`
	Start            string `json:"start,omitempty"`
	End              string `json:"end,omitempty"`
	Ofs              int64  `json:"ofs,omitempty"`
	ConsolidateTaker *bool  `json:"consolidate_taker,omitempty"` // Pointer
}

// TradesHistory type
type TradesHistory struct {
	Trades map[string]TradeInfo `json:"trades"`
	Count  int64                `json:"count"`
}

// TradeInfo type
type TradeInfo struct {
	OrderTxID                  string   `json:"ordertxid"`
	Pair                       string   `json:"pair"`
	Time                       int64    `json:"time"` // Changed
	Type                       string   `json:"type"`
	OrderType                  string   `json:"ordertype"`
	Price                      float64  `json:"price,string"`
	Cost                       float64  `json:"cost,string"`
	Fee                        float64  `json:"fee,string"`
	Volume                     float64  `json:"vol,string"`
	Margin                     float64  `json:"margin,string"`
	Misc                       string   `json:"misc"`
	PosTxID                    string   `json:"postxid,omitempty"` // Added omitempty
	ClosedPositionAveragePrice float64  `json:"cprice,string,omitempty"` // Added omitempty
	ClosedPositionFee          float64  `json:"cfee,string,omitempty"`   // Added omitempty
	ClosedPositionVolume       float64  `json:"cvol,string,omitempty"`   // Added omitempty
	ClosedPositionMargin       float64  `json:"cmargin,string,omitempty"`// Added omitempty
	Net                        float64  `json:"net,string,omitempty"`    // Added
	Trades                     []string `json:"trades,omitempty"`        // Added omitempty
	PosStatus                  string   `json:"posstatus,omitempty"`     // Added omitempty
}

// Position holds the opened position
type Position struct {
	Ordertxid    string  `json:"ordertxid"`
	Pair         string  `json:"pair"`
	Time         int64   `json:"time"` // Changed
	Type         string  `json:"type"`
	OrderType    string  `json:"ordertype"`
	Cost         float64 `json:"cost,string"`
	Fee          float64 `json:"fee,string"`
	Volume       float64 `json:"vol,string"`
	VolumeClosed float64 `json:"vol_closed,string"`
	Margin       float64 `json:"margin,string"`
	Value        float64 `json:"value,string,omitempty"` // Added
	Net          string  `json:"net,omitempty"`        // Added omitempty, type was already string
	RolloverTime int64   `json:"rollovertm,string"`
	Misc         string  `json:"misc"`
	OrderFlags   string  `json:"oflags"`
	Terms        string  `json:"terms"`
	// PositionStatus field removed
}

// GetLedgersOptions type
type GetLedgersOptions struct {
	Aclass string
	Asset  string
	Type   string
	Start  string
	End    string
	Ofs    int64
}

// Ledgers type
type Ledgers struct {
	Ledger map[string]LedgerInfo `json:"ledger"`
	Count  int64                 `json:"count"`
}

// LedgerInfo type
type LedgerInfo struct {
	Refid   string  `json:"refid"`
	Time    int64   `json:"time"` // Changed
	Type    string  `json:"type"`
	Subtype string  `json:"subtype,omitempty"` // Added
	Aclass  string  `json:"aclass"`
	Asset   string  `json:"asset"`
	Amount  float64 `json:"amount,string"`
	Fee     float64 `json:"fee,string"`
	Balance float64 `json:"balance,string"`
}

// GetOrderAmendsOptions represents the parameters for fetching order amends.
type GetOrderAmendsOptions struct {
	OrderID string // Required: The order ID to retrieve amends for.
	UserRef int32  // Optional: Restrict results to given user reference id.
}

// OrderAmendEntry represents a single amend transaction.
type OrderAmendEntry struct {
	TxID       string  `json:"txid"`        // Amend transaction ID
	OrderID    string  `json:"order_id"`    // Order ID
	AmendID    string  `json:"amend_id"`    // Unique ID for this amend
	AmendType  string  `json:"amend_type"`  // Type of amend
	NewPrice   string  `json:"new_price,omitempty"`
	OrigPrice  string  `json:"orig_price,omitempty"`
	NewVolume  string  `json:"new_volume,omitempty"`
	OrigVolume string  `json:"orig_volume,omitempty"`
	Timestamp  int64   `json:"timestamp"`   // Unix timestamp
	UserRef    int32   `json:"userref,omitempty"`
	Pair       string  `json:"pair,omitempty"`
	Fee        string  `json:"fee,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// GetOrderAmendsResponse represents the response from the GetOrderAmends endpoint.
type GetOrderAmendsResponse struct {
	Count  int64             `json:"count"`
	Amends []OrderAmendEntry `json:"amends"`
}

// ExportReportType defines the type of report to export.
type ExportReportType string

const (
	ExportReportTypeTrades  ExportReportType = "trades"
	ExportReportTypeLedgers ExportReportType = "ledgers"
)

// ExportReportFormat defines the file format for the export.
type ExportReportFormat string

const (
	ExportReportFormatCSV ExportReportFormat = "CSV"
	ExportReportFormatTSV ExportReportFormat = "TSV"
)

// RequestExportReportOptions represents parameters for requesting an export report.
type RequestExportReportOptions struct {
	Report      ExportReportType   // Required: trades or ledgers
	Format      ExportReportFormat // Optional: CSV (default) or TSV
	Description string             // Required: User-defined description
	Fields      string             // Optional: Comma-delimited list of fields, default "all"
	StartTm     int64              // Optional: Unix start timestamp
	EndTm       int64              // Optional: Unix end timestamp
	Asset       string             // Optional: Comma-delimited list of assets (for ledgers report)
}

// RequestExportReportResponse represents the response from requesting an export.
type RequestExportReportResponse struct {
	ID string `json:"id"` // Report ID
}

// ExportStatusOptions represents parameters for getting export report status.
type ExportStatusOptions struct {
	Report ExportReportType // Required: trades or ledgers
}

// ExportReportInfo represents information about a single export report.
type ExportReportInfo struct {
	ID          string             `json:"id"`
	Descr       string             `json:"descr"`
	Format      ExportReportFormat `json:"format"`
	Report      ExportReportType   `json:"report"`
	Subtype     string             `json:"subtype,omitempty"` // e.g. "all", specific assets
	Status      string             `json:"status"`
	Flags       string             `json:"flags,omitempty"`
	Fields      string             `json:"fields"`
	CreatedTm   int64              `json:"createdtm,string"` // Timestamp string
	StartTm     int64              `json:"starttm,string"`   // Timestamp string
	EndTm       int64              `json:"endtm,string"`     // Timestamp string
	CompletedTm int64              `json:"completedtm,string,omitempty"` // Timestamp string
	DataStartTm int64              `json:"datastarttm,string,omitempty"` // Timestamp string
	DataEndTm   int64              `json:"dataendtm,string,omitempty"`   // Timestamp string
	Aclass      string             `json:"aclass,omitempty"`
	Asset       string             `json:"asset,omitempty"` // Comma-delimited list of assets
}

// DeleteExportType defines the type of delete operation.
type DeleteExportType string

const (
	DeleteExportTypeDelete DeleteExportType = "delete"
	DeleteExportTypeCancel DeleteExportType = "cancel"
)

// DeleteExportOptions represents parameters for deleting or cancelling an export.
type DeleteExportOptions struct {
	ID   string           // Required: Report ID to delete or cancel
	Type DeleteExportType // Required: "delete" or "cancel"
}

// DeleteExportResponse represents the response from deleting/cancelling an export.
type DeleteExportResponse struct {
	Delete bool `json:"delete,omitempty"` // True if deleted
	Cancel bool `json:"cancel,omitempty"` // True if cancelled
}

// AmendOrderOptions represents the parameters for amending an order.
type AmendOrderOptions struct {
	OrderID        string  // Required: Original order ID
	UserRef        int32   // Optional: User reference ID of the original order
	Pair           string  // Required: Asset pair
	Volume         string  // Optional: New order volume (as string to preserve precision)
	Price          string  // Optional: New primary price (as string)
	Price2         string  // Optional: New secondary price (as string)
	OFlags         string  // Optional: Comma-delimited list of order flags
	Deadline       string  // Optional: RFC3339 timestamp for order cancellation
	CancelResponse bool    // Optional: If true, response includes cancel_txid if amend cancels order
	Validate       bool    // Optional: Validate inputs only; do not submit order (send as "true" string if true)
}

// AmendOrderResponse represents the response from amending an order.
type AmendOrderResponse struct {
	TxID       string   `json:"txid"`                 // Original order ID
	AmendTxID  string   `json:"amend_txid"`           // Unique Kraken amend identifier
	Status     string   `json:"status"`               // Status of the amend request
	Descr      string   `json:"descr"`                // Human-readable description
	CancelTxID string   `json:"cancel_txid,omitempty"`// If amend resulted in cancellation
	Errors     []string `json:"errors,omitempty"`     // List of errors if any
}

// CancelAllOrdersResponse represents the response from cancelling all open orders.
type CancelAllOrdersResponse struct {
	Count int64 `json:"count"` // Number of orders canceled.
}

// CancelAllOrdersAfterResponse represents the response from the CancelAllOrdersAfter endpoint.
type CancelAllOrdersAfterResponse struct {
	CurrentTime string `json:"currentTime"` // Current server time
	TriggerTime string `json:"triggerTime"` // Time that the server will cancel orders
}

// BatchOrderRequest represents an individual order to be submitted in a batch.
// Fields are similar to AddOrderOptions, but types are adjusted for JSON array embedding.
// All prices and volumes should be strings to maintain precision.
type BatchOrderRequest struct {
	OrderType      string `json:"ordertype"` // e.g., "limit", "market"
	Type           string `json:"type"`      // "buy" or "sell"
	Volume         string `json:"volume"`
	Price          string `json:"price,omitempty"`
	Price2         string `json:"price2,omitempty"`
	Leverage       string `json:"leverage,omitempty"`
	OFlags         string `json:"oflags,omitempty"`
	StartTm        string `json:"starttm,omitempty"`  // Scheduled start time (0 or null for no schedule)
	ExpireTm       string `json:"expiretm,omitempty"` // Expiration time (0 or null for no expiration)
	UserRef        int32  `json:"userref,omitempty"`  // User reference ID (Ensure this can be omitted if 0)
	Validate       string `json:"validate,omitempty"` // "true" or "false" (or omitted)
	TimeInForce    string `json:"timeinforce,omitempty"` // e.g., GTC, IOC, GTD
	// Conditional close parameters
	CloseOrderType string `json:"close[ordertype],omitempty"`
	ClosePrice     string `json:"close[price],omitempty"`
	ClosePrice2    string `json:"close[price2],omitempty"`
}

// AddOrderBatchOptions represents parameters for the AddOrderBatch endpoint.
type AddOrderBatchOptions struct {
	Pair     string              `json:"pair"`    // Required
	Orders   []BatchOrderRequest `json:"orders"`  // Required, JSON encoded string for the form field
	Deadline string              `json:"deadline,omitempty"` // Optional RFC3339 timestamp
	Validate bool                `json:"validate,omitempty"` // Optional: Validate inputs only (send as "true" string if true)
}

// AddOrderBatchResponseEntry represents the result for a single order in a batch.
type AddOrderBatchResponseEntry struct {
	Descr  *OrderDescription `json:"descr,omitempty"` // Using existing OrderDescription
	TxID   string            `json:"txid,omitempty"`
	Error  string            `json:"error,omitempty"`  // Kraken might use "error" for single order errors
	Errors []string          `json:"errors,omitempty"` // Or "errors" for multiple issues
}

// AddOrderBatchResponse represents the response from the AddOrderBatch endpoint.
type AddOrderBatchResponse struct {
	Orders []AddOrderBatchResponseEntry `json:"orders"`
	// The top-level response might also have a general "error" field for batch-level errors
	Error []string `json:"error,omitempty"` // For batch-level errors
}

// CancelOrderBatchOptions represents parameters for the CancelOrderBatch endpoint.
type CancelOrderBatchOptions struct {
	Orders []string `json:"orders"` // Required: Array of order_id, userref, or cl_ord_id (max 50)
}

// TradeVolumeResponse type
type TradeVolumeResponse struct {
	Currency  string                    `json:"currency"`
	Volume    float64                   `json:"volume,string"`
	Fees      map[string]TradeVolumeFee `json:"fees"`
	FeesMaker map[string]TradeVolumeFee `json:"fees_maker"`
}

// TradeVolumeFee type
type TradeVolumeFee struct {
	Fee        float64 `json:"fee,string"`
	MinFee     float64 `json:"minfee,string"`
	MaxFee     float64 `json:"maxfee,string"`
	NextFee    float64 `json:"nextfee,string"`
	NextVolume float64 `json:"nextvolume,string"`
	TierVolume float64 `json:"tiervolume,string"`
}

// AddOrderResponse type
type AddOrderResponse struct {
	Description    OrderDescription `json:"descr"`
	TransactionIDs []string         `json:"txid"`
}

// WithdrawInformation Used to check withdrawal fees and info
type WithdrawInformation struct {
	Method      string  `json:"method"`
	Limit       float64 `json:"limit,string"` // Max net amount that can be withdrawn
	Amount      string  `json:"amount"`       // Net amount that will be sent, after fees
	Fee         float64 `json:"fee,string"`
	AddressName string  `json:"address_name,omitempty"` // Name of the address on file
}

// WithdrawResponse represents the response from a withdrawal request.
type WithdrawResponse struct {
	RefID string `json:"refid"` // Reference ID for the withdrawal
}

// DepositMethods Used to check deposit fees/methods
type DepositMethods struct {
	Method          string  `json:"method"`
	Limit           any     `json:"limit"` // string or false
	Fee             float64 `json:"fee,string,omitempty"` // Optional
	AddressSetupFee float64 `json:"address-setup-fee,string,omitempty"` // Optional
	GenAddress      bool    `json:"gen-address"` // Whether a new address can be generated
}

// OrderDescription represents an orders description
type OrderDescription struct {
	Close string `json:"close"`
	Order string `json:"order"`
}

// AddOrderOptions represents the AddOrder options
type AddOrderOptions struct {
	UserRef        int32
	OrderFlags     string  // maps to 'oflags'
	StartTm        string  // maps to 'starttm'
	ExpireTm       string  // maps to 'expiretm'
	CloseOrderType string  // maps to 'close[ordertype]'
	ClosePrice     float64 // maps to 'close[price]'
	ClosePrice2    float64 // maps to 'close[price2]'
	Validate       bool    // maps to 'validate'
	TimeInForce    string  // maps to 'timeinforce'
	Trigger        string  // New: maps to 'trigger' (last or index)
	ReduceOnly     bool    // New: maps to 'reduce_only'
	PostOnly       bool    // New: maps to 'post_only'
	StpType        string  // New: maps to 'stp_type' (self-trade prevention)
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

// DepositAddress defines a deposit address
type DepositAddress struct {
	Address    string `json:"address"`
	ExpireTime any    `json:"expiretm"` // this is an int when new is specified
	Tag        string `json:"tag"`
	New        bool   `json:"new"`
}

// WithdrawStatusOptions represents parameters for the WithdrawStatus endpoint.
type WithdrawStatusOptions struct {
	Asset   string // Required: Asset to check status for.
	Method  string // Optional: Withdrawal method name.
	Aclass  string // Optional: Asset class (default: "currency").
	Network string // Optional: Network to use.
	Cursor  bool   // Optional: If true, requests cursor for pagination.
	Limit   int    // Optional: Number of results (default/max 100 if cursor=true).
}

// WithdrawStatusResponse defines a single withdrawal status entry.
type WithdrawStatusResponse struct {
	Method        string   `json:"method"`
	Aclass        string   `json:"aclass"`
	Asset         string   `json:"asset"`
	RefID         string   `json:"refid"`
	TxID          string   `json:"txid"`
	Info          string   `json:"info"`
	Amount        float64  `json:"amount,string"`
	Fee           float64  `json:"fee,string"`
	Time          int64    `json:"time"` // Changed from float64
	Status        string   `json:"status"`
	StatusProp    string   `json:"status-prop,omitempty"` // Added
	Originators   []string `json:"originators,omitempty"` // Added
	Beneficiaries []string `json:"beneficiaries,omitempty"` // Added
}

// WithdrawCancelResponse represents the response from a withdrawal cancellation request.
type WithdrawCancelResponse struct {
	Result bool `json:"result"` // True if cancellation was successful/requested.
}

// DepositStatusOptions represents parameters for the DepositStatus endpoint.
type DepositStatusOptions struct {
	Asset   string // Optional: Asset to filter by.
	Aclass  string // Optional: Asset class (default: "currency").
	Method  string // Optional: Name of deposit method.
	Network string // Optional: Network to use.
	Cursor  bool   // Optional: If true, requests cursor for pagination.
	Limit   int    // Optional: Number of results (default/max 100 if cursor=true).
}

// DepositStatusEntry defines a single deposit status entry.
type DepositStatusEntry struct {
	Method        string   `json:"method"`
	Aclass        string   `json:"aclass"`
	Asset         string   `json:"asset"`
	RefID         string   `json:"refid"`
	TxID          string   `json:"txid"`
	Info          string   `json:"info"`
	Amount        float64  `json:"amount,string"`
	Fee           float64  `json:"fee,string"` // Typically 0 for deposits, but field exists
	Time          int64    `json:"time"`
	Status        string   `json:"status"`
	StatusProp    string   `json:"status-prop,omitempty"`
	Originators   []string `json:"originators,omitempty"`
	Beneficiaries []string `json:"beneficiaries,omitempty"`
}

// DepositStatusPage represents the full response for DepositStatus, possibly with a cursor.
type DepositStatusPage struct {
	Deposits   []DepositStatusEntry `json:"deposits"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

// WithdrawalMethodOptions represents parameters for the GetWithdrawalMethods endpoint.
type WithdrawalMethodOptions struct {
	Asset   string // Optional: Asset to get withdrawal methods for.
	Aclass  string // Optional: Asset class (default: "currency").
	Network string // Optional: Network to use.
}

// WithdrawalMethod represents a single withdrawal method.
type WithdrawalMethod struct {
	Asset   string `json:"asset"`   // Asset associated with the method
	Method  string `json:"method"`  // Display name of the withdrawal method
	Network string `json:"network"` // Network used by the method
	Minimum string `json:"minimum"` // Minimum net amount that can be withdrawn (as string)
}

// WithdrawalAddressOptions represents parameters for the GetWithdrawalAddresses endpoint.
type WithdrawalAddressOptions struct {
	Asset    string // Optional: Asset to get withdrawal addresses for.
	Aclass   string // Optional: Asset class (default: "currency").
	Network  string // Optional: Network to use.
	Method   string // Optional: Withdrawal method name to filter by.
	Key      string // Optional: Name of the withdrawal key to filter by.
	Verified bool   // Optional: Filter by verification status (default: false - any status).
}

// WithdrawalAddress represents a single withdrawal address.
type WithdrawalAddress struct {
	Address  string `json:"address"`  // Withdrawal address
	Asset    string `json:"asset"`    // Asset of the address
	Aclass   string `json:"aclass"`   // Asset class
	Network  string `json:"network"`  // Network of the address
	Key      string `json:"key"`      // Withdrawal key name
	Verified bool   `json:"verified"` // Whether the address is verified
	Method   string `json:"method"`   // Method name associated with this address
	Memo     string `json:"memo,omitempty"` // Memo/tag for the address, if applicable
}

// WalletTransferOptions represents parameters for the WalletTransfer endpoint.
type WalletTransferOptions struct {
	Asset  string // Required: Asset to transfer.
	Amount string // Required: Amount to transfer (as string for precision).
	From   string // Required: Source wallet name (e.g., "Spot Wallet").
	To     string // Required: Destination wallet name (e.g., "Futures Wallet").
}

// WalletTransferResponse represents the response from a wallet transfer request.
type WalletTransferResponse struct {
	RefID string `json:"refid"` // Reference ID for the transfer
}

// CreateSubaccountOptions represents parameters for the CreateSubaccount endpoint.
type CreateSubaccountOptions struct {
	Username string // Required: Username for the new subaccount.
	Email    string // Required: Email address for the new subaccount.
	Password string // Optional: Password. If not provided, subaccount is API-only.
}

// CreateSubaccountResponse represents the response from creating a subaccount.
type CreateSubaccountResponse struct {
	Result bool `json:"result"` // True if subaccount creation was successful/requested.
}

// AccountTransferOptions represents parameters for the AccountTransfer endpoint.
type AccountTransferOptions struct {
	Asset       string // Required: Asset to transfer.
	Amount      string // Required: Amount to transfer (as string for precision).
	FromAccount string // Required: Account ID of the source account.
	ToAccount   string // Required: Account ID of the destination account.
}

// AccountTransferResponse represents the response from an account transfer request.
type AccountTransferResponse struct {
	TransferID string `json:"transfer_id"` // Unique identifier for the transfer
	Status     string `json:"status"`      // Status of the transfer (e.g., "Success", "Pending")
}

// ListEarnStrategiesOptions represents parameters for the ListEarnStrategies endpoint.
type ListEarnStrategiesOptions struct {
	Asset    string // Optional: Comma-delimited list of assets to filter by.
	LockType string // Optional: Comma-delimited list of lock types (flex, bonded, instant).
	Cursor   string // Optional: For pagination (currently not implemented by Kraken per docs).
	Limit    int    // Optional: Number of results (currently not implemented by Kraken per docs).
}

// APREstimate holds the estimated APR range for an Earn strategy.
type APREstimate struct {
	Min string `json:"min"` // Minimum APR (string representation of a number)
	Max string `json:"max"` // Maximum APR (string representation of a number)
}

// YieldSource describes the source of the yield.
type YieldSource struct {
	Type string `json:"type"` // e.g., "staking"
}

// BondingInfo describes unbonding parameters if applicable.
type BondingInfo struct {
	Period  int  `json:"period"`  // Unbonding period in days
	Rewards bool `json:"rewards"` // If rewards accrue during unbonding
}

// AllocationRestrictionInfo describes why allocation might be restricted.
type AllocationRestrictionInfo struct {
	Type string `json:"type"` // e.g., "Tier"
}

// EarnStrategy represents a single Earn strategy.
type EarnStrategy struct {
	ID                        string                       `json:"id"`          // Unique ID for the strategy
	Asset                     string                       `json:"asset"`       // Asset of the strategy
	LockType                  string                       `json:"lock_type"`   // flex, bonded, or instant
	APREstimate               APREstimate                  `json:"apr_estimate"`
	UserMinAllocation         string                       `json:"user_min_allocation"` // Minimum amount user can allocate (string repr of number)
	AllocationAsset           string                       `json:"allocation_asset"`
	DeallocationAsset         string                       `json:"deallocation_asset"`
	CanAllocate               bool                         `json:"can_allocate"`
	CanDeallocate             bool                         `json:"can_deallocate"`
	IsHidden                  bool                         `json:"is_hidden"`
	YieldSource               YieldSource                  `json:"yield_source"`
	Bonding                   *BondingInfo                 `json:"bonding,omitempty"` // Optional
	AllocationRestrictionInfo *AllocationRestrictionInfo `json:"allocation_restriction_info,omitempty"` // Optional
}

// ListEarnStrategiesResponse represents the response from the ListEarnStrategies endpoint.
type ListEarnStrategiesResponse struct {
	Items      []EarnStrategy `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"` // For future pagination
}

// ListEarnAllocationsOptions represents parameters for the ListEarnAllocations endpoint.
type ListEarnAllocationsOptions struct {
	HideZeroAllocations bool   // Optional: If true, removes zero balance entries. Default false.
	ConvertedAsset      string // Optional: Asset to denominate all amounts in.
	Cursor              string // Optional: For pagination (currently not implemented by Kraken per docs).
	Limit               int    // Optional: Number of results (currently not implemented by Kraken per docs).
}

// BondingStatus represents the status of funds in bonding, unbonding, or exit_queue.
type BondingStatus struct {
	Amount            string `json:"amount"`                        // Amount in native_asset
	AmountConverted   string `json:"amount_converted,omitempty"`    // Amount in converted_asset
	Expires           int64  `json:"expires,omitempty"`           // Unix timestamp when period ends
	AccruedRewards    string `json:"accrued_rewards,omitempty"`   // (Not explicitly in listAllocations doc, but common for such states)
	RewardsAccruing   bool   `json:"rewards_accruing,omitempty"`  // (Not explicitly in listAllocations doc)
}

// EarnAllocation represents a single Earn allocation.
type EarnAllocation struct {
	StrategyID              string         `json:"strategy_id"`
	NativeAsset             string         `json:"native_asset"`
	TotalAllocated          string         `json:"total_allocated"`           // In native_asset
	TotalRewarded           string         `json:"total_rewarded"`            // In native_asset
	NextRewardTimestamp     int64          `json:"next_reward_timestamp,omitempty"`
	ConvertedAsset          string         `json:"converted_asset,omitempty"` // If conversion was requested
	TotalAllocatedConverted string         `json:"total_allocated_converted,omitempty"`
	TotalRewardedConverted  string         `json:"total_rewarded_converted,omitempty"`
	Bonding                 *BondingStatus `json:"bonding,omitempty"`
	Unbonding               *BondingStatus `json:"unbonding,omitempty"`
	ExitQueue               *BondingStatus `json:"exit_queue,omitempty"` // ETH only
}

// ListEarnAllocationsResponse represents the response from the ListEarnAllocations endpoint.
type ListEarnAllocationsResponse struct {
	Items      []EarnAllocation `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"` // For future pagination
}

// AllocateEarnFundsOptions represents parameters for the AllocateEarnFunds endpoint.
type AllocateEarnFundsOptions struct {
	StrategyID string // Required: ID of the Earn strategy.
	Amount     string // Required: Amount to allocate (as string for precision).
}

// AllocateEarnFundsResponse represents the response from allocating funds to an Earn strategy.
type AllocateEarnFundsResponse struct {
	// Assuming a simple boolean success for initiating the async operation, nested in "result".
	// This might need adjustment if the API returns a specific ID for the pending operation.
	Result bool `json:"result"`
}

// WebsocketSubRequest contains request data for Subscribe/Unsubscribe to channels
type WebsocketSubRequest struct {
	Event        string                    `json:"event"`
	RequestID    int64                     `json:"reqid,omitempty"`
	Pairs        []string                  `json:"pair,omitempty"`
	Subscription WebsocketSubscriptionData `json:"subscription"`
}

// WebsocketSubscriptionData contains details on WS channel
type WebsocketSubscriptionData struct {
	Name     string `json:"name,omitempty"`     // ticker|ohlc|trade|book|spread|*, * for all (ohlc interval value is 1 if all channels subscribed)
	Interval int    `json:"interval,omitempty"` // Optional - Timeframe for candles subscription in minutes; default 1. Valid: 1|5|15|30|60|240|1440|10080|21600
	Depth    int    `json:"depth,omitempty"`    // Optional - Depth associated with orderbook; default 10. Valid: 10|25|100|500|1000
	Token    string `json:"token,omitempty"`    // Optional - Token for authenticated channels
}

// WebsocketEventResponse holds all data response types
type WebsocketEventResponse struct {
	Event        string                            `json:"event"`
	Status       string                            `json:"status"`
	Pair         currency.Pair                     `json:"pair"`
	RequestID    int64                             `json:"reqid,omitempty"`
	Subscription WebsocketSubscriptionResponseData `json:"subscription"`
	ChannelName  string                            `json:"channelName,omitempty"`
	WebsocketSubscriptionEventResponse
	WebsocketErrorResponse
}

// WebsocketSubscriptionEventResponse defines a websocket socket event response
type WebsocketSubscriptionEventResponse struct {
	ChannelID int64 `json:"channelID"`
}

// WebsocketSubscriptionResponseData defines a websocket subscription response
type WebsocketSubscriptionResponseData struct {
	Name string `json:"name"`
}

// WebsocketErrorResponse defines a websocket error response
type WebsocketErrorResponse struct {
	ErrorMessage string `json:"errorMessage"`
}

// WsTokenResponse holds the WS auth token
type WsTokenResponse struct {
	Expires int64  `json:"expires"`
	Token   string `json:"token"`
}

type wsSystemStatus struct {
	ConnectionID float64 `json:"connectionID"`
	Event        string  `json:"event"`
	Status       string  `json:"status"`
	Version      string  `json:"version"`
}

// WsOpenOrder contains all open order data from ws feed
type WsOpenOrder struct {
	UserReferenceID int64   `json:"userref"`
	ExpireTime      float64 `json:"expiretm,string"`
	LastUpdated     float64 `json:"lastupdated,string"`
	OpenTime        float64 `json:"opentm,string"`
	StartTime       float64 `json:"starttm,string"`
	Fee             float64 `json:"fee,string"`
	LimitPrice      float64 `json:"limitprice,string"`
	StopPrice       float64 `json:"stopprice,string"`
	Volume          float64 `json:"vol,string"`
	ExecutedVolume  float64 `json:"vol_exec,string"`
	Cost            float64 `json:"cost,string"`
	AveragePrice    float64 `json:"avg_price,string"`
	Misc            string  `json:"misc"`
	OFlags          string  `json:"oflags"`
	RefID           string  `json:"refid"`
	Status          string  `json:"status"`
	Description     struct {
		Close     string  `json:"close"`
		Price     float64 `json:"price,string"`
		Price2    float64 `json:"price2,string"`
		Leverage  float64 `json:"leverage,string"`
		Order     string  `json:"order"`
		OrderType string  `json:"ordertype"`
		Pair      string  `json:"pair"`
		Type      string  `json:"type"`
	} `json:"descr"`
}

// WsOwnTrade ws auth owntrade data
type WsOwnTrade struct {
	Cost               float64 `json:"cost,string"`
	Fee                float64 `json:"fee,string"`
	Margin             float64 `json:"margin,string"`
	OrderTransactionID string  `json:"ordertxid"`
	OrderType          string  `json:"ordertype"`
	Pair               string  `json:"pair"`
	PostTransactionID  string  `json:"postxid"`
	Price              float64 `json:"price,string"`
	Time               float64 `json:"time,string"`
	Type               string  `json:"type"`
	Vol                float64 `json:"vol,string"`
}

// WsOpenOrders ws auth open order data
type WsOpenOrders struct {
	Cost           float64                `json:"cost,string"`
	Description    WsOpenOrderDescription `json:"descr"`
	ExpireTime     time.Time              `json:"expiretm"`
	Fee            float64                `json:"fee,string"`
	LimitPrice     float64                `json:"limitprice,string"`
	Misc           string                 `json:"misc"`
	OFlags         string                 `json:"oflags"`
	OpenTime       time.Time              `json:"opentm"`
	Price          float64                `json:"price,string"`
	RefID          string                 `json:"refid"`
	StartTime      time.Time              `json:"starttm"`
	Status         string                 `json:"status"`
	StopPrice      float64                `json:"stopprice,string"`
	UserReference  float64                `json:"userref"`
	Volume         float64                `json:"vol,string"`
	ExecutedVolume float64                `json:"vol_exec,string"`
}

// WsOpenOrderDescription additional data for WsOpenOrders
type WsOpenOrderDescription struct {
	Close     string  `json:"close"`
	Leverage  string  `json:"leverage"`
	Order     string  `json:"order"`
	OrderType string  `json:"ordertype"`
	Pair      string  `json:"pair"`
	Price     float64 `json:"price,string"`
	Price2    float64 `json:"price2,string"`
	Type      string  `json:"type"`
}

// WsAddOrderRequest request type for ws adding order
type WsAddOrderRequest struct {
	Event           string  `json:"event"`
	Token           string  `json:"token"`
	RequestID       int64   `json:"reqid,omitempty"` // Optional, client originated ID reflected in response message.
	OrderType       string  `json:"ordertype"`
	OrderSide       string  `json:"type"`
	Pair            string  `json:"pair"`
	Price           float64 `json:"price,string,omitempty"`  // optional
	Price2          float64 `json:"price2,string,omitempty"` // optional
	Volume          float64 `json:"volume,string,omitempty"`
	Leverage        float64 `json:"leverage,omitempty"`         // optional
	OFlags          string  `json:"oflags,omitempty"`           // optional
	StartTime       string  `json:"starttm,omitempty"`          // optional
	ExpireTime      string  `json:"expiretm,omitempty"`         // optional
	UserReferenceID string  `json:"userref,omitempty"`          // optional
	Validate        string  `json:"validate,omitempty"`         // optional
	CloseOrderType  string  `json:"close[ordertype],omitempty"` // optional
	ClosePrice      float64 `json:"close[price],omitempty"`     // optional
	ClosePrice2     float64 `json:"close[price2],omitempty"`    // optional
	TimeInForce     string  `json:"timeinforce,omitempty"`      // optional
}

// WsAddOrderResponse response data for ws order
type WsAddOrderResponse struct {
	Event         string `json:"event"`
	RequestID     int64  `json:"reqid"`
	Status        string `json:"status"`
	TransactionID string `json:"txid"`
	Description   string `json:"descr"`
	ErrorMessage  string `json:"errorMessage"`
}

// WsCancelOrderRequest request for ws cancel order
type WsCancelOrderRequest struct {
	Event          string   `json:"event"`
	Token          string   `json:"token"`
	TransactionIDs []string `json:"txid,omitempty"`
	RequestID      int64    `json:"reqid,omitempty"` // Optional, client originated ID reflected in response message.
}

// WsCancelOrderResponse response data for ws cancel order and ws cancel all orders
type WsCancelOrderResponse struct {
	Event        string `json:"event"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage"`
	RequestID    int64  `json:"reqid"`
	Count        int64  `json:"count"`
}

// OrderVars stores side, status and type for any order/trade
type OrderVars struct {
	Side      order.Side
	Status    order.Status
	OrderType order.Type
	Fee       float64
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
