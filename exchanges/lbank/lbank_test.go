package lbank

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5" //nolint:gosec // Used for this exchange
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

// Please supply your own keys here for due diligence testing
const (
	apiKey                  = ""
	apiSecret               = ""
	canManipulateRealOrders = false
)

var (
	e        *Exchange
	testPair = currency.NewBTCUSDT().Format(currency.PairFormat{Delimiter: "_"})
)

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Lbank Setup error: %s", err)
	}
	if apiKey != "" && apiSecret != "" {
		e.API.AuthenticatedSupport = true
		e.SetCredentials(apiKey, apiSecret, "", "", "", "")
	}
	os.Exit(m.Run())
}

func TestGetTicker(t *testing.T) {
	t.Parallel()
	_, err := e.GetTicker(t.Context(), testPair.String())
	assert.NoError(t, err, "GetTicker should not error")
}

func TestGetTimestamp(t *testing.T) {
	t.Parallel()
	ts, err := e.GetTimestamp(t.Context())
	require.NoError(t, err, "GetTimestamp must not error")
	assert.NotZero(t, ts, "GetTimestamp should return a non-zero time")
}

func TestGetTickers(t *testing.T) {
	t.Parallel()
	tickers, err := e.GetTickers(t.Context())
	require.NoError(t, err, "GetTickers must not error")
	assert.Greater(t, len(tickers), 1, "GetTickers should return more than 1 ticker")
}

func TestGetCurrencyPairs(t *testing.T) {
	t.Parallel()
	_, err := e.GetCurrencyPairs(t.Context())
	assert.NoError(t, err, "GetCurrencyPairs should not error")
}

func TestGetMarketDepths(t *testing.T) {
	t.Parallel()
	d, err := e.GetMarketDepths(t.Context(), testPair.String(), 4)
	require.NoError(t, err, "GetMarketDepths must not error")
	require.NotEmpty(t, d, "GetMarketDepths must return a non-empty response")
	assert.Len(t, d.Data.Asks, 4, "GetMarketDepths should return 4 asks")
}

func TestGetTrades(t *testing.T) {
	t.Parallel()
	r, err := e.GetTrades(t.Context(), testPair.String(), 420, time.Time{})
	require.NoError(t, err, "GetTrades must not error")
	require.NotEmpty(t, r, "GetTrades must return a non-empty response")
	assert.LessOrEqual(t, len(r), 420, "GetTrades should respect the requested trade limit")
}

func TestGetKlines(t *testing.T) {
	t.Parallel()
	_, err := e.GetKlines(t.Context(), testPair.String(), "600", "minute1", time.Now())
	assert.NoError(t, err, "GetKlines should not error")
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateOrderbook(t.Context(), currency.EMPTYPAIR, asset.Spot)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.UpdateOrderbook(t.Context(), testPair, asset.Options)
	assert.ErrorIs(t, err, asset.ErrNotSupported)
	_, err = e.UpdateOrderbook(t.Context(), testPair, asset.Spot)
	assert.NoError(t, err, "UpdateOrderbook should not error")
}

func TestGetUserInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetUserInfo(t.Context())
	require.NoError(t, err, "GetUserInfo must not error")
}

func TestCreateOrder(t *testing.T) {
	t.Parallel()

	_, err := e.CreateOrder(t.Context(), testPair.String(), "what", 1231, 12314)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)
	_, err = e.CreateOrder(t.Context(), testPair.String(), order.Buy.String(), 0, 0)
	require.ErrorIs(t, err, limits.ErrAmountBelowMin)
	_, err = e.CreateOrder(t.Context(), testPair.String(), order.Sell.String(), 1231, 0)
	require.ErrorIs(t, err, limits.ErrPriceBelowMin)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err = e.CreateOrder(t.Context(), testPair.String(), order.Buy.String(), 58, 681)
	assert.NoError(t, err, "CreateOrder should not error")
}

func TestRemoveOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.RemoveOrder(t.Context(), testPair.String(), "24f7ce27-af1d-4dca-a8c1-ef1cbeec1b23")
	assert.NoError(t, err, "RemoveOrder should not error")
}

func TestQueryOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.QueryOrder(t.Context(), testPair.String(), "1")
	assert.NoError(t, err, "QueryOrder should not error")
}

func TestQueryOrderHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.QueryOrderHistory(t.Context(), testPair.String(), "1", "100")
	assert.NoError(t, err, "QueryOrderHistory should not error")
}

func TestGetPairInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetPairInfo(t.Context())
	assert.NoError(t, err, "GetPairInfo should not error")
}

func TestOrderTransactionDetails(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.OrderTransactionDetails(t.Context(), testPair.String(), "24f7ce27-af1d-4dca-a8c1-ef1cbeec1b23")
	assert.NoError(t, err, "OrderTransactionDetails should not error")
}

func TestTransactionHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.TransactionHistory(t.Context(), &TransactionHistoryRequest{Symbol: testPair.String()})
	assert.NoError(t, err, "TransactionHistory should not error")
}

func TestGetOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetOpenOrders(t.Context(), testPair.String(), "1", "50")
	assert.NoError(t, err, "GetOpenOrders should not error")
}

func TestUSD2RMBRate(t *testing.T) {
	t.Parallel()
	_, err := e.USD2RMBRate(t.Context())
	assert.NoError(t, err, "USD2RMBRate should not error")
}

func TestGetWithdrawConfig(t *testing.T) {
	t.Parallel()
	c, err := e.GetWithdrawConfig(t.Context(), currency.ETH)
	require.NoError(t, err, "GetWithdrawConfig must not error")
	assert.NotEmpty(t, c)
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	_, err := e.Withdraw(t.Context(), &WithdrawRequest{})
	require.NoError(t, err, "Withdraw must not error")
}

func TestGetWithdrawRecords(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetWithdrawalRecords(t.Context(), &WithdrawalRecordsRequest{
		Coin:   currency.ETH,
		Status: "1",
	})
	assert.NoError(t, err, "GetWithdrawRecords should not error")
}

func newTestHTTPExchange(t *testing.T, handler http.HandlerFunc) *Exchange {
	t.Helper()

	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	require.NoError(t,
		ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL),
		"SetRunningURL must not error")
	return ex
}

func newTestAuthHTTPExchange(t *testing.T, handler http.HandlerFunc) (*Exchange, context.Context) {
	t.Helper()

	ex := newTestHTTPExchange(t, handler)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "GenerateKey must not error")
	ex.privateKey = key

	ctx := accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{
		Key:    "test-key",
		Secret: "test-secret",
	})
	return ex, ctx
}

