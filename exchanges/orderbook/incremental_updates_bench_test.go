package orderbook

import (
	"testing"
	"time"
)

// Benchstat medians for PR base to measured production head
// (20 counterbalanced fresh-process observations per revision):
// Before: 5021.0 ns/op  10880 B/op  1 allocs/op; After: 688.1 ns/op  0 B/op  0 allocs/op
func BenchmarkProcessUpdateInsertDelete(b *testing.B) {
	depth := NewDepth(id)
	if err := depth.LoadSnapshot(newSnapshot(256)); err != nil {
		b.Fatal(err)
	}

	updateTime := time.Unix(1, 0)
	insertUpdate := &Update{
		UpdateTime: updateTime,
		Asks:       Levels{{Price: 1465.5, Amount: 2, ID: 1000}},
		Action:     InsertAction,
	}
	deleteUpdate := &Update{
		UpdateTime: updateTime,
		Asks:       Levels{{ID: 1000}},
		Action:     DeleteAction,
	}
	for b.Loop() {
		if err := depth.ProcessUpdate(insertUpdate); err != nil {
			b.Fatal(err)
		}
		if err := depth.ProcessUpdate(deleteUpdate); err != nil {
			b.Fatal(err)
		}
	}
}

// Amendment near the touch on a 400 level book with verification enabled, which is the shape most
// websocket feeds produce.
//
// Benchstat over 12 counterbalanced observations per revision on go1.27.0, walking the whole book
// after each update against checking only the neighbourhoods it touched:
// Before: 2619.5n ± 2%; After: 113.2n ± 14%, -95.68% (p=0.000 n=12). No allocations either side.
func BenchmarkProcessUpdateValidated(b *testing.B) {
	depth := NewDepth(id)
	if err := depth.LoadSnapshot(newSnapshot(400)); err != nil {
		b.Fatal(err)
	}
	depth.validateOrderbook = true
	update := &Update{UpdateTime: time.Unix(1, 0), Bids: Levels{{Price: 1332, Amount: 2}}}
	for b.Loop() {
		if err := depth.ProcessUpdate(update); err != nil {
			b.Fatal(err)
		}
	}
}
