package hyperliquid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

func TestLiveGetUserNonFundingLedgerUpdates(t *testing.T) {
	if livetest.ShouldSkip() {
		t.Skipf(livetest.LiveTestingSkipped, e.Name)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	user, err := e.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error")
	end := time.Now().UTC()
	_, err = e.GetUserNonFundingLedgerUpdates(t.Context(), &UserLedgerRequest{User: user, StartTime: end.Add(-24 * time.Hour), EndTime: end})
	assert.NoError(t, err, "GetUserNonFundingLedgerUpdates should succeed for accounts with or without recent transfers")
}
