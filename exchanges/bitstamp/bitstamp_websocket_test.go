package bitstamp

import (
	"maps"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fill"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var (
	wsSpotPair      = currency.NewPairWithDelimiter("BTC", "USD", "/")
	wsPerpetualPair = currency.NewPairWithDelimiter("BTC", "USD-PERP", "/")
)

// newWebsocketTestInstance returns an exchange instance named after the test, so the orderbooks it stores do not
// collide with those of other tests
func newWebsocketTestInstance(t *testing.T) *Exchange {
	t.Helper()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	ex.Name = t.Name()
	return ex
}

// dataHandlerPayloads closes the data handler and returns everything sent to it
func dataHandlerPayloads(ex *Exchange) []any {
	ex.Websocket.DataHandler.Close()
	var payloads []any
	for p := range ex.Websocket.DataHandler.C {
		payloads = append(payloads, p.Data)
	}
	return payloads
}

func TestWsHandleDataErrors(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	for _, tc := range []struct {
		name string
		msg  string
		err  error
	}{
		{name: "missing event", msg: `{"channel":"live_trades_btcusd","data":{}}`, err: common.ErrParsingWSField},
		{name: "missing channel", msg: `{"event":"trade","data":{}}`, err: common.ErrParsingWSField},
		{name: "malformed channel", msg: `{"event":"trade","channel":"livetrades","data":{}}`, err: errMalformedChannel},
		{name: "unknown channel market", msg: `{"event":"trade","channel":"live_trades","data":{}}`, err: errParsingWSPair},
		{name: "missing data", msg: `{"event":"trade","channel":"live_trades_btcusd"}`, err: common.ErrParsingWSField},
		{name: "unknown market", msg: `{"event":"trade","channel":"live_trades_btcwibble","data":{}}`, err: errParsingWSPair},
		{name: "unrequested subscription", msg: `{"event":"bts:subscription_succeeded","channel":"live_trades_btcusd","data":{}}`, err: websocket.ErrSignatureNotMatched},
		{name: "subscription without channel", msg: `{"event":"bts:subscription_succeeded","data":{}}`, err: common.ErrParsingWSField},
		{name: "server error", msg: `{"event":"bts:error","channel":"","data":{"code":4009,"message":"Connection is unauthorized."}}`, err: errWebsocketError},
		{name: "order without ID", msg: `{"event":"order_created","channel":"private-my_orders_btcusd-314159","data":{"amount_str":"1"}}`, err: errOrderIDMissing},
		{name: "malformed order", msg: `{"event":"order_created","channel":"private-my_orders_btcusd-314159","data":{"id":"wibble"}}`},
		{name: "malformed orderbook", msg: `{"event":"data","channel":"order_book_btcusd","data":{"bids":"wibble"}}`},
		{name: "malformed funding rate", msg: `{"event":"funding_rate_saved","channel":"funding_rate_btcusd-perp","data":{"funding_rate":[]}}`},
		{name: "malformed settlement", msg: `{"event":"settlement","channel":"private-my_settlements-314159","data":{"as":[]}}`},
		{name: "malformed liquidation", msg: `{"event":"liquidation_start","channel":"private-my_liquidations-314159","data":{"initial_margin_ratio":[]}}`},
		{name: "malformed token settlement", msg: `{"event":"token_ready","channel":"private-my_token_settlements-314159","data":{"order_id":"wibble"}}`},
		{name: "malformed announcement", msg: `{"event":"published","channel":"announcements","data":{"announcement_id":"wibble"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ex.wsHandleData(t.Context(), []byte(tc.msg))
			if tc.err == nil {
				assert.Error(t, err, "wsHandleData should error")
				return
			}
			assert.ErrorIs(t, err, tc.err, "wsHandleData should return the correct error")
		})
	}
}

func TestWsHandleDataIgnoredEvents(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	for _, msg := range []string{
		`{"event":"bts:heartbeat","channel":"","data":{"status":"success"}}`,
		`{"event":"order_created","channel":"live_orders_btcusd-perp","data":{"id":2054076732301440,"order_type":0,"order_subtype":5,"amount_str":"0.00134","price_str":"75609"}}`,
		`{"event":"trade","channel":"live_trades_btcusd","data":{"id":104007706,"amount_str":"0.00598803","price_str":"9334.73","type":1,"microtimestamp":"1580336751488517"}}`,
		`{"event":"trade","channel":"private-my_trades_btcusd-314159","data":{"id":296050733,"order_id":1500000001,"amount":0.1,"price":61200,"side":"buy"}}`,
	} {
		assert.NoErrorf(t, ex.wsHandleData(t.Context(), []byte(msg)), "wsHandleData should not error for %s", msg)
	}
	assert.Empty(t, dataHandlerPayloads(ex), "heartbeats, public orders and disabled trade and fill feeds should not be sent to the data handler")
}

func TestWsHandleDataUnhandled(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	for _, msg := range []string{
		`{"event":"data","channel":"diff_order_book_btcusd","data":{}}`,
		`{"event":"settlement","channel":"my_settlements","data":{}}`,
		`{"event":"order_unknown","channel":"private-my_orders_btcusd-314159","data":{"id":1500000001}}`,
	} {
		assert.NoErrorf(t, ex.wsHandleData(t.Context(), []byte(msg)), "wsHandleData should not error for %s", msg)
	}
	payloads := dataHandlerPayloads(ex)
	require.Len(t, payloads, 3, "each unhandled message must be sent to the data handler")
	for _, p := range payloads {
		assert.IsType(t, websocket.UnhandledMessageWarning{}, p, "unhandled messages should be sent as warnings")
	}
}

func TestWsRequestReconnect(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	err := ex.wsHandleData(t.Context(), []byte(`{"event":"bts:request_reconnect","channel":"","data":null}`))
	assert.NoError(t, err, "wsHandleData should not error on a reconnection request")
}

func TestHandleWSError(t *testing.T) {
	t.Parallel()
	err := handleWSError([]byte(`{"event":"bts:error","channel":"","data":{"code":4009,"message":"Connection is unauthorized."}}`))
	assert.ErrorIs(t, err, errWebsocketError, "handleWSError should return a websocket error")
	assert.ErrorContains(t, err, "4009: Connection is unauthorized.", "handleWSError should include the code and message")

	err = handleWSError([]byte(`{"event":"bts:error","channel":"","data":{"code":null,"message":"Bad subscription string."}}`))
	assert.ErrorIs(t, err, errWebsocketError, "handleWSError should return a websocket error without a code")
	assert.EqualError(t, err, "websocket error: Bad subscription string.", "handleWSError should only include the message without a code")

	err = handleWSError([]byte(`{"event":"bts:error","channel":""}`))
	assert.ErrorIs(t, err, common.ErrParsingWSField, "handleWSError should error without data")

	err = handleWSError([]byte(`{"event":"bts:error","channel":"","data":{"code":"wibble"}}`))
	assert.Error(t, err, "handleWSError should error on malformed data")
}

func TestWsOrderbook(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	for _, msg := range []string{
		`{"data":{"timestamp":"1580336834","microtimestamp":"1580336834607546","bids":[["9328.28","0.05925332"],["9327.34","0.43120000"],["0","69"]],"asks":[["9337.10","0.03000000"],["9340.85","2.67820000"]]},"event":"data","channel":"order_book_btcusd"}`,
		`{"data":{"timestamp":"1790318551","microtimestamp":"1790318551908216","bids":[["84010","0.01190"],["84006","0.33643"]],"asks":[["84018","0.33440"],["84021","1.07000"]]},"channel":"order_book_btcusd-perp","event":"data"}`,
	} {
		require.NoErrorf(t, ex.wsHandleData(t.Context(), []byte(msg)), "wsHandleData must not error for %s", msg)
	}

	ob, err := ex.Websocket.Orderbook.GetOrderbook(wsSpotPair, asset.Spot)
	require.NoError(t, err, "GetOrderbook must not error for spot")
	assert.Equal(t, orderbook.Levels{{Price: 9328.28, Amount: 0.05925332}, {Price: 9327.34, Amount: 0.4312}}, ob.Bids, "Bids should exclude the zero priced bid")
	assert.Equal(t, orderbook.Levels{{Price: 9337.1, Amount: 0.03}, {Price: 9340.85, Amount: 2.6782}}, ob.Asks, "Asks should be correct")
	assert.Equal(t, time.UnixMicro(1580336834607546), ob.LastUpdated, "LastUpdated should be the microtimestamp")

	ob, err = ex.Websocket.Orderbook.GetOrderbook(wsPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetOrderbook must not error for perpetual contracts")
	assert.Equal(t, orderbook.Levels{{Price: 84010, Amount: 0.0119}, {Price: 84006, Amount: 0.33643}}, ob.Bids, "Bids should be correct")
	assert.Equal(t, orderbook.Levels{{Price: 84018, Amount: 0.3344}, {Price: 84021, Amount: 1.07}}, ob.Asks, "Asks should be correct")
}

func TestWsTrade(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	ex.SetTradeFeedStatus(true)
	ex.Websocket.Trade.Setup(true, ex.Websocket.DataHandler)
	for _, msg := range []string{
		`{"data":{"microtimestamp":"1580336751488517","amount":0.00598803,"buy_order_id":4621328909,"sell_order_id":4621329035,"amount_str":"0.00598803","price_str":"9334.73","timestamp":"1580336751","price":9334.73,"type":1,"id":104007706},"event":"trade","channel":"live_trades_btcusd"}`,
		`{"data":{"id":502711286,"timestamp":"1790318552","amount":0.01,"amount_str":"0.01000","price":84010,"price_str":"84010","type":0,"microtimestamp":"1790318552115000","buy_order_id":2054076731695232,"sell_order_id":2054076732833920},"channel":"live_trades_btcusd-perp","event":"trade"}`,
	} {
		require.NoErrorf(t, ex.wsHandleData(t.Context(), []byte(msg)), "wsHandleData must not error for %s", msg)
	}
	assert.Equal(t, []any{
		[]trade.Data{{
			TID:          "104007706",
			Exchange:     ex.Name,
			CurrencyPair: wsSpotPair,
			AssetType:    asset.Spot,
			Side:         order.Sell,
			Price:        9334.73,
			Amount:       0.00598803,
			Timestamp:    time.UnixMicro(1580336751488517),
		}},
		[]trade.Data{{
			TID:          "502711286",
			Exchange:     ex.Name,
			CurrencyPair: wsPerpetualPair,
			AssetType:    asset.PerpetualContract,
			Side:         order.Buy,
			Price:        84010,
			Amount:       0.01,
			Timestamp:    time.UnixMicro(1790318552115000),
		}},
	}, dataHandlerPayloads(ex), "trades should be sent to the data handler")
}

func TestWsFundingRate(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	msg := `{"data":{"market":"btcusd-perp","mark_price":"83997.21486186","index_price":"83991.59300000002","funding_rate":"0.0001","timestamp":"1790318552","next_funding_time":"1790323200"},"channel":"funding_rate_btcusd-perp","event":"funding_rate_saved"}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(msg)), "wsHandleData must not error")
	assert.Equal(t, []any{websocket.FundingData{
		Timestamp:    time.Unix(1790318552, 0),
		CurrencyPair: wsPerpetualPair,
		AssetType:    asset.PerpetualContract,
		Exchange:     ex.Name,
		Rate:         0.0001,
	}}, dataHandlerPayloads(ex), "funding rates should be sent to the data handler")
}

