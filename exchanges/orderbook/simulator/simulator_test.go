package simulator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/bitstamp"
)

func TestSimulate(t *testing.T) {
	b := bitstamp.Exchange{}
	b.SetDefaults()
	b.Verbose = false
	b.CurrencyPairs = currency.PairsManager{
		UseGlobalFormat: true,
		RequestFormat: &currency.PairFormat{
			Uppercase: true,
		},
		Pairs: map[asset.Item]*currency.PairStore{
			asset.Spot: {
				AssetEnabled: true,
			},
		},
	}
	o, err := b.UpdateOrderbook(t.Context(),
		currency.NewBTCUSD(), asset.Spot)
	require.NoError(t, err, "UpdateOrderbook must not error")
	_, err = o.SimulateOrder(10000000, true)
	require.NoError(t, err, "SimulateOrder must not error for buy order")
	_, err = o.SimulateOrder(2171, false)
	require.NoError(t, err, "SimulateOrder must not error for sell order")
}
