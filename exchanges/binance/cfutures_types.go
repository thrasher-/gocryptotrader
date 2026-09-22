package binance

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Response holds basic binance api response data
type Response struct {
	Code int64  `json:"code"` // Signed because Binance error codes are negative
	Msg  string `json:"msg"`
}

// FuturesPublicTradesData stores recent public trades for futures. Quantity counts contracts, so
// BaseQuantity is the traded base asset amount; coin margined futures sends no quote figure
type FuturesPublicTradesData struct {
	ID           int64        `json:"id"`
	Price        types.Number `json:"price"`
	Quantity     types.Number `json:"qty"`
	BaseQuantity types.Number `json:"baseQty"`
	Time         types.Time   `json:"time"`
	IsBuyerMaker bool         `json:"isBuyerMaker"`
}

// CompressedTradesData stores futures trades data in a compressed format
type CompressedTradesData struct {
	TradeID      int64      `json:"a"`
	Price        float64    `json:"p"`
	Quantity     float64    `json:"q"`
	FirstTradeID uint64     `json:"f"`
	LastTradeID  uint64     `json:"l"`
	Timestamp    types.Time `json:"t"`
	BuyerMaker   bool       `json:"b"`
}

// SymbolPriceTicker stores ticker price stats
type SymbolPriceTicker struct {
	Symbol string       `json:"symbol"`
	Price  types.Number `json:"price"`
	Time   types.Time   `json:"time"`
}

// SymbolOrderBookTicker stores orderbook ticker data
type SymbolOrderBookTicker struct {
	Symbol      string       `json:"symbol"`
	BidPrice    types.Number `json:"bidPrice"`
	AskPrice    types.Number `json:"askPrice"`
	BidQuantity types.Number `json:"bidQty"`
	AskQuantity types.Number `json:"askQty"`
	Time        types.Time   `json:"time"`
}

// FuturesCandleStick holds kline data
type FuturesCandleStick struct {
	OpenTime                types.Time
	Open                    types.Number
	High                    types.Number
	Low                     types.Number
	Close                   types.Number
	Volume                  types.Number
	CloseTime               types.Time
	BaseAssetVolume         types.Number
	NumberOfTrades          int64
	TakerBuyVolume          types.Number
	TakerBuyBaseAssetVolume types.Number
}

// UFuturesCandleStick holds kline data for USDT/USDC margined futures contracts
type UFuturesCandleStick struct {
	FuturesCandleStick
	QuoteAssetVolume types.Number
}

// UnmarshalJSON deserialises byte data into a UFuturesCandleStick instance
func (f *UFuturesCandleStick) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, &[12]any{&f.OpenTime, &f.Open, &f.High, &f.Low, &f.Close, &f.Volume, &f.CloseTime, &f.QuoteAssetVolume, &f.NumberOfTrades, &f.TakerBuyVolume, &f.TakerBuyBaseAssetVolume, nil})
	return err
}

// CFuturesCandleStick holds kline data for coin margined futures contracts
type CFuturesCandleStick struct {
	FuturesCandleStick
	BaseAssetVolume types.Number
}

// UnmarshalJSON deserialises byte data into a CFuturesCandleStick instance
func (f *CFuturesCandleStick) UnmarshalJSON(data []byte) error {
	target := [12]any{&f.OpenTime, &f.Open, &f.High, &f.Low, &f.Close, &f.Volume, &f.CloseTime, &f.BaseAssetVolume, &f.NumberOfTrades, &f.TakerBuyVolume, &f.TakerBuyBaseAssetVolume, nil}
	err := json.Unmarshal(data, &target)
	return err
}

// AllLiquidationOrders gets all liquidation orders
type AllLiquidationOrders struct {
	Symbol           string       `json:"symbol"`
	Price            types.Number `json:"price"`
	OriginalQuantity types.Number `json:"origQty"`
	ExecutedQuantity types.Number `json:"executedQty"`
	AveragePrice     types.Number `json:"averagePrice"`
	Status           string       `json:"status"`
	TimeInForce      string       `json:"timeInForce"`
	OrderType        string       `json:"type"`
	Side             string       `json:"side"`
	Time             types.Time   `json:"time"`
}

// OpenInterestData stores open interest data
type OpenInterestData struct {
	Symbol       string       `json:"symbol"`
	Pair         string       `json:"pair"`
	OpenInterest types.Number `json:"openInterest"`
	ContractType string       `json:"contractType"`
	Time         types.Time   `json:"time"`
}

// OpenInterestStats stores stats for open interest data
type OpenInterestStats struct {
	Pair                 string       `json:"pair"`
	ContractType         string       `json:"contractType"`
	SumOpenInterest      types.Number `json:"sumOpenInterest"`
	SumOpenInterestValue types.Number `json:"sumOpenInterestValue"`
	Timestamp            types.Time   `json:"timestamp"`
}

