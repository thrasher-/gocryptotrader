package gateio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func TestTransferCollateralToIsolatedMargin(t *testing.T) {
	t.Parallel()
	_, err := e.TransferCollateralToIsolatedMargin(t.Context(), BTCUSDT, currency.EMPTYCODE, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "empty currency code must return the expected error")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.TransferCollateralToIsolatedMargin(t.Context(), BTCUSDT, currency.USDT, -10)
	require.NoError(t, err, "TransferCollateralToIsolatedMargin must not error")
}

func TestTransferCollateralFromIsolatedMargin(t *testing.T) {
	t.Parallel()
	_, err := e.TransferCollateralFromIsolatedMargin(t.Context(), BTCUSDT, currency.EMPTYCODE, 10)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "empty currency code must return the expected error")

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.TransferCollateralFromIsolatedMargin(t.Context(), BTCUSDT, currency.USDT, -1)
	require.NoError(t, err, "TransferCollateralFromIsolatedMargin must not error")
}

func TestGetIsolatedMarginAccountBalanceChangeHistory(t *testing.T) {
	t.Parallel()

	tn := time.Now()
	_, err := e.GetIsolatedMarginAccountBalanceChangeHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, tn.Add(time.Hour), tn, 0, 0, "")
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "start time after end time must return the expected error")

	_, err = e.GetIsolatedMarginAccountBalanceChangeHistory(t.Context(), currency.BTC, BTCUSDT, time.Time{}, time.Time{}, 1, 100, "invalid")
	require.ErrorIs(t, err, errInvalidIsolatedMarginAccountType, "invalid account type must return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/account_book", `[{"id":"record-1","currency":"BTC","currency_pair":"BTC_USDT","type":"margin_out"}]`)
	records, err := ex.GetIsolatedMarginAccountBalanceChangeHistory(t.Context(), currency.BTC, BTCUSDT, time.Time{}, time.Time{}, 1, 100, "margin_out")
	require.NoError(t, err, "GetIsolatedMarginAccountBalanceChangeHistory must not error")
	require.Len(t, records, 1, "GetIsolatedMarginAccountBalanceChangeHistory must decode one record")
	assert.Equal(t, "record-1", records[0].ID, "record ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/margin/account_book", gotRequest.path, "request path should match")
	assert.Equal(t, "BTC", gotRequest.query.Get("currency"), "currency should match")
	assert.Equal(t, BTCUSDT.String(), gotRequest.query.Get(gateIOTestCurrencyPairQueryKey), "currency pair should match")
	assert.Equal(t, "1", gotRequest.query.Get("page"), "page should match")
	assert.Equal(t, "100", gotRequest.query.Get(gateIOTestLimitQueryKey), "limit should match")
	assert.Equal(t, "margin_out", gotRequest.query.Get(gateIOTestTypeQueryKey), "account type should match")

	records, err = ex.GetIsolatedMarginAccountBalanceChangeHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, time.Time{}, time.Time{}, 0, 0, "")
	require.NoError(t, err, "GetIsolatedMarginAccountBalanceChangeHistory must accept omitted optional parameters")
	require.Len(t, records, 1, "GetIsolatedMarginAccountBalanceChangeHistory must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/margin/account_book", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginAccountBalanceChangeHistory(ctx, currency.BTC, BTCUSDT, time.Time{}, time.Time{}, 1, 100, "margin_out")
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginAccountBalanceChangeHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, time.Time{}, time.Time{}, 0, 0, "margin_out")
		require.NoError(t, err, "GetIsolatedMarginAccountBalanceChangeHistory must not error against the live API")
	})
}

func TestGetIsolatedMarginFundingAccountList(t *testing.T) {
	t.Parallel()

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/funding_accounts", `[{"currency":"BTC","available":"2"}]`)
	accounts, err := ex.GetIsolatedMarginFundingAccountList(t.Context(), currency.BTC)
	require.NoError(t, err, "GetIsolatedMarginFundingAccountList must not error")
	require.Len(t, accounts, 1, "GetIsolatedMarginFundingAccountList must decode one account")
	assert.Equal(t, "BTC", accounts[0].Currency, "account currency should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/margin/funding_accounts", gotRequest.path, "request path should match")
	assert.Equal(t, "BTC", gotRequest.query.Get("currency"), "currency should match")

	accounts, err = ex.GetIsolatedMarginFundingAccountList(t.Context(), currency.EMPTYCODE)
	require.NoError(t, err, "GetIsolatedMarginFundingAccountList must accept an omitted currency")
	require.Len(t, accounts, 1, "GetIsolatedMarginFundingAccountList must decode the response with an omitted currency")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "an empty currency should be omitted")

	requireGateIORequestErrors(t, "/api/v4/margin/funding_accounts", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginFundingAccountList(ctx, currency.BTC)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginFundingAccountList(t.Context(), currency.EMPTYCODE)
		require.NoError(t, err, "GetIsolatedMarginFundingAccountList must not error against the live API")
	})
}

