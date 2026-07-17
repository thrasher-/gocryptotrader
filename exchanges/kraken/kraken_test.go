package kraken

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/futures"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

var (
	e               *Exchange
	spotTestPair    = currency.NewPair(currency.XBT, currency.USD)
	futuresTestPair = currency.NewPairWithDelimiter("PF", "XBTUSD", "_")
)

// Please add your own APIkeys to do correct due diligence testing.
const (
	apiKey                  = ""
	apiSecret               = ""
	canManipulateRealOrders = false
)

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Kraken Setup error: %s", err)
	}
	if apiKey != "" && apiSecret != "" {
		e.API.AuthenticatedSupport = true
		e.SetCredentials(apiKey, apiSecret, "", "", "", "")
	}
	os.Exit(m.Run())
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
}

func TestGetCurrentServerTime(t *testing.T) {
	t.Parallel()
	_, err := e.GetCurrentServerTime(t.Context())
	assert.NoError(t, err, "GetCurrentServerTime should not error")
}

func TestWrapperGetServerTime(t *testing.T) {
	t.Parallel()
	st, err := e.GetServerTime(t.Context(), asset.Spot)
	require.NoError(t, err, "GetServerTime must not error")
	assert.WithinRange(t, st, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), "ServerTime should be within a day of now")
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		t.Run(a.String(), func(t *testing.T) {
			t.Parallel()
			require.NoError(t, e.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must not error")
			pairs, err := e.CurrencyPairs.GetPairs(a, false)
			require.NoError(t, err, "GetPairs must not error")
			for _, p := range pairs {
				l, err := e.GetOrderExecutionLimits(a, p)
				require.NoError(t, err, "GetOrderExecutionLimits must not error")
				assert.Positive(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive")
				assert.Positive(t, l.PriceStepIncrementSize, "PriceStepIncrementSize should be positive")
			}
		})
	}

	t.Run("unsupported asset", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, e.UpdateOrderExecutionLimits(t.Context(), asset.Binary), asset.ErrNotSupported)
	})
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTradablePairs(t.Context(), asset.Futures)
	assert.NoError(t, err, "FetchTradablePairs should not error")
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	_, err := e.UpdateTicker(t.Context(), spotTestPair, asset.Spot)
	assert.NoError(t, err, "UpdateTicker spot asset should not error")

	_, err = e.UpdateTicker(t.Context(), futuresTestPair, asset.Futures)
	assert.NoError(t, err, "UpdateTicker futures asset should not error")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")

	testexch.UpdatePairsOnce(t, e)

	err := e.UpdateTickers(t.Context(), asset.Spot)
	require.NoError(t, err, "UpdateTickers must not error")

	ap, err := e.GetAvailablePairs(asset.Spot)
	require.NoError(t, err, "GetAvailablePairs must not error")

	for i := range ap {
		_, err = ticker.GetTicker(e.Name, ap[i], asset.Spot)
		assert.NoErrorf(t, err, "GetTicker should not error for %s", ap[i])
	}

	ap, err = e.GetAvailablePairs(asset.Futures)

	require.NoError(t, err, "GetAvailablePairs must not error")
	err = e.UpdateTickers(t.Context(), asset.Futures)
	require.NoError(t, err, "UpdateTickers must not error")

	for i := range ap {
		_, err = ticker.GetTicker(e.Name, ap[i], asset.Futures)
		assert.NoErrorf(t, err, "GetTicker should not error for %s", ap[i])
	}

	err = e.UpdateTickers(t.Context(), asset.Index)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "UpdateTickers should error correctly for asset.Index")
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup instance must not error")
	ex.Name += "-UpdateOrderbook"

	_, err := ex.UpdateOrderbook(t.Context(), spotTestPair, asset.Spot)
	assert.NoError(t, err, "UpdateOrderbook spot asset should not error")
	_, err = ex.UpdateOrderbook(t.Context(), futuresTestPair, asset.Futures)
	assert.NoError(t, err, "UpdateOrderbook futures asset should not error")
}

func TestFuturesBatchOrder(t *testing.T) {
	t.Parallel()
	req := []PlaceBatchOrderData{{
		PlaceOrderType: "meow",
		OrderID:        "test123",
		Symbol:         futuresTestPair.Lower().String(),
	}}
	_, err := e.FuturesBatchOrder(t.Context(), req)
	assert.ErrorIs(t, err, errInvalidBatchOrderType, "FuturesBatchOrder should error correctly")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	req[0].PlaceOrderType = "cancel"
	_, err = e.FuturesBatchOrder(t.Context(), req)
	assert.NoError(t, err, "FuturesBatchOrder should not error")
}

func TestFuturesEditOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesEditOrder(t.Context(), "test123", "", 5.2, 1, 0)
	assert.NoError(t, err, "FuturesEditOrder should not error")
}

func TestFuturesSendOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesSendOrder(t.Context(), order.Limit, futuresTestPair, "buy", "", "", "", order.ImmediateOrCancel, 1, 1, 0.9)
	assert.NoError(t, err, "FuturesSendOrder should not error")
}

func TestFuturesCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesCancelOrder(t.Context(), "test123", "")
	assert.NoError(t, err, "FuturesCancelOrder should not error")
}

func TestFuturesGetFills(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesGetFills(t.Context(), time.Now().Add(-time.Hour*24))
	assert.NoError(t, err, "FuturesGetFills should not error")
}

func TestFuturesTransfer(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesTransfer(t.Context(), "cash", "futures", "btc", 2)
	assert.NoError(t, err, "FuturesTransfer should not error")
}

func TestFuturesGetOpenPositions(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesGetOpenPositions(t.Context())
	assert.NoError(t, err, "FuturesGetOpenPositions should not error")
}

func TestFuturesNotifications(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesNotifications(t.Context())
	assert.NoError(t, err, "FuturesNotifications should not error")
}

func TestFuturesCancelAllOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesCancelAllOrders(t.Context(), futuresTestPair)
	assert.NoError(t, err, "FuturesCancelAllOrders should not error")
}

func TestGetFuturesAccountData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetFuturesAccountData(t.Context())
	assert.NoError(t, err, "GetFuturesAccountData should not error")
}

func TestFuturesCancelAllOrdersAfter(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesCancelAllOrdersAfter(t.Context(), 50)
	assert.NoError(t, err, "FuturesCancelAllOrdersAfter should not error")
}

func TestFuturesOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesOpenOrders(t.Context())
	assert.NoError(t, err, "FuturesOpenOrders should not error")
}

func TestFuturesRecentOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesRecentOrders(t.Context(), futuresTestPair)
	assert.NoError(t, err, "FuturesRecentOrders should not error")
}

func TestFuturesWithdrawToSpotWallet(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesWithdrawToSpotWallet(t.Context(), "xbt", 5)
	assert.NoError(t, err, "FuturesWithdrawToSpotWallet should not error")
}

func TestFuturesGetTransfers(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesGetTransfers(t.Context(), time.Now().Add(-time.Hour*24))
	assert.NoError(t, err, "FuturesGetTransfers should not error")
}

func TestGetFuturesOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesOrderbook(t.Context(), futuresTestPair)
	assert.NoError(t, err, "GetFuturesOrderbook should not error")
}

func TestGetFuturesMarkets(t *testing.T) {
	t.Parallel()
	_, err := e.GetInstruments(t.Context())
	assert.NoError(t, err, "GetInstruments should not error")
}

func TestGetFuturesTickers(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesTickers(t.Context())
	assert.NoError(t, err, "GetFuturesTickers should not error")
}

func TestGetFuturesTradeHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesTradeHistory(t.Context(), futuresTestPair, time.Now().Add(-time.Hour*24))
	assert.NoError(t, err, "GetFuturesTradeHistory should not error")
}

func TestGetAssets(t *testing.T) {
	t.Parallel()
	_, err := e.GetAssets(t.Context())
	assert.NoError(t, err, "GetAssets should not error")
}

func TestSeedAssetTranslator(t *testing.T) {
	t.Parallel()

	err := e.SeedAssets(t.Context())
	require.NoError(t, err, "SeedAssets must not error")

	for from, to := range map[string]string{"XBTUSD": "XXBTZUSD", "USD": "ZUSD", "XBT": "XXBT"} {
		assert.Equal(t, from, assetTranslator.LookupAltName(to), "LookupAltName should return the correct value")
		assert.Equal(t, to, assetTranslator.LookupCurrency(from), "LookupCurrency should return the correct value")
	}
}

func TestSeedAssets(t *testing.T) {
	t.Parallel()
	var a assetTranslatorStore
	assert.Empty(t, a.LookupAltName("ZUSD"), "LookupAltName on unseeded store should return empty")
	a.Seed("ZUSD", "USD")
	assert.Equal(t, "USD", a.LookupAltName("ZUSD"), "LookupAltName should return the correct value")
	a.Seed("ZUSD", "BLA")
	assert.Equal(t, "USD", a.LookupAltName("ZUSD"), "Store should ignore second reseed of existing currency")
}

