package kraken

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/database"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/futures"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
	"github.com/thrasher-corp/gocryptotrader/portfolio/banking"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

var (
	e                *Exchange
	spotLiveExchange *Exchange
	futuresTestPair  = currency.NewPairWithDelimiter("PF", "XBTUSD", "_")
)

// Please add your own API keys to do correct due diligence testing.
// canManipulateRealOrders is retained for the pre-existing wrapper and Futures
// tests. New Spot endpoint live mutations use endpoint-specific opt-ins so one
// enabled test cannot trigger a different destructive endpoint.
const (
	canManipulateRealOrders = false

	canAmendRealSpotOrder              = false
	canCancelAllRealSpotOrders         = false
	canArmRealSpotDeadMansSwitch       = false
	canValidateRealSpotOrderBatch      = false
	canCancelRealSpotOrderBatch        = false
	canValidateRealSpotOrder           = false
	canCancelRealSpotOrder             = false
	canWithdrawRealSpotFunds           = false
	canCancelRealSpotWithdrawal        = false
	canTransferRealSpotWalletFunds     = false
	canAllocateRealSpotEarnFunds       = false
	canDeallocateRealSpotEarnFunds     = false
	canRequestRealSpotExportReport     = false
	canDeleteRealSpotExportReport      = false
	canCreateRealSpotSubaccount        = false
	canTransferRealSpotSubaccountFunds = false
)

var apiCredentials = &accounts.Credentials{
	Key:    "",
	Secret: "",
}

func fillDataHandler(t *testing.T, ex *Exchange) {
	t.Helper()
	ex.Websocket.DataHandler = stream.NewRelay(1)
	require.NoError(t, ex.Websocket.DataHandler.Send(t.Context(), "filler"), "DataHandler.Send must accept its first message")
}

func cloneExchangeConfig(t *testing.T) *config.Exchange {
	t.Helper()
	encoded, err := json.Marshal(e.Config)
	require.NoError(t, err, "json.Marshal must encode the exchange config")
	var cloned config.Exchange
	require.NoError(t, json.Unmarshal(encoded, &cloned), "json.Unmarshal must decode the exchange config")
	return &cloned
}

func TestSetDefaults(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	ex.SetDefaults()
	assert.Equal(t, "Kraken", ex.Name, "SetDefaults should set the exchange name")
	assert.True(t, ex.SupportsAsset(asset.Spot), "SetDefaults should enable Spot")
	assert.True(t, ex.SupportsAsset(asset.Futures), "SetDefaults should enable Futures")
	assert.False(t, ex.IsAssetWebsocketSupported(asset.Futures), "SetDefaults should disable Futures WebSocket support")
	publicURL, err := ex.API.Endpoints.GetURL(exchange.WebsocketSpot)
	require.NoError(t, err, "SetDefaults must set the public WebSocket URL")
	assert.Equal(t, krakenWSURL, publicURL, "SetDefaults should use Spot WebSocket v2")
	authURL, err := ex.API.Endpoints.GetURL(exchange.WebsocketSpotSupplementary)
	require.NoError(t, err, "SetDefaults must set the authenticated WebSocket URL")
	assert.Equal(t, krakenAuthWSURL, authURL, "SetDefaults should use authenticated Spot WebSocket v2")
}

func TestSetup(t *testing.T) {
	t.Parallel()

	require.Error(t, new(Exchange).Setup(nil), "Setup must reject a nil exchange config")

	disabled := cloneExchangeConfig(t)
	disabled.Enabled = false
	disabledExchange := new(Exchange)
	require.NoError(t, disabledExchange.Setup(disabled), "Setup must accept a disabled exchange")
	assert.False(t, disabledExchange.IsEnabled(), "Setup should keep a disabled exchange disabled")

	invalidConfig := cloneExchangeConfig(t)
	invalidConfig.API.Endpoints["InvalidURL"] = "https://example.com"
	invalidExchange := new(Exchange)
	invalidExchange.SetDefaults()
	require.Error(t, invalidExchange.Setup(invalidConfig), "Setup must return SetupDefaults errors")

	missingPublic := new(Exchange)
	missingPublic.SetDefaults()
	missingPublic.API.Endpoints = missingPublic.NewEndpoints()
	require.ErrorIs(t, missingPublic.Setup(cloneExchangeConfig(t)), exchange.ErrEndpointPathNotFound, "Setup must require the public WebSocket endpoint")

	invalidWebsocket := new(Exchange)
	invalidWebsocket.SetDefaults()
	invalidWebsocketConfig := cloneExchangeConfig(t)
	invalidWebsocketConfig.WebsocketTrafficTimeout = time.Millisecond
	require.Error(t, invalidWebsocket.Setup(invalidWebsocketConfig), "Setup must return WebSocket manager setup errors")

	missingAuth := new(Exchange)
	missingAuth.SetDefaults()
	missingAuth.API.Endpoints = missingAuth.NewEndpoints()
	require.NoError(t, missingAuth.API.Endpoints.SetDefaultEndpoints(map[exchange.URL]string{
		exchange.WebsocketSpot: krakenWSURL,
	}), "SetDefaultEndpoints must set only the public WebSocket endpoint")
	require.ErrorIs(t, missingAuth.Setup(cloneExchangeConfig(t)), exchange.ErrEndpointPathNotFound, "Setup must require the authenticated WebSocket endpoint")

	ex := new(Exchange)
	ex.SetDefaults()
	require.NoError(t, ex.Setup(cloneExchangeConfig(t)), "Setup must configure current Spot WebSocket connections")
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
}

func TestGetServerTime(t *testing.T) {
	t.Parallel()
	st, err := e.GetServerTime(t.Context(), asset.Spot)
	require.NoError(t, err, "GetServerTime must not error")
	assert.WithinRange(t, st, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), "ServerTime should be within a day of now")

	_, err = newSpotNullResultExchange(t).GetServerTime(t.Context(), asset.Spot)
	require.ErrorIs(t, err, common.ErrNoResponse, "GetServerTime must reject a null response")
	_, err = newSpotErrorExchange(t).GetServerTime(t.Context(), asset.Spot)
	require.ErrorIs(t, err, errSpotTransport, "GetServerTime must surface request errors")
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		t.Run(a.String(), func(t *testing.T) {
			t.Parallel()
			require.NoError(t, e.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must not error")
			pairs, err := e.CurrencyPairs.GetPairs(a, false)
			require.NoError(t, err, "GetPairs must not error")
			for _, p := range pairs {
				l, err := e.GetOrderExecutionLimits(a, p)
				require.NoError(t, err, "GetOrderExecutionLimits must not error")
				assert.Positive(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive")
				assert.Positive(t, l.PriceStepIncrementSize, "PriceStepIncrementSize should be positive")
			}
		})
	}

	t.Run("unsupported asset", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, e.UpdateOrderExecutionLimits(t.Context(), asset.Binary), asset.ErrNotSupported)
	})
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTradablePairs(t.Context(), asset.Futures)
	assert.NoError(t, err, "FetchTradablePairs should not error")
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	_, err := e.UpdateTicker(t.Context(), spotTestPair, asset.Spot)
	assert.NoError(t, err, "UpdateTicker spot asset should not error")

	_, err = e.UpdateTicker(t.Context(), futuresTestPair, asset.Futures)
	assert.NoError(t, err, "UpdateTicker futures asset should not error")
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")

	testexch.UpdatePairsOnce(t, e)

	err := e.UpdateTickers(t.Context(), asset.Spot)
	require.NoError(t, err, "UpdateTickers must not error")

	ap, err := e.GetAvailablePairs(asset.Spot)
	require.NoError(t, err, "GetAvailablePairs must not error")

	for i := range ap {
		_, err = ticker.GetTicker(e.Name, ap[i], asset.Spot)
		assert.NoErrorf(t, err, "GetTicker should not error for %s", ap[i])
	}

	ap, err = e.GetAvailablePairs(asset.Futures)

	require.NoError(t, err, "GetAvailablePairs must not error")
	err = e.UpdateTickers(t.Context(), asset.Futures)
	require.NoError(t, err, "UpdateTickers must not error")

	for i := range ap {
		_, err = ticker.GetTicker(e.Name, ap[i], asset.Futures)
		assert.NoErrorf(t, err, "GetTicker should not error for %s", ap[i])
	}

	err = e.UpdateTickers(t.Context(), asset.Index)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "UpdateTickers should error correctly for asset.Index")
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	ex.Name += "-UpdateOrderbook"

	_, err := ex.UpdateOrderbook(t.Context(), spotTestPair, asset.Spot)
	assert.NoError(t, err, "UpdateOrderbook spot asset should not error")
	_, err = ex.UpdateOrderbook(t.Context(), futuresTestPair, asset.Futures)
	assert.NoError(t, err, "UpdateOrderbook futures asset should not error")
}

func TestFuturesBatchOrder(t *testing.T) {
	t.Parallel()
	req := []PlaceBatchOrderData{{
		PlaceOrderType: "meow",
		OrderID:        "test123",
		Symbol:         futuresTestPair.Lower().String(),
	}}
	_, err := e.FuturesBatchOrder(t.Context(), req)
	assert.ErrorIs(t, err, errInvalidBatchOrderType, "FuturesBatchOrder should error correctly")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	req[0].PlaceOrderType = "cancel"
	_, err = e.FuturesBatchOrder(t.Context(), req)
	assert.NoError(t, err, "FuturesBatchOrder should not error")
}

func TestFuturesEditOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesEditOrder(t.Context(), "test123", "", 5.2, 1, 0)
	assert.NoError(t, err, "FuturesEditOrder should not error")
}

func TestFuturesSendOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesSendOrder(t.Context(), order.Limit, futuresTestPair, "buy", "", "", "", order.ImmediateOrCancel, 1, 1, 0.9)
	assert.NoError(t, err, "FuturesSendOrder should not error")
}

func TestFuturesCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesCancelOrder(t.Context(), "test123", "")
	assert.NoError(t, err, "FuturesCancelOrder should not error")
}

func TestFuturesGetFills(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesGetFills(t.Context(), time.Now().Add(-time.Hour*24))
	assert.NoError(t, err, "FuturesGetFills should not error")
}

func TestFuturesTransfer(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesTransfer(t.Context(), "cash", "futures", "btc", 2)
	assert.NoError(t, err, "FuturesTransfer should not error")
}

func TestFuturesGetOpenPositions(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesGetOpenPositions(t.Context())
	assert.NoError(t, err, "FuturesGetOpenPositions should not error")
}

func TestFuturesNotifications(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesNotifications(t.Context())
	assert.NoError(t, err, "FuturesNotifications should not error")
}

func TestFuturesCancelAllOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesCancelAllOrders(t.Context(), futuresTestPair)
	assert.NoError(t, err, "FuturesCancelAllOrders should not error")
}

func TestGetFuturesAccountData(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetFuturesAccountData(t.Context())
	assert.NoError(t, err, "GetFuturesAccountData should not error")
}

func TestFuturesCancelAllOrdersAfter(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesCancelAllOrdersAfter(t.Context(), 50)
	assert.NoError(t, err, "FuturesCancelAllOrdersAfter should not error")
}

func TestFuturesOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesOpenOrders(t.Context())
	assert.NoError(t, err, "FuturesOpenOrders should not error")
}

func TestFuturesRecentOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.FuturesRecentOrders(t.Context(), futuresTestPair)
	assert.NoError(t, err, "FuturesRecentOrders should not error")
}

func TestFuturesWithdrawToSpotWallet(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesWithdrawToSpotWallet(t.Context(), "xbt", 5)
	assert.NoError(t, err, "FuturesWithdrawToSpotWallet should not error")
}

func TestFuturesGetTransfers(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.FuturesGetTransfers(t.Context(), time.Now().Add(-time.Hour*24))
	assert.NoError(t, err, "FuturesGetTransfers should not error")
}

func TestGetFuturesOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesOrderbook(t.Context(), futuresTestPair)
	assert.NoError(t, err, "GetFuturesOrderbook should not error")
}

func TestGetFuturesMarkets(t *testing.T) {
	t.Parallel()
	_, err := e.GetInstruments(t.Context())
	assert.NoError(t, err, "GetInstruments should not error")
}

func TestGetFuturesTickers(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesTickers(t.Context())
	assert.NoError(t, err, "GetFuturesTickers should not error")
}

func TestGetFuturesTradeHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesTradeHistory(t.Context(), futuresTestPair, time.Now().Add(-time.Hour*24))
	assert.NoError(t, err, "GetFuturesTradeHistory should not error")
}

func setFeeBuilder() *exchange.FeeBuilder {
	return &exchange.FeeBuilder{
		Amount:              1,
		FeeType:             exchange.CryptocurrencyTradeFee,
		Pair:                currency.NewPair(currency.XXBT, currency.ZUSD),
		PurchasePrice:       1,
		FiatCurrency:        currency.USD,
		BankTransactionType: exchange.WireTransfer,
	}
}

func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	t.Parallel()
	feeBuilder := setFeeBuilder()
	f, err := e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFeeByType must not error")
	assert.Positive(t, f, "GetFeeByType should return a positive value")
	if !sharedtestvalues.AreAPICredentialsSet(e) {
		assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "GetFeeByType should set FeeType correctly")
	} else {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "GetFeeByType should set FeeType correctly")
	}
}

// TestGetFee exercises GetFee
func TestGetFee(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t, allSpotFixtures...)
	for _, tc := range []struct {
		name      string
		feeType   exchange.FeeType
		maker     bool
		bankType  exchange.InternationalBankTransactionType
		price     float64
		expected  float64
		assertFee bool
	}{
		{name: "taker trade", feeType: exchange.CryptocurrencyTradeFee, price: 100},
		{name: "maker trade", feeType: exchange.CryptocurrencyTradeFee, maker: true, price: 100},
		{name: "cryptocurrency withdrawal", feeType: exchange.CryptocurrencyWithdrawalFee},
		{name: "wire deposit", feeType: exchange.InternationalBankDepositFee, bankType: exchange.WireTransfer, expected: 5, assertFee: true},
		{name: "non-wire deposit", feeType: exchange.InternationalBankDepositFee},
		{name: "cryptocurrency deposit", feeType: exchange.CryptocurrencyDepositFee},
		{name: "bank withdrawal", feeType: exchange.InternationalBankWithdrawalFee},
		{name: "offline trade", feeType: exchange.OfflineTradeFee, price: 100, expected: 0.16, assertFee: true},
		{name: "negative fee clamp", feeType: exchange.OfflineTradeFee, price: -100, expected: 0, assertFee: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			feeBuilder := setFeeBuilder()
			feeBuilder.FeeType = tc.feeType
			feeBuilder.IsMaker = tc.maker
			feeBuilder.BankTransactionType = tc.bankType
			feeBuilder.PurchasePrice = tc.price
			fee, err := ex.GetFee(t.Context(), feeBuilder)
			require.NoError(t, err, "GetFee must not error for supported fee types")
			if tc.assertFee {
				assert.Equal(t, tc.expected, fee, "GetFee should return the expected fee")
			}
		})
	}

	feeBuilder := setFeeBuilder()
	_, err := new(Exchange).GetFee(t.Context(), feeBuilder)
	require.Error(t, err, "GetFee must surface pair-format errors")

	feeBuilder = setFeeBuilder()
	_, err = newSpotErrorExchange(t).GetFee(t.Context(), feeBuilder)
	require.ErrorIs(t, err, errSpotTransport, "GetFee must surface trade-volume request errors")
	_, err = newSpotNullResultExchange(t).GetFee(t.Context(), feeBuilder)
	require.ErrorIs(t, err, common.ErrNoResponse, "GetFee must reject a null trade-volume response")
	feeBuilder.FeeType = exchange.InternationalBankDepositFee
	_, err = newSpotErrorExchange(t).GetFee(t.Context(), feeBuilder)
	require.ErrorIs(t, err, errSpotTransport, "GetFee must surface deposit-method request errors")
}

