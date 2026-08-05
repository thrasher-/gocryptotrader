package kraken

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

const invalidSpotValue = "invalid"

func TestSpotPublicEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()
	preEpoch := time.Unix(-1, 0)

	_, err := ex.GetAssets(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetAssets must reject a nil request")
	_, err = ex.GetAssets(ctx, &GetAssetsRequest{AssetClass: invalidSpotValue})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetAssets must reject an invalid asset class")
	_, err = ex.GetAssets(ctx, &GetAssetsRequest{AssetVersion: 2})
	require.ErrorIs(t, err, errAssetVersionInvalid, "GetAssets must reject an unsupported asset version")
	assets, err := ex.GetAssets(ctx, &GetAssetsRequest{Assets: currency.Currencies{currency.BTC, currency.USD}, AssetClass: AssetClassCurrency, AssetVersion: AssetVersionDisplay})
	require.NoError(t, err, "GetAssets must not error")
	assert.Equal(t, "currency", assets["BTC"].AssetClass, "GetAssets should decode asset class")
	assert.Equal(t, 1.0, assets["BTC"].CollateralValue.Float64(), "GetAssets should decode collateral value")
	assert.Equal(t, 0.02, assets["BTC"].MarginRate.Float64(), "GetAssets should decode margin rate")
	assert.Equal(t, "enabled", assets["BTC"].Status, "GetAssets should decode status")
	values := requireSpotRequest(t, requests, "/0/public/Assets")
	assert.Equal(t, "BTC,USD", values.Get("asset"), "GetAssets should encode assets")
	assert.Equal(t, "currency", values.Get("aclass"), "GetAssets should encode asset class")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetAssets should encode assetVersion")
	_, err = ex.GetAssets(ctx, &GetAssetsRequest{})
	require.NoError(t, err, "GetAssets must allow omitted filters")
	values = requireSpotRequest(t, requests, "/0/public/Assets")
	assert.Empty(t, values, "GetAssets should omit unset filters")

	_, err = ex.GetAssetPairs(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetAssetPairs must reject a nil request")
	_, err = ex.GetAssetPairs(ctx, &GetAssetPairsRequest{AssetClassBase: invalidSpotValue})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetAssetPairs must reject an invalid asset class")
	_, err = ex.GetAssetPairs(ctx, &GetAssetPairsRequest{Info: invalidSpotValue})
	require.ErrorIs(t, err, errInfoInvalid, "GetAssetPairs must reject an invalid info filter")
	_, err = ex.GetAssetPairs(ctx, &GetAssetPairsRequest{ExecutionVenue: invalidSpotValue})
	require.ErrorIs(t, err, errExecutionVenueInvalid, "GetAssetPairs must reject an invalid execution venue")
	_, err = ex.GetAssetPairs(ctx, &GetAssetPairsRequest{AssetVersion: 2})
	require.ErrorIs(t, err, errAssetVersionInvalid, "GetAssetPairs must reject an unsupported asset version")
	_, err = ex.GetAssetPairs(ctx, &GetAssetPairsRequest{Pairs: currency.Pairs{{Base: currency.BTC}}})
	require.ErrorIs(t, err, errPairRequired, "GetAssetPairs must reject a partially populated pair")
	_, err = new(Exchange).GetAssetPairs(ctx, &GetAssetPairsRequest{Pairs: currency.Pairs{spotTestPair}})
	require.Error(t, err, "GetAssetPairs must surface pair-format errors")
	pairs, err := ex.GetAssetPairs(ctx, &GetAssetPairsRequest{
		Pairs:          currency.Pairs{spotTestPair},
		AssetClassBase: AssetClassTokenizedAsset,
		Info:           AssetPairInfoMargin,
		CountryCode:    "GB",
		ExecutionVenue: ExecutionVenueBitnomial,
		AssetVersion:   AssetVersionDisplay,
	})
	require.NoError(t, err, "GetAssetPairs must not error")
	assert.Equal(t, 5, pairs["BTC/USD"].CostDecimals, "GetAssetPairs should decode cost decimals")
	assert.Equal(t, 0.5, pairs["BTC/USD"].CostMinimum.Float64(), "GetAssetPairs should decode cost minimum")
	assert.Equal(t, 250.0, pairs["BTC/USD"].LongPositionLimit.Float64(), "GetAssetPairs should decode long position limit")
	assert.Equal(t, 200.0, pairs["BTC/USD"].ShortPositionLimit.Float64(), "GetAssetPairs should decode short position limit")
	assert.Equal(t, "international", pairs["BTC/USD"].ExecutionVenue, "GetAssetPairs should decode execution venue")
	values = requireSpotRequest(t, requests, "/0/public/AssetPairs")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "GetAssetPairs should encode pairs")
	assert.Equal(t, "tokenized_asset", values.Get("aclass_base"), "GetAssetPairs should encode base asset class")
	assert.Equal(t, "margin", values.Get("info"), "GetAssetPairs should encode info")
	assert.Equal(t, "GB", values.Get("country_code"), "GetAssetPairs should encode country code")
	assert.Equal(t, "bitnomial_exchange", values.Get("execution_venue"), "GetAssetPairs should encode execution venue")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetAssetPairs should encode assetVersion")
	_, err = ex.GetAssetPairs(ctx, &GetAssetPairsRequest{})
	require.NoError(t, err, "GetAssetPairs must allow omitted filters")
	values = requireSpotRequest(t, requests, "/0/public/AssetPairs")
	assert.Empty(t, values, "GetAssetPairs should omit unset filters")

	_, err = ex.GetTicker(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetTicker must reject a nil request")
	_, err = ex.GetTicker(ctx, &GetTickerRequest{AssetClass: "currency"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetTicker must reject an invalid asset class")
	_, err = ex.GetTicker(ctx, &GetTickerRequest{AssetVersion: 2})
	require.ErrorIs(t, err, errAssetVersionInvalid, "GetTicker must reject an unsupported asset version")
	_, err = ex.GetTicker(ctx, &GetTickerRequest{Pairs: currency.Pairs{{Base: currency.BTC}}})
	require.ErrorIs(t, err, errPairRequired, "GetTicker must reject a partially populated pair")
	_, err = new(Exchange).GetTicker(ctx, &GetTickerRequest{Pairs: currency.Pairs{spotTestPair}})
	require.Error(t, err, "GetTicker must surface pair-format errors")
	tickers, err := ex.GetTicker(ctx, &GetTickerRequest{Pairs: currency.Pairs{spotTestPair}, AssetClass: AssetClassForex, AssetVersion: AssetVersionDisplay})
	require.NoError(t, err, "GetTicker must not error")
	assert.Equal(t, 100.0, tickers["BTC/USD"].Last[0].Float64(), "GetTicker should decode last price")
	values = requireSpotRequest(t, requests, "/0/public/Ticker")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "GetTicker should encode pairs")
	assert.Equal(t, "forex", values.Get("asset_class"), "GetTicker should encode asset class")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetTicker should encode assetVersion")
	_, err = ex.GetTicker(ctx, &GetTickerRequest{})
	require.NoError(t, err, "GetTicker must allow omitted filters")
	requireSpotRequest(t, requests, "/0/public/Ticker")

	_, err = ex.GetOHLC(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetOHLC must reject a nil request")
	_, err = ex.GetOHLC(ctx, &GetOHLCRequest{})
	require.ErrorIs(t, err, errPairRequired, "GetOHLC must require a pair")
	_, err = ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair, Interval: 2 * time.Minute})
	require.ErrorIs(t, err, errIntervalInvalid, "GetOHLC must reject an invalid interval")
	_, err = ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair, AssetClass: AssetClassCurrency})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetOHLC must reject an invalid asset class")
	_, err = ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair, AssetVersion: 2})
	require.ErrorIs(t, err, errAssetVersionInvalid, "GetOHLC must reject an unsupported asset version")
	_, err = new(Exchange).GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair})
	require.Error(t, err, "GetOHLC must surface pair-format errors")
	_, err = ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair, Since: preEpoch})
	require.ErrorIs(t, err, errTimestampInvalid, "GetOHLC must reject a pre-epoch timestamp")
	since := time.Unix(1695828270, 0)
	ohlc, err := ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair, Interval: time.Hour, Since: since, AssetClass: AssetClassTokenizedAsset, AssetVersion: AssetVersionDisplay})
	require.NoError(t, err, "GetOHLC must not error")
	assert.Equal(t, uint64(3), ohlc.Candles["BTC/USD"][0].Count, "GetOHLC should decode candle count")
	values = requireSpotRequest(t, requests, "/0/public/OHLC")
	assert.Equal(t, "60", values.Get("interval"), "GetOHLC should encode interval")
	assert.Equal(t, "1695828270", values.Get("since"), "GetOHLC should encode since")
	assert.Equal(t, "tokenized_asset", values.Get("asset_class"), "GetOHLC should encode asset class")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetOHLC should encode assetVersion")
	_, err = ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetOHLC must allow optional parameters to be omitted")
	values = requireSpotRequest(t, requests, "/0/public/OHLC")
	assert.Empty(t, values.Get("interval"), "GetOHLC should omit the default interval")
	since = time.Unix(0, 0)
	_, err = ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair, Since: since})
	require.NoError(t, err, "GetOHLC must accept an explicit zero since value")
	values = requireSpotRequest(t, requests, "/0/public/OHLC")
	assert.Equal(t, "0", values.Get("since"), "GetOHLC should encode an explicit zero since value")

	_, err = ex.GetDepth(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetDepth must reject a nil request")
	_, err = ex.GetDepth(ctx, &GetDepthRequest{})
	require.ErrorIs(t, err, errPairRequired, "GetDepth must require a pair")
	_, err = ex.GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair, Count: 501})
	require.ErrorIs(t, err, errDepthCountInvalid, "GetDepth must reject a count above five hundred")
	_, err = ex.GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair, AssetClass: AssetClassCurrency})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetDepth must reject an invalid asset class")
	_, err = ex.GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair, AssetVersion: 2})
	require.ErrorIs(t, err, errAssetVersionInvalid, "GetDepth must reject an unsupported asset version")
	_, err = new(Exchange).GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair})
	require.Error(t, err, "GetDepth must surface pair-format errors")
	book, err := ex.GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair, Count: 500, AssetClass: AssetClassTokenizedAsset, AssetVersion: AssetVersionDisplay})
	require.NoError(t, err, "GetDepth must not error")
	assert.Equal(t, 2.0, book["BTC/USD"].Bids[0].Quantity.Float64(), "GetDepth should decode bid quantity")
	values = requireSpotRequest(t, requests, "/0/public/Depth")
	assert.Equal(t, "500", values.Get("count"), "GetDepth should encode count")
	assert.Equal(t, "tokenized_asset", values.Get("asset_class"), "GetDepth should encode asset class")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetDepth should encode assetVersion")
	_, err = ex.GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetDepth must allow count to be omitted")
	values = requireSpotRequest(t, requests, "/0/public/Depth")
	assert.Empty(t, values.Get("count"), "GetDepth should omit the default count")

	_, err = ex.GetTrades(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetTrades must reject a nil request")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{})
	require.ErrorIs(t, err, errPairRequired, "GetTrades must require a pair")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair, Count: 1001})
	require.ErrorIs(t, err, errTradeCountInvalid, "GetTrades must reject a count above one thousand")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair, AssetClass: AssetClassCurrency})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetTrades must reject an invalid asset class")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair, AssetVersion: 2})
	require.ErrorIs(t, err, errAssetVersionInvalid, "GetTrades must reject an unsupported asset version")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair, Since: time.Unix(1, 0), Cursor: "CURSOR"})
	require.ErrorIs(t, err, errSinceCursorConflict, "GetTrades must reject conflicting pagination values")
	_, err = new(Exchange).GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair})
	require.Error(t, err, "GetTrades must surface pair-format errors")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair, Since: preEpoch})
	require.ErrorIs(t, err, errTimestampInvalid, "GetTrades must reject a pre-epoch timestamp")
	since = time.Unix(1695828270, 0)
	trades, err := ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair, Since: since, Count: 1000, AssetClass: AssetClassTokenizedAsset, AssetVersion: AssetVersionDisplay})
	require.NoError(t, err, "GetTrades must not error")
	assert.Equal(t, 100.0, trades.Trades["BTC/USD"][0].Price.Float64(), "GetTrades should decode trade price")
	values = requireSpotRequest(t, requests, "/0/public/Trades")
	assert.Equal(t, "1695828270", values.Get("since"), "GetTrades should encode since")
	assert.Equal(t, "1000", values.Get("count"), "GetTrades should encode count")
	assert.Equal(t, "tokenized_asset", values.Get("asset_class"), "GetTrades should encode asset class")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetTrades should encode assetVersion")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetTrades must allow optional parameters to be omitted")
	values = requireSpotRequest(t, requests, "/0/public/Trades")
	assert.Empty(t, values.Get("count"), "GetTrades should omit the default count")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair, Cursor: "CURSOR"})
	require.NoError(t, err, "GetTrades must accept an opaque cursor")
	values = requireSpotRequest(t, requests, "/0/public/Trades")
	assert.Equal(t, "CURSOR", values.Get("since"), "GetTrades should encode an opaque cursor")

	_, err = ex.GetSpread(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetSpread must reject a nil request")
	_, err = ex.GetSpread(ctx, &GetSpreadRequest{})
	require.ErrorIs(t, err, errPairRequired, "GetSpread must require a pair")
	_, err = ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair, AssetClass: AssetClassCurrency})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetSpread must reject an invalid asset class")
	_, err = ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair, AssetVersion: 2})
	require.ErrorIs(t, err, errAssetVersionInvalid, "GetSpread must reject an unsupported asset version")
	_, err = new(Exchange).GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair})
	require.Error(t, err, "GetSpread must surface pair-format errors")
	_, err = ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair, Since: preEpoch})
	require.ErrorIs(t, err, errTimestampInvalid, "GetSpread must reject a pre-epoch timestamp")
	since = time.Unix(1695828270, 0)
	spread, err := ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair, Since: since, AssetClass: AssetClassTokenizedAsset, AssetVersion: AssetVersionDisplay})
	require.NoError(t, err, "GetSpread must not error")
	assert.Equal(t, 99.0, spread.Spreads["BTC/USD"][0].Bid.Float64(), "GetSpread should decode bid price")
	values = requireSpotRequest(t, requests, "/0/public/Spread")
	assert.Equal(t, "1695828270", values.Get("since"), "GetSpread should encode since")
	assert.Equal(t, "tokenized_asset", values.Get("asset_class"), "GetSpread should encode asset class")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetSpread should encode assetVersion")
	_, err = ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetSpread must allow optional parameters to be omitted")
	values = requireSpotRequest(t, requests, "/0/public/Spread")
	assert.Empty(t, values.Get("since"), "GetSpread should omit an unset since value")
	since = time.Unix(0, 0)
	_, err = ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair, Since: since})
	require.NoError(t, err, "GetSpread must accept an explicit zero since value")
	values = requireSpotRequest(t, requests, "/0/public/Spread")
	assert.Equal(t, "0", values.Get("since"), "GetSpread should encode an explicit zero since value")
}

