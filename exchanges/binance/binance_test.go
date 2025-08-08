package binance

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/collateral"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/futures"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Please supply your own keys here for due diligence testing
const (
	apiKey                  = ""
	apiSecret               = ""
	canManipulateRealOrders = false
	useTestNet              = false
)

var (
	e *Exchange
	// this pair is used to ensure that endpoints match it correctly
	testPairMapping = currency.NewPair(currency.DOGE, currency.USDT)
)

func setFeeBuilder() *exchange.FeeBuilder {
	return &exchange.FeeBuilder{
		Amount:        1,
		FeeType:       exchange.CryptocurrencyTradeFee,
		Pair:          currency.NewPair(currency.BTC, currency.LTC),
		PurchasePrice: 1,
	}
}

// getTime returns a static time for mocking endpoints, if mock is not enabled
// this will default to time now with a window size of 30 days.
// Mock details are unix seconds; start = 1577836800 and end = 1580515200
func getTime() (start, end time.Time) {
	if mockTests {
		return time.Unix(1577836800, 0), time.Unix(1580515200, 0)
	}

	tn := time.Now()
	offset := time.Hour * 24 * 6
	return tn.Add(-offset), tn
}

func TestUServerTime(t *testing.T) {
	t.Parallel()
	_, err := e.UServerTime(t.Context())
	assert.NoError(t, err, "UServerTime should not return an error")
}

func TestWrapperGetServerTime(t *testing.T) {
	t.Parallel()
	_, err := e.GetServerTime(t.Context(), asset.Empty)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetServerTime with empty asset must error")

	st, err := e.GetServerTime(t.Context(), asset.Spot)
	require.NoError(t, err, "GetServerTime for spot must not error")
	assert.False(t, st.IsZero(), "GetServerTime for spot should return a time")

	st, err = e.GetServerTime(t.Context(), asset.USDTMarginedFutures)
	require.NoError(t, err, "GetServerTime for USDT margined futures must not error")
	assert.False(t, st.IsZero(), "GetServerTime for USDT margined futures should return a time")

	st, err = e.GetServerTime(t.Context(), asset.CoinMarginedFutures)
	require.NoError(t, err, "GetServerTime for coin margined futures must not error")
	assert.False(t, st.IsZero(), "GetServerTime for coin margined futures should return a time")
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	r, err := e.UpdateTicker(t.Context(), testPairMapping, asset.Spot)
	require.NoError(t, err, "UpdateTicker for spot must not error")
	assert.Equal(t, currency.DOGE, r.Pair.Base, "invalid pair base")
	assert.Equal(t, currency.USDT, r.Pair.Quote, "invalid pair quote")

	tradablePairs, err := e.FetchTradablePairs(t.Context(), asset.CoinMarginedFutures)
	require.NoError(t, err, "FetchTradablePairs for coin margined futures must not error")
	require.NotEmpty(t, tradablePairs, "tradable pairs for coin margined futures should not be empty")

	_, err = e.UpdateTicker(t.Context(), tradablePairs[0], asset.CoinMarginedFutures)
	assert.NoError(t, err, "UpdateTicker for coin margined futures should not error")

	usdtMarginedPairs, err := e.FetchTradablePairs(t.Context(), asset.USDTMarginedFutures)
	require.NoError(t, err, "FetchTradablePairs for USDT margined futures must not error")
	require.NotEmpty(t, usdtMarginedPairs, "tradable pairs for USDT margined futures should not be empty")

	_, err = e.UpdateTicker(t.Context(), usdtMarginedPairs[0], asset.USDTMarginedFutures)
	assert.NoError(t, err, "UpdateTicker for USDT margined futures should not error")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	err := e.UpdateTickers(t.Context(), asset.Spot)
	assert.NoError(t, err, "UpdateTickers for spot should not error")

	err = e.UpdateTickers(t.Context(), asset.CoinMarginedFutures)
	assert.NoError(t, err, "UpdateTickers for coin margined futures should not error")

	err = e.UpdateTickers(t.Context(), asset.USDTMarginedFutures)
	assert.NoError(t, err, "UpdateTickers for USDT margined futures should not error")
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	cp, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString should not error")

	_, err = e.UpdateOrderbook(t.Context(), cp, asset.Spot)
	assert.NoError(t, err, "UpdateOrderbook for spot should not error")

	_, err = e.UpdateOrderbook(t.Context(), cp, asset.Margin)
	assert.NoError(t, err, "UpdateOrderbook for margin should not error")

	_, err = e.UpdateOrderbook(t.Context(), cp, asset.USDTMarginedFutures)
	assert.NoError(t, err, "UpdateOrderbook for USDT margined futures should not error")

	cp2, err := currency.NewPairFromString("BTCUSD_PERP")
	require.NoError(t, err, "NewPairFromString should not error")

	_, err = e.UpdateOrderbook(t.Context(), cp2, asset.CoinMarginedFutures)
	assert.NoError(t, err, "UpdateOrderbook for coin margined futures should not error")
}

// USDT Margined Futures

func TestUExchangeInfo(t *testing.T) {
	t.Parallel()
	_, err := e.UExchangeInfo(t.Context())
	assert.NoError(t, err, "UExchangeInfo should not error")
}

func TestUFuturesOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesOrderbook(t.Context(), currency.NewBTCUSDT(), 1000)
	assert.NoError(t, err, "UFuturesOrderbook should not error")
}

func TestURecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.URecentTrades(t.Context(), currency.NewBTCUSDT(), "", 1000)
	assert.NoError(t, err, "URecentTrades should not error")
}

func TestUCompressedTrades(t *testing.T) {
	t.Parallel()
	_, err := e.UCompressedTrades(t.Context(), currency.NewBTCUSDT(), "", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UCompressedTrades should not error")

	start, end := getTime()
	_, err = e.UCompressedTrades(t.Context(), currency.NewPair(currency.LTC, currency.USDT), "", 0, start, end)
	assert.NoError(t, err, "UCompressedTrades should not error")
}

func TestUKlineData(t *testing.T) {
	t.Parallel()
	_, err := e.UKlineData(t.Context(), currency.NewBTCUSDT(), "1d", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UKlineData should not error")
	start, end := getTime()
	_, err = e.UKlineData(t.Context(), currency.NewPair(currency.LTC, currency.USDT), "5m", 0, start, end)
	assert.NoError(t, err, "UKlineData should not error")
}

func TestUGetMarkPrice(t *testing.T) {
	t.Parallel()
	_, err := e.UGetMarkPrice(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UGetMarkPrice should not error")
	_, err = e.UGetMarkPrice(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "UGetMarkPrice should not error")
}

func TestUGetFundingHistory(t *testing.T) {
	t.Parallel()
	_, err := e.UGetFundingHistory(t.Context(), currency.NewBTCUSDT(), 1, time.Time{}, time.Time{})
	assert.NoError(t, err, "UGetFundingHistory should not error")

	start, end := getTime()
	_, err = e.UGetFundingHistory(t.Context(), currency.NewPair(currency.LTC, currency.USDT), 1, start, end)
	assert.NoError(t, err, "UGetFundingHistory should not error")
}

func TestU24HTickerPriceChangeStats(t *testing.T) {
	t.Parallel()
	_, err := e.U24HTickerPriceChangeStats(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "U24HTickerPriceChangeStats should not error")
	_, err = e.U24HTickerPriceChangeStats(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "U24HTickerPriceChangeStats should not error")
}

func TestUSymbolPriceTicker(t *testing.T) {
	t.Parallel()
	_, err := e.USymbolPriceTicker(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "USymbolPriceTicker should not error")
	_, err = e.USymbolPriceTicker(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "USymbolPriceTicker should not error")
}

func TestUSymbolOrderbookTicker(t *testing.T) {
	t.Parallel()
	_, err := e.USymbolOrderbookTicker(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "USymbolOrderbookTicker should not error")
	_, err = e.USymbolOrderbookTicker(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "USymbolOrderbookTicker should not error")
}

func TestUOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := e.UOpenInterest(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UOpenInterest should not error")
}

func TestUOpenInterestStats(t *testing.T) {
	t.Parallel()
	_, err := e.UOpenInterestStats(t.Context(), currency.NewBTCUSDT(), "5m", 1, time.Time{}, time.Time{})
	assert.NoError(t, err, "UOpenInterestStats should not error")

	start, end := getTime()
	_, err = e.UOpenInterestStats(t.Context(), currency.NewPair(currency.LTC, currency.USDT), "1d", 10, start, end)
	assert.NoError(t, err, "UOpenInterestStats should not error")
}

func TestUTopAcccountsLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := e.UTopAcccountsLongShortRatio(t.Context(), currency.NewBTCUSDT(), "5m", 2, time.Time{}, time.Time{})
	assert.NoError(t, err, "UTopAcccountsLongShortRatio should not error")
	start, end := getTime()
	_, err = e.UTopAcccountsLongShortRatio(t.Context(), currency.NewBTCUSDT(), "5m", 2, start, end)
	assert.NoError(t, err, "UTopAcccountsLongShortRatio should not error")
}

func TestUTopPostionsLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := e.UTopPostionsLongShortRatio(t.Context(), currency.NewBTCUSDT(), "5m", 3, time.Time{}, time.Time{})
	assert.NoError(t, err, "UTopPostionsLongShortRatio should not error")
	start, end := getTime()
	_, err = e.UTopPostionsLongShortRatio(t.Context(), currency.NewBTCUSDT(), "1d", 0, start, end)
	assert.NoError(t, err, "UTopPostionsLongShortRatio should not error")
}

func TestUGlobalLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := e.UGlobalLongShortRatio(t.Context(), currency.NewBTCUSDT(), "5m", 3, time.Time{}, time.Time{})
	assert.NoError(t, err, "UGlobalLongShortRatio should not error")
	start, end := getTime()
	_, err = e.UGlobalLongShortRatio(t.Context(), currency.NewBTCUSDT(), "4h", 0, start, end)
	assert.NoError(t, err, "UGlobalLongShortRatio should not error")
}

func TestUTakerBuySellVol(t *testing.T) {
	t.Parallel()
	start, end := getTime()
	_, err := e.UTakerBuySellVol(t.Context(), currency.NewBTCUSDT(), "5m", 10, start, end)
	assert.NoError(t, err, "UTakerBuySellVol should not error")
}

func TestUCompositeIndexInfo(t *testing.T) {
	t.Parallel()
	cp, err := currency.NewPairFromString("DEFI-USDT")
	require.NoError(t, err, "NewPairFromString should not error")

	_, err = e.UCompositeIndexInfo(t.Context(), cp)
	assert.NoError(t, err, "UCompositeIndexInfo should not error")

	_, err = e.UCompositeIndexInfo(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "UCompositeIndexInfo should not error")
}

func TestUFuturesNewOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UFuturesNewOrder(t.Context(),
		&UFuturesNewOrderRequest{
			Symbol:      currency.NewBTCUSDT(),
			Side:        "BUY",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Quantity:    1,
			Price:       1,
		},
	)
	assert.NoError(t, err, "UFuturesNewOrder should not error")
}

func TestUPlaceBatchOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	var data []PlaceBatchOrderData
	var tempData PlaceBatchOrderData
	tempData.Symbol = "BTCUSDT"
	tempData.Side = "BUY"
	tempData.OrderType = "LIMIT"
	tempData.Quantity = 4
	tempData.Price = 1
	tempData.TimeInForce = "GTC"
	data = append(data, tempData)
	_, err := e.UPlaceBatchOrders(t.Context(), data)
	assert.NoError(t, err, "UPlaceBatchOrders should not error")
}

func TestUGetOrderData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UGetOrderData(t.Context(), currency.NewBTCUSDT(), "123", "")
	assert.NoError(t, err, "UGetOrderData should not error")
}

func TestUCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UCancelOrder(t.Context(), currency.NewBTCUSDT(), "123", "")
	assert.NoError(t, err, "UCancelOrder should not error")
}

func TestUCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UCancelAllOpenOrders(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UCancelAllOpenOrders should not error")
}

func TestUCancelBatchOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UCancelBatchOrders(t.Context(), currency.NewBTCUSDT(), []string{"123"}, []string{})
	assert.NoError(t, err, "UCancelBatchOrders should not error")
}

func TestUAutoCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UAutoCancelAllOpenOrders(t.Context(), currency.NewBTCUSDT(), 30)
	assert.NoError(t, err, "UAutoCancelAllOpenOrders should not error")
}

func TestUFetchOpenOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UFetchOpenOrder(t.Context(), currency.NewBTCUSDT(), "123", "")
	assert.NoError(t, err, "UFetchOpenOrder should not error")
}

func TestUAllAccountOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UAllAccountOpenOrders(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UAllAccountOpenOrders should not error")
}

func TestUAllAccountOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UAllAccountOrders(t.Context(), currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "UAllAccountOrders should not error")
	_, err = e.UAllAccountOrders(t.Context(), currency.NewBTCUSDT(), 0, 5, time.Now().Add(-time.Hour*4), time.Now())
	assert.NoError(t, err, "UAllAccountOrders should not error")
}

func TestUAccountBalanceV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UAccountBalanceV2(t.Context())
	assert.NoError(t, err, "UAccountBalanceV2 should not error")
}

func TestUAccountInformationV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UAccountInformationV2(t.Context())
	assert.NoError(t, err, "UAccountInformationV2 should not error")
}

func TestUChangeInitialLeverageRequest(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UChangeInitialLeverageRequest(t.Context(), currency.NewBTCUSDT(), 2)
	assert.NoError(t, err, "UChangeInitialLeverageRequest should not error")
}

func TestUChangeInitialMarginType(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.UChangeInitialMarginType(t.Context(), currency.NewBTCUSDT(), "ISOLATED")
	assert.NoError(t, err, "UChangeInitialMarginType should not error")
}

func TestUModifyIsolatedPositionMarginReq(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UModifyIsolatedPositionMarginReq(t.Context(), currency.NewBTCUSDT(), "LONG", "add", 5)
	assert.NoError(t, err, "UModifyIsolatedPositionMarginReq should not error")
}

func TestUPositionMarginChangeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UPositionMarginChangeHistory(t.Context(), currency.NewBTCUSDT(), "add", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UPositionMarginChangeHistory should not error")
}

func TestUPositionsInfoV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UPositionsInfoV2(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UPositionsInfoV2 should not error")
}

func TestUAccountTradesHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UAccountTradesHistory(t.Context(), currency.NewBTCUSDT(), "", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UAccountTradesHistory should not error")
}

func TestUAccountIncomeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UAccountIncomeHistory(t.Context(), currency.EMPTYPAIR, "", 5, time.Now().Add(-time.Hour*48), time.Now())
	assert.NoError(t, err, "UAccountIncomeHistory should not error")
}

func TestUGetNotionalAndLeverageBrackets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UGetNotionalAndLeverageBrackets(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UGetNotionalAndLeverageBrackets should not error")
}

func TestUPositionsADLEstimate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UPositionsADLEstimate(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UPositionsADLEstimate should not error")
}

func TestUAccountForcedOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UAccountForcedOrders(t.Context(), currency.NewBTCUSDT(), "ADL", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UAccountForcedOrders should not error")
}

// Coin Margined Futures

func TestGetFuturesExchangeInfo(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesExchangeInfo(t.Context())
	assert.NoError(t, err, "FuturesExchangeInfo should not error")
}

func TestGetFuturesOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesOrderbook(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 1000)
	assert.NoError(t, err, "GetFuturesOrderbook should not error")
}

func TestGetFuturesPublicTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesPublicTrades(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5)
	assert.NoError(t, err, "GetFuturesPublicTrades should not error")
}

func TestGetPastPublicTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetPastPublicTrades(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5, 0)
	assert.NoError(t, err, "GetPastPublicTrades should not error")
}

func TestGetAggregatedTradesList(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesAggregatedTradesList(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 0, 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetFuturesAggregatedTradesList should not error")
}

func TestGetPerpsExchangeInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetPerpMarkets(t.Context())
	assert.NoError(t, err, "GetPerpMarkets should not error")
}

func TestGetIndexAndMarkPrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexAndMarkPrice(t.Context(), "", "BTCUSD")
	assert.NoError(t, err, "GetIndexAndMarkPrice should not error")
}

func TestGetFuturesKlineData(t *testing.T) {
	t.Parallel()
	r, err := e.GetFuturesKlineData(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "1M", 5, time.Time{}, time.Time{})
	require.NoError(t, err, "GetFuturesKlineData must not error")
	if mockTests {
		require.Len(t, r, 5, "GetFuturesKlineData must return 5 items in mock test")
		exp := FuturesCandleStick{
			OpenTime:                types.Time(time.UnixMilli(1596240000000)),
			Open:                    11785,
			High:                    12513.6,
			Low:                     11114.1,
			Close:                   11663.5,
			Volume:                  12155433,
			CloseTime:               types.Time(time.UnixMilli(1598918399999)),
			BaseAssetVolume:         104142.54608485,
			NumberOfTrades:          359100,
			TakerBuyVolume:          6013546,
			TakerBuyBaseAssetVolume: 51511.95826419,
		}
		assert.Equal(t, exp, r[0])
	} else {
		assert.NotEmpty(t, r, "GetFuturesKlineData should return data")
	}

	start, end := getTime()
	r, err = e.GetFuturesKlineData(t.Context(), currency.NewPairWithDelimiter("LTCUSD", "PERP", "_"), "5m", 5, start, end)
	require.NoError(t, err, "GetFuturesKlineData must not error")
	assert.NotEmpty(t, r, "GetFuturesKlineData should return data")
}

func TestGetContinuousKlineData(t *testing.T) {
	t.Parallel()
	_, err := e.GetContinuousKlineData(t.Context(), "BTCUSD", "CURRENT_QUARTER", "1M", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetContinuousKlineData should not error")
	start, end := getTime()
	_, err = e.GetContinuousKlineData(t.Context(), "BTCUSD", "CURRENT_QUARTER", "1M", 5, start, end)
	assert.NoError(t, err, "GetContinuousKlineData should not error")
}

func TestGetIndexPriceKlines(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexPriceKlines(t.Context(), "BTCUSD", "1M", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetIndexPriceKlines should not error")
	start, end := getTime()
	_, err = e.GetIndexPriceKlines(t.Context(), "BTCUSD", "1M", 5, start, end)
	assert.NoError(t, err, "GetIndexPriceKlines should not error")
}

func TestGetFuturesSwapTickerChangeStats(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesSwapTickerChangeStats(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesSwapTickerChangeStats should not error")
	_, err = e.GetFuturesSwapTickerChangeStats(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesSwapTickerChangeStats should not error")
	_, err = e.GetFuturesSwapTickerChangeStats(t.Context(), currency.EMPTYPAIR, "")
	assert.NoError(t, err, "GetFuturesSwapTickerChangeStats should not error")
}

func TestFuturesGetFundingHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesGetFundingHistory(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "FuturesGetFundingHistory should not error")
	start, end := getTime()
	_, err = e.FuturesGetFundingHistory(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 50, start, end)
	assert.NoError(t, err, "FuturesGetFundingHistory should not error")
}

func TestGetFuturesHistoricalTrades(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFuturesHistoricalTrades(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "", 5)
	assert.NoError(t, err, "GetFuturesHistoricalTrades should not error")
	_, err = e.GetFuturesHistoricalTrades(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "", 0)
	assert.NoError(t, err, "GetFuturesHistoricalTrades should not error")
}

func TestGetFuturesSymbolPriceTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesSymbolPriceTicker(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesSymbolPriceTicker should not error")
}

func TestGetFuturesOrderbookTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesOrderbookTicker(t.Context(), currency.EMPTYPAIR, "")
	assert.NoError(t, err, "GetFuturesOrderbookTicker should not error")
	_, err = e.GetFuturesOrderbookTicker(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesOrderbookTicker should not error")
}

func TestOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := e.OpenInterest(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"))
	assert.NoError(t, err, "OpenInterest should not error")
}

func TestGetOpenInterestStats(t *testing.T) {
	t.Parallel()
	_, err := e.GetOpenInterestStats(t.Context(), "BTCUSD", "CURRENT_QUARTER", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetOpenInterestStats should not error")
	start, end := getTime()
	_, err = e.GetOpenInterestStats(t.Context(), "BTCUSD", "CURRENT_QUARTER", "5m", 0, start, end)
	assert.NoError(t, err, "GetOpenInterestStats should not error")
}

func TestGetTraderFuturesAccountRatio(t *testing.T) {
	t.Parallel()
	_, err := e.GetTraderFuturesAccountRatio(t.Context(), "BTCUSD", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetTraderFuturesAccountRatio should not error")
	start, end := getTime()
	_, err = e.GetTraderFuturesAccountRatio(t.Context(), "BTCUSD", "5m", 0, start, end)
	assert.NoError(t, err, "GetTraderFuturesAccountRatio should not error")
}

func TestGetTraderFuturesPositionsRatio(t *testing.T) {
	t.Parallel()
	_, err := e.GetTraderFuturesPositionsRatio(t.Context(), "BTCUSD", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetTraderFuturesPositionsRatio should not error")
	start, end := getTime()
	_, err = e.GetTraderFuturesPositionsRatio(t.Context(), "BTCUSD", "5m", 0, start, end)
	assert.NoError(t, err, "GetTraderFuturesPositionsRatio should not error")
}

func TestGetMarketRatio(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarketRatio(t.Context(), "BTCUSD", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetMarketRatio should not error")
	start, end := getTime()
	_, err = e.GetMarketRatio(t.Context(), "BTCUSD", "5m", 0, start, end)
	assert.NoError(t, err, "GetMarketRatio should not error")
}

func TestGetFuturesTakerVolume(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesTakerVolume(t.Context(), "BTCUSD", "ALL", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetFuturesTakerVolume should not error")
	start, end := getTime()
	_, err = e.GetFuturesTakerVolume(t.Context(), "BTCUSD", "ALL", "5m", 0, start, end)
	assert.NoError(t, err, "GetFuturesTakerVolume should not error")
}

func TestFuturesBasisData(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesBasisData(t.Context(), "BTCUSD", "CURRENT_QUARTER", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetFuturesBasisData should not error")
	start, end := getTime()
	_, err = e.GetFuturesBasisData(t.Context(), "BTCUSD", "CURRENT_QUARTER", "5m", 0, start, end)
	assert.NoError(t, err, "GetFuturesBasisData should not error")
}

func TestFuturesNewOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.FuturesNewOrder(
		t.Context(),
		&FuturesNewOrderRequest{
			Symbol:      currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"),
			Side:        "BUY",
			OrderType:   "LIMIT",
			TimeInForce: order.GoodTillCancel.String(),
			Quantity:    1,
			Price:       1,
		},
	)
	assert.NoError(t, err, "FuturesNewOrder should not error")
}

func TestFuturesBatchOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	var data []PlaceBatchOrderData
	var tempData PlaceBatchOrderData
	tempData.Symbol = "BTCUSD_PERP"
	tempData.Side = "BUY"
	tempData.OrderType = "LIMIT"
	tempData.Quantity = 1
	tempData.Price = 1
	tempData.TimeInForce = "GTC"

	data = append(data, tempData)
	_, err := e.FuturesBatchOrder(t.Context(), data)
	assert.NoError(t, err, "FuturesBatchOrder should not error")
}

func TestFuturesBatchCancelOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.FuturesBatchCancelOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), []string{"123"}, []string{})
	assert.NoError(t, err, "FuturesBatchCancelOrders should not error")
}

func TestFuturesGetOrderData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesGetOrderData(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "123", "")
	assert.NoError(t, err, "FuturesGetOrderData should not error")
}

func TestCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.FuturesCancelAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"))
	assert.NoError(t, err, "FuturesCancelAllOpenOrders should not error")
}

func TestAutoCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.AutoCancelAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 30000)
	assert.NoError(t, err, "AutoCancelAllOpenOrders should not error")
}