func TestGetIsolatedMarginUserAutoRepaymentSetting(t *testing.T) {
	t.Parallel()

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/auto_repay", `{"status":"on"}`)
	status, err := ex.GetIsolatedMarginUserAutoRepaymentSetting(t.Context())
	require.NoError(t, err, "GetIsolatedMarginUserAutoRepaymentSetting must not error")
	require.NotNil(t, status, "GetIsolatedMarginUserAutoRepaymentSetting must decode a status")
	assert.Equal(t, "on", status.Status, "auto-repayment status should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/margin/auto_repay", gotRequest.path, "request path should match")

	requireGateIORequestErrors(t, "/api/v4/margin/auto_repay", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginUserAutoRepaymentSetting(ctx)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginUserAutoRepaymentSetting(t.Context())
		require.NoError(t, err, "GetIsolatedMarginUserAutoRepaymentSetting must not error against the live API")
	})
}

func TestUpdateIsolatedMarginUsersAutoRepaymentSetting(t *testing.T) {
	t.Parallel()

	ex, requests := setupGateIOHTTPTest(t, http.MethodPost, "/api/v4/margin/auto_repay", `{"status":"updated"}`)
	status, err := ex.UpdateIsolatedMarginUsersAutoRepaymentSetting(t.Context(), false)
	require.NoError(t, err, "UpdateIsolatedMarginUsersAutoRepaymentSetting must not error")
	require.NotNil(t, status, "UpdateIsolatedMarginUsersAutoRepaymentSetting must decode a status")
	assert.Equal(t, "updated", status.Status, "auto-repayment status should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodPost, gotRequest.method, "request method should be POST")
	assert.Equal(t, url.Values{"status": {"off"}}, gotRequest.query, "disabled status should match")
	status, err = ex.UpdateIsolatedMarginUsersAutoRepaymentSetting(t.Context(), true)
	require.NoError(t, err, "UpdateIsolatedMarginUsersAutoRepaymentSetting must not error")
	require.NotNil(t, status, "UpdateIsolatedMarginUsersAutoRepaymentSetting must decode a status")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{"status": {"on"}}, gotRequest.query, "enabled status should match")

	requireGateIORequestErrors(t, "/api/v4/margin/auto_repay", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.UpdateIsolatedMarginUsersAutoRepaymentSetting(ctx, true)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_AUTO_REPAYMENT_STATUS")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveAutoRepaymentStatus](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_AUTO_REPAYMENT_STATUS"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_AUTO_REPAYMENT_STATUS must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_AUTO_REPAYMENT_STATUS must set dedicated_test_account=true")
		prior, err := e.GetIsolatedMarginUserAutoRepaymentSetting(t.Context())
		require.NoError(t, err, "GetIsolatedMarginUserAutoRepaymentSetting must retrieve the prior live setting")
		require.NotNil(t, prior, "GetIsolatedMarginUserAutoRepaymentSetting must return the prior live setting")
		require.Contains(t, []string{"on", "off"}, prior.Status, "prior auto-repayment status must be on or off")
		readStatus := func(ctx context.Context) (string, error) {
			status, err := e.GetIsolatedMarginUserAutoRepaymentSetting(ctx)
			if err != nil {
				return "", err
			}
			if status == nil {
				return "", errors.New("auto-repayment status response is nil")
			}
			return status.Status, nil
		}
		applied := "off"
		if config.Enabled {
			applied = "on"
		}
		require.NotEqual(t, prior.Status, applied, "GCT_GATEIO_LIVE_AUTO_REPAYMENT_STATUS target must differ from the current setting")
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, reconcileGateIOLiveMutation(ctx, readStatus, func(ctx context.Context) error {
				_, err := e.UpdateIsolatedMarginUsersAutoRepaymentSetting(ctx, prior.Status == "on")
				return err
			}, prior.Status, applied, func(a, b string) bool { return a == b }, gateIOLiveReconciliationPollInterval, gateIOLiveReconciliationPollAttempts), "UpdateIsolatedMarginUsersAutoRepaymentSetting live cleanup should reconcile the prior setting")
		})
		_, err = e.UpdateIsolatedMarginUsersAutoRepaymentSetting(t.Context(), config.Enabled)
		require.NoError(t, err, "UpdateIsolatedMarginUsersAutoRepaymentSetting must not error against the live API")
	})
}

