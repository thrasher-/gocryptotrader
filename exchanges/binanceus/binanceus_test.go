package binanceus

import (
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

// Please supply your own keys here to test authenticated endpoints
const (
	apiKey                  = ""
	apiSecret               = ""
	canManipulateRealOrders = false
)

var (
	e               = &Exchange{}
	testPairMapping = currency.NewBTCUSDT()
	// this lock guards against orderbook tests race
	binanceusOrderBookLock = &sync.Mutex{}
)

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Binanceus Setup error: %s", err)
	}

	if apiKey != "" && apiSecret != "" {
		e.API.AuthenticatedSupport = true
		e.API.AuthenticatedWebsocketSupport = true
		e.SetCredentials(apiKey, apiSecret, "", "", "", "")
	}

	e.WebsocketResponseMaxLimit = exchange.DefaultWebsocketResponseMaxLimit
	os.Exit(m.Run())
}

func TestServerTime(t *testing.T) {
	t.Parallel()
	_, err := e.GetServerTime(t.Context(), asset.Spot)
	require.NoError(t, err, "GetServerTime must not error")
}

func TestServerStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSystemStatus(t.Context())
	require.NoError(t, err, "GetSystemStatus must not error")
}

func TestGetExchangeInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetExchangeInfo(t.Context())
	require.NoError(t, err, "GetExchangeInfo must not error")
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	r, err := e.UpdateTicker(t.Context(), testPairMapping, asset.Spot)
	require.NoError(t, err, "UpdateTicker must not error")
	assert.Equal(t, currency.BTC, r.Pair.Base, "UpdateTicker should set base to BTC")
	assert.Equal(t, currency.USDT, r.Pair.Quote, "UpdateTicker should set quote to USDT")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	err := e.UpdateTickers(t.Context(), asset.Spot)
	require.NoError(t, err, "UpdateTickers must not error")
}

func TestUpdateOrderBook(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateOrderbook(t.Context(), testPairMapping, asset.Spot)
	assert.NoError(t, err, "UpdateOrderbook should not error")
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FetchTradablePairs(t.Context(), asset.Spot)
	assert.NoError(t, err, "FetchTradablePairs should not error")
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	err := e.UpdateTradablePairs(t.Context())
	assert.NoError(t, err, "UpdateTradablePairs should not error")
}

func TestUpdateAccountBalances(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UpdateAccountBalances(t.Context(), asset.Spot)
	require.NoError(t, err)
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	pair := currency.Pair{Base: currency.BTC, Quote: currency.USD}
	_, err := e.GetRecentTrades(t.Context(), pair, asset.Spot)
	require.NoError(t, err, "GetRecentTrades must not error")
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	p := currency.NewBTCUSDT()
	start := time.Now().Add(-time.Hour * 24 * 90).Truncate(time.Minute) // 3 months ago
	end := start.Add(15 * time.Minute)
	result, err := e.GetHistoricTrades(t.Context(), p, asset.Spot, start, end)
	require.NoError(t, err, "GetHistoricTrades must not error")
	assert.NotEmpty(t, result, "GetHistoricTrades should have trades")
	for _, r := range result {
		require.WithinRange(t, r.Timestamp, start, end, "All trades must be within time range")
	}
}

func TestGetFeeByType(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		IsMaker: true,
		Pair:    currency.NewPair(currency.USD, currency.BTC),
		FeeType: exchange.CryptocurrencyTradeFee,
	})
	require.NoError(t, err, "GetFeeByType must not return error for maker trade fee")
	_, err = e.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		IsMaker: true,
		Pair:    currency.NewPair(currency.USD, currency.BTC),
		FeeType: exchange.CryptocurrencyWithdrawalFee,
	})
	require.NoError(t, err, "GetFeeByType must not return error for withdrawal fee")
}

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)
	orderSubmission := &order.Submit{
		Pair: currency.Pair{
			Base:  currency.XRP,
			Quote: currency.USD,
		},
		AssetType: asset.Spot,
		Side:      order.Sell,
		Type:      order.Limit,
		Price:     1000,
		Amount:    20,
		ClientID:  "binanceSamOrder",
		Exchange:  e.Name,
	}
	response, err := e.SubmitOrder(t.Context(), orderSubmission)
	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	if credsSet {
		if err != nil && strings.Contains(err.Error(), "{\"code\":-1013,\"msg\":\"Market is closed.\"") {
			t.Skip("SubmitOrder should be skipped when market closed")
		}
		require.NoError(t, err, "SubmitOrder must not return error when credentials set")
		require.Equal(t, order.Filled, response.Status, "SubmitOrder must return filled status when order placed")
		return
	}
	assert.Error(t, err, "SubmitOrder should error when credentials unset")
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()

	pair := currency.NewBTCUSD()
	err := e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.Spot,
		OrderID:   "1337",
	})
	require.ErrorIs(t, err, errMissingCurrencySymbol)
	err = e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.Futures,
		OrderID:   "69",
		Pair:      pair,
	})
	require.ErrorIs(t, err, asset.ErrNotSupported)
	err = e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.Spot,
		OrderID:   "",
		Pair:      pair,
	})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	cancellationOrder := &order.Cancel{
		OrderID:   "1",
		Pair:      pair,
		AssetType: asset.Spot,
	}
	err = e.CancelOrder(t.Context(), cancellationOrder)
	assert.ErrorContains(t, err, "Unknown order sent.")
}

func TestCancelAllOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	orderCancellation := &order.Cancel{
		Pair:      currency.NewPair(currency.LTC, currency.BTC),
		AssetType: asset.Spot,
	}
	_, err := e.CancelAllOrders(t.Context(), orderCancellation)
	require.NoError(t, err, "CancelAllOrders must not return error when credentials set")
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	tradablePairs, err := e.FetchTradablePairs(t.Context(), asset.Spot)
	require.NoError(t, err, "FetchTradablePairs must not return error when credentials set")
	require.NotEmpty(t, tradablePairs, "FetchTradablePairs must return tradable pairs")
	_, err = e.GetOrderInfo(t.Context(), "123", tradablePairs[0], asset.Spot)
	require.Error(t, err, "GetOrderInfo must return error for missing order")
	assert.Contains(t, err.Error(), "Order does not exist.", "GetOrderInfo should include missing order message")
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	_, err := e.GetDepositAddress(t.Context(), currency.EMPTYCODE, "", currency.BNB.String())
	assert.ErrorIs(t, err, errMissingRequiredArgumentCoin)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err = e.GetDepositAddress(t.Context(), currency.USDT, "", currency.BNB.String())
	assert.NoError(t, err)
}

func TestGetWithdrawalHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)
	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	_, err := e.GetWithdrawalsHistory(t.Context(), currency.ETH, asset.Spot)
	if credsSet {
		require.NoError(t, err, "GetWithdrawalsHistory must not return error when credentials set")
		return
	}
	assert.Error(t, err, "GetWithdrawalsHistory should error when credentials unset")
}

