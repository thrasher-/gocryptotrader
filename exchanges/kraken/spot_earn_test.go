package kraken

import (
	"math"
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

func TestAllocateEarnFunds(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotEarnFixtures)
	ctx := t.Context()

	_, err := ex.AllocateEarnFunds(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AllocateEarnFunds must reject a nil request")
	_, err = ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{})
	require.ErrorIs(t, err, errAmountInvalid, "AllocateEarnFunds must require a positive amount")
	_, err = ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1})
	require.ErrorIs(t, err, errStrategyIDRequired, "AllocateEarnFunds must require a strategy identifier")
	_, err = ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: math.NaN(), StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errNumericValueInvalid, "AllocateEarnFunds must reject a non-finite amount")
	allocated, err := ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.NoError(t, err, "AllocateEarnFunds must not error")
	require.NotNil(t, allocated, "AllocateEarnFunds must decode a non-null result")
	assert.True(t, *allocated, "AllocateEarnFunds should decode success")
	responseJSON, err := json.Marshal(allocated)
	require.NoError(t, err, "AllocateEarnFunds must encode the decoded response")
	assert.Contains(t, string(responseJSON), `true`, "AllocateEarnFunds should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/Earn/Allocate")
	assert.Equal(t, "STRATEGY", values.Get("strategy_id"), "AllocateEarnFunds should encode the strategy identifier")
	assert.Equal(t, "1", values.Get("amount"), "AllocateEarnFunds should encode amount")

	allocated, err = newSpotNullResultExchange(t).AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.NoError(t, err, "AllocateEarnFunds must accept a null result")
	require.NotNil(t, allocated, "AllocateEarnFunds must return a zero-value scalar for a null result")
	assert.False(t, *allocated, "AllocateEarnFunds should return the zero-value scalar for a null result")
	allocated, err = newSpotErrorExchange(t).AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errSpotTransport, "AllocateEarnFunds must surface request errors")
	assert.Nil(t, allocated, "AllocateEarnFunds result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLiveEarnAllocation)
		amount, err := parseSpotLiveTestPositiveFloat(spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_EARN_ALLOCATE_AMOUNT"))
		require.NoError(t, err, "AllocateEarnFunds live amount must be valid")
		strategyID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_EARN_STRATEGY_ID")
		response, err := spotLiveExchange.AllocateEarnFunds(t.Context(), &AllocateEarnFundsRequest{Amount: amount, StrategyID: strategyID})
		require.NoError(t, err, "AllocateEarnFunds must not error against the live API")
		require.NotNil(t, response, "AllocateEarnFunds must return a response from the live API")
		require.True(t, *response, "AllocateEarnFunds live response must confirm the allocation")
	})
}

func TestDeallocateEarnFunds(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotEarnFixtures)
	ctx := t.Context()

	_, err := ex.DeallocateEarnFunds(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "DeallocateEarnFunds must reject a nil request")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{})
	require.ErrorIs(t, err, errAmountInvalid, "DeallocateEarnFunds must require a positive amount")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1})
	require.ErrorIs(t, err, errStrategyIDRequired, "DeallocateEarnFunds must require a strategy identifier")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: math.NaN(), StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errNumericValueInvalid, "DeallocateEarnFunds must reject a non-finite amount")
	deallocated, err := ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.NoError(t, err, "DeallocateEarnFunds must not error")
	require.NotNil(t, deallocated, "DeallocateEarnFunds must decode a non-null result")
	assert.True(t, *deallocated, "DeallocateEarnFunds should decode success")
	responseJSON, err := json.Marshal(deallocated)
	require.NoError(t, err, "DeallocateEarnFunds must encode the decoded response")
	assert.Contains(t, string(responseJSON), `true`, "DeallocateEarnFunds should decode the response")
	requireSpotRequest(t, requests, "/0/private/Earn/Deallocate")

	deallocated, err = newSpotNullResultExchange(t).DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.NoError(t, err, "DeallocateEarnFunds must accept a null result")
	require.NotNil(t, deallocated, "DeallocateEarnFunds must return a zero-value scalar for a null result")
	assert.False(t, *deallocated, "DeallocateEarnFunds should return the zero-value scalar for a null result")
	deallocated, err = newSpotErrorExchange(t).DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errSpotTransport, "DeallocateEarnFunds must surface request errors")
	assert.Nil(t, deallocated, "DeallocateEarnFunds result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLiveEarnDeallocation)
		amount, err := parseSpotLiveTestPositiveFloat(spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_EARN_DEALLOCATE_AMOUNT"))
		require.NoError(t, err, "DeallocateEarnFunds live amount must be valid")
		strategyID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_EARN_STRATEGY_ID")
		response, err := spotLiveExchange.DeallocateEarnFunds(t.Context(), &DeallocateEarnFundsRequest{Amount: amount, StrategyID: strategyID})
		require.NoError(t, err, "DeallocateEarnFunds must not error against the live API")
		require.NotNil(t, response, "DeallocateEarnFunds must return a response from the live API")
		require.True(t, *response, "DeallocateEarnFunds live response must confirm the deallocation")
	})
}