func TestCalculateTradingFee(t *testing.T) {
	fee := calculateTradingFee("USD", map[string]TradeVolumeFee{"USD": {Fee: 0.2}}, 100, 2)
	assert.Equal(t, 0.4, fee, "calculateTradingFee should apply the percentage to price and amount")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	exp := exchange.AutoWithdrawCryptoWithSetupText + " & " + exchange.WithdrawCryptoWith2FAText + " & " + exchange.AutoWithdrawFiatWithSetupText + " & " + exchange.WithdrawFiatWith2FAText
	withdrawPermissions := e.FormatWithdrawPermissions()
	assert.Equal(t, exp, withdrawPermissions, "FormatWithdrawPermissions should return correct value")
}

func TestGetActiveOrdersLive(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Pairs:     currency.Pairs{spotTestPair},
		Side:      order.AnySide,
	}

	_, err := e.GetActiveOrders(t.Context(), &getOrdersRequest)
	assert.NoError(t, err, "GetActiveOrders should not error")
}

func TestGetOrderHistoryLive(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	getOrdersRequest := order.MultiOrderRequest{
		Type:      order.AnyType,
		AssetType: asset.Spot,
		Side:      order.AnySide,
	}

	_, err := e.GetOrderHistory(t.Context(), &getOrdersRequest)
	assert.NoError(t, err)
}

// TestGetOrderInfo exercises GetOrderInfo
func TestGetOrderInfoLive(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetOrderInfo(t.Context(), "OZPTPJ-HVYHF-EDIGXS", currency.EMPTYPAIR, asset.Spot)
	assert.ErrorContains(t, err, "order OZPTPJ-HVYHF-EDIGXS not found in response", "GetOrderInfo should report an order missing from the response")
}

// Any tests below this line have the ability to impact your orders on the exchange. Enable canManipulateRealOrders to run them
// ----------------------------------------------------------------------------------------------------------------------------

func TestSubmitOrderLive(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	orderSubmission := &order.Submit{
		Exchange:  e.Name,
		Pair:      spotTestPair,
		Side:      order.Buy,
		Type:      order.Limit,
		Price:     1,
		Amount:    1,
		ClientID:  "meowOrder",
		AssetType: asset.Spot,
	}
	response, err := e.SubmitOrder(t.Context(), orderSubmission)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "SubmitOrder should not error")
		assert.Equal(t, order.New, response.Status, "SubmitOrder should return a New order status")
	} else {
		assert.ErrorIs(t, err, exchange.ErrAuthenticationSupportNotEnabled, "SubmitOrder should error correctly")
	}
}

func TestCancelExchangeOrder(t *testing.T) {
	t.Parallel()

	err := e.CancelOrder(t.Context(), &order.Cancel{
		AssetType: asset.Options,
		OrderID:   "1337",
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "CancelOrder should error on Options asset")

	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	orderCancellation := &order.Cancel{
		OrderID:   "OGEX6P-B5Q74-IGZ72R",
		AssetType: asset.Spot,
	}

	err = e.CancelOrder(t.Context(), orderCancellation)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelOrder should not error")
	} else {
		assert.ErrorIs(t, err, exchange.ErrAuthenticationSupportNotEnabled, "CancelOrder should error correctly")
	}
}

func TestCancelBatchExchangeOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	ordersCancellation := []order.Cancel{
		{Pair: currency.NewPairWithDelimiter(currency.BTC.String(), currency.USD.String(), "/"), OrderID: "OGEX6P-B5Q74-IGZ72R", AssetType: asset.Spot},
		{Pair: currency.NewPairWithDelimiter(currency.BTC.String(), currency.USD.String(), "/"), OrderID: "OGEX6P-B5Q74-IGZ722", AssetType: asset.Spot},
	}

	_, err := e.CancelBatchOrders(t.Context(), ordersCancellation)
	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelBatchOrder should not error")
	} else {
		assert.ErrorIs(t, err, exchange.ErrAuthenticationSupportNotEnabled, "CancelBatchOrders should error correctly")
	}
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	resp, err := e.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Spot})

	if sharedtestvalues.AreAPICredentialsSet(e) {
		assert.NoError(t, err, "CancelAllOrders should not error")
	} else {
		assert.ErrorIs(t, err, exchange.ErrAuthenticationSupportNotEnabled, "CancelBatchOrders should error correctly")
	}

	assert.Empty(t, resp.Status, "CancelAllOrders Status should not contain any failed order errors")
}

// TestUpdateAccountBalances exercises UpdateAccountBalances
func TestUpdateAccountBalances(t *testing.T) {
	t.Parallel()

	for _, a := range []asset.Item{asset.Spot, asset.Futures} {
		_, err := e.UpdateAccountBalances(t.Context(), a)

		if sharedtestvalues.AreAPICredentialsSet(e) {
			assert.NoErrorf(t, err, "UpdateAccountBalances should not error for asset %s", a) // Note Well: Spot and Futures have separate api keys
		} else {
			assert.ErrorIsf(t, err, exchange.ErrAuthenticationSupportNotEnabled, "UpdateAccountBalances should error correctly for asset %s", a)
		}
	}
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()

	_, err := e.ModifyOrder(t.Context(), &order.Modify{AssetType: asset.Spot})
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "ModifyOrder should error correctly")
}

func TestWithdraw(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t, allSpotFixtures...)
	request := &withdraw.Request{
		Exchange:      ex.Name,
		Currency:      currency.BTC,
		Amount:        1,
		Type:          withdraw.Crypto,
		TradePassword: "wallet",
		Crypto:        withdraw.CryptoRequest{Address: core.BitcoinDonationAddress},
	}
	response, err := ex.WithdrawCryptocurrencyFunds(t.Context(), request)
	require.NoError(t, err, "WithdrawCryptocurrencyFunds must not error")
	assert.Equal(t, "WITHDRAWAL", response.ID, "WithdrawCryptocurrencyFunds should return the reference identifier")

	_, err = ex.WithdrawCryptocurrencyFunds(t.Context(), nil)
	require.ErrorIs(t, err, withdraw.ErrRequestCannotBeNil, "WithdrawCryptocurrencyFunds must validate the request")
	_, err = newSpotErrorExchange(t).WithdrawCryptocurrencyFunds(t.Context(), request)
	require.ErrorIs(t, err, errSpotTransport, "WithdrawCryptocurrencyFunds must surface request errors")
	_, err = newSpotNullResultExchange(t).WithdrawCryptocurrencyFunds(t.Context(), request)
	require.ErrorIs(t, err, common.ErrNoResponse, "WithdrawCryptocurrencyFunds must reject a null response")
}

func TestWithdrawFiat(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t, allSpotFixtures...)
	request := &withdraw.Request{
		Exchange:      ex.Name,
		Currency:      currency.USD,
		Amount:        1,
		Type:          withdraw.Fiat,
		TradePassword: "bank",
		Fiat: withdraw.FiatRequest{Bank: banking.Account{
			Enabled:             true,
			AccountNumber:       "123",
			SWIFTCode:           "SWIFT",
			SupportedCurrencies: "USD",
			SupportedExchanges:  ex.Name,
		}},
	}
	response, err := ex.WithdrawFiatFunds(t.Context(), request)
	require.NoError(t, err, "WithdrawFiatFunds must not error")
	assert.Equal(t, "WITHDRAWAL", response.Status, "WithdrawFiatFunds should return the reference identifier")

	_, err = ex.WithdrawFiatFunds(t.Context(), nil)
	require.ErrorIs(t, err, withdraw.ErrRequestCannotBeNil, "WithdrawFiatFunds must validate the request")
	_, err = newSpotErrorExchange(t).WithdrawFiatFunds(t.Context(), request)
	require.ErrorIs(t, err, errSpotTransport, "WithdrawFiatFunds must surface request errors")
	_, err = newSpotNullResultExchange(t).WithdrawFiatFunds(t.Context(), request)
	require.ErrorIs(t, err, common.ErrNoResponse, "WithdrawFiatFunds must reject a null response")
}

func TestWithdrawInternationalBank(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t, allSpotFixtures...)
	request := &withdraw.Request{
		Exchange:      ex.Name,
		Currency:      currency.USD,
		Amount:        1,
		Type:          withdraw.Fiat,
		TradePassword: "bank",
		Fiat: withdraw.FiatRequest{Bank: banking.Account{
			Enabled:             true,
			AccountNumber:       "123",
			SWIFTCode:           "SWIFT",
			SupportedCurrencies: "USD",
			SupportedExchanges:  ex.Name,
		}},
	}
	response, err := ex.WithdrawFiatFundsToInternationalBank(t.Context(), request)
	require.NoError(t, err, "WithdrawFiatFundsToInternationalBank must not error")
	assert.Equal(t, "WITHDRAWAL", response.Status, "WithdrawFiatFundsToInternationalBank should return the reference identifier")

	_, err = ex.WithdrawFiatFundsToInternationalBank(t.Context(), nil)
	require.ErrorIs(t, err, withdraw.ErrRequestCannotBeNil, "WithdrawFiatFundsToInternationalBank must validate the request")
	_, err = newSpotErrorExchange(t).WithdrawFiatFundsToInternationalBank(t.Context(), request)
	require.ErrorIs(t, err, errSpotTransport, "WithdrawFiatFundsToInternationalBank must surface request errors")
	_, err = newSpotNullResultExchange(t).WithdrawFiatFundsToInternationalBank(t.Context(), request)
	require.ErrorIs(t, err, common.ErrNoResponse, "WithdrawFiatFundsToInternationalBank must reject a null response")
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	if sharedtestvalues.AreAPICredentialsSet(e) {
		_, err := e.GetDepositAddress(t.Context(), currency.USDT, "", "")
		if err != nil {
			t.Error("GetDepositAddress() error", err)
		}
	} else {
		_, err := e.GetDepositAddress(t.Context(), currency.BTC, "", "")
		if err == nil {
			t.Error("GetDepositAddress() error can not be nil")
		}
	}
}

// ---------------------------- Websocket tests -----------------------------------------

// TestWsSubscribe tests unauthenticated websocket subscriptions
// Specifically looking to ensure multiple errors are collected and returned and ws.Subscriptions Added/Removed in cases of:
// single pass, single fail, mixed fail, multiple pass, all fail
// No objection to this becoming a fixture test, so long as it integrates through Un/Subscribe roundtrip
func TestWsSubscribe(t *testing.T) {
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup must not error")
	testexch.SetupWs(t, e)

	for _, enabled := range []bool{false, true} {
		require.NoError(t, e.SetPairs(currency.Pairs{
			spotTestPair,
			currency.NewPairWithDelimiter("ETH", "USD", "/"),
			currency.NewPairWithDelimiter("LTC", "ETH", "/"),
			currency.NewPairWithDelimiter("ETH", "XBT", "/"),
			// Enable pairs that won't error locally, so we get upstream errors to test error combinations
			currency.NewPairWithDelimiter("DWARF", "HOBBIT", "/"),
			currency.NewPairWithDelimiter("DWARF", "GOBLIN", "/"),
			currency.NewPairWithDelimiter("DWARF", "ELF", "/"),
		}, asset.Spot, enabled), "SetPairs must not error")
	}

	err := e.Subscribe(subscription.List{{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{spotTestPair}}})
	require.NoError(t, err, "Subscribe must not error for one subscription")
	subs := e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "subs must contain one subscription")
	assert.Equal(t, subscription.SubscribedState, subs[0].State(), "subs[0].State should be subscribed")

	err = e.Subscribe(subscription.List{{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{spotTestPair}}})
	assert.ErrorIs(t, err, subscription.ErrDuplicate, "Subscribe should return ErrDuplicate for the same channel")
	subs = e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "subs must retain one subscription after an error")
	assert.Equal(t, subscription.SubscribedState, subs[0].State(), "subs[0].State should remain subscribed after an error")

	err = e.Subscribe(subscription.List{{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "HOBBIT", "/")}}})
	assert.ErrorContains(t, err, "Currency pair not supported; Channel: ticker Pairs: DWARF/HOBBIT", "Subscribe should return the invalid pair error")
	require.Len(t, e.Websocket.GetSubscriptions(), 1, "GetSubscriptions must retain one subscription after an error")

	// Mix success and failure
	err = e.Subscribe(subscription.List{
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("ETH", "USD", "/")}},
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "HOBBIT", "/")}},
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "ELF", "/")}},
	})
	assert.ErrorContains(t, err, "Currency pair not supported; Channel: ticker Pairs:", "Subscribe should return the mixed invalid pair error")
	assert.ErrorContains(t, err, "DWARF/HOBBIT", "Subscribe should identify the first invalid pair")
	assert.ErrorContains(t, err, "DWARF/ELF", "Subscribe should identify the second invalid pair")
	require.Len(t, e.Websocket.GetSubscriptions(), 2, "GetSubscriptions must contain two subscriptions after mixed results")

	// Just failures
	err = e.Subscribe(subscription.List{
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "HOBBIT", "/")}},
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "GOBLIN", "/")}},
	})
	assert.ErrorContains(t, err, "Currency pair not supported; Channel: ticker Pairs:", "Subscribe should return the invalid pair errors")
	assert.ErrorContains(t, err, "DWARF/HOBBIT", "Subscribe should identify the first invalid pair")
	assert.ErrorContains(t, err, "DWARF/GOBLIN", "Subscribe should identify the second invalid pair")
	require.Len(t, e.Websocket.GetSubscriptions(), 2, "GetSubscriptions must retain two subscriptions after failed additions")

	// Just success
	err = e.Subscribe(subscription.List{
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("ETH", "XBT", "/")}},
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("LTC", "ETH", "/")}},
	})
	assert.NoError(t, err, "Subscribe should not error for multiple valid subscriptions")

	subs = e.Websocket.GetSubscriptions()
	assert.Len(t, subs, 4, "subs should contain four subscriptions")

	err = e.Unsubscribe(subs[:1])
	assert.NoError(t, err, "Unsubscribe should remove one subscription")
	assert.Len(t, e.Websocket.GetSubscriptions(), 3, "GetSubscriptions should contain three subscriptions")

	err = e.Unsubscribe(subscription.List{{Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "WIZARD", "/")}, Key: 1337}})
	assert.ErrorIs(t, err, subscription.ErrNotFound, "Unsubscribe should return ErrNotFound for an unknown subscription")
	assert.ErrorContains(t, err, "DWARF/WIZARD", "Unsubscribe should identify the invalid pair")
	assert.Len(t, e.Websocket.GetSubscriptions(), 3, "GetSubscriptions should retain three subscriptions after an error")

	err = e.Unsubscribe(subscription.List{
		subs[1],
		{Asset: asset.Spot, Channel: subscription.TickerChannel, Pairs: currency.Pairs{currency.NewPairWithDelimiter("DWARF", "EAGLE", "/")}, Key: 1338},
	})
	assert.ErrorIs(t, err, subscription.ErrNotFound, "Unsubscribe should return ErrNotFound for mixed results")
	assert.ErrorContains(t, err, "Channel: ticker Pairs: DWARF/EAGLE", "Unsubscribe should identify the invalid pair in mixed results")

	subs = e.Websocket.GetSubscriptions()
	assert.Len(t, subs, 2, "subs should contain two subscriptions after mixed results")

	err = e.Unsubscribe(subs)
	assert.NoError(t, err, "Unsubscribe should not error for multiple valid subscriptions")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "GetSubscriptions should be empty after removing all channels")

	for _, c := range []string{"ohlc", "ohlc-5"} {
		err = e.Subscribe(subscription.List{{
			Asset:   asset.Spot,
			Channel: c,
			Pairs:   currency.Pairs{spotTestPair},
		}})
		assert.ErrorIs(t, err, subscription.ErrUseConstChannelName, "Subscribe should return ErrUseConstChannelName for a private channel name")
		assert.ErrorContains(t, err, c+" => subscription.CandlesChannel", "Subscribe should identify the replacement constant channel name")
	}
}