func assertV2AuthRequest(t *testing.T, r *http.Request, path string, expected map[string]string) {
	t.Helper()

	assert.Equal(t, http.MethodPost, r.Method, "Authenticated requests should use POST")
	assert.Equal(t, path, r.URL.Path, "Authenticated requests should use the expected path")
	require.NoError(t, r.ParseForm(), "Authenticated requests must parse form data")

	assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"), "Authenticated requests should set the form content type")
	assert.Equal(t, "test-key", r.Form.Get("api_key"), "Authenticated requests should send the api key")
	assert.Equal(t, "RSA", r.Form.Get("signature_method"), "Authenticated v2 requests should send the signature method")
	assert.Equal(t, "RSA", r.Header.Get("signature_method"), "Authenticated v2 requests should mirror the signature method header")
	assert.NotEmpty(t, r.Form.Get("timestamp"), "Authenticated v2 requests should send a timestamp")
	assert.Equal(t, r.Form.Get("timestamp"), r.Header.Get("timestamp"), "Authenticated v2 requests should mirror the timestamp header")
	assert.Len(t, r.Form.Get("echostr"), 32, "Authenticated v2 requests should send a 32 character echostr")
	assert.Equal(t, r.Form.Get("echostr"), r.Header.Get("echostr"), "Authenticated v2 requests should mirror the echostr header")
	assert.NotEmpty(t, r.Form.Get("sign"), "Authenticated requests should send a signature")
	for key, value := range expected {
		assert.Equal(t, value, r.Form.Get(key), "Authenticated requests should send the expected form value")
	}
}

func assertV1AuthRequest(t *testing.T, r *http.Request, path string, expected map[string]string) {
	t.Helper()

	assert.Equal(t, http.MethodPost, r.Method, "Authenticated requests should use POST")
	assert.Equal(t, path, r.URL.Path, "Authenticated requests should use the expected path")
	require.NoError(t, r.ParseForm(), "Authenticated requests must parse form data")

	assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"), "Authenticated requests should set the form content type")
	assert.Equal(t, "test-key", r.Form.Get("api_key"), "Authenticated requests should send the api key")
	assert.Empty(t, r.Form.Get("signature_method"), "Authenticated v1 requests should not send the v2 signature method field")
	assert.Empty(t, r.Form.Get("timestamp"), "Authenticated v1 requests should not send the v2 timestamp field")
	assert.Empty(t, r.Form.Get("echostr"), "Authenticated v1 requests should not send the v2 echostr field")
	assert.NotEmpty(t, r.Form.Get("sign"), "Authenticated requests should send a signature")
	for key, value := range expected {
		assert.Equal(t, value, r.Form.Get(key), "Authenticated requests should send the expected form value")
	}
}

func TestCaptureResponseError(t *testing.T) {
	t.Parallel()

	err := captureResponseError(ErrCapture{
		Message: "Validation Failed",
		Error:   10002,
	})
	require.Error(t, err, "captureResponseError must return an error for exchange failures")
	assert.EqualError(t, err, "Validation Failed", "captureResponseError should prefer the exchange message when it matches")

	err = captureResponseError(ErrCapture{
		Message: "custom failure",
		Error:   19999,
	})
	require.Error(t, err, "captureResponseError must return an error for unknown exchange failures")
	assert.ErrorContains(t, err, "custom failure", "captureResponseError should include the exchange message")
	assert.ErrorContains(t, err, "19999", "captureResponseError should include the unmapped error code")
}

func TestCaptureResponseErrorSupportsCodeField(t *testing.T) {
	t.Parallel()

	err := captureResponseError(ErrCapture{Code: 10002})
	require.Error(t, err, "captureResponseError must return an error for v2 code based failures")
	assert.EqualError(t, err, "Validation Failed", "captureResponseError should decode the code field")
}

func TestGetTimestampReturnsV2ResponseError(t *testing.T) {
	t.Parallel()

	ex := newTestHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "GetTimestamp should use GET")
		assert.Equal(t, "/v2/timestamp.do", r.URL.Path, "GetTimestamp should use the v2 timestamp endpoint")
		_, err := w.Write([]byte(`{"msg":"Validation Failed","result":"false","data":0,"error_code":10002,"ts":1}`))
		assert.NoError(t, err, "Writing the timestamp response should not error")
	})

	_, err := ex.GetTimestamp(t.Context())
	require.Error(t, err, "GetTimestamp must return an error when LBank returns an error envelope")
	assert.EqualError(t, err, "Validation Failed", "GetTimestamp should surface the exchange message")
}