func TestLookupCurrency(t *testing.T) {
	t.Parallel()
	var a assetTranslatorStore
	assert.Empty(t, a.LookupCurrency("USD"), "LookupCurrency on unseeded store should return empty")
	a.Seed("ZUSD", "USD")
	assert.Equal(t, "ZUSD", a.LookupCurrency("USD"), "LookupCurrency should return the correct value")
	assert.Empty(t, a.LookupCurrency("EUR"), "LookupCurrency should still not return an unseeded key")
}

func TestGetAssetPairs(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"fees", "leverage", "margin", ""} {
		_, err := e.GetAssetPairs(t.Context(), []string{}, v)
		require.NoErrorf(t, err, "GetAssetPairs %s must not error", v)
	}
}

func TestGetTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetTicker(t.Context(), spotTestPair)
	assert.NoError(t, err, "GetTicker should not error")
}

func TestGetTickers(t *testing.T) {
	t.Parallel()
	_, err := e.GetTickers(t.Context(), "LTCUSD,ETCUSD")
	assert.NoError(t, err, "GetTickers should not error")
}

func TestGetOHLC(t *testing.T) {
	t.Parallel()
	_, err := e.GetOHLC(t.Context(), currency.NewPairWithDelimiter("XXBT", "ZUSD", ""), "1440")
	assert.NoError(t, err, "GetOHLC should not error")
}

func TestGetDepth(t *testing.T) {
	t.Parallel()
	_, err := e.GetDepth(t.Context(), spotTestPair)
	assert.NoError(t, err, "GetDepth should not error")
}

func TestGetTrades(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	r, err := e.GetTrades(t.Context(), spotTestPair, time.Now().Add(-time.Hour*4), 1000)
	require.NoError(t, err, "GetTrades must not error")
	require.NotNil(t, r, "GetTrades must return a valid response")
}

func TestGetSpread(t *testing.T) {
	t.Parallel()
	p := currency.NewPair(currency.BCH, currency.EUR) // XBTUSD not in spread data
	r, err := e.GetSpread(t.Context(), p, time.Now().Add(-time.Hour*4))
	require.NoError(t, err, "GetSpread must not error")
	require.NotNil(t, r, "GetSpread must return a valid response")
	require.NotZero(t, r.Last.Time(), "GetSpread must return a valid last updated time")
	v, ok := r.Spreads[p.String()]
	require.True(t, ok, "GetSpread must return valid spread data for the given pair")
	assert.NotEmpty(t, v, "GetSpread should return some spread data for the given pair")
}

func TestGetBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetBalance(t.Context())
	assert.NoError(t, err, "GetBalance should not error")
}

func TestGetDepositMethods(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetDepositMethods(t.Context(), "USDT")
	assert.NoError(t, err, "GetDepositMethods should not error")
}

func TestGetTradeBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	args := TradeBalanceOptions{Asset: "ZEUR"}
	_, err := e.GetTradeBalance(t.Context(), args)
	assert.NoError(t, err)
}

func TestGetOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	args := OrderInfoOptions{Trades: true}
	_, err := e.GetOpenOrders(t.Context(), args)
	assert.NoError(t, err)
}

func TestGetClosedOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	args := GetClosedOrdersOptions{Trades: true, Start: "OE4KV4-4FVQ5-V7XGPU"}
	_, err := e.GetClosedOrders(t.Context(), args)
	assert.NoError(t, err)
}

func TestQueryOrdersInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	args := OrderInfoOptions{Trades: true}
	_, err := e.QueryOrdersInfo(t.Context(), args, "OR6ZFV-AA6TT-CKFFIW", "OAMUAJ-HLVKG-D3QJ5F")
	assert.NoError(t, err)
}

func TestGetTradesHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	args := GetTradesHistoryOptions{Trades: true, Start: "TMZEDR-VBJN2-NGY6DX", End: "TVRXG2-R62VE-RWP3UW"}
	_, err := e.GetTradesHistory(t.Context(), args)
	assert.NoError(t, err)
}

func TestQueryTrades(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.QueryTrades(t.Context(), true, "TMZEDR-VBJN2-NGY6DX", "TFLWIB-KTT7L-4TWR3L", "TDVRAH-2H6OS-SLSXRX")
	assert.NoError(t, err)
}

func TestOpenPositions(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.OpenPositions(t.Context(), false)
	assert.NoError(t, err)
}

// TestGetLedgers TODO: needs a positive test
func TestGetLedgers(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	args := GetLedgersOptions{Start: "LRUHXI-IWECY-K4JYGO", End: "L5NIY7-JZQJD-3J4M2V", Ofs: 15}
	_, err := e.GetLedgers(t.Context(), args)
	assert.ErrorContains(t, err, "EQuery:Unknown asset pair", "GetLedger should error on imaginary ledgers")
}

func TestQueryLedgers(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.QueryLedgers(t.Context(), "LVTSFS-NHZVM-EXNZ5M")
	assert.NoError(t, err)
}

func TestGetTradeVolume(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetTradeVolume(t.Context(), true, spotTestPair)
	assert.NoError(t, err, "GetTradeVolume should not error")
}

// TestOrders Tests AddOrder and CancelExistingOrder
func TestOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	args := AddOrderOptions{OrderFlags: "fcib"}
	cp, err := currency.NewPairFromString("XXBTZUSD")
	assert.NoError(t, err, "NewPairFromString should not error")
	resp, err := e.AddOrder(t.Context(),
		cp,
		order.Buy.Lower(), order.Limit.Lower(),
		0.0001, 9000, 9000, 0, &args)

	if assert.NoError(t, err, "AddOrder should not error") {
		if assert.Len(t, resp.TransactionIDs, 1, "One TransactionId should be returned") {
			id := resp.TransactionIDs[0]
			_, err = e.CancelExistingOrder(t.Context(), id)
			assert.NoErrorf(t, err, "CancelExistingOrder should not error, Please ensure order %s is cancelled manually", id)
		}
	}
}

func TestCancelExistingOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelExistingOrder(t.Context(), "OAVY7T-MV5VK-KHDF5X")
	if assert.Error(t, err, "Cancel with imaginary order-id should error") {
		assert.ErrorContains(t, err, "EOrder:Unknown order", "Cancel with imaginary order-id should error Unknown Order")
	}
}

func setFeeBuilder() *exchange.FeeBuilder {
	return &exchange.FeeBuilder{
		Amount:              1,
		FeeType:             exchange.CryptocurrencyTradeFee,
		Pair:                currency.NewPair(currency.XXBT, currency.ZUSD),
		PurchasePrice:       1,
		FiatCurrency:        currency.USD,
		BankTransactionType: exchange.WireTransfer,
	}
}

func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()
	f, err := e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFeeByType must not error")
	assert.Positive(t, f, "GetFeeByType should return a positive value")
	if !sharedtestvalues.AreAPICredentialsSet(e) {
		assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "GetFeeByType should set FeeType correctly")
	} else {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "GetFeeByType should set FeeType correctly")
	}
}

// TestGetFee exercises GetFee
func TestGetFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()

	if sharedtestvalues.AreAPICredentialsSet(e) {
		_, err := e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "CryptocurrencyTradeFee Basic GetFee should not error")

		feeBuilder = setFeeBuilder()
		feeBuilder.Amount = 1000
		feeBuilder.PurchasePrice = 1000
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "CryptocurrencyTradeFee High quantity GetFee should not error")

		feeBuilder = setFeeBuilder()
		feeBuilder.IsMaker = true
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "CryptocurrencyTradeFee IsMaker GetFee should not error")

		feeBuilder = setFeeBuilder()
		feeBuilder.PurchasePrice = -1000
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "CryptocurrencyTradeFee Negative purchase price GetFee should not error")

		feeBuilder = setFeeBuilder()
		feeBuilder.FeeType = exchange.InternationalBankDepositFee
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "InternationalBankDepositFee Basic GetFee should not error")
	}

	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyDepositFee
	feeBuilder.Pair.Base = currency.XXBT
	_, err := e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "CryptocurrencyDepositFee Basic GetFee should not error")

	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "CryptocurrencyWithdrawalFee Basic GetFee should not error")

	feeBuilder = setFeeBuilder()
	feeBuilder.Pair.Base = currency.NewCode("hello")
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "CryptocurrencyWithdrawalFee Invalid currency GetFee should not error")

	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.USD
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "InternationalBankWithdrawalFee Basic GetFee should not error")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	exp := exchange.AutoWithdrawCryptoWithSetupText + " & " + exchange.WithdrawCryptoWith2FAText + " & " + exchange.AutoWithdrawFiatWithSetupText + " & " + exchange.WithdrawFiatWith2FAText
	withdrawPermissions := e.FormatWithdrawPermissions()
	assert.Equal(t, exp, withdrawPermissions, "FormatWithdrawPermissions should return correct value")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Pairs:     currency.Pairs{spotTestPair},
		Side:      order.AnySide,
	}

	_, err := e.GetActiveOrders(t.Context(), &getOrdersRequest)
	assert.NoError(t, err, "GetActiveOrders should not error")
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
	assert.NoError(t, err)
}

// TestGetOrderInfo exercises GetOrderInfo
func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetOrderInfo(t.Context(), "OZPTPJ-HVYHF-EDIGXS", currency.EMPTYPAIR, asset.Spot)
	assert.ErrorContains(t, err, "order OZPTPJ-HVYHF-EDIGXS not found in response", "Should error that order was not found in response")
}

