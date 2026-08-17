package kraken

import (
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

type capturedSpotRequest struct {
	path   string
	values url.Values
}

var errSpotTransport = errors.New("spot REST transport failure")

type spotTestRoundTripper func(*http.Request) (*http.Response, error)

func (r spotTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

func TestSpotTestRoundTripper(t *testing.T) {
	t.Parallel()
	expectedRequest := httptest.NewRequest(http.MethodGet, "https://kraken.test", http.NoBody)
	expectedResponse := &http.Response{Body: http.NoBody}
	roundTripper := spotTestRoundTripper(func(req *http.Request) (*http.Response, error) {
		require.Same(t, expectedRequest, req, "RoundTrip must pass through the request")
		return expectedResponse, errSpotTransport
	})
	response, err := roundTripper.RoundTrip(expectedRequest)
	require.ErrorIs(t, err, errSpotTransport, "RoundTrip must return the transport error")
	require.Same(t, expectedResponse, response, "RoundTrip must return the transport response")
	require.NoError(t, response.Body.Close(), "RoundTrip response body must close")
}

func TestFormatSpotFloat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		value         float64
		expected      string
		expectedError error
	}{
		{name: "zero", value: 0, expected: "0"},
		{name: "negative", value: -1.25, expected: "-1.25"},
		{name: "small decimal", value: 0.000000001, expected: "0.000000001"},
		{name: "NaN", value: math.NaN(), expectedError: errNumericValueInvalid},
		{name: "positive infinity", value: math.Inf(1), expectedError: errNumericValueInvalid},
		{name: "negative infinity", value: math.Inf(-1), expectedError: errNumericValueInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatSpotFloat(tc.value)
			require.ErrorIs(t, err, tc.expectedError, "formatSpotFloat must return the expected error")
			assert.Equal(t, tc.expected, result, "formatSpotFloat should return the expected wire value")
		})
	}
}

func TestFormatOrderPrice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		price         *OrderPrice
		expected      string
		expectedError error
	}{
		{name: "omitted"},
		{name: "explicit zero", price: &OrderPrice{}, expected: "0"},
		{name: "absolute", price: &OrderPrice{Value: 100.25}, expected: "100.25"},
		{name: "small absolute", price: &OrderPrice{Value: 0.000000001}, expected: "0.000000001"},
		{name: "relative addition", price: &OrderPrice{Expression: "+5"}, expected: "+5"},
		{name: "relative subtraction", price: &OrderPrice{Expression: "-5"}, expected: "-5"},
		{name: "relative static", price: &OrderPrice{Expression: "#5"}, expected: "#5"},
		{name: "relative percentage", price: &OrderPrice{Expression: "+5%"}, expected: "+5%"},
		{name: "conflicting representations", price: &OrderPrice{Value: 1, Expression: "+1"}, expectedError: errOrderPriceConflict},
		{name: "negative absolute", price: &OrderPrice{Value: -1}, expectedError: errOrderPriceInvalid},
		{name: "NaN absolute", price: &OrderPrice{Value: math.NaN()}, expectedError: errOrderPriceInvalid},
		{name: "infinite absolute", price: &OrderPrice{Value: math.Inf(1)}, expectedError: errOrderPriceInvalid},
		{name: "missing prefix", price: &OrderPrice{Expression: "5"}, expectedError: errOrderPriceInvalid},
		{name: "missing relative value", price: &OrderPrice{Expression: "+"}, expectedError: errOrderPriceInvalid},
		{name: "negative relative magnitude", price: &OrderPrice{Expression: "+-5"}, expectedError: errOrderPriceInvalid},
		{name: "invalid relative value", price: &OrderPrice{Expression: "+price"}, expectedError: errOrderPriceInvalid},
		{name: "infinite relative value", price: &OrderPrice{Expression: "+Inf"}, expectedError: errOrderPriceInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatOrderPrice(tc.price)
			require.ErrorIs(t, err, tc.expectedError, "formatOrderPrice must return the expected error")
			assert.Equal(t, tc.expected, result, "formatOrderPrice should return the expected wire value")
		})
	}
}

func TestFormatTimeOrTransactionID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		value         TimeOrTransactionID
		expected      string
		expectedError error
	}{
		{name: "omitted"},
		{name: "transaction ID", value: TimeOrTransactionID{TransactionID: "TX"}, expected: "TX"},
		{name: "epoch", value: TimeOrTransactionID{Time: time.Unix(0, 0)}, expected: "0"},
		{name: "timestamp", value: TimeOrTransactionID{Time: time.Unix(123, 0)}, expected: "123"},
		{name: "conflicting representations", value: TimeOrTransactionID{Time: time.Unix(123, 0), TransactionID: "TX"}, expectedError: errTimeOrIDConflict},
		{name: "pre-epoch timestamp", value: TimeOrTransactionID{Time: time.Unix(-1, 0)}, expectedError: errTimestampInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatTimeOrTransactionID(tc.value)
			require.ErrorIs(t, err, tc.expectedError, "formatTimeOrTransactionID must return the expected error")
			assert.Equal(t, tc.expected, result, "formatTimeOrTransactionID should return the expected wire value")
		})
	}
}

func TestFormatScheduledTime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		absolute        time.Time
		relative        time.Duration
		minimumRelative time.Duration
		expected        string
		expectedError   error
	}{
		{name: "omitted", minimumRelative: time.Second},
		{name: "epoch", absolute: time.Unix(0, 0), minimumRelative: time.Second, expected: "0"},
		{name: "absolute", absolute: time.Unix(123, 0), minimumRelative: time.Second, expected: "123"},
		{name: "minimum relative", relative: 5 * time.Second, minimumRelative: 5 * time.Second, expected: "+5"},
		{name: "conflicting representations", absolute: time.Unix(123, 0), relative: time.Second, minimumRelative: time.Second, expectedError: errScheduledTimeConflict},
		{name: "pre-epoch absolute", absolute: time.Unix(-1, 0), minimumRelative: time.Second, expectedError: errScheduledTimeInvalid},
		{name: "negative relative", relative: -time.Second, minimumRelative: time.Second, expectedError: errScheduledTimeInvalid},
		{name: "below minimum relative", relative: 4 * time.Second, minimumRelative: 5 * time.Second, expectedError: errScheduledTimeInvalid},
		{name: "fractional relative", relative: 1500 * time.Millisecond, minimumRelative: time.Second, expectedError: errScheduledTimeInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatScheduledTime(tc.absolute, tc.relative, tc.minimumRelative)
			require.ErrorIs(t, err, tc.expectedError, "formatScheduledTime must return the expected error")
			assert.Equal(t, tc.expected, result, "formatScheduledTime should return the expected wire value")
		})
	}
}

func TestFormatOrderFlags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		flags         []OrderFlag
		expected      string
		expectedError error
	}{
		{name: "omitted"},
		{name: "documented flags", flags: []OrderFlag{OrderFlagPostOnly, OrderFlagFeeInBase, OrderFlagFeeInQuote, OrderFlagVolumeInQuote, OrderFlagNoMarketProtect}, expected: "post,fcib,fciq,viqc,nompp"},
		{name: "empty flag", flags: []OrderFlag{""}, expectedError: errOrderFlagInvalid},
		{name: "invalid flag", flags: []OrderFlag{"invalid"}, expectedError: errOrderFlagInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatOrderFlags(tc.flags)
			require.ErrorIs(t, err, tc.expectedError, "formatOrderFlags must return the expected error")
			assert.Equal(t, tc.expected, result, "formatOrderFlags should return the expected wire value")
		})
	}
}

func TestFormatDeadline(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	location := time.FixedZone("AEST", 10*60*60)
	for _, tc := range []struct {
		name          string
		deadline      time.Time
		expected      string
		expectedError error
	}{
		{name: "omitted"},
		{name: "too soon", deadline: now.Add(2*time.Second - time.Nanosecond), expectedError: errDeadlineInvalid},
		{name: "too late", deadline: now.Add(time.Minute + time.Nanosecond), expectedError: errDeadlineInvalid},
		{name: "minimum", deadline: now.Add(2 * time.Second), expected: "2026-08-05T00:00:02Z"},
		{name: "maximum in another location", deadline: now.Add(time.Minute).In(location), expected: "2026-08-05T00:01:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatDeadline(tc.deadline, now)
			require.ErrorIs(t, err, tc.expectedError, "formatDeadline must return the expected error")
			assert.Equal(t, tc.expected, result, "formatDeadline should return the expected UTC value")
		})
	}
}

func TestGetWebsocketToken(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	response, err := ex.GetWebsocketToken(t.Context())
	require.NoError(t, err, "GetWebsocketToken must not error")
	assert.Equal(t, "TOKEN", response.Token, "GetWebsocketToken should decode the token")
	assert.Equal(t, int64(900), response.Expires, "GetWebsocketToken should decode the expiry")
	requireSpotRequest(t, requests, "/0/private/GetWebSocketsToken")
}

func TestGetWebsocketTokenNullResult(t *testing.T) {
	token, err := newSpotNullResultExchange(t).GetWebsocketToken(t.Context())
	require.NoError(t, err, "GetWebsocketToken must accept a null REST result")
	require.Nil(t, token, "GetWebsocketToken must return nil for a null REST result")
}

func cloneSpotValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, value := range values {
		cloned[key] = slices.Clone(value)
	}
	return cloned
}

