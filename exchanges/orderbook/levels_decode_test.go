package orderbook

import (
	"bytes"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// decodeViaReflection is the implementation the scanner replaced, kept as its reference
func decodeViaReflection(data []byte) (Levels, error) {
	var v [][2]types.Number
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	out := make(Levels, len(v))
	for x := range v {
		out[x].Price = v[x][0].Float64()
		out[x].Amount = v[x][1].Float64()
	}
	return out, nil
}

func equalLevels(a, b Levels) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		// NaN is reachable through a quoted price, so compare bit patterns rather than values
		if math.Float64bits(a[i].Price) != math.Float64bits(b[i].Price) ||
			math.Float64bits(a[i].Amount) != math.Float64bits(b[i].Amount) {
			return false
		}
	}
	return true
}

func TestLevelsArrayPriceAmountMatchesReflection(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		`[[1,2]]`, `[[1,2,3]]`, `[[1,2,3,4]]`, `[[1]]`, `[[]]`, `[]`, `null`,
		// surplus of any type was discarded by the decoder this replaced
		`[[1,2,"metadata"]]`, `[[1,2,{}]]`, `[[1,2,[3]]]`, `[[1,2,true]]`, `[[1,2,false]]`,
		`[[1,2,null]]`, `[[1,2,{"a":[1,"]"]},"x"]]`, `[[1,2,"br]ackets"]]`,
		`[[1,2],[3,4]]`, `[ [ "1.5" , "2.5" ] ]`, `[["",""]]`, `[[null,null]]`,
		`[["1e3","2E-2"]]`, `[[1,"2"]]`, `[["+1","2"]]`, `[[".5","2"]]`, `[["1.","2"]]`,
		`[["01","2"]]`, `[["1_0","2"]]`, `[["infinity","2"]]`, `[["NaN","Inf"]]`,
		`[[-1.5,-2.5]]`, `[[0,0]]`, `[[1e-9,1E+9]]`, "[[1,2]]\n", " \t[[1,2]]\r\n",
		// forms the reference rejects
		`[[true,false]]`, `[[+1,2]]`, `[[.5,2]]`, `[[1.,2]]`, `[[01,2]]`, `[[1,2],]`,
		`[[1,2]`, `[[1,2]]]`, `[["0x10","2"]]`, `[[" 1","2"]]`, `[[1,2]] x`, `[`, ``,
		`[[1,,2]]`, `[[1e,2]]`, `[[-,2]]`, `[["1,2]]`, `{}`, `[{}]`, `[[1,2],`,
	} {
		want, wantErr := decodeViaReflection([]byte(payload))
		var got LevelsArrayPriceAmount
		gotErr := got.UnmarshalJSON([]byte(payload))
		if wantErr != nil {
			assert.Errorf(t, gotErr, "scanner should reject %s which the reference rejects with %v", payload, wantErr)
			continue
		}
		require.NoErrorf(t, gotErr, "scanner must accept %s which the reference accepts", payload)
		assert.Truef(t, equalLevels(want, Levels(got)), "scanner should decode %s to %v, got %v", payload, want, Levels(got))
	}
}

// randomPayload varies quoting, spacing and number shape across the forms venues send
func randomPayload(r *rand.Rand) string {
	var b strings.Builder
	sp := func() {
		for range r.IntN(3) {
			b.WriteByte(" \t\r\n"[r.IntN(4)])
		}
	}
	b.WriteByte('[')
	for n := r.IntN(12); n > 0; n-- {
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		sp()
		b.WriteByte('[')
		for f := range 2 {
			if f > 0 {
				sp()
				b.WriteByte(',')
			}
			sp()
			switch r.IntN(8) {
			case 0:
				b.WriteString("null")
			case 1:
				b.WriteString(`""`)
			default:
				v := strconv.FormatFloat((r.Float64()-0.5)*math.Pow(10, float64(r.IntN(12)-4)), []byte{'f', 'e', 'g'}[r.IntN(3)], r.IntN(10)-1, 64)
				if r.IntN(2) == 0 {
					b.WriteString(`"` + v + `"`)
				} else {
					b.WriteString(v)
				}
			}
		}
		sp()
		b.WriteByte(']')
	}
	sp()
	b.WriteByte(']')
	return b.String()
}

