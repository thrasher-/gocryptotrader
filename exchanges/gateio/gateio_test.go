package gateio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/futures"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Please supply your own APIKEYS here for due diligence testing

const (
	canManipulateRealOrders              = false
	gateIOTestBTCUSDT                    = "BTC_USDT"
	gateIOTestBuySide                    = "buy"
	gateIOTestContractQueryKey           = "contract"
	gateIOTestCoinMarginedName           = "coin margined"
	gateIOTestCrossMarginCurrenciesPath  = "/api/v4/margin/cross/currencies"
	gateIOTestCurrencyPairQueryKey       = "currency_pair"
	gateIOTestDeliveryUSDTContractsPath  = "/api/v4/delivery/usdt/contracts"
	gateIOTestFromQueryKey               = "from"
	gateIOTestIntervalQueryKey           = "interval"
	gateIOTestLimitQueryKey              = "limit"
	gateIOTestOptionsContractsPath       = "/api/v4/options/contracts"
	gateIOTestOptionsUnderlyingResponse  = `[{"name":"BTC_USDT"}]`
	gateIOTestOptionsUnderlyingsPath     = "/api/v4/options/underlyings"
	gateIOTestOrderCorrelation           = "correlation"
	gateIOTestSpotCurrencyPairsPath      = "/api/v4/spot/currency_pairs"
	gateIOTestTypeQueryKey               = "type"
	gateIOTestUSDTMarginedName           = "USDT margined"
	gateIOLiveReconciliationPollInterval = 500 * time.Millisecond
	gateIOLiveReconciliationPollAttempts = 60
	gateIOLiveReconciliationTimeout      = 90 * time.Second
	gateIOLiveOrderCleanupPollAttempts   = 20
	gateIOTestLimitOrderType             = "limit"
)

var apiCredentials = &accounts.Credentials{
	Key:    "",
	Secret: "",
}

type gateIOLiveCancelAllSpotOrders struct {
	DedicatedTestAccount bool               `json:"dedicated_test_account"`
	Order                CreateOrderRequest `json:"order"`
	Side                 order.Side         `json:"side"`
}

type gateIOLiveAmendSpotOrder struct {
	DedicatedTestAccount bool               `json:"dedicated_test_account"`
	Order                CreateOrderRequest `json:"order"`
	Change               PriceAndAmount     `json:"change"`
}

type gateIOLiveCancelSingleSpotOrder struct {
	DedicatedTestAccount bool               `json:"dedicated_test_account"`
	Order                CreateOrderRequest `json:"order"`
}

type gateIOLiveBatchOrders struct {
	DedicatedTestAccount bool                 `json:"dedicated_test_account"`
	Orders               []CreateOrderRequest `json:"orders"`
}

type gateIOLiveSpotOrder struct {
	DedicatedTestAccount bool               `json:"dedicated_test_account"`
	Order                CreateOrderRequest `json:"order"`
}

type gateIOLiveIsolatedBorrowOrRepay struct {
	DedicatedTestAccount bool                       `json:"dedicated_test_account"`
	Request              IsolatedBorrowRepayRequest `json:"request"`
	Restore              IsolatedBorrowRepayRequest `json:"restore"`
}

type gateIOLiveLeverageSetting struct {
	DedicatedTestAccount bool          `json:"dedicated_test_account"`
	Pair                 currency.Pair `json:"pair"`
	Leverage             float64       `json:"leverage"`
}

type gateIOLiveAutoRepaymentStatus struct {
	DedicatedTestAccount bool `json:"dedicated_test_account"`
	Enabled              bool `json:"enabled"`
}

type gateIOLiveTransferCurrency struct {
	DedicatedTestAccount bool                  `json:"dedicated_test_account"`
	Request              TransferCurrencyParam `json:"request"`
}

type gateIOLiveSubAccountTransfer struct {
	DedicatedTestAccount bool                    `json:"dedicated_test_account"`
	Request              SubAccountTransferParam `json:"request"`
}

// Mutation live tests are disabled while their named GCT_GATEIO_LIVE_* environment variable is unset.
// The variable's JSON value both opts in and supplies the account-specific request values.
// Legacy canManipulateRealOrders tests retain their existing gating until they are converted separately.
var (
	gateIOLiveMutationMu  sync.Mutex
	gateIOLiveOrderTextID atomic.Uint64
)

var e *Exchange

type gateIOHTTPRequest struct {
	method string
	path   string
	query  url.Values
	body   []byte
}

func setupGateIOHTTPTest(t *testing.T, expectedMethod, expectedPath, response string, responseStatus ...int) (testExchange *Exchange, receivedRequests <-chan gateIOHTTPRequest) {
	t.Helper()
	require.LessOrEqual(t, len(responseStatus), 1, "response status must be omitted or contain one value")
	status := http.StatusOK
	if len(responseStatus) == 1 {
		status = responseStatus[0]
	}

	requests := make(chan gateIOHTTPRequest, 16)
	t.Cleanup(func() {
		assert.Empty(t, requests, "recorded requests should be consumed")
	})
	ex := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != expectedMethod || r.URL.Path != expectedPath {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err, "reading request body should not error")
		recorded := gateIOHTTPRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
			body:   body,
		}
		select {
		case requests <- recorded:
		default:
			assert.Fail(t, "recorded request buffer should not overflow")
		}
		w.WriteHeader(status)
		_, err = fmt.Fprint(w, response)
		assert.NoError(t, err, "writing response should not error")
	}))
	return ex, requests
}

func requireGateIORequestErrors(t *testing.T, expectedPath string, responseDecoding bool, requestCall func(context.Context, *Exchange) error) {
	t.Helper()

	setupResponse := func(response string, status int) *Exchange {
		return setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != expectedPath {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(status)
			_, err := fmt.Fprint(w, response)
			assert.NoError(t, err, "writing response should not error")
		}))
	}
	failing := setupResponse(`{}`, http.StatusBadGateway)
	cancelledContext, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, requestCall(cancelledContext, failing), context.Canceled, "endpoint must return a canceled transport error")
	require.Error(t, requestCall(t.Context(), failing), "endpoint must return a non-success HTTP error")
	if responseDecoding {
		malformed := setupResponse(`{`, http.StatusOK)
		require.Error(t, requestCall(t.Context(), malformed), "endpoint must return a malformed-response error")
	}
}

func requireGateIOHTTPRequest(t *testing.T, requests <-chan gateIOHTTPRequest) gateIOHTTPRequest {
	t.Helper()
	gotRequest, err := waitForGateIOHTTPRequest(t.Context(), requests, time.Second)
	require.NoError(t, err, "endpoint request must reach the recording handler")
	return gotRequest
}

func waitForGateIOHTTPRequest(ctx context.Context, requests <-chan gateIOHTTPRequest, timeout time.Duration) (gateIOHTTPRequest, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case gotRequest, ok := <-requests:
		if !ok {
			return gateIOHTTPRequest{}, errors.New("GateIO request channel closed before receiving a request")
		}
		return gotRequest, nil
	case <-ctx.Done():
		return gateIOHTTPRequest{}, ctx.Err()
	case <-timer.C:
		return gateIOHTTPRequest{}, errors.New("timed out waiting for GateIO endpoint request")
	}
}

func gateIOLiveTestValue(tb testing.TB, name string) string {
	tb.Helper()
	value := os.Getenv(name)
	if value == "" {
		tb.Skipf("live test parameter %s is unset", name)
	}
	return value
}

func decodeGateIOLiveTestJSON[T any](value string) (T, error) {
	var result T
	return result, json.Unmarshal([]byte(value), &result)
}

type gateIOLiveSpotOrderCleanup struct {
	orderID       string
	text          string
	currencyPair  currency.Pair
	isCrossMargin bool
}

func newGateIOLiveOrderText() string {
	return "t-gct-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(gateIOLiveOrderTextID.Add(1), 36)
}

func prepareGateIOLiveSpotOrder(arg *CreateOrderRequest) error {
	if arg == nil {
		return errNilArgument
	}
	if arg.CurrencyPair.IsInvalid() {
		return currency.ErrCurrencyPairEmpty
	}
	if arg.Account != spotAccount {
		return fmt.Errorf("live order account must be %q", spotAccount)
	}
	if arg.Type != gateIOTestLimitOrderType {
		return errors.New("live order type must be limit")
	}
	if arg.Side != gateIOTestBuySide && arg.Side != "sell" {
		return order.ErrSideIsInvalid
	}
	if arg.Amount <= 0 {
		return errInvalidAmount
	}
	if arg.Price <= 0 {
		return errInvalidPrice
	}
	if arg.TimeInForce != pocTIF {
		return fmt.Errorf("live order time in force must be %q", pocTIF)
	}
	if arg.AutoBorrow || arg.AutoRepay {
		return errors.New("live order must not enable automatic borrowing or repayment")
	}
	if arg.Text != "" {
		return errors.New("live order text is reserved for cleanup correlation")
	}
	arg.Text = newGateIOLiveOrderText()
	arg.ActionMode = "FULL"
	return nil
}

func reconcileGateIOLiveMutation[T any](ctx context.Context, read func(context.Context) (T, error), restore func(context.Context) error, before, applied T, equal func(T, T) bool, pollInterval time.Duration, pollAttempts int) error {
	if pollAttempts <= 0 {
		return errors.New("live mutation reconciliation poll attempts must be positive")
	}
	if equal(before, applied) {
		return errors.New("live mutation original and applied states must be distinguishable")
	}
	wait := func() error {
		if pollInterval <= 0 {
			return nil
		}
		timer := time.NewTimer(pollInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}

	appliedObserved := false
	var reconciliationErr error
	for attempt := range pollAttempts {
		current, err := read(ctx)
		switch {
		case err != nil:
			reconciliationErr = errors.Join(reconciliationErr, err)
		case equal(current, applied):
			appliedObserved = true
		case !equal(current, before):
			return errors.Join(reconciliationErr, errors.New("live mutation state is neither the original nor expected applied state"))
		}
		if appliedObserved {
			break
		}
		if attempt+1 < pollAttempts {
			if err := wait(); err != nil {
				return errors.Join(reconciliationErr, err)
			}
		}
	}
	if !appliedObserved {
		return errors.Join(reconciliationErr, errors.New("live mutation application was not observed before the reconciliation deadline"))
	}
	if err := restore(ctx); err != nil {
		reconciliationErr = errors.Join(reconciliationErr, err)
	}
	for attempt := range pollAttempts {
		current, err := read(ctx)
		switch {
		case err != nil:
			reconciliationErr = errors.Join(reconciliationErr, err)
		case equal(current, before):
			return nil
		case !equal(current, applied):
			return errors.Join(reconciliationErr, errors.New("live mutation cleanup state is neither the original nor expected applied state"))
		}
		if attempt+1 < pollAttempts {
			if err := wait(); err != nil {
				return errors.Join(reconciliationErr, err)
			}
		}
	}
	return errors.Join(reconciliationErr, errors.New("live mutation cleanup did not restore the original state"))
}

func cleanupGateIOLiveSpotOrders(ctx context.Context, list func(context.Context, currency.Pair, string, uint64, uint64) ([]SpotOrder, error), cancel func(context.Context, string, string, bool) (*SpotOrder, error), orders []gateIOLiveSpotOrderCleanup, pollInterval time.Duration, pollAttempts int) error {
	if pollAttempts <= 0 {
		return errors.New("live order cleanup poll attempts must be positive")
	}
	wait := func() error {
		if pollInterval <= 0 {
			return nil
		}
		timer := time.NewTimer(pollInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}

	var errs error
	for _, item := range orders {
		if item.orderID == "" && item.text == "" {
			continue
		}
		knownOrderIDs := make([]string, 0, 1)
		if item.orderID != "" {
			knownOrderIDs = append(knownOrderIDs, item.orderID)
		}
		observed := false
		consecutiveAbsent := 0
		var lastAttemptErr error
		confirmedAbsent := false
		for attempt := range pollAttempts {
			var listErr error
			activeOrderIDs := make([]string, 0, len(knownOrderIDs))
			for page := uint64(1); ; page++ {
				pageOrders, err := list(ctx, item.currencyPair, statusOpen, page, 100)
				if err != nil {
					listErr = err
					break
				}
				for i := range pageOrders {
					if (item.text != "" && pageOrders[i].Text == item.text) || (item.orderID != "" && pageOrders[i].OrderID == item.orderID) {
						if pageOrders[i].OrderID != "" && !slices.Contains(activeOrderIDs, pageOrders[i].OrderID) {
							activeOrderIDs = append(activeOrderIDs, pageOrders[i].OrderID)
							if !slices.Contains(knownOrderIDs, pageOrders[i].OrderID) {
								knownOrderIDs = append(knownOrderIDs, pageOrders[i].OrderID)
							}
						}
					}
				}
				if len(pageOrders) < 100 {
					break
				}
			}
			switch {
			case len(activeOrderIDs) > 0:
				observed = true
				consecutiveAbsent = 0
			case listErr == nil:
				consecutiveAbsent++
			default:
				consecutiveAbsent = 0
			}

			cancelOrderIDs := slices.Clone(knownOrderIDs)
			for _, orderID := range activeOrderIDs {
				if !slices.Contains(cancelOrderIDs, orderID) {
					cancelOrderIDs = append(cancelOrderIDs, orderID)
				}
			}
			lastAttemptErr = listErr
			for _, orderID := range cancelOrderIDs {
				_, err := cancel(ctx, orderID, item.currencyPair.String(), item.isCrossMargin)
				lastAttemptErr = common.AppendError(lastAttemptErr, err)
			}

			minimumAbsentObservations := 2
			if pollAttempts == 1 {
				minimumAbsentObservations = 1
			}
			if consecutiveAbsent >= minimumAbsentObservations && (len(knownOrderIDs) > 0 || observed) {
				confirmedAbsent = true
				break
			}
			if attempt+1 < pollAttempts {
				if err := wait(); err != nil {
					lastAttemptErr = common.AppendError(lastAttemptErr, err)
					break
				}
			}
		}
		if !confirmedAbsent {
			errs = common.AppendError(errs, lastAttemptErr)
			errs = common.AppendError(errs, fmt.Errorf("live order cleanup could not confirm absence for order %q text %q", item.orderID, item.text))
		}
	}
	return errs
}

func TestGateIOLiveTestValue(t *testing.T) {
	const name = "GCT_GATEIO_LIVE_TEST_VALUE"
	t.Run("set", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("continue", func(t *testing.T) {
			t.Setenv(name, "value")
			assert.Equal(t, "value", gateIOLiveTestValue(t, name), "gateIOLiveTestValue should return the configured value")
			reachedEndpoint = true
		}), "gateIOLiveTestValue set subtest must not fail")
		assert.True(t, reachedEndpoint, "gateIOLiveTestValue should continue when the value is set")
	})

	t.Run("unset", func(t *testing.T) {
		t.Setenv(name, "")
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			gateIOLiveTestValue(t, name)
			reachedEndpoint = true
		}), "gateIOLiveTestValue unset subtest must not fail")
		assert.False(t, reachedEndpoint, "gateIOLiveTestValue should skip when the value is unset")
	})
}

func TestDecodeGateIOLiveTestJSON(t *testing.T) {
	t.Parallel()
	type value struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}

	got, err := decodeGateIOLiveTestJSON[value](`{"name":"fixture","enabled":true}`)
	require.NoError(t, err, "decodeGateIOLiveTestJSON must decode valid JSON")
	assert.Equal(t, value{Name: "fixture", Enabled: true}, got, "decoded live test value should match")

	_, err = decodeGateIOLiveTestJSON[value](`{`)
	require.Error(t, err, "decodeGateIOLiveTestJSON must return malformed JSON errors")
}

func TestNewGateIOLiveOrderText(t *testing.T) {
	t.Parallel()

	first := newGateIOLiveOrderText()
	second := newGateIOLiveOrderText()
	assert.True(t, strings.HasPrefix(first, "t-gct-"), "live order text should use the GateIO custom-text prefix")
	assert.LessOrEqual(t, len(first), 30, "live order text should fit GateIO's 28-byte payload limit after the t- prefix")
	assert.NotEqual(t, first, second, "live order text should be unique")
}

func TestPrepareGateIOLiveSpotOrder(t *testing.T) {
	t.Parallel()

	valid := CreateOrderRequest{CurrencyPair: BTCUSDT, Type: gateIOTestLimitOrderType, Account: spotAccount, Side: gateIOTestBuySide, Amount: 1, Price: 2, TimeInForce: pocTIF}
	require.ErrorIs(t, prepareGateIOLiveSpotOrder(nil), errNilArgument, "nil live orders must be rejected")
	for _, tc := range []struct {
		name string
		edit func(*CreateOrderRequest)
		err  error
	}{
		{name: "pair", edit: func(arg *CreateOrderRequest) { arg.CurrencyPair = currency.EMPTYPAIR }, err: currency.ErrCurrencyPairEmpty},
		{name: "account", edit: func(arg *CreateOrderRequest) { arg.Account = marginAccount }},
		{name: "order type", edit: func(arg *CreateOrderRequest) { arg.Type = "market" }},
		{name: "side", edit: func(arg *CreateOrderRequest) { arg.Side = "hold" }, err: order.ErrSideIsInvalid},
		{name: "amount", edit: func(arg *CreateOrderRequest) { arg.Amount = 0 }, err: errInvalidAmount},
		{name: "price", edit: func(arg *CreateOrderRequest) { arg.Price = 0 }, err: errInvalidPrice},
		{name: "time in force", edit: func(arg *CreateOrderRequest) { arg.TimeInForce = gtcTIF }},
		{name: "auto borrow", edit: func(arg *CreateOrderRequest) { arg.AutoBorrow = true }},
		{name: "auto repay", edit: func(arg *CreateOrderRequest) { arg.AutoRepay = true }},
		{name: "text", edit: func(arg *CreateOrderRequest) { arg.Text = "t-caller" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			arg := valid
			tc.edit(&arg)
			err := prepareGateIOLiveSpotOrder(&arg)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err, "invalid live order must return the expected error")
			} else {
				require.Error(t, err, "invalid live order must return an error")
			}
		})
	}

	first := valid
	require.NoError(t, prepareGateIOLiveSpotOrder(&first), "valid live order must be prepared")
	assert.True(t, strings.HasPrefix(first.Text, "t-gct-"), "prepared order should have a cleanup correlation ID")
	assert.Equal(t, "FULL", first.ActionMode, "prepared order should request the full response")
	second := valid
	require.NoError(t, prepareGateIOLiveSpotOrder(&second), "second valid live order must be prepared")
	assert.NotEqual(t, first.Text, second.Text, "prepared live order correlation IDs should be unique")
}

func TestReconcileGateIOLiveMutation(t *testing.T) {
	t.Parallel()

	const pollAttempts = 3
	expectedErr := errors.New("forced reconciliation error")
	equal := func(a, b int) bool { return a == b }
	t.Run("application not observed", func(t *testing.T) {
		t.Parallel()
		reads := 0
		restored := false
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) {
			reads++
			return 1, nil
		}, func(context.Context) error {
			restored = true
			return nil
		}, 1, 2, equal, 0, pollAttempts)
		require.ErrorContains(t, err, "application was not observed", "an unobserved acknowledged mutation must remain unresolved")
		assert.Equal(t, pollAttempts, reads, "original state should be observed for the full polling window")
		assert.False(t, restored, "an unobserved mutation should not invoke a potentially unsafe inverse operation")
	})
	t.Run("indistinguishable states", func(t *testing.T) {
		t.Parallel()
		read := false
		restored := false
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) {
			read = true
			return 1, nil
		}, func(context.Context) error {
			restored = true
			return nil
		}, 1, 1, equal, 0, pollAttempts)
		require.ErrorContains(t, err, "must be distinguishable", "indistinguishable mutation states must be rejected")
		assert.False(t, read, "indistinguishable mutation states should be rejected before polling")
		assert.False(t, restored, "indistinguishable mutation states should not invoke an unsafe inverse operation")
	})
	t.Run("applied", func(t *testing.T) {
		t.Parallel()
		state := 2
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) { return state, nil }, func(context.Context) error {
			state = 1
			return nil
		}, 1, 2, equal, 0, pollAttempts)
		require.NoError(t, err, "applied state must be restored")
		assert.Equal(t, 1, state, "cleanup should restore the original state")
	})
	t.Run("delayed application and restoration", func(t *testing.T) {
		t.Parallel()
		states := []int{1, 1, 2, 2, 1}
		reads := 0
		restores := 0
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) {
			state := states[reads]
			reads++
			return state, nil
		}, func(context.Context) error {
			restores++
			return nil
		}, 1, 2, equal, time.Nanosecond, pollAttempts)
		require.NoError(t, err, "delayed applied and restored states must be reconciled")
		assert.Equal(t, len(states), reads, "reconciliation should poll through both delayed transitions")
		assert.Equal(t, 1, restores, "cleanup should restore the delayed mutation once")
	})
	t.Run("transient observation error", func(t *testing.T) {
		t.Parallel()
		state := 2
		reads := 0
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) {
			reads++
			if reads == 1 {
				return 0, expectedErr
			}
			return state, nil
		}, func(context.Context) error {
			state = 1
			return nil
		}, 1, 2, equal, 0, pollAttempts)
		require.NoError(t, err, "transient observation errors must not prevent verified cleanup")
		assert.Equal(t, 1, state, "cleanup should restore state after a transient observation error")
	})
	t.Run("ambiguous restore error", func(t *testing.T) {
		t.Parallel()
		state := 2
		restores := 0
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) {
			return state, nil
		}, func(context.Context) error {
			restores++
			return expectedErr
		}, 1, 2, equal, 0, pollAttempts)
		require.ErrorIs(t, err, expectedErr, "ambiguous restore errors must be returned")
		assert.Equal(t, 1, restores, "cleanup should not retry a potentially non-idempotent inverse operation")
	})
	t.Run("transient verification error", func(t *testing.T) {
		t.Parallel()
		state := 2
		reads := 0
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) {
			reads++
			if reads == 2 {
				return 0, expectedErr
			}
			return state, nil
		}, func(context.Context) error {
			state = 1
			return nil
		}, 1, 2, equal, 0, pollAttempts)
		require.NoError(t, err, "transient verification errors must not prevent verified cleanup")
		assert.Equal(t, 1, state, "cleanup should restore state after a transient verification error")
	})
	t.Run("initial read error", func(t *testing.T) {
		t.Parallel()
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) { return 0, expectedErr }, func(context.Context) error { return nil }, 1, 2, equal, 0, pollAttempts)
		require.ErrorIs(t, err, expectedErr, "initial state read errors must be returned")
	})
	t.Run("unexpected state", func(t *testing.T) {
		t.Parallel()
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) { return 3, nil }, func(context.Context) error { return nil }, 1, 2, equal, 0, pollAttempts)
		require.ErrorContains(t, err, "neither the original nor expected applied state", "unexpected mutation state must be rejected")
	})
	t.Run("restore error", func(t *testing.T) {
		t.Parallel()
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) { return 2, nil }, func(context.Context) error { return expectedErr }, 1, 2, equal, 0, pollAttempts)
		require.ErrorIs(t, err, expectedErr, "restore errors must be returned")
	})
	t.Run("verification read error", func(t *testing.T) {
		t.Parallel()
		reads := 0
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) {
			reads++
			if reads == 2 {
				return 0, expectedErr
			}
			return 2, nil
		}, func(context.Context) error { return nil }, 1, 2, equal, 0, pollAttempts)
		require.ErrorIs(t, err, expectedErr, "verification state read errors must be returned")
	})
	t.Run("unexpected verification state", func(t *testing.T) {
		t.Parallel()
		reads := 0
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) {
			reads++
			if reads == 1 {
				return 2, nil
			}
			return 3, nil
		}, func(context.Context) error { return nil }, 1, 2, equal, 0, pollAttempts)
		require.ErrorContains(t, err, "cleanup state is neither", "unexpected cleanup state must be rejected")
	})
	t.Run("not restored", func(t *testing.T) {
		t.Parallel()
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) { return 2, nil }, func(context.Context) error { return nil }, 1, 2, equal, 0, pollAttempts)
		require.ErrorContains(t, err, "did not restore", "cleanup that leaves the applied state must be rejected")
	})
	t.Run("context cancelled while observing", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := reconcileGateIOLiveMutation(ctx, func(context.Context) (int, error) { return 1, nil }, func(context.Context) error { return nil }, 1, 2, equal, time.Hour, pollAttempts)
		require.ErrorIs(t, err, context.Canceled, "observation waits must return context cancellation")
	})
	t.Run("invalid attempts", func(t *testing.T) {
		t.Parallel()
		err := reconcileGateIOLiveMutation(t.Context(), func(context.Context) (int, error) { return 1, nil }, func(context.Context) error { return nil }, 1, 2, equal, 0, 0)
		require.ErrorContains(t, err, "poll attempts must be positive", "reconciliation must reject an empty polling window")
	})
}

func TestCleanupGateIOLiveSpotOrders(t *testing.T) {
	t.Parallel()

	t.Run("invalid attempts", func(t *testing.T) {
		t.Parallel()
		err := cleanupGateIOLiveSpotOrders(t.Context(), nil, nil, nil, 0, 0)
		require.ErrorContains(t, err, "poll attempts must be positive", "cleanup must reject an empty polling window")
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, cleanupGateIOLiveSpotOrders(t.Context(), func(context.Context, currency.Pair, string, uint64, uint64) ([]SpotOrder, error) {
			require.FailNow(t, "empty cleanup must not list orders")
			return nil, nil
		}, func(context.Context, string, string, bool) (*SpotOrder, error) {
			require.FailNow(t, "empty cleanup must not cancel orders")
			return nil, nil
		}, []gateIOLiveSpotOrderCleanup{{currencyPair: BTCUSDT}}, 0, 1), "cleanup must accept empty fixtures")
	})

	t.Run("known ID survives transient listing error", func(t *testing.T) {
		t.Parallel()
		expectedListErr := errors.New("forced list error")
		listCalls := 0
		cancelCalls := 0
		err := cleanupGateIOLiveSpotOrders(t.Context(), func(_ context.Context, pair currency.Pair, status string, page, limit uint64) ([]SpotOrder, error) {
			assert.Equal(t, BTCUSDT, pair, "cleanup pair should match")
			assert.Equal(t, statusOpen, status, "cleanup should query open orders")
			assert.Equal(t, uint64(1), page, "cleanup should start with the first API page")
			assert.Equal(t, uint64(100), limit, "cleanup page size should match")
			listCalls++
			if listCalls == 1 {
				return nil, expectedListErr
			}
			return nil, nil
		}, func(_ context.Context, orderID, pair string, isCrossMargin bool) (*SpotOrder, error) {
			cancelCalls++
			assert.Equal(t, "known-id", orderID, "known order ID should be cancelled directly")
			assert.Equal(t, BTCUSDT.String(), pair, "cancellation pair should match")
			assert.False(t, isCrossMargin, "spot cleanup should not use cross margin")
			return &SpotOrder{OrderID: orderID}, nil
		}, []gateIOLiveSpotOrderCleanup{{orderID: "known-id", text: gateIOTestOrderCorrelation, currencyPair: BTCUSDT}}, 0, 3)
		require.NoError(t, err, "cleanup must recover from a transient listing error when the known ID can be cancelled")
		assert.Equal(t, 3, listCalls, "cleanup should confirm absence twice after the transient error")
		assert.Equal(t, 3, cancelCalls, "cleanup should keep targeting the known ID until absence is confirmed")
	})

	t.Run("text discovery persists across listing failures and confirms absence", func(t *testing.T) {
		t.Parallel()
		expectedListErr := errors.New("forced list error")
		attempt := 0
		cancelCalls := 0
		err := cleanupGateIOLiveSpotOrders(t.Context(), func(_ context.Context, _ currency.Pair, _ string, page, limit uint64) ([]SpotOrder, error) {
			assert.Equal(t, uint64(100), limit, "cleanup page size should match")
			if page == 1 {
				attempt++
			}
			if attempt == 1 && page == 1 {
				orders := make([]SpotOrder, 100)
				for i := range orders {
					orders[i].OrderID = strconv.Itoa(i + 1)
				}
				return orders, nil
			}
			if attempt == 1 && page == 2 {
				return []SpotOrder{{OrderID: "found-id", Text: gateIOTestOrderCorrelation}}, nil
			}
			if attempt == 2 && page == 1 {
				return nil, expectedListErr
			}
			return nil, nil
		}, func(_ context.Context, orderID, _ string, isCrossMargin bool) (*SpotOrder, error) {
			cancelCalls++
			assert.Equal(t, "found-id", orderID, "text-correlated order ID should be cancelled")
			assert.True(t, isCrossMargin, "configured cross-margin cleanup should be preserved")
			return &SpotOrder{OrderID: orderID}, nil
		}, []gateIOLiveSpotOrderCleanup{{text: gateIOTestOrderCorrelation, currencyPair: BTCUSDT, isCrossMargin: true}}, 0, 4)
		require.NoError(t, err, "cleanup must retain a paginated text-correlated order across a transient listing error")
		assert.Equal(t, 4, attempt, "cleanup should recover from the listing error and confirm two absent observations")
		assert.Equal(t, 4, cancelCalls, "cleanup should retain and retry the discovered order ID on every attempt")
	})

	t.Run("text discovery survives a later page failure", func(t *testing.T) {
		t.Parallel()
		expectedListErr := errors.New("forced later page error")
		attempt := 0
		cancelCalls := 0
		err := cleanupGateIOLiveSpotOrders(t.Context(), func(_ context.Context, _ currency.Pair, _ string, page, limit uint64) ([]SpotOrder, error) {
			assert.Equal(t, uint64(100), limit, "cleanup page size should match")
			if page == 1 {
				attempt++
			}
			if attempt <= 2 {
				if page == 2 {
					return nil, expectedListErr
				}
				orders := make([]SpotOrder, 100)
				if attempt == 1 {
					orders[0] = SpotOrder{OrderID: "found-id", Text: gateIOTestOrderCorrelation}
				}
				return orders, nil
			}
			return nil, nil
		}, func(_ context.Context, orderID, _ string, _ bool) (*SpotOrder, error) {
			cancelCalls++
			assert.Equal(t, "found-id", orderID, "text-correlated order ID should be cancelled")
			return &SpotOrder{OrderID: orderID}, nil
		}, []gateIOLiveSpotOrderCleanup{{text: gateIOTestOrderCorrelation, currencyPair: BTCUSDT}}, 0, 4)
		require.NoError(t, err, "cleanup must retain an ID discovered before a later page failure")
		assert.Equal(t, 4, attempt, "cleanup should retry after later page failures and confirm absence twice")
		assert.Equal(t, 4, cancelCalls, "cleanup should cancel the discovered ID during every attempt")
	})

	t.Run("text-only absence is observed for the full window", func(t *testing.T) {
		t.Parallel()
		listCalls := 0
		err := cleanupGateIOLiveSpotOrders(t.Context(), func(context.Context, currency.Pair, string, uint64, uint64) ([]SpotOrder, error) {
			listCalls++
			return nil, nil
		}, func(context.Context, string, string, bool) (*SpotOrder, error) {
			require.FailNow(t, "absent text-only fixture must not be cancelled")
			return nil, nil
		}, []gateIOLiveSpotOrderCleanup{{text: "never-visible", currencyPair: BTCUSDT}}, 0, 3)
		require.ErrorContains(t, err, "could not confirm absence", "a text-only fixture that was never observed must remain unresolved")
		assert.Equal(t, 3, listCalls, "text-only absence should be observed for the full polling window")
	})

	t.Run("unresolved listing and cancellation errors", func(t *testing.T) {
		t.Parallel()
		expectedListErr := errors.New("forced list error")
		expectedCancelErr := errors.New("forced cancellation error")
		err := cleanupGateIOLiveSpotOrders(t.Context(), func(context.Context, currency.Pair, string, uint64, uint64) ([]SpotOrder, error) {
			return nil, expectedListErr
		}, func(context.Context, string, string, bool) (*SpotOrder, error) {
			return nil, expectedCancelErr
		}, []gateIOLiveSpotOrderCleanup{{orderID: "known-id", currencyPair: BTCUSDT}}, 0, 2)
		require.ErrorIs(t, err, expectedListErr, "unresolved cleanup must return the final listing error")
		require.ErrorIs(t, err, expectedCancelErr, "unresolved cleanup must return the final cancellation error")
		require.ErrorContains(t, err, "could not confirm absence", "unresolved cleanup must identify the fixture")
	})

	t.Run("context cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := cleanupGateIOLiveSpotOrders(ctx, func(context.Context, currency.Pair, string, uint64, uint64) ([]SpotOrder, error) {
			return []SpotOrder{{OrderID: "found-id", Text: gateIOTestOrderCorrelation}}, nil
		}, func(context.Context, string, string, bool) (*SpotOrder, error) {
			return &SpotOrder{}, nil
		}, []gateIOLiveSpotOrderCleanup{{text: gateIOTestOrderCorrelation, currencyPair: BTCUSDT}}, time.Hour, 2)
		require.ErrorIs(t, err, context.Canceled, "cleanup polling must return context cancellation")
	})
}

func TestRequireGateIORequestErrors(t *testing.T) {
	t.Parallel()
	requestCall := func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetServerTime(ctx, asset.Spot)
		return err
	}
	t.Run("transport and decoding", func(t *testing.T) {
		t.Parallel()
		requireGateIORequestErrors(t, "/api/v4/spot/time", true, requestCall)
	})
	t.Run("transport only", func(t *testing.T) {
		t.Parallel()
		requireGateIORequestErrors(t, "/api/v4/spot/time", false, requestCall)
	})
}

func TestRequireGateIOHTTPRequest(t *testing.T) {
	t.Parallel()
	expected := gateIOHTTPRequest{method: http.MethodGet, path: "/api/v4/fixture", query: url.Values{"key": {"value"}}, body: []byte(`{}`)}
	requests := make(chan gateIOHTTPRequest, 1)
	requests <- expected
	assert.Equal(t, expected, requireGateIOHTTPRequest(t, requests), "required request should match the recorded value")
}

func TestWaitForGateIOHTTPRequest(t *testing.T) {
	t.Parallel()
	expected := gateIOHTTPRequest{method: http.MethodGet, path: "/api/v4/fixture"}
	requests := make(chan gateIOHTTPRequest, 1)
	requests <- expected
	got, err := waitForGateIOHTTPRequest(t.Context(), requests, time.Second)
	require.NoError(t, err, "recorded request must be returned")
	assert.Equal(t, expected, got, "recorded request should match")

	closedRequests := make(chan gateIOHTTPRequest)
	close(closedRequests)
	_, err = waitForGateIOHTTPRequest(t.Context(), closedRequests, time.Second)
	require.ErrorContains(t, err, "channel closed", "closed request channels must return an error")

	cancelledContext, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = waitForGateIOHTTPRequest(cancelledContext, make(chan gateIOHTTPRequest), time.Hour)
	require.ErrorIs(t, err, context.Canceled, "context cancellation must be returned")

	_, err = waitForGateIOHTTPRequest(t.Context(), make(chan gateIOHTTPRequest), time.Nanosecond)
	require.ErrorContains(t, err, "timed out", "missing requests must return a bounded timeout error")
}

func TestSetupGateIOHTTPTest(t *testing.T) {
	t.Parallel()

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/spot/time", `{"server_time":1787110000}`)
	serverTime, err := ex.GetServerTime(t.Context(), asset.Spot)
	require.NoError(t, err, "GetServerTime must not error for the configured route")
	assert.Equal(t, time.Unix(1787110000, 0), serverTime, "server time should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodGet, gotRequest.method, "request method should match")
	assert.Equal(t, "/api/v4/spot/time", gotRequest.path, "request path should match")
	endpoint, err := ex.API.Endpoints.GetURL(exchange.RestSpot)
	require.NoError(t, err, "GateIO REST endpoint must be available")
	httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint+"spot/time", http.NoBody)
	require.NoError(t, err, "method-mismatch request must be created")
	response, err := http.DefaultClient.Do(httpRequest)
	require.NoError(t, err, "method-mismatch request must complete")
	t.Cleanup(func() {
		assert.NoError(t, response.Body.Close(), "closing method-mismatch response body should not error")
	})
	assert.Equal(t, http.StatusNotFound, response.StatusCode, "unexpected request method should be rejected")

	var result any
	err = ex.SendHTTPRequest(t.Context(), exchange.RestSpot, publicGetServerTimeEPL, "unknown", &result)
	require.ErrorContains(t, err, "404", "unknown routes must return an HTTP error")
}

func skipGateIOLiveTest(tb testing.TB, requiresCredentials bool) {
	tb.Helper()
	liveEnabled := !livetest.ShouldSkip() && strings.EqualFold(strings.TrimSpace(os.Getenv("GCT_GATEIO_LIVE_TESTS")), "true")
	skipGateIOLiveTestWithState(tb, requiresCredentials, liveEnabled, e)
}

func skipGateIOLiveTestWithState(tb testing.TB, requiresCredentials, liveEnabled bool, ex *Exchange) {
	tb.Helper()
	if !liveEnabled {
		tb.Skip("live testing disabled; set GCT_GATEIO_LIVE_TESTS=true to enable")
	}
	if requiresCredentials {
		sharedtestvalues.SkipTestIfCredentialsUnset(tb, ex)
	}
}

func skipGateIOLiveMutationTest(tb testing.TB, enableEnvironment string) {
	tb.Helper()
	liveEnabled := !livetest.ShouldSkip() && strings.EqualFold(strings.TrimSpace(os.Getenv("GCT_GATEIO_LIVE_TESTS")), "true")
	skipGateIOLiveMutationTestWithState(tb, enableEnvironment, os.Getenv(enableEnvironment) != "", liveEnabled, e)
}

func skipGateIOLiveMutationTestWithState(tb testing.TB, enableEnvironment string, enabled, liveEnabled bool, ex *Exchange) {
	tb.Helper()
	skipGateIOLiveTestWithState(tb, true, liveEnabled, ex)
	if !enabled {
		tb.Skipf("live mutation disabled; populate %s", enableEnvironment)
	}
	gateIOLiveMutationMu.Lock()
	tb.Cleanup(gateIOLiveMutationMu.Unlock)
}

func TestSkipGateIOLiveMutationTest(t *testing.T) {
	t.Setenv("GCT_GATEIO_LIVE_TESTS", "")
	const enableEnvironment = "GCT_GATEIO_LIVE_TEST_MUTATION"
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "disabled"},
		{name: "enabled", value: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(enableEnvironment, tc.value)
			reachedEndpoint := false
			require.True(t, t.Run("configured build", func(t *testing.T) {
				skipGateIOLiveMutationTest(t, enableEnvironment)
				reachedEndpoint = true
			}), "configured-build mutation subtest must not fail")
			assert.False(t, reachedEndpoint, "mutation gate should remain disabled while the package live opt-in is unset")
		})
	}
}

func TestSkipGateIOLiveTest(t *testing.T) {
	t.Setenv("GCT_GATEIO_LIVE_TESTS", "")
	reachedEndpoint := false
	require.True(t, t.Run("configured build", func(t *testing.T) {
		skipGateIOLiveTest(t, false)
		reachedEndpoint = true
	}), "skipGateIOLiveTest configured-build subtest must not fail")
	assert.False(t, reachedEndpoint, "skipGateIOLiveTest should require the package live opt-in")
}