func TestWsOrderUpdate(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	testexch.FixtureToDataHandler(t, "testdata/wsMyOrders.json", ex.wsHandleData)
	payloads := dataHandlerPayloads(ex)
	require.Len(t, payloads, 8, "the data handler must receive an update for each order event")

	// The fixture was captured before order subtypes were published, so every order is decoded as a limit order
	partialMarketBuy, partialMarketSell := 0.00038667-0.00000001, 0.00038679-0.00000001
	for i, exp := range []struct {
		orderID, clientOrderID                    string
		status                                    order.Status
		side                                      order.Side
		price, amount, remainingAmount, execution float64
		date                                      time.Time
	}{
		{"1658864794234880", "test_market_buy", order.New, order.Buy, 999999999, 0, 0, 0, time.UnixMicro(1693831262313000)},
		{"1658864794234880", "test_market_buy", order.PartiallyFilled, order.Buy, 25862, 0.00038667, 0.00000001, partialMarketBuy, time.UnixMicro(1693831262313000)},
		{"1658864794234880", "test_market_buy", order.Cancelled, order.Buy, 25862, 0.00038667, 0.00000001, partialMarketBuy, time.UnixMicro(1693831262313000)},
		{"1658870500933632", "test_market_sell", order.New, order.Sell, 0, 0, 0, 0, time.UnixMicro(1693832655550000)},
		{"1658870500933632", "test_market_sell", order.PartiallyFilled, order.Sell, 25854, 0.00038679, 0.00000001, partialMarketSell, time.UnixMicro(1693832655550000)},
		{"1658870500933632", "test_market_sell", order.Cancelled, order.Sell, 25854, 0.00038679, 0.00000001, partialMarketSell, time.UnixMicro(1693832655550000)},
		{"1658869033291777", "test_limit_sell", order.New, order.Sell, 25845, 0.00038692, 0.00038692, 0, time.UnixMicro(1693832297239000)},
		{"1658869033291777", "test_limit_sell", order.Filled, order.Sell, 25845, 0.00038692, 0, 0.00038692, time.UnixMicro(1693832302664000)},
	} {
		d, ok := payloads[i].(*order.Detail)
		require.Truef(t, ok, "payload %d must be an order detail", i)
		assert.Equalf(t, &order.Detail{
			Exchange:        ex.Name,
			OrderID:         exp.orderID,
			ClientOrderID:   exp.clientOrderID,
			Type:            order.Limit,
			TimeInForce:     order.GoodTillCancel,
			Side:            exp.side,
			Status:          exp.status,
			AssetType:       asset.Spot,
			Pair:            wsSpotPair,
			Date:            exp.date,
			Price:           exp.price,
			Amount:          exp.amount,
			RemainingAmount: exp.remainingAmount,
			ExecutedAmount:  exp.execution,
		}, d, "order event %d should be correct", i)
	}
}

