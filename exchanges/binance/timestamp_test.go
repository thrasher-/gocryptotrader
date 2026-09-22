package binance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func TestTimestamp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    string
		expected time.Time
	}{
		{"123456789", time.UnixMilli(123456789)},
		{"-1", time.UnixMilli(-1)},
		{`"-1"`, time.UnixMilli(-1)},
		{"1684804350068", time.UnixMilli(1684804350068)},
		{`"2019-10-12 11:12:02"`, time.Date(2019, 10, 12, 11, 12, 2, 0, time.UTC)},
		{`"2019-10-12T13:12:02+02:00"`, time.Date(2019, 10, 12, 11, 12, 2, 0, time.UTC)},
		{"null", time.Time{}},
		{"0", time.Time{}},
		{`""`, time.Time{}},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			value := Timestamp(time.Now())
			require.NoError(t, json.Unmarshal([]byte(tc.input), &value), "documented timestamp must decode")
			assert.True(t, value.Time().Equal(tc.expected), "timestamp should retain the documented time and unit")
		})
	}
	for _, input := range []string{`"invalid"`, `true`, `{}`, `"2019-20-50 00:00:00"`} {
		var value Timestamp
		assert.ErrorIs(t, value.UnmarshalJSON([]byte(input)), types.ErrInvalidTimestampFormat, "invalid timestamp should return its sentinel")
	}
}
