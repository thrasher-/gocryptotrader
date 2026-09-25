package bitstamp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Please supply your own keys in bitstamp_live_test.go for due diligence testing
const (
	canManipulateRealOrders = false
	// useTestNet directs live REST tests at the sandbox, which requires sandbox API credentials
	useTestNet = false
)

var (
	e             *Exchange
	spotPair      = currency.NewBTCUSD()
	perpetualPair = currency.NewPair(currency.BTC, currency.NewCode("USD-PERP"))
)

// skipIfLiveWithoutCredentials skips authenticated tests when live testing without credentials, or without permission
// to manipulate orders when canManipulateOrders is supplied
func skipIfLiveWithoutCredentials(t *testing.T, canManipulateOrders ...bool) {
	t.Helper()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateOrders...)
	}
}

// newTestExchange returns an exchange whose REST requests are served by handler
func newTestExchange(t *testing.T, handler http.HandlerFunc) *Exchange {
	t.Helper()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	ex.Name = t.Name()
	ex.API.AuthenticatedSupport = true
	ex.SetCredentials(&accounts.Credentials{Key: "key", Secret: "secret"})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL+"/api"), "SetRunningURL must not error")
	return ex
}

func TestSendAuthenticatedHTTPRequestSignature(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                string
		method              string
		params              url.Values
		body                any
		subAccount          string
		expectedContentType string
		expectedBody        string
		expectedQuery       string
	}{
		{name: "empty POST", method: http.MethodPost},
		{name: "empty form", method: http.MethodPost, body: url.Values{}},
		{name: "form", method: http.MethodPost, body: url.Values{"offset": {"1"}}, expectedContentType: formContentType, expectedBody: "offset=1"},
		{name: "JSON", method: http.MethodPost, body: &closePositionBody{PositionID: "1234567890"}, expectedContentType: jsonContentType, expectedBody: `{"position_id":"1234567890"}`},
		{name: "query", method: http.MethodGet, params: url.Values{"limit": {"10"}, "sort": {"asc"}}, expectedQuery: "limit=10&sort=asc"},
		{name: "sub account", method: http.MethodPost, subAccount: "990129"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var captured *http.Request
			var capturedBody []byte
			ex := newTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
				captured = r
				var err error
				capturedBody, err = io.ReadAll(r.Body)
				assert.NoError(t, err, "ReadAll should not error")
				_, err = w.Write([]byte(`{"status":"ok"}`))
				assert.NoError(t, err, "Write should not error")
			})
			creds := &accounts.Credentials{Key: "key", Secret: "secret", SubAccount: tc.subAccount}
			ex.SetCredentials(creds)
			err := ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, tc.method, "/v2/user_transactions/", tc.params, tc.body, nil)
			require.NoError(t, err, "SendAuthenticatedHTTPRequest must not error")
			require.NotNil(t, captured, "request must be captured")

			assert.Equal(t, tc.method, captured.Method, "method should be correct")
			assert.Equal(t, tc.expectedBody, string(capturedBody), "body should be correct")
			assert.Equal(t, tc.expectedQuery, captured.URL.RawQuery, "query should be correct")
			assert.Equal(t, tc.expectedContentType, captured.Header.Get("Content-Type"), "Content-Type should only be set with a body")
			assert.Equal(t, "BITSTAMP key", captured.Header.Get("X-Auth"), "X-Auth should be correct")
			assert.Equal(t, "v2", captured.Header.Get("X-Auth-Version"), "X-Auth-Version should be correct")
			assert.Equal(t, tc.subAccount, captured.Header.Get("X-Auth-Subaccount-Id"), "X-Auth-Subaccount-Id should be correct")
			nonce := captured.Header.Get("X-Auth-Nonce")
			assert.Len(t, nonce, 36, "X-Auth-Nonce should be a UUID")
			timestamp := captured.Header.Get("X-Auth-Timestamp")
			ms, err := strconv.ParseInt(timestamp, 10, 64)
			require.NoError(t, err, "X-Auth-Timestamp must be an integer")
			assert.WithinDuration(t, time.Now(), time.UnixMilli(ms), time.Minute, "X-Auth-Timestamp should be in milliseconds")

			query := ""
			if tc.expectedQuery != "" {
				query = "?" + tc.expectedQuery
			}
			message := "BITSTAMP key" + tc.method + captured.Host + "/api/v2/user_transactions/" + query + tc.expectedContentType + nonce + timestamp + "v2" + tc.expectedBody
			mac := hmac.New(sha256.New, []byte("secret"))
			_, err = mac.Write([]byte(message))
			require.NoError(t, err, "Write must not error")
			assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), captured.Header.Get("X-Auth-Signature"), "X-Auth-Signature should sign the documented message")
		})
	}
}

func TestSendAuthenticatedHTTPRequestErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		status   int
		response string
		contains string
	}{
		{name: "status error", status: http.StatusOK, response: `{"status":"error","reason":"Order not found","code":"404.002"}`, contains: "404.002 Order not found"},
		{name: "field errors", status: http.StatusOK, response: `{"status":"error","reason":{"__all__":["Minimum order size is 10.0 USD."],"price":["Enter a number."]}}`, contains: "Minimum order size is 10.0 USD.; price: Enter a number."},
		{name: "simple error", status: http.StatusOK, response: `{"error":"No permission found"}`, contains: "No permission found"},
		{name: "auth failure", status: http.StatusForbidden, response: `{"status":"error","reason":"Invalid signature","code":"API0005"}`, contains: "API0005 Invalid signature"},
		{name: "derivatives error", status: http.StatusBadRequest, response: `{"code":"API5506","message":"Trade account does not support derivatives.","field":"market"}`, contains: "API5506 Trade account does not support derivatives.; field: market"},
		{name: "unauthorised", status: http.StatusUnauthorized, response: `{"message":"Unauthorized"}`, contains: "Unauthorized"},
		{name: "response code", status: http.StatusOK, response: `{"status":"error","reason":"Request rejected","response_code":"400.002","response_explanation":"Request rejected due to exceeded rate limit."}`, contains: "400.002 Request rejected; Request rejected due to exceeded rate limit."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex := newTestExchange(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, err := w.Write([]byte(tc.response))
				assert.NoError(t, err, "Write should not error")
			})
			err := ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, http.MethodPost, "/v2/order_status/", nil, nil, nil)
			require.ErrorIs(t, err, errAPIResponse, "SendAuthenticatedHTTPRequest must return an API error")
			assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest should flag an authenticated request failure")
			assert.ErrorContains(t, err, tc.contains, "error should contain the reported details")
		})
	}

	ex := newTestExchange(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, err := w.Write([]byte(`<html>bad gateway</html>`))
		assert.NoError(t, err, "Write should not error")
	})
	err := ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, http.MethodPost, "/v2/order_status/", nil, nil, nil)
	assert.ErrorIs(t, err, request.ErrBadStatus, "SendAuthenticatedHTTPRequest should return the status error for an unrecognised body")
	assert.NotErrorIs(t, err, errAPIResponse, "SendAuthenticatedHTTPRequest should not report an API error for an unrecognised body")
}

func TestSendHTTPRequestErrors(t *testing.T) {
	t.Parallel()
	ex := newTestExchange(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := ex.GetFundingRate(t.Context(), spotPair)
	assert.ErrorIs(t, err, request.ErrBadStatus, "GetFundingRate should return the status error for an empty body")

	ex = newTestExchange(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(`{"message":"Cannot fetch funding rate history for market BTC/USD-PERP"}`))
		assert.NoError(t, err, "Write should not error")
	})
	_, err = ex.GetFundingRateHistory(t.Context(), &FundingRateHistoryRequest{Pair: perpetualPair})
	require.ErrorIs(t, err, errAPIResponse, "GetFundingRateHistory must return an API error")
	assert.NotErrorIs(t, err, request.ErrAuthRequestFailed, "GetFundingRateHistory should not flag an authenticated request failure")
	assert.ErrorContains(t, err, "Cannot fetch funding rate history", "error should contain the reported message")
}

func TestCheckResponseError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		data               string
		unsuccessfulStatus bool
		err                error
		contains           string
	}{
		{name: "array", data: `[{"status":"error"}]`},
		{name: "empty", data: ``},
		{name: "null", data: `null`},
		{name: "cancelled order", data: `{"id":1,"status":"Canceled"}`},
		{name: "transfer", data: `{"status":"ok"}`},
		{name: "code without message", data: `{"code":"123"}`},
		{name: "status error without details", data: `{"status":"error"}`, err: errAPIResponse, contains: `{"status":"error"}`},
		{name: "reason list", data: `{"status":"error","reason":["a","b"]}`, err: errAPIResponse, contains: "a, b"},
		{name: "numeric code", data: `{"code":4009,"message":"Connection is unauthorized."}`, err: errAPIResponse, contains: "4009 Connection is unauthorized."},
		{name: "error object", data: `{"error":{"amount":["Required"]}}`, err: errAPIResponse, contains: "amount: Required"},
		{name: "code only on failure", data: `{"code":"API0001"}`, unsuccessfulStatus: true, err: errAPIResponse, contains: "API0001"},
		{name: "unrecognised failure", data: `{"detail":"nope"}`, unsuccessfulStatus: true},
		{name: "mismatched types", data: `{"status":["error"],"reason":1}`, unsuccessfulStatus: true, err: errAPIResponse, contains: "API returned an error: 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkResponseError(json.RawMessage(tc.data), tc.unsuccessfulStatus)
			if tc.err == nil {
				assert.NoError(t, err, "checkResponseError should not error")
				return
			}
			assert.ErrorIs(t, err, tc.err, "checkResponseError should return the correct error")
			assert.ErrorContains(t, err, tc.contains, "checkResponseError should include the details")
		})
	}
}

func TestProcessResponse(t *testing.T) {
	t.Parallel()
	errRequest := errors.New("request failed")
	assert.ErrorIs(t, processResponse(nil, errRequest, nil, false), errRequest, "processResponse should return the request error")
	assert.NoError(t, processResponse(nil, nil, new(any), true), "processResponse should not error on an empty body")

	var resp *AccountBalanceResponse
	require.NoError(t, processResponse(json.RawMessage(`{"currency":"btc","total":"1.5"}`), nil, &resp, true), "processResponse must not error")
	require.NotNil(t, resp, "response must be decoded")
	assert.Equal(t, 1.5, resp.Total.Float64(), "Total should be decoded")

	err := processResponse(json.RawMessage(`{"status":"error","reason":"nope"}`), nil, &resp, false)
	assert.ErrorIs(t, err, errAPIResponse, "processResponse should return an API error")
	assert.NotErrorIs(t, err, request.ErrAuthRequestFailed, "processResponse should not flag an unauthenticated request")
}

func TestFormatMarketSymbolAndName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		pair   currency.Pair
		symbol string
		name   string
	}{
		{pair: spotPair, symbol: "btcusd", name: "BTC/USD"},
		{pair: perpetualPair, symbol: "btcusd-perp", name: "BTC/USD-PERP"},
		{pair: currency.NewPairWithDelimiter("eth", "usd-perp", "_"), symbol: "ethusd-perp", name: "ETH/USD-PERP"},
	} {
		assert.Equalf(t, tc.symbol, formatMarketSymbol(tc.pair), "formatMarketSymbol should format %s", tc.pair)
		assert.Equalf(t, tc.name, formatMarketName(tc.pair), "formatMarketName should format %s", tc.pair)
	}
}

func TestFormatOrderSide(t *testing.T) {
	t.Parallel()
	for side, exp := range map[order.Side]string{order.Buy: "buy", order.Bid: "buy", order.Long: "buy", order.Sell: "sell", order.Ask: "sell", order.Short: "sell"} {
		s, err := formatOrderSide(side)
		require.NoErrorf(t, err, "formatOrderSide must not error for %s", side)
		assert.Equalf(t, exp, s, "formatOrderSide should format %s", side)
	}
	_, err := formatOrderSide(order.AnySide)
	assert.ErrorIs(t, err, order.ErrSideIsInvalid, "formatOrderSide should reject an invalid side")
}

