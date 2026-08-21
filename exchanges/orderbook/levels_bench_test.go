package orderbook

import (
	"strconv"
	"strings"
	"testing"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

// 27906781	        42.4 ns/op	       0 B/op	       0 allocs/op (old)
// 84119028	        13.87 ns/op	       0 B/op	       0 allocs/op (new)
func BenchmarkLoad(b *testing.B) {
	ts := Levels{}
	for b.Loop() {
		ts.load(ask)
	}
}

// 46043871	        25.9 ns/op	       0 B/op	       0 allocs/op (old)
// 63445401	        18.51 ns/op	       0 B/op	       0 allocs/op (new)
func BenchmarkUpdateByID(b *testing.B) {
	asks := Levels{}
	asksSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	asks.load(asksSnapshot)

	for b.Loop() {
		err := asks.updateByID(asksSnapshot)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// 26724331	        44.69 ns/op	       0 B/op	       0 allocs/op
func BenchmarkDeleteByID(b *testing.B) {
	asks := Levels{}
	asksSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	asks.load(asksSnapshot)

	for b.Loop() {
		err := asks.deleteByID(asksSnapshot, false)
		if err != nil {
			b.Fatal(err)
		}
		asks.load(asksSnapshot) // reset
	}
}

// 8384302	       150.9 ns/op	     480 B/op	       1 allocs/op
func BenchmarkRetrieve(b *testing.B) {
	asks := Levels{}
	asksSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	asks.load(asksSnapshot)

	for b.Loop() {
		_ = asks.retrieve(6)
	}
}

// 134830672	         9.83 ns/op	       0 B/op	       0 allocs/op (old)
// 206689897	         5.761 ns/op	   0 B/op	       0 allocs/op (new)
func BenchmarkUpdateInsertByPrice_Amend(b *testing.B) {
	a := askLevels{}
	a.load(ask, nil)

	updates := Levels{
		{
			Price:  1337, // Amend
			Amount: 2,
		},
		{
			Price:  1337, // Amend
			Amount: 1,
		},
	}

	for b.Loop() {
		a.updateInsertByPrice(updates, nil, 0)
	}
}

// 49763002	        24.9 ns/op	       0 B/op	       0 allocs/op (old)
// 25662849	        45.32 ns/op	       0 B/op	       0 allocs/op (new)
func BenchmarkUpdateInsertByPrice_Insert_Delete(b *testing.B) {
	a := askLevels{}

	a.load(ask, nil)

	updates := Levels{
		{
			Price:  1337.5, // Insert
			Amount: 2,
		},
		{
			Price:  1337.5, // Delete
			Amount: 0,
		},
	}

	for b.Loop() {
		a.updateInsertByPrice(updates, nil, 0)
	}
}

// 21614455	        81.74 ns/op	       0 B/op	       0 allocs/op
func BenchmarkUpdateInsertByID_asks(b *testing.B) {
	asks := Levels{}
	asksSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	asks.load(asksSnapshot)

	for b.Loop() {
		err := asks.updateInsertByID(asksSnapshot, askCompare)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// 20328886	        59.94 ns/op	       0 B/op	       0 allocs/op
func BenchmarkUpdateInsertByID_bids(b *testing.B) {
	bids := Levels{}
	bidsSnapshot := Levels{
		{Price: 0.5, Amount: 2, ID: 0},
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}
	bids.load(bidsSnapshot)

	for b.Loop() {
		err := bids.updateInsertByID(bidsSnapshot, bidCompare)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchstat medians for PR base to measured production head
// (20 counterbalanced fresh-process observations per revision):
// MiddleFreshCopy: Before 11.156 µs/op, 32640 B/op, 2 allocs/op; After 7.631 µs/op, 21760 B/op, 1 allocs/op
// MiddleSpareCapacity: Before 4643.0 ns/op, 10880 B/op, 1 allocs/op; After 462.4 ns/op, 0 B/op, 0 allocs/op
// TailSpareCapacity guard: no significant difference at n=20, p=0.774; 565.1 vs 566.1 ns/op, 0 B/op, 0 allocs/op
// Collision guard: no significant difference at n=20, p=0.899; both ~626 ns/op, 104 B/op, 3 allocs/op
func BenchmarkInsertUpdates(b *testing.B) {
	const (
		levelCount = 256
		middle     = levelCount / 2
	)

	snapshot := make(Levels, levelCount)
	for i := range snapshot {
		snapshot[i] = Level{Price: float64(i * 2), Amount: 1, ID: int64(i + 1)}
	}
	middleUpdate := Levels{{Price: float64(middle*2 - 1), Amount: 2, ID: levelCount + 1}}

	b.Run("MiddleFreshCopy", func(b *testing.B) {
		for b.Loop() {
			levels := append(Levels(nil), snapshot...)
			if err := levels.insertUpdates(middleUpdate, askCompare); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("MiddleSpareCapacity", func(b *testing.B) {
		levels := make(Levels, len(snapshot), len(snapshot)+1)
		copy(levels, snapshot)
		for b.Loop() {
			if err := levels.insertUpdates(middleUpdate, askCompare); err != nil {
				b.Fatal(err)
			}
			copy(levels[middle:], levels[middle+1:])
			levels = levels[:len(snapshot)]
		}
	})

	b.Run("TailSpareCapacity", func(b *testing.B) {
		levels := make(Levels, len(snapshot), len(snapshot)+1)
		copy(levels, snapshot)
		update := Levels{{Price: float64(levelCount * 2), Amount: 2, ID: levelCount + 1}}
		for b.Loop() {
			if err := levels.insertUpdates(update, askCompare); err != nil {
				b.Fatal(err)
			}
			levels = levels[:len(snapshot)]
		}
	})

	b.Run("Collision", func(b *testing.B) {
		levels := make(Levels, len(snapshot))
		copy(levels, snapshot)
		update := Levels{{Price: snapshot[middle].Price, Amount: 2, ID: levelCount + 1}}
		for b.Loop() {
			if err := levels.insertUpdates(update, askCompare); err == nil {
				b.Fatal("insertUpdates should return a collision error")
			}
		}
	})
}

// Decoding a full depth payload, the dominant cost in websocket orderbook processing.
//
// Benchstat over 12 counterbalanced observations per revision on go1.27.0, decoding through
// encoding/json into an intermediate [][2]types.Number against scanning straight into the levels:
//
//	  50 levels   21.26µ -> 11.67µ  -45.09% (p=0.000)   10 -> 2 allocs
//	 400 levels  155.22µ -> 85.06µ  -45.20% (p=0.000)   13 -> 2 allocs
//	1000 levels   387.5µ -> 212.2µ  -45.23% (p=0.000)   15 -> 2 allocs
//
// The remainder is not reachable from here: profiling puts a third of what is left in encoding/json
// itself, which must scan the value to find its extent before it can call this method at all.
func BenchmarkLevelsArrayPriceAmountUnmarshal(b *testing.B) {
	for _, levels := range []int{50, 400, 1000} {
		var sb strings.Builder
		sb.WriteByte('[')
		for i := range levels {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(`["` + strconv.FormatFloat(87842.42-float64(i)*0.01, 'f', -1, 64) +
				`","` + strconv.FormatFloat(1.23456789+float64(i%7), 'f', -1, 64) + `"]`)
		}
		sb.WriteByte(']')
		data := []byte(sb.String())
		b.Run(strconv.Itoa(levels), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				var l LevelsArrayPriceAmount
				if err := json.Unmarshal(data, &l); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
