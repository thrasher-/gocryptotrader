package orderbook

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

// randomSide builds an aligned side, the precondition validateAround relies on
func randomSide(r *rand.Rand, length int, isAscending bool) Levels {
	l := make(Levels, length)
	price := 1000 + r.Float64()*100
	for i := range l {
		step := 0.01 + r.Float64()
		if isAscending {
			price += step
		} else {
			price -= step
		}
		l[i] = Level{Price: price, Amount: 0.01 + r.Float64(), ID: int64(i + 1), Period: 1, StrPrice: "p", StrAmount: "a"}
	}
	return l
}

// randomUpdates mixes amendments with inserts and deletes, seeding the faults checkAlignment
// screens for so both checks have something to disagree over
func randomUpdates(r *rand.Rand, side Levels, count int) Levels {
	updts := make(Levels, count)
	for i := range updts {
		u := Level{Amount: 0.01 + r.Float64(), ID: int64(r.IntN(64)), Period: 1, StrPrice: "p", StrAmount: "a"}
		switch {
		case len(side) > 0 && r.IntN(3) == 0: // amend or delete a stored level
			u.Price = side[r.IntN(len(side))].Price
			if r.IntN(2) == 0 {
				u.Amount = 0 // delete
			}
		case r.IntN(16) == 0:
			u.Price = 0 // ErrPriceZero
		default:
			u.Price = 1000 + r.Float64()*200 // insert
		}
		switch r.IntN(16) {
		case 0:
			u.Period = 0 // errPeriodUnset
		case 1:
			u.StrPrice = "" // errChecksumStringNotSet
		case 2:
			u.StrAmount = ""
		}
		updts[i] = u
	}
	return updts
}

// TestValidateAroundMatchesValidate asserts the windowed check reaches the same verdict as the full
// walk, which holds only because checkAlignment compares strictly adjacent levels
func TestValidateAroundMatchesValidate(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(0x9E3779B9, 0x7F4A7C15)) //nolint:gosec // deterministically seeded so a failing corpus is reproducible
	var windowed, full int
	for range 50_000 {
		opts := &Book{
			Exchange:               "test",
			IsFundingRate:          r.IntN(2) == 0,
			PriceDuplication:       r.IntN(2) == 0,
			IDAlignment:            r.IntN(2) == 0,
			ChecksumStringRequired: r.IntN(2) == 0,
		}
		bids := bidLevels{}
		asks := askLevels{}
		bids.load(randomSide(r, r.IntN(64), descending))
		asks.load(randomSide(r, r.IntN(64), ascending))

		opts.Bids, opts.Asks = bids.Levels, asks.Levels
		if validate(opts) != nil {
			continue // only a book that starts aligned says anything about the window
		}

		bidUpdts := randomUpdates(r, bids.Levels, 1+r.IntN(4))
		askUpdts := randomUpdates(r, asks.Levels, 1+r.IntN(4))
		maxDepth := 0
		if r.IntN(4) == 0 {
			maxDepth = 1 + r.IntN(64)
		}
		bids.updateInsertByPrice(bidUpdts, maxDepth)
		asks.updateInsertByPrice(askUpdts, maxDepth)
		if !bids.ordered || !asks.ordered {
			continue // production falls back to the full walk here
		}

		opts.Bids, opts.Asks = bids.Levels, asks.Levels
		expected := validate(opts)
		actual := validateAround(opts, bidUpdts, askUpdts)
		if expected == nil {
			require.NoError(t, actual, "validateAround must not report a fault the full walk does not")
			continue
		}
		full++
		if actual != nil {
			windowed++
		}
		// Which fault surfaces first can differ, since the window visits levels in update order
		// rather than book order, but a book the full walk rejects must be rejected here too.
		require.Error(t, actual, "validateAround must reject a book the full walk rejects")
	}
	t.Logf("books rejected by the full walk: %d, also rejected by the window: %d", full, windowed)
	assert.Positive(t, full, "the corpus should produce faults for the two checks to agree on")
	assert.Equal(t, full, windowed, "every fault the full walk finds should be found by the window")
}

