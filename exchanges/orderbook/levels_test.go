package orderbook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ask = Levels{
	{Price: 1337, Amount: 1},
	{Price: 1338, Amount: 1},
	{Price: 1339, Amount: 1},
	{Price: 1340, Amount: 1},
	{Price: 1341, Amount: 1},
	{Price: 1342, Amount: 1},
	{Price: 1343, Amount: 1},
	{Price: 1344, Amount: 1},
	{Price: 1345, Amount: 1},
	{Price: 1346, Amount: 1},
	{Price: 1347, Amount: 1},
	{Price: 1348, Amount: 1},
	{Price: 1349, Amount: 1},
	{Price: 1350, Amount: 1},
	{Price: 1351, Amount: 1},
	{Price: 1352, Amount: 1},
	{Price: 1353, Amount: 1},
	{Price: 1354, Amount: 1},
	{Price: 1355, Amount: 1},
	{Price: 1356, Amount: 1},
}

var bid = Levels{
	{Price: 1336, Amount: 1},
	{Price: 1335, Amount: 1},
	{Price: 1334, Amount: 1},
	{Price: 1333, Amount: 1},
	{Price: 1332, Amount: 1},
	{Price: 1331, Amount: 1},
	{Price: 1330, Amount: 1},
	{Price: 1329, Amount: 1},
	{Price: 1328, Amount: 1},
	{Price: 1327, Amount: 1},
	{Price: 1326, Amount: 1},
	{Price: 1325, Amount: 1},
	{Price: 1324, Amount: 1},
	{Price: 1323, Amount: 1},
	{Price: 1322, Amount: 1},
	{Price: 1321, Amount: 1},
	{Price: 1320, Amount: 1},
	{Price: 1319, Amount: 1},
	{Price: 1318, Amount: 1},
	{Price: 1317, Amount: 1},
}

func TestLoad(t *testing.T) {
	list := askLevels{}
	Check(t, list, 0, 0, 0)

	list.load(Levels{
		{Price: 1, Amount: 1},
		{Price: 3, Amount: 1},
		{Price: 5, Amount: 1},
		{Price: 7, Amount: 1},
		{Price: 9, Amount: 1},
		{Price: 11, Amount: 1},
	})

	Check(t, list, 6, 36, 6)

	list.load(Levels{
		{Price: 1, Amount: 1},
		{Price: 3, Amount: 1},
		{Price: 5, Amount: 1},
	})

	Check(t, list, 3, 9, 3)

	list.load(Levels{
		{Price: 1, Amount: 1},
		{Price: 3, Amount: 1},
		{Price: 5, Amount: 1},
		{Price: 7, Amount: 1},
	})

	Check(t, list, 4, 16, 4)

	// purge entire list
	list.load(nil)
	Check(t, list, 0, 0, 0)
}

// 27906781	        42.4 ns/op	       0 B/op	       0 allocs/op (old)
// 84119028	        13.87 ns/op	       0 B/op	       0 allocs/op (new)
func BenchmarkLoad(b *testing.B) {
	ts := Levels{}
	for b.Loop() {
		ts.load(ask)
	}
}

func TestUpdateInsertByPrice(t *testing.T) {
	a := askLevels{}
	asksSnapshot := Levels{
		{Price: 1, Amount: 1},
		{Price: 3, Amount: 1},
		{Price: 5, Amount: 1},
		{Price: 7, Amount: 1},
		{Price: 9, Amount: 1},
		{Price: 11, Amount: 1},
	}
	a.load(asksSnapshot)

	// Update one instance with matching price
	a.updateInsertByPrice(Levels{{Price: 1, Amount: 2}}, 0)

	Check(t, a, 7, 37, 6)

	// Insert at head
	a.updateInsertByPrice(Levels{
		{Price: 0.5, Amount: 2},
	}, 0)

	Check(t, a, 9, 38, 7)

	// Insert at tail
	a.updateInsertByPrice(Levels{
		{Price: 12, Amount: 2},
	}, 0)

	Check(t, a, 11, 62, 8)

	// Insert between price and up to and beyond max allowable depth level
	a.updateInsertByPrice(Levels{
		{Price: 11.5, Amount: 2},
		{Price: 10.5, Amount: 2},
		{Price: 13, Amount: 2},
	}, 10)

	Check(t, a, 15, 106, 10)

	// delete at tail
	a.updateInsertByPrice(Levels{{Price: 12, Amount: 0}}, 0)

	Check(t, a, 13, 82, 9)

	// delete at mid
	a.updateInsertByPrice(Levels{{Price: 7, Amount: 0}}, 0)

	Check(t, a, 12, 75, 8)

	// delete at head
	a.updateInsertByPrice(Levels{{Price: 0.5, Amount: 0}}, 0)

	Check(t, a, 10, 74, 7)

	// purge if liquidity plunges to zero
	a.load(nil)

	// rebuild everything again
	a.updateInsertByPrice(Levels{
		{Price: 1, Amount: 1},
		{Price: 3, Amount: 1},
		{Price: 5, Amount: 1},
		{Price: 7, Amount: 1},
		{Price: 9, Amount: 1},
		{Price: 11, Amount: 1},
	}, 0)

	Check(t, a, 6, 36, 6)

	b := bidLevels{}
	bidsSnapshot := Levels{
		{Price: 11, Amount: 1},
		{Price: 9, Amount: 1},
		{Price: 7, Amount: 1},
		{Price: 5, Amount: 1},
		{Price: 3, Amount: 1},
		{Price: 1, Amount: 1},
	}
	b.load(bidsSnapshot)

	// Update one instance with matching price
	b.updateInsertByPrice(Levels{{Price: 11, Amount: 2}}, 0)

	Check(t, b, 7, 47, 6)

	// Insert at head
	b.updateInsertByPrice(Levels{{Price: 12, Amount: 2}}, 0)

	Check(t, b, 9, 71, 7)

	// Insert at tail
	b.updateInsertByPrice(Levels{{Price: 0.5, Amount: 2}}, 0)

	Check(t, b, 11, 72, 8)

	// Insert between price and up to and beyond max allowable depth level
	b.updateInsertByPrice(Levels{
		{Price: 11.5, Amount: 2},
		{Price: 10.5, Amount: 2},
		{Price: 13, Amount: 2},
	}, 10)

	Check(t, b, 15, 141, 10)

	// Insert between price and up to and beyond max allowable depth level
	b.updateInsertByPrice(Levels{{Price: 1, Amount: 0}}, 0)

	Check(t, b, 14, 140, 9)

	// delete at mid
	b.updateInsertByPrice(Levels{{Price: 10.5, Amount: 0}}, 0)

	Check(t, b, 12, 119, 8)

	// delete at head
	b.updateInsertByPrice(Levels{{Price: 13, Amount: 0}}, 0)

	Check(t, b, 10, 93, 7)

	// purge if liquidity plunges to zero
	b.load(nil)

	// rebuild everything again
	b.updateInsertByPrice(Levels{
		{Price: 1, Amount: 1},
		{Price: 3, Amount: 1},
		{Price: 5, Amount: 1},
		{Price: 7, Amount: 1},
		{Price: 9, Amount: 1},
		{Price: 11, Amount: 1},
	}, 0)

	Check(t, b, 6, 36, 6)
}