// TestWsResubscribe tests websocket resubscription
func TestWsResubscribe(t *testing.T) {
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	testexch.SetupWs(t, e)

	err := e.Subscribe(subscription.List{{Asset: asset.Spot, Channel: subscription.OrderbookChannel, Levels: 1000}})
	require.NoError(t, err, "Subscribe must not error")
	subs := e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "subs must contain one subscription")
	require.Equal(t, subscription.SubscribedState, subs[0].State(), "subs[0].State must be subscribed")

	require.Eventually(t, func() bool {
		b, e2 := e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
		if e2 == nil {
			return !b.LastUpdated.IsZero()
		}
		return false
	}, time.Second*4, time.Millisecond*10, "GetOrderbook must return a streaming orderbook")

	// Set the state to Unsub so we definitely know Resub worked
	err = subs[0].SetState(subscription.UnsubscribingState)
	require.NoError(t, err, "sub.SetState must enter unsubscribing state")

	err = e.Websocket.ResubscribeToChannel(t.Context(), e.Websocket.Conn, subs[0])
	require.NoError(t, err, "Resubscribe must not error")
	require.Equal(t, subscription.SubscribedState, subs[0].State(), "subs[0].State must be subscribed after resubscription")
}

// TestWsOrderbookSub tests orderbook subscriptions for MaxDepth params
func TestWsOrderbookSub(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	testexch.SetupWs(t, e)

	err := e.Subscribe(subscription.List{{
		Asset:   asset.Spot,
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
	}})
	require.NoError(t, err, "Subscribe must accept an omitted Spot book depth")

	subs := e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "Subscribe must retain one default-depth book subscription")
	assert.Equal(t, wsDefaultBookDepth, subs[0].Levels, "Subscribe should store Kraken's default book depth")

	err = e.Unsubscribe(subscription.List{{
		Asset:   asset.Spot,
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
	}})
	require.NoError(t, err, "Unsubscribe must match an omitted Spot book depth")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "Unsubscribe should remove the default-depth book subscription")

	err = e.Subscribe(subscription.List{{
		Asset:   asset.Spot,
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
		Levels:  25,
	}})
	require.NoError(t, err, "Subscribe must accept an explicit supported book depth")
	subs = e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "Subscribe must retain one explicit-depth book subscription")
	assert.Equal(t, 25, subs[0].Levels, "Subscribe should preserve an explicit supported book depth")
	require.NoError(t, e.Unsubscribe(subs), "Unsubscribe must remove an explicit-depth book subscription")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "Unsubscribe should remove the explicit-depth book subscription")

	err = e.Subscribe(subscription.List{{
		Asset:   asset.Spot,
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
		Levels:  42,
	}})
	assert.ErrorContains(t, err, "Subscription depth not supported", "Subscribe should reject an unsupported book depth")
}

// TestWsCandlesSub tests candles subscription for Timeframe params
func TestWsCandlesSub(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	testexch.SetupWs(t, e)

	err := e.Subscribe(subscription.List{{
		Asset:    asset.Spot,
		Channel:  subscription.CandlesChannel,
		Pairs:    currency.Pairs{spotTestPair},
		Interval: kline.OneHour,
	}})
	require.NoError(t, err, "Subscribe must accept a supported candle interval")

	subs := e.Websocket.GetSubscriptions()
	require.Len(t, subs, 1, "subs must contain the candle subscription")

	err = e.Unsubscribe(subs)
	assert.NoError(t, err, "Unsubscribe should not error")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "GetSubscriptions should be empty after Unsubscribe")

	err = e.Subscribe(subscription.List{{
		Asset:    asset.Spot,
		Channel:  subscription.CandlesChannel,
		Pairs:    currency.Pairs{spotTestPair},
		Interval: kline.Interval(time.Minute * time.Duration(127)),
	}})
	assert.ErrorContains(t, err, "Subscription ohlc interval not supported", "Subscribe should reject an unsupported candle interval")
}

func TestWsProcessCandles(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")

	validCandle := json.RawMessage(`{"symbol":"BTC/USD","interval":5,"interval_begin":"2018-11-12T20:35:14Z","open":3586.7,"high":3586.7,"low":3586.6,"close":3586.6,"volume":0.03373,"vwap":3586.68,"trades":2}`)
	err := ex.wsProcessCandles(t.Context(), []json.RawMessage{validCandle})
	require.NoError(t, err, "wsProcessCandles must accept valid candle data")

	select {
	case msg := <-ex.Websocket.DataHandler.C:
		got, ok := msg.Data.(kline.Item)
		require.True(t, ok, "msg.Data must contain a kline item")
		assert.Equal(t, kline.Item{
			Asset:    asset.Spot,
			Pair:     spotTestPair,
			Exchange: ex.Name,
			Interval: kline.FiveMin,
			Candles: []kline.Candle{{
				Time:        time.Date(2018, 11, 12, 20, 35, 14, 0, time.UTC),
				Open:        3586.7,
				High:        3586.7,
				Low:         3586.6,
				Close:       3586.6,
				Volume:      0.03373,
				QuoteVolume: 120.97871640000001,
			}},
		}, got)
	default:
		require.Fail(t, "ex.Websocket.DataHandler.C must contain a candle payload")
	}

	require.ErrorContains(t, ex.wsProcessCandles(t.Context(), []json.RawMessage{json.RawMessage(`{`)}), "error unmarshalling candle data", "wsProcessCandles must reject malformed candle data")
	require.ErrorContains(t, ex.wsProcessCandles(t.Context(), []json.RawMessage{json.RawMessage(`{"symbol":"invalid"}`)}), "error parsing candle symbol", "wsProcessCandles must reject invalid candle symbols")
	fillDataHandler(t, ex)
	require.Error(t, ex.wsProcessCandles(t.Context(), []json.RawMessage{validCandle}), "wsProcessCandles must return candle delivery errors")
}

// TestWsExecutionsSub tests the authenticated executions subscription channel.
func TestWsExecutionsSub(t *testing.T) {
	t.Parallel()

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	testexch.SetupWs(t, e)

	err := e.Subscribe(subscription.List{{Channel: subscription.MyAccountChannel, Authenticated: true}})
	assert.NoError(t, err, "Subscribe should add the executions channel")

	subs := e.Websocket.GetSubscriptions()
	assert.Len(t, subs, 1, "subs should contain the executions subscription")

	err = e.Unsubscribe(subs)
	assert.NoError(t, err, "Unsubscribe should remove the authenticated channel")
	assert.Empty(t, e.Websocket.GetSubscriptions(), "GetSubscriptions should be empty after Unsubscribe")
}

func TestWsProcessSubStatusAuthenticated(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	s := &subscription.Subscription{Channel: subscription.MyAccountChannel, QualifiedChannel: wsExecutions, Authenticated: true}
	require.NoError(t, ex.Websocket.AddSubscriptions(nil, s), "AddSubscriptions must add the authenticated subscription in subscribing state")

	ex.wsProcessSubStatus([]byte(`{"method":"subscribe","result":{"channel":"executions","snap_orders":true,"snap_trades":true},"success":true,"req_id":3}`))
	assert.Equal(t, subscription.SubscribedState, s.State(), "s.State should be subscribed without requiring a pair field")

	ex.wsProcessSubStatus([]byte(`{"method":"unsubscribe","result":{"channel":"executions"},"success":true,"req_id":4}`))
	assert.Equal(t, subscription.UnsubscribedState, s.State(), "s.State should be unsubscribed after the authenticated unsubscribe status")
}

// TestGenerateSubscriptions tests the subscriptions generated from configuration
func TestGenerateSubscriptions(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")

	pairs, err := ex.GetEnabledPairs(asset.Spot)
	require.NoError(t, err, "GetEnabledPairs must not error")
	require.False(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "CanUseAuthenticatedEndpoints must be false by default")
	exp := subscription.List{
		{Channel: subscription.TickerChannel},
		{Channel: subscription.AllTradesChannel},
		{Channel: subscription.CandlesChannel, Interval: kline.OneMin},
		{Channel: subscription.OrderbookChannel, Levels: 1000},
	}
	for _, s := range exp {
		s.QualifiedChannel = channelName(s)
		s.Asset = asset.Spot
		s.Pairs = pairs
	}
	subs, err := ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")
	testsubs.EqualLists(t, exp, subs)

	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
	exp = append(exp, subscription.List{
		{Channel: subscription.MyAccountChannel, QualifiedChannel: wsExecutions},
	}...)
	subs, err = ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")
	testsubs.EqualLists(t, exp, subs)

	ex.Features.Subscriptions = subscription.List{
		{Enabled: true, Asset: asset.Spot, Channel: subscription.OrderbookChannel},
	}
	subs, err = ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must expand an omitted Spot book depth")
	require.NotEmpty(t, subs, "generateSubscriptions must return the configured Spot book subscriptions")
	for _, sub := range subs {
		assert.Equal(t, wsDefaultBookDepth, sub.Levels, "generateSubscriptions should apply the default Spot book depth")
	}

	ex.Features.Subscriptions = subscription.List{
		{Enabled: true, Asset: asset.Spot, Channel: subscription.OrderbookChannel, Levels: 10},
		{Enabled: true, Asset: asset.Spot, Channel: subscription.OrderbookChannel, Levels: 25},
	}
	subs, err = ex.generateSubscriptions()
	require.ErrorIs(t, err, subscription.ErrExclusiveSubscription, "generateSubscriptions must reject conflicting Spot book depths")
	assert.Nil(t, subs, "generateSubscriptions result should be nil when subscription validation fails")
}

// TestWsAddOrder exercises roundtrip of wsAddOrder; See also: mockWsAddOrder
func TestWsAddOrder(t *testing.T) {
	t.Parallel()

	k := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	require.True(t, k.IsWebsocketAuthenticationSupported(), "IsWebsocketAuthenticationSupported must be true")
	limitPrice := 80000.0
	id, err := k.wsAddOrder(t.Context(), &WebsocketAddOrderParams{
		OrderType:  "limit",
		Side:       order.Buy.Lower(),
		Symbol:     "XBT/USD",
		LimitPrice: &limitPrice,
		OrderQty:   1,
	})
	require.NoError(t, err, "wsAddOrder must not error")
	assert.Equal(t, "ONPNXH-KMKMU-F4MR5V", id, "wsAddOrder should return correct order ID")
}

// TestWsCancelOrders exercises roundtrip of wsCancelOrders; See also: mockWsCancelOrders
func TestWsCancelOrders(t *testing.T) {
	t.Parallel()

	k := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	require.True(t, k.IsWebsocketAuthenticationSupported(), "IsWebsocketAuthenticationSupported must be true")

	err := k.wsCancelOrders(t.Context(), []string{"RABBIT", "BATFISH", "SQUIRREL", "CATFISH", "MOUSE"})
	assert.ErrorIs(t, err, errCancellingOrder, "wsCancelOrders should return errCancellingOrder")
	assert.ErrorContains(t, err, "BATFISH", "wsCancelOrders should identify each failed transaction")
	assert.ErrorContains(t, err, "CATFISH", "wsCancelOrders should identify each failed transaction")
	assert.ErrorContains(t, err, "EOrder:Unknown order", "wsCancelOrders should retain the server error")

	err = k.wsCancelOrders(t.Context(), []string{"RABBIT", "SQUIRREL", "MOUSE"})
	assert.NoError(t, err, "wsCancelOrders should accept valid IDs")

	err = k.wsCancelOrders(t.Context(), []string{"GLOBAL", "MOUSE"})
	require.ErrorContains(t, err, "EGeneral:Invalid arguments", "wsCancelOrders must return a whole-request rejection without waiting for per-order responses")
}

func TestWsCancelAllOrders(t *testing.T) {
	t.Parallel()

	k := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	count, err := k.wsCancelAllOrders(t.Context())
	require.NoError(t, err, "wsCancelAllOrders must not error")
	assert.Equal(t, int64(3), count, "wsCancelAllOrders should return the cancelled order count")
}

func TestWsHandleData(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		payload    string
		errContain string
	}{
		{name: "channel message", payload: `{"channel":"status","type":"update","data":[{"api_version":"v2","system":"online"}]}`},
		{name: "method response", payload: `{"method":"pong","req_id":1,"success":true}`},
		{name: "unhandled", payload: `{}`},
		{name: "malformed", payload: `[`, errContain: "error unmarshalling WebSocket message envelope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex := new(Exchange)
			require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
			err := ex.wsHandleData(t.Context(), []byte(tc.payload))
			if tc.errContain != "" {
				require.ErrorContains(t, err, tc.errContain, "wsHandleData must return the expected payload error")
				return
			}
			require.NoError(t, err, "wsHandleData must accept a valid payload")
		})
	}
}

func TestCurrencyToExchange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    currency.Code
		expected currency.Code
	}{
		{input: currency.XBT, expected: currency.BTC},
		{input: currency.XXBT, expected: currency.BTC},
		{input: currency.XDG, expected: currency.DOGE},
		{input: currency.XXDG, expected: currency.DOGE},
		{input: currency.USD, expected: currency.USD},
	} {
		assert.Equal(t, tc.expected, currencyToExchange(tc.input), "currencyToExchange should translate to the exchange symbol")
	}
}

func TestCurrencyFromExchange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    currency.Code
		expected currency.Code
	}{
		{input: currency.BTC, expected: currency.XBT},
		{input: currency.DOGE, expected: currency.XDG},
		{input: currency.USD, expected: currency.USD},
	} {
		assert.Equal(t, tc.expected, currencyFromExchange(tc.input), "currencyFromExchange should translate from the exchange symbol")
	}
}