func TestFuturesOpenOrderData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesOpenOrderData(t.Context(), currency.NewBTCUSDT(), "", "")
	assert.NoError(t, err, "FuturesOpenOrderData should not error")
}

func TestGetFuturesAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFuturesAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesAllOpenOrders should not error")
}

func TestGetAllFuturesOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAllFuturesOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), currency.EMPTYPAIR, time.Time{}, time.Time{}, 0, 2)
	assert.NoError(t, err, "GetAllFuturesOrders should not error")
}

func TestFuturesChangeMarginType(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.FuturesChangeMarginType(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "ISOLATED")
	assert.NoError(t, err, "FuturesChangeMarginType should not error")
}

func TestGetFuturesAccountBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFuturesAccountBalance(t.Context())
	assert.NoError(t, err, "GetFuturesAccountBalance should not error")
}

func TestGetFuturesAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFuturesAccountInfo(t.Context())
	assert.NoError(t, err, "GetFuturesAccountInfo should not error")
}

func TestFuturesChangeInitialLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.FuturesChangeInitialLeverage(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5)
	assert.NoError(t, err, "FuturesChangeInitialLeverage should not error")
}

func TestModifyIsolatedPositionMargin(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.ModifyIsolatedPositionMargin(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "BOTH", "add", 5)
	assert.NoError(t, err, "ModifyIsolatedPositionMargin should not error")
}

func TestFuturesMarginChangeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesMarginChangeHistory(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "add", time.Time{}, time.Time{}, 10)
	assert.NoError(t, err, "FuturesMarginChangeHistory should not error")
}

func TestFuturesPositionsInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesPositionsInfo(t.Context(), "BTCUSD", "")
	assert.NoError(t, err, "FuturesPositionsInfo should not error")
}

func TestFuturesTradeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesTradeHistory(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "", time.Time{}, time.Time{}, 5, 0)
	assert.NoError(t, err, "FuturesTradeHistory should not error")
}

func TestFuturesIncomeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesIncomeHistory(t.Context(), currency.EMPTYPAIR, "TRANSFER", time.Time{}, time.Time{}, 5)
	assert.NoError(t, err, "FuturesIncomeHistory should not error")
}

func TestFuturesForceOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesForceOrders(t.Context(), currency.EMPTYPAIR, "ADL", time.Time{}, time.Time{})
	assert.NoError(t, err, "FuturesForceOrders should not error")
}

func TestUGetNotionalLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesNotionalBracket(t.Context(), "BTCUSD")
	assert.NoError(t, err, "FuturesNotionalBracket should not error")
	_, err = e.FuturesNotionalBracket(t.Context(), "")
	assert.NoError(t, err, "FuturesNotionalBracket should not error")
}

func TestFuturesPositionsADLEstimate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.FuturesPositionsADLEstimate(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "FuturesPositionsADLEstimate should not error")
}

func TestGetMarkPriceKline(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarkPriceKline(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "1M", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetMarkPriceKline should not error")
}

func TestGetExchangeInfo(t *testing.T) {
	t.Parallel()
	info, err := e.GetExchangeInfo(t.Context())
	require.NoError(t, err, "GetExchangeInfo must not error")
	if mockTests {
		exp := time.Date(2024, 5, 10, 6, 8, 1, int(707*time.Millisecond), time.UTC)
		assert.Truef(t, info.ServerTime.Time().Equal(exp), "expected %v received %v", exp.UTC(), info.ServerTime.Time().UTC())
	} else {
		assert.WithinRange(t, info.ServerTime.Time(), time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), "ServerTime should be within a day of now")
	}
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTradablePairs(t.Context(), asset.Spot)
	assert.NoError(t, err, "FetchTradablePairs for spot should not error")

	_, err = e.FetchTradablePairs(t.Context(), asset.CoinMarginedFutures)
	assert.NoError(t, err, "FetchTradablePairs for coin margined futures should not error")

	_, err = e.FetchTradablePairs(t.Context(), asset.USDTMarginedFutures)
	assert.NoError(t, err, "FetchTradablePairs for USDT margined futures should not error")
}

func TestGetOrderBook(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderBook(t.Context(),
		OrderBookDataRequestParams{
			Symbol: currency.NewBTCUSDT(),
			Limit:  1000,
		})
	assert.NoError(t, err, "GetOrderBook should not error")
}

func TestGetMostRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetMostRecentTrades(t.Context(),
		RecentTradeRequestParams{
			Symbol: currency.NewBTCUSDT(),
			Limit:  15,
		})
	assert.NoError(t, err, "GetMostRecentTrades should not error")
}

func TestGetHistoricalTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricalTrades(t.Context(), "BTCUSDT", 5, -1)
	if !mockTests {
		assert.Error(t, err, "GetHistoricalTrades should error when not in mock mode")
	} else {
		assert.NoError(t, err, "GetHistoricalTrades should not error in mock mode")
	}
}

func TestGetAggregatedTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetAggregatedTrades(t.Context(),
		&AggregatedTradeRequestParams{
			Symbol: currency.NewBTCUSDT(),
			Limit:  5,
		})
	assert.NoError(t, err, "GetAggregatedTrades should not error")
}

func TestGetSpotKline(t *testing.T) {
	t.Parallel()
	start, end := getTime()
	r, err := e.GetSpotKline(t.Context(), &KlinesRequestParams{
		Symbol:    currency.NewBTCUSDT(),
		Interval:  kline.FiveMin.Short(),
		Limit:     24,
		StartTime: start,
		EndTime:   end,
	})
	require.NoError(t, err, "GetSpotKline must not error")
	if mockTests {
		require.Len(t, r, 24, "GetSpotKline must return 24 items in mock test")
		exp := CandleStick{
			OpenTime:                 types.Time(time.UnixMilli(1577836800000)),
			Open:                     7195.24,
			High:                     7196.25,
			Low:                      7178.64,
			Close:                    7179.78,
			Volume:                   95.509133,
			CloseTime:                types.Time(time.UnixMilli(1577837099999)),
			QuoteAssetVolume:         686317.13625177,
			TradeCount:               1127,
			TakerBuyAssetVolume:      32.773245,
			TakerBuyQuoteAssetVolume: 235537.29504531,
		}
		assert.Equal(t, exp, r[0])
	} else {
		assert.NotEmpty(t, r, "GetSpotKline should return data")
	}
}

func TestGetAveragePrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetAveragePrice(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "GetAveragePrice should not error")
}

func TestGetPriceChangeStats(t *testing.T) {
	t.Parallel()
	_, err := e.GetPriceChangeStats(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "GetPriceChangeStats should not error")
}

func TestGetTickers(t *testing.T) {
	t.Parallel()
	_, err := e.GetTickers(t.Context())
	require.NoError(t, err, "GetTickers should not error")

	resp, err := e.GetTickers(t.Context(),
		currency.NewBTCUSDT(),
		currency.NewPair(currency.ETH, currency.USDT))
	require.NoError(t, err, "GetTickers should not error")
	require.Len(t, resp, 2, "GetTickers should return 2 tickers")
}

func TestGetLatestSpotPrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetLatestSpotPrice(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "GetLatestSpotPrice should not error")
}

func TestGetBestPrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetBestPrice(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "GetBestPrice should not error")
}

func TestQueryOrder(t *testing.T) {
	t.Parallel()
	_, err := e.QueryOrder(t.Context(), currency.NewBTCUSDT(), "", 1337)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "QueryOrder should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "QueryOrder should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock QueryOrder should not error")
	}
}

func TestOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.OpenOrders(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "OpenOrders with empty pair should not error")

	p := currency.NewBTCUSDT()
	_, err = e.OpenOrders(t.Context(), p)
	assert.NoError(t, err, "OpenOrders with a pair should not error")
}

func TestAllOrders(t *testing.T) {
	t.Parallel()
	_, err := e.AllOrders(t.Context(), currency.NewBTCUSDT(), "", "")
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "AllOrders should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "AllOrders should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock AllOrders should not error")
	}
}

// TestGetFeeByTypeOfflineTradeFee logic test
func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()
	_, err := e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFeeByType must not error")

	if !sharedtestvalues.AreAPICredentialsSet(e) || mockTests {
		assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "FeeType should be OfflineTradeFee")
	} else {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "FeeType should be CryptocurrencyTradeFee")
	}
}

func TestGetFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()

	if sharedtestvalues.AreAPICredentialsSet(e) && mockTests {
		// CryptocurrencyTradeFee Basic
		_, err := e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee for CryptocurrencyTradeFee should not error")

		// CryptocurrencyTradeFee High quantity
		feeBuilder = setFeeBuilder()
		feeBuilder.Amount = 1000
		feeBuilder.PurchasePrice = 1000
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee for high quantity CryptocurrencyTradeFee should not error")

		// CryptocurrencyTradeFee IsMaker
		feeBuilder = setFeeBuilder()
		feeBuilder.IsMaker = true
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee for maker CryptocurrencyTradeFee should not error")

		// CryptocurrencyTradeFee Negative purchase price
		feeBuilder = setFeeBuilder()
		feeBuilder.PurchasePrice = -1000
		_, err = e.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee for negative purchase price CryptocurrencyTradeFee should not error")
	}

	// CryptocurrencyWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err := e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee for CryptocurrencyWithdrawalFee should not error")

	// CryptocurrencyDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyDepositFee
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee for CryptocurrencyDepositFee should not error")

	// InternationalBankDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	feeBuilder.FiatCurrency = currency.HKD
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee for InternationalBankDepositFee should not error")

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.HKD
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee for InternationalBankWithdrawalFee should not error")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	expectedResult := exchange.AutoWithdrawCryptoText + " & " + exchange.NoFiatWithdrawalsText
	withdrawPermissions := e.FormatWithdrawPermissions()
	assert.Equal(t, expectedResult, withdrawPermissions, "FormatWithdrawPermissions should return correct string")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	pair, err := currency.NewPairFromString("BTC_USDT")
	require.NoError(t, err, "NewPairFromString should not error")

	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		Pairs:     currency.Pairs{pair},
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	_, err = e.GetActiveOrders(t.Context(), &getOrdersRequest)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "GetActiveOrders should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "GetActiveOrders should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock GetActiveOrders should not error")
	}
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	_, err := e.GetOrderHistory(t.Context(), &getOrdersRequest)
	assert.Error(t, err, "GetOrderHistory should error when no currency is provided")

	getOrdersRequest.Pairs = []currency.Pair{
		currency.NewPair(currency.LTC,
			currency.BTC),
	}

	_, err = e.GetOrderHistory(t.Context(), &getOrdersRequest)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "GetOrderHistory should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "GetOrderHistory should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock GetOrderHistory should not error")
	}
}