// 134830672	         9.83 ns/op	       0 B/op	       0 allocs/op (old)
// 206689897	         5.761 ns/op	   0 B/op	       0 allocs/op (new)
func BenchmarkUpdateInsertByPrice_Amend(b *testing.B) {
	a := askLevels{}
	a.load(ask)

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
		a.updateInsertByPrice(updates, 0)
	}
}

// 49763002	        24.9 ns/op	       0 B/op	       0 allocs/op (old)
// 25662849	        45.32 ns/op	       0 B/op	       0 allocs/op (new)
func BenchmarkUpdateInsertByPrice_Insert_Delete(b *testing.B) {
	a := askLevels{}

	a.load(ask)

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
		a.updateInsertByPrice(updates, 0)
	}
}

func TestUpdateByID(t *testing.T) {
	a := askLevels{}
	asksSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	a.load(asksSnapshot)

	err := a.updateByID(Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	})
	require.NoError(t, err, "updateByID must not error for matching IDs")

	Check(t, a, 6, 36, 6)

	err = a.updateByID(Levels{
		{Price: 11, Amount: 1, ID: 1337},
	})
	require.ErrorIs(t, err, errIDCannotBeMatched)

	err = a.updateByID(Levels{ // Simulate Bitmex updating
		{Price: 0, Amount: 1337, ID: 3},
	})
	require.NoError(t, err)

	got := a.retrieve(2)
	require.Len(t, got, 2, "retrieve must return two levels after truncation")
	assert.NotEqual(t, 0.0, got[1].Price, "retrieve should preserve original price when updating by ID")

	got = a.retrieve(3)
	require.Len(t, got, 3, "retrieve must return three levels when requesting 3 entries")
	assert.Equal(t, 1337.0, got[1].Amount, "retrieve should include updated amount for ID 3")

	got = a.retrieve(1000)
	require.Len(t, got, 6, "retrieve must return all available levels when more requested")
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

