package bitfinex

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Please supply API keys here or in config/testdata.json to test authenticated endpoints
const (
	apiKey                  = ""
	apiSecret               = ""
	canManipulateRealOrders = false
)

var (
	e          *Exchange
	btcusdPair = currency.NewBTCUSD()
)

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Bitfinex Setup error: %s", err)
	}

	if apiKey != "" && apiSecret != "" {
		e.Websocket.SetCanUseAuthenticatedEndpoints(true)
		e.API.AuthenticatedSupport = true
		e.API.AuthenticatedWebsocketSupport = true
		e.SetCredentials(apiKey, apiSecret, "", "", "", "")
	}

	os.Exit(m.Run())
}

func TestGetV2MarginFunding(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetV2MarginFunding(t.Context(), "fUSD", "2", 2)
	assert.NoError(t, err, "GetV2MarginFunding should not error")
}

func TestGetV2MarginInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetV2MarginInfo(t.Context(), "base")
	assert.NoError(t, err, "GetV2MarginInfo base should not error")
	_, err = e.GetV2MarginInfo(t.Context(), "tBTCUSD")
	assert.NoError(t, err, "GetV2MarginInfo tBTCUSD should not error")
	_, err = e.GetV2MarginInfo(t.Context(), "sym_all")
	assert.NoError(t, err, "GetV2MarginInfo sym_all should not error")
}

func TestGetAccountInfoV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAccountInfoV2(t.Context())
	assert.NoError(t, err, "GetAccountInfoV2 should not error")
}

func TestGetV2FundingInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetV2FundingInfo(t.Context(), "fUST")
	assert.NoError(t, err, "GetV2FundingInfo should not error")
}

func TestGetV2Balances(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetV2Balances(t.Context())
	assert.NoError(t, err, "GetV2Balances should not error")
}

func TestGetDerivativeStatusInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetDerivativeStatusInfo(t.Context(), "ALL", "", "", 0, 0)
	assert.NoError(t, err, "GetDerivativeStatusInfo should not error")
}

func TestGetPairs(t *testing.T) {
	t.Parallel()

	_, err := e.GetPairs(t.Context(), asset.Binary)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	assets := e.GetAssetTypes(false)
	for x := range assets {
		_, err := e.GetPairs(t.Context(), assets[x])
		assert.NoErrorf(t, err, "GetPairs should not error for asset %s", assets[x])
	}
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	for _, a := range e.GetAssetTypes(false) {
		t.Run(a.String(), func(t *testing.T) {
			t.Parallel()
			switch a {
			case asset.Spot:
				require.NoError(t, e.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must not error")
				pairs, err := e.CurrencyPairs.GetPairs(a, false)
				require.NoError(t, err, "GetPairs must not error")
				l, err := e.GetOrderExecutionLimits(a, pairs[0])
				require.NoError(t, err, "GetOrderExecutionLimits must not error")
				assert.Positive(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive")
			default:
				require.ErrorIs(t, e.UpdateOrderExecutionLimits(t.Context(), a), common.ErrNotYetImplemented)
			}
		})
	}
}

func TestAppendOptionalDelimiter(t *testing.T) {
	t.Parallel()
	curr1, err := currency.NewPairFromString("BTCUSD")
	require.NoError(t, err)

	e.appendOptionalDelimiter(&curr1)
	assert.Empty(t, curr1.Delimiter, "appendOptionalDelimiter should leave delimiter empty when absent")
	curr2, err := currency.NewPairFromString("DUSK:USD")
	require.NoError(t, err)

	curr2.Delimiter = ""
	e.appendOptionalDelimiter(&curr2)
	assert.Equal(t, ":", curr2.Delimiter, "appendOptionalDelimiter should reapply colon delimiter")
}

func TestGetPlatformStatus(t *testing.T) {
	t.Parallel()
	result, err := e.GetPlatformStatus(t.Context())
	require.NoError(t, err, "GetPlatformStatus must not error")
	assert.Contains(t, []int{bitfinexOperativeMode, bitfinexMaintenanceMode}, result,
		"GetPlatformStatus should return a known status code")
}

func TestGetTickerBatch(t *testing.T) {
	t.Parallel()
	ticks, err := e.GetTickerBatch(t.Context())
	require.NoError(t, err, "GetTickerBatch must not error")
	require.NotEmpty(t, ticks, "GetTickerBatch must return some ticks")
	require.Contains(t, ticks, "tBTCUSD", "Ticker batch must contain tBTCUSD")
	checkTradeTick(t, ticks["tBTCUSD"])
	require.Contains(t, ticks, "fUSD", "Ticker batch must contain fUSD")
	checkTradeTick(t, ticks["fUSD"])
}

func TestGetTicker(t *testing.T) {
	t.Parallel()
	tick, err := e.GetTicker(t.Context(), "tBTCUSD")
	require.NoError(t, err, "GetTicker must not error")
	checkTradeTick(t, tick)
}

func TestTickerFromResp(t *testing.T) {
	t.Parallel()
	_, err := tickerFromResp("tBTCUSD", []any{100.0, nil, 100.0, nil, nil, nil, nil, nil, nil, nil})
	assert.ErrorIs(t, err, errTickerInvalidResp, "tickerFromResp should error correctly")
	assert.ErrorContains(t, err, "BidSize", "tickerFromResp should error correctly")
	assert.ErrorContains(t, err, "tBTCUSD", "tickerFromResp should error correctly")

	_, err = tickerFromResp("tBTCUSD", []any{100.0, nil, 100.0, nil, nil, nil, nil, nil, nil})
	assert.ErrorIs(t, err, errTickerInvalidFieldCount, "tickerFromResp should error correctly")
	assert.ErrorContains(t, err, "tBTCUSD", "tickerFromResp should error correctly")

	tick, err := tickerFromResp("tBTCUSD", []any{1.1, 2.2, 3.3, 4.4, 5.5, 6.6, 7.7, 8.8, 9.9, 10.10})
	require.NoError(t, err, "tickerFromResp must error correctly")
	assert.Equal(t, 1.1, tick.Bid, "Tick Bid should be correct")
	assert.Equal(t, 2.2, tick.BidSize, "Tick BidSize should be correct")
	assert.Equal(t, 3.3, tick.Ask, "Tick Ask should be correct")
	assert.Equal(t, 4.4, tick.AskSize, "Tick AskSize should be correct")
	assert.Equal(t, 5.5, tick.DailyChange, "Tick DailyChange should be correct")
	assert.Equal(t, 6.6, tick.DailyChangePerc, "Tick DailyChangePerc should be correct")
	assert.Equal(t, 7.7, tick.Last, "Tick Last should be correct")
	assert.Equal(t, 8.8, tick.Volume, "Tick Volume should be correct")
	assert.Equal(t, 9.9, tick.High, "Tick High should be correct")
	assert.Equal(t, 10.10, tick.Low, "Tick Low should be correct")

	_, err = tickerFromResp("fBTC", []any{100.0, nil, 100.0, nil, nil, nil, nil, nil, nil, nil})
	assert.ErrorIs(t, err, errTickerInvalidFieldCount, "tickerFromResp should delegate to tickerFromFundingResp and error correctly")
	assert.ErrorContains(t, err, "fBTC", "tickerFromResp should delegate to tickerFromFundingResp and error correctly")
}

func TestTickerFromFundingResp(t *testing.T) {
	t.Parallel()
	_, err := tickerFromFundingResp("fBTC", []any{nil, 100.0, nil, 100.0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil})
	assert.ErrorIs(t, err, errTickerInvalidResp, "tickerFromFundingResp should error correctly")
	assert.ErrorContains(t, err, "FlashReturnRate", "tickerFromFundingResp should error correctly")
	assert.ErrorContains(t, err, "fBTC", "tickerFromFundingResp should error correctly")

	_, err = tickerFromFundingResp("fBTC", []any{100.0, nil, 100.0, nil, nil, nil, nil, nil, nil})
	assert.ErrorIs(t, err, errTickerInvalidFieldCount, "tickerFromFundingResp should error correctly")
	assert.ErrorContains(t, err, "fBTC", "tickerFromFundingResp should error correctly")

	tick, err := tickerFromFundingResp("fBTC", []any{1.1, 2.2, 3.0, 4.4, 5.5, 6.0, 7.7, 8.8, 9.9, 10.10, 11.11, 12.12, 13.13, nil, nil, 15.15})
	require.NoError(t, err, "tickerFromFundingResp must error correctly")
	assert.Equal(t, 1.1, tick.FlashReturnRate, "Tick FlashReturnRate should be correct")
	assert.Equal(t, 2.2, tick.Bid, "Tick Bid should be correct")
	assert.Equal(t, int64(3), tick.BidPeriod, "Tick BidPeriod should be correct")
	assert.Equal(t, 4.4, tick.BidSize, "Tick BidSize should be correct")
	assert.Equal(t, 5.5, tick.Ask, "Tick Ask should be correct")
	assert.Equal(t, int64(6), tick.AskPeriod, "Tick AskPeriod should be correct")
	assert.Equal(t, 7.7, tick.AskSize, "Tick AskSize should be correct")
	assert.Equal(t, 8.8, tick.DailyChange, "Tick DailyChange should be correct")
	assert.Equal(t, 9.9, tick.DailyChangePerc, "Tick DailyChangePerc should be correct")
	assert.Equal(t, 10.10, tick.Last, "Tick Last should be correct")
	assert.Equal(t, 11.11, tick.Volume, "Tick Volume should be correct")
	assert.Equal(t, 12.12, tick.High, "Tick High should be correct")
	assert.Equal(t, 13.13, tick.Low, "Tick Low should be correct")
	assert.Equal(t, 15.15, tick.FRRAmountAvailable, "Tick FRRAmountAvailable should be correct")
}

func TestGetTickerFunding(t *testing.T) {
	t.Parallel()
	tick, err := e.GetTicker(t.Context(), "fUSD")
	require.NoError(t, err, "GetTicker must not error")
	checkFundingTick(t, tick)
}

func checkTradeTick(tb testing.TB, tick *Ticker) {
	tb.Helper()
	assert.Positive(tb, tick.Bid, "Tick Bid should be positive")
	assert.Positive(tb, tick.BidSize, "Tick BidSize should be positive")
	assert.Positive(tb, tick.Ask, "Tick Ask should be positive")
	assert.Positive(tb, tick.AskSize, "Tick AskSize should be positive")
	assert.Positive(tb, tick.Last, "Tick Last should be positive")
	// Can't test DailyChange*, Volume, High or Low without false positives when they're occasionally 0
}

func checkFundingTick(tb testing.TB, tick *Ticker) {
	tb.Helper()
	assert.NotZero(tb, tick.FlashReturnRate, "Tick FlashReturnRate should not be zero")
	assert.Positive(tb, tick.Bid, "Tick Bid should be positive")
	assert.Positive(tb, tick.BidPeriod, "Tick BidPeriod should be positive")
	assert.Positive(tb, tick.BidSize, "Tick BidSize should be positive")
	assert.Positive(tb, tick.Ask, "Tick Ask should be positive")
	assert.Positive(tb, tick.AskPeriod, "Tick AskPeriod should be positive")
	assert.Positive(tb, tick.AskSize, "Tick AskSize should be positive")
	assert.Positive(tb, tick.Last, "Tick Last should be positive")
	// Can't test FRRAmountAvailable as it's occasionally 0
}

func TestGetTrades(t *testing.T) {
	t.Parallel()

	r, err := e.GetTrades(t.Context(), "tBTCUSD", 5, time.Time{}, time.Time{}, false)
	require.NoError(t, err, "GetTrades must not error")
	assert.NotEmpty(t, r, "GetTrades should return some trades")
}

func TestGetOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderbook(t.Context(), "tBTCUSD", "R0", 1)
	assert.NoError(t, err, "GetOrderbook should not error for tBTCUSD R0")
	_, err = e.GetOrderbook(t.Context(), "fUSD", "R0", 1)
	assert.NoError(t, err, "GetOrderbook should not error for fUSD R0")
	_, err = e.GetOrderbook(t.Context(), "tBTCUSD", "P0", 1)
	assert.NoError(t, err, "GetOrderbook should not error for tBTCUSD P0")
	_, err = e.GetOrderbook(t.Context(), "fUSD", "P0", 1)
	assert.NoError(t, err, "GetOrderbook should not error for fUSD P0")
	_, err = e.GetOrderbook(t.Context(), "tLINK:UST", "P0", 1)
	assert.NoError(t, err, "GetOrderbook should not error for colon-delimited pair")
}

func TestGetStats(t *testing.T) {
	t.Parallel()
	_, err := e.GetStats(t.Context(), "btcusd")
	assert.NoError(t, err, "GetStats should not error")
}

func TestGetFundingBook(t *testing.T) {
	t.Parallel()
	_, err := e.GetFundingBook(t.Context(), "usd")
	assert.NoError(t, err, "GetFundingBook should not error")
}

func TestGetLends(t *testing.T) {
	t.Parallel()
	_, err := e.GetLends(t.Context(), "usd", nil)
	assert.NoError(t, err, "GetLends should not error")
}

func TestGetCandles(t *testing.T) {
	t.Parallel()
	c, err := e.GetCandles(t.Context(), "fUST", "1D", time.Now().AddDate(0, -1, 0), time.Now(), 10000, true)
	require.NoError(t, err, "GetCandles must not error")
	assert.NotEmpty(t, c, "GetCandles should return some candles")
}

func TestGetLeaderboard(t *testing.T) {
	t.Parallel()
	// Test invalid key
	_, err := e.GetLeaderboard(t.Context(), "", "", "", 0, 0, "", "")
	assert.Error(t, err, "GetLeaderboard should error for invalid key")
	// Test default
	_, err = e.GetLeaderboard(t.Context(),
		LeaderboardUnrealisedProfitInception,
		"1M",
		"tGLOBAL:USD",
		0,
		0,
		"",
		"")
	require.NoError(t, err)
	// Test params
	var result []LeaderboardEntry
	result, err = e.GetLeaderboard(t.Context(),
		LeaderboardUnrealisedProfitInception,
		"1M",
		"tGLOBAL:USD",
		-1,
		1000,
		"1582695181661",
		"1583299981661")
	require.NoError(t, err)
	assert.NotEmpty(t, result, "GetLeaderboard should return leaderboard entries")
}

func TestGetAccountFees(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UpdateAccountBalances(t.Context(), asset.Spot)
	assert.NoError(t, err)
}

func TestGetWithdrawalFee(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetWithdrawalFees(t.Context())
	assert.NoError(t, err, "GetWithdrawalFees should not error")
}

func TestGetAccountSummary(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAccountSummary(t.Context())
	assert.NoError(t, err, "GetAccountSummary should not error")
}

func TestNewDeposit(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.NewDeposit(t.Context(), "blabla", "testwallet", 0)
	assert.NoError(t, err, "NewDeposit should not error for unsupported currency fallback")
	_, err = e.NewDeposit(t.Context(), "bitcoin", "testwallet", 0)
	assert.NoError(t, err, "NewDeposit should not error for bitcoin")
	_, err = e.NewDeposit(t.Context(), "ripple", "", 0)
	assert.NoError(t, err, "NewDeposit should not error when wallet empty")
}

func TestGetKeyPermissions(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetKeyPermissions(t.Context())
	assert.NoError(t, err, "GetKeyPermissions should not error")
}

func TestGetMarginInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMarginInfo(t.Context())
	assert.NoError(t, err, "GetMarginInfo should not error")
}

func TestGetAccountBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAccountBalance(t.Context())
	assert.NoError(t, err, "GetAccountBalance should not error")
}

func TestWalletTransfer(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.WalletTransfer(t.Context(), 0.01, "btc", "bla", "bla")
	assert.NoError(t, err, "WalletTransfer should not error")
}

func TestNewOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.NewOrder(t.Context(),
		"BTCUSD",
		order.Limit.Lower(),
		-1,
		2,
		false,
		true)
	assert.NoError(t, err, "NewOrder should not error")
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()

	_, err := e.UpdateTicker(t.Context(), btcusdPair, asset.Spot)
	assert.NoError(t, common.ExcludeError(err, ticker.ErrBidEqualsAsk), "UpdateTicker may only error about locked markets")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	testexch.UpdatePairsOnce(t, e)

	for _, a := range e.GetAssetTypes(true) {
		avail, err := e.GetAvailablePairs(a)
		require.NoError(t, err, "GetAvailablePairs must not error")

		err = e.CurrencyPairs.StorePairs(a, avail, true)
		require.NoError(t, err, "StorePairs must not error")

		err = e.UpdateTickers(t.Context(), a)
		require.NoError(t, common.ExcludeError(err, ticker.ErrBidEqualsAsk), "UpdateTickers must only error about locked markets")

		// Bitfinex leaves delisted pairs in Available info/conf endpoints
		// We want to assert that most pairs are valid, so we'll check that no more than 5% are erroring
		acceptableThreshold := 95.0
		okay := 0.0
		var errs error
		for _, p := range avail {
			if _, err = ticker.GetTicker(e.Name, p, a); err != nil {
				errs = common.AppendError(errs, err)
			} else {
				okay++
			}
		}
		if !assert.Greaterf(t, okay/float64(len(avail))*100.0, acceptableThreshold, "At least %.f%% of %s tickers should not error", acceptableThreshold, a) {
			assert.NoError(t, errs, "Collection of all the ticker errors")
		}
	}
}

func TestNewOrderMulti(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	newOrder := []PlaceOrder{
		{
			Symbol:   "BTCUSD",
			Amount:   1,
			Price:    1,
			Exchange: "bitfinex",
			Side:     order.Buy.Lower(),
			Type:     order.Limit.Lower(),
		},
	}

	_, err := e.NewOrderMulti(t.Context(), newOrder)
	assert.NoError(t, err, "NewOrderMulti should not error")
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.CancelExistingOrder(t.Context(), 1337)
	assert.NoError(t, err, "CancelExistingOrder should not error")
}

func TestCancelMultipleOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.CancelMultipleOrders(t.Context(), []int64{1337, 1336})
	assert.NoError(t, err, "CancelMultipleOrders should not error")
}

func TestCancelAllOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.CancelAllExistingOrders(t.Context())
	assert.NoError(t, err, "CancelAllExistingOrders should not error")
}

func TestReplaceOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.ReplaceOrder(t.Context(), 1337, "BTCUSD",
		1, 1, true, order.Limit.Lower(), false)
	assert.NoError(t, err, "ReplaceOrder should not error")
}

func TestGetOrderStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetOrderStatus(t.Context(), 1337)
	assert.NoError(t, err, "GetOrderStatus should not error")
}

func TestGetOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetOpenOrders(t.Context())
	assert.NoError(t, err, "GetOpenOrders without filters should not error")
	_, err = e.GetOpenOrders(t.Context(), 1, 2, 3, 4)
	assert.NoError(t, err, "GetOpenOrders with filters should not error")
}

func TestGetActivePositions(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetActivePositions(t.Context())
	assert.NoError(t, err, "GetActivePositions should not error")
}

func TestClaimPosition(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.ClaimPosition(t.Context(), 1337)
	assert.NoError(t, err, "ClaimPosition should not error")
}

func TestGetBalanceHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetBalanceHistory(t.Context(), "USD", time.Time{}, time.Time{}, 1, "deposit")
	assert.NoError(t, err, "GetBalanceHistory should not error")
}

func TestGetMovementHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMovementHistory(t.Context(), "USD", "bitcoin", time.Time{}, time.Time{}, 1)
	assert.NoError(t, err, "GetMovementHistory should not error")
}

func TestMovementHistoryUnmarshalJSON(t *testing.T) {
	t.Parallel()
	deposit := []byte(`[13105603,"ETH","ETHEREUM",null,null,1569348774000,1569348774000,null,null,"COMPLETED",null,null,0.26300954,-0.00135,null,null,"DESTINATION_ADDRESS",null,null,null,"TRANSACTION_ID",null]`)
	var result MovementHistory
	require.NoError(t, json.Unmarshal(deposit, &result))
	stringPtr := func(s string) *string {
		return &s
	}
	exp := MovementHistory{
		ID:                 13105603,
		Currency:           "ETH",
		CurrencyName:       "ETHEREUM",
		MTSStarted:         types.Time(time.Unix(1569348774, 0)),
		MTSUpdated:         types.Time(time.Unix(1569348774, 0)),
		Status:             "COMPLETED",
		Amount:             0.26300954,
		Fees:               -0.00135,
		DestinationAddress: "DESTINATION_ADDRESS",
		TransactionID:      stringPtr("TRANSACTION_ID"),
		TransactionType:    "deposit",
	}
	assert.Equal(t, exp, result, "MovementHistory should unmarshal correctly")
	withdrawal := []byte(`[13293039,"ETH","ETHEREUM",null,null,1574175052000,1574181326000,null,null,"CANCELED",null,null,-0.24,-0.00135,null,null,"DESTINATION_ADDRESS",null,null,null,"TRANSACTION_ID","Purchase of 100 pizzas"]`)
	require.NoError(t, json.Unmarshal(withdrawal, &result))
	exp = MovementHistory{
		ID:                 13293039,
		Currency:           "ETH",
		CurrencyName:       "ETHEREUM",
		MTSStarted:         types.Time(time.Unix(1574175052, 0)),
		MTSUpdated:         types.Time(time.Unix(1574181326, 0)),
		Status:             "CANCELED",
		Amount:             -0.24,
		Fees:               -0.00135,
		DestinationAddress: "DESTINATION_ADDRESS",
		TransactionID:      stringPtr("TRANSACTION_ID"),
		TransactionNote:    stringPtr("Purchase of 100 pizzas"),
		TransactionType:    "withdrawal",
	}
	assert.Equal(t, exp, result, "MovementHistory should unmarshal correctly")
}

func TestNewOffer(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.NewOffer(t.Context(), "BTC", 1, 1, 1, "loan")
	assert.NoError(t, err, "NewOffer should not error")
}

func TestCancelOffer(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.CancelOffer(t.Context(), 1337)
	assert.NoError(t, err, "CancelOffer should not error")
}

func TestGetWithdrawalsHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetWithdrawalsHistory(t.Context(), currency.BTC, asset.Spot)
	assert.NoError(t, err, "GetWithdrawalsHistory should not error")
}

func TestGetOfferStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetOfferStatus(t.Context(), 1337)
	assert.NoError(t, err, "GetOfferStatus should not error")
}

func TestGetActiveCredits(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetActiveCredits(t.Context())
	assert.NoError(t, err, "GetActiveCredits should not error")
}

func TestGetActiveOffers(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetActiveOffers(t.Context())
	assert.NoError(t, err, "GetActiveOffers should not error")
}

func TestGetActiveMarginFunding(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetActiveMarginFunding(t.Context())
	assert.NoError(t, err, "GetActiveMarginFunding should not error")
}

func TestGetUnusedMarginFunds(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetUnusedMarginFunds(t.Context())
	assert.NoError(t, err, "GetUnusedMarginFunds should not error")
}

func TestGetMarginTotalTakenFunds(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMarginTotalTakenFunds(t.Context())
	assert.NoError(t, err, "GetMarginTotalTakenFunds should not error")
}

func TestCloseMarginFunding(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.CloseMarginFunding(t.Context(), 1337)
	assert.NoError(t, err, "CloseMarginFunding should not error")
}

func setFeeBuilder() *exchange.FeeBuilder {
	return &exchange.FeeBuilder{
		Amount:        1,
		FeeType:       exchange.CryptocurrencyTradeFee,
		Pair:          currency.NewPair(currency.BTC, currency.LTC),
		PurchasePrice: 1,
	}
}

func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	feeBuilder := setFeeBuilder()
	_, err := e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err)
	if !sharedtestvalues.AreAPICredentialsSet(e) {
		assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "GetFeeByType should switch to OfflineTradeFee")
	} else {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "GetFeeByType should keep CryptocurrencyTradeFee")
	}
}

func TestGetFee(t *testing.T) {
	feeBuilder := setFeeBuilder()
	t.Parallel()

	if sharedtestvalues.AreAPICredentialsSet(e) {
		// CryptocurrencyTradeFee Basic
		_, err := e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee should not error for standard trade fee")

		// CryptocurrencyTradeFee High quantity
		feeBuilder = setFeeBuilder()
		feeBuilder.Amount = 1000
		feeBuilder.PurchasePrice = 1000
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee should not error for high quantity")

		// CryptocurrencyTradeFee IsMaker
		feeBuilder = setFeeBuilder()
		feeBuilder.IsMaker = true
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee should not error for maker trades")

		// CryptocurrencyTradeFee Negative purchase price
		feeBuilder = setFeeBuilder()
		feeBuilder.PurchasePrice = -1000
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee should not error for negative price")

		// CryptocurrencyWithdrawalFee Basic
		feeBuilder = setFeeBuilder()
		feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee should not error for withdrawal fee")
	}

	// CryptocurrencyDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyDepositFee
	_, err := e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee should not error for crypto deposit")

	// InternationalBankDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	feeBuilder.FiatCurrency = currency.HKD
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee should not error for international deposit")

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.HKD
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee should not error for international withdrawal")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	expectedResult := exchange.AutoWithdrawCryptoWithAPIPermissionText + " & " + exchange.AutoWithdrawFiatWithAPIPermissionText
	withdrawPermissions := e.FormatWithdrawPermissions()
	assert.Equal(t, expectedResult, withdrawPermissions, "FormatWithdrawPermissions should return expected text")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	_, err := e.GetActiveOrders(t.Context(), &getOrdersRequest)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "GetActiveOrders should not error with credentials")
	} else {
		assert.Error(t, err, "GetActiveOrders should error when credentials missing")
	}
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}
	_, err := e.GetOrderHistory(t.Context(), &getOrdersRequest)
	assert.NoError(t, err, "GetOrderHistory should not error")
}

// Any tests below this line have the ability to impact your orders on the exchange. Enable canManipulateRealOrders to run them
// ----------------------------------------------------------------------------------------------------------------------------
func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)
	orderSubmission := &order.Submit{
		Exchange: e.Name,
		Pair: currency.Pair{
			Delimiter: "_",
			Base:      currency.XRP,
			Quote:     currency.USD,
		},
		AssetType: asset.Spot,
		Side:      order.Sell,
		Type:      order.Limit,
		Price:     1000,
		Amount:    20,
		ClientID:  "meowOrder",
	}
	response, err := e.SubmitOrder(t.Context(), orderSubmission)

	if sharedtestvalues.AreAPICredentialsSet(e) {
		require.NoError(t, err, "SubmitOrder must not error with credentials")
		assert.Equal(t, order.New, response.Status, "SubmitOrder response.Status should be order.New")
	} else {
		assert.Error(t, err, "SubmitOrder should error without credentials")
	}
}

func TestCancelExchangeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	currencyPair := currency.NewPair(currency.LTC, currency.BTC)
	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currencyPair,
		AssetType: asset.Spot,
	}

	err := e.CancelOrder(t.Context(), orderCancellation)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelOrder should not error with credentials")
	} else {
		assert.Error(t, err, "CancelOrder should error without credentials")
	}
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	currencyPair := currency.NewPair(currency.LTC, currency.BTC)
	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currencyPair,
		AssetType: asset.Spot,
	}

	resp, err := e.CancelAllOrders(t.Context(), orderCancellation)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		require.NoError(t, err, "CancelAllOrders must not error with credentials")
		assert.Empty(t, resp.Status, "CancelAllOrders response.Status should be empty when successful")
	} else {
		assert.Error(t, err, "CancelAllOrders should error without credentials")
	}
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.ModifyOrder(
		t.Context(),
		&order.Modify{
			OrderID:   "1337",
			AssetType: asset.Spot,
			Pair:      currency.NewBTCUSD(),
		})
	assert.NoError(t, err, "ModifyOrder should not error with valid credentials")
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawCryptoRequest := withdraw.Request{
		Exchange:    e.Name,
		Amount:      -1,
		Currency:    currency.USDT,
		Description: "WITHDRAW IT ALL",
		Crypto: withdraw.CryptoRequest{
			Address: "0x1nv4l1d",
			Chain:   "tetheruse",
		},
	}

	_, err := e.WithdrawCryptocurrencyFunds(t.Context(), &withdrawCryptoRequest)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "WithdrawCryptocurrencyFunds should not error with credentials")
	} else {
		assert.Error(t, err, "WithdrawCryptocurrencyFunds should error without credentials")
	}
}

