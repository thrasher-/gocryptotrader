package binance

import (
	"errors"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var (
	validFuturesIntervals = []string{
		"1m", "3m", "5m", "15m", "30m",
		"1h", "2h", "4h", "6h", "8h",
		"12h", "1d", "3d", "1w", "1M",
	}

	validContractType = []string{
		"ALL", "CURRENT_QUARTER", "NEXT_QUARTER",
	}

	validNewOrderRespType = []string{"ACK", "RESULT"}

	validWorkingType = []string{"MARK_PRICE", "CONTRACT_TYPE"}

	validPositionSide = []string{"BOTH", "LONG", "SHORT"}

	validMarginType = []string{"ISOLATED", "CROSSED"}

	validIncomeType = []string{"TRANSFER", "WELCOME_BONUS", "REALIZED_PNL", "FUNDING_FEE", "COMMISSION", "INSURANCE_CLEAR"}

	validAutoCloseTypes = []string{"LIQUIDATION", "ADL"}

	validMarginChange = map[string]int64{
		"add":    1,
		"reduce": 2,
	}

	uValidOBLimits = []string{"5", "10", "20", "50", "100", "500", "1000"}

	uValidPeriods = []string{"5m", "15m", "30m", "1h", "2h", "4h", "6h", "12h", "1d"}

	errContractTypeIsRequired  = errors.New("contract type is required")
	errInvalidPeriodOrInterval = errors.New("invalid period")
)

// UPublicTradesData stores trade data. It serves both usdt margined futures and the coin margined
// historical trades endpoint, which sends BaseQuantity in place of QuoteQuantity and no IsRPITrade
type UPublicTradesData struct {
	ID            int64        `json:"id"`
	Price         types.Number `json:"price"`
	Quantity      types.Number `json:"qty"`
	BaseQuantity  types.Number `json:"baseQty"`
	QuoteQuantity types.Number `json:"quoteQty"`
	Time          types.Time   `json:"time"`
	IsBuyerMaker  bool         `json:"isBuyerMaker"`
	IsRPITrade    bool         `json:"isRPITrade"`
}

// UCompressedTradeData stores compressed trade data
type UCompressedTradeData struct {
	AggregateTradeID uint64       `json:"a"`
	Price            types.Number `json:"p"`
	Quantity         types.Number `json:"q"`
	// NormalQuantity is the aggregate quantity excluding trades that involved retail price
	// improvement orders, so it is a quantity rather than a notional
	NormalQuantity types.Number `json:"nq"`
	FirstTradeID   uint64       `json:"f"`
	LastTradeID    uint64       `json:"l"`
	Timestamp      types.Time   `json:"T"`
	IsBuyerMaker   bool         `json:"m"`
}

// UMarkPrice stores mark price data
type UMarkPrice struct {
	Symbol               string       `json:"symbol"`
	MarkPrice            types.Number `json:"markPrice"`
	IndexPrice           types.Number `json:"indexPrice"`
	LastFundingRate      types.Number `json:"lastFundingRate"`
	EstimatedSettlePrice types.Number `json:"estimatedSettlePrice"`
	InterestRate         types.Number `json:"interestRate"`
	NextFundingTime      types.Time   `json:"nextFundingTime"`
	Time                 types.Time   `json:"time"`
}

// FundingRateInfoResponse stores funding rate info
type FundingRateInfoResponse struct {
	Symbol                   string       `json:"symbol"`
	AdjustedFundingRateCap   types.Number `json:"adjustedFundingRateCap"`
	AdjustedFundingRateFloor types.Number `json:"adjustedFundingRateFloor"`
	FundingIntervalHours     int64        `json:"fundingIntervalHours"`
	Disclaimer               bool         `json:"disclaimer"`
	UpdateTime               types.Time   `json:"updateTime"`
}

// FundingRateHistory stores funding rate history
type FundingRateHistory struct {
	Symbol      string       `json:"symbol"`
	FundingRate types.Number `json:"fundingRate"`
	FundingTime types.Time   `json:"fundingTime"`
	MarkPrice   types.Number `json:"markPrice"`
	RateType    string       `json:"rateType"`
}

// U24HrPriceChangeStats stores price change stats data
type U24HrPriceChangeStats struct {
	Symbol               string       `json:"symbol"`
	PriceChange          types.Number `json:"priceChange"`
	PriceChangePercent   types.Number `json:"priceChangePercent"`
	WeightedAveragePrice types.Number `json:"weightedAvgPrice"`
	PrevClosePrice       types.Number `json:"prevClosePrice"`
	LastPrice            types.Number `json:"lastPrice"`
	LastQuantity         types.Number `json:"lastQty"`
	OpenPrice            types.Number `json:"openPrice"`
	HighPrice            types.Number `json:"highPrice"`
	LowPrice             types.Number `json:"lowPrice"`
	Volume               types.Number `json:"volume"`
	QuoteVolume          types.Number `json:"quoteVolume"`
	OpenTime             types.Time   `json:"openTime"`
	CloseTime            types.Time   `json:"closeTime"`
	FirstID              int64        `json:"firstId"`
	LastID               int64        `json:"lastId"`
	Count                int64        `json:"count"`
}

// USymbolPriceTicker stores symbol price ticker data
type USymbolPriceTicker struct {
	Symbol string       `json:"symbol"`
	Price  types.Number `json:"price"`
	Time   types.Time   `json:"time"`
}

// USymbolOrderbookTicker stores symbol orderbook ticker data
type USymbolOrderbookTicker struct {
	Symbol       string       `json:"symbol"`
	BidPrice     types.Number `json:"bidPrice"`
	BidQuantity  types.Number `json:"bidQty"`
	AskPrice     types.Number `json:"askPrice"`
	AskQuantity  types.Number `json:"askQty"`
	Time         types.Time   `json:"time"`
	LastUpdateID int64        `json:"lastUpdateId"`
}

// ULiquidationOrdersData stores liquidation orders data
type ULiquidationOrdersData struct {
	Symbol           string            `json:"symbol"`
	Price            types.Number      `json:"price"`
	OriginalQuantity types.Number      `json:"origQty"`
	ExecutedQuantity types.Number      `json:"executedQty"`
	AveragePrice     types.Number      `json:"averagePrice"`
	Status           string            `json:"status"`
	TimeInForce      order.TimeInForce `json:"timeInForce"`
	OrderType        string            `json:"type"`
	Side             string            `json:"side"`
	Time             types.Time        `json:"time"`
}

// UOpenInterestData stores open interest data
type UOpenInterestData struct {
	OpenInterest types.Number `json:"openInterest"`
	Symbol       string       `json:"symbol"`
	Time         types.Time   `json:"time"`
}

// UOpenInterestStats stores open interest stats data
type UOpenInterestStats struct {
	Symbol               string       `json:"symbol"`
	SumOpenInterest      types.Number `json:"sumOpenInterest"`
	SumOpenInterestValue types.Number `json:"sumOpenInterestValue"`
	CMCCirculatingSupply types.Number `json:"CMCCirculatingSupply"`
	Timestamp            types.Time   `json:"timestamp"`
}

// ULongShortRatio stores top trader accounts' or positions' or global long/short ratio data
type ULongShortRatio struct {
	Symbol         string       `json:"symbol"`
	LongShortRatio types.Number `json:"longShortRatio"`
	LongAccount    types.Number `json:"longAccount"`
	ShortAccount   types.Number `json:"shortAccount"`
	Timestamp      types.Time   `json:"timestamp"`
}

// UTakerVolumeData stores volume data on buy/sell side from takers
type UTakerVolumeData struct {
	BuySellRatio types.Number `json:"buySellRatio"`
	BuyVol       types.Number `json:"buyVol"`
	SellVol      types.Number `json:"sellVol"`
	Timestamp    types.Time   `json:"timestamp"`
}

// UCompositeIndexInfoData stores composite index data for usdt margined futures
type UCompositeIndexInfoData struct {
	Symbol        string     `json:"symbol"`
	Time          types.Time `json:"time"`
	BaseAssetList []struct {
		BaseAsset          currency.Code `json:"baseAsset"`
		QuoteAsset         currency.Code `json:"quoteAsset"`
		WeightInQuantity   types.Number  `json:"weightInQuantity"`
		WeightInPercentage types.Number  `json:"weightInPercentage"`
	} `json:"baseAssetList"`
}

// UOrderData stores order data
type UOrderData struct {
	ClientOrderID           string            `json:"clientOrderId"`
	Time                    types.Time        `json:"time"`
	CumulativeQuantity      types.Number      `json:"cumQty"`
	CumulativeQuote         types.Number      `json:"cumQuote"`
	ExecutedQuantity        types.Number      `json:"executedQty"`
	OrderID                 uint64            `json:"orderId"`
	AveragePrice            types.Number      `json:"avgPrice"`
	OriginalQuantity        types.Number      `json:"origQty"`
	Price                   types.Number      `json:"price"`
	ReduceOnly              bool              `json:"reduceOnly"`
	Side                    string            `json:"side"`
	PositionSide            string            `json:"positionSide"`
	Status                  string            `json:"status"`
	StopPrice               types.Number      `json:"stopPrice"`
	ClosePosition           bool              `json:"closePosition"`
	Symbol                  string            `json:"symbol"`
	TimeInForce             order.TimeInForce `json:"timeInForce"`
	OrderType               string            `json:"type"`
	OriginalType            string            `json:"origType"`
	ActivatePrice           types.Number      `json:"activatePrice"`
	PriceRate               types.Number      `json:"priceRate"`
	UpdateTime              types.Time        `json:"updateTime"`
	WorkingType             string            `json:"workingType"`
	PriceProtect            bool              `json:"priceProtect"`
	PriceMatch              string            `json:"priceMatch"`
	SelfTradePreventionMode string            `json:"selfTradePreventionMode"`
	GoodTillDate            types.Time        `json:"goodTillDate"`
	Code                    int64             `json:"code"`
	Message                 string            `json:"msg"`
	Pair                    string            `json:"pair"`
	CumBase                 string            `json:"cumBase"`
}

// UFuturesOrderData stores order data for ufutures
type UFuturesOrderData struct {
	AveragePrice            types.Number      `json:"avgPrice"`
	ClientOrderID           string            `json:"clientOrderId"`
	CumulativeQuote         types.Number      `json:"cumQuote"`
	CumulativeBase          types.Number      `json:"cumBase"`
	ExecutedQuantity        types.Number      `json:"executedQty"`
	OrderID                 uint64            `json:"orderId"`
	OriginalQuantity        types.Number      `json:"origQty"`
	OriginalType            string            `json:"origType"`
	Price                   types.Number      `json:"price"`
	ReduceOnly              bool              `json:"reduceOnly"`
	Side                    string            `json:"side"`
	PositionSide            string            `json:"positionSide"`
	Status                  string            `json:"status"`
	StopPrice               types.Number      `json:"stopPrice"`
	ClosePosition           bool              `json:"closePosition"`
	Symbol                  string            `json:"symbol"`
	Pair                    string            `json:"pair"`
	Time                    types.Time        `json:"time"`
	TimeInForce             order.TimeInForce `json:"timeInForce"`
	OrderType               string            `json:"type"`
	ActivatePrice           types.Number      `json:"activatePrice"`
	PriceRate               types.Number      `json:"priceRate"`
	UpdateTime              types.Time        `json:"updateTime"`
	WorkingType             string            `json:"workingType"`
	PriceProtect            bool              `json:"priceProtect"`
	PriceMatch              string            `json:"priceMatch"`
	SelfTradePreventionMode string            `json:"selfTradePreventionMode"`
	GoodTillDate            types.Time        `json:"goodTillDate"`
}

// UAccountBalanceV2Data stores account balance data for ufutures
type UAccountBalanceV2Data struct {
	AccountAlias       string        `json:"accountAlias"`
	Asset              currency.Code `json:"asset"`
	Balance            types.Number  `json:"balance"`
	CrossWalletBalance types.Number  `json:"crossWalletBalance"`
	CrossUnrealizedPNL types.Number  `json:"crossUnPnl"`
	AvailableBalance   types.Number  `json:"availableBalance"`
	MaxWithdrawAmount  types.Number  `json:"maxWithdrawAmount"`
}

// UAccountInformationV2Data stores account info for ufutures
type UAccountInformationV2Data struct {
	FeeTier                     int64        `json:"feeTier"`
	CanTrade                    bool         `json:"canTrade"`
	CanDeposit                  bool         `json:"canDeposit"`
	CanWithdraw                 bool         `json:"canWithdraw"`
	UpdateTime                  types.Time   `json:"updateTime"`
	MultiAssetsMargin           bool         `json:"multiAssetsMargin"`
	TotalInitialMargin          types.Number `json:"totalInitialMargin"`
	TotalMaintenanceMargin      types.Number `json:"totalMaintMargin"`
	TotalWalletBalance          types.Number `json:"totalWalletBalance"`
	TotalUnrealizedProfit       types.Number `json:"totalUnrealizedProfit"`
	TotalMarginBalance          types.Number `json:"totalMarginBalance"`
	TotalPositionInitialMargin  types.Number `json:"totalPositionInitialMargin"`
	TotalOpenOrderInitialMargin types.Number `json:"totalOpenOrderInitialMargin"`
	TotalCrossWalletBalance     types.Number `json:"totalCrossWalletBalance"`
	TotalCrossUnrealizedPNL     types.Number `json:"totalCrossUnPnl"`
	AvailableBalance            types.Number `json:"availableBalance"`
	MaxWithdrawAmount           types.Number `json:"maxWithdrawAmount"`
	Assets                      []UAsset     `json:"assets"`
	Positions                   []UPosition  `json:"positions"`
}

// UAsset holds account asset information
type UAsset struct {
	Asset                  string       `json:"asset"`
	WalletBalance          types.Number `json:"walletBalance"`
	UnrealizedProfit       types.Number `json:"unrealizedProfit"`
	MarginBalance          types.Number `json:"marginBalance"`
	MaintenanceMargin      types.Number `json:"maintMargin"`
	InitialMargin          types.Number `json:"initialMargin"`
	PositionInitialMargin  types.Number `json:"positionInitialMargin"`
	OpenOrderInitialMargin types.Number `json:"openOrderInitialMargin"`
	CrossWalletBalance     types.Number `json:"crossWalletBalance"`
	CrossUnPnl             types.Number `json:"crossUnPnl"`
	AvailableBalance       types.Number `json:"availableBalance"`
	MaxWithdrawAmount      types.Number `json:"maxWithdrawAmount"`
}

// UPosition holds account position information
type UPosition struct {
	Symbol                 string       `json:"symbol"`
	InitialMargin          types.Number `json:"initialMargin"`
	MaintenanceMargin      types.Number `json:"maintMargin"`
	UnrealisedProfit       types.Number `json:"unrealizedProfit"`
	PositionInitialMargin  types.Number `json:"positionInitialMargin"`
	OpenOrderInitialMargin types.Number `json:"openOrderInitialMargin"`
	Leverage               types.Number `json:"leverage"`
	Isolated               bool         `json:"isolated"`
	IsolatedWallet         types.Number `json:"isolatedWallet"`
	EntryPrice             types.Number `json:"entryPrice"`
	MaxNotional            types.Number `json:"maxNotional"`
	BidNotional            types.Number `json:"bidNotional"`
	AskNotional            types.Number `json:"askNotional"`
	PositionSide           string       `json:"positionSide"`
	PositionAmount         types.Number `json:"positionAmt"`
	UpdateTime             types.Time   `json:"updateTime"`
}

// UChangeInitialLeverage stores leverage change data
type UChangeInitialLeverage struct {
	Leverage         int64        `json:"leverage"`
	MaxNotionalValue types.Number `json:"maxNotionalValue"`
	Symbol           string       `json:"symbol"`
}

// UModifyIsolatedPosMargin stores modified isolated margin positions' data
type UModifyIsolatedPosMargin struct {
	Amount     types.Number `json:"amount"`
	MarginType int64        `json:"type"`
}

// UPositionMarginChangeHistoryData gets position margin change history data
type UPositionMarginChangeHistoryData struct {
	Amount       types.Number `json:"amount"`
	Asset        string       `json:"asset"`
	Symbol       string       `json:"symbol"`
	Time         types.Time   `json:"time"`
	MarginType   int64        `json:"type"`
	PositionSide string       `json:"positionSide"`
}

// UPositionInformationV2 stores positions data
type UPositionInformationV2 struct {
	Symbol           string       `json:"symbol"`
	PositionAmount   types.Number `json:"positionAmt"`
	EntryPrice       types.Number `json:"entryPrice"`
	MarkPrice        types.Number `json:"markPrice"`
	UnrealizedProfit types.Number `json:"unrealizedProfit"`
	LiquidationPrice types.Number `json:"liquidationPrice"`
	Leverage         types.Number `json:"leverage"`
	MaxNotionalValue types.Number `json:"maxNotionalValue"`
	MarginType       string       `json:"marginType"`
	IsAutoAddMargin  bool         `json:"isAutoAddMargin,string"`
	PositionSide     string       `json:"positionSide"`
	Notional         types.Number `json:"notional"`
	IsolatedWallet   types.Number `json:"isolatedWallet"`
	IsolatedMargin   types.Number `json:"isolatedMargin"`
	UpdateTime       types.Time   `json:"updateTime"`
}

// UAccountTradeHistory stores trade data for the users account
type UAccountTradeHistory struct {
	Buyer           bool          `json:"buyer"`
	Commission      types.Number  `json:"commission"`
	CommissionAsset currency.Code `json:"commissionAsset"`
	ID              int64         `json:"id"`
	Maker           bool          `json:"maker"`
	OrderID         uint64        `json:"orderId"`
	Price           types.Number  `json:"price"`
	Quantity        types.Number  `json:"qty"`
	BaseQuantity    types.Number  `json:"baseQty"`
	QuoteQuantity   types.Number  `json:"quoteQty"`
	RealizedPNL     types.Number  `json:"realizedPnl"`
	MarginAsset     currency.Code `json:"marginAsset"`
	Side            string        `json:"side"`
	PositionSide    string        `json:"positionSide"`
	Symbol          string        `json:"symbol"`
	Pair            string        `json:"pair"`
	Time            types.Time    `json:"time"`
}

// UAccountIncomeHistory stores income history data
type UAccountIncomeHistory struct {
	Symbol     string       `json:"symbol"`
	IncomeType string       `json:"incomeType"`
	Income     types.Number `json:"income"`
	Asset      string       `json:"asset"`
	Info       string       `json:"info"`
	Time       types.Time   `json:"time"`
	TranID     uint64       `json:"tranId"`
	TradeID    string       `json:"tradeId"`
}

// UNotionalLeverageAndBrakcetsData stores notional and leverage brackets data for the account
type UNotionalLeverageAndBrakcetsData struct {
	Symbol   string             `json:"symbol"`
	Brackets []UNotionalBracket `json:"brackets"`
}

// UNotionalBracket is a single notional and leverage bracket
type UNotionalBracket struct {
	Bracket                int64   `json:"bracket"`
	InitialLeverage        float64 `json:"initialLeverage"`
	NotionalCap            float64 `json:"notionalCap"`
	NotionalFloor          float64 `json:"notionalFloor"`
	MaintenanceMarginRatio float64 `json:"maintMarginRatio"`
	Cumulative             float64 `json:"cum"`
}

// UPositionADLEstimationData stores ADL estimation data for a position
type UPositionADLEstimationData struct {
	Symbol      string `json:"symbol"`
	ADLQuantile struct {
		Long  int64 `json:"LONG"`
		Short int64 `json:"SHORT"`
		Hedge int64 `json:"HEDGE"`
	} `json:"adlQuantile"`
}

// UForceOrdersData stores liquidation orders data for the account
type UForceOrdersData struct {
	OrderID          uint64            `json:"orderId"`
	Symbol           string            `json:"symbol"`
	Status           string            `json:"status"`
	ClientOrderID    string            `json:"clientOrderId"`
	Price            types.Number      `json:"price"`
	AvgPrice         types.Number      `json:"avgPrice"`
	OriginalQuantity types.Number      `json:"origQty"`
	ExecutedQuantity types.Number      `json:"executedQty"`
	CumQuote         types.Number      `json:"cumQuote"`
	TimeInForce      order.TimeInForce `json:"timeInForce"`
	OrderType        string            `json:"type"`
	ReduceOnly       bool              `json:"reduceOnly"`
	ClosePosition    bool              `json:"closePosition"`
	Side             string            `json:"side"`
	PositionSide     string            `json:"positionSide"`
	StopPrice        types.Number      `json:"stopPrice"`
	WorkingType      string            `json:"workingType"`
	PriceProtect     bool              `json:"priceProtect"`
	OrigType         string            `json:"origType"`
	Time             types.Time        `json:"time"`
	UpdateTime       types.Time        `json:"updateTime"`
}

// UFuturesNewOrderRequest stores order data for placing
type UFuturesNewOrderRequest struct {
	Symbol           currency.Pair `json:"symbol"`
	Side             string        `json:"side"`
	PositionSide     string        `json:"positionSide,omitempty"`
	OrderType        string        `json:"type,omitempty"`
	TimeInForce      string        `json:"timeInForce,omitempty"`
	NewClientOrderID string        `json:"newClientOrderId,omitempty"`
	ClosePosition    string        `json:"closePosition,omitempty"`
	WorkingType      string        `json:"workingType,omitempty"`
	NewOrderRespType string        `json:"newOrderRespType,omitempty"`
	Quantity         float64       `json:"quantity,omitempty"`
	Price            float64       `json:"price,omitempty"`
	StopPrice        float64       `json:"stopPrice,omitempty"`
	ActivationPrice  float64       `json:"activationPrice,omitempty"`
	CallbackRate     float64       `json:"callbackRate,omitempty"`
	ReduceOnly       bool          `json:"reduceOnly,omitempty"`
}

// WebsocketAPIError represents an error message sent through the websocket API.
type WebsocketAPIError struct {
	Code    int64  `json:"code"`
	Message string `json:"msg"`
}

// SettlementPrice represents a quarterly contract settlement price information
type SettlementPrice struct {
	DeliveryTime  types.Time   `json:"deliveryTime"`
	DeliveryPrice types.Number `json:"deliveryPrice"`
}

// BasisInfo represents a basis price difference information between index and futures
type BasisInfo struct {
	IndexPrice          types.Number `json:"indexPrice"`
	ContractType        string       `json:"contractType"`
	BasisRate           types.Number `json:"basisRate"`
	FuturesPrice        types.Number `json:"futuresPrice"`
	AnnualizedBasisRate types.Number `json:"annualizedBasisRate"`
	Basis               types.Number `json:"basis"`
	Pair                string       `json:"pair"`
	Timestamp           types.Time   `json:"timestamp"`
}

// AssetIndex holds asset index detail for multi-assets mode
type AssetIndex struct {
	Symbol                string       `json:"symbol"`
	Time                  types.Time   `json:"time"`
	Index                 types.Number `json:"index"`
	AskBuffer             types.Number `json:"askBuffer"`
	BidBuffer             types.Number `json:"bidBuffer"`
	BidRate               types.Number `json:"bidRate"`
	AskRate               types.Number `json:"askRate"`
	AutoExchangeBidBuffer types.Number `json:"autoExchangeBidBuffer"`
	AutoExchangeAskBuffer types.Number `json:"autoExchangeAskBuffer"`
	AutoExchangeBidRate   types.Number `json:"autoExchangeBidRate"`
	AutoExchangeAskRate   types.Number `json:"autoExchangeAskRate"`
}

// AssetIndexResponse represents a list of asset indexes
type AssetIndexResponse []*AssetIndex

// IndexPriceConstituent represents an index price constituents
type IndexPriceConstituent struct {
	Symbol       string     `json:"symbol"`
	Time         types.Time `json:"time"`
	Constituents []struct {
		Exchange string `json:"exchange"`
		Symbol   string `json:"symbol"`
	} `json:"constituents"`
}

// PositionMode represents whether the position mode is 'hedge mode' or 'one-way mode'
type PositionMode struct {
	DualSidePosition bool `json:"dualSidePosition"` // "true": Hedge Mode; "false": One-way Mode
}

// USDTOrderUpdateRequest represents an updating parameter of USDT margined futures orders.
type USDTOrderUpdateRequest struct {
	OrderID           uint64        `json:"orderID"`
	OrigClientOrderID string        `json:"origClientOrderID,omitempty"`
	Side              string        `json:"side,omitempty"`
	PriceMatch        string        `json:"priceMatch,omitempty"`
	Symbol            currency.Pair `json:"symbol"`
	Amount            float64       `json:"quantity,omitempty"`
	Price             float64       `json:"price,omitempty"`
}

// USDTAmendInfo represents a USDT margined futures order amendment history item.
type USDTAmendInfo struct {
	AmendmentID   int64      `json:"amendmentId"`
	Symbol        string     `json:"symbol"`
	Pair          string     `json:"pair"`
	OrderID       uint64     `json:"orderId"`
	ClientOrderID string     `json:"clientOrderId"`
	Time          types.Time `json:"time"`
	Amendment     struct {
		Price struct {
			Before types.Number `json:"before"`
			After  types.Number `json:"after"`
		} `json:"price"`
		OriginalQuantity struct {
			Before types.Number `json:"before"`
			After  types.Number `json:"after"`
		} `json:"origQty"`
		Count int64 `json:"count"`
	} `json:"amendment"`
	PriceMatch string `json:"priceMatch"`
}

// TradingQuantitativeRulesIndicators represents a trading quantity rules indicators instance.
type TradingQuantitativeRulesIndicators struct {
	Indicators map[string][]struct {
		IsLocked           bool       `json:"isLocked"`
		PlannedRecoverTime types.Time `json:"plannedRecoverTime"`
		Indicator          string     `json:"indicator"`
		Value              float64    `json:"value"`
		TriggerValue       float64    `json:"triggerValue"`
	} `json:"indicators"`
	UpdateTime types.Time `json:"updateTime"`
}

// RateLimitInfo represents users rate limit information
type RateLimitInfo struct {
	RateLimitType string `json:"rateLimitType"`
	Interval      string `json:"interval"`
	IntervalNum   int64  `json:"intervalNum"`
	Limit         int64  `json:"limit"`
}

// UTransactionDownloadID represents a future transaction download ID.
type UTransactionDownloadID struct {
	AvgCostTimestampOfLast30D types.Time `json:"avgCostTimestampOfLast30d"`
	DownloadID                string     `json:"downloadId"`
}

// UTransactionHistoryDownloadLink represents a futures transaction history download link
type UTransactionHistoryDownloadLink struct {
	DownloadID          string     `json:"downloadId"`
	Status              string     `json:"status"` // Enum：completed，processing
	URL                 string     `json:"url"`
	Notified            bool       `json:"notified"`
	ExpirationTimestamp types.Time `json:"expirationTimestamp"`
	IsExpired           any        `json:"isExpired"`
	S3Link              any        `json:"s3Link"`
}

// WSBalanceAndPositionUpdate represents usd margined account balance update stream data
type WSBalanceAndPositionUpdate struct {
	EventType   string      `json:"e"`
	EventTime   types.Time  `json:"E"`
	Transaction types.Time  `json:"T"`
	UpdateData  *UpdateData `json:"a"`
}

// UpdateData re[resents a balance and position update detail
type UpdateData struct {
	ReasonType string               `json:"m"`
	Balances   []*WSAccountBalance  `json:"B"`
	Positions  []*WSAccountPosition `json:"P"`
}

// WSAccountBalance represents an account balance detail
type WSAccountBalance struct {
	Asset         currency.Code `json:"a"`
	WalletBalance types.Number  `json:"wb"`
	CrossWallet   types.Number  `json:"cw"`
	BalanceChange types.Number  `json:"bc"`
}

// WSAccountPosition represents an account position
type WSAccountPosition struct {
	Symbol              string       `json:"s"`
	PositionAmount      types.Number `json:"pa"`
	EntryPrice          types.Number `json:"ep"`
	BreakevenPrice      types.Number `json:"bep"`
	AccumulatedRealized types.Number `json:"cr,omitempty"`
	UnrealizedPNL       types.Number `json:"up"`
	MarginType          string       `json:"mt"`
	IsolatedWallet      types.Number `json:"iw"`
	PositionSide        string       `json:"ps"`
}

// FuturesOrderTradeUpdate is the ORDER_TRADE_UPDATE user data event, pushed whenever a
// futures order is created, filled, cancelled or expired.
type FuturesOrderTradeUpdate struct {
	EventType       string                       `json:"e"`
	EventTime       types.Time                   `json:"E"`
	TransactionTime types.Time                   `json:"T"`
	Order           *FuturesOrderTradeUpdateData `json:"o"`
}

// FuturesOrderTradeUpdateData carries the order detail of an ORDER_TRADE_UPDATE event.
type FuturesOrderTradeUpdateData struct {
	Symbol                    string            `json:"s"`
	ClientOrderID             string            `json:"c"`
	Side                      string            `json:"S"`
	OrderType                 string            `json:"o"`
	TimeInForce               order.TimeInForce `json:"f"`
	OriginalQuantity          types.Number      `json:"q"`
	OriginalPrice             types.Number      `json:"p"`
	AveragePrice              types.Number      `json:"ap"`
	StopPrice                 types.Number      `json:"sp"`
	ExecutionType             string            `json:"x"`
	OrderStatus               string            `json:"X"`
	OrderID                   uint64            `json:"i"`
	LastFilledQuantity        types.Number      `json:"l"`
	FilledAccumulatedQuantity types.Number      `json:"z"`
	LastFilledPrice           types.Number      `json:"L"`
	CommissionAsset           currency.Code     `json:"N"`
	Commission                types.Number      `json:"n"`
	OrderTradeTime            types.Time        `json:"T"`
	TradeID                   int64             `json:"t"`
	BidsNotional              types.Number      `json:"b"`
	AskNotional               types.Number      `json:"a"`
	IsMaker                   bool              `json:"m"`
	IsReduceOnly              bool              `json:"R"`
	StopPriceWorkingType      string            `json:"wt"`
	OriginalOrderType         string            `json:"ot"`
	PositionSide              string            `json:"ps"`
	CloseAll                  bool              `json:"cp"`
	ActivationPrice           types.Number      `json:"AP"`
	CallbackRate              types.Number      `json:"cr"`
	RealizedProfit            types.Number      `json:"rp"`
	SelfTradePreventionMode   string            `json:"V"`
	PriceMatchType            string            `json:"pm"`
	GoodTillDate              types.Time        `json:"gtd"`
}

// FuturesAccountConfigUpdate is the ACCOUNT_CONFIG_UPDATE user data event, pushed when the
// leverage of a symbol changes (ac) or the multi-assets margin mode changes (ai).
type FuturesAccountConfigUpdate struct {
	EventType       string     `json:"e"`
	EventTime       types.Time `json:"E"`
	TransactionTime types.Time `json:"T"`
	SymbolConfig    *struct {
		Symbol   string  `json:"s"`
		Leverage float64 `json:"l"`
	} `json:"ac"`
	AccountConfig *struct {
		MultiAssetsMode bool `json:"j"`
	} `json:"ai"`
}

// FuturesListenKeyExpired is the listenKeyExpired user data event, pushed when the listen key
// backing the connection expires. No further user data arrives until a new key is obtained.
type FuturesListenKeyExpired struct {
	EventType string     `json:"e"`
	EventTime types.Time `json:"E"`
	ListenKey string     `json:"listenKey"`
}

// UFuturesAlgoOrderRequest holds parameters for placing a USD-M futures algo order.
// Algo orders carry every conditional order type: STOP, STOP_MARKET, TAKE_PROFIT,
// TAKE_PROFIT_MARKET and TRAILING_STOP_MARKET all route here rather than through /fapi/v1/order.
type UFuturesAlgoOrderRequest struct {
	AlgoType              string            `json:"algoType"` // currently only "CONDITIONAL"
	Symbol                currency.Pair     `json:"symbol"`
	Side                  string            `json:"side"`
	OrderType             string            `json:"type"`
	PositionSide          string            `json:"positionSide,omitempty"`
	TimeInForce           order.TimeInForce `json:"timeInForce,omitempty"`
	Quantity              float64           `json:"quantity,omitempty"`
	Price                 float64           `json:"price,omitempty"`
	TriggerPrice          float64           `json:"triggerPrice,omitempty"`
	WorkingType           string            `json:"workingType,omitempty"`
	PriceMatch            string            `json:"priceMatch,omitempty"`
	ClosePosition         bool              `json:"closePosition,omitempty"`
	PriceProtect          bool              `json:"priceProtect,omitempty"`
	ReduceOnly            bool              `json:"reduceOnly,omitempty"`
	ActivatePrice         float64           `json:"activatePrice,omitempty"`
	CallbackRate          float64           `json:"callbackRate,omitempty"`
	ClientAlgoID          string            `json:"clientAlgoId,omitempty"`
	GoodTillDate          time.Time         `json:"-"`
	GoodTillDateTimestamp int64             `json:"goodTillDate,omitempty"`
}

// UFuturesAlgoOrder represents a USD-M futures algo order.
type UFuturesAlgoOrder struct {
	AlgoID                  uint64            `json:"algoId"`
	ClientAlgoID            string            `json:"clientAlgoId"`
	AlgoType                string            `json:"algoType"`
	OrderType               string            `json:"orderType"`
	Symbol                  string            `json:"symbol"`
	Side                    string            `json:"side"`
	PositionSide            string            `json:"positionSide"`
	TimeInForce             order.TimeInForce `json:"timeInForce"`
	Quantity                types.Number      `json:"quantity"`
	AlgoStatus              string            `json:"algoStatus"`
	TriggerPrice            types.Number      `json:"triggerPrice"`
	Price                   types.Number      `json:"price"`
	IcebergQuantity         types.Number      `json:"icebergQuantity"`
	SelfTradePreventionMode string            `json:"selfTradePreventionMode"`
	WorkingType             string            `json:"workingType"`
	PriceMatch              string            `json:"priceMatch"`
	ClosePosition           bool              `json:"closePosition"`
	PriceProtect            bool              `json:"priceProtect"`
	ReduceOnly              bool              `json:"reduceOnly"`
	ActivatePrice           types.Number      `json:"activatePrice"`
	CallbackRate            types.Number      `json:"callbackRate"`
	CreateTime              types.Time        `json:"createTime"`
	UpdateTime              types.Time        `json:"updateTime"`
	TriggerTime             types.Time        `json:"triggerTime"`
	GoodTillDate            types.Time        `json:"goodTillDate"`
}

// UFuturesAlgoOrderCancelResponse is returned when cancelling a single algo order.
type UFuturesAlgoOrderCancelResponse struct {
	AlgoID       uint64 `json:"algoId"`
	ClientAlgoID string `json:"clientAlgoId"`
	Code         int64  `json:"code"`
	Message      string `json:"msg"`
}

// UFuturesAccountBalance is a USD-M futures account balance entry. The v3 response adds
// marginAvailable and updateTime over v2.
type UFuturesAccountBalance struct {
	AccountAlias       string        `json:"accountAlias"`
	Asset              currency.Code `json:"asset"`
	Balance            types.Number  `json:"balance"`
	CrossWalletBalance types.Number  `json:"crossWalletBalance"`
	CrossUnrealizedPNL types.Number  `json:"crossUnPnl"`
	AvailableBalance   types.Number  `json:"availableBalance"`
	MaxWithdrawAmount  types.Number  `json:"maxWithdrawAmount"`
	MarginAvailable    bool          `json:"marginAvailable"`
	UpdateTime         types.Time    `json:"updateTime"`
}

// UFuturesAccountConfig holds the account level trading configuration.
type UFuturesAccountConfig struct {
	FeeTier           int64      `json:"feeTier"`
	CanTrade          bool       `json:"canTrade"`
	CanDeposit        bool       `json:"canDeposit"`
	CanWithdraw       bool       `json:"canWithdraw"`
	DualSidePosition  bool       `json:"dualSidePosition"`
	UpdateTime        types.Time `json:"updateTime"`
	MultiAssetsMargin bool       `json:"multiAssetsMargin"`
	TradeGroupID      int64      `json:"tradeGroupId"`
}

// UFuturesFeeBurnStatus reports whether the BNB fee discount is enabled.
type UFuturesFeeBurnStatus struct {
	FeeBurn bool `json:"feeBurn"`
}

// UFuturesConvertPair is a convertible asset pair and its size bounds.
type UFuturesConvertPair struct {
	FromAsset          currency.Code `json:"fromAsset"`
	ToAsset            currency.Code `json:"toAsset"`
	FromAssetMinAmount types.Number  `json:"fromAssetMinAmount"`
	FromAssetMaxAmount types.Number  `json:"fromAssetMaxAmount"`
	ToAssetMinAmount   types.Number  `json:"toAssetMinAmount"`
	ToAssetMaxAmount   types.Number  `json:"toAssetMaxAmount"`
}

// UFuturesConvertQuoteRequest holds parameters for requesting a convert quote.
type UFuturesConvertQuoteRequest struct {
	FromAsset  currency.Code `json:"fromAsset"`
	ToAsset    currency.Code `json:"toAsset"`
	FromAmount float64       `json:"fromAmount,omitempty"`
	ToAmount   float64       `json:"toAmount,omitempty"`
	ValidTime  string        `json:"validTime,omitempty"` // 10s, 30s, 1m or 2m; defaults to 10s
}

// UFuturesConvertQuote is a convert quote, valid until ValidTimestamp.
type UFuturesConvertQuote struct {
	QuoteID        string       `json:"quoteId"`
	Ratio          types.Number `json:"ratio"`
	InverseRatio   types.Number `json:"inverseRatio"`
	ValidTimestamp types.Time   `json:"validTimestamp"`
	ToAmount       types.Number `json:"toAmount"`
	FromAmount     types.Number `json:"fromAmount"`
}

// UFuturesConvertAcceptance is returned when a convert quote is accepted.
type UFuturesConvertAcceptance struct {
	OrderID     string     `json:"orderId"`
	CreateTime  types.Time `json:"createTime"`
	OrderStatus string     `json:"orderStatus"`
}

// UFuturesConvertOrderStatus is the status of a convert order.
type UFuturesConvertOrderStatus struct {
	OrderID      uint64        `json:"orderId"`
	OrderStatus  string        `json:"orderStatus"`
	FromAsset    currency.Code `json:"fromAsset"`
	FromAmount   types.Number  `json:"fromAmount"`
	ToAsset      currency.Code `json:"toAsset"`
	ToAmount     types.Number  `json:"toAmount"`
	Ratio        types.Number  `json:"ratio"`
	InverseRatio types.Number  `json:"inverseRatio"`
	CreateTime   types.Time    `json:"createTime"`
}

// UFuturesInsuranceBalances holds the insurance fund response, which is a single object when
// a symbol is supplied and an array of them when it is not.
type UFuturesInsuranceBalances []*UFuturesInsuranceBalance

// UFuturesInsuranceBalance is a snapshot of one insurance fund group's balance.
type UFuturesInsuranceBalance struct {
	Symbols []string `json:"symbols"`
	Assets  []struct {
		Asset         currency.Code `json:"asset"`
		MarginBalance types.Number  `json:"marginBalance"`
		UpdateTime    types.Time    `json:"updateTime"`
	} `json:"assets"`
}

// UFuturesSymbolADLRisk is the auto-deleveraging risk rating for a symbol.
type UFuturesSymbolADLRisk struct {
	Symbol     string     `json:"symbol"`
	ADLRisk    string     `json:"adlRisk"`
	UpdateTime types.Time `json:"updateTime"`
}

// UFuturesIndexConstituents lists the exchanges and weights making up an index price.
type UFuturesIndexConstituents struct {
	Symbol       string     `json:"symbol"`
	Time         types.Time `json:"time"`
	Constituents []struct {
		Exchange string       `json:"exchange"`
		Symbol   string       `json:"symbol"`
		Price    types.Number `json:"price"`
		Weight   types.Number `json:"weight"`
	} `json:"constituents"`
}

// UFuturesTradingSchedule holds the trading session schedule per market. Markets observed
// live are HK_EQUITY, EQUITY, COMMODITY and KR_EQUITY; the map keeps new ones working
// without a code change.
type UFuturesTradingSchedule struct {
	UpdateTime      types.Time `json:"updateTime"`
	MarketSchedules map[string]struct {
		Sessions []struct {
			StartTime types.Time `json:"startTime"`
			EndTime   types.Time `json:"endTime"`
			Type      string     `json:"type"` // REGULAR or NO_TRADING
		} `json:"sessions"`
	} `json:"marketSchedules"`
}

// UFuturesSymbolConfig is the per symbol USD-M configuration. Unlike the portfolio margin
// equivalent, isAutoAddMargin is returned here as a bare bool.
type UFuturesSymbolConfig struct {
	Symbol           string       `json:"symbol"`
	MarginType       string       `json:"marginType"`
	IsAutoAddMargin  bool         `json:"isAutoAddMargin"`
	Leverage         int64        `json:"leverage"`
	MaxNotionalValue types.Number `json:"maxNotionalValue"`
}

// UFuturesAccountV3 is the V3 USD-M account information.
type UFuturesAccountV3 struct {
	TotalInitialMargin          types.Number                `json:"totalInitialMargin"`
	TotalMaintenanceMargin      types.Number                `json:"totalMaintMargin"`
	TotalWalletBalance          types.Number                `json:"totalWalletBalance"`
	TotalUnrealizedProfit       types.Number                `json:"totalUnrealizedProfit"`
	TotalMarginBalance          types.Number                `json:"totalMarginBalance"`
	TotalPositionInitialMargin  types.Number                `json:"totalPositionInitialMargin"`
	TotalOpenOrderInitialMargin types.Number                `json:"totalOpenOrderInitialMargin"`
	TotalCrossWalletBalance     types.Number                `json:"totalCrossWalletBalance"`
	TotalCrossUnrealizedPNL     types.Number                `json:"totalCrossUnPnl"`
	AvailableBalance            types.Number                `json:"availableBalance"`
	MaxWithdrawAmount           types.Number                `json:"maxWithdrawAmount"`
	Assets                      []UFuturesAccountAssetV3    `json:"assets"`
	Positions                   []UFuturesAccountPositionV3 `json:"positions"`
}

// UFuturesAccountAssetV3 is a per-asset balance entry of a V3 USD-M account.
type UFuturesAccountAssetV3 struct {
	Asset                  currency.Code `json:"asset"`
	WalletBalance          types.Number  `json:"walletBalance"`
	UnrealizedProfit       types.Number  `json:"unrealizedProfit"`
	MarginBalance          types.Number  `json:"marginBalance"`
	MaintenanceMargin      types.Number  `json:"maintMargin"`
	InitialMargin          types.Number  `json:"initialMargin"`
	PositionInitialMargin  types.Number  `json:"positionInitialMargin"`
	OpenOrderInitialMargin types.Number  `json:"openOrderInitialMargin"`
	CrossWalletBalance     types.Number  `json:"crossWalletBalance"`
	CrossUnrealizedPNL     types.Number  `json:"crossUnPnl"`
	AvailableBalance       types.Number  `json:"availableBalance"`
	MaxWithdrawAmount      types.Number  `json:"maxWithdrawAmount"`
	UpdateTime             types.Time    `json:"updateTime"`
}

// UFuturesAccountPositionV3 is a position entry of a V3 USD-M account.
type UFuturesAccountPositionV3 struct {
	Symbol            string       `json:"symbol"`
	PositionSide      string       `json:"positionSide"`
	PositionAmount    types.Number `json:"positionAmt"`
	UnrealizedProfit  types.Number `json:"unrealizedProfit"`
	IsolatedMargin    types.Number `json:"isolatedMargin"`
	Notional          types.Number `json:"notional"`
	IsolatedWallet    types.Number `json:"isolatedWallet"`
	InitialMargin     types.Number `json:"initialMargin"`
	MaintenanceMargin types.Number `json:"maintMargin"`
	UpdateTime        types.Time   `json:"updateTime"`
}

// UFuturesPositionV3 is a V3 USD-M position.
type UFuturesPositionV3 struct {
	Symbol                 string        `json:"symbol"`
	PositionSide           string        `json:"positionSide"`
	PositionAmount         types.Number  `json:"positionAmt"`
	EntryPrice             types.Number  `json:"entryPrice"`
	BreakEvenPrice         types.Number  `json:"breakEvenPrice"`
	MarkPrice              types.Number  `json:"markPrice"`
	UnrealizedProfit       types.Number  `json:"unRealizedProfit"`
	LiquidationPrice       types.Number  `json:"liquidationPrice"`
	IsolatedMargin         types.Number  `json:"isolatedMargin"`
	Notional               types.Number  `json:"notional"`
	MarginAsset            currency.Code `json:"marginAsset"`
	IsolatedWallet         types.Number  `json:"isolatedWallet"`
	InitialMargin          types.Number  `json:"initialMargin"`
	MaintenanceMargin      types.Number  `json:"maintMargin"`
	PositionInitialMargin  types.Number  `json:"positionInitialMargin"`
	OpenOrderInitialMargin types.Number  `json:"openOrderInitialMargin"`
	UpdateTime             types.Time    `json:"updateTime"`
}

// ClassicPortfolioMarginAccountInfo is the classic portfolio margin account information.
type ClassicPortfolioMarginAccountInfo struct {
	MaxWithdrawAmountUSD types.Number  `json:"maxWithdrawAmountUSD"`
	Asset                currency.Code `json:"asset"`
	MaxWithdrawAmount    types.Number  `json:"maxWithdrawAmount"`
}

// UCompressedTradesRequest holds the parameters for UCompressedTrades.
type UCompressedTradesRequest struct {
	Symbol    currency.Pair
	FromID    string
	Limit     int64
	StartTime time.Time
	EndTime   time.Time
}

// UKlineDataRequest holds the parameters for UKlineData.
type UKlineDataRequest struct {
	Symbol    currency.Pair
	Interval  string
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetUFuturesContinuousKlineDataRequest holds the parameters for GetUFuturesContinuousKlineData.
type GetUFuturesContinuousKlineDataRequest struct {
	Pair         currency.Pair
	ContractType string
	Interval     string
	StartTime    time.Time
	EndTime      time.Time
	Limit        int64
}

// GetIndexPriceKlineDataRequest holds the parameters for GetIndexPriceKlineData.
type GetIndexPriceKlineDataRequest struct {
	Pair      currency.Pair
	Interval  string
	StartTime time.Time
	EndTime   time.Time
	Limit     int64
}

// GetMarkPriceKlineCandlesticksRequest holds the parameters for GetMarkPriceKlineCandlesticks.
type GetMarkPriceKlineCandlesticksRequest struct {
	Symbol    currency.Pair
	Interval  string
	StartTime time.Time
	EndTime   time.Time
	Limit     int64
}

// GetPremiumIndexKlineCandlesticksRequest holds the parameters for GetPremiumIndexKlineCandlesticks.
type GetPremiumIndexKlineCandlesticksRequest struct {
	Symbol    currency.Pair
	Interval  string
	StartTime time.Time
	EndTime   time.Time
	Limit     int64
}

// UOpenInterestStatsRequest holds the parameters for UOpenInterestStats.
type UOpenInterestStatsRequest struct {
	Symbol    currency.Pair
	Period    string
	Limit     int64
	StartTime time.Time
	EndTime   time.Time
}

// UTopAccountsLongShortRatioRequest holds the parameters for UTopAccountsLongShortRatio.
type UTopAccountsLongShortRatioRequest struct {
	Symbol    currency.Pair
	Period    string
	Limit     int64
	StartTime time.Time
	EndTime   time.Time
}

// UTopPositionsLongShortRatioRequest holds the parameters for UTopPositionsLongShortRatio.
type UTopPositionsLongShortRatioRequest struct {
	Symbol    currency.Pair
	Period    string
	Limit     int64
	StartTime time.Time
	EndTime   time.Time
}

// UGlobalLongShortRatioRequest holds the parameters for UGlobalLongShortRatio.
type UGlobalLongShortRatioRequest struct {
	Symbol    currency.Pair
	Period    string
	Limit     int64
	StartTime time.Time
	EndTime   time.Time
}

// UTakerBuySellVolRequest holds the parameters for UTakerBuySellVol.
type UTakerBuySellVolRequest struct {
	Symbol    currency.Pair
	Period    string
	Limit     int64
	StartTime time.Time
	EndTime   time.Time
}

// GetBasisRequest holds the parameters for GetBasis.
type GetBasisRequest struct {
	Pair         currency.Pair
	ContractType string
	Period       string
	StartTime    time.Time
	EndTime      time.Time
	Limit        int64
}

// GetUSDTOrderModifyHistoryRequest holds the parameters for GetUSDTOrderModifyHistory.
type GetUSDTOrderModifyHistoryRequest struct {
	Symbol            currency.Pair
	OrigClientOrderID string
	OrderID           int64
	Limit             int64
	StartTime         time.Time
	EndTime           time.Time
}

// UAllAccountOrdersRequest holds the parameters for UAllAccountOrders.
type UAllAccountOrdersRequest struct {
	Symbol    currency.Pair
	OrderID   uint64
	Limit     int64
	StartTime time.Time
	EndTime   time.Time
}

// UPositionMarginChangeHistoryRequest holds the parameters for UPositionMarginChangeHistory.
type UPositionMarginChangeHistoryRequest struct {
	Symbol     currency.Pair
	ChangeType string
	Limit      int64
	StartTime  time.Time
	EndTime    time.Time
}

// UAccountTradesHistoryRequest holds the parameters for UAccountTradesHistory.
type UAccountTradesHistoryRequest struct {
	Symbol    currency.Pair
	FromID    string
	Limit     int64
	StartTime time.Time
	EndTime   time.Time
}

// UAccountIncomeHistoryRequest holds the parameters for UAccountIncomeHistory.
type UAccountIncomeHistoryRequest struct {
	Symbol     currency.Pair
	IncomeType string
	Limit      int64
	StartTime  time.Time
	EndTime    time.Time
}

// UAccountForcedOrdersRequest holds the parameters for UAccountForcedOrders.
type UAccountForcedOrdersRequest struct {
	Symbol        currency.Pair
	AutoCloseType string
	Limit         int64
	StartTime     time.Time
	EndTime       time.Time
}
