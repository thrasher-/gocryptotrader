package gateio

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
)

func TestGetStakingCoins(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedHTTPRouteTestExchange(t, http.MethodGet, "/api/v4/earn/staking/coins", `[{"pid":1,"productType":0,"isDefi":1,"currency":"GT","estimateApr":"36.00","minStakeAmount":"1","maxStakeAmount":"700","protocolName":"Gatechain","redeemPeriod":0,"exchangeRate":"1","exchangeRateReserve":"1","extraInterest":[{"start_time":1749427201,"end_time":1749513601,"reward_coin":"USDT","segment_interest":[{"money_min":"1","money_max":"10","money_rate":"0.1"}]}],"currencyRewards":[{"apr":"0.2","reward_coin":"USDT","reward_delay_days":-1,"interest_delay_days":1}]}]`)
	}
	result, err := ex.GetStakingCoins(t.Context(), "")
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		require.Len(t, result, 1)
		assert.Equal(t, uint64(1), result[0].PID)
		assert.Equal(t, int64(-1), result[0].CurrencyRewards[0].RewardDelayDays)
	}
}

func TestSwapStakingCoins(t *testing.T) {
	t.Parallel()
	_, err := e.SwapStakingCoins(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "nil arg must return ErrNilPointer")

	_, err = e.SwapStakingCoins(t.Context(), &StakingSwapRequest{Side: 0, Amount: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "empty coin must return ErrCurrencyCodeEmpty")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedHTTPRouteTestExchange(t, http.MethodPost, "/api/v4/earn/staking/swap", `{"id":1,"pid":2,"uid":3,"coin":"ETH","type":0,"subtype":"stake","amount":"0.01","exchange_rate":"1","exchange_amount":"0.01","updateStamp":1749427201,"createStamp":1749427101,"status":1,"protocol_type":0,"client_order_id":"client-1","source":"api"}`)
	}
	result, err := ex.SwapStakingCoins(t.Context(), &StakingSwapRequest{Coin: "ETH", Side: 0, Amount: 0.01})
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		assert.Equal(t, uint64(1), result.ID)
		assert.Equal(t, "stake", result.Subtype)
	}
}

func TestGetStakingOrders(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedHTTPRouteTestExchange(t, http.MethodGet, "/api/v4/earn/staking/order_list?type=0", `{"page":1,"pageSize":10,"pageCount":1,"totalCount":1,"list":[{"pid":1,"coin":"GT","amount":"2","type":0,"status":1,"redeem_stamp":1749427201,"createStamp":1749427101,"exchange_amount":"2","fee":"0.1"}]}`)
	}
	result, err := ex.GetStakingOrders(t.Context(), currency.EMPTYCODE, 0, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		assert.Equal(t, uint64(1), result.TotalCount)
		require.Len(t, result.List, 1)
		assert.Equal(t, uint64(1), result.List[0].PID)
	}
}

func TestGetStakingDividendRecords(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedHTTPRouteTestExchange(t, http.MethodGet, "/api/v4/earn/staking/award_list?page=10", `{"page":10,"pageSize":10,"pageCount":1,"totalCount":1,"list":[{"pid":1,"mortgage_coin":"GT","amount":"2","reward_coin":"USDT","interest":"0.1","fee":"0.01","status":4,"bonus_date":"2026-07-31","should_bonus_stamp":1749427201}]}`)
	}
	result, err := ex.GetStakingDividendRecords(t.Context(), currency.EMPTYCODE, 10, 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		assert.Equal(t, uint64(1), result.TotalCount)
		assert.Equal(t, uint64(4), result.List[0].Status)
	}
}

func TestGetStakingAssets(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedHTTPRouteTestExchange(t, http.MethodGet, "/api/v4/earn/staking/assets", `[{"pid":1,"mortgage_coin":"GT","mortgage_amount":"2","createStamp":1749427101,"extra_income":"0.1","freeze_amount":"0","move_income":"0","type":0,"status":1,"income_total":"0.2","yesterday_income_multi":[{"coin":"USDT","amount":"0.1"}],"reward_coins":[{"reward_coin":"USDT","interest_delay_days":1,"reward_delay_days":-1}],"defi_income":{"total":[{"coin":"COMP","amount":"0.01"}]}}]`)
	}
	result, err := ex.GetStakingAssets(t.Context(), currency.EMPTYCODE)
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		require.Len(t, result, 1)
		assert.Equal(t, uint64(1), result[0].PID)
		assert.Equal(t, currency.COMP, result[0].DecentralisedFinancial.Total[0].Coin)
	}
}
