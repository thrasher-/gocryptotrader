package bitstamp

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/futures"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/portfolio/banking"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

// newTestInstance returns an exchange instance named after the test, so tests which process tickers and orderbooks do
// not share global state
func newTestInstance(t *testing.T) *Exchange {
	t.Helper()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	ex.Name = t.Name()
	if mockTests {
		require.NoError(t, testexch.MockHTTPInstance(ex, "api"), "MockHTTPInstance must not error")
	}
	return ex
}

func setFeeBuilder() *exchange.FeeBuilder {
	return &exchange.FeeBuilder{
		Amount:        5,
		FeeType:       exchange.CryptocurrencyTradeFee,
		Pair:          currency.NewPair(currency.LTC, currency.BTC),
		PurchasePrice: 1800,
	}
}

func TestGetFeeByTypeOfflineTradeFee(t *testing.T) {
	t.Parallel()
	_, err := e.GetFeeByType(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetFeeByType should error on a nil fee builder")

	skipIfLiveWithoutCredentials(t)
	feeBuilder := setFeeBuilder()
	_, err = e.GetFeeByType(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFeeByType must not error")
	if mockTests {
		assert.Equal(t, exchange.OfflineTradeFee, feeBuilder.FeeType, "FeeType should be offline when authentication checks are skipped")
	} else {
		assert.Equal(t, exchange.CryptocurrencyTradeFee, feeBuilder.FeeType, "FeeType should not change with valid credentials")
	}
}

func TestGetFee(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)

	feeBuilder := setFeeBuilder()
	fee, err := e.GetFee(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFee must not error")
	if mockTests {
		assert.InDelta(t, 0.4/100*1800*5, fee, 1e-9, "taker fee should be correct")
	}

	feeBuilder.IsMaker = true
	fee, err = e.GetFee(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFee must not error for a maker fee")
	if mockTests {
		assert.InDelta(t, 0.3/100*1800*5, fee, 1e-9, "maker fee should be correct")
	}

	feeBuilder = setFeeBuilder()
	feeBuilder.PurchasePrice = -1000
	fee, err = e.GetFee(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFee must not error for a negative purchase price")
	assert.Zero(t, fee, "negative fees should be zero")

	feeBuilder = setFeeBuilder()
	feeBuilder.FeeType = exchange.CryptocurrencyWithdrawalFee
	fee, err = e.GetFee(t.Context(), feeBuilder)
	require.NoError(t, err, "GetFee must not error for a withdrawal fee")
	if mockTests {
		assert.Equal(t, 0.001, fee, "withdrawal fee should be correct")
	}

	for feeType, exp := range map[exchange.FeeType]float64{
		exchange.CryptocurrencyDepositFee:       0,
		exchange.InternationalBankDepositFee:    7.5,
		exchange.InternationalBankWithdrawalFee: 15,
		exchange.OfflineTradeFee:                0.0025 * 1800 * 5,
	} {
		feeBuilder = setFeeBuilder()
		feeBuilder.FeeType = feeType
		fee, err = e.GetFee(t.Context(), feeBuilder)
		require.NoErrorf(t, err, "GetFee must not error for %v", feeType)
		assert.InDeltaf(t, exp, fee, 1e-9, "GetFee should return the fee for %v", feeType)
	}
}

func TestGetInternationalBankFees(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 7.5, getInternationalBankDepositFee(1000), "deposit fee should have a minimum")
	assert.Equal(t, 10.0, getInternationalBankDepositFee(20000), "deposit fee should be a percentage")
	assert.Equal(t, 300.0, getInternationalBankDepositFee(1000000), "deposit fee should have a maximum")
	assert.Equal(t, 15.0, getInternationalBankWithdrawalFee(1000), "withdrawal fee should have a minimum")
	assert.Equal(t, 18.0, getInternationalBankWithdrawalFee(20000), "withdrawal fee should be a percentage")
}

func TestFormatWithdrawPermissions(t *testing.T) {
	t.Parallel()
	assert.Equal(t, exchange.AutoWithdrawCryptoText+" & "+exchange.AutoWithdrawFiatText, e.FormatWithdrawPermissions(), "FormatWithdrawPermissions should be correct")
}

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	_, err := e.FetchTradablePairs(t.Context(), asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "FetchTradablePairs should error on an unsupported asset")

	for a, exp := range map[asset.Item]currency.Pair{asset.Spot: spotPair, asset.PerpetualContract: perpetualPair} {
		pairs, err := e.FetchTradablePairs(t.Context(), a)
		require.NoErrorf(t, err, "FetchTradablePairs must not error for %s", a)
		assert.Truef(t, pairs.Contains(exp, true), "FetchTradablePairs should return %s for %s", exp, a)
		for _, p := range pairs {
			assert.Equalf(t, a == asset.PerpetualContract, p.Quote.Upper().String() == "USD-PERP" || len(p.Quote.String()) > 5 && p.Quote.Upper().String()[len(p.Quote.String())-5:] == "-PERP", "pair %s should belong to %s", p, a)
		}
	}
}

func TestTradablePairs(t *testing.T) {
	t.Parallel()
	markets := []MarketResponse{
		{Name: "BTC/USD", MarketType: MarketTypeSpot, Trading: tradingEnabled},
		{Name: "ETH/USD", MarketType: MarketTypeSpot, Trading: "Disabled"},
		{Name: "BTC/USD-PERP", MarketType: MarketTypePerpetual, Trading: tradingEnabled},
	}
	pairs, err := tradablePairs(markets, MarketTypeSpot)
	require.NoError(t, err, "tradablePairs must not error")
	assert.Equal(t, currency.Pairs{currency.NewPairWithDelimiter("BTC", "USD", "/")}, pairs, "tradablePairs should skip disabled and other market types")

	_, err = tradablePairs([]MarketResponse{{Name: "BTCUSD", MarketType: MarketTypeSpot, Trading: tradingEnabled}}, MarketTypeSpot)
	assert.Error(t, err, "tradablePairs should error on a malformed market name")
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.GetAvailablePairs(a)
		require.NoErrorf(t, err, "GetAvailablePairs must not error for %s", a)
		assert.NotEmptyf(t, pairs, "available pairs should be updated for %s", a)
	}
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, e.UpdateOrderExecutionLimits(t.Context(), asset.Binary), asset.ErrNotSupported, "UpdateOrderExecutionLimits should error on an unsupported asset")

	for a, pair := range map[asset.Item]currency.Pair{asset.Spot: spotPair, asset.PerpetualContract: perpetualPair} {
		require.NoErrorf(t, e.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must not error for %s", a)
		l, err := e.GetOrderExecutionLimits(a, pair)
		require.NoErrorf(t, err, "GetOrderExecutionLimits must not error for %s", pair)
		assert.Positivef(t, l.PriceStepIncrementSize, "PriceStepIncrementSize should be positive for %s", pair)
		assert.Positivef(t, l.AmountStepIncrementSize, "AmountStepIncrementSize should be positive for %s", pair)
		assert.Positivef(t, l.MinimumQuoteAmount, "MinimumQuoteAmount should be positive for %s", pair)
		if a == asset.PerpetualContract {
			assert.Positivef(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive for %s", pair)
			assert.Greaterf(t, l.MaximumBaseAmount, l.MinimumBaseAmount, "MaximumBaseAmount should exceed MinimumBaseAmount for %s", pair)
			assert.Positivef(t, l.MaximumQuoteAmount, "MaximumQuoteAmount should be positive for %s", pair)
		}
	}
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	ex := newTestInstance(t)
	assert.ErrorIs(t, ex.UpdateTickers(t.Context(), asset.Futures), asset.ErrNotSupported, "UpdateTickers should error on an unsupported asset")

	for a, pair := range map[asset.Item]currency.Pair{asset.Spot: spotPair, asset.PerpetualContract: perpetualPair} {
		require.NoErrorf(t, ex.UpdateTickers(t.Context(), a), "UpdateTickers must not error for %s", a)
		tick, err := ticker.GetTicker(ex.Name, pair, a)
		require.NoErrorf(t, err, "GetTicker must not error for %s", pair)
		assert.Positivef(t, tick.Last, "Last should be positive for %s", pair)
		assert.NotZerof(t, tick.LastUpdated, "LastUpdated should be set for %s", pair)
		if a == asset.PerpetualContract {
			assert.Positivef(t, tick.MarkPrice, "MarkPrice should be positive for %s", pair)
			assert.Positivef(t, tick.IndexPrice, "IndexPrice should be positive for %s", pair)
			assert.Positivef(t, tick.OpenInterest, "OpenInterest should be positive for %s", pair)
		}
	}
	_, err := ticker.GetTicker(ex.Name, perpetualPair, asset.Spot)
	assert.ErrorIs(t, err, ticker.ErrTickerNotFound, "UpdateTickers should not store perpetual tickers as spot")
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	ex := newTestInstance(t)
	_, err := ex.UpdateTicker(t.Context(), currency.EMPTYPAIR, asset.Spot)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "UpdateTicker should error on an empty pair")
	_, err = ex.UpdateTicker(t.Context(), spotPair, asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "UpdateTicker should error on an unsupported asset")

	for a, pair := range map[asset.Item]currency.Pair{asset.Spot: spotPair, asset.PerpetualContract: perpetualPair} {
		tick, err := ex.UpdateTicker(t.Context(), pair, a)
		require.NoErrorf(t, err, "UpdateTicker must not error for %s", pair)
		assert.Truef(t, tick.Pair.Equal(pair), "Pair should be correct for %s", pair)
		assert.Equalf(t, a, tick.AssetType, "AssetType should be correct for %s", pair)
		assert.Positivef(t, tick.Last, "Last should be positive for %s", pair)
		assert.Positivef(t, tick.BaseVolume, "BaseVolume should be positive for %s", pair)
	}
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	ex := newTestInstance(t)
	_, err := ex.UpdateOrderbook(t.Context(), currency.EMPTYPAIR, asset.Spot)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "UpdateOrderbook should error on an empty pair")
	_, err = ex.UpdateOrderbook(t.Context(), spotPair, asset.Futures)
	assert.Error(t, err, "UpdateOrderbook should error on an unsupported asset")

	for a, pair := range map[asset.Item]currency.Pair{asset.Spot: spotPair, asset.PerpetualContract: perpetualPair} {
		ob, err := ex.UpdateOrderbook(t.Context(), pair, a)
		require.NoErrorf(t, err, "UpdateOrderbook must not error for %s", pair)
		require.NotEmptyf(t, ob.Bids, "Bids must not be empty for %s", pair)
		require.NotEmptyf(t, ob.Asks, "Asks must not be empty for %s", pair)
		assert.Greaterf(t, ob.Asks[0].Price, ob.Bids[0].Price, "best ask should exceed the best bid for %s", pair)
		assert.NotZerof(t, ob.LastUpdated, "LastUpdated should be set for %s", pair)
	}
}