func TestSpotPublicEnums(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)

	for _, value := range []AssetClass{AssetClassCurrency, AssetClassTokenizedAsset} {
		t.Run("asset class "+string(value), func(t *testing.T) {
			_, err := ex.GetAssets(t.Context(), &GetAssetsRequest{AssetClass: value})
			require.NoError(t, err, "GetAssets must accept the documented asset class")
			requireSpotRequest(t, requests, "/0/public/Assets")
		})
	}
	for _, value := range []AssetPairInfo{AssetPairInfoAll, AssetPairInfoLeverage, AssetPairInfoFees, AssetPairInfoMargin} {
		t.Run("asset pair info "+string(value), func(t *testing.T) {
			_, err := ex.GetAssetPairs(t.Context(), &GetAssetPairsRequest{Info: value})
			require.NoError(t, err, "GetAssetPairs must accept the documented info filter")
			requireSpotRequest(t, requests, "/0/public/AssetPairs")
		})
	}
	for _, value := range []AssetClass{AssetClassCurrency, AssetClassTokenizedAsset} {
		t.Run("asset pair base class "+string(value), func(t *testing.T) {
			_, err := ex.GetAssetPairs(t.Context(), &GetAssetPairsRequest{AssetClassBase: value})
			require.NoError(t, err, "GetAssetPairs must accept the documented base asset class")
			requireSpotRequest(t, requests, "/0/public/AssetPairs")
		})
	}
	for _, value := range []ExecutionVenue{ExecutionVenueInternational, ExecutionVenueBitnomial} {
		t.Run("execution venue "+string(value), func(t *testing.T) {
			_, err := ex.GetAssetPairs(t.Context(), &GetAssetPairsRequest{ExecutionVenue: value})
			require.NoError(t, err, "GetAssetPairs must accept the documented execution venue")
			requireSpotRequest(t, requests, "/0/public/AssetPairs")
		})
	}
	for _, value := range []AssetClass{AssetClassTokenizedAsset, AssetClassForex} {
		t.Run("ticker asset class "+string(value), func(t *testing.T) {
			_, err := ex.GetTicker(t.Context(), &GetTickerRequest{AssetClass: value})
			require.NoError(t, err, "GetTicker must accept the documented asset class")
			requireSpotRequest(t, requests, "/0/public/Ticker")
		})
	}
	for _, value := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 4 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour, 15 * 24 * time.Hour} {
		t.Run("OHLC interval", func(t *testing.T) {
			_, err := ex.GetOHLC(t.Context(), &GetOHLCRequest{Pair: spotTestPair, Interval: value})
			require.NoError(t, err, "GetOHLC must accept the documented interval")
			requireSpotRequest(t, requests, "/0/public/OHLC")
		})
	}
}

