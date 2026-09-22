package binance

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var errPaymentMethodRequired = errors.New("payment method required")

// FiatDepositRequest contains the JSON business fields; RecvWindow is sent in the signed query.
type FiatDepositRequest struct {
	Currency         currency.Code  `json:"currency"`
	APIPaymentMethod string         `json:"apiPaymentMethod"`
	Amount           types.Number   `json:"amount"`
	Ext              map[string]any `json:"ext,omitempty"`
	RecvWindow       uint64         `json:"-"`
}

// FiatWithdrawRequest uses the integer amount documented by the v2 withdrawal API.
type FiatWithdrawRequest struct {
	Currency         currency.Code    `json:"currency"`
	APIPaymentMethod string           `json:"apiPaymentMethod"`
	Amount           uint64           `json:"amount"`
	AccountInfo      *FiatBankAccount `json:"accountInfo"`
	Ext              map[string]any   `json:"ext,omitempty"`
	RecvWindow       uint64           `json:"-"`
}

// FiatBankAccount identifies the destination bank account without discarding leading zeroes.
type FiatBankAccount struct {
	AccountNumber  string `json:"accountNumber"`
	Agency         string `json:"agency,omitempty"`
	BankCodeForPix string `json:"bankCodeForPix,omitempty"`
	AccountType    string `json:"accountType,omitempty"`
}

// FiatOrderResponse contains the order identifier used to poll GetFiatOrderDetail.
type FiatOrderResponse struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Data    *FiatOrderReference `json:"data"`
}

// FiatOrderReference retains the exchange's string identifier.
type FiatOrderReference struct {
	OrderID string `json:"orderId"`
}

// DepositFiat creates a PIX deposit order before a bank transfer is made.
func (e *Exchange) DepositFiat(ctx context.Context, arg *FiatDepositRequest) (*FiatOrderResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Currency.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if arg.APIPaymentMethod == "" {
		return nil, errPaymentMethodRequired
	}
	if arg.Amount <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	params := url.Values{}
	if arg.RecvWindow != 0 {
		params.Set("recvWindow", strconv.FormatUint(arg.RecvWindow, 10))
	}
	var response *FiatOrderResponse
	return response, e.SendAuthJSONHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/sapi/v1/fiat/deposit", params, sapiFiatOrdersRate, arg, &response)
}

// WithdrawFiat creates a bank-transfer withdrawal order.
func (e *Exchange) WithdrawFiat(ctx context.Context, arg *FiatWithdrawRequest) (*FiatOrderResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Currency.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if arg.APIPaymentMethod == "" {
		return nil, errPaymentMethodRequired
	}
	if arg.Amount == 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.AccountInfo == nil || arg.AccountInfo.AccountNumber == "" {
		return nil, errAccountRequired
	}
	params := url.Values{}
	if arg.RecvWindow != 0 {
		params.Set("recvWindow", strconv.FormatUint(arg.RecvWindow, 10))
	}
	var response *FiatOrderResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/sapi/v2/fiat/withdraw", params, sapiFiatOrdersRate, arg, &response)
}