func TestWsOrderEvents(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	for _, msg := range []string{
		`{"event":"order_replaced","channel":"private-my_orders_btcusd-perp-123456","trade_account_id":0,"event_id":"019f4ac8-5e3b-11ef-af00-3e9012000021","order_source":"orderbook","data":{"id":1500000002,"id_str":"1500000002","orig_order_id":1500000001,"client_order_id":"my-order-002","amount":0.3,"amount_str":"0.30000000","amount_traded":"0.00000000","amount_at_create":"0.30000000","price":61500.00,"price_str":"61500.00","order_type":0,"order_subtype":8,"datetime":"1712131201","microtimestamp":"1712131201000000","trade_account_id":0,"is_liquidation":false}}`,
		`{"event":"stop_active","channel":"private-my_orders_btcusd-perp-123456","data":{"id":1500000003,"id_str":"1500000003","amount":0.01,"amount_str":"0.01000","amount_traded":"0","amount_at_create":"0.01000","price":83000,"price_str":"83000","order_type":1,"order_subtype":22,"datetime":"1712131202","microtimestamp":"1712131202000000","trade_account_id":0,"is_liquidation":false,"reduce_only":true,"stop_price":"83500"}}`,
		`{"event":"stop_inactive","channel":"private-my_orders_btcusd-perp-123456","data":{"id":1500000004,"id_str":"1500000004","amount":0.01,"amount_str":"0.01000","amount_traded":"0","amount_at_create":"0.01000","price":0,"price_str":"0","order_type":1,"order_subtype":20,"datetime":"1712131203","microtimestamp":"1712131203000000","trade_account_id":0,"is_liquidation":false,"stop_price":"82000"}}`,
	} {
		require.NoErrorf(t, ex.wsHandleData(t.Context(), []byte(msg)), "wsHandleData must not error for %s", msg)
	}
	assert.Equal(t, []any{
		&order.Detail{
			Exchange:        ex.Name,
			OrderID:         "1500000002",
			ClientOrderID:   "my-order-002",
			Type:            order.Limit,
			TimeInForce:     order.GoodTillTime,
			Side:            order.Buy,
			Status:          order.New,
			AssetType:       asset.PerpetualContract,
			Pair:            wsPerpetualPair,
			Date:            time.UnixMicro(1712131201000000),
			Price:           61500,
			Amount:          0.3,
			RemainingAmount: 0.3,
		},
		&order.Detail{
			Exchange:    ex.Name,
			OrderID:     "1500000001",
			Status:      order.Cancelled,
			AssetType:   asset.PerpetualContract,
			Pair:        wsPerpetualPair,
			LastUpdated: time.UnixMicro(1712131201000000),
		},
		&order.Detail{
			Exchange:        ex.Name,
			OrderID:         "1500000003",
			Type:            order.StopLimit,
			Side:            order.Sell,
			Status:          order.Active,
			AssetType:       asset.PerpetualContract,
			Pair:            wsPerpetualPair,
			Date:            time.UnixMicro(1712131202000000),
			Price:           83000,
			Amount:          0.01,
			RemainingAmount: 0.01,
			ReduceOnly:      true,
			TriggerPrice:    83500,
		},
		&order.Detail{
			Exchange:        ex.Name,
			OrderID:         "1500000004",
			Type:            order.StopMarket,
			Side:            order.Sell,
			Status:          order.Cancelled,
			AssetType:       asset.PerpetualContract,
			Pair:            wsPerpetualPair,
			Date:            time.UnixMicro(1712131203000000),
			Amount:          0.01,
			RemainingAmount: 0.01,
			TriggerPrice:    82000,
		},
	}, dataHandlerPayloads(ex), "order events should be sent to the data handler")
}

