package hyperliquid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
	"github.com/thrasher-corp/gocryptotrader/types"
)

const (
	testWrapperPrivateKey = "0x4f3edf983ac636a65a842ce7c78d9aa706d3b113b37ad5dee0c90c0f0da58c16"
	testWrapperAddress    = "0x78a6a3319e34ae14e87b046407675784c002d534"
	testValidatorAddress  = "0x000000000000000000000000000000000000dead"
	testDexUserAddress    = "0x00000000000000000000000000000000000000ab"
	testMultiSigUser      = "0x00000000000000000000000000000000000000cd"
	exchangeOKResponse    = `{"status":"ok"}`
)

var (
	testMetaResponse = MetaResponse{
		Universe: []PerpetualMarket{
			{Name: "BTC", SzDecimals: 3, MaxLeverage: 50},
			{Name: "ETH", SzDecimals: 3, IsDelisted: true, MaxLeverage: 50},
			{Name: "SOL", SzDecimals: 2, MaxLeverage: 25},
		},
	}
	testSpotMetaResponse = SpotMetaResponse{
		Tokens: []SpotToken{
			{Name: "USDC", SZDecimals: 2},
			{Name: "PURR", SZDecimals: 3},
			{Name: "FOO", SZDecimals: 3},
		},
		Universe: []SpotMarket{
			{Tokens: []int{1, 0}, IsCanonical: true},
			{Tokens: []int{2, 0}, IsCanonical: false},
		},
	}
	testAllMids = map[string]string{
		"BTC":       "100",
		"SOL":       "200",
		"PURR/USDC": "1.5",
	}
	testOrderbookSnapshot = OrderbookSnapshot{
		Coin: "BTC",
		Time: types.Time(time.UnixMilli(1700000000000)),
		Levels: [][]BookLevel{
			{
				{Price: types.Number(99), Size: types.Number(1)},
				{Price: types.Number(98), Size: types.Number(0.5)},
			},
			{
				{Price: types.Number(101), Size: types.Number(2)},
				{Price: types.Number(102), Size: types.Number(0.25)},
			},
		},
	}
	testRecentTrades = []RecentTrade{
		{Coin: "BTC", Side: "B", Price: types.Number(100), Size: types.Number(0.1), Time: types.Time(time.UnixMilli(1700000001000)), TID: 1},
		{Coin: "BTC", Side: "A", Price: types.Number(101), Size: types.Number(0.2), Time: types.Time(time.UnixMilli(1700000002000)), TID: 2},
	}
	testCandleSnapshots = []CandleSnapshot{
		{
			OpenTime:  types.Time(time.UnixMilli(1700000040000)),
			CloseTime: types.Time(time.UnixMilli(1700000100000)),
			Symbol:    "BTC",
			Interval:  "1m",
			Open:      types.Number(100),
			Close:     types.Number(101),
			High:      types.Number(102),
			Low:       types.Number(99.5),
			Volume:    types.Number(12.5),
			Trades:    42,
		},
		{
			OpenTime:  types.Time(time.UnixMilli(1700000100000)),
			CloseTime: types.Time(time.UnixMilli(1700000160000)),
			Symbol:    "BTC",
			Interval:  "1m",
			Open:      types.Number(101),
			Close:     types.Number(100.5),
			High:      types.Number(103),
			Low:       types.Number(100),
			Volume:    types.Number(9.75),
			Trades:    37,
		},
	}
	testUserStateResponse = map[string]any{
		"withdrawable": "75",
		"marginSummary": map[string]any{
			"accountValue":    "100",
			"totalMarginUsed": "25",
		},
		"assetPositions": []any{
			map[string]any{
				"position": map[string]any{
					"coin":       "BTC",
					"szi":        "0.1",
					"marginUsed": "5",
					"leverage": map[string]any{
						"type":  "cross",
						"value": "20",
					},
				},
			},
		},
	}
	testSpotUserStateResponse = map[string]any{
		"balances": []any{
			map[string]any{
				"coin":  "USDC",
				"token": 0,
				"total": "125.5",
				"hold":  "10.5",
			},
			map[string]any{
				"coin":  "PURR",
				"token": 1,
				"total": "2.75",
				"hold":  "0",
			},
		},
	}
	testUserFundingHistory = []map[string]any{
		{
			"time": float64(1700000000000),
			"hash": "0xhyperfund",
			"delta": map[string]any{
				"coin":        "APE",
				"fundingRate": "-0.00029319",
				"nSamples":    3,
				"szi":         "40.13333333",
				"type":        "funding",
				"usdc":        "0.145796",
			},
		},
	}
	testFundingHistory = []map[string]any{
		{
			"coin":        "BTC",
			"fundingRate": "0.0001",
			"premium":     "0.00005",
			"time":        float64(1700000000000),
		},
		{
			"coin":        "BTC",
			"fundingRate": "0.0002",
			"premium":     "0.00004",
			"time":        float64(1700000036000),
		},
	}
	testUserNonFundingLedger = []map[string]any{
		{
			"time": float64(1700000010000),
			"hash": "0xwithdrawhash",
			"delta": map[string]any{
				"type":  "withdraw",
				"usdc":  "1500.5",
				"fee":   "1.0",
				"nonce": 1700000009999,
			},
		},
		{
			"time": float64(1700002000000),
			"hash": "0xignored",
			"delta": map[string]any{
				"type": "deposit",
				"usdc": "900.0",
			},
		},
	}
	testOpenOrders = []map[string]any{
		{
			"coin":       "BTC",
			"limitPx":    "123.45",
			"oid":        77738308,
			"side":       "B",
			"sz":         "0.5",
			"timestamp":  1700000000000,
			"reduceOnly": true,
			"orderType":  "Limit",
			"tif":        "Gtc",
			"cloid":      "0xclient-1",
		},
		{
			"coin":      "SOL",
			"limitPx":   "19.75",
			"oid":       77738309,
			"side":      "A",
			"sz":        "12.5",
			"timestamp": 1700000005000,
			"orderType": "Limit",
			"tif":       "Alo",
		},
	}
	testUserFeesResponse = map[string]any{
		"userAddRate":   "0.0002",
		"userCrossRate": "0.00035",
		"feeSchedule": map[string]any{
			"add":   "0.0005",
			"cross": "0.0007",
		},
	}
	testPerpAssetContexts = []PerpetualAssetContext{
		{
			OpenInterest: types.Number(1234.5),
			MarkPrice:    types.Number(101),
			OraclePrice:  types.Number(101),
		},
		{
			OpenInterest: types.Number(5678.9),
			MarkPrice:    types.Number(202),
			OraclePrice:  types.Number(202),
		},
	}
	testHistoricalOrders = []map[string]any{
		{
			"order": map[string]any{
				"coin":           "BTC",
				"side":           "B",
				"limitPx":        "130.5",
				"sz":             "0",
				"origSz":         "0.5",
				"oid":            80000001,
				"timestamp":      1700000100000,
				"triggerPx":      "0",
				"orderType":      "Limit",
				"tif":            "Fok",
				"reduceOnly":     false,
				"cloid":          nil,
				"children":       []any{},
				"isTrigger":      false,
				"isPositionTpsl": false,
			},
			"status":          "Filled",
			"statusTimestamp": 1700000200000,
		},
		{
			"order": map[string]any{
				"coin":           "SOL",
				"side":           "A",
				"limitPx":        "20.1",
				"sz":             "10",
				"origSz":         "10",
				"oid":            80000002,
				"timestamp":      1700000300000,
				"triggerPx":      "0",
				"orderType":      "Limit",
				"tif":            "Ioc",
				"reduceOnly":     false,
				"cloid":          "0xclient-hist",
				"children":       []any{},
				"isTrigger":      false,
				"isPositionTpsl": false,
			},
			"status":          "iocCancelRejected",
			"statusTimestamp": 1700000310000,
		},
	}
)