func TestWithdrawFiat(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawFiatRequest := withdraw.Request{
		Amount:      -1,
		Currency:    currency.USD,
		Description: "WITHDRAW IT ALL",
		Fiat: withdraw.FiatRequest{
			WireCurrency: currency.USD.String(),
		},
	}

	_, err := e.WithdrawFiatFunds(t.Context(), &withdrawFiatRequest)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "WithdrawFiatFunds should not error with credentials")
	} else {
		assert.Error(t, err, "WithdrawFiatFunds should error without credentials")
	}
}

func TestWithdrawInternationalBank(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawFiatRequest := withdraw.Request{
		Amount:      -1,
		Currency:    currency.BTC,
		Description: "WITHDRAW IT ALL",
		Fiat: withdraw.FiatRequest{
			WireCurrency:                  currency.USD.String(),
			RequiresIntermediaryBank:      true,
			IsExpressWire:                 false,
			IntermediaryBankAccountNumber: 12345,
			IntermediaryBankAddress:       "123 Fake St",
			IntermediaryBankCity:          "Tarry Town",
			IntermediaryBankCountry:       "Hyrule",
			IntermediaryBankName:          "Federal Reserve Bank",
			IntermediarySwiftCode:         "Taylor",
		},
	}

	_, err := e.WithdrawFiatFundsToInternationalBank(t.Context(),
		&withdrawFiatRequest)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "WithdrawFiatFundsToInternationalBank should not error with credentials")
	} else {
		assert.Error(t, err, "WithdrawFiatFundsToInternationalBank should error without credentials")
	}
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	if sharedtestvalues.AreAPICredentialsSet(e) {
		_, err := e.GetDepositAddress(t.Context(), currency.USDT, "", "TETHERUSE")
		assert.NoError(t, err, "GetDepositAddress should not error with credentials")
	} else {
		_, err := e.GetDepositAddress(t.Context(), currency.BTC, "deposit", "")
		assert.Error(t, err, "GetDepositAddress should error without credentials")
	}
}

func TestWSAuth(t *testing.T) {
	if !e.Websocket.IsEnabled() {
		t.Skip(websocket.ErrWebsocketNotEnabled.Error())
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if !e.API.AuthenticatedWebsocketSupport {
		t.Skip("Authentecated API support not enabled")
	}
	testexch.SetupWs(t, e)
	require.True(t, e.Websocket.CanUseAuthenticatedEndpoints(), "CanUseAuthenticatedEndpoints must be turned on")

	var resp map[string]any
	catcher := func() (ok bool) {
		select {
		case v := <-e.Websocket.ToRoutine:
			resp, ok = v.(map[string]any)
		default:
		}
		return ok
	}

	if assert.Eventually(t, catcher, sharedtestvalues.WebsocketResponseDefaultTimeout, time.Millisecond*10, "Auth response should arrive") {
		assert.Equal(t, "auth", resp["event"], "websocket auth response event should be auth")
		assert.Equal(t, "OK", resp["status"], "websocket auth status should be OK")
		assert.NotEmpty(t, resp["auth_id"], "websocket auth_id should be populated")
	}
}

func TestGenerateSubscriptions(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup must not error")
	e.Websocket.SetCanUseAuthenticatedEndpoints(true)
	require.True(t, e.Websocket.CanUseAuthenticatedEndpoints(), "CanUseAuthenticatedEndpoints must return true")
	subs, err := e.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")
	exp := subscription.List{}
	for _, baseSub := range e.Features.Subscriptions {
		for _, a := range e.GetAssetTypes(true) {
			if baseSub.Asset != asset.All && baseSub.Asset != a {
				continue
			}
			pairs, err := e.GetEnabledPairs(a)
			require.NoErrorf(t, err, "GetEnabledPairs %s must not error", a)
			for _, p := range pairs.Format(currency.PairFormat{Uppercase: true}) {
				s := baseSub.Clone()
				s.Asset = a
				s.Pairs = currency.Pairs{p}
				prefix := "t"
				if a == asset.MarginFunding {
					prefix = "f"
				}
				switch s.Channel {
				case subscription.TickerChannel:
					s.QualifiedChannel = `{"channel":"ticker","symbol":"` + prefix + p.String() + `"}`
				case subscription.CandlesChannel:
					if a == asset.MarginFunding {
						s.QualifiedChannel = `{"channel":"candles","key":"trade:1m:` + prefix + p.String() + `:p30"}`
					} else {
						s.QualifiedChannel = `{"channel":"candles","key":"trade:1m:` + prefix + p.String() + `"}`
					}
				case subscription.OrderbookChannel:
					s.QualifiedChannel = `{"channel":"book","len":100,"prec":"R0","symbol":"` + prefix + p.String() + `"}`
				case subscription.AllTradesChannel:
					s.QualifiedChannel = `{"channel":"trades","symbol":"` + prefix + p.String() + `"}`
				}
				exp = append(exp, s)
			}
		}
	}
	testsubs.EqualLists(t, exp, subs)
}

// TestWSSubscribe tests Subscribe and Unsubscribe functionality
// See also TestSubscribeReq which covers key and symbol conversion
func TestWSSubscribe(t *testing.T) {
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "TestInstance must not error")
	testexch.SetupWs(t, e)
	err := e.Subscribe(subscription.List{{Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewBTCUSD()}, Asset: asset.Spot}})
	require.NoError(t, err, "Subrcribe must not error")
	catcher := func() (ok bool) {
		i := <-e.Websocket.ToRoutine
		_, ok = i.(*ticker.Price)
		return ok
	}
	assert.Eventually(t, catcher, sharedtestvalues.WebsocketResponseDefaultTimeout, time.Millisecond*10, "Ticker subscribe should deliver a ticker.Price")

	subs, err := e.GetSubscriptions()
	require.NoError(t, err, "GetSubscriptions must not error")
	require.Len(t, subs, 1, "We must only have 1 subscription; subID subscription must have been Removed by subscribeToChan")

	err = e.Subscribe(subscription.List{{Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewBTCUSD()}, Asset: asset.Spot}})
	require.ErrorContains(t, err, "subscribe: dup (code: 10301)", "Duplicate subscription must error correctly")

	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		i := <-e.Websocket.ToRoutine
		e, ok := i.(error)
		require.True(t, ok, "must find an error")
		assert.ErrorContains(t, e, "subscribe: dup (code: 10301)", "error should be correct")
	}, sharedtestvalues.WebsocketResponseDefaultTimeout, time.Millisecond*10, "error response should go to ToRoutine")

	subs, err = e.GetSubscriptions()
	require.NoError(t, err, "GetSubscriptions must not error")
	require.Len(t, subs, 1, "We must only have one subscription after an error attempt")

	err = e.Unsubscribe(subs)
	assert.NoError(t, err, "Unsubscribing should not error")

	chanID, ok := subs[0].Key.(int)
	assert.True(t, ok, "sub.Key should be an int")

	err = e.Unsubscribe(subs)
	assert.ErrorContains(t, err, strconv.Itoa(chanID), "Unsubscribe should contain correct chanId")
	assert.ErrorContains(t, err, "unsubscribe: invalid (code: 10400)", "Unsubscribe should contain correct upstream error")

	err = e.Subscribe(subscription.List{{
		Channel: subscription.TickerChannel,
		Pairs:   currency.Pairs{currency.NewBTCUSD()},
		Asset:   asset.Spot,
		Params:  map[string]any{"key": "tBTCUSD"},
	}})
	assert.ErrorIs(t, err, errParamNotAllowed, "Trying to use a 'key' param should error errParamNotAllowed")
}

// TestSubToMap tests the channel to request map marshalling
func TestSubToMap(t *testing.T) {
	s := &subscription.Subscription{
		Channel:  subscription.CandlesChannel,
		Asset:    asset.Spot,
		Pairs:    currency.Pairs{currency.NewBTCUSD()},
		Interval: kline.OneMin,
	}

	r := subToMap(s, s.Asset, s.Pairs[0])
	assert.Equal(t, "trade:1m:tBTCUSD", r["key"], "key should contain a specific timeframe and no period")

	s.Interval = kline.FifteenMin
	s.Asset = asset.MarginFunding
	s.Params = map[string]any{CandlesPeriodKey: "p30"}

	r = subToMap(s, s.Asset, s.Pairs[0])
	assert.Equal(t, "trade:15m:fBTCUSD:p30", r["key"], "key should contain a period and specific timeframe")

	s.Interval = kline.FifteenMin

	s = &subscription.Subscription{
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{currency.NewPair(currency.BTC, currency.DOGE)},
		Asset:   asset.Spot,
	}
	r = subToMap(s, s.Asset, s.Pairs[0])
	assert.Equal(t, "tBTC:DOGE", r["symbol"], "symbol should use colon delimiter if a currency is > 3 chars")

	s.Pairs = currency.Pairs{currency.NewPair(currency.BTC, currency.LTC)}
	r = subToMap(s, s.Asset, s.Pairs[0])
	assert.Equal(t, "tBTCLTC", r["symbol"], "symbol should not use colon delimiter if both currencies < 3 chars")
}