func TestOrderTypeFromSubtypeID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		subtype uint8
		typ     order.Type
		tif     order.TimeInForce
	}{
		{0, order.Limit, order.GoodTillCancel},
		{1, order.Market, order.UnknownTIF},
		{2, order.Market, order.UnknownTIF},
		{3, order.Limit, order.GoodTillDay},
		{4, order.Limit, order.ImmediateOrCancel},
		{5, order.Limit, order.PostOnly},
		{6, order.Limit, order.FillOrKill},
		{7, order.Market, order.UnknownTIF},
		{8, order.Limit, order.GoodTillTime},
		{20, order.StopMarket, order.UnknownTIF},
		{21, order.TakeProfitMarket, order.UnknownTIF},
		{22, order.StopLimit, order.UnknownTIF},
		{23, order.TakeProfit | order.Limit, order.UnknownTIF},
		{24, order.TrailingStop, order.UnknownTIF},
		{25, order.TrailingStop, order.UnknownTIF},
		{26, order.TrailingStopLimit, order.UnknownTIF},
		{27, order.TrailingStopLimit, order.UnknownTIF},
		{99, order.UnknownType, order.UnknownTIF},
	} {
		typ, tif := orderTypeFromSubtypeID(tc.subtype)
		assert.Equalf(t, tc.typ, typ, "orderTypeFromSubtypeID should return the order type for %d", tc.subtype)
		assert.Equalf(t, tc.tif, tif, "orderTypeFromSubtypeID should return the time in force for %d", tc.subtype)
	}
}