func TestGetIsolatedMarginMaxTransferableAmount(t *testing.T) {
	t.Parallel()

	_, err := e.GetIsolatedMarginMaxTransferableAmount(t.Context(), currency.EMPTYCODE, BTCUSDT)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "empty currency code must return the expected error")

	_, err = e.GetIsolatedMarginMaxTransferableAmount(t.Context(), currency.USDT, currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "empty currency pair must return the expected error")

	for _, pair := range []currency.Pair{
		currency.NewPair(currency.BTC, currency.BTC),
		currency.NewPair(currency.BTC, currency.EMPTYCODE),
	} {
		_, err = e.GetIsolatedMarginMaxTransferableAmount(t.Context(), currency.BTC, pair)
		require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "malformed currency pair must return the expected error")
	}

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/transferable", `{"currency":"BTC","currency_pair":"BTC_USDT","amount":"2"}`)
	amount, err := ex.GetIsolatedMarginMaxTransferableAmount(t.Context(), currency.BTC, BTCUSDT)
	require.NoError(t, err, "GetIsolatedMarginMaxTransferableAmount must not error")
	require.NotNil(t, amount, "GetIsolatedMarginMaxTransferableAmount must decode a response")
	assert.Equal(t, "BTC", amount.Currency, "response currency should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/margin/transferable", gotRequest.path, "request path should match")
	assert.Equal(t, "BTC", gotRequest.query.Get("currency"), "currency should match")
	assert.Equal(t, BTCUSDT.String(), gotRequest.query.Get(gateIOTestCurrencyPairQueryKey), "currency pair should match")

	amount, err = ex.GetIsolatedMarginMaxTransferableAmount(t.Context(), currency.BTC, currency.EMPTYPAIR)
	require.NoError(t, err, "GetIsolatedMarginMaxTransferableAmount must accept an omitted pair for a base currency")
	require.NotNil(t, amount, "GetIsolatedMarginMaxTransferableAmount must decode the response with an omitted pair")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{"currency": {"BTC"}}, gotRequest.query, "an empty currency pair should be omitted")

	requireGateIORequestErrors(t, "/api/v4/margin/transferable", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginMaxTransferableAmount(ctx, currency.BTC, BTCUSDT)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginMaxTransferableAmount(t.Context(), currency.USDT, BTCUSDT)
		require.NoError(t, err, "GetIsolatedMarginMaxTransferableAmount must not error against the live API")
	})
}

func TestIsolatedMarginLendingMarketIsTradable(t *testing.T) {
	t.Parallel()
	now := time.Unix(2_000_000_000, 0)

	for _, tc := range []struct {
		name         string
		status       string
		delistedTime time.Time
		expected     bool
	}{
		{name: "enabled without delisting", status: "enabled", expected: true},
		{name: "disabled", status: "disabled", expected: false},
		{name: "past delisting", status: "enabled", delistedTime: now.Add(-time.Second), expected: false},
		{name: "delisting now", status: "enabled", delistedTime: now, expected: true},
		{name: "future delisting", status: "enabled", delistedTime: now.Add(time.Second), expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			market := IsolatedMarginLendingMarket{Status: tc.status, DelistedTime: types.Time(tc.delistedTime)}
			assert.Equal(t, tc.expected, market.IsTradable(now), "tradability should match the market status and delisting time")
		})
	}
}

func TestGetIsolatedMarginLendingMarkets(t *testing.T) {
	t.Parallel()
	markets, err := e.GetIsolatedMarginLendingMarkets(t.Context())
	require.NoError(t, err, "GetIsolatedMarginLendingMarkets must not error")
	require.NotEmpty(t, markets, "GetIsolatedMarginLendingMarkets must return some markets")
}

func TestGetIsolatedMarginLendingMarketDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetIsolatedMarginLendingMarketDetails(t.Context(), currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "empty currency pair must return the expected error")

	market, err := e.GetIsolatedMarginLendingMarketDetails(t.Context(), BTCUSDT)
	require.NoError(t, err, "GetIsolatedMarginLendingMarketDetails must not error")
	require.NotNil(t, market, "GetIsolatedMarginLendingMarketDetails must return a market")
}

func TestGetIsolatedMarginEstimatedInterestRate(t *testing.T) {
	t.Parallel()

	_, err := e.GetIsolatedMarginEstimatedInterestRate(t.Context(), nil)
	require.ErrorIs(t, err, currency.ErrCurrencyCodesEmpty, "nil currencies must return the expected error")

	_, err = e.GetIsolatedMarginEstimatedInterestRate(t.Context(), currency.Currencies{currency.EMPTYCODE})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "empty currency code must return the expected error")

	_, err = e.GetIsolatedMarginEstimatedInterestRate(t.Context(), currency.Currencies{
		currency.USDT,
		currency.BTC,
		currency.ETH,
		currency.XRP,
		currency.LTC,
		currency.DOGE,
		currency.BCH,
		currency.SOL,
		currency.ADA,
		currency.DOT,
		currency.MATIC,
	})
	require.ErrorIs(t, err, errTooManyCurrencyCodes, "too many currency codes must return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/uni/estimate_rate", `{"BTC":"1","USDT":"2"}`)
	got, err := ex.GetIsolatedMarginEstimatedInterestRate(t.Context(), currency.Currencies{currency.BTC, currency.USDT})
	require.NoError(t, err, "GetIsolatedMarginEstimatedInterestRate must not error")
	val, ok := got["BTC"]
	require.True(t, ok, "result map must contain BTC key")
	require.Positive(t, val.Float64(), "estimated interest rate must not be 0")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "BTC,USDT", gotRequest.query.Get("currencies"), "currencies should match")

	requireGateIORequestErrors(t, "/api/v4/margin/uni/estimate_rate", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginEstimatedInterestRate(ctx, currency.Currencies{currency.BTC, currency.USDT})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		got, err := e.GetIsolatedMarginEstimatedInterestRate(t.Context(), currency.Currencies{currency.BTC, currency.USDT})
		require.NoError(t, err, "GetIsolatedMarginEstimatedInterestRate must not error against the live API")
		require.Contains(t, got, "BTC", "GetIsolatedMarginEstimatedInterestRate must return BTC against the live API")
	})
}