func TestOHLCResponseUnmarshalJSON(t *testing.T) {
	var response OHLCResponse
	require.Error(t, response.UnmarshalJSON([]byte(`{`)), "UnmarshalJSON must reject malformed response JSON")
	require.Error(t, json.Unmarshal([]byte(`{"last":{}}`), &response), "UnmarshalJSON must reject an invalid last marker")
	require.Error(t, json.Unmarshal([]byte(`{"BTC/USD":{}}`), &response), "UnmarshalJSON must reject invalid candle data")
	require.NoError(t, json.Unmarshal([]byte(`{}`), &response), "UnmarshalJSON must decode an empty response")
	assert.Empty(t, response.Candles, "UnmarshalJSON should initialise an empty candle map")
	require.NoError(t, json.Unmarshal([]byte(`{"BTC/USD":[[1695828271,"1","2","0.5","1.5","1.2","10",3]],"last":1695828272}`), &response), "UnmarshalJSON must decode current OHLC data")
	assert.Equal(t, uint64(3), response.Candles["BTC/USD"][0].Count, "UnmarshalJSON should decode candle count")
	before := response
	require.Error(t, json.Unmarshal([]byte(`{"last":{}}`), &response), "UnmarshalJSON must reject invalid replacement data")
	assert.Equal(t, before, response, "UnmarshalJSON should preserve the receiver after an error")
}

func TestOHLCEntryUnmarshalJSON(t *testing.T) {
	var entry OHLCEntry
	require.Error(t, entry.UnmarshalJSON([]byte(`{`)), "UnmarshalJSON must reject malformed candle JSON")
	require.Error(t, entry.UnmarshalJSON([]byte(`[]`)), "UnmarshalJSON must reject an incomplete candle")
	require.Error(t, entry.UnmarshalJSON([]byte(`[1,2,3,4,5,6,7,8,9]`)), "UnmarshalJSON must reject an oversized candle")
	require.NoError(t, entry.UnmarshalJSON([]byte(`[1695828271,"1","2","0.5","1.5","1.2","10",3]`)), "UnmarshalJSON must decode a current candle")
	assert.Equal(t, 1.5, entry.Close.Float64(), "UnmarshalJSON should decode close price")
	before := entry
	require.Error(t, entry.UnmarshalJSON([]byte(`[1695828271,"1","2","0.5","1.5","1.2","10","invalid"]`)), "UnmarshalJSON must reject an invalid late candle field")
	assert.Equal(t, before, entry, "UnmarshalJSON should preserve the receiver after an error")
}

func TestOrderbookEntryUnmarshalJSON(t *testing.T) {
	var entry OrderbookEntry
	require.Error(t, entry.UnmarshalJSON([]byte(`{`)), "UnmarshalJSON must reject malformed order-book JSON")
	require.Error(t, entry.UnmarshalJSON([]byte(`[]`)), "UnmarshalJSON must reject an incomplete order-book level")
	require.Error(t, entry.UnmarshalJSON([]byte(`[1,2,3,4]`)), "UnmarshalJSON must reject an oversized order-book level")
	require.NoError(t, entry.UnmarshalJSON([]byte(`["1","2",1695828271]`)), "UnmarshalJSON must decode a current order-book level")
	assert.Equal(t, 2.0, entry.Quantity.Float64(), "UnmarshalJSON should decode quantity")
	before := entry
	require.Error(t, entry.UnmarshalJSON([]byte(`["1","2",{}]`)), "UnmarshalJSON must reject an invalid late order-book field")
	assert.Equal(t, before, entry, "UnmarshalJSON should preserve the receiver after an error")
}

func TestRecentTradesResponseUnmarshalJSON(t *testing.T) {
	var response RecentTradesResponse
	require.Error(t, response.UnmarshalJSON([]byte(`{`)), "UnmarshalJSON must reject malformed trade response JSON")
	require.Error(t, response.UnmarshalJSON([]byte(`{"last":{}}`)), "UnmarshalJSON must reject an invalid last marker")
	require.Error(t, response.UnmarshalJSON([]byte(`{"BTC/USD":{}}`)), "UnmarshalJSON must reject invalid trade data")
	require.NoError(t, response.UnmarshalJSON([]byte(`{}`)), "UnmarshalJSON must decode an empty trade response")
	assert.Empty(t, response.Trades, "UnmarshalJSON should initialise an empty trade map")
	require.NoError(t, response.UnmarshalJSON([]byte(`{"BTC/USD":[["100","2",1695828271,"b","m","",61044952]],"last":"1695828272000000000"}`)), "UnmarshalJSON must decode recent trades")
	assert.Equal(t, 100.0, response.Trades["BTC/USD"][0].Price.Float64(), "UnmarshalJSON should decode trade price")
	assert.Equal(t, "1695828272000000000", response.Last, "UnmarshalJSON should preserve the opaque pagination marker")
	before := response
	require.Error(t, response.UnmarshalJSON([]byte(`{"BTC/USD":[["100","2",1695828271,"b","m","",{}]],"last":"1695828272000000000"}`)), "UnmarshalJSON must reject invalid replacement trade data")
	assert.Equal(t, before, response, "UnmarshalJSON should preserve the receiver after an error")
}

func TestRecentTradeResponseItemUnmarshalJSON(t *testing.T) {
	var item RecentTradeResponseItem
	require.Error(t, item.UnmarshalJSON([]byte(`{`)), "UnmarshalJSON must reject malformed trade JSON")
	require.Error(t, item.UnmarshalJSON([]byte(`[]`)), "UnmarshalJSON must reject an incomplete trade")
	require.Error(t, item.UnmarshalJSON([]byte(`[1,2,3,4,5,6,7,8]`)), "UnmarshalJSON must reject an oversized trade")
	require.NoError(t, item.UnmarshalJSON([]byte(`["100","2",1695828271,"b","m","",61044952]`)), "UnmarshalJSON must decode a recent trade")
	assert.Equal(t, 61044952.0, item.TradeID.Float64(), "UnmarshalJSON should decode trade ID")
	before := item
	require.Error(t, item.UnmarshalJSON([]byte(`["100","2",1695828271,"b","m","",{}]`)), "UnmarshalJSON must reject an invalid late trade field")
	assert.Equal(t, before, item, "UnmarshalJSON should preserve the receiver after an error")
}

func TestSpreadItemUnmarshalJSON(t *testing.T) {
	var item SpreadItem
	require.Error(t, item.UnmarshalJSON([]byte(`{`)), "UnmarshalJSON must reject malformed spread JSON")
	require.Error(t, item.UnmarshalJSON([]byte(`[]`)), "UnmarshalJSON must reject an incomplete spread")
	require.Error(t, item.UnmarshalJSON([]byte(`[1,2,3,4]`)), "UnmarshalJSON must reject an oversized spread")
	require.NoError(t, item.UnmarshalJSON([]byte(`[1695828271,"99","101"]`)), "UnmarshalJSON must decode a spread")
	assert.Equal(t, 101.0, item.Ask.Float64(), "UnmarshalJSON should decode ask price")
	before := item
	require.Error(t, item.UnmarshalJSON([]byte(`[1695828271,"99",{}]`)), "UnmarshalJSON must reject an invalid late spread field")
	assert.Equal(t, before, item, "UnmarshalJSON should preserve the receiver after an error")
}