// Any tests below this line have the ability to impact your orders on the exchange. Enable canManipulateRealOrders to run them
// ----------------------------------------------------------------------------------------------------------------------------

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	orderSubmission := &order.Submit{
		Exchange:  e.Name,
		Pair:      spotTestPair,
		Side:      order.Buy,
		Type:      order.Limit,
		Price:     1,
		Amount:    1,
		ClientID:  "meowOrder",
		AssetType: asset.Spot,
	}
	response, err := e.SubmitOrder(t.Context(), orderSubmission)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "SubmitOrder should not error")
		assert.Equal(t, order.New, response.Status, "SubmitOrder should return a New order status")
	} else {
		assert.ErrorIs(t, err, exchange.ErrAuthenticationSupportNotEnabled, "SubmitOrder should error correctly")
	}
}

func TestCancelExchangeOrder(t *testing.T) {
	t.Parallel()

	err := e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.Options,
		OrderID:   "1337",
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "CancelOrder should error on Options asset")

	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	orderCancellation := &order.Cancel{
		OrderID:   "OGEX6P-B5Q74-IGZ72R",
		AssetType: asset.Spot,
	}

	err = e.CancelOrder(t.Context(), orderCancellation)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelOrder should not error")
	} else {
		assert.ErrorIs(t, err, exchange.ErrAuthenticationSupportNotEnabled, "CancelOrder should error correctly")
	}
}

func TestCancelBatchExchangeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	ordersCancellation := []order.Cancel{{
		Pair:      currency.NewPairWithDelimiter(currency.BTC.String(), currency.USD.String(), "/"),
		OrderID:   "OGEX6P-B5Q74-IGZ72R,OGEX6P-B5Q74-IGZ722",
		AssetType: asset.Spot,
	}}

	_, err := e.CancelBatchOrders(t.Context(), ordersCancellation)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelBatchOrder should not error")
	} else {
		assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "CancelBatchOrders should error correctly")
	}
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	resp, err := e.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Spot})

	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelAllOrders should not error")
	} else {
		assert.ErrorIs(t, err, exchange.ErrAuthenticationSupportNotEnabled, "CancelBatchOrders should error correctly")
	}

	assert.Empty(t, resp.Status, "CancelAllOrders Status should not contain any failed order errors")
}

// TestUpdateAccountBalances exercises UpdateAccountBalances
func TestUpdateAccountBalances(t *testing.T) {
	t.Parallel()

	for _, a := range []asset.Item{asset.Spot, asset.Futures} {
		_, err := e.UpdateAccountBalances(t.Context(), a)

		if sharedtestvalues.AreAPICredentialsSet(e) {
			assert.NoErrorf(t, err, "UpdateAccountBalances should not error for asset %s", a) // Note Well: Spot and Futures have separate api keys
		} else {
			assert.ErrorIsf(t, err, exchange.ErrAuthenticationSupportNotEnabled, "UpdateAccountBalances should error correctly for asset %s", a)
		}
	}
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()

	_, err := e.ModifyOrder(t.Context(), &order.Modify{AssetType: asset.Spot})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "ModifyOrder should error correctly")
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawCryptoRequest := withdraw.Request{
		Exchange: e.Name,
		Crypto: withdraw.CryptoRequest{
			Address: core.BitcoinDonationAddress,
		},
		Amount:        -1,
		Currency:      currency.XXBT,
		Description:   "WITHDRAW IT ALL",
		TradePassword: "Key",
	}

	_, err := e.WithdrawCryptocurrencyFunds(t.Context(),
		&withdrawCryptoRequest)
	if !sharedtestvalues.AreAPICredentialsSet(e) && err == nil {
		t.Error("Expecting an error when no keys are set")
	}
	if sharedtestvalues.AreAPICredentialsSet(e) && err != nil {
		t.Errorf("Withdraw failed to be placed: %v", err)
	}
}

func TestWithdrawFiat(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawFiatRequest := withdraw.Request{
		Amount:        -1,
		Currency:      currency.EUR,
		Description:   "WITHDRAW IT ALL",
		TradePassword: "someBank",
	}

	_, err := e.WithdrawFiatFunds(t.Context(), &withdrawFiatRequest)
	if !sharedtestvalues.AreAPICredentialsSet(e) && err == nil {
		t.Error("Expecting an error when no keys are set")
	}
	if sharedtestvalues.AreAPICredentialsSet(e) && err != nil {
		t.Errorf("Withdraw failed to be placed: %v", err)
	}
}

func TestWithdrawInternationalBank(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawFiatRequest := withdraw.Request{
		Amount:        -1,
		Currency:      currency.EUR,
		Description:   "WITHDRAW IT ALL",
		TradePassword: "someBank",
	}

	_, err := e.WithdrawFiatFundsToInternationalBank(t.Context(),
		&withdrawFiatRequest)
	if !sharedtestvalues.AreAPICredentialsSet(e) && err == nil {
		t.Error("Expecting an error when no keys are set")
	}
	if sharedtestvalues.AreAPICredentialsSet(e) && err != nil {
		t.Errorf("Withdraw failed to be placed: %v", err)
	}
}

func TestGetCryptoDepositAddress(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetCryptoDepositAddress(t.Context(), "Bitcoin", "XBT", false)
	if err != nil {
		t.Error(err)
	}
	if !canManipulateRealOrders {
		t.Skip("canManipulateRealOrders not set, skipping test")
	}
	_, err = e.GetCryptoDepositAddress(t.Context(), "Bitcoin", "XBT", true)
	if err != nil {
		t.Error(err)
	}
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	if sharedtestvalues.AreAPICredentialsSet(e) {
		_, err := e.GetDepositAddress(t.Context(), currency.USDT, "", "")
		if err != nil {
			t.Error("GetDepositAddress() error", err)
		}
	} else {
		_, err := e.GetDepositAddress(t.Context(), currency.BTC, "", "")
		if err == nil {
			t.Error("GetDepositAddress() error can not be nil")
		}
	}
}

func TestWithdrawStatus(t *testing.T) {
	t.Parallel()
	if sharedtestvalues.AreAPICredentialsSet(e) {
		_, err := e.WithdrawStatus(t.Context(), currency.BTC, "")
		if err != nil {
			t.Error("WithdrawStatus() error", err)
		}
	} else {
		_, err := e.WithdrawStatus(t.Context(), currency.BTC, "")
		if err == nil {
			t.Error("GetDepositAddress() error can not be nil")
		}
	}
}

func TestWithdrawCancel(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawCancel(t.Context(), currency.BTC, "")
	if sharedtestvalues.AreAPICredentialsSet(e) && err == nil {
		t.Error("WithdrawCancel() error cannot be nil")
	} else if !sharedtestvalues.AreAPICredentialsSet(e) && err == nil {
		t.Errorf("WithdrawCancel() error - expecting an error when no keys are set but received nil")
	}
}

// ---------------------------- Websocket tests -----------------------------------------