func TestNewOrderTest(t *testing.T) {
	t.Parallel()
	req := &NewOrderRequest{
		Symbol:      currency.NewPair(currency.LTC, currency.BTC),
		Side:        order.Buy.String(),
		TradeType:   BinanceRequestParamsOrderLimit,
		Price:       0.0025,
		Quantity:    100000,
		TimeInForce: order.GoodTillCancel.String(),
	}

	err := e.NewOrderTest(t.Context(), req)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "NewOrderTest should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "NewOrderTest should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock NewOrderTest should not error")
	}

	req = &NewOrderRequest{
		Symbol:        currency.NewPair(currency.LTC, currency.BTC),
		Side:          order.Sell.String(),
		TradeType:     BinanceRequestParamsOrderMarket,
		Price:         0.0045,
		QuoteOrderQty: 10,
	}

	err = e.NewOrderTest(t.Context(), req)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "NewOrderTest should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "NewOrderTest should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock NewOrderTest should not error")
	}
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	p := currency.NewBTCUSDT()
	start := time.Unix(1577977445, 0)  // 2020-01-02 15:04:05
	end := start.Add(15 * time.Minute) // 2020-01-02 15:19:05
	result, err := e.GetHistoricTrades(t.Context(), p, asset.Spot, start, end)
	assert.NoError(t, err, "GetHistoricTrades should not error")
	expected := 2134
	if mockTests {
		expected = 1002
	}
	assert.Len(t, result, expected, "GetHistoricTrades should return correct number of entries")
	for _, r := range result {
		if !assert.WithinRange(t, r.Timestamp, start, end, "All trades should be within time range") {
			break
		}
	}
}

func TestGetAggregatedTradesBatched(t *testing.T) {
	t.Parallel()
	currencyPair, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString must not error")

	start := time.Date(2020, 1, 2, 15, 4, 5, 0, time.UTC)
	expectTime := time.Date(2020, 1, 2, 16, 19, 4, 831_000_000, time.UTC)
	tests := []struct {
		name string
		// mock test or live test
		mock         bool
		args         *AggregatedTradeRequestParams
		numExpected  int
		lastExpected time.Time
	}{
		{
			name: "mock batch with timerange",
			mock: true,
			args: &AggregatedTradeRequestParams{
				Symbol:    currencyPair,
				StartTime: start,
				EndTime:   start.Add(75 * time.Minute),
			},
			numExpected:  1012,
			lastExpected: time.Date(2020, 1, 2, 16, 18, 31, int(919*time.Millisecond), time.UTC),
		},
		{
			name: "batch with timerange",
			args: &AggregatedTradeRequestParams{
				Symbol:    currencyPair,
				StartTime: start,
				EndTime:   start.Add(75 * time.Minute),
			},
			numExpected:  12130,
			lastExpected: expectTime,
		},
		{
			name: "mock custom limit with start time set, no end time",
			mock: true,
			args: &AggregatedTradeRequestParams{
				Symbol:    currency.NewBTCUSDT(),
				StartTime: start,
				Limit:     1001,
			},
			numExpected:  1001,
			lastExpected: time.Date(2020, 1, 2, 15, 18, 39, int(226*time.Millisecond), time.UTC),
		},
		{
			name: "custom limit with start time set, no end time",
			args: &AggregatedTradeRequestParams{
				Symbol:    currency.NewBTCUSDT(),
				StartTime: time.Date(2020, 11, 18, 23, 0, 28, 921, time.UTC),
				Limit:     1001,
			},
			numExpected:  1001,
			lastExpected: time.Date(2020, 11, 18, 23, 1, 33, int(62*time.Millisecond*10), time.UTC),
		},
		{
			name: "mock recent trades",
			mock: true,
			args: &AggregatedTradeRequestParams{
				Symbol: currency.NewBTCUSDT(),
				Limit:  3,
			},
			numExpected:  3,
			lastExpected: time.Date(2020, 1, 2, 16, 19, 5, int(200*time.Millisecond), time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.mock != mockTests {
				t.Skip("mock mismatch, skipping")
			}
			result, err := e.GetAggregatedTrades(t.Context(), tt.args)
			require.NoError(t, err, "GetAggregatedTrades must not error")
			assert.Len(t, result, tt.numExpected, "GetAggregatedTradesBatched should return correct number of entries")
			lastTradeTime := result[len(result)-1].TimeStamp
			assert.True(t, lastTradeTime.Time().Equal(tt.lastExpected), "last trade time should match expected")
		})
	}
}

func TestGetAggregatedTradesErrors(t *testing.T) {
	t.Parallel()
	start := time.Date(2020, 1, 2, 15, 4, 5, 0, time.UTC)
	tests := []struct {
		name string
		args *AggregatedTradeRequestParams
	}{
		{
			name: "get recent trades does not support custom limit",
			args: &AggregatedTradeRequestParams{
				Symbol: currency.NewBTCUSDT(),
				Limit:  1001,
			},
		},
		{
			name: "start time and fromId cannot be both set",
			args: &AggregatedTradeRequestParams{
				Symbol:    currency.NewBTCUSDT(),
				StartTime: start,
				EndTime:   start.Add(75 * time.Minute),
				FromID:    2,
			},
		},
		{
			name: "can't get most recent 5000 (more than 1000 not allowed)",
			args: &AggregatedTradeRequestParams{
				Symbol: currency.NewBTCUSDT(),
				Limit:  5000,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := e.GetAggregatedTrades(t.Context(), tt.args)
			assert.Error(t, err, "GetAggregatedTrades should error")
		})
	}
}

// Any tests below this line have the ability to impact your orders on the exchange. Enable canManipulateRealOrders to run them
// -----------------------------------------------------------------------------------------------------------------------------

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	orderSubmission := &order.Submit{
		Exchange: e.Name,
		Pair: currency.Pair{
			Delimiter: "_",
			Base:      currency.LTC,
			Quote:     currency.BTC,
		},
		Side:      order.Buy,
		Type:      order.Limit,
		Price:     1,
		Amount:    1000000000,
		ClientID:  "meowOrder",
		AssetType: asset.Spot,
	}

	_, err := e.SubmitOrder(t.Context(), orderSubmission)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "SubmitOrder should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "SubmitOrder should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock SubmitOrder should not error")
	}
}

func TestCancelExchangeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currency.NewPair(currency.LTC, currency.BTC),
		AssetType: asset.Spot,
	}

	err := e.CancelOrder(t.Context(), orderCancellation)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelOrder should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "CancelOrder should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock CancelOrder should not error")
	}
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currency.NewPair(currency.LTC, currency.BTC),
		AssetType: asset.Spot,
	}

	_, err := e.CancelAllOrders(t.Context(), orderCancellation)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelAllOrders should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "CancelAllOrders should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock CancelAllOrders should not error")
	}
}

func TestGetAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	items := asset.Items{
		asset.CoinMarginedFutures,
		asset.USDTMarginedFutures,
		asset.Spot,
		asset.Margin,
	}
	for i := range items {
		assetType := items[i]
		t.Run(fmt.Sprintf("Update info of account [%s]", assetType.String()), func(t *testing.T) {
			t.Parallel()
			_, err := e.UpdateAccountInfo(t.Context(), assetType)
			assert.NoError(t, err, "UpdateAccountInfo should not error")
		})
	}
}

func TestWrapperGetActiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	p, err := currency.NewPairFromString("EOS-USDT")
	require.NoError(t, err, "NewPairFromString should not error")

	_, err = e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		Pairs:     currency.Pairs{p},
		AssetType: asset.CoinMarginedFutures,
	})
	assert.NoError(t, err, "GetActiveOrders for coin margined futures should not error")

	p2, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString should not error")

	_, err = e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		Pairs:     currency.Pairs{p2},
		AssetType: asset.USDTMarginedFutures,
	})
	assert.NoError(t, err, "GetActiveOrders for USDT margined futures should not error")
}

func TestWrapperGetOrderHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	p, err := currency.NewPairFromString("EOSUSD_PERP")
	require.NoError(t, err, "NewPairFromString should not error")

	_, err = e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		Type:        order.AnyType,
		Side:        order.AnySide,
		FromOrderID: "123",
		Pairs:       currency.Pairs{p},
		AssetType:   asset.CoinMarginedFutures,
	})
	assert.NoError(t, err, "GetOrderHistory for coin margined futures should not error")

	p2, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString should not error")

	_, err = e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		Type:        order.AnyType,
		Side:        order.AnySide,
		FromOrderID: "123",
		Pairs:       currency.Pairs{p2},
		AssetType:   asset.USDTMarginedFutures,
	})
	assert.NoError(t, err, "GetOrderHistory for USDT margined futures should not error")

	_, err = e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		AssetType: asset.USDTMarginedFutures,
	})
	assert.Error(t, err, "GetOrderHistory should error for invalid param combination")
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	p, err := currency.NewPairFromString("EOS-USDT")
	require.NoError(t, err, "NewPairFromString should not error")

	fPair, err := e.FormatExchangeCurrency(p, asset.CoinMarginedFutures)
	require.NoError(t, err, "FormatExchangeCurrency should not error")

	err = e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.CoinMarginedFutures,
		Pair:      fPair,
		OrderID:   "1234",
	})
	assert.NoError(t, err, "CancelOrder for coin margined futures should not error")

	p2, err := currency.NewPairFromString("BTC-USDT")
	require.NoError(t, err, "NewPairFromString should not error")

	fpair2, err := e.FormatExchangeCurrency(p2, asset.USDTMarginedFutures)
	require.NoError(t, err, "FormatExchangeCurrency should not error")

	err = e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.USDTMarginedFutures,
		Pair:      fpair2,
		OrderID:   "1234",
	})
	assert.NoError(t, err, "CancelOrder for USDT margined futures should not error")
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	tradablePairs, err := e.FetchTradablePairs(t.Context(), asset.CoinMarginedFutures)
	require.NoError(t, err, "FetchTradablePairs must not error")
	require.NotEmpty(t, tradablePairs, "tradable pairs should not be empty")

	_, err = e.GetOrderInfo(t.Context(), "123", tradablePairs[0], asset.CoinMarginedFutures)
	assert.NoError(t, err, "GetOrderInfo should not error")
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	_, err := e.ModifyOrder(t.Context(),
		&order.Modify{AssetType: asset.Spot})
	assert.Error(t, err, "ModifyOrder should error")
}

func TestGetAllCoinsInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAllCoinsInfo(t.Context())
	assert.NoError(t, err, "GetAllCoinsInfo should not error")
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawCryptoRequest := withdraw.Request{
		Exchange:    e.Name,
		Amount:      -1,
		Currency:    currency.BTC,
		Description: "WITHDRAW IT ALL",
		Crypto: withdraw.CryptoRequest{
			Address: core.BitcoinDonationAddress,
		},
	}

	_, err := e.WithdrawCryptocurrencyFunds(t.Context(),
		&withdrawCryptoRequest)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "Withdraw should not error when credentials are set")
	} else {
		assert.Error(t, err, "Withdraw should error when no keys are set")
	}
}

func TestDepositHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)
	_, err := e.DepositHistory(t.Context(), currency.ETH, "", time.Time{}, time.Time{}, 0, 10000)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "DepositHistory should not error when credentials are set")
	} else {
		assert.Error(t, err, "DepositHistory should error when no keys are set")
	}
}

func TestWithdrawHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)
	_, err := e.GetWithdrawalsHistory(t.Context(), currency.ETH, asset.Spot)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "GetWithdrawalsHistory should not error when credentials are set")
	} else {
		assert.Error(t, err, "GetWithdrawalsHistory should error when no keys are set")
	}
}

func TestWithdrawFiat(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawFiatFunds(t.Context(),
		&withdraw.Request{})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "WithdrawFiatFunds should return ErrFunctionNotSupported")
}

func TestWithdrawInternationalBank(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawFiatFundsToInternationalBank(t.Context(),
		&withdraw.Request{})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "WithdrawFiatFundsToInternationalBank should return ErrFunctionNotSupported")
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	_, err := e.GetDepositAddress(t.Context(), currency.USDT, "", currency.BNB.String())
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "GetDepositAddress should not error when credentials are set")
	} else if !mockTests {
		assert.Error(t, err, "GetDepositAddress should error when no keys are set")
	} else {
		assert.NoError(t, err, "Mock GetDepositAddress should not error")
	}
}

