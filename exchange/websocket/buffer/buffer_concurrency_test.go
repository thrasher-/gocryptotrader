package buffer

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
)

// TestConcurrentUpdatesOnOneBook drives several routines at one pair with buffering on. The map lock
// only ever guarded the lookup, so the buffer was appended, sorted and resliced unsynchronised and
// updates could be lost, duplicated or reset by another routine. Run under -race.
func TestConcurrentUpdatesOnOneBook(t *testing.T) {
	t.Parallel()
	cp, err := getExclusivePair()
	require.NoError(t, err)
	cfg := &config.Exchange{Name: exchangeName}
	cfg.Orderbook.WebsocketBufferEnabled = true
	cfg.Orderbook.WebsocketBufferLimit = 4
	o := &Orderbook{}
	require.NoError(t, o.Setup(cfg, &Config{}, stream.NewRelay(200)))
	require.NoError(t, o.LoadSnapshot(&orderbook.Book{
		Exchange: exchangeName, Pair: cp, Asset: asset.Spot, LastUpdated: time.Now(),
		Bids: orderbook.Levels{{Price: 100, Amount: 1}}, Asks: orderbook.Levels{{Price: 101, Amount: 1}},
	}))

	// a flush racing the writers exercises the other side of the same buffer
	go func() {
		for range 20 {
			o.FlushBuffer()
		}
	}()

	var wg sync.WaitGroup
	for g := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 50 {
				_ = o.Update(&orderbook.Update{
					Pair: cp, Asset: asset.Spot, UpdateTime: time.Now(),
					UpdateID: int64(g*50 + i),
					Bids:     orderbook.Levels{{Price: 100 - float64(i%5), Amount: 1}},
				})
			}
		}()
	}
	wg.Wait()

	// the book must still be readable and consistent afterwards
	_, err = o.GetOrderbook(cp, asset.Spot)
	require.NoError(t, err, "the book must survive concurrent updates")
}