func TestWsMyTrade(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	ex.SetFillsFeedStatus(true)
	ex.Websocket.Fills.Setup(true, ex.Websocket.DataHandler)
	msg := `{"event":"trade","channel":"private-my_trades_btcusd-perp-123456","data":{"id":296050733,"id_str":"296050733","trade_uti":"BSTP-20240403-296050733","order_id":1500000001,"client_order_id":"my-order-001","amount":0.1,"price":61200.00,"fee":"0.00","side":"buy","microtimestamp":"1712131200500000","trade_account_id":0,"position_id":null,"is_liquidation":null,"trade_type":"ORDERBOOK"}}`
	require.NoError(t, ex.wsHandleData(t.Context(), []byte(msg)), "wsHandleData must not error")
	err := ex.wsHandleData(t.Context(), []byte(`{"event":"trade","channel":"private-my_trades_btcusd-123456","data":{"id":1,"side":"wibble"}}`))
	assert.ErrorIs(t, err, order.ErrSideIsInvalid, "wsHandleData should error on an invalid side")
	assert.Equal(t, []any{[]fill.Data{{
		Timestamp:     time.UnixMicro(1712131200500000),
		Exchange:      ex.Name,
		AssetType:     asset.PerpetualContract,
		CurrencyPair:  wsPerpetualPair,
		Side:          order.Buy,
		OrderID:       "1500000001",
		ClientOrderID: "my-order-001",
		TradeID:       "296050733",
		Price:         61200,
		Amount:        0.1,
	}}}, dataHandlerPayloads(ex), "fills should be sent to the data handler")
}

