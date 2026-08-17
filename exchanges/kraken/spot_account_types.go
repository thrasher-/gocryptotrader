package kraken

import (
	"errors"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var (
	errCloseTimeInvalid      = errors.New("close time must be open, close, or both")
	errConsolidationInvalid  = errors.New("position consolidation must be market")
	errDescriptionRequired   = errors.New("description is required")
	errExportFieldInvalid    = errors.New("export field is invalid for the selected report")
	errExportFormatInvalid   = errors.New("export format must be CSV or TSV")
	errExportRemovalInvalid  = errors.New("export removal type must be cancel or delete")
	errExportReportInvalid   = errors.New("export report type must be trades or ledgers")
	errIDRequired            = errors.New("identifier is required")
	errLedgerIdentifierCount = errors.New("ledger query must contain between 1 and 20 identifiers")
	errLedgerTypeInvalid     = errors.New("ledger type is invalid")
	errOrderIdentifierCount  = errors.New("order query must contain between 1 and 50 identifiers")
	errReportRequired        = errors.New("report type is required")
	errTimeOrIDConflict      = errors.New("timestamp and transaction identifier are mutually exclusive")
	errTradeIdentifierCount  = errors.New("trade query must contain between 1 and 20 identifiers")
	errTradeLimitInvalid     = errors.New("trade history limit must be between 1 and 100")
	errTradeTypeInvalid      = errors.New("trade history type is invalid")
	errTransactionIDRequired = errors.New("transaction identifier is required")
	errTypeRequired          = errors.New("type is required")
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

// TimeOrTransactionID represents an account-history boundary as either a timestamp or a Kraken transaction ID.
type TimeOrTransactionID struct {
	Time          time.Time
	TransactionID string
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
