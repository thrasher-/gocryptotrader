package binance

import (
	"bytes"
	"errors"
	"fmt"
	"os"
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
	b = &Binance{}
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
	_, err := b.UServerTime(t.Context())
	assert.NoError(t, err, "UServerTime should not error")
}

func TestWrapperGetServerTime(t *testing.T) {
	t.Parallel()
	_, err := b.GetServerTime(t.Context(), asset.Empty)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetServerTime with empty asset must return ErrNotSupported")

	for _, a := range []asset.Item{asset.Spot, asset.USDTMarginedFutures, asset.CoinMarginedFutures} {
		t.Run(a.String(), func(t *testing.T) {
			t.Parallel()
			st, err := b.GetServerTime(t.Context(), a)
			require.NoErrorf(t, err, "GetServerTime for asset %s must not error", a)
			assert.NotZerof(t, st, "GetServerTime for asset %s should return a valid time", a)
		})
	}
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	r, err := b.UpdateTicker(t.Context(), testPairMapping, asset.Spot)
	require.NoError(t, err, "UpdateTicker for spot must not error")
	assert.Equal(t, currency.DOGE, r.Pair.Base, "UpdateTicker for spot should have DOGE as base currency")
	assert.Equal(t, currency.USDT, r.Pair.Quote, "UpdateTicker for spot should have USDT as quote currency")

	tradablePairs, err := b.FetchTradablePairs(t.Context(), asset.CoinMarginedFutures)
	require.NoError(t, err, "FetchTradablePairs for coin margined futures must not error")
	require.NotEmpty(t, tradablePairs, "FetchTradablePairs for coin margined futures must return tradable pairs")
	_, err = b.UpdateTicker(t.Context(), tradablePairs[0], asset.CoinMarginedFutures)
	assert.NoError(t, err, "UpdateTicker for coin margined futures should not error")

	usdtMarginedPairs, err := b.FetchTradablePairs(t.Context(), asset.USDTMarginedFutures)
	require.NoError(t, err, "FetchTradablePairs for USDT margined futures must not error")
	require.NotEmpty(t, usdtMarginedPairs, "FetchTradablePairs for USDT margined futures must return tradable pairs")
	_, err = b.UpdateTicker(t.Context(), usdtMarginedPairs[0], asset.USDTMarginedFutures)
	assert.NoError(t, err, "UpdateTicker for USDT margined futures should not error")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	err := b.UpdateTickers(t.Context(), asset.Spot)
	assert.NoError(t, err, "UpdateTickers for spot should not error")

	err = b.UpdateTickers(t.Context(), asset.CoinMarginedFutures)
	assert.NoError(t, err, "UpdateTickers for coin margined futures should not error")

	err = b.UpdateTickers(t.Context(), asset.USDTMarginedFutures)
	assert.NoError(t, err, "UpdateTickers for USDT margined futures should not error")
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	cp, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSDT")
	_, err = b.UpdateOrderbook(t.Context(), cp, asset.Spot)
	assert.NoError(t, err, "UpdateOrderbook for spot should not error")
	_, err = b.UpdateOrderbook(t.Context(), cp, asset.Margin)
	assert.NoError(t, err, "UpdateOrderbook for margin should not error")
	_, err = b.UpdateOrderbook(t.Context(), cp, asset.USDTMarginedFutures)
	assert.NoError(t, err, "UpdateOrderbook for USDT margined futures should not error")

	cp2, err := currency.NewPairFromString("BTCUSD_PERP")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSD_PERP")
	_, err = b.UpdateOrderbook(t.Context(), cp2, asset.CoinMarginedFutures)
	assert.NoError(t, err, "UpdateOrderbook for coin margined futures should not error")
}

// USDT Margined Futures

func TestUExchangeInfo(t *testing.T) {
	t.Parallel()
	_, err := b.UExchangeInfo(t.Context())
	assert.NoError(t, err, "UExchangeInfo should not error")
}

func TestUFuturesOrderbook(t *testing.T) {
	t.Parallel()
	_, err := b.UFuturesOrderbook(t.Context(), currency.NewBTCUSDT(), 1000)
	assert.NoError(t, err, "UFuturesOrderbook should not error")
}

func TestURecentTrades(t *testing.T) {
	t.Parallel()
	_, err := b.URecentTrades(t.Context(), currency.NewBTCUSDT(), "", 1000)
	assert.NoError(t, err, "URecentTrades should not error")
}

func TestUCompressedTrades(t *testing.T) {
	t.Parallel()
	_, err := b.UCompressedTrades(t.Context(), currency.NewBTCUSDT(), "", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UCompressedTrades should not error")

	start, end := getTime()
	_, err = b.UCompressedTrades(t.Context(), currency.NewPair(currency.LTC, currency.USDT), "", 0, start, end)
	assert.NoError(t, err, "UCompressedTrades with time range should not error")
}

func TestUKlineData(t *testing.T) {
	t.Parallel()
	_, err := b.UKlineData(t.Context(), currency.NewBTCUSDT(), "1d", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UKlineData should not error")

	start, end := getTime()
	_, err = b.UKlineData(t.Context(), currency.NewPair(currency.LTC, currency.USDT), "5m", 0, start, end)
	assert.NoError(t, err, "UKlineData with time range should not error")
}

func TestUGetMarkPrice(t *testing.T) {
	t.Parallel()
	_, err := b.UGetMarkPrice(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UGetMarkPrice with a pair should not error")

	_, err = b.UGetMarkPrice(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "UGetMarkPrice with empty pair should not error")
}

func TestUGetFundingHistory(t *testing.T) {
	t.Parallel()
	_, err := b.UGetFundingHistory(t.Context(), currency.NewBTCUSDT(), 1, time.Time{}, time.Time{})
	assert.NoError(t, err, "UGetFundingHistory should not error")

	start, end := getTime()
	_, err = b.UGetFundingHistory(t.Context(), currency.NewPair(currency.LTC, currency.USDT), 1, start, end)
	assert.NoError(t, err, "UGetFundingHistory with time range should not error")
}

func TestU24HTickerPriceChangeStats(t *testing.T) {
	t.Parallel()
	_, err := b.U24HTickerPriceChangeStats(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "U24HTickerPriceChangeStats with a pair should not error")

	_, err = b.U24HTickerPriceChangeStats(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "U24HTickerPriceChangeStats with empty pair should not error")
}

func TestUSymbolPriceTicker(t *testing.T) {
	t.Parallel()
	_, err := b.USymbolPriceTicker(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "USymbolPriceTicker with a pair should not error")

	_, err = b.USymbolPriceTicker(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "USymbolPriceTicker with empty pair should not error")
}

func TestUSymbolOrderbookTicker(t *testing.T) {
	t.Parallel()
	_, err := b.USymbolOrderbookTicker(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "USymbolOrderbookTicker with a pair should not error")

	_, err = b.USymbolOrderbookTicker(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "USymbolOrderbookTicker with empty pair should not error")
}

func TestUOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := b.UOpenInterest(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UOpenInterest should not error")
}

func TestUOpenInterestStats(t *testing.T) {
	t.Parallel()
	_, err := b.UOpenInterestStats(t.Context(), currency.NewBTCUSDT(), "5m", 1, time.Time{}, time.Time{})
	assert.NoError(t, err, "UOpenInterestStats should not error")

	start, end := getTime()
	_, err = b.UOpenInterestStats(t.Context(), currency.NewPair(currency.LTC, currency.USDT), "1d", 10, start, end)
	assert.NoError(t, err, "UOpenInterestStats with time range should not error")
}

func TestUTopAcccountsLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := b.UTopAcccountsLongShortRatio(t.Context(), currency.NewBTCUSDT(), "5m", 2, time.Time{}, time.Time{})
	assert.NoError(t, err, "UTopAcccountsLongShortRatio should not error")

	start, end := getTime()
	_, err = b.UTopAcccountsLongShortRatio(t.Context(), currency.NewBTCUSDT(), "5m", 2, start, end)
	assert.NoError(t, err, "UTopAcccountsLongShortRatio with time range should not error")
}

func TestUTopPostionsLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := b.UTopPostionsLongShortRatio(t.Context(), currency.NewBTCUSDT(), "5m", 3, time.Time{}, time.Time{})
	assert.NoError(t, err, "UTopPostionsLongShortRatio should not error")

	start, end := getTime()
	_, err = b.UTopPostionsLongShortRatio(t.Context(), currency.NewBTCUSDT(), "1d", 0, start, end)
	assert.NoError(t, err, "UTopPostionsLongShortRatio with time range should not error")
}

func TestUGlobalLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := b.UGlobalLongShortRatio(t.Context(), currency.NewBTCUSDT(), "5m", 3, time.Time{}, time.Time{})
	assert.NoError(t, err, "UGlobalLongShortRatio should not error")

	start, end := getTime()
	_, err = b.UGlobalLongShortRatio(t.Context(), currency.NewBTCUSDT(), "4h", 0, start, end)
	assert.NoError(t, err, "UGlobalLongShortRatio with time range should not error")
}

func TestUTakerBuySellVol(t *testing.T) {
	t.Parallel()
	start, end := getTime()
	_, err := b.UTakerBuySellVol(t.Context(), currency.NewBTCUSDT(), "5m", 10, start, end)
	assert.NoError(t, err, "UTakerBuySellVol should not error")
}

func TestUCompositeIndexInfo(t *testing.T) {
	t.Parallel()
	cp, err := currency.NewPairFromString("DEFI-USDT")
	require.NoError(t, err, "NewPairFromString must not error for DEFI-USDT")
	_, err = b.UCompositeIndexInfo(t.Context(), cp)
	assert.NoError(t, err, "UCompositeIndexInfo with a pair should not error")

	_, err = b.UCompositeIndexInfo(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "UCompositeIndexInfo with empty pair should not error")
}

func TestUFuturesNewOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.UFuturesNewOrder(t.Context(),
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
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	var data []PlaceBatchOrderData
	var tempData PlaceBatchOrderData
	tempData.Symbol = "BTCUSDT"
	tempData.Side = "BUY"
	tempData.OrderType = "LIMIT"
	tempData.Quantity = 4
	tempData.Price = 1
	tempData.TimeInForce = "GTC"
	data = append(data, tempData)
	_, err := b.UPlaceBatchOrders(t.Context(), data)
	assert.NoError(t, err, "UPlaceBatchOrders should not error")
}

func TestUGetOrderData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UGetOrderData(t.Context(), currency.NewBTCUSDT(), "123", "")
	assert.NoError(t, err, "UGetOrderData should not error")
}

func TestUCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.UCancelOrder(t.Context(), currency.NewBTCUSDT(), "123", "")
	assert.NoError(t, err, "UCancelOrder should not error")
}

func TestUCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.UCancelAllOpenOrders(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UCancelAllOpenOrders should not error")
}

func TestUCancelBatchOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.UCancelBatchOrders(t.Context(), currency.NewBTCUSDT(), []string{"123"}, []string{})
	assert.NoError(t, err, "UCancelBatchOrders should not error")
}

func TestUAutoCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.UAutoCancelAllOpenOrders(t.Context(), currency.NewBTCUSDT(), 30)
	assert.NoError(t, err, "UAutoCancelAllOpenOrders should not error")
}

func TestUFetchOpenOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UFetchOpenOrder(t.Context(), currency.NewBTCUSDT(), "123", "")
	assert.NoError(t, err, "UFetchOpenOrder should not error")
}

func TestUAllAccountOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UAllAccountOpenOrders(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UAllAccountOpenOrders should not error")
}

func TestUAllAccountOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UAllAccountOrders(t.Context(), currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "UAllAccountOrders with empty pair should not error")

	_, err = b.UAllAccountOrders(t.Context(), currency.NewBTCUSDT(), 0, 5, time.Now().Add(-time.Hour*4), time.Now())
	assert.NoError(t, err, "UAllAccountOrders with a pair and time range should not error")
}

func TestUAccountBalanceV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UAccountBalanceV2(t.Context())
	assert.NoError(t, err, "UAccountBalanceV2 should not error")
}

func TestUAccountInformationV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UAccountInformationV2(t.Context())
	assert.NoError(t, err, "UAccountInformationV2 should not error")
}

func TestUChangeInitialLeverageRequest(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.UChangeInitialLeverageRequest(t.Context(), currency.NewBTCUSDT(), 2)
	assert.NoError(t, err, "UChangeInitialLeverageRequest should not error")
}

func TestUChangeInitialMarginType(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	err := b.UChangeInitialMarginType(t.Context(), currency.NewBTCUSDT(), "ISOLATED")
	assert.NoError(t, err, "UChangeInitialMarginType should not error")
}

func TestUModifyIsolatedPositionMarginReq(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.UModifyIsolatedPositionMarginReq(t.Context(), currency.NewBTCUSDT(), "LONG", "add", 5)
	assert.NoError(t, err, "UModifyIsolatedPositionMarginReq should not error")
}

func TestUPositionMarginChangeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.UPositionMarginChangeHistory(t.Context(), currency.NewBTCUSDT(), "add", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UPositionMarginChangeHistory should not error")
}

func TestUPositionsInfoV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UPositionsInfoV2(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UPositionsInfoV2 should not error")
}

func TestUAccountTradesHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UAccountTradesHistory(t.Context(), currency.NewBTCUSDT(), "", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UAccountTradesHistory should not error")
}

func TestUAccountIncomeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UAccountIncomeHistory(t.Context(), currency.EMPTYPAIR, "", 5, time.Now().Add(-time.Hour*48), time.Now())
	assert.NoError(t, err, "UAccountIncomeHistory should not error")
}

func TestUGetNotionalAndLeverageBrackets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UGetNotionalAndLeverageBrackets(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UGetNotionalAndLeverageBrackets should not error")
}

func TestUPositionsADLEstimate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UPositionsADLEstimate(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "UPositionsADLEstimate should not error")
}

func TestUAccountForcedOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.UAccountForcedOrders(t.Context(), currency.NewBTCUSDT(), "ADL", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "UAccountForcedOrders should not error")
}

// Coin Margined Futures

func TestGetFuturesExchangeInfo(t *testing.T) {
	t.Parallel()
	_, err := b.FuturesExchangeInfo(t.Context())
	assert.NoError(t, err, "FuturesExchangeInfo should not error")
}

func TestGetFuturesOrderbook(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesOrderbook(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 1000)
	assert.NoError(t, err, "GetFuturesOrderbook should not error")
}

func TestGetFuturesPublicTrades(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesPublicTrades(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5)
	assert.NoError(t, err, "GetFuturesPublicTrades should not error")
}

func TestGetPastPublicTrades(t *testing.T) {
	t.Parallel()
	_, err := b.GetPastPublicTrades(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5, 0)
	assert.NoError(t, err, "GetPastPublicTrades should not error")
}

func TestGetAggregatedTradesList(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesAggregatedTradesList(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 0, 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetFuturesAggregatedTradesList should not error")
}

func TestGetPerpsExchangeInfo(t *testing.T) {
	t.Parallel()
	_, err := b.GetPerpMarkets(t.Context())
	assert.NoError(t, err, "GetPerpMarkets should not error")
}

func TestGetIndexAndMarkPrice(t *testing.T) {
	t.Parallel()
	_, err := b.GetIndexAndMarkPrice(t.Context(), "", "BTCUSD")
	assert.NoError(t, err, "GetIndexAndMarkPrice should not error")
}

func TestGetFuturesKlineData(t *testing.T) {
	t.Parallel()
	r, err := b.GetFuturesKlineData(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "1M", 5, time.Time{}, time.Time{})
	require.NoError(t, err, "GetFuturesKlineData must not error")
	if mockTests {
		require.Equal(t, 5, len(r), "GetFuturesKlineData must return 5 items in mock test")
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
	r, err = b.GetFuturesKlineData(t.Context(), currency.NewPairWithDelimiter("LTCUSD", "PERP", "_"), "5m", 5, start, end)
	require.NoError(t, err, "GetFuturesKlineData must not error")
	assert.NotEmpty(t, r, "GetFuturesKlineData should return data")
}

func TestGetContinuousKlineData(t *testing.T) {
	t.Parallel()
	_, err := b.GetContinuousKlineData(t.Context(), "BTCUSD", "CURRENT_QUARTER", "1M", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetContinuousKlineData should not error")

	start, end := getTime()
	_, err = b.GetContinuousKlineData(t.Context(), "BTCUSD", "CURRENT_QUARTER", "1M", 5, start, end)
	assert.NoError(t, err, "GetContinuousKlineData with time range should not error")
}

func TestGetIndexPriceKlines(t *testing.T) {
	t.Parallel()
	_, err := b.GetIndexPriceKlines(t.Context(), "BTCUSD", "1M", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetIndexPriceKlines should not error")

	start, end := getTime()
	_, err = b.GetIndexPriceKlines(t.Context(), "BTCUSD", "1M", 5, start, end)
	assert.NoError(t, err, "GetIndexPriceKlines with time range should not error")
}

func TestGetFuturesSwapTickerChangeStats(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesSwapTickerChangeStats(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesSwapTickerChangeStats with a pair should not error")

	_, err = b.GetFuturesSwapTickerChangeStats(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesSwapTickerChangeStats with a pair (called again) should not error")

	_, err = b.GetFuturesSwapTickerChangeStats(t.Context(), currency.EMPTYPAIR, "")
	assert.NoError(t, err, "GetFuturesSwapTickerChangeStats with empty pair should not error")
}

func TestFuturesGetFundingHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesGetFundingHistory(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "FuturesGetFundingHistory should not error")

	start, end := getTime()
	_, err = b.FuturesGetFundingHistory(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 50, start, end)
	assert.NoError(t, err, "FuturesGetFundingHistory with time range should not error")
}

func TestGetFuturesHistoricalTrades(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetFuturesHistoricalTrades(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "", 5)
	assert.NoError(t, err, "GetFuturesHistoricalTrades should not error")

	_, err = b.GetFuturesHistoricalTrades(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "", 0)
	assert.NoError(t, err, "GetFuturesHistoricalTrades with limit 0 should not error")
}

func TestGetFuturesSymbolPriceTicker(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesSymbolPriceTicker(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesSymbolPriceTicker should not error")
}

func TestGetFuturesOrderbookTicker(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesOrderbookTicker(t.Context(), currency.EMPTYPAIR, "")
	assert.NoError(t, err, "GetFuturesOrderbookTicker with empty pair should not error")

	_, err = b.GetFuturesOrderbookTicker(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesOrderbookTicker with a pair should not error")
}

func TestOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := b.OpenInterest(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"))
	assert.NoError(t, err, "OpenInterest should not error")
}

func TestGetOpenInterestStats(t *testing.T) {
	t.Parallel()
	_, err := b.GetOpenInterestStats(t.Context(), "BTCUSD", "CURRENT_QUARTER", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetOpenInterestStats should not error")

	start, end := getTime()
	_, err = b.GetOpenInterestStats(t.Context(), "BTCUSD", "CURRENT_QUARTER", "5m", 0, start, end)
	assert.NoError(t, err, "GetOpenInterestStats with time range should not error")
}

func TestGetTraderFuturesAccountRatio(t *testing.T) {
	t.Parallel()
	_, err := b.GetTraderFuturesAccountRatio(t.Context(), "BTCUSD", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetTraderFuturesAccountRatio should not error")

	start, end := getTime()
	_, err = b.GetTraderFuturesAccountRatio(t.Context(), "BTCUSD", "5m", 0, start, end)
	assert.NoError(t, err, "GetTraderFuturesAccountRatio with time range should not error")
}

func TestGetTraderFuturesPositionsRatio(t *testing.T) {
	t.Parallel()
	_, err := b.GetTraderFuturesPositionsRatio(t.Context(), "BTCUSD", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetTraderFuturesPositionsRatio should not error")

	start, end := getTime()
	_, err = b.GetTraderFuturesPositionsRatio(t.Context(), "BTCUSD", "5m", 0, start, end)
	assert.NoError(t, err, "GetTraderFuturesPositionsRatio with time range should not error")
}

func TestGetMarketRatio(t *testing.T) {
	t.Parallel()
	_, err := b.GetMarketRatio(t.Context(), "BTCUSD", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetMarketRatio should not error")

	start, end := getTime()
	_, err = b.GetMarketRatio(t.Context(), "BTCUSD", "5m", 0, start, end)
	assert.NoError(t, err, "GetMarketRatio with time range should not error")
}

func TestGetFuturesTakerVolume(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesTakerVolume(t.Context(), "BTCUSD", "ALL", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetFuturesTakerVolume should not error")

	start, end := getTime()
	_, err = b.GetFuturesTakerVolume(t.Context(), "BTCUSD", "ALL", "5m", 0, start, end)
	assert.NoError(t, err, "GetFuturesTakerVolume with time range should not error")
}

func TestFuturesBasisData(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesBasisData(t.Context(), "BTCUSD", "CURRENT_QUARTER", "5m", 0, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetFuturesBasisData should not error")

	start, end := getTime()
	_, err = b.GetFuturesBasisData(t.Context(), "BTCUSD", "CURRENT_QUARTER", "5m", 0, start, end)
	assert.NoError(t, err, "GetFuturesBasisData with time range should not error")
}

func TestFuturesNewOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.FuturesNewOrder(
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
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	var data []PlaceBatchOrderData
	var tempData PlaceBatchOrderData
	tempData.Symbol = "BTCUSD_PERP"
	tempData.Side = "BUY"
	tempData.OrderType = "LIMIT"
	tempData.Quantity = 1
	tempData.Price = 1
	tempData.TimeInForce = "GTC"

	data = append(data, tempData)
	_, err := b.FuturesBatchOrder(t.Context(), data)
	assert.NoError(t, err, "FuturesBatchOrder should not error")
}

func TestFuturesBatchCancelOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.FuturesBatchCancelOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), []string{"123"}, []string{})
	assert.NoError(t, err, "FuturesBatchCancelOrders should not error")
}

func TestFuturesGetOrderData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesGetOrderData(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "123", "")
	assert.NoError(t, err, "FuturesGetOrderData should not error")
}

func TestCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.FuturesCancelAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"))
	assert.NoError(t, err, "FuturesCancelAllOpenOrders should not error")
}

func TestAutoCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.AutoCancelAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 30000)
	assert.NoError(t, err, "AutoCancelAllOpenOrders should not error")
}

func TestFuturesOpenOrderData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesOpenOrderData(t.Context(), currency.NewBTCUSDT(), "", "")
	assert.NoError(t, err, "FuturesOpenOrderData should not error")
}

func TestGetFuturesAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetFuturesAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	assert.NoError(t, err, "GetFuturesAllOpenOrders should not error")
}

func TestGetAllFuturesOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetAllFuturesOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), currency.EMPTYPAIR, time.Time{}, time.Time{}, 0, 2)
	assert.NoError(t, err, "GetAllFuturesOrders should not error")
}

func TestFuturesChangeMarginType(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.FuturesChangeMarginType(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "ISOLATED")
	assert.NoError(t, err, "FuturesChangeMarginType should not error")
}

func TestGetFuturesAccountBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetFuturesAccountBalance(t.Context())
	assert.NoError(t, err, "GetFuturesAccountBalance should not error")
}

func TestGetFuturesAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetFuturesAccountInfo(t.Context())
	assert.NoError(t, err, "GetFuturesAccountInfo should not error")
}

func TestFuturesChangeInitialLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.FuturesChangeInitialLeverage(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5)
	assert.NoError(t, err, "FuturesChangeInitialLeverage should not error")
}

func TestModifyIsolatedPositionMargin(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.ModifyIsolatedPositionMargin(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "BOTH", "add", 5)
	assert.NoError(t, err, "ModifyIsolatedPositionMargin should not error")
}

func TestFuturesMarginChangeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesMarginChangeHistory(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "add", time.Time{}, time.Time{}, 10)
	assert.NoError(t, err, "FuturesMarginChangeHistory should not error")
}

func TestFuturesPositionsInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesPositionsInfo(t.Context(), "BTCUSD", "")
	assert.NoError(t, err, "FuturesPositionsInfo should not error")
}

func TestFuturesTradeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesTradeHistory(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "", time.Time{}, time.Time{}, 5, 0)
	assert.NoError(t, err, "FuturesTradeHistory should not error")
}

func TestFuturesIncomeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesIncomeHistory(t.Context(), currency.EMPTYPAIR, "TRANSFER", time.Time{}, time.Time{}, 5)
	assert.NoError(t, err, "FuturesIncomeHistory should not error")
}

func TestFuturesForceOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesForceOrders(t.Context(), currency.EMPTYPAIR, "ADL", time.Time{}, time.Time{})
	assert.NoError(t, err, "FuturesForceOrders should not error")
}

func TestUGetNotionalLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesNotionalBracket(t.Context(), "BTCUSD")
	assert.NoError(t, err, "FuturesNotionalBracket with a symbol should not error")

	_, err = b.FuturesNotionalBracket(t.Context(), "")
	assert.NoError(t, err, "FuturesNotionalBracket with empty symbol should not error")
}

func TestFuturesPositionsADLEstimate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FuturesPositionsADLEstimate(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "FuturesPositionsADLEstimate should not error")
}

func TestGetMarkPriceKline(t *testing.T) {
	t.Parallel()
	_, err := b.GetMarkPriceKline(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "1M", 5, time.Time{}, time.Time{})
	assert.NoError(t, err, "GetMarkPriceKline should not error")
}

func TestGetExchangeInfo(t *testing.T) {
	t.Parallel()
	info, err := b.GetExchangeInfo(t.Context())
	require.NoError(t, err, "GetExchangeInfo must not error")
	if mockTests {
		exp := time.Date(2024, 5, 10, 6, 8, 1, int(707*time.Millisecond), time.UTC)
		assert.Truef(t, info.ServerTime.Time().Equal(exp), "GetExchangeInfo server time in mock test should be %v, received %v", exp.UTC(), info.ServerTime.Time().UTC())
	} else {
		assert.WithinRange(t, info.ServerTime.Time(), time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), "GetExchangeInfo server time should be within a day of now in live test")
	}
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := b.FetchTradablePairs(t.Context(), asset.Spot)
	assert.NoError(t, err, "FetchTradablePairs for spot should not error")

	_, err = b.FetchTradablePairs(t.Context(), asset.CoinMarginedFutures)
	assert.NoError(t, err, "FetchTradablePairs for coin margined futures should not error")

	_, err = b.FetchTradablePairs(t.Context(), asset.USDTMarginedFutures)
	assert.NoError(t, err, "FetchTradablePairs for USDT margined futures should not error")
}

func TestGetOrderBook(t *testing.T) {
	t.Parallel()
	_, err := b.GetOrderBook(t.Context(),
		OrderBookDataRequestParams{
			Symbol: currency.NewBTCUSDT(),
			Limit:  1000,
		})
	assert.NoError(t, err, "GetOrderBook should not error")
}

func TestGetMostRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := b.GetMostRecentTrades(t.Context(),
		RecentTradeRequestParams{
			Symbol: currency.NewBTCUSDT(),
			Limit:  15,
		})
	assert.NoError(t, err, "GetMostRecentTrades should not error")
}

func TestGetHistoricalTrades(t *testing.T) {
	t.Parallel()
	_, err := b.GetHistoricalTrades(t.Context(), "BTCUSDT", 5, -1)
	if !mockTests && err == nil {
		assert.Error(t, err, "GetHistoricalTrades in live test should error for invalid parameters")
	} else if mockTests && err != nil {
		assert.NoError(t, err, "GetHistoricalTrades in mock test should not error")
	}
}

func TestGetAggregatedTrades(t *testing.T) {
	t.Parallel()
	_, err := b.GetAggregatedTrades(t.Context(),
		&AggregatedTradeRequestParams{
			Symbol: currency.NewBTCUSDT(),
			Limit:  5,
		})
	assert.NoError(t, err, "GetAggregatedTrades should not error")
}

func TestGetSpotKline(t *testing.T) {
	t.Parallel()
	start, end := getTime()
	_, err := b.GetSpotKline(t.Context(),
		&KlinesRequestParams{
			Symbol:    currency.NewBTCUSDT(),
			Interval:  kline.FiveMin.Short(),
			Limit:     24,
			StartTime: start,
			EndTime:   end,
		})
	assert.NoError(t, err, "GetSpotKline should not error")
}

func TestGetAveragePrice(t *testing.T) {
	t.Parallel()
	_, err := b.GetAveragePrice(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "GetAveragePrice should not error")
}

func TestGetPriceChangeStats(t *testing.T) {
	t.Parallel()
	_, err := b.GetPriceChangeStats(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "GetPriceChangeStats should not error")
}

func TestGetTickers(t *testing.T) {
	t.Parallel()
	_, err := b.GetTickers(t.Context())
	require.NoError(t, err, "GetTickers with no pairs must not error")

	resp, err := b.GetTickers(t.Context(),
		currency.NewBTCUSDT(),
		currency.NewPair(currency.ETH, currency.USDT))
	require.NoError(t, err, "GetTickers with specific pairs must not error")
	require.Len(t, resp, 2, "GetTickers with specific pairs must return 2 tickers")
}

func TestGetLatestSpotPrice(t *testing.T) {
	t.Parallel()
	_, err := b.GetLatestSpotPrice(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "GetLatestSpotPrice should not error")
}

func TestGetBestPrice(t *testing.T) {
	t.Parallel()
	_, err := b.GetBestPrice(t.Context(), currency.NewBTCUSDT())
	assert.NoError(t, err, "GetBestPrice should not error")
}

func TestQueryOrder(t *testing.T) {
	t.Parallel()
	_, err := b.QueryOrder(t.Context(), currency.NewBTCUSDT(), "", 1337)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "QueryOrder with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "QueryOrder without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "QueryOrder in mock test should not error")
	}
}

func TestOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.OpenOrders(t.Context(), currency.EMPTYPAIR)
	assert.NoError(t, err, "OpenOrders with empty pair should not error")

	p := currency.NewBTCUSDT()
	_, err = b.OpenOrders(t.Context(), p)
	assert.NoError(t, err, "OpenOrders with a specific pair should not error")
}

func TestAllOrders(t *testing.T) {
	t.Parallel()
	_, err := b.AllOrders(t.Context(), currency.NewBTCUSDT(), "", "")
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "AllOrders with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "AllOrders without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "AllOrders in mock test should not error")
	}
}

func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()
	_, err := b.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFeeByType must not error")

	if !sharedtestvalues.AreAPICredentialsSet(b) || mockTests {
		assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "FeeType should be OfflineTradeFee when credentials are not set or in mock mode")
	} else {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "FeeType should be CryptocurrencyTradeFee when credentials are set and not in mock mode")
	}
}

func TestGetFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()

	if sharedtestvalues.AreAPICredentialsSet(b) && mockTests {
		// CryptocurrencyTradeFee Basic
		_, err := b.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee for CryptocurrencyTradeFee (Basic) should not error in mock test")

		// CryptocurrencyTradeFee High quantity
		feeBuilder = setFeeBuilder()
		feeBuilder.Amount = 1000
		feeBuilder.PurchasePrice = 1000
		_, err = b.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee for CryptocurrencyTradeFee (High Quantity) should not error in mock test")

		// CryptocurrencyTradeFee IsMaker
		feeBuilder = setFeeBuilder()
		feeBuilder.IsMaker = true
		_, err = b.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee for CryptocurrencyTradeFee (IsMaker) should not error in mock test")

		// CryptocurrencyTradeFee Negative purchase price
		feeBuilder = setFeeBuilder()
		feeBuilder.PurchasePrice = -1000
		_, err = b.GetFee(t.Context(), feeBuilder)
		assert.NoError(t, err, "GetFee for CryptocurrencyTradeFee (Negative Purchase Price) should not error in mock test")
	}

	// CryptocurrencyWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err := b.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee for CryptocurrencyWithdrawalFee (Basic) should not error")

	// CryptocurrencyDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyDepositFee
	_, err = b.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee for CryptocurrencyDepositFee (Basic) should not error")

	// InternationalBankDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	feeBuilder.FiatCurrency = currency.HKD
	_, err = b.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee for InternationalBankDepositFee (Basic) should not error")

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.HKD
	_, err = b.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err, "GetFee for InternationalBankWithdrawalFee (Basic) should not error")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	expectedResult := exchange.AutoWithdrawCryptoText + " & " + exchange.NoFiatWithdrawalsText
	withdrawPermissions := b.FormatWithdrawPermissions()
	assert.Equal(t, expectedResult, withdrawPermissions, "FormatWithdrawPermissions should return the correct string")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	pair, err := currency.NewPairFromString("BTC_USDT")
	require.NoError(t, err, "NewPairFromString must not error for BTC_USDT")

	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		Pairs:     currency.Pairs{pair},
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	_, err = b.GetActiveOrders(t.Context(), &getOrdersRequest)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "GetActiveOrders with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "GetActiveOrders without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "GetActiveOrders in mock test should not error")
	}
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	_, err := b.GetOrderHistory(t.Context(), &getOrdersRequest)
	assert.Error(t, err, "GetOrderHistory without pairs should error")
	assert.ErrorContains(t, err, "at least one currency is required to fetch order history", "GetOrderHistory error message should be correct")

	getOrdersRequest.Pairs = []currency.Pair{
		currency.NewPair(currency.LTC, currency.BTC),
	}

	_, err = b.GetOrderHistory(t.Context(), &getOrdersRequest)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "GetOrderHistory with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "GetOrderHistory without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "GetOrderHistory in mock test should not error")
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

	err := b.NewOrderTest(t.Context(), req)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "NewOrderTest (BUY) with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "NewOrderTest (BUY) without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "NewOrderTest (BUY) in mock test should not error")
	}

	req = &NewOrderRequest{
		Symbol:        currency.NewPair(currency.LTC, currency.BTC),
		Side:          order.Sell.String(),
		TradeType:     BinanceRequestParamsOrderMarket,
		Price:         0.0045,
		QuoteOrderQty: 10,
	}

	err = b.NewOrderTest(t.Context(), req)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "NewOrderTest (SELL) with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "NewOrderTest (SELL) without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "NewOrderTest (SELL) in mock test should not error")
	}
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	p := currency.NewBTCUSDT()
	start := time.Unix(1577977445, 0)  // 2020-01-02 15:04:05
	end := start.Add(15 * time.Minute) // 2020-01-02 15:19:05
	result, err := b.GetHistoricTrades(t.Context(), p, asset.Spot, start, end)
	assert.NoError(t, err, "GetHistoricTrades should not error")

	expectedLen := 2134
	if mockTests {
		expectedLen = 1002
	}
	assert.Len(t, result, expectedLen, "GetHistoricTrades should return correct number of entries")

	for _, r := range result {
		assert.WithinRange(t, r.Timestamp, start, end, "All trades should be within time range")
	}
}

