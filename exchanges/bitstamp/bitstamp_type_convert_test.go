package bitstamp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

func TestOrderSideUnmarshalJSON(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		data string
		exp  order.Side
		err  error
	}{
		// Bare numbers arrive on the websocket order feed, quoted ones on the REST ticker
		{data: `0`, exp: order.Buy},
		{data: `1`, exp: order.Sell},
		{data: `"0"`, exp: order.Buy},
		{data: `"1"`, exp: order.Sell},

		// Bad data bings
		{data: `null`, err: order.ErrSideIsInvalid},
		{data: `1.2`, err: order.ErrSideIsInvalid},
		{data: `-0`, err: order.ErrSideIsInvalid},
		{data: `-1`, err: order.ErrSideIsInvalid},
		{data: `""`, err: order.ErrSideIsInvalid},
		{data: `"buy"`, err: order.ErrSideIsInvalid},
		{data: `true`, err: order.ErrSideIsInvalid},
		{data: `1e0`, err: order.ErrSideIsInvalid},
		{data: `"-0"`, err: order.ErrSideIsInvalid},
		{data: `"2"`, err: order.ErrSideIsInvalid},
		{data: `"1e0"`, err: order.ErrSideIsInvalid},
	} {
		t.Run(tc.data, func(t *testing.T) {
			t.Parallel()
			var s orderSide
			err := json.Unmarshal([]byte(tc.data), &s)
			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err, "Unmarshal should reject a non-binary side")
				return
			}
			require.NoError(t, err, "Unmarshal must not error")
			assert.Equal(t, tc.exp, s.Side(), "Side should decode")
		})
	}
}

func TestOrderbookLevelUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var levels []OrderbookLevel
	require.NoError(t, json.Unmarshal([]byte(`[["84010","0.01190"],["84006","0.33643","1458532827766784"]]`), &levels), "Unmarshal must not error")
	exp := []OrderbookLevel{
		{Price: 84010, Amount: 0.0119},
		{Price: 84006, Amount: 0.33643, OrderID: 1458532827766784},
	}
	assert.Equal(t, exp, levels, "OrderbookLevel should unmarshal grouped and ungrouped levels")

	for _, data := range []string{
		`["84010"]`,
		`["84010","0.01190","1","2"]`,
		`["wibble","0.01190"]`,
		`["84010","wibble"]`,
		`["84010","0.01190","wibble"]`,
	} {
		var l OrderbookLevel
		assert.ErrorIsf(t, json.Unmarshal([]byte(data), &l), errMalformedOrderbookLevel, "Unmarshal should error on %s", data)
	}
	var l OrderbookLevel
	assert.Error(t, json.Unmarshal([]byte(`{"price":"84010"}`), &l), "Unmarshal should error on an object")
}

func TestOrderTransactionUnmarshalJSON(t *testing.T) {
	t.Parallel()
	const data = `{"tid":209895701,"price":"20000.00","fee":8,"datetime":"2022-01-31 14:45:15.322000","type":2,"btc":"0.10000000","usd":-2000,"trade_uti":"BSTP-20220131-209895701","note":"wibble"}`
	var tx OrderTransaction
	require.NoError(t, json.Unmarshal([]byte(data), &tx), "Unmarshal must not error")
	exp := OrderTransaction{
		TradeID:  209895701,
		Price:    20000,
		Fee:      8,
		DateTime: time.Date(2022, 1, 31, 14, 45, 15, 322000000, time.UTC),
		Type:     TransactionTypeMarketTrade,
		Amounts:  map[currency.Code]float64{currency.BTC: 0.1, currency.USD: -2000},
	}
	assert.Equal(t, exp, tx, "OrderTransaction should unmarshal the currency amounts and ignore other fields")

	for _, data := range []string{
		`{"tid":"wibble"}`,
		`{"price":"wibble"}`,
		`{"fee":null}`,
		`{"datetime":"wibble"}`,
		`{"type":"wibble"}`,
	} {
		assert.ErrorIsf(t, json.Unmarshal([]byte(data), &tx), errMalformedTransaction, "Unmarshal should error on %s", data)
	}
	assert.Error(t, json.Unmarshal([]byte(`[]`), &tx), "Unmarshal should error on an array")
}

