package binance

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/mock"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// TestPublicEndpointsRecorded replays production responses through the PR's VCR mock server.
func TestPublicEndpointsRecorded(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "mock exchange must initialise")
	server, client, err := mock.NewVCRServer("testdata/public_api/http.json")
	require.NoError(t, err, "recorded public mock server must initialise")
	observer := &publicAuditTransport{t: t, base: client.Transport}
	client.Transport = observer
	require.NoError(t, local.SetHTTPClient(client), "mock HTTP client must initialise")
	for key := range local.API.Endpoints.GetURLMap() {
		require.NoError(t, local.API.Endpoints.SetRunningURL(key, server), "all HTTP endpoints must use the mock server")
	}
	runPublicEndpointChecks(t, local, observer, false)
}

// TestPublicEndpointsLive opts into production market data without loading credentials.
// Run with BINANCE_LIVE_PUBLIC=1 go test ./exchanges/binance -run '^TestPublicEndpointsLive$' -count=1 -v.
// BINANCE_RECORD_PUBLIC optionally records the responses in the existing VCR fixture format.
func TestPublicEndpointsLive(t *testing.T) {
	if os.Getenv("BINANCE_LIVE_PUBLIC") != "1" {
		t.Skip("set BINANCE_LIVE_PUBLIC=1 to check production public APIs")
	}
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "public exchange must initialise")
	observer := &publicAuditTransport{t: t, base: http.DefaultTransport, live: true}
	if path := os.Getenv("BINANCE_RECORD_PUBLIC"); path != "" {
		observer.recording = &mock.VCRMock{Routes: make(map[string]map[string][]mock.HTTPResponse)}
		t.Cleanup(func() {
			data, err := json.MarshalIndent(observer.recording, "", " ")
			require.NoError(t, err, "public responses must serialise as mock fixtures")
			require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600), "public mock fixture must be saved")
		})
	}
	require.NoError(t, local.SetHTTPClient(&http.Client{Timeout: 20 * time.Second, Transport: observer}), "public HTTP client must initialise")
	runPublicEndpointChecks(t, local, observer, true)
}

