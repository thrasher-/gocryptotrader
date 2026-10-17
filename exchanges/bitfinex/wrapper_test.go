package bitfinex

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/deposit"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/portfolio/banking"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

// testWrapperOrderID is the order the wrapper's mock order tests act on, the documented Submit Order example's
const testWrapperOrderID = "1747566428"

// testHistoryStart starts the window the mock history tests ask for, which their fixtures were recorded with
var testHistoryStart = time.UnixMilli(1792022400000).UTC()

// newWrapperTestExchange returns an exchange named after the test, trading BTCUSD and ETHUSD on spot, BTCUSD on margin
// and funding USD
func newWrapperTestExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := newTestExchange(t)
	for a, pairs := range map[asset.Item]currency.Pairs{
		asset.Spot:          {currency.NewBTCUSD(), testETHUSD},
		asset.Margin:        {currency.NewBTCUSD()},
		asset.MarginFunding: {testFundingUSD},
	} {
		require.NoErrorf(t, ex.CurrencyPairs.StorePairs(a, pairs, false), "StorePairs must not error for %s available pairs", a)
		require.NoErrorf(t, ex.CurrencyPairs.StorePairs(a, pairs, true), "StorePairs must not error for %s enabled pairs", a)
	}
	return ex
}

// historyWindow returns the window a history test asks for: the mock's recorded one, or the last hour live
func historyWindow(d time.Duration) (start, end time.Time) {
	if mockTests {
		return testHistoryStart, testHistoryStart.Add(d)
	}
	end = time.Now().UTC().Truncate(time.Minute)
	return end.Add(-d), end
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTradablePairs(t.Context(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "FetchTradablePairs must refuse an asset Bitfinex does not trade")
	for a, exp := range map[asset.Item]currency.Pairs{
		asset.Spot: {
			currency.NewPair(currency.AAVE, currency.USD), currency.NewPair(currency.AAVE, currency.UST), currency.NewPair(currency.ADA, currency.BTC),
			currency.NewPair(currency.ADA, currency.USD), currency.NewPair(currency.ADA, currency.UST),
		},
		asset.Margin: {
			currency.NewPair(currency.ADA, currency.BTC), currency.NewPair(currency.ADA, currency.USD), currency.NewPair(currency.ADA, currency.UST),
			currency.NewPair(currency.NewCode("ALG"), currency.USD), currency.NewPair(currency.APT, currency.USD),
		},
		asset.MarginFunding: {testFundingUSD, {Base: currency.BTC}},
	} {
		pairs, err := e.FetchTradablePairs(t.Context(), a)
		require.NoErrorf(t, err, "FetchTradablePairs must not error for %s", a)
		require.NotEmptyf(t, pairs, "FetchTradablePairs must return %s pairs", a)
		for _, p := range pairs {
			assert.NotContainsf(t, p.String(), testListingMarker, "FetchTradablePairs should leave out %s test pairs", a)
		}
		if mockTests {
			assert.Truef(t, exp.Equal(pairs), "FetchTradablePairs should return the %s pairs Bitfinex lists, got %s", a, pairs)
			continue
		}
		want := currency.NewBTCUSD()
		if a == asset.MarginFunding {
			want = testFundingUSD
		}
		assert.Truef(t, pairs.Contains(want, true), "FetchTradablePairs should return %s for %s", want, a)
	}
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	require.NoError(t, ex.UpdateTradablePairs(t.Context()), "UpdateTradablePairs must not error")
	for _, a := range ex.GetAssetTypes(false) {
		available, err := ex.GetAvailablePairs(a)
		require.NoErrorf(t, err, "GetAvailablePairs must not error for %s", a)
		assert.NotEmptyf(t, available, "UpdateTradablePairs should store %s's available pairs", a)
	}
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	assert.ErrorIs(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.MarginFunding), common.ErrNotYetImplemented, "UpdateOrderExecutionLimits should not load funding limits")
	assert.ErrorIs(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.Options), asset.ErrNotSupported, "UpdateOrderExecutionLimits should refuse an asset Bitfinex does not trade")
	p := currency.NewBTCUSD()
	if mockTests {
		p = currency.NewPair(currency.ADA, currency.USD)
	}
	for _, a := range []asset.Item{asset.Spot, asset.Margin} {
		require.NoErrorf(t, ex.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must not error for %s", a)
		l, err := limits.GetOrderExecutionLimits(key.NewExchangeAssetPair(ex.Name, a, p))
		require.NoErrorf(t, err, "GetOrderExecutionLimits must not error for %s %s", a, p)
		if mockTests {
			assert.Equalf(t, 4.0, l.MinimumBaseAmount, "the %s minimum order should be Bitfinex's", a)
			assert.Equalf(t, 250000.0, l.MaximumBaseAmount, "the %s maximum order should be Bitfinex's", a)
			continue
		}
		assert.Positivef(t, l.MinimumBaseAmount, "the %s minimum order should be set", a)
		assert.Greaterf(t, l.MaximumBaseAmount, l.MinimumBaseAmount, "the %s maximum order should exceed the minimum", a)
	}
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	require.ErrorIs(t, ex.UpdateTickers(t.Context(), asset.Options), asset.ErrNotSupported, "UpdateTickers must refuse an asset Bitfinex does not trade")
	for a, pairs := range map[asset.Item]currency.Pairs{
		asset.Spot:          {currency.NewBTCUSD(), testETHUSD},
		asset.Margin:        {currency.NewBTCUSD()},
		asset.MarginFunding: {testFundingUSD},
	} {
		require.NoErrorf(t, ex.UpdateTickers(t.Context(), a), "UpdateTickers must not error for %s", a)
		for _, p := range pairs {
			got, err := ticker.GetTicker(ex.Name, p, a)
			require.NoErrorf(t, err, "UpdateTickers must store %s %s's ticker", a, p)
			assert.Positivef(t, got.Last, "the %s %s ticker should have a last price", a, p)
			assert.WithinDurationf(t, time.Now(), got.LastUpdated, time.Minute, "the %s %s ticker should be stamped when received, not with its first trade", a, p)
		}
	}
	if mockTests {
		got, err := ticker.GetTicker(ex.Name, currency.NewBTCUSD(), asset.Spot)
		require.NoError(t, err, "GetTicker must not error")
		got.LastUpdated = time.Time{}
		dailyChangeRelative := 0.01367597
		assert.Equal(t, ticker.Price{
			Last: 82126, High: 82641, Low: 79821, Bid: 82109, BidSize: 1.49200629, Ask: 82121, AskSize: 1.90605581, BaseVolume: 505.79793186,
			Open24Hour: 82126 - 1108, PercentChange24Hour: dailyChangeRelative * 100, Pair: currency.NewBTCUSD(), ExchangeName: ex.Name, AssetType: asset.Spot,
		}, *got, "the stored spot ticker should be Bitfinex's, its open the last price less the daily change")
	}
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	_, err := ex.UpdateTicker(t.Context(), currency.EMPTYPAIR, asset.Spot)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "UpdateTicker must refuse an empty pair")
	_, err = ex.UpdateTicker(t.Context(), currency.NewBTCUSD(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateTicker must refuse an asset Bitfinex has no symbol for")
	for a, p := range map[asset.Item]currency.Pair{asset.Spot: currency.NewBTCUSD(), asset.MarginFunding: testFundingUSD} {
		got, err := ex.UpdateTicker(t.Context(), p, a)
		require.NoErrorf(t, err, "UpdateTicker must not error for %s", a)
		assert.Positivef(t, got.Last, "the %s ticker should have a last price", a)
		assert.Equalf(t, a, got.AssetType, "the %s ticker should be stored for its asset", a)
	}
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	_, err := ex.UpdateOrderbook(t.Context(), currency.EMPTYPAIR, asset.Spot)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "UpdateOrderbook must refuse an empty pair")
	for a, p := range map[asset.Item]currency.Pair{asset.Spot: currency.NewBTCUSD(), asset.MarginFunding: testFundingUSD} {
		book, err := ex.UpdateOrderbook(t.Context(), p, a)
		require.NoErrorf(t, err, "UpdateOrderbook must not error for %s", a)
		require.NotEmptyf(t, book.Bids, "the %s book must hold bids", a)
		require.NotEmptyf(t, book.Asks, "the %s book must hold asks", a)
		assert.Equalf(t, a == asset.MarginFunding, book.IsFundingRate, "the %s book should be marked a funding book only for funding", a)
		assert.NotZerof(t, book.Bids[0].ID, "the %s book should hold orders by ID", a)
		if a == asset.MarginFunding {
			assert.NotZero(t, book.Bids[0].Period, "the funding book should hold each offer's period")
		}
	}
}