func TestDeleteByID(t *testing.T) {
	a := askLevels{}
	asksSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	a.load(asksSnapshot)

	// Delete at head
	err := a.deleteByID(Levels{{Price: 1, Amount: 1, ID: 1}}, false)
	require.NoError(t, err, "deleteByID must not error when deleting head level")

	Check(t, a, 5, 35, 5)

	// Delete at tail
	err = a.deleteByID(Levels{{Price: 1, Amount: 1, ID: 11}}, false)
	require.NoError(t, err, "deleteByID must not error when deleting tail level")

	Check(t, a, 4, 24, 4)

	// Delete in middle
	err = a.deleteByID(Levels{{Price: 1, Amount: 1, ID: 5}}, false)
	require.NoError(t, err, "deleteByID must not error when deleting middle level")

	Check(t, a, 3, 19, 3)

	// Intentional error
	err = a.deleteByID(Levels{{Price: 11, Amount: 1, ID: 1337}}, false)
	require.ErrorIs(t, err, errIDCannotBeMatched)

	// Error bypass
	err = a.deleteByID(Levels{{Price: 11, Amount: 1, ID: 1337}}, true)
	require.NoError(t, err, "deleteByID must not error when bypass flag true")
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

func TestUpdateInsertByIDAsk(t *testing.T) {
	a := askLevels{}
	asksSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	a.load(asksSnapshot)

	// Update one instance with matching ID
	require.NoError(t, a.updateInsertByID(Levels{{Price: 1, Amount: 2, ID: 1}}), "updateInsertByID must not error for matching IDs")

	Check(t, a, 7, 37, 6)

	// Reset
	a.load(asksSnapshot)

	// Update all instances with matching ID in order
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 5, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "updateInsertByID must not error when updating IDs in order")

	Check(t, a, 12, 72, 6)

	// Update all instances with matching ID in backwards
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 11, Amount: 2, ID: 11},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 5, Amount: 2, ID: 5},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 1, Amount: 2, ID: 1},
	}), "updateInsertByID must not error when updating IDs in reverse order")

	Check(t, a, 12, 72, 6)

	// Update all instances with matching ID all over the ship
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 11, Amount: 2, ID: 11},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 1, Amount: 2, ID: 1},
		{Price: 5, Amount: 2, ID: 5},
	}), "updateInsertByID must not error when updating IDs arbitrarily")

	Check(t, a, 12, 72, 6)

	// Update all instances move one before ID in middle
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 2, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "updateInsertByID must not error when moving ID before middle")

	Check(t, a, 12, 66, 6)

	// Update all instances move one before ID at head
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: .5, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "updateInsertByID must not error when moving ID before head")

	Check(t, a, 12, 63, 6)

	// Reset
	a.load(asksSnapshot)

	// Update all instances move one after ID
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 8, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "updateInsertByID must not error when moving ID after middle")

	Check(t, a, 12, 78, 6)

	// Reset
	a.load(asksSnapshot)

	// Update all instances move one after ID to tail
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "updateInsertByID must not error when moving ID after tail")

	Check(t, a, 12, 86, 6)

	// Update all instances then pop new instance
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
		{Price: 10, Amount: 2, ID: 10},
	}), "updateInsertByID must not error when inserting new ID")

	Check(t, a, 14, 106, 7)

	// Reset
	a.load(asksSnapshot)

	// Update all instances pop at head
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 0.5, Amount: 2, ID: 0},
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "updateInsertByID must not error when inserting new head")

	Check(t, a, 14, 87, 7)

	// bookmark head and move to mid
	require.NoError(t, a.updateInsertByID(Levels{{Price: 7.5, Amount: 2, ID: 0}}), "updateInsertByID must not error when moving bookmarked head to mid")

	Check(t, a, 14, 101, 7)

	// bookmark head and move to tail
	require.NoError(t, a.updateInsertByID(Levels{{Price: 12.5, Amount: 2, ID: 1}}), "updateInsertByID must not error when moving bookmarked head to tail")

	Check(t, a, 14, 124, 7)

	// move tail location to head
	require.NoError(t, a.updateInsertByID(Levels{{Price: 2.5, Amount: 2, ID: 1}}), "updateInsertByID must not error when moving tail location to head")

	Check(t, a, 14, 104, 7)

	// move tail location to mid
	require.NoError(t, a.updateInsertByID(Levels{{Price: 8, Amount: 2, ID: 5}}), "updateInsertByID must not error when moving tail location to mid")

	Check(t, a, 14, 96, 7)

	// insert at tail dont match
	require.NoError(t, a.updateInsertByID(Levels{{Price: 30, Amount: 2, ID: 1234}}), "updateInsertByID must not error when inserting unmatched tail entry")

	Check(t, a, 16, 156, 8)

	// insert between last and 2nd last
	require.NoError(t, a.updateInsertByID(Levels{{Price: 12, Amount: 2, ID: 12345}}), "updateInsertByID must not error when inserting between tail entries")

	Check(t, a, 18, 180, 9)

	// readjust at end
	require.NoError(t, a.updateInsertByID(Levels{{Price: 29, Amount: 3, ID: 1234}}), "updateInsertByID must not error when readjusting tail entry")

	Check(t, a, 19, 207, 9)

	// readjust further and decrease price past tail
	require.NoError(t, a.updateInsertByID(Levels{{Price: 31, Amount: 3, ID: 1234}}), "updateInsertByID must not error when adjusting tail beyond original price")

	Check(t, a, 19, 213, 9)

	// purge
	a.load(nil)

	// insert with no liquidity and jumbled
	require.NoError(t, a.updateInsertByID(Levels{
		{Price: 11, Amount: 2, ID: 11},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 0.5, Amount: 2, ID: 0},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
	}), "updateInsertByID must not error when rebuilding from empty liquidity")

	Check(t, a, 14, 87, 7)
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
		require.NoError(b, asks.updateInsertByID(asksSnapshot, askCompare), "updateInsertByID must not error in benchmark")
	}
}

