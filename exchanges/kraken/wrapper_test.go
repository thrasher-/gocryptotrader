package kraken

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/deposit"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func newAuthenticatedSpotExchange(t *testing.T, serverURL string) *Exchange {
	t.Helper()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	ex.API.AuthenticatedSupport = true
	ex.SetCredentials(&accounts.Credentials{
		Key:    "test-key",
		Secret: base64.StdEncoding.EncodeToString([]byte("test-secret")),
	})
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), serverURL), "SetRunningURL must set the Spot endpoint")
	return ex
}

func newAuthenticatedFuturesExchange(t *testing.T, serverURL string) *Exchange {
	t.Helper()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	ex.API.AuthenticatedSupport = true
	ex.SetCredentials(&accounts.Credentials{
		Key:    "test-key",
		Secret: base64.StdEncoding.EncodeToString([]byte("test-secret")),
	})
	require.True(t, ex.API.AuthenticatedSupport, "SetCredentials must retain authenticated support")
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestFutures.String(), serverURL+"/derivatives"), "SetRunningURL must set the Futures endpoint")
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestFuturesSupplementary.String(), serverURL+"/api/"), "SetRunningURL must set the supplementary Futures endpoint")
	return ex
}

func enableTestOptions(t *testing.T, ex *Exchange) {
	t.Helper()
	require.NoError(t, ex.SetAssetPairStore(asset.Options, currency.PairStore{
		AssetEnabled:  true,
		RequestFormat: &currency.PairFormat{Uppercase: true},
		ConfigFormat:  &currency.PairFormat{Uppercase: true, Delimiter: currency.UnderscoreDelimiter},
	}), "SetAssetPairStore must enable the test asset")
}

func validSubmit(ex *Exchange, a asset.Item) *order.Submit {
	pair := spotTestPair
	if a == asset.Futures {
		pair = futuresTestPair
	}
	return &order.Submit{
		Exchange:  ex.Name,
		Pair:      pair,
		Side:      order.Buy,
		Type:      order.Limit,
		Price:     100,
		Amount:    1,
		AssetType: a,
	}
}

func TestAddOrderRequestFromSubmit(t *testing.T) {
	_, err := addOrderRequestFromSubmit(nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "addOrderRequestFromSubmit must reject a nil submission")

	endTime := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	for _, tc := range []struct {
		name     string
		submit   *order.Submit
		expected *AddOrderRequest
		errIs    error
	}{
		{
			name:     "Default",
			submit:   &order.Submit{Type: order.Limit},
			expected: &AddOrderRequest{OrderType: OrderTypeLimit, Side: "unknown", Price: new(OrderPrice)},
		},
		{
			name: "PostOnlyGoodTillTimeReduceOnly",
			submit: &order.Submit{
				Type:        order.Limit,
				TimeInForce: order.PostOnly | order.GoodTillTime,
				ReduceOnly:  true,
				EndTime:     endTime,
			},
			expected: &AddOrderRequest{
				OrderType:   OrderTypeLimit,
				Side:        "unknown",
				Price:       new(OrderPrice),
				OrderFlags:  []OrderFlag{OrderFlagPostOnly},
				ExpireTime:  endTime,
				ReduceOnly:  true,
				TimeInForce: OrderTimeInForceGTD,
			},
		},
		{
			name:     "GoodTillDay",
			submit:   &order.Submit{Type: order.Limit, TimeInForce: order.GoodTillDay, EndTime: endTime},
			expected: &AddOrderRequest{OrderType: OrderTypeLimit, Side: "unknown", Price: new(OrderPrice), ExpireTime: endTime, TimeInForce: OrderTimeInForceGTD},
		},
		{
			name:     "FillOrKill",
			submit:   &order.Submit{Type: order.Limit, TimeInForce: order.FillOrKill},
			expected: &AddOrderRequest{OrderType: OrderTypeLimit, Side: "unknown", Price: new(OrderPrice), TimeInForce: OrderTimeInForceFOK},
		},
		{
			name:     "ImmediateOrCancel",
			submit:   &order.Submit{Type: order.Market, TimeInForce: order.ImmediateOrCancel},
			expected: &AddOrderRequest{OrderType: OrderTypeMarket, Side: "unknown", TimeInForce: OrderTimeInForceIOC},
		},
		{
			name:   "PostOnlyMarket",
			submit: &order.Submit{Type: order.Market, TimeInForce: order.PostOnly},
			errIs:  order.ErrUnsupportedTimeInForce,
		},
		{
			name:   "FillOrKillMarket",
			submit: &order.Submit{Type: order.Market, TimeInForce: order.FillOrKill},
			errIs:  order.ErrUnsupportedTimeInForce,
		},
		{
			name:   "MissingEndTime",
			submit: &order.Submit{Type: order.Limit, TimeInForce: order.GoodTillTime},
			errIs:  errEndTimeNotSet,
		},
		{
			name:   "PastEndTime",
			submit: &order.Submit{Type: order.Limit, TimeInForce: order.GoodTillTime, EndTime: time.Now().Add(-time.Minute)},
			errIs:  errEndTimeOutOfRange,
		},
		{
			name:   "EndTimeOverOneMonth",
			submit: &order.Submit{Type: order.Limit, TimeInForce: order.GoodTillTime, EndTime: time.Now().AddDate(0, 1, 1)},
			errIs:  errEndTimeOutOfRange,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := addOrderRequestFromSubmit(tc.submit)
			if tc.errIs != nil {
				require.ErrorIs(t, err, tc.errIs, "addOrderRequestFromSubmit must return the expected sentinel")
				assert.Nil(t, result, "addOrderRequestFromSubmit result should be nil on error")
				return
			}
			require.NoError(t, err, "addOrderRequestFromSubmit must map supported options")
			assert.Equal(t, tc.expected, result, "addOrderRequestFromSubmit request should match")
		})
	}

	for _, tc := range []struct {
		name              string
		submit            *order.Submit
		expectedError     error
		expectedPrice     string
		expectedSecondary string
	}{
		{name: "invalid order type", submit: &order.Submit{Type: order.Type(255)}, expectedError: order.ErrTypeIsInvalid},
		{name: "negative leverage", submit: &order.Submit{Type: order.Market, Leverage: -1}, expectedError: errNumericValueInvalid},
		{name: "excessive leverage", submit: &order.Submit{Type: order.Market, Leverage: 256}, expectedError: errNumericValueInvalid},
		{name: "fractional leverage", submit: &order.Submit{Type: order.Market, Leverage: 1.5}, expectedError: errNumericValueInvalid},
		{name: "stop missing trigger", submit: &order.Submit{Type: order.Stop}, expectedError: errTriggerPriceNotSet},
		{name: "stop invalid trigger reference", submit: &order.Submit{Type: order.Stop, TriggerPrice: 100, TriggerPriceType: order.UnknownPriceType}, expectedError: order.ErrUnknownPriceType},
		{name: "stop", submit: &order.Submit{Type: order.Stop, TriggerPrice: 100}},
		{name: "stop limit missing trigger", submit: &order.Submit{Type: order.StopLimit}, expectedError: errTriggerPriceNotSet},
		{name: "stop limit invalid trigger reference", submit: &order.Submit{Type: order.StopLimit, TriggerPrice: 100, TriggerPriceType: order.UnknownPriceType}, expectedError: order.ErrUnknownPriceType},
		{name: "stop limit", submit: &order.Submit{Type: order.StopLimit, TriggerPrice: 100, Price: 99}},
		{name: "trailing stop missing tracking value", submit: &order.Submit{Type: order.TrailingStop}, expectedError: errTrackingValueNotSet},
		{name: "trailing stop invalid trigger reference", submit: &order.Submit{Type: order.TrailingStop, TrackingValue: 5, TrackingMode: order.Distance, TriggerPriceType: order.UnknownPriceType}, expectedError: order.ErrUnknownPriceType},
		{name: "trailing stop non-finite value", submit: &order.Submit{Type: order.TrailingStop, TrackingValue: math.NaN(), TrackingMode: order.Distance}, expectedError: errNumericValueInvalid},
		{name: "trailing stop invalid tracking mode", submit: &order.Submit{Type: order.TrailingStop, TrackingValue: 5}, expectedError: order.ErrUnknownTrackingMode},
		{name: "trailing stop distance down", submit: &order.Submit{Type: order.TrailingStop, TrackingValue: 5, TrackingMode: order.Distance}, expectedPrice: "-5"},
		{name: "trailing stop percentage up", submit: &order.Submit{Type: order.TrailingStop, TrackingValue: 5, TrackingMode: order.Percentage, StopDirection: order.StopUp}, expectedPrice: "+5%"},
		{name: "trailing stop limit non-finite limit", submit: &order.Submit{Type: order.TrailingStopLimit, TrackingValue: 5, TrackingMode: order.Distance, LimitTrackingValue: math.NaN(), LimitTrackingMode: order.Distance}, expectedError: errNumericValueInvalid},
		{name: "trailing stop limit invalid limit mode", submit: &order.Submit{Type: order.TrailingStopLimit, TrackingValue: 5, TrackingMode: order.Distance, LimitTrackingValue: 2}, expectedError: order.ErrUnknownTrackingMode},
		{name: "trailing stop limit", submit: &order.Submit{Type: order.TrailingStopLimit, TrackingValue: 5, TrackingMode: order.Distance, LimitTrackingValue: 2, LimitTrackingMode: order.Percentage}, expectedPrice: "-5", expectedSecondary: "-2%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := addOrderRequestFromSubmit(tc.submit)
			if tc.expectedError != nil {
				require.ErrorIs(t, err, tc.expectedError, "addOrderRequestFromSubmit must return the expected error")
				assert.Nil(t, result, "addOrderRequestFromSubmit result should be nil on error")
				return
			}
			require.NoError(t, err, "addOrderRequestFromSubmit must map the order")
			require.NotNil(t, result, "addOrderRequestFromSubmit must return a request")
			if tc.expectedPrice != "" {
				require.NotNil(t, result.Price, "addOrderRequestFromSubmit must set the tracking price")
				assert.Equal(t, tc.expectedPrice, result.Price.Expression, "addOrderRequestFromSubmit tracking price should match")
			}
			if tc.expectedSecondary != "" {
				require.NotNil(t, result.SecondaryPrice, "addOrderRequestFromSubmit must set the secondary tracking price")
				assert.Equal(t, tc.expectedSecondary, result.SecondaryPrice.Expression, "addOrderRequestFromSubmit secondary price should match")
			}
		})
	}
}

