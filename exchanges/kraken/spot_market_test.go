package kraken

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

var spotMarketFixtures = spotFixtureSet{results: map[string]string{
	"/0/public/Time":         `{"unixtime":1785636405,"rfc1123":"Sun,  2 Aug 26 05:06:45 +0000"}`,
	"/0/public/SystemStatus": `{"status":"online","timestamp":"2026-08-02T00:00:00Z"}`,
	"/0/public/Assets":       `{"BTC":{"aclass":"currency","altname":"XBT","decimals":10,"display_decimals":5,"collateral_value":"1","margin_rate":0.02,"status":"enabled"},"XXBT":{"aclass":"currency","altname":"XBT","decimals":10,"display_decimals":5,"status":"enabled"},"USD":{"aclass":"currency","altname":"USD","decimals":4,"display_decimals":2,"status":"enabled"}}`,
	"/0/public/AssetPairs":   `{"BTC/USD":{"altname":"XBTUSD","wsname":"XBT/USD","aclass_base":"currency","base":"BTC","aclass_quote":"currency","quote":"USD","execution_venue":"international","lot":"unit","cost_decimals":5,"pair_decimals":1,"lot_decimals":8,"lot_multiplier":1,"leverage_buy":[2],"leverage_sell":[2],"fees":[[0,0.26]],"fees_maker":[[0,0.16]],"fee_volume_currency":"USD","margin_call":80,"margin_stop":40,"ordermin":"0.0001","costmin":"0.5","tick_size":"0.1","status":"online","long_position_limit":"250","short_position_limit":"200"}}`,
	"/0/public/Ticker":       `{"BTC/USD":{"a":["101","1","2"],"b":["99","1","2"],"c":["100","1"],"v":["10","20"],"p":["99","100"],"t":[1,2],"l":["90","91"],"h":["110","111"],"o":"95"}}`,
	"/0/public/OHLC":         `{"BTC/USD":[[1695828271,"1","2","0.5","1.5","1.2","10",3]],"last":1695828271}`,
	"/0/public/Depth":        `{"BTC/USD":{"bids":[["1","2",1695828271]],"asks":[]}}`,
	"/0/public/Trades":       `{"BTC/USD":[["100","2",1695828271,"b","m","",61044952],["101","1",1695828272,"s","l","",61044953]],"last":"1695828273000000000"}`,
	"/0/public/Spread":       `{"BTC/USD":[[1695828271,"99","101"]],"last":1695828272}`,
	"/0/public/GroupedBook":  `{"pair":"XBTUSD","grouping":5,"bids":[{"price":"100","qty":"2"}],"asks":[]}`,
	"/0/private/Level3":      `{"pair":"XBTUSD","bids":[{"price":"100","qty":"2","order_id":"ORDER","timestamp":1765622008594292000}],"asks":[]}`,
	"/0/public/PreTrade":     `{"symbol":"BTC/USD","description":"Bitcoin / US Dollars","base_asset":"BTC","base_dti_code":"4H95J0R2X","base_dti_short_name":"BTC","base_notation":"UNIT","quote_asset":"USD","quote_dti_code":"4H95J0R2Y","quote_dti_short_name":"USD","quote_notation":"MONE","venue":"PGSL","system":"CLOB","bids":[{"side":"BUY","price":"100","qty":"2","count":1,"submission_ts":"2026-08-02T00:00:00Z","publication_ts":"2026-08-02T00:00:01Z"}],"asks":[]}`,
	"/0/public/PostTrade":    `{"last_ts":"2026-08-02T00:00:01Z","count":1,"trades":[{"trade_id":"TRADE","price":"100","quantity":"2","symbol":"BTC/USD","description":"Bitcoin / US Dollars","base_asset":"BTC","base_notation":"UNIT","quote_asset":"USD","quote_notation":"MONE","trade_venue":"PGSL","trade_ts":"2026-08-02T00:00:00Z","publication_venue":"PGSL","publication_ts":"2026-08-02T00:00:01Z"}]}`,
}}