func TestPairToExchange(t *testing.T) {
	t.Parallel()
	outbound := pairToExchange(currency.NewPair(currency.XBT, currency.XDG))
	assert.True(t, outbound.Equal(currency.NewPair(currency.BTC, currency.DOGE)), "pairToExchange should use exchange symbols")
}

func TestPairFromExchange(t *testing.T) {
	t.Parallel()
	inbound, err := pairFromExchange("BTC/DOGE")
	require.NoError(t, err, "pairFromExchange must parse a valid symbol")
	assert.Equal(t, currency.NewPair(currency.XBT, currency.XDG), inbound, "pairFromExchange should use configured symbols and internal formatting")

	_, err = pairFromExchange("invalid")
	assert.Error(t, err, "pairFromExchange should return an error for an invalid symbol")
}

func TestPairsToExchange(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		currency.Pairs{currency.NewBTCUSD(), currency.NewPair(currency.DOGE, currency.USD)},
		pairsToExchange(currency.Pairs{currency.NewPair(currency.XBT, currency.USD), currency.NewPair(currency.XDG, currency.USD)}),
		"pairsToExchange should use exchange symbols")
}

func TestWsOrderType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    string
		expected order.Type
	}{
		{input: "limit", expected: order.Limit},
		{input: "market", expected: order.Market},
		{input: "stop-loss", expected: order.Stop},
		{input: "stop-loss-limit", expected: order.StopLimit},
		{input: "take-profit", expected: order.TakeProfitMarket},
		{input: "take-profit-limit", expected: order.TakeProfit},
		{input: "trailing-stop", expected: order.TrailingStop},
		{input: "trailing-stop-limit", expected: order.TrailingStopLimit},
		{input: "unknown", expected: order.UnknownType},
	} {
		assert.Equal(t, tc.expected, wsOrderType(tc.input), "wsOrderType should map to the expected type")
	}
}

func TestKrakenOrderTypeName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input    order.Type
		expected string
	}{
		{input: order.Limit, expected: "limit"},
		{input: order.Market, expected: "market"},
		{input: order.Stop, expected: "stop-loss"},
		{input: order.StopLimit, expected: "stop-loss-limit"},
		{input: order.TakeProfitMarket, expected: "take-profit"},
		{input: order.TakeProfit, expected: "take-profit-limit"},
		{input: order.TrailingStop, expected: "trailing-stop"},
		{input: order.TrailingStopLimit, expected: "trailing-stop-limit"},
	} {
		actual, err := krakenOrderTypeName(tc.input)
		require.NoError(t, err, "krakenOrderTypeName must not error for a supported order type")
		assert.Equal(t, tc.expected, actual, "krakenOrderTypeName should return the matching name")
	}
	_, err := krakenOrderTypeName(order.UnknownType)
	assert.ErrorIs(t, err, order.ErrTypeIsInvalid, "krakenOrderTypeName should return order.ErrTypeIsInvalid for an unsupported type")
}

func TestWsOrderStatus(t *testing.T) {
	t.Parallel()
	status, err := wsOrderStatus("pending_new")
	require.NoError(t, err, "wsOrderStatus must not error for pending_new")
	assert.Equal(t, order.Pending, status, "wsOrderStatus should map pending_new to pending")
	status, err = wsOrderStatus("filled")
	require.NoError(t, err, "wsOrderStatus must not error for filled")
	assert.Equal(t, order.Filled, status, "wsOrderStatus should map filled to filled")
	_, err = wsOrderStatus("unsupported")
	assert.Error(t, err, "wsOrderStatus should return an error for an unsupported status")
}

func TestWsAddOrderParamsFromSubmit(t *testing.T) {
	t.Parallel()

	_, err := wsAddOrderParamsFromSubmit(nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "wsAddOrderParamsFromSubmit must return common.ErrNilPointer for a nil submission")

	endTime := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	params, err := wsAddOrderParamsFromSubmit(&order.Submit{
		Type:          order.Limit,
		Side:          order.Buy,
		Pair:          spotTestPair,
		TimeInForce:   order.GoodTillDay | order.PostOnly,
		ReduceOnly:    true,
		Price:         100,
		Amount:        2,
		ClientOrderID: "client-1",
		EndTime:       endTime,
	})
	require.NoError(t, err, "wsAddOrderParamsFromSubmit must map a limit submission")
	expectedLimitPrice := 100.0
	assert.Equal(t, &WebsocketAddOrderParams{
		ClientOrderID: "client-1",
		ExpireTime:    endTime.Format(time.RFC3339),
		LimitPrice:    &expectedLimitPrice,
		OrderQty:      2,
		OrderType:     "limit",
		PostOnly:      true,
		ReduceOnly:    true,
		Side:          "buy",
		Symbol:        "BTC/USD",
		TimeInForce:   "gtd",
	}, params, "wsAddOrderParamsFromSubmit should map all supported limit fields")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Leverage: 2})
	require.ErrorIs(t, err, order.ErrSubmitLeverageNotSupported, "wsAddOrderParamsFromSubmit must reject numeric leverage")

	params, err = wsAddOrderParamsFromSubmit(&order.Submit{
		Type:             order.StopLimit,
		Side:             order.Sell,
		Pair:             spotTestPair,
		Price:            99,
		TriggerPrice:     98,
		TriggerPriceType: order.IndexPrice,
		Amount:           1,
	})
	require.NoError(t, err, "wsAddOrderParamsFromSubmit must map a stop-limit submission")
	if assert.NotNil(t, params.LimitPrice, "params.LimitPrice should be present for a stop-limit submission") {
		assert.Equal(t, 99.0, *params.LimitPrice, "params.LimitPrice should match the stop-limit price")
	}
	assert.Equal(t, &WebsocketOrderTriggers{Price: 98, PriceType: "static", Reference: "index"}, params.Triggers, "params.Triggers should match the stop-limit trigger")

	params, err = wsAddOrderParamsFromSubmit(&order.Submit{
		Type:             order.TrailingStop,
		Side:             order.Sell,
		Pair:             spotTestPair,
		Amount:           1,
		TriggerPriceType: order.LastPrice,
		TrackingMode:     order.Percentage,
		TrackingValue:    5,
	})
	require.NoError(t, err, "wsAddOrderParamsFromSubmit must map a trailing-stop submission")
	assert.Equal(t, &WebsocketOrderTriggers{Price: 5, PriceType: "pct", Reference: "last"}, params.Triggers, "params.Triggers should match the trailing-stop trigger")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Stop, Side: order.Sell, Pair: spotTestPair, Amount: 1})
	require.ErrorIs(t, err, errTriggerPriceNotSet, "wsAddOrderParamsFromSubmit must reject a triggered submission without a trigger price")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.TrailingStop, Side: order.Sell, Pair: spotTestPair, Amount: 1})
	require.ErrorIs(t, err, errTrackingValueNotSet, "wsAddOrderParamsFromSubmit must reject a trailing-stop submission without a tracking value")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.UnknownType})
	require.ErrorIs(t, err, order.ErrTypeIsInvalid, "wsAddOrderParamsFromSubmit must reject an unknown order type")

	params, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Limit, TimeInForce: order.FillOrKill})
	require.NoError(t, err, "wsAddOrderParamsFromSubmit must map a fill-or-kill limit order")
	assert.Equal(t, "fok", params.TimeInForce, "params.TimeInForce should use the v2 fill-or-kill value")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Market, TimeInForce: order.FillOrKill})
	require.ErrorIs(t, err, order.ErrUnsupportedTimeInForce, "wsAddOrderParamsFromSubmit must reject a fill-or-kill market order")

	params, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Market, TimeInForce: order.ImmediateOrCancel})
	require.NoError(t, err, "wsAddOrderParamsFromSubmit must map an immediate-or-cancel market order")
	assert.Equal(t, "ioc", params.TimeInForce, "params.TimeInForce should use the v2 immediate-or-cancel value")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Market, TimeInForce: order.PostOnly})
	require.ErrorIs(t, err, order.ErrUnsupportedTimeInForce, "wsAddOrderParamsFromSubmit must reject a post-only market order")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.StopLimit, TimeInForce: order.PostOnly})
	require.ErrorIs(t, err, order.ErrUnsupportedTimeInForce, "wsAddOrderParamsFromSubmit must reject a post-only stop-limit order")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Limit, TimeInForce: order.GoodTillTime})
	require.ErrorIs(t, err, errEndTimeNotSet, "wsAddOrderParamsFromSubmit must reject a good-till-date order without an end time")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Limit, TimeInForce: order.GoodTillTime, EndTime: time.Now().Add(-time.Minute)})
	require.ErrorIs(t, err, errEndTimeOutOfRange, "wsAddOrderParamsFromSubmit must reject a past good-till-date end time")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Limit, TimeInForce: order.GoodTillTime, EndTime: time.Now().AddDate(0, 1, 1)})
	require.ErrorIs(t, err, errEndTimeOutOfRange, "wsAddOrderParamsFromSubmit must reject a good-till-date end time over one month away")

	goodTillTimeEnd := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	params, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Limit, TimeInForce: order.GoodTillTime, EndTime: goodTillTimeEnd})
	require.NoError(t, err, "wsAddOrderParamsFromSubmit must map a valid good-till-date order")
	assert.Equal(t, goodTillTimeEnd.Format(time.RFC3339), params.ExpireTime, "params.ExpireTime should match the good-till-date expiration")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.Stop, TriggerPrice: 1, TriggerPriceType: order.PriceType(255)})
	require.ErrorIs(t, err, order.ErrUnknownPriceType, "wsAddOrderParamsFromSubmit must reject an unknown trigger price reference")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.TrailingStop, TrackingValue: 1, TriggerPriceType: order.PriceType(255)})
	require.ErrorIs(t, err, order.ErrUnknownPriceType, "wsAddOrderParamsFromSubmit must reject an unknown trailing price reference")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{Type: order.TrailingStop, TrackingValue: 1, TriggerPriceType: order.LastPrice, TrackingMode: order.TrackingMode(255)})
	require.ErrorIs(t, err, order.ErrUnknownTrackingMode, "wsAddOrderParamsFromSubmit must reject an unknown trailing tracking mode")

	params, err = wsAddOrderParamsFromSubmit(&order.Submit{
		Type:               order.TrailingStopLimit,
		TrackingValue:      1,
		TriggerPriceType:   order.LastPrice,
		TrackingMode:       order.Distance,
		LimitTrackingValue: 0,
		LimitTrackingMode:  order.Percentage,
	})
	require.NoError(t, err, "wsAddOrderParamsFromSubmit must map a trailing-stop-limit order")
	if assert.NotNil(t, params.LimitPrice, "params.LimitPrice should preserve a trailing-stop-limit zero price") {
		assert.Zero(t, *params.LimitPrice, "params.LimitPrice should match the trailing-stop-limit zero price")
	}
	assert.Equal(t, "pct", params.LimitPriceType, "params.LimitPriceType should match the trailing-stop-limit price type")
	payload, err := json.Marshal(params)
	require.NoError(t, err, "json.Marshal must encode trailing-stop-limit parameters")
	assert.Contains(t, string(payload), `"limit_price":0`, "payload should contain an explicit trailing-stop-limit zero price")

	_, err = wsAddOrderParamsFromSubmit(&order.Submit{
		Type:               order.TrailingStopLimit,
		TrackingValue:      1,
		TriggerPriceType:   order.LastPrice,
		TrackingMode:       order.Distance,
		LimitTrackingValue: 2,
		LimitTrackingMode:  order.TrackingMode(255),
	})
	require.ErrorIs(t, err, order.ErrUnknownTrackingMode, "wsAddOrderParamsFromSubmit must reject an unknown trailing-stop-limit tracking mode")
}

func TestKrakenTriggerReference(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		priceType order.PriceType
		expected  string
	}{
		{name: "last price", priceType: order.LastPrice, expected: "last"},
		{name: "index price", priceType: order.IndexPrice, expected: "index"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := krakenTriggerReference(tc.priceType)
			require.NoError(t, err, "krakenTriggerReference must not error for a supported reference")
			assert.Equal(t, tc.expected, actual, "krakenTriggerReference should return the matching reference")
		})
	}

	_, err := krakenTriggerReference(order.PriceType(255))
	assert.ErrorIs(t, err, order.ErrUnknownPriceType, "krakenTriggerReference should return order.ErrUnknownPriceType for an unsupported reference")
}

func TestKrakenTrackingPriceType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		mode     order.TrackingMode
		expected string
	}{
		{name: "distance", mode: order.Distance, expected: "quote"},
		{name: "percentage", mode: order.Percentage, expected: "pct"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := krakenTrackingPriceType(tc.mode)
			require.NoError(t, err, "krakenTrackingPriceType must not error for a supported tracking mode")
			assert.Equal(t, tc.expected, actual, "krakenTrackingPriceType should return the matching price type")
		})
	}

	_, err := krakenTrackingPriceType(order.TrackingMode(255))
	assert.ErrorIs(t, err, order.ErrUnknownTrackingMode, "krakenTrackingPriceType should return order.ErrUnknownTrackingMode for an unsupported mode")
}

func TestWebsocketBookLevelJSONUnmarshal(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		payload        string
		expectedPrice  float64
		expectedQty    float64
		expectedRaw    string
		expectedQtyRaw string
		err            bool
	}{
		{name: "invalid JSON", payload: `{`, err: true},
		{name: "quoted", payload: `{"price":"45285.20","qty":"0.00100000"}`, expectedPrice: 45285.2, expectedQty: 0.001, expectedRaw: "45285.20", expectedQtyRaw: "0.00100000"},
		{name: "numeric", payload: `{"price":45285.2,"qty":0.001}`, expectedPrice: 45285.2, expectedQty: 0.001, expectedRaw: "45285.2", expectedQtyRaw: "0.001"},
		{name: "invalid price", payload: `{"price":"invalid","qty":"0.001"}`, err: true},
		{name: "invalid quantity", payload: `{"price":"45285.2","qty":"invalid"}`, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var level websocketBookLevel
			err := json.Unmarshal([]byte(tc.payload), &level)
			if tc.err {
				require.Error(t, err, "json.Unmarshal must reject an invalid websocketBookLevel")
				return
			}
			require.NoError(t, err, "json.Unmarshal must accept a valid websocketBookLevel")
			assert.Equal(t, tc.expectedPrice, level.Price.Float64(), "websocketBookLevel.Price should decode correctly")
			assert.Equal(t, tc.expectedQty, level.Quantity.Float64(), "websocketBookLevel.Quantity should decode correctly")
			assert.Equal(t, tc.expectedRaw, level.Price.String(), "websocketBookLevel.Price should preserve its decimal representation")
			assert.Equal(t, tc.expectedQtyRaw, level.Quantity.String(), "websocketBookLevel.Quantity should preserve its decimal representation")
		})
	}
}

func TestWsProcessStatus(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	for _, tc := range []struct {
		name        string
		data        []json.RawMessage
		errContains string
	}{
		{name: "online v2", data: []json.RawMessage{json.RawMessage(`{"api_version":"v2","system":"online"}`)}},
		{name: "missing item", errContains: "expected one status item"},
		{name: "malformed", data: []json.RawMessage{json.RawMessage(`{`)}, errContains: "error unmarshalling status data"},
		{name: "offline", data: []json.RawMessage{json.RawMessage(`{"api_version":"v2","system":"maintenance"}`)}, errContains: "system status not online"},
		{name: "superseded API", data: []json.RawMessage{json.RawMessage(`{"api_version":"v1","system":"online"}`)}, errContains: "unsupported WebSocket API version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := e.wsProcessStatus(tc.data)
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains, "wsProcessStatus must return the expected error")
				return
			}
			require.NoError(t, err, "wsProcessStatus must accept an online v2 status")
		})
	}
}

