package hyperliquid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

func TestGetRateLimits(t *testing.T) {
	t.Parallel()
	limits := GetRateLimits()
	require.Contains(t, limits, infoStandardEPL, "GetRateLimits must configure the standard info endpoint limit")
	require.Contains(t, limits, infoLightEPL, "GetRateLimits must configure the light info endpoint limit")
	require.Contains(t, limits, infoRecentTradesEPL, "GetRateLimits must configure the recent trades endpoint limit")
	require.Contains(t, limits, infoFundingHistoryEPL, "GetRateLimits must configure the funding history endpoint limit")
	require.Contains(t, limits, infoUserLedgerEPL, "GetRateLimits must configure the user-ledger endpoint")
	require.Contains(t, limits, candleEndpointLimit(maximumCandleCount), "GetRateLimits must configure the maximum candle endpoint limit")
	assert.Equal(t, 21, recentTradesWeight, "recentTradesWeight should reserve one response-size weight bucket")
	assert.Equal(t, 45, fundingHistoryWeight, "fundingHistoryWeight should reserve all 500 response-size buckets")
	assert.Equal(t, 45, userLedgerHistoryWeight, "userLedgerHistoryWeight should reserve all 500 response-size buckets")
}

func TestCandleEndpointLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		count uint64
		want  request.EndpointLimit
	}{
		{name: "zero defaults to one", count: 0, want: 1},
		{name: "one", count: 1, want: 1},
		{name: "weight boundary", count: 60, want: 1},
		{name: "next weight", count: 61, want: 2},
		{name: "maximum", count: maximumCandleCount, want: 84},
		{name: "clamped", count: maximumCandleCount + 1, want: 84},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, candleEPLBase+tc.want, candleEndpointLimit(tc.count), "candleEndpointLimit should match its weighted request bucket")
		})
	}
}

func TestFormatInterval(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		interval kline.Interval
		want     string
	}{
		{interval: kline.OneMin, want: "1m"},
		{interval: kline.ThreeMin, want: "3m"},
		{interval: kline.FiveMin, want: "5m"},
		{interval: kline.FifteenMin, want: "15m"},
		{interval: kline.ThirtyMin, want: "30m"},
		{interval: kline.OneHour, want: "1h"},
		{interval: kline.TwoHour, want: "2h"},
		{interval: kline.FourHour, want: "4h"},
		{interval: kline.EightHour, want: "8h"},
		{interval: kline.TwelveHour, want: "12h"},
		{interval: kline.OneDay, want: "1d"},
		{interval: kline.ThreeDay, want: "3d"},
		{interval: kline.OneWeek, want: "1w"},
		{interval: kline.OneMonth, want: "1M"},
	} {
		got, err := formatInterval(tc.interval)
		require.NoError(t, err, "formatInterval must not error for a supported interval")
		assert.Equal(t, tc.want, got, "got should match the Hyperliquid API interval")
	}

	_, err := formatInterval(kline.Interval(42))
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "formatInterval must return the expected error for an unsupported interval")
}