func TestFilterOrderbookZeroBidPrice(t *testing.T) {
	t.Parallel()
	ob := &orderbook.Book{}
	filterOrderbookZeroBidPrice(ob)
	assert.Empty(t, ob.Bids, "empty bids should remain empty")

	ob.Bids = orderbook.Levels{{Price: 69, Amount: 1337}, {Price: 0, Amount: 69}}
	filterOrderbookZeroBidPrice(ob)
	assert.Equal(t, orderbook.Levels{{Price: 69, Amount: 1337}}, ob.Bids, "the trailing zero priced bid should be removed")

	ob.Bids = orderbook.Levels{{Price: 59, Amount: 1337}, {Price: 42, Amount: 8595}}
	filterOrderbookZeroBidPrice(ob)
	assert.Equal(t, orderbook.Levels{{Price: 59, Amount: 1337}, {Price: 42, Amount: 8595}}, ob.Bids, "priced bids should be retained")
}

func TestUpdateAccountBalances(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateAccountBalances(t.Context(), asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "UpdateAccountBalances should error on an unsupported asset")

	skipIfLiveWithoutCredentials(t)
	ctx := t.Context()
	if mockTests {
		// Balances are stored against the credentials used, which the mock instance does not have
		ctx = accounts.DeployCredentialsToContext(ctx, &accounts.Credentials{Key: "key", Secret: "secret"})
	}
	for a, exp := range map[asset.Item]map[currency.Code]float64{
		asset.Spot:              {currency.USD: 100, currency.BTC: 1.5},
		asset.PerpetualContract: {currency.USD: 15000, currency.BTC: 0.1},
	} {
		subAccts, err := e.UpdateAccountBalances(ctx, a)
		require.NoErrorf(t, err, "UpdateAccountBalances must not error for %s", a)
		require.Lenf(t, subAccts, 1, "UpdateAccountBalances must return a sub account for %s", a)
		assert.Equalf(t, a, subAccts[0].AssetType, "AssetType should be correct for %s", a)
		if !mockTests {
			continue
		}
		for c, total := range exp {
			balance, ok := subAccts[0].Balances[c]
			require.Truef(t, ok, "balance must be set for %s %s", a, c)
			assert.Equalf(t, total, balance.Total, "Total should be correct for %s %s", a, c)
		}
	}
}

func TestGetAccountFundingHistory(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	history, err := e.GetAccountFundingHistory(t.Context())
	require.NoError(t, err, "GetAccountFundingHistory must not error")
	if mockTests {
		require.Len(t, history, 2, "GetAccountFundingHistory must return the deposit and withdrawal")
		assert.Equal(t, exchange.FundingHistory{
			ExchangeName:      e.Name,
			Status:            "PENDING",
			TransferID:        "1",
			Timestamp:         time.Unix(1759995000, 0),
			Currency:          "BTC",
			Amount:            1.23,
			TransferType:      "deposit",
			CryptoToAddress:   "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			CryptoFromAddress: "1htZQJS6YUUPIGNNPC9425xENXQS2JwM7C",
			CryptoTxID:        "e4123d1d57df4106aaae5ec4d77eb6cd42e226d020a0a2c1c7919d14b93",
			CryptoChain:       "bitcoin",
		}, history[0], "deposit should be correct")
		assert.Equal(t, "withdrawal", history[1].TransferType, "TransferType should be correct")
		assert.Equal(t, "3FiKkjgZ6Sj4RWp3ZsCjYh5Pt7ZCBsL7uF", history[1].CryptoToAddress, "CryptoToAddress should be correct")
	}
}

func TestGetWithdrawalsHistory(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	for c, exp := range map[currency.Code]int{currency.EMPTYCODE: 2, currency.BTC: 1} {
		history, err := e.GetWithdrawalsHistory(t.Context(), c, asset.Spot)
		require.NoErrorf(t, err, "GetWithdrawalsHistory must not error for %q", c)
		if !mockTests {
			continue
		}
		require.Lenf(t, history, exp, "GetWithdrawalsHistory must return the withdrawals for %q", c)
		assert.Equal(t, exchange.WithdrawalHistory{
			Status:          "Finished",
			TransferID:      "1",
			Timestamp:       time.Date(2022, 1, 31, 16, 7, 32, 0, time.UTC),
			Currency:        "BTC",
			Amount:          0.00006,
			TransferType:    "Cryptocurrency",
			CryptoToAddress: core.BitcoinDonationAddress,
			CryptoTxID:      "NsOeFbQhRnpGzNIThWGBTkQwRJqTNOGPVhYavrVyMfkAyMUmIlUpFIwGTzSvpeOP",
			CryptoChain:     "bitcoin",
		}, history[0], "withdrawal should be correct")
	}
}