func TestGetTickerEndpointsUseV2(t *testing.T) {
	t.Parallel()

	call := 0
	ex := newTestHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		call++
		assert.Equal(t, http.MethodGet, r.Method, "Ticker requests should use GET")
		assert.Equal(t, "/v2/ticker/24hr.do", r.URL.Path, "Ticker requests should use the v2 24hr ticker endpoint")

		switch call {
		case 1:
			assert.Equal(t, "btc_usdt", r.URL.Query().Get("symbol"), "GetTicker should request the target symbol")
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[{"symbol":"btc_usdt","ticker":{"high":"76000.38","vol":"12976.9135","low":"73281.22","change":"1.13","turnover":"966280941.61","latest":"74116.9"},"timestamp":1772335196000}],"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the ticker response should not error")
		case 2:
			assert.Equal(t, "all", r.URL.Query().Get("symbol"), "GetTickers should request all symbols")
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[{"symbol":"btc_usdt","ticker":{"high":"76000.38","vol":"12976.9135","low":"73281.22","change":"1.13","turnover":"966280941.61","latest":"74116.9"},"timestamp":1772335196000},{"symbol":"eth_usdt","ticker":{"high":"4000","vol":"123.45","low":"3900","change":"0.75","turnover":"500000","latest":"3950.5"},"timestamp":1772335197000}],"error_code":0,"ts":1772335197000}`))
			assert.NoError(t, err, "Writing the ticker list response should not error")
		default:
			t.Fatalf("unexpected ticker request count: %d", call)
		}
	})

	tickerResp, err := ex.GetTicker(t.Context(), "btc_usdt")
	require.NoError(t, err, "GetTicker must not error for a valid v2 response")
	require.NotNil(t, tickerResp, "GetTicker must return a response")
	assert.Equal(t, currency.BTC.Lower().String(), tickerResp.Symbol.Base.Lower().String(), "GetTicker should decode the base currency")
	assert.Equal(t, currency.USDT.Lower().String(), tickerResp.Symbol.Quote.Lower().String(), "GetTicker should decode the quote currency")
	assert.Equal(t, 74116.9, tickerResp.Ticker.Latest, "GetTicker should decode the latest price")
	assert.Equal(t, 12976.9135, tickerResp.Ticker.Volume, "GetTicker should decode the volume")

	tickers, err := ex.GetTickers(t.Context())
	require.NoError(t, err, "GetTickers must not error for a valid v2 response")
	require.Len(t, tickers, 2, "GetTickers must decode both returned tickers")
	assert.Equal(t, currency.ETH.Lower().String(), tickers[1].Symbol.Base.Lower().String(), "GetTickers should decode the second base currency")
	assert.Equal(t, currency.USDT.Lower().String(), tickers[1].Symbol.Quote.Lower().String(), "GetTickers should decode the second quote currency")
	assert.Equal(t, 3950.5, tickers[1].Ticker.Latest, "GetTickers should decode string prices into float fields")
	assert.Equal(t, 2, call, "Ticker requests should hit the mock server twice")
}

func TestGetTickerReturnsErrorOnEmptyV2Response(t *testing.T) {
	t.Parallel()

	ex := newTestHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "GetTicker should use GET")
		assert.Equal(t, "/v2/ticker/24hr.do", r.URL.Path, "GetTicker should use the v2 24hr ticker endpoint")
		assert.Equal(t, "btc_usdt", r.URL.Query().Get("symbol"), "GetTicker should request the target symbol")
		_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[],"error_code":0,"ts":1772335196000}`))
		assert.NoError(t, err, "Writing the empty ticker response should not error")
	})

	_, err := ex.GetTicker(t.Context(), "btc_usdt")
	require.ErrorIs(t, err, errTickerDataUnavailable, "GetTicker must return errTickerDataUnavailable for an empty v2 response")
	assert.ErrorContains(t, err, "btc_usdt", "GetTicker should include the symbol in the empty-response error")
}

func TestGetMarketMetadataEndpointsUseV2(t *testing.T) {
	t.Parallel()

	ex := newTestHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "Public metadata requests should use GET")

		switch r.URL.Path {
		case "/v2/currencyPairs.do":
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":["btc_usdt","eth_usdt"],"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the currency pairs response should not error")
		case "/v2/accuracy.do":
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[{"symbol":"btc_usdt","quantityAccuracy":"5","minTranQua":"0.00001","priceAccuracy":"2"}],"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the pair info response should not error")
		default:
			t.Fatalf("unexpected metadata request path: %s", r.URL.Path)
		}
	})

	pairs, err := ex.GetCurrencyPairs(t.Context())
	require.NoError(t, err, "GetCurrencyPairs must not error for a valid v2 response")
	assert.Equal(t, []string{"btc_usdt", "eth_usdt"}, pairs, "GetCurrencyPairs should decode the response data")

	pairInfo, err := ex.GetPairInfo(t.Context())
	require.NoError(t, err, "GetPairInfo must not error for a valid v2 response")
	require.Len(t, pairInfo, 1, "GetPairInfo must decode the returned entry")
	assert.Equal(t, "btc_usdt", pairInfo[0].Symbol, "GetPairInfo should decode the symbol")
	assert.Equal(t, "0.00001", pairInfo[0].MinimumQuantity, "GetPairInfo should decode the minimum quantity")
}

func TestGetKlinesUsesV2ResponseEnvelope(t *testing.T) {
	t.Parallel()

	requestTime := time.Unix(1772334000, 0).UTC()
	ex := newTestHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "GetKlines should use GET")
		assert.Equal(t, "/v2/kline.do", r.URL.Path, "GetKlines should use the v2 kline endpoint")
		assert.Equal(t, "btc_usdt", r.URL.Query().Get("symbol"), "GetKlines should pass the symbol")
		assert.Equal(t, "2", r.URL.Query().Get("size"), "GetKlines should pass the size")
		assert.Equal(t, "minute1", r.URL.Query().Get("type"), "GetKlines should pass the interval type")
		assert.Equal(t, "1772334000", r.URL.Query().Get("time"), "GetKlines should pass the unix timestamp")
		_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[[1772334000,67600,67700,67500,67650,120.5],[1772334060,67650,67800,67620,67710,95.25]],"error_code":0,"ts":1772335196000}`))
		assert.NoError(t, err, "Writing the kline response should not error")
	})

	klines, err := ex.GetKlines(t.Context(), "btc_usdt", "2", "minute1", requestTime)
	require.NoError(t, err, "GetKlines must not error for a valid v2 response")
	require.Len(t, klines, 2, "GetKlines must decode both returned klines")
	assert.Equal(t, requestTime, klines[0].TimeStamp, "GetKlines should decode the first timestamp")
	assert.Equal(t, 67710.0, klines[1].ClosePrice, "GetKlines should decode the close price")
	assert.Equal(t, 95.25, klines[1].TradingVolume, "GetKlines should decode the trading volume")
}

func TestGetMarketDepthsUsesV2ResponseEnvelope(t *testing.T) {
	t.Parallel()

	ex := newTestHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "GetMarketDepths should use GET")
		assert.Equal(t, "/v2/depth.do", r.URL.Path, "GetMarketDepths should use the v2 depth endpoint")
		assert.Equal(t, "btc_usdt", r.URL.Query().Get("symbol"), "GetMarketDepths should pass the symbol")
		assert.Equal(t, "2", r.URL.Query().Get("size"), "GetMarketDepths should pass the requested depth size")
		_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"asks":[[67610.1,1.2],[67620.5,0.8]],"bids":[[67600.4,0.7],[67599.9,1.1]],"timestamp":1772335196000},"error_code":0,"ts":1772335196000}`))
		assert.NoError(t, err, "Writing the market depth response should not error")
	})

	depth, err := ex.GetMarketDepths(t.Context(), "btc_usdt", 2)
	require.NoError(t, err, "GetMarketDepths must not error for a valid v2 response")
	require.NotNil(t, depth, "GetMarketDepths must return a response")
	require.Len(t, depth.Data.Asks, 2, "GetMarketDepths must decode both asks")
	require.Len(t, depth.Data.Bids, 2, "GetMarketDepths must decode both bids")
	assert.Equal(t, 67610.1, depth.Data.Asks[0].Price, "GetMarketDepths should decode ask prices")
	assert.Equal(t, 1.1, depth.Data.Bids[1].Amount, "GetMarketDepths should decode bid amounts")
}