func TestGetIsolatedMarginLoans(t *testing.T) {
	t.Parallel()

	_, err := e.GetIsolatedMarginLoans(t.Context(), currency.BTC, currency.NewBTCUSD(), 0, 101)
	require.ErrorIs(t, err, errInvalidLimit, "limit above maximum must return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/uni/loans", `[{"type":"borrow","currency_pair":"BTC_USDT","currency":"BTC","amount":"2"}]`)
	loans, err := ex.GetIsolatedMarginLoans(t.Context(), currency.BTC, BTCUSDT, 2, 100)
	require.NoError(t, err, "GetIsolatedMarginLoans must not error")
	require.Len(t, loans, 1, "GetIsolatedMarginLoans must decode one loan")
	assert.Equal(t, currency.BTC, loans[0].Currency, "loan currency should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "BTC", gotRequest.query.Get("currency"), "currency should match")
	assert.Equal(t, BTCUSDT.String(), gotRequest.query.Get(gateIOTestCurrencyPairQueryKey), "currency pair should match")
	assert.Equal(t, "2", gotRequest.query.Get("page"), "page should match")
	assert.Equal(t, "100", gotRequest.query.Get(gateIOTestLimitQueryKey), "limit should match")

	loans, err = ex.GetIsolatedMarginLoans(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, 0, 0)
	require.NoError(t, err, "GetIsolatedMarginLoans must accept omitted optional parameters")
	require.Len(t, loans, 1, "GetIsolatedMarginLoans must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/margin/uni/loans", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginLoans(ctx, currency.BTC, BTCUSDT, 2, 100)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginLoans(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, 0, 0)
		require.NoError(t, err, "GetIsolatedMarginLoans must not error against the live API")
	})
}

func TestIsolatedMarginBorrowOrRepay(t *testing.T) {
	t.Parallel()

	assert.ErrorIs(t, e.IsolatedMarginBorrowOrRepay(t.Context(), nil), errNilArgument, "nil request should return the expected error")
	assert.ErrorIs(t, e.IsolatedMarginBorrowOrRepay(t.Context(), &IsolatedBorrowRepayRequest{
		Currency: currency.BTC,
		Type:     "borrow",
		Amount:   1,
	}), currency.ErrCurrencyPairEmpty, "empty currency pair should return the expected error")
	assert.ErrorIs(t, e.IsolatedMarginBorrowOrRepay(t.Context(), &IsolatedBorrowRepayRequest{
		CurrencyPair: BTCUSDT,
		Type:         "borrow",
		Amount:       1,
	}), currency.ErrCurrencyCodeEmpty, "empty currency code should return the expected error")
	assert.ErrorIs(t, e.IsolatedMarginBorrowOrRepay(t.Context(), &IsolatedBorrowRepayRequest{
		CurrencyPair: BTCUSDT,
		Currency:     currency.BTC,
		Type:         "invalid",
		Amount:       1,
	}), errInvalidIsolatedMarginLoanType, "invalid loan type should return the expected error")
	assert.ErrorIs(t, e.IsolatedMarginBorrowOrRepay(t.Context(), &IsolatedBorrowRepayRequest{
		CurrencyPair: BTCUSDT,
		Currency:     currency.BTC,
		Type:         "borrow",
		Amount:       0,
	}), errInvalidAmount, "zero amount should return the expected error")
	assert.ErrorIs(t, e.IsolatedMarginBorrowOrRepay(t.Context(), &IsolatedBorrowRepayRequest{
		CurrencyPair: BTCUSDT,
		Currency:     currency.BTC,
		Type:         "repay",
		Amount:       1,
		RepaidAll:    true,
	}), errAmountOverriddenByRepaidAll, "amount with full repayment should return the expected error")
	assert.ErrorIs(t, e.IsolatedMarginBorrowOrRepay(t.Context(), &IsolatedBorrowRepayRequest{
		CurrencyPair: BTCUSDT,
		Currency:     currency.BTC,
		Type:         "borrow",
		RepaidAll:    true,
	}), errInvalidRepaidAllOperation, "full repayment on a borrow should return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodPost, "/api/v4/margin/uni/loans", "", http.StatusNoContent)
	require.NoError(t, ex.IsolatedMarginBorrowOrRepay(t.Context(), &IsolatedBorrowRepayRequest{
		CurrencyPair: BTCUSDT,
		Currency:     currency.BTC,
		Type:         "repay",
		Amount:       0.00004,
	}), "IsolatedMarginBorrowOrRepay must not error")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodPost, gotRequest.method, "request method should be POST")
	assert.Equal(t, "/api/v4/margin/uni/loans", gotRequest.path, "request path should match")
	assert.JSONEq(t, `{"currency_pair":"BTC_USDT","currency":"BTC","type":"repay","amount":"0.00004"}`, string(gotRequest.body), "request body should match")

	requireGateIORequestErrors(t, "/api/v4/margin/uni/loans", false, func(ctx context.Context, ex *Exchange) error {
		return ex.IsolatedMarginBorrowOrRepay(ctx, &IsolatedBorrowRepayRequest{
			CurrencyPair: BTCUSDT,
			Currency:     currency.BTC,
			Type:         "repay",
			Amount:       0.00004,
		})
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_ISOLATED_BORROW_OR_REPAY")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveIsolatedBorrowOrRepay](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_ISOLATED_BORROW_OR_REPAY"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_ISOLATED_BORROW_OR_REPAY must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_ISOLATED_BORROW_OR_REPAY must set dedicated_test_account=true")
		require.Equal(t, "borrow", config.Request.Type, "live isolated-margin mutation request must borrow")
		require.False(t, config.Request.CurrencyPair.IsEmpty(), "live isolated-margin mutation request pair must be set")
		require.False(t, config.Request.Currency.IsEmpty(), "live isolated-margin mutation request currency must be set")
		require.Positive(t, config.Request.Amount, "live isolated-margin mutation request amount must be positive")
		require.False(t, config.Request.RepaidAll, "live isolated-margin borrow request must not set full repayment")
		require.Equal(t, "repay", config.Restore.Type, "live isolated-margin restore request must repay")
		require.True(t, config.Restore.CurrencyPair.Equal(config.Request.CurrencyPair), "live isolated-margin restore pair must match the request")
		require.True(t, config.Restore.Currency.Equal(config.Request.Currency), "live isolated-margin restore currency must match the request")
		require.Equal(t, config.Request.Amount, config.Restore.Amount, "live isolated-margin restore amount must match the request")
		require.False(t, config.Restore.RepaidAll, "live isolated-margin restore request must repay only the test amount")
		readBorrowed := func(ctx context.Context) (types.Number, error) {
			accounts, err := e.GetIsolatedMarginAccountList(ctx, config.Request.CurrencyPair)
			if err != nil {
				return 0, err
			}
			for i := range accounts {
				if !accounts[i].CurrencyPair.Equal(config.Request.CurrencyPair) {
					continue
				}
				if config.Request.Currency.Equal(config.Request.CurrencyPair.Base) {
					return accounts[i].Base.Borrowed, nil
				}
				if config.Request.Currency.Equal(config.Request.CurrencyPair.Quote) {
					return accounts[i].Quote.Borrowed, nil
				}
			}
			return 0, fmt.Errorf("isolated margin borrowed balance for %s was not returned", config.Request.Currency)
		}
		before, err := readBorrowed(t.Context())
		require.NoError(t, err, "live isolated-margin mutation must snapshot outstanding principal")
		applied := before + config.Request.Amount
		equal := func(a, b types.Number) bool { return math.Abs(a.Float64()-b.Float64()) <= 1e-10 }
		require.False(t, equal(before, applied), "live isolated-margin mutation amount must produce a distinguishable principal change")
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, reconcileGateIOLiveMutation(ctx, readBorrowed, func(ctx context.Context) error {
				return e.IsolatedMarginBorrowOrRepay(ctx, &config.Restore)
			}, before, applied, equal, gateIOLiveReconciliationPollInterval, gateIOLiveReconciliationPollAttempts), "IsolatedMarginBorrowOrRepay live cleanup should reconcile outstanding principal")
		})
		err = e.IsolatedMarginBorrowOrRepay(t.Context(), &config.Request)
		require.NoError(t, err, "IsolatedMarginBorrowOrRepay must not error against the live API")
	})
}