func TestSkipGateIOLiveTestWithState(t *testing.T) {
	t.Run("live disabled", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			skipGateIOLiveTestWithState(t, false, false, nil)
			reachedEndpoint = true
		}), "mock-build subtest must not fail")
		assert.False(t, reachedEndpoint, "disabled live state should skip the endpoint")
	})

	t.Run("public live build", func(t *testing.T) {
		skipGateIOLiveTestWithState(t, false, true, nil)
	})

	t.Run("private live build without credentials", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			ex := new(Exchange)
			ex.SetDefaults()
			skipGateIOLiveTestWithState(t, true, true, ex)
			reachedEndpoint = true
		}), "missing-credentials subtest must not fail")
		assert.False(t, reachedEndpoint, "missing credentials should skip the live endpoint")
	})

	t.Run("private live build with credentials", func(t *testing.T) {
		ex := new(Exchange)
		ex.SetDefaults()
		ex.SetCredentials(&accounts.Credentials{Key: "key", Secret: "secret"})
		skipGateIOLiveTestWithState(t, true, true, ex)
	})
}

func TestSkipGateIOLiveMutationTestWithState(t *testing.T) {
	credentialedExchange := func() *Exchange {
		ex := new(Exchange)
		ex.SetDefaults()
		ex.SetCredentials(&accounts.Credentials{Key: "key", Secret: "secret"})
		return ex
	}

	t.Run("live disabled", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			skipGateIOLiveMutationTestWithState(t, "GCT_GATEIO_LIVE_TEST_MUTATION", true, false, nil)
			reachedEndpoint = true
		}), "mock-build mutation subtest must not fail")
		assert.False(t, reachedEndpoint, "disabled live state should skip the mutation")
	})

	t.Run("missing credentials", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			ex := new(Exchange)
			ex.SetDefaults()
			skipGateIOLiveMutationTestWithState(t, "GCT_GATEIO_LIVE_TEST_MUTATION", true, true, ex)
			reachedEndpoint = true
		}), "missing-credentials mutation subtest must not fail")
		assert.False(t, reachedEndpoint, "missing credentials should skip the live mutation")
	})

	t.Run("mutation disabled", func(t *testing.T) {
		reachedEndpoint := false
		require.True(t, t.Run("skip", func(t *testing.T) {
			skipGateIOLiveMutationTestWithState(t, "GCT_GATEIO_LIVE_TEST_MUTATION", false, true, credentialedExchange())
			reachedEndpoint = true
		}), "disabled mutation subtest must not fail")
		assert.False(t, reachedEndpoint, "disabled mutation should skip the live endpoint")
	})

	t.Run("mutation enabled", func(t *testing.T) {
		skipGateIOLiveMutationTestWithState(t, "GCT_GATEIO_LIVE_TEST_MUTATION", true, true, credentialedExchange())
	})
}

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Gateio Setup error: %s", err)
	}

	if apiCredentials.Key != "" && apiCredentials.Secret != "" {
		e.API.AuthenticatedSupport = true
		e.API.AuthenticatedWebsocketSupport = true
		e.SetCredentials(apiCredentials)
	}

	os.Exit(m.Run())
}

func TestSetUnixTimeRangeParams(t *testing.T) {
	t.Parallel()
	from := time.Unix(1710000000, 0)
	to := from.Add(time.Hour)
	for _, tc := range []struct {
		name           string
		from           time.Time
		to             time.Time
		expectedParams url.Values
		expectedErr    error
	}{
		{
			name:           "both set",
			from:           from,
			to:             to,
			expectedParams: url.Values{gateIOTestFromQueryKey: {strconv.FormatInt(from.Unix(), 10)}, "to": {strconv.FormatInt(to.Unix(), 10)}},
		},
		{
			name:           "from only",
			from:           from,
			expectedParams: url.Values{gateIOTestFromQueryKey: {strconv.FormatInt(from.Unix(), 10)}},
		},
		{
			name:           "to only",
			to:             to,
			expectedParams: url.Values{"to": {strconv.FormatInt(to.Unix(), 10)}},
		},
		{
			name:           "both zero",
			expectedParams: url.Values{},
		},
		{
			name:           "start after end",
			from:           to,
			to:             from,
			expectedParams: url.Values{},
			expectedErr:    common.ErrStartAfterEnd,
		},
		{
			name:           "start equals end",
			from:           from,
			to:             from,
			expectedParams: url.Values{gateIOTestFromQueryKey: {strconv.FormatInt(from.Unix(), 10)}, "to": {strconv.FormatInt(from.Unix(), 10)}},
		},
		{
			name:           "start after current time",
			from:           time.Date(2222, 1, 1, 0, 0, 0, 0, time.UTC),
			to:             time.Date(2222, 1, 2, 0, 0, 0, 0, time.UTC),
			expectedParams: url.Values{},
			expectedErr:    common.ErrStartAfterTimeNow,
		},
		{
			name:           "equal times after current time",
			from:           time.Date(2222, 1, 1, 0, 0, 0, 0, time.UTC),
			to:             time.Date(2222, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedParams: url.Values{},
			expectedErr:    common.ErrStartAfterTimeNow,
		},
		{
			name:           "unix epoch",
			from:           time.Unix(0, 0),
			to:             to,
			expectedParams: url.Values{},
			expectedErr:    common.ErrDateUnset,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params := url.Values{}
			err := setUnixTimeRangeParams(&params, tc.from, tc.to)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr, "setUnixTimeRangeParams must return the expected error")
			} else {
				require.NoError(t, err, "setUnixTimeRangeParams must not error")
			}
			assert.Equal(t, tc.expectedParams, params, "params should match expected values")
		})
	}
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
}

func TestCancelAllExchangeOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelAllOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrCancelOrderIsNil)

	r := &order.Cancel{
		OrderID:   "1",
		AccountID: "1",
	}

	for _, a := range e.GetAssetTypes(false) {
		r.AssetType = a
		r.Pair = currency.EMPTYPAIR
		_, err = e.CancelAllOrders(t.Context(), r)
		assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

		r.Pair = getPair(t, a)
		_, err = e.CancelAllOrders(t.Context(), r)
		require.NoError(t, err)
	}
}

func TestUpdateAccountBalances(t *testing.T) {
	t.Parallel()

	responses := map[string]string{
		"/api/v4/spot/accounts":          `[{"currency":"BTC","available":"2","locked":"1"}]`,
		"/api/v4/margin/user/account":    `[{"base":{"currency":"BTC","available":"2","borrowed":"0.5","locked":"1"},"quote":{"currency":"USDT","available":"3","borrowed":"1","locked":"2"}}]`,
		"/api/v4/margin/cross/accounts":  `{"balances":{"BTC":{"available":"2","freeze":"1","borrowed":"0.5","interest":"0.1"}}}`,
		"/api/v4/futures/btc/accounts":   `{"currency":"BTC","total":"3","available":"2"}`,
		"/api/v4/futures/usdt/accounts":  `{"currency":"USDT","total":"3","available":"2"}`,
		"/api/v4/delivery/usdt/accounts": `{"currency":"USDT","total":"3","available":"2"}`,
		"/api/v4/options/accounts":       `{"currency":"USDT","total":"3","available":"2"}`,
	}
	ex := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing account response should not error")
	}))
	// UpdateAccountBalances persists through the account manager, which requires credentials in the context even when request signing is skipped.
	ctx := accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "key", Secret: "secret"})
	expectedBalances := map[asset.Item]accounts.CurrencyBalances{
		asset.Spot: {
			currency.BTC: {Currency: currency.BTC, Total: 3, Hold: 1, Free: 2},
		},
		asset.Margin: {
			currency.BTC:  {Currency: currency.BTC, Total: 3, Hold: 1, Free: 2, AvailableWithoutBorrow: 1.5, Borrowed: 0.5},
			currency.USDT: {Currency: currency.USDT, Total: 5, Hold: 2, Free: 3, AvailableWithoutBorrow: 2, Borrowed: 1},
		},
		asset.CrossMargin: {
			currency.BTC: {Currency: currency.BTC, Total: 3, Hold: 1, Free: 2, AvailableWithoutBorrow: 1.4, Borrowed: 0.6},
		},
		asset.CoinMarginedFutures: {
			currency.BTC: {Currency: currency.BTC, Total: 3, Hold: 1, Free: 2},
		},
		asset.USDTMarginedFutures: {
			currency.USDT: {Currency: currency.USDT, Total: 3, Hold: 1, Free: 2},
		},
		asset.DeliveryFutures: {
			currency.USDT: {Currency: currency.USDT, Total: 3, Hold: 1, Free: 2},
		},
		asset.Options: {
			currency.USDT: {Currency: currency.USDT, Total: 3, Hold: 1, Free: 2},
		},
	}
	for _, a := range []asset.Item{asset.Spot, asset.Margin, asset.CrossMargin, asset.CoinMarginedFutures, asset.USDTMarginedFutures, asset.DeliveryFutures, asset.Options} {
		subAccounts, err := ex.UpdateAccountBalances(ctx, a)
		require.NoErrorf(t, err, "UpdateAccountBalances must not error for %s", a)
		require.Lenf(t, subAccounts, 1, "UpdateAccountBalances must return one subaccount for %s", a)
		assert.Equalf(t, a, subAccounts[0].AssetType, "subaccount asset should match for %s", a)
		assert.Emptyf(t, subAccounts[0].ID, "subaccount ID should be empty for %s", a)
		for ccy, balance := range subAccounts[0].Balances {
			assert.Falsef(t, balance.UpdatedAt.IsZero(), "balance update time should be populated for %s %s", a, ccy)
			balance.UpdatedAt = time.Time{}
			subAccounts[0].Balances[ccy] = balance
		}
		assert.Equalf(t, expectedBalances[a], subAccounts[0].Balances, "balances should match for %s", a)
	}

	malformed := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/margin/user/account" {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, `[{"base":{},"quote":{}}]`)
		assert.NoError(t, err, "writing malformed margin response should not error")
	}))
	_, err := malformed.UpdateAccountBalances(t.Context(), asset.Margin)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "UpdateAccountBalances must return malformed balance errors")

	failing := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := responses[r.URL.Path]; !ok {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, `{`)
		assert.NoError(t, err, "writing invalid response should not error")
	}))
	for _, a := range []asset.Item{asset.Spot, asset.Margin, asset.CrossMargin, asset.CoinMarginedFutures, asset.USDTMarginedFutures, asset.DeliveryFutures, asset.Options} {
		_, err = failing.UpdateAccountBalances(ctx, a)
		require.Errorf(t, err, "UpdateAccountBalances must return the decode error for %s", a)
	}

	_, err = ex.UpdateAccountBalances(t.Context(), asset.Empty)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateAccountBalances must reject an unsupported asset")

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		for _, a := range e.GetAssetTypes(false) {
			_, err := e.UpdateAccountBalances(t.Context(), a)
			require.NoErrorf(t, err, "UpdateAccountBalances must not error for %s against the live API", a)
		}
	})
}

func TestSetCrossMarginAccountBalances(t *testing.T) {
	t.Parallel()

	balances := accounts.CurrencyBalances{}
	setCrossMarginAccountBalances(&balances, nil)

	setCrossMarginAccountBalances(&balances, &CrossMarginAccount{
		Balances: map[string]CrossMarginCurrencyBalance{
			"BTC": {
				Available: types.Number(2),
				Freeze:    types.Number(0.5),
				Borrowed:  types.Number(0.25),
				Interest:  types.Number(0.05),
			},
		},
	})

	got := balances[currency.BTC]
	assert.InDelta(t, 2.5, got.Total, 0.00000001, "total should include available and frozen balances")
	assert.InDelta(t, 0.5, got.Hold, 0.00000001, "hold should match frozen balance")
	assert.InDelta(t, 2, got.Free, 0.00000001, "free should match available balance")
	assert.InDelta(t, 0.3, got.Borrowed, 0.00000001, "borrowed should include principal and interest")
	assert.InDelta(t, 1.7, got.AvailableWithoutBorrow, 0.00000001, "available without borrow should subtract borrowed principal and interest")
}

func TestSetIsolatedMarginAccountBalances(t *testing.T) {
	t.Parallel()

	err := setIsolatedMarginAccountBalances(&accounts.CurrencyBalances{}, []MarginAccountItem{{}})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	err = setIsolatedMarginAccountBalances(&accounts.CurrencyBalances{}, []MarginAccountItem{{
		Base: AccountBalanceInformation{Currency: currency.BTC},
	}})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	err = setIsolatedMarginAccountBalances(&accounts.CurrencyBalances{}, []MarginAccountItem{{
		AccountType: "inactive",
	}})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	balances := accounts.CurrencyBalances{}
	err = setIsolatedMarginAccountBalances(&balances, []MarginAccountItem{
		{
			Base: AccountBalanceInformation{
				Currency:     currency.BTC,
				Available:    types.Number(1),
				LockedAmount: types.Number(0.2),
				Borrowed:     types.Number(0.25),
			},
			Quote: AccountBalanceInformation{
				Currency:     currency.USDT,
				Available:    types.Number(10),
				LockedAmount: types.Number(2),
				Borrowed:     types.Number(2),
			},
		},
		{
			Base: AccountBalanceInformation{
				Currency:     currency.BTC,
				Available:    types.Number(3),
				LockedAmount: types.Number(0.4),
				Borrowed:     types.Number(0.5),
			},
			Quote: AccountBalanceInformation{
				Currency:     currency.ETH,
				Available:    types.Number(5),
				LockedAmount: types.Number(0.6),
			},
		},
		{
			Base: AccountBalanceInformation{
				Currency:     currency.ETH,
				Available:    types.Number(7),
				LockedAmount: types.Number(0.8),
				Borrowed:     types.Number(1),
			},
			Quote: AccountBalanceInformation{
				Currency:     currency.USDT,
				Available:    types.Number(20),
				LockedAmount: types.Number(4),
			},
		},
	})
	require.NoError(t, err, "setIsolatedMarginAccountBalances must add valid isolated margin balances")

	btc := balances[currency.BTC]
	assert.InDelta(t, 4.6, btc.Total, 0.00000001, "BTC total should include all isolated margin markets")
	assert.InDelta(t, 0.6, btc.Hold, 0.00000001, "BTC hold should include all isolated margin markets")
	assert.InDelta(t, 4, btc.Free, 0.00000001, "BTC free should include all isolated margin markets")
	assert.InDelta(t, 0.75, btc.Borrowed, 0.00000001, "BTC borrowed should include principal from all isolated margin markets")
	assert.InDelta(t, 3.25, btc.AvailableWithoutBorrow, 0.00000001, "BTC available without borrow should subtract borrowed principal")

	usdt := balances[currency.USDT]
	assert.InDelta(t, 36, usdt.Total, 0.00000001, "USDT total should include all isolated margin markets")
	assert.InDelta(t, 6, usdt.Hold, 0.00000001, "USDT hold should include all isolated margin markets")
	assert.InDelta(t, 30, usdt.Free, 0.00000001, "USDT free should include all isolated margin markets")
	assert.InDelta(t, 2, usdt.Borrowed, 0.00000001, "USDT borrowed should include principal from all isolated margin markets")
	assert.InDelta(t, 28, usdt.AvailableWithoutBorrow, 0.00000001, "USDT available without borrow should subtract borrowed principal")

	eth := balances[currency.ETH]
	assert.InDelta(t, 13.4, eth.Total, 0.00000001, "ETH total should include base and quote isolated margin entries")
	assert.InDelta(t, 1.4, eth.Hold, 0.00000001, "ETH hold should include base and quote isolated margin entries")
	assert.InDelta(t, 12, eth.Free, 0.00000001, "ETH free should include base and quote isolated margin entries")
	assert.InDelta(t, 1, eth.Borrowed, 0.00000001, "ETH borrowed should include principal from all isolated margin markets")
	assert.InDelta(t, 11, eth.AvailableWithoutBorrow, 0.00000001, "ETH available without borrow should subtract borrowed principal")
}

func TestAddIsolatedMarginAccountBalanceWithNegativeAvailable(t *testing.T) {
	t.Parallel()
	balances := accounts.CurrencyBalances{}
	err := addIsolatedMarginAccountBalance(&balances, AccountBalanceInformation{
		Currency:  currency.LRC,
		Available: types.Number(-0.01462404),
		Borrowed:  types.Number(4.85),
	})
	require.NoError(t, err, "addIsolatedMarginAccountBalance must add a valid isolated margin balance")

	lrc := balances[currency.LRC]
	assert.InDelta(t, -0.01462404, lrc.Total, 0.00000001, "total should preserve the exchange-reported negative available balance")
	assert.InDelta(t, -0.01462404, lrc.Free, 0.00000001, "free should preserve the exchange-reported negative available balance")
	assert.InDelta(t, 4.85, lrc.Borrowed, 0.00000001, "borrowed should include the outstanding principal")
	assert.InDelta(t, -4.86462404, lrc.AvailableWithoutBorrow, 0.00000001, "available without borrow should account for the outstanding principal")
}

func TestWithdraw(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	cryptocurrencyChains, err := e.GetAvailableTransferChains(t.Context(), currency.BTC)
	require.NoError(t, err, "GetAvailableTransferChains must not error")
	require.NotEmpty(t, cryptocurrencyChains, "GetAvailableTransferChains must return some chains")
	withdrawCryptoRequest := withdraw.Request{
		Exchange:    e.Name,
		Amount:      -0.1,
		Currency:    currency.BTC,
		Description: "WITHDRAW IT ALL",
		Crypto: withdraw.CryptoRequest{
			Address: core.BitcoinDonationAddress,
			Chain:   cryptocurrencyChains[0],
		},
	}
	_, err = e.WithdrawCryptocurrencyFunds(t.Context(), &withdrawCryptoRequest)
	require.NoError(t, err)
}

func TestGetOrderInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	for _, a := range e.GetAssetTypes(false) {
		_, err := e.GetOrderInfo(t.Context(), "917591554", getPair(t, a), a)
		require.NoErrorf(t, err, "GetOrderInfo must not error for asset %s", a)
	}
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	for _, a := range e.GetAssetTypes(false) {
		_, err := e.UpdateTicker(t.Context(), getPair(t, a), a)
		assert.NoErrorf(t, err, "UpdateTicker should not error for %s", a)
	}

	pair := currency.NewPairWithDelimiter("BTC", "USD", currency.UnderscoreDelimiter)
	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/futures/btc/tickers", `[{"contract":"BTC_USD","last":"2"}]`)
	updated, err := ex.UpdateTicker(t.Context(), pair, asset.CoinMarginedFutures)
	require.NoError(t, err, "UpdateTicker must not error for coin-margined futures")
	assert.Equal(t, 2.0, updated.Last, "last price should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestContractQueryKey: {pair.String()}}, gotRequest.query, "request query should match")
}

func TestListSpotCurrencies(t *testing.T) {
	t.Parallel()
	if _, err := e.ListSpotCurrencies(t.Context()); err != nil {
		t.Errorf("%s ListAllCurrencies() error %v", e.Name, err)
	}
}

func TestGetCurrencyDetail(t *testing.T) {
	t.Parallel()
	if _, err := e.GetCurrencyDetail(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s GetCurrencyDetail() error %v", e.Name, err)
	}
}

func TestListAllCurrencyPairs(t *testing.T) {
	t.Parallel()
	if _, err := e.ListSpotCurrencyPairs(t.Context()); err != nil {
		t.Errorf("%s ListAllCurrencyPairs() error %v", e.Name, err)
	}
}

func TestGetCurrencyPairDetal(t *testing.T) {
	t.Parallel()
	if _, err := e.GetCurrencyPairDetail(t.Context(), currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter}.String()); err != nil {
		t.Errorf("%s GetCurrencyPairDetal() error %v", e.Name, err)
	}
}

func TestGetTickers(t *testing.T) {
	t.Parallel()
	if _, err := e.GetTickers(t.Context(), gateIOTestBTCUSDT, ""); err != nil {
		t.Errorf("%s GetTickers() error %v", e.Name, err)
	}
}

func TestGetTicker(t *testing.T) {
	t.Parallel()
	if _, err := e.GetTicker(t.Context(), currency.Pair{Base: currency.BTC, Delimiter: currency.UnderscoreDelimiter, Quote: currency.USDT}.String(), utc8TimeZone); err != nil {
		t.Errorf("%s GetTicker() error %v", e.Name, err)
	}
}

func TestGetOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetOrderbook(t.Context(), getPair(t, asset.Spot).String(), "0.1", 10, false)
	assert.NoError(t, err, "GetOrderbook should not error")
}

func TestGetMarketTrades(t *testing.T) {
	t.Parallel()

	_, err := e.GetMarketTrades(t.Context(), currency.EMPTYPAIR, 0, "", false, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetMarketTrades must reject an empty pair")

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetMarketTrades(t.Context(), pair, 0, "", false, from, from.Add(-time.Minute), 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetMarketTrades must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/spot/trades", `[{"id":"7","side":"buy","price":"2"}]`)
	trades, err := ex.GetMarketTrades(t.Context(), pair, 25, "last", true, from, to, 2)
	require.NoError(t, err, "GetMarketTrades must not error")
	require.Len(t, trades, 1, "GetMarketTrades must decode one trade")
	assert.Equal(t, int64(7), trades[0].ID, "trade ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodGet, gotRequest.method, "request method should be GET")
	assert.Equal(t, "/api/v4/spot/trades", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:         {"1700000000"},
		"last_id":                      {"last"},
		gateIOTestLimitQueryKey:        {"25"},
		"page":                         {"2"},
		"reverse":                      {"true"},
		"to":                           {"1700000060"},
	}, gotRequest.query, "request query should match")

	trades, err = ex.GetMarketTrades(t.Context(), pair, 0, "", false, time.Time{}, time.Time{}, 0)
	require.NoError(t, err, "GetMarketTrades must accept omitted optional parameters")
	require.Len(t, trades, 1, "GetMarketTrades must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/spot/trades", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetMarketTrades(ctx, pair, 0, "", false, time.Time{}, time.Time{}, 0)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetMarketTrades(t.Context(), getPair(t, asset.Spot), 0, "", true, time.Time{}, time.Time{}, 1)
		require.NoError(t, err, "GetMarketTrades must not error against the live API")
	})
}

func TestCandlestickUnmarshalJSON(t *testing.T) {
	t.Parallel()
	data := []byte(`[["1738108800","229534412.73508700","103734.3","104779.9","101336.6","101343.8","2232.94510000","true"],["1738195200","178316032.62306100","104718.6","106467.1","103286.4","103734.4","1695.00787000","true"],["1738281600","231315376.16747100","102431","106042.7","101555.9","104718.6","2228.03609000","true"]]`)
	var targets []Candlestick
	err := json.Unmarshal(data, &targets)
	require.NoError(t, err)
	require.Len(t, targets, 3)
	assert.Equal(t, Candlestick{
		Timestamp:      types.Time(time.Unix(1738108800, 0)),
		QuoteCcyVolume: 229534412.73508700,
		ClosePrice:     103734.3,
		HighestPrice:   104779.9,
		LowestPrice:    101336.6,
		OpenPrice:      101343.8,
		BaseCcyAmount:  2232.94510000,
		WindowClosed:   true,
	}, targets[0])
}

func TestGetCandlesticks(t *testing.T) {
	t.Parallel()

	_, err := e.GetCandlesticks(t.Context(), currency.EMPTYPAIR, 0, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetCandlesticks must reject an empty pair")

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	_, err = e.GetCandlesticks(t.Context(), pair, 0, time.Time{}, time.Time{}, kline.ThreeDay)
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "GetCandlesticks must reject an unsupported interval")

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetCandlesticks(t.Context(), pair, 0, from, from.Add(-time.Minute), 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetCandlesticks must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/spot/candlesticks", `[["1738108800","1","2","3","0.5","1.5","4","true"]]`)
	candlesticks, err := ex.GetCandlesticks(t.Context(), pair, 25, from, to, kline.OneDay)
	require.NoError(t, err, "GetCandlesticks must not error")
	require.Len(t, candlesticks, 1, "GetCandlesticks must decode one candlestick")
	assert.Equal(t, types.Number(2), candlesticks[0].ClosePrice, "close price should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/spot/candlesticks", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:         {"1700000000"},
		gateIOTestIntervalQueryKey:     {"1d"},
		gateIOTestLimitQueryKey:        {"25"},
		"to":                           {"1700000060"},
	}, gotRequest.query, "request query should match")

	candlesticks, err = ex.GetCandlesticks(t.Context(), pair, 0, time.Time{}, time.Time{}, 0)
	require.NoError(t, err, "GetCandlesticks must accept omitted optional parameters")
	require.Len(t, candlesticks, 1, "GetCandlesticks must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/spot/candlesticks", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetCandlesticks(ctx, pair, 0, time.Time{}, time.Time{}, kline.OneDay)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetCandlesticks(t.Context(), getPair(t, asset.Spot), 0, time.Time{}, time.Time{}, kline.OneDay)
		require.NoError(t, err, "GetCandlesticks must not error against the live API")
	})
}

func TestGetTradingFeeRatio(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetTradingFeeRatio(t.Context(), currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter}); err != nil {
		t.Errorf("%s GetTradingFeeRatio() error %v", e.Name, err)
	}
}

func TestGetSpotAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSpotAccounts(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s GetSpotAccounts() error %v", e.Name, err)
	}
}

func TestCreateBatchOrders(t *testing.T) {
	t.Parallel()

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	valid := CreateOrderRequest{CurrencyPair: pair, Side: "BUY", Amount: 1, Price: 2, Account: spotAccount, Type: gateIOTestLimitOrderType}
	tooMany := make([]CreateOrderRequest, 11)
	for i := range tooMany {
		tooMany[i] = valid
	}

	for _, tc := range []struct {
		name string
		args []CreateOrderRequest
		err  error
	}{
		{name: "too many orders", args: tooMany, err: errMultipleOrders},
		{name: "different accounts", args: []CreateOrderRequest{valid, {CurrencyPair: pair, Side: gateIOTestBuySide, Amount: 1, Price: 2, Account: marginAccount, Type: gateIOTestLimitOrderType}}, err: errDifferentAccount},
		{name: "empty pair", args: []CreateOrderRequest{{Side: gateIOTestBuySide, Amount: 1, Price: 2, Account: spotAccount, Type: gateIOTestLimitOrderType}}, err: currency.ErrCurrencyPairEmpty},
		{name: "invalid type", args: []CreateOrderRequest{{CurrencyPair: pair, Side: gateIOTestBuySide, Amount: 1, Price: 2, Account: spotAccount, Type: "market"}}, err: errOrderTypeNotLimit},
		{name: "invalid side", args: []CreateOrderRequest{{CurrencyPair: pair, Side: "HOLD", Amount: 1, Price: 2, Account: spotAccount, Type: gateIOTestLimitOrderType}}, err: order.ErrSideIsInvalid},
		{name: "invalid account", args: []CreateOrderRequest{{CurrencyPair: pair, Side: gateIOTestBuySide, Amount: 1, Price: 2, Account: futuresAccount, Type: gateIOTestLimitOrderType}}, err: errOrderAccountInvalid},
		{name: "invalid amount", args: []CreateOrderRequest{{CurrencyPair: pair, Side: gateIOTestBuySide, Price: 2, Account: spotAccount, Type: gateIOTestLimitOrderType}}, err: errInvalidAmount},
		{name: "invalid price", args: []CreateOrderRequest{{CurrencyPair: pair, Side: gateIOTestBuySide, Amount: 1, Account: spotAccount, Type: gateIOTestLimitOrderType}}, err: errInvalidPrice},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := slices.Clone(tc.args)
			_, err := e.CreateBatchOrders(t.Context(), tc.args)
			require.ErrorIs(t, err, tc.err, "CreateBatchOrders must return the expected error")
			assert.Equal(t, original, tc.args, "request arguments should remain unchanged")
		})
	}

	ex, requests := setupGateIOHTTPTest(t, http.MethodPost, "/api/v4/spot/batch_orders", `[{"id":"order-1","succeeded":true}]`)
	orderRequests := []CreateOrderRequest{valid, valid}
	orders, err := ex.CreateBatchOrders(t.Context(), orderRequests)
	require.NoError(t, err, "CreateBatchOrders must not error")
	require.Len(t, orders, 1, "CreateBatchOrders must decode one order")
	assert.Equal(t, "order-1", orders[0].OrderID, "order ID should match")
	assert.True(t, orders[0].Succeeded, "order result should report success")
	assert.Equal(t, "BUY", orderRequests[0].Side, "request side should remain unchanged")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodPost, gotRequest.method, "request method should be POST")
	assert.Equal(t, "/api/v4/spot/batch_orders", gotRequest.path, "request path should match")
	assert.JSONEq(t, `[{"currency_pair":"BTC_USDT","type":"limit","account":"spot","side":"buy","amount":"1","price":"2"},{"currency_pair":"BTC_USDT","type":"limit","account":"spot","side":"buy","amount":"1","price":"2"}]`, string(gotRequest.body), "request body should match")

	requireGateIORequestErrors(t, "/api/v4/spot/batch_orders", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.CreateBatchOrders(ctx, []CreateOrderRequest{valid})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_BATCH_ORDERS")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveBatchOrders](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_BATCH_ORDERS"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_BATCH_ORDERS must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_BATCH_ORDERS must set dedicated_test_account=true")
		require.NotEmpty(t, config.Orders, "GCT_GATEIO_LIVE_BATCH_ORDERS must contain at least one order")
		cleanup := make([]gateIOLiveSpotOrderCleanup, len(config.Orders))
		for i := range config.Orders {
			require.NoErrorf(t, prepareGateIOLiveSpotOrder(&config.Orders[i]), "GCT_GATEIO_LIVE_BATCH_ORDERS order %d must be a safe post-only fixture", i)
			cleanup[i] = gateIOLiveSpotOrderCleanup{text: config.Orders[i].Text, currencyPair: config.Orders[i].CurrencyPair}
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, cleanupGateIOLiveSpotOrders(ctx, e.GetSpotOrders, e.CancelSingleSpotOrder, cleanup, gateIOLiveReconciliationPollInterval, gateIOLiveOrderCleanupPollAttempts), "CreateBatchOrders live cleanup should reconcile fixture orders")
		})
		orders, err := e.CreateBatchOrders(t.Context(), config.Orders)
		require.NoError(t, err, "CreateBatchOrders must not error against the live API")
		require.Len(t, orders, len(config.Orders), "CreateBatchOrders must return one result per live request")
		for i := range orders {
			require.Truef(t, orders[i].Succeeded, "CreateBatchOrders live order %d must succeed", i)
			require.NotEmptyf(t, orders[i].OrderID, "CreateBatchOrders live order %d must return an order ID", i)
			require.NotEmptyf(t, orders[i].Text, "CreateBatchOrders live order %d must return its correlation ID", i)
			configIndex := slices.IndexFunc(config.Orders, func(arg CreateOrderRequest) bool { return arg.Text == orders[i].Text })
			require.NotEqualf(t, -1, configIndex, "CreateBatchOrders live order %d must match a configured correlation ID", i)
			cleanup[configIndex].orderID = orders[i].OrderID
			if orders[i].CurrencyPair != "" {
				responsePair, err := currency.NewPairFromString(orders[i].CurrencyPair)
				require.NoErrorf(t, err, "CreateBatchOrders live order %d response pair must parse", i)
				cleanup[configIndex].currencyPair = responsePair
			}
			if orders[i].Account != "" {
				require.Equalf(t, spotAccount, orders[i].Account, "CreateBatchOrders live order %d response account must remain spot", i)
			}
		}
	})
}

func TestGetSpotOpenOrders(t *testing.T) {
	t.Parallel()

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/spot/open_orders", `[{"currency_pair":"BTC_USDT","total":"1","orders":[{"id":"order-1"}]}]`)
	orders, err := ex.GetSpotOpenOrders(t.Context(), 2, 25, true)
	require.NoError(t, err, "GetSpotOpenOrders must not error")
	require.Len(t, orders, 1, "GetSpotOpenOrders must decode one order group")
	assert.Equal(t, gateIOTestBTCUSDT, orders[0].CurrencyPair, "currency pair should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodGet, gotRequest.method, "request method should be GET")
	assert.Equal(t, "/api/v4/spot/open_orders", gotRequest.path, "request path should match")
	assert.Equal(t, "2", gotRequest.query.Get("page"), "page should match")
	assert.Equal(t, "25", gotRequest.query.Get(gateIOTestLimitQueryKey), "limit should match")
	assert.Equal(t, crossMarginAccount, gotRequest.query.Get("account"), "account should match")

	orders, err = ex.GetSpotOpenOrders(t.Context(), 0, 0, false)
	require.NoError(t, err, "GetSpotOpenOrders must accept omitted optional parameters")
	require.Len(t, orders, 1, "GetSpotOpenOrders must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/spot/open_orders", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetSpotOpenOrders(ctx, 0, 0, false)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetSpotOpenOrders(t.Context(), 0, 0, false)
		require.NoError(t, err, "GetSpotOpenOrders must not error against the live API")
	})
}

func TestSpotClosePositionWhenCrossCurrencyDisabled(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.SpotClosePositionWhenCrossCurrencyDisabled(t.Context(), &ClosePositionRequestParam{
		Amount:       0.1,
		Price:        1234567384,
		CurrencyPair: getPair(t, asset.Spot),
	}); err != nil {
		t.Errorf("%s SpotClosePositionWhenCrossCurrencyDisabled() error %v", e.Name, err)
	}
}

func TestPlaceSpotOrder(t *testing.T) {
	t.Parallel()

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	_, err := e.PlaceSpotOrder(t.Context(), nil)
	require.ErrorIs(t, err, errNilArgument, "PlaceSpotOrder must reject nil input")
	_, err = e.PlaceSpotOrder(t.Context(), &CreateOrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "PlaceSpotOrder must reject an empty pair")
	invalidSide := &CreateOrderRequest{CurrencyPair: pair, Side: "HOLD", Account: spotAccount, Amount: 1}
	_, err = e.PlaceSpotOrder(t.Context(), invalidSide)
	require.ErrorIs(t, err, order.ErrSideIsInvalid, "PlaceSpotOrder must reject an invalid side")
	assert.Equal(t, "HOLD", invalidSide.Side, "request side should remain unchanged")
	_, err = e.PlaceSpotOrder(t.Context(), &CreateOrderRequest{CurrencyPair: pair, Side: gateIOTestBuySide, Account: futuresAccount, Amount: 1})
	require.ErrorIs(t, err, errOrderAccountInvalid, "PlaceSpotOrder must reject an invalid account")
	_, err = e.PlaceSpotOrder(t.Context(), &CreateOrderRequest{CurrencyPair: pair, Side: gateIOTestBuySide, Account: spotAccount})
	require.ErrorIs(t, err, errInvalidAmount, "PlaceSpotOrder must reject an invalid amount")
	_, err = e.PlaceSpotOrder(t.Context(), &CreateOrderRequest{CurrencyPair: pair, Side: gateIOTestBuySide, Account: spotAccount, Amount: 1, Price: -1})
	require.ErrorIs(t, err, errInvalidPrice, "PlaceSpotOrder must reject an invalid price")

	ex, requests := setupGateIOHTTPTest(t, http.MethodPost, "/api/v4/spot/orders", `{"id":"order-1","succeeded":true}`)
	orderRequest := &CreateOrderRequest{CurrencyPair: pair, Side: "SELL", Account: spotAccount, Amount: 1}
	placed, err := ex.PlaceSpotOrder(t.Context(), orderRequest)
	require.NoError(t, err, "PlaceSpotOrder must not error")
	require.NotNil(t, placed, "PlaceSpotOrder must decode an order")
	assert.Equal(t, "order-1", placed.OrderID, "order ID should match")
	assert.Equal(t, "SELL", orderRequest.Side, "request side should remain unchanged")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodPost, gotRequest.method, "request method should be POST")
	assert.Equal(t, "/api/v4/spot/orders", gotRequest.path, "request path should match")
	assert.JSONEq(t, `{"currency_pair":"BTC_USDT","account":"spot","side":"sell","amount":"1"}`, string(gotRequest.body), "request body should match")

	requireGateIORequestErrors(t, "/api/v4/spot/orders", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.PlaceSpotOrder(ctx, &CreateOrderRequest{CurrencyPair: pair, Side: "sell", Account: spotAccount, Amount: 1})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_SPOT_ORDER")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveSpotOrder](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_SPOT_ORDER"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_SPOT_ORDER must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_SPOT_ORDER must set dedicated_test_account=true")
		require.NoError(t, prepareGateIOLiveSpotOrder(&config.Order), "GCT_GATEIO_LIVE_SPOT_ORDER order must be a safe post-only fixture")
		cleanup := []gateIOLiveSpotOrderCleanup{{text: config.Order.Text, currencyPair: config.Order.CurrencyPair}}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, cleanupGateIOLiveSpotOrders(ctx, e.GetSpotOrders, e.CancelSingleSpotOrder, cleanup, gateIOLiveReconciliationPollInterval, gateIOLiveOrderCleanupPollAttempts), "PlaceSpotOrder live cleanup should reconcile the fixture order")
		})
		placed, err := e.PlaceSpotOrder(t.Context(), &config.Order)
		require.NoError(t, err, "PlaceSpotOrder must not error against the live API")
		require.NotNil(t, placed, "PlaceSpotOrder must return a live order")
		require.NotEmpty(t, placed.OrderID, "PlaceSpotOrder must return a live order ID")
		cleanup[0].orderID = placed.OrderID
	})
}

func TestGetSpotOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSpotOrders(t.Context(), currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter}, statusOpen, 0, 0)
	assert.NoError(t, err, "GetSpotOrders should not error")
}