func TestGetSystemStatus(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

	status, err := ex.GetSystemStatus(ctx)
	require.NoError(t, err, "GetSystemStatus must not error")
	require.NotNil(t, status, "GetSystemStatus must return a response")
	assert.Equal(t, "online", status.Status, "GetSystemStatus should decode status")
	responseJSON, err := json.Marshal(status)
	require.NoError(t, err, "GetSystemStatus must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"status":"online"`, "GetSystemStatus should decode the response")
	requireSpotRequest(t, requests, "/0/public/SystemStatus")

	status, err = newSpotNullResultExchange(t).GetSystemStatus(ctx)
	require.NoError(t, err, "GetSystemStatus must accept a null result")
	assert.Nil(t, status, "GetSystemStatus should return nil for a null result")
	status, err = newSpotErrorExchange(t).GetSystemStatus(ctx)
	require.ErrorIs(t, err, errSpotTransport, "GetSystemStatus must surface request errors")
	assert.Nil(t, status, "GetSystemStatus result should remain nil on request errors")
}

func TestGetGroupedOrderBook(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

	_, err := ex.GetGroupedOrderBook(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetGroupedOrderBook must reject a nil request")
	_, err = ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{})
	require.ErrorIs(t, err, errPairRequired, "GetGroupedOrderBook must require a pair")
	_, err = ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair, Depth: 1})
	require.ErrorIs(t, err, errGroupedDepthInvalid, "GetGroupedOrderBook must reject unsupported depth")
	_, err = ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair, Grouping: 2})
	require.ErrorIs(t, err, errGroupingInvalid, "GetGroupedOrderBook must reject unsupported grouping")
	assert.Contains(t, errGroupedDepthInvalid.Error(), "omitted", "errGroupedDepthInvalid should describe omission as valid")
	assert.Contains(t, errGroupingInvalid.Error(), "omitted", "errGroupingInvalid should describe omission as valid")
	_, err = new(Exchange).GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair})
	require.Error(t, err, "GetGroupedOrderBook must surface pair-format errors")
	for _, tc := range []struct {
		name     string
		depth    GroupedOrderBookDepth
		grouping OrderBookGrouping
	}{
		{name: "lower bounds", depth: 10, grouping: 1},
		{name: "upper bounds", depth: 1000, grouping: 1000},
	} {
		t.Run("grouped "+tc.name, func(t *testing.T) {
			_, err := ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair, Depth: tc.depth, Grouping: tc.grouping})
			require.NoError(t, err, "GetGroupedOrderBook must accept documented bounds")
			values := requireSpotRequest(t, requests, "/0/public/GroupedBook")
			assert.Equal(t, strconv.FormatUint(uint64(tc.depth), 10), values.Get("depth"), "GetGroupedOrderBook should encode documented depth")
			assert.Equal(t, strconv.FormatUint(uint64(tc.grouping), 10), values.Get("grouping"), "GetGroupedOrderBook should encode documented grouping")
		})
	}
	_, err = ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetGroupedOrderBook must allow optional parameters to be omitted")
	values := requireSpotRequest(t, requests, "/0/public/GroupedBook")
	assert.Empty(t, values.Get("depth"), "GetGroupedOrderBook should omit the default depth")
	assert.Empty(t, values.Get("grouping"), "GetGroupedOrderBook should omit the default grouping")
	grouped, err := ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair, Depth: 25, Grouping: 5})
	require.NoError(t, err, "GetGroupedOrderBook must not error")
	require.NotNil(t, grouped, "GetGroupedOrderBook must return a response")
	assert.Len(t, grouped.Bids, 1, "GetGroupedOrderBook should decode bids")
	responseJSON, err := json.Marshal(grouped)
	require.NoError(t, err, "GetGroupedOrderBook must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"pair":"XBTUSD"`, "GetGroupedOrderBook should decode the response")
	values = requireSpotRequest(t, requests, "/0/public/GroupedBook")
	assert.Equal(t, "25", values.Get("depth"), "GetGroupedOrderBook should encode depth")
	assert.Equal(t, "5", values.Get("grouping"), "GetGroupedOrderBook should encode grouping")

	grouped, err = newSpotNullResultExchange(t).GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetGroupedOrderBook must accept a null result")
	assert.Nil(t, grouped, "GetGroupedOrderBook should return nil for a null result")
	grouped, err = newSpotErrorExchange(t).GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair})
	require.ErrorIs(t, err, errSpotTransport, "GetGroupedOrderBook must surface request errors")
	assert.Nil(t, grouped, "GetGroupedOrderBook result should remain nil on request errors")
}

