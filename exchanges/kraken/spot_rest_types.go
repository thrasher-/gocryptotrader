package kraken

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var (
	errAmountInvalid              = errors.New("amount must be greater than zero")
	errAssetRequired              = errors.New("asset is required")
	errAssetClassInvalid          = errors.New("asset class is not supported by this endpoint")
	errAssetVersionInvalid        = errors.New("asset version must be 1 when set")
	errBatchCancelOrderCount      = errors.New("batch cancellation must contain between 1 and 50 order identifiers")
	errBatchCloseTypeInvalid      = errors.New("batch conditional close order type is invalid")
	errBatchIdentityConflict      = errors.New("batch order user reference and client order identifier are mutually exclusive")
	errBatchOrderCount            = errors.New("batch order request must contain between 2 and 15 orders")
	errBatchOrderFields           = errors.New("batch orders require order type and side")
	errBatchOrderTypeInvalid      = errors.New("batch order type is invalid")
	errBatchSelfTradeInvalid      = errors.New("batch order self-trade policy is invalid")
	errBatchSideInvalid           = errors.New("batch order side is invalid")
	errBatchTimeInForceInvalid    = errors.New("batch order time-in-force is invalid")
	errBatchTriggerInvalid        = errors.New("batch order trigger is invalid")
	errDescriptionRequired        = errors.New("description is required")
	errCloseTimeInvalid           = errors.New("close time must be open, close, or both")
	errConsolidationInvalid       = errors.New("position consolidation must be market")
	errCursorConflict             = errors.New("cursor value and pagination toggle are mutually exclusive")
	errDeadlineInvalid            = errors.New("deadline must be between 2 and 60 seconds in the future")
	errDepthCountInvalid          = errors.New("order book count must be omitted or between 1 and 500")
	errDepositResultInvalid       = errors.New("deposit result contains no deposit fields")
	errEmailRequired              = errors.New("email is required")
	errEarnLockTypeInvalid        = errors.New("earn strategy lock type is invalid")
	errExportFormatInvalid        = errors.New("export format must be CSV or TSV")
	errExportFieldInvalid         = errors.New("export field is invalid for the selected report")
	errExportRemovalInvalid       = errors.New("export removal type must be cancel or delete")
	errExportReportInvalid        = errors.New("export report type must be trades or ledgers")
	errExecutionVenueInvalid      = errors.New("execution venue must be international or bitnomial_exchange")
	errFromRequired               = errors.New("source account is required")
	errFromWalletInvalid          = errors.New("source wallet must be Spot Wallet")
	errGroupedDepthInvalid        = errors.New("grouped order book depth must be omitted or 10, 25, 100, 250, or 1000")
	errGroupingInvalid            = errors.New("order book grouping must be omitted or 1, 5, 10, 25, 50, 100, 250, 500, or 1000")
	errIDRequired                 = errors.New("identifier is required")
	errInfoInvalid                = errors.New("asset pair info must be info, leverage, fees, or margin")
	errIntervalInvalid            = errors.New("OHLC interval is invalid")
	errKeyRequired                = errors.New("key is required")
	errLedgerIdentifierCount      = errors.New("ledger query must contain between 1 and 20 identifiers")
	errLedgerTypeInvalid          = errors.New("ledger type is invalid")
	errLevel3DepthInvalid         = errors.New("level 3 order book depth must be 0, 10, 25, 100, 250, or 1000")
	errMaximumFeeInvalid          = errors.New("maximum fee must not be negative")
	errMethodRequired             = errors.New("method is required")
	errOrderIDRequired            = errors.New("order identifier is required")
	errOrderIdentifierCount       = errors.New("order query must contain between 1 and 50 identifiers")
	errOrderIdentityConflict      = errors.New("order identity fields are mutually exclusive")
	errOrderIdentityRequired      = errors.New("transaction or client order identifier is required")
	errOrderFlagInvalid           = errors.New("order flag is invalid")
	errOrderPriceConflict         = errors.New("absolute and relative order prices are mutually exclusive")
	errOrderPriceInvalid          = errors.New("order price is invalid")
	errOrderSideInvalid           = errors.New("order side must be buy or sell")
	errOrderTypeInvalid           = errors.New("order type is invalid")
	errPaginatedDepositInvalid    = errors.New("paginated deposit result contains no deposit fields")
	errPaginatedWithdrawalInvalid = errors.New("paginated withdrawal result contains no withdrawal fields")
	errPairRequired               = errors.New("pair is required")
	errPostTradeCountTooLarge     = errors.New("post-trade count cannot exceed 1000")
	errReportRequired             = errors.New("report type is required")
	errRebaseMultiplierInvalid    = errors.New("rebase multiplier must be rebased or base")
	errReferenceIDRequired        = errors.New("reference identifier is required")
	errStrategyIDRequired         = errors.New("earn strategy identifier is required")
	errSelfTradeInvalid           = errors.New("self-trade policy is invalid")
	errSinceCursorConflict        = errors.New("since timestamp and cursor are mutually exclusive")
	errSymbolRequired             = errors.New("symbol is required")
	errSymbolLengthInvalid        = errors.New("symbol must contain between 3 and 32 characters")
	errTimestampInvalid           = errors.New("timestamp must not precede the Unix epoch")
	errTimeOrIDConflict           = errors.New("timestamp and transaction identifier are mutually exclusive")
	errTimeRangeInvalid           = errors.New("end time must not precede start time")
	errScheduledTimeConflict      = errors.New("absolute and relative scheduled times are mutually exclusive")
	errScheduledTimeInvalid       = errors.New("scheduled time is invalid")
	errTimeoutTooLarge            = errors.New("cancel-all timeout must be less than 86400 seconds")
	errTimeoutInvalid             = errors.New("cancel-all timeout must be a non-negative whole number of seconds")
	errTimeInForceInvalid         = errors.New("time-in-force is invalid")
	errToRequired                 = errors.New("destination account is required")
	errToWalletInvalid            = errors.New("destination wallet must be Futures Wallet")
	errTransactionIDRequired      = errors.New("transaction identifier is required")
	errTradeCountInvalid          = errors.New("trade count must be omitted or between 1 and 1000")
	errTradeIdentifierCount       = errors.New("trade query must contain between 1 and 20 identifiers")
	errTradeLimitInvalid          = errors.New("trade history limit must be between 1 and 100")
	errTradeTypeInvalid           = errors.New("trade history type is invalid")
	errTriggerInvalid             = errors.New("order trigger must be index or last")
	errTypeRequired               = errors.New("type is required")
	errUsernameRequired           = errors.New("username is required")
	errVolumeInvalid              = errors.New("volume must not be negative")
	errWithdrawalResultInvalid    = errors.New("withdrawal result contains no withdrawal fields")
	errNumericValueInvalid        = errors.New("numeric value must be finite")
)