func TestWSNewOrder(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	testexch.SetupWs(t, e)
	_, err := e.WsNewOrder(t.Context(), &WsNewOrderRequest{
		GroupID: 1,
		Type:    "EXCHANGE LIMIT",
		Symbol:  "tXRPUSD",
		Amount:  -20,
		Price:   1000,
	})
	assert.NoError(t, err)
}

func TestWSCancelOrder(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	testexch.SetupWs(t, e)
	err := e.WsCancelOrder(t.Context(), 1234)
	assert.NoError(t, err, "WsCancelOrder should not error")
}

func TestWSModifyOrder(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	testexch.SetupWs(t, e)
	err := e.WsModifyOrder(t.Context(), &WsUpdateOrderRequest{
		OrderID: 1234,
		Price:   -111,
		Amount:  111,
	})
	assert.NoError(t, err, "WsModifyOrder should not error")
}

func TestWSCancelAllOrders(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	testexch.SetupWs(t, e)
	err := e.WsCancelAllOrders(t.Context())
	assert.NoError(t, err, "WsCancelAllOrders should not error")
}

func TestWSCancelMultiOrders(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	testexch.SetupWs(t, e)
	err := e.WsCancelMultiOrders(t.Context(), []int64{1, 2, 3, 4})
	assert.NoError(t, err, "WsCancelMultiOrders should not error")
}

func TestWSNewOffer(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	testexch.SetupWs(t, e)
	err := e.WsNewOffer(t.Context(), &WsNewOfferRequest{
		Type:   order.Limit.String(),
		Symbol: "fBTC",
		Amount: -10,
		Rate:   10,
		Period: 30,
	})
	assert.NoError(t, err, "WsNewOffer should not error")
}

func TestWSCancelOffer(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	testexch.SetupWs(t, e)
	err := e.WsCancelOffer(t.Context(), 1234)
	assert.NoError(t, err, "WsCancelOffer should not error")
}

func TestWSSubscribedResponse(t *testing.T) {
	ch, err := e.Websocket.Match.Set("subscribe:waiter1", 1)
	assert.NoError(t, err, "Setting a matcher should not error")
	err = e.wsHandleData(t.Context(), []byte(`{"event":"subscribed","channel":"ticker","chanId":224555,"subId":"waiter1","symbol":"tBTCUSD","pair":"BTCUSD"}`))
	if assert.Error(t, err, "Should error if sub is not registered yet") {
		assert.ErrorIs(t, err, websocket.ErrSubscriptionFailure, "Should error SubFailure if sub isn't registered yet")
		assert.ErrorIs(t, err, subscription.ErrNotFound, "Should error SubNotFound if sub isn't registered yet")
		assert.ErrorContains(t, err, "waiter1", "Should error containing subID if")
	}

	err = e.Websocket.AddSubscriptions(e.Websocket.Conn, &subscription.Subscription{Key: "waiter1"})
	require.NoError(t, err, "AddSubscriptions must not error")
	err = e.wsHandleData(t.Context(), []byte(`{"event":"subscribed","channel":"ticker","chanId":224555,"subId":"waiter1","symbol":"tBTCUSD","pair":"BTCUSD"}`))
	assert.NoError(t, err, "wsHandleData should not error")
	if assert.NotEmpty(t, ch, "subscribe matcher should receive response") {
		msg := <-ch
		cID, err := jsonparser.GetInt(msg, "chanId")
		assert.NoError(t, err, "Should get chanId from sub notification without error")
		assert.EqualValues(t, 224555, cID, "Should get the correct chanId through the matcher notification")
	}
}

func TestWSOrderBook(t *testing.T) {
	err := e.Websocket.AddSubscriptions(e.Websocket.Conn, &subscription.Subscription{Key: 23405, Asset: asset.Spot, Pairs: currency.Pairs{btcusdPair}, Channel: subscription.OrderbookChannel})
	require.NoError(t, err, "AddSubscriptions must not error")
	pressXToJSON := `[23405,[[38334303613,9348.8,0.53],[38334308111,9348.8,5.98979404],[38331335157,9344.1,1.28965787],[38334302803,9343.8,0.08230094],[38334279092,9343,0.8],[38334307036,9342.938663676,0.8],[38332749107,9342.9,0.2],[38332277330,9342.8,0.85],[38329406786,9342,0.1432012],[38332841570,9341.947288638,0.3],[38332163238,9341.7,0.3],[38334303384,9341.6,0.324],[38332464840,9341.4,0.5],[38331935870,9341.2,0.5],[38334312082,9340.9,0.02126899],[38334261292,9340.8,0.26763],[38334138680,9340.625455254,0.12],[38333896802,9339.8,0.85],[38331627527,9338.9,1.57863959],[38334186713,9338.9,0.26769],[38334305819,9338.8,2.999],[38334211180,9338.75285796,3.999],[38334310699,9337.8,0.10679883],[38334307414,9337.5,1],[38334179822,9337.1,0.26773],[38334306600,9336.659955102,1.79],[38334299667,9336.6,1.1],[38334306452,9336.6,0.13979771],[38325672859,9336.3,1.25],[38334311646,9336.2,1],[38334258509,9336.1,0.37],[38334310592,9336,1.79],[38334310378,9335.6,1.43],[38334132444,9335.2,0.26777],[38331367325,9335,0.07],[38334310703,9335,0.10680562],[38334298209,9334.7,0.08757301],[38334304857,9334.456899462,0.291],[38334309940,9334.088390727,0.0725],[38334310377,9333.7,1.2868],[38334297615,9333.607784,0.1108],[38334095188,9333.3,0.26785],[38334228913,9332.7,0.40861186],[38334300526,9332.363996604,0.3884],[38334310701,9332.2,0.10680562],[38334303548,9332.005382871,0.07],[38334311798,9331.8,0.41285228],[38334301012,9331.7,1.7952],[38334089877,9331.4,0.2679],[38321942150,9331.2,0.2],[38334310670,9330,1.069],[38334063096,9329.6,0.26796],[38334310700,9329.4,0.10680562],[38334310404,9329.3,1],[38334281630,9329.1,6.57150597],[38334036864,9327.7,0.26801],[38334310702,9326.6,0.10680562],[38334311799,9326.1,0.50220625],[38334164163,9326,0.219638],[38334309722,9326,1.5],[38333051682,9325.8,0.26807],[38334302027,9325.7,0.75],[38334203435,9325.366592,0.32397696],[38321967613,9325,0.05],[38334298787,9324.9,0.3],[38334301719,9324.8,3.6227592],[38331316716,9324.763454646,0.71442],[38334310698,9323.8,0.10680562],[38334035499,9323.7,0.23431017],[38334223472,9322.670551788,0.42150603],[38334163459,9322.560399006,0.143967],[38321825171,9320.8,2],[38334075805,9320.467496148,0.30772633],[38334075800,9319.916732238,0.61457592],[38333682302,9319.7,0.0011],[38331323088,9319.116771762,0.12913],[38333677480,9319,0.0199],[38334277797,9318.6,0.89],[38325235155,9318.041088,1.20249],[38334310910,9317.82382938,1.79],[38334311811,9317.2,0.61079138],[38334311812,9317.2,0.71937652],[38333298214,9317.1,50],[38334306359,9317,1.79],[38325531545,9316.382823951,0.21263],[38333727253,9316.3,0.02316372],[38333298213,9316.1,45],[38333836479,9316,2.135],[38324520465,9315.9,2.7681],[38334307411,9315.5,1],[38330313617,9315.3,0.84455],[38334077770,9315.294024,0.01248397],[38334286663,9315.294024,1],[38325533762,9315.290315394,2.40498],[38334310018,9315.2,3],[38333682617,9314.6,0.0011],[38334304794,9314.6,0.76364676],[38334304798,9314.3,0.69242113],[38332915733,9313.8,0.0199],[38334084411,9312.8,1],[38334311893,9350.1,-1.015],[38334302734,9350.3,-0.26737],[38334300732,9350.8,-5.2],[38333957619,9351,-0.90677089],[38334300521,9351,-1.6457],[38334301600,9351.012829557,-0.0523],[38334308878,9351.7,-2.5],[38334299570,9351.921544,-0.1015],[38334279367,9352.1,-0.26732],[38334299569,9352.411802928,-0.4036],[38334202773,9353.4,-0.02139404],[38333918472,9353.7,-1.96412776],[38334278782,9354,-0.26731],[38334278606,9355,-1.2785],[38334302105,9355.439221251,-0.79191542],[38313897370,9355.569409242,-0.43363],[38334292995,9355.584296,-0.0979],[38334216989,9355.8,-0.03686414],[38333894025,9355.9,-0.26721],[38334293798,9355.936691952,-0.4311],[38331159479,9356,-0.4204022],[38333918888,9356.1,-1.10885563],[38334298205,9356.4,-0.20124428],[38328427481,9356.5,-0.1],[38333343289,9356.6,-0.41034213],[38334297205,9356.6,-0.08835018],[38334277927,9356.741101161,-0.0737],[38334311645,9356.8,-0.5],[38334309002,9356.9,-5],[38334309736,9357,-0.10680107],[38334306448,9357.4,-0.18645275],[38333693302,9357.7,-0.2672],[38332815159,9357.8,-0.0011],[38331239824,9358.2,-0.02],[38334271608,9358.3,-2.999],[38334311971,9358.4,-0.55],[38333919260,9358.5,-1.9972841],[38334265365,9358.5,-1.7841],[38334277960,9359,-3],[38334274601,9359.020969848,-3],[38326848839,9359.1,-0.84],[38334291080,9359.247048,-0.16199869],[38326848844,9359.4,-1.84],[38333680200,9359.6,-0.26713],[38331326606,9359.8,-0.84454],[38334309738,9359.8,-0.10680107],[38331314707,9359.9,-0.2],[38333919803,9360.9,-1.41177599],[38323651149,9361.33417827,-0.71442],[38333656906,9361.5,-0.26705],[38334035500,9361.5,-0.40861586],[38334091886,9362.4,-6.85940815],[38334269617,9362.5,-4],[38323629409,9362.545858872,-2.40497],[38334309737,9362.7,-0.10680107],[38334312380,9362.7,-3],[38325280830,9362.8,-1.75123],[38326622800,9362.8,-1.05145],[38333175230,9363,-0.0011],[38326848745,9363.2,-0.79],[38334308960,9363.206775564,-0.12],[38333920234,9363.3,-1.25318113],[38326848843,9363.4,-1.29],[38331239823,9363.4,-0.02],[38333209613,9363.4,-0.26719],[38334299964,9364,-0.05583123],[38323470224,9364.161816648,-0.12912],[38334284711,9365,-0.21346019],[38334299594,9365,-2.6757062],[38323211816,9365.073132585,-0.21262],[38334312456,9365.1,-0.11167861],[38333209612,9365.2,-0.26719],[38327770474,9365.3,-0.0073],[38334298788,9365.3,-0.3],[38334075803,9365.409831204,-0.30772637],[38334309740,9365.5,-0.10680107],[38326608767,9365.7,-2.76809],[38333920657,9365.7,-1.25848083],[38329594226,9366.6,-0.02587],[38334311813,9366.7,-4.72290945],[38316386301,9367.39258128,-2.37581],[38334302026,9367.4,-4.5],[38334228915,9367.9,-0.81725458],[38333921381,9368.1,-1.72213641],[38333175678,9368.2,-0.0011],[38334301150,9368.2,-2.654604],[38334297208,9368.3,-0.78036466],[38334309739,9368.3,-0.10680107],[38331227515,9368.7,-0.02],[38331184470,9369,-0.003975],[38334203436,9369.319616,-0.32397695],[38334269964,9369.7,-0.5],[38328386732,9370,-4.11759935],[38332719555,9370,-0.025],[38333921935,9370.5,-1.2224398],[38334258511,9370.5,-0.35],[38326848842,9370.8,-0.34],[38333985038,9370.9,-0.8551502],[38334283018,9370.9,-1],[38326848744,9371,-1.34]],5]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for orderbook snapshot")
	pressXToJSON = `[23405,[7617,52.98726298,7617.1,53.601795929999994,-550.9,-0.0674,7617,8318.92961981,8257.8,7500],6]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for orderbook update")
	pressXToJSON = `[23405,[7617,52.98726298,7617.1,53.601795929999994,-550.9,-0.0674,7617,8318.92961981,8257.8,7500]]`
	assert.NotPanics(t, func() { err = e.wsHandleData(t.Context(), []byte(pressXToJSON)) }, "handleWSBookUpdate should not panic when seqNo is not configured to be sent")
	assert.ErrorIs(t, err, errNoSeqNo, "handleWSBookUpdate should send correct error")
}