func TestQueryLevel3OrderBook(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

	_, err := ex.QueryLevel3OrderBook(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "QueryLevel3OrderBook must reject a nil request")
	_, err = ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{})
	require.ErrorIs(t, err, errPairRequired, "QueryLevel3OrderBook must require a pair")
	invalidDepth := Level3OrderBookDepth(1)
	_, err = ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair, Depth: &invalidDepth})
	require.ErrorIs(t, err, errLevel3DepthInvalid, "QueryLevel3OrderBook must reject unsupported depth")
	_, err = new(Exchange).QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair})
	require.Error(t, err, "QueryLevel3OrderBook must surface pair-format errors")
	for _, depth := range []Level3OrderBookDepth{Level3OrderBookDepthFull, Level3OrderBookDepth1000} {
		_, err := ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair, Depth: &depth})
		require.NoError(t, err, "QueryLevel3OrderBook must accept documented bounds")
		levelValues := requireSpotRequest(t, requests, "/0/private/Level3")
		assert.Equal(t, strconv.FormatUint(uint64(depth), 10), levelValues.Get("depth"), "QueryLevel3OrderBook should encode documented depth")
	}
	level3, err := ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair})
	require.NoError(t, err, "QueryLevel3OrderBook must not error")
	require.NotNil(t, level3, "QueryLevel3OrderBook must return a response")
	assert.Equal(t, int64(1765622008594292000), level3.Bids[0].Timestamp.Time().UnixNano(), "QueryLevel3OrderBook should retain nanosecond timestamps")
	responseJSON, err := json.Marshal(level3)
	require.NoError(t, err, "QueryLevel3OrderBook must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"pair":"XBTUSD"`, "QueryLevel3OrderBook should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/Level3")
	assert.Empty(t, values.Get("depth"), "QueryLevel3OrderBook should omit depth to use the server default")

	level3, err = newSpotNullResultExchange(t).QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair})
	require.NoError(t, err, "QueryLevel3OrderBook must accept a null result")
	assert.Nil(t, level3, "QueryLevel3OrderBook should return nil for a null result")
	level3, err = newSpotErrorExchange(t).QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair})
	require.ErrorIs(t, err, errSpotTransport, "QueryLevel3OrderBook must surface request errors")
	assert.Nil(t, level3, "QueryLevel3OrderBook result should remain nil on request errors")
}

func TestGetPreTradeData(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

	_, err := ex.GetPreTradeData(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetPreTradeData must reject a nil request")
	_, err = ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{})
	require.ErrorIs(t, err, errSymbolRequired, "GetPreTradeData must require a symbol")
	_, err = new(Exchange).GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: spotTestPair})
	require.Error(t, err, "GetPreTradeData must surface pair-format errors")
	_, err = ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: currency.NewPairWithDelimiter("A", "B", "")})
	require.ErrorIs(t, err, errSymbolLengthInvalid, "GetPreTradeData must reject a symbol shorter than three characters")
	_, err = ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: currency.NewPairWithDelimiter(strings.Repeat("A", 16), strings.Repeat("B", 17), "")})
	require.ErrorIs(t, err, errSymbolLengthInvalid, "GetPreTradeData must reject a symbol longer than thirty-two characters")
	for _, tc := range []struct {
		name           string
		pair           currency.Pair
		expectedSymbol string
	}{
		{name: "three characters", pair: currency.NewPairWithDelimiter("A", "BC", ""), expectedSymbol: "ABC"},
		{name: "thirty-two characters", pair: currency.NewPairWithDelimiter(strings.Repeat("A", 16), strings.Repeat("B", 16), ""), expectedSymbol: strings.Repeat("A", 16) + strings.Repeat("B", 16)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: tc.pair})
			require.NoError(t, err, "GetPreTradeData must accept documented symbol bounds")
			values := requireSpotRequest(t, requests, "/0/public/PreTrade")
			assert.Equal(t, tc.expectedSymbol, values.Get("symbol"), "GetPreTradeData should encode the boundary symbol")
		})
	}
	preTrade, err := ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetPreTradeData must not error")
	require.NotNil(t, preTrade, "GetPreTradeData must return a response")
	assert.Equal(t, "4H95J0R2X", preTrade.BaseDTICode, "GetPreTradeData should decode DTI metadata")
	assert.False(t, preTrade.Bids[0].SubmissionTimestamp.IsZero(), "GetPreTradeData should decode submission timestamps")
	responseJSON, err := json.Marshal(preTrade)
	require.NoError(t, err, "GetPreTradeData must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"symbol":"BTC/USD"`, "GetPreTradeData should decode the response")
	values := requireSpotRequest(t, requests, "/0/public/PreTrade")
	assert.Equal(t, "XBTUSD", values.Get("symbol"), "GetPreTradeData should encode symbol")

	preTrade, err = newSpotNullResultExchange(t).GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetPreTradeData must accept a null result")
	assert.Nil(t, preTrade, "GetPreTradeData should return nil for a null result")
	preTrade, err = newSpotErrorExchange(t).GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: spotTestPair})
	require.ErrorIs(t, err, errSpotTransport, "GetPreTradeData must surface request errors")
	assert.Nil(t, preTrade, "GetPreTradeData result should remain nil on request errors")
}

