package hyperliquid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

func TestLiveAccountInfo(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")

	t.Run("Role", func(t *testing.T) {
		role, err := e.GetUserRole(t.Context(), user)
		require.NoError(t, err, "GetUserRole must not error")
		require.NotNil(t, role, "role must not be nil")
		assert.NotEmpty(t, role.Role, "role.Role should be populated")
	})
	t.Run("Abstraction", func(t *testing.T) {
		mode, err := e.GetUserAbstraction(t.Context(), user)
		require.NoError(t, err, "GetUserAbstraction must not error")
		assert.Contains(t, []AccountAbstraction{
			AccountAbstractionDefault, AccountAbstractionDisabled, AccountAbstractionDEX,
			AccountAbstractionUnified, AccountAbstractionPortfolio,
		}, mode, "mode should be a supported AccountAbstraction")
	})
	t.Run("SpotBalances", func(t *testing.T) {
		state, err := e.GetSpotClearinghouseState(t.Context(), user)
		require.NoError(t, err, "GetSpotClearinghouseState must not error")
		assert.NotNil(t, state, "state should not be nil even without spot balances")
	})
	t.Run("PerpetualBalances", func(t *testing.T) {
		state, err := e.GetClearinghouseState(t.Context(), user)
		require.NoError(t, err, "GetClearinghouseState must not error")
		assert.NotNil(t, state, "state should not be nil even without perpetual positions")
	})
	t.Run("Fees", func(t *testing.T) {
		fees, err := e.GetUserFees(t.Context(), user)
		require.NoError(t, err, "GetUserFees must not error")
		assert.NotNil(t, fees, "fees should not be nil")
	})
	t.Run("OpenOrders", func(t *testing.T) {
		_, err := e.GetOpenOrdersForUser(t.Context(), user)
		assert.NoError(t, err, "GetOpenOrdersForUser should succeed for accounts with or without orders")
	})
	t.Run("OrderHistory", func(t *testing.T) {
		orders, err := e.GetHistoricalOrdersForUser(t.Context(), user)
		require.NoError(t, err, "GetHistoricalOrdersForUser must not error")
		if len(orders) == 0 {
			return
		}
		status, err := e.GetOrderStatusForUser(t.Context(), user, orders[0].Order.OrderID)
		require.NoError(t, err, "GetOrderStatusForUser must not error")
		require.NotNil(t, status, "status must not be nil")
		require.NotNil(t, status.Order, "status.Order must not be nil for a historical order")
		assert.Equal(t, orders[0].Order.OrderID, status.Order.Order.OrderID, "status.Order.Order.OrderID should identify the requested order")
	})
	t.Run("LedgerHistory", func(t *testing.T) {
		end := time.Now().UTC()
		_, err := e.GetUserNonFundingLedgerUpdates(t.Context(), user, end.Add(-24*time.Hour), end)
		assert.NoError(t, err, "GetUserNonFundingLedgerUpdates should succeed for accounts with or without recent transfers")
	})
}
