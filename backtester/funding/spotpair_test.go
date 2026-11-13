package funding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gctorder "github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types/decimal"
)

func TestBaseInitialFunds(t *testing.T) {
	t.Parallel()
	baseItem, err := CreateItem(exchName, a, pair.Base, decimal.Zero, decimal.Zero)
	assert.NoError(t, err)

	quoteItem, err := CreateItem(exchName, a, pair.Quote, elite, decimal.Zero)
	assert.NoError(t, err)

	baseItem.pairedWith = quoteItem
	quoteItem.pairedWith = baseItem
	pairItems := SpotPair{base: baseItem, quote: quoteItem}
	funds := pairItems.BaseInitialFunds()
	if !funds.IsZero() {
		t.Errorf("received '%v' expected '%v'", funds, baseItem.available)
	}
}

func TestQuoteInitialFunds(t *testing.T) {
	t.Parallel()
	baseItem, err := CreateItem(exchName, a, pair.Base, decimal.Zero, decimal.Zero)
	assert.NoError(t, err)

	quoteItem, err := CreateItem(exchName, a, pair.Quote, elite, decimal.Zero)
	assert.NoError(t, err)

	baseItem.pairedWith = quoteItem
	quoteItem.pairedWith = baseItem
	pairItems := SpotPair{base: baseItem, quote: quoteItem}
	funds := pairItems.QuoteInitialFunds()
	if !funds.Equal(elite) {
		t.Errorf("received '%v' expected '%v'", funds, elite)
	}
}

func TestBaseAvailable(t *testing.T) {
	t.Parallel()
	baseItem, err := CreateItem(exchName, a, pair.Base, decimal.Zero, decimal.Zero)
	assert.NoError(t, err)

	quoteItem, err := CreateItem(exchName, a, pair.Quote, elite, decimal.Zero)
	assert.NoError(t, err)

	baseItem.pairedWith = quoteItem
	quoteItem.pairedWith = baseItem
	pairItems := SpotPair{base: baseItem, quote: quoteItem}
	funds := pairItems.BaseAvailable()
	if !funds.IsZero() {
		t.Errorf("received '%v' expected '%v'", funds, baseItem.available)
	}
}

func TestQuoteAvailable(t *testing.T) {
	t.Parallel()
	baseItem, err := CreateItem(exchName, a, pair.Base, decimal.Zero, decimal.Zero)
	assert.NoError(t, err)

	quoteItem, err := CreateItem(exchName, a, pair.Quote, elite, decimal.Zero)
	assert.NoError(t, err)

	baseItem.pairedWith = quoteItem
	quoteItem.pairedWith = baseItem
	pairItems := SpotPair{base: baseItem, quote: quoteItem}
	funds := pairItems.QuoteAvailable()
	if !funds.Equal(elite) {
		t.Errorf("received '%v' expected '%v'", funds, elite)
	}
}

func TestReservePair(t *testing.T) {
	t.Parallel()
	baseItem, err := CreateItem(exchName, a, pair.Base, decimal.Zero, decimal.Zero)
	assert.NoError(t, err)

	quoteItem, err := CreateItem(exchName, a, pair.Quote, elite, decimal.Zero)
	assert.NoError(t, err)

	baseItem.pairedWith = quoteItem
	quoteItem.pairedWith = baseItem
	pairItems := SpotPair{base: baseItem, quote: quoteItem}
	err = pairItems.Reserve(decimal.Zero, gctorder.Buy)
	assert.ErrorIs(t, err, errZeroAmountReceived)

	err = pairItems.Reserve(elite, gctorder.Buy)
	assert.NoError(t, err)

	err = pairItems.Reserve(decimal.Zero, gctorder.Sell)
	assert.ErrorIs(t, err, errZeroAmountReceived)

	err = pairItems.Reserve(elite, gctorder.Sell)
	assert.ErrorIs(t, err, errCannotAllocate)

	err = pairItems.Reserve(elite, gctorder.DoNothing)
	assert.ErrorIs(t, err, errCannotAllocate)
}

func TestReleasePair(t *testing.T) {
	t.Parallel()
	baseItem, err := CreateItem(exchName, a, pair.Base, decimal.Zero, decimal.Zero)
	assert.NoError(t, err)

	quoteItem, err := CreateItem(exchName, a, pair.Quote, elite, decimal.Zero)
	assert.NoError(t, err)

	baseItem.pairedWith = quoteItem
	quoteItem.pairedWith = baseItem
	pairItems := SpotPair{base: baseItem, quote: quoteItem}
	err = pairItems.Reserve(decimal.Zero, gctorder.Buy)
	assert.ErrorIs(t, err, errZeroAmountReceived)

	err = pairItems.Reserve(elite, gctorder.Buy)
	assert.NoError(t, err)

	err = pairItems.Reserve(decimal.Zero, gctorder.Sell)
	assert.ErrorIs(t, err, errZeroAmountReceived)

	err = pairItems.Reserve(elite, gctorder.Sell)
	assert.ErrorIs(t, err, errCannotAllocate)

	err = pairItems.Release(decimal.Zero, decimal.Zero, gctorder.Buy)
	assert.ErrorIs(t, err, errZeroAmountReceived)

	err = pairItems.Release(elite, decimal.Zero, gctorder.Buy)
	assert.NoError(t, err)

	err = pairItems.Release(elite, decimal.Zero, gctorder.Buy)
	assert.ErrorIs(t, err, errCannotAllocate)

	err = pairItems.Release(elite, decimal.Zero, gctorder.DoNothing)
	assert.ErrorIs(t, err, errCannotAllocate)

	err = pairItems.Release(elite, decimal.Zero, gctorder.Sell)
	assert.ErrorIs(t, err, errCannotAllocate)

	err = pairItems.Release(decimal.Zero, decimal.Zero, gctorder.Sell)
	assert.ErrorIs(t, err, errZeroAmountReceived)
}

