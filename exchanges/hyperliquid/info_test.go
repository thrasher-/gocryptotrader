package hyperliquid

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

func newTestExchange(t *testing.T, baseURL string, client *http.Client) *Exchange {
	t.Helper()
	e := new(Exchange)
	e.SetDefaults()
	require.NoError(t, e.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), baseURL))
	require.NoError(t, e.Requester.SetHTTPClient(client))
	return e
}

func TestInfoEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		call     func(ctx context.Context, e *Exchange) (json.RawMessage, error)
		expected map[string]any
	}{
		{
			name: "GetUserState",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserState(ctx, "0xabc", "perp")
			},
			expected: map[string]any{"type": "clearinghouseState", "user": "0xabc", "dex": "perp"},
		},
		{
			name: "GetSpotUserState",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetSpotUserState(ctx, "0xabc")
			},
			expected: map[string]any{"type": "spotClearinghouseState", "user": "0xabc"},
		},
		{
			name: "GetOpenOrders",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetOpenOrders(ctx, "0xabc", "perp")
			},
			expected: map[string]any{"type": "openOrders", "user": "0xabc", "dex": "perp"},
		},
		{
			name: "GetFrontendOpenOrders",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetFrontendOpenOrders(ctx, "0xabc", "")
			},
			expected: map[string]any{"type": "frontendOpenOrders", "user": "0xabc"},
		},
		{
			name: "GetAllMids",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetAllMids(ctx, "")
			},
			expected: map[string]any{"type": "allMids"},
		},
		{
			name: "GetUserFills",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserFills(ctx, "0xabc")
			},
			expected: map[string]any{"type": "userFills", "user": "0xabc"},
		},
		{
			name: "GetUserFillsByTime",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				end := int64(2)
				return e.GetUserFillsByTime(ctx, "0xabc", 1, &end, true)
			},
			expected: map[string]any{"type": "userFillsByTime", "user": "0xabc", "startTime": float64(1), "endTime": float64(2), "aggregateByTime": true},
		},
		{
			name: "GetMeta",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetMeta(ctx, "")
			},
			expected: map[string]any{"type": "meta"},
		},
		{
			name: "GetMetaAndAssetContexts",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetMetaAndAssetContexts(ctx)
			},
			expected: map[string]any{"type": "metaAndAssetCtxs"},
		},
		{
			name: "GetPerpDexs",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetPerpDexs(ctx)
			},
			expected: map[string]any{"type": "perpDexs"},
		},
		{
			name: "GetSpotMeta",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetSpotMeta(ctx)
			},
			expected: map[string]any{"type": "spotMeta"},
		},
		{
			name: "GetSpotMetaAndAssetContexts",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetSpotMetaAndAssetContexts(ctx)
			},
			expected: map[string]any{"type": "spotMetaAndAssetCtxs"},
		},
		{
			name: "GetFundingHistory",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetFundingHistory(ctx, "BTC", 1, nil)
			},
			expected: map[string]any{"type": "fundingHistory", "coin": "BTC", "startTime": float64(1)},
		},
		{
			name: "GetUserFundingHistory",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				end := int64(3)
				return e.GetUserFundingHistory(ctx, "0xabc", 2, &end)
			},
			expected: map[string]any{"type": "userFunding", "user": "0xabc", "startTime": float64(2), "endTime": float64(3)},
		},
		{
			name: "GetL2Snapshot",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetL2Snapshot(ctx, "BTC")
			},
			expected: map[string]any{"type": "l2Book", "coin": "BTC"},
		},
		{
			name: "GetCandleSnapshot",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetCandleSnapshot(ctx, "BTC", "1m", 1, 2)
			},
			expected: map[string]any{
				"type": "candleSnapshot",
				"req": map[string]any{
					"coin":      "BTC",
					"interval":  "1m",
					"startTime": float64(1),
					"endTime":   float64(2),
				},
			},
		},
		{
			name: "GetUserFees",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserFees(ctx, "0xabc")
			},
			expected: map[string]any{"type": "userFees", "user": "0xabc"},
		},
		{
			name: "GetUserStakingSummary",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserStakingSummary(ctx, "0xabc")
			},
			expected: map[string]any{"type": "delegatorSummary", "user": "0xabc"},
		},
		{
			name: "GetUserStakingDelegations",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserStakingDelegations(ctx, "0xabc")
			},
			expected: map[string]any{"type": "delegations", "user": "0xabc"},
		},
		{
			name: "GetUserStakingRewards",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserStakingRewards(ctx, "0xabc")
			},
			expected: map[string]any{"type": "delegatorRewards", "user": "0xabc"},
		},
		{
			name: "GetDelegatorHistory",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetDelegatorHistory(ctx, "0xabc")
			},
			expected: map[string]any{"type": "delegatorHistory", "user": "0xabc"},
		},
		{
			name: "GetOrderStatusByOID",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetOrderStatusByOID(ctx, "0xabc", 42)
			},
			expected: map[string]any{"type": "orderStatus", "user": "0xabc", "oid": float64(42)},
		},
		{
			name: "GetOrderStatusByCloid",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetOrderStatusByCloid(ctx, "0xabc", "0xfeed")
			},
			expected: map[string]any{"type": "orderStatus", "user": "0xabc", "oid": "0xfeed"},
		},
		{
			name: "GetReferralState",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetReferralState(ctx, "0xabc")
			},
			expected: map[string]any{"type": "referral", "user": "0xabc"},
		},
		{
			name: "GetSubAccounts",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetSubAccounts(ctx, "0xabc")
			},
			expected: map[string]any{"type": "subAccounts", "user": "0xabc"},
		},
		{
			name: "GetUserToMultiSigSigners",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserToMultiSigSigners(ctx, "0xabc")
			},
			expected: map[string]any{"type": "userToMultiSigSigners", "user": "0xabc"},
		},
		{
			name: "GetPerpDeployAuctionStatus",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetPerpDeployAuctionStatus(ctx)
			},
			expected: map[string]any{"type": "perpDeployAuctionStatus"},
		},
		{
			name: "GetUserDexAbstraction",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserDexAbstraction(ctx, "0xabc")
			},
			expected: map[string]any{"type": "userDexAbstraction", "user": "0xabc"},
		},
		{
			name: "GetHistoricalOrders",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetHistoricalOrders(ctx, "0xabc")
			},
			expected: map[string]any{"type": "historicalOrders", "user": "0xabc"},
		},
		{
			name: "GetUserPortfolio",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserPortfolio(ctx, "0xabc")
			},
			expected: map[string]any{"type": "portfolio", "user": "0xabc"},
		},
		{
			name: "GetUserTwapSliceFills",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserTwapSliceFills(ctx, "0xabc")
			},
			expected: map[string]any{"type": "userTwapSliceFills", "user": "0xabc"},
		},
		{
			name: "GetUserVaultEquities",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserVaultEquities(ctx, "0xabc")
			},
			expected: map[string]any{"type": "userVaultEquities", "user": "0xabc"},
		},
		{
			name: "GetUserRole",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserRole(ctx, "0xabc")
			},
			expected: map[string]any{"type": "userRole", "user": "0xabc"},
		},
		{
			name: "GetUserRateLimit",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetUserRateLimit(ctx, "0xabc")
			},
			expected: map[string]any{"type": "userRateLimit", "user": "0xabc"},
		},
		{
			name: "GetSpotDeployState",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetSpotDeployState(ctx, "0xabc")
			},
			expected: map[string]any{"type": "spotDeployState", "user": "0xabc"},
		},
		{
			name: "GetExtraAgents",
			call: func(ctx context.Context, e *Exchange) (json.RawMessage, error) {
				return e.GetExtraAgents(ctx, "0xabc")
			},
			expected: map[string]any{"type": "extraAgents", "user": "0xabc"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var captured map[string]any
			response := `{"status":"ok"}`
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, infoPath, r.URL.Path)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(body, &captured))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			defer s.Close()

			e := newTestExchange(t, s.URL, s.Client())
			ctx := context.Background()
			resp, err := tc.call(ctx, e)
			require.NoError(t, err)
			require.JSONEq(t, response, string(resp))
			require.Equal(t, tc.expected, captured)
		})
	}
}
