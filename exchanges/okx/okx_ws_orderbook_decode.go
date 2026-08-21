package okx

import (
	"errors"
	"fmt"

	"github.com/thrasher-corp/gocryptotrader/types"
)

var errMalformedOrderbookRows = errors.New("malformed orderbook rows")

// WsOrderBookLevels is a websocket book side, each row holding price, amount, liquidated orders and
// order count. Its underlying type is unchanged, so indexing and assignment behave as before.
type WsOrderBookLevels [][4]types.Number

// UnmarshalJSON implements json.Unmarshaler.
//
// Only the array structure is scanned here; each cell is still handed to types.Number, so what a
// value means is unchanged. Going through encoding/json cost reflection for every cell of every row.
func (l *WsOrderBookLevels) UnmarshalJSON(data []byte) error {
	i := skipRowSpace(data, 0)
	if i+4 <= len(data) && string(data[i:i+4]) == "null" {
		*l = nil
		return nil
	}
	if i == len(data) || data[i] != '[' {
		return fmt.Errorf("%w: expected '['", errMalformedOrderbookRows)
	}

	rows := make(WsOrderBookLevels, 0, min(len(data)/24+1, maxOrderbookRows))
	i = skipRowSpace(data, i+1)
	for i < len(data) && data[i] != ']' {
		if data[i] != '[' {
			return fmt.Errorf("%w: expected '['", errMalformedOrderbookRows)
		}
		var row [4]types.Number
		i++
		for cell := 0; ; cell++ {
			if i = skipRowSpace(data, i); i == len(data) {
				return fmt.Errorf("%w: unterminated row", errMalformedOrderbookRows)
			}
			if data[i] == ']' {
				i++
				break
			}
			if cell > 0 {
				if data[i] != ',' {
					return fmt.Errorf("%w: expected ',' or ']'", errMalformedOrderbookRows)
				}
				if i = skipRowSpace(data, i+1); i == len(data) {
					return fmt.Errorf("%w: unterminated row", errMalformedOrderbookRows)
				}
			}
			start := i
			if i = skipRowValue(data, i); i < 0 {
				return fmt.Errorf("%w: expected a value", errMalformedOrderbookRows)
			}
			if err := validRowLiteral(data[start:i]); err != nil {
				return err
			}
			if cell < len(row) {
				if err := row[cell].UnmarshalJSON(data[start:i]); err != nil {
					return err
				}
			}
		}
		rows = append(rows, row)

		if i = skipRowSpace(data, i); i == len(data) {
			break
		}
		switch data[i] {
		case ']':
		case ',':
			if i = skipRowSpace(data, i+1); i < len(data) && data[i] == ']' {
				return fmt.Errorf("%w: trailing comma", errMalformedOrderbookRows)
			}
		default:
			return fmt.Errorf("%w: expected ',' or ']'", errMalformedOrderbookRows)
		}
	}
	if i == len(data) {
		return fmt.Errorf("%w: unterminated array", errMalformedOrderbookRows)
	}
	if skipRowSpace(data, i+1) != len(data) {
		return fmt.Errorf("%w: trailing data", errMalformedOrderbookRows)
	}
	*l = rows
	return nil
}

// maxOrderbookRows bounds the capacity guessed from the payload length, which whitespace inflates
// without producing any rows.
const maxOrderbookRows = 4096

// skipRowValue returns the index after one cell, or a negative number when there is not one there.
func skipRowValue(data []byte, i int) int {
	if data[i] == '"' {
		for j := i + 1; j < len(data); j++ {
			if data[j] == '\\' {
				j++
				continue
			}
			if data[j] == '"' {
				return j + 1
			}
		}
		return -1
	}
	start := i
	for ; i < len(data); i++ {
		switch data[i] {
		case ',', ']', '}', ' ', '\t', '\r', '\n':
			if i == start {
				return -1
			}
			return i
		}
	}
	if i == start {
		return -1
	}
	return i
}

func skipRowSpace(data []byte, i int) int {
	for ; i < len(data); i++ {
		switch data[i] {
		case ' ', '\t', '\r', '\n':
		default:
			return i
		}
	}
	return i
}

// validRowLiteral screens an unquoted cell before types.Number sees it, which reads anything
// beginning with an n as null and so accepted corrupted tokens the decoder this replaced rejected.
func validRowLiteral(v []byte) error {
	if len(v) == 0 {
		return fmt.Errorf("%w: expected a value", errMalformedOrderbookRows)
	}
	if v[0] == '"' {
		return nil
	}
	switch string(v) {
	case "null", "true", "false":
		return nil
	}
	i := 0
	if v[0] == '-' {
		i++
	}
	// JSON allows a single leading zero only, so 0 and 0.5 are values while 07 is not
	switch {
	case i == len(v):
		return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookRows, v)
	case v[i] == '0':
		i++
	case v[i] >= '1' && v[i] <= '9':
		for i++; i < len(v) && v[i] >= '0' && v[i] <= '9'; {
			i++
		}
	default:
		return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookRows, v)
	}
	if i < len(v) && v[i] == '.' {
		i++
		frac := i
		for i < len(v) && v[i] >= '0' && v[i] <= '9' {
			i++
		}
		if i == frac {
			return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookRows, v)
		}
	}
	if i < len(v) && (v[i] == 'e' || v[i] == 'E') {
		i++
		if i < len(v) && (v[i] == '+' || v[i] == '-') {
			i++
		}
		exp := i
		for i < len(v) && v[i] >= '0' && v[i] <= '9' {
			i++
		}
		if i == exp {
			return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookRows, v)
		}
	}
	if i != len(v) {
		return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookRows, v)
	}
	return nil
}