func decodeExchangeResponse(t *testing.T, payload string) *ExchangeResponse {
	t.Helper()
	var resp ExchangeResponse
	mustUnmarshalJSON(t, []byte(payload), &resp)
	return &resp
}

func setupInfoServer(t *testing.T) (ex *Exchange, cleanup func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer mustCloseBody(t, r.Body)
		var payload map[string]any
		mustDecodeJSON(t, r.Body, &payload)
		typ, _ := payload["type"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch typ {
		case "meta":
			mustEncodeJSON(t, w, testMetaResponse)
		case "spotMeta":
			mustEncodeJSON(t, w, testSpotMetaResponse)
		case "spotClearinghouseState":
			mustEncodeJSON(t, w, testSpotUserStateResponse)
		case "openOrders":
			mustEncodeJSON(t, w, testOpenOrders)
		case "historicalOrders":
			mustEncodeJSON(t, w, testHistoricalOrders)
		case "allMids":
			mustEncodeJSON(t, w, testAllMids)
		case "l2Book":
			mustEncodeJSON(t, w, struct {
				Coin   string        `json:"coin"`
				Time   int64         `json:"time"`
				Levels [][]BookLevel `json:"levels"`
			}{
				Coin:   testOrderbookSnapshot.Coin,
				Time:   testOrderbookSnapshot.Time.Time().UTC().UnixMilli(),
				Levels: testOrderbookSnapshot.Levels,
			})
		case "recentTrades":
			resp := make([]map[string]any, len(testRecentTrades))
			for i, trade := range testRecentTrades {
				resp[i] = map[string]any{
					"coin":  trade.Coin,
					"side":  trade.Side,
					"px":    trade.Price,
					"sz":    trade.Size,
					"time":  trade.Time.Time().UTC().UnixMilli(),
					"tid":   trade.TID,
					"hash":  trade.Hash,
					"users": trade.Users,
				}
			}
			mustEncodeJSON(t, w, resp)
		case "candleSnapshot":
			reqPayload := requireMap(t, payload["req"])
			startVal := requireFloatAny(t, reqPayload["startTime"], "startTime")
			endVal := requireFloatAny(t, reqPayload["endTime"], "endTime")
			start := time.UnixMilli(int64(startVal)).UTC()
			end := time.UnixMilli(int64(endVal)).UTC()
			results := make([]map[string]any, 0, len(testCandleSnapshots))
			for i := range testCandleSnapshots {
				snap := &testCandleSnapshots[i]
				open := snap.OpenTime.Time().UTC()
				if (open.Equal(start) || open.After(start)) && open.Before(end) {
					results = append(results, map[string]any{
						"t": snap.OpenTime.Time().UTC().UnixMilli(),
						"T": snap.CloseTime.Time().UTC().UnixMilli(),
						"s": snap.Symbol,
						"i": snap.Interval,
						"o": snap.Open,
						"c": snap.Close,
						"h": snap.High,
						"l": snap.Low,
						"v": snap.Volume,
						"n": snap.Trades,
					})
				}
			}
			mustEncodeJSON(t, w, results)
		case "userFees":
			mustEncodeJSON(t, w, testUserFeesResponse)
		case "fundingHistory":
			startVal := requireFloatAny(t, payload["startTime"], "startTime")
			end := time.Now().UnixMilli()
			if endVal, ok := payload["endTime"].(float64); ok {
				end = int64(endVal)
			}
			start := int64(startVal)
			entries := make([]map[string]any, 0, len(testFundingHistory))
			for _, entry := range testFundingHistory {
				timeVal, _ := entry["time"].(float64)
				if int64(timeVal) >= start && int64(timeVal) <= end {
					entries = append(entries, entry)
				}
			}
			mustEncodeJSON(t, w, entries)
		case "metaAndAssetCtxs":
			mustEncodeJSON(t, w, []any{testMetaResponse, testPerpAssetContexts})
		case "clearinghouseState":
			mustEncodeJSON(t, w, testUserStateResponse)
		case "userFunding":
			if _, ok := payload["startTime"]; !ok {
				t.Fatal("startTime required")
			}
			mustEncodeJSON(t, w, testUserFundingHistory)
		case "userNonFundingLedgerUpdates":
			if _, ok := payload["startTime"]; !ok {
				t.Fatal("startTime required")
			}
			mustEncodeJSON(t, w, testUserNonFundingLedger)
		default:
			t.Fatalf("unexpected info request type %v", typ)
		}
	}))

	ex = new(Exchange)
	ex.SetDefaults()
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL))
	require.NoError(t, ex.Requester.SetHTTPClient(server.Client()))
	ex.Accounts = accounts.MustNewAccounts(ex)

	cleanup = func() {
		server.Close()
	}
	return ex, cleanup
}

func createSignedExchangeResponder(t *testing.T, responder func(map[string]any) string) *Exchange {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer mustCloseBody(t, r.Body)
		body := mustReadAll(t, r.Body)
		var payload map[string]any
		mustUnmarshalJSON(t, body, &payload)
		switch r.URL.Path {
		case infoPath:
			if payload["type"] != "meta" {
				t.Fatalf("expected meta request type, got %v", payload["type"])
			}
			w.Header().Set("Content-Type", "application/json")
			mustEncodeJSON(t, w, testMetaResponse)
		case "/exchange":
			respBody := exchangeOKResponse
			if responder != nil {
				if custom := responder(payload); custom != "" {
					respBody = custom
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(respBody))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	ex := new(Exchange)
	ex.SetDefaults()
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL))
	require.NoError(t, ex.Requester.SetHTTPClient(server.Client()))
	ex.now = func() time.Time { return time.UnixMilli(1700000000000) }
	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")
	return ex
}

func TestFetchTradablePairs_Perps(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	pairs, err := ex.FetchTradablePairs(context.Background(), asset.PerpetualContract)
	require.NoError(t, err)
	require.Len(t, pairs, 2)
	require.Contains(t, pairs, currencyPair(t, "BTC", "USDC"))
	require.Contains(t, pairs, currencyPair(t, "SOL", "USDC"))
}

func TestFetchTradablePairs_Spot(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	pairs, err := ex.FetchTradablePairs(context.Background(), asset.Spot)
	require.NoError(t, err)
	require.Len(t, pairs, 1)
	require.Contains(t, pairs, currencyPair(t, "PURR", "USDC"))
}