func TestUpdateAccountBalances(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateAccountBalances(t.Context(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateAccountBalances must refuse an asset Bitfinex has no wallet for")
	skipLiveWithoutCredentials(t)
	ex := newWrapperTestExchange(t)
	for _, a := range []asset.Item{asset.Spot, asset.Margin, asset.MarginFunding} {
		subAccts, err := ex.UpdateAccountBalances(t.Context(), a)
		require.NoErrorf(t, err, "UpdateAccountBalances must not error for %s", a)
		require.Lenf(t, subAccts, 1, "UpdateAccountBalances must return %s's wallet", a)
		assert.Equalf(t, a, subAccts[0].AssetType, "UpdateAccountBalances should return the %s wallet", a)
		if !mockTests {
			continue
		}
		switch a {
		case asset.Spot:
			assert.Equal(t, accounts.CurrencyBalances{currency.UST: {Currency: currency.UST, Total: 19788.6529257, Free: 19788.6529257, UpdatedAt: subAccts[0].Balances[currency.UST].UpdatedAt}}, subAccts[0].Balances, "the exchange wallet should hold its balances")
		case asset.Margin:
			assert.Empty(t, subAccts[0].Balances, "the margin wallet should hold nothing")
		case asset.MarginFunding:
			b := subAccts[0].Balances[currency.BTC]
			assert.Equal(t, 0.5, b.Total, "the funding wallet should hold its balance")
			assert.Zero(t, b.Free, "a balance whose available amount Bitfinex has not calculated should not be free")
			assert.Equal(t, 0.5, b.Hold, "a balance whose available amount Bitfinex has not calculated should be held")
		}
	}
}

func TestValidateAPICredentials(t *testing.T) {
	t.Parallel()
	skipLiveWithoutCredentials(t)
	assert.NoError(t, newWrapperTestExchange(t).ValidateAPICredentials(t.Context(), asset.Spot), "ValidateAPICredentials should not error")
}

func TestGetAccountFundingHistory(t *testing.T) {
	t.Parallel()
	skipLiveWithoutCredentials(t)
	history, err := e.GetAccountFundingHistory(t.Context())
	require.NoError(t, err, "GetAccountFundingHistory must not error")
	if !mockTests {
		return
	}
	require.Len(t, history, 2, "GetAccountFundingHistory must return every movement")
	assert.Equal(t, exchange.FundingHistory{
		ExchangeName: e.Name, Status: "COMPLETED", TransferID: "13105603", Timestamp: time.UnixMilli(1569348774000), Currency: "ETH", Amount: 0.26300954,
		Fee: 0.00135, TransferType: "withdrawal", CryptoToAddress: "DESTINATION_ADDRESS", CryptoTxID: "TRANSACTION_ID", CryptoChain: "ethereum",
	}, history[0], "a withdrawal should be reported with its positive amount and fee, and the method it moved over")
	assert.Equal(t, "deposit", history[1].TransferType, "a positive movement should be a deposit")
}

func TestGetWithdrawalsHistory(t *testing.T) {
	t.Parallel()
	skipLiveWithoutCredentials(t)
	for _, c := range []currency.Code{currency.ETH, currency.EMPTYCODE} {
		history, err := e.GetWithdrawalsHistory(t.Context(), c, asset.Spot)
		require.NoErrorf(t, err, "GetWithdrawalsHistory must not error for currency %q", c)
		for i := range history {
			assert.Equal(t, "withdrawal", history[i].TransferType, "GetWithdrawalsHistory should return withdrawals alone")
		}
		if mockTests {
			require.Lenf(t, history, 1, "GetWithdrawalsHistory must leave out the deposit for currency %q", c)
			assert.Equal(t, 0.26300954, history[0].Amount, "a withdrawal's amount should be positive")
		}
	}
}

func TestPageBack(t *testing.T) {
	t.Parallel()
	type record struct {
		id uint64
		at time.Time
	}
	ms := time.UnixMilli
	id := func(r *record) uint64 { return r.id }
	at := func(r *record) time.Time { return r.at }
	// serve returns a fetch serving history as Bitfinex does, newest first, records up to the end and from the start
	serve := func(history []record, start time.Time, limit int, ends *[]time.Time) func(time.Time) ([]record, error) {
		return func(end time.Time) ([]record, error) {
			*ends = append(*ends, end)
			var page []record
			for _, r := range history {
				if (end.IsZero() || !r.at.After(end)) && !r.at.Before(start) && len(page) < limit {
					page = append(page, r)
				}
			}
			return page, nil
		}
	}
	history := []record{{9, ms(50)}, {8, ms(40)}, {7, ms(40)}, {6, ms(30)}, {5, ms(30)}, {4, ms(20)}, {3, ms(10)}}
	var ends []time.Time
	got, err := pageBack(time.Time{}, time.Time{}, 3, serve(history, time.Time{}, 3, &ends), id, at)
	require.NoError(t, err, "pageBack must not error")
	assert.Equal(t, history, got, "pageBack should return each record once, newest first")
	assert.Equal(t, []time.Time{{}, ms(40), ms(30), ms(20)}, ends, "each page should end at its predecessor's oldest record")

	ends = nil
	got, err = pageBack(ms(30), time.Time{}, 3, serve(history, ms(30), 3, &ends), id, at)
	require.NoError(t, err, "pageBack must not error")
	assert.Equal(t, history[:5], got, "pageBack should return the records from the start")
	assert.Equal(t, []time.Time{{}, ms(40), ms(30)}, ends, "pageBack should page no further than the start")

	ends = nil
	got, err = pageBack(ms(35), time.Time{}, 3, serve(history, time.Time{}, 3, &ends), id, at)
	require.NoError(t, err, "pageBack must not error")
	assert.Equal(t, history[:4], got, "pageBack should stop at a page reaching before the start")
	assert.Equal(t, []time.Time{{}, ms(40)}, ends, "pageBack should not fetch past a page reaching before the start")

	crowded := []record{{4, ms(20)}, {3, ms(20)}, {2, ms(20)}, {1, ms(20)}, {0, ms(10)}}
	_, err = pageBack(time.Time{}, time.Time{}, 3, serve(crowded, time.Time{}, 3, &ends), id, at)
	assert.ErrorIs(t, err, errHistoryPageFull, "pageBack should error for a millisecond holding a page of records")

	var calls int
	ignoringEnd := func(time.Time) ([]record, error) {
		calls++
		return history[:3], nil
	}
	_, err = pageBack(time.Time{}, time.Time{}, 3, ignoringEnd, id, at)
	assert.ErrorIs(t, err, errHistoryPageFull, "pageBack should error for a server ignoring the end")
	assert.Equal(t, 2, calls, "pageBack should stop paging a server ignoring the end at its second page")

	errPageRefused := errors.New("page refused")
	_, err = pageBack(time.Time{}, time.Time{}, 3, func(time.Time) ([]record, error) { return nil, errPageRefused }, id, at)
	assert.ErrorIs(t, err, errPageRefused, "pageBack should return a page's error")
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	for a, p := range map[asset.Item]currency.Pair{asset.Spot: currency.NewBTCUSD(), asset.MarginFunding: testFundingUSD} {
		trades, err := ex.GetRecentTrades(t.Context(), p, a)
		require.NoErrorf(t, err, "GetRecentTrades must not error for %s", a)
		require.NotEmptyf(t, trades, "GetRecentTrades must return %s trades", a)
		assert.Falsef(t, trades[0].Timestamp.After(trades[len(trades)-1].Timestamp), "GetRecentTrades should return %s trades oldest first", a)
		for i := range trades {
			assert.Positivef(t, trades[i].Amount, "a %s trade's amount should be positive, its side giving its sign", a)
		}
	}
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	start, end := historyWindow(time.Hour)
	_, err := ex.GetHistoricTrades(t.Context(), currency.NewBTCUSD(), asset.Spot, end, start)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetHistoricTrades must refuse a window ending before it starts")
	trades, err := ex.GetHistoricTrades(t.Context(), currency.NewBTCUSD(), asset.Spot, start, end)
	require.NoError(t, err, "GetHistoricTrades must not error")
	require.NotEmpty(t, trades, "GetHistoricTrades must return trades")
	for i := range trades {
		assert.Falsef(t, trades[i].Timestamp.Before(start) || trades[i].Timestamp.After(end), "trade %s should be within the window", trades[i].TID)
	}
}

func TestPlaceOrderRequest(t *testing.T) {
	t.Parallel()
	until := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		s    *order.Submit
		exp  *PlaceOrderRequest
		err  error
	}{
		{
			name: "spot limit buy",
			s:    &order.Submit{AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.Limit, Side: order.Buy, Amount: 1, Price: 100, ClientOrderID: "12"},
			exp:  &PlaceOrderRequest{Type: "EXCHANGE LIMIT", Symbol: "tBTCUSD", Amount: 1, Price: 100, ClientOrderID: 12},
		},
		{
			name: "margin limit sell, post only and reduce only",
			s:    &order.Submit{AssetType: asset.Margin, Pair: currency.NewBTCUSD(), Type: order.Limit, Side: order.Sell, Amount: 1, Price: 100, TimeInForce: order.PostOnly, ReduceOnly: true},
			exp:  &PlaceOrderRequest{Type: "LIMIT", Symbol: "tBTCUSD", Amount: -1, Price: 100, Flags: OrderFlagPostOnly | OrderFlagReduceOnly},
		},
		{
			name: "fill or kill",
			s:    &order.Submit{AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.Limit, Side: order.Buy, Amount: 1, Price: 100, TimeInForce: order.FillOrKill},
			exp:  &PlaceOrderRequest{Type: "EXCHANGE FOK", Symbol: "tBTCUSD", Amount: 1, Price: 100},
		},
		{
			name: "immediate or cancel",
			s:    &order.Submit{AssetType: asset.Margin, Pair: currency.NewBTCUSD(), Type: order.Limit, Side: order.Short, Amount: 1, Price: 100, TimeInForce: order.ImmediateOrCancel},
			exp:  &PlaceOrderRequest{Type: "IOC", Symbol: "tBTCUSD", Amount: -1, Price: 100},
		},
		{
			name: "good till time",
			s:    &order.Submit{AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.Limit, Side: order.Buy, Amount: 1, Price: 100, TimeInForce: order.GoodTillTime, EndTime: until},
			exp:  &PlaceOrderRequest{Type: "EXCHANGE LIMIT", Symbol: "tBTCUSD", Amount: 1, Price: 100, TimeInForce: until},
		},
		{
			name: "market",
			s:    &order.Submit{AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.Market, Side: order.Sell, Amount: 1, Price: 100},
			exp:  &PlaceOrderRequest{Type: "EXCHANGE MARKET", Symbol: "tBTCUSD", Amount: -1},
		},
		{
			name: "stop",
			s:    &order.Submit{AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.Stop, Side: order.Sell, Amount: 1, TriggerPrice: 90},
			exp:  &PlaceOrderRequest{Type: "EXCHANGE STOP", Symbol: "tBTCUSD", Amount: -1, Price: 90},
		},
		{
			name: "stop limit",
			s:    &order.Submit{AssetType: asset.Margin, Pair: currency.NewPair(currency.AVAX, currency.USD), Type: order.StopLimit, Side: order.Buy, Amount: 1, Price: 105, TriggerPrice: 110},
			exp:  &PlaceOrderRequest{Type: "STOP LIMIT", Symbol: "tAVAX:USD", Amount: 1, Price: 110, AuxiliaryLimitPrice: 105},
		},
		{
			name: "trailing stop",
			s:    &order.Submit{AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.TrailingStop, Side: order.Sell, Amount: 1, TrackingValue: 50},
			exp:  &PlaceOrderRequest{Type: "EXCHANGE TRAILING STOP", Symbol: "tBTCUSD", Amount: -1, TrailingPrice: 50},
		},
		{name: "funding", s: &order.Submit{AssetType: asset.MarginFunding, Pair: testFundingUSD, Type: order.Limit}, err: asset.ErrNotSupported},
		{name: "unsupported type", s: &order.Submit{AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.OCO}, err: errUnsupportedOrderType},
		{name: "client order ID", s: &order.Submit{AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.Limit, ClientOrderID: "abc"}, err: errInvalidClientOrderID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := placeOrderRequest(tc.s)
			require.ErrorIs(t, err, tc.err, "placeOrderRequest must return the expected error")
			assert.Equal(t, tc.exp, got, "placeOrderRequest should map the order to Bitfinex's request")
		})
	}
}

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	_, err := e.SubmitOrder(t.Context(), &order.Submit{Exchange: e.Name, AssetType: asset.MarginFunding, Pair: testFundingUSD, Type: order.Limit, Side: order.Buy, Amount: 1, Price: 1})
	require.ErrorIs(t, err, asset.ErrNotSupported, "SubmitOrder must refuse funding orders")
	skipLiveUnlessCanManipulate(t)
	ex := newWrapperTestExchange(t)
	s := &order.Submit{Exchange: ex.Name, AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.Limit, Side: order.Buy, Amount: 0.1, Price: 10000, ClientOrderID: "1678987199446"}
	if !mockTests {
		req := liveTestOrderRequest(t)
		s.Amount, s.Price, s.ClientOrderID = req.Amount, req.Price, ""
	}
	resp, err := ex.SubmitOrder(t.Context(), s)
	require.NoError(t, err, "SubmitOrder must not error")
	if !mockTests {
		id, err := parseOrderID(resp.OrderID)
		require.NoError(t, err, "SubmitOrder must return the order's ID")
		cancelLiveTestOrderOnCleanup(t, id)
	} else {
		assert.Equal(t, testWrapperOrderID, resp.OrderID, "SubmitOrder should return the order's ID")
	}
	assert.Equal(t, order.Active, resp.Status, "SubmitOrder should return the order's status")
}