func TestGetEarnAllocationStatus(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotEarnFixtures)
	ctx := t.Context()

	_, err := ex.GetEarnAllocationStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetEarnAllocationStatus must reject a nil request")
	_, err = ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{})
	require.ErrorIs(t, err, errStrategyIDRequired, "GetEarnAllocationStatus must require a strategy identifier")
	allocationStatus, err := ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.NoError(t, err, "GetEarnAllocationStatus must not error")
	require.NotNil(t, allocationStatus, "GetEarnAllocationStatus must decode a non-null result")
	assert.False(t, allocationStatus.Pending, "GetEarnAllocationStatus should decode pending status")
	responseJSON, err := json.Marshal(allocationStatus)
	require.NoError(t, err, "GetEarnAllocationStatus must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"pending":false`, "GetEarnAllocationStatus should decode the response")
	requireSpotRequest(t, requests, "/0/private/Earn/AllocateStatus")

	allocationStatus, err = newSpotNullResultExchange(t).GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.NoError(t, err, "GetEarnAllocationStatus must accept a null result")
	assert.Nil(t, allocationStatus, "GetEarnAllocationStatus should return nil for a null result")
	allocationStatus, err = newSpotErrorExchange(t).GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errSpotTransport, "GetEarnAllocationStatus must surface request errors")
	assert.Nil(t, allocationStatus, "GetEarnAllocationStatus result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		strategyID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_EARN_STRATEGY_ID")
		response, err := spotLiveExchange.GetEarnAllocationStatus(t.Context(), &EarnOperationStatusRequest{StrategyID: strategyID})
		require.NoError(t, err, "GetEarnAllocationStatus must not error against the live API")
		require.NotNil(t, response, "GetEarnAllocationStatus must return a response from the live API")
	})
}

func TestGetEarnDeallocationStatus(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotEarnFixtures)
	ctx := t.Context()

	_, err := ex.GetEarnDeallocationStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetEarnDeallocationStatus must reject a nil request")
	_, err = ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{})
	require.ErrorIs(t, err, errStrategyIDRequired, "GetEarnDeallocationStatus must require a strategy identifier")
	deallocationStatus, err := ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.NoError(t, err, "GetEarnDeallocationStatus must not error")
	require.NotNil(t, deallocationStatus, "GetEarnDeallocationStatus must decode a non-null result")
	assert.False(t, deallocationStatus.Pending, "GetEarnDeallocationStatus should decode pending status")
	responseJSON, err := json.Marshal(deallocationStatus)
	require.NoError(t, err, "GetEarnDeallocationStatus must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"pending":false`, "GetEarnDeallocationStatus should decode the response")
	requireSpotRequest(t, requests, "/0/private/Earn/DeallocateStatus")

	deallocationStatus, err = newSpotNullResultExchange(t).GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.NoError(t, err, "GetEarnDeallocationStatus must accept a null result")
	assert.Nil(t, deallocationStatus, "GetEarnDeallocationStatus should return nil for a null result")
	deallocationStatus, err = newSpotErrorExchange(t).GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errSpotTransport, "GetEarnDeallocationStatus must surface request errors")
	assert.Nil(t, deallocationStatus, "GetEarnDeallocationStatus result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		strategyID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_EARN_STRATEGY_ID")
		response, err := spotLiveExchange.GetEarnDeallocationStatus(t.Context(), &EarnOperationStatusRequest{StrategyID: strategyID})
		require.NoError(t, err, "GetEarnDeallocationStatus must not error against the live API")
		require.NotNil(t, response, "GetEarnDeallocationStatus must return a response from the live API")
	})
}