func TestValidateExecutionSequence(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.ErrorIs(t, ex.validateExecutionSequence("snapshot", 0), errExecutionSequence, "validateExecutionSequence must reject a missing sequence")
	assert.Zero(t, ex.executionSequence, "ex.executionSequence should be cleared after a missing sequence")

	require.ErrorIs(t, ex.validateExecutionSequence("update", 1), errExecutionSequence, "validateExecutionSequence must reject an update before a snapshot")
	assert.Zero(t, ex.executionSequence, "ex.executionSequence should remain unset before a snapshot")

	require.NoError(t, ex.validateExecutionSequence("snapshot", 10), "validateExecutionSequence must accept a snapshot")
	assert.Equal(t, uint64(10), ex.executionSequence, "ex.executionSequence should store the snapshot sequence")
	require.NoError(t, ex.validateExecutionSequence("update", 11), "validateExecutionSequence must accept a contiguous update")
	assert.Equal(t, uint64(11), ex.executionSequence, "ex.executionSequence should store the update sequence")

	require.ErrorIs(t, ex.validateExecutionSequence("update", 13), errExecutionSequence, "validateExecutionSequence must reject a sequence gap")
	assert.Zero(t, ex.executionSequence, "ex.executionSequence should be cleared after a sequence gap")
	require.ErrorIs(t, ex.validateExecutionSequence("update", 14), errExecutionSequence, "validateExecutionSequence must reject an update after a sequence gap")

	require.NoError(t, ex.validateExecutionSequence("snapshot", 20), "validateExecutionSequence must recover from a new snapshot")
	require.ErrorIs(t, ex.validateExecutionSequence("heartbeat", 21), errExecutionSequence, "validateExecutionSequence must reject an unsupported message type")
	assert.Zero(t, ex.executionSequence, "ex.executionSequence should be cleared after an unsupported message type")
}

func TestWsHandleMessage(t *testing.T) {
	t.Parallel()

	executionRecoveryPending := func(ex *Exchange) bool {
		ex.executionSequenceMtx.Lock()
		defer ex.executionSequenceMtx.Unlock()
		return ex.executionResubPending
	}

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"heartbeat"}`)), "wsHandleMessage must not error for a heartbeat")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"status","data":[{"api_version":"v2","system":"online"}]}`)), "wsHandleMessage must not error for a status update")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"ticker","data":[]}`)), "wsHandleMessage must not error for empty ticker data")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"trade","data":[]}`)), "wsHandleMessage must not error for empty trade data")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"ohlc","data":[]}`)), "wsHandleMessage must not error for empty OHLC data")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"book","type":"snapshot","data":[]}`)), "wsHandleMessage must not error for empty orderbook data")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"snapshot","sequence":1,"data":[]}`)), "wsHandleMessage must not error for an empty executions snapshot")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"update","sequence":2,"data":[]}`)), "wsHandleMessage must not error for a contiguous empty executions update")
	accountSub := &subscription.Subscription{Channel: subscription.MyAccountChannel}
	require.NoError(t, e.Websocket.AddSuccessfulSubscriptions(e.Websocket.AuthConn, accountSub), "AddSuccessfulSubscriptions must add the authenticated subscription")
	resubErr := errors.New("resubscribe failed")
	var resubscribeCalls atomic.Int64
	resubscribeStarted := make(chan struct{})
	releaseResubscribe := make(chan struct{})
	e.Websocket.Unsubscriber = func(subscription.List) error {
		if resubscribeCalls.Add(1) == 1 {
			close(resubscribeStarted)
			<-releaseResubscribe
		}
		return resubErr
	}
	require.ErrorIs(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"update","sequence":4,"data":[]}`)), errExecutionSequence, "wsHandleMessage must return an execution sequence gap")
	require.Eventually(t, func() bool {
		select {
		case <-resubscribeStarted:
			return accountSub.State() == subscription.ResubscribingState
		default:
			return false
		}
	}, time.Second, time.Millisecond, "wsHandleMessage must trigger an asynchronous resubscription after a sequence gap")
	assert.True(t, executionRecoveryPending(e), "executionRecoveryPending should report pending recovery after a sequence gap")
	require.ErrorIs(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"update","sequence":5,"data":[]}`)), errExecutionSequence, "wsHandleMessage must reject updates during recovery")
	assert.Never(t, func() bool {
		return resubscribeCalls.Load() > 1
	}, 50*time.Millisecond, time.Millisecond, "wsHandleMessage should not start duplicate resubscriptions during recovery")
	close(releaseResubscribe)
	require.Eventually(t, func() bool {
		return !executionRecoveryPending(e) && accountSub.State() == subscription.SubscribedState
	}, time.Second, time.Millisecond, "wsHandleMessage must restore retryable state after a failed resubscription")
	require.ErrorIs(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"update","sequence":6,"data":[]}`)), errExecutionSequence, "wsHandleMessage must retry recovery after a failed resubscription")
	require.Eventually(t, func() bool {
		return resubscribeCalls.Load() == 2 && !executionRecoveryPending(e) && accountSub.State() == subscription.SubscribedState
	}, time.Second, time.Millisecond, "wsHandleMessage must allow a later retry after a failed resubscription")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"snapshot","sequence":7,"data":[]}`)), "wsHandleMessage must complete recovery from a fresh snapshot")
	assert.False(t, executionRecoveryPending(e), "executionRecoveryPending should be false after a fresh snapshot")

	failed := new(Exchange)
	require.NoError(t, testexch.Setup(failed), "testexch.Setup must not error")
	failedSub := &subscription.Subscription{Channel: subscription.MyAccountChannel}
	require.NoError(t, failed.Websocket.AddSuccessfulSubscriptions(failed.Websocket.AuthConn, failedSub), "AddSuccessfulSubscriptions must add the authenticated subscription")
	var failedResubscribeCalls atomic.Int64
	failed.Websocket.Unsubscriber = func(subscription.List) error {
		failedResubscribeCalls.Add(1)
		return resubErr
	}
	require.ErrorContains(t, failed.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"snapshot","sequence":1,"data":[{"symbol":"invalid"}]}`)), "error parsing execution symbol", "wsHandleMessage must return execution processing errors")
	require.Eventually(t, func() bool {
		return failedSub.State() == subscription.SubscribedState && failedResubscribeCalls.Load() == 1 && !executionRecoveryPending(failed)
	}, time.Second, time.Millisecond, "wsHandleMessage must trigger recovery after failed execution processing")
	assert.Zero(t, failed.executionSequence, "failed.executionSequence should reset after failed execution processing")
	require.ErrorIs(t, failed.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"update","sequence":2,"data":[]}`)), errExecutionSequence, "wsHandleMessage must reject an update after failed execution processing")
	require.Eventually(t, func() bool {
		return failedResubscribeCalls.Load() == 2 && failedSub.State() == subscription.SubscribedState && !executionRecoveryPending(failed)
	}, time.Second, time.Millisecond, "wsHandleMessage must allow a recovery retry after failed execution processing")

	successful := new(Exchange)
	require.NoError(t, testexch.Setup(successful), "testexch.Setup must not error")
	successfulSub := &subscription.Subscription{Channel: subscription.MyAccountChannel}
	require.NoError(t, successful.Websocket.AddSuccessfulSubscriptions(successful.Websocket.AuthConn, successfulSub), "AddSuccessfulSubscriptions must add the authenticated subscription")
	var successfulResubscribeCalls atomic.Int64
	successful.Websocket.Unsubscriber = func(subscription.List) error {
		successfulResubscribeCalls.Add(1)
		return nil
	}
	successful.Websocket.Subscriber = func(subs subscription.List) error {
		for _, sub := range subs {
			if err := sub.SetState(subscription.SubscribedState); err != nil {
				return err
			}
		}
		return nil
	}
	require.NoError(t, successful.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"snapshot","sequence":1,"data":[]}`)), "wsHandleMessage must accept an initial execution snapshot")
	require.ErrorIs(t, successful.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"update","sequence":3,"data":[]}`)), errExecutionSequence, "wsHandleMessage must trigger recovery for an execution gap")
	require.Eventually(t, func() bool {
		return successfulResubscribeCalls.Load() == 1 && successfulSub.State() == subscription.SubscribedState && executionRecoveryPending(successful)
	}, time.Second, time.Millisecond, "wsHandleMessage must await a fresh snapshot after resubscription")
	require.ErrorIs(t, successful.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"update","sequence":4,"data":[]}`)), errExecutionSequence, "wsHandleMessage must reject an update before the recovery snapshot")
	assert.Never(t, func() bool {
		return successfulResubscribeCalls.Load() > 1
	}, 50*time.Millisecond, time.Millisecond, "wsHandleMessage should not start duplicate resubscriptions during recovery")
	require.ErrorContains(t, successful.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"snapshot","sequence":5,"data":[{"symbol":"invalid"}]}`)), "error parsing execution symbol", "wsHandleMessage must return a failed recovery snapshot error")
	require.Eventually(t, func() bool {
		return successfulResubscribeCalls.Load() == 2 && successfulSub.State() == subscription.SubscribedState && executionRecoveryPending(successful)
	}, time.Second, time.Millisecond, "wsHandleMessage must start another resubscription after a failed recovery snapshot")
	require.NoError(t, successful.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"snapshot","sequence":6,"data":[]}`)), "wsHandleMessage must finish recovery from a fresh snapshot")
	assert.False(t, executionRecoveryPending(successful), "executionRecoveryPending should be false after successful recovery")

	missingSub := new(Exchange)
	require.NoError(t, testexch.Setup(missingSub), "testexch.Setup must not error")
	require.NoError(t, missingSub.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"snapshot","sequence":1,"data":[]}`)), "wsHandleMessage must accept an initial snapshot without a subscription")
	require.ErrorIs(t, missingSub.wsHandleMessage(t.Context(), []byte(`{"channel":"executions","type":"update","sequence":3,"data":[]}`)), errExecutionSequence, "wsHandleMessage must return a gap without a stored subscription")
	assert.False(t, executionRecoveryPending(missingSub), "executionRecoveryPending should remain false without a stored subscription")
	require.NoError(t, e.wsHandleMessage(t.Context(), []byte(`{"channel":"unknown"}`)), "wsHandleMessage must relay an unknown channel")
	require.ErrorContains(t, e.wsHandleMessage(t.Context(), []byte(`{`)), "error unmarshalling WebSocket message", "wsHandleMessage must return an error for malformed data")
}

func TestWsHandleResponse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		payload    string
		matched    bool
		errIs      error
		errContain string
	}{
		{name: "pong", payload: `{"method":"pong","req_id":1,"success":true}`},
		{name: "matched request", payload: `{"method":"add_order","req_id":4,"success":true}`, matched: true},
		{name: "server error", payload: `{"method":"add_order","error":"EOrder:Rejected","req_id":2,"success":false}`, errContain: "EOrder:Rejected"},
		{name: "unmatched request", payload: `{"method":"subscribe","req_id":3,"success":true}`, errIs: websocket.ErrSignatureNotMatched},
		{name: "unhandled", payload: `{"method":"system_status","success":true}`},
		{name: "malformed", payload: `{`, errIs: common.ErrInvalidResponse, errContain: "error unmarshalling WebSocket response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex := new(Exchange)
			require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
			var matched <-chan []byte
			if tc.matched {
				var err error
				matched, err = ex.Websocket.Match.Set(int64(4), 1)
				require.NoError(t, err, "Match.Set must not error")
			}
			err := ex.wsHandleResponse(t.Context(), []byte(tc.payload))
			if tc.errIs != nil {
				require.ErrorIs(t, err, tc.errIs, "wsHandleResponse must return the expected sentinel error")
			}
			if tc.errContain != "" {
				require.ErrorContains(t, err, tc.errContain, "wsHandleResponse must return the expected error")
			}
			if tc.errIs == nil && tc.errContain == "" {
				require.NoError(t, err, "wsHandleResponse must accept a valid response")
			}
			if tc.matched {
				assert.JSONEq(t, tc.payload, string(<-matched), "wsHandleResponse should relay a matched response to the requester")
			}
		})
	}
}

func TestWsProcessTickers(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	validTicker := json.RawMessage(`{"symbol":"BTC/USD","bid":100,"bid_qty":1,"ask":101,"ask_qty":2,"last":100.5,"volume":10,"vwap":100.2,"low":90,"high":110,"change":2,"timestamp":"2024-01-01T00:00:00Z"}`)
	require.NoError(t, e.wsProcessTickers(t.Context(), []json.RawMessage{validTicker}), "wsProcessTickers must accept valid ticker data")
	message := <-e.Websocket.DataHandler.C
	tick, ok := message.Data.(*ticker.Price)
	require.True(t, ok, "message.Data must contain a ticker price")
	assert.True(t, tick.Pair.Equal(spotTestPair), "tick.Pair should translate BTC to XBT")
	assert.Equal(t, 101.0, tick.Ask, "tick.Ask should match")
	assert.Equal(t, 2.0, tick.AskSize, "tick.AskSize should match")
	assert.Equal(t, 100.0, tick.Bid, "tick.Bid should match")
	assert.Equal(t, 1.0, tick.BidSize, "tick.BidSize should match")
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), tick.LastUpdated, "tick.LastUpdated should match")

	require.ErrorContains(t, e.wsProcessTickers(t.Context(), []json.RawMessage{json.RawMessage(`{`)}), "error unmarshalling ticker data", "wsProcessTickers must reject malformed ticker data")
	require.ErrorContains(t, e.wsProcessTickers(t.Context(), []json.RawMessage{json.RawMessage(`{"symbol":"invalid"}`)}), "error parsing ticker symbol", "wsProcessTickers must reject invalid ticker symbols")
	fillDataHandler(t, e)
	require.Error(t, e.wsProcessTickers(t.Context(), []json.RawMessage{validTicker}), "wsProcessTickers must return ticker delivery errors")
}

