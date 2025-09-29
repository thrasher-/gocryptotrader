package orderbook

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
)

func testSetup() Book {
	return Book{
		Exchange: "a",
		Pair:     currency.NewBTCUSD(),
		Asks:     []Level{{Price: 7000, Amount: 1}, {Price: 7001, Amount: 2}},
		Bids:     []Level{{Price: 6999, Amount: 1}, {Price: 6998, Amount: 2}},
	}
}

func assertWhaleBombResult(t *testing.T, result *WhaleBombResult, amount, minPrice, maxPrice, percent float64, context string) {
	t.Helper()
	assert.Equalf(t, amount, result.Amount, "WhaleBomb %s result.Amount should equal %f", context, amount)
	assert.Equalf(t, maxPrice, result.MaximumPrice, "WhaleBomb %s result.MaximumPrice should equal %f", context, maxPrice)
	assert.Equalf(t, minPrice, result.MinimumPrice, "WhaleBomb %s result.MinimumPrice should equal %f", context, minPrice)
	assert.Equalf(t, percent, result.PercentageGainOrLoss, "WhaleBomb %s result.PercentageGainOrLoss should equal %f", context, percent)
}

func assertSimulateOrderResult(t *testing.T, result *WhaleBombResult, amount, minPrice, maxPrice float64, expectWarning bool, expectedOrders int, context string) {
	t.Helper()
	assert.Equalf(t, amount, result.Amount, "SimulateOrder %s result.Amount should equal %f", context, amount)
	assert.Equalf(t, minPrice, result.MinimumPrice, "SimulateOrder %s result.MinimumPrice should equal %f", context, minPrice)
	assert.Equalf(t, maxPrice, result.MaximumPrice, "SimulateOrder %s result.MaximumPrice should equal %f", context, maxPrice)
	if expectWarning {
		assert.Containsf(t, result.Status, fullLiquidityUsageWarning, "SimulateOrder %s result.Status should include liquidity warning", context)
	} else {
		assert.NotContainsf(t, result.Status, fullLiquidityUsageWarning, "SimulateOrder %s result.Status should not include liquidity warning", context)
	}
	assert.Lenf(t, result.Orders, expectedOrders, "SimulateOrder %s result.Orders should contain %d entries", context, expectedOrders)
}

func TestWhaleBomb(t *testing.T) {
	t.Parallel()
	b := testSetup()

	_, err := b.WhaleBomb(-1, true)
	require.ErrorIs(t, err, errPriceTargetInvalid)

	result, err := b.WhaleBomb(7001, true) // <- This price should not be wiped out on the book.
	require.NoError(t, err)
	assertWhaleBombResult(t, result, 7000, 7000, 7001, 0.014285714285714287, "ask level hold")

	result, err = b.WhaleBomb(7000.5, true) // <- Slot between prices will lift to next ask level
	require.NoError(t, err)
	assertWhaleBombResult(t, result, 7000, 7000, 7001, 0.014285714285714287, "ask level gap fill")

	result, err = b.WhaleBomb(7002, true) // <- exceed available quotations
	require.NoError(t, err)
	assert.Contains(t, result.Status, fullLiquidityUsageWarning, "WhaleBomb result.Status should mention liquidity warning when exceeding asks")

	result, err = b.WhaleBomb(7000, true) // <- Book should not move
	require.NoError(t, err)
	assertWhaleBombResult(t, result, 0, 7000, 7000, 0, "ask no movement")

	_, err = b.WhaleBomb(6000, true)
	require.ErrorIs(t, err, errCannotShiftPrice)

	_, err = b.WhaleBomb(-1, false)
	require.ErrorIs(t, err, errPriceTargetInvalid)

	result, err = b.WhaleBomb(6998, false) // <- This price should not be wiped out on the book.
	require.NoError(t, err)
	assertWhaleBombResult(t, result, 1, 6998, 6999, -0.014287755393627661, "bid hold")

	result, err = b.WhaleBomb(6998.5, false) // <- Slot between prices will drop to next bid level
	require.NoError(t, err)
	assertWhaleBombResult(t, result, 1, 6998, 6999, -0.014287755393627661, "bid gap fill")

	result, err = b.WhaleBomb(6997, false) // <- exceed available quotations
	require.NoError(t, err)
	assert.Contains(t, result.Status, fullLiquidityUsageWarning, "WhaleBomb result.Status should mention liquidity warning when exceeding bids")

	result, err = b.WhaleBomb(6999, false) // <- Book should not move
	require.NoError(t, err)
	assertWhaleBombResult(t, result, 0, 6999, 6999, 0, "bid no movement")

	_, err = b.WhaleBomb(7500, false)
	require.ErrorIs(t, err, errCannotShiftPrice)
}

