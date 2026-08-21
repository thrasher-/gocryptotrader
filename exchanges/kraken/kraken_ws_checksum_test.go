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

func oldChecksum(bidDigits, askDigits []orderbook.LevelDigits) uint32 {
	var checkStr strings.Builder
	for i := 0; i < 10 && i < len(askDigits); i++ {
		checkStr.WriteString(oldTrim(askDigits[i].Price + oldTrim(askDigits[i].Amount)))
	}
	for i := 0; i < 10 && i < len(bidDigits); i++ {
		checkStr.WriteString(oldTrim(bidDigits[i].Price) + oldTrim(bidDigits[i].Amount))
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
		bids := make([]orderbook.LevelDigits, n)
		asks := make([]orderbook.LevelDigits, n)
		for i := range n {
			bids[i] = orderbook.LevelDigits{Price: digits(), Amount: digits()}
			asks[i] = orderbook.LevelDigits{Price: digits(), Amount: digits()}
		}
		want := oldChecksum(bids, asks)
		top := func(l []orderbook.LevelDigits) []orderbook.LevelDigits {
			if len(l) > krakenChecksumLevels {
				return l[:krakenChecksumLevels]
			}
			return l
		}
		require.NoError(t, validateCRC32(top(bids), top(asks), currency.NewBTCUSD(), want),
			"the rewritten checksum must match the previous implementation")
	}
}
