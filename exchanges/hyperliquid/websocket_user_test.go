package hyperliquid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func TestHandleUserEvents(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)
	ex.accountAddr = "0xabc"

	payload := &wsUserEventsData{
		Fills: []UserFill{
			{
				Coin:          "BTC",
				Price:         types.Number(26000),
				Size:          types.Number(2),
				Side:          "B",
				Time:          types.Time(time.UnixMilli(1700000000000)),
				StartPosition: types.Number(1),
				Direction:     "open",
				ClosedPnl:     types.Number(0),
				Hash:          "0xhash",
				OrderID:       123,
				Crossed:       false,
				Fee:           types.Number(0.1),
			},
		},
	}

	require.NoError(t, ex.handleUserEvents(payload))

	select {
	case raw := <-ex.Websocket.DataHandler:
		update, ok := raw.(UserEventsUpdate)
		require.True(t, ok)
		assert.Equal(t, "0xabc", update.User)
		require.Len(t, update.Fills, 1)
		fill := update.Fills[0]
		assert.True(t, fill.Pair.Equal(currency.NewBTCUSDC()))
		assert.Equal(t, order.Buy, fill.Side)
		assert.InDelta(t, 26000, fill.Price, 1e-9)
		assert.Equal(t, asset.PerpetualContract, fill.Asset)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for user events update")
	}
}

func TestHandleUserFills(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)

	payload := &wsUserFillsPayload{
		User:       "0xdef",
		IsSnapshot: true,
		Fills: []UserFill{
			{
				Coin:          "SOL/USDC",
				Price:         types.Number(150),
				Size:          types.Number(4),
				Side:          "A",
				Time:          types.Time(time.UnixMilli(1700001000000)),
				StartPosition: types.Number(3),
				Direction:     "close",
				ClosedPnl:     types.Number(5),
				Hash:          "0xfeed",
				OrderID:       456,
				Crossed:       true,
				Fee:           types.Number(0.02),
			},
		},
	}

	require.NoError(t, ex.handleUserFills(payload))

	select {
	case raw := <-ex.Websocket.DataHandler:
		update, ok := raw.(UserFillsUpdate)
		require.True(t, ok)
		assert.True(t, update.IsSnapshot)
		assert.Equal(t, "0xdef", update.User)
		require.Len(t, update.Fills, 1)
		fill := update.Fills[0]
		spotPair, err := currency.NewPairFromString("SOL/USDC")
		require.NoError(t, err)
		assert.True(t, fill.Pair.Equal(spotPair))
		assert.Equal(t, order.Sell, fill.Side)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for user fills update")
	}
}

func TestHandleOrderUpdates(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)

	payload := &wsOrderUpdatesPayload{
		User: "0xabc",
		Statuses: []OrderStatusEntry{
			{
				Status:          "filled",
				StatusTimestamp: types.Time(time.UnixMilli(1700002000000)),
				Order: &OrderStatusOrder{
					Coin:        "BTC",
					Side:        "B",
					LimitPrice:  types.Number(20000),
					Size:        types.Number(0),
					OrigSize:    types.Number(1),
					OrderID:     789,
					Timestamp:   types.Time(time.UnixMilli(1700001000000)),
					OrderType:   "limit",
					TimeInForce: "gtc",
					ReduceOnly:  false,
				},
			},
		},
	}

	require.NoError(t, ex.handleOrderUpdates(payload))

	select {
	case raw := <-ex.Websocket.DataHandler:
		evt, ok := raw.(OrderUpdateEvent)
		require.True(t, ok)
		assert.Equal(t, "0xabc", evt.User)
		assert.Equal(t, order.Filled, evt.Detail.Status)
		assert.Equal(t, "789", evt.Detail.OrderID)
		assert.True(t, evt.Detail.Pair.Equal(currency.NewBTCUSDC()))
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for order update event")
	}
}

func TestHandleUserFunding(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)

	payload := &wsUserFundingPayload{
		User: "0xabc",
		UserFundingHistoryEntry: UserFundingHistoryEntry{
			Delta: struct {
				Coin        string       `json:"coin"`
				FundingRate string       `json:"fundingRate"`
				NSamples    int64        `json:"nSamples"`
				Szi         types.Number `json:"szi"`
				Type        string       `json:"type"`
				USDC        types.Number `json:"usdc"`
			}{Coin: "BTC", FundingRate: "0.01", NSamples: 8, Szi: types.Number(1), Type: "funding", USDC: types.Number(10)},
			Hash: "0xfunding",
			Time: types.Time(time.UnixMilli(1700003000000)),
		},
	}

	require.NoError(t, ex.handleUserFunding(payload))

	select {
	case raw := <-ex.Websocket.DataHandler:
		evt, ok := raw.(UserFundingEvent)
		require.True(t, ok)
		assert.Equal(t, "0xabc", evt.User)
		assert.Equal(t, payload.Time, evt.Entry.Time)
		assert.Equal(t, payload.Time.Time().UTC(), evt.Timestamp)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for user funding event")
	}
}

func TestHandleUserLedgerUpdates(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)

	payload := &wsUserNonFundingLedgerPayload{
		User: "0xabc",
		UserNonFundingLedgerEntry: UserNonFundingLedgerEntry{
			Time: types.Time(time.UnixMilli(1700004000000)),
			Hash: "0xledger",
		},
	}

	payload.UserNonFundingLedgerEntry.Delta.Type = "withdrawal"
	payload.UserNonFundingLedgerEntry.Delta.USDC = types.Number(50)

	require.NoError(t, ex.handleUserLedgerUpdates(payload))

	select {
	case raw := <-ex.Websocket.DataHandler:
		evt, ok := raw.(UserLedgerUpdateEvent)
		require.True(t, ok)
		assert.Equal(t, "0xabc", evt.User)
		assert.Equal(t, payload.Time, evt.Entry.Time)
		assert.Equal(t, payload.Time.Time().UTC(), evt.Timestamp)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for user ledger event")
	}
}

func TestHandleWebData2(t *testing.T) {
	t.Parallel()

	ex := setupWebsocketExchange(t)

	payload := &wsWebData2Payload{
		User: "0xabc",
		Data: map[string]any{"key": "value", "count": 5},
	}

	require.NoError(t, ex.handleWebData2(payload))

	select {
	case raw := <-ex.Websocket.DataHandler:
		evt, ok := raw.(WebData2Event)
		require.True(t, ok)
		assert.Equal(t, "0xabc", evt.User)
		require.Len(t, evt.Data, 2)
		assert.Equal(t, "value", evt.Data["key"])
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webData2 event")
	}
}