func TestUpdateInsertByIDBids(t *testing.T) {
	b := bidLevels{}
	bidsSnapshot := Levels{
		{Price: 11, Amount: 1, ID: 11},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 1, Amount: 1, ID: 1},
	}
	b.load(bidsSnapshot)

	// Update one instance with matching ID
	require.NoError(t, b.updateInsertByID(Levels{{Price: 1, Amount: 2, ID: 1}}), "bid updateInsertByID must not error for matching IDs")

	Check(t, b, 7, 37, 6)

	// Reset
	b.load(bidsSnapshot)

	// Update all instances with matching ID in order
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 5, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "bid updateInsertByID must not error when updating IDs in order")

	Check(t, b, 12, 72, 6)

	// Update all instances with matching ID in backwards
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 11, Amount: 2, ID: 11},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 5, Amount: 2, ID: 5},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 1, Amount: 2, ID: 1},
	}), "bid updateInsertByID must not error when reversing IDs")

	Check(t, b, 12, 72, 6)

	// Update all instances with matching ID all over the ship
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 11, Amount: 2, ID: 11},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 1, Amount: 2, ID: 1},
		{Price: 5, Amount: 2, ID: 5},
	}), "bid updateInsertByID must not error when updating IDs out of order")

	Check(t, b, 12, 72, 6)

	// Update all instances move one before ID in middle
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 2, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "bid updateInsertByID must not error when moving before middle")

	Check(t, b, 12, 66, 6)

	// Update all instances move one before ID at head
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: .5, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "bid updateInsertByID must not error when moving before head")

	Check(t, b, 12, 63, 6)

	// Reset
	b.load(bidsSnapshot)

	// Update all instances move one after ID
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 8, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "bid updateInsertByID must not error when moving after middle")

	Check(t, b, 12, 78, 6)

	// Reset
	b.load(bidsSnapshot)

	// Update all instances move one after ID to tail
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "bid updateInsertByID must not error when moving after tail")

	Check(t, b, 12, 86, 6)

	// Update all instances then pop new instance
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
		{Price: 10, Amount: 2, ID: 10},
	}), "bid updateInsertByID must not error when inserting new ID")

	Check(t, b, 14, 106, 7)

	// Reset
	b.load(bidsSnapshot)

	// Update all instances pop at tail
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 0.5, Amount: 2, ID: 0},
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "bid updateInsertByID must not error when inserting new head")

	Check(t, b, 14, 87, 7)

	// bookmark head and move to mid
	require.NoError(t, b.updateInsertByID(Levels{{Price: 9.5, Amount: 2, ID: 5}}), "bid updateInsertByID must not error when moving bookmarked head to mid")

	Check(t, b, 14, 82, 7)

	// bookmark head and move to tail
	require.NoError(t, b.updateInsertByID(Levels{{Price: 0.25, Amount: 2, ID: 11}}), "bid updateInsertByID must not error when moving bookmarked head to tail")

	Check(t, b, 14, 60.5, 7)

	// move tail location to head
	require.NoError(t, b.updateInsertByID(Levels{{Price: 10, Amount: 2, ID: 11}}), "bid updateInsertByID must not error when moving tail to head")

	Check(t, b, 14, 80, 7)

	// move tail location to mid
	require.NoError(t, b.updateInsertByID(Levels{{Price: 7.5, Amount: 2, ID: 0}}), "bid updateInsertByID must not error when moving tail to mid")

	Check(t, b, 14, 94, 7)

	// insert at head dont match
	require.NoError(t, b.updateInsertByID(Levels{{Price: 30, Amount: 2, ID: 1234}}), "bid updateInsertByID must not error when inserting unmatched head entry")

	Check(t, b, 16, 154, 8)

	// insert between last and 2nd last
	require.NoError(t, b.updateInsertByID(Levels{{Price: 1.5, Amount: 2, ID: 12345}}), "bid updateInsertByID must not error when inserting between tail entries")
	Check(t, b, 18, 157, 9)

	// readjust at end
	require.NoError(t, b.updateInsertByID(Levels{{Price: 1, Amount: 3, ID: 1}}), "bid updateInsertByID must not error when readjusting head entry")
	Check(t, b, 19, 158, 9)

	// readjust further and decrease price past tail
	require.NoError(t, b.updateInsertByID(Levels{{Price: .9, Amount: 3, ID: 1}}), "bid updateInsertByID must not error when adjusting price past tail")
	Check(t, b, 19, 157.7, 9)

	// purge
	b.load(nil)

	// insert with no liquidity and jumbled
	require.NoError(t, b.updateInsertByID(Levels{
		{Price: 0.5, Amount: 2, ID: 0},
		{Price: 1, Amount: 2, ID: 1},
		{Price: 3, Amount: 2, ID: 3},
		{Price: 12, Amount: 2, ID: 5},
		{Price: 7, Amount: 2, ID: 7},
		{Price: 9, Amount: 2, ID: 9},
		{Price: 11, Amount: 2, ID: 11},
	}), "bid updateInsertByID must not error when rebuilding from empty liquidity")

	Check(t, b, 14, 87, 7)
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
		require.NoError(b, bids.updateInsertByID(bidsSnapshot, bidCompare), "updateInsertByID must not error in bid benchmark")
	}
}

func TestInsertUpdatesBid(t *testing.T) {
	b := bidLevels{}
	bidsSnapshot := Levels{
		{Price: 11, Amount: 1, ID: 11},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 1, Amount: 1, ID: 1},
	}
	b.load(bidsSnapshot)

	err := b.insertUpdates(Levels{
		{Price: 11, Amount: 1, ID: 11},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 1, Amount: 1, ID: 1},
	})
	require.ErrorIs(t, err, errCollisionDetected)

	Check(t, b, 6, 36, 6)

	// Insert at head
	require.NoError(t, b.insertUpdates(Levels{{Price: 12, Amount: 1, ID: 11}}), "insertUpdates must not error when inserting at head for bids")

	Check(t, b, 7, 48, 7)

	// Insert at tail
	require.NoError(t, b.insertUpdates(Levels{{Price: 0.5, Amount: 1, ID: 12}}), "insertUpdates must not error when inserting at tail for bids")

	Check(t, b, 8, 48.5, 8)

	// Insert at mid
	require.NoError(t, b.insertUpdates(Levels{{Price: 5.5, Amount: 1, ID: 13}}), "insertUpdates must not error when inserting mid for bids")

	Check(t, b, 9, 54, 9)

	// purge
	b.load(nil)

	// Add one at head
	require.NoError(t, b.insertUpdates(Levels{{Price: 5.5, Amount: 1, ID: 13}}), "insertUpdates must not error when inserting into empty bids")

	Check(t, b, 1, 5.5, 1)
}

func TestInsertUpdatesAsk(t *testing.T) {
	a := askLevels{}
	askSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	a.load(askSnapshot)

	err := a.insertUpdates(Levels{
		{Price: 11, Amount: 1, ID: 11},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 1, Amount: 1, ID: 1},
	})
	require.ErrorIs(t, err, errCollisionDetected)

	Check(t, a, 6, 36, 6)

	// Insert at tail
	require.NoError(t, a.insertUpdates(Levels{{Price: 12, Amount: 1, ID: 11}}), "insertUpdates must not error when inserting at tail for asks")

	Check(t, a, 7, 48, 7)

	// Insert at head
	require.NoError(t, a.insertUpdates(Levels{{Price: 0.5, Amount: 1, ID: 12}}), "insertUpdates must not error when inserting at head for asks")

	Check(t, a, 8, 48.5, 8)

	// Insert at mid
	require.NoError(t, a.insertUpdates(Levels{{Price: 5.5, Amount: 1, ID: 13}}), "insertUpdates must not error when inserting mid for asks")

	Check(t, a, 9, 54, 9)

	// purge
	a.load(nil)

	// Add one at head
	require.NoError(t, a.insertUpdates(Levels{{Price: 5.5, Amount: 1, ID: 13}}), "insertUpdates must not error when inserting into empty asks")

	Check(t, a, 1, 5.5, 1)
}

