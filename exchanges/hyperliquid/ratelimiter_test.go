package hyperliquid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

func TestGetRateLimits(t *testing.T) {
	t.Parallel()
	limits := GetRateLimits()
	for _, epl := range []request.EndpointLimit{
		infoStandardEPL,
		infoLightEPL,
		infoRecentTradesEPL,
		infoUserRoleEPL,
		infoHistoricalOrdersEPL,
		infoFundingHistoryEPL,
		infoUserLedgerEPL,
		infoUserFillsEPL,
		infoUserFundingEPL,
		infoStakingHistoryEPL,
		candleEndpointLimit(1),
		candleEndpointLimit(maximumCandleCount),
		exchangeActionEndpointLimit(1),
		exchangeActionEndpointLimit(maximumActionBatchSize),
	} {
		assert.Containsf(t, limits, epl, "GetRateLimits should define endpoint limit %d", epl)
	}
	assert.Len(t, limits, 10+maximumCandleBuckets+maximumActionBatchSize/actionBatchWeightSize+1, "GetRateLimits should define every info, candle and action weight")
	assert.Equal(t, 21, recentTradesWeight, "recentTradesWeight should reserve one per-item weight for ten trades")
	assert.Equal(t, 120, historicalOrdersWeight, "historicalOrdersWeight should reserve per-item weight for 2000 orders")
	assert.Equal(t, 45, fundingHistoryWeight, "fundingHistoryWeight should reserve per-item weight for 500 records")
	assert.Equal(t, 45, userLedgerHistoryWeight, "userLedgerHistoryWeight should reserve per-item weight for 500 records")
	assert.Equal(t, 120, userFillsWeight, "userFillsWeight should reserve per-item weight for 2000 fills")
	assert.Equal(t, 45, userFundingWeight, "userFundingWeight should reserve per-item weight for 500 payments")
	assert.Equal(t, 120, stakingHistoryWeight, "stakingHistoryWeight should reserve per-item weight for 2000 entries")
}

func TestCandleEndpointLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		count uint64
		exp   request.EndpointLimit
	}{
		{count: 0, exp: 1},
		{count: 1, exp: 1},
		{count: 60, exp: 1},
		{count: 61, exp: 2},
		{count: maximumCandleCount, exp: 84},
		{count: maximumCandleCount + 1, exp: 84},
	} {
		assert.Equalf(t, candleEPLBase+tc.exp, candleEndpointLimit(tc.count), "candleEndpointLimit should weigh %d candles", tc.count)
	}
}

func TestExchangeActionEndpointLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		batchLength int
		exp         request.EndpointLimit
	}{
		{batchLength: -1, exp: 0},
		{batchLength: 0, exp: 0},
		{batchLength: 1, exp: 0},
		{batchLength: 39, exp: 0},
		{batchLength: 40, exp: 1},
		{batchLength: 79, exp: 1},
		{batchLength: maximumActionBatchSize, exp: 25},
		{batchLength: maximumActionBatchSize + 1, exp: 25},
	} {
		assert.Equalf(t, exchangeActionEPLBase+tc.exp, exchangeActionEndpointLimit(tc.batchLength), "exchangeActionEndpointLimit should weigh a batch of %d", tc.batchLength)
	}
}
