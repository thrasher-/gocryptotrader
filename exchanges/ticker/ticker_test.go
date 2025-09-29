package ticker

import (
	"log"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/dispatch"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

func TestMain(m *testing.M) {
	err := dispatch.Start(1, dispatch.DefaultJobsLimit)
	if err != nil {
		log.Fatal(err)
	}

	cpyMux = service.mux

	os.Exit(m.Run())
}

var cpyMux *dispatch.Mux

func TestSubscribeTicker(t *testing.T) {
	_, err := SubscribeTicker("", currency.EMPTYPAIR, asset.Empty)
	assert.Error(t, err, "SubscribeTicker should error for empty exchange")

	p := currency.NewBTCUSD()

	// force error
	service.mux = nil
	err = ProcessTicker(&Price{
		Pair:         p,
		ExchangeName: "subscribetest",
		AssetType:    asset.Spot,
	})
	assert.Error(t, err, "ProcessTicker should error when mux missing")

	sillyP := p
	sillyP.Base = currency.GALA_NEO
	err = ProcessTicker(&Price{
		Pair:         sillyP,
		ExchangeName: "subscribetest",
		AssetType:    asset.Spot,
	})
	assert.Error(t, err, "ProcessTicker should error for unsupported currency base")

	sillyP.Quote = currency.AAA
	err = ProcessTicker(&Price{
		Pair:         sillyP,
		ExchangeName: "subscribetest",
		AssetType:    asset.Spot,
	})
	assert.Error(t, err, "ProcessTicker should error for unsupported currency quote")

	err = ProcessTicker(&Price{
		Pair:         sillyP,
		ExchangeName: "subscribetest",
		AssetType:    asset.DownsideProfitContract,
	})
	assert.Error(t, err, "ProcessTicker should error for unsupported asset type")
	// reinstate mux
	service.mux = cpyMux

	require.NoError(t, ProcessTicker(&Price{
		Pair:         p,
		ExchangeName: "subscribetest",
		AssetType:    asset.Spot,
	}), "ProcessTicker must not error with valid inputs")

	_, err = SubscribeTicker("subscribetest", p, asset.Spot)
	require.NoError(t, err, "SubscribeTicker must not error with valid parameters")
}

func TestSubscribeToExchangeTickers(t *testing.T) {
	_, err := SubscribeToExchangeTickers("")
	assert.Error(t, err, "SubscribeToExchangeTickers should error for empty exchange")

	p := currency.NewBTCUSD()

	require.NoError(t, ProcessTicker(&Price{
		Pair:         p,
		ExchangeName: "subscribeExchangeTest",
		AssetType:    asset.Spot,
	}), "ProcessTicker must not error when preparing exchange ticker subscription")

	_, err = SubscribeToExchangeTickers("subscribeExchangeTest")
	require.NoError(t, err, "SubscribeToExchangeTickers must not error with valid exchange")
}

func TestGetTicker(t *testing.T) {
	newPair, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err)
	priceStruct := Price{
		Pair:         newPair,
		Last:         1200,
		High:         1298,
		Low:          1148,
		Bid:          1195,
		Ask:          1220,
		Volume:       5,
		PriceATH:     1337,
		ExchangeName: "bitfinex",
		AssetType:    asset.Spot,
	}

	require.NoError(t, ProcessTicker(&priceStruct), "ProcessTicker must not error for initial ticker")

	tickerPrice, err := GetTicker("bitfinex", newPair, asset.Spot)
	require.NoError(t, err, "GetTicker must not error for stored spot ticker")
	assert.True(t, tickerPrice.Pair.Equal(newPair), "GetTicker pair should match original pair")

	_, err = GetTicker("blah", newPair, asset.Spot)
	assert.Error(t, err, "GetTicker should error for unknown exchange")

	newPair.Base = currency.ETH
	_, err = GetTicker("bitfinex", newPair, asset.Spot)
	assert.Error(t, err, "GetTicker should error for unsupported base currency")

	btcltcPair, err := currency.NewPairFromStrings("BTC", "LTC")
	require.NoError(t, err)

	_, err = GetTicker("bitfinex", btcltcPair, asset.Spot)
	assert.Error(t, err, "GetTicker should error for unsupported quote currency")

	priceStruct.PriceATH = 9001
	priceStruct.Pair.Base = currency.ETH
	priceStruct.AssetType = asset.DownsideProfitContract
	require.NoError(t, ProcessTicker(&priceStruct), "ProcessTicker must not error for downside contract")

	tickerPrice, err = GetTicker("bitfinex", newPair, asset.DownsideProfitContract)
	require.NoError(t, err, "GetTicker must not error for populated pair")
	assert.Equal(t, 9001.0, tickerPrice.PriceATH, "GetTicker PriceATH should preserve processed value")
	_, err = GetTicker("bitfinex", newPair, asset.UpsideProfitContract)
	assert.Error(t, err, "GetTicker should error for unsupported asset")

	priceStruct.AssetType = asset.UpsideProfitContract
	require.NoError(t, ProcessTicker(&priceStruct), "ProcessTicker must not error when asset set")

	// process update again
	require.NoError(t, ProcessTicker(&priceStruct), "ProcessTicker must not error when reprocessing same ticker")
}