func TestUpdateTicker(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	perpPair := currencyPair(t, "BTC", "USDC")
	price, err := ex.UpdateTicker(context.Background(), perpPair, asset.PerpetualContract)
	require.NoError(t, err)
	require.Equal(t, 100.0, price.Last)

	spotPair := currencyPair(t, "PURR", "USDC")
	spotPrice, err := ex.UpdateTicker(context.Background(), spotPair, asset.Spot)
	require.NoError(t, err)
	require.Equal(t, 1.5, spotPrice.Last)
}

func TestUpdateTickers(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	perpPair := currencyPair(t, "BTC", "USDC")
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, currency.Pairs{perpPair}, false))
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, currency.Pairs{perpPair}, true))
	require.NoError(t, ex.UpdateTickers(context.Background(), asset.PerpetualContract))
	_, err := ticker.GetTicker(ex.Name, perpPair, asset.PerpetualContract)
	require.NoError(t, err)
}

func TestUpdateOrderbook(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	pair := currencyPair(t, "BTC", "USDC")
	book, err := ex.UpdateOrderbook(context.Background(), pair, asset.PerpetualContract)
	require.NoError(t, err)
	require.Len(t, book.Bids, 2)
	require.Len(t, book.Asks, 2)
	require.Equal(t, 99.0, book.Bids[0].Price)
	require.Equal(t, 101.0, book.Asks[0].Price)
}

func TestGetRecentTrades(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	pair := currencyPair(t, "BTC", "USDC")
	trades, err := ex.GetRecentTrades(context.Background(), pair, asset.PerpetualContract)
	require.NoError(t, err)
	require.Len(t, trades, len(testRecentTrades))
	require.Equal(t, order.Buy, trades[0].Side)
	require.Equal(t, order.Sell, trades[1].Side)
}

func TestGetHistoricTrades(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	pair := currencyPair(t, "BTC", "USDC")
	start := time.UnixMilli(1700000001500)
	end := time.UnixMilli(1700000002500)
	trades, err := ex.GetHistoricTrades(context.Background(), pair, asset.PerpetualContract, start, end)
	require.NoError(t, err)
	require.Len(t, trades, 1)
	require.Equal(t, order.Sell, trades[0].Side)
	require.Equal(t, 101.0, trades[0].Price)
}

func TestGetHistoricCandles(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	pair := currencyPair(t, "BTC", "USDC")
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, currency.Pairs{pair}, true))
	start := time.UnixMilli(1700000040000)
	end := time.UnixMilli(1700000160000)

	item, err := ex.GetHistoricCandles(context.Background(), pair, asset.PerpetualContract, kline.OneMin, start, end)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, ex.Name, item.Exchange)
	require.Equal(t, pair, item.Pair)
	require.Equal(t, asset.PerpetualContract, item.Asset)
	require.Len(t, item.Candles, 2)
	require.InDelta(t, 100.0, item.Candles[0].Open, 1e-9)
	require.InDelta(t, 101.0, item.Candles[0].Close, 1e-9)
	require.InDelta(t, 9.75, item.Candles[1].Volume, 1e-9)
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	pair := currencyPair(t, "BTC", "USDC")
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, currency.Pairs{pair}, true))
	start := time.UnixMilli(1700000040000)
	end := time.UnixMilli(1700000280000)

	item, err := ex.GetHistoricCandlesExtended(context.Background(), pair, asset.PerpetualContract, kline.OneMin, start, end)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, kline.OneMin, item.Interval)
	require.GreaterOrEqual(t, len(item.Candles), 2)
}

func TestUpdateAccountBalances(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")
	balances, err := ex.UpdateAccountBalances(context.Background(), asset.PerpetualContract)
	require.NoError(t, err)
	require.Len(t, balances, 1)
	usdcBal, ok := balances[0].Balances[currency.USDC]
	require.True(t, ok)
	require.InDelta(t, 100.0, usdcBal.Total, 1e-9)
	require.InDelta(t, 75.0, usdcBal.Free, 1e-9)
	require.InDelta(t, 25.0, usdcBal.Hold, 1e-9)
	btcBal, ok := balances[0].Balances[currency.NewCode("BTC")]
	require.True(t, ok)
	require.InDelta(t, 0.1, btcBal.Total, 1e-9)
}

func TestUpdateAccountBalances_Spot(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")
	balances, err := ex.UpdateAccountBalances(context.Background(), asset.Spot)
	require.NoError(t, err)
	require.Len(t, balances, 1)
	spotBal := balances[0].Balances[currency.USDC]
	require.InDelta(t, 125.5, spotBal.Total, 1e-9)
	require.InDelta(t, 10.5, spotBal.Hold, 1e-9)
	require.InDelta(t, 115.0, spotBal.Free, 1e-9)
	purrBal := balances[0].Balances[currency.NewCode("PURR")]
	require.InDelta(t, 2.75, purrBal.Total, 1e-9)
	require.InDelta(t, 0.0, purrBal.Hold, 1e-9)
}

func TestGetDepositAddress(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()

	addr, err := ex.GetDepositAddress(context.Background(), currency.USDC, "", "")
	require.NoError(t, err)
	require.Equal(t, hyperliquidBridgeMainnetAddress, addr.Address)
	require.Equal(t, hyperliquidBridgeChain, addr.Chain)

	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), "https://api.hyperliquid-testnet.xyz"))
	testnetAddr, err := ex.GetDepositAddress(context.Background(), currency.USDC, "", "arbitrum")
	require.NoError(t, err)
	require.Equal(t, hyperliquidBridgeTestnetAddress, testnetAddr.Address)

	_, err = ex.GetDepositAddress(context.Background(), currency.BTC, "", "")
	require.Error(t, err)

	_, err = ex.GetDepositAddress(context.Background(), currency.USDC, "", "unsupported")
	require.Error(t, err)
}

func TestGetAvailableTransferChains(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()

	chains, err := ex.GetAvailableTransferChains(context.Background(), currency.USDC)
	require.NoError(t, err)
	require.Equal(t, []string{hyperliquidBridgeChain}, chains)

	_, err = ex.GetAvailableTransferChains(context.Background(), currency.BTC)
	require.Error(t, err)
}

func TestWithdrawCryptocurrencyFunds(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer mustCloseBody(t, r.Body)
		var payload map[string]any
		mustDecodeJSON(t, r.Body, &payload)
		if r.URL.Path == "/exchange" {
			captured = payload
			w.Header().Set("Content-Type", "application/json")
			mustEncodeJSON(t, w, map[string]any{
				"status": "ok",
				"txHash": "0xabc123",
			})
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}))
	t.Cleanup(server.Close)

	ex := new(Exchange)
	ex.SetDefaults()
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL))
	require.NoError(t, ex.Requester.SetHTTPClient(server.Client()))
	ex.Accounts = accounts.MustNewAccounts(ex)
	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")

	req := &withdraw.Request{
		Exchange: ex.Name,
		Currency: currency.USDC,
		Amount:   1.25,
		Type:     withdraw.Crypto,
		Crypto: withdraw.CryptoRequest{
			Address: testWrapperAddress,
			Chain:   "arbitrum",
		},
	}
	resp, err := ex.WithdrawCryptocurrencyFunds(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "0xabc123", resp.ID)
	require.Equal(t, "ok", resp.Status)
	require.NotNil(t, captured)
	action := requireMap(t, captured["action"])
	require.Equal(t, "withdraw3", action["type"])

	req.Crypto.Chain = "unsupported"
	_, err = ex.WithdrawCryptocurrencyFunds(context.Background(), req)
	require.Error(t, err)
}