func TestGetAggregatedTradesBatched(t *testing.T) {
	t.Parallel()
	currencyPair, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSDT")

	start, err := time.Parse(time.RFC3339, "2020-01-02T15:04:05Z")
	require.NoError(t, err, "Parsing start time must not error")

	expectTime, err := time.Parse(time.RFC3339Nano, "2020-01-02T16:19:04.831Z")
	require.NoError(t, err, "Parsing expectTime must not error")

	tests := []struct {
		name         string
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
			result, err := b.GetAggregatedTrades(t.Context(), tt.args)
			require.NoError(t, err, "GetAggregatedTrades must not error")
			require.Len(t, result, tt.numExpected, "GetAggregatedTradesBatched must return expected number of entries")
			lastTradeTime := result[len(result)-1].TimeStamp
			assert.Truef(t, lastTradeTime.Time().Equal(tt.lastExpected), "Last trade time should be %v, got %v", tt.lastExpected.UTC(), lastTradeTime.Time().UTC())
		})
	}
}

func TestGetAggregatedTradesErrors(t *testing.T) {
	t.Parallel()
	start, err := time.Parse(time.RFC3339, "2020-01-02T15:04:05Z")
	require.NoError(t, err, "Parsing start time must not error")

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
			_, err := b.GetAggregatedTrades(t.Context(), tt.args)
			assert.Error(t, err, "GetAggregatedTrades with invalid params should error")
		})
	}
}

// Any tests below this line have the ability to impact your orders on the exchange. Enable canManipulateRealOrders to run them
// -----------------------------------------------------------------------------------------------------------------------------

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCannotManipulateOrders(t, b, canManipulateRealOrders)
	}

	orderSubmission := &order.Submit{
		Exchange: b.Name,
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

	_, err := b.SubmitOrder(t.Context(), orderSubmission)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "SubmitOrder with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "SubmitOrder without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "SubmitOrder in mock test should not error")
	}
}

func TestCancelExchangeOrder(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCannotManipulateOrders(t, b, canManipulateRealOrders)
	}
	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currency.NewPair(currency.LTC, currency.BTC),
		AssetType: asset.Spot,
	}

	err := b.CancelOrder(t.Context(), orderCancellation)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "CancelOrder with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "CancelOrder without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "CancelOrder in mock test should not error")
	}
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCannotManipulateOrders(t, b, canManipulateRealOrders)
	}
	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currency.NewPair(currency.LTC, currency.BTC),
		AssetType: asset.Spot,
	}

	_, err := b.CancelAllOrders(t.Context(), orderCancellation)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "CancelAllOrders with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "CancelAllOrders without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "CancelAllOrders in mock test should not error")
	}
}

func TestGetAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
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
			_, err := b.UpdateAccountInfo(t.Context(), assetType)
			assert.NoErrorf(t, err, "UpdateAccountInfo for asset type %s should not error", assetType)
		})
	}
}

func TestWrapperGetActiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	p, err := currency.NewPairFromString("EOS-USDT")
	require.NoError(t, err, "NewPairFromString must not error for EOS-USDT")

	_, err = b.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		Pairs:     currency.Pairs{p},
		AssetType: asset.CoinMarginedFutures,
	})
	assert.NoError(t, err, "GetActiveOrders for coin margined futures should not error")

	p2, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSDT")
	_, err = b.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		Pairs:     currency.Pairs{p2},
		AssetType: asset.USDTMarginedFutures,
	})
	assert.NoError(t, err, "GetActiveOrders for USDT margined futures should not error")
}

func TestWrapperGetOrderHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	p, err := currency.NewPairFromString("EOSUSD_PERP")
	require.NoError(t, err, "NewPairFromString must not error for EOSUSD_PERP")

	_, err = b.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		Type:        order.AnyType,
		Side:        order.AnySide,
		FromOrderID: "123",
		Pairs:       currency.Pairs{p},
		AssetType:   asset.CoinMarginedFutures,
	})
	assert.NoError(t, err, "GetOrderHistory for coin margined futures should not error")

	p2, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSDT")
	_, err = b.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		Type:        order.AnyType,
		Side:        order.AnySide,
		FromOrderID: "123",
		Pairs:       currency.Pairs{p2},
		AssetType:   asset.USDTMarginedFutures,
	})
	assert.NoError(t, err, "GetOrderHistory for USDT margined futures should not error")

	_, err = b.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		AssetType: asset.USDTMarginedFutures,
	})
	assert.Error(t, err, "GetOrderHistory with invalid params should error")
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	p, err := currency.NewPairFromString("EOS-USDT")
	require.NoError(t, err, "NewPairFromString must not error for EOS-USDT")

	fPair, err := b.FormatExchangeCurrency(p, asset.CoinMarginedFutures)
	require.NoError(t, err, "FormatExchangeCurrency for coin margined futures must not error")
	err = b.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.CoinMarginedFutures,
		Pair:      fPair,
		OrderID:   "1234",
	})
	assert.NoError(t, err, "CancelOrder for coin margined futures should not error")

	p2, err := currency.NewPairFromString("BTC-USDT")
	require.NoError(t, err, "NewPairFromString must not error for BTC-USDT")
	fpair2, err := b.FormatExchangeCurrency(p2, asset.USDTMarginedFutures)
	require.NoError(t, err, "FormatExchangeCurrency for USDT margined futures must not error")
	err = b.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.USDTMarginedFutures,
		Pair:      fpair2,
		OrderID:   "1234",
	})
	assert.NoError(t, err, "CancelOrder for USDT margined futures should not error")
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	tradablePairs, err := b.FetchTradablePairs(t.Context(), asset.CoinMarginedFutures)
	require.NoError(t, err, "FetchTradablePairs for coin margined futures must not error")
	require.NotEmpty(t, tradablePairs, "FetchTradablePairs for coin margined futures must return tradable pairs")

	_, err = b.GetOrderInfo(t.Context(), "123", tradablePairs[0], asset.CoinMarginedFutures)
	assert.NoError(t, err, "GetOrderInfo should not error")
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	_, err := b.ModifyOrder(t.Context(), &order.Modify{AssetType: asset.Spot})
	assert.Error(t, err, "ModifyOrder should error as it's not supported")
}

func TestGetAllCoinsInfo(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	}
	_, err := b.GetAllCoinsInfo(t.Context())
	assert.NoError(t, err, "GetAllCoinsInfo should not error")
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCannotManipulateOrders(t, b, canManipulateRealOrders)
	}

	withdrawCryptoRequest := withdraw.Request{
		Exchange:    b.Name,
		Amount:      -1,
		Currency:    currency.BTC,
		Description: "WITHDRAW IT ALL",
		Crypto: withdraw.CryptoRequest{
			Address: core.BitcoinDonationAddress,
		},
	}

	_, err := b.WithdrawCryptocurrencyFunds(t.Context(), &withdrawCryptoRequest)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "WithdrawCryptocurrencyFunds with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "WithdrawCryptocurrencyFunds without API credentials in live test should error")
	}
}

func TestDepositHistory(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCannotManipulateOrders(t, b, canManipulateRealOrders)
	}
	_, err := b.DepositHistory(t.Context(), currency.ETH, "", time.Time{}, time.Time{}, 0, 10000)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "DepositHistory with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "DepositHistory without API credentials in live test should error")
	}
}

func TestWithdrawHistory(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCannotManipulateOrders(t, b, canManipulateRealOrders)
	}
	_, err := b.GetWithdrawalsHistory(t.Context(), currency.ETH, asset.Spot)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "GetWithdrawalsHistory with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "GetWithdrawalsHistory without API credentials in live test should error")
	}
}

func TestWithdrawFiat(t *testing.T) {
	t.Parallel()
	_, err := b.WithdrawFiatFunds(t.Context(), &withdraw.Request{})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "WithdrawFiatFunds should return ErrFunctionNotSupported")
}

func TestWithdrawInternationalBank(t *testing.T) {
	t.Parallel()
	_, err := b.WithdrawFiatFundsToInternationalBank(t.Context(), &withdraw.Request{})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "WithdrawFiatFundsToInternationalBank should return ErrFunctionNotSupported")
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	_, err := b.GetDepositAddress(t.Context(), currency.USDT, "", currency.BNB.String())
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "GetDepositAddress with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "GetDepositAddress without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "GetDepositAddress in mock test should not error")
	}
}

func BenchmarkWsHandleData(bb *testing.B) {
	bb.ReportAllocs()
	ap, err := b.CurrencyPairs.GetPairs(asset.Spot, false)
	require.NoError(bb, err)
	err = b.CurrencyPairs.StorePairs(asset.Spot, ap, true)
	require.NoError(bb, err)

	data, err := os.ReadFile("testdata/wsHandleData.json")
	require.NoError(bb, err)
	lines := bytes.Split(data, []byte("\n"))
	require.Len(bb, lines, 8)
	go func() {
		for {
			<-b.Websocket.DataHandler
		}
	}()
	for bb.Loop() {
		for x := range lines {
			assert.NoError(bb, b.wsHandleData(lines[x]))
		}
	}
}

