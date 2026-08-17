package kraken

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

var spotEarnFixtures = spotFixtureSet{results: map[string]string{
	"/0/private/Earn/Allocate":         `true`,
	"/0/private/Earn/Deallocate":       `true`,
	"/0/private/Earn/AllocateStatus":   `{"pending":false}`,
	"/0/private/Earn/DeallocateStatus": `{"pending":false}`,
	"/0/private/Earn/Strategies":       `{"items":[{"id":"STRATEGY","asset":"DOT","lock_type":{"type":"instant","payout_frequency":604800},"apr_estimate":{"low":"8","high":"12"},"allocation_fee":"0","deallocation_fee":0,"auto_compound":{"type":"enabled"},"yield_source":{"type":"staking"},"can_allocate":true,"can_deallocate":true,"allocation_restriction_info":[]}],"next_cursor":"NEXT"}`,
	"/0/private/Earn/Allocations":      `{"converted_asset":"USD","total_allocated":"10","total_rewarded":"1","next_cursor":"NEXT","items":[{"strategy_id":"STRATEGY","native_asset":"DOT","amount_allocated":{"total":{"native":"1","converted":"10"}},"total_rewarded":{"native":"0.1","converted":"1"}}]}`,
}}

func TestSpotEarnEndpointErrors(t *testing.T) {
	ex := newSpotErrorExchange(t)
	ctx := t.Context()

	_, err := ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.Error(t, err, "AllocateEarnFunds must surface request errors")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.Error(t, err, "DeallocateEarnFunds must surface request errors")
	_, err = ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.Error(t, err, "GetEarnAllocationStatus must surface request errors")
	_, err = ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.Error(t, err, "GetEarnDeallocationStatus must surface request errors")
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{})
	require.Error(t, err, "ListEarnStrategies must surface request errors")
	_, err = ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{})
	require.Error(t, err, "ListEarnAllocations must surface request errors")
}

