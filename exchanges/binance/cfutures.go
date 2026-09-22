package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// FuturesExchangeInfo stores CoinMarginedFutures, data
func (e *Exchange) FuturesExchangeInfo(ctx context.Context) (*CExchangeInfo, error) {
	var resp *CExchangeInfo
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, "/dapi/v1/exchangeInfo", cFuturesDefaultRate, &resp)
}

// GetFuturesOrderbook gets orderbook data for CoinMarginedFutures,
func (e *Exchange) GetFuturesOrderbook(ctx context.Context, symbol currency.Pair, limit uint64) (*OrderBook, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	rateBudget := cFuturesOrderbook1000Rate
	switch {
	case limit == 5, limit == 10, limit == 20, limit == 50:
		rateBudget = cFuturesOrderbook50Rate
	case limit >= 100 && limit < 500:
		rateBudget = cFuturesOrderbook100Rate
	case limit == 0, limit >= 500 && limit < 1000:
		rateBudget = cFuturesOrderbook500Rate
	}
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	params.Set("symbol", symbolValue)
	var resp *OrderBook
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/depth", params), rateBudget, &resp)
}

// GetFuturesPublicTrades gets recent public trades for CoinMarginedFutures,
func (e *Exchange) GetFuturesPublicTrades(ctx context.Context, symbol currency.Pair, limit uint64) ([]*FuturesPublicTradesData, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	var resp []*FuturesPublicTradesData
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/trades", params), cFuturesTradesRate, &resp)
}

// GetFuturesHistoricalTrades gets historical public trades for CoinMarginedFutures,
func (e *Exchange) GetFuturesHistoricalTrades(ctx context.Context, symbol currency.Pair, fromID string, limit uint64) ([]*UPublicTradesData, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if fromID != "" {
		params.Set("fromId", fromID)
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	var resp []*UPublicTradesData
	return resp, e.SendAPIKeyHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, common.EncodeURLValues("/dapi/v1/historicalTrades", params), cFuturesHistoricalTradesRate, &resp)
}

// GetPastPublicTrades gets past public trades for CoinMarginedFutures,
func (e *Exchange) GetPastPublicTrades(ctx context.Context, symbol currency.Pair, limit, fromID uint64) ([]*FuturesPublicTradesData, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	if fromID != 0 {
		params.Set("fromId", strconv.FormatUint(fromID, 10))
	}
	var resp []*FuturesPublicTradesData
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/trades", params), cFuturesTradesRate, &resp)
}

// GetFuturesAggregatedTradesList gets aggregated trades list for CoinMarginedFutures,
func (e *Exchange) GetFuturesAggregatedTradesList(ctx context.Context, arg *GetFuturesAggregatedTradesListRequest) ([]*AggregatedTrade, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, fromID, limit := arg.Symbol, arg.FromID, arg.Limit
	startTime, endTime := arg.StartTime, arg.EndTime
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	if fromID != 0 {
		params.Set("fromId", strconv.FormatUint(fromID, 10))
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	var resp []*AggregatedTrade
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/aggTrades", params), cFuturesHistoricalTradesRate, &resp)
}

// GetIndexAndMarkPrice gets index and mark prices  for CoinMarginedFutures,
func (e *Exchange) GetIndexAndMarkPrice(ctx context.Context, symbol, pair string) ([]*IndexMarkPrice, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	var resp []*IndexMarkPrice
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/premiumIndex", params), cFuturesIndexMarkPriceRate, &resp)
}

// GetFundingRateInfo retrieves funding rate info for symbols that had FundingRateCap/ FundingRateFloor / fundingIntervalHours adjustment
func (e *Exchange) GetFundingRateInfo(ctx context.Context) ([]*FundingRateInfoResponse, error) {
	var resp []*FundingRateInfoResponse
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, "/dapi/v1/fundingInfo", uFuturesDefaultRate, &resp)
}

