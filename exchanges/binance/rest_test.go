package binance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
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
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/collateral"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/futures"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

const (
	canManipulateRealOrders     = false
	canManipulateAPICredentials = false
	useTestNet                  = false

	apiStreamingIsNotConnected = "API streaming is not connected"
)

var (
	// Please supply your own credentials here for due diligence testing
	apiCredentials = &accounts.Credentials{
		Key:    "",
		Secret: "",
	}

	e *Exchange

	// enabled and active tradable pairs used to test endpoints.
	spotTradablePair, marginTradablePair, usdtmTradablePair, coinmTradablePair, optionsTradablePair currency.Pair

	assetToTradablePairMap map[asset.Item]currency.Pair
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
func getTime(expanded ...bool) (startTime, endTime time.Time) {
	if len(expanded) > 0 && mockTests {
		return time.UnixMilli(1744103851944), time.UnixMilli(1744190254944)
	} else if mockTests {
		return time.UnixMilli(1744103854944), time.UnixMilli(1744190254944)
	}
	tn := time.Now()
	offset := time.Hour * 24 * 6
	return tn.Add(-offset), tn
}

func TestUServerTime(t *testing.T) {
	t.Parallel()
	result, err := e.UServerTime(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWrapperGetServerTime(t *testing.T) {
	t.Parallel()
	_, err := e.GetServerTime(t.Context(), asset.Empty)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	assetTypes := e.GetAssetTypes(true)
	for _, a := range assetTypes {
		st, err := e.GetServerTime(t.Context(), a)
		require.NoError(t, err)
		assert.NotEmpty(t, st)
	}
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	for assetType, pair := range assetToTradablePairMap {
		r, err := e.UpdateTicker(t.Context(), pair, assetType)
		require.NoErrorf(t, err, "expected nil, got %v for asset type: %s pair: %v", err, assetType, pair)
		assert.NotNilf(t, r, "unexpected value nil for asset type: %s pair: %v", assetType, pair)
	}
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	enabledAssets := e.GetAssetTypes(true)
	for _, assetType := range enabledAssets {
		err := e.UpdateTickers(t.Context(), assetType)
		assert.NoError(t, err)
	}
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	for assetType, tp := range assetToTradablePairMap {
		result, err := e.UpdateOrderbook(t.Context(), tp, assetType)
		require.NoErrorf(t, err, "%v: %v", err, assetType)
		assert.NotNil(t, result)
	}
}

func TestUExchangeInfo(t *testing.T) {
	t.Parallel()
	result, err := e.UExchangeInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesOrderbook(t.Context(), currency.EMPTYPAIR, 1000)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.UFuturesOrderbook(t.Context(), usdtmTradablePair, 1000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetURPIOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetURPIOrderbook(t.Context(), currency.EMPTYPAIR, 100)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetURPIOrderbook(t.Context(), usdtmTradablePair, 1000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestURecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.URecentTrades(t.Context(), currency.EMPTYPAIR, "", 1000)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.URecentTrades(t.Context(), usdtmTradablePair, "", 1000)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.URecentTrades(t.Context(), usdtmTradablePair, "7442186355", 1000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUCompressedTrades(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime(true)
	_, err := e.UCompressedTrades(t.Context(), &UCompressedTradesRequest{Limit: 5, StartTime: startTime, EndTime: endTime})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.UCompressedTrades(t.Context(), &UCompressedTradesRequest{Symbol: usdtmTradablePair, Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UCompressedTrades(t.Context(), &UCompressedTradesRequest{Symbol: usdtmTradablePair, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	_, err = e.UCompressedTrades(t.Context(), &UCompressedTradesRequest{Symbol: usdtmTradablePair, FromID: "7442186355", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestUKlineData(t *testing.T) {
	t.Parallel()
	_, err := e.UKlineData(t.Context(), &UKlineDataRequest{Interval: "1d", Limit: 5})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UKlineData(t.Context(), &UKlineDataRequest{Symbol: usdtmTradablePair, Limit: 5})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval)

	_, err = e.UKlineData(t.Context(), &UKlineDataRequest{Symbol: usdtmTradablePair, Limit: 5})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval)

	startTime, endTime := getTime()
	_, err = e.UKlineData(t.Context(), &UKlineDataRequest{Symbol: usdtmTradablePair, Interval: "5m", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.UKlineData(t.Context(), &UKlineDataRequest{Symbol: usdtmTradablePair, Interval: "1d", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UKlineData(t.Context(), &UKlineDataRequest{Symbol: usdtmTradablePair, Interval: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUFuturesContinuousKlineData(t *testing.T) {
	t.Parallel()
	_, err := e.GetUFuturesContinuousKlineData(t.Context(), &GetUFuturesContinuousKlineDataRequest{ContractType: "CURRENT_QUARTER", Interval: "1d", Limit: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetUFuturesContinuousKlineData(t.Context(), &GetUFuturesContinuousKlineDataRequest{Pair: usdtmTradablePair, Interval: "1d", Limit: 10})
	require.ErrorIs(t, err, errContractTypeIsRequired)

	_, err = e.GetUFuturesContinuousKlineData(t.Context(), &GetUFuturesContinuousKlineDataRequest{Pair: usdtmTradablePair, ContractType: "CURRENT_QUARTER", Limit: 10})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval)

	_, err = e.GetUFuturesContinuousKlineData(t.Context(), &GetUFuturesContinuousKlineDataRequest{Pair: usdtmTradablePair, ContractType: "CURRENT_QUARTER", Limit: 10})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval)

	startTime, endTime := getTime()
	_, err = e.GetUFuturesContinuousKlineData(t.Context(), &GetUFuturesContinuousKlineDataRequest{Pair: usdtmTradablePair, ContractType: "CURRENT_QUARTER", Interval: "5m", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.GetUFuturesContinuousKlineData(t.Context(), &GetUFuturesContinuousKlineDataRequest{Pair: usdtmTradablePair, ContractType: "PERPETUAL", Interval: "1d", StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIndexOrCandlesticPriceKlineData(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexPriceKlineData(t.Context(), &GetIndexPriceKlineDataRequest{Interval: "1d", EndTime: time.Now()})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetIndexPriceKlineData(t.Context(), &GetIndexPriceKlineDataRequest{Pair: usdtmTradablePair, EndTime: time.Now()})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval)

	startTime, endTime := getTime()
	_, err = e.GetIndexPriceKlineData(t.Context(), &GetIndexPriceKlineDataRequest{Pair: usdtmTradablePair, Interval: "1d", StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetIndexPriceKlineData(t.Context(), &GetIndexPriceKlineDataRequest{Pair: usdtmTradablePair, Interval: "1d", StartTime: startTime, EndTime: endTime, Limit: 100})
	require.NoError(t, err)
}

func TestGetMarkPriceKlineCandlesticks(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarkPriceKlineCandlesticks(t.Context(), &GetMarkPriceKlineCandlesticksRequest{Interval: "1d", Limit: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetMarkPriceKlineCandlesticks(t.Context(), &GetMarkPriceKlineCandlesticksRequest{Symbol: usdtmTradablePair, Limit: 10})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval)

	startTime, endTime := getTime()
	_, err = e.GetMarkPriceKlineCandlesticks(t.Context(), &GetMarkPriceKlineCandlesticksRequest{Symbol: usdtmTradablePair, Interval: "1d", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.GetMarkPriceKlineCandlesticks(t.Context(), &GetMarkPriceKlineCandlesticksRequest{Symbol: usdtmTradablePair, Interval: "1d", StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPremiumIndexKlineCandlesticks(t *testing.T) {
	t.Parallel()
	_, err := e.GetPremiumIndexKlineCandlesticks(t.Context(), &GetPremiumIndexKlineCandlesticksRequest{Interval: "1d", Limit: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetPremiumIndexKlineCandlesticks(t.Context(), &GetPremiumIndexKlineCandlesticksRequest{Symbol: usdtmTradablePair, Limit: 10})
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval)

	startTime, endTime := getTime()
	_, err = e.GetPremiumIndexKlineCandlesticks(t.Context(), &GetPremiumIndexKlineCandlesticksRequest{Symbol: usdtmTradablePair, Interval: "1d", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.GetPremiumIndexKlineCandlesticks(t.Context(), &GetPremiumIndexKlineCandlesticksRequest{Symbol: usdtmTradablePair, Interval: "1d", StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUGetMarkPrice(t *testing.T) {
	t.Parallel()
	result, err := e.UGetMarkPrice(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UGetMarkPrice(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUGetFundingHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.UGetFundingHistory(t.Context(), usdtmTradablePair, 1000, endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.UGetFundingHistory(t.Context(), usdtmTradablePair, 1000, startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestU24HTickerPriceChangeStats(t *testing.T) {
	t.Parallel()
	result, err := e.U24HTickerPriceChangeStats(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.U24HTickerPriceChangeStats(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUSymbolPriceTickerV2(t *testing.T) {
	t.Parallel()
	result, err := e.USymbolPriceTicker(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
	result, err = e.USymbolPriceTicker(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUSymbolOrderbookTicker(t *testing.T) {
	t.Parallel()
	result, err := e.USymbolOrderbookTicker(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
	result, err = e.USymbolOrderbookTicker(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := e.UOpenInterest(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.UOpenInterest(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetQuarterlyContractSettlementPrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetQuarterlyContractSettlementPrice(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetQuarterlyContractSettlementPrice(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUOpenInterestStats(t *testing.T) {
	t.Parallel()
	_, err := e.UOpenInterestStats(t.Context(), &UOpenInterestStatsRequest{Period: "5m", Limit: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UOpenInterestStats(t.Context(), &UOpenInterestStatsRequest{Symbol: usdtmTradablePair, Limit: 1})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.UOpenInterestStats(t.Context(), &UOpenInterestStatsRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 1, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.UOpenInterestStats(t.Context(), &UOpenInterestStatsRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 1, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UOpenInterestStats(t.Context(), &UOpenInterestStatsRequest{Symbol: usdtmTradablePair, Period: "1d", Limit: 10, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUTopAccountsLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := e.UTopAccountsLongShortRatio(t.Context(), &UTopAccountsLongShortRatioRequest{Period: "5m", Limit: 2})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UTopAccountsLongShortRatio(t.Context(), &UTopAccountsLongShortRatioRequest{Symbol: usdtmTradablePair, Limit: 2})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.UTopAccountsLongShortRatio(t.Context(), &UTopAccountsLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 2, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.UTopAccountsLongShortRatio(t.Context(), &UTopAccountsLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 2, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UTopAccountsLongShortRatio(t.Context(), &UTopAccountsLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 2, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUTopPositionsLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := e.UTopPositionsLongShortRatio(t.Context(), &UTopPositionsLongShortRatioRequest{Period: "5m", Limit: 3})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UTopPositionsLongShortRatio(t.Context(), &UTopPositionsLongShortRatioRequest{Symbol: usdtmTradablePair, Limit: 3})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.UTopPositionsLongShortRatio(t.Context(), &UTopPositionsLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 3, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.UTopPositionsLongShortRatio(t.Context(), &UTopPositionsLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 3, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UTopPositionsLongShortRatio(t.Context(), &UTopPositionsLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "1d", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUGlobalLongShortRatio(t *testing.T) {
	t.Parallel()
	_, err := e.UGlobalLongShortRatio(t.Context(), &UGlobalLongShortRatioRequest{Period: "5m", Limit: 3})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UGlobalLongShortRatio(t.Context(), &UGlobalLongShortRatioRequest{Symbol: usdtmTradablePair, Limit: 3})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.UGlobalLongShortRatio(t.Context(), &UGlobalLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 3, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.UGlobalLongShortRatio(t.Context(), &UGlobalLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 3, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UGlobalLongShortRatio(t.Context(), &UGlobalLongShortRatioRequest{Symbol: usdtmTradablePair, Period: "4h", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUTakerBuySellVol(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.UTakerBuySellVol(t.Context(), &UTakerBuySellVolRequest{Limit: 10, StartTime: startTime, EndTime: endTime})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UTakerBuySellVol(t.Context(), &UTakerBuySellVolRequest{Symbol: usdtmTradablePair, Limit: 10, StartTime: startTime, EndTime: endTime})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	_, err = e.UTakerBuySellVol(t.Context(), &UTakerBuySellVolRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 10, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.UTakerBuySellVol(t.Context(), &UTakerBuySellVolRequest{Symbol: usdtmTradablePair, Period: "5m", Limit: 10, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBasis(t *testing.T) {
	t.Parallel()
	_, err := e.GetBasis(t.Context(), &GetBasisRequest{ContractType: "CURRENT_QUARTER", Period: "15m", Limit: 20})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetBasis(t.Context(), &GetBasisRequest{Pair: usdtmTradablePair, Period: "15m", Limit: 20})
	require.ErrorIs(t, err, errContractTypeIsRequired)

	_, err = e.GetBasis(t.Context(), &GetBasisRequest{Pair: usdtmTradablePair, ContractType: "PERPETUAL", Limit: 20})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.GetBasis(t.Context(), &GetBasisRequest{Pair: usdtmTradablePair, ContractType: "PERPETUAL", Period: "15m", StartTime: endTime, EndTime: startTime, Limit: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.GetBasis(t.Context(), &GetBasisRequest{Pair: usdtmTradablePair, ContractType: "PERPETUAL", Period: "15m", StartTime: startTime, EndTime: endTime, Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetBasis(t.Context(), &GetBasisRequest{Pair: usdtmTradablePair, ContractType: "PERPETUAL", Period: "15m", StartTime: startTime, EndTime: endTime, Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
	result, err = e.GetBasis(t.Context(), &GetBasisRequest{Pair: usdtmTradablePair, ContractType: "PERPETUAL", Period: "15m", StartTime: startTime, EndTime: endTime, Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUCompositeIndexesInfo(t *testing.T) {
	t.Parallel()
	result, err := e.UCompositeIndexesInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMultiAssetModeAssetIndex(t *testing.T) {
	t.Parallel()
	result, err := e.GetMultiAssetModeAssetIndex(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetMultiAssetModeAssetIndex(t.Context(), currency.NewBTCUSD())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIndexPriceConstituents(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexPriceConstituents(t.Context(), "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetIndexPriceConstituents(t.Context(), "BTCUSD")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesNewOrder(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesNewOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &UFuturesNewOrderRequest{
		ReduceOnly:   true,
		PositionSide: "position-side",
	}
	_, err = e.UFuturesNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidPositionSide)

	arg.PositionSide = "LONG"
	arg.WorkingType = "abc"
	_, err = e.UFuturesNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidWorkingType)

	arg.WorkingType = "MARK_PRICE"
	arg.NewOrderRespType = "abc"
	_, err = e.UFuturesNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidNewOrderResponseType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UFuturesNewOrder(
		t.Context(),
		&UFuturesNewOrderRequest{
			Symbol:      currency.NewBTCUSDT(),
			Side:        "BUY",
			OrderType:   order.Limit.String(),
			TimeInForce: "GTC",
			Quantity:    1,
			Price:       1,
		},
	)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUModifyOrder(t *testing.T) {
	t.Parallel()
	_, err := e.UModifyOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &USDTOrderUpdateRequest{PriceMatch: "1234"}
	_, err = e.UModifyOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	arg.OrderID = 1234
	_, err = e.UModifyOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.UModifyOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.UModifyOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.Amount = 1
	_, err = e.UModifyOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UModifyOrder(t.Context(), &USDTOrderUpdateRequest{
		OrderID:           1,
		OrigClientOrderID: "",
		Side:              order.Sell.String(),
		PriceMatch:        "TAKE_PROFIT",
		Symbol:            usdtmTradablePair,
		Amount:            0.0000001,
		Price:             123455554,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUPlaceBatchOrders(t *testing.T) {
	t.Parallel()
	_, err := e.UPlaceBatchOrders(t.Context(), []*PlaceBatchOrderData{})
	require.ErrorIs(t, err, common.ErrEmptyParams)

	arg := &PlaceBatchOrderData{
		TimeInForce:  "GTC",
		PositionSide: "abc",
	}
	_, err = e.UPlaceBatchOrders(t.Context(), []*PlaceBatchOrderData{arg})
	require.ErrorIs(t, err, errInvalidPositionSide)

	arg.PositionSide = "SHORT"
	arg.WorkingType = "abc"
	_, err = e.UPlaceBatchOrders(t.Context(), []*PlaceBatchOrderData{arg})
	require.ErrorIs(t, err, errInvalidWorkingType)

	arg.WorkingType = "CONTRACT_TYPE"
	arg.NewOrderRespType = "abc"
	_, err = e.UPlaceBatchOrders(t.Context(), []*PlaceBatchOrderData{arg})
	require.ErrorIs(t, err, errInvalidNewOrderResponseType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	tempData := &PlaceBatchOrderData{
		Symbol:      currency.Pair{Base: currency.BTC, Quote: currency.USDT},
		Side:        "BUY",
		OrderType:   order.Limit.String(),
		Quantity:    4,
		Price:       1,
		TimeInForce: "GTC",
	}
	result, err := e.UPlaceBatchOrders(t.Context(), []*PlaceBatchOrderData{tempData})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestModifyMultipleOrders(t *testing.T) {
	t.Parallel()
	_, err := e.UModifyMultipleOrders(t.Context(), []USDTOrderUpdateRequest{})
	require.ErrorIs(t, err, common.ErrEmptyParams)

	arg := USDTOrderUpdateRequest{}
	_, err = e.UModifyMultipleOrders(t.Context(), []USDTOrderUpdateRequest{arg})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	arg.OrderID = 1
	_, err = e.UModifyMultipleOrders(t.Context(), []USDTOrderUpdateRequest{arg})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = spotTradablePair
	_, err = e.UModifyMultipleOrders(t.Context(), []USDTOrderUpdateRequest{arg})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.UModifyMultipleOrders(t.Context(), []USDTOrderUpdateRequest{arg})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.Amount = 0.0001
	_, err = e.UModifyMultipleOrders(t.Context(), []USDTOrderUpdateRequest{arg})
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UModifyMultipleOrders(t.Context(), []USDTOrderUpdateRequest{
		{
			OrderID:           1,
			OrigClientOrderID: "",
			Side:              order.Sell.String(),
			PriceMatch:        "TAKE_PROFIT",
			Symbol:            spotTradablePair,
			Amount:            0.0000001,
			Price:             123455554,
		},
		{
			OrderID:           1,
			OrigClientOrderID: "",
			Side:              "BUY",
			PriceMatch:        order.Limit.String(),
			Symbol:            spotTradablePair,
			Amount:            0.0000001,
			Price:             123455554,
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUSDTOrderModifyHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetUSDTOrderModifyHistory(t.Context(), &GetUSDTOrderModifyHistoryRequest{OrderID: 1234, Limit: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetUSDTOrderModifyHistory(t.Context(), &GetUSDTOrderModifyHistoryRequest{Symbol: usdtmTradablePair, Limit: 10})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	startTime, endTime := getTime()
	_, err = e.GetUSDTOrderModifyHistory(t.Context(), &GetUSDTOrderModifyHistoryRequest{Symbol: usdtmTradablePair, OrderID: 1234, Limit: 10, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUSDTOrderModifyHistory(t.Context(), &GetUSDTOrderModifyHistoryRequest{Symbol: usdtmTradablePair, OrderID: 1234, Limit: 10, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUGetOrderData(t *testing.T) {
	t.Parallel()
	_, err := e.UGetOrderData(t.Context(), currency.EMPTYPAIR, "123", "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UGetOrderData(t.Context(), usdtmTradablePair, "123", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUCancelOrder(t *testing.T) {
	t.Parallel()
	_, err := e.UCancelOrder(t.Context(), currency.EMPTYPAIR, "123", "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UCancelOrder(t.Context(), usdtmTradablePair, "123", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	_, err := e.UCancelAllOpenOrders(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UCancelAllOpenOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUCancelBatchOrders(t *testing.T) {
	t.Parallel()
	_, err := e.UCancelBatchOrders(t.Context(), currency.EMPTYPAIR, []string{"123"}, []string{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UCancelBatchOrders(t.Context(), usdtmTradablePair, []string{"123"}, []string{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUAutoCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	_, err := e.UAutoCancelAllOpenOrders(t.Context(), currency.EMPTYPAIR, 30)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UAutoCancelAllOpenOrders(t.Context(), usdtmTradablePair, 30)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFetchOpenOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UFetchOpenOrder(t.Context(), usdtmTradablePair, "123", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUAllAccountOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UAllAccountOpenOrders(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UAllAccountOpenOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUAllAccountOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.UAllAccountOrders(t.Context(), &UAllAccountOrdersRequest{StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UAllAccountOrders(t.Context(), &UAllAccountOrdersRequest{StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UAllAccountOrders(t.Context(), &UAllAccountOrdersRequest{Symbol: usdtmTradablePair, Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUChangeInitialLeverageRequest(t *testing.T) {
	t.Parallel()
	_, err := e.UChangeInitialLeverageRequest(t.Context(), usdtmTradablePair, 0)
	require.ErrorIs(t, err, order.ErrSubmitLeverageNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UChangeInitialLeverageRequest(t.Context(), usdtmTradablePair, 2)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUChangeInitialMarginType(t *testing.T) {
	t.Parallel()
	err := e.UChangeInitialMarginType(t.Context(), usdtmTradablePair, "")
	require.ErrorIs(t, err, margin.ErrInvalidMarginType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err = e.UChangeInitialMarginType(t.Context(), usdtmTradablePair, "ISOLATED")
	assert.NoError(t, err)
}

func TestUModifyIsolatedPositionMarginReq(t *testing.T) {
	t.Parallel()
	_, err := e.UModifyIsolatedPositionMarginReq(t.Context(), usdtmTradablePair, "LONG", "", 5)
	require.ErrorIs(t, err, errMarginChangeTypeInvalid)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UModifyIsolatedPositionMarginReq(t.Context(), usdtmTradablePair, "LONG", "add", 5)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUPositionMarginChangeHistory(t *testing.T) {
	t.Parallel()
	_, err := e.UPositionMarginChangeHistory(t.Context(), &UPositionMarginChangeHistoryRequest{Symbol: usdtmTradablePair, Limit: 5})
	require.ErrorIs(t, err, errMarginChangeTypeInvalid)

	startTime, endTime := getTime()
	_, err = e.UPositionMarginChangeHistory(t.Context(), &UPositionMarginChangeHistoryRequest{Symbol: usdtmTradablePair, ChangeType: "add", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UPositionMarginChangeHistory(t.Context(), &UPositionMarginChangeHistoryRequest{Symbol: usdtmTradablePair, ChangeType: "add", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUGetCommissionRates(t *testing.T) {
	t.Parallel()
	_, err := e.UGetCommissionRates(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UGetCommissionRates(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUSDTUserRateLimits(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUSDTUserRateLimits(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDownloadIDForFuturesTransactionHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetDownloadIDForFuturesTransactionHistory(t.Context(), endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDownloadIDForFuturesTransactionHistory(t.Context(), startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesTransactionHistoryDownloadLinkByID(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesTransactionHistoryDownloadLinkByID(t.Context(), "download-id-here")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesOrderHistoryDownloadLinkByID(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesOrderHistoryDownloadLinkByID(t.Context(), "")
	require.ErrorIs(t, err, errDownloadIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesOrderHistoryDownloadLinkByID(t.Context(), "download-id-here")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesTradeDownloadLinkByID(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesTradeDownloadLinkByID(t.Context(), "")
	require.ErrorIs(t, err, errDownloadIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesTradeDownloadLinkByID(t.Context(), "download-id-here")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesOrderHistoryDownloadID(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.UFuturesOrderHistoryDownloadID(t.Context(), endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UFuturesOrderHistoryDownloadID(t.Context(), startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesTradeHistoryDownloadID(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.FuturesTradeHistoryDownloadID(t.Context(), endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesTradeHistoryDownloadID(t.Context(), startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUAccountTradesHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.UAccountTradesHistory(t.Context(), &UAccountTradesHistoryRequest{Limit: 5, StartTime: startTime, EndTime: endTime})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UAccountTradesHistory(t.Context(), &UAccountTradesHistoryRequest{Symbol: usdtmTradablePair, Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UAccountTradesHistory(t.Context(), &UAccountTradesHistoryRequest{Symbol: usdtmTradablePair, Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUAccountIncomeHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.UAccountIncomeHistory(t.Context(), &UAccountIncomeHistoryRequest{IncomeType: "something-else", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.ErrorIs(t, err, errIncomeTypeRequired)
	_, err = e.UAccountIncomeHistory(t.Context(), &UAccountIncomeHistoryRequest{Symbol: usdtmTradablePair, IncomeType: "something-else", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UAccountIncomeHistory(t.Context(), &UAccountIncomeHistoryRequest{Symbol: usdtmTradablePair, Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUGetNotionalAndLeverageBrackets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UGetNotionalAndLeverageBrackets(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUPositionsADLEstimate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UPositionsADLEstimate(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUAccountForcedOrders(t *testing.T) {
	t.Parallel()
	_, err := e.UAccountForcedOrders(t.Context(), &UAccountForcedOrdersRequest{Symbol: usdtmTradablePair, AutoCloseType: "something-else", Limit: 5})
	require.ErrorIs(t, err, errInvalidAutoCloseType)

	startTime, endTime := getTime()
	_, err = e.UAccountForcedOrders(t.Context(), &UAccountForcedOrdersRequest{Symbol: usdtmTradablePair, AutoCloseType: "ADL", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UAccountForcedOrders(t.Context(), &UAccountForcedOrdersRequest{Symbol: usdtmTradablePair, AutoCloseType: "ADL", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesTradingWuantitativeRulesIndicators(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UFuturesTradingWuantitativeRulesIndicators(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesExchangeInfo(t *testing.T) {
	t.Parallel()
	result, err := e.FuturesExchangeInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesOrderbook(t *testing.T) {
	t.Parallel()
	result, err := e.GetFuturesOrderbook(t.Context(), coinmTradablePair, 1000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesPublicTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesPublicTrades(t.Context(), currency.EMPTYPAIR, 5)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetFuturesPublicTrades(t.Context(), coinmTradablePair, 5)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPastPublicTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetPastPublicTrades(t.Context(), currency.EMPTYPAIR, 5, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetPastPublicTrades(t.Context(), coinmTradablePair, 5, 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAggregatedTradesList(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFuturesAggregatedTradesList(t.Context(), &GetFuturesAggregatedTradesListRequest{Symbol: coinmTradablePair, Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.GetFuturesAggregatedTradesList(t.Context(), &GetFuturesAggregatedTradesListRequest{Symbol: coinmTradablePair, Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPerpsExchangeInfo(t *testing.T) {
	t.Parallel()
	result, err := e.GetPerpMarkets(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIndexAndMarkPrice(t *testing.T) {
	t.Parallel()
	result, err := e.GetIndexAndMarkPrice(t.Context(), "", "BTCUSD")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesKlineData(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesKlineData(t.Context(), &GetFuturesKlineDataRequest{Symbol: coinmTradablePair, Interval: "1Mo", Limit: 5})
	require.ErrorIs(t, err, kline.ErrInvalidInterval)

	startTime, endTime := getTime()
	_, err = e.GetFuturesKlineData(t.Context(), &GetFuturesKlineDataRequest{Symbol: coinmTradablePair, Interval: "5m", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetFuturesKlineData(t.Context(), &GetFuturesKlineDataRequest{Symbol: coinmTradablePair, Interval: "1M", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)

	_, err = e.GetFuturesKlineData(t.Context(), &GetFuturesKlineDataRequest{Symbol: coinmTradablePair, Interval: "5m", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestGetContinuousKlineData(t *testing.T) {
	t.Parallel()
	_, err := e.GetContinuousKlineData(t.Context(), &GetContinuousKlineDataRequest{ContractType: "CURRENT_QUARTER", Interval: "1M", Limit: 5})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetContinuousKlineData(t.Context(), &GetContinuousKlineDataRequest{Pair: "BTCUSD", Interval: "1M", Limit: 5})
	require.ErrorIs(t, err, errContractTypeIsRequired)

	_, err = e.GetContinuousKlineData(t.Context(), &GetContinuousKlineDataRequest{Pair: "BTCUSD", ContractType: "CURRENT_QUARTER", Limit: 5})
	require.ErrorIs(t, err, kline.ErrInvalidInterval)

	startTime, endTime := getTime()
	_, err = e.GetContinuousKlineData(t.Context(), &GetContinuousKlineDataRequest{Pair: "BTCUSD", ContractType: "CURRENT_QUARTER", Interval: "1M", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetContinuousKlineData(t.Context(), &GetContinuousKlineDataRequest{Pair: "BTCUSD", ContractType: "CURRENT_QUARTER", Interval: "1M", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)

	_, err = e.GetContinuousKlineData(t.Context(), &GetContinuousKlineDataRequest{Pair: "BTCUSD", ContractType: "CURRENT_QUARTER", Interval: "1M", Limit: 5, StartTime: startTime, EndTime: endTime})
	assert.NoError(t, err)
}

func TestGetIndexPriceKlines(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexPriceKlines(t.Context(), &GetIndexPriceKlinesRequest{Pair: "BTCUSD", Limit: 5})
	require.ErrorIs(t, err, kline.ErrInvalidInterval)

	startTime, endTime := getTime()
	_, err = e.GetIndexPriceKlines(t.Context(), &GetIndexPriceKlinesRequest{Pair: "BTCUSD", Interval: "1M", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetIndexPriceKlines(t.Context(), &GetIndexPriceKlinesRequest{Pair: "BTCUSD", Interval: "1M", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestGetFuturesSwapTickerChangeStats(t *testing.T) {
	t.Parallel()
	result, err := e.GetFuturesSwapTickerChangeStats(t.Context(), coinmTradablePair, "")
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetFuturesSwapTickerChangeStats(t.Context(), currency.EMPTYPAIR, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesGetFundingHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.FuturesGetFundingHistory(t.Context(), coinmTradablePair, 5, endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesGetFundingHistory(t.Context(), coinmTradablePair, 5, startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.FuturesGetFundingHistory(t.Context(), coinmTradablePair, 50, startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesHistoricalTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesHistoricalTrades(t.Context(), currency.EMPTYPAIR, "", 5)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesHistoricalTrades(t.Context(), coinmTradablePair, "", 5)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesSymbolPriceTicker(t *testing.T) {
	t.Parallel()
	result, err := e.GetFuturesSymbolPriceTicker(t.Context(), coinmTradablePair, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesOrderbookTicker(t *testing.T) {
	t.Parallel()
	result, err := e.GetFuturesOrderbookTicker(t.Context(), currency.EMPTYPAIR, "")
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetFuturesOrderbookTicker(t.Context(), coinmTradablePair, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCFuturesIndexPriceConstituents(t *testing.T) {
	t.Parallel()
	_, err := e.GetCFuturesIndexPriceConstituents(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetCFuturesIndexPriceConstituents(t.Context(), currency.NewBTCUSD())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestOpenInterest(t *testing.T) {
	t.Parallel()
	result, err := e.OpenInterest(t.Context(), coinmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCFuturesQuarterlyContractSettlementPrice(t *testing.T) {
	t.Parallel()
	_, err := e.CFuturesQuarterlyContractSettlementPrice(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.CFuturesQuarterlyContractSettlementPrice(t.Context(), coinmTradablePair)
	require.NoError(t, err)
}

func TestGetOpenInterestStats(t *testing.T) {
	t.Parallel()
	_, err := e.GetOpenInterestStats(t.Context(), &GetOpenInterestStatsRequest{Pair: "BTCUSD", ContractType: "QUARTER", Period: "5m"})
	require.ErrorIs(t, err, errContractTypeIsRequired)

	_, err = e.GetOpenInterestStats(t.Context(), &GetOpenInterestStatsRequest{Pair: "BTCUSD", ContractType: "CURRENT_QUARTER", Period: "5mo"})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.GetOpenInterestStats(t.Context(), &GetOpenInterestStatsRequest{Pair: "BTCUSD", ContractType: "CURRENT_QUARTER", Period: "5m", StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.GetOpenInterestStats(t.Context(), &GetOpenInterestStatsRequest{Pair: "BTCUSD", ContractType: "CURRENT_QUARTER", Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetOpenInterestStats(t.Context(), &GetOpenInterestStatsRequest{Pair: "BTCUSD", ContractType: "CURRENT_QUARTER", Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetTraderFuturesAccountRatio(t *testing.T) {
	t.Parallel()
	_, err := e.GetTraderFuturesAccountRatio(t.Context(), &GetTraderFuturesAccountRatioRequest{Period: "5m"})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetTraderFuturesAccountRatio(t.Context(), &GetTraderFuturesAccountRatioRequest{Pair: usdtmTradablePair})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.GetTraderFuturesAccountRatio(t.Context(), &GetTraderFuturesAccountRatioRequest{Pair: usdtmTradablePair, Period: "5m", StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetTraderFuturesAccountRatio(t.Context(), &GetTraderFuturesAccountRatioRequest{Pair: usdtmTradablePair, Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)

	_, err = e.GetTraderFuturesAccountRatio(t.Context(), &GetTraderFuturesAccountRatioRequest{Pair: usdtmTradablePair, Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestGetTraderFuturesPositionsRatio(t *testing.T) {
	t.Parallel()
	_, err := e.GetTraderFuturesPositionsRatio(t.Context(), &GetTraderFuturesPositionsRatioRequest{Period: "5m"})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.GetTraderFuturesPositionsRatio(t.Context(), &GetTraderFuturesPositionsRatioRequest{Pair: coinmTradablePair, Period: "5mo"})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.GetTraderFuturesPositionsRatio(t.Context(), &GetTraderFuturesPositionsRatioRequest{Pair: coinmTradablePair, Period: "5m", StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetTraderFuturesPositionsRatio(t.Context(), &GetTraderFuturesPositionsRatioRequest{Pair: coinmTradablePair, Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)

	_, err = e.GetTraderFuturesPositionsRatio(t.Context(), &GetTraderFuturesPositionsRatioRequest{Pair: coinmTradablePair, Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestGetMarketRatio(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarketRatio(t.Context(), &GetMarketRatioRequest{Period: "5m"})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetMarketRatio(t.Context(), &GetMarketRatioRequest{Pair: coinmTradablePair, Period: "5mo"})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	startTime, endTime := getTime()
	_, err = e.GetMarketRatio(t.Context(), &GetMarketRatioRequest{Pair: coinmTradablePair, Period: "5m", StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetMarketRatio(t.Context(), &GetMarketRatioRequest{Pair: coinmTradablePair, Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)

	_, err = e.GetMarketRatio(t.Context(), &GetMarketRatioRequest{Pair: coinmTradablePair, Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestGetFuturesTakerVolume(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesTakerVolume(t.Context(), &GetFuturesTakerVolumeRequest{ContractType: "ALL", Period: "5m"})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetFuturesTakerVolume(t.Context(), &GetFuturesTakerVolumeRequest{Pair: coinmTradablePair, ContractType: "abc", Period: "5m"})
	require.ErrorIs(t, err, errContractTypeIsRequired)

	_, err = e.GetFuturesTakerVolume(t.Context(), &GetFuturesTakerVolumeRequest{Pair: coinmTradablePair, ContractType: "ALL", Period: "5mo"})
	require.ErrorIs(t, err, kline.ErrInvalidInterval)

	startTime, endTime := getTime()
	_, err = e.GetFuturesTakerVolume(t.Context(), &GetFuturesTakerVolumeRequest{Pair: coinmTradablePair, ContractType: "ALL", Period: "5m", StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetFuturesTakerVolume(t.Context(), &GetFuturesTakerVolumeRequest{Pair: coinmTradablePair, ContractType: "ALL", Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)

	_, err = e.GetFuturesTakerVolume(t.Context(), &GetFuturesTakerVolumeRequest{Pair: coinmTradablePair, ContractType: "ALL", Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestFuturesBasisData(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesBasisData(t.Context(), &GetFuturesBasisDataRequest{ContractType: "CURRENT_QUARTER", Period: "5m"})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetFuturesBasisData(t.Context(), &GetFuturesBasisDataRequest{Pair: coinmTradablePair, ContractType: "QUARTER", Period: "5m"})
	require.ErrorIs(t, err, errContractTypeIsRequired)
	_, err = e.GetFuturesBasisData(t.Context(), &GetFuturesBasisDataRequest{Pair: coinmTradablePair, ContractType: "CURRENT_QUARTER", Period: "5mo"})
	require.ErrorIs(t, err, errInvalidPeriodOrInterval)

	result, err := e.GetFuturesBasisData(t.Context(), &GetFuturesBasisDataRequest{Pair: currency.NewBTCUSD(), ContractType: "CURRENT_QUARTER", Period: "5m"})
	require.NoError(t, err)
	assert.NotNil(t, result)

	startTime, endTime := getTime()
	_, err = e.GetFuturesBasisData(t.Context(), &GetFuturesBasisDataRequest{Pair: currency.NewBTCUSD(), ContractType: "CURRENT_QUARTER", Period: "5m", StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err = e.GetFuturesBasisData(t.Context(), &GetFuturesBasisDataRequest{Pair: currency.NewBTCUSD(), ContractType: "CURRENT_QUARTER", Period: "5m", StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesNewOrder(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesNewOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &FuturesNewOrderRequest{Symbol: usdtmTradablePair, Side: "BUY", OrderType: order.Limit.String(), PositionSide: "abcd", TimeInForce: order.GoodTillCancel.String(), Quantity: 1, Price: 1}
	_, err = e.FuturesNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidPositionSide)

	arg.PositionSide = ""
	arg.WorkingType = "abc"
	_, err = e.FuturesNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidWorkingType)

	arg.WorkingType = ""
	arg.NewOrderRespType = "abcd"
	_, err = e.FuturesNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidNewOrderResponseType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FuturesNewOrder(t.Context(), &FuturesNewOrderRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), Side: "BUY", OrderType: order.Limit.String(), TimeInForce: order.GoodTillCancel.String(), Quantity: 1, Price: 1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesBatchOrder(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesBatchOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrEmptyParams)

	arg := &PlaceBatchOrderData{
		Symbol:       coinmTradablePair,
		Side:         "BUY",
		OrderType:    order.Limit.String(),
		Quantity:     1,
		Price:        1,
		TimeInForce:  "GTC",
		PositionSide: "abcd",
	}
	_, err = e.FuturesBatchOrder(t.Context(), []*PlaceBatchOrderData{arg})
	require.ErrorIs(t, err, errInvalidPositionSide)

	arg.PositionSide = ""
	arg.WorkingType = "abcd"
	_, err = e.FuturesBatchOrder(t.Context(), []*PlaceBatchOrderData{arg})
	require.ErrorIs(t, err, errInvalidWorkingType)

	arg.NewOrderRespType = "abcd"
	arg.WorkingType = ""
	_, err = e.FuturesBatchOrder(t.Context(), []*PlaceBatchOrderData{arg})
	require.ErrorIs(t, err, errInvalidNewOrderResponseType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FuturesBatchOrder(t.Context(), []*PlaceBatchOrderData{
		{
			Symbol:      coinmTradablePair,
			Side:        "BUY",
			OrderType:   order.Limit.String(),
			Quantity:    1,
			Price:       1,
			TimeInForce: "GTC",
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesBatchCancelOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FuturesBatchCancelOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), []string{"123"}, []string{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesGetOrderData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesGetOrderData(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "123", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FuturesCancelAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"))
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestAutoCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.AutoCancelAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 30000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesOpenOrderData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesOpenOrderData(t.Context(), currency.NewBTCUSD(), "", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesAllOpenOrders(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllFuturesOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetAllFuturesOrders(t.Context(), &GetAllFuturesOrdersRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), StartTime: endTime, EndTime: startTime, OrderID: 1, Limit: 2})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllFuturesOrders(t.Context(), &GetAllFuturesOrdersRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), StartTime: startTime, EndTime: endTime, Limit: 2})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesChangeMarginType(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesChangeMarginType(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "abcd")
	require.ErrorIs(t, err, margin.ErrInvalidMarginType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FuturesChangeMarginType(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "ISOLATED")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesAccountBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesAccountBalance(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesAccountInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesChangeInitialLeverage(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesChangeInitialLeverage(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 129)
	require.ErrorIs(t, err, order.ErrSubmitLeverageNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FuturesChangeInitialLeverage(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), 5)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestModifyIsolatedPositionMargin(t *testing.T) {
	t.Parallel()
	_, err := e.ModifyIsolatedPositionMargin(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "", "abcd", 0)
	require.ErrorIs(t, err, errMarginChangeTypeInvalid)

	_, err = e.ModifyIsolatedPositionMargin(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "abcd", "", 0)
	require.ErrorIs(t, err, errInvalidPositionSide)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ModifyIsolatedPositionMargin(t.Context(), currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), "BOTH", "add", 5)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesMarginChangeHistory(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesMarginChangeHistory(t.Context(), &FuturesMarginChangeHistoryRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), ChangeType: "abc", Limit: 10})
	require.ErrorIs(t, err, errMarginChangeTypeInvalid)

	startTime, endTime := getTime()
	_, err = e.FuturesMarginChangeHistory(t.Context(), &FuturesMarginChangeHistoryRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), ChangeType: "add", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesMarginChangeHistory(t.Context(), &FuturesMarginChangeHistoryRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), ChangeType: "add", StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesPositionsInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesPositionsInfo(t.Context(), "BTCUSD", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesTradeHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.FuturesTradeHistory(t.Context(), &FuturesTradeHistoryRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), StartTime: endTime, EndTime: startTime, Limit: 5})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesTradeHistory(t.Context(), &FuturesTradeHistoryRequest{Symbol: currency.NewPairWithDelimiter("BTCUSD", "PERP", "_"), StartTime: startTime, EndTime: endTime, Limit: 5})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesIncomeHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.FuturesIncomeHistory(t.Context(), &FuturesIncomeHistoryRequest{IncomeType: "TRANSFER", StartTime: endTime, EndTime: startTime, Limit: 5})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesIncomeHistory(t.Context(), &FuturesIncomeHistoryRequest{IncomeType: "TRANSFER", StartTime: startTime, EndTime: endTime, Limit: 5})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesForceOrders(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesForceOrders(t.Context(), currency.EMPTYPAIR, "abcd", time.Time{}, time.Time{})
	require.ErrorIs(t, err, errInvalidAutoCloseType)

	startTime, endTime := getTime()
	_, err = e.FuturesForceOrders(t.Context(), currency.EMPTYPAIR, "abcd", endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesForceOrders(t.Context(), currency.EMPTYPAIR, "ADL", startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUGetNotionalLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCFuturesLeverageBracket(t.Context(), coinmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetCFuturesLeverageBracket(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesPositionsADLEstimate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FuturesPositionsADLEstimate(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarkPriceKline(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarkPriceKline(t.Context(), &GetMarkPriceKlineRequest{Symbol: coinmTradablePair, Interval: "1Mo", Limit: 5})
	require.ErrorIs(t, err, kline.ErrInvalidInterval)

	startTime, endTime := getTime()
	_, err = e.GetMarkPriceKline(t.Context(), &GetMarkPriceKlineRequest{Symbol: coinmTradablePair, Interval: "1M", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetMarkPriceKline(t.Context(), &GetMarkPriceKlineRequest{Symbol: coinmTradablePair, Interval: "1M", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestGetPremiumIndexKlineData(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetPremiumIndexKlineData(t.Context(), &GetPremiumIndexKlineDataRequest{Symbol: coinmTradablePair, Interval: "1M", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetPremiumIndexKlineData(t.Context(), &GetPremiumIndexKlineDataRequest{Interval: "1M", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetPremiumIndexKlineData(t.Context(), &GetPremiumIndexKlineDataRequest{Symbol: coinmTradablePair, Interval: "1M", Limit: 5, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetPremiumIndexKlineData(t.Context(), &GetPremiumIndexKlineDataRequest{Symbol: coinmTradablePair, Interval: "1M", Limit: 5, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
}

func TestGetExchangeInfo(t *testing.T) {
	t.Parallel()
	result, err := e.GetExchangeInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
	if !mockTests {
		assert.WithinRange(t, result.ServerTime.Time(), time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), "ServerTime should be within a day of now")
	}
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTradablePairs(t.Context(), asset.Empty)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	assetTypes := e.GetAssetTypes(true)
	for a := range []asset.Item{asset.CoinMarginedFutures} {
		results, err := e.FetchTradablePairs(t.Context(), assetTypes[a])
		assert.NoError(t, err)
		assert.NotNil(t, results)
	}
}

func TestGetOrderBook(t *testing.T) {
	t.Parallel()
	result, err := e.GetOrderBook(t.Context(),
		OrderBookDataRequest{
			Symbol: currency.NewBTCUSDT(),
			Limit:  1000,
		})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMostRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetMostRecentTrades(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	result, err := e.GetMostRecentTrades(t.Context(), &RecentTradeRequest{Symbol: usdtmTradablePair, Limit: 15})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetHistoricalTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricalTrades(t.Context(), currency.EMPTYPAIR, 5, -1)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetHistoricalTrades(t.Context(), usdtmTradablePair, 5, -1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAggregatedTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetAggregatedTrades(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetAggregatedTrades(t.Context(), &AggregatedTradeRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetAggregatedTrades(t.Context(), &AggregatedTradeRequest{Symbol: usdtmTradablePair, Limit: 5})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSpotKline(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotKline(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	startTime, endTime := getTime()
	_, err = e.GetSpotKline(t.Context(), &KlinesRequest{Symbol: usdtmTradablePair, Interval: kline.FiveMin.Short(), Limit: 24, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.GetSpotKline(t.Context(), &KlinesRequest{Symbol: usdtmTradablePair, Interval: kline.FiveMin.Short(), Limit: 24, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Timezone must be sent; the mock routes it to a distinct single-candle payload,
	// so dropping the parameter silently returns the untimezoned fixture instead
	result, err = e.GetSpotKline(t.Context(), &KlinesRequest{Symbol: usdtmTradablePair, Interval: kline.FiveMin.Short(), Limit: 24, StartTime: startTime, EndTime: endTime, Timezone: "+08:00"})
	require.NoError(t, err)
	require.Len(t, result, 1, "timeZone must be forwarded to the request")
	assert.Equal(t, 1.5, result[0].Close.Float64(), "close should come from the timezoned fixture")
}

func TestGetUIKline(t *testing.T) {
	t.Parallel()
	_, err := e.GetUIKline(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	startTime, endTime := getTime()
	_, err = e.GetUIKline(t.Context(), &KlinesRequest{Symbol: usdtmTradablePair, Interval: kline.FiveMin.Short(), Limit: 24, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	result, err := e.GetUIKline(t.Context(), &KlinesRequest{Symbol: usdtmTradablePair, Interval: kline.FiveMin.Short(), Limit: 24, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAveragePrice(t *testing.T) {
	t.Parallel()
	result, err := e.GetAveragePrice(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPriceChangeStats(t *testing.T) {
	t.Parallel()
	result, err := e.GetPriceChangeStats(t.Context(), spotTradablePair, currency.Pairs{})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetPriceChangeStats(t.Context(), marginTradablePair, currency.Pairs{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetTradingDayTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetTradingDayTicker(t.Context(), []currency.Pair{}, "", "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairsEmpty)
	_, err = e.GetTradingDayTicker(t.Context(), []currency.Pair{currency.EMPTYPAIR}, "", "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetTradingDayTicker(t.Context(), []currency.Pair{spotTradablePair}, "", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLatestSpotPrice(t *testing.T) {
	t.Parallel()
	result, err := e.GetLatestSpotPrice(t.Context(), usdtmTradablePair, currency.Pairs{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBestPrice(t *testing.T) {
	t.Parallel()
	result, err := e.GetBestPrice(t.Context(), spotTradablePair, currency.Pairs{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetTickerData(t *testing.T) {
	t.Parallel()
	_, err := e.GetTickerData(t.Context(), []currency.Pair{}, time.Minute*20, "FULL")
	require.ErrorIs(t, err, currency.ErrCurrencyPairsEmpty)

	result, err := e.GetTickerData(t.Context(), []currency.Pair{spotTradablePair}, time.Minute*20, "FULL")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDepositAddressForCurrency(t *testing.T) {
	t.Parallel()
	_, err := e.GetDepositAddressForCurrency(t.Context(), currency.EMPTYCODE, "")
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDepositAddressForCurrency(t.Context(), currency.BTC, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAssetsThatCanBeConvertedIntoBNB(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAssetsThatCanBeConvertedIntoBNB(t.Context(), "MINI")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWithdrawCrypto(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawCrypto(t.Context(), &WithdrawCryptoRequest{WithdrawOrderID: "123435", Address: "address-here", Amount: 100})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.WithdrawCrypto(t.Context(), &WithdrawCryptoRequest{Coin: currency.USDT, Amount: 100})
	require.ErrorIs(t, err, errAddressRequired)
	_, err = e.WithdrawCrypto(t.Context(), &WithdrawCryptoRequest{Coin: currency.USDT, WithdrawOrderID: "123435", Address: "address-here", AddressTag: "123213"})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.WithdrawCrypto(t.Context(), &WithdrawCryptoRequest{Coin: currency.USDT, WithdrawOrderID: "123435", Address: "address", Amount: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDustTransfer(t *testing.T) {
	t.Parallel()
	_, err := e.DustTransfer(t.Context(), []string{}, "SPOT")
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.DustTransfer(t.Context(), []string{"BTC", "USDT"}, "SPOT")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAssetDevidendRecords(t *testing.T) {
	t.Parallel()
	_, err := e.GetAssetDevidendRecords(t.Context(), currency.EMPTYCODE, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	startTime, endTime := getTime()
	_, err = e.GetAssetDevidendRecords(t.Context(), currency.BTC, endTime, startTime, 1)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAssetDevidendRecords(t.Context(), currency.BTC, startTime, endTime, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAssetDetail(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAssetDetail(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetTradeFees(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetTradeFees(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)

	_, err = e.GetTradeFees(t.Context(), spotTradablePair)
	require.NoError(t, err)
}

func TestUserUniversalTransfer(t *testing.T) {
	t.Parallel()
	_, err := e.UserUniversalTransfer(t.Context(), &UserUniversalTransferRequest{Amount: 123.234, Currency: currency.BTC})
	require.ErrorIs(t, err, errTransferTypeRequired)
	_, err = e.UserUniversalTransfer(t.Context(), &UserUniversalTransferRequest{TransferType: ttMainUMFuture, Amount: 123.234})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.UserUniversalTransfer(t.Context(), &UserUniversalTransferRequest{TransferType: ttMainUMFuture, Currency: currency.BTC})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UserUniversalTransfer(t.Context(), &UserUniversalTransferRequest{TransferType: ttMainUMFuture, Amount: 123.234, Currency: currency.BTC})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserUniversalTransferHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetUserUniversalTransferHistory(t.Context(), &GetUserUniversalTransferHistoryRequest{FromSymbol: currency.BTC, ToSymbol: currency.USDT})
	require.ErrorIs(t, err, errTransferTypeRequired)
	_, err = e.GetUserUniversalTransferHistory(t.Context(), &GetUserUniversalTransferHistoryRequest{TransferType: ttUMFutureMargin, FromSymbol: currency.BTC, ToSymbol: currency.USDT})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	startTime, endTime := getTime()
	_, err = e.GetUserUniversalTransferHistory(t.Context(), &GetUserUniversalTransferHistoryRequest{TransferType: ttUMFutureMargin, StartTime: endTime, EndTime: startTime, Current: 1, Size: 1234, FromSymbol: currency.BTC, ToSymbol: currency.USDT})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserUniversalTransferHistory(t.Context(), &GetUserUniversalTransferHistoryRequest{TransferType: ttUMFutureMargin, EndTime: endTime, Current: 1, Size: 1234, FromSymbol: currency.BTC, ToSymbol: currency.USDT})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFundingAssets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFundingAssets(t.Context(), currency.BTC, true)
	require.NoError(t, err)
}

func TestGetUserAssets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserAssets(t.Context(), currency.BTC, true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestConvertBUSD(t *testing.T) {
	t.Parallel()
	_, err := e.ConvertBUSD(t.Context(), &ConvertBUSDRequest{AccountType: "MAIN", AssetCcy: currency.ETH, TargetAsset: currency.USD, Amount: 1234})
	require.ErrorIs(t, err, errTransactionIDRequired)
	_, err = e.ConvertBUSD(t.Context(), &ConvertBUSDRequest{ClientTransactionID: "12321412312", AccountType: "MAIN", TargetAsset: currency.USD, Amount: 1234})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.ConvertBUSD(t.Context(), &ConvertBUSDRequest{ClientTransactionID: "12321412312", AccountType: "MAIN", AssetCcy: currency.ETH, TargetAsset: currency.USD})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.ConvertBUSD(t.Context(), &ConvertBUSDRequest{ClientTransactionID: "12321412312", AccountType: "MAIN", AssetCcy: currency.ETH, Amount: 1234})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ConvertBUSD(t.Context(), &ConvertBUSDRequest{ClientTransactionID: "12321412312", AccountType: "MAIN", AssetCcy: currency.ETH, TargetAsset: currency.USD, Amount: 1234})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBUSDConvertHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.BUSDConvertHistory(t.Context(), &BUSDConvertHistoryRequest{TransactionID: "transaction-id", ClientTransactionID: "233423423", AccountType: "CARD", Asset: currency.BTC, StartTime: endTime, EndTime: startTime, Size: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.BUSDConvertHistory(t.Context(), &BUSDConvertHistoryRequest{TransactionID: "transaction-id", ClientTransactionID: "233423423", AccountType: "CARD", Asset: currency.BTC, StartTime: startTime, EndTime: endTime, Size: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCloudMiningPaymentAndRefundHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetCloudMiningPaymentAndRefundHistory(t.Context(), &GetCloudMiningPaymentAndRefundHistoryRequest{ClientTransactionID: "1234", AssetCcy: currency.BTC, StartTime: endTime, EndTime: startTime, TransactionID: 1232313})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCloudMiningPaymentAndRefundHistory(t.Context(), &GetCloudMiningPaymentAndRefundHistoryRequest{ClientTransactionID: "1234", AssetCcy: currency.BTC, StartTime: startTime, EndTime: endTime, TransactionID: 1232313})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserAccountInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAPIKeyPermission(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAPIKeyPermission(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAutoConvertingStableCoins(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAutoConvertingStableCoins(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSwitchOnOffBUSDAndStableCoinsConversion(t *testing.T) {
	t.Parallel()
	err := e.SwitchOnOffBUSDAndStableCoinsConversion(t.Context(), currency.EMPTYCODE, false)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err = e.SwitchOnOffBUSDAndStableCoinsConversion(t.Context(), currency.BTC, false)
	assert.NoError(t, err)
}

func TestOneClickArrivalDepositApply(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.OneClickArrivalDepositApply(t.Context(), "", 0, 0, 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDepositAddressListWithNetwork(t *testing.T) {
	t.Parallel()
	_, err := e.GetDepositAddressListWithNetwork(t.Context(), currency.EMPTYCODE, "")
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDepositAddressListWithNetwork(t.Context(), currency.BTC, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserWalletBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserWalletBalance(t.Context(), currency.ETH)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserDelegationHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetUserDelegationHistory(t.Context(), &GetUserDelegationHistoryRequest{Delegation: "Delegate", Currency: currency.BTC})
	require.ErrorIs(t, err, errValidEmailRequired)

	startTime, endTime := getTime()
	_, err = e.GetUserDelegationHistory(t.Context(), &GetUserDelegationHistoryRequest{Email: "someone@thrasher.com", Delegation: "Delegate", StartTime: endTime, EndTime: startTime, Currency: currency.BTC})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserDelegationHistory(t.Context(), &GetUserDelegationHistoryRequest{Email: "someone@thrasher.com", Delegation: "Delegate", StartTime: startTime, EndTime: endTime, Currency: currency.BTC})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSymbolsDelistScheduleForSpot(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSymbolsDelistScheduleForSpot(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateVirtualSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CreateVirtualSubAccount(t.Context(), "something-string")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountList(t.Context(), "testsub@gmail.com", false, 0, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountSpotAssetTransferHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSubAccountSpotAssetTransferHistory(t.Context(), &GetSubAccountSpotAssetTransferHistoryRequest{StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountSpotAssetTransferHistory(t.Context(), &GetSubAccountSpotAssetTransferHistoryRequest{EndTime: time.Now(), Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountFuturesAssetTransferHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountFuturesAssetTransferHistory(t.Context(), &GetSubAccountFuturesAssetTransferHistoryRequest{EndTime: time.Now(), FuturesType: 2})
	require.ErrorIs(t, err, errValidEmailRequired)

	_, err = e.GetSubAccountFuturesAssetTransferHistory(t.Context(), &GetSubAccountFuturesAssetTransferHistoryRequest{Email: "someone@gmail.com", EndTime: time.Now()})
	require.ErrorIs(t, err, errInvalidFuturesType)

	_, err = e.GetSubAccountFuturesAssetTransferHistory(t.Context(), &GetSubAccountFuturesAssetTransferHistoryRequest{Email: "someone@gmail.com", StartTime: time.Now(), EndTime: time.Now().Add(-time.Hour * 20), FuturesType: 1})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	startTime, endTime := time.Now().Add(-time.Hour*240), time.Now()
	if mockTests {
		startTime, endTime = time.UnixMilli(1773095429591), time.UnixMilli(1773959429591)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountFuturesAssetTransferHistory(t.Context(), &GetSubAccountFuturesAssetTransferHistoryRequest{Email: "samuaeladnew.zebir@gmail.com", StartTime: startTime, EndTime: endTime, FuturesType: 2})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubAccountFuturesAssetTransfer(t *testing.T) {
	t.Parallel()
	_, err := e.SubAccountFuturesAssetTransfer(t.Context(), &SubAccountFuturesAssetTransferRequest{FromEmail: "from_someone", ToEmail: "to_someont@thrasher.io", FuturesType: 1, Currency: currency.USDT, Amount: 0.1})
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.SubAccountFuturesAssetTransfer(t.Context(), &SubAccountFuturesAssetTransferRequest{FromEmail: "from_someone@thrasher.io", ToEmail: "to_someont", FuturesType: 1, Currency: currency.USDT, Amount: 0.1})
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.SubAccountFuturesAssetTransfer(t.Context(), &SubAccountFuturesAssetTransferRequest{FromEmail: "from_someone@thrasher.io", ToEmail: "to_someont@thrasher.io", FuturesType: -1, Currency: currency.USDT, Amount: 0.1})
	require.ErrorIs(t, err, errInvalidFuturesType)
	_, err = e.SubAccountFuturesAssetTransfer(t.Context(), &SubAccountFuturesAssetTransferRequest{FromEmail: "from_someone@thrasher.io", ToEmail: "to_someont@thrasher.io", FuturesType: 1, Amount: 0.1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubAccountFuturesAssetTransfer(t.Context(), &SubAccountFuturesAssetTransferRequest{FromEmail: "from_someone@thrasher.io", ToEmail: "to_someont@thrasher.io", FuturesType: 1, Currency: currency.USDT, Amount: 0.1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountAssets(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountAssets(t.Context(), "email_address")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountAssets(t.Context(), "address@gmail.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetManagedSubAccountList(t.Context(), "address@gmail.com", 0, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountTransactionStatistics(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountTransactionStatistics(t.Context(), "addressio")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountTransactionStatistics(t.Context(), "address@thrasher.io")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountDepositAddress(t *testing.T) {
	t.Parallel()
	_, err := e.GetManagedSubAccountDepositAddress(t.Context(), currency.ETH, "destination", "")
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.GetManagedSubAccountDepositAddress(t.Context(), currency.EMPTYCODE, "destination@thrasher.io", "")
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetManagedSubAccountDepositAddress(t.Context(), currency.ETH, "destination@thrasher.io", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableOptionsForSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.EnableOptionsForSubAccount(t.Context(), "")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.EnableOptionsForSubAccount(t.Context(), "address@mail.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountTransferLog(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetManagedSubAccountTransferLog(t.Context(), &GetManagedSubAccountTransferLogRequest{StartTime: endTime, EndTime: startTime, Page: 1, Limit: 10, TransferFunctionAccountType: "MARGIN"})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)
	_, err = e.GetManagedSubAccountTransferLog(t.Context(), &GetManagedSubAccountTransferLogRequest{StartTime: startTime, EndTime: endTime, Page: -1, Limit: 10, TransferFunctionAccountType: "MARGIN"})
	require.ErrorIs(t, err, errPageNumberRequired)
	_, err = e.GetManagedSubAccountTransferLog(t.Context(), &GetManagedSubAccountTransferLogRequest{StartTime: startTime, EndTime: endTime, Page: 1, Limit: -1, TransferFunctionAccountType: "MARGIN"})
	require.ErrorIs(t, err, errLimitNumberRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetManagedSubAccountTransferLog(t.Context(), &GetManagedSubAccountTransferLogRequest{StartTime: startTime, EndTime: endTime, Page: 1, Limit: 10, TransferFunctionAccountType: "MARGIN"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountSpotAssetsSummary(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountSpotAssetsSummary(t.Context(), "the_address@thrasher.io", 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountDepositAddress(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountDepositAddress(t.Context(), "", "BTC", "", 0.1)
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.GetSubAccountDepositAddress(t.Context(), "the_address@thrasher.io", "", "", 0.1)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountDepositAddress(t.Context(), "the_address@thrasher.io", "BTC", "", 0.1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountDepositHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountDepositHistory(t.Context(), &GetSubAccountDepositHistoryRequest{Email: "someoneio", Coin: "BTC", EndTime: time.Now(), Limit: 10})
	require.ErrorIs(t, err, errValidEmailRequired)

	startTime, endTime := getTime()
	_, err = e.GetSubAccountDepositHistory(t.Context(), &GetSubAccountDepositHistoryRequest{Email: "someone@thrasher.io", Coin: "BTC", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountDepositHistory(t.Context(), &GetSubAccountDepositHistoryRequest{Email: "someone@thrasher.io", Coin: "BTC", StartTime: startTime, EndTime: endTime, Status: 1, Offset: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountStatusOnMarginFutures(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountStatusOnMarginFutures(t.Context(), "myemail@mail.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableMarginForSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.EnableMarginForSubAccount(t.Context(), "sampleemaicom")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.EnableMarginForSubAccount(t.Context(), "sampleemail@email.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDetailOnSubAccountMarginAccount(t *testing.T) {
	t.Parallel()
	_, err := e.GetDetailOnSubAccountMarginAccount(t.Context(), "com")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDetailOnSubAccountMarginAccount(t.Context(), "test@gmail.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSummaryOfSubAccountMarginAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSummaryOfSubAccountMarginAccount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableFuturesSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.EnableFuturesSubAccount(t.Context(), "address")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.EnableFuturesSubAccount(t.Context(), "address@gmail.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesPositionRiskSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.GetV2FuturesPositionRiskSubAccount(t.Context(), "address", 1)
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.GetV2FuturesPositionRiskSubAccount(t.Context(), "address@mail.com", -1)
	require.ErrorIs(t, err, errInvalidFuturesType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetV2FuturesPositionRiskSubAccount(t.Context(), "address@mail.com", 1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableLeverageTokenForSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.EnableLeverageTokenForSubAccount(t.Context(), "email-address", false)
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.EnableLeverageTokenForSubAccount(t.Context(), "someone@thrasher.io", false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIPRestrictionForSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.GetIPRestrictionForSubAccountAPIKey(t.Context(), "emailaddress", apiCredentials.Key)
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.GetIPRestrictionForSubAccountAPIKey(t.Context(), "emailaddress@thrasher.io", "")
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetIPRestrictionForSubAccountAPIKey(t.Context(), "emailaddress@thrasher.io", apiCredentials.Key)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDeleteIPListForSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.DeleteIPListForSubAccountAPIKey(t.Context(), "emailaddress", apiCredentials.Key, "196.168.4.1")
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.DeleteIPListForSubAccountAPIKey(t.Context(), "emailaddress@thrasher.io", "", "196.168.4.1")
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.DeleteIPListForSubAccountAPIKey(t.Context(), "emailaddress@thrasher.io", apiCredentials.Key, "196.168.4.1")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestAddIPRestrictionForSubAccountAPIkey(t *testing.T) {
	t.Parallel()
	_, err := e.AddIPRestrictionForSubAccountAPIkey(t.Context(), "addressthrasher", apiCredentials.Key, "", true)
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.AddIPRestrictionForSubAccountAPIkey(t.Context(), "address@thrasher.io", "", "", true)
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.AddIPRestrictionForSubAccountAPIkey(t.Context(), "address@thrasher.io", apiCredentials.Key, "", true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDepositAssetsIntoTheManagedSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.DepositAssetsIntoTheManagedSubAccount(t.Context(), "toemail", currency.BTC, 0.0001)
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.DepositAssetsIntoTheManagedSubAccount(t.Context(), "toemail@mail.com", currency.EMPTYCODE, 0.0001)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.DepositAssetsIntoTheManagedSubAccount(t.Context(), "toemail@mail.com", currency.BTC, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.DepositAssetsIntoTheManagedSubAccount(t.Context(), "toemail@mail.com", currency.BTC, 0.0001)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountAssetsDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetManagedSubAccountAssetsDetails(t.Context(), "emailaddress")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.GetManagedSubAccountAssetsDetails(t.Context(), "emailaddress@thrashser.io")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWithdrawAssetsFromManagedSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawAssetsFromManagedSubAccount(t.Context(), "source", currency.BTC, 0.0000001, time.Now().Add(-time.Hour*24*50))
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.WithdrawAssetsFromManagedSubAccount(t.Context(), "source@email.com", currency.EMPTYCODE, 0.0000001, time.Now().Add(-time.Hour*24*50))
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.WithdrawAssetsFromManagedSubAccount(t.Context(), "source@email.com", currency.BTC, 0, time.Now().Add(-time.Hour*24*50))
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.WithdrawAssetsFromManagedSubAccount(t.Context(), "source@email.com", currency.BTC, 0.0000001, time.Now().Add(-time.Hour*24*50))
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountSnapshot(t *testing.T) {
	t.Parallel()
	_, err := e.GetManagedSubAccountSnapshot(t.Context(), &GetManagedSubAccountSnapshotRequest{Email: "address", AssetType: "SPOT", EndTime: time.Now(), Limit: 10})
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.GetManagedSubAccountSnapshot(t.Context(), &GetManagedSubAccountSnapshotRequest{Email: "address@thrasher.io", EndTime: time.Now(), Limit: 10})
	require.ErrorIs(t, err, asset.ErrInvalidAsset)

	startTime, endTime := getTime()
	_, err = e.GetManagedSubAccountSnapshot(t.Context(), &GetManagedSubAccountSnapshotRequest{Email: "address@thrasher.io", AssetType: "SPOT", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetManagedSubAccountSnapshot(t.Context(), &GetManagedSubAccountSnapshotRequest{Email: "address@thrasher.io", AssetType: "SPOT", StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountTransferLogForInvestorMasterAccount(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetManagedSubAccountTransferLogForInvestorMasterAccount(t.Context(), &GetManagedSubAccountTransferLogForInvestorMasterAccountRequest{Email: "address.com", Transfers: "TO", TransferFunctionAccountType: "SPOT", StartTime: startTime, EndTime: endTime, Page: 1, Limit: 10})
	require.ErrorIs(t, err, errValidEmailRequired)

	_, err = e.GetManagedSubAccountTransferLogForInvestorMasterAccount(t.Context(), &GetManagedSubAccountTransferLogForInvestorMasterAccountRequest{Email: "address@gmail.com", Transfers: "TO", TransferFunctionAccountType: "SPOT", StartTime: startTime, EndTime: endTime, Page: -1, Limit: 10})
	require.ErrorIs(t, err, errPageNumberRequired)

	_, err = e.GetManagedSubAccountTransferLogForInvestorMasterAccount(t.Context(), &GetManagedSubAccountTransferLogForInvestorMasterAccountRequest{Email: "address@gmail.com", Transfers: "TO", TransferFunctionAccountType: "SPOT", StartTime: startTime, EndTime: endTime, Page: 1})
	require.ErrorIs(t, err, errLimitNumberRequired)

	_, err = e.GetManagedSubAccountTransferLogForInvestorMasterAccount(t.Context(), &GetManagedSubAccountTransferLogForInvestorMasterAccountRequest{Email: "address@gmail.com", Transfers: "TO", TransferFunctionAccountType: "SPOT", StartTime: endTime, EndTime: startTime, Page: 1, Limit: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetManagedSubAccountTransferLogForInvestorMasterAccount(t.Context(), &GetManagedSubAccountTransferLogForInvestorMasterAccountRequest{Email: "address@gmail.com", Transfers: "TO", TransferFunctionAccountType: "SPOT", StartTime: startTime, EndTime: endTime, Page: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountTransferLogForTradingTeam(t *testing.T) {
	t.Parallel()
	_, err := e.GetManagedSubAccountTransferLogForTradingTeam(t.Context(), &GetManagedSubAccountTransferLogForTradingTeamRequest{Email: "address", Transfers: "FROM", TransferFunctionAccountType: "ISOLATED_MARGIN", Page: 1, Limit: 10})
	require.ErrorIs(t, err, errValidEmailRequired)

	startTime, endTime := getTime()
	_, err = e.GetManagedSubAccountTransferLogForTradingTeam(t.Context(), &GetManagedSubAccountTransferLogForTradingTeamRequest{Email: "address@gmail.com", Transfers: "FROM", TransferFunctionAccountType: "ISOLATED_MARGIN", StartTime: time.Now(), Page: -1, Limit: 10})
	require.ErrorIs(t, err, errPageNumberRequired)

	_, err = e.GetManagedSubAccountTransferLogForTradingTeam(t.Context(), &GetManagedSubAccountTransferLogForTradingTeamRequest{Email: "address@gmail.com", Transfers: "FROM", TransferFunctionAccountType: "ISOLATED_MARGIN", StartTime: startTime, Page: 1})
	require.ErrorIs(t, err, errLimitNumberRequired)

	_, err = e.GetManagedSubAccountTransferLogForTradingTeam(t.Context(), &GetManagedSubAccountTransferLogForTradingTeamRequest{Email: "address@gmail.com", Transfers: "FROM", TransferFunctionAccountType: "ISOLATED_MARGIN", StartTime: endTime, EndTime: startTime, Page: 1, Limit: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetManagedSubAccountTransferLogForTradingTeam(t.Context(), &GetManagedSubAccountTransferLogForTradingTeamRequest{Email: "address@gmail.com", Transfers: "FROM", TransferFunctionAccountType: "ISOLATED_MARGIN", StartTime: startTime, EndTime: endTime, Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountFutureesAssetDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetManagedSubAccountFutureesAssetDetails(t.Context(), "address")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetManagedSubAccountFutureesAssetDetails(t.Context(), "address@email.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetManagedSubAccountMarginAssetDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetManagedSubAccountMarginAssetDetails(t.Context(), "address")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetManagedSubAccountMarginAssetDetails(t.Context(), "address@gmail.com")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFuturesTransferSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesTransferSubAccount(t.Context(), "someone.com", currency.BTC, 1.1, 1)
	require.ErrorIs(t, err, errValidEmailRequired)

	_, err = e.FuturesTransferSubAccount(t.Context(), "someone@mail.com", currency.EMPTYCODE, 1.1, 1)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.FuturesTransferSubAccount(t.Context(), "someone@mail.com", currency.BTC, 0, 1)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	_, err = e.FuturesTransferSubAccount(t.Context(), "someone@mail.com", currency.BTC, 1.1, 0)
	require.ErrorIs(t, err, errTransferTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FuturesTransferSubAccount(t.Context(), "someone@mail.com", currency.BTC, 1.1, 1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginTransferForSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.MarginTransferForSubAccount(t.Context(), "someone", currency.BTC, 1.1, 1)
	require.ErrorIs(t, err, errValidEmailRequired)

	_, err = e.MarginTransferForSubAccount(t.Context(), "someone@mail.com", currency.EMPTYCODE, 1.1, 1)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.MarginTransferForSubAccount(t.Context(), "someone@mail.com", currency.BTC, 0, 1)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	_, err = e.MarginTransferForSubAccount(t.Context(), "someone@mail.com", currency.BTC, 1.1, -1)
	require.ErrorIs(t, err, errTransferTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.MarginTransferForSubAccount(t.Context(), "someone@mail.com", currency.BTC, 1.1, 1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestTransferToSubAccountOfSameMaster(t *testing.T) {
	t.Parallel()
	_, err := e.TransferToSubAccountOfSameMaster(t.Context(), "thrasher", currency.ETH, 10)
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.TransferToSubAccountOfSameMaster(t.Context(), "toEmail@thrasher.io", currency.EMPTYCODE, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.TransferToSubAccountOfSameMaster(t.Context(), "toEmail@thrasher.io", currency.ETH, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.TransferToSubAccountOfSameMaster(t.Context(), "toEmail@thrasher.io", currency.ETH, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFromSubAccountTransferToMaster(t *testing.T) {
	t.Parallel()
	_, err := e.FromSubAccountTransferToMaster(t.Context(), currency.EMPTYCODE, 0.1)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FromSubAccountTransferToMaster(t.Context(), currency.LTC, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FromSubAccountTransferToMaster(t.Context(), currency.LTC, 0.1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubAccountTransferHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.SubAccountTransferHistory(t.Context(), &SubAccountTransferHistoryRequest{Currency: currency.BTC, TransferType: 1, Limit: 10, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubAccountTransferHistory(t.Context(), &SubAccountTransferHistoryRequest{Currency: currency.BTC, TransferType: 1, Limit: 10, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubAccountTransferHistoryForSubAccount(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.SubAccountTransferHistoryForSubAccount(t.Context(), &SubAccountTransferHistoryForSubAccountRequest{Currency: currency.LTC, TransferType: 2, StartTime: endTime, EndTime: startTime, ReturnFailHistory: true})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubAccountTransferHistoryForSubAccount(t.Context(), &SubAccountTransferHistoryForSubAccountRequest{Currency: currency.LTC, TransferType: 2, StartTime: startTime, EndTime: endTime, ReturnFailHistory: true})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUniversalTransferForMasterAccount(t *testing.T) {
	t.Parallel()
	_, err := e.UniversalTransferForMasterAccount(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &UniversalTransferRequest{
		ClientTransactionID: "transaction-id",
	}
	_, err = e.UniversalTransferForMasterAccount(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidAccountType)

	arg.ToAccountType = "SPOT"
	_, err = e.UniversalTransferForMasterAccount(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidAccountType)

	arg.FromAccountType = "ISOLATED_MARGIN"
	_, err = e.UniversalTransferForMasterAccount(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.Asset = currency.BTC
	_, err = e.UniversalTransferForMasterAccount(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UniversalTransferForMasterAccount(t.Context(), &UniversalTransferRequest{
		FromEmail:           "source@thrasher.io",
		ToEmail:             "destination@thrasher.io",
		FromAccountType:     "ISOLATED_MARGIN",
		ToAccountType:       "SPOT",
		ClientTransactionID: "transaction-id",
		Symbol:              "",
		Asset:               currency.BTC,
		Amount:              0.0003,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUniversalTransferHistoryForMasterAccount(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetUniversalTransferHistoryForMasterAccount(t.Context(), &GetUniversalTransferHistoryForMasterAccountRequest{StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUniversalTransferHistoryForMasterAccount(t.Context(), &GetUniversalTransferHistoryForMasterAccountRequest{StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDetailOnSubAccountsFuturesAccountV2(t *testing.T) {
	t.Parallel()
	_, err := e.GetDetailOnSubAccountsFuturesAccount(t.Context(), "thrasher", 1)
	require.ErrorIs(t, err, errValidEmailRequired)
	_, err = e.GetDetailOnSubAccountsFuturesAccount(t.Context(), "address@thrasher.io", 0)
	require.ErrorIs(t, err, errInvalidFuturesType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDetailOnSubAccountsFuturesAccount(t.Context(), "address@thrasher.io", 1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSummaryOfSubAccountsFuturesAccountV2(t *testing.T) {
	t.Parallel()
	_, err := e.GetSummaryOfSubAccountsFuturesAccount(t.Context(), 0, 0, 10)
	require.ErrorIs(t, err, errInvalidFuturesType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSummaryOfSubAccountsFuturesAccount(t.Context(), 1, 0, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestQueryOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.QueryOrder(t.Context(), usdtmTradablePair, "", 1337)
	require.False(t, sharedtestvalues.AreAPICredentialsSet(e) && err != nil, err)
	require.False(t, !sharedtestvalues.AreAPICredentialsSet(e) && err == nil && !mockTests, "expecting an error when no keys are set")
	assert.False(t, mockTests && err != nil, err)
}

func TestCancelExistingOrderAndSendNewOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelExistingOrderAndSendNewOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &CancelReplaceOrderRequest{
		TimeInForce: "GTC",
	}
	_, err = e.CancelExistingOrderAndSendNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.CancelExistingOrderAndSendNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "BUY"
	_, err = e.CancelExistingOrderAndSendNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = order.Limit.String()
	_, err = e.CancelExistingOrderAndSendNewOrder(t.Context(), arg)
	require.ErrorIs(t, err, errCancelReplaceModeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelExistingOrderAndSendNewOrder(t.Context(), &CancelReplaceOrderRequest{
		Symbol:            usdtmTradablePair,
		Side:              "BUY",
		OrderType:         order.Limit.String(),
		CancelReplaceMode: "STOP_ON_FAILURE",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.OpenOrders(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)

	p := usdtmTradablePair
	result, err = e.OpenOrders(t.Context(), p)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelAllOpenOrderOnSymbol(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllOpenOrderOnSymbol(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelAllOpenOrderOnSymbol(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestAllOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.AllOrders(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.AllOrders(t.Context(), &AllOrdersRequest{Symbol: usdtmTradablePair})
	require.False(t, sharedtestvalues.AreAPICredentialsSet(e) && err != nil, err)
	require.False(t, !sharedtestvalues.AreAPICredentialsSet(e) && err == nil && !mockTests, "expecting an error when no keys are set")
	assert.False(t, mockTests && err != nil, err)
}

func TestNewOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewOCOOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OCOOrderRequest{
		TrailingDelta: 1,
	}
	_, err = e.NewOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.NewOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "Buy"
	_, err = e.NewOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.Amount = 0.1
	_, err = e.NewOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	arg.Price = 0.001
	_, err = e.NewOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewOCOOrder(t.Context(), &OCOOrderRequest{
		Symbol:             usdtmTradablePair,
		ListClientOrderID:  "1231231231231",
		Side:               "Buy",
		Amount:             0.1,
		LimitClientOrderID: "3423423",
		Price:              0.001,
		StopPrice:          1234.21,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelOCOOrderList(t *testing.T) {
	t.Parallel()
	_, err := e.CancelOCOOrder(t.Context(), currency.EMPTYPAIR, "", "newderID", "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.CancelOCOOrder(t.Context(), spotTradablePair, "", "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelOCOOrder(t.Context(), spotTradablePair, "", "newderID", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOCOOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetOCOOrders(t.Context(), "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOCOOrders(t.Context(), "123456", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllOCOOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetAllOCOOrders(t.Context(), "", endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllOCOOrders(t.Context(), "", startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOpenOCOList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOpenOCOList(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewOrderUsingSOR(t *testing.T) {
	t.Parallel()
	_, err := e.NewOrderUsingSOR(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &SOROrderRequest{
		TimeInForce: "GTC",
	}
	_, err = e.NewOrderUsingSOR(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = currency.Pair{Base: currency.BTC, Quote: currency.LTC}
	_, err = e.NewOrderUsingSOR(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.NewOrderUsingSOR(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = order.Limit.String()
	_, err = e.NewOrderUsingSOR(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrAmountIsInvalid)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewOrderUsingSOR(t.Context(), &SOROrderRequest{
		Symbol:    currency.Pair{Base: currency.BTC, Quote: currency.LTC},
		Side:      "Buy",
		OrderType: order.Limit.String(),
		Quantity:  0.001,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewOrderUsingSORTest(t *testing.T) {
	t.Parallel()
	_, err := e.NewOrderUsingSORTest(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewOrderUsingSORTest(t.Context(), &SOROrderRequest{
		Symbol:    currency.Pair{Base: currency.BTC, Quote: currency.LTC},
		Side:      "Buy",
		OrderType: order.Limit.String(),
		Quantity:  0.001,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	t.Parallel()
	_, err := e.GetFeeByType(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	feeBuilder := setFeeBuilder()
	result, err := e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err)
	assert.NotNil(t, result)

	if !sharedtestvalues.AreAPICredentialsSet(e) || mockTests {
		assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType)
	} else {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType)
	}
}

func TestGetFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()
	if sharedtestvalues.AreAPICredentialsSet(e) && mockTests {
		// CryptocurrencyTradeFee Basic
		_, err := e.GetFee(t.Context(), feeBuilder)
		require.NoError(t, err)

		// CryptocurrencyTradeFee High quantity
		feeBuilder = setFeeBuilder()
		feeBuilder.Amount = 1000
		feeBuilder.PurchasePrice = 1000
		_, err = e.GetFee(t.Context(), feeBuilder)
		require.NoError(t, err)

		// CryptocurrencyTradeFee IsMaker
		feeBuilder = setFeeBuilder()
		feeBuilder.IsMaker = true
		_, err = e.GetFee(t.Context(), feeBuilder)
		require.NoError(t, err)

		// CryptocurrencyTradeFee Negative purchase price
		feeBuilder = setFeeBuilder()
		feeBuilder.PurchasePrice = -1000
		_, err = e.GetFee(t.Context(), feeBuilder)
		require.NoError(t, err)
	}

	// CryptocurrencyWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err := e.GetFee(t.Context(), feeBuilder)
	require.NoError(t, err)

	// CryptocurrencyDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyDepositFee
	_, err = e.GetFee(t.Context(), feeBuilder)
	require.NoError(t, err)

	// InternationalBankDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	feeBuilder.FiatCurrency = currency.HKD
	_, err = e.GetFee(t.Context(), feeBuilder)
	require.NoError(t, err)

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.HKD
	_, err = e.GetFee(t.Context(), feeBuilder)
	assert.NoError(t, err)
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	expectedResult := exchange.AutoWithdrawCryptoText + " & " + exchange.NoFiatWithdrawalsText
	withdrawPermissions := e.FormatWithdrawPermissions()
	require.Equal(t, expectedResult, withdrawPermissions)
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		Pairs:     currency.Pairs{usdtmTradablePair},
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}
	result, err := e.GetActiveOrders(t.Context(), &getOrdersRequest)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}
	_, err := e.GetOrderHistory(t.Context(), &getOrdersRequest)
	require.ErrorIs(t, err, currency.ErrCurrencyPairsEmpty)

	getOrdersRequest.Pairs = []currency.Pair{
		currency.NewPair(currency.LTC,
			currency.BTC),
	}

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOrderHistory(t.Context(), &getOrdersRequest)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewOrderTest(t *testing.T) {
	t.Parallel()
	err := e.NewOrderTest(t.Context(), nil, false)
	require.ErrorIs(t, err, common.ErrNilPointer)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	err = e.NewOrderTest(t.Context(), &NewOrderRequest{
		Symbol:      currency.NewPair(currency.LTC, currency.BTC),
		Side:        order.Buy.String(),
		TradeType:   order.Limit.String(),
		Price:       0.0025,
		Quantity:    100000,
		TimeInForce: order.GoodTillCancel.String(),
	}, false)
	require.NoError(t, err)

	err = e.NewOrderTest(t.Context(), &NewOrderRequest{
		Symbol:             currency.NewPair(currency.LTC, currency.BTC),
		Side:               order.Sell.String(),
		TradeType:          order.Market.String(),
		Price:              0.0045,
		QuoteOrderQuantity: 10,
	}, true)
	assert.NoError(t, err)
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	if e.IsAPIStreamConnected() {
		startTime = time.Now().Add(-time.Hour * 10)
		endTime = time.Now().Add(-time.Hour)
	}
	_, err := e.GetHistoricTrades(t.Context(), usdtmTradablePair, asset.USDTMarginedFutures, startTime, endTime)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.GetHistoricTrades(t.Context(), spotTradablePair, asset.Spot, time.Time{}, endTime)
	require.NoError(t, err)

	_, err = e.GetHistoricTrades(t.Context(), marginTradablePair, asset.Margin, time.Time{}, endTime)
	require.NoError(t, err)
}

// TestGetAggregatedTradesBatched exercises TestGetAggregatedTradesBatched to ensure our date and limit scanning works correctly
// This test is susceptible to failure if volumes change a lot, during wash trading or zero-fee periods
// In live tests, 45 minutes is expected to return more than 1000 records
func TestGetAggregatedTradesBatched(t *testing.T) {
	t.Parallel()
	t.SkipNow()
	startTime, err := time.Parse(time.RFC3339, "2020-01-02T15:04:05Z")
	require.NoError(t, err)

	expectTime, err := time.Parse(time.RFC3339Nano, "2020-01-02T16:19:04.831Z")
	require.NoError(t, err)

	tests := []struct {
		name string
		// mock test or live test
		mock         bool
		args         *AggregatedTradeRequest
		numExpected  int
		lastExpected time.Time
	}{
		{
			name: "mock batch with timerange",
			mock: true,
			args: &AggregatedTradeRequest{
				Symbol:    usdtmTradablePair,
				StartTime: startTime,
				EndTime:   startTime.Add(75 * time.Minute),
			},
			numExpected:  1012,
			lastExpected: time.Date(2020, 1, 2, 16, 18, 31, int(919*time.Millisecond), time.UTC),
		},
		{
			name: "batch with timerange",
			args: &AggregatedTradeRequest{
				Symbol:    usdtmTradablePair,
				StartTime: startTime,
				EndTime:   startTime.Add(75 * time.Minute),
			},
			numExpected:  12130,
			lastExpected: expectTime,
		},
		{
			name: "mock custom limit with start time set, no end time",
			mock: true,
			args: &AggregatedTradeRequest{
				Symbol:    usdtmTradablePair,
				StartTime: startTime,
				Limit:     1001,
			},
			numExpected:  1001,
			lastExpected: time.Date(2020, 1, 2, 15, 18, 39, int(226*time.Millisecond), time.UTC),
		},
		{
			name: "custom limit with start time set, no end time",
			args: &AggregatedTradeRequest{
				Symbol:    usdtmTradablePair,
				StartTime: time.Date(2020, 11, 18, 23, 0, 28, 921, time.UTC),
				Limit:     1001,
			},
			numExpected:  1001,
			lastExpected: time.Date(2020, 11, 18, 23, 1, 33, int(62*time.Millisecond*10), time.UTC),
		},
		{
			name: "mock recent trades",
			mock: true,
			args: &AggregatedTradeRequest{
				Symbol: usdtmTradablePair,
				Limit:  3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.mock != mockTests {
				t.Skip("mock mismatch, skipping")
			}
			result, err := e.GetAggregatedTrades(t.Context(), tt.args)
			assert.NoError(t, err)

			assert.Len(t, result, tt.numExpected)
			lastTradeTime := result[len(result)-1].TimeStamp
			if !lastTradeTime.Time().Equal(tt.lastExpected) {
				t.Errorf("last trade expected %v, got %v", tt.lastExpected.UTC(), lastTradeTime.Time().UTC())
			}
		})
	}
}

func TestGetAggregatedTradesErrors(t *testing.T) {
	t.Parallel()
	startTime, err := time.Parse(time.RFC3339, "2020-01-02T15:04:05Z")
	require.NoError(t, err)
	tests := []struct {
		name string
		args *AggregatedTradeRequest
	}{
		{
			name: "get recent trades does not support custom limit",
			args: &AggregatedTradeRequest{
				Symbol: usdtmTradablePair,
				Limit:  1001,
			},
		},
		{
			name: "start time and fromId cannot be both set",
			args: &AggregatedTradeRequest{
				Symbol:    usdtmTradablePair,
				StartTime: startTime,
				EndTime:   startTime.Add(75 * time.Minute),
				FromID:    2,
			},
		},
		{
			name: "can't get most recent 5000 (more than 1000 not allowed)",
			args: &AggregatedTradeRequest{
				Symbol: usdtmTradablePair,
				Limit:  5000,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := e.GetAggregatedTrades(t.Context(), tt.args)
			require.Error(t, err)
		})
	}
}

// Any tests below this line have the ability to impact your orders on the exchange. Enable canManipulateRealOrders to run them
// -----------------------------------------------------------------------------------------------------------------------------

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubmitOrder(t.Context(), &order.Submit{
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
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelExchangeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.CancelOrder(t.Context(), &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currency.NewPair(currency.LTC, currency.BTC),
		AssetType: asset.Spot,
	})
	assert.NoError(t, err)
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelAllOrders(t.Context(), &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      spotTradablePair,
		AssetType: asset.Spot,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.CancelAllOrders(t.Context(), &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      optionsTradablePair,
		AssetType: asset.Options,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUpdateAccountBalances(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
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
			result, err := e.UpdateAccountBalances(t.Context(), assetType)
			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

func TestWrapperGetActiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		Pairs:     currency.Pairs{spotTradablePair},
		AssetType: asset.Spot,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		Pairs:     currency.Pairs{coinmTradablePair},
		AssetType: asset.CoinMarginedFutures,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		Pairs:     currency.Pairs{usdtmTradablePair},
		AssetType: asset.USDTMarginedFutures,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Type:      order.AnyType,
		Side:      order.AnySide,
		Pairs:     currency.Pairs{optionsTradablePair},
		AssetType: asset.Options,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWrapperGetOrderHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{AssetType: asset.USDTMarginedFutures})
	assert.Error(t, err)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	require.NoError(t, err)
	result, err := e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		Type:        order.AnyType,
		Side:        order.AnySide,
		FromOrderID: "123",
		Pairs:       currency.Pairs{coinmTradablePair},
		AssetType:   asset.CoinMarginedFutures,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		Type:        order.AnyType,
		Side:        order.AnySide,
		FromOrderID: "123",
		Pairs:       currency.Pairs{usdtmTradablePair},
		AssetType:   asset.USDTMarginedFutures,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	p, err := currency.NewPairFromString("EOS-USDT")
	require.NoError(t, err)
	fPair, err := e.FormatExchangeCurrency(p, asset.CoinMarginedFutures)
	require.NoError(t, err)
	err = e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.CoinMarginedFutures,
		Pair:      fPair,
		OrderID:   "1234",
	})
	require.NoError(t, err)
	p2, err := currency.NewPairFromString("BTC-USDT")
	require.NoError(t, err)
	fpair2, err := e.FormatExchangeCurrency(p2, asset.USDTMarginedFutures)
	require.NoError(t, err)
	err = e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.USDTMarginedFutures,
		Pair:      fpair2,
		OrderID:   "1234",
	})
	require.NoError(t, err)
	err = e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.Options,
		Pair:      fpair2,
		OrderID:   "1234",
	})
	assert.NoError(t, err)
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	tradablePairs, err := e.FetchTradablePairs(t.Context(),
		asset.CoinMarginedFutures)
	require.NoError(t, err)
	require.NotEmpty(t, tradablePairs, "no tradable pairs")
	result, err := e.GetOrderInfo(t.Context(), "123", tradablePairs[0], asset.CoinMarginedFutures)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	p := currency.NewBTCUSDT()
	_, err := e.ModifyOrder(t.Context(), &order.Modify{AssetType: asset.Spot, Pair: p, OrderID: "1234"})
	require.ErrorIs(t, err, common.ErrFunctionNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	fPair, err := e.FormatExchangeCurrency(p, asset.USDTMarginedFutures)
	require.NoError(t, err)
	_, err = e.ModifyOrder(t.Context(), &order.Modify{
		AssetType: asset.USDTMarginedFutures,
		Pair:      fPair,
		OrderID:   "1234",
		Side:      order.Buy,
		Amount:    1,
		Price:     1,
	})
	assert.NoError(t, err)
}

func TestCancelBatchOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelBatchOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrCancelOrderIsNil)

	p := currency.NewBTCUSDT()
	_, err = e.CancelBatchOrders(t.Context(), []order.Cancel{
		{AssetType: asset.USDTMarginedFutures, Pair: p, OrderID: "1"},
		{AssetType: asset.CoinMarginedFutures, Pair: p, OrderID: "2"},
	})
	require.ErrorIs(t, err, errBatchCancelRequiresSamePair)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	fPair, err := e.FormatExchangeCurrency(p, asset.USDTMarginedFutures)
	require.NoError(t, err)
	_, err = e.CancelBatchOrders(t.Context(), []order.Cancel{
		{AssetType: asset.USDTMarginedFutures, Pair: fPair, OrderID: "1234"},
		{AssetType: asset.USDTMarginedFutures, Pair: fPair, OrderID: "5678"},
	})
	assert.NoError(t, err)
}

func TestGetAccountFundingHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccountFundingHistory(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllCoinsInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllCoinsInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.WithdrawCryptocurrencyFunds(t.Context(),
		&withdraw.Request{
			Exchange:    e.Name,
			Amount:      -1,
			Currency:    currency.BTC,
			Description: "WITHDRAW IT ALL",
			Crypto: withdraw.CryptoRequest{
				Address: core.BitcoinDonationAddress,
			},
		})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDepositHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.DepositHistory(t.Context(), &DepositHistoryRequest{Currency: currency.ETH, StartTime: endTime, EndTime: startTime, Limit: 10000})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.DepositHistory(t.Context(), &DepositHistoryRequest{Currency: currency.ETH, StartTime: startTime, EndTime: endTime, Limit: 10000})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWithdrawHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetWithdrawalsHistory(t.Context(), currency.ETH, asset.Spot)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWithdrawFiat(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawFiatFunds(t.Context(), &withdraw.Request{})
	assert.Equal(t, err, common.ErrFunctionNotSupported)
}

func TestWithdrawInternationalBank(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawFiatFundsToInternationalBank(t.Context(), &withdraw.Request{})
	require.Equal(t, err, common.ErrFunctionNotSupported)
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetDepositAddress(t.Context(), currency.USDT, "", currency.BNB.String())
	require.NoError(t, err)
}

func BenchmarkWsHandleData(b *testing.B) {
	b.ReportAllocs()
	data, err := os.ReadFile("testdata/wsHandleData.json")
	require.NoError(b, err)
	lines := bytes.Split(data, []byte("\n"))
	require.Len(b, lines, 8)
	go func() {
		timer := time.NewTimer(time.Second * 5)
		for {
			select {
			case _, ok := <-e.Websocket.DataHandler.C:
				if !ok {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()
	for b.Loop() {
		for x := range lines {
			assert.NoError(b, e.wsHandleData(b.Context(), nil, lines[x]))
		}
	}
}

type FixtureConnection struct{ websocket.Connection }

func (d *FixtureConnection) SendMessageReturnResponse(context.Context, request.EndpointLimit, any, any) ([]byte, error) {
	return []byte(`{"result":null,"id":"%s"}`), nil
}

func (d *FixtureConnection) GetURL() string { return "wss://test" }

func TestSubscribe(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	channels, err := e.generateSubscriptions() // Note: We grab this before it's overwritten by MockWsInstance below
	require.NoError(t, err, "generateSubscriptions must not error")

	exp := []string{"btcusdt@depth@100ms", "btcusdt@kline_1m", "btcusdt@ticker", "btcusdt@trade", "dogeusdt@depth@100ms", "dogeusdt@kline_1m", "dogeusdt@ticker", "dogeusdt@trade"}
	mock := func(tb testing.TB, msg []byte, w *gws.Conn) error {
		tb.Helper()
		var req WsPayload
		require.NoError(tb, json.Unmarshal(msg, &req), "Unmarshal must not error")
		require.ElementsMatch(tb, req.Params, exp, "Params must have correct channels")
		return w.WriteMessage(gws.TextMessage, fmt.Appendf(nil, `{"result":null,"id":"%s"}`, req.ID))
	}
	e = testexch.MockWsInstance[Exchange](t, mockws.CurryWsMockUpgrader(t, mock))

	err = e.Subscribe(t.Context(), &FixtureConnection{}, channels)
	require.NoError(t, err)
	err = e.Unsubscribe(t.Context(), &FixtureConnection{}, channels)
	assert.NoError(t, err)
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
		return w.WriteMessage(gws.TextMessage, fmt.Appendf(nil, `{"result":{"error":"carrots"},"id":"%s"}`, req.ID))
	}
	e := testexch.MockWsInstance[Exchange](t, mockws.CurryWsMockUpgrader(t, mock))

	testexch.SetupWs(t, e)
	conn, err := e.Websocket.GetConnection(asset.Spot)
	require.NoError(t, err)
	require.NotNil(t, conn)

	err = e.Subscribe(t.Context(), conn, channels)
	require.ErrorIs(t, err, websocket.ErrSubscriptionFailure, "Subscribe must error ErrSubscriptionFailure")
	require.ErrorIs(t, err, common.ErrUnknownError, "Subscribe must error errUnknownError")
	assert.ErrorContains(t, err, "carrots", "Subscribe should error containing the carrots")
}

func TestWsTickerUpdate(t *testing.T) {
	t.Parallel()
	keyValues := map[string]string{
		"Ticker":         `{"stream":"btcusdt@ticker","data":{"e":"24hrTicker","E":1580254809477,"s":"BTCUSDT","p":"420.97000000","P":"4.720","w":"9058.27981278","x":"8917.98000000","c":"9338.96000000","Q":"0.17246300","b":"9338.03000000","B":"0.18234600","a":"9339.70000000","A":"0.14097600","o":"8917.99000000","h":"9373.19000000","l":"8862.40000000","v":"72229.53692000","q":"654275356.16896672","O":1580168409456,"C":1580254809456,"F":235294268,"L":235894703,"n":600436}}`,
		"Kline Data":     `{"stream":"btcusdt@kline_1m","data":{ "e": "kline", "E": 1234567891, "s": "BTCUSDT", "k": { "t": 1234000001, "T": 1234600001, "s": "BTCUSDT", "i": "1m", "f": 100, "L": 200, "o": "0.0010", "c": "0.0020", "h": "0.0025", "l": "0.0015", "v": "1000", "n": 100, "x": false, "q": "1.0000", "V": "500", "Q": "0.500", "B": "123456" } }}`,
		"Trade Data":     `{"stream":"btcusdt@trade","data":{ "e": "trade", "E": 1234567891, "s": "BTCUSDT", "t": 12345, "p": "0.001", "q": "100", "b": 88, "a": 50, "T": 1234567851, "m": true, "M": true }}`,
		"Balance Update": `{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{ "e": "balanceUpdate", "E": 1573200697110, "a": "BTC", "d": "100.00000000", "T": 1573200697068}}`,
		"List Status":    `{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{ "e": "listStatus", "E": 1564035303637, "s": "BTCUSDT", "g": 2, "c": "OCO", "l": "EXEC_STARTED", "L": "EXECUTING", "r": "NONE", "C": "F4QN4G8DlFATFlIUQ0cjdD", "T": 1564035303625, "O": [ { "s": "BTCUSDT", "i": 17, "c": "AJYsMjErWJesZvqlJCTUgL" }, { "s": "BTCUSDT", "i": 18, "c": "bfYPSQdLoqAJeNrOr9adzq" } ] }}`,
	}
	for key, val := range keyValues {
		err := e.wsHandleData(t.Context(), nil, []byte(val))
		require.NoErrorf(t, err, "%s: %v", key, err)
	}
}

func TestWsDepthUpdate(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	e.setupOrderbookManager(t.Context())
	seedLastUpdateID := int64(161)
	book := OrderBook{
		Asks: orderbook.LevelsArrayPriceAmount(orderbook.Levels{
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
		}),
		Bids: orderbook.LevelsArrayPriceAmount(orderbook.Levels{
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
		}),
		LastUpdateID: seedLastUpdateID,
	}

	update1 := []byte(`{"stream":"btcusdt@depth","data":{ "e": "depthUpdate", "E": 1234567881, "s": "BTCUSDT", "U": 157, "u": 160, "b": [ ["6621.45", "0.3"] ], "a": [ ["6622.46", "1.5"] ] }}`)

	p := currency.NewPairWithDelimiter("BTC", "USDT", "-")
	err := e.SeedLocalCacheWithBook(p, &book)
	require.NoError(t, err)

	if err := e.wsHandleData(t.Context(), nil, update1); err != nil {
		t.Fatal(err)
	}

	e.obm.state[currency.BTC][currency.USDT][asset.Spot].fetchingBook = false

	ob, err := e.Websocket.Orderbook.GetOrderbook(p, asset.Spot)
	require.NoError(t, err)

	exp, got := seedLastUpdateID, ob.LastUpdateID
	require.Equalf(t, exp, got, "Last update id of orderbook for old update. Exp: %d, got: %d", exp, got)
	expAmnt, gotAmnt := 2.3, ob.Asks[2].Amount
	require.Equalf(t, expAmnt, gotAmnt, "Ask altered by outdated update. Exp: %f, got %f", expAmnt, gotAmnt)
	expAmnt, gotAmnt = 0.163526, ob.Bids[1].Amount
	require.Equalf(t, expAmnt, gotAmnt, "Bid altered by outdated update. Exp: %f, got %f", expAmnt, gotAmnt)

	update2 := []byte(`{"stream":"btcusdt@depth","data":{ "e": "depthUpdate", "E": 1234567892, "s": "BTCUSDT", "U": 161, "u": 165, "b": [ ["6621.45", "0.163526"] ], "a": [ ["6622.46", "2.3"], ["6622.47", "1.9"] ] }}`)

	if err = e.wsHandleData(t.Context(), nil, update2); err != nil {
		t.Error(err)
	}

	ob, err = e.Websocket.Orderbook.GetOrderbook(p, asset.Spot)
	require.NoError(t, err)
	exp, got = int64(165), ob.LastUpdateID
	require.Equalf(t, exp, got, "Unexpected Last update id of orderbook for new update. Exp: %d, got: %d", exp, got)
	expAmnt, gotAmnt = 2.3, ob.Asks[2].Amount
	require.Equalf(t, expAmnt, gotAmnt, "Unexpected Ask amount. Exp: %f, got %f", expAmnt, gotAmnt)
	expAmnt, gotAmnt = 1.9, ob.Asks[3].Amount
	require.Equalf(t, expAmnt, gotAmnt, "Unexpected Ask amount. Exp: %f, got %f", exp, got)
	expAmnt, gotAmnt = 0.163526, ob.Bids[1].Amount
	require.Equalf(t, expAmnt, gotAmnt, "Unexpected Bid amount. Exp: %f, got %f", exp, got)

	// reset order book sync status
	e.obm.state[currency.BTC][currency.USDT][asset.Spot].lastUpdateID = 0
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
		require.Equalf(t, result, testCases[i].Result, "Expected: %v, received: %v", testCases[i].Result, result)
	}
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime(true)
	if mockTests {
		startTime, endTime = time.UnixMilli(1774479176769), time.UnixMilli(1774997576769)
	}
	for assetType, pair := range assetToTradablePairMap {
		result, err := e.GetHistoricCandles(t.Context(), pair, assetType, kline.FiveMin, startTime, endTime)
		require.NoErrorf(t, err, "%v %v", assetType, err)
		require.NotNil(t, result)
	}
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime(true)
	if mockTests {
		startTime, endTime = time.UnixMilli(1774479176769), time.UnixMilli(1774997576769)
	}
	for assetType, pair := range assetToTradablePairMap {
		result, err := e.GetHistoricCandlesExtended(t.Context(), pair, assetType, kline.FiveMin, startTime, endTime)
		require.NoErrorf(t, err, "asset type: %v error: %v", assetType, err)
		assert.NotNil(t, result)
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
		t.Run(tc.output, func(t *testing.T) {
			t.Parallel()
			ret := e.FormatExchangeKlineInterval(tc.interval)
			require.Equalf(t, ret, tc.output, "unexpected result return expected: %v received: %v", tc.output, ret)
		})
	}
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	pair := usdtmTradablePair
	result, err := e.GetRecentTrades(t.Context(), pair, asset.Spot)
	require.NoError(t, err)
	assert.NotNil(t, result)
	result, err = e.GetRecentTrades(t.Context(),
		pair, asset.USDTMarginedFutures)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetRecentTrades(t.Context(), coinmTradablePair, asset.CoinMarginedFutures)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAvailableTransferChains(t.Context(), currency.BTC)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSeedLocalCache(t *testing.T) {
	t.Parallel()
	err := e.SeedLocalCache(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
}

func TestGenerateSubscriptions(t *testing.T) {
	t.Parallel()
	exp := subscription.List{}
	pairs, err := e.GetEnabledPairs(asset.Spot)
	require.NoError(t, err)
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

func TestProcessOrderbookUpdate(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	e.setupOrderbookManager(t.Context())
	p := currency.NewBTCUSDT()
	var depth WebsocketDepthStream
	err := json.Unmarshal([]byte(`{"E":1608001030784,"U":7145637266,"a":[["19455.19000000","0.59490200"],["19455.37000000","0.00000000"],["19456.11000000","0.00000000"],["19456.16000000","0.00000000"],["19458.67000000","0.06400000"],["19460.73000000","0.05139800"],["19461.43000000","0.00000000"],["19464.59000000","0.00000000"],["19466.03000000","0.45000000"],["19466.36000000","0.00000000"],["19508.67000000","0.00000000"],["19572.96000000","0.00217200"],["24386.00000000","0.00256600"]],"b":[["19455.18000000","2.94649200"],["19453.15000000","0.01233600"],["19451.18000000","0.00000000"],["19446.85000000","0.11427900"],["19446.74000000","0.00000000"],["19446.73000000","0.00000000"],["19444.45000000","0.14937800"],["19426.75000000","0.00000000"],["19416.36000000","0.36052100"]],"e":"depthUpdate","s":"BTCUSDT","u":7145637297}`),
		&depth)
	require.NoError(t, err)

	err = e.obm.stageWsUpdate(&depth, p, asset.Spot)
	require.NoError(t, err)

	err = e.obm.fetchBookViaREST(p)
	require.NoError(t, err)

	err = e.obm.cleanup(p)
	require.NoError(t, err)

	// reset order book sync status
	e.obm.state[currency.BTC][currency.USDT][asset.Spot].lastUpdateID = 0
}

func TestUFuturesHistoricalTrades(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesHistoricalTrades(t.Context(), currency.EMPTYPAIR, "", 5)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UFuturesHistoricalTrades(t.Context(), usdtmTradablePair, "", 5)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.UFuturesHistoricalTrades(t.Context(), usdtmTradablePair, "", 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFetchCoinMarginExchangeLimits(t *testing.T) {
	t.Parallel()
	result, err := e.FetchCoinMarginExchangeLimits(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetExchangeOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	assetTypes := e.GetAssetTypes(true)
	for a := range assetTypes {
		err := e.UpdateOrderExecutionLimits(t.Context(), assetTypes[a])
		require.NoErrorf(t, err, "%v: asset type: %v", err, assetTypes[a])
	}

	err := e.UpdateOrderExecutionLimits(t.Context(), asset.Binary)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	l, err := e.GetOrderExecutionLimits(asset.CoinMarginedFutures, coinmTradablePair)
	require.NoError(t, err)
	require.NotEmpty(t, l, "exchange limit must be loaded")

	err = l.Validate(0.000001, 0.1, order.Limit)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	err = l.Validate(0.01, 1, order.Limit)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)
}

func TestWsOrderExecutionReport(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
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
		TimeInForce:          order.GoodTillCancel,
	}
	// empty the channel. otherwise mock_test will fail
drain:
	for {
		select {
		case <-e.Websocket.DataHandler.C:
		default:
			break drain
		}
	}

	err := e.wsHandleData(t.Context(), nil, payload)
	if err != nil {
		t.Fatal(err)
	}
	res := <-e.Websocket.DataHandler.C
	switch r := res.Data.(type) {
	case *order.Detail:
		// The WebSocket handler returns two order details for a single symbol:
		// one for spot and one for margin. To avoid mismatches due to asset type
		// precedence, we align the expected asset type with the received one.
		if r.AssetType == asset.Margin {
			expectedResult.AssetType = asset.Margin
		}
		require.Truef(t, reflect.DeepEqual(expectedResult, *r), "results do not match:\nexpected: %v\nreceived: %v", expectedResult, *r)
	default:
		t.Fatalf("expected type order.Detail, found %T", res)
	}

	payload = []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"executionReport","E":1616633041556,"s":"BTCUSDT","c":"YeULctvPAnHj5HXCQo9Mob","S":"BUY","o":"LIMIT","f":"GTC","q":"0.00028600","p":"52436.85000000","P":"0.00000000","F":"0.00000000","g":-1,"C":"","x":"TRADE","X":"FILLED","r":"NONE","i":5341783271,"l":"0.00028600","z":"0.00028600","L":"52436.85000000","n":"0.00000029","N":"BTC","T":1616633041555,"t":726946523,"I":11390206312,"w":false,"m":false,"M":true,"O":1616633041555,"Z":"14.99693910","Y":"14.99693910","Q":"0.00000000","W":1616633041555}}`)
	err = e.wsHandleData(t.Context(), nil, payload)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWsOutboundAccountPosition(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"stream":"jTfvpakT2yT0hVIo5gYWVihZhdM2PrBgJUZ5PyfZ4EVpCkx4Uoxk5timcrQc","data":{"e":"outboundAccountPosition","E":1616628815745,"u":1616628815745,"B":[{"a":"BTC","f":"0.00225109","l":"0.00123000"},{"a":"BNB","f":"0.00000000","l":"0.00000000"},{"a":"USDT","f":"54.43390661","l":"0.00000000"}]}}`)
	require.NoError(t, e.wsHandleData(t.Context(), nil, payload))
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
			name:  "coinmarginedfutures-btcusd_perp",
			pair:  currency.NewPair(currency.BTC, currency.NewCode("USD_PERP")),
			asset: asset.CoinMarginedFutures,
		},
		{
			name:  "coinmarginedfutures-btcusd_211231",
			pair:  currency.NewPair(currency.BTC, currency.NewCode("USD_211231")),
			asset: asset.CoinMarginedFutures,
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
	}
	for i := range testerinos {
		tt := testerinos[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := e.FormatExchangeCurrency(tt.pair, tt.asset)
			require.NoError(t, err)
			require.Equal(t, tt.expectedDelimiter, result.Delimiter)
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
			pair:           currency.NewPairWithDelimiter("BTC", "USD_PERP", ""),
			asset:          asset.CoinMarginedFutures,
			expectedString: "BTCUSD_PERP",
		},
		{
			name:           "coinmarginedfutures-BTCUSD_211231",
			pair:           currency.NewPair(currency.BTC, currency.NewCode("USD_211231")),
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
			name:           "usdtmarginedfutures-BTCUSDT",
			pair:           currency.NewBTCUSDT(),
			asset:          asset.USDTMarginedFutures,
			expectedString: "BTCUSDT",
		},
	}
	for _, tt := range testerinos {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := e.FormatSymbol(tt.pair, tt.asset)
			require.NoError(t, err)
			require.Equal(t, tt.expectedString, result)
		})
	}
}

func TestFormatUSDTMarginedFuturesPair(t *testing.T) {
	t.Parallel()
	pairFormat := currency.PairFormat{Uppercase: true}
	resp := e.formatUSDTMarginedFuturesPair(currency.NewPair(currency.DOGE, currency.USDT), pairFormat)
	require.Equal(t, "DOGEUSDT", resp.String())

	resp = e.formatUSDTMarginedFuturesPair(currency.NewPair(currency.DOGE, currency.NewCode("1234567890")), pairFormat)
	assert.Equal(t, "DOGE_1234567890", resp.String())
}

func TestFetchExchangeLimits(t *testing.T) {
	t.Parallel()
	l, err := e.FetchExchangeLimits(t.Context(), asset.Spot)
	require.NoError(t, err)
	require.NotEmpty(t, l, "Should get some limits back")

	l, err = e.FetchExchangeLimits(t.Context(), asset.Margin)
	require.NoError(t, err)
	require.NotEmpty(t, l, "Should get some limits back")

	_, err = e.FetchExchangeLimits(t.Context(), asset.Futures)
	require.ErrorIs(t, err, asset.ErrNotSupported, "FetchExchangeLimits must error on other asset types")
}

// TestExchangeLimitsFromSymbols covers the spot/margin limit parsing with in-memory fixtures so value
// correctness is asserted independently of the mock recording, whose symbols slice is truncated by the
// recording slice limit and may not contain the pair under test.
func TestExchangeLimitsFromSymbols(t *testing.T) {
	t.Parallel()
	symbols := []*SymbolInfo{
		{
			BaseAsset:      "BTC",
			QuoteAsset:     "USDT",
			PermissionSets: [][]string{{"SPOT", "MARGIN"}},
			Filters: []*filterData{
				{FilterType: priceFilter, MinPrice: 0.01, MaxPrice: 1000000, TickSize: 0.01},
				{FilterType: lotSizeFilter, MinQuantity: 0.0001, MaxQuantity: 9000, StepSize: 0.0001},
				{FilterType: icebergPartsFilter, Limit: 10},
				{FilterType: marketLotSizeFilter, MinQuantity: 0, MaxQuantity: 100, StepSize: 0.0001},
				{FilterType: maxNumOrdersFilter, MaxNumberOrders: 200, MaxNumberAlgoOrders: 5},
				{FilterType: notionalFilter, MinNotional: 5},
			},
		},
		{ // MARGIN-only symbol must be excluded from a spot lookup
			BaseAsset:      "ETH",
			QuoteAsset:     "USDT",
			PermissionSets: [][]string{{"MARGIN"}},
			Filters:        []*filterData{{FilterType: priceFilter, MinPrice: 0.01, MaxPrice: 100, TickSize: 0.01}},
		},
	}

	l, err := e.exchangeLimitsFromSymbols(asset.Spot, symbols)
	require.NoError(t, err, "exchangeLimitsFromSymbols must not error")
	require.Len(t, l, 1, "only the SPOT-permitted symbol must be returned")
	got := l[0]
	assert.Equal(t, currency.BTC.Item, got.Key.Base, "limit base should be BTC")
	assert.Equal(t, currency.USDT.Item, got.Key.Quote, "limit quote should be USDT")
	assert.Equal(t, 0.01, got.MinPrice, "MinPrice should match price filter")
	assert.Equal(t, 1000000.0, got.MaxPrice, "MaxPrice should match price filter")
	assert.Equal(t, 0.01, got.PriceStepIncrementSize, "PriceStepIncrementSize should match tick size")
	assert.Equal(t, 0.0001, got.MinimumBaseAmount, "MinimumBaseAmount should match lot size")
	assert.Equal(t, 9000.0, got.MaximumBaseAmount, "MaximumBaseAmount should match lot size")
	assert.Equal(t, int64(10), got.MaxIcebergParts, "MaxIcebergParts should match iceberg filter")
	assert.Equal(t, int64(200), got.MaxTotalOrders, "MaxTotalOrders should match max num orders filter")
	assert.Equal(t, 5.0, got.MinNotional, "MinNotional should match notional filter")

	l, err = e.exchangeLimitsFromSymbols(asset.Margin, symbols)
	require.NoError(t, err, "exchangeLimitsFromSymbols must not error for margin")
	assert.Len(t, l, 2, "both MARGIN-permitted symbols should be returned")

	_, err = e.exchangeLimitsFromSymbols(asset.Spot, []*SymbolInfo{{BaseAsset: "BT C", QuoteAsset: "USDT", PermissionSets: [][]string{{"SPOT"}}}})
	require.Error(t, err, "an invalid currency string must return an error")
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	for _, a := range e.GetAssetTypes(true) {
		t.Run(a.String(), func(t *testing.T) {
			t.Parallel()
			require.NoError(t, e.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must not error")
			pairs, err := e.CurrencyPairs.GetPairs(a, true)
			require.NoError(t, err, "GetPairs must not error")
			for _, p := range pairs {
				l, err := e.GetOrderExecutionLimits(a, p)
				if errors.Is(err, limits.ErrOrderLimitNotFound) {
					// The mock exchangeInfo recording is truncated by the recording slice limit, so the
					// enabled pair may be absent. Parsing correctness is covered by TestExchangeLimitsFromSymbols.
					continue
				}
				require.NoError(t, err, "GetOrderExecutionLimits must not error")
				assert.Positive(t, l.MinPrice, "MinPrice should be positive")
				assert.Positive(t, l.MaxPrice, "MaxPrice should be positive")
				assert.Positive(t, l.PriceStepIncrementSize, "PriceStepIncrementSize should be positive")
				assert.Positive(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive")
				assert.Positive(t, l.MaximumBaseAmount, "MaximumBaseAmount should be positive")
				assert.Positive(t, l.AmountStepIncrementSize, "AmountStepIncrementSize should be positive")
				assert.Positive(t, l.MarketMaxQty, "MarketMaxQty should be positive")
				assert.Positive(t, l.MaxTotalOrders, "MaxTotalOrders should be positive")
				switch a {
				case asset.Spot, asset.Margin:
					assert.Positive(t, l.MaxIcebergParts, "MaxIcebergParts should be positive")
				case asset.USDTMarginedFutures:
					assert.Positive(t, l.MinNotional, "MinNotional should be positive")
					assert.Positive(t, l.MultiplierUp, "MultiplierUp should be positive")
					assert.Positive(t, l.MultiplierDown, "MultiplierDown should be positive")
					assert.Positive(t, l.MarketMinQty, "MarketMinQty should be positive")
					assert.Positive(t, l.MarketStepIncrementSize, "MarketStepIncrementSize should be positive")
				case asset.CoinMarginedFutures:
					assert.Positive(t, l.MultiplierUp, "MultiplierUp should be positive")
					assert.Positive(t, l.MultiplierDown, "MultiplierDown should be positive")
					assert.Positive(t, l.MarketMinQty, "MarketMinQty should be positive")
					assert.Positive(t, l.MarketStepIncrementSize, "MarketStepIncrementSize should be positive")
					assert.Positive(t, l.MaxAlgoOrders, "MaxAlgoOrders should be positive")
				}
			}
		})
	}
	t.Run("unsupported asset", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, e.UpdateOrderExecutionLimits(t.Context(), asset.Binary), asset.ErrNotSupported)
	})
}

func TestGetHistoricalFundingRates(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 currency.NewBTCUSDT(),
		StartDate:            startTime,
		EndDate:              endTime,
		IncludePayments:      true,
		IncludePredictedRate: true,
	})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.USDTMarginedFutures,
		Pair:            currency.NewBTCUSDT(),
		StartDate:       startTime,
		EndDate:         endTime,
		PaymentCurrency: currency.DOGE,
	})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported)

	r := &fundingrate.HistoricalRatesRequest{
		Asset:     asset.USDTMarginedFutures,
		Pair:      currency.NewBTCUSDT(),
		StartDate: startTime,
		EndDate:   endTime,
	}
	if sharedtestvalues.AreAPICredentialsSet(e) {
		r.IncludePayments = true
	}
	result, err := e.GetHistoricalFundingRates(t.Context(), r)
	require.NoError(t, err)
	assert.NotNil(t, result)

	r.Asset = asset.CoinMarginedFutures
	r.Pair = coinmTradablePair
	result, err = e.GetHistoricalFundingRates(t.Context(), r)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLatestFundingRates(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup must not error for local exchange instance")
	if mockTests {
		require.NoError(t, testexch.MockHTTPInstance(e), "MockHTTPInstance must not error for local exchange instance")
	}
	testexch.UpdatePairsOnce(t, e)
	usdtPerpetualPair := currency.NewBTCUSDT()
	_, err := e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 usdtPerpetualPair,
		IncludePredictedRate: true,
	})
	require.ErrorIs(t, err, common.ErrFunctionNotSupported)
	err = e.CurrencyPairs.EnablePair(asset.USDTMarginedFutures, usdtPerpetualPair)
	require.True(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled), err)

	result, err := e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  usdtPerpetualPair,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.CoinMarginedFutures,
		Pair:  coinmTradablePair,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestIsPerpetualFutureCurrency(t *testing.T) {
	t.Parallel()
	is, err := e.IsPerpetualFutureCurrency(asset.Binary, usdtmTradablePair)
	require.NoError(t, err)
	require.False(t, is)

	is, err = e.IsPerpetualFutureCurrency(asset.CoinMarginedFutures, usdtmTradablePair)
	require.NoError(t, err)
	require.False(t, is)
	is, err = e.IsPerpetualFutureCurrency(asset.CoinMarginedFutures, currency.NewPair(currency.BTC, currency.PERP))
	require.NoError(t, err)
	require.True(t, is)

	is, err = e.IsPerpetualFutureCurrency(asset.USDTMarginedFutures, usdtmTradablePair)
	require.NoError(t, err)
	require.True(t, is)

	is, err = e.IsPerpetualFutureCurrency(asset.USDTMarginedFutures, currency.NewPair(currency.BTC, currency.PERP))
	require.NoError(t, err)
	assert.False(t, is)
}

func TestGetUserMarginInterestHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetUserMarginInterestHistory(t.Context(), &GetUserMarginInterestHistoryRequest{AssetName: currency.USDT, IsolatedSymbol: usdtmTradablePair, StartTime: endTime, EndTime: startTime, Current: 1, Size: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserMarginInterestHistory(t.Context(), &GetUserMarginInterestHistoryRequest{AssetName: currency.USDT, IsolatedSymbol: usdtmTradablePair, StartTime: startTime, EndTime: endTime, Current: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetForceLiquidiationRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetForceLiquidiationRecord(t.Context(), &GetForceLiquidiationRecordRequest{StartTime: endTime, EndTime: startTime, IsolatedSymbol: usdtmTradablePair, Size: 12})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetForceLiquidiationRecord(t.Context(), &GetForceLiquidiationRecordRequest{StartTime: startTime, EndTime: endTime, IsolatedSymbol: usdtmTradablePair, Size: 12})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCrossMarginAccountDetail(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCrossMarginAccountDetail(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountsOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginAccountsOrder(t.Context(), currency.EMPTYPAIR, "", "112233424", false)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetMarginAccountsOrder(t.Context(), usdtmTradablePair, "", "", false)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountsOrder(t.Context(), usdtmTradablePair, "", "112233424", false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountsOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountsOpenOrders(t.Context(), assetToTradablePairMap[asset.Margin], false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountAllOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginAccountAllOrders(t.Context(), &GetMarginAccountAllOrdersRequest{IsIsolated: true, Limit: 20})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	startTime, endTime := getTime()
	_, err = e.GetMarginAccountAllOrders(t.Context(), &GetMarginAccountAllOrdersRequest{Symbol: assetToTradablePairMap[asset.Margin], IsIsolated: true, StartTime: endTime, EndTime: startTime, Limit: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountAllOrders(t.Context(), &GetMarginAccountAllOrdersRequest{Symbol: assetToTradablePairMap[asset.Margin], IsIsolated: true, StartTime: startTime, EndTime: endTime, OrderID: "1", Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetAssetsMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	is, err := e.GetAssetsMode(t.Context())
	require.NoError(t, err)

	err = e.SetAssetsMode(t.Context(), !is)
	require.NoError(t, err)

	err = e.SetAssetsMode(t.Context(), is)
	assert.NoError(t, err)
}

func TestGetAssetsMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAssetsMode(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCollateralMode(t *testing.T) {
	t.Parallel()
	_, err := e.GetCollateralMode(t.Context(), asset.Spot)
	require.ErrorIs(t, err, asset.ErrNotSupported)
	_, err = e.GetCollateralMode(t.Context(), asset.CoinMarginedFutures)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.GetCollateralMode(t.Context(), asset.USDTMarginedFutures)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetCollateralMode(t *testing.T) {
	t.Parallel()
	err := e.SetCollateralMode(t.Context(), asset.USDTMarginedFutures, collateral.PortfolioMode)
	require.ErrorIs(t, err, order.ErrCollateralInvalid)
	err = e.SetCollateralMode(t.Context(), asset.Spot, collateral.SingleMode)
	require.ErrorIs(t, err, asset.ErrNotSupported)
	err = e.SetCollateralMode(t.Context(), asset.CoinMarginedFutures, collateral.SingleMode)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err = e.SetCollateralMode(t.Context(), asset.USDTMarginedFutures, collateral.MultiMode)
	require.NoError(t, err)
}

func TestChangePositionMargin(t *testing.T) {
	t.Parallel()
	_, err := e.ChangePositionMargin(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &margin.PositionChangeRequest{}
	_, err = e.ChangePositionMargin(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Pair = currency.NewBTCUSDT()
	_, err = e.ChangePositionMargin(t.Context(), arg)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	arg.Asset = asset.USDTMarginedFutures
	_, err = e.ChangePositionMargin(t.Context(), arg)
	require.ErrorIs(t, err, margin.ErrNewAllocatedMarginRequired)

	arg.NewAllocatedMargin = 1333337
	_, err = e.ChangePositionMargin(t.Context(), arg)
	require.ErrorIs(t, err, margin.ErrOriginalPositionMarginRequired)

	arg.OriginalAllocatedMargin = 1337
	arg.MarginType = margin.Multi
	_, err = e.ChangePositionMargin(t.Context(), arg)
	require.ErrorIs(t, err, margin.ErrMarginTypeUnsupported)

	arg.MarginType = margin.Isolated
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangePositionMargin(t.Context(), &margin.PositionChangeRequest{
		Pair:                    currency.NewBTCUSDT(),
		Asset:                   asset.USDTMarginedFutures,
		MarginType:              margin.Isolated,
		OriginalAllocatedMargin: 1337,
		NewAllocatedMargin:      1333337,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPositionSummary(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset:          asset.Spot,
		Pair:           coinmTradablePair,
		UnderlyingPair: currency.NewBTCUSD(),
	})
	require.ErrorIs(t, err, asset.ErrNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	bb := currency.NewBTCUSDT()
	result, err := e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  bb,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	bb.Quote = currency.BUSD
	result, err = e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  bb,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	bb.Quote = currency.USD
	result, err = e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{
		Asset:          asset.CoinMarginedFutures,
		Pair:           coinmTradablePair,
		UnderlyingPair: bb,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesPositionOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesPositionOrders(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &futures.PositionsRequest{
		RespectOrderHistoryLimits: true,
	}
	_, err = e.GetFuturesPositionOrders(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairsEmpty)

	arg.Pairs = []currency.Pair{currency.NewBTCUSDT()}
	_, err = e.GetFuturesPositionOrders(t.Context(), arg)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesPositionOrders(t.Context(), &futures.PositionsRequest{
		Asset:                     asset.USDTMarginedFutures,
		Pairs:                     []currency.Pair{currency.NewBTCUSDT()},
		StartDate:                 time.Now().Add(-time.Hour * 24 * 70),
		RespectOrderHistoryLimits: true,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetFuturesPositionOrders(t.Context(), &futures.PositionsRequest{
		Asset:                     asset.CoinMarginedFutures,
		Pairs:                     []currency.Pair{coinmTradablePair},
		StartDate:                 time.Now().Add(time.Hour * 24 * -70),
		RespectOrderHistoryLimits: true,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetMarginType(t *testing.T) {
	t.Parallel()
	err := e.SetMarginType(t.Context(), asset.Spot, usdtmTradablePair, margin.Isolated)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err = e.SetMarginType(t.Context(), asset.USDTMarginedFutures, usdtmTradablePair, margin.Isolated)
	require.NoError(t, err)

	err = e.SetMarginType(t.Context(), asset.CoinMarginedFutures, coinmTradablePair, margin.Isolated)
	assert.NoError(t, err)
}

func TestGetLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLeverage(t.Context(), asset.USDTMarginedFutures, currency.NewBTCUSDT(), 0, order.UnknownSide)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetLeverage(t.Context(), asset.CoinMarginedFutures, coinmTradablePair, 0, order.UnknownSide)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.SetLeverage(t.Context(), asset.USDTMarginedFutures, currency.NewBTCUSDT(), margin.Multi, 5, order.UnknownSide)
	require.NoError(t, err)
	err = e.SetLeverage(t.Context(), asset.CoinMarginedFutures, coinmTradablePair, margin.Multi, 5, order.UnknownSide)
	require.NoError(t, err)
}

func TestGetCryptoLoansIncomeHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.CryptoLoanIncomeHistory(t.Context(), &CryptoLoanIncomeHistoryRequest{Curr: currency.USDT, StartTime: endTime, EndTime: startTime, Limit: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CryptoLoanIncomeHistory(t.Context(), &CryptoLoanIncomeHistoryRequest{Curr: currency.USDT, StartTime: startTime, EndTime: endTime, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanBorrow(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanBorrow(t.Context(), &CryptoLoanBorrowRequest{LoanAmount: 1000, CollateralCoin: currency.BTC, CollateralAmount: 1, LoanTerm: 7})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.CryptoLoanBorrow(t.Context(), &CryptoLoanBorrowRequest{LoanCoin: currency.USDT, LoanAmount: 1000, CollateralAmount: 1, LoanTerm: 7})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.CryptoLoanBorrow(t.Context(), &CryptoLoanBorrowRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC, CollateralAmount: 1})
	require.ErrorIs(t, err, errLoanTermMustBeSet)
	_, err = e.CryptoLoanBorrow(t.Context(), &CryptoLoanBorrowRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC, LoanTerm: 7})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CryptoLoanBorrow(t.Context(), &CryptoLoanBorrowRequest{LoanCoin: currency.USDT, LoanAmount: 1000, CollateralCoin: currency.BTC, CollateralAmount: 1, LoanTerm: 7})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanBorrowHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.CryptoLoanBorrowHistory(t.Context(), &CryptoLoanBorrowHistoryRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CryptoLoanBorrowHistory(t.Context(), &CryptoLoanBorrowHistoryRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC, StartTime: startTime, EndTime: endTime, Current: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanOngoingOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CryptoLoanOngoingOrders(t.Context(), &CryptoLoanOngoingOrdersRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanRepay(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanRepay(t.Context(), 0, 1000, 1, false)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	_, err = e.CryptoLoanRepay(t.Context(), 42069, 0, 1, false)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CryptoLoanRepay(t.Context(), 42069, 1000, 1, false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanRepaymentHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.CryptoLoanRepaymentHistory(t.Context(), &CryptoLoanRepaymentHistoryRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CryptoLoanRepaymentHistory(t.Context(), &CryptoLoanRepaymentHistoryRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC, StartTime: startTime, EndTime: endTime, Current: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanAdjustLTV(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanAdjustLTV(t.Context(), 0, true, 1)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.CryptoLoanAdjustLTV(t.Context(), 42069, true, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CryptoLoanAdjustLTV(t.Context(), 42069, true, 1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanLTVAdjustmentHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.CryptoLoanLTVAdjustmentHistory(t.Context(), &CryptoLoanLTVAdjustmentHistoryRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CryptoLoanLTVAdjustmentHistory(t.Context(), &CryptoLoanLTVAdjustmentHistoryRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC, StartTime: startTime, EndTime: endTime, Current: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CryptoLoanAssetsData(t.Context(), currency.EMPTYCODE, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanCollateralAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CryptoLoanCollateralAssetsData(t.Context(), currency.EMPTYCODE, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanCheckCollateralRepayRate(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.EMPTYCODE, currency.BNB, 69)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.EMPTYCODE, 69)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.BNB, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CryptoLoanCheckCollateralRepayRate(t.Context(), currency.BUSD, currency.BNB, 69)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCryptoLoanCustomiseMarginCall(t *testing.T) {
	t.Parallel()
	_, err := e.CryptoLoanCustomiseMarginCall(t.Context(), 0, currency.BTC, 0)
	require.ErrorIs(t, err, errMarginCallValueRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CryptoLoanCustomiseMarginCall(t.Context(), 1337, currency.BTC, .70)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanBorrow(t *testing.T) {
	t.Parallel()
	_, err := e.FlexibleLoanBorrow(t.Context(), currency.EMPTYCODE, currency.USDC, 1, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.EMPTYCODE, 1, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.USDC, 0, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FlexibleLoanBorrow(t.Context(), currency.ATOM, currency.USDC, 1, 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanOngoingOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FlexibleLoanOngoingOrders(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE, 0, 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanBorrowHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.FlexibleLoanBorrowHistory(t.Context(), &FlexibleLoanBorrowHistoryRequest{StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FlexibleLoanBorrowHistory(t.Context(), &FlexibleLoanBorrowHistoryRequest{StartTime: startTime, EndTime: endTime, Current: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanRepay(t *testing.T) {
	t.Parallel()
	_, err := e.FlexibleLoanRepay(t.Context(), &FlexibleLoanRepayRequest{CollateralCoin: currency.BTC, Amount: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FlexibleLoanRepay(t.Context(), &FlexibleLoanRepayRequest{LoanCoin: currency.USDT, Amount: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FlexibleLoanRepay(t.Context(), &FlexibleLoanRepayRequest{LoanCoin: currency.USDT, CollateralCoin: currency.BTC})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FlexibleLoanRepay(t.Context(), &FlexibleLoanRepayRequest{LoanCoin: currency.ATOM, CollateralCoin: currency.USDC, Amount: 1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanRepayHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.FlexibleLoanRepayHistory(t.Context(), &FlexibleLoanRepayHistoryRequest{StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FlexibleLoanRepayHistory(t.Context(), &FlexibleLoanRepayHistoryRequest{StartTime: startTime, EndTime: endTime, Current: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanCollateralRepayment(t *testing.T) {
	t.Parallel()
	_, err := e.FlexibleLoanCollateralRepayment(t.Context(), currency.EMPTYCODE, currency.USDT, 1000, true)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FlexibleLoanCollateralRepayment(t.Context(), currency.BTC, currency.EMPTYCODE, 1000, true)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FlexibleLoanCollateralRepayment(t.Context(), currency.BTC, currency.USDT, 0, true)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FlexibleLoanCollateralRepayment(t.Context(), currency.BTC, currency.USDT, 1000, true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCheckCollateralRepayRate(t *testing.T) {
	t.Parallel()
	_, err := e.CheckCollateralRepayRate(t.Context(), currency.EMPTYCODE, currency.USDT)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.CheckCollateralRepayRate(t.Context(), currency.BTC, currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CheckCollateralRepayRate(t.Context(), currency.BTC, currency.USDT)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFlexibleLoanLiquidiationHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFlexibleLoanLiquidiationHistory(t.Context(), &GetFlexibleLoanLiquidiationHistoryRequest{LoanCoin: currency.BTC, StartTime: endTime, EndTime: startTime, Limit: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFlexibleLoanLiquidiationHistory(t.Context(), &GetFlexibleLoanLiquidiationHistoryRequest{LoanCoin: currency.BTC, StartTime: startTime, EndTime: endTime, Current: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanAdjustLTV(t *testing.T) {
	t.Parallel()
	_, err := e.FlexibleLoanAdjustLTV(t.Context(), currency.EMPTYCODE, currency.BTC, 1, true)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FlexibleLoanAdjustLTV(t.Context(), currency.USDT, currency.EMPTYCODE, 1, true)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.FlexibleLoanAdjustLTV(t.Context(), currency.USDT, currency.BTC, 0, true)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FlexibleLoanAdjustLTV(t.Context(), currency.USDT, currency.BTC, 1, true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanLTVAdjustmentHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.FlexibleLoanLTVAdjustmentHistory(t.Context(), &FlexibleLoanLTVAdjustmentHistoryRequest{StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FlexibleLoanLTVAdjustmentHistory(t.Context(), &FlexibleLoanLTVAdjustmentHistoryRequest{StartTime: startTime, EndTime: endTime, Current: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleLoanAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FlexibleLoanAssetsData(t.Context(), currency.EMPTYCODE)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFlexibleCollateralAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FlexibleCollateralAssetsData(t.Context(), currency.EMPTYCODE)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesContractDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesContractDetails(t.Context(), asset.Spot)
	require.ErrorIs(t, err, futures.ErrNotFuturesAsset)
	_, err = e.GetFuturesContractDetails(t.Context(), asset.Futures)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	result, err := e.GetFuturesContractDetails(t.Context(), asset.USDTMarginedFutures)
	require.NoError(t, err)
	assert.NotNil(t, result)
	result, err = e.GetFuturesContractDetails(t.Context(), asset.CoinMarginedFutures)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFundingRateInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetFundingRateInfo(t.Context())
	require.NoError(t, err)
}

func TestUGetFundingRateInfo(t *testing.T) {
	t.Parallel()
	result, err := e.UGetFundingRateInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsUFuturesConnect(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.Websocket.IsConnected() {
		t.SkipNow()
	}
	conn, err := e.Websocket.GetConnection(asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.NotNil(t, conn)

	err = e.WsUFuturesConnect(t.Context(), conn)
	require.NoError(t, err)
}

func TestHandleData(t *testing.T) {
	t.Parallel()
	for k, v := range map[string]string{
		"Asset Index":                   `{"stream": "!assetIndex@arr", "data": [{ "e":"assetIndexUpdate", "E":1686749230000, "s":"ADAUSD", "i":"0.27462452", "b":"0.10000000", "a":"0.10000000", "B":"0.24716207", "A":"0.30208698", "q":"0.05000000", "g":"0.05000000", "Q":"0.26089330", "G":"0.28835575" }, { "e":"assetIndexUpdate", "E":1686749230000, "s":"USDTUSD", "i":"0.99987691", "b":"0.00010000", "a":"0.00010000", "B":"0.99977692", "A":"0.99997689", "q":"0.00010000", "g":"0.00010000", "Q":"0.99977692", "G":"0.99997689" } ]}`,
		"Contract Info":                 `{"stream": "!contractInfo", "data": {"e":"contractInfo", "E":1669356423908, "s":"IOTAUSDT", "ps":"IOTAUSDT", "ct":"PERPETUAL", "dt":4133404800000, "ot":1569398400000, "cs":"TRADING", "bks":[ { "bs":1, "bnf":0, "bnc":5000, "mmr":0.01, "cf":0, "mi":21, "ma":50 }, { "bs":2, "bnf":5000, "bnc":25000, "mmr":0.025, "cf":75, "mi":11, "ma":20 } ] }}`,
		"Force Order":                   `{"stream": "!forceOrder@arr", "data": {"e":"forceOrder", "E":1568014460893, "o":{ "s":"BTCUSDT", "S":"SELL", "o":"LIMIT", "f":"IOC", "q":"0.014", "p":"9910", "ap":"9910", "X":"FILLED", "l":"0.014", "z":"0.014", "T":1568014460893 }}}`,
		"All BookTicker":                `{"stream": "!bookTicker","data":{"e":"bookTicker","u":3682854202063,"s":"NEARUSDT","b":"2.4380","B":"20391","a":"2.4390","A":"271","T":1703015198639,"E":1703015198640}}`,
		"Multiple Market Ticker":        `{"stream": "!ticker@arr", "data": [{"e":"24hrTicker","E":1703018247910,"s":"ICPUSDT","p":"-0.540000","P":"-5.395","w":"9.906194","c":"9.470000","Q":"1","o":"10.010000","h":"10.956000","l":"9.236000","v":"34347035","q":"340248403.001000","O":1702931820000,"C":1703018247909,"F":78723309,"L":80207941,"n":1484628},{"e":"24hrTicker","E":1703018247476,"s":"MEMEUSDT","p":"0.0020900","P":"7.331","w":"0.0300554","c":"0.0305980","Q":"7568","o":"0.0285080","h":"0.0312730","l":"0.0284120","v":"5643663185","q":"169622568.3721920","O":1702931820000,"C":1703018247475,"F":88665791,"L":89517438,"n":851643},{"e":"24hrTicker","E":1703018247822,"s":"SOLUSDT","p":"0.8680","P":"1.192","w":"74.4933","c":"73.6900","Q":"21","o":"72.8220","h":"76.3840","l":"71.8000","v":"26283647","q":"1957955612.4830","O":1702931820000,"C":1703018247820,"F":1126774871,"L":1129007642,"n":2232761},{"e":"24hrTicker","E":1703018247254,"s":"IMXUSDT","p":"0.0801","P":"3.932","w":"2.1518","c":"2.1171","Q":"225","o":"2.0370","h":"2.2360","l":"2.0319","v":"59587050","q":"128216496.4538","O":1702931820000,"C":1703018247252,"F":169814879,"L":170587124,"n":772246},{"e":"24hrTicker","E":1703018247309,"s":"DYDXUSDT","p":"-0.036","P":"-1.255","w":"2.896","c":"2.832","Q":"169.6","o":"2.868","h":"2.987","l":"2.782","v":"81690098.5","q":"236599791.383","O":1702931820000,"C":1703018247308,"F":385238821,"L":385888621,"n":649799},{"e":"24hrTicker","E":1703018247240,"s":"ONTUSDT","p":"0.0022","P":"1.011","w":"0.2213","c":"0.2197","Q":"45.7","o":"0.2175","h":"0.2251","l":"0.2157","v":"60880132.6","q":"13471239.8637","O":1702931820000,"C":1703018247238,"F":186008331,"L":186088275,"n":79945},{"e":"24hrTicker","E":1703018247658,"s":"AAVEUSDT","p":"4.660","P":"4.778","w":"102.969","c":"102.190","Q":"0.4","o":"97.530","h":"108.000","l":"97.370","v":"1205430.6","q":"124121750.870","O":1702931820000,"C":1703018247657,"F":343017862,"L":343487276,"n":469414},{"e":"24hrTicker","E":1703018247545,"s":"USTCUSDT","p":"0.0018500","P":"5.628","w":"0.0348991","c":"0.0347200","Q":"2316","o":"0.0328700","h":"0.0371100","l":"0.0328000","v":"2486985654","q":"86793545.3903700","O":1702931820000,"C":1703018247544,"F":32136013,"L":32601947,"n":465935},{"e":"24hrTicker","E":1703018247997,"s":"FTMUSDT","p":"-0.005000","P":"-1.221","w":"0.409721","c":"0.404400","Q":"1421","o":"0.409400","h":"0.421200","l":"0.392100","v":"471077518","q":"193010517.884400","O":1702931820000,"C":1703018247996,"F":716077491,"L":716712548,"n":635055},{"e":"24hrTicker","E":1703018247338,"s":"LRCUSDT","p":"-0.00290","P":"-1.104","w":"0.26531","c":"0.25980","Q":"113","o":"0.26270","h":"0.27190","l":"0.25590","v":"142488749","q":"37803477.10260","O":1702931820000,"C":1703018247336,"F":318115460,"L":318317340,"n":201880},{"e":"24hrTicker","E":1703018247776,"s":"TRBUSDT","p":"25.037","P":"21.840","w":"131.860","c":"139.677","Q":"0.3","o":"114.640","h":"143.900","l":"113.600","v":"3955845.0","q":"521616257.947","O":1702931820000,"C":1703018247775,"F":417041483,"L":419226886,"n":2185249},{"e":"24hrTicker","E":1703018247513,"s":"ACEUSDT","p":"0.108200","P":"0.826","w":"13.544944","c":"13.211400","Q":"14.37","o":"13.103200","h":"15.131200","l":"12.402900","v":"41359842.25","q":"560216757.038015","O":1702931820000,"C":1703018247512,"F":2261106,"L":4779982,"n":2518828}]}`,
		"Single Market Ticker":          `{"stream": "BTCUSDT@ticker", "data": { "e": "24hrTicker", "E": 1571889248277, "s": "BTCUSDT", "p": "0.0015", "P": "250.00", "w": "0.0018", "c": "0.0025", "Q": "10", "o": "0.0010", "h": "0.0025", "l": "0.0010", "v": "10000", "q": "18", "O": 0, "C": 1703019429985, "F": 0, "L": 18150, "n": 18151 } }`,
		"Multiple Mini Tickers":         `{"stream": "!miniTicker@arr","data":[{"e":"24hrMiniTicker","E":1703019429455,"s":"BICOUSDT","c":"0.3667000","o":"0.3792000","h":"0.3892000","l":"0.3639000","v":"28768370","q":"10779000.9922000"},{"e":"24hrMiniTicker","E":1703019429985,"s":"API3USDT","c":"1.6834","o":"1.7326","h":"1.8406","l":"1.6699","v":"12371516.4","q":"21642153.0574"},{"e":"24hrMiniTicker","E":1703019429111,"s":"ICPUSDT","c":"9.414000","o":"10.126000","h":"10.956000","l":"9.236000","v":"34262192","q":"339148145.539000"},{"e":"24hrMiniTicker","E":1703019429945,"s":"SOLUSDT","c":"73.0930","o":"73.2180","h":"76.3840","l":"71.8000","v":"26319095","q":"1960871540.2620"}]}`,
		"Multi Asset Mode Asset":        `{"stream": "!assetIndex@arr", "data":[{ "e":"assetIndexUpdate", "E":1686749230000, "s":"ADAUSD","i":"0.27462452","b":"0.10000000","a":"0.10000000","B":"0.24716207","A":"0.30208698","q":"0.05000000","g":"0.05000000","Q":"0.26089330","G":"0.28835575"}, { "e":"assetIndexUpdate", "E":1686749230000, "s":"USDTUSD", "i":"0.99987691", "b":"0.00010000", "a":"0.00010000", "B":"0.99977692", "A":"0.99997689", "q":"0.00010000", "g":"0.00010000", "Q":"0.99977692", "G":"0.99997689" }]}`,
		"Composite Index Symbol":        `{"stream": "BTCUSDT@compositeIndex", "data":{ "e":"compositeIndex", "E":1602310596000, "s":"DEFIUSDT", "p":"554.41604065", "C":"baseAsset", "c":[ { "b":"BAL", "q":"USDT", "w":"1.04884844", "W":"0.01457800", "i":"24.33521021" }, { "b":"BAND", "q":"USDT" , "w":"3.53782729", "W":"0.03935200", "i":"7.26420084" } ] } }`,
		"Diff Book Depth Stream":        `{"stream": "BTCUSDT@depth@500ms", "data": { "e": "depthUpdate", "E": 1571889248277, "T": 1571889248276, "s": "BTCUSDT", "U": 157, "u": 160, "pu": 149, "b": [ [ "0.0024", "10" ] ], "a": [ [ "0.0026", "100" ] ] } }`,
		"Partial Book Depth Stream":     `{"stream": "BTCUSDT@depth5", "data":{ "e": "depthUpdate", "E": 1571889248277, "T": 1571889248276, "s": "BTCUSDT", "U": 390497796, "u": 390497878, "pu": 390497794, "b": [ [ "7403.89", "0.002" ], [ "7403.90", "3.906" ], [ "7404.00", "1.428" ], [ "7404.85", "5.239" ], [ "7405.43", "2.562" ] ], "a": [ [ "7405.96", "3.340" ], [ "7406.63", "4.525" ], [ "7407.08", "2.475" ], [ "7407.15", "4.800" ], [ "7407.20","0.175"]]}}`,
		"Individual Symbol Mini Ticker": `{"stream": "BTCUSDT@miniTicker", "data": { "e": "24hrMiniTicker", "E": 1571889248277, "s": "BTCUSDT", "c": "0.0025", "o": "0.0010", "h": "0.0025", "l": "0.0010", "v": "10000", "q": "18"}}`,
	} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			err := e.wsHandleFuturesData(t.Context(), nil, []byte(v))
			assert.NoError(t, err)
		})
	}
}

func TestListSubscriptions(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.Websocket.IsConnected() {
		t.SkipNow()
	}
	conn, err := e.Websocket.GetConnection(usdtmPublicFilter)
	require.NoError(t, err)
	require.NotNil(t, conn)

	result, err := e.ListSubscriptions(t.Context(), conn)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetProperty(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.Websocket.IsConnected() {
		t.SkipNow()
	}
	conn, err := e.Websocket.GetConnection(usdtmPrivateFilter)
	require.NoError(t, err)
	require.NotNil(t, conn)

	if !e.Websocket.IsConnected() {
		err = e.WsUFuturesConnect(t.Context(), conn)
		require.NoError(t, err)
	}

	err = e.SetProperty(t.Context(), conn, "combined", true)
	require.NoError(t, err)
}

func TestGetWsOrderbook(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWsOrderbook(&OrderBookDataRequest{Symbol: usdtmTradablePair, Limit: 1000})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWsMostRecentTrades(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWsMostRecentTrades(&RecentTradeRequest{
		Symbol: usdtmTradablePair,
		Limit:  15,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWsAggregatedTrades(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWsAggregatedTrades(&WsAggregateTradeRequest{
		Symbol: usdtmTradablePair,
		Limit:  5,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWsKlines(t *testing.T) {
	t.Parallel()
	_, err := e.GetWsCandlestick(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &KlinesRequest{Timezone: "GMT+2"}
	_, err = e.GetWsCandlestick(arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = spotTradablePair
	_, err = e.GetWsCandlestick(arg)
	require.ErrorIs(t, err, kline.ErrInvalidInterval)

	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	startTime, endTime := getTime()
	result, err := e.GetWsCandlestick(&KlinesRequest{
		Symbol:    usdtmTradablePair,
		Interval:  kline.FiveMin.Short(),
		Limit:     24,
		StartTime: startTime,
		EndTime:   endTime,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWsOptimizedCandlestick(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	startTime, endTime := getTime()
	result, err := e.GetWsOptimizedCandlestick(&KlinesRequest{
		Symbol:    usdtmTradablePair,
		Interval:  kline.FiveMin.Short(),
		Limit:     24,
		StartTime: startTime,
		EndTime:   endTime,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCurrenctAveragePrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetWsCurrenctAveragePrice(currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWsCurrenctAveragePrice(usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWs24HourPriceChanges(t *testing.T) {
	t.Parallel()
	_, err := e.GetWs24HourPriceChanges(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetWs24HourPriceChanges(&PriceChangeRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairsEmpty)

	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWs24HourPriceChanges(&PriceChangeRequest{Symbols: []currency.Pair{usdtmTradablePair}})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWsTradingDayTickers(t *testing.T) {
	t.Parallel()
	_, err := e.GetWsTradingDayTickers(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetWsTradingDayTickers(&PriceChangeRequest{Timezone: "GMT+3"})
	require.ErrorIs(t, err, currency.ErrCurrencyPairsEmpty)

	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWsTradingDayTickers(&PriceChangeRequest{
		Symbols: []currency.Pair{usdtmTradablePair},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWsRollingWindowPriceChanges(t *testing.T) {
	t.Parallel()
	_, err := e.GetWsRollingWindowPriceChanges(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetWsRollingWindowPriceChanges(&WsRollingWindowPriceRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWsRollingWindowPriceChanges(&WsRollingWindowPriceRequest{Symbols: []currency.Pair{usdtmTradablePair}})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSymbolPriceTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetSymbolPriceTicker(currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairsEmpty)

	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetSymbolPriceTicker(usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWsSymbolOrderbookTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetWsSymbolOrderbookTicker([]currency.Pair{currency.EMPTYPAIR})
	require.ErrorIs(t, err, currency.ErrCurrencyPairsEmpty)

	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWsSymbolOrderbookTicker([]currency.Pair{usdtmTradablePair})
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetWsSymbolOrderbookTicker([]currency.Pair{
		usdtmTradablePair,
		currency.NewPair(currency.ETH, currency.USDT),
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetQuerySessionStatus(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetQuerySessionStatus()
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLogOutOfSession(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetLogOutOfSession()
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPlaceNewOrder(t *testing.T) {
	t.Parallel()
	_, err := e.WsPlaceNewOrder(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &TradeOrderRequest{}
	_, err = e.WsPlaceNewOrder(arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.WsPlaceNewOrder(arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.WsPlaceNewOrder(arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsPlaceNewOrder(&TradeOrderRequest{
		Symbol:      usdtmTradablePair,
		Side:        order.Sell.String(),
		OrderType:   order.Limit.String(),
		TimeInForce: "GTC",
		Price:       1234,
		Quantity:    1,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestValidatePlaceNewOrderRequest(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	err := e.ValidatePlaceNewOrderRequest(&TradeOrderRequest{
		Symbol:      usdtmTradablePair,
		Side:        order.Sell.String(),
		OrderType:   order.Limit.String(),
		TimeInForce: "GTC",
		Price:       1234,
		Quantity:    1,
	})
	require.NoError(t, err)
}

func TestWsQueryOrder(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsQueryOrder(&QueryOrderRequest{
		Symbol:  usdtmTradablePair,
		OrderID: 12345,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSignRequestExcludesSignature(t *testing.T) {
	t.Parallel()
	var ex Exchange
	ex.SetCredentials(&accounts.Credentials{Key: "testkey", Secret: "testsecret"})

	base := map[string]any{"symbol": "BTCUSDT", "timestamp": int64(1785892806252)}
	_, want, err := ex.SignRequest(maps.Clone(base))
	require.NoError(t, err, "signing must not error")

	// A struct reused for a second request still carries the previous signature; it must
	// not contribute to the HMAC, or the transmitted payload would not match the signed one
	stale := maps.Clone(base)
	stale["signature"] = "a-previous-signature"
	_, got, err := ex.SignRequest(stale)
	require.NoError(t, err, "signing must not error with a stale signature present")
	assert.Equal(t, want, got, "a pre-existing signature should not change the result")
	assert.NotContains(t, stale, "signature", "the stale signature should be removed before signing")
}

func TestSignRequest(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	_, signature, err := e.SignRequest(map[string]any{
		"name": "nameValue",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, signature, "unexpected signature")
}

func TestWsCancelAndReplaceTradeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsCancelAndReplaceTradeOrder(&WsCancelAndReplaceRequest{
		Symbol:                      usdtmTradablePair,
		CancelReplaceMode:           "ALLOW_FAILURE",
		CancelOriginalClientOrderID: "4d96324ff9d44481926157",
		Side:                        order.Sell.String(),
		OrderType:                   order.Limit.String(),
		TimeInForce:                 "GTC",
		Price:                       23416.10000000,
		Quantity:                    0.00847000,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsCurrentOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsCurrentOpenOrders(usdtmTradablePair, 6000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsCancelOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsCancelOpenOrders(usdtmTradablePair, 6000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsPlaceOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.WsPlaceOCOOrder(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &PlaceOCOOrderRequest{StopLimitTimeInForce: "GTC"}
	_, err = e.WsPlaceOCOOrder(arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.WsPlaceOCOOrder(arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.WsPlaceOCOOrder(arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsPlaceOCOOrder(&PlaceOCOOrderRequest{
		Symbol:               usdtmTradablePair,
		Side:                 order.Sell.String(),
		Price:                23420.00000000,
		Quantity:             0.00650000,
		StopPrice:            23410.00000000,
		StopLimitPrice:       23405.00000000,
		StopLimitTimeInForce: "GTC",
		NewOrderRespType:     "RESULT",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsQueryOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.WsQueryOCOOrder("", 0, 0)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsQueryOCOOrder("123456788", 0, 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsCancelOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.WsCancelOCOOrder(currency.EMPTYPAIR, "someID", "12354", "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.WsCancelOCOOrder(spotTradablePair, "", "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsCancelOCOOrder(
		usdtmTradablePair, "someID", "12354", "",
	)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsCurrentOpenOCOOrders(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsCurrentOpenOCOOrders(0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsPlaceNewSOROrder(t *testing.T) {
	t.Parallel()
	_, err := e.WsPlaceNewSOROrder(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &WsOSRPlaceOrderRequest{TimeInForce: "GTC"}
	_, err = e.WsPlaceNewSOROrder(arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = spotTradablePair
	_, err = e.WsPlaceNewSOROrder(arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "BUY"
	_, err = e.WsPlaceNewSOROrder(arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = "limit"
	_, err = e.WsPlaceNewSOROrder(arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsPlaceNewSOROrder(&WsOSRPlaceOrderRequest{
		Symbol:      usdtmTradablePair,
		Side:        "BUY",
		OrderType:   order.Limit.String(),
		Quantity:    0.5,
		TimeInForce: "GTC",
		Price:       31000,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsTestNewOrderUsingSOR(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if mockTests {
		t.SkipNow()
	}
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	err := e.WsTestNewOrderUsingSOR(&WsOSRPlaceOrderRequest{
		Symbol:      usdtmTradablePair,
		Side:        "BUY",
		OrderType:   order.Limit.String(),
		Quantity:    0.5,
		TimeInForce: "GTC",
		Price:       31000,
	})
	require.NoError(t, err)
}

func TestToMap(t *testing.T) {
	t.Parallel()
	input := &struct {
		Zebiba bool   `json:"zebiba"`
		Value  int64  `json:"value"`
		Abebe  string `json:"abebe"`
		Name   string `json:"name"`
	}{
		Name:  "theName",
		Value: 347,
	}
	result, err := e.ToMap(input)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Large integers must survive as their literal text: rendering them via float64
	// would sign a different payload from the one transmitted
	big := &struct {
		Timestamp int64  `json:"timestamp"`
		OrderID   uint64 `json:"orderId"`
	}{Timestamp: 1785892806252, OrderID: 12345678901234567}
	result, err = e.ToMap(big)
	require.NoError(t, err)
	assert.Equal(t, "1785892806252", fmt.Sprintf("%v", result["timestamp"]), "timestamp should keep its literal form")
	assert.Equal(t, "12345678901234567", fmt.Sprintf("%v", result["orderId"]), "order ID should keep its literal form")
}

func TestSortingTest(t *testing.T) {
	params := map[string]any{"apiCredentials.Key": "wwhj3r3amR", "signature": "f89c6e5c0b", "timestamp": 1704873175325, "symbol": usdtmTradablePair, "startTime": 1704009175325, "endTime": 1704873175325, "limit": 5}
	sortedKeys := []string{"apiCredentials.Key", "endTime", "limit", "signature", "startTime", "symbol", "timestamp"}
	keys := sortMapKeys(params)
	require.Len(t, keys, len(sortedKeys), "unexpected keys length")
	for a := range keys {
		require.Equal(t, keys[a], sortedKeys[a])
	}
}

func TestGetAccountInformation(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.GetWsAccountInfo(0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsQueryAccountOrderRateLimits(t *testing.T) {
	t.Parallel()
	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsQueryAccountOrderRateLimits(0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsQueryAccountOrderHistory(t *testing.T) {
	t.Parallel()
	_, err := e.WsQueryAccountOrderHistory(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.WsQueryAccountOrderHistory(&AccountOrderRequest{Limit: 5})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsQueryAccountOrderHistory(&AccountOrderRequest{
		Symbol:    usdtmTradablePair,
		StartTime: time.Now().Add(-time.Hour * 24 * 10),
		EndTime:   time.Now().Add(-time.Hour * 6),
		Limit:     5,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsQueryAccountOCOOrderHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.WsQueryAccountOCOOrderHistory(0, 0, 0, endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsQueryAccountOCOOrderHistory(0, 0, 0, startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsAccountTradeHistory(t *testing.T) {
	t.Parallel()
	_, err := e.WsAccountTradeHistory(nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.WsAccountTradeHistory(&AccountOrderRequest{OrderID: 1234})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsAccountTradeHistory(&AccountOrderRequest{Symbol: usdtmTradablePair, OrderID: 1234})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsAccountPreventedMatches(t *testing.T) {
	t.Parallel()
	_, err := e.WsAccountPreventedMatches(currency.EMPTYPAIR, 1223456, 0, 0, 0, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.WsAccountPreventedMatches(spotTradablePair, 0, 0, 0, 0, 0)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsAccountPreventedMatches(usdtmTradablePair, 1223456, 0, 0, 0, 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsAccountAllocation(t *testing.T) {
	t.Parallel()
	_, err := e.WsAccountAllocation(currency.EMPTYPAIR, time.Time{}, time.Now(), 0, 0, 0, 19)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsAccountAllocation(spotTradablePair, time.Time{}, time.Now(), 0, 0, 0, 19)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWsAccountCommissionRates(t *testing.T) {
	t.Parallel()
	_, err := e.WsAccountCommissionRates(currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	if mockTests {
		t.SkipNow()
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	testexch.SetupWs(t, e)
	if !e.IsAPIStreamConnected() {
		t.Skip(apiStreamingIsNotConnected)
	}
	result, err := e.WsAccountCommissionRates(spotTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.BTC.Item,
		Quote: currency.USDT.Item,
		Asset: asset.Spot,
	})
	require.ErrorIs(t, err, asset.ErrNotSupported)

	result, err := e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.BTC.Item,
		Quote: currency.USDT.Item,
		Asset: asset.USDTMarginedFutures,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result)

	result, err = e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  coinmTradablePair.Base.Item,
		Quote: coinmTradablePair.Quote.Item,
		Asset: asset.CoinMarginedFutures,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestSystemStatus(t *testing.T) {
	t.Parallel()
	result, err := e.GetSystemStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDailyAccountSnapshot(t *testing.T) {
	t.Parallel()
	_, err := e.GetDailyAccountSnapshot(t.Context(), "", time.Time{}, time.Now(), 0)
	require.ErrorIs(t, err, asset.ErrInvalidAsset)

	startTime, endTime := getTime()
	_, err = e.GetDailyAccountSnapshot(t.Context(), "SPOT", endTime, startTime, 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDailyAccountSnapshot(t.Context(), "SPOT", time.Time{}, time.Now(), 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDisableFastWithdrawalSwitch(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.DisableFastWithdrawalSwitch(t.Context())
	assert.NoError(t, err)
}

func TestEnableFastWithdrawalSwitch(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.EnableFastWithdrawalSwitch(t.Context())
	assert.NoError(t, err)
}

func TestGetAccountStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccountStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAccountTradingAPIStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccountTradingAPIStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDustLog(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetDustLog(t.Context(), "MARGIN", endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDustLog(t.Context(), "MARGIN", time.Time{}, time.Now())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCheckServerTime(t *testing.T) {
	t.Parallel()
	result, err := e.GetExchangeServerTime(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccount(t.Context(), true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAccountTradeList(t *testing.T) {
	t.Parallel()
	_, err := e.GetAccountTradeList(t.Context(), &GetAccountTradeListRequest{Limit: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	startTime, endTime := getTime()
	_, err = e.GetAccountTradeList(t.Context(), &GetAccountTradeListRequest{Symbol: assetToTradablePairMap[asset.Margin], StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccountTradeList(t.Context(), &GetAccountTradeListRequest{Symbol: assetToTradablePairMap[asset.Margin], StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCurrentOrderCountUsage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCurrentOrderCountUsage(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPreventedMatches(t *testing.T) {
	t.Parallel()
	_, err := e.GetPreventedMatches(t.Context(), &GetPreventedMatchesRequest{OrderID: 12, Limit: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetPreventedMatches(t.Context(), &GetPreventedMatchesRequest{Symbol: usdtmTradablePair})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPreventedMatches(t.Context(), &GetPreventedMatchesRequest{Symbol: usdtmTradablePair, OrderID: 12, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllocations(t *testing.T) {
	t.Parallel()
	_, err := e.GetAllocations(t.Context(), &GetAllocationsRequest{FromAllocationID: 10, OrderID: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	startTime, endTime := getTime()
	_, err = e.GetAllocations(t.Context(), &GetAllocationsRequest{Symbol: usdtmTradablePair, StartTime: endTime, EndTime: startTime, FromAllocationID: 10, OrderID: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllocations(t.Context(), &GetAllocationsRequest{Symbol: usdtmTradablePair, StartTime: startTime, EndTime: endTime, FromAllocationID: 10, OrderID: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCommissionRate(t *testing.T) {
	t.Parallel()
	_, err := e.GetCommissionRates(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCommissionRates(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginAccountBorrowRepay(t *testing.T) {
	t.Parallel()
	_, err := e.MarginAccountBorrowRepay(t.Context(), &MarginAccountBorrowRepayRequest{AssetName: currency.ETH, LendingType: "BORROW", Amount: 0.1234})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.MarginAccountBorrowRepay(t.Context(), &MarginAccountBorrowRepayRequest{Symbol: usdtmTradablePair, LendingType: "BORROW", Amount: 0.1234})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.MarginAccountBorrowRepay(t.Context(), &MarginAccountBorrowRepayRequest{AssetName: currency.ETH, Symbol: usdtmTradablePair, Amount: 0.1234})
	require.ErrorIs(t, err, errLendingTypeRequired)
	_, err = e.MarginAccountBorrowRepay(t.Context(), &MarginAccountBorrowRepayRequest{AssetName: currency.ETH, Symbol: usdtmTradablePair, LendingType: "BORROW"})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.MarginAccountBorrowRepay(t.Context(), &MarginAccountBorrowRepayRequest{AssetName: currency.ETH, Symbol: usdtmTradablePair, LendingType: "BORROW", Amount: 0.1234})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBorrowOrRepayRecordsInMarginAccount(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetBorrowOrRepayRecordsInMarginAccount(t.Context(), &MarginBorrowRepayRecordsRequest{Asset: currency.LTC, LendingType: "REPAY", Current: 10, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBorrowOrRepayRecordsInMarginAccount(t.Context(), &MarginBorrowRepayRecordsRequest{Asset: currency.LTC, LendingType: "REPAY", Current: 10, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllMarginAssets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllMarginAssets(t.Context(), currency.BTC)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllCrossMarginPairs(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllCrossMarginPairs(t.Context(), assetToTradablePairMap[asset.Margin])
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginPriceIndex(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginPriceIndex(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginPriceIndex(t.Context(), assetToTradablePairMap[asset.Margin])
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPostMarginAccountOrder(t *testing.T) {
	t.Parallel()
	_, err := e.PostMarginAccountOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	autoRepay := true
	arg := &MarginAccountOrderRequest{AutoRepayAtCancel: &autoRepay}
	_, err = e.PostMarginAccountOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.PostMarginAccountOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Buy.String()
	_, err = e.PostMarginAccountOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.PostMarginAccountOrder(t.Context(), &MarginAccountOrderRequest{
		Symbol:    usdtmTradablePair,
		Side:      order.Buy.String(),
		OrderType: order.Limit.String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelMarginAccountOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelMarginAccountOrder(t.Context(), &CancelMarginAccountOrderRequest{OrderID: "12314234", IsIsolated: true})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.CancelMarginAccountOrder(t.Context(), &CancelMarginAccountOrderRequest{Symbol: usdtmTradablePair, IsIsolated: true})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelMarginAccountOrder(t.Context(), &CancelMarginAccountOrderRequest{Symbol: usdtmTradablePair, OrderID: "12314234", IsIsolated: true})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginAccountCancelAllOpenOrdersOnSymbol(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllOpenMarginAccountOrdersOnSymbol(t.Context(), currency.EMPTYPAIR, true)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CancelAllOpenMarginAccountOrdersOnSymbol(t.Context(), usdtmTradablePair, true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUnmarshalJSONForAssetIndex(t *testing.T) {
	t.Parallel()
	var resp *AssetIndexResponse
	data := [][]byte{
		[]byte(`{ "symbol": "ADAUSD", "time": 1635740268004, "index": "1.92957370", "bidBuffer": "0.10000000", "askBuffer": "0.10000000", "bidRate": "1.73661633", "askRate": "2.12253107", "autoExchangeBidBuffer": "0.05000000", "autoExchangeAskBuffer": "0.05000000", "autoExchangeBidRate": "1.83309501", "autoExchangeAskRate": "2.02605238" }`),
		[]byte(`[ { "symbol": "ADAUSD", "time": 1635740268004, "index": "1.92957370", "bidBuffer": "0.10000000", "askBuffer": "0.10000000", "bidRate": "1.73661633", "askRate": "2.12253107", "autoExchangeBidBuffer": "0.05000000", "autoExchangeAskBuffer": "0.05000000", "autoExchangeBidRate": "1.83309501", "autoExchangeAskRate": "2.02605238" } ]`),
	}
	err := json.Unmarshal(data[0], &resp)
	require.NoError(t, err)
	err = json.Unmarshal(data[1], &resp)
	assert.NoError(t, err)
}

func TestChangePositionMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.ChangePositionMode(t.Context(), false)
	assert.NoError(t, err)
}

func TestGetCurrentPositionMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCurrentPositionMode(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// ---------------------------  European Option Endpoints test -----------------------------------

func TestCheckEOptionsServerTime(t *testing.T) {
	t.Parallel()
	serverTime, err := e.CheckEOptionsServerTime(t.Context())
	require.NoError(t, err)
	assert.NotEmpty(t, serverTime)
}

func TestGetEOptionsOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetEOptionsOrderbook(t.Context(), currency.EMPTYPAIR, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetEOptionsOrderbook(t.Context(), optionsTradablePair, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEOptionsRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetEOptionsRecentTrades(t.Context(), currency.EMPTYPAIR, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetEOptionsRecentTrades(t.Context(), optionsTradablePair, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEOptionsCandlesticks(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetEOptionsCandlesticks(t.Context(), &GetEOptionsCandlesticksRequest{Interval: kline.OneDay, StartTime: startTime, EndTime: endTime, Limit: 1000})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetEOptionsCandlesticks(t.Context(), &GetEOptionsCandlesticksRequest{Symbol: optionsTradablePair, StartTime: startTime, EndTime: endTime, Limit: 1000})
	require.ErrorIs(t, err, kline.ErrInvalidInterval)

	_, err = e.GetEOptionsCandlesticks(t.Context(), &GetEOptionsCandlesticksRequest{Symbol: optionsTradablePair, Interval: kline.OneDay, StartTime: startTime, EndTime: endTime, Limit: 1000})
	require.NoError(t, err)
}

func TestGetOptionMarkPrice(t *testing.T) {
	t.Parallel()
	result, err := e.GetOptionMarkPrice(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetOptionMarkPrice(t.Context(), optionsTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEOptions24hrTickerPriceChangeStatistics(t *testing.T) {
	t.Parallel()
	result, err := e.GetEOptions24hrTickerPriceChangeStatistics(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetEOptions24hrTickerPriceChangeStatistics(t.Context(), optionsTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEOptionsSymbolPriceTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetEOptionsSymbolPriceTicker(t.Context(), "")
	require.ErrorIs(t, err, errUnderlyingIsRequired)

	result, err := e.GetEOptionsSymbolPriceTicker(t.Context(), "BTCUSDT")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEOptionsHistoricalExerciseRecords(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetEOptionsHistoricalExerciseRecords(t.Context(), "BTCUSDT", endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetEOptionsHistoricalExerciseRecords(t.Context(), "BTCUSDT", startTime, endTime, 10)
	require.NoError(t, err)
}

func TestGetEOptionsOpenInterests(t *testing.T) {
	t.Parallel()
	expiration := time.UnixMilli(1774598400000)
	_, err := e.GetEOptionsOpenInterests(t.Context(), currency.EMPTYCODE, expiration)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.GetEOptionsOpenInterests(t.Context(), currency.ETH, time.Time{})
	require.ErrorIs(t, err, errExpirationTimeRequired)

	result, err := e.GetEOptionsOpenInterests(t.Context(), currency.ETH, expiration)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOptionsAccountInformation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOptionsAccountInformation(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewOptionsOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewOptionsOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OptionsOrderRequest{
		PostOnly: true,
	}
	_, err = e.NewOptionsOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = optionsTradablePair
	_, err = e.NewOptionsOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.NewOptionsOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = order.Limit.String()
	_, err = e.NewOptionsOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewOptionsOrder(t.Context(), &OptionsOrderRequest{
		Symbol:                  optionsTradablePair,
		Side:                    order.Sell.String(),
		OrderType:               order.Limit.String(),
		Amount:                  0.00001,
		Price:                   0.00001,
		ReduceOnly:              false,
		PostOnly:                true,
		NewOrderResponseType:    "RESULT",
		ClientOrderID:           "the-client-order-id",
		IsMarketMakerProtection: true,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPlaceEOptionsOrder(t *testing.T) {
	t.Parallel()
	_, err := e.PlaceBatchEOptionsOrder(t.Context(), []*OptionsOrderRequest{})
	require.ErrorIs(t, err, common.ErrEmptyParams)

	arg := &OptionsOrderRequest{
		PostOnly: true,
	}
	_, err = e.PlaceBatchEOptionsOrder(t.Context(), []*OptionsOrderRequest{arg})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = currency.Pair{Base: currency.BTC, Delimiter: currency.DashDelimiter, Quote: currency.NewCode("200730-9000-C")}
	_, err = e.PlaceBatchEOptionsOrder(t.Context(), []*OptionsOrderRequest{arg})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.PlaceBatchEOptionsOrder(t.Context(), []*OptionsOrderRequest{arg})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = order.Limit.String()
	_, err = e.PlaceBatchEOptionsOrder(t.Context(), []*OptionsOrderRequest{arg})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.PlaceBatchEOptionsOrder(t.Context(), []*OptionsOrderRequest{
		{
			Symbol:                  optionsTradablePair,
			Side:                    order.Sell.String(),
			OrderType:               order.Limit.String(),
			Amount:                  0.00001,
			Price:                   0.00001,
			ReduceOnly:              false,
			PostOnly:                true,
			NewOrderResponseType:    "RESULT",
			ClientOrderID:           "the-client-order-id",
			IsMarketMakerProtection: true,
		}, {
			Symbol:                  optionsTradablePair,
			Side:                    "Buy",
			OrderType:               "Market",
			Amount:                  0.00001,
			PostOnly:                true,
			NewOrderResponseType:    "RESULT",
			ClientOrderID:           "the-client-order-id-2",
			IsMarketMakerProtection: true,
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSingleEOptionsOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetSingleEOptionsOrder(t.Context(), currency.EMPTYPAIR, "", 4611875134427365377)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetSingleEOptionsOrder(t.Context(), optionsTradablePair, "", 0)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSingleEOptionsOrder(t.Context(), optionsTradablePair, "", 4611875134427365377)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelOptionsOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelOptionsOrder(t.Context(), currency.EMPTYPAIR, "213123", "4611875134427365377")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.CancelOptionsOrder(t.Context(), optionsTradablePair, "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelOptionsOrder(t.Context(), optionsTradablePair, "213123", "4611875134427365377")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelBatchOptionsOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelBatchOptionsOrders(t.Context(), currency.EMPTYPAIR, []int64{4611875134427365377}, []string{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.CancelBatchOptionsOrders(t.Context(), optionsTradablePair, []int64{}, []string{})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelBatchOptionsOrders(t.Context(), optionsTradablePair, []int64{4611875134427365377}, []string{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelAllOptionOrdersOnSpecificSymbol(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	err := e.CancelAllOptionOrdersOnSpecificSymbol(t.Context(), optionsTradablePair)
	assert.NoError(t, err)
}

func TestCancelAllOptionsOrdersByUnderlying(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelAllOptionsOrdersByUnderlying(t.Context(), "BTCUSDT")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCurrentOpenOptionsOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetCurrentOpenOptionsOrders(t.Context(), &GetCurrentOpenOptionsOrdersRequest{Symbol: optionsTradablePair, StartTime: endTime, EndTime: startTime, OrderID: 4611875134427365377})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	results, err := e.GetCurrentOpenOptionsOrders(t.Context(), &GetCurrentOpenOptionsOrdersRequest{Symbol: optionsTradablePair, StartTime: startTime, EndTime: endTime, OrderID: 4611875134427365377})
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestGetOptionsOrdersHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetOptionsOrdersHistory(t.Context(), &GetOptionsOrdersHistoryRequest{Symbol: optionsTradablePair, StartTime: endTime, EndTime: startTime, OrderID: 4611875134427365377})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	results, err := e.GetOptionsOrdersHistory(t.Context(), &GetOptionsOrdersHistoryRequest{Symbol: optionsTradablePair, StartTime: startTime, EndTime: endTime, OrderID: 4611875134427365377, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestGetOptionPositionInformation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOptionPositionInformation(t.Context(), optionsTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEOptionsAccountTradeList(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetEOptionsAccountTradeList(t.Context(), &GetEOptionsAccountTradeListRequest{Symbol: optionsTradablePair, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetEOptionsAccountTradeList(t.Context(), &GetEOptionsAccountTradeListRequest{Symbol: optionsTradablePair, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserOptionsExerciseRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetUserOptionsExerciseRecord(t.Context(), optionsTradablePair, endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserOptionsExerciseRecord(t.Context(), optionsTradablePair, startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAccountFundingFlow(t *testing.T) {
	t.Parallel()
	_, err := e.GetAccountFundingFlow(t.Context(), &GetAccountFundingFlowRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	startTime, endTime := getTime()
	_, err = e.GetAccountFundingFlow(t.Context(), &GetAccountFundingFlowRequest{Currency: currency.ETH, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccountFundingFlow(t.Context(), &GetAccountFundingFlowRequest{Currency: currency.USDT, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDownloadIDForOptionTransactionHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetDownloadIDForOptionTransactionHistory(t.Context(), endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDownloadIDForOptionTransactionHistory(t.Context(), startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOptionTransactionHistoryDownloadLinkByID(t *testing.T) {
	t.Parallel()
	_, err := e.GetOptionTransactionHistoryDownloadLinkByID(t.Context(), "")
	require.ErrorIs(t, err, errDownloadIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOptionTransactionHistoryDownloadLinkByID(t.Context(), "download-id")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOptionMarginAccountInformation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOptionMarginAccountInformation(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetMarketMakerProtectionConfig(t *testing.T) {
	t.Parallel()
	_, err := e.SetOptionsMarketMakerProtectionConfig(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.SetOptionsMarketMakerProtectionConfig(t.Context(), &MarketMakerProtectionConfig{
		WindowTimeInMilliseconds: 3000,
		FrozenTimeInMilliseconds: 300000,
		QuantityLimit:            1.5,
		NetDeltaLimit:            1.5,
	})
	require.ErrorIs(t, err, errUnderlyingIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SetOptionsMarketMakerProtectionConfig(t.Context(), &MarketMakerProtectionConfig{
		Underlying:               "BTCUSDT",
		WindowTimeInMilliseconds: 3000,
		FrozenTimeInMilliseconds: 300000,
		QuantityLimit:            1.5,
		NetDeltaLimit:            1.5,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOptionsMarketMakerProtection(t *testing.T) {
	t.Parallel()
	_, err := e.GetOptionsMarketMakerProtection(t.Context(), "")
	require.ErrorIs(t, err, errUnderlyingIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOptionsMarketMakerProtection(t.Context(), "BTCUSDT")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestResetMarketMaketProtection(t *testing.T) {
	t.Parallel()
	_, err := e.ResetMarketMaketProtection(t.Context(), "")
	require.ErrorIs(t, err, errUnderlyingIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.ResetMarketMaketProtection(t.Context(), "BTCUSDT")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetOptionsAutoCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	_, err := e.SetOptionsAutoCancelAllOpenOrders(t.Context(), "", 30000)
	require.ErrorIs(t, err, errUnderlyingIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.SetOptionsAutoCancelAllOpenOrders(t.Context(), "BTCUSDT", 30000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAutoCancelAllOpenOrdersConfig(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAutoCancelAllOpenOrdersConfig(t.Context(), "BTCUSDT")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOptionsAutoCancelAllOpenOrdersHeartbeat(t *testing.T) {
	t.Parallel()
	_, err := e.GetOptionsAutoCancelAllOpenOrdersHeartbeat(t.Context(), []string{})
	require.ErrorIs(t, err, errUnderlyingIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.GetOptionsAutoCancelAllOpenOrdersHeartbeat(t.Context(), []string{"ETHUSDT"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOptionsExchangeInformation(t *testing.T) {
	t.Parallel()
	exchangeinformation, err := e.GetOptionsExchangeInformation(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, exchangeinformation)
}

// ---------------------------------------   Portfolio Margin  ---------------------------------------------

func TestNewUMOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewUMOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &UMOrderRequest{ReduceOnly: true}
	_, err = e.NewUMOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.NewUMOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "BUY"
	_, err = e.NewUMOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = "limit"
	_, err = e.NewUMOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrInvalidTimeInForce)

	arg.TimeInForce = "GTC"
	_, err = e.NewUMOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.Quantity = 1.
	_, err = e.NewUMOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	arg.Price = 1234
	arg.OrderType = "market"
	arg.Quantity = 0
	_, err = e.NewUMOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.OrderType = "stop"
	_, err = e.NewUMOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrUnsupportedOrderType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewUMOrder(t.Context(), &UMOrderRequest{
		Symbol:       usdtmTradablePair,
		Side:         "BUY",
		PositionSide: "BOTH",
		OrderType:    "market",
		Quantity:     1,
		ReduceOnly:   false,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewCMOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewCMOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &UMOrderRequest{
		ReduceOnly: true,
	}
	_, err = e.NewCMOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.NewCMOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "BUY"
	_, err = e.NewCMOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = "OCO"
	_, err = e.NewCMOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrUnsupportedOrderType)

	arg.OrderType = "MARKET"
	_, err = e.NewCMOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.OrderType = order.Limit.String()
	_, err = e.NewCMOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrInvalidTimeInForce)

	arg.TimeInForce = "GTC"
	_, err = e.NewCMOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.Quantity = .1
	_, err = e.NewCMOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewCMOrder(t.Context(), &UMOrderRequest{
		Symbol:       usdtmTradablePair,
		Side:         "BUY",
		PositionSide: "BOTH",
		OrderType:    "limit",
		Quantity:     1,
		ReduceOnly:   false,
		TimeInForce:  "GTD",
		Price:        000.1,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewMarginOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewMarginOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &MarginOrderRequest{
		TimeInForce: "GTC",
	}
	_, err = e.NewMarginOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = spotTradablePair
	_, err = e.NewMarginOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.NewMarginOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.OrderType = order.Limit.String()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewMarginOrder(t.Context(), arg)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginAccountBorrow(t *testing.T) {
	t.Parallel()
	_, err := e.MarginAccountBorrow(t.Context(), currency.EMPTYCODE, 0.001)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.MarginAccountBorrow(t.Context(), currency.USDT, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.MarginAccountBorrow(t.Context(), currency.USDT, 0.001)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginAccountRepay(t *testing.T) {
	t.Parallel()
	_, err := e.MarginAccountRepay(t.Context(), currency.EMPTYCODE, 0.001)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.MarginAccountRepay(t.Context(), currency.USDT, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.MarginAccountRepay(t.Context(), currency.USDT, 0.001)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginAccountNewOCO(t *testing.T) {
	t.Parallel()
	_, err := e.MarginAccountNewOCO(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OCOOrderRequest{
		TrailingDelta: 1,
	}
	_, err = e.MarginAccountNewOCO(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.MarginAccountNewOCO(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "Buy"
	_, err = e.MarginAccountNewOCO(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.Amount = 0.1
	_, err = e.MarginAccountNewOCO(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	arg.Price = 0.001
	_, err = e.MarginAccountNewOCO(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewOCOOrder(t.Context(), &OCOOrderRequest{
		Symbol:             usdtmTradablePair,
		ListClientOrderID:  "1231231231231",
		Side:               "Buy",
		Amount:             0.1,
		LimitClientOrderID: "3423423",
		Price:              0.001,
		StopPrice:          1234.21,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewOCOOrderList(t *testing.T) {
	t.Parallel()
	_, err := e.NewOCOOrderList(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OCOOrderListRequest{
		AboveTimeInForce: "GTC",
	}
	_, err = e.NewOCOOrderList(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = "LTCBTC"
	_, err = e.NewOCOOrderList(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.NewOCOOrderList(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.Quantity = 1
	_, err = e.NewOCOOrderList(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	arg.AboveType = "STOP_LOSS_LIMIT"
	_, err = e.NewOCOOrderList(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.NewOCOOrderList(t.Context(), &OCOOrderListRequest{
		Symbol:     "LTCBTC",
		Side:       order.Sell.String(),
		Quantity:   1,
		AbovePrice: 100,
		AboveType:  "STOP_LOSS_LIMIT",
		BelowType:  "LIMIT_MAKER",
		BelowPrice: 25,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewUMConditionalOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewUMConditionalOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &ConditionalOrderRequest{PriceProtect: true}
	_, err = e.NewUMConditionalOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.NewUMConditionalOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	_, err = e.NewUMConditionalOrder(t.Context(), arg)
	require.ErrorIs(t, err, errStrategyTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewUMConditionalOrder(t.Context(), &ConditionalOrderRequest{
		Symbol:       usdtmTradablePair,
		Side:         order.Sell.String(),
		PositionSide: "SHORT",
		StrategyType: "STOP_MARKET",
		PriceProtect: true,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewCMConditionalOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewCMConditionalOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &ConditionalOrderRequest{
		PositionSide: "LONG",
	}
	_, err = e.NewCMConditionalOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = coinmTradablePair
	_, err = e.NewCMConditionalOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "Buy"
	_, err = e.NewCMConditionalOrder(t.Context(), arg)
	require.ErrorIs(t, err, errStrategyTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewCMConditionalOrder(t.Context(), &ConditionalOrderRequest{
		Symbol:       coinmTradablePair,
		Side:         "Buy",
		PositionSide: "LONG",
		StrategyType: "TAKE_PROFIT",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelUMOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelUMOrder(t.Context(), currency.EMPTYPAIR, "", "1234132")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.CancelUMOrder(t.Context(), usdtmTradablePair, "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelUMOrder(t.Context(), usdtmTradablePair, "", "1234132")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelCMOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelCMOrder(t.Context(), currency.EMPTYPAIR, "", "21321312")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.CancelCMOrder(t.Context(), usdtmTradablePair, "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelCMOrder(t.Context(), usdtmTradablePair, "", "21321312")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelAllUMOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllUMOrders(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelAllUMOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.Code)
}

func TestCancelAllCMOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllCMOrders(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelAllCMOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPMCancelMarginAccountOrder(t *testing.T) {
	t.Parallel()
	_, err := e.PMCancelMarginAccountOrder(t.Context(), currency.EMPTYPAIR, "", "12314")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.PMCancelMarginAccountOrder(t.Context(), assetToTradablePairMap[asset.Margin], "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.PMCancelMarginAccountOrder(t.Context(), assetToTradablePairMap[asset.Margin], "", "12314")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelAllMarginOpenOrdersBySymbol(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllMarginOpenOrdersBySymbol(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelAllMarginOpenOrdersBySymbol(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelMarginAccountOCOOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelMarginAccountOCOOrders(t.Context(), currency.EMPTYPAIR, "", "", 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelMarginAccountOCOOrders(t.Context(), assetToTradablePairMap[asset.Margin], "", "", 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelUMConditionalOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelUMConditionalOrder(t.Context(), currency.EMPTYPAIR, "", 2000)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.CancelUMConditionalOrder(t.Context(), usdtmTradablePair, "", 0)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelUMConditionalOrder(t.Context(), usdtmTradablePair, "", 2000)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelCMConditionalOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelCMConditionalOrder(t.Context(), currency.EMPTYPAIR, "", 1231231)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.CancelCMConditionalOrder(t.Context(), usdtmTradablePair, "", 0)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelCMConditionalOrder(t.Context(), usdtmTradablePair, "", 1231231)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelAllUMOpenConditionalOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllUMOpenConditionalOrders(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelAllUMOpenConditionalOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelAllCMOpenConditionalOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllCMOpenConditionalOrders(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelAllCMOpenConditionalOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetUMOrder(t.Context(), currency.EMPTYPAIR, "", "1234")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetUMOrder(t.Context(), usdtmTradablePair, "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMOrder(t.Context(), usdtmTradablePair, "", "1234")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMOpenOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetUMOpenOrder(t.Context(), currency.EMPTYPAIR, "", "1234")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetUMOpenOrder(t.Context(), usdtmTradablePair, "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMOpenOrder(t.Context(), usdtmTradablePair, "", "1234")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllUMOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllUMOpenOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllUMOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetAllUMOrders(t.Context(), &GetAllUMOrdersRequest{Symbol: usdtmTradablePair, StartTime: endTime, EndTime: startTime, StartingOrderID: "1", Limit: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllUMOrders(t.Context(), &GetAllUMOrdersRequest{Symbol: usdtmTradablePair, StartTime: startTime, EndTime: endTime, StartingOrderID: "1", Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetCMOrder(t.Context(), currency.EMPTYPAIR, "", "1234")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMOrder(t.Context(), coinmTradablePair, "", "1234")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMOpenOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetCMOpenOrder(t.Context(), currency.EMPTYPAIR, "", "1234")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetCMOpenOrder(t.Context(), coinmTradablePair, "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMOpenOrder(t.Context(), coinmTradablePair, "", "1234")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllCMOpenOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetAllCMOpenOrders(t.Context(), currency.EMPTYPAIR, "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllCMOpenOrders(t.Context(), coinmTradablePair, "BTCUSD")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllCMOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetAllCMOrders(t.Context(), &GetAllCMOrdersRequest{Limit: 20})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	startTime, endTime := getTime()
	_, err = e.GetAllCMOrders(t.Context(), &GetAllCMOrdersRequest{Symbol: coinmTradablePair, StartTime: endTime, EndTime: startTime, Pair: "BTCUSD", Limit: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllCMOrders(t.Context(), &GetAllCMOrdersRequest{Symbol: coinmTradablePair, StartTime: startTime, EndTime: endTime, Pair: "BTCUSD", Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOpenUMConditionalOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetOpenUMConditionalOrder(t.Context(), usdtmTradablePair, "", 0)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOpenUMConditionalOrder(t.Context(), usdtmTradablePair, "newClientStrategyId", 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllUMOpenConditionalOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllUMOpenConditionalOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllUMConditionalOrderHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllUMConditionalOrderHistory(t.Context(), usdtmTradablePair, "abc", 123432423)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllUMConditionalOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllUMConditionalOrders(t.Context(), &GetAllUMConditionalOrdersRequest{Symbol: usdtmTradablePair, EndTime: time.Now(), Limit: 123432423})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOpenCMConditionalOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetOpenCMConditionalOrder(t.Context(), coinmTradablePair, "", 0)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOpenCMConditionalOrder(t.Context(), coinmTradablePair, "", 1234)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllCMOpenConditionalOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllCMOpenConditionalOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllCMConditionalOrderHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllCMConditionalOrderHistory(t.Context(), usdtmTradablePair, "abc", 123432423)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllCMConditionalOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllCMConditionalOrders(t.Context(), &GetAllCMConditionalOrdersRequest{Symbol: usdtmTradablePair, EndTime: time.Now(), Limit: 123432423})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginAccountOrder(t.Context(), currency.EMPTYPAIR, "", "12434")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetMarginAccountOrder(t.Context(), assetToTradablePairMap[asset.Margin], "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountOrder(t.Context(), assetToTradablePairMap[asset.Margin], "", "12434")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCurrentMarginOpenOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCurrentMarginOpenOrders(t.Context(), assetToTradablePairMap[asset.Margin])
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllMarginAccountOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetAllMarginAccountOrders(t.Context(), &GetAllMarginAccountOrdersRequest{Limit: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	startTime, endTime := getTime()
	_, err = e.GetAllMarginAccountOrders(t.Context(), &GetAllMarginAccountOrdersRequest{Symbol: assetToTradablePairMap[asset.Margin], StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllMarginAccountOrders(t.Context(), &GetAllMarginAccountOrdersRequest{Symbol: assetToTradablePairMap[asset.Margin], StartTime: startTime, EndTime: endTime, OrderID: "1234", Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountOCO(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountOCO(t.Context(), 0, "123421-abcde")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPMMarginAccountAllOCO(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetPMMarginAccountAllOCO(t.Context(), endTime, startTime, 1, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPMMarginAccountAllOCO(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountsOpenOCO(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountsOpenOCO(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPMMarginAccountTradeList(t *testing.T) {
	t.Parallel()
	_, err := e.GetPMMarginAccountTradeList(t.Context(), &GetPMMarginAccountTradeListRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	startTime, endTime := getTime()
	_, err = e.GetPMMarginAccountTradeList(t.Context(), &GetPMMarginAccountTradeListRequest{Symbol: assetToTradablePairMap[asset.Margin], StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPMMarginAccountTradeList(t.Context(), &GetPMMarginAccountTradeListRequest{Symbol: assetToTradablePairMap[asset.Margin], StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAccountBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccountBalance(t.Context(), currency.EMPTYCODE)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPortfolioMarginAccountInformation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPortfolioMarginAccountInformation(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginMaxBorrow(t *testing.T) {
	t.Parallel()
	_, err := e.GetPMMarginMaxBorrow(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPMMarginMaxBorrow(t.Context(), currency.ETH)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginMaxWithdrawal(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginMaxWithdrawal(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginMaxWithdrawal(t.Context(), currency.BTC)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMPositionInformation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMPositionInformation(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMPositionInformation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMPositionInformation(t.Context(), currency.ETH, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeUMInitialLeverage(t *testing.T) {
	t.Parallel()
	_, err := e.ChangeUMInitialLeverage(t.Context(), currency.EMPTYPAIR, 29)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.ChangeUMInitialLeverage(t.Context(), usdtmTradablePair, 0)
	require.ErrorIs(t, err, order.ErrSubmitLeverageNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeUMInitialLeverage(t.Context(), usdtmTradablePair, 29)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeCMInitialLeverage(t *testing.T) {
	t.Parallel()
	_, err := e.ChangeCMInitialLeverage(t.Context(), currency.EMPTYPAIR, 29)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.ChangeCMInitialLeverage(t.Context(), usdtmTradablePair, 0)
	require.ErrorIs(t, err, order.ErrSubmitLeverageNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeCMInitialLeverage(t.Context(), usdtmTradablePair, 29)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeUMPositionMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeUMPositionMode(t.Context(), true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeCMPositionMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeCMPositionMode(t.Context(), true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMCurrentPositionMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMCurrentPositionMode(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMCurrentPositionMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMCurrentPositionMode(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMAccountTradeList(t *testing.T) {
	t.Parallel()
	_, err := e.GetUMAccountTradeList(t.Context(), &GetUMAccountTradeListRequest{StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24)})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMAccountTradeList(t.Context(), &GetUMAccountTradeListRequest{Symbol: usdtmTradablePair, StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24)})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMAccountTradeList(t *testing.T) {
	t.Parallel()
	_, err := e.GetCMAccountTradeList(t.Context(), &GetCMAccountTradeListRequest{StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24)})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMAccountTradeList(t.Context(), &GetCMAccountTradeListRequest{Symbol: coinmTradablePair, StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24)})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// pair is a documented alternative to symbol on the CM endpoint
	result, err = e.GetCMAccountTradeList(t.Context(), &GetCMAccountTradeListRequest{Pair: "BTCUSD", StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24)})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMNotionalAndLeverageBrackets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMNotionalAndLeverageBrackets(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMNotionalAndLeverageBrackets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMNotionalAndLeverageBrackets(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUsersMarginForceOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUsersMarginForceOrders(t.Context(), time.Now().Add(-time.Hour*24*5), time.Now().Add(-time.Hour*24), 0, 5)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUsersUMForceOrderst(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetUsersUMForceOrders(t.Context(), &GetUsersUMForceOrdersRequest{Symbol: usdtmTradablePair, StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUsersUMForceOrders(t.Context(), &GetUsersUMForceOrdersRequest{Symbol: usdtmTradablePair, StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUsersCMForceOrderst(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetUsersCMForceOrders(t.Context(), &GetUsersCMForceOrdersRequest{Symbol: usdtmTradablePair, StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUsersCMForceOrders(t.Context(), &GetUsersCMForceOrdersRequest{Symbol: usdtmTradablePair, StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPortfolioMarginUMTradingQuantitativeRulesIndicator(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPortfolioMarginUMTradingQuantitativeRulesIndicator(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMUserCommissionRate(t *testing.T) {
	t.Parallel()
	_, err := e.GetUMUserCommissionRate(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMUserCommissionRate(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMUserCommissionRate(t *testing.T) {
	t.Parallel()
	_, err := e.GetCMUserCommissionRate(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMUserCommissionRate(t.Context(), coinmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginLoanRecord(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginLoanRecord(t.Context(), &GetMarginLoanRecordRequest{StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24), Current: 10, Size: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginLoanRecord(t.Context(), &GetMarginLoanRecordRequest{AssetName: currency.ETH, StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24), Current: 10, Size: 1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginRepayRecord(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginRepayRecord(t.Context(), &GetMarginRepayRecordRequest{StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24), Current: 10, Size: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginRepayRecord(t.Context(), &GetMarginRepayRecordRequest{AssetName: currency.ETH, StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24), Current: 10, Size: 1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginBorrowOrLoanInterestHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginBorrowOrLoanInterestHistory(t.Context(), &GetMarginBorrowOrLoanInterestHistoryRequest{AssetName: currency.ETH, StartTime: time.Now().Add(-time.Hour * 24 * 5), EndTime: time.Now().Add(-time.Hour * 24), Current: 10, Size: 1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPortfolioMarginNegativeBalanceInterestHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPortfolioMarginNegativeBalanceInterestHistory(t.Context(), currency.ETH, time.Now().Add(-time.Hour*24*5), time.Now().Add(-time.Hour*24), 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFundAutoCollection(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FundAutoCollection(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFundCollectionByAsset(t *testing.T) {
	t.Parallel()
	_, err := e.FundCollectionByAsset(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FundCollectionByAsset(t.Context(), currency.ETH)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBNBTransferClassic(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.BNBTransferClassic(t.Context(), 0.0001, "TO_UM")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBNBTransfer(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.BNBTransfer(t.Context(), 0.0001, "TO_UM")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMAccountDetail(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMAccountDetail(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeAutoRepayFuturesStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.ChangeAutoRepayFuturesStatus(t.Context(), false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAutoRepayFuturesStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAutoRepayFuturesStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRepayFuturesNegativeBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RepayFuturesNegativeBalance(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMPositionADLQuantileEstimation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMPositionADLQuantileEstimation(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCMPositionADLQuantileEstimation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMPositionADLQuantileEstimation(t.Context(), coinmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserRateLimits(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserRateLimits(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestAdjustCrossMarginMaxLeverage(t *testing.T) {
	t.Parallel()
	_, err := e.AdjustCrossMarginMaxLeverage(t.Context(), 0)
	require.ErrorIs(t, err, order.ErrSubmitLeverageNotSupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.AdjustCrossMarginMaxLeverage(t.Context(), 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCrossMarginTransferHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetCrossMarginTransferHistory(t.Context(), &GetCrossMarginTransferHistoryRequest{AssetName: currency.ETH, TransferType: "ROLL_IN", StartTime: endTime, EndTime: startTime, Current: 10, Size: 30})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCrossMarginTransferHistory(t.Context(), &GetCrossMarginTransferHistoryRequest{AssetName: currency.ETH, TransferType: "ROLL_IN", StartTime: startTime, EndTime: endTime, Current: 10, Size: 30})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewMarginAccountOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewMarginAccountOCOOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &MarginOCOOrderRequest{
		IsIsolated: true,
	}
	_, err = e.NewMarginAccountOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	arg.Symbol = usdtmTradablePair
	_, err = e.NewMarginAccountOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Buy.String()
	_, err = e.NewMarginAccountOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.Quantity = 0.000001
	_, err = e.NewMarginAccountOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	arg.Price = 12312
	_, err = e.NewMarginAccountOCOOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewMarginAccountOCOOrder(t.Context(), &MarginOCOOrderRequest{
		Symbol:    usdtmTradablePair,
		Side:      order.Buy.String(),
		Quantity:  0.000001,
		Price:     12312,
		StopPrice: 12345,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelMarginAccountOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelMarginAccountOCOOrder(t.Context(), &CancelMarginAccountOCOOrderRequest{ListClientOrderID: "12345678", IsIsolated: true})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelMarginAccountOCOOrder(t.Context(), &CancelMarginAccountOCOOrderRequest{Symbol: assetToTradablePairMap[asset.Margin], ListClientOrderID: "12345678", IsIsolated: true})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountOCOOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountOCOOrder(t.Context(), assetToTradablePairMap[asset.Margin], "12345", 0, false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountAllOCO(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetMarginAccountAllOCO(t.Context(), &GetMarginAccountAllOCORequest{Symbol: assetToTradablePairMap[asset.Margin], IsIsolated: true, StartTime: endTime, EndTime: startTime, Limit: 12})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountAllOCO(t.Context(), &GetMarginAccountAllOCORequest{Symbol: assetToTradablePairMap[asset.Margin], IsIsolated: true, StartTime: startTime, EndTime: endTime, Limit: 12})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountsOpenOCOOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginAccountsOpenOCOOrder(t.Context(), true, currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountsOpenOCOOrder(t.Context(), true, usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAccountTradeList(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginAccountTradeList(t.Context(), &GetMarginAccountTradeListRequest{IsIsolated: true})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	startTime, endTime := getTime()
	_, err = e.GetMarginAccountTradeList(t.Context(), &GetMarginAccountTradeListRequest{Symbol: assetToTradablePairMap[asset.Margin], IsIsolated: true, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAccountTradeList(t.Context(), &GetMarginAccountTradeListRequest{Symbol: assetToTradablePairMap[asset.Margin], IsIsolated: true, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMaxBorrow(t *testing.T) {
	t.Parallel()
	_, err := e.GetMaxBorrow(t.Context(), currency.EMPTYCODE, assetToTradablePairMap[asset.Margin])
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMaxBorrow(t.Context(), currency.ETH, assetToTradablePairMap[asset.Margin])
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMaxTransferOutAmount(t *testing.T) {
	t.Parallel()
	_, err := e.GetMaxTransferOutAmount(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMaxTransferOutAmount(t.Context(), currency.ETH, currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSummaryOfMarginAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSummaryOfMarginAccount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIsolatedMarginAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetIsolatedMarginAccountInfo(t.Context(), []string{usdtmTradablePair.String()})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDisableIsolatedMarginAccount(t *testing.T) {
	t.Parallel()
	_, err := e.DisableIsolatedMarginAccount(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.DisableIsolatedMarginAccount(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableIsolatedMarginAccount(t *testing.T) {
	t.Parallel()
	_, err := e.EnableIsolatedMarginAccount(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.EnableIsolatedMarginAccount(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEnabledIsolatedMarginAccountLimit(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetEnabledIsolatedMarginAccountLimit(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllIsolatedMarginSymbols(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllIsolatedMarginSymbols(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestToggleBNBBurn(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ToggleBNBBurn(t.Context(), true, false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBNBBurnStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.GetBNBBurnStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginInterestRateHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginInterestRateHistory(t.Context(), currency.EMPTYCODE, 0, time.Time{}, time.Time{})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	startTime, endTime := getTime()
	_, err = e.GetMarginInterestRateHistory(t.Context(), currency.ETH, 0, endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginInterestRateHistory(t.Context(), currency.ETH, 0, startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCrossMarginFeeData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCrossMarginFeeData(t.Context(), 0, currency.BTC)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIsolatedMaringFeeData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetIsolatedMaringFeeData(t.Context(), 1, usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIsolatedMarginTierData(t *testing.T) {
	t.Parallel()
	_, err := e.GetIsolatedMarginTierData(t.Context(), currency.EMPTYPAIR, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetIsolatedMarginTierData(t.Context(), usdtmTradablePair, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCurrencyMarginOrderCountUsage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCurrencyMarginOrderCountUsage(t.Context(), true, usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCrossMarginCollateralRatio(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCrossMarginCollateralRatio(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSmallLiabilityExchangeCoinList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSmallLiabilityExchangeCoinList(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginSmallLiabilityExchange(t *testing.T) {
	t.Parallel()
	_, err := e.MarginSmallLiabilityExchange(t.Context(), []string{})
	require.ErrorIs(t, err, errEmptyCurrencyCodes)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.MarginSmallLiabilityExchange(t.Context(), []string{"BTC", "ETH"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSmallLiabilityExchangeHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetSmallLiabilityExchangeHistory(t.Context(), 0, 10, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errPageNumberRequired)
	_, err = e.GetSmallLiabilityExchangeHistory(t.Context(), 1, 0, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errPageSizeRequired)

	startTime, endTime := getTime()
	_, err = e.GetSmallLiabilityExchangeHistory(t.Context(), 1, 10, endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSmallLiabilityExchangeHistory(t.Context(), 1, 10, startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFutureHourlyInterestRate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFutureHourlyInterestRate(t.Context(), []string{"BTC", "ETH"}, true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCrossOrIsolatedMarginCapitalFlow(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetCrossOrIsolatedMarginCapitalFlow(t.Context(), &GetCrossOrIsolatedMarginCapitalFlowRequest{AssetName: currency.ETH, FlowType: capitalFlowBorrow, StartTime: endTime, EndTime: startTime, FromID: 10, Limit: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCrossOrIsolatedMarginCapitalFlow(t.Context(), &GetCrossOrIsolatedMarginCapitalFlowRequest{AssetName: currency.ETH, FlowType: capitalFlowBorrow, StartTime: startTime, EndTime: endTime, FromID: 10, Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetTokensOrSymbolsDelistSchedule(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetTokensOrSymbolsDelistSchedule(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAvailableInventory(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginAvailableInventory(t.Context(), "")
	require.ErrorIs(t, err, margin.ErrInvalidMarginType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAvailableInventory(t.Context(), "ISOLATED")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginManualLiquidiation(t *testing.T) {
	t.Parallel()
	_, err := e.MarginManualLiquidiation(t.Context(), "", currency.EMPTYPAIR)
	require.ErrorIs(t, err, margin.ErrInvalidMarginType)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.MarginManualLiquidiation(t.Context(), "ISOLATED", currency.EMPTYPAIR)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLiabilityCoinLeverageBracketInCrossMarginProMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLiabilityCoinLeverageBracketInCrossMarginProMode(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSimpleEarnFlexibleProductList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSimpleEarnFlexibleProductList(t.Context(), currency.BTC, 2, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSimpleEarnLockedProducts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSimpleEarnLockedProducts(t.Context(), currency.BTC, 2, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubscribeToFlexibleProducts(t *testing.T) {
	t.Parallel()
	_, err := e.SubscribeToFlexibleProducts(t.Context(), "", "FUND", 1, false)
	require.ErrorIs(t, err, errProductIDRequired)
	_, err = e.SubscribeToFlexibleProducts(t.Context(), "project-id", "FUND", 0, false)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubscribeToFlexibleProducts(t.Context(), "product-id", "FUND", 1, true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubscribeToLockedProducts(t *testing.T) {
	t.Parallel()
	_, err := e.SubscribeToLockedProducts(t.Context(), "", "SPOT", 1, false)
	require.ErrorIs(t, err, errProjectIDRequired)
	_, err = e.SubscribeToLockedProducts(t.Context(), "project-id", "SPOT", 0, false)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubscribeToLockedProducts(t.Context(), "project-id", "SPOT", 1, false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRedeemFlexibleProduct(t *testing.T) {
	t.Parallel()
	_, err := e.RedeemFlexibleProduct(t.Context(), "", "FUND", true, 0.1234)
	require.ErrorIs(t, err, errProductIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RedeemFlexibleProduct(t.Context(), "product-id", "FUND", true, 0.1234)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRedeemLockedProduct(t *testing.T) {
	t.Parallel()
	_, err := e.RedeemLockedProduct(t.Context(), 0)
	require.ErrorIs(t, err, errPositionIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RedeemLockedProduct(t.Context(), 12345)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFlexibleProductPosition(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFlexibleProductPosition(t.Context(), currency.BTC, "", 0, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLockedProductPosition(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLockedProductPosition(t.Context(), &GetLockedProductPositionRequest{AssetName: currency.ETH, Size: 12})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSimpleAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.SimpleAccount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFlexibleSubscriptionRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFlexibleSubscriptionRecord(t.Context(), &GetFlexibleSubscriptionRecordRequest{AssetName: currency.ETH, StartTime: endTime, EndTime: startTime, Size: 12})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFlexibleSubscriptionRecord(t.Context(), &GetFlexibleSubscriptionRecordRequest{AssetName: currency.ETH, StartTime: startTime, EndTime: endTime, Current: 1, Size: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLockedSubscriptionsRecords(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetLockedSubscriptionsRecords(t.Context(), &GetLockedSubscriptionsRecordsRequest{AssetName: currency.ETH, StartTime: endTime, EndTime: startTime, Current: 1, Size: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLockedSubscriptionsRecords(t.Context(), &GetLockedSubscriptionsRecordsRequest{AssetName: currency.ETH, StartTime: startTime, EndTime: endTime, Current: 1, Size: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFlexibleRedemptionRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFlexibleRedemptionRecord(t.Context(), &GetFlexibleRedemptionRecordRequest{RedeemID: "1234", AssetName: currency.LTC, StartTime: endTime, EndTime: startTime, Size: 12})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFlexibleRedemptionRecord(t.Context(), &GetFlexibleRedemptionRecordRequest{RedeemID: "1234", AssetName: currency.LTC, StartTime: startTime, EndTime: endTime, Size: 12})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLockedRedemptionRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetLockedRedemptionRecord(t.Context(), &GetLockedRedemptionRecordRequest{RedeemID: "1234", AssetName: currency.LTC, StartTime: endTime, EndTime: startTime, Size: 12})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLockedRedemptionRecord(t.Context(), &GetLockedRedemptionRecordRequest{RedeemID: "1234", AssetName: currency.LTC, StartTime: startTime, EndTime: endTime, Size: 12})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFlexibleRewardHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFlexibleRewardHistory(t.Context(), &GetFlexibleRewardHistoryRequest{ProductID: "product-type", AssetName: currency.BTC, StartTime: endTime, EndTime: startTime, Current: 1, Size: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFlexibleRewardHistory(t.Context(), &GetFlexibleRewardHistoryRequest{ProductID: "product-type", AssetName: currency.BTC, StartTime: startTime, EndTime: endTime, Current: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLockedRewardHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetLockedRewardHistory(t.Context(), &GetLockedRewardHistoryRequest{PositionID: "12345", AssetName: currency.BTC, StartTime: endTime, EndTime: startTime, Current: 10, Size: 40})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLockedRewardHistory(t.Context(), &GetLockedRewardHistoryRequest{PositionID: "12345", AssetName: currency.BTC, StartTime: startTime, EndTime: endTime, Current: 10, Size: 40})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetFlexibleAutoSusbcribe(t *testing.T) {
	t.Parallel()
	_, err := e.SetFlexibleAutoSusbcribe(t.Context(), "", true)
	require.ErrorIs(t, err, errProductIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SetFlexibleAutoSusbcribe(t.Context(), "product-id", true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetLockedAutoSubscribe(t *testing.T) {
	t.Parallel()
	_, err := e.SetLockedAutoSubscribe(t.Context(), "", true)
	require.ErrorIs(t, err, errPositionIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SetLockedAutoSubscribe(t.Context(), "position-id", true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFlexiblePersonalLeftQuota(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFlexiblePersonalLeftQuota(t.Context(), "12345")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLockedPersonalLeftQuota(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLockedPersonalLeftQuota(t.Context(), "12345")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFlexibleSubscriptionPreview(t *testing.T) {
	t.Parallel()
	_, err := e.GetFlexibleSubscriptionPreview(t.Context(), "", 0.0001)
	require.ErrorIs(t, err, errProductIDRequired)
	_, err = e.GetFlexibleSubscriptionPreview(t.Context(), "1234", 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFlexibleSubscriptionPreview(t.Context(), "1234", 0.0001)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLockedSubscriptionPreview(t *testing.T) {
	t.Parallel()
	_, err := e.GetLockedSubscriptionPreview(t.Context(), "", 0.1234, false)
	require.ErrorIs(t, err, errProjectIDRequired)
	_, err = e.GetLockedSubscriptionPreview(t.Context(), "12345", 0, false)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLockedSubscriptionPreview(t.Context(), "12345", 0.1234, false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetLockedProductRedeemOption(t *testing.T) {
	t.Parallel()
	_, err := e.SetLockedProductRedeemOption(t.Context(), "", "abcdefg")
	require.ErrorIs(t, err, errPositionIDRequired)
	_, err = e.SetLockedProductRedeemOption(t.Context(), "12345", "")
	require.ErrorIs(t, err, errRedemptionAccountRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.SetLockedProductRedeemOption(t.Context(), "12345", "abcdefg")
	assert.NoError(t, err)
}

func TestGetSimpleEarnRatehistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSimpleEarnRatehistory(t.Context(), &GetSimpleEarnRatehistoryRequest{ProjectID: "project-id", StartTime: endTime, EndTime: startTime, Size: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSimpleEarnRatehistory(t.Context(), &GetSimpleEarnRatehistoryRequest{ProjectID: "project-id", StartTime: startTime, EndTime: endTime, Current: 1, Size: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSimpleEarnCollateralRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSimpleEarnCollateralRecord(t.Context(), &GetSimpleEarnCollateralRecordRequest{ProductID: "project-id", StartTime: endTime, EndTime: startTime, Size: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSimpleEarnCollateralRecord(t.Context(), &GetSimpleEarnCollateralRecordRequest{ProductID: "project-id", StartTime: startTime, EndTime: endTime.Add(-time.Hour * 2), Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDualInvestmentProductList(t *testing.T) {
	t.Parallel()
	_, err := e.GetDualInvestmentProductList(t.Context(), &GetDualInvestmentProductListRequest{ExerciseCoin: currency.BTC, InvestCoin: currency.ETH})
	require.ErrorIs(t, err, errOptionTypeRequired)
	_, err = e.GetDualInvestmentProductList(t.Context(), &GetDualInvestmentProductListRequest{OptionType: "CALL", InvestCoin: currency.ETH})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.GetDualInvestmentProductList(t.Context(), &GetDualInvestmentProductListRequest{OptionType: "CALL", ExerciseCoin: currency.BTC})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.GetDualInvestmentProductList(t.Context(), &GetDualInvestmentProductListRequest{OptionType: "CALL", ExerciseCoin: currency.BTC, InvestCoin: currency.ETH})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubscribeDualInvestmentProducts(t *testing.T) {
	t.Parallel()
	_, err := e.SubscribeDualInvestmentProducts(t.Context(), "", "order-id", "STANDARD", 0.1)
	require.ErrorIs(t, err, errProductIDRequired)
	_, err = e.SubscribeDualInvestmentProducts(t.Context(), "1234", "", "STANDARD", 0.1)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.SubscribeDualInvestmentProducts(t.Context(), "1234", "order-id", "STANDARD", 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.SubscribeDualInvestmentProducts(t.Context(), "1234", "order-id", "", 1)
	require.ErrorIs(t, err, errPlanTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubscribeDualInvestmentProducts(t.Context(), "1234", "order-id", "STANDARD", 0.1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDualInvestmentPositions(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDualInvestmentPositions(t.Context(), "PURCHASE_FAIL", 0, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCheckDualInvestmentAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CheckDualInvestmentAccounts(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeAutoCompoundStatus(t *testing.T) {
	t.Parallel()
	_, err := e.ChangeAutoCompoundStatus(t.Context(), "", "STANDARD")
	require.ErrorIs(t, err, errPositionIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeAutoCompoundStatus(t.Context(), "123456789", "STANDARD")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetTargetAssetList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetTargetAssetList(t.Context(), currency.BTC, 10, 40)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetTargetAssetROIData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetTargetAssetROIData(t.Context(), currency.ETH, "THREE_YEAR")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllSourceAssetAndTargetAsset(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAllSourceAssetAndTargetAsset(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSourceAssetList(t *testing.T) {
	t.Parallel()
	_, err := e.GetSourceAssetList(t.Context(), &GetSourceAssetListRequest{TargetAsset: currency.BTC, IndexID: 123, SourceType: "MAIN_SITE", FlexibleAllowedToUse: true})
	require.ErrorIs(t, err, errUsageTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSourceAssetList(t.Context(), &GetSourceAssetListRequest{TargetAsset: currency.BTC, IndexID: 123, UsageType: "RECURRING", SourceType: "MAIN_SITE", FlexibleAllowedToUse: true})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestInvestmentPlanCreation(t *testing.T) {
	t.Parallel()
	_, err := e.InvestmentPlanCreation(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &InvestmentPlanRequest{}
	_, err = e.InvestmentPlanCreation(t.Context(), arg)
	require.ErrorIs(t, err, errSourceTypeRequired)

	arg.SourceType = "MAIN_SITE"
	_, err = e.InvestmentPlanCreation(t.Context(), arg)
	require.ErrorIs(t, err, errPlanTypeRequired)

	arg.PlanType = "SINGLE"
	_, err = e.InvestmentPlanCreation(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.SubscriptionAmount = 4
	_, err = e.InvestmentPlanCreation(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidSubscriptionStartTime)

	arg.SubscriptionStartDay = 1
	arg.SubscriptionStartTime = 8
	_, err = e.InvestmentPlanCreation(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.SourceAsset = currency.USDT
	_, err = e.InvestmentPlanCreation(t.Context(), arg)
	require.ErrorIs(t, err, errPortfolioDetailRequired)

	arg.Details = []PortfolioDetail{{}}
	_, err = e.InvestmentPlanCreation(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.Details = []PortfolioDetail{{TargetAsset: currency.BTC, Percentage: -1}}
	_, err = e.InvestmentPlanCreation(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidPercentageAmount)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.InvestmentPlanCreation(t.Context(), &InvestmentPlanRequest{
		SourceType:            "MAIN_SITE",
		PlanType:              "SINGLE",
		SubscriptionAmount:    4,
		SubscriptionCycle:     "H4",
		SubscriptionStartTime: 8,
		SourceAsset:           currency.USDT,
		Details: []PortfolioDetail{
			{
				TargetAsset: currency.ETH,
				Percentage:  12,
			},
			{
				TargetAsset: currency.ETH,
				Percentage:  20,
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestInvestmentPlanAdjustment(t *testing.T) {
	t.Parallel()
	_, err := e.InvestmentPlanAdjustment(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &AdjustInvestmentPlan{}
	_, err = e.InvestmentPlanAdjustment(t.Context(), arg)
	require.ErrorIs(t, err, errPlanIDRequired)

	arg.PlanID = 1234232
	_, err = e.InvestmentPlanAdjustment(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.SubscriptionAmount = 4
	_, err = e.InvestmentPlanAdjustment(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidSubscriptionCycle)

	arg.SubscriptionCycle = "H4"
	arg.SubscriptionStartTime = -1
	_, err = e.InvestmentPlanAdjustment(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidSubscriptionStartTime)

	arg.SubscriptionStartTime = 8
	_, err = e.InvestmentPlanAdjustment(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.SourceAsset = currency.USDT
	_, err = e.InvestmentPlanAdjustment(t.Context(), arg)
	require.ErrorIs(t, err, errPortfolioDetailRequired)

	arg.Details = []PortfolioDetail{{}}
	_, err = e.InvestmentPlanAdjustment(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.Details = []PortfolioDetail{{TargetAsset: currency.BTC, Percentage: -1}}
	_, err = e.InvestmentPlanAdjustment(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidPercentageAmount)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.InvestmentPlanAdjustment(t.Context(), &AdjustInvestmentPlan{
		PlanID:                1234232,
		SubscriptionAmount:    4,
		SubscriptionCycle:     "H4",
		SubscriptionStartTime: 8,
		SourceAsset:           currency.USDT,
		Details: []PortfolioDetail{
			{
				TargetAsset: currency.ETH,
				Percentage:  12,
			},
			{
				TargetAsset: currency.ETH,
				Percentage:  20,
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangePlanStatus(t *testing.T) {
	t.Parallel()
	_, err := e.ChangePlanStatus(t.Context(), 0, "PAUSED")
	require.ErrorIs(t, err, errPlanIDRequired)

	_, err = e.ChangePlanStatus(t.Context(), 12345, "")
	require.ErrorIs(t, err, errPlanStatusRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangePlanStatus(t.Context(), 12345, "PAUSED")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetListOfPlans(t *testing.T) {
	t.Parallel()
	_, err := e.GetListOfPlans(t.Context(), "")
	require.ErrorIs(t, err, errPlanTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetListOfPlans(t.Context(), "SINGLE")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetHoldingDetailsOfPlan(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetHoldingDetailsOfPlan(t.Context(), 1234, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubscriptionsTransactionHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSubscriptionsTransactionHistory(t.Context(), &GetSubscriptionsTransactionHistoryRequest{PlanID: 1232, Size: 20, StartTime: endTime, EndTime: startTime, TargetAsset: currency.BTC, PlanType: "PORTFOLIO"})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubscriptionsTransactionHistory(t.Context(), &GetSubscriptionsTransactionHistoryRequest{PlanID: 1232, Size: 20, StartTime: startTime, EndTime: endTime, TargetAsset: currency.BTC, PlanType: "PORTFOLIO"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIndexDetail(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexDetail(t.Context(), 0)
	require.ErrorIs(t, err, errIndexIDIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetIndexDetail(t.Context(), 1234)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIndexLinkedPlanPositionDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexLinkedPlanPositionDetails(t.Context(), 0)
	require.ErrorIs(t, err, errIndexIDIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetIndexLinkedPlanPositionDetails(t.Context(), 123)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestOneTimeTransaction(t *testing.T) {
	t.Parallel()
	_, err := e.OneTimeTransaction(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OneTimeTransactionRequest{}
	_, err = e.OneTimeTransaction(t.Context(), arg)
	require.ErrorIs(t, err, errSourceTypeRequired)

	arg.SourceType = "MAIN_SITE"
	_, err = e.OneTimeTransaction(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	arg.SubscriptionAmount = 12
	_, err = e.OneTimeTransaction(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.SourceAsset = currency.USDT
	_, err = e.OneTimeTransaction(t.Context(), arg)
	require.ErrorIs(t, err, errPortfolioDetailRequired)

	_, err = e.OneTimeTransaction(t.Context(), arg)
	require.ErrorIs(t, err, errPortfolioDetailRequired)

	arg.Details = []PortfolioDetail{{}}
	_, err = e.OneTimeTransaction(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.Details = []PortfolioDetail{{TargetAsset: currency.BTC}}
	_, err = e.OneTimeTransaction(t.Context(), arg)
	require.ErrorIs(t, err, errInvalidPercentageAmount)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.OneTimeTransaction(t.Context(), &OneTimeTransactionRequest{
		SourceType:         "MAIN_SITE",
		SubscriptionAmount: 12,
		SourceAsset:        currency.USDT,
		Details: []PortfolioDetail{
			{
				TargetAsset: currency.BTC,
				Percentage:  30,
			},
			{
				TargetAsset: currency.ETH,
				Percentage:  50,
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOneTimeTransactionStatus(t *testing.T) {
	t.Parallel()
	_, err := e.GetOneTimeTransactionStatus(t.Context(), 0, "")
	require.ErrorIs(t, err, errTransactionIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOneTimeTransactionStatus(t.Context(), 1234, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestIndexLinkedPlanRedemption(t *testing.T) {
	t.Parallel()
	_, err := e.IndexLinkedPlanRedemption(t.Context(), 0, 30, "")
	require.ErrorIs(t, err, errIndexIDIsRequired)

	_, err = e.IndexLinkedPlanRedemption(t.Context(), 12333, 0, "")
	require.ErrorIs(t, err, errInvalidPercentageAmount)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.IndexLinkedPlanRedemption(t.Context(), 12333, 30, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIndexLinkedPlanRedemption(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexLinkedPlanRedemption(t.Context(), &GetIndexLinkedPlanRedemptionRequest{AssetName: currency.ETH, Size: 10})
	require.ErrorIs(t, err, errRequestIDRequired)

	startTime, endTime := getTime()
	_, err = e.GetIndexLinkedPlanRedemption(t.Context(), &GetIndexLinkedPlanRedemptionRequest{RequestID: "123123", StartTime: endTime, EndTime: startTime, AssetName: currency.ETH, Size: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.GetIndexLinkedPlanRedemption(t.Context(), &GetIndexLinkedPlanRedemptionRequest{RequestID: "123123", StartTime: startTime, EndTime: endTime, AssetName: currency.ETH, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetIndexLinkedPlanRebalanceDetails(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetIndexLinkedPlanRebalanceDetails(t.Context(), endTime, startTime, 1, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetIndexLinkedPlanRebalanceDetails(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSusbcribeETHStakingV2(t *testing.T) {
	t.Parallel()
	_, err := e.SubscribeETHStaking(t.Context(), 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.SubscribeETHStaking(t.Context(), 0.123)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRedeemETH(t *testing.T) {
	t.Parallel()
	_, err := e.RedeemETH(t.Context(), 0, currency.ETH)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RedeemETH(t.Context(), 0.123, currency.ETH)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetETHStakingHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetETHStakingHistory(t.Context(), endTime, startTime, 1, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetETHStakingHistory(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetETHRedemptionHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetETHRedemptionHistory(t.Context(), endTime, startTime, 0, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetETHRedemptionHistory(t.Context(), startTime, endTime, 0, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBETHRewardsDistributionHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetBETHRewardsDistributionHistory(t.Context(), endTime, startTime, 0, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBETHRewardsDistributionHistory(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCurrentETHStakingQuota(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCurrentETHStakingQuota(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWBETHRateHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetWBETHRateHistory(t.Context(), endTime, startTime, 1, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetWBETHRateHistory(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetETHStakingAccountV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetETHStakingAccount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWrapBETH(t *testing.T) {
	t.Parallel()
	_, err := e.WrapBETH(t.Context(), 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.WrapBETH(t.Context(), 0.001)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWBETHWrapHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetWBETHWrapHistory(t.Context(), endTime, startTime, 0, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetWBETHWrapHistory(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWBETHUnwrapHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetWBETHUnwrapHistory(t.Context(), endTime, startTime, 0, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetWBETHUnwrapHistory(t.Context(), startTime, endTime, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWBETHRewardHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetWBETHRewardHistory(t.Context(), endTime, startTime, 0, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetWBETHRewardHistory(t.Context(), startTime, endTime, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSOLStakingAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSOLStakingAccount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSOLStakingQuotaDetails(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSOLStakingQuotaDetails(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubscribeToSOLStaking(t *testing.T) {
	t.Parallel()
	_, err := e.SubscribeToSOLStaking(t.Context(), 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubscribeToSOLStaking(t.Context(), 1.2)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRedeemSOL(t *testing.T) {
	t.Parallel()
	_, err := e.RedeemSOL(t.Context(), 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RedeemSOL(t.Context(), 1.2)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestClaimBoostRewards(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ClaimBoostRewards(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSOLStakingHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSOLStakingHistory(t.Context(), endTime, startTime, 0, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSOLStakingHistory(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSOLRedemptionHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSOLRedemptionHistory(t.Context(), endTime, startTime, 1, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSOLRedemptionHistory(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBNSOLRewardsHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetBNSOLRewardsHistory(t.Context(), endTime, startTime, 1, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBNSOLRewardsHistory(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBNSOLRateHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetBNSOLRateHistory(t.Context(), endTime, startTime, 1, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBNSOLRateHistory(t.Context(), startTime, endTime, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBoostRewardsHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetBoostRewardsHistory(t.Context(), &GetBoostRewardsHistoryRequest{StartTime: startTime, EndTime: endTime, Size: 100})
	require.ErrorIs(t, err, errRewardTypeMissing)
	_, err = e.GetBoostRewardsHistory(t.Context(), &GetBoostRewardsHistoryRequest{RewardType: "CLAIM", StartTime: endTime, EndTime: startTime, Size: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBoostRewardsHistory(t.Context(), &GetBoostRewardsHistoryRequest{RewardType: "CLAIM", StartTime: startTime, EndTime: endTime, Current: 1, Size: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUnclaimedRewards(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUnclaimedRewards(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestAcquiringAlgorithm(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.AcquiringAlgorithm(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCoinNames(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCoinNames(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDetailMinerList(t *testing.T) {
	t.Parallel()
	_, err := e.GetDetailMinerList(t.Context(), "sha256", "", "bhdc1.16A10404B")
	require.ErrorIs(t, err, errNameRequired)

	_, err = e.GetDetailMinerList(t.Context(), "", "sams", "bhdc1.16A10404B")
	require.ErrorIs(t, err, errTransferAlgorithmRequired)

	_, err = e.GetDetailMinerList(t.Context(), "sha256", "sams", "")
	require.ErrorIs(t, err, errNameRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDetailMinerList(t.Context(), "sha256", "sams", "bhdc1.16A10404B")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMinersList(t *testing.T) {
	t.Parallel()
	_, err := e.GetMinersList(t.Context(), &GetMinersListRequest{UserName: "sams", SortInNegativeSequence: true, SortColumn: 10, WorkerStatus: 10})
	require.ErrorIs(t, err, errTransferAlgorithmRequired)
	_, err = e.GetMinersList(t.Context(), &GetMinersListRequest{Algorithm: "sha256", SortInNegativeSequence: true, SortColumn: 10, WorkerStatus: 10})
	require.ErrorIs(t, err, errNameRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMinersList(t.Context(), &GetMinersListRequest{Algorithm: "sha256", UserName: "sams", SortInNegativeSequence: true, SortColumn: 10, WorkerStatus: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEarningList(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetEarningList(t.Context(), &GetEarningListRequest{TransferAlgorithm: "sha256", UserName: "sams", Coin: currency.ETH, StartDate: endTime, EndDate: startTime, PageSize: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetEarningList(t.Context(), &GetEarningListRequest{TransferAlgorithm: "sha256", UserName: "sams", Coin: currency.ETH, StartDate: startTime, EndDate: endTime, PageIndex: 1, PageSize: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestExtraBonousList(t *testing.T) {
	t.Parallel()
	_, err := e.ExtraBonousList(t.Context(), &ExtraBonousListRequest{UserName: "sams", Coin: currency.ETH, PageSize: 10})
	require.ErrorIs(t, err, errTransferAlgorithmRequired)
	_, err = e.ExtraBonousList(t.Context(), &ExtraBonousListRequest{TransferAlgorithm: "sha256", Coin: currency.ETH, PageSize: 10})
	require.ErrorIs(t, err, errUsernameRequired)

	startTime, endTime := getTime()
	_, err = e.ExtraBonousList(t.Context(), &ExtraBonousListRequest{TransferAlgorithm: "sha256", UserName: "sams", Coin: currency.ETH, StartDate: endTime, EndDate: startTime, PageSize: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.ExtraBonousList(t.Context(), &ExtraBonousListRequest{TransferAlgorithm: "sha256", UserName: "sams", Coin: currency.ETH, StartDate: startTime, EndDate: endTime, PageIndex: 1, PageSize: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetHashrateRescaleList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetHashrateRescaleList(t.Context(), 10, 20)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetHashrateRescaleDetail(t *testing.T) {
	t.Parallel()
	_, err := e.GetHashRateRescaleDetail(t.Context(), "", "sams", 10, 20)
	require.ErrorIs(t, err, errConfigIDRequired)
	_, err = e.GetHashRateRescaleDetail(t.Context(), "168", "", 10, 20)
	require.ErrorIs(t, err, errUsernameRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetHashRateRescaleDetail(t.Context(), "168", "sams", 10, 20)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestHashrateRescaleRequest(t *testing.T) {
	t.Parallel()
	_, err := e.HashRateRescaleRequest(t.Context(), &HashRateRescaleRequestRequest{Algorithm: "sha256", ToPoolUser: "S19pro", HashRate: 10000})
	require.ErrorIs(t, err, errUsernameRequired)
	_, err = e.HashRateRescaleRequest(t.Context(), &HashRateRescaleRequestRequest{UserName: "sams", ToPoolUser: "S19pro", HashRate: 10000})
	require.ErrorIs(t, err, errTransferAlgorithmRequired)
	_, err = e.HashRateRescaleRequest(t.Context(), &HashRateRescaleRequestRequest{UserName: "sams", Algorithm: "sha256", ToPoolUser: "S19pro", HashRate: 10000})
	require.ErrorIs(t, err, common.ErrDateUnset)

	startTime, endTime := getTime()
	_, err = e.HashRateRescaleRequest(t.Context(), &HashRateRescaleRequestRequest{UserName: "sams", Algorithm: "sha256", StartTime: startTime, EndTime: endTime, HashRate: 10000})
	require.ErrorIs(t, err, errAccountRequired)
	_, err = e.HashRateRescaleRequest(t.Context(), &HashRateRescaleRequestRequest{UserName: "sams", Algorithm: "sha256", ToPoolUser: "S19pro", StartTime: startTime, EndTime: endTime})
	require.ErrorIs(t, err, errHashRateRequired)
	_, err = e.HashRateRescaleRequest(t.Context(), &HashRateRescaleRequestRequest{UserName: "sams", Algorithm: "sha256", ToPoolUser: "S19pro", StartTime: endTime, EndTime: startTime, HashRate: 10000})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.HashRateRescaleRequest(t.Context(), &HashRateRescaleRequestRequest{UserName: "sams", Algorithm: "sha256", ToPoolUser: "S19pro", StartTime: startTime, EndTime: endTime, HashRate: 10000})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelHashrateRescaleConfiguration(t *testing.T) {
	t.Parallel()
	_, err := e.CancelHashrateRescaleConfiguration(t.Context(), "", "sams")
	require.ErrorIs(t, err, errConfigIDRequired)
	_, err = e.CancelHashrateRescaleConfiguration(t.Context(), "189", "")
	require.ErrorIs(t, err, errUsernameRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelHashrateRescaleConfiguration(t.Context(), "189", "sams")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestStatisticsList(t *testing.T) {
	t.Parallel()
	_, err := e.StatisticsList(t.Context(), "", "sams")
	require.ErrorIs(t, err, errTransferAlgorithmRequired)
	_, err = e.StatisticsList(t.Context(), "sha256", "")
	require.ErrorIs(t, err, errUsernameRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.StatisticsList(t.Context(), "sha256", "sams")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAccountList(t *testing.T) {
	t.Parallel()
	_, err := e.GetAccountList(t.Context(), "", "sams")
	require.ErrorIs(t, err, errTransferAlgorithmRequired)
	_, err = e.GetAccountList(t.Context(), "sha256", "")
	require.ErrorIs(t, err, errUsernameRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccountList(t.Context(), "sha256", "sams")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMiningAccountEarningRate(t *testing.T) {
	t.Parallel()
	_, err := e.GetMiningAccountEarningRate(t.Context(), &GetMiningAccountEarningRateRequest{PageSize: 10})
	require.ErrorIs(t, err, errTransferAlgorithmRequired)

	startTime, endTime := getTime()
	_, err = e.GetMiningAccountEarningRate(t.Context(), &GetMiningAccountEarningRateRequest{Algorithm: "sha256", StartTime: endTime, EndTime: startTime, PageSize: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMiningAccountEarningRate(t.Context(), &GetMiningAccountEarningRateRequest{Algorithm: "sha256", StartTime: startTime, EndTime: endTime, PageIndex: 1, PageSize: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewFuturesAccountTransfer(t *testing.T) {
	t.Parallel()
	_, err := e.NewFuturesAccountTransfer(t.Context(), currency.EMPTYCODE, 0.001, 2)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.NewFuturesAccountTransfer(t.Context(), currency.ETH, 0, 2)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.NewFuturesAccountTransfer(t.Context(), currency.ETH, 0.001, 0)
	require.ErrorIs(t, err, errTransferTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewFuturesAccountTransfer(t.Context(), currency.ETH, 0.001, 2)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesAccountTransactionHistoryList(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFuturesAccountTransactionHistoryList(t.Context(), &GetFuturesAccountTransactionHistoryListRequest{AssetName: currency.BTC, StartTime: endTime, EndTime: startTime, Current: 10, Size: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.GetFuturesAccountTransactionHistoryList(t.Context(), &GetFuturesAccountTransactionHistoryListRequest{AssetName: currency.BTC, StartTime: startTime, EndTime: endTime, Current: 10, Size: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFutureTickLevelOrderbookHistoricalDataDownloadLink(t *testing.T) {
	t.Parallel()
	_, err := e.GetFutureTickLevelOrderbookHistoricalDataDownloadLink(t.Context(), currency.EMPTYPAIR, "T_DEPTH", time.Time{}, time.Time{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetFutureTickLevelOrderbookHistoricalDataDownloadLink(t.Context(), usdtmTradablePair, "T_DEPTH", time.Time{}, time.Time{})
	require.ErrorIs(t, err, errStartTimeRequired)

	startTime, endTime := getTime()
	_, err = e.GetFutureTickLevelOrderbookHistoricalDataDownloadLink(t.Context(), usdtmTradablePair, "T_DEPTH", endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFutureTickLevelOrderbookHistoricalDataDownloadLink(t.Context(), usdtmTradablePair, "T_DEPTH", startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestVolumeParticipationNewOrder(t *testing.T) {
	t.Parallel()
	_, err := e.VolumeParticipationNewOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)
	_, err = e.VolumeParticipationNewOrder(t.Context(), &VolumeParticipationOrderRequest{Urgency: "HIGH"})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.VolumeParticipationNewOrder(t.Context(), &VolumeParticipationOrderRequest{
		Symbol:       usdtmTradablePair,
		PositionSide: "BOTH",
	})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)
	_, err = e.VolumeParticipationNewOrder(t.Context(), &VolumeParticipationOrderRequest{
		Symbol:       usdtmTradablePair,
		Side:         order.Sell.String(),
		PositionSide: "BOTH",
	})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.VolumeParticipationNewOrder(t.Context(), &VolumeParticipationOrderRequest{
		Symbol:       usdtmTradablePair,
		Side:         order.Sell.String(),
		PositionSide: "BOTH",
		Quantity:     0.012,
	})
	require.ErrorIs(t, err, errPossibleValuesRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.VolumeParticipationNewOrder(t.Context(), &VolumeParticipationOrderRequest{
		Symbol:       usdtmTradablePair,
		Side:         order.Sell.String(),
		PositionSide: "BOTH",
		Quantity:     0.012,
		Urgency:      "HIGH",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestTWAPOrder(t *testing.T) {
	t.Parallel()
	_, err := e.FuturesTWAPOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)
	_, err = e.FuturesTWAPOrder(t.Context(), &TWAPOrderRequest{
		Duration: 1000,
	})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.FuturesTWAPOrder(t.Context(), &TWAPOrderRequest{
		Symbol: usdtmTradablePair,
	})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)
	_, err = e.FuturesTWAPOrder(t.Context(), &TWAPOrderRequest{
		Symbol: usdtmTradablePair,
		Side:   order.Sell.String(),
	})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.FuturesTWAPOrder(t.Context(), &TWAPOrderRequest{
		Symbol:   usdtmTradablePair,
		Side:     order.Sell.String(),
		Quantity: 0.012,
	})
	require.ErrorIs(t, err, errDurationRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.FuturesTWAPOrder(t.Context(), &TWAPOrderRequest{
		Symbol:       usdtmTradablePair,
		Side:         order.Sell.String(),
		PositionSide: "BOTH",
		Quantity:     0.012,
		Duration:     1000,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelAlgoOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelFuturesAlgoOrder(t.Context(), "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelFuturesAlgoOrder(t.Context(), "1234")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCurrentAlgoOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesCurrentAlgoOpenOrders(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetHistoricalAlgoOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFuturesHistoricalAlgoOrders(t.Context(), &GetFuturesHistoricalAlgoOrdersRequest{Symbol: usdtmTradablePair, Side: "BUY", StartTime: endTime, EndTime: startTime, Page: 10, PageSize: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesHistoricalAlgoOrders(t.Context(), &GetFuturesHistoricalAlgoOrdersRequest{Symbol: usdtmTradablePair, Side: "BUY", StartTime: startTime, EndTime: endTime, Page: 10, PageSize: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesSubOrders(t.Context(), 0, 0, 40)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesSubOrders(t.Context(), 1234, 0, 40)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestTWAPNewOrder(t *testing.T) {
	t.Parallel()
	_, err := e.SpotTWAPNewOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)
	_, err = e.SpotTWAPNewOrder(t.Context(), &SpotTWAPOrderRequest{Duration: 86400})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.SpotTWAPNewOrder(t.Context(), &SpotTWAPOrderRequest{Symbol: usdtmTradablePair})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)
	_, err = e.SpotTWAPNewOrder(t.Context(), &SpotTWAPOrderRequest{Symbol: usdtmTradablePair, Side: order.Sell.String()})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.SpotTWAPNewOrder(t.Context(), &SpotTWAPOrderRequest{Symbol: usdtmTradablePair, Side: order.Sell.String(), Quantity: 0.012})
	require.ErrorIs(t, err, errDurationRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SpotTWAPNewOrder(t.Context(), &SpotTWAPOrderRequest{
		Symbol:   usdtmTradablePair,
		Side:     order.Sell.String(),
		Quantity: 0.012,
		Duration: 86400,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelSpotAlgoOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelSpotAlgoOrder(t.Context(), "1234")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCurrentSpotAlgoOpenOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCurrentSpotAlgoOpenOrder(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSpotHistoricalAlgoOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSpotHistoricalAlgoOrders(t.Context(), &GetSpotHistoricalAlgoOrdersRequest{Symbol: usdtmTradablePair, Side: "BUY", StartTime: endTime, EndTime: startTime, Page: 10, PageSize: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotHistoricalAlgoOrders(t.Context(), &GetSpotHistoricalAlgoOrdersRequest{Symbol: usdtmTradablePair, Side: "BUY", StartTime: startTime, EndTime: endTime, Page: 10, PageSize: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSpotSubOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotSubOrders(t.Context(), 0, 1, 40)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotSubOrders(t.Context(), 1234, 1, 40)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetClassicPortfolioMarginAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetClassicPortfolioMarginAccountInfo(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetClassicPortfolioMarginCollateralRate(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetClassicPortfolioMarginCollateralRate(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetClassicPortfolioMarginBankruptacyLoanAmount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetClassicPortfolioMarginBankruptacyLoanAmount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRepayClassicPMBankruptacyLoan(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RepayClassicPMBankruptacyLoan(t.Context(), "SPOT")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetClassicPMNegativeBalanceInterestHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetClassicPMNegativeBalanceInterestHistory(t.Context(), currency.ETH, endTime, startTime, 100)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetClassicPMNegativeBalanceInterestHistory(t.Context(), currency.ETH, startTime, endTime, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPMAssetIndexPrice(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPMAssetIndexPrice(t.Context(), currency.ETH)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestClassicPMFundAutoCollection(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ClassicPMFundAutoCollection(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestClassicFundCollectionByAsset(t *testing.T) {
	t.Parallel()
	_, err := e.ClassicFundCollectionByAsset(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ClassicFundCollectionByAsset(t.Context(), currency.LTC)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeAutoRepayFuturesStatusClassic(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeAutoRepayFuturesStatusClassic(t.Context(), false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAutoRepayFuturesStatusClassic(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAutoRepayFuturesStatusClassic(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRepayFuturesNegativeBalanceClassic(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RepayFuturesNegativeBalanceClassic(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPortfolioMarginAssetLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPortfolioMarginAssetLeverage(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserNegativeBalanceAutoExchangeRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetUserNegativeBalanceAutoExchangeRecord(t.Context(), endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserNegativeBalanceAutoExchangeRecord(t.Context(), startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBLVTInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBLVTInfo(t.Context(), "BTCDOWN")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubscribeBLVT(t *testing.T) {
	t.Parallel()
	_, err := e.SubscribeBLVT(t.Context(), "", 0.011)
	require.ErrorIs(t, err, errNameRequired)
	_, err = e.SubscribeBLVT(t.Context(), "BTCUP", 0)
	require.ErrorIs(t, err, errCostRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubscribeBLVT(t.Context(), "BTCUP", 0.011)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSusbcriptionRecords(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSusbcriptionRecords(t.Context(), &GetSusbcriptionRecordsRequest{TokenName: "BTCDOWN", StartTime: endTime, EndTime: startTime, ID: 10, Limit: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSusbcriptionRecords(t.Context(), &GetSusbcriptionRecordsRequest{TokenName: "BTCDOWN", StartTime: startTime, EndTime: endTime, ID: 10, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRedeemBLVT(t *testing.T) {
	t.Parallel()
	_, err := e.RedeemBLVT(t.Context(), currency.EMPTYPAIR, 2)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.RedeemBLVT(t.Context(), usdtmTradablePair, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.RedeemBLVT(t.Context(), usdtmTradablePair, 2)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetRedemptionRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetRedemptionRecord(t.Context(), &GetRedemptionRecordRequest{TokenName: "BTCDOWN", StartTime: endTime, EndTime: startTime, ID: 10, Limit: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetRedemptionRecord(t.Context(), &GetRedemptionRecordRequest{TokenName: "BTCDOWN", StartTime: startTime, EndTime: endTime, ID: 1, Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBLVTUserLimitInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBLVTUserLimitInfo(t.Context(), "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFiatDepositAndWithdrawalHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetFiatDepositAndWithdrawalHistory(t.Context(), &GetFiatDepositAndWithdrawalHistoryRequest{TransactionType: -5, Rows: 50})
	require.ErrorIs(t, err, errInvalidTransactionType)

	startTime, endTime := getTime()
	_, err = e.GetFiatDepositAndWithdrawalHistory(t.Context(), &GetFiatDepositAndWithdrawalHistoryRequest{BeginTime: endTime, EndTime: startTime, TransactionType: 1, Rows: 50})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFiatDepositAndWithdrawalHistory(t.Context(), &GetFiatDepositAndWithdrawalHistoryRequest{BeginTime: startTime, EndTime: endTime, TransactionType: 1, Page: 10, Rows: 50})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFiatPaymentHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetFiatPaymentHistory(t.Context(), &GetFiatPaymentHistoryRequest{TransactionType: -1, Rows: 50})
	require.ErrorIs(t, err, errInvalidTransactionType)

	startTime, endTime := getTime()
	_, err = e.GetFiatPaymentHistory(t.Context(), &GetFiatPaymentHistoryRequest{BeginTime: endTime, EndTime: startTime, TransactionType: 1, Rows: 50})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFiatPaymentHistory(t.Context(), &GetFiatPaymentHistoryRequest{BeginTime: startTime, EndTime: endTime, TransactionType: 1, Rows: 50})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetC2CTradeHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetC2CTradeHistory(t.Context(), &GetC2CTradeHistoryRequest{Rows: 50})
	require.ErrorIs(t, err, errTradeTypeRequired)

	startTime, endTime := getTime()
	_, err = e.GetC2CTradeHistory(t.Context(), &GetC2CTradeHistoryRequest{TradeType: order.Sell.String(), StartTime: endTime, EndTime: startTime, Page: 1, Rows: 50})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetC2CTradeHistory(t.Context(), &GetC2CTradeHistoryRequest{TradeType: order.Sell.String(), StartTime: startTime, EndTime: endTime, Page: 1, Rows: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPLoanOngoingOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPLoanOngoingOrders(t.Context(), &GetVIPLoanOngoingOrdersRequest{OrderID: 1232, CollateralAccountID: 21231, Limit: 10, LoanCoin: currency.BTC, CollateralCoin: currency.ETH})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPLoanRepay(t *testing.T) {
	t.Parallel()
	_, err := e.VIPLoanRepay(t.Context(), 0, 0.2)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.VIPLoanRepay(t.Context(), 1234, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.VIPLoanRepay(t.Context(), 1234, 0.2)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetPayTradeHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetPayTradeHistory(t.Context(), endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetPayTradeHistory(t.Context(), startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllConvertPairs(t *testing.T) {
	t.Parallel()
	_, err := e.GetAllConvertPairs(t.Context(), currency.EMPTYCODE, currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	result, err := e.GetAllConvertPairs(t.Context(), currency.BTC, currency.EMPTYCODE)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOrderQuantityPrecisionPerAsset(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOrderQuantityPrecisionPerAsset(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSendQuoteRequest(t *testing.T) {
	t.Parallel()
	_, err := e.SendQuoteRequest(t.Context(), &SendQuoteRequestRequest{ToAsset: currency.USDT, FromAmount: 10, ToAmount: 20, WalletType: "FUNDING", ValidTime: "1m"})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.SendQuoteRequest(t.Context(), &SendQuoteRequestRequest{FromAsset: currency.BTC, FromAmount: 10, ToAmount: 20, WalletType: "FUNDING", ValidTime: "1m"})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.SendQuoteRequest(t.Context(), &SendQuoteRequestRequest{FromAsset: currency.BTC, ToAsset: currency.USDT, WalletType: "FUNDING", ValidTime: "1m"})
	require.ErrorIs(t, err, order.ErrAmountIsInvalid)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SendQuoteRequest(t.Context(), &SendQuoteRequestRequest{FromAsset: currency.BTC, ToAsset: currency.USDT, FromAmount: 10, ToAmount: 20, WalletType: "FUNDING", ValidTime: "1m"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestAcceptQuote(t *testing.T) {
	t.Parallel()
	_, err := e.AcceptQuote(t.Context(), "")
	require.ErrorIs(t, err, errQuoteIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.AcceptQuote(t.Context(), "933256278426274426")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetConvertOrderStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetConvertOrderStatus(t.Context(), "933256278426274426", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPlaceLimitOrder(t *testing.T) {
	t.Parallel()
	_, err := e.PlaceLimitOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &ConvertPlaceLimitOrderRequest{
		ExpiredType: "7_D",
	}
	_, err = e.PlaceLimitOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.BaseAsset = currency.BTC
	arg.QuoteAsset = currency.ETH
	_, err = e.PlaceLimitOrder(t.Context(), arg)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	arg.LimitPrice = 0.0122
	_, err = e.PlaceLimitOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = order.Sell.String()
	arg.ExpiredType = ""
	_, err = e.PlaceLimitOrder(t.Context(), arg)
	require.ErrorIs(t, err, errExpiredTypeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.PlaceLimitOrder(t.Context(), &ConvertPlaceLimitOrderRequest{
		BaseAsset:   currency.BTC,
		QuoteAsset:  currency.ETH,
		LimitPrice:  0.0122,
		Side:        order.Sell.String(),
		ExpiredType: "7_D",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCancelLimitOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelLimitOrder(t.Context(), "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CancelLimitOrder(t.Context(), "123434")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLimitOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLimitOpenOrders(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetConvertTradeHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetConvertTradeHistory(t.Context(), endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetConvertTradeHistory(t.Context(), startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSpotRebateHistoryRecords(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSpotRebateHistoryRecords(t.Context(), endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotRebateHistoryRecords(t.Context(), startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetNFTTransactionHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetNFTTransactionHistory(t.Context(), &GetNFTTransactionHistoryRequest{OrderType: -1, Limit: 10, Page: 40})
	require.ErrorIs(t, err, order.ErrUnsupportedOrderType)

	startTime, endTime := getTime()
	_, err = e.GetNFTTransactionHistory(t.Context(), &GetNFTTransactionHistoryRequest{OrderType: 1, StartTime: endTime, EndTime: startTime, Limit: 10, Page: 40})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetNFTTransactionHistory(t.Context(), &GetNFTTransactionHistoryRequest{OrderType: 1, StartTime: startTime, EndTime: endTime, Limit: 10, Page: 40})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetNFTDepositHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetNFTDepositHistory(t.Context(), endTime, startTime, 10, 40)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetNFTDepositHistory(t.Context(), startTime, endTime, 10, 40)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetNFTWithdrawalHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetNFTWithdrawalHistory(t.Context(), endTime, startTime, 10, 40)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetNFTWithdrawalHistory(t.Context(), startTime, endTime, 10, 40)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetNFTAsset(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetNFTAsset(t.Context(), 10, 20)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateSingleTokenGiftCard(t *testing.T) {
	t.Parallel()
	_, err := e.CreateSingleTokenGiftCard(t.Context(), currency.EMPTYCODE, 0.1234)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.CreateSingleTokenGiftCard(t.Context(), currency.BUSD, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CreateSingleTokenGiftCard(t.Context(), currency.BUSD, 0.1234)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateDualTokenGiftCard(t *testing.T) {
	t.Parallel()
	_, err := e.CreateDualTokenGiftCard(t.Context(), currency.EMPTYCODE, currency.BNB, 10, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.CreateDualTokenGiftCard(t.Context(), currency.BUSD, currency.EMPTYCODE, 10, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.CreateDualTokenGiftCard(t.Context(), currency.BUSD, currency.BNB, 0, 10)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.CreateDualTokenGiftCard(t.Context(), currency.BUSD, currency.BNB, 10, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CreateDualTokenGiftCard(t.Context(), currency.BUSD, currency.BNB, 10, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRedeemBinanaceGiftCard(t *testing.T) {
	t.Parallel()
	_, err := e.RedeemBinanaceGiftCard(t.Context(), "", "12345")
	require.ErrorIs(t, err, errCodeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RedeemBinanaceGiftCard(t.Context(), "0033002328060227", "12345")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestVerifyBinanceGiftCardNumber(t *testing.T) {
	t.Parallel()
	_, err := e.VerifyBinanceGiftCardNumber(t.Context(), "")
	require.ErrorIs(t, err, errReferenceNumberRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.VerifyBinanceGiftCardNumber(t.Context(), "123456")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFetchRSAPublicKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FetchRSAPublicKey(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFetchTokenLimit(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTokenLimit(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.FetchTokenLimit(t.Context(), currency.BUSD)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPLoanRepaymentHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetVIPLoanRepaymentHistory(t.Context(), &GetVIPLoanRepaymentHistoryRequest{LoanCoin: currency.ETH, StartTime: endTime, EndTime: startTime, OrderID: 1234, Limit: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPLoanRepaymentHistory(t.Context(), &GetVIPLoanRepaymentHistoryRequest{LoanCoin: currency.ETH, StartTime: startTime, EndTime: endTime, OrderID: 1234, Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestVIPLoanRenew(t *testing.T) {
	t.Parallel()
	_, err := e.VIPLoanRenew(t.Context(), 0, 60)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.VIPLoanRenew(t.Context(), 1234, 60)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCheckLockedValueVIPCollateralAccount(t *testing.T) {
	t.Parallel()
	_, err := e.CheckLockedValueVIPCollateralAccount(t.Context(), 0, 40)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.CheckLockedValueVIPCollateralAccount(t.Context(), 1223, 0)
	require.ErrorIs(t, err, errAccountIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CheckLockedValueVIPCollateralAccount(t.Context(), 1223, 40)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestVIPLoanBorrow(t *testing.T) {
	t.Parallel()
	_, err := e.VIPLoanBorrow(t.Context(), &VIPLoanBorrowRequest{LoanTerm: 30, LoanCoin: currency.ETH, CollateralCoin: currency.LTC, LoanAmount: 123, CollateralAccountID: "1234"})
	require.ErrorIs(t, err, errAccountIDRequired)
	_, err = e.VIPLoanBorrow(t.Context(), &VIPLoanBorrowRequest{LoanAccountID: 1234, LoanTerm: 30, CollateralCoin: currency.LTC, LoanAmount: 123, CollateralAccountID: "1234"})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.VIPLoanBorrow(t.Context(), &VIPLoanBorrowRequest{LoanAccountID: 1234, LoanTerm: 30, LoanCoin: currency.ETH, CollateralCoin: currency.LTC, CollateralAccountID: "1234"})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.VIPLoanBorrow(t.Context(), &VIPLoanBorrowRequest{LoanAccountID: 1234, LoanTerm: 30, LoanCoin: currency.ETH, CollateralCoin: currency.LTC, LoanAmount: 1.2})
	require.ErrorIs(t, err, errAccountIDRequired)
	_, err = e.VIPLoanBorrow(t.Context(), &VIPLoanBorrowRequest{LoanAccountID: 1234, LoanTerm: 30, LoanCoin: currency.ETH, LoanAmount: 123, CollateralAccountID: "1234"})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.VIPLoanBorrow(t.Context(), &VIPLoanBorrowRequest{LoanAccountID: 1234, LoanCoin: currency.ETH, CollateralCoin: currency.LTC, LoanAmount: 123, CollateralAccountID: "1234"})
	require.ErrorIs(t, err, errLoanTermMustBeSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.VIPLoanBorrow(t.Context(), &VIPLoanBorrowRequest{LoanAccountID: 1234, LoanTerm: 30, LoanCoin: currency.ETH, CollateralCoin: currency.LTC, LoanAmount: 123, CollateralAccountID: "1234"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPLoanableAssetsData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPLoanableAssetsData(t.Context(), currency.BTC, 2)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPCollateralAssetData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPCollateralAssetData(t.Context(), currency.BTC)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPApplicationStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPApplicationStatus(t.Context(), 10, 20)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPBorrowInterestRate(t *testing.T) {
	t.Parallel()
	_, err := e.GetVIPBorrowInterestRate(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPBorrowInterestRate(t.Context(), currency.ETH)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPLoanAccruedInterest(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetVIPLoanAccruedInterest(t.Context(), &GetVIPLoanAccruedInterestRequest{OrderID: "12345", LoanCoin: currency.BTC, StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPLoanAccruedInterest(t.Context(), &GetVIPLoanAccruedInterestRequest{OrderID: "12345", LoanCoin: currency.BTC, StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPLoanInterestRateHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetVIPLoanInterestRateHistory(t.Context(), &GetVIPLoanInterestRateHistoryRequest{Limit: 10})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	startTime, endTime := getTime()
	_, err = e.GetVIPLoanInterestRateHistory(t.Context(), &GetVIPLoanInterestRateHistoryRequest{Coin: currency.BTC, StartTime: endTime, EndTime: startTime, Limit: 20})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPLoanInterestRateHistory(t.Context(), &GetVIPLoanInterestRateHistoryRequest{Coin: currency.BTC, StartTime: startTime, EndTime: endTime, Current: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateMarginListenKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CreateMarginListenKey(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestKeepMarginListenKeyAlive(t *testing.T) {
	t.Parallel()
	err := e.KeepMarginListenKeyAlive(t.Context(), "")
	require.ErrorIs(t, err, errListenKeyIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	err = e.KeepMarginListenKeyAlive(t.Context(), "T3ee22BIYuWqmvne0HNq2A2WsFlEtLhvWCtItw6ffhhdmjifQ2tRbuKkTHhr")
	assert.NoError(t, err)
}

func TestCloseMarginListenKey(t *testing.T) {
	t.Parallel()
	err := e.CloseMarginListenKey(t.Context(), "")
	require.ErrorIs(t, err, errListenKeyIsRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	err = e.CloseMarginListenKey(t.Context(), "T3ee22BIYuWqmvne0HNq2A2WsFlEtLhvWCtItw6ffhhdmjifQ2tRbuKkTHhr")
	assert.NoError(t, err)
}

func TestUnmarshalJSON(t *testing.T) {
	t.Parallel()
	data := []byte(`{"data":[{"1":"0.6"}, {"2":"0.6"}]}`)
	resp := &struct {
		Data WalletAssetCosts `json:"data"`
	}{}
	err := json.Unmarshal(data, resp)
	require.NoError(t, err)
	require.Equal(t, 0.6, resp.Data[0]["1"].Float64())
	assert.Equal(t, 0.6, resp.Data[1]["2"].Float64())
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		require.NoErrorf(t, err, "cannot get pairs for %s", a)
		require.NotEmptyf(t, pairs, "no pairs for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		require.NoError(t, err)
		require.NotEmpty(t, resp)
	}
}

func TestFetchOptionsExchangeLimits(t *testing.T) {
	t.Parallel()
	l, err := e.FetchOptionsExchangeLimits(t.Context())
	require.NoError(t, err)
	assert.NotEmpty(t, l, "Should get some limits back")
}

// ----------------- Copy Trading endpoints unit-tests ----------------

func TestGetFuturesLeadTraderStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesLeadTraderStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesLeadTradingSymbolWhitelist(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesLeadTradingSymbolWhitelist(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWithdrawalHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.WithdrawalHistory(t.Context(), &LocalEntityWithdrawalHistoryRequest{TravelRuleRecordIDs: []string{"1234"}, TransactionIDs: []string{"0xb5ef8c13b968a406cc62a93a8bd80f9e9a906ef1b3fcf20a2e48573c17659268"}, TravelRuleStatus: "0", Limit: 100, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.WithdrawalHistory(t.Context(), &LocalEntityWithdrawalHistoryRequest{TravelRuleRecordIDs: []string{"1234"}, TransactionIDs: []string{"0xb5ef8c13b968a406cc62a93a8bd80f9e9a906ef1b3fcf20a2e48573c17659268"}, TravelRuleStatus: "0", Limit: 100, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWithdrawalHistoryV2(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.WithdrawalHistory(t.Context(), &LocalEntityWithdrawalHistoryRequest{TravelRuleRecordIDs: []string{"1234"}, TransactionIDs: []string{"0xb5ef8c13b968a406cc62a93a8bd80f9e9a906ef1b3fcf20a2e48573c17659268"}, TravelRuleStatus: "0", Limit: 100, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.WithdrawalHistory(t.Context(), &LocalEntityWithdrawalHistoryRequest{TravelRuleRecordIDs: []string{"1234"}, TransactionIDs: []string{"0xb5ef8c13b968a406cc62a93a8bd80f9e9a906ef1b3fcf20a2e48573c17659268"}, TravelRuleStatus: "0", Limit: 100, StartTime: startTime, EndTime: endTime})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubmitDepositQuestionnaire(t *testing.T) {
	t.Parallel()
	_, err := e.SubmitDepositQuestionnaire(t.Context(), "", nil)
	require.ErrorIs(t, err, errTransactionIDRequired)

	_, err = e.SubmitDepositQuestionnaire(t.Context(), "765127651", nil)
	require.ErrorIs(t, err, errQuestionnaireRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubmitDepositQuestionnaire(t.Context(), "765127651", map[string]any{
		"isAddressOwner": 2,
		"sendTo":         1,
		"vaspCountry":    "cn",
		"vaspRegion":     "notNortheasternProvinces",
		"txnPurpose":     "3",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetLocalEntitiesDepositHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetLocalEntitiesDepositHistory(t.Context(), &LocalEntityDepositHistoryRequest{Network: "BNB", Coin: currency.USDT, TravelRuleStatus: "1", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetLocalEntitiesDepositHistory(t.Context(), &LocalEntityDepositHistoryRequest{Network: "BNB", Coin: currency.USDT, TravelRuleStatus: "1", StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOnboardedVASPList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOnboardedVASPList(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CreateSubAccount(t.Context(), "tag-here")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccounts(t.Context(), "1", 0, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableFuturesForSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.EnableFuturesForSubAccount(t.Context(), "", false)
	require.ErrorIs(t, err, errSubAccountIDMissing)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.EnableFuturesForSubAccount(t.Context(), "1", false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateAPIKeyForSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.CreateAPIKeyForSubAccount(t.Context(), "", false, true, true)
	require.ErrorIs(t, err, errSubAccountIDMissing)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CreateAPIKeyForSubAccount(t.Context(), "1", false, true, true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeSubAccountAPIPermission(t *testing.T) {
	t.Parallel()
	_, err := e.ChangeSubAccountAPIPermission(t.Context(), &ChangeSubAccountAPIPermissionRequest{SubAccountAPIKey: "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A", MarginTrade: true, FuturesTrade: true})
	require.ErrorIs(t, err, errSubAccountIDMissing)
	_, err = e.ChangeSubAccountAPIPermission(t.Context(), &ChangeSubAccountAPIPermissionRequest{SubAccountID: "1", MarginTrade: true, FuturesTrade: true})
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeSubAccountAPIPermission(t.Context(), &ChangeSubAccountAPIPermissionRequest{MarginTrade: true, FuturesTrade: true})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableUniversalTransferPermissionForSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.EnableUniversalTransferPermissionForSubAccountAPIKey(t.Context(), "", "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A", false)
	require.ErrorIs(t, err, errSubAccountIDMissing)
	_, err = e.EnableUniversalTransferPermissionForSubAccountAPIKey(t.Context(), "1", "", false)
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.EnableUniversalTransferPermissionForSubAccountAPIKey(t.Context(), "1", "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A", false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUpdateIPRestrictionForSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateIPRestrictionForSubAccountAPIKey(t.Context(), "", "", "2", "")
	require.ErrorIs(t, err, errSubAccountIDMissing)
	_, err = e.UpdateIPRestrictionForSubAccountAPIKey(t.Context(), "123", "", "2", "")
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)
	_, err = e.UpdateIPRestrictionForSubAccountAPIKey(t.Context(), "123", "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A", "", "")
	require.ErrorIs(t, err, errSubAccountStatusMissing)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UpdateIPRestrictionForSubAccountAPIKey(t.Context(), "123", "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A", "2", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDeleteIPRestrictionForSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.DeleteIPRestrictionForSubAccountAPIKey(t.Context(), "", "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A", "")
	require.ErrorIs(t, err, errSubAccountIDMissing)
	_, err = e.DeleteIPRestrictionForSubAccountAPIKey(t.Context(), "123", "", "")
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.DeleteIPRestrictionForSubAccountAPIKey(t.Context(), "123", "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDeleteBrokerSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.DeleteBrokerSubAccountAPIKey(t.Context(), "", "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A")
	require.ErrorIs(t, err, errSubAccountIDMissing)
	_, err = e.DeleteBrokerSubAccountAPIKey(t.Context(), "123", "")
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.DeleteBrokerSubAccountAPIKey(t.Context(), "123", "vmPUZE6mv9SD5VNHk4HlWFsOr6aKE2zvsw0MuIgwCIPy6utIco14y7Ju91duEh8A")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeSubAccountCommission(t *testing.T) {
	t.Parallel()
	_, err := e.ChangeSubAccountCommission(t.Context(), &ChangeSubAccountCommissionRequest{MakerCommission: 1., TakerCommission: 2.})
	require.ErrorIs(t, err, errSubAccountIDMissing)
	_, err = e.ChangeSubAccountCommission(t.Context(), &ChangeSubAccountCommissionRequest{SubAccountID: "2", TakerCommission: 2.})
	require.ErrorIs(t, err, errCommissionValueRequired)
	_, err = e.ChangeSubAccountCommission(t.Context(), &ChangeSubAccountCommissionRequest{SubAccountID: "2", MakerCommission: 1.})
	require.ErrorIs(t, err, errCommissionValueRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeSubAccountCommission(t.Context(), &ChangeSubAccountCommissionRequest{SubAccountID: "2", MakerCommission: 1., TakerCommission: 2.})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBNBBurnStatusForSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.GetBNBBurnStatusForSubAccount(t.Context(), "")
	require.ErrorIs(t, err, errSubAccountIDMissing)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBNBBurnStatusForSubAccount(t.Context(), "1")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubAccountTransferWithSpotBroker(t *testing.T) {
	t.Parallel()
	_, err := e.SubAccountTransferWithSpotBroker(t.Context(), &SubAccountTransferWithSpotBrokerRequest{Amount: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.SubAccountTransferWithSpotBroker(t.Context(), &SubAccountTransferWithSpotBrokerRequest{Currency: currency.BTC})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubAccountTransferWithSpotBroker(t.Context(), &SubAccountTransferWithSpotBrokerRequest{Currency: currency.BTC, Amount: 13})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSpotBrokerSubAccountTransferHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSpotBrokerSubAccountTransferHistory(t.Context(), &BrokerSubAccountTransferHistoryRequest{ShowAllStatus: true, StartTime: endTime, EndTime: startTime, Page: 1, Limit: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotBrokerSubAccountTransferHistory(t.Context(), &BrokerSubAccountTransferHistoryRequest{ShowAllStatus: true, StartTime: startTime, EndTime: endTime, Page: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubAccountTransferWithFuturesBroker(t *testing.T) {
	t.Parallel()
	_, err := e.SubAccountTransferWithFuturesBroker(t.Context(), &SubAccountTransferWithFuturesBrokerRequest{FuturesType: 1, Amount: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.SubAccountTransferWithFuturesBroker(t.Context(), &SubAccountTransferWithFuturesBrokerRequest{Currency: currency.BTC, FuturesType: 2})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SubAccountTransferWithFuturesBroker(t.Context(), &SubAccountTransferWithFuturesBrokerRequest{Currency: currency.BTC, FuturesType: 1, Amount: 1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesBrokerSubAccountTransferHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFuturesBrokerSubAccountTransferHistory(t.Context(), &GetFuturesBrokerSubAccountTransferHistoryRequest{StartTime: endTime, EndTime: startTime, Limit: 100})
	require.ErrorIs(t, err, errSubAccountIDMissing)

	_, err = e.GetFuturesBrokerSubAccountTransferHistory(t.Context(), &GetFuturesBrokerSubAccountTransferHistoryRequest{SubAccountID: "abcdef", StartTime: endTime, EndTime: startTime, Limit: 100})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesBrokerSubAccountTransferHistory(t.Context(), &GetFuturesBrokerSubAccountTransferHistoryRequest{SubAccountID: "abcdef", StartTime: startTime, EndTime: endTime, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountDepositHistoryWithBroker(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSubAccountDepositHistoryWithBroker(t.Context(), &GetSubAccountDepositHistoryWithBrokerRequest{Coin: currency.BTC, StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountDepositHistoryWithBroker(t.Context(), &GetSubAccountDepositHistoryWithBrokerRequest{Coin: currency.BTC, StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountSpotAssetInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountSpotAssetInfo(t.Context(), "1234", 0, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountMarginAssetInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountMarginAssetInfo(t.Context(), "", 0, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountFuturesAssetInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountFuturesAssetInfo(t.Context(), "1234", true, 0, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUniversalTransferWithBroker(t *testing.T) {
	t.Parallel()
	_, err := e.UniversalTransferWithBroker(t.Context(), &UniversalTransferWithBrokerRequest{ToAccountType: "USDT_FUTURE", Currency: currency.BTC, Amount: 1})
	require.ErrorIs(t, err, errInvalidAccountType)
	_, err = e.UniversalTransferWithBroker(t.Context(), &UniversalTransferWithBrokerRequest{FromAccountType: "SPOT", Currency: currency.BTC, Amount: 1})
	require.ErrorIs(t, err, errInvalidAccountType)
	_, err = e.UniversalTransferWithBroker(t.Context(), &UniversalTransferWithBrokerRequest{FromAccountType: "SPOT", ToAccountType: "USDT_FUTURE", Amount: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)
	_, err = e.UniversalTransferWithBroker(t.Context(), &UniversalTransferWithBrokerRequest{FromAccountType: "SPOT", ToAccountType: "USDT_FUTURE", Currency: currency.BTC})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UniversalTransferWithBroker(t.Context(), &UniversalTransferWithBrokerRequest{FromAccountType: "SPOT", ToAccountType: "USDT_FUTURE", Currency: currency.BTC, Amount: 1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUniversalTransferHistoryThroughBroker(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetUniversalTransferHistoryThroughBroker(t.Context(), &GetUniversalTransferHistoryThroughBrokerRequest{StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUniversalTransferHistoryThroughBroker(t.Context(), &GetUniversalTransferHistoryThroughBrokerRequest{StartTime: startTime, EndTime: endTime, Page: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateBrokerSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CreateBrokerSubAccount(t.Context(), "1234")
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.CreateBrokerSubAccount(t.Context(), "Thrasher")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBrokerSubAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetBrokerSubAccounts(t.Context(), "123", 0, 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableOrDisableBNBBurnForSubAccountMarginInterest(t *testing.T) {
	t.Parallel()
	_, err := e.EnableOrDisableBNBBurnForSubAccountMarginInterest(t.Context(), "", false)
	require.ErrorIs(t, err, errSubAccountIDMissing)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.EnableOrDisableBNBBurnForSubAccountMarginInterest(t.Context(), "3", false)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEnableOrDisableBNBBurnForSubAccountSpotAndMargin(t *testing.T) {
	t.Parallel()
	_, err := e.EnableOrDisableBNBBurnForSubAccountSpotAndMargin(t.Context(), "", true)
	require.ErrorIs(t, err, errSubAccountIDMissing)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.EnableOrDisableBNBBurnForSubAccountSpotAndMargin(t.Context(), "1", true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestLinkAccountInformation(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.LinkAccountInformation(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeSubAccountUSDTMarginedFuturesCommissionAdjustment(t *testing.T) {
	t.Parallel()
	_, err := e.ChangeSubAccountUSDTMarginedFuturesCommissionAdjustment(t.Context(), "", spotTradablePair, 1, 10)
	require.ErrorIs(t, err, errSubAccountIDMissing)
	_, err = e.ChangeSubAccountUSDTMarginedFuturesCommissionAdjustment(t.Context(), "234", currency.EMPTYPAIR, 1, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.ChangeSubAccountUSDTMarginedFuturesCommissionAdjustment(t.Context(), "234", spotTradablePair, 0, 10)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.ChangeSubAccountUSDTMarginedFuturesCommissionAdjustment(t.Context(), "234", spotTradablePair, 1, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeSubAccountUSDTMarginedFuturesCommissionAdjustment(t.Context(), "234", spotTradablePair, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountUSDMarginedFuturesCommissionAdjustment(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountUSDMarginedFuturesCommissionAdjustment(t.Context(), "", usdtmTradablePair)
	require.ErrorIs(t, err, errSubAccountIDMissing)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountUSDMarginedFuturesCommissionAdjustment(t.Context(), "123", usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestChangeSubAccountCoinMarginedFuturesCommissionAdjustment(t *testing.T) {
	t.Parallel()
	_, err := e.ChangeSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "", coinmTradablePair, 1., 2.)
	require.ErrorIs(t, err, errSubAccountIDMissing)
	_, err = e.ChangeSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "231", currency.EMPTYPAIR, 1., 2.)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.ChangeSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "231", coinmTradablePair, 0, 2.)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.ChangeSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "231", coinmTradablePair, 1., 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.ChangeSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "231", coinmTradablePair, 1., 2.)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSubAccountCoinMarginedFuturesCommissionAdjustment(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "", coinmTradablePair)
	require.ErrorIs(t, err, errSubAccountIDMissing)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "123", coinmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetBrokerCommissionRebateRecentRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSpotBrokerCommissionRebateRecentRecord(t.Context(), &GetSpotBrokerCommissionRebateRecentRecordRequest{SubAccountID: "1234", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotBrokerCommissionRebateRecentRecord(t.Context(), &GetSpotBrokerCommissionRebateRecentRecordRequest{SubAccountID: "1234", StartTime: startTime, EndTime: endTime, Page: 1, Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesBrokerCommissionRebateRecentRecord(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFuturesBrokerCommissionRebateRecentRecord(t.Context(), &GetFuturesBrokerCommissionRebateRecentRecordRequest{StartTime: endTime, EndTime: startTime, Size: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesBrokerCommissionRebateRecentRecord(t.Context(), &GetFuturesBrokerCommissionRebateRecentRecordRequest{StartTime: startTime, EndTime: endTime, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// ---------- Binance Link endpoints ----------------------------------

func TestGetInfoAboutIfUserIsNew(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotInfoAboutIfUserIsNew(t.Context(), "")
	require.ErrorIs(t, err, errCodeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotInfoAboutIfUserIsNew(t.Context(), "1234")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCustomiseIDForClient(t *testing.T) {
	t.Parallel()
	_, err := e.CustomiseSpotPartnerClientID(t.Context(), "", "someone@thrasher.io")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.CustomiseSpotPartnerClientID(t.Context(), "1233", "")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CustomiseSpotPartnerClientID(t.Context(), "1233", "someone@thrasher.io")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetClientEmailCustomisedID(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotClientEmailCustomisedID(t.Context(), "", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesClientEmailCustomisedID(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesClientEmailCustomisedID(t.Context(), "", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCustomiseOwnClientID(t *testing.T) {
	t.Parallel()
	_, err := e.CustomiseSpotOwnClientID(t.Context(), "", "ABCDEFG")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.CustomiseSpotOwnClientID(t.Context(), "the-unique-id", "")
	require.ErrorIs(t, err, errCodeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CustomiseSpotOwnClientID(t.Context(), "the-unique-id", "ABCDEFG")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCustomiseFuturesOwnClientID(t *testing.T) {
	t.Parallel()
	_, err := e.CustomiseFuturesOwnClientID(t.Context(), "", "ABCDEFG")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.CustomiseFuturesOwnClientID(t.Context(), "the-unique-id", "")
	require.ErrorIs(t, err, errCodeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CustomiseFuturesOwnClientID(t.Context(), "the-unique-id", "ABCDEFG")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUsersCustomisedID(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotUsersCustomisedID(t.Context(), "")
	require.ErrorIs(t, err, errCodeRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotUsersCustomisedID(t.Context(), "1234ABCD")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesUsersCustomisedID(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesUsersCustomisedID(t.Context(), "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesUsersCustomisedID(t.Context(), "1234ABCD")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOthersRebateRecentRecord(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotOthersRebateRecentRecord(t.Context(), "", time.Time{}, time.Time{}, 10)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	startTime, endTime := getTime()
	_, err = e.GetSpotOthersRebateRecentRecord(t.Context(), "123123", endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotOthersRebateRecentRecord(t.Context(), "123123", startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOwnRebateRecentRecords(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSpotOwnRebateRecentRecords(t.Context(), endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSpotOwnRebateRecentRecords(t.Context(), startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesClientIfNewUser(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesClientIfNewUser(t.Context(), "", 1)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesClientIfNewUser(t.Context(), "1234", 1)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCustomiseFuturesPartnerClientID(t *testing.T) {
	t.Parallel()
	_, err := e.CustomiseFuturesPartnerClientID(t.Context(), "", "someone@thrasher.io")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.CustomiseFuturesPartnerClientID(t.Context(), "1233", "")
	require.ErrorIs(t, err, errValidEmailRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CustomiseFuturesPartnerClientID(t.Context(), "1233", "someone@thrasher.io")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesUserIncomeHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFuturesUserIncomeHistory(t.Context(), &GetFuturesUserIncomeHistoryRequest{Symbol: usdtmTradablePair, IncomeType: "COMMISSION", StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesUserIncomeHistory(t.Context(), &GetFuturesUserIncomeHistoryRequest{Symbol: usdtmTradablePair, IncomeType: "COMMISSION", StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesReferredTradersNumber(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFuturesReferredTradersNumber(t.Context(), false, endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesReferredTradersNumber(t.Context(), true, startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesRebateDataOverview(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesRebateDataOverview(t.Context(), true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserTradeVolume(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetUserTradeVolume(t.Context(), false, endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUserTradeVolume(t.Context(), true, startTime, endTime, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetRebateVolume(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetRebateVolume(t.Context(), false, endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetRebateVolume(t.Context(), false, startTime, endTime, 100)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetTraderDetail(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetTraderDetail(t.Context(), &GetTraderDetailRequest{CustomerID: "sde001", CoinMargined: true, StartTime: endTime, EndTime: startTime, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetTraderDetail(t.Context(), &GetTraderDetailRequest{CustomerID: "sde001", CoinMargined: true, StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFuturesClientifNewUser(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesClientifNewUser(t.Context(), "", false)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFuturesClientifNewUser(t.Context(), "123123", false)
	require.NoError(t, err)
	assert.NotNil(t, result)

	result, err = e.GetFuturesClientifNewUser(t.Context(), "123123", true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCustomiseIDForClientToReferredUser(t *testing.T) {
	t.Parallel()
	_, err := e.CustomiseIDForClientToReferredUser(t.Context(), "", "1234")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
	_, err = e.CustomiseIDForClientToReferredUser(t.Context(), "12345678", "")
	require.ErrorIs(t, err, errInvalidBrokerID)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.CustomiseIDForClientToReferredUser(t.Context(), "12345678", "987654321")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUsersCustomiseIDs(t *testing.T) {
	t.Parallel()
	_, err := e.GetUsersCustomiseIDs(t.Context(), "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUsersCustomiseIDs(t.Context(), "12345678")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUserStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFastAPIUserStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.CreateAPIKey(t.Context(), &BrokerAPIKeyRequest{PublicKey: "12312", Status: "1", EnableTrade: true, EnableFutureTrade: true, EnableEuropeanOptions: true})
	require.ErrorIs(t, err, errAPIKeyNameRequired)
	_, err = e.CreateAPIKey(t.Context(), &BrokerAPIKeyRequest{APIName: "Thrasher", Status: "1", EnableTrade: true, EnableFutureTrade: true, EnableEuropeanOptions: true})
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateAPICredentials)
	result, err := e.CreateAPIKey(t.Context(), &BrokerAPIKeyRequest{APIName: "Thrasher", PublicKey: "sub-acc-API-key-here", Status: "1", EnableTrade: true, EnableFutureTrade: true, EnableEuropeanOptions: true})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestOrderTypeFromString(t *testing.T) {
	t.Parallel()
	orderTypeFromStringList := []struct {
		String    string
		OrderType order.Type
		Error     error
	}{
		{"STOP_MARKET", order.StopMarket, nil},
		{"TAKE_PROFIT", order.TakeProfit, nil},
		{"TAKE_PROFIT_MARKET", order.TakeProfitMarket, nil},
		{"TRAILING_STOP_MARKET", order.TrailingStop, nil},
		{"STOP_LOSS_LIMIT", order.StopLimit, nil},
		{"TAKE_PROFIT_LIMIT", order.TakeProfitLimit, nil},
		{"LIMIT_MAKER", order.LimitMaker, nil},
		{"LIMIT", order.Limit, nil},
		{"MARKET", order.Market, nil},
		{"STOP", order.Stop, nil},
		{"OCO", order.OCO, nil},
		{"OTO", order.OTO, nil},
		{"STOP_LOSS", order.Stop, nil},
		{"abcd", order.UnknownType, order.ErrUnsupportedOrderType},
	}
	for _, val := range orderTypeFromStringList {
		result, err := stringToOrderType(val.String)
		require.ErrorIs(t, err, val.Error)
		assert.Equal(t, result, val.OrderType)
	}
}

func TestOrderTypeString(t *testing.T) {
	t.Parallel()
	orderTypeStringToTypeList := []struct {
		OrderType order.Type
		String    string
		Error     error
	}{
		{order.Limit, "LIMIT", nil},
		{order.StopMarket, "STOP_MARKET", nil},
		{order.TakeProfit, "TAKE_PROFIT", nil},
		{order.TakeProfitMarket, "TAKE_PROFIT_MARKET", nil},
		{order.TrailingStop, "TRAILING_STOP_MARKET", nil},
		{order.StopLimit, "STOP_LOSS_LIMIT", nil},
		{order.TakeProfitLimit, "TAKE_PROFIT_LIMIT", nil},
		{order.LimitMaker, "LIMIT_MAKER", nil},
		{order.Market, "MARKET", nil},
		{order.OCO, "OCO", nil},
		{order.OTO, "OTO", nil},
		{order.Stop, "STOP_LOSS", nil},
		{order.IOS, "", order.ErrUnsupportedOrderType},
	}
	for _, value := range orderTypeStringToTypeList {
		result, err := orderTypeString(value.OrderType, asset.Spot)
		require.ErrorIs(t, err, value.Error)
		assert.Equal(t, result, value.String)
	}

	// Futures names the conditional types differently to spot
	for _, tc := range []struct {
		OrderType order.Type
		Expected  string
	}{
		{order.Stop, "STOP"},
		{order.StopLimit, "STOP"},
		{order.TakeProfitLimit, "TAKE_PROFIT"},
		{order.StopMarket, "STOP_MARKET"},
		{order.TakeProfitMarket, "TAKE_PROFIT_MARKET"},
		{order.Limit, "LIMIT"},
	} {
		for _, a := range []asset.Item{asset.USDTMarginedFutures, asset.CoinMarginedFutures} {
			result, err := orderTypeString(tc.OrderType, a)
			require.NoErrorf(t, err, "orderTypeString must not error for %v on %s", tc.OrderType, a)
			assert.Equalf(t, tc.Expected, result, "%v should map to %s on %s", tc.OrderType, tc.Expected, a)
		}
	}
}

func TestTimeInForceString(t *testing.T) {
	t.Parallel()
	timeInForceStringList := []struct {
		TIF    order.TimeInForce
		OType  order.Type
		String string
	}{
		{order.FillOrKill, 0, "FOK"},
		{order.ImmediateOrCancel, 0, "IOC"},
		{order.GoodTillCancel, 0, "GTC"},
		{order.GoodTillDay, 0, "GTD"},
		{order.GoodTillCrossing, 0, "GTX"},
		{order.UnknownTIF, order.Limit, "GTC"},
		{order.UnknownTIF, order.Market, "IOC"},
		{order.UnknownTIF, order.UnknownType, ""},
	}
	for _, val := range timeInForceStringList {
		result := timeInForceString(val.TIF, val.OType)
		assert.Equal(t, val.String, result)
	}
}

func TestSendHTTPRequestErrorResponse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
		wantMsg    string
	}{
		{
			name:       "mapped error code",
			statusCode: http.StatusBadRequest,
			body:       `{"code":-1121,"msg":"Invalid symbol."}`,
			wantErr:    currency.ErrCurrencyPairEmpty,
			wantMsg:    "msg: Invalid symbol.",
		},
		{
			name:       "unmapped error code",
			statusCode: http.StatusBadRequest,
			body:       `{"code":-9999,"msg":"Unknown error."}`,
			wantMsg:    "err code: -9999 msg: Unknown error.",
		},
		{
			name:       "non error response body",
			statusCode: http.StatusInternalServerError,
			body:       `{"someField":"value"}`,
			wantErr:    request.ErrBadStatus,
		},
		{
			name:       "non json body",
			statusCode: http.StatusBadGateway,
			body:       `not json`,
			wantErr:    request.ErrBadStatus,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := new(Exchange)
			require.NoError(t, testexch.Setup(e), "Setup must not error")

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, err := w.Write([]byte(tc.body))
				assert.NoError(t, err, "Writing response should not error")
			}))
			defer server.Close()

			require.NoError(t, e.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL must not error")

			var result any
			err := e.SendHTTPRequest(t.Context(), exchange.RestSpot, "/foo", request.UnauthenticatedRequest, &result)
			require.Error(t, err, "SendHTTPRequest must return an error")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr, "SendHTTPRequest must return the expected error")
			}
			if tc.wantMsg != "" {
				assert.Contains(t, err.Error(), tc.wantMsg, "error should contain the response message")
			}
		})
	}
}

func TestInterfaceToParams(t *testing.T) {
	t.Parallel()
	arg := &struct {
		Timestamp int64   `json:"timestamp"`
		OrderID   uint64  `json:"orderId"`
		Price     float64 `json:"price"`
		Symbol    string  `json:"symbol"`
	}{
		Timestamp: 1735689600000,
		OrderID:   12345678901234567,
		Price:     0.00000123,
		Symbol:    "BTCUSDT",
	}

	params, err := interfaceToParams(arg)
	require.NoError(t, err, "interfaceToParams must not error")

	// Numbers must survive verbatim; decoding into a bare any yields float64 and
	// formats back out as scientific notation, corrupting timestamps and IDs.
	assert.Equal(t, "1735689600000", params.Get("timestamp"), "timestamp should not be converted to scientific notation")
	assert.Equal(t, "12345678901234567", params.Get("orderId"), "large order ID should retain full precision")
	assert.Equal(t, "0.00000123", params.Get("price"), "small price should retain its literal representation")
	assert.Equal(t, "BTCUSDT", params.Get("symbol"), "string values should be passed through unchanged")
}

func TestCreateUFuturesListenKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CreateUFuturesListenKey(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestKeepUFuturesListenKeyAlive(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.KeepUFuturesListenKeyAlive(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCloseUFuturesListenKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	assert.NoError(t, e.CloseUFuturesListenKey(t.Context()), "CloseUFuturesListenKey should not error")
}

func TestCreateCFuturesListenKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CreateCFuturesListenKey(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestKeepCFuturesListenKeyAlive(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.KeepCFuturesListenKeyAlive(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCloseCFuturesListenKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	assert.NoError(t, e.CloseCFuturesListenKey(t.Context()), "CloseCFuturesListenKey should not error")
}

func TestCreatePortfolioMarginListenKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CreatePortfolioMarginListenKey(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestKeepPortfolioMarginListenKeyAlive(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.KeepPortfolioMarginListenKeyAlive(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestClosePortfolioMarginListenKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	assert.NoError(t, e.ClosePortfolioMarginListenKey(t.Context()), "ClosePortfolioMarginListenKey should not error")
}

func TestCreateMarginListenToken(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.CreateMarginListenToken(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestListenKeyStorePerAsset(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup must not error")

	// Binance issues a distinct listen key per product line; storing one must not disturb another.
	e.setListenKey(asset.Spot, "spot-key")
	e.setListenKey(asset.USDTMarginedFutures, "usdm-key")
	e.setListenKey(asset.Options, "options-key")

	assert.Equal(t, "spot-key", e.getListenKey(asset.Spot), "spot listen key should be retained")
	assert.Equal(t, "usdm-key", e.getListenKey(asset.USDTMarginedFutures), "USD-M listen key should be retained")
	assert.Equal(t, "options-key", e.getListenKey(asset.Options), "options listen key should be retained")
	assert.Empty(t, e.getListenKey(asset.CoinMarginedFutures), "an unset product line should return an empty key")
}

func TestWsHandleFuturesUserData(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup must not error")
	if mockTests {
		require.NoError(t, testexch.MockHTTPInstance(e), "MockHTTPInstance must not error")
	}
	testexch.UpdatePairsOnce(t, e)
	e.Websocket.DataHandler = stream.NewRelay(sharedtestvalues.WebsocketRelayBufferCapacity)

	// A USD-M user data event arrives on /ws/<listenKey> as a bare object with no "stream"
	// wrapper, so routing on the event type is what makes it reachable at all.
	const orderTradeUpdate = `{"e":"ORDER_TRADE_UPDATE","E":1568879465651,"T":1568879465650,"o":{` +
		`"s":"BTCUSDT","c":"TEST","S":"SELL","o":"LIMIT","f":"GTC","q":"0.001","p":"7100.0",` +
		`"ap":"7100.0","sp":"0","x":"TRADE","X":"FILLED","i":8886774,"l":"0.001","z":"0.001",` +
		`"L":"7100.0","N":"USDT","n":"0.01","T":1568879465650,"t":123,"m":false,"R":false,` +
		`"wt":"CONTRACT_PRICE","ot":"LIMIT","ps":"BOTH","cp":false,"rp":"0"}}`

	require.NoError(t, e.wsHandleFuturesUserData(t.Context(), "ORDER_TRADE_UPDATE", []byte(orderTradeUpdate)),
		"ORDER_TRADE_UPDATE must be handled")

	const listenKeyExpired = `{"e":"listenKeyExpired","E":1576653824250}`
	require.NoError(t, e.wsHandleFuturesUserData(t.Context(), "listenKeyExpired", []byte(listenKeyExpired)),
		"listenKeyExpired must be handled")

	const accountConfigUpdate = `{"e":"ACCOUNT_CONFIG_UPDATE","E":1611646737479,"T":1611646737476,"ac":{"s":"BTCUSDT","l":25}}`
	require.NoError(t, e.wsHandleFuturesUserData(t.Context(), "ACCOUNT_CONFIG_UPDATE", []byte(accountConfigUpdate)),
		"ACCOUNT_CONFIG_UPDATE must be handled")

	// MARGIN_CALL is forwarded untyped rather than erroring as unhandled
	const marginCall = `{"e":"MARGIN_CALL","E":1587727187525,"p":[]}`
	assert.NoError(t, e.wsHandleFuturesUserData(t.Context(), "MARGIN_CALL", []byte(marginCall)),
		"MARGIN_CALL should be forwarded rather than reported unhandled")

	err := e.wsHandleFuturesUserData(t.Context(), "SOMETHING_NEW", []byte(`{"e":"SOMETHING_NEW"}`))
	assert.ErrorContains(t, err, websocket.UnhandledMessage, "an unrecognised event should be reported as unhandled")
}

func TestProcessFuturesOrderTradeUpdate(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup must not error")
	if mockTests {
		require.NoError(t, testexch.MockHTTPInstance(e), "MockHTTPInstance must not error")
	}
	testexch.UpdatePairsOnce(t, e)
	relay := stream.NewRelay(sharedtestvalues.WebsocketRelayBufferCapacity)
	e.Websocket.DataHandler = relay

	const payload = `{"e":"ORDER_TRADE_UPDATE","E":1568879465651,"T":1568879465650,"o":{` +
		`"s":"BTCUSDT","c":"TEST","S":"BUY","o":"LIMIT","f":"GTC","q":"2","p":"7100.0",` +
		`"ap":"7050.0","sp":"0","x":"TRADE","X":"PARTIALLY_FILLED","i":8886774,"l":"0.5","z":"0.5",` +
		`"L":"7050.0","N":"USDT","n":"0.01","T":1568879465650,"t":123,"m":false,"R":false,` +
		`"wt":"CONTRACT_PRICE","ot":"LIMIT","ps":"BOTH","cp":false,"rp":"0"}}`

	require.NoError(t, e.processFuturesOrderTradeUpdate(t.Context(), []byte(payload), asset.USDTMarginedFutures),
		"processFuturesOrderTradeUpdate must not error")

	select {
	case v := <-relay.C:
		d, ok := v.Data.(*order.Detail)
		require.Truef(t, ok, "must relay an order detail, got %T", v)
		assert.Equal(t, "8886774", d.OrderID, "order ID should be carried through")
		assert.Equal(t, order.Buy, d.Side, "side should be parsed")
		assert.Equal(t, order.PartiallyFilled, d.Status, "status should be parsed")
		assert.Equal(t, 2.0, d.Amount, "amount should be the original quantity")
		assert.Equal(t, 0.5, d.ExecutedAmount, "executed amount should be the accumulated fill")
		assert.Equal(t, 1.5, d.RemainingAmount, "remaining amount should be original minus filled")
		assert.Equal(t, currency.USDT, d.FeeAsset, "commission asset should be parsed")
	default:
		t.Fatal("expected an order detail to be relayed")
	}
}

func TestSentinelErrorPaths(t *testing.T) {
	t.Parallel()
	_, err := e.GetAggregatedTrades(t.Context(), &AggregatedTradeRequest{Symbol: spotTradablePair, EndTime: time.Now()})
	assert.ErrorIs(t, err, errStartOrFromIDRequired, "GetAggregatedTrades should require a start time or from ID")

	_, err = e.SetOptionsMarketMakerProtectionConfig(t.Context(), &MarketMakerProtectionConfig{Underlying: "BTCUSDT"})
	assert.ErrorIs(t, err, errWindowTimeRequired, "the market maker protection config should require a window time")

	// A zero frozen time means the protection stays frozen until manually reset, so it
	// must be accepted; only a negative value is rejected
	_, err = e.SetOptionsMarketMakerProtectionConfig(t.Context(), &MarketMakerProtectionConfig{Underlying: "BTCUSDT", WindowTimeInMilliseconds: 5000, FrozenTimeInMilliseconds: -1})
	assert.ErrorIs(t, err, errFrozenTimeRequired, "a negative frozen time should be rejected")

	_, err = e.SetOptionsMarketMakerProtectionConfig(t.Context(), &MarketMakerProtectionConfig{Underlying: "BTCUSDT", WindowTimeInMilliseconds: 5000, FrozenTimeInMilliseconds: 0})
	assert.ErrorIs(t, err, errQuantityLimitRequired, "a zero frozen time should be accepted and validation continue")
}

func TestAutoRepayAtCancelFalseIsSent(t *testing.T) {
	t.Parallel()
	// Binance defaults an absent autoRepayAtCancel to true, so an explicit false must
	// reach the request rather than being dropped as a zero value
	f := false
	p, err := interfaceToParams(&MarginAccountOrderRequest{AutoRepayAtCancel: &f})
	require.NoError(t, err, "serialising must not error")
	assert.Equal(t, "false", p.Get("autoRepayAtCancel"), "an explicit false should be transmitted")

	p, err = interfaceToParams(&MarginAccountOrderRequest{})
	require.NoError(t, err, "serialising must not error")
	assert.Empty(t, p.Get("autoRepayAtCancel"), "an unset value should be omitted")
}

func TestStringToOrderStatusSentinel(t *testing.T) {
	t.Parallel()
	_, err := stringToOrderStatus("NOT_A_REAL_STATUS")
	assert.ErrorIs(t, err, errUnrecognisedOrderStatus, "an unknown status should return the sentinel")

	for _, tc := range []struct {
		in   string
		want order.Status
	}{
		{"FILLED", order.Filled},
		{"NEW", order.New},
		{"PENDING_NEW", order.Pending},
		{"PARTIALLY_FILLED", order.PartiallyFilled},
		{"CANCELED", order.Cancelled},
		{"PENDING_CANCEL", order.PendingCancel},
		{"REJECTED", order.Rejected},
		{"EXPIRED", order.Expired},
		{"EXPIRED_IN_MATCH", order.STP},
		{"ACCEPTED", order.New},
		{"CANCELLED", order.Cancelled},
		{"NEW_ADL", order.AutoDeleverage},
		{"NEW_INSURANCE", order.Liquidated},
	} {
		status, err := stringToOrderStatus(tc.in)
		require.NoErrorf(t, err, "a known status must not error for %s", tc.in)
		assert.Equalf(t, tc.want, status, "%s should map to the expected status", tc.in)
	}
}

func TestUFuturesNewAlgoOrder(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesNewAlgoOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.UFuturesNewAlgoOrder(t.Context(), &UFuturesAlgoOrderRequest{})
	require.ErrorIs(t, err, errAlgoTypeRequired)

	_, err = e.UFuturesNewAlgoOrder(t.Context(), &UFuturesAlgoOrderRequest{AlgoType: "CONDITIONAL"})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UFuturesNewAlgoOrder(t.Context(), &UFuturesAlgoOrderRequest{AlgoType: "CONDITIONAL", Symbol: usdtmTradablePair})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	_, err = e.UFuturesNewAlgoOrder(t.Context(), &UFuturesAlgoOrderRequest{AlgoType: "CONDITIONAL", Symbol: usdtmTradablePair, Side: "BUY"})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	_, err = e.UFuturesNewAlgoOrder(t.Context(), &UFuturesAlgoOrderRequest{
		AlgoType: "CONDITIONAL", Symbol: usdtmTradablePair, Side: "BUY", OrderType: "STOP_MARKET", PositionSide: "SIDEWAYS",
	})
	require.ErrorIs(t, err, errInvalidPositionSide)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UFuturesNewAlgoOrder(t.Context(), &UFuturesAlgoOrderRequest{
		AlgoType: "CONDITIONAL", Symbol: usdtmTradablePair, Side: "BUY",
		OrderType: "STOP_MARKET", Quantity: 0.001, TriggerPrice: 100000,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesGetAlgoOrder(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesGetAlgoOrder(t.Context(), 0, "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UFuturesGetAlgoOrder(t.Context(), 1234567, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesCancelAlgoOrder(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesCancelAlgoOrder(t.Context(), 0, "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UFuturesCancelAlgoOrder(t.Context(), 1234567, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesCancelAllAlgoOrders(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesCancelAllAlgoOrders(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.UFuturesCancelAllAlgoOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesOpenAlgoOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UFuturesOpenAlgoOrders(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesAllAlgoOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.UFuturesAllAlgoOrders(t.Context(), usdtmTradablePair, endTime, startTime, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UFuturesAllAlgoOrders(t.Context(), usdtmTradablePair, startTime, endTime, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUAccountBalanceV3(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UAccountBalance(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUFuturesAccountConfig(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUFuturesAccountConfig(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUFuturesFeeBurnStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUFuturesFeeBurnStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetUFuturesFeeBurn(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	assert.NoError(t, e.SetUFuturesFeeBurn(t.Context(), true), "SetUFuturesFeeBurn should not error")
}

func TestUFuturesPing(t *testing.T) {
	t.Parallel()
	assert.NoError(t, e.UFuturesPing(t.Context()), "UFuturesPing should not error")
}

func TestGetUFuturesConvertPairs(t *testing.T) {
	t.Parallel()
	result, err := e.GetUFuturesConvertPairs(t.Context(), currency.BTC, currency.USDT)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSendUFuturesConvertQuoteRequest(t *testing.T) {
	t.Parallel()
	_, err := e.SendUFuturesConvertQuoteRequest(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.SendUFuturesConvertQuoteRequest(t.Context(), &UFuturesConvertQuoteRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.SendUFuturesConvertQuoteRequest(t.Context(), &UFuturesConvertQuoteRequest{FromAsset: currency.BTC, ToAsset: currency.USDT})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.SendUFuturesConvertQuoteRequest(t.Context(), &UFuturesConvertQuoteRequest{
		FromAsset: currency.BTC, ToAsset: currency.USDT, FromAmount: 0.001,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestAcceptUFuturesConvertQuote(t *testing.T) {
	t.Parallel()
	_, err := e.AcceptUFuturesConvertQuote(t.Context(), "")
	require.ErrorIs(t, err, errQuoteIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.AcceptUFuturesConvertQuote(t.Context(), "12415572564")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUFuturesConvertOrderStatus(t *testing.T) {
	t.Parallel()
	_, err := e.GetUFuturesConvertOrderStatus(t.Context(), "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUFuturesConvertOrderStatus(t.Context(), "933256278426274426", "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUFuturesInsuranceBalance(t *testing.T) {
	t.Parallel()
	result, err := e.GetUFuturesInsuranceBalance(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUFuturesSymbolADLRisk(t *testing.T) {
	t.Parallel()
	_, err := e.GetUFuturesSymbolADLRisk(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetUFuturesSymbolADLRisk(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUFuturesIndexConstituents(t *testing.T) {
	t.Parallel()
	_, err := e.GetUFuturesIndexConstituents(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetUFuturesIndexConstituents(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCFuturesPing(t *testing.T) {
	t.Parallel()
	assert.NoError(t, e.CFuturesPing(t.Context()), "CFuturesPing should not error")
}

func TestCFuturesServerTime(t *testing.T) {
	t.Parallel()
	result, err := e.CFuturesServerTime(t.Context())
	require.NoError(t, err)
	assert.False(t, result.IsZero(), "server time should not be zero")
}

func TestGetCFuturesOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := e.GetCFuturesOpenInterest(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetCFuturesOpenInterest(t.Context(), coinmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCFuturesCommissionRate(t *testing.T) {
	t.Parallel()
	_, err := e.GetCFuturesCommissionRate(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCFuturesCommissionRate(t.Context(), coinmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCFuturesPositionSideDual(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCFuturesPositionSideDual(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCFuturesLeverageBracketV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCFuturesLeverageBracket(t.Context(), coinmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEOptionsPing(t *testing.T) {
	t.Parallel()
	assert.NoError(t, e.EOptionsPing(t.Context()), "EOptionsPing should not error")
}

func TestGetEOptionsRecentBlockTrades(t *testing.T) {
	t.Parallel()
	result, err := e.GetEOptionsRecentBlockTrades(t.Context(), optionsTradablePair, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewOTOOrderList(t *testing.T) {
	t.Parallel()
	_, err := e.NewOTOOrderList(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.NewOTOOrderList(t.Context(), &OTOOrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.NewOTOOrderList(t.Context(), &OTOOrderRequest{Symbol: spotTradablePair})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	_, err = e.NewOTOOrderList(t.Context(), &OTOOrderRequest{Symbol: spotTradablePair, WorkingType: "LIMIT"})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	// pendingType and pendingSide are both mandatory for this endpoint
	_, err = e.NewOTOOrderList(t.Context(), &OTOOrderRequest{Symbol: spotTradablePair, WorkingType: "LIMIT", WorkingSide: "BUY"})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	_, err = e.NewOTOOrderList(t.Context(), &OTOOrderRequest{Symbol: spotTradablePair, WorkingType: "LIMIT", WorkingSide: "BUY", PendingSide: "SELL"})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewOTOOrderList(t.Context(), &OTOOrderRequest{
		Symbol: spotTradablePair, WorkingType: "LIMIT", WorkingSide: "BUY",
		WorkingPrice: 10000, WorkingQuantity: 0.001, PendingType: "LIMIT", PendingQuantity: 0.001,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewOTOCOOrderList(t *testing.T) {
	t.Parallel()
	_, err := e.NewOTOCOOrderList(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.NewOTOCOOrderList(t.Context(), &OTOCOOrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.NewOTOCOOrderList(t.Context(), &OTOCOOrderRequest{Symbol: spotTradablePair})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	_, err = e.NewOTOCOOrderList(t.Context(), &OTOCOOrderRequest{Symbol: spotTradablePair, WorkingType: "LIMIT", PendingAboveType: "LIMIT"})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)
}

func TestAmendOrderKeepPriority(t *testing.T) {
	t.Parallel()
	_, err := e.AmendOrderKeepPriority(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.AmendOrderKeepPriority(t.Context(), &AmendKeepPriorityRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.AmendOrderKeepPriority(t.Context(), &AmendKeepPriorityRequest{Symbol: spotTradablePair})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	_, err = e.AmendOrderKeepPriority(t.Context(), &AmendKeepPriorityRequest{Symbol: spotTradablePair, OrderID: 12345})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	// The endpoint takes no price parameter, so a quantity alone is a complete request
	_, err = e.AmendOrderKeepPriority(t.Context(), &AmendKeepPriorityRequest{Symbol: spotTradablePair, OrderID: 12345, NewQuantity: 1})
	assert.NoError(t, err, "AmendOrderKeepPriority should not error")
}

func TestGetOrderAmendments(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderAmendments(t.Context(), &GetOrderAmendmentsRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetOrderAmendments(t.Context(), &GetOrderAmendmentsRequest{Symbol: spotTradablePair})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOrderAmendments(t.Context(), &GetOrderAmendmentsRequest{Symbol: spotTradablePair, OrderID: 12345, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAccountSymbolFilters(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetAccountSymbolFilters(t.Context(), spotTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetExecutionRules(t *testing.T) {
	t.Parallel()
	result, err := e.GetExecutionRules(t.Context(), spotTradablePair, nil, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSpotPing(t *testing.T) {
	t.Parallel()
	assert.NoError(t, e.SpotPing(t.Context()), "SpotPing should not error")
}

func TestGetSpotReferencePrice(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotReferencePrice(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetSpotReferencePrice(t.Context(), spotTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetSpotReferencePriceCalculation(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotReferencePriceCalculation(t.Context(), currency.EMPTYPAIR, "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetSpotReferencePriceCalculation(t.Context(), spotTradablePair, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetHistoricalBlockTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricalBlockTrades(t.Context(), currency.EMPTYPAIR, 0, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	result, err := e.GetHistoricalBlockTrades(t.Context(), spotTradablePair, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesInsuranceBalancesUnmarshalJSON(t *testing.T) {
	t.Parallel()
	// The endpoint returns a single object when a symbol is supplied and an array when not.
	var single UFuturesInsuranceBalances
	require.NoError(t, json.Unmarshal([]byte(`{"symbols":["BTCUSDT"],"assets":[{"asset":"USDT","marginBalance":"1.5","updateTime":1785715201000}]}`), &single))
	require.Len(t, single, 1, "a single object must unmarshal to one element")
	assert.Equal(t, 1.5, single[0].Assets[0].MarginBalance.Float64(), "margin balance should be parsed")

	var many UFuturesInsuranceBalances
	require.NoError(t, json.Unmarshal([]byte(`[{"symbols":["BTCUSDT"],"assets":[]},{"symbols":["ETHUSDT"],"assets":[]}]`), &many))
	assert.Len(t, many, 2, "an array should unmarshal to two elements")

	var bad UFuturesInsuranceBalances
	assert.Error(t, json.Unmarshal([]byte(`"nope"`), &bad), "invalid input should error")
}

func TestPortfolioMarginPing(t *testing.T) {
	t.Parallel()
	assert.NoError(t, e.PortfolioMarginPing(t.Context()), "PortfolioMarginPing should not error")
}

func TestGetUMAccountDetailV2(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMAccountDetail(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewUMAlgoOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewUMAlgoOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.NewUMAlgoOrder(t.Context(), &UMAlgoOrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.NewUMAlgoOrder(t.Context(), &UMAlgoOrderRequest{Symbol: usdtmTradablePair})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)
}

func TestCancelUMAlgoOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelUMAlgoOrder(t.Context(), 0)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)
}

func TestCancelAllUMAlgoOpenOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllUMAlgoOpenOrders(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
}

func TestGetUMOpenAlgoOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetUMOpenAlgoOrder(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMOpenAlgoOrder(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAllUMOpenAlgoOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetAllUMOpenAlgoOrders(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
}

func TestGetUMAlgoOrderHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetUMAlgoOrderHistory(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
}

func TestGetUMFuturesAccountConfig(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMFuturesAccountConfig(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUMFuturesSymbolConfig(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMFuturesSymbolConfig(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRepayMarginDebt(t *testing.T) {
	t.Parallel()
	_, err := e.RepayMarginDebt(t.Context(), currency.EMPTYCODE, 0, nil)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.RepayMarginDebt(t.Context(), currency.USDT, 0, nil)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.RepayMarginDebt(t.Context(), currency.USDT, 1, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginPreventedMatches(t *testing.T) {
	t.Parallel()
	_, err := e.GetMarginPreventedMatches(t.Context(), &GetMarginPreventedMatchesRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginPreventedMatches(t.Context(), &GetMarginPreventedMatchesRequest{Symbol: spotTradablePair, OrderID: 12345, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginLimitPricePairs(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginLimitPricePairs(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginListSchedule(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginListSchedule(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginRestrictedAssets(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginRestrictedAssets(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginAssetLiquidationRatios(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginAssetLiquidationRatios(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCFuturesDownloadIDEndpoints(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	for _, fn := range []func(context.Context, time.Time, time.Time) (*UTransactionDownloadID, error){
		e.GetCFuturesTransactionHistoryDownloadID,
		e.GetCFuturesOrderHistoryDownloadID,
		e.GetCFuturesTradeHistoryDownloadID,
	} {
		_, err := fn(t.Context(), endTime, startTime)
		require.ErrorIs(t, err, common.ErrStartAfterEnd, "reversed times must be rejected")
	}

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCFuturesTransactionHistoryDownloadID(t.Context(), startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCFuturesDownloadLinkEndpoints(t *testing.T) {
	t.Parallel()
	for _, fn := range []func(context.Context, string) (*UTransactionHistoryDownloadLink, error){
		e.GetCFuturesTransactionHistoryDownloadLink,
		e.GetCFuturesOrderHistoryDownloadLink,
		e.GetCFuturesTradeHistoryDownloadLink,
	} {
		_, err := fn(t.Context(), "")
		require.ErrorIs(t, err, errDownloadIDRequired, "an empty download ID must be rejected")
	}
}

func TestPortfolioMarginDownloadEndpoints(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	for _, fn := range []func(context.Context, time.Time, time.Time) (*UTransactionDownloadID, error){
		e.GetUMTransactionHistoryDownloadID,
		e.GetUMOrderHistoryDownloadID,
		e.GetUMTradeHistoryDownloadID,
	} {
		_, err := fn(t.Context(), endTime, startTime)
		require.ErrorIs(t, err, common.ErrStartAfterEnd, "reversed times must be rejected")
	}
	for _, fn := range []func(context.Context, string) (*UTransactionHistoryDownloadLink, error){
		e.GetUMTransactionHistoryDownloadLink,
		e.GetUMOrderHistoryDownloadLink,
		e.GetUMTradeHistoryDownloadLink,
	} {
		_, err := fn(t.Context(), "")
		require.ErrorIs(t, err, errDownloadIDRequired, "an empty download ID must be rejected")
	}
}

func TestStablecoinYieldAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	bfusd, err := e.GetBFUSDAccount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, bfusd)

	rwusd, err := e.GetRWUSDAccount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, rwusd)
}

func TestStablecoinYieldQuotas(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	for _, fn := range []func(context.Context) (*StablecoinYieldQuota, error){e.GetBFUSDQuota, e.GetRWUSDQuota} {
		result, err := fn(t.Context())
		require.NoError(t, err)
		assert.NotNil(t, result)
	}
}

func TestSubscribeStablecoinYield(t *testing.T) {
	t.Parallel()
	for _, fn := range []func(context.Context, currency.Code, float64) (*StablecoinYieldSubscription, error){e.SubscribeBFUSD, e.SubscribeRWUSD} {
		_, err := fn(t.Context(), currency.EMPTYCODE, 1)
		require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

		_, err = fn(t.Context(), currency.USDT, 0)
		require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	}
}

func TestRedeemStablecoinYield(t *testing.T) {
	t.Parallel()
	for _, fn := range []func(context.Context, float64, string) (*StablecoinYieldRedemption, error){e.RedeemBFUSD, e.RedeemRWUSD} {
		_, err := fn(t.Context(), 0, "")
		require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	}
}

func TestStablecoinYieldHistories(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetBFUSDRateHistory(t.Context(), endTime, startTime, 1, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	rates, err := e.GetBFUSDRateHistory(t.Context(), startTime, endTime, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, rates)

	rewards, err := e.GetRWUSDRewardsHistory(t.Context(), startTime, endTime, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, rewards)

	subs, err := e.GetBFUSDSubscriptionHistory(t.Context(), &GetBFUSDSubscriptionHistoryRequest{AssetCode: currency.USDT, StartTime: startTime, EndTime: endTime, Current: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, subs)

	redemptions, err := e.GetRWUSDRedemptionHistory(t.Context(), startTime, endTime, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, redemptions)
}

func TestGetYieldArenaActivities(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetYieldArenaActivities(t.Context(), "en")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOnChainYieldsAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOnChainYieldsAccount(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOnChainYieldsLockedProducts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOnChainYieldsLockedProducts(t.Context(), currency.USDT, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOnChainYieldsPositions(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOnChainYieldsPositions(t.Context(), &GetOnChainYieldsPositionsRequest{AssetCode: currency.USDT, Current: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOnChainYieldsLeftQuota(t *testing.T) {
	t.Parallel()
	_, err := e.GetOnChainYieldsLeftQuota(t.Context(), "")
	require.ErrorIs(t, err, errProjectIDRequired)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOnChainYieldsLeftQuota(t.Context(), "Bitcoin*180")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOnChainYieldsSubscriptionPreview(t *testing.T) {
	t.Parallel()
	_, err := e.GetOnChainYieldsSubscriptionPreview(t.Context(), "", 1, false)
	require.ErrorIs(t, err, errProjectIDRequired)

	_, err = e.GetOnChainYieldsSubscriptionPreview(t.Context(), "Bitcoin*180", 0, false)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
}

func TestSubscribeOnChainYieldsLockedProduct(t *testing.T) {
	t.Parallel()
	_, err := e.SubscribeOnChainYieldsLockedProduct(t.Context(), &SubscribeOnChainYieldsLockedProductRequest{Amount: 1})
	require.ErrorIs(t, err, errProjectIDRequired)

	_, err = e.SubscribeOnChainYieldsLockedProduct(t.Context(), &SubscribeOnChainYieldsLockedProductRequest{ProjectID: "Bitcoin*180"})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
}

func TestOnChainYieldsPositionManagement(t *testing.T) {
	t.Parallel()
	_, err := e.RedeemOnChainYieldsLockedProduct(t.Context(), "", "")
	require.ErrorIs(t, err, errPositionIDRequired)

	_, err = e.SetOnChainYieldsAutoSubscribe(t.Context(), "", true)
	require.ErrorIs(t, err, errPositionIDRequired)

	_, err = e.SetOnChainYieldsRedeemOption(t.Context(), "", "SPOT")
	require.ErrorIs(t, err, errPositionIDRequired)

	_, err = e.SetOnChainYieldsRedeemOption(t.Context(), "123", "")
	require.ErrorIs(t, err, errRedemptionAccountRequired)
}

func TestGetSoftStakingProducts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSoftStakingProducts(t.Context(), currency.USDT, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetSoftStaking(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.SetSoftStaking(t.Context(), true)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetWithdrawQuota(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetWithdrawQuota(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetOpenSymbolList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetOpenSymbolList(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetQuestionnaireRequirements(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetQuestionnaireRequirements(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetDustConvertibleAssets(t *testing.T) {
	t.Parallel()
	_, err := e.GetDustConvertibleAssets(t.Context(), currency.EMPTYCODE, "", 0)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetDustConvertibleAssets(t.Context(), currency.BNB, "SPOT", 0)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDustConvert(t *testing.T) {
	t.Parallel()
	_, err := e.DustConvert(t.Context(), &DustConvertRequest{TargetAsset: currency.BNB})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.DustConvert(t.Context(), &DustConvertRequest{Assets: currency.Currencies{currency.LTC}})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.DustConvert(t.Context(), &DustConvertRequest{Assets: currency.Currencies{currency.LTC}, TargetAsset: currency.BNB, AccountType: "SPOT"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMarginSpecialKeyManagement(t *testing.T) {
	t.Parallel()
	_, err := e.CreateMarginSpecialKey(t.Context(), &CreateMarginSpecialKeyRequest{Symbol: spotTradablePair})
	require.ErrorIs(t, err, errAPIKeyNameRequired)

	err = e.DeleteMarginSpecialKey(t.Context(), "", spotTradablePair)
	require.ErrorIs(t, err, errAPIKeyNameRequired)

	err = e.EditMarginSpecialKeyIP(t.Context(), "", spotTradablePair)
	require.ErrorIs(t, err, errInvalidIPAddress)
}

func TestGetMarginSpecialKey(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginSpecialKey(t.Context(), spotTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMarginSpecialKeyList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginSpecialKeyList(t.Context(), spotTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestExitMarginSpecialKeyMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	assert.NoError(t, e.ExitMarginSpecialKeyMode(t.Context()), "ExitMarginSpecialKeyMode should not error")
}

func TestRepayMarginLiquidationLoan(t *testing.T) {
	t.Parallel()
	_, err := e.RepayMarginLiquidationLoan(t.Context(), currency.EMPTYCODE, 1)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.RepayMarginLiquidationLoan(t.Context(), currency.USDT, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
}

func TestGetMarginLiquidationLoanRepayHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetMarginLiquidationLoanRepayHistory(t.Context(), endTime, startTime, 1, 10)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetMarginLiquidationLoanRepayHistory(t.Context(), startTime, endTime, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSubAccountAPIKeyManagement(t *testing.T) {
	t.Parallel()
	_, err := e.CreateSubAccountAPIKey(t.Context(), &SubAccountAPIKeyRequest{Email: "notanemail", APIName: "key", CanTrade: true})
	require.ErrorIs(t, err, errValidEmailRequired)

	_, err = e.CreateSubAccountAPIKey(t.Context(), &SubAccountAPIKeyRequest{Email: "sub@thrasher.io", CanTrade: true})
	require.ErrorIs(t, err, errAPIKeyNameRequired)

	_, err = e.GetSubAccountAPIKeys(t.Context(), "notanemail", "", 1, 10)
	require.ErrorIs(t, err, errValidEmailRequired)

	err = e.DeleteSubAccountAPIKey(t.Context(), "notanemail", "key")
	require.ErrorIs(t, err, errValidEmailRequired)

	err = e.DeleteSubAccountAPIKey(t.Context(), "sub@thrasher.io", "")
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)

	_, err = e.ModifySubAccountAPIKeyPermission(t.Context(), &SubAccountAPIKeyPermissionRequest{Email: "sub@thrasher.io", CanTrade: true})
	require.ErrorIs(t, err, errEmptySubAccountAPIKey)
}

func TestGetFiatOrderDetail(t *testing.T) {
	t.Parallel()
	_, err := e.GetFiatOrderDetail(t.Context(), "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFiatOrderDetail(t.Context(), "12345678")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetVIPFixedRateLoanMarket(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetVIPFixedRateLoanMarket(t.Context(), currency.USDT, 30, 1, 10)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetFlexibleLoanInterestRateHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFlexibleLoanInterestRateHistory(t.Context(), &GetFlexibleLoanInterestRateHistoryRequest{Coin: currency.USDT, StartTime: endTime, EndTime: startTime, Current: 1, Limit: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetFlexibleLoanInterestRateHistory(t.Context(), &GetFlexibleLoanInterestRateHistoryRequest{Coin: currency.USDT, StartTime: startTime, EndTime: endTime, Current: 1, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestOnChainYieldsHistoryRecords(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetOnChainYieldsRedemptionRecords(t.Context(), &GetOnChainYieldsRedemptionRecordsRequest{AssetCode: currency.USDT, StartTime: endTime, EndTime: startTime, Current: 1, Size: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	redemptions, err := e.GetOnChainYieldsRedemptionRecords(t.Context(), &GetOnChainYieldsRedemptionRecordsRequest{AssetCode: currency.USDT, StartTime: startTime, EndTime: endTime, Current: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, redemptions)

	rewards, err := e.GetOnChainYieldsRewardsRecords(t.Context(), &GetOnChainYieldsRewardsRecordsRequest{AssetCode: currency.USDT, StartTime: startTime, EndTime: endTime, Current: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, rewards)

	subs, err := e.GetOnChainYieldsSubscriptionRecords(t.Context(), &GetOnChainYieldsSubscriptionRecordsRequest{AssetCode: currency.USDT, StartTime: startTime, EndTime: endTime, Current: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, subs)

	soft, err := e.GetSoftStakingRewardsRecords(t.Context(), &GetSoftStakingRewardsRecordsRequest{AssetCode: currency.USDT, StartTime: startTime, EndTime: endTime, Current: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, soft)
}

func TestGetUFuturesSymbolConfig(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUFuturesSymbolConfig(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUAccountInformationV3(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UAccountInformation(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUPositionsInfoV3(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UPositionsInfo(t.Context(), usdtmTradablePair)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetUFuturesPortfolioMarginAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUFuturesPortfolioMarginAccountInfo(t.Context(), currency.USDT)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetCFuturesOrderModifyHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetCFuturesOrderModifyHistory(t.Context(), &GetCFuturesOrderModifyHistoryRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	startTime, endTime := getTime()
	_, err = e.GetCFuturesOrderModifyHistory(t.Context(), &GetCFuturesOrderModifyHistoryRequest{Symbol: coinmTradablePair, StartTime: endTime, EndTime: startTime})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCFuturesOrderModifyHistory(t.Context(), &GetCFuturesOrderModifyHistoryRequest{Symbol: coinmTradablePair, StartTime: startTime, EndTime: endTime, Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPortfolioMarginOrderModifyHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetUMOrderModifyHistory(t.Context(), &GetUMOrderModifyHistoryRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetCMOrderModifyHistory(t.Context(), &GetCMOrderModifyHistoryRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
}

func TestUMFeeBurn(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetUMFeeBurnStatus(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetUMFeeBurn(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	assert.NoError(t, e.SetUMFeeBurn(t.Context(), true), "SetUMFeeBurn should not error")
}

func TestGetCMConditionalOpenOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetCMConditionalOpenOrder(t.Context(), currency.EMPTYPAIR, 0, "")
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetCMConditionalOpenOrder(t.Context(), coinmTradablePair, 0, "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetCMConditionalOpenOrder(t.Context(), coinmTradablePair, 1234567, "")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewMarginOTOOrderList(t *testing.T) {
	t.Parallel()
	_, err := e.NewMarginOTOOrderList(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.NewMarginOTOOrderList(t.Context(), &MarginOTOOrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.NewMarginOTOOrderList(t.Context(), &MarginOTOOrderRequest{Symbol: spotTradablePair})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	_, err = e.NewMarginOTOOrderList(t.Context(), &MarginOTOOrderRequest{
		Symbol: spotTradablePair, WorkingType: "LIMIT", PendingType: "LIMIT",
	})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	_, err = e.NewMarginOTOOrderList(t.Context(), &MarginOTOOrderRequest{
		Symbol: spotTradablePair, WorkingType: "LIMIT", PendingType: "LIMIT",
		WorkingSide: "BUY", PendingSide: "SELL",
	})
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
}

func TestNewMarginOTOCOOrderList(t *testing.T) {
	t.Parallel()
	_, err := e.NewMarginOTOCOOrderList(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.NewMarginOTOCOOrderList(t.Context(), &MarginOTOCOOrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.NewMarginOTOCOOrderList(t.Context(), &MarginOTOCOOrderRequest{Symbol: spotTradablePair})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	_, err = e.NewMarginOTOCOOrderList(t.Context(), &MarginOTOCOOrderRequest{
		Symbol: spotTradablePair, WorkingType: "LIMIT", PendingAboveType: "LIMIT_MAKER",
	})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)
}

func TestNewEOptionsBlockTradeOrder(t *testing.T) {
	t.Parallel()
	_, err := e.NewEOptionsBlockTradeOrder(t.Context(), "", nil)
	require.ErrorIs(t, err, errLiquidityRequired)

	_, err = e.NewEOptionsBlockTradeOrder(t.Context(), "TAKER", nil)
	require.ErrorIs(t, err, common.ErrEmptyParams)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.NewEOptionsBlockTradeOrder(t.Context(), "TAKER", []EOptionsBlockTradeLeg{{
		Symbol: optionsTradablePair.String(), Side: "BUY", Type: "LIMIT", Quantity: 1, Price: 100,
	}})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEOptionsBlockTradeOrderLifecycle(t *testing.T) {
	t.Parallel()
	for _, fn := range []func(context.Context, string) (*EOptionsBlockTradeOrder, error){
		e.ExtendEOptionsBlockTradeOrder,
		e.CancelEOptionsBlockTradeOrder,
		e.AcceptEOptionsBlockTradeOrder,
		e.GetEOptionsBlockTradeDetail,
	} {
		_, err := fn(t.Context(), "")
		require.ErrorIs(t, err, errBlockOrderMatchingKeyRequired, "an empty matching key must be rejected")
	}
}

func TestGetEOptionsBlockTradeOrders(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetEOptionsBlockTradeOrders(t.Context(), "", "BTCUSDT", endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetEOptionsBlockTradeOrders(t.Context(), "", "BTCUSDT", startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEOptionsBlockUserTrades(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetEOptionsBlockUserTrades(t.Context(), "BTCUSDT", endTime, startTime)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetEOptionsBlockUserTrades(t.Context(), "BTCUSDT", startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEOptionsUserCommission(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetEOptionsUserCommission(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUFuturesTestNewOrder(t *testing.T) {
	t.Parallel()
	_, err := e.UFuturesTestNewOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.UFuturesTestNewOrder(t.Context(), &UFuturesNewOrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.UFuturesTestNewOrder(t.Context(), &UFuturesNewOrderRequest{Symbol: usdtmTradablePair})
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	_, err = e.UFuturesTestNewOrder(t.Context(), &UFuturesNewOrderRequest{Symbol: usdtmTradablePair, Side: "BUY"})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.UFuturesTestNewOrder(t.Context(), &UFuturesNewOrderRequest{
		Symbol: usdtmTradablePair, Side: "BUY", OrderType: "MARKET", Quantity: 0.001,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBorrowVIPFixedRateLoan(t *testing.T) {
	t.Parallel()
	_, err := e.BorrowVIPFixedRateLoan(t.Context(), &BorrowVIPFixedRateLoanRequest{LoanTerm: 30, CollateralCoin: currency.BTC})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	result, err := e.BorrowVIPFixedRateLoan(t.Context(), &BorrowVIPFixedRateLoanRequest{SupplyRequest: "1234", BorrowCoin: currency.USDT, LoanTerm: 30, CollateralCoin: currency.BTC, AutoRepay: true})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMoveSubAccountPosition(t *testing.T) {
	t.Parallel()
	_, err := e.MoveSubAccountPosition(t.Context(), "notanemail", "to@thrasher.io", "UM", nil)
	require.ErrorIs(t, err, errValidEmailRequired)

	_, err = e.MoveSubAccountPosition(t.Context(), "from@thrasher.io", "to@thrasher.io", "UM", nil)
	require.ErrorIs(t, err, common.ErrEmptyParams)
}

func TestGetSubAccountMovePositionHistory(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetSubAccountMovePositionHistory(t.Context(), &GetSubAccountMovePositionHistoryRequest{Symbol: usdtmTradablePair, StartTime: endTime, EndTime: startTime, Page: 1, Rows: 10})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	result, err := e.GetSubAccountMovePositionHistory(t.Context(), &GetSubAccountMovePositionHistoryRequest{Symbol: usdtmTradablePair, StartTime: startTime, EndTime: endTime, Page: 1, Rows: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPaginateOrderHistory(t *testing.T) {
	t.Parallel()

	type row struct{ id uint64 }
	lastID := func(r row) uint64 { return r.id }

	// A short first page terminates immediately
	calls := 0
	out, err := paginateOrderHistory(orderHistoryPageSize, func(uint64) ([]row, error) {
		calls++
		return []row{{id: 1}, {id: 2}}, nil
	}, lastID)
	require.NoError(t, err, "a short page must not error")
	assert.Len(t, out, 2, "a short page should return its rows")
	assert.Equal(t, 1, calls, "a short page should only be fetched once")

	// Full pages are followed until a short page arrives, resuming past the last ID
	calls = 0
	base := uint64(1000)
	var seen []uint64
	out, err = paginateOrderHistory(orderHistoryPageSize, func(fromID uint64) ([]row, error) {
		seen = append(seen, fromID)
		calls++
		if calls > 2 {
			return []row{{id: base}}, nil
		}
		page := make([]row, orderHistoryPageSize)
		id := base
		for i := range page {
			page[i] = row{id: id}
			id++
		}
		base += 1000
		return page, nil
	}, lastID)
	require.NoError(t, err, "paging must not error")
	assert.Len(t, out, orderHistoryPageSize*2+1, "every page should be accumulated")
	assert.Equal(t, []uint64{0, 1000 + orderHistoryPageSize - 1 + 1, 2000 + orderHistoryPageSize - 1 + 1}, seen,
		"each page should resume one past the last order ID")

	// A response that never advances the cursor must terminate rather than loop
	calls = 0
	out, err = paginateOrderHistory(orderHistoryPageSize, func(uint64) ([]row, error) {
		calls++
		page := make([]row, orderHistoryPageSize)
		for i := range page {
			page[i] = row{id: 0}
		}
		return page, nil
	}, lastID)
	require.NoError(t, err, "a non-advancing cursor must not error")
	// The first page legitimately advances the cursor from 0 to 1; the second cannot
	// advance further, so paging stops there rather than repeating indefinitely.
	assert.Len(t, out, orderHistoryPageSize*2, "a non-advancing cursor should stop after the second page")
	assert.Equal(t, 2, calls, "a non-advancing cursor should stop rather than loop")

	// Errors propagate
	_, err = paginateOrderHistory(orderHistoryPageSize, func(uint64) ([]row, error) {
		return nil, errUnrecognisedOrderStatus
	}, lastID)
	assert.ErrorIs(t, err, errUnrecognisedOrderStatus, "fetch errors should propagate")
}