func TestWsProcessTrades(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	validTrade := json.RawMessage(`{"symbol":"BTC/USD","side":"buy","ord_type":"limit","price":100.25,"qty":0.5,"trade_id":42,"timestamp":"2024-01-01T00:00:01Z"}`)
	e.SetSaveTradeDataStatus(false)
	e.SetTradeFeedStatus(false)
	require.NoError(t, e.wsProcessTrades(t.Context(), []json.RawMessage{validTrade}), "wsProcessTrades must skip processing when both outputs are disabled")
	e.SetTradeFeedStatus(true)
	require.NoError(t, e.wsProcessTrades(t.Context(), []json.RawMessage{validTrade}), "wsProcessTrades must accept valid trade data")
	message := <-e.Websocket.DataHandler.C
	got, ok := message.Data.(trade.Data)
	require.True(t, ok, "message.Data must contain trade data")
	assert.True(t, got.CurrencyPair.Equal(spotTestPair), "got.CurrencyPair should translate BTC to XBT")
	assert.Equal(t, "42", got.TID, "got.TID should match")
	assert.Equal(t, order.Buy, got.Side, "got.Side should match")

	require.ErrorContains(t, e.wsProcessTrades(t.Context(), []json.RawMessage{json.RawMessage(`{`)}), "error unmarshalling trade data", "wsProcessTrades must reject malformed trade data")
	require.ErrorContains(t, e.wsProcessTrades(t.Context(), []json.RawMessage{json.RawMessage(`{"symbol":"invalid","side":"buy"}`)}), "error parsing trade symbol", "wsProcessTrades must reject invalid trade symbols")
	require.Error(t, e.wsProcessTrades(t.Context(), []json.RawMessage{json.RawMessage(`{"symbol":"BTC/USD","side":"invalid"}`)}), "wsProcessTrades must reject invalid trade sides")

	e.SetTradeFeedStatus(false)
	e.SetSaveTradeDataStatus(true)
	require.NoError(t, e.wsProcessTrades(t.Context(), []json.RawMessage{validTrade}), "wsProcessTrades must pass valid trades to the persistence buffer")

	e.SetSaveTradeDataStatus(false)
	e.SetTradeFeedStatus(true)
	fillDataHandler(t, e)
	require.Error(t, e.wsProcessTrades(t.Context(), []json.RawMessage{validTrade}), "wsProcessTrades must return trade delivery errors")
}

func TestWsProcessExecutions(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	require.NoError(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"order_id":"ORDER-1","cl_ord_id":"CLIENT-1","exec_id":"EXEC-1","exec_type":"trade","order_status":"partially_filled","order_type":"limit","side":"buy","symbol":"BTC/USD","order_qty":2,"cum_cost":50,"cum_qty":0.5,"last_qty":0.5,"last_price":100,"limit_price":101,"avg_price":100,"reduce_only":true,"time_in_force":"GTC","fees":[{"asset":"USD","qty":0.2}],"timestamp":"2024-01-01T00:00:01Z"}`)}), "wsProcessExecutions must accept valid execution data")

	message := <-e.Websocket.DataHandler.C
	detail, ok := message.Data.(*order.Detail)
	require.True(t, ok, "message.Data must contain an order detail")
	assert.Equal(t, "ORDER-1", detail.OrderID, "detail.OrderID should match")
	assert.Equal(t, "CLIENT-1", detail.ClientOrderID, "detail.ClientOrderID should match")
	assert.True(t, detail.Pair.Equal(spotTestPair), "detail.Pair should translate BTC to XBT")
	assert.Equal(t, order.PartiallyFilled, detail.Status, "detail.Status should match")
	assert.Equal(t, 1.5, detail.RemainingAmount, "detail.RemainingAmount should match")
	assert.Equal(t, 50.0, detail.Cost, "detail.Cost should match")
	assert.Equal(t, currency.USD, detail.CostAsset, "detail.CostAsset should match the pair quote")
	assert.True(t, detail.ReduceOnly, "detail.ReduceOnly should match")
	assert.Equal(t, order.GoodTillCancel, detail.TimeInForce, "detail.TimeInForce should match")
	assert.Equal(t, 0.2, detail.Fee, "detail.Fee should match")
	assert.Equal(t, currency.USD, detail.FeeAsset, "detail.FeeAsset should match")
	assert.True(t, detail.Date.IsZero(), "detail.Date should remain unset for a trade event")
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC), detail.LastUpdated, "detail.LastUpdated should use the event time")
	require.Len(t, detail.Trades, 1, "detail.Trades must contain the trade execution")
	assert.Equal(t, "EXEC-1", detail.Trades[0].TID, "detail.Trades[0].TID should use the execution ID")
	assert.Equal(t, currency.USD.String(), detail.Trades[0].FeeAsset, "detail.Trades[0].FeeAsset should match")

	for _, tc := range []struct {
		name          string
		execution     json.RawMessage
		expectedQuote currency.Code
	}{
		{
			name:          "BTC fee",
			execution:     json.RawMessage(`{"exec_type":"trade","symbol":"ETH/BTC","fees":[{"asset":"BTC","qty":0.1}]}`),
			expectedQuote: currency.XBT,
		},
		{
			name:          "DOGE fee",
			execution:     json.RawMessage(`{"exec_type":"trade","symbol":"ETH/DOGE","fees":[{"asset":"DOGE","qty":0.2}]}`),
			expectedQuote: currency.XDG,
		},
	} {
		require.NoError(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{tc.execution}), "wsProcessExecutions must accept "+tc.name)
		message = <-e.Websocket.DataHandler.C
		detail, ok = message.Data.(*order.Detail)
		require.True(t, ok, "message.Data must contain a translated order detail")
		assert.Equal(t, tc.expectedQuote, detail.Pair.Quote, "detail.Pair.Quote should use the internal currency code")
		assert.Equal(t, tc.expectedQuote, detail.FeeAsset, "detail.FeeAsset should use the internal currency code")
		require.Len(t, detail.Trades, 1, "detail.Trades must contain the translated execution")
		assert.Equal(t, tc.expectedQuote.String(), detail.Trades[0].FeeAsset, "detail.Trades[0].FeeAsset should use the internal currency code")
	}

	require.NoError(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"order_id":"ORDER-NEW","exec_type":"new","timestamp":"2024-01-01T00:00:02Z"}`)}), "wsProcessExecutions must accept a new order event")
	message = <-e.Websocket.DataHandler.C
	detail, ok = message.Data.(*order.Detail)
	require.True(t, ok, "wsProcessExecutions must emit an order detail for a new event")
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC), detail.Date, "detail.Date should use the new order event time")
	assert.Equal(t, detail.Date, detail.LastUpdated, "detail.LastUpdated should use the new order event time")

	require.NoError(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"order_id":"ORDER-2","exec_type":"trade","trade_id":42}`)}), "wsProcessExecutions must accept a numeric trade ID fallback")
	message = <-e.Websocket.DataHandler.C
	detail, ok = message.Data.(*order.Detail)
	require.True(t, ok, "message.Data must contain a fallback order detail")
	require.Len(t, detail.Trades, 1, "detail.Trades must contain the fallback execution")
	assert.Equal(t, "42", detail.Trades[0].TID, "detail.Trades[0].TID should use the numeric trade ID when exec_id is absent")

	require.ErrorContains(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{`)}), "error unmarshalling execution data", "wsProcessExecutions must reject malformed execution data")
	require.ErrorContains(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"symbol":"invalid"}`)}), "error parsing execution symbol", "wsProcessExecutions must reject invalid execution symbols")
	require.Error(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"side":"invalid"}`)}), "wsProcessExecutions must reject invalid execution sides")
	require.Error(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"order_status":"invalid"}`)}), "wsProcessExecutions must reject invalid execution statuses")
	require.Error(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"time_in_force":"invalid"}`)}), "wsProcessExecutions must reject invalid time in force")
	require.ErrorContains(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{"fees":[{"asset":"USD","qty":0.1},{"asset":"BTC","qty":0.2}]}`)}), "execution fees use multiple assets", "wsProcessExecutions must reject mixed fee assets")
	fillDataHandler(t, e)
	require.Error(t, e.wsProcessExecutions(t.Context(), []json.RawMessage{json.RawMessage(`{}`)}), "wsProcessExecutions must return delivery errors")
}

func TestWsProcessOrderbooks(t *testing.T) {
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	e.Name += "-WsProcessOrderbooks"
	sub := &subscription.Subscription{
		Channel: subscription.OrderbookChannel,
		Pairs:   currency.Pairs{spotTestPair},
		Asset:   asset.Spot,
		Levels:  10,
	}
	require.NoError(t, e.Websocket.AddSuccessfulSubscriptions(e.Websocket.Conn, sub), "AddSuccessfulSubscriptions must add the orderbook subscription")

	payload := []byte(`{"channel":"book","type":"snapshot","data":[{"symbol":"BTC/USD","bids":[{"price":45283.5,"qty":0.10000000},{"price":45283.4,"qty":1.54582015},{"price":45282.1,"qty":0.10000000},{"price":45281.0,"qty":0.10000000},{"price":45280.3,"qty":1.54592586},{"price":45279.0,"qty":0.07990000},{"price":45277.6,"qty":0.03310103},{"price":45277.5,"qty":0.30000000},{"price":45277.3,"qty":1.54602737},{"price":45276.6,"qty":0.15445238}],"asks":[{"price":45285.2,"qty":0.00100000},{"price":45286.4,"qty":1.54571953},{"price":45286.6,"qty":1.54571109},{"price":45289.6,"qty":1.54560911},{"price":45290.2,"qty":0.15890660},{"price":45291.8,"qty":1.54553491},{"price":45294.7,"qty":0.04454749},{"price":45296.1,"qty":0.35380000},{"price":45297.5,"qty":0.09945542},{"price":45299.5,"qty":0.18772827}],"checksum":3310070434}]}`)
	var message websocketMessage
	require.NoError(t, json.Unmarshal(payload, &message), "json.Unmarshal must decode the official checksum example")
	require.NoError(t, e.wsProcessOrderbooks(t.Context(), message.Type, message.Data), "wsProcessOrderbooks must validate the official checksum example")

	book, err := e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.NoError(t, err, "GetOrderbook must return the stored orderbook")
	require.Len(t, book.Bids, 10, "book.Bids must retain ten levels")
	require.Len(t, book.Asks, 10, "book.Asks must retain ten levels")
	assert.Equal(t, "45285.2", book.Asks[0].StrPrice, "book.Asks[0].StrPrice should retain raw precision")
	assert.Equal(t, "0.00100000", book.Asks[0].StrAmount, "book.Asks[0].StrAmount should retain raw precision")

	require.ErrorContains(t, e.wsProcessOrderbooks(t.Context(), "snapshot", []json.RawMessage{json.RawMessage(`{`)}), "error unmarshalling orderbook data", "wsProcessOrderbooks must reject malformed orderbook data")
	require.ErrorContains(t, e.wsProcessOrderbooks(t.Context(), "snapshot", []json.RawMessage{json.RawMessage(`{"symbol":"invalid"}`)}), "error parsing orderbook symbol", "wsProcessOrderbooks must reject invalid orderbook symbols")
	require.ErrorIs(t, e.wsProcessOrderbooks(t.Context(), "snapshot", []json.RawMessage{json.RawMessage(`{"symbol":"ETH/USD"}`)}), subscription.ErrNotFound, "wsProcessOrderbooks must reject data without a subscription")

	require.NoError(t, sub.SetState(subscription.UnsubscribingState), "sub.SetState must enter unsubscribing state")
	require.NoError(t, e.wsProcessOrderbooks(t.Context(), "snapshot", []json.RawMessage{json.RawMessage(`{"symbol":"BTC/USD"}`)}), "wsProcessOrderbooks must ignore updates while unsubscribing")
	require.NoError(t, sub.SetState(subscription.SubscribedState), "sub.SetState must return to subscribed state")
	require.ErrorContains(t, e.wsProcessOrderbooks(t.Context(), "unsupported", []json.RawMessage{json.RawMessage(`{"symbol":"BTC/USD"}`)}), "unsupported orderbook message type", "wsProcessOrderbooks must reject unsupported message types")

	resubErr := errors.New("resubscribe failed")
	resubscribeState := make(chan subscription.State, 1)
	e.Websocket.Unsubscriber = func(subscription.List) error {
		resubscribeState <- sub.State()
		return resubErr
	}
	err = e.wsProcessOrderbooks(t.Context(), "update", []json.RawMessage{json.RawMessage(`{"symbol":"BTC/USD","bids":[{"price":"45283.5","qty":"0.20000000"}],"checksum":1}`)})
	require.ErrorIs(t, err, errInvalidChecksum, "wsProcessOrderbooks must return an invalid checksum")
	_, err = e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.ErrorIs(t, err, orderbook.ErrOrderbookInvalid, "GetOrderbook must report synchronous invalidation after an invalid checksum")
	select {
	case state := <-resubscribeState:
		assert.Equal(t, subscription.ResubscribingState, state, "wsProcessOrderbooks should enter resubscribing state after an invalid checksum")
	case <-time.After(time.Second):
		require.FailNow(t, "wsProcessOrderbooks must trigger an asynchronous resubscription after an invalid checksum")
	}
	require.Eventually(t, func() bool {
		return sub.State() == subscription.SubscribedState
	}, time.Second, time.Millisecond, "wsProcessOrderbooks must restore the subscription state after a failed resubscription")
}

func TestPairChannelKey(t *testing.T) {
	t.Parallel()

	nilKey := pairChannelKey{}
	assert.Nil(t, nilKey.GetSubscription(), "nilKey.GetSubscription should return no subscription")
	assert.Equal(t, "Uninitialised pairChannelKey", nilKey.String(), "nilKey.String should identify an uninitialised key")
	assert.False(t, nilKey.Match(nil), "nilKey.Match should return false")

	sub := &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{spotTestPair}}
	pairKey := pairChannelKey{Subscription: sub}
	assert.Same(t, sub, pairKey.GetSubscription(), "pairKey.GetSubscription should expose its subscription")
	assert.Equal(t, sub.String(), pairKey.String(), "pairKey.String should use its subscription")
	assert.False(t, pairKey.Match(nil), "pairKey.Match should not match a nil key")
	assert.False(t, pairKey.Match(subscription.ExactKey{}), "pairKey.Match should not match a key without a subscription")
	assert.False(t, pairChannelKey{Subscription: &subscription.Subscription{
		Channel: subscription.OrderbookChannel,
		Asset:   asset.Spot,
		Pairs:   currency.Pairs{spotTestPair, currency.NewPair(currency.ETH, currency.USD)},
	}}.Match(subscription.ExactKey{Subscription: sub}), "pairChannelKey.Match should reject multiple pairs")

	for _, tc := range []struct {
		name     string
		sub      *subscription.Subscription
		expected bool
	}{
		{name: "Match", sub: sub, expected: true},
		{name: "DifferentChannel", sub: &subscription.Subscription{Channel: subscription.TickerChannel, Asset: asset.Spot, Pairs: currency.Pairs{spotTestPair}}},
		{name: "DifferentAsset", sub: &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.Futures, Pairs: currency.Pairs{spotTestPair}}},
		{name: "DifferentPair", sub: &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{currency.NewPair(currency.ETH, currency.USD)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, pairKey.Match(subscription.ExactKey{Subscription: tc.sub}), "pairKey.Match should return the expected result")
		})
	}
}

func TestWsSubscriptionForPair(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	sub := &subscription.Subscription{Channel: subscription.OrderbookChannel, Asset: asset.Spot, Pairs: currency.Pairs{spotTestPair}}
	require.NoError(t, e.Websocket.AddSuccessfulSubscriptions(e.Websocket.Conn, sub), "AddSuccessfulSubscriptions must add the orderbook subscription")
	assert.Same(t, sub, e.wsSubscriptionForPair(subscription.OrderbookChannel, spotTestPair), "wsSubscriptionForPair should return a matching subscription")
	assert.Nil(t, e.wsSubscriptionForPair(subscription.TickerChannel, spotTestPair), "wsSubscriptionForPair should not return a non-matching subscription")
}

func TestWsProcessOrderbookSnapshot(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Setup must not error")
	e.Name += "-WsProcessOrderbookSnapshot"
	snapshot := new(websocketBook)
	require.NoError(t, json.Unmarshal([]byte(`{"asks":[{"price":"101.0","qty":"2.000"}],"bids":[{"price":"100.0","qty":"1.000"}],"checksum":155902695,"symbol":"BTC/USD"}`), snapshot), "json.Unmarshal must decode the orderbook snapshot")
	require.NoError(t, e.wsProcessOrderbookSnapshot(spotTestPair, wsDefaultBookDepth, snapshot), "wsProcessOrderbookSnapshot must accept a valid default-depth snapshot")
	book, err := e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.NoError(t, err, "GetOrderbook must return the stored snapshot")
	require.Len(t, book.Asks, 1, "book.Asks must contain one level")
	require.Len(t, book.Bids, 1, "book.Bids must contain one level")
	assert.Equal(t, wsDefaultBookDepth, book.MaxDepth, "book.MaxDepth should match Kraken's default depth")
	assert.Equal(t, "2.000", book.Asks[0].StrAmount, "book.Asks[0].StrAmount should preserve raw precision")

	invalid := *snapshot
	invalid.Checksum = 1
	require.ErrorIs(t, e.wsProcessOrderbookSnapshot(spotTestPair, wsDefaultBookDepth, &invalid), errInvalidChecksum, "wsProcessOrderbookSnapshot must reject an invalid checksum")
	_, err = e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.ErrorIs(t, err, orderbook.ErrOrderbookInvalid, "wsProcessOrderbookSnapshot must invalidate a corrupt stored book")

	require.Error(t, e.wsProcessOrderbookSnapshot(currency.Pair{}, wsDefaultBookDepth, snapshot), "wsProcessOrderbookSnapshot must reject an empty pair")
}

func TestWsProcessOrderbookUpdate(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "testexch.Setup must not error")
	e.Name += "-WsProcessOrderbookUpdate"
	require.NoError(t, e.Websocket.Orderbook.LoadSnapshot(&orderbook.Book{
		Pair:                   spotTestPair,
		Asset:                  asset.Spot,
		Exchange:               e.Name,
		LastUpdated:            time.Now(),
		ChecksumStringRequired: true,
		Asks:                   orderbook.Levels{{Price: 101, Amount: 2, StrPrice: "101.0", StrAmount: "2.000"}},
		Bids:                   orderbook.Levels{{Price: 100, Amount: 1, StrPrice: "100.0", StrAmount: "1.000"}},
	}), "LoadSnapshot must load the initial orderbook")
	update := new(websocketBook)
	require.NoError(t, json.Unmarshal([]byte(`{"bids":[{"price":"100.0","qty":"1.500"}],"checksum":260120588}`), update), "json.Unmarshal must decode the orderbook update")
	require.NoError(t, e.wsProcessOrderbookUpdate(spotTestPair, update), "wsProcessOrderbookUpdate must accept a valid update")
	book, err := e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.NoError(t, err, "GetOrderbook must return the updated orderbook")
	require.Len(t, book.Bids, 1, "book.Bids must contain one level")
	assert.Equal(t, 1.5, book.Bids[0].Amount, "book.Bids[0].Amount should be updated")
	assert.Equal(t, "1.500", book.Bids[0].StrAmount, "book.Bids[0].StrAmount should preserve raw precision")

	invalid := *update
	invalid.Checksum = 1
	require.ErrorIs(t, e.wsProcessOrderbookUpdate(spotTestPair, &invalid), errInvalidChecksum, "wsProcessOrderbookUpdate must reject an invalid checksum")
	_, err = e.Websocket.Orderbook.GetOrderbook(spotTestPair, asset.Spot)
	require.ErrorIs(t, err, orderbook.ErrOrderbookInvalid, "wsProcessOrderbookUpdate must invalidate a corrupt stored book")

	missing := new(Exchange)
	require.NoError(t, testexch.Setup(missing), "testexch.Setup must not error")
	require.ErrorIs(t, missing.wsProcessOrderbookUpdate(spotTestPair, update), orderbook.ErrDepthNotFound, "wsProcessOrderbookUpdate must reject an update without a snapshot")
}

func TestWsValidateOrderbookChecksum(t *testing.T) {
	t.Parallel()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "testexch.Setup must not error")
	ex.Name += "-WsValidateOrderbookChecksum"
	require.NoError(t, ex.Websocket.Orderbook.LoadSnapshot(&orderbook.Book{
		Pair:                   spotTestPair,
		Asset:                  asset.Spot,
		Exchange:               ex.Name,
		LastUpdated:            time.Now(),
		ChecksumStringRequired: true,
		Asks:                   orderbook.Levels{{Price: 101, Amount: 2, StrPrice: "101.0", StrAmount: "2.000"}},
		Bids:                   orderbook.Levels{{Price: 100, Amount: 1, StrPrice: "100.0", StrAmount: "1.000"}},
	}), "LoadSnapshot must load the initial orderbook")
	require.NoError(t, ex.wsValidateOrderbookChecksum(spotTestPair, 155902695), "wsValidateOrderbookChecksum must accept the matching checksum")
	require.ErrorIs(t, ex.wsValidateOrderbookChecksum(spotTestPair, 1), errInvalidChecksum, "wsValidateOrderbookChecksum must reject a mismatched checksum")
	require.NoError(t, ex.Websocket.Orderbook.InvalidateOrderbook(spotTestPair, asset.Spot), "InvalidateOrderbook must not error")
	err := ex.wsValidateOrderbookChecksum(spotTestPair, 155902695)
	require.ErrorIs(t, err, orderbook.ErrOrderbookInvalid, "wsValidateOrderbookChecksum must return stored orderbook errors")
	assert.ErrorContains(t, err, "cannot retrieve orderbook for checksum validation", "wsValidateOrderbookChecksum should describe retrieval failures")
}

func TestGetHistoricCandles(t *testing.T) {
	end := time.Now().Truncate(time.Second)
	start := end.Add(-12 * time.Hour)
	candleTime := end.UTC().Truncate(time.Hour).Add(-time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := fmt.Fprintf(w, `{"error":[],"result":{"BTC/USD":[[%d,"1","2","0.5","1.5","1.2","10",3],[%d,"1","2","0.5","1.5","1.2","10",3]],"last":%d}}`, start.Add(-time.Hour).Unix(), candleTime.Unix(), end.Unix())
		assert.NoError(t, writeErr, "Mock response writing should not error")
	}))
	t.Cleanup(server.Close)
	ex := newAuthenticatedSpotExchange(t, server.URL)
	item, err := ex.GetHistoricCandles(t.Context(), spotTestPair, asset.Spot, kline.OneHour, start, end)
	require.NoError(t, err, "GetHistoricCandles must not error")
	require.NotEmpty(t, item.Candles, "GetHistoricCandles must retain in-range candles")

	_, err = ex.GetHistoricCandles(t.Context(), currency.EMPTYPAIR, asset.Spot, kline.OneHour, start, end)
	require.Error(t, err, "GetHistoricCandles must validate the request")
	_, err = newSpotErrorExchange(t).GetHistoricCandles(t.Context(), spotTestPair, asset.Spot, kline.OneHour, start, end)
	require.ErrorIs(t, err, errSpotTransport, "GetHistoricCandles must surface OHLC request errors")
	_, err = newSpotNullResultExchange(t).GetHistoricCandles(t.Context(), spotTestPair, asset.Spot, kline.OneHour, start, end)
	require.ErrorIs(t, err, common.ErrNoResponse, "GetHistoricCandles must reject a null OHLC response")
	_, err = ex.GetHistoricCandles(t.Context(), futuresTestPair, asset.Futures, kline.OneHour, start, end)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetHistoricCandles must reject unsupported assets")
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricCandlesExtended(t.Context(), futuresTestPair, asset.Spot, kline.OneMin, time.Now().Add(-time.Minute*3), time.Now())
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricCandlesExtended should error correctly")
}

func TestFormatExchangeKlineInterval(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		interval kline.Interval
		exp      string
	}{
		{kline.OneMin, "1"},
		{kline.OneDay, "1440"},
	} {
		assert.Equalf(t, tt.exp, e.FormatExchangeKlineInterval(tt.interval), "FormatExchangeKlineInterval should return correct output for %s", tt.interval.Short())
	}
}

func TestGetRecentTrades(t *testing.T) {
	assetTranslator.l.Lock()
	originalAssets := assetTranslator.Assets
	assetTranslator.Assets = nil
	assetTranslator.l.Unlock()
	t.Cleanup(func() {
		assetTranslator.l.Lock()
		assetTranslator.Assets = originalAssets
		assetTranslator.l.Unlock()
	})

	ex, _ := newSpotEndpointExchange(t, allSpotFixtures...)
	assetTranslator.Seed("BTC/USD", "XBTUSD")
	trades, err := ex.GetRecentTrades(t.Context(), spotTestPair, asset.Spot)
	require.NoError(t, err, "GetRecentTrades must not error for Spot")
	require.Len(t, trades, 2, "GetRecentTrades must return every Spot trade")

	_, err = ex.GetRecentTrades(t.Context(), currency.EMPTYPAIR, asset.Spot)
	require.Error(t, err, "GetRecentTrades must validate the pair")
	_, err = newSpotErrorExchange(t).GetRecentTrades(t.Context(), spotTestPair, asset.Spot)
	require.ErrorIs(t, err, errSpotTransport, "GetRecentTrades must surface Spot request errors")
	_, err = newSpotNullResultExchange(t).GetRecentTrades(t.Context(), spotTestPair, asset.Spot)
	require.ErrorContains(t, err, "unable to find symbol", "GetRecentTrades must reject a missing Spot symbol")
	emptyResultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"error":[],"result":{}}`))
		assert.NoError(t, writeErr, "GetRecentTrades response writing should not error for an empty result")
	}))
	t.Cleanup(emptyResultServer.Close)
	_, err = newAuthenticatedSpotExchange(t, emptyResultServer.URL).GetRecentTrades(t.Context(), spotTestPair, asset.Spot)
	require.ErrorContains(t, err, "unable to find symbol", "GetRecentTrades must reject an empty Spot result")
	_, err = newSpotErrorExchange(t).GetRecentTrades(t.Context(), futuresTestPair, asset.Futures)
	require.ErrorIs(t, err, errSpotTransport, "GetRecentTrades must surface Futures request errors")
	futuresServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"elements":[{"uid":"BUY","event":{"execution":{"execution":{"makerOrder":{"direction":"buy","limitPrice":"100","quantity":"2","timestamp":1785888000}}}}},{"uid":"SELL","event":{"execution":{"execution":{"makerOrder":{"direction":"sell","limitPrice":"101","quantity":"1","timestamp":1785888001}}}}}]}`))
		assert.NoError(t, writeErr, "Mock response writing should not error")
	}))
	t.Cleanup(futuresServer.Close)
	futuresTrades, err := newAuthenticatedFuturesExchange(t, futuresServer.URL).GetRecentTrades(t.Context(), futuresTestPair, asset.Futures)
	require.NoError(t, err, "GetRecentTrades must not error for Futures")
	require.Len(t, futuresTrades, 2, "GetRecentTrades must return every Futures trade")
	assert.Equal(t, order.Buy, futuresTrades[0].Side, "GetRecentTrades should map a Futures buy")
	assert.Equal(t, order.Sell, futuresTrades[1].Side, "GetRecentTrades should map a Futures sell")

	unsupported := new(Exchange)
	require.NoError(t, testexch.Setup(unsupported), "Setup must not error")
	enableTestOptions(t, unsupported)
	_, err = unsupported.GetRecentTrades(t.Context(), currency.NewBTCUSD(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetRecentTrades must reject unsupported assets")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"error":[],"result":{"BTC/USD":[["0","2",1695828271,"b","m","",61044952]],"last":"1695828272000000000"}}`))
		assert.NoError(t, writeErr, "Mock response writing should not error")
	}))
	t.Cleanup(server.Close)
	invalidTradeExchange := newAuthenticatedSpotExchange(t, server.URL)
	invalidTradeExchange.SetSaveTradeDataStatus(true)
	previousDatabaseConfig := database.DB.GetConfig()
	if previousDatabaseConfig == nil {
		previousDatabaseConfig = new(database.Config)
	}
	require.NoError(t, database.DB.SetConfig(&database.Config{Enabled: true}), "SetConfig must enable trade validation")
	t.Cleanup(func() {
		require.NoError(t, database.DB.SetConfig(previousDatabaseConfig), "SetConfig must restore the database configuration")
	})
	_, err = invalidTradeExchange.GetRecentTrades(t.Context(), spotTestPair, asset.Spot)
	require.ErrorContains(t, err, "invalid trade data", "GetRecentTrades must surface trade-buffer errors")
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricTrades(t.Context(), spotTestPair, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricTrades should error")
}