// TestValidateUpdatedFallsBackToFullWalk covers where the window cannot be trusted: an ID keyed
// update moves levels it does not name, and an unordered side cannot be searched
func TestValidateUpdatedFallsBackToFullWalk(t *testing.T) {
	t.Parallel()

	d := NewDepth(id)
	require.NoError(t, d.LoadSnapshot(newSnapshot(20)))
	d.validateOrderbook = true
	d.bidLevels.Levels[19].Amount = 0
	err := d.ProcessUpdate(&Update{UpdateTime: time.Now(), Action: UpdateAction, Asks: Levels{{Price: 1338, Amount: 2, ID: 21}}})
	require.ErrorIs(t, err, errAmountInvalid, "an ID keyed update must walk the whole book")

	d = NewDepth(id)
	require.NoError(t, d.LoadSnapshot(newSnapshot(20)))
	d.validateOrderbook = true
	// Mark the side unsearchable as a NaN price would, without needing one in the fixture
	d.bidLevels.ordered, d.bidLevels.orderedScanned = false, true
	d.bidLevels.Levels[19].Amount = 0
	err = d.ProcessUpdate(&Update{UpdateTime: time.Now(), Asks: Levels{{Price: 1337.5, Amount: 2}}})
	require.ErrorIs(t, err, errAmountInvalid, "an unsearchable side must walk the whole book")
}

// TestFullValidationInterval asserts a fault the windows never cover is still caught
func TestFullValidationInterval(t *testing.T) {
	t.Parallel()

	d := NewDepth(id)
	require.NoError(t, d.LoadSnapshot(newSnapshot(20)))
	d.validateOrderbook = true
	d.bidLevels.Levels[19].Amount = 0 // the updates below only ever touch the asks

	u := &Update{UpdateTime: time.Now(), Asks: Levels{{Price: 1337.5, Amount: 2}}}
	for range fullValidationInterval - 1 {
		require.NoError(t, d.ProcessUpdate(u), "a windowed check must not see a fault outside the update")
	}
	require.ErrorIs(t, d.ProcessUpdate(u), errAmountInvalid, "the periodic full walk must find it")
}

// TestValidateUpdatedWidthPicksFullWalk asserts the dispatcher switches to the full walk once an
// update touches enough of the book that checking each neighbourhood costs more than walking it.
// The two are distinguishable by whether a fault outside every window is caught.
func TestValidateUpdatedWidthPicksFullWalk(t *testing.T) {
	t.Parallel()

	const depth = 40
	updateFor := func(touched int) *Update {
		bids := make(Levels, touched)
		for i := range bids {
			bids[i] = Level{Price: 1337 - float64(i), Amount: 2}
		}
		return &Update{UpdateTime: time.Now(), Bids: bids}
	}

	// A narrow update leaves the far end of the asks unchecked
	d := NewDepth(id)
	require.NoError(t, d.LoadSnapshot(newSnapshot(depth)))
	d.validateOrderbook = true
	d.askLevels.Levels[depth-1].Amount = 0
	require.NoError(t, d.ProcessUpdate(updateFor(1)), "a narrow update must not walk the whole book")

	// A wide one covers it, since the full walk is then the cheaper check
	d = NewDepth(id)
	require.NoError(t, d.LoadSnapshot(newSnapshot(depth)))
	d.validateOrderbook = true
	d.askLevels.Levels[depth-1].Amount = 0
	// The guard weighs the whole update against the whole book, and this update is all bids
	wide := updateFor(2*depth/windowedCheckRatio + 1)
	require.ErrorIs(t, d.ProcessUpdate(wide), errAmountInvalid, "a wide update must walk the whole book")
}

