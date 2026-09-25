package bitstamp

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func (s *orderSide) UnmarshalJSON(data []byte) error {
	// The REST ticker quotes the side whereas the websocket order feed sends a bare number
	switch string(bytes.Trim(data, `"`)) {
	case "0":
		*s = orderSide(order.Buy)
	case "1":
		*s = orderSide(order.Sell)
	default:
		return fmt.Errorf("%w: %s", order.ErrSideIsInvalid, data)
	}

	return nil
}

// Side returns the order side
func (s orderSide) Side() order.Side {
	return order.Side(s)
}

// UnmarshalJSON decodes an orderbook level sent as [price, amount] or [price, amount, order_id]
func (l *OrderbookLevel) UnmarshalJSON(data []byte) error {
	var level []string
	if err := json.Unmarshal(data, &level); err != nil {
		return err
	}
	if len(level) != 2 && len(level) != 3 {
		return fmt.Errorf("%w: %s", errMalformedOrderbookLevel, data)
	}
	var err error
	if l.Price, err = strconv.ParseFloat(level[0], 64); err != nil {
		return fmt.Errorf("%w price %q: %w", errMalformedOrderbookLevel, level[0], err)
	}
	if l.Amount, err = strconv.ParseFloat(level[1], 64); err != nil {
		return fmt.Errorf("%w amount %q: %w", errMalformedOrderbookLevel, level[1], err)
	}
	if len(level) == 3 {
		if l.OrderID, err = strconv.ParseUint(level[2], 10, 64); err != nil {
			return fmt.Errorf("%w order id %q: %w", errMalformedOrderbookLevel, level[2], err)
		}
	}
	return nil
}

// UnmarshalJSON decodes an order transaction, collecting the dynamically named currency amounts
func (t *OrderTransaction) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*t = OrderTransaction{Amounts: make(map[currency.Code]float64)}
	for key, value := range fields {
		var err error
		switch key {
		case "tid":
			err = json.Unmarshal(value, &t.TradeID)
		case "price":
			t.Price, err = parseNumber(value)
		case "fee":
			t.Fee, err = parseNumber(value)
		case "datetime":
			var dt types.DateTime
			err = json.Unmarshal(value, &dt)
			t.DateTime = dt.Time()
		case "type":
			err = unmarshalTransactionType(value, &t.Type)
		default:
			if !isAlphanumeric(key) {
				continue // Undocumented fields cannot be currency amounts
			}
			amount, parseErr := parseNumber(value)
			if parseErr != nil {
				continue // Undocumented non-numeric fields cannot be currency amounts
			}
			t.Amounts[currency.NewCode(key).Upper()] = amount
		}
		if err != nil {
			return fmt.Errorf("%w %q: %w", errMalformedTransaction, key, err)
		}
	}
	return nil
}

// UnmarshalJSON decodes a user transaction, collecting the dynamically named currency amounts and exchange rates
func (t *UserTransactionResponse) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*t = UserTransactionResponse{
		Amounts:       make(map[currency.Code]float64),
		ExchangeRates: make(map[currency.Pair]float64),
	}
	for key, value := range fields {
		var err error
		switch key {
		case "id":
			err = json.Unmarshal(value, &t.ID)
		case "datetime":
			var dt types.DateTime
			err = json.Unmarshal(value, &dt)
			t.DateTime = dt.Time()
		case "type":
			err = unmarshalTransactionType(value, &t.Type)
		case "fee":
			t.Fee, err = parseNumber(value)
		case "order_id":
			err = json.Unmarshal(value, &t.OrderID)
		case "self_trade":
			err = json.Unmarshal(value, &t.SelfTrade)
		case "self_trade_order_id":
			err = json.Unmarshal(value, &t.SelfTradeOrderID)
		default:
			base, quote, isPair := strings.Cut(key, "_")
			if !isAlphanumeric(base) || (isPair && !isAlphanumeric(quote)) {
				continue // Undocumented fields cannot be currency amounts or exchange rates
			}
			amount, parseErr := parseNumber(value)
			if parseErr != nil {
				continue // Undocumented non-numeric fields cannot be currency amounts or exchange rates
			}
			if isPair {
				t.ExchangeRates[currency.NewPair(currency.NewCode(base).Upper(), currency.NewCode(quote).Upper())] = amount
				continue
			}
			t.Amounts[currency.NewCode(key).Upper()] = amount
		}
		if err != nil {
			return fmt.Errorf("%w %q: %w", errMalformedTransaction, key, err)
		}
	}
	return nil
}

// UnmarshalJSON implements json.Unmarshaler
func (u *unconfirmedDeposits) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '{' {
		var deposit UnconfirmedDepositResponse
		if err := json.Unmarshal(data, &deposit); err != nil {
			return err
		}
		*u = unconfirmedDeposits{deposit}
		return nil
	}
	var deposits []UnconfirmedDepositResponse
	if err := json.Unmarshal(data, &deposits); err != nil {
		return err
	}
	*u = deposits
	return nil
}

// parseNumber decodes a number which may be sent as either a JSON number or a numeric string
func parseNumber(data json.RawMessage) (float64, error) {
	if bytes.Equal(data, []byte("null")) {
		return 0, errNullNumber
	}
	var n types.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return 0, err
	}
	return n.Float64(), nil
}

// isAlphanumeric reports whether s is a non-empty string of ASCII letters and digits, as used by currency codes
func isAlphanumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// unmarshalTransactionType decodes a transaction type which may be sent as either a JSON number or a numeric string
func unmarshalTransactionType(data json.RawMessage, t *TransactionType) error {
	v, err := strconv.ParseUint(string(bytes.Trim(data, `"`)), 10, 8)
	if err != nil {
		return err
	}
	*t = TransactionType(v)
	return nil
}