func TestGetCurrencies(t *testing.T) {
	t.Parallel()
	currencies, err := e.GetCurrencies(t.Context())
	require.NoError(t, err, "GetCurrencies must not error")
	require.NotEmpty(t, currencies, "GetCurrencies must return currencies")
	var btc *CurrencyResponse
	for i := range currencies {
		if currencies[i].Currency.Equal(currency.BTC) {
			btc = &currencies[i]
		}
	}
	require.NotNil(t, btc, "BTC must be listed")
	assert.Equal(t, "crypto", btc.Type, "Type should be correct")
	assert.Equal(t, uint8(8), btc.Decimals, "Decimals should be correct")
	assert.Positive(t, btc.AvailableSupply.Float64(), "AvailableSupply should be positive")
	require.NotEmpty(t, btc.Networks, "Networks must not be empty")
	assert.Equal(t, "bitcoin", btc.Networks[0].Network, "Network should be correct")
}

func TestGetTickers(t *testing.T) {
	t.Parallel()
	tickers, err := e.GetTickers(t.Context())
	require.NoError(t, err, "GetTickers must not error")
	marketTypes := make(map[string]bool)
	for i := range tickers {
		assert.NotEmpty(t, tickers[i].Market, "Market should be set")
		assert.Positive(t, tickers[i].Last.Float64(), "Last should be positive")
		marketTypes[tickers[i].MarketType] = true
	}
	assert.True(t, marketTypes[MarketTypeSpot], "GetTickers should return spot markets")
	assert.True(t, marketTypes[MarketTypePerpetual], "GetTickers should return perpetual markets")
}

func TestGetTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetTicker(t.Context(), currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetTicker should error on an empty pair")
	_, err = e.GetHourlyTicker(t.Context(), currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetHourlyTicker should error on an empty pair")

	for _, tc := range []struct {
		pair       currency.Pair
		marketType string
	}{
		{pair: spotPair, marketType: MarketTypeSpot},
		{pair: perpetualPair, marketType: MarketTypePerpetual},
	} {
		for name, get := range map[string]func() (*TickerResponse, error){
			"GetTicker":       func() (*TickerResponse, error) { return e.GetTicker(t.Context(), tc.pair) },
			"GetHourlyTicker": func() (*TickerResponse, error) { return e.GetHourlyTicker(t.Context(), tc.pair) },
		} {
			tick, err := get()
			require.NoErrorf(t, err, "%s must not error for %s", name, tc.pair)
			require.NotNilf(t, tick, "%s must return a ticker for %s", name, tc.pair)
			assert.Positivef(t, tick.Last.Float64(), "%s Last should be positive for %s", name, tc.pair)
			assert.Positivef(t, tick.Bid.Float64(), "%s Bid should be positive for %s", name, tc.pair)
			assert.Positivef(t, tick.Ask.Float64(), "%s Ask should be positive for %s", name, tc.pair)
			assert.NotZerof(t, tick.Timestamp.Time(), "%s Timestamp should be set for %s", name, tc.pair)
			assert.Containsf(t, []order.Side{order.Buy, order.Sell}, tick.Side.Side(), "%s Side should be valid for %s", name, tc.pair)
			assert.Equalf(t, tc.marketType, tick.MarketType, "%s MarketType should be correct for %s", name, tc.pair)
			if tc.marketType == MarketTypePerpetual {
				assert.Positivef(t, tick.MarkPrice.Float64(), "%s MarkPrice should be positive for %s", name, tc.pair)
				assert.Positivef(t, tick.IndexPrice.Float64(), "%s IndexPrice should be positive for %s", name, tc.pair)
			}
		}
	}
}

func TestGetOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderbook(t.Context(), currency.EMPTYPAIR, OrderbookGroupedByPrice)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetOrderbook should error on an empty pair")
	_, err = e.GetOrderbook(t.Context(), spotPair, OrderbookUngroupedWithOrderIDs+1)
	assert.ErrorIs(t, err, errInvalidOrderbookGrouping, "GetOrderbook should error on an invalid grouping")

	for _, tc := range []struct {
		pair     currency.Pair
		grouping OrderbookGrouping
	}{
		{pair: spotPair, grouping: OrderbookUngrouped},
		{pair: spotPair, grouping: OrderbookGroupedByPrice},
		{pair: spotPair, grouping: OrderbookUngroupedWithOrderIDs},
		{pair: perpetualPair, grouping: OrderbookGroupedByPrice},
	} {
		ob, err := e.GetOrderbook(t.Context(), tc.pair, tc.grouping)
		require.NoErrorf(t, err, "GetOrderbook must not error for %s grouping %d", tc.pair, tc.grouping)
		assert.NotZerof(t, ob.Microtimestamp.Time(), "Microtimestamp should be set for %s grouping %d", tc.pair, tc.grouping)
		for _, levels := range [][]OrderbookLevel{ob.Bids, ob.Asks} {
			require.NotEmptyf(t, levels, "levels must not be empty for %s grouping %d", tc.pair, tc.grouping)
			assert.Positivef(t, levels[0].Price, "Price should be positive for %s grouping %d", tc.pair, tc.grouping)
			assert.Positivef(t, levels[0].Amount, "Amount should be positive for %s grouping %d", tc.pair, tc.grouping)
			if tc.grouping == OrderbookUngroupedWithOrderIDs {
				assert.NotZerof(t, levels[0].OrderID, "OrderID should be set for %s grouping %d", tc.pair, tc.grouping)
			} else {
				assert.Zerof(t, levels[0].OrderID, "OrderID should not be set for %s grouping %d", tc.pair, tc.grouping)
			}
		}
	}
}

func TestGetTransactions(t *testing.T) {
	t.Parallel()
	_, err := e.GetTransactions(t.Context(), currency.EMPTYPAIR, "")
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetTransactions should error on an empty pair")
	_, err = e.GetTransactions(t.Context(), spotPair, "week")
	assert.ErrorIs(t, err, errInvalidTransactionPeriod, "GetTransactions should error on an invalid time period")

	for _, period := range []string{"", TransactionPeriodHour} {
		transactions, err := e.GetTransactions(t.Context(), spotPair, period)
		require.NoErrorf(t, err, "GetTransactions must not error for period %q", period)
		require.NotEmptyf(t, transactions, "GetTransactions must return transactions for period %q", period)
		for _, tr := range transactions {
			assert.NotZero(t, tr.Date.Time(), "Date should be set")
			assert.NotZero(t, tr.TradeID, "TradeID should be set")
			assert.Positive(t, tr.Price.Float64(), "Price should be positive")
			assert.Positive(t, tr.Amount.Float64(), "Amount should be positive")
			assert.Contains(t, []order.Side{order.Buy, order.Sell}, tr.Side.Side(), "Side should be valid")
		}
	}
}

func TestGetMarkets(t *testing.T) {
	t.Parallel()
	for _, entity := range []string{"", "EUROPE_SA"} {
		markets, err := e.GetMarkets(t.Context(), entity)
		require.NoErrorf(t, err, "GetMarkets must not error for entity %q", entity)
		marketTypes := make(map[string]bool)
		for i := range markets {
			m := &markets[i]
			marketTypes[m.MarketType] = true
			assert.NotEmpty(t, m.Name, "Name should be set")
			assert.NotEmpty(t, m.MarketSymbol, "MarketSymbol should be set")
			assert.False(t, m.BaseCurrency.IsEmpty(), "BaseCurrency should be set")
			assert.False(t, m.CounterCurrency.IsEmpty(), "CounterCurrency should be set")
			assert.Positive(t, m.MinimumOrderValue.Float64(), "MinimumOrderValue should be positive")
			if m.MarketType == MarketTypePerpetual {
				assert.Positive(t, m.MaxLeverage.Float64(), "MaxLeverage should be positive")
				assert.Positive(t, m.ContractSize.Float64(), "ContractSize should be positive")
				assert.Equal(t, "Linear", m.PayoffType, "PayoffType should be correct")
			}
		}
		assert.Truef(t, marketTypes[MarketTypeSpot], "GetMarkets should return spot markets for entity %q", entity)
		if entity == "" {
			assert.True(t, marketTypes[MarketTypePerpetual], "GetMarkets should return perpetual markets")
		}
	}
}

func TestGetOHLC(t *testing.T) {
	t.Parallel()
	start := time.Unix(1735689600, 0)
	end := time.Unix(1735693200, 0)
	for _, tc := range []struct {
		req *OHLCRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &OHLCRequest{}, err: currency.ErrCurrencyPairEmpty},
		{req: &OHLCRequest{Pair: spotPair}, err: errInvalidInterval},
		{req: &OHLCRequest{Pair: spotPair, Step: kline.OneMin}, err: errInvalidLimit},
		{req: &OHLCRequest{Pair: spotPair, Step: kline.OneMin, Limit: 1001}, err: errInvalidLimit},
		{req: &OHLCRequest{Pair: spotPair, Step: kline.OneMin, Limit: 10, Start: end, End: start}, err: common.ErrStartAfterEnd},
	} {
		_, err := e.GetOHLC(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetOHLC should return the correct error")
	}

	for _, exclude := range []bool{false, true} {
		resp, err := e.GetOHLC(t.Context(), &OHLCRequest{Pair: spotPair, Step: kline.OneMin, Limit: 10, Start: start, End: end, ExcludeCurrentCandle: exclude})
		require.NoError(t, err, "GetOHLC must not error")
		assert.Equal(t, "BTC/USD", resp.Data.Pair, "Pair should be correct")
		require.Len(t, resp.Data.OHLC, 10, "GetOHLC must return the requested candles")
		for _, c := range resp.Data.OHLC {
			assert.False(t, c.Timestamp.Time().Before(start), "Timestamp should not be before the start")
			assert.Positive(t, c.Open.Float64(), "Open should be positive")
			assert.Positive(t, c.High.Float64(), "High should be positive")
			assert.Positive(t, c.Low.Float64(), "Low should be positive")
			assert.Positive(t, c.Close.Float64(), "Close should be positive")
		}
	}
}

func TestGetEURUSDConversionRate(t *testing.T) {
	t.Parallel()
	rate, err := e.GetEURUSDConversionRate(t.Context())
	require.NoError(t, err, "GetEURUSDConversionRate must not error")
	assert.Positive(t, rate.Buy.Float64(), "Buy should be positive")
	assert.Positive(t, rate.Sell.Float64(), "Sell should be positive")
}

func TestGetFundingRate(t *testing.T) {
	t.Parallel()
	_, err := e.GetFundingRate(t.Context(), currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetFundingRate should error on an empty pair")

	rate, err := e.GetFundingRate(t.Context(), perpetualPair)
	require.NoError(t, err, "GetFundingRate must not error")
	assert.Equal(t, "BTC/USD-PERP", rate.Market, "Market should be correct")
	assert.NotZero(t, rate.Timestamp.Time(), "Timestamp should be set")
	assert.True(t, rate.NextFundingTime.Time().After(rate.Timestamp.Time()), "NextFundingTime should be after Timestamp")
}

func TestGetFundingRateHistory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *FundingRateHistoryRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &FundingRateHistoryRequest{}, err: currency.ErrCurrencyPairEmpty},
		{req: &FundingRateHistoryRequest{Pair: perpetualPair, Limit: 101}, err: errInvalidLimit},
		{req: &FundingRateHistoryRequest{Pair: perpetualPair, Since: time.Unix(2, 0), Until: time.Unix(1, 0)}, err: common.ErrStartAfterEnd},
	} {
		_, err := e.GetFundingRateHistory(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetFundingRateHistory should return the correct error")
	}

	history, err := e.GetFundingRateHistory(t.Context(), &FundingRateHistoryRequest{Pair: perpetualPair, Limit: 3})
	require.NoError(t, err, "GetFundingRateHistory must not error")
	assert.Equal(t, "BTC/USD-PERP", history.Market, "Market should be correct")
	require.Len(t, history.FundingRateHistory, 3, "GetFundingRateHistory must return the requested rates")
	for i := 1; i < len(history.FundingRateHistory); i++ {
		assert.True(t, history.FundingRateHistory[i].Timestamp.Time().After(history.FundingRateHistory[i-1].Timestamp.Time()), "rates should be in ascending order")
	}
}

