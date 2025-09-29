package bitflyer

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
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

// Please supply your own keys here for due diligence testing
const (
	apiKey                  = ""
	apiSecret               = ""
	canManipulateRealOrders = false
)

var e *Exchange

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Bitflyer Setup error: %s", err)
	}

	if apiKey != "" && apiSecret != "" {
		e.API.AuthenticatedSupport = true
		e.SetCredentials(apiKey, apiSecret, "", "", "", "")
	}

	os.Exit(m.Run())
}

func TestGetLatestBlockCA(t *testing.T) {
	t.Parallel()
	_, err := e.GetLatestBlockCA(t.Context())
	require.NoError(t, err, "GetLatestBlockCA must not error")
}

func TestGetBlockCA(t *testing.T) {
	t.Parallel()
	_, err := e.GetBlockCA(t.Context(), "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	require.NoError(t, err, "GetBlockCA must not error")
}

func TestGetBlockbyHeightCA(t *testing.T) {
	t.Parallel()
	_, err := e.GetBlockbyHeightCA(t.Context(), 0)
	require.NoError(t, err, "GetBlockbyHeightCA must not error")
}

func TestGetTransactionByHashCA(t *testing.T) {
	t.Parallel()
	_, err := e.GetTransactionByHashCA(t.Context(), "0562d1f063cd4127053d838b165630445af5e480ceb24e1fd9ecea52903cb772")
	require.NoError(t, err, "GetTransactionByHashCA must not error")
}

func TestGetAddressInfoCA(t *testing.T) {
	t.Parallel()
	v, err := e.GetAddressInfoCA(t.Context(), core.BitcoinDonationAddress)
	require.NoError(t, err, "GetAddressInfoCA must not error")
	if v.UnconfirmedBalance == 0 || v.ConfirmedBalance == 0 {
		t.Log("Donation wallet is empty :( - please consider donating")
	}
}

func TestGetMarkets(t *testing.T) {
	t.Parallel()
	markets, err := e.GetMarkets(t.Context())
	require.NoError(t, err, "GetMarkets must not error")
	for _, market := range markets {
		assert.NotEmpty(t, market.ProductCode, "GetMarkets product.ProductCode should not be empty")
		assert.NotEmpty(t, market.MarketType, "GetMarkets product.MarketType should not be empty")
	}
}

func TestGetOrderBook(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderBook(t.Context(), "BTC_JPY")
	require.NoError(t, err, "GetOrderBook must not error")
}

func TestGetTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetTicker(t.Context(), "BTC_JPY")
	require.NoError(t, err, "GetTicker must not error")
}

func TestGetExecutionHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetExecutionHistory(t.Context(), "BTC_JPY")
	require.NoError(t, err, "GetExecutionHistory must not error")
}

func TestGetExchangeStatus(t *testing.T) {
	t.Parallel()
	_, err := e.GetExchangeStatus(t.Context())
	require.NoError(t, err, "GetExchangeStatus must not error")
}

func TestCheckFXString(t *testing.T) {
	t.Parallel()
	p, err := currency.NewPairDelimiter("FXBTC_JPY", "_")
	require.NoError(t, err)
	p = e.CheckFXString(p)
	assert.Equal(t, "FX_BTC", p.Base.String(), "CheckFXString should adjust base currency")
}

func setFeeBuilder() *exchange.FeeBuilder {
	return &exchange.FeeBuilder{
		Amount:              1,
		FeeType:             exchange.CryptocurrencyTradeFee,
		Pair:                currency.NewPair(currency.BTC, currency.LTC),
		PurchasePrice:       1,
		FiatCurrency:        currency.JPY,
		BankTransactionType: exchange.WireTransfer,
	}
}

func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	feeBuilder := setFeeBuilder()
	_, err := e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err)
	if !sharedtestvalues.AreAPICredentialsSet(e) {
		assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "GetFeeByType feeBuilder.FeeType should switch to OfflineTradeFee without credentials")
	} else {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "GetFeeByType feeBuilder.FeeType should remain cryptocurrency with credentials")
	}
}

func TestGetFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()

	if sharedtestvalues.AreAPICredentialsSet(e) {
		// CryptocurrencyTradeFee Basic
		_, err := e.GetFee(feeBuilder)
		assert.NoError(t, err, "GetFee should not error for trade fee")

		// CryptocurrencyTradeFee High quantity
		feeBuilder = setFeeBuilder()
		feeBuilder.Amount = 1000
		feeBuilder.PurchasePrice = 1000
		_, err = e.GetFee(feeBuilder)
		assert.NoError(t, err, "GetFee should not error for high quantity")

		// CryptocurrencyTradeFee IsMaker
		feeBuilder = setFeeBuilder()
		feeBuilder.IsMaker = true
		_, err = e.GetFee(feeBuilder)
		assert.NoError(t, err, "GetFee should not error when maker")

		// CryptocurrencyTradeFee Negative purchase price
		feeBuilder = setFeeBuilder()
		feeBuilder.PurchasePrice = -1000
		_, err = e.GetFee(feeBuilder)
		assert.NoError(t, err, "GetFee should not error for negative purchase price")

		// CryptocurrencyWithdrawalFee Basic
		feeBuilder = setFeeBuilder()
		feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
		_, err = e.GetFee(feeBuilder)
		assert.NoError(t, err, "GetFee should not error for withdrawal fee")
	}

	// CryptocurrencyDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyDepositFee
	_, err := e.GetFee(feeBuilder)
	assert.NoError(t, err, "GetFee should not error for cryptocurrency deposit")

	// InternationalBankDepositFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	feeBuilder.FiatCurrency = currency.JPY
	_, err = e.GetFee(feeBuilder)
	assert.NoError(t, err, "GetFee should not error for international deposit")

	// InternationalBankWithdrawalFee Basic
	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.InternationalBankWithdrawalFee
	feeBuilder.FiatCurrency = currency.JPY
	_, err = e.GetFee(feeBuilder)
	assert.NoError(t, err, "GetFee should not error for international withdrawal")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	expectedResult := exchange.AutoWithdrawFiatText + " & " + exchange.WithdrawCryptoViaWebsiteOnlyText
	withdrawPermissions := e.FormatWithdrawPermissions()
	assert.Equal(t, expectedResult, withdrawPermissions, "FormatWithdrawPermissions should return expected combination")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	_, err := e.GetActiveOrders(t.Context(), &getOrdersRequest)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "GetActiveOrders should not error with credentials")
	} else {
		assert.Error(t, err, "GetActiveOrders should error without credentials")
	}
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	_, err := e.GetOrderHistory(t.Context(), &getOrdersRequest)
	assert.ErrorIs(t, err, common.ErrNotYetImplemented, "GetOrderHistory should return not yet implemented")
}

// Any tests below this line have the ability to impact your orders on the exchange. Enable canManipulateRealOrders to run them
// ----------------------------------------------------------------------------------------------------------------------------

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	orderSubmission := &order.Submit{
		Exchange: e.Name,
		Pair: currency.Pair{
			Base:  currency.BTC,
			Quote: currency.LTC,
		},
		Side:      order.Buy,
		Type:      order.Limit,
		Price:     1,
		Amount:    1,
		ClientID:  "meowOrder",
		AssetType: asset.Spot,
	}
	_, err := e.SubmitOrder(t.Context(), orderSubmission)
	assert.ErrorIs(t, err, common.ErrNotYetImplemented, "SubmitOrder should return not yet implemented")
}

func TestCancelExchangeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	currencyPair := currency.NewPair(currency.LTC, currency.BTC)
	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currencyPair,
		AssetType: asset.Spot,
	}

	err := e.CancelOrder(t.Context(), orderCancellation)
	assert.ErrorIs(t, err, common.ErrNotYetImplemented, "CancelOrder should return not yet implemented")
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	currencyPair := currency.NewPair(currency.LTC, currency.BTC)
	orderCancellation := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
		Pair:      currencyPair,
		AssetType: asset.Spot,
	}

	_, err := e.CancelAllOrders(t.Context(), orderCancellation)
	assert.ErrorIs(t, err, common.ErrNotYetImplemented, "CancelAllOrders should return not yet implemented")
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	withdrawCryptoRequest := withdraw.Request{
		Exchange:    e.Name,
		Amount:      -1,
		Currency:    currency.BTC,
		Description: "WITHDRAW IT ALL",
		Crypto: withdraw.CryptoRequest{
			Address: core.BitcoinDonationAddress,
		},
	}

	_, err := e.WithdrawCryptocurrencyFunds(t.Context(), &withdrawCryptoRequest)
	assert.ErrorIs(t, err, common.ErrNotYetImplemented, "WithdrawCryptocurrencyFunds should return not yet implemented")
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.ModifyOrder(t.Context(), &order.Modify{AssetType: asset.Spot})
	assert.Error(t, err, "ModifyOrder should error when spot order modifications unsupported")
}

func TestWithdrawFiat(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	withdrawFiatRequest := withdraw.Request{}

	_, err := e.WithdrawFiatFunds(t.Context(), &withdrawFiatRequest)
	assert.ErrorIs(t, err, common.ErrNotYetImplemented, "WithdrawFiatFunds should return not yet implemented")
}

func TestWithdrawInternationalBank(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	withdrawFiatRequest := withdraw.Request{}

	_, err := e.WithdrawFiatFundsToInternationalBank(t.Context(), &withdrawFiatRequest)
	assert.ErrorIs(t, err, common.ErrNotYetImplemented, "WithdrawFiatFundsToInternationalBank should return not yet implemented")
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	currencyPair, err := currency.NewPairFromString("BTC_JPY")
	require.NoError(t, err)
	_, err = e.GetRecentTrades(t.Context(), currencyPair, asset.Spot)
	assert.NoError(t, err)
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	currencyPair, err := currency.NewPairFromString("BTC_JPY")
	require.NoError(t, err)
	_, err = e.GetHistoricTrades(t.Context(), currencyPair, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	if err != nil {
		assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricTrades should only error with ErrFunctionNotSupported")
	}
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	err := e.CurrencyPairs.SetAssetEnabled(asset.Futures, false)
	require.NoError(t, err, "SetAssetEnabled must not error")
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		require.NoErrorf(t, err, "cannot get pairs for %s", a)
		require.NotEmptyf(t, pairs, "no pairs for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		require.NoError(t, err, "GetCurrencyTradeURL must not error")
		assert.NotEmpty(t, resp, "GetCurrencyTradeURL should return an url")
	}
}