func TestSpreadResponseUnmarshalJSON(t *testing.T) {
	var response SpreadResponse
	require.Error(t, response.UnmarshalJSON([]byte(`{`)), "UnmarshalJSON must reject malformed spread response JSON")
	require.Error(t, response.UnmarshalJSON([]byte(`{"last":{}}`)), "UnmarshalJSON must reject an invalid last marker")
	require.Error(t, response.UnmarshalJSON([]byte(`{"BTC/USD":{}}`)), "UnmarshalJSON must reject invalid spread data")
	require.NoError(t, response.UnmarshalJSON([]byte(`{}`)), "UnmarshalJSON must decode an empty spread response")
	assert.Empty(t, response.Spreads, "UnmarshalJSON should initialise an empty spread map")
	require.NoError(t, response.UnmarshalJSON([]byte(`{"BTC/USD":[[1695828271,"99","101"]],"last":1695828272}`)), "UnmarshalJSON must decode recent spreads")
	assert.Equal(t, 99.0, response.Spreads["BTC/USD"][0].Bid.Float64(), "UnmarshalJSON should decode bid price")
	before := response
	require.Error(t, response.UnmarshalJSON([]byte(`{"BTC/USD":[[1695828271,"99",{}]],"last":1695828272}`)), "UnmarshalJSON must reject invalid replacement spread data")
	assert.Equal(t, before, response, "UnmarshalJSON should preserve the receiver after an error")
}

func TestTradeInfoJSONUnmarshal(t *testing.T) {
	var trade TradeInfo
	require.Error(t, json.Unmarshal([]byte(`{`), &trade), "json.Unmarshal must reject malformed trade info JSON")
	require.Error(t, json.Unmarshal([]byte(`{"cprice":{}}`), &trade), "json.Unmarshal must reject an invalid closed-position value")
	require.NoError(t, json.Unmarshal([]byte(`{"price":"100","cost":"100","fee":"0.2","vol":"1","margin":"0","leverage":"2","cprice":101.5,"ccost":100.5,"cfee":0.1,"cvol":1,"cmargin":20,"net":1.5}`), &trade), "json.Unmarshal must decode numeric closed-position values")
	assert.Equal(t, 101.5, trade.ClosedPositionAveragePrice.Float64(), "TradeInfo.ClosedPositionAveragePrice should decode correctly")
	assert.Equal(t, 100.5, trade.ClosedPositionCost.Float64(), "TradeInfo.ClosedPositionCost should decode correctly")
	require.NoError(t, json.Unmarshal([]byte(`{"price":"100","cost":"100","fee":"0.2","vol":"1","margin":"0","cprice":"101.5","cfee":"0.1","cvol":"1","cmargin":"20"}`), &trade), "json.Unmarshal must decode quoted closed-position values")
	assert.Equal(t, 20.0, trade.ClosedPositionMargin.Float64(), "TradeInfo.ClosedPositionMargin should decode correctly")
}

func TestTradeVolumeFeeJSONUnmarshal(t *testing.T) {
	var fee TradeVolumeFee
	require.Error(t, json.Unmarshal([]byte(`{`), &fee), "json.Unmarshal must reject malformed fee info JSON")
	require.Error(t, json.Unmarshal([]byte(`{"fee":"0.2","volumeoffset":{}}`), &fee), "json.Unmarshal must reject an invalid fee-tier value")
	require.NoError(t, json.Unmarshal([]byte(`{"fee":"0.2","minfee":"0.1","maxfee":"0.3","nextfee":null,"tiervolume":"100","tierfuturesvolume":null,"nextvolume":null,"nextfuturesvolume":null,"volumeoffset":"5"}`), &fee), "json.Unmarshal must decode nullable fee-tier values")
	assert.Nil(t, fee.NextFee, "TradeVolumeFee.NextFee should preserve a null value")
	assert.Nil(t, fee.NextVolume, "TradeVolumeFee.NextVolume should preserve a null value")
	require.NotNil(t, fee.VolumeOffset, "TradeVolumeFee.VolumeOffset must decode a non-null value")
	assert.Equal(t, 5.0, fee.VolumeOffset.Float64(), "TradeVolumeFee.VolumeOffset should decode correctly")
	require.NoError(t, json.Unmarshal([]byte(`{"fee":"0.2","minfee":"0.1","maxfee":"0.3","nextfee":"0.15","tiervolume":"100","tierfuturesvolume":"50","nextvolume":"200","nextfuturesvolume":"100","volumeoffset":null}`), &fee), "json.Unmarshal must decode non-null fee-tier values")
	require.NotNil(t, fee.NextFee, "TradeVolumeFee.NextFee must decode a non-null value")
	assert.Equal(t, 0.15, fee.NextFee.Float64(), "TradeVolumeFee.NextFee should decode correctly")
	require.NotNil(t, fee.NextVolume, "TradeVolumeFee.NextVolume must decode a non-null value")
	assert.Equal(t, 200.0, fee.NextVolume.Float64(), "TradeVolumeFee.NextVolume should decode correctly")
}

