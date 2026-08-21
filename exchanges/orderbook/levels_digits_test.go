package orderbook

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDigitsStayAligned drives random amendments, insertions, deletions and truncations at a side
// carrying digits and checks after each that entry i still describes level i. The digits are only
// useful while that holds, and a checksum built from a drifted sidecar would pass or fail at random.
func TestDigitsStayAligned(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(0xA11A, 0x11ED)) //nolint:gosec // deterministically seeded corpus

	digitsFor := func(price, amount float64) LevelDigits {
		// Shortest round-trip form, so parsing back is exact and the check can be too
		return LevelDigits{
			Price:  strconv.FormatFloat(price, 'f', -1, 64),
			Amount: strconv.FormatFloat(amount, 'f', -1, 64),
		}
	}

	for round := range 5000 {
		var side askLevels
		n := 1 + r.IntN(20)
		levels := make(Levels, n)
		digits := make([]LevelDigits, n)
		for i := range n {
			price, amount := 100+float64(i), 1+r.Float64()
			levels[i] = Level{Price: price, Amount: amount}
			digits[i] = digitsFor(price, amount)
		}
		side.load(levels, digits)

		maxDepth := 0
		if r.IntN(3) == 0 {
			maxDepth = 1 + r.IntN(24)
		}

		for range 1 + r.IntN(8) {
			m := 1 + r.IntN(4)
			updts := make(Levels, m)
			updtDigits := make([]LevelDigits, m)
			for j := range m {
				// half land on a stored price, half between them, and some delete
				price := 100 + float64(r.IntN(n+4))
				if r.IntN(2) == 0 {
					price += 0.5
				}
				amount := 1 + r.Float64()
				if r.IntN(4) == 0 {
					amount = 0 // delete
				}
				updts[j] = Level{Price: price, Amount: amount}
				updtDigits[j] = digitsFor(price, amount)
			}
			side.updateInsertByPrice(updts, updtDigits, maxDepth)

			require.Lenf(t, side.digits, len(side.Levels),
				"round %d: the sidecar must stay the same length as the levels", round)
			for i := range side.Levels {
				price, err := strconv.ParseFloat(side.digits[i].Price, 64)
				require.NoErrorf(t, err, "round %d: entry %d must hold a parsable price", round, i)
				assert.Equalf(t, side.Levels[i].Price, price,
					"round %d: entry %d must describe the price beside it", round, i)
				// An amendment leaves the price alone and moves only the amount, so the amount is
				// what catches a sidecar that is written on insertion but not on amendment
				amount, err := strconv.ParseFloat(side.digits[i].Amount, 64)
				require.NoErrorf(t, err, "round %d: entry %d must hold a parsable amount", round, i)
				assert.Equalf(t, side.Levels[i].Amount, amount,
					"round %d: entry %d must describe the amount beside it", round, i)
			}
		}
	}
}

// TestDigitsDroppedByIDKeyedMutations covers every mutation that changes the levels without moving
// the sidecar with them. Leaving it behind would put entry i beside a different level, and an insert
// past its end panicked outright before these dropped it.
func TestDigitsDroppedByIDKeyedMutations(t *testing.T) {
	t.Parallel()

	levels := Levels{{Price: 100, Amount: 1, ID: 1}, {Price: 101, Amount: 1, ID: 2}, {Price: 102, Amount: 1, ID: 3}}
	digits := []LevelDigits{{Price: "100"}, {Price: "101"}, {Price: "102"}}

	for name, mutate := range map[string]func(*askLevels) error{
		"insertUpdates":    func(s *askLevels) error { return s.insertUpdates(Levels{{Price: 103, Amount: 1, ID: 4}}) },
		"deleteByID":       func(s *askLevels) error { return s.deleteByID(Levels{{ID: 2}}, false) },
		"updateByID":       func(s *askLevels) error { return s.updateByID(Levels{{Price: 101, Amount: 5, ID: 2}}) },
		"updateInsertByID": func(s *askLevels) error { return s.updateInsertByID(Levels{{Price: 101, Amount: 5, ID: 2}}) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var side askLevels
			side.load(append(Levels(nil), levels...), append([]LevelDigits(nil), digits...))
			require.NotEmpty(t, side.digits, "the sidecar must start populated")
			require.NoError(t, mutate(&side))
			assert.Emptyf(t, side.digits, "%s should drop the sidecar rather than leave it out of step", name)

			// a following price keyed insert past the old sidecar length must not panic
			assert.NotPanicsf(t, func() {
				side.updateInsertByPrice(Levels{{Price: 105, Amount: 1}}, []LevelDigits{{Price: "105"}}, 0)
			}, "a price keyed insert after %s should not panic", name)
		})
	}
}

// TestDigitsReleaseDiscardedPayloads checks that a slot the sidecar stops using is cleared rather
// than left beyond the slice length. Each digit is a slice of the payload it arrived in, so one
// forgotten header keeps a whole websocket message alive for as long as the book holds the array.
func TestDigitsReleaseDiscardedPayloads(t *testing.T) {
	t.Parallel()

	beyondLength := func(d []LevelDigits) []LevelDigits { return d[len(d):cap(d)] }

	t.Run("deletion", func(t *testing.T) {
		t.Parallel()
		var side askLevels
		side.load(Levels{{Price: 100, Amount: 1}, {Price: 101, Amount: 1}},
			[]LevelDigits{{Price: "100", Amount: "1"}, {Price: "101", Amount: "1"}})
		side.updateInsertByPrice(Levels{{Price: 101, Amount: 0}}, []LevelDigits{{Price: "101"}}, 0)
		require.Len(t, side.digits, 1, "the sidecar must shrink with the levels")
		for _, d := range beyondLength(side.digits) {
			assert.Equal(t, LevelDigits{}, d, "a vacated slot should not keep pinning its payload")
		}
	})

	t.Run("truncation", func(t *testing.T) {
		t.Parallel()
		var side askLevels
		levels := make(Levels, 6)
		digits := make([]LevelDigits, 6)
		for i := range levels {
			levels[i] = Level{Price: 100 + float64(i), Amount: 1}
			digits[i] = LevelDigits{Price: strconv.Itoa(100 + i), Amount: "1"}
		}
		side.load(levels, digits)
		side.updateInsertByPrice(Levels{{Price: 99, Amount: 1}}, []LevelDigits{{Price: "99", Amount: "1"}}, 3)
		require.Len(t, side.digits, 3, "the sidecar must truncate with the levels")
		for _, d := range beyondLength(side.digits) {
			assert.Equal(t, LevelDigits{}, d, "a truncated slot should not keep pinning its payload")
		}
	})
}