func TestIsolatedBorrowRepayRequestFullRepayment(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(&IsolatedBorrowRepayRequest{
		CurrencyPair: BTCUSDT,
		Currency:     currency.BTC,
		Type:         "repay",
		RepaidAll:    true,
	})
	require.NoError(t, err, "marshalling a full repayment request must not error")
	assert.JSONEq(t, `{"currency_pair":"BTC_USDT","currency":"BTC","type":"repay","amount":"","repaid_all":true}`, string(payload), "full repayment request should include repaid_all")
}

func TestGetIsolatedMarginLoanRecords(t *testing.T) {
	t.Parallel()

	_, err := e.GetIsolatedMarginLoanRecords(t.Context(), currency.BTC, currency.NewBTCUSDT(), 0, 101, "")
	require.ErrorIs(t, err, errInvalidLimit, "limit above maximum must return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/uni/loan_records", `[{"type":"repay","currency_pair":"BTC_USDT","currency":"BTC","amount":"2"}]`)
	loans, err := ex.GetIsolatedMarginLoanRecords(t.Context(), currency.BTC, BTCUSDT, 2, 100, "repay")
	require.NoError(t, err, "GetIsolatedMarginLoanRecords must not error")
	require.Len(t, loans, 1, "GetIsolatedMarginLoanRecords must decode one loan")
	assert.Equal(t, "repay", loans[0].Type, "loan type should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "repay", gotRequest.query.Get(gateIOTestTypeQueryKey), "loan type should match")
	assert.Equal(t, "BTC", gotRequest.query.Get("currency"), "currency should match")
	assert.Equal(t, BTCUSDT.String(), gotRequest.query.Get(gateIOTestCurrencyPairQueryKey), "currency pair should match")
	assert.Equal(t, "2", gotRequest.query.Get("page"), "page should match")
	assert.Equal(t, "100", gotRequest.query.Get(gateIOTestLimitQueryKey), "limit should match")

	loans, err = ex.GetIsolatedMarginLoanRecords(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, 0, 0, "")
	require.NoError(t, err, "GetIsolatedMarginLoanRecords must accept omitted optional parameters")
	require.Len(t, loans, 1, "GetIsolatedMarginLoanRecords must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/margin/uni/loan_records", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginLoanRecords(ctx, currency.BTC, BTCUSDT, 2, 100, "repay")
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginLoanRecords(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, 0, 0, "")
		require.NoError(t, err, "GetIsolatedMarginLoanRecords must not error against the live API")
	})
}