func TestGetVASPs(t *testing.T) {
	t.Parallel()
	vasps, err := e.GetVASPs(t.Context(), 1, 3)
	require.NoError(t, err, "GetVASPs must not error")
	require.NotEmpty(t, vasps.Data, "GetVASPs must return VASPs")
	assert.NotEmpty(t, vasps.Data[0].UUID, "UUID should be set")
	assert.NotEmpty(t, vasps.Data[0].Name, "Name should be set")
	if mockTests {
		assert.Equal(t, Pagination{Page: 1, Size: 3, Count: 1204}, vasps.Pagination, "Pagination should be correct")
	}
}

func TestGetAccountBalances(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	balances, err := e.GetAccountBalances(t.Context())
	require.NoError(t, err, "GetAccountBalances must not error")
	if mockTests {
		require.Len(t, balances, 2, "GetAccountBalances must return the balances")
		assert.True(t, balances[1].Currency.Equal(currency.BTC), "Currency should be correct")
		assert.Equal(t, 1.5, balances[1].Total.Float64(), "Total should be correct")
		assert.Equal(t, 1.25, balances[1].Available.Float64(), "Available should be correct")
		assert.Equal(t, 0.25, balances[1].Reserved.Float64(), "Reserved should be correct")
	}

	_, err = e.GetAccountBalance(t.Context(), currency.EMPTYCODE)
	assert.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "GetAccountBalance should error on an empty currency")
	balance, err := e.GetAccountBalance(t.Context(), currency.BTC)
	require.NoError(t, err, "GetAccountBalance must not error")
	assert.True(t, balance.Currency.Equal(currency.BTC), "Currency should be correct")
}

func TestGetTradingFees(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	fees, err := e.GetTradingFees(t.Context())
	require.NoError(t, err, "GetTradingFees must not error")
	require.NotEmpty(t, fees, "GetTradingFees must return fees")
	for _, f := range fees {
		assert.NotEmpty(t, f.Market, "Market should be set")
		assert.GreaterOrEqual(t, f.Fees.Taker.Float64(), f.Fees.Maker.Float64(), "Taker fee should not be below the maker fee")
	}

	_, err = e.GetTradingFee(t.Context(), currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetTradingFee should error on an empty pair")
	fee, err := e.GetTradingFee(t.Context(), spotPair)
	require.NoError(t, err, "GetTradingFee must not error")
	assert.Equal(t, "btcusd", fee.Market, "Market should be correct")
	if mockTests {
		assert.Equal(t, MakerTakerFees{Maker: 0.3, Taker: 0.4}, fee.Fees, "Fees should be correct")
	}
}

func TestGetWithdrawalFees(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	fees, err := e.GetWithdrawalFees(t.Context())
	require.NoError(t, err, "GetWithdrawalFees must not error")
	require.NotEmpty(t, fees, "GetWithdrawalFees must return fees")

	_, err = e.GetWithdrawalFee(t.Context(), currency.EMPTYCODE, "")
	assert.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "GetWithdrawalFee should error on an empty currency")
	for _, network := range []string{"", "lightning"} {
		fee, err := e.GetWithdrawalFee(t.Context(), currency.BTC, network)
		require.NoErrorf(t, err, "GetWithdrawalFee must not error for network %q", network)
		assert.True(t, fee.Currency.Equal(currency.BTC), "Currency should be correct")
		if mockTests {
			exp := map[string]WithdrawalFeeResponse{
				"":          {Currency: currency.NewCode("btc"), Fee: 0.00015, Network: "bitcoin"},
				"lightning": {Currency: currency.NewCode("btc"), Fee: 0.00001, Network: "lightning"},
			}[network]
			assert.Equalf(t, exp, *fee, "GetWithdrawalFee should return the fee for network %q", network)
		}
	}
}

func TestGetOrderStatus(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderStatus(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetOrderStatus should error on a nil request")
	_, err = e.GetOrderStatus(t.Context(), &OrderStatusRequest{})
	assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "GetOrderStatus should error without an order ID")

	skipIfLiveWithoutCredentials(t)
	status, err := e.GetOrderStatus(t.Context(), &OrderStatusRequest{OrderID: 1458532827766784})
	if !mockTests {
		assert.ErrorIs(t, err, errAPIResponse, "GetOrderStatus should error for an unknown order")
		return
	}
	require.NoError(t, err, "GetOrderStatus must not error")
	exp := &OrderStatusResponse{
		ID:       1458532827766784,
		DateTime: types.DateTime(time.Date(2022, 1, 31, 14, 43, 15, 0, time.UTC)),
		Side:     orderSide(order.Buy),
		Status:   "Open",
		Market:   "BTC/USD",
		Transactions: []OrderTransaction{
			{TradeID: 209895701, Price: 20000, Fee: 8, DateTime: time.Date(2022, 1, 31, 14, 45, 15, 322000000, time.UTC), Type: TransactionTypeMarketTrade, Amounts: map[currency.Code]float64{currency.USD: -2000, currency.BTC: 0.1}},
			{TradeID: 209895702, Price: 20100, Fee: 8.04, DateTime: time.Date(2022, 1, 31, 14, 46, 15, 0, time.UTC), Type: TransactionTypeMarketTrade, Amounts: map[currency.Code]float64{currency.USD: -2010, currency.BTC: 0.1}},
		},
		AmountRemaining: 0.3,
		ClientOrderID:   "my-order-123",
	}
	assert.Equal(t, exp, status, "GetOrderStatus should return the order status")

	status, err = e.GetOrderStatus(t.Context(), &OrderStatusRequest{ClientOrderID: "my-order-123", OmitTransactions: true})
	require.NoError(t, err, "GetOrderStatus must not error with a client order ID")
	assert.Empty(t, status.Transactions, "Transactions should be omitted")
}

func TestGetOrderData(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderData(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetOrderData should error on a nil request")
	_, err = e.GetOrderData(t.Context(), &OrderEventsRequest{})
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetOrderData should error on an empty pair")

	skipIfLiveWithoutCredentials(t)
	events, err := e.GetOrderData(t.Context(), &OrderEventsRequest{Pair: spotPair})
	require.NoError(t, err, "GetOrderData must not error")
	if mockTests {
		require.Len(t, events, 1, "GetOrderData must return the events")
		assert.Equal(t, "order_created", events[0].Event, "Event should be correct")
		assert.Equal(t, OrderSourceOrderbook, events[0].OrderSource, "OrderSource should be correct")
		assert.Equal(t, uint64(1458532827766784), events[0].Data.ID, "ID should be correct")
		assert.Equal(t, 50000.0, events[0].Data.Price.Float64(), "Price should be correct")
		assert.Equal(t, 0.5, events[0].Data.Amount.Float64(), "Amount should be correct")
	}
}

func TestGetAccountOrderData(t *testing.T) {
	t.Parallel()
	_, err := e.GetAccountOrderData(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetAccountOrderData should error on a nil request")
	_, err = e.GetAccountOrderData(t.Context(), &OrderEventsRequest{Pair: perpetualPair})
	assert.ErrorIs(t, err, errOrderSourceRequired, "GetAccountOrderData should error without an order source")
	_, err = e.GetAccountOrderData(t.Context(), &OrderEventsRequest{OrderSource: OrderSourceOrderbook})
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetAccountOrderData should error on an empty pair")

	skipIfLiveWithoutCredentials(t)
	events, err := e.GetAccountOrderData(t.Context(), &OrderEventsRequest{Pair: perpetualPair, OrderSource: OrderSourceOrderbook, SinceID: "019f4ac8-1234-abcd-af00-3e9012000020"})
	require.NoError(t, err, "GetAccountOrderData must not error")
	if mockTests {
		require.Len(t, events, 1, "GetAccountOrderData must return the events")
		exp := OrderEventResponse{
			Event:       "order_replaced",
			EventID:     "019f4ac8-1234-abcd-af00-3e9012000021",
			OrderSource: OrderSourceOrderbook,
			Data: OrderEventData{
				ID:              1458532827766786,
				Side:            orderSide(order.Sell),
				OrderSubtype:    22,
				DateTime:        types.Time(time.Unix(1643698765, 0)),
				Microtimestamp:  types.Time(time.UnixMicro(1643698765000000)),
				Amount:          0.5,
				AmountAtCreate:  1,
				Price:           50000,
				ClientOrderID:   "my-order-123",
				OriginalOrderID: 1458532827766700,
				ReduceOnly:      true,
				StopPrice:       48000,
				ActivationPrice: 49000,
				TrailingDelta:   50,
			},
		}
		assert.Equal(t, exp, events[0], "GetAccountOrderData should return the event")
	}
}

func TestGetOpenOrders(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	for _, pair := range []currency.Pair{currency.EMPTYPAIR, spotPair, perpetualPair} {
		orders, err := e.GetOpenOrders(t.Context(), pair)
		require.NoErrorf(t, err, "GetOpenOrders must not error for %q", pair)
		if !mockTests {
			continue
		}
		require.NotEmptyf(t, orders, "GetOpenOrders must return orders for %q", pair)
		for i := range orders {
			o := &orders[i]
			switch o.Market {
			case "BTC/USD":
				exp := OpenOrderResponse{
					ID:             1234123412341234,
					DateTime:       types.DateTime(time.Date(2022, 1, 31, 14, 43, 15, 0, time.UTC)),
					Side:           orderSide(order.Buy),
					Price:          100,
					Amount:         0.4,
					AmountAtCreate: 0.5,
					Market:         "BTC/USD",
					LimitPrice:     110,
					ClientOrderID:  "my-order-123",
				}
				assert.Equal(t, exp, *o, "spot order should be correct")
			case "BTC/USD-PERP":
				assert.Equal(t, uint64(1234123412341235), o.ID, "ID should be correct")
				assert.Equal(t, OrderSubtypeStopLossLimit, o.Subtype, "Subtype should be correct")
				assert.Equal(t, MarginModeCross, o.MarginMode, "MarginMode should be correct")
				assert.Equal(t, 3.0, o.Leverage.Float64(), "Leverage should be correct")
				assert.Equal(t, 84500.0, o.StopPrice.Float64(), "StopPrice should be correct")
				assert.True(t, o.ReduceOnly, "ReduceOnly should be correct")
			default:
				assert.Failf(t, "unexpected market", "GetOpenOrders returned market %q", o.Market)
			}
		}
	}
}

func TestPlaceLimitOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *LimitOrderRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &LimitOrderRequest{}, err: currency.ErrCurrencyPairEmpty},
		{req: &LimitOrderRequest{Pair: spotPair}, err: order.ErrSideIsInvalid},
		{req: &LimitOrderRequest{Pair: spotPair, Side: order.Buy}, err: order.ErrPriceMustBeSetIfLimitOrder},
		{req: &LimitOrderRequest{Pair: spotPair, Side: order.Buy, Price: 1, GoodTillDate: true}, err: common.ErrDateUnset},
	} {
		_, err := e.PlaceLimitOrder(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "PlaceLimitOrder should return the correct error")
	}

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.PlaceLimitOrder(t.Context(), &LimitOrderRequest{Pair: spotPair, Side: order.Buy, Amount: 0.5, Price: 20000, ClientOrderID: "123456789"})
	require.NoError(t, err, "PlaceLimitOrder must not error")
	if mockTests {
		exp := &OrderResponse{
			ID:            1234123412341236,
			Market:        "BTC/USD",
			DateTime:      types.DateTime(time.Date(2022, 1, 31, 14, 43, 15, 796000000, time.UTC)),
			Side:          orderSide(order.Buy),
			Price:         20000,
			Amount:        0.5,
			ClientOrderID: "123456789",
		}
		assert.Equal(t, exp, resp, "PlaceLimitOrder should return the order")

		resp, err = e.PlaceLimitOrder(t.Context(), &LimitOrderRequest{Pair: spotPair, Side: order.Buy, Amount: 0.5, Price: 20000, LimitPrice: 21000, GoodTillDate: true, ExpireTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
		require.NoError(t, err, "PlaceLimitOrder must not error for a good till date order")
		assert.Equal(t, uint64(1234123412341237), resp.ID, "ID should be correct")
	}
}

func TestPlaceInstantOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *InstantOrderRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &InstantOrderRequest{}, err: currency.ErrCurrencyPairEmpty},
		{req: &InstantOrderRequest{Pair: spotPair}, err: order.ErrSideIsInvalid},
		{req: &InstantOrderRequest{Pair: spotPair, Side: order.Buy}, err: order.ErrAmountIsInvalid},
	} {
		_, err := e.PlaceInstantOrder(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "PlaceInstantOrder should return the correct error")
	}

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	for side, exp := range map[order.Side]uint64{order.Buy: 1234123412341239, order.Sell: 1234123412341240} {
		resp, err := e.PlaceInstantOrder(t.Context(), &InstantOrderRequest{Pair: spotPair, Side: side, Amount: 100, AmountInCounter: true})
		require.NoErrorf(t, err, "PlaceInstantOrder must not error for %s", side)
		if mockTests {
			assert.Equalf(t, exp, resp.ID, "ID should be correct for %s", side)
		}
	}
}

func TestPlaceMarketOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *MarketOrderRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &MarketOrderRequest{}, err: currency.ErrCurrencyPairEmpty},
		{req: &MarketOrderRequest{Pair: spotPair}, err: order.ErrSideIsInvalid},
	} {
		_, err := e.PlaceMarketOrder(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "PlaceMarketOrder should return the correct error")
	}

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.PlaceMarketOrder(t.Context(), &MarketOrderRequest{Pair: spotPair, Side: order.Sell, Amount: 0.1})
	require.NoError(t, err, "PlaceMarketOrder must not error")
	if mockTests {
		assert.Equal(t, uint64(1234123412341238), resp.ID, "ID should be correct")
		assert.Equal(t, order.Sell, resp.Side.Side(), "Side should be correct")
	}
}

func TestCancelExistingOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CancelExistingOrder(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "CancelExistingOrder should error on a nil request")
	_, err = e.CancelExistingOrder(t.Context(), &CancelOrderRequest{})
	assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "CancelExistingOrder should error without an order ID")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.CancelExistingOrder(t.Context(), &CancelOrderRequest{OrderID: 1453282316578816})
	if !mockTests {
		assert.ErrorIs(t, err, errAPIResponse, "CancelExistingOrder should error for an unknown order")
		return
	}
	require.NoError(t, err, "CancelExistingOrder must not error")
	exp := &CancelOrderResponse{ID: 1453282316578816, Amount: 0.02035278, Price: 2100.45, Side: orderSide(order.Buy), Market: "BTC/USD", Status: "Canceled"}
	assert.Equal(t, exp, resp, "CancelExistingOrder should return the cancelled order")

	resp, err = e.CancelExistingOrder(t.Context(), &CancelOrderRequest{ClientOrderID: "my-order-123"})
	require.NoError(t, err, "CancelExistingOrder must not error with a client order ID")
	assert.Equal(t, "Cancel pending", resp.Status, "Status should be correct")
	assert.Equal(t, 0.5, resp.Amount.Float64(), "Amount should decode from a number")
}

func TestCancelAllExistingOrders(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	for pair, exp := range map[currency.Pair]int{currency.EMPTYPAIR: 2, spotPair: 1} {
		resp, err := e.CancelAllExistingOrders(t.Context(), pair)
		require.NoErrorf(t, err, "CancelAllExistingOrders must not error for %q", pair)
		if mockTests {
			assert.Truef(t, resp.Success, "Success should be true for %q", pair)
			assert.Lenf(t, resp.Canceled, exp, "Canceled should contain the orders for %q", pair)
		}
	}
}

func TestReplaceOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *ReplaceOrderRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &ReplaceOrderRequest{}, err: order.ErrOrderIDNotSet},
		{req: &ReplaceOrderRequest{OrderID: 1}, err: order.ErrAmountIsInvalid},
		{req: &ReplaceOrderRequest{OrderID: 1, Amount: 1}, err: order.ErrPriceMustBeSetIfLimitOrder},
	} {
		_, err := e.ReplaceOrder(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "ReplaceOrder should return the correct error")
	}

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.ReplaceOrder(t.Context(), &ReplaceOrderRequest{OrderID: 1453282316578816, Amount: 0.02, Price: 2100.45})
	if !mockTests {
		assert.ErrorIs(t, err, errAPIResponse, "ReplaceOrder should error for an unknown order")
		return
	}
	require.NoError(t, err, "ReplaceOrder must not error")
	exp := &ReplaceOrderResponse{
		OrderID:         1453282316578817,
		Side:            orderSide(order.Buy),
		Market:          "BTC/USD",
		Amount:          0.02,
		Price:           2100.45,
		DateTime:        time.Date(2025, 10, 17, 14, 23, 1, 725000000, time.UTC),
		OriginalOrderID: 1453282316578816,
		Status:          "Open",
	}
	assert.Equal(t, exp, resp, "ReplaceOrder should return the replacement order")

	resp, err = e.ReplaceOrder(t.Context(), &ReplaceOrderRequest{OriginalClientOrderID: "my-order-123", ClientOrderID: "my-order-456", Amount: 0.02, Price: 2100.45})
	require.NoError(t, err, "ReplaceOrder must not error with a client order ID")
	assert.Equal(t, "my-order-123", resp.OriginalClientOrderID, "OriginalClientOrderID should be correct")
}

func TestGetTradingMarkets(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	markets, err := e.GetTradingMarkets(t.Context())
	require.NoError(t, err, "GetTradingMarkets must not error")
	require.NotEmpty(t, markets, "GetTradingMarkets must return markets")
	if mockTests {
		assert.Equal(t, TradingMarketResponse{Name: "BTC/USD", URLSymbol: "btcusd"}, markets[0], "market should be correct")
	}
}

func TestGetMaxOrderAmount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *MaxOrderAmountRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &MaxOrderAmountRequest{}, err: currency.ErrCurrencyPairEmpty},
		{req: &MaxOrderAmountRequest{Pair: perpetualPair}, err: errMarginModeRequired},
		{req: &MaxOrderAmountRequest{Pair: perpetualPair, MarginMode: MarginModeIsolated}, err: errLeverageRequired},
		{req: &MaxOrderAmountRequest{Pair: perpetualPair, MarginMode: MarginModeIsolated, Leverage: 3}, err: errOrderTypeRequired},
		{req: &MaxOrderAmountRequest{Pair: perpetualPair, MarginMode: MarginModeIsolated, Leverage: 3, OrderType: OrderSubtypeLimit}, err: order.ErrSideIsInvalid},
	} {
		_, err := e.GetMaxOrderAmount(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetMaxOrderAmount should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	resp, err := e.GetMaxOrderAmount(t.Context(), &MaxOrderAmountRequest{
		Pair:                 perpetualPair,
		MarginMode:           MarginModeIsolated,
		Leverage:             3,
		OrderType:            OrderSubtypeLimit,
		Side:                 order.Buy,
		Price:                84000,
		AdditionalCollateral: map[currency.Code]float64{currency.USD: 120},
	})
	require.NoError(t, err, "GetMaxOrderAmount must not error")
	if mockTests {
		assert.Equal(t, 1.23456, resp.MaximumOrderAmount.Float64(), "MaximumOrderAmount should be correct")
		assert.True(t, resp.MaximumOrderAmountCurrency.Equal(currency.BTC), "MaximumOrderAmountCurrency should be correct")
		assert.True(t, resp.MaximumOrderValueCurrency.Equal(currency.USD), "MaximumOrderValueCurrency should be correct")
	}
}

func TestGetWithdrawalRequests(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *WithdrawalRequestsRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &WithdrawalRequestsRequest{TimeDelta: maxWithdrawalRequestsTimeDelta + time.Second}, err: errTimeDeltaTooLarge},
		{req: &WithdrawalRequestsRequest{TimeDelta: -time.Second}, err: errTimeDeltaTooLarge},
		{req: &WithdrawalRequestsRequest{Limit: 1001}, err: errInvalidLimit},
	} {
		_, err := e.GetWithdrawalRequests(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetWithdrawalRequests should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	requests, err := e.GetWithdrawalRequests(t.Context(), &WithdrawalRequestsRequest{TimeDelta: 24 * time.Hour, Limit: 10})
	require.NoError(t, err, "GetWithdrawalRequests must not error")
	if mockTests {
		require.Len(t, requests, 1, "GetWithdrawalRequests must return the requests")
		exp := WithdrawalRequestResponse{
			ID:            1,
			DateTime:      types.DateTime(time.Date(2022, 1, 31, 16, 7, 32, 0, time.UTC)),
			Type:          1,
			Currency:      currency.BTC,
			Network:       "bitcoin",
			Amount:        0.00006,
			Status:        2,
			TxID:          1,
			Address:       core.BitcoinDonationAddress,
			TransactionID: "NsOeFbQhRnpGzNIThWGBTkQwRJqTNOGPVhYavrVyMfkAyMUmIlUpFIwGTzSvpeOP",
		}
		assert.Equal(t, exp, requests[0], "GetWithdrawalRequests should return the request")
	}
}

func TestOpenBankWithdrawal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *BankWithdrawalRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &BankWithdrawalRequest{}, err: order.ErrAmountIsInvalid},
		{req: &BankWithdrawalRequest{Amount: 1}, err: currency.ErrCurrencyCodeEmpty},
		{req: &BankWithdrawalRequest{Amount: 1, AccountCurrency: currency.EUR}, err: errBankDetailsRequired},
	} {
		_, err := e.OpenBankWithdrawal(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "OpenBankWithdrawal should return the correct error")
	}
	// Covered with valid requests by TestWithdrawFiatFunds and TestWithdrawFiatFundsToInternationalBank
}

func TestCancelWithdrawal(t *testing.T) {
	t.Parallel()
	_, err := e.CancelWithdrawal(t.Context(), 0)
	assert.ErrorIs(t, err, errWithdrawalIDRequired, "CancelWithdrawal should error without an ID")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.CancelWithdrawal(t.Context(), 1)
	if !mockTests {
		assert.ErrorIs(t, err, errAPIResponse, "CancelWithdrawal should error for an unknown withdrawal")
		return
	}
	require.NoError(t, err, "CancelWithdrawal must not error")
	exp := &CancelWithdrawalResponse{ID: 1, Amount: 100, Currency: currency.EUR, AccountCurrency: currency.EUR, Type: "EU bank transfer (SEPA)"}
	assert.Equal(t, exp, resp, "CancelWithdrawal should return the cancelled withdrawal")
}

func TestGetFiatWithdrawalStatus(t *testing.T) {
	t.Parallel()
	_, err := e.GetFiatWithdrawalStatus(t.Context(), 0)
	assert.ErrorIs(t, err, errWithdrawalIDRequired, "GetFiatWithdrawalStatus should error without an ID")

	skipIfLiveWithoutCredentials(t)
	resp, err := e.GetFiatWithdrawalStatus(t.Context(), 1)
	if !mockTests {
		assert.ErrorIs(t, err, errAPIResponse, "GetFiatWithdrawalStatus should error for an unknown withdrawal")
		return
	}
	require.NoError(t, err, "GetFiatWithdrawalStatus must not error")
	assert.Equal(t, "Waiting to be processed", resp.Status, "Status should be correct")
}