func TestSetDefaultsTransferChains(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	ex.SetDefaults()
	f := ex.Features.Supports.RESTCapabilities
	assert.True(t, f.MultiChainDeposits, "deposits should be supported over each of a currency's methods")
	assert.True(t, f.MultiChainWithdrawals, "withdrawals should be supported over each of a currency's methods")
	assert.True(t, f.MultiChainDepositRequiresChainSet, "a deposit address should need its method, as a currency with several has no default")
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	_, err := e.ModifyOrder(t.Context(), &order.Modify{OrderID: "x", AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Amount: 1})
	require.ErrorIs(t, err, errOrderIDRequired, "ModifyOrder must need a numeric order ID")
	_, err = e.ModifyOrder(t.Context(), &order.Modify{OrderID: testWrapperOrderID, AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Amount: 1})
	require.ErrorIs(t, err, order.ErrSideIsInvalid, "ModifyOrder must need the side of an order whose amount it changes")
	skipLiveUnlessCanManipulate(t)
	ex := newWrapperTestExchange(t)
	m := &order.Modify{OrderID: testWrapperOrderID, AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Side: order.Buy, Amount: 0.25, Price: 25000}
	if !mockTests {
		placed := placeLiveTestOrder(t)
		m.OrderID, m.Amount, m.Price = strconv.FormatUint(placed.ID, 10), placed.Amount, placed.Price-1000
	}
	resp, err := ex.ModifyOrder(t.Context(), m)
	require.NoError(t, err, "ModifyOrder must not error")
	assert.Equal(t, m.OrderID, resp.OrderID, "ModifyOrder should return the order modified")
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.CancelOrder(t.Context(), &order.Cancel{OrderID: "x", AssetType: asset.Spot}), errOrderIDRequired, "CancelOrder must need a numeric order ID")
	skipLiveUnlessCanManipulate(t)
	ex := newWrapperTestExchange(t)
	id := testWrapperOrderID
	if !mockTests {
		id = strconv.FormatUint(placeLiveTestOrder(t).ID, 10)
	}
	assert.NoError(t, ex.CancelOrder(t.Context(), &order.Cancel{OrderID: id, AssetType: asset.Spot, Pair: currency.NewBTCUSD()}), "CancelOrder should not error")
}

func TestCancelBatchOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelBatchOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrCancelOrderIsNil, "CancelBatchOrders must need orders")
	_, err = e.CancelBatchOrders(t.Context(), []order.Cancel{{OrderID: "x"}})
	require.ErrorIs(t, err, errOrderIDRequired, "CancelBatchOrders must need numeric order IDs")
	skipLiveUnlessCanManipulate(t)
	ex := newWrapperTestExchange(t)
	ids := []string{testWrapperOrderID, "1747566429"}
	if !mockTests {
		ids = []string{strconv.FormatUint(placeLiveTestOrder(t).ID, 10)}
	}
	cancels := make([]order.Cancel, len(ids))
	for i := range ids {
		cancels[i] = order.Cancel{OrderID: ids[i], AssetType: asset.Spot, Pair: currency.NewBTCUSD()}
	}
	resp, err := ex.CancelBatchOrders(t.Context(), cancels)
	require.NoError(t, err, "CancelBatchOrders must not error")
	assert.Equal(t, order.Cancelled.String(), resp.Status[ids[0]], "an order Bitfinex accepted for cancellation should be cancelled")
	if mockTests {
		assert.Equal(t, order.UnknownStatus.String(), resp.Status["1747566429"], "an order Bitfinex did not report should be unknown")
	}
}

func TestCancelAllOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllOrders(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CancelAllOrders must reject a nil request")
	skipLiveUnlessCanManipulate(t)
	ex := newWrapperTestExchange(t)
	if !mockTests {
		placeLiveTestOrder(t)
	}
	resp, err := ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Spot, Pair: currency.NewBTCUSD()})
	require.NoError(t, err, "CancelAllOrders must not error")
	require.NotEmpty(t, resp.Status, "CancelAllOrders must cancel the open spot order")
	for id, status := range resp.Status {
		assert.Equalf(t, order.Cancelled.String(), status, "CancelAllOrders should cancel order %s", id)
	}
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderInfo(t.Context(), "x", currency.EMPTYPAIR, asset.Empty)
	require.ErrorIs(t, err, errOrderIDRequired, "GetOrderInfo must need a numeric order ID")
	skipLiveWithoutCredentials(t)
	id, p := "33950998275", currency.NewPair(currency.ETH, currency.USD)
	if !mockTests {
		orders, err := e.GetOrdersHistory(t.Context(), &OrdersHistoryRequest{Limit: 1})
		require.NoError(t, err, "GetOrdersHistory must not error")
		if len(orders) == 0 {
			t.Skip("The account has no recent orders whose information could be returned")
		}
		id = strconv.FormatUint(orders[0].ID, 10)
		p, err = pairFromSymbol(orders[0].Symbol)
		require.NoError(t, err, "the order's symbol must be a pair")
	}
	got, err := e.GetOrderInfo(t.Context(), id, p, asset.Empty)
	require.NoError(t, err, "GetOrderInfo must not error")
	assert.Equal(t, id, got.OrderID, "GetOrderInfo should return the order")
	if mockTests {
		assert.Equal(t, asset.Margin, got.AssetType, "the documented LIMIT order should be margin")
		require.Len(t, got.Trades, 1, "GetOrderInfo must return the order's trades")
		assert.InDelta(t, 0.09868591, got.Fee, 1e-12, "GetOrderInfo should total the order's trading fees as positive")
		_, err = e.GetOrderInfo(t.Context(), id, currency.NewBTCUSD(), asset.Empty)
		assert.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo should refuse an order on another pair")
		_, err = e.GetOrderInfo(t.Context(), "1", currency.EMPTYPAIR, asset.Empty)
		assert.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo should report an order neither open nor in the history")
	}
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	skipLiveWithoutCredentials(t)
	for _, a := range []asset.Item{asset.Spot, asset.Margin} {
		orders, err := e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{AssetType: a, Side: order.AnySide, Type: order.AnyType})
		require.NoErrorf(t, err, "GetActiveOrders must not error for %s", a)
		for i := range orders {
			assert.Equalf(t, a, orders[i].AssetType, "GetActiveOrders should return %s orders alone", a)
		}
		if mockTests && a == asset.Margin {
			require.Len(t, orders, 1, "GetActiveOrders must return the open margin order")
			assert.Equal(t, "33950998275", orders[0].OrderID, "GetActiveOrders should return the open order")
		}
	}
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	skipLiveWithoutCredentials(t)
	for _, pairs := range []currency.Pairs{nil, {currency.NewBTCUSD()}} {
		orders, err := e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{AssetType: asset.Spot, Pairs: pairs, Side: order.AnySide, Type: order.AnyType})
		require.NoErrorf(t, err, "GetOrderHistory must not error for pairs %s", pairs)
		for i := range orders {
			assert.Equal(t, asset.Spot, orders[i].AssetType, "GetOrderHistory should return spot orders alone")
		}
	}
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	_, err := e.GetDepositAddress(t.Context(), currency.EMPTYCODE, "", "")
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "GetDepositAddress must need a currency")
	skipLiveWithoutCredentials(t)
	got, err := e.GetDepositAddress(t.Context(), currency.BTC, WalletTypeExchange, "")
	require.NoError(t, err, "GetDepositAddress must not error")
	assert.NotEmpty(t, got.Address, "GetDepositAddress should return an address")
	assert.Equal(t, "bitcoin", got.Chain, "GetDepositAddress should return the method the address deposits over")
}

func TestGetDepositAddressTagsAndChains(t *testing.T) {
	t.Parallel()
	var deposits []string
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch r.URL.Path {
		case "/v2/conf/pub:map:tx:method":
			body = `[[["BITCOIN",["BTC"]],["RIPPLE",["XRP"]],["TETHERUSE",["UST"]],["TETHERUSX",["UST"]]]]`
		case "/v2/auth/w/deposit/address":
			raw, err := io.ReadAll(r.Body)
			assert.NoError(t, err, "Reading the request should not error")
			deposits = append(deposits, string(raw))
			body = `[1568738594687,"acc_dep",null,null,[null,"RIPPLE","XRP",null,"2011546049","rLW9gnQo7BQhU6igk5keqYnH3TVrCxGRzm"],null,"SUCCESS","success"]`
		default:
			assert.Failf(t, "unexpected request", "%s %s", r.Method, r.URL.Path)
		}
		_, err := w.Write([]byte(body))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	got, err := ex.GetDepositAddress(t.Context(), currency.XRP, "", "")
	require.NoError(t, err, "GetDepositAddress must not error for a currency with a tag")
	assert.Equal(t, &deposit.Address{Address: "rLW9gnQo7BQhU6igk5keqYnH3TVrCxGRzm", Tag: "2011546049", Chain: "ripple"}, got, "a tagged currency's address should be its pool address, with its tag")
	assert.Equal(t, []string{`{"wallet":"funding","method":"ripple"}`}, deposits, "GetDepositAddress should deposit into the funding wallet by default")

	_, err = ex.GetDepositAddress(t.Context(), currency.USDT, "", "")
	assert.ErrorIs(t, err, errTransferMethodNeeded, "GetDepositAddress should need a chain for a currency with several")
	chains, err := ex.GetAvailableTransferChains(t.Context(), currency.USDT)
	require.NoError(t, err, "GetAvailableTransferChains must not error for USDT")
	assert.Equal(t, []string{"tetheruse", "tetherusx"}, chains, "USDT, which Bitfinex names UST, should have both its chains")
	_, err = ex.GetDepositAddress(t.Context(), currency.BTC, "", "dogecoin")
	assert.ErrorIs(t, err, errTransferMethodUnknown, "GetDepositAddress should refuse a chain the currency has not")
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	_, err := e.GetAvailableTransferChains(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "GetAvailableTransferChains must need a currency")
	chains, err := e.GetAvailableTransferChains(t.Context(), currency.BTC)
	require.NoError(t, err, "GetAvailableTransferChains must not error")
	assert.Contains(t, chains, "bitcoin", "BTC should be transferred over bitcoin")
	_, err = e.GetAvailableTransferChains(t.Context(), currency.NewCode("NOPE"))
	assert.ErrorIs(t, err, errTransferMethodUnknown, "GetAvailableTransferChains should report a currency without a method")
}