func TestGetIsolatedMarginInterestDeductionRecords(t *testing.T) {
	t.Parallel()

	_, err := e.GetIsolatedMarginInterestDeductionRecords(t.Context(), currency.BTC, BTCUSDT, 0, 1001, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errInvalidLimit, "limit above maximum must return the expected error")
	tn := time.Now()
	_, err = e.GetIsolatedMarginInterestDeductionRecords(t.Context(), currency.BTC, BTCUSDT, 0, 0, tn.Add(time.Hour), tn)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "start time after end time must return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/uni/interest_records", `[{"currency":"BTC","currency_pair":"BTC_USDT","interest":"2","type":"margin"}]`)
	records, err := ex.GetIsolatedMarginInterestDeductionRecords(t.Context(), currency.BTC, BTCUSDT, 2, 1000, time.Time{}, time.Time{})
	require.NoError(t, err, "GetIsolatedMarginInterestDeductionRecords must not error")
	require.Len(t, records, 1, "GetIsolatedMarginInterestDeductionRecords must decode one record")
	assert.Equal(t, currency.BTC, records[0].Currency, "record currency should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, BTCUSDT.String(), gotRequest.query.Get(gateIOTestCurrencyPairQueryKey), "currency pair should match")
	assert.Equal(t, "BTC", gotRequest.query.Get("currency"), "currency should match")
	assert.Equal(t, "2", gotRequest.query.Get("page"), "page should match")
	assert.Equal(t, "1000", gotRequest.query.Get(gateIOTestLimitQueryKey), "limit should match")

	records, err = ex.GetIsolatedMarginInterestDeductionRecords(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
	require.NoError(t, err, "GetIsolatedMarginInterestDeductionRecords must accept omitted optional parameters")
	require.Len(t, records, 1, "GetIsolatedMarginInterestDeductionRecords must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/margin/uni/interest_records", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginInterestDeductionRecords(ctx, currency.BTC, BTCUSDT, 2, 1000, time.Time{}, time.Time{})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginInterestDeductionRecords(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
		require.NoError(t, err, "GetIsolatedMarginInterestDeductionRecords must not error against the live API")
	})
}

func TestGetIsolatedMarginMaxBorrowableAmount(t *testing.T) {
	t.Parallel()

	_, err := e.GetIsolatedMarginMaxBorrowableAmount(t.Context(), currency.EMPTYCODE, BTCUSDT)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "empty currency code must return the expected error")

	_, err = e.GetIsolatedMarginMaxBorrowableAmount(t.Context(), currency.BTC, currency.EMPTYPAIR)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "empty currency pair must return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/uni/borrowable", `{"currency":"BTC","currency_pair":"BTC_USDT","borrowable":"2"}`)
	amount, err := ex.GetIsolatedMarginMaxBorrowableAmount(t.Context(), currency.BTC, BTCUSDT)
	require.NoError(t, err, "GetIsolatedMarginMaxBorrowableAmount must not error")
	require.NotNil(t, amount, "GetIsolatedMarginMaxBorrowableAmount must decode a response")
	assert.Equal(t, currency.BTC, amount.Currency, "response currency should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/margin/uni/borrowable", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{"currency": {"BTC"}, gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "request query should match")

	requireGateIORequestErrors(t, "/api/v4/margin/uni/borrowable", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginMaxBorrowableAmount(ctx, currency.BTC, BTCUSDT)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginMaxBorrowableAmount(t.Context(), currency.BTC, BTCUSDT)
		require.NoError(t, err, "GetIsolatedMarginMaxBorrowableAmount must not error against the live API")
	})
}

func TestGetIsolatedMarginUserLeverageTiers(t *testing.T) {
	t.Parallel()

	_, err := e.GetIsolatedMarginUserLeverageTiers(t.Context(), currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "empty currency pair should return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/user/loan_margin_tiers", `[{"upper_limit":"2","mmr":"0.1","leverage":"3"}]`)
	tiers, err := ex.GetIsolatedMarginUserLeverageTiers(t.Context(), BTCUSDT)
	require.NoError(t, err, "GetIsolatedMarginUserLeverageTiers must not error")
	require.Len(t, tiers, 1, "GetIsolatedMarginUserLeverageTiers must decode one tier")
	assert.Equal(t, types.Number(3), tiers[0].MaximumPermissibleLeverage, "tier leverage should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/margin/user/loan_margin_tiers", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "request query should match")

	requireGateIORequestErrors(t, "/api/v4/margin/user/loan_margin_tiers", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginUserLeverageTiers(ctx, BTCUSDT)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		tiers, err := e.GetIsolatedMarginUserLeverageTiers(t.Context(), BTCUSDT)
		require.NoError(t, err, "GetIsolatedMarginUserLeverageTiers must not error against the live API")
		require.NotEmpty(t, tiers, "GetIsolatedMarginUserLeverageTiers must return tiers from the live API")
	})
}