func TestWsAccountChannels(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	for _, msg := range []string{
		`{"event":"settlement","channel":"private-my_settlements-123456","data":{"id":"1234567890","tnx_id":"123123","ts":"1712131200","price":"61200","ccy":"USD","as":"-1.5","ap":"-1","atf":"-0.25","alf":"0","afu":"-0.25","asl":"0","sp":"61000","typ":"C"}}`,
		`{"event":"insurance_fund_premium","channel":"private-my_settlements-123456","data":{"tnx_id":"123124","ts":"1712131200","ccy":"USD","amount":"-0.5"}}`,
		`{"event":"liquidation_start","channel":"private-my_liquidations-123456","data":{"position_id":"1234567890","trade_account_id":0,"microtimestamp":"1712131200000000","alert_type":"liquidation_start","margin_mode":"ISOLATED","initial_margin_ratio":"1.2","maintenance_margin_ratio":"0.9"}}`,
		`{"event":"token_ready","channel":"private-my_token_settlements-123456","data":{"action":"credit","order_id":2046687597465600,"order_id_str":"2046687597465600","instrument":"TSLA/USD","token_security_currency":"TSLA","tokens_quantity":"1.00000000","datetime":"2026-08-25T10:35:09+00:00"}}`,
		`{"event":"in_progress","channel":"announcements","data":{"announcement_id":12345,"announcement_type":"trading_halt","status":"in_progress","published_at":"2026-08-25T10:00:00Z","updated_at":"2026-08-25T10:00:00Z","product_type":"TOKENIZED_SECURITY","product":["TSLA/USD"],"trading_halt_details":{"reason":"circuit_breaker","start_time":"2026-08-25T10:00:00Z","end_time":null}}}`,
	} {
		require.NoErrorf(t, ex.wsHandleData(t.Context(), []byte(msg)), "wsHandleData must not error for %s", msg)
	}
	haltStart := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, []any{
		&WebsocketSettlement{
			Event:                     "settlement",
			PositionID:                "1234567890",
			TransactionID:             "123123",
			Timestamp:                 types.Time(time.Unix(1712131200, 0)),
			Price:                     61200,
			Currency:                  "USD",
			AmountSettled:             -1.5,
			AmountFromPrice:           -1,
			AmountFromTradingFees:     -0.25,
			AmountFromFunding:         -0.25,
			StrikePrice:               61000,
			SettlementType:            "C",
			AmountFromLiquidationFees: 0,
		},
		&WebsocketSettlement{
			Event:         "insurance_fund_premium",
			TransactionID: "123124",
			Timestamp:     types.Time(time.Unix(1712131200, 0)),
			Currency:      "USD",
			Amount:        -0.5,
		},
		&WebsocketLiquidationAlert{
			PositionID:             "1234567890",
			Microtimestamp:         types.Time(time.UnixMicro(1712131200000000)),
			AlertType:              "liquidation_start",
			MarginMode:             MarginModeIsolated,
			InitialMarginRatio:     1.2,
			MaintenanceMarginRatio: 0.9,
		},
		&WebsocketTokenSettlement{
			Action:                "credit",
			OrderID:               2046687597465600,
			Instrument:            "TSLA/USD",
			TokenSecurityCurrency: "TSLA",
			TokensQuantity:        1,
			DateTime:              time.Date(2026, 8, 25, 10, 35, 9, 0, time.FixedZone("", 0)),
		},
		&WebsocketAnnouncement{
			Event:              "in_progress",
			AnnouncementID:     12345,
			AnnouncementType:   "trading_halt",
			Status:             "in_progress",
			PublishedAt:        haltStart,
			UpdatedAt:          haltStart,
			ProductType:        "TOKENIZED_SECURITY",
			Product:            []string{"TSLA/USD"},
			TradingHaltDetails: &TradingHaltDetails{Reason: "circuit_breaker", StartTime: haltStart},
		},
	}, dataHandlerPayloads(ex), "account channel events should be sent to the data handler")
}

func TestSplitChannel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		channel, name, market string
		private               bool
		err                   error
	}{
		{channel: "live_trades_btcusd", name: "live_trades", market: "btcusd"},
		{channel: "order_book_btcusd-perp", name: "order_book", market: "btcusd-perp"},
		{channel: "funding_rate_btcusd-perp", name: "funding_rate", market: "btcusd-perp"},
		{channel: "announcements", name: "announcements"},
		{channel: "private-my_orders_btcusd-314159", name: "my_orders", market: "btcusd", private: true},
		{channel: "private-my_trades_btcusd-perp-314159", name: "my_trades", market: "btcusd-perp", private: true},
		{channel: "private-my_settlements-314159", name: "my_settlements", private: true},
		{channel: "private-my_token_settlements-314159", name: "my_token_settlements", private: true},
		{channel: "", err: errMalformedChannel},
		{channel: "livetrades", err: errMalformedChannel},
		{channel: "live_trades_", err: errMalformedChannel},
		{channel: "_btcusd", err: errMalformedChannel},
		{channel: "private-my_orders_btcusd", err: errMalformedChannel},
		{channel: "private-my_orders_btcusd-perp", err: errMalformedChannel},
		{channel: "private--314159", err: errMalformedChannel},
	} {
		name, market, private, err := splitChannel(tc.channel)
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "splitChannel should error for %q", tc.channel)
			continue
		}
		require.NoErrorf(t, err, "splitChannel must not error for %q", tc.channel)
		assert.Equalf(t, tc.name, name, "name should be correct for %q", tc.channel)
		assert.Equalf(t, tc.market, market, "market should be correct for %q", tc.channel)
		assert.Equalf(t, tc.private, private, "private should be correct for %q", tc.channel)
	}
}