func newSpotEndpointExchange(t *testing.T) (ex *Exchange, capturedRequests <-chan capturedSpotRequest) {
	t.Helper()
	requests := make(chan capturedSpotRequest, 128)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := cloneSpotValues(r.URL.Query())
		payload, _ := io.ReadAll(r.Body)
		bodyValues, _ := url.ParseQuery(string(payload))
		for key, value := range bodyValues {
			values[key] = slices.Clone(value)
		}
		requests <- capturedSpotRequest{path: r.URL.Path, values: values}

		if r.URL.Path == "/0/private/RetrieveExport" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "PK\x03\x04export")
			return
		}
		if r.URL.Path == "/0/private/RetrieveExportError" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"error":["EGeneral:boom"],"result":null}`)
			return
		}
		if r.URL.Path == "/0/private/RawScalar" {
			_, _ = io.WriteString(w, `123`)
			return
		}
		if r.URL.Path == "/0/private/RawObject" {
			_, _ = io.WriteString(w, `{"report":"data"}`)
			return
		}
		if r.URL.Path == "/0/private/NormalError" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"error":["EGeneral:boom"],"result":null}`)
			return
		}
		if r.URL.Path == "/0/private/SemanticError" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"error":[1],"result":null}`)
			return
		}
		if r.URL.Path == "/0/private/Warning" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"error":["WGeneral:warning"],"result":{"amend_id":"AMEND"}}`)
			return
		}
		if r.URL.Path == "/0/private/Malformed" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{`)
			return
		}

		result := `{}`
		switch r.URL.Path {
		case "/0/public/Time":
			result = `{"unixtime":1785636405,"rfc1123":"Sun,  2 Aug 26 05:06:45 +0000"}`
		case "/0/public/SystemStatus":
			result = `{"status":"online","timestamp":"2026-08-02T00:00:00Z"}`
		case "/0/public/Assets":
			result = `{"BTC":{"aclass":"currency","altname":"XBT","decimals":10,"display_decimals":5,"collateral_value":"1","margin_rate":0.02,"status":"enabled"},"XXBT":{"aclass":"currency","altname":"XBT","decimals":10,"display_decimals":5,"status":"enabled"},"USD":{"aclass":"currency","altname":"USD","decimals":4,"display_decimals":2,"status":"enabled"}}`
		case "/0/public/AssetPairs":
			result = `{"BTC/USD":{"altname":"XBTUSD","wsname":"XBT/USD","aclass_base":"currency","base":"BTC","aclass_quote":"currency","quote":"USD","execution_venue":"international","lot":"unit","cost_decimals":5,"pair_decimals":1,"lot_decimals":8,"lot_multiplier":1,"leverage_buy":[2],"leverage_sell":[2],"fees":[[0,0.26]],"fees_maker":[[0,0.16]],"fee_volume_currency":"USD","margin_call":80,"margin_stop":40,"ordermin":"0.0001","costmin":"0.5","tick_size":"0.1","status":"online","long_position_limit":"250","short_position_limit":"200"}}`
		case "/0/public/Ticker":
			result = `{"BTC/USD":{"a":["101","1","2"],"b":["99","1","2"],"c":["100","1"],"v":["10","20"],"p":["99","100"],"t":[1,2],"l":["90","91"],"h":["110","111"],"o":"95"}}`
		case "/0/public/OHLC":
			result = `{"BTC/USD":[[1695828271,"1","2","0.5","1.5","1.2","10",3]],"last":1695828271}`
		case "/0/public/Depth":
			result = `{"BTC/USD":{"bids":[["1","2",1695828271]],"asks":[]}}`
		case "/0/public/Trades":
			result = `{"BTC/USD":[["100","2",1695828271,"b","m","",61044952],["101","1",1695828272,"s","l","",61044953]],"last":"1695828273000000000"}`
		case "/0/public/Spread":
			result = `{"BTC/USD":[[1695828271,"99","101"]],"last":1695828272}`
		case "/0/public/GroupedBook":
			result = `{"pair":"XBTUSD","grouping":5,"bids":[{"price":"100","qty":"2"}],"asks":[]}`
		case "/0/private/Level3":
			result = `{"pair":"XBTUSD","bids":[{"price":"100","qty":"2","order_id":"ORDER","timestamp":1765622008594292000}],"asks":[]}`
		case "/0/public/PreTrade":
			result = `{"symbol":"BTC/USD","description":"Bitcoin / US Dollars","base_asset":"BTC","base_dti_code":"4H95J0R2X","base_dti_short_name":"BTC","base_notation":"UNIT","quote_asset":"USD","quote_dti_code":"4H95J0R2Y","quote_dti_short_name":"USD","quote_notation":"MONE","venue":"PGSL","system":"CLOB","bids":[{"side":"BUY","price":"100","qty":"2","count":1,"submission_ts":"2026-08-02T00:00:00Z","publication_ts":"2026-08-02T00:00:01Z"}],"asks":[]}`
		case "/0/public/PostTrade":
			result = `{"last_ts":"2026-08-02T00:00:01Z","count":1,"trades":[{"trade_id":"TRADE","price":"100","quantity":"2","symbol":"BTC/USD","description":"Bitcoin / US Dollars","base_asset":"BTC","base_notation":"UNIT","quote_asset":"USD","quote_notation":"MONE","trade_venue":"PGSL","trade_ts":"2026-08-02T00:00:00Z","publication_venue":"PGSL","publication_ts":"2026-08-02T00:00:01Z"}]}`
		case "/0/private/Balance":
			result = `{"XXBT":"1.25"}`
		case "/0/private/BalanceEx":
			result = `{"XXBT":{"balance":1.25,"credit":"0.50","credit_used":0.10,"hold_trade":"0.25"},"UNKNOWN":{"balance":"2","credit":"0","credit_used":"0","hold_trade":"0"}}`
		case "/0/private/CreditLines":
			result = `{"asset_details":{"USD":{"balance":"100","hold_trade":"5","collateral_value":0.99,"credit_limit":"50","credit_used":"10","available_credit":"40"}},"limits_monitor":{"total_credit_usd":"50"}}`
		case "/0/private/TradeBalance":
			result = `{"eb":"1101.3425","tb":"392.2264","m":"7.0354","n":"-10.0232","c":"21.1063","v":"31.1297","e":"382.2032","mf":"375.1678","mfo":"374","ml":"5432.57","uv":"2"}`
		case "/0/private/OpenOrders":
			result = `{"open":{"ORDER":{"refid":"REF","userref":1,"cl_ord_id":"CLIENT","status":"open","opentm":1695828271,"starttm":0,"expiretm":0,"descr":{"pair":"BTC/USD","type":"buy","ordertype":"limit","price":"100","price2":"0","leverage":"none","order":"buy 1 BTC/USD","close":"","aclass":"currency"},"time_in_force":"gtc","vol":"1","vol_exec":"0","cost":"0","fee":"0","price":"0","stopprice":"0","limitprice":"0","trigger":"last","margin":false,"misc":"","oflags":"post","trades":[],"sender_sub_id":"SUB"}}}`
		case "/0/private/ClosedOrders":
			result = `{"closed":{"ORDER":{"status":"closed","reason":"User requested","descr":{"pair":"BTC/USD","aclass":"currency"},"time_in_force":"fok","vol":"1","vol_exec":"1","cost":"100","fee":"0.2","price":"100","stopprice":"0","limitprice":"0","trigger":"last","margin":true,"sender_sub_id":"SUB"}},"count":1}`
		case "/0/private/QueryOrders":
			result = `{"ORDER":{"status":"closed","descr":{"pair":"BTC/USD","aclass":"currency"},"time_in_force":"ioc","vol":"1","vol_exec":"1","cost":"100","fee":"0.2","price":"100","stopprice":"0","limitprice":"0","trigger":"last","margin":false,"sender_sub_id":"SUB"}}`
		case "/0/private/TradesHistory":
			result = `{"trades":{"TRADE":{"ordertxid":"ORDER","postxid":"POSITION","pair":"BTC/USD","time":1695828271,"type":"buy","ordertype":"limit","price":"100","cost":"100","fee":"0.2","vol":"1","margin":"0","leverage":"2","misc":"closing","cprice":101.5,"ccost":100.5,"cfee":0.1,"cvol":1,"cmargin":20,"net":1.5,"trades":["CLOSE"],"ledgers":["LEDGER"],"trade_id":42,"maker":true,"aclass":"currency","tradeordertype":"market","posstatus":"closed"}},"count":1}`
		case "/0/private/QueryTrades":
			result = `{"TRADE":{"ordertxid":"ORDER","postxid":"POSITION","pair":"BTC/USD","time":1695828271,"type":"buy","ordertype":"limit","price":"100","cost":"100","fee":"0.2","vol":"1","margin":"0","leverage":"2","misc":"closing","cprice":"101.5","ccost":"100.5","cfee":"0.1","cvol":"1","cmargin":"20","net":"1.5","trades":["CLOSE"],"ledgers":["LEDGER"],"trade_id":42,"maker":true,"aclass":"currency","tradeordertype":"market","posstatus":"closed"}}`
		case "/0/private/OpenPositions":
			result = `{"POSITION":{"ordertxid":"ORDER","class":"currency","pair":"BTC/USD","time":1695828271,"type":"buy","ordertype":"limit","cost":"100","fee":"0.2","vol":"1","vol_closed":"0","margin":"20","value":120,"rollovertm":"0","misc":"","oflags":""}}`
		case "/0/private/QueryLedgers":
			result = `{"LEDGER":{"refid":"REFERENCE","time":1695828271,"type":"trade","subtype":"spotfromfutures","aclass":"currency","asset":"BTC","amount":"1","fee":"0","balance":"1"}}`
		case "/0/private/TradeVolume":
			result = `{"currency":"USD","asset_class":"currency","volume":"200","inputs":{"domain_spot_volume_30d":"200","domain_futures_volume_30d":"10","domain_assets_on_platform":"1000"},"fees":{"BTC/USD":{"fee":"0.2","minfee":"0.1","maxfee":"0.3","nextfee":null,"tiervolume":"100","tierfuturesvolume":null,"nextvolume":null,"nextfuturesvolume":null,"volumeoffset":"5"}},"fees_maker":{"BTC/USD":{"fee":"0.1","minfee":"0.05","maxfee":"0.2","nextfee":"0.08","tiervolume":"100","tierfuturesvolume":"50","nextvolume":"200","nextfuturesvolume":"100","volumeoffset":null}},"volume_subaccounts":[{"iiban":"SUB","volume":"50"}],"schedules":[{"pair":"BTC/USD","class":"forex","tiers":[{"maker_fee":"0.1","taker_fee":"0.2","min_spot_volume":"100","min_futures_volume":null,"min_assets_on_platform":"500","active":true}]}]}`
		case "/0/private/OrderAmends":
			result = `{"count":1,"amends":[{"amend_id":"AMEND","amend_type":"original","order_qty":"1","timestamp":1724158070287558000}]}`
		case "/0/private/AddExport":
			result = `{"id":"REPORT"}`
		case "/0/private/ExportStatus":
			result = `[{"id":"REPORT","status":"Processed","error":"","asset_classes":["currency"],"endtm":"1688669085","delete":false}]`
		case "/0/private/RemoveExport":
			result = `{"delete":true,"cancel":false}`
		case "/0/private/GetApiKeyInfo":
			result = `{"apiKeyName":"spot","apiKey":"key","nonce":"1","nonceWindow":0,"permissions":["earn-funds"],"validUntil":"0","queryFrom":"0","queryTo":"0","createdTime":"1772542900","modifiedTime":"1772543095","lastUsed":null}`
		case "/0/private/AmendOrder":
			result = `{"amend_id":"AMEND"}`
		case "/0/private/AddOrder":
			result = `{"descr":{"order":"buy 1 BTC/USD","close":"close @ 90"},"txid":["ORDER"]}`
		case "/0/private/CancelOrder":
			result = `{"count":1,"pending":false}`
		case "/0/private/CancelAll":
			result = `{"count":2,"pending":false}`
		case "/0/private/CancelAllOrdersAfter":
			result = `{"currentTime":"2026-08-02T00:00:00Z","triggerTime":"2026-08-02T00:00:00Z"}`
		case "/0/private/AddOrderBatch":
			result = `{"orders":[{"descr":{"order":"buy 1 XBTUSD"},"txid":"ORDER"}]}`
		case "/0/private/CancelOrderBatch":
			result = `{"count":3}`
		case "/0/private/Ledgers":
			result = `{"ledger":{"LEDGER":{"refid":"REFERENCE","time":1695828271,"type":"trade","subtype":"spotfromfutures","aclass":"currency","asset":"XXBT","amount":"1","fee":"0","balance":"1"}},"count":1}`
		case "/0/private/DepositMethods":
			result = `[{"method":"Bitcoin","limit":false,"fee":"0.0001","fee-percentage":"0.1","address-setup-fee":"0","gen-address":true,"minimum":"0.001"},{"method":"SynapsePay (US Wire)","limit":false,"fee":"5","fee-percentage":"0","address-setup-fee":"0","gen-address":false,"minimum":"1"}]`
		case "/0/private/DepositAddresses":
			result = `[{"address":"bc1q","expiretm":"0","new":true,"tag":"TAG","memo":"MEMO"}]`
		case "/0/private/DepositStatus":
			result = `{"deposit":[{"method":"Bitcoin","aclass":"currency","asset":"XXBT","refid":"REF","txid":"TX","amount":"1","fee":"0","time":1695828271,"status":"Success"}],"next_cursor":"NEXT"}`
		case "/0/private/WithdrawInfo":
			result = `{"method":"Bitcoin","limit":"10","amount":"0.9","fee":"0.1"}`
		case "/0/private/Withdraw":
			result = `{"refid":"WITHDRAWAL"}`
		case "/0/private/WithdrawCancel":
			result = `true`
		case "/0/private/WithdrawStatus":
			result = `{"withdrawal":[{"method":"Bitcoin","network":"Bitcoin","aclass":"currency","asset":"XXBT","refid":"REF","txid":"TX","info":"bc1q","amount":"1","fee":"0.1","time":1695828271,"status":"Success","key":"wallet"}],"next_cursor":"NEXT"}`
		case "/0/private/WithdrawMethods":
			result = `[{"asset":"XXBT","method":"Bitcoin","method_id":"METHOD","network":"Bitcoin","network_id":"NETWORK","minimum":"0.0004","fee":{"aclass":"currency","asset":"XXBT","fee":"0.00001","fee_percentage":"0.1"},"limits":[{"description":"daily","limit_type":"amount","limits":{"86400":{"maximum":"10","remaining":"8","used":"2"}}}]}]`
		case "/0/private/WithdrawAddresses":
			result = `[{"address":"bc1q","asset":"XBT","method":"Bitcoin","key":"wallet","tag":"","verified":true}]`
		case "/0/private/WalletTransfer":
			result = `{"refid":"TRANSFER"}`
		case "/0/private/CreateSubaccount":
			result = `true`
		case "/0/private/AccountTransfer":
			result = `{"transfer_id":"TRANSFER","status":"complete"}`
		case "/0/private/GetWebSocketsToken":
			result = `{"token":"TOKEN","expires":900}`
		case "/0/private/Earn/Allocate", "/0/private/Earn/Deallocate":
			result = `true`
		case "/0/private/Earn/AllocateStatus", "/0/private/Earn/DeallocateStatus":
			result = `{"pending":false}`
		case "/0/private/Earn/Strategies":
			result = `{"items":[{"id":"STRATEGY","asset":"DOT","lock_type":{"type":"instant","payout_frequency":604800},"apr_estimate":{"low":"8","high":"12"},"allocation_fee":"0","deallocation_fee":0,"auto_compound":{"type":"enabled"},"yield_source":{"type":"staking"},"can_allocate":true,"can_deallocate":true,"allocation_restriction_info":[]}],"next_cursor":"NEXT"}`
		case "/0/private/Earn/Allocations":
			result = `{"converted_asset":"USD","total_allocated":"10","total_rewarded":"1","next_cursor":"NEXT","items":[{"strategy_id":"STRATEGY","native_asset":"DOT","amount_allocated":{"total":{"native":"1","converted":"10"}},"total_rewarded":{"native":"0.1","converted":"1"}}]}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"error":[],"result":`+result+`}`)
	}))
	t.Cleanup(server.Close)
	ex = newAuthenticatedSpotExchange(t, server.URL)
	require.NoError(t, ex.Requester.DisableRateLimiter(), "DisableRateLimiter must disable rate limiting for endpoint tests")
	return ex, requests
}

func newSpotErrorExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := newAuthenticatedSpotExchange(t, "https://kraken.invalid")
	err := ex.Requester.SetHTTPClient(&http.Client{Transport: spotTestRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errSpotTransport
	})})
	require.NoError(t, err, "SetHTTPClient must install the endpoint error transport")
	require.NoError(t, ex.Requester.DisableRateLimiter(), "DisableRateLimiter must disable rate limiting for endpoint error tests")
	return ex
}

func newSpotNullResultExchange(t *testing.T) *Exchange {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"error":[],"result":null}`)
	}))
	t.Cleanup(server.Close)
	ex := newAuthenticatedSpotExchange(t, server.URL)
	require.NoError(t, ex.Requester.DisableRateLimiter(), "DisableRateLimiter must disable rate limiting for nil-result tests")
	return ex
}

func requireSpotRequest(t *testing.T, requests <-chan capturedSpotRequest, path string) url.Values {
	t.Helper()
	select {
	case capturedRequest := <-requests:
		require.Equal(t, path, capturedRequest.path, "Spot REST request must use the expected path")
		return capturedRequest.values
	case <-time.After(time.Second):
		require.FailNow(t, "Spot REST request must reach the mock server")
		return nil
	}
}

func TestSpotMarketEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()

	status, err := ex.GetSystemStatus(ctx)
	require.NoError(t, err, "GetSystemStatus must not error")
	assert.Equal(t, "online", status.Status, "GetSystemStatus should decode status")
	requireSpotRequest(t, requests, "/0/public/SystemStatus")

	_, err = ex.GetGroupedOrderBook(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetGroupedOrderBook must reject a nil request")
	_, err = ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{})
	require.ErrorIs(t, err, errPairRequired, "GetGroupedOrderBook must require a pair")
	_, err = ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair, Depth: 1})
	require.ErrorIs(t, err, errGroupedDepthInvalid, "GetGroupedOrderBook must reject unsupported depth")
	_, err = ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair, Grouping: 2})
	require.ErrorIs(t, err, errGroupingInvalid, "GetGroupedOrderBook must reject unsupported grouping")
	for _, tc := range []struct {
		name     string
		depth    GroupedOrderBookDepth
		grouping OrderBookGrouping
	}{
		{name: "lower bounds", depth: 10, grouping: 1},
		{name: "upper bounds", depth: 1000, grouping: 1000},
	} {
		t.Run("grouped "+tc.name, func(t *testing.T) {
			_, err := ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair, Depth: tc.depth, Grouping: tc.grouping})
			require.NoError(t, err, "GetGroupedOrderBook must accept documented bounds")
			requireSpotRequest(t, requests, "/0/public/GroupedBook")
		})
	}
	grouped, err := ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair, Depth: 25, Grouping: 5})
	require.NoError(t, err, "GetGroupedOrderBook must not error")
	assert.Len(t, grouped.Bids, 1, "GetGroupedOrderBook should decode bids")
	values := requireSpotRequest(t, requests, "/0/public/GroupedBook")
	assert.Equal(t, "25", values.Get("depth"), "GetGroupedOrderBook should encode depth")
	assert.Equal(t, "5", values.Get("grouping"), "GetGroupedOrderBook should encode grouping")

	_, err = ex.QueryLevel3OrderBook(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "QueryLevel3OrderBook must reject a nil request")
	_, err = ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{})
	require.ErrorIs(t, err, errPairRequired, "QueryLevel3OrderBook must require a pair")
	invalidDepth := Level3OrderBookDepth(1)
	_, err = ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair, Depth: &invalidDepth})
	require.ErrorIs(t, err, errLevel3DepthInvalid, "QueryLevel3OrderBook must reject unsupported depth")
	for _, depth := range []Level3OrderBookDepth{Level3OrderBookDepthFull, Level3OrderBookDepth1000} {
		_, err := ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair, Depth: &depth})
		require.NoError(t, err, "QueryLevel3OrderBook must accept documented bounds")
		levelValues := requireSpotRequest(t, requests, "/0/private/Level3")
		assert.Equal(t, strconv.FormatUint(uint64(depth), 10), levelValues.Get("depth"), "QueryLevel3OrderBook should encode documented depth")
	}
	level3, err := ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair})
	require.NoError(t, err, "QueryLevel3OrderBook must not error")
	assert.Equal(t, int64(1765622008594292000), level3.Bids[0].Timestamp.Time().UnixNano(), "QueryLevel3OrderBook should retain nanosecond timestamps")
	values = requireSpotRequest(t, requests, "/0/private/Level3")
	assert.Empty(t, values.Get("depth"), "QueryLevel3OrderBook should omit depth to use the server default")

	_, err = ex.GetPreTradeData(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetPreTradeData must reject a nil request")
	_, err = ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{})
	require.ErrorIs(t, err, errSymbolRequired, "GetPreTradeData must require a symbol")
	_, err = new(Exchange).GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: spotTestPair})
	require.Error(t, err, "GetPreTradeData must surface pair-format errors")
	_, err = ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: currency.NewPairWithDelimiter("A", "B", "")})
	require.ErrorIs(t, err, errSymbolLengthInvalid, "GetPreTradeData must reject a symbol shorter than three characters")
	_, err = ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: currency.NewPairWithDelimiter(strings.Repeat("A", 16), strings.Repeat("B", 17), "")})
	require.ErrorIs(t, err, errSymbolLengthInvalid, "GetPreTradeData must reject a symbol longer than thirty-two characters")
	for _, pair := range []currency.Pair{currency.NewPairWithDelimiter("A", "BC", ""), currency.NewPairWithDelimiter(strings.Repeat("A", 16), strings.Repeat("B", 16), "")} {
		_, err := ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: pair})
		require.NoError(t, err, "GetPreTradeData must accept documented symbol bounds")
		requireSpotRequest(t, requests, "/0/public/PreTrade")
	}
	preTrade, err := ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: spotTestPair})
	require.NoError(t, err, "GetPreTradeData must not error")
	assert.Equal(t, "4H95J0R2X", preTrade.BaseDTICode, "GetPreTradeData should decode DTI metadata")
	assert.False(t, preTrade.Bids[0].SubmissionTimestamp.IsZero(), "GetPreTradeData should decode submission timestamps")
	values = requireSpotRequest(t, requests, "/0/public/PreTrade")
	assert.Equal(t, "XBTUSD", values.Get("symbol"), "GetPreTradeData should encode symbol")

	_, err = ex.GetPostTradeData(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetPostTradeData must reject a nil request")
	_, err = ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{Count: 1001})
	require.ErrorIs(t, err, errPostTradeCountTooLarge, "GetPostTradeData must enforce Kraken's result limit")
	from := time.Date(2026, 8, 1, 11, 2, 3, 4, time.FixedZone("AEST", 10*60*60))
	to := from.Add(time.Hour)
	_, err = ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{FromTimestamp: to, ToTimestamp: from})
	require.ErrorIs(t, err, errTimeRangeInvalid, "GetPostTradeData must reject a reversed time range")
	_, err = ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{Pair: currency.Pair{Base: currency.BTC}})
	require.ErrorIs(t, err, errPairRequired, "GetPostTradeData must reject a partially populated pair")
	_, err = new(Exchange).GetPostTradeData(ctx, &GetPostTradeDataRequest{Pair: spotTestPair})
	require.Error(t, err, "GetPostTradeData must surface pair-format errors")
	postTrade, err := ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{Pair: spotTestPair, FromTimestamp: from, ToTimestamp: to, Count: 100})
	require.NoError(t, err, "GetPostTradeData must not error")
	assert.Equal(t, uint64(1), postTrade.Count, "GetPostTradeData should decode the trade count")
	values = requireSpotRequest(t, requests, "/0/public/PostTrade")
	assert.Equal(t, "XBTUSD", values.Get("symbol"), "GetPostTradeData should encode the formatted pair")
	assert.Equal(t, from.UTC().Format(time.RFC3339Nano), values.Get("from_ts"), "GetPostTradeData should encode the start timestamp in UTC")
	assert.Equal(t, to.UTC().Format(time.RFC3339Nano), values.Get("to_ts"), "GetPostTradeData should encode the end timestamp in UTC")
	assert.Equal(t, "100", values.Get("count"), "GetPostTradeData should encode count")
}

func TestSpotAccountEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()

	_, err := ex.GetAccountBalance(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetAccountBalance must reject a nil request")
	_, err = ex.GetAccountBalance(ctx, &GetAccountBalanceRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetAccountBalance must reject an invalid rebase multiplier")
	balance, err := ex.GetAccountBalance(ctx, &GetAccountBalanceRequest{RebaseMultiplier: "base"})
	require.NoError(t, err, "GetAccountBalance must not error")
	assert.Equal(t, 1.25, balance["XXBT"].Float64(), "GetAccountBalance should decode balances")
	values := requireSpotRequest(t, requests, "/0/private/Balance")
	assert.Equal(t, "base", values.Get("rebase_multiplier"), "GetAccountBalance should encode the rebase multiplier")

	_, err = ex.GetExtendedBalance(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetExtendedBalance must reject a nil request")
	_, err = ex.GetExtendedBalance(ctx, &GetExtendedBalanceRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetExtendedBalance must reject an invalid rebase multiplier")
	extended, err := ex.GetExtendedBalance(ctx, &GetExtendedBalanceRequest{RebaseMultiplier: "rebased"})
	require.NoError(t, err, "GetExtendedBalance must not error")
	assert.Equal(t, 1.25, extended["XXBT"].Balance.Float64(), "GetExtendedBalance should decode numeric balances")
	assert.Equal(t, 0.5, extended["XXBT"].Credit.Float64(), "GetExtendedBalance should decode credit")
	assert.Equal(t, 0.1, extended["XXBT"].CreditUsed.Float64(), "GetExtendedBalance should decode used credit")
	assert.Equal(t, 0.25, extended["XXBT"].HoldTrade.Float64(), "GetExtendedBalance should decode held balances")
	values = requireSpotRequest(t, requests, "/0/private/BalanceEx")
	assert.Equal(t, "rebased", values.Get("rebase_multiplier"), "GetExtendedBalance should encode the rebase multiplier")

	_, err = ex.GetCreditLines(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetCreditLines must reject a nil request")
	_, err = ex.GetCreditLines(ctx, &GetCreditLinesRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetCreditLines must reject an invalid rebase multiplier")
	credit, err := ex.GetCreditLines(ctx, &GetCreditLinesRequest{RebaseMultiplier: "base"})
	require.NoError(t, err, "GetCreditLines must not error")
	assert.Equal(t, 40.0, credit.AssetDetails["USD"].AvailableCredit.Float64(), "GetCreditLines should decode available credit")
	assert.Equal(t, 5.0, credit.AssetDetails["USD"].HoldTrade.Float64(), "GetCreditLines should decode held balances")
	assert.Equal(t, 0.99, credit.AssetDetails["USD"].CollateralValue.Float64(), "GetCreditLines should decode collateral value")
	requireSpotRequest(t, requests, "/0/private/CreditLines")

	_, err = ex.GetOrderAmends(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetOrderAmends must reject a nil request")
	_, err = ex.GetOrderAmends(ctx, &GetOrderAmendsRequest{})
	require.ErrorIs(t, err, errOrderIDRequired, "GetOrderAmends must require an order identifier")
	_, err = ex.GetOrderAmends(ctx, &GetOrderAmendsRequest{OrderID: "ORDER", RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetOrderAmends must reject an invalid rebase multiplier")
	amends, err := ex.GetOrderAmends(ctx, &GetOrderAmendsRequest{OrderID: "ORDER", RebaseMultiplier: "base"})
	require.NoError(t, err, "GetOrderAmends must not error")
	assert.Equal(t, int64(1724158070287558000), amends.Amends[0].Timestamp.Time().UnixNano(), "GetOrderAmends should retain nanosecond timestamps")
	values = requireSpotRequest(t, requests, "/0/private/OrderAmends")
	assert.Equal(t, "ORDER", values.Get("order_id"), "GetOrderAmends should encode the order identifier")

	_, err = ex.RequestExportReport(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "RequestExportReport must reject a nil request")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{})
	require.ErrorIs(t, err, errReportRequired, "RequestExportReport must require a report type")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: "invalid"})
	require.ErrorIs(t, err, errExportReportInvalid, "RequestExportReport must reject an invalid report type")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: "trades"})
	require.ErrorIs(t, err, errDescriptionRequired, "RequestExportReport must require a description")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: "trades", Description: "annual", Format: "JSON"})
	require.ErrorIs(t, err, errExportFormatInvalid, "RequestExportReport must reject an invalid format")
	start := time.Unix(1, 0)
	end := time.Unix(2, 0)
	preEpoch := time.Unix(-1, 0)
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Description: "annual", Fields: []ExportField{ExportFieldAmount}})
	require.ErrorIs(t, err, errExportFieldInvalid, "RequestExportReport must reject fields from a different report type")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Description: "annual", StartTime: preEpoch})
	require.ErrorIs(t, err, errTimestampInvalid, "RequestExportReport must reject a pre-epoch start time")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Description: "annual", EndTime: preEpoch})
	require.ErrorIs(t, err, errTimestampInvalid, "RequestExportReport must reject a pre-epoch end time")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Description: "annual", StartTime: end, EndTime: start})
	require.ErrorIs(t, err, errTimeRangeInvalid, "RequestExportReport must reject a reversed time range")
	report, err := ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Format: ExportFormatTSV, Description: "annual", Fields: []ExportField{ExportFieldTime, ExportFieldPrice}, StartTime: start, EndTime: end})
	require.NoError(t, err, "RequestExportReport must not error")
	assert.Equal(t, "REPORT", report.ID, "RequestExportReport should decode the report identifier")
	values = requireSpotRequest(t, requests, "/0/private/AddExport")
	assert.Equal(t, "annual", values.Get("description"), "RequestExportReport should encode description")
	assert.Equal(t, "TSV", values.Get("format"), "RequestExportReport should encode format")
	assert.Equal(t, "time,price", values.Get("fields"), "RequestExportReport should encode selected fields")
	assert.Equal(t, "1", values.Get("starttm"), "RequestExportReport should encode start time")
	assert.Equal(t, "2", values.Get("endtm"), "RequestExportReport should encode end time")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportLedgers, Description: "ledger", Fields: []ExportField{ExportFieldAmount, ExportFieldWallet}})
	require.NoError(t, err, "RequestExportReport must accept ledger fields")
	values = requireSpotRequest(t, requests, "/0/private/AddExport")
	assert.Equal(t, "amount,wallet", values.Get("fields"), "RequestExportReport should encode ledger fields")

	_, err = ex.GetExportReportStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetExportReportStatus must reject a nil request")
	_, err = ex.GetExportReportStatus(ctx, &GetExportReportStatusRequest{})
	require.ErrorIs(t, err, errReportRequired, "GetExportReportStatus must require a report type")
	_, err = ex.GetExportReportStatus(ctx, &GetExportReportStatusRequest{Report: "invalid"})
	require.ErrorIs(t, err, errExportReportInvalid, "GetExportReportStatus must reject an invalid report type")
	reports, err := ex.GetExportReportStatus(ctx, &GetExportReportStatusRequest{Report: "trades"})
	require.NoError(t, err, "GetExportReportStatus must not error")
	assert.Equal(t, []string{"currency"}, reports[0].AssetClasses, "GetExportReportStatus should decode current asset classes")
	assert.Equal(t, int64(1688669085), reports[0].EndTime.Time().Unix(), "GetExportReportStatus should decode report timestamps")
	requireSpotRequest(t, requests, "/0/private/ExportStatus")

	_, err = ex.RetrieveDataExport(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "RetrieveDataExport must reject a nil request")
	_, err = ex.RetrieveDataExport(ctx, &RetrieveDataExportRequest{})
	require.ErrorIs(t, err, errIDRequired, "RetrieveDataExport must require an identifier")
	archive, err := ex.RetrieveDataExport(ctx, &RetrieveDataExportRequest{ID: "REPORT"})
	require.NoError(t, err, "RetrieveDataExport must not error")
	assert.Equal(t, "PK\x03\x04export", string(archive), "RetrieveDataExport should preserve binary response bytes")
	requireSpotRequest(t, requests, "/0/private/RetrieveExport")

	_, err = ex.DeleteExportReport(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "DeleteExportReport must reject a nil request")
	_, err = ex.DeleteExportReport(ctx, &DeleteExportReportRequest{})
	require.ErrorIs(t, err, errIDRequired, "DeleteExportReport must require an identifier")
	_, err = ex.DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT"})
	require.ErrorIs(t, err, errTypeRequired, "DeleteExportReport must require a removal type")
	_, err = ex.DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT", Type: "invalid"})
	require.ErrorIs(t, err, errExportRemovalInvalid, "DeleteExportReport must reject an invalid removal type")
	deleted, err := ex.DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT", Type: "delete"})
	require.NoError(t, err, "DeleteExportReport must not error")
	assert.True(t, deleted.Delete, "DeleteExportReport should decode deletion status")
	requireSpotRequest(t, requests, "/0/private/RemoveExport")

	_, err = ex.GetAPIKeyInfo(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetAPIKeyInfo must reject a nil request")
	keyInfo, err := ex.GetAPIKeyInfo(ctx, &GetAPIKeyInfoRequest{OTP: "123456"})
	require.NoError(t, err, "GetAPIKeyInfo must not error")
	assert.Equal(t, "spot", keyInfo.APIKeyName, "GetAPIKeyInfo should decode the key name")
	assert.Nil(t, keyInfo.LastUsed, "GetAPIKeyInfo should decode a null last-used timestamp")
	assert.Equal(t, int64(1772542900), keyInfo.CreatedTime.Time().Unix(), "GetAPIKeyInfo should decode creation time")
	values = requireSpotRequest(t, requests, "/0/private/GetApiKeyInfo")
	assert.Equal(t, "123456", values.Get("otp"), "GetAPIKeyInfo should encode OTP")

	_, err = ex.GetLedgers(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetLedgers must reject a nil request")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Type: "invalid"})
	require.ErrorIs(t, err, errLedgerTypeInvalid, "GetLedgers must reject an invalid ledger type")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetLedgers must reject an invalid rebase multiplier")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Assets: []string{""}})
	require.ErrorIs(t, err, errAssetRequired, "GetLedgers must reject an empty asset")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Start: TimeOrTransactionID{Time: time.Unix(1, 0), TransactionID: "LEDGER"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetLedgers must reject conflicting start values")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{End: TimeOrTransactionID{Time: time.Unix(1, 0), TransactionID: "LEDGER"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetLedgers must reject conflicting end values")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Start: TimeOrTransactionID{Time: time.Unix(2, 0)}, End: TimeOrTransactionID{Time: time.Unix(1, 0)}})
	require.ErrorIs(t, err, errTimeRangeInvalid, "GetLedgers must reject a reversed time range")
	ledgers, err := ex.GetLedgers(ctx, new(GetLedgersRequest))
	require.NoError(t, err, "GetLedgers must allow omitted options")
	assert.Equal(t, int64(1), ledgers.Count, "GetLedgers should decode the ledger count")
	assert.Equal(t, "spotfromfutures", ledgers.Ledger["LEDGER"].Subtype, "GetLedgers should decode ledger subtype")
	requireSpotRequest(t, requests, "/0/private/Ledgers")
	ledgers, err = ex.GetLedgers(ctx, &GetLedgersRequest{
		AssetClass:       AssetClassCurrency,
		Assets:           []string{"XBT", "USD"},
		Type:             LedgerTypeTrade,
		Start:            TimeOrTransactionID{TransactionID: "1"},
		End:              TimeOrTransactionID{TransactionID: "2"},
		Offset:           3,
		WithoutCount:     true,
		RebaseMultiplier: RebaseMultiplierBase,
	})
	require.NoError(t, err, "GetLedgers must not error")
	assert.Equal(t, "REFERENCE", ledgers.Ledger["LEDGER"].Refid, "GetLedgers should decode ledger entries")
	values = requireSpotRequest(t, requests, "/0/private/Ledgers")
	assert.Equal(t, "currency", values.Get("aclass"), "GetLedgers should encode asset class")
	assert.Equal(t, "XBT,USD", values.Get("asset"), "GetLedgers should encode assets")
	assert.Equal(t, "trade", values.Get("type"), "GetLedgers should encode ledger type")
	assert.Equal(t, "1", values.Get("start"), "GetLedgers should encode start")
	assert.Equal(t, "2", values.Get("end"), "GetLedgers should encode end")
	assert.Equal(t, "3", values.Get("ofs"), "GetLedgers should encode offset")
	assert.Equal(t, "true", values.Get("without_count"), "GetLedgers should encode count suppression")
	assert.Equal(t, "base", values.Get("rebase_multiplier"), "GetLedgers should encode rebase multiplier")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Start: TimeOrTransactionID{Time: time.Unix(1, 0)}, End: TimeOrTransactionID{Time: time.Unix(2, 0)}})
	require.NoError(t, err, "GetLedgers must accept timestamp bounds")
	values = requireSpotRequest(t, requests, "/0/private/Ledgers")
	assert.Equal(t, "1", values.Get("start"), "GetLedgers should encode the start timestamp")
	assert.Equal(t, "2", values.Get("end"), "GetLedgers should encode the end timestamp")
}