var testOb = orderbook.Book{
	Asks: []orderbook.Level{
		// NOTE: 0.00000500 float64 == 0.000005
		{Price: 0.05005, StrPrice: "0.05005", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05010, StrPrice: "0.05010", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05015, StrPrice: "0.05015", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05020, StrPrice: "0.05020", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05025, StrPrice: "0.05025", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05030, StrPrice: "0.05030", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05035, StrPrice: "0.05035", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05040, StrPrice: "0.05040", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05045, StrPrice: "0.05045", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.05050, StrPrice: "0.05050", Amount: 0.00000500, StrAmount: "0.00000500"},
	},
	Bids: []orderbook.Level{
		{Price: 0.05000, StrPrice: "0.05000", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04995, StrPrice: "0.04995", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04990, StrPrice: "0.04990", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04980, StrPrice: "0.04980", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04975, StrPrice: "0.04975", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04970, StrPrice: "0.04970", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04965, StrPrice: "0.04965", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04960, StrPrice: "0.04960", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04955, StrPrice: "0.04955", Amount: 0.00000500, StrAmount: "0.00000500"},
		{Price: 0.04950, StrPrice: "0.04950", Amount: 0.00000500, StrAmount: "0.00000500"},
	},
}

const krakenAPIDocChecksum = 974947235

func TestChecksumCalculation(t *testing.T) {
	t.Parallel()
	expected := "5005"
	if v := trim("0.05005"); v != expected {
		t.Errorf("expected %s but received %s", expected, v)
	}

	expected = "500"
	if v := trim("0.00000500"); v != expected {
		t.Errorf("expected %s but received %s", expected, v)
	}

	err := validateCRC32(&testOb, krakenAPIDocChecksum)
	if err != nil {
		t.Error(err)
	}
}

func TestGetCharts(t *testing.T) {
	t.Parallel()
	resp, err := e.GetFuturesCharts(t.Context(), "1d", "spot", futuresTestPair, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Candles)

	end := resp.Candles[0].Time.Time()
	_, err = e.GetFuturesCharts(t.Context(), "1d", "spot", futuresTestPair, end.Add(-time.Hour*24*7), end)
	require.NoError(t, err)
}

