package simulator

import (
	"context"
	"testing"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/bitstamp"
)

func TestSimulate(t *testing.T) {
	t.Parallel()
	b := bitstamp.Bitstamp{}
	b.SetDefaults()
	o, err := b.FetchOrderbook(context.Background(),
		currency.NewPair(currency.BTC, currency.USD), asset.Spot)
	if err != nil {
		t.Error(err)
	}

	r := o.SimulateOrder(10000000, true)
	t.Log(r.Status)
	r = o.SimulateOrder(2171, false)
	t.Log(r.Status)
}