func TestSpotAccountRequestModels(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()
	zeroUserReference := int32(0)
	falseValue := false
	trueValue := true
	tradeLimit := uint64(100)
	startTime := time.Unix(2, 0)
	endTime := time.Unix(1, 0)

	_, err := ex.GetTradeBalance(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetTradeBalance must reject a nil request")
	_, err = ex.GetTradeBalance(ctx, &GetTradeBalanceRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetTradeBalance must reject an invalid rebase multiplier")
	balance, err := ex.GetTradeBalance(ctx, &GetTradeBalanceRequest{Asset: "USD", RebaseMultiplier: "base"})
	require.NoError(t, err, "GetTradeBalance must not error")
	assert.Equal(t, 21.1063, balance.CostBasis.Float64(), "GetTradeBalance should decode cost basis")
	assert.Equal(t, 31.1297, balance.CurrentValuation.Float64(), "GetTradeBalance should decode current valuation")
	assert.Equal(t, 374.0, balance.FreeMarginOrders.Float64(), "GetTradeBalance should decode free margin for orders")
	assert.Equal(t, 2.0, balance.UnexecutedValue.Float64(), "GetTradeBalance should decode unexecuted value")
	values := requireSpotRequest(t, requests, "/0/private/TradeBalance")
	assert.Equal(t, "USD", values.Get("asset"), "GetTradeBalance should encode asset")
	assert.Equal(t, "base", values.Get("rebase_multiplier"), "GetTradeBalance should encode rebase multiplier")
	_, err = ex.GetTradeBalance(ctx, &GetTradeBalanceRequest{})
	require.NoError(t, err, "GetTradeBalance must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/TradeBalance")

	_, err = ex.GetOpenOrders(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetOpenOrders must reject a nil request")
	_, err = ex.GetOpenOrders(ctx, &GetOpenOrdersRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetOpenOrders must reject an invalid rebase multiplier")
	openOrders, err := ex.GetOpenOrders(ctx, &GetOpenOrdersRequest{Trades: true, UserReference: &zeroUserReference, ClientOrderID: "CLIENT", RebaseMultiplier: "rebased"})
	require.NoError(t, err, "GetOpenOrders must not error")
	assert.Equal(t, "CLIENT", openOrders.Open["ORDER"].ClientOrderID, "GetOpenOrders should decode client order ID")
	assert.Equal(t, "currency", openOrders.Open["ORDER"].Description.AssetClass, "GetOpenOrders should decode description asset class")
	assert.Equal(t, "gtc", openOrders.Open["ORDER"].TimeInForce, "GetOpenOrders should decode time-in-force")
	assert.Equal(t, "last", openOrders.Open["ORDER"].Trigger, "GetOpenOrders should decode trigger")
	assert.Equal(t, "SUB", openOrders.Open["ORDER"].SenderSubID, "GetOpenOrders should decode sender subaccount")
	values = requireSpotRequest(t, requests, "/0/private/OpenOrders")
	assert.Equal(t, "true", values.Get("trades"), "GetOpenOrders should encode trades")
	assert.Equal(t, "0", values.Get("userref"), "GetOpenOrders should encode an explicit zero user reference")
	assert.Equal(t, "CLIENT", values.Get("cl_ord_id"), "GetOpenOrders should encode client order ID")
	assert.Equal(t, "rebased", values.Get("rebase_multiplier"), "GetOpenOrders should encode rebase multiplier")
	_, err = ex.GetOpenOrders(ctx, &GetOpenOrdersRequest{})
	require.NoError(t, err, "GetOpenOrders must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/OpenOrders")

	_, err = ex.GetClosedOrders(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetClosedOrders must reject a nil request")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{CloseTime: invalidSpotValue})
	require.ErrorIs(t, err, errCloseTimeInvalid, "GetClosedOrders must reject an invalid close-time selector")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetClosedOrders must reject an invalid rebase multiplier")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{Start: TimeOrTransactionID{Time: startTime, TransactionID: "START"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetClosedOrders must reject conflicting start boundaries")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{End: TimeOrTransactionID{Time: endTime, TransactionID: "END"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetClosedOrders must reject conflicting end boundaries")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{Start: TimeOrTransactionID{Time: startTime}, End: TimeOrTransactionID{Time: endTime}})
	require.ErrorIs(t, err, errTimeRangeInvalid, "GetClosedOrders must reject a reversed time range")
	closedOrders, err := ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{
		Trades:           true,
		UserReference:    &zeroUserReference,
		ClientOrderID:    "CLIENT",
		Start:            TimeOrTransactionID{TransactionID: "START"},
		End:              TimeOrTransactionID{TransactionID: "END"},
		Offset:           1,
		CloseTime:        CloseTimeBoth,
		ConsolidateTaker: &falseValue,
		WithoutCount:     true,
		RebaseMultiplier: RebaseMultiplierBase,
	})
	require.NoError(t, err, "GetClosedOrders must not error")
	assert.Equal(t, int64(1), closedOrders.Count, "GetClosedOrders should decode count")
	assert.True(t, closedOrders.Closed["ORDER"].Margin, "GetClosedOrders should decode margin status")
	assert.Equal(t, "User requested", closedOrders.Closed["ORDER"].Reason, "GetClosedOrders should decode status reason")
	values = requireSpotRequest(t, requests, "/0/private/ClosedOrders")
	assert.Equal(t, "0", values.Get("userref"), "GetClosedOrders should encode an explicit zero user reference")
	assert.Equal(t, "START", values.Get("start"), "GetClosedOrders should encode start")
	assert.Equal(t, "END", values.Get("end"), "GetClosedOrders should encode end")
	assert.Equal(t, "1", values.Get("ofs"), "GetClosedOrders should encode offset")
	assert.Equal(t, "both", values.Get("closetime"), "GetClosedOrders should encode close-time selector")
	assert.Equal(t, "false", values.Get("consolidate_taker"), "GetClosedOrders should encode explicit false consolidation")
	assert.Equal(t, "true", values.Get("without_count"), "GetClosedOrders should encode count suppression")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{})
	require.NoError(t, err, "GetClosedOrders must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/ClosedOrders")

	_, err = ex.QueryOrdersInfo(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "QueryOrdersInfo must reject a nil request")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{})
	require.ErrorIs(t, err, errOrderIdentifierCount, "QueryOrdersInfo must require an order identifier")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: make([]string, 51)})
	require.ErrorIs(t, err, errOrderIdentifierCount, "QueryOrdersInfo must reject more than fifty identifiers")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{""}})
	require.ErrorIs(t, err, errOrderIDRequired, "QueryOrdersInfo must reject an empty order identifier")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{"ORDER"}, RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "QueryOrdersInfo must reject an invalid rebase multiplier")
	orders, err := ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{"ORDER", "ORDER2"}, Trades: true, UserReference: &zeroUserReference, ConsolidateTaker: &falseValue, RebaseMultiplier: "base"})
	require.NoError(t, err, "QueryOrdersInfo must not error")
	assert.Equal(t, "ioc", orders["ORDER"].TimeInForce, "QueryOrdersInfo should decode time-in-force")
	values = requireSpotRequest(t, requests, "/0/private/QueryOrders")
	assert.Equal(t, "ORDER,ORDER2", values.Get("txid"), "QueryOrdersInfo should encode order identifiers")
	assert.Equal(t, "false", values.Get("consolidate_taker"), "QueryOrdersInfo should encode explicit false consolidation")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{"ORDER"}})
	require.NoError(t, err, "QueryOrdersInfo must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/QueryOrders")

	_, err = ex.GetTradesHistory(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetTradesHistory must reject a nil request")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Type: invalidSpotValue})
	require.ErrorIs(t, err, errTradeTypeInvalid, "GetTradesHistory must reject an invalid trade type")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetTradesHistory must reject an invalid rebase multiplier")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{AssetClass: AssetClassCurrency})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetTradesHistory must reject an invalid asset class")
	invalidTradeLimit := uint64(0)
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Limit: &invalidTradeLimit})
	require.ErrorIs(t, err, errTradeLimitInvalid, "GetTradesHistory must reject an invalid result limit")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Start: TimeOrTransactionID{Time: startTime, TransactionID: "START"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetTradesHistory must reject conflicting start boundaries")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{End: TimeOrTransactionID{Time: endTime, TransactionID: "END"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetTradesHistory must reject conflicting end boundaries")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Start: TimeOrTransactionID{Time: startTime}, End: TimeOrTransactionID{Time: endTime}})
	require.ErrorIs(t, err, errTimeRangeInvalid, "GetTradesHistory must reject a reversed time range")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Pair: currency.Pair{Base: currency.BTC}})
	require.ErrorIs(t, err, errPairRequired, "GetTradesHistory must reject a partially populated pair")
	_, err = new(Exchange).GetTradesHistory(ctx, &GetTradesHistoryRequest{Pair: spotTestPair})
	require.Error(t, err, "GetTradesHistory must surface pair-format errors")
	history, err := ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{
		Type:             TradeHistoryClosedPosition,
		Trades:           true,
		Start:            TimeOrTransactionID{TransactionID: "START"},
		End:              TimeOrTransactionID{TransactionID: "END"},
		Offset:           1,
		WithoutCount:     true,
		ConsolidateTaker: &falseValue,
		Ledgers:          true,
		RebaseMultiplier: RebaseMultiplierRebased,
		AssetClass:       AssetClassForex,
		Pair:             spotTestPair,
		Limit:            &tradeLimit,
	})
	require.NoError(t, err, "GetTradesHistory must not error")
	assert.Equal(t, int64(1), history.Count, "GetTradesHistory should decode count")
	trade := history.Trades["TRADE"]
	assert.Equal(t, "POSITION", trade.PosTxID, "GetTradesHistory should decode position identifier")
	assert.Equal(t, "2", trade.Leverage, "GetTradesHistory should decode leverage")
	assert.Equal(t, 101.5, trade.ClosedPositionAveragePrice.Float64(), "GetTradesHistory should decode numeric closed-position price")
	assert.Equal(t, 100.5, trade.ClosedPositionCost.Float64(), "GetTradesHistory should decode closed-position cost")
	assert.Equal(t, 1.5, trade.Net.Float64(), "GetTradesHistory should decode net result")
	assert.Equal(t, []string{"LEDGER"}, trade.Ledgers, "GetTradesHistory should decode ledger identifiers")
	assert.Equal(t, uint64(42), trade.TradeID, "GetTradesHistory should decode trade identifier")
	assert.True(t, trade.Maker, "GetTradesHistory should decode maker status")
	assert.Equal(t, "currency", trade.AssetClass, "GetTradesHistory should decode asset class")
	assert.Equal(t, "market", trade.TradeOrderType, "GetTradesHistory should decode execution order type")
	values = requireSpotRequest(t, requests, "/0/private/TradesHistory")
	assert.Equal(t, "closed position", values.Get("type"), "GetTradesHistory should encode trade type")
	assert.Equal(t, "1", values.Get("ofs"), "GetTradesHistory should encode offset")
	assert.Equal(t, "true", values.Get("without_count"), "GetTradesHistory should encode count suppression")
	assert.Equal(t, "false", values.Get("consolidate_taker"), "GetTradesHistory should encode explicit false consolidation")
	assert.Equal(t, "true", values.Get("ledgers"), "GetTradesHistory should encode ledger inclusion")
	assert.Equal(t, "forex", values.Get("aclass"), "GetTradesHistory should encode asset class")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "GetTradesHistory should encode the formatted pair")
	assert.Equal(t, "100", values.Get("limit"), "GetTradesHistory should encode the result limit")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{})
	require.NoError(t, err, "GetTradesHistory must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/TradesHistory")

	_, err = ex.QueryTrades(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "QueryTrades must reject a nil request")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{})
	require.ErrorIs(t, err, errTradeIdentifierCount, "QueryTrades must require a trade identifier")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: make([]string, 21)})
	require.ErrorIs(t, err, errTradeIdentifierCount, "QueryTrades must reject more than twenty identifiers")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{""}})
	require.ErrorIs(t, err, errTransactionIDRequired, "QueryTrades must reject an empty trade identifier")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{"TRADE"}, RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "QueryTrades must reject an invalid rebase multiplier")
	queriedTrades, err := ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{"TRADE", "TRADE2"}, Trades: true, RebaseMultiplier: "base"})
	require.NoError(t, err, "QueryTrades must not error")
	assert.Equal(t, "ORDER", queriedTrades["TRADE"].OrderTxID, "QueryTrades should decode order identifier")
	assert.Equal(t, 101.5, queriedTrades["TRADE"].ClosedPositionAveragePrice.Float64(), "QueryTrades should decode quoted closed-position values")
	values = requireSpotRequest(t, requests, "/0/private/QueryTrades")
	assert.Equal(t, "TRADE,TRADE2", values.Get("txid"), "QueryTrades should encode trade identifiers")
	assert.Equal(t, "true", values.Get("trades"), "QueryTrades should encode related trades")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{"TRADE"}})
	require.NoError(t, err, "QueryTrades must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/QueryTrades")

	_, err = ex.OpenPositions(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "OpenPositions must reject a nil request")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{Consolidation: invalidSpotValue})
	require.ErrorIs(t, err, errConsolidationInvalid, "OpenPositions must reject an invalid consolidation")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "OpenPositions must reject an invalid rebase multiplier")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{TransactionIDs: []string{""}})
	require.ErrorIs(t, err, errTransactionIDRequired, "OpenPositions must reject an empty transaction identifier")
	positions, err := ex.OpenPositions(ctx, &OpenPositionsRequest{TransactionIDs: []string{"POSITION", "POSITION2"}, DoCalculations: true, Consolidation: "market", RebaseMultiplier: "base"})
	require.NoError(t, err, "OpenPositions must not error")
	assert.Equal(t, "ORDER", positions["POSITION"].Ordertxid, "OpenPositions should decode order identifier")
	assert.Equal(t, "currency", positions["POSITION"].AssetClass, "OpenPositions should decode asset class")
	assert.Equal(t, 120.0, positions["POSITION"].Value.Float64(), "OpenPositions should decode position value")
	values = requireSpotRequest(t, requests, "/0/private/OpenPositions")
	assert.Equal(t, "POSITION,POSITION2", values.Get("txid"), "OpenPositions should encode position identifiers")
	assert.Equal(t, "true", values.Get("docalcs"), "OpenPositions should encode calculations")
	assert.Equal(t, "market", values.Get("consolidation"), "OpenPositions should encode consolidation")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{})
	require.NoError(t, err, "OpenPositions must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/OpenPositions")

	_, err = ex.QueryLedgers(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "QueryLedgers must reject a nil request")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{})
	require.ErrorIs(t, err, errLedgerIdentifierCount, "QueryLedgers must require a ledger identifier")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: make([]string, 21)})
	require.ErrorIs(t, err, errLedgerIdentifierCount, "QueryLedgers must reject more than twenty identifiers")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{""}})
	require.ErrorIs(t, err, errIDRequired, "QueryLedgers must reject an empty ledger identifier")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{"LEDGER"}, RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "QueryLedgers must reject an invalid rebase multiplier")
	queriedLedgers, err := ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{"LEDGER", "LEDGER2"}, Trades: true, RebaseMultiplier: "base"})
	require.NoError(t, err, "QueryLedgers must not error")
	assert.Equal(t, "REFERENCE", queriedLedgers["LEDGER"].Refid, "QueryLedgers should decode reference identifier")
	assert.Equal(t, "spotfromfutures", queriedLedgers["LEDGER"].Subtype, "QueryLedgers should decode ledger subtype")
	values = requireSpotRequest(t, requests, "/0/private/QueryLedgers")
	assert.Equal(t, "LEDGER,LEDGER2", values.Get("id"), "QueryLedgers should encode ledger identifiers")
	assert.Equal(t, "true", values.Get("trades"), "QueryLedgers should encode related trades")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{"LEDGER"}})
	require.NoError(t, err, "QueryLedgers must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/QueryLedgers")

	_, err = ex.GetTradeVolume(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetTradeVolume must reject a nil request")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetTradeVolume must reject an invalid rebase multiplier")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{AssetClass: AssetClassForex}}})
	require.ErrorIs(t, err, errAssetRequired, "GetTradeVolume must require an asset for class-qualified pairs")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{Asset: "XBTUSD"}}})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetTradeVolume must require an asset class for class-qualified pairs")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{Asset: "XBTUSD", AssetClass: invalidSpotValue}}})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetTradeVolume must reject an invalid class-qualified pair")
	feeInfo := false
	volume, err := ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{Asset: "XBTUSD", AssetClass: AssetClassForex}}, FeeInfo: &feeInfo, FeeSchedule: &trueValue, RebaseMultiplier: RebaseMultiplierRebased})
	require.NoError(t, err, "GetTradeVolume must not error")
	assert.Equal(t, "currency", volume.AssetClass, "GetTradeVolume should decode asset class")
	assert.Equal(t, 200.0, volume.Inputs.SpotVolume30D.Float64(), "GetTradeVolume should decode spot volume input")
	assert.Equal(t, "SUB", volume.VolumeSubaccounts[0].IIBAN, "GetTradeVolume should decode subaccount volume")
	assert.Equal(t, "BTC/USD", volume.Schedules[0].Pair, "GetTradeVolume should decode fee schedule")
	assert.True(t, *volume.Schedules[0].Tiers[0].Active, "GetTradeVolume should decode active fee tier")
	assert.Nil(t, volume.Fees["BTC/USD"].NextFee, "GetTradeVolume should preserve a null next fee")
	assert.Nil(t, volume.Fees["BTC/USD"].NextVolume, "GetTradeVolume should preserve a null next volume")
	assert.Equal(t, 5.0, volume.Fees["BTC/USD"].VolumeOffset.Float64(), "GetTradeVolume should decode a volume offset")
	require.NotNil(t, volume.FeesMaker["BTC/USD"].NextFee, "GetTradeVolume must decode a non-null next fee")
	assert.Equal(t, 0.08, volume.FeesMaker["BTC/USD"].NextFee.Float64(), "GetTradeVolume should decode the next fee")
	assert.Equal(t, 50.0, volume.FeesMaker["BTC/USD"].TierFuturesVolume.Float64(), "GetTradeVolume should decode the futures-volume tier")
	assert.Equal(t, 100.0, volume.FeesMaker["BTC/USD"].NextFuturesVolume.Float64(), "GetTradeVolume should decode the next futures-volume tier")
	values = requireSpotRequest(t, requests, "/0/private/TradeVolume")
	assert.JSONEq(t, `[{"asset":"XBTUSD","aclass":"forex"}]`, values.Get("pair"), "GetTradeVolume should encode class-qualified pairs")
	assert.Equal(t, "false", values.Get("fee-info"), "GetTradeVolume should encode explicit false fee info")
	assert.Equal(t, "true", values.Get("fee_schedule"), "GetTradeVolume should encode fee schedule selection")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{})
	require.NoError(t, err, "GetTradeVolume must allow optional parameters to be omitted")
	values = requireSpotRequest(t, requests, "/0/private/TradeVolume")
	values.Del("nonce")
	assert.Empty(t, values, "GetTradeVolume should omit unset parameters")
}