func TestListEarnStrategies(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotEarnFixtures)
	ctx := t.Context()

	_, err := ex.ListEarnStrategies(ctx, nil)
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
	responseJSON, err := json.Marshal(strategies)
	require.NoError(t, err, "ListEarnStrategies must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"id":"STRATEGY"`, "ListEarnStrategies should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/Earn/Strategies")
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

	strategies, err = newSpotNullResultExchange(t).ListEarnStrategies(ctx, &ListEarnStrategiesRequest{})
	require.NoError(t, err, "ListEarnStrategies must accept a null result")
	assert.Nil(t, strategies, "ListEarnStrategies should return nil for a null result")
	strategies, err = newSpotErrorExchange(t).ListEarnStrategies(ctx, &ListEarnStrategiesRequest{})
	require.ErrorIs(t, err, errSpotTransport, "ListEarnStrategies must surface request errors")
	assert.Nil(t, strategies, "ListEarnStrategies result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.ListEarnStrategies(t.Context(), new(ListEarnStrategiesRequest))
		require.NoError(t, err, "ListEarnStrategies must not error against the live API")
		require.NotNil(t, response, "ListEarnStrategies must return a response from the live API")
		require.NotNil(t, response.Items, "ListEarnStrategies live response must include strategy items")
	})
}

func TestListEarnAllocations(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotEarnFixtures)
	ctx := t.Context()
	ascending := true

	_, err := ex.ListEarnAllocations(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "ListEarnAllocations must reject a nil request")
	hideZero := false
	allocations, err := ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{Ascending: &ascending, ConvertedAsset: "USD", HideZeroAllocations: &hideZero})
	require.NoError(t, err, "ListEarnAllocations must not error")
	require.NotNil(t, allocations, "ListEarnAllocations must decode a non-null result")
	assert.Equal(t, "STRATEGY", allocations.Items[0].StrategyID, "ListEarnAllocations should decode strategy allocations")
	assert.Equal(t, "NEXT", allocations.NextCursor, "ListEarnAllocations should decode pagination")
	responseJSON, err := json.Marshal(allocations)
	require.NoError(t, err, "ListEarnAllocations must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"strategy_id":"STRATEGY"`, "ListEarnAllocations should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/Earn/Allocations")
	assert.Equal(t, "false", values.Get("hide_zero_allocations"), "ListEarnAllocations should encode a false zero-allocation filter")
	_, err = ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{})
	require.NoError(t, err, "ListEarnAllocations must allow an unfiltered request")
	requireSpotRequest(t, requests, "/0/private/Earn/Allocations")

	allocations, err = newSpotNullResultExchange(t).ListEarnAllocations(ctx, &ListEarnAllocationsRequest{})
	require.NoError(t, err, "ListEarnAllocations must accept a null result")
	assert.Nil(t, allocations, "ListEarnAllocations should return nil for a null result")
	allocations, err = newSpotErrorExchange(t).ListEarnAllocations(ctx, &ListEarnAllocationsRequest{})
	require.ErrorIs(t, err, errSpotTransport, "ListEarnAllocations must surface request errors")
	assert.Nil(t, allocations, "ListEarnAllocations result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.ListEarnAllocations(t.Context(), new(ListEarnAllocationsRequest))
		require.NoError(t, err, "ListEarnAllocations must not error against the live API")
		require.NotNil(t, response, "ListEarnAllocations must return a response from the live API")
		require.NotNil(t, response.Items, "ListEarnAllocations live response must include allocation items")
	})
}