func TestGetTradesUsesV2ResponseEnvelope(t *testing.T) {
	t.Parallel()

	requestTime := time.UnixMilli(1772334000000).UTC()
	ex := newTestHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "GetTrades should use GET")
		assert.Equal(t, "/v2/supplement/trades.do", r.URL.Path, "GetTrades should use the v2 trades endpoint")
		assert.Equal(t, "btc_usdt", r.URL.Query().Get("symbol"), "GetTrades should pass the symbol")
		assert.Equal(t, "2", r.URL.Query().Get("size"), "GetTrades should pass the requested trade count")
		assert.Equal(t, "1772334000000", r.URL.Query().Get("time"), "GetTrades should pass the unix timestamp in milliseconds")
		_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[{"quoteQty":1250.2534918,"price":67617.82,"qty":0.01849,"id":"trade-1","time":1772334000388,"isBuyerMaker":true},{"quoteQty":98.7154326,"price":67613.31,"qty":0.00146,"id":"trade-2","time":1772334000648,"isBuyerMaker":false}],"error_code":0,"ts":1772335196000}`))
		assert.NoError(t, err, "Writing the trades response should not error")
	})

	trades, err := ex.GetTrades(t.Context(), "btc_usdt", 2, requestTime)
	require.NoError(t, err, "GetTrades must not error for a valid v2 response")
	require.Len(t, trades, 2, "GetTrades must decode both returned trades")
	assert.Equal(t, strings.ToLower(order.Sell.String()), trades[0].Type, "GetTrades should infer sell trades when the buyer is the maker")
	assert.Equal(t, strings.ToLower(order.Buy.String()), trades[1].Type, "GetTrades should infer buy trades when the seller is the maker")
	assert.Equal(t, 0.00146, trades[1].Amount, "GetTrades should decode the trade quantity")
	assert.Equal(t, int64(1772334000648), trades[1].DateMS.Time().UnixMilli(), "GetTrades should decode the trade timestamp")
}

func TestGetWalletMetadataEndpointsUseV2(t *testing.T) {
	t.Parallel()

	ex := newTestHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "Wallet metadata requests should use GET")

		switch r.URL.Path {
		case "/v2/usdToCny.do":
			_, err := w.Write([]byte(`{"result":"true","data":"6.9021","error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the exchange rate response should not error")
		case "/v2/withdrawConfigs.do":
			assert.Equal(t, "eth", r.URL.Query().Get("assetCode"), "GetWithdrawConfig should pass the asset code")
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[{"amountScale":"4","chain":"eth","assetCode":"eth","min":"0.01","transferAmtScale":"4","canWithDraw":true,"fee":0.01,"minTransfer":"0.01","type":"1"}],"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the withdraw config response should not error")
		default:
			t.Fatalf("unexpected wallet metadata request path: %s", r.URL.Path)
		}
	})

	rate, err := ex.USD2RMBRate(t.Context())
	require.NoError(t, err, "USD2RMBRate must not error for a valid v2 response")
	assert.Equal(t, "6.9021", rate.USD2CNY, "USD2RMBRate should decode the returned exchange rate")

	config, err := ex.GetWithdrawConfig(t.Context(), currency.ETH)
	require.NoError(t, err, "GetWithdrawConfig must not error for a valid v2 response")
	require.Len(t, config, 1, "GetWithdrawConfig must decode the returned configuration")
	assert.Equal(t, currency.ETH.Lower().String(), config[0].AssetCode.Lower().String(), "GetWithdrawConfig should decode the asset code")
	assert.Equal(t, 0.01, config[0].Fee, "GetWithdrawConfig should decode the withdraw fee")
}

func TestGetUserInfoUsesV2ResponseTransform(t *testing.T) {
	t.Parallel()

	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assertV2AuthRequest(t, r, "/v2/supplement/user_info.do", nil)
		_, err := w.Write([]byte(`{"result":"true","data":[{"coin":"btc","assetAmt":"1.5","usableAmt":"1.25","freezeAmt":"0.25"},{"coin":"usdt","assetAmt":"50","usableAmt":"45","freezeAmt":"5"}],"code":0}`))
		assert.NoError(t, err, "Writing the user info response should not error")
	})

	resp, err := ex.GetUserInfo(ctx)
	require.NoError(t, err, "GetUserInfo must not error for a valid v2 response")
	assert.Equal(t, 1.5, resp.Info.Asset["btc"].Float64(), "GetUserInfo should map asset balances into the legacy response shape")
	assert.Equal(t, 0.25, resp.Info.Freeze["btc"].Float64(), "GetUserInfo should map frozen balances into the legacy response shape")
	assert.Equal(t, 45.0, resp.Info.Free["usdt"].Float64(), "GetUserInfo should map available balances into the legacy response shape")
}

func TestGetUserInfoReturnsV2CodeError(t *testing.T) {
	t.Parallel()

	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assertV2AuthRequest(t, r, "/v2/supplement/user_info.do", nil)
		_, err := w.Write([]byte(`{"result":"false","code":10002}`))
		assert.NoError(t, err, "Writing the user info error response should not error")
	})

	_, err := ex.GetUserInfo(ctx)
	require.Error(t, err, "GetUserInfo must return an error when LBank returns a code failure")
	assert.EqualError(t, err, "Validation Failed", "GetUserInfo should surface code based errors")
}