func TestGetPostTradeData(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

	_, err := ex.GetPostTradeData(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetPostTradeData must reject a nil request")
	_, err = ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{Count: 1001})
	require.ErrorIs(t, err, errPostTradeCountTooLarge, "GetPostTradeData must enforce Kraken's result limit")
	from := time.Date(2026, 8, 1, 11, 2, 3, 4, time.FixedZone("AEST", 10*60*60))
	to := from.Add(time.Hour)
	_, err = ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{FromTimestamp: to, ToTimestamp: from})
	require.ErrorIs(t, err, errTimeRangeInvalid, "GetPostTradeData must reject a reversed time range")
	_, err = ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{Pair: currency.Pair{Base: currency.BTC}})
	require.ErrorIs(t, err, errPairRequired, "GetPostTradeData must reject a partially populated pair")
	_, err = new(Exchange).GetPostTradeData(ctx, &GetPostTradeDataRequest{Pair: spotTestPair})
	require.Error(t, err, "GetPostTradeData must surface pair-format errors")
	postTrade, err := ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{Pair: spotTestPair, FromTimestamp: from, ToTimestamp: to, Count: 100})
	require.NoError(t, err, "GetPostTradeData must not error")
	require.NotNil(t, postTrade, "GetPostTradeData must return a response")
	assert.Equal(t, uint64(1), postTrade.Count, "GetPostTradeData should decode the trade count")
	responseJSON, err := json.Marshal(postTrade)
	require.NoError(t, err, "GetPostTradeData must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"trade_id":"TRADE"`, "GetPostTradeData should decode the response")
	values := requireSpotRequest(t, requests, "/0/public/PostTrade")
	assert.Equal(t, "XBTUSD", values.Get("symbol"), "GetPostTradeData should encode the formatted pair")
	assert.Equal(t, from.UTC().Format(time.RFC3339Nano), values.Get("from_ts"), "GetPostTradeData should encode the start timestamp in UTC")
	assert.Equal(t, to.UTC().Format(time.RFC3339Nano), values.Get("to_ts"), "GetPostTradeData should encode the end timestamp in UTC")
	assert.Equal(t, "100", values.Get("count"), "GetPostTradeData should encode count")

	postTrade, err = newSpotNullResultExchange(t).GetPostTradeData(ctx, &GetPostTradeDataRequest{})
	require.NoError(t, err, "GetPostTradeData must accept a null result")
	assert.Nil(t, postTrade, "GetPostTradeData should return nil for a null result")
	postTrade, err = newSpotErrorExchange(t).GetPostTradeData(ctx, &GetPostTradeDataRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetPostTradeData must surface request errors")
	assert.Nil(t, postTrade, "GetPostTradeData result should remain nil on request errors")
}

func TestGetAssets(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

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
	for _, value := range []AssetClass{AssetClassCurrency, AssetClassTokenizedAsset} {
		t.Run("asset class "+string(value), func(t *testing.T) {
			_, err := ex.GetAssets(t.Context(), &GetAssetsRequest{AssetClass: value})
			require.NoError(t, err, "GetAssets must accept the documented asset class")
			values := requireSpotRequest(t, requests, "/0/public/Assets")
			assert.Equal(t, string(value), values.Get("aclass"), "GetAssets should encode the documented asset class")
		})
	}

	assets, err = newSpotErrorExchange(t).GetAssets(ctx, &GetAssetsRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetAssets must surface request errors")
	assert.Nil(t, assets, "GetAssets result should remain nil on request errors")
}

func TestGetAssetPairs(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

	_, err := ex.GetAssetPairs(ctx, nil)
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
	values := requireSpotRequest(t, requests, "/0/public/AssetPairs")
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
	for _, value := range []AssetPairInfo{AssetPairInfoAll, AssetPairInfoLeverage, AssetPairInfoFees, AssetPairInfoMargin} {
		t.Run("asset pair info "+string(value), func(t *testing.T) {
			_, err := ex.GetAssetPairs(t.Context(), &GetAssetPairsRequest{Info: value})
			require.NoError(t, err, "GetAssetPairs must accept the documented info filter")
			values := requireSpotRequest(t, requests, "/0/public/AssetPairs")
			assert.Equal(t, string(value), values.Get("info"), "GetAssetPairs should encode the documented info filter")
		})
	}
	for _, value := range []AssetClass{AssetClassCurrency, AssetClassTokenizedAsset} {
		t.Run("asset pair base class "+string(value), func(t *testing.T) {
			_, err := ex.GetAssetPairs(t.Context(), &GetAssetPairsRequest{AssetClassBase: value})
			require.NoError(t, err, "GetAssetPairs must accept the documented base asset class")
			values := requireSpotRequest(t, requests, "/0/public/AssetPairs")
			assert.Equal(t, string(value), values.Get("aclass_base"), "GetAssetPairs should encode the documented base asset class")
		})
	}
	for _, value := range []ExecutionVenue{ExecutionVenueInternational, ExecutionVenueBitnomial} {
		t.Run("execution venue "+string(value), func(t *testing.T) {
			_, err := ex.GetAssetPairs(t.Context(), &GetAssetPairsRequest{ExecutionVenue: value})
			require.NoError(t, err, "GetAssetPairs must accept the documented execution venue")
			values := requireSpotRequest(t, requests, "/0/public/AssetPairs")
			assert.Equal(t, string(value), values.Get("execution_venue"), "GetAssetPairs should encode the documented execution venue")
		})
	}

	pairs, err = newSpotErrorExchange(t).GetAssetPairs(ctx, &GetAssetPairsRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetAssetPairs must surface request errors")
	assert.Nil(t, pairs, "GetAssetPairs result should remain nil on request errors")
}

func TestGetTicker(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

	_, err := ex.GetTicker(ctx, nil)
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
	values := requireSpotRequest(t, requests, "/0/public/Ticker")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "GetTicker should encode pairs")
	assert.Equal(t, "forex", values.Get("asset_class"), "GetTicker should encode asset class")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetTicker should encode assetVersion")
	_, err = ex.GetTicker(ctx, &GetTickerRequest{})
	require.NoError(t, err, "GetTicker must allow omitted filters")
	requireSpotRequest(t, requests, "/0/public/Ticker")
	for _, value := range []AssetClass{AssetClassTokenizedAsset, AssetClassForex} {
		t.Run("asset class "+string(value), func(t *testing.T) {
			_, err := ex.GetTicker(t.Context(), &GetTickerRequest{AssetClass: value})
			require.NoError(t, err, "GetTicker must accept the documented asset class")
			values := requireSpotRequest(t, requests, "/0/public/Ticker")
			assert.Equal(t, string(value), values.Get("asset_class"), "GetTicker should encode the documented asset class")
		})
	}

	tickers, err = newSpotErrorExchange(t).GetTicker(ctx, &GetTickerRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetTicker must surface request errors")
	assert.Nil(t, tickers, "GetTicker result should remain nil on request errors")
}

func TestGetOHLC(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()
	preEpoch := time.Unix(-1, 0)

	_, err := ex.GetOHLC(ctx, nil)
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
	require.NotNil(t, ohlc, "GetOHLC must return a response")
	assert.Equal(t, uint64(3), ohlc.Candles["BTC/USD"][0].Count, "GetOHLC should decode candle count")
	responseJSON, err := json.Marshal(ohlc)
	require.NoError(t, err, "GetOHLC must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"Candles":{"BTC/USD":[{`, "GetOHLC should decode the response")
	values := requireSpotRequest(t, requests, "/0/public/OHLC")
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
	for _, value := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 4 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour, 15 * 24 * time.Hour} {
		t.Run(value.String(), func(t *testing.T) {
			_, err := ex.GetOHLC(t.Context(), &GetOHLCRequest{Pair: spotTestPair, Interval: value})
			require.NoError(t, err, "GetOHLC must accept the documented interval")
			values := requireSpotRequest(t, requests, "/0/public/OHLC")
			assert.Equal(t, strconv.FormatInt(int64(value/time.Minute), 10), values.Get("interval"), "GetOHLC should encode the documented interval")
		})
	}

	ohlc, err = newSpotNullResultExchange(t).GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetOHLC must accept a null result")
	assert.Nil(t, ohlc, "GetOHLC should return nil for a null result")
	ohlc, err = newSpotErrorExchange(t).GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair})
	require.ErrorIs(t, err, errSpotTransport, "GetOHLC must surface request errors")
	assert.Nil(t, ohlc, "GetOHLC result should remain nil on request errors")
}

func TestGetDepth(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()

	_, err := ex.GetDepth(ctx, nil)
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
	values := requireSpotRequest(t, requests, "/0/public/Depth")
	assert.Equal(t, "500", values.Get("count"), "GetDepth should encode count")
	assert.Equal(t, "tokenized_asset", values.Get("asset_class"), "GetDepth should encode asset class")
	assert.Equal(t, "1", values.Get("assetVersion"), "GetDepth should encode assetVersion")
	_, err = ex.GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetDepth must allow count to be omitted")
	values = requireSpotRequest(t, requests, "/0/public/Depth")
	assert.Empty(t, values.Get("count"), "GetDepth should omit the default count")

	book, err = newSpotErrorExchange(t).GetDepth(ctx, &GetDepthRequest{Pair: spotTestPair})
	require.ErrorIs(t, err, errSpotTransport, "GetDepth must surface request errors")
	assert.Nil(t, book, "GetDepth result should remain nil on request errors")
}

func TestGetTrades(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()
	preEpoch := time.Unix(-1, 0)

	_, err := ex.GetTrades(ctx, nil)
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
	since := time.Unix(1695828270, 0)
	trades, err := ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair, Since: since, Count: 1000, AssetClass: AssetClassTokenizedAsset, AssetVersion: AssetVersionDisplay})
	require.NoError(t, err, "GetTrades must not error")
	require.NotNil(t, trades, "GetTrades must return a response")
	assert.Equal(t, 100.0, trades.Trades["BTC/USD"][0].Price.Float64(), "GetTrades should decode trade price")
	responseJSON, err := json.Marshal(trades)
	require.NoError(t, err, "GetTrades must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"Trades":{"BTC/USD":[{`, "GetTrades should decode the response")
	values := requireSpotRequest(t, requests, "/0/public/Trades")
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

	trades, err = newSpotNullResultExchange(t).GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetTrades must accept a null result")
	assert.Nil(t, trades, "GetTrades should return nil for a null result")
	trades, err = newSpotErrorExchange(t).GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair})
	require.ErrorIs(t, err, errSpotTransport, "GetTrades must surface request errors")
	assert.Nil(t, trades, "GetTrades result should remain nil on request errors")
}