func TestSubscribe(t *testing.T) {
	t.Parallel()
	b := new(Binance) //nolint:govet // Intentional shadow to avoid future copy/paste mistakes
	require.NoError(t, testexch.Setup(b), "Test instance Setup must not error")
	channels, err := b.generateSubscriptions() // Note: We grab this before it's overwritten by MockWsInstance below
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
		b = testexch.MockWsInstance[Binance](t, mockws.CurryWsMockUpgrader(t, mock))
	} else {
		testexch.SetupWs(t, b)
	}
	err = b.Subscribe(channels)
	require.NoError(t, err, "Subscribe must not error")
	err = b.Unsubscribe(channels)
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
	b := testexch.MockWsInstance[Binance](t, mockws.CurryWsMockUpgrader(t, mock)) //nolint:govet // Intentional shadow to avoid future copy/paste mistakes
	err := b.Subscribe(channels)
	assert.ErrorIs(t, err, common.ErrUnknownError, "Subscribe should error correctly")
	assert.ErrorContains(t, err, "carrots", "Subscribe should error containing the carrots")
}

func TestWsTickerUpdate(t *testing.T) {
	t.Parallel()
	pressXToJSON := []byte(`{"stream":"btcusdt@ticker","data":{"e":"24hrTicker","E":1580254809477,"s":"BTCUSDT","p":"420.97000000","P":"4.720","w":"9058.27981278","x":"8917.98000000","c":"9338.96000000","Q":"0.17246300","b":"9338.03000000","B":"0.18234600","a":"9339.70000000","A":"0.14097600","o":"8917.99000000","h":"9373.19000000","l":"8862.40000000","v":"72229.53692000","q":"654275356.16896672","O":1580168409456,"C":1580254809456,"F":235294268,"L":235894703,"n":600436}}`)
	err := b.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData for ticker update should not error")
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
	err := b.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData for kline update should not error")
}

func TestWsTradeUpdate(t *testing.T) {
	t.Parallel()
	b.SetSaveTradeDataStatus(true)
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
	err := b.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData for trade update should not error")
}

func TestWsDepthUpdate(t *testing.T) {
	t.Parallel()
	b := new(Binance) //nolint:govet // Intentional shadow to avoid future copy/paste mistakes
	require.NoError(t, testexch.Setup(b), "Test instance Setup must not error")
	b.setupOrderbookManager()
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
	err := b.SeedLocalCacheWithBook(p, &book)
	require.NoError(t, err, "SeedLocalCacheWithBook must not error")

	err = b.wsHandleData(update1)
	require.NoError(t, err, "wsHandleData for update1 must not error")

	b.obm.state[currency.BTC][currency.USDT][asset.Spot].fetchingBook = false

	ob, err := b.Websocket.Orderbook.GetOrderbook(p, asset.Spot)
	require.NoError(t, err, "GetOrderbook must not error")

	assert.Equal(t, seedLastUpdateID, ob.LastUpdateID, "LastUpdateID should match seed for old update")
	assert.Equal(t, 2.3, ob.Asks[2].Amount, "Ask amount should not be altered by outdated update")
	assert.Equal(t, 0.163526, ob.Bids[1].Amount, "Bid amount should not be altered by outdated update")

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

	err = b.wsHandleData(update2)
	assert.NoError(t, err, "wsHandleData for update2 should not error")

	ob, err = b.Websocket.Orderbook.GetOrderbook(p, asset.Spot)
	require.NoError(t, err, "GetOrderbook after update2 must not error")
	assert.Equal(t, int64(165), ob.LastUpdateID, "LastUpdateID should be updated for new update")
	assert.Equal(t, 2.3, ob.Asks[2].Amount, "Ask amount should be correct after update2")
	assert.Equal(t, 1.9, ob.Asks[3].Amount, "Ask amount should be correct after update2")
	assert.Equal(t, 0.163526, ob.Bids[1].Amount, "Bid amount should be correct after update2")

	// reset order book sync status
	b.obm.state[currency.BTC][currency.USDT][asset.Spot].lastUpdateID = 0
}

func TestWsBalanceUpdate(t *testing.T) {
	t.Parallel()
	pressXToJSON := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{
  "e": "balanceUpdate",         
  "E": 1573200697110,           
  "a": "BTC",                   
  "d": "100.00000000",          
  "T": 1573200697068}}`)
	err := b.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData for balance update should not error")
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
	err := b.wsHandleData(pressXToJSON)
	assert.NoError(t, err, "wsHandleData for OCO update should not error")
}

func TestGetWsAuthStreamKey(t *testing.T) {
	t.Parallel()
	key, err := b.GetWsAuthStreamKey(t.Context())
	switch {
	case mockTests && err != nil:
		require.NoError(t, err, "GetWsAuthStreamKey in mock test must not error")
	case !mockTests && sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		require.NoError(t, err, "GetWsAuthStreamKey with API credentials must not error")
	case !mockTests && !sharedtestvalues.AreAPICredentialsSet(b) && err == nil:
		assert.Error(t, err, "GetWsAuthStreamKey without API credentials in live test should error")
	}

	if sharedtestvalues.AreAPICredentialsSet(b) || mockTests {
		assert.NotEmpty(t, key, "GetWsAuthStreamKey should return a key when credentials are set or in mock mode")
	}
}

func TestMaintainWsAuthStreamKey(t *testing.T) {
	t.Parallel()
	err := b.MaintainWsAuthStreamKey(t.Context())
	switch {
	case mockTests && err != nil:
		require.NoError(t, err, "MaintainWsAuthStreamKey in mock test must not error")
	case !mockTests && sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		require.NoError(t, err, "MaintainWsAuthStreamKey with API credentials must not error")
	case !mockTests && !sharedtestvalues.AreAPICredentialsSet(b) && err == nil:
		assert.Error(t, err, "MaintainWsAuthStreamKey without API credentials in live test should error")
	}
}

func TestExecutionTypeToOrderStatus(t *testing.T) {
	t.Parallel()
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
	for _, tc := range testCases {
		t.Run(tc.Case, func(t *testing.T) {
			t.Parallel()
			result, _ := stringToOrderStatus(tc.Case)
			assert.Equal(t, tc.Result, result, "Result should match expected order status")
		})
	}
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	startTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := startTime.Add(time.Hour * 24 * 7)
	bAssets := b.GetAssetTypes(false)
	for _, assetType := range bAssets {
		cps, err := b.GetAvailablePairs(assetType)
		require.NoErrorf(t, err, "GetAvailablePairs for asset %s must not error", assetType)
		require.NotEmptyf(t, cps, "GetAvailablePairs for asset %s must return at least one pair", assetType)
		err = b.CurrencyPairs.EnablePair(assetType, cps[0])
		require.Truef(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled),
			"EnablePair for asset %s and pair %s must not error: %s", assetType, cps[0], err)
		_, err = b.GetHistoricCandles(t.Context(), cps[0], assetType, kline.OneDay, startTime, end)
		assert.NoErrorf(t, err, "GetHistoricCandles should not error for asset %s and pair %s", assetType, cps[0])
	}

	startTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := b.GetHistoricCandles(t.Context(), currency.NewBTCUSDT(), asset.Spot, kline.Interval(time.Hour*7), startTime, end)
	require.ErrorIs(t, err, kline.ErrRequestExceedsExchangeLimits, "GetHistoricCandles with interval exceeding limits must error")
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	startTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := startTime.Add(time.Hour * 24 * 7)
	bAssets := b.GetAssetTypes(false)
	for i := range bAssets {
		t.Run(bAssets[i].String(), func(t *testing.T) {
			t.Parallel()
			cps, err := b.GetAvailablePairs(bAssets[i])
			require.NoErrorf(t, err, "GetAvailablePairs for asset %s must not error", bAssets[i])
			require.NotEmptyf(t, cps, "GetAvailablePairs for asset %s must return at least one pair", bAssets[i])
			err = b.CurrencyPairs.EnablePair(bAssets[i], cps[0])
			require.Truef(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled),
				"EnablePair for asset %s and pair %s must not error: %s", bAssets[i], cps[0], err)
			_, err = b.GetHistoricCandlesExtended(t.Context(), cps[0], bAssets[i], kline.OneDay, startTime, end)
			assert.NoErrorf(t, err, "GetHistoricCandlesExtended should not error for asset %s and pair %s", bAssets[i], cps[0])
		})
	}
}

func TestFormatExchangeKlineInterval(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		interval kline.Interval
		output   string
	}{
		{kline.OneMin, "1m"},
		{kline.OneDay, "1d"},
		{kline.OneWeek, "1w"},
		{kline.OneMonth, "1M"},
	} {
		t.Run(tc.interval.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tc.output, b.FormatExchangeKlineInterval(tc.interval), "FormatExchangeKlineInterval should return correct string for interval %s", tc.interval)
		})
	}
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	pair := currency.NewBTCUSDT()
	_, err := b.GetRecentTrades(t.Context(), pair, asset.Spot)
	assert.NoError(t, err, "GetRecentTrades for spot should not error")

	_, err = b.GetRecentTrades(t.Context(), pair, asset.USDTMarginedFutures)
	assert.NoError(t, err, "GetRecentTrades for USDT margined futures should not error")

	pair.Base = currency.NewCode("BTCUSD")
	pair.Quote = currency.PERP
	_, err = b.GetRecentTrades(t.Context(), pair, asset.CoinMarginedFutures)
	assert.NoError(t, err, "GetRecentTrades for coin margined futures should not error")
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	_, err := b.GetAvailableTransferChains(t.Context(), currency.BTC)
	switch {
	case sharedtestvalues.AreAPICredentialsSet(b) && err != nil:
		assert.NoError(t, err, "GetAvailableTransferChains with API credentials should not error")
	case !sharedtestvalues.AreAPICredentialsSet(b) && err == nil && !mockTests:
		assert.Error(t, err, "GetAvailableTransferChains without API credentials in live test should error")
	case mockTests && err != nil:
		assert.NoError(t, err, "GetAvailableTransferChains in mock test should not error")
	}
}

func TestSeedLocalCache(t *testing.T) {
	t.Parallel()
	err := b.SeedLocalCache(t.Context(), currency.NewBTCUSDT())
	require.NoError(t, err, "SeedLocalCache must not error")
}

func TestGenerateSubscriptions(t *testing.T) {
	t.Parallel()
	exp := subscription.List{}
	pairs, err := b.GetEnabledPairs(asset.Spot)
	require.NoError(t, err, "GetEnabledPairs must not error")
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
	subs, err := b.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")
	testsubs.EqualLists(t, exp, subs)
}

// TestFormatChannelInterval exercises formatChannelInterval
func TestFormatChannelInterval(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "@1000ms", formatChannelInterval(&subscription.Subscription{Channel: subscription.OrderbookChannel, Interval: kline.ThousandMilliseconds}), "formatChannelInterval for 1s Orderbook should be @1000ms")
	assert.Equal(t, "@1m", formatChannelInterval(&subscription.Subscription{Channel: subscription.OrderbookChannel, Interval: kline.OneMin}), "formatChannelInterval for 1m Orderbook should be @1m")
	assert.Equal(t, "_15m", formatChannelInterval(&subscription.Subscription{Channel: subscription.CandlesChannel, Interval: kline.FifteenMin}), "formatChannelInterval for 15m Candles should be _15m")
}

// TestFormatChannelLevels exercises formatChannelLevels
func TestFormatChannelLevels(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "10", formatChannelLevels(&subscription.Subscription{Channel: subscription.OrderbookChannel, Levels: 10}), "formatChannelLevels for 10 levels should be 10")
	assert.Empty(t, formatChannelLevels(&subscription.Subscription{Channel: subscription.OrderbookChannel, Levels: 0}), "formatChannelLevels for 0 levels should be empty")
}

var websocketDepthUpdate = []byte(`{"E":1608001030784,"U":7145637266,"a":[["19455.19000000","0.59490200"],["19455.37000000","0.00000000"],["19456.11000000","0.00000000"],["19456.16000000","0.00000000"],["19458.67000000","0.06400000"],["19460.73000000","0.05139800"],["19461.43000000","0.00000000"],["19464.59000000","0.00000000"],["19466.03000000","0.45000000"],["19466.36000000","0.00000000"],["19508.67000000","0.00000000"],["19572.96000000","0.00217200"],["24386.00000000","0.00256600"]],"b":[["19455.18000000","2.94649200"],["19453.15000000","0.01233600"],["19451.18000000","0.00000000"],["19446.85000000","0.11427900"],["19446.74000000","0.00000000"],["19446.73000000","0.00000000"],["19444.45000000","0.14937800"],["19426.75000000","0.00000000"],["19416.36000000","0.36052100"]],"e":"depthUpdate","s":"BTCUSDT","u":7145637297}`)

func TestProcessOrderbookUpdate(t *testing.T) {
	t.Parallel()
	b := new(Binance) //nolint:govet // Intentional shadow to avoid future copy/paste mistakes
	require.NoError(t, testexch.Setup(b), "Test instance Setup must not error")
	b.setupOrderbookManager()
	p := currency.NewBTCUSDT()
	var depth WebsocketDepthStream
	err := json.Unmarshal(websocketDepthUpdate, &depth)
	require.NoError(t, err, "Unmarshal websocketDepthUpdate must not error")

	err = b.obm.stageWsUpdate(&depth, p, asset.Spot)
	require.NoError(t, err, "stageWsUpdate must not error")

	err = b.obm.fetchBookViaREST(p)
	require.NoError(t, err, "fetchBookViaREST must not error")

	err = b.obm.cleanup(p)
	require.NoError(t, err, "cleanup must not error")

	// reset order book sync status
	b.obm.state[currency.BTC][currency.USDT][asset.Spot].lastUpdateID = 0
}

func TestUFuturesHistoricalTrades(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	cp, err := currency.NewPairFromString("BTCUSDT")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSDT")

	_, err = b.UFuturesHistoricalTrades(t.Context(), cp, "", 5)
	assert.NoError(t, err, "UFuturesHistoricalTrades should not error")

	_, err = b.UFuturesHistoricalTrades(t.Context(), cp, "", 0)
	assert.NoError(t, err, "UFuturesHistoricalTrades with limit 0 should not error")
}

func TestSetExchangeOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	err := b.UpdateOrderExecutionLimits(t.Context(), asset.Spot)
	require.NoError(t, err, "UpdateOrderExecutionLimits for spot must not error")

	err = b.UpdateOrderExecutionLimits(t.Context(), asset.CoinMarginedFutures)
	require.NoError(t, err, "UpdateOrderExecutionLimits for coin margined futures must not error")

	err = b.UpdateOrderExecutionLimits(t.Context(), asset.USDTMarginedFutures)
	require.NoError(t, err, "UpdateOrderExecutionLimits for USDT margined futures must not error")

	err = b.UpdateOrderExecutionLimits(t.Context(), asset.Binary)
	require.Error(t, err, "UpdateOrderExecutionLimits for binary asset type must error")

	cmfCP, err := currency.NewPairFromStrings("BTCUSD", "PERP")
	require.NoError(t, err, "NewPairFromStrings for BTCUSD_PERP must not error")

	limit, err := b.GetOrderExecutionLimits(asset.CoinMarginedFutures, cmfCP)
	require.NoError(t, err, "GetOrderExecutionLimits for coin margined futures must not error")
	require.NotEqual(t, order.MinMaxLevel{}, limit, "GetOrderExecutionLimits for coin margined futures must return a valid limit structure")

	err = limit.Conforms(0.000001, 0.1, order.Limit)
	assert.ErrorIs(t, err, order.ErrAmountBelowMin, "Conforms should return ErrAmountBelowMin for too small amount")

	err = limit.Conforms(0.01, 1, order.Limit)
	assert.ErrorIs(t, err, order.ErrPriceBelowMin, "Conforms should return ErrPriceBelowMin for too low price")
}

func TestWsOrderExecutionReport(t *testing.T) {
	t.Parallel()
	b := new(Binance) //nolint:govet // Intentional shadow to avoid future copy/paste mistakes
	require.NoError(t, testexch.Setup(b), "Test instance Setup must not error")
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
	for len(b.Websocket.DataHandler) > 0 {
		<-b.Websocket.DataHandler
	}

	err := b.wsHandleData(payload)
	require.NoError(t, err, "wsHandleData for execution report (NEW) must not error")

	res := <-b.Websocket.DataHandler
	r, ok := res.(*order.Detail)
	require.True(t, ok, "Processed data should be of type *order.Detail")
	assert.Equal(t, expectedResult, *r, "Processed order detail should match expected result for NEW order")

	payload = []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"executionReport","E":1616633041556,"s":"BTCUSDT","c":"YeULctvPAnHj5HXCQo9Mob","S":"BUY","o":"LIMIT","f":"GTC","q":"0.00028600","p":"52436.85000000","P":"0.00000000","F":"0.00000000","g":-1,"C":"","x":"TRADE","X":"FILLED","r":"NONE","i":5341783271,"l":"0.00028600","z":"0.00028600","L":"52436.85000000","n":"0.00000029","N":"BTC","T":1616633041555,"t":726946523,"I":11390206312,"w":false,"m":false,"M":true,"O":1616633041555,"Z":"14.99693910","Y":"14.99693910","Q":"0.00000000","W":1616633041555}}`)
	err = b.wsHandleData(payload)
	assert.NoError(t, err, "wsHandleData for execution report (FILLED) should not error")
}