func TestSpotTradeVolumeAssetClasses(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	for _, value := range []AssetClass{AssetClassCurrency, AssetClassForex, AssetClassEquity, AssetClassEquityPair, AssetClassNFT, AssetClassDerivatives, AssetClassTokenizedAsset, AssetClassFuturesContract} {
		t.Run(string(value), func(t *testing.T) {
			_, err := ex.GetTradeVolume(t.Context(), &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{Asset: "XBTUSD", AssetClass: value}}})
			require.NoError(t, err, "GetTradeVolume must accept the documented asset class")
			requireSpotRequest(t, requests, "/0/private/TradeVolume")
		})
	}
}

func TestSpotAccountEnums(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	for _, value := range []CloseTime{CloseTimeOpen, CloseTimeClose, CloseTimeBoth} {
		t.Run("close time "+string(value), func(t *testing.T) {
			_, err := ex.GetClosedOrders(t.Context(), &GetClosedOrdersRequest{CloseTime: value})
			require.NoError(t, err, "GetClosedOrders must accept the documented close-time selector")
			requireSpotRequest(t, requests, "/0/private/ClosedOrders")
		})
	}
	for _, value := range []TradeHistoryType{TradeHistoryAll, TradeHistoryAnyPosition, TradeHistoryClosedPosition, TradeHistoryClosingPosition, TradeHistoryNoPosition} {
		t.Run("trade type "+string(value), func(t *testing.T) {
			_, err := ex.GetTradesHistory(t.Context(), &GetTradesHistoryRequest{Type: value})
			require.NoError(t, err, "GetTradesHistory must accept the documented trade type")
			requireSpotRequest(t, requests, "/0/private/TradesHistory")
		})
	}
	for _, value := range []RebaseMultiplier{RebaseMultiplierRebased, RebaseMultiplierBase} {
		t.Run("rebase multiplier "+string(value), func(t *testing.T) {
			_, err := ex.GetTradeBalance(t.Context(), &GetTradeBalanceRequest{RebaseMultiplier: value})
			require.NoError(t, err, "GetTradeBalance must accept the documented rebase multiplier")
			requireSpotRequest(t, requests, "/0/private/TradeBalance")
		})
	}
}

