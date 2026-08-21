package okx

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// decodeRowsViaReflection is the path the scanner replaced, kept as its reference
func decodeRowsViaReflection(data []byte) ([][4]types.Number, error) {
	var v [][4]types.Number
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func requireSameRows(t *testing.T, payload string, want [][4]types.Number, got WsOrderBookLevels) {
	t.Helper()
	require.Lenf(t, got, len(want), "scanner must decode %s to %d rows", payload, len(want))
	for i := range want {
		assert.Equalf(t, want[i], got[i], "row %d of %s should match the reference", i, payload)
	}
}

func TestWsOrderBookLevelsMatchesReflection(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		`[["1","2","0","3"]]`, `[[1,2,0,3]]`, `[["1.5","2.5","0","3"],["1.6","0","1","4"]]`,
		`[]`, `[ ]`, `null`, `[["1","2"]]`, `[["1","2","0","3","surplus"]]`, `[[]]`,
		`[ [ "1" , "2" , "0" , "3" ] ]`, `[["","","",""]]`, `[[null,null,null,null]]`,
		// forms the reference rejects
		`[["1","2","0","3"]`, `[`, ``, `{}`, `[[1,2],]`, `[["1","2","0","3"]] x`,
		`[[1,2,3,4] [5,6,7,8]]`, `[[1,,2,3]]`, `[["1","2","0","3"]]]`,
	} {
		want, wantErr := decodeRowsViaReflection([]byte(payload))
		var got WsOrderBookLevels
		gotErr := got.UnmarshalJSON([]byte(payload))
		if wantErr != nil {
			assert.Errorf(t, gotErr, "scanner should reject %s which the reference rejects with %v", payload, wantErr)
			continue
		}
		require.NoErrorf(t, gotErr, "scanner must accept %s which the reference accepts", payload)
		requireSameRows(t, payload, want, got)
	}
}

func TestWsOrderBookLevelsFuzzMatchesReflection(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(0x0C7, 0x5EED)) //nolint:gosec // deterministically seeded corpus
	var accepted, rejected int
	for range 20_000 {
		var b strings.Builder
		sp := func() {
			for range r.IntN(3) {
				b.WriteByte(" \t\r\n"[r.IntN(4)])
			}
		}
		b.WriteByte('[')
		for n := r.IntN(8); n > 0; n-- {
			if b.Len() > 1 {
				b.WriteByte(',')
			}
			sp()
			b.WriteByte('[')
			for cell := range 4 {
				if cell > 0 {
					b.WriteByte(',')
				}
				sp()
				v := strconv.FormatFloat(r.Float64()*10000, 'f', r.IntN(9), 64)
				if r.IntN(2) == 0 {
					b.WriteString(`"` + v + `"`)
				} else {
					b.WriteString(v)
				}
				sp()
			}
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
		want, wantErr := decodeRowsViaReflection([]byte(payload))
		var got WsOrderBookLevels
		gotErr := got.UnmarshalJSON([]byte(payload))
		if wantErr != nil {
			rejected++
			require.Errorf(t, gotErr, "scanner must reject %q which the reference rejects with %v", payload, wantErr)
			continue
		}
		accepted++
		require.NoErrorf(t, gotErr, "scanner must accept %q which the reference accepts", payload)
		requireSameRows(t, payload, want, got)
	}
	t.Logf("accepted by both: %d, rejected by both: %d", accepted, rejected)
	assert.Positive(t, accepted, "the corpus should contain well formed payloads")
	assert.Positive(t, rejected, "the corpus should contain malformed payloads")
}