// check checks depth values after an update has taken place
func Check(t *testing.T, depth any, liquidity, value float64, expectedLen int) {
	t.Helper()
	b, isBid := depth.(bidLevels)
	a, isAsk := depth.(askLevels)

	var l Levels
	switch {
	case isBid:
		l = b.Levels
	case isAsk:
		l = a.Levels
	default:
		require.Failf(t, "Check must receive bidLevels or askLevels", "depth type %T must be bidLevels or askLevels", depth)
		return
	}

	liquidityTotal, valueTotal := l.amount()
	assert.Equalf(t, liquidity, liquidityTotal, "Levels amount should equal expected liquidity for %T", depth)
	assert.Equalf(t, value, valueTotal, "Levels amount should equal expected value for %T", depth)
	assert.Lenf(t, l, expectedLen, "Levels length should match expected length for %T", depth)

	if len(l) == 0 {
		return
	}

	var price float64
	for x := range l {
		switch {
		case price == 0:
			price = l[x].Price
		case isBid && price < l[x].Price:
			require.Failf(t, "bidLevels must be in descending order", "observed previous price %f before %f", price, l[x].Price)
		case isAsk && price > l[x].Price:
			require.Failf(t, "askLevels must be in ascending order", "observed previous price %f before %f", price, l[x].Price)
		default:
			price = l[x].Price
		}
	}
}

func TestAmount(t *testing.T) {
	a := askLevels{}
	askSnapshot := Levels{
		{Price: 1, Amount: 1, ID: 1},
		{Price: 3, Amount: 1, ID: 3},
		{Price: 5, Amount: 1, ID: 5},
		{Price: 7, Amount: 1, ID: 7},
		{Price: 9, Amount: 1, ID: 9},
		{Price: 11, Amount: 1, ID: 11},
	}
	a.load(askSnapshot)

	liquidity, value := a.amount()
	assert.Equal(t, 6.0, liquidity, "amount should return expected liquidity for asks")
	assert.Equal(t, 36.0, value, "amount should return expected value for asks")
}

func TestGetMovementByBaseAmount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name            string
		BaseAmount      float64
		ReferencePrice  float64
		BidLiquidity    Levels
		ExpectedNominal float64
		ExpectedImpact  float64
		ExpectedCost    float64
		ExpectedError   error
	}{
		{
			Name:          "no amount",
			ExpectedError: errBaseAmountInvalid,
		},
		{
			Name:          "no reference price",
			BaseAmount:    1,
			ExpectedError: errInvalidReferencePrice,
		},
		{
			Name:           "not enough liquidity to service quote amount",
			BaseAmount:     1,
			ReferencePrice: 1000,
			ExpectedError:  errNoLiquidity,
		},
		{
			Name:            "thrasher test",
			BidLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			BaseAmount:      10,
			ReferencePrice:  10000,
			ExpectedNominal: 0.8999999999999999,
			ExpectedImpact:  2,
			ExpectedCost:    900,
		},
		{
			Name:            "consume first level",
			BidLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			BaseAmount:      2,
			ReferencePrice:  10000,
			ExpectedNominal: 0,
			ExpectedImpact:  1,
			ExpectedCost:    0,
		},
		{
			Name:            "consume most of first level",
			BidLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			BaseAmount:      1.5,
			ReferencePrice:  10000,
			ExpectedNominal: 0,
			ExpectedImpact:  0,
			ExpectedCost:    0,
		},
		{
			Name:            "consume full liquidity",
			BidLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			BaseAmount:      12,
			ReferencePrice:  10000,
			ExpectedNominal: 1.0833333333333395,
			ExpectedImpact:  FullLiquidityExhaustedPercentage,
			ExpectedCost:    1300,
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			depth := NewDepth(id)
			require.NoErrorf(t, depth.LoadSnapshot(&Book{Bids: tt.BidLiquidity, LastUpdated: time.Now(), LastPushed: time.Now(), RestSnapshot: true}), "LoadSnapshot must not error for %s", tt.Name)
			movement, err := depth.bidLevels.getMovementByBase(tt.BaseAmount, tt.ReferencePrice, false)
			require.ErrorIs(t, err, tt.ExpectedError)

			if movement == nil {
				return
			}
			assert.Equalf(t, tt.ExpectedNominal, movement.NominalPercentage, "getMovementByBase nominal should match for %s", tt.Name)
			assert.Equalf(t, tt.ExpectedImpact, movement.ImpactPercentage, "getMovementByBase impact should match for %s", tt.Name)
			assert.Equalf(t, tt.ExpectedCost, movement.SlippageCost, "getMovementByBase cost should match for %s", tt.Name)
		})
	}
}

func assertMovement(tb testing.TB, expected, movement *Movement) {
	tb.Helper()
	assert.InDelta(tb, expected.Sold, movement.Sold, accuracy10dp, "Sold should be correct")
	assert.InDelta(tb, expected.Purchased, movement.Purchased, accuracy10dp, "Purchased should be correct")
	assert.InDelta(tb, expected.AverageOrderCost, movement.AverageOrderCost, accuracy10dp, "AverageOrderCost should be correct")
	assert.InDelta(tb, expected.StartPrice, movement.StartPrice, accuracy10dp, "StartPrice should be correct")
	assert.InDelta(tb, expected.EndPrice, movement.EndPrice, accuracy10dp, "EndPrice should be correct")
	assert.InDelta(tb, expected.NominalPercentage, movement.NominalPercentage, accuracy10dp, "NominalPercentage should be correct")
	assert.Equal(tb, expected.FullBookSideConsumed, movement.FullBookSideConsumed, "FullBookSideConsumed should be correct")
}