func TestCancelAllOpenOrdersSpecifiedCurrencyPair(t *testing.T) {
	t.Parallel()

	_, err := e.CancelAllOpenOrdersSpecifiedCurrencyPair(t.Context(), currency.EMPTYPAIR, order.AnySide, asset.Empty)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "CancelAllOpenOrdersSpecifiedCurrencyPair must reject an empty pair")

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	_, err = e.CancelAllOpenOrdersSpecifiedCurrencyPair(t.Context(), pair, order.AnySide, asset.USDTMarginedFutures)
	require.ErrorIs(t, err, asset.ErrNotSupported, "CancelAllOpenOrdersSpecifiedCurrencyPair must reject an unsupported asset")

	ex, requests := setupGateIOHTTPTest(t, http.MethodDelete, "/api/v4/spot/orders", `[{"id":"order-1","succeeded":true}]`)
	cancelled, err := ex.CancelAllOpenOrdersSpecifiedCurrencyPair(t.Context(), pair, order.Sell, asset.Margin)
	require.NoError(t, err, "CancelAllOpenOrdersSpecifiedCurrencyPair must not error")
	require.Len(t, cancelled, 1, "CancelAllOpenOrdersSpecifiedCurrencyPair must decode one order")
	assert.Equal(t, "order-1", cancelled[0].OrderID, "order ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodDelete, gotRequest.method, "request method should be DELETE")
	assert.Equal(t, "/api/v4/spot/orders", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		"account":                      {marginAccount},
		gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT},
		"side":                         {"sell"},
	}, gotRequest.query, "request query should match")

	cancelled, err = ex.CancelAllOpenOrdersSpecifiedCurrencyPair(t.Context(), pair, order.AnySide, asset.Empty)
	require.NoError(t, err, "CancelAllOpenOrdersSpecifiedCurrencyPair must accept omitted optional parameters")
	require.Len(t, cancelled, 1, "CancelAllOpenOrdersSpecifiedCurrencyPair must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/spot/orders", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.CancelAllOpenOrdersSpecifiedCurrencyPair(ctx, pair, order.Sell, asset.Margin)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_CANCEL_ALL_SPOT_ORDERS")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveCancelAllSpotOrders](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_CANCEL_ALL_SPOT_ORDERS"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_CANCEL_ALL_SPOT_ORDERS must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_CANCEL_ALL_SPOT_ORDERS must set dedicated_test_account=true")
		require.NoError(t, prepareGateIOLiveSpotOrder(&config.Order), "GCT_GATEIO_LIVE_CANCEL_ALL_SPOT_ORDERS order must be a safe post-only fixture")
		require.Contains(t, []order.Side{order.Buy, order.Sell}, config.Side, "GCT_GATEIO_LIVE_CANCEL_ALL_SPOT_ORDERS side must be buy or sell")
		require.True(t, strings.EqualFold(config.Side.String(), config.Order.Side), "GCT_GATEIO_LIVE_CANCEL_ALL_SPOT_ORDERS order side must match the cancellation side")
		existing, err := e.GetSpotOrders(t.Context(), config.Order.CurrencyPair, statusOpen, 0, 0)
		require.NoError(t, err, "GetSpotOrders must check the live cancellation scope")
		require.Empty(t, existing, "GCT_GATEIO_LIVE_CANCEL_ALL_SPOT_ORDERS pair must have no existing open orders")
		cleanup := []gateIOLiveSpotOrderCleanup{{text: config.Order.Text, currencyPair: config.Order.CurrencyPair}}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, cleanupGateIOLiveSpotOrders(ctx, e.GetSpotOrders, e.CancelSingleSpotOrder, cleanup, gateIOLiveReconciliationPollInterval, gateIOLiveOrderCleanupPollAttempts), "CancelAllOpenOrdersSpecifiedCurrencyPair live cleanup should reconcile the fixture order")
		})
		placed, err := e.PlaceSpotOrder(t.Context(), &config.Order)
		require.NoError(t, err, "PlaceSpotOrder must create the live cancellation fixture")
		require.NotNil(t, placed, "PlaceSpotOrder must return the live cancellation fixture")
		require.NotEmpty(t, placed.OrderID, "PlaceSpotOrder must return the live cancellation fixture ID")
		cleanup[0].orderID = placed.OrderID
		openOrders, err := e.GetSpotOrders(t.Context(), config.Order.CurrencyPair, statusOpen, 0, 0)
		require.NoError(t, err, "GetSpotOrders must recheck the exact live cancellation scope")
		require.Len(t, openOrders, 1, "live cancellation scope must contain only the fixture order")
		require.Equal(t, placed.OrderID, openOrders[0].OrderID, "live cancellation scope order ID must match the fixture")
		require.Equal(t, config.Order.Text, openOrders[0].Text, "live cancellation scope correlation ID must match the fixture")
		cancelled, err := e.CancelAllOpenOrdersSpecifiedCurrencyPair(t.Context(), config.Order.CurrencyPair, config.Side, asset.Spot)
		require.NoError(t, err, "CancelAllOpenOrdersSpecifiedCurrencyPair must not error against the live API")
		fixtureCancelled := false
		for i := range cancelled {
			if cancelled[i].OrderID == placed.OrderID && cancelled[i].Succeeded {
				fixtureCancelled = true
				break
			}
		}
		require.True(t, fixtureCancelled, "CancelAllOpenOrdersSpecifiedCurrencyPair must cancel the fixture order")
	})
}

func TestCancelBatchOrdersWithIDList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CancelBatchOrdersWithIDList(t.Context(), []CancelOrderByIDParam{
		{
			CurrencyPair: getPair(t, asset.Spot),
			ID:           "1234567",
		},
		{
			CurrencyPair: currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter},
			ID:           "something",
		},
	}); err != nil {
		t.Errorf("%s CancelBatchOrderWithIDList() error %v", e.Name, err)
	}
}

func TestGetSpotOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSpotOrder(t.Context(), "1234", currency.Pair{
		Base:      currency.BTC,
		Delimiter: currency.UnderscoreDelimiter,
		Quote:     currency.USDT,
	}, asset.Spot); err != nil {
		t.Errorf("%s GetSpotOrder() error %v", e.Name, err)
	}
}

func TestAmendSpotOrder(t *testing.T) {
	t.Parallel()

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	_, err := e.AmendSpotOrder(t.Context(), "123", pair, false, nil)
	require.ErrorIs(t, err, errNilArgument, "AmendSpotOrder must reject nil input")
	_, err = e.AmendSpotOrder(t.Context(), "", pair, false, &PriceAndAmount{Price: 1})
	require.ErrorIs(t, err, errInvalidOrderID, "AmendSpotOrder must reject an empty order ID")
	_, err = e.AmendSpotOrder(t.Context(), "123", currency.EMPTYPAIR, false, &PriceAndAmount{Price: 1})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "AmendSpotOrder must reject an empty pair")
	_, err = e.AmendSpotOrder(t.Context(), "123", pair, false, &PriceAndAmount{Amount: 1, Price: 1})
	require.ErrorIs(t, err, errAmendAmountAndPriceSet, "AmendSpotOrder must reject simultaneous amount and price changes")

	ex, requests := setupGateIOHTTPTest(t, http.MethodPatch, "/api/v4/spot/orders/123", `{"id":"123","price":"1"}`)
	amended, err := ex.AmendSpotOrder(t.Context(), "123", pair, true, &PriceAndAmount{Price: 1})
	require.NoError(t, err, "AmendSpotOrder must not error")
	require.NotNil(t, amended, "AmendSpotOrder must decode an order")
	assert.Equal(t, "123", amended.OrderID, "order ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodPatch, gotRequest.method, "request method should be PATCH")
	assert.Equal(t, "/api/v4/spot/orders/123", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		"account":                      {crossMarginAccount},
		gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT},
	}, gotRequest.query, "request query should match")
	assert.JSONEq(t, `{"price":"1"}`, string(gotRequest.body), "request body should match")

	amended, err = ex.AmendSpotOrder(t.Context(), "123", pair, false, &PriceAndAmount{Price: 1})
	require.NoError(t, err, "AmendSpotOrder must accept an omitted account")
	require.NotNil(t, amended, "AmendSpotOrder must decode the response with an omitted account")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "a false cross-margin flag should omit the account")

	requireGateIORequestErrors(t, "/api/v4/spot/orders/123", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.AmendSpotOrder(ctx, "123", pair, false, &PriceAndAmount{Price: 1})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_AMEND_SPOT_ORDER")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveAmendSpotOrder](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_AMEND_SPOT_ORDER"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_AMEND_SPOT_ORDER must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_AMEND_SPOT_ORDER must set dedicated_test_account=true")
		require.NoError(t, prepareGateIOLiveSpotOrder(&config.Order), "GCT_GATEIO_LIVE_AMEND_SPOT_ORDER order must be a safe post-only fixture")
		require.NotEqual(t, config.Change.Amount != 0, config.Change.Price != 0, "GCT_GATEIO_LIVE_AMEND_SPOT_ORDER change must set exactly one of amount or price")
		cleanup := []gateIOLiveSpotOrderCleanup{{text: config.Order.Text, currencyPair: config.Order.CurrencyPair}}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, cleanupGateIOLiveSpotOrders(ctx, e.GetSpotOrders, e.CancelSingleSpotOrder, cleanup, gateIOLiveReconciliationPollInterval, gateIOLiveOrderCleanupPollAttempts), "AmendSpotOrder live cleanup should reconcile the fixture order")
		})
		placed, err := e.PlaceSpotOrder(t.Context(), &config.Order)
		require.NoError(t, err, "PlaceSpotOrder must create the live amendment fixture")
		require.NotNil(t, placed, "PlaceSpotOrder must return the live amendment fixture")
		require.NotEmpty(t, placed.OrderID, "PlaceSpotOrder must return the live amendment fixture ID")
		cleanup[0].orderID = placed.OrderID
		amended, err := e.AmendSpotOrder(t.Context(), placed.OrderID, config.Order.CurrencyPair, false, &config.Change)
		require.NoError(t, err, "AmendSpotOrder must not error against the live API")
		require.NotNil(t, amended, "AmendSpotOrder must return the amended fixture order")
		require.Equal(t, placed.OrderID, amended.OrderID, "AmendSpotOrder must amend the fixture order")
	})
}

func TestCancelSingleSpotOrder(t *testing.T) {
	t.Parallel()

	_, err := e.CancelSingleSpotOrder(t.Context(), "", gateIOTestBTCUSDT, false)
	require.ErrorIs(t, err, errInvalidOrderID, "CancelSingleSpotOrder must reject an empty order ID")
	_, err = e.CancelSingleSpotOrder(t.Context(), "123", "", false)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "CancelSingleSpotOrder must reject an empty pair")

	ex, requests := setupGateIOHTTPTest(t, http.MethodDelete, "/api/v4/spot/orders/123", `{"id":"123","status":"cancelled"}`)
	cancelled, err := ex.CancelSingleSpotOrder(t.Context(), "123", gateIOTestBTCUSDT, true)
	require.NoError(t, err, "CancelSingleSpotOrder must not error")
	require.NotNil(t, cancelled, "CancelSingleSpotOrder must decode an order")
	assert.Equal(t, "123", cancelled.OrderID, "order ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodDelete, gotRequest.method, "request method should be DELETE")
	assert.Equal(t, "/api/v4/spot/orders/123", gotRequest.path, "request path should match")
	assert.Equal(t, gateIOTestBTCUSDT, gotRequest.query.Get(gateIOTestCurrencyPairQueryKey), "currency pair should match")
	assert.Equal(t, crossMarginAccount, gotRequest.query.Get("account"), "account should match")

	cancelled, err = ex.CancelSingleSpotOrder(t.Context(), "123", gateIOTestBTCUSDT, false)
	require.NoError(t, err, "CancelSingleSpotOrder must accept an omitted account")
	require.NotNil(t, cancelled, "CancelSingleSpotOrder must decode the response with an omitted account")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestCurrencyPairQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "a false cross-margin flag should omit the account")

	requireGateIORequestErrors(t, "/api/v4/spot/orders/123", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.CancelSingleSpotOrder(ctx, "123", gateIOTestBTCUSDT, false)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_CANCEL_SINGLE_SPOT_ORDER")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveCancelSingleSpotOrder](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_CANCEL_SINGLE_SPOT_ORDER"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_CANCEL_SINGLE_SPOT_ORDER must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_CANCEL_SINGLE_SPOT_ORDER must set dedicated_test_account=true")
		require.NoError(t, prepareGateIOLiveSpotOrder(&config.Order), "GCT_GATEIO_LIVE_CANCEL_SINGLE_SPOT_ORDER order must be a safe post-only fixture")
		cleanup := []gateIOLiveSpotOrderCleanup{{text: config.Order.Text, currencyPair: config.Order.CurrencyPair}}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, cleanupGateIOLiveSpotOrders(ctx, e.GetSpotOrders, e.CancelSingleSpotOrder, cleanup, gateIOLiveReconciliationPollInterval, gateIOLiveOrderCleanupPollAttempts), "CancelSingleSpotOrder live cleanup should reconcile the fixture order")
		})
		placed, err := e.PlaceSpotOrder(t.Context(), &config.Order)
		require.NoError(t, err, "PlaceSpotOrder must create the live cancellation fixture")
		require.NotNil(t, placed, "PlaceSpotOrder must return the live cancellation fixture")
		require.NotEmpty(t, placed.OrderID, "PlaceSpotOrder must return the live cancellation fixture ID")
		cleanup[0].orderID = placed.OrderID
		cancelled, err := e.CancelSingleSpotOrder(t.Context(), placed.OrderID, config.Order.CurrencyPair.String(), false)
		require.NoError(t, err, "CancelSingleSpotOrder must not error against the live API")
		require.NotNil(t, cancelled, "CancelSingleSpotOrder must return the cancelled fixture order")
		require.Equal(t, placed.OrderID, cancelled.OrderID, "CancelSingleSpotOrder must cancel the fixture order")
	})
}

func TestGetMySpotTradingHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMySpotTradingHistory(t.Context(), currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter}, "", 0, 0, time.Time{}, time.Time{})
	require.NoError(t, err)
}

func TestGetServerTime(t *testing.T) {
	t.Parallel()
	if _, err := e.GetServerTime(t.Context(), asset.Spot); err != nil {
		t.Errorf("%s GetServerTime() error %v", e.Name, err)
	}
}

func TestCountdownCancelSpotOrders(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		requiresAuth bool
		arg          CountdownCancelOrderParam
		expectedErr  error
	}{
		{
			name:         "valid",
			requiresAuth: true,
			arg: CountdownCancelOrderParam{
				Timeout:      10,
				CurrencyPair: currency.Pair{Base: currency.BTC, Quote: currency.ETH, Delimiter: currency.UnderscoreDelimiter},
			},
		},
		{
			name: "timeout_zero",
			arg: CountdownCancelOrderParam{
				Timeout:      0,
				CurrencyPair: currency.Pair{Base: currency.BTC, Quote: currency.ETH, Delimiter: currency.UnderscoreDelimiter},
			},
			expectedErr: errInvalidCountdown,
		},
		{
			name: "timeout_negative",
			arg: CountdownCancelOrderParam{
				Timeout:      -1,
				CurrencyPair: currency.Pair{Base: currency.BTC, Quote: currency.ETH, Delimiter: currency.UnderscoreDelimiter},
			},
			expectedErr: errInvalidCountdown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.requiresAuth {
				sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
			}
			_, err := e.CountdownCancelSpotOrders(t.Context(), tc.arg)
			if tc.expectedErr != nil {
				assert.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCreatePriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CreatePriceTriggeredOrder(t.Context(), &PriceTriggeredOrderParam{
		Trigger: TriggerPriceInfo{
			Price:      123,
			Rule:       ">=",
			Expiration: 3600,
		},
		Put: PutOrderData{
			Type:        gateIOTestLimitOrderType,
			Side:        "sell",
			Price:       2312312,
			Amount:      30,
			TimeInForce: "gtc",
		},
		Market: currency.Pair{Base: currency.GT, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter},
	}); err != nil {
		t.Errorf("%s CreatePriceTriggeredOrder() error %v", e.Name, err)
	}
}

func TestGetPriceTriggeredOrderList(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetPriceTriggeredOrderList(t.Context(), statusOpen, currency.EMPTYPAIR, asset.Empty, 0, 0)
	assert.NoError(t, err, "GetPriceTriggeredOrderList should not error")
}

func TestCancelAllOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CancelMultipleSpotOpenOrders(t.Context(), currency.EMPTYPAIR, asset.CrossMargin); err != nil {
		t.Errorf("%s CancelAllOpenOrders() error %v", e.Name, err)
	}
}

func TestGetSinglePriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSinglePriceTriggeredOrder(t.Context(), "1234"); err != nil {
		t.Errorf("%s GetSinglePriceTriggeredOrder() error %v", e.Name, err)
	}
}

func TestCancelPriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CancelPriceTriggeredOrder(t.Context(), "1234"); err != nil {
		t.Errorf("%s CancelPriceTriggeredOrder() error %v", e.Name, err)
	}
}

func TestMarginLoan(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.MarginLoan(t.Context(), &MarginLoanRequestParam{
		Side:         "borrow",
		Amount:       1,
		Currency:     currency.BTC,
		CurrencyPair: currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter},
		Days:         10,
		Rate:         0.0002,
	}); err != nil {
		t.Errorf("%s MarginLoan() error %v", e.Name, err)
	}
}

func TestGetMarginAllLoans(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMarginAllLoans(t.Context(), statusOpen, "lend", "", currency.BTC, currency.Pair{Base: currency.BTC, Delimiter: currency.UnderscoreDelimiter, Quote: currency.USDT}, false, 0, 0)
	assert.NoError(t, err, "GetMarginAllLoans should not error")
}

func TestMergeMultipleLendingLoans(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.MergeMultipleLendingLoans(t.Context(), currency.USDT, []string{"123", "23423"}); err != nil {
		t.Errorf("%s MergeMultipleLendingLoans() error %v", e.Name, err)
	}
}

func TestRetrieveOneSingleLoanDetail(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.RetrieveOneSingleLoanDetail(t.Context(), "borrow", "123")
	assert.NoError(t, err, "RetrieveOneSingleLoanDetail should not error")
}

func TestModifyALoan(t *testing.T) {
	t.Parallel()
	_, err := e.ModifyALoan(t.Context(), "1234", &ModifyLoanRequestParam{
		Currency:  currency.BTC,
		Side:      "borrow",
		AutoRenew: false,
	})
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.ModifyALoan(t.Context(), "1234", &ModifyLoanRequestParam{
		Currency:     currency.BTC,
		Side:         "borrow",
		AutoRenew:    false,
		CurrencyPair: currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter},
	}); err != nil {
		t.Errorf("%s ModifyALoan() error %v", e.Name, err)
	}
}

func TestCancelLendingLoan(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.CancelLendingLoan(t.Context(), currency.BTC, "1234"); err != nil {
		t.Errorf("%s CancelLendingLoan() error %v", e.Name, err)
	}
}

func TestRepayALoan(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.RepayALoan(t.Context(), "1234", &RepayLoanRequestParam{
		CurrencyPair: currency.NewBTCUSDT(),
		Currency:     currency.BTC,
		Mode:         "all",
	}); err != nil {
		t.Errorf("%s RepayALoan() error %v", e.Name, err)
	}
}

func TestListLoanRepaymentRecords(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.ListLoanRepaymentRecords(t.Context(), "1234"); err != nil {
		t.Errorf("%s LoanRepaymentRecord() error %v", e.Name, err)
	}
}

func TestListRepaymentRecordsOfSpecificLoan(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.ListRepaymentRecordsOfSpecificLoan(t.Context(), "1234", "", 0, 0); err != nil {
		t.Errorf("%s error while ListRepaymentRecordsOfSpecificLoan() %v", e.Name, err)
	}
}

func TestGetOneSingleloanRecord(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetOneSingleLoanRecord(t.Context(), "1234", "123"); err != nil {
		t.Errorf("%s error while GetOneSingleloanRecord() %v", e.Name, err)
	}
}

func TestModifyALoanRecord(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.ModifyALoanRecord(t.Context(), "1234", &ModifyLoanRequestParam{
		Currency:     currency.USDT,
		CurrencyPair: currency.NewBTCUSDT(),
		Side:         "lend",
		AutoRenew:    true,
		LoanID:       "1234",
	}); err != nil {
		t.Errorf("%s ModifyALoanRecord() error %v", e.Name, err)
	}
}

func TestQueryInterestDeductionRecords(t *testing.T) {
	t.Parallel()

	_, err := e.QueryInterestDeductionRecords(t.Context(), currency.BTC, 0, 101, time.Time{}, time.Time{}, "")
	require.ErrorIs(t, err, errInvalidLimit)

	tn := time.Now()
	_, err = e.QueryInterestDeductionRecords(t.Context(), currency.BTC, 0, 0, tn.Add(time.Hour), tn, "")
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.QueryInterestDeductionRecords(t.Context(), currency.BTC, 0, 0, time.Time{}, time.Time{}, "invalid")
	require.ErrorIs(t, err, errInvalidLoanType)

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/unified/interest_records", `[{"currency":"BTC","interest":"1","type":"platform"}]`)
	records, err := ex.QueryInterestDeductionRecords(t.Context(), currency.BTC, 2, 100, time.Time{}, time.Time{}, "platform")
	require.NoError(t, err, "QueryInterestDeductionRecords must not error")
	require.Len(t, records, 1, "QueryInterestDeductionRecords must decode one record")
	assert.Equal(t, currency.BTC, records[0].Currency, "record currency should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/unified/interest_records", gotRequest.path, "request path should match")
	assert.Equal(t, "BTC", gotRequest.query.Get("currency"), "currency should match")
	assert.Equal(t, "2", gotRequest.query.Get("page"), "page should match")
	assert.Equal(t, "100", gotRequest.query.Get(gateIOTestLimitQueryKey), "limit should match")
	assert.Equal(t, "platform", gotRequest.query.Get(gateIOTestTypeQueryKey), "loan type should match")

	records, err = ex.QueryInterestDeductionRecords(t.Context(), currency.EMPTYCODE, 0, 0, time.Time{}, time.Time{}, "")
	require.NoError(t, err, "QueryInterestDeductionRecords must accept omitted optional parameters")
	require.Len(t, records, 1, "QueryInterestDeductionRecords must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/unified/interest_records", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.QueryInterestDeductionRecords(ctx, currency.BTC, 2, 100, time.Time{}, time.Time{}, "platform")
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.QueryInterestDeductionRecords(t.Context(), currency.EMPTYCODE, 0, 0, time.Time{}, time.Time{}, "")
		require.NoError(t, err, "QueryInterestDeductionRecords must not error against the live API")
	})
}

func TestCurrencySupportedByCrossMargin(t *testing.T) {
	t.Parallel()
	got, err := e.CurrencySupportedByCrossMargin(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, got)
}

func TestGetCrossMarginSupportedCurrencyDetail(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetCrossMarginSupportedCurrencyDetail(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s GetCrossMarginSupportedCurrencyDetail() error %v", e.Name, err)
	}
}

func TestGetCrossMarginAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetCrossMarginAccounts(t.Context()); err != nil {
		t.Errorf("%s GetCrossMarginAccounts() error %v", e.Name, err)
	}
}

func TestGetCrossMarginAccountChangeHistory(t *testing.T) {
	t.Parallel()

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err := e.GetCrossMarginAccountChangeHistory(t.Context(), currency.EMPTYCODE, from, from.Add(-time.Minute), 0, 0, "")
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetCrossMarginAccountChangeHistory must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/margin/cross/account_book", `[{"id":"record-1","currency":"BTC","type":"in"}]`)
	records, err := ex.GetCrossMarginAccountChangeHistory(t.Context(), currency.BTC, from, to, 2, 25, "in")
	require.NoError(t, err, "GetCrossMarginAccountChangeHistory must not error")
	require.Len(t, records, 1, "GetCrossMarginAccountChangeHistory must decode one record")
	assert.Equal(t, "record-1", records[0].ID, "record ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/margin/cross/account_book", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		"currency":              {"BTC"},
		gateIOTestFromQueryKey:  {"1700000000"},
		gateIOTestLimitQueryKey: {"25"},
		"page":                  {"2"},
		"to":                    {"1700000060"},
		gateIOTestTypeQueryKey:  {"in"},
	}, gotRequest.query, "request query should match")

	records, err = ex.GetCrossMarginAccountChangeHistory(t.Context(), currency.EMPTYCODE, time.Time{}, time.Time{}, 0, 0, "")
	require.NoError(t, err, "GetCrossMarginAccountChangeHistory must accept omitted optional parameters")
	require.Len(t, records, 1, "GetCrossMarginAccountChangeHistory must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/margin/cross/account_book", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetCrossMarginAccountChangeHistory(ctx, currency.BTC, time.Time{}, time.Time{}, 0, 0, "in")
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetCrossMarginAccountChangeHistory(t.Context(), currency.BTC, time.Time{}, time.Time{}, 0, 6, "in")
		require.NoError(t, err, "GetCrossMarginAccountChangeHistory must not error against the live API")
	})
}

var createCrossMarginBorrowLoanJSON = `{"id": "17",	"create_time": 1620381696159,	"update_time": 1620381696159,	"currency": "EOS",	"amount": "110.553635",	"text": "web",	"status": 2,	"repaid": "110.506649705159",	"repaid_interest": "0.046985294841",	"unpaid_interest": "0.0000074393366667"}`

func TestCreateCrossMarginBorrowLoan(t *testing.T) {
	t.Parallel()
	var response CrossMarginLoanResponse
	if err := json.Unmarshal([]byte(createCrossMarginBorrowLoanJSON), &response); err != nil {
		t.Errorf("%s error while deserialising to CrossMarginBorrowLoanResponse %v", e.Name, err)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CreateCrossMarginBorrowLoan(t.Context(), CrossMarginBorrowLoanParams{
		Currency: currency.BTC,
		Amount:   3,
	}); err != nil {
		t.Errorf("%s CreateCrossMarginBorrowLoan() error %v", e.Name, err)
	}
}

func TestGetCrossMarginBorrowHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetCrossMarginBorrowHistory(t.Context(), 1, currency.BTC, 0, 0, false); err != nil {
		t.Errorf("%s GetCrossMarginBorrowHistory() error %v", e.Name, err)
	}
}

func TestGetSingleBorrowLoanDetail(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSingleBorrowLoanDetail(t.Context(), "1234"); err != nil {
		t.Errorf("%s GetSingleBorrowLoanDetail() error %v", e.Name, err)
	}
}

func TestExecuteRepayment(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.ExecuteRepayment(t.Context(), CurrencyAndAmount{
		Currency: currency.USD,
		Amount:   1234.55,
	}); err != nil {
		t.Errorf("%s ExecuteRepayment() error %v", e.Name, err)
	}
}

func TestGetCrossMarginRepayments(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetCrossMarginRepayments(t.Context(), currency.BTC, "123", 0, 0, false); err != nil {
		t.Errorf("%s GetCrossMarginRepayments() error %v", e.Name, err)
	}
}

func TestGetMaxTransferableAmountForSpecificCrossMarginCurrency(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetMaxTransferableAmountForSpecificCrossMarginCurrency(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s GetMaxTransferableAmountForSpecificCrossMarginCurrency() error %v", e.Name, err)
	}
}

func TestGetMaxBorrowableAmountForSpecificCrossMarginCurrency(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetMaxBorrowableAmountForSpecificCrossMarginCurrency(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s GetMaxBorrowableAmountForSpecificCrossMarginCurrency() error %v", e.Name, err)
	}
}

func TestListCurrencyChain(t *testing.T) {
	t.Parallel()
	if _, err := e.ListCurrencyChain(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s ListCurrencyChain() error %v", e.Name, err)
	}
}

func TestGenerateCurrencyDepositAddress(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GenerateCurrencyDepositAddress(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s GenerateCurrencyDepositAddress() error %v", e.Name, err)
	}
}

func TestGetWithdrawalRecords(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetWithdrawalRecords(t.Context(), currency.BTC, time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Errorf("%s GetWithdrawalRecords() error %v", e.Name, err)
	}
}

func TestGetDepositRecords(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetDepositRecords(t.Context(), currency.BTC, time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Errorf("%s GetDepositRecords() error %v", e.Name, err)
	}
}

func TestTransferCurrency(t *testing.T) {
	t.Parallel()
	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)

	for _, tc := range []struct {
		name string
		arg  *TransferCurrencyParam
		err  error
	}{
		{name: "nil argument", err: errNilArgument},
		{name: "empty currency", arg: &TransferCurrencyParam{}, err: currency.ErrCurrencyCodeEmpty},
		{name: "empty from", arg: &TransferCurrencyParam{Currency: currency.BTC}, err: errTransferFromAccountRequired},
		{name: "empty to", arg: &TransferCurrencyParam{Currency: currency.BTC, From: spotAccount}, err: errTransferToAccountRequired},
		{name: "same account", arg: &TransferCurrencyParam{Currency: currency.BTC, From: spotAccount, To: spotAccount}, err: errTransferAccountsIdentical},
		{name: "to margin without pair", arg: &TransferCurrencyParam{Currency: currency.BTC, From: spotAccount, To: marginAccount}, err: errTransferPairRequired},
		{name: "from margin without pair", arg: &TransferCurrencyParam{Currency: currency.BTC, From: marginAccount, To: spotAccount}, err: errTransferPairRequired},
		{name: "to futures without settlement", arg: &TransferCurrencyParam{Currency: currency.BTC, From: spotAccount, To: futuresAccount}, err: errTransferSettlementRequired},
		{name: "from futures without settlement", arg: &TransferCurrencyParam{Currency: currency.BTC, From: futuresAccount, To: spotAccount}, err: errTransferSettlementRequired},
		{name: "invalid amount", arg: &TransferCurrencyParam{Currency: currency.BTC, From: spotAccount, To: optionsAccount}, err: errInvalidAmount},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := e.TransferCurrency(t.Context(), tc.arg)
			require.ErrorIs(t, err, tc.err, "TransferCurrency must return the expected error")
		})
	}

	ex, requests := setupGateIOHTTPTest(t, http.MethodPost, "/api/v4/wallet/transfers", `{"tx_id":7}`)
	transfer, err := ex.TransferCurrency(t.Context(), &TransferCurrencyParam{Currency: currency.BTC, From: spotAccount, To: marginAccount, Amount: 1, CurrencyPair: pair})
	require.NoError(t, err, "TransferCurrency must not error")
	require.NotNil(t, transfer, "TransferCurrency must decode a response")
	assert.Equal(t, int64(7), transfer.TransactionID, "transaction ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodPost, gotRequest.method, "request method should be POST")
	assert.Equal(t, "/api/v4/wallet/transfers", gotRequest.path, "request path should match")
	assert.JSONEq(t, `{"currency":"BTC","from":"spot","to":"margin","amount":"1","currency_pair":"BTC_USDT","settle":""}`, string(gotRequest.body), "request body should match")

	requireGateIORequestErrors(t, "/api/v4/wallet/transfers", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.TransferCurrency(ctx, &TransferCurrencyParam{Currency: currency.BTC, From: spotAccount, To: marginAccount, Amount: 1, CurrencyPair: pair})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_TRANSFER_CURRENCY")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveTransferCurrency](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_TRANSFER_CURRENCY"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_TRANSFER_CURRENCY must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_TRANSFER_CURRENCY must set dedicated_test_account=true")
		require.True(t,
			config.Request.From == spotAccount && config.Request.To == marginAccount || config.Request.From == marginAccount && config.Request.To == spotAccount,
			"GCT_GATEIO_LIVE_TRANSFER_CURRENCY must transfer only between spot and isolated margin")
		require.True(t, config.Request.CurrencyPair.IsPopulated(), "GCT_GATEIO_LIVE_TRANSFER_CURRENCY currency pair must be populated")
		require.True(t, config.Request.Currency.Equal(config.Request.CurrencyPair.Base) || config.Request.Currency.Equal(config.Request.CurrencyPair.Quote), "GCT_GATEIO_LIVE_TRANSFER_CURRENCY currency must belong to the configured pair")
		require.Positive(t, config.Request.Amount, "GCT_GATEIO_LIVE_TRANSFER_CURRENCY amount must be positive")
		type balanceState struct {
			from types.Number
			to   types.Number
		}
		readBalances := func(ctx context.Context) (balanceState, error) {
			spotBalances, err := e.GetSpotAccounts(ctx, config.Request.Currency)
			if err != nil {
				return balanceState{}, err
			}
			var spotBalance types.Number
			spotFound := false
			for i := range spotBalances {
				if spotBalances[i].Currency.Equal(config.Request.Currency) {
					spotBalance = spotBalances[i].Available
					spotFound = true
					break
				}
			}
			if !spotFound {
				return balanceState{}, fmt.Errorf("spot balance for %s was not returned", config.Request.Currency)
			}
			marginAccounts, err := e.GetIsolatedMarginAccountList(ctx, config.Request.CurrencyPair)
			if err != nil {
				return balanceState{}, err
			}
			var marginBalance types.Number
			marginFound := false
			for i := range marginAccounts {
				if !marginAccounts[i].CurrencyPair.Equal(config.Request.CurrencyPair) {
					continue
				}
				if config.Request.Currency.Equal(config.Request.CurrencyPair.Base) {
					marginBalance = marginAccounts[i].Base.Available
				} else {
					marginBalance = marginAccounts[i].Quote.Available
				}
				marginFound = true
				break
			}
			if !marginFound {
				return balanceState{}, fmt.Errorf("isolated margin balance for %s was not returned", config.Request.CurrencyPair)
			}
			if config.Request.From == spotAccount {
				return balanceState{from: spotBalance, to: marginBalance}, nil
			}
			return balanceState{from: marginBalance, to: spotBalance}, nil
		}
		before, err := readBalances(t.Context())
		require.NoError(t, err, "GCT_GATEIO_LIVE_TRANSFER_CURRENCY must snapshot both balances")
		applied := balanceState{from: before.from - config.Request.Amount, to: before.to + config.Request.Amount}
		equal := func(a, b balanceState) bool {
			return math.Abs(a.from.Float64()-b.from.Float64()) <= 1e-10 && math.Abs(a.to.Float64()-b.to.Float64()) <= 1e-10
		}
		require.False(t, equal(before, applied), "GCT_GATEIO_LIVE_TRANSFER_CURRENCY amount must produce a distinguishable balance change")
		restore := config.Request
		restore.From, restore.To = config.Request.To, config.Request.From
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, reconcileGateIOLiveMutation(ctx, readBalances, func(ctx context.Context) error {
				_, err := e.TransferCurrency(ctx, &restore)
				return err
			}, before, applied, equal, gateIOLiveReconciliationPollInterval, gateIOLiveReconciliationPollAttempts), "TransferCurrency live cleanup should reconcile the transferred funds")
		})
		_, err = e.TransferCurrency(t.Context(), &config.Request)
		require.NoError(t, err, "TransferCurrency must not error against the live API")
	})
}

func TestAssetTypeToString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		asset    asset.Item
		expected string
	}{
		{name: "spot", asset: asset.Spot, expected: spotAccount},
		{name: "margin", asset: asset.Margin, expected: marginAccount},
		{name: "cross margin", asset: asset.CrossMargin, expected: crossMarginAccount},
		{name: "options", asset: asset.Options, expected: optionsAccount},
		{name: "fallback", asset: asset.CoinMarginedFutures, expected: asset.CoinMarginedFutures.String()},
		{name: "empty", asset: asset.Empty, expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, e.assetTypeToString(tc.asset), "assetTypeToString should return expected account type")
		})
	}
}

func TestIsSpotOrderAccount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		account  string
		expected bool
	}{
		{name: "spot", account: spotAccount, expected: true},
		{name: "margin", account: marginAccount, expected: true},
		{name: "cross margin", account: crossMarginAccount, expected: true},
		{name: "empty", account: "", expected: false},
		{name: "options", account: optionsAccount, expected: false},
		{name: "futures", account: futuresAccount, expected: false},
		{name: "spot uppercase", account: "SPOT", expected: true},
		{name: "margin mixed case", account: "Margin", expected: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, isSpotOrderAccount(tc.account), "isSpotOrderAccount should return expected support status")
		})
	}
}

func TestSubAccountTransfer(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	req := SubAccountTransferParam{SubAccountType: "index"}
	require.ErrorIs(t, e.SubAccountTransfer(ctx, req), currency.ErrCurrencyCodeEmpty)
	req.Currency = currency.BTC
	require.ErrorIs(t, e.SubAccountTransfer(ctx, req), errInvalidSubAccount)
	req.SubAccount = "1337"
	require.ErrorIs(t, e.SubAccountTransfer(ctx, req), errInvalidTransferDirection)
	req.Direction = "to"
	require.ErrorIs(t, e.SubAccountTransfer(ctx, req), errInvalidAmount)
	req.Amount = 1.337
	require.ErrorIs(t, e.SubAccountTransfer(ctx, req), asset.ErrNotSupported)
	ex, requests := setupGateIOHTTPTest(t, http.MethodPost, "/api/v4/wallet/sub_account_transfers", `{}`)
	req.SubAccountType = "spot"
	require.NoError(t, ex.SubAccountTransfer(ctx, req), "SubAccountTransfer must not error")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, http.MethodPost, gotRequest.method, "request method should be POST")
	assert.Equal(t, "/api/v4/wallet/sub_account_transfers", gotRequest.path, "request path should match")
	assert.JSONEq(t, `{"currency":"BTC","sub_account":"1337","direction":"to","amount":"1.337","sub_account_type":"spot"}`, string(gotRequest.body), "request body should match")

	requireGateIORequestErrors(t, "/api/v4/wallet/sub_account_transfers", false, func(ctx context.Context, ex *Exchange) error {
		return ex.SubAccountTransfer(ctx, req)
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()

		skipGateIOLiveMutationTest(t, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER")
		config, err := decodeGateIOLiveTestJSON[gateIOLiveSubAccountTransfer](gateIOLiveTestValue(t, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER"))
		require.NoError(t, err, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER must contain valid JSON")
		require.True(t, config.DedicatedTestAccount, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER must set dedicated_test_account=true")
		require.Equal(t, spotAccount, config.Request.SubAccountType, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER must use the subaccount spot balance")
		require.False(t, config.Request.Currency.IsEmpty(), "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER currency must be set")
		require.NotEmpty(t, config.Request.SubAccount, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER subaccount must be set")
		require.Positive(t, config.Request.Amount, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER amount must be positive")
		restore := config.Request
		switch strings.ToLower(config.Request.Direction) {
		case "to":
			restore.Direction = "from"
		case "from":
			restore.Direction = "to"
		default:
			require.FailNow(t, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER direction must be to or from")
		}
		type balanceState struct {
			from types.Number
			to   types.Number
		}
		readBalances := func(ctx context.Context) (balanceState, error) {
			spotBalances, err := e.GetSpotAccounts(ctx, config.Request.Currency)
			if err != nil {
				return balanceState{}, err
			}
			var mainBalance types.Number
			mainFound := false
			for i := range spotBalances {
				if spotBalances[i].Currency.Equal(config.Request.Currency) {
					mainBalance = spotBalances[i].Available
					mainFound = true
					break
				}
			}
			if !mainFound {
				return balanceState{}, fmt.Errorf("main spot balance for %s was not returned", config.Request.Currency)
			}
			subBalances, err := e.GetSubAccountBalances(ctx, config.Request.SubAccount)
			if err != nil {
				return balanceState{}, err
			}
			var subBalance types.Number
			subFound := false
			for i := range subBalances {
				if value, ok := subBalances[i].Available[config.Request.Currency.String()]; ok {
					subBalance = value
					subFound = true
					break
				}
			}
			if !subFound {
				return balanceState{}, fmt.Errorf("subaccount spot balance for %s was not returned", config.Request.Currency)
			}
			if strings.EqualFold(config.Request.Direction, "to") {
				return balanceState{from: mainBalance, to: subBalance}, nil
			}
			return balanceState{from: subBalance, to: mainBalance}, nil
		}
		before, err := readBalances(t.Context())
		require.NoError(t, err, "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER must snapshot both balances")
		applied := balanceState{from: before.from - config.Request.Amount, to: before.to + config.Request.Amount}
		equal := func(a, b balanceState) bool {
			return math.Abs(a.from.Float64()-b.from.Float64()) <= 1e-10 && math.Abs(a.to.Float64()-b.to.Float64()) <= 1e-10
		}
		require.False(t, equal(before, applied), "GCT_GATEIO_LIVE_SUBACCOUNT_TRANSFER amount must produce a distinguishable balance change")
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), gateIOLiveReconciliationTimeout)
			defer cancel()
			assert.NoError(t, reconcileGateIOLiveMutation(ctx, readBalances, func(ctx context.Context) error {
				return e.SubAccountTransfer(ctx, restore)
			}, before, applied, equal, gateIOLiveReconciliationPollInterval, gateIOLiveReconciliationPollAttempts), "SubAccountTransfer live cleanup should reconcile the transferred funds")
		})
		err = e.SubAccountTransfer(t.Context(), config.Request)
		require.NoError(t, err, "SubAccountTransfer must not error against the live API")
	})
}

func TestGetSubAccountTransferHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.GetSubAccountTransferHistory(t.Context(), "", time.Time{}, time.Time{}, 0, 0); err != nil {
		t.Errorf("%s GetSubAccountTransferHistory() error %v", e.Name, err)
	}
}

func TestSubAccountTransferToSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if err := e.SubAccountTransferToSubAccount(t.Context(), &InterSubAccountTransferParams{
		Currency:                currency.BTC,
		SubAccountFromUserID:    "1234",
		SubAccountFromAssetType: asset.Spot,
		SubAccountToUserID:      "4567",
		SubAccountToAssetType:   asset.Spot,
		Amount:                  1234,
	}); err != nil {
		t.Error(err)
	}
}

func TestGetWithdrawalStatus(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetWithdrawalStatus(t.Context(), currency.NewCode("")); err != nil {
		t.Errorf("%s GetWithdrawalStatus() error %v", e.Name, err)
	}
}

func TestGetSubAccountBalances(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSubAccountBalances(t.Context(), ""); err != nil {
		t.Errorf("%s GetSubAccountBalances() error %v", e.Name, err)
	}
}

func TestGetSubAccountMarginBalances(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSubAccountMarginBalances(t.Context(), ""); err != nil {
		t.Errorf("%s GetSubAccountMarginBalances() error %v", e.Name, err)
	}
}

func TestGetSubAccountFuturesBalances(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSubAccountFuturesBalances(t.Context(), "", currency.EMPTYCODE)
	assert.NoError(t, err, "GetSubAccountFuturesBalances should not error")
}

func TestGetSubAccountCrossMarginBalances(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSubAccountCrossMarginBalances(t.Context(), ""); err != nil {
		t.Errorf("%s GetSubAccountCrossMarginBalances() error %v", e.Name, err)
	}
}

func TestGetSavedAddresses(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSavedAddresses(t.Context(), currency.BTC, "", 0); err != nil {
		t.Errorf("%s GetSavedAddresses() error %v", e.Name, err)
	}
}

func TestGetPersonalTradingFee(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetPersonalTradingFee(t.Context(), currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter}, currency.EMPTYCODE)
	assert.NoError(t, err, "GetPersonalTradingFee should not error")
}

func TestGetUsersTotalBalance(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetUsersTotalBalance(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s GetUsersTotalBalance() error %v", e.Name, err)
	}
}

func TestGetOrderbookOfLendingLoans(t *testing.T) {
	t.Parallel()
	if _, err := e.GetOrderbookOfLendingLoans(t.Context(), currency.BTC); err != nil {
		t.Errorf("%s GetOrderbookOfLendingLoans() error %v", e.Name, err)
	}
}

func TestGetAllFutureContracts(t *testing.T) {
	t.Parallel()

	for _, c := range []currency.Code{currency.BTC, currency.USDT} {
		_, err := e.GetAllFutureContracts(t.Context(), c)
		assert.NoErrorf(t, err, "GetAllFutureContracts %s should not error", c)
	}
}

func TestGetFuturesContract(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesContract(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures).String())
	assert.NoError(t, err, "GetFuturesContract should not error")
	_, err = e.GetFuturesContract(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures).String())
	assert.NoError(t, err, "GetFuturesContract should not error")
}

func TestGetFuturesOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesOrderbook(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures).String(), "", 10, false)
	assert.NoError(t, err, "GetFuturesOrderbook should not error for CoinMarginedFutures")
	_, err = e.GetFuturesOrderbook(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures).String(), "", 10, false)
	assert.NoError(t, err, "GetFuturesOrderbook should not error for USDTMarginedFutures")
}

func TestGetFuturesTradingHistory(t *testing.T) {
	t.Parallel()

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	_, err := e.GetFuturesTradingHistory(t.Context(), currency.EMPTYCODE, pair, 0, 0, "", time.Time{}, time.Time{})
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetFuturesTradingHistory must reject an empty settlement currency")
	_, err = e.GetFuturesTradingHistory(t.Context(), currency.USDT, currency.EMPTYPAIR, 0, 0, "", time.Time{}, time.Time{})
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetFuturesTradingHistory must reject an empty contract")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetFuturesTradingHistory(t.Context(), currency.USDT, pair, 0, 0, "", from, from.Add(-time.Minute))
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetFuturesTradingHistory must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/futures/usdt/trades", `[{"id":7,"contract":"BTC_USDT","price":"2"}]`)
	trades, err := ex.GetFuturesTradingHistory(t.Context(), currency.USDT, pair, 25, 3, "last", from, to)
	require.NoError(t, err, "GetFuturesTradingHistory must not error")
	require.Len(t, trades, 1, "GetFuturesTradingHistory must decode one trade")
	assert.Equal(t, int64(7), trades[0].ID, "trade ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/futures/usdt/trades", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		"last_id":                  {"last"},
		gateIOTestLimitQueryKey:    {"25"},
		"offset":                   {"3"},
		"to":                       {"1700000060"},
	}, gotRequest.query, "request query should match")

	trades, err = ex.GetFuturesTradingHistory(t.Context(), currency.USDT, pair, 0, 0, "", time.Time{}, time.Time{})
	require.NoError(t, err, "GetFuturesTradingHistory must accept omitted optional parameters")
	require.Len(t, trades, 1, "GetFuturesTradingHistory must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestContractQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/trades", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetFuturesTradingHistory(ctx, currency.USDT, pair, 0, 0, "", time.Time{}, time.Time{})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetFuturesTradingHistory(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), 0, 0, "", time.Time{}, time.Time{})
		require.NoError(t, err, "GetFuturesTradingHistory must not error for coin-margined futures against the live API")
		_, err = e.GetFuturesTradingHistory(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures), 0, 0, "", time.Time{}, time.Time{})
		require.NoError(t, err, "GetFuturesTradingHistory must not error for USDT-margined futures against the live API")
	})
}

func TestGetFuturesCandlesticks(t *testing.T) {
	t.Parallel()

	_, err := e.GetFuturesCandlesticks(t.Context(), currency.EMPTYCODE, gateIOTestBTCUSDT, time.Time{}, time.Time{}, 0, 0)
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetFuturesCandlesticks must reject an empty settlement currency")
	_, err = e.GetFuturesCandlesticks(t.Context(), currency.USDT, "", time.Time{}, time.Time{}, 0, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetFuturesCandlesticks must reject an empty contract")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetFuturesCandlesticks(t.Context(), currency.USDT, gateIOTestBTCUSDT, from, from.Add(-time.Minute), 0, 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetFuturesCandlesticks must reject a reversed time range")
	_, err = e.GetFuturesCandlesticks(t.Context(), currency.USDT, gateIOTestBTCUSDT, time.Time{}, time.Time{}, 0, kline.ThreeDay)
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "GetFuturesCandlesticks must reject an unsupported interval")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/futures/usdt/candlesticks", `[{"t":1738108800,"c":"2","n":"BTC_USDT"}]`)
	candlesticks, err := ex.GetFuturesCandlesticks(t.Context(), currency.USDT, "btc_usdt", from, to, 25, kline.OneDay)
	require.NoError(t, err, "GetFuturesCandlesticks must not error")
	require.Len(t, candlesticks, 1, "GetFuturesCandlesticks must decode one candlestick")
	assert.Equal(t, gateIOTestBTCUSDT, candlesticks[0].Name, "contract should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/futures/usdt/candlesticks", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		gateIOTestIntervalQueryKey: {"1d"},
		gateIOTestLimitQueryKey:    {"25"},
		"to":                       {"1700000060"},
	}, gotRequest.query, "request query should match")

	candlesticks, err = ex.GetFuturesCandlesticks(t.Context(), currency.USDT, "btc_usdt", time.Time{}, time.Time{}, 0, 0)
	require.NoError(t, err, "GetFuturesCandlesticks must accept omitted optional parameters")
	require.Len(t, candlesticks, 1, "GetFuturesCandlesticks must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestContractQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/candlesticks", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetFuturesCandlesticks(ctx, currency.USDT, gateIOTestBTCUSDT, time.Time{}, time.Time{}, 0, kline.OneDay)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetFuturesCandlesticks(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures).String(), time.Time{}, time.Time{}, 0, kline.OneWeek)
		require.NoError(t, err, "GetFuturesCandlesticks must not error for coin-margined futures against the live API")
		_, err = e.GetFuturesCandlesticks(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures).String(), time.Time{}, time.Time{}, 0, kline.OneWeek)
		require.NoError(t, err, "GetFuturesCandlesticks must not error for USDT-margined futures against the live API")
	})
}

func TestPremiumIndexKLine(t *testing.T) {
	t.Parallel()
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(kline.OneWeek.Duration())
	for _, tc := range []struct {
		name     string
		settle   currency.Code
		contract currency.Pair
		path     string
	}{
		{name: gateIOTestCoinMarginedName, settle: currency.BTC, contract: currency.NewPairWithDelimiter("BTC", "USD", currency.UnderscoreDelimiter), path: "/api/v4/futures/btc/premium_index"},
		{name: gateIOTestUSDTMarginedName, settle: currency.USDT, contract: currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter), path: "/api/v4/futures/usdt/premium_index"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex, requests := setupGateIOHTTPTest(t, http.MethodGet, tc.path, `[{"t":1700000000,"c":"2"}]`)
			candles, err := ex.PremiumIndexKLine(t.Context(), tc.settle, tc.contract, from, to, 25, kline.OneWeek)
			require.NoError(t, err, "PremiumIndexKLine must not error")
			require.Len(t, candles, 1, "PremiumIndexKLine must decode one candlestick")
			assert.Equal(t, 2.0, candles[0].ClosePrice.Float64(), "close price should match")
			gotRequest := requireGateIOHTTPRequest(t, requests)
			assert.Equal(t, url.Values{
				gateIOTestContractQueryKey: {tc.contract.String()},
				gateIOTestFromQueryKey:     {"1700000000"},
				gateIOTestIntervalQueryKey: {"7d"},
				gateIOTestLimitQueryKey:    {"25"},
				"to":                       {strconv.FormatInt(to.Unix(), 10)},
			}, gotRequest.query, "request query should match")

			candles, err = ex.PremiumIndexKLine(t.Context(), tc.settle, tc.contract, time.Time{}, time.Time{}, 0, kline.OneWeek)
			require.NoError(t, err, "PremiumIndexKLine must accept omitted optional parameters")
			require.Len(t, candles, 1, "PremiumIndexKLine must decode the response with omitted optional parameters")
			gotRequest = requireGateIOHTTPRequest(t, requests)
			assert.Equal(t, url.Values{
				gateIOTestContractQueryKey: {tc.contract.String()},
				gateIOTestIntervalQueryKey: {"7d"},
			}, gotRequest.query, "zero-value optional parameters should be omitted")
		})
	}

	t.Run("validation", func(t *testing.T) {
		t.Parallel()
		contract := currency.NewBTCUSDT()
		_, err := e.PremiumIndexKLine(t.Context(), currency.Code{}, contract, time.Time{}, time.Time{}, 0, kline.OneWeek)
		assert.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "empty settlement currency should return expected error")
		_, err = e.PremiumIndexKLine(t.Context(), currency.USDT, currency.Pair{}, time.Time{}, time.Time{}, 0, kline.OneWeek)
		assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "empty contract should return expected error")
		_, err = e.PremiumIndexKLine(t.Context(), currency.USDT, contract, time.Time{}, time.Time{}, 0, kline.FiveDay)
		assert.ErrorIs(t, err, kline.ErrUnsupportedInterval, "unsupported interval should return expected error")
	})

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/premium_index", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.PremiumIndexKLine(ctx, currency.USDT, currency.NewBTCUSDT(), time.Time{}, time.Time{}, 0, kline.OneWeek)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.PremiumIndexKLine(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), time.Time{}, time.Time{}, 0, kline.OneWeek)
		require.NoError(t, err, "PremiumIndexKLine must not error for coin-margined futures against the live API")
		_, err = e.PremiumIndexKLine(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures), time.Time{}, time.Time{}, 0, kline.OneWeek)
		require.NoError(t, err, "PremiumIndexKLine must not error for USDT-margined futures against the live API")
	})
}

func TestGetFuturesTickers(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesTickers(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR)
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetFuturesTickers must reject an empty settlement currency")
	for _, tc := range []struct {
		name     string
		settle   currency.Code
		contract currency.Pair
		path     string
	}{
		{name: gateIOTestCoinMarginedName, settle: currency.BTC, contract: currency.NewPairWithDelimiter("BTC", "USD", currency.UnderscoreDelimiter), path: "/api/v4/futures/btc/tickers"},
		{name: gateIOTestUSDTMarginedName, settle: currency.USDT, contract: currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter), path: "/api/v4/futures/usdt/tickers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex, requests := setupGateIOHTTPTest(t, http.MethodGet, tc.path, fmt.Sprintf(`[{"contract":%q,"last":"2"}]`, tc.contract.String()))
			tickers, err := ex.GetFuturesTickers(t.Context(), tc.settle, tc.contract)
			require.NoError(t, err, "GetFuturesTickers must not error")
			require.Len(t, tickers, 1, "GetFuturesTickers must decode one ticker")
			assert.Equal(t, tc.contract.String(), tickers[0].Contract, "contract should match")
			gotRequest := requireGateIOHTTPRequest(t, requests)
			assert.Equal(t, url.Values{gateIOTestContractQueryKey: {tc.contract.String()}}, gotRequest.query, "request query should match")

			tickers, err = ex.GetFuturesTickers(t.Context(), tc.settle, currency.EMPTYPAIR)
			require.NoError(t, err, "GetFuturesTickers must accept an omitted contract")
			require.Len(t, tickers, 1, "GetFuturesTickers must decode the response with an omitted contract")
			gotRequest = requireGateIOHTTPRequest(t, requests)
			assert.Empty(t, gotRequest.query, "empty contract should be omitted")
		})
	}

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/tickers", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetFuturesTickers(ctx, currency.USDT, currency.NewBTCUSDT())
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetFuturesTickers(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures))
		require.NoError(t, err, "GetFuturesTickers must not error for coin-margined futures against the live API")
		_, err = e.GetFuturesTickers(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures))
		require.NoError(t, err, "GetFuturesTickers must not error for USDT-margined futures against the live API")
	})
}

func TestGetFutureFundingRates(t *testing.T) {
	t.Parallel()
	_, err := e.GetFutureFundingRates(t.Context(), currency.EMPTYCODE, currency.NewBTCUSDT(), 0)
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetFutureFundingRates must reject an empty settlement currency")
	_, err = e.GetFutureFundingRates(t.Context(), currency.USDT, currency.EMPTYPAIR, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetFutureFundingRates must reject an empty contract")
	for _, tc := range []struct {
		name     string
		settle   currency.Code
		contract currency.Pair
		path     string
	}{
		{name: gateIOTestCoinMarginedName, settle: currency.BTC, contract: currency.NewPairWithDelimiter("BTC", "USD", currency.UnderscoreDelimiter), path: "/api/v4/futures/btc/funding_rate"},
		{name: gateIOTestUSDTMarginedName, settle: currency.USDT, contract: currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter), path: "/api/v4/futures/usdt/funding_rate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex, requests := setupGateIOHTTPTest(t, http.MethodGet, tc.path, `[{"t":1700000000,"r":"0.1"}]`)
			rates, err := ex.GetFutureFundingRates(t.Context(), tc.settle, tc.contract, 25)
			require.NoError(t, err, "GetFutureFundingRates must not error")
			require.Len(t, rates, 1, "GetFutureFundingRates must decode one rate")
			assert.Equal(t, 0.1, rates[0].Rate.Float64(), "funding rate should match")
			gotRequest := requireGateIOHTTPRequest(t, requests)
			assert.Equal(t, url.Values{
				gateIOTestContractQueryKey: {tc.contract.String()},
				gateIOTestLimitQueryKey:    {"25"},
			}, gotRequest.query, "request query should match")

			rates, err = ex.GetFutureFundingRates(t.Context(), tc.settle, tc.contract, 0)
			require.NoError(t, err, "GetFutureFundingRates must accept an omitted limit")
			require.Len(t, rates, 1, "GetFutureFundingRates must decode the response with an omitted limit")
			gotRequest = requireGateIOHTTPRequest(t, requests)
			assert.Equal(t, url.Values{gateIOTestContractQueryKey: {tc.contract.String()}}, gotRequest.query, "zero-value limit should be omitted")
		})
	}

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/funding_rate", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetFutureFundingRates(ctx, currency.USDT, currency.NewBTCUSDT(), 0)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetFutureFundingRates(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), 0)
		require.NoError(t, err, "GetFutureFundingRates must not error for coin-margined futures against the live API")
		_, err = e.GetFutureFundingRates(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures), 0)
		require.NoError(t, err, "GetFutureFundingRates must not error for USDT-margined futures against the live API")
	})
}

func TestGetFuturesInsuranceBalanceHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesInsuranceBalanceHistory(t.Context(), currency.USDT, 0)
	assert.NoError(t, err, "GetFuturesInsuranceBalanceHistory should not error")
}

func TestGetFutureStats(t *testing.T) {
	t.Parallel()
	_, err := e.GetFutureStats(t.Context(), currency.EMPTYCODE, currency.NewBTCUSDT(), time.Time{}, 0, 0)
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetFutureStats must reject an empty settlement currency")
	_, err = e.GetFutureStats(t.Context(), currency.USDT, currency.EMPTYPAIR, time.Time{}, 0, 0)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty, "GetFutureStats must reject an empty contract")
	_, err = e.GetFutureStats(t.Context(), currency.USDT, currency.NewBTCUSDT(), time.Time{}, kline.FiveDay, 0)
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "GetFutureStats must reject an unsupported interval")
	from := time.Unix(1_700_000_000, 0)
	for _, tc := range []struct {
		name     string
		settle   currency.Code
		contract currency.Pair
		path     string
	}{
		{name: gateIOTestCoinMarginedName, settle: currency.BTC, contract: currency.NewPairWithDelimiter("BTC", "USD", currency.UnderscoreDelimiter), path: "/api/v4/futures/btc/contract_stats"},
		{name: gateIOTestUSDTMarginedName, settle: currency.USDT, contract: currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter), path: "/api/v4/futures/usdt/contract_stats"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex, requests := setupGateIOHTTPTest(t, http.MethodGet, tc.path, `[{"time":1700000000,"open_interest":"3"}]`)
			stats, err := ex.GetFutureStats(t.Context(), tc.settle, tc.contract, from, kline.OneDay, 25)
			require.NoError(t, err, "GetFutureStats must not error")
			require.Len(t, stats, 1, "GetFutureStats must decode one statistic")
			assert.Equal(t, 3.0, stats[0].OpenInterest.Float64(), "open interest should match")
			gotRequest := requireGateIOHTTPRequest(t, requests)
			assert.Equal(t, url.Values{
				gateIOTestContractQueryKey: {tc.contract.String()},
				gateIOTestFromQueryKey:     {"1700000000"},
				gateIOTestIntervalQueryKey: {"1d"},
				gateIOTestLimitQueryKey:    {"25"},
			}, gotRequest.query, "request query should match")

			stats, err = ex.GetFutureStats(t.Context(), tc.settle, tc.contract, time.Time{}, 0, 0)
			require.NoError(t, err, "GetFutureStats must accept omitted optional parameters")
			require.Len(t, stats, 1, "GetFutureStats must decode the response with omitted optional parameters")
			gotRequest = requireGateIOHTTPRequest(t, requests)
			assert.Equal(t, url.Values{gateIOTestContractQueryKey: {tc.contract.String()}}, gotRequest.query, "zero-value optional parameters should be omitted")
		})
	}

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/contract_stats", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetFutureStats(ctx, currency.USDT, currency.NewBTCUSDT(), time.Time{}, 0, 0)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetFutureStats(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), time.Time{}, 0, 0)
		require.NoError(t, err, "GetFutureStats must not error for coin-margined futures against the live API")
		_, err = e.GetFutureStats(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures), time.Time{}, 0, 0)
		require.NoError(t, err, "GetFutureStats must not error for USDT-margined futures against the live API")
	})
}

func TestGetIndexConstituent(t *testing.T) {
	t.Parallel()
	_, err := e.GetIndexConstituent(t.Context(), currency.USDT, currency.Pair{Base: currency.BTC, Quote: currency.USDT, Delimiter: currency.UnderscoreDelimiter}.String())
	assert.NoError(t, err, "GetIndexConstituent should not error")
}

func TestGetLiquidationHistory(t *testing.T) {
	t.Parallel()

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	_, err := e.GetLiquidationHistory(t.Context(), currency.EMPTYCODE, pair, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetLiquidationHistory must reject an empty settlement currency")
	_, err = e.GetLiquidationHistory(t.Context(), currency.USDT, currency.EMPTYPAIR, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, errInvalidOrMissingContractParam, "GetLiquidationHistory must reject an empty contract")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetLiquidationHistory(t.Context(), currency.USDT, pair, from, from.Add(-time.Minute), 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetLiquidationHistory must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/futures/usdt/liq_orders", `[{"contract":"BTC_USDT","order_id":7}]`)
	liquidations, err := ex.GetLiquidationHistory(t.Context(), currency.USDT, pair, from, to, 25)
	require.NoError(t, err, "GetLiquidationHistory must not error")
	require.Len(t, liquidations, 1, "GetLiquidationHistory must decode one liquidation")
	assert.Equal(t, int64(7), liquidations[0].OrderID, "order ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/futures/usdt/liq_orders", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		gateIOTestLimitQueryKey:    {"25"},
		"to":                       {"1700000060"},
	}, gotRequest.query, "request query should match")

	liquidations, err = ex.GetLiquidationHistory(t.Context(), currency.USDT, pair, time.Time{}, time.Time{}, 0)
	require.NoError(t, err, "GetLiquidationHistory must accept omitted optional parameters")
	require.Len(t, liquidations, 1, "GetLiquidationHistory must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestContractQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/liq_orders", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetLiquidationHistory(ctx, currency.USDT, pair, time.Time{}, time.Time{}, 0)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetLiquidationHistory(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), time.Time{}, time.Time{}, 0)
		require.NoError(t, err, "GetLiquidationHistory must not error for coin-margined futures against the live API")
		_, err = e.GetLiquidationHistory(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures), time.Time{}, time.Time{}, 0)
		require.NoError(t, err, "GetLiquidationHistory must not error for USDT-margined futures against the live API")
	})
}

func TestQueryFuturesAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.QueryFuturesAccount(t.Context(), currency.USDT)
	assert.NoError(t, err, "QueryFuturesAccount should not error")
}

func TestGetFuturesAccountBooks(t *testing.T) {
	t.Parallel()

	_, err := e.GetFuturesAccountBooks(t.Context(), currency.EMPTYCODE, 0, time.Time{}, time.Time{}, "")
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetFuturesAccountBooks must reject an empty settlement currency")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetFuturesAccountBooks(t.Context(), currency.USDT, 0, from, from.Add(-time.Minute), "")
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetFuturesAccountBooks must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/futures/usdt/account_book", `[{"text":"fixture","balance":"2"}]`)
	records, err := ex.GetFuturesAccountBooks(t.Context(), currency.USDT, 25, from, to, "dnw")
	require.NoError(t, err, "GetFuturesAccountBooks must not error")
	require.Len(t, records, 1, "GetFuturesAccountBooks must decode one record")
	assert.Equal(t, "fixture", records[0].Text, "record text should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/futures/usdt/account_book", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestFromQueryKey:  {"1700000000"},
		gateIOTestLimitQueryKey: {"25"},
		"to":                    {"1700000060"},
		gateIOTestTypeQueryKey:  {"dnw"},
	}, gotRequest.query, "request query should match")

	records, err = ex.GetFuturesAccountBooks(t.Context(), currency.USDT, 0, time.Time{}, time.Time{}, "")
	require.NoError(t, err, "GetFuturesAccountBooks must accept omitted optional parameters")
	require.Len(t, records, 1, "GetFuturesAccountBooks must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/account_book", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetFuturesAccountBooks(ctx, currency.USDT, 0, time.Time{}, time.Time{}, "")
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetFuturesAccountBooks(t.Context(), currency.USDT, 0, time.Time{}, time.Time{}, "dnw")
		require.NoError(t, err, "GetFuturesAccountBooks must not error against the live API")
	})
}

func TestGetAllFuturesPositionsOfUsers(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAllFuturesPositionsOfUsers(t.Context(), currency.USDT, true)
	assert.NoError(t, err, "GetAllPositionsOfUsers should not error")
}

func TestGetSinglePosition(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSinglePosition(t.Context(), currency.USDT, currency.Pair{Quote: currency.BTC, Base: currency.USDT})
	assert.NoError(t, err, "GetSinglePosition should not error")
}

func TestUpdateFuturesPositionMargin(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UpdateFuturesPositionMargin(t.Context(), currency.BTC, 0.01, getPair(t, asset.CoinMarginedFutures))
	assert.NoError(t, err, "UpdateFuturesPositionMargin should not error for CoinMarginedFutures")
	_, err = e.UpdateFuturesPositionMargin(t.Context(), currency.USDT, 0.01, getPair(t, asset.USDTMarginedFutures))
	assert.NoError(t, err, "UpdateFuturesPositionMargin should not error for USDTMarginedFutures")
}

func TestUpdateFuturesPositionLeverage(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UpdateFuturesPositionLeverage(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), 1, 0)
	assert.NoError(t, err, "UpdateFuturesPositionLeverage should not error for CoinMarginedFutures")
	_, err = e.UpdateFuturesPositionLeverage(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures), 1, 0)
	assert.NoError(t, err, "UpdateFuturesPositionLeverage should not error for USDTMarginedFutures")
}

func TestSetLeverage(t *testing.T) {
	t.Parallel()

	err := e.SetLeverage(t.Context(), asset.Spot, getPair(t, asset.Spot), margin.Isolated, 1, order.UnknownSide)
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	err = e.SetLeverage(t.Context(), asset.CoinMarginedFutures, getPair(t, asset.CoinMarginedFutures), margin.NoMargin, 1, order.UnknownSide)
	assert.ErrorIs(t, err, margin.ErrMarginTypeUnsupported)

	err = e.SetLeverage(t.Context(), asset.CoinMarginedFutures, getPair(t, asset.CoinMarginedFutures), margin.Isolated, 0, order.UnknownSide)
	assert.ErrorIs(t, err, errInvalidLeverage)

	err = e.SetLeverage(t.Context(), asset.CoinMarginedFutures, currency.EMPTYPAIR, margin.Isolated, 1, order.UnknownSide)
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	err = e.SetLeverage(t.Context(), asset.DeliveryFutures, getPair(t, asset.DeliveryFutures), margin.Isolated, 0, order.UnknownSide)
	assert.ErrorIs(t, err, errInvalidLeverage)

	err = e.SetLeverage(t.Context(), asset.DeliveryFutures, getPair(t, asset.DeliveryFutures), margin.Multi, 1, order.UnknownSide)
	assert.ErrorIs(t, err, margin.ErrMarginTypeUnsupported)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	err = e.SetLeverage(t.Context(), asset.CoinMarginedFutures, getPair(t, asset.CoinMarginedFutures), margin.Isolated, 1, order.UnknownSide)
	assert.NoError(t, err)

	err = e.SetLeverage(t.Context(), asset.USDTMarginedFutures, getPair(t, asset.USDTMarginedFutures), margin.Multi, 5, order.UnknownSide)
	assert.NoError(t, err)

	err = e.SetLeverage(t.Context(), asset.DeliveryFutures, getPair(t, asset.DeliveryFutures), margin.Isolated, 1, order.UnknownSide)
	assert.NoError(t, err)
}

func TestPlaceDeliveryOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.PlaceDeliveryOrder(t.Context(), &DeliveryOrderCreateParams{
		Contract:    getPair(t, asset.DeliveryFutures),
		Size:        6024,
		Iceberg:     0,
		Price:       3765,
		Text:        "t-my-custom-id",
		Settle:      currency.USDT,
		TimeInForce: gtcTIF,
	})
	assert.NoError(t, err, "CreateDeliveryOrder should not error")
}

func TestGetDeliveryOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetDeliveryOrders(t.Context(), getPair(t, asset.DeliveryFutures), statusOpen, currency.USDT, "", 0, 0, true)
	assert.NoError(t, err, "GetDeliveryOrders should not error")
}

func TestCancelMultipleDeliveryOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelMultipleDeliveryOrders(t.Context(), getPair(t, asset.DeliveryFutures), "ask", currency.USDT)
	assert.NoError(t, err, "CancelMultipleDeliveryOrders should not error")
}

func TestGetSingleDeliveryOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetSingleDeliveryOrder(t.Context(), currency.EMPTYCODE, "123456")
	assert.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetSingleDeliveryOrder should return errEmptyOrInvalidSettlementCurrency")
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err = e.GetSingleDeliveryOrder(t.Context(), currency.USDT, "123456")
	assert.NoError(t, err, "GetSingleDeliveryOrder should not error")
}

func TestCancelSingleDeliveryOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelSingleDeliveryOrder(t.Context(), currency.USDT, "123456")
	assert.NoError(t, err, "CancelSingleDeliveryOrder should not error")
}

func TestGetMyDeliveryTradingHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMyDeliveryTradingHistory(t.Context(), currency.USDT, "", getPair(t, asset.DeliveryFutures), 0, 0, 1, "")
	assert.NoError(t, err, "GetMyDeliveryTradingHistory should not error")
}

func TestGetDeliveryPositionCloseHistory(t *testing.T) {
	t.Parallel()

	_, err := e.GetDeliveryPositionCloseHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetDeliveryPositionCloseHistory must reject an empty settlement currency")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetDeliveryPositionCloseHistory(t.Context(), currency.USDT, currency.EMPTYPAIR, 0, 0, from, from.Add(-time.Minute))
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetDeliveryPositionCloseHistory must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/delivery/usdt/position_close", `[{"contract":"BTC_USDT","text":"fixture"}]`)
	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	positions, err := ex.GetDeliveryPositionCloseHistory(t.Context(), currency.USDT, pair, 25, 3, from, to)
	require.NoError(t, err, "GetDeliveryPositionCloseHistory must not error")
	require.Len(t, positions, 1, "GetDeliveryPositionCloseHistory must decode one position")
	assert.Equal(t, gateIOTestBTCUSDT, positions[0].Contract, "position contract should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/delivery/usdt/position_close", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		gateIOTestLimitQueryKey:    {"25"},
		"offset":                   {"3"},
		"to":                       {"1700000060"},
	}, gotRequest.query, "request query should match")

	positions, err = ex.GetDeliveryPositionCloseHistory(t.Context(), currency.USDT, currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
	require.NoError(t, err, "GetDeliveryPositionCloseHistory must accept omitted optional parameters")
	require.Len(t, positions, 1, "GetDeliveryPositionCloseHistory must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/delivery/usdt/position_close", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetDeliveryPositionCloseHistory(ctx, currency.USDT, pair, 0, 0, time.Time{}, time.Time{})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetDeliveryPositionCloseHistory(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures), 0, 0, time.Time{}, time.Time{})
		require.NoError(t, err, "GetDeliveryPositionCloseHistory must not error against the live API")
	})
}

func TestGetDeliveryLiquidationHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetDeliveryLiquidationHistory(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures), 0, time.Now())
	assert.NoError(t, err, "GetDeliveryLiquidationHistory should not error")
}

func TestGetDeliverySettlementHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetDeliverySettlementHistory(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures), 0, time.Now())
	assert.NoError(t, err, "GetDeliverySettlementHistory should not error")
}

func TestGetDeliveryPriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetDeliveryPriceTriggeredOrder(t.Context(), currency.USDT, &FuturesPriceTriggeredOrderParam{
		Initial: FuturesInitial{
			Price:    1234.,
			Size:     12,
			Contract: getPair(t, asset.DeliveryFutures),
		},
		Trigger: FuturesTrigger{
			Rule:      1,
			OrderType: "close-short-position",
			Price:     123400,
		},
	})
	assert.NoError(t, err, "GetDeliveryPriceTriggeredOrder should not error")
}

func TestGetDeliveryAllAutoOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetDeliveryAllAutoOrder(t.Context(), statusOpen, currency.USDT, getPair(t, asset.DeliveryFutures), 0, 1)
	assert.NoError(t, err, "GetDeliveryAllAutoOrder should not error")
}

func TestCancelAllDeliveryPriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelAllDeliveryPriceTriggeredOrder(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures))
	assert.NoError(t, err, "CancelAllDeliveryPriceTriggeredOrder should not error")
}

func TestGetSingleDeliveryPriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSingleDeliveryPriceTriggeredOrder(t.Context(), currency.USDT, "12345")
	assert.NoError(t, err, "GetSingleDeliveryPriceTriggeredOrder should not error")
}

func TestCancelDeliveryPriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelDeliveryPriceTriggeredOrder(t.Context(), currency.USDT, "12345")
	assert.NoError(t, err, "CancelDeliveryPriceTriggeredOrder should not error")
}

func TestEnableOrDisableDualMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.EnableOrDisableDualMode(t.Context(), currency.BTC, true)
	assert.NoError(t, err, "EnableOrDisableDualMode should not error")
}

func TestRetrievePositionDetailInDualMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.RetrievePositionDetailInDualMode(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures))
	assert.NoError(t, err, "RetrievePositionDetailInDualMode should not error for CoinMarginedFutures")
	_, err = e.RetrievePositionDetailInDualMode(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures))
	assert.NoError(t, err, "RetrievePositionDetailInDualMode should not error for USDTMarginedFutures")
}

func TestUpdatePositionMarginInDualMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UpdatePositionMarginInDualMode(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), 0.001, "dual_long")
	assert.NoError(t, err, "UpdatePositionMarginInDualMode should not error for CoinMarginedFutures")
	_, err = e.UpdatePositionMarginInDualMode(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures), 0.001, "dual_long")
	assert.NoError(t, err, "UpdatePositionMarginInDualMode should not error for USDTMarginedFutures")
}

func TestUpdatePositionLeverageInDualMode(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.UpdatePositionLeverageInDualMode(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), 0.001, 0.001)
	assert.NoError(t, err, "UpdatePositionLeverageInDualMode should not error for CoinMarginedFutures")
	_, err = e.UpdatePositionLeverageInDualMode(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures), 0.001, 0.001)
	assert.NoError(t, err, "UpdatePositionLeverageInDualMode should not error for USDTMarginedFutures")
}

func TestPlaceFuturesOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.PlaceFuturesOrder(t.Context(), &FuturesOrderCreateParams{
		Contract:    getPair(t, asset.CoinMarginedFutures),
		Size:        6024,
		Iceberg:     0,
		Price:       3765,
		TimeInForce: "gtc",
		Text:        "t-my-custom-id",
		Settle:      currency.BTC,
	})
	assert.NoError(t, err, "PlaceFuturesOrder should not error")
}

func TestGetFuturesOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFuturesOrders(t.Context(), currency.NewBTCUSD(), statusOpen, "", currency.BTC, 0, 0, true)
	assert.NoError(t, err, "GetFuturesOrders should not error")
}

func TestCancelMultipleFuturesOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelMultipleFuturesOpenOrders(t.Context(), getPair(t, asset.USDTMarginedFutures), "ask", currency.USDT)
	assert.NoError(t, err, "CancelMultipleFuturesOpenOrders should not error")
}

func TestGetSingleFuturesPriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSingleFuturesPriceTriggeredOrder(t.Context(), currency.BTC, "12345")
	assert.NoError(t, err, "GetSingleFuturesPriceTriggeredOrder should not error")
}

func TestCancelFuturesPriceTriggeredOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelFuturesPriceTriggeredOrder(t.Context(), currency.USDT, "12345")
	assert.NoError(t, err, "CancelFuturesPriceTriggeredOrder should not error")
}

func TestPlaceBatchFuturesOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.PlaceBatchFuturesOrders(t.Context(), currency.BTC, []FuturesOrderCreateParams{
		{
			Contract:    getPair(t, asset.CoinMarginedFutures),
			Size:        6024,
			Iceberg:     0,
			Price:       3765,
			TimeInForce: "gtc",
			Text:        "t-my-custom-id",
			Settle:      currency.BTC,
		},
		{
			Contract:    getPair(t, asset.CoinMarginedFutures),
			Size:        232,
			Iceberg:     0,
			Price:       376225,
			TimeInForce: "gtc",
			Text:        "t-my-custom-id",
			Settle:      currency.BTC,
		},
	})
	assert.NoError(t, err, "PlaceBatchFuturesOrders should not error")
}

func TestGetSingleFuturesOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSingleFuturesOrder(t.Context(), currency.BTC, "12345")
	assert.NoError(t, err, "GetSingleFuturesOrder should not error")
}

func TestCancelSingleFuturesOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelSingleFuturesOrder(t.Context(), currency.BTC, "12345")
	assert.NoError(t, err, "CancelSingleFuturesOrder should not error")
}

func TestAmendFuturesOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.AmendFuturesOrder(t.Context(), currency.BTC, "1234", AmendFuturesOrderParam{
		Price: 12345.990,
	})
	assert.NoError(t, err, "AmendFuturesOrder should not error")
}

func TestGetMyFuturesTradingHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMyFuturesTradingHistory(t.Context(), currency.BTC, "", "", getPair(t, asset.CoinMarginedFutures), 0, 0, 0)
	assert.NoError(t, err, "GetMyFuturesTradingHistory should not error")
}

func TestGetFuturesPositionCloseHistory(t *testing.T) {
	t.Parallel()

	_, err := e.GetFuturesPositionCloseHistory(t.Context(), currency.EMPTYCODE, currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetFuturesPositionCloseHistory must reject an empty settlement currency")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetFuturesPositionCloseHistory(t.Context(), currency.USDT, currency.EMPTYPAIR, 0, 0, from, from.Add(-time.Minute))
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetFuturesPositionCloseHistory must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/futures/usdt/position_close", `[{"contract":"BTC_USDT","text":"fixture"}]`)
	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	positions, err := ex.GetFuturesPositionCloseHistory(t.Context(), currency.USDT, pair, 25, 3, from, to)
	require.NoError(t, err, "GetFuturesPositionCloseHistory must not error")
	require.Len(t, positions, 1, "GetFuturesPositionCloseHistory must decode one position")
	assert.Equal(t, gateIOTestBTCUSDT, positions[0].Contract, "position contract should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/futures/usdt/position_close", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		gateIOTestLimitQueryKey:    {"25"},
		"offset":                   {"3"},
		"to":                       {"1700000060"},
	}, gotRequest.query, "request query should match")

	positions, err = ex.GetFuturesPositionCloseHistory(t.Context(), currency.USDT, currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
	require.NoError(t, err, "GetFuturesPositionCloseHistory must accept omitted optional parameters")
	require.Len(t, positions, 1, "GetFuturesPositionCloseHistory must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/futures/usdt/position_close", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetFuturesPositionCloseHistory(ctx, currency.USDT, pair, 0, 0, time.Time{}, time.Time{})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetFuturesPositionCloseHistory(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), 0, 0, time.Time{}, time.Time{})
		require.NoError(t, err, "GetFuturesPositionCloseHistory must not error against the live API")
	})
}

func TestGetFuturesLiquidationHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetFuturesLiquidationHistory(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures), 0, time.Time{})
	assert.NoError(t, err, "GetFuturesLiquidationHistory should not error")
}

func TestCountdownCancelFuturesOrders(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		requiresAuth bool
		settle       currency.Code
		arg          CountdownParams
		expectedErr  error
	}{
		{
			name:         "valid",
			requiresAuth: true,
			settle:       currency.BTC,
			arg: CountdownParams{
				Timeout: 8,
			},
		},
		{
			name:   "empty_settlement",
			settle: currency.EMPTYCODE,
			arg: CountdownParams{
				Timeout: 8,
			},
			expectedErr: errEmptyOrInvalidSettlementCurrency,
		},
		{
			name:   "negative_timeout",
			settle: currency.BTC,
			arg: CountdownParams{
				Timeout: -1,
			},
			expectedErr: errInvalidTimeout,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.requiresAuth {
				sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
			}
			_, err := e.CountdownCancelFuturesOrders(t.Context(), tc.settle, tc.arg)
			if tc.expectedErr != nil {
				assert.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCreatePriceTriggeredFuturesOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	for _, tc := range []struct {
		c currency.Code
		a asset.Item
	}{
		{currency.BTC, asset.CoinMarginedFutures},
		{currency.USDT, asset.USDTMarginedFutures},
	} {
		_, err := e.CreatePriceTriggeredFuturesOrder(t.Context(), tc.c, &FuturesPriceTriggeredOrderParam{
			Initial: FuturesInitial{
				Price:    1234.,
				Size:     2,
				Contract: getPair(t, tc.a),
			},
			Trigger: FuturesTrigger{
				Rule:      1,
				OrderType: "close-short-position",
			},
		})
		assert.NoErrorf(t, err, "CreatePriceTriggeredFuturesOrder should not error for settlement currency: %s, asset: %s", tc.c, tc.a)
	}
}

func TestListAllFuturesAutoOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.ListAllFuturesAutoOrders(t.Context(), statusOpen, currency.BTC, currency.EMPTYPAIR, 0, 0)
	assert.NoError(t, err, "ListAllFuturesAutoOrders should not error")
}

func TestCancelAllFuturesOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.CancelAllFuturesOpenOrders(t.Context(), currency.BTC, getPair(t, asset.CoinMarginedFutures))
	assert.NoError(t, err, "CancelAllFuturesOpenOrders should not error for CoinMarginedFutures")
	_, err = e.CancelAllFuturesOpenOrders(t.Context(), currency.USDT, getPair(t, asset.USDTMarginedFutures))
	assert.NoError(t, err, "CancelAllFuturesOpenOrders should not error for USDTMarginedFutures")
}