// TopTraderAccountRatio stores account ratio data for top traders
type TopTraderAccountRatio struct {
	Pair           string       `json:"pair"`
	LongShortRatio types.Number `json:"longShortRatio"`
	LongAccount    types.Number `json:"longAccount"`
	ShortAccount   types.Number `json:"shortAccount"`
	Timestamp      types.Time   `json:"timestamp"`
}

// TopTraderPositionRatio stores position ratio for top trader accounts
type TopTraderPositionRatio struct {
	Pair           string       `json:"pair"`
	LongShortRatio types.Number `json:"longShortRatio"`
	LongPosition   types.Number `json:"longPosition"`
	ShortPosition  types.Number `json:"shortPosition"`
	Timestamp      types.Time   `json:"timestamp"`
}

// TakerBuySellVolume stores taker buy sell volume
type TakerBuySellVolume struct {
	Pair           string       `json:"pair"`
	ContractType   string       `json:"contractType"`
	TakerBuyVolume types.Number `json:"takerBuyVol"`
	BuySellRatio   types.Number `json:"takerSellVol"`
	BuyVol         types.Number `json:"takerBuyVolValue"`
	SellVol        types.Number `json:"takerSellVolValue"`
	Timestamp      types.Time   `json:"timestamp"`
}

// FuturesBasisData gets futures basis data
type FuturesBasisData struct {
	Pair         string       `json:"pair"`
	ContractType string       `json:"contractType"`
	FuturesPrice types.Number `json:"futuresPrice"`
	IndexPrice   types.Number `json:"indexPrice"`
	Basis        types.Number `json:"basis"`
	BasisRate    types.Number `json:"basisRate"`
	Timestamp    types.Time   `json:"timestamp"`
}

// PlaceBatchOrderData stores batch order data for placing
type PlaceBatchOrderData struct {
	Symbol           currency.Pair `json:"symbol"`
	Side             string        `json:"side"`
	PositionSide     string        `json:"positionSide,omitempty"`
	OrderType        string        `json:"type"`
	TimeInForce      string        `json:"timeInForce,omitempty"`
	Quantity         float64       `json:"quantity"`
	ReduceOnly       string        `json:"reduceOnly,omitempty"`
	Price            float64       `json:"price"`
	NewClientOrderID string        `json:"newClientOrderId,omitempty"`
	StopPrice        float64       `json:"stopPrice,omitempty"`
	ActivationPrice  float64       `json:"activationPrice,omitempty"`
	CallbackRate     float64       `json:"callbackRate,omitempty"`
	WorkingType      string        `json:"workingType,omitempty"`
	PriceProtect     string        `json:"priceProtect,omitempty"`
	NewOrderRespType string        `json:"newOrderRespType,omitempty"`
}

// BatchCancelOrderData stores batch cancel order data
type BatchCancelOrderData struct {
	ClientOrderID           string       `json:"clientOrderId"`
	CumulativeQuantity      types.Number `json:"cumQty"`
	ExecutedQuantity        types.Number `json:"executedQty"`
	OrderID                 int64        `json:"orderId"`
	OriginalQuantity        types.Number `json:"origQty"`
	Price                   types.Number `json:"price"`
	ReduceOnly              bool         `json:"reduceOnly"`
	Side                    string       `json:"side"`
	PositionSide            string       `json:"positionSide"`
	Status                  string       `json:"status"`
	StopPrice               types.Number `json:"stopPrice"`
	ClosePosition           bool         `json:"closePosition"`
	Symbol                  string       `json:"symbol"`
	Pair                    string       `json:"pair"`
	TimeInForce             string       `json:"timeInForce"`
	OrderType               string       `json:"type"`
	OriginalType            string       `json:"origType"`
	ActivatePrice           types.Number `json:"activatePrice"`
	PriceRate               types.Number `json:"priceRate"`
	UpdateTime              types.Time   `json:"updateTime"`
	WorkingType             string       `json:"workingType"`
	PriceProtect            bool         `json:"priceProtect"`
	PriceMatch              string       `json:"priceMatch"`
	SelfTradePreventionMode string       `json:"selfTradePreventionMode"`
	Code                    int64        `json:"code"`
	Message                 string       `json:"msg"`
	CumBase                 types.Number `json:"cumBase"`
	AvgPrice                types.Number `json:"avgPrice"`
}

