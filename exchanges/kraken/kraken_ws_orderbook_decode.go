package kraken

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// maxPresizedLevels bounds the capacity guessed from the payload length, which whitespace inflates
// without producing any entries.
const maxPresizedLevels = 4096

var errMalformedOrderbookLevels = errors.New("malformed orderbook levels")

// wsOrderbookItems is an array of ["price","volume","time"] entries.
type wsOrderbookItems []wsOrderbookItem

// UnmarshalJSON implements json.Unmarshaler.
//
// Kraken's checksum needs the digits exactly as sent, and decoding each entry through a [3]any cost
// reflection plus a string allocation for both the price and the amount. One copy of the payload
// backs every raw field instead, since slicing a string does not allocate.
func (l *wsOrderbookItems) UnmarshalJSON(data []byte) error {
	raw := string(data)
	i := skipJSONSpace(raw, 0)
	if strings.HasPrefix(raw[i:], "null") {
		if skipJSONSpace(raw, i+4) != len(raw) {
			return fmt.Errorf("%w: trailing data", errMalformedOrderbookLevels)
		}
		*l = nil
		return nil
	}
	if i == len(raw) || raw[i] != '[' {
		return fmt.Errorf("%w: expected '['", errMalformedOrderbookLevels)
	}

	items := make(wsOrderbookItems, 0, min(len(raw)/48+1, maxPresizedLevels))
	i = skipJSONSpace(raw, i+1)
	for i < len(raw) && raw[i] != ']' {
		if raw[i] != '[' {
			return fmt.Errorf("%w: expected '['", errMalformedOrderbookLevels)
		}
		item, next, err := parseOrderbookItem(raw, data, i+1)
		if err != nil {
			return err
		}
		items = append(items, item)

		if i = skipJSONSpace(raw, next); i == len(raw) {
			break
		}
		switch raw[i] {
		case ']':
		case ',':
			if i = skipJSONSpace(raw, i+1); i < len(raw) && raw[i] == ']' {
				return fmt.Errorf("%w: trailing comma", errMalformedOrderbookLevels)
			}
		default:
			return fmt.Errorf("%w: expected ',' or ']'", errMalformedOrderbookLevels)
		}
	}
	if i == len(raw) {
		return fmt.Errorf("%w: unterminated array", errMalformedOrderbookLevels)
	}
	if skipJSONSpace(raw, i+1) != len(raw) {
		return fmt.Errorf("%w: trailing data", errMalformedOrderbookLevels)
	}
	*l = items
	return nil
}

// parseOrderbookItem reads one entry body, starting after its opening bracket. A short entry leaves
// the missing fields zero and a long one discards the surplus, as decoding into a [3]any did.
func parseOrderbookItem(raw string, data []byte, i int) (wsOrderbookItem, int, error) {
	var item wsOrderbookItem
	for field := 0; ; field++ {
		if i = skipJSONSpace(raw, i); i == len(raw) {
			return item, i, fmt.Errorf("%w: unterminated entry", errMalformedOrderbookLevels)
		}
		if raw[i] == ']' {
			if err := item.parseNumbers(); err != nil {
				return item, i, err
			}
			return item, i + 1, nil
		}
		if field > 0 {
			if raw[i] != ',' {
				return item, i, fmt.Errorf("%w: expected ',' or ']'", errMalformedOrderbookLevels)
			}
			if i = skipJSONSpace(raw, i+1); i == len(raw) {
				return item, i, fmt.Errorf("%w: unterminated entry", errMalformedOrderbookLevels)
			}
		}
		start, end, next, err := scanJSONValue(raw, i)
		if err != nil {
			return item, next, err
		}
		switch field {
		case 0, 1:
			// The price and volume arrive quoted; a bare number was rejected by decoding into a string
			if raw[start-1] != '"' {
				return item, next, fmt.Errorf("%w: expected a string", errMalformedOrderbookLevels)
			}
			if field == 0 {
				item.PriceRaw = raw[start:end]
			} else {
				item.AmountRaw = raw[start:end]
			}
		case 2:
			if err := item.Time.UnmarshalJSON(data[i:next]); err != nil {
				return item, next, err
			}
		}
		i = next
	}
}

// parseNumbers converts the retained digits, reporting the same errors the previous decode did.
func (ws *wsOrderbookItem) parseNumbers() error {
	var err error
	if ws.Price, err = strconv.ParseFloat(ws.PriceRaw, 64); err != nil {
		return fmt.Errorf("error parsing price: %w", err)
	}
	if ws.Amount, err = strconv.ParseFloat(ws.AmountRaw, 64); err != nil {
		return fmt.Errorf("error parsing amount: %w", err)
	}
	return nil
}

// scanJSONValue returns the bounds of the value at i, excluding the quotes of a string, along with
// the index after it.
func scanJSONValue(raw string, i int) (start, end, next int, err error) {
	if raw[i] == '"' {
		for next = i + 1; next < len(raw); next++ {
			if raw[next] == '\\' {
				next++
				continue
			}
			if raw[next] == '"' {
				return i + 1, next, next + 1, nil
			}
		}
		return i, i, next, fmt.Errorf("%w: unterminated string", errMalformedOrderbookLevels)
	}
	if raw[i] == '[' || raw[i] == '{' {
		// Entries hold scalars only, so a nested value means the entry never closed
		return i, i, i, fmt.Errorf("%w: unexpected %q", errMalformedOrderbookLevels, raw[i])
	}
	for next = i; next < len(raw); next++ {
		switch raw[next] {
		case ',', ']', '}', ' ', '\t', '\r', '\n':
			return i, next, next, validLiteral(raw[i:next])
		}
	}
	return i, next, next, validLiteral(raw[i:next])
}

func skipJSONSpace(raw string, i int) int {
	for ; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\t', '\r', '\n':
		default:
			return i
		}
	}
	return i
}

// validLiteral reports whether an unquoted value is one JSON allows. types.Time reads anything
// beginning with an n as null, so a corrupted token has to be screened before reaching it.
func validLiteral(s string) error {
	switch s {
	case "null", "true", "false":
		return nil
	}
	i := 0
	if strings.HasPrefix(s, "-") {
		i++
	}
	digits := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == digits {
		return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookLevels, s)
	}
	if i < len(s) && s[i] == '.' {
		i++
		frac := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == frac {
			return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookLevels, s)
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		exp := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == exp {
			return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookLevels, s)
		}
	}
	if i != len(s) {
		return fmt.Errorf("%w: %q is not a value", errMalformedOrderbookLevels, s)
	}
	return nil
}
