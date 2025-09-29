package kline

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

func TestCreateKlineRequest(t *testing.T) {
	t.Parallel()
	_, err := CreateKlineRequest("", currency.EMPTYPAIR, currency.EMPTYPAIR, 0, 0, 0, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, ErrUnsetName)

	_, err = CreateKlineRequest("name", currency.EMPTYPAIR, currency.EMPTYPAIR, 0, 0, 0, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	pair := currency.NewBTCUSDT()
	_, err = CreateKlineRequest("name", pair, currency.EMPTYPAIR, 0, 0, 0, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	pair2 := pair.Upper()
	_, err = CreateKlineRequest("name", pair, pair2, 0, 0, 0, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = CreateKlineRequest("name", pair, pair2, asset.Spot, 0, 0, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, ErrInvalidInterval)

	_, err = CreateKlineRequest("name", pair, pair2, asset.Spot, OneHour, 0, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, ErrInvalidInterval)

	_, err = CreateKlineRequest("name", pair, pair2, asset.Spot, OneHour, OneMin, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, common.ErrDateUnset)

	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = CreateKlineRequest("name", pair, pair2, asset.Spot, OneHour, OneMin, start, time.Time{}, 0)
	require.ErrorIs(t, err, common.ErrDateUnset)

	end := start.AddDate(0, 0, 1)
	_, err = CreateKlineRequest("name", pair, pair2, asset.Spot, OneHour, OneMin, start, end, 0)
	require.ErrorIs(t, err, errInvalidSpecificEndpointLimit)

	r, err := CreateKlineRequest("name", pair, pair2, asset.Spot, OneHour, OneMin, start, end, 1)
	require.NoError(t, err)

	require.Equal(t, "name", r.Exchange, "Request.Exchange must match expected value")
	require.True(t, r.Pair.Equal(pair), "Request.Pair must equal expected pair")
	require.Equal(t, asset.Spot, r.Asset, "Request.Asset must match expected asset type")
	require.Equal(t, OneMin, r.ExchangeInterval, "Request.ExchangeInterval must match expected value")
	require.Equal(t, OneHour, r.ClientRequired, "Request.ClientRequired must match expected interval")
	require.Equal(t, start, r.Start, "Request.Start must match expected start time")
	require.Equal(t, end, r.End, "Request.End must match expected end time")
	require.Equal(t, "BTCUSDT", r.RequestFormatted.String(), "Request.RequestFormatted must return expected string")

	// Check end date/time shift if the request time is mid candle and not
	// aligned correctly.
	end = end.Round(0)
	end = end.Add(time.Second * 30)
	r, err = CreateKlineRequest("name", pair, pair2, asset.Spot, OneHour, OneMin, start, end, 1)
	require.NoError(t, err)

	require.True(t, r.End.Equal(end.Add(OneHour.Duration()-(time.Second*30))), "Request.End must align to expected candle close")
}

func TestGetRanges(t *testing.T) {
	t.Parallel()

	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	pair := currency.NewBTCUSDT()

	var r *Request
	_, err := r.GetRanges(100)
	require.ErrorIs(t, err, errNilRequest)

	r, err = CreateKlineRequest("name", pair, pair, asset.Spot, OneHour, OneMin, start, end, 1)
	require.NoError(t, err)

	holder, err := r.GetRanges(100)
	require.NoError(t, err)

	require.Len(t, holder.Ranges, 15, "GetRanges must return expected range count")
}

var protecThyCandles sync.Mutex

func getOneMinute() []Candle {
	protecThyCandles.Lock()
	candles := make([]Candle, len(oneMinuteCandles))
	copy(candles, oneMinuteCandles)
	protecThyCandles.Unlock()
	return candles
}

var oneMinuteCandles = func() []Candle {
	var candles []Candle
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for x := range 1442 { // two extra candles.
		candles = append(candles, Candle{
			Time:   start,
			Volume: 1,
			Open:   1,
			High:   float64(1 + x),
			Low:    float64(-(1 + x)),
			Close:  1,
		})
		start = start.Add(time.Minute)
	}
	return candles
}()

func getOneHour() []Candle {
	protecThyCandles.Lock()
	candles := make([]Candle, len(oneHourCandles))
	copy(candles, oneHourCandles)
	protecThyCandles.Unlock()
	return candles
}

var oneHourCandles = func() []Candle {
	var candles []Candle
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for x := range 24 {
		candles = append(candles, Candle{
			Time:   start,
			Volume: 1,
			Open:   1,
			High:   float64(1 + x),
			Low:    float64(-(1 + x)),
			Close:  1,
		})
		start = start.Add(time.Hour)
	}
	return candles
}()

func TestRequest_ProcessResponse(t *testing.T) {
	t.Parallel()

	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	pair := currency.NewBTCUSDT()

	var r *Request
	_, err := r.ProcessResponse(nil)
	require.ErrorIs(t, err, errNilRequest)

	r = &Request{}
	_, err = r.ProcessResponse(nil)
	require.ErrorIs(t, err, ErrNoTimeSeriesDataToConvert)

	_, err = CreateKlineRequest("name", pair, pair, asset.Spot, OneHour, OneHour, start, end, 0)
	require.ErrorIs(t, err, errInvalidSpecificEndpointLimit)

	// no conversion
	r, err = CreateKlineRequest("name", pair, pair, asset.Spot, OneHour, OneHour, start, end, 1)
	require.NoError(t, err)

	holder, err := r.ProcessResponse(getOneHour())
	require.NoError(t, err)

	require.Len(t, holder.Candles, 24, "ProcessResponse must return expected candle count without conversion")

	// with conversion
	r, err = CreateKlineRequest("name", pair, pair, asset.Spot, OneHour, OneMin, start, end, 1)
	require.NoError(t, err)

	holder, err = r.ProcessResponse(getOneMinute())
	require.NoError(t, err)

	require.Len(t, holder.Candles, 24, "ProcessResponse must return expected candle count with conversion")

	// Potential partial candle
	end = time.Now().UTC()
	start = end.AddDate(0, 0, -5).Truncate(time.Duration(OneDay))
	r, err = CreateKlineRequest("name", pair, pair, asset.Spot, OneDay, OneDay, start, end, 1)
	require.NoError(t, err)

	require.True(t, r.PartialCandle, "Request.PartialCandle must be true when end is mid candle")

	hasIncomplete := []Candle{
		{Time: start, Close: 1},
		{Time: start.Add(OneDay.Duration()), Close: 2},
		{Time: start.Add(OneDay.Duration() * 2), Close: 3},
		{Time: start.Add(OneDay.Duration() * 3), Close: 4},
		{Time: start.Add(OneDay.Duration() * 4), Close: 5},
		{Time: start.Add(OneDay.Duration() * 5), Close: 5.5},
	}

	sweetItem, err := r.ProcessResponse(hasIncomplete)
	require.NoError(t, err)

	require.Equal(t, PartialCandle, sweetItem.Candles[len(sweetItem.Candles)-1].ValidationIssues, "ProcessResponse must mark last candle as partial when incomplete data provided")

	missingIncomplete := []Candle{
		{Time: start, Close: 1},
		{Time: start.Add(OneDay.Duration()), Close: 2},
		{Time: start.Add(OneDay.Duration() * 2), Close: 3},
		{Time: start.Add(OneDay.Duration() * 3), Close: 4},
		{Time: start.Add(OneDay.Duration() * 4), Close: 5},
	}

	sweetItem, err = r.ProcessResponse(missingIncomplete)
	require.NoError(t, err)

	require.NotEqual(t, PartialCandle, sweetItem.Candles[len(sweetItem.Candles)-1].ValidationIssues, "ProcessResponse must not mark last candle partial when data complete")

	// end date far into the dark depths of future reality
	r, err = CreateKlineRequest("name", pair, pair, asset.Spot, OneDay, OneDay, start, end.AddDate(1, 0, 0), 1)
	require.NoError(t, err)

	sweetItem, err = r.ProcessResponse(hasIncomplete)
	require.NoError(t, err)

	require.Equal(t, PartialCandle, sweetItem.Candles[len(sweetItem.Candles)-1].ValidationIssues, "ProcessResponse must flag partial candle when end date extends beyond data")

	sweetItem, err = r.ProcessResponse(missingIncomplete)
	require.NoError(t, err)

	require.Len(t, sweetItem.Candles, 5, "ProcessResponse must retain expected candle count when data shorter than range")

	require.NotEqual(t, PartialCandle, sweetItem.Candles[len(sweetItem.Candles)-1].ValidationIssues, "ProcessResponse must not flag partial candle when data aligns with range")

	laterEndDate := end.AddDate(1, 0, 0).UTC().Truncate(time.Duration(OneDay)).Add(-time.Duration(OneDay))
	require.False(t, sweetItem.Candles[len(sweetItem.Candles)-1].Time.Equal(laterEndDate), "ProcessResponse must adjust candle end time when truncated")
}

func TestExtendedRequest_ProcessResponse(t *testing.T) {
	t.Parallel()
	ohc := getOneHour()
	start := ohc[0].Time
	end := ohc[len(ohc)-1].Time.Add(OneHour.Duration())
	pair := currency.NewBTCUSDT()

	var rExt *ExtendedRequest
	_, err := rExt.ProcessResponse(nil)
	require.ErrorIs(t, err, errNilRequest)

	rExt = &ExtendedRequest{}
	_, err = rExt.ProcessResponse(nil)
	require.ErrorIs(t, err, ErrNoTimeSeriesDataToConvert)

	// no conversion
	r, err := CreateKlineRequest("name", pair, pair, asset.Spot, OneHour, OneHour, start, end, 1)
	require.NoError(t, err)

	r.ProcessedCandles = ohc
	dates, err := r.GetRanges(100)
	require.NoError(t, err)

	rExt = &ExtendedRequest{r, dates}

	holder, err := rExt.ProcessResponse(ohc)
	require.NoError(t, err)

	require.Len(t, holder.Candles, 24, "ExtendedRequest.ProcessResponse must return expected candle count without conversion")

	// with conversion
	ohc = getOneMinute()
	r, err = CreateKlineRequest("name", pair, pair, asset.Spot, OneHour, OneMin, start, end, 1)
	require.NoError(t, err)

	dates, err = r.GetRanges(100)
	require.NoError(t, err)

	r.IsExtended = true
	rExt = &ExtendedRequest{r, dates}
	holder, err = rExt.ProcessResponse(ohc)
	require.NoError(t, err)

	require.Len(t, holder.Candles, 24, "ExtendedRequest.ProcessResponse must return expected candle count with conversion")
}

func TestExtendedRequest_Size(t *testing.T) {
	t.Parallel()

	var rExt *ExtendedRequest
	require.Zero(t, rExt.Size(), "ExtendedRequest.Size must return zero for nil request")

	rExt = &ExtendedRequest{RangeHolder: &IntervalRangeHolder{Limit: 100, Ranges: []IntervalRange{{}, {}}}}
	require.Equal(t, uint64(200), rExt.Size(), "ExtendedRequest.Size must multiply ranges by limit")
}

func TestRequest_Size(t *testing.T) {
	t.Parallel()

	var r *Request
	require.Zero(t, r.Size(), "Request.Size must return zero for nil request")

	r = &Request{
		Start:            time.Now().Add(-time.Hour * 2).Truncate(time.Hour),
		End:              time.Now().Truncate(time.Hour),
		ExchangeInterval: OneHour,
	}
	require.Equal(t, uint64(2), r.Size(), "Request.Size must return expected value for populated request")
}