func TestGetFuturesTrades(t *testing.T) {
	t.Parallel()
	require.True(t, strings.HasSuffix(krakenFuturesSupplementaryURL, "/"), "krakenFuturesSupplementaryURL must end with a slash")

	type requestData struct {
		path       string
		rawQuery   string
		requestURI string
	}

	since := time.UnixMilli(1700000000123)
	before := time.UnixMilli(1700003600456)
	for _, tc := range []struct {
		name              string
		since             time.Time
		before            time.Time
		response          string
		expectedQuery     string
		expectedLen       int64
		expectedToken     string
		expectedUID       string
		expectedTime      time.Time
		expectedJSONError string
		errorContains     string
	}{
		{
			name:     "no range",
			response: `{"elements":[],"len":0}`,
		},
		{
			name:          "since only",
			since:         since,
			response:      `{"elements":[],"len":0}`,
			expectedQuery: "since=1700000000123",
		},
		{
			name:          "before only",
			before:        before,
			response:      `{"elements":[],"len":0}`,
			expectedQuery: "before=1700003600456",
		},
		{
			name:          "complete range",
			since:         since,
			before:        before,
			response:      `{"elements":[],"len":0}`,
			expectedQuery: "before=1700003600456&since=1700000000123",
		},
		{
			name:          "populated response",
			response:      `{"continuationToken":"next-page","elements":[{"event":{"Execution":{"execution":{"makerOrder":{"direction":"Sell","limitPrice":"123.40","quantity":"1.00","timestamp":1699999999123},"price":"123.45","quantity":"0.25","timestamp":1700000000223,"uid":"execution-id"},"takerReducedQuantity":""}},"timestamp":1700000000123,"uid":"trade-id"}],"len":1}`,
			expectedLen:   1,
			expectedToken: "next-page",
			expectedUID:   "trade-id",
			expectedTime:  time.UnixMilli(1700000000123),
		},
		{
			name:              "invalid response",
			response:          `{"elements":`,
			expectedJSONError: "syntax",
		},
		{
			name:              "invalid trade data",
			response:          `{"elements":"invalid"}`,
			expectedJSONError: "type",
		},
		{
			name:          "futures error response",
			response:      `{"result":"error","error":"invalid range"}`,
			errorContains: "invalid range",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			requestC := make(chan requestData, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requestC <- requestData{path: r.URL.Path, rawQuery: r.URL.RawQuery, requestURI: r.RequestURI}:
				default:
					assert.Fail(t, "GetFuturesTrades should send one request")
				}
				_, err := w.Write([]byte(tc.response))
				assert.NoError(t, err, "ResponseWriter.Write should not error")
			}))
			t.Cleanup(server.Close)

			ex := new(Exchange)
			require.NoError(t, testexch.Setup(ex), "Setup must not error")
			require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestFuturesSupplementary.String(), server.URL+"/"), "SetRunningURL must not error")

			resp, err := ex.GetFuturesTrades(t.Context(), futuresTestPair, tc.since, tc.before)
			if tc.expectedJSONError != "" || tc.errorContains != "" {
				switch tc.expectedJSONError {
				case "syntax":
					if json.Implementation == "bytedance/sonic" {
						assert.ErrorContains(t, err, "Syntax error at index", "GetFuturesTrades should return the correct JSON syntax error")
					} else {
						var target *json.SyntaxError
						assert.ErrorAs(t, err, &target, "GetFuturesTrades should return the correct JSON syntax error type")
					}
				case "type":
					if json.Implementation == "bytedance/sonic" {
						assert.ErrorContains(t, err, "Mismatch type", "GetFuturesTrades should return the correct JSON type error")
					} else {
						var target *json.UnmarshalTypeError
						assert.ErrorAs(t, err, &target, "GetFuturesTrades should return the correct JSON type error type")
					}
				}
				if tc.errorContains != "" {
					assert.ErrorContains(t, err, tc.errorContains, "GetFuturesTrades should return the correct error")
				}
			} else {
				require.NoError(t, err, "GetFuturesTrades must not error")
				require.NotNil(t, resp, "GetFuturesTrades response must not be nil")
				assert.Equal(t, tc.expectedLen, resp.Len, "GetFuturesTrades should return the correct response length")
				assert.Equal(t, tc.expectedToken, resp.ContinuationToken, "GetFuturesTrades should return the correct continuation token")
				require.Len(t, resp.Elements, int(tc.expectedLen), "GetFuturesTrades response elements must contain the correct number of entries")
				if tc.expectedUID != "" {
					element := resp.Elements[0]
					execution := element.ExecutionEvent.OuterExecutionHolder.Execution
					assert.Equal(t, tc.expectedUID, element.UID, "GetFuturesTrades should return the correct trade UID")
					assert.Equal(t, tc.expectedTime, element.Timestamp.Time(), "GetFuturesTrades should return the correct trade timestamp")
					assert.Equal(t, "execution-id", execution.UID, "GetFuturesTrades should return the correct execution UID")
					assert.Equal(t, 123.45, execution.Price, "GetFuturesTrades should return the correct execution price")
					assert.Equal(t, 0.25, execution.Quantity, "GetFuturesTrades should return the correct execution quantity")
					assert.Equal(t, time.UnixMilli(1700000000223), execution.Timestamp.Time(), "GetFuturesTrades should return the correct execution timestamp")
					assert.Equal(t, "Sell", execution.MakerOrder.Direction, "GetFuturesTrades should return the correct maker order direction")
					assert.Equal(t, 123.4, execution.MakerOrder.LimitPrice, "GetFuturesTrades should return the correct maker order price")
					assert.Equal(t, 1.0, execution.MakerOrder.Quantity, "GetFuturesTrades should return the correct maker order quantity")
					assert.Equal(t, time.UnixMilli(1699999999123), execution.MakerOrder.Timestamp.Time(), "GetFuturesTrades should return the correct maker order timestamp")
					assert.Empty(t, element.ExecutionEvent.OuterExecutionHolder.TakerReducedQuantity, "GetFuturesTrades should return the correct reduced quantity")
				}
			}
			require.Len(t, requestC, 1, "GetFuturesTrades must send one request")
			request := <-requestC
			const expectedPath = "/history/v2/market/PF_XBTUSD/executions"
			assert.Equal(t, expectedPath, request.path, "GetFuturesTrades should request the correct path")
			assert.Equal(t, tc.expectedQuery, request.rawQuery, "GetFuturesTrades request query should match correctly")
			expectedRequestURI := expectedPath
			if tc.expectedQuery != "" {
				expectedRequestURI += "?" + tc.expectedQuery
			}
			assert.Equal(t, expectedRequestURI, request.requestURI, "GetFuturesTrades should request the correct URI")
		})
	}

	t.Run("pair format error", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		ex.CurrencyPairs.UseGlobalFormat = true
		_, err := ex.GetFuturesTrades(t.Context(), futuresTestPair, time.Time{}, time.Time{})
		assert.ErrorIs(t, err, currency.ErrPairFormatIsNil, "GetFuturesTrades should error correctly when pair format is missing")
	})

	t.Run("endpoint error", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		ex.API.Endpoints = ex.NewEndpoints()
		_, err := ex.GetFuturesTrades(t.Context(), futuresTestPair, time.Time{}, time.Time{})
		assert.ErrorIs(t, err, exchange.ErrEndpointPathNotFound, "GetFuturesTrades should error correctly when the futures endpoint is missing")
	})

	t.Run("request error", func(t *testing.T) {
		t.Parallel()
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := ex.GetFuturesTrades(ctx, futuresTestPair, time.Time{}, time.Time{})
		assert.ErrorIs(t, err, context.Canceled, "GetFuturesTrades should error correctly for a cancelled request")
	})

	t.Run("live request", func(t *testing.T) {
		t.Parallel()
		_, err := e.GetFuturesTrades(t.Context(), futuresTestPair, time.Time{}, time.Time{})
		assert.NoError(t, err, "GetFuturesTrades should not error")

		_, err = e.GetFuturesTrades(t.Context(), futuresTestPair, time.Now().Add(-time.Hour), time.Now())
		assert.NoError(t, err, "GetFuturesTrades should not error")
	})
}

func TestGetFuturesContractDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesContractDetails(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, futures.ErrNotFuturesAsset)

	_, err = e.GetFuturesContractDetails(t.Context(), asset.USDTMarginedFutures)
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.GetFuturesContractDetails(t.Context(), asset.Futures)
	assert.NoError(t, err, "GetFuturesContractDetails should not error")
}

func TestGetLatestFundingRates(t *testing.T) {
	t.Parallel()
	_, err := e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 currency.NewBTCUSD(),
		IncludePredictedRate: true,
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetLatestFundingRates should error")

	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.Futures,
	})
	assert.NoError(t, err, "GetLatestFundingRates should not error")

	err = e.CurrencyPairs.EnablePair(asset.Futures, futuresTestPair)
	assert.Truef(t, err == nil || errors.Is(err, currency.ErrPairAlreadyEnabled), "EnablePair should not error: %s", err)
	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset:                asset.Futures,
		Pair:                 futuresTestPair,
		IncludePredictedRate: true,
	})
	assert.NoError(t, err, "GetLatestFundingRates should not error")
}

func TestIsPerpetualFutureCurrency(t *testing.T) {
	t.Parallel()
	is, err := e.IsPerpetualFutureCurrency(asset.Binary, currency.NewBTCUSDT())
	assert.NoError(t, err)
	assert.False(t, is, "IsPerpetualFutureCurrency should return false for a binary asset")

	is, err = e.IsPerpetualFutureCurrency(asset.Futures, currency.NewBTCUSDT())
	assert.NoError(t, err)
	assert.False(t, is, "IsPerpetualFutureCurrency should return false for a non-perpetual future")

	is, err = e.IsPerpetualFutureCurrency(asset.Futures, futuresTestPair)
	assert.NoError(t, err)
	assert.True(t, is, "IsPerpetualFutureCurrency should return true for a perpetual future")
}

func TestGetOpenInterest(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")

	_, err := e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.ETH.Item,
		Quote: currency.USDT.Item,
		Asset: asset.USDTMarginedFutures,
	})
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	cp1 := currency.NewPair(currency.PF, currency.NewCode("XBTUSD"))
	cp2 := currency.NewPair(currency.PF, currency.NewCode("ETHUSD"))
	sharedtestvalues.SetupCurrencyPairsForExchangeAsset(t, e, asset.Futures, cp1, cp2)

	resp, err := e.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  cp1.Base.Item,
		Quote: cp1.Quote.Item,
		Asset: asset.Futures,
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, resp)

	resp, err = e.GetOpenInterest(t.Context(),
		key.PairAsset{
			Base:  cp1.Base.Item,
			Quote: cp1.Quote.Item,
			Asset: asset.Futures,
		},
		key.PairAsset{
			Base:  cp2.Base.Item,
			Quote: cp2.Quote.Item,
			Asset: asset.Futures,
		})
	assert.NoError(t, err)
	assert.NotEmpty(t, resp)

	resp, err = e.GetOpenInterest(t.Context())
	assert.NoError(t, err)
	assert.NotEmpty(t, resp)
}

// curryWsMockUpgrader handles Kraken specific http auth token responses prior to handling off to standard Websocket upgrader
func curryWsMockUpgrader(tb testing.TB, h mockws.WsMockFunc) http.HandlerFunc {
	tb.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "GetWebSocketsToken") {
			_, err := w.Write([]byte(`{"result":{"token":"mockAuth"}}`))
			assert.NoError(tb, err, "Write should not error")
			return
		}
		mockws.WsMockUpgrader(tb, w, r, h)
	}
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		if len(pairs) == 0 {
			continue
		}
		require.NoErrorf(t, err, "cannot get pairs for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		if a != asset.Spot && a != asset.Futures {
			assert.ErrorIs(t, err, asset.ErrNotSupported)
			continue
		}
		require.NoError(t, err)
		assert.NotEmpty(t, resp)
	}
}

func TestGetFuturesErr(t *testing.T) {
	t.Parallel()

	assert.ErrorContains(t, getFuturesErr(json.RawMessage(`unparsable rubbish`)), "invalid char", "Bad JSON should error correctly")
	assert.NoError(t, getFuturesErr(json.RawMessage(`{"candles":[]}`)), "JSON with no Result should not error")
	assert.NoError(t, getFuturesErr(json.RawMessage(`{"Result":"4 goats"}`)), "JSON with non-error Result should not error")
	assert.ErrorIs(t, getFuturesErr(json.RawMessage(`{"Result":"error"}`)), common.ErrUnknownError, "JSON with error Result should error correctly")
	assert.ErrorContains(t, getFuturesErr(json.RawMessage(`{"Result":"error", "error": "1 goat"}`)), "1 goat", "JSON with an error should error correctly")
	err := getFuturesErr(json.RawMessage(`{"Result":"error", "errors": ["2 goats", "3 goats"]}`))
	assert.ErrorContains(t, err, "2 goat", "JSON with errors should error correctly")
	assert.ErrorContains(t, err, "3 goat", "JSON with errors should error correctly")
	err = getFuturesErr(json.RawMessage(`{"Result":"error", "error": "too many goats", "errors": ["2 goats", "3 goats"]}`))
	assert.ErrorContains(t, err, "2 goat", "JSON with both error and errors should error correctly")
	assert.ErrorContains(t, err, "3 goat", "JSON with both error and errors should error correctly")
	assert.ErrorContains(t, err, "too many goat", "JSON both error and with errors should error correctly")
}

func TestEnforceStandardChannelNames(t *testing.T) {
	for _, n := range []string{
		wsTicker, subscription.TickerChannel, subscription.OrderbookChannel, subscription.CandlesChannel,
		subscription.AllTradesChannel, subscription.MyAccountChannel,
	} {
		assert.NoError(t, enforceStandardChannelNames(&subscription.Subscription{Channel: n}), "Standard channel names and bespoke names should not error")
	}
	for _, n := range []string{wsOrderbook, wsOHLC, wsTrade, wsExecutions, wsOrderbook + "-5"} {
		err := enforceStandardChannelNames(&subscription.Subscription{Channel: n})
		assert.ErrorIsf(t, err, subscription.ErrUseConstChannelName, "Private channel names should not be allowed for %s", n)
	}
	for _, n := range []string{subscription.MyTradesChannel, subscription.MyOrdersChannel} {
		err := enforceStandardChannelNames(&subscription.Subscription{Channel: n})
		assert.ErrorIsf(t, err, subscription.ErrNotSupported, "Superseded private channel names should not be supported for %s", n)
	}
}

func TestWebsocketAuthToken(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	e.setWebsocketAuthToken("meep")
	const n = 69
	var wg sync.WaitGroup
	wg.Add(2 * n)

	start := make(chan struct{})
	for range n {
		go func() {
			defer wg.Done()
			<-start
			e.setWebsocketAuthToken("69420")
		}()
	}
	for range n {
		go func() {
			defer wg.Done()
			<-start
			e.websocketAuthToken()
		}()
	}
	close(start)
	wg.Wait()
	assert.Equal(t, "69420", e.websocketAuthToken(), "websocketAuthToken should return correctly after concurrent reads and writes")
}

func TestSetWebsocketAuthToken(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	e.setWebsocketAuthToken("69420")
	assert.Equal(t, "69420", e.websocketAuthToken())
}
