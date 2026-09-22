package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
)

// The deposit page explicitly requires JSON despite its generated form example.
// https://developers.binance.com/en/docs/catalog/investment-and-services-fiat/api/rest-api/~
func TestDocumentedFiatRequests(t *testing.T) {
	const response = `{"code":"000000","message":"success","data":{"orderId":"04595xxxxxxxxx37"}}`
	for _, deposit := range []bool{false, true} {
		local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method, "fiat instruction should use POST")
			params := r.URL.Query()
			sig := params.Get("signature")
			params.Del("signature")
			mac := hmac.New(sha256.New, []byte("test-secret"))
			_, _ = mac.Write([]byte(params.Encode()))
			assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), sig, "signature should match the transmitted query")
			assert.Equal(t, "6000", params.Get("recvWindow"), "receive window should remain in the signed query")
			assert.NotEmpty(t, params.Get("timestamp"), "request should include its timestamp")
			params.Del("recvWindow")
			params.Del("timestamp")
			if deposit {
				assert.Equal(t, "/sapi/v1/fiat/deposit", r.URL.Path, "deposit should use its documented path")
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"), "deposit should use the explicitly documented JSON transport")
				assert.Empty(t, params, "business fields should occur only in the JSON body")
				var fields map[string]json.RawMessage
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&fields), "deposit body should decode") {
					return
				}
				want := map[string]json.RawMessage{"currency": json.RawMessage(`"BRL"`), "apiPaymentMethod": json.RawMessage(`"pix"`), "amount": json.RawMessage(`"1.25"`), "ext": json.RawMessage(`{"reference":"test"}`)}
				assert.Equal(t, want, fields, "deposit should retain all fields and its quoted decimal amount")
			} else {
				assert.Equal(t, "/sapi/v2/fiat/withdraw", r.URL.Path, "withdrawal should use v2")
				assert.Equal(t, "BRL", params.Get("currency"), "currency should use its code")
				assert.Equal(t, "bank_transfer", params.Get("apiPaymentMethod"), "method should be explicit")
				assert.Equal(t, "10", params.Get("amount"), "withdrawal should use the documented integer amount")
				assert.JSONEq(t, `{"accountNumber":"001234","agency":"123","bankCodeForPix":"222","accountType":"current"}`, params.Get("accountInfo"), "all bank fields should retain their documented spelling")
				assert.JSONEq(t, `{"reference":"test"}`, params.Get("ext"), "extension object should be retained")
				assert.Len(t, params, 5, "withdrawal should send only business fields after auth is removed")
			}
			_, err := w.Write([]byte(response))
			assert.NoError(t, err, "documented response should write")
		})
		var result *FiatOrderResponse
		var err error
		if deposit {
			result, err = local.DepositFiat(t.Context(), &FiatDepositRequest{Currency: currency.BRL, APIPaymentMethod: "pix", Amount: 1.25, RecvWindow: 6000, Ext: map[string]any{"reference": "test"}})
		} else {
			result, err = local.WithdrawFiat(t.Context(), &FiatWithdrawRequest{Currency: currency.BRL, APIPaymentMethod: "bank_transfer", Amount: 10, AccountInfo: &FiatBankAccount{AccountNumber: "001234", Agency: "123", BankCodeForPix: "222", AccountType: "current"}, RecvWindow: 6000, Ext: map[string]any{"reference": "test"}})
		}
		require.NoError(t, err, "documented success response must decode")
		assert.Equal(t, "04595xxxxxxxxx37", result.Data.OrderID, "order ID should remain a string")
		assertResponseFields(t, []byte(response), reflect.TypeOf(result), "fiat")
	}
}

func TestFiatValidation(t *testing.T) {
	local := new(Exchange)
	_, err := local.DepositFiat(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "nil deposit should fail offline")
	_, err = local.WithdrawFiat(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "nil withdrawal should fail offline")
	for _, tc := range []struct {
		currency currency.Code
		method   string
		amount   uint64
		account  *FiatBankAccount
		want     error
	}{
		{want: currency.ErrCurrencyCodeEmpty},
		{currency: currency.BRL, want: errPaymentMethodRequired},
		{currency: currency.BRL, method: "pix", want: limits.ErrAmountBelowMin},
	} {
		_, err = local.DepositFiat(t.Context(), &FiatDepositRequest{Currency: tc.currency, APIPaymentMethod: tc.method})
		assert.ErrorIs(t, err, tc.want, "deposit validation should retain its sentinel")
		_, err = local.WithdrawFiat(t.Context(), &FiatWithdrawRequest{Currency: tc.currency, APIPaymentMethod: tc.method})
		assert.ErrorIs(t, err, tc.want, "withdrawal validation should retain its sentinel")
	}
	for _, account := range []*FiatBankAccount{nil, {}} {
		_, err = local.WithdrawFiat(t.Context(), &FiatWithdrawRequest{Currency: currency.BRL, APIPaymentMethod: "bank_transfer", Amount: 10, AccountInfo: account})
		assert.ErrorIs(t, err, errAccountRequired, "destination account should be required")
	}
}