func TestUserTransactionResponseUnmarshalJSON(t *testing.T) {
	t.Parallel()
	const data = `{"id":1,"datetime":"2022-03-01 10:54:53.849000","type":"2","fee":"0.40","order_id":1458532827766784,"self_trade":true,"self_trade_order_id":1458532827766785,"btc":"0.1","usd":-2000,"eur":0.0,"usdc":null,"btc_usd":20000,"eur_usd":"1.08","trade-uti":"1","note":"wibble","fee_currency":"USD"}`
	var tx UserTransactionResponse
	require.NoError(t, json.Unmarshal([]byte(data), &tx), "Unmarshal must not error")
	exp := UserTransactionResponse{
		ID:               1,
		DateTime:         time.Date(2022, 3, 1, 10, 54, 53, 849000000, time.UTC),
		Type:             TransactionTypeMarketTrade,
		Fee:              0.4,
		OrderID:          1458532827766784,
		SelfTrade:        true,
		SelfTradeOrderID: 1458532827766785,
		Amounts:          map[currency.Code]float64{currency.BTC: 0.1, currency.USD: -2000, currency.EUR: 0},
		ExchangeRates: map[currency.Pair]float64{
			currency.NewBTCUSD():                         20000,
			currency.NewPair(currency.EUR, currency.USD): 1.08,
		},
	}
	assert.Equal(t, exp, tx, "UserTransactionResponse should unmarshal the currency amounts and exchange rates, and ignore other fields")

	for _, data := range []string{
		`{"id":"wibble"}`,
		`{"datetime":"wibble"}`,
		`{"type":256}`,
		`{"fee":"wibble"}`,
		`{"order_id":"wibble"}`,
		`{"self_trade":"wibble"}`,
		`{"self_trade_order_id":"wibble"}`,
	} {
		assert.ErrorIsf(t, json.Unmarshal([]byte(data), &tx), errMalformedTransaction, "Unmarshal should error on %s", data)
	}
	assert.Error(t, json.Unmarshal([]byte(`[]`), &tx), "Unmarshal should error on an array")
}

func TestUnconfirmedDepositsUnmarshalJSON(t *testing.T) {
	t.Parallel()
	deposit := UnconfirmedDepositResponse{Amount: 0.1, Address: "3FiKkjgZ6Sj4RWp3ZsCjYh5Pt7ZCBsL7uF", Confirmations: 2, TransferID: 1}
	for _, data := range []string{
		` {"amount":"0.1","address":"3FiKkjgZ6Sj4RWp3ZsCjYh5Pt7ZCBsL7uF","confirmations":2,"transfer_id":1}`,
		`[{"amount":"0.1","address":"3FiKkjgZ6Sj4RWp3ZsCjYh5Pt7ZCBsL7uF","confirmations":2,"transfer_id":1}]`,
	} {
		var deposits unconfirmedDeposits
		require.NoErrorf(t, json.Unmarshal([]byte(data), &deposits), "Unmarshal must not error for %s", data)
		assert.Equalf(t, unconfirmedDeposits{deposit}, deposits, "Unmarshal should decode %s", data)
	}
	var deposits unconfirmedDeposits
	assert.Error(t, json.Unmarshal([]byte(`{"amount":[]}`), &deposits), "Unmarshal should error on a malformed deposit")
	assert.Error(t, json.Unmarshal([]byte(`[{"amount":[]}]`), &deposits), "Unmarshal should error on malformed deposits")
}

func TestParseNumber(t *testing.T) {
	t.Parallel()
	for data, exp := range map[string]float64{`"1.5"`: 1.5, `2`: 2, `""`: 0} {
		n, err := parseNumber(json.RawMessage(data))
		require.NoErrorf(t, err, "parseNumber must not error for %s", data)
		assert.Equalf(t, exp, n, "parseNumber should decode %s", data)
	}
	_, err := parseNumber(json.RawMessage(`null`))
	assert.ErrorIs(t, err, errNullNumber, "parseNumber should error on null")
	_, err = parseNumber(json.RawMessage(`"wibble"`))
	assert.Error(t, err, "parseNumber should error on a non-numeric string")
}

func TestIsAlphanumeric(t *testing.T) {
	t.Parallel()
	for s, exp := range map[string]bool{"btc": true, "USD": true, "1INCH": true, "": false, "trade_uti": false, "usd-perp": false, "ü": false} {
		assert.Equalf(t, exp, isAlphanumeric(s), "isAlphanumeric should be correct for %q", s)
	}
}

func TestUnmarshalTransactionType(t *testing.T) {
	t.Parallel()
	for _, data := range []string{`2`, `"2"`} {
		var typ TransactionType
		require.NoErrorf(t, unmarshalTransactionType(json.RawMessage(data), &typ), "unmarshalTransactionType must not error for %s", data)
		assert.Equalf(t, TransactionTypeMarketTrade, typ, "unmarshalTransactionType should decode %s", data)
	}
	var typ TransactionType
	assert.Error(t, unmarshalTransactionType(json.RawMessage(`256`), &typ), "unmarshalTransactionType should error on an out of range type")
	assert.Error(t, unmarshalTransactionType(json.RawMessage(`"wibble"`), &typ), "unmarshalTransactionType should error on a non-numeric type")
}
