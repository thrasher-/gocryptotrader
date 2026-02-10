package kraken

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types"
)

const (
	// Rate limit consts
	krakenRateInterval = time.Second
	krakenRequestRate  = 1

	// Status consts
	statusOpen = "open"
)

var (
	assetTranslator                 assetTranslatorStore
	errBadChannelSuffix             = errors.New("bad websocket channel suffix")
	errInvalidAssetPairInfo         = errors.New("parameter info can only be 'margin', 'leverage', 'fees' or 'info'")
	errInvalidDataReturned          = errors.New("invalid data returned")
	errTransactionIDRequired        = errors.New("transaction id is required")
	errIDRequired                   = errors.New("id is required")
	errNoAddressesReturned          = errors.New("no addresses returned")
	errOrderIDRequired              = errors.New("order id is required")
	errReportRequired               = errors.New("report is required")
	errFormatRequired               = errors.New("format is required")
	errTypeRequired                 = errors.New("type is required")
	errTimeoutMustBeGreaterThanZero = errors.New("timeout must be greater than zero")
	errOrdersRequired               = errors.New("orders are required")
	errOrdersOrClientOrdersRequired = errors.New("orders or client orders are required")
	errAssetRequired                = errors.New("asset is required")
	errFromRequired                 = errors.New("from is required")
	errToRequired                   = errors.New("to is required")
	errAmountRequired               = errors.New("amount is required")
	errUsernameRequired             = errors.New("username is required")
	errEmailRequired                = errors.New("email is required")
	errStrategyIDRequired           = errors.New("strategy id is required")
	errSymbolRequired               = errors.New("symbol is required")
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
	AclassBase      string `json:"aclass_base"`
	Decimals        int    `json:"decimals"`
	DisplayDecimals int    `json:"display_decimals"`
}

// GetAssetsRequest defines optional request params for the assets endpoint.
type GetAssetsRequest struct {
	Asset  string
	Aclass string
}

// SystemStatusResponse holds exchange system status information
type SystemStatusResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
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

// GetAssetPairsRequest defines optional request params for the asset pairs endpoint.
type GetAssetPairsRequest struct {
	AssetPairs     []string
	Info           string
	AssetClassBase string
	CountryCode    string
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
	Open                       types.Number    `json:"o"`
}

// GetTickerRequest defines optional request params for the ticker endpoint.
type GetTickerRequest struct {
	Pair       currency.Pair
	AssetClass string
}

// GetTickersRequest defines optional request params for the tickers endpoint.
type GetTickersRequest struct {
	PairList   string
	AssetClass string
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
	Count                      float64
}

// OHLCResponse defines typed OHLC payload values.
type OHLCResponse struct {
	Data map[string][]OHLCResponseItem
	Last types.Time
}

// OHLCResponseItem defines a typed OHLC entry.
type OHLCResponseItem struct {
	Time                       types.Time
	Open                       types.Number
	High                       types.Number
	Low                        types.Number
	Close                      types.Number
	VolumeWeightedAveragePrice types.Number
	Volume                     types.Number
	Count                      float64
}

// UnmarshalJSON unmarshals OHLC entries encoded as arrays.
func (r *OHLCResponseItem) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &[8]any{
		&r.Time,
		&r.Open,
		&r.High,
		&r.Low,
		&r.Close,
		&r.VolumeWeightedAveragePrice,
		&r.Volume,
		&r.Count,
	})
}

// UnmarshalJSON unmarshals OHLC response payload variants.
func (r *OHLCResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Data = make(map[string][]OHLCResponseItem)
	for key, value := range raw {
		if key == "last" {
			if err := json.Unmarshal(value, &r.Last); err != nil {
				return err
			}
			continue
		}

		var entries []OHLCResponseItem
		if err := json.Unmarshal(value, &entries); err != nil {
			return err
		}
		r.Data[key] = entries
	}
	return nil
}

// GetOHLCRequest defines optional request params for the OHLC endpoint.
type GetOHLCRequest struct {
	Pair       currency.Pair
	Interval   string
	Since      time.Time
	AssetClass string
}

// RecentTradesResponse holds recent trade data
type RecentTradesResponse struct {
	Trades map[string][]RecentTradeResponseItem
	Last   types.Time
}

// UnmarshalJSON unmarshals the recent trades response
func (r *RecentTradesResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Trades = make(map[string][]RecentTradeResponseItem)
	for key, raw := range raw {
		if key == "last" {
			if err := json.Unmarshal(raw, &r.Last); err != nil {
				return err
			}
		} else {
			var trades []RecentTradeResponseItem
			if err := json.Unmarshal(raw, &trades); err != nil {
				return err
			}
			r.Trades[key] = trades
		}
	}
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
	return json.Unmarshal(data, &[7]any{&r.Price, &r.Volume, &r.Time, &r.BuyOrSell, &r.MarketOrLimit, &r.Miscellaneous, &r.TradeID})
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

// GetDepthRequest defines optional request params for the order book endpoint.
type GetDepthRequest struct {
	Pair       currency.Pair
	Count      uint64
	AssetClass string
}

// SpreadItem holds the spread between trades
type SpreadItem struct {
	Time types.Time
	Bid  types.Number
	Ask  types.Number
}

// UnmarshalJSON unmarshals the spread item
func (s *SpreadItem) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &[3]any{&s.Time, &s.Bid, &s.Ask})
}