func TestGetBaseAmountFromNominalSlippage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name            string
		NominalSlippage float64
		ReferencePrice  float64
		BidLiquidity    Levels
		ExpectedShift   *Movement
		ExpectedError   error
	}{
		{
			Name:            "invalid slippage",
			NominalSlippage: -1,
			ExpectedError:   errInvalidNominalSlippage,
		},
		{
			Name:            "invalid slippage - larger than 100%",
			NominalSlippage: 101,
			ExpectedError:   errInvalidSlippageCannotExceed100,
		},
		{
			Name:            "no reference price",
			NominalSlippage: 1,
			ExpectedError:   errInvalidReferencePrice,
		},
		{
			Name:            "no liquidity to service quote amount",
			NominalSlippage: 1,
			ReferencePrice:  1000,
			ExpectedError:   errNoLiquidity,
		},
		{
			Name:            "thrasher test",
			BidLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			NominalSlippage: 1,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:              11,
				Purchased:         108900,
				AverageOrderCost:  9900,
				NominalPercentage: 1,
				StartPrice:        10000,
				EndPrice:          9800,
			},
		},
		{
			Name:            "consume first level - take one amount out of second",
			BidLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			NominalSlippage: 0.33333333333334,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:              3.0000000000000275,
				Purchased:         29900.00000000027,
				AverageOrderCost:  9966.666666666664,
				NominalPercentage: 0.33333333333334,
				StartPrice:        10000,
				EndPrice:          9900,
			},
		},
		{
			Name:            "consume full liquidity",
			BidLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			NominalSlippage: 10,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:                 12,
				Purchased:            118700,
				AverageOrderCost:     9891.666666666666,
				NominalPercentage:    1.0833333333333395,
				StartPrice:           10000,
				EndPrice:             9800,
				FullBookSideConsumed: true,
			},
		},
		{
			Name:            "scotts lovely slippery slippage requirements",
			BidLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			NominalSlippage: 0.00000000000000000000000000000000000000000000000000000000000000000000000000000000001,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:             2,
				Purchased:        20000,
				AverageOrderCost: 10000,
				StartPrice:       10000,
				EndPrice:         10000,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			depth := NewDepth(id)
			err := depth.LoadSnapshot(&Book{Bids: tt.BidLiquidity, LastUpdated: time.Now(), LastPushed: time.Now(), RestSnapshot: true})
			assert.NoError(t, err, "LoadSnapshot should not error")

			base, err := depth.bidLevels.hitBidsByNominalSlippage(tt.NominalSlippage, tt.ReferencePrice)
			if tt.ExpectedError != nil {
				assert.ErrorIs(t, err, tt.ExpectedError, "Should error correctly")
			} else {
				assertMovement(t, tt.ExpectedShift, base)
			}
		})
	}
}

// IsEqual is a tester function for comparison.
func (m *Movement) IsEqual(that *Movement) bool {
	if m == nil || that == nil {
		return m == nil && that == nil
	}
	return m.FullBookSideConsumed == that.FullBookSideConsumed &&
		m.Sold == that.Sold &&
		m.Purchased == that.Purchased &&
		m.NominalPercentage == that.NominalPercentage &&
		m.ImpactPercentage == that.ImpactPercentage &&
		m.EndPrice == that.EndPrice &&
		m.StartPrice == that.StartPrice &&
		m.AverageOrderCost == that.AverageOrderCost
}

func TestGetBaseAmountFromImpact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name           string
		ImpactSlippage float64
		ReferencePrice float64
		BidLiquidity   Levels
		ExpectedShift  *Movement
		ExpectedError  error
	}{
		{
			Name:          "invalid slippage",
			ExpectedError: errInvalidImpactSlippage,
		},
		{
			Name:           "invalid slippage - exceed 100%",
			ImpactSlippage: 101,
			ExpectedError:  errInvalidSlippageCannotExceed100,
		},
		{
			Name:           "no reference price",
			ImpactSlippage: 1,
			ExpectedError:  errInvalidReferencePrice,
		},
		{
			Name:           "no liquidity",
			ImpactSlippage: 1,
			ReferencePrice: 10000,
			ExpectedError:  errNoLiquidity,
		},
		{
			Name:           "thrasher test",
			BidLiquidity:   Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			ImpactSlippage: 1,
			ReferencePrice: 10000,
			ExpectedShift: &Movement{
				Sold:             2,
				Purchased:        20000,
				ImpactPercentage: 1,
				AverageOrderCost: 10000,
				StartPrice:       10000,
				EndPrice:         9900,
			},
		},
		{
			Name:           "consume first level and second level",
			BidLiquidity:   Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			ImpactSlippage: 2,
			ReferencePrice: 10000,
			ExpectedShift: &Movement{
				Sold:             9,
				Purchased:        89300,
				AverageOrderCost: 9922.222222222223,
				ImpactPercentage: 2,
				StartPrice:       10000,
				EndPrice:         9800,
			},
		},
		{
			Name:           "consume full liquidity",
			BidLiquidity:   Levels{{Price: 10000, Amount: 2}, {Price: 9900, Amount: 7}, {Price: 9800, Amount: 3}},
			ImpactSlippage: 10,
			ReferencePrice: 10000,
			ExpectedShift: &Movement{
				Sold:                 12,
				Purchased:            118700,
				ImpactPercentage:     FullLiquidityExhaustedPercentage,
				AverageOrderCost:     9891.666666666666,
				StartPrice:           10000,
				EndPrice:             9800,
				FullBookSideConsumed: true,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			depth := NewDepth(id)
			require.NoErrorf(t, depth.LoadSnapshot(&Book{Bids: tt.BidLiquidity, LastUpdated: time.Now(), LastPushed: time.Now(), RestSnapshot: true}), "LoadSnapshot must not error for %s", tt.Name)
			base, err := depth.bidLevels.hitBidsByImpactSlippage(tt.ImpactSlippage, tt.ReferencePrice)
			require.ErrorIs(t, err, tt.ExpectedError)

			if base == nil || tt.ExpectedShift == nil {
				assert.Equalf(t, tt.ExpectedShift, base, "hitBidsByImpactSlippage should match expected shift for %s", tt.Name)
			} else {
				assert.Truef(t, base.IsEqual(tt.ExpectedShift), "hitBidsByImpactSlippage should produce expected movement for %s", tt.Name)
			}
		})
	}
}

