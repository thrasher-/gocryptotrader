package hyperliquid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

func TestLiveGetUserFees(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	fees, err := e.GetUserFees(t.Context(), user)
	require.NoError(t, err, "GetUserFees must not error")
	assert.NotNil(t, fees, "fees should not be nil")
}

func TestLiveGetUserRole(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	role, err := e.GetUserRole(t.Context(), user)
	require.NoError(t, err, "GetUserRole must not error")
	require.NotNil(t, role, "role must not be nil")
	assert.NotEmpty(t, role.Role, "role.Role should be populated")
}

func TestLiveGetSpotClearinghouseState(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	state, err := e.GetSpotClearinghouseState(t.Context(), user)
	require.NoError(t, err, "GetSpotClearinghouseState must not error")
	assert.NotNil(t, state, "state should not be nil even without spot balances")
}

func TestLiveGetUserAbstraction(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	mode, err := e.GetUserAbstraction(t.Context(), user)
	require.NoError(t, err, "GetUserAbstraction must not error")
	assert.Contains(t, []AccountAbstraction{
		AccountAbstractionDefault, AccountAbstractionDisabled, AccountAbstractionDEX,
		AccountAbstractionUnified, AccountAbstractionPortfolio,
	}, mode, "mode should be a supported AccountAbstraction")
}

func TestLiveGetClearinghouseState(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	state, err := e.GetClearinghouseState(t.Context(), user)
	require.NoError(t, err, "GetClearinghouseState must not error")
	assert.NotNil(t, state, "state should not be nil even without perpetual positions")
}