func BenchmarkWsHandleData(bb *testing.B) {
	bb.ReportAllocs()
	ap, err := e.CurrencyPairs.GetPairs(asset.Spot, false)
	require.NoError(bb, err, "GetPairs must not error")
	err = e.CurrencyPairs.StorePairs(asset.Spot, ap, true)
	require.NoError(bb, err, "StorePairs must not error")

	data, err := os.ReadFile("testdata/wsHandleData.json")
	require.NoError(bb, err, "ReadFile must not error")
	lines := bytes.Split(data, []byte("\n"))
	require.Len(bb, lines, 8, "wsHandleData.json should contain 8 lines")
	go func() {
		for {
			<-e.Websocket.DataHandler
		}
	}()
	for bb.Loop() {
		for x := range lines {
			assert.NoError(bb, e.wsHandleData(lines[x]), "wsHandleData should not error")
		}
	}
}

func TestSubscribe(t *testing.T) {
	t.Parallel()
	e := new(Exchange) //nolint:govet // Intentional shadow
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	channels, err := e.generateSubscriptions() // Note: We grab this before it's overwritten by MockWsInstance below
	require.NoError(t, err, "generateSubscriptions must not error")
	if mockTests {
		exp := []string{"btcusdt@depth@100ms", "btcusdt@kline_1m", "btcusdt@ticker", "btcusdt@trade", "dogeusdt@depth@100ms", "dogeusdt@kline_1m", "dogeusdt@ticker", "dogeusdt@trade"}
		mock := func(tb testing.TB, msg []byte, w *gws.Conn) error {
			tb.Helper()
			var req WsPayload
			require.NoError(tb, json.Unmarshal(msg, &req), "Unmarshal must not error")
			require.ElementsMatch(tb, req.Params, exp, "Params must have correct channels")
			return w.WriteMessage(gws.TextMessage, fmt.Appendf(nil, `{"result":null,"id":%d}`, req.ID))
		}
		e = testexch.MockWsInstance[Exchange](t, mockws.CurryWsMockUpgrader(t, mock))
	} else {
		testexch.SetupWs(t, e)
	}
	err = e.Subscribe(channels)
	require.NoError(t, err, "Subscribe must not error")
	err = e.Unsubscribe(channels)
	require.NoError(t, err, "Unsubscribe must not error")
}

func TestSubscribeBadResp(t *testing.T) {
	t.Parallel()
	channels := subscription.List{
		{Channel: "moons@ticker"},
	}
	mock := func(tb testing.TB, msg []byte, w *gws.Conn) error {
		tb.Helper()
		var req WsPayload
		err := json.Unmarshal(msg, &req)
		require.NoError(tb, err, "Unmarshal must not error")
		return w.WriteMessage(gws.TextMessage, fmt.Appendf(nil, `{"result":{"error":"carrots"},"id":%d}`, req.ID))
	}
	b := testexch.MockWsInstance[Exchange](t, mockws.CurryWsMockUpgrader(t, mock))
	err := b.Subscribe(channels)
	assert.ErrorIs(t, err, common.ErrUnknownError, "Subscribe should error correctly")
	assert.ErrorContains(t, err, "carrots", "Subscribe should error containing the carrots")
}

func TestWsTickerUpdate(t *testing.T) {
	t.Parallel()
	pressXToJSON := []byte(`{"stream":"btcusdt@ticker","data":{"e":"24hrTicker","E":1580254809477,"s":"BTCUSDT","p":"420.97000000","P":"4.720","w":"9058.27981278","x":"8917.98000000","c":"9338.96000000","Q":"0.17246300","b":"9338.03000000","B":"0.18234600","a":"9339.70000000","A":"0.14097600","o":"8917.99000000","h":"9373.19000000","l":"8862.40000000","v":"72229.53692000","q":"654275356.16896672","O":1580168409456,"C":1580254809456,"F":235294268,"L":235894703,"n":600436}}`)
	err := e.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData should not error")
}

func TestWsKlineUpdate(t *testing.T) {
	t.Parallel()
	pressXToJSON := []byte(`{"stream":"btcusdt@kline_1m","data":{
	  "e": "kline",
	  "E": 1234567891,   
	  "s": "BTCUSDT",    
	  "k": {
		"t": 1234000001, 
		"T": 1234600001, 
		"s": "BTCUSDT",  
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
	}}`)
	err := e.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData should not error")
}

func TestWsTradeUpdate(t *testing.T) {
	t.Parallel()
	e.SetSaveTradeDataStatus(true)
	pressXToJSON := []byte(`{"stream":"btcusdt@trade","data":{
	  "e": "trade",     
	  "E": 1234567891,   
	  "s": "BTCUSDT",    
	  "t": 12345,       
	  "p": "0.001",     
	  "q": "100",       
	  "b": 88,          
	  "a": 50,          
	  "T": 1234567851,   
	  "m": true,        
	  "M": true         
	}}`)
	err := e.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData should not error")
}

func TestWsDepthUpdate(t *testing.T) {
	t.Parallel()
	e := new(Exchange) //nolint:govet // Intentional shadow
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	e.setupOrderbookManager(t.Context())
	seedLastUpdateID := int64(161)
	book := OrderBook{
		Asks: []OrderbookItem{
			{Price: 6621.80000000, Quantity: 0.00198100},
			{Price: 6622.14000000, Quantity: 4.00000000},
			{Price: 6622.46000000, Quantity: 2.30000000},
			{Price: 6622.47000000, Quantity: 1.18633300},
			{Price: 6622.64000000, Quantity: 4.00000000},
			{Price: 6622.73000000, Quantity: 0.02900000},
			{Price: 6622.76000000, Quantity: 0.12557700},
			{Price: 6622.81000000, Quantity: 2.08994200},
			{Price: 6622.82000000, Quantity: 0.01500000},
			{Price: 6623.17000000, Quantity: 0.16831300},
		},
		Bids: []OrderbookItem{
			{Price: 6621.55000000, Quantity: 0.16356700},
			{Price: 6621.45000000, Quantity: 0.16352600},
			{Price: 6621.41000000, Quantity: 0.86091200},
			{Price: 6621.25000000, Quantity: 0.16914100},
			{Price: 6621.23000000, Quantity: 0.09193600},
			{Price: 6621.22000000, Quantity: 0.00755100},
			{Price: 6621.13000000, Quantity: 0.08432000},
			{Price: 6621.03000000, Quantity: 0.00172000},
			{Price: 6620.94000000, Quantity: 0.30506700},
			{Price: 6620.93000000, Quantity: 0.00200000},
		},
		LastUpdateID: seedLastUpdateID,
	}

	update1 := []byte(`{"stream":"btcusdt@depth","data":{
	  "e": "depthUpdate", 
	  "E": 1234567881,     
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
	require.NoError(t, e.SeedLocalCacheWithBook(p, &book), "SeedLocalCacheWithBook must not error")
	require.NoError(t, e.wsHandleData(update1), "wsHandleData must not error")

	e.obm.state[currency.BTC][currency.USDT][asset.Spot].fetchingBook = false

	ob, err := e.Websocket.Orderbook.GetOrderbook(p, asset.Spot)
	require.NoError(t, err, "GetOrderbook must not error")

	assert.Equal(t, seedLastUpdateID, ob.LastUpdateID, "Unexpected Last update id of orderbook for old update")
	assert.Equal(t, 2.3, ob.Asks[2].Amount, "Ask altered by outdated update")
	assert.Equal(t, 0.163526, ob.Bids[1].Amount, "Bid altered by outdated update")

	update2 := []byte(`{"stream":"btcusdt@depth","data":{
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
	}}`)

	assert.NoError(t, e.wsHandleData(update2), "wsHandleData should not error")

	ob, err = e.Websocket.Orderbook.GetOrderbook(p, asset.Spot)
	require.NoError(t, err, "GetOrderbook must not error")

	assert.Equal(t, int64(165), ob.LastUpdateID, "Unexpected Last update id of orderbook for new update")
	assert.Equal(t, 2.3, ob.Asks[2].Amount, "Unexpected Ask amount")
	assert.Equal(t, 1.9, ob.Asks[3].Amount, "Unexpected Ask amount")
	assert.Equal(t, 0.163526, ob.Bids[1].Amount, "Unexpected Bid amount")

	// reset order book sync status
	e.obm.state[currency.BTC][currency.USDT][asset.Spot].lastUpdateID = 0
}

func TestWsBalanceUpdate(t *testing.T) {
	t.Parallel()
	pressXToJSON := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{
  "e": "balanceUpdate",         
  "E": 1573200697110,           
  "a": "BTC",                   
  "d": "100.00000000",          
  "T": 1573200697068}}`)
	err := e.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData should not error")
}

