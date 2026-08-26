package kraken

import (
	"hash/crc32"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
)

// oldTrim and oldChecksum are the implementations replaced, kept as the reference
func oldTrim(s string) string {
	s = strings.Replace(s, ".", "", 1)
	return strings.TrimLeft(s, "0")
}

func oldChecksum(b *orderbook.Book) uint32 {
	var checkStr strings.Builder
	for i := 0; i < 10 && i < len(b.Asks); i++ {
		checkStr.WriteString(oldTrim(b.Asks[i].StrPrice + oldTrim(b.Asks[i].StrAmount)))
	}
	for i := 0; i < 10 && i < len(b.Bids); i++ {
		checkStr.WriteString(oldTrim(b.Bids[i].StrPrice) + oldTrim(b.Bids[i].StrAmount))
	}
	return crc32.ChecksumIEEE([]byte(checkStr.String()))
}

func TestCRC32MatchesPrevious(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic corpus
	digits := func() string {
		switch r.IntN(6) {
		case 0:
			return "0." + strings.Repeat("0", r.IntN(5)) + strconv.Itoa(r.IntN(1000))
		case 1:
			return strings.Repeat("0", r.IntN(3)) + strconv.FormatFloat(r.Float64()*1000, 'f', r.IntN(9), 64)
		case 2:
			return "0.0"
		case 3:
			return strconv.Itoa(r.IntN(100000)) // no point at all
		case 4:
			return "000"
		default:
			return strconv.FormatFloat(r.Float64()*100000, 'f', r.IntN(9), 64)
		}
	}
	for range 20_000 {
		n := r.IntN(12)
		bids := make(orderbook.Levels, n)
		asks := make(orderbook.Levels, n)
		for i := range n {
			bids[i] = orderbook.Level{StrPrice: digits(), StrAmount: digits()}
			asks[i] = orderbook.Level{StrPrice: digits(), StrAmount: digits()}
		}
		want := oldChecksum(&orderbook.Book{Bids: bids, Asks: asks})
		top := func(l orderbook.Levels) orderbook.Levels {
			if len(l) > krakenChecksumLevels {
				return l[:krakenChecksumLevels]
			}
			return l
		}
		require.NoError(t, validateCRC32(top(bids), top(asks), currency.NewBTCUSD(), want),
			"the rewritten checksum must match the previous implementation")
	}
}