// AssetClass identifies a Kraken asset or pair class.
type AssetClass string

// Kraken asset classes.
const (
	AssetClassCurrency        AssetClass = "currency"
	AssetClassForex           AssetClass = "forex"
	AssetClassEquity          AssetClass = "equity"
	AssetClassEquityPair      AssetClass = "equity_pair"
	AssetClassNFT             AssetClass = "nft"
	AssetClassDerivatives     AssetClass = "derivatives"
	AssetClassTokenizedAsset  AssetClass = "tokenized_asset"
	AssetClassFuturesContract AssetClass = "futures_contract"
	AssetClassSyntheticPair   AssetClass = "synthetic_pair"
	AssetClassExternalPair    AssetClass = "external_pair"
)

// AssetVersion selects the naming convention used in Kraken responses.
type AssetVersion uint8

// AssetVersionDisplay requests canonical display names.
const AssetVersionDisplay AssetVersion = 1

// AssetPairInfo selects the tradable-pair information returned by Kraken.
type AssetPairInfo string

// Kraken tradable-pair information filters.
const (
	AssetPairInfoAll      AssetPairInfo = "info"
	AssetPairInfoLeverage AssetPairInfo = "leverage"
	AssetPairInfoFees     AssetPairInfo = "fees"
	AssetPairInfoMargin   AssetPairInfo = "margin"
)

// ExecutionVenue identifies a Kraken execution venue.
type ExecutionVenue string

// Kraken execution venues.
const (
	ExecutionVenueInternational ExecutionVenue = "international"
	ExecutionVenueBitnomial     ExecutionVenue = "bitnomial_exchange"
)

// RebaseMultiplier controls how tokenized-asset values are displayed.
type RebaseMultiplier string

// Kraken rebase multiplier modes.
const (
	RebaseMultiplierRebased RebaseMultiplier = "rebased"
	RebaseMultiplierBase    RebaseMultiplier = "base"
)

// CloseTime selects which order timestamp Kraken searches.
type CloseTime string

// Kraken closed-order time filters.
const (
	CloseTimeOpen  CloseTime = "open"
	CloseTimeClose CloseTime = "close"
	CloseTimeBoth  CloseTime = "both"
)

// TradeHistoryType filters account trade history by position state.
type TradeHistoryType string

// Kraken trade-history filters.
const (
	TradeHistoryAll             TradeHistoryType = "all"
	TradeHistoryAnyPosition     TradeHistoryType = "any position"
	TradeHistoryClosedPosition  TradeHistoryType = "closed position"
	TradeHistoryClosingPosition TradeHistoryType = "closing position"
	TradeHistoryNoPosition      TradeHistoryType = "no position"
)

// LedgerType filters account ledger entries by activity.
type LedgerType string

// Kraken ledger activity filters.
const (
	LedgerTypeAll        LedgerType = "all"
	LedgerTypeTrade      LedgerType = "trade"
	LedgerTypeDeposit    LedgerType = "deposit"
	LedgerTypeWithdrawal LedgerType = "withdrawal"
	LedgerTypeTransfer   LedgerType = "transfer"
	LedgerTypeMargin     LedgerType = "margin"
	LedgerTypeAdjustment LedgerType = "adjustment"
	LedgerTypeRollover   LedgerType = "rollover"
	LedgerTypeCredit     LedgerType = "credit"
	LedgerTypeSettled    LedgerType = "settled"
	LedgerTypeStaking    LedgerType = "staking"
	LedgerTypeDividend   LedgerType = "dividend"
	LedgerTypeSale       LedgerType = "sale"
	LedgerTypeNFTRebate  LedgerType = "nft_rebate"
)

// PositionConsolidation selects how open positions are consolidated.
type PositionConsolidation string

// PositionConsolidationMarket consolidates positions by market.
const PositionConsolidationMarket PositionConsolidation = "market"

// ExportReportType identifies a Kraken export report.
type ExportReportType string

// Kraken export report types.
const (
	ExportReportTrades  ExportReportType = "trades"
	ExportReportLedgers ExportReportType = "ledgers"
)

// ExportFormat identifies a Kraken export file format.
type ExportFormat string