// TestWsSubscribe tests unauthenticated websocket subscriptions
// Specifically looking to ensure multiple errors are collected and returned and ws.Subscriptions Added/Removed in cases of:
// single pass, single fail, mixed fail, multiple pass, all fail
// No objection to this becoming a fixture test, so long as it integrates through Un/Subscribe roundtrip
func TestWsSubscribe(t *testing.T) {
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	testexch.SetupWs(t, e)

	for _, enabled := range []bool{false, true} {
		require.NoError(t, e.SetPairs(currency.Pairs{
			spotTestPair,
			currency.NewPairWithDelimiter("ETH", "USD", "/"),
			currency.NewPairWithDelimiter("LTC", "ETH", "/"),
			currency.NewPairWithDelimiter("ETH", "XBT", "/"),
			// Enable pairs that won't error locally, so we get upstream errors to test error combinations
			currency.NewPairWithDelimiter("DWARF", "HOBBIT", "/"),
			currency.NewPairWithDelimiter("DWARF", "GOBLIN", "/"),
			currency.NewPairWithDelimiter("DWARF", "ELF", "/"),
		}, asset.Spot, enabled), "SetPairs must not error")
	}

	err := e.Subscribe(subscription.List{{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{spotTestPair}}})
	require.NoError(t, err, "Simple subscription must not error")
	subs := e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "Should add 1 Subscription")
	assert.Equal(t, subscription.SubscribedState, subs[0].State(), "Subscription should be subscribed state")

	err = e.Subscribe(subscription.List{{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{spotTestPair}}})
	assert.ErrorIs(t, err, subscription.ErrDuplicate, "Resubscribing to the same channel should error with SubscribedAlready")
	subs = e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "Should not add a subscription on error")
	assert.Equal(t, subscription.SubscribedState, subs[0].State(), "Existing subscription state should not change")

	err = e.Subscribe(subscription.List{{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "HOBBIT", "/")}}})
	assert.ErrorContains(t, err, "Currency pair not supported; Channel: ticker Pairs: DWARF/HOBBIT", "Subscribing to an invalid pair should error correctly")
	require.Len(t, e.Websocket.GetSubscriptions(), 1, "Should not add a subscription on error")

	// Mix success and failure
	err = e.Subscribe(subscription.List{
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("ETH", "USD", "/")}},
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "HOBBIT", "/")}},
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "ELF", "/")}},
	})
	assert.ErrorContains(t, err, "Currency pair not supported; Channel: ticker Pairs:", "Subscribing to an invalid pair should error correctly")
	assert.ErrorContains(t, err, "DWARF/HOBBIT", "Subscribing to an invalid pair should error correctly")
	assert.ErrorContains(t, err, "DWARF/ELF", "Subscribing to an invalid pair should error correctly")
	require.Len(t, e.Websocket.GetSubscriptions(), 2, "Should have 2 subscriptions after mixed success/failures")

	// Just failures
	err = e.Subscribe(subscription.List{
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "HOBBIT", "/")}},
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "GOBLIN", "/")}},
	})
	assert.ErrorContains(t, err, "Currency pair not supported; Channel: ticker Pairs:", "Subscribing to an invalid pair should error correctly")
	assert.ErrorContains(t, err, "DWARF/HOBBIT", "Subscribing to an invalid pair should error correctly")
	assert.ErrorContains(t, err, "DWARF/GOBLIN", "Subscribing to an invalid pair should error correctly")
	require.Len(t, e.Websocket.GetSubscriptions(), 2, "Should have 2 subscriptions after mixed success/failures")

	// Just success
	err = e.Subscribe(subscription.List{
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("ETH", "XBT", "/")}},
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("LTC", "ETH", "/")}},
	})
	assert.NoError(t, err, "Multiple successful subscriptions should not error")

	subs = e.Websocket.GetSubscriptions()
	assert.Len(t, subs, 4, "Should have correct number of subscriptions")

	err = e.Unsubscribe(subs[:1])
	assert.NoError(t, err, "Simple Unsubscribe should succeed")
	assert.Len(t, e.Websocket.GetSubscriptions(), 3, "Should have removed 1 channel")

	err = e.Unsubscribe(subscription.List{{Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "WIZARD", "/")}, Key: 1337}})
	assert.ErrorIs(t, err, subscription.ErrNotFound, "Simple failing Unsubscribe should error NotFound")
	assert.ErrorContains(t, err, "DWARF/WIZARD", "Unsubscribing from an invalid pair should error correctly")
	assert.Len(t, e.Websocket.GetSubscriptions(), 3, "Should not have removed any channels")

	err = e.Unsubscribe(subscription.List{
		subs[1],
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "EAGLE", "/")}, Key: 1338},
	})
	assert.ErrorIs(t, err, subscription.ErrNotFound, "Mixed failing Unsubscribe should error NotFound")
	assert.ErrorContains(t, err, "Channel: ticker Pairs: DWARF/EAGLE", "Unsubscribing from an invalid pair should error correctly")

	subs = e.Websocket.GetSubscriptions()
	assert.Len(t, subs, 2, "Should have removed only 1 more channel")

	err = e.Unsubscribe(subs)
	assert.NoError(t, err, "Unsubscribe multiple passing subscriptions should not error")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "Should have successfully removed all channels")

	for _, c := range []string{"ohlc", "ohlc-5"} {
		err = e.Subscribe(subscription.List{{
			Asset:   asset.Spot,
			Channel: c,
			Pairs:   currency.Pairs{spotTestPair},
		}})
		assert.ErrorIs(t, err, subscription.ErrUseConstChannelName, "Must error when trying to use a private channel name")
		assert.ErrorContains(t, err, c+" => subscription.CandlesChannel", "Must error when trying to use a private channel name")
	}
}

// TestWsResubscribe tests websocket resubscription
func TestWsResubscribe(t *testing.T) {
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "TestInstance must not error")
	testexch.SetupWs(t, e)

	err := e.Subscribe(subscription.List{{Asset: asset.Spot, Channel: subscription.OrderbookChannel, Levels: 1000}})
	require.NoError(t, err, "Subscribe must not error")
	subs := e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "Should add 1 Subscription")
	require.Equal(t, subscription.SubscribedState, subs[0].State(), "Subscription must be in a subscribed state")

	require.Eventually(t, func() bool {
		b, e2 := e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
		if e2 == nil {
			return !b.LastUpdated.IsZero()
		}
		return false
	}, time.Second*4, time.Millisecond*10, "orderbook must start streaming")

	// Set the state to Unsub so we definitely know Resub worked
	err = subs[0].SetState(subscription.UnsubscribingState)
	require.NoError(t, err)

	err = e.Websocket.ResubscribeToChannel(t.Context(), e.Websocket.Conn, subs[0])
	require.NoError(t, err, "Resubscribe must not error")
	require.Equal(t, subscription.SubscribedState, subs[0].State(), "subscription must be subscribed again")
}

// TestWsOrderbookSub tests orderbook subscriptions for MaxDepth params
func TestWsOrderbookSub(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	testexch.SetupWs(t, e)

	err := e.Subscribe(subscription.List{{
		Asset:   asset.Spot,
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
		Levels:  25,
	}})
	require.NoError(t, err, "Simple subscription must not error")

	subs := e.Websocket.GetSubscriptions()
	require.Equal(t, 1, len(subs), "Must have 1 subscription channel")

	err = e.Unsubscribe(subs)
	assert.NoError(t, err, "Unsubscribe should not error")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "Should have successfully removed all channels")

	err = e.Subscribe(subscription.List{{
		Asset:   asset.Spot,
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
		Levels:  42,
	}})
	assert.ErrorContains(t, err, "Subscription depth not supported", "Bad subscription should error about depth")
}

// TestWsCandlesSub tests candles subscription for Timeframe params
func TestWsCandlesSub(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	testexch.SetupWs(t, e)

	err := e.Subscribe(subscription.List{{
		Asset:    asset.Spot,
		Channel:  subscription.CandlesChannel,
		Pairs:    currency.Pairs{spotTestPair},
		Interval: kline.OneHour,
	}})
	require.NoError(t, err, "Simple subscription must not error")

	subs := e.Websocket.GetSubscriptions()
	require.Equal(t, 1, len(subs), "Should add 1 Subscription")

	err = e.Unsubscribe(subs)
	assert.NoError(t, err, "Unsubscribe should not error")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "Should have successfully removed all channels")

	err = e.Subscribe(subscription.List{{
		Asset:    asset.Spot,
		Channel:  subscription.CandlesChannel,
		Pairs:    currency.Pairs{spotTestPair},
		Interval: kline.Interval(time.Minute * time.Duration(127)),
	}})
	assert.ErrorContains(t, err, "Subscription ohlc interval not supported", "Bad subscription should error about interval")
}

func TestWsProcessCandles(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup Instance must not error")

	err := ex.wsProcessCandles(t.Context(), []json.RawMessage{json.RawMessage(`{"symbol":"BTC/USD","interval":5,"interval_begin":"2018-11-12T20:35:14Z","open":3586.7,"high":3586.7,"low":3586.6,"close":3586.6,"volume":0.03373,"vwap":3586.68,"trades":2}`)})
	require.NoError(t, err, "valid candle data must not error")

	select {
	case msg := <-ex.Websocket.DataHandler.C:
		got, ok := msg.Data.(kline.Item)
		require.True(t, ok, "expected kline item")
		assert.Equal(t, kline.Item{
			Asset:    asset.Spot,
			Pair:     spotTestPair,
			Exchange: ex.Name,
			Interval: kline.FiveMin,
			Candles: []kline.Candle{{
				Time:        time.Date(2018, 11, 12, 20, 35, 14, 0, time.UTC),
				Open:        3586.7,
				High:        3586.7,
				Low:         3586.6,
				Close:       3586.6,
				Volume:      0.03373,
				QuoteVolume: 120.97871640000001,
			}},
		}, got)
	default:
		require.Fail(t, "expected websocket candle payload")
	}
}

// TestWsExecutionsSub tests the authenticated executions subscription channel.
func TestWsExecutionsSub(t *testing.T) {
	t.Parallel()

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	testexch.SetupWs(t, e)

	err := e.Subscribe(subscription.List{{Channel: subscription.MyAccountChannel, Authenticated: true}})
	assert.NoError(t, err, "Subscribing to executions should not error")

	subs := e.Websocket.GetSubscriptions()
	assert.Len(t, subs, 1, "Should add 1 Subscription")

	err = e.Unsubscribe(subs)
	assert.NoError(t, err, "Unsubscribing an auth channel should not error")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "Should have successfully removed channel")
}

func TestWsProcessSubStatusAuthenticated(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup Instance must not error")
	s := &subscription.Subscription{Channel: subscription.MyAccountChannel, QualifiedChannel: wsExecutions, Authenticated: true}
	require.NoError(t, ex.Websocket.AddSubscriptions(nil, s), "authenticated subscription must be added in subscribing state")

	ex.wsProcessSubStatus([]byte(`{"method":"subscribe","result":{"channel":"executions","snap_orders":true,"snap_trades":true},"success":true,"req_id":3}`))
	assert.Equal(t, subscription.SubscribedState, s.State(), "authenticated subscription status should be updated without requiring a pair field")
}