func TestSpotLedgerTypes(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	for _, ledgerType := range []LedgerType{LedgerTypeAll, LedgerTypeTrade, LedgerTypeDeposit, LedgerTypeWithdrawal, LedgerTypeTransfer, LedgerTypeMargin, LedgerTypeAdjustment, LedgerTypeRollover, LedgerTypeCredit, LedgerTypeSettled, LedgerTypeStaking, LedgerTypeDividend, LedgerTypeSale, LedgerTypeNFTRebate} {
		t.Run(string(ledgerType), func(t *testing.T) {
			_, err := ex.GetLedgers(t.Context(), &GetLedgersRequest{Type: ledgerType})
			require.NoError(t, err, "GetLedgers must accept the documented ledger type")
			values := requireSpotRequest(t, requests, "/0/private/Ledgers")
			assert.Equal(t, string(ledgerType), values.Get("type"), "GetLedgers should encode the documented ledger type")
		})
	}
}

func TestSpotTradingEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()
	deadline := time.Now().Add(30 * time.Second).In(time.FixedZone("AEST", 10*60*60))
	orderQuantity := 2.0
	displayQuantity := 1.0

	_, err := ex.AmendOrder(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AmendOrder must reject a nil request")
	_, err = ex.AmendOrder(ctx, &AmendOrderRequest{})
	require.ErrorIs(t, err, errOrderIdentityRequired, "AmendOrder must require an order identifier")
	_, err = ex.AmendOrder(ctx, &AmendOrderRequest{TransactionID: "ORDER", ClientOrderID: "CLIENT"})
	require.ErrorIs(t, err, errOrderIdentityConflict, "AmendOrder must reject conflicting order identifiers")
	amended, err := ex.AmendOrder(ctx, &AmendOrderRequest{
		TransactionID:   "ORDER",
		OrderQuantity:   &orderQuantity,
		DisplayQuantity: &displayQuantity,
		LimitPrice:      &OrderPrice{Value: 100},
		TriggerPrice:    &OrderPrice{Value: 90},
		Pair:            spotTestPair,
		PostOnly:        true,
		Deadline:        deadline,
	})
	require.NoError(t, err, "AmendOrder must not error")
	assert.Equal(t, "AMEND", amended.AmendID, "AmendOrder should decode the amend identifier")
	values := requireSpotRequest(t, requests, "/0/private/AmendOrder")
	assert.Equal(t, "ORDER", values.Get("txid"), "AmendOrder should encode the transaction identifier")
	assert.Equal(t, "2", values.Get("order_qty"), "AmendOrder should encode order quantity")
	assert.Equal(t, "1", values.Get("display_qty"), "AmendOrder should encode display quantity")
	assert.Equal(t, "100", values.Get("limit_price"), "AmendOrder should encode limit price")
	assert.Equal(t, "90", values.Get("trigger_price"), "AmendOrder should encode trigger price")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "AmendOrder should encode the formatted pair")
	assert.Equal(t, "true", values.Get("post_only"), "AmendOrder should encode post-only")
	assert.Equal(t, deadline.UTC().Format(time.RFC3339Nano), values.Get("deadline"), "AmendOrder should encode deadline in UTC")

	_, err = ex.AmendOrder(ctx, &AmendOrderRequest{ClientOrderID: "CLIENT"})
	require.NoError(t, err, "AmendOrder must accept a client order identifier")
	values = requireSpotRequest(t, requests, "/0/private/AmendOrder")
	assert.Empty(t, values.Get("txid"), "AmendOrder should omit an empty transaction identifier")

	cancelled, err := ex.CancelAllOpenOrders(ctx)
	require.NoError(t, err, "CancelAllOpenOrders must not error")
	assert.Equal(t, int64(2), cancelled.Count, "CancelAllOpenOrders should decode the cancellation count")
	requireSpotRequest(t, requests, "/0/private/CancelAll")

	_, err = ex.CancelAllOrdersAfter(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CancelAllOrdersAfter must reject a nil request")
	_, err = ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{Timeout: 24 * time.Hour})
	require.ErrorIs(t, err, errTimeoutTooLarge, "CancelAllOrdersAfter must enforce Kraken's timeout limit")
	deadMan, err := ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{})
	require.NoError(t, err, "CancelAllOrdersAfter must accept zero to disable the timer")
	assert.False(t, deadMan.CurrentTime.IsZero(), "CancelAllOrdersAfter should decode the current time")
	values = requireSpotRequest(t, requests, "/0/private/CancelAllOrdersAfter")
	assert.Equal(t, "0", values.Get("timeout"), "CancelAllOrdersAfter should encode a zero timeout")

	_, err = ex.AddOrderBatch(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AddOrderBatch must reject a nil request")
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{})
	require.ErrorIs(t, err, errBatchOrderCount, "AddOrderBatch must require at least two orders")
	tooManyOrders := make([]AddOrderBatchOrderRequest, 16)
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: tooManyOrders})
	require.ErrorIs(t, err, errBatchOrderCount, "AddOrderBatch must reject more than fifteen orders")
	validOrders := []AddOrderBatchOrderRequest{
		{OrderType: OrderTypeLimit, OrderSide: OrderSideBuy, Volume: 1, Price: &OrderPrice{Value: 100}, Close: &AddOrderBatchCloseRequest{OrderType: OrderTypeStopLoss, Price: &OrderPrice{Value: 90}}},
		{OrderType: OrderTypeLimit, OrderSide: OrderSideSell, Volume: 1, Price: &OrderPrice{Value: 110}},
	}
	zeroUserReference := int32(0)
	validOrders[0].UserReference = &zeroUserReference
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: validOrders})
	require.ErrorIs(t, err, errPairRequired, "AddOrderBatch must require a pair")
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: validOrders, Pair: spotTestPair, AssetClass: AssetClassCurrency})
	require.ErrorIs(t, err, errAssetClassInvalid, "AddOrderBatch must reject an invalid asset class")
	invalidOrders := slices.Clone(validOrders)
	invalidOrders[1].OrderType = ""
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchOrderFields, "AddOrderBatch must require fields on every order")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].OrderType = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchOrderTypeInvalid, "AddOrderBatch must reject an invalid order type")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].OrderSide = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchSideInvalid, "AddOrderBatch must reject an invalid order side")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].Trigger = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchTriggerInvalid, "AddOrderBatch must reject an invalid trigger")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].SelfTradePolicy = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchSelfTradeInvalid, "AddOrderBatch must reject an invalid self-trade policy")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].TimeInForce = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchTimeInForceInvalid, "AddOrderBatch must reject an invalid time-in-force")
	invalidOrders = slices.Clone(validOrders)
	conflictingUserReference := int32(1)
	invalidOrders[1].UserReference = &conflictingUserReference
	invalidOrders[1].ClientOrderID = "CLIENT"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchIdentityConflict, "AddOrderBatch must reject conflicting client identifiers")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[0].Close = new(AddOrderBatchCloseRequest)
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchCloseTypeInvalid, "AddOrderBatch must require a conditional close order type")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[0].Close = &AddOrderBatchCloseRequest{OrderType: "invalid"}
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchCloseTypeInvalid, "AddOrderBatch must reject an invalid conditional close order type")
	batch, err := ex.AddOrderBatch(ctx, &AddOrderBatchRequest{
		Orders:     validOrders,
		Pair:       spotTestPair,
		AssetClass: AssetClassTokenizedAsset,
		Deadline:   deadline,
		Validate:   true,
		Broker:     "BROKER",
	})
	require.NoError(t, err, "AddOrderBatch must not error")
	assert.Equal(t, "ORDER", batch.Orders[0].Transaction, "AddOrderBatch should decode order identifiers")
	values = requireSpotRequest(t, requests, "/0/private/AddOrderBatch")
	assert.Contains(t, values.Get("orders"), `"close"`, "AddOrderBatch should encode conditional close orders")
	assert.Contains(t, values.Get("orders"), `"userref":0`, "AddOrderBatch should encode an explicit zero user reference")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "AddOrderBatch should encode the formatted pair")
	assert.Equal(t, "BROKER", values.Get("broker"), "AddOrderBatch should encode broker")
	assert.Equal(t, "true", values.Get("validate"), "AddOrderBatch should encode validation mode")
	assert.Equal(t, deadline.UTC().Format(time.RFC3339Nano), values.Get("deadline"), "AddOrderBatch should encode deadline in UTC")

	_, err = ex.CancelOrderBatch(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CancelOrderBatch must reject a nil request")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{})
	require.ErrorIs(t, err, errBatchCancelOrderCount, "CancelOrderBatch must require an identifier")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: make([]string, 51)})
	require.ErrorIs(t, err, errBatchCancelOrderCount, "CancelOrderBatch must reject more than fifty identifiers")
	batchCancelled, err := ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{
		TransactionIDs: []string{"ORDER"},
		UserReferences: []int32{42},
		ClientOrderIDs: []string{"CLIENT"},
	})
	require.NoError(t, err, "CancelOrderBatch must not error")
	assert.Equal(t, uint64(3), batchCancelled.Count, "CancelOrderBatch should decode the cancellation count")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrderBatch")
	assert.Equal(t, `["ORDER",42]`, values.Get("orders"), "CancelOrderBatch should encode primitive transaction and user identifiers")
	assert.Equal(t, `["CLIENT"]`, values.Get("cl_ord_ids"), "CancelOrderBatch should encode client order identifiers")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{ClientOrderIDs: []string{"CLIENT"}})
	require.NoError(t, err, "CancelOrderBatch must accept only client order identifiers")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrderBatch")
	assert.Empty(t, values.Get("orders"), "CancelOrderBatch should omit an empty orders list")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: []string{"ORDER"}})
	require.NoError(t, err, "CancelOrderBatch must accept only transaction identifiers")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrderBatch")
	assert.Empty(t, values.Get("cl_ord_ids"), "CancelOrderBatch should omit an empty client order identifier list")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{UserReferences: []int32{42}})
	require.NoError(t, err, "CancelOrderBatch must accept only user references")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrderBatch")
	assert.Equal(t, `[42]`, values.Get("orders"), "CancelOrderBatch should encode user references without transaction identifiers")
}

func TestSpotAddOrderBatchEnums(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	testValue := func(t *testing.T, mutate func(*AddOrderBatchRequest)) {
		t.Helper()
		req := &AddOrderBatchRequest{
			Orders: []AddOrderBatchOrderRequest{
				{OrderType: OrderTypeLimit, OrderSide: OrderSideBuy, Volume: 1},
				{OrderType: OrderTypeLimit, OrderSide: OrderSideSell, Volume: 1},
			},
			Pair: spotTestPair,
		}
		mutate(req)
		_, err := ex.AddOrderBatch(t.Context(), req)
		require.NoError(t, err, "AddOrderBatch must accept the documented enum value")
		requireSpotRequest(t, requests, "/0/private/AddOrderBatch")
	}

	for _, value := range []OrderType{OrderTypeMarket, OrderTypeLimit, OrderTypeIceberg, OrderTypeStopLoss, OrderTypeTakeProfit, OrderTypeStopLossLimit, OrderTypeTakeProfitLimit, OrderTypeTrailingStop, OrderTypeTrailingStopLimit, OrderTypeSettlePosition} {
		t.Run("order type "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].OrderType = value })
		})
	}
	for _, value := range []OrderSide{OrderSideBuy, OrderSideSell} {
		t.Run("side "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].OrderSide = value })
		})
	}
	for _, value := range []OrderTrigger{OrderTriggerIndex, OrderTriggerLast} {
		t.Run("trigger "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].Trigger = value })
		})
	}
	for _, value := range []SelfTradePolicy{SelfTradePolicyCancelNewest, SelfTradePolicyCancelOldest, SelfTradePolicyCancelBoth} {
		t.Run("self trade "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].SelfTradePolicy = value })
		})
	}
	for _, value := range []OrderTimeInForce{OrderTimeInForceGTC, OrderTimeInForceIOC, OrderTimeInForceGTD} {
		t.Run("time in force "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].TimeInForce = value })
		})
	}
	for _, value := range []OrderType{OrderTypeLimit, OrderTypeIceberg, OrderTypeStopLoss, OrderTypeTakeProfit, OrderTypeStopLossLimit, OrderTypeTakeProfitLimit, OrderTypeTrailingStop, OrderTypeTrailingStopLimit} {
		t.Run("close order type "+string(value), func(t *testing.T) {
			testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].Close = &AddOrderBatchCloseRequest{OrderType: value} })
		})
	}
	t.Run("asset class tokenized asset", func(t *testing.T) {
		testValue(t, func(req *AddOrderBatchRequest) { req.AssetClass = AssetClassTokenizedAsset })
	})
}

func TestSpotFundingEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()
	amount := 1.0
	maximumFee := 0.1
	start := time.Unix(1, 0)
	end := time.Unix(2, 0)

	_, err := ex.GetDepositMethods(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetDepositMethods must reject a nil request")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{})
	require.ErrorIs(t, err, errAssetRequired, "GetDepositMethods must require an asset")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT", AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetDepositMethods must reject an invalid asset class")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT", RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetDepositMethods must reject an invalid rebase multiplier")
	depositMethods, err := ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT", AssetClass: "tokenized_asset", RebaseMultiplier: "base"})
	require.NoError(t, err, "GetDepositMethods must not error")
	assert.True(t, depositMethods[0].GeneratesAddress, "GetDepositMethods should decode address generation support")
	assert.Equal(t, 0.1, depositMethods[0].FeePercent.Float64(), "GetDepositMethods should decode percentage fees")
	values := requireSpotRequest(t, requests, "/0/private/DepositMethods")
	assert.Equal(t, "tokenized_asset", values.Get("aclass"), "GetDepositMethods should encode asset class")
	assert.Equal(t, "base", values.Get("rebase_multiplier"), "GetDepositMethods should encode rebase multiplier")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT"})
	require.NoError(t, err, "GetDepositMethods must allow omitted optional parameters")
	requireSpotRequest(t, requests, "/0/private/DepositMethods")

	_, err = ex.GetDepositAddresses(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetDepositAddresses must reject a nil request")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{})
	require.ErrorIs(t, err, errAssetRequired, "GetDepositAddresses must require an asset")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errMethodRequired, "GetDepositAddresses must require a method")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin", AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetDepositAddresses must reject an invalid asset class")
	depositAddresses, err := ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", AssetClass: AssetClassCurrency, Method: "Bitcoin Lightning", New: true, Amount: &amount})
	require.NoError(t, err, "GetDepositAddresses must not error")
	assert.Equal(t, "MEMO", depositAddresses[0].Memo, "GetDepositAddresses should decode deposit memos")
	values = requireSpotRequest(t, requests, "/0/private/DepositAddresses")
	assert.Equal(t, "true", values.Get("new"), "GetDepositAddresses should encode address generation")
	assert.Equal(t, "1", values.Get("amount"), "GetDepositAddresses should encode Lightning deposit amounts")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin"})
	require.NoError(t, err, "GetDepositAddresses must allow omitted optional parameters")
	requireSpotRequest(t, requests, "/0/private/DepositAddresses")

	_, err = ex.GetWithdrawalInformation(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetWithdrawalInformation must reject a nil request")
	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{})
	require.ErrorIs(t, err, errAssetRequired, "GetWithdrawalInformation must require an asset")
	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errKeyRequired, "GetWithdrawalInformation must require a key")
	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet"})
	require.ErrorIs(t, err, errAmountInvalid, "GetWithdrawalInformation must require a positive amount")
	withdrawalInfo, err := ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet", Amount: amount})
	require.NoError(t, err, "GetWithdrawalInformation must not error")
	assert.Equal(t, 0.9, withdrawalInfo.Amount.Float64(), "GetWithdrawalInformation should decode the net amount")
	requireSpotRequest(t, requests, "/0/private/WithdrawInfo")

	_, err = ex.WithdrawFunds(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "WithdrawFunds must reject a nil request")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{})
	require.ErrorIs(t, err, errAssetRequired, "WithdrawFunds must require an asset")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errKeyRequired, "WithdrawFunds must require a key")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet"})
	require.ErrorIs(t, err, errAmountInvalid, "WithdrawFunds must require a positive amount")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: amount, AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "WithdrawFunds must reject an invalid asset class")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: amount, RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "WithdrawFunds must reject an invalid rebase multiplier")
	withdrawal, err := ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", AssetClass: AssetClassCurrency, Key: "wallet", Address: "bc1q", Amount: amount, MaximumFee: &maximumFee, RebaseMultiplier: RebaseMultiplierBase})
	require.NoError(t, err, "WithdrawFunds must not error")
	assert.Equal(t, "WITHDRAWAL", withdrawal.ReferenceID, "WithdrawFunds should decode the withdrawal reference")
	values = requireSpotRequest(t, requests, "/0/private/Withdraw")
	assert.Equal(t, "bc1q", values.Get("address"), "WithdrawFunds should encode a confirmation address")
	assert.Equal(t, "0.1", values.Get("max_fee"), "WithdrawFunds should encode a maximum fee")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: amount})
	require.NoError(t, err, "WithdrawFunds must allow omitted optional parameters")
	requireSpotRequest(t, requests, "/0/private/Withdraw")

	_, err = ex.GetRecentDepositsStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetRecentDepositsStatus must reject a nil request")
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetRecentDepositsStatus must reject an invalid asset class")
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetRecentDepositsStatus must reject an invalid rebase multiplier")
	depositLimit := uint64(25)
	deposits, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{
		Asset:            "XBT",
		AssetClass:       AssetClassCurrency,
		Method:           "Bitcoin",
		Start:            start,
		End:              end,
		Cursor:           "CURSOR",
		Limit:            &depositLimit,
		RebaseMultiplier: RebaseMultiplierBase,
	})
	require.NoError(t, err, "GetRecentDepositsStatus must not error")
	assert.Equal(t, "NEXT", deposits.NextCursor, "GetRecentDepositsStatus should decode pagination")
	assert.Equal(t, "REF", deposits.Deposits[0].ReferenceID, "GetRecentDepositsStatus should decode deposits")
	values = requireSpotRequest(t, requests, "/0/private/DepositStatus")
	assert.Equal(t, "CURSOR", values.Get("cursor"), "GetRecentDepositsStatus should encode a cursor")
	assert.Equal(t, "1", values.Get("start"), "GetRecentDepositsStatus should encode start time")
	assert.Equal(t, "2", values.Get("end"), "GetRecentDepositsStatus should encode end time")
	assert.Equal(t, "25", values.Get("limit"), "GetRecentDepositsStatus should encode a limit")
	paginate := true
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Paginate: &paginate})
	require.NoError(t, err, "GetRecentDepositsStatus must accept a pagination flag")
	values = requireSpotRequest(t, requests, "/0/private/DepositStatus")
	assert.Equal(t, "true", values.Get("cursor"), "GetRecentDepositsStatus should encode a pagination flag")
	depositLimit = 0
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Limit: &depositLimit})
	require.NoError(t, err, "GetRecentDepositsStatus must accept an explicit zero limit")
	values = requireSpotRequest(t, requests, "/0/private/DepositStatus")
	assert.Equal(t, "0", values.Get("limit"), "GetRecentDepositsStatus should encode an explicit zero limit")

	_, err = ex.GetRecentWithdrawalsStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetRecentWithdrawalsStatus must reject a nil request")
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetRecentWithdrawalsStatus must reject an invalid asset class")
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetRecentWithdrawalsStatus must reject an invalid rebase multiplier")
	withdrawalLimit := uint64(500)
	withdrawals, err := ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{
		Asset:            "XBT",
		AssetClass:       AssetClassCurrency,
		Method:           "Bitcoin",
		Start:            start,
		End:              end,
		Cursor:           "CURSOR",
		Limit:            &withdrawalLimit,
		RebaseMultiplier: RebaseMultiplierBase,
	})
	require.NoError(t, err, "GetRecentWithdrawalsStatus must not error")
	assert.Equal(t, "NEXT", withdrawals.NextCursor, "GetRecentWithdrawalsStatus should decode pagination")
	assert.Equal(t, "REF", withdrawals.Withdrawals[0].ReferenceID, "GetRecentWithdrawalsStatus should decode withdrawals")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawStatus")
	assert.Equal(t, "CURSOR", values.Get("cursor"), "GetRecentWithdrawalsStatus should encode a cursor")
	assert.Equal(t, "1", values.Get("start"), "GetRecentWithdrawalsStatus should encode start time")
	assert.Equal(t, "2", values.Get("end"), "GetRecentWithdrawalsStatus should encode end time")
	assert.Equal(t, "500", values.Get("limit"), "GetRecentWithdrawalsStatus should encode a limit")
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Paginate: &paginate})
	require.NoError(t, err, "GetRecentWithdrawalsStatus must accept a pagination flag")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawStatus")
	assert.Equal(t, "true", values.Get("cursor"), "GetRecentWithdrawalsStatus should encode a pagination flag")
	withdrawalLimit = 0
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Limit: &withdrawalLimit})
	require.NoError(t, err, "GetRecentWithdrawalsStatus must accept an explicit zero limit")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawStatus")
	assert.Equal(t, "0", values.Get("limit"), "GetRecentWithdrawalsStatus should encode an explicit zero limit")

	_, err = ex.GetWithdrawalMethods(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetWithdrawalMethods must reject a nil request")
	_, err = ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetWithdrawalMethods must reject an invalid asset class")
	_, err = ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetWithdrawalMethods must reject an invalid rebase multiplier")
	methods, err := ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{Asset: "XBT", AssetClass: "currency", Network: "Bitcoin", RebaseMultiplier: "base"})
	require.NoError(t, err, "GetWithdrawalMethods must not error")
	assert.Equal(t, "METHOD", methods[0].MethodID, "GetWithdrawalMethods should decode method identifiers")
	assert.Equal(t, 8.0, methods[0].Limits[0].Limits["86400"].Remaining.Float64(), "GetWithdrawalMethods should decode current rate limits")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawMethods")
	assert.Equal(t, "Bitcoin", values.Get("network"), "GetWithdrawalMethods should encode network")
	_, err = ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{})
	require.NoError(t, err, "GetWithdrawalMethods must allow unfiltered requests")
	requireSpotRequest(t, requests, "/0/private/WithdrawMethods")

	_, err = ex.GetWithdrawalAddresses(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetWithdrawalAddresses must reject a nil request")
	_, err = ex.GetWithdrawalAddresses(ctx, &GetWithdrawalAddressesRequest{AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetWithdrawalAddresses must reject an invalid asset class")
	verified := false
	addresses, err := ex.GetWithdrawalAddresses(ctx, &GetWithdrawalAddressesRequest{Asset: "XBT", AssetClass: "currency", Method: "Bitcoin", Key: "wallet", Verified: &verified})
	require.NoError(t, err, "GetWithdrawalAddresses must not error")
	assert.True(t, addresses[0].Verified, "GetWithdrawalAddresses should decode verification status")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawAddresses")
	assert.Equal(t, "false", values.Get("verified"), "GetWithdrawalAddresses should encode a false verification filter")
	_, err = ex.GetWithdrawalAddresses(ctx, &GetWithdrawalAddressesRequest{})
	require.NoError(t, err, "GetWithdrawalAddresses must allow unfiltered requests")
	requireSpotRequest(t, requests, "/0/private/WithdrawAddresses")

	_, err = ex.WalletTransfer(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "WalletTransfer must reject a nil request")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{})
	require.ErrorIs(t, err, errAssetRequired, "WalletTransfer must require an asset")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errFromRequired, "WalletTransfer must require a source wallet")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletFutures})
	require.ErrorIs(t, err, errFromWalletInvalid, "WalletTransfer must reject an invalid source wallet")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot})
	require.ErrorIs(t, err, errToRequired, "WalletTransfer must require a destination wallet")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletSpot})
	require.ErrorIs(t, err, errToWalletInvalid, "WalletTransfer must reject an invalid destination wallet")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures})
	require.ErrorIs(t, err, errAmountInvalid, "WalletTransfer must require a positive amount")
	transfer, err := ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures, Amount: amount})
	require.NoError(t, err, "WalletTransfer must not error")
	assert.Equal(t, "TRANSFER", transfer.ReferenceID, "WalletTransfer should decode the transfer reference")
	values = requireSpotRequest(t, requests, "/0/private/WalletTransfer")
	assert.Equal(t, "1", values.Get("amount"), "WalletTransfer should encode amount")

	_, err = ex.CreateSubaccount(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CreateSubaccount must reject a nil request")
	_, err = ex.CreateSubaccount(ctx, &CreateSubaccountRequest{})
	require.ErrorIs(t, err, errUsernameRequired, "CreateSubaccount must require a username")
	_, err = ex.CreateSubaccount(ctx, &CreateSubaccountRequest{Username: "subaccount"})
	require.ErrorIs(t, err, errEmailRequired, "CreateSubaccount must require an email address")
	created, err := ex.CreateSubaccount(ctx, &CreateSubaccountRequest{Username: "subaccount", Email: "subaccount@example.com"})
	require.NoError(t, err, "CreateSubaccount must not error")
	assert.True(t, created, "CreateSubaccount should decode success")
	requireSpotRequest(t, requests, "/0/private/CreateSubaccount")

	_, err = ex.AccountTransfer(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AccountTransfer must reject a nil request")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{})
	require.ErrorIs(t, err, errAssetRequired, "AccountTransfer must require an asset")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errAmountInvalid, "AccountTransfer must require a positive amount")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: amount})
	require.ErrorIs(t, err, errFromRequired, "AccountTransfer must require a source account")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: amount, From: "PRIMARY"})
	require.ErrorIs(t, err, errToRequired, "AccountTransfer must require a destination account")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", AssetClass: "invalid", Amount: amount, From: "PRIMARY", To: "SUB"})
	require.ErrorIs(t, err, errAssetClassInvalid, "AccountTransfer must reject an invalid asset class")
	accountTransfer, err := ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", AssetClass: AssetClassCurrency, Amount: amount, From: "PRIMARY", To: "SUB"})
	require.NoError(t, err, "AccountTransfer must not error")
	assert.Equal(t, "complete", accountTransfer.Status, "AccountTransfer should decode status")
	values = requireSpotRequest(t, requests, "/0/private/AccountTransfer")
	assert.Equal(t, "currency", values.Get("asset_class"), "AccountTransfer should encode asset class")
	assert.Equal(t, "1", values.Get("amount"), "AccountTransfer should encode amount")
}

func TestSpotEarnEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()

	_, err := ex.AllocateEarnFunds(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AllocateEarnFunds must reject a nil request")
	_, err = ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{})
	require.ErrorIs(t, err, errAmountInvalid, "AllocateEarnFunds must require a positive amount")
	_, err = ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1})
	require.ErrorIs(t, err, errStrategyIDRequired, "AllocateEarnFunds must require a strategy identifier")
	allocated, err := ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.NoError(t, err, "AllocateEarnFunds must not error")
	require.NotNil(t, allocated, "AllocateEarnFunds must decode a non-null result")
	assert.True(t, *allocated, "AllocateEarnFunds should decode success")
	values := requireSpotRequest(t, requests, "/0/private/Earn/Allocate")
	assert.Equal(t, "STRATEGY", values.Get("strategy_id"), "AllocateEarnFunds should encode the strategy identifier")
	assert.Equal(t, "1", values.Get("amount"), "AllocateEarnFunds should encode amount")

	_, err = ex.DeallocateEarnFunds(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "DeallocateEarnFunds must reject a nil request")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{})
	require.ErrorIs(t, err, errAmountInvalid, "DeallocateEarnFunds must require a positive amount")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1})
	require.ErrorIs(t, err, errStrategyIDRequired, "DeallocateEarnFunds must require a strategy identifier")
	deallocated, err := ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.NoError(t, err, "DeallocateEarnFunds must not error")
	require.NotNil(t, deallocated, "DeallocateEarnFunds must decode a non-null result")
	assert.True(t, *deallocated, "DeallocateEarnFunds should decode success")
	requireSpotRequest(t, requests, "/0/private/Earn/Deallocate")

	_, err = ex.GetEarnAllocationStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetEarnAllocationStatus must reject a nil request")
	_, err = ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{})
	require.ErrorIs(t, err, errStrategyIDRequired, "GetEarnAllocationStatus must require a strategy identifier")
	allocationStatus, err := ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.NoError(t, err, "GetEarnAllocationStatus must not error")
	require.NotNil(t, allocationStatus, "GetEarnAllocationStatus must decode a non-null result")
	assert.False(t, allocationStatus.Pending, "GetEarnAllocationStatus should decode pending status")
	requireSpotRequest(t, requests, "/0/private/Earn/AllocateStatus")

	_, err = ex.GetEarnDeallocationStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetEarnDeallocationStatus must reject a nil request")
	_, err = ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{})
	require.ErrorIs(t, err, errStrategyIDRequired, "GetEarnDeallocationStatus must require a strategy identifier")
	deallocationStatus, err := ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.NoError(t, err, "GetEarnDeallocationStatus must not error")
	require.NotNil(t, deallocationStatus, "GetEarnDeallocationStatus must decode a non-null result")
	assert.False(t, deallocationStatus.Pending, "GetEarnDeallocationStatus should decode pending status")
	requireSpotRequest(t, requests, "/0/private/Earn/DeallocateStatus")

	_, err = ex.ListEarnStrategies(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "ListEarnStrategies must reject a nil request")
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{LockType: []EarnLockType{""}})
	require.ErrorIs(t, err, errEarnLockTypeInvalid, "ListEarnStrategies must reject an empty lock type")
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{LockType: []EarnLockType{"invalid"}})
	require.ErrorIs(t, err, errEarnLockTypeInvalid, "ListEarnStrategies must reject an invalid lock type")
	ascending := true
	strategyLimit := uint16(25)
	strategies, err := ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{
		Ascending: &ascending,
		Asset:     "DOT",
		Cursor:    "CURSOR",
		Limit:     &strategyLimit,
		LockType:  []EarnLockType{EarnLockTypeFlexible, EarnLockTypeBonded, EarnLockTypeTimed, EarnLockTypeInstant},
	})
	require.NoError(t, err, "ListEarnStrategies must not error")
	require.NotNil(t, strategies, "ListEarnStrategies must decode a non-null result")
	assert.Equal(t, "staking", strategies.Items[0].YieldSource.Type, "ListEarnStrategies should expose staking-backed strategies")
	assert.Equal(t, "NEXT", strategies.NextCursor, "ListEarnStrategies should decode pagination")
	values = requireSpotRequest(t, requests, "/0/private/Earn/Strategies")
	assert.Equal(t, `["flex","bonded","timed","instant"]`, values.Get("lock_type"), "ListEarnStrategies should encode lock types as JSON")
	assert.Equal(t, "true", values.Get("ascending"), "ListEarnStrategies should encode sort direction")
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{})
	require.NoError(t, err, "ListEarnStrategies must allow an unfiltered request")
	requireSpotRequest(t, requests, "/0/private/Earn/Strategies")
	strategyLimit = 0
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{Limit: &strategyLimit})
	require.NoError(t, err, "ListEarnStrategies must accept an explicit zero limit")
	values = requireSpotRequest(t, requests, "/0/private/Earn/Strategies")
	assert.Equal(t, "0", values.Get("limit"), "ListEarnStrategies should encode an explicit zero limit")

	_, err = ex.ListEarnAllocations(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "ListEarnAllocations must reject a nil request")
	hideZero := false
	allocations, err := ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{Ascending: &ascending, ConvertedAsset: "USD", HideZeroAllocations: &hideZero})
	require.NoError(t, err, "ListEarnAllocations must not error")
	require.NotNil(t, allocations, "ListEarnAllocations must decode a non-null result")
	assert.Equal(t, "STRATEGY", allocations.Items[0].StrategyID, "ListEarnAllocations should decode strategy allocations")
	assert.Equal(t, "NEXT", allocations.NextCursor, "ListEarnAllocations should decode pagination")
	values = requireSpotRequest(t, requests, "/0/private/Earn/Allocations")
	assert.Equal(t, "false", values.Get("hide_zero_allocations"), "ListEarnAllocations should encode a false zero-allocation filter")
	_, err = ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{})
	require.NoError(t, err, "ListEarnAllocations must allow an unfiltered request")
	requireSpotRequest(t, requests, "/0/private/Earn/Allocations")
}

func TestRecentDepositsStatusResponseUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name          string
		payload       string
		expectedCount int
		expectedNext  string
		errExpected   bool
		expectedErr   error
	}{
		{name: "array", payload: `[{"refid":"ARRAY"}]`, expectedCount: 1},
		{name: "empty array", payload: `[]`},
		{name: "single", payload: `{"refid":"SINGLE"}`, expectedCount: 1},
		{name: "paginated array", payload: `{"deposit":[{"refid":"PAGE"}],"next_cursor":"NEXT"}`, expectedCount: 1, expectedNext: "NEXT"},
		{name: "paginated single", payload: `{"deposit":{"refid":"PAGE"},"next_cursor":"NEXT"}`, expectedCount: 1, expectedNext: "NEXT"},
		{name: "invalid paginated deposit", payload: `{"deposit":"invalid"}`, errExpected: true},
		{name: "invalid paginated deposit field", payload: `{"deposit":{"amount":{}}}`, errExpected: true},
		{name: "empty paginated deposit", payload: `{"deposit":{}}`, errExpected: true, expectedErr: errPaginatedDepositInvalid},
		{name: "null paginated deposit", payload: `{"deposit":null}`, errExpected: true, expectedErr: errPaginatedDepositInvalid},
		{name: "empty object", payload: `{}`, errExpected: true, expectedErr: errDepositResultInvalid},
		{name: "invalid deposit field", payload: `{"amount":{}}`, errExpected: true},
		{name: "unrelated object", payload: `{"unexpected":true}`, errExpected: true, expectedErr: errDepositResultInvalid},
		{name: "null", payload: `null`, errExpected: true, expectedErr: errDepositResultInvalid},
		{name: "invalid JSON", payload: `{`, errExpected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response RecentDepositsStatusResponse
			err := response.UnmarshalJSON([]byte(tc.payload))
			if tc.errExpected {
				if tc.expectedErr != nil {
					require.ErrorIs(t, err, tc.expectedErr, "UnmarshalJSON must return the expected deposit shape error")
					return
				}
				require.Error(t, err, "UnmarshalJSON must reject invalid deposit data")
				return
			}
			require.NoError(t, err, "UnmarshalJSON must not error")
			assert.Len(t, response.Deposits, tc.expectedCount, "UnmarshalJSON should normalise deposit results")
			assert.Equal(t, tc.expectedNext, response.NextCursor, "UnmarshalJSON should retain the next cursor")
		})
	}

	var reused RecentDepositsStatusResponse
	require.NoError(t, reused.UnmarshalJSON([]byte(`{"deposit":{"refid":"PAGE"},"next_cursor":"NEXT"}`)), "UnmarshalJSON must decode a reusable response")
	beforeError := reused
	err := reused.UnmarshalJSON([]byte(`{"deposit":{"amount":{}}}`))
	require.Error(t, err, "UnmarshalJSON must reject invalid data when reusing a response")
	assert.Equal(t, beforeError, reused, "UnmarshalJSON should not partially mutate the receiver on error")
	require.NoError(t, reused.UnmarshalJSON([]byte(`[{"refid":"ARRAY"}]`)), "UnmarshalJSON must replace a reusable response")
	assert.Empty(t, reused.NextCursor, "UnmarshalJSON should clear a stale cursor on a non-paginated response")
	assert.Equal(t, "ARRAY", reused.Deposits[0].ReferenceID, "UnmarshalJSON should replace stale deposit data")
}

func TestRecentWithdrawalsStatusResponseUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name          string
		payload       string
		expectedCount int
		expectedNext  string
		errExpected   bool
		expectedErr   error
	}{
		{name: "array", payload: `[{"refid":"ARRAY"}]`, expectedCount: 1},
		{name: "empty array", payload: `[]`},
		{name: "single", payload: `{"refid":"SINGLE"}`, expectedCount: 1},
		{name: "paginated array", payload: `{"withdrawal":[{"refid":"PAGE"}],"next_cursor":"NEXT"}`, expectedCount: 1, expectedNext: "NEXT"},
		{name: "paginated single", payload: `{"withdrawal":{"refid":"PAGE"},"next_cursor":"NEXT"}`, expectedCount: 1, expectedNext: "NEXT"},
		{name: "invalid paginated withdrawal", payload: `{"withdrawal":"invalid"}`, errExpected: true},
		{name: "invalid paginated withdrawal field", payload: `{"withdrawal":{"amount":{}}}`, errExpected: true},
		{name: "empty paginated withdrawal", payload: `{"withdrawal":{}}`, errExpected: true, expectedErr: errPaginatedWithdrawalInvalid},
		{name: "null paginated withdrawal", payload: `{"withdrawal":null}`, errExpected: true, expectedErr: errPaginatedWithdrawalInvalid},
		{name: "empty object", payload: `{}`, errExpected: true, expectedErr: errWithdrawalResultInvalid},
		{name: "invalid withdrawal field", payload: `{"amount":{}}`, errExpected: true},
		{name: "unrelated object", payload: `{"unexpected":true}`, errExpected: true, expectedErr: errWithdrawalResultInvalid},
		{name: "null", payload: `null`, errExpected: true, expectedErr: errWithdrawalResultInvalid},
		{name: "invalid JSON", payload: `{`, errExpected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response RecentWithdrawalsStatusResponse
			err := response.UnmarshalJSON([]byte(tc.payload))
			if tc.errExpected {
				if tc.expectedErr != nil {
					require.ErrorIs(t, err, tc.expectedErr, "UnmarshalJSON must return the expected withdrawal shape error")
					return
				}
				require.Error(t, err, "UnmarshalJSON must reject invalid withdrawal data")
				return
			}
			require.NoError(t, err, "UnmarshalJSON must not error")
			assert.Len(t, response.Withdrawals, tc.expectedCount, "UnmarshalJSON should normalise withdrawal results")
			assert.Equal(t, tc.expectedNext, response.NextCursor, "UnmarshalJSON should retain the next cursor")
		})
	}

	var reused RecentWithdrawalsStatusResponse
	require.NoError(t, reused.UnmarshalJSON([]byte(`{"withdrawal":{"refid":"PAGE"},"next_cursor":"NEXT"}`)), "UnmarshalJSON must decode a reusable response")
	beforeError := reused
	err := reused.UnmarshalJSON([]byte(`{"withdrawal":{"amount":{}}}`))
	require.Error(t, err, "UnmarshalJSON must reject invalid data when reusing a response")
	assert.Equal(t, beforeError, reused, "UnmarshalJSON should not partially mutate the receiver on error")
	require.NoError(t, reused.UnmarshalJSON([]byte(`[{"refid":"ARRAY"}]`)), "UnmarshalJSON must replace a reusable response")
	assert.Empty(t, reused.NextCursor, "UnmarshalJSON should clear a stale cursor on a non-paginated response")
	assert.Equal(t, "ARRAY", reused.Withdrawals[0].ReferenceID, "UnmarshalJSON should replace stale withdrawal data")
}

func TestIsValidSpotEnum(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		value    string
		expected bool
	}{
		{name: "empty optional value", expected: true},
		{name: "allowed value", value: "allowed", expected: true},
		{name: "invalid value", value: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, isValidSpotEnum(tc.value, "allowed"), "isValidSpotEnum should validate documented values")
		})
	}
}

func TestContainsDepositField(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		fields   map[string]json.RawMessage
		expected bool
	}{
		{name: "nil fields"},
		{name: "unrelated field", fields: map[string]json.RawMessage{"unexpected": nil}},
		{name: "deposit field", fields: map[string]json.RawMessage{"refid": nil}, expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, containsDepositField(tc.fields), "containsDepositField should identify known deposit fields")
		})
	}
}

func TestContainsWithdrawalField(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		fields   map[string]json.RawMessage
		expected bool
	}{
		{name: "nil fields"},
		{name: "unrelated field", fields: map[string]json.RawMessage{"unexpected": nil}},
		{name: "withdrawal field", fields: map[string]json.RawMessage{"refid": nil}, expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, containsWithdrawalField(tc.fields), "containsWithdrawalField should identify known withdrawal fields")
		})
	}
}

func TestSendAuthenticatedHTTPRequestRawResult(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)

	var nilRaw *request.RawResponse
	err := ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RetrieveExport", url.Values{}, nilRaw)
	require.ErrorIs(t, err, common.ErrNilPointer, "SendAuthenticatedHTTPRequest must reject a nil raw result pointer")

	var raw request.RawResponse
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RetrieveExport", url.Values{"id": {"REPORT"}}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must not error for a raw response")
	assert.Equal(t, "PK\x03\x04export", string(raw), "SendAuthenticatedHTTPRequest should preserve raw bytes")
	requireSpotRequest(t, requests, "/0/private/RetrieveExport")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RetrieveExportError", url.Values{}, &raw)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must surface JSON errors returned by a raw endpoint")
	requireSpotRequest(t, requests, "/0/private/RetrieveExportError")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RawJSON", url.Values{}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must accept an error-free JSON response from a raw endpoint")
	requireSpotRequest(t, requests, "/0/private/RawJSON")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RawScalar", url.Values{}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must preserve a scalar JSON export body")
	assert.Equal(t, "123", string(raw), "SendAuthenticatedHTTPRequest should preserve a scalar JSON export body")
	requireSpotRequest(t, requests, "/0/private/RawScalar")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "RawObject", url.Values{}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must preserve a JSON object without an API error envelope")
	assert.JSONEq(t, `{"report":"data"}`, string(raw), "SendAuthenticatedHTTPRequest should preserve a JSON object without an API error envelope")
	requireSpotRequest(t, requests, "/0/private/RawObject")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "SemanticError", url.Values{}, &raw)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must reject semantically invalid JSON from a raw endpoint")
	requireSpotRequest(t, requests, "/0/private/SemanticError")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "Warning", url.Values{}, &raw)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must allow warnings returned by a raw endpoint")
	requireSpotRequest(t, requests, "/0/private/Warning")

	var result AmendOrderResponse
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "AmendOrder", url.Values{"txid": {"ORDER"}}, &result)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must not error for a JSON response")
	assert.Equal(t, "AMEND", result.AmendID, "SendAuthenticatedHTTPRequest should decode JSON results")
	requireSpotRequest(t, requests, "/0/private/AmendOrder")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "NormalError", url.Values{}, &result)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must surface JSON API errors")
	requireSpotRequest(t, requests, "/0/private/NormalError")
	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "SemanticError", url.Values{}, &result)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must reject semantically invalid JSON")
	requireSpotRequest(t, requests, "/0/private/SemanticError")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "Malformed", url.Values{}, &result)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SendAuthenticatedHTTPRequest must reject malformed JSON")
	requireSpotRequest(t, requests, "/0/private/Malformed")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "Warning", url.Values{}, &result)
	require.NoError(t, err, "SendAuthenticatedHTTPRequest must allow warning-only responses")
	requireSpotRequest(t, requests, "/0/private/Warning")

	err = new(Exchange).SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "AmendOrder", url.Values{}, &result)
	require.Error(t, err, "SendAuthenticatedHTTPRequest must reject unavailable credentials")

	err = ex.SendAuthenticatedHTTPRequest(t.Context(), exchange.URL(255), "AmendOrder", url.Values{}, &result)
	require.Error(t, err, "SendAuthenticatedHTTPRequest must reject an invalid endpoint type")

	err = newSpotErrorExchange(t).SendAuthenticatedHTTPRequest(t.Context(), exchange.RestSpot, "AmendOrder", url.Values{}, &result)
	require.ErrorIs(t, err, errSpotTransport, "SendAuthenticatedHTTPRequest must surface transport errors")
}

