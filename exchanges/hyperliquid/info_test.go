package hyperliquid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func newTestExchange(t *testing.T, baseURL string, client *http.Client) *Exchange {
	t.Helper()
	ex := new(Exchange)
	ex.SetDefaults()
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), baseURL), "endpoint must set")
	require.NoError(t, ex.Requester.SetHTTPClient(client), "client must set")
	return ex
}

func setupInfoTest(t *testing.T, expected map[string]any, response string) *Exchange {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer mustCloseBody(t, r.Body)
		if r.Method != http.MethodPost {
			t.Fatalf("method must be POST, got %s", r.Method)
		}
		if r.URL.Path != infoPath {
			t.Fatalf("path must be %s, got %s", infoPath, r.URL.Path)
		}
		var payload map[string]any
		mustDecodeJSON(t, r.Body, &payload)
		if !reflect.DeepEqual(payload, expected) {
			t.Fatalf("payload must match expected.\nexpected: %#v\ngot: %#v", expected, payload)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(response)); err != nil {
			t.Fatalf("response must write: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return newTestExchange(t, server.URL, server.Client())
}

func decodeValue[T any](t *testing.T, data string) T {
	t.Helper()
	var target T
	mustUnmarshalJSON(t, []byte(data), &target)
	return target
}

func TestGetUserState(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "clearinghouseState", "user": "0xabc", "dex": "perp"}
	response := `{"withdrawable":"12.34","marginSummary":{"accountValue":"1000","totalMarginUsed":"250"},"assetPositions":[{"position":{"coin":"BTC","szi":"1","marginUsed":"50","leverage":{"type":"cross","value":"5","rawUsd":"250"}}}]}`
	expected := decodeValue[*UserStateResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserState(context.Background(), "0xabc", "perp")
	require.NoError(t, err, "GetUserState must not error")
	require.Equal(t, expected, resp, "GetUserState response must match expected")
}

func TestGetSpotUserState(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "spotClearinghouseState", "user": "0xabc"}
	response := `{"balances":[{"coin":"USDC","token":1,"total":"100","hold":"5","entryNtl":"50","symbol":"USDC"}]}`
	expected := decodeValue[*SpotUserStateResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetSpotUserState(context.Background(), "0xabc")
	require.NoError(t, err, "GetSpotUserState must not error")
	require.Equal(t, expected, resp, "GetSpotUserState response must match expected")
}

func TestGetOpenOrders(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "openOrders", "user": "0xabc", "dex": "perp"}
	response := `[{"coin":"BTC","limitPx":"45000","oid":123,"side":"B","sz":"0.5","timestamp":1700000000000,"reduceOnly":false,"orderType":"limit","tif":"GTC","cloid":"0xbeef"}]`
	expected := decodeValue[[]OpenOrderResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetOpenOrders(context.Background(), "0xabc", "perp")
	require.NoError(t, err, "GetOpenOrders must not error")
	require.Equal(t, expected, resp, "GetOpenOrders response must match expected")
}

func TestGetFrontendOpenOrders(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "frontendOpenOrders", "user": "0xabc"}
	response := `[{"coin":"ETH","limitPx":"3500","oid":456,"side":"A","sz":"1.2","timestamp":1700000000100,"reduceOnly":true,"orderType":"limit","tif":"IOC"}]`
	expected := decodeValue[[]OpenOrderResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetFrontendOpenOrders(context.Background(), "0xabc", "")
	require.NoError(t, err, "GetFrontendOpenOrders must not error")
	require.Equal(t, expected, resp, "GetFrontendOpenOrders response must match expected")
}

func TestGetAllMids(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "allMids"}
	response := `{"BTC":"50000","ETH":"3500"}`
	expected := decodeValue[map[string]types.Number](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetAllMids(context.Background(), "")
	require.NoError(t, err, "GetAllMids must not error")
	require.Equal(t, expected, resp, "GetAllMids response must match expected")
}

func TestGetRecentPublicTrades(t *testing.T) {
	t.Parallel()
	request := map[string]any{
		"type":      "recentTrades",
		"coin":      "BTC",
		"n":         float64(5),
		"startTime": float64(1700000000000),
		"endTime":   float64(1700000005000),
	}
	response := `[{"coin":"BTC","side":"B","px":"50000","sz":"0.1","time":1700000001000,"tid":1,"hash":"0x1","users":["user1","user2"]}]`
	expected := decodeValue[[]RecentTrade](t, response)
	ex := setupInfoTest(t, request, response)

	limit := 5
	start := time.UnixMilli(1700000000000)
	end := time.UnixMilli(1700000005000)
	resp, err := ex.GetRecentPublicTrades(context.Background(), "BTC", &limit, &start, &end)
	require.NoError(t, err, "GetRecentPublicTrades must not error")
	require.Equal(t, expected, resp, "GetRecentPublicTrades response must match expected")
}

func TestGetUserFills(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userFills", "user": "0xabc"}
	response := `[{"coin":"BTC","px":"50000","sz":"0.2","side":"A","time":1700000002000,"startPosition":"0","dir":"open","closedPnl":"0","hash":"0x2","oid":100,"crossed":false,"fee":"10"}]`
	expected := decodeValue[[]UserFill](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserFills(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserFills must not error")
	require.Equal(t, expected, resp, "GetUserFills response must match expected")
}

func TestGetUserFillsByTime(t *testing.T) {
	t.Parallel()
	request := map[string]any{
		"type":            "userFillsByTime",
		"user":            "0xabc",
		"startTime":       float64(1700000000000),
		"endTime":         float64(1700000003000),
		"aggregateByTime": true,
	}
	response := `[{"coin":"ETH","px":"3500","sz":"1","side":"B","time":1700000002500,"startPosition":"0","dir":"close","closedPnl":"25","hash":"0x3","oid":200,"crossed":true,"fee":"5","feeToken":"HYPE"}]`
	expected := decodeValue[[]UserFill](t, response)
	ex := setupInfoTest(t, request, response)

	start := time.UnixMilli(1700000000000)
	end := time.UnixMilli(1700000003000)
	resp, err := ex.GetUserFillsByTime(context.Background(), "0xabc", start, &end, true)
	require.NoError(t, err, "GetUserFillsByTime must not error")
	require.Equal(t, expected, resp, "GetUserFillsByTime response must match expected")
}

func TestGetMeta(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "meta", "dex": "perp"}
	response := `{"universe":[{"name":"BTC","szDecimals":3,"maxLeverage":25,"marginTableId":1,"onlyIsolated":false,"isDelisted":false}]}`
	expected := decodeValue[*MetaResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetMeta(context.Background(), "perp")
	require.NoError(t, err, "GetMeta must not error")
	require.Equal(t, expected, resp, "GetMeta response must match expected")
}

func TestGetMetaAndAssetContexts(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "metaAndAssetCtxs"}
	response := `[{"universe":[{"name":"BTC","szDecimals":3,"maxLeverage":25,"marginTableId":1,"onlyIsolated":false,"isDelisted":false}]},[{"funding":"0.01","openInterest":"100","prevDayPx":"48000","dayNtlVlm":"1000000","premium":"0.001","oraclePx":"50000","markPx":"50500","midPx":"50250","impactPxs":["50000","51000"],"dayBaseVlm":"250","coin":"BTC"}]]`
	expected := decodeValue[*MetaAndAssetContextsResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetMetaAndAssetContexts(context.Background())
	require.NoError(t, err, "GetMetaAndAssetContexts must not error")
	require.Equal(t, expected, resp, "GetMetaAndAssetContexts response must match expected")
}

func TestGetPerpDexs(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "perpDexs"}
	response := `[null,{"name":"dex","fullName":"Dex","deployer":"0x1","oracleUpdater":null,"feeRecipient":"0x2","assetToStreamingOiCap":[["btc","1000"]]}]`
	expected := decodeValue[[]PerpDex](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetPerpDexs(context.Background())
	require.NoError(t, err, "GetPerpDexs must not error")
	require.Equal(t, expected, resp, "GetPerpDexs response must match expected")
}

func TestGetSpotMeta(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "spotMeta"}
	response := `{"universe":[{"tokens":[1,2],"name":"BTC/USDC","index":0,"isCanonical":true}],"tokens":[{"name":"BTC","szDecimals":8,"weiDecimals":18,"index":1,"tokenId":"btc","isCanonical":true}]}`
	expected := decodeValue[*SpotMetaResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetSpotMeta(context.Background())
	require.NoError(t, err, "GetSpotMeta must not error")
	require.Equal(t, expected, resp, "GetSpotMeta response must match expected")
}

func TestGetSpotMetaAndAssetContexts(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "spotMetaAndAssetCtxs"}
	response := `[{"universe":[{"tokens":[1,2],"name":"BTC/USDC","index":0,"isCanonical":true}],"tokens":[{"name":"BTC","szDecimals":8,"weiDecimals":18,"index":1,"tokenId":"btc","isCanonical":true}]},[{"dayNtlVlm":"100000","markPx":"50500","midPx":"50500","prevDayPx":"48000","circulatingSupply":"21000000","coin":"BTC"}]]`
	expected := decodeValue[*SpotMetaAndAssetContextsResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetSpotMetaAndAssetContexts(context.Background())
	require.NoError(t, err, "GetSpotMetaAndAssetContexts must not error")
	require.Equal(t, expected, resp, "GetSpotMetaAndAssetContexts response must match expected")
}

func TestGetFundingHistory(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "fundingHistory", "coin": "BTC", "startTime": float64(1700000000000)}
	response := `[{"coin":"BTC","fundingRate":"0.01","time":1700000001000,"period":"8h","premium":"0.002","bound":"upper","interval":"8h","oraclePx":"50000","markPx":"50500","nextFundingRate":"0.009"}]`
	expected := decodeValue[[]FundingHistoryEntry](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetFundingHistory(context.Background(), "BTC", time.UnixMilli(1700000000000), nil)
	require.NoError(t, err, "GetFundingHistory must not error")
	require.Equal(t, expected, resp, "GetFundingHistory response must match expected")
}

func TestGetUserFundingHistory(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userFunding", "user": "0xabc", "startTime": float64(1700000000000), "endTime": float64(1700000002000)}
	response := `[{"delta":{"coin":"BTC","fundingRate":"0.01","nSamples":8,"szi":"1","type":"funding","usdc":"10"},"hash":"0x10","time":1700000001500}]`
	expected := decodeValue[[]UserFundingHistoryEntry](t, response)
	ex := setupInfoTest(t, request, response)

	start := time.UnixMilli(1700000000000)
	end := time.UnixMilli(1700000002000)
	resp, err := ex.GetUserFundingHistory(context.Background(), "0xabc", start, &end)
	require.NoError(t, err, "GetUserFundingHistory must not error")
	require.Equal(t, expected, resp, "GetUserFundingHistory response must match expected")
}

func TestGetUserNonFundingLedgerUpdates(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userNonFundingLedgerUpdates", "user": "0xabc", "startTime": float64(1700000000000)}
	response := `[{"time":1700000001200,"hash":"0x20","delta":{"type":"transfer","usdc":"100","fee":"1","destination":"0xdest","user":"0xabc","vault":"0xvault","requestedUsd":"100","netWithdrawnUsd":"0","commission":"0","closingCost":"0","closeSz":"0","closingDir":"none","openSz":"0","openDir":"none","referrer":"0xref","reward":"0"}}]`
	expected := decodeValue[[]UserNonFundingLedgerEntry](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserNonFundingLedgerUpdates(context.Background(), "0xabc", time.UnixMilli(1700000000000), nil)
	require.NoError(t, err, "GetUserNonFundingLedgerUpdates must not error")
	require.Equal(t, expected, resp, "GetUserNonFundingLedgerUpdates response must match expected")
}

func TestGetL2Snapshot(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "l2Book", "coin": "BTC"}
	response := `{"coin":"BTC","time":1700000000000,"levels":[[{"px":"50000","sz":"1","n":10}],[{"px":"50500","sz":"2","n":8}]]}`
	expected := decodeValue[*OrderbookSnapshot](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetL2Snapshot(context.Background(), "BTC")
	require.NoError(t, err, "GetL2Snapshot must not error")
	require.Equal(t, expected, resp, "GetL2Snapshot response must match expected")
}

func TestGetCandleSnapshot(t *testing.T) {
	t.Parallel()
	request := map[string]any{
		"type": "candleSnapshot",
		"req": map[string]any{
			"coin":      "BTC",
			"interval":  "1m",
			"startTime": float64(1700000000000),
			"endTime":   float64(1700000006000),
		},
	}
	response := `[{"t":1700000000000,"T":1700000006000,"s":"BTC","i":"1m","o":"50000","c":"50500","h":"50600","l":"49900","v":"100","n":50}]`
	expected := decodeValue[[]CandleSnapshot](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetCandleSnapshot(context.Background(), "BTC", "1m", time.UnixMilli(1700000000000), time.UnixMilli(1700000006000))
	require.NoError(t, err, "GetCandleSnapshot must not error")
	require.Equal(t, expected, resp, "GetCandleSnapshot response must match expected")
}

func TestGetUserFees(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userFees", "user": "0xabc"}
	response := `{"userAddRate":"0.0002","userCrossRate":"0.0003","feeSchedule":{"add":"0.0005","cross":"0.0007"}}`
	expected := decodeValue[*UserFeesResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserFees(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserFees must not error")
	require.Equal(t, expected, resp, "GetUserFees response must match expected")
}

func TestGetUserStakingSummary(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "delegatorSummary", "user": "0xabc"}
	response := `{"delegated":"1000","undelegated":"100","totalPendingWithdrawal":"10","nPendingWithdrawals":1}`
	expected := decodeValue[*UserStakingSummaryResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserStakingSummary(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserStakingSummary must not error")
	require.Equal(t, expected, resp, "GetUserStakingSummary response must match expected")
}

func TestGetUserStakingDelegations(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "delegations", "user": "0xabc"}
	response := `[{"validator":"validator","amount":"100","lockedUntilTimestamp":1700000005000}]`
	expected := decodeValue[[]UserStakingDelegation](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserStakingDelegations(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserStakingDelegations must not error")
	require.Equal(t, expected, resp, "GetUserStakingDelegations response must match expected")
}

func TestGetUserStakingRewards(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "delegatorRewards", "user": "0xabc"}
	response := `[{"time":1700000004000,"source":"program","totalAmount":"5","token":"HYPE"}]`
	expected := decodeValue[[]UserStakingReward](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserStakingRewards(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserStakingRewards must not error")
	require.Equal(t, expected, resp, "GetUserStakingRewards response must match expected")
}

func TestGetDelegatorHistory(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "delegatorHistory", "user": "0xabc"}
	response := `[{"time":1700000004500,"hash":"0x30","delta":{"delegate":{"validator":"validator","amount":"10","isUndelegate":false}}}]`
	expected := decodeValue[[]DelegatorHistoryEntry](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetDelegatorHistory(context.Background(), "0xabc")
	require.NoError(t, err, "GetDelegatorHistory must not error")
	require.Equal(t, expected, resp, "GetDelegatorHistory response must match expected")
}

func TestGetOrderStatusByOID(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "orderStatus", "user": "0xabc", "oid": float64(123)}
	response := `{"status":"success","response":{"statuses":[{"status":"filled","statusTimestamp":1700000006000,"order":{"coin":"BTC","side":"B","limitPx":"50000","sz":"1","origSz":"1","oid":123,"timestamp":1700000005000,"orderType":"limit","tif":"GTC","reduceOnly":false}}]}}`
	expected := decodeValue[*OrderStatusResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetOrderStatusByOID(context.Background(), "0xabc", 123)
	require.NoError(t, err, "GetOrderStatusByOID must not error")
	require.Equal(t, expected, resp, "GetOrderStatusByOID response must match expected")
}

func TestGetOrderStatusByCloid(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "orderStatus", "user": "0xabc", "oid": "0xbeef"}
	response := `{"status":"success","response":{"statuses":[{"status":"open","statusTimestamp":1700000007000,"order":{"coin":"ETH","side":"A","limitPx":"3500","sz":"2","origSz":"2","oid":456,"timestamp":1700000006500,"orderType":"limit","tif":"GTC","reduceOnly":false,"cloid":"0xbeef"}}]}}`
	expected := decodeValue[*OrderStatusResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetOrderStatusByCloid(context.Background(), "0xabc", "0xbeef")
	require.NoError(t, err, "GetOrderStatusByCloid must not error")
	require.Equal(t, expected, resp, "GetOrderStatusByCloid response must match expected")
}

func TestGetReferralState(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "referral", "user": "0xabc"}
	response := `{"referredBy":"0xparent","cumVlm":"10000","unclaimedRewards":"50","claimedRewards":"25","builderRewards":"10","referrerState":{"stage":"active","data":{"tier":"gold"}},"rewardHistory":[{"time":1700000008000,"amount":"5","token":"HYPE","type":"referral","source":"trade"}],"tokenToState":[{"token":"HYPE","stage":"active","data":{"tier":"silver"}}]}`
	expected := decodeValue[*ReferralStateResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetReferralState(context.Background(), "0xabc")
	require.NoError(t, err, "GetReferralState must not error")
	require.Equal(t, expected, resp, "GetReferralState response must match expected")
}

func TestGetSubAccounts(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "subAccounts", "user": "0xabc"}
	response := `[{"name":"vault","subAccountUser":"0xvault","isVault":true,"vaultAddress":"0xvault","createdTimestamp":1700000009000}]`
	expected := decodeValue[[]SubAccount](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetSubAccounts(context.Background(), "0xabc")
	require.NoError(t, err, "GetSubAccounts must not error")
	require.Equal(t, expected, resp, "GetSubAccounts response must match expected")
}

func TestGetUserToMultiSigSigners(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userToMultiSigSigners", "user": "0xabc"}
	response := `[{"signer":"0xsigner","validUntil":1700000010000,"weight":1,"name":"Signer"}]`
	expected := decodeValue[[]UserMultiSigSigner](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserToMultiSigSigners(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserToMultiSigSigners must not error")
	require.Equal(t, expected, resp, "GetUserToMultiSigSigners response must match expected")
}

func TestGetPerpDeployAuctionStatus(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "perpDeployAuctionStatus"}
	response := `{"startTimeSeconds":1700000011000,"durationSeconds":3600,"startGas":"0.1","currentGas":"0.05","endGas":"0.01"}`
	expected := decodeValue[*PerpDeployAuctionStatusResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetPerpDeployAuctionStatus(context.Background())
	require.NoError(t, err, "GetPerpDeployAuctionStatus must not error")
	require.Equal(t, expected, resp, "GetPerpDeployAuctionStatus response must match expected")
}

func TestGetUserDexAbstraction(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userDexAbstraction", "user": "0xabc"}
	response := `{"user":"0xabc","enabled":true,"timestamp":1700000012000}`
	expected := decodeValue[*UserDexAbstractionResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserDexAbstraction(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserDexAbstraction must not error")
	require.Equal(t, expected, resp, "GetUserDexAbstraction response must match expected")
}

func TestGetHistoricalOrders(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "historicalOrders", "user": "0xabc"}
	response := `[{"order":{"coin":"BTC","side":"B","limitPx":"50000","sz":"1","origSz":"1","oid":123,"timestamp":1700000005000,"triggerPx":"49000","orderType":"limit","tif":"GTC","reduceOnly":false},"status":"filled","statusTimestamp":1700000006000}]`
	expected := decodeValue[[]HistoricalOrderEntry](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetHistoricalOrders(context.Background(), "0xabc")
	require.NoError(t, err, "GetHistoricalOrders must not error")
	require.Equal(t, expected, resp, "GetHistoricalOrders response must match expected")
}

func TestGetUserPortfolio(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "portfolio", "user": "0xabc"}
	response := `[["7d",{"accountValueHistory":[[1700000000000,"1000"]],"pnlHistory":[[1700000000000,"10"]],"vlm":"5000"}]]`
	expected := decodeValue[[]PortfolioPeriod](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserPortfolio(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserPortfolio must not error")
	require.Equal(t, expected, resp, "GetUserPortfolio response must match expected")
}

func TestGetUserTwapSliceFills(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userTwapSliceFills", "user": "0xabc"}
	response := `[{"coin":"BTC","px":"50000","sz":"0.5","side":"B","time":1700000013000,"startPosition":"0","dir":"open","closedPnl":"0","hash":"0x40","oid":789,"crossed":false,"fee":"1"}]`
	expected := decodeValue[[]UserFill](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserTwapSliceFills(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserTwapSliceFills must not error")
	require.Equal(t, expected, resp, "GetUserTwapSliceFills response must match expected")
}

func TestGetUserVaultEquities(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userVaultEquities", "user": "0xabc"}
	response := `[{"vaultAddress":"0xvault","equity":"1000","lockedUntilTimestamp":1700000014000,"pendingWithdrawal":"10"}]`
	expected := decodeValue[[]UserVaultEquity](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserVaultEquities(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserVaultEquities must not error")
	require.Equal(t, expected, resp, "GetUserVaultEquities response must match expected")
}

func TestGetUserRole(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userRole", "user": "0xabc"}
	response := `{"role":"trader","accountType":"standard"}`
	expected := decodeValue[*UserRoleResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserRole(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserRole must not error")
	require.Equal(t, expected, resp, "GetUserRole response must match expected")
}

func TestGetUserRateLimit(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "userRateLimit", "user": "0xabc"}
	response := `{"cumVlm":"1000","nRequestsUsed":10,"nRequestsCap":100}`
	expected := decodeValue[*UserRateLimitResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetUserRateLimit(context.Background(), "0xabc")
	require.NoError(t, err, "GetUserRateLimit must not error")
	require.Equal(t, expected, resp, "GetUserRateLimit response must match expected")
}

func TestGetSpotDeployState(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "spotDeployState", "user": "0xabc"}
	response := `{"states":[{"name":"deploy","data":{"status":"active"}}],"gasAuction":{"startTimeSeconds":1700000015000,"durationSeconds":1800,"startGas":"0.1","currentGas":"0.05","endGas":"0.01"}}`
	expected := decodeValue[*SpotDeployStateResponse](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetSpotDeployState(context.Background(), "0xabc")
	require.NoError(t, err, "GetSpotDeployState must not error")
	require.Equal(t, expected, resp, "GetSpotDeployState response must match expected")
}

func TestGetExtraAgents(t *testing.T) {
	t.Parallel()
	request := map[string]any{"type": "extraAgents", "user": "0xabc"}
	response := `[{"name":"agent","address":"0xagent","validUntil":1700000016000}]`
	expected := decodeValue[[]ExtraAgent](t, response)
	ex := setupInfoTest(t, request, response)

	resp, err := ex.GetExtraAgents(context.Background(), "0xabc")
	require.NoError(t, err, "GetExtraAgents must not error")
	require.Equal(t, expected, resp, "GetExtraAgents response must match expected")
}