// SpreadResponse holds the spread response data
type SpreadResponse struct {
	Spreads map[string][]SpreadItem
	Last    types.Time
}

// GetTradesRequest defines optional request params for recent trades endpoint.
type GetTradesRequest struct {
	Pair       currency.Pair
	Since      time.Time
	Count      uint64
	AssetClass string
}

// GetSpreadRequest defines optional request params for spreads endpoint.
type GetSpreadRequest struct {
	Pair       currency.Pair
	Since      time.Time
	AssetClass string
}

// UnmarshalJSON unmarshals the spread response
func (s *SpreadResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s.Spreads = make(map[string][]SpreadItem)
	for key, raw := range raw {
		if key == "last" {
			if err := json.Unmarshal(raw, &s.Last); err != nil {
				return err
			}
		} else {
			var spreads []SpreadItem
			if err := json.Unmarshal(raw, &spreads); err != nil {
				return err
			}
			s.Spreads[key] = spreads
		}
	}
	return nil
}

// Balance represents account asset balances
type Balance struct {
	Total float64 `json:"balance,string"`
	Hold  float64 `json:"hold_trade,string"`
}

// GetAccountBalanceRequest defines optional request params for account balance endpoint.
type GetAccountBalanceRequest struct {
	RebaseMultiplier string
}

// GetExtendedBalanceRequest defines optional request params for extended balance endpoint.
type GetExtendedBalanceRequest struct {
	RebaseMultiplier string
}

// TradeBalanceOptions type
type TradeBalanceOptions struct {
	Asset            string
	RebaseMultiplier string
}

// TradeBalanceInfo type
type TradeBalanceInfo struct {
	EquivalentBalance float64 `json:"eb,string"` // combined balance of all currencies
	TradeBalance      float64 `json:"tb,string"` // combined balance of all equity currencies
	MarginAmount      float64 `json:"m,string"`  // margin amount of open positions
	Net               float64 `json:"n,string"`  // unrealized net profit/loss of open positions
	Equity            float64 `json:"e,string"`  // trade balance + unrealized net profit/loss
	FreeMargin        float64 `json:"mf,string"` // equity - initial margin (maximum margin available to open new positions)
	MarginLevel       float64 `json:"ml,string"` // (equity / initial margin) * 100
}