func TestSpotEndpointErrors(t *testing.T) {
	ex := newSpotErrorExchange(t)
	ctx := t.Context()
	validBatch := []AddOrderBatchOrderRequest{
		{OrderType: OrderTypeLimit, OrderSide: OrderSideBuy, Volume: 1},
		{OrderType: OrderTypeLimit, OrderSide: OrderSideSell, Volume: 1},
	}

	_, err := ex.GetSystemStatus(ctx)
	require.Error(t, err, "GetSystemStatus must surface request errors")
	_, err = ex.GetCurrentServerTime(ctx)
	require.Error(t, err, "GetCurrentServerTime must surface request errors")
	_, err = ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair})
	require.Error(t, err, "GetGroupedOrderBook must surface request errors")
	_, err = ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair})
	require.Error(t, err, "QueryLevel3OrderBook must surface request errors")
	_, err = ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: spotTestPair})
	require.Error(t, err, "GetPreTradeData must surface request errors")
	_, err = ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{})
	require.Error(t, err, "GetPostTradeData must surface request errors")
	_, err = ex.GetAccountBalance(ctx, &GetAccountBalanceRequest{})
	require.Error(t, err, "GetAccountBalance must surface request errors")
	_, err = ex.GetExtendedBalance(ctx, &GetExtendedBalanceRequest{})
	require.Error(t, err, "GetExtendedBalance must surface request errors")
	_, err = ex.GetCreditLines(ctx, &GetCreditLinesRequest{})
	require.Error(t, err, "GetCreditLines must surface request errors")
	_, err = ex.GetOrderAmends(ctx, &GetOrderAmendsRequest{OrderID: "ORDER"})
	require.Error(t, err, "GetOrderAmends must surface request errors")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: "trades", Description: "test"})
	require.Error(t, err, "RequestExportReport must surface request errors")
	_, err = ex.GetExportReportStatus(ctx, &GetExportReportStatusRequest{Report: "trades"})
	require.Error(t, err, "GetExportReportStatus must surface request errors")
	_, err = ex.RetrieveDataExport(ctx, &RetrieveDataExportRequest{ID: "REPORT"})
	require.Error(t, err, "RetrieveDataExport must surface request errors")
	_, err = ex.DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT", Type: "delete"})
	require.Error(t, err, "DeleteExportReport must surface request errors")
	_, err = ex.GetAPIKeyInfo(ctx, &GetAPIKeyInfoRequest{})
	require.Error(t, err, "GetAPIKeyInfo must surface request errors")
	_, err = ex.GetLedgers(ctx, new(GetLedgersRequest))
	require.Error(t, err, "GetLedgers must surface request errors")
	_, err = ex.AmendOrder(ctx, &AmendOrderRequest{TransactionID: "ORDER"})
	require.Error(t, err, "AmendOrder must surface request errors")
	_, err = ex.CancelAllOpenOrders(ctx)
	require.Error(t, err, "CancelAllOpenOrders must surface request errors")
	_, err = ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{})
	require.Error(t, err, "CancelAllOrdersAfter must surface request errors")
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: validBatch, Pair: spotTestPair})
	require.Error(t, err, "AddOrderBatch must surface request errors")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: []string{"ORDER"}})
	require.Error(t, err, "CancelOrderBatch must surface request errors")
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{})
	require.Error(t, err, "GetRecentDepositsStatus must surface request errors")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT"})
	require.Error(t, err, "GetDepositMethods must surface request errors")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin"})
	require.Error(t, err, "GetDepositAddresses must surface request errors")
	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet", Amount: 1})
	require.Error(t, err, "GetWithdrawalInformation must surface request errors")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: 1})
	require.Error(t, err, "WithdrawFunds must surface request errors")
	_, err = ex.CancelWithdrawal(ctx, &CancelWithdrawalRequest{Asset: "BTC", ReferenceID: "REFERENCE"})
	require.Error(t, err, "CancelWithdrawal must surface request errors")
	_, err = ex.GetWebsocketToken(ctx)
	require.Error(t, err, "GetWebsocketToken must surface request errors")
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{})
	require.Error(t, err, "GetRecentWithdrawalsStatus must surface request errors")
	_, err = ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{})
	require.Error(t, err, "GetWithdrawalMethods must surface request errors")
	_, err = ex.GetWithdrawalAddresses(ctx, &GetWithdrawalAddressesRequest{})
	require.Error(t, err, "GetWithdrawalAddresses must surface request errors")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures, Amount: 1})
	require.Error(t, err, "WalletTransfer must surface request errors")
	_, err = ex.CreateSubaccount(ctx, &CreateSubaccountRequest{Username: "subaccount", Email: "subaccount@example.com"})
	require.Error(t, err, "CreateSubaccount must surface request errors")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: 1, From: "PRIMARY", To: "SUB"})
	require.Error(t, err, "AccountTransfer must surface request errors")
	_, err = ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.Error(t, err, "AllocateEarnFunds must surface request errors")
	_, err = ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
	require.Error(t, err, "DeallocateEarnFunds must surface request errors")
	_, err = ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.Error(t, err, "GetEarnAllocationStatus must surface request errors")
	_, err = ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
	require.Error(t, err, "GetEarnDeallocationStatus must surface request errors")
	_, err = ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{})
	require.Error(t, err, "ListEarnStrategies must surface request errors")
	_, err = ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{})
	require.Error(t, err, "ListEarnAllocations must surface request errors")

	bareEx := new(Exchange)
	_, err = bareEx.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair})
	require.Error(t, err, "GetGroupedOrderBook must surface pair-format errors")
	_, err = bareEx.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair})
	require.Error(t, err, "QueryLevel3OrderBook must surface pair-format errors")
}