func TestSpotTradingRequestModels(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()
	userReference := int32(0)
	cancelUserReference := int32(-42)
	deadline := time.Now().Add(30 * time.Second).In(time.FixedZone("AEST", 10*60*60))
	displayVolume := 0.5
	validRequest := &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Volume: 1, Pair: spotTestPair}

	_, err := ex.AddOrder(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AddOrder must reject a nil request")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{})
	require.ErrorIs(t, err, errOrderTypeInvalid, "AddOrder must require an order type")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: invalidSpotValue})
	require.ErrorIs(t, err, errOrderTypeInvalid, "AddOrder must reject an invalid order type")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: "limit"})
	require.ErrorIs(t, err, errOrderSideInvalid, "AddOrder must require an order side")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: "limit", Side: invalidSpotValue})
	require.ErrorIs(t, err, errOrderSideInvalid, "AddOrder must reject an invalid order side")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy})
	require.ErrorIs(t, err, errPairRequired, "AddOrder must require a pair")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Pair: spotTestPair})
	require.NoError(t, err, "AddOrder must accept an explicit zero volume")
	values := requireSpotRequest(t, requests, "/0/private/AddOrder")
	assert.Equal(t, "0", values.Get("volume"), "AddOrder should encode an explicit zero volume")
	conflictingIdentity := *validRequest
	conflictingIdentity.UserReference = &userReference
	conflictingIdentity.ClientOrderID = "CLIENT"
	_, err = ex.AddOrder(ctx, &conflictingIdentity)
	require.ErrorIs(t, err, errOrderIdentityConflict, "AddOrder must reject conflicting client identifiers")
	invalidRequest := *validRequest
	invalidRequest.AssetClass = "currency"
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errAssetClassInvalid, "AddOrder must reject an invalid asset class")
	invalidRequest = *validRequest
	invalidRequest.Trigger = invalidSpotValue
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errTriggerInvalid, "AddOrder must reject an invalid trigger")
	invalidRequest = *validRequest
	invalidRequest.SelfTradePolicy = invalidSpotValue
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errSelfTradeInvalid, "AddOrder must reject an invalid self-trade policy")
	invalidRequest = *validRequest
	invalidRequest.TimeInForce = invalidSpotValue
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errTimeInForceInvalid, "AddOrder must reject an invalid time-in-force")
	invalidRequest = *validRequest
	invalidRequest.Close = new(AddOrderCloseRequest)
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errBatchCloseTypeInvalid, "AddOrder must require a conditional close order type")
	invalidRequest = *validRequest
	invalidRequest.Close = &AddOrderCloseRequest{OrderType: invalidSpotValue}
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errBatchCloseTypeInvalid, "AddOrder must reject an invalid conditional close order type")

	added, err := ex.AddOrder(ctx, &AddOrderRequest{
		UserReference:   &userReference,
		OrderType:       OrderTypeLimit,
		Side:            OrderSideBuy,
		Volume:          1,
		DisplayVolume:   &displayVolume,
		Pair:            spotTestPair,
		AssetClass:      AssetClassTokenizedAsset,
		Price:           &OrderPrice{Value: 100},
		SecondaryPrice:  &OrderPrice{Value: 90},
		Trigger:         OrderTriggerLast,
		Leverage:        2,
		ReduceOnly:      true,
		SelfTradePolicy: SelfTradePolicyCancelBoth,
		OrderFlags:      []OrderFlag{OrderFlagPostOnly},
		TimeInForce:     OrderTimeInForceFOK,
		StartDelay:      5 * time.Second,
		ExpireAfter:     time.Minute,
		Close:           &AddOrderCloseRequest{OrderType: OrderTypeStopLossLimit, Price: &OrderPrice{Value: 80}, SecondaryPrice: &OrderPrice{Value: 75}},
		Deadline:        deadline,
		Validate:        true,
		Broker:          "BROKER",
	})
	require.NoError(t, err, "AddOrder must not error")
	assert.Equal(t, []string{"ORDER"}, added.TransactionIDs, "AddOrder should decode transaction IDs")
	values = requireSpotRequest(t, requests, "/0/private/AddOrder")
	assert.Equal(t, "0", values.Get("userref"), "AddOrder should encode an explicit zero user reference")
	assert.Equal(t, "0.5", values.Get("displayvol"), "AddOrder should encode display volume")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "AddOrder should encode the formatted pair")
	assert.Equal(t, "tokenized_asset", values.Get("asset_class"), "AddOrder should encode asset class")
	assert.Equal(t, "90", values.Get("price2"), "AddOrder should encode secondary price")
	assert.Equal(t, "last", values.Get("trigger"), "AddOrder should encode trigger")
	assert.Equal(t, "2", values.Get("leverage"), "AddOrder should encode leverage")
	assert.Equal(t, "true", values.Get("reduce_only"), "AddOrder should encode reduce-only")
	assert.Equal(t, "cancel-both", values.Get("stptype"), "AddOrder should encode self-trade policy")
	assert.Equal(t, "FOK", values.Get("timeinforce"), "AddOrder should encode time-in-force")
	assert.Equal(t, "+5", values.Get("starttm"), "AddOrder should preserve relative start time")
	assert.Equal(t, "+60", values.Get("expiretm"), "AddOrder should preserve relative expiry time")
	assert.Equal(t, "stop-loss-limit", values.Get("close[ordertype]"), "AddOrder should encode conditional close type")
	assert.Equal(t, "80", values.Get("close[price]"), "AddOrder should encode conditional close price")
	assert.Equal(t, "75", values.Get("close[price2]"), "AddOrder should encode conditional close secondary price")
	assert.Equal(t, deadline.UTC().Format(time.RFC3339Nano), values.Get("deadline"), "AddOrder should encode deadline in UTC")
	assert.Equal(t, "BROKER", values.Get("broker"), "AddOrder should encode broker")

	clientRequest := *validRequest
	clientRequest.ClientOrderID = "CLIENT"
	_, err = ex.AddOrder(ctx, &clientRequest)
	require.NoError(t, err, "AddOrder must accept a client order ID")
	values = requireSpotRequest(t, requests, "/0/private/AddOrder")
	assert.Equal(t, "CLIENT", values.Get("cl_ord_id"), "AddOrder should encode client order ID")
	assert.Empty(t, values.Get("userref"), "AddOrder should omit user reference when unset")
	assert.Empty(t, values.Get("close[price]"), "AddOrder should omit conditional close fields when unset")

	_, err = ex.CancelExistingOrder(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CancelExistingOrder must reject a nil request")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{})
	require.ErrorIs(t, err, errOrderIdentityRequired, "CancelExistingOrder must require an order identifier")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER", UserReference: &cancelUserReference})
	require.ErrorIs(t, err, errOrderIdentityConflict, "CancelExistingOrder must reject multiple order identifiers")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER", ClientOrderID: "CLIENT"})
	require.ErrorIs(t, err, errOrderIdentityConflict, "CancelExistingOrder must reject transaction and client identifiers together")
	cancelled, err := ex.CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER"})
	require.NoError(t, err, "CancelExistingOrder must accept a transaction identifier")
	assert.Equal(t, int64(1), cancelled.Count, "CancelExistingOrder should decode cancellation count")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrder")
	assert.Equal(t, "ORDER", values.Get("txid"), "CancelExistingOrder should encode transaction identifier")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{UserReference: &cancelUserReference})
	require.NoError(t, err, "CancelExistingOrder must accept a user reference")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrder")
	assert.Equal(t, "-42", values.Get("txid"), "CancelExistingOrder should encode signed user reference")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{ClientOrderID: "CLIENT"})
	require.NoError(t, err, "CancelExistingOrder must accept a client order ID")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrder")
	assert.Equal(t, "CLIENT", values.Get("cl_ord_id"), "CancelExistingOrder should encode client order ID")
}