// TestGenerateSubscriptions tests the subscriptions generated from configuration
func TestGenerateSubscriptions(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup instance must not error")

	pairs, err := ex.GetEnabledPairs(asset.Spot)
	require.NoError(t, err, "GetEnabledPairs must not error")
	require.False(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "Websocket must not be authenticated by default")
	exp := subscription.List{
		{Channel: subscription.TickerChannel},
		{Channel: subscription.AllTradesChannel},
		{Channel: subscription.CandlesChannel, Interval: kline.OneMin},
		{Channel: subscription.OrderbookChannel, Levels: 1000},
	}
	for _, s := range exp {
		s.QualifiedChannel = channelName(s)
		s.Asset = asset.Spot
		s.Pairs = pairs
	}
	subs, err := ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")
	testsubs.EqualLists(t, exp, subs)

	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
	exp = append(exp, subscription.List{
		{Channel: subscription.MyAccountChannel, QualifiedChannel: wsExecutions},
	}...)
	subs, err = ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")
	testsubs.EqualLists(t, exp, subs)
}

func TestGetWSToken(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	testexch.SetupWs(t, e)

	resp, err := e.GetWebsocketToken(t.Context())
	require.NoError(t, err, "GetWebsocketToken must not error")
	assert.NotEmpty(t, resp, "Token should not be empty")
}

// TestWsAddOrder exercises roundtrip of wsAddOrder; See also: mockWsAddOrder
func TestWsAddOrder(t *testing.T) {
	t.Parallel()

	k := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	require.True(t, k.IsWebsocketAuthenticationSupported(), "WS must be authenticated")
	id, err := k.wsAddOrder(t.Context(), &WebsocketAddOrderParams{
		OrderType:  "limit",
		Side:       order.Buy.Lower(),
		Symbol:     "XBT/USD",
		LimitPrice: 80000,
		OrderQty:   1,
	})
	require.NoError(t, err, "wsAddOrder must not error")
	assert.Equal(t, "ONPNXH-KMKMU-F4MR5V", id, "wsAddOrder should return correct order ID")
}

// TestWsCancelOrders exercises roundtrip of wsCancelOrders; See also: mockWsCancelOrders
func TestWsCancelOrders(t *testing.T) {
	t.Parallel()

	k := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	require.True(t, k.IsWebsocketAuthenticationSupported(), "WS must be authenticated")

	err := k.wsCancelOrders(t.Context(), []string{"RABBIT", "BATFISH", "SQUIRREL", "CATFISH", "MOUSE"})
	assert.ErrorIs(t, err, errCancellingOrder, "Should error cancelling order")
	assert.ErrorContains(t, err, "BATFISH", "Should error containing txn id")
	assert.ErrorContains(t, err, "CATFISH", "Should error containing txn id")
	assert.ErrorContains(t, err, "EOrder:Unknown order", "Should error containing server error")

	err = k.wsCancelOrders(t.Context(), []string{"RABBIT", "SQUIRREL", "MOUSE"})
	assert.NoError(t, err, "Should not error with valid ids")
}

func TestWsCancelAllOrders(t *testing.T) {
	t.Parallel()

	k := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	count, err := k.wsCancelAllOrders(t.Context())
	require.NoError(t, err, "wsCancelAllOrders must not error")
	assert.Equal(t, int64(3), count, "wsCancelAllOrders should return the cancelled order count")
}

func TestWsHandleData(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		payload    string
		errContain string
	}{
		{name: "channel message", payload: `{"channel":"status","type":"update","data":[{"api_version":"v2","system":"online"}]}`},
		{name: "method response", payload: `{"method":"pong","req_id":1,"success":true}`},
		{name: "malformed", payload: `[`, errContain: "error unmarshalling WebSocket message envelope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex := new(Exchange)
			require.NoError(t, testexch.Setup(ex), "Setup Instance must not error")
			err := ex.wsHandleData(t.Context(), []byte(tc.payload))
			if tc.errContain != "" {
				require.ErrorContains(t, err, tc.errContain, "malformed payload must return the expected error")
				return
			}
			require.NoError(t, err, "valid payload must not error")
		})
	}
}

func TestCurrencyToExchange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    currency.Code
		expected currency.Code
	}{
		{input: currency.XBT, expected: currency.BTC},
		{input: currency.XXBT, expected: currency.BTC},
		{input: currency.XDG, expected: currency.DOGE},
		{input: currency.XXDG, expected: currency.DOGE},
		{input: currency.USD, expected: currency.USD},
	} {
		assert.Equal(t, tc.expected, currencyToExchange(tc.input), "currency should translate to the exchange symbol")
	}
}

func TestCurrencyFromExchange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    currency.Code
		expected currency.Code
	}{
		{input: currency.BTC, expected: currency.XBT},
		{input: currency.DOGE, expected: currency.XDG},
		{input: currency.USD, expected: currency.USD},
	} {
		assert.Equal(t, tc.expected, currencyFromExchange(tc.input), "currency should translate from the exchange symbol")
	}
}

func TestPairToExchange(t *testing.T) {
	t.Parallel()
	outbound := pairToExchange(currency.NewPair(currency.XBT, currency.XDG))
	assert.True(t, outbound.Equal(currency.NewPair(currency.BTC, currency.DOGE)), "pair should use exchange symbols")
}

func TestPairFromExchange(t *testing.T) {
	t.Parallel()
	inbound, err := pairFromExchange("BTC/DOGE")
	require.NoError(t, err, "valid symbol must parse")
	assert.Equal(t, currency.NewPair(currency.XBT, currency.XDG), inbound, "pair should use configured symbols and internal formatting")

	_, err = pairFromExchange("invalid")
	assert.Error(t, err, "invalid symbol should return an error")
}

func TestPairsToExchange(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		currency.Pairs{currency.NewBTCUSD(), currency.NewPair(currency.DOGE, currency.USD)},
		pairsToExchange(currency.Pairs{currency.NewPair(currency.XBT, currency.USD), currency.NewPair(currency.XDG, currency.USD)}),
		"pairs should use exchange symbols")
}

func TestWsOrderType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    string
		expected order.Type
	}{
		{input: "limit", expected: order.Limit},
		{input: "market", expected: order.Market},
		{input: "stop-loss", expected: order.Stop},
		{input: "stop-loss-limit", expected: order.StopLimit},
		{input: "take-profit", expected: order.TakeProfitMarket},
		{input: "take-profit-limit", expected: order.TakeProfit},
		{input: "trailing-stop", expected: order.TrailingStop},
		{input: "trailing-stop-limit", expected: order.TrailingStopLimit},
		{input: "unknown", expected: order.UnknownType},
	} {
		assert.Equal(t, tc.expected, wsOrderType(tc.input), "websocket order type should map to the expected type")
	}
}

func TestWsOrderTypeName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    order.Type
		expected string
	}{
		{input: order.Limit, expected: "limit"},
		{input: order.Market, expected: "market"},
		{input: order.Stop, expected: "stop-loss"},
		{input: order.StopLimit, expected: "stop-loss-limit"},
		{input: order.TakeProfitMarket, expected: "take-profit"},
		{input: order.TakeProfit, expected: "take-profit-limit"},
		{input: order.TrailingStop, expected: "trailing-stop"},
		{input: order.TrailingStopLimit, expected: "trailing-stop-limit"},
	} {
		actual, err := wsOrderTypeName(tc.input)
		require.NoError(t, err, "supported order type must not error")
		assert.Equal(t, tc.expected, actual, "order type name should match")
	}
	_, err := wsOrderTypeName(order.UnknownType)
	assert.ErrorIs(t, err, order.ErrTypeIsInvalid, "unsupported order type should return order.ErrTypeIsInvalid")
}

func TestWsOrderStatus(t *testing.T) {
	t.Parallel()
	status, err := wsOrderStatus("pending_new")
	require.NoError(t, err, "pending status must not error")
	assert.Equal(t, order.Pending, status, "pending_new should map to pending")
	status, err = wsOrderStatus("filled")
	require.NoError(t, err, "filled status must not error")
	assert.Equal(t, order.Filled, status, "filled should map to filled")
	_, err = wsOrderStatus("unsupported")
	assert.Error(t, err, "unsupported status should return an error")
}