func TestWithdrawalStatusAndType(t *testing.T) {
	t.Parallel()
	for status, exp := range map[uint8]string{0: "Open", 1: "In process", 2: "Finished", 3: "Canceled", 4: "Failed", 11: "Reversed", 7: "7"} {
		assert.Equalf(t, exp, withdrawalStatus(status), "withdrawalStatus should describe %d", status)
	}
	for withdrawalTypeID, exp := range map[uint64]string{0: "SEPA", 2: "Wire transfer", 1: "Cryptocurrency", 22: "Cryptocurrency"} {
		assert.Equalf(t, exp, withdrawalType(withdrawalTypeID), "withdrawalType should describe %d", withdrawalTypeID)
	}
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetRecentTrades(t.Context(), currency.EMPTYPAIR, asset.Spot)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetRecentTrades should error on an empty pair")
	_, err = e.GetRecentTrades(t.Context(), spotPair, asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetRecentTrades should error on an unsupported asset")

	for a, pair := range map[asset.Item]currency.Pair{asset.Spot: spotPair, asset.PerpetualContract: perpetualPair} {
		trades, err := e.GetRecentTrades(t.Context(), pair, a)
		require.NoErrorf(t, err, "GetRecentTrades must not error for %s", pair)
		require.NotEmptyf(t, trades, "GetRecentTrades must return trades for %s", pair)
		for i := range trades {
			assert.Equalf(t, a, trades[i].AssetType, "AssetType should be correct for %s", pair)
			assert.Truef(t, trades[i].CurrencyPair.Equal(pair), "CurrencyPair should be correct for %s", pair)
			assert.NotEmptyf(t, trades[i].TID, "TID should be set for %s", pair)
			assert.Positivef(t, trades[i].Price, "Price should be positive for %s", pair)
			assert.Positivef(t, trades[i].Amount, "Amount should be positive for %s", pair)
			if i > 0 {
				assert.Falsef(t, trades[i].Timestamp.Before(trades[i-1].Timestamp), "trades should be sorted by date for %s", pair)
			}
		}
	}
}

func TestGetHistoricTrades(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricTrades(t.Context(), spotPair, asset.Spot, time.Now().Add(-time.Minute*15), time.Now())
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetHistoricTrades should not be supported")
}

func TestSubmitOrder(t *testing.T) {
	t.Parallel()
	_, err := e.SubmitOrder(t.Context(), nil)
	assert.ErrorIs(t, err, order.ErrSubmissionIsNil, "SubmitOrder should error on a nil submission")
	_, err = e.SubmitOrder(t.Context(), &order.Submit{Exchange: e.Name, Pair: spotPair, AssetType: asset.Futures, Side: order.Buy, Type: order.Market, Amount: 1})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "SubmitOrder should error on an unsupported asset")
	_, err = e.SubmitOrder(t.Context(), &order.Submit{Exchange: e.Name, Pair: spotPair, AssetType: asset.Spot, Side: order.Buy, Type: order.StopLimit, Amount: 1, Price: 1})
	assert.ErrorIs(t, err, order.ErrUnsupportedOrderType, "SubmitOrder should error on a conditional spot order")
	_, err = e.SubmitOrder(t.Context(), &order.Submit{Exchange: e.Name, Pair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Buy, Type: order.StopLimit, Amount: 1, Price: 1, Leverage: 2})
	assert.ErrorIs(t, err, order.ErrPriceMustBeSetIfLimitOrder, "SubmitOrder should error on a conditional order without a trigger price")
	_, err = e.SubmitOrder(t.Context(), &order.Submit{Exchange: e.Name, Pair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Buy, Type: order.TrailingStop, Amount: 1, TriggerPrice: 1, Leverage: 2})
	assert.ErrorIs(t, err, order.ErrUnsupportedOrderType, "SubmitOrder should error on an unsupported conditional order")
	_, err = e.SubmitOrder(t.Context(), &order.Submit{Exchange: e.Name, Pair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Buy, Type: order.Limit, Amount: 1, Price: 1, Leverage: 2, MarginType: margin.SpotIsolated})
	assert.ErrorIs(t, err, margin.ErrMarginTypeUnsupported, "SubmitOrder should error on an unsupported margin type")
	_, err = e.SubmitOrder(t.Context(), &order.Submit{Exchange: e.Name, Pair: spotPair, AssetType: asset.Spot, Side: order.Buy, Type: order.Limit, Amount: 1, Price: 1, TimeInForce: order.GoodTillCrossing})
	assert.ErrorIs(t, err, order.ErrUnsupportedTimeInForce, "SubmitOrder should error on an unsupported time in force")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	for _, tc := range []struct {
		name string
		s    *order.Submit
		exp  string
	}{
		{name: "spot limit", s: &order.Submit{Pair: spotPair, AssetType: asset.Spot, Side: order.Buy, Type: order.Limit, Amount: 0.5, Price: 20000, ClientOrderID: "123456789"}, exp: "1234123412341236"},
		{name: "spot market", s: &order.Submit{Pair: spotPair, AssetType: asset.Spot, Side: order.Sell, Type: order.Market, Amount: 0.1}, exp: "1234123412341238"},
		{name: "spot market quote amount", s: &order.Submit{Pair: spotPair, AssetType: asset.Spot, Side: order.Buy, Type: order.Market, QuoteAmount: 100}, exp: "1234123412341239"},
		{name: "perpetual limit", s: &order.Submit{Pair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Buy, Type: order.Limit, Amount: 0.01, Price: 20000, Leverage: 5, MarginType: margin.Isolated}, exp: "1234123412341241"},
		{name: "perpetual market with current leverage", s: &order.Submit{Pair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Sell, Type: order.Market, Amount: 0.01}, exp: "1234123412341242"},
		{name: "perpetual stop loss", s: &order.Submit{Pair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Sell, Type: order.StopMarket, Amount: 0.01, TriggerPrice: 19000, TriggerPriceType: order.MarkPrice, Leverage: 2, ReduceOnly: true}, exp: "1234123412341243"},
		{name: "perpetual take profit limit", s: &order.Submit{Pair: perpetualPair, AssetType: asset.PerpetualContract, Side: order.Buy, Type: order.TakeProfit | order.Limit, Amount: 0.01, Price: 21000, TriggerPrice: 20500, Leverage: 2}, exp: "1234123412341244"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.s.Exchange = e.Name
			resp, err := e.SubmitOrder(t.Context(), tc.s)
			require.NoError(t, err, "SubmitOrder must not error")
			if mockTests {
				assert.Equal(t, tc.exp, resp.OrderID, "OrderID should be correct")
				assert.Equal(t, tc.s.AssetType, resp.AssetType, "AssetType should be correct")
			}
		})
	}
}

func TestSetLimitOrderTimeInForce(t *testing.T) {
	t.Parallel()
	endTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		tif order.TimeInForce
		exp LimitOrderRequest
		err error
	}{
		{tif: order.UnknownTIF},
		{tif: order.GoodTillCancel},
		{tif: order.ImmediateOrCancel, exp: LimitOrderRequest{ImmediateOrCancel: true}},
		{tif: order.FillOrKill, exp: LimitOrderRequest{FillOrKill: true}},
		{tif: order.GoodTillCancel | order.PostOnly, exp: LimitOrderRequest{MakerOrCancel: true}},
		{tif: order.GoodTillDay, exp: LimitOrderRequest{DailyOrder: true}},
		{tif: order.GoodTillTime, exp: LimitOrderRequest{GoodTillDate: true, ExpireTime: endTime}},
		{tif: order.GoodTillCrossing, err: order.ErrUnsupportedTimeInForce},
		{tif: order.StopOrReduce, err: order.ErrUnsupportedTimeInForce},
	} {
		var req LimitOrderRequest
		err := setLimitOrderTimeInForce(&req, tc.tif, endTime)
		if tc.err != nil {
			assert.ErrorIsf(t, err, tc.err, "setLimitOrderTimeInForce should error for %s", tc.tif)
			continue
		}
		require.NoErrorf(t, err, "setLimitOrderTimeInForce must not error for %s", tc.tif)
		assert.Equalf(t, tc.exp, req, "setLimitOrderTimeInForce should set the flags for %s", tc.tif)
	}
	assert.ErrorIs(t, setLimitOrderTimeInForce(&LimitOrderRequest{}, order.GoodTillTime, time.Time{}), common.ErrDateUnset, "setLimitOrderTimeInForce should error without an end time")
}

