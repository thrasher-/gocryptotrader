package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

const (
	testExchange = "Okx"
)

func TestLenByPrice(t *testing.T) {
	t.Parallel()
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	localItems := []Item{
		{
			Exchange:  testExchange,
			Pair:      p,
			AssetType: asset.Spot,
			Price:     1200,
			Volume:    5,
		},
	}

	assert.GreaterOrEqual(t, byPrice.Len(localItems), 1, "byPrice.Len should report item count")
}

func TestLessByPrice(t *testing.T) {
	t.Parallel()
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	localItems := []Item{
		{
			Exchange:  "bitstamp",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     1200,
			Volume:    5,
		},
		{
			Exchange:  "bitfinex",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     1198,
			Volume:    20,
		},
	}

	assert.True(t, byPrice.Less(localItems, 1, 0), "byPrice.Less should order by price descending")
	assert.False(t, byPrice.Less(localItems, 0, 1), "byPrice.Less should order by price descending")
}

func TestSwapByPrice(t *testing.T) {
	t.Parallel()
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	localItems := []Item{
		{
			Exchange:  "bitstamp",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     1324,
			Volume:    5,
		},
		{
			Exchange:  "bitfinex",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     7863,
			Volume:    20,
		},
	}

	byPrice.Swap(localItems, 0, 1)
	assert.Equal(t, "bitfinex", localItems[0].Exchange, "byPrice.Swap should swap first item")
	assert.Equal(t, "bitstamp", localItems[1].Exchange, "byPrice.Swap should swap second item")
}

func TestLenByVolume(t *testing.T) {
	t.Parallel()
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	localItems := []Item{
		{
			Exchange:  "bitstamp",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     1324,
			Volume:    5,
		},
		{
			Exchange:  "bitfinex",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     7863,
			Volume:    20,
		},
	}

	assert.Equal(t, 2, byVolume.Len(localItems), "byVolume.Len should report item count")
}

func TestLessByVolume(t *testing.T) {
	t.Parallel()
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	localItems := []Item{
		{
			Exchange:  "bitstamp",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     1324,
			Volume:    5,
		},
		{
			Exchange:  "bitfinex",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     7863,
			Volume:    20,
		},
	}
	assert.True(t, byVolume.Less(localItems, 0, 1), "byVolume.Less should compare by volume ascending")
}

func TestSwapByVolume(t *testing.T) {
	t.Parallel()
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	localItems := []Item{
		{
			Exchange:  "bitstamp",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     1324,
			Volume:    5,
		},
		{
			Exchange:  "bitfinex",
			Pair:      p,
			AssetType: asset.Spot,
			Price:     7863,
			Volume:    20,
		},
	}
	byVolume.Swap(localItems, 0, 1)
	assert.Equal(t, "bitfinex", localItems[0].Exchange, "byVolume.Swap should swap first item")
	assert.Equal(t, "bitstamp", localItems[1].Exchange, "byVolume.Swap should swap second item")
}

func TestAdd(t *testing.T) {
	items = items[:0]
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	require.NoError(t, Add(testExchange, p, asset.Spot, 1200, 42), "Add must not error for valid input")
	assert.Len(t, items, 1, "Add should append first entry")

	err = Add("", p, asset.Empty, 0, 0)
	assert.Error(t, err, "Add should error when asset type empty")
	assert.Len(t, items, 1, "Add should not append invalid entry")

	p.Base = currency.XBT
	require.NoError(t, Add(testExchange, p, asset.Spot, 1201, 43), "Add must not error after base conversion")
	require.GreaterOrEqual(t, len(items), 2, "Add must append converted pair")
	assert.Condition(t, func() bool {
		for _, it := range items {
			if it.Pair.String() == "XBTUSD" {
				return true
			}
		}
		return false
	}, "Add should normalise base currency to XBTUSD")

	p, err = currency.NewPairFromStrings("ETH", "USDT")
	require.NoError(t, err, "NewPairFromStrings must not error")
	require.NoError(t, Add(testExchange, p, asset.Spot, 300, 1000), "Add must support USD stable pairs")
	require.GreaterOrEqual(t, len(items), 3, "Add must append ETH entry")
	found := false
	for _, it := range items {
		if it.Pair.String() == "ETHUSD" {
			found = true
			break
		}
	}
	assert.True(t, found, "Add should normalise quote currency to ETHUSD")
}

func TestAppend(t *testing.T) {
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	originalLen := len(items)
	Append("sillyexchange", p, asset.Spot, 1234, 45)
	assert.Equal(t, originalLen+1, len(items), "Append should add new exchange values")

	Append("sillyexchange", p, asset.Spot, 1234, 45)
	assert.Equal(t, originalLen+1, len(items), "Append should not duplicate existing entry")
}

func TestAlreadyExists(t *testing.T) {
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	assert.True(t, AlreadyExists(testExchange, p, asset.Spot, 1200, 42), "AlreadyExists should detect stored exchange")
	p.Base = currency.NewCode("dii")
	assert.False(t, AlreadyExists("bla", p, asset.Spot, 1234, 123), "AlreadyExists should reject unknown exchange")
}

func TestSortExchangesByVolume(t *testing.T) {
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	topVolume := SortExchangesByVolume(p, asset.Spot, true)
	require.NotEmpty(t, topVolume, "SortExchangesByVolume must return results")
	assert.Equal(t, "sillyexchange", topVolume[0].Exchange, "SortExchangesByVolume should order highest volumes first")

	topVolume = SortExchangesByVolume(p, asset.Spot, false)
	require.NotEmpty(t, topVolume, "SortExchangesByVolume must return results")
	assert.Equal(t, testExchange, topVolume[0].Exchange, "SortExchangesByVolume should order lowest volumes first")
}

func TestSortExchangesByPrice(t *testing.T) {
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "NewPairFromStrings must not error")
	topPrice := SortExchangesByPrice(p, asset.Spot, true)
	require.NotEmpty(t, topPrice, "SortExchangesByPrice must return results")
	assert.Equal(t, "sillyexchange", topPrice[0].Exchange, "SortExchangesByPrice should order highest prices first")

	topPrice = SortExchangesByPrice(p, asset.Spot, false)
	require.NotEmpty(t, topPrice, "SortExchangesByPrice must return results")
	assert.Equal(t, testExchange, topPrice[0].Exchange, "SortExchangesByPrice should order lowest prices first")
}