func TestWsAddOrderParamsFromSubmit(t *testing.T) {
	t.Parallel()

	_, err := wsAddOrderParamsFromSubmit(nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "nil submission must return common.ErrNilPointer")

	endTime := time.Date(2026, 7, 17, 1, 2, 3, 0, time.UTC)
	params, err := wsAddOrderParamsFromSubmit(&order.Submit{
		Type:          order.Limit,
		Side:          order.Buy,
		Pair:          spotTestPair,
		TimeInForce:   order.GoodTillDay | order.PostOnly,
		ReduceOnly:    true,
		Leverage:      2,
		Price:         100,
		Amount:        2,
		ClientOrderID: "client-1",
		EndTime:       endTime,
	})
	require.NoError(t, err, "limit submission must map to websocket parameters")
	assert.Equal(t, &WebsocketAddOrderParams{
		ClientOrderID: "client-1",
		ExpireTime:    "2026-07-17T01:02:03Z",
		LimitPrice:    100,
		Margin:        true,
		OrderQty:      2,
		OrderType:     "limit",
		PostOnly:      true,
		ReduceOnly:    true,
		Side:          "buy",
		Symbol:        "BTC/USD",
		TimeInForce:   "gtd",
	}, params, "limit submission should map all supported websocket fields")

	params, err = wsAddOrderParamsFromSubmit(&order.Submit{
		Type:             order.StopLimit,
		Side:             order.Sell,
		Pair:             spotTestPair,
		Price:            99,
		TriggerPrice:     98,
		TriggerPriceType: order.IndexPrice,
		Amount:           1,
	})
	require.NoError(t, err, "stop-limit submission must map to websocket parameters")
	assert.Equal(t, 99.0, params.LimitPrice, "stop-limit price should match")
	assert.Equal(t, &WebsocketOrderTriggers{Price: 98, PriceType: "static", Reference: "index"}, params.Triggers, "stop-limit trigger should match")

	params, err = wsAddOrderParamsFromSubmit(&order.Submit{
		Type:             order.TrailingStop,
		Side:             order.Sell,
		Pair:             spotTestPair,
		Amount:           1,
		TriggerPriceType: order.LastPrice,
		TrackingMode:     order.Percentage,
		TrackingValue:    5,
	})
	require.NoError(t, err, "trailing-stop submission must map to websocket parameters")
	assert.Equal(t, &WebsocketOrderTriggers{Price: 5, PriceType: "pct", Reference: "last"}, params.Triggers, "trailing-stop trigger should match")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Stop, Side: order.Sell, Pair: spotTestPair, Amount: 1})
	require.ErrorIs(t, err, errTriggerPriceNotSet, "triggered submission without a trigger price must return the sentinel error")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.TrailingStop, Side: order.Sell, Pair: spotTestPair, Amount: 1})
	require.ErrorIs(t, err, errTrackingValueNotSet, "trailing-stop submission without a tracking value must return the sentinel error")
}

func TestWsTriggerReference(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		priceType order.PriceType
		expected  string
	}{
		{name: "last price", priceType: order.LastPrice, expected: "last"},
		{name: "index price", priceType: order.IndexPrice, expected: "index"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := wsTriggerReference(tc.priceType)
			require.NoError(t, err, "supported trigger reference must not error")
			assert.Equal(t, tc.expected, actual, "trigger reference should match")
		})
	}

	_, err := wsTriggerReference(order.PriceType(255))
	assert.ErrorIs(t, err, order.ErrUnknownPriceType, "unsupported trigger reference should return order.ErrUnknownPriceType")
}

func TestWsTrackingPriceType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		mode     order.TrackingMode
		expected string
	}{
		{name: "distance", mode: order.Distance, expected: "quote"},
		{name: "percentage", mode: order.Percentage, expected: "pct"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := wsTrackingPriceType(tc.mode)
			require.NoError(t, err, "supported tracking mode must not error")
			assert.Equal(t, tc.expected, actual, "tracking price type should match")
		})
	}

	_, err := wsTrackingPriceType(order.TrackingMode(255))
	assert.ErrorIs(t, err, order.ErrUnknownTrackingMode, "unsupported tracking mode should return order.ErrUnknownTrackingMode")
}

func TestWebsocketBookLevelUnmarshalJSON(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		payload     string
		expected    websocketBookLevel
		errContains string
	}{
		{name: "quoted", payload: `{"price":"45285.20","qty":"0.00100000"}`, expected: websocketBookLevel{Price: 45285.2, PriceString: "45285.20", Quantity: 0.001, QtyString: "0.00100000"}},
		{name: "numeric", payload: `{"price":45285.2,"qty":0.001}`, expected: websocketBookLevel{Price: 45285.2, PriceString: "45285.2", Quantity: 0.001, QtyString: "0.001"}},
		{name: "invalid price", payload: `{"price":"invalid","qty":"0.001"}`, errContains: "error parsing price"},
		{name: "invalid quantity", payload: `{"price":"45285.2","qty":"invalid"}`, errContains: "error parsing quantity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var level websocketBookLevel
			err := level.UnmarshalJSON([]byte(tc.payload))
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains, "invalid book level must return the expected parse error")
				return
			}
			require.NoError(t, err, "valid book level must unmarshal")
			assert.Equal(t, tc.expected, level, "book level should preserve its decimal representation")
		})
	}
}

func TestWsProcessStatus(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	for _, tc := range []struct {
		name        string
		data        []json.RawMessage
		errContains string
	}{
		{name: "online v2", data: []json.RawMessage{json.RawMessage(`{"api_version":"v2","system":"online"}`)}},
		{name: "missing item", errContains: "expected one status item"},
		{name: "offline", data: []json.RawMessage{json.RawMessage(`{"api_version":"v2","system":"maintenance"}`)}, errContains: "system status not online"},
		{name: "superseded API", data: []json.RawMessage{json.RawMessage(`{"api_version":"v1","system":"online"}`)}, errContains: "unsupported WebSocket API version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := e.wsProcessStatus(tc.data)
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains, "invalid v2 status must return the expected error")
				return
			}
			require.NoError(t, err, "online v2 status must not error")
		})
	}
}

func TestWsHandleMessage(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"heartbeat"}`)), "heartbeat message must not error")
	require.ErrorContains(t, e.wsHandleMessage(t.Context(), []byte(`{`)), "error unmarshalling WebSocket message", "malformed message must return the expected error")
}

func TestWsHandleResponse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		payload    string
		errIs      error
		errContain string
	}{
		{name: "pong", payload: `{"method":"pong","req_id":1,"success":true}`},
		{name: "server error", payload: `{"method":"add_order","error":"EOrder:Rejected","req_id":2,"success":false}`, errContain: "EOrder:Rejected"},
		{name: "unmatched request", payload: `{"method":"subscribe","req_id":3,"success":true}`, errIs: websocket.ErrSignatureNotMatched},
		{name: "malformed", payload: `{`, errContain: "error unmarshalling WebSocket response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex := new(Exchange)
			require.NoError(t, testexch.Setup(ex), "Setup Instance must not error")
			err := ex.wsHandleResponse(t.Context(), []byte(tc.payload))
			if tc.errIs != nil {
				require.ErrorIs(t, err, tc.errIs, "response must return the expected sentinel error")
			}
			if tc.errContain != "" {
				require.ErrorContains(t, err, tc.errContain, "response must return the expected error")
			}
			if tc.errIs == nil && tc.errContain == "" {
				require.NoError(t, err, "valid response must not error")
			}
		})
	}
}

func TestWsProcessTickers(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	require.NoError(t, e.wsProcessTickers(t.Context(), []json.RawMessage{json.RawMessage(`{"symbol":"BTC/USD","bid":100,"bid_qty":1,"ask":101,"ask_qty":2,"last":100.5,"volume":10,"vwap":100.2,"low":90,"high":110,"change":2,"timestamp":"2024-01-01T00:00:00Z"}`)}), "valid ticker data must not error")
	message := <-e.Websocket.DataHandler.C
	tick, ok := message.Data.(*ticker.Price)
	require.True(t, ok, "ticker message must contain a ticker price")
	assert.True(t, tick.Pair.Equal(spotTestPair), "ticker pair should translate BTC to XBT")
	assert.Equal(t, 101.0, tick.Ask, "ticker ask should match")
	assert.Equal(t, 100.0, tick.Bid, "ticker bid should match")
}

func TestWsProcessTrades(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	e.SetTradeFeedStatus(true)
	require.NoError(t, e.wsProcessTrades(t.Context(), []json.RawMessage{json.RawMessage(`{"symbol":"BTC/USD","side":"buy","ord_type":"limit","price":100.25,"qty":0.5,"trade_id":42,"timestamp":"2024-01-01T00:00:01Z"}`)}), "valid trade data must not error")
	message := <-e.Websocket.DataHandler.C
	got, ok := message.Data.(trade.Data)
	require.True(t, ok, "trade message must contain trade data")
	assert.True(t, got.CurrencyPair.Equal(spotTestPair), "trade pair should translate BTC to XBT")
	assert.Equal(t, "42", got.TID, "trade ID should match")
	assert.Equal(t, order.Buy, got.Side, "trade side should match")
}

func TestWsProcessExecutions(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	require.NoError(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"order_id":"ORDER-1","exec_id":"EXEC-1","exec_type":"trade","order_status":"partially_filled","order_type":"limit","side":"buy","symbol":"BTC/USD","order_qty":2,"cum_qty":0.5,"last_qty":0.5,"last_price":100,"limit_price":101,"avg_price":100,"fees":[{"asset":"USD","qty":0.2}],"timestamp":"2024-01-01T00:00:01Z"}`)}), "valid execution data must not error")

	message := <-e.Websocket.DataHandler.C
	detail, ok := message.Data.(*order.Detail)
	require.True(t, ok, "execution message must contain an order detail")
	assert.Equal(t, "ORDER-1", detail.OrderID, "order ID should match")
	assert.True(t, detail.Pair.Equal(spotTestPair), "execution pair should translate BTC to XBT")
	assert.Equal(t, order.PartiallyFilled, detail.Status, "order status should match")
	assert.Equal(t, 1.5, detail.RemainingAmount, "remaining amount should match")
	assert.Equal(t, 0.2, detail.Fee, "fee should match")
	require.Len(t, detail.Trades, 1, "trade execution must be attached to the order")
	assert.Equal(t, "EXEC-1", detail.Trades[0].TID, "execution ID should become the trade ID")
}

