package kraken

import (
	"context"
	"net/url"
	"strconv"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

// AllocateEarnFunds allocates funds to an Earn strategy, Kraken's replacement for legacy staking.
func (e *Exchange) AllocateEarnFunds(ctx context.Context, req *AllocateEarnFundsRequest) (*bool, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Amount <= 0 {
		return nil, errAmountInvalid
	}
	if req.StrategyID == "" {
		return nil, errStrategyIDRequired
	}

	amount, err := formatSpotFloat(req.Amount)
	if err != nil {
		return nil, err
	}
	params := url.Values{"amount": {amount}, "strategy_id": {req.StrategyID}}
	var result bool
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Earn/Allocate", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeallocateEarnFunds deallocates funds from an Earn strategy.
func (e *Exchange) DeallocateEarnFunds(ctx context.Context, req *DeallocateEarnFundsRequest) (*bool, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Amount <= 0 {
		return nil, errAmountInvalid
	}
	if req.StrategyID == "" {
		return nil, errStrategyIDRequired
	}

	amount, err := formatSpotFloat(req.Amount)
	if err != nil {
		return nil, err
	}
	params := url.Values{"amount": {amount}, "strategy_id": {req.StrategyID}}
	var result bool
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Earn/Deallocate", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetEarnAllocationStatus returns the status of an allocation operation.
func (e *Exchange) GetEarnAllocationStatus(ctx context.Context, req *EarnOperationStatusRequest) (*EarnOperationStatusResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.StrategyID == "" {
		return nil, errStrategyIDRequired
	}

	var result EarnOperationStatusResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Earn/AllocateStatus", url.Values{"strategy_id": {req.StrategyID}}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetEarnDeallocationStatus returns the status of a deallocation operation.
func (e *Exchange) GetEarnDeallocationStatus(ctx context.Context, req *EarnOperationStatusRequest) (*EarnOperationStatusResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.StrategyID == "" {
		return nil, errStrategyIDRequired
	}

	var result EarnOperationStatusResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Earn/DeallocateStatus", url.Values{"strategy_id": {req.StrategyID}}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListEarnStrategies returns Kraken Earn strategies, including staking-backed strategies.
func (e *Exchange) ListEarnStrategies(ctx context.Context, req *ListEarnStrategiesRequest) (*ListEarnStrategiesResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	for i := range req.LockType {
		if req.LockType[i] == "" || !isValidSpotEnum(req.LockType[i], "flex", "bonded", "timed", "instant") {
			return nil, errEarnLockTypeInvalid
		}
	}
	params := url.Values{}
	if req.Ascending != nil {
		params.Set("ascending", strconv.FormatBool(*req.Ascending))
	}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}
	if req.Limit != nil {
		params.Set("limit", strconv.FormatUint(uint64(*req.Limit), 10))
	}
	if len(req.LockType) > 0 {
		encodedLockTypes, _ := json.Marshal(req.LockType) // Lock types have string wire values.
		params.Set("lock_type", string(encodedLockTypes))
	}

	var result ListEarnStrategiesResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Earn/Strategies", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListEarnAllocations returns the account's current Earn allocations and rewards.
func (e *Exchange) ListEarnAllocations(ctx context.Context, req *ListEarnAllocationsRequest) (*ListEarnAllocationsResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	params := url.Values{}
	if req.Ascending != nil {
		params.Set("ascending", strconv.FormatBool(*req.Ascending))
	}
	if req.ConvertedAsset != "" {
		params.Set("converted_asset", req.ConvertedAsset)
	}
	if req.HideZeroAllocations != nil {
		params.Set("hide_zero_allocations", strconv.FormatBool(*req.HideZeroAllocations))
	}

	var result ListEarnAllocationsResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Earn/Allocations", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