func TestWsOutboundAccountPosition(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"outboundAccountPosition","E":1616628815745,"u":1616628815745,"B":[{"a":"BTC","f":"0.00225109","l":"0.00123000"},{"a":"BNB","f":"0.00000000","l":"0.00000000"},{"a":"USDT","f":"54.43390661","l":"0.00000000"}]}}`)
	err := b.wsHandleData(payload)
	assert.NoError(t, err, "wsHandleData for outbound account position should not error")
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
			result, err := b.FormatExchangeCurrency(tt.pair, tt.asset)
			require.NoError(t, err, "FormatExchangeCurrency must not error")
			assert.Equalf(t, tt.expectedDelimiter, result.Delimiter, "Delimiter should be %s for %s", tt.expectedDelimiter, tt.name)
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
			result, err := b.FormatSymbol(tt.pair, tt.asset)
			require.NoError(t, err, "FormatSymbol must not error")
			assert.Equalf(t, tt.expectedString, result, "Symbol string should be %s for %s", tt.expectedString, tt.name)
		})
	}
}

func TestFormatUSDTMarginedFuturesPair(t *testing.T) {
	t.Parallel()
	pairFormat := currency.PairFormat{Uppercase: true}
	resp := b.formatUSDTMarginedFuturesPair(currency.NewPair(currency.DOGE, currency.USDT), pairFormat)
	assert.Equal(t, "DOGEUSDT", resp.String(), "formatUSDTMarginedFuturesPair for DOGE/USDT should be DOGEUSDT")

	resp = b.formatUSDTMarginedFuturesPair(currency.NewPair(currency.DOGE, currency.NewCode("1234567890")), pairFormat)
	assert.Equal(t, "DOGE_1234567890", resp.String(), "formatUSDTMarginedFuturesPair for DOGE/1234567890 should be DOGE_1234567890")
}

func TestFetchExchangeLimits(t *testing.T) {
	t.Parallel()
	limits, err := b.FetchExchangeLimits(t.Context(), asset.Spot)
	assert.NoError(t, err, "FetchExchangeLimits for spot should not error")
	assert.NotEmpty(t, limits, "FetchExchangeLimits for spot should return some limits")

	limits, err = b.FetchExchangeLimits(t.Context(), asset.Margin)
	assert.NoError(t, err, "FetchExchangeLimits for margin should not error")
	assert.NotEmpty(t, limits, "FetchExchangeLimits for margin should return some limits")

	_, err = b.FetchExchangeLimits(t.Context(), asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "FetchExchangeLimits for futures should return ErrNotSupported")
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()

	tests := map[asset.Item]currency.Pair{
		asset.Spot:   currency.NewBTCUSDT(),
		asset.Margin: currency.NewPair(currency.ETH, currency.BTC),
	}
	for _, a := range []asset.Item{asset.CoinMarginedFutures, asset.USDTMarginedFutures} {
		pairs, err := b.FetchTradablePairs(t.Context(), a)
		require.NoErrorf(t, err, "FetchTradablePairs must not error for %s", a)
		require.NotEmptyf(t, pairs, "Must get some pairs for %s", a)
		tests[a] = pairs[0]
	}

	for _, a := range b.GetAssetTypes(false) {
		t.Run(a.String(), func(t *testing.T) {
			t.Parallel()
			err := b.UpdateOrderExecutionLimits(t.Context(), a)
			require.NoError(t, err, "UpdateOrderExecutionLimits must not error")

			p := tests[a]
			limits, err := b.GetOrderExecutionLimits(a, p)
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
		})
	}
}

func TestGetHistoricalFundingRates(t *testing.T) {
	t.Parallel()
	s, e := getTime()
	_, err := b.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 currency.NewBTCUSDT(),
		StartDate:            s,
		EndDate:              e,
		IncludePayments:      true,
		IncludePredictedRate: true,
	})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricalFundingRates with IncludePayments and IncludePredictedRate should return ErrFunctionNotSupported")

	_, err = b.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.USDTMarginedFutures,
		Pair:            currency.NewBTCUSDT(),
		StartDate:       s,
		EndDate:         e,
		PaymentCurrency: currency.DOGE,
	})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricalFundingRates with PaymentCurrency should return ErrFunctionNotSupported")

	r := &fundingrate.HistoricalRatesRequest{
		Asset:     asset.USDTMarginedFutures,
		Pair:      currency.NewBTCUSDT(),
		StartDate: s,
		EndDate:   e,
	}
	if sharedtestvalues.AreAPICredentialsSet(b) {
		r.IncludePayments = true
	}
	_, err = b.GetHistoricalFundingRates(t.Context(), r)
	assert.NoError(t, err, "GetHistoricalFundingRates for USDTMarginedFutures should not error")

	r.Asset = asset.CoinMarginedFutures
	r.Pair, err = currency.NewPairFromString("BTCUSD_PERP")
	require.NoError(t, err, "NewPairFromString for BTCUSD_PERP must not error")
	_, err = b.GetHistoricalFundingRates(t.Context(), r)
	assert.NoError(t, err, "GetHistoricalFundingRates for CoinMarginedFutures should not error")
}

func TestGetLatestFundingRates(t *testing.T) {
	t.Parallel()
	cp := currency.NewBTCUSDT()
	_, err := b.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 cp,
		IncludePredictedRate: true,
	})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetLatestFundingRates with IncludePredictedRate should return ErrFunctionNotSupported")

	err = b.CurrencyPairs.EnablePair(asset.USDTMarginedFutures, cp)
	require.Truef(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled),
		"EnablePair for asset %s and pair %s must not error: %s", asset.USDTMarginedFutures, cp, err)

	_, err = b.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  cp,
	})
	assert.NoError(t, err, "GetLatestFundingRates should not error for USDTMarginedFutures")
	_, err = b.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.CoinMarginedFutures,
	})
	assert.NoError(t, err, "GetLatestFundingRates should not error for CoinMarginedFutures")
}

func TestIsPerpetualFutureCurrency(t *testing.T) {
	t.Parallel()
	is, err := b.IsPerpetualFutureCurrency(asset.Binary, currency.NewBTCUSDT())
	require.NoError(t, err, "IsPerpetualFutureCurrency for Binary must not error")
	assert.False(t, is, "IsPerpetualFutureCurrency for Binary should be false")

	is, err = b.IsPerpetualFutureCurrency(asset.CoinMarginedFutures, currency.NewBTCUSDT())
	require.NoError(t, err, "IsPerpetualFutureCurrency for CoinMarginedFutures with non-PERP pair must not error")
	assert.False(t, is, "IsPerpetualFutureCurrency for CoinMarginedFutures with non-PERP pair should be false")

	is, err = b.IsPerpetualFutureCurrency(asset.CoinMarginedFutures, currency.NewPair(currency.BTC, currency.PERP))
	require.NoError(t, err, "IsPerpetualFutureCurrency for CoinMarginedFutures with PERP pair must not error")
	assert.True(t, is, "IsPerpetualFutureCurrency for CoinMarginedFutures with PERP pair should be true")

	is, err = b.IsPerpetualFutureCurrency(asset.USDTMarginedFutures, currency.NewBTCUSDT())
	require.NoError(t, err, "IsPerpetualFutureCurrency for USDTMarginedFutures with base/quote pair must not error")
	assert.True(t, is, "IsPerpetualFutureCurrency for USDTMarginedFutures with base/quote pair should be true")

	is, err = b.IsPerpetualFutureCurrency(asset.USDTMarginedFutures, currency.NewPair(currency.BTC, currency.PERP))
	require.NoError(t, err, "IsPerpetualFutureCurrency for USDTMarginedFutures with PERP pair must not error")
	assert.False(t, is, "IsPerpetualFutureCurrency for USDTMarginedFutures with PERP pair should be false")
}

func TestGetUserMarginInterestHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetUserMarginInterestHistory(t.Context(), currency.USDT, currency.NewBTCUSDT(), time.Now().Add(-time.Hour*24), time.Now(), 1, 10, false)
	assert.NoError(t, err, "GetUserMarginInterestHistory should not error")
}

func TestSetAssetsMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	is, err := b.GetAssetsMode(t.Context())
	require.NoError(t, err, "GetAssetsMode must not error")

	err = b.SetAssetsMode(t.Context(), !is)
	assert.NoErrorf(t, err, "SetAssetsMode to %v should not error", !is)

	err = b.SetAssetsMode(t.Context(), is)
	assert.NoErrorf(t, err, "SetAssetsMode back to %v should not error", is)
}

func TestGetAssetsMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetAssetsMode(t.Context())
	assert.NoError(t, err, "GetAssetsMode should not error")
}

func TestGetCollateralMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.GetCollateralMode(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetCollateralMode for spot must return ErrNotSupported")

	_, err = b.GetCollateralMode(t.Context(), asset.CoinMarginedFutures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetCollateralMode for coin margined futures must return ErrNotSupported")

	_, err = b.GetCollateralMode(t.Context(), asset.USDTMarginedFutures)
	assert.NoError(t, err, "GetCollateralMode for USDT margined futures should not error")
}

func TestSetCollateralMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	err := b.SetCollateralMode(t.Context(), asset.Spot, collateral.SingleMode)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "SetCollateralMode for spot must return ErrNotSupported")

	err = b.SetCollateralMode(t.Context(), asset.CoinMarginedFutures, collateral.SingleMode)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "SetCollateralMode for coin margined futures must return ErrNotSupported")

	err = b.SetCollateralMode(t.Context(), asset.USDTMarginedFutures, collateral.MultiMode)
	assert.NoError(t, err, "SetCollateralMode to MultiMode for USDT margined futures should not error")

	err = b.SetCollateralMode(t.Context(), asset.USDTMarginedFutures, collateral.PortfolioMode)
	assert.ErrorIs(t, err, order.ErrCollateralInvalid, "SetCollateralMode to PortfolioMode for USDT margined futures must return ErrCollateralInvalid")
}

func TestChangePositionMargin(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err := b.ChangePositionMargin(t.Context(), &margin.PositionChangeRequest{
		Pair:                    currency.NewBTCUSDT(),
		Asset:                   asset.USDTMarginedFutures,
		MarginType:              margin.Isolated,
		OriginalAllocatedMargin: 1337,
		NewAllocatedMargin:      1333337,
	})
	assert.NoError(t, err, "ChangePositionMargin should not error")
}

func TestGetPositionSummary(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)

	bb := currency.NewBTCUSDT()
	_, err := b.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  bb,
	})
	assert.NoError(t, err, "GetFuturesPositionSummary for USDTMarginedFutures (USDT pair) should not error")

	bb.Quote = currency.BUSD
	_, err = b.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  bb,
	})
	assert.NoError(t, err, "GetFuturesPositionSummary for USDTMarginedFutures (BUSD pair) should not error")

	p, err := currency.NewPairFromString("BTCUSD_PERP")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSD_PERP")
	bb.Quote = currency.USD
	_, err = b.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset:          asset.CoinMarginedFutures,
		Pair:           p,
		UnderlyingPair: bb,
	})
	assert.NoError(t, err, "GetFuturesPositionSummary for CoinMarginedFutures should not error")

	_, err = b.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset:          asset.Spot,
		Pair:           p,
		UnderlyingPair: bb,
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetFuturesPositionSummary for Spot must return ErrNotSupported")
}

func TestGetFuturesPositionOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetFuturesPositionOrders(t.Context(), &futures.PositionsRequest{
		Asset:                     asset.USDTMarginedFutures,
		Pairs:                     []currency.Pair{currency.NewBTCUSDT()},
		StartDate:                 time.Now().Add(-time.Hour * 24 * 70),
		RespectOrderHistoryLimits: true,
	})
	assert.NoError(t, err, "GetFuturesPositionOrders for USDTMarginedFutures should not error")

	p, err := currency.NewPairFromString("ADAUSD_PERP")
	require.NoError(t, err, "NewPairFromString must not error for ADAUSD_PERP")
	_, err = b.GetFuturesPositionOrders(t.Context(), &futures.PositionsRequest{
		Asset:                     asset.CoinMarginedFutures,
		Pairs:                     []currency.Pair{p},
		StartDate:                 time.Now().Add(time.Hour * 24 * -70),
		RespectOrderHistoryLimits: true,
	})
	assert.NoError(t, err, "GetFuturesPositionOrders for CoinMarginedFutures should not error")
}

func TestSetMarginType(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)

	err := b.SetMarginType(t.Context(), asset.USDTMarginedFutures, currency.NewBTCUSDT(), margin.Isolated)
	assert.NoError(t, err, "SetMarginType for USDTMarginedFutures should not error")

	p, err := currency.NewPairFromString("BTCUSD_PERP")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSD_PERP")
	err = b.SetMarginType(t.Context(), asset.CoinMarginedFutures, p, margin.Isolated)
	assert.NoError(t, err, "SetMarginType for CoinMarginedFutures should not error")

	err = b.SetMarginType(t.Context(), asset.Spot, currency.NewBTCUSDT(), margin.Isolated)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "SetMarginType for Spot must return ErrNotSupported")
}

func TestGetLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.GetLeverage(t.Context(), asset.USDTMarginedFutures, currency.NewBTCUSDT(), 0, order.UnknownSide)
	assert.NoError(t, err, "GetLeverage for USDTMarginedFutures should not error")

	p, err := currency.NewPairFromString("BTCUSD_PERP")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSD_PERP")
	_, err = b.GetLeverage(t.Context(), asset.CoinMarginedFutures, p, 0, order.UnknownSide)
	assert.NoError(t, err, "GetLeverage for CoinMarginedFutures should not error")

	_, err = b.GetLeverage(t.Context(), asset.Spot, currency.NewBTCUSDT(), 0, order.UnknownSide)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetLeverage for Spot must return ErrNotSupported")
}

func TestSetLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	err := b.SetLeverage(t.Context(), asset.USDTMarginedFutures, currency.NewBTCUSDT(), margin.Multi, 5, order.UnknownSide)
	assert.NoError(t, err, "SetLeverage for USDTMarginedFutures should not error")

	p, err := currency.NewPairFromString("BTCUSD_PERP")
	require.NoError(t, err, "NewPairFromString must not error for BTCUSD_PERP")
	err = b.SetLeverage(t.Context(), asset.CoinMarginedFutures, p, margin.Multi, 5, order.UnknownSide)
	assert.NoError(t, err, "SetLeverage for CoinMarginedFutures should not error")

	err = b.SetLeverage(t.Context(), asset.Spot, p, margin.Multi, 5, order.UnknownSide)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "SetLeverage for Spot must return ErrNotSupported")
}

func TestGetCryptoLoansIncomeHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.CryptoLoanIncomeHistory(t.Context(), currency.USDT, "", time.Time{}, time.Time{}, 100)
	assert.NoError(t, err, "CryptoLoanIncomeHistory should not error")
}

func TestCryptoLoanBorrow(t *testing.T) {
	t.Parallel()
	_, err := b.CryptoLoanBorrow(t.Context(), currency.EMPTYCODE, 1000, currency.BTC, 1, 7)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet, "CryptoLoanBorrow with empty loan coin should return errLoanCoinMustBeSet")
	_, err = b.CryptoLoanBorrow(t.Context(), currency.USDT, 1000, currency.EMPTYCODE, 1, 7)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet, "CryptoLoanBorrow with empty collateral coin should return errCollateralCoinMustBeSet")
	_, err = b.CryptoLoanBorrow(t.Context(), currency.USDT, 0, currency.BTC, 1, 0)
	assert.ErrorIs(t, err, errLoanTermMustBeSet, "CryptoLoanBorrow with zero loan term should return errLoanTermMustBeSet")
	_, err = b.CryptoLoanBorrow(t.Context(), currency.USDT, 0, currency.BTC, 0, 7)
	assert.ErrorIs(t, err, errEitherLoanOrCollateralAmountsMustBeSet, "CryptoLoanBorrow with zero loan and collateral amounts should return errEitherLoanOrCollateralAmountsMustBeSet")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err = b.CryptoLoanBorrow(t.Context(), currency.USDT, 1000, currency.BTC, 1, 7)
	assert.NoError(t, err, "CryptoLoanBorrow with valid parameters should not error")
}

func TestCryptoLoanBorrowHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.CryptoLoanBorrowHistory(t.Context(), 0, currency.USDT, currency.BTC, time.Time{}, time.Time{}, 0, 0)
	assert.NoError(t, err, "CryptoLoanBorrowHistory should not error")
}

func TestCryptoLoanOngoingOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.CryptoLoanOngoingOrders(t.Context(), 0, currency.USDT, currency.BTC, 0, 0)
	assert.NoError(t, err, "CryptoLoanOngoingOrders should not error")
}

func TestCryptoLoanRepay(t *testing.T) {
	t.Parallel()
	_, err := b.CryptoLoanRepay(t.Context(), 0, 1000, 1, false)
	assert.ErrorIs(t, err, errOrderIDMustBeSet, "CryptoLoanRepay with zero orderID should return errOrderIDMustBeSet")
	_, err = b.CryptoLoanRepay(t.Context(), 42069, 0, 1, false)
	assert.ErrorIs(t, err, errAmountMustBeSet, "CryptoLoanRepay with zero amount should return errAmountMustBeSet")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err = b.CryptoLoanRepay(t.Context(), 42069, 1000, 1, false)
	assert.NoError(t, err, "CryptoLoanRepay with valid parameters should not error")
}

func TestCryptoLoanRepaymentHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.CryptoLoanRepaymentHistory(t.Context(), 0, currency.USDT, currency.BTC, time.Time{}, time.Time{}, 0, 0)
	assert.NoError(t, err, "CryptoLoanRepaymentHistory should not error")
}

func TestCryptoLoanAdjustLTV(t *testing.T) {
	t.Parallel()
	_, err := b.CryptoLoanAdjustLTV(t.Context(), 0, true, 1)
	assert.ErrorIs(t, err, errOrderIDMustBeSet, "CryptoLoanAdjustLTV with zero orderID should return errOrderIDMustBeSet")
	_, err = b.CryptoLoanAdjustLTV(t.Context(), 42069, true, 0)
	assert.ErrorIs(t, err, errAmountMustBeSet, "CryptoLoanAdjustLTV with zero amount should return errAmountMustBeSet")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err = b.CryptoLoanAdjustLTV(t.Context(), 42069, true, 1)
	assert.NoError(t, err, "CryptoLoanAdjustLTV with valid parameters should not error")
}

func TestCryptoLoanLTVAdjustmentHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.CryptoLoanLTVAdjustmentHistory(t.Context(), 0, currency.USDT, currency.BTC, time.Time{}, time.Time{}, 0, 0)
	assert.NoError(t, err, "CryptoLoanLTVAdjustmentHistory should not error")
}

func TestCryptoLoanAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.CryptoLoanAssetsData(t.Context(), currency.EMPTYCODE, 0)
	assert.NoError(t, err, "CryptoLoanAssetsData should not error")
}

func TestCryptoLoanCollateralAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.CryptoLoanCollateralAssetsData(t.Context(), currency.EMPTYCODE, 0)
	assert.NoError(t, err, "CryptoLoanCollateralAssetsData should not error")
}

func TestCryptoLoanCheckCollateralRepayRate(t *testing.T) {
	t.Parallel()
	_, err := b.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.EMPTYCODE, currency.BNB, 69)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet, "CryptoLoanCheckCollateralRepayRate with empty loan coin should return errLoanCoinMustBeSet")
	_, err = b.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.EMPTYCODE, 69)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet, "CryptoLoanCheckCollateralRepayRate with empty collateral coin should return errCollateralCoinMustBeSet")
	_, err = b.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.BNB, 0)
	assert.ErrorIs(t, err, errAmountMustBeSet, "CryptoLoanCheckCollateralRepayRate with zero amount should return errAmountMustBeSet")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err = b.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.BNB, 69)
	assert.NoError(t, err, "CryptoLoanCheckCollateralRepayRate with valid parameters should not error")
}

func TestCryptoLoanCustomiseMarginCall(t *testing.T) {
	t.Parallel()
	_, err := b.CryptoLoanCustomiseMarginCall(t.Context(), 0, currency.BTC, 0)
	assert.Error(t, err, "CryptoLoanCustomiseMarginCall with invalid orderID should error")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err = b.CryptoLoanCustomiseMarginCall(t.Context(), 1337, currency.BTC, .70)
	assert.NoError(t, err, "CryptoLoanCustomiseMarginCall with valid parameters should not error")
}

func TestFlexibleLoanBorrow(t *testing.T) {
	t.Parallel()
	_, err := b.FlexibleLoanBorrow(t.Context(), currency.EMPTYCODE, currency.USDC, 1, 0)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet, "FlexibleLoanBorrow with empty loan coin should return errLoanCoinMustBeSet")
	_, err = b.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.EMPTYCODE, 1, 0)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet, "FlexibleLoanBorrow with empty collateral coin should return errCollateralCoinMustBeSet")
	_, err = b.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.USDC, 0, 0)
	assert.ErrorIs(t, err, errEitherLoanOrCollateralAmountsMustBeSet, "FlexibleLoanBorrow with zero loan and collateral amounts should return errEitherLoanOrCollateralAmountsMustBeSet")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err = b.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.USDC, 1, 0)
	assert.NoError(t, err, "FlexibleLoanBorrow with valid parameters should not error")
}

func TestFlexibleLoanOngoingOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FlexibleLoanOngoingOrders(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, 0, 0)
	assert.NoError(t, err, "FlexibleLoanOngoingOrders should not error")
}

func TestFlexibleLoanBorrowHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FlexibleLoanBorrowHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, time.Time{}, time.Time{}, 0, 0)
	assert.NoError(t, err, "FlexibleLoanBorrowHistory should not error")
}

func TestFlexibleLoanRepay(t *testing.T) {
	t.Parallel()
	_, err := b.FlexibleLoanRepay(t.Context(), currency.EMPTYCODE, currency.BTC, 1, false, false)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet, "FlexibleLoanRepay with empty loan coin should return errLoanCoinMustBeSet")
	_, err = b.FlexibleLoanRepay(t.Context(), currency.USDT, currency.EMPTYCODE, 1, false, false)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet, "FlexibleLoanRepay with empty collateral coin should return errCollateralCoinMustBeSet")
	_, err = b.FlexibleLoanRepay(t.Context(), currency.USDT, currency.BTC, 0, false, false)
	assert.ErrorIs(t, err, errAmountMustBeSet, "FlexibleLoanRepay with zero amount should return errAmountMustBeSet")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err = b.FlexibleLoanRepay(t.Context(), currency.ATOM, currency.USDC, 1, false, false)
	assert.NoError(t, err, "FlexibleLoanRepay with valid parameters should not error")
}

func TestFlexibleLoanRepayHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FlexibleLoanRepayHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, time.Time{}, time.Time{}, 0, 0)
	assert.NoError(t, err, "FlexibleLoanRepayHistory should not error")
}

func TestFlexibleLoanAdjustLTV(t *testing.T) {
	t.Parallel()
	_, err := b.FlexibleLoanAdjustLTV(t.Context(), currency.EMPTYCODE, currency.BTC, 1, true)
	assert.ErrorIs(t, err, errLoanCoinMustBeSet, "FlexibleLoanAdjustLTV with empty loan coin should return errLoanCoinMustBeSet")
	_, err = b.FlexibleLoanAdjustLTV(t.Context(), currency.USDT, currency.EMPTYCODE, 1, true)
	assert.ErrorIs(t, err, errCollateralCoinMustBeSet, "FlexibleLoanAdjustLTV with empty collateral coin should return errCollateralCoinMustBeSet")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, b, canManipulateRealOrders)
	_, err = b.FlexibleLoanAdjustLTV(t.Context(), currency.USDT, currency.BTC, 1, true)
	assert.NoError(t, err, "FlexibleLoanAdjustLTV with valid parameters should not error")
}

func TestFlexibleLoanLTVAdjustmentHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FlexibleLoanLTVAdjustmentHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, time.Time{}, time.Time{}, 0, 0)
	assert.NoError(t, err, "FlexibleLoanLTVAdjustmentHistory should not error")
}

func TestFlexibleLoanAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FlexibleLoanAssetsData(t.Context(), currency.EMPTYCODE)
	assert.NoError(t, err, "FlexibleLoanAssetsData should not error")
}

func TestFlexibleCollateralAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, b)
	_, err := b.FlexibleCollateralAssetsData(t.Context(), currency.EMPTYCODE)
	assert.NoError(t, err, "FlexibleCollateralAssetsData should not error")
}

func TestGetFuturesContractDetails(t *testing.T) {
	t.Parallel()
	_, err := b.GetFuturesContractDetails(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, futures.ErrNotFuturesAsset, "GetFuturesContractDetails for Spot must return ErrNotFuturesAsset")

	_, err = b.GetFuturesContractDetails(t.Context(), asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetFuturesContractDetails for Futures must return ErrNotSupported")

	_, err = b.GetFuturesContractDetails(t.Context(), asset.USDTMarginedFutures)
	assert.NoError(t, err, "GetFuturesContractDetails for USDTMarginedFutures should not error")

	_, err = b.GetFuturesContractDetails(t.Context(), asset.CoinMarginedFutures)
	assert.NoError(t, err, "GetFuturesContractDetails for CoinMarginedFutures should not error")
}

func TestGetFundingRateInfo(t *testing.T) {
	t.Parallel()
	_, err := b.GetFundingRateInfo(t.Context())
	assert.NoError(t, err, "GetFundingRateInfo should not error")
}

func TestUGetFundingRateInfo(t *testing.T) {
	t.Parallel()
	_, err := b.UGetFundingRateInfo(t.Context())
	assert.NoError(t, err, "UGetFundingRateInfo should not error")
}

func TestGetOpenInterest(t *testing.T) {
	t.Parallel()
	resp, err := b.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.BTC.Item,
		Quote: currency.USDT.Item,
		Asset: asset.USDTMarginedFutures,
	})
	assert.NoError(t, err, "GetOpenInterest for USDTMarginedFutures should not error")
	assert.NotEmpty(t, resp, "GetOpenInterest for USDTMarginedFutures should return data")

	resp, err = b.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.NewCode("BTCUSD").Item,
		Quote: currency.PERP.Item,
		Asset: asset.CoinMarginedFutures,
	})
	assert.NoError(t, err, "GetOpenInterest for CoinMarginedFutures should not error")
	assert.NotEmpty(t, resp, "GetOpenInterest for CoinMarginedFutures should return data")

	_, err = b.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.BTC.Item,
		Quote: currency.USDT.Item,
		Asset: asset.Spot,
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetOpenInterest for Spot must return ErrNotSupported")
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, b)
	for _, a := range b.GetAssetTypes(false) {
		t.Run(a.String(), func(t *testing.T) {
			t.Parallel()
			pairs, err := b.CurrencyPairs.GetPairs(a, false)
			require.NoErrorf(t, err, "cannot get pairs for %s", a)
			require.NotEmptyf(t, pairs, "no pairs for %s", a)
			resp, err := b.GetCurrencyTradeURL(t.Context(), a, pairs[0])
			require.NoError(t, err, "GetCurrencyTradeURL must not error")
			assert.NotEmpty(t, resp, "GetCurrencyTradeURL should return a URL")
		})
	}
}