func TestWithdrawCryptocurrencyFunds(t *testing.T) {
	t.Parallel()
	if !mockTests {
		t.Skip("A withdrawal cannot be undone")
	}
	resp, err := e.WithdrawCryptocurrencyFunds(t.Context(), &withdraw.Request{
		Exchange: e.Name, Currency: currency.ETH, Amount: 0.01, Description: "test", Type: withdraw.Crypto,
		Crypto: withdraw.CryptoRequest{Address: "0x6C20eBD2222CE0F80971A5D2C4Da29e70a0B9b23"},
	})
	require.NoError(t, err, "WithdrawCryptocurrencyFunds must not error")
	assert.Equal(t, &withdraw.ExchangeResponse{ID: "13080092", Status: NotificationStatusSuccess}, resp, "WithdrawCryptocurrencyFunds should return the withdrawal's ID")
}

func TestWithdrawCryptocurrencyFundsTravelRule(t *testing.T) {
	t.Parallel()
	var sent []byte
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `[[["ETHEREUM",["ETH"]]]]`
		if r.URL.Path == "/v2/auth/w/withdraw" {
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err, "Reading the withdrawal's body should not error")
			sent = body
			response = `[1568742390999,"acc_wd-req",null,null,[13080092,null,"ethereum",null,"exchange",0.01,null,null,0.00135],null,"SUCCESS","Your withdrawal request has been successfully submitted."]`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	w := &withdraw.Request{
		Exchange: ex.Name, Currency: currency.ETH, Amount: 0.01, Description: "test", Type: withdraw.Crypto,
		Crypto: withdraw.CryptoRequest{Address: "0x6C20eBD2222CE0F80971A5D2C4Da29e70a0B9b23"},
		Travel: withdraw.TravelRule{BeneficiaryName: "Mary Ann Smith", BeneficiaryFinancialInstitution: "Strawb Exchange Ltd"},
	}
	_, err := ex.WithdrawCryptocurrencyFunds(t.Context(), w)
	require.NoError(t, err, "WithdrawCryptocurrencyFunds must not error")
	assert.JSONEq(t, `{"wallet":"exchange","method":"ethereum","amount":"0.01","address":"0x6C20eBD2222CE0F80971A5D2C4Da29e70a0B9b23","note":"test","travel_rule_tos":true,"vasp_name":"Strawb Exchange Ltd","dest_firstname":"Mary Ann","dest_lastname":"Smith"}`, string(sent), "a withdrawal should send its beneficiary's names and provider")
}

func TestWithdrawalTravelRule(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		travel withdraw.TravelRule
		exp    *TravelRuleDetails
		err    error
	}{
		{name: "none"},
		{name: "self", travel: withdraw.TravelRule{IsSelf: true, BeneficiaryName: "John Doe"}, exp: &TravelRuleDetails{BeneficiarySelf: true}},
		{name: "provider", travel: withdraw.TravelRule{BeneficiaryFinancialInstitution: "Strawb Exchange Ltd"}, exp: &TravelRuleDetails{VASPName: "Strawb Exchange Ltd"}},
		{name: "person", travel: withdraw.TravelRule{BeneficiaryName: "  John   Doe "}, exp: &TravelRuleDetails{FirstName: "John", LastName: "Doe"}},
		{name: "one name", travel: withdraw.TravelRule{BeneficiaryName: "Satoshi"}, err: errTravelRuleNameIncomplete},
	} {
		got, err := withdrawalTravelRule(&tc.travel)
		require.ErrorIsf(t, err, tc.err, "withdrawalTravelRule must return the expected error for %s", tc.name)
		assert.Equalf(t, tc.exp, got, "withdrawalTravelRule should return the travel rule details of %s", tc.name)
	}
}

func TestRequestFiatWithdrawalPayload(t *testing.T) {
	t.Parallel()
	var payload map[string]any
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoded, err := base64.StdEncoding.DecodeString(r.Header.Get("X-BFX-PAYLOAD"))
		assert.NoError(t, err, "a v1 request's payload should be base64 encoded")
		payload = nil
		assert.NoError(t, json.Unmarshal(decoded, &payload), "a v1 request's payload should be JSON")
		_, err = w.Write([]byte(`[{"status":"success","message":"Your withdrawal request has been successfully submitted.","withdrawal_id":586829}]`))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	newWithdrawal := func() *withdraw.Request {
		return &withdraw.Request{
			Exchange: ex.Name, Currency: currency.USD, Amount: 1000, Type: withdraw.Fiat,
			Fiat: withdraw.FiatRequest{
				Bank: banking.Account{
					Enabled: true, AccountName: "Satoshi Nakamoto", AccountNumber: "12345", BankName: "Federal Reserve Bank", BankAddress: "33 Liberty Street",
					BankPostalCity: "New York", BankCountry: "United States", SWIFTCode: "FRNYUS33", SupportedCurrencies: "USD", SupportedExchanges: ex.Name,
				},
				IsExpressWire:            true,
				RequiresIntermediaryBank: true, IntermediaryBankName: "Intermediary Bank", IntermediaryBankAccountNumber: 123456789,
			},
		}
	}
	w := newWithdrawal()
	_, err := ex.RequestFiatWithdrawal(t.Context(), &FiatWithdrawalRequest{Wallet: WalletTypeExchange, Withdrawal: w})
	require.NoError(t, err, "RequestFiatWithdrawal must not error")
	assert.NotContains(t, payload, "account_address", "a wire should send no account holder's address, which GCT does not hold")
	assert.Equal(t, "33 Liberty Street", payload["bank_address"], "a wire should send the bank's address")
	assert.Equal(t, 1.0, payload["expressWire"], "an express wire should be flagged 1, as the docs ask")
	assert.Equal(t, "123456789", payload["intermediary_bank_account"], "a wire should send its intermediary bank's account number exactly")

	w = newWithdrawal()
	w.Fiat.IsExpressWire = false
	w.Fiat.IntermediaryIBAN = "DE89370400440532013000"
	_, err = ex.RequestFiatWithdrawal(t.Context(), &FiatWithdrawalRequest{Wallet: WalletTypeExchange, Withdrawal: w})
	require.NoError(t, err, "RequestFiatWithdrawal must not error")
	assert.NotContains(t, payload, "expressWire", "a normal wire should leave the express flag out")
	assert.Equal(t, "DE89370400440532013000", payload["intermediary_bank_account"], "a wire should send its intermediary bank's IBAN as its account")

	for _, tc := range []struct {
		account float64
		err     error
	}{
		{account: 0, err: errIntermediaryBankAccountRequired},
		{account: 123456789012345678, err: errIntermediaryBankAccountInexact},
		{account: 1.5, err: errIntermediaryBankAccountInexact},
		{account: -1, err: errIntermediaryBankAccountInexact},
	} {
		w = newWithdrawal()
		w.Fiat.IntermediaryBankAccountNumber = tc.account
		_, err = ex.RequestFiatWithdrawal(t.Context(), &FiatWithdrawalRequest{Wallet: WalletTypeExchange, Withdrawal: w})
		assert.ErrorIsf(t, err, tc.err, "RequestFiatWithdrawal should refuse intermediary account number %v", tc.account)
	}
}