func TestGetAllDeliveryContracts(t *testing.T) {
	t.Parallel()
	r, err := e.GetAllDeliveryContracts(t.Context(), currency.USDT)
	require.NoError(t, err, "GetAllDeliveryContracts must not error")
	assert.NotEmpty(t, r, "GetAllDeliveryContracts should return data")
	r, err = e.GetAllDeliveryContracts(t.Context(), currency.BTC)
	require.NoError(t, err, "GetAllDeliveryContracts must not error")
	// The test below will fail if support for BTC settlement is added. This is intentional, as it ensures we are alerted when it's time to reintroduce support
	if !assert.Empty(t, r, "GetAllDeliveryContracts should not return any data with unsupported settlement currency BTC") {
		t.Error("BTC settlement for delivery futures appears to be supported again by the API. Please raise an issue to reintroduce BTC support for this exchange")
	}
}

func TestGetDeliveryContract(t *testing.T) {
	t.Parallel()
	_, err := e.GetDeliveryContract(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures))
	assert.NoError(t, err, "GetDeliveryContract should not error")
}

func TestGetDeliveryOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetDeliveryOrderbook(t.Context(), currency.USDT, "0", getPair(t, asset.DeliveryFutures), 0, false)
	assert.NoError(t, err, "GetDeliveryOrderbook should not error")
}

func TestGetDeliveryTradingHistory(t *testing.T) {
	t.Parallel()

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	_, err := e.GetDeliveryTradingHistory(t.Context(), currency.EMPTYCODE, "", pair, 0, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetDeliveryTradingHistory must reject an empty settlement currency")
	_, err = e.GetDeliveryTradingHistory(t.Context(), currency.USDT, "", currency.EMPTYPAIR, 0, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errInvalidOrMissingContractParam, "GetDeliveryTradingHistory must reject an empty contract")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetDeliveryTradingHistory(t.Context(), currency.USDT, "", pair, 0, from, from.Add(-time.Minute))
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetDeliveryTradingHistory must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/delivery/usdt/trades", `[{"id":7,"contract":"BTC_USDT","price":"2"}]`)
	trades, err := ex.GetDeliveryTradingHistory(t.Context(), currency.USDT, "last", pair, 25, from, to)
	require.NoError(t, err, "GetDeliveryTradingHistory must not error")
	require.Len(t, trades, 1, "GetDeliveryTradingHistory must decode one trade")
	assert.Equal(t, int64(7), trades[0].ID, "trade ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/delivery/usdt/trades", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		"last_id":                  {"last"},
		gateIOTestLimitQueryKey:    {"25"},
		"to":                       {"1700000060"},
	}, gotRequest.query, "request query should match")

	trades, err = ex.GetDeliveryTradingHistory(t.Context(), currency.USDT, "", pair, 0, time.Time{}, time.Time{})
	require.NoError(t, err, "GetDeliveryTradingHistory must accept omitted optional parameters")
	require.Len(t, trades, 1, "GetDeliveryTradingHistory must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestContractQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/delivery/usdt/trades", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetDeliveryTradingHistory(ctx, currency.USDT, "", pair, 0, time.Time{}, time.Time{})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetDeliveryTradingHistory(t.Context(), currency.USDT, "", getPair(t, asset.DeliveryFutures), 0, time.Time{}, time.Time{})
		require.NoError(t, err, "GetDeliveryTradingHistory must not error against the live API")
	})
}

func TestGetDeliveryFuturesCandlesticks(t *testing.T) {
	t.Parallel()

	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	_, err := e.GetDeliveryFuturesCandlesticks(t.Context(), currency.EMPTYCODE, pair, time.Time{}, time.Time{}, 0, 0)
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetDeliveryFuturesCandlesticks must reject an empty settlement currency")
	_, err = e.GetDeliveryFuturesCandlesticks(t.Context(), currency.USDT, currency.EMPTYPAIR, time.Time{}, time.Time{}, 0, 0)
	require.ErrorIs(t, err, errInvalidOrMissingContractParam, "GetDeliveryFuturesCandlesticks must reject an empty contract")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetDeliveryFuturesCandlesticks(t.Context(), currency.USDT, pair, from, from.Add(-time.Minute), 0, 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetDeliveryFuturesCandlesticks must reject a reversed time range")
	_, err = e.GetDeliveryFuturesCandlesticks(t.Context(), currency.USDT, pair, time.Time{}, time.Time{}, 0, kline.ThreeDay)
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "GetDeliveryFuturesCandlesticks must reject an unsupported interval")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/delivery/usdt/candlesticks", `[{"t":1738108800,"c":"2","n":"BTC_USDT"}]`)
	candlesticks, err := ex.GetDeliveryFuturesCandlesticks(t.Context(), currency.USDT, pair, from, to, 25, kline.OneDay)
	require.NoError(t, err, "GetDeliveryFuturesCandlesticks must not error")
	require.Len(t, candlesticks, 1, "GetDeliveryFuturesCandlesticks must decode one candlestick")
	assert.Equal(t, gateIOTestBTCUSDT, candlesticks[0].Name, "contract should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/delivery/usdt/candlesticks", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		gateIOTestIntervalQueryKey: {"1d"},
		gateIOTestLimitQueryKey:    {"25"},
		"to":                       {"1700000060"},
	}, gotRequest.query, "request query should match")

	candlesticks, err = ex.GetDeliveryFuturesCandlesticks(t.Context(), currency.USDT, pair, time.Time{}, time.Time{}, 0, 0)
	require.NoError(t, err, "GetDeliveryFuturesCandlesticks must accept omitted optional parameters")
	require.Len(t, candlesticks, 1, "GetDeliveryFuturesCandlesticks must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestContractQueryKey: {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/delivery/usdt/candlesticks", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetDeliveryFuturesCandlesticks(ctx, currency.USDT, pair, time.Time{}, time.Time{}, 0, kline.OneDay)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetDeliveryFuturesCandlesticks(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures), time.Time{}, time.Time{}, 0, kline.OneWeek)
		require.NoError(t, err, "GetDeliveryFuturesCandlesticks must not error against the live API")
	})
}

func TestGetDeliveryFutureTickers(t *testing.T) {
	t.Parallel()
	_, err := e.GetDeliveryFutureTickers(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures))
	assert.NoError(t, err, "GetDeliveryFutureTickers should not error")
}

func TestGetDeliveryInsuranceBalanceHistory(t *testing.T) {
	t.Parallel()
	_, err := e.GetDeliveryInsuranceBalanceHistory(t.Context(), currency.BTC, 0)
	assert.NoError(t, err, "GetDeliveryInsuranceBalanceHistory should not error")
}

func TestQueryDeliveryFuturesAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetDeliveryFuturesAccounts(t.Context(), currency.USDT)
	assert.NoError(t, err, "GetDeliveryFuturesAccounts should not error")
}

func TestGetDeliveryAccountBooks(t *testing.T) {
	t.Parallel()

	_, err := e.GetDeliveryAccountBooks(t.Context(), currency.EMPTYCODE, 0, time.Time{}, time.Time{}, "")
	require.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency, "GetDeliveryAccountBooks must reject an empty settlement currency")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetDeliveryAccountBooks(t.Context(), currency.USDT, 0, from, from.Add(-time.Minute), "")
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetDeliveryAccountBooks must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/delivery/usdt/account_book", `[{"text":"fixture","balance":"2"}]`)
	records, err := ex.GetDeliveryAccountBooks(t.Context(), currency.USDT, 25, from, to, "dnw")
	require.NoError(t, err, "GetDeliveryAccountBooks must not error")
	require.Len(t, records, 1, "GetDeliveryAccountBooks must decode one record")
	assert.Equal(t, "fixture", records[0].Text, "record text should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/delivery/usdt/account_book", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestFromQueryKey:  {"1700000000"},
		gateIOTestLimitQueryKey: {"25"},
		"to":                    {"1700000060"},
		gateIOTestTypeQueryKey:  {"dnw"},
	}, gotRequest.query, "request query should match")

	records, err = ex.GetDeliveryAccountBooks(t.Context(), currency.USDT, 0, time.Time{}, time.Time{}, "")
	require.NoError(t, err, "GetDeliveryAccountBooks must accept omitted optional parameters")
	require.Len(t, records, 1, "GetDeliveryAccountBooks must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/delivery/usdt/account_book", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetDeliveryAccountBooks(ctx, currency.USDT, 0, time.Time{}, time.Time{}, "")
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, true)
		_, err := e.GetDeliveryAccountBooks(t.Context(), currency.USDT, 0, time.Time{}, time.Now(), "dnw")
		require.NoError(t, err, "GetDeliveryAccountBooks must not error against the live API")
	})
}

func TestGetAllDeliveryPositionsOfUser(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAllDeliveryPositionsOfUser(t.Context(), currency.USDT)
	assert.NoError(t, err, "GetAllDeliveryPositionsOfUser should not error")
}

func TestGetSingleDeliveryPosition(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetSingleDeliveryPosition(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures))
	assert.NoError(t, err, "GetSingleDeliveryPosition should not error")
}

func TestUpdateDeliveryPositionMargin(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateDeliveryPositionMargin(t.Context(), currency.EMPTYCODE, 0.001, currency.Pair{})
	assert.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.UpdateDeliveryPositionMargin(t.Context(), currency.USDT, 0.001, getPair(t, asset.DeliveryFutures))
	assert.NoError(t, err, "UpdateDeliveryPositionMargin should not error")
}

func TestUpdateDeliveryPositionLeverage(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateDeliveryPositionLeverage(t.Context(), currency.EMPTYCODE, currency.Pair{}, 0.001)
	assert.ErrorIs(t, err, errEmptyOrInvalidSettlementCurrency)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.UpdateDeliveryPositionLeverage(t.Context(), currency.USDT, getPair(t, asset.DeliveryFutures), 0.001)
	assert.NoError(t, err, "UpdateDeliveryPositionLeverage should not error")
}

func TestGetAllOptionsUnderlyings(t *testing.T) {
	t.Parallel()
	if _, err := e.GetAllOptionsUnderlyings(t.Context()); err != nil {
		t.Errorf("%s GetAllOptionsUnderlyings() error %v", e.Name, err)
	}
}

func TestGetExpirationTime(t *testing.T) {
	t.Parallel()
	_, err := e.GetExpirationTime(t.Context(), "")
	assert.ErrorIs(t, err, errInvalidUnderlying)
	_, err = e.GetExpirationTime(t.Context(), gateIOTestBTCUSDT)
	assert.NoError(t, err, "GetExpirationTime should not error")
}

func TestGetAllContractOfUnderlyingWithinExpiryDate(t *testing.T) {
	t.Parallel()
	if _, err := e.GetAllContractOfUnderlyingWithinExpiryDate(t.Context(), gateIOTestBTCUSDT, time.Time{}); err != nil {
		t.Errorf("%s GetAllContractOfUnderlyingWithinExpiryDate() error %v", e.Name, err)
	}
}

func TestGetOptionsSpecifiedContractDetail(t *testing.T) {
	t.Parallel()
	if _, err := e.GetOptionsSpecifiedContractDetail(t.Context(), getPair(t, asset.Options)); err != nil {
		t.Errorf("%s GetOptionsSpecifiedContractDetail() error %v", e.Name, err)
	}
}

func TestGetSettlementHistory(t *testing.T) {
	t.Parallel()

	_, err := e.GetSettlementHistory(t.Context(), "", 0, 0, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errInvalidUnderlying, "GetSettlementHistory must reject an empty underlying")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetSettlementHistory(t.Context(), gateIOTestBTCUSDT, 0, 0, from, from.Add(-time.Minute))
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetSettlementHistory must reject a reversed time range")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/options/settlements", `[{"contract":"BTC_USDT","settle_price":"2"}]`)
	settlements, err := ex.GetSettlementHistory(t.Context(), gateIOTestBTCUSDT, 3, 25, from, to)
	require.NoError(t, err, "GetSettlementHistory must not error")
	require.Len(t, settlements, 1, "GetSettlementHistory must decode one settlement")
	assert.Equal(t, gateIOTestBTCUSDT, settlements[0].Contract, "settlement contract should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/options/settlements", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestFromQueryKey:  {"1700000000"},
		gateIOTestLimitQueryKey: {"25"},
		"offset":                {"3"},
		"to":                    {"1700000060"},
		"underlying":            {gateIOTestBTCUSDT},
	}, gotRequest.query, "request query should match")

	settlements, err = ex.GetSettlementHistory(t.Context(), gateIOTestBTCUSDT, 0, 0, time.Time{}, time.Time{})
	require.NoError(t, err, "GetSettlementHistory must accept omitted optional parameters")
	require.Len(t, settlements, 1, "GetSettlementHistory must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{"underlying": {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/options/settlements", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetSettlementHistory(ctx, gateIOTestBTCUSDT, 0, 0, time.Time{}, time.Time{})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetSettlementHistory(t.Context(), gateIOTestBTCUSDT, 0, 0, time.Time{}, time.Time{})
		require.NoError(t, err, "GetSettlementHistory must not error against the live API")
	})
}

func TestGetOptionsSpecifiedSettlementHistory(t *testing.T) {
	t.Parallel()
	underlying := gateIOTestBTCUSDT
	optionsSettlement, err := e.GetSettlementHistory(t.Context(), underlying, 0, 1, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	cp, err := currency.NewPairFromString(optionsSettlement[0].Contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.GetOptionsSpecifiedContractsSettlement(t.Context(), cp, underlying, optionsSettlement[0].Timestamp.Time().Unix()); err != nil {
		t.Errorf("%s GetOptionsSpecifiedContractsSettlement() error %s", e.Name, err)
	}
}

func TestGetSupportedFlashSwapCurrencies(t *testing.T) {
	t.Parallel()
	if _, err := e.GetSupportedFlashSwapCurrencies(t.Context()); err != nil {
		t.Errorf("%s GetSupportedFlashSwapCurrencies() error %v", e.Name, err)
	}
}

const flashSwapOrderResponseJSON = `{"id": 54646,  "create_time": 1651116876378,  "update_time": 1651116876378,  "user_id": 11135567,  "sell_currency": "BTC",  "sell_amount": "0.01",  "buy_currency": "USDT",  "buy_amount": "10",  "price": "100",  "status": 1}`

func TestCreateFlashSwapOrder(t *testing.T) {
	t.Parallel()
	var response FlashSwapOrderResponse
	if err := json.Unmarshal([]byte(flashSwapOrderResponseJSON), &response); err != nil {
		t.Errorf("%s error while deserialising to FlashSwapOrderResponse %v", e.Name, err)
	}
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CreateFlashSwapOrder(t.Context(), FlashSwapOrderParams{
		PreviewID:    "1234",
		SellCurrency: currency.USDT,
		BuyCurrency:  currency.BTC,
		BuyAmount:    34234,
		SellAmount:   34234,
	}); err != nil {
		t.Errorf("%s CreateFlashSwapOrder() error %v", e.Name, err)
	}
}

func TestGetAllFlashSwapOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetAllFlashSwapOrders(t.Context(), 1, currency.EMPTYCODE, currency.EMPTYCODE, true, 0, 0); err != nil {
		t.Errorf("%s GetAllFlashSwapOrders() error %v", e.Name, err)
	}
}

func TestGetSingleFlashSwapOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSingleFlashSwapOrder(t.Context(), "1234"); err != nil {
		t.Errorf("%s GetSingleFlashSwapOrder() error %v", e.Name, err)
	}
}

func TestInitiateFlashSwapOrderReview(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.InitiateFlashSwapOrderReview(t.Context(), FlashSwapOrderParams{
		PreviewID:    "1234",
		SellCurrency: currency.USDT,
		BuyCurrency:  currency.BTC,
		SellAmount:   100,
	}); err != nil {
		t.Errorf("%s InitiateFlashSwapOrderReview() error %v", e.Name, err)
	}
}

func TestGetMyOptionsSettlements(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetMyOptionsSettlements(t.Context(), gateIOTestBTCUSDT, currency.EMPTYPAIR, 0, 0, time.Time{}); err != nil {
		t.Errorf("%s GetMyOptionsSettlements() error %v", e.Name, err)
	}
}

func TestGetOptionAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetOptionAccounts(t.Context()); err != nil {
		t.Errorf("%s GetOptionAccounts() error %v", e.Name, err)
	}
}

func TestGetAccountChangingHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetAccountChangingHistory(t.Context(), 0, 0, time.Time{}, time.Time{}, ""); err != nil {
		t.Errorf("%s GetAccountChangingHistory() error %v", e.Name, err)
	}
}

func TestGetUsersPositionSpecifiedUnderlying(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetUsersPositionSpecifiedUnderlying(t.Context(), ""); err != nil {
		t.Errorf("%s GetUsersPositionSpecifiedUnderlying() error %v", e.Name, err)
	}
}

func TestGetSpecifiedContractPosition(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpecifiedContractPosition(t.Context(), currency.EMPTYPAIR)
	assert.ErrorIs(t, err, errInvalidOrMissingContractParam)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err = e.GetSpecifiedContractPosition(t.Context(), getPair(t, asset.Options))
	assert.NoError(t, err, "GetSpecifiedContractPosition should not error")
}

func TestGetUsersLiquidationHistoryForSpecifiedUnderlying(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetUsersLiquidationHistoryForSpecifiedUnderlying(t.Context(), gateIOTestBTCUSDT, currency.EMPTYPAIR); err != nil {
		t.Errorf("%s GetUsersLiquidationHistoryForSpecifiedUnderlying() error %v", e.Name, err)
	}
}

func TestPlaceOptionOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err := e.PlaceOptionOrder(t.Context(), &OptionOrderParam{
		Contract:    getPair(t, asset.Options).String(),
		OrderSize:   -1,
		Iceberg:     0,
		Text:        "-",
		TimeInForce: "gtc",
		Price:       100,
	})
	if err != nil {
		t.Errorf("%s PlaceOptionOrder() error %v", e.Name, err)
	}
}

func TestGetOptionFuturesOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetOptionFuturesOrders(t.Context(), currency.EMPTYPAIR, "", "", 0, 0, time.Time{}, time.Time{}); err != nil {
		t.Errorf("%s GetOptionFuturesOrders() error %v", e.Name, err)
	}
}

func TestCancelOptionOpenOrders(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CancelMultipleOptionOpenOrders(t.Context(), getPair(t, asset.Options), "", ""); err != nil {
		t.Errorf("%s CancelOptionOpenOrders() error %v", e.Name, err)
	}
}

func TestGetSingleOptionOrder(t *testing.T) {
	t.Parallel()
	_, err := e.GetSingleOptionOrder(t.Context(), "")
	assert.ErrorIs(t, err, errInvalidOrderID)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)

	_, err = e.GetSingleOptionOrder(t.Context(), "1234")
	assert.NoError(t, err, "GetSingleOptionOrder should not error")
}

func TestCancelSingleOrder(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CancelOptionSingleOrder(t.Context(), "1234"); err != nil {
		t.Errorf("%s CancelSingleOrder() error %v", e.Name, err)
	}
}

func TestGetMyOptionsTradingHistory(t *testing.T) {
	t.Parallel()

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetMyOptionsTradingHistory(t.Context(), gateIOTestBTCUSDT, currency.EMPTYPAIR, 0, 0, time.Time{}, time.Time{})
	require.NoError(t, err)
}

func TestWithdrawCurrency(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawCurrency(t.Context(), WithdrawalRequestParam{})
	assert.ErrorIs(t, err, errInvalidAmount)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	_, err = e.WithdrawCurrency(t.Context(), WithdrawalRequestParam{
		Currency: currency.BTC,
		Amount:   0.00000001,
		Chain:    "BTC",
		Address:  core.BitcoinDonationAddress,
	})
	if err != nil {
		t.Errorf("%s WithdrawCurrency() expecting error %v, but found %v", e.Name, errInvalidAmount, err)
	}
}

func TestCancelWithdrawalWithSpecifiedID(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CancelWithdrawalWithSpecifiedID(t.Context(), "1234567"); err != nil {
		t.Errorf("%s CancelWithdrawalWithSpecifiedID() error %v", e.Name, err)
	}
}

func TestGetOptionsOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.GetOptionsOrderbook(t.Context(), getPair(t, asset.Options), "0.1", 9, true)
	assert.NoError(t, err, "GetOptionsOrderbook should not error")
}

func TestGetOptionsTickers(t *testing.T) {
	t.Parallel()
	if _, err := e.GetOptionsTickers(t.Context(), gateIOTestBTCUSDT); err != nil {
		t.Errorf("%s GetOptionsTickers() error %v", e.Name, err)
	}
}

func TestGetOptionUnderlyingTickers(t *testing.T) {
	t.Parallel()
	if _, err := e.GetOptionUnderlyingTickers(t.Context(), gateIOTestBTCUSDT); err != nil {
		t.Errorf("%s GetOptionUnderlyingTickers() error %v", e.Name, err)
	}
}

func TestGetOptionFuturesCandlesticks(t *testing.T) {
	t.Parallel()

	_, err := e.GetOptionFuturesCandlesticks(t.Context(), currency.EMPTYPAIR, 0, time.Time{}, time.Time{}, kline.OneDay)
	require.ErrorIs(t, err, errInvalidOrMissingContractParam, "GetOptionFuturesCandlesticks must reject an empty contract")
	pair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetOptionFuturesCandlesticks(t.Context(), pair, 0, from, from.Add(-time.Minute), kline.OneDay)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetOptionFuturesCandlesticks must reject a reversed time range")
	_, err = e.GetOptionFuturesCandlesticks(t.Context(), pair, 0, time.Time{}, time.Time{}, kline.ThreeDay)
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "GetOptionFuturesCandlesticks must reject an unsupported interval")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/options/candlesticks", `[{"t":1738108800,"c":"2","n":"BTC_USDT"}]`)
	candlesticks, err := ex.GetOptionFuturesCandlesticks(t.Context(), pair, 25, from, to, kline.OneDay)
	require.NoError(t, err, "GetOptionFuturesCandlesticks must not error")
	require.Len(t, candlesticks, 1, "GetOptionFuturesCandlesticks must decode one candlestick")
	assert.Equal(t, gateIOTestBTCUSDT, candlesticks[0].Name, "contract should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/options/candlesticks", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		gateIOTestIntervalQueryKey: {"1d"},
		gateIOTestLimitQueryKey:    {"25"},
		"to":                       {"1700000060"},
	}, gotRequest.query, "request query should match")

	candlesticks, err = ex.GetOptionFuturesCandlesticks(t.Context(), pair, 0, time.Time{}, time.Time{}, kline.OneDay)
	require.NoError(t, err, "GetOptionFuturesCandlesticks must accept omitted optional parameters")
	require.Len(t, candlesticks, 1, "GetOptionFuturesCandlesticks must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestContractQueryKey: {gateIOTestBTCUSDT}, gateIOTestIntervalQueryKey: {"1d"}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/options/candlesticks", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetOptionFuturesCandlesticks(ctx, pair, 0, time.Time{}, time.Time{}, kline.OneDay)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetOptionFuturesCandlesticks(t.Context(), getPair(t, asset.Options), 0, time.Now().Add(-10*time.Hour), time.Time{}, kline.ThirtyMin)
		require.NoError(t, err, "GetOptionFuturesCandlesticks must not error against the live API")
	})
}

func TestGetOptionFuturesMarkPriceCandlesticks(t *testing.T) {
	t.Parallel()

	_, err := e.GetOptionFuturesMarkPriceCandlesticks(t.Context(), "", 0, time.Time{}, time.Time{}, 0)
	require.ErrorIs(t, err, errInvalidUnderlying, "GetOptionFuturesMarkPriceCandlesticks must reject an empty underlying")
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err = e.GetOptionFuturesMarkPriceCandlesticks(t.Context(), gateIOTestBTCUSDT, 0, from, from.Add(-time.Minute), 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetOptionFuturesMarkPriceCandlesticks must reject a reversed time range")
	_, err = e.GetOptionFuturesMarkPriceCandlesticks(t.Context(), gateIOTestBTCUSDT, 0, time.Time{}, time.Time{}, kline.ThreeDay)
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "GetOptionFuturesMarkPriceCandlesticks must reject an unsupported interval")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/options/underlying/candlesticks", `[{"t":1738108800,"c":"2","n":"BTC_USDT"}]`)
	candlesticks, err := ex.GetOptionFuturesMarkPriceCandlesticks(t.Context(), gateIOTestBTCUSDT, 25, from, to, kline.OneDay)
	require.NoError(t, err, "GetOptionFuturesMarkPriceCandlesticks must not error")
	require.Len(t, candlesticks, 1, "GetOptionFuturesMarkPriceCandlesticks must decode one candlestick")
	assert.Equal(t, gateIOTestBTCUSDT, candlesticks[0].Name, "contract should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/options/underlying/candlesticks", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestFromQueryKey:     {"1700000000"},
		gateIOTestIntervalQueryKey: {"1d"},
		gateIOTestLimitQueryKey:    {"25"},
		"to":                       {"1700000060"},
		"underlying":               {gateIOTestBTCUSDT},
	}, gotRequest.query, "request query should match")

	candlesticks, err = ex.GetOptionFuturesMarkPriceCandlesticks(t.Context(), gateIOTestBTCUSDT, 0, time.Time{}, time.Time{}, 0)
	require.NoError(t, err, "GetOptionFuturesMarkPriceCandlesticks must accept omitted optional parameters")
	require.Len(t, candlesticks, 1, "GetOptionFuturesMarkPriceCandlesticks must decode the response with omitted optional parameters")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{"underlying": {gateIOTestBTCUSDT}}, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/options/underlying/candlesticks", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetOptionFuturesMarkPriceCandlesticks(ctx, gateIOTestBTCUSDT, 0, time.Time{}, time.Time{}, kline.OneDay)
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetOptionFuturesMarkPriceCandlesticks(t.Context(), gateIOTestBTCUSDT, 0, time.Time{}, time.Time{}, kline.OneMonth)
		require.NoError(t, err, "GetOptionFuturesMarkPriceCandlesticks must not error against the live API")
	})
}

func TestGetOptionsTradeHistory(t *testing.T) {
	t.Parallel()

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Minute)
	_, err := e.GetOptionsTradeHistory(t.Context(), currency.EMPTYPAIR, "C", 0, 0, from, from.Add(-time.Minute))
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetOptionsTradeHistory must reject a reversed time range")
	_, err = e.GetOptionsTradeHistory(t.Context(), currency.EMPTYPAIR, "invalid", 0, 0, time.Time{}, time.Time{})
	require.ErrorIs(t, err, errInvalidOptionsCallType, "GetOptionsTradeHistory must reject an unsupported call type")

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/options/trades", `[{"id":7,"contract":"BTC_USDT","price":"2"}]`)
	trades, err := ex.GetOptionsTradeHistory(t.Context(), currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter), "c", 3, 25, from, to)
	require.NoError(t, err, "GetOptionsTradeHistory must not error")
	require.Len(t, trades, 1, "GetOptionsTradeHistory must decode one trade")
	assert.Equal(t, int64(7), trades[0].ID, "trade ID should match")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, "/api/v4/options/trades", gotRequest.path, "request path should match")
	assert.Equal(t, url.Values{
		gateIOTestContractQueryKey: {gateIOTestBTCUSDT},
		gateIOTestFromQueryKey:     {"1700000000"},
		gateIOTestLimitQueryKey:    {"25"},
		"offset":                   {"3"},
		"to":                       {"1700000060"},
		gateIOTestTypeQueryKey:     {"C"},
	}, gotRequest.query, "request query should match")

	_, err = ex.GetOptionsTradeHistory(t.Context(), currency.EMPTYPAIR, "p", 0, 0, time.Time{}, time.Time{})
	require.NoError(t, err, "GetOptionsTradeHistory must accept put call types")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Equal(t, url.Values{gateIOTestTypeQueryKey: {"P"}}, gotRequest.query, "put call type should be normalized")

	_, err = ex.GetOptionsTradeHistory(t.Context(), currency.EMPTYPAIR, "", 0, 0, time.Time{}, time.Time{})
	require.NoError(t, err, "GetOptionsTradeHistory must accept an omitted call type")
	gotRequest = requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "zero-value optional parameters should be omitted")

	requireGateIORequestErrors(t, "/api/v4/options/trades", true, func(ctx context.Context, ex *Exchange) error {
		_, err := ex.GetOptionsTradeHistory(ctx, currency.EMPTYPAIR, "C", 0, 0, time.Time{}, time.Time{})
		return err
	})

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		_, err := e.GetOptionsTradeHistory(t.Context(), getPair(t, asset.Options), "C", 0, 0, time.Time{}, time.Time{})
		require.NoError(t, err, "GetOptionsTradeHistory must not error against the live API")
	})
}

// Sub-account endpoints

func TestCreateNewSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CreateNewSubAccount(t.Context(), SubAccountParams{
		LoginName: "Sub_Account_for_testing",
	}); err != nil {
		t.Errorf("%s CreateNewSubAccount() error %v", e.Name, err)
	}
}

func TestGetSubAccounts(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSubAccounts(t.Context()); err != nil {
		t.Errorf("%s GetSubAccounts() error %v", e.Name, err)
	}
}

func TestGetSingleSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetSingleSubAccount(t.Context(), "123423"); err != nil {
		t.Errorf("%s GetSingleSubAccount() error %v", e.Name, err)
	}
}

// Wrapper test functions

func TestFetchTradablePairs(t *testing.T) {
	t.Parallel()
	t.Run("margin product sources", testFetchTradablePairsUsesMarginProductSources)

	responses := map[string]string{
		gateIOTestSpotCurrencyPairsPath:     `[{"id":"BTC_USDT","base":"BTC","quote":"USDT","trade_status":"tradable"},{"id":"DOGE_USDT","base":"DOGE","quote":"USDT","trade_status":"untradable"}]`,
		"/api/v4/margin/uni/currency_pairs": `[{"currency_pair":"BTC_USDT","status":"enabled","delisted_time":0},{"currency_pair":"DOGE_USDT","status":"disabled","delisted_time":0}]`,
		gateIOTestCrossMarginCurrenciesPath: `[{"name":"BTC","min_borrow_amount":"1","loanable":true,"status":1},{"name":"USDT","min_borrow_amount":"1","loanable":true,"status":1}]`,
		"/api/v4/futures/btc/contracts":     `[{"name":"BTC_USD","delisted_time":0},{"name":"DOGE_USD","delisted_time":1600000000}]`,
		"/api/v4/futures/usdt/contracts":    `[{"name":"BTC_USDT","delisted_time":0},{"name":"DOGE_USDT","delisted_time":1600000000}]`,
		gateIOTestDeliveryUSDTContractsPath: `[{"name":"BTC_USDT_20260925","in_delisting":false},{"name":"DOGE_USDT_20260925","in_delisting":true}]`,
		gateIOTestOptionsUnderlyingsPath:    gateIOTestOptionsUnderlyingResponse,
		gateIOTestOptionsContractsPath:      `[{"name":"BTC_USDT-20260925-50000-C"}]`,
	}
	ex := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing tradable-pairs response should not error")
	}))
	expectedPairs := map[asset.Item]currency.Pair{
		asset.Spot:                currency.NewBTCUSDT(),
		asset.Margin:              currency.NewBTCUSDT(),
		asset.CrossMargin:         currency.NewBTCUSDT(),
		asset.CoinMarginedFutures: currency.NewPairWithDelimiter("BTC", "USD", currency.UnderscoreDelimiter),
		asset.USDTMarginedFutures: currency.NewBTCUSDT(),
		asset.DeliveryFutures:     currency.NewPairWithDelimiter("BTC", "USDT_20260925", currency.UnderscoreDelimiter),
		asset.Options:             currency.NewPairWithDelimiter("BTC", "USDT-20260925-50000-C", currency.UnderscoreDelimiter),
	}
	for _, a := range []asset.Item{asset.Spot, asset.Margin, asset.CrossMargin, asset.CoinMarginedFutures, asset.USDTMarginedFutures, asset.DeliveryFutures, asset.Options} {
		pairs, err := ex.FetchTradablePairs(t.Context(), a)
		require.NoErrorf(t, err, "FetchTradablePairs must not error for %s", a)
		require.Lenf(t, pairs, 1, "FetchTradablePairs must filter unavailable pairs for %s", a)
		assert.Truef(t, pairs[0].Equal(expectedPairs[a]), "FetchTradablePairs should retain the expected contract for %s", a)
	}

	_, err := ex.FetchTradablePairs(t.Context(), asset.Empty)
	require.ErrorIs(t, err, asset.ErrNotSupported, "FetchTradablePairs must reject an unsupported asset")
	ex.CurrencyPairs.Pairs[asset.Futures] = new(currency.PairStore)
	_, err = ex.FetchTradablePairs(t.Context(), asset.Futures)
	require.ErrorIs(t, err, asset.ErrNotSupported, "FetchTradablePairs must reject an unsupported mapped asset")

	failing := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := responses[r.URL.Path]; !ok {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, `{`)
		assert.NoError(t, err, "writing invalid response should not error")
	}))
	for _, a := range []asset.Item{asset.Spot, asset.Margin, asset.CrossMargin, asset.CoinMarginedFutures, asset.USDTMarginedFutures, asset.DeliveryFutures, asset.Options} {
		_, err = failing.FetchTradablePairs(t.Context(), a)
		require.Errorf(t, err, "FetchTradablePairs must return endpoint errors for %s", a)
	}

	crossSpotFailure := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response string
		switch r.URL.Path {
		case gateIOTestCrossMarginCurrenciesPath:
			response = `[{"name":"BTC","min_borrow_amount":"1","loanable":true,"status":1}]`
		case gateIOTestSpotCurrencyPairsPath:
			response = `{`
		default:
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing cross-margin response should not error")
	}))
	_, err = crossSpotFailure.FetchTradablePairs(t.Context(), asset.CrossMargin)
	require.Error(t, err, "FetchTradablePairs must return the spot endpoint error for cross margin")

	deliveryParseFailure := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != gateIOTestDeliveryUSDTContractsPath {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, `[{"name":"","in_delisting":false}]`)
		assert.NoError(t, err, "writing delivery response should not error")
	}))
	_, err = deliveryParseFailure.FetchTradablePairs(t.Context(), asset.DeliveryFutures)
	require.Error(t, err, "FetchTradablePairs must return delivery pair parsing errors")

	optionsFailures := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response string
		switch r.URL.Path {
		case gateIOTestOptionsUnderlyingsPath:
			response = gateIOTestOptionsUnderlyingResponse
		case gateIOTestOptionsContractsPath:
			response = `{`
		default:
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing options response should not error")
	}))
	_, err = optionsFailures.FetchTradablePairs(t.Context(), asset.Options)
	require.Error(t, err, "FetchTradablePairs must return options contract endpoint errors")

	optionsParseFailure := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response string
		switch r.URL.Path {
		case gateIOTestOptionsUnderlyingsPath:
			response = gateIOTestOptionsUnderlyingResponse
		case gateIOTestOptionsContractsPath:
			response = `[{"name":""}]`
		default:
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing options response should not error")
	}))
	_, err = optionsParseFailure.FetchTradablePairs(t.Context(), asset.Options)
	require.Error(t, err, "FetchTradablePairs must return options pair parsing errors")

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		for _, a := range e.GetAssetTypes(false) {
			pairs, err := e.FetchTradablePairs(t.Context(), a)
			require.NoErrorf(t, err, "FetchTradablePairs must not error for %s against the live API", a)
			require.NotEmptyf(t, pairs, "FetchTradablePairs must return pairs for %s from the live API", a)
			if a == asset.USDTMarginedFutures || a == asset.CoinMarginedFutures {
				for _, p := range pairs {
					_, err := getSettlementCurrency(p, a)
					require.NoErrorf(t, err, "fetched pair %s %s must have a live settlement currency", a, p)
				}
			}
		}
	})
}

func TestUpdateTickers(t *testing.T) {
	t.Parallel()
	for _, a := range e.GetAssetTypes(false) {
		err := e.UpdateTickers(t.Context(), a)
		assert.NoErrorf(t, err, "UpdateTickers should not error for %s", a)
	}

	ex, requests := setupGateIOHTTPTest(t, http.MethodGet, "/api/v4/futures/btc/tickers", `[{"contract":"BTC_USD","last":"2"}]`)
	require.NoError(t, ex.UpdateTickers(t.Context(), asset.CoinMarginedFutures), "UpdateTickers must not error for coin-margined futures")
	gotRequest := requireGateIOHTTPRequest(t, requests)
	assert.Empty(t, gotRequest.query, "request query should be empty")
}

func TestUpdateOrderbook(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateOrderbook(t.Context(), currency.EMPTYPAIR, 1336)
	require.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)
	for _, a := range e.GetAssetTypes(false) {
		pair := getPair(t, a)
		t.Run(a.String()+" "+pair.String(), func(t *testing.T) {
			t.Parallel()
			o, err := e.UpdateOrderbook(t.Context(), pair, a)
			require.NoError(t, err)
			if a != asset.Options { // Options orderbooks can be empty
				assert.NotEmpty(t, o.Bids)
				assert.NotEmpty(t, o.Asks)
			}
		})
	}
}

func TestGetWithdrawalsHistory(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if _, err := e.GetWithdrawalsHistory(t.Context(), currency.BTC, asset.Empty); err != nil {
		t.Errorf("%s GetWithdrawalsHistory() error %v", e.Name, err)
	}
}

func TestGetRecentTrades(t *testing.T) {
	t.Parallel()
	for _, a := range e.GetAssetTypes(false) {
		if a != asset.CoinMarginedFutures {
			_, err := e.GetRecentTrades(t.Context(), getPair(t, a), a)
			assert.NoErrorf(t, err, "GetRecentTrades should not error for %s", a)
		}
	}
}

func TestSubmitOrder(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	for _, a := range e.GetAssetTypes(false) {
		_, err := e.SubmitOrder(t.Context(), &order.Submit{
			Exchange:    e.Name,
			Pair:        getPair(t, a),
			Side:        order.Buy,
			Type:        order.Limit,
			Price:       1,
			Amount:      1,
			AssetType:   a,
			TimeInForce: order.GoodTillCancel,
		})
		assert.NoErrorf(t, err, "SubmitOrder should not error for %s", a)
	}
}

func TestCancelExchangeOrder(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	for _, a := range e.GetAssetTypes(false) {
		orderCancellation := &order.Cancel{
			OrderID:   "1",
			AccountID: "1",
			Pair:      getPair(t, a),
			AssetType: a,
		}
		err := e.CancelOrder(t.Context(), orderCancellation)
		assert.NoErrorf(t, err, "CancelOrder should not error for %s", a)
	}
}