// FuturesNewOrderRequest stores all the data needed to submit a
// delivery/coin-margined-futures order.
type FuturesNewOrderRequest struct {
	Symbol           currency.Pair `json:"symbol"`
	Side             string        `json:"side,omitempty"`
	PositionSide     string        `json:"positionSide,omitempty"`
	OrderType        string        `json:"type,omitempty"`
	TimeInForce      string        `json:"timeInForce,omitempty"`
	NewClientOrderID string        `json:"newClientOrderID,omitempty"`
	ClosePosition    string        `json:"closePosition,omitempty"`
	WorkingType      string        `json:"workingType,omitempty"`
	NewOrderRespType string        `json:"newOrderRespType,omitempty"`
	Quantity         float64       `json:"quantity,omitempty"`
	Price            float64       `json:"price,omitempty"`
	StopPrice        float64       `json:"stopPrice,omitempty"`
	ActivationPrice  float64       `json:"activationPrice,omitempty"`
	CallbackRate     float64       `json:"callbackRate,omitempty"`
	ReduceOnly       bool          `json:"reduceOnly,omitempty"`
	PriceProtect     bool          `json:"priceProtect,omitempty"`
}

// FuturesOrderPlaceData stores futures order data
type FuturesOrderPlaceData struct {
	ClientOrderID           string            `json:"clientOrderId"`
	CumulativeQuantity      types.Number      `json:"cumQty"`
	ExecutedQuantity        types.Number      `json:"executedQty"`
	OrderID                 uint64            `json:"orderId"`
	OriginalQuantity        types.Number      `json:"origQty"`
	Price                   types.Number      `json:"price"`
	ReduceOnly              bool              `json:"reduceOnly"`
	Side                    string            `json:"side"`
	PositionSide            string            `json:"positionSide"`
	Status                  string            `json:"status"`
	StopPrice               types.Number      `json:"stopPrice"`
	ClosePosition           bool              `json:"closePosition"`
	Symbol                  string            `json:"symbol"`
	Pair                    string            `json:"pair"`
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
	// Code and Message carry a per-item rejection in a batch placement response, where an entry
	// that failed is otherwise indistinguishable from a zero-valued success
	Code           int64        `json:"code"`
	Message        string       `json:"msg"`
	AveragePrice   types.Number `json:"avgPrice"`
	CumulativeBase types.Number `json:"cumBase"`
	Time           types.Time   `json:"time"`
}

// FuturesOrderGetData stores futures order data for get requests
type FuturesOrderGetData struct {
	AveragePrice            types.Number      `json:"avgPrice"`
	ClientOrderID           string            `json:"clientOrderId"`
	CumulativeQuantity      types.Number      `json:"cumQty"`
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
	TimeInForce             order.TimeInForce `json:"timeInForce"`
	OrderType               string            `json:"type"`
	ActivatePrice           types.Number      `json:"activatePrice"`
	PriceRate               types.Number      `json:"priceRate"`
	Time                    types.Time        `json:"time"`
	UpdateTime              types.Time        `json:"updateTime"`
	WorkingType             string            `json:"workingType"`
	PriceProtect            bool              `json:"priceProtect"`
	PriceMatch              string            `json:"priceMatch"`
	SelfTradePreventionMode string            `json:"selfTradePreventionMode"`
}

