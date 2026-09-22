package binance

import (
	"os"
	"reflect"
	"testing"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

// These are Binance's official response examples; public live replays are
// separate so differences in the live service take precedence over examples.
func TestDocumentedWebsocketResponses(t *testing.T) {
	data, err := os.ReadFile("testdata/documented_websocket_responses.json")
	require.NoError(t, err, "WebSocket fixtures must load")
	var fixtures map[string]struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(data, &fixtures), "WebSocket fixtures must decode")
	local := mockSpotAPI(t, false, func(tb testing.TB, data []byte, conn *gws.Conn) error {
		tb.Helper()
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		require.NoError(tb, json.Unmarshal(data, &request), "mock request must decode")
		fixture, ok := fixtures[request.Method]
		require.True(tb, ok, "requested method must have a documented response")
		var response map[string]json.RawMessage
		require.NoError(tb, json.Unmarshal(fixture.Data, &response), "fixture envelope must decode")
		response["id"] = request.ID
		return conn.WriteJSON(response)
	})
	for _, tc := range []struct {
		method   string
		response any
	}{
		{"account.commission", new(*CommissionRateInto)},
		{"account.rateLimits.orders", new([]*RateLimitItem)},
		{"account.status", new(*Account)},
		{"allOrderLists", new([]*OCOOrder)},
		{"allOrders", new([]*TradeOrder)},
		{"myAllocations", new([]*SORReplacements)},
		{"myFilters", new(*AccountFiltersResponse)},
		{"myPreventedMatches", new([]*SelfTradePrevention)},
		{"myTrades", new([]*TradeHistory)},
		{"openOrderLists.status", new([]*OCOOrder)},
		{"openOrders.status", new([]*TradeOrder)},
		{"order.amendments", new([]*OrderAmendment)},
		{"orderList.status", new(*OCOOrderInfo)},
		{"order.status", new(*TradeOrder)},
		{"session.logon", new(*FuturesAuthenticationResponse)},
		{"session.logout", new(*FuturesAuthenticationResponse)},
		{"session.status", new(*FuturesAuthenticationResponse)},
		{"exchangeInfo", new(*ExchangeInfo)},
		{"executionRules", new(*SymbolExecutionRules)},
		{"ping", new(struct{})},
		{"time", new(*WsServerTimeResponse)},
		{"avgPrice", new(*SymbolAveragePrice)},
		{"blockTrades.historical", new([]*HistoricalBlockTrade)},
		{"depth", new(*OrderBook)},
		{"klines", new([]*CandleStick)},
		{"referencePrice", new(*ReferencePrice)},
		{"referencePrice.calculation", new(*ReferencePriceCalculation)},
		{"ticker", new(PriceChanges)},
		{"ticker.24hr", new(PriceChanges)},
		{"ticker.book", new(WsOrderbookTickers)},
		{"ticker.price", new(SymbolTickers)},
		{"ticker.tradingDay", new(PriceChanges)},
		{"trades.aggregate", new([]*AggregatedTrade)},
		{"trades.historical", new([]*HistoricalTrade)},
		{"trades.recent", new([]*RecentTrade)},
		{"uiKlines", new([]*CandleStick)},
		{"openOrders.cancelAll", new([]*WsCancelOrder)},
		{"order.amend.keepPriority", new(*AmendKeepPriorityResponse)},
		{"order.cancel", new(*TradeOrder)},
		{"order.cancelReplace", new(*WsCancelAndReplaceTradeOrderResponse)},
		{"orderList.cancel", new(*OCOOrder)},
		{"orderList.place", new(*OCOOrder)},
		{"orderList.place.oco", new(*OCOListOrderResponse)},
		{"orderList.place.opo", new(*OCOOrder)},
		{"orderList.place.opoco", new(*OCOOrder)},
		{"orderList.place.oto", new(*OCOOrder)},
		{"orderList.place.otoco", new(*OCOOrder)},
		{"order.place", new(*TradeOrderResponse)},
		{"order.test", new(*TestOrderResponse)},
		{"sor.order.place", new([]*OSROrder)},
		{"sor.order.test", new(*TestOrderResponse)},
		{"session.subscriptions", new([]*UserDataStreamSubscriptionResponse)},
		{"userDataStream.subscribe", new(*UserDataStreamSubscriptionResponse)},
		{"userDataStream.subscribe.signature", new(*UserDataStreamSubscriptionResponse)},
		{"userDataStream.unsubscribe", new(struct{})},
	} {
		t.Run(tc.method, func(t *testing.T) {
			require.NoError(t, local.SendWsRequest(tc.method, nil, tc.response), "documented response must decode through mockws")
			var envelope WsAPIResponse
			require.NoError(t, json.Unmarshal(fixtures[tc.method].Data, &envelope), "fixture envelope must decode")
			assertResponseFields(t, envelope.Result, reflect.TypeOf(tc.response), tc.method)
		})
	}
}