func TestCryptoWithdrawal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *CryptoWithdrawalRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &CryptoWithdrawalRequest{}, err: currency.ErrCurrencyCodeEmpty},
		{req: &CryptoWithdrawalRequest{Currency: currency.BTC}, err: order.ErrAmountIsInvalid},
		{req: &CryptoWithdrawalRequest{Currency: currency.BTC, Amount: 1}, err: common.ErrAddressIsEmptyOrInvalid},
	} {
		_, err := e.CryptoWithdrawal(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "CryptoWithdrawal should return the correct error")
	}

	if !mockTests {
		t.Skip("CryptoWithdrawal is not tested live to avoid withdrawing funds")
	}
	resp, err := e.CryptoWithdrawal(t.Context(), &CryptoWithdrawalRequest{
		Currency:              currency.BTC,
		Amount:                0.002,
		Address:               core.BitcoinDonationAddress,
		OriginatorInfo:        &CustomerInfo{CorporateInfo: &CorporateInfo{CompanyName: "Good company", LEI: "5493001KJTIIGC8Y1R12"}},
		BeneficiaryInfo:       &CustomerInfo{RetailInfo: &RetailInfo{FirstName: "John", LastName: "Doe"}},
		BeneficiaryThirdParty: true,
		VASPUUID:              "48feffe5-a5d0-44fe-bf4c-734e7d721249",
	})
	require.NoError(t, err, "CryptoWithdrawal must not error")
	assert.Equal(t, uint64(3), resp.ID, "ID should be correct")
}

func TestRippleIOUWithdrawal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *RippleIOUWithdrawalRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &RippleIOUWithdrawalRequest{}, err: currency.ErrCurrencyCodeEmpty},
		{req: &RippleIOUWithdrawalRequest{Currency: currency.USD}, err: order.ErrAmountIsInvalid},
		{req: &RippleIOUWithdrawalRequest{Currency: currency.USD, Amount: 1}, err: common.ErrAddressIsEmptyOrInvalid},
	} {
		_, err := e.RippleIOUWithdrawal(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "RippleIOUWithdrawal should return the correct error")
	}

	if !mockTests {
		t.Skip("RippleIOUWithdrawal is not tested live to avoid withdrawing funds")
	}
	resp, err := e.RippleIOUWithdrawal(t.Context(), &RippleIOUWithdrawalRequest{Currency: currency.USD, Amount: 123, Address: "rvYAfWj5gh67oV6fW32ZzP3Aw4Eubs59B"})
	require.NoError(t, err, "RippleIOUWithdrawal must not error")
	assert.Equal(t, uint64(2), resp.ID, "ID should be correct")
}

func TestGetCryptoDepositAddress(t *testing.T) {
	t.Parallel()
	_, err := e.GetCryptoDepositAddress(t.Context(), currency.EMPTYCODE, "")
	assert.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "GetCryptoDepositAddress should error on an empty currency")

	skipIfLiveWithoutCredentials(t)
	for _, tc := range []struct {
		c       currency.Code
		network string
		exp     DepositAddressResponse
	}{
		{c: currency.BTC, exp: DepositAddressResponse{Address: core.BitcoinDonationAddress}},
		{c: currency.ETH, network: "ethereum", exp: DepositAddressResponse{Address: "0x6a56f5b80f04b4fd70d64d72e1396698635e5436"}},
		{c: currency.XRP, exp: DepositAddressResponse{Address: "rvYAfWj5gh67oV6fW32ZzP3Aw4Eubs59B", DestinationTag: 89473951}},
	} {
		addr, err := e.GetCryptoDepositAddress(t.Context(), tc.c, tc.network)
		require.NoErrorf(t, err, "GetCryptoDepositAddress must not error for %s", tc.c)
		if mockTests {
			assert.Equalf(t, tc.exp, *addr, "GetCryptoDepositAddress should return the address for %s", tc.c)
		}
	}
}

func TestGetUnconfirmedBitcoinDeposits(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	deposits, err := e.GetUnconfirmedBitcoinDeposits(t.Context())
	require.NoError(t, err, "GetUnconfirmedBitcoinDeposits must not error")
	if mockTests {
		require.Len(t, deposits, 1, "GetUnconfirmedBitcoinDeposits must return the deposits")
		assert.Equal(t, UnconfirmedDepositResponse{Amount: 0.001, Address: core.BitcoinDonationAddress, Confirmations: 1}, deposits[0], "deposit should be correct")
	}
}

func TestGetRippleIOUDepositAddress(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	addr, err := e.GetRippleIOUDepositAddress(t.Context())
	require.NoError(t, err, "GetRippleIOUDepositAddress must not error")
	if mockTests {
		assert.Equal(t, &RippleIOUDepositAddressResponse{Address: "rvYAfWj5gh67oV6fW32ZzP3Aw4Eubs59B", DestinationTag: 89473951}, addr, "address should be correct")
	}
}

func TestTransferToMainAccount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *TransferRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &TransferRequest{}, err: order.ErrAmountIsInvalid},
		{req: &TransferRequest{Amount: 1}, err: currency.ErrCurrencyCodeEmpty},
	} {
		assert.ErrorIs(t, e.TransferToMainAccount(t.Context(), tc.req), tc.err, "TransferToMainAccount should return the correct error")
	}

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	err := e.TransferToMainAccount(t.Context(), &TransferRequest{Amount: 10000, Currency: currency.BTC, SubAccount: 1234567})
	assert.ErrorIs(t, err, errAPIResponse, "TransferToMainAccount should error for an unknown sub account")
	assert.ErrorContains(t, err, `Sub account with identifier "1234567" does not exist.`, "error should contain the reason")
	if mockTests {
		assert.NoError(t, e.TransferToMainAccount(t.Context(), &TransferRequest{Amount: 10, Currency: currency.BTC, SubAccount: 990129}), "TransferToMainAccount should not error")
	}
}

func TestTransferFromMainAccount(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, e.TransferFromMainAccount(t.Context(), &TransferRequest{Amount: 10, Currency: currency.BTC}), errSubAccountRequired, "TransferFromMainAccount should error without a sub account")

	if !mockTests {
		t.Skip("TransferFromMainAccount is not tested live to avoid transferring funds")
	}
	assert.NoError(t, e.TransferFromMainAccount(t.Context(), &TransferRequest{Amount: 10, Currency: currency.BTC, SubAccount: 990129}), "TransferFromMainAccount should not error")
}

func TestCreateInstantConvertAddress(t *testing.T) {
	t.Parallel()
	_, err := e.CreateInstantConvertAddress(t.Context(), currency.EMPTYCODE, "")
	assert.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "CreateInstantConvertAddress should error on an empty currency")

	if !mockTests {
		t.Skip("CreateInstantConvertAddress is not tested live to avoid creating addresses which sell deposits")
	}
	resp, err := e.CreateInstantConvertAddress(t.Context(), currency.USD, "BECH32")
	require.NoError(t, err, "CreateInstantConvertAddress must not error")
	assert.Equal(t, "3MDvHUAg41uJx1511gDotsf4ccKuc9frz1", resp.Address, "Address should be correct")
}

func TestGetInstantConvertAddressInfo(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	info, err := e.GetInstantConvertAddressInfo(t.Context(), "")
	require.NoError(t, err, "GetInstantConvertAddressInfo must not error")
	if mockTests {
		exp := []InstantConvertAddressInfoResponse{{
			Address:      "3MDvHUAg41uJx1511gDotsf4ccKuc9frz1",
			CurrencyPair: "BTC/USD",
			Transactions: []InstantConvertTransaction{{OrderID: 1, Count: 1, Trades: []InstantConvertTrade{{ExchangeRate: 84000, BTCAmount: 0.01}}}},
		}}
		assert.Equal(t, exp, info, "GetInstantConvertAddressInfo should return the address transactions")
	}
}

func TestGetWebsocketToken(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	token, err := e.GetWebsocketToken(t.Context())
	require.NoError(t, err, "GetWebsocketToken must not error")
	assert.Len(t, token.Token, 32, "Token should be 32 characters")
	assert.Positive(t, token.UserID, "UserID should be positive")
	assert.Positive(t, token.ValidSeconds, "ValidSeconds should be positive")
}

func TestGetUserTransactions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *UserTransactionsRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &UserTransactionsRequest{Limit: 1001}, err: errInvalidLimit},
		{req: &UserTransactionsRequest{Since: time.Unix(2, 0), Until: time.Unix(1, 0)}, err: common.ErrStartAfterEnd},
	} {
		_, err := e.GetUserTransactions(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetUserTransactions should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	transactions, err := e.GetUserTransactions(t.Context(), &UserTransactionsRequest{Pair: spotPair, Limit: 10, Sort: SortDescending})
	require.NoError(t, err, "GetUserTransactions must not error")
	if mockTests {
		require.Len(t, transactions, 2, "GetUserTransactions must return the transactions")
		exp := UserTransactionResponse{
			ID:            50197021,
			DateTime:      time.Date(2022, 3, 1, 10, 55, 53, 0, time.UTC),
			Type:          TransactionTypeMarketTrade,
			Fee:           8.04,
			OrderID:       1458532827766784,
			Amounts:       map[currency.Code]float64{currency.USD: -2010, currency.BTC: 0.1, currency.EUR: 0},
			ExchangeRates: map[currency.Pair]float64{spotPair: 20100},
		}
		assert.Equal(t, exp, transactions[1], "GetUserTransactions should return the transaction")
	}
}

func TestGetCryptoTransactions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *CryptoTransactionsRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &CryptoTransactionsRequest{Limit: 1001}, err: errInvalidLimit},
		{req: &CryptoTransactionsRequest{Since: time.Unix(2, 0), Until: time.Unix(1, 0)}, err: common.ErrStartAfterEnd},
	} {
		_, err := e.GetCryptoTransactions(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetCryptoTransactions should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	txs, err := e.GetCryptoTransactions(t.Context(), &CryptoTransactionsRequest{Limit: 10, IncludeIOUs: true})
	require.NoError(t, err, "GetCryptoTransactions must not error")
	if mockTests {
		require.Len(t, txs.Deposits, 1, "GetCryptoTransactions must return the deposits")
		require.Len(t, txs.Withdrawals, 1, "GetCryptoTransactions must return the withdrawals")
		assert.Equal(t, "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", txs.Deposits[0].DestinationAddress, "DestinationAddress should be correct")
		assert.Equal(t, "PENDING", txs.Deposits[0].Status, "Status should be correct")
		exp := CryptoTransaction{
			Currency:           currency.BTC,
			Network:            "bitcoin",
			DestinationAddress: "3FiKkjgZ6Sj4RWp3ZsCjYh5Pt7ZCBsL7uF",
			Amount:             0.00012,
			DateTime:           types.Time(time.Unix(1642665114, 0)),
		}
		assert.Equal(t, exp, txs.Withdrawals[0], "withdrawal should be correct")
	}
}

func TestGetCryptoDeposits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *CryptoDepositsRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &CryptoDepositsRequest{Limit: 1001}, err: errInvalidLimit},
		{req: &CryptoDepositsRequest{Since: time.Unix(2, 0), Until: time.Unix(1, 0)}, err: common.ErrStartAfterEnd},
	} {
		_, err := e.GetCryptoDeposits(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetCryptoDeposits should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	deposits, err := e.GetCryptoDeposits(t.Context(), &CryptoDepositsRequest{Limit: 10, Status: "PENDING"})
	require.NoError(t, err, "GetCryptoDeposits must not error")
	if mockTests {
		require.Len(t, deposits, 1, "GetCryptoDeposits must return the deposits")
		exp := CryptoDepositResponse{
			ID:                 1,
			Network:            "bitcoin",
			Currency:           currency.BTC,
			DestinationAddress: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			OriginatorAddress:  "1htZQJS6YUUPIGNNPC9425xENXQS2JwM7C",
			TxID:               "e4123d1d57df4106aaae5ec4d77eb6cd42e226d020a0a2c1c7919d14b93",
			Amount:             1.23,
			DateTime:           types.Time(time.Unix(1759995000, 0)),
			Status:             "PENDING",
			PendingReason:      "ADDRESS_VERIFICATION_NEEDED",
		}
		assert.Equal(t, exp, deposits[0], "GetCryptoDeposits should return the deposit")
	}
}

func TestUpdateCryptoDepositOriginator(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, e.UpdateCryptoDepositOriginator(t.Context(), 0, &DepositOriginatorRequest{}), errDepositIDRequired, "UpdateCryptoDepositOriginator should error without a deposit ID")
	assert.ErrorIs(t, e.UpdateCryptoDepositOriginator(t.Context(), 1, nil), common.ErrNilPointer, "UpdateCryptoDepositOriginator should error on a nil request")

	if !mockTests {
		t.Skip("UpdateCryptoDepositOriginator is not tested live as it requires a pending deposit")
	}
	err := e.UpdateCryptoDepositOriginator(t.Context(), 1, &DepositOriginatorRequest{
		OriginatorThirdParty: true,
		OriginatorInfo:       &CustomerInfo{RetailInfo: &RetailInfo{FirstName: "John", LastName: "Doe"}},
	})
	assert.NoError(t, err, "UpdateCryptoDepositOriginator should not error")
}

