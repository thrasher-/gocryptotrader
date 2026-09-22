package binance

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func TestRemainingValidationSentinels(t *testing.T) {
	// Use the PR's mock transport for validation tests even if live tests are enabled.
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	require.NoError(t, testexch.MockHTTPInstance(local), "validation tests must use the existing mock transport")
	local.accountTypeIsUnified = new(bool)
	local.umAccountPositionMode = OneWayMode
	for _, countdown := range []int64{-1, 1, 4999} {
		_, err := local.SetOptionsAutoCancelAllOpenOrders(t.Context(), "BTCUSDT", countdown)
		assert.ErrorIs(t, err, errCountdownTimeTooSmall, "invalid countdown should retain its sentinel")
	}
	_, err := local.SetOptionsMarketMakerProtectionConfig(t.Context(), &MarketMakerProtectionConfig{Underlying: "BTCUSDT", WindowTimeInMilliseconds: 1000, QuantityLimit: 1})
	assert.ErrorIs(t, err, errNetDeltaLimitRequired, "missing delta limit should fail offline")
	_, err = local.GetFutureTickLevelOrderbookHistoricalDataDownloadLink(t.Context(), currency.NewBTCUSDT(), "", time.Time{}, time.Time{})
	assert.ErrorIs(t, err, errDataTypeRequired, "missing data type should fail offline")
	_, err = local.GetAllFuturesOrders(t.Context(), &GetAllFuturesOrdersRequest{Pair: currency.NewBTCUSD(), OrderID: 1})
	assert.ErrorIs(t, err, errInvalidOrderQueryCombination, "COIN-M pair and order ID should be mutually exclusive")
	_, err = local.SubmitOrder(t.Context(), &order.Submit{Exchange: local.Name, Pair: currency.NewBTCUSDT(), AssetType: asset.USDTMarginedFutures, Type: order.VolumeParticipation, Side: order.Buy, Amount: 1, ClientOrderID: "expired-order", EndTime: time.Now().Add(-time.Hour)})
	assert.ErrorIs(t, err, errEndTimeInThePast, "expired volume-participation order should fail offline")
	for _, a := range []asset.Item{asset.USDTMarginedFutures, asset.CoinMarginedFutures} {
		_, err = local.GetOrderHistory(t.Context(), &order.MultiOrderRequest{Pairs: currency.Pairs{currency.NewBTCUSDT()}, AssetType: a, Type: order.AnyType, Side: order.AnySide, StartTime: time.Now().Add(-8 * 24 * time.Hour), EndTime: time.Now()})
		assert.ErrorIs(t, err, errOrderHistoryWindowExceeded, "a futures query spanning over seven days should fail offline")
	}
}

func TestFuturesHistoryQueryWindows(t *testing.T) {
	local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, []string{"/fapi/v1/allOrders", "/dapi/v1/allOrders"}, r.URL.Path, "history should select a futures endpoint")
		_, err := w.Write([]byte(`[]`))
		assert.NoError(t, err, "empty history should write")
	})
	local.accountTypeIsUnified = new(bool)
	for _, a := range []asset.Item{asset.USDTMarginedFutures, asset.CoinMarginedFutures} {
		for _, tc := range []struct {
			name       string
			start, end time.Time
			fromID     string
		}{
			{"recent defaults", time.Time{}, time.Time{}, ""},
			{"old short interval", time.Now().Add(-40 * 24 * time.Hour), time.Now().Add(-39 * 24 * time.Hour), ""},
			{"start only", time.Now().Add(-24 * time.Hour), time.Time{}, ""},
			{"end only", time.Time{}, time.Now().Add(-24 * time.Hour), ""},
			{"time and order cursor", time.Now().Add(-24 * time.Hour), time.Now(), "9007199254740993"},
		} {
			t.Run(a.String()+"/"+tc.name, func(t *testing.T) {
				_, err := local.GetOrderHistory(t.Context(), &order.MultiOrderRequest{Pairs: currency.Pairs{currency.NewBTCUSDT()}, AssetType: a, Type: order.AnyType, Side: order.AnySide, StartTime: tc.start, EndTime: tc.end, FromOrderID: tc.fromID})
				assert.NoError(t, err, "documented optional history filters should be accepted")
			})
		}
	}
}