func TestAuthenticateWebsocket(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	require.NoError(t, ex.AuthenticateWebsocket(t.Context()), "AuthenticateWebsocket must not error")
	assert.Equal(t, "TOKEN", ex.websocketAuthToken(), "AuthenticateWebsocket should retain the token")

	err := newSpotErrorExchange(t).AuthenticateWebsocket(t.Context())
	require.ErrorIs(t, err, errSpotTransport, "AuthenticateWebsocket must surface token request errors")
}

func TestGetAvailableTransferChains(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	chains, err := ex.GetAvailableTransferChains(t.Context(), currency.BTC)
	require.NoError(t, err, "GetAvailableTransferChains must not error")
	assert.Equal(t, []string{"Bitcoin", "SynapsePay (US Wire)"}, chains, "GetAvailableTransferChains should return every deposit method")

	_, err = newSpotErrorExchange(t).GetAvailableTransferChains(t.Context(), currency.BTC)
	require.ErrorIs(t, err, errSpotTransport, "GetAvailableTransferChains must surface request errors")
}

func TestFetchSpotPairInfo(t *testing.T) {
	assetTranslator.l.Lock()
	originalAssets := assetTranslator.Assets
	assetTranslator.Assets = nil
	assetTranslator.l.Unlock()
	t.Cleanup(func() {
		assetTranslator.l.Lock()
		assetTranslator.Assets = originalAssets
		assetTranslator.l.Unlock()
	})

	_, err := newSpotErrorExchange(t).fetchSpotPairInfo(t.Context())
	require.ErrorIs(t, err, errSpotTransport, "fetchSpotPairInfo must surface pair request errors")

	for _, tc := range []struct {
		name          string
		assets        map[string]string
		pair          string
		expectedError string
		expectedLen   int
	}{
		{name: "offline pair", pair: `{"PAIR":{"status":"offline","base":"BASE","quote":"QUOTE"}}`},
		{name: "missing base translation", pair: `{"PAIR":{"status":"online","base":"BASE","quote":"QUOTE"}}`},
		{name: "missing quote translation", assets: map[string]string{"BASE": "BTC"}, pair: `{"PAIR":{"status":"online","base":"BASE","quote":"QUOTE"}}`},
		{name: "invalid translated pair", assets: map[string]string{"BASE": "BAD CODE", "QUOTE": "USD"}, pair: `{"PAIR":{"status":"online","base":"BASE","quote":"QUOTE"}}`, expectedError: "invalid base currency"},
		{name: "online pair", assets: map[string]string{"BASE": "BTC", "QUOTE": "USD"}, pair: `{"PAIR":{"status":"online","base":"BASE","quote":"QUOTE"}}`, expectedLen: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assetTranslator.l.Lock()
			assetTranslator.Assets = tc.assets
			assetTranslator.l.Unlock()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, writeErr := w.Write([]byte(`{"error":[],"result":` + tc.pair + `}`))
				assert.NoError(t, writeErr, "Mock response writing should not error")
			}))
			t.Cleanup(server.Close)
			pairs, err := newAuthenticatedSpotExchange(t, server.URL).fetchSpotPairInfo(t.Context())
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError, "fetchSpotPairInfo must return the expected error")
				return
			}
			require.NoError(t, err, "fetchSpotPairInfo must not error")
			assert.Len(t, pairs, tc.expectedLen, "fetchSpotPairInfo should return the expected pairs")
		})
	}
}

func TestUpdateOrderExecutionLimitsPaths(t *testing.T) {
	assetTranslator.l.Lock()
	originalAssets := assetTranslator.Assets
	assetTranslator.Assets = nil
	assetTranslator.l.Unlock()
	t.Cleanup(func() {
		assetTranslator.l.Lock()
		assetTranslator.Assets = originalAssets
		assetTranslator.l.Unlock()
	})

	err := newSpotErrorExchange(t).UpdateOrderExecutionLimits(t.Context(), asset.Spot)
	require.ErrorIs(t, err, errSpotTransport, "UpdateOrderExecutionLimits must surface asset seeding errors")
	assetTranslator.l.Lock()
	assetTranslator.Assets = nil
	assetTranslator.l.Unlock()
	ex, _ := newSpotEndpointExchange(t)
	require.NoError(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.Spot), "UpdateOrderExecutionLimits must seed and load Spot limits")

	assetTranslator.l.Lock()
	assetTranslator.Assets = map[string]string{"SEEDED": "SEEDED"}
	assetTranslator.l.Unlock()
	err = newSpotErrorExchange(t).UpdateOrderExecutionLimits(t.Context(), asset.Spot)
	require.ErrorContains(t, err, errSpotTransport.Error(), "UpdateOrderExecutionLimits must surface Spot pair errors")
	err = newSpotErrorExchange(t).UpdateOrderExecutionLimits(t.Context(), asset.Futures)
	require.ErrorContains(t, err, errSpotTransport.Error(), "UpdateOrderExecutionLimits must surface Futures instrument errors")

	for _, tc := range []struct {
		name          string
		response      string
		expectedError string
	}{
		{name: "non-tradable instrument", response: `{"instruments":[{"symbol":"PF_XBTUSD","tradeable":false}]}`, expectedError: "no levels supplied"},
		{name: "invalid instrument symbol", response: `{"instruments":[{"symbol":"BAD PAIR","tradeable":true}]}`, expectedError: "invalid quote currency"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "Mock response writing should not error")
			}))
			t.Cleanup(server.Close)
			err := newAuthenticatedFuturesExchange(t, server.URL).UpdateOrderExecutionLimits(t.Context(), asset.Futures)
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError, "UpdateOrderExecutionLimits must return the expected error")
				return
			}
			require.NoError(t, err, "UpdateOrderExecutionLimits must load Futures limits")
		})
	}

	require.ErrorIs(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.Options), asset.ErrNotSupported, "UpdateOrderExecutionLimits must reject unsupported assets")
}

func TestUpdateTickersPaths(t *testing.T) {
	require.ErrorIs(t, newSpotErrorExchange(t).UpdateTickers(t.Context(), asset.Spot), errSpotTransport, "UpdateTickers must surface Spot ticker errors")
	require.ErrorIs(t, newSpotErrorExchange(t).UpdateTickers(t.Context(), asset.Futures), errSpotTransport, "UpdateTickers must surface Futures ticker errors")

	for _, tc := range []struct {
		name          string
		response      string
		assetType     asset.Item
		expectedError error
	}{
		{name: "invalid Spot symbol", response: `{"error":[],"result":{"":{"a":["101","1","2"],"b":["99","1","2"],"c":["100","1"],"v":["10","20"],"l":["90","91"],"h":["110","111"],"o":"95"}}}`, assetType: asset.Spot, expectedError: currency.ErrSymbolStringEmpty},
		{name: "invalid Spot ticker", response: `{"error":[],"result":{"XBTUSD":{"a":["100","1","2"],"b":["100","1","2"],"c":["100","1"],"v":["10","20"],"l":["90","91"],"h":["110","111"],"o":"95"}}}`, assetType: asset.Spot, expectedError: ticker.ErrBidEqualsAsk},
		{name: "invalid Futures ticker", response: `{"tickers":[{"symbol":"PF_XBTUSD","bid":100,"ask":100}]}`, assetType: asset.Futures, expectedError: ticker.ErrBidEqualsAsk},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "Mock response writing should not error")
			}))
			t.Cleanup(server.Close)
			ex := newAuthenticatedSpotExchange(t, server.URL)
			if tc.assetType == asset.Futures {
				ex = newAuthenticatedFuturesExchange(t, server.URL)
			}
			err := ex.UpdateTickers(t.Context(), tc.assetType)
			require.ErrorIs(t, err, tc.expectedError, "UpdateTickers must return the expected processing error")
		})
	}
}