func TestModifyOrder(t *testing.T) {
	t.Parallel()
	_, err := e.ModifyOrder(t.Context(), nil)
	assert.ErrorIs(t, err, order.ErrModifyOrderIsNil, "ModifyOrder should error on a nil request")
	_, err = e.ModifyOrder(t.Context(), &order.Modify{Pair: spotPair, AssetType: asset.Futures, OrderID: "1"})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "ModifyOrder should error on an unsupported asset")
	_, err = e.ModifyOrder(t.Context(), &order.Modify{Pair: spotPair, AssetType: asset.Spot, OrderID: "nope"})
	assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "ModifyOrder should error on an invalid order ID")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.ModifyOrder(t.Context(), &order.Modify{Exchange: e.Name, Pair: spotPair, AssetType: asset.Spot, OrderID: "1453282316578816", Amount: 0.02, Price: 2100.45})
	if !mockTests {
		assert.ErrorIs(t, err, errAPIResponse, "ModifyOrder should error for an unknown order")
		return
	}
	require.NoError(t, err, "ModifyOrder must not error")
	assert.Equal(t, "1453282316578817", resp.OrderID, "OrderID should be the replacement order's ID")
	assert.Equal(t, 0.02, resp.Amount, "Amount should be correct")
	assert.Equal(t, 2100.45, resp.Price, "Price should be correct")

	resp, err = e.ModifyOrder(t.Context(), &order.Modify{Exchange: e.Name, Pair: spotPair, AssetType: asset.Spot, ClientOrderID: "my-order-123", NewClientOrderID: "my-order-456", Amount: 0.02, Price: 2100.45})
	require.NoError(t, err, "ModifyOrder must not error with a client order ID")
	assert.Equal(t, "my-order-456", resp.ClientOrderID, "ClientOrderID should be the replacement order's client order ID")
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, e.CancelOrder(t.Context(), nil), order.ErrCancelOrderIsNil, "CancelOrder should error on a nil request")
	assert.ErrorIs(t, e.CancelOrder(t.Context(), &order.Cancel{OrderID: "nope"}), order.ErrOrderIDNotSet, "CancelOrder should error on an invalid order ID")
	assert.ErrorIs(t, e.CancelOrder(t.Context(), &order.Cancel{}), order.ErrOrderIDNotSet, "CancelOrder should error without an order ID")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	err := e.CancelOrder(t.Context(), &order.Cancel{OrderID: "1453282316578816"})
	if !mockTests {
		assert.ErrorIs(t, err, errAPIResponse, "CancelOrder should error for an unknown order")
		return
	}
	assert.NoError(t, err, "CancelOrder should not error")
	assert.NoError(t, e.CancelOrder(t.Context(), &order.Cancel{ClientOrderID: "my-order-123"}), "CancelOrder should not error with a client order ID")
}

func TestCancelOrderPrefersOrderID(t *testing.T) {
	t.Parallel()
	ex := newTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/cancel_order/", r.URL.Path, "path should be correct")
		assert.NoError(t, r.ParseForm(), "ParseForm should not error")
		assert.Equal(t, url.Values{"id": {"1453282316578816"}}, r.PostForm, "only the order ID should be sent")
		_, err := w.Write([]byte(`{"id":1453282316578816,"amount":0.02035278,"price":2100.45,"type":0,"market":"BTC/USD","status":"Canceled"}`))
		assert.NoError(t, err, "Write should not error")
	})
	err := ex.CancelOrder(t.Context(), &order.Cancel{OrderID: "1453282316578816", ClientOrderID: "my-order-123"})
	assert.NoError(t, err, "CancelOrder should not error with both IDs")
}

func TestCancelBatchOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelBatchOrders(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "CancelBatchOrders should not be supported")
}

func TestCancelAllOrders(t *testing.T) {
	t.Parallel()
	_, err := e.CancelAllOrders(t.Context(), nil)
	assert.ErrorIs(t, err, order.ErrCancelOrderIsNil, "CancelAllOrders should error on a nil request")
	_, err = e.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Futures})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "CancelAllOrders should error on an unsupported asset")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	for _, req := range []*order.Cancel{{AssetType: asset.Spot, Pair: spotPair}, {AssetType: asset.Spot}} {
		resp, err := e.CancelAllOrders(t.Context(), req)
		require.NoErrorf(t, err, "CancelAllOrders must not error for %q", req.Pair)
		if mockTests {
			assert.Equalf(t, map[string]string{"1234123412341234": order.Cancelled.String()}, resp.Status, "CancelAllOrders should only cancel spot orders for %q", req.Pair)
		}
	}
	if !mockTests {
		return
	}
	resp, err := e.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	assert.ErrorIs(t, err, errCancelAllOrdersFailed, "CancelAllOrders should error when not all orders are cancelled")
	assert.Equal(t, map[string]string{"1234123412341235": order.Cancelled.String()}, resp.Status, "CancelAllOrders should report the cancelled perpetual orders")
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderInfo(t.Context(), "nope", spotPair, asset.Spot)
	assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "GetOrderInfo should error on an invalid order ID")

	skipIfLiveWithoutCredentials(t)
	d, err := e.GetOrderInfo(t.Context(), "1458532827766784", spotPair, asset.Spot)
	if !mockTests {
		assert.ErrorIs(t, err, errAPIResponse, "GetOrderInfo should error for an unknown order")
		return
	}
	require.NoError(t, err, "GetOrderInfo must not error")
	assert.Equal(t, "1458532827766784", d.OrderID, "OrderID should be correct")
	assert.Equal(t, "my-order-123", d.ClientOrderID, "ClientOrderID should be correct")
	assert.Equal(t, order.Limit, d.Type, "Type should be correct")
	assert.Equal(t, order.Buy, d.Side, "Side should be correct")
	assert.Equal(t, order.PartiallyFilled, d.Status, "Status should be correct")
	assert.Equal(t, asset.Spot, d.AssetType, "AssetType should be correct")
	assert.True(t, d.Pair.Equal(spotPair), "Pair should be correct")
	assert.Equal(t, time.Date(2022, 1, 31, 14, 43, 15, 0, time.UTC), d.Date, "Date should be correct")
	assert.Equal(t, time.Date(2022, 1, 31, 14, 46, 15, 0, time.UTC), d.LastUpdated, "LastUpdated should be the latest trade")
	assert.InDelta(t, 0.5, d.Amount, 1e-9, "Amount should include executed and remaining amounts")
	assert.InDelta(t, 0.2, d.ExecutedAmount, 1e-9, "ExecutedAmount should be correct")
	assert.InDelta(t, 0.3, d.RemainingAmount, 1e-9, "RemainingAmount should be correct")
	assert.InDelta(t, 4010, d.Cost, 1e-9, "Cost should be correct")
	assert.InDelta(t, 20050, d.AverageExecutedPrice, 1e-9, "AverageExecutedPrice should be correct")
	assert.InDelta(t, 16.04, d.Fee, 1e-9, "Fee should be correct")
	require.Len(t, d.Trades, 2, "Trades must contain the order's trades")
	assert.Equal(t, order.TradeHistory{TID: "209895701", Price: 20000, Amount: 0.1, Fee: 8, Exchange: e.Name, Side: order.Buy, Timestamp: time.Date(2022, 1, 31, 14, 45, 15, 322000000, time.UTC)}, d.Trades[0], "trade should be correct")

	d, err = e.GetOrderInfo(t.Context(), "1458532827766785", perpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetOrderInfo must not error for a perpetual order")
	assert.Equal(t, order.StopLimit, d.Type, "Type should be correct")
	assert.Equal(t, order.Sell, d.Side, "Side should be correct")
	assert.Equal(t, order.PartiallyCancelled, d.Status, "Status should be correct")
	assert.Equal(t, asset.PerpetualContract, d.AssetType, "AssetType should be correct")
	assert.True(t, d.Pair.Equal(perpetualPair), "Pair should be correct")
	assert.Equal(t, margin.Isolated, d.MarginType, "MarginType should be correct")
	assert.Equal(t, 3.1, d.Leverage, "Leverage should be correct")
	assert.Equal(t, 83500.0, d.TriggerPrice, "TriggerPrice should be correct")
	assert.InDelta(t, 0.01, d.ExecutedAmount, 1e-9, "ExecutedAmount should be correct")
}

func TestOrderStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status            string
		partiallyExecuted bool
		exp               order.Status
	}{
		{status: "Open", exp: order.Open},
		{status: "Open", partiallyExecuted: true, exp: order.PartiallyFilled},
		{status: "Finished", exp: order.Filled},
		{status: "Canceled", exp: order.Cancelled},
		{status: "Canceled", partiallyExecuted: true, exp: order.PartiallyCancelled},
		{status: "Expired", exp: order.Expired},
		{status: "Unknown", exp: order.UnknownStatus},
	} {
		assert.Equalf(t, tc.exp, orderStatus(tc.status, tc.partiallyExecuted), "orderStatus should convert %q", tc.status)
	}
}

