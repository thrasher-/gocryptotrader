package exmo

import (
	"log"
	"os"
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
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

const (
	APIKey                  = ""
	APISecret               = ""
	canManipulateRealOrders = false
)

var (
	e        *Exchange
	testPair = currency.NewBTCUSD().Format(currency.PairFormat{Uppercase: true, Delimiter: "_"})
)

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("EXMO Setup error: %s", err)
	}

	if APIKey != "" && APISecret != "" {
		e.API.AuthenticatedSupport = true
		e.SetCredentials(APIKey, APISecret, "", "", "", "")
	}

	os.Exit(m.Run())
}

func TestGetTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetTrades(t.Context(), testPair.String())
	assert.NoError(t, err, "GetTrades should not error")
}

func TestGetOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderbook(t.Context(), testPair.String())
	assert.NoError(t, err, "GetOrderbook should not error")
}

func TestGetTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetTicker(t.Context())
	require.NoError(t, err, "GetTicker must not error")
}

func TestGetPairSettings(t *testing.T) {
	t.Parallel()
	_, err := e.GetPairSettings(t.Context())
	require.NoError(t, err, "GetPairSettings must not error")
}

func TestGetCurrency(t *testing.T) {
	t.Parallel()
	_, err := e.GetCurrency(t.Context())
	require.NoError(t, err, "GetCurrency must not error")
}

func TestGetUserInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetUserInfo(t.Context())
	require.NoError(t, err, "GetUserInfo must not error")
}

func TestGetRequiredAmount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetRequiredAmount(t.Context(), testPair.String(), 100)
	assert.NoError(t, err, "GetRequiredAmount should not error")
}

func setFeeBuilder() *exchange.FeeBuilder {
	return &exchange.FeeBuilder{
		Amount:              1,
		FeeType:             exchange.CryptocurrencyTradeFee,
		Pair:                testPair,
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
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "GetFeeByType should switch to CryptocurrencyTradeFee when credentials set")
		return
	}
	assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "GetFeeByType should remain OfflineTradeFee without credentials")
}

func TestGetFee(t *testing.T) {
	t.Parallel()

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
	require.NoError(t, err, "GetFee must not return error for negative purchase price trade fee")

	// CryptocurrencyWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for cryptocurrency withdrawal")

	// CryptocurrencyWithdrawalFee Invalid currency
	feeBuilder = setFeeBuilder()
	feeBuilder.Pair.Base = currency.NewCode("hello")
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error when withdrawal currency invalid")

	// CryptocurrencyDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyDepositFee
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for deposit fee")

	// InternationalBankDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	feeBuilder.FiatCurrency = currency.RUB
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for RUB bank deposit")

	// InternationalBankDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	feeBuilder.FiatCurrency = currency.PLN
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for PLN bank deposit")

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.PLN
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for PLN bank withdrawal")

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.TRY
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for TRY bank withdrawal")

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.EUR
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for EUR bank withdrawal")

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.RUB
	_, err = e.GetFee(feeBuilder)
	require.NoError(t, err, "GetFee must not return error for RUB bank withdrawal")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	expectedResult := exchange.AutoWithdrawCryptoWithSetupText + " & " + exchange.NoFiatWithdrawalsText
	withdrawPermissions := e.FormatWithdrawPermissions()
	assert.Equal(t, expectedResult, withdrawPermissions, "FormatWithdrawPermissions should match expected result")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
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
		Side:      order.AnySide,
	}
	currPair := currency.NewBTCUSD()
	currPair.Delimiter = "_"
	getOrdersRequest.Pairs = []currency.Pair{currPair}

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
		Exchange:  e.Name,
		Pair:      testPair,
		Side:      order.Buy,
		Type:      order.Limit,
		Price:     1,
		Amount:    1,
		ClientID:  "meowOrder",
		AssetType: asset.Spot,
	}
	credsSet := sharedtestvalues.AreAPICredentialsSet(e)
	response, err := e.SubmitOrder(t.Context(), orderSubmission)
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

	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      testPair,
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

	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      testPair,
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
	if sharedtestvalues.AreAPICredentialsSet(e) {
		_, err := e.GetDepositAddress(t.Context(), currency.USDT, "", "ERC20")
		require.NoError(t, err, "GetDepositAddress must not return error when credentials set")
		return
	}
	_, err := e.GetDepositAddress(t.Context(), currency.LTC, "", "")
	assert.Error(t, err, "GetDepositAddress should error when credentials unset")
}

func TestGetCryptoDepositAddress(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetCryptoDepositAddress(t.Context())
	require.NoError(t, err, "GetCryptoDepositAddress must not error")
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetRecentTrades(t.Context(), testPair, asset.Spot)
	assert.NoError(t, err, "GetRecentTrades should not error")
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricTrades(t.Context(), testPair, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported)
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateTicker(t.Context(), testPair, asset.Spot)
	assert.NoError(t, err, "UpdateTicker should not error")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()

	err := e.UpdateTickers(t.Context(), asset.Spot)
	require.NoError(t, err, "UpdateTickers must not error")

	enabled, err := e.GetEnabledPairs(asset.Spot)
	require.NoError(t, err, "GetEnabledPairs must not error")

	for _, pair := range enabled {
		_, err := ticker.GetTicker(e.Name, pair, asset.Spot)
		require.NoError(t, err, "GetTicker must not return error for enabled pair")
	}
}

func TestGetCryptoPaymentProvidersList(t *testing.T) {
	t.Parallel()
	_, err := e.GetCryptoPaymentProvidersList(t.Context())
	require.NoError(t, err, "GetCryptoPaymentProvidersList must not error")
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	_, err := e.GetAvailableTransferChains(t.Context(), currency.USDT)
	require.NoError(t, err, "GetAvailableTransferChains must not error")
}

func TestGetAccountFundingHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAccountFundingHistory(t.Context())
	require.NoError(t, err, "GetAccountFundingHistory must not error")
}

func TestGetWithdrawalsHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetWithdrawalsHistory(t.Context(), currency.BTC, asset.Spot)
	require.NoError(t, err, "GetWithdrawalsHistory must not error")
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		require.NoErrorf(t, err, "cannot get pairs for %s", a)
		require.NotEmptyf(t, pairs, "no pairs for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		require.NoError(t, err)
		assert.NotEmpty(t, resp)
	}
}