func TestFindLast(t *testing.T) {
	cp := currency.NewPair(currency.BTC, currency.XRP)
	_, err := FindLast(cp, asset.Spot)
	assert.ErrorIs(t, err, ErrTickerNotFound)

	err = service.update(&Price{Last: 0, ExchangeName: "testerinos", Pair: cp, AssetType: asset.Spot})
	require.NoError(t, err, "service update must not error")

	_, err = FindLast(cp, asset.Spot)
	assert.ErrorIs(t, err, errInvalidTicker)

	err = service.update(&Price{Last: 1337, ExchangeName: "testerinos", Pair: cp, AssetType: asset.Spot})
	require.NoError(t, err, "service update must not error")

	last, err := FindLast(cp, asset.Spot)
	assert.NoError(t, err)
	assert.Equal(t, 1337.0, last)
}

func TestProcessTicker(t *testing.T) { // non-appending function to tickers
	exchName := "bitstamp"
	newPair, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err)

	priceStruct := Price{
		Last:     1200,
		High:     1298,
		Low:      1148,
		Bid:      1195,
		Ask:      1220,
		Volume:   5,
		PriceATH: 1337,
	}

	assert.Error(t, ProcessTicker(&priceStruct), "ProcessTicker should error when exchange name empty")

	priceStruct.ExchangeName = exchName

	// test for empty pair
	assert.Error(t, ProcessTicker(&priceStruct), "ProcessTicker should error when pair empty")

	// test for empty asset type
	priceStruct.Pair = newPair
	assert.Error(t, ProcessTicker(&priceStruct), "ProcessTicker should error when asset type empty")
	priceStruct.AssetType = asset.Spot
	// now process a valid ticker
	require.NoError(t, ProcessTicker(&priceStruct), "ProcessTicker must not error for valid ticker")
	result, err := GetTicker(exchName, newPair, asset.Spot)
	require.NoError(t, err, "GetTicker must find processed ticker")
	assert.True(t, result.Pair.Equal(newPair), "GetTicker pair should match stored pair")

	err = ProcessTicker(&Price{
		ExchangeName: "Bitfinex",
		Pair:         currency.NewBTCUSD(),
		AssetType:    asset.Margin,
		Bid:          1337,
		Ask:          1337,
	})
	assert.ErrorIs(t, err, ErrBidEqualsAsk, "ProcessTicker should error locked market")

	err = ProcessTicker(&Price{
		ExchangeName: "Bitfinex",
		Pair:         currency.NewBTCUSD(),
		AssetType:    asset.Margin,
		Bid:          1338,
		Ask:          1336,
	})
	assert.ErrorIs(t, err, errBidGreaterThanAsk)

	err = ProcessTicker(&Price{
		ExchangeName: "Bitfinex",
		Pair:         currency.NewBTCUSD(),
		AssetType:    asset.MarginFunding,
		Bid:          1338,
		Ask:          1336,
	})
	assert.NoError(t, err)

	// now test for processing a pair with a different quote currency
	newPair, err = currency.NewPairFromStrings("BTC", "AUD")
	require.NoError(t, err)

	priceStruct.Pair = newPair
	require.NoError(t, ProcessTicker(&priceStruct), "ProcessTicker must not error for updated pair")
	_, err = GetTicker(exchName, newPair, asset.Spot)
	require.NoError(t, err, "GetTicker must return ticker after processing new quote")
	_, err = GetTicker(exchName, newPair, asset.Spot)
	require.NoError(t, err, "GetTicker must return ticker on repeated call")

	// now test for processing a pair which has a different base currency
	newPair, err = currency.NewPairFromStrings("LTC", "AUD")
	require.NoError(t, err)

	priceStruct.Pair = newPair
	require.NoError(t, ProcessTicker(&priceStruct), "ProcessTicker must not error for new base symbol")
	_, err = GetTicker(exchName, newPair, asset.Spot)
	require.NoError(t, err, "GetTicker must return ticker with new base")
	_, err = GetTicker(exchName, newPair, asset.Spot)
	require.NoError(t, err, "GetTicker must allow repeated retrieval for new base")

	type quick struct {
		Name string
		P    currency.Pair
		TP   Price
	}

	testArray := make([]quick, 0, 500)
	_ = rand.NewSource(time.Now().Unix())
	for range 500 {
		//nolint:gosec // no need to import crypto/rand for testing
		newName := "Exchange" + strconv.FormatInt(rand.Int63(), 10)
		newPairs, err := currency.NewPairFromStrings(
			"BTC"+strconv.FormatInt(rand.Int63(), 10), //nolint:gosec // no need to import crypto/rand for testing
			"USD"+strconv.FormatInt(rand.Int63(), 10), //nolint:gosec // no need to import crypto/rand for testing
		)
		require.NoError(t, err)

		tp := Price{
			Pair:         newPairs,
			Last:         rand.Float64(), //nolint:gosec // no need to import crypto/rand for testing
			ExchangeName: newName,
			AssetType:    asset.Spot,
		}
		require.NoError(t, ProcessTicker(&tp), "ProcessTicker must not error for generated ticker")
		testArray = append(testArray, quick{Name: newName, P: newPairs, TP: tp})
	}

	for _, test := range testArray {
		result, err := GetTicker(test.Name, test.P, asset.Spot)
		require.NoErrorf(t, err, "GetTicker must return stored ticker for %s", test.Name)
		assert.Equalf(t, test.TP.Last, result.Last, "GetTicker.Last should match processed value for %s", test.Name)
	}
}