func TestRejectCryptoDeposit(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, e.RejectCryptoDeposit(t.Context(), 0), errDepositIDRequired, "RejectCryptoDeposit should error without a deposit ID")

	if !mockTests {
		t.Skip("RejectCryptoDeposit is not tested live as it requires a pending deposit")
	}
	assert.NoError(t, e.RejectCryptoDeposit(t.Context(), 1), "RejectCryptoDeposit should not error")
}

var testContact = ContactResponse{
	ID:          "258b8c23-48c0-4916-bba9-0dd47bcdd7cf",
	Description: "Cold wallet owner",
	RetailInfo:  &RetailInfo{FirstName: "John", LastName: "Doe", DateOfBirth: "1990-01-31", Country: "AU"},
}

func TestGetContact(t *testing.T) {
	t.Parallel()
	_, err := e.GetContact(t.Context(), "")
	assert.ErrorIs(t, err, errContactIDRequired, "GetContact should error without a contact ID")

	skipIfLiveWithoutCredentials(t)
	contact, err := e.GetContact(t.Context(), testContact.ID)
	if !mockTests {
		assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "GetContact should error for an unknown contact")
		return
	}
	require.NoError(t, err, "GetContact must not error")
	assert.Equal(t, &testContact, contact, "GetContact should return the contact")
}

func TestGetContacts(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	contacts, err := e.GetContacts(t.Context(), 1, 10)
	require.NoError(t, err, "GetContacts must not error")
	if mockTests {
		assert.Equal(t, []ContactResponse{testContact}, contacts, "GetContacts should return the contacts")
	}
}

func TestCreateContact(t *testing.T) {
	t.Parallel()
	_, err := e.CreateContact(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "CreateContact should error on a nil request")
	_, err = e.CreateContact(t.Context(), &ContactRequest{})
	assert.ErrorIs(t, err, errContactInfoRequired, "CreateContact should error without contact information")

	if !mockTests {
		t.Skip("CreateContact is not tested live to avoid creating contacts")
	}
	contact, err := e.CreateContact(t.Context(), &ContactRequest{RetailInfo: testContact.RetailInfo, Description: testContact.Description})
	require.NoError(t, err, "CreateContact must not error")
	assert.Equal(t, &testContact, contact, "CreateContact should return the contact")
}

func TestSubmitCounterpartyInfo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *CounterpartyAddressRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &CounterpartyAddressRequest{}, err: common.ErrAddressIsEmptyOrInvalid},
		{req: &CounterpartyAddressRequest{Address: core.BitcoinDonationAddress}, err: errNetworkRequired},
	} {
		_, err := e.SubmitCounterpartyInfo(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "SubmitCounterpartyInfo should return the correct error")
	}

	if !mockTests {
		t.Skip("SubmitCounterpartyInfo is not tested live to avoid registering addresses")
	}
	resp, err := e.SubmitCounterpartyInfo(t.Context(), &CounterpartyAddressRequest{
		Address:           core.BitcoinDonationAddress,
		Network:           "bitcoin",
		ContactThirdParty: true,
		ContactUUID:       testContact.ID,
		VASPUUID:          "48feffe5-a5d0-44fe-bf4c-734e7d721249",
	})
	require.NoError(t, err, "SubmitCounterpartyInfo must not error")
	exp := &CounterpartyAddressResponse{
		Address:           core.BitcoinDonationAddress,
		Network:           "bitcoin",
		ContactThirdParty: true,
		ContactUUID:       testContact.ID,
		VASPUUID:          "48feffe5-a5d0-44fe-bf4c-734e7d721249",
	}
	assert.Equal(t, exp, resp, "SubmitCounterpartyInfo should return the address")
}

var testSatoshiTest = SatoshiTestResponse{
	ID:             "6f1c2a4e-7b3d-4e5f-9a8b-1c2d3e4f5a6b",
	Network:        "bitcoin",
	Currency:       currency.BTC,
	Amount:         0.00001234,
	UserAddress:    core.BitcoinDonationAddress,
	DepositAddress: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
	Status:         "pending",
	Expires:        types.Time(time.Unix(1759995000, 0)),
}

func TestGetSatoshiTests(t *testing.T) {
	t.Parallel()
	_, err := e.GetSatoshiTests(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetSatoshiTests should error on a nil request")

	skipIfLiveWithoutCredentials(t)
	tests, err := e.GetSatoshiTests(t.Context(), &SatoshiTestsRequest{Network: "bitcoin", Status: "pending"})
	require.NoError(t, err, "GetSatoshiTests must not error")
	if mockTests {
		assert.Equal(t, []SatoshiTestResponse{testSatoshiTest}, tests, "GetSatoshiTests should return the tests")
	}
}

func TestCreateSatoshiTest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *SatoshiTestRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &SatoshiTestRequest{}, err: common.ErrAddressIsEmptyOrInvalid},
		{req: &SatoshiTestRequest{Address: core.BitcoinDonationAddress}, err: errNetworkRequired},
		{req: &SatoshiTestRequest{Address: core.BitcoinDonationAddress, Network: "bitcoin"}, err: currency.ErrCurrencyCodeEmpty},
	} {
		_, err := e.CreateSatoshiTest(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "CreateSatoshiTest should return the correct error")
	}

	if !mockTests {
		t.Skip("CreateSatoshiTest is not tested live to avoid creating tests")
	}
	resp, err := e.CreateSatoshiTest(t.Context(), &SatoshiTestRequest{Address: core.BitcoinDonationAddress, Network: "bitcoin", Currency: currency.BTC})
	require.NoError(t, err, "CreateSatoshiTest must not error")
	assert.Equal(t, &testSatoshiTest, resp, "CreateSatoshiTest should return the test")
}

func TestGetSatoshiTest(t *testing.T) {
	t.Parallel()
	_, err := e.GetSatoshiTest(t.Context(), "")
	assert.ErrorIs(t, err, errSatoshiTestIDRequired, "GetSatoshiTest should error without an ID")

	skipIfLiveWithoutCredentials(t)
	resp, err := e.GetSatoshiTest(t.Context(), testSatoshiTest.ID)
	if !mockTests {
		assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "GetSatoshiTest should error for an unknown test")
		return
	}
	require.NoError(t, err, "GetSatoshiTest must not error")
	assert.Equal(t, &testSatoshiTest, resp, "GetSatoshiTest should return the test")
}

const testXpubRegistrationID = "9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b"

func TestGetXpubRegistrations(t *testing.T) {
	t.Parallel()
	_, err := e.GetXpubRegistrations(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetXpubRegistrations should error on a nil request")
	_, err = e.GetXpubRegistrations(t.Context(), &XpubRegistrationsRequest{Limit: 1001})
	assert.ErrorIs(t, err, errInvalidLimit, "GetXpubRegistrations should error on an invalid limit")

	skipIfLiveWithoutCredentials(t)
	registrations, err := e.GetXpubRegistrations(t.Context(), &XpubRegistrationsRequest{Network: "bitcoin", Limit: 10})
	require.NoError(t, err, "GetXpubRegistrations must not error")
	if mockTests {
		require.Len(t, registrations, 1, "GetXpubRegistrations must return the registrations")
		exp := XpubRegistrationResponse{
			ID:        testXpubRegistrationID,
			Network:   "bitcoin",
			Label:     "btc cold wallet",
			Status:    "PENDING",
			Proof:     XpubRegistrationProof{ID: testSatoshiTest.ID, Method: "satoshi_test"},
			CreatedAt: time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
		}
		assert.Equal(t, exp, registrations[0], "GetXpubRegistrations should return the registration")
	}
}

func TestCreateXpubRegistration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *XpubRegistrationRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &XpubRegistrationRequest{}, err: errNetworkRequired},
		{req: &XpubRegistrationRequest{Network: "bitcoin"}, err: errPublicKeyRequired},
	} {
		_, err := e.CreateXpubRegistration(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "CreateXpubRegistration should return the correct error")
	}

	if !mockTests {
		t.Skip("CreateXpubRegistration is not tested live to avoid registering keys")
	}
	resp, err := e.CreateXpubRegistration(t.Context(), &XpubRegistrationRequest{
		Network:           "bitcoin",
		ExtendedPublicKey: "xpub6CUGRUonZSQ4TWtTMmzXdrXDtypWKiKrhko4egpiMZbpiaQL2jkwSB1icqYh2cfDfVxdx4df189oLKnC5fSwqPfgyP3hooxujYzAu3fDVmz",
		Label:             "btc cold wallet",
	})
	require.NoError(t, err, "CreateXpubRegistration must not error")
	assert.Equal(t, testXpubRegistrationID, resp.ID, "ID should be correct")
	assert.Equal(t, "PENDING", resp.Status, "Status should be correct")
}

func TestRevokeXpubRegistration(t *testing.T) {
	t.Parallel()
	_, err := e.RevokeXpubRegistration(t.Context(), "")
	assert.ErrorIs(t, err, errRegistrationIDRequired, "RevokeXpubRegistration should error without an ID")

	if !mockTests {
		t.Skip("RevokeXpubRegistration is not tested live to avoid revoking registrations")
	}
	resp, err := e.RevokeXpubRegistration(t.Context(), testXpubRegistrationID)
	require.NoError(t, err, "RevokeXpubRegistration must not error")
	assert.Equal(t, &XpubRevocationResponse{ID: testXpubRegistrationID, Status: "REVOKED"}, resp, "RevokeXpubRegistration should return the revocation")
}

func TestGetXpubRegistration(t *testing.T) {
	t.Parallel()
	_, err := e.GetXpubRegistration(t.Context(), "")
	assert.ErrorIs(t, err, errRegistrationIDRequired, "GetXpubRegistration should error without an ID")

	skipIfLiveWithoutCredentials(t)
	resp, err := e.GetXpubRegistration(t.Context(), testXpubRegistrationID)
	if !mockTests {
		assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "GetXpubRegistration should error for an unknown registration")
		return
	}
	require.NoError(t, err, "GetXpubRegistration must not error")
	assert.Equal(t, "ACTIVE", resp.Status, "Status should be correct")
	assert.Equal(t, time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC), resp.ActivatedAt, "ActivatedAt should be correct")
	assert.Zero(t, resp.RevokedAt, "RevokedAt should be zero")
}