func TestMoveUSDClassCollateral(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "usdClassTransfer", actionPayload["type"])
		require.Equal(t, "12.5 subaccount:"+strings.ToLower(testVault), actionPayload["amount"])
		require.Equal(t, true, actionPayload["toPerp"])
		return exchangeOKResponse
	})
	ex.SetVaultAddress(testVault)

	require.NoError(t, ex.MoveUSDClassCollateral(context.Background(), 12.5, true))
}

func TestTransferBetweenDexes(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "sendAsset", actionPayload["type"])
		require.Equal(t, strings.ToLower("0xabc"), actionPayload["destination"])
		require.Equal(t, "perp", actionPayload["sourceDex"])
		require.Equal(t, "spot", actionPayload["destinationDex"])
		require.Equal(t, "USDC", actionPayload["token"])
		require.Equal(t, "3.25", actionPayload["amount"])
		require.Equal(t, strings.ToLower(testVault), actionPayload["fromSubAccount"])
		return exchangeOKResponse
	})
	ex.SetVaultAddress(testVault)

	require.NoError(t, ex.TransferBetweenDexes(context.Background(), "perp", "spot", "USDC", "0xABC", 3.25))
}

func TestTransferUSDToSubAccount(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "subAccountTransfer", actionPayload["type"])
		require.Equal(t, strings.ToLower("0xdef"), actionPayload["subAccountUser"])
		require.Equal(t, true, actionPayload["isDeposit"])
		require.Equal(t, float64(500), actionPayload["usd"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.TransferUSDToSubAccount(context.Background(), "0xDEF", 500, true))
}

func TestTransferSpotToSubAccount(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "subAccountSpotTransfer", actionPayload["type"])
		require.Equal(t, strings.ToLower("0xdef"), actionPayload["subAccountUser"])
		require.Equal(t, false, actionPayload["isDeposit"])
		require.Equal(t, "PURR", actionPayload["token"])
		require.Equal(t, "4.2", actionPayload["amount"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.TransferSpotToSubAccount(context.Background(), "0xDEF", "PURR", 4.2, false))
}

func TestTransferUSDToVault(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "vaultTransfer", actionPayload["type"])
		require.Equal(t, strings.ToLower("0xfeed"), actionPayload["vaultAddress"])
		require.Equal(t, false, actionPayload["isDeposit"])
		require.Equal(t, float64(750), actionPayload["usd"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.TransferUSDToVault(context.Background(), "0xFEED", 750, false))
}

func TestSendUSDC(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "usdSend", actionPayload["type"])
		require.Equal(t, strings.ToLower("0xbeef"), actionPayload["destination"])
		require.Equal(t, "8.75", actionPayload["amount"])
		require.Equal(t, "0x18bcfe56800", actionPayload["time"])
		return exchangeOKResponse
	})
	ex.SetVaultAddress(testVault)

	require.NoError(t, ex.SendUSDC(context.Background(), "0xBEEF", 8.75))
}

func TestSendSpotToken(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotSend", actionPayload["type"])
		require.Equal(t, strings.ToLower("0xbeef"), actionPayload["destination"])
		require.Equal(t, "PURR", actionPayload["token"])
		require.Equal(t, "3.14", actionPayload["amount"])
		require.Equal(t, "0x18bcfe56800", actionPayload["time"])
		return exchangeOKResponse
	})
	ex.SetVaultAddress(testVault)

	require.NoError(t, ex.SendSpotToken(context.Background(), "0xBEEF", "PURR", 3.14))
}

func TestDelegateValidatorTokens(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "tokenDelegate", actionPayload["type"])
		require.Equal(t, strings.ToLower(testValidatorAddress), actionPayload["validator"])
		require.Equal(t, "0x7b", actionPayload["wei"])
		require.Equal(t, true, actionPayload["isUndelegate"])
		require.Equal(t, "0x18bcfe56800", actionPayload["nonce"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.DelegateValidatorTokens(context.Background(), testValidatorAddress, 123, true))
}

func TestWithdrawBridgeUSDC(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "withdraw3", actionPayload["type"])
		require.Equal(t, strings.ToLower("0xface"), actionPayload["destination"])
		require.Equal(t, "15", actionPayload["amount"])
		require.Equal(t, "0x18bcfe56800", actionPayload["time"])
		return exchangeOKResponse
	})
	ex.SetVaultAddress(testVault)

	require.NoError(t, ex.WithdrawBridgeUSDC(context.Background(), "0xFACE", 15))
}

func TestApproveAgentWithName(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "approveAgent", actionPayload["type"])
		require.Equal(t, "helper", actionPayload["agentName"])
		return exchangeOKResponse
	})

	agentKey, err := ex.ApproveAgentWithName(context.Background(), "helper")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(agentKey, "0x"))
	require.Len(t, agentKey, 66)
}

func TestSetBuilderMaxFeeRate(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "approveBuilderFee", actionPayload["type"])
		require.Equal(t, strings.ToLower(testWrapperAddress), actionPayload["builder"])
		require.Equal(t, "0.001", actionPayload["maxFeeRate"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.SetBuilderMaxFeeRate(context.Background(), testWrapperAddress, "0.001"))
}

func TestConvertAccountToMultiSig(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "convertToMultiSigUser", actionPayload["type"])
		signersJSON := requireString(t, actionPayload["signers"])
		var decoded struct {
			AuthorizedUsers []string `json:"authorizedUsers"`
			Threshold       int      `json:"threshold"`
		}
		require.NoError(t, json.Unmarshal([]byte(signersJSON), &decoded))
		require.Equal(t, []string{"0x1", "0x2"}, decoded.AuthorizedUsers)
		require.Equal(t, 2, decoded.Threshold)
		return exchangeOKResponse
	})

	require.NoError(t, ex.ConvertAccountToMultiSig(context.Background(), []string{"0x1", "0x2"}, 2))
}

func TestSubmitMultiSigAction(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "multiSig", actionPayload["type"])
		payloadBody := requireMap(t, actionPayload["payload"])
		require.Equal(t, strings.ToLower(testWrapperAddress), payloadBody["outerSigner"])
		require.Equal(t, strings.ToLower(testMultiSigUser), payloadBody["multiSigUser"])
		return exchangeOKResponse
	})

	req := &MultiSigRequest{
		MultiSigUser: testMultiSigUser,
		Action:       map[string]any{"type": "noop"},
		Signatures:   []MultiSigSignature{{R: "0x1", S: "0x2", V: 27}},
		Nonce:        42,
	}

	require.NoError(t, ex.SubmitMultiSigAction(context.Background(), req))
}

