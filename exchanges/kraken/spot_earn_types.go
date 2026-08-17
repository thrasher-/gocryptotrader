package kraken

import (
	"errors"
	"time"

	"github.com/thrasher-corp/gocryptotrader/types"
)

var (
	errEarnLockTypeInvalid = errors.New("earn strategy lock type is invalid")
	errStrategyIDRequired  = errors.New("earn strategy identifier is required")
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