func TestWsProcessOrderbooks(t *testing.T) {
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	e.Name += "-WsProcessOrderbooks"
	require.NoError(t, e.Websocket.AddSuccessfulSubscriptions(e.Websocket.Conn, &subscription.Subscription{
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
		Asset:   asset.Spot,
		Levels:  10,
	}), "orderbook subscription must be added")

	payload := []byte(`{"channel":"book","type":"snapshot","data":[{"symbol":"BTC/USD","bids":[{"price":"45283.5","qty":"0.10000000"},{"price":"45283.4","qty":"1.54582015"},{"price":"45282.1","qty":"0.10000000"},{"price":"45281.0","qty":"0.10000000"},{"price":"45280.3","qty":"1.54592586"},{"price":"45279.0","qty":"0.07990000"},{"price":"45277.6","qty":"0.03310103"},{"price":"45277.5","qty":"0.30000000"},{"price":"45277.3","qty":"1.54602737"},{"price":"45276.6","qty":"0.15445238"}],"asks":[{"price":"45285.2","qty":"0.00100000"},{"price":"45286.4","qty":"1.54571953"},{"price":"45286.6","qty":"1.54571109"},{"price":"45289.6","qty":"1.54560911"},{"price":"45290.2","qty":"0.15890660"},{"price":"45291.8","qty":"1.54553491"},{"price":"45294.7","qty":"0.04454749"},{"price":"45296.1","qty":"0.35380000"},{"price":"45297.5","qty":"0.09945542"},{"price":"45299.5","qty":"0.18772827"}],"checksum":3310070434}]}`)
	var message websocketMessage
	require.NoError(t, json.Unmarshal(payload, &message), "official checksum example must unmarshal")
	require.NoError(t, e.wsProcessOrderbooks(t.Context(), message.Type, message.Data), "official checksum example must validate")

	book, err := e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.NoError(t, err, "orderbook must be stored")
	require.Len(t, book.Bids, 10, "book must retain ten bids")
	require.Len(t, book.Asks, 10, "book must retain ten asks")
	assert.Equal(t, "45285.2", book.Asks[0].StrPrice, "raw price precision should be retained")
	assert.Equal(t, "0.00100000", book.Asks[0].StrAmount, "raw quantity precision should be retained")
}

func TestWsSubscriptionForPair(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	sub := &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{spotTestPair}}
	require.NoError(t, e.Websocket.AddSuccessfulSubscriptions(e.Websocket.Conn, sub), "orderbook subscription must be added")
	assert.Same(t, sub, e.wsSubscriptionForPair(subscription.OrderbookChannel, spotTestPair), "matching subscription should be returned")
	assert.Nil(t, e.wsSubscriptionForPair(subscription.TickerChannel, spotTestPair), "non-matching subscription should not be returned")
}

func TestWsProcessOrderbookSnapshot(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	e.Name += "-WsProcessOrderbookSnapshot"
	snapshot := &websocketBook{
		Asks:     []websocketBookLevel{{Price: 101, PriceString: "101.0", Quantity: 2, QtyString: "2.000"}},
		Bids:     []websocketBookLevel{{Price: 100, PriceString: "100.0", Quantity: 1, QtyString: "1.000"}},
		Checksum: 155902695,
		Symbol:   "BTC/USD",
	}
	require.NoError(t, e.wsProcessOrderbookSnapshot(spotTestPair, 10, snapshot), "valid snapshot must not error")
	book, err := e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.NoError(t, err, "stored orderbook must be available")
	require.Len(t, book.Asks, 1, "stored orderbook must contain one ask")
	require.Len(t, book.Bids, 1, "stored orderbook must contain one bid")
	assert.Equal(t, "2.000", book.Asks[0].StrAmount, "snapshot should preserve raw quantity precision")
}

func TestWsProcessOrderbookUpdate(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup Instance must not error")
	e.Name += "-WsProcessOrderbookUpdate"
	require.NoError(t, e.Websocket.Orderbook.LoadSnapshot(&orderbook.Book{
		Pair:                   spotTestPair,
		Asset:                  asset.Spot,
		Exchange:               e.Name,
		LastUpdated:            time.Now(),
		ChecksumStringRequired: true,
		Asks:                   orderbook.Levels{{Price: 101, Amount: 2, StrPrice: "101.0", StrAmount: "2.000"}},
		Bids:                   orderbook.Levels{{Price: 100, Amount: 1, StrPrice: "100.0", StrAmount: "1.000"}},
	}), "initial orderbook must load")
	update := &websocketBook{
		Bids:     []websocketBookLevel{{Price: 100, PriceString: "100.0", Quantity: 1.5, QtyString: "1.500"}},
		Checksum: 260120588,
	}
	require.NoError(t, e.wsProcessOrderbookUpdate(spotTestPair, update), "valid update must not error")
	book, err := e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.NoError(t, err, "updated orderbook must be available")
	require.Len(t, book.Bids, 1, "updated orderbook must contain one bid")
	assert.Equal(t, 1.5, book.Bids[0].Amount, "bid amount should be updated")
	assert.Equal(t, "1.500", book.Bids[0].StrAmount, "update should preserve raw quantity precision")
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)

	_, err := e.GetHistoricCandles(t.Context(), spotTestPair, asset.Spot, kline.OneHour, time.Now().Add(-time.Hour*12), time.Now())
	assert.NoError(t, err, "GetHistoricCandles should not error")

	_, err = e.GetHistoricCandles(t.Context(), futuresTestPair, asset.Futures, kline.OneHour, time.Now().Add(-time.Hour*12), time.Now())
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetHistoricCandles should error with asset.ErrNotSupported")
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricCandlesExtended(t.Context(), futuresTestPair, asset.Spot, kline.OneMin, time.Now().Add(-time.Minute*3), time.Now())
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricCandlesExtended should error correctly")
}

func TestFormatExchangeKlineInterval(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		interval kline.Interval
		exp      string
	}{
		{kline.OneMin, "1"},
		{kline.OneDay, "1440"},
	} {
		assert.Equalf(t, tt.exp, e.FormatExchangeKlineInterval(tt.interval), "FormatExchangeKlineInterval should return correct output for %s", tt.interval.Short())
	}
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)

	_, err := e.GetRecentTrades(t.Context(), spotTestPair, asset.Spot)
	assert.NoError(t, err, "GetRecentTrades should not error")

	_, err = e.GetRecentTrades(t.Context(), futuresTestPair, asset.Futures)
	assert.NoError(t, err, "GetRecentTrades should not error")
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricTrades(t.Context(), spotTestPair, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricTrades should error")
}

var testOb = orderbook.Book{
	Asks: []orderbook.Level{
		// NOTE: 0.00000500 float64 == 0.000005
		{Price: 0.05005, StrPrice: "0.05005", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05010, StrPrice: "0.05010", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05015, StrPrice: "0.05015", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05020, StrPrice: "0.05020", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05025, StrPrice: "0.05025", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05030, StrPrice: "0.05030", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05035, StrPrice: "0.05035", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05040, StrPrice: "0.05040", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05045, StrPrice: "0.05045", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05050, StrPrice: "0.05050", Amount: 0.00000500, StrAmount: "0.00000500"},
	},
	Bids: []orderbook.Level{
		{Price: 0.05000, StrPrice: "0.05000", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04995, StrPrice: "0.04995", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04990, StrPrice: "0.04990", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04980, StrPrice: "0.04980", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04975, StrPrice: "0.04975", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04970, StrPrice: "0.04970", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04965, StrPrice: "0.04965", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04960, StrPrice: "0.04960", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04955, StrPrice: "0.04955", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04950, StrPrice: "0.04950", Amount: 0.00000500, StrAmount: "0.00000500"},
	},
}

const krakenAPIDocChecksum = 974947235

func TestChecksumCalculation(t *testing.T) {
	t.Parallel()
	expected := "5005"
	if v := trim("0.05005"); v != expected {
		t.Errorf("expected %s but received %s", expected, v)
	}

	expected = "500"
	if v := trim("0.00000500"); v != expected {
		t.Errorf("expected %s but received %s", expected, v)
	}

	err := validateCRC32(&testOb, krakenAPIDocChecksum)
	if err != nil {
		t.Error(err)
	}
}

func TestGetCharts(t *testing.T) {
	t.Parallel()
	resp, err := e.GetFuturesCharts(t.Context(), "1d", "spot", futuresTestPair, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Candles)

	end := resp.Candles[0].Time.Time()
	_, err = e.GetFuturesCharts(t.Context(), "1d", "spot", futuresTestPair, end.Add(-time.Hour*24*7), end)
	require.NoError(t, err)
}

func TestGetFuturesTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesTrades(t.Context(), futuresTestPair, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetFuturesTrades should not error")

	_, err = e.GetFuturesTrades(t.Context(), futuresTestPair, time.Now().Add(-time.Hour), time.Now())
	assert.NoError(t, err, "GetFuturesTrades should not error")
}

func TestGetFuturesContractDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesContractDetails(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, futures.ErrNotFuturesAsset)

	_, err = e.GetFuturesContractDetails(t.Context(), asset.USDTMarginedFutures)
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.GetFuturesContractDetails(t.Context(), asset.Futures)
	assert.NoError(t, err, "GetFuturesContractDetails should not error")
}