func TestWSAllTrades(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	err := e.Websocket.AddSubscriptions(e.Websocket.Conn, &subscription.Subscription{Asset: asset.Spot, Pairs: currency.Pairs{btcusdPair}, Channel: subscription.AllTradesChannel, Key: 18788})
	require.NoError(t, err, "AddSubscriptions must not error")
	testexch.FixtureToDataHandler(t, "testdata/wsAllTrades.json", e.wsHandleData)
	close(e.Websocket.DataHandler)
	expJSON := []string{
		`{"TID":"412685577","AssetType":"spot","Side":"BUY","Price":176.3,"Amount":11.1998,"Timestamp":"2020-01-29T03:27:24.802Z"}`,
		`{"TID":"412685578","AssetType":"spot","Side":"SELL","Price":176.29952759,"Amount":5,"Timestamp":"2020-01-29T03:28:04.802Z"}`,
		`{"TID":"412685579","AssetType":"marginFunding","Side":"BUY","Price":0.1244,"Amount":4.2,"Timestamp":"2020-01-29T03:36:45.757Z"}`,
		`{"TID":"5690221201","AssetType":"spot","Side":"BUY","Price":102570,"Amount":0.00991467,"Timestamp":"2024-12-15T04:30:17.719Z"}`,
		`{"TID":"5690221202","AssetType":"spot","Side":"SELL","Price":102560,"Amount":0.01925285,"Timestamp":"2024-12-15T04:30:17.704Z"}`,
		`{"TID":"5690221203","AssetType":"marginFunding","Side":"BUY","Price":102550,"Amount":0.00991467,"Timestamp":"2024-12-15T04:30:18.019Z"}`,
		`{"TID":"5690221204","AssetType":"marginFunding","Side":"SELL","Price":102540,"Amount":0.01925285,"Timestamp":"2024-12-15T04:30:18.094Z"}`,
	}
	require.Len(t, e.Websocket.DataHandler, len(expJSON), "Must see correct number of trades")
	for resp := range e.Websocket.DataHandler {
		switch v := resp.(type) {
		case trade.Data:
			i := 6 - len(e.Websocket.DataHandler)
			exp := trade.Data{
				Exchange:     e.Name,
				CurrencyPair: btcusdPair,
			}
			require.NoErrorf(t, json.Unmarshal([]byte(expJSON[i]), &exp), "Must not error unmarshalling json %d: %s", i, expJSON[i])
			require.Equalf(t, exp, v, "Trade [%d] must be correct", i)
		case error:
			assert.Failf(t, "DataHandler should not receive error", "error: %v", v)
		default:
			assert.Failf(t, "Unexpected type received", "type %T value %v", v, v)
		}
	}
}

func TestWSTickerResponse(t *testing.T) {
	err := e.Websocket.AddSubscriptions(e.Websocket.Conn, &subscription.Subscription{Asset: asset.Spot, Pairs: currency.Pairs{btcusdPair}, Channel: subscription.TickerChannel, Key: 11534})
	require.NoError(t, err, "AddSubscriptions must not error")
	pressXToJSON := `[11534,[61.304,2228.36155358,61.305,1323.2442970500003,0.395,0.0065,61.371,50973.3020771,62.5,57.421]]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for spot ticker")
	pair, err := currency.NewPairFromString("XAUTF0:USTF0")
	require.NoError(t, err)
	err = e.Websocket.AddSubscriptions(e.Websocket.Conn, &subscription.Subscription{Asset: asset.Spot, Pairs: currency.Pairs{pair}, Channel: subscription.TickerChannel, Key: 123412})
	require.NoError(t, err, "AddSubscriptions must not error")
	pressXToJSON = `[123412,[61.304,2228.36155358,61.305,1323.2442970500003,0.395,0.0065,61.371,50973.3020771,62.5,57.421]]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for futures ticker")
	pair, err = currency.NewPairFromString("trade:1m:tXRPUSD")
	require.NoError(t, err)
	err = e.Websocket.AddSubscriptions(e.Websocket.Conn, &subscription.Subscription{Asset: asset.Spot, Pairs: currency.Pairs{pair}, Channel: subscription.TickerChannel, Key: 123413})
	require.NoError(t, err, "AddSubscriptions must not error")
	pressXToJSON = `[123413,[61.304,2228.36155358,61.305,1323.2442970500003,0.395,0.0065,61.371,50973.3020771,62.5,57.421]]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for candle ticker")
	pair, err = currency.NewPairFromString("trade:1m:fZRX:p30")
	require.NoError(t, err)
	err = e.Websocket.AddSubscriptions(e.Websocket.Conn, &subscription.Subscription{Asset: asset.Spot, Pairs: currency.Pairs{pair}, Channel: subscription.TickerChannel, Key: 123414})
	require.NoError(t, err, "AddSubscriptions must not error")
	pressXToJSON = `[123414,[61.304,2228.36155358,61.305,1323.2442970500003,0.395,0.0065,61.371,50973.3020771,62.5,57.421]]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for funding ticker")
}

func TestWSCandleResponse(t *testing.T) {
	err := e.Websocket.AddSubscriptions(e.Websocket.Conn, &subscription.Subscription{Asset: asset.Spot, Pairs: currency.Pairs{btcusdPair}, Channel: subscription.CandlesChannel, Key: 343351})
	require.NoError(t, err, "AddSubscriptions must not error")
	pressXToJSON := `[343351,[[1574698260000,7379.785503,7383.8,7388.3,7379.785503,1.68829482]]]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for candle snapshot")
	pressXToJSON = `[343351,[1574698200000,7399.9,7379.7,7399.9,7371.8,41.63633658]]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for candle update")
}