func TestGetAddressVerification(t *testing.T) {
	t.Parallel()
	_, err := e.GetAddressVerification(t.Context(), "", core.BitcoinDonationAddress)
	assert.ErrorIs(t, err, errNetworkRequired, "GetAddressVerification should error without a network")
	_, err = e.GetAddressVerification(t.Context(), "bitcoin", "")
	assert.ErrorIs(t, err, common.ErrAddressIsEmptyOrInvalid, "GetAddressVerification should error without an address")

	skipIfLiveWithoutCredentials(t)
	resp, err := e.GetAddressVerification(t.Context(), "bitcoin", core.BitcoinDonationAddress)
	require.NoError(t, err, "GetAddressVerification must not error")
	if mockTests {
		exp := &AddressVerificationResponse{
			Network:    "bitcoin",
			Address:    core.BitcoinDonationAddress,
			Verified:   true,
			Method:     "satoshi_test",
			VerifiedAt: time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC),
		}
		assert.Equal(t, exp, resp, "GetAddressVerification should return the verification")
	}
}

func TestEarnSubscriptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *EarnSubscriptionRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &EarnSubscriptionRequest{}, err: currency.ErrCurrencyCodeEmpty},
		{req: &EarnSubscriptionRequest{Currency: currency.ETH}, err: errEarnTypeRequired},
		{req: &EarnSubscriptionRequest{Currency: currency.ETH, EarnType: EarnTypeStaking}, err: errEarnTermRequired},
		{req: &EarnSubscriptionRequest{Currency: currency.ETH, EarnType: EarnTypeStaking, EarnTerm: EarnTermFlexible}, err: order.ErrAmountIsInvalid},
	} {
		assert.ErrorIs(t, e.EarnSubscribe(t.Context(), tc.req), tc.err, "EarnSubscribe should return the correct error")
		assert.ErrorIs(t, e.EarnUnsubscribe(t.Context(), tc.req), tc.err, "EarnUnsubscribe should return the correct error")
	}

	if !mockTests {
		t.Skip("Earn subscriptions are not tested live to avoid moving funds")
	}
	req := &EarnSubscriptionRequest{Currency: currency.ETH, EarnType: EarnTypeStaking, EarnTerm: EarnTermFlexible, Amount: 10}
	assert.NoError(t, e.EarnSubscribe(t.Context(), req), "EarnSubscribe should not error")
	assert.NoError(t, e.EarnUnsubscribe(t.Context(), req), "EarnUnsubscribe should not error")
}

func TestSetEarnSubscriptionSetting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *EarnSubscriptionSettingRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &EarnSubscriptionSettingRequest{}, err: errEarnSettingRequired},
		{req: &EarnSubscriptionSettingRequest{Setting: EarnSettingOptIn}, err: currency.ErrCurrencyCodeEmpty},
		{req: &EarnSubscriptionSettingRequest{Setting: EarnSettingOptIn, Currency: currency.ADA}, err: errEarnTypeRequired},
	} {
		assert.ErrorIs(t, e.SetEarnSubscriptionSetting(t.Context(), tc.req), tc.err, "SetEarnSubscriptionSetting should return the correct error")
	}

	if !mockTests {
		t.Skip("SetEarnSubscriptionSetting is not tested live to avoid changing settings")
	}
	err := e.SetEarnSubscriptionSetting(t.Context(), &EarnSubscriptionSettingRequest{Setting: EarnSettingOptIn, Currency: currency.ADA, EarnType: EarnTypeStaking})
	assert.NoError(t, err, "SetEarnSubscriptionSetting should not error")
}

func TestGetEarnTransactions(t *testing.T) {
	t.Parallel()
	_, err := e.GetEarnTransactions(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetEarnTransactions should error on a nil request")
	_, err = e.GetEarnTransactions(t.Context(), &EarnTransactionsRequest{Limit: 1001})
	assert.ErrorIs(t, err, errInvalidLimit, "GetEarnTransactions should error on an invalid limit")

	skipIfLiveWithoutCredentials(t)
	txs, err := e.GetEarnTransactions(t.Context(), &EarnTransactionsRequest{Currency: currency.ETH, Limit: 10})
	require.NoError(t, err, "GetEarnTransactions must not error")
	if mockTests {
		exp := []EarnTransactionResponse{{
			DateTime:      types.DateTime(time.Date(2022, 1, 31, 14, 43, 15, 796000000, time.UTC)),
			Type:          "SUBSCRIBE",
			Amount:        10,
			Currency:      currency.ETH,
			Value:         20000,
			QuoteCurrency: currency.USD,
			Status:        "COMPLETED",
		}}
		assert.Equal(t, exp, txs, "GetEarnTransactions should return the transactions")
	}
}

func TestGetEarnSubscriptions(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	subs, err := e.GetEarnSubscriptions(t.Context())
	require.NoError(t, err, "GetEarnSubscriptions must not error")
	if mockTests {
		exp := []EarnSubscriptionResponse{{
			Currency:             currency.ETH,
			Type:                 EarnTypeStaking,
			Term:                 EarnTermFlexible,
			EstimatedAnnualYield: 3.5,
			DistributionPeriod:   "WEEKLY",
			ActivationPeriod:     "24 hours",
			Amount:               10,
			AvailableAmount:      9.5,
			AmountEarned:         0.12,
		}}
		assert.Equal(t, exp, subs, "GetEarnSubscriptions should return the subscriptions")
	}
}

func TestRevokeAllAPIKeys(t *testing.T) {
	t.Parallel()
	if !mockTests {
		t.Skip("RevokeAllAPIKeys is never tested live as it revokes every API key")
	}
	resp, err := e.RevokeAllAPIKeys(t.Context())
	require.NoError(t, err, "RevokeAllAPIKeys must not error")
	assert.Len(t, resp.RevokedAPIKeys, 2, "RevokedAPIKeys should contain the revoked keys")
}

func TestGetMarginTiers(t *testing.T) {
	t.Parallel()
	tiers, err := e.GetMarginTiers(t.Context())
	require.NoError(t, err, "GetMarginTiers must not error")
	require.NotEmpty(t, tiers, "GetMarginTiers must return tiers")
	for _, m := range tiers {
		assert.NotEmpty(t, m.Market, "Market should be set")
		require.NotEmpty(t, m.Tiers, "Tiers must not be empty")
		assert.Equal(t, uint64(1), m.Tiers[0].Tier, "first tier should be tier 1")
		assert.Positive(t, m.Tiers[0].MaxLeverage.Float64(), "MaxLeverage should be positive")
		assert.Greater(t, m.Tiers[0].SizeLimitHigh.Float64(), m.Tiers[0].SizeLimitLow.Float64(), "SizeLimitHigh should exceed SizeLimitLow")
	}
}

func TestGetMarketHours(t *testing.T) {
	t.Parallel()
	hours, err := e.GetMarketHours(t.Context())
	require.NoError(t, err, "GetMarketHours must not error")
	require.NotEmpty(t, hours, "GetMarketHours must return schedules")
	for _, h := range hours {
		assert.NotEmpty(t, h.Market, "Market should be set")
		assert.NotEmpty(t, h.TradingHours, "TradingHours should be set")
		require.NotEmpty(t, h.ReferenceIndexPublishingHours.Schedule, "Schedule must not be empty")
		assert.True(t, h.ReferenceIndexPublishingHours.Schedule[0].End.After(h.ReferenceIndexPublishingHours.Schedule[0].Start), "schedule entries should end after they start")
	}

	_, err = e.GetMarketHoursForMarket(t.Context(), currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetMarketHoursForMarket should error on an empty pair")
	gold, err := e.GetMarketHoursForMarket(t.Context(), currency.NewPair(currency.NewCode("GOLD"), currency.NewCode("USD-PERP")))
	require.NoError(t, err, "GetMarketHoursForMarket must not error")
	assert.Equal(t, "GOLD/USD-PERP", gold.Market, "Market should be correct")
	assert.NotZero(t, gold.ReferenceIndexPublishingHours.NextOpen, "NextOpen should be set")
}

func TestGetOpenPositions(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	positions, err := e.GetOpenPositions(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err, "GetOpenPositions must not error")
	if mockTests {
		assert.Len(t, positions, 3, "GetOpenPositions should return all positions")
	}
	positions, err = e.GetOpenPositions(t.Context(), perpetualPair)
	require.NoError(t, err, "GetOpenPositions must not error for a market")
	if mockTests {
		require.Len(t, positions, 2, "GetOpenPositions must return the market's positions")
		p := &positions[1]
		assert.Equal(t, "1234567890", p.ID, "ID should be correct")
		assert.Equal(t, MarginModeIsolated, p.MarginMode, "MarginMode should be correct")
		assert.True(t, p.SettlementCurrency.Equal(currency.USD), "SettlementCurrency should be correct")
		assert.Equal(t, PositionSideShort, p.Side, "Side should be correct")
		assert.Equal(t, 0.01, p.Size.Float64(), "Size should be correct")
		assert.Equal(t, -2.5, p.PNLRealised.Float64(), "PNLRealised should be correct")
		assert.Zero(t, p.CumulativeLiquidationFees.Float64(), "null values should decode as zero")
		assert.Equal(t, 55000.0, p.EstimatedLiquidationPrice.Float64(), "EstimatedLiquidationPrice should be correct")
	}
}

func TestGetPositionStatus(t *testing.T) {
	t.Parallel()
	_, err := e.GetPositionStatus(t.Context(), "")
	assert.ErrorIs(t, err, errPositionIDRequired, "GetPositionStatus should error without a position ID")

	skipIfLiveWithoutCredentials(t)
	status, err := e.GetPositionStatus(t.Context(), "1234567890")
	if !mockTests {
		assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "GetPositionStatus should error for an unknown position")
		return
	}
	require.NoError(t, err, "GetPositionStatus must not error")
	assert.Equal(t, "1234567890", status.ID, "ID should be correct")
	assert.Equal(t, PositionStatusOpen, status.Status, "Status should be correct")
	assert.Equal(t, time.UnixMicro(1750068000000000), status.TimeOpened.Time(), "TimeOpened should be correct")
	assert.Zero(t, status.TimeClosed.Time(), "TimeClosed should be zero for an open position")
}

func TestGetPositionHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetPositionHistory(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetPositionHistory should error on a nil request")
	_, err = e.GetPositionHistory(t.Context(), &PositionHistoryRequest{Limit: 1001})
	assert.ErrorIs(t, err, errInvalidLimit, "GetPositionHistory should error on an invalid limit")

	skipIfLiveWithoutCredentials(t)
	for _, req := range []*PositionHistoryRequest{{Limit: 10, Sort: SortDescending}, {Pair: perpetualPair, SinceID: "1234567870"}} {
		history, err := e.GetPositionHistory(t.Context(), req)
		require.NoError(t, err, "GetPositionHistory must not error")
		if mockTests {
			require.Len(t, history, 1, "GetPositionHistory must return the positions")
			exp := PositionHistoryResponse{
				ID:                       "1234567880",
				Market:                   "BTC/USD-PERP",
				MarketType:               MarketTypePerpetual,
				MarginMode:               MarginModeIsolated,
				PNLCurrency:              currency.USD,
				EntryPrice:               80000,
				PNLPercentage:            5,
				PNLRealised:              40,
				PNLSettled:               40,
				Leverage:                 3,
				PNL:                      40,
				CumulativePricePNL:       42,
				CumulativeTradingFees:    1.6,
				CumulativeFunding:        -0.4,
				AmountDelta:              0.01,
				TimeOpened:               types.Time(time.UnixMicro(1750068000000000)),
				TimeClosed:               types.Time(time.UnixMicro(1750154400000000)),
				Status:                   PositionStatusSettled,
				ExitPrice:                84000,
				SettlementPrice:          84000,
				CumulativeSocialisedLoss: 0,
			}
			assert.Equal(t, exp, history[0], "GetPositionHistory should return the position")
		}
	}
}

func TestClosePositions(t *testing.T) {
	t.Parallel()
	_, err := e.ClosePositions(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "ClosePositions should error on a nil request")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.ClosePositions(t.Context(), &ClosePositionsRequest{Pair: perpetualPair, MarginMode: MarginModeIsolated})
	require.NoError(t, err, "ClosePositions must not error")
	if mockTests {
		require.Len(t, resp.Closed, 1, "ClosePositions must return the closed positions")
		assert.Empty(t, resp.Failed, "Failed should be empty")
		assert.Equal(t, 0.34, resp.Closed[0].ClosingFeeAmount.Float64(), "ClosingFeeAmount should be correct")
		assert.Equal(t, PositionStatusSettled, resp.Closed[0].Status, "Status should decode from the embedded position")
	}
}

func TestClosePosition(t *testing.T) {
	t.Parallel()
	_, err := e.ClosePosition(t.Context(), "")
	assert.ErrorIs(t, err, errPositionIDRequired, "ClosePosition should error without a position ID")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.ClosePosition(t.Context(), "1234567890")
	if !mockTests {
		assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "ClosePosition should error for an unknown position")
		return
	}
	require.NoError(t, err, "ClosePosition must not error")
	assert.Equal(t, "1234567890", resp.ID, "ID should be correct")
	assert.Equal(t, 0.34, resp.ClosingFeeAmount.Float64(), "ClosingFeeAmount should be correct")
}