// OrderInfo type
type OrderInfo struct {
	RefID       string     `json:"refid"`
	UserRef     int32      `json:"userref"`
	Status      string     `json:"status"`
	OpenTime    types.Time `json:"opentm"`
	CloseTime   types.Time `json:"closetm"`
	StartTime   types.Time `json:"starttm"`
	ExpireTime  types.Time `json:"expiretm"`
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

// GetClosedOrdersOptions type
type GetClosedOrdersOptions struct {
	Trades           bool
	UserRef          int32
	ClientOrderID    string
	Start            string
	End              string
	Ofs              int64
	CloseTime        string
	ConsolidateTaker bool
	WithoutCount     bool
	RebaseMultiplier string
}

// OrderInfoOptions type
type OrderInfoOptions struct {
	Trades           bool
	UserRef          int32
	ClientOrderID    string
	ConsolidateTaker bool
	RebaseMultiplier string
}

// GetTradesHistoryOptions type
type GetTradesHistoryOptions struct {
	Type             string
	Trades           bool
	Start            string
	End              string
	Ofs              int64
	WithoutCount     bool
	ConsolidateTaker bool
	Ledgers          bool
	RebaseMultiplier string
}

// TradesHistory type
type TradesHistory struct {
	Trades map[string]TradeInfo `json:"trades"`
	Count  int64                `json:"count"`
}

// TradeInfo type
type TradeInfo struct {
	OrderTxID                  string     `json:"ordertxid"`
	Pair                       string     `json:"pair"`
	Time                       types.Time `json:"time"`
	Type                       string     `json:"type"`
	OrderType                  string     `json:"ordertype"`
	Price                      float64    `json:"price,string"`
	Cost                       float64    `json:"cost,string"`
	Fee                        float64    `json:"fee,string"`
	Volume                     float64    `json:"vol,string"`
	Margin                     float64    `json:"margin,string"`
	Misc                       string     `json:"misc"`
	PosTxID                    string     `json:"postxid"`
	ClosedPositionAveragePrice float64    `json:"cprice,string"`
	ClosedPositionFee          float64    `json:"cfee,string"`
	ClosedPositionVolume       float64    `json:"cvol,string"`
	ClosedPositionMargin       float64    `json:"cmargin,string"`
	Trades                     []string   `json:"trades"`
	PosStatus                  string     `json:"posstatus"`
}

// Position holds the opened position
type Position struct {
	Ordertxid      string     `json:"ordertxid"`
	Pair           string     `json:"pair"`
	Time           types.Time `json:"time"`
	Type           string     `json:"type"`
	OrderType      string     `json:"ordertype"`
	Cost           float64    `json:"cost,string"`
	Fee            float64    `json:"fee,string"`
	Volume         float64    `json:"vol,string"`
	VolumeClosed   float64    `json:"vol_closed,string"`
	Margin         float64    `json:"margin,string"`
	RolloverTime   int64      `json:"rollovertm,string"`
	Misc           string     `json:"misc"`
	OrderFlags     string     `json:"oflags"`
	PositionStatus string     `json:"posstatus"`
	Net            string     `json:"net"`
	Terms          string     `json:"terms"`
}

// OpenPositionsRequest defines request params for open positions endpoint.
type OpenPositionsRequest struct {
	TransactionIDList []string
	DoCalculations    bool
	Consolidation     string
	RebaseMultiplier  string
}

// GetLedgersOptions type
type GetLedgersOptions struct {
	Aclass           string
	Asset            string
	Type             string
	Start            string
	End              string
	Ofs              int64
	WithoutCount     bool
	RebaseMultiplier string
}

// Ledgers type
type Ledgers struct {
	Ledger map[string]LedgerInfo `json:"ledger"`
	Count  int64                 `json:"count"`
}

// QueryLedgersRequest defines request params for query ledgers endpoint.
type QueryLedgersRequest struct {
	ID               string
	IDs              []string
	Trades           bool
	RebaseMultiplier string
}

// LedgerInfo type
type LedgerInfo struct {
	Refid   string     `json:"refid"`
	Time    types.Time `json:"time"`
	Type    string     `json:"type"`
	Aclass  string     `json:"aclass"`
	Asset   string     `json:"asset"`
	Amount  float64    `json:"amount,string"`
	Fee     float64    `json:"fee,string"`
	Balance float64    `json:"balance,string"`
}

// TradeVolumeResponse type
type TradeVolumeResponse struct {
	Currency  string                    `json:"currency"`
	Volume    float64                   `json:"volume,string"`
	Fees      map[string]TradeVolumeFee `json:"fees"`
	FeesMaker map[string]TradeVolumeFee `json:"fees_maker"`
}

// QueryTradesRequest defines request params for query trades endpoint.
type QueryTradesRequest struct {
	TransactionID    string
	TransactionIDs   []string
	Trades           bool
	RebaseMultiplier string
}

// GetTradeVolumeRequest defines request params for trade volume endpoint.
type GetTradeVolumeRequest struct {
	Pairs            []currency.Pair
	RebaseMultiplier string
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

// WithdrawInformation Used to check withdrawal fees
type WithdrawInformation struct {
	Method string  `json:"method"`
	Limit  float64 `json:"limit,string"`
	Fee    float64 `json:"fee,string"`
}

// GetDepositMethodsRequest defines optional request params for deposit methods endpoint.
type GetDepositMethodsRequest struct {
	Asset            string
	AssetClass       string
	RebaseMultiplier string
}

// GetCryptoDepositAddressRequest defines optional request params for deposit addresses endpoint.
type GetCryptoDepositAddressRequest struct {
	Asset      string
	Method     string
	CreateNew  bool
	AssetClass string
	Amount     string
}

// DepositMethods Used to check deposit fees
type DepositMethods struct {
	Method          string  `json:"method"`
	Limit           any     `json:"limit"` // If no limit amount, this comes back as boolean
	Fee             float64 `json:"fee,string"`
	AddressSetupFee float64 `json:"address-setup-fee,string"`
}

// OrderDescription represents an orders description
type OrderDescription struct {
	Close string `json:"close"`
	Order string `json:"order"`
}

// AddOrderOptions represents the AddOrder options
type AddOrderOptions struct {
	UserRef         int32
	ClientOrderID   string
	OrderFlags      string
	StartTm         string
	ExpireTm        string
	AssetClass      string
	DisplayVolume   float64
	Trigger         string
	ReduceOnly      bool
	SelfTradePolicy string
	CloseOrderType  string
	ClosePrice      float64
	ClosePrice2     float64
	Validate        bool
	TimeInForce     string
	Deadline        string
}

// CancelOrderResponse type
type CancelOrderResponse struct {
	Count   int64 `json:"count"`
	Pending bool  `json:"pending"`
}

// GroupedOrderBookRequest defines request params for GroupedBook endpoint.
type GroupedOrderBookRequest struct {
	Pair     currency.Pair
	Depth    uint64
	Grouping uint64
}

// GroupedOrderBookResponse defines grouped L2 orderbook data.
type GroupedOrderBookResponse struct {
	Pair     string                  `json:"pair"`
	Grouping uint64                  `json:"grouping"`
	Bids     []GroupedOrderBookEntry `json:"bids"`
	Asks     []GroupedOrderBookEntry `json:"asks"`
}

// GroupedOrderBookEntry defines a grouped price level.
type GroupedOrderBookEntry struct {
	Price    types.Number `json:"price"`
	Quantity types.Number `json:"qty"`
}

// QueryLevel3OrderBookRequest defines request params for Level3 endpoint.
type QueryLevel3OrderBookRequest struct {
	Pair  currency.Pair
	Depth uint64
}

// QueryLevel3OrderBookResponse defines level 3 orderbook data.
type QueryLevel3OrderBookResponse struct {
	Pair string                 `json:"pair"`
	Bids []Level3OrderBookEntry `json:"bids"`
	Asks []Level3OrderBookEntry `json:"asks"`
}

// Level3OrderBookEntry defines a single level 3 orderbook entry.
type Level3OrderBookEntry struct {
	Price     types.Number `json:"price"`
	Quantity  types.Number `json:"qty"`
	OrderID   string       `json:"order_id"`
	Timestamp int64        `json:"timestamp"`
}

// GetCreditLinesRequest defines request params for credit lines endpoint.
type GetCreditLinesRequest struct {
	RebaseMultiplier string
}

// GetCreditLinesResponse defines credit line and monitoring values.
type GetCreditLinesResponse struct {
	AssetDetails  map[string]CreditLineAssetDetails `json:"asset_details"`
	LimitsMonitor CreditLineLimitsMonitor           `json:"limits_monitor"`
}

// CreditLineAssetDetails defines balance and credit details for an asset.
type CreditLineAssetDetails struct {
	Balance         types.Number `json:"balance"`
	CreditLimit     types.Number `json:"credit_limit"`
	CreditUsed      types.Number `json:"credit_used"`
	AvailableCredit types.Number `json:"available_credit"`
}

// CreditLineLimitsMonitor defines account-wide credit monitoring values.
type CreditLineLimitsMonitor struct {
	TotalCreditUSD          *types.Number `json:"total_credit_usd"`
	TotalCreditUsedUSD      *types.Number `json:"total_credit_used_usd"`
	TotalCollateralValueUSD *types.Number `json:"total_collateral_value_usd"`
	EquityUSD               *types.Number `json:"equity_usd"`
	OngoingBalance          *types.Number `json:"ongoing_balance"`
	DebtToEquity            *types.Number `json:"debt_to_equity"`
}

// GetOrderAmendsRequest defines request params for order amends endpoint.
type GetOrderAmendsRequest struct {
	OrderID          string
	RebaseMultiplier string
}

// GetOrderAmendsResponse defines order amend history data.
type GetOrderAmendsResponse struct {
	Count  uint64       `json:"count"`
	Amends []OrderAmend `json:"amends"`
}

// OrderAmend defines a single order amendment event.
type OrderAmend struct {
	AmendID       string       `json:"amend_id"`
	AmendType     string       `json:"amend_type"`
	OrderQuantity types.Number `json:"order_qty"`
	DisplayVolume types.Number `json:"display_qty"`
	RemainingQty  types.Number `json:"remaining_qty"`
	LimitPrice    types.Number `json:"limit_price"`
	TriggerPrice  types.Number `json:"trigger_price"`
	Reason        string       `json:"reason"`
	PostOnly      bool         `json:"post_only"`
	Timestamp     int64        `json:"timestamp"`
}

// RequestExportReportRequest defines request params for creating an export report.
type RequestExportReportRequest struct {
	Report      string
	Format      string
	Description string
	Fields      string
	StartTime   int64
	EndTime     int64
}

// RequestExportReportResponse defines an export report identifier.
type RequestExportReportResponse struct {
	ID string `json:"id"`
}

// GetExportReportStatusRequest defines request params for export status endpoint.
type GetExportReportStatusRequest struct {
	Report string
}

// ExportReportStatusResponse defines export report status details.
type ExportReportStatusResponse struct {
	ID            string `json:"id"`
	Description   string `json:"descr"`
	Format        string `json:"format"`
	Report        string `json:"report"`
	Subtype       string `json:"subtype"`
	Status        string `json:"status"`
	Flags         string `json:"flags"`
	Fields        string `json:"fields"`
	CreatedTime   string `json:"createdtm"`
	ExpiryTime    string `json:"expiretm"`
	StartTime     string `json:"starttm"`
	CompletedTime string `json:"completedtm"`
	DataStartTime string `json:"datastarttm"`
	DataEndTime   string `json:"dataendtm"`
	AssetClass    string `json:"aclass"`
	Asset         string `json:"asset"`
}

// RetrieveDataExportRequest defines request params for data export retrieval.
type RetrieveDataExportRequest struct {
	ID string
}

// DeleteExportReportRequest defines request params for removing an export report.
type DeleteExportReportRequest struct {
	ID   string
	Type string
}

// DeleteExportReportResponse defines export removal flags.
type DeleteExportReportResponse struct {
	Delete bool `json:"delete"`
	Cancel bool `json:"cancel"`
}

// AmendOrderRequest defines request params for amending an open order.
type AmendOrderRequest struct {
	TransactionID   string
	ClientOrderID   string
	OrderQuantity   string
	DisplayQuantity string
	LimitPrice      string
	TriggerPrice    string
	Pair            string
	PostOnly        bool
	Deadline        string
}

// AmendOrderResponse defines an order amend identifier.
type AmendOrderResponse struct {
	AmendID string `json:"amend_id"`
}

// CancelAllOrdersAfterRequest defines request params for timed cancel-all.
type CancelAllOrdersAfterRequest struct {
	Timeout uint64
}

// CancelAllOrdersAfterResponse defines cancel-all trigger timing.
type CancelAllOrdersAfterResponse struct {
	CurrentTime string `json:"currentTime"`
	TriggerTime string `json:"triggerTime"`
}

// AddOrderBatchRequest defines request params for batch order placement.
type AddOrderBatchRequest struct {
	Orders     []AddOrderBatchOrderRequest
	Pair       string
	AssetClass string
	Deadline   string
	Validate   bool
}

// AddOrderBatchOrderRequest defines a single batch order request.
type AddOrderBatchOrderRequest struct {
	UserReference   int32  `json:"userref,omitempty"`
	ClientOrderID   string `json:"cl_ord_id,omitempty"`
	OrderType       string `json:"ordertype,omitempty"`
	OrderSide       string `json:"type,omitempty"`
	Volume          string `json:"volume,omitempty"`
	DisplayVolume   string `json:"displayvol,omitempty"`
	Price           string `json:"price,omitempty"`
	SecondaryPrice  string `json:"price2,omitempty"`
	Trigger         string `json:"trigger,omitempty"`
	Leverage        string `json:"leverage,omitempty"`
	ReduceOnly      bool   `json:"reduce_only,omitempty"`
	SelfTradePolicy string `json:"stptype,omitempty"`
	OrderFlags      string `json:"oflags,omitempty"`
	TimeInForce     string `json:"timeinforce,omitempty"`
	StartTime       string `json:"starttm,omitempty"`
	ExpireTime      string `json:"expiretm,omitempty"`
}

// AddOrderBatchResponse defines batch placement results.
type AddOrderBatchResponse struct {
	Orders []AddOrderBatchOrderResponse `json:"orders"`
}

// AddOrderBatchOrderResponse defines a single placed order response.
type AddOrderBatchOrderResponse struct {
	Description AddOrderBatchOrderDescription `json:"descr"`
	Error       string                        `json:"error"`
	Transaction string                        `json:"txid"`
}

// AddOrderBatchOrderDescription defines order description details.
type AddOrderBatchOrderDescription struct {
	Order string `json:"order"`
}

// CancelOrderBatchRequest defines request params for batch cancellation.
type CancelOrderBatchRequest struct {
	Orders      []CancelOrderBatchOrderRequest
	ClientOrder []CancelOrderBatchClientOrderIDItem
}

// CancelOrderBatchOrderRequest defines a transaction-id batch cancel item.
type CancelOrderBatchOrderRequest struct {
	TransactionID string `json:"txid"`
}

// CancelOrderBatchClientOrderIDItem defines a client-order-id batch cancel item.
type CancelOrderBatchClientOrderIDItem struct {
	ClientOrderID string `json:"cl_ord_id"`
}

// CancelOrderBatchResponse defines batch cancellation totals.
type CancelOrderBatchResponse struct {
	Count uint64 `json:"count"`
}

// EditOrderRequest defines request params for editing an open order.
type EditOrderRequest struct {
	UserReference  int32
	TransactionID  string
	Volume         string
	DisplayVolume  string
	Pair           string
	AssetClass     string
	Price          string
	SecondaryPrice string
	OrderFlags     string
	Deadline       string
	CancelResponse bool
	Validate       bool
}

// EditOrderResponse defines edit order response data.
type EditOrderResponse struct {
	Description           AddOrderBatchOrderDescription `json:"descr"`
	TransactionID         string                        `json:"txid"`
	NewUserReference      string                        `json:"newuserref"`
	OldUserReference      string                        `json:"olduserref"`
	OrdersCancelled       uint64                        `json:"orders_cancelled"`
	OriginalTransactionID string                        `json:"originaltxid"`
	Status                string                        `json:"status"`
	Volume                types.Number                  `json:"volume"`
	Price                 types.Number                  `json:"price"`
	SecondaryPrice        types.Number                  `json:"price2"`
	ErrorMessage          string                        `json:"error_message"`
}

// GetRecentDepositsStatusRequest defines request params for recent deposits status endpoint.
type GetRecentDepositsStatusRequest struct {
	Asset            string
	AssetClass       string
	Method           string
	Start            string
	End              string
	Cursor           string
	Limit            uint64
	RebaseMultiplier string
}

// RecentDepositsStatusResponse defines recent deposit status response payload.
type RecentDepositsStatusResponse struct {
	Deposits   []RecentDepositStatus `json:"-"`
	NextCursor string                `json:"-"`
}

// UnmarshalJSON unmarshals deposit status payload variants.
func (r *RecentDepositsStatusResponse) UnmarshalJSON(data []byte) error {
	var depositList []RecentDepositStatus
	if err := json.Unmarshal(data, &depositList); err == nil {
		r.Deposits = depositList
		return nil
	}

	var singleDeposit RecentDepositStatus
	if err := json.Unmarshal(data, &singleDeposit); err == nil && (singleDeposit.ReferenceID != "" || singleDeposit.TransactionID != "") {
		r.Deposits = []RecentDepositStatus{singleDeposit}
		return nil
	}

	var paginated struct {
		Deposit    json.RawMessage `json:"deposit"`
		NextCursor string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(data, &paginated); err != nil {
		return err
	}

	r.NextCursor = paginated.NextCursor
	if len(paginated.Deposit) == 0 {
		return nil
	}

	if err := json.Unmarshal(paginated.Deposit, &depositList); err == nil {
		r.Deposits = depositList
		return nil
	}

	if err := json.Unmarshal(paginated.Deposit, &singleDeposit); err != nil {
		return err
	}
	r.Deposits = []RecentDepositStatus{singleDeposit}
	return nil
}

// RecentDepositStatus defines an individual deposit status.
type RecentDepositStatus struct {
	Method           string       `json:"method"`
	AssetClass       string       `json:"aclass"`
	Asset            string       `json:"asset"`
	ReferenceID      string       `json:"refid"`
	TransactionID    string       `json:"txid"`
	Information      string       `json:"info"`
	Amount           types.Number `json:"amount"`
	Fee              types.Number `json:"fee"`
	Time             types.Time   `json:"time"`
	Status           string       `json:"status"`
	StatusProperties string       `json:"status-prop"`
	Originators      []string     `json:"originators"`
}

// GetWithdrawalMethodsRequest defines request params for withdrawal methods.
type GetWithdrawalMethodsRequest struct {
	Asset            string
	AssetClass       string
	Network          string
	RebaseMultiplier string
}

// WithdrawalMethodResponse defines a withdrawal method entry.
type WithdrawalMethodResponse struct {
	Asset   string       `json:"asset"`
	Method  string       `json:"method"`
	Network string       `json:"network"`
	Minimum types.Number `json:"minimum"`
}

// GetWithdrawalAddressesRequest defines request params for withdrawal addresses.
type GetWithdrawalAddressesRequest struct {
	Asset      string
	AssetClass string
	Method     string
	Key        string
	Verified   *bool
}

// WithdrawalAddressResponse defines a withdrawal address entry.
type WithdrawalAddressResponse struct {
	Address  string `json:"address"`
	Asset    string `json:"asset"`
	Method   string `json:"method"`
	Key      string `json:"key"`
	Tag      string `json:"tag"`
	Verified bool   `json:"verified"`
}

// WalletTransferRequest defines request params for wallet transfer.
type WalletTransferRequest struct {
	Asset  string
	From   string
	To     string
	Amount string
}

// WalletTransferResponse defines a wallet transfer reference.
type WalletTransferResponse struct {
	ReferenceID string `json:"refid"`
}

// CreateSubaccountRequest defines request params for subaccount creation.
type CreateSubaccountRequest struct {
	Username string
	Email    string
}

// CreateSubaccountResponse defines subaccount creation result.
type CreateSubaccountResponse struct {
	Created bool
}

// UnmarshalJSON unmarshals subaccount creation bool responses.
func (r *CreateSubaccountResponse) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &r.Created)
}

// AccountTransferRequest defines request params for account transfer.
type AccountTransferRequest struct {
	Asset      string
	AssetClass string
	Amount     string
	From       string
	To         string
}

// AccountTransferResponse defines account transfer response payload.
type AccountTransferResponse struct {
	TransferID string `json:"transfer_id"`
	Status     string `json:"status"`
}

// AllocateEarnFundsRequest defines request params for earn allocation.
type AllocateEarnFundsRequest struct {
	Amount     string
	StrategyID string
}

// AllocateEarnFundsResponse defines earn allocation response payload.
type AllocateEarnFundsResponse struct {
	Success *bool
}

// UnmarshalJSON unmarshals nullable allocation result values.
func (r *AllocateEarnFundsResponse) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		r.Success = nil
		return nil
	}
	var success bool
	if err := json.Unmarshal(data, &success); err != nil {
		return err
	}
	r.Success = &success
	return nil
}