func TestGetIsolatedMarginMarketLeverageTiers(t *testing.T) {
	t.Parallel()
	_, err := e.GetIsolatedMarginMarketLeverageTiers(t.Context(), currency.EMPTYPAIR)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "empty currency pair should return the expected error")

	tiers, err := e.GetIsolatedMarginMarketLeverageTiers(t.Context(), BTCUSDT)
	require.NoError(t, err, "GetIsolatedMarginMarketLeverageTiers must not error")
	require.NotEmpty(t, tiers, "GetIsolatedMarginMarketLeverageTiers must return some tiers")
}

func TestSetUserMarketLeverageMultiplier(t *testing.T) {
	t.Parallel()

	err := e.SetUserMarketLeverageMultiplier(t.Context(), currency.EMPTYPAIR, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "empty currency pair must return the expected error")
	err = e.SetUserMarketLeverageMultiplier(t.Context(), BTCUSDT, 0)
	require.ErrorIs(t, err, errInvalidLeverage, "zero leverage must return the expected error")

	ex, requests := setupGateIOHTTPTest(t, http.MethodPost, "/api/v4/margin/leverage/user_market_setting", "", http.StatusNoContent)
	err = ex.SetUserMarketLeverageMultiplier(t.Context(), BTCUSDT, 1)
	require.NoError(t, err, "SetUserMarketLeverageMultiplier must not error")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodPost, gotRequest.method, "request method should be POST")
	assert.Equal(t, "/api/v4/margin/leverage/user_market_setting", gotRequest.path, "request path should match")
	assert.JSONEq(t, `{"leverage":"1","currency_pair":"BTC_USDT"}`, string(gotRequest.body), "request body should match")

	requireGateIORequestErrors(t, "/api/v4/margin/leverage/user_market_setting", false, func(ctx context.Context, ex *Exchange) error {
		return ex.SetUserMarketLeverageMultiplier(ctx, BTCUSDT, 1)
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_ISOLATED_LEVERAGE_SETTING")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveLeverageSetting](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_ISOLATED_LEVERAGE_SETTING"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_ISOLATED_LEVERAGE_SETTING must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_ISOLATED_LEVERAGE_SETTING must set dedicated_test_account=true")
		require.Positive(t, config.Leverage, "GCT_GATEIO_LIVE_ISOLATED_LEVERAGE_SETTING leverage must be positive")
		readLeverage := func(ctx context.Context) (float64, error) {
			accounts, err := e.GetIsolatedMarginAccountList(ctx, config.Pair)
			if err != nil {
				return 0, err
			}
			for i := range accounts {
				if accounts[i].CurrencyPair.Equal(config.Pair) {
					return accounts[i].Leverage.Float64(), nil
				}
			}
			return 0, fmt.Errorf("isolated margin leverage for %s was not returned", config.Pair)
		}
		priorLeverage, err := readLeverage(t.Context())
		require.NoError(t, err, "GetIsolatedMarginAccountList must retrieve the prior live leverage")
		require.Positive(t, priorLeverage, "prior live leverage must be positive")
		equal := func(a, b float64) bool { return math.Abs(a-b) <= 1e-10 }
		require.False(t, equal(priorLeverage, config.Leverage), "GCT_GATEIO_LIVE_ISOLATED_LEVERAGE_SETTING leverage must differ from the current setting")
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, reconcileGateIOLiveMutation(ctx, readLeverage, func(ctx context.Context) error {
				return e.SetUserMarketLeverageMultiplier(ctx, config.Pair, priorLeverage)
			}, priorLeverage, config.Leverage, equal, gateIOLiveReconciliationPollInterval, gateIOLiveReconciliationPollAttempts), "SetUserMarketLeverageMultiplier live cleanup should reconcile the prior setting")
		})
		err = e.SetUserMarketLeverageMultiplier(t.Context(), config.Pair, config.Leverage)
		require.NoError(t, err, "SetUserMarketLeverageMultiplier must not error against the live API")
	})
}

func TestGetIsolatedMarginAccountList(t *testing.T) {
	t.Parallel()

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/user/account", `[{"currency_pair":"BTC_USDT","account_type":"risk","base":{"currency":"BTC"},"quote":{"currency":"USDT"}}]`)
	accounts, err := ex.GetIsolatedMarginAccountList(t.Context(), BTCUSDT)
	require.NoError(t, err, "GetIsolatedMarginAccountList must not error")
	require.Len(t, accounts, 1, "GetIsolatedMarginAccountList must decode one account")
	assert.Equal(t, "risk", accounts[0].AccountType, "account type should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/margin/user/account", gotRequest.path, "request path should match")
	assert.Equal(t, BTCUSDT.String(), gotRequest.query.Get(gateIOTestCurrencyPairQueryKey), "currency pair should match")

	accounts, err = ex.GetIsolatedMarginAccountList(t.Context(), currency.EMPTYPAIR)
	require.NoError(t, err, "GetIsolatedMarginAccountList must accept an omitted pair")
	require.Len(t, accounts, 1, "GetIsolatedMarginAccountList must decode the response with an omitted pair")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "an empty currency pair should be omitted")

	requireGateIORequestErrors(t, "/api/v4/margin/user/account", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginAccountList(ctx, BTCUSDT)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetIsolatedMarginAccountList(t.Context(), currency.EMPTYPAIR)
		require.NoError(t, err, "GetIsolatedMarginAccountList must not error against the live API")
	})
}