func TestAuthenticatedOrderEndpointsUseV2(t *testing.T) {
	t.Parallel()

	call := 0
	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		call++

		switch call {
		case 1:
			assertV2AuthRequest(t, r, "/v2/supplement/create_order.do", map[string]string{
				"symbol": "btc_usdt",
				"type":   "buy",
				"price":  "42000",
				"amount": "0.5",
			})
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"order_id":"new-order"},"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the create order response should not error")
		case 2:
			assertV2AuthRequest(t, r, "/v2/supplement/cancel_order.do", map[string]string{
				"symbol":  "btc_usdt",
				"orderId": "order-a",
			})
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"status":0},"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the cancel order response should not error")
		case 3:
			assertV2AuthRequest(t, r, "/v2/supplement/cancel_order.do", map[string]string{
				"symbol":  "btc_usdt",
				"orderId": "order-b",
			})
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"status":0},"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the second cancel order response should not error")
		case 4:
			assertV2AuthRequest(t, r, "/v2/spot/trade/orders_info.do", map[string]string{
				"symbol":  "btc_usdt",
				"orderId": "order-a",
			})
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"cummulativeQuoteQty":"21000","symbol":"btc_usdt","executedQty":"0.5","orderId":"order-a","origQty":"0.5","price":"42000","time":1772335196000,"type":"buy","status":2},"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the query order response should not error")
		case 5:
			assertV2AuthRequest(t, r, "/v2/spot/trade/orders_info.do", map[string]string{
				"symbol":  "btc_usdt",
				"orderId": "order-b",
			})
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"cummulativeQuoteQty":"0","symbol":"btc_usdt","executedQty":"0","orderId":"order-b","origQty":"0.4","price":"41000","time":1772335197000,"type":"sell","status":0},"error_code":0,"ts":1772335197000}`))
			assert.NoError(t, err, "Writing the second query order response should not error")
		case 6:
			assertV2AuthRequest(t, r, "/v2/spot/trade/orders_info_history.do", map[string]string{
				"symbol":       "btc_usdt",
				"current_page": "1",
				"page_length":  "2",
			})
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"total":"1","page_length":2,"orders":[{"cummulativeQuoteQty":"21000","symbol":"btc_usdt","executedQty":"0.5","orderId":"order-a","origQty":"0.5","price":"42000","time":1772335196000,"type":"buy","status":2}],"current_page":1},"error_code":0,"ts":1772335196000}`))
			assert.NoError(t, err, "Writing the order history response should not error")
		case 7:
			assertV2AuthRequest(t, r, "/v2/supplement/orders_info_no_deal.do", map[string]string{
				"symbol":       "btc_usdt",
				"current_page": "1",
				"page_length":  "2",
			})
			_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"total":"2","page_length":2,"orders":[{"cummulativeQuoteQty":"0","symbol":"btc_usdt","executedQty":"0","orderId":"order-b","origQty":"0.4","price":"41000","time":1772335197000,"type":"sell","status":0}],"current_page":1},"error_code":0,"ts":1772335197000}`))
			assert.NoError(t, err, "Writing the open orders response should not error")
		default:
			t.Fatalf("unexpected authenticated order request count: %d", call)
		}
	})

	createResp, err := ex.CreateOrder(ctx, "btc_usdt", order.Buy.String(), 0.5, 42000)
	require.NoError(t, err, "CreateOrder must not error for a valid v2 response")
	assert.Equal(t, "new-order", createResp.OrderID, "CreateOrder should decode the returned order id")

	cancelResp, err := ex.RemoveOrder(ctx, "btc_usdt", "order-a,order-b")
	require.NoError(t, err, "RemoveOrder must not error for valid v2 responses")
	assert.Equal(t, "order-a,order-b", cancelResp.Success, "RemoveOrder should preserve multi-order cancellation support")

	queryResp, err := ex.QueryOrder(ctx, "btc_usdt", "order-a,order-b")
	require.NoError(t, err, "QueryOrder must not error for valid v2 responses")
	require.Len(t, queryResp.Orders, 2, "QueryOrder must decode both returned orders")
	assert.Equal(t, 42000.0, queryResp.Orders[0].AvgPrice, "QueryOrder should derive the average price from the cumulative quote quantity")
	assert.Equal(t, 0.4, queryResp.Orders[1].Amount, "QueryOrder should decode the original quantity")

	historyResp, err := ex.QueryOrderHistory(ctx, "btc_usdt", "1", "2")
	require.NoError(t, err, "QueryOrderHistory must not error for valid v2 responses")
	require.Len(t, historyResp.Orders, 1, "QueryOrderHistory must decode the returned order list")
	assert.Equal(t, uint8(1), historyResp.CurrentPage, "QueryOrderHistory should decode the current page")

	openOrdersResp, err := ex.GetOpenOrders(ctx, "btc_usdt", "1", "2")
	require.NoError(t, err, "GetOpenOrders must not error for valid v2 responses")
	require.Len(t, openOrdersResp.Orders, 1, "GetOpenOrders must decode the returned order list")
	assert.Equal(t, "2", openOrdersResp.Total, "GetOpenOrders should decode the total value")
	assert.Equal(t, uint8(1), openOrdersResp.PageNumber, "GetOpenOrders should map current_page into the legacy page number field")
}

func TestTransactionHistoryUsesV2RequestShape(t *testing.T) {
	t.Parallel()

	startTime := time.Date(2025, 2, 28, 16, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 3, 2, 4, 34, 56, 0, time.UTC)
	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assertV2AuthRequest(t, r, "/v2/supplement/transaction_history.do", map[string]string{
			"symbol":    "btc_usdt",
			"startTime": "2025-03-01 00:00:00",
			"endTime":   "2025-03-02 12:34:56",
			"fromId":    "trade-1",
			"limit":     "2",
		})
		_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[{"symbol":"btc_usdt","quoteQty":"21000","orderId":"order-a","price":"42000","qty":"0.5","commission":"10.5","id":"trade-1","time":1772335196000,"isMaker":false,"isBuyer":true}],"error_code":0,"ts":1772335196000}`))
		assert.NoError(t, err, "Writing the v2 transaction history response should not error")
	})

	resp, err := ex.TransactionHistory(ctx, &TransactionHistoryRequest{
		Symbol:    "btc_usdt",
		StartTime: startTime,
		EndTime:   endTime,
		FromID:    "trade-1",
		Limit:     2,
	})
	require.NoError(t, err, "TransactionHistory must not error for supported v2 parameters")
	require.Len(t, resp.Transaction, 1, "TransactionHistory must decode the v2 transaction list")
	assert.Equal(t, strings.ToLower(order.Buy.String()), resp.Transaction[0].TradeType, "TransactionHistory should infer the trade side from isBuyer")
	assert.Equal(t, 21000.0, resp.Transaction[0].DealVolPrice, "TransactionHistory should map the quote quantity into the legacy volume field")
}

func TestTransactionHistoryReturnsV2Error(t *testing.T) {
	t.Parallel()

	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assertV2AuthRequest(t, r, "/v2/supplement/transaction_history.do", map[string]string{
			"symbol": "btc_usdt",
		})
		_, err := w.Write([]byte(`{"msg":"Validation Failed","result":"false","error_code":10002}`))
		assert.NoError(t, err, "Writing the transaction history error response should not error")
	})

	_, err := ex.TransactionHistory(ctx, &TransactionHistoryRequest{Symbol: "btc_usdt"})
	require.Error(t, err, "TransactionHistory must return an error when the v2 endpoint fails")
	assert.EqualError(t, err, "Validation Failed", "TransactionHistory should surface the mapped v2 error")
}

