package yobit

import (
	"errors"
	"log"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/currency"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

var e *Exchange

// Please supply your own keys for better unit testing
const (
	apiKey                  = ""
	apiSecret               = ""
	canManipulateRealOrders = false
)

var testPair = currency.NewBTCUSD().Format(currency.PairFormat{Delimiter: "_"})

func skipIfDDoSGuard(t *testing.T, err error) bool {
	t.Helper()
	if err == nil {
		return false
	}
	if errors.Is(err, request.ErrBadStatus) || strings.Contains(err.Error(), "403") {
		t.Skip("Skipping test due to HTTP 403 response from Yobit (DDoS-Guard)")
		return true
	}
	return false
}

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Yobit Setup error: %s", err)
	}

	if apiKey != "" && apiSecret != "" {
		e.API.AuthenticatedSupport = true
		e.SetCredentials(apiKey, apiSecret, "", "", "", "")
	}

	os.Exit(m.Run())
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTradablePairs(t.Context(), asset.Spot)
	if skipIfDDoSGuard(t, err) {
		return
	}
	require.NoError(t, err, "FetchTradablePairs must not error")
}

func TestGetInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetInfo(t.Context())
	if skipIfDDoSGuard(t, err) {
		return
	}
	require.NoError(t, err, "GetInfo must not error")
}

func TestGetTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetTicker(t.Context(), testPair.String())
	if skipIfDDoSGuard(t, err) {
		return
	}
	assert.NoError(t, err, "GetTicker should not error")
}

func TestGetDepth(t *testing.T) {
	t.Parallel()
	_, err := e.GetDepth(t.Context(), testPair.String())
	if skipIfDDoSGuard(t, err) {
		return
	}
	assert.NoError(t, err, "GetDepth should not error")
}

func TestGetTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetTrades(t.Context(), testPair.String())
	if skipIfDDoSGuard(t, err) {
		return
	}
	assert.NoError(t, err, "GetTrades should not error")
}

func TestGetAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.UpdateAccountBalances(t.Context(), asset.Spot)
	require.NoError(t, err)
}

func TestGetOpenOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetOpenOrders(t.Context(), "")
	assert.Error(t, err, "GetOpenOrders should error when credentials unset")
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetOrderInfo(t.Context(), "1337", currency.NewBTCUSD(), asset.Spot)
	if skipIfDDoSGuard(t, err) {
		return
	}
	require.NoError(t, err, "GetOrderInfo must not return error when credentials set")
}

func TestGetCryptoDepositAddress(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetCryptoDepositAddress(t.Context(), "bTc", false)
	if skipIfDDoSGuard(t, err) {
		return
	}
	require.NoError(t, err, "GetCryptoDepositAddress must not return error when credentials set")
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	err := e.CancelExistingOrder(t.Context(), 1337)
	assert.Error(t, err, "CancelExistingOrder should error when order id invalid")
}

func TestTrade(t *testing.T) {
	t.Parallel()
	_, err := e.Trade(t.Context(), "", order.Buy.String(), 0, 0)
	assert.Error(t, err, "Trade should error when parameters invalid")
}

func TestWithdrawCoinsToAddress(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawCoinsToAddress(t.Context(), "", 0, "")
	assert.Error(t, err, "WithdrawCoinsToAddress should error when parameters invalid")
}

func TestCreateYobicode(t *testing.T) {
	t.Parallel()
	_, err := e.CreateCoupon(t.Context(), "bla", 0)
	assert.Error(t, err, "CreateCoupon should error when parameters invalid")
}

func TestRedeemYobicode(t *testing.T) {
	t.Parallel()
	_, err := e.RedeemCoupon(t.Context(), "bla2")
	assert.Error(t, err, "RedeemCoupon should error when parameters invalid")
}

func setFeeBuilder() *exchange.FeeBuilder {
	return &exchange.FeeBuilder{
		Amount:  1,
		FeeType: exchange.CryptocurrencyTradeFee,
		Pair: currency.NewPairWithDelimiter(currency.LTC.String(),
			currency.BTC.String(),
			"-"),
		PurchasePrice:       1,
		FiatCurrency:        currency.USD,
		BankTransactionType: exchange.WireTransfer,
	}
}

func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	feeBuilder := setFeeBuilder()
	_, err := e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFeeByType must not error")
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "GetFeeByType should switch to trade fee when credentials set")
		return
	}
	assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "GetFeeByType should remain offline trade fee when credentials unset")
}