func runPublicEndpointChecks(t *testing.T, local *Exchange, observer *publicAuditTransport, live bool) {
	t.Helper()
	spotPair := currency.NewBTCUSDT()
	coinPair := currency.NewPairWithDelimiter("BTCUSD", "PERP", currency.UnderscoreDelimiter)
	coinUnderlying := currency.NewBTCUSD()
	info, err := local.GetOptionsExchangeInformation(t.Context())
	require.NoError(t, err, "active options must load")
	var optionPair currency.Pair
	var expiration time.Time
	for _, symbol := range info.OptionSymbols {
		if symbol.Underlying == "BTCUSDT" && (!live || symbol.ExpiryDate.Time().After(time.Now())) && (expiration.IsZero() || symbol.ExpiryDate.Time().Before(expiration)) {
			optionPair = symbol.Symbol
			expiration = symbol.ExpiryDate.Time()
		}
	}
	require.False(t, optionPair.IsEmpty(), "an active BTC option must be available")
	t.Logf("Public probes use option %s, observed at %s", optionPair, time.Now().UTC().Format(time.RFC3339))
	for _, tc := range []struct {
		name string
		call func(*testing.T) (any, error)
	}{
		{"CFuturesPing", func(t *testing.T) (any, error) { t.Helper(); return nil, local.CFuturesPing(t.Context()) }},
		{"CFuturesQuarterlyContractSettlementPrice", func(t *testing.T) (any, error) {
			t.Helper()
			return local.CFuturesQuarterlyContractSettlementPrice(t.Context(), coinUnderlying)
		}},
		{"CFuturesServerTime", func(t *testing.T) (any, error) { t.Helper(); return local.CFuturesServerTime(t.Context()) }},
		{"CheckEOptionsServerTime", func(t *testing.T) (any, error) { t.Helper(); return local.CheckEOptionsServerTime(t.Context()) }},
		{"EOptionsPing", func(t *testing.T) (any, error) { t.Helper(); return nil, local.EOptionsPing(t.Context()) }},
		{"FuturesExchangeInfo", func(t *testing.T) (any, error) { t.Helper(); return local.FuturesExchangeInfo(t.Context()) }},
		{"FuturesGetFundingHistory", func(t *testing.T) (any, error) {
			t.Helper()
			return local.FuturesGetFundingHistory(t.Context(), coinPair, 10, time.Time{}, time.Time{})
		}},
		{"GetAggregatedTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetAggregatedTrades(t.Context(), &AggregatedTradeRequest{Symbol: spotPair, Limit: 10})
		}},
		{"GetAllConvertPairs", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetAllConvertPairs(t.Context(), currency.BTC, currency.USDT)
		}},
		{"GetAveragePrice", func(t *testing.T) (any, error) { t.Helper(); return local.GetAveragePrice(t.Context(), spotPair) }},
		{"GetBasis", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetBasis(t.Context(), &GetBasisRequest{Pair: spotPair, ContractType: "PERPETUAL", Period: "5m", Limit: 10})
		}},
		{"GetBestPrice", func(t *testing.T) (any, error) { t.Helper(); return local.GetBestPrice(t.Context(), spotPair, nil) }},
		{"GetCFuturesIndexPriceConstituents", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetCFuturesIndexPriceConstituents(t.Context(), coinUnderlying)
		}},
		{"GetCFuturesOpenInterest", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetCFuturesOpenInterest(t.Context(), coinPair)
		}},
		{"GetContinuousKlineData", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetContinuousKlineData(t.Context(), &GetContinuousKlineDataRequest{Pair: "BTCUSD", ContractType: "PERPETUAL", Interval: "1m", Limit: 10})
		}},
		{"GetEOptions24hrTickerPriceChangeStatistics", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetEOptions24hrTickerPriceChangeStatistics(t.Context(), optionPair)
		}},
		{"GetEOptionsCandlesticks", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetEOptionsCandlesticks(t.Context(), &GetEOptionsCandlesticksRequest{Symbol: optionPair, Interval: kline.OneMin, Limit: 10})
		}},
		{"GetEOptionsHistoricalExerciseRecords", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetEOptionsHistoricalExerciseRecords(t.Context(), "BTCUSDT", time.Time{}, time.Time{}, 10)
		}},
		{"GetEOptionsOpenInterests", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetEOptionsOpenInterests(t.Context(), currency.BTC, expiration)
		}},
		{"GetEOptionsOrderbook", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetEOptionsOrderbook(t.Context(), optionPair, 10)
		}},
		{"GetEOptionsRecentBlockTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetEOptionsRecentBlockTrades(t.Context(), currency.EMPTYPAIR, 10)
		}},
		{"GetEOptionsRecentTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetEOptionsRecentTrades(t.Context(), optionPair, 10)
		}},
		{"GetEOptionsSymbolPriceTicker", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetEOptionsSymbolPriceTicker(t.Context(), "BTCUSDT")
		}},
		{"GetExchangeInfo", func(t *testing.T) (any, error) { t.Helper(); return local.GetExchangeInfo(t.Context()) }},
		{"GetExchangeServerTime", func(t *testing.T) (any, error) { t.Helper(); return local.GetExchangeServerTime(t.Context()) }},
		{"GetExecutionRules", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetExecutionRules(t.Context(), spotPair, nil, "")
		}},
		{"GetFundingRateInfo", func(t *testing.T) (any, error) { t.Helper(); return local.GetFundingRateInfo(t.Context()) }},
		{"GetFuturesAggregatedTradesList", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesAggregatedTradesList(t.Context(), &GetFuturesAggregatedTradesListRequest{Symbol: coinPair, Limit: 10})
		}},
		{"GetFuturesBasisData", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesBasisData(t.Context(), &GetFuturesBasisDataRequest{Pair: coinUnderlying, ContractType: "PERPETUAL", Period: "5m", Limit: 10})
		}},
		{"GetFuturesKlineData", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesKlineData(t.Context(), &GetFuturesKlineDataRequest{Symbol: coinPair, Interval: "1m", Limit: 10})
		}},
		{"GetFuturesOrderbook", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesOrderbook(t.Context(), coinPair, 10)
		}},
		{"GetFuturesOrderbookTicker", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesOrderbookTicker(t.Context(), coinPair, "BTCUSD")
		}},
		{"GetFuturesPublicTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesPublicTrades(t.Context(), coinPair, 10)
		}},
		{"GetFuturesSwapTickerChangeStats", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesSwapTickerChangeStats(t.Context(), coinPair, "BTCUSD")
		}},
		{"GetFuturesSymbolPriceTicker", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesSymbolPriceTicker(t.Context(), coinPair, "BTCUSD")
		}},
		{"GetFuturesTakerVolume", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetFuturesTakerVolume(t.Context(), &GetFuturesTakerVolumeRequest{Pair: coinUnderlying, ContractType: "PERPETUAL", Period: "5m", Limit: 10})
		}},
		{"GetHistoricalBlockTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetHistoricalBlockTrades(t.Context(), spotPair, 0, 10)
		}},
		{"GetHistoricalTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetHistoricalTrades(t.Context(), spotPair, 10, 0)
		}},
		{"GetIndexAndMarkPrice", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetIndexAndMarkPrice(t.Context(), "BTCUSD_PERP", "BTCUSD")
		}},
		{"GetIndexPriceConstituents", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetIndexPriceConstituents(t.Context(), "BTCUSDT")
		}},
		{"GetIndexPriceKlineData", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetIndexPriceKlineData(t.Context(), &GetIndexPriceKlineDataRequest{Pair: spotPair, Interval: "1m", Limit: 10})
		}},
		{"GetIndexPriceKlines", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetIndexPriceKlines(t.Context(), &GetIndexPriceKlinesRequest{Pair: "BTCUSD", Interval: "1m", Limit: 10})
		}},
		{"GetLatestSpotPrice", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetLatestSpotPrice(t.Context(), spotPair, nil)
		}},
		{"GetMarkPriceKline", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetMarkPriceKline(t.Context(), &GetMarkPriceKlineRequest{Symbol: coinPair, Interval: "1m", Limit: 10})
		}},
		{"GetMarkPriceKlineCandlesticks", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetMarkPriceKlineCandlesticks(t.Context(), &GetMarkPriceKlineCandlesticksRequest{Symbol: spotPair, Interval: "1m", Limit: 10})
		}},
		{"GetMarketRatio", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetMarketRatio(t.Context(), &GetMarketRatioRequest{Pair: coinUnderlying, Period: "5m", Limit: 10})
		}},
		{"GetMostRecentTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetMostRecentTrades(t.Context(), &RecentTradeRequest{Symbol: spotPair, Limit: 10})
		}},
		{"GetMultiAssetModeAssetIndex", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetMultiAssetModeAssetIndex(t.Context(), currency.NewBTCUSD())
		}},
		{"GetOpenInterestStats", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetOpenInterestStats(t.Context(), &GetOpenInterestStatsRequest{Pair: "BTCUSD", ContractType: "PERPETUAL", Period: "5m", Limit: 10})
		}},
		{"GetOptionMarkPrice", func(t *testing.T) (any, error) { t.Helper(); return local.GetOptionMarkPrice(t.Context(), optionPair) }},
		{"GetOptionsExchangeInformation", func(t *testing.T) (any, error) { t.Helper(); return local.GetOptionsExchangeInformation(t.Context()) }},
		{"GetOrderBook", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetOrderBook(t.Context(), OrderBookDataRequest{Symbol: spotPair, Limit: 10})
		}},
		{"GetPastPublicTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetPastPublicTrades(t.Context(), coinPair, 10, 0)
		}},
		{"GetPastPublicTrades/legacy_cursor", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetPastPublicTrades(t.Context(), coinPair, 10, 1)
		}},
		{"GetPerpMarkets", func(t *testing.T) (any, error) { t.Helper(); return local.GetPerpMarkets(t.Context()) }},
		{"GetPremiumIndexKlineCandlesticks", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetPremiumIndexKlineCandlesticks(t.Context(), &GetPremiumIndexKlineCandlesticksRequest{Symbol: spotPair, Interval: "1m", Limit: 10})
		}},
		{"GetPremiumIndexKlineData", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetPremiumIndexKlineData(t.Context(), &GetPremiumIndexKlineDataRequest{Symbol: coinPair, Interval: "1m", Limit: 10})
		}},
		{"GetPriceChangeStats", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetPriceChangeStats(t.Context(), spotPair, nil)
		}},
		{"GetQuarterlyContractSettlementPrice", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetQuarterlyContractSettlementPrice(t.Context(), spotPair)
		}},
		{"GetSpotKline", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetSpotKline(t.Context(), &KlinesRequest{Symbol: spotPair, Interval: "1m", Limit: 10})
		}},
		{"GetSpotReferencePrice", func(t *testing.T) (any, error) { t.Helper(); return local.GetSpotReferencePrice(t.Context(), spotPair) }},
		{"GetSpotReferencePriceCalculation", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetSpotReferencePriceCalculation(t.Context(), spotPair, "")
		}},
		{"GetSystemStatus", func(t *testing.T) (any, error) { t.Helper(); return local.GetSystemStatus(t.Context()) }},
		{"GetTickerData", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetTickerData(t.Context(), currency.Pairs{spotPair, currency.NewPair(currency.ETH, currency.USDT)}, time.Hour, "FULL")
		}},
		{"GetTraderFuturesAccountRatio", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetTraderFuturesAccountRatio(t.Context(), &GetTraderFuturesAccountRatioRequest{Pair: coinUnderlying, Period: "5m", Limit: 10})
		}},
		{"GetTraderFuturesPositionsRatio", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetTraderFuturesPositionsRatio(t.Context(), &GetTraderFuturesPositionsRatioRequest{Pair: coinUnderlying, Period: "5m", Limit: 10})
		}},
		{"GetTradingDayTicker", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetTradingDayTicker(t.Context(), currency.Pairs{spotPair, currency.NewPair(currency.ETH, currency.USDT)}, "", "FULL")
		}},
		{"GetUFuturesContinuousKlineData", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUFuturesContinuousKlineData(t.Context(), &GetUFuturesContinuousKlineDataRequest{Pair: spotPair, ContractType: "PERPETUAL", Interval: "1m", Limit: 10})
		}},
		{"GetUFuturesConvertPairs", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUFuturesConvertPairs(t.Context(), currency.BTC, currency.USDT)
		}},
		{"GetUFuturesIndexConstituents", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUFuturesIndexConstituents(t.Context(), spotPair)
		}},
		{"GetUFuturesInsuranceBalance", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUFuturesInsuranceBalance(t.Context(), spotPair)
		}},
		{"GetUFuturesSymbolADLRisk", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUFuturesSymbolADLRisk(t.Context(), spotPair)
		}},
		{"GetUFuturesSymbolADLRiskAll", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUFuturesSymbolADLRisk(t.Context(), currency.EMPTYPAIR)
		}},
		{"GetExecutionRulesSymbols", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetExecutionRules(t.Context(), currency.EMPTYPAIR, currency.Pairs{currency.NewPairWithDelimiter("BTC", "USDT", "-"), currency.NewPairWithDelimiter("ETH", "USDT", "-")}, "")
		}},
		{"GetExchangeInfoSymbols", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetExchangeInfo(t.Context(), &GetExchangeInfoRequest{Symbols: currency.Pairs{currency.NewPairWithDelimiter("BTC", "USDT", "-"), currency.NewPairWithDelimiter("ETH", "USDT", "-")}})
		}},
		{"GetUFuturesTradingSchedule", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUFuturesTradingSchedule(t.Context(), spotPair)
		}},
		{"GetUIKline", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUIKline(t.Context(), &KlinesRequest{Symbol: spotPair, Interval: "1m", Limit: 10})
		}},
		{"GetURPIOrderbook", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetURPIOrderbook(t.Context(), spotPair, 1000)
		}},
		{"OpenInterest", func(t *testing.T) (any, error) { t.Helper(); return local.OpenInterest(t.Context(), coinPair) }},
		{"PortfolioMarginPing", func(t *testing.T) (any, error) { t.Helper(); return nil, local.PortfolioMarginPing(t.Context()) }},
		{"SpotPing", func(t *testing.T) (any, error) { t.Helper(); return nil, local.SpotPing(t.Context()) }},
		{"U24HTickerPriceChangeStats", func(t *testing.T) (any, error) {
			t.Helper()
			return local.U24HTickerPriceChangeStats(t.Context(), spotPair)
		}},
		{"UCompositeIndexesInfo", func(t *testing.T) (any, error) { t.Helper(); return local.UCompositeIndexesInfo(t.Context()) }},
		{"UCompressedTrades", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UCompressedTrades(t.Context(), &UCompressedTradesRequest{Symbol: spotPair, Limit: 10})
		}},
		{"UExchangeInfo", func(t *testing.T) (any, error) { t.Helper(); return local.UExchangeInfo(t.Context()) }},
		{"UFuturesOrderbook", func(t *testing.T) (any, error) { t.Helper(); return local.UFuturesOrderbook(t.Context(), spotPair, 10) }},
		{"UFuturesPing", func(t *testing.T) (any, error) { t.Helper(); return nil, local.UFuturesPing(t.Context()) }},
		{"UGetFundingHistory", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UGetFundingHistory(t.Context(), spotPair, 10, time.Time{}, time.Time{})
		}},
		{"UGetFundingRateInfo", func(t *testing.T) (any, error) { t.Helper(); return local.UGetFundingRateInfo(t.Context()) }},
		{"UGetMarkPrice", func(t *testing.T) (any, error) { t.Helper(); return local.UGetMarkPrice(t.Context(), spotPair) }},
		{"UGlobalLongShortRatio", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UGlobalLongShortRatio(t.Context(), &UGlobalLongShortRatioRequest{Symbol: spotPair, Period: "5m", Limit: 10})
		}},
		{"UKlineData", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UKlineData(t.Context(), &UKlineDataRequest{Symbol: spotPair, Interval: "1m", Limit: 10})
		}},
		{"UOpenInterest", func(t *testing.T) (any, error) { t.Helper(); return local.UOpenInterest(t.Context(), spotPair) }},
		{"UOpenInterestStats", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UOpenInterestStats(t.Context(), &UOpenInterestStatsRequest{Symbol: spotPair, Period: "5m", Limit: 10})
		}},
		{"URecentTrades", func(t *testing.T) (any, error) { t.Helper(); return local.URecentTrades(t.Context(), spotPair, "", 10) }},
		{"URecentTrades/legacy_cursor", func(t *testing.T) (any, error) {
			t.Helper()
			return local.URecentTrades(t.Context(), spotPair, "1", 10)
		}},
		{"UServerTime", func(t *testing.T) (any, error) { t.Helper(); return local.UServerTime(t.Context()) }},
		{"USymbolOrderbookTicker", func(t *testing.T) (any, error) {
			t.Helper()
			return local.USymbolOrderbookTicker(t.Context(), spotPair)
		}},
		{"USymbolPriceTicker", func(t *testing.T) (any, error) { t.Helper(); return local.USymbolPriceTicker(t.Context(), spotPair) }},
		{"UTakerBuySellVol", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UTakerBuySellVol(t.Context(), &UTakerBuySellVolRequest{Symbol: spotPair, Period: "5m", Limit: 10})
		}},
		{"UTopAccountsLongShortRatio", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UTopAccountsLongShortRatio(t.Context(), &UTopAccountsLongShortRatioRequest{Symbol: spotPair, Period: "5m", Limit: 10})
		}},
		{"UTopPositionsLongShortRatio", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UTopPositionsLongShortRatio(t.Context(), &UTopPositionsLongShortRatioRequest{Symbol: spotPair, Period: "5m", Limit: 10})
		}},
		{"GetBestPrice/multiple", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetBestPrice(t.Context(), currency.EMPTYPAIR, currency.Pairs{spotPair, currency.NewPair(currency.ETH, currency.USDT)})
		}},
		{"GetBestPrice/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetBestPrice(t.Context(), currency.EMPTYPAIR, nil)
		}},
		{"GetLatestSpotPrice/multiple", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetLatestSpotPrice(t.Context(), currency.EMPTYPAIR, currency.Pairs{spotPair, currency.NewPair(currency.ETH, currency.USDT)})
		}},
		{"GetLatestSpotPrice/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetLatestSpotPrice(t.Context(), currency.EMPTYPAIR, nil)
		}},
		{"GetPriceChangeStats/multiple", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetPriceChangeStats(t.Context(), currency.EMPTYPAIR, currency.Pairs{spotPair, currency.NewPair(currency.ETH, currency.USDT)})
		}},
		{"GetPriceChangeStats/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetPriceChangeStats(t.Context(), currency.EMPTYPAIR, nil)
		}},
		{"U24HTickerPriceChangeStats/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.U24HTickerPriceChangeStats(t.Context(), currency.EMPTYPAIR)
		}},
		{"UGetMarkPrice/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.UGetMarkPrice(t.Context(), currency.EMPTYPAIR)
		}},
		{"USymbolPriceTicker/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.USymbolPriceTicker(t.Context(), currency.EMPTYPAIR)
		}},
		{"USymbolOrderbookTicker/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.USymbolOrderbookTicker(t.Context(), currency.EMPTYPAIR)
		}},
		{"GetMultiAssetModeAssetIndex/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetMultiAssetModeAssetIndex(t.Context(), currency.EMPTYPAIR)
		}},
		{"GetUFuturesInsuranceBalance/all", func(t *testing.T) (any, error) {
			t.Helper()
			return local.GetUFuturesInsuranceBalance(t.Context(), currency.EMPTYPAIR)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call(t)
			require.NoError(t, err, "public endpoint must accept the documented request and decode its live response")
			switch value := result.(type) {
			case time.Time:
				assert.False(t, value.IsZero(), "server time should decode")
			case types.Time:
				assert.False(t, value.Time().IsZero(), "server time should decode")
			}
			if result != nil {
				switch result.(type) {
				case time.Time, types.Time:
				default:
					assertResponseFields(t, observer.body, reflect.TypeOf(result), tc.name)
				}
			}
			t.Logf("Decoded %T", result)
		})
	}
}