func TestCancelBatchOrders(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	for _, a := range e.GetAssetTypes(false) {
		_, err := e.CancelBatchOrders(t.Context(), []order.Cancel{
			{
				OrderID:   "1",
				AccountID: "1",
				Pair:      getPair(t, a),
				AssetType: a,
			}, {
				OrderID:   "2",
				AccountID: "1",
				Pair:      getPair(t, a),
				AssetType: a,
			},
		})
		assert.NoErrorf(t, err, "CancelBatchOrders should not error for %s", a)
	}
}

func TestGetDepositAddress(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	chains, err := e.GetAvailableTransferChains(t.Context(), currency.BTC)
	if err != nil {
		t.Fatal(err)
	}
	for i := range chains {
		_, err = e.GetDepositAddress(t.Context(), currency.BTC, "", chains[i])
		if err != nil {
			t.Error("Test Fail - GetDepositAddress error", err)
		}
	}
}

func TestGetActiveOrders(t *testing.T) {
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	for _, a := range e.GetAssetTypes(false) {
		enabledPairs := getPairs(t, a)
		if len(enabledPairs) > 2 {
			enabledPairs = enabledPairs[:2]
		}
		_, err := e.GetActiveOrders(t.Context(), &order.MultiOrderRequest{
			Pairs:     enabledPairs,
			Type:      order.AnyType,
			Side:      order.AnySide,
			AssetType: a,
		})
		assert.NoErrorf(t, err, "GetActiveOrders should not error for %s", a)
	}
}

func TestGetOrderHistory(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	type testCase struct {
		name         string
		requiresAuth bool
		request      order.MultiOrderRequest
		expectedErr  error
	}

	testCases := make([]testCase, 0, len(e.GetAssetTypes(false))*2+1)
	for _, a := range e.GetAssetTypes(false) {
		enabledPairs := getPairs(t, a)
		enabledPairs = enabledPairs[:min(4, len(enabledPairs))]

		withPairs := testCase{
			name:         a.String() + "/with_pairs",
			requiresAuth: true,
			request: order.MultiOrderRequest{
				Type:      order.AnyType,
				Side:      order.Buy,
				Pairs:     enabledPairs,
				AssetType: a,
			},
		}
		testCases = append(testCases, withPairs)

		noPairs := testCase{
			name:         a.String() + "/without_pairs",
			requiresAuth: true,
			request: order.MultiOrderRequest{
				Type:      order.AnyType,
				Side:      order.Buy,
				AssetType: a,
			},
		}
		if a == asset.Options {
			noPairs.requiresAuth = false
			noPairs.expectedErr = currency.ErrCurrencyPairsEmpty
		}
		testCases = append(testCases, noPairs)
	}

	testCases = append(testCases, testCase{
		name:         "unsupported/default_case_binary",
		requiresAuth: false,
		request: order.MultiOrderRequest{
			Type:      order.AnyType,
			Side:      order.Buy,
			AssetType: asset.Binary,
		},
		expectedErr: asset.ErrNotSupported,
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.requiresAuth {
				sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
			}
			orders, err := e.GetOrderHistory(t.Context(), &tc.request)
			if tc.expectedErr != nil {
				assert.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			for j := range orders {
				assert.Equal(t, tc.request.AssetType, orders[j].AssetType)
				assert.Equal(t, e.Name, orders[j].Exchange)
				assert.True(t, orders[j].Pair.IsPopulated(), "pair should be populated for order history response")
			}
		})
	}
}

func TestGetOrderHistoryRequestImmutability(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	enabledPairs := getPairs(t, asset.Spot)
	enabledPairs = enabledPairs[:min(2, len(enabledPairs))]

	type testCase struct {
		name    string
		request order.MultiOrderRequest
	}

	testCases := []testCase{
		{
			name: "nil_pairs",
			request: order.MultiOrderRequest{
				Type:      order.AnyType,
				Side:      order.Buy,
				AssetType: asset.Spot,
			},
		},
		{
			name: "provided_pairs",
			request: order.MultiOrderRequest{
				Type:      order.AnyType,
				Side:      order.Buy,
				Pairs:     slices.Clone(enabledPairs),
				AssetType: asset.Spot,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expectedPairs := slices.Clone(tc.request.Pairs)
			_, err := e.GetOrderHistory(t.Context(), &tc.request)
			require.NoError(t, err)
			assert.Equal(t, expectedPairs, tc.request.Pairs)
		})
	}
}

func TestGetHistoricCandles(t *testing.T) {
	t.Parallel()
	startTime := time.Now().Add(-time.Hour * 10)
	for _, a := range e.GetAssetTypes(false) {
		_, err := e.GetHistoricCandles(t.Context(), getPair(t, a), a, kline.OneDay, startTime, time.Now())
		if a == asset.Options {
			assert.ErrorIs(t, err, asset.ErrNotSupported, "GetHistoricCandles should error correctly for options")
		} else {
			assert.NoErrorf(t, err, "GetHistoricCandles should not error for %s", a)
		}
	}
}

func TestGetHistoricCandlesExtended(t *testing.T) {
	t.Parallel()
	startTime := time.Now().Add(-time.Hour * 5)
	for _, a := range e.GetAssetTypes(false) {
		_, err := e.GetHistoricCandlesExtended(t.Context(), getPair(t, a), a, kline.OneMin, startTime, time.Now())
		if a == asset.Options {
			assert.ErrorIs(t, err, asset.ErrNotSupported, "GetHistoricCandlesExtended should error correctly for options")
		} else {
			assert.NoErrorf(t, err, "GetHistoricCandlesExtended should not error for %s", a)
		}
	}
}

func TestGetAvailableTransferTrains(t *testing.T) {
	t.Parallel()
	_, err := e.GetAvailableTransferChains(t.Context(), currency.USDT)
	if err != nil {
		t.Error(err)
	}
}

func TestGetUnderlyingFromCurrencyPair(t *testing.T) {
	t.Parallel()
	if uly, err := e.GetUnderlyingFromCurrencyPair(currency.Pair{Delimiter: currency.UnderscoreDelimiter, Base: currency.BTC, Quote: currency.NewCode("USDT_LLK")}); err != nil {
		t.Error(err)
	} else if !uly.Equal(currency.NewBTCUSDT()) {
		t.Error("unexpected underlying")
	}
}

const wsTickerPushDataJSON = `{"time": 1606291803,	"channel": "spot.tickers",	"event": "update",	"result": {	  "currency_pair": "BTC_USDT",	  "last": "19106.55",	  "lowest_ask": "19108.71",	  "highest_bid": "19106.55",	  "change_percentage": "3.66",	  "base_volume": "2811.3042155865",	  "quote_volume": "53441606.52411221454674732293",	  "high_24h": "19417.74",	  "low_24h": "18434.21"	}}`

func TestWsTickerPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleSpotData(t.Context(), nil, []byte(wsTickerPushDataJSON)); err != nil {
		t.Errorf("%s websocket ticker push data error: %v", e.Name, err)
	}
}

const wsTradePushDataJSON = `{	"time": 1606292218,	"channel": "spot.trades",	"event": "update",	"result": {	  "id": 309143071,	  "create_time": 1606292218,	  "create_time_ms": "1606292218213.4578",	  "side": "sell",	  "currency_pair": "BTC_USDT",	  "amount": "16.4700000000",	  "price": "0.4705000000"}}`

func TestWsTradePushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleSpotData(t.Context(), nil, []byte(wsTradePushDataJSON)); err != nil {
		t.Errorf("%s websocket trade push data error: %v", e.Name, err)
	}
}

const wsCandlestickPushDataJSON = `{"time": 1606292600,	"channel": "spot.candlesticks",	"event": "update",	"result": {	  "t": "1606292580",	  "v": "2362.32035",	  "c": "19128.1",	  "h": "19128.1",	  "l": "19128.1",	  "o": "19128.1","n": "1m_BTC_USDT"}}`

func TestWsCandlestickPushData(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Test instance Setup must not error")
	require.NoError(t, ex.WsHandleSpotData(t.Context(), nil, []byte(wsCandlestickPushDataJSON)))

	select {
	case msg := <-ex.Websocket.DataHandler.C:
		got, ok := msg.Data.([]kline.Item)
		require.True(t, ok, "expected []kline.Item")
		require.NotEmpty(t, got, "expected at least one candle")
		for _, item := range got {
			assert.Equal(t, kline.OneMin, item.Interval)
			assert.Equal(t, currency.NewPairWithDelimiter("BTC", "USDT", "_"), item.Pair)
			assert.Equal(t, ex.Name, item.Exchange)
			require.Len(t, item.Candles, 1)
			assert.Equal(t, kline.Candle{
				Time:   time.Unix(1606292580, 0),
				Open:   19128.1,
				Close:  19128.1,
				High:   19128.1,
				Low:    19128.1,
				Volume: 2362.32035,
			}, item.Candles[0])
		}
	default:
		require.Fail(t, "expected websocket candlestick payload")
	}
}

const wsOrderbookTickerJSON = `{"time": 1606293275,	"channel": "spot.book_ticker",	"event": "update",	"result": {	  "t": 1606293275123,	  "u": 48733182,	  "s": "BTC_USDT",	  "b": "19177.79",	  "B": "0.0003341504",	  "a": "19179.38",	  "A": "0.09"	}}`

func TestWsOrderbookTickerPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleSpotData(t.Context(), nil, []byte(wsOrderbookTickerJSON)); err != nil {
		t.Errorf("%s websocket orderbook push data error: %v", e.Name, err)
	}
}

const (
	wsOrderbookUpdatePushDataJSON   = `{"time": 1606294781,	"channel": "spot.order_book_update",	"event": "update",	"result": {	  "t": 1606294781123,	  "e": "depthUpdate",	  "E": 1606294781,"s": "BTC_USDT","U": 48776301,"u": 48776306,"b": [["19137.74","0.0001"],["19088.37","0"]],"a": [["19137.75","0.6135"]]	}}`
	wsOrderbookSnapshotPushDataJSON = `{"time":1606295412,"channel": "spot.order_book",	"event": "update",	"result": {	  "t": 1606295412123,	  "lastUpdateId": 48791820,	  "s": "BTC_USDT",	  "bids": [		[		  "19079.55",		  "0.0195"		],		[		  "19079.07",		  "0.7341"],["19076.23",		  "0.00011808"		],		[		  "19073.9",		  "0.105"		],		[		  "19068.83",		  "0.1009"		]	  ],	  "asks": [		[		  "19080.24",		  "0.1638"		],		[		  "19080.91","0.1366"],["19080.92","0.01"],["19081.29","0.01"],["19083.8","0.097"]]}}`
)

func TestWsOrderbookSnapshotPushData(t *testing.T) {
	t.Parallel()
	err := e.WsHandleSpotData(t.Context(), nil, []byte(wsOrderbookSnapshotPushDataJSON))
	if err != nil {
		t.Errorf("%s websocket orderbook snapshot push data error: %v", e.Name, err)
	}
	if err = e.WsHandleSpotData(t.Context(), nil, []byte(wsOrderbookUpdatePushDataJSON)); err != nil {
		t.Errorf("%s websocket orderbook update push data error: %v", e.Name, err)
	}
}

const wsSpotOrderPushDataJSON = `{"time": 1605175506,	"channel": "spot.orders",	"event": "update",	"result": [	  {		"id": "30784435",		"user": 123456,		"text": "t-abc",		"create_time": "1605175506",		"create_time_ms": "1605175506123",		"update_time": "1605175506",		"update_time_ms": "1605175506123",		"event": "put",		"currency_pair": "BTC_USDT",		"type": "limit",		"account": "spot",		"side": "sell",		"amount": "1",		"price": "10001",		"time_in_force": "gtc",		"left": "1",		"filled_total": "0",		"fee": "0",		"fee_currency": "USDT",		"point_fee": "0",		"gt_fee": "0",		"gt_discount": true,		"rebated_fee": "0",		"rebated_fee_currency": "USDT"}	]}`

func TestWsPushOrders(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleSpotData(t.Context(), nil, []byte(wsSpotOrderPushDataJSON)); err != nil {
		t.Errorf("%s websocket orders push data error: %v", e.Name, err)
	}
}

const wsUserTradePushDataJSON = `{"time": 1605176741,	"channel": "spot.usertrades",	"event": "update",	"result": [	  {		"id": 5736713,		"user_id": 1000001,		"order_id": "30784428",		"currency_pair": "BTC_USDT",		"create_time": 1605176741,		"create_time_ms": "1605176741123.456",		"side": "sell",		"amount": "1.00000000",		"role": "taker",		"price": "10000.00000000",		"fee": "0.00200000000000",		"point_fee": "0",		"gt_fee": "0",		"text": "apiv4"	  }	]}`

func TestWsUserTradesPushDataJSON(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleSpotData(t.Context(), nil, []byte(wsUserTradePushDataJSON)); err != nil {
		t.Errorf("%s websocket users trade push data error: %v", e.Name, err)
	}
}

const wsBalancesPushDataJSON = `{"time": 1605248616,	"channel": "spot.balances",	"event": "update",	"result": [	  {		"timestamp": "1605248616",		"timestamp_ms": "1605248616123",		"user": "1000001",		"currency": "USDT",		"change": "100",		"total": "1032951.325075926",		"available": "1022943.325075926"}	]}`

func TestBalancesPushData(t *testing.T) {
	t.Parallel()
	ctx := accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "test", Secret: "test"})
	if err := e.WsHandleSpotData(ctx, nil, []byte(wsBalancesPushDataJSON)); err != nil {
		t.Errorf("%s websocket balances push data error: %v", e.Name, err)
	}
}

const wsMarginBalancePushDataJSON = `{"time": 1605248616,	"channel": "spot.funding_balances",	"event": "update",	"result": [	  {"timestamp": "1605248616","timestamp_ms": "1605248616123","user": "1000001","currency": "USDT","change": "100","freeze": "100","lent": "0"}	]}`

func TestMarginBalancePushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleSpotData(t.Context(), nil, []byte(wsMarginBalancePushDataJSON)); err != nil {
		t.Errorf("%s websocket margin balance push data error: %v", e.Name, err)
	}
}

const wsCrossMarginBalancePushDataJSON = `{"time": 1605248616,"channel": "spot.cross_balances","event": "update",	"result": [{"timestamp": "1605248616","timestamp_ms": "1605248616123","user": "1000001","currency": "USDT",	"change": "100","total": "1032951.325075926","available": "1022943.325075926"}]}`

func TestCrossMarginBalancePushData(t *testing.T) {
	t.Parallel()
	ctx := accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "test", Secret: "test"})
	if err := e.WsHandleSpotData(ctx, nil, []byte(wsCrossMarginBalancePushDataJSON)); err != nil {
		t.Errorf("%s websocket cross margin balance push data error: %v", e.Name, err)
	}
}

const wsCrossMarginBalanceLoan = `{	"time":1658289372,	"channel":"spot.cross_loan",	"event":"update",	"result":{	  "timestamp":1658289372338,	  "user":"1000001",	  "currency":"BTC",	  "change":"0.01",	  "total":"4.992341029566",	  "available":"0.078054772536",	  "borrowed":"0.01",	  "interest":"0.00001375"	}}`

func TestCrossMarginBalanceLoan(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleSpotData(t.Context(), nil, []byte(wsCrossMarginBalanceLoan)); err != nil {
		t.Errorf("%s websocket cross margin loan push data error: %v", e.Name, err)
	}
}

// TestFuturesDataHandler ensures that messages from various futures channels do not error
func TestFuturesDataHandler(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	testexch.FixtureToDataHandler(t, "testdata/wsFutures.json", func(ctx context.Context, m []byte) error {
		if strings.Contains(string(m), "futures.balances") {
			ctx = accounts.DeployCredentialsToContext(ctx, &accounts.Credentials{Key: "test", Secret: "test"})
		}
		return e.WsHandleFuturesData(ctx, nil, m, asset.CoinMarginedFutures)
	})
	e.Websocket.DataHandler.Close()
	assert.Len(t, e.Websocket.DataHandler.C, 15, "Should see the correct number of messages")
	for resp := range e.Websocket.DataHandler.C {
		if err, isErr := resp.Data.(error); isErr {
			assert.NoError(t, err, "Should not get any errors down the data handler")
		}
	}
}

func TestProcessFuturesCandlesticksIntervalMapping(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Test instance Setup must not error")

	payload := []byte(`{"time":1606292600,"channel":"futures.candlesticks","event":"update","result":[{"t":1606292580,"v":"2362.32035","c":"19128.1","h":"19128.1","l":"19128.1","o":"19128.1","n":"1m_BTC_USDT"}]}`)
	require.NoError(t, ex.processFuturesCandlesticks(t.Context(), payload, asset.CoinMarginedFutures))

	select {
	case msg := <-ex.Websocket.DataHandler.C:
		got, ok := msg.Data.([]kline.Item)
		require.True(t, ok, "expected []kline.Item")
		assert.Equal(t, []kline.Item{{
			Pair:     currency.NewPairWithDelimiter("BTC", "USDT", "_"),
			Asset:    asset.CoinMarginedFutures,
			Exchange: ex.Name,
			Interval: kline.OneMin,
			Candles: []kline.Candle{{
				Time:   time.Unix(1606292580, 0),
				Open:   19128.1,
				Close:  19128.1,
				High:   19128.1,
				Low:    19128.1,
				Volume: 2362.32035,
			}},
		}}, got)
	default:
		require.Fail(t, "expected futures websocket candle payload")
	}
}

// ******************************************** Options web-socket unit test funcs ********************

const optionsContractTickerPushDataJSON = `{"time": 1630576352,	"channel": "options.contract_tickers",	"event": "update",	"result": {    "name": "BTC_USDT-20211231-59800-P",    "last_price": "11349.5",    "mark_price": "11170.19",    "index_price": "",    "position_size": 993,    "bid1_price": "10611.7",    "bid1_size": 100,    "ask1_price": "11728.7",    "ask1_size": 100,    "vega": "34.8731",    "theta": "-72.80588",    "rho": "-28.53331",    "gamma": "0.00003",    "delta": "-0.78311",    "mark_iv": "0.86695",    "bid_iv": "0.65481",    "ask_iv": "0.88145",    "leverage": "3.5541112718136"	}}`

func TestOptionsContractTickerPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsContractTickerPushDataJSON)); err != nil {
		t.Errorf("%s websocket options contract ticker push data failed with error %v", e.Name, err)
	}
}

const optionsUnderlyingTickerPushDataJSON = `{"time": 1630576352,	"channel": "options.ul_tickers",	"event": "update",	"result": {	   "trade_put": 800,	   "trade_call": 41700,	   "index_price": "50695.43",	   "name": "BTC_USDT"	}}`

func TestOptionsUnderlyingTickerPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsUnderlyingTickerPushDataJSON)); err != nil {
		t.Errorf("%s websocket options underlying ticker push data error: %v", e.Name, err)
	}
}

const optionsContractTradesPushDataJSON = `{"time": 1630576356,	"channel": "options.trades",	"event": "update",	"result": [    {        "contract": "BTC_USDT-20211231-59800-C",        "create_time": 1639144526,        "id": 12279,        "price": 997.8,        "size": -100,        "create_time_ms": 1639144526597,        "underlying": "BTC_USDT"    }	]}`

func TestOptionsContractTradesPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsContractTradesPushDataJSON)); err != nil {
		t.Errorf("%s websocket contract trades push data error: %v", e.Name, err)
	}
}

const optionsUnderlyingTradesPushDataJSON = `{"time": 1630576356,	"channel": "options.ul_trades",	"event": "update",	"result": [{"contract": "BTC_USDT-20211231-59800-C","create_time": 1639144526,"id": 12279,"price": 997.8,"size": -100,"create_time_ms": 1639144526597,"underlying": "BTC_USDT","is_call": true}	]}`

func TestOptionsUnderlyingTradesPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsUnderlyingTradesPushDataJSON)); err != nil {
		t.Errorf("%s websocket underlying trades push data error: %v", e.Name, err)
	}
}

const optionsUnderlyingPricePushDataJSON = `{	"time": 1630576356,	"channel": "options.ul_price",	"event": "update",	"result": {	   "underlying": "BTC_USDT",	   "price": 49653.24,"time": 1639143988,"time_ms": 1639143988931}}`

func TestOptionsUnderlyingPricePushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsUnderlyingPricePushDataJSON)); err != nil {
		t.Errorf("%s websocket underlying price push data error: %v", e.Name, err)
	}
}

const optionsMarkPricePushDataJSON = `{	"time": 1630576356,	"channel": "options.mark_price",	"event": "update",	"result": {    "contract": "BTC_USDT-20211231-59800-P",    "price": 11021.27,    "time": 1639143401,    "time_ms": 1639143401676}}`

func TestOptionsMarkPricePushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsMarkPricePushDataJSON)); err != nil {
		t.Errorf("%s websocket mark price push data error: %v", e.Name, err)
	}
}

const optionsSettlementsPushDataJSON = `{	"time": 1630576356,	"channel": "options.settlements",	"event": "update",	"result": {	   "contract": "BTC_USDT-20211130-55000-P",	   "orderbook_id": 2,	   "position_size": 1,	   "profit": 0.5,	   "settle_price": 70000,	   "strike_price": 65000,	   "tag": "WEEK",	   "trade_id": 1,	   "trade_size": 1,	   "underlying": "BTC_USDT",	   "time": 1639051907,	   "time_ms": 1639051907000}}`

func TestSettlementsPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsSettlementsPushDataJSON)); err != nil {
		t.Errorf("%s websocket options settlements push data error: %v", e.Name, err)
	}
}

const optionsContractPushDataJSON = `{"time": 1630576356,	"channel": "options.contracts",	"event": "update",	"result": {	   "contract": "BTC_USDT-20211130-50000-P",	   "create_time": 1637917026,	   "expiration_time": 1638230400,	   "init_margin_high": 0.15,	   "init_margin_low": 0.1,	   "is_call": false,	   "maint_margin_base": 0.075,	   "maker_fee_rate": 0.0004,	   "mark_price_round": 0.1,	   "min_balance_short": 0.5,	   "min_order_margin": 0.1,	   "multiplier": 0.0001,	   "order_price_deviate": 0,	   "order_price_round": 0.1,	   "order_size_max": 1,	   "order_size_min": 10,	   "orders_limit": 100000,	   "ref_discount_rate": 0.1,	   "ref_rebate_rate": 0,	   "strike_price": 50000,	   "tag": "WEEK",	   "taker_fee_rate": 0.0004,	   "underlying": "BTC_USDT",	   "time": 1639051907,	   "time_ms": 1639051907000}}`

func TestOptionsContractPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsContractPushDataJSON)); err != nil {
		t.Errorf("%s websocket options contracts push data error: %v", e.Name, err)
	}
}

const (
	optionsContractCandlesticksPushDataJSON   = `{	"time": 1630650451,	"channel": "options.contract_candlesticks",	"event": "update",	"result": [   {       "t": 1639039260,       "v": 100,       "c": "1041.4",       "h": "1041.4",       "l": "1041.4",       "o": "1041.4",       "a": "0",       "n": "10s_BTC_USDT-20211231-59800-C"   }	]}`
	optionsUnderlyingCandlesticksPushDataJSON = `{	"time": 1630650451,	"channel": "options.ul_candlesticks",	"event": "update",	"result": [    {        "t": 1639039260,        "v": 100,        "c": "1041.4",        "h": "1041.4",        "l": "1041.4",        "o": "1041.4",        "a": "0",        "n": "10s_BTC_USDT"    }	]}`
)

func TestOptionsCandlesticksPushData(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Test instance Setup must not error")
	require.NoError(t, ex.WsHandleOptionsData(t.Context(), nil, []byte(optionsContractCandlesticksPushDataJSON)))
	require.NoError(t, ex.WsHandleOptionsData(t.Context(), nil, []byte(optionsUnderlyingCandlesticksPushDataJSON)))

	for _, expPair := range []currency.Pair{
		currency.NewPairWithDelimiter("BTC", "USDT-20211231-59800-C", "_"),
		currency.NewPairWithDelimiter("BTC", "USDT", "_"),
	} {
		select {
		case msg := <-ex.Websocket.DataHandler.C:
			got, ok := msg.Data.([]kline.Item)
			require.True(t, ok, "expected []kline.Item")
			require.Len(t, got, 1)
			assert.Equal(t, []kline.Item{{
				Pair:     expPair,
				Asset:    asset.Options,
				Exchange: ex.Name,
				Interval: kline.TenSecond,
				Candles: []kline.Candle{{
					Time:   time.Unix(1639039260, 0),
					Open:   1041.4,
					Close:  1041.4,
					High:   1041.4,
					Low:    1041.4,
					Volume: 0,
				}},
			}}, got)
		default:
			require.Fail(t, "expected websocket options candle payload")
		}
	}
}

const (
	optionsOrderbookTickerPushDataJSON              = `{	"time": 1630650452,	"channel": "options.book_ticker",	"event": "update",	"result": {    "t": 1615366379123,    "u": 2517661076,    "s": "BTC_USDT-20211130-50000-C",    "b": "54696.6",    "B": 37000,    "a": "54696.7",    "A": 47061	}}`
	optionsOrderbookUpdatePushDataJSON              = `{	"time": 1630650445,	"channel": "options.order_book_update",	"event": "update",	"result": {    "t": 1615366381417,    "s": "%s",    "U": 2517661101,    "u": 2517661113,    "b": [        {            "p": "54672.1",            "s": 95        },        {            "p": "54664.5",            "s": 58794        }    ],    "a": [        {            "p": "54743.6",            "s": 95        },        {            "p": "54742",            "s": 95        }    ]	}}`
	optionsOrderbookSnapshotPushDataJSON            = `{	"time": 1630650445,	"channel": "options.order_book",	"event": "all",	"result": {    "t": 1541500161123,    "contract": "BTC_USDT-20211130-50000-C",    "id": 93973511,    "asks": [        {            "p": "97.1",            "s": 2245        },		{            "p": "97.2",            "s": 2245        }    ],    "bids": [		{            "p": "97.2",            "s": 2245        },        {            "p": "97.1",            "s": 2245        }    ]	}}`
	optionsOrderbookSnapshotUpdateEventPushDataJSON = `{"channel": "options.order_book",	"event": "update",	"time": 1630650445,	"result": [	  {		"p": "49525.6",		"s": 7726,		"c": "BTC_USDT-20211130-50000-C",		"id": 93973511	  }	]}`
)

func TestOptionsOrderbookPushData(t *testing.T) {
	t.Parallel()
	p := getPair(t, asset.Options)
	assert.NoError(t, e.WsHandleOptionsData(t.Context(), nil, []byte(optionsOrderbookTickerPushDataJSON)))
	assert.NoError(t, e.WsHandleOptionsData(t.Context(), nil, fmt.Appendf(nil, optionsOrderbookUpdatePushDataJSON, p.Upper().String())))
	assert.NoError(t, e.WsHandleOptionsData(t.Context(), nil, []byte(optionsOrderbookSnapshotPushDataJSON)))
	assert.NoError(t, e.WsHandleOptionsData(t.Context(), nil, []byte(optionsOrderbookSnapshotUpdateEventPushDataJSON)))
}

const optionsOrderPushDataJSON = `{"time": 1630654851,"channel": "options.orders",	"event": "update",	"result": [	   {		  "contract": "BTC_USDT-20211130-65000-C",		  "create_time": 1637897000,		  "fill_price": 0,		  "finish_as": "cancelled",		  "iceberg": 0,		  "id": 106,		  "is_close": false,		  "is_liq": false,		  "is_reduce_only": false,		  "left": -10,		  "mkfr": 0.0004,		  "price": 15000,		  "refr": 0,		  "refu": 0,		  "size": -10,		  "status": "finished",		  "text": "web",		  "tif": "gtc",		  "tkfr": 0.0004,		  "underlying": "BTC_USDT",		  "user": "9xxx",		  "time": 1639051907,"time_ms": 1639051907000}]}`

func TestOptionsOrderPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsOrderPushDataJSON)); err != nil {
		t.Errorf("%s websocket options orders push data error: %v", e.Name, err)
	}
}

const optionsUsersTradesPushDataJSON = `{	"time": 1639144214,	"channel": "options.usertrades",	"event": "update",	"result": [{"id": "1","underlying": "BTC_USDT","order": "557940","contract": "BTC_USDT-20211216-44800-C","create_time": 1639144214,"create_time_ms": 1639144214583,"price": "4999","role": "taker","size": -1}]}`

func TestOptionUserTradesPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsUsersTradesPushDataJSON)); err != nil {
		t.Errorf("%s websocket options orders push data error: %v", e.Name, err)
	}
}

const optionsLiquidatesPushDataJSON = `{	"channel": "options.liquidates",	"event": "update",	"time": 1630654851,	"result": [	   {		  "user": "1xxxx",		  "init_margin": 1190,		  "maint_margin": 1042.5,		  "order_margin": 0,		  "time": 1639051907,		  "time_ms": 1639051907000}	]}`

func TestOptionsLiquidatesPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsLiquidatesPushDataJSON)); err != nil {
		t.Errorf("%s websocket options liquidates push data error: %v", e.Name, err)
	}
}

const optionsSettlementPushDataJSON = `{	"channel": "options.user_settlements",	"event": "update",	"time": 1639051907,	"result": [{"contract": "BTC_USDT-20211130-65000-C","realised_pnl": -13.028,"settle_price": 70000,"settle_profit": 5,"size": 10,"strike_price": 65000,"underlying": "BTC_USDT","user": "9xxx","time": 1639051907,"time_ms": 1639051907000}]}`

func TestOptionsSettlementPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsSettlementPushDataJSON)); err != nil {
		t.Errorf("%s websocket options settlement push data error: %v", e.Name, err)
	}
}

const optionsPositionClosePushDataJSON = `{"channel": "options.position_closes",	"event": "update",	"time": 1630654851,	"result": [{"contract": "BTC_USDT-20211130-50000-C","pnl": -0.0056,"settle_size": 0,"side": "long","text": "web","underlying": "BTC_USDT","user": "11xxxxx","time": 1639051907,"time_ms": 1639051907000}]}`

func TestOptionsPositionClosePushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsPositionClosePushDataJSON)); err != nil {
		t.Errorf("%s websocket options position close push data error: %v", e.Name, err)
	}
}

const optionsBalancePushDataJSON = `{	"channel": "options.balances",	"event": "update",	"time": 1630654851,	"result": [	   {		  "balance": 60.79009,"change": -0.5,"text": "BTC_USDT-20211130-55000-P","type": "set","user": "11xxxx","time": 1639051907,"time_ms": 1639051907000}]}`

func TestOptionsBalancePushData(t *testing.T) {
	t.Parallel()
	ctx := accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "test", Secret: "test"})
	if err := e.WsHandleOptionsData(ctx, nil, []byte(optionsBalancePushDataJSON)); err != nil {
		t.Errorf("%s websocket options balance push data error: %v", e.Name, err)
	}
}

const optionsPositionPushDataJSON = `{"time": 1630654851,	"channel": "options.positions",	"event": "update",	"error": null,	"result": [	   {		  "entry_price": 0,		  "realised_pnl": -13.028,		  "size": 0,		  "contract": "BTC_USDT-20211130-65000-C",		  "user": "9010",		  "time": 1639051907,		  "time_ms": 1639051907000}	]}`

func TestOptionsPositionPushData(t *testing.T) {
	t.Parallel()
	if err := e.WsHandleOptionsData(t.Context(), nil, []byte(optionsPositionPushDataJSON)); err != nil {
		t.Errorf("%s websocket options position push data error: %v", e.Name, err)
	}
}

func TestOptionsPongPushData(t *testing.T) {
	t.Parallel()
	err := e.WsHandleOptionsData(t.Context(), nil, []byte(`{"time":1756700469,"channel":"options.pong","event":"","result":null}`))
	require.NoError(t, err)
}

func TestGenerateSubscriptionsSpot(t *testing.T) {
	t.Parallel()

	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")

	e.Websocket.SetCanUseAuthenticatedEndpoints(true)
	subs, err := e.generateSubscriptionsSpot()
	require.NoError(t, err, "generateSubscriptions must not error")
	exp := subscription.List{}
	assets := slices.DeleteFunc(e.GetAssetTypes(true), func(a asset.Item) bool { return !e.IsAssetWebsocketSupported(a) })
	for _, s := range e.Features.Subscriptions {
		for _, a := range assets {
			if s.Asset != asset.All && s.Asset != a {
				continue
			}
			pairs, err := e.GetEnabledPairs(a)
			require.NoErrorf(t, err, "GetEnabledPairs %s must not error", a)
			pairs = common.SortStrings(pairs).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"})
			s := s.Clone() //nolint:govet // Intentional lexical scope shadow
			s.Asset = a
			if singleSymbolChannel(channelName(s)) {
				for i := range pairs {
					s := s.Clone() //nolint:govet // Intentional lexical scope shadow
					switch s.Channel {
					case subscription.CandlesChannel:
						s.QualifiedChannel = "5m," + pairs[i].String()
					case subscription.OrderbookChannel:
						s.QualifiedChannel = pairs[i].String() + ",100ms"
					case spotOrderbookChannel:
						s.QualifiedChannel = pairs[i].String() + ",5,1000ms"
					case spotOrderbookV2:
						s.QualifiedChannel = fmt.Sprintf("ob.%s.%d", pairs[i].String(), s.Levels)
					}
					s.Pairs = pairs[i : i+1]
					exp = append(exp, s)
				}
			} else {
				s.Pairs = pairs
				s.QualifiedChannel = pairs.Join()
				exp = append(exp, s)
			}
		}
	}
	testsubs.EqualLists(t, exp, subs)
}

func TestSubscribe(t *testing.T) {
	t.Parallel()
	subs, err := e.Features.Subscriptions.ExpandTemplates(e)
	require.NoError(t, err, "ExpandTemplates must not error")
	e.Features.Subscriptions = subscription.List{}
	err = e.Subscribe(t.Context(), &FixtureConnection{}, subs)
	require.NoError(t, err, "Subscribe must not error")
}

func TestGenerateDeliveryFuturesDefaultSubscriptions(t *testing.T) {
	t.Parallel()
	if _, err := e.GenerateDeliveryFuturesDefaultSubscriptions(); err != nil {
		t.Error(err)
	}
}

func TestGenerateFuturesDefaultSubscriptions(t *testing.T) {
	t.Parallel()
	e := new(Exchange)
	require.NoError(t, testexch.Setup(e), "Test instance Setup must not error")
	subs, err := e.GenerateFuturesDefaultSubscriptions(asset.USDTMarginedFutures)
	require.NoError(t, err)
	require.NotEmpty(t, subs)
	subs, err = e.GenerateFuturesDefaultSubscriptions(asset.CoinMarginedFutures)
	require.NoError(t, err)
	require.NotEmpty(t, subs)
	require.NoError(t, e.CurrencyPairs.SetAssetEnabled(asset.USDTMarginedFutures, false), "SetAssetEnabled must not error")
	subs, err = e.GenerateFuturesDefaultSubscriptions(asset.USDTMarginedFutures)
	require.NoError(t, err, "Disabled asset must not error")
	require.Empty(t, subs, "Disabled asset must return no pairs")
}

func TestGenerateOptionsDefaultSubscriptions(t *testing.T) {
	t.Parallel()
	if _, err := e.GenerateOptionsDefaultSubscriptions(); err != nil {
		t.Error(err)
	}
}

func TestCreateAPIKeysOfSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if _, err := e.CreateAPIKeysOfSubAccount(t.Context(), CreateAPIKeySubAccountParams{
		SubAccountUserID: 12345,
		Body: &SubAccountKey{
			APIKeyName: "12312mnfsndfsfjsdklfjsdlkfj",
			Permissions: []APIV4KeyPerm{
				{
					PermissionName: "wallet",
					ReadOnly:       false,
				},
				{
					PermissionName: "spot",
					ReadOnly:       false,
				},
				{
					PermissionName: "futures",
					ReadOnly:       false,
				},
				{
					PermissionName: "delivery",
					ReadOnly:       false,
				},
				{
					PermissionName: "earn",
					ReadOnly:       false,
				},
				{
					PermissionName: "options",
					ReadOnly:       false,
				},
			},
		},
	}); err != nil {
		t.Error(err)
	}
}

func TestListAllAPIKeyOfSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAllAPIKeyOfSubAccount(t.Context(), 1234)
	if err != nil {
		t.Error(err)
	}
}

func TestUpdateAPIKeyOfSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	if err := e.UpdateAPIKeyOfSubAccount(t.Context(), apiCredentials.Key, CreateAPIKeySubAccountParams{
		SubAccountUserID: 12345,
		Body: &SubAccountKey{
			APIKeyName: "12312mnfsndfsfjsdklfjsdlkfj",
			Permissions: []APIV4KeyPerm{
				{
					PermissionName: "wallet",
					ReadOnly:       false,
				},
				{
					PermissionName: "spot",
					ReadOnly:       false,
				},
				{
					PermissionName: "futures",
					ReadOnly:       false,
				},
				{
					PermissionName: "delivery",
					ReadOnly:       false,
				},
				{
					PermissionName: "earn",
					ReadOnly:       false,
				},
				{
					PermissionName: "options",
					ReadOnly:       false,
				},
			},
		},
	}); err != nil {
		t.Error(err)
	}
}

func TestGetAPIKeyOfSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	_, err := e.GetAPIKeyOfSubAccount(t.Context(), 1234, "target_api_key")
	if err != nil {
		t.Error(err)
	}
}

func TestLockSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if err := e.LockSubAccount(t.Context(), 1234); err != nil {
		t.Error(err)
	}
}

func TestUnlockSubAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	if err := e.UnlockSubAccount(t.Context(), 1234); err != nil {
		t.Error(err)
	}
}