func TestMarketPair(t *testing.T) {
	t.Parallel()
	for market, exp := range map[string]struct {
		pair currency.Pair
		a    asset.Item
	}{
		"btcusd":      {wsSpotPair, asset.Spot},
		"btcusd-perp": {wsPerpetualPair, asset.PerpetualContract},
	} {
		pair, a, err := e.marketPair(market)
		require.NoErrorf(t, err, "marketPair must not error for %s", market)
		assert.Equalf(t, exp.pair, pair, "marketPair should return the pair for %s", market)
		assert.Equalf(t, exp.a, a, "marketPair should return the asset for %s", market)
	}
	_, _, err := e.marketPair("wibble-perp")
	assert.ErrorIs(t, err, errParsingWSPair, "marketPair should error on an unknown market")
}

func TestChannelName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, channelOrderbook, channelName(&subscription.Subscription{Channel: subscription.OrderbookChannel}), "channelName should convert global channel names")
	assert.Equal(t, channelFundingRate, channelName(&subscription.Subscription{Channel: channelFundingRate}), "channelName should return exchange channel names")
	assert.PanicsWithError(t, "subscription channel not supported: wibble", func() { channelName(&subscription.Subscription{Channel: "wibble"}) }, "channelName should panic on an unsupported channel")
	assert.True(t, isAccountChannel(&subscription.Subscription{Channel: channelMySettlements}), "isAccountChannel should be true for account channels")
	assert.False(t, isAccountChannel(&subscription.Subscription{Channel: subscription.MyOrdersChannel}), "isAccountChannel should be false for market channels")
}

func TestGenerateSubscriptions(t *testing.T) {
	t.Parallel()
	ex := newWebsocketTestInstance(t)
	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
	ex.Features.Subscriptions = append(ex.Features.Subscriptions,
		&subscription.Subscription{Enabled: true, Channel: channelAnnouncements},
		&subscription.Subscription{Enabled: true, Channel: channelMySettlements, Authenticated: true},
	)
	subs, err := ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error")

	exp := subscription.List{}
	for _, s := range ex.Features.Subscriptions {
		if isAccountChannel(s) {
			sub := s.Clone()
			sub.QualifiedChannel = channelName(s)
			exp = append(exp, sub)
			continue
		}
		for _, a := range []asset.Item{asset.Spot, asset.PerpetualContract} {
			if s.Asset != asset.All && s.Asset != a {
				continue
			}
			pairs, err := ex.GetEnabledPairs(a)
			require.NoErrorf(t, err, "GetEnabledPairs must not error for %s", a)
			for _, p := range pairs.Format(currency.PairFormat{}) {
				sub := s.Clone()
				sub.Asset = a
				sub.Pairs = currency.Pairs{p}
				sub.QualifiedChannel = channelName(s) + "_" + p.String()
				exp = append(exp, sub)
			}
		}
	}
	testsubs.EqualLists(t, exp, subs)

	ex.Websocket.SetCanUseAuthenticatedEndpoints(false)
	subs, err = ex.generateSubscriptions()
	require.NoError(t, err, "generateSubscriptions must not error without authentication")
	assert.Empty(t, subs.Private(), "generateSubscriptions should not return authenticated subscriptions without authentication")
}

// mockWsSubscriptions records the channels subscribed to by a mock websocket server
type mockWsSubscriptions struct {
	mu       sync.Mutex
	channels map[string]string
}

func (m *mockWsSubscriptions) handle(tb testing.TB, msg []byte, c *gws.Conn) error {
	tb.Helper()
	var req websocketEventRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		return err
	}
	event := "bts:subscription_succeeded"
	m.mu.Lock()
	if req.Event == "bts:unsubscribe" {
		event = "bts:unsubscription_succeeded"
		delete(m.channels, req.Data.Channel)
	} else {
		m.channels[req.Data.Channel] = req.Data.Auth
	}
	m.mu.Unlock()
	resp, err := json.Marshal(map[string]any{"event": event, "channel": req.Data.Channel, "data": struct{}{}})
	if err != nil {
		return err
	}
	return c.WriteMessage(gws.TextMessage, resp)
}

func (m *mockWsSubscriptions) subscribed() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.channels)
}