func TestToggleBigBlocks(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "evmUserModify", actionPayload["type"])
		require.Equal(t, true, actionPayload["usingBigBlocks"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.ToggleBigBlocks(context.Background(), true))
}

func TestEnableAgentDexAbstraction(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "agentEnableDexAbstraction", actionPayload["type"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.EnableAgentDexAbstraction(context.Background()))
}

func TestSetUserDexAbstractionState(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		message := requireMap(t, payload["action"])
		require.Equal(t, "userDexAbstraction", message["type"])
		require.Equal(t, strings.ToLower(testDexUserAddress), message["user"])
		require.Equal(t, true, message["enabled"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.SetUserDexAbstractionState(context.Background(), testDexUserAddress, true))
}

func TestSubmitNoopAction(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "noop", actionPayload["type"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.SubmitNoopAction(context.Background(), 99))
}

func TestAssignReferrer(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "setReferrer", actionPayload["type"])
		require.Equal(t, "HELLO", actionPayload["code"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.AssignReferrer(context.Background(), "HELLO"))
}

func TestAddSubAccount(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "createSubAccount", actionPayload["type"])
		require.Equal(t, "Funding", actionPayload["name"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.AddSubAccount(context.Background(), "Funding"))
}

func TestScheduleMassCancel(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "scheduleCancel", actionPayload["type"])
		require.Equal(t, float64(1700), actionPayload["time"])
		return exchangeOKResponse
	})

	ts := uint64(1700)
	require.NoError(t, ex.ScheduleMassCancel(context.Background(), &ts))

	ex = createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "scheduleCancel", actionPayload["type"])
		_, ok := actionPayload["time"]
		require.False(t, ok)
		return exchangeOKResponse
	})

	require.NoError(t, ex.ScheduleMassCancel(context.Background(), nil))
}

func TestUpdateLeverageSetting(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "updateLeverage", actionPayload["type"])
		require.Equal(t, float64(20), actionPayload["leverage"])
		require.Equal(t, true, actionPayload["isCross"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.UpdateLeverageSetting(context.Background(), "BTC", 20, true))
}

func TestAdjustIsolatedMargin(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "updateIsolatedMargin", actionPayload["type"])
		require.Equal(t, true, actionPayload["isBuy"])
		require.Equal(t, float64(500000), actionPayload["ntli"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.AdjustIsolatedMargin(context.Background(), "BTC", 0.5, true))
}

func TestRegisterSpotToken(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		register := requireMap(t, actionPayload["registerToken2"])
		spec := requireMap(t, register["spec"])
		require.Equal(t, "TKN", spec["name"])
		require.Equal(t, float64(8), spec["szDecimals"])
		require.Equal(t, float64(18), spec["weiDecimals"])
		require.Equal(t, float64(4321), register["maxGas"])
		require.Equal(t, "Token Name", register["fullName"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.RegisterSpotToken(context.Background(), &SpotDeployRegisterTokenRequest{
		TokenName:    "TKN",
		SizeDecimals: 8,
		WeiDecimals:  18,
		MaxGas:       4321,
		FullName:     "Token Name",
	}))
}

func TestConfigureSpotGenesis(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		genesis := requireMap(t, actionPayload["userGenesis"])
		require.Equal(t, float64(7), genesis["token"])
		users := requireSlice(t, genesis["userAndWei"])
		require.Equal(t, []any{strings.ToLower("0xA"), "25"}, requireSlice(t, users[0]))
		existing := requireSlice(t, genesis["existingTokenAndWei"])
		require.Equal(t, []any{float64(4), "30"}, requireSlice(t, existing[0]))
		return exchangeOKResponse
	})

	require.NoError(t, ex.ConfigureSpotGenesis(context.Background(), &SpotDeployUserGenesisRequest{
		Token:               7,
		UserAndWei:          []SpotDeployUserGenesisEntry{{User: "0xA", Wei: "25"}},
		ExistingTokenAndWei: []SpotDeployExistingTokenWeiEntry{{Token: 4, Wei: "30"}},
	}))
}

func TestEnableSpotFreezePrivilege(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		enable := requireMap(t, actionPayload["enableFreezePrivilege"])
		require.Equal(t, float64(5), enable["token"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.EnableSpotFreezePrivilege(context.Background(), 5))
}

func TestDisableSpotFreezePrivilege(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		revoke := requireMap(t, actionPayload["revokeFreezePrivilege"])
		require.Equal(t, float64(6), revoke["token"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.DisableSpotFreezePrivilege(context.Background(), 6))
}

func TestFreezeSpotUser(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		freeze := requireMap(t, actionPayload["freezeUser"])
		require.Equal(t, float64(8), freeze["token"])
		require.Equal(t, strings.ToLower("0x123"), freeze["user"])
		require.Equal(t, true, freeze["freeze"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.FreezeSpotUser(context.Background(), &SpotDeployFreezeUserRequest{
		Token:  8,
		User:   "0x123",
		Freeze: true,
	}))
}

func TestEnableSpotQuoteToken(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		enable := requireMap(t, actionPayload["enableQuoteToken"])
		require.Equal(t, float64(9), enable["token"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.EnableSpotQuoteToken(context.Background(), 9))
}

func TestFinaliseSpotGenesis(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		genesis := requireMap(t, actionPayload["genesis"])
		require.Equal(t, float64(10), genesis["token"])
		require.Equal(t, "1000", genesis["maxSupply"])
		require.Equal(t, true, genesis["noHyperliquidity"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.FinaliseSpotGenesis(context.Background(), &SpotDeployGenesisRequest{
		Token:            10,
		MaxSupply:        "1000",
		NoHyperliquidity: true,
	}))
}

func TestRegisterSpotMarket(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		register := requireMap(t, actionPayload["registerSpot"])
		require.Equal(t, float64(11), register["baseToken"])
		require.Equal(t, float64(0), register["quoteToken"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.RegisterSpotMarket(context.Background(), &SpotDeployRegisterSpotRequest{
		BaseToken:  11,
		QuoteToken: 0,
	}))
}

func TestConfigureSpotHyperliquidity(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		config := requireMap(t, actionPayload["registerHyperliquidity"])
		require.Equal(t, float64(12), config["spot"])
		require.Equal(t, "1.05", config["startPx"])
		require.Equal(t, "0.25", config["orderSz"])
		require.Equal(t, float64(6), config["nOrders"])
		require.Equal(t, float64(2), config["nSeededLevels"])
		return exchangeOKResponse
	})

	nSeeded := 2
	require.NoError(t, ex.ConfigureSpotHyperliquidity(context.Background(), &SpotDeployRegisterHyperliquidityRequest{
		Spot:         12,
		StartPrice:   1.05,
		OrderSize:    0.25,
		Orders:       6,
		SeededLevels: &nSeeded,
	}))
}

func TestSetSpotDeployerTradingFeeShare(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "spotDeploy", actionPayload["type"])
		setShare := requireMap(t, actionPayload["setDeployerTradingFeeShare"])
		require.Equal(t, float64(13), setShare["token"])
		require.Equal(t, "0.15", setShare["share"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.SetSpotDeployerTradingFeeShare(context.Background(), &SpotDeploySetDeployerTradingFeeShareRequest{
		Token: 13,
		Share: "0.15",
	}))
}

func TestRegisterPerpAsset(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "perpDeploy", actionPayload["type"])
		register := requireMap(t, actionPayload["registerAsset"])
		require.Equal(t, "dex1", register["dex"])
		require.Equal(t, float64(3210), register["maxGas"])
		assetRequest := requireMap(t, register["assetRequest"])
		require.Equal(t, "BTC", assetRequest["coin"])
		require.Equal(t, float64(3), assetRequest["szDecimals"])
		require.Equal(t, "50000", assetRequest["oraclePx"])
		require.Equal(t, float64(1), assetRequest["marginTableId"])
		require.Equal(t, true, assetRequest["onlyIsolated"])
		schema := requireMap(t, register["schema"])
		require.Equal(t, "Bitcoin Perp", schema["fullName"])
		require.Equal(t, "USDC", schema["collateralToken"])
		require.Equal(t, strings.ToLower("0xbeef"), schema["oracleUpdater"])
		return exchangeOKResponse
	})

	updater := "0xBEEF"
	require.NoError(t, ex.RegisterPerpAsset(context.Background(), &PerpDeployRegisterAssetRequest{
		Dex:           "dex1",
		MaxGas:        ptrToInt(3210),
		Coin:          "BTC",
		SizeDecimals:  3,
		OraclePrice:   "50000",
		MarginTableID: 1,
		OnlyIsolated:  true,
		Schema: &PerpDeploySchema{
			FullName:        "Bitcoin Perp",
			CollateralToken: "USDC",
			OracleUpdater:   &updater,
		},
	}))
}

func TestUpdatePerpOracle(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "perpDeploy", actionPayload["type"])
		setOracle := requireMap(t, actionPayload["setOracle"])
		require.Equal(t, "dex2", setOracle["dex"])
		oracle := requireSlice(t, setOracle["oraclePxs"])
		require.Equal(t, []any{"BTC", "50000"}, requireSlice(t, oracle[0]))
		mark := requireSlice(t, setOracle["markPxs"])
		require.Equal(t, []any{"BTC", "50010"}, requireSlice(t, requireSlice(t, mark[0])[0]))
		external := requireSlice(t, setOracle["externalPerpPxs"])
		require.Equal(t, []any{"BTC", "49990"}, requireSlice(t, external[0]))
		return exchangeOKResponse
	})

	require.NoError(t, ex.UpdatePerpOracle(context.Background(), &PerpDeploySetOracleRequest{
		Dex: "dex2",
		OraclePrices: map[string]string{
			"BTC": "50000",
		},
		MarkPrices: []map[string]string{
			{"BTC": "50010"},
		},
		ExternalPerpPrices: map[string]string{
			"BTC": "49990",
		},
	}))
}

