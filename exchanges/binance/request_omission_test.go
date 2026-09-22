package binance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
)

func TestOptionalCurrencyParameters(t *testing.T) {
	local := new(Exchange)
	for _, code := range []currency.Code{currency.EMPTYCODE, currency.BTC} {
		arg := &WsCFuturesGetPositionsRequest{MarginAsset: code}
		params, err := local.ToMap(arg)
		require.NoError(t, err, "WebSocket request must encode")
		form, err := interfaceToParams(arg)
		require.NoError(t, err, "REST parameters must encode")
		if code.IsEmpty() {
			assert.NotContains(t, params, "marginAsset", "omitted currency should not become an empty API filter")
			assert.NotContains(t, form, "marginAsset", "omitted currency should not become an empty signed query field")
		} else {
			assert.Equal(t, "BTC", params["marginAsset"], "WebSocket filter should retain its currency code")
			assert.Equal(t, "BTC", form.Get("marginAsset"), "REST filter should retain its currency code")
		}
	}
}

func testRequestAs[T any](tb testing.TB, value any) T {
	tb.Helper()
	result, ok := value.(T)
	require.True(tb, ok, "test request must have its declared type")
	return result
}