func TestWithdrawFiat(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.WithdrawFiat(t.Context(), &WithdrawFiatRequestParams{
		PaymentChannel: "SILVERGATE",
		PaymentAccount: "myaccount",
		PaymentMethod:  "SEN",
		Amount:         1,
	})
	if err != nil {
		assert.Contains(t, err.Error(), "You are not authorized to execute this request.", "WithdrawFiat error should indicate unauthorized access")
	}
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}
	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	_, err := e.GetActiveOrders(t.Context(), &getOrdersRequest)
	if credsSet {
		require.NoError(t, err, "GetActiveOrders must not return error when credentials set")
		return
	}
	assert.Error(t, err, "GetActiveOrders should error when credentials unset")
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	withdrawCryptoRequest := withdraw.Request{
		Exchange:    e.Name,
		Amount:      -1,
		Currency:    currency.BTC,
		Description: "WITHDRAW IT ALL",
		Crypto: withdraw.CryptoRequest{
			Address: core.BitcoinDonationAddress,
			Chain:   "BSC",
		},
	}
	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	_, err := e.WithdrawCryptocurrencyFunds(t.Context(), &withdrawCryptoRequest)
	if credsSet {
		assert.Equal(t, errAmountValueMustBeGreaterThan0.Error(), err.Error(), "WithdrawCryptocurrencyFunds error should state amount is greater than 0")
	} else {
		assert.Error(t, err, "WithdrawCryptocurrencyFunds should error when credentials unset")
	}
	withdrawCryptoRequest.Amount = 1
	_, err = e.WithdrawCryptocurrencyFunds(t.Context(), &withdrawCryptoRequest)
	if err != nil {
		assert.Contains(t, err.Error(), "You are not authorized to execute this request.", "WithdrawCryptocurrencyFunds error should indicate unauthorized access")
	}
}

func TestGetFee(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	feeBuilder := &exchange.FeeBuilder{
		Amount:        1,
		FeeType:       exchange.CryptocurrencyTradeFee,
		Pair:          currency.NewPair(currency.BTC, currency.LTC),
		PurchasePrice: 1,
	}
	_, err := e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFeeByType must not return error for trade fee")
	withdrawalFeeBuilder := &exchange.FeeBuilder{
		Amount:        1,
		FeeType:       exchange.CryptocurrencyWithdrawalFee,
		Pair:          currency.NewPair(currency.BTC, currency.LTC),
		PurchasePrice: 1,
	}
	_, err = e.GetFeeByType(t.Context(), withdrawalFeeBuilder)
	require.NoError(t, err, "GetFeeByType must not return error for withdrawal fee")
	offlineFeeTradeBuilder := &exchange.FeeBuilder{
		Amount:        1,
		FeeType:       exchange.OfflineTradeFee,
		Pair:          currency.NewPair(currency.BTC, currency.LTC),
		PurchasePrice: 1,
	}
	_, err = e.GetFeeByType(t.Context(), offlineFeeTradeBuilder)
	require.NoError(t, err, "GetFeeByType must not return error for offline trade fee")
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	pair := currency.NewBTCUSDT()
	startTime := time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2021, 2, 15, 0, 0, 0, 0, time.UTC)

	_, err := e.GetHistoricCandles(t.Context(), pair, asset.Spot, kline.Interval(time.Hour*5), startTime, endTime)
	require.ErrorIs(t, err, kline.ErrRequestExceedsExchangeLimits)

	_, err = e.GetHistoricCandles(t.Context(), pair, asset.Spot, kline.OneDay, startTime, endTime)
	require.NoError(t, err, "GetHistoricCandles must not return error for one day interval")
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	pair := currency.NewBTCUSDT()
	startTime := time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2021, 2, 15, 0, 0, 0, 0, time.UTC)

	_, err := e.GetHistoricCandlesExtended(t.Context(), pair, asset.Spot, kline.OneDay, startTime, endTime)
	require.NoError(t, err, "GetHistoricCandlesExtended must not return error for one day interval")

	startTime = time.Now().Add(-time.Hour * 30)
	endTime = time.Now()

	_, err = e.GetHistoricCandlesExtended(t.Context(), pair, asset.Spot, kline.FourHour, startTime, endTime)
	require.NoError(t, err, "GetHistoricCandlesExtended must not return error for four hour interval")
}

/************************************************************************/

// TestGetMostRecentTrades -- test most recent trades end-point
func TestGetMostRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetMostRecentTrades(t.Context(), RecentTradeRequestParams{
		Symbol: currency.NewBTCUSDT(),
		Limit:  15,
	})
	require.NoError(t, err, "GetMostRecentTrades must not error")
}

func TestGetHistoricalTrades(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetHistoricalTrades(t.Context(), HistoricalTradeParams{
		Symbol: "BTCUSDT",
		Limit:  5,
		FromID: 0,
	})
	require.NoError(t, err, "GetHistoricalTrades must not error")
}

func TestGetAggregateTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetAggregateTrades(t.Context(),
		&AggregatedTradeRequestParams{
			Symbol: currency.NewBTCUSDT(),
			Limit:  5,
		})
	require.NoError(t, err, "GetAggregateTrades must not error")
}

func TestGetOrderBookDepth(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderBookDepth(t.Context(), currency.NewBTCUSDT(), 1000)
	assert.NoError(t, err)
}

func TestGetCandlestickData(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotKline(t.Context(), &KlinesRequestParams{
		Symbol:    currency.NewBTCUSDT(),
		Interval:  kline.FiveMin.Short(),
		Limit:     24,
		StartTime: time.Unix(1577836800, 0),
		EndTime:   time.Unix(1580515200, 0),
	})
	require.NoError(t, err, "GetSpotKline must not error")
}

func TestGetPriceDatas(t *testing.T) {
	t.Parallel()
	_, err := e.GetPriceDatas(t.Context())
	require.NoError(t, err, "GetPriceDatas must not error")
}

func TestGetSinglePriceData(t *testing.T) {
	t.Parallel()
	_, err := e.GetSinglePriceData(t.Context(), currency.Pair{
		Base:  currency.BTC,
		Quote: currency.USDT,
	})
	require.NoError(t, err, "GetSinglePriceData must not error")
}

func TestGetAveragePrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetAveragePrice(t.Context(), currency.NewBTCUSDT())
	require.NoError(t, err, "GetAveragePrice must not error")
}

func TestGetBestPrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetBestPrice(t.Context(), currency.NewBTCUSDT())
	require.NoError(t, err, "GetBestPrice must not error")
}

func TestGetPriceChangeStats(t *testing.T) {
	t.Parallel()
	_, err := e.GetPriceChangeStats(t.Context(), currency.NewBTCUSDT())
	require.NoError(t, err, "GetPriceChangeStats must not error")
}

func TestGetTickers(t *testing.T) {
	t.Parallel()
	_, err := e.GetTickers(t.Context())
	require.NoError(t, err, "GetTickers must not error")
}

func TestGetAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAccount(t.Context())
	require.NoError(t, err, "GetAccount must not error")
}

func TestGetUserAccountStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetUserAccountStatus(t.Context(), 3000)
	require.NoError(t, err, "GetUserAccountStatus must not error")
}

func TestGetUserAPITradingStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetUserAPITradingStatus(t.Context(), 3000)
	require.NoError(t, err, "GetUserAPITradingStatus must not error")
}

func TestGetTradeFee(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetTradeFee(t.Context(), 3000, "BTC-USDT")
	require.NoError(t, err, "GetTradeFee must not error")
}

func TestGetAssetDistributionHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAssetDistributionHistory(t.Context(), "", 0, 0, 3000)
	require.NoError(t, err, "GetAssetDistributionHistory must not error")
}

func TestGetMasterAccountTotalUSDValue(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMasterAccountTotalUSDValue(t.Context(), "", 0, 0)
	if err != nil {
		assert.Contains(t, err.Error(), "Sub-account function is not enabled.", "GetMasterAccountTotalUSDValue error should indicate disabled sub-account function")
	}
}

func TestGetSubaccountStatusList(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubaccountStatusList(t.Context(), "")
	assert.ErrorIs(t, err, errMissingSubAccountEmail)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err = e.GetSubaccountStatusList(t.Context(), "someone@thrasher.corp")
	assert.ErrorContains(t, err, "Sub-account function is not enabled.")
}

func TestGetSubAccountDepositAddress(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountDepositAddress(t.Context(), SubAccountDepositAddressRequestParams{})
	assert.ErrorIs(t, err, errMissingSubAccountEmail)
	_, err = e.GetSubAccountDepositAddress(t.Context(), SubAccountDepositAddressRequestParams{
		Email: "someone@thrasher.io",
	})
	assert.ErrorIs(t, err, errMissingCurrencyCoin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err = e.GetSubAccountDepositAddress(t.Context(), SubAccountDepositAddressRequestParams{
		Email: "someone@thrasher.io",
		Coin:  currency.BTC,
	})
	assert.ErrorContains(t, err, "This parent sub have no relation")
}

var subAccountDepositHistoryItemJSON = `{
	"amount": "9.9749",
	"coin": "BTC", 
	"network": "btc",
	"status": 4, 
	"address": "bc1qxurvdd7tzn09agdvg3j8xpm3f7e978y07wg83s",
	"addressTag": "",
	"txId": "0x1b4b8c8090d15e3c1b0476b1c19118b1f00066e01de567cd7bc5b6e9c100193f",
	"insertTime": 1652942429211,
	"transferType": 0,
	"confirmTimes": "0/0"
}`

func TestGetSubAccountDepositHistory(t *testing.T) {
	t.Parallel()
	var resp SubAccountDepositItem
	require.NoError(t, json.Unmarshal([]byte(subAccountDepositHistoryItemJSON), &resp))
	_, err := e.GetSubAccountDepositHistory(t.Context(), "", currency.BTC, 1, time.Time{}, time.Time{}, 0, 0)
	assert.ErrorIs(t, err, errMissingSubAccountEmail)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err = e.GetSubAccountDepositHistory(t.Context(), "someone@thrasher.io", currency.BTC, 1, time.Time{}, time.Time{}, 0, 0)
	assert.ErrorContains(t, err, "This parent sub have no relation")
}

var subaccountItemJSON = `{
	"email": "123@test.com",
	"status": "enabled",
	"activated": true,
	"mobile": "91605290",
	"gAuth": true,
	"createTime": 1544433328000
}`

func TestGetSubaccountInformation(t *testing.T) {
	t.Parallel()
	var resp SubAccount
	require.NoError(t, json.Unmarshal([]byte(subaccountItemJSON), &resp), "SubAccount JSON must unmarshal")
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSubaccountInformation(t.Context(), 1, 100, "", "")
	if err != nil {
		assert.Contains(t, err.Error(), "Sub-account function is not enabled.", "GetSubaccountInformation error should indicate disabled sub-account function")
	}
}

var referalRewardHistoryResponse = `{
    "total": 1,
    "rows": [
        {
            "userId": 350991652,
            "rewardAmount": "8",
            "receiveDateTime": 1651131084091,
            "rewardType": "USD"
        }
    ]
}`

func TestGetReferralRewardHistory(t *testing.T) {
	t.Parallel()
	var resp ReferralRewardHistoryResponse
	require.NoError(t, json.Unmarshal([]byte(referalRewardHistoryResponse), &resp))
	_, err := e.GetReferralRewardHistory(t.Context(), 9, 5, 50)
	assert.ErrorIs(t, err, errInvalidUserBusinessType)
	_, err = e.GetReferralRewardHistory(t.Context(), 1, 0, 50)
	assert.ErrorIs(t, err, errMissingPageNumber)
	_, err = e.GetReferralRewardHistory(t.Context(), 1, 5, 0)
	assert.ErrorIs(t, err, errInvalidRowNumber)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err = e.GetReferralRewardHistory(t.Context(), 1, 5, 50)
	assert.NoError(t, err)
}

func TestGetSubaccountTransferHistory(t *testing.T) {
	t.Parallel()

	_, err := e.GetSubaccountTransferHistory(t.Context(), "", 0, 0, 0, 0)
	assert.ErrorIs(t, err, errNotValidEmailAddress)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err = e.GetSubaccountTransferHistory(t.Context(), "example@golang.org", 0, 0, 0, 0)
	assert.Error(t, err, "GetSubaccountTransferHistory should return an error on a bogus email")
}

func TestExecuteSubAccountTransfer(t *testing.T) {
	t.Parallel()
	_, err := e.ExecuteSubAccountTransfer(t.Context(), &SubAccountTransferRequestParams{})
	assert.ErrorIs(t, err, errUnacceptableSenderEmail)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.ExecuteSubAccountTransfer(t.Context(), &SubAccountTransferRequestParams{
		FromEmail: "fromemail@thrasher.io",
		ToEmail:   "toemail@thrasher.io",
		Asset:     "BTC",
		Amount:    0.000005,
	})
	assert.ErrorContains(t, err, "You are not authorized to execute this request.")
}

func TestGetSubaccountAssets(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubaccountAssets(t.Context(), "")
	assert.ErrorIs(t, err, errNotValidEmailAddress)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err = e.GetSubaccountAssets(t.Context(), "subaccount@thrasher.io")
	assert.ErrorContains(t, err, "This account does not exist.")
}

func TestGetOrderRateLimits(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetOrderRateLimits(t.Context(), 0)
	require.NoError(t, err, "GetOrderRateLimits must not error")
}

var testNewOrderResponseJSON = `{
	"symbol": "BTCUSDT",
	"orderId": 28,
	"orderListId": -1,
	"clientOrderId": "6gCrw2kRUAF9CvJDGP16IP",
	"transactTime": 1507725176595,
	"price": "0.00000000",
	"origQty": "10.00000000",
	"executedQty": "10.00000000",
	"cummulativeQuoteQty": "10.00000000",
	"status": "FILLED",
	"timeInForce": "GTC",
	"type": "MARKET",
	"side": "SELL"
  }`

func TestNewOrderTest(t *testing.T) {
	t.Parallel()
	var resp NewOrderResponse
	require.NoError(t, json.Unmarshal([]byte(testNewOrderResponseJSON), &resp), "NewOrderResponse JSON must unmarshal")
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	req := &NewOrderRequest{
		Symbol:      currency.NewPair(currency.LTC, currency.BTC),
		Side:        order.Buy.String(),
		TradeType:   BinanceRequestParamsOrderLimit,
		Price:       0.0025,
		Quantity:    100000,
		TimeInForce: order.GoodTillCancel.String(),
	}
	_, err := e.NewOrderTest(t.Context(), req)
	require.NoError(t, err, "NewOrderTest must not return error for limit order")
	req = &NewOrderRequest{
		Symbol:        currency.NewPair(currency.LTC, currency.BTC),
		Side:          order.Sell.String(),
		TradeType:     BinanceRequestParamsOrderMarket,
		Price:         0.0045,
		QuoteOrderQty: 10,
	}
	_, err = e.NewOrderTest(t.Context(), req)
	require.NoError(t, err, "NewOrderTest must not return error for market order")
}

func TestNewOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	req := &NewOrderRequest{
		Symbol:      currency.NewPair(currency.LTC, currency.BTC),
		Side:        order.Buy.String(),
		TradeType:   BinanceRequestParamsOrderLimit,
		Price:       0.0025,
		Quantity:    100000,
		TimeInForce: order.GoodTillCancel.String(),
	}
	_, err := e.NewOrder(t.Context(), req)
	if err != nil {
		assert.Contains(t, err.Error(), "Account has insufficient balance for requested action", "NewOrder error should indicate insufficient balance")
	}
}

func TestGetOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrder(t.Context(), &OrderRequestParams{})
	assert.ErrorIs(t, err, errIncompleteArguments)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err = e.GetOrder(t.Context(), &OrderRequestParams{
		Symbol:            "BTCUSDT",
		OrigClientOrderID: "something",
	})
	// You can check the existence of an order using a valid Symbol and OrigClient Order ID
	assert.ErrorContains(t, err, "Order does not exist.")
}

var openOrdersItemJSON = `{
    "symbol": "LTCBTC",
    "orderId": 1,
    "orderListId": -1,
    "clientOrderId": "myOrder1",
    "price": "0.1",
    "origQty": "1.0",
    "executedQty": "0.0",
    "cummulativeQuoteQty": "0.0",
    "status": "NEW",
    "timeInForce": "GTC",
    "type": "LIMIT",
    "side": "BUY",
    "stopPrice": "0.0",
    "icebergQty": "0.0",
    "time": 1499827319559,
    "updateTime": 1499827319559,
    "isWorking": true,
    "origQuoteOrderQty": "0.000000"
  }`

func TestGetAllOpenOrders(t *testing.T) {
	t.Parallel()
	var resp Order
	require.NoError(t, json.Unmarshal([]byte(openOrdersItemJSON), &resp), "Order JSON must unmarshal")
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetAllOpenOrders(t.Context(), "")
	require.NoError(t, err, "GetAllOpenOrders must not error")
}

func TestCancelExistingOrder(t *testing.T) {
	t.Parallel()

	_, err := e.CancelExistingOrder(t.Context(), &CancelOrderRequestParams{})
	assert.ErrorIs(t, err, errMissingCurrencySymbol)

	_, err = e.CancelExistingOrder(t.Context(), &CancelOrderRequestParams{
		Symbol: currency.NewBTCUSDT(),
	})
	assert.ErrorIs(t, err, errEitherOrderIDOrClientOrderIDIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.CancelExistingOrder(t.Context(), &CancelOrderRequestParams{
		Symbol:                currency.NewBTCUSDT(),
		ClientSuppliedOrderID: "1234",
	})
	assert.ErrorContains(t, err, "Unknown order sent.")
}

func TestCancelOpenOrdersForSymbol(t *testing.T) {
	t.Parallel()
	_, err := e.CancelOpenOrdersForSymbol(t.Context(), "")
	assert.ErrorIs(t, err, errMissingCurrencySymbol)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err = e.CancelOpenOrdersForSymbol(t.Context(), "BTCUSDT")
	assert.NoError(t, err)
}

// TestGetTrades test for fetching the list of
// trades attached with this account.
func TestGetTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetTrades(t.Context(), &GetTradesParams{})
	assert.ErrorIs(t, err, errIncompleteArguments)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err = e.GetTrades(t.Context(), &GetTradesParams{Symbol: "BTCUSDT"})
	assert.NoError(t, err)
}

func TestCreateNewOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CreateNewOCOOrder(t.Context(),
		&OCOOrderInputParams{
			StopPrice: 1000,
			Side:      order.Buy.String(),
			Quantity:  0.0000001,
			Price:     1232334.00,
		})
	assert.ErrorIs(t, err, errIncompleteArguments)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err = e.CreateNewOCOOrder(t.Context(),
		&OCOOrderInputParams{
			Symbol:               "XTZUSD",
			Price:                100,
			StopPrice:            3,
			StopLimitPrice:       2.5,
			Side:                 order.Buy.String(),
			Quantity:             1,
			StopLimitTimeInForce: "GTC",
			RecvWindow:           6000,
		})
	assert.ErrorContains(t, err, "Precision is over the maximum defined for this asset.")
}

var ocoOrderJSON = `{
	"orderListId": 27,
	"contingencyType": "OCO",
	"listStatusType": "EXEC_STARTED",
	"listOrderStatus": "EXECUTING",
	"listClientOrderId": "h2USkA5YQpaXHPIrkd96xE",
	"transactionTime": 1565245656253,
	"symbol": "LTCBTC",
	"orders": [
	  {
		"symbol": "LTCBTC",
		"orderId": 4,
		"clientOrderId": "qD1gy3kc3Gx0rihm9Y3xwS"
	  },
	  {
		"symbol": "LTCBTC",
		"orderId": 5,
		"clientOrderId": "ARzZ9I00CPM8i3NhmU9Ega"
	  }
	]
  }`

func TestGetOCOOrder(t *testing.T) {
	t.Parallel()
	var resp OCOOrderResponse
	require.NoError(t, json.Unmarshal([]byte(ocoOrderJSON), &resp))
	_, err := e.GetOCOOrder(t.Context(), &GetOCOOrderRequestParams{})
	assert.ErrorIs(t, err, errIncompleteArguments)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err = e.GetOCOOrder(t.Context(), &GetOCOOrderRequestParams{
		OrderListID: "123445",
	})
	assert.ErrorContains(t, err, "Order list does not exist.")
}

func TestGetAllOCOOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAllOCOOrder(t.Context(), &OCOOrdersRequestParams{})
	require.NoError(t, err, "GetAllOCOOrder must not error")
}

func TestGetOpenOCOOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetOpenOCOOrders(t.Context(), 0)
	require.NoError(t, err, "GetOpenOCOOrders must not error")
}

func TestCancelOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelOCOOrder(t.Context(), &OCOOrdersDeleteRequestParams{})
	assert.ErrorIs(t, err, errIncompleteArguments)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.CancelOCOOrder(t.Context(), &OCOOrdersDeleteRequestParams{
		Symbol:      "BTCUSDT",
		OrderListID: 123456,
	})
	assert.NoError(t, err)
}