// DeallocateEarnFundsRequest defines request params for earn deallocation.
type DeallocateEarnFundsRequest struct {
	Amount     string
	StrategyID string
}

// DeallocateEarnFundsResponse defines earn deallocation response payload.
type DeallocateEarnFundsResponse struct {
	Success *bool
}

// UnmarshalJSON unmarshals nullable deallocation result values.
func (r *DeallocateEarnFundsResponse) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		r.Success = nil
		return nil
	}
	var success bool
	if err := json.Unmarshal(data, &success); err != nil {
		return err
	}
	r.Success = &success
	return nil
}

// EarnOperationStatusRequest defines request params for earn operation status endpoints.
type EarnOperationStatusRequest struct {
	StrategyID string
}

// EarnOperationStatusResponse defines earn operation status payload.
type EarnOperationStatusResponse struct {
	Pending bool `json:"pending"`
}

// ListEarnStrategiesRequest defines request params for earn strategies endpoint.
type ListEarnStrategiesRequest struct {
	Ascending *bool
	Asset     string
	Cursor    string
	Limit     uint64
	LockType  []string
}

// ListEarnStrategiesResponse defines earn strategies response payload.
type ListEarnStrategiesResponse struct {
	Items      []EarnStrategy `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

// EarnStrategy defines a single earn strategy.
type EarnStrategy struct {
	ID                        string                   `json:"id"`
	Asset                     string                   `json:"asset"`
	LockType                  EarnStrategyLockType     `json:"lock_type"`
	APREstimate               *EarnStrategyAPR         `json:"apr_estimate"`
	UserCap                   *types.Number            `json:"user_cap"`
	UserMinimumAllocation     *types.Number            `json:"user_min_allocation"`
	AllocationFee             types.Number             `json:"allocation_fee"`
	DeallocationFee           types.Number             `json:"deallocation_fee"`
	AutoCompound              EarnStrategyAutoCompound `json:"auto_compound"`
	YieldSource               EarnStrategyYieldSource  `json:"yield_source"`
	CanAllocate               bool                     `json:"can_allocate"`
	CanDeallocate             bool                     `json:"can_deallocate"`
	AllocationRestrictionInfo []string                 `json:"allocation_restriction_info"`
}

// EarnStrategyAPR defines APR range metadata for a strategy.
type EarnStrategyAPR struct {
	Low  string `json:"low"`
	High string `json:"high"`
}

// EarnStrategyLockType defines strategy locking model metadata.
type EarnStrategyLockType struct {
	Type                    string `json:"type"`
	BondingPeriod           uint64 `json:"bonding_period"`
	BondingPeriodVariable   bool   `json:"bonding_period_variable"`
	BondingRewards          bool   `json:"bonding_rewards"`
	ExitQueuePeriod         uint64 `json:"exit_queue_period"`
	PayoutFrequency         uint64 `json:"payout_frequency"`
	UnbondingPeriod         uint64 `json:"unbonding_period"`
	UnbondingPeriodVariable bool   `json:"unbonding_period_variable"`
	UnbondingRewards        bool   `json:"unbonding_rewards"`
}

// EarnStrategyAutoCompound defines auto-compounding behavior.
type EarnStrategyAutoCompound struct {
	Type    string `json:"type"`
	Default bool   `json:"default"`
}

// EarnStrategyYieldSource defines strategy yield source metadata.
type EarnStrategyYieldSource struct {
	Type string `json:"type"`
}

// ListEarnAllocationsRequest defines request params for earn allocations endpoint.
type ListEarnAllocationsRequest struct {
	Ascending           *bool
	ConvertedAsset      string
	HideZeroAllocations *bool
}

// ListEarnAllocationsResponse defines earn allocations response payload.
type ListEarnAllocationsResponse struct {
	ConvertedAsset string           `json:"converted_asset"`
	TotalAllocated types.Number     `json:"total_allocated"`
	TotalRewarded  types.Number     `json:"total_rewarded"`
	Items          []EarnAllocation `json:"items"`
}

// EarnAllocation defines allocation data for a strategy.
type EarnAllocation struct {
	StrategyID      string                `json:"strategy_id"`
	NativeAsset     string                `json:"native_asset"`
	AmountAllocated EarnAllocationAmount  `json:"amount_allocated"`
	TotalRewarded   EarnAllocationReward  `json:"total_rewarded"`
	Payout          *EarnAllocationPayout `json:"payout"`
}

// EarnAllocationAmount defines allocation amounts by state.
type EarnAllocationAmount struct {
	Bonding   *EarnAllocationAmountState `json:"bonding"`
	ExitQueue *EarnAllocationAmountState `json:"exit_queue"`
	Pending   *EarnAllocationAmountState `json:"pending"`
	Total     EarnAllocationAmountState  `json:"total"`
	Unbonding *EarnAllocationAmountState `json:"unbonding"`
}

// EarnAllocationAmountState defines allocation state amounts and details.
type EarnAllocationAmountState struct {
	Native          types.Number           `json:"native"`
	Converted       types.Number           `json:"converted"`
	AllocationCount uint64                 `json:"allocation_count,omitempty"`
	Allocations     []EarnAllocationDetail `json:"allocations,omitempty"`
}

// EarnAllocationDetail defines a granular allocation event.
type EarnAllocationDetail struct {
	Native    types.Number `json:"native"`
	Converted types.Number `json:"converted"`
	CreatedAt time.Time    `json:"created_at"`
	Expires   time.Time    `json:"expires"`
}

// EarnAllocationPayout defines payout period reward details.
type EarnAllocationPayout struct {
	AccumulatedReward EarnAllocationReward `json:"accumulated_reward"`
	EstimatedReward   EarnAllocationReward `json:"estimated_reward"`
	PeriodStart       time.Time            `json:"period_start"`
	PeriodEnd         time.Time            `json:"period_end"`
}

// EarnAllocationReward defines native and converted reward values.
type EarnAllocationReward struct {
	Native    types.Number `json:"native"`
	Converted types.Number `json:"converted"`
}

// GetPreTradeDataRequest defines request params for pre-trade transparency endpoint.
type GetPreTradeDataRequest struct {
	Symbol string
}

// GetPreTradeDataResponse defines pre-trade transparency response payload.
type GetPreTradeDataResponse struct {
	Symbol        string              `json:"symbol"`
	Description   string              `json:"description"`
	BaseAsset     string              `json:"base_asset"`
	BaseNotation  string              `json:"base_notation"`
	QuoteAsset    string              `json:"quote_asset"`
	QuoteNotation string              `json:"quote_notation"`
	Venue         string              `json:"venue"`
	System        string              `json:"system"`
	Bids          []PreTradeBookLevel `json:"bids"`
	Asks          []PreTradeBookLevel `json:"asks"`
}

// PreTradeBookLevel defines a pre-trade transparency orderbook level.
type PreTradeBookLevel struct {
	Side                 string       `json:"side"`
	Price                types.Number `json:"price"`
	Quantity             types.Number `json:"qty"`
	Count                uint64       `json:"count"`
	PublicationTimestamp time.Time    `json:"publication_ts"`
}

// GetPostTradeDataRequest defines request params for post-trade transparency endpoint.
type GetPostTradeDataRequest struct {
	Symbol        string
	FromTimestamp time.Time
	ToTimestamp   time.Time
	Count         uint64
}

// GetPostTradeDataResponse defines post-trade transparency response payload.
type GetPostTradeDataResponse struct {
	LastTimestamp time.Time       `json:"last_ts"`
	Count         uint64          `json:"count"`
	Trades        []PostTradeData `json:"trades"`
}

// PostTradeData defines a post-trade transparency trade record.
type PostTradeData struct {
	TradeID              string       `json:"trade_id"`
	Price                types.Number `json:"price"`
	Quantity             types.Number `json:"quantity"`
	Symbol               string       `json:"symbol"`
	Description          string       `json:"description"`
	BaseAsset            string       `json:"base_asset"`
	BaseNotation         string       `json:"base_notation"`
	QuoteAsset           string       `json:"quote_asset"`
	QuoteNotation        string       `json:"quote_notation"`
	TradeVenue           string       `json:"trade_venue"`
	TradeTimestamp       time.Time    `json:"trade_ts"`
	PublicationVenue     string       `json:"publication_venue"`
	PublicationTimestamp time.Time    `json:"publication_ts"`
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

// WithdrawStatusResponse defines a withdrawal status response
type WithdrawStatusResponse struct {
	Method string     `json:"method"`
	Aclass string     `json:"aclass"`
	Asset  string     `json:"asset"`
	Refid  string     `json:"refid"`
	TxID   string     `json:"txid"`
	Info   string     `json:"info"`
	Amount float64    `json:"amount,string"`
	Fee    float64    `json:"fee,string"`
	Time   types.Time `json:"time"`
	Status string     `json:"status"`
}

// WithdrawStatusRequest defines optional request params for withdrawal status endpoint.
type WithdrawStatusRequest struct {
	Asset            currency.Code
	Method           string
	AssetClass       string
	Start            string
	End              string
	Cursor           string
	Limit            uint64
	RebaseMultiplier string
}

// WithdrawRequest defines optional request params for withdraw funds endpoint.
type WithdrawRequest struct {
	Asset            string
	Key              string
	Amount           float64
	AssetClass       string
	Address          string
	MaxFee           string
	RebaseMultiplier string
}

// WithdrawResponse defines response params for withdraw funds endpoint.
type WithdrawResponse struct {
	ReferenceID string `json:"refid"`
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
	UserReferenceID int64      `json:"userref"`
	ExpireTime      types.Time `json:"expiretm"`
	LastUpdated     types.Time `json:"lastupdated"`
	OpenTime        types.Time `json:"opentm"`
	StartTime       types.Time `json:"starttm"`
	Fee             float64    `json:"fee,string"`
	LimitPrice      float64    `json:"limitprice,string"`
	StopPrice       float64    `json:"stopprice,string"`
	Volume          float64    `json:"vol,string"`
	ExecutedVolume  float64    `json:"vol_exec,string"`
	Cost            float64    `json:"cost,string"`
	AveragePrice    float64    `json:"avg_price,string"`
	Misc            string     `json:"misc"`
	OFlags          string     `json:"oflags"`
	RefID           string     `json:"refid"`
	Status          string     `json:"status"`
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
	Cost               float64    `json:"cost,string"`
	Fee                float64    `json:"fee,string"`
	Margin             float64    `json:"margin,string"`
	OrderTransactionID string     `json:"ordertxid"`
	OrderType          string     `json:"ordertype"`
	Pair               string     `json:"pair"`
	PostTransactionID  string     `json:"postxid"`
	Price              float64    `json:"price,string"`
	Time               types.Time `json:"time"`
	Type               string     `json:"type"`
	Vol                float64    `json:"vol,string"`
}

// WsOpenOrders ws auth open order data
type WsOpenOrders struct {
	Cost           float64                `json:"cost,string"`
	Description    WsOpenOrderDescription `json:"descr"`
	ExpireTime     types.Time             `json:"expiretm"`
	Fee            float64                `json:"fee,string"`
	LimitPrice     float64                `json:"limitprice,string"`
	Misc           string                 `json:"misc"`
	OFlags         string                 `json:"oflags"`
	OpenTime       types.Time             `json:"opentm"`
	Price          float64                `json:"price,string"`
	RefID          string                 `json:"refid"`
	StartTime      types.Time             `json:"starttm"`
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

type wsTicker struct {
	Ask                        [3]types.Number `json:"a"`
	Bid                        [3]types.Number `json:"b"`
	Last                       [2]types.Number `json:"c"`
	Volume                     [2]types.Number `json:"v"`
	VolumeWeightedAveragePrice [2]types.Number `json:"p"`
	Trades                     [2]int64        `json:"t"`
	Low                        [2]types.Number `json:"l"`
	High                       [2]types.Number `json:"h"`
	Open                       [2]types.Number `json:"o"`
}

type wsSpread struct {
	Bid       types.Number
	Ask       types.Number
	Time      types.Time
	BidVolume types.Number
	AskVolume types.Number
}

func (w *wsSpread) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &[5]any{&w.Bid, &w.Ask, &w.Time, &w.BidVolume, &w.AskVolume})
}

type wsTrades struct {
	Price     types.Number
	Volume    types.Number
	Time      types.Time
	Side      string
	OrderType string
	Misc      string
}

func (w *wsTrades) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &[6]any{&w.Price, &w.Volume, &w.Time, &w.Side, &w.OrderType, &w.Misc})
}

type wsCandle struct {
	LastUpdateTime types.Time
	EndTime        types.Time
	Open           types.Number
	High           types.Number
	Low            types.Number
	Close          types.Number
	VWAP           types.Number
	Volume         types.Number
	Count          int64
}

func (w *wsCandle) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &[9]any{&w.LastUpdateTime, &w.EndTime, &w.Open, &w.High, &w.Low, &w.Close, &w.VWAP, &w.Volume, &w.Count})
}

type wsSnapshot struct {
	Asks []wsOrderbookItem `json:"as"`
	Bids []wsOrderbookItem `json:"bs"`
}

type wsUpdate struct {
	Asks     []wsOrderbookItem `json:"a"`
	Bids     []wsOrderbookItem `json:"b"`
	Checksum uint32            `json:"c,string"`
}

type wsOrderbookItem struct {
	Price     float64
	PriceRaw  string
	Amount    float64
	AmountRaw string
	Time      types.Time
}

func (ws *wsOrderbookItem) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, &[3]any{&ws.PriceRaw, &ws.AmountRaw, &ws.Time})
	if err != nil {
		return err
	}
	ws.Price, err = strconv.ParseFloat(ws.PriceRaw, 64)
	if err != nil {
		return fmt.Errorf("error parsing price: %w", err)
	}
	ws.Amount, err = strconv.ParseFloat(ws.AmountRaw, 64)
	if err != nil {
		return fmt.Errorf("error parsing amount: %w", err)
	}
	return nil
}
