package binance

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/mock"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func TestDerivativeDepthSynchronisation(t *testing.T) {
	server, client, err := mock.NewVCRServer("testdata/depth_snapshots.json")
	require.NoError(t, err, "depth mock server must initialise")
	observer := &publicAuditTransport{t: t, base: client.Transport}
	client.Transport = observer
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	require.NoError(t, local.SetHTTPClient(client), "offline client must initialise")
	require.NoError(t, local.API.Endpoints.SetRunningURL(exchange.RestUSDTMargined.String(), server+"/initial"), "snapshot URL must be local")
	pair := currency.NewBTCUSDT()
	apply := func(first, last, previous int64, bids string) {
		t.Helper()
		payload := fmt.Appendf(nil, `{"e":"depthUpdate","E":1789953400000,"T":1789953400000,"s":"BTCUSDT","U":%d,"u":%d,"pu":%d,"b":%s,"a":[]}`, first, last, previous, bids)
		require.NoError(t, local.processDerivativeDepth(t.Context(), payload, asset.USDTMarginedFutures, false), "diff must be synchronised with its snapshot")
	}
	apply(90, 99, 89, `[["100","99"]]`)
	book, err := local.Websocket.Orderbook.GetOrderbook(pair, asset.USDTMarginedFutures)
	require.NoError(t, err, "snapshot must be available")
	assert.Equal(t, 2.0, book.Bids[0].Amount, "a diff older than the snapshot should be ignored")
	apply(99, 101, 98, `[["100","0"],["101","5"]]`)
	apply(102, 105, 101, `[["101","6"]]`)
	apply(102, 105, 101, `[["101","90"]]`)
	book, err = local.Websocket.Orderbook.GetOrderbook(pair, asset.USDTMarginedFutures)
	require.NoError(t, err, "synchronised book must be available")
	assert.Equal(t, int64(105), book.LastUpdateID, "book should track the final update ID")
	require.Len(t, book.Bids, 2, "snapshot levels untouched by diffs must remain")
	assert.Equal(t, 101.0, book.Bids[0].Price, "new bid should replace the deleted top bid")
	assert.Equal(t, 6.0, book.Bids[0].Amount, "duplicate update should not change the book")
	assert.Equal(t, 99.0, book.Bids[1].Price, "unchanged snapshot level should remain")
	assert.Equal(t, uint64(1), observer.requests, "contiguous diffs should reuse the snapshot")
	require.NoError(t, local.API.Endpoints.SetRunningURL(exchange.RestUSDTMargined.String(), server+"/recovery"), "recovery must use its recorded snapshot")
	apply(201, 202, 200, `[["100","7"]]`)
	assert.Equal(t, uint64(2), observer.requests, "a missing predecessor should force a fresh snapshot")
	book, err = local.Websocket.Orderbook.GetOrderbook(pair, asset.USDTMarginedFutures)
	require.NoError(t, err, "recovered book must be available")
	assert.Equal(t, 100.0, book.Bids[0].Price, "recovery should discard levels from the previous book")
	assert.Equal(t, 7.0, book.Bids[0].Amount, "the update following the new snapshot should apply")
}

func TestDerivativePartialDepthAndBookTicker(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	local.Websocket.DataHandler = stream.NewRelay(10)
	first := []byte(`{"e":"depthUpdate","E":1789953400000,"T":1789953400000,"s":"BTCUSDT","U":10,"u":10,"pu":9,"b":[["100","2"],["99","3"]],"a":[["110","4"]]}`)
	second := []byte(`{"e":"depthUpdate","E":1789953400001,"T":1789953400001,"s":"BTCUSDT","U":11,"u":11,"pu":10,"b":[["98","2"]],"a":[["111","4"]]}`)
	pair := currency.NewBTCUSDT()
	for _, a := range []asset.Item{asset.CoinMarginedFutures, asset.USDTMarginedFutures} {
		// The same internal key must remain separate across assets, even with overlapping wire symbols.
		require.NoError(t, local.CurrencyPairs.StorePairs(a, currency.Pairs{pair}, false), "overlapping symbol must be available")
		require.NoError(t, local.processDerivativeDepth(t.Context(), first, a, true), "partial stream must load its snapshot")
		require.NoError(t, local.processDerivativeDepth(t.Context(), second, a, true), "next partial stream must replace its snapshot")
		book, err := local.Websocket.Orderbook.GetOrderbook(pair, a)
		require.NoError(t, err, "each asset must own a usable book")
		require.Len(t, book.Bids, 1, "partial snapshot must remove levels absent from the next message")
		assert.Equal(t, 98.0, book.Bids[0].Price, "partial book should contain the new price")
		require.NoError(t, local.processBookTicker(t.Context(), []byte(`{"e":"bookTicker","E":1789953400002,"T":1789953400002,"s":"BTCUSDT","u":12,"b":"97","B":"8","a":"112","A":"9"}`), a), "book ticker must publish independently")
		message := <-local.Websocket.DataHandler.C
		value, ok := message.Data.(*ticker.Price)
		require.True(t, ok, "book ticker must publish a ticker")
		assert.Equal(t, a, value.AssetType, "ticker should preserve its asset")
		book, err = local.Websocket.Orderbook.GetOrderbook(pair, a)
		require.NoError(t, err, "depth book must remain available")
		assert.Equal(t, 98.0, book.Bids[0].Price, "book ticker should preserve depth owned by the depth stream")
	}
}

func TestIsPartialDepthStream(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"btcusdt@depth5@100ms", "btcusd_perp@depth20", "btc-260921-81000-c@depth100@500ms"} {
		assert.True(t, isPartialDepthStream(name), "numbered depth channel should be a snapshot")
	}
	for _, name := range []string{"btcusdt@depth@100ms", "btcusd_perp@depth", "btcusdt@bookTicker"} {
		assert.False(t, isPartialDepthStream(name), "other channels should not be partial snapshots")
	}
}