// newMockWsInstance returns an exchange connected to a mock websocket server which confirms every subscription
// The server's REST endpoint returns websocket tokens when tokenAvailable is set
func newMockWsInstance(t *testing.T, tokenAvailable bool) (*Exchange, *mockWsSubscriptions) {
	t.Helper()
	m := &mockWsSubscriptions{channels: make(map[string]string)}
	wsHandler := mockws.CurryWsMockUpgrader(t, m.handle)
	ex := testexch.MockWsInstance[Exchange](t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			assert.Equal(t, "/v2/websockets_token/", r.URL.Path, "websocket token path should be correct")
			if !tokenAvailable {
				w.WriteHeader(http.StatusForbidden)
				_, err := w.Write([]byte(`{"status":"error","reason":"Invalid signature","code":"API0005"}`))
				assert.NoError(t, err, "Write should not error")
				return
			}
			_, err := w.Write([]byte(`{"token":"Tr6kJCxPiqCcTVWRmNaNBR1EUGPUTjlA","valid_sec":60,"user_id":314159}`))
			assert.NoError(t, err, "Write should not error")
			return
		}
		wsHandler(w, r)
	})
	ex.API.AuthenticatedSupport = true
	ex.SetCredentials(&accounts.Credentials{Key: "key", Secret: "secret"})
	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
	return ex, m
}

func TestSubscribe(t *testing.T) {
	t.Parallel()
	ex, m := newMockWsInstance(t, true)
	subs, err := subscription.List{
		{Enabled: true, Asset: asset.All, Channel: subscription.AllTradesChannel},
		{Enabled: true, Asset: asset.PerpetualContract, Channel: channelFundingRate},
		{Enabled: true, Asset: asset.Spot, Channel: subscription.MyOrdersChannel, Authenticated: true},
		{Enabled: true, Asset: asset.PerpetualContract, Channel: subscription.MyTradesChannel, Authenticated: true},
		{Enabled: true, Channel: channelAnnouncements},
		{Enabled: true, Channel: channelMySettlements, Authenticated: true},
	}.ExpandTemplates(ex)
	require.NoError(t, err, "ExpandTemplates must not error")

	require.NoError(t, ex.Subscribe(subs), "Subscribe must not error")
	for _, s := range subs {
		assert.Equalf(t, subscription.SubscribedState, s.State(), "subscription %s should be subscribed", s)
	}
	const token = "Tr6kJCxPiqCcTVWRmNaNBR1EUGPUTjlA"
	assert.Equal(t, map[string]string{
		"live_trades_btcusd":                   "",
		"live_trades_btceur":                   "",
		"live_trades_eurusd":                   "",
		"live_trades_xrpusd":                   "",
		"live_trades_xrpeur":                   "",
		"live_trades_btcusd-perp":              "",
		"funding_rate_btcusd-perp":             "",
		"private-my_orders_btcusd-314159":      token,
		"private-my_orders_btceur-314159":      token,
		"private-my_orders_eurusd-314159":      token,
		"private-my_orders_xrpusd-314159":      token,
		"private-my_orders_xrpeur-314159":      token,
		"private-my_trades_btcusd-perp-314159": token,
		"announcements":                        "",
		"private-my_settlements-314159":        token,
	}, m.subscribed(), "the server should receive every channel, with private channels suffixed with the user ID and authorised with the token")

	require.NoError(t, ex.Unsubscribe(subs), "Unsubscribe must not error")
	for _, s := range subs {
		assert.Equalf(t, subscription.UnsubscribedState, s.State(), "subscription %s should be unsubscribed", s)
	}
	assert.Empty(t, m.subscribed(), "the server should receive an unsubscription for every channel")
}

func TestSubscribeWithoutWebsocketToken(t *testing.T) {
	t.Parallel()
	ex, m := newMockWsInstance(t, false)
	subs, err := subscription.List{
		{Enabled: true, Asset: asset.PerpetualContract, Channel: channelFundingRate},
		{Enabled: true, Asset: asset.PerpetualContract, Channel: subscription.MyOrdersChannel, Authenticated: true},
	}.ExpandTemplates(ex)
	require.NoError(t, err, "ExpandTemplates must not error")

	err = ex.Subscribe(subs)
	assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "Subscribe should error when a websocket token cannot be fetched")
	assert.ErrorIs(t, err, errAPIResponse, "Subscribe should return the websocket token error")
	assert.Equal(t, map[string]string{"funding_rate_btcusd-perp": ""}, m.subscribed(), "public channels should still be subscribed")
	assert.Equal(t, subscription.SubscribedState, subs[0].State(), "the public subscription should be subscribed")
	assert.NotEqual(t, subscription.SubscribedState, subs[1].State(), "the private subscription should not be subscribed")
}