func TestGetLatestFundingRates(t *testing.T) {
	t.Parallel()
	_, err := e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 currency.NewBTCUSD(),
		IncludePredictedRate: true,
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetLatestFundingRates should error")

	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.Futures,
	})
	assert.NoError(t, err, "GetLatestFundingRates should not error")

	err = e.CurrencyPairs.EnablePair(asset.Futures, futuresTestPair)
	assert.Truef(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled), "EnablePair should not error: %s", err)
	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset:                asset.Futures,
		Pair:                 futuresTestPair,
		IncludePredictedRate: true,
	})
	assert.NoError(t, err, "GetLatestFundingRates should not error")
}

func TestIsPerpetualFutureCurrency(t *testing.T) {
	t.Parallel()
	is, err := e.IsPerpetualFutureCurrency(asset.Binary, currency.NewBTCUSDT())
	assert.NoError(t, err)
	assert.False(t, is, "IsPerpetualFutureCurrency should return false for a binary asset")

	is, err = e.IsPerpetualFutureCurrency(asset.Futures, currency.NewBTCUSDT())
	assert.NoError(t, err)
	assert.False(t, is, "IsPerpetualFutureCurrency should return false for a non-perpetual future")

	is, err = e.IsPerpetualFutureCurrency(asset.Futures, futuresTestPair)
	assert.NoError(t, err)
	assert.True(t, is, "IsPerpetualFutureCurrency should return true for a perpetual future")
}

func TestGetOpenInterest(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")

	_, err := e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.ETH.Item,
		Quote: currency.USDT.Item,
		Asset: asset.USDTMarginedFutures,
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	cp1 := currency.NewPair(currency.PF, currency.NewCode("XBTUSD"))
	cp2 := currency.NewPair(currency.PF, currency.NewCode("ETHUSD"))
	sharedtestvalues.SetupCurrencyPairsForExchangeAsset(t, e, asset.Futures, cp1, cp2)

	resp, err := e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  cp1.Base.Item,
		Quote: cp1.Quote.Item,
		Asset: asset.Futures,
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, resp)

	resp, err = e.GetOpenInterest(t.Context(),
		key.PairAsset{
			Base:  cp1.Base.Item,
			Quote: cp1.Quote.Item,
			Asset: asset.Futures,
		},
		key.PairAsset{
			Base:  cp2.Base.Item,
			Quote: cp2.Quote.Item,
			Asset: asset.Futures,
		})
	assert.NoError(t, err)
	assert.NotEmpty(t, resp)

	resp, err = e.GetOpenInterest(t.Context())
	assert.NoError(t, err)
	assert.NotEmpty(t, resp)
}

// curryWsMockUpgrader handles Kraken specific http auth token responses prior to handling off to standard Websocket upgrader
func curryWsMockUpgrader(tb testing.TB, h mockws.WsMockFunc) http.HandlerFunc {
	tb.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "GetWebSocketsToken") {
			_, err := w.Write([]byte(`{"result":{"token":"mockAuth"}}`))
			assert.NoError(tb, err, "Write should not error")
			return
		}
		mockws.WsMockUpgrader(tb, w, r, h)
	}
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		if len(pairs) == 0 {
			continue
		}
		require.NoErrorf(t, err, "cannot get pairs for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		if a != asset.Spot && a != asset.Futures {
			assert.ErrorIs(t, err, asset.ErrNotSupported)
			continue
		}
		require.NoError(t, err)
		assert.NotEmpty(t, resp)
	}
}

func TestErrorResponse(t *testing.T) {
	var g genericRESTResponse

	tests := []struct {
		name          string
		jsonStr       string
		expectError   bool
		errorMsg      string
		warningMsg    string
		requiresReset bool
	}{
		{
			name:    "No errors or warnings",
			jsonStr: `{"error":[],"result":{"unixtime":1721884425,"rfc1123":"Thu, 25 Jul 24 05:13:45 +0000"}}`,
		},
		{
			name:        "Invalid error type int",
			jsonStr:     `{"error":[69420],"result":{}}`,
			expectError: true,
			errorMsg:    "unable to convert 69420 to string",
		},
		{
			name:        "Unhandled error type float64",
			jsonStr:     `{"error":124,"result":{}}`,
			expectError: true,
			errorMsg:    "unhandled error response type float64",
		},
		{
			name:     "Known error string",
			jsonStr:  `{"error":["EQuery:Unknown asset pair"],"result":{}}`,
			errorMsg: "EQuery:Unknown asset pair",
		},
		{
			name:     "Known error string (single)",
			jsonStr:  `{"error":"EService:Unavailable","result":{}}`,
			errorMsg: "EService:Unavailable",
		},
		{
			name:          "Warning string in array",
			jsonStr:       `{"error":["WGeneral:Danger"],"result":{}}`,
			warningMsg:    "WGeneral:Danger",
			requiresReset: true,
		},
		{
			name:          "Warning string",
			jsonStr:       `{"error":"WGeneral:Unknown warning","result":{}}`,
			warningMsg:    "WGeneral:Unknown warning",
			requiresReset: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.requiresReset {
				g = genericRESTResponse{}
			}
			err := json.Unmarshal([]byte(tt.jsonStr), &g)
			if tt.expectError {
				require.ErrorContains(t, err, tt.errorMsg, "Unmarshal must error")
			} else {
				require.NoError(t, err)
				if tt.errorMsg != "" {
					assert.ErrorContainsf(t, g.Error.Errors(), tt.errorMsg, "Errors should contain %s", tt.errorMsg)
				} else {
					assert.NoError(t, g.Error.Errors(), "Errors should not error")
				}
				if tt.warningMsg != "" {
					assert.Containsf(t, g.Error.Warnings(), tt.warningMsg, "Warnings should contain %s", tt.warningMsg)
				} else {
					assert.Empty(t, g.Error.Warnings(), "Warnings should be empty")
				}
			}
		})
	}
}

func TestGetFuturesErr(t *testing.T) {
	t.Parallel()

	assert.ErrorContains(t, getFuturesErr(json.RawMessage(`unparsable rubbish`)), "invalid char", "Bad JSON should error correctly")
	assert.NoError(t, getFuturesErr(json.RawMessage(`{"candles":[]}`)), "JSON with no Result should not error")
	assert.NoError(t, getFuturesErr(json.RawMessage(`{"Result":"4 goats"}`)), "JSON with non-error Result should not error")
	assert.ErrorIs(t, getFuturesErr(json.RawMessage(`{"Result":"error"}`)), common.ErrUnknownError, "JSON with error Result should error correctly")
	assert.ErrorContains(t, getFuturesErr(json.RawMessage(`{"Result":"error", "error": "1 goat"}`)), "1 goat", "JSON with an error should error correctly")
	err := getFuturesErr(json.RawMessage(`{"Result":"error", "errors": ["2 goats", "3 goats"]}`))
	assert.ErrorContains(t, err, "2 goat", "JSON with errors should error correctly")
	assert.ErrorContains(t, err, "3 goat", "JSON with errors should error correctly")
	err = getFuturesErr(json.RawMessage(`{"Result":"error", "error": "too many goats", "errors": ["2 goats", "3 goats"]}`))
	assert.ErrorContains(t, err, "2 goat", "JSON with both error and errors should error correctly")
	assert.ErrorContains(t, err, "3 goat", "JSON with both error and errors should error correctly")
	assert.ErrorContains(t, err, "too many goat", "JSON both error and with errors should error correctly")
}

func TestEnforceStandardChannelNames(t *testing.T) {
	for _, n := range []string{
		wsTicker, subscription.TickerChannel, subscription.OrderbookChannel, subscription.CandlesChannel,
		subscription.AllTradesChannel, subscription.MyAccountChannel,
	} {
		assert.NoError(t, enforceStandardChannelNames(&subscription.Subscription{Channel: n}), "Standard channel names and bespoke names should not error")
	}
	for _, n := range []string{wsOrderbook, wsOHLC, wsTrade, wsExecutions, wsOrderbook + "-5"} {
		err := enforceStandardChannelNames(&subscription.Subscription{Channel: n})
		assert.ErrorIsf(t, err, subscription.ErrUseConstChannelName, "Private channel names should not be allowed for %s", n)
	}
	for _, n := range []string{subscription.MyTradesChannel, subscription.MyOrdersChannel} {
		err := enforceStandardChannelNames(&subscription.Subscription{Channel: n})
		assert.ErrorIsf(t, err, subscription.ErrNotSupported, "Superseded private channel names should not be supported for %s", n)
	}
}

func TestWebsocketAuthToken(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	e.setWebsocketAuthToken("meep")
	const n = 69
	var wg sync.WaitGroup
	wg.Add(2 * n)

	start := make(chan struct{})
	for range n {
		go func() {
			defer wg.Done()
			<-start
			e.setWebsocketAuthToken("69420")
		}()
	}
	for range n {
		go func() {
			defer wg.Done()
			<-start
			e.websocketAuthToken()
		}()
	}
	close(start)
	wg.Wait()
	assert.Equal(t, "69420", e.websocketAuthToken(), "websocketAuthToken should return correctly after concurrent reads and writes")
}

func TestSetWebsocketAuthToken(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	e.setWebsocketAuthToken("69420")
	assert.Equal(t, "69420", e.websocketAuthToken())
}