func TestGetSpread(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	ctx := t.Context()
	preEpoch := time.Unix(-1, 0)

	_, err := ex.GetSpread(ctx, nil)
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
	since := time.Unix(1695828270, 0)
	spread, err := ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair, Since: since, AssetClass: AssetClassTokenizedAsset, AssetVersion: AssetVersionDisplay})
	require.NoError(t, err, "GetSpread must not error")
	require.NotNil(t, spread, "GetSpread must return a response")
	assert.Equal(t, 99.0, spread.Spreads["BTC/USD"][0].Bid.Float64(), "GetSpread should decode bid price")
	responseJSON, err := json.Marshal(spread)
	require.NoError(t, err, "GetSpread must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"Spreads":{"BTC/USD":[{`, "GetSpread should decode the response")
	values := requireSpotRequest(t, requests, "/0/public/Spread")
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

	spread, err = newSpotNullResultExchange(t).GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetSpread must accept a null result")
	assert.Nil(t, spread, "GetSpread should return nil for a null result")
	spread, err = newSpotErrorExchange(t).GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair})
	require.ErrorIs(t, err, errSpotTransport, "GetSpread must surface request errors")
	assert.Nil(t, spread, "GetSpread result should remain nil on request errors")
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

func TestGetCurrentServerTime(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotMarketFixtures)
	result, err := ex.GetCurrentServerTime(t.Context())
	require.NoError(t, err, "GetCurrentServerTime must not error")
	require.NotNil(t, result, "GetCurrentServerTime must decode a response")
	assert.Equal(t, int64(1785636405), result.Unixtime.Time().Unix(), "GetCurrentServerTime should decode Unix time")
	requireSpotRequest(t, requests, "/0/public/Time")
	result, err = newSpotNullResultExchange(t).GetCurrentServerTime(t.Context())
	require.NoError(t, err, "GetCurrentServerTime must accept a null result")
	assert.Nil(t, result, "GetCurrentServerTime should return nil for a null result")
	result, err = newSpotErrorExchange(t).GetCurrentServerTime(t.Context())
	require.ErrorIs(t, err, errSpotTransport, "GetCurrentServerTime must surface request errors")
	assert.Nil(t, result, "GetCurrentServerTime should return nil after a request error")
}