type publicAuditTransport struct {
	t         *testing.T
	base      http.RoundTripper
	live      bool
	body      json.RawMessage
	requests  uint64
	recording *mock.VCRMock
	mu        sync.Mutex
}

func (p *publicAuditTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet || req.Header.Get("X-MBX-APIKEY") != "" || req.URL.Query().Has("signature") {
		return nil, fmt.Errorf("public test refused an authenticated or non-GET request to %s", req.URL.Path)
	}
	response, err := p.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests++
	p.body = body
	if p.live {
		p.t.Logf("GET %s: status=%d bytes=%d sha256=%x", req.URL.String(), response.StatusCode, len(body), sha256.Sum256(body))
	}
	if p.recording != nil && response.StatusCode == http.StatusOK && json.Valid(body) {
		if p.recording.Routes[req.URL.Path] == nil {
			p.recording.Routes[req.URL.Path] = make(map[string][]mock.HTTPResponse)
		}
		records := p.recording.Routes[req.URL.Path][req.Method]
		query := req.URL.Query().Encode()
		if !slices.ContainsFunc(records, func(record mock.HTTPResponse) bool { return record.QueryString == query }) {
			p.recording.Routes[req.URL.Path][req.Method] = append(records, mock.HTTPResponse{Data: body, QueryString: query})
		}
	}
	return response, nil
}