func TestSpotEarnResponseObjectContract(t *testing.T) {
	successEx, _ := newSpotEndpointExchange(t, spotEarnFixtures)
	nilResultEx := newSpotNullResultExchange(t)
	errorEx := newSpotErrorExchange(t)
	ctx := t.Context()

	for _, tc := range []struct {
		name            string
		call            func(*Exchange) (any, error)
		expectedJSON    string
		zeroValueOnNull bool
	}{
		{
			name:            "AllocateEarnFunds",
			zeroValueOnNull: true,
			call: func(ex *Exchange) (any, error) {
				return ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
			},
			expectedJSON: `true`,
		},
		{
			name:            "DeallocateEarnFunds",
			zeroValueOnNull: true,
			call: func(ex *Exchange) (any, error) {
				return ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
			},
			expectedJSON: `true`,
		},
		{
			name: "GetEarnAllocationStatus",
			call: func(ex *Exchange) (any, error) {
				return ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
			},
			expectedJSON: `"pending":false`,
		},
		{
			name: "GetEarnDeallocationStatus",
			call: func(ex *Exchange) (any, error) {
				return ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
			},
			expectedJSON: `"pending":false`,
		},
		{
			name:         "ListEarnStrategies",
			call:         func(ex *Exchange) (any, error) { return ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{}) },
			expectedJSON: `"id":"STRATEGY"`,
		},
		{
			name:         "ListEarnAllocations",
			call:         func(ex *Exchange) (any, error) { return ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{}) },
			expectedJSON: `"strategy_id":"STRATEGY"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call(successEx)
			require.NoError(t, err, tc.name+" must not error")
			require.NotNil(t, result, tc.name+" must return a response")
			responseJSON, err := json.Marshal(result)
			require.NoError(t, err, tc.name+" must encode the decoded response")
			assert.Contains(t, string(responseJSON), tc.expectedJSON, tc.name+" should decode the response")

			result, err = tc.call(nilResultEx)
			require.NoError(t, err, tc.name+" must accept a null result")
			if tc.zeroValueOnNull {
				require.NotNil(t, result, tc.name+" must return a zero-value scalar for a null result")
				assert.True(t, reflect.ValueOf(result).Elem().IsZero(), tc.name+" should return the zero-value scalar for a null result")
			} else {
				assert.Nil(t, result, tc.name+" should return nil for a null result")
			}

			result, err = tc.call(errorEx)
			require.ErrorIs(t, err, errSpotTransport, tc.name+" must surface request errors")
			assert.Nil(t, result, tc.name+" result should remain nil on request errors")
		})
	}
}

func TestSpotEarnEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotEarnFixtures)
	ctx := t.Context()

	_, err := ex.AllocateEarnFunds(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AllocateEarnFunds must reject a nil request")
	_, err = ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{})
	require.ErrorIs(t, err, errAmountInvalid, "AllocateEarnFunds must require a positive amount")
	_, err = ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1})
	require.ErrorIs(t, err, errStrategyIDRequired, "AllocateEarnFunds must require a strategy identifier")
	allocated, err := ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.NoError(t, err, "AllocateEarnFunds must not error")
	require.NotNil(t, allocated, "AllocateEarnFunds must decode a non-null result")
	assert.True(t, *allocated, "AllocateEarnFunds should decode success")
	values := requireSpotRequest(t, requests, "/0/private/Earn/Allocate")
	assert.Equal(t, "STRATEGY", values.Get("strategy_id"), "AllocateEarnFunds should encode the strategy identifier")
	assert.Equal(t, "1", values.Get("amount"), "AllocateEarnFunds should encode amount")

	_, err = ex.DeallocateEarnFunds(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "DeallocateEarnFunds must reject a nil request")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{})
	require.ErrorIs(t, err, errAmountInvalid, "DeallocateEarnFunds must require a positive amount")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1})
	require.ErrorIs(t, err, errStrategyIDRequired, "DeallocateEarnFunds must require a strategy identifier")
	deallocated, err := ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.NoError(t, err, "DeallocateEarnFunds must not error")
	require.NotNil(t, deallocated, "DeallocateEarnFunds must decode a non-null result")
	assert.True(t, *deallocated, "DeallocateEarnFunds should decode success")
	requireSpotRequest(t, requests, "/0/private/Earn/Deallocate")

	_, err = ex.GetEarnAllocationStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetEarnAllocationStatus must reject a nil request")
	_, err = ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{})
	require.ErrorIs(t, err, errStrategyIDRequired, "GetEarnAllocationStatus must require a strategy identifier")
	allocationStatus, err := ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.NoError(t, err, "GetEarnAllocationStatus must not error")
	require.NotNil(t, allocationStatus, "GetEarnAllocationStatus must decode a non-null result")
	assert.False(t, allocationStatus.Pending, "GetEarnAllocationStatus should decode pending status")
	requireSpotRequest(t, requests, "/0/private/Earn/AllocateStatus")

	_, err = ex.GetEarnDeallocationStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetEarnDeallocationStatus must reject a nil request")
	_, err = ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{})
	require.ErrorIs(t, err, errStrategyIDRequired, "GetEarnDeallocationStatus must require a strategy identifier")
	deallocationStatus, err := ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.NoError(t, err, "GetEarnDeallocationStatus must not error")
	require.NotNil(t, deallocationStatus, "GetEarnDeallocationStatus must decode a non-null result")
	assert.False(t, deallocationStatus.Pending, "GetEarnDeallocationStatus should decode pending status")
	requireSpotRequest(t, requests, "/0/private/Earn/DeallocateStatus")

	_, err = ex.ListEarnStrategies(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "ListEarnStrategies must reject a nil request")
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{LockType: []EarnLockType{""}})
	require.ErrorIs(t, err, errEarnLockTypeInvalid, "ListEarnStrategies must reject an empty lock type")
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{LockType: []EarnLockType{"invalid"}})
	require.ErrorIs(t, err, errEarnLockTypeInvalid, "ListEarnStrategies must reject an invalid lock type")
	ascending := true
	strategyLimit := uint16(25)
	strategies, err := ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{
		Ascending: &ascending,
		Asset:     "DOT",
		Cursor:    "CURSOR",
		Limit:     &strategyLimit,
		LockType:  []EarnLockType{EarnLockTypeFlexible, EarnLockTypeBonded, EarnLockTypeTimed, EarnLockTypeInstant},
	})
	require.NoError(t, err, "ListEarnStrategies must not error")
	require.NotNil(t, strategies, "ListEarnStrategies must decode a non-null result")
	assert.Equal(t, "staking", strategies.Items[0].YieldSource.Type, "ListEarnStrategies should expose staking-backed strategies")
	assert.Equal(t, "NEXT", strategies.NextCursor, "ListEarnStrategies should decode pagination")
	values = requireSpotRequest(t, requests, "/0/private/Earn/Strategies")
	assert.Equal(t, `["flex","bonded","timed","instant"]`, values.Get("lock_type"), "ListEarnStrategies should encode lock types as JSON")
	assert.Equal(t, "true", values.Get("ascending"), "ListEarnStrategies should encode sort direction")
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{})
	require.NoError(t, err, "ListEarnStrategies must allow an unfiltered request")
	requireSpotRequest(t, requests, "/0/private/Earn/Strategies")
	strategyLimit = 0
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{Limit: &strategyLimit})
	require.NoError(t, err, "ListEarnStrategies must accept an explicit zero limit")
	values = requireSpotRequest(t, requests, "/0/private/Earn/Strategies")
	assert.Equal(t, "0", values.Get("limit"), "ListEarnStrategies should encode an explicit zero limit")

	_, err = ex.ListEarnAllocations(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "ListEarnAllocations must reject a nil request")
	hideZero := false
	allocations, err := ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{Ascending: &ascending, ConvertedAsset: "USD", HideZeroAllocations: &hideZero})
	require.NoError(t, err, "ListEarnAllocations must not error")
	require.NotNil(t, allocations, "ListEarnAllocations must decode a non-null result")
	assert.Equal(t, "STRATEGY", allocations.Items[0].StrategyID, "ListEarnAllocations should decode strategy allocations")
	assert.Equal(t, "NEXT", allocations.NextCursor, "ListEarnAllocations should decode pagination")
	values = requireSpotRequest(t, requests, "/0/private/Earn/Allocations")
	assert.Equal(t, "false", values.Get("hide_zero_allocations"), "ListEarnAllocations should encode a false zero-allocation filter")
	_, err = ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{})
	require.NoError(t, err, "ListEarnAllocations must allow an unfiltered request")
	requireSpotRequest(t, requests, "/0/private/Earn/Allocations")
}

func TestSpotEarnTypedRequestValidation(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t, spotEarnFixtures)
	notANumber := math.NaN()

	_, err := ex.AllocateEarnFunds(t.Context(), &AllocateEarnFundsRequest{Amount: notANumber, StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errNumericValueInvalid, "AllocateEarnFunds must reject a non-finite amount")
	_, err = ex.DeallocateEarnFunds(t.Context(), &DeallocateEarnFundsRequest{Amount: notANumber, StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errNumericValueInvalid, "DeallocateEarnFunds must reject a non-finite amount")
}