func TestWSOrderSnapshot(t *testing.T) {
	pressXToJSON := `[0,"os",[[34930659963,null,1574955083558,"tETHUSD",1574955083558,1574955083573,0.201104,0.201104,"EXCHANGE LIMIT",null,null,null,0,"ACTIVE",null,null,120,0,0,0,null,null,null,0,0,null,null,null,"BFX",null,null,null]]]`
	err := e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for order snapshot")
	pressXToJSON = `[0,"oc",[34930659963,null,1574955083558,"tETHUSD",1574955083558,1574955354487,0.201104,0.201104,"EXCHANGE LIMIT",null,null,null,0,"CANCELED",null,null,120,0,0,0,null,null,null,0,0,null,null,null,"BFX",null,null,null]]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for order cancel update")
}

func TestWSNotifications(t *testing.T) {
	pressXToJSON := `[0,"n",[1575282446099,"fon-req",null,null,[41238905,null,null,null,-1000,null,null,null,null,null,null,null,null,null,0.002,2,null,null,null,null,null],null,"SUCCESS","Submitting funding bid of 1000.0 USD at 0.2000 for 2 days."]]`
	err := e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for funding notification")

	pressXToJSON = `[0,"n",[1575287438.515,"on-req",null,null,[1185815098,null,1575287436979,"tETHUSD",1575287438515,1575287438515,-2.5,-2.5,"LIMIT",null,null,null,0,"ACTIVE",null,null,230,0,0,0,null,null,null,0,null,null,null,null,"API>BFX",null,null,null],null,"SUCCESS","Submitting limit sell order for -2.5 ETH."]]`
	err = e.wsHandleData(t.Context(), []byte(pressXToJSON))
	assert.NoError(t, err, "wsHandleData should not error for order notification")
}

func TestWSFundingOfferSnapshotAndUpdate(t *testing.T) {
	pressXToJSON := `[0,"fos",[[41237920,"fETH",1573912039000,1573912039000,0.5,0.5,"LIMIT",null,null,0,"ACTIVE",null,null,null,0.0024,2,0,0,null,0,null]]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding offer snapshot")

	pressXToJSON = `[0,"fon",[41238747,"fUST",1575026670000,1575026670000,5000,5000,"LIMIT",null,null,0,"ACTIVE",null,null,null,0.006000000000000001,30,0,0,null,0,null]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding offer update")
}

func TestWSFundingCreditSnapshotAndUpdate(t *testing.T) {
	pressXToJSON := `[0,"fcs",[[26223578,"fUST",1,1575052261000,1575296187000,350,0,"ACTIVE",null,null,null,0,30,1575052261000,1575293487000,0,0,null,0,null,0,"tBTCUST"],[26223711,"fUSD",-1,1575291961000,1575296187000,180,0,"ACTIVE",null,null,null,0.002,7,1575282446000,1575295587000,0,0,null,0,null,0,"tETHUSD"]]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding credit snapshot")

	pressXToJSON = `[0,"fcu",[26223578,"fUST",1,1575052261000,1575296787000,350,0,"ACTIVE",null,null,null,0,30,1575052261000,1575293487000,0,0,null,0,null,0,"tBTCUST"]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding credit update")
}

func TestWSFundingLoanSnapshotAndUpdate(t *testing.T) {
	pressXToJSON := `[0,"fls",[[2995442,"fUSD",-1,1575291961000,1575295850000,820,0,"ACTIVE",null,null,null,0.002,7,1575282446000,1575295850000,0,0,null,0,null,0]]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding loan snapshot")

	pressXToJSON = `[0,"fln",[2995444,"fUSD",-1,1575298742000,1575298742000,1000,0,"ACTIVE",null,null,null,0.002,7,1575298742000,1575298742000,0,0,null,0,null,0]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding loan update")
}

func TestWSWalletSnapshot(t *testing.T) {
	pressXToJSON := `[0,"ws",[["exchange","SAN",19.76,0,null,null,null]]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for wallet snapshot")
}

func TestWSBalanceUpdate(t *testing.T) {
	const pressXToJSON = `[0,"bu",[4131.85,4131.85]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for balance update")
}

func TestWSMarginInfoUpdate(t *testing.T) {
	const pressXToJSON = `[0,"miu",["base",[-13.014640000000007,0,49331.70267297,49318.68803297,27]]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for margin info update")
}

func TestWSFundingInfoUpdate(t *testing.T) {
	const pressXToJSON = `[0,"fiu",["sym","tETHUSD",[149361.09689202666,149639.26293509,830.0182168075556,895.0658432466332]]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding info update")
}

func TestWSFundingTrade(t *testing.T) {
	pressXToJSON := `[0,"fte",[636854,"fUSD",1575282446000,41238905,-1000,0.002,7,null]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding trade execution")

	pressXToJSON = `[0,"ftu",[636854,"fUSD",1575282446000,41238905,-1000,0.002,7,null]]`
	assert.NoError(t, e.wsHandleData(t.Context(), []byte(pressXToJSON)), "wsHandleData should not error for funding trade update")
}

func TestGetHistoricCandles(t *testing.T) {
	startTime := time.Now().Add(-time.Hour * 24)
	endTime := time.Now().Add(-time.Hour * 20)

	_, err := e.GetHistoricCandles(t.Context(), btcusdPair, asset.Spot, kline.OneHour, startTime, endTime)
	require.NoError(t, err)
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	startTime := time.Now().Add(-time.Hour * 24)
	endTime := time.Now().Add(-time.Hour * 20)

	_, err := e.GetHistoricCandlesExtended(t.Context(), btcusdPair, asset.Spot, kline.OneHour, startTime, endTime)
	require.NoError(t, err)
}

func TestFixCasing(t *testing.T) {
	ret, err := e.fixCasing(btcusdPair, asset.Spot)
	require.NoError(t, err)
	assert.Equal(t, "tBTCUSD", ret)

	pair, err := currency.NewPairFromString("TBTCUSD")
	require.NoError(t, err)
	ret, err = e.fixCasing(pair, asset.Spot)
	require.NoError(t, err)
	assert.Equal(t, "tBTCUSD", ret)

	pair, err = currency.NewPairFromString("tBTCUSD")
	require.NoError(t, err)
	ret, err = e.fixCasing(pair, asset.Spot)
	require.NoError(t, err)
	assert.Equal(t, "tBTCUSD", ret)

	ret, err = e.fixCasing(btcusdPair, asset.Margin)
	require.NoError(t, err)
	assert.Equal(t, "tBTCUSD", ret)

	ret, err = e.fixCasing(btcusdPair, asset.Spot)
	require.NoError(t, err)
	assert.Equal(t, "tBTCUSD", ret)

	pair, err = currency.NewPairFromString("FUNETH")
	require.NoError(t, err)
	ret, err = e.fixCasing(pair, asset.Spot)
	require.NoError(t, err)
	assert.Equal(t, "tFUNETH", ret)

	pair, err = currency.NewPairFromString("TNBUSD")
	require.NoError(t, err)
	ret, err = e.fixCasing(pair, asset.Spot)
	require.NoError(t, err)
	assert.Equal(t, "tTNBUSD", ret)

	pair, err = currency.NewPairFromString("tTNBUSD")
	require.NoError(t, err)
	ret, err = e.fixCasing(pair, asset.Spot)
	require.NoError(t, err)
	assert.Equal(t, "tTNBUSD", ret)

	pair, err = currency.NewPairFromStrings("fUSD", "")
	require.NoError(t, err)
	ret, err = e.fixCasing(pair, asset.MarginFunding)
	require.NoError(t, err)
	assert.Equal(t, "fUSD", ret)

	pair, err = currency.NewPairFromStrings("USD", "")
	require.NoError(t, err)
	ret, err = e.fixCasing(pair, asset.MarginFunding)
	require.NoError(t, err)
	assert.Equal(t, "fUSD", ret)

	pair, err = currency.NewPairFromStrings("FUSD", "")
	require.NoError(t, err)
	ret, err = e.fixCasing(pair, asset.MarginFunding)
	require.NoError(t, err)
	assert.Equal(t, "fUSD", ret)

	_, err = e.fixCasing(currency.NewPair(currency.EMPTYCODE, currency.BTC), asset.MarginFunding)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.fixCasing(currency.NewPair(currency.BTC, currency.EMPTYCODE), asset.MarginFunding)
	require.NoError(t, err)

	_, err = e.fixCasing(currency.EMPTYPAIR, asset.MarginFunding)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
}

func TestFormatExchangeKlineInterval(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		interval kline.Interval
		output   string
	}{
		{
			kline.OneMin,
			"1m",
		},
		{
			kline.OneDay,
			"1D",
		},
		{
			kline.OneWeek,
			"7D",
		},
		{
			kline.OneWeek * 2,
			"14D",
		},
	} {
		t.Run(tc.interval.String(), func(t *testing.T) {
			t.Parallel()
			ret, err := e.FormatExchangeKlineInterval(tc.interval)
			require.NoError(t, err, "FormatExchangeKlineInterval must not error")
			assert.Equal(t, tc.output, ret)
		})
	}
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetRecentTrades(t.Context(), btcusdPair, asset.Spot)
	assert.NoError(t, err, "GetRecentTrades should not error for BTCUSD spot")

	currencyPair, err := currency.NewPairFromString("USD")
	require.NoError(t, err)
	_, err = e.GetRecentTrades(t.Context(), currencyPair, asset.Margin)
	assert.NoError(t, err, "GetRecentTrades should not error for USD margin pair")
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricTrades(t.Context(), btcusdPair, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	assert.NoError(t, err, "GetHistoricTrades should not error for short range")
	_, err = e.GetHistoricTrades(t.Context(), btcusdPair, asset.Spot, time.Now().Add(-time.Hour*100), time.Now().Add(-time.Hour*99))
	assert.NoError(t, err, "GetHistoricTrades should not error for long range")
}

var testOb = orderbook.Book{
	Asks: []orderbook.Level{
		{Price: 0.05005, Amount: 0.00000500},
		{Price: 0.05010, Amount: 0.00000500},
		{Price: 0.05015, Amount: 0.00000500},
		{Price: 0.05020, Amount: 0.00000500},
		{Price: 0.05025, Amount: 0.00000500},
		{Price: 0.05030, Amount: 0.00000500},
		{Price: 0.05035, Amount: 0.00000500},
		{Price: 0.05040, Amount: 0.00000500},
		{Price: 0.05045, Amount: 0.00000500},
		{Price: 0.05050, Amount: 0.00000500},
	},
	Bids: []orderbook.Level{
		{Price: 0.05000, Amount: 0.00000500},
		{Price: 0.04995, Amount: 0.00000500},
		{Price: 0.04990, Amount: 0.00000500},
		{Price: 0.04980, Amount: 0.00000500},
		{Price: 0.04975, Amount: 0.00000500},
		{Price: 0.04970, Amount: 0.00000500},
		{Price: 0.04965, Amount: 0.00000500},
		{Price: 0.04960, Amount: 0.00000500},
		{Price: 0.04955, Amount: 0.00000500},
		{Price: 0.04950, Amount: 0.00000500},
	},
}

func TestChecksum(t *testing.T) {
	require.NoError(t, validateCRC32(&testOb, 190468240), "validateCRC32 must not error for known checksum")
}

func TestReOrderbyID(t *testing.T) {
	asks := []orderbook.Level{
		{ID: 4, Price: 100, Amount: 0.00000500},
		{ID: 3, Price: 100, Amount: 0.00000500},
		{ID: 2, Price: 100, Amount: 0.00000500},
		{ID: 1, Price: 100, Amount: 0.00000500},
		{ID: 5, Price: 101, Amount: 0.00000500},
		{ID: 6, Price: 102, Amount: 0.00000500},
		{ID: 8, Price: 103, Amount: 0.00000500},
		{ID: 7, Price: 103, Amount: 0.00000500},
		{ID: 9, Price: 104, Amount: 0.00000500},
		{ID: 10, Price: 105, Amount: 0.00000500},
	}
	reOrderByID(asks)

	for i := range asks {
		assert.Equalf(t, int64(i+1), asks[i].ID, "reOrderByID should order asks by ID")
	}

	bids := []orderbook.Level{
		{ID: 4, Price: 100, Amount: 0.00000500},
		{ID: 3, Price: 100, Amount: 0.00000500},
		{ID: 2, Price: 100, Amount: 0.00000500},
		{ID: 1, Price: 100, Amount: 0.00000500},
		{ID: 5, Price: 99, Amount: 0.00000500},
		{ID: 6, Price: 98, Amount: 0.00000500},
		{ID: 8, Price: 97, Amount: 0.00000500},
		{ID: 7, Price: 97, Amount: 0.00000500},
		{ID: 9, Price: 96, Amount: 0.00000500},
		{ID: 10, Price: 95, Amount: 0.00000500},
	}
	reOrderByID(bids)

	for i := range bids {
		assert.Equalf(t, int64(i+1), bids[i].ID, "reOrderByID should order bids by ID")
	}
}

func TestPopulateAcceptableMethods(t *testing.T) {
	t.Parallel()
	if acceptableMethods.loaded() {
		// we may have been loaded from another test, so reset
		acceptableMethods.m.Lock()
		acceptableMethods.a = make(map[string][]string)
		acceptableMethods.m.Unlock()
		assert.False(t, acceptableMethods.loaded(), "acceptableMethods should be empty after reset")
	}
	require.NoError(t, e.PopulateAcceptableMethods(t.Context()))
	assert.True(t, acceptableMethods.loaded(), "acceptable method store should be loaded")
	if methods := acceptableMethods.lookup(currency.NewCode("UST")); len(methods) == 0 {
		assert.Fail(t, "acceptableMethods.lookup should return USDT methods")
	}
	if methods := acceptableMethods.lookup(currency.NewCode("ASdasdasdasd")); len(methods) != 0 {
		assert.Fail(t, "acceptableMethods.lookup should return no methods for unknown code")
	}
	// since we're already loaded, this will return nil
	require.NoError(t, e.PopulateAcceptableMethods(t.Context()), "PopulateAcceptableMethods must not error when already loaded")
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	r, err := e.GetAvailableTransferChains(t.Context(), currency.USDT)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(r), 2, "GetAvailableTransferChains should return at least two chains for USDT")
}

func TestAccetableMethodStore(t *testing.T) {
	t.Parallel()
	var a acceptableMethodStore
	assert.False(t, a.loaded(), "acceptableMethodStore.loaded should return false when empty")
	data := map[string][]string{
		"BITCOIN": {"BTC"},
		"TETHER1": {"UST"},
		"TETHER2": {"UST"},
	}
	a.load(data)
	assert.True(t, a.loaded(), "acceptableMethodStore.load should mark store as loaded")
	if name := a.lookup(currency.NewCode("BTC")); assert.Len(t, name, 1, "lookup should return one BTC method") {
		assert.Equal(t, "BITCOIN", name[0], "lookup BTC should return BITCOIN method")
	}
	name := a.lookup(currency.NewCode("UST"))
	assert.ElementsMatch(t, []string{"TETHER1", "TETHER2"}, name, "lookup UST should return tether methods")
	assert.Empty(t, a.lookup(currency.NewCode("PANDA_HORSE")), "lookup should return empty slice for unknown code")
}

func TestGetSiteListConfigData(t *testing.T) {
	t.Parallel()

	_, err := e.GetSiteListConfigData(t.Context(), "")
	require.ErrorIs(t, err, errSetCannotBeEmpty)

	pairs, err := e.GetSiteListConfigData(t.Context(), bitfinexSecuritiesPairs)
	require.NoError(t, err)
	require.NotEmpty(t, pairs, "GetSiteListConfigData must return pairs")
}

func TestGetSiteInfoConfigData(t *testing.T) {
	t.Parallel()
	for _, assetType := range []asset.Item{asset.Spot, asset.Futures} {
		pairs, err := e.GetSiteInfoConfigData(t.Context(), assetType)
		assert.NoErrorf(t, err, "GetSiteInfoConfigData should not error for %s", assetType)
		assert.NotEmptyf(t, pairs, "GetSiteInfoConfigData should return pairs for %s", assetType)
	}
}

func TestOrderUpdate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.OrderUpdate(t.Context(), "1234", "", "", 1, 1, 1)
	assert.NoError(t, err, "OrderUpdate should not error")
}

func TestGetInactiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetInactiveOrders(t.Context(), "tBTCUSD")
	assert.NoError(t, err, "GetInactiveOrders should not error for default params")

	_, err = e.GetInactiveOrders(t.Context(), "tBTCUSD", 1, 2, 3, 4)
	assert.NoError(t, err, "GetInactiveOrders should not error with filters")
}

func TestCancelMultipleOrdersV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelMultipleOrdersV2(t.Context(), 1337, 0, 0, time.Time{}, false)
	assert.NoError(t, err, "CancelMultipleOrdersV2 should not error")
}

// TestGetErrResp unit tests the helper func getErrResp
func TestGetErrResp(t *testing.T) {
	t.Parallel()
	fixture, err := os.Open("testdata/getErrResp.json")
	require.NoError(t, err, "os.Open must succeed for fixture")
	s := bufio.NewScanner(fixture)
	seen := 0
	for s.Scan() {
		testErr := e.getErrResp(s.Bytes())
		seen++
		switch seen {
		case 1: // no event
			assert.ErrorIs(t, testErr, common.ErrParsingWSField, "Message with no event should get correct error type")
			assert.ErrorContains(t, testErr, "'event'", "Message with no event error should contain missing field name")
			assert.ErrorContains(t, testErr, "nightjar", "Message with no event error should contain the message")
		case 2: // with {} for event
			assert.NoError(t, testErr, "Message with '{}' for event field should not error")
		case 3: // event != 'error'
			assert.NoError(t, testErr, "Message with non-'error' event field should not error")
		case 4: // event="error"
			assert.ErrorIs(t, testErr, common.ErrUnknownError, "error without a message should throw unknown error")
			assert.ErrorContains(t, testErr, "code: 0", "error without a code should throw code 0")
		case 5: // Fully formatted
			assert.ErrorContains(t, testErr, "redcoats", "message field should be in the error")
			assert.ErrorContains(t, testErr, "code: 42", "code field should be in the error")
		}
	}
	assert.NoError(t, s.Err(), "Fixture Scanner should not error")
	assert.NoError(t, fixture.Close(), "Closing the fixture file should not error")
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		require.NoErrorf(t, err, "cannot get pairs for %s", a)
		require.NotEmptyf(t, pairs, "no pairs for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		require.NoError(t, err)
		assert.NotEmpty(t, resp)
	}
}