func TestGetDepositAddress(t *testing.T) {
	t.Parallel()
	skipIfLiveWithoutCredentials(t)
	for _, tc := range []struct {
		c     currency.Code
		chain string
		exp   string
		tag   string
	}{
		{c: currency.BTC, exp: core.BitcoinDonationAddress},
		{c: currency.ETH, chain: "ethereum", exp: "0x6a56f5b80f04b4fd70d64d72e1396698635e5436"},
		{c: currency.XRP, exp: "rvYAfWj5gh67oV6fW32ZzP3Aw4Eubs59B", tag: "89473951"},
		{c: currency.XLM, exp: "GAHK7EEG2WWHVKDNT4CEQFZGKF2LGDSW2IVM4S5DP42RBW3K6BTODB4A", tag: "299576079"},
	} {
		addr, err := e.GetDepositAddress(t.Context(), tc.c, "", tc.chain)
		require.NoErrorf(t, err, "GetDepositAddress must not error for %s", tc.c)
		if mockTests {
			assert.Equalf(t, tc.exp, addr.Address, "Address should be correct for %s", tc.c)
			assert.Equalf(t, tc.tag, addr.Tag, "Tag should be correct for %s", tc.c)
			assert.Equalf(t, tc.chain, addr.Chain, "Chain should be correct for %s", tc.c)
		}
	}
}

func TestGetAvailableTransferChains(t *testing.T) {
	t.Parallel()
	_, err := e.GetAvailableTransferChains(t.Context(), currency.EMPTYCODE)
	assert.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "GetAvailableTransferChains should error on an empty currency")
	_, err = e.GetAvailableTransferChains(t.Context(), currency.NewCode("NOTACURRENCY"))
	assert.ErrorIs(t, err, currency.ErrCurrencyNotFound, "GetAvailableTransferChains should error on an unlisted currency")

	chains, err := e.GetAvailableTransferChains(t.Context(), currency.ETH)
	require.NoError(t, err, "GetAvailableTransferChains must not error")
	assert.Contains(t, chains, "ethereum", "GetAvailableTransferChains should return the currency's networks")
}

func TestWithdrawCryptocurrencyFunds(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawCryptocurrencyFunds(t.Context(), &withdraw.Request{})
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet, "WithdrawCryptocurrencyFunds should validate the request")

	if !mockTests {
		t.Skip("WithdrawCryptocurrencyFunds is not tested live to avoid withdrawing funds")
	}
	for _, tc := range []struct {
		c       currency.Code
		amount  float64
		address string
		chain   string
		tag     string
		exp     string
	}{
		{c: currency.BTC, amount: 0.001, address: core.BitcoinDonationAddress, chain: "bitcoin", exp: "1"},
		{c: currency.XRP, amount: 20, address: "rvYAfWj5gh67oV6fW32ZzP3Aw4Eubs59B", tag: "12345", exp: "4"},
		{c: currency.XLM, amount: 20, address: "GAHK7EEG2WWHVKDNT4CEQFZGKF2LGDSW2IVM4S5DP42RBW3K6BTODB4A", tag: "12345", exp: "5"},
	} {
		resp, err := e.WithdrawCryptocurrencyFunds(t.Context(), &withdraw.Request{
			Exchange: e.Name,
			Type:     withdraw.Crypto,
			Currency: tc.c,
			Amount:   tc.amount,
			Crypto:   withdraw.CryptoRequest{Address: tc.address, AddressTag: tc.tag, Chain: tc.chain},
		})
		require.NoErrorf(t, err, "WithdrawCryptocurrencyFunds must not error for %s", tc.c)
		assert.Equalf(t, tc.exp, resp.ID, "ID should be correct for %s", tc.c)
	}
}

func testBankWithdrawalRequest(c currency.Code, amount float64) *withdraw.Request {
	return &withdraw.Request{
		Type:     withdraw.Fiat,
		Exchange: e.Name,
		Fiat: withdraw.FiatRequest{
			Bank: banking.Account{
				SupportedExchanges:  e.Name,
				Enabled:             true,
				AccountName:         "Satoshi Nakamoto",
				AccountNumber:       "12345",
				BankAddress:         "123 Fake St",
				BankPostalCity:      "Tarry Town",
				BankCountry:         "AU",
				BankName:            "Federal Reserve Bank",
				SWIFTCode:           "CTBAAU2S",
				BankPostalCode:      "2088",
				IBAN:                "IT60X0542811101000000123456",
				SupportedCurrencies: c.String(),
			},
			WireCurrency:               currency.USD.String(),
			IntermediaryBankAddress:    "123 Fake St",
			IntermediaryBankCity:       "Tarry Town",
			IntermediaryBankCountry:    "AU",
			IntermediaryBankName:       "Federal Reserve Bank",
			IntermediaryBankPostalCode: "2088",
		},
		Amount:      amount,
		Currency:    c,
		Description: "WITHDRAW IT ALL",
	}
}

func TestWithdrawFiatFunds(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawFiatFunds(t.Context(), &withdraw.Request{})
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet, "WithdrawFiatFunds should validate the request")

	if !mockTests {
		t.Skip("WithdrawFiatFunds is not tested live to avoid withdrawing funds")
	}
	resp, err := e.WithdrawFiatFunds(t.Context(), testBankWithdrawalRequest(currency.EUR, 10))
	require.NoError(t, err, "WithdrawFiatFunds must not error")
	assert.Equal(t, "1", resp.ID, "ID should be correct")
}

func TestWithdrawFiatFundsToInternationalBank(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawFiatFundsToInternationalBank(t.Context(), &withdraw.Request{})
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet, "WithdrawFiatFundsToInternationalBank should validate the request")

	if !mockTests {
		t.Skip("WithdrawFiatFundsToInternationalBank is not tested live to avoid withdrawing funds")
	}
	resp, err := e.WithdrawFiatFundsToInternationalBank(t.Context(), testBankWithdrawalRequest(currency.USD, 50))
	require.NoError(t, err, "WithdrawFiatFundsToInternationalBank must not error")
	assert.Equal(t, "2", resp.ID, "ID should be correct")
}

func TestGetActiveOrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetActiveOrders(t.Context(), nil)
	assert.ErrorIs(t, err, order.ErrGetOrdersRequestIsNil, "GetActiveOrders should error on a nil request")
	_, err = e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{AssetType: asset.Futures, Side: order.AnySide, Type: order.AnyType})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetActiveOrders should error on an unsupported asset")

	skipIfLiveWithoutCredentials(t)
	for _, req := range []*order.MultiOrderRequest{
		{AssetType: asset.Spot, Side: order.AnySide, Type: order.AnyType},
		{AssetType: asset.Spot, Side: order.AnySide, Type: order.AnyType, Pairs: currency.Pairs{spotPair}},
		{AssetType: asset.PerpetualContract, Side: order.AnySide, Type: order.AnyType},
	} {
		orders, err := e.GetActiveOrders(t.Context(), req)
		require.NoErrorf(t, err, "GetActiveOrders must not error for %s %s", req.AssetType, req.Pairs)
		if !mockTests {
			continue
		}
		require.Lenf(t, orders, 1, "GetActiveOrders must only return orders for %s", req.AssetType)
		o := orders[0]
		assert.Equal(t, req.AssetType, o.AssetType, "AssetType should be correct")
		if req.AssetType == asset.Spot {
			assert.Equal(t, order.Detail{
				Exchange:        e.Name,
				OrderID:         "1234123412341234",
				ClientOrderID:   "my-order-123",
				Type:            order.Limit,
				Side:            order.Buy,
				Status:          order.PartiallyFilled,
				AssetType:       asset.Spot,
				Pair:            currency.NewPairWithDelimiter("BTC", "USD", "/"),
				Date:            time.Date(2022, 1, 31, 14, 43, 15, 0, time.UTC),
				Price:           100,
				Amount:          0.5,
				ExecutedAmount:  0.09999999999999998,
				RemainingAmount: 0.4,
			}, o, "spot order should be correct")
			continue
		}
		assert.Equal(t, "1234123412341235", o.OrderID, "OrderID should be correct")
		assert.Equal(t, order.StopLimit, o.Type, "Type should be correct")
		assert.Equal(t, order.Open, o.Status, "Status should be correct")
		assert.Equal(t, margin.Multi, o.MarginType, "MarginType should be correct")
		assert.Equal(t, 3.0, o.Leverage, "Leverage should be correct")
		assert.Equal(t, 84500.0, o.TriggerPrice, "TriggerPrice should be correct")
		assert.True(t, o.ReduceOnly, "ReduceOnly should be correct")
	}
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderHistory(t.Context(), nil)
	assert.ErrorIs(t, err, order.ErrGetOrdersRequestIsNil, "GetOrderHistory should error on a nil request")
	_, err = e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{AssetType: asset.Futures, Side: order.AnySide, Type: order.AnyType})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetOrderHistory should error on an unsupported asset")

	skipIfLiveWithoutCredentials(t)
	orders, err := e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{AssetType: asset.Spot, Side: order.AnySide, Type: order.AnyType})
	require.NoError(t, err, "GetOrderHistory must not error for spot")
	if mockTests {
		require.Len(t, orders, 2, "GetOrderHistory must aggregate the trades into orders")
		o := orders[0]
		assert.Equal(t, "1458532827766784", o.OrderID, "OrderID should be correct")
		assert.Equal(t, order.Buy, o.Side, "Side should be correct")
		assert.True(t, o.Pair.Equal(spotPair), "Pair should be correct")
		assert.InDelta(t, 0.2, o.ExecutedAmount, 1e-9, "ExecutedAmount should be correct")
		assert.InDelta(t, 20050, o.AverageExecutedPrice, 1e-9, "AverageExecutedPrice should be correct")
		assert.InDelta(t, 16.04, o.Fee, 1e-9, "Fee should be correct")
		assert.True(t, o.FeeAsset.Equal(currency.USD), "FeeAsset should be correct")
		assert.Equal(t, time.Date(2022, 3, 1, 10, 54, 53, 849000000, time.UTC), o.Date, "Date should be the first trade")
		assert.Equal(t, time.Date(2022, 3, 1, 10, 55, 53, 0, time.UTC), o.LastUpdated, "LastUpdated should be the last trade")
		assert.Len(t, o.Trades, 2, "Trades should contain the order's trades")
		assert.Equal(t, order.Sell, orders[1].Side, "Side should be derived from the base amount")
		assert.True(t, orders[1].Pair.Equal(currency.NewPair(currency.ETH, currency.USD)), "Pair should be derived from the exchange rate")
	}

	orders, err = e.GetOrderHistory(t.Context(), &order.MultiOrderRequest{AssetType: asset.PerpetualContract, Side: order.AnySide, Type: order.AnyType, Pairs: currency.Pairs{perpetualPair}})
	require.NoError(t, err, "GetOrderHistory must not error for perpetual contracts")
	if mockTests {
		require.Len(t, orders, 2, "GetOrderHistory must aggregate the trades into orders")
		o := orders[1]
		assert.Equal(t, "1234123412341235", o.OrderID, "OrderID should be correct")
		assert.Equal(t, order.Sell, o.Side, "Side should be correct")
		assert.Equal(t, asset.PerpetualContract, o.AssetType, "AssetType should be correct")
		assert.InDelta(t, 0.01, o.ExecutedAmount, 1e-9, "ExecutedAmount should be correct")
		assert.InDelta(t, 84006, o.AverageExecutedPrice, 1e-9, "AverageExecutedPrice should be correct")
		assert.InDelta(t, 0.33602, o.Fee, 1e-9, "Fee should be correct")
		assert.Equal(t, margin.Isolated, o.MarginType, "MarginType should be correct")
		assert.Equal(t, 3.0, o.Leverage, "Leverage should be correct")
	}
}

func TestWithinHistoryLimit(t *testing.T) {
	t.Parallel()
	assert.Zero(t, withinHistoryLimit(time.Time{}), "zero times should remain zero")
	assert.Zero(t, withinHistoryLimit(time.Now().Add(-maxHistoryAge)), "times outside the retention period should be omitted")
	recent := time.Now().Add(-time.Hour)
	assert.Equal(t, recent, withinHistoryLimit(recent), "times inside the retention period should be retained")
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	start := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 9, 30, 0, 0, 0, 0, time.UTC)
	for a, pair := range map[asset.Item]currency.Pair{asset.Spot: spotPair, asset.PerpetualContract: perpetualPair} {
		for name, get := range map[string]func() (*kline.Item, error){
			"GetHistoricCandles": func() (*kline.Item, error) {
				return e.GetHistoricCandles(t.Context(), pair, a, kline.OneDay, start, end)
			},
			"GetHistoricCandlesExtended": func() (*kline.Item, error) {
				return e.GetHistoricCandlesExtended(t.Context(), pair, a, kline.OneDay, start, end)
			},
		} {
			candles, err := get()
			require.NoErrorf(t, err, "%s must not error for %s", name, pair)
			assert.Truef(t, candles.Pair.Equal(pair), "%s Pair should be correct for %s", name, pair)
			require.NotEmptyf(t, candles.Candles, "%s must return candles for %s", name, pair)
			for _, c := range candles.Candles {
				assert.Falsef(t, c.Time.Before(start) || c.Time.After(end), "%s candles should be within the requested range for %s", name, pair)
				assert.Positivef(t, c.Close, "%s Close should be positive for %s", name, pair)
			}
		}
	}
}

func TestGetServerTime(t *testing.T) {
	t.Parallel()
	_, err := e.GetServerTime(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, common.ErrFunctionNotSupported, "GetServerTime should not be supported")
}

func TestGetFuturesContractDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesContractDetails(t.Context(), asset.Spot)
	assert.ErrorIs(t, err, futures.ErrNotFuturesAsset, "GetFuturesContractDetails should error on a spot asset")
	_, err = e.GetFuturesContractDetails(t.Context(), asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetFuturesContractDetails should error on an unsupported asset")

	contracts, err := e.GetFuturesContractDetails(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "GetFuturesContractDetails must not error")
	var found bool
	for i := range contracts {
		c := &contracts[i]
		assert.Equal(t, futures.Perpetual, c.Type, "Type should be correct")
		assert.Equal(t, asset.PerpetualContract, c.Asset, "Asset should be correct")
		if !c.Name.Equal(perpetualPair) {
			continue
		}
		found = true
		assert.True(t, c.Underlying.Equal(spotPair), "Underlying should be correct")
		assert.True(t, c.SettlementCurrency.Equal(currency.USD), "SettlementCurrency should be correct")
		assert.Equal(t, futures.Linear, c.SettlementType, "SettlementType should be correct")
		assert.True(t, c.IsActive, "IsActive should be correct")
		assert.Positive(t, c.MaxLeverage, "MaxLeverage should be positive")
		assert.Equal(t, 1.0, c.Multiplier, "Multiplier should be correct")
	}
	assert.True(t, found, "GetFuturesContractDetails should return the BTC/USD-PERP contract")
}

func TestGetLatestFundingRates(t *testing.T) {
	t.Parallel()
	_, err := e.GetLatestFundingRates(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer, "GetLatestFundingRates should error on a nil request")
	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.Spot, Pair: spotPair})
	assert.ErrorIs(t, err, futures.ErrNotPerpetualFuture, "GetLatestFundingRates should error on a spot asset")

	for _, req := range []*fundingrate.LatestRateRequest{
		{Asset: asset.PerpetualContract, Pair: perpetualPair, IncludePredictedRate: true},
		{Asset: asset.PerpetualContract},
	} {
		rates, err := e.GetLatestFundingRates(t.Context(), req)
		require.NoError(t, err, "GetLatestFundingRates must not error")
		require.NotEmpty(t, rates, "GetLatestFundingRates must return rates")
		r := rates[0]
		assert.True(t, r.Pair.Equal(perpetualPair), "Pair should be correct")
		assert.Equal(t, asset.PerpetualContract, r.Asset, "Asset should be correct")
		assert.NotZero(t, r.LatestRate.Time, "LatestRate Time should be set")
		assert.True(t, r.TimeOfNextRate.After(r.LatestRate.Time), "TimeOfNextRate should be after the latest rate")
		if req.IncludePredictedRate {
			assert.Equal(t, r.TimeOfNextRate, r.PredictedUpcomingRate.Time, "PredictedUpcomingRate should apply at the next funding time")
			assert.True(t, r.PredictedUpcomingRate.Rate.Equal(r.LatestRate.Rate), "PredictedUpcomingRate should be the current rate")
		} else {
			assert.Zero(t, r.PredictedUpcomingRate, "PredictedUpcomingRate should not be set")
		}
	}
}