func TestGetIsolatedMarginPoolLoans(t *testing.T) {
	t.Parallel()

	errorExchange, errorRequests := setupGateIOHTTPTest(t, http.MethodGet, "/apiw/v2/spot_loan/margin/margin_loan_info", `{"code":400,"message":"bad request","data":{"total":0,"list":[],"vip_settings":[]}}`)
	cancelledContext, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := errorExchange.GetIsolatedMarginPoolLoans(cancelledContext, currency.EMPTYCODE, 0, 0)
	require.ErrorIs(t, err, context.Canceled, "GetIsolatedMarginPoolLoans must return transport errors")
	_, err = errorExchange.GetIsolatedMarginPoolLoans(t.Context(), currency.EMPTYCODE, 0, 0)
	require.ErrorContains(t, err, "error code 400", "GetIsolatedMarginPoolLoans must return API errors")
	requireGateIOHTTPRequest(t, errorRequests)
	requireGateIORequestErrors(t, "/apiw/v2/spot_loan/margin/margin_loan_info", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetIsolatedMarginPoolLoans(ctx, currency.BTC, 0, 100)
		return err
	})

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/apiw/v2/spot_loan/margin/margin_loan_info", `{"code":200,"data":{"total":1,"list":[{"market":"BTC_USDT","deal":"2"}],"vip_settings":[]}}`)
	got, err := ex.GetIsolatedMarginPoolLoans(t.Context(), currency.BTC, 2, 100)
	require.NoError(t, err, "GetIsolatedMarginPoolLoans must not error")
	require.NotNil(t, got, "GetIsolatedMarginPoolLoans must return a response")
	require.Len(t, got.Data.List, 1, "GetIsolatedMarginPoolLoans must decode one loan")
	assert.Equal(t, BTCUSDT, got.Data.List[0].Market, "loan market should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/apiw/v2/spot_loan/margin/margin_loan_info", gotRequest.path, "request path should match")
	assert.Equal(t, "BTC", gotRequest.query.Get("search_coin"), "search coin should match")
	assert.Equal(t, "2", gotRequest.query.Get("page"), "page should match")
	assert.Equal(t, "100", gotRequest.query.Get(gateIOTestLimitQueryKey), "limit should match")
	got, err = ex.GetIsolatedMarginPoolLoans(t.Context(), currency.EMPTYCODE, 0, 0)
	require.NoError(t, err, "GetIsolatedMarginPoolLoans must accept omitted optional parameters")
	require.NotNil(t, got, "GetIsolatedMarginPoolLoans must return a response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	boundaryExchange, boundaryRequests := setupGateIOHTTPTest(t, http.MethodGet, "/apiw/v2/spot_loan/margin/margin_loan_info", `{"code":400,"message":"bad request","data":{"total":0,"list":[],"vip_settings":[]}}`)
	_, err = boundaryExchange.GetIsolatedMarginPoolLoans(t.Context(), currency.BTC, ^uint64(0), 100)
	require.ErrorContains(t, err, "error code 400", "GetIsolatedMarginPoolLoans must return the boundary fixture API error")
	gotRequest = requireGateIOHTTPRequest(t, boundaryRequests)
	assert.Equal(t, "18446744073709551615", gotRequest.query.Get("page"), "maximum page should retain its exact wire value")

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		ctx := request.WithHeaders(t.Context(), http.Header{
			"User-Agent":                {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"},
			"Accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
			"Accept-Language":           {"en-US,en;q=0.9"},
			"Sec-Ch-Ua":                 {`"Chromium";v="148", "Google Chrome";v="148", "Not/A)Brand";v="99"`},
			"Sec-Ch-Ua-Mobile":          {"?0"},
			"Sec-Ch-Ua-Platform":        {`"Windows"`},
			"Sec-Fetch-Dest":            {"document"},
			"Sec-Fetch-Mode":            {"navigate"},
			"Sec-Fetch-Site":            {"none"},
			"Sec-Fetch-User":            {"?1"},
			"Upgrade-Insecure-Requests": {"1"},
		})
		got, err := e.GetIsolatedMarginPoolLoans(ctx, currency.BTC, 0, 100)
		if err != nil && strings.Contains(err.Error(), "504") {
			t.Skipf("GateIO pool-loan live endpoint returned a gateway timeout: %v", err)
		}
		require.NoError(t, err, "GetIsolatedMarginPoolLoans must not error against the live API")
		require.NotNil(t, got, "GetIsolatedMarginPoolLoans must return a live response")
	})
}
