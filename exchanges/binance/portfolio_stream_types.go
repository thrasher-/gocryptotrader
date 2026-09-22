package binance

import (
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// PortfolioMarginBalanceUpdate includes the portfolio account's update sequence.
type PortfolioMarginBalanceUpdate struct {
	UserDataEvent
	Asset        currency.Code `json:"a"`
	BalanceDelta types.Number  `json:"d"`
	UpdateID     uint64        `json:"U"`
}

// PortfolioMarginAccountPosition reports changed margin balances.
type PortfolioMarginAccountPosition struct {
	WsAccountPositionData
	UpdateID uint64 `json:"U"`
}

// PortfolioMarginLiabilityChange identifies a borrow or repayment by transaction ID.
type PortfolioMarginLiabilityChange struct {
	EventType      string        `json:"e"`
	EventTime      types.Time    `json:"E"`
	Asset          currency.Code `json:"a"`
	Type           string        `json:"t"`
	TransactionID  uint64        `json:"T"`
	Principal      types.Number  `json:"p"`
	Interest       types.Number  `json:"i"`
	TotalLiability types.Number  `json:"l"`
}

// PortfolioMarginOpenOrderLoss reports potential losses from margin open orders.
type PortfolioMarginOpenOrderLoss struct {
	UserDataEvent
	Losses []PortfolioMarginAssetLoss `json:"O"`
}

// PortfolioMarginAssetLoss is the potential open-order loss for one asset.
type PortfolioMarginAssetLoss struct {
	Asset  currency.Code `json:"a"`
	Amount types.Number  `json:"o"`
}

// PortfolioMarginRiskLevelChange reports the portfolio's unified margin risk.
type PortfolioMarginRiskLevelChange struct {
	UserDataEvent
	UnifiedMaintenanceMarginRatio types.Number `json:"u"`
	RiskLevel                     string       `json:"s"`
	AccountEquity                 types.Number `json:"eq"`
	ActualEquity                  types.Number `json:"ae"`
	MaintenanceMargin             types.Number `json:"m"`
}

// PortfolioMarginAlgoUpdate uses ao for its order, unlike the regular UM stream.
type PortfolioMarginAlgoUpdate struct {
	UserDataEvent
	BusinessUnit string                  `json:"fs"`
	Order        *FuturesAlgoUpdateOrder `json:"ao"`
}

// PortfolioMarginConditionalOrderUpdate retains the deprecated conditional event
// for clients processing older stored streams; new UM orders use ALGO_UPDATE.
type PortfolioMarginConditionalOrderUpdate struct {
	UserDataEvent
	BusinessUnit string                           `json:"fs"`
	Order        *PortfolioMarginConditionalOrder `json:"so"`
}

// PortfolioMarginConditionalOrder describes the strategy behind a conditional order.
type PortfolioMarginConditionalOrder struct {
	Symbol                  string       `json:"s"`
	ClientOrderID           string       `json:"c"`
	StrategyID              uint64       `json:"si"`
	Side                    string       `json:"S"`
	StrategyType            string       `json:"st"`
	TimeInForce             string       `json:"f"`
	Quantity                types.Number `json:"q"`
	Price                   types.Number `json:"p"`
	StopPrice               types.Number `json:"sp"`
	Status                  string       `json:"os"`
	OrderTime               types.Time   `json:"T"`
	UpdateTime              types.Time   `json:"ut"`
	ReduceOnly              bool         `json:"R"`
	WorkingType             string       `json:"wt"`
	PositionSide            string       `json:"ps"`
	CloseAll                bool         `json:"cp"`
	ActivationPrice         types.Number `json:"AP"`
	CallbackRate            types.Number `json:"cr"`
	OrderID                 uint64       `json:"i"`
	SelfTradePreventionMode string       `json:"V"`
	GoodTillDate            types.Time   `json:"gtd"`
}

// PortfolioMarginProAccountUpdate reports the classic portfolio's account totals.
type PortfolioMarginProAccountUpdate struct {
	UserDataEvent
	UnifiedMaintenanceMarginRatio types.Number `json:"u"`
	AccountEquity                 types.Number `json:"eq"`
	ActualEquity                  types.Number `json:"ae"`
	InitialMargin                 types.Number `json:"im"`
	MaintenanceMargin             types.Number `json:"mm"`
	AvailableBalance              types.Number `json:"avb"`
	VirtualMaxWithdrawAmount      types.Number `json:"vmw"`
}