// FuturesOrderData stores order data for futures. cumQuote and goodTillDate were added to
// allOrders on 2026-08-05; openOrders shares this type and does not send them, so they are zero there
type FuturesOrderData struct {
	AveragePrice            types.Number      `json:"avgPrice"`
	ClientOrderID           string            `json:"clientOrderId"`
	CumulativeBase          types.Number      `json:"cumBase"`
	CumulativeQuote         types.Number      `json:"cumQuote"`
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

// OrderVars stores side, status and type for any order/trade
type OrderVars struct {
	Side      order.Side
	Status    order.Status
	OrderType order.Type
	Fee       float64
}

// AutoCancelAllOrdersData gives data of auto cancelling all open orders
type AutoCancelAllOrdersData struct {
	Symbol        string       `json:"symbol"`
	CountdownTime types.Number `json:"countdownTime"`
}

// LevelDetail stores level detail data
type LevelDetail struct {
	Level         string       `json:"level"`
	MaxBorrowable types.Number `json:"maxBorrowable"`
	InterestRate  types.Number `json:"interestRate"`
}

// MarginInfoData stores margin info data
type MarginInfoData struct {
	Data []struct {
		MarginRatio string `json:"marginRatio"`
		Base        struct {
			AssetName    string        `json:"assetName"`
			LevelDetails []LevelDetail `json:"levelDetails"`
		} `json:"base"`
		Quote struct {
			AssetName    string        `json:"assetName"`
			LevelDetails []LevelDetail `json:"levelDetails"`
		} `json:"quote"`
	} `json:"data"`
}

// FuturesAccountBalanceData stores account balance data for futures
type FuturesAccountBalanceData struct {
	AccountAlias       string       `json:"accountAlias"`
	Asset              string       `json:"asset"`
	Balance            types.Number `json:"balance"`
	WithdrawAvailable  types.Number `json:"withdrawAvailable"`
	CrossWalletBalance types.Number `json:"crossWalletBalance"`
	CrossUnPNL         types.Number `json:"crossUnPnl"`
	AvailableBalance   types.Number `json:"availableBalance"`
	UpdateTime         types.Time   `json:"updateTime"`
}

// FuturesAccountInformationPosition holds account position data
type FuturesAccountInformationPosition struct {
	Symbol                 string       `json:"symbol"`
	Amount                 types.Number `json:"positionAmt"`
	InitialMargin          types.Number `json:"initialMargin"`
	MaintenanceMargin      types.Number `json:"maintMargin"`
	UnrealizedProfit       types.Number `json:"unrealizedProfit"`
	PositionInitialMargin  types.Number `json:"positionInitialMargin"`
	OpenOrderInitialMargin types.Number `json:"openOrderInitialMargin"`
	Leverage               types.Number `json:"leverage"`
	Isolated               bool         `json:"isolated"`
	PositionSide           string       `json:"positionSide"`
	EntryPrice             types.Number `json:"entryPrice"`
	MaxQuantity            types.Number `json:"maxQty"`
	UpdateTime             types.Time   `json:"updateTime"`
	NotionalValue          types.Number `json:"notionalValue"`
	IsolatedWallet         types.Number `json:"isolatedWallet"`
}

// FuturesAccountInformation stores account information for futures account
type FuturesAccountInformation struct {
	Assets      []FuturesAccountAsset               `json:"assets"`
	Positions   []FuturesAccountInformationPosition `json:"positions"`
	CanDeposit  bool                                `json:"canDeposit"`
	CanTrade    bool                                `json:"canTrade"`
	CanWithdraw bool                                `json:"canWithdraw"`
	FeeTier     int64                               `json:"feeTier"`
	UpdateTime  types.Time                          `json:"updateTime"`
}

// FuturesAccountAsset holds account asset information
type FuturesAccountAsset struct {
	Asset                  currency.Code `json:"asset"`
	WalletBalance          types.Number  `json:"walletBalance"`
	UnrealizedProfit       types.Number  `json:"unrealizedProfit"`
	MarginBalance          types.Number  `json:"marginBalance"`
	MaintenanceMargin      types.Number  `json:"maintMargin"`
	InitialMargin          types.Number  `json:"initialMargin"`
	PositionInitialMargin  types.Number  `json:"positionInitialMargin"`
	OpenOrderInitialMargin types.Number  `json:"openOrderInitialMargin"`
	MaxWithdrawAmount      types.Number  `json:"maxWithdrawAmount"`
	CrossWalletBalance     types.Number  `json:"crossWalletBalance"`
	CrossUnPNL             types.Number  `json:"crossUnPnl"`
	AvailableBalance       types.Number  `json:"availableBalance"`
}

// GenericAuthResponse is a general data response for a post auth request
type GenericAuthResponse struct {
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
}

// FuturesMarginUpdatedResponse stores margin update response data
type FuturesMarginUpdatedResponse struct {
	Amount float64 `json:"amount"`
	Type   uint64  `json:"type"`
	GenericAuthResponse
}

// FuturesLeverageData stores leverage data for futures
type FuturesLeverageData struct {
	Leverage    int64        `json:"leverage"`
	MaxQuantity types.Number `json:"maxQty"`
	Symbol      string       `json:"symbol"`
}

// GetPositionMarginChangeHistoryData gets margin change history for positions
type GetPositionMarginChangeHistoryData struct {
	Amount           types.Number  `json:"amount"`
	Asset            currency.Code `json:"asset"`
	Symbol           string        `json:"symbol"`
	Timestamp        types.Time    `json:"time"`
	MarginChangeType int64         `json:"type"`
	PositionSide     string        `json:"positionSide"`
}

// FuturesPositionInformation stores futures position info
type FuturesPositionInformation struct {
	Symbol           string       `json:"symbol"`
	PositionAmount   types.Number `json:"positionAmt"`
	EntryPrice       types.Number `json:"entryPrice"`
	MarkPrice        types.Number `json:"markPrice"`
	UnRealizedProfit types.Number `json:"unRealizedProfit"`
	LiquidationPrice types.Number `json:"liquidationPrice"`
	Leverage         types.Number `json:"leverage"`
	MaxQuantity      types.Number `json:"maxQty"`
	MarginType       string       `json:"marginType"`
	IsolatedMargin   types.Number `json:"isolatedMargin"`
	IsAutoAddMargin  bool         `json:"isAutoAddMargin,string"`
	PositionSide     string       `json:"positionSide"`
	NotionalValue    types.Number `json:"notionalValue"`
	IsolatedWallet   types.Number `json:"isolatedWallet"`
	UpdateTime       types.Time   `json:"updateTime"`
}

// FuturesAccountTradeList stores account trade list data
type FuturesAccountTradeList struct {
	Symbol          string        `json:"symbol"`
	ID              int64         `json:"id"`
	OrderID         uint64        `json:"orderId"`
	Pair            string        `json:"pair"`
	Side            string        `json:"side"`
	Price           types.Number  `json:"price"`
	Quantity        types.Number  `json:"qty"`
	RealizedPNL     types.Number  `json:"realizedPnl"`
	MarginAsset     currency.Code `json:"marginAsset"`
	BaseQuantity    types.Number  `json:"baseQty"`
	QuoteQuantity   types.Number  `json:"quoteQty"`
	Commission      types.Number  `json:"commission"`
	CommissionAsset currency.Code `json:"commissionAsset"`
	Timestamp       types.Time    `json:"time"`
	PositionSide    string        `json:"positionSide"`
	Buyer           bool          `json:"buyer"`
	Maker           bool          `json:"maker"`
}

// FuturesIncomeHistoryData stores futures income history data
type FuturesIncomeHistoryData struct {
	Symbol     string       `json:"symbol"`
	IncomeType string       `json:"incomeType"`
	Income     types.Number `json:"income"`
	Asset      string       `json:"asset"`
	Info       string       `json:"info"`
	Timestamp  types.Time   `json:"time"`
}

// NotionalBracketData stores notional bracket data
type NotionalBracketData struct {
	Pair     string            `json:"pair"`
	Brackets []NotionalBracket `json:"brackets"`
}

// NotionalBracket is a single leverage bracket
type NotionalBracket struct {
	Bracket          int64   `json:"bracket"`
	InitialLeverage  float64 `json:"initialLeverage"`
	QtyCap           float64 `json:"qtyCap"`
	QtylFloor        float64 `json:"qtylFloor"` // Binance's own spelling, typo included
	MaintMarginRatio float64 `json:"maintMarginRatio"`
	Cumulative       float64 `json:"cum"`
}

// ForcedOrdersData stores forced orders data
type ForcedOrdersData struct {
	OrderID          uint64       `json:"orderId"`
	Symbol           string       `json:"symbol"`
	Pair             string       `json:"pair"`
	Status           string       `json:"status"`
	ClientOrderID    string       `json:"clientOrderId"`
	Price            types.Number `json:"price"`
	AveragePrice     types.Number `json:"avgPrice"`
	OriginalQuantity types.Number `json:"origQty"`
	ExecutedQuantity types.Number `json:"executedQty"`
	CumulativeBase   types.Number `json:"cumBase"`
	CumulativeQuote  types.Number `json:"cumQuote"`
	TimeInForce      string       `json:"timeInForce"`
	OrderType        string       `json:"type"`
	ReduceOnly       bool         `json:"reduceOnly"`
	ClosePosition    bool         `json:"closePosition"`
	Side             string       `json:"side"`
	PositionSide     string       `json:"positionSide"`
	StopPrice        types.Number `json:"stopPrice"`
	WorkingType      string       `json:"workingType"`
	PriceProtect     bool         `json:"priceProtect"`
	OriginalType     string       `json:"origType"`
	Time             types.Time   `json:"time"`
	UpdateTime       types.Time   `json:"updateTime"`
	GoodTillDate     types.Time   `json:"goodTillDate"`
}

// ADLEstimateData stores data for ADL estimates
type ADLEstimateData struct {
	Symbol      string `json:"symbol"`
	ADLQuantile struct {
		Long  float64 `json:"LONG"`
		Short float64 `json:"SHORT"`
		Hedge float64 `json:"HEDGE"`
	} `json:"adlQuantile"`
}

// InterestHistoryData gets interest history data
type InterestHistoryData struct {
	Asset       string     `json:"asset"`
	Interest    float64    `json:"interest"`
	LendingType string     `json:"lendingType"`
	ProductName string     `json:"productName"`
	Time        types.Time `json:"time"`
}

// FundingRateData stores funding rates data
type FundingRateData struct {
	Symbol      string       `json:"symbol"`
	FundingRate types.Number `json:"fundingRate"`
	FundingTime types.Time   `json:"fundingTime"`
}

// SymbolsData stores perp futures' symbols
type SymbolsData struct {
	Symbol string `json:"symbol"`
}

// PerpsExchangeInfo stores data for perps
type PerpsExchangeInfo struct {
	Symbols []SymbolsData `json:"symbols"`
}

// UFuturesExchangeInfo stores exchange info for ufutures
type UFuturesExchangeInfo struct {
	RateLimits []struct {
		Interval      string `json:"interval"`
		IntervalNum   int64  `json:"intervalNum"`
		Limit         int64  `json:"limit"`
		RateLimitType string `json:"rateLimitType"`
	} `json:"rateLimits"`
	ServerTime types.Time           `json:"serverTime"`
	Symbols    []UFuturesSymbolInfo `json:"symbols"`
	Timezone   string               `json:"timezone"`
}

// UFuturesSymbolInfo contains details of a currency symbol
// for a usdt margined future contract
type UFuturesSymbolInfo struct {
	Symbol                   string                  `json:"symbol"`
	Pair                     string                  `json:"pair"`
	ContractType             string                  `json:"contractType"`
	DeliveryDate             types.Time              `json:"deliveryDate"`
	OnboardDate              types.Time              `json:"onboardDate"`
	Status                   string                  `json:"status"`
	MaintenanceMarginPercent types.Number            `json:"maintMarginPercent"`
	RequiredMarginPercent    types.Number            `json:"requiredMarginPercent"`
	BaseAsset                string                  `json:"baseAsset"`
	QuoteAsset               string                  `json:"quoteAsset"`
	MarginAsset              string                  `json:"marginAsset"`
	PricePrecision           int64                   `json:"pricePrecision"`
	QuantityPrecision        int64                   `json:"quantityPrecision"`
	BaseAssetPrecision       int64                   `json:"baseAssetPrecision"`
	QuotePrecision           int64                   `json:"quotePrecision"`
	UnderlyingType           string                  `json:"underlyingType"`
	UnderlyingSubType        []string                `json:"underlyingSubType"`
	SettlePlan               float64                 `json:"settlePlan"`
	TriggerProtect           types.Number            `json:"triggerProtect"`
	Filters                  []*OrderExecutionLimits `json:"filters"`
	OrderTypes               []string                `json:"orderTypes"`
	TimeInForce              []string                `json:"timeInForce"`
	LiquidationFee           types.Number            `json:"liquidationFee"`
	MarketTakeBound          types.Number            `json:"marketTakeBound"`
}

// OrderExecutionLimits represents an order execution limits
type OrderExecutionLimits struct {
	FilterType        string       `json:"filterType"`
	MinPrice          types.Number `json:"minPrice"`
	MaxPrice          types.Number `json:"maxPrice"`
	TickSize          types.Number `json:"tickSize"`
	StepSize          types.Number `json:"stepSize"`
	MaxQuantity       types.Number `json:"maxQty"`
	MinQuantity       types.Number `json:"minQty"`
	Limit             int64        `json:"limit"`
	MultiplierDown    types.Number `json:"multiplierDown"`
	MultiplierUp      types.Number `json:"multiplierUp"`
	MultiplierDecimal types.Number `json:"multiplierDecimal"`
	Notional          types.Number `json:"notional"`
}

// CExchangeInfo stores exchange info for cfutures
type CExchangeInfo struct {
	ExchangeFilters []any `json:"exchangeFilters"`
	RateLimits      []struct {
		Interval      string `json:"interval"`
		IntervalNum   int64  `json:"intervalNum"`
		Limit         int64  `json:"limit"`
		RateLimitType string `json:"rateLimitType"`
	} `json:"rateLimits"`
	ServerTime types.Time `json:"serverTime"`
	Symbols    []struct {
		Filters               []*OrderExecutionLimits `json:"filters"`
		OrderTypes            []string                `json:"orderTypes"`
		TimeInForce           []string                `json:"timeInForce"`
		Symbol                string                  `json:"symbol"`
		Pair                  string                  `json:"pair"`
		ContractType          string                  `json:"contractType"`
		DeliveryDate          types.Time              `json:"deliveryDate"`
		OnboardDate           types.Time              `json:"onboardDate"`
		ContractStatus        string                  `json:"contractStatus"`
		ContractSize          int64                   `json:"contractSize"`
		QuoteAsset            string                  `json:"quoteAsset"`
		BaseAsset             string                  `json:"baseAsset"`
		MarginAsset           string                  `json:"marginAsset"`
		PricePrecision        int64                   `json:"pricePrecision"`
		QuantityPrecision     int64                   `json:"quantityPrecision"`
		BaseAssetPrecision    int64                   `json:"baseAssetPrecision"`
		QuotePrecision        int64                   `json:"quotePrecision"`
		MaintMarginPercent    types.Number            `json:"maintMarginPercent"`
		RequiredMarginPercent types.Number            `json:"requiredMarginPercent"`
	} `json:"symbols"`
	Timezone string `json:"timezone"`
}

// CFutureAggregateTrade represents a coin margined future push data instance.
type CFutureAggregateTrade struct {
	EventType        string       `json:"e"`
	EventTime        types.Time   `json:"E"`
	AggregateTradeID uint64       `json:"a"`
	Symbol           string       `json:"s"`
	Price            types.Number `json:"p"`
	Quantity         types.Number `json:"q"`
	FirstTradeID     uint64       `json:"f"`
	LastTradeID      uint64       `json:"l"`
	TradeTime        types.Time   `json:"T"`
	IsMarketMaker    bool         `json:"m"`
}

// CFuturesMarketLiquidiation represents liquidation order snapshot
type CFuturesMarketLiquidiation struct {
	EventType string     `json:"e"`
	EventTime types.Time `json:"E"`
	Order     struct {
		Symbol                         string       `json:"s"`
		Pair                           string       `json:"ps"`
		Side                           string       `json:"S"`
		OrderType                      string       `json:"o"`
		TimeInForce                    string       `json:"f"`
		OriginalQuantity               types.Number `json:"q"`
		Price                          types.Number `json:"p"`
		AveragePrice                   types.Number `json:"ap"`
		OrderStatus                    string       `json:"X"`
		LastFilledQuantity             types.Number `json:"l"`
		OrderFilledAccumulatedQuantity types.Number `json:"z"`
		OrderTradeTime                 types.Time   `json:"T"`
	} `json:"o"`
}

// CFutureMarkOrIndexPriceKline represents mark/index price kline data.
type CFutureMarkOrIndexPriceKline struct {
	EventType string     `json:"e"`
	EventTime types.Time `json:"E"`
	Pair      string     `json:"ps"`
	Kline     struct {
		StartTime         types.Time   `json:"t"`
		CloseTime         types.Time   `json:"T"`
		S                 string       `json:"s"` // Symbol for Mark Price, Ignored for Index Price
		Interval          string       `json:"i"`
		F                 int64        `json:"f"`
		L                 int64        `json:"L"`
		OpenPrice         types.Number `json:"o"`
		ClosePrice        types.Number `json:"c"`
		HighPrice         types.Number `json:"h"`
		LowPrice          types.Number `json:"l"`
		V                 string       `json:"v"`
		NumberOfBasicData int64        `json:"n"`
		IsKlineClosed     bool         `json:"x"`
		Q                 string       `json:"q"`
		V0                string       `json:"V"`
		Q0                string       `json:"Q"`
		B                 string       `json:"B"`
	} `json:"k"`
}

// CFutureIndexPriceStream represents an index price stream data.
type CFutureIndexPriceStream struct {
	EventType  string       `json:"e"`
	EventTime  types.Time   `json:"E"`
	Pair       string       `json:"i"`
	IndexPrice types.Number `json:"p"`
}

// CFutureKlineData represents a
type CFutureKlineData struct {
	EventType string     `json:"e"`
	EventTime types.Time `json:"E"`
	Symbol    string     `json:"s"`
	KlineData struct {
		StartTime               types.Time   `json:"t"`
		CloseTime               types.Time   `json:"T"`
		Symbol                  string       `json:"s"`
		Interval                string       `json:"i"`
		FirstTradeID            uint64       `json:"f"`
		LastTradeID             uint64       `json:"L"`
		OpenPrice               types.Number `json:"o"`
		ClosePrice              types.Number `json:"c"`
		HighPrice               types.Number `json:"h"`
		LowPrice                types.Number `json:"l"`
		Volume                  types.Number `json:"v"`
		NumberOfTrades          int64        `json:"n"`
		IsKlineClose            bool         `json:"x"`
		BaseAssetVolume         types.Number `json:"q"`
		TakerBuyVolume          types.Number `json:"V"`
		TakerBuyBaseAssetVolume types.Number `json:"Q"`
		B                       string       `json:"B"`
	} `json:"k"`
}

// CFuturesMarketTicker 24hr rolling window ticker statistics for all symbols
// CFuturesMarketTicker redeclares v and q because COIN-M reports a contract count in
// "v" and the base asset volume in "q", whereas the embedded USD-M shape names those
// same keys TotalTradeBaseVolume and TotalQuoteAssetVolume. The redeclared fields
// shadow the embedded pair, so the embedded TotalTradeBaseVolume and
// TotalQuoteAssetVolume never populate and must not be read from this type.
type CFuturesMarketTicker struct {
	UFutureMarketTicker
	Pair                       string       `json:"ps"`
	TotalTradedVolume          types.Number `json:"v"`
	TotalTradedBaseAssetVolume types.Number `json:"q"`
}

// CFuturesIndexPriceConstituents represents a list of index price constituents
type CFuturesIndexPriceConstituents struct {
	Symbol       string     `json:"symbol"`
	Time         types.Time `json:"time"`
	Constituents []struct {
		Exchange string `json:"exchange"`
		Symbol   string `json:"symbol"`
	} `json:"constituents"`
}

// CFuturesOpenInterest is the open interest for a COIN-M contract.
type CFuturesOpenInterest struct {
	Symbol       string       `json:"symbol"`
	Pair         string       `json:"pair"`
	OpenInterest types.Number `json:"openInterest"`
	ContractType string       `json:"contractType"`
	Time         types.Time   `json:"time"`
}

// CFuturesCommissionRate holds the maker and taker commission rates for a symbol.
type CFuturesCommissionRate struct {
	Symbol              string       `json:"symbol"`
	MakerCommissionRate types.Number `json:"makerCommissionRate"`
	TakerCommissionRate types.Number `json:"takerCommissionRate"`
}

// CFuturesPositionSideDual reports whether hedge (dual side) position mode is enabled.
type CFuturesPositionSideDual struct {
	DualSidePosition bool `json:"dualSidePosition"`
}

// CFuturesLeverageBracketV2 is the V2 leverage bracket response, which adds notionalCoef
// over V1.
type CFuturesLeverageBracketV2 struct {
	Symbol       string       `json:"symbol"`
	NotionalCoef types.Number `json:"notionalCoef"`
	Brackets     []struct {
		Bracket          int64        `json:"bracket"`
		InitialLeverage  types.Number `json:"initialLeverage"`
		QuantityCap      types.Number `json:"qtyCap"`
		QuantityFloor    types.Number `json:"qtyFloor"`
		MaintMarginRatio types.Number `json:"maintMarginRatio"`
		Cumulative       types.Number `json:"cum"`
	} `json:"brackets"`
}

// GetFuturesAggregatedTradesListRequest holds the parameters for GetFuturesAggregatedTradesList.
type GetFuturesAggregatedTradesListRequest struct {
	Symbol    currency.Pair
	FromID    uint64
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetFuturesKlineDataRequest holds the parameters for GetFuturesKlineData.
type GetFuturesKlineDataRequest struct {
	Symbol    currency.Pair
	Interval  string
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetContinuousKlineDataRequest holds the parameters for GetContinuousKlineData.
type GetContinuousKlineDataRequest struct {
	Pair         string
	ContractType string
	Interval     string
	Limit        uint64
	StartTime    time.Time
	EndTime      time.Time
}

// GetIndexPriceKlinesRequest holds the parameters for GetIndexPriceKlines.
type GetIndexPriceKlinesRequest struct {
	Pair      string
	Interval  string
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetMarkPriceKlineRequest holds the parameters for GetMarkPriceKline.
type GetMarkPriceKlineRequest struct {
	Symbol    currency.Pair
	Interval  string
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetPremiumIndexKlineDataRequest holds the parameters for GetPremiumIndexKlineData.
type GetPremiumIndexKlineDataRequest struct {
	Symbol    currency.Pair
	Interval  string
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetOpenInterestStatsRequest holds the parameters for GetOpenInterestStats.
type GetOpenInterestStatsRequest struct {
	Pair         string
	ContractType string
	Period       string
	Limit        uint64
	StartTime    time.Time
	EndTime      time.Time
}

// GetTraderFuturesAccountRatioRequest holds the parameters for GetTraderFuturesAccountRatio.
type GetTraderFuturesAccountRatioRequest struct {
	Pair      currency.Pair
	Period    string
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetTraderFuturesPositionsRatioRequest holds the parameters for GetTraderFuturesPositionsRatio.
type GetTraderFuturesPositionsRatioRequest struct {
	Pair      currency.Pair
	Period    string
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetMarketRatioRequest holds the parameters for GetMarketRatio.
type GetMarketRatioRequest struct {
	Pair      currency.Pair
	Period    string
	Limit     uint64
	StartTime time.Time
	EndTime   time.Time
}

// GetFuturesTakerVolumeRequest holds the parameters for GetFuturesTakerVolume.
type GetFuturesTakerVolumeRequest struct {
	Pair         currency.Pair
	ContractType string
	Period       string
	Limit        uint64
	StartTime    time.Time
	EndTime      time.Time
}

// GetFuturesBasisDataRequest holds the parameters for GetFuturesBasisData.
type GetFuturesBasisDataRequest struct {
	Pair         currency.Pair
	ContractType string
	Period       string
	Limit        uint64
	StartTime    time.Time
	EndTime      time.Time
}

// GetAllFuturesOrdersRequest holds the parameters for GetAllFuturesOrders.
type GetAllFuturesOrdersRequest struct {
	Symbol    currency.Pair
	Pair      currency.Pair
	StartTime time.Time
	EndTime   time.Time
	OrderID   uint64
	Limit     uint64
}

// FuturesMarginChangeHistoryRequest holds the parameters for FuturesMarginChangeHistory.
type FuturesMarginChangeHistoryRequest struct {
	Symbol     currency.Pair
	ChangeType string
	StartTime  time.Time
	EndTime    time.Time
	Limit      int64
}

// FuturesTradeHistoryRequest holds the parameters for FuturesTradeHistory.
type FuturesTradeHistoryRequest struct {
	Symbol    currency.Pair
	Pair      string
	StartTime time.Time
	EndTime   time.Time
	Limit     int64
	FromID    int64
}

// FuturesIncomeHistoryRequest holds the parameters for FuturesIncomeHistory.
type FuturesIncomeHistoryRequest struct {
	Symbol     currency.Pair
	IncomeType string
	StartTime  time.Time
	EndTime    time.Time
	Limit      int64
}

// GetCFuturesOrderModifyHistoryRequest holds the parameters for GetCFuturesOrderModifyHistory.
type GetCFuturesOrderModifyHistoryRequest struct {
	Symbol            currency.Pair
	OrderID           uint64
	OrigClientOrderID string
	StartTime         time.Time
	EndTime           time.Time
	Limit             int64
}