func TestAssetUnmarshalJSON(t *testing.T) {
	var assets map[string]Asset
	require.NoError(t, json.Unmarshal([]byte(`{"BTC":{"aclass":"currency","collateral_value":"0.9","status":"enabled"}}`), &assets), "Asset must decode current fields")
	assert.Equal(t, "currency", assets["BTC"].AssetClass, "Asset should decode asset class")
}

func TestAssetTranslatorStore(t *testing.T) {
	t.Parallel()
	var translator assetTranslatorStore
	assert.False(t, translator.Seeded(), "Seeded should report an empty store")
	assert.Empty(t, translator.LookupAltName("ZUSD"), "LookupAltName should return empty for an unseeded currency")
	assert.Empty(t, translator.LookupCurrency("USD"), "LookupCurrency should return empty for an unseeded alternate name")

	translator.Seed("ZUSD", "USD")
	assert.True(t, translator.Seeded(), "Seeded should report a populated store")
	assert.Equal(t, "USD", translator.LookupAltName("ZUSD"), "LookupAltName should return the alternate name")
	assert.Equal(t, "ZUSD", translator.LookupCurrency("USD"), "LookupCurrency should return the original currency")
	assert.Empty(t, translator.LookupCurrency("EUR"), "LookupCurrency should return empty for an unknown alternate name")

	translator.Seed("ZUSD", "BLA")
	assert.Equal(t, "USD", translator.LookupAltName("ZUSD"), "Seed should preserve an existing translation")
}