func TestWsOCO(t *testing.T) {
	t.Parallel()
	pressXToJSON := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{
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
}}`)
	err := e.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData should not error")
}

func TestGetWsAuthStreamKey(t *testing.T) {
	t.Parallel()
	key, err := e.GetWsAuthStreamKey(t.Context())
	if mockTests {
		require.NoError(t, err, "GetWsAuthStreamKey must not error in mock mode")
		assert.NotEmpty(t, key, "key should not be empty in mock mode")
	} else if sharedtestvalues.AreAPICredentialsSet(e) {
		require.NoError(t, err, "GetWsAuthStreamKey must not error when credentials are set")
		assert.NotEmpty(t, key, "key should not be empty when credentials are set")
	} else {
		assert.Error(t, err, "GetWsAuthStreamKey must error when no credentials are set")
		assert.Empty(t, key, "key should be empty when no credentials are set")
	}
}

func TestMaintainWsAuthStreamKey(t *testing.T) {
	t.Parallel()
	err := e.MaintainWsAuthStreamKey(t.Context())
	if mockTests {
		assert.NoError(t, err, "MaintainWsAuthStreamKey should not error in mock mode")
	} else if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "MaintainWsAuthStreamKey should not error when credentials are set")
	} else {
		assert.Error(t, err, "MaintainWsAuthStreamKey should error when no credentials are set")
	}
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
		if result != testCases[i].Result {
			t.Errorf("Expected: %v, received: %v", testCases[i].Result, result)
		}
	}
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	startTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := startTime.Add(time.Hour * 24 * 7)
	bAssets := e.GetAssetTypes(false)
	for i := range bAssets {
		cps, err := e.GetAvailablePairs(bAssets[i])
		require.NoErrorf(t, err, "GetAvailablePairs for asset %s must not error", bAssets[i])
		require.NotEmptyf(t, cps, "GetAvailablePairs for asset %s must return at least one pair", bAssets[i])
		err = e.CurrencyPairs.EnablePair(bAssets[i], cps[0])
		require.Truef(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled),
			"EnablePair for asset %s and pair %s must not error: %s", bAssets[i], cps[0], err)
		_, err = e.GetHistoricCandles(t.Context(), cps[0], bAssets[i], kline.OneDay, startTime, end)
		assert.NoErrorf(t, err, "GetHistoricCandles should not error for asset %s and pair %s", bAssets[i], cps[0])
	}

	startTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := e.GetHistoricCandles(t.Context(), currency.NewBTCUSDT(), asset.Spot, kline.Interval(time.Hour*7), startTime, end)
	require.ErrorIs(t, err, kline.ErrRequestExceedsExchangeLimits)
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	startTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := startTime.Add(time.Hour * 24 * 7)
	bAssets := e.GetAssetTypes(false)
	for i := range bAssets {
		cps, err := e.GetAvailablePairs(bAssets[i])
		require.NoErrorf(t, err, "GetAvailablePairs for asset %s must not error", bAssets[i])
		require.NotEmptyf(t, cps, "GetAvailablePairs for asset %s must return at least one pair", bAssets[i])
		err = e.CurrencyPairs.EnablePair(bAssets[i], cps[0])
		require.Truef(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled),
			"EnablePair for asset %s and pair %s must not error: %s", bAssets[i], cps[0], err)
		_, err = e.GetHistoricCandlesExtended(t.Context(), cps[0], bAssets[i], kline.OneDay, startTime, end)
		assert.NoErrorf(t, err, "GetHistoricCandlesExtended should not error for asset %s and pair %s", bAssets[i], cps[0])
	}
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
			"1d",
		},
		{
			kline.OneWeek,
			"1w",
		},
		{
			kline.OneMonth,
			"1M",
		},
	} {
		t.Run(tc.interval.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.output, e.FormatExchangeKlineInterval(tc.interval))
		})
	}
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	pair := currency.NewBTCUSDT()
	_, err := e.GetRecentTrades(t.Context(),
		pair, asset.Spot)
	if err != nil {
		t.Error(err)
	}
	_, err = e.GetRecentTrades(t.Context(),
		pair, asset.USDTMarginedFutures)
	if err != nil {
		t.Error(err)
	}
	pair.Base = currency.NewCode("BTCUSD")
	pair.Quote = currency.PERP
	_, err = e.GetRecentTrades(t.Context(),
		pair, asset.CoinMarginedFutures)
	if err != nil {
		t.Error(err)
	}
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	_, err := e.GetAvailableTransferChains(t.Context(), currency.BTC)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(e) && err != nil:
		t.Error(err)
	case !sharedtestvalues.AreAPICredentialsSet(e) && err == nil && !mockTests:
		t.Error("error cannot be nil")
	case mockTests && err != nil:
		t.Error(err)
	}
}

func TestSeedLocalCache(t *testing.T) {
	t.Parallel()
	err := e.SeedLocalCache(t.Context(), currency.NewBTCUSDT())
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateSubscriptions(t *testing.T) {
	t.Parallel()
	exp := subscription.List{}
	pairs, err := e.GetEnabledPairs(asset.Spot)
	assert.NoError(t, err, "GetEnabledPairs should not error")
	wsFmt := currency.PairFormat{Uppercase: false, Delimiter: ""}
	baseExp := subscription.List{
		{Channel: subscription.CandlesChannel, QualifiedChannel: "kline_1m", Asset: asset.Spot, Interval: kline.OneMin},
		{Channel: subscription.OrderbookChannel, QualifiedChannel: "depth@100ms", Asset: asset.Spot, Interval: kline.HundredMilliseconds},
		{Channel: subscription.TickerChannel, QualifiedChannel: "ticker", Asset: asset.Spot},
		{Channel: subscription.AllTradesChannel, QualifiedChannel: "trade", Asset: asset.Spot},
	}
	for _, p := range pairs {
		for _, baseSub := range baseExp {
			sub := baseSub.Clone()
			sub.Pairs = currency.Pairs{p}
			sub.QualifiedChannel = wsFmt.Format(p) + "@" + sub.QualifiedChannel
			exp = append(exp, sub)
		}
	}
	subs, err := e.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")
	testsubs.EqualLists(t, exp, subs)
}

// TestFormatChannelInterval exercises formatChannelInterval
func TestFormatChannelInterval(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "@1000ms", formatChannelInterval(&subscription.Subscription{Channel: subscription.OrderbookChannel, Interval: kline.ThousandMilliseconds}), "1s should format correctly for Orderbook")
	assert.Equal(t, "@1m", formatChannelInterval(&subscription.Subscription{Channel: subscription.OrderbookChannel, Interval: kline.OneMin}), "Orderbook should format correctly")
	assert.Equal(t, "_15m", formatChannelInterval(&subscription.Subscription{Channel: subscription.CandlesChannel, Interval: kline.FifteenMin}), "Candles should format correctly")
}

// TestFormatChannelLevels exercises formatChannelLevels
func TestFormatChannelLevels(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "10", formatChannelLevels(&subscription.Subscription{Channel: subscription.OrderbookChannel, Levels: 10}), "Levels should format correctly")
	assert.Empty(t, formatChannelLevels(&subscription.Subscription{Channel: subscription.OrderbookChannel, Levels: 0}), "Levels should format correctly")
}

var websocketDepthUpdate = []byte(`{"E":1608001030784,"U":7145637266,"a":[["19455.19000000","0.59490200"],["19455.37000000","0.00000000"],["19456.11000000","0.00000000"],["19456.16000000","0.00000000"],["19458.67000000","0.06400000"],["19460.73000000","0.05139800"],["19461.43000000","0.00000000"],["19464.59000000","0.00000000"],["19466.03000000","0.45000000"],["19466.36000000","0.00000000"],["19508.67000000","0.00000000"],["19572.96000000","0.00217200"],["24386.00000000","0.00256600"]],"b":[["19455.18000000","2.94649200"],["19453.15000000","0.01233600"],["19451.18000000","0.00000000"],["19446.85000000","0.11427900"],["19446.74000000","0.00000000"],["19446.73000000","0.00000000"],["19444.45000000","0.14937800"],["19426.75000000","0.00000000"],["19416.36000000","0.36052100"]],"e":"depthUpdate","s":"BTCUSDT","u":7145637297}`)

func TestProcessOrderbookUpdate(t *testing.T) {
	t.Parallel()
	e := new(Exchange) //nolint:govet // Intentional shadow
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	e.setupOrderbookManager(t.Context())
	p := currency.NewBTCUSDT()
	var depth WebsocketDepthStream
	err := json.Unmarshal(websocketDepthUpdate, &depth)
	if err != nil {
		t.Fatal(err)
	}

	err = e.obm.stageWsUpdate(&depth, p, asset.Spot)
	if err != nil {
		t.Fatal(err)
	}

	err = e.obm.fetchBookViaREST(p)
	if err != nil {
		t.Fatal(err)
	}

	err = e.obm.cleanup(p)
	if err != nil {
		t.Fatal(err)
	}

	// reset order book sync status
	e.obm.state[currency.BTC][currency.USDT][asset.Spot].lastUpdateID = 0
}

func TestUFuturesHistoricalTrades(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	cp, err := currency.NewPairFromString("BTCUSDT")
	if err != nil {
		t.Error(err)
	}
	_, err = e.UFuturesHistoricalTrades(t.Context(), cp, "", 5)
	if err != nil {
		t.Error(err)
	}
	_, err = e.UFuturesHistoricalTrades(t.Context(), cp, "", 0)
	if err != nil {
		t.Error(err)
	}
}

func TestSetExchangeOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	err := e.UpdateOrderExecutionLimits(t.Context(), asset.Spot)
	if err != nil {
		t.Fatal(err)
	}
	err = e.UpdateOrderExecutionLimits(t.Context(), asset.CoinMarginedFutures)
	if err != nil {
		t.Fatal(err)
	}

	err = e.UpdateOrderExecutionLimits(t.Context(), asset.USDTMarginedFutures)
	if err != nil {
		t.Fatal(err)
	}

	err = e.UpdateOrderExecutionLimits(t.Context(), asset.Binary)
	if err == nil {
		t.Fatal("expected unhandled case")
	}

	cmfCP, err := currency.NewPairFromStrings("BTCUSD", "PERP")
	if err != nil {
		t.Fatal(err)
	}

	limit, err := e.GetOrderExecutionLimits(asset.CoinMarginedFutures, cmfCP)
	if err != nil {
		t.Fatal(err)
	}

	if limit == (order.MinMaxLevel{}) {
		t.Fatal("exchange limit should be loaded")
	}

	err = limit.Conforms(0.000001, 0.1, order.Limit)
	require.ErrorIs(t, err, order.ErrAmountBelowMin)

	err = limit.Conforms(0.01, 1, order.Limit)
	require.ErrorIs(t, err, order.ErrPriceBelowMin)
}

func TestWsOrderExecutionReport(t *testing.T) {
	t.Parallel()
	e := new(Exchange) //nolint:govet // Intentional shadow
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	payload := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"executionReport","E":1616627567900,"s":"BTCUSDT","c":"c4wyKsIhoAaittTYlIVLqk","S":"BUY","o":"LIMIT","f":"GTC","q":"0.00028400","p":"52789.10000000","P":"0.00000000","F":"0.00000000","g":-1,"C":"","x":"NEW","X":"NEW","r":"NONE","i":5340845958,"l":"0.00000000","z":"0.00000000","L":"0.00000000","n":"0","N":"BTC","T":1616627567900,"t":-1,"I":11388173160,"w":true,"m":false,"M":false,"O":1616627567900,"Z":"0.00000000","Y":"0.00000000","Q":"0.00000000","W":1616627567900}}`)
	// this is a buy BTC order, normally commission is charged in BTC, vice versa.
	expectedResult := order.Detail{
		Price:                52789.1,
		Amount:               0.00028400,
		AverageExecutedPrice: 0,
		QuoteAmount:          0,
		ExecutedAmount:       0,
		RemainingAmount:      0.00028400,
		Cost:                 0,
		CostAsset:            currency.USDT,
		Fee:                  0,
		FeeAsset:             currency.BTC,
		Exchange:             "Binance",
		OrderID:              "5340845958",
		ClientOrderID:        "c4wyKsIhoAaittTYlIVLqk",
		Type:                 order.Limit,
		Side:                 order.Buy,
		Status:               order.New,
		AssetType:            asset.Spot,
		Date:                 time.UnixMilli(1616627567900),
		LastUpdated:          time.UnixMilli(1616627567900),
		Pair:                 currency.NewBTCUSDT(),
	}
	// empty the channel. otherwise mock_test will fail
	for len(e.Websocket.DataHandler) > 0 {
		<-e.Websocket.DataHandler
	}

	err := e.wsHandleData(payload)
	if err != nil {
		t.Fatal(err)
	}
	res := <-e.Websocket.DataHandler
	switch r := res.(type) {
	case *order.Detail:
		if !reflect.DeepEqual(expectedResult, *r) {
			t.Errorf("Results do not match:\nexpected: %v\nreceived: %v", expectedResult, *r)
		}
	default:
		t.Fatalf("expected type order.Detail, found %T", res)
	}

	payload = []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"executionReport","E":1616633041556,"s":"BTCUSDT","c":"YeULctvPAnHj5HXCQo9Mob","S":"BUY","o":"LIMIT","f":"GTC","q":"0.00028600","p":"52436.85000000","P":"0.00000000","F":"0.00000000","g":-1,"C":"","x":"TRADE","X":"FILLED","r":"NONE","i":5341783271,"l":"0.00028600","z":"0.00028600","L":"52436.85000000","n":"0.00000029","N":"BTC","T":1616633041555,"t":726946523,"I":11390206312,"w":false,"m":false,"M":true,"O":1616633041555,"Z":"14.99693910","Y":"14.99693910","Q":"0.00000000","W":1616633041555}}`)
	err = e.wsHandleData(payload)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWsOutboundAccountPosition(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"outboundAccountPosition","E":1616628815745,"u":1616628815745,"B":[{"a":"BTC","f":"0.00225109","l":"0.00123000"},{"a":"BNB","f":"0.00000000","l":"0.00000000"},{"a":"USDT","f":"54.43390661","l":"0.00000000"}]}}`)
	if err := e.wsHandleData(payload); err != nil {
		t.Fatal(err)
	}
}