func TestWithdrawFiatFunds(t *testing.T) {
	t.Parallel()
	_, err := e.RequestFiatWithdrawal(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "RequestFiatWithdrawal must reject a nil request")
	_, err = e.RequestFiatWithdrawal(t.Context(), &FiatWithdrawalRequest{Wallet: "novel", Withdrawal: &withdraw.Request{}})
	require.ErrorIs(t, err, errInvalidAccountWallet, "RequestFiatWithdrawal must need a wallet")
	_, err = e.RequestFiatWithdrawal(t.Context(), &FiatWithdrawalRequest{Wallet: WalletTypeExchange, Withdrawal: &withdraw.Request{}})
	require.ErrorIs(t, err, errInvalidAmount, "RequestFiatWithdrawal must need an amount")
	if !mockTests {
		t.Skip("A withdrawal cannot be undone")
	}
	w := &withdraw.Request{
		Exchange: e.Name, Currency: currency.USD, Amount: 1000, Description: "test", Type: withdraw.Fiat,
		Fiat: withdraw.FiatRequest{Bank: banking.Account{
			Enabled: true, AccountName: "Satoshi Nakamoto", AccountNumber: "12345", BankName: "Federal Reserve Bank", BankAddress: "33 Liberty Street",
			BankPostalCity: "New York", BankCountry: "United States", SWIFTCode: "FRNYUS33", SupportedCurrencies: "USD", SupportedExchanges: e.Name,
		}},
	}
	resp, err := e.WithdrawFiatFunds(t.Context(), w)
	require.NoError(t, err, "WithdrawFiatFunds must not error")
	assert.Equal(t, &withdraw.ExchangeResponse{ID: "586829", Status: "success"}, resp, "WithdrawFiatFunds should return the withdrawal's ID")
	resp, err = e.WithdrawFiatFundsToInternationalBank(t.Context(), w)
	require.NoError(t, err, "WithdrawFiatFundsToInternationalBank must not error")
	assert.Equal(t, "586829", resp.ID, "WithdrawFiatFundsToInternationalBank should withdraw by wire")
}

func TestGetFeeByType(t *testing.T) {
	t.Parallel()
	_, err := e.GetFeeByType(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetFeeByType must reject a nil fee builder")
	wires := newTestExchange(t)
	for _, tc := range []struct {
		feeType exchange.FeeType
		c       currency.Code
		amount  float64
		exp     float64
	}{
		{feeType: exchange.InternationalBankDepositFee, c: currency.USD, amount: 10000, exp: 60},
		{feeType: exchange.InternationalBankDepositFee, c: currency.EUR, amount: 100000, exp: 100},
		{feeType: exchange.InternationalBankWithdrawalFee, c: currency.USD, amount: 10000, exp: 100},
		{feeType: exchange.InternationalBankWithdrawalFee, c: currency.EUR, amount: 200000, exp: 200},
		{feeType: exchange.InternationalBankWithdrawalFee, c: currency.GBP, amount: 1000, exp: 1},
	} {
		fee, err := wires.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: tc.feeType, FiatCurrency: tc.c, Amount: tc.amount})
		require.NoErrorf(t, err, "GetFeeByType must not error for a %s wire", tc.c)
		assert.Equalf(t, tc.exp, fee, "a %s wire of %v should cost a tenth of a percent, and at least Bitfinex's minimum for dollars and euros", tc.c, tc.amount)
	}
	offline := newTestExchange(t)
	offline.SetCredentials(&accounts.Credentials{})
	fee, err := offline.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, PurchasePrice: 100, Amount: 1})
	require.NoError(t, err, "GetFeeByType must not error without credentials")
	assert.Zero(t, fee, "a trade without credentials should be estimated free, as Bitfinex charges no trading fees")

	fee, err = e.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyWithdrawalFee, Pair: currency.NewPair(currency.ADA, currency.USD)})
	require.NoError(t, err, "GetFeeByType must not error for a withdrawal fee")
	if mockTests {
		assert.Equal(t, 0.3, fee, "the withdrawal fee should be Bitfinex's")
	}
	_, err = e.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyWithdrawalFee, Pair: currency.NewPair(currency.NewCode("NOPE"), currency.USD)})
	assert.ErrorIs(t, err, currency.ErrCurrencyNotSupported, "GetFeeByType should report a currency without a withdrawal fee")

	skipLiveWithoutCredentials(t)
	fee, err = e.GetFeeByType(t.Context(), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee, Pair: currency.NewBTCUSD(), PurchasePrice: 1000, Amount: 1})
	require.NoError(t, err, "GetFeeByType must not error for a trade")
	if mockTests {
		assert.InDelta(t, 2.0, fee, 1e-9, "a taker trade quoted in fiat should be charged the account's taker fiat rate")
	}
}