func TestSpotAddOrderEnums(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	testValue := func(t *testing.T, mutate func(*AddOrderRequest)) {
		t.Helper()
		req := &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Volume: 1, Pair: spotTestPair}
		mutate(req)
		_, err := ex.AddOrder(t.Context(), req)
		require.NoError(t, err, "AddOrder must accept the documented enum value")
		requireSpotRequest(t, requests, "/0/private/AddOrder")
	}

	for _, value := range []OrderType{OrderTypeMarket, OrderTypeLimit, OrderTypeIceberg, OrderTypeStopLoss, OrderTypeTakeProfit, OrderTypeStopLossLimit, OrderTypeTakeProfitLimit, OrderTypeTrailingStop, OrderTypeTrailingStopLimit, OrderTypeSettlePosition} {
		t.Run("order type "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderRequest) { req.OrderType = value })
		})
	}
	for _, value := range []OrderSide{OrderSideBuy, OrderSideSell} {
		t.Run("side "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderRequest) { req.Side = value })
		})
	}
	for _, value := range []OrderTrigger{OrderTriggerIndex, OrderTriggerLast} {
		t.Run("trigger "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderRequest) { req.Trigger = value })
		})
	}
	for _, value := range []SelfTradePolicy{SelfTradePolicyCancelNewest, SelfTradePolicyCancelOldest, SelfTradePolicyCancelBoth} {
		t.Run("self trade "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderRequest) { req.SelfTradePolicy = value })
		})
	}
	for _, value := range []OrderTimeInForce{OrderTimeInForceGTC, OrderTimeInForceIOC, OrderTimeInForceGTD, OrderTimeInForceFOK} {
		t.Run("time in force "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderRequest) { req.TimeInForce = value })
		})
	}
	for _, value := range []OrderType{OrderTypeLimit, OrderTypeIceberg, OrderTypeStopLoss, OrderTypeTakeProfit, OrderTypeStopLossLimit, OrderTypeTakeProfitLimit, OrderTypeTrailingStop, OrderTypeTrailingStopLimit} {
		t.Run("close order type "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderRequest) { req.Close = &AddOrderCloseRequest{OrderType: value} })
		})
	}
}

func TestSpotEndpointTransportErrors(t *testing.T) {
	ex := newSpotErrorExchange(t)
	ctx := t.Context()

	_, err := ex.GetAssets(ctx, &GetAssetsRequest{})
	require.Error(t, err, "GetAssets must surface request errors")
	_, err = ex.GetAssetPairs(ctx, &GetAssetPairsRequest{})
	require.Error(t, err, "GetAssetPairs must surface request errors")
	_, err = ex.GetTicker(ctx, &GetTickerRequest{})
	require.Error(t, err, "GetTicker must surface request errors")
	_, err = ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair})
	require.Error(t, err, "GetOHLC must surface request errors")
	_, err = ex.GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair})
	require.Error(t, err, "GetDepth must surface request errors")
	_, err = ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair})
	require.Error(t, err, "GetTrades must surface request errors")
	_, err = ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair})
	require.Error(t, err, "GetSpread must surface request errors")
	_, err = ex.GetTradeBalance(ctx, &GetTradeBalanceRequest{})
	require.Error(t, err, "GetTradeBalance must surface request errors")
	_, err = ex.GetOpenOrders(ctx, &GetOpenOrdersRequest{})
	require.Error(t, err, "GetOpenOrders must surface request errors")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{})
	require.Error(t, err, "GetClosedOrders must surface request errors")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{"ORDER"}})
	require.Error(t, err, "QueryOrdersInfo must surface request errors")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{})
	require.Error(t, err, "GetTradesHistory must surface request errors")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{"TRADE"}})
	require.Error(t, err, "QueryTrades must surface request errors")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{})
	require.Error(t, err, "OpenPositions must surface request errors")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{"LEDGER"}})
	require.Error(t, err, "QueryLedgers must surface request errors")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{})
	require.Error(t, err, "GetTradeVolume must surface request errors")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Volume: 1, Pair: spotTestPair})
	require.Error(t, err, "AddOrder must surface request errors")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER"})
	require.Error(t, err, "CancelExistingOrder must surface request errors")
}

func TestSpotResponseModels(t *testing.T) {
	var assets map[string]Asset
	require.NoError(t, json.Unmarshal([]byte(`{"BTC":{"aclass":"currency","collateral_value":"0.9","status":"enabled"}}`), &assets), "Asset must decode current fields")
	assert.Equal(t, "currency", assets["BTC"].AssetClass, "Asset should decode asset class")

	var order OrderInfo
	require.NoError(t, json.Unmarshal([]byte(`{"cl_ord_id":"CLIENT","descr":{"aclass":"tokenized_asset"},"time_in_force":"fok","trigger":"index","margin":true,"sender_sub_id":"SUB"}`), &order), "OrderInfo must decode current fields")
	assert.Equal(t, "CLIENT", order.ClientOrderID, "OrderInfo should decode client order ID")
	assert.Equal(t, "tokenized_asset", order.Description.AssetClass, "OrderInfo should decode description asset class")
	assert.Equal(t, "fok", order.TimeInForce, "OrderInfo should decode time-in-force")
	assert.Equal(t, "index", order.Trigger, "OrderInfo should decode trigger")
	assert.True(t, order.Margin, "OrderInfo should decode margin")
	assert.Equal(t, "SUB", order.SenderSubID, "OrderInfo should decode sender subaccount")

	var volume TradeVolumeResponse
	require.NoError(t, json.Unmarshal([]byte(`{"asset_class":"currency","inputs":{"domain_spot_volume_30d":"1","domain_futures_volume_30d":"2","domain_assets_on_platform":"3"},"volume_subaccounts":[{"iiban":"SUB","volume":"4"}],"schedules":[{"pair":"BTC/USD","class":"forex","tiers":[{"maker_fee":"0.1","taker_fee":"0.2","min_spot_volume":null,"min_futures_volume":"5","min_assets_on_platform":null,"active":false}]}]}`), &volume), "TradeVolumeResponse must decode current fields")
	assert.Equal(t, 2.0, volume.Inputs.FuturesVolume30D.Float64(), "TradeVolumeResponse should decode futures volume input")
	assert.Nil(t, volume.Schedules[0].Tiers[0].MinimumSpotVolume, "TradeVolumeResponse should decode a null spot threshold")
	assert.Equal(t, 5.0, volume.Schedules[0].Tiers[0].MinimumFuturesVolume.Float64(), "TradeVolumeResponse should decode futures threshold")
	assert.False(t, *volume.Schedules[0].Tiers[0].Active, "TradeVolumeResponse should decode an explicit false active flag")

	assert.Contains(t, errGroupedDepthInvalid.Error(), "omitted", "grouped depth error should describe omission as valid")
	assert.Contains(t, errGroupingInvalid.Error(), "omitted", "grouping error should describe omission as valid")
}