func TestGetPositionSettlementTransactions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *SettlementTransactionsRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &SettlementTransactionsRequest{Limit: 1001}, err: errInvalidLimit},
		{req: &SettlementTransactionsRequest{Since: time.Unix(2, 0), Until: time.Unix(1, 0)}, err: common.ErrStartAfterEnd},
	} {
		_, err := e.GetPositionSettlementTransactions(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetPositionSettlementTransactions should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	for _, req := range []*SettlementTransactionsRequest{{Limit: 10}, {TransactionID: "123123"}} {
		txs, err := e.GetPositionSettlementTransactions(t.Context(), req)
		if req.TransactionID != "" && !mockTests {
			assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "GetPositionSettlementTransactions should error for an unknown transaction")
			continue
		}
		require.NoError(t, err, "GetPositionSettlementTransactions must not error")
		if mockTests {
			require.Len(t, txs, 1, "GetPositionSettlementTransactions must return the transactions")
			exp := SettlementTransactionResponse{
				TransactionID:            "123123",
				PositionID:               "1234567890",
				SettlementTime:           types.Time(time.Unix(1750089600, 0)),
				SettlementType:           "PERIODIC",
				SettlementPrice:          84000,
				Market:                   "BTC/USD-PERP",
				MarketType:               MarketTypePerpetual,
				PNLCurrency:              currency.USD,
				PNLSettled:               12.12,
				PNLComponentPrice:        12.5,
				PNLComponentFees:         -0.34,
				PNLComponentFunding:      -0.04,
				MarginMode:               MarginModeIsolated,
				Size:                     0.01,
				StrikePrice:              84000,
				FeesComponentTrading:     0.34,
				FeesComponentLiquidation: 0,
			}
			assert.Equal(t, exp, txs[0], "GetPositionSettlementTransactions should return the transaction")
		}
	}
}

func TestGetDerivativesTradeHistory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *DerivativesTradeHistoryRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &DerivativesTradeHistoryRequest{}, err: errPairOrOrderIDRequired},
		{req: &DerivativesTradeHistoryRequest{Pair: perpetualPair, Limit: 1001}, err: errInvalidLimit},
		{req: &DerivativesTradeHistoryRequest{Pair: perpetualPair, Since: time.Unix(2, 0), Until: time.Unix(1, 0)}, err: common.ErrStartAfterEnd},
	} {
		_, err := e.GetDerivativesTradeHistory(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetDerivativesTradeHistory should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	trades, err := e.GetDerivativesTradeHistory(t.Context(), &DerivativesTradeHistoryRequest{OrderID: 1234123412341235, Limit: 10, Sort: SortAscending})
	require.NoError(t, err, "GetDerivativesTradeHistory must not error")
	if mockTests {
		require.Len(t, trades, 2, "GetDerivativesTradeHistory must return the trades")
		exp := DerivativesTradeResponse{
			TradeID:     1234123412341300,
			OrderID:     1234123412341235,
			PositionID:  "1234567890",
			DateTime:    types.DateTime(time.Date(2025, 6, 16, 10, 1, 0, 849000000, time.UTC)),
			Fee:         0.1344,
			FeeCurrency: currency.USD,
			Market:      "BTC/USD-PERP",
			MarginMode:  MarginModeIsolated,
			Leverage:    3,
			Side:        "SELL",
			Type:        "TRADE",
			Price:       84000,
			Amount:      0.004,
			TradeUTI:    "4851007IRIW87EC5H6978f171f98ee8973c09fecb910a08d173f",
		}
		assert.Equal(t, exp, trades[0], "GetDerivativesTradeHistory should return the trade")
	}
}

func TestGetMarginInfo(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	info, err := e.GetMarginInfo(t.Context())
	require.NoError(t, err, "GetMarginInfo must not error")
	if mockTests {
		assert.Equal(t, 20000.0, info.AccountMargin.Float64(), "AccountMargin should be correct")
		assert.True(t, info.AccountMarginCurrency.Equal(currency.USD), "AccountMarginCurrency should be correct")
		require.Len(t, info.Assets, 2, "Assets must contain the collateral")
		assert.Equal(t, AssetMarginInfo{Asset: currency.BTC, TotalAmount: 0.1, Available: 0.09, Reserved: 0.01, MarginAvailable: 7560}, info.Assets[1], "asset should be correct")
		assert.Zero(t, info.ImpliedLeverage.Float64(), "null ImpliedLeverage should decode as zero")
	}
}

func TestGetEstimatedOrderImpact(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *OrderImpactRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &OrderImpactRequest{}, err: currency.ErrCurrencyPairEmpty},
		{req: &OrderImpactRequest{Pair: perpetualPair}, err: errOrderTypeRequired},
		{req: &OrderImpactRequest{Pair: perpetualPair, OrderType: OrderSubtypeMarket}, err: order.ErrAmountIsInvalid},
		{req: &OrderImpactRequest{Pair: perpetualPair, OrderType: OrderSubtypeMarket, Amount: 1}, err: order.ErrSideIsInvalid},
		{req: &OrderImpactRequest{Pair: perpetualPair, OrderType: OrderSubtypeMarket, Amount: 1, Side: order.Sell}, err: errMarginModeRequired},
		{req: &OrderImpactRequest{Pair: perpetualPair, OrderType: OrderSubtypeMarket, Amount: 1, Side: order.Sell, MarginMode: MarginModeCross}, err: errLeverageRequired},
	} {
		_, err := e.GetEstimatedOrderImpact(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetEstimatedOrderImpact should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	resp, err := e.GetEstimatedOrderImpact(t.Context(), &OrderImpactRequest{Pair: perpetualPair, OrderType: OrderSubtypeMarket, Amount: 0.01, Side: order.Sell, MarginMode: MarginModeCross, Leverage: 3, ReduceOnly: true})
	require.NoError(t, err, "GetEstimatedOrderImpact must not error")
	if mockTests {
		assert.Equal(t, "YES", resp.IsOrderPlaceable, "IsOrderPlaceable should be correct")
		assert.Equal(t, map[string]types.Number{"BTC/USD-PERP": 90000, "ETH/USD-PERP": 900}, resp.EstimatedLiquidationPrices, "EstimatedLiquidationPrices should be correct")
		assert.Equal(t, []MarginTierExposure{{Tier: 1, Exposure: 1000, Leverage: 5}}, resp.MarginTier.Breakdown, "MarginTier should be correct")
	}
}

func TestGetCollateralChangeImpact(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *CollateralChangeImpactRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &CollateralChangeImpactRequest{}, err: errMarginModeRequired},
		{req: &CollateralChangeImpactRequest{MarginMode: MarginModeCross}, err: errCollateralRequired},
		{req: &CollateralChangeImpactRequest{MarginMode: MarginModeCross, TargetCollateral: map[currency.Code]float64{currency.USD: 1}, CollateralDeltas: map[currency.Code]float64{currency.USD: 1}}, err: errCollateralRequired},
	} {
		_, err := e.GetCollateralChangeImpact(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetCollateralChangeImpact should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	resp, err := e.GetCollateralChangeImpact(t.Context(), &CollateralChangeImpactRequest{MarginMode: MarginModeCross, CollateralDeltas: map[currency.Code]float64{currency.BTC: -0.01, currency.USD: 120}})
	require.NoError(t, err, "GetCollateralChangeImpact must not error")
	if mockTests {
		exp := &CollateralChangeImpactResponse{
			MarginCurrency:                  currency.USD,
			EstimatedInitialMarginRatio:     50,
			EstimatedMaintenanceMarginRatio: 25,
			EstimatedLiquidationPrices:      map[string]types.Number{"1234567890": 9000},
			TotalEstimatedMargin:            1200.61,
		}
		assert.Equal(t, exp, resp, "GetCollateralChangeImpact should return the impact")
	}
}

func TestGetCollateralCurrencies(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	currencies, err := e.GetCollateralCurrencies(t.Context())
	require.NoError(t, err, "GetCollateralCurrencies must not error")
	if mockTests {
		assert.Equal(t, []CollateralCurrencyResponse{{Currency: currency.USD}, {Currency: currency.BTC, Haircut: 0.1}}, currencies, "GetCollateralCurrencies should return the currencies")
	}
}

func TestAdjustPositionCollateral(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, e.AdjustPositionCollateral(t.Context(), "", 1), errPositionIDRequired, "AdjustPositionCollateral should error without a position ID")
	assert.ErrorIs(t, e.AdjustPositionCollateral(t.Context(), "1234567890", 0), order.ErrAmountIsInvalid, "AdjustPositionCollateral should error without an amount")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	err := e.AdjustPositionCollateral(t.Context(), "1234567890", 12.5)
	if !mockTests {
		assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "AdjustPositionCollateral should error for an unknown position")
		return
	}
	assert.NoError(t, err, "AdjustPositionCollateral should not error")
}

func TestGetLeverageSettings(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	settings, err := e.GetLeverageSettings(t.Context(), "", currency.EMPTYPAIR)
	require.NoError(t, err, "GetLeverageSettings must not error")
	if mockTests {
		assert.Len(t, settings, 2, "GetLeverageSettings should return all settings")
	}
	settings, err = e.GetLeverageSettings(t.Context(), MarginModeCross, perpetualPair)
	require.NoError(t, err, "GetLeverageSettings must not error with filters")
	if mockTests {
		assert.Equal(t, []LeverageSettingResponse{{MarginMode: MarginModeCross, Market: "BTC/USD-PERP", LeverageCurrent: 3, LeverageMax: 10}}, settings, "GetLeverageSettings should return the filtered settings")
	}
}

func TestUpdateLeverageSetting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *LeverageSettingRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &LeverageSettingRequest{}, err: currency.ErrCurrencyPairEmpty},
		{req: &LeverageSettingRequest{Pair: perpetualPair}, err: errMarginModeRequired},
		{req: &LeverageSettingRequest{Pair: perpetualPair, MarginMode: MarginModeIsolated}, err: errLeverageRequired},
	} {
		_, err := e.UpdateLeverageSetting(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "UpdateLeverageSetting should return the correct error")
	}

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.UpdateLeverageSetting(t.Context(), &LeverageSettingRequest{Pair: perpetualPair, MarginMode: MarginModeIsolated, Leverage: 5})
	require.NoError(t, err, "UpdateLeverageSetting must not error")
	if mockTests {
		assert.Equal(t, &LeverageSettingResponse{MarginMode: MarginModeIsolated, Market: "BTC/USD-PERP", LeverageCurrent: 5, LeverageMax: 10}, resp, "UpdateLeverageSetting should return the setting")
	}
}