// Kraken export formats.
const (
	ExportFormatCSV ExportFormat = "CSV"
	ExportFormatTSV ExportFormat = "TSV"
)

// ExportRemovalType selects whether an export is cancelled or deleted.
type ExportRemovalType string

// Kraken export removal operations.
const (
	ExportRemovalCancel ExportRemovalType = "cancel"
	ExportRemovalDelete ExportRemovalType = "delete"
)

// ExportField identifies a field included in a Kraken export report.
type ExportField string

// Kraken export fields.
const (
	ExportFieldOrderTransactionID ExportField = "ordertxid"
	ExportFieldTime               ExportField = "time"
	ExportFieldOrderType          ExportField = "ordertype"
	ExportFieldPrice              ExportField = "price"
	ExportFieldCost               ExportField = "cost"
	ExportFieldFee                ExportField = "fee"
	ExportFieldVolume             ExportField = "vol"
	ExportFieldMargin             ExportField = "margin"
	ExportFieldMisc               ExportField = "misc"
	ExportFieldLedgers            ExportField = "ledgers"
	ExportFieldReferenceID        ExportField = "refid"
	ExportFieldType               ExportField = "type"
	ExportFieldSubtype            ExportField = "subtype"
	ExportFieldAssetClass         ExportField = "aclass"
	ExportFieldAsset              ExportField = "asset"
	ExportFieldAmount             ExportField = "amount"
	ExportFieldBalance            ExportField = "balance"
	ExportFieldWallet             ExportField = "wallet"
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

// EarnLockType identifies an Earn strategy lock model.
type EarnLockType string

// Kraken Earn lock types.
const (
	EarnLockTypeFlexible EarnLockType = "flex"
	EarnLockTypeBonded   EarnLockType = "bonded"
	EarnLockTypeTimed    EarnLockType = "timed"
	EarnLockTypeInstant  EarnLockType = "instant"
)

// Wallet identifies a Kraken wallet used in a wallet transfer.
type Wallet string

// Kraken wallet transfer endpoints.
const (
	WalletSpot    Wallet = "Spot Wallet"
	WalletFutures Wallet = "Futures Wallet"
)

// OrderPrice represents either an ordinary numeric price or a Kraken relative-price expression.
type OrderPrice struct {
	Value      float64
	Expression string
}

// TimeOrTransactionID represents an account-history boundary as either a timestamp or a Kraken transaction ID.
type TimeOrTransactionID struct {
	Time          time.Time
	TransactionID string
}

// GroupedOrderBookDepth identifies a supported grouped L2 order-book depth.
type GroupedOrderBookDepth uint64

// Kraken grouped L2 order-book depths.
const (
	GroupedOrderBookDepth10   GroupedOrderBookDepth = 10
	GroupedOrderBookDepth25   GroupedOrderBookDepth = 25
	GroupedOrderBookDepth100  GroupedOrderBookDepth = 100
	GroupedOrderBookDepth250  GroupedOrderBookDepth = 250
	GroupedOrderBookDepth1000 GroupedOrderBookDepth = 1000
)

// Level3OrderBookDepth identifies a supported authenticated L3 order-book depth.
type Level3OrderBookDepth uint64

// Kraken authenticated L3 order-book depths.
const (
	Level3OrderBookDepthFull Level3OrderBookDepth = 0
	Level3OrderBookDepth10   Level3OrderBookDepth = 10
	Level3OrderBookDepth25   Level3OrderBookDepth = 25
	Level3OrderBookDepth100  Level3OrderBookDepth = 100
	Level3OrderBookDepth250  Level3OrderBookDepth = 250
	Level3OrderBookDepth1000 Level3OrderBookDepth = 1000
)

// OrderBookGrouping identifies the number of ticks accumulated into a grouped price level.
type OrderBookGrouping uint64

// Kraken grouped-order-book tick groupings.
const (
	OrderBookGrouping1    OrderBookGrouping = 1
	OrderBookGrouping5    OrderBookGrouping = 5
	OrderBookGrouping10   OrderBookGrouping = 10
	OrderBookGrouping25   OrderBookGrouping = 25
	OrderBookGrouping50   OrderBookGrouping = 50
	OrderBookGrouping100  OrderBookGrouping = 100
	OrderBookGrouping250  OrderBookGrouping = 250
	OrderBookGrouping500  OrderBookGrouping = 500
	OrderBookGrouping1000 OrderBookGrouping = 1000
)

// SystemStatusResponse defines Kraken's current Spot system status.
type SystemStatusResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// GroupedOrderBookRequest defines request parameters for the grouped L2 order book.
type GroupedOrderBookRequest struct {
	Pair     currency.Pair
	Depth    GroupedOrderBookDepth
	Grouping OrderBookGrouping
}

// GroupedOrderBookResponse defines grouped L2 order book data.
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

// QueryLevel3OrderBookRequest defines request parameters for the authenticated L3 order book.
type QueryLevel3OrderBookRequest struct {
	Pair  currency.Pair
	Depth *Level3OrderBookDepth
}

// GetAssetsRequest defines current asset-info query parameters.
type GetAssetsRequest struct {
	Assets       currency.Currencies
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// GetAssetPairsRequest defines current tradable-pair query parameters.
type GetAssetPairsRequest struct {
	Pairs          currency.Pairs
	AssetClassBase AssetClass
	Info           AssetPairInfo
	CountryCode    string
	ExecutionVenue ExecutionVenue
	AssetVersion   AssetVersion
}

// GetTickerRequest defines current ticker query parameters.
type GetTickerRequest struct {
	Pairs        currency.Pairs
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// GetOHLCRequest defines current OHLC query parameters.
type GetOHLCRequest struct {
	Pair         currency.Pair
	Interval     time.Duration
	Since        time.Time
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// OHLCResponse defines candles grouped by pair and Kraken's pagination marker.
type OHLCResponse struct {
	Candles map[string][]OHLCEntry
	Last    types.Time
}

// UnmarshalJSON decodes pair-keyed OHLC arrays and the last marker.
func (r *OHLCResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	response := OHLCResponse{Candles: make(map[string][]OHLCEntry, len(raw))}
	for key, value := range raw {
		if key == "last" {
			if err := json.Unmarshal(value, &response.Last); err != nil {
				return err
			}
			continue
		}
		var candles []OHLCEntry
		if err := json.Unmarshal(value, &candles); err != nil {
			return err
		}
		response.Candles[key] = candles
	}
	*r = response
	return nil
}

// OHLCEntry defines one current OHLC candle.
type OHLCEntry struct {
	Time                       types.Time
	Open                       types.Number
	High                       types.Number
	Low                        types.Number
	Close                      types.Number
	VolumeWeightedAveragePrice types.Number
	Volume                     types.Number
	Count                      uint64
}

// UnmarshalJSON decodes Kraken's positional OHLC entry.
func (o *OHLCEntry) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 8 {
		return fmt.Errorf("expected 8 OHLC fields, got %d", len(fields))
	}
	decoded := OHLCEntry{}
	if err := json.Unmarshal(data, &[8]any{
		&decoded.Time,
		&decoded.Open,
		&decoded.High,
		&decoded.Low,
		&decoded.Close,
		&decoded.VolumeWeightedAveragePrice,
		&decoded.Volume,
		&decoded.Count,
	}); err != nil {
		return err
	}
	*o = decoded
	return nil
}

// GetDepthRequest defines current L2 order-book query parameters.
type GetDepthRequest struct {
	Pair         currency.Pair
	Count        uint64
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// OrderbookResponse defines pair-keyed current L2 order books.
type OrderbookResponse map[string]Orderbook

// Orderbook defines current L2 bids and asks.
type Orderbook struct {
	Bids []OrderbookEntry `json:"bids"`
	Asks []OrderbookEntry `json:"asks"`
}

// OrderbookEntry defines one positional L2 price level.
type OrderbookEntry struct {
	Price     types.Number
	Quantity  types.Number
	Timestamp types.Time
}

// UnmarshalJSON decodes Kraken's positional L2 price level.
func (o *OrderbookEntry) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 3 {
		return fmt.Errorf("expected 3 order book fields, got %d", len(fields))
	}
	decoded := OrderbookEntry{}
	if err := json.Unmarshal(data, &[3]any{&decoded.Price, &decoded.Quantity, &decoded.Timestamp}); err != nil {
		return err
	}
	*o = decoded
	return nil
}

// GetTradesRequest defines current recent-trades query parameters.
type GetTradesRequest struct {
	Pair         currency.Pair
	Since        time.Time
	Cursor       string
	Count        uint64
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// GetSpreadRequest defines current recent-spread query parameters.
type GetSpreadRequest struct {
	Pair         currency.Pair
	Since        time.Time
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// QueryLevel3OrderBookResponse defines L3 order book data.
type QueryLevel3OrderBookResponse struct {
	Pair string                 `json:"pair"`
	Bids []Level3OrderBookEntry `json:"bids"`
	Asks []Level3OrderBookEntry `json:"asks"`
}

// Level3OrderBookEntry defines a single L3 order.
type Level3OrderBookEntry struct {
	Price     types.Number `json:"price"`
	Quantity  types.Number `json:"qty"`
	OrderID   string       `json:"order_id"`
	Timestamp types.Time   `json:"timestamp"`
}

// GetPreTradeDataRequest defines request parameters for pre-trade transparency data.
type GetPreTradeDataRequest struct {
	Pair currency.Pair
}

// GetPreTradeDataResponse defines pre-trade transparency data.
type GetPreTradeDataResponse struct {
	Symbol            string              `json:"symbol"`
	Description       string              `json:"description"`
	BaseAsset         string              `json:"base_asset"`
	BaseDTICode       string              `json:"base_dti_code"`
	BaseDTIShortName  string              `json:"base_dti_short_name"`
	BaseNotation      string              `json:"base_notation"`
	QuoteAsset        string              `json:"quote_asset"`
	QuoteDTICode      string              `json:"quote_dti_code"`
	QuoteDTIShortName string              `json:"quote_dti_short_name"`
	QuoteNotation     string              `json:"quote_notation"`
	Venue             string              `json:"venue"`
	System            string              `json:"system"`
	Bids              []PreTradeBookLevel `json:"bids"`
	Asks              []PreTradeBookLevel `json:"asks"`
}

// PreTradeBookLevel defines an aggregated transparency order book level.
type PreTradeBookLevel struct {
	Side                 string       `json:"side"`
	Price                types.Number `json:"price"`
	Quantity             types.Number `json:"qty"`
	Count                uint64       `json:"count"`
	SubmissionTimestamp  time.Time    `json:"submission_ts"`
	PublicationTimestamp time.Time    `json:"publication_ts"`
}

// GetPostTradeDataRequest defines filters for post-trade transparency data.
type GetPostTradeDataRequest struct {
	Pair          currency.Pair
	FromTimestamp time.Time
	ToTimestamp   time.Time
	Count         uint64
}

// GetPostTradeDataResponse defines post-trade transparency data.
type GetPostTradeDataResponse struct {
	LastTimestamp time.Time       `json:"last_ts"`
	Count         uint64          `json:"count"`
	Trades        []PostTradeData `json:"trades"`
}

// PostTradeData defines a published Spot trade.
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

// GetAccountBalanceRequest defines optional account balance parameters.
type GetAccountBalanceRequest struct {
	RebaseMultiplier RebaseMultiplier
}

// GetTradeBalanceRequest defines current trade-balance parameters.
type GetTradeBalanceRequest struct {
	Asset            string
	RebaseMultiplier RebaseMultiplier
}

// GetOpenOrdersRequest defines current open-order filters.
type GetOpenOrdersRequest struct {
	Trades           bool
	UserReference    *int32
	ClientOrderID    string
	RebaseMultiplier RebaseMultiplier
}

// GetClosedOrdersRequest defines current closed-order filters.
type GetClosedOrdersRequest struct {
	Trades           bool
	UserReference    *int32
	ClientOrderID    string
	Start            TimeOrTransactionID
	End              TimeOrTransactionID
	Offset           uint64
	CloseTime        CloseTime
	ConsolidateTaker *bool
	WithoutCount     bool
	RebaseMultiplier RebaseMultiplier
}

// QueryOrdersInfoRequest defines current order-query parameters.
type QueryOrdersInfoRequest struct {
	TransactionIDs   []string
	Trades           bool
	UserReference    *int32
	ConsolidateTaker *bool
	RebaseMultiplier RebaseMultiplier
}

// GetTradesHistoryRequest defines current trade-history filters.
type GetTradesHistoryRequest struct {
	Type             TradeHistoryType
	Trades           bool
	Start            TimeOrTransactionID
	End              TimeOrTransactionID
	Offset           uint64
	WithoutCount     bool
	ConsolidateTaker *bool
	Ledgers          bool
	RebaseMultiplier RebaseMultiplier
	AssetClass       AssetClass
	Pair             currency.Pair
	Limit            *uint64
}

// QueryTradesRequest defines current trade-query parameters.
type QueryTradesRequest struct {
	TransactionIDs   []string
	Trades           bool
	RebaseMultiplier RebaseMultiplier
}

// OpenPositionsRequest defines current open-position parameters.
type OpenPositionsRequest struct {
	TransactionIDs   []string
	DoCalculations   bool
	Consolidation    PositionConsolidation
	RebaseMultiplier RebaseMultiplier
}

// GetLedgersRequest defines current ledger filters.
type GetLedgersRequest struct {
	AssetClass       AssetClass
	Assets           []string
	Type             LedgerType
	Start            TimeOrTransactionID
	End              TimeOrTransactionID
	Offset           uint64
	WithoutCount     bool
	RebaseMultiplier RebaseMultiplier
}

// QueryLedgersRequest defines current ledger-query parameters.
type QueryLedgersRequest struct {
	IDs              []string
	Trades           bool
	RebaseMultiplier RebaseMultiplier
}

// GetTradeVolumeRequest defines current trade-volume parameters.
type GetTradeVolumeRequest struct {
	Pairs            []TradeVolumePairRequest
	FeeInfo          *bool
	FeeSchedule      *bool
	RebaseMultiplier RebaseMultiplier
}

// TradeVolumePairRequest defines an exchange-native asset identifier and its asset class.
type TradeVolumePairRequest struct {
	Asset      string
	AssetClass AssetClass
}

// GetExtendedBalanceRequest defines optional extended balance parameters.
type GetExtendedBalanceRequest struct {
	RebaseMultiplier RebaseMultiplier
}

// ExtendedBalanceResponse defines total, held, and credit amounts for one asset.
type ExtendedBalanceResponse struct {
	Balance    types.Number `json:"balance"`
	Credit     types.Number `json:"credit"`
	CreditUsed types.Number `json:"credit_used"`
	HoldTrade  types.Number `json:"hold_trade"`
}

// GetCreditLinesRequest defines optional credit line parameters.
type GetCreditLinesRequest struct {
	RebaseMultiplier RebaseMultiplier
}

// GetCreditLinesResponse defines asset credit lines and account-wide monitoring values.
type GetCreditLinesResponse struct {
	AssetDetails  map[string]CreditLineAssetDetails `json:"asset_details"`
	LimitsMonitor CreditLineLimitsMonitor           `json:"limits_monitor"`
}

// CreditLineAssetDetails defines balance and credit details for an asset.
type CreditLineAssetDetails struct {
	Balance         types.Number `json:"balance"`
	HoldTrade       types.Number `json:"hold_trade"`
	CollateralValue types.Number `json:"collateral_value"`
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

// GetOrderAmendsRequest defines order amendment history parameters.
type GetOrderAmendsRequest struct {
	OrderID          string
	RebaseMultiplier RebaseMultiplier
}

// GetOrderAmendsResponse defines order amendment history.
type GetOrderAmendsResponse struct {
	Count  uint64       `json:"count"`
	Amends []OrderAmend `json:"amends"`
}

// OrderAmend defines one order amendment event.
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
	Timestamp     types.Time   `json:"timestamp"`
}

// RequestExportReportRequest defines parameters for creating an export report.
type RequestExportReportRequest struct {
	Report      ExportReportType
	Format      ExportFormat
	Description string
	Fields      []ExportField
	StartTime   time.Time
	EndTime     time.Time
}

// RequestExportReportResponse defines an export report identifier.
type RequestExportReportResponse struct {
	ID string `json:"id"`
}

// GetExportReportStatusRequest defines export status parameters.
type GetExportReportStatusRequest struct {
	Report ExportReportType
}

// ExportReportStatusResponse defines export report status details.
type ExportReportStatusResponse struct {
	ID            string     `json:"id"`
	Description   string     `json:"descr"`
	Format        string     `json:"format"`
	Report        string     `json:"report"`
	Subtype       string     `json:"subtype"`
	Status        string     `json:"status"`
	Error         string     `json:"error"`
	Flags         string     `json:"flags"`
	Fields        string     `json:"fields"`
	CreatedTime   types.Time `json:"createdtm"`
	ExpiryTime    types.Time `json:"expiretm"`
	StartTime     types.Time `json:"starttm"`
	CompletedTime types.Time `json:"completedtm"`
	DataStartTime types.Time `json:"datastarttm"`
	DataEndTime   types.Time `json:"dataendtm"`
	AssetClass    string     `json:"aclass"`
	Asset         string     `json:"asset"`
	AssetClasses  []string   `json:"asset_classes"`
	EndTime       types.Time `json:"endtm"`
	Delete        bool       `json:"delete"`
}

// RetrieveDataExportRequest defines export retrieval parameters.
type RetrieveDataExportRequest struct {
	ID string
}

// DeleteExportReportRequest defines export removal parameters.
type DeleteExportReportRequest struct {
	ID   string
	Type ExportRemovalType
}

// DeleteExportReportResponse defines export removal results.
type DeleteExportReportResponse struct {
	Delete bool `json:"delete"`
	Cancel bool `json:"cancel"`
}

// GetAPIKeyInfoRequest defines optional API key information parameters.
type GetAPIKeyInfoRequest struct {
	OTP string
}

// GetAPIKeyInfoResponse defines metadata for the authenticated API key.
type GetAPIKeyInfoResponse struct {
	APIKeyName   string       `json:"apiKeyName"`
	APIKey       string       `json:"apiKey"`
	Nonce        types.Number `json:"nonce"`
	NonceWindow  uint64       `json:"nonceWindow"`
	Permissions  []string     `json:"permissions"`
	IBAN         string       `json:"iban"`
	ValidUntil   types.Time   `json:"validUntil"`
	QueryFrom    types.Time   `json:"queryFrom"`
	QueryTo      types.Time   `json:"queryTo"`
	CreatedTime  types.Time   `json:"createdTime"`
	ModifiedTime types.Time   `json:"modifiedTime"`
	IPAllowlist  []string     `json:"ipAllowlist"`
	LastUsed     *types.Time  `json:"lastUsed"`
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

// GetRecentDepositsStatusRequest defines filters for recent deposit status.
type GetRecentDepositsStatusRequest struct {
	Asset            string
	AssetClass       AssetClass
	Method           string
	Start            time.Time
	End              time.Time
	Cursor           string
	Paginate         *bool
	Limit            *uint64
	RebaseMultiplier RebaseMultiplier
}

// GetDepositMethodsRequest defines current deposit-method parameters.
type GetDepositMethodsRequest struct {
	Asset            string
	AssetClass       AssetClass
	RebaseMultiplier RebaseMultiplier
}

// DepositMethodResponse defines an available deposit method.
type DepositMethodResponse struct {
	Method           string       `json:"method"`
	Limit            any          `json:"limit"`
	Fee              types.Number `json:"fee"`
	FeePercent       types.Number `json:"fee-percentage"`
	AddressSetupFee  types.Number `json:"address-setup-fee"`
	GeneratesAddress bool         `json:"gen-address"`
	Minimum          types.Number `json:"minimum"`
}

// GetDepositAddressesRequest defines current deposit-address parameters.
type GetDepositAddressesRequest struct {
	Asset      string
	AssetClass AssetClass
	Method     string
	New        bool
	Amount     *float64
}

// DepositAddressResponse defines a current deposit address.
type DepositAddressResponse struct {
	Address    string `json:"address"`
	ExpireTime string `json:"expiretm"`
	New        bool   `json:"new"`
	Tag        string `json:"tag"`
	Memo       string `json:"memo"`
}

// GetWithdrawalInformationRequest defines withdrawal information parameters.
type GetWithdrawalInformationRequest struct {
	Asset  string
	Key    string
	Amount float64
}

// WithdrawalInformationResponse defines the calculated withdrawal amount and fees.
type WithdrawalInformationResponse struct {
	Method string       `json:"method"`
	Limit  types.Number `json:"limit"`
	Amount types.Number `json:"amount"`
	Fee    types.Number `json:"fee"`
}

// WithdrawFundsRequest defines current withdrawal parameters.
type WithdrawFundsRequest struct {
	Asset            string
	AssetClass       AssetClass
	Key              string
	Address          string
	Amount           float64
	MaximumFee       *float64
	RebaseMultiplier RebaseMultiplier
}

// WithdrawFundsResponse defines a withdrawal reference.
type WithdrawFundsResponse struct {
	ReferenceID string `json:"refid"`
}

// GetRecentWithdrawalsStatusRequest defines filters for recent withdrawal status.
type GetRecentWithdrawalsStatusRequest struct {
	Asset            string
	AssetClass       AssetClass
	Method           string
	Start            time.Time
	End              time.Time
	Cursor           string
	Paginate         *bool
	Limit            *uint64
	RebaseMultiplier RebaseMultiplier
}

// RecentWithdrawalsStatusResponse normalises Kraken's paginated and non-paginated result shapes.
type RecentWithdrawalsStatusResponse struct {
	Withdrawals []RecentWithdrawalStatus
	NextCursor  string
}

// UnmarshalJSON normalises Kraken's array, single-withdrawal, and paginated withdrawal results.
func (r *RecentWithdrawalsStatusResponse) UnmarshalJSON(data []byte) error {
	var withdrawals []RecentWithdrawalStatus
	if !bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		if err := json.Unmarshal(data, &withdrawals); err == nil {
			*r = RecentWithdrawalsStatusResponse{Withdrawals: withdrawals}
			return nil
		}
	}

	var paginated struct {
		Withdrawal json.RawMessage `json:"withdrawal"`
		NextCursor string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(data, &paginated); err == nil && len(paginated.Withdrawal) > 0 {
		response := RecentWithdrawalsStatusResponse{NextCursor: paginated.NextCursor}
		if !bytes.Equal(bytes.TrimSpace(paginated.Withdrawal), []byte("null")) {
			if err := json.Unmarshal(paginated.Withdrawal, &withdrawals); err == nil {
				response.Withdrawals = withdrawals
				*r = response
				return nil
			}
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(paginated.Withdrawal, &fields); err != nil {
			return err
		}
		if !containsWithdrawalField(fields) {
			return errPaginatedWithdrawalInvalid
		}
		var withdrawal RecentWithdrawalStatus
		if err := json.Unmarshal(paginated.Withdrawal, &withdrawal); err != nil {
			return err
		}
		response.Withdrawals = []RecentWithdrawalStatus{withdrawal}
		*r = response
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if !containsWithdrawalField(fields) {
		return errWithdrawalResultInvalid
	}
	var withdrawal RecentWithdrawalStatus
	if err := json.Unmarshal(data, &withdrawal); err != nil {
		return err
	}
	*r = RecentWithdrawalsStatusResponse{Withdrawals: []RecentWithdrawalStatus{withdrawal}}
	return nil
}

func containsWithdrawalField(fields map[string]json.RawMessage) bool {
	for key := range fields {
		switch key {
		case "method", "network", "aclass", "asset", "refid", "txid", "info", "amount", "fee", "time", "status", "status-prop", "key":
			return true
		}
	}
	return false
}

// RecentWithdrawalStatus defines one withdrawal status entry.
type RecentWithdrawalStatus struct {
	Method           string       `json:"method"`
	Network          string       `json:"network"`
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
	Key              string       `json:"key"`
}

// RecentDepositsStatusResponse normalises Kraken's paginated and non-paginated result shapes.
type RecentDepositsStatusResponse struct {
	Deposits   []RecentDepositStatus
	NextCursor string
}

// UnmarshalJSON normalises Kraken's array, single-deposit, and paginated deposit results.
func (r *RecentDepositsStatusResponse) UnmarshalJSON(data []byte) error {
	var deposits []RecentDepositStatus
	if !bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		if err := json.Unmarshal(data, &deposits); err == nil {
			*r = RecentDepositsStatusResponse{Deposits: deposits}
			return nil
		}
	}

	var paginated struct {
		Deposit    json.RawMessage `json:"deposit"`
		NextCursor string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(data, &paginated); err == nil && len(paginated.Deposit) > 0 {
		response := RecentDepositsStatusResponse{NextCursor: paginated.NextCursor}
		if !bytes.Equal(bytes.TrimSpace(paginated.Deposit), []byte("null")) {
			if err := json.Unmarshal(paginated.Deposit, &deposits); err == nil {
				response.Deposits = deposits
				*r = response
				return nil
			}
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(paginated.Deposit, &fields); err != nil {
			return err
		}
		if !containsDepositField(fields) {
			return errPaginatedDepositInvalid
		}
		var deposit RecentDepositStatus
		if err := json.Unmarshal(paginated.Deposit, &deposit); err != nil {
			return err
		}
		response.Deposits = []RecentDepositStatus{deposit}
		*r = response
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if !containsDepositField(fields) {
		return errDepositResultInvalid
	}
	var deposit RecentDepositStatus
	if err := json.Unmarshal(data, &deposit); err != nil {
		return err
	}
	*r = RecentDepositsStatusResponse{Deposits: []RecentDepositStatus{deposit}}
	return nil
}

func isValidSpotEnum[T ~string](value T, allowed ...string) bool {
	return value == "" || slices.Contains(allowed, string(value))
}

func formatSpotFloat(value float64) (string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", errNumericValueInvalid
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
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

func formatTimeOrTransactionID(value TimeOrTransactionID) (string, error) {
	if !value.Time.IsZero() && value.TransactionID != "" {
		return "", errTimeOrIDConflict
	}
	if value.TransactionID != "" {
		return value.TransactionID, nil
	}
	if value.Time.IsZero() {
		return "", nil
	}
	if value.Time.Unix() < 0 {
		return "", errTimestampInvalid
	}
	return strconv.FormatInt(value.Time.Unix(), 10), nil
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

func containsDepositField(fields map[string]json.RawMessage) bool {
	for key := range fields {
		switch key {
		case "method", "aclass", "asset", "refid", "txid", "info", "amount", "fee", "time", "status", "status-prop", "originators":
			return true
		}
	}
	return false
}

// RecentDepositStatus defines one deposit status entry.
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

// GetWithdrawalMethodsRequest defines withdrawal method filters.
type GetWithdrawalMethodsRequest struct {
	Asset            string
	AssetClass       AssetClass
	Network          string
	RebaseMultiplier RebaseMultiplier
}

// WithdrawalMethodResponse defines an available withdrawal method.
type WithdrawalMethodResponse struct {
	Asset     string                  `json:"asset"`
	Method    string                  `json:"method"`
	MethodID  string                  `json:"method_id"`
	Network   string                  `json:"network"`
	NetworkID string                  `json:"network_id"`
	Minimum   types.Number            `json:"minimum"`
	Fee       WithdrawalMethodFee     `json:"fee"`
	Limits    []WithdrawalMethodLimit `json:"limits"`
}

// WithdrawalMethodFee defines flat or percentage withdrawal fees.
type WithdrawalMethodFee struct {
	AssetClass string       `json:"aclass"`
	Asset      string       `json:"asset"`
	Fee        types.Number `json:"fee"`
	FeePercent types.Number `json:"fee_percentage"`
}

// WithdrawalMethodLimit defines rate limits for a withdrawal method.
type WithdrawalMethodLimit struct {
	Description string                          `json:"description"`
	LimitType   string                          `json:"limit_type"`
	Limits      map[string]WithdrawalLimitUsage `json:"limits"`
}

// WithdrawalLimitUsage defines usage within one withdrawal rate-limit window.
type WithdrawalLimitUsage struct {
	Maximum   types.Number `json:"maximum"`
	Remaining types.Number `json:"remaining"`
	Used      types.Number `json:"used"`
}

// GetWithdrawalAddressesRequest defines withdrawal address filters.
type GetWithdrawalAddressesRequest struct {
	Asset      string
	AssetClass AssetClass
	Method     string
	Key        string
	Verified   *bool
}

// WithdrawalAddressResponse defines a configured withdrawal address.
type WithdrawalAddressResponse struct {
	Address  string `json:"address"`
	Asset    string `json:"asset"`
	Method   string `json:"method"`
	Key      string `json:"key"`
	Tag      string `json:"tag"`
	Verified bool   `json:"verified"`
}

// CancelWithdrawalRequest defines a withdrawal cancellation.
type CancelWithdrawalRequest struct {
	Asset       string
	ReferenceID string
}

// WalletTransferRequest defines a transfer between Spot and Futures wallets.
type WalletTransferRequest struct {
	Asset  string
	From   Wallet
	To     Wallet
	Amount float64
}

// WalletTransferResponse defines a wallet transfer reference.
type WalletTransferResponse struct {
	ReferenceID string `json:"refid"`
}

// CreateSubaccountRequest defines subaccount creation parameters.
type CreateSubaccountRequest struct {
	Username string
	Email    string
}

// AccountTransferRequest defines a transfer between a primary account and subaccount.
type AccountTransferRequest struct {
	Asset      string
	AssetClass AssetClass
	Amount     float64
	From       string
	To         string
}

// AccountTransferResponse defines an account transfer result.
type AccountTransferResponse struct {
	TransferID string `json:"transfer_id"`
	Status     string `json:"status"`
}

// AllocateEarnFundsRequest defines Earn allocation parameters.
type AllocateEarnFundsRequest struct {
	Amount     float64
	StrategyID string
}

// DeallocateEarnFundsRequest defines Earn deallocation parameters.
type DeallocateEarnFundsRequest struct {
	Amount     float64
	StrategyID string
}

// EarnOperationStatusRequest defines Earn operation status parameters.
type EarnOperationStatusRequest struct {
	StrategyID string
}

// EarnOperationStatusResponse defines asynchronous Earn operation status.
type EarnOperationStatusResponse struct {
	Pending bool `json:"pending"`
}

// ListEarnStrategiesRequest defines Earn strategy filters.
type ListEarnStrategiesRequest struct {
	Ascending *bool
	Asset     string
	Cursor    string
	Limit     *uint16
	LockType  []EarnLockType
}

// ListEarnStrategiesResponse defines paginated Earn strategies.
type ListEarnStrategiesResponse struct {
	Items      []EarnStrategy `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

// EarnStrategy defines one current Kraken Earn strategy.
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

// EarnStrategyAPR defines an estimated annual percentage-rate range.
type EarnStrategyAPR struct {
	Low  types.Number `json:"low"`
	High types.Number `json:"high"`
}

// EarnStrategyLockType defines strategy locking details.
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

// EarnStrategyAutoCompound defines strategy auto-compounding behaviour.
type EarnStrategyAutoCompound struct {
	Type    string `json:"type"`
	Default bool   `json:"default"`
}

// EarnStrategyYieldSource defines the strategy yield mechanism.
type EarnStrategyYieldSource struct {
	Type string `json:"type"`
}

// ListEarnAllocationsRequest defines Earn allocation filters.
type ListEarnAllocationsRequest struct {
	Ascending           *bool
	ConvertedAsset      string
	HideZeroAllocations *bool
}

// ListEarnAllocationsResponse defines current Earn allocations.
type ListEarnAllocationsResponse struct {
	ConvertedAsset string           `json:"converted_asset"`
	TotalAllocated types.Number     `json:"total_allocated"`
	TotalRewarded  types.Number     `json:"total_rewarded"`
	NextCursor     string           `json:"next_cursor"`
	Items          []EarnAllocation `json:"items"`
}

// EarnAllocation defines allocation data for one strategy.
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

// EarnAllocationAmountState defines native and converted allocation amounts.
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

// EarnAllocationPayout defines current payout-period rewards.
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