// OTC end Points test code.
func TestGetSupportedCoinPairs(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSupportedCoinPairs(t.Context(), currency.Pair{Base: currency.BTC, Quote: currency.USDT})
	require.NoError(t, err, "GetSupportedCoinPairs must not error")
}

func TestRequestForQuote(t *testing.T) {
	t.Parallel()
	_, err := e.RequestForQuote(t.Context(), &RequestQuoteParams{ToCoin: "BTC", RequestCoin: "USDT", RequestAmount: 1})
	assert.ErrorIs(t, err, errMissingFromCoinName)
	_, err = e.RequestForQuote(t.Context(), &RequestQuoteParams{FromCoin: "ETH", RequestCoin: "USDT", RequestAmount: 1})
	assert.ErrorIs(t, err, errMissingToCoinName)
	_, err = e.RequestForQuote(t.Context(), &RequestQuoteParams{FromCoin: "ETH", ToCoin: "BTC", RequestCoin: "USDT"})
	assert.ErrorIs(t, err, errMissingRequestAmount)
	_, err = e.RequestForQuote(t.Context(), &RequestQuoteParams{FromCoin: "ETH", ToCoin: "BTC", RequestAmount: 1})
	assert.ErrorIs(t, err, errMissingRequestCoin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err = e.RequestForQuote(t.Context(), &RequestQuoteParams{FromCoin: "BTC", ToCoin: "USDT", RequestCoin: "BTC", RequestAmount: 1})
	assert.NoError(t, err)
}

var testPlaceOTCTradeOrderJSON = `{
    "orderId": "10002349",
    "createTime": 1641906714,
    "orderStatus": "PROCESS"
}`

func TestPlaceOTCTradeOrder(t *testing.T) {
	t.Parallel()
	var resp OTCTradeOrderResponse
	require.NoError(t, json.Unmarshal([]byte(testPlaceOTCTradeOrderJSON), &resp))
	_, err := e.PlaceOTCTradeOrder(t.Context(), "")
	assert.ErrorIs(t, err, errMissingQuoteID)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.PlaceOTCTradeOrder(t.Context(), "15848701022")
	assert.ErrorContains(t, err, "-9000")
}

var testGetOTCTradeOrderJSON = `{
    "quoteId": "4e5446f2cc6f44ab86ab02abf19a2fd2",
    "orderId": "10002349", 
    "orderStatus": "SUCCESS",
    "fromCoin": "BTC",
    "fromAmount": 1,
    "toCoin": "USDT",
    "toAmount": 50550.26,
    "ratio": 50550.26,
    "inverseRatio": 0.00001978,
    "createTime": 1641806714
}`

func TestGetOTCTradeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	var val OTCTradeOrder
	require.NoError(t, json.Unmarshal([]byte(testGetOTCTradeOrderJSON), &val), "OTCTradeOrder JSON must unmarshal")
	_, err := e.GetOTCTradeOrder(t.Context(), 10002349)
	if err != nil {
		assert.Contains(t, err.Error(), "status code: 400", "GetOTCTradeOrder error should indicate HTTP 400 status code")
	}
}

var getAllOTCTradeOrders = `[
    {
        "quoteId": "4e5446f2cc6f44ab86ab02abf19a2fd2",
        "orderId": "10002349", 
        "orderStatus": "SUCCESS",
        "fromCoin": "BTC",
        "fromAmount": 1,
        "toCoin": "USDT",
        "toAmount": 50550.26,
        "ratio": 50550.26,
        "inverseRatio": 0.00001978,
        "createTime": 1641806714
    },
    {
        "quoteId": "15848645308",
        "orderId": "10002380", 
        "orderStatus": "PROCESS",
        "fromCoin": "SHIB",
        "fromAmount": 10000,
        "toCoin": "KSHIB",
        "toAmount": 10,
        "ratio": 0.001,
        "inverseRatio": 1000,
        "createTime": 1641916714
    }
]
`

func TestGetAllOTCTradeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	var orders []OTCTradeOrder
	require.NoError(t, json.Unmarshal([]byte(getAllOTCTradeOrders), &orders), "OTCTradeOrders JSON must unmarshal")
	_, err := e.GetAllOTCTradeOrders(t.Context(), &OTCTradeOrderRequestParams{})
	require.NoError(t, err, "GetAllOTCTradeOrders must not error")
}

var ocbsTradeOrderJSON = `
{
  "quoteId": "4e5446f2cc6f44ab86ab02abf19abvd",
  "orderId": "1000238000", 
  "orderStatus": "FAIL",
  "fromCoin": "USD",
  "fromAmount": 1000.5,
  "toCoin": "ETH",
  "toAmount": 0.5,
  "feeCoin": "USD",
  "feeAmount": 0.5,
  "ratio": 2000,
  "createTime": 1641916714
}`

func TestGetAllOCBSTradeOrders(t *testing.T) {
	t.Parallel()
	var orderDetail OCBSOrder
	require.NoError(t, json.Unmarshal([]byte(ocbsTradeOrderJSON), &orderDetail), "OCBSOrder JSON must unmarshal")
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAllOCBSTradeOrders(t.Context(), OCBSOrderRequestParams{})
	require.NoError(t, err, "GetAllOCBSTradeOrders must not error")
}

func TestGetAssetFeesAndWalletStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAssetFeesAndWalletStatus(t.Context())
	require.NoError(t, err, "GetAssetFeesAndWalletStatus must not error")
}

func TestWithdrawCrypto(t *testing.T) {
	t.Parallel()

	_, err := e.WithdrawCrypto(t.Context(), &withdraw.Request{})
	assert.ErrorIs(t, err, errMissingRequiredArgumentCoin)
	_, err = e.WithdrawCrypto(t.Context(), &withdraw.Request{
		Currency: currency.BTC,
	})
	assert.ErrorIs(t, err, errMissingRequiredArgumentNetwork)
	params := &withdraw.Request{
		Currency: currency.BTC,
		Crypto: withdraw.CryptoRequest{
			Chain: "BSC",
		},
	}
	_, err = e.WithdrawCrypto(t.Context(), params)
	assert.ErrorIs(t, err, errMissingRequiredParameterAddress)
	params.Crypto.Address = "1234567"
	_, err = e.WithdrawCrypto(t.Context(), params)
	assert.ErrorIs(t, err, errAmountValueMustBeGreaterThan0)
	params.Amount = 1
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.WithdrawCrypto(t.Context(), params)
	assert.ErrorContains(t, err, "You are not authorized to execute this request.")
}

func TestFiatWithdrawalHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FiatWithdrawalHistory(t.Context(), &FiatWithdrawalRequestParams{
		FiatCurrency: "USDT",
	})
	require.NoError(t, err, "FiatWithdrawalHistory must not error")
}

func TestDepositHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.DepositHistory(t.Context(), currency.USD, 1, time.Time{}, time.Time{}, 0, 100)
	require.NoError(t, err, "DepositHistory must not error")
}

func TestFiatDepositHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FiatDepositHistory(t.Context(), &FiatWithdrawalRequestParams{})
	require.NoError(t, err, "FiatDepositHistory must not error")
}

// WEBSOCKET support testing
// Since both binance and Binance US has same websocket functions,
// the tests functions are also similar

// TestWebsocketStreamKey  this test mmethod handles the
// creating, updating, and deleting of user stream key or "listenKey"
// all the three methods in one test methods.
func TestWebsocketStreamKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetWsAuthStreamKey(t.Context())
	require.NoError(t, err, "GetWsAuthStreamKey must not error")
	err = e.MaintainWsAuthStreamKey(t.Context())
	require.NoError(t, err, "MaintainWsAuthStreamKey must not error")
	err = e.CloseUserDataStream(t.Context())
	require.NoError(t, err, "CloseUserDataStream must not error")
}

var subscriptionRequestString = `{
	"method": "SUBSCRIBE",
	"params": [
	  "btcusdt@aggTrade",
	  "btcusdt@depth"
	],
	"id": 1
  }`

func TestWebsocketSubscriptionHandling(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	rawData := []byte(subscriptionRequestString)
	err := e.wsHandleData(rawData)
	require.NoError(t, err, "wsHandleData must not return error for subscription request")
}

func TestWebsocketUnsubscriptionHandling(t *testing.T) {
	pressXToJSON := []byte(`{
	"method": "UNSUBSCRIBE",
	"params": [
		"btcusdt@depth"
	],
	"id": 312
	}`)
	err := e.wsHandleData(pressXToJSON)
	require.NoError(t, err, "wsHandleData must not return error for unsubscription request")
}

func TestGetSubscriptions(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubscriptions()
	require.NoError(t, err, "GetSubscriptions must not error")
}

var ticker24hourChangeStream = `{
	"stream":"btcusdt@ticker",
	"data" :{
		"e": "24hrTicker",  
		"E": 1234567891,     
		"s": "BNBBTC",      
		"p": "0.0015",      
		"P": "250.00",      
		"w": "0.0018",      
		"x": "0.0009",      
		"c": "0.0025",      
		"Q": "10",          
		"b": "0.0024",       
		"B": "10",           
		"a": "0.0026",       
		"A": "100",          
		"o": "0.0010",      
		"h": "0.0025",      
		"l": "0.0010",      
		"v": "10000",        
		"q": "18",           
		"O": 0,             
		"C": 8640000011,      
		"F": 0,             
		"L": 18150,         
		"n": 18151           
  }
}`

func TestWebsocketTickerUpdate(t *testing.T) {
	t.Parallel()
	err := e.wsHandleData([]byte(ticker24hourChangeStream))
	require.NoError(t, err, "wsHandleData must not return error for ticker update")
}

func TestWebsocketKlineUpdate(t *testing.T) {
	t.Parallel()
	pressXToJSON := []byte(`
	{
		"stream":"btcusdt@kline_1m",
		"data":{
			"e": "kline",     
			"E": 1234567891,   
			"s": "BNBBTC",    
			"k": {
				"t": 1234000001, 
				"T": 1234600001, 
				"s": "BNBBTC",  
				"i": "1m",      
				"f": 100,       
				"L": 200,       
				"o": "0.0010",  
				"c": "0.0020",  
				"h": "0.0025",  
				"l": "0.0015",  
				"v": "1000",    
				"n": 100,       
				"x": false,     
				"q": "1.0000",  
				"V": "500",     
				"Q": "0.500",   
				"B": "123456"   
	  			}
			}
		}`)
	err := e.wsHandleData(pressXToJSON)
	require.NoError(t, err, "wsHandleData must not return error for kline update")
}

func TestWebsocketStreamTradeUpdate(t *testing.T) {
	t.Parallel()
	pressXToJSON := []byte(`{"stream":"btcusdt@trade","data":{
	  "e": "trade",     
	  "E": 123456789,   
	  "s": "BNBBTC",    
	  "t": 12345,       
	  "p": "0.001",     
	  "q": "100",
	  "b": 88,        
	  "a": 50,          
	  "T": 123456785,
	  "m": true,        
	  "M": true         
	}}`)
	err := e.wsHandleData(pressXToJSON)
	require.NoError(t, err, "wsHandleData must not return error for trade update")
}

// TestWsDepthUpdate copied from the Binance Test
func TestWebsocketOrderBookDepthDiffStream(t *testing.T) {
	binanceusOrderBookLock.Lock()
	defer binanceusOrderBookLock.Unlock()
	e.setupOrderbookManager(t.Context())
	seedLastUpdateID := int64(161)
	book := OrderBook{
		Asks: []orderbook.Level{
			{Price: 6621.80000000, Amount: 0.00198100},
			{Price: 6622.14000000, Amount: 4.00000000},
			{Price: 6622.46000000, Amount: 2.30000000},
			{Price: 6622.47000000, Amount: 1.18633300},
			{Price: 6622.64000000, Amount: 4.00000000},
			{Price: 6622.73000000, Amount: 0.02900000},
			{Price: 6622.76000000, Amount: 0.12557700},
			{Price: 6622.81000000, Amount: 2.08994200},
			{Price: 6622.82000000, Amount: 0.01500000},
			{Price: 6623.17000000, Amount: 0.16831300},
		},
		Bids: []orderbook.Level{
			{Price: 6621.55000000, Amount: 0.16356700},
			{Price: 6621.45000000, Amount: 0.16352600},
			{Price: 6621.41000000, Amount: 0.86091200},
			{Price: 6621.25000000, Amount: 0.16914100},
			{Price: 6621.23000000, Amount: 0.09193600},
			{Price: 6621.22000000, Amount: 0.00755100},
			{Price: 6621.13000000, Amount: 0.08432000},
			{Price: 6621.03000000, Amount: 0.00172000},
			{Price: 6620.94000000, Amount: 0.30506700},
			{Price: 6620.93000000, Amount: 0.00200000},
		},
		LastUpdateID: seedLastUpdateID,
	}
	update1 := []byte(`{"stream":"btcusdt@depth","data":{
	  "e": "depthUpdate", 
	  "E": 1234567891,     
	  "s": "BTCUSDT",      
	  "U": 157,           
	  "u": 160,           
	  "b": [              
		["6621.45", "0.3"]
	  ],
	  "a": [              
		["6622.46", "1.5"]
	  ]
	}}`)

	p := currency.NewPairWithDelimiter("BTC", "USDT", "-")
	if err := e.SeedLocalCacheWithBook(p, &book); err != nil {
		require.NoError(t, err, "SeedLocalCacheWithBook must not error")
	}
	if err := e.wsHandleData(update1); err != nil {
		require.NoError(t, err, "wsHandleData must not error for initial depth update")
	}
	e.obm.state[currency.BTC][currency.USDT][asset.Spot].fetchingBook = false
	ob, err := e.Websocket.Orderbook.GetOrderbook(p, asset.Spot)
	require.NoError(t, err, "Orderbook.GetOrderbook must not error")
	assert.Equalf(t, seedLastUpdateID, ob.LastUpdateID, "Orderbook.LastUpdateID should remain unchanged after stale update")
	assert.Equalf(t, 2.3, ob.Asks[2].Amount, "Orderbook.Asks should remain unchanged after stale update")
	assert.Equalf(t, 0.163526, ob.Bids[1].Amount, "Orderbook.Bids should remain unchanged after stale update")
	update2 := []byte(`{
		"stream":"btcusdt@depth","data":{
			"e": "depthUpdate", 
			"E": 1234567892,     
			"s": "BTCUSDT",      
			"U": 161,           
			"u": 165,           
			"b": [           
				["6621.45", "0.163526"]
			],
			"a": [             
				["6622.46", "2.3"], 
				["6622.47", "1.9"]
			]
		}
	}`)
	err = e.wsHandleData(update2)
	require.NoError(t, err, "wsHandleData must not return error for orderbook depth update")
	ob, err = e.Websocket.Orderbook.GetOrderbook(p, asset.Spot)
	require.NoError(t, err, "Orderbook.GetOrderbook must not return error after live update")
	assert.Equalf(t, int64(165), ob.LastUpdateID, "Orderbook.LastUpdateID should update after new depth data")
	assert.Equalf(t, 2.3, ob.Asks[2].Amount, "Orderbook.Asks should reflect updated amount")
	assert.Equalf(t, 1.9, ob.Asks[3].Amount, "Orderbook.Asks should append new levels correctly")
	assert.Equalf(t, 0.163526, ob.Bids[1].Amount, "Orderbook.Bids should update amount correctly")
	e.obm.state[currency.BTC][currency.USDT][asset.Spot].lastUpdateID = 0
}