func TestWithdrawUsesV2WalletEndpoint(t *testing.T) {
	t.Parallel()

	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assertV2AuthRequest(t, r, "/v2/spot/wallet/withdraw.do", map[string]string{
			"address":         "addr",
			"networkName":     "btc",
			"coin":            "btc",
			"amount":          "0.1",
			"memo":            "memo",
			"mark":            "mark",
			"fee":             "0.0002",
			"name":            "wallet-label",
			"withdrawOrderId": "client-1",
			"type":            "1",
		})
		_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":{"fee":"0.0002","withdrawId":93182},"error_code":0,"ts":1772335196000}`))
		assert.NoError(t, err, "Writing the withdraw response should not error")
	})

	withdrawResp, err := ex.Withdraw(ctx, &WithdrawRequest{
		Address:         "addr",
		NetworkName:     "btc",
		Coin:            currency.BTC,
		Amount:          0.1,
		Memo:            "memo",
		Mark:            "mark",
		Fee:             0.0002,
		Name:            "wallet-label",
		WithdrawOrderID: "client-1",
		Type:            "1",
	})
	require.NoError(t, err, "Withdraw must not error for a valid v2 response")
	assert.Equal(t, "93182", withdrawResp.WithdrawID, "Withdraw should decode the v2 withdraw id")
	assert.Equal(t, 0.0002, withdrawResp.Fee, "Withdraw should decode the v2 fee value")
}

func TestWithdrawReturnsV2Error(t *testing.T) {
	t.Parallel()

	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assertV2AuthRequest(t, r, "/v2/spot/wallet/withdraw.do", map[string]string{
			"address": "addr",
			"coin":    "btc",
			"amount":  "0.1",
			"fee":     "0.0002",
		})
		_, err := w.Write([]byte(`{"msg":"Has no privilege to withdraw","result":"false","error_code":10100}`))
		assert.NoError(t, err, "Writing the withdraw error response should not error")
	})

	_, err := ex.Withdraw(ctx, &WithdrawRequest{
		Address: "addr",
		Coin:    currency.BTC,
		Amount:  0.1,
		Fee:     0.0002,
	})
	require.Error(t, err, "Withdraw must return an error for a failing v2 response")
	assert.EqualError(t, err, "Has no privilege to withdraw", "Withdraw should surface the mapped v2 error")
}

func TestGetWithdrawalRecordsUsesV2WalletEndpoint(t *testing.T) {
	t.Parallel()

	startTime := time.UnixMilli(1772334000000).UTC()
	endTime := time.UnixMilli(1772337600000).UTC()
	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assertV2AuthRequest(t, r, "/v2/spot/wallet/withdraws.do", map[string]string{
			"status":          "1",
			"coin":            "btc",
			"withdrawOrderId": "client-1",
			"startTime":       "1772334000000",
			"endTime":         "1772337600000",
		})
		_, err := w.Write([]byte(`{"msg":"Success","result":"true","data":[{"amount":"0.1","coid":"btc","address":"addr","withdrawOrderId":"client-1","fee":"0.0002","networkName":"btc","transferType":"Digital Asset Withdrawal","txId":"hash","feeAssetCode":"btc","id":93182,"applyTime":1772335196000,"status":"1"}],"error_code":0,"ts":1772335196000}`))
		assert.NoError(t, err, "Writing the withdrawal records response should not error")
	})

	recordsResp, err := ex.GetWithdrawalRecords(ctx, &WithdrawalRecordsRequest{
		Coin:            currency.BTC,
		Status:          "1",
		WithdrawOrderID: "client-1",
		StartTime:       startTime,
		EndTime:         endTime,
	})
	require.NoError(t, err, "GetWithdrawalRecords must not error for a valid v2 response")
	require.Len(t, recordsResp, 1, "GetWithdrawalRecords must decode the returned records")
	assert.True(t, recordsResp[0].Coin.Equal(currency.BTC), "GetWithdrawalRecords should decode the returned currency")
	assert.Equal(t, "hash", recordsResp[0].TransactionID, "GetWithdrawalRecords should decode the transaction id")
	assert.Equal(t, int64(1772335196000), recordsResp[0].ApplyTime.UnixMilli(), "GetWithdrawalRecords should decode the apply time")
}

func TestGetWithdrawalRecordsReturnsV2Error(t *testing.T) {
	t.Parallel()

	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assertV2AuthRequest(t, r, "/v2/spot/wallet/withdraws.do", map[string]string{
			"coin": "btc",
		})
		_, err := w.Write([]byte(`{"msg":"Validation Failed","result":"false","error_code":10002}`))
		assert.NoError(t, err, "Writing the withdrawal records error response should not error")
	})

	_, err := ex.GetWithdrawalRecords(ctx, &WithdrawalRecordsRequest{Coin: currency.BTC})
	require.Error(t, err, "GetWithdrawalRecords must return an error for a failing v2 response")
	assert.EqualError(t, err, "Validation Failed", "GetWithdrawalRecords should surface the mapped v2 error")
}

func TestEndpointsWithoutUsableV2MappingsStayOnV1(t *testing.T) {
	t.Parallel()

	call := 0
	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		call++

		switch call {
		case 1:
			assertV1AuthRequest(t, r, "/v1/order_transaction_detail.do", map[string]string{
				"symbol":   "btc_usdt",
				"order_id": "order-a",
			})
			_, err := w.Write([]byte(`{"result":"true","transaction":[{"txUuid":"detail-trade","orderUuid":"order-a","tradeType":"buy","dealTime":1772335196000,"dealPrice":42000,"dealQuantity":0.5,"dealVolumePrice":21000,"tradeFee":10.5,"tradeFeeRate":0.1}],"error_code":0}`))
			assert.NoError(t, err, "Writing the order transaction detail response should not error")
		case 2:
			assertV1AuthRequest(t, r, "/v1/withdrawCancel.do", map[string]string{
				"withdrawId": "123",
			})
			_, err := w.Write([]byte(`{"result":"true","withdrawId":"123","error_code":0}`))
			assert.NoError(t, err, "Writing the revoke withdraw response should not error")
		default:
			t.Fatalf("unexpected v1 fallback request count: %d", call)
		}
	})

	detailResp, err := ex.OrderTransactionDetails(ctx, "btc_usdt", "order-a")
	require.NoError(t, err, "OrderTransactionDetails must not error for a valid legacy response")
	require.Len(t, detailResp.Transaction, 1, "OrderTransactionDetails must decode the returned transaction list")
	assert.Equal(t, "detail-trade", detailResp.Transaction[0].TxUUID, "OrderTransactionDetails should preserve the legacy response shape")

	revokeResp, err := ex.RevokeWithdraw(ctx, "123")
	require.NoError(t, err, "RevokeWithdraw must not error for a valid legacy response")
	assert.Equal(t, "123", revokeResp.WithdrawID, "RevokeWithdraw should decode the withdraw id")
}