func TestGetFee(t *testing.T) {
	feeBuilder := setFeeBuilder()

	// CryptocurrencyTradeFee Basic
	_, err := e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for basic trade fee")

	// CryptocurrencyTradeFee High quantity
	feeBuilder = setFeeBuilder()
	feeBuilder.Amount = 1000
	feeBuilder.PurchasePrice = 1000
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for high quantity trade fee")

	// CryptocurrencyTradeFee IsMaker
	feeBuilder = setFeeBuilder()
	feeBuilder.IsMaker = true
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for maker trade fee")

	// CryptocurrencyTradeFee Negative purchase price
	feeBuilder = setFeeBuilder()
	feeBuilder.PurchasePrice = -1000
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for negative purchase price")
	// CryptocurrencyWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for withdrawal fee")
	// CryptocurrencyWithdrawalFee Invalid currency
	feeBuilder = setFeeBuilder()
	feeBuilder.Pair.Base = currency.NewCode("hello")
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for withdrawal fee with invalid currency")
	// CryptocurrencyDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyDepositFee
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for deposit fee")
	// InternationalBankDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for bank deposit fee")
	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.USD
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for basic bank withdrawal fee")
	// InternationalBankWithdrawalFee QIWI
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.USD
	feeBuilder.BankTransactionType = exchange.Qiwi
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for Qiwi withdrawal")
	// InternationalBankWithdrawalFee Wire
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.USD
	feeBuilder.BankTransactionType = exchange.WireTransfer
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for wire withdrawal")
	// InternationalBankWithdrawalFee Payeer
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.USD
	feeBuilder.BankTransactionType = exchange.Payeer
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for Payeer withdrawal")
	// InternationalBankWithdrawalFee Capitalist
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.RUR
	feeBuilder.BankTransactionType = exchange.Capitalist
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for Capitalist withdrawal")
	// InternationalBankWithdrawalFee AdvCash
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.USD
	feeBuilder.BankTransactionType = exchange.AdvCash
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for AdvCash withdrawal")
	// InternationalBankWithdrawalFee PerfectMoney
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.RUR
	feeBuilder.BankTransactionType = exchange.PerfectMoney
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for PerfectMoney withdrawal")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	expectedResult := exchange.AutoWithdrawCryptoWithAPIPermissionText + " & " + exchange.WithdrawFiatViaWebsiteOnlyText
	withdrawPermissions := e.FormatWithdrawPermissions()
	assert.Equal(t, expectedResult, withdrawPermissions, "FormatWithdrawPermissions should match expected result")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		Pairs:     []currency.Pair{currency.NewPair(currency.LTC, currency.BTC)},
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	_, err := e.GetActiveOrders(t.Context(), &getOrdersRequest)
	if credsSet {
		require.NoError(t, err, "GetActiveOrders must not return error when credentials set")
		return
	}
	assert.Error(t, err, "GetActiveOrders should error when credentials unset")
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Pairs:     []currency.Pair{currency.NewPair(currency.LTC, currency.BTC)},
		StartTime: time.Unix(0, 0),
		EndTime:   time.Unix(math.MaxInt64, 0),
		Side:      order.AnySide,
	}

	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	_, err := e.GetOrderHistory(t.Context(), &getOrdersRequest)
	if credsSet {
		require.NoError(t, err, "GetOrderHistory must not return error when credentials set")
		return
	}
	assert.Error(t, err, "GetOrderHistory should error when credentials unset")
}

// Any tests below this line have the ability to impact your orders on the exchange. Enable canManipulateRealOrders to run them
// ----------------------------------------------------------------------------------------------------------------------------
func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	orderSubmission := &order.Submit{
		Exchange: e.Name,
		Pair: currency.Pair{
			Delimiter: "_",
			Base:      currency.BTC,
			Quote:     currency.USD,
		},
		Side:      order.Buy,
		Type:      order.Limit,
		Price:     1,
		Amount:    1,
		ClientID:  "meowOrder",
		AssetType: asset.Spot,
	}
	response, err := e.SubmitOrder(t.Context(), orderSubmission)
	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	if credsSet {
		require.NoError(t, err, "SubmitOrder must not return error when credentials set")
		require.Equal(t, order.New, response.Status, "SubmitOrder must return new status when order created")
		return
	}
	assert.Error(t, err, "SubmitOrder should error when credentials unset")
}

func TestCancelExchangeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	currencyPair := currency.NewPair(currency.LTC, currency.BTC)
	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currencyPair,
		AssetType: asset.Spot,
	}

	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	err := e.CancelOrder(t.Context(), orderCancellation)
	if credsSet {
		require.NoError(t, err, "CancelOrder must not return error when credentials set")
		return
	}
	assert.Error(t, err, "CancelOrder should error when credentials unset")
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	currencyPair := currency.NewPair(currency.LTC, currency.BTC)
	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currencyPair,
		AssetType: asset.Spot,
	}

	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	resp, err := e.CancelAllOrders(t.Context(), orderCancellation)
	if credsSet {
		require.NoError(t, err, "CancelAllOrders must not return error when credentials set")
		assert.Empty(t, resp.Status, "CancelAllOrders should clear all cancellations")
		return
	}
	assert.Error(t, err, "CancelAllOrders should error when credentials unset")
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	_, err := e.ModifyOrder(t.Context(), &order.Modify{AssetType: asset.Spot})
	assert.Error(t, err, "ModifyOrder should error when parameters invalid")
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawCryptoRequest := withdraw.Request{
		Exchange:    e.Name,
		Amount:      -1,
		Currency:    currency.BTC,
		Description: "WITHDRAW IT ALL",
		Crypto: withdraw.CryptoRequest{
			Address: core.BitcoinDonationAddress,
		},
	}

	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	_, err := e.WithdrawCryptocurrencyFunds(t.Context(), &withdrawCryptoRequest)
	if credsSet {
		require.NoError(t, err, "WithdrawCryptocurrencyFunds must not return error when credentials set")
	} else {
		assert.Error(t, err, "WithdrawCryptocurrencyFunds should error when credentials unset")
	}
}

func TestWithdrawFiat(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawFiatRequest := withdraw.Request{}
	_, err := e.WithdrawFiatFunds(t.Context(), &withdrawFiatRequest)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "WithdrawFiatFunds should return ErrFunctionNotSupported")
}

func TestWithdrawInternationalBank(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	withdrawFiatRequest := withdraw.Request{}
	_, err := e.WithdrawFiatFundsToInternationalBank(t.Context(), &withdrawFiatRequest)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "WithdrawFiatFundsToInternationalBank should return ErrFunctionNotSupported")
}

func TestGetDepositAddress(t *testing.T) {
	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	_, err := e.GetDepositAddress(t.Context(), currency.BTC, "", "")
	if credsSet {
		require.NoError(t, err, "GetDepositAddress must not return error when credentials set")
		return
	}
	assert.Error(t, err, "GetDepositAddress should error when credentials unset")
}

func TestGetRecentTrades(t *testing.T) {
	_, err := e.GetRecentTrades(t.Context(), testPair, asset.Spot)
	if skipIfDDoSGuard(t, err) {
		return
	}
	assert.NoError(t, err, "GetRecentTrades should not error")
}

func TestGetHistoricTrades(t *testing.T) {
	_, err := e.GetHistoricTrades(t.Context(), testPair, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported)
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateTicker(t.Context(), testPair, asset.Spot)
	if skipIfDDoSGuard(t, err) {
		return
	}
	assert.NoError(t, err, "UpdateTicker should not error")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	err := e.UpdateTickers(t.Context(), asset.Spot)
	if skipIfDDoSGuard(t, err) {
		return
	}
	require.NoError(t, err, "UpdateTickers must not error")
}

func TestWrapperGetServerTime(t *testing.T) {
	t.Parallel()
	st, err := e.GetServerTime(t.Context(), asset.Spot)
	if skipIfDDoSGuard(t, err) {
		return
	}
	require.NoError(t, err, "GetServerTime must not error")
	require.False(t, st.IsZero(), "GetServerTime must return a non-zero time")
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	err := e.UpdateTradablePairs(t.Context())
	if skipIfDDoSGuard(t, err) {
		return
	}
	require.NoError(t, err, "UpdateTradablePairs must not error")
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		require.NoErrorf(t, err, "cannot get pairs for %s", a)
		require.NotEmptyf(t, pairs, "no pairs for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		if skipIfDDoSGuard(t, err) {
			continue
		}
		require.NoError(t, err)
		assert.NotEmpty(t, resp)
	}
}