// TestWebsocketPartialOrderBookDepthStream copied from the Binance Test
func TestWebsocketPartialOrderBookDepthStream(t *testing.T) {
	t.Parallel()
	update1 := []byte(`{"stream":"btcusdt@depth5","data":
	{
		"lastUpdateId": 160,
		"bids": [           
		  [
			"0.0024",       
			"10"            
		  ]
		],
		"asks": [           
		  [
			"0.0026",       
			"100"           
		  ]
		]
	  }}`)
	var err error
	err = e.wsHandleData(update1)
	require.NoError(t, err, "wsHandleData must not return error for partial depth stream update1")
	update2 := []byte(`{
		"stream":"btcusdt@depth10",
		"data":{
			"lastUpdateId": 160, 
			"bids": [            
					[
						"0.0024",        
						"10"             
					]
			],
			"asks": [            
				[
					"0.0026",        
					"100"            
				]
			]
		}
	  }`)
	err = e.wsHandleData(update2)
	require.NoError(t, err, "wsHandleData must not return error for partial depth stream update2")
}

func TestWebsocketBookTicker(t *testing.T) {
	t.Parallel()
	bookTickerJSON := []byte(
		`{
		"stream": "btcusdt@bookTicker",
		"data": {
			"u":400900217,   
			"s":"BNBUSDT",  
			"b":"25.35190000",
			"B":"31.21000000",
			"a":"25.36520000",
			"A":"40.66000000" 
		}
	  }`)
	err := e.wsHandleData(bookTickerJSON)
	require.NoError(t, err, "wsHandleData must not return error for book ticker")
	bookTickerForAllSymbols := []byte(`
	{
		"stream" : "!bookTicker",
		"data":{
			"u":400900217,    
			"s":"BNBUSDT",    
			"b":"25.35190000",
			"B":"31.21000000",
			"a":"25.36520000",
			"A":"40.66000000" 
		}
	}`)
	err = e.wsHandleData(bookTickerForAllSymbols)
	require.NoError(t, err, "wsHandleData must not return error for global book ticker")
}

func TestWebsocketAggTrade(t *testing.T) {
	t.Parallel()
	aggTradejson := []byte(
		`{  
			"stream":"btcusdt@aggTrade", 
			"data": {
				"e": "aggTrade",  
				"E": 1672515782136,   
				"s": "BNBBTC",
				"a": 12345,       
				"p": "0.001",     
				"q": "100",   
				"f": 100,     
				"l": 105,   
				"T": 1672515782136,
				"m": true,
				"M": true         
			}
	   }`)
	err := e.wsHandleData(aggTradejson)
	require.NoError(t, err, "wsHandleData must not return error for aggregated trade update")
}

var balanceUpdateInputJSON = `
{
	"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc",
	"data":{
		"e": "balanceUpdate",         
		"E": 1573200697110,           
		"a": "BTC",                   
		"d": "100.00000000",          
		"T": 1573200697068}}`

func TestWebsocketBalanceUpdate(t *testing.T) {
	t.Parallel()
	thejson := []byte(balanceUpdateInputJSON)
	err := e.wsHandleData(thejson)
	require.NoError(t, err, "wsHandleData must not return error for balance update")
}

var listStatusUserDataStreamPayload = `
{
	"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc",
	"data":{
		"e": "listStatus",                
		"E": 1564035303637,               
		"s": "ETHBTC",                    
		"g": 2,                           
		"c": "OCO",                       
		"l": "EXEC_STARTED",              
		"L": "EXECUTING",                 
		"r": "NONE",                      
		"C": "F4QN4G8DlFATFlIUQ0cjdD",    
		"T": 1564035303625,               
		"O": [                            
			{
				"s": "ETHBTC",                
				"i": 17,                      
				"c": "AJYsMjErWJesZvqlJCTUgL" 
			},
			{
				"s": "ETHBTC",
				"i": 18,
				"c": "bfYPSQdLoqAJeNrOr9adzq"
			}
		]
	}
}`

func TestWebsocketListStatus(t *testing.T) {
	t.Parallel()
	err := e.wsHandleData([]byte(listStatusUserDataStreamPayload))
	require.NoError(t, err, "wsHandleData must not return error for list status update")
}

func TestExecutionTypeToOrderStatus(t *testing.T) {
	type TestCases struct {
		Case   string
		Result order.Status
	}
	testCases := []TestCases{
		{Case: "NEW", Result: order.New},
		{Case: "PARTIALLY_FILLED", Result: order.PartiallyFilled},
		{Case: "FILLED", Result: order.Filled},
		{Case: "CANCELED", Result: order.Cancelled},
		{Case: "PENDING_CANCEL", Result: order.PendingCancel},
		{Case: "REJECTED", Result: order.Rejected},
		{Case: "EXPIRED", Result: order.Expired},
		{Case: "LOL", Result: order.UnknownStatus},
	}
	for i := range testCases {
		result, _ := stringToOrderStatus(testCases[i].Case)
		assert.Equalf(t, testCases[i].Result, result, "stringToOrderStatus should map %s correctly", testCases[i].Case)
	}
}

var websocketDepthUpdate = []byte(
	`{
		"e": "depthUpdate",
		"E": 12345678911,    
		"s": "BNBBTC",     
		"U": 157,          
		"u": 160,          
		"b": [             
		  [
			"0.0024",      
			"10"           
		  ]
		],
		"a": [             
		  [
			"0.0026",      
			"100"          
		  ]
		]
	  }
	`)

