package gateio

import (
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// StakingCoin holds an on-chain staking coin product detail
type StakingCoin struct {
	PID                 uint64                   `json:"pid"`
	ProductType         uint64                   `json:"productType"`
	IsDeFi              uint64                   `json:"isDefi"`
	Currency            string                   `json:"currency"`
	EstimatedAPR        types.Number             `json:"estimateApr"`
	MinStakeAmount      types.Number             `json:"minStakeAmount"`
	MaxStakeAmount      types.Number             `json:"maxStakeAmount"`
	ProtocolName        string                   `json:"protocolName"`
	RedeemPeriod        uint64                   `json:"redeemPeriod"`
	ExchangeRate        types.Number             `json:"exchangeRate"`
	ExchangeRateReserve types.Number             `json:"exchangeRateReserve"`
	ExtraInterest       []StakingExtraInterest   `json:"extraInterest"`
	CurrencyRewards     []StakingCurrencyRewards `json:"currencyRewards"`
}

// StakingExtraInterest holds an additional staking reward period.
type StakingExtraInterest struct {
	StartTime       types.Time               `json:"start_time"`
	EndTime         types.Time               `json:"end_time"`
	RewardCoin      currency.Code            `json:"reward_coin"`
	SegmentInterest []StakingSegmentInterest `json:"segment_interest"`
}

// StakingSegmentInterest holds a staking reward tier.
type StakingSegmentInterest struct {
	MinimumAmount types.Number `json:"money_min"`
	MaximumAmount types.Number `json:"money_max"`
	Rate          types.Number `json:"money_rate"`
}

// StakingCurrencyRewards holds reward currency distribution details.
type StakingCurrencyRewards struct {
	APR               types.Number  `json:"apr"`
	RewardCoin        currency.Code `json:"reward_coin"`
	RewardDelayDays   int64         `json:"reward_delay_days"`
	InterestDelayDays uint64        `json:"interest_delay_days"`
}

// StakingSwapRequest holds an on-chain token swap request for earned coins
type StakingSwapRequest struct {
	Coin   string       `json:"coin"`
	Side   uint64       `json:"side"`
	Amount types.Number `json:"amount"`
	PID    uint64       `json:"pid,omitempty"`
}

// StakingSwapResponse holds the response for an on-chain staking swap
type StakingSwapResponse struct {
	ID             uint64       `json:"id"`
	PID            uint64       `json:"pid"`
	UID            uint64       `json:"uid"`
	Coin           string       `json:"coin"`
	Type           uint64       `json:"type"`
	Subtype        string       `json:"subtype"`
	Amount         types.Number `json:"amount"`
	ExchangeRate   types.Number `json:"exchange_rate"`
	ExchangeAmount types.Number `json:"exchange_amount"`
	UpdateTime     types.Time   `json:"updateStamp"`
	CreateTime     types.Time   `json:"createStamp"`
	Status         uint64       `json:"status"`
	ProtocolType   uint64       `json:"protocol_type"`
	ClientOrderID  string       `json:"client_order_id"`
	Source         string       `json:"source"`
}

// StakingOrderItem holds an on-chain staking order item
type StakingOrderItem struct {
	PID            uint64       `json:"pid"`
	Coin           string       `json:"coin"`
	Amount         types.Number `json:"amount"`
	Type           uint64       `json:"type"`
	Status         uint64       `json:"status"`
	RedeemTime     types.Time   `json:"redeem_stamp"`
	CreateTime     types.Time   `json:"createStamp"`
	ExchangeAmount types.Number `json:"exchange_amount"`
	Fee            types.Number `json:"fee"`
}

// StakingOrdersResponse holds the paginated response for on-chain staking orders
type StakingOrdersResponse struct {
	Page       uint64              `json:"page"`
	PageSize   uint64              `json:"pageSize"`
	PageCount  uint64              `json:"pageCount"`
	TotalCount uint64              `json:"totalCount"`
	List       []*StakingOrderItem `json:"list"`
}

// StakingDividendRecord holds an on-chain staking dividend record item
type StakingDividendRecord struct {
	PID          uint64        `json:"pid"`
	MortgageCoin currency.Code `json:"mortgage_coin"`
	Amount       types.Number  `json:"amount"`
	RewardCoin   currency.Code `json:"reward_coin"`
	Interest     types.Number  `json:"interest"`
	Fee          types.Number  `json:"fee"`
	Status       uint64        `json:"status"`
	BonusDate    string        `json:"bonus_date"`
	BonusTime    types.Time    `json:"should_bonus_stamp"`
}

// StakingDividendRecordsResponse holds the paginated response for staking dividend records
type StakingDividendRecordsResponse struct {
	Page       uint64                   `json:"page"`
	PageSize   uint64                   `json:"pageSize"`
	PageCount  uint64                   `json:"pageCount"`
	TotalCount uint64                   `json:"totalCount"`
	List       []*StakingDividendRecord `json:"list"`
}

// StakingAssetItem holds an on-chain staking asset item
type StakingAssetItem struct {
	PID                    uint64                   `json:"pid"`
	MortgageCoin           string                   `json:"mortgage_coin"`
	MortgageAmount         types.Number             `json:"mortgage_amount"`
	CreateTime             types.Time               `json:"createStamp"`
	ExtraIncome            types.Number             `json:"extra_income"`
	FreezeAmount           types.Number             `json:"freeze_amount"`
	MoveIncome             types.Number             `json:"move_income"`
	Type                   uint64                   `json:"type"`
	Status                 uint64                   `json:"status"`
	IncomeTotal            types.Number             `json:"income_total"`
	YesterdayIncomeByCoin  []StakingCoinIncome      `json:"yesterday_income_multi"`
	RewardCoins            []StakingCurrencyRewards `json:"reward_coins"`
	DecentralisedFinancial StakingDeFiIncome        `json:"defi_income"`
}

// StakingCoinIncome holds a staking reward amount denominated in one currency.
type StakingCoinIncome struct {
	Coin   currency.Code `json:"coin"`
	Amount types.Number  `json:"amount"`
}

// StakingDeFiIncome holds decentralised-finance staking earnings.
type StakingDeFiIncome struct {
	Total []StakingCoinIncome `json:"total"`
}