func TestLevelsArrayPriceAmountFuzzMatchesReflection(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(0x5DEECE66D, 0xB)) //nolint:gosec // deterministically seeded so a failing corpus is reproducible
	var accepted, rejected int
	for range 20_000 {
		payload := randomPayload(r)
		if r.IntN(3) == 0 { // corrupt a byte to exercise the rejection paths
			if b := []byte(payload); len(b) > 0 {
				b[r.IntN(len(b))] = " ,[]\"0e+-nul{}"[r.IntN(13)]
				payload = string(b)
			}
		}
		want, wantErr := decodeViaReflection([]byte(payload))
		var got LevelsArrayPriceAmount
		gotErr := got.UnmarshalJSON([]byte(payload))
		if wantErr != nil {
			rejected++
			require.Errorf(t, gotErr, "scanner must reject %q which the reference rejects with %v", payload, wantErr)
			continue
		}
		accepted++
		require.NoErrorf(t, gotErr, "scanner must accept %q which the reference accepts", payload)
		require.Truef(t, equalLevels(want, Levels(got)), "scanner must decode %q to %v, got %v", payload, want, Levels(got))
	}
	t.Logf("accepted by both: %d, rejected by both: %d", accepted, rejected)
	assert.Positive(t, accepted, "the corpus should contain well formed payloads")
	assert.Positive(t, rejected, "the corpus should contain malformed payloads")
}

// TestLevelsArrayPriceAmountPresizeBounded guards the capacity estimate, which is taken from the
// payload length and would otherwise let a run of whitespace reserve levels that never arrive
func TestLevelsArrayPriceAmountPresizeBounded(t *testing.T) {
	t.Parallel()
	data := append(append([]byte{'['}, bytes.Repeat([]byte{' '}, 1<<20)...), ']')
	var l LevelsArrayPriceAmount
	require.NoError(t, l.UnmarshalJSON(data))
	assert.Empty(t, l, "whitespace should decode to no levels")
	assert.LessOrEqual(t, cap(l), maxPresizedLevels, "capacity should be bounded regardless of payload length")
}

func TestLevelsArrayPriceAmountSentinel(t *testing.T) {
	t.Parallel()
	var l LevelsArrayPriceAmount
	require.ErrorIs(t, l.UnmarshalJSON([]byte(`[[1,2}]`)), errMalformedLevels, "a malformed array must report the sentinel")
	require.ErrorIs(t, l.UnmarshalJSON([]byte(`[[1,2]] x`)), errMalformedLevels, "trailing data must report the sentinel")
}

// TestLevelsArrayPriceAmountOrderCountMatchesReflection holds the three cell scanner against the
// [][3]types.Number decode it replaced, over generated payloads.
func TestLevelsArrayPriceAmountOrderCountMatchesReflection(t *testing.T) {
	t.Parallel()

	viaReflection := func(data []byte) (Levels, error) {
		var rows [][3]types.Number
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		out := make(Levels, len(rows))
		for i := range rows {
			out[i].Price = rows[i][0].Float64()
			out[i].Amount = rows[i][1].Float64()
			out[i].OrderCount = rows[i][2].Int64()
		}
		return out, nil
	}

	for _, payload := range []string{
		`[]`,
		`[["100.5","2.25","7"]]`,
		`[[100.5,2.25,7]]`,
		`[["0.00000100","1e-8","0"],[1.5,"2",3]]`,
		`[ [ "1" , "2" , "3" ] , [ 4 , 5 , 6 ] ]`,
		`[["100","1","2"],["101","2","3"],["102","3","4"]]`,
	} {
		want, err := viaReflection([]byte(payload))
		require.NoErrorf(t, err, "the reflection decode must accept %s", payload)

		var got LevelsArrayPriceAmountOrderCount
		require.NoErrorf(t, got.UnmarshalJSON([]byte(payload)), "the scanner must accept %s", payload)
		assert.Equalf(t, want, got.Levels(), "the scanner should match the reflection decode for %s", payload)
	}
}

func TestLevelsArrayPriceAmountOrderCountFuzzMatchesReflection(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(9, 12))
	for range 2000 {
		rows := rng.IntN(6)
		var sb strings.Builder
		sb.WriteByte('[')
		for i := range rows {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(`[`)
			for cell := range 3 {
				if cell > 0 {
					sb.WriteByte(',')
				}
				v := strconv.FormatFloat(rng.Float64()*1000, 'f', rng.IntN(9), 64)
				if rng.IntN(2) == 0 {
					sb.WriteString(`"` + v + `"`)
				} else {
					sb.WriteString(v)
				}
			}
			sb.WriteString(`]`)
		}
		sb.WriteByte(']')
		payload := []byte(sb.String())

		var rowsOut [][3]types.Number
		require.NoErrorf(t, json.Unmarshal(payload, &rowsOut), "the reflection decode must accept %s", payload)
		want := make(Levels, len(rowsOut))
		for i := range rowsOut {
			want[i].Price = rowsOut[i][0].Float64()
			want[i].Amount = rowsOut[i][1].Float64()
			want[i].OrderCount = rowsOut[i][2].Int64()
		}

		var got LevelsArrayPriceAmountOrderCount
		require.NoErrorf(t, got.UnmarshalJSON(payload), "the scanner must accept %s", payload)
		assert.Equalf(t, want, got.Levels(), "the scanner should match the reflection decode for %s", payload)
	}
}