func TestFormatExchangeCurrency(t *testing.T) {
	t.Parallel()
	type testos struct {
		name              string
		pair              currency.Pair
		asset             asset.Item
		expectedDelimiter string
	}
	testerinos := []testos{
		{
			name:              "spot-btcusdt",
			pair:              currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter),
			asset:             asset.Spot,
			expectedDelimiter: "",
		},
		{
			name:              "coinmarginedfutures-btcusd_perp",
			pair:              currency.NewPairWithDelimiter("BTCUSD", "PERP", currency.DashDelimiter),
			asset:             asset.CoinMarginedFutures,
			expectedDelimiter: currency.UnderscoreDelimiter,
		},
		{
			name:              "coinmarginedfutures-btcusd_211231",
			pair:              currency.NewPairWithDelimiter("BTCUSD", "211231", currency.DashDelimiter),
			asset:             asset.CoinMarginedFutures,
			expectedDelimiter: currency.UnderscoreDelimiter,
		},
		{
			name:              "margin-ltousdt",
			pair:              currency.NewPairWithDelimiter("LTO", "USDT", currency.UnderscoreDelimiter),
			asset:             asset.Margin,
			expectedDelimiter: "",
		},
		{
			name:              "usdtmarginedfutures-btcusdt",
			pair:              currency.NewPairWithDelimiter("btc", "usdt", currency.DashDelimiter),
			asset:             asset.USDTMarginedFutures,
			expectedDelimiter: "",
		},
		{
			name:              "usdtmarginedfutures-btcusdt_211231",
			pair:              currency.NewPairWithDelimiter("btcusdt", "211231", currency.UnderscoreDelimiter),
			asset:             asset.USDTMarginedFutures,
			expectedDelimiter: currency.UnderscoreDelimiter,
		},
	}
	for i := range testerinos {
		tt := testerinos[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := e.FormatExchangeCurrency(tt.pair, tt.asset)
			if err != nil {
				t.Error(err)
			}
			if result.Delimiter != tt.expectedDelimiter {
				t.Errorf("received '%v' expected '%v'", result.Delimiter, tt.expectedDelimiter)
			}
		})
	}
}

func TestFormatSymbol(t *testing.T) {
	t.Parallel()
	type testos struct {
		name           string
		pair           currency.Pair
		asset          asset.Item
		expectedString string
	}
	testerinos := []testos{
		{
			name:           "spot-BTCUSDT",
			pair:           currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter),
			asset:          asset.Spot,
			expectedString: "BTCUSDT",
		},
		{
			name:           "coinmarginedfutures-btcusdperp",
			pair:           currency.NewPairWithDelimiter("BTCUSD", "PERP", currency.DashDelimiter),
			asset:          asset.CoinMarginedFutures,
			expectedString: "BTCUSD_PERP",
		},
		{
			name:           "coinmarginedfutures-BTCUSD_211231",
			pair:           currency.NewPairWithDelimiter("BTCUSD", "211231", currency.DashDelimiter),
			asset:          asset.CoinMarginedFutures,
			expectedString: "BTCUSD_211231",
		},
		{
			name:           "margin-LTOUSDT",
			pair:           currency.NewPairWithDelimiter("LTO", "USDT", currency.UnderscoreDelimiter),
			asset:          asset.Margin,
			expectedString: "LTOUSDT",
		},
		{
			name:           "usdtmarginedfutures-BTCUSDT",
			pair:           currency.NewPairWithDelimiter("btc", "usdt", currency.DashDelimiter),
			asset:          asset.USDTMarginedFutures,
			expectedString: "BTCUSDT",
		},
		{
			name:           "usdtmarginedfutures-BTCUSDT_211231",
			pair:           currency.NewPairWithDelimiter("btcusdt", "211231", currency.UnderscoreDelimiter),
			asset:          asset.USDTMarginedFutures,
			expectedString: "BTCUSDT_211231",
		},
	}
	for _, tt := range testerinos {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := e.FormatSymbol(tt.pair, tt.asset)
			if err != nil {
				t.Error(err)
			}
			if result != tt.expectedString {
				t.Errorf("received '%v' expected '%v'", result, tt.expectedString)
			}
		})
	}
}

func TestFormatUSDTMarginedFuturesPair(t *testing.T) {
	t.Parallel()
	pairFormat := currency.PairFormat{Uppercase: true}
	resp := e.formatUSDTMarginedFuturesPair(currency.NewPair(currency.DOGE, currency.USDT), pairFormat)
	if resp.String() != "DOGEUSDT" {
		t.Errorf("received '%v' expected '%v'", resp.String(), "DOGEUSDT")
	}

	resp = e.formatUSDTMarginedFuturesPair(currency.NewPair(currency.DOGE, currency.NewCode("1234567890")), pairFormat)
	if resp.String() != "DOGE_1234567890" {
		t.Errorf("received '%v' expected '%v'", resp.String(), "DOGE_1234567890")
	}
}

func TestFetchExchangeLimits(t *testing.T) {
	t.Parallel()
	limits, err := e.FetchExchangeLimits(t.Context(), asset.Spot)
	assert.NoError(t, err, "FetchExchangeLimits should not error")
	assert.NotEmpty(t, limits, "Should get some limits back")

	limits, err = e.FetchExchangeLimits(t.Context(), asset.Margin)
	assert.NoError(t, err, "FetchExchangeLimits should not error")
	assert.NotEmpty(t, limits, "Should get some limits back")

	_, err = e.FetchExchangeLimits(t.Context(), asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "FetchExchangeLimits should error on other asset types")
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()

	tests := map[asset.Item]currency.Pair{
		asset.Spot:   currency.NewBTCUSDT(),
		asset.Margin: currency.NewPair(currency.ETH, currency.BTC),
	}
	for _, a := range []asset.Item{asset.CoinMarginedFutures, asset.USDTMarginedFutures} {
		pairs, err := e.FetchTradablePairs(t.Context(), a)
		require.NoErrorf(t, err, "FetchTradablePairs must not error for %s", a)
		require.NotEmptyf(t, pairs, "Must get some pairs for %s", a)
		tests[a] = pairs[0]
	}

	for _, a := range e.GetAssetTypes(false) {
		err := e.UpdateOrderExecutionLimits(t.Context(), a)
		require.NoError(t, err, "UpdateOrderExecutionLimits must not error")

		p := tests[a]
		limits, err := e.GetOrderExecutionLimits(a, p)
		require.NoErrorf(t, err, "GetOrderExecutionLimits must not error for %s pair %s", a, p)
		assert.Positivef(t, limits.MinPrice, "MinPrice should be positive for %s pair %s", a, p)
		assert.Positivef(t, limits.MaxPrice, "MaxPrice should be positive for %s pair %s", a, p)
		assert.Positivef(t, limits.PriceStepIncrementSize, "PriceStepIncrementSize should be positive for %s pair %s", a, p)
		assert.Positivef(t, limits.MinimumBaseAmount, "MinimumBaseAmount should be positive for %s pair %s", a, p)
		assert.Positivef(t, limits.MaximumBaseAmount, "MaximumBaseAmount should be positive for %s pair %s", a, p)
		assert.Positivef(t, limits.AmountStepIncrementSize, "AmountStepIncrementSize should be positive for %s pair %s", a, p)
		assert.Positivef(t, limits.MarketMaxQty, "MarketMaxQty should be positive for %s pair %s", a, p)
		assert.Positivef(t, limits.MaxTotalOrders, "MaxTotalOrders should be positive for %s pair %s", a, p)
		switch a {
		case asset.Spot, asset.Margin:
			assert.Positivef(t, limits.MaxIcebergParts, "MaxIcebergParts should be positive for %s pair %s", a, p)
		case asset.USDTMarginedFutures:
			assert.Positivef(t, limits.MinNotional, "MinNotional should be positive for %s pair %s", a, p)
			fallthrough
		case asset.CoinMarginedFutures:
			assert.Positivef(t, limits.MultiplierUp, "MultiplierUp should be positive for %s pair %s", a, p)
			assert.Positivef(t, limits.MultiplierDown, "MultiplierDown should be positive for %s pair %s", a, p)
			assert.Positivef(t, limits.MarketMinQty, "MarketMinQty should be positive for %s pair %s", a, p)
			assert.Positivef(t, limits.MarketStepIncrementSize, "MarketStepIncrementSize should be positive for %s pair %s", a, p)
			assert.Positivef(t, limits.MaxAlgoOrders, "MaxAlgoOrders should be positive for %s pair %s", a, p)
		}
	}
}

func TestGetHistoricalFundingRates(t *testing.T) {
	t.Parallel()
	start, end := getTime()
	_, err := e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 currency.NewBTCUSDT(),
		StartDate:            start,
		EndDate:              end,
		IncludePayments:      true,
		IncludePredictedRate: true,
	})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.USDTMarginedFutures,
		Pair:            currency.NewBTCUSDT(),
		StartDate:       start,
		EndDate:         end,
		PaymentCurrency: currency.DOGE,
	})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported)

	r := &fundingrate.HistoricalRatesRequest{
		Asset:     asset.USDTMarginedFutures,
		Pair:      currency.NewBTCUSDT(),
		StartDate: start,
		EndDate:   end,
	}
	if sharedtestvalues.AreAPICredentialsSet(e) {
		r.IncludePayments = true
	}
	_, err = e.GetHistoricalFundingRates(t.Context(), r)
	if err != nil {
		t.Error(err)
	}

	r.Asset = asset.CoinMarginedFutures
	r.Pair, err = currency.NewPairFromString("BTCUSD_PERP")
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.GetHistoricalFundingRates(t.Context(), r)
	if err != nil {
		t.Error(err)
	}
}

func TestGetLatestFundingRates(t *testing.T) {
	t.Parallel()
	cp := currency.NewBTCUSDT()
	_, err := e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 cp,
		IncludePredictedRate: true,
	})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported)

	err = e.CurrencyPairs.EnablePair(asset.USDTMarginedFutures, cp)
	require.Truef(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled),
		"EnablePair for asset %s and pair %s must not error: %s", asset.USDTMarginedFutures, cp, err)

	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  cp,
	})
	assert.NoError(t, err, "GetLatestFundingRates should not error for USDTMarginedFutures")
	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.CoinMarginedFutures,
	})
	assert.NoError(t, err, "GetLatestFundingRates should not error for CoinMarginedFutures")
}

func TestIsPerpetualFutureCurrency(t *testing.T) {
	t.Parallel()
	is, err := e.IsPerpetualFutureCurrency(asset.Binary, currency.NewBTCUSDT())
	if err != nil {
		t.Error(err)
	}
	if is {
		t.Error("expected false")
	}

	is, err = e.IsPerpetualFutureCurrency(asset.CoinMarginedFutures, currency.NewBTCUSDT())
	if err != nil {
		t.Error(err)
	}
	if is {
		t.Error("expected false")
	}
	is, err = e.IsPerpetualFutureCurrency(asset.CoinMarginedFutures, currency.NewPair(currency.BTC, currency.PERP))
	if err != nil {
		t.Error(err)
	}
	if !is {
		t.Error("expected true")
	}

	is, err = e.IsPerpetualFutureCurrency(asset.USDTMarginedFutures, currency.NewBTCUSDT())
	if err != nil {
		t.Error(err)
	}
	if !is {
		t.Error("expected true")
	}
	is, err = e.IsPerpetualFutureCurrency(asset.USDTMarginedFutures, currency.NewPair(currency.BTC, currency.PERP))
	if err != nil {
		t.Error(err)
	}
	if is {
		t.Error("expected false")
	}
}

func TestGetUserMarginInterestHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetUserMarginInterestHistory(t.Context(), currency.USDT, currency.NewBTCUSDT(), time.Now().Add(-time.Hour*24), time.Now(), 1, 10, false)
	if err != nil {
		t.Error(err)
	}
}

func TestSetAssetsMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	is, err := e.GetAssetsMode(t.Context())
	assert.NoError(t, err)

	err = e.SetAssetsMode(t.Context(), !is)
	assert.NoError(t, err)

	err = e.SetAssetsMode(t.Context(), is)
	assert.NoError(t, err)
}

func TestGetAssetsMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAssetsMode(t.Context())
	assert.NoError(t, err)
}

func TestGetCollateralMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.GetCollateralMode(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.GetCollateralMode(t.Context(), asset.CoinMarginedFutures)
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.GetCollateralMode(t.Context(), asset.USDTMarginedFutures)
	assert.NoError(t, err)
}

func TestSetCollateralMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.SetCollateralMode(t.Context(), asset.Spot, collateral.SingleMode)
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	err = e.SetCollateralMode(t.Context(), asset.CoinMarginedFutures, collateral.SingleMode)
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	err = e.SetCollateralMode(t.Context(), asset.USDTMarginedFutures, collateral.MultiMode)
	assert.NoError(t, err)

	err = e.SetCollateralMode(t.Context(), asset.USDTMarginedFutures, collateral.PortfolioMode)
	assert.ErrorIs(t, err, order.ErrCollateralInvalid)
}

func TestChangePositionMargin(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.ChangePositionMargin(t.Context(), &margin.PositionChangeRequest{
		Pair:                    currency.NewBTCUSDT(),
		Asset:                   asset.USDTMarginedFutures,
		MarginType:              margin.Isolated,
		OriginalAllocatedMargin: 1337,
		NewAllocatedMargin:      1333337,
	})
	if err != nil {
		t.Error(err)
	}
}

func TestGetPositionSummary(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	bb := currency.NewBTCUSDT()
	_, err := e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  bb,
	})
	if err != nil {
		t.Error(err)
	}

	bb.Quote = currency.BUSD
	_, err = e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  bb,
	})
	if err != nil {
		t.Error(err)
	}

	p, err := currency.NewPairFromString("BTCUSD_PERP")
	if err != nil {
		t.Fatal(err)
	}
	bb.Quote = currency.USD
	_, err = e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset:          asset.CoinMarginedFutures,
		Pair:           p,
		UnderlyingPair: bb,
	})
	if err != nil {
		t.Error(err)
	}

	_, err = e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset:          asset.Spot,
		Pair:           p,
		UnderlyingPair: bb,
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported)
}

func TestGetFuturesPositionOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFuturesPositionOrders(t.Context(), &futures.PositionsRequest{
		Asset:                     asset.USDTMarginedFutures,
		Pairs:                     []currency.Pair{currency.NewBTCUSDT()},
		StartDate:                 time.Now().Add(-time.Hour * 24 * 70),
		RespectOrderHistoryLimits: true,
	})
	if err != nil {
		t.Error(err)
	}

	p, err := currency.NewPairFromString("ADAUSD_PERP")
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.GetFuturesPositionOrders(t.Context(), &futures.PositionsRequest{
		Asset:                     asset.CoinMarginedFutures,
		Pairs:                     []currency.Pair{p},
		StartDate:                 time.Now().Add(time.Hour * 24 * -70),
		RespectOrderHistoryLimits: true,
	})
	if err != nil {
		t.Error(err)
	}
}

func TestSetMarginType(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	err := e.SetMarginType(t.Context(), asset.USDTMarginedFutures, currency.NewBTCUSDT(), margin.Isolated)
	assert.NoError(t, err)

	p, err := currency.NewPairFromString("BTCUSD_PERP")
	if err != nil {
		t.Fatal(err)
	}
	err = e.SetMarginType(t.Context(), asset.CoinMarginedFutures, p, margin.Isolated)
	assert.NoError(t, err)

	err = e.SetMarginType(t.Context(), asset.Spot, currency.NewBTCUSDT(), margin.Isolated)
	assert.ErrorIs(t, err, asset.ErrNotSupported)
}

func TestGetLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetLeverage(t.Context(), asset.USDTMarginedFutures, currency.NewBTCUSDT(), 0, order.UnknownSide)
	if err != nil {
		t.Error(err)
	}

	p, err := currency.NewPairFromString("BTCUSD_PERP")
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.GetLeverage(t.Context(), asset.CoinMarginedFutures, p, 0, order.UnknownSide)
	if err != nil {
		t.Error(err)
	}
	_, err = e.GetLeverage(t.Context(), asset.Spot, currency.NewBTCUSDT(), 0, order.UnknownSide)
	assert.ErrorIs(t, err, asset.ErrNotSupported)
}

func TestSetLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.SetLeverage(t.Context(), asset.USDTMarginedFutures, currency.NewBTCUSDT(), margin.Multi, 5, order.UnknownSide)
	if err != nil {
		t.Error(err)
	}

	p, err := currency.NewPairFromString("BTCUSD_PERP")
	if err != nil {
		t.Fatal(err)
	}
	err = e.SetLeverage(t.Context(), asset.CoinMarginedFutures, p, margin.Multi, 5, order.UnknownSide)
	if err != nil {
		t.Error(err)
	}
	err = e.SetLeverage(t.Context(), asset.Spot, p, margin.Multi, 5, order.UnknownSide)
	assert.ErrorIs(t, err, asset.ErrNotSupported)
}

func TestGetCryptoLoansIncomeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CryptoLoanIncomeHistory(t.Context(), currency.USDT, "", time.Time{}, time.Time{}, 100); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanBorrow(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanBorrow(t.Context(), currency.EMPTYCODE, 1000, currency.BTC, 1, 7)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet)
	_, err = e.CryptoLoanBorrow(t.Context(), currency.USDT, 1000, currency.EMPTYCODE, 1, 7)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet)
	_, err = e.CryptoLoanBorrow(t.Context(), currency.USDT, 0, currency.BTC, 1, 0)
	assert.ErrorIs(t, err, errLoanTermMustBeSet)
	_, err = e.CryptoLoanBorrow(t.Context(), currency.USDT, 0, currency.BTC, 0, 7)
	assert.ErrorIs(t, err, errEitherLoanOrCollateralAmountsMustBeSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CryptoLoanBorrow(t.Context(), currency.USDT, 1000, currency.BTC, 1, 7); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanBorrowHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CryptoLoanBorrowHistory(t.Context(), 0, currency.USDT, currency.BTC, time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanOngoingOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CryptoLoanOngoingOrders(t.Context(), 0, currency.USDT, currency.BTC, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanRepay(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanRepay(t.Context(), 0, 1000, 1, false)
	assert.ErrorIs(t, err, errOrderIDMustBeSet)
	_, err = e.CryptoLoanRepay(t.Context(), 42069, 0, 1, false)
	assert.ErrorIs(t, err, errAmountMustBeSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CryptoLoanRepay(t.Context(), 42069, 1000, 1, false); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanRepaymentHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CryptoLoanRepaymentHistory(t.Context(), 0, currency.USDT, currency.BTC, time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanAdjustLTV(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanAdjustLTV(t.Context(), 0, true, 1)
	assert.ErrorIs(t, err, errOrderIDMustBeSet)
	_, err = e.CryptoLoanAdjustLTV(t.Context(), 42069, true, 0)
	assert.ErrorIs(t, err, errAmountMustBeSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CryptoLoanAdjustLTV(t.Context(), 42069, true, 1); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanLTVAdjustmentHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CryptoLoanLTVAdjustmentHistory(t.Context(), 0, currency.USDT, currency.BTC, time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CryptoLoanAssetsData(t.Context(), currency.EMPTYCODE, 0); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanCollateralAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CryptoLoanCollateralAssetsData(t.Context(), currency.EMPTYCODE, 0); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanCheckCollateralRepayRate(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.EMPTYCODE, currency.BNB, 69)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet)
	_, err = e.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.EMPTYCODE, 69)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet)
	_, err = e.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.BNB, 0)
	assert.ErrorIs(t, err, errAmountMustBeSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.BNB, 69); err != nil {
		t.Error(err)
	}
}

func TestCryptoLoanCustomiseMarginCall(t *testing.T) {
	t.Parallel()
	if _, err := e.CryptoLoanCustomiseMarginCall(t.Context(), 0, currency.BTC, 0); err == nil {
		t.Error("expected an error")
	}

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CryptoLoanCustomiseMarginCall(t.Context(), 1337, currency.BTC, .70); err != nil {
		t.Error(err)
	}
}

func TestFlexibleLoanBorrow(t *testing.T) {
	t.Parallel()
	_, err := e.FlexibleLoanBorrow(t.Context(), currency.EMPTYCODE, currency.USDC, 1, 0)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet)
	_, err = e.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.EMPTYCODE, 1, 0)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet)
	_, err = e.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.USDC, 0, 0)
	assert.ErrorIs(t, err, errEitherLoanOrCollateralAmountsMustBeSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.USDC, 1, 0); err != nil {
		t.Error(err)
	}
}

func TestFlexibleLoanOngoingOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.FlexibleLoanOngoingOrders(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestFlexibleLoanBorrowHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.FlexibleLoanBorrowHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestFlexibleLoanRepay(t *testing.T) {
	t.Parallel()
	_, err := e.FlexibleLoanRepay(t.Context(), currency.EMPTYCODE, currency.BTC, 1, false, false)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet)
	_, err = e.FlexibleLoanRepay(t.Context(), currency.USDT, currency.EMPTYCODE, 1, false, false)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet)
	_, err = e.FlexibleLoanRepay(t.Context(), currency.USDT, currency.BTC, 0, false, false)
	assert.ErrorIs(t, err, errAmountMustBeSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.FlexibleLoanRepay(t.Context(), currency.ATOM, currency.USDC, 1, false, false); err != nil {
		t.Error(err)
	}
}

func TestFlexibleLoanRepayHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.FlexibleLoanRepayHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestFlexibleLoanAdjustLTV(t *testing.T) {
	t.Parallel()
	_, err := e.FlexibleLoanAdjustLTV(t.Context(), currency.EMPTYCODE, currency.BTC, 1, true)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet)
	_, err = e.FlexibleLoanAdjustLTV(t.Context(), currency.USDT, currency.EMPTYCODE, 1, true)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.FlexibleLoanAdjustLTV(t.Context(), currency.USDT, currency.BTC, 1, true); err != nil {
		t.Error(err)
	}
}

func TestFlexibleLoanLTVAdjustmentHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.FlexibleLoanLTVAdjustmentHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestFlexibleLoanAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.FlexibleLoanAssetsData(t.Context(), currency.EMPTYCODE); err != nil {
		t.Error(err)
	}
}

func TestFlexibleCollateralAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.FlexibleCollateralAssetsData(t.Context(), currency.EMPTYCODE); err != nil {
		t.Error(err)
	}
}

func TestGetFuturesContractDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesContractDetails(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, futures.ErrNotFuturesAsset)

	_, err = e.GetFuturesContractDetails(t.Context(), asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.GetFuturesContractDetails(t.Context(), asset.USDTMarginedFutures)
	assert.NoError(t, err)

	_, err = e.GetFuturesContractDetails(t.Context(), asset.CoinMarginedFutures)
	assert.NoError(t, err)
}

func TestGetFundingRateInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetFundingRateInfo(t.Context())
	assert.NoError(t, err)
}

func TestUGetFundingRateInfo(t *testing.T) {
	t.Parallel()
	_, err := e.UGetFundingRateInfo(t.Context())
	assert.NoError(t, err)
}

func TestGetOpenInterest(t *testing.T) {
	t.Parallel()
	resp, err := e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.BTC.Item,
		Quote: currency.USDT.Item,
		Asset: asset.USDTMarginedFutures,
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, resp)

	resp, err = e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.NewCode("BTCUSD").Item,
		Quote: currency.PERP.Item,
		Asset: asset.CoinMarginedFutures,
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, resp)

	_, err = e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.BTC.Item,
		Quote: currency.USDT.Item,
		Asset: asset.Spot,
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported)
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