func TestRegisterValidator(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "CValidatorAction", actionPayload["type"])
		register := requireMap(t, actionPayload["register"])
		profile := requireMap(t, register["profile"])
		nodeIP := requireMap(t, profile["node_ip"])
		require.Equal(t, "10.0.0.1", nodeIP["Ip"])
		require.Equal(t, "Validator", profile["name"])
		require.Equal(t, "Desc", profile["description"])
		require.Equal(t, true, profile["delegations_disabled"])
		require.Equal(t, float64(250), profile["commission_bps"])
		require.Equal(t, strings.ToLower("0xSigner"), profile["signer"])
		require.Equal(t, true, register["unjailed"])
		require.Equal(t, float64(1000), register["initial_wei"])
		return exchangeOKResponse
	})

	require.NoError(t, ex.RegisterValidator(context.Background(), &CValidatorRegisterRequest{
		NodeIP:              "10.0.0.1",
		Name:                "Validator",
		Description:         "Desc",
		DelegationsDisabled: true,
		CommissionBPS:       250,
		Signer:              "0xSigner",
		Unjailed:            true,
		InitialWei:          1000,
	}))
}

func TestUpdateValidatorProfile(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "CValidatorAction", actionPayload["type"])
		change := requireMap(t, actionPayload["changeProfile"])
		nodeIP := requireMap(t, change["node_ip"])
		require.Equal(t, "10.0.0.2", nodeIP["Ip"])
		require.Equal(t, "New Name", change["name"])
		require.Equal(t, "New Desc", change["description"])
		require.Equal(t, true, change["disable_delegations"])
		require.Equal(t, float64(150), change["commission_bps"])
		require.Equal(t, strings.ToLower("0xnewsigner"), change["signer"])
		require.Equal(t, true, change["unjailed"])
		return exchangeOKResponse
	})

	name := "New Name"
	desc := "New Desc"
	nodeIP := "10.0.0.2"
	signer := "0xNewSigner"
	disable := true
	commission := 150
	require.NoError(t, ex.UpdateValidatorProfile(context.Background(), &CValidatorChangeProfileRequest{
		NodeIP:             &nodeIP,
		Name:               &name,
		Description:        &desc,
		DisableDelegations: &disable,
		CommissionBPS:      &commission,
		Signer:             &signer,
		Unjailed:           true,
	}))
}

func TestUnregisterValidator(t *testing.T) {
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		actionPayload := requireMap(t, payload["action"])
		require.Equal(t, "CValidatorAction", actionPayload["type"])
		_, ok := actionPayload["unregister"]
		require.True(t, ok, "unregister flag must be present")
		require.Nil(t, actionPayload["unregister"], "unregister payload must be null")
		return exchangeOKResponse
	})

	require.NoError(t, ex.UnregisterValidator(context.Background()))
}

func TestSubmitOrder(t *testing.T) {
	var captured map[string]any
	exchangeResponse := `{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":77738308}}]}}}`
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		captured = payload
		return exchangeResponse
	})
	ex.Accounts = accounts.MustNewAccounts(ex)

	submit := &order.Submit{
		Exchange:    ex.Name,
		Pair:        currencyPair(t, "BTC", "USDC"),
		AssetType:   asset.PerpetualContract,
		Type:        order.Limit,
		Side:        order.Buy,
		TimeInForce: order.GoodTillCancel,
		Price:       123.45,
		Amount:      0.5,
	}

	resp, err := ex.SubmitOrder(context.Background(), submit)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "77738308", resp.OrderID)
	require.Equal(t, order.Active, resp.Status)
	require.NotNil(t, captured)
	action := requireMap(t, captured["action"])
	require.Equal(t, "order", action["type"])
	orders := requireSlice(t, action["orders"])
	require.Len(t, orders, 1)
	orderWire := requireMap(t, orders[0])
	require.Equal(t, "123.45", orderWire["p"])
	require.Equal(t, "0.5", orderWire["s"])
}