func TestUpdateOrderExecutionLimits(t *testing.T) {
	t.Parallel()
	t.Run("product borrow minimums", testUpdateOrderExecutionLimitsUsesProductBorrowMinimums)

	responses := map[string]string{
		gateIOTestSpotCurrencyPairsPath:     `[{"id":"BTC_USDT","base":"BTC","quote":"USDT","min_base_amount":"0","min_quote_amount":"1","max_base_amount":"5","max_quote_amount":"6","amount_precision":3,"precision":2,"trade_status":"tradable","sell_start":1700000000,"buy_start":1700000100,"delisting_time":1800000000,"up_rate":"0.2","down_rate":"0.1","market_order_max_stock":"7"},{"id":"ETH_USDT","base":"ETH","quote":"USDT","min_base_amount":"1","min_quote_amount":"1","amount_precision":3,"precision":2,"trade_status":"tradable"},{"id":"UNTRADEABLE_USDT","base":"UNTRADEABLE","quote":"USDT","trade_status":"untradable"}]`,
		"/api/v4/margin/uni/currency_pairs": `[{"currency_pair":"BTC_USDT","base_min_borrow_amount":"0.01","quote_min_borrow_amount":"2","status":"enabled","delisted_time":0},{"currency_pair":"DOGE_USDT","base_min_borrow_amount":"1","quote_min_borrow_amount":"2","status":"enabled","delisted_time":0},{"currency_pair":"ETH_USDT","base_min_borrow_amount":"1","quote_min_borrow_amount":"2","status":"disabled","delisted_time":0}]`,
		gateIOTestCrossMarginCurrenciesPath: `[{"name":"BTC","min_borrow_amount":"0.03","loanable":true,"status":1},{"name":"USDT","min_borrow_amount":"4","loanable":true,"status":1}]`,
		"/api/v4/futures/btc/contracts":     `[{"name":"MBABYDOGE_USD","order_size_min":"1","order_size_max":"2","order_price_round":"0.1","quanto_multiplier":"1"},{"name":"BTC_USD","order_size_min":"1","order_size_max":"2","order_price_round":"0.1","quanto_multiplier":"1","launch_time":1700000000,"delisting_time":1750000000,"delisted_time":1800000000}]`,
		"/api/v4/futures/usdt/contracts":    `[{"name":"MBABYDOGE_USDT","order_size_min":"1","order_size_max":"2","order_price_round":"0.1","quanto_multiplier":"1"},{"name":"BTC_USDT","order_size_min":"1","order_size_max":"2","order_price_round":"0.1","quanto_multiplier":"1","launch_time":1700000000,"delisting_time":1750000000,"delisted_time":1800000000}]`,
		"/api/v4/delivery/btc/contracts":    `[{"name":"MBABYDOGE_USD_20260925","order_size_min":1,"order_size_max":2,"order_price_round":"0.1","quanto_multiplier":"1"},{"name":"BTC_USD_20260925","order_size_min":1,"order_size_max":2,"order_price_round":"0.1","quanto_multiplier":"1","expire_time":1800000000}]`,
		gateIOTestDeliveryUSDTContractsPath: `[{"name":"MBABYDOGE_USDT_20260925","order_size_min":1,"order_size_max":2,"order_price_round":"0.1","quanto_multiplier":"1"},{"name":"BTC_USDT_20260925","order_size_min":1,"order_size_max":2,"order_price_round":"0.1","quanto_multiplier":"1","expire_time":1800000000}]`,
		gateIOTestOptionsUnderlyingsPath:    gateIOTestOptionsUnderlyingResponse,
		gateIOTestOptionsContractsPath:      `[{"name":"BTC_USDT-20260925-50000-C","order_size_min":"1","order_size_max":"2","order_price_round":"0.1","multiplier":"1"}]`,
	}
	ex := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing execution-limits response should not error")
	}))
	for _, tc := range []struct {
		asset                    asset.Item
		pair                     currency.Pair
		scaledPair               currency.Pair
		minimumBaseAmount        float64
		maximumBaseAmount        float64
		minimumQuoteAmount       float64
		maximumQuoteAmount       float64
		priceStepIncrementSize   float64
		amountStepIncrementSize  float64
		multiplierDecimal        float64
		priceDivisor             float64
		minimumBorrowAmountBase  float64
		minimumBorrowAmountQuote float64
		multiplierUp             float64
		multiplierDown           float64
		marketMaxQty             float64
		listedUnix               int64
		delistingUnix            int64
		delistedUnix             int64
		expiryUnix               int64
	}{
		{asset: asset.Spot, pair: currency.NewBTCUSDT(), minimumBaseAmount: 0.001, maximumBaseAmount: 5, minimumQuoteAmount: 1, maximumQuoteAmount: 6, priceStepIncrementSize: 0.01, amountStepIncrementSize: 0.001, multiplierUp: 0.2, multiplierDown: 0.1, marketMaxQty: 7, listedUnix: 1_700_000_000, delistedUnix: 1_800_000_000},
		{asset: asset.Margin, pair: currency.NewBTCUSDT(), minimumBaseAmount: 0.001, minimumQuoteAmount: 1, priceStepIncrementSize: 0.01, amountStepIncrementSize: 0.001, minimumBorrowAmountBase: 0.01, minimumBorrowAmountQuote: 2, delistedUnix: 1_800_000_000},
		{asset: asset.CrossMargin, pair: currency.NewBTCUSDT(), minimumBaseAmount: 0.001, minimumQuoteAmount: 1, priceStepIncrementSize: 0.01, amountStepIncrementSize: 0.001, minimumBorrowAmountBase: 0.03, minimumBorrowAmountQuote: 4, delistedUnix: 1_800_000_000},
		{asset: asset.CoinMarginedFutures, pair: currency.NewPairWithDelimiter("BTC", "USD", currency.UnderscoreDelimiter), scaledPair: currency.NewPairWithDelimiter(divisorCurrency.String(), "USD", currency.UnderscoreDelimiter), minimumBaseAmount: 1, maximumBaseAmount: 2, priceStepIncrementSize: 0.1, amountStepIncrementSize: 1, multiplierDecimal: 1, priceDivisor: 1, listedUnix: 1_700_000_000, delistingUnix: 1_750_000_000, delistedUnix: 1_800_000_000},
		{asset: asset.USDTMarginedFutures, pair: currency.NewBTCUSDT(), scaledPair: currency.NewPair(divisorCurrency, currency.USDT), minimumBaseAmount: 1, maximumBaseAmount: 2, priceStepIncrementSize: 0.1, amountStepIncrementSize: 1, multiplierDecimal: 1, priceDivisor: 1, listedUnix: 1_700_000_000, delistingUnix: 1_750_000_000, delistedUnix: 1_800_000_000},
		{asset: asset.DeliveryFutures, pair: currency.NewPairWithDelimiter("BTC", "USDT_20260925", currency.UnderscoreDelimiter), scaledPair: currency.NewPairWithDelimiter(divisorCurrency.String(), "USDT_20260925", currency.UnderscoreDelimiter), minimumBaseAmount: 1, maximumBaseAmount: 2, priceStepIncrementSize: 0.1, amountStepIncrementSize: 1, multiplierDecimal: 1, priceDivisor: 1, expiryUnix: 1_800_000_000},
		{asset: asset.Options, pair: currency.NewPairWithDelimiter("BTC", "USDT-20260925-50000-C", currency.UnderscoreDelimiter), minimumBaseAmount: 1, maximumBaseAmount: 2, priceStepIncrementSize: 0.1, amountStepIncrementSize: 1, multiplierDecimal: 1, priceDivisor: 1},
	} {
		require.NoErrorf(t, ex.UpdateOrderExecutionLimits(t.Context(), tc.asset), "UpdateOrderExecutionLimits must not error for %s", tc.asset)
		level, err := ex.GetOrderExecutionLimits(tc.asset, tc.pair)
		require.NoErrorf(t, err, "GetOrderExecutionLimits must return %s for %s", tc.pair, tc.asset)
		assert.InDeltaf(t, tc.minimumBaseAmount, level.MinimumBaseAmount, 1e-12, "minimum base amount should match for %s", tc.asset)
		assert.InDeltaf(t, tc.maximumBaseAmount, level.MaximumBaseAmount, 1e-12, "maximum base amount should match for %s", tc.asset)
		assert.InDeltaf(t, tc.minimumQuoteAmount, level.MinimumQuoteAmount, 1e-12, "minimum quote amount should match for %s", tc.asset)
		assert.InDeltaf(t, tc.maximumQuoteAmount, level.MaximumQuoteAmount, 1e-12, "maximum quote amount should match for %s", tc.asset)
		assert.InDeltaf(t, tc.priceStepIncrementSize, level.PriceStepIncrementSize, 1e-12, "price step increment should match for %s", tc.asset)
		assert.InDeltaf(t, tc.amountStepIncrementSize, level.AmountStepIncrementSize, 1e-12, "amount step increment should match for %s", tc.asset)
		assert.InDeltaf(t, tc.multiplierDecimal, level.MultiplierDecimal, 1e-12, "multiplier should match for %s", tc.asset)
		assert.InDeltaf(t, tc.priceDivisor, level.PriceDivisor, 1e-12, "price divisor should match for %s", tc.asset)
		assert.InDeltaf(t, tc.minimumBorrowAmountBase, level.MinimumBorrowAmountBase, 1e-12, "minimum base borrow amount should match for %s", tc.asset)
		assert.InDeltaf(t, tc.minimumBorrowAmountQuote, level.MinimumBorrowAmountQuote, 1e-12, "minimum quote borrow amount should match for %s", tc.asset)
		assert.InDeltaf(t, tc.multiplierUp, level.MultiplierUp, 1e-12, "upward multiplier should match for %s", tc.asset)
		assert.InDeltaf(t, tc.multiplierDown, level.MultiplierDown, 1e-12, "downward multiplier should match for %s", tc.asset)
		assert.InDeltaf(t, tc.marketMaxQty, level.MarketMaxQty, 1e-12, "market maximum quantity should match for %s", tc.asset)
		if tc.listedUnix == 0 {
			assert.Truef(t, level.Listed.IsZero(), "listed time should be zero for %s", tc.asset)
		} else {
			assert.Equalf(t, tc.listedUnix, level.Listed.Unix(), "listed time should match for %s", tc.asset)
		}
		if tc.delistingUnix == 0 {
			assert.Truef(t, level.Delisting.IsZero(), "delisting time should be zero for %s", tc.asset)
		} else {
			assert.Equalf(t, tc.delistingUnix, level.Delisting.Unix(), "delisting time should match for %s", tc.asset)
		}
		if tc.delistedUnix == 0 {
			assert.Truef(t, level.Delisted.IsZero(), "delisted time should be zero for %s", tc.asset)
		} else {
			assert.Equalf(t, tc.delistedUnix, level.Delisted.Unix(), "delisted time should match for %s", tc.asset)
		}
		if tc.expiryUnix == 0 {
			assert.Truef(t, level.Expiry.IsZero(), "expiry time should be zero for %s", tc.asset)
		} else {
			assert.Equalf(t, tc.expiryUnix, level.Expiry.Unix(), "expiry time should match for %s", tc.asset)
		}
		if tc.asset.IsFutures() {
			scaled, err := ex.GetOrderExecutionLimits(tc.asset, tc.scaledPair)
			require.NoErrorf(t, err, "GetOrderExecutionLimits must return the scaled contract for %s", tc.asset)
			assert.Equalf(t, 1e6, scaled.PriceDivisor, "scaled contract price divisor should match for %s", tc.asset)
		}
	}

	require.ErrorIs(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.Empty), asset.ErrNotSupported, "UpdateOrderExecutionLimits must reject an unsupported asset")
	ex.CurrencyPairs.Pairs[asset.Futures] = new(currency.PairStore)
	require.ErrorIs(t, ex.UpdateOrderExecutionLimits(t.Context(), asset.Futures), asset.ErrNotSupported, "UpdateOrderExecutionLimits must reject an unsupported mapped asset")

	failing := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := responses[r.URL.Path]; !ok {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, `{`)
		assert.NoError(t, err, "writing invalid response should not error")
	}))
	for _, a := range []asset.Item{asset.Spot, asset.Margin, asset.CrossMargin, asset.CoinMarginedFutures, asset.USDTMarginedFutures, asset.DeliveryFutures, asset.Options} {
		require.Errorf(t, failing.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must return endpoint errors for %s", a)
	}

	for _, a := range []asset.Item{asset.Margin, asset.CrossMargin} {
		secondaryFailure := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var response string
			switch r.URL.Path {
			case "/api/v4/margin/uni/currency_pairs":
				response = `[{"currency_pair":"BTC_USDT","status":"enabled","delisted_time":0}]`
			case gateIOTestCrossMarginCurrenciesPath:
				response = `[{"name":"BTC","min_borrow_amount":"1","loanable":true,"status":1}]`
			case gateIOTestSpotCurrencyPairsPath:
				response = `{`
			default:
				http.NotFound(w, r)
				return
			}
			_, err := fmt.Fprint(w, response)
			assert.NoError(t, err, "writing secondary endpoint response should not error")
		}))
		require.Errorf(t, secondaryFailure.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must return the secondary endpoint error for %s", a)
	}

	deliveryParseFailure := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/delivery/btc/contracts" {
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, `[{"name":""}]`)
		assert.NoError(t, err, "writing delivery response should not error")
	}))
	require.Error(t, deliveryParseFailure.UpdateOrderExecutionLimits(t.Context(), asset.DeliveryFutures), "UpdateOrderExecutionLimits must return delivery pair parsing errors")

	optionsContractFailure := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response string
		switch r.URL.Path {
		case gateIOTestOptionsUnderlyingsPath:
			response = gateIOTestOptionsUnderlyingResponse
		case gateIOTestOptionsContractsPath:
			response = `{`
		default:
			http.NotFound(w, r)
			return
		}
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing options response should not error")
	}))
	require.Error(t, optionsContractFailure.UpdateOrderExecutionLimits(t.Context(), asset.Options), "UpdateOrderExecutionLimits must return options contract endpoint errors")

	for _, contract := range []string{"", "MBABYDOGE_USDT"} {
		optionsContractFailure := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var response string
			switch r.URL.Path {
			case gateIOTestOptionsUnderlyingsPath:
				response = gateIOTestOptionsUnderlyingResponse
			case gateIOTestOptionsContractsPath:
				response = fmt.Sprintf(`[{"name":%q,"order_size_min":"1","order_size_max":"2","order_price_round":"0.1","multiplier":"1"}]`, contract)
			default:
				http.NotFound(w, r)
				return
			}
			_, err := fmt.Fprint(w, response)
			assert.NoError(t, err, "writing options response should not error")
		}))
		require.Errorf(t, optionsContractFailure.UpdateOrderExecutionLimits(t.Context(), asset.Options), "UpdateOrderExecutionLimits must return options contract conversion errors for %q", contract)
	}

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		testexch.UpdatePairsOnce(t, e)
		for _, a := range e.GetAssetTypes(false) {
			t.Run(a.String(), func(t *testing.T) {
				t.Parallel()
				require.NoError(t, e.UpdateOrderExecutionLimits(t.Context(), a), "UpdateOrderExecutionLimits must not error")
				pairs, err := e.GetAvailablePairs(a)
				require.NoError(t, err, "GetAvailablePairs must not error")
				for _, p := range pairs {
					l, err := e.GetOrderExecutionLimits(a, p)
					require.NoErrorf(t, err, "GetOrderExecutionLimits must not error for %s", p)
					require.NotNilf(t, l, "GetOrderExecutionLimits result must not be nil for %s", p)
					assert.Equalf(t, a, l.Key.Asset, "asset should equal for %s", p)
					assert.Truef(t, p.Equal(l.Key.Pair()), "pair should equal for %s", p)
					switch a {
					case asset.Options:
						assert.Positivef(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive for %s", p)
						assert.Positivef(t, l.MaximumBaseAmount, "MaximumBaseAmount should be positive for %s", p)
						assert.Positivef(t, l.PriceStepIncrementSize, "PriceStepIncrementSize should be positive for %s", p)
						assert.Positivef(t, l.AmountStepIncrementSize, "AmountStepIncrementSize should be positive for %s", p)
						assert.Positivef(t, l.MultiplierDecimal, "MultiplierDecimal should be positive for %s", p)
					case asset.USDTMarginedFutures:
						assert.Positivef(t, l.MultiplierDecimal, "MultiplierDecimal should be positive for %s", p)
						assert.NotZerof(t, l.Listed, "Listed should be populated for %s", p)
						fallthrough
					case asset.CoinMarginedFutures:
						if !l.Delisted.IsZero() {
							assert.Truef(t, l.Delisted.After(l.Delisting), "Delisted should be after Delisting for %s", p)
						}
						assert.Positivef(t, l.AmountStepIncrementSize, "AmountStepIncrementSize should be positive for %s", p)
					case asset.Spot:
						assert.Positivef(t, l.MinimumQuoteAmount, "MinimumQuoteAmount should be positive for %s", p)
						if l.QuoteStepIncrementSize != 0 {
							assert.Positivef(t, l.QuoteStepIncrementSize, "QuoteStepIncrementSize should be positive for %s when set", p)
						}
						assert.Positivef(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive for %s", p)
						assert.Positivef(t, l.AmountStepIncrementSize, "AmountStepIncrementSize should be positive for %s", p)
					case asset.Margin, asset.CrossMargin:
						assert.Positivef(t, l.MinimumQuoteAmount, "MinimumQuoteAmount should be positive for %s", p)
						assert.Positivef(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive for %s", p)
						assert.Positivef(t, l.PriceStepIncrementSize, "PriceStepIncrementSize should be positive for %s", p)
						invalidPrice := l.PriceStepIncrementSize / 2
						err = l.Validate(invalidPrice, l.MinimumBaseAmount, order.Limit)
						assert.ErrorIsf(t, err, limits.ErrPriceExceedsStep, "Validate should reject an invalid price tick for %s", p)
						if l.QuoteStepIncrementSize != 0 {
							assert.Positivef(t, l.QuoteStepIncrementSize, "QuoteStepIncrementSize should be positive for %s when set", p)
						}
						if l.AmountStepIncrementSize != 0 {
							assert.Positivef(t, l.AmountStepIncrementSize, "AmountStepIncrementSize should be positive for %s when set", p)
						}
						assert.Positivef(t, l.MinimumBorrowAmountBase, "MinimumBorrowAmountBase should be positive for %s", p)
						assert.Positivef(t, l.MinimumBorrowAmountQuote, "MinimumBorrowAmountQuote should be positive for %s", p)
					case asset.DeliveryFutures:
						assert.NotZerof(t, l.Expiry, "Expiry should be populated for %s", p)
						assert.Positivef(t, l.MinimumBaseAmount, "MinimumBaseAmount should be positive for %s", p)
						assert.Positivef(t, l.AmountStepIncrementSize, "AmountStepIncrementSize should be positive for %s", p)
						assert.Positivef(t, l.MultiplierDecimal, "MultiplierDecimal should be positive for %s", p)
					}
				}
			})
		}
	})
}

func TestGetFuturesContractDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetFuturesContractDetails(t.Context(), asset.Spot)
	require.ErrorIs(t, err, futures.ErrNotFuturesAsset)

	_, err = e.GetFuturesContractDetails(t.Context(), asset.PerpetualContract)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	exp, err := e.GetAllDeliveryContracts(t.Context(), currency.USDT)
	require.NoError(t, err, "GetAllDeliveryContracts must not error")
	c, err := e.GetFuturesContractDetails(t.Context(), asset.DeliveryFutures)
	require.NoError(t, err, "GetFuturesContractDetails must not error for DeliveryFutures")
	assert.Equal(t, len(exp), len(c), "GetFuturesContractDetails should return same number of Delivery contracts as exist")

	for _, a := range []asset.Item{asset.CoinMarginedFutures, asset.USDTMarginedFutures} {
		c, err = e.GetFuturesContractDetails(t.Context(), a)
		require.NoErrorf(t, err, "GetFuturesContractDetails must not error for %s", a)
		assert.NotEmptyf(t, c, "GetFuturesContractDetails should return some contracts for %s", a)
	}
}

func TestGetLatestFundingRates(t *testing.T) {
	t.Parallel()
	_, err := e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 currency.NewBTCUSDT(),
		IncludePredictedRate: true,
	})
	assert.NoError(t, err)

	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{
		Asset: asset.CoinMarginedFutures,
		Pair:  currency.NewBTCUSD(),
	})
	assert.NoError(t, err)

	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.CoinMarginedFutures})
	assert.NoError(t, err)
	_, err = e.GetLatestFundingRates(t.Context(), &fundingrate.LatestRateRequest{Asset: asset.USDTMarginedFutures})
	assert.NoError(t, err)
}

func TestGetHistoricalFundingRates(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricalFundingRates(t.Context(), nil)
	assert.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{})
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{Asset: asset.CoinMarginedFutures})
	assert.ErrorIs(t, err, currency.ErrCurrencyPairEmpty)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{Asset: asset.Futures})
	assert.ErrorIs(t, err, asset.ErrNotSupported)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset: asset.USDTMarginedFutures,
		Pair:  currency.NewPair(currency.ENJ, currency.USDT),
	})
	assert.ErrorIs(t, err, fundingrate.ErrPaymentCurrencyCannotBeEmpty)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.USDTMarginedFutures,
		Pair:            currency.NewPair(currency.ENJ, currency.USDT),
		PaymentCurrency: currency.USDT,
		IncludePayments: true,
	})
	assert.ErrorIs(t, err, common.ErrNotYetImplemented)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:                asset.USDTMarginedFutures,
		Pair:                 currency.NewPair(currency.ENJ, currency.USDT),
		PaymentCurrency:      currency.USDT,
		IncludePredictedRate: true,
	})
	assert.ErrorIs(t, err, common.ErrNotYetImplemented)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.USDTMarginedFutures,
		Pair:            currency.NewPair(currency.ENJ, currency.USDT),
		PaymentCurrency: currency.USDT,
		StartDate:       time.Now().Add(time.Hour * 16),
		EndDate:         time.Now(),
	})
	assert.ErrorIs(t, err, common.ErrStartAfterEnd)

	_, err = e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.USDTMarginedFutures,
		Pair:            currency.NewPair(currency.ENJ, currency.USDT),
		PaymentCurrency: currency.USDT,
		StartDate:       time.Now().Add(-time.Hour * 8008),
		EndDate:         time.Now(),
	})
	assert.ErrorIs(t, err, fundingrate.ErrFundingRateOutsideLimits)

	history, err := e.GetHistoricalFundingRates(t.Context(), &fundingrate.HistoricalRatesRequest{
		Asset:           asset.USDTMarginedFutures,
		Pair:            currency.NewPair(currency.ENJ, currency.USDT),
		PaymentCurrency: currency.USDT,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, history)
}

func TestGetOpenInterest(t *testing.T) {
	t.Parallel()
	coinPair := currency.NewPairWithDelimiter("BTC", "USD", currency.UnderscoreDelimiter)
	usdtPair := currency.NewPairWithDelimiter("BTC", "USDT", currency.UnderscoreDelimiter)
	deliveryPair := currency.NewPairWithDelimiter("BTC", "USDT_20260925", currency.UnderscoreDelimiter)
	fixtures := []struct {
		name         string
		asset        asset.Item
		pair         currency.Pair
		contractPath string
		allPath      string
		statsPath    string
		singleWant   float64
	}{
		{name: gateIOTestCoinMarginedName, asset: asset.CoinMarginedFutures, pair: coinPair, contractPath: "/api/v4/futures/btc/contracts/BTC_USD", allPath: "/api/v4/futures/btc/contracts", statsPath: "/api/v4/futures/btc/contract_stats", singleWant: 3},
		{name: gateIOTestUSDTMarginedName, asset: asset.USDTMarginedFutures, pair: usdtPair, contractPath: "/api/v4/futures/usdt/contracts/BTC_USDT", allPath: "/api/v4/futures/usdt/contracts", statsPath: "/api/v4/futures/usdt/contract_stats", singleWant: 3},
		{name: "delivery", asset: asset.DeliveryFutures, pair: deliveryPair, contractPath: "/api/v4/delivery/usdt/contracts/BTC_USDT_20260925", allPath: gateIOTestDeliveryUSDTContractsPath, singleWant: 20},
	}
	responses := map[string]string{
		"/api/v4/futures/btc/contracts":                     `[{"name":"BTC_USD","quanto_multiplier":"1","index_price":"10","position_size":2}]`,
		"/api/v4/futures/btc/contracts/BTC_USD":             `{"name":"BTC_USD","quanto_multiplier":"1","index_price":"10","position_size":2}`,
		"/api/v4/futures/btc/contract_stats":                `[{"time":1700000000,"open_interest":"3"}]`,
		"/api/v4/futures/usdt/contracts":                    `[{"name":"BTC_USDT","quanto_multiplier":"1","index_price":"10","position_size":2}]`,
		"/api/v4/futures/usdt/contracts/BTC_USDT":           `{"name":"BTC_USDT","quanto_multiplier":"1","index_price":"10","position_size":2}`,
		"/api/v4/futures/usdt/contract_stats":               `[{"time":1700000000,"open_interest":"3"}]`,
		gateIOTestDeliveryUSDTContractsPath:                 `[{"name":"BTC_USDT_20260925","quanto_multiplier":"1","index_price":"10","position_size":2}]`,
		"/api/v4/delivery/usdt/contracts/BTC_USDT_20260925": `{"name":"BTC_USDT_20260925","quanto_multiplier":"1","index_price":"10","position_size":2}`,
	}
	requests := make(chan gateIOHTTPRequest, 32)
	t.Cleanup(func() {
		assert.Empty(t, requests, "all open-interest requests should be consumed")
	})
	ex := setupGateIOHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		response, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		requests <- gateIOHTTPRequest{method: r.Method, path: r.URL.Path, query: r.URL.Query()}
		_, err := fmt.Fprint(w, response)
		assert.NoError(t, err, "writing an open-interest response should not error")
	}))
	assertRequest := func(path string, query url.Values) {
		t.Helper()
		gotRequest := requireGateIOHTTPRequest(t, requests)
		assert.Equal(t, http.MethodGet, gotRequest.method, "request method should match")
		assert.Equal(t, path, gotRequest.path, "request path should match")
		assert.Equal(t, query, gotRequest.query, "request query should match")
	}
	for _, tc := range fixtures {
		require.NoErrorf(t, ex.CurrencyPairs.StorePairs(tc.asset, currency.Pairs{tc.pair}, false), "StorePairs must not error for available %s fixture pairs", tc.asset)
		require.NoErrorf(t, ex.CurrencyPairs.StorePairs(tc.asset, currency.Pairs{tc.pair}, true), "StorePairs must not error for enabled %s fixture pairs", tc.asset)
	}

	_, err := ex.GetOpenInterest(t.Context(), key.PairAsset{
		Base:  currency.NewCode("GOLDFISH").Item,
		Quote: currency.USDT.Item,
		Asset: asset.USDTMarginedFutures,
	})
	assert.ErrorIs(t, err, currency.ErrPairNotFound, "unavailable pair should return the expected error")
	assert.Empty(t, requests, "an unavailable pair should not send an HTTP request")

	var resp []futures.OpenInterest
	for _, tc := range fixtures {
		resp, err = ex.GetOpenInterest(t.Context(), key.PairAsset{
			Base:  tc.pair.Base.Item,
			Quote: tc.pair.Quote.Item,
			Asset: tc.asset,
		})
		require.NoErrorf(t, err, "GetOpenInterest must not error for %s", tc.name)
		require.Lenf(t, resp, 1, "GetOpenInterest must return one item for %s", tc.name)
		assert.Equal(t, tc.asset, resp[0].Key.Asset, "asset should match")
		assert.True(t, tc.pair.Equal(resp[0].Key.Pair()), "pair should match")
		assert.Equal(t, tc.singleWant, resp[0].OpenInterest, "open interest should match")
		assertRequest(tc.contractPath, url.Values{})
		if tc.statsPath != "" {
			assertRequest(tc.statsPath, url.Values{
				gateIOTestContractQueryKey: {tc.pair.String()},
				gateIOTestLimitQueryKey:    {"1"},
			})
		}
	}

	resp, err = ex.GetOpenInterest(
		t.Context(),
		key.PairAsset{Base: coinPair.Base.Item, Quote: coinPair.Quote.Item, Asset: asset.CoinMarginedFutures},
		key.PairAsset{Base: usdtPair.Base.Item, Quote: usdtPair.Quote.Item, Asset: asset.USDTMarginedFutures},
	)
	require.NoError(t, err, "GetOpenInterest must not error for multiple explicit perpetual pairs")
	require.Len(t, resp, 2, "GetOpenInterest must return exactly the requested perpetual pairs")
	for i, tc := range fixtures[:2] {
		assert.Equal(t, tc.asset, resp[i].Key.Asset, "asset should match")
		assert.True(t, tc.pair.Equal(resp[i].Key.Pair()), "pair should match")
		assert.Equal(t, 20.0, resp[i].OpenInterest, "contract-derived open interest should match")
		assertRequest(tc.allPath, url.Values{})
	}

	resp, err = ex.GetOpenInterest(t.Context())
	require.NoError(t, err, "GetOpenInterest must not error without explicit pairs")
	require.Len(t, resp, len(fixtures), "GetOpenInterest must return every enabled fixture pair")
	for i, tc := range []struct {
		asset asset.Item
		pair  currency.Pair
		path  string
	}{
		{asset: asset.DeliveryFutures, pair: deliveryPair, path: gateIOTestDeliveryUSDTContractsPath},
		{asset: asset.CoinMarginedFutures, pair: coinPair, path: "/api/v4/futures/btc/contracts"},
		{asset: asset.USDTMarginedFutures, pair: usdtPair, path: "/api/v4/futures/usdt/contracts"},
	} {
		assert.Equal(t, tc.asset, resp[i].Key.Asset, "asset should match")
		assert.True(t, tc.pair.Equal(resp[i].Key.Pair()), "pair should match")
		assert.Equal(t, 20.0, resp[i].OpenInterest, "contract-derived open interest should match")
		assertRequest(tc.path, url.Values{})
	}

	t.Run("live", func(t *testing.T) {
		t.Parallel()
		skipGateIOLiveTest(t, false)
		for _, tc := range []struct {
			name  string
			asset asset.Item
		}{
			{name: gateIOTestCoinMarginedName, asset: asset.CoinMarginedFutures},
			{name: gateIOTestUSDTMarginedName, asset: asset.USDTMarginedFutures},
			{name: "delivery", asset: asset.DeliveryFutures},
		} {
			pair := getPair(t, tc.asset)
			got, err := e.GetOpenInterest(t.Context(), key.PairAsset{Base: pair.Base.Item, Quote: pair.Quote.Item, Asset: tc.asset})
			require.NoErrorf(t, err, "GetOpenInterest must not error for %s against the live API", tc.name)
			require.NotEmptyf(t, got, "GetOpenInterest must return data for %s from the live API", tc.name)
		}
	})
}

func TestGetClientOrderIDFromText(t *testing.T) {
	t.Parallel()
	assert.Empty(t, getClientOrderIDFromText("api"), "should not return anything")
	assert.Equal(t, "t-123", getClientOrderIDFromText("t-123"), "should return t-123")
}

func TestFormatClientOrderID(t *testing.T) {
	t.Parallel()
	assert.Empty(t, formatClientOrderID(""), "should not return anything")
	assert.Equal(t, "t-123", formatClientOrderID("t-123"), "should return t-123")
	assert.Equal(t, "t-456", formatClientOrderID("456"), "should return t-456")
}

func TestGetSideAndAmountFromSize(t *testing.T) {
	t.Parallel()
	side, amount, remaining := getSideAndAmountFromSize(1, 1)
	assert.Equal(t, order.Long, side, "should be a buy order")
	assert.Equal(t, 1.0, amount, "should be 1.0")
	assert.Equal(t, 1.0, remaining, "should be 1.0")

	side, amount, remaining = getSideAndAmountFromSize(-1, -1)
	assert.Equal(t, order.Short, side, "should be a sell order")
	assert.Equal(t, 1.0, amount, "should be 1.0")
	assert.Equal(t, 1.0, remaining, "should be 1.0")
}

func TestGetFutureOrderSize(t *testing.T) {
	t.Parallel()
	_, err := getFutureOrderSize(&order.Submit{Side: order.CouldNotCloseShort, Amount: 1})
	assert.ErrorIs(t, err, order.ErrSideIsInvalid)

	ret, err := getFutureOrderSize(&order.Submit{Side: order.Buy, Amount: 1})
	require.NoError(t, err)
	assert.Equal(t, 1.0, ret)

	ret, err = getFutureOrderSize(&order.Submit{Side: order.Sell, Amount: 1})
	require.NoError(t, err)
	assert.Equal(t, -1.0, ret)
}

func TestProcessFuturesOrdersPushData(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		incoming string
		status   order.Status
	}{
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"open","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.Open},
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"filled","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"finished","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.Filled},
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"cancelled","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"finished","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.Cancelled},
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"liquidated","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"finished","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.Liquidated},
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"ioc","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"finished","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.Cancelled},
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"auto_deleveraged","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"finished","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.AutoDeleverage},
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"reduce_only","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"finished","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.Cancelled},
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"position_closed","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"finished","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.Closed},
		{`{"channel":"futures.orders","event":"update","time":1541505434,"time_ms":1541505434123,"result":[{"contract":"BTC_USD","create_time":1628736847,"create_time_ms":1628736847325,"fill_price":40000.4,"finish_as":"stp","finish_time":1628736848,"finish_time_ms":1628736848321,"iceberg":0,"id":4872460,"is_close":false,"is_liq":false,"is_reduce_only":false,"left":0,"mkfr":-0.00025,"price":40000.4,"refr":0,"refu":0,"size":1,"status":"finished","text":"-","tif":"gtc","tkfr":0.0005,"user":"110xxxxx"}]}`, order.STP},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			processed, err := e.processFuturesOrdersPushData([]byte(tc.incoming), asset.CoinMarginedFutures)
			require.NoError(t, err)
			require.NotNil(t, processed)
			for i := range processed {
				assert.Equal(t, tc.status.String(), processed[i].Status.String())
			}
		})
	}
}

func TestGetCurrencyTradeURL(t *testing.T) {
	t.Parallel()
	testexch.UpdatePairsOnce(t, e)
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.CurrencyPairs.GetPairs(a, false)
		require.NoErrorf(t, err, "cannot get pairs for %s", a)
		require.NotEmptyf(t, pairs, "no pairs for %s", a)
		resp, err := e.GetCurrencyTradeURL(t.Context(), a, pairs[0])
		if a == asset.Options {
			require.ErrorIs(t, err, asset.ErrNotSupported)
		} else {
			require.NoError(t, err)
			assert.NotEmpty(t, resp)
		}
	}
}

func TestGetUnifiedAccount(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	// Requires unified account to be enabled for this to function.
	payload, err := e.GetUnifiedAccount(t.Context(), currency.EMPTYCODE)
	require.NoError(t, err)
	require.NotEmpty(t, payload)
}

func TestGetSettlementCurrency(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		a   asset.Item
		p   currency.Pair
		exp currency.Code
		err error
	}{
		{asset.Futures, currency.EMPTYPAIR, currency.EMPTYCODE, asset.ErrNotSupported},
		{asset.DeliveryFutures, currency.EMPTYPAIR, currency.USDT, nil},
		{asset.DeliveryFutures, getPair(t, asset.DeliveryFutures), currency.USDT, nil},
		{asset.USDTMarginedFutures, currency.EMPTYPAIR, currency.USDT, nil},
		{asset.USDTMarginedFutures, getPair(t, asset.USDTMarginedFutures), currency.USDT, nil},
		{asset.USDTMarginedFutures, getPair(t, asset.CoinMarginedFutures), currency.EMPTYCODE, errInvalidSettlementQuote},
		{asset.CoinMarginedFutures, currency.EMPTYPAIR, currency.BTC, nil},
		{asset.CoinMarginedFutures, getPair(t, asset.CoinMarginedFutures), currency.BTC, nil},
		{asset.CoinMarginedFutures, getPair(t, asset.USDTMarginedFutures), currency.EMPTYCODE, errInvalidSettlementBase},
		{asset.CoinMarginedFutures, currency.Pair{Base: currency.ETH, Quote: currency.USD}, currency.EMPTYCODE, errInvalidSettlementBase},
		{asset.CoinMarginedFutures, currency.NewBTCUSDT(), currency.EMPTYCODE, errInvalidSettlementQuote},
	} {
		c, err := getSettlementCurrency(tt.p, tt.a)
		if tt.err == nil {
			require.NoErrorf(t, err, "getSettlementCurrency must not error for %s %s", tt.a, tt.p)
		} else {
			assert.ErrorIsf(t, err, tt.err, "getSettlementCurrency should return correct error for %s %s", tt.a, tt.p)
		}
		assert.Equalf(t, tt.exp, c, "getSettlementCurrency should return correct settlement currency for %s %s", tt.a, tt.p)
	}
}

type FixtureConnection struct{ websocket.Connection }

func (d *FixtureConnection) SendMessageReturnResponse(context.Context, request.EndpointLimit, any, any) ([]byte, error) {
	return []byte(`{"time":1726121320,"time_ms":1726121320745,"id":1,"conn_id":"f903779a148987ca","trace_id":"d8ee37cd14347e4ed298d44e69aedaa7","channel":"spot.tickers","event":"subscribe","payload":["BRETT_USDT"],"result":{"status":"success"},"requestId":"d8ee37cd14347e4ed298d44e69aedaa7"}`), nil
}

func (d *FixtureConnection) GetURL() string { return "wss://test" }

func TestHandleSubscriptions(t *testing.T) {
	t.Parallel()

	subs := subscription.List{{Channel: subscription.OrderbookChannel}}

	err := e.handleSubscription(t.Context(), &FixtureConnection{}, subscribeEvent, subs, func(context.Context, string, subscription.List) ([]WsInput, error) {
		return []WsInput{{}}, nil
	})
	require.NoError(t, err)

	err = e.handleSubscription(t.Context(), &FixtureConnection{}, unsubscribeEvent, subs, func(context.Context, string, subscription.List) ([]WsInput, error) {
		return []WsInput{{}}, nil
	})
	require.NoError(t, err)
}

func TestParseWSHeader(t *testing.T) {
	in := []string{
		`{"time":1726121320,"time_ms":1726121320745,"id":1,"channel":"spot.tickers","event":"subscribe","result":{"status":"success"},"request_id":"a4"}`,
		`{"time_ms":1726121320746,"id":2,"channel":"spot.tickers","event":"subscribe","result":{"status":"success"},"request_id":"a4"}`,
		`{"time":1726121321,"id":3,"channel":"spot.tickers","event":"subscribe","result":{"status":"success"},"request_id":"a4"}`,
	}
	for _, i := range in {
		h, err := parseWSHeader([]byte(i))
		require.NoError(t, err)
		require.NotEmpty(t, h.ID)
		assert.Equal(t, "a4", h.RequestID)
		assert.Equal(t, "spot.tickers", h.Channel)
		assert.Equal(t, "subscribe", h.Event)
		assert.NotEmpty(t, h.Result)
		switch h.ID {
		case 1:
			assert.Equal(t, int64(1726121320745), h.Time.UnixMilli())
		case 2:
			assert.Equal(t, int64(1726121320746), h.Time.UnixMilli())
		case 3:
			assert.Equal(t, int64(1726121321), h.Time.Unix())
		}
	}
}

func TestDeriveSpotWebsocketOrderResponse(t *testing.T) {
	t.Parallel()

	var resp *WebsocketOrderResponse
	require.NoError(t, json.Unmarshal([]byte(`{"left":"0","update_time":"1735720637","amount":"0.0001","create_time":"1735720637","price":"0","finish_as":"filled","time_in_force":"ioc","currency_pair":"BTC_USDT","type":"market","account":"spot","side":"sell","amend_text":"-","text":"t-1735720637181634009","status":"closed","iceberg":"0","avg_deal_price":"93503.3","filled_total":"9.35033","id":"766075454481","fill_price":"9.35033","update_time_ms":1735720637188,"create_time_ms":1735720637188}`), &resp), "unmarshal must not error")

	got, err := e.deriveSpotWebsocketOrderResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, &order.SubmitResponse{
		Exchange:             e.Name,
		OrderID:              "766075454481",
		AssetType:            asset.Spot,
		Pair:                 currency.NewBTCUSDT().Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
		ClientOrderID:        "t-1735720637181634009",
		Date:                 time.UnixMilli(1735720637188),
		LastUpdated:          time.UnixMilli(1735720637188),
		Amount:               0.0001,
		AverageExecutedPrice: 93503.3,
		Type:                 order.Market,
		Side:                 order.Sell,
		Status:               order.Filled,
		TimeInForce:          order.ImmediateOrCancel,
		Cost:                 0.0001,
		Purchased:            9.35033,
	}, got)
}

