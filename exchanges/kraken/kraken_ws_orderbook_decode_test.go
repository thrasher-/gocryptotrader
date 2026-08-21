package kraken

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

// decodePerItem is the per entry path the slice scanner replaced, kept as its reference
func decodePerItem(data []byte) ([]wsOrderbookItem, error) {
	var v []wsOrderbookItem
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func requireSameItems(t *testing.T, payload string, want []wsOrderbookItem, got wsOrderbookItems) {
	t.Helper()
	require.Lenf(t, got, len(want), "scanner must decode %s to %d entries", payload, len(want))
	for i := range want {
		require.Equalf(t, want[i], got[i], "entry %d of %s must match the reference", i, payload)
	}
}

func TestWsOrderbookItemsMatchesPerItemDecode(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		`[["1.5","2.5","1534614248.123678"]]`,
		`[["1.5","2.5","1534614248.123678"],["1.6","0.0","1534614249.000000"]]`,
		`[]`, `[ ]`, "[\n\t]",
		`[ [ "1.5" , "2.5" , "1534614248.123678" ] ]`,
		`[["0.00000100","0.00000000","1534614248"]]`,
		`[["1.5","2.5","1534614248.123678","surplus"]]`,
		`[["1.5","2.5"]]`,
		// forms the reference rejects
		`[["1.5","2.5","x"]]`, `[[1.5,"2.5","1"]]`, `[["a","2.5","1"]]`, `[["1.5","b","1"]]`,
		`[[]]`, `[["1.5"]]`, `[`, ``, `null`, `{}`, `[[1,2],]`, `[["1.5","2.5","1"]] x`,
		`[["1.5","2.5","1"]`, `[["1.5","2.5","1"] ["1.6","2.6","2"]]`,
	} {
		want, wantErr := decodePerItem([]byte(payload))
		var got wsOrderbookItems
		gotErr := got.UnmarshalJSON([]byte(payload))
		if wantErr != nil {
			assert.Errorf(t, gotErr, "scanner should reject %s which the reference rejects with %v", payload, wantErr)
			continue
		}
		require.NoErrorf(t, gotErr, "scanner must accept %s which the reference accepts", payload)
		requireSameItems(t, payload, want, got)
	}
}

func TestWsOrderbookItemsFuzzMatchesPerItemDecode(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(0xC0FFEE, 0xBEEF)) //nolint:gosec // deterministically seeded so a failing corpus is reproducible
	var accepted, rejected int
	for range 20_000 {
		var b strings.Builder
		sp := func() {
			for range r.IntN(3) {
				b.WriteByte(" \t\r\n"[r.IntN(4)])
			}
		}
		b.WriteByte('[')
		for n := r.IntN(10); n > 0; n-- {
			if b.Len() > 1 {
				b.WriteByte(',')
			}
			sp()
			b.WriteByte('[')
			sp()
			b.WriteString(`"` + strconv.FormatFloat(r.Float64()*100000, 'f', r.IntN(9), 64) + `"`)
			sp()
			b.WriteString(`,"` + strconv.FormatFloat(r.Float64()*10, 'f', r.IntN(9), 64) + `"`)
			sp()
			b.WriteString(`,"` + strconv.FormatFloat(1534614248+r.Float64(), 'f', r.IntN(7), 64) + `"`)
			sp()
			b.WriteByte(']')
		}
		sp()
		b.WriteByte(']')
		payload := b.String()
		if r.IntN(3) == 0 {
			p := []byte(payload)
			p[r.IntN(len(p))] = " ,[]\"0.eN-null"[r.IntN(14)]
			payload = string(p)
		}
		want, wantErr := decodePerItem([]byte(payload))
		var got wsOrderbookItems
		gotErr := got.UnmarshalJSON([]byte(payload))
		if wantErr != nil {
			rejected++
			require.Errorf(t, gotErr, "scanner must reject %q which the reference rejects with %v", payload, wantErr)
			continue
		}
		accepted++
		require.NoErrorf(t, gotErr, "scanner must accept %q which the reference accepts", payload)
		requireSameItems(t, payload, want, got)
	}
	t.Logf("accepted by both: %d, rejected by both: %d", accepted, rejected)
	assert.Positive(t, accepted, "the corpus should contain well formed payloads")
	assert.Positive(t, rejected, "the corpus should contain malformed payloads")
}