func TestGetMovementByQuoteAmount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name            string
		QuoteAmount     float64
		ReferencePrice  float64
		AskLiquidity    Levels
		ExpectedNominal float64
		ExpectedImpact  float64
		ExpectedCost    float64
		ExpectedError   error
	}{
		{
			Name:          "no amount",
			ExpectedError: errQuoteAmountInvalid,
		},
		{
			Name:          "no reference price",
			QuoteAmount:   1,
			ExpectedError: errInvalidReferencePrice,
		},
		{
			Name:           "not enough liquidity to service quote amount",
			QuoteAmount:    1,
			ReferencePrice: 1000,
			ExpectedError:  errNoLiquidity,
		},
		{
			Name:            "thrasher test",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			QuoteAmount:     100900,
			ReferencePrice:  10000,
			ExpectedNominal: 0.8999999999999999,
			ExpectedImpact:  2,
			ExpectedCost:    900,
		},
		{
			Name:            "consume first level",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			QuoteAmount:     20000,
			ReferencePrice:  10000,
			ExpectedNominal: 0,
			ExpectedImpact:  1,
			ExpectedCost:    0,
		},
		{
			Name:            "consume most of first level",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			QuoteAmount:     15000,
			ReferencePrice:  10000,
			ExpectedNominal: 0,
			ExpectedImpact:  0,
			ExpectedCost:    0,
		},
		{
			Name:            "consume full liquidity",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			QuoteAmount:     121300,
			ReferencePrice:  10000,
			ExpectedNominal: 1.0833333333333395,
			ExpectedImpact:  FullLiquidityExhaustedPercentage,
			ExpectedCost:    1300,
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			depth := NewDepth(id)
			require.NoErrorf(t, depth.LoadSnapshot(&Book{Asks: tt.AskLiquidity, LastUpdated: time.Now(), LastPushed: time.Now(), RestSnapshot: true}), "LoadSnapshot must not error for %s", tt.Name)
			movement, err := depth.askLevels.getMovementByQuotation(tt.QuoteAmount, tt.ReferencePrice, false)
			require.ErrorIs(t, err, tt.ExpectedError)

			if movement == nil {
				return
			}
			assert.Equalf(t, tt.ExpectedNominal, movement.NominalPercentage, "getMovementByQuoteAmount nominal should match for %s", tt.Name)
			assert.Equalf(t, tt.ExpectedImpact, movement.ImpactPercentage, "getMovementByQuoteAmount impact should match for %s", tt.Name)
			assert.Equalf(t, tt.ExpectedCost, movement.SlippageCost, "getMovementByQuoteAmount cost should match for %s", tt.Name)
		})
	}
}

func TestGetQuoteAmountFromNominalSlippage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name            string
		NominalSlippage float64
		ReferencePrice  float64
		AskLiquidity    Levels
		ExpectedShift   *Movement
		ExpectedError   error
	}{
		{
			Name:            "invalid slippage",
			NominalSlippage: -1,
			ExpectedError:   errInvalidNominalSlippage,
		},
		{
			Name:            "no reference price",
			NominalSlippage: 1,
			ExpectedError:   errInvalidReferencePrice,
		},
		{
			Name:            "no liquidity",
			NominalSlippage: 1,
			ReferencePrice:  10000,
			ExpectedError:   errNoLiquidity,
		},
		{
			Name:            "consume first level - one amount on second level",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			NominalSlippage: 0.33333333333334,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:              30100.000000000276, // <- expected rounding issue
				Purchased:         3.0000000000000275,
				AverageOrderCost:  10033.333333333333333333333333333,
				NominalPercentage: 0.33333333333334,
				StartPrice:        10000,
				EndPrice:          10100,
			},
		},
		{
			Name:            "last level total agg meeting 1 percent nominally",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			NominalSlippage: 1,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:              111100,
				Purchased:         11,
				AverageOrderCost:  10100,
				NominalPercentage: 1,
				StartPrice:        10000,
				EndPrice:          10200,
			},
		},
		{
			Name:            "take full second level",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			NominalSlippage: 0.7777777777777738,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:              90700,
				Purchased:         9,
				AverageOrderCost:  10077.777777777777,
				NominalPercentage: 0.7777777777777738,
				StartPrice:        10000,
				EndPrice:          10100,
			},
		},
		{
			Name:            "consume full liquidity",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			NominalSlippage: 10,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:                 121300,
				Purchased:            12,
				AverageOrderCost:     10108.333333333334,
				NominalPercentage:    1.0833333333333395,
				StartPrice:           10000,
				EndPrice:             10200,
				FullBookSideConsumed: true,
			},
		},
		{
			Name:            "scotts lovely slippery slippage requirements",
			AskLiquidity:    Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			NominalSlippage: 0.00000000000000000000000000000000000000000000000000000000000000000000000000000000001,
			ReferencePrice:  10000,
			ExpectedShift: &Movement{
				Sold:             20000,
				Purchased:        2,
				AverageOrderCost: 10000,
				StartPrice:       10000,
				EndPrice:         10000,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			depth := NewDepth(id)
			require.NoErrorf(t, depth.LoadSnapshot(&Book{Asks: tt.AskLiquidity, LastUpdated: time.Now(), LastPushed: time.Now(), RestSnapshot: true}), "LoadSnapshot must not error for %s", tt.Name)

			quote, err := depth.askLevels.liftAsksByNominalSlippage(tt.NominalSlippage, tt.ReferencePrice)
			if tt.ExpectedError != nil {
				assert.ErrorIs(t, err, tt.ExpectedError, "Should error correctly")
			} else {
				assertMovement(t, tt.ExpectedShift, quote)
			}
		})
	}
}