func TestEndpointsWithoutUsableV2MappingsSurfaceV1Errors(t *testing.T) {
	t.Parallel()

	call := 0
	ex, ctx := newTestAuthHTTPExchange(t, func(w http.ResponseWriter, r *http.Request) {
		call++

		switch call {
		case 1:
			assertV1AuthRequest(t, r, "/v1/order_transaction_detail.do", map[string]string{
				"symbol":   "btc_usdt",
				"order_id": "order-a",
			})
			_, err := w.Write([]byte(`{"result":"false","error_code":10002}`))
			assert.NoError(t, err, "Writing the order transaction detail error response should not error")
		case 2:
			assertV1AuthRequest(t, r, "/v1/withdrawCancel.do", map[string]string{
				"withdrawId": "123",
			})
			_, err := w.Write([]byte(`{"result":"false","error_code":10104}`))
			assert.NoError(t, err, "Writing the revoke withdraw error response should not error")
		default:
			t.Fatalf("unexpected v1 error request count: %d", call)
		}
	})

	_, err := ex.OrderTransactionDetails(ctx, "btc_usdt", "order-a")
	require.Error(t, err, "OrderTransactionDetails must return an error for a failing legacy response")
	assert.EqualError(t, err, "Validation Failed", "OrderTransactionDetails should surface the mapped v1 error")

	_, err = ex.RevokeWithdraw(ctx, "123")
	require.Error(t, err, "RevokeWithdraw must return an error for a failing legacy response")
	assert.EqualError(t, err, "Cancel was rejected", "RevokeWithdraw should surface the mapped v1 error")
}

func TestLoadPrivKey(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	e.SetDefaults()
	require.ErrorIs(t, e.loadPrivKey(t.Context()), exchange.ErrCredentialsAreEmpty)

	ctx := accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "test", Secret: "errortest"})
	assert.ErrorIs(t, e.loadPrivKey(ctx), errPEMBlockIsNil)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der := x509.MarshalPKCS1PrivateKey(key)
	ctx = accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "test", Secret: base64.StdEncoding.EncodeToString(der)})
	require.ErrorIs(t, e.loadPrivKey(ctx), errUnableToParsePrivateKey)

	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err = x509.MarshalPKCS8PrivateKey(ecdsaKey)
	require.NoError(t, err)
	ctx = accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "test", Secret: base64.StdEncoding.EncodeToString(der)})
	require.ErrorIs(t, e.loadPrivKey(ctx), common.ErrTypeAssertFailure)

	key, err = rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err = x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	ctx = accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "test", Secret: base64.StdEncoding.EncodeToString(der)})
	assert.NoError(t, e.loadPrivKey(ctx), "loadPrivKey should not error")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	assert.NoError(t, e.loadPrivKey(t.Context()), "loadPrivKey should not error")
}

func TestSign(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	e.SetDefaults()
	_, err := e.sign("hello123")
	require.ErrorIs(t, err, errPrivateKeyNotLoaded)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "GenerateKey must not error")
	e.privateKey = key

	targetMessage := "hello123"
	msg, err := e.sign(targetMessage)
	require.NoError(t, err, "sign must not error")

	md5sum := md5.Sum([]byte(targetMessage)) //nolint:gosec // Used for this exchange
	shasum := sha256.Sum256([]byte(strings.ToUpper(hex.EncodeToString(md5sum[:]))))
	sigBytes, err := base64.StdEncoding.DecodeString(msg)
	require.NoError(t, err)
	err = rsa.VerifyPKCS1v15(&e.privateKey.PublicKey, crypto.SHA256, shasum[:], sigBytes)
	require.NoError(t, err)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	require.NoError(t, e.loadPrivKey(t.Context()), "loadPrivKey must not error")

	_, err = e.sign("hello123")
	assert.NoError(t, err, "sign should not error")
}

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCannotManipulateOrders(t, e, canManipulateRealOrders)

	r, err := e.SubmitOrder(t.Context(), &order.Submit{
		Exchange:  e.Name,
		Pair:      testPair,
		Side:      order.Buy,
		Type:      order.Limit,
		Price:     1,
		Amount:    1,
		ClientID:  "meowOrder",
		AssetType: asset.Spot,
	})
	if sharedtestvalues.AreAPICredentialsSet(e) {
		require.NoError(t, err, "SubmitOrder must not error")
		assert.Equal(t, order.New, r.Status, "SubmitOrder should return order status New")
	} else {
		assert.Error(t, err, "SubmitOrder should error when credentials are not set")
	}
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	err := e.CancelOrder(t.Context(), &order.Cancel{
		Pair:      testPair,
		AssetType: asset.Spot,
		OrderID:   "24f7ce27-af1d-4dca-a8c1-ef1cbeec1b23",
	})
	assert.NoError(t, err, "CancelOrder should not error")
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetOrderInfo(t.Context(), "9ead39f5-701a-400b-b635-d7349eb0f6b", currency.EMPTYPAIR, asset.Spot)
	assert.NoError(t, err, "GetOrderInfo should not error")
}

func TestGetAllOpenOrderID(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.getAllOpenOrderID(t.Context())
	assert.NoError(t, err, "getAllOpenOrderID should not error")
}

func TestGetFeeByType(t *testing.T) {
	t.Parallel()
	_, err := e.GetFeeByType(t.Context(), &exchange.FeeBuilder{
		Amount:  2,
		FeeType: exchange.CryptocurrencyWithdrawalFee,
		Pair:    testPair,
	})
	assert.NoError(t, err, "GetFeeByType should not error")
}

func TestGetAccountInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.UpdateAccountBalances(t.Context(), asset.Spot)
	assert.NoError(t, err, "UpdateAccountBalances should not error")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
		Side:      order.AnySide,
		AssetType: asset.Spot,
		Type:      order.AnyType,
	})
	assert.NoError(t, err, "GetActiveOrders should not error")
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{
		Side:      order.AnySide,
		AssetType: asset.Spot,
		Type:      order.AnyType,
	})
	assert.NoError(t, err, "GetOrderHistory should not error")
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricCandles(t.Context(), currency.EMPTYPAIR, asset.Spot, kline.OneMin, time.Time{}, time.Time{})
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetHistoricCandles(t.Context(), testPair, asset.Spot, kline.OneMin, time.Now().Add(-24*time.Hour), time.Now())
	assert.NoError(t, err, "GetHistoricCandles should not error")
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricCandlesExtended(t.Context(), testPair, asset.Spot, kline.OneMin, time.Now().Add(-time.Minute*2), time.Now())
	assert.NoError(t, err, "GetHistoricCandlesExtended should not error")
}

func TestFormatExchangeKlineInterval(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		interval kline.Interval
		output   string
	}{
		{
			kline.OneMin,
			"minute1",
		},
		{
			kline.OneHour,
			"hour1",
		},
		{
			kline.OneDay,
			"day1",
		},
		{
			kline.OneWeek,
			"week1",
		},
		{
			kline.FifteenDay,
			"",
		},
	} {
		t.Run(tc.interval.String(), func(t *testing.T) {
			t.Parallel()
			ret := e.FormatExchangeKlineInterval(tc.interval)
			assert.Equalf(t, tc.output, ret, "FormatExchangeKlineInterval(%s) should return %q", tc.interval, tc.output)
		})
	}
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetRecentTrades(t.Context(), testPair, asset.Spot)
	assert.NoError(t, err, "GetRecentTrades should not error")
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricTrades(t.Context(), testPair, asset.Spot, time.Now().AddDate(69, 0, 0), time.Now())
	assert.ErrorIs(t, err, common.ErrStartAfterEnd)
	_, err = e.GetHistoricTrades(t.Context(), currency.EMPTYPAIR, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	_, err = e.GetHistoricTrades(t.Context(), testPair, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	assert.NoError(t, err, "GetHistoricTrades should not error")
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateTicker(t.Context(), testPair, asset.Spot)
	assert.NoError(t, err, "UpdateTicker should not error")
}

func TestTransactionHistoryRequestValidate(t *testing.T) {
	t.Parallel()

	start := time.Unix(1772334000, 0).UTC()
	end := start.Add(time.Hour)
	for _, tc := range []struct {
		name string
		req  *TransactionHistoryRequest
		err  error
	}{
		{name: "nil", req: nil, err: common.ErrNilPointer},
		{name: "empty symbol", req: &TransactionHistoryRequest{}, err: errSymbolCannotBeEmpty},
		{name: "end before start", req: &TransactionHistoryRequest{Symbol: "btc_usdt", StartTime: end, EndTime: start}, err: errEndTimeBeforeStart},
		{name: "valid", req: &TransactionHistoryRequest{Symbol: "btc_usdt", StartTime: start, EndTime: end}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.req.validate()
			if tc.err == nil {
				assert.NoError(t, err, "validate should not error for a valid transaction history request")
				return
			}
			assert.ErrorIs(t, err, tc.err, "validate should return the expected transaction history request error")
		})
	}
}

func TestWithdrawRequestValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		req  *WithdrawRequest
		err  error
	}{
		{name: "nil", req: nil, err: common.ErrNilPointer},
		{name: "empty address", req: &WithdrawRequest{Coin: currency.BTC, Amount: 1}, err: errAddressCannotBeEmpty},
		{name: "empty coin", req: &WithdrawRequest{Address: "wallet", Amount: 1}, err: errCoinCannotBeEmpty},
		{name: "non-positive amount", req: &WithdrawRequest{Address: "wallet", Coin: currency.BTC}, err: errAmountMustBePositive},
		{name: "negative fee", req: &WithdrawRequest{Address: "wallet", Coin: currency.BTC, Amount: 1, Fee: -0.1}, err: errFeeCannotBeNegative},
		{name: "valid", req: &WithdrawRequest{Address: "wallet", Coin: currency.BTC, Amount: 1, Fee: 0.1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.req.validate()
			if tc.err == nil {
				assert.NoError(t, err, "validate should not error for a valid withdrawal request")
				return
			}
			assert.ErrorIs(t, err, tc.err, "validate should return the expected withdrawal request error")
		})
	}
}

func TestWithdrawalRecordsRequestValidate(t *testing.T) {
	t.Parallel()

	start := time.Unix(1772334000, 0).UTC()
	end := start.Add(time.Hour)
	for _, tc := range []struct {
		name string
		req  *WithdrawalRecordsRequest
		err  error
	}{
		{name: "nil", req: nil, err: common.ErrNilPointer},
		{name: "end before start", req: &WithdrawalRecordsRequest{StartTime: end, EndTime: start}, err: errEndTimeBeforeStart},
		{name: "valid", req: &WithdrawalRecordsRequest{Coin: currency.BTC, StartTime: start, EndTime: end}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.req.validate()
			if tc.err == nil {
				assert.NoError(t, err, "validate should not error for a valid withdrawal records request")
				return
			}
			assert.ErrorIs(t, err, tc.err, "validate should return the expected withdrawal records request error")
		})
	}
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	err := e.UpdateTickers(t.Context(), asset.Spot)
	assert.NoError(t, err, "UpdateTickers should not error")
}

func TestGetStatus(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		status int64
		resp   order.Status
	}{
		{status: -1, resp: order.Cancelled},
		{status: 0, resp: order.Active},
		{status: 1, resp: order.PartiallyFilled},
		{status: 2, resp: order.Filled},
		{status: 4, resp: order.Cancelling},
		{status: 5, resp: order.UnknownStatus},
	} {
		t.Run(tt.resp.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tt.resp.String(), e.GetStatus(tt.status).String(), "GetStatus(%d) should return %s", tt.status, tt.resp)
		})
	}
}

func TestGetServerTime(t *testing.T) {
	t.Parallel()
	ts, err := e.GetServerTime(t.Context(), asset.Spot)
	require.NoError(t, err, "GetServerTime must not error")
	assert.NotZero(t, ts, "GetServerTime should return a non-zero time")
}

func TestGetWithdrawalsHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err := e.GetWithdrawalsHistory(t.Context(), currency.BTC, asset.Spot)
	assert.NoError(t, err, "GetWithdrawalsHistory should not error")
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		require.NoErrorf(t, err, "GetPairs must not error for asset %s", a)
		require.NotEmptyf(t, pairs, "GetPairs for asset %s must return pairs", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		require.NoError(t, err)
		assert.NotEmpty(t, resp)
	}
}