func TestSpotResponseObjectResults(t *testing.T) {
	successEx, _ := newSpotEndpointExchange(t)
	nilResultEx := newSpotNullResultExchange(t)
	errorEx := newSpotErrorExchange(t)
	ctx := t.Context()
	validBatch := []AddOrderBatchOrderRequest{
		{OrderType: OrderTypeLimit, OrderSide: OrderSideBuy, Volume: 1},
		{OrderType: OrderTypeLimit, OrderSide: OrderSideSell, Volume: 1},
	}

	for _, tc := range []struct {
		name            string
		call            func(*Exchange) (any, error)
		expectedJSON    string
		zeroValueOnNull bool
	}{
		{
			name:         "GetWebsocketToken",
			call:         func(ex *Exchange) (any, error) { return ex.GetWebsocketToken(ctx) },
			expectedJSON: `"token":"TOKEN"`,
		},
		{
			name:         "GetCreditLines",
			call:         func(ex *Exchange) (any, error) { return ex.GetCreditLines(ctx, &GetCreditLinesRequest{}) },
			expectedJSON: `"asset_details":{"USD"`,
		},
		{
			name:         "GetTradeBalance",
			call:         func(ex *Exchange) (any, error) { return ex.GetTradeBalance(ctx, &GetTradeBalanceRequest{}) },
			expectedJSON: `"eb":"1101.3425"`,
		},
		{
			name:         "GetOpenOrders",
			call:         func(ex *Exchange) (any, error) { return ex.GetOpenOrders(ctx, &GetOpenOrdersRequest{}) },
			expectedJSON: `"open":{"ORDER"`,
		},
		{
			name:         "GetClosedOrders",
			call:         func(ex *Exchange) (any, error) { return ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{}) },
			expectedJSON: `"closed":{"ORDER"`,
		},
		{
			name:         "GetTradesHistory",
			call:         func(ex *Exchange) (any, error) { return ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{}) },
			expectedJSON: `"trades":{"TRADE"`,
		},
		{
			name:         "GetLedgers",
			call:         func(ex *Exchange) (any, error) { return ex.GetLedgers(ctx, &GetLedgersRequest{}) },
			expectedJSON: `"ledger":{"LEDGER"`,
		},
		{
			name:         "GetTradeVolume",
			call:         func(ex *Exchange) (any, error) { return ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{}) },
			expectedJSON: `"currency":"USD"`,
		},
		{
			name: "GetOrderAmends",
			call: func(ex *Exchange) (any, error) {
				return ex.GetOrderAmends(ctx, &GetOrderAmendsRequest{OrderID: "ORDER"})
			},
			expectedJSON: `"amend_id":"AMEND"`,
		},
		{
			name: "RequestExportReport",
			call: func(ex *Exchange) (any, error) {
				return ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: "trades", Description: "test"})
			},
			expectedJSON: `"id":"REPORT"`,
		},
		{
			name: "DeleteExportReport",
			call: func(ex *Exchange) (any, error) {
				return ex.DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT", Type: "delete"})
			},
			expectedJSON: `"delete":true`,
		},
		{
			name:         "GetAPIKeyInfo",
			call:         func(ex *Exchange) (any, error) { return ex.GetAPIKeyInfo(ctx, &GetAPIKeyInfoRequest{}) },
			expectedJSON: `"apiKeyName":"spot"`,
		},
		{
			name: "GetWithdrawalInformation",
			call: func(ex *Exchange) (any, error) {
				return ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet", Amount: 1})
			},
			expectedJSON: `"method":"Bitcoin"`,
		},
		{
			name: "WithdrawFunds",
			call: func(ex *Exchange) (any, error) {
				return ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: 1})
			},
			expectedJSON: `"refid":"WITHDRAWAL"`,
		},
		{
			name: "GetRecentDepositsStatus",
			call: func(ex *Exchange) (any, error) {
				return ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{})
			},
			expectedJSON: `"NextCursor":"NEXT"`,
		},
		{
			name: "GetRecentWithdrawalsStatus",
			call: func(ex *Exchange) (any, error) {
				return ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{})
			},
			expectedJSON: `"NextCursor":"NEXT"`,
		},
		{
			name: "WalletTransfer",
			call: func(ex *Exchange) (any, error) {
				return ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures, Amount: 1})
			},
			expectedJSON: `"refid":"TRANSFER"`,
		},
		{
			name: "AccountTransfer",
			call: func(ex *Exchange) (any, error) {
				return ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: 1, From: "PRIMARY", To: "SUB"})
			},
			expectedJSON: `"transfer_id":"TRANSFER"`,
		},
		{
			name:         "GetSystemStatus",
			call:         func(ex *Exchange) (any, error) { return ex.GetSystemStatus(ctx) },
			expectedJSON: `"status":"online"`,
		},
		{
			name:         "GetOHLC",
			call:         func(ex *Exchange) (any, error) { return ex.GetOHLC(ctx, &GetOHLCRequest{Pair: spotTestPair}) },
			expectedJSON: `"Candles":{"BTC/USD":[{`,
		},
		{
			name: "GetTrades",
			call: func(ex *Exchange) (any, error) {
				return ex.GetTrades(ctx, &GetTradesRequest{Pair: spotTestPair})
			},
			expectedJSON: `"Trades":{"BTC/USD":[{`,
		},
		{
			name: "GetSpread",
			call: func(ex *Exchange) (any, error) {
				return ex.GetSpread(ctx, &GetSpreadRequest{Pair: spotTestPair})
			},
			expectedJSON: `"Spreads":{"BTC/USD":[{`,
		},
		{
			name: "GetGroupedOrderBook",
			call: func(ex *Exchange) (any, error) {
				return ex.GetGroupedOrderBook(ctx, &GroupedOrderBookRequest{Pair: spotTestPair})
			},
			expectedJSON: `"pair":"XBTUSD"`,
		},
		{
			name: "QueryLevel3OrderBook",
			call: func(ex *Exchange) (any, error) {
				return ex.QueryLevel3OrderBook(ctx, &QueryLevel3OrderBookRequest{Pair: spotTestPair})
			},
			expectedJSON: `"pair":"XBTUSD"`,
		},
		{
			name: "GetPreTradeData",
			call: func(ex *Exchange) (any, error) {
				return ex.GetPreTradeData(ctx, &GetPreTradeDataRequest{Pair: spotTestPair})
			},
			expectedJSON: `"symbol":"BTC/USD"`,
		},
		{
			name:         "GetPostTradeData",
			call:         func(ex *Exchange) (any, error) { return ex.GetPostTradeData(ctx, &GetPostTradeDataRequest{}) },
			expectedJSON: `"trade_id":"TRADE"`,
		},
		{
			name: "AddOrder",
			call: func(ex *Exchange) (any, error) {
				return ex.AddOrder(ctx, &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Volume: 1, Pair: spotTestPair})
			},
			expectedJSON: `"txid":["ORDER"]`,
		},
		{
			name: "CancelExistingOrder",
			call: func(ex *Exchange) (any, error) {
				return ex.CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER"})
			},
			expectedJSON: `"count":1`,
		},
		{
			name:         "AmendOrder",
			call:         func(ex *Exchange) (any, error) { return ex.AmendOrder(ctx, &AmendOrderRequest{TransactionID: "ORDER"}) },
			expectedJSON: `"amend_id":"AMEND"`,
		},
		{
			name:         "CancelAllOpenOrders",
			call:         func(ex *Exchange) (any, error) { return ex.CancelAllOpenOrders(ctx) },
			expectedJSON: `"count":2`,
		},
		{
			name:         "CancelAllOrdersAfter",
			call:         func(ex *Exchange) (any, error) { return ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{}) },
			expectedJSON: `"currentTime":"2026-08-02T00:00:00Z"`,
		},
		{
			name: "AddOrderBatch",
			call: func(ex *Exchange) (any, error) {
				return ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: validBatch, Pair: spotTestPair})
			},
			expectedJSON: `"txid":"ORDER"`,
		},
		{
			name: "CancelOrderBatch",
			call: func(ex *Exchange) (any, error) {
				return ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: []string{"ORDER"}})
			},
			expectedJSON: `"count":3`,
		},
		{
			name:            "AllocateEarnFunds",
			zeroValueOnNull: true,
			call: func(ex *Exchange) (any, error) {
				return ex.AllocateEarnFunds(ctx, &AllocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
			},
			expectedJSON: `true`,
		},
		{
			name:            "DeallocateEarnFunds",
			zeroValueOnNull: true,
			call: func(ex *Exchange) (any, error) {
				return ex.DeallocateEarnFunds(ctx, &DeallocateEarnFundsRequest{Amount: 1, StrategyID: "STRATEGY"})
			},
			expectedJSON: `true`,
		},
		{
			name: "GetEarnAllocationStatus",
			call: func(ex *Exchange) (any, error) {
				return ex.GetEarnAllocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
			},
			expectedJSON: `"pending":false`,
		},
		{
			name: "GetEarnDeallocationStatus",
			call: func(ex *Exchange) (any, error) {
				return ex.GetEarnDeallocationStatus(ctx, &EarnOperationStatusRequest{StrategyID: "STRATEGY"})
			},
			expectedJSON: `"pending":false`,
		},
		{
			name:         "ListEarnStrategies",
			call:         func(ex *Exchange) (any, error) { return ex.ListEarnStrategies(ctx, &ListEarnStrategiesRequest{}) },
			expectedJSON: `"id":"STRATEGY"`,
		},
		{
			name:         "ListEarnAllocations",
			call:         func(ex *Exchange) (any, error) { return ex.ListEarnAllocations(ctx, &ListEarnAllocationsRequest{}) },
			expectedJSON: `"strategy_id":"STRATEGY"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call(successEx)
			require.NoError(t, err, tc.name+" must not error")
			require.NotNil(t, result, tc.name+" must return a response")
			responseJSON, err := json.Marshal(result)
			require.NoError(t, err, tc.name+" must encode the decoded response")
			assert.Contains(t, string(responseJSON), tc.expectedJSON, tc.name+" should decode the response")

			result, err = tc.call(nilResultEx)
			require.NoError(t, err, tc.name+" must accept a null result")
			if tc.zeroValueOnNull {
				require.NotNil(t, result, tc.name+" must return a zero-value scalar for a null result")
				assert.True(t, reflect.ValueOf(result).Elem().IsZero(), tc.name+" should return the zero-value scalar for a null result")
			} else {
				assert.Nil(t, result, tc.name+" should return nil for a null result")
			}

			result, err = tc.call(errorEx)
			require.ErrorIs(t, err, errSpotTransport, tc.name+" must surface request errors")
			assert.Nil(t, result, tc.name+" result should remain nil on request errors")
		})
	}
}

func TestSpotTradingTypedRequestValidation(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t)
	ctx := t.Context()
	negative := -1.0
	notANumber := math.NaN()
	invalidDeadline := time.Now().Add(time.Second)
	validAddOrder := AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Volume: 1, Pair: spotTestPair}

	for _, tc := range []struct {
		name     string
		mutate   func(*AddOrderRequest)
		expected error
	}{
		{name: "negative volume", mutate: func(req *AddOrderRequest) { req.Volume = negative }, expected: errVolumeInvalid},
		{name: "non-finite volume", mutate: func(req *AddOrderRequest) { req.Volume = notANumber }, expected: errNumericValueInvalid},
		{name: "negative display volume", mutate: func(req *AddOrderRequest) { req.DisplayVolume = &negative }, expected: errVolumeInvalid},
		{name: "non-finite display volume", mutate: func(req *AddOrderRequest) { req.DisplayVolume = &notANumber }, expected: errNumericValueInvalid},
		{name: "invalid price", mutate: func(req *AddOrderRequest) { req.Price = &OrderPrice{Expression: "invalid"} }, expected: errOrderPriceInvalid},
		{name: "invalid secondary price", mutate: func(req *AddOrderRequest) { req.SecondaryPrice = &OrderPrice{Expression: "invalid"} }, expected: errOrderPriceInvalid},
		{name: "invalid start delay", mutate: func(req *AddOrderRequest) { req.StartDelay = time.Millisecond }, expected: errScheduledTimeInvalid},
		{name: "invalid expiry delay", mutate: func(req *AddOrderRequest) { req.ExpireAfter = time.Second }, expected: errScheduledTimeInvalid},
		{name: "invalid deadline", mutate: func(req *AddOrderRequest) { req.Deadline = invalidDeadline }, expected: errDeadlineInvalid},
		{name: "invalid order flag", mutate: func(req *AddOrderRequest) { req.OrderFlags = []OrderFlag{"invalid"} }, expected: errOrderFlagInvalid},
		{name: "invalid close price", mutate: func(req *AddOrderRequest) {
			req.Close = &AddOrderCloseRequest{OrderType: OrderTypeLimit, Price: &OrderPrice{Expression: "invalid"}}
		}, expected: errOrderPriceInvalid},
		{name: "invalid close secondary price", mutate: func(req *AddOrderRequest) {
			req.Close = &AddOrderCloseRequest{OrderType: OrderTypeLimit, SecondaryPrice: &OrderPrice{Expression: "invalid"}}
		}, expected: errOrderPriceInvalid},
	} {
		t.Run("AddOrder "+tc.name, func(t *testing.T) {
			req := validAddOrder
			tc.mutate(&req)
			_, err := ex.AddOrder(ctx, &req)
			require.ErrorIs(t, err, tc.expected, "AddOrder must reject "+tc.name)
		})
	}

	_, err := new(Exchange).AddOrder(ctx, &validAddOrder)
	require.Error(t, err, "AddOrder must surface pair-format errors")

	for _, tc := range []struct {
		name     string
		request  AmendOrderRequest
		expected error
	}{
		{name: "negative order quantity", request: AmendOrderRequest{TransactionID: "ORDER", OrderQuantity: &negative}, expected: errVolumeInvalid},
		{name: "non-finite order quantity", request: AmendOrderRequest{TransactionID: "ORDER", OrderQuantity: &notANumber}, expected: errNumericValueInvalid},
		{name: "negative display quantity", request: AmendOrderRequest{TransactionID: "ORDER", DisplayQuantity: &negative}, expected: errVolumeInvalid},
		{name: "non-finite display quantity", request: AmendOrderRequest{TransactionID: "ORDER", DisplayQuantity: &notANumber}, expected: errNumericValueInvalid},
		{name: "invalid limit price", request: AmendOrderRequest{TransactionID: "ORDER", LimitPrice: &OrderPrice{Expression: "invalid"}}, expected: errOrderPriceInvalid},
		{name: "unsupported relative limit price", request: AmendOrderRequest{TransactionID: "ORDER", LimitPrice: &OrderPrice{Expression: "#1"}}, expected: errOrderPriceInvalid},
		{name: "invalid trigger price", request: AmendOrderRequest{TransactionID: "ORDER", TriggerPrice: &OrderPrice{Expression: "invalid"}}, expected: errOrderPriceInvalid},
		{name: "unsupported relative trigger price", request: AmendOrderRequest{TransactionID: "ORDER", TriggerPrice: &OrderPrice{Expression: "#1"}}, expected: errOrderPriceInvalid},
		{name: "invalid deadline", request: AmendOrderRequest{TransactionID: "ORDER", Deadline: invalidDeadline}, expected: errDeadlineInvalid},
		{name: "partially populated pair", request: AmendOrderRequest{TransactionID: "ORDER", Pair: currency.Pair{Base: currency.BTC}}, expected: errPairRequired},
	} {
		t.Run("AmendOrder "+tc.name, func(t *testing.T) {
			_, err := ex.AmendOrder(ctx, &tc.request)
			require.ErrorIs(t, err, tc.expected, "AmendOrder must reject "+tc.name)
		})
	}
	_, err = new(Exchange).AmendOrder(ctx, &AmendOrderRequest{TransactionID: "ORDER", Pair: spotTestPair})
	require.Error(t, err, "AmendOrder must surface pair-format errors")

	_, err = ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{Timeout: -time.Second})
	require.ErrorIs(t, err, errTimeoutInvalid, "CancelAllOrdersAfter must reject negative timeouts")
	_, err = ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{Timeout: time.Millisecond})
	require.ErrorIs(t, err, errTimeoutInvalid, "CancelAllOrdersAfter must reject fractional-second timeouts")

	validBatch := func() *AddOrderBatchRequest {
		return &AddOrderBatchRequest{
			Orders: []AddOrderBatchOrderRequest{
				{OrderType: OrderTypeLimit, OrderSide: OrderSideBuy, Volume: 1},
				{OrderType: OrderTypeLimit, OrderSide: OrderSideSell, Volume: 1},
			},
			Pair: spotTestPair,
		}
	}
	for _, tc := range []struct {
		name     string
		mutate   func(*AddOrderBatchOrderRequest)
		expected error
	}{
		{name: "negative volume", mutate: func(req *AddOrderBatchOrderRequest) { req.Volume = negative }, expected: errVolumeInvalid},
		{name: "non-finite volume", mutate: func(req *AddOrderBatchOrderRequest) { req.Volume = notANumber }, expected: errNumericValueInvalid},
		{name: "negative display volume", mutate: func(req *AddOrderBatchOrderRequest) { req.DisplayVolume = &negative }, expected: errVolumeInvalid},
		{name: "non-finite display volume", mutate: func(req *AddOrderBatchOrderRequest) { req.DisplayVolume = &notANumber }, expected: errNumericValueInvalid},
		{name: "invalid price", mutate: func(req *AddOrderBatchOrderRequest) { req.Price = &OrderPrice{Expression: "invalid"} }, expected: errOrderPriceInvalid},
		{name: "invalid secondary price", mutate: func(req *AddOrderBatchOrderRequest) { req.SecondaryPrice = &OrderPrice{Expression: "invalid"} }, expected: errOrderPriceInvalid},
		{name: "invalid order flag", mutate: func(req *AddOrderBatchOrderRequest) { req.OrderFlags = []OrderFlag{"invalid"} }, expected: errOrderFlagInvalid},
		{name: "invalid start delay", mutate: func(req *AddOrderBatchOrderRequest) { req.StartDelay = time.Millisecond }, expected: errScheduledTimeInvalid},
		{name: "invalid expiry delay", mutate: func(req *AddOrderBatchOrderRequest) { req.ExpireAfter = time.Second }, expected: errScheduledTimeInvalid},
		{name: "invalid close price", mutate: func(req *AddOrderBatchOrderRequest) {
			req.Close = &AddOrderBatchCloseRequest{OrderType: OrderTypeLimit, Price: &OrderPrice{Expression: "invalid"}}
		}, expected: errOrderPriceInvalid},
		{name: "invalid close secondary price", mutate: func(req *AddOrderBatchOrderRequest) {
			req.Close = &AddOrderBatchCloseRequest{OrderType: OrderTypeLimit, SecondaryPrice: &OrderPrice{Expression: "invalid"}}
		}, expected: errOrderPriceInvalid},
	} {
		t.Run("AddOrderBatch "+tc.name, func(t *testing.T) {
			req := validBatch()
			tc.mutate(&req.Orders[0])
			_, err := ex.AddOrderBatch(ctx, req)
			require.ErrorIs(t, err, tc.expected, "AddOrderBatch must reject "+tc.name)
		})
	}
	invalidBatchDeadline := validBatch()
	invalidBatchDeadline.Deadline = invalidDeadline
	_, err = ex.AddOrderBatch(ctx, invalidBatchDeadline)
	require.ErrorIs(t, err, errDeadlineInvalid, "AddOrderBatch must reject an invalid deadline")

	displayVolume := 0.5
	richBatch := validBatch()
	richBatch.Orders[0].DisplayVolume = &displayVolume
	richBatch.Orders[0].SecondaryPrice = &OrderPrice{Value: 90}
	richBatch.Orders[0].Leverage = 2
	richBatch.Orders[0].OrderFlags = []OrderFlag{OrderFlagPostOnly}
	richBatch.Orders[0].StartDelay = time.Second
	richBatch.Orders[0].ExpireAfter = 5 * time.Second
	richBatch.Orders[0].Close = &AddOrderBatchCloseRequest{OrderType: OrderTypeLimit, Price: &OrderPrice{Value: 80}, SecondaryPrice: &OrderPrice{Value: 70}}
	beforeOrders := slices.Clone(richBatch.Orders)
	_, err = ex.AddOrderBatch(ctx, richBatch)
	require.NoError(t, err, "AddOrderBatch must accept typed optional fields")
	assert.Equal(t, beforeOrders, richBatch.Orders, "AddOrderBatch should not mutate caller orders")
	values := requireSpotRequest(t, requests, "/0/private/AddOrderBatch")
	assert.Contains(t, values.Get("orders"), `"displayvol":"0.5"`, "AddOrderBatch should encode display volume")
	assert.Contains(t, values.Get("orders"), `"leverage":"2"`, "AddOrderBatch should encode leverage")
	assert.Contains(t, values.Get("orders"), `"price2":"90"`, "AddOrderBatch should encode secondary price")
	assert.Contains(t, values.Get("orders"), `"starttm":"+1"`, "AddOrderBatch should encode start delay")
	assert.Contains(t, values.Get("orders"), `"expiretm":"+5"`, "AddOrderBatch should encode expiry delay")
	assert.Contains(t, values.Get("orders"), `"oflags":"post"`, "AddOrderBatch should encode order flags")
	assert.Contains(t, values.Get("orders"), `"price2":"70"`, "AddOrderBatch should encode the close secondary price")
	_, err = new(Exchange).AddOrderBatch(ctx, validBatch())
	require.Error(t, err, "AddOrderBatch must surface pair-format errors")

	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: []string{""}})
	require.ErrorIs(t, err, errOrderIDRequired, "CancelOrderBatch must reject an empty transaction identifier")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{ClientOrderIDs: []string{""}})
	require.ErrorIs(t, err, errOrderIDRequired, "CancelOrderBatch must reject an empty client order identifier")
}

func TestSpotFundingTypedRequestValidation(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	ctx := t.Context()
	zero := 0.0
	negative := -1.0
	notANumber := math.NaN()
	preEpoch := time.Unix(-1, 0)
	start := time.Unix(2, 0)
	end := time.Unix(1, 0)
	paginate := true

	_, err := ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin Lightning", Amount: &zero})
	require.ErrorIs(t, err, errAmountInvalid, "GetDepositAddresses must reject an explicit zero amount")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin Lightning", Amount: &negative})
	require.ErrorIs(t, err, errAmountInvalid, "GetDepositAddresses must reject a negative amount")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin Lightning", Amount: &notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "GetDepositAddresses must reject a non-finite amount")

	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet", Amount: notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "GetWithdrawalInformation must reject a non-finite amount")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "WithdrawFunds must reject a non-finite amount")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: 1, MaximumFee: &negative})
	require.ErrorIs(t, err, errMaximumFeeInvalid, "WithdrawFunds must reject a negative maximum fee")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: 1, MaximumFee: &notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "WithdrawFunds must reject a non-finite maximum fee")

	for _, tc := range []struct {
		name     string
		call     func() error
		expected error
	}{
		{name: "deposit cursor conflict", call: func() error {
			_, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Cursor: "CURSOR", Paginate: &paginate})
			return err
		}, expected: errCursorConflict},
		{name: "deposit pre-epoch timestamp", call: func() error {
			_, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Start: preEpoch})
			return err
		}, expected: errTimestampInvalid},
		{name: "deposit reversed range", call: func() error {
			_, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Start: start, End: end})
			return err
		}, expected: errTimeRangeInvalid},
		{name: "withdrawal cursor conflict", call: func() error {
			_, err := ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Cursor: "CURSOR", Paginate: &paginate})
			return err
		}, expected: errCursorConflict},
		{name: "withdrawal pre-epoch timestamp", call: func() error {
			_, err := ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Start: preEpoch})
			return err
		}, expected: errTimestampInvalid},
		{name: "withdrawal reversed range", call: func() error {
			_, err := ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Start: start, End: end})
			return err
		}, expected: errTimeRangeInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			require.ErrorIs(t, err, tc.expected, tc.name+" must be rejected")
		})
	}

	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures, Amount: notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "WalletTransfer must reject a non-finite amount")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: notANumber, From: "PRIMARY", To: "SUB"})
	require.ErrorIs(t, err, errNumericValueInvalid, "AccountTransfer must reject a non-finite amount")
}

func TestSpotEarnTypedRequestValidation(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t)
	notANumber := math.NaN()

	_, err := ex.AllocateEarnFunds(t.Context(), &AllocateEarnFundsRequest{Amount: notANumber, StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errNumericValueInvalid, "AllocateEarnFunds must reject a non-finite amount")
	_, err = ex.DeallocateEarnFunds(t.Context(), &DeallocateEarnFundsRequest{Amount: notANumber, StrategyID: "STRATEGY"})
	require.ErrorIs(t, err, errNumericValueInvalid, "DeallocateEarnFunds must reject a non-finite amount")
}