func TestGetQuoteAmountFromImpact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name           string
		ImpactSlippage float64
		ReferencePrice float64
		AskLiquidity   Levels
		ExpectedShift  *Movement
		ExpectedError  error
	}{
		{
			Name:           "invalid slippage",
			ImpactSlippage: -1,
			ExpectedError:  errInvalidImpactSlippage,
		},
		{
			Name:           "no reference price",
			ImpactSlippage: 1,
			ExpectedError:  errInvalidReferencePrice,
		},
		{
			Name:           "no liquidity",
			ImpactSlippage: 1,
			ReferencePrice: 1000,
			ExpectedError:  errNoLiquidity,
		},
		{
			Name:           "thrasher test",
			AskLiquidity:   Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			ImpactSlippage: 1,
			ReferencePrice: 10000,
			ExpectedShift: &Movement{
				Sold:             20000,
				Purchased:        2,
				AverageOrderCost: 10000,
				ImpactPercentage: 1,
				StartPrice:       10000,
				EndPrice:         10100,
			},
		},
		{
			Name:           "consume first level and second level",
			AskLiquidity:   Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			ImpactSlippage: 2,
			ReferencePrice: 10000,
			ExpectedShift: &Movement{
				Sold:             90700,
				Purchased:        9,
				AverageOrderCost: 10077.777777777777777777777777778,
				ImpactPercentage: 2,
				StartPrice:       10000,
				EndPrice:         10200,
			},
		},
		{
			Name:           "consume full liquidity",
			AskLiquidity:   Levels{{Price: 10000, Amount: 2}, {Price: 10100, Amount: 7}, {Price: 10200, Amount: 3}},
			ImpactSlippage: 10,
			ReferencePrice: 10000,
			ExpectedShift: &Movement{
				Sold:                 121300,
				Purchased:            12,
				AverageOrderCost:     10108.333333333333333333333333333,
				ImpactPercentage:     FullLiquidityExhaustedPercentage,
				StartPrice:           10000,
				EndPrice:             10200,
				FullBookSideConsumed: true,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			depth := NewDepth(id)
			require.NoErrorf(t, depth.LoadSnapshot(&Book{Asks: tt.AskLiquidity, LastUpdated: time.Now(), LastPushed: time.Now(), RestSnapshot: true}), "LoadSnapshot must not error for %s", tt.Name)

			quote, err := depth.askLevels.liftAsksByImpactSlippage(tt.ImpactSlippage, tt.ReferencePrice)
			if tt.ExpectedError != nil {
				assert.ErrorIs(t, err, tt.ExpectedError, "Should error correctly")
			} else {
				assertMovement(t, tt.ExpectedShift, quote)
			}
		})
	}
}

func TestGetHeadPrice(t *testing.T) {
	t.Parallel()
	depth := NewDepth(id)
	_, err := depth.bidLevels.getHeadPriceNoLock()
	require.ErrorIs(t, err, errNoLiquidity)
	_, err = depth.askLevels.getHeadPriceNoLock()
	require.ErrorIs(t, err, errNoLiquidity)

	err = depth.LoadSnapshot(&Book{Bids: bid, Asks: ask, LastUpdated: time.Now(), LastPushed: time.Now(), RestSnapshot: true})
	require.NoError(t, err, "LoadSnapshot must not error")

	val, err := depth.bidLevels.getHeadPriceNoLock()
	require.NoError(t, err)

	assert.Equal(t, 1336.0, val, "getHeadPriceNoLock should return expected bid head price")

	val, err = depth.askLevels.getHeadPriceNoLock()
	require.NoError(t, err)

	assert.Equal(t, 1337.0, val, "getHeadPriceNoLock should return expected ask head price")
}

func TestFinalizeFields(t *testing.T) {
	m := &Movement{}
	_, err := m.finalizeFields(0, 0, 0, 0, false)
	assert.ErrorIs(t, err, errInvalidCost, "finalizeFields should error when cost is invalid")

	_, err = m.finalizeFields(1, 0, 0, 0, false)
	assert.ErrorIs(t, err, errInvalidAmount, "finalizeFields should error when amount is invalid")

	_, err = m.finalizeFields(1, 1, 0, 0, false)
	assert.ErrorIs(t, err, errInvalidHeadPrice, "finalizeFields should error correctly with bad head price")

	// Test slippage as per https://en.wikipedia.org/wiki/Slippage_(finance)
	mov, err := m.finalizeFields(20000*151.11585, 20000, 151.08, 0, false)
	assert.NoError(t, err, "finalizeFields should not error")
	assert.InDelta(t, 717.0, mov.SlippageCost, 0.000000001, "SlippageCost should be correct")
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
