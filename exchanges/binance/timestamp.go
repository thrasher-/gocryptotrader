package binance

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Timestamp decodes Binance's millisecond timestamps, including -1 sentinels,
// and the UTC date strings returned by withdrawal history.
type Timestamp time.Time

// UnmarshalJSON preserves the endpoint's millisecond unit instead of inferring
// a unit from the number of digits in a timestamp.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*t = Timestamp{}
		return nil
	}
	value := string(data)
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("%w: %w", types.ErrInvalidTimestampFormat, err)
		}
	}
	if value == "" || value == "0" {
		*t = Timestamp{}
		return nil
	}
	if milliseconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		*t = Timestamp(time.UnixMilli(milliseconds).UTC())
		return nil
	}
	for _, layout := range []string{time.DateTime, time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, value); err == nil {
			*t = Timestamp(parsed.UTC())
			return nil
		}
	}
	return fmt.Errorf("%w: %q", types.ErrInvalidTimestampFormat, value)
}

// MarshalJSON returns the underlying time in RFC 3339 format.
func (t Timestamp) MarshalJSON() ([]byte, error) { return time.Time(t).MarshalJSON() }

// Time returns the underlying timestamp.
func (t Timestamp) Time() time.Time { return time.Time(t) }