func TestSubmitOrderUnsupportedType(t *testing.T) {
	ex := createSignedExchangeResponder(t, nil)
	ex.Accounts = accounts.MustNewAccounts(ex)

	submit := &order.Submit{
		Exchange:  ex.Name,
		Pair:      currencyPair(t, "BTC", "USDC"),
		AssetType: asset.PerpetualContract,
		Type:      order.Market,
		Side:      order.Buy,
		Amount:    1,
	}
	_, err := ex.SubmitOrder(context.Background(), submit)
	require.Error(t, err)
}

func TestModifyOrder(t *testing.T) {
	var captured map[string]any
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		captured = payload
		return `{"status":"ok","response":{"type":"batchModify","data":{"statuses":["success"]}}}`
	})

	modify := &order.Modify{
		Exchange:    ex.Name,
		OrderID:     "77738308",
		Pair:        currencyPair(t, "BTC", "USDC"),
		AssetType:   asset.PerpetualContract,
		Type:        order.Limit,
		Side:        order.Buy,
		TimeInForce: order.GoodTillCancel,
		Price:       125.5,
		Amount:      0.75,
	}

	resp, err := ex.ModifyOrder(context.Background(), modify)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, order.Open, resp.Status)
	action := requireMap(t, captured["action"])
	require.Equal(t, "batchModify", action["type"])
}

func TestCancelOrder(t *testing.T) {
	var captured map[string]any
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		captured = payload
		return `{"status":"ok","response":{"type":"cancel","data":{"statuses":["success"]}}}`
	})

	cancelReq := &order.Cancel{
		Exchange:  ex.Name,
		OrderID:   "77738308",
		Pair:      currencyPair(t, "BTC", "USDC"),
		AssetType: asset.PerpetualContract,
	}
	require.NoError(t, ex.CancelOrder(context.Background(), cancelReq))
	action := requireMap(t, captured["action"])
	require.Equal(t, "cancel", action["type"])
}

func TestCancelBatchOrders(t *testing.T) {
	var count int
	ex := createSignedExchangeResponder(t, func(_ map[string]any) string {
		count++
		return `{"status":"ok","response":{"type":"cancel","data":{"statuses":["success"]}}}`
	})

	orders := []order.Cancel{
		{
			Exchange:  ex.Name,
			OrderID:   "1001",
			Pair:      currencyPair(t, "BTC", "USDC"),
			AssetType: asset.PerpetualContract,
		},
		{
			Exchange:  ex.Name,
			OrderID:   "1002",
			Pair:      currencyPair(t, "BTC", "USDC"),
			AssetType: asset.PerpetualContract,
		},
	}

	resp, err := ex.CancelBatchOrders(context.Background(), orders)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Len(t, resp.Status, 2)
	require.Equal(t, "success", resp.Status["1001"])
	require.Equal(t, "success", resp.Status["1002"])
}

func TestCancelAllOrders(t *testing.T) {
	t.Parallel()

	var (
		mu      sync.Mutex
		actions []map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer mustCloseBody(t, r.Body)
		switch r.URL.Path {
		case infoPath:
			var payload map[string]any
			mustDecodeJSON(t, r.Body, &payload)
			switch payload["type"] {
			case "meta":
				mustEncodeJSON(t, w, testMetaResponse)
			case "openOrders":
				mustEncodeJSON(t, w, testOpenOrders)
			default:
				t.Fatalf("unexpected info request type %v", payload["type"])
			}
		case "/exchange":
			var payload map[string]any
			mustDecodeJSON(t, r.Body, &payload)
			mu.Lock()
			actions = append(actions, payload)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"cancel","data":{"statuses":["success"]}}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	ex := new(Exchange)
	ex.SetDefaults()
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL))
	require.NoError(t, ex.Requester.SetHTTPClient(server.Client()))
	ex.Accounts = accounts.MustNewAccounts(ex)
	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")

	resp, err := ex.CancelAllOrders(context.Background(), &order.Cancel{
		AssetType: asset.PerpetualContract,
	})
	require.NoError(t, err)
	require.Len(t, resp.Status, len(testOpenOrders))
	require.Equal(t, "success", resp.Status["77738308"])
	require.Equal(t, "success", resp.Status["77738309"])

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, actions, len(testOpenOrders))
	for _, actionPayload := range actions {
		action := requireMap(t, actionPayload["action"])
		require.Equal(t, "cancel", action["type"])
	}
}

func TestGetAccountFundingHistory(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")
	ex.now = func() time.Time { return time.UnixMilli(1700000005000) }

	history, err := ex.GetAccountFundingHistory(context.Background())
	require.NoError(t, err)
	require.Len(t, history, len(testUserFundingHistory))
	entry := history[0]
	require.Equal(t, ex.Name, entry.ExchangeName)
	require.Equal(t, "funding", entry.Status)
	require.Equal(t, "0xhyperfund", entry.TransferID)
	require.Equal(t, "APE", entry.Currency)
	require.InDelta(t, 0.145796, entry.Amount, 1e-9)
	require.Contains(t, entry.Description, "APE funding rate")
	require.Equal(t, time.UnixMilli(1700000000000).UTC(), entry.Timestamp)
	require.Equal(t, "funding", entry.TransferType)
}

func TestGetWithdrawalsHistory(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")
	ex.now = func() time.Time { return time.UnixMilli(1700001000000) }

	withdrawals, err := ex.GetWithdrawalsHistory(context.Background(), currency.USDC, asset.PerpetualContract)
	require.NoError(t, err)
	require.Len(t, withdrawals, 1)
	entry := withdrawals[0]
	require.Equal(t, "withdraw", entry.Status)
	require.Equal(t, "0xwithdrawhash", entry.TransferID)
	require.InDelta(t, 1500.5, entry.Amount, 1e-9)
	require.InDelta(t, 1.0, entry.Fee, 1e-9)
	require.Equal(t, hyperliquidBridgeChain, entry.CryptoChain)
	require.Contains(t, entry.Description, "nonce")

	historySpot, err := ex.GetWithdrawalsHistory(context.Background(), currency.USDC, asset.Spot)
	require.NoError(t, err)
	require.Len(t, historySpot, 1)

	_, err = ex.GetWithdrawalsHistory(context.Background(), currency.BTC, asset.Spot)
	require.Error(t, err)
}

