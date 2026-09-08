package hyperliquid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

func TestLiveGetOpenOrdersForUser(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	_, err = e.GetOpenOrdersForUser(t.Context(), user)
	assert.NoError(t, err, "GetOpenOrdersForUser should succeed for accounts with or without orders")
}

func TestLiveGetHistoricalOrdersForUser(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	_, err = e.GetHistoricalOrdersForUser(t.Context(), user)
	assert.NoError(t, err, "GetHistoricalOrdersForUser should succeed for accounts with or without orders")
}

func TestLiveGetOrderStatusForUser(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	orders, err := e.GetHistoricalOrdersForUser(t.Context(), user)
	require.NoError(t, err, "GetHistoricalOrdersForUser must not error")
	if len(orders) == 0 {
		t.Skip("GetOrderStatusForUser requires an account with a historical order")
	}
	status, err := e.GetOrderStatusForUser(t.Context(), &OrderStatusRequest{User: user, OrderID: orders[0].Order.OrderID})
	require.NoError(t, err, "GetOrderStatusForUser must not error")
	require.NotNil(t, status, "status must not be nil")
	require.NotNil(t, status.Order, "status.Order must not be nil for a historical order")
	assert.Equal(t, orders[0].Order.OrderID, status.Order.Order.OrderID, "status.Order.Order.OrderID should identify the requested order")
}