func TestIncreaseAvailablePair(t *testing.T) {
	t.Parallel()
	baseItem, err := CreateItem(exchName, a, pair.Base, decimal.Zero, decimal.Zero)
	assert.NoError(t, err)

	quoteItem, err := CreateItem(exchName, a, pair.Quote, elite, decimal.Zero)
	assert.NoError(t, err)

	baseItem.pairedWith = quoteItem
	quoteItem.pairedWith = baseItem
	pairItems := SpotPair{base: baseItem, quote: quoteItem}
	err = pairItems.IncreaseAvailable(decimal.Zero, gctorder.Buy)
	assert.ErrorIs(t, err, errZeroAmountReceived)

	if !pairItems.quote.available.Equal(elite) {
		t.Errorf("received '%v' expected '%v'", elite, pairItems.quote.available)
	}
	err = pairItems.IncreaseAvailable(decimal.Zero, gctorder.Sell)
	assert.ErrorIs(t, err, errZeroAmountReceived)

	if !pairItems.base.available.IsZero() {
		t.Errorf("received '%v' expected '%v'", decimal.Zero, pairItems.base.available)
	}

	err = pairItems.IncreaseAvailable(elite.Neg(), gctorder.Sell)
	assert.ErrorIs(t, err, errZeroAmountReceived)

	if !pairItems.quote.available.Equal(elite) {
		t.Errorf("received '%v' expected '%v'", elite, pairItems.quote.available)
	}
	err = pairItems.IncreaseAvailable(elite, gctorder.Buy)
	assert.NoError(t, err)

	if !pairItems.base.available.Equal(elite) {
		t.Errorf("received '%v' expected '%v'", elite, pairItems.base.available)
	}

	err = pairItems.IncreaseAvailable(elite, gctorder.DoNothing)
	assert.ErrorIs(t, err, errCannotAllocate)

	if !pairItems.base.available.Equal(elite) {
		t.Errorf("received '%v' expected '%v'", elite, pairItems.base.available)
	}
}

func TestCanPlaceOrderPair(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                          string
		side                          gctorder.Side
		baseAvailable, quoteAvailable decimal.Decimal
		expected                      bool
	}{
		{name: "do nothing without funds", side: gctorder.DoNothing},
		{name: "buy without funds", side: gctorder.Buy},
		{name: "sell without funds", side: gctorder.Sell},
		{name: "buy with quote funds", side: gctorder.Buy, quoteAvailable: decimal.NewFromInt(32), expected: true},
		{name: "sell with base funds", side: gctorder.Sell, baseAvailable: decimal.NewFromInt(32), quoteAvailable: decimal.NewFromInt(32), expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := SpotPair{
				base:  &Item{available: tc.baseAvailable},
				quote: &Item{available: tc.quoteAvailable},
			}
			assert.Equal(t, tc.expected, p.CanPlaceOrder(tc.side))
		})
	}
}

func TestGetPairReader(t *testing.T) {
	t.Parallel()
	p := &SpotPair{
		base: &Item{exchange: "hello"},
	}
	ip, err := p.GetPairReader()
	require.NoError(t, err, "GetPairReader must not error")
	assert.Equal(t, p, ip)
}

func TestGetCollateralReader(t *testing.T) {
	t.Parallel()
	p := &SpotPair{
		base: &Item{exchange: "hello"},
	}
	_, err := p.GetCollateralReader()
	assert.ErrorIs(t, err, ErrNotCollateral)
}

func TestFundReader(t *testing.T) {
	t.Parallel()
	p := &SpotPair{
		base: &Item{exchange: "hello"},
	}
	if p.FundReader() != p {
		t.Error("expected the same thing")
	}
}

func TestFundReserver(t *testing.T) {
	t.Parallel()
	p := &SpotPair{
		base: &Item{exchange: "hello"},
	}
	if p.FundReserver() != p {
		t.Error("expected the same thing")
	}
}

func TestFundReleaser(t *testing.T) {
	t.Parallel()
	p := &SpotPair{
		base: &Item{exchange: "hello"},
	}
	if p.FundReleaser() != p {
		t.Error("expected the same thing")
	}
}

func TestPairReleaser(t *testing.T) {
	t.Parallel()
	p := &SpotPair{
		base: &Item{exchange: "hello"},
	}
	_, err := p.PairReleaser()
	assert.NoError(t, err)
}

func TestCollateralReleaser(t *testing.T) {
	t.Parallel()
	p := &SpotPair{
		base: &Item{exchange: "hello"},
	}
	_, err := p.CollateralReleaser()
	assert.ErrorIs(t, err, ErrNotCollateral)
}

func TestLiquidate(t *testing.T) {
	t.Parallel()
	p := &SpotPair{
		base: &Item{
			available: decimal.NewFromInt(1337),
		},
		quote: &Item{
			available: decimal.NewFromInt(1337),
		},
	}
	p.Liquidate()
	if !p.base.available.IsZero() {
		t.Errorf("received '%v' expected '%v'", p.base.available, "0")
	}
	if !p.quote.available.IsZero() {
		t.Errorf("received '%v' expected '%v'", p.quote.available, "0")
	}
}
