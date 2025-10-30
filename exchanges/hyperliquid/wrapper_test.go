package hyperliquid

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
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
	testWrapperAddress    = "0x90f8bf6a479f320ead074411a4b0e7944ea8c9c1"
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
		Time: 1700000000000,
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
		{Coin: "BTC", Side: "B", Price: types.Number(100), Size: types.Number(0.1), Time: 1700000001000, TID: 1},
		{Coin: "BTC", Side: "A", Price: types.Number(101), Size: types.Number(0.2), Time: 1700000002000, TID: 2},
	}
	testCandleSnapshots = []CandleSnapshot{
		{
			OpenTime:  1700000040000,
			CloseTime: 1700000100000,
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
			OpenTime:  1700000100000,
			CloseTime: 1700000160000,
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

func setupInfoServer(t *testing.T) (*Exchange, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		typ, _ := payload["type"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch typ {
		case "meta":
			require.NoError(t, json.NewEncoder(w).Encode(testMetaResponse))
		case "spotMeta":
			require.NoError(t, json.NewEncoder(w).Encode(testSpotMetaResponse))
		case "spotClearinghouseState":
			require.NoError(t, json.NewEncoder(w).Encode(testSpotUserStateResponse))
		case "openOrders":
			require.NoError(t, json.NewEncoder(w).Encode(testOpenOrders))
		case "historicalOrders":
			require.NoError(t, json.NewEncoder(w).Encode(testHistoricalOrders))
		case "allMids":
			require.NoError(t, json.NewEncoder(w).Encode(testAllMids))
		case "l2Book":
			require.NoError(t, json.NewEncoder(w).Encode(testOrderbookSnapshot))
		case "recentTrades":
			require.NoError(t, json.NewEncoder(w).Encode(testRecentTrades))
		case "candleSnapshot":
			reqPayload, ok := payload["req"].(map[string]any)
			require.True(t, ok)
			startVal, ok := reqPayload["startTime"].(float64)
			require.True(t, ok)
			endVal, ok := reqPayload["endTime"].(float64)
			require.True(t, ok)
			start := int64(startVal)
			end := int64(endVal)
			results := make([]CandleSnapshot, 0, len(testCandleSnapshots))
			for _, snap := range testCandleSnapshots {
				if snap.OpenTime >= start && snap.OpenTime < end {
					results = append(results, snap)
				}
			}
			require.NoError(t, json.NewEncoder(w).Encode(results))
		case "userFees":
			require.NoError(t, json.NewEncoder(w).Encode(testUserFeesResponse))
		case "fundingHistory":
			startVal, ok := payload["startTime"].(float64)
			require.True(t, ok)
			endVal, hasEnd := payload["endTime"].(float64)
			start := int64(startVal)
			end := int64(time.Now().UnixMilli())
			if hasEnd {
				end = int64(endVal)
			}
			entries := make([]map[string]any, 0, len(testFundingHistory))
			for _, entry := range testFundingHistory {
				timeVal, _ := entry["time"].(float64)
				if int64(timeVal) >= start && int64(timeVal) <= end {
					entries = append(entries, entry)
				}
			}
			require.NoError(t, json.NewEncoder(w).Encode(entries))
		case "metaAndAssetCtxs":
			require.NoError(t, json.NewEncoder(w).Encode([]any{testMetaResponse, testPerpAssetContexts}))
		case "clearinghouseState":
			require.NoError(t, json.NewEncoder(w).Encode(testUserStateResponse))
		case "userFunding":
			require.Contains(t, payload, "startTime")
			require.NoError(t, json.NewEncoder(w).Encode(testUserFundingHistory))
		case "userNonFundingLedgerUpdates":
			require.Contains(t, payload, "startTime")
			require.NoError(t, json.NewEncoder(w).Encode(testUserNonFundingLedger))
		default:
			t.Fatalf("unexpected info request type %v", typ)
		}
	}))

	ex := new(Exchange)
	ex.SetDefaults()
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL))
	require.NoError(t, ex.Requester.SetHTTPClient(server.Client()))
	ex.Accounts = accounts.MustNewAccounts(ex)

	cleanup := func() {
		server.Close()
	}
	return ex, cleanup
}

func createSignedExchangeResponder(t *testing.T, responder func(map[string]any) string) *Exchange {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, r.Body.Close())
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		switch r.URL.Path {
		case infoPath:
			require.Equal(t, "meta", payload["type"])
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(testMetaResponse))
		case "/exchange":
			respBody := `{"status":"ok"}`
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
		defer func() { _ = r.Body.Close() }()
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		if r.URL.Path == "/exchange" {
			captured = payload
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"txHash": "0xabc123",
			}))
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
	action := captured["action"].(map[string]any)
	require.Equal(t, "withdraw3", action["type"])

	req.Crypto.Chain = "unsupported"
	_, err = ex.WithdrawCryptocurrencyFunds(context.Background(), req)
	require.Error(t, err)
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
	action := captured["action"].(map[string]any)
	require.Equal(t, "order", action["type"])
	orders := action["orders"].([]any)
	require.Len(t, orders, 1)
	orderWire := orders[0].(map[string]any)
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
	action := captured["action"].(map[string]any)
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
	action := captured["action"].(map[string]any)
	require.Equal(t, "cancel", action["type"])
}

func TestCancelBatchOrders(t *testing.T) {
	var count int
	ex := createSignedExchangeResponder(t, func(payload map[string]any) string {
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
		defer func() { _ = r.Body.Close() }()
		switch r.URL.Path {
		case infoPath:
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			switch payload["type"] {
			case "meta":
				require.NoError(t, json.NewEncoder(w).Encode(testMetaResponse))
			case "openOrders":
				require.NoError(t, json.NewEncoder(w).Encode(testOpenOrders))
			default:
				t.Fatalf("unexpected info request type %v", payload["type"])
			}
		case "/exchange":
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
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
		action := actionPayload["action"].(map[string]any)
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
	require.Equal(t, currency.NewPair(currency.NewCode("BTC"), currency.USDC), contracts[0].Name)
}

func TestGetLatestFundingRates(t *testing.T) {
	ex, cleanup := setupInfoServer(t)
	defer cleanup()
	ex.now = func() time.Time { return time.UnixMilli(1700000200000) }

	resp, err := ex.GetLatestFundingRates(context.Background(), &fundingrate.LatestRateRequest{
		Asset: asset.PerpetualContract,
		Pair:  currency.NewPair(currency.NewCode("BTC"), currency.USDC),
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
		Pair:                 currency.NewPair(currency.NewCode("BTC"), currency.USDC),
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
	action := captured["action"].(map[string]any)
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

func currencyPair(t *testing.T, base, quote string) currency.Pair {
	t.Helper()
	b := currency.NewCode(base)
	q := currency.NewCode(quote)
	return currency.NewPair(b, q)
}