func TestUpdateOrderbookPaths(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	_, err := ex.UpdateOrderbook(t.Context(), currency.EMPTYPAIR, asset.Spot)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "UpdateOrderbook must require a pair")
	_, err = ex.UpdateOrderbook(t.Context(), spotTestPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateOrderbook must require an enabled asset")
	_, err = newSpotErrorExchange(t).UpdateOrderbook(t.Context(), spotTestPair, asset.Spot)
	require.ErrorIs(t, err, errSpotTransport, "UpdateOrderbook must surface Spot depth errors")
	_, err = newSpotErrorExchange(t).UpdateOrderbook(t.Context(), futuresTestPair, asset.Futures)
	require.ErrorIs(t, err, errSpotTransport, "UpdateOrderbook must surface Futures orderbook errors")

	enableTestOptions(t, ex)
	_, err = ex.UpdateOrderbook(t.Context(), currency.NewBTCUSD(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateOrderbook must reject unsupported enabled assets")

	processErrorExchange, _ := newSpotEndpointExchange(t)
	processErrorExchange.Name = ""
	_, err = processErrorExchange.UpdateOrderbook(t.Context(), spotTestPair, asset.Spot)
	require.ErrorIs(t, err, common.ErrExchangeNameNotSet, "UpdateOrderbook must surface orderbook processing errors")
}

func TestUpdateAccountBalancesPaths(t *testing.T) {
	assetTranslator.l.Lock()
	originalAssets := assetTranslator.Assets
	assetTranslator.Assets = nil
	assetTranslator.l.Unlock()
	t.Cleanup(func() {
		assetTranslator.l.Lock()
		assetTranslator.Assets = originalAssets
		assetTranslator.l.Unlock()
	})

	_, err := newSpotErrorExchange(t).UpdateAccountBalances(t.Context(), asset.Spot)
	require.ErrorIs(t, err, errSpotTransport, "UpdateAccountBalances must surface asset seeding errors")
	assetTranslator.l.Lock()
	assetTranslator.Assets = nil
	assetTranslator.l.Unlock()
	ex, _ := newSpotEndpointExchange(t)
	balances, err := ex.UpdateAccountBalances(t.Context(), asset.Spot)
	require.NoError(t, err, "UpdateAccountBalances must seed and load Spot balances")
	require.Len(t, balances, 1, "UpdateAccountBalances must return the Spot account")
	assert.Len(t, balances[0].Balances, 1, "UpdateAccountBalances should skip currencies without a translation")

	assetTranslator.l.Lock()
	assetTranslator.Assets = map[string]string{"SEEDED": "SEEDED"}
	assetTranslator.l.Unlock()
	_, err = newSpotErrorExchange(t).UpdateAccountBalances(t.Context(), asset.Spot)
	require.ErrorIs(t, err, errSpotTransport, "UpdateAccountBalances must surface Spot balance errors")
	_, err = newSpotErrorExchange(t).UpdateAccountBalances(t.Context(), asset.Futures)
	require.ErrorIs(t, err, errSpotTransport, "UpdateAccountBalances must surface Futures balance errors")

	futuresServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"accounts":{"cash":{"balances":{"USD":10,"BTC":2}}}}`))
		assert.NoError(t, writeErr, "Mock response writing should not error")
	}))
	t.Cleanup(futuresServer.Close)
	balances, err = newAuthenticatedFuturesExchange(t, futuresServer.URL).UpdateAccountBalances(t.Context(), asset.Futures)
	require.NoError(t, err, "UpdateAccountBalances must load Futures balances")
	require.Len(t, balances, 1, "UpdateAccountBalances must return every Futures account")
	assert.Len(t, balances[0].Balances, 2, "UpdateAccountBalances should return every Futures balance")

	unauthenticated := new(Exchange)
	require.NoError(t, testexch.Setup(unauthenticated), "Setup must not error")
	_, err = unauthenticated.UpdateAccountBalances(t.Context(), asset.Options)
	require.Error(t, err, "UpdateAccountBalances must surface account persistence errors")
}

func TestGetWithdrawalsHistory(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	history, err := ex.GetWithdrawalsHistory(t.Context(), currency.BTC, asset.Spot)
	require.NoError(t, err, "GetWithdrawalsHistory must not error")
	require.Len(t, history, 1, "GetWithdrawalsHistory must return the decoded withdrawal")
	assert.Equal(t, "REF", history[0].TransferID, "GetWithdrawalsHistory should map the reference identifier")
	assert.Equal(t, 1.0, history[0].Amount, "GetWithdrawalsHistory should map the amount")
	assert.Equal(t, 0.1, history[0].Fee, "GetWithdrawalsHistory should map the fee")

	_, err = newSpotErrorExchange(t).GetWithdrawalsHistory(t.Context(), currency.BTC, asset.Spot)
	require.ErrorIs(t, err, errSpotTransport, "GetWithdrawalsHistory must surface request errors")
}

func TestGetDepositAddressPaths(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	address, err := ex.GetDepositAddress(t.Context(), currency.BTC, "", "Bitcoin")
	require.NoError(t, err, "GetDepositAddress must accept an explicit chain")
	assert.Equal(t, "bc1q", address.Address, "GetDepositAddress should return the address")
	assert.Equal(t, "TAG", address.Tag, "GetDepositAddress should return the tag")

	address, err = ex.GetDepositAddress(t.Context(), currency.BTC, "", "")
	require.NoError(t, err, "GetDepositAddress must discover a chain")
	assert.Equal(t, "bc1q", address.Address, "GetDepositAddress should return the discovered-chain address")

	for _, tc := range []struct {
		name          string
		chain         string
		responses     []string
		expectedError string
		expected      *deposit.Address
	}{
		{name: "deposit method request error", responses: []string{`{"error":["EGeneral:methods"],"result":null}`}, expectedError: "methods"},
		{name: "no deposit methods", responses: []string{`{"error":[],"result":[]}`}, expectedError: "unable to get any deposit methods"},
		{name: "address request error", chain: "Bitcoin", responses: []string{`{"error":["EGeneral:address"],"result":null}`}, expectedError: "address"},
		{name: "retry request error", chain: "Bitcoin", responses: []string{`{"error":[],"result":[]}`, `{"error":["EGeneral:retry"],"result":null}`}, expectedError: "retry"},
		{name: "no addresses", chain: "Bitcoin", responses: []string{`{"error":[],"result":[]}`, `{"error":[],"result":[]}`}, expectedError: "no addresses returned"},
		{name: "retry success", chain: "Bitcoin", responses: []string{`{"error":[],"result":[]}`, `{"error":[],"result":[{"address":"retry-address","tag":"retry-tag"}]}`}, expected: &deposit.Address{Address: "retry-address", Tag: "retry-tag"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				response := tc.responses[call]
				call++
				_, writeErr := w.Write([]byte(response))
				assert.NoError(t, writeErr, "Mock response writing should not error")
			}))
			t.Cleanup(server.Close)
			address, err := newAuthenticatedSpotExchange(t, server.URL).GetDepositAddress(t.Context(), currency.BTC, "", tc.chain)
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError, "GetDepositAddress must return the expected error")
				assert.Nil(t, address, "GetDepositAddress result should be nil on error")
				return
			}
			require.NoError(t, err, "GetDepositAddress must retry with a new address")
			assert.Equal(t, tc.expected, address, "GetDepositAddress should return the retry result")
		})
	}
}

func TestCancelOrderPaths(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	require.Error(t, ex.CancelOrder(t.Context(), &order.Cancel{}), "CancelOrder must validate the request")
	require.NoError(t, ex.CancelOrder(t.Context(), &order.Cancel{AssetType: asset.Spot, OrderID: "ORDER"}), "CancelOrder must cancel a Spot order over REST")
	require.ErrorIs(t, newSpotErrorExchange(t).CancelOrder(t.Context(), &order.Cancel{AssetType: asset.Spot, OrderID: "ORDER"}), errSpotTransport, "CancelOrder must surface Spot REST errors")

	websocketExchange := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	require.NoError(t, websocketExchange.CancelOrder(t.Context(), &order.Cancel{AssetType: asset.Spot, OrderID: "RABBIT"}), "CancelOrder must cancel a Spot order over WebSocket")

	for _, tc := range []struct {
		name          string
		response      string
		expectedError string
	}{
		{name: "success", response: `{}`},
		{name: "error", response: `{"result":"error","error":"cancel failed"}`, expectedError: "cancel failed"},
	} {
		t.Run("Futures/"+tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "Mock response writing should not error")
			}))
			t.Cleanup(server.Close)
			err := newAuthenticatedFuturesExchange(t, server.URL).CancelOrder(t.Context(), &order.Cancel{AssetType: asset.Futures, OrderID: "ORDER"})
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError, "CancelOrder must surface Futures errors")
				return
			}
			require.NoError(t, err, "CancelOrder must cancel a Futures order")
		})
	}

	require.ErrorIs(t, ex.CancelOrder(t.Context(), &order.Cancel{AssetType: asset.Options, OrderID: "ORDER"}), asset.ErrNotSupported, "CancelOrder must reject unsupported assets")
}

func TestCancelBatchOrdersPaths(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	_, err := ex.CancelBatchOrders(t.Context(), []order.Cancel{{}})
	require.Error(t, err, "CancelBatchOrders must validate every request")
	_, err = ex.CancelBatchOrders(t.Context(), []order.Cancel{{AssetType: asset.Spot, OrderID: "ORDER-1"}, {AssetType: asset.Spot, OrderID: "ORDER-2"}})
	require.NoError(t, err, "CancelBatchOrders must cancel orders over REST")
	_, err = newSpotErrorExchange(t).CancelBatchOrders(t.Context(), []order.Cancel{{AssetType: asset.Spot, OrderID: "ORDER-1"}})
	require.ErrorIs(t, err, errSpotTransport, "CancelBatchOrders must surface REST errors")

	websocketExchange := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	_, err = websocketExchange.CancelBatchOrders(t.Context(), []order.Cancel{{AssetType: asset.Spot, OrderID: "RABBIT"}, {AssetType: asset.Spot, OrderID: "SQUIRREL"}})
	require.NoError(t, err, "CancelBatchOrders must cancel orders over WebSocket")
}

func TestSubmitOrder(t *testing.T) {
	_, err := new(Exchange).SubmitOrder(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrSubmissionIsNil, "SubmitOrder must reject a nil submission")

	wsExchange := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	wsSubmit := validSubmit(wsExchange, asset.Spot)
	wsSubmit.Price = 80000
	response, err := wsExchange.SubmitOrder(t.Context(), wsSubmit)
	require.NoError(t, err, "SubmitOrder must place a Spot order over WebSocket v2")
	assert.Equal(t, "ONPNXH-KMKMU-F4MR5V", response.OrderID, "response.OrderID should match")

	invalidTrigger := validSubmit(wsExchange, asset.Spot)
	invalidTrigger.Type = order.Stop
	_, err = wsExchange.SubmitOrder(t.Context(), invalidTrigger)
	require.ErrorIs(t, err, errTriggerPriceNotSet, "SubmitOrder must return WebSocket parameter errors")

	sendErr := errors.New("send failed")
	wsExchange.Websocket.AuthConn = &mockAuthSubConnection{err: sendErr}
	_, err = wsExchange.SubmitOrder(t.Context(), validSubmit(wsExchange, asset.Spot))
	require.ErrorIs(t, err, sendErr, "SubmitOrder must return WebSocket request errors")

	leveragedEndTime := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	leverageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/0/private/AddOrder", r.URL.Path, "SubmitOrder should use the current Spot REST path for leverage")
		body, readErr := io.ReadAll(r.Body)
		if !assert.NoError(t, readErr, "SubmitOrder request body should be readable for leverage") {
			return
		}
		form, parseErr := url.ParseQuery(string(body))
		if !assert.NoError(t, parseErr, "SubmitOrder request form should parse for leverage") {
			return
		}
		assert.Equal(t, "2", form.Get("leverage"), "SubmitOrder should preserve the requested leverage ratio")
		assert.Equal(t, "GTD", form.Get("timeinforce"), "SubmitOrder should preserve leveraged good-till-date")
		assert.Equal(t, "post", form.Get("oflags"), "SubmitOrder should preserve leveraged post-only")
		assert.Equal(t, "true", form.Get("reduce_only"), "SubmitOrder should preserve leveraged reduce-only")
		assert.Equal(t, strconv.FormatInt(leveragedEndTime.Unix(), 10), form.Get("expiretm"), "SubmitOrder should preserve leveraged expiry")
		_, writeErr := w.Write([]byte(`{"error":[],"result":{"txid":["SPOT-LEVERAGE"]}}`))
		assert.NoError(t, writeErr, "SubmitOrder response writing should not error for leverage")
	}))
	defer leverageServer.Close()
	wsExchange.API.AuthenticatedSupport = true
	wsExchange.SetCredentials(&accounts.Credentials{
		Key:    "test-key",
		Secret: base64.StdEncoding.EncodeToString([]byte("test-secret")),
	})
	require.NoError(t, wsExchange.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), leverageServer.URL), "SetRunningURL must set the leveraged Spot endpoint")
	leveragedSubmit := validSubmit(wsExchange, asset.Spot)
	leveragedSubmit.Leverage = 2
	leveragedSubmit.TimeInForce = order.GoodTillTime | order.PostOnly
	leveragedSubmit.ReduceOnly = true
	leveragedSubmit.EndTime = leveragedEndTime
	response, err = wsExchange.SubmitOrder(t.Context(), leveragedSubmit)
	require.NoError(t, err, "SubmitOrder must preserve numeric leverage through Spot REST")
	assert.Equal(t, "SPOT-LEVERAGE", response.OrderID, "response.OrderID should match for leverage")

	for _, tc := range []struct {
		name           string
		response       string
		orderType      order.Type
		timeInForce    order.TimeInForce
		endTime        time.Time
		expectedParam  string
		expectedExpiry string
		expectedStatus order.Status
		errIs          error
		errContains    string
	}{
		{
			name:           "GoodTillDay",
			response:       `{"error":[],"result":{"txid":["SPOT-GTD"]}}`,
			orderType:      order.Limit,
			timeInForce:    order.GoodTillDay,
			endTime:        leveragedEndTime,
			expectedParam:  "GTD",
			expectedExpiry: strconv.FormatInt(leveragedEndTime.Unix(), 10),
			expectedStatus: order.New,
		},
		{
			name:           "FillOrKill",
			response:       `{"error":[],"result":{"txid":["SPOT-FOK"]}}`,
			orderType:      order.Limit,
			timeInForce:    order.FillOrKill,
			expectedParam:  "FOK",
			expectedStatus: order.New,
		},
		{
			name:           "ImmediateOrCancel",
			response:       `{"error":[],"result":{"txid":["SPOT-IOC"]}}`,
			orderType:      order.Limit,
			timeInForce:    order.ImmediateOrCancel,
			expectedParam:  "IOC",
			expectedStatus: order.New,
		},
		{
			name:        "InvalidFillOrKillMarket",
			orderType:   order.Market,
			timeInForce: order.FillOrKill,
			errIs:       order.ErrUnsupportedTimeInForce,
		},
		{
			name:           "Market",
			response:       `{"error":[],"result":{"txid":["SPOT-MARKET"]}}`,
			orderType:      order.Market,
			expectedStatus: order.Filled,
		},
		{
			name:           "NoOrderID",
			response:       `{"error":[],"result":{"txid":[]}}`,
			orderType:      order.Limit,
			errIs:          order.ErrOrderIDNotSet,
			expectedStatus: order.UnknownStatus,
		},
		{
			name:        "APIError",
			response:    `{"error":["EOrder:Rejected"],"result":{}}`,
			orderType:   order.Limit,
			errContains: "EOrder:Rejected",
		},
	} {
		t.Run("Spot/"+tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/0/private/AddOrder", r.URL.Path, "SubmitOrder should use the current Spot REST order path")
				body, readErr := io.ReadAll(r.Body)
				if !assert.NoError(t, readErr, "SubmitOrder body should be readable") {
					return
				}
				form, parseErr := url.ParseQuery(string(body))
				if !assert.NoError(t, parseErr, "SubmitOrder form should parse") {
					return
				}
				assert.Equal(t, tc.expectedParam, form.Get("timeinforce"), "SubmitOrder time in force should match")
				assert.Equal(t, tc.expectedExpiry, form.Get("expiretm"), "SubmitOrder expiry should match")
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Spot SubmitOrder response")
			}))
			defer server.Close()
			ex := newAuthenticatedSpotExchange(t, server.URL)
			submit := validSubmit(ex, asset.Spot)
			submit.Type = tc.orderType
			submit.TimeInForce = tc.timeInForce
			submit.EndTime = tc.endTime
			if tc.orderType == order.Market {
				submit.Price = 0
			}

			result, submitErr := ex.SubmitOrder(t.Context(), submit)
			if tc.errIs != nil {
				require.ErrorIs(t, submitErr, tc.errIs, "SubmitOrder must return the expected sentinel error")
				return
			}
			if tc.errContains != "" {
				require.ErrorContains(t, submitErr, tc.errContains, "SubmitOrder must return the expected API error")
				require.ErrorIs(t, submitErr, request.ErrAuthRequestFailed, "SubmitOrder must wrap Spot API errors as authenticated request failures")
				return
			}
			require.NoError(t, submitErr, "SubmitOrder must place the Spot REST order")
			assert.Equal(t, tc.expectedStatus, result.Status, "result.Status should match")
		})
	}

	for _, tc := range []struct {
		name        string
		response    string
		errContains string
		expectedID  string
	}{
		{name: "Placed", response: `{"sendStatus":{"status":"placed","order_id":"FUTURES-1"}}`, expectedID: "FUTURES-1"},
		{name: "NotPlaced", response: `{"sendStatus":{"status":"rejected"}}`, errContains: "submit order failed: rejected"},
		{name: "APIError", response: `{"result":"error","error":"authenticationError"}`, errContains: "authenticationError"},
	} {
		t.Run("Futures/"+tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/derivatives/api/v3/sendorder", r.URL.Path, "SubmitOrder should use Futures REST v3")
				assert.Equal(t, "lmt", r.URL.Query().Get("orderType"), "SubmitOrder should use the Futures limit order type")
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Futures SubmitOrder response")
			}))
			defer server.Close()
			ex := newAuthenticatedFuturesExchange(t, server.URL)

			result, submitErr := ex.SubmitOrder(t.Context(), validSubmit(ex, asset.Futures))
			if tc.errContains != "" {
				require.ErrorContains(t, submitErr, tc.errContains, "SubmitOrder must return the expected Futures error")
				return
			}
			require.NoError(t, submitErr, "SubmitOrder must place the Futures order")
			assert.Equal(t, tc.expectedID, result.OrderID, "result.OrderID should match for Futures")
		})
	}

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	enableTestOptions(t, ex)
	unsupported := validSubmit(ex, asset.Spot)
	unsupported.AssetType = asset.Options
	_, err = ex.SubmitOrder(t.Context(), unsupported)
	require.ErrorIs(t, err, asset.ErrNotSupported, "SubmitOrder must reject unsupported assets")
}

func TestGetActiveOrders(t *testing.T) {
	validRequest := func(a asset.Item) *order.MultiOrderRequest {
		return &order.MultiOrderRequest{
			Type:      order.AnyType,
			AssetType: a,
			Side:      order.AnySide,
		}
	}
	newSpot := func(t *testing.T, response string) *Exchange {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/0/private/OpenOrders", r.URL.Path, "GetActiveOrders Spot request path should match")
			_, err := w.Write([]byte(response))
			assert.NoError(t, err, "GetActiveOrders Spot response writing should not error")
		}))
		t.Cleanup(server.Close)
		ex := newAuthenticatedSpotExchange(t, server.URL)
		require.NoError(t, ex.SetAssetPairStore(asset.Spot, currency.PairStore{
			AssetEnabled:  true,
			Enabled:       currency.Pairs{spotTestPair},
			Available:     currency.Pairs{spotTestPair},
			RequestFormat: &currency.PairFormat{Uppercase: true},
			ConfigFormat:  &currency.PairFormat{Uppercase: true, Delimiter: currency.UnderscoreDelimiter},
		}), "SetAssetPairStore must configure Spot pairs")
		return ex
	}
	newFutures := func(t *testing.T, response string) *Exchange {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/derivatives/api/v3/openorders", r.URL.Path, "GetActiveOrders Futures request path should match")
			_, err := w.Write([]byte(response))
			assert.NoError(t, err, "GetActiveOrders Futures response writing should not error")
		}))
		t.Cleanup(server.Close)
		ex := newAuthenticatedFuturesExchange(t, server.URL)
		require.NoError(t, ex.SetAssetPairStore(asset.Futures, currency.PairStore{
			AssetEnabled:  true,
			Enabled:       currency.Pairs{futuresTestPair},
			Available:     currency.Pairs{futuresTestPair},
			RequestFormat: &currency.PairFormat{Uppercase: true, Delimiter: currency.UnderscoreDelimiter},
			ConfigFormat:  &currency.PairFormat{Uppercase: true, Delimiter: currency.UnderscoreDelimiter},
		}), "SetAssetPairStore must configure Futures pairs")
		return ex
	}
	t.Run("NilRequest", func(t *testing.T) {
		result, err := new(Exchange).GetActiveOrders(t.Context(), nil)
		require.ErrorIs(t, err, order.ErrGetOrdersRequestIsNil, "GetActiveOrders must reject a nil request")
		assert.Nil(t, result, "GetActiveOrders should not return orders for a nil request")
	})

	t.Run("InvalidAsset", func(t *testing.T) {
		result, err := new(Exchange).GetActiveOrders(t.Context(), validRequest(asset.Empty))
		require.ErrorIs(t, err, asset.ErrNotSupported, "GetActiveOrders must reject an invalid asset")
		assert.Nil(t, result, "GetActiveOrders should not return orders for an invalid asset")
	})

	t.Run("InvalidSide", func(t *testing.T) {
		req := validRequest(asset.Spot)
		req.Side = order.UnknownSide
		result, err := new(Exchange).GetActiveOrders(t.Context(), req)
		require.ErrorIs(t, err, order.ErrSideIsInvalid, "GetActiveOrders must reject an invalid side")
		assert.Nil(t, result, "GetActiveOrders should not return orders for an invalid side")
	})

	t.Run("InvalidType", func(t *testing.T) {
		req := validRequest(asset.Spot)
		req.Type = order.UnknownType
		result, err := new(Exchange).GetActiveOrders(t.Context(), req)
		require.ErrorIs(t, err, order.ErrUnrecognisedOrderType, "GetActiveOrders must reject an invalid type")
		assert.Nil(t, result, "GetActiveOrders should not return orders for an invalid type")
	})

	t.Run("UnsupportedAsset", func(t *testing.T) {
		result, err := new(Exchange).GetActiveOrders(t.Context(), validRequest(asset.Options))
		require.ErrorIs(t, err, asset.ErrNotSupported, "GetActiveOrders must reject an unsupported asset")
		assert.Nil(t, result, "GetActiveOrders should not return orders for an unsupported asset")
	})

	t.Run("SpotAPIError", func(t *testing.T) {
		ex := newSpot(t, `{"error":["EGeneral:Temporary lockout"],"result":{}}`)
		result, err := ex.GetActiveOrders(t.Context(), validRequest(asset.Spot))
		require.ErrorContains(t, err, "Temporary lockout", "GetActiveOrders must return the Spot API error")
		assert.Nil(t, result, "GetActiveOrders should not return Spot orders after an API error")
	})

	t.Run("SpotPairsError", func(t *testing.T) {
		ex := newSpot(t, `{"error":[],"result":{"open":{}}}`)
		ex.CurrencyPairs.Delete(asset.Spot)
		result, err := ex.GetActiveOrders(t.Context(), validRequest(asset.Spot))
		require.Error(t, err, "GetActiveOrders must return a missing Spot pair-store error")
		assert.Nil(t, result, "GetActiveOrders should not return Spot orders without a pair store")
	})

	t.Run("SpotRequestFormatError", func(t *testing.T) {
		ex := newSpot(t, `{"error":[],"result":{"open":{}}}`)
		ex.CurrencyPairs.Pairs[asset.Spot].RequestFormat = nil
		result, err := ex.GetActiveOrders(t.Context(), validRequest(asset.Spot))
		require.ErrorIs(t, err, currency.ErrPairFormatIsNil, "GetActiveOrders must return a missing Spot request format error")
		assert.Nil(t, result, "GetActiveOrders should not return Spot orders without a request format")
	})

	t.Run("SpotPairError", func(t *testing.T) {
		ex := newSpot(t, `{"error":[],"result":{"open":{"ORDER-1":{"descr":{"pair":"","type":"buy","ordertype":"limit","price":"100"},"vol":"2","vol_exec":"1"}}}}`)
		result, err := ex.GetActiveOrders(t.Context(), validRequest(asset.Spot))
		require.Error(t, err, "GetActiveOrders must reject an invalid Spot order pair")
		assert.Nil(t, result, "GetActiveOrders should not return an order with an invalid pair")
	})

	t.Run("SpotSideError", func(t *testing.T) {
		ex := newSpot(t, `{"error":[],"result":{"open":{"ORDER-1":{"descr":{"pair":"XBTUSD","type":"hold","ordertype":"limit","price":"100"},"vol":"2","vol_exec":"1"}}}}`)
		result, err := ex.GetActiveOrders(t.Context(), validRequest(asset.Spot))
		require.Error(t, err, "GetActiveOrders must reject an invalid Spot order side")
		assert.Nil(t, result, "GetActiveOrders should not return an order with an invalid side")
	})

	t.Run("SpotOrders", func(t *testing.T) {
		ex := newSpot(t, `{"error":[],"result":{"open":{
			"ORDER-1":{"descr":{"pair":"XBTUSD","type":"buy","ordertype":"limit","price":"100"},"opentm":1700000000,"vol":"2","vol_exec":"0.5"},
			"ORDER-2":{"descr":{"pair":"XBTUSD","type":"sell","ordertype":"unexpected","price":"110"},"opentm":1700000001,"vol":"3","vol_exec":"1"}
		}}}`)
		req := validRequest(asset.Spot)
		req.Pairs = currency.Pairs{spotTestPair}
		result, err := ex.GetActiveOrders(t.Context(), req)
		require.NoError(t, err, "GetActiveOrders must translate valid Spot orders")
		require.Len(t, result, 2, "GetActiveOrders must return both Spot orders")
		foundUnknownType := false
		for i := range result {
			assert.Equal(t, asset.Spot, result[i].AssetType, "result[i].AssetType should match Spot")
			assert.Equal(t, order.Open, result[i].Status, "result[i].Status should be open")
			if result[i].OrderID == "ORDER-2" {
				foundUnknownType = result[i].Type == order.UnknownType
			}
		}
		assert.True(t, foundUnknownType, "GetActiveOrders should retain a Spot order with an unknown server type")
	})

	t.Run("FuturesPairsError", func(t *testing.T) {
		ex := newFutures(t, `{"result":"success","openOrders":[]}`)
		ex.CurrencyPairs.Delete(asset.Futures)
		result, err := ex.GetActiveOrders(t.Context(), validRequest(asset.Futures))
		require.Error(t, err, "GetActiveOrders must return a missing Futures pair-store error")
		assert.Empty(t, result, "GetActiveOrders should not return Futures orders without a pair store")
	})

	t.Run("FuturesAPIError", func(t *testing.T) {
		ex := newFutures(t, `{"result":"error","error":"authenticationError"}`)
		req := validRequest(asset.Futures)
		req.Pairs = currency.Pairs{futuresTestPair}
		result, err := ex.GetActiveOrders(t.Context(), req)
		require.ErrorContains(t, err, "authenticationError", "GetActiveOrders must return the Futures API error")
		assert.Empty(t, result, "GetActiveOrders should not return Futures orders after an API error")
	})

	t.Run("FuturesPairFormatError", func(t *testing.T) {
		ex := newFutures(t, `{"result":"success","openOrders":[]}`)
		req := validRequest(asset.Futures)
		req.Pairs = currency.Pairs{currency.EMPTYPAIR}
		result, err := ex.GetActiveOrders(t.Context(), req)
		require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetActiveOrders must reject an empty Futures pair")
		assert.Empty(t, result, "GetActiveOrders should not return orders for an empty Futures pair")
	})

	t.Run("FuturesSymbolMismatch", func(t *testing.T) {
		ex := newFutures(t, `{"result":"success","openOrders":[{"order_id":"ORDER-1","symbol":"PF_ETHUSD","side":"buy","orderType":"lmt"}]}`)
		req := validRequest(asset.Futures)
		req.Pairs = currency.Pairs{futuresTestPair}
		result, err := ex.GetActiveOrders(t.Context(), req)
		require.NoError(t, err, "GetActiveOrders must ignore a Futures order for another symbol")
		assert.Empty(t, result, "GetActiveOrders should not return a Futures order for another symbol")
	})

	t.Run("FuturesSideError", func(t *testing.T) {
		ex := newFutures(t, `{"result":"success","openOrders":[{"order_id":"ORDER-1","symbol":"PF_XBTUSD","side":"hold","orderType":"lmt"}]}`)
		req := validRequest(asset.Futures)
		req.Pairs = currency.Pairs{futuresTestPair}
		result, err := ex.GetActiveOrders(t.Context(), req)
		require.Error(t, err, "GetActiveOrders must reject an invalid Futures order side")
		assert.Empty(t, result, "GetActiveOrders should not return a Futures order with an invalid side")
	})

	t.Run("FuturesTypeError", func(t *testing.T) {
		ex := newFutures(t, `{"result":"success","openOrders":[{"order_id":"ORDER-1","symbol":"PF_XBTUSD","side":"buy","orderType":"unexpected"}]}`)
		req := validRequest(asset.Futures)
		req.Pairs = currency.Pairs{futuresTestPair}
		result, err := ex.GetActiveOrders(t.Context(), req)
		require.Error(t, err, "GetActiveOrders must reject an invalid Futures order type")
		assert.Empty(t, result, "GetActiveOrders should not return a Futures order with an invalid type")
	})

	t.Run("FuturesOrders", func(t *testing.T) {
		ex := newFutures(t, `{"result":"success","openOrders":[{"order_id":"ORDER-1","symbol":"PF_XBTUSD","side":"buy","orderType":"lmt","limitPrice":50000,"filledSize":0.5,"receivedTime":"2026-07-29T01:02:03Z"}]}`)
		result, err := ex.GetActiveOrders(t.Context(), validRequest(asset.Futures))
		require.NoError(t, err, "GetActiveOrders must translate valid Futures orders")
		require.Len(t, result, 1, "GetActiveOrders must return the Futures order")
		assert.Equal(t, "ORDER-1", result[0].OrderID, "result[0].OrderID should match")
		assert.Equal(t, order.Buy, result[0].Side, "result[0].Side should match")
		assert.Equal(t, order.Limit, result[0].Type, "result[0].Type should match")
		assert.Equal(t, asset.Futures, result[0].AssetType, "result[0].AssetType should match Futures")
		assert.Equal(t, order.Open, result[0].Status, "result[0].Status should be open")
	})
}

func TestCancelAllOrders(t *testing.T) {
	_, err := new(Exchange).CancelAllOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrCancelOrderIsNil, "CancelAllOrders must reject a nil request")

	wsExchange := testexch.MockWsInstance[Exchange](t, curryWsMockUpgrader(t, mockWsServer))
	result, err := wsExchange.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Spot})
	require.NoError(t, err, "CancelAllOrders must cancel Spot orders over WebSocket v2")
	assert.Len(t, result.Status, 3, "CancelAllOrders should report the WebSocket cancellation count")

	sendErr := errors.New("send failed")
	wsExchange.Websocket.AuthConn = &mockAuthSubConnection{err: sendErr}
	_, err = wsExchange.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Spot})
	require.ErrorIs(t, err, sendErr, "CancelAllOrders must return WebSocket errors")

	t.Run("Spot REST open orders error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, writeErr := w.Write([]byte(`{"error":["EGeneral:Temporary lockout"],"result":{}}`))
			assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Spot error response")
		}))
		defer server.Close()
		ex := newAuthenticatedSpotExchange(t, server.URL)
		_, cancelErr := ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Spot})
		require.ErrorContains(t, cancelErr, "Temporary lockout", "CancelAllOrders must return the open-orders error")
	})

	t.Run("Spot REST success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/0/private/CancelAll", r.URL.Path, "CancelAllOrders should use the current Spot cancel-all endpoint")
			_, writeErr := w.Write([]byte(`{"error":[],"result":{"count":2,"pending":false}}`))
			assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Spot cancellation response")
		}))
		defer server.Close()
		ex := newAuthenticatedSpotExchange(t, server.URL)
		result, cancelErr := ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Spot})
		require.NoError(t, cancelErr, "CancelAllOrders must not error for a successful Spot cancellation")
		assert.Equal(t, "cancelled", result.Status["Unknown:1"], "CancelAllOrders should report the first Spot cancellation")
		assert.Equal(t, "cancelled", result.Status["Unknown:2"], "CancelAllOrders should report the second Spot cancellation")
	})

	for _, tc := range []struct {
		name        string
		response    string
		errContains string
		expected    int
	}{
		{name: "Success", response: `{"cancelStatus":{"cancelledOrders":[{"order_id":"FUTURES-1"},{"order_id":"FUTURES-2"}]}}`, expected: 2},
		{name: "APIError", response: `{"result":"error","error":"authenticationError"}`, errContains: "authenticationError"},
	} {
		t.Run("Futures/"+tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/derivatives/api/v3/cancelallorders", r.URL.Path, "CancelAllOrders should use Futures REST v3")
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Futures CancelAllOrders response")
			}))
			defer server.Close()
			ex := newAuthenticatedFuturesExchange(t, server.URL)
			result, cancelErr := ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Futures})
			if tc.errContains != "" {
				require.ErrorContains(t, cancelErr, tc.errContains, "CancelAllOrders must return the Futures error")
				return
			}
			require.NoError(t, cancelErr, "CancelAllOrders must cancel Futures orders")
			assert.Len(t, result.Status, tc.expected, "CancelAllOrders should report each Futures cancellation")
		})
	}
}

func spotOrderResponse(pair, side, orderType, status string) string {
	return fmt.Sprintf(`{
		"error":[],
		"result":{
			"ORDER-1":{
				"status":%q,
				"opentm":1710000000,
				"closetm":1710000001,
				"descr":{"pair":%q,"type":%q,"ordertype":%q,"price":"101"},
				"vol":"2",
				"vol_exec":"1",
				"cost":"100",
				"fee":"0.1",
				"price":"100",
				"trades":["TRADE-1"]
			}
		}
	}`, status, pair, side, orderType)
}

func spotOrderHistoryResponse(pair, side, orderType, status string) string {
	return fmt.Sprintf(`{
		"error":[],
		"result":{
			"closed":{
				"ORDER-1":{
					"status":%q,
					"opentm":1710000000,
					"closetm":1710000001,
					"descr":{"pair":%q,"type":%q,"ordertype":%q,"price":"101"},
					"vol":"2",
					"vol_exec":"1",
					"cost":"100",
					"fee":"0.1",
					"price":"100",
					"trades":["TRADE-1"]
				}
			}
		}
	}`, status, pair, side, orderType)
}

func futuresFillsResponse(orderID, symbol, side, fillType string) string {
	return fmt.Sprintf(`{
		"fills":[{
			"fill_id":"FILL-1",
			"symbol":%q,
			"side":%q,
			"order_id":%q,
			"size":1,
			"price":50000,
			"fillTime":"2026-07-29T01:02:03Z",
			"fillType":%q
		}]
	}`, symbol, side, orderID, fillType)
}

func futuresRecentOrderResponse(event, side, orderType string) string {
	recentOrder := fmt.Sprintf(`{
		"uid":"ORDER-1",
		"accountId":"ACCOUNT-1",
		"tradeable":"PF_XBTUSD",
		"direction":%q,
		"quantity":"2",
		"filled":"1",
		"timestamp":"2026-07-29T01:02:03Z",
		"limitPrice":"50000",
		"orderType":%q,
		"clientId":"CLIENT-1"
	}`, side, orderType)

	var eventData string
	switch event {
	case "execution":
		eventData = fmt.Sprintf(`{"executionEvent":{"execution":{"uid":"EXECUTION-1","takerOrder":%s}}}`, recentOrder)
	case "rejected":
		eventData = fmt.Sprintf(`{"orderRejected":{"order":%s}}`, recentOrder)
	case "cancelled":
		eventData = fmt.Sprintf(`{"orderCancelled":{"order":%s}}`, recentOrder)
	case "placed":
		eventData = fmt.Sprintf(`{"orderPlaced":{"order":%s}}`, recentOrder)
	default:
		eventData = `{}`
	}
	return fmt.Sprintf(`{"orderEvents":[{"event":%s}]}`, eventData)
}

func TestGetOrderInfo(t *testing.T) {
	_, err := new(Exchange).GetOrderInfo(t.Context(), "ORDER-1", currency.EMPTYPAIR, asset.Spot)
	require.Error(t, err, "GetOrderInfo must reject a disabled asset")

	for _, tc := range []struct {
		name        string
		response    string
		prepare     func(*Exchange)
		errContains string
		expected    *order.Detail
	}{
		{
			name:        "APIError",
			response:    `{"error":["EOrder:Unknown order"],"result":{}}`,
			errContains: "EOrder:Unknown order",
		},
		{
			name:        "NotFound",
			response:    `{"error":[],"result":{}}`,
			errContains: "order ORDER-1 not found in response",
		},
		{
			name:     "AvailablePairFormatError",
			response: spotOrderResponse("XBTUSD", "buy", "limit", "open"),
			prepare: func(ex *Exchange) {
				ex.CurrencyPairs.Pairs[asset.Spot].ConfigFormat = nil
			},
			errContains: "pair format is nil",
		},
		{
			name:     "RequestPairFormatError",
			response: spotOrderResponse("XBTUSD", "buy", "limit", "open"),
			prepare: func(ex *Exchange) {
				ex.CurrencyPairs.Pairs[asset.Spot].RequestFormat = nil
			},
			errContains: "pair format is nil",
		},
		{
			name:        "InvalidSide",
			response:    spotOrderResponse("XBTUSD", "hold", "limit", "open"),
			errContains: "side",
		},
		{
			name:        "InvalidOrderType",
			response:    spotOrderResponse("XBTUSD", "buy", "unknown", "open"),
			errContains: "unrecognised order type",
		},
		{
			name:        "InvalidPair",
			response:    spotOrderResponse("", "buy", "limit", "open"),
			errContains: "string too short",
		},
		{
			name:     "UnknownStatus",
			response: spotOrderResponse("XBTUSD", "buy", "limit", "unknown"),
		},
		{
			name:     "Open",
			response: spotOrderResponse("XBTUSD", "buy", "limit", "open"),
			expected: &order.Detail{
				OrderID:         "ORDER-1",
				Side:            order.Buy,
				Type:            order.Limit,
				Status:          order.Open,
				Price:           101,
				Amount:          2,
				ExecutedAmount:  1,
				RemainingAmount: 1,
				Fee:             0.1,
				Cost:            100,
				AssetType:       asset.Spot,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/0/private/QueryOrders", r.URL.Path, "GetOrderInfo should use the current Spot REST path")
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Spot GetOrderInfo response")
			}))
			defer server.Close()
			ex := newAuthenticatedSpotExchange(t, server.URL)
			if tc.prepare != nil {
				tc.prepare(ex)
			}

			result, getErr := ex.GetOrderInfo(t.Context(), "ORDER-1", currency.EMPTYPAIR, asset.Spot)
			if tc.errContains != "" {
				require.ErrorContains(t, getErr, tc.errContains, "GetOrderInfo must return the expected Spot error")
				return
			}
			require.NoError(t, getErr, "GetOrderInfo must parse the Spot response")
			require.NotNil(t, result, "GetOrderInfo must return an order")
			if tc.expected != nil {
				assert.Equal(t, tc.expected.OrderID, result.OrderID, "result.OrderID should match")
				assert.Equal(t, tc.expected.Side, result.Side, "result.Side should match")
				assert.Equal(t, tc.expected.Type, result.Type, "result.Type should match")
				assert.Equal(t, tc.expected.Status, result.Status, "result.Status should match")
				assert.Equal(t, tc.expected.Price, result.Price, "result.Price should use the Spot order description price")
				assert.Equal(t, tc.expected.Amount, result.Amount, "result.Amount should match")
				assert.Equal(t, tc.expected.ExecutedAmount, result.ExecutedAmount, "result.ExecutedAmount should match")
				assert.Equal(t, tc.expected.RemainingAmount, result.RemainingAmount, "result.RemainingAmount should match")
				assert.Equal(t, tc.expected.Fee, result.Fee, "result.Fee should match")
				assert.Equal(t, tc.expected.Cost, result.Cost, "result.Cost should match")
				assert.Equal(t, tc.expected.AssetType, result.AssetType, "result.AssetType should match")
				require.Len(t, result.Trades, 1, "result.Trades must be retained")
				assert.Equal(t, "TRADE-1", result.Trades[0].TID, "result.Trades[0].TID should match")
			}
		})
	}

	for _, tc := range []struct {
		name        string
		response    string
		errContains string
		expectedID  string
	}{
		{name: "APIError", response: `{"result":"error","error":"authenticationError"}`, errContains: "authenticationError"},
		{name: "MismatchedOrder", response: futuresFillsResponse("OTHER", "PF_XBTUSD", "buy", "maker")},
		{name: "InvalidSymbol", response: futuresFillsResponse("ORDER-1", "", "buy", "maker"), errContains: "string too short"},
		{name: "InvalidSide", response: futuresFillsResponse("ORDER-1", "PF_XBTUSD", "hold", "maker"), errContains: "invalid side"},
		{name: "InvalidFillType", response: futuresFillsResponse("ORDER-1", "PF_XBTUSD", "buy", "unknown"), errContains: "invalid orderPriceType"},
		{name: "Success", response: futuresFillsResponse("ORDER-1", "PF_XBTUSD", "buy", "maker"), expectedID: "ORDER-1"},
	} {
		t.Run("Futures/"+tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/derivatives/api/v3/fills", r.URL.Path, "GetOrderInfo should use Futures REST v3 fills")
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Futures GetOrderInfo response")
			}))
			defer server.Close()
			ex := newAuthenticatedFuturesExchange(t, server.URL)

			result, getErr := ex.GetOrderInfo(t.Context(), "ORDER-1", currency.EMPTYPAIR, asset.Futures)
			if tc.errContains != "" {
				require.ErrorContains(t, getErr, tc.errContains, "GetOrderInfo must return the expected Futures error")
				return
			}
			require.NoError(t, getErr, "GetOrderInfo must parse the Futures response")
			require.NotNil(t, result, "GetOrderInfo must return a Futures order detail")
			assert.Equal(t, tc.expectedID, result.OrderID, "result.OrderID should match for Futures")
			if tc.expectedID != "" {
				assert.Equal(t, order.Buy, result.Side, "result.Side should match for Futures")
				assert.Equal(t, order.Limit, result.Type, "result.Type should match for Futures")
				assert.Equal(t, asset.Futures, result.AssetType, "result.AssetType should match Futures")
			}
		})
	}

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	enableTestOptions(t, ex)
	_, err = ex.GetOrderInfo(t.Context(), "ORDER-1", currency.EMPTYPAIR, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetOrderInfo must reject unsupported assets")
}

func TestGetOrderHistory(t *testing.T) {
	_, err := new(Exchange).GetOrderHistory(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrGetOrdersRequestIsNil, "GetOrderHistory must reject a nil request")

	for _, tc := range []struct {
		name        string
		response    string
		prepare     func(*Exchange)
		start       time.Time
		end         time.Time
		expected    int
		errContains string
	}{
		{
			name:        "APIError",
			response:    `{"error":["EGeneral:Temporary lockout"],"result":{}}`,
			errContains: "Temporary lockout",
		},
		{
			name:     "AvailablePairFormatError",
			response: spotOrderHistoryResponse("XBTUSD", "buy", "limit", "closed"),
			prepare: func(ex *Exchange) {
				ex.CurrencyPairs.Pairs[asset.Spot].ConfigFormat = nil
			},
			errContains: "pair format is nil",
		},
		{
			name:     "RequestPairFormatError",
			response: spotOrderHistoryResponse("XBTUSD", "buy", "limit", "closed"),
			prepare: func(ex *Exchange) {
				ex.CurrencyPairs.Pairs[asset.Spot].RequestFormat = nil
			},
			errContains: "pair format is nil",
		},
		{
			name:        "InvalidPair",
			response:    spotOrderHistoryResponse("", "buy", "limit", "closed"),
			errContains: "string too short",
		},
		{
			name:     "UnknownEnums",
			response: spotOrderHistoryResponse("XBTUSD", "hold", "unknown", "unknown"),
			expected: 1,
		},
		{
			name:     "SuccessWithRange",
			response: spotOrderHistoryResponse("XBTUSD", "sell", "limit", "closed"),
			start:    time.Unix(1709999999, 0),
			end:      time.Unix(1710000002, 0),
			expected: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/0/private/ClosedOrders", r.URL.Path, "GetOrderHistory should use the current Spot REST path")
				body, readErr := io.ReadAll(r.Body)
				if !assert.NoError(t, readErr, "io.ReadAll should read the GetOrderHistory body") {
					return
				}
				form, parseErr := url.ParseQuery(string(body))
				if !assert.NoError(t, parseErr, "url.ParseQuery should parse the GetOrderHistory form") {
					return
				}
				if !tc.start.IsZero() {
					assert.Equal(t, strconv.FormatInt(tc.start.Unix(), 10), form.Get("start"), "GetOrderHistory start time should use seconds")
				}
				if !tc.end.IsZero() {
					assert.Equal(t, strconv.FormatInt(tc.end.Unix(), 10), form.Get("end"), "GetOrderHistory end time should use seconds")
				}
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Spot GetOrderHistory response")
			}))
			defer server.Close()
			ex := newAuthenticatedSpotExchange(t, server.URL)
			if tc.prepare != nil {
				tc.prepare(ex)
			}

			result, historyErr := ex.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
				AssetType: asset.Spot,
				Type:      order.AnyType,
				Side:      order.AnySide,
				StartTime: tc.start,
				EndTime:   tc.end,
			})
			if tc.errContains != "" {
				require.ErrorContains(t, historyErr, tc.errContains, "GetOrderHistory must return the expected Spot error")
				return
			}
			require.NoError(t, historyErr, "GetOrderHistory must parse the Spot response")
			assert.Len(t, result, tc.expected, "GetOrderHistory should return the expected Spot orders")
		})
	}

	t.Run("FuturesPairsError", func(t *testing.T) {
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		ex.CurrencyPairs.Delete(asset.Futures)
		result, historyErr := ex.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
			AssetType: asset.Futures,
			Type:      order.AnyType,
			Side:      order.AnySide,
		})
		require.Error(t, historyErr, "GetOrderHistory must return a missing Futures pair-store error")
		assert.Empty(t, result, "GetOrderHistory should not return Futures orders without a pair store")
	})

	t.Run("FuturesEmptyPair", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/derivatives/api/v3/recentorders", r.URL.Path, "GetOrderHistory should use Futures REST v3 recent orders")
			_, writeErr := w.Write([]byte(`{"orderEvents":[]}`))
			assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Futures GetOrderHistory response")
		}))
		defer server.Close()
		ex := newAuthenticatedFuturesExchange(t, server.URL)
		result, historyErr := ex.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
			AssetType: asset.Futures,
			Type:      order.AnyType,
			Side:      order.AnySide,
			Pairs:     currency.Pairs{currency.EMPTYPAIR},
		})
		require.NoError(t, historyErr, "GetOrderHistory must preserve existing empty Futures pair handling")
		assert.Empty(t, result, "GetOrderHistory should not return an order for an empty Futures pair")
	})

	for _, tc := range []struct {
		name           string
		response       string
		errContains    string
		expected       int
		expectedStatus order.Status
		usePairStore   bool
	}{
		{name: "APIError", response: `{"result":"error","error":"authenticationError"}`, errContains: "authenticationError"},
		{name: "ExecutionSideError", response: futuresRecentOrderResponse("execution", "hold", "lmt"), errContains: "invalid side"},
		{name: "ExecutionTypeError", response: futuresRecentOrderResponse("execution", "buy", "unknown"), errContains: "invalid orderType"},
		{name: "Execution", response: futuresRecentOrderResponse("execution", "buy", "lmt"), expected: 1, usePairStore: true},
		{name: "RejectedSideError", response: futuresRecentOrderResponse("rejected", "hold", "lmt"), errContains: "invalid side"},
		{name: "RejectedTypeError", response: futuresRecentOrderResponse("rejected", "buy", "unknown"), errContains: "invalid orderType"},
		{name: "Rejected", response: futuresRecentOrderResponse("rejected", "buy", "lmt"), expected: 1, expectedStatus: order.Rejected},
		{name: "CancelledSideError", response: futuresRecentOrderResponse("cancelled", "hold", "lmt"), errContains: "invalid side"},
		{name: "CancelledTypeError", response: futuresRecentOrderResponse("cancelled", "buy", "unknown"), errContains: "invalid orderType"},
		{name: "Cancelled", response: futuresRecentOrderResponse("cancelled", "buy", "lmt"), expected: 1, expectedStatus: order.Cancelled},
		{name: "PlacedSideError", response: futuresRecentOrderResponse("placed", "hold", "lmt"), errContains: "invalid side"},
		{name: "PlacedTypeError", response: futuresRecentOrderResponse("placed", "buy", "unknown"), errContains: "invalid orderType"},
		{name: "Placed", response: futuresRecentOrderResponse("placed", "buy", "lmt"), expected: 1},
		{name: "InvalidEvent", response: futuresRecentOrderResponse("invalid", "buy", "lmt"), errContains: "invalid orderHistory data"},
	} {
		t.Run("Futures/"+tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/derivatives/api/v3/recentorders", r.URL.Path, "GetOrderHistory should use Futures REST v3 recent orders")
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "ResponseWriter.Write should not error for the Futures GetOrderHistory response")
			}))
			defer server.Close()
			ex := newAuthenticatedFuturesExchange(t, server.URL)
			require.NoError(t, ex.SetAssetPairStore(asset.Futures, currency.PairStore{
				AssetEnabled:  true,
				Enabled:       currency.Pairs{futuresTestPair},
				Available:     currency.Pairs{futuresTestPair},
				RequestFormat: &currency.PairFormat{Uppercase: true, Delimiter: currency.UnderscoreDelimiter},
				ConfigFormat:  &currency.PairFormat{Uppercase: true, Delimiter: currency.UnderscoreDelimiter},
			}), "SetAssetPairStore must configure Futures pairs")
			req := &order.MultiOrderRequest{
				AssetType: asset.Futures,
				Type:      order.AnyType,
				Side:      order.AnySide,
			}
			if !tc.usePairStore {
				req.Pairs = currency.Pairs{futuresTestPair}
			}

			result, historyErr := ex.GetOrderHistory(t.Context(), req)
			if tc.errContains != "" {
				require.ErrorContains(t, historyErr, tc.errContains, "GetOrderHistory must return the expected Futures error")
				return
			}
			require.NoError(t, historyErr, "GetOrderHistory must parse the Futures response")
			require.Len(t, result, tc.expected, "GetOrderHistory must return the expected Futures orders")
			if tc.expected > 0 {
				assert.Equal(t, "ORDER-1", result[0].OrderID, "result[0].OrderID should match")
				assert.Equal(t, tc.expectedStatus, result[0].Status, "result[0].Status should match")
			}
		})
	}
}
