package binance

import (
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// UserDataEvent identifies and dates a user-data notification.
type UserDataEvent struct {
	EventType       string     `json:"e"`
	EventTime       types.Time `json:"E"`
	TransactionTime types.Time `json:"T"`
}

// OptionsAccountUpdate reports account equity and margin in USDT.
type OptionsAccountUpdate struct {
	UserDataEvent
	Equity            types.Number `json:"eq"`
	AdjustedEquity    types.Number `json:"aeq"`
	WalletBalance     types.Number `json:"b"`
	PositionValue     types.Number `json:"m"`
	UnrealisedPNL     types.Number `json:"u"`
	InitialMargin     types.Number `json:"i"`
	MaintenanceMargin types.Number `json:"M"`
}

// OptionsBalancePositionUpdate contains only balances and positions that changed.
type OptionsBalancePositionUpdate struct {
	UserDataEvent
	Reason    string                  `json:"m"`
	Balances  []OptionsBalanceUpdate  `json:"B"`
	Positions []OptionsPositionUpdate `json:"P"`
}

// OptionsBalanceUpdate reports an options margin balance change.
type OptionsBalanceUpdate struct {
	Asset         currency.Code `json:"a"`
	Balance       types.Number  `json:"b"`
	BalanceChange types.Number  `json:"bc"`
}

// OptionsPositionUpdate reports an options position change.
type OptionsPositionUpdate struct {
	Symbol            string       `json:"s"`
	Quantity          types.Number `json:"c"`
	Value             types.Number `json:"p"`
	AverageEntryPrice types.Number `json:"a"`
}

// OptionsGreekUpdate reports risk sensitivities for underlying assets.
type OptionsGreekUpdate struct {
	UserDataEvent
	Greeks []OptionsGreek `json:"G"`
}

// OptionsGreek contains the risk sensitivities for one underlying.
type OptionsGreek struct {
	Underlying currency.Code `json:"u"`
	Delta      types.Number  `json:"d"`
	Gamma      types.Number  `json:"g"`
	Theta      types.Number  `json:"t"`
	Vega       types.Number  `json:"v"`
}

// OptionsRiskLevelChange reports the account's risk level and margin.
type OptionsRiskLevelChange struct {
	UserDataEvent
	RiskLevel         string       `json:"s"`
	MarginBalance     types.Number `json:"mb"`
	MaintenanceMargin types.Number `json:"mm"`
}

// FuturesMarginCall identifies positions subject to a margin call.
type FuturesMarginCall struct {
	UserDataEvent
	AccountAlias       string                      `json:"i"`
	CrossWalletBalance types.Number                `json:"cw"`
	Positions          []FuturesMarginCallPosition `json:"p"`
}

// FuturesMarginCallPosition contains a position's margin requirements.
type FuturesMarginCallPosition struct {
	Symbol            string       `json:"s"`
	PositionSide      string       `json:"ps"`
	PositionAmount    types.Number `json:"pa"`
	MarginType        string       `json:"mt"`
	IsolatedWallet    types.Number `json:"iw"`
	MarkPrice         types.Number `json:"mp"`
	UnrealisedPNL     types.Number `json:"up"`
	MaintenanceMargin types.Number `json:"mm"`
}

// FuturesTradeLite is the compact futures trade execution event.
type FuturesTradeLite struct {
	UserDataEvent
	Symbol             string       `json:"s"`
	OriginalQuantity   types.Number `json:"q"`
	OriginalPrice      types.Number `json:"p"`
	IsMaker            bool         `json:"m"`
	ClientOrderID      string       `json:"c"`
	Side               string       `json:"S"`
	LastFilledPrice    types.Number `json:"L"`
	LastFilledQuantity types.Number `json:"l"`
	TradeID            uint64       `json:"t"`
	OrderID            uint64       `json:"i"`
}

// FuturesConditionalOrderReject reports a rejected trigger.
type FuturesConditionalOrderReject struct {
	UserDataEvent
	Order *FuturesRejectedOrder `json:"or"`
}

// FuturesRejectedOrder identifies an order and its rejection reason.
type FuturesRejectedOrder struct {
	Symbol  string `json:"s"`
	OrderID uint64 `json:"i"`
	Reason  string `json:"r"`
}

// FuturesStrategyUpdate reports a strategy change.
type FuturesStrategyUpdate struct {
	UserDataEvent
	Strategy *FuturesStrategy `json:"su"`
}

// FuturesStrategy describes the strategy and the reason it changed.
type FuturesStrategy struct {
	StrategyID     uint64     `json:"si"`
	StrategyType   string     `json:"st"`
	StrategyStatus string     `json:"ss"`
	Symbol         string     `json:"s"`
	UpdateTime     types.Time `json:"ut"`
	OperationCode  uint64     `json:"c"`
}

// FuturesGridUpdate decodes the legacy grid strategy event.
type FuturesGridUpdate struct {
	UserDataEvent
	Grid *FuturesGrid `json:"gu"`
}

// FuturesGrid contains a grid strategy's realised and unmatched amounts.
type FuturesGrid struct {
	StrategyID            uint64       `json:"si"`
	StrategyType          string       `json:"st"`
	StrategyStatus        string       `json:"ss"`
	Symbol                string       `json:"s"`
	RealisedPNL           types.Number `json:"r"`
	UnmatchedAveragePrice types.Number `json:"up"`
	UnmatchedQuantity     types.Number `json:"uq"`
	UnmatchedFee          types.Number `json:"uf"`
	MatchedPNL            types.Number `json:"mp"`
	UpdateTime            types.Time   `json:"ut"`
}

// FuturesAlgoUpdate reports a conditional algo order's current state.
type FuturesAlgoUpdate struct {
	UserDataEvent
	Order *FuturesAlgoUpdateOrder `json:"o"`
}

// FuturesAlgoUpdateOrder includes execution details once the algo is triggered.
type FuturesAlgoUpdateOrder struct {
	ClientAlgoID            string              `json:"caid"`
	AlgoID                  uint64              `json:"aid"`
	AlgoType                string              `json:"at"`
	OrderType               string              `json:"o"`
	Symbol                  string              `json:"s"`
	Side                    string              `json:"S"`
	PositionSide            string              `json:"ps"`
	TimeInForce             string              `json:"f"`
	Quantity                types.Number        `json:"q"`
	AlgoStatus              string              `json:"X"`
	ActualOrderID           types.PreciseNumber `json:"ai"`
	AverageFillPrice        types.Number        `json:"ap"`
	ExecutedQuantity        types.Number        `json:"aq"`
	ActualOrderType         string              `json:"act"`
	TriggerPrice            types.Number        `json:"tp"`
	Price                   types.Number        `json:"p"`
	SelfTradePreventionMode string              `json:"V"`
	WorkingType             string              `json:"wt"`
	PriceMatch              string              `json:"pm"`
	CloseAll                bool                `json:"cp"`
	PriceProtect            bool                `json:"pP"`
	ReduceOnly              bool                `json:"R"`
	TriggerTime             Timestamp           `json:"tt"`
	GoodTillDate            Timestamp           `json:"gtd"`
	FailureReason           string              `json:"rm"`
	Activated               bool                `json:"ia"`
}

// FuturesSymbolConfigUpdate reports the leverage for one symbol.
type FuturesSymbolConfigUpdate struct {
	Symbol   string `json:"s"`
	Leverage uint64 `json:"l"`
}

// FuturesMultiAssetConfigUpdate reports whether multi-asset margin is active.
type FuturesMultiAssetConfigUpdate struct {
	MultiAssetsMode bool `json:"j"`
}