func TestGetHistoricalFundingRates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *fundingrate.HistoricalRatesRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &fundingrate.HistoricalRatesRequest{Asset: asset.Spot, Pair: spotPair}, err: futures.ErrNotPerpetualFuture},
		{req: &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract}, err: currency.ErrCurrencyPairEmpty},
		{req: &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, IncludePayments: true}, err: common.ErrFunctionNotSupported},
		{req: &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: time.Now(), EndDate: time.Now().Add(-time.Hour)}, err: common.ErrStartAfterEnd},
		{req: &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, StartDate: time.Now().Add(-maxHistoryAge)}, err: fundingrate.ErrFundingRateOutsideLimits},
	} {
		_, err := e.GetHistoricalFundingRates(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetHistoricalFundingRates should return the correct error")
	}

	rates, err := e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{Asset: asset.PerpetualContract, Pair: perpetualPair})
	require.NoError(t, err, "GetHistoricalFundingRates must not error")
	require.NotEmpty(t, rates.FundingRates, "GetHistoricalFundingRates must return rates")
	assert.True(t, rates.Pair.Equal(perpetualPair), "Pair should be correct")
	assert.Equal(t, rates.FundingRates[0].Time, rates.StartDate, "StartDate should be the first rate")
	assert.Equal(t, rates.FundingRates[len(rates.FundingRates)-1], rates.LatestRate, "LatestRate should be the last rate")
	for i := 1; i < len(rates.FundingRates); i++ {
		assert.Equal(t, 8*time.Hour, rates.FundingRates[i].Time.Sub(rates.FundingRates[i-1].Time), "rates should be eight hours apart")
	}
}

func TestIsPerpetualFutureCurrency(t *testing.T) {
	t.Parallel()
	for a, exp := range map[asset.Item]bool{asset.Spot: false, asset.PerpetualContract: true, asset.Futures: false} {
		is, err := e.IsPerpetualFutureCurrency(a, perpetualPair)
		require.NoErrorf(t, err, "IsPerpetualFutureCurrency must not error for %s", a)
		assert.Equalf(t, exp, is, "IsPerpetualFutureCurrency should be correct for %s", a)
	}
}

func TestGetCollateralCurrencyForContract(t *testing.T) {
	t.Parallel()
	_, _, err := e.GetCollateralCurrencyForContract(asset.Spot, spotPair)
	assert.ErrorIs(t, err, futures.ErrNotPerpetualFuture, "GetCollateralCurrencyForContract should error on a spot asset")
	_, _, err = e.GetCollateralCurrencyForContract(asset.PerpetualContract, currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetCollateralCurrencyForContract should error on an empty pair")

	c, a, err := e.GetCollateralCurrencyForContract(asset.PerpetualContract, perpetualPair)
	require.NoError(t, err, "GetCollateralCurrencyForContract must not error")
	assert.True(t, c.Equal(currency.USD), "GetCollateralCurrencyForContract should return the settlement currency")
	assert.Equal(t, asset.PerpetualContract, a, "GetCollateralCurrencyForContract should return the asset")
}

func TestGetOpenInterest(t *testing.T) {
	t.Parallel()
	_, err := e.GetOpenInterest(t.Context(), key.PairAsset{Base: currency.BTC.Item, Quote: currency.USD.Item, Asset: asset.Spot})
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetOpenInterest should error on a spot asset")

	interest, err := e.GetOpenInterest(t.Context())
	require.NoError(t, err, "GetOpenInterest must not error")
	require.NotEmpty(t, interest, "GetOpenInterest must return open interest")
	for i := range interest {
		assert.Equal(t, asset.PerpetualContract, interest[i].Key.Asset, "Asset should be correct")
		assert.Positive(t, interest[i].OpenInterest, "OpenInterest should be positive")
	}

	interest, err = e.GetOpenInterest(t.Context(), key.PairAsset{Base: perpetualPair.Base.Item, Quote: perpetualPair.Quote.Item, Asset: asset.PerpetualContract})
	require.NoError(t, err, "GetOpenInterest must not error for a pair")
	require.Len(t, interest, 1, "GetOpenInterest must return the pair's open interest")
	assert.True(t, interest[0].Key.Pair().Equal(perpetualPair), "Pair should be correct")
}

func TestSetLeverage(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, e.SetLeverage(t.Context(), asset.Spot, spotPair, margin.Isolated, 5, order.Buy), futures.ErrNotPerpetualFuture, "SetLeverage should error on a spot asset")
	assert.ErrorIs(t, e.SetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, margin.SpotIsolated, 5, order.Buy), margin.ErrMarginTypeUnsupported, "SetLeverage should error on an unsupported margin type")

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	assert.NoError(t, e.SetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, margin.Isolated, 5, order.Buy), "SetLeverage should not error")
}

func TestGetLeverage(t *testing.T) {
	t.Parallel()
	_, err := e.GetLeverage(t.Context(), asset.Spot, spotPair, margin.Isolated, order.Buy)
	assert.ErrorIs(t, err, futures.ErrNotPerpetualFuture, "GetLeverage should error on a spot asset")
	_, err = e.GetLeverage(t.Context(), asset.PerpetualContract, currency.EMPTYPAIR, margin.Isolated, order.Buy)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetLeverage should error on an empty pair")
	_, err = e.GetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, margin.NoMargin, order.Buy)
	assert.ErrorIs(t, err, margin.ErrMarginTypeUnsupported, "GetLeverage should error on an unsupported margin type")

	skipIfLiveWithoutCredentials(t)
	for marginType, exp := range map[margin.Type]float64{margin.Isolated: 5, margin.Multi: 3, margin.Unset: 3} {
		leverage, err := e.GetLeverage(t.Context(), asset.PerpetualContract, perpetualPair, marginType, order.Buy)
		require.NoErrorf(t, err, "GetLeverage must not error for %s", marginType)
		if mockTests {
			assert.Equalf(t, exp, leverage, "GetLeverage should return the leverage for %s", marginType)
		}
	}
}

func TestGetFuturesPositionSummary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *futures.PositionSummaryRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &futures.PositionSummaryRequest{Asset: asset.Spot, Pair: spotPair}, err: futures.ErrNotPerpetualFuture},
		{req: &futures.PositionSummaryRequest{Asset: asset.PerpetualContract}, err: currency.ErrCurrencyPairEmpty},
	} {
		_, err := e.GetFuturesPositionSummary(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetFuturesPositionSummary should return the correct error")
	}

	skipIfLiveWithoutCredentials(t)
	summary, err := e.GetFuturesPositionSummary(t.Context(), &futures.PositionSummaryRequest{Asset: asset.PerpetualContract, Pair: perpetualPair})
	if !mockTests {
		assert.ErrorIs(t, err, futures.ErrNoPositionsFound, "GetFuturesPositionSummary should error without a position")
		return
	}
	require.NoError(t, err, "GetFuturesPositionSummary must not error")
	assert.Equal(t, margin.Isolated, summary.MarginType, "isolated positions should be preferred")
	assert.True(t, summary.Currency.Equal(currency.USD), "Currency should be correct")
	assert.Equal(t, -0.01, summary.CurrentSize.InexactFloat64(), "CurrentSize should be negative for short positions")
	assert.Equal(t, 3.0, summary.Leverage.InexactFloat64(), "Leverage should be correct")
	assert.Equal(t, 55000.0, summary.EstimatedLiquidationPrice.InexactFloat64(), "EstimatedLiquidationPrice should be correct")
	assert.Equal(t, 80000.0, summary.AverageOpenPrice.InexactFloat64(), "AverageOpenPrice should be correct")
	assert.Equal(t, 40.0, summary.UnrealisedPNL.InexactFloat64(), "UnrealisedPNL should be correct")
	assert.Equal(t, futures.Linear, summary.ContractSettlementType, "ContractSettlementType should be correct")
}

func TestSelectPosition(t *testing.T) {
	t.Parallel()
	positions := []PositionResponse{
		{ID: "1", Market: "ETH/USD-PERP", MarginMode: MarginModeIsolated},
		{ID: "2", Market: "BTC/USD-PERP", MarginMode: MarginModeCross},
	}
	p, err := selectPosition(positions, "BTC/USD-PERP")
	require.NoError(t, err, "selectPosition must not error")
	assert.Equal(t, "2", p.ID, "selectPosition should select the market's position")
	_, err = selectPosition(positions, "SOL/USD-PERP")
	assert.ErrorIs(t, err, futures.ErrNoPositionsFound, "selectPosition should error without a position")
}

