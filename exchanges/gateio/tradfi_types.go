package gateio

import (
	"fmt"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var klineIntervalToTypeMap = []struct {
	Interval       kline.Interval
	IntervalString string
}{
	{Interval: kline.OneMin, IntervalString: "1m"},
	{Interval: kline.FifteenMin, IntervalString: "15m"},
	{Interval: kline.OneHour, IntervalString: "1h"},
	{Interval: kline.FourHour, IntervalString: "4h"},
	{Interval: kline.OneDay, IntervalString: "1d"},
	{Interval: kline.OneWeek, IntervalString: "7d"},
	{Interval: kline.OneMonth, IntervalString: "30d"},
}

func klineIntervalToTypeString(interval kline.Interval) (string, error) {
	for _, result := range klineIntervalToTypeMap {
		if result.Interval == interval {
			return result.IntervalString, nil
		}
	}
	return "", fmt.Errorf("%w: %v", kline.ErrUnsupportedInterval, interval)
}

// TradFiMT5Account holds MT5 account information.
type TradFiMT5Account struct {
	MT5UID       uint64 `json:"mt5_uid"`
	Leverage     uint64 `json:"leverage"`
	StopOutLevel string `json:"stop_out_level"`
	Status       uint64 `json:"status"` // Status: 1=not opened, 2=pending review, 3=active.
}

// TradFiCategory holds a single symbol category.
type TradFiCategory struct {
	CategoryID   uint64 `json:"category_id"`
	IsFavorite   bool   `json:"is_favorite"`
	CategoryName string `json:"category_name"`
}

// TradFiCategoryList wraps the category list data field.
type TradFiCategoryList struct {
	List []*TradFiCategory `json:"list"`
}

// TradFiSymbolCommission holds the commission charged per lot for a TradFi symbol.
type TradFiSymbolCommission struct {
	CategoryCode string       `json:"category_code"`
	Symbol       string       `json:"symbol"`
	FeePerLot    types.Number `json:"fee_per_lot"`
}

// TradFiSymbolCommissionList wraps the symbol commission list data field.
type TradFiSymbolCommissionList struct {
	List []*TradFiSymbolCommission `json:"list"`
}

// TradFiSymbol holds trading symbol information.
type TradFiSymbol struct {
	Symbol                   string        `json:"symbol"`
	SymbolDescription        string        `json:"symbol_desc"`
	CategoryID               uint64        `json:"category_id"`
	Status                   string        `json:"status"`     // Status: open=tradable, closed=non-tradable.
	TradeMode                string        `json:"trade_mode"` // TradeMode: 0=disabled, 1=long only, 2=short only, 3=close only, 4=full trading access.
	IconLink                 string        `json:"icon_link"`
	CloseTime                types.Time    `json:"close_time"`
	OpenTime                 types.Time    `json:"open_time"`
	NextOpenTime             types.Time    `json:"next_open_time"`
	SettlementCurrency       currency.Code `json:"settlement_currency"`
	SettlementCurrencySymbol string        `json:"settlement_currency_symbol"`
	PricePrecision           uint64        `json:"price_precision"`
}

// TradFiSymbolList wraps the symbol list data field.
type TradFiSymbolList struct {
	List []*TradFiSymbol `json:"list"`
}

// TradFiSymbolDetail holds detailed contract information for a trading symbol.
type TradFiSymbolDetail struct {
	Symbol             string        `json:"symbol"`
	SymbolDescription  string        `json:"symbol_desc"`
	CategoryName       string        `json:"category_name"`
	ContractVolume     types.Number  `json:"contract_volume"`
	SettlementCurrency currency.Code `json:"settlement_currency"`
	MaxOrderVolume     types.Number  `json:"max_order_volume"`
	MinOrderVolume     types.Number  `json:"min_order_volume"`
	Leverage           types.Number  `json:"leverage"`
	PricePrecision     uint64        `json:"price_precision"`
	PriceStopLossLevel types.Number  `json:"price_sl_level"`
	SwapCostType       string        `json:"swap_cost_type"`
	BuySwapCostRate    types.Number  `json:"buy_swap_cost_rate"`
	SellSwapCostRate   types.Number  `json:"sell_swap_cost_rate"`
	SwapCost3Day       string        `json:"swap_cost_3day"`
	TradeTimezone      string        `json:"trade_timezone"`
	TradeMode          string        `json:"trade_mode"` // TradeMode: 0=disabled, 1=long only, 2=short only, 3=close only, 4=full trading access.
	IconLink           string        `json:"icon_link"`
}

// TradFiSymbolDetailList wraps the symbol detail list data field.
type TradFiSymbolDetailList struct {
	List []*TradFiSymbolDetail `json:"list"`
}

// TradFiKline holds a single candlestick data point.
type TradFiKline struct {
	Open      types.Number `json:"o"`
	Close     types.Number `json:"c"`
	Low       types.Number `json:"l"`
	High      types.Number `json:"h"`
	Timestamp types.Time   `json:"t"`
}

// TradFiKlineList wraps the kline list data field.
type TradFiKlineList struct {
	List []*TradFiKline `json:"list"`
}

// TradFiTicker holds ticker information for a trading symbol.
type TradFiTicker struct {
	HighestPrice        types.Number `json:"highest_price"`
	LowestPrice         types.Number `json:"lowest_price"`
	PriceChange         string       `json:"price_change"`
	PriceChangeAmount   types.Number `json:"price_change_amount"`
	TodayOpenPrice      types.Number `json:"today_open_price"`
	LastTodayClosePrice types.Number `json:"last_today_close_price"`
	LastPrice           types.Number `json:"last_price"`
	BidPrice            types.Number `json:"bid_price"`
	AskPrice            types.Number `json:"ask_price"`
	Favorite            bool         `json:"favorite"`
	Status              string       `json:"status"` // Status: open=tradable, closed=non-tradable.
	CloseTime           types.Time   `json:"close_time"`
	OpenTime            types.Time   `json:"open_time"`
	NextOpenTime        types.Time   `json:"next_open_time"`
	TradeMode           string       `json:"trade_mode"`
	CategoryName        string       `json:"category_name"`
}

// TradFiUserInfo holds TradFi user account information returned after activation.
type TradFiUserInfo struct {
	Status   uint64 `json:"status"` // Status: 1=not opened, 2=pending review, 3=opened.
	Leverage uint64 `json:"leverage"`
	MT5UID   string `json:"mt5_uid"`
}

// TradFiUserAssets holds TradFi account asset information.
type TradFiUserAssets struct {
	Equity        string       `json:"equity"`
	MarginLevel   string       `json:"margin_level"`
	Balance       types.Number `json:"balance"`
	Margin        types.Number `json:"margin"`
	MarginFree    types.Number `json:"margin_free"`
	UnrealizedPNL types.Number `json:"unrealized_pnl"`
	MT5UID        string       `json:"mt5_uid"`
}

// TradFiTransaction holds a single fund transfer transaction record.
type TradFiTransaction struct {
	Asset           currency.Code `json:"asset"`
	Type            string        `json:"type"` // Type: deposit=transfer in, withdraw=transfer out, dividend=dividend payment, fill_negative=cover negative balance.
	TypeDescription string        `json:"type_desc"`
	Change          types.Number  `json:"change"`
	Balance         types.Number  `json:"balance"`
	Time            types.Time    `json:"time"`
}

// TradFiTransactionListData wraps the transaction list data with pagination.
type TradFiTransactionListData struct {
	Total     uint64               `json:"total"`
	TotalPage uint64               `json:"total_page"`
	List      []*TradFiTransaction `json:"list"`
	Timestamp types.Time           `json:"timestamp"`
}

// TradFiTransactionRequest is the request body for fund deposit or withdrawal.
type TradFiTransactionRequest struct {
	Asset  currency.Code `json:"asset"`
	Change types.Number  `json:"change"`
	Type   string        `json:"type"`
}

// TradFiOrder holds an active pending order.
type TradFiOrder struct {
	OrderID           uint64       `json:"order_id"`
	Symbol            string       `json:"symbol"`
	SymbolDescription string       `json:"symbol_desc"`
	PriceType         string       `json:"price_type"` // PriceType: market=market price, trigger=trigger price.
	State             uint64       `json:"state"`
	StateDescription  string       `json:"state_desc"`
	Finished          uint64       `json:"finished"` // Finished: 0=shown in active order list, 1=not shown.
	Side              uint64       `json:"side"`     // Side: 1=sell, 2=buy.
	Volume            types.Number `json:"volume"`
	Price             types.Number `json:"price"`
	PriceTakeProfit   types.Number `json:"price_tp"`
	PriceStopLoss     types.Number `json:"price_sl"`
	TimeSetup         types.Time   `json:"time_setup"`
}

// TradFiOrderList wraps the active order list data field.
type TradFiOrderList struct {
	List []*TradFiOrder `json:"list"`
}

// TradFiOrderRequest is the request body for creating an order.
type TradFiOrderRequest struct {
	Price           types.Number  `json:"price"`
	PriceType       string        `json:"price_type"`
	Side            uint64        `json:"side"` // Side: 1=sell, 2=buy.
	Symbol          currency.Pair `json:"symbol"`
	Volume          types.Number  `json:"volume"`
	PriceTakeProfit types.Number  `json:"price_tp,omitempty"`
	PriceStopLoss   types.Number  `json:"price_sl,omitempty"`
}

// TradFiCreateOrderResult holds the queue task ID returned after order creation.
type TradFiCreateOrderResult struct {
	ID string `json:"id"`
}

// TradFiOrderUpdateRequest is the request body for modifying an existing order.
type TradFiOrderUpdateRequest struct {
	Price           string       `json:"price"`
	PriceTakeProfit types.Number `json:"price_tp,omitempty"`
	PriceStopLoss   types.Number `json:"price_sl,omitempty"`
}

// TradFiUpdatedOrder holds the order state after modification.
type TradFiUpdatedOrder struct {
	OrderID         uint64       `json:"order_id"`
	Symbol          string       `json:"symbol"`
	State           string       `json:"state"`
	Volume          types.Number `json:"volume"`
	Price           types.Number `json:"price"`
	PriceTakeProfit types.Number `json:"price_tp"`
	PriceStopLoss   types.Number `json:"price_sl"`
}

// TradFiHistoricalOrder holds a completed order record.
type TradFiHistoricalOrder struct {
	OrderID           uint64       `json:"order_id"`
	Symbol            string       `json:"symbol"`
	SymbolDescription string       `json:"symbol_desc"`
	PriceType         string       `json:"price_type"`     // PriceType: market=market price, trigger=trigger price.
	OrderOptType      uint64       `json:"order_opt_type"` // OrderOptType: 1=sell, 2=buy, 3=close long, 4=close short, 5=force close long, 6=force close short.
	State             uint64       `json:"state"`
	StateDescription  string       `json:"state_desc"`
	Side              uint64       `json:"side"` // Side: 1=sell, 2=buy.
	Volume            types.Number `json:"volume"`
	FillVolume        types.Number `json:"fill_volume"`
	ClosePNL          types.Number `json:"close_pnl"`
	Price             types.Number `json:"price"`
	TriggerPrice      types.Number `json:"trigger_price"`
	PriceTakeProfit   types.Number `json:"price_tp"`
	PriceStopLoss     types.Number `json:"price_sl"`
	TimeSetup         types.Time   `json:"time_setup"`
	TimeDone          types.Time   `json:"time_done"`
}

// TradFiOrderHistoryList wraps the historical order list data field.
type TradFiOrderHistoryList struct {
	List []*TradFiHistoricalOrder `json:"list"`
}

// TradFiOrderLog holds order details returned for an order placement log ID.
type TradFiOrderLog struct {
	OrderID   uint64       `json:"order_id"`
	LogID     uint64       `json:"log_id"`
	Symbol    string       `json:"symbol"`
	PriceType string       `json:"price_type"` // PriceType: market=market price, trigger=trigger price.
	State     uint64       `json:"state"`      // State: 1=placed, 2=canceled, 3=partially filled, 4=filled, 5=rejected.
	Side      uint64       `json:"side"`       // Side: 1=sell, 2=buy.
	Volume    types.Number `json:"volume"`
	Price     types.Number `json:"price"`
}

// TradFiPosition holds an active open position.
type TradFiPosition struct {
	PositionID        uint64       `json:"position_id"`
	Symbol            string       `json:"symbol"`
	SymbolDescription string       `json:"symbol_desc"`
	Margin            string       `json:"margin"`
	UnrealizedPNL     types.Number `json:"unrealized_pnl"`
	UnrealizedPNLRate types.Number `json:"unrealized_pnl_rate"`
	Volume            types.Number `json:"volume"`
	PriceOpen         types.Number `json:"price_open"`
	PositionDir       string       `json:"position_dir"` // PositionDir: Long=long position, Short=short position.
}

// TradFiPositionList wraps the active position list data field.
type TradFiPositionList struct {
	List []*TradFiPosition `json:"list"`
}

// TradFiPositionUpdateRequest is the request body for modifying a position's TP/SL.
type TradFiPositionUpdateRequest struct {
	PriceTakeProfit types.Number `json:"price_tp,omitempty"`
	PriceStopLoss   types.Number `json:"price_sl,omitempty"`
}

// TradFiClosePositionRequest is the request body for closing a position.
type TradFiClosePositionRequest struct {
	CloseType   uint64       `json:"close_type"` // CloseType: 1=partial close, 2=full close.
	CloseVolume types.Number `json:"close_volume,omitempty"`
}

// TradFiLiquidationDetail holds margin details recorded at the time of liquidation.
type TradFiLiquidationDetail struct {
	MarginLevel  string `json:"margin_level"`
	Margin       string `json:"margin"`
	Equity       string `json:"equity"`
	StopOutLevel string `json:"stop_out_level"`
}

// TradFiRealizedPNLDetail holds a breakdown of realized profit and loss.
type TradFiRealizedPNLDetail struct {
	ClosedPNL string       `json:"closed_pnl"`
	Swap      string       `json:"swap"`
	Fee       types.Number `json:"fee"`
}

// TradFiHistoricalPosition holds a closed position record.
type TradFiHistoricalPosition struct {
	PositionID        uint64                   `json:"position_id"`
	Symbol            string                   `json:"symbol"`
	RealizedPNL       types.Number             `json:"realized_pnl"`
	RealizedPNLRate   types.Number             `json:"realized_pnl_rate"`
	Volume            types.Number             `json:"volume"`
	VolumeClosed      types.Number             `json:"volume_closed"`
	PriceOpen         types.Number             `json:"price_open"`
	PositionDir       string                   `json:"position_dir"` // PositionDir: Long=long position, Short=short position.
	PriceTakeProfit   types.Number             `json:"price_tp"`
	PriceStopLoss     types.Number             `json:"price_sl"`
	CounterpartyPrice types.Number             `json:"counterparty_price"`
	ClosePrice        types.Number             `json:"close_price"`
	TimeCreate        types.Time               `json:"time_create"`
	TimeClose         types.Time               `json:"time_close"`
	PositionStatus    string                   `json:"position_status"` // PositionStatus: 1=fully closed, 2=forced liquidation.
	CloseDetail       *TradFiLiquidationDetail `json:"close_detail"`
	RealizedPNLDetail TradFiRealizedPNLDetail  `json:"realized_pnl_detail"`
}

// TradFiHistoricalPositionListData wraps historical position data with pagination.
type TradFiHistoricalPositionListData struct {
	Total     uint64                      `json:"total"`
	TotalPage uint64                      `json:"total_page"`
	List      []*TradFiHistoricalPosition `json:"list"`
}

// GetTradFiKlinesRequest holds the query parameters for the klines endpoint.
type GetTradFiKlinesRequest struct {
	KlineType string // KlineType is required: TradFiKlineType1m, 15m, 1h, 4h, 1d, 7d, or 30d.
	BeginTime time.Time
	EndTime   time.Time
	Limit     uint64
}

// GetTradFiSymbolCommissionsRequest holds filters for querying symbol commission rates.
type GetTradFiSymbolCommissionsRequest struct {
	Symbols       []string
	CategoryCodes []string
}

// GetTradFiTransactionsRequest holds the query parameters for listing transactions.
type GetTradFiTransactionsRequest struct {
	BeginTime time.Time
	EndTime   time.Time
	Type      string // Type filters by transaction type; one of TradFiTransaction* constants or empty for all.
	Page      uint64
	PageSize  uint64
}

// GetTradFiOrderHistoryRequest holds the query parameters for historical order list.
type GetTradFiOrderHistoryRequest struct {
	BeginTime time.Time
	EndTime   time.Time
	Symbol    currency.Pair
	Side      uint64 // Side filters by order side: 1=sell, 2=buy; 0 means no filter.
}

// GetTradFiPositionHistoryRequest holds the query parameters for historical position list.
type GetTradFiPositionHistoryRequest struct {
	Page        uint64
	PageSize    uint64
	BeginTime   time.Time
	EndTime     time.Time
	Symbol      currency.Pair
	PositionDir string // PositionDir filters by direction: TradFiPositionLong, TradFiPositionShort, or empty for all.
}