func TestProcessUpdate(t *testing.T) {
	t.Parallel()
	binanceusOrderBookLock.Lock()
	defer binanceusOrderBookLock.Unlock()
	e.setupOrderbookManager(t.Context())
	p := currency.NewBTCUSDT()
	var depth WebsocketDepthStream
	err := json.Unmarshal(websocketDepthUpdate, &depth)
	require.NoError(t, err, "json.Unmarshal must not error for depth update")
	err = e.obm.stageWsUpdate(&depth, p, asset.Spot)
	require.NoError(t, err, "stageWsUpdate must not error")
	err = e.obm.fetchBookViaREST(p)
	require.NoError(t, err, "fetchBookViaREST must not error")
	err = e.obm.cleanup(p)
	require.NoError(t, err, "cleanup must not error")
	e.obm.state[currency.BTC][currency.USDT][asset.Spot].lastUpdateID = 0
}

func TestWebsocketOrderExecutionReport(t *testing.T) {
	payload := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"executionReport","E":1616627567900,"s":"BTCUSDT","c":"c4wyKsIhoAaittTYlIVLqk","S":"BUY","o":"LIMIT","f":"GTC","q":"0.00028400","p":"52789.10000000","P":"0.00000000","F":"0.00000000","g":-1,"C":"","x":"NEW","X":"NEW","r":"NONE","i":5340845958,"l":"0.00000000","z":"0.00000000","L":"0.00000000","n":"0","N":"BTC","T":1616627567900,"t":-1,"I":11388173160,"w":true,"m":false,"M":false,"O":1616627567900,"Z":"0.00000000","Y":"0.00000000","Q":"0.00000000"}}`)
	expectedResult := order.Detail{
		Price:           52789.1,
		Amount:          0.00028400,
		RemainingAmount: 0.00028400,
		CostAsset:       currency.USDT,
		FeeAsset:        currency.BTC,
		Exchange:        "Binanceus",
		OrderID:         "5340845958",
		ClientOrderID:   "c4wyKsIhoAaittTYlIVLqk",
		Type:            order.Limit,
		Side:            order.Buy,
		Status:          order.New,
		AssetType:       asset.Spot,
		Date:            time.UnixMilli(1616627567900),
		LastUpdated:     time.UnixMilli(1616627567900),
		Pair:            currency.NewBTCUSDT(),
	}
	for len(e.Websocket.DataHandler) > 0 {
		<-e.Websocket.DataHandler
	}
	err := e.wsHandleData(payload)
	require.NoError(t, err, "wsHandleData must not return error for execution report payload")
	res := <-e.Websocket.DataHandler
	switch r := res.(type) {
	case *order.Detail:
		assert.Equal(t, expectedResult, *r, "wsHandleData should emit expected order detail")
	default:
		assert.Failf(t, "unexpected type", "wsHandleData should emit order.Detail but received %T", res)
	}
	payload = []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"executionReport","E":1616633041556,"s":"BTCUSDT","c":"YeULctvPAnHj5HXCQo9Mob","S":"BUY","o":"LIMIT","f":"GTC","q":"0.00028600","p":"52436.85000000","P":"0.00000000","F":"0.00000000","g":-1,"C":"","x":"TRADE","X":"FILLED","r":"NONE","i":5341783271,"l":"0.00028600","z":"0.00028600","L":"52436.85000000","n":"0.00000029","N":"BTC","T":1616633041555,"t":726946523,"I":11390206312,"w":false,"m":false,"M":true,"O":1616633041555,"Z":"14.99693910","Y":"14.99693910","Q":"0.00000000"}}`)
	err = e.wsHandleData(payload)
	require.NoError(t, err, "wsHandleData must not return error for filled execution report")
}

func TestWebsocketOutboundAccountPosition(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"outboundAccountPosition","E":1616628815745,"u":1616628815745,"B":[{"a":"BTC","f":"0.00225109","l":"0.00123000"},{"a":"BNB","f":"0.00000000","l":"0.00000000"},{"a":"USDT","f":"54.43390661","l":"0.00000000"}]}}`)
	err := e.wsHandleData(payload)
	require.NoError(t, err, "wsHandleData must not return error for outboundAccountPosition event")
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAvailableTransferChains(t.Context(), currency.BTC)
	require.NoError(t, err, "GetAvailableTransferChains must not error")
}

func TestQuickEnableCryptoWithdrawal(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	err := e.QuickEnableCryptoWithdrawal(t.Context())
	if err != nil {
		assert.Contains(t, err.Error(), "unexpected end of JSON input", "QuickEnableCryptoWithdrawal error should indicate missing response body")
	}
}

func TestQuickDisableCryptoWithdrawal(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	err := e.QuickDisableCryptoWithdrawal(t.Context())
	if err != nil {
		assert.Contains(t, err.Error(), "unexpected end of JSON input", "QuickDisableCryptoWithdrawal error should indicate missing response body")
	}
}

func TestGetUsersSpotAssetSnapshot(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetUsersSpotAssetSnapshot(t.Context(), time.Time{}, time.Time{}, 10, 6)
	require.NoError(t, err, "GetUsersSpotAssetSnapshot must not error")
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

// TestGetAggregatedTradesBatched exercises TestGetAggregatedTradesBatched to ensure our date and limit scanning works correctly
// This test is susceptible to failure if volumes change a lot, during wash trading or zero-fee periods
// In live tests, 6 hours is expected to return about 1000 records
func TestGetAggregatedTradesBatched(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name    string
		args    *AggregatedTradeRequestParams
		expFunc func(*testing.T, []AggregatedTrade)
	}

	var tests []testCase
	start := time.Now().Add(-time.Hour * 24 * 90).Truncate(time.Minute) // 3 months ago
	tests = []testCase{
		{
			name: "batch with timerange",
			args: &AggregatedTradeRequestParams{StartTime: start, EndTime: start.Add(6 * time.Hour)},
			expFunc: func(t *testing.T, results []AggregatedTrade) {
				t.Helper()
				require.NotEmpty(t, results, "must have records")
				assert.Less(t, len(results), 10000, "should return a quantity below a sane threshold of records")
				assert.WithinDuration(t, results[len(results)-1].TimeStamp.Time(), start, 6*time.Hour, "last record should be within range of start time")
			},
		},
		{
			name: "custom limit with start time set, no end time",
			args: &AggregatedTradeRequestParams{StartTime: start, Limit: 2042},
			expFunc: func(t *testing.T, results []AggregatedTrade) {
				t.Helper()
				// 2000 records in was about 32 hours in 2025; Adjust if BinanceUS enters a phase of zero-fees or low-volume
				require.Equal(t, 2042, len(results), "must return exactly the limit number of records")
				assert.WithinDuration(t, results[len(results)-1].TimeStamp.Time(), start, 72*time.Hour, "last record should be within 72 hours of start time")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.args.Symbol = currency.NewBTCUSDT()
			results, err := e.GetAggregateTrades(t.Context(), tt.args)
			require.NoError(t, err)
			tt.expFunc(t, results)
		})
	}
}