func TestDeriveSpotWebsocketOrderResponses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		orders   [][]byte
		error    error
		expected []*order.SubmitResponse
	}{
		{
			name:   "no response",
			orders: [][]byte{},
			error:  common.ErrNoResponse,
		},
		{
			name: "assortment of spot orders",
			orders: [][]byte{
				[]byte(`{"left":"0","update_time":"1735720637","amount":"0.0001","create_time":"1735720637","price":"0","finish_as":"filled","time_in_force":"ioc","currency_pair":"BTC_USDT","type":"market","account":"spot","side":"sell","amend_text":"-","text":"t-1735720637181634009","status":"closed","iceberg":"0","avg_deal_price":"93503.3","filled_total":"9.35033","id":"766075454481","fill_price":"9.35033","update_time_ms":1735720637188,"create_time_ms":1735720637188}`),
				[]byte(`{"left":"0.000008","update_time":"1735720637","amount":"9.99152","create_time":"1735720637","price":"0","finish_as":"filled","time_in_force":"ioc","currency_pair":"HNS_USDT","type":"market","account":"spot","side":"buy","amend_text":"-","text":"t-1735720637126962151","status":"closed","iceberg":"0","avg_deal_price":"0.01224","filled_total":"9.991512","id":"766075454188","fill_price":"9.991512","update_time_ms":1735720637142,"create_time_ms":1735720637142}`),
				[]byte(`{"left":"0","update_time":"1735778597","amount":"200","create_time":"1735778597","price":"0.03673","finish_as":"filled","time_in_force":"fok","currency_pair":"REX_USDT","type":"limit","account":"spot","side":"buy","amend_text":"-","text":"t-1364","status":"closed","iceberg":"0","avg_deal_price":"0.03673","filled_total":"7.346","id":"766488882062","fill_price":"7.346","update_time_ms":1735778597363,"create_time_ms":1735778597363}`),
				[]byte(`{"left":"0.0003","update_time":"1735780321","amount":"0.0003","create_time":"1735780321","price":"20000","finish_as":"open","time_in_force":"poc","currency_pair":"BTC_USDT","type":"limit","account":"spot","side":"buy","amend_text":"-","text":"t-1735780321603944400","status":"open","iceberg":"0","filled_total":"0","id":"766504537761","fill_price":"0","update_time_ms":1735780321729,"create_time_ms":1735780321729}`),
				[]byte(`{"left":"1","update_time":"1735784755","amount":"1","create_time":"1735784755","price":"100","finish_as":"open","time_in_force":"gtc","currency_pair":"GT_USDT","type":"limit","account":"spot","side":"sell","amend_text":"-","text":"t-1735784754905434100","status":"open","iceberg":"0","filled_total":"0","id":"766536556747","fill_price":"0","update_time_ms":1735784755068,"create_time_ms":1735784755068}`),
			},
			expected: []*order.SubmitResponse{
				{
					Exchange:             e.Name,
					OrderID:              "766075454481",
					AssetType:            asset.Spot,
					Pair:                 currency.NewBTCUSDT().Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:        "t-1735720637181634009",
					Date:                 time.UnixMilli(1735720637188),
					LastUpdated:          time.UnixMilli(1735720637188),
					Amount:               0.0001,
					AverageExecutedPrice: 93503.3,
					Type:                 order.Market,
					Side:                 order.Sell,
					Status:               order.Filled,
					TimeInForce:          order.ImmediateOrCancel,
					Cost:                 0.0001,
					Purchased:            9.35033,
				},
				{
					Exchange:             e.Name,
					OrderID:              "766075454188",
					AssetType:            asset.Spot,
					Pair:                 currency.NewPair(currency.HNS, currency.USDT).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:        "t-1735720637126962151",
					Date:                 time.UnixMilli(1735720637142),
					LastUpdated:          time.UnixMilli(1735720637142),
					RemainingAmount:      0.000008,
					Amount:               9.99152,
					AverageExecutedPrice: 0.01224,
					Type:                 order.Market,
					Side:                 order.Buy,
					Status:               order.Filled,
					TimeInForce:          order.ImmediateOrCancel,
					Cost:                 9.991512,
					Purchased:            816.3,
				},
				{
					Exchange:             e.Name,
					OrderID:              "766488882062",
					AssetType:            asset.Spot,
					Pair:                 currency.NewPair(currency.NewCode("REX"), currency.USDT).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:        "t-1364",
					Date:                 time.UnixMilli(1735778597363),
					LastUpdated:          time.UnixMilli(1735778597363),
					Amount:               200,
					Price:                0.03673,
					AverageExecutedPrice: 0.03673,
					Type:                 order.Limit,
					Side:                 order.Buy,
					Status:               order.Filled,
					TimeInForce:          order.FillOrKill,
					Cost:                 7.346,
					Purchased:            200,
				},
				{
					Exchange:        e.Name,
					OrderID:         "766504537761",
					AssetType:       asset.Spot,
					Pair:            currency.NewBTCUSDT().Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:   "t-1735780321603944400",
					Date:            time.UnixMilli(1735780321729),
					LastUpdated:     time.UnixMilli(1735780321729),
					RemainingAmount: 0.0003,
					Amount:          0.0003,
					Price:           20000,
					Type:            order.Limit,
					Side:            order.Buy,
					Status:          order.Open,
					TimeInForce:     order.PostOnly,
				},
				{
					Exchange:        e.Name,
					OrderID:         "766536556747",
					AssetType:       asset.Spot,
					Pair:            currency.NewPair(currency.NewCode("GT"), currency.USDT).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:   "t-1735784754905434100",
					Date:            time.UnixMilli(1735784755068),
					LastUpdated:     time.UnixMilli(1735784755068),
					RemainingAmount: 1,
					Amount:          1,
					Price:           100,
					Type:            order.Limit,
					Side:            order.Sell,
					Status:          order.Open,
					TimeInForce:     order.GoodTillCancel,
				},
			},
		},
		{
			name: "batch of spot orders with error at end",
			// This is specifically testing the return responses of WebsocketSpotSubmitOrders
			// AverageDealPrice is not returned when using this endpoint so purchased and cost fields cannot be set.
			orders: [][]byte{
				[]byte(`{"account":"spot","status":"closed","side":"buy","amount":"9.98","id":"775453816782","create_time":"1736980695","update_time":"1736980695","text":"t-740","left":"0.047239","currency_pair":"ETH_USDT","type":"market","finish_as":"filled","price":"0","time_in_force":"fok","iceberg":"0","filled_total":"9.932761","fill_price":"9.932761","create_time_ms":1736980695949,"update_time_ms":1736980695949,"succeeded":true}`),
				[]byte(`{"account":"spot","status":"closed","side":"buy","amount":"0.00289718","id":"775453816824","create_time":"1736980695","update_time":"1736980695","text":"t-741","left":"0.00000000962","currency_pair":"LIKE_ETH","type":"market","finish_as":"filled","price":"0","time_in_force":"fok","iceberg":"0","filled_total":"0.00289717038","fill_price":"0.00289717038","create_time_ms":1736980695956,"update_time_ms":1736980695956,"succeeded":true}`),
				[]byte(`{"text":"t-742","label":"BALANCE_NOT_ENOUGH","message":"Not enough balance"}`),
			},
			expected: []*order.SubmitResponse{
				{
					Exchange:        e.Name,
					OrderID:         "775453816782",
					AssetType:       asset.Spot,
					Pair:            currency.NewPair(currency.ETH, currency.USDT).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:   "t-740",
					Date:            time.UnixMilli(1736980695949),
					LastUpdated:     time.UnixMilli(1736980695949),
					Amount:          9.98,
					RemainingAmount: 0.047239,
					Type:            order.Market,
					Side:            order.Buy,
					Status:          order.Filled,
					TimeInForce:     order.FillOrKill,
				},
				{
					Exchange:        e.Name,
					OrderID:         "775453816824",
					AssetType:       asset.Spot,
					Pair:            currency.NewPair(currency.LIKE, currency.ETH).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:   "t-741",
					Date:            time.UnixMilli(1736980695956),
					LastUpdated:     time.UnixMilli(1736980695956),
					RemainingAmount: 0.00000000962,
					Amount:          0.00289718,
					Type:            order.Market,
					Side:            order.Buy,
					Status:          order.Filled,
					TimeInForce:     order.FillOrKill,
				},
				{
					Exchange:        e.Name,
					ClientOrderID:   "t-742",
					SubmissionError: order.ErrUnableToPlaceOrder,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			orders := bytes.Join(tc.orders, []byte(","))
			orders = append([]byte("["), append(orders, []byte("]")...)...)

			var resp []*WebsocketOrderResponse
			require.NoError(t, json.Unmarshal(orders, &resp), "unmarshal must not error")

			got, err := e.deriveSpotWebsocketOrderResponses(resp)
			require.ErrorIs(t, err, tc.error)

			require.Len(t, got, len(tc.expected))
			for i := range got {
				if tc.expected[i].SubmissionError != nil {
					assert.ErrorIs(t, got[i].SubmissionError, tc.expected[i].SubmissionError)
					assert.Equal(t, tc.expected[i].Exchange, got[i].Exchange)
					assert.Equal(t, tc.expected[i].ClientOrderID, got[i].ClientOrderID)
					continue
				}
				assert.Equal(t, tc.expected[i], got[i])
			}
		})
	}
}

func TestDeriveFuturesWebsocketOrderResponse(t *testing.T) {
	t.Parallel()

	var resp *WebsocketFuturesOrderResponse
	require.NoError(t, json.Unmarshal([]byte(`{"text":"t-1337","price":"0","biz_info":"-","tif":"ioc","amend_text":"-","status":"finished","contract":"CWIF_USDT","stp_act":"-","finish_as":"filled","fill_price":"0.0000002625","id":596729318437,"create_time":1735787107.449,"size":2,"finish_time":1735787107.45,"update_time":1735787107.45,"left":0,"user":12870774,"is_reduce_only":true}`), &resp), "unmarshal must not error")

	got, err := e.deriveFuturesWebsocketOrderResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, &order.SubmitResponse{
		Exchange:             e.Name,
		OrderID:              "596729318437",
		AssetType:            asset.Futures,
		Pair:                 currency.NewPair(currency.NewCode("CWIF"), currency.USDT).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
		ClientOrderID:        "t-1337",
		Date:                 time.UnixMilli(1735787107449),
		LastUpdated:          time.UnixMilli(1735787107450),
		Amount:               2,
		AverageExecutedPrice: 0.0000002625,
		Type:                 order.Market,
		Side:                 order.Long,
		Status:               order.Filled,
		TimeInForce:          order.ImmediateOrCancel,
		ReduceOnly:           true,
	}, got)
}

func TestDeriveFuturesWebsocketOrderResponses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		orders   [][]byte
		error    error
		expected []*order.SubmitResponse
	}{
		{
			name:   "no response",
			orders: [][]byte{},
			error:  common.ErrNoResponse,
		},
		{
			name: "assortment of futures orders",
			orders: [][]byte{
				[]byte(`{"text":"t-1337","price":"0","biz_info":"-","tif":"ioc","amend_text":"-","status":"finished","contract":"CWIF_USDT","stp_act":"-","finish_as":"filled","fill_price":"0.0000002625","id":596729318437,"create_time":1735787107.449,"size":2,"finish_time":1735787107.45,"update_time":1735787107.45,"left":0,"user":12870774,"is_reduce_only":true}`),
				[]byte(`{"text":"t-1336","price":"0","biz_info":"-","tif":"ioc","amend_text":"-","status":"finished","contract":"REX_USDT","stp_act":"-","finish_as":"filled","fill_price":"0.03654","id":596662040388,"create_time":1735778597.374,"size":-2,"finish_time":1735778597.374,"update_time":1735778597.374,"left":0,"user":12870774}`),
				[]byte(`{"text":"apiv4-ws","price":"40000","biz_info":"-","tif":"gtc","amend_text":"-","status":"open","contract":"BTC_USDT","stp_act":"-","fill_price":"0","id":596746193678,"create_time":1735789790.476,"size":1,"update_time":1735789790.476,"left":1,"user":2365748}`),
				[]byte(`{"text":"apiv4-ws","price":"200000","biz_info":"-","tif":"gtc","amend_text":"-","status":"open","contract":"BTC_USDT","stp_act":"-","fill_price":"0","id":596748780649,"create_time":1735790222.185,"size":-1,"update_time":1735790222.185,"left":-1,"user":2365748}`),
				[]byte(`{"text":"apiv4-ws","price":"0","biz_info":"-","tif":"ioc","amend_text":"-","status":"finished","contract":"BTC_USDT","stp_act":"-","finish_as":"filled","fill_price":"98172.9","id":36028797827161124,"create_time":1740108860.761,"size":1,"finish_time":1740108860.761,"update_time":1740108860.761,"left":0,"user":2365748}`),
				[]byte(`{"text":"apiv4-ws","price":"0","biz_info":"-","tif":"ioc","amend_text":"-","status":"finished","contract":"BTC_USDT","stp_act":"-","finish_as":"filled","fill_price":"98113.1","id":36028797827225781,"create_time":1740109172.06,"size":-1,"finish_time":1740109172.06,"update_time":1740109172.06,"left":0,"user":2365748,"is_reduce_only":true}`),
			},
			expected: []*order.SubmitResponse{
				{
					Exchange:             e.Name,
					OrderID:              "596729318437",
					AssetType:            asset.Futures,
					Pair:                 currency.NewPair(currency.NewCode("CWIF"), currency.USDT).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:        "t-1337",
					Date:                 time.UnixMilli(1735787107449),
					LastUpdated:          time.UnixMilli(1735787107450),
					Amount:               2,
					AverageExecutedPrice: 0.0000002625,
					Type:                 order.Market,
					Side:                 order.Long,
					Status:               order.Filled,
					TimeInForce:          order.ImmediateOrCancel,
					ReduceOnly:           true,
				},
				{
					Exchange:             e.Name,
					OrderID:              "596662040388",
					AssetType:            asset.Futures,
					Pair:                 currency.NewPair(currency.NewCode("REX"), currency.USDT).Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					ClientOrderID:        "t-1336",
					Date:                 time.UnixMilli(1735778597374),
					LastUpdated:          time.UnixMilli(1735778597374),
					Amount:               2,
					AverageExecutedPrice: 0.03654,
					Type:                 order.Market,
					Side:                 order.Short,
					Status:               order.Filled,
					TimeInForce:          order.ImmediateOrCancel,
				},
				{
					Exchange:        e.Name,
					OrderID:         "596746193678",
					AssetType:       asset.Futures,
					Pair:            currency.NewBTCUSDT().Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					Date:            time.UnixMilli(1735789790476),
					LastUpdated:     time.UnixMilli(1735789790476),
					RemainingAmount: 1,
					Amount:          1,
					Price:           40000,
					Type:            order.Limit,
					Side:            order.Long,
					Status:          order.Open,
					TimeInForce:     order.GoodTillCancel,
				},
				{
					Exchange:        e.Name,
					OrderID:         "596748780649",
					AssetType:       asset.Futures,
					Pair:            currency.NewBTCUSDT().Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					Date:            time.UnixMilli(1735790222185),
					LastUpdated:     time.UnixMilli(1735790222185),
					RemainingAmount: 1,
					Amount:          1,
					Price:           200000,
					Type:            order.Limit,
					Side:            order.Short,
					Status:          order.Open,
					TimeInForce:     order.GoodTillCancel,
				},
				{
					Exchange:             e.Name,
					OrderID:              "36028797827161124",
					AssetType:            asset.Futures,
					Pair:                 currency.NewBTCUSDT().Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					Date:                 time.UnixMilli(1740108860761),
					LastUpdated:          time.UnixMilli(1740108860761),
					Amount:               1,
					AverageExecutedPrice: 98172.9,
					Type:                 order.Market,
					Side:                 order.Long,
					Status:               order.Filled,
					TimeInForce:          order.ImmediateOrCancel,
				},
				{
					Exchange:             e.Name,
					OrderID:              "36028797827225781",
					AssetType:            asset.Futures,
					Pair:                 currency.NewBTCUSDT().Format(currency.PairFormat{Uppercase: true, Delimiter: "_"}),
					Date:                 time.UnixMilli(1740109172060),
					LastUpdated:          time.UnixMilli(1740109172060),
					Amount:               1,
					AverageExecutedPrice: 98113.1,
					Type:                 order.Market,
					Side:                 order.Short,
					Status:               order.Filled,
					TimeInForce:          order.ImmediateOrCancel,
					ReduceOnly:           true,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			orders := bytes.Join(tc.orders, []byte(","))
			orders = append([]byte("["), append(orders, []byte("]")...)...)

			var resp []*WebsocketFuturesOrderResponse
			require.NoError(t, json.Unmarshal(orders, &resp), "unmarshal must not error")

			got, err := e.deriveFuturesWebsocketOrderResponses(resp)
			require.ErrorIs(t, err, tc.error)

			require.Len(t, got, len(tc.expected))
			for i := range got {
				assert.Equal(t, tc.expected[i], got[i])
			}
		})
	}
}

func TestConvertSmallBalances(t *testing.T) {
	t.Parallel()
	err := e.ConvertSmallBalances(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	err = e.ConvertSmallBalances(t.Context(), currency.F16)
	require.NoError(t, err)
}

func TestGetAccountDetails(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	got, err := e.GetAccountDetails(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, got)
}

func TestGetUserTransactionRateLimitInfo(t *testing.T) {
	t.Parallel()
	sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	got, err := e.GetUserTransactionRateLimitInfo(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, got)
}

var pairMap = map[asset.Item]currency.Pairs{}

var pairsGuard sync.RWMutex

func getPair(tb testing.TB, a asset.Item) currency.Pair {
	tb.Helper()
	if p := getPairs(tb, a); len(p) != 0 {
		return p[0]
	}
	return currency.EMPTYPAIR
}

func getPairs(tb testing.TB, a asset.Item) currency.Pairs {
	tb.Helper()
	pairsGuard.RLock()
	p, ok := pairMap[a]
	pairsGuard.RUnlock()
	if ok {
		return p
	}
	pairsGuard.Lock()
	defer pairsGuard.Unlock()
	p, ok = pairMap[a] // Protect Race if we blocked on Lock and another RW populated
	if ok {
		return p
	}

	testexch.UpdatePairsOnce(tb, e)
	enabledPairs, err := e.GetEnabledPairs(a)
	assert.NoErrorf(tb, err, "%s GetEnabledPairs should not error", a)
	if !assert.NotEmptyf(tb, enabledPairs, "%s GetEnabledPairs should not be empty", a) {
		tb.Fatalf("No pair available for asset %s", a)
		return nil
	}
	pairMap[a] = enabledPairs

	return enabledPairs
}

func BenchmarkTimeInForceFromString(b *testing.B) {
	for b.Loop() {
		for _, tifString := range []string{gtcTIF, iocTIF, pocTIF, fokTIF} {
			if _, err := timeInForceFromString(tifString); err != nil {
				b.Fatal(tifString)
			}
		}
	}
}

func TestTimeInForceFromString(t *testing.T) {
	t.Parallel()
	_, err := timeInForceFromString("abcdef")
	assert.ErrorIs(t, err, order.ErrUnsupportedTimeInForce)

	for k, v := range map[string]order.TimeInForce{gtcTIF: order.GoodTillCancel, iocTIF: order.ImmediateOrCancel, pocTIF: order.PostOnly, fokTIF: order.FillOrKill} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			tif, err := timeInForceFromString(k)
			require.NoError(t, err)
			assert.Equal(t, v, tif)
		})
	}
}

func TestGetTypeFromTimeInForce(t *testing.T) {
	t.Parallel()
	typeResp := getTypeFromTimeInForce("gtc", 0)
	assert.Equal(t, order.Limit, typeResp)

	typeResp = getTypeFromTimeInForce("ioc", 0)
	assert.Equal(t, order.Market, typeResp, "should be market order")

	typeResp = getTypeFromTimeInForce("poc", 123)
	assert.Equal(t, order.Limit, typeResp, "should be limit order")

	typeResp = getTypeFromTimeInForce("fok", 0)
	assert.Equal(t, order.Market, typeResp, "should be market order")
}

func TestToExchangeTIF(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		tif      order.TimeInForce
		price    float64
		expected string
		err      error
	}{
		{price: 0, expected: iocTIF}, // market orders default to IOC
		{price: 0, tif: order.FillOrKill, expected: fokTIF},
		{price: 420, expected: gtcTIF}, // limit orders default to GTC
		{price: 420, tif: order.GoodTillCancel, expected: gtcTIF},
		{price: 420, tif: order.ImmediateOrCancel, expected: iocTIF},
		{price: 420, tif: order.PostOnly, expected: pocTIF},
		{price: 420, tif: order.FillOrKill, expected: fokTIF},
		{tif: order.GoodTillTime, err: order.ErrUnsupportedTimeInForce},
	} {
		t.Run(fmt.Sprintf("TIF:%q Price:'%v'", tc.tif, tc.price), func(t *testing.T) {
			t.Parallel()
			got, err := toExchangeTIF(tc.tif, tc.price)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestIsSingleOrderbookChannel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		channel  string
		expected bool
	}{
		{channel: spotOrderbookUpdateChannel, expected: true},
		{channel: spotOrderbookChannel, expected: true},
		{channel: spotOrderbookTickerChannel, expected: true},
		{channel: futuresOrderbookChannel, expected: true},
		{channel: futuresOrderbookTickerChannel, expected: true},
		{channel: futuresOrderbookUpdateChannel, expected: true},
		{channel: optionsOrderbookChannel, expected: true},
		{channel: optionsOrderbookTickerChannel, expected: true},
		{channel: optionsOrderbookUpdateChannel, expected: true},
		{channel: spotTickerChannel, expected: false},
		{channel: "sad", expected: false},
	} {
		assert.Equal(t, tc.expected, isSingleOrderbookChannel(tc.channel))
	}
}

func TestValidateSubscriptions(t *testing.T) {
	t.Parallel()
	require.NoError(t, e.ValidateSubscriptions(nil))
	require.NoError(t, e.ValidateSubscriptions([]*subscription.Subscription{{Channel: spotTickerChannel, Pairs: []currency.Pair{currency.NewBTCUSDT()}}}))
	require.NoError(t, e.ValidateSubscriptions([]*subscription.Subscription{
		{Channel: spotTickerChannel, Pairs: []currency.Pair{currency.NewBTCUSD()}},
		{Channel: spotOrderbookUpdateChannel, Pairs: []currency.Pair{currency.NewBTCUSD()}},
	}))
	require.NoError(t, e.ValidateSubscriptions([]*subscription.Subscription{
		{Channel: spotTickerChannel, Pairs: []currency.Pair{currency.NewBTCUSD()}},
		{Channel: spotOrderbookUpdateChannel, Pairs: []currency.Pair{currency.NewBTCUSD(), currency.NewBTCUSDT()}},
	}))
	require.NoError(t, e.ValidateSubscriptions([]*subscription.Subscription{
		{Channel: spotTickerChannel, Pairs: []currency.Pair{currency.NewBTCUSD()}},
		{Channel: spotOrderbookUpdateChannel, Pairs: []currency.Pair{currency.NewBTCUSD()}},
		{Channel: spotOrderbookUpdateChannel, Pairs: []currency.Pair{currency.NewBTCUSDT()}},
	}))
	require.ErrorIs(t, e.ValidateSubscriptions([]*subscription.Subscription{
		{Channel: spotTickerChannel, Pairs: []currency.Pair{currency.NewBTCUSD()}},
		{Channel: spotOrderbookUpdateChannel, Pairs: []currency.Pair{currency.NewBTCUSD()}},
		{Channel: spotOrderbookChannel, Pairs: []currency.Pair{currency.NewBTCUSD()}},
	}), subscription.ErrExclusiveSubscription)
}

func TestCandlesChannelIntervals(t *testing.T) {
	t.Parallel()
	s := &subscription.Subscription{Channel: subscription.CandlesChannel, Asset: asset.Spot, Interval: 0}
	_, err := candlesChannelInterval(s)
	require.ErrorIs(t, err, kline.ErrUnsupportedInterval, "candlestickChannelInterval must error correctly with a 0 interval")
	s.Interval = kline.ThousandMilliseconds
	i, err := candlesChannelInterval(s)
	require.NoError(t, err)
	assert.Equal(t, "1000ms", i)
}

func TestOrderbookChannelIntervals(t *testing.T) {
	t.Parallel()

	s := &subscription.Subscription{Channel: futuresOrderbookUpdateChannel, Interval: kline.TwentyMilliseconds, Levels: 100}
	_, err := orderbookChannelInterval(s, asset.Futures)
	require.ErrorIs(t, err, subscription.ErrInvalidInterval)
	require.ErrorContains(t, err, "20ms only valid with Levels 20")
	s.Levels = 20
	i, err := orderbookChannelInterval(s, asset.Futures)
	require.NoError(t, err)
	assert.Equal(t, "20ms", i)

	for s, exp := range map[*subscription.Subscription]error{
		{Asset: asset.Binary, Channel: "unknown_channel", Interval: kline.OneYear}:                                   nil,
		{Asset: asset.Spot, Channel: spotOrderbookTickerChannel, Interval: kline.OneDay}:                             subscription.ErrInvalidInterval,
		{Asset: asset.Spot, Channel: spotOrderbookTickerChannel, Interval: 0}:                                        nil,
		{Asset: asset.Spot, Channel: spotOrderbookChannel, Interval: kline.OneDay}:                                   subscription.ErrInvalidInterval,
		{Asset: asset.Spot, Channel: spotOrderbookChannel, Interval: kline.HundredMilliseconds}:                      nil,
		{Asset: asset.Spot, Channel: spotOrderbookChannel, Interval: kline.ThousandMilliseconds}:                     nil,
		{Asset: asset.Spot, Channel: spotOrderbookUpdateChannel, Interval: kline.OneDay}:                             subscription.ErrInvalidInterval,
		{Asset: asset.Spot, Channel: spotOrderbookUpdateChannel, Interval: kline.HundredMilliseconds}:                nil,
		{Asset: asset.Futures, Channel: futuresOrderbookTickerChannel, Interval: kline.TenMilliseconds}:              subscription.ErrInvalidInterval,
		{Asset: asset.Futures, Channel: futuresOrderbookTickerChannel, Interval: 0}:                                  nil,
		{Asset: asset.Futures, Channel: futuresOrderbookChannel, Interval: kline.TenMilliseconds}:                    subscription.ErrInvalidInterval,
		{Asset: asset.Futures, Channel: futuresOrderbookChannel, Interval: 0}:                                        nil,
		{Asset: asset.Futures, Channel: futuresOrderbookUpdateChannel, Interval: kline.OneDay}:                       subscription.ErrInvalidInterval,
		{Asset: asset.Futures, Channel: futuresOrderbookUpdateChannel, Interval: kline.HundredMilliseconds}:          nil,
		{Asset: asset.DeliveryFutures, Channel: futuresOrderbookTickerChannel, Interval: kline.TenMilliseconds}:      subscription.ErrInvalidInterval,
		{Asset: asset.DeliveryFutures, Channel: futuresOrderbookTickerChannel, Interval: 0}:                          nil,
		{Asset: asset.DeliveryFutures, Channel: futuresOrderbookChannel, Interval: kline.TenMilliseconds}:            subscription.ErrInvalidInterval,
		{Asset: asset.DeliveryFutures, Channel: futuresOrderbookChannel, Interval: 0}:                                nil,
		{Asset: asset.DeliveryFutures, Channel: futuresOrderbookUpdateChannel, Interval: kline.OneDay}:               subscription.ErrInvalidInterval,
		{Asset: asset.DeliveryFutures, Channel: futuresOrderbookUpdateChannel, Interval: kline.HundredMilliseconds}:  nil,
		{Asset: asset.DeliveryFutures, Channel: futuresOrderbookUpdateChannel, Interval: kline.ThousandMilliseconds}: nil,

		{Asset: asset.Options, Channel: optionsOrderbookTickerChannel, Interval: kline.TenMilliseconds}:          subscription.ErrInvalidInterval,
		{Asset: asset.Options, Channel: optionsOrderbookTickerChannel, Interval: 0}:                              nil,
		{Asset: asset.Options, Channel: optionsOrderbookChannel, Interval: kline.TwoHundredAndFiftyMilliseconds}: subscription.ErrInvalidInterval,
		{Asset: asset.Options, Channel: optionsOrderbookChannel, Interval: 0}:                                    nil,
		{Asset: asset.Options, Channel: optionsOrderbookUpdateChannel, Interval: kline.OneDay}:                   subscription.ErrInvalidInterval,
		{Asset: asset.Options, Channel: optionsOrderbookUpdateChannel, Interval: kline.HundredMilliseconds}:      nil,
		{Asset: asset.Options, Channel: optionsOrderbookUpdateChannel, Interval: kline.ThousandMilliseconds}:     nil,
	} {
		t.Run(s.Asset.String()+"/"+s.Channel+"/"+s.Interval.Short(), func(t *testing.T) {
			t.Parallel()
			i, err := orderbookChannelInterval(s, s.Asset)
			if exp != nil {
				require.ErrorIs(t, err, exp)
			} else {
				switch {
				case s.Channel == "unknown_channel":
					assert.Empty(t, i, "orderbookChannelInterval should return empty for unknown channels")
				case strings.HasSuffix(s.Channel, "_ticker"):
					assert.Empty(t, i)
				case s.Interval == 0:
					assert.Equal(t, "0", i)
				default:
					exp, err2 := getIntervalString(s.Interval)
					require.NoError(t, err2, "getIntervalString must not error for validating expected value")
					require.Equal(t, exp, i)
				}
			}
		})
	}
}

func TestChannelLevels(t *testing.T) {
	t.Parallel()

	for s, exp := range map[*subscription.Subscription]error{
		{Channel: "unknown_channel", Asset: asset.Binary}:                                   nil,
		{Channel: spotOrderbookTickerChannel, Asset: asset.Spot}:                            nil,
		{Channel: spotOrderbookTickerChannel, Asset: asset.Spot, Levels: 1}:                 subscription.ErrInvalidLevel,
		{Channel: spotOrderbookUpdateChannel, Asset: asset.Spot}:                            nil,
		{Channel: spotOrderbookUpdateChannel, Asset: asset.Spot, Levels: 100}:               subscription.ErrInvalidLevel,
		{Channel: spotOrderbookChannel, Asset: asset.Spot}:                                  subscription.ErrInvalidLevel,
		{Channel: spotOrderbookChannel, Asset: asset.Spot, Levels: 5}:                       nil,
		{Channel: spotOrderbookChannel, Asset: asset.Spot, Levels: 10}:                      nil,
		{Channel: spotOrderbookChannel, Asset: asset.Spot, Levels: 20}:                      nil,
		{Channel: spotOrderbookChannel, Asset: asset.Spot, Levels: 50}:                      nil,
		{Channel: spotOrderbookChannel, Asset: asset.Spot, Levels: 100}:                     nil,
		{Channel: futuresOrderbookChannel, Asset: asset.Futures}:                            subscription.ErrInvalidLevel,
		{Channel: futuresOrderbookChannel, Asset: asset.Futures, Levels: 1}:                 nil,
		{Channel: futuresOrderbookChannel, Asset: asset.Futures, Levels: 5}:                 nil,
		{Channel: futuresOrderbookChannel, Asset: asset.Futures, Levels: 10}:                nil,
		{Channel: futuresOrderbookChannel, Asset: asset.Futures, Levels: 20}:                nil,
		{Channel: futuresOrderbookChannel, Asset: asset.Futures, Levels: 50}:                nil,
		{Channel: futuresOrderbookChannel, Asset: asset.Futures, Levels: 100}:               nil,
		{Channel: futuresOrderbookTickerChannel, Asset: asset.Futures}:                      nil,
		{Channel: futuresOrderbookTickerChannel, Asset: asset.Futures, Levels: 1}:           subscription.ErrInvalidLevel,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.Futures}:                      subscription.ErrInvalidLevel,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.Futures, Levels: 20}:          nil,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.Futures, Levels: 50}:          nil,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.DeliveryFutures}:              subscription.ErrInvalidLevel,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.DeliveryFutures, Levels: 5}:   nil,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.DeliveryFutures, Levels: 10}:  nil,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.DeliveryFutures, Levels: 20}:  nil,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.DeliveryFutures, Levels: 50}:  nil,
		{Channel: futuresOrderbookUpdateChannel, Asset: asset.DeliveryFutures, Levels: 100}: nil,
		{Channel: optionsOrderbookTickerChannel, Asset: asset.Options}:                      nil,
		{Channel: optionsOrderbookTickerChannel, Asset: asset.Options, Levels: 1}:           subscription.ErrInvalidLevel,
		{Channel: optionsOrderbookUpdateChannel, Asset: asset.Options}:                      subscription.ErrInvalidLevel,
		{Channel: optionsOrderbookUpdateChannel, Asset: asset.Options, Levels: 5}:           nil,
		{Channel: optionsOrderbookUpdateChannel, Asset: asset.Options, Levels: 10}:          nil,
		{Channel: optionsOrderbookUpdateChannel, Asset: asset.Options, Levels: 20}:          nil,
		{Channel: optionsOrderbookUpdateChannel, Asset: asset.Options, Levels: 50}:          nil,
		{Channel: optionsOrderbookChannel, Asset: asset.Options}:                            subscription.ErrInvalidLevel,
		{Channel: optionsOrderbookChannel, Asset: asset.Options, Levels: 5}:                 nil,
		{Channel: optionsOrderbookChannel, Asset: asset.Options, Levels: 10}:                nil,
		{Channel: optionsOrderbookChannel, Asset: asset.Options, Levels: 20}:                nil,
		{Channel: optionsOrderbookChannel, Asset: asset.Options, Levels: 50}:                nil,
	} {
		t.Run(s.Asset.String()+"/"+s.Channel+"/"+strconv.Itoa(s.Levels), func(t *testing.T) {
			t.Parallel()
			l, err := channelLevels(s, s.Asset)
			switch {
			case exp != nil:
				require.ErrorIs(t, err, exp)
			case s.Levels == 0:
				assert.Empty(t, l)
			default:
				require.NoError(t, err)
				require.NotEmpty(t, l)
			}
		})
	}
}

func TestGetIntervalString(t *testing.T) {
	t.Parallel()
	for k, exp := range map[kline.Interval]string{
		kline.TenMilliseconds:                "10ms",
		kline.TwentyMilliseconds:             "20ms",
		kline.HundredMilliseconds:            "100ms",
		kline.TwoHundredAndFiftyMilliseconds: "250ms",
		kline.ThousandMilliseconds:           "1000ms",
		kline.TenSecond:                      "10s",
		kline.ThirtySecond:                   "30s",
		kline.OneMin:                         "1m",
		kline.FiveMin:                        "5m",
		kline.FifteenMin:                     "15m",
		kline.ThirtyMin:                      "30m",
		kline.OneHour:                        "1h",
		kline.TwoHour:                        "2h",
		kline.FourHour:                       "4h",
		kline.EightHour:                      "8h",
		kline.TwelveHour:                     "12h",
		kline.OneDay:                         "1d",
		kline.SevenDay:                       "7d",
		kline.OneMonth:                       "30d",
	} {
		t.Run(exp, func(t *testing.T) {
			t.Parallel()
			s, err := getIntervalString(k)
			require.NoError(t, err)
			assert.Equal(t, exp, s)
		})
	}
	_, err := getIntervalString(0)
	assert.ErrorIs(t, err, kline.ErrUnsupportedInterval, "0 should be an invalid interval")
	_, err = getIntervalString(kline.FiveDay)
	assert.ErrorIs(t, err, kline.ErrUnsupportedInterval, "Any other random interval should also be invalid")
}

func TestWebsocketSubmitOrders(t *testing.T) {
	t.Parallel()

	_, err := e.WebsocketSubmitOrders(t.Context(), nil)
	require.ErrorIs(t, err, asset.ErrNotSupported)

	sub := &order.Submit{
		Exchange:    e.Name,
		AssetType:   asset.Spot,
		Side:        order.Buy,
		Type:        order.Market,
		QuoteAmount: 10,
	}
	_, err = e.WebsocketSubmitOrders(t.Context(), []*order.Submit{sub})
	require.ErrorIs(t, err, order.ErrPairIsEmpty)

	sub.Pair = currency.NewBTCUSD()
	cpy := *sub
	cpy.AssetType = asset.Futures
	_, err = e.WebsocketSubmitOrders(t.Context(), []*order.Submit{sub, &cpy})
	require.ErrorIs(t, err, errSingleAssetRequired)

	cpy.AssetType = asset.Spread
	sub.AssetType = asset.Spread
	_, err = e.WebsocketSubmitOrders(t.Context(), []*order.Submit{sub, &cpy})
	require.ErrorIs(t, err, asset.ErrNotSupported)

	sub.AssetType = asset.USDTMarginedFutures
	cpy.AssetType = asset.USDTMarginedFutures
	_, err = e.WebsocketSubmitOrders(t.Context(), []*order.Submit{sub, &cpy})
	require.ErrorIs(t, err, errInvalidOrderSize)

	sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)

	e := newExchangeWithWebsocket(t, asset.Spot)

	sub.AssetType = asset.Spot
	cpy.AssetType = asset.Spot
	_, err = e.WebsocketSubmitOrders(request.WithVerbose(t.Context()), []*order.Submit{sub, &cpy})
	require.NoError(t, err)
}

func TestValidateOrderCreateParams(t *testing.T) {
	t.Parallel()

	// Test nil pointer cases separately since they can't be constructed from shared fields.
	assert.ErrorIs(t, (*FuturesOrderCreateParams)(nil).validate(false), common.ErrNilPointer, "nil FuturesOrderCreateParams should error")
	assert.ErrorIs(t, (*DeliveryOrderCreateParams)(nil).validate(false), common.ErrNilPointer, "nil DeliveryOrderCreateParams should error")

	for _, tc := range []struct {
		name        string
		contract    currency.Pair
		size        float64
		timeInForce string
		text        string
		autoSize    string
		settle      currency.Code
		isRest      bool
		err         error
	}{
		{name: "empty-contract", err: currency.ErrCurrencyPairEmpty},
		{name: "invalid-order-size", contract: BTCUSDT, err: errInvalidOrderSize},
		{name: "bad-time-in-force", contract: BTCUSDT, size: 1, timeInForce: "bad", err: order.ErrUnsupportedTimeInForce},
		{name: "unsupported-poc-tif", contract: BTCUSDT, size: 1, timeInForce: pocTIF, err: order.ErrUnsupportedTimeInForce},
		{name: "invalid-text-prefix", contract: BTCUSDT, size: 1, timeInForce: iocTIF, text: "test", err: errInvalidTextPrefix},
		{name: "invalid-auto-size", contract: BTCUSDT, size: 1, timeInForce: iocTIF, text: "t-test", autoSize: "silly_billy", err: errInvalidAutoSize},
		{name: "size-nonzero-with-auto-size", contract: BTCUSDT, size: 1, timeInForce: iocTIF, text: "t-test", autoSize: "close_long", err: errInvalidOrderSize},
		{name: "rest-missing-settle", contract: BTCUSDT, timeInForce: iocTIF, text: "t-test", autoSize: "close_long", isRest: true, err: errEmptyOrInvalidSettlementCurrency},
		{name: "ws-invalid-settle", contract: BTCUSDT, timeInForce: iocTIF, text: "t-test", autoSize: "close_long", settle: currency.NewCode("Silly"), err: errEmptyOrInvalidSettlementCurrency},
		{name: "valid", contract: BTCUSDT, timeInForce: iocTIF, text: "t-test", autoSize: "close_long", settle: currency.USDT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fp := &FuturesOrderCreateParams{
				Contract: tc.contract, Size: tc.size, TimeInForce: tc.timeInForce,
				Text: tc.text, AutoSize: tc.autoSize, Settle: tc.settle,
			}
			assert.ErrorIs(t, fp.validate(tc.isRest), tc.err, "FuturesOrderCreateParams validate should return expected error")

			dp := &DeliveryOrderCreateParams{
				Contract: tc.contract, Size: tc.size, TimeInForce: tc.timeInForce,
				Text: tc.text, AutoSize: tc.autoSize, Settle: tc.settle,
			}
			assert.ErrorIs(t, dp.validate(tc.isRest), tc.err, "DeliveryOrderCreateParams validate should return expected error")
		})
	}
}

func TestUnmarshalJSONOrderbookLevels(t *testing.T) {
	t.Parallel()
	var ob OrderbookLevels
	require.NoError(t, ob.UnmarshalJSON([]byte(`[{"p":"123.45","s":"0.001"}]`)))
	assert.Equal(t, 123.45, ob[0].Price, "Price should be correct")
	assert.Equal(t, 0.001, ob[0].Amount, "Amount should be correct")

	require.Error(t, ob.UnmarshalJSON([]byte(`["p":"123.45","s":"0.001"]`)))
}