func TestSimulateOrder(t *testing.T) {
	t.Parallel()
	b := testSetup()

	// Invalid
	_, err := b.SimulateOrder(-8000, true)
	require.ErrorIs(t, err, errQuoteAmountInvalid)

	_, err = (&Book{}).SimulateOrder(1337, true)
	require.ErrorIs(t, err, errNoLiquidity)

	// Full liquidity used
	result, err := b.SimulateOrder(21002, true)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 3, 7000, 7001, true, 2, "buy full liquidity")

	// Exceed full liquidity used
	result, err = b.SimulateOrder(21003, true)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 3, 7000, 7001, true, 2, "buy full liquidity exceeded")

	// First level
	result, err = b.SimulateOrder(7000, true)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 1, 7000, 7001, false, 1, "buy first level")

	// Half of first tranch
	result, err = b.SimulateOrder(3500, true)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 0.5, 7000, 7000, false, 1, "buy half top level")
	assert.Equalf(t, 0.5, result.Orders[0].Amount, "SimulateOrder buy half top level first order amount should equal 0.5")

	// Half of second level
	result, err = b.SimulateOrder(14001, true)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 2, 7000, 7001, false, 2, "buy half second level")
	assert.Equalf(t, 1.0, result.Orders[1].Amount, "SimulateOrder buy half second level second order amount should equal 1")

	// Hitting bids

	// Invalid

	_, err = (&Book{}).SimulateOrder(-1, false)
	require.ErrorIs(t, err, errBaseAmountInvalid)

	_, err = (&Book{}).SimulateOrder(2, false)
	require.ErrorIs(t, err, errNoLiquidity)

	// Full liquidity used
	result, err = b.SimulateOrder(3, false)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 20995, 6998, 6999, true, 2, "sell full liquidity")

	// Exceed full liquidity used
	result, err = b.SimulateOrder(3.1, false)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 20995, 6998, 6999, true, 2, "sell full liquidity exceeded")

	// First level
	result, err = b.SimulateOrder(1, false)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 6999, 6998, 6999, false, 1, "sell first level")

	// Half of first tranch
	result, err = b.SimulateOrder(.5, false)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 3499.5, 6999, 6999, false, 1, "sell half first level")
	assert.Equalf(t, 0.5, result.Orders[0].Amount, "SimulateOrder sell half first level order amount should equal 0.5")

	// Half of second level
	result, err = b.SimulateOrder(2, false)
	require.NoError(t, err)
	assertSimulateOrderResult(t, result, 13997, 6998, 6999, false, 2, "sell half second level")
	assert.Equalf(t, 1.0, result.Orders[1].Amount, "SimulateOrder sell half second level second order amount should equal 1")
}

func TestGetAveragePrice(t *testing.T) {
	b := Book{
		Exchange: "Binance",
		Pair:     currency.NewBTCUSD(),
	}
	_, err := b.GetAveragePrice(false, 5)
	assert.ErrorIs(t, err, errNotEnoughLiquidity)

	b = Book{
		Asks: []Level{
			{Amount: 5, Price: 1},
			{Amount: 5, Price: 2},
			{Amount: 5, Price: 3},
			{Amount: 5, Price: 4},
		},
	}
	_, err = b.GetAveragePrice(true, -2)
	assert.ErrorIs(t, err, errAmountInvalid)

	avgPrice, err := b.GetAveragePrice(true, 15)
	require.NoError(t, err)
	assert.Equal(t, 2.0, avgPrice)

	avgPrice, err = b.GetAveragePrice(true, 18)
	require.NoError(t, err)
	assert.Equal(t, 2.333, math.Round(avgPrice*1000)/1000)

	_, err = b.GetAveragePrice(true, 25)
	assert.ErrorIs(t, err, errNotEnoughLiquidity)
}

func TestFindNominalAmount(t *testing.T) {
	b := Levels{
		{Amount: 5, Price: 1},
		{Amount: 5, Price: 2},
		{Amount: 5, Price: 3},
		{Amount: 5, Price: 4},
	}
	nomAmt, remainingAmt := b.FindNominalAmount(15)
	assert.Equal(t, 30.0, nomAmt, "FindNominalAmount should return nominal amount 30 for 15 units")
	assert.Equal(t, 0.0, remainingAmt, "FindNominalAmount should return zero remaining when fully satisfied")
	b = Levels{}
	nomAmt, remainingAmt = b.FindNominalAmount(15)
	assert.Equal(t, 0.0, nomAmt, "FindNominalAmount should return zero nominal amount for empty levels")
	assert.Equal(t, 15.0, remainingAmt, "FindNominalAmount should return full remaining quantity for empty levels")
}