func TestGetFeeByType(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")

	fee, err := ex.GetFeeByType(context.Background(), &exchange.FeeBuilder{
		FeeType:       exchange.CryptocurrencyTradeFee,
		IsMaker:       true,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.04, fee, 1e-9)

	takerFee, err := ex.GetFeeByType(context.Background(), &exchange.FeeBuilder{
		FeeType:       exchange.OfflineTradeFee,
		IsMaker:       false,
		PurchasePrice: 100,
		Amount:        2,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.07, takerFee, 1e-9)
}

func TestGetFuturesContractDetails(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	contracts, err := ex.GetFuturesContractDetails(context.Background(), asset.PerpetualContract)
	require.NoError(t, err)
	require.Len(t, contracts, 2)
	require.Equal(t, currency.NewBTCUSDC(), contracts[0].Name)
}

func TestGetLatestFundingRates(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()
	ex.now = func() time.Time { return time.UnixMilli(1700000200000) }

	resp, err := ex.GetLatestFundingRates(context.Background(), &fundingrate.LatestRateRequest{
		Asset: asset.PerpetualContract,
		Pair:  currency.NewBTCUSDC(),
	})
	require.NoError(t, err)
	require.Len(t, resp, 1)
	require.InDelta(t, 0.0002, resp[0].LatestRate.Rate.InexactFloat64(), 1e-9)
}

func TestGetHistoricalFundingRates(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()
	ex.now = func() time.Time { return time.UnixMilli(1700000200000) }

	start := time.UnixMilli(1700000000000)
	end := time.UnixMilli(1700000060000)
	resp, err := ex.GetHistoricalFundingRates(context.Background(), &fundingrate.HistoricalRatesRequest{
		Asset:                asset.PerpetualContract,
		Pair:                 currency.NewBTCUSDC(),
		StartDate:            start,
		EndDate:              end,
		IncludePredictedRate: true,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.FundingRates)
}

func TestGetOpenInterest(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ints, err := ex.GetOpenInterest(context.Background())
	require.NoError(t, err)
	require.Len(t, ints, 2)
	require.InDelta(t, 1234.5, ints[0].OpenInterest, 1e-9)
}

func TestGetCurrencyTradeURL(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	perp := currencyPair(t, "BTC", "USDC")
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.PerpetualContract, currency.Pairs{perp}, true))
	url, err := ex.GetCurrencyTradeURL(context.Background(), asset.PerpetualContract, perp)
	require.NoError(t, err)
	require.Equal(t, "https://app.hyperliquid.xyz/trade/BTC", url)

	spot := currencyPair(t, "PURR", "USDC")
	require.NoError(t, ex.CurrencyPairs.StorePairs(asset.Spot, currency.Pairs{spot}, true))
	spotURL, err := ex.GetCurrencyTradeURL(context.Background(), asset.Spot, spot)
	require.NoError(t, err)
	require.Equal(t, "https://app.hyperliquid.xyz/spot/PURR-USDC", spotURL)
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	require.NoError(t, ex.UpdateOrderExecutionLimits(context.Background(), asset.PerpetualContract))

	limit, err := limits.GetOrderExecutionLimits(key.NewExchangeAssetPair(ex.Name, asset.PerpetualContract, currencyPair(t, "BTC", "USDC")))
	require.NoError(t, err)
	require.InDelta(t, 0.001, limit.AmountStepIncrementSize, 1e-9)
}

func TestGetLeverage(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")
	lev, err := ex.GetLeverage(context.Background(), asset.PerpetualContract, currencyPair(t, "BTC", "USDC"), margin.Multi, order.Buy)
	require.NoError(t, err)
	require.InDelta(t, 20, lev, 1e-9)
}

func TestSetLeverage(t *testing.T) {
	var captured map[string]any
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
		captured = payload
		return `{"status":"ok","response":{"type":"updateLeverage","data":{"statuses":["success"]}}}`
	})

	err := ex.SetLeverage(context.Background(), asset.PerpetualContract, currencyPair(t, "BTC", "USDC"), margin.Multi, 15, order.Buy)
	require.NoError(t, err)
	action := requireMap(t, captured["action"])
	require.Equal(t, "updateLeverage", action["type"])
	require.EqualValues(t, 15, action["leverage"])
}

func TestGetActiveOrders(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")

	orders, err := ex.GetActiveOrders(context.Background(), &order.MultiOrderRequest{
		AssetType: asset.PerpetualContract,
		Type:      order.AnyType,
		Side:      order.AnySide,
	})
	require.NoError(t, err)
	require.Len(t, orders, len(testOpenOrders))
	first := orders[0]
	require.Equal(t, "77738308", first.OrderID)
	require.Equal(t, order.Active, first.Status)
	require.Equal(t, order.Limit, first.Type)
	require.Equal(t, "BID", first.Side.String())
	require.True(t, first.ReduceOnly)
	require.InDelta(t, 123.45, first.Price, 1e-9)
	require.Equal(t, "0xclient-1", first.ClientOrderID)
}

func TestGetOrderHistory(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")

	history, err := ex.GetOrderHistory(context.Background(), &order.MultiOrderRequest{
		AssetType: asset.PerpetualContract,
		Type:      order.AnyType,
		Side:      order.AnySide,
	})
	require.NoError(t, err)
	require.Len(t, history, len(testHistoricalOrders))
	require.Equal(t, order.Filled, history[0].Status)
	require.InDelta(t, 0.5, history[0].Amount, 1e-9)
	require.InDelta(t, 0.5, history[0].ExecutedAmount, 1e-9)
	require.Equal(t, order.Rejected, history[1].Status)
}

func TestGetOrderInfo(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()

	ex.SetCredentials(testWrapperAddress, testWrapperPrivateKey, "", "", "", "")

	detail, err := ex.GetOrderInfo(context.Background(), "77738308", currency.EMPTYPAIR, asset.PerpetualContract)
	require.NoError(t, err)
	require.Equal(t, "77738308", detail.OrderID)
	require.Equal(t, order.Active, detail.Status)

	detail, err = ex.GetOrderInfo(context.Background(), "80000001", currency.EMPTYPAIR, asset.PerpetualContract)
	require.NoError(t, err)
	require.Equal(t, order.Filled, detail.Status)
}

func TestParseActionStatusesStatusNotOK(t *testing.T) {
	t.Parallel()

	resp := decodeExchangeResponse(t, `{"status":"failure","response":{"type":"order","data":{"statuses":[]}}}`)
	_, status, submissionErr, err := parseActionStatuses(resp)
	require.NoError(t, err)
	require.Equal(t, order.UnknownStatus, status)
	require.ErrorIs(t, submissionErr, errActionStatusNotOK)
	require.ErrorContains(t, submissionErr, "failure")
}

func TestParseActionStatusesSubmissionStatusFailure(t *testing.T) {
	t.Parallel()

	resp := decodeExchangeResponse(t, `{"status":"ok","response":{"type":"order","data":{"statuses":["Rejected"]}}}`)
	_, status, submissionErr, err := parseActionStatuses(resp)
	require.NoError(t, err)
	require.Equal(t, order.UnknownStatus, status)
	require.ErrorIs(t, submissionErr, errActionSubmissionStatusFailure)
	require.ErrorContains(t, submissionErr, "Rejected")
}

func TestParseActionStatusesSubmissionError(t *testing.T) {
	t.Parallel()

	resp := decodeExchangeResponse(t, `{"status":"ok","response":{"type":"order","data":{"statuses":[{"error":"insufficient margin"}]}}}`)
	_, status, submissionErr, err := parseActionStatuses(resp)
	require.NoError(t, err)
	require.Equal(t, order.UnknownStatus, status)
	require.ErrorIs(t, submissionErr, errActionSubmissionError)
	require.ErrorContains(t, submissionErr, "insufficient margin")
}

func currencyPair(t *testing.T, base, quote string) currency.Pair { //nolint:unparam // quote retained for clarity when mirroring API inputs
	t.Helper()
	b := currency.NewCode(base)
	q := currency.NewCode(quote)
	return currency.NewPair(b, q)
}