func TestSeedAssets(t *testing.T) {
	assetTranslator.l.Lock()
	originalAssets := assetTranslator.Assets
	assetTranslator.Assets = nil
	assetTranslator.l.Unlock()
	t.Cleanup(func() {
		assetTranslator.l.Lock()
		assetTranslator.Assets = originalAssets
		assetTranslator.l.Unlock()
	})

	ex, _ := newSpotEndpointExchange(t, spotMarketFixtures)
	err := ex.SeedAssets(t.Context())
	require.NoError(t, err, "SeedAssets must not error")

	assert.Equal(t, "XBT", assetTranslator.LookupAltName("BTC"), "LookupAltName should return the canonical asset alternate name")
	assert.Equal(t, "XBT", assetTranslator.LookupAltName("XXBT"), "LookupAltName should return the legacy asset alternate name")
	assert.Contains(t, []string{"BTC", "XXBT"}, assetTranslator.LookupCurrency("XBT"), "LookupCurrency should return an original asset for a shared alternate name")
	assert.Equal(t, "XBTUSD", assetTranslator.LookupAltName("BTC/USD"), "LookupAltName should return the pair alternate name")
	assert.Equal(t, "BTC/USD", assetTranslator.LookupCurrency("XBTUSD"), "LookupCurrency should return the original pair")

	err = newSpotErrorExchange(t).SeedAssets(t.Context())
	require.ErrorIs(t, err, errSpotTransport, "SeedAssets must surface asset request errors")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/0/public/Assets" {
			_, _ = w.Write([]byte(`{"error":[],"result":{"BTC":{"altname":"XBT"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":["EGeneral:pair failure"],"result":{}}`))
	}))
	t.Cleanup(server.Close)
	err = newAuthenticatedSpotExchange(t, server.URL).SeedAssets(t.Context())
	require.ErrorContains(t, err, "pair failure", "SeedAssets must surface asset-pair request errors")
}
