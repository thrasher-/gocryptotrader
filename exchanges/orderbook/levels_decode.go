package orderbook

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// maxPresizedLevels bounds the capacity guessed from the payload length. Beyond it append grows the
// slice from what is actually there, which no venue reaches.
const maxPresizedLevels = 4096

var errMalformedLevels = errors.New("malformed price amount array")

// What a third cell holds, where a venue sends one. Anything further is surplus either way.
type thirdCell uint8

const (
	thirdCellSurplus thirdCell = iota
	thirdCellOrderCount
	thirdCellID
)

// UnmarshalJSON implements json.Unmarshaler. The payload is a fixed shape, [[price,amount],...],
// with each number bare or quoted. Routing it through encoding/json costs reflection per element and
// an intermediate [][2]types.Number, so it is scanned straight into the levels instead.
func (l *LevelsArrayPriceAmount) UnmarshalJSON(data []byte) error {
	levels, err := scanLevels(data, thirdCellSurplus)
	if err != nil {
		return err
	}
	*l = LevelsArrayPriceAmount(levels)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for [[price,amount,orderCount],...], which is the same
// payload with the third cell kept rather than stepped over.
func (l *LevelsArrayPriceAmountOrderCount) UnmarshalJSON(data []byte) error {
	levels, err := scanLevels(data, thirdCellOrderCount)
	if err != nil {
		return err
	}
	*l = LevelsArrayPriceAmountOrderCount(levels)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for [[price,amount,id],...], which is the same payload
// with the third cell read as the level's exchange ID.
func (l *LevelsArrayPriceAmountID) UnmarshalJSON(data []byte) error {
	levels, err := scanLevels(data, thirdCellID)
	if err != nil {
		return err
	}
	*l = LevelsArrayPriceAmountID(levels)
	return nil
}

// scanLevels reads the array of levels, third naming what a third cell holds.
func scanLevels(data []byte, third thirdCell) (Levels, error) {
	i := skipSpace(data, 0)
	if hasLiteral(data, i, "null") {
		if skipSpace(data, i+4) != len(data) {
			return nil, fmt.Errorf("%w: trailing data", errMalformedLevels)
		}
		return Levels{}, nil
	}
	if i == len(data) || data[i] != '[' {
		return nil, fmt.Errorf("%w: expected '['", errMalformedLevels)
	}

	// A level is seldom under twenty bytes, so this lands within one doubling of the final length.
	// Capped because the estimate comes from the byte length, which a run of whitespace inflates
	// without producing any levels.
	levels := make(Levels, 0, min(len(data)/20+1, maxPresizedLevels))

	i = skipSpace(data, i+1)
	for i < len(data) && data[i] != ']' {
		if data[i] != '[' {
			return nil, fmt.Errorf("%w: expected '['", errMalformedLevels)
		}
		var level Level
		var err error
		if level, i, err = parseLevel(data, i+1, third); err != nil {
			return nil, err
		}
		levels = append(levels, level)

		if i = skipSpace(data, i); i == len(data) {
			break
		}
		switch data[i] {
		case ']':
		case ',':
			if i = skipSpace(data, i+1); i < len(data) && data[i] == ']' {
				return nil, fmt.Errorf("%w: trailing comma", errMalformedLevels)
			}
		default:
			return nil, fmt.Errorf("%w: expected ',' or ']'", errMalformedLevels)
		}
	}
	if i == len(data) {
		return nil, fmt.Errorf("%w: unterminated array", errMalformedLevels)
	}
	if skipSpace(data, i+1) != len(data) {
		return nil, fmt.Errorf("%w: trailing data", errMalformedLevels)
	}
	return levels, nil
}

// parseLevel reads one [price,amount] body, starting after its opening bracket. A short inner array
// leaves the missing field zero and a long one discards the surplus, as a [2]Number decode did.
func parseLevel(data []byte, i int, third thirdCell) (level Level, next int, err error) {
	for field := 0; ; field++ {
		if i = skipSpace(data, i); i == len(data) {
			return level, i, fmt.Errorf("%w: unterminated level", errMalformedLevels)
		}
		if data[i] == ']' {
			return level, i + 1, nil
		}
		if field > 0 {
			if data[i] != ',' {
				return level, i, fmt.Errorf("%w: expected ',' or ']'", errMalformedLevels)
			}
			if i = skipSpace(data, i+1); i == len(data) {
				return level, i, fmt.Errorf("%w: unterminated level", errMalformedLevels)
			}
		}
		if field > 1 {
			if field == 2 && third != thirdCellSurplus {
				value, next, err := parseNumber(data, i)
				if err != nil {
					return level, next, err
				}
				if third == thirdCellOrderCount {
					level.OrderCount = int64(value)
				} else {
					level.ID = int64(value)
				}
				i = next
				continue
			}
			// Anything past the amount is surplus. Decoding into a two element array of numbers
			// discarded a value of any type here, so it is stepped over rather than parsed.
			next, err := skipValue(data, i)
			if err != nil {
				return level, next, err
			}
			i = next
			continue
		}
		value, next, err := parseNumber(data, i)
		if err != nil {
			return level, next, err
		}
		if field == 0 {
			level.Price = value
		} else {
			level.Amount = value
		}
		i = next
	}
}

// parseNumber reads one number and returns the index after it. Bare values are held to the JSON
// grammar and quoted ones handed to strconv, which is the split types.Number produced.
func parseNumber(data []byte, i int) (value float64, next int, err error) {
	if hasLiteral(data, i, "null") {
		return 0, i + 4, nil
	}

	if data[i] == '"' {
		start := i + 1
		end := bytes.IndexByte(data[start:], '"')
		if end < 0 {
			return 0, i, fmt.Errorf("%w: unterminated string", errMalformedLevels)
		}
		i = start + end
		if end == 0 {
			return 0, i + 1, nil // types.Number reads an empty string as zero
		}
		f, err := strconv.ParseFloat(string(data[start:i]), 64)
		if err != nil {
			return 0, i, fmt.Errorf("%w: %s", errMalformedLevels, data[start:i])
		}
		return f, i + 1, nil
	}

	end, err := scanJSONNumber(data, i)
	if err != nil {
		return 0, end, err
	}
	f, ferr := strconv.ParseFloat(string(data[i:end]), 64)
	if ferr != nil {
		return 0, end, fmt.Errorf("%w: %s", errMalformedLevels, data[i:end])
	}
	return f, end, nil
}

// scanJSONNumber returns the index after a bare number, rejecting forms JSON disallows but strconv
// accepts, such as a leading plus, a bare fraction or a leading zero.
func scanJSONNumber(data []byte, i int) (int, error) {
	start := i
	if i < len(data) && data[i] == '-' {
		i++
	}
	switch {
	case i == len(data):
		return i, fmt.Errorf("%w: expected number", errMalformedLevels)
	case data[i] == '0':
		i++
	case data[i] >= '1' && data[i] <= '9':
		for i++; i < len(data) && isDigit(data[i]); {
			i++
		}
	default:
		return i, fmt.Errorf("%w: expected number at %q", errMalformedLevels, data[start:min(start+8, len(data))])
	}
	if i < len(data) && data[i] == '.' {
		i++
		if i == len(data) || !isDigit(data[i]) {
			return i, fmt.Errorf("%w: expected digit after '.'", errMalformedLevels)
		}
		for i < len(data) && isDigit(data[i]) {
			i++
		}
	}
	if i < len(data) && (data[i] == 'e' || data[i] == 'E') {
		i++
		if i < len(data) && (data[i] == '+' || data[i] == '-') {
			i++
		}
		if i == len(data) || !isDigit(data[i]) {
			return i, fmt.Errorf("%w: expected digit in exponent", errMalformedLevels)
		}
		for i < len(data) && isDigit(data[i]) {
			i++
		}
	}
	return i, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func hasLiteral(data []byte, i int, lit string) bool {
	return i+len(lit) <= len(data) && string(data[i:i+len(lit)]) == lit
}

func skipSpace(data []byte, i int) int {
	for ; i < len(data); i++ {
		switch data[i] {
		case ' ', '\t', '\r', '\n':
		default:
			return i
		}
	}
	return i
}

// skipValue steps over one JSON value of any type, returning the index after it.
func skipValue(data []byte, i int) (int, error) {
	i = skipSpace(data, i)
	if i == len(data) {
		return i, fmt.Errorf("%w: expected a value", errMalformedLevels)
	}
	switch data[i] {
	case '"':
		for j := i + 1; j < len(data); j++ {
			switch data[j] {
			case '\\':
				j++
			case '"':
				return j + 1, nil
			}
		}
		return len(data), fmt.Errorf("%w: unterminated string", errMalformedLevels)
	case '[', '{':
		depth := 0
		for j := i; j < len(data); j++ {
			switch data[j] {
			case '"':
				for j++; j < len(data); j++ {
					if data[j] == '\\' {
						j++
						continue
					}
					if data[j] == '"' {
						break
					}
				}
			case '[', '{':
				depth++
			case ']', '}':
				if depth--; depth == 0 {
					return j + 1, nil
				}
			}
		}
		return len(data), fmt.Errorf("%w: unterminated value", errMalformedLevels)
	case 't':
		if hasLiteral(data, i, "true") {
			return i + 4, nil
		}
	case 'f':
		if hasLiteral(data, i, "false") {
			return i + 5, nil
		}
	case 'n':
		if hasLiteral(data, i, "null") {
			return i + 4, nil
		}
	default:
		return scanJSONNumber(data, i)
	}
	return i, fmt.Errorf("%w: expected a value", errMalformedLevels)
}