// TestNonFiniteValuesRejected covers the values that slip past an ordinary bounds check. NaN
// compares false against everything, so `Amount <= 0` let it through and it was then carried into
// every liquidity and slippage figure taken from the book.
func TestNonFiniteValuesRejected(t *testing.T) {
	t.Parallel()

	for name, level := range map[string]Level{
		"NaN amount":          {Price: 100, Amount: math.NaN()},
		"positive inf amount": {Price: 100, Amount: math.Inf(1)},
		"negative inf amount": {Price: 100, Amount: math.Inf(-1)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.ErrorIs(t, checkAlignment(Levels{level}, false, true, false, false, ascending, "test"),
				errAmountInvalid, "%s should be rejected", name)
		})
	}

	for name, level := range map[string]Level{
		"NaN price":          {Price: math.NaN(), Amount: 1},
		"positive inf price": {Price: math.Inf(1), Amount: 1},
		"negative inf price": {Price: math.Inf(-1), Amount: 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.ErrorIs(t, checkAlignment(Levels{level}, false, true, false, false, ascending, "test"),
				errPriceInvalid, "%s should be rejected", name)
		})
	}

	// end to end: an update carrying a NaN amount must invalidate rather than store it
	d := NewDepth(id)
	require.NoError(t, d.LoadSnapshot(newSnapshot(20)))
	d.validateOrderbook = true
	price := d.bidLevels.Levels[0].Price
	err := d.ProcessUpdate(&Update{UpdateTime: time.Now(), Bids: Levels{{Price: price, Amount: math.NaN()}}})
	require.ErrorIs(t, err, errAmountInvalid, "a NaN amount must invalidate the book")
	assert.False(t, d.IsValid(), "the book should not remain valid")
}

// TestSnapshotAndChecksumPreconditions covers the premises the windowed check rests on: a snapshot
// is validated as it is installed, and a checksum is verified whenever a generator is supplied
// rather than only when the expected value happens to be non-zero.
func TestSnapshotAndChecksumPreconditions(t *testing.T) {
	t.Parallel()

	// a snapshot with a fault the windows would never reach must be rejected on the way in
	d := NewDepth(id)
	d.options.validateOrderbook = true
	bad := newSnapshot(20)
	bad.Bids[19].Amount = 0
	require.ErrorIs(t, d.LoadSnapshot(bad), errAmountInvalid, "an invalid snapshot must not install")

	// a checksum of zero is a real value, so a supplied generator must still be consulted
	d = NewDepth(id)
	d.options.validateOrderbook = true
	require.NoError(t, d.LoadSnapshot(newSnapshot(20)))
	err := d.ProcessUpdate(&Update{
		UpdateTime: time.Now(), Bids: Levels{{Price: 1337, Amount: 2}},
		ExpectedChecksum: 0, GenerateChecksum: func(*Book) uint32 { return 1 },
	})
	require.ErrorIs(t, err, errChecksumMismatch, "a zero expected checksum must still be verified")
}

// TestChecksumStringsMustNotBeEmpty pins the invariant that a level a checksum is built from
// carries the digits the venue wrote. Its counterpart on the sidecar branch is
// TestChecksumDigitsMustNotBeEmpty.
func TestChecksumStringsMustNotBeEmpty(t *testing.T) {
	t.Parallel()

	newBook := func(bids Levels) *Book {
		return &Book{
			Exchange: "test", Pair: currency.NewBTCUSD(), Asset: asset.Spot, LastUpdated: time.Now(),
			ChecksumStringRequired: true,
			Bids:                   bids,
			Asks:                   Levels{{Price: 101, Amount: 1, StrPrice: "101", StrAmount: "1"}},
		}
	}

	good := Levels{{Price: 100, Amount: 1, StrPrice: "100", StrAmount: "1"}}
	require.NoError(t, validate(newBook(good)), "a book with digits on every level must validate")

	for name, bids := range map[string]Levels{
		"empty price":  {{Price: 100, Amount: 1, StrPrice: "", StrAmount: "1"}},
		"empty amount": {{Price: 100, Amount: 1, StrPrice: "100", StrAmount: ""}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.ErrorIsf(t, validate(newBook(bids)), errChecksumStringNotSet, "%s should be rejected", name)
		})
	}

	// The windowed check must reject it too, or an update could install one for a thousand updates
	d := NewDepth(id)
	d.AssignOptions(newBook(good))
	require.NoError(t, d.LoadSnapshot(newBook(good)))
	d.validateOrderbook = true
	err := d.ProcessUpdate(&Update{
		UpdateTime: time.Now(),
		Bids:       Levels{{Price: 99, Amount: 1, StrPrice: "", StrAmount: "1"}},
	})
	assert.ErrorIs(t, err, errChecksumStringNotSet, "the windowed check should reject an empty string too")
}