// GetFuturesKlineData gets futures kline data for CoinMarginedFutures,
func (e *Exchange) GetFuturesKlineData(ctx context.Context, arg *GetFuturesKlineDataRequest) ([]*CFuturesCandleStick, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, interval, limit := arg.Symbol, arg.Interval, arg.Limit
	startTime, endTime := arg.StartTime, arg.EndTime
	if !slices.Contains(validFuturesIntervals, interval) {
		return nil, kline.ErrInvalidInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if !symbol.IsEmpty() {
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	params.Set("interval", interval)
	var data []*CFuturesCandleStick
	rateBudget := getKlineRateBudget(limit)
	return data, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/klines", params), rateBudget, &data)
}

// GetContinuousKlineData gets continuous kline data
func (e *Exchange) GetContinuousKlineData(ctx context.Context, arg *GetContinuousKlineDataRequest) ([]*CFuturesCandleStick, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	pair, contractType, interval := arg.Pair, arg.ContractType, arg.Interval
	limit, startTime, endTime := arg.Limit, arg.StartTime, arg.EndTime
	if pair == "" {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !slices.Contains(validContractType, contractType) {
		return nil, errContractTypeIsRequired
	}
	if !slices.Contains(validFuturesIntervals, interval) {
		return nil, kline.ErrInvalidInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	params.Set("pair", pair)
	params.Set("contractType", contractType)
	params.Set("interval", interval)
	rateBudget := getKlineRateBudget(limit)
	var data []*CFuturesCandleStick
	return data, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/continuousKlines", params), rateBudget, &data)
}

// GetIndexPriceKlines gets continuous kline data
func (e *Exchange) GetIndexPriceKlines(ctx context.Context, arg *GetIndexPriceKlinesRequest) ([]*CFuturesCandleStick, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	pair, interval, limit := arg.Pair, arg.Interval, arg.Limit
	startTime, endTime := arg.StartTime, arg.EndTime
	if !slices.Contains(validFuturesIntervals, interval) {
		return nil, kline.ErrInvalidInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	params.Set("interval", interval)
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	params.Set("pair", pair)
	rateBudget := getKlineRateBudget(limit)
	var data []*CFuturesCandleStick
	return data, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/indexPriceKlines", params), rateBudget, &data)
}

// GetMarkPriceKline gets mark price kline data
func (e *Exchange) GetMarkPriceKline(ctx context.Context, arg *GetMarkPriceKlineRequest) ([]*CFuturesCandleStick, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, interval, limit := arg.Symbol, arg.Interval, arg.Limit
	startTime, endTime := arg.StartTime, arg.EndTime
	return e.getKline(ctx, symbol, interval, "/dapi/v1/markPriceKlines", limit, startTime, endTime)
}

// GetPremiumIndexKlineData premium index kline bars of a symbol.
// Klines are uniquely identified by their open time.
func (e *Exchange) GetPremiumIndexKlineData(ctx context.Context, arg *GetPremiumIndexKlineDataRequest) ([]*CFuturesCandleStick, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, interval, limit := arg.Symbol, arg.Interval, arg.Limit
	startTime, endTime := arg.StartTime, arg.EndTime
	return e.getKline(ctx, symbol, interval, "/dapi/v1/premiumIndexKlines", limit, startTime, endTime)
}

func (e *Exchange) getKline(ctx context.Context, symbol currency.Pair, interval, path string, limit uint64, startTime, endTime time.Time) ([]*CFuturesCandleStick, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(validFuturesIntervals, interval) {
		return nil, kline.ErrInvalidInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	params.Set("interval", interval)
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	params.Set("symbol", symbolValue)
	rateBudget := getKlineRateBudget(limit)
	var data []*CFuturesCandleStick
	return data, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues(path, params), rateBudget, &data)
}

func getKlineRateBudget(limit uint64) request.EndpointLimit {
	rateBudget := cFuturesDefaultRate
	switch {
	case limit > 0 && limit < 100:
		rateBudget = cFuturesKline100Rate
	case limit >= 100 && limit < 500:
		rateBudget = cFuturesKline500Rate
	case limit >= 500 && limit < 1000:
		rateBudget = cFuturesKline1000Rate
	case limit >= 1000:
		rateBudget = cFuturesKlineMaxRate
	}
	return rateBudget
}

// GetFuturesSwapTickerChangeStats gets 24hr ticker change stats for CoinMarginedFutures,
func (e *Exchange) GetFuturesSwapTickerChangeStats(ctx context.Context, symbol currency.Pair, pair string) ([]*PriceChangeStats, error) {
	params := url.Values{}
	rateLimit := cFuturesTickerPriceHistoryRate
	if !symbol.IsEmpty() {
		rateLimit = cFuturesDefaultRate
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	var resp []*PriceChangeStats
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/ticker/24hr", params), rateLimit, &resp)
}

// FuturesGetFundingHistory gets funding history for CoinMarginedFutures,
func (e *Exchange) FuturesGetFundingHistory(ctx context.Context, symbol currency.Pair, limit int, startTime, endTime time.Time) ([]*FundingRateHistory, error) {
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !symbol.IsEmpty() {
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	var resp []*FundingRateHistory
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/fundingRate", params), cFuturesDefaultRate, &resp)
}

// GetFuturesSymbolPriceTicker gets price ticker for symbol
func (e *Exchange) GetFuturesSymbolPriceTicker(ctx context.Context, symbol currency.Pair, pair string) ([]*SymbolPriceTicker, error) {
	params := url.Values{}
	rateLimit := cFuturesOrderbookTickerAllRate
	if !symbol.IsEmpty() {
		rateLimit = cFuturesDefaultRate
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	var resp []*SymbolPriceTicker
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/ticker/price", params), rateLimit, &resp)
}

// GetFuturesOrderbookTicker gets orderbook ticker for symbol
func (e *Exchange) GetFuturesOrderbookTicker(ctx context.Context, symbol currency.Pair, pair string) ([]*SymbolOrderBookTicker, error) {
	params := url.Values{}
	rateLimit := cFuturesOrderbookTickerAllRate
	if !symbol.IsEmpty() {
		rateLimit = cFuturesDefaultRate
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	var resp []*SymbolOrderBookTicker
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/ticker/bookTicker", params), rateLimit, &resp)
}

// GetCFuturesIndexPriceConstituents retrieved index price constituents detail
func (e *Exchange) GetCFuturesIndexPriceConstituents(ctx context.Context, symbol currency.Pair) (*CFuturesIndexPriceConstituents, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	params := url.Values{}
	params.Set("symbol", symbol.String())
	var resp *CFuturesIndexPriceConstituents
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/constituents", params), cFuturesDefaultRate, &resp)
}

// OpenInterest gets open interest data for a symbol
func (e *Exchange) OpenInterest(ctx context.Context, symbol currency.Pair) (*OpenInterestData, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	var resp *OpenInterestData
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, "/dapi/v1/openInterest?symbol="+symbolValue, cFuturesDefaultRate, &resp)
}

// CFuturesQuarterlyContractSettlementPrice retrieves coin margined futures quarterly contract settlement price
func (e *Exchange) CFuturesQuarterlyContractSettlementPrice(ctx context.Context, pair currency.Pair) ([]*SettlementPrice, error) {
	if pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	var resp []*SettlementPrice
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, "/futures/data/delivery-price?pair="+pair.String(), cFuturesDefaultRate, &resp)
}

// GetOpenInterestStats gets open interest stats for a symbol
func (e *Exchange) GetOpenInterestStats(ctx context.Context, arg *GetOpenInterestStatsRequest) ([]*OpenInterestStats, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	pair, contractType, period := arg.Pair, arg.ContractType, arg.Period
	limit, startTime, endTime := arg.Limit, arg.StartTime, arg.EndTime
	if !slices.Contains(validContractType, contractType) {
		return nil, fmt.Errorf("%w: invalid interval %s", errContractTypeIsRequired, contractType)
	}
	if !slices.Contains(validFuturesIntervals, period) {
		return nil, errInvalidPeriodOrInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	params.Set("contractType", contractType)
	params.Set("period", period)
	if pair != "" {
		params.Set("pair", pair)
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	var resp []*OpenInterestStats
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/futures/data/openInterestHist", params), cFuturesDefaultRate, &resp)
}

// GetTraderFuturesAccountRatio gets a traders futures account long/short ratio
func (e *Exchange) GetTraderFuturesAccountRatio(ctx context.Context, arg *GetTraderFuturesAccountRatioRequest) ([]*TopTraderAccountRatio, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	pair, period, limit := arg.Pair, arg.Period, arg.Limit
	startTime, endTime := arg.StartTime, arg.EndTime
	if pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !slices.Contains(validFuturesIntervals, period) {
		return nil, errInvalidPeriodOrInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	params.Set("pair", pair.String())
	params.Set("period", period)
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	var resp []*TopTraderAccountRatio
	if arg.ContractType != "" {
		params.Set("contractType", arg.ContractType)
	}

	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/futures/data/topLongShortAccountRatio", params), cFuturesDefaultRate, &resp)
}

// GetTraderFuturesPositionsRatio gets a traders futures positions' long/short ratio
func (e *Exchange) GetTraderFuturesPositionsRatio(ctx context.Context, arg *GetTraderFuturesPositionsRatioRequest) ([]*TopTraderPositionRatio, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	pair, period, limit := arg.Pair, arg.Period, arg.Limit
	startTime, endTime := arg.StartTime, arg.EndTime
	if pair.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if !slices.Contains(validFuturesIntervals, period) {
		return nil, errInvalidPeriodOrInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	params.Set("pair", pair.String())
	params.Set("period", period)
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	var resp []*TopTraderPositionRatio
	if arg.ContractType != "" {
		params.Set("contractType", arg.ContractType)
	}

	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/futures/data/topLongShortPositionRatio", params), cFuturesDefaultRate, &resp)
}

// GetMarketRatio gets global long/short ratio
func (e *Exchange) GetMarketRatio(ctx context.Context, arg *GetMarketRatioRequest) ([]*TopTraderAccountRatio, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	pair, period, limit := arg.Pair, arg.Period, arg.Limit
	startTime, endTime := arg.StartTime, arg.EndTime
	if pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !slices.Contains(validFuturesIntervals, period) {
		return nil, errInvalidPeriodOrInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	params.Set("pair", pair.String())
	params.Set("period", period)
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	var resp []*TopTraderAccountRatio
	if arg.ContractType != "" {
		params.Set("contractType", arg.ContractType)
	}

	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/futures/data/globalLongShortAccountRatio", params), cFuturesDefaultRate, &resp)
}

// GetFuturesTakerVolume gets futures taker buy/sell volumes
func (e *Exchange) GetFuturesTakerVolume(ctx context.Context, arg *GetFuturesTakerVolumeRequest) ([]*TakerBuySellVolume, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	pair, contractType, period := arg.Pair, arg.ContractType, arg.Period
	limit, startTime, endTime := arg.Limit, arg.StartTime, arg.EndTime
	if pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !slices.Contains(validContractType, contractType) {
		return nil, errContractTypeIsRequired
	}
	if !slices.Contains(validFuturesIntervals, period) {
		return nil, kline.ErrInvalidInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	params.Set("pair", pair.String())
	params.Set("contractType", contractType)
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	params.Set("period", period)
	var resp []*TakerBuySellVolume
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/futures/data/takerBuySellVol", params), cFuturesDefaultRate, &resp)
}

// GetFuturesBasisData gets futures basis data
func (e *Exchange) GetFuturesBasisData(ctx context.Context, arg *GetFuturesBasisDataRequest) ([]*FuturesBasisData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	pair, contractType, period := arg.Pair, arg.ContractType, arg.Period
	limit, startTime, endTime := arg.Limit, arg.StartTime, arg.EndTime
	if pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !slices.Contains(validContractType, contractType) {
		return nil, errContractTypeIsRequired
	}
	if !slices.Contains(validFuturesIntervals, period) {
		return nil, errInvalidPeriodOrInterval
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	params.Set("pair", pair.String())
	params.Set("contractType", contractType)
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	params.Set("period", period)
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	var resp []*FuturesBasisData
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/futures/data/basis", params), cFuturesDefaultRate, &resp)
}

// FuturesNewOrder sends a new futures order to the exchange
func (e *Exchange) FuturesNewOrder(ctx context.Context, x *FuturesNewOrderRequest) (*FuturesOrderPlaceData, error) {
	if err := common.NilGuard(x); err != nil {
		return nil, err
	}
	var err error
	x.Symbol, err = e.FormatExchangeCurrency(x.Symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	if x.PositionSide != "" && !slices.Contains(validPositionSide, x.PositionSide) {
		return nil, fmt.Errorf("%w %s", errInvalidPositionSide, x.PositionSide)
	}
	if x.WorkingType != "" && !slices.Contains(validWorkingType, x.WorkingType) {
		return nil, errInvalidWorkingType
	}
	if x.NewOrderRespType != "" && !slices.Contains(validNewOrderRespType, x.NewOrderRespType) {
		return nil, errInvalidNewOrderResponseType
	}
	var resp *FuturesOrderPlaceData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPost, "/dapi/v1/order", nil, cFuturesOrdersDefaultRate, x, &resp)
}

// FuturesBatchOrder sends a batch order request
func (e *Exchange) FuturesBatchOrder(ctx context.Context, data []*PlaceBatchOrderData) ([]*FuturesOrderPlaceData, error) {
	if len(data) == 0 {
		return nil, common.ErrEmptyParams
	}
	var err error
	for x := range data {
		data[x].Symbol, err = e.FormatExchangeCurrency(data[x].Symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		if data[x].PositionSide != "" && !slices.Contains(validPositionSide, data[x].PositionSide) {
			return nil, fmt.Errorf("%w %s", errInvalidPositionSide, data[x].PositionSide)
		}
		if data[x].WorkingType != "" && !slices.Contains(validWorkingType, data[x].WorkingType) {
			return nil, errInvalidWorkingType
		}
		if data[x].NewOrderRespType != "" && !slices.Contains(validNewOrderRespType, data[x].NewOrderRespType) {
			return nil, errInvalidNewOrderResponseType
		}
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("batchOrders", string(jsonData))
	var resp []*FuturesOrderPlaceData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPost, "/dapi/v1/batchOrders", params, cFuturesBatchOrdersRate, nil, &resp)
}

// FuturesBatchCancelOrders sends a batch request to cancel orders
func (e *Exchange) FuturesBatchCancelOrders(ctx context.Context, symbol currency.Pair, orderList, origClientOrderIDList []string) ([]*BatchCancelOrderData, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	if len(orderList) != 0 {
		jsonOrderList, err := json.Marshal(orderList)
		if err != nil {
			return nil, err
		}
		params.Set("orderIdList", string(jsonOrderList))
	}
	if len(origClientOrderIDList) != 0 {
		jsonCliOrdIDList, err := json.Marshal(origClientOrderIDList)
		if err != nil {
			return nil, err
		}
		params.Set("origClientOrderIdList", string(jsonCliOrdIDList))
	}
	var resp []*BatchCancelOrderData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodDelete, "/dapi/v1/batchOrders", params, cFuturesOrdersDefaultRate, nil, &resp)
}

// FuturesGetOrderData gets futures order data
func (e *Exchange) FuturesGetOrderData(ctx context.Context, symbol currency.Pair, orderID, origClientOrderID string) (*FuturesOrderGetData, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp *FuturesOrderGetData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/order", params, cFuturesOrdersDefaultRate, nil, &resp)
}

// FuturesCancelOrder cancels a futures order
func (e *Exchange) FuturesCancelOrder(ctx context.Context, symbol currency.Pair, orderID, origClientOrderID string) (*FuturesOrderGetData, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp *FuturesOrderGetData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodDelete, "/dapi/v1/order", params, cFuturesOrdersDefaultRate, nil, &resp)
}

// FuturesCancelAllOpenOrders cancels a futures order
func (e *Exchange) FuturesCancelAllOpenOrders(ctx context.Context, symbol currency.Pair) (*GenericAuthResponse, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	var resp *GenericAuthResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodDelete, "/dapi/v1/allOpenOrders", params, cFuturesOrdersDefaultRate, nil, &resp)
}

// AutoCancelAllOpenOrders cancels all open futures orders
// countdownTime 1000 = 1s, example - to cancel all orders after 30s (countdownTime: 30000)
func (e *Exchange) AutoCancelAllOpenOrders(ctx context.Context, symbol currency.Pair, countdownTime int64) (*AutoCancelAllOrdersData, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	params.Set("countdownTime", strconv.FormatInt(countdownTime, 10))
	var resp *AutoCancelAllOrdersData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPost, "/dapi/v1/countdownCancelAll", params, cFuturesCancelAllOrdersRate, nil, &resp)
}

// FuturesOpenOrderData gets open order data for CoinMarginedFutures
func (e *Exchange) FuturesOpenOrderData(ctx context.Context, symbol currency.Pair, orderID, origClientOrderID string) (*FuturesOrderGetData, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	if orderID != "" {
		params.Set("orderId", orderID)
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	var resp *FuturesOrderGetData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/openOrder", params, cFuturesOrdersDefaultRate, nil, &resp)
}

// GetFuturesAllOpenOrders gets all open orders data for CoinMarginedFutures,
func (e *Exchange) GetFuturesAllOpenOrders(ctx context.Context, symbol currency.Pair, pair string) ([]*FuturesOrderData, error) {
	var (
		p   string
		err error
	)
	rateLimit := cFuturesGetAllOpenOrdersRate
	params := url.Values{}
	if !symbol.IsEmpty() {
		rateLimit = cFuturesOrdersDefaultRate
		p, err = e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", p)
	} else {
		// extend the receive window when all currencies to prevent "recvwindow" error
		params.Set("recvWindow", "10000")
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	var resp []*FuturesOrderData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/openOrders", params, rateLimit, nil, &resp)
}

// GetAllFuturesOrders gets all orders active cancelled or filled
func (e *Exchange) GetAllFuturesOrders(ctx context.Context, arg *GetAllFuturesOrdersRequest) ([]*FuturesOrderData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, pair, startTime := arg.Symbol, arg.Pair, arg.StartTime
	endTime, orderID, limit := arg.EndTime, arg.OrderID, arg.Limit
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
		if endTime.Sub(startTime) >= 7*24*time.Hour {
			return nil, fmt.Errorf("%w: futures history queries must span less than 7 days", errOrderHistoryWindowExceeded)
		}
	}
	if symbol.IsEmpty() && pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !pair.IsEmpty() && orderID != 0 {
		return nil, fmt.Errorf("%w: pair cannot be combined with orderId", errInvalidOrderQueryCombination)
	}
	params := url.Values{}
	rateLimit := cFuturesPairOrdersRate
	if !symbol.IsEmpty() {
		rateLimit = cFuturesSymbolOrdersRate
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if !pair.IsEmpty() {
		params.Set("pair", pair.String())
	}
	if orderID != 0 {
		params.Set("orderId", strconv.FormatUint(orderID, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatUint(limit, 10))
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	var resp []*FuturesOrderData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/allOrders", params, rateLimit, nil, &resp)
}

// GetFuturesAccountBalance gets account balance data for CoinMarginedFutures, account
func (e *Exchange) GetFuturesAccountBalance(ctx context.Context) ([]*FuturesAccountBalanceData, error) {
	var resp []*FuturesAccountBalanceData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/balance", nil, cFuturesDefaultRate, nil, &resp)
}

// GetFuturesAccountInfo gets account info data for CoinMarginedFutures, account
func (e *Exchange) GetFuturesAccountInfo(ctx context.Context) (*FuturesAccountInformation, error) {
	var resp *FuturesAccountInformation
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/account", nil, cFuturesAccountInformationRate, nil, &resp)
}

// FuturesChangeInitialLeverage changes initial leverage for the account
func (e *Exchange) FuturesChangeInitialLeverage(ctx context.Context, symbol currency.Pair, leverage float64) (*FuturesLeverageData, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	if leverage < 1 || leverage > 125 {
		return nil, fmt.Errorf("%w: leverage value should range between 1 and 125", order.ErrSubmitLeverageNotSupported)
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	params.Set("leverage", strconv.FormatFloat(leverage, 'f', -1, 64))
	var resp *FuturesLeverageData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPost, "/dapi/v1/leverage", params, cFuturesDefaultRate, nil, &resp)
}

// FuturesChangeMarginType changes margin type
func (e *Exchange) FuturesChangeMarginType(ctx context.Context, symbol currency.Pair, marginType string) (*GenericAuthResponse, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(validMarginType, marginType) {
		return nil, margin.ErrInvalidMarginType
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	params.Set("marginType", marginType)
	var resp *GenericAuthResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPost, "/dapi/v1/marginType", params, cFuturesDefaultRate, nil, &resp)
}

// ModifyIsolatedPositionMargin changes margin for an isolated position
func (e *Exchange) ModifyIsolatedPositionMargin(ctx context.Context, symbol currency.Pair, positionSide, changeType string, amount float64) (*FuturesMarginUpdatedResponse, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	if changeType != "" {
		cType, ok := validMarginChange[changeType]
		if !ok {
			return nil, fmt.Errorf("%w: possible position margin are 1: add position margin 2: reduce position margin", errMarginChangeTypeInvalid)
		}
		params.Set("type", strconv.FormatInt(cType, 10))
	}
	if positionSide != "" {
		if !slices.Contains(validPositionSide, positionSide) {
			return nil, errInvalidPositionSide
		}
		params.Set("positionSide", positionSide)
	}
	params.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	var resp *FuturesMarginUpdatedResponse
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodPost, "/dapi/v1/positionMargin", params, cFuturesDefaultRate, nil, &resp)
}

// FuturesMarginChangeHistory gets past margin changes for positions
func (e *Exchange) FuturesMarginChangeHistory(ctx context.Context, arg *FuturesMarginChangeHistoryRequest) ([]*GetPositionMarginChangeHistoryData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, changeType, startTime := arg.Symbol, arg.ChangeType, arg.StartTime
	endTime, limit := arg.EndTime, arg.Limit
	symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	cType, ok := validMarginChange[changeType]
	if !ok {
		return nil, errMarginChangeTypeInvalid
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	params.Set("symbol", symbolValue)
	params.Set("type", strconv.FormatInt(cType, 10))
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit != 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*GetPositionMarginChangeHistoryData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/positionMargin/history", params, cFuturesDefaultRate, nil, &resp)
}

// FuturesPositionsInfo gets futures positions info
// "pair" for coinmarginedfutures in GCT terms is the pair base
// eg ADAUSD_PERP the "pair" parameter is ADAUSD
func (e *Exchange) FuturesPositionsInfo(ctx context.Context, marginAsset, pair string) ([]*FuturesPositionInformation, error) {
	params := url.Values{}
	if marginAsset != "" {
		params.Set("marginAsset", marginAsset)
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	var resp []*FuturesPositionInformation
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/positionRisk", params, cFuturesDefaultRate, nil, &resp)
}

// FuturesTradeHistory gets trade history for CoinMarginedFutures, account
func (e *Exchange) FuturesTradeHistory(ctx context.Context, arg *FuturesTradeHistoryRequest) ([]*FuturesAccountTradeList, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, pair, startTime := arg.Symbol, arg.Pair, arg.StartTime
	endTime, limit, fromID := arg.EndTime, arg.Limit, arg.FromID
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	rateLimit := cFuturesPairOrdersRate
	if !symbol.IsEmpty() {
		rateLimit = cFuturesSymbolOrdersRate
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if pair != "" {
		params.Set("pair", pair)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit != 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	if fromID != 0 {
		params.Set("fromId", strconv.FormatInt(fromID, 10))
	}
	var resp []*FuturesAccountTradeList
	if arg.OrderID != "" {
		params.Set("orderId", arg.OrderID)
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/userTrades", params, rateLimit, nil, &resp)
}

// FuturesIncomeHistory gets income history for CoinMarginedFutures,
func (e *Exchange) FuturesIncomeHistory(ctx context.Context, arg *FuturesIncomeHistoryRequest) ([]*FuturesIncomeHistoryData, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, incomeType, startTime := arg.Symbol, arg.IncomeType, arg.StartTime
	endTime, limit := arg.EndTime, arg.Limit
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !symbol.IsEmpty() {
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if incomeType != "" {
		if !slices.Contains(validIncomeType, incomeType) {
			return nil, fmt.Errorf("invalid incomeType: %v", incomeType)
		}
		params.Set("incomeType", incomeType)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit != 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*FuturesIncomeHistoryData
	if arg.Page != 0 {
		params.Set("page", strconv.FormatUint(arg.Page, 10))
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/income", params, cFuturesIncomeHistoryRate, nil, &resp)
}

// FuturesForceOrders gets futures forced orders
func (e *Exchange) FuturesForceOrders(ctx context.Context, symbol currency.Pair, autoCloseType string, startTime, endTime time.Time, options ...*FuturesForceOrdersRequest) ([]*ForcedOrdersData, error) {
	option := new(FuturesForceOrdersRequest)
	if len(options) != 0 && options[0] != nil {
		option = options[0]
	}

	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	rateLimit := cFuturesAllForceOrdersRate
	if !symbol.IsEmpty() {
		rateLimit = cFuturesCurrencyForceOrdersRate
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	if autoCloseType != "" {
		if !slices.Contains(validAutoCloseTypes, autoCloseType) {
			return nil, errInvalidAutoCloseType
		}
		params.Set("autoCloseType", autoCloseType)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	var resp []*ForcedOrdersData
	if option.Limit != 0 {
		params.Set("limit", strconv.FormatUint(option.Limit, 10))
	}

	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/forceOrders", params, rateLimit, nil, &resp)
}

// FuturesPositionsADLEstimate estimates ADL on positions
func (e *Exchange) FuturesPositionsADLEstimate(ctx context.Context, symbol currency.Pair) ([]*ADLEstimateData, error) {
	params := url.Values{}
	if !symbol.IsEmpty() {
		symbolValue, err := e.FormatSymbol(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolValue)
	}
	var resp []*ADLEstimateData
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/adlQuantile", params, cFuturesAccountInformationRate, nil, &resp)
}

// FetchCoinMarginExchangeLimits fetches coin margined order execution limits
func (e *Exchange) FetchCoinMarginExchangeLimits(ctx context.Context) ([]limits.MinMaxLevel, error) {
	coinFutures, err := e.FuturesExchangeInfo(ctx)
	if err != nil {
		return nil, err
	}

	l := make([]limits.MinMaxLevel, 0, len(coinFutures.Symbols))
	for x := range coinFutures.Symbols {
		sym := coinFutures.Symbols[x]
		var cp currency.Pair
		cp, err = currency.NewPairFromString(sym.Symbol)
		if err != nil {
			return nil, err
		}
		mml := limits.MinMaxLevel{
			Key: key.NewExchangeAssetPair(e.Name, asset.CoinMarginedFutures, cp),
		}
		applyFuturesFilters(&mml, sym.Filters)
		l = append(l, mml)
	}
	return l, nil
}

// CFuturesPing tests connectivity to the COIN-M futures REST API.
func (e *Exchange) CFuturesPing(ctx context.Context) error {
	return e.SendHTTPRequest(ctx, exchange.RestCoinMargined, "/dapi/v1/ping", cFuturesDefaultRate, &struct{}{})
}

// CFuturesServerTime returns the COIN-M futures server time.
func (e *Exchange) CFuturesServerTime(ctx context.Context) (time.Time, error) {
	var resp struct {
		ServerTime types.Time `json:"serverTime"`
	}
	if err := e.SendHTTPRequest(ctx, exchange.RestCoinMargined, "/dapi/v1/time", cFuturesDefaultRate, &resp); err != nil {
		return time.Time{}, err
	}
	return resp.ServerTime.Time(), nil
}

// GetCFuturesOpenInterest returns the open interest for a COIN-M contract.
func (e *Exchange) GetCFuturesOpenInterest(ctx context.Context, symbol currency.Pair) (*CFuturesOpenInterest, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	symbolFmt, err := e.FormatExchangeCurrency(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolFmt.String())
	var resp *CFuturesOpenInterest
	return resp, e.SendHTTPRequest(ctx, exchange.RestCoinMargined, common.EncodeURLValues("/dapi/v1/openInterest", params), cFuturesDefaultRate, &resp)
}

// GetCFuturesCommissionRate returns the maker and taker commission rates for a symbol.
func (e *Exchange) GetCFuturesCommissionRate(ctx context.Context, symbol currency.Pair) (*CFuturesCommissionRate, error) {
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	symbolFmt, err := e.FormatExchangeCurrency(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolFmt.String())
	var resp *CFuturesCommissionRate
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/commissionRate", params, cFuturesCommissionRateRate, nil, &resp)
}

// GetCFuturesPositionSideDual reports whether hedge mode is enabled for COIN-M futures.
func (e *Exchange) GetCFuturesPositionSideDual(ctx context.Context) (*CFuturesPositionSideDual, error) {
	var resp *CFuturesPositionSideDual
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/positionSide/dual", nil, cFuturesPositionSideDualRate, nil, &resp)
}

// GetCFuturesLeverageBracket returns the V2 leverage brackets, which add notionalCoef over V1.
// Omitting the symbol returns every bracket and costs twice the weight.
func (e *Exchange) GetCFuturesLeverageBracket(ctx context.Context, symbol currency.Pair) ([]*CFuturesLeverageBracketV2, error) {
	params := url.Values{}
	rateLimit := cFuturesLeverageBracketAllRate
	if !symbol.IsEmpty() {
		symbolFmt, err := e.FormatExchangeCurrency(symbol, asset.CoinMarginedFutures)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbolFmt.String())
		rateLimit = cFuturesDefaultRate
	}
	var resp []*CFuturesLeverageBracketV2
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v2/leverageBracket", params, rateLimit, nil, &resp)
}

// GetCFuturesTransactionHistoryDownloadID requests an async download of COIN-M transaction history.
func (e *Exchange) GetCFuturesTransactionHistoryDownloadID(ctx context.Context, startTime, endTime time.Time) (*UTransactionDownloadID, error) {
	return e.cFuturesDownloadID(ctx, "/dapi/v1/income/asyn", startTime, endTime)
}

// GetCFuturesOrderHistoryDownloadID requests an async download of COIN-M order history.
func (e *Exchange) GetCFuturesOrderHistoryDownloadID(ctx context.Context, startTime, endTime time.Time) (*UTransactionDownloadID, error) {
	return e.cFuturesDownloadID(ctx, "/dapi/v1/order/asyn", startTime, endTime)
}

// GetCFuturesTradeHistoryDownloadID requests an async download of COIN-M trade history.
func (e *Exchange) GetCFuturesTradeHistoryDownloadID(ctx context.Context, startTime, endTime time.Time) (*UTransactionDownloadID, error) {
	return e.cFuturesDownloadID(ctx, "/dapi/v1/trade/asyn", startTime, endTime)
}

func (e *Exchange) cFuturesDownloadID(ctx context.Context, path string, startTime, endTime time.Time) (*UTransactionDownloadID, error) {
	if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	var resp *UTransactionDownloadID
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, path, params, cFuturesDefaultRate, nil, &resp)
}

// GetCFuturesTransactionHistoryDownloadLink resolves a COIN-M transaction history download ID to a link.
func (e *Exchange) GetCFuturesTransactionHistoryDownloadLink(ctx context.Context, downloadID string) (*UTransactionHistoryDownloadLink, error) {
	return e.cFuturesDownloadLinkByID(ctx, downloadID, "/dapi/v1/income/asyn/id")
}

// GetCFuturesOrderHistoryDownloadLink resolves a COIN-M order history download ID to a link.
func (e *Exchange) GetCFuturesOrderHistoryDownloadLink(ctx context.Context, downloadID string) (*UTransactionHistoryDownloadLink, error) {
	return e.cFuturesDownloadLinkByID(ctx, downloadID, "/dapi/v1/order/asyn/id")
}

// GetCFuturesTradeHistoryDownloadLink resolves a COIN-M trade history download ID to a link.
func (e *Exchange) GetCFuturesTradeHistoryDownloadLink(ctx context.Context, downloadID string) (*UTransactionHistoryDownloadLink, error) {
	return e.cFuturesDownloadLinkByID(ctx, downloadID, "/dapi/v1/trade/asyn/id")
}

func (e *Exchange) cFuturesDownloadLinkByID(ctx context.Context, downloadID, path string) (*UTransactionHistoryDownloadLink, error) {
	if downloadID == "" {
		return nil, errDownloadIDRequired
	}
	var resp *UTransactionHistoryDownloadLink
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, path, url.Values{"downloadId": {downloadID}}, cFuturesDefaultRate, nil, &resp)
}

// GetCFuturesOrderModifyHistory returns the amendment history of COIN-M orders.
func (e *Exchange) GetCFuturesOrderModifyHistory(ctx context.Context, arg *GetCFuturesOrderModifyHistoryRequest) ([]*USDTAmendInfo, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	symbol, orderID, origClientOrderID := arg.Symbol, arg.OrderID, arg.OrigClientOrderID
	startTime, endTime, limit := arg.StartTime, arg.EndTime, arg.Limit
	if symbol.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if !startTime.IsZero() && !endTime.IsZero() {
		if err := common.StartEndTimeCheck(startTime, endTime); err != nil {
			return nil, err
		}
	}
	symbolFmt, err := e.FormatExchangeCurrency(symbol, asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", symbolFmt.String())
	if orderID > 0 {
		params.Set("orderId", strconv.FormatUint(orderID, 10))
	}
	if origClientOrderID != "" {
		params.Set("origClientOrderId", origClientOrderID)
	}
	if !startTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if !endTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.FormatInt(limit, 10))
	}
	var resp []*USDTAmendInfo
	return resp, e.SendAuthHTTPRequest(ctx, exchange.RestCoinMargined, http.MethodGet, "/dapi/v1/orderAmendment", params, cFuturesDefaultRate, nil, &resp)
}