func TestGetFuturesPositionOrders(t *testing.T) {
	t.Parallel()
	start := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	end := start.Add(12 * time.Hour)
	for _, tc := range []struct {
		req *futures.PositionsRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &futures.PositionsRequest{Asset: asset.Spot}, err: futures.ErrNotPerpetualFuture},
		{req: &futures.PositionsRequest{Asset: asset.PerpetualContract}, err: currency.ErrCurrencyPairsEmpty},
		{req: &futures.PositionsRequest{Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}}, err: common.ErrDateUnset},
		{req: &futures.PositionsRequest{Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, StartDate: time.Now().Add(-maxHistoryAge), EndDate: end}, err: futures.ErrOrderHistoryTooLarge},
	} {
		_, err := e.GetFuturesPositionOrders(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "GetFuturesPositionOrders should return the correct error")
	}

	ex := newTestExchange(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/trade_history/btcusd-perp/", r.URL.Path, "path should be correct")
		assert.Equal(t, strconv.FormatInt(start.Unix(), 10), r.URL.Query().Get("since_timestamp"), "since_timestamp should be the start date")
		assert.Equal(t, strconv.FormatInt(end.Unix(), 10), r.URL.Query().Get("until_timestamp"), "until_timestamp should be the end date")
		_, err := w.Write([]byte(`[{"trade_id":"1","order_id":"2","datetime":"2025-06-16 10:01:00","fee":"0.1","fee_currency":"USD","market":"BTC/USD-PERP","margin_mode":"CROSS","leverage":"2","side":"BUY","type":"TRADE","price":"84000","amount":"0.01"}]`))
		assert.NoError(t, err, "Write should not error")
	})
	resp, err := ex.GetFuturesPositionOrders(t.Context(), &futures.PositionsRequest{Asset: asset.PerpetualContract, Pairs: currency.Pairs{perpetualPair}, StartDate: start, EndDate: end})
	require.NoError(t, err, "GetFuturesPositionOrders must not error")
	require.Len(t, resp, 1, "GetFuturesPositionOrders must return a response per pair")
	assert.Equal(t, futures.Linear, resp[0].ContractSettlementType, "ContractSettlementType should be correct")
	require.Len(t, resp[0].Orders, 1, "Orders must contain the aggregated order")
	assert.Equal(t, "2", resp[0].Orders[0].OrderID, "OrderID should be correct")
	assert.Equal(t, margin.Multi, resp[0].Orders[0].MarginType, "MarginType should be correct")
}

func TestChangePositionMargin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		req *margin.PositionChangeRequest
		err error
	}{
		{req: nil, err: common.ErrNilPointer},
		{req: &margin.PositionChangeRequest{Asset: asset.Spot}, err: futures.ErrNotPerpetualFuture},
		{req: &margin.PositionChangeRequest{Asset: asset.PerpetualContract}, err: currency.ErrCurrencyPairEmpty},
		{req: &margin.PositionChangeRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, MarginType: margin.Multi}, err: margin.ErrMarginTypeUnsupported},
		{req: &margin.PositionChangeRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, MarginType: margin.Isolated}, err: margin.ErrNewAllocatedMarginRequired},
	} {
		_, err := e.ChangePositionMargin(t.Context(), tc.req)
		assert.ErrorIs(t, err, tc.err, "ChangePositionMargin should return the correct error")
	}

	skipIfLiveWithoutCredentials(t, canManipulateRealOrders)
	resp, err := e.ChangePositionMargin(t.Context(), &margin.PositionChangeRequest{Asset: asset.PerpetualContract, Pair: perpetualPair, MarginType: margin.Isolated, NewAllocatedMargin: 12.5})
	if !mockTests {
		assert.ErrorIs(t, err, futures.ErrNoPositionsFound, "ChangePositionMargin should error without a position")
		return
	}
	require.NoError(t, err, "ChangePositionMargin must not error")
	assert.Equal(t, 12.5, resp.AllocatedMargin, "AllocatedMargin should be correct")
	assert.Equal(t, margin.Isolated, resp.MarginType, "MarginType should be correct")
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, true)
		require.NoErrorf(t, err, "GetPairs must not error for %s", a)
		require.NotEmptyf(t, pairs, "enabled pairs must not be empty for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		require.NoErrorf(t, err, "GetCurrencyTradeURL must not error for %s", a)
		assert.Equalf(t, tradeBaseURL+formatMarketSymbol(pairs[0])+"/", resp, "GetCurrencyTradeURL should return the market's URL for %s", a)
	}
	_, err := e.GetCurrencyTradeURL(t.Context(), asset.Futures, spotPair)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "GetCurrencyTradeURL should error on an unsupported asset")
}

func TestMarketTypeForAsset(t *testing.T) {
	t.Parallel()
	for a, exp := range map[asset.Item]string{asset.Spot: MarketTypeSpot, asset.PerpetualContract: MarketTypePerpetual} {
		marketType, err := marketTypeForAsset(a)
		require.NoErrorf(t, err, "marketTypeForAsset must not error for %s", a)
		assert.Equalf(t, exp, marketType, "marketTypeForAsset should be correct for %s", a)
	}
	_, err := marketTypeForAsset(asset.Futures)
	assert.ErrorIs(t, err, asset.ErrNotSupported, "marketTypeForAsset should error on an unsupported asset")
}

func TestMarketPairAsset(t *testing.T) {
	t.Parallel()
	for market, exp := range map[string]asset.Item{"BTC/USD": asset.Spot, "BTC/USD-PERP": asset.PerpetualContract} {
		pair, a, err := marketPairAsset(market)
		require.NoErrorf(t, err, "marketPairAsset must not error for %s", market)
		assert.Equalf(t, exp, a, "marketPairAsset should return the asset for %s", market)
		assert.Equalf(t, market, pair.String(), "marketPairAsset should return the pair for %s", market)
	}
	_, _, err := marketPairAsset("BTCUSD")
	assert.Error(t, err, "marketPairAsset should error on a malformed market")
}

func TestMarginModes(t *testing.T) {
	t.Parallel()
	for marginType, exp := range map[margin.Type]string{margin.Unset: MarginModeCross, margin.Multi: MarginModeCross, margin.Isolated: MarginModeIsolated} {
		mode, err := formatMarginMode(marginType)
		require.NoErrorf(t, err, "formatMarginMode must not error for %s", marginType)
		assert.Equalf(t, exp, mode, "formatMarginMode should be correct for %s", marginType)
	}
	_, err := formatMarginMode(margin.NoMargin)
	assert.ErrorIs(t, err, margin.ErrMarginTypeUnsupported, "formatMarginMode should error on an unsupported margin type")

	for mode, exp := range map[string]margin.Type{MarginModeCross: margin.Multi, MarginModeIsolated: margin.Isolated, "": margin.Unset} {
		assert.Equalf(t, exp, marginTypeFromMode(mode), "marginTypeFromMode should be correct for %q", mode)
	}
}

func TestFormatTrigger(t *testing.T) {
	t.Parallel()
	for priceType, exp := range map[order.PriceType]string{order.LastPrice: TriggerLastTradedPrice, order.IndexPrice: TriggerIndexPrice, order.MarkPrice: TriggerMarkPrice} {
		assert.Equalf(t, exp, formatTrigger(priceType), "formatTrigger should be correct for %s", priceType)
	}
}

func TestOrderTypeFromSubtype(t *testing.T) {
	t.Parallel()
	for subtype, exp := range map[string]order.Type{
		"":                                  order.Limit,
		OrderSubtypeLimit:                   order.Limit,
		OrderSubtypeMarket:                  order.Market,
		OrderSubtypeInstant:                 order.Market,
		OrderSubtypeCash:                    order.Market,
		OrderSubtypeStopMarket:              order.StopMarket,
		OrderSubtypeStopLoss:                order.StopMarket,
		OrderSubtypeStopLimit:               order.StopLimit,
		OrderSubtypeStopLossLimit:           order.StopLimit,
		OrderSubtypeTakeProfit:              order.TakeProfitMarket,
		OrderSubtypeTakeProfitLimit:         order.TakeProfit | order.Limit,
		OrderSubtypeTrailingStopLoss:        order.TrailingStop,
		OrderSubtypeTrailingTakeProfit:      order.TrailingStop,
		OrderSubtypeTrailingStopLossLimit:   order.TrailingStopLimit,
		OrderSubtypeTrailingTakeProfitLimit: order.TrailingStopLimit,
		"UNKNOWN":                           order.UnknownType,
	} {
		assert.Equalf(t, exp, orderTypeFromSubtype(subtype), "orderTypeFromSubtype should be correct for %q", subtype)
	}
}