func TestGetAssociation(t *testing.T) {
	_, err := service.getAssociations("")
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	service.mux = nil

	_, err = service.getAssociations("getassociation")
	assert.Error(t, err, "getAssociations should error when mux unavailable")

	service.mux = cpyMux
}

func TestGetExchangeTickersPublic(t *testing.T) {
	_, err := GetExchangeTickers("")
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)
}

func TestGetExchangeTickers(t *testing.T) {
	t.Parallel()
	s := Service{
		Tickers:  make(map[key.ExchangeAssetPair]*Ticker),
		Exchange: make(map[string]uuid.UUID),
	}

	_, err := s.getExchangeTickers("")
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	_, err = s.getExchangeTickers("test")
	assert.ErrorIs(t, err, errExchangeNotFound)

	s.Tickers[key.NewExchangeAssetPair("test", asset.Spot, currency.NewPair(currency.XBT, currency.DOGE))] = &Ticker{
		Price: Price{
			Pair:         currency.NewPair(currency.XBT, currency.DOGE),
			ExchangeName: "test",
			AssetType:    asset.Futures,
			OpenInterest: 1337,
		},
	}
	s.Exchange["test"] = uuid.Must(uuid.NewV4())

	resp, err := s.getExchangeTickers("test")
	assert.NoError(t, err)
	assert.Len(t, resp, 1, "getExchangeTickers should return single ticker")
	assert.Equal(t, 1337.0, resp[0].OpenInterest)
}