func TestTradingFeeRate(t *testing.T) {
	t.Parallel()
	rates := &SummaryFeeRates{
		Maker: SummaryMakerFeeRates{ToCrypto: 0.1, ToStable: 0.2, ToFiat: 0.3},
		Taker: SummaryTakerFeeRates{ToCrypto: 0.4, ToStable: 0.5, ToFiat: 0.6},
	}
	for _, tc := range []struct {
		p     currency.Pair
		maker bool
		exp   float64
	}{
		{p: currency.NewPair(currency.ETH, currency.BTC), maker: true, exp: 0.1},
		{p: currency.NewPair(currency.ETH, currency.UST), maker: true, exp: 0.2},
		{p: currency.NewPair(currency.ETH, currency.USDT), maker: true, exp: 0.2},
		{p: currency.NewPair(currency.ETH, currency.USD), maker: true, exp: 0.3},
		{p: currency.NewPair(currency.ETH, currency.BTC), exp: 0.4},
		{p: currency.NewPair(currency.ETH, currency.UST), exp: 0.5},
		{p: currency.NewPair(currency.ETH, currency.EUR), exp: 0.6},
	} {
		assert.Equalf(t, tc.exp, tradingFeeRate(rates, tc.p, tc.maker), "tradingFeeRate should give %s's rate, maker %t", tc.p, tc.maker)
	}
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	start, end := historyWindow(24 * time.Hour)
	for a, p := range map[asset.Item]currency.Pair{asset.Spot: currency.NewBTCUSD(), asset.MarginFunding: testFundingUSD} {
		item, err := ex.GetHistoricCandles(t.Context(), p, a, kline.OneHour, start, end)
		require.NoErrorf(t, err, "GetHistoricCandles must not error for %s", a)
		require.NotEmptyf(t, item.Candles, "GetHistoricCandles must return %s candles", a)
		assert.Truef(t, item.Candles[0].Time.Before(item.Candles[len(item.Candles)-1].Time), "GetHistoricCandles should return %s candles oldest first", a)
	}
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	start, end := historyWindow(24 * time.Hour)
	item, err := ex.GetHistoricCandlesExtended(t.Context(), currency.NewBTCUSD(), asset.Spot, kline.OneHour, start, end)
	require.NoError(t, err, "GetHistoricCandlesExtended must not error")
	assert.NotEmpty(t, item.Candles, "GetHistoricCandlesExtended should return candles")
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	ex := newWrapperTestExchange(t)
	for a, exp := range map[asset.Item]string{
		asset.Spot:          tradeBaseURL + "/t/BTC:USD?type=exchange",
		asset.Margin:        tradeBaseURL + "/t/BTC:USD?type=margin",
		asset.MarginFunding: tradeBaseURL + "/f/USD",
	} {
		p := currency.NewBTCUSD()
		if a == asset.MarginFunding {
			p = testFundingUSD
		}
		got, err := ex.GetCurrencyTradeURL(t.Context(), a, p)
		require.NoErrorf(t, err, "GetCurrencyTradeURL must not error for %s", a)
		assert.Equalf(t, exp, got, "GetCurrencyTradeURL should link %s's trade page", a)
	}
	_, err := ex.GetCurrencyTradeURL(t.Context(), asset.Spot, currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetCurrencyTradeURL should refuse an empty pair")
}

func TestUnsupportedWrapperMethods(t *testing.T) {
	t.Parallel()
	_, err := e.GetServerTime(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetServerTime should not be supported")
	_, err = e.GetFuturesContractDetails(t.Context(), asset.Futures)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetFuturesContractDetails should not be supported")
	_, err = e.GetLatestFundingRates(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetLatestFundingRates should not be supported")
	_, err = e.GetOpenInterest(t.Context())
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetOpenInterest should not be supported")
}

func TestCancelBatchOrdersThroughWebsocket(t *testing.T) {
	t.Parallel()
	skipLiveUnlessCanManipulate(t)
	ex, venue := newWebsocketTestExchange(t, subscription.List{})
	require.True(t, ex.Websocket.CanUseAuthenticatedWebsocketForWrapper(), "the wrapper must be able to use the authenticated websocket")
	ids := []string{testWrapperOrderID, "1747566429"}
	if !mockTests {
		ids = []string{strconv.FormatUint(placeLiveTestOrder(t).ID, 10)}
	}
	cancels := make([]order.Cancel, len(ids))
	for i := range ids {
		cancels[i] = order.Cancel{OrderID: ids[i], AssetType: asset.Spot, Pair: currency.NewBTCUSD()}
	}
	resp, err := ex.CancelBatchOrders(t.Context(), cancels)
	require.NoError(t, err, "CancelBatchOrders must not error through the websocket")
	for _, id := range ids {
		assert.Equalf(t, order.PendingCancel.String(), resp.Status[id], "order %s should be pending cancellation until its close event confirms it", id)
	}
	if mockTests {
		assert.Len(t, venue.inputs(wsOrderCancelMulti), 1, "CancelBatchOrders should cancel the orders in one websocket request")
	}
}

func TestWrapperOrdersThroughWebsocket(t *testing.T) {
	t.Parallel()
	skipLiveUnlessCanManipulate(t)
	ex, venue := newWebsocketTestExchange(t, subscription.List{})
	require.True(t, ex.Websocket.CanUseAuthenticatedWebsocketForWrapper(), "the wrapper must be able to use the authenticated websocket")
	req := wsTestOrderRequest(t)
	resp, err := ex.SubmitOrder(t.Context(), &order.Submit{Exchange: ex.Name, AssetType: asset.Spot, Pair: currency.NewBTCUSD(), Type: order.Limit, Side: order.Buy, Amount: req.Amount, Price: req.Price})
	require.NoError(t, err, "SubmitOrder must not error through the websocket")
	if !mockTests {
		id, err := parseOrderID(resp.OrderID)
		require.NoError(t, err, "SubmitOrder must return the order's ID")
		cancelLiveTestOrderOnCleanup(t, id)
	}
	require.NoError(t, ex.CancelOrder(t.Context(), &order.Cancel{OrderID: resp.OrderID, AssetType: asset.Spot, Pair: currency.NewBTCUSD()}), "CancelOrder must not error through the websocket")
	if mockTests {
		assert.Len(t, venue.inputs(wsOrderNew), 1, "SubmitOrder should place the order through the websocket")
		assert.Len(t, venue.inputs(wsOrderCancel), 1, "CancelOrder should cancel the order through the websocket")
	}
}

func TestAuthenticateWebsocket(t *testing.T) {
	t.Parallel()
	skipLiveWithoutCredentials(t)
	ex, venue := newWebsocketTestExchange(t, nil)
	require.NoError(t, ex.AuthenticateWebsocket(t.Context()), "AuthenticateWebsocket must not error")
	if mockTests {
		assert.Len(t, venue.events("auth"), 2, "the account connection should authenticate on connecting and again when asked")
	}
}
