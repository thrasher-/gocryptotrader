package hyperliquid

import (
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
)

const (
	// testBuilderAddress and testValidatorAddress are the placeholder builder and validator of the mock responses
	testBuilderAddress   = "0x0000000000000000000000000000000000000015"
	testValidatorAddress = "0x0000000000000000000000000000000000000016"
	// testPriorityClientOrderID is the client order ID of the mock responses' priority order
	testPriorityClientOrderID = "0x000000000000000000065d27f23d74a0"
	defaultActionResponse     = `{"status":"ok","response":{"type":"default"}}`
)

// newActionServerExchange returns an exchange with the given credentials whose exchange requests are decoded into
// captured and answered with response; info requests receive a user role, or an owned subaccount role for
// testSubAccountAddress, the xyz DEX registry, USDC collateral metadata and the USDC and HYPE spot tokens
func newActionServerExchange(t *testing.T, credentials *accounts.Credentials, captured *SignedActionRequest, response string) *Exchange {
	t.Helper()
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := response
		switch r.URL.Path {
		case "/info":
			var infoRequest InfoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&infoRequest), "Decoding the info request should not error") {
				return
			}
			switch infoRequest.Type {
			case "userRole":
				body = `{"role":"user"}`
				if infoRequest.User == testSubAccountAddress {
					body = `{"role":"subAccount","data":{"master":"` + testAccountAddress + `"}}`
				}
			case "perpDexs":
				body = `[null,{"name":"xyz"}]`
			case "meta":
				body = `{"universe":[],"collateralToken":0}`
			case "spotMeta":
				body = `{"universe":[],"tokens":[{"name":"USDC","index":0,"tokenId":"0x6d1e7cde53ba9467b783cb7c530ce054"},{"name":"HYPE","index":150,"tokenId":"0x0d01dc56dcaaca66ad901c959b4011ec"}]}`
			default:
				http.Error(w, "unexpected info request "+infoRequest.Type, http.StatusBadRequest)
				return
			}
		case "/exchange":
			*captured = SignedActionRequest{}
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(captured), "Decoding the exchange request should not error") {
				return
			}
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		_, err := w.Write([]byte(body))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	ex.Name = t.Name()
	setTestCredentials(ex, credentials)
	return ex
}

// signingCredentials returns credentials holding the mock signing account's own key
func signingCredentials() *accounts.Credentials {
	return &accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey}
}

// subAccountCredentials returns signing credentials that act for testSubAccountAddress
func subAccountCredentials() *accounts.Credentials {
	return &accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey, SubAccount: testSubAccountAddress}
}

// capturedAction returns a captured request's action as JSON
func capturedAction(t *testing.T, captured *SignedActionRequest) string {
	t.Helper()
	encoded, err := json.Marshal(captured.Action)
	require.NoError(t, err, "Marshal must not error")
	return string(encoded)
}

// skipUnlessMockTesting skips the part of a test that moves funds or changes the account irreversibly, which only runs
// against the mock responses
func skipUnlessMockTesting(t *testing.T) {
	t.Helper()
	if !mockTests {
		t.Skip("This action moves funds or changes the account irreversibly, so it only runs against the mock responses")
	}
}

func TestPlaceOrders(t *testing.T) {
	t.Parallel()
	limit := OrderRequest{Asset: 0, IsBuy: true, Price: 85000, Size: 0.001, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}, ClientOrderID: testClientOrderID}
	immediate := OrderRequest{Asset: 0, IsBuy: true, Price: 86000, Size: 0.002, Limit: &LimitOrderType{TimeInForce: TimeInForceIOC}}
	addLiquidity := OrderRequest{Asset: 0, IsBuy: true, Price: 84000, Size: 0.002, Limit: &LimitOrderType{TimeInForce: TimeInForceALO}}
	for _, tc := range []struct {
		name string
		arg  *PlaceOrdersRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "no orders", arg: &PlaceOrdersRequest{}, err: errNoActionItems},
		{name: "unknown grouping", arg: &PlaceOrdersRequest{Orders: []OrderRequest{limit}, Grouping: "other"}, err: errInvalidGrouping},
		{name: "invalid order", arg: &PlaceOrdersRequest{Orders: []OrderRequest{{Price: 1}}}, err: order.ErrAmountIsInvalid},
		{name: "priority rate above the maximum", arg: &PlaceOrdersRequest{Orders: []OrderRequest{immediate}, PriorityRate: maximumPriorityRate + 1}, err: errPriorityRateInvalid},
		{name: "priority with TP/SL grouping", arg: &PlaceOrdersRequest{Orders: []OrderRequest{immediate}, Grouping: GroupingNormalTPSL, PriorityRate: 1}, err: errPriorityGroupingInvalid},
		{name: "priority on an outcome", arg: &PlaceOrdersRequest{Orders: []OrderRequest{{Asset: outcomeAssetIDBase, Price: 0.5, Size: 1, Limit: &LimitOrderType{TimeInForce: TimeInForceIOC}}}, PriorityRate: 1}, err: errPriorityGroupingInvalid},
		{name: "priority on a trigger order", arg: &PlaceOrdersRequest{Orders: []OrderRequest{{Price: 1, Size: 1, Trigger: &TriggerOrderType{TriggerPrice: 1, TakeProfitStopLoss: TriggerStopLoss}}}, PriorityRate: 1}, err: errPriorityGroupingInvalid},
		{name: "priority on a GTC order", arg: &PlaceOrdersRequest{Orders: []OrderRequest{limit}, PriorityRate: 1}, err: errPriorityGroupingInvalid},
		{name: "priority on a reduce-only ALO order", arg: &PlaceOrdersRequest{Orders: []OrderRequest{{Price: 1, Size: 1, ReduceOnly: true, Limit: &LimitOrderType{TimeInForce: TimeInForceALO}}}, PriorityRate: 1}, err: errPriorityGroupingInvalid},
		{name: "priority on mixed IOC and ALO orders", arg: &PlaceOrdersRequest{Orders: []OrderRequest{immediate, addLiquidity}, PriorityRate: 1}, err: errPriorityGroupingInvalid},
		{name: "invalid builder", arg: &PlaceOrdersRequest{Orders: []OrderRequest{limit}, Builder: &BuilderFee{Builder: "invalid"}}, err: errInvalidAddress},
		{name: "perpetual builder fee above the maximum", arg: &PlaceOrdersRequest{Orders: []OrderRequest{limit}, Builder: &BuilderFee{Builder: testBuilderAddress, Fee: maximumPerpetualBuilderFee + 1}}, err: errBuilderFeeInvalid},
		{name: "expired action", arg: &PlaceOrdersRequest{Orders: []OrderRequest{limit}, ExpiresAfter: time.Now().Add(-time.Second)}, err: errExpiresAfterPassed},
	} {
		_, err := e.PlaceOrders(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "PlaceOrders should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, statusesResponse("order", `{"resting":{"oid":1}}`))
	expiresAfter := time.Now().Add(time.Minute)
	spot := OrderRequest{Asset: 10107, Price: 95.5, Size: 1.5, Limit: &LimitOrderType{TimeInForce: TimeInForceALO}}
	_, err := ex.PlaceOrders(t.Context(), &PlaceOrdersRequest{Orders: []OrderRequest{spot}, PriorityRate: 5, Builder: &BuilderFee{Builder: strings.ToUpper(testBuilderAddress[2:]), Fee: maximumSpotBuilderFee}, ExpiresAfter: expiresAfter})
	require.ErrorIs(t, err, errInvalidAddress, "PlaceOrders must reject a builder address without the 0x prefix")
	statuses, err := ex.PlaceOrders(t.Context(), &PlaceOrdersRequest{Orders: []OrderRequest{spot}, PriorityRate: 5, Builder: &BuilderFee{Builder: "0X" + strings.ToUpper(testBuilderAddress[2:]), Fee: maximumSpotBuilderFee}, ExpiresAfter: expiresAfter})
	require.NoError(t, err, "PlaceOrders must not error for a spot order with the spot builder fee maximum")
	assert.Equal(t, []OrderActionStatus{{Resting: &RestingOrderStatus{OrderID: 1}}}, statuses, "PlaceOrders should return the order's status")
	assert.JSONEq(t, `{"type":"order","orders":[{"a":10107,"b":false,"p":"95.5","s":"1.5","r":false,"t":{"limit":{"tif":"Alo"}}}],"grouping":{"p":5},"builder":{"b":"`+testBuilderAddress+`","f":1000}}`, capturedAction(t, &captured), "PlaceOrders should send a priority grouping and the lower-case builder")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "PlaceOrders should act for the configured subaccount")
	require.NotNil(t, captured.ExpiresAfter, "PlaceOrders must send the expiry")
	assert.Equal(t, unixMilli(expiresAfter), *captured.ExpiresAfter, "PlaceOrders should send the expiry")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	statuses, err = e.PlaceOrders(t.Context(), &PlaceOrdersRequest{
		Orders: []OrderRequest{
			limit,
			{Asset: 0, Price: 81000, Size: 0.001, ReduceOnly: true, Trigger: &TriggerOrderType{IsMarket: true, TriggerPrice: 90000, TakeProfitStopLoss: TriggerTakeProfit}},
			{Asset: 0, Price: 79000, Size: 0.001, ReduceOnly: true, Trigger: &TriggerOrderType{TriggerPrice: 80000, TakeProfitStopLoss: TriggerStopLoss}},
		},
		Grouping: GroupingNormalTPSL,
	})
	require.NoError(t, err, "PlaceOrders must not error for a bracket order")
	if mockTests {
		exp := []OrderActionStatus{
			{Resting: &RestingOrderStatus{OrderID: testOrderID, ClientOrderID: testClientOrderID}},
			{Waiting: orderStatusWaitingForFill},
			{Waiting: orderStatusWaitingForFill},
		}
		assert.Equal(t, exp, statuses, "PlaceOrders should return the parent's resting status and its children's waiting statuses")

		statuses, err = e.PlaceOrders(t.Context(), &PlaceOrdersRequest{
			Orders: []OrderRequest{
				{Asset: 0, IsBuy: true, Price: 86000, Size: 0.002, Limit: &LimitOrderType{TimeInForce: TimeInForceIOC}, ClientOrderID: testPriorityClientOrderID},
				{Asset: 1, Price: 2700, Size: 0.05, Limit: &LimitOrderType{TimeInForce: TimeInForceIOC}},
			},
			PriorityRate: 1000,
		})
		require.NoError(t, err, "PlaceOrders must not error for priority orders")
		exp = []OrderActionStatus{
			{Filled: &FilledOrderStatus{TotalSize: 0.002, AveragePrice: 85813.5, OrderID: 566563303545, ClientOrderID: testPriorityClientOrderID}},
			{Error: "Order could not immediately match against any resting orders. asset=1"},
		}
		assert.Equal(t, exp, statuses, "PlaceOrders should return each priority order's status")
	}
}

func TestCancelOrdersByOrderID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *CancelOrdersRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "no cancels", arg: &CancelOrdersRequest{}, err: errNoActionItems},
		{name: "a zero order ID", arg: &CancelOrdersRequest{Cancels: []CancelRequest{{Asset: 1}}}, err: order.ErrOrderIDNotSet},
	} {
		_, err := e.CancelOrders(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "CancelOrders should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, statusesResponse("cancel", `"success"`))
	_, err := ex.CancelOrders(t.Context(), &CancelOrdersRequest{Cancels: []CancelRequest{{Asset: 1, OrderID: 2}}})
	require.NoError(t, err, "CancelOrders must not error")
	assert.JSONEq(t, `{"type":"cancel","cancels":[{"a":1,"o":2}]}`, capturedAction(t, &captured), "CancelOrders should omit the fast flag unless it is set")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "CancelOrders should act for the configured subaccount")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	statuses, err := e.CancelOrders(t.Context(), &CancelOrdersRequest{Cancels: []CancelRequest{{Asset: 0, OrderID: testOrderID}, {Asset: 1, OrderID: 566563303543}}, Fast: true})
	require.NoError(t, err, "CancelOrders must not error")
	if mockTests {
		assert.Equal(t, []CancelActionStatus{{}, {Error: "Order was never placed, already canceled, or filled. asset=1"}}, statuses, "CancelOrders should return each cancel's status")
	}
}

func TestCancelOrdersByClientOrderID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *CancelOrdersByClientOrderIDRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "no cancels", arg: &CancelOrdersByClientOrderIDRequest{}, err: errNoActionItems},
		{name: "an invalid client order ID", arg: &CancelOrdersByClientOrderIDRequest{Cancels: []CancelByClientOrderIDRequest{{ClientOrderID: "0x1"}}}, err: errClientOrderIDInvalid},
	} {
		_, err := e.CancelOrdersByClientOrderID(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "CancelOrdersByClientOrderID should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, signingCredentials(), &captured, statusesResponse("cancel", `"success"`))
	_, err := ex.CancelOrdersByClientOrderID(t.Context(), &CancelOrdersByClientOrderIDRequest{Cancels: []CancelByClientOrderIDRequest{{Asset: 1, ClientOrderID: strings.ToUpper(testClientOrderID)}}, Fast: true})
	require.NoError(t, err, "CancelOrdersByClientOrderID must not error")
	assert.JSONEq(t, `{"type":"cancelByCloid","cancels":[{"asset":1,"cloid":"`+testClientOrderID+`"}],"f":true}`, capturedAction(t, &captured), "CancelOrdersByClientOrderID should send lower-case client order IDs and the fast flag")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	statuses, err := e.CancelOrdersByClientOrderID(t.Context(), &CancelOrdersByClientOrderIDRequest{Cancels: []CancelByClientOrderIDRequest{{Asset: 0, ClientOrderID: testClientOrderID}}, Fast: true})
	require.NoError(t, err, "CancelOrdersByClientOrderID must not error")
	if mockTests {
		assert.Equal(t, []CancelActionStatus{{}}, statuses, "CancelOrdersByClientOrderID should return the cancel's status")
	}
}

func TestScheduleCancel(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.ScheduleCancel(t.Context(), nil), common.ErrNilPointer, "ScheduleCancel must reject a nil request")
	require.ErrorIs(t, e.ScheduleCancel(t.Context(), &ScheduleCancelRequest{Time: time.Now().Add(time.Second)}), errScheduleCancelTimeInvalid, "ScheduleCancel must reject a time less than 5 seconds ahead")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	cancelTime := time.Now().Add(time.Minute)
	require.NoError(t, ex.ScheduleCancel(t.Context(), &ScheduleCancelRequest{Time: cancelTime}), "ScheduleCancel must not error")
	assert.JSONEq(t, `{"type":"scheduleCancel","time":`+strconv.FormatUint(unixMilli(cancelTime), 10)+`}`, capturedAction(t, &captured), "ScheduleCancel should send the cancel time")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "ScheduleCancel should act for the configured subaccount")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	assert.NoError(t, e.ScheduleCancel(t.Context(), &ScheduleCancelRequest{}), "ScheduleCancel should not error when clearing the schedule")
}

func TestModifySingleOrder(t *testing.T) {
	t.Parallel()
	replacement := OrderRequest{Asset: 0, IsBuy: true, Price: 84500, Size: 0.001, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}, ClientOrderID: testClientOrderID}
	for _, tc := range []struct {
		name string
		arg  *ModifyOrderRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "no identifier", arg: &ModifyOrderRequest{Order: replacement}, err: order.ErrOrderIDNotSet},
		{name: "both identifiers", arg: &ModifyOrderRequest{OrderID: 1, ClientOrderID: testClientOrderID, Order: replacement}, err: errOrderIdentifiersConflict},
		{name: "an invalid client order ID", arg: &ModifyOrderRequest{ClientOrderID: "0x1", Order: replacement}, err: errClientOrderIDInvalid},
		{name: "an invalid replacement", arg: &ModifyOrderRequest{OrderID: 1, Order: OrderRequest{Price: 1, Size: 1}}, err: errOrderTypeRequired},
	} {
		assert.ErrorIsf(t, e.ModifySingleOrder(t.Context(), tc.arg), tc.err, "ModifySingleOrder should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse)
	require.NoError(t, ex.ModifySingleOrder(t.Context(), &ModifyOrderRequest{ClientOrderID: strings.ToUpper(testClientOrderID), Order: replacement}), "ModifySingleOrder must not error")
	assert.JSONEq(t, `{"type":"modify","oid":"`+testClientOrderID+`","order":{"a":0,"b":true,"p":"84500","s":"0.001","r":false,"t":{"limit":{"tif":"Gtc"}},"c":"`+testClientOrderID+`"}}`, capturedAction(t, &captured), "ModifySingleOrder should omit always place unless it is set")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	assert.NoError(t, e.ModifySingleOrder(t.Context(), &ModifyOrderRequest{OrderID: testOrderID, Order: replacement, AlwaysPlace: true}), "ModifySingleOrder should not error")
}

func TestModifyOrders(t *testing.T) {
	t.Parallel()
	replacement := OrderRequest{Asset: 0, Price: 79500, Size: 0.001, ReduceOnly: true, Trigger: &TriggerOrderType{IsMarket: true, TriggerPrice: 80500, TakeProfitStopLoss: TriggerStopLoss}}
	for _, tc := range []struct {
		name string
		arg  *ModifyOrdersRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "no modifies", arg: &ModifyOrdersRequest{}, err: errNoActionItems},
		{name: "both identifiers", arg: &ModifyOrdersRequest{Modifies: []OrderModification{{OrderID: 1, ClientOrderID: testClientOrderID, Order: replacement}}}, err: errOrderIdentifiersConflict},
		{name: "an invalid replacement", arg: &ModifyOrdersRequest{Modifies: []OrderModification{{OrderID: 1}}}, err: order.ErrAmountIsInvalid},
	} {
		_, err := e.ModifyOrders(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "ModifyOrders should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, statusesResponse("order", `{"resting":{"oid":2}}`))
	_, err := ex.ModifyOrders(t.Context(), &ModifyOrdersRequest{Modifies: []OrderModification{{OrderID: 1, Order: replacement}}, AlwaysPlace: true})
	require.NoError(t, err, "ModifyOrders must not error")
	assert.JSONEq(t, `{"type":"batchModify","modifies":[{"oid":1,"order":{"a":0,"b":false,"p":"79500","s":"0.001","r":true,"t":{"trigger":{"isMarket":true,"triggerPx":"80500","tpsl":"sl"}}}}],"a":true}`, capturedAction(t, &captured), "ModifyOrders should send always place")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "ModifyOrders should act for the configured subaccount")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	statuses, err := e.ModifyOrders(t.Context(), &ModifyOrdersRequest{Modifies: []OrderModification{{ClientOrderID: testClientOrderID, Order: replacement}}, AlwaysPlace: true})
	require.NoError(t, err, "ModifyOrders must not error")
	if mockTests {
		assert.Equal(t, []OrderActionStatus{{Resting: &RestingOrderStatus{OrderID: 566563303546}}}, statuses, "ModifyOrders should return the replacement's status")
	}
}

func TestUpdateLeverage(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.UpdateLeverage(t.Context(), nil), common.ErrNilPointer, "UpdateLeverage must reject a nil request")
	require.ErrorIs(t, e.UpdateLeverage(t.Context(), &UpdateLeverageRequest{}), errInvalidLeverage, "UpdateLeverage must reject zero leverage")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	require.NoError(t, ex.UpdateLeverage(t.Context(), &UpdateLeverageRequest{Asset: 1, Leverage: 5, ExpiresAfter: time.Now().Add(time.Minute)}), "UpdateLeverage must not error")
	assert.JSONEq(t, `{"type":"updateLeverage","asset":1,"isCross":false,"leverage":5}`, capturedAction(t, &captured), "UpdateLeverage should send the leverage")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "UpdateLeverage should act for the configured subaccount")
	assert.NotNil(t, captured.ExpiresAfter, "UpdateLeverage should send the expiry")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	assert.NoError(t, e.UpdateLeverage(t.Context(), &UpdateLeverageRequest{Asset: 0, IsCross: true, Leverage: 20}), "UpdateLeverage should not error")
}

func TestUpdateIsolatedMargin(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.UpdateIsolatedMargin(t.Context(), nil), common.ErrNilPointer, "UpdateIsolatedMargin must reject a nil request")
	require.ErrorIs(t, e.UpdateIsolatedMargin(t.Context(), &UpdateIsolatedMarginRequest{}), errMarginChangeInvalid, "UpdateIsolatedMargin must reject a zero margin change")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	require.NoError(t, ex.UpdateIsolatedMargin(t.Context(), &UpdateIsolatedMarginRequest{Asset: 1, SignedNotional: -1000000}), "UpdateIsolatedMargin must not error")
	assert.JSONEq(t, `{"type":"updateIsolatedMargin","asset":1,"isBuy":true,"ntli":-1000000}`, capturedAction(t, &captured), "UpdateIsolatedMargin should send a signed margin change")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "UpdateIsolatedMargin should act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.UpdateIsolatedMargin(t.Context(), &UpdateIsolatedMarginRequest{Asset: 0, SignedNotional: 2000000}), "UpdateIsolatedMargin should not error")
}

func TestTopUpIsolatedOnlyMargin(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.TopUpIsolatedOnlyMargin(t.Context(), nil), common.ErrNilPointer, "TopUpIsolatedOnlyMargin must reject a nil request")
	for _, leverage := range []float64{0, -1, math.NaN()} {
		assert.ErrorIsf(t, e.TopUpIsolatedOnlyMargin(t.Context(), &TopUpIsolatedOnlyMarginRequest{Leverage: leverage}), errTargetLeverageInvalid, "TopUpIsolatedOnlyMargin should reject leverage %v", leverage)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	require.NoError(t, ex.TopUpIsolatedOnlyMargin(t.Context(), &TopUpIsolatedOnlyMarginRequest{Asset: 3, Leverage: 0.5}), "TopUpIsolatedOnlyMargin must not error")
	assert.JSONEq(t, `{"type":"topUpIsolatedOnlyMargin","asset":3,"leverage":"0.5"}`, capturedAction(t, &captured), "TopUpIsolatedOnlyMargin should send the target leverage as a decimal string")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "TopUpIsolatedOnlyMargin should act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.TopUpIsolatedOnlyMargin(t.Context(), &TopUpIsolatedOnlyMarginRequest{Asset: 3, Leverage: 1.5}), "TopUpIsolatedOnlyMargin should not error")
}

func TestSendAsset(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *SendAssetRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "an invalid destination", arg: &SendAssetRequest{Destination: "invalid", Amount: 1}, err: errInvalidAddress},
		{name: "no amount", arg: &SendAssetRequest{Destination: testWithdrawalAddress}, err: errTransferAmountInvalid},
	} {
		_, err := e.SendAsset(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "SendAsset should reject a request with %s", tc.name)
	}
	agent := newActionServerExchange(t, &accounts.Credentials{Key: testAccountAddress, Secret: testAgentPrivateKey}, new(SignedActionRequest), defaultActionResponse)
	_, err := agent.SendAsset(t.Context(), &SendAssetRequest{Destination: testWithdrawalAddress, Token: currency.USDC, Amount: 1})
	assert.ErrorIs(t, err, errUserSignedMasterRequired, "SendAsset should require the account's own key")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	for _, tc := range []struct {
		name string
		arg  *SendAssetRequest
		err  error
	}{
		{name: "an unregistered DEX", arg: &SendAssetRequest{Destination: testWithdrawalAddress, SourceDEX: "abc", Token: currency.USDC, Amount: 1}, err: errTransferDEXInvalid},
		{name: "a token that is not the DEX's collateral", arg: &SendAssetRequest{Destination: testWithdrawalAddress, DestinationDEX: "xyz", Token: currency.HYPE, Amount: 1}, err: errTransferTokenInvalid},
	} {
		_, err := ex.SendAsset(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "SendAsset should reject a request with %s", tc.name)
	}
	nonce, err := ex.SendAsset(t.Context(), &SendAssetRequest{Destination: testWithdrawalAddress, SourceDEX: "SPOT", DestinationDEX: "xyz", Token: currency.USDC, Amount: 1.5})
	require.NoError(t, err, "SendAsset must not error")
	exp := `{"type":"sendAsset","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","destination":"` + testWithdrawalAddress + `","sourceDex":"spot","destinationDex":"xyz","token":"USDC","amount":"1.5","fromSubAccount":"` + testSubAccountAddress + `","nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "SendAsset should send from the configured subaccount")
	assert.Empty(t, captured.VaultAddress, "SendAsset should not send a vault address")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).SendAsset(t.Context(), &SendAssetRequest{Destination: testWithdrawalAddress, DestinationDEX: "spot", Token: currency.USDC, Amount: 10})
	require.NoError(t, err, "SendAsset must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "SendAsset should return the action's nonce")
}

func TestAgentSendAsset(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *AgentSendAssetRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "an invalid destination", arg: &AgentSendAssetRequest{Destination: "invalid", Amount: 1}, err: errInvalidAddress},
		{name: "no amount", arg: &AgentSendAssetRequest{Destination: testWithdrawalAddress}, err: errTransferAmountInvalid},
	} {
		_, err := e.AgentSendAsset(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "AgentSendAsset should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	_, err := ex.AgentSendAsset(t.Context(), &AgentSendAssetRequest{Destination: testWithdrawalAddress, SourceDEX: "xyz", Token: currency.HYPE, Amount: 1})
	assert.ErrorIs(t, err, errTransferTokenInvalid, "AgentSendAsset should reject a token that is not the DEX's collateral")
	_, err = ex.AgentSendAsset(t.Context(), &AgentSendAssetRequest{Destination: testWithdrawalAddress, Token: currency.USDC, Amount: 1, ExpiresAfter: time.Now().Add(-time.Second)})
	assert.ErrorIs(t, err, errExpiresAfterPassed, "AgentSendAsset should reject a passed expiry")
	expiresAfter := time.Now().Add(time.Minute)
	nonce, err := ex.AgentSendAsset(t.Context(), &AgentSendAssetRequest{Destination: testWithdrawalAddress, SourceDEX: "xyz", DestinationDEX: "spot", Token: currency.USDC, Amount: 2, ExpiresAfter: expiresAfter})
	require.NoError(t, err, "AgentSendAsset must not error")
	exp := `{"type":"agentSendAsset","destination":"` + testWithdrawalAddress + `","sourceDex":"xyz","destinationDex":"spot","token":"USDC","amount":"2","fromSubAccount":"` + testSubAccountAddress + `","nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "AgentSendAsset should sign the request nonce inside the action")
	assert.Equal(t, nonce, captured.Nonce, "AgentSendAsset should send the action's nonce as the request nonce")
	assert.Empty(t, captured.VaultAddress, "AgentSendAsset should not act for the vault")
	assert.Equal(t, new(unixMilli(expiresAfter)), captured.ExpiresAfter, "AgentSendAsset should send the expiry")
	action := AgentSendAssetAction{Type: "agentSendAsset", Destination: testWithdrawalAddress, SourceDEX: "xyz", DestinationDEX: "spot", Token: "USDC", Amount: "2", FromSubAccount: testSubAccountAddress, Nonce: nonce}
	signature, err := signL1Action(testPrivateKey, action, "", nonce, new(unixMilli(expiresAfter)), true)
	require.NoError(t, err, "signL1Action must not error")
	assert.Equal(t, signature, captured.Signature, "AgentSendAsset should sign the action holding its nonce, with the expiry and without the vault")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).AgentSendAsset(t.Context(), &AgentSendAssetRequest{Destination: testWithdrawalAddress, SourceDEX: "spot", Token: currency.USDC, Amount: 10})
	require.NoError(t, err, "AgentSendAsset must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "AgentSendAsset should return the action's nonce")
}

func TestSendToEVMWithData(t *testing.T) {
	t.Parallel()
	valid := SendToEVMWithDataRequest{Token: currency.USDC, Amount: 5, SourceDEX: "spot", DestinationRecipient: testWithdrawalAddress, AddressEncoding: AddressEncodingHex, DestinationChainID: 999, GasLimit: 200000}
	for _, tc := range []struct {
		name   string
		mutate func(*SendToEVMWithDataRequest)
		err    error
	}{
		{name: "no amount", mutate: func(r *SendToEVMWithDataRequest) { r.Amount = 0 }, err: errTransferAmountInvalid},
		{name: "an unknown address encoding", mutate: func(r *SendToEVMWithDataRequest) { r.AddressEncoding = "bech32" }, err: errAddressEncodingInvalid},
		{name: "an unprefixed hex recipient", mutate: func(r *SendToEVMWithDataRequest) { r.DestinationRecipient = testWithdrawalAddress[2:] }, err: errDestinationRecipientInvalid},
		{name: "an invalid hex recipient", mutate: func(r *SendToEVMWithDataRequest) { r.DestinationRecipient = "0xzz" }, err: errDestinationRecipientInvalid},
		{name: "no base58 recipient", mutate: func(r *SendToEVMWithDataRequest) {
			r.AddressEncoding = AddressEncodingBase58
			r.DestinationRecipient = " "
		}, err: errDestinationRecipientInvalid},
	} {
		arg := valid
		tc.mutate(&arg)
		_, err := e.SendToEVMWithData(t.Context(), &arg)
		assert.ErrorIsf(t, err, tc.err, "SendToEVMWithData should reject a request with %s", tc.name)
	}
	_, err := e.SendToEVMWithData(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "SendToEVMWithData must reject a nil request")
	_, err = newActionServerExchange(t, subAccountCredentials(), new(SignedActionRequest), defaultActionResponse).SendToEVMWithData(t.Context(), &valid)
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "SendToEVMWithData must reject a configured subaccount")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse)
	for _, tc := range []struct {
		name   string
		mutate func(*SendToEVMWithDataRequest)
		err    error
	}{
		{name: "an unregistered DEX", mutate: func(r *SendToEVMWithDataRequest) { r.SourceDEX = "abc" }, err: errTransferDEXInvalid},
		{name: "an unknown token", mutate: func(r *SendToEVMWithDataRequest) { r.Token = currency.BTC }, err: errTransferTokenInvalid},
	} {
		arg := valid
		tc.mutate(&arg)
		_, err := ex.SendToEVMWithData(t.Context(), &arg)
		assert.ErrorIsf(t, err, tc.err, "SendToEVMWithData should reject a request with %s", tc.name)
	}
	base58 := valid
	base58.AddressEncoding = AddressEncodingBase58
	base58.DestinationRecipient = "GCTRecipient11111111111111111111111111111111"
	nonce, err := ex.SendToEVMWithData(t.Context(), &base58)
	require.NoError(t, err, "SendToEVMWithData must not error for a base58 recipient")
	exp := `{"type":"sendToEvmWithData","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","token":"USDC:0x6d1e7cde53ba9467b783cb7c530ce054","amount":"5","sourceDex":"spot","destinationRecipient":"GCTRecipient11111111111111111111111111111111","addressEncoding":"base58","destinationChainId":999,"gasLimit":200000,"data":"0x","nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "SendToEVMWithData should send empty data as 0x and keep a base58 recipient")

	skipUnlessMockTesting(t)
	arg := valid
	arg.DestinationRecipient = "0x" + strings.ToUpper(testWithdrawalAddress[2:])
	arg.Data = []byte{0xde, 0xad, 0xbe, 0xef}
	nonce, err = newUserSignedTestExchange(t).SendToEVMWithData(t.Context(), &arg)
	require.NoError(t, err, "SendToEVMWithData must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "SendToEVMWithData should return the action's nonce")
}

func TestSendCoreUSDC(t *testing.T) {
	t.Parallel()
	_, err := e.SendCoreUSDC(t.Context(), "invalid", 1)
	require.ErrorIs(t, err, errInvalidAddress, "SendCoreUSDC must reject an invalid destination")
	_, err = e.SendCoreUSDC(t.Context(), testWithdrawalAddress, 0)
	require.ErrorIs(t, err, errTransferAmountInvalid, "SendCoreUSDC must reject a zero amount")
	_, err = newActionServerExchange(t, subAccountCredentials(), new(SignedActionRequest), defaultActionResponse).SendCoreUSDC(t.Context(), testWithdrawalAddress, 1)
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "SendCoreUSDC must reject a configured subaccount")

	var captured SignedActionRequest
	nonce, err := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse).SendCoreUSDC(t.Context(), testWithdrawalAddress, 1.5)
	require.NoError(t, err, "SendCoreUSDC must not error")
	exp := `{"type":"usdSend","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","destination":"` + testWithdrawalAddress + `","amount":"1.5","time":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "SendCoreUSDC should send the transfer with a time nonce")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).SendCoreUSDC(t.Context(), testWithdrawalAddress, 25)
	require.NoError(t, err, "SendCoreUSDC must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "SendCoreUSDC should return the action's nonce")
}

func TestSendCoreSpot(t *testing.T) {
	t.Parallel()
	_, err := e.SendCoreSpot(t.Context(), "invalid", currency.HYPE, 1)
	require.ErrorIs(t, err, errInvalidAddress, "SendCoreSpot must reject an invalid destination")
	_, err = e.SendCoreSpot(t.Context(), testWithdrawalAddress, currency.HYPE, 0)
	require.ErrorIs(t, err, errTransferAmountInvalid, "SendCoreSpot must reject a zero amount")
	_, err = newActionServerExchange(t, subAccountCredentials(), new(SignedActionRequest), defaultActionResponse).SendCoreSpot(t.Context(), testWithdrawalAddress, currency.HYPE, 1)
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "SendCoreSpot must reject a configured subaccount")
	_, err = newActionServerExchange(t, signingCredentials(), new(SignedActionRequest), defaultActionResponse).SendCoreSpot(t.Context(), testWithdrawalAddress, currency.BTC, 1)
	require.ErrorIs(t, err, errTransferTokenInvalid, "SendCoreSpot must reject an unknown token")

	var captured SignedActionRequest
	nonce, err := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse).SendCoreSpot(t.Context(), testWithdrawalAddress, currency.HYPE, 1.5)
	require.NoError(t, err, "SendCoreSpot must not error")
	exp := `{"type":"spotSend","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","destination":"` + testWithdrawalAddress + `","token":"HYPE:0x0d01dc56dcaaca66ad901c959b4011ec","amount":"1.5","time":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "SendCoreSpot should send the token by its NAME:TOKEN_ID identifier with a time nonce")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).SendCoreSpot(t.Context(), testWithdrawalAddress, currency.HYPE, 2.5)
	require.NoError(t, err, "SendCoreSpot must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "SendCoreSpot should return the action's nonce")
}

func TestWithdrawFromBridge(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawFromBridge(t.Context(), "invalid", 1)
	require.ErrorIs(t, err, errInvalidAddress, "WithdrawFromBridge must reject an invalid destination")
	_, err = e.WithdrawFromBridge(t.Context(), testWithdrawalAddress, -1)
	require.ErrorIs(t, err, errTransferAmountInvalid, "WithdrawFromBridge must reject a negative amount")
	_, err = newActionServerExchange(t, subAccountCredentials(), new(SignedActionRequest), defaultActionResponse).WithdrawFromBridge(t.Context(), testWithdrawalAddress, 1)
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "WithdrawFromBridge must reject a configured subaccount")

	var captured SignedActionRequest
	nonce, err := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse).WithdrawFromBridge(t.Context(), testWithdrawalAddress, 1.5)
	require.NoError(t, err, "WithdrawFromBridge must not error")
	exp := `{"type":"withdraw3","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","destination":"` + testWithdrawalAddress + `","amount":"1.5","time":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "WithdrawFromBridge should send the withdrawal with a time nonce")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).WithdrawFromBridge(t.Context(), testWithdrawalAddress, 25)
	require.NoError(t, err, "WithdrawFromBridge must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "WithdrawFromBridge should return the action's nonce")
}

func TestTransferUSDCBetweenSpotAndPerp(t *testing.T) {
	t.Parallel()
	_, err := e.TransferUSDCBetweenSpotAndPerp(t.Context(), 0, true)
	require.ErrorIs(t, err, errTransferAmountInvalid, "TransferUSDCBetweenSpotAndPerp must reject a zero amount")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	nonce, err := ex.TransferUSDCBetweenSpotAndPerp(t.Context(), 1, false)
	require.NoError(t, err, "TransferUSDCBetweenSpotAndPerp must not error for a subaccount")
	exp := `{"type":"usdClassTransfer","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","amount":"1 subaccount:` + testSubAccountAddress + `","toPerp":false,"nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "TransferUSDCBetweenSpotAndPerp should name the subaccount in the amount")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).TransferUSDCBetweenSpotAndPerp(t.Context(), 100, true)
	require.NoError(t, err, "TransferUSDCBetweenSpotAndPerp must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "TransferUSDCBetweenSpotAndPerp should return the action's nonce")
}

func TestDepositIntoStaking(t *testing.T) {
	t.Parallel()
	_, err := e.DepositIntoStaking(t.Context(), 0)
	require.ErrorIs(t, err, errStakeAmountInvalid, "DepositIntoStaking must reject a zero amount")
	_, err = newActionServerExchange(t, subAccountCredentials(), new(SignedActionRequest), defaultActionResponse).DepositIntoStaking(t.Context(), 1)
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "DepositIntoStaking must reject a configured subaccount")

	var captured SignedActionRequest
	nonce, err := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse).DepositIntoStaking(t.Context(), 1)
	require.NoError(t, err, "DepositIntoStaking must not error")
	assert.JSONEq(t, `{"type":"cDeposit","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","wei":1,"nonce":`+strconv.FormatUint(nonce, 10)+`}`, capturedAction(t, &captured), "DepositIntoStaking should send the amount in wei")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).DepositIntoStaking(t.Context(), 100000000)
	require.NoError(t, err, "DepositIntoStaking must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "DepositIntoStaking should return the action's nonce")
}

func TestWithdrawFromStaking(t *testing.T) {
	t.Parallel()
	_, err := e.WithdrawFromStaking(t.Context(), 0)
	require.ErrorIs(t, err, errStakeAmountInvalid, "WithdrawFromStaking must reject a zero amount")

	skipUnlessMockTesting(t)
	nonce, err := newUserSignedTestExchange(t).WithdrawFromStaking(t.Context(), 50000000)
	require.NoError(t, err, "WithdrawFromStaking must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "WithdrawFromStaking should return the action's nonce")
}

func TestDelegateStake(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *DelegateStakeRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "an invalid validator", arg: &DelegateStakeRequest{Validator: "invalid", Wei: 1}, err: errInvalidAddress},
		{name: "no amount", arg: &DelegateStakeRequest{Validator: testValidatorAddress}, err: errStakeAmountInvalid},
	} {
		_, err := e.DelegateStake(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "DelegateStake should reject a request with %s", tc.name)
	}
	_, err := newActionServerExchange(t, subAccountCredentials(), new(SignedActionRequest), defaultActionResponse).DelegateStake(t.Context(), &DelegateStakeRequest{Validator: testValidatorAddress, Wei: 1})
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "DelegateStake must reject a configured subaccount")

	var captured SignedActionRequest
	nonce, err := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse).DelegateStake(t.Context(), &DelegateStakeRequest{Validator: "0X" + strings.ToUpper(testValidatorAddress[2:]), Wei: 2, IsUndelegate: true})
	require.NoError(t, err, "DelegateStake must not error")
	exp := `{"type":"tokenDelegate","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","validator":"` + testValidatorAddress + `","wei":2,"isUndelegate":true,"nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "DelegateStake should send the lower-case validator")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).DelegateStake(t.Context(), &DelegateStakeRequest{Validator: testValidatorAddress, Wei: 100000000})
	require.NoError(t, err, "DelegateStake must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "DelegateStake should return the action's nonce")
}

func TestTransferVaultUSDC(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *VaultTransferRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "an invalid vault", arg: &VaultTransferRequest{VaultAddress: "invalid", USD: 1}, err: errInvalidAddress},
		{name: "no amount", arg: &VaultTransferRequest{VaultAddress: testVaultAddress}, err: errTransferAmountInvalid},
	} {
		assert.ErrorIsf(t, e.TransferVaultUSDC(t.Context(), tc.arg), tc.err, "TransferVaultUSDC should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	require.NoError(t, ex.TransferVaultUSDC(t.Context(), &VaultTransferRequest{VaultAddress: testVaultAddress, USD: 1000000}), "TransferVaultUSDC must not error")
	assert.JSONEq(t, `{"type":"vaultTransfer","vaultAddress":"`+testVaultAddress+`","isDeposit":false,"usd":1000000}`, capturedAction(t, &captured), "TransferVaultUSDC should send the vault inside the action")
	assert.Empty(t, captured.VaultAddress, "TransferVaultUSDC should not act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.TransferVaultUSDC(t.Context(), &VaultTransferRequest{VaultAddress: testVaultAddress, IsDeposit: true, USD: 5000000}), "TransferVaultUSDC should not error")
}

func TestTransferHIP3BackstopLiquidator(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *HIP3LiquidatorTransferRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "no amount", arg: &HIP3LiquidatorTransferRequest{DEX: "xyz"}, err: errLiquidatorTransferInvalid},
		{name: "a part of 1000 quote tokens", arg: &HIP3LiquidatorTransferRequest{DEX: "xyz", Notional: liquidatorTransferStep + 1}, err: errLiquidatorTransferInvalid},
		{name: "the default DEX", arg: &HIP3LiquidatorTransferRequest{Notional: liquidatorTransferStep}, err: errTransferDEXInvalid},
		{name: "spot", arg: &HIP3LiquidatorTransferRequest{DEX: "Spot", Notional: liquidatorTransferStep}, err: errTransferDEXInvalid},
	} {
		assert.ErrorIsf(t, e.TransferHIP3BackstopLiquidator(t.Context(), tc.arg), tc.err, "TransferHIP3BackstopLiquidator should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	require.ErrorIs(t, ex.TransferHIP3BackstopLiquidator(t.Context(), &HIP3LiquidatorTransferRequest{DEX: "abc", Notional: liquidatorTransferStep}), errTransferDEXInvalid, "TransferHIP3BackstopLiquidator must reject an unregistered DEX")
	require.NoError(t, ex.TransferHIP3BackstopLiquidator(t.Context(), &HIP3LiquidatorTransferRequest{DEX: "xyz", Notional: 2 * liquidatorTransferStep}), "TransferHIP3BackstopLiquidator must not error")
	assert.JSONEq(t, `{"type":"hip3LiquidatorTransfer","dex":"xyz","ntl":2000000000,"isDeposit":false}`, capturedAction(t, &captured), "TransferHIP3BackstopLiquidator should send the transfer")
	assert.Empty(t, captured.VaultAddress, "TransferHIP3BackstopLiquidator should not act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.TransferHIP3BackstopLiquidator(t.Context(), &HIP3LiquidatorTransferRequest{DEX: "xyz", Notional: liquidatorTransferStep, IsDeposit: true}), "TransferHIP3BackstopLiquidator should not error")
}

func TestApproveAgent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *ApproveAgentRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "an invalid agent", arg: &ApproveAgentRequest{AgentAddress: "invalid"}, err: errInvalidAddress},
		{name: "a long name", arg: &ApproveAgentRequest{AgentAddress: testAgentAddress, AgentName: strings.Repeat("a", maximumAgentNameLength+1)}, err: errAgentNameInvalid},
		{name: "an expiry without a name", arg: &ApproveAgentRequest{AgentAddress: testAgentAddress, ValidUntil: time.Now().Add(time.Hour)}, err: errAgentNameInvalid},
		{name: "a passed expiry", arg: &ApproveAgentRequest{AgentAddress: testAgentAddress, AgentName: "gct", ValidUntil: time.Now().Add(-time.Hour)}, err: errAgentValidUntilInvalid},
		{name: "an expiry beyond 180 days", arg: &ApproveAgentRequest{AgentAddress: testAgentAddress, AgentName: "gct", ValidUntil: time.Now().Add(maximumAgentValidity + time.Hour)}, err: errAgentValidUntilInvalid},
	} {
		_, err := e.ApproveAgent(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "ApproveAgent should reject a request with %s", tc.name)
	}
	_, err := newActionServerExchange(t, subAccountCredentials(), new(SignedActionRequest), defaultActionResponse).ApproveAgent(t.Context(), &ApproveAgentRequest{AgentAddress: testAgentAddress})
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "ApproveAgent must reject a configured subaccount")

	var captured SignedActionRequest
	validUntil := time.Now().Add(time.Hour)
	nonce, err := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse).ApproveAgent(t.Context(), &ApproveAgentRequest{AgentAddress: testAgentAddress, AgentName: strings.Repeat("é", maximumAgentNameLength), ValidUntil: validUntil})
	require.NoError(t, err, "ApproveAgent must not error for an expiring agent")
	exp := `{"type":"approveAgent","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","agentAddress":"` + testAgentAddress + `","agentName":"` + strings.Repeat("é", maximumAgentNameLength) + ` valid_until ` + strconv.FormatUint(unixMilli(validUntil), 10) + `","nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "ApproveAgent should append the expiry to the agent name")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).ApproveAgent(t.Context(), &ApproveAgentRequest{AgentAddress: testAgentAddress, AgentName: "gct"})
	require.NoError(t, err, "ApproveAgent must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "ApproveAgent should return the action's nonce")
}

func TestApproveBuilderFee(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *ApproveBuilderFeeRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "an invalid builder", arg: &ApproveBuilderFeeRequest{Builder: "invalid"}, err: errInvalidAddress},
		{name: "a negative rate", arg: &ApproveBuilderFeeRequest{Builder: testBuilderAddress, MaxFeeRate: -0.001}, err: errBuilderFeeRateInvalid},
		{name: "a rate that is not a number", arg: &ApproveBuilderFeeRequest{Builder: testBuilderAddress, MaxFeeRate: math.NaN()}, err: errBuilderFeeRateInvalid},
	} {
		_, err := e.ApproveBuilderFee(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "ApproveBuilderFee should reject a request with %s", tc.name)
	}
	_, err := newActionServerExchange(t, subAccountCredentials(), new(SignedActionRequest), defaultActionResponse).ApproveBuilderFee(t.Context(), &ApproveBuilderFeeRequest{Builder: testBuilderAddress})
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "ApproveBuilderFee must reject a configured subaccount")

	var captured SignedActionRequest
	nonce, err := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse).ApproveBuilderFee(t.Context(), &ApproveBuilderFeeRequest{Builder: testBuilderAddress, MaxFeeRate: 0.001})
	require.NoError(t, err, "ApproveBuilderFee must not error")
	exp := `{"type":"approveBuilderFee","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","maxFeeRate":"0.001%","builder":"` + testBuilderAddress + `","nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "ApproveBuilderFee should send the rate as a percentage")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).ApproveBuilderFee(t.Context(), &ApproveBuilderFeeRequest{Builder: testBuilderAddress, MaxFeeRate: 0.01})
	require.NoError(t, err, "ApproveBuilderFee must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "ApproveBuilderFee should return the action's nonce")
}

func TestPlaceTWAPOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *TWAPOrderRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "too few minutes", arg: &TWAPOrderRequest{Size: 1, Minutes: minimumTWAPMinutes - 1}, err: errTWAPMinutesInvalid},
		{name: "too many minutes", arg: &TWAPOrderRequest{Size: 1, Minutes: maximumTWAPMinutes + 1}, err: errTWAPMinutesInvalid},
		{name: "no size", arg: &TWAPOrderRequest{Minutes: 30}, err: order.ErrAmountIsInvalid},
		{name: "a size beyond 8 decimals", arg: &TWAPOrderRequest{Size: 0.000000001, Minutes: 30}, err: errWireNumberRounding},
	} {
		_, err := e.PlaceTWAPOrder(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "PlaceTWAPOrder should reject a request with %s", tc.name)
	}
	for response, exp := range map[string]error{
		`{"status":"ok","response":{"type":"twapOrder","data":{"status":{"error":"Invalid TWAP duration: 1 min(s)"}}}}`: errActionResponse,
		`{"status":"ok","response":{"type":"twapOrder","data":{"status":{}}}}`:                                          errActionStatusMalformed,
		`{"status":"ok","response":{"type":"twapOrder","data":{}}}`:                                                     errActionStatusMalformed,
	} {
		_, err := newActionServerExchange(t, signingCredentials(), new(SignedActionRequest), response).PlaceTWAPOrder(t.Context(), &TWAPOrderRequest{Size: 1, Minutes: 30})
		assert.ErrorIsf(t, err, exp, "PlaceTWAPOrder should reject the response %s", response)
	}

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, `{"status":"ok","response":{"type":"twapOrder","data":{"status":{"running":{"twapId":1}}}}}`)
	_, err := ex.PlaceTWAPOrder(t.Context(), &TWAPOrderRequest{Asset: 1, Size: 2, ReduceOnly: true, Minutes: 5})
	require.NoError(t, err, "PlaceTWAPOrder must not error")
	assert.JSONEq(t, `{"type":"twapOrder","twap":{"a":1,"b":false,"s":"2","r":true,"m":5,"t":false}}`, capturedAction(t, &captured), "PlaceTWAPOrder should send the TWAP order")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "PlaceTWAPOrder should act for the configured subaccount")

	skipUnlessMockTesting(t)
	status, err := e.PlaceTWAPOrder(t.Context(), &TWAPOrderRequest{Asset: 0, IsBuy: true, Size: 0.01, Minutes: 30, Randomise: true})
	require.NoError(t, err, "PlaceTWAPOrder must not error")
	assert.Equal(t, &TWAPOrderStatus{Running: &TWAPRunningStatus{TWAPID: 77738308}}, status, "PlaceTWAPOrder should return the running TWAP order")
}

func TestCancelTWAPOrder(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.CancelTWAPOrder(t.Context(), nil), common.ErrNilPointer, "CancelTWAPOrder must reject a nil request")
	require.ErrorIs(t, e.CancelTWAPOrder(t.Context(), &TWAPCancelRequest{}), errTWAPIDRequired, "CancelTWAPOrder must require a TWAP order ID")
	ex := newActionServerExchange(t, signingCredentials(), new(SignedActionRequest), `{"status":"ok","response":{"type":"twapCancel","data":{"status":{"error":"TWAP was never placed, already canceled, or filled."}}}}`)
	require.ErrorIs(t, ex.CancelTWAPOrder(t.Context(), &TWAPCancelRequest{TWAPID: 1}), errActionResponse, "CancelTWAPOrder must return a failed cancel")
	ex = newActionServerExchange(t, signingCredentials(), new(SignedActionRequest), `{"status":"ok","response":{"type":"twapCancel","data":{"status":"unknown"}}}`)
	require.ErrorIs(t, ex.CancelTWAPOrder(t.Context(), &TWAPCancelRequest{TWAPID: 1}), errUnknownCancelStatus, "CancelTWAPOrder must reject an unknown status")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.CancelTWAPOrder(t.Context(), &TWAPCancelRequest{Asset: 0, TWAPID: 77738308}), "CancelTWAPOrder should not error")
}

func TestPlaceTrailingStopOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  *TrailingStopRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "no size", arg: &TrailingStopRequest{RetracementPercent: 1}, err: order.ErrAmountIsInvalid},
		{name: "no retracement", arg: &TrailingStopRequest{Size: 1}, err: errRetracementInvalid},
		{name: "both retracements", arg: &TrailingStopRequest{Size: 1, RetracementPercent: 1, RetracementPrice: 1}, err: errRetracementInvalid},
		{name: "a retracement percentage beyond 8 decimals", arg: &TrailingStopRequest{Size: 1, RetracementPercent: 0.000000001}, err: errRetracementInvalid},
		{name: "a negative activation price", arg: &TrailingStopRequest{Size: 1, RetracementPrice: 1, ActivationPrice: -1}, err: errActivationPriceInvalid},
		{name: "an activation price that is not a number", arg: &TrailingStopRequest{Size: 1, RetracementPrice: 1, ActivationPrice: math.NaN()}, err: errActivationPriceInvalid},
	} {
		_, err := e.PlaceTrailingStopOrder(t.Context(), tc.arg)
		assert.ErrorIsf(t, err, tc.err, "PlaceTrailingStopOrder should reject a request with %s", tc.name)
	}
	_, err := newActionServerExchange(t, signingCredentials(), new(SignedActionRequest), `{"status":"ok","response":{"type":"trailingStop","data":{}}}`).PlaceTrailingStopOrder(t.Context(), &TrailingStopRequest{Size: 1, RetracementPrice: 1})
	require.ErrorIs(t, err, errActionStatusMalformed, "PlaceTrailingStopOrder must reject a response without an order ID")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, `{"status":"ok","response":{"type":"trailingStop","data":{"oid":1}}}`)
	orderID, err := ex.PlaceTrailingStopOrder(t.Context(), &TrailingStopRequest{Asset: 1, IsBuy: true, Size: 2, RetracementPrice: 500})
	require.NoError(t, err, "PlaceTrailingStopOrder must not error")
	assert.Equal(t, uint64(1), orderID, "PlaceTrailingStopOrder should return the order ID")
	assert.JSONEq(t, `{"type":"trailingStop","asset":1,"isBuy":true,"sz":"2","reduceOnly":false,"retracement":{"px":"500"},"activationPx":null}`, capturedAction(t, &captured), "PlaceTrailingStopOrder should send a null activation price when tracking starts immediately")
	assert.Equal(t, testSubAccountAddress, captured.VaultAddress, "PlaceTrailingStopOrder should act for the configured subaccount")

	skipUnlessMockTesting(t)
	orderID, err = e.PlaceTrailingStopOrder(t.Context(), &TrailingStopRequest{Asset: 0, Size: 0.001, ReduceOnly: true, RetracementPercent: 1.5, ActivationPrice: 90000})
	require.NoError(t, err, "PlaceTrailingStopOrder must not error")
	assert.Equal(t, uint64(566563303544), orderID, "PlaceTrailingStopOrder should return the order ID")
}

func TestReserveRequestWeight(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.ReserveRequestWeight(t.Context(), nil), common.ErrNilPointer, "ReserveRequestWeight must reject a nil request")
	require.ErrorIs(t, e.ReserveRequestWeight(t.Context(), &ReserveRequestWeightRequest{}), errRequestWeightInvalid, "ReserveRequestWeight must reject a zero weight")
	require.ErrorIs(t, e.ReserveRequestWeight(t.Context(), &ReserveRequestWeightRequest{Weight: 1, Destination: "invalid"}), errInvalidAddress, "ReserveRequestWeight must reject an invalid destination")

	var captured SignedActionRequest
	require.NoError(t, newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse).ReserveRequestWeight(t.Context(), &ReserveRequestWeightRequest{Weight: 10}), "ReserveRequestWeight must not error")
	assert.JSONEq(t, `{"type":"reserveRequestWeight","weight":10}`, capturedAction(t, &captured), "ReserveRequestWeight should omit an unset destination")
	assert.Empty(t, captured.VaultAddress, "ReserveRequestWeight should not act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.ReserveRequestWeight(t.Context(), &ReserveRequestWeightRequest{Weight: 1000, Destination: testWithdrawalAddress}), "ReserveRequestWeight should not error")
}

func TestInvalidatePendingNonce(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.InvalidatePendingNonce(t.Context(), 0), errNonceRequired, "InvalidatePendingNonce must require a nonce")

	var captured SignedActionRequest
	require.NoError(t, newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse).InvalidatePendingNonce(t.Context(), vectorNonce), "InvalidatePendingNonce must not error")
	assert.JSONEq(t, `{"type":"noop"}`, capturedAction(t, &captured), "InvalidatePendingNonce should send a noop")
	assert.Equal(t, uint64(vectorNonce), captured.Nonce, "InvalidatePendingNonce should sign the pending nonce")
	assert.Empty(t, captured.VaultAddress, "InvalidatePendingNonce should not act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.InvalidatePendingNonce(t.Context(), vectorNonce), "InvalidatePendingNonce should not error")
}

func TestSetUserDEXAbstraction(t *testing.T) {
	t.Parallel()
	_, err := e.SetUserDEXAbstraction(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "SetUserDEXAbstraction must reject a nil request")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	_, err = ex.SetUserDEXAbstraction(t.Context(), &UserDEXAbstractionRequest{User: "invalid"})
	require.ErrorIs(t, err, errInvalidAddress, "SetUserDEXAbstraction must reject an invalid user")
	nonce, err := ex.SetUserDEXAbstraction(t.Context(), &UserDEXAbstractionRequest{})
	require.NoError(t, err, "SetUserDEXAbstraction must not error for the configured subaccount")
	exp := `{"type":"userDexAbstraction","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","user":"` + testSubAccountAddress + `","enabled":false,"nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "SetUserDEXAbstraction should default to the configured subaccount")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).SetUserDEXAbstraction(t.Context(), &UserDEXAbstractionRequest{Enabled: true})
	require.NoError(t, err, "SetUserDEXAbstraction must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "SetUserDEXAbstraction should return the action's nonce")
}

func TestAgentEnableDEXAbstraction(t *testing.T) {
	t.Parallel()
	var captured SignedActionRequest
	require.NoError(t, newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse).AgentEnableDEXAbstraction(t.Context()), "AgentEnableDEXAbstraction must not error")
	assert.JSONEq(t, `{"type":"agentEnableDexAbstraction"}`, capturedAction(t, &captured), "AgentEnableDEXAbstraction should send the action")
	assert.Empty(t, captured.VaultAddress, "AgentEnableDEXAbstraction should not act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.AgentEnableDEXAbstraction(t.Context()), "AgentEnableDEXAbstraction should not error")
}

func TestSetUserAbstraction(t *testing.T) {
	t.Parallel()
	_, err := e.SetUserAbstraction(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "SetUserAbstraction must reject a nil request")
	for _, abstraction := range []AccountAbstraction{"", AccountAbstractionDefault, AccountAbstractionDEX} {
		_, err := e.SetUserAbstraction(t.Context(), &SetUserAbstractionRequest{Abstraction: abstraction})
		assert.ErrorIsf(t, err, errAccountAbstractionInvalid, "SetUserAbstraction should reject abstraction %q", abstraction)
	}

	var captured SignedActionRequest
	nonce, err := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse).SetUserAbstraction(t.Context(), &SetUserAbstractionRequest{User: testAccountAddress, Abstraction: AccountAbstractionPortfolio})
	require.NoError(t, err, "SetUserAbstraction must not error")
	exp := `{"type":"userSetAbstraction","signatureChainId":"0x66eee","hyperliquidChain":"Mainnet","user":"` + testAccountAddress + `","abstraction":"portfolioMargin","nonce":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "SetUserAbstraction should apply to the requested user")

	skipUnlessMockTesting(t)
	nonce, err = newUserSignedTestExchange(t).SetUserAbstraction(t.Context(), &SetUserAbstractionRequest{Abstraction: AccountAbstractionUnified})
	require.NoError(t, err, "SetUserAbstraction must not error")
	assert.Equal(t, uint64(mockUserSignedNonce), nonce, "SetUserAbstraction should return the action's nonce")
}

func TestAgentSetUserAbstraction(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.AgentSetUserAbstraction(t.Context(), AccountAbstractionDEX), errAccountAbstractionInvalid, "AgentSetUserAbstraction must reject an abstraction it cannot set")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	for abstraction, code := range map[AccountAbstraction]string{AccountAbstractionDisabled: "i", AccountAbstractionUnified: "u", AccountAbstractionPortfolio: "p"} {
		require.NoErrorf(t, ex.AgentSetUserAbstraction(t.Context(), abstraction), "AgentSetUserAbstraction must not error for %s", abstraction)
		assert.JSONEqf(t, `{"type":"agentSetAbstraction","abstraction":"`+code+`"}`, capturedAction(t, &captured), "AgentSetUserAbstraction should send %s as %s", abstraction, code)
		assert.Emptyf(t, captured.VaultAddress, "AgentSetUserAbstraction should not act for the configured subaccount for %s", abstraction)
	}

	skipUnlessMockTesting(t)
	assert.NoError(t, e.AgentSetUserAbstraction(t.Context(), AccountAbstractionPortfolio), "AgentSetUserAbstraction should not error")
}

func TestUserOutcomeActions(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, e.SplitOutcome(t.Context(), nil), common.ErrNilPointer, "SplitOutcome must reject a nil request")
	require.ErrorIs(t, e.MergeOutcome(t.Context(), nil), common.ErrNilPointer, "MergeOutcome must reject a nil request")
	require.ErrorIs(t, e.MergeQuestion(t.Context(), nil), common.ErrNilPointer, "MergeQuestion must reject a nil request")
	require.ErrorIs(t, e.NegateOutcome(t.Context(), nil), common.ErrNilPointer, "NegateOutcome must reject a nil request")
	for _, amount := range []float64{0, -1, math.Inf(1)} {
		assert.ErrorIsf(t, e.SplitOutcome(t.Context(), &SplitOutcomeRequest{Amount: amount}), errOutcomeAmountInvalid, "SplitOutcome should reject amount %v", amount)
		assert.ErrorIsf(t, e.NegateOutcome(t.Context(), &NegateOutcomeRequest{Amount: amount}), errOutcomeAmountInvalid, "NegateOutcome should reject amount %v", amount)
	}
	assert.ErrorIs(t, e.MergeOutcome(t.Context(), &MergeOutcomeRequest{Amount: -1}), errOutcomeAmountInvalid, "MergeOutcome should reject a negative amount")
	assert.ErrorIs(t, e.MergeQuestion(t.Context(), &MergeQuestionRequest{Amount: -1}), errOutcomeAmountInvalid, "MergeQuestion should reject a negative amount")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse)
	expiresAfter := time.Now().Add(time.Minute)
	for _, tc := range []struct {
		send func() error
		exp  string
	}{
		{send: func() error {
			return ex.SplitOutcome(t.Context(), &SplitOutcomeRequest{Outcome: 3, Amount: 4, ExpiresAfter: expiresAfter})
		}, exp: `{"type":"userOutcome","splitOutcome":{"outcome":3,"amount":"4"}}`},
		{send: func() error {
			return ex.MergeOutcome(t.Context(), &MergeOutcomeRequest{Outcome: 1, ExpiresAfter: expiresAfter})
		}, exp: `{"type":"userOutcome","mergeOutcome":{"outcome":1,"amount":null}}`},
		{send: func() error {
			return ex.MergeQuestion(t.Context(), &MergeQuestionRequest{Question: 2, Amount: 3, ExpiresAfter: expiresAfter})
		}, exp: `{"type":"userOutcome","mergeQuestion":{"question":2,"amount":"3"}}`},
		{send: func() error {
			return ex.NegateOutcome(t.Context(), &NegateOutcomeRequest{Question: 5, Outcome: 6, Amount: 7, ExpiresAfter: expiresAfter})
		}, exp: `{"type":"userOutcome","negateOutcome":{"question":5,"outcome":6,"amount":"7"}}`},
	} {
		require.NoErrorf(t, tc.send(), "userOutcome must not error for %s", tc.exp)
		assert.JSONEqf(t, tc.exp, capturedAction(t, &captured), "userOutcome should send %s", tc.exp)
		assert.Emptyf(t, captured.VaultAddress, "userOutcome should not act for the configured subaccount for %s", tc.exp)
		assert.Equalf(t, new(unixMilli(expiresAfter)), captured.ExpiresAfter, "userOutcome should send the expiry for %s", tc.exp)
	}

	skipUnlessMockTesting(t)
	assert.NoError(t, e.SplitOutcome(t.Context(), &SplitOutcomeRequest{Outcome: 9015, Amount: 10}), "SplitOutcome should not error")
	assert.NoError(t, e.MergeOutcome(t.Context(), &MergeOutcomeRequest{Outcome: 9015}), "MergeOutcome should not error")
	assert.NoError(t, e.MergeQuestion(t.Context(), &MergeQuestionRequest{Question: 198, Amount: 5}), "MergeQuestion should not error")
	assert.NoError(t, e.NegateOutcome(t.Context(), &NegateOutcomeRequest{Question: 198, Outcome: 1473, Amount: 2}), "NegateOutcome should not error")
}

func TestClaimRewards(t *testing.T) {
	t.Parallel()
	var captured SignedActionRequest
	require.NoError(t, newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse).ClaimRewards(t.Context()), "ClaimRewards must not error")
	assert.JSONEq(t, `{"type":"claimRewards"}`, capturedAction(t, &captured), "ClaimRewards should send the action")
	assert.Empty(t, captured.VaultAddress, "ClaimRewards should not act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.ClaimRewards(t.Context()), "ClaimRewards should not error")
}

func TestBidGossipPriority(t *testing.T) {
	t.Parallel()
	ip := netip.MustParseAddr("203.0.113.7")
	for _, tc := range []struct {
		name string
		arg  *GossipPriorityBidRequest
		err  error
	}{
		{name: "nil request", err: common.ErrNilPointer},
		{name: "an unknown slot", arg: &GossipPriorityBidRequest{SlotID: 2, IP: ip, MaxGas: 1}, err: errGossipSlotInvalid},
		{name: "no IP address", arg: &GossipPriorityBidRequest{MaxGas: 1}, err: errGossipIPInvalid},
		{name: "no maximum gas", arg: &GossipPriorityBidRequest{IP: ip}, err: errMaxGasInvalid},
	} {
		assert.ErrorIsf(t, e.BidGossipPriority(t.Context(), tc.arg), tc.err, "BidGossipPriority should reject a request with %s", tc.name)
	}

	var captured SignedActionRequest
	require.NoError(t, newActionServerExchange(t, subAccountCredentials(), &captured, defaultActionResponse).BidGossipPriority(t.Context(), &GossipPriorityBidRequest{IP: netip.MustParseAddr("::ffff:1.2.3.4"), MaxGas: 100000000}), "BidGossipPriority must not error")
	assert.JSONEq(t, `{"type":"gossipPriorityBid","slotId":0,"ip":"1.2.3.4","maxGas":100000000}`, capturedAction(t, &captured), "BidGossipPriority should send an IPv4-mapped address as IPv4")
	assert.Empty(t, captured.VaultAddress, "BidGossipPriority should not act for the configured subaccount")

	skipUnlessMockTesting(t)
	assert.NoError(t, e.BidGossipPriority(t.Context(), &GossipPriorityBidRequest{SlotID: 1, IP: ip, MaxGas: 10000000}), "BidGossipPriority should not error")
}

func TestOrderRequestToWire(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		arg  OrderRequest
		err  error
	}{
		{name: "no size", arg: OrderRequest{Price: 1, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}}, err: order.ErrAmountIsInvalid},
		{name: "no price", arg: OrderRequest{Size: 1, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}}, err: errOrderPriceInvalid},
		{name: "a size beyond 8 decimals", arg: OrderRequest{Price: 1, Size: 0.000000001, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}}, err: errWireNumberRounding},
		{name: "a price beyond 8 decimals", arg: OrderRequest{Price: 0.000000001, Size: 1, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}}, err: errWireNumberRounding},
		{name: "an invalid client order ID", arg: OrderRequest{Price: 1, Size: 1, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}, ClientOrderID: "0x1"}, err: errClientOrderIDInvalid},
		{name: "an unknown time in force", arg: OrderRequest{Price: 1, Size: 1, Limit: &LimitOrderType{TimeInForce: "Fok"}}, err: order.ErrUnsupportedTimeInForce},
		{name: "an unknown trigger kind", arg: OrderRequest{Price: 1, Size: 1, Trigger: &TriggerOrderType{TriggerPrice: 1}}, err: errInvalidTriggerKind},
		{name: "no trigger price", arg: OrderRequest{Price: 1, Size: 1, Trigger: &TriggerOrderType{TakeProfitStopLoss: TriggerStopLoss}}, err: errTriggerPriceRequired},
		{name: "a trigger price beyond 8 decimals", arg: OrderRequest{Price: 1, Size: 1, Trigger: &TriggerOrderType{TriggerPrice: 0.000000001, TakeProfitStopLoss: TriggerStopLoss}}, err: errWireNumberRounding},
		{name: "no order type", arg: OrderRequest{Price: 1, Size: 1}, err: errOrderTypeRequired},
		{name: "both order types", arg: OrderRequest{Price: 1, Size: 1, Limit: &LimitOrderType{TimeInForce: TimeInForceGTC}, Trigger: &TriggerOrderType{TriggerPrice: 1, TakeProfitStopLoss: TriggerStopLoss}}, err: errOrderTypeRequired},
	} {
		_, err := tc.arg.ToWire()
		assert.ErrorIsf(t, err, tc.err, "ToWire should reject an order with %s", tc.name)
	}

	limit := OrderRequest{Asset: 10107, IsBuy: true, Price: 95.5, Size: 1.5, ReduceOnly: true, Limit: &LimitOrderType{TimeInForce: TimeInForceALO}, ClientOrderID: strings.ToUpper(testClientOrderID)}
	wire, err := limit.ToWire()
	require.NoError(t, err, "ToWire must not error for a limit order")
	exp := OrderWire{Asset: 10107, IsBuy: true, Price: "95.5", Size: "1.5", ReduceOnly: true, Type: OrderTypeWire{Limit: &LimitOrderTypeWire{TimeInForce: TimeInForceALO}}, ClientOrderID: testClientOrderID}
	assert.Equal(t, exp, wire, "ToWire should format the limit order with a lower-case client order ID")

	trigger := OrderRequest{Asset: 0, Price: 79000, Size: 0.001, ReduceOnly: true, Trigger: &TriggerOrderType{IsMarket: true, TriggerPrice: 80000, TakeProfitStopLoss: TriggerStopLoss}}
	wire, err = trigger.ToWire()
	require.NoError(t, err, "ToWire must not error for a trigger order")
	exp = OrderWire{Price: "79000", Size: "0.001", ReduceOnly: true, Type: OrderTypeWire{Trigger: &TriggerOrderTypeWire{IsMarket: true, TriggerPrice: "80000", TakeProfitStopLoss: TriggerStopLoss}}}
	assert.Equal(t, exp, wire, "ToWire should format the trigger order")
}

func TestValidatePriorityGrouping(t *testing.T) {
	t.Parallel()
	immediate := OrderRequest{Limit: &LimitOrderType{TimeInForce: TimeInForceIOC}}
	addLiquidity := OrderRequest{Limit: &LimitOrderType{TimeInForce: TimeInForceALO}}
	for _, tc := range []struct {
		name string
		arg  *PlaceOrdersRequest
		err  error
	}{
		{name: "IOC orders", arg: &PlaceOrdersRequest{Orders: []OrderRequest{immediate, immediate}, PriorityRate: maximumPriorityRate}},
		{name: "ALO orders", arg: &PlaceOrdersRequest{Orders: []OrderRequest{addLiquidity}, Grouping: GroupingNone, PriorityRate: 1}},
		{name: "a rate above the maximum", arg: &PlaceOrdersRequest{Orders: []OrderRequest{immediate}, PriorityRate: maximumPriorityRate + 1}, err: errPriorityRateInvalid},
		{name: "a position TP/SL grouping", arg: &PlaceOrdersRequest{Orders: []OrderRequest{immediate}, Grouping: GroupingPositionTPSL, PriorityRate: 1}, err: errPriorityGroupingInvalid},
		{name: "mixed IOC and ALO orders", arg: &PlaceOrdersRequest{Orders: []OrderRequest{addLiquidity, immediate}, PriorityRate: 1}, err: errPriorityGroupingInvalid},
	} {
		assert.ErrorIsf(t, validatePriorityGrouping(tc.arg), tc.err, "validatePriorityGrouping should return the expected error for %s", tc.name)
	}
}

func TestBuilderFeeWire(t *testing.T) {
	t.Parallel()
	spot := OrderRequest{Asset: 10107}
	perpetual := OrderRequest{Asset: 0}
	builder := OrderRequest{Asset: 110000}
	for _, tc := range []struct {
		name   string
		orders []OrderRequest
		fee    uint64
		err    error
	}{
		{name: "spot orders at the spot maximum", orders: []OrderRequest{spot}, fee: maximumSpotBuilderFee},
		{name: "spot orders above the spot maximum", orders: []OrderRequest{spot}, fee: maximumSpotBuilderFee + 1, err: errBuilderFeeInvalid},
		{name: "a perpetual order above the perpetual maximum", orders: []OrderRequest{spot, perpetual}, fee: maximumPerpetualBuilderFee + 1, err: errBuilderFeeInvalid},
		{name: "a builder DEX order above the perpetual maximum", orders: []OrderRequest{builder}, fee: maximumPerpetualBuilderFee + 1, err: errBuilderFeeInvalid},
		{name: "a perpetual order at the perpetual maximum", orders: []OrderRequest{perpetual}, fee: maximumPerpetualBuilderFee},
	} {
		wire, err := builderFeeWire(&BuilderFee{Builder: testBuilderAddress, Fee: tc.fee}, tc.orders)
		require.ErrorIsf(t, err, tc.err, "builderFeeWire must return the expected error for %s", tc.name)
		if tc.err == nil {
			assert.Equalf(t, BuilderFeeWire{Builder: testBuilderAddress, Fee: tc.fee}, wire, "builderFeeWire should return the fee for %s", tc.name)
		}
	}
	_, err := builderFeeWire(&BuilderFee{Builder: "invalid"}, []OrderRequest{spot})
	assert.ErrorIs(t, err, errInvalidAddress, "builderFeeWire should reject an invalid builder")
}

func TestModifyOrderID(t *testing.T) {
	t.Parallel()
	orderID, err := modifyOrderID(7, "")
	require.NoError(t, err, "modifyOrderID must not error for an order ID")
	assert.Equal(t, uint64(7), orderID, "modifyOrderID should return the order ID")
	orderID, err = modifyOrderID(0, strings.ToUpper(testClientOrderID))
	require.NoError(t, err, "modifyOrderID must not error for a client order ID")
	assert.Equal(t, testClientOrderID, orderID, "modifyOrderID should return the lower-case client order ID")
	_, err = modifyOrderID(7, testClientOrderID)
	assert.ErrorIs(t, err, errOrderIdentifiersConflict, "modifyOrderID should reject both identifiers")
	_, err = modifyOrderID(0, "")
	assert.ErrorIs(t, err, order.ErrOrderIDNotSet, "modifyOrderID should require an identifier")
	_, err = modifyOrderID(0, "0x1")
	assert.ErrorIs(t, err, errClientOrderIDInvalid, "modifyOrderID should reject an invalid client order ID")
}

func TestParseActionStatuses(t *testing.T) {
	t.Parallel()
	_, err := parseActionStatuses[CancelActionStatus](json.RawMessage(`[`), 1)
	assert.Error(t, err, "parseActionStatuses should reject invalid JSON")
	_, err = parseActionStatuses[CancelActionStatus](json.RawMessage(`{"data":{"statuses":["success","success"]}}`), 3)
	assert.ErrorIs(t, err, errActionStatusCount, "parseActionStatuses should reject a status count that does not match the batch")
	statuses, err := parseActionStatuses[CancelActionStatus](json.RawMessage(`{"type":"cancel","data":{"statuses":[{"error":"Invalid nonce"}]}}`), 2)
	require.NoError(t, err, "parseActionStatuses must not error for a lone status")
	assert.Equal(t, []CancelActionStatus{{Error: "Invalid nonce"}, {Error: "Invalid nonce"}}, statuses, "parseActionStatuses should apply a lone status to every item")
}

func TestParseActionStatus(t *testing.T) {
	t.Parallel()
	_, err := parseActionStatus[CancelActionStatus](json.RawMessage(`[`))
	assert.Error(t, err, "parseActionStatus should reject invalid JSON")
	for _, response := range []string{`{"data":{}}`, `{"data":{"status":null}}`} {
		_, err = parseActionStatus[CancelActionStatus](json.RawMessage(response))
		assert.ErrorIsf(t, err, errActionStatusMalformed, "parseActionStatus should reject %s", response)
	}
	_, err = parseActionStatus[CancelActionStatus](json.RawMessage(`{"data":{"status":"unknown"}}`))
	assert.ErrorIs(t, err, errUnknownCancelStatus, "parseActionStatus should return the status's decoding error")
	status, err := parseActionStatus[TWAPOrderStatus](json.RawMessage(`{"type":"twapOrder","data":{"status":{"running":{"twapId":2}}}}`))
	require.NoError(t, err, "parseActionStatus must not error")
	assert.Equal(t, &TWAPOrderStatus{Running: &TWAPRunningStatus{TWAPID: 2}}, status, "parseActionStatus should decode the status")
}

func TestOrderActionStatusUnmarshalJSON(t *testing.T) {
	t.Parallel()
	for data, exp := range map[string]OrderActionStatus{
		`"waitingForFill"`:    {Waiting: orderStatusWaitingForFill},
		`"waitingForTrigger"`: {Waiting: orderStatusWaitingForTrigger},
		`{"resting":{"oid":1,"cloid":"` + testClientOrderID + `"}}`:                                  {Resting: &RestingOrderStatus{OrderID: 1, ClientOrderID: testClientOrderID}},
		`{"filled":{"totalSz":"0.02","avgPx":"1891.4","oid":2,"cloid":"` + testClientOrderID + `"}}`: {Filled: &FilledOrderStatus{TotalSize: 0.02, AveragePrice: 1891.4, OrderID: 2, ClientOrderID: testClientOrderID}},
		`{"error":"Order must have minimum value of $10."}`:                                          {Error: "Order must have minimum value of $10."},
	} {
		var status OrderActionStatus
		require.NoErrorf(t, json.Unmarshal([]byte(data), &status), "Unmarshal must not error for %s", data)
		assert.Equalf(t, exp, status, "OrderActionStatus should decode %s", data)
	}
	for _, data := range []string{`"unknown"`, `{}`, `{"resting":{"oid":0}}`, `{"filled":{"oid":0}}`, `{"resting":{"oid":1},"error":"e"}`} {
		var status OrderActionStatus
		assert.ErrorIsf(t, json.Unmarshal([]byte(data), &status), errActionStatusMalformed, "OrderActionStatus should reject %s", data)
	}
	for _, data := range []string{`"`, `{"resting":1}`} {
		var status OrderActionStatus
		assert.Errorf(t, json.Unmarshal([]byte(data), &status), "OrderActionStatus should reject %s", data)
	}
}

func TestCancelActionStatusUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var status CancelActionStatus
	require.NoError(t, json.Unmarshal([]byte(`"success"`), &status), "Unmarshal must not error for success")
	assert.Equal(t, CancelActionStatus{}, status, "CancelActionStatus should decode success without an error")
	require.NoError(t, json.Unmarshal([]byte(`{"error":"Order was never placed, already canceled, or filled."}`), &status), "Unmarshal must not error for an error")
	assert.Equal(t, CancelActionStatus{Error: "Order was never placed, already canceled, or filled."}, status, "CancelActionStatus should decode the error")
	assert.ErrorIs(t, json.Unmarshal([]byte(`"unknown"`), &status), errUnknownCancelStatus, "CancelActionStatus should reject an unknown status")
	assert.ErrorIs(t, json.Unmarshal([]byte(`{}`), &status), errActionStatusMalformed, "CancelActionStatus should reject an object without an error")
	assert.Error(t, json.Unmarshal([]byte(`{"error":1}`), &status), "CancelActionStatus should reject a mistyped error")
	assert.Error(t, json.Unmarshal([]byte(`"`), &status), "CancelActionStatus should reject an unterminated string")
}

func TestTWAPOrderStatusUnmarshalJSON(t *testing.T) {
	t.Parallel()
	for data, exp := range map[string]TWAPOrderStatus{
		`{"running":{"twapId":77738308}}`:             {Running: &TWAPRunningStatus{TWAPID: 77738308}},
		`{"error":"Invalid TWAP duration: 1 min(s)"}`: {Error: "Invalid TWAP duration: 1 min(s)"},
	} {
		var status TWAPOrderStatus
		require.NoErrorf(t, json.Unmarshal([]byte(data), &status), "Unmarshal must not error for %s", data)
		assert.Equalf(t, exp, status, "TWAPOrderStatus should decode %s", data)
	}
	for _, data := range []string{`{}`, `{"running":{"twapId":0}}`, `{"running":{"twapId":1},"error":"e"}`} {
		var status TWAPOrderStatus
		assert.ErrorIsf(t, json.Unmarshal([]byte(data), &status), errActionStatusMalformed, "TWAPOrderStatus should reject %s", data)
	}
	var status TWAPOrderStatus
	assert.Error(t, json.Unmarshal([]byte(`[]`), &status), "TWAPOrderStatus should reject an array")
}

func TestValidateClientOrderID(t *testing.T) {
	t.Parallel()
	for _, clientOrderID := range []string{testClientOrderID, strings.ToUpper(testClientOrderID)} {
		assert.NoErrorf(t, validateClientOrderID(clientOrderID), "validateClientOrderID should accept %s", clientOrderID)
	}
	for _, clientOrderID := range []string{"", "0x1", testClientOrderID[2:] + "00", "0x" + strings.Repeat("g", 32)} {
		assert.ErrorIsf(t, validateClientOrderID(clientOrderID), errClientOrderIDInvalid, "validateClientOrderID should reject %q", clientOrderID)
	}
}

func TestFormatOutcomeAmount(t *testing.T) {
	t.Parallel()
	amount, err := formatOutcomeAmount(10)
	require.NoError(t, err, "formatOutcomeAmount must not error")
	assert.Equal(t, "10", amount, "formatOutcomeAmount should format the amount")
	for _, value := range []float64{0, -1, math.NaN()} {
		_, err := formatOutcomeAmount(value)
		assert.ErrorIsf(t, err, errOutcomeAmountInvalid, "formatOutcomeAmount should reject %v", value)
	}
}

func TestFormatMergeAmount(t *testing.T) {
	t.Parallel()
	amount, err := formatMergeAmount(0)
	require.NoError(t, err, "formatMergeAmount must not error for zero")
	assert.Nil(t, amount, "formatMergeAmount should merge the maximum for zero")
	amount, err = formatMergeAmount(2.5)
	require.NoError(t, err, "formatMergeAmount must not error")
	assert.Equal(t, new("2.5"), amount, "formatMergeAmount should format the amount")
	_, err = formatMergeAmount(-1)
	assert.ErrorIs(t, err, errOutcomeAmountInvalid, "formatMergeAmount should reject a negative amount")
}

func TestExpiresAfterMilli(t *testing.T) {
	t.Parallel()
	expiresAfter, err := expiresAfterMilli(time.Time{})
	require.NoError(t, err, "expiresAfterMilli must not error for no expiry")
	assert.Nil(t, expiresAfter, "expiresAfterMilli should not set an expiry")
	_, err = expiresAfterMilli(time.Now().Add(-time.Millisecond))
	assert.ErrorIs(t, err, errExpiresAfterPassed, "expiresAfterMilli should reject a passed expiry")
	future := time.Now().Add(time.Minute)
	expiresAfter, err = expiresAfterMilli(future)
	require.NoError(t, err, "expiresAfterMilli must not error for a future expiry")
	assert.Equal(t, new(unixMilli(future)), expiresAfter, "expiresAfterMilli should convert the expiry to Unix milliseconds")
}

func TestUnixMilli(t *testing.T) {
	t.Parallel()
	assert.Equal(t, uint64(1700000000000), unixMilli(time.UnixMilli(1700000000000)), "unixMilli should return Unix milliseconds")
}

func TestSendUserSignedAction(t *testing.T) {
	t.Parallel()
	_, err := e.sendUserSignedAction(t.Context(), signingCredentials(), nil)
	require.ErrorIs(t, err, errUserSignedActionInvalid, "sendUserSignedAction must reject a nil payload")

	missing, _ := newRoleServerExchange(t, nil, nil, nil)
	_, err = missing.sendUserSignedAction(t.Context(), signingCredentials(), &USDSendAction{Destination: testWithdrawalAddress, Amount: "1"})
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "sendUserSignedAction must report failed authority as an authentication failure")

	var captured SignedActionRequest
	ex := newActionServerExchange(t, signingCredentials(), &captured, defaultActionResponse)
	_, err = ex.sendUserSignedAction(t.Context(), signingCredentials(), &TokenDelegateAction{Validator: "invalid", Wei: 1})
	require.ErrorIs(t, err, errEIP712Field, "sendUserSignedAction must return a signing error")
	ex.Config.UseSandbox = true
	nonce, err := ex.sendUserSignedAction(t.Context(), signingCredentials(), &USDSendAction{Destination: testWithdrawalAddress, Amount: "1"})
	require.NoError(t, err, "sendUserSignedAction must not error")
	exp := `{"type":"usdSend","signatureChainId":"0x66eee","hyperliquidChain":"Testnet","destination":"` + testWithdrawalAddress + `","amount":"1","time":` + strconv.FormatUint(nonce, 10) + `}`
	assert.JSONEq(t, exp, capturedAction(t, &captured), "sendUserSignedAction should send the payload for testnet in sandbox mode")
	assert.Equal(t, nonce, captured.Nonce, "sendUserSignedAction should send the action's nonce as the request nonce")
	signature, err := signUserSignedAction(testPrivateKey, "HyperliquidTransaction:UsdSend", []eip712Field{
		{Name: "hyperliquidChain", Type: "string", Value: "Testnet"},
		{Name: "destination", Type: "string", Value: testWithdrawalAddress},
		{Name: "amount", Type: "string", Value: "1"},
		{Name: "time", Type: "uint64", Value: nonce},
	})
	require.NoError(t, err, "signUserSignedAction must not error")
	assert.Equal(t, signature, captured.Signature, "sendUserSignedAction should sign the chain, fields and nonce it sends, in order")

	rejected := newActionServerExchange(t, signingCredentials(), new(SignedActionRequest), `{"status":"err","response":"Insufficient balance."}`)
	_, err = rejected.sendUserSignedAction(t.Context(), signingCredentials(), &USDSendAction{Destination: testWithdrawalAddress, Amount: "1"})
	assert.ErrorIs(t, err, errActionResponse, "sendUserSignedAction should return a rejected action")
	assert.False(t, rejected.authorityValidated, "sendUserSignedAction should revalidate authority after a rejected action")
}

func TestUserSignedPayloads(t *testing.T) {
	t.Parallel()
	const nonce = 1700000000000
	for _, tc := range []struct {
		actionType  string
		payload     userSignedPayload
		primaryType string
		fields      []eip712Field
	}{
		{
			actionType:  "sendAsset",
			payload:     &SendAssetAction{Destination: testWithdrawalAddress, SourceDEX: "spot", DestinationDEX: "xyz", Token: "USDC", Amount: "1.5", FromSubAccount: testSubAccountAddress},
			primaryType: "HyperliquidTransaction:SendAsset",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "destination", Type: "string", Value: testWithdrawalAddress},
				{Name: "sourceDex", Type: "string", Value: "spot"},
				{Name: "destinationDex", Type: "string", Value: "xyz"},
				{Name: "token", Type: "string", Value: "USDC"},
				{Name: "amount", Type: "string", Value: "1.5"},
				{Name: "fromSubAccount", Type: "string", Value: testSubAccountAddress},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType: "sendToEvmWithData",
			payload: &SendToEVMWithDataAction{
				Token:                "USDC:0x6d1e7cde53ba9467b783cb7c530ce054",
				Amount:               "5",
				SourceDEX:            "spot",
				DestinationRecipient: testWithdrawalAddress,
				AddressEncoding:      AddressEncodingHex,
				DestinationChainID:   999,
				GasLimit:             200000,
				Data:                 "0xdeadbeef",
			},
			primaryType: "HyperliquidTransaction:SendToEvmWithData",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "token", Type: "string", Value: "USDC:0x6d1e7cde53ba9467b783cb7c530ce054"},
				{Name: "amount", Type: "string", Value: "5"},
				{Name: "sourceDex", Type: "string", Value: "spot"},
				{Name: "destinationRecipient", Type: "string", Value: testWithdrawalAddress},
				{Name: "addressEncoding", Type: "string", Value: AddressEncodingHex},
				{Name: "destinationChainId", Type: "uint32", Value: uint32(999)},
				{Name: "gasLimit", Type: "uint64", Value: uint64(200000)},
				{Name: "data", Type: "bytes", Value: "0xdeadbeef"},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "usdSend",
			payload:     &USDSendAction{Destination: testWithdrawalAddress, Amount: "1.5"},
			primaryType: "HyperliquidTransaction:UsdSend",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "destination", Type: "string", Value: testWithdrawalAddress},
				{Name: "amount", Type: "string", Value: "1.5"},
				{Name: "time", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "spotSend",
			payload:     &SpotSendAction{Destination: testWithdrawalAddress, Token: "HYPE:0x0d01dc56dcaaca66ad901c959b4011ec", Amount: "1.5"},
			primaryType: "HyperliquidTransaction:SpotSend",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "destination", Type: "string", Value: testWithdrawalAddress},
				{Name: "token", Type: "string", Value: "HYPE:0x0d01dc56dcaaca66ad901c959b4011ec"},
				{Name: "amount", Type: "string", Value: "1.5"},
				{Name: "time", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "withdraw3",
			payload:     &Withdraw3Action{Destination: testWithdrawalAddress, Amount: "1.5"},
			primaryType: "HyperliquidTransaction:Withdraw",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "destination", Type: "string", Value: testWithdrawalAddress},
				{Name: "amount", Type: "string", Value: "1.5"},
				{Name: "time", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "usdClassTransfer",
			payload:     &USDClassTransferAction{Amount: "1 subaccount:" + testSubAccountAddress, ToPerp: true},
			primaryType: "HyperliquidTransaction:UsdClassTransfer",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "amount", Type: "string", Value: "1 subaccount:" + testSubAccountAddress},
				{Name: "toPerp", Type: "bool", Value: true},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "cDeposit",
			payload:     &CDepositAction{Wei: 100000000},
			primaryType: "HyperliquidTransaction:CDeposit",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "wei", Type: "uint64", Value: uint64(100000000)},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "cWithdraw",
			payload:     &CWithdrawAction{Wei: 50000000},
			primaryType: "HyperliquidTransaction:CWithdraw",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "wei", Type: "uint64", Value: uint64(50000000)},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "tokenDelegate",
			payload:     &TokenDelegateAction{Validator: testValidatorAddress, Wei: 2, IsUndelegate: true},
			primaryType: "HyperliquidTransaction:TokenDelegate",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "validator", Type: "address", Value: testValidatorAddress},
				{Name: "wei", Type: "uint64", Value: uint64(2)},
				{Name: "isUndelegate", Type: "bool", Value: true},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "approveAgent",
			payload:     &ApproveAgentAction{AgentAddress: testAgentAddress, AgentName: "gct valid_until 1700000600000"},
			primaryType: "HyperliquidTransaction:ApproveAgent",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "agentAddress", Type: "address", Value: testAgentAddress},
				{Name: "agentName", Type: "string", Value: "gct valid_until 1700000600000"},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "approveBuilderFee",
			payload:     &ApproveBuilderFeeAction{MaxFeeRate: "0.001%", Builder: testBuilderAddress},
			primaryType: "HyperliquidTransaction:ApproveBuilderFee",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "maxFeeRate", Type: "string", Value: "0.001%"},
				{Name: "builder", Type: "address", Value: testBuilderAddress},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "userDexAbstraction",
			payload:     &UserDEXAbstractionAction{User: testSubAccountAddress, Enabled: true},
			primaryType: "HyperliquidTransaction:UserDexAbstraction",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "user", Type: "address", Value: testSubAccountAddress},
				{Name: "enabled", Type: "bool", Value: true},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
		{
			actionType:  "userSetAbstraction",
			payload:     &UserSetAbstractionAction{User: testAccountAddress, Abstraction: AccountAbstractionPortfolio},
			primaryType: "HyperliquidTransaction:UserSetAbstraction",
			fields: []eip712Field{
				{Name: "hyperliquidChain", Type: "string", Value: "Mainnet"},
				{Name: "user", Type: "address", Value: testAccountAddress},
				{Name: "abstraction", Type: "string", Value: "portfolioMargin"},
				{Name: "nonce", Type: "uint64", Value: uint64(nonce)},
			},
		},
	} {
		tc.payload.setEnvelope("Mainnet", nonce)
		primaryType, fields := tc.payload.signingFields()
		assert.Equalf(t, tc.primaryType, primaryType, "signingFields should return the %s primary type", tc.actionType)
		assert.Equalf(t, tc.fields, fields, "signingFields should list the %s fields in EIP-712 type order", tc.actionType)
		_, err := eip712UserDigest(primaryType, fields)
		assert.NoErrorf(t, err, "eip712UserDigest should accept the %s fields", tc.actionType)

		exp := map[string]any{"type": tc.actionType, "signatureChainId": userSignedChainIDHex}
		for _, field := range fields {
			exp[field.Name] = field.Value
		}
		expJSON, err := json.Marshal(exp)
		require.NoErrorf(t, err, "Marshal must not error for the expected %s body", tc.actionType)
		payloadJSON, err := json.Marshal(tc.payload)
		require.NoErrorf(t, err, "Marshal must not error for the %s payload", tc.actionType)
		assert.JSONEqf(t, string(expJSON), string(payloadJSON), "the %s payload should send its type, signature chain ID and exactly the fields it signs", tc.actionType)
	}
}

func TestUserSignedFields(t *testing.T) {
	t.Parallel()
	exp := []eip712Field{
		{Name: "hyperliquidChain", Type: "string", Value: "Testnet"},
		{Name: "amount", Type: "string", Value: "1"},
		{Name: "time", Type: "uint64", Value: uint64(1700000000000)},
	}
	assert.Equal(t, exp, userSignedFields("Testnet", nonceFieldTime, 1700000000000, eip712Field{Name: "amount", Type: "string", Value: "1"}), "userSignedFields should list the chain, the fields and then the nonce")
}

func TestPrepareTransfer(t *testing.T) {
	t.Parallel()
	_, _, err := prepareTransfer("invalid", 1)
	assert.ErrorIs(t, err, errInvalidAddress, "prepareTransfer should reject an invalid destination")
	_, _, err = prepareTransfer(testWithdrawalAddress, 0)
	assert.ErrorIs(t, err, errTransferAmountInvalid, "prepareTransfer should reject a zero amount")
	destination, amount, err := prepareTransfer("0x"+strings.ToUpper(testWithdrawalAddress[2:]), 1.5)
	require.NoError(t, err, "prepareTransfer must not error")
	assert.Equal(t, testWithdrawalAddress, destination, "prepareTransfer should normalise the destination")
	assert.Equal(t, "1.5", amount, "prepareTransfer should format the amount")
}

func TestGetAccountSigningCredentials(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	ex.SetDefaults()
	setTestCredentials(ex, signingCredentials())
	credentials, err := ex.getAccountSigningCredentials(t.Context())
	require.NoError(t, err, "getAccountSigningCredentials must not error")
	assert.Equal(t, testAccountAddress, credentials.Key, "getAccountSigningCredentials should return the account's credentials")
	setTestCredentials(ex, subAccountCredentials())
	_, err = ex.getAccountSigningCredentials(t.Context())
	assert.ErrorIs(t, err, errTransferSubAccountUnsupported, "getAccountSigningCredentials should reject a configured subaccount")
	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress, Secret: testAgentPrivateKey})
	_, err = ex.getAccountSigningCredentials(t.Context())
	assert.ErrorIs(t, err, errUserSignedMasterRequired, "getAccountSigningCredentials should require the account's own key")
}

func TestGetBridgeChain(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	assert.Equal(t, "Arbitrum", ex.getBridgeChain(), "getBridgeChain should return Arbitrum without a configuration")
	ex.SetDefaults()
	cfg, err := ex.GetStandardConfig()
	require.NoError(t, err, "GetStandardConfig must not error")
	cfg.UseSandbox = true
	ex.Config = cfg
	assert.Equal(t, "Arbitrum Sepolia", ex.getBridgeChain(), "getBridgeChain should return Arbitrum Sepolia in sandbox mode")
}

func TestFormatTransferAmount(t *testing.T) {
	t.Parallel()
	amount, err := formatTransferAmount(1.23)
	require.NoError(t, err, "formatTransferAmount must not error")
	assert.Equal(t, "1.23", amount, "formatTransferAmount should format the amount")
	for _, value := range []float64{0, -1, math.NaN(), 0.000000001} {
		_, err := formatTransferAmount(value)
		assert.ErrorIsf(t, err, errTransferAmountInvalid, "formatTransferAmount should reject %v", value)
	}
}

func TestValidateUserSignedSubAccount(t *testing.T) {
	t.Parallel()
	roles := map[string]string{
		testSubAccountAddress: `{"role":"subAccount","data":{"master":"` + testAccountAddress + `"}}`,
		testVaultAddress:      `{"role":"vault"}`,
		testTraderAddress:     `{"role":"subAccount","data":{"master":"` + testOtherAddress + `"}}`,
	}
	ex, _ := newRoleServerExchange(t, roles, nil, nil)
	credentials := signingCredentials()
	assert.NoError(t, ex.validateUserSignedSubAccount(t.Context(), credentials, ""), "validateUserSignedSubAccount should accept no subaccount")
	assert.NoError(t, ex.validateUserSignedSubAccount(t.Context(), credentials, testSubAccountAddress), "validateUserSignedSubAccount should accept an owned subaccount")
	for _, subAccount := range []string{testVaultAddress, testTraderAddress, testOtherAddress} {
		assert.ErrorIsf(t, ex.validateUserSignedSubAccount(t.Context(), credentials, subAccount), errTransferSubAccountInvalid, "validateUserSignedSubAccount should reject %s", subAccount)
	}
	assert.ErrorIs(t, ex.validateUserSignedSubAccount(t.Context(), &accounts.Credentials{Key: "invalid"}, testSubAccountAddress), errInvalidAddress, "validateUserSignedSubAccount should reject an invalid account")
	assert.Error(t, newUnavailableServerExchange(t).validateUserSignedSubAccount(t.Context(), credentials, testSubAccountAddress), "validateUserSignedSubAccount should return a role failure")
}

func TestAbstractionUser(t *testing.T) {
	t.Parallel()
	ex, _ := newRoleServerExchange(t, map[string]string{testSubAccountAddress: `{"role":"subAccount","data":{"master":"` + testAccountAddress + `"}}`}, nil, nil)
	credentials := signingCredentials()
	for _, tc := range []struct {
		subAccount, user string
		exp              string
		err              error
	}{
		{exp: testAccountAddress},
		{subAccount: testSubAccountAddress, exp: testSubAccountAddress},
		{subAccount: testSubAccountAddress, user: "0x" + strings.ToUpper(testSubAccountAddress[2:]), exp: testSubAccountAddress},
		{subAccount: testSubAccountAddress, user: testAccountAddress, exp: testAccountAddress},
		{user: "0x" + strings.ToUpper(testTraderAddress[2:]), exp: testTraderAddress},
		{subAccount: testSubAccountAddress, user: strings.ToUpper(testAccountAddress[2:]), err: errInvalidAddress},
		{subAccount: testVaultAddress, err: errTransferSubAccountInvalid},
		{subAccount: testVaultAddress, user: testVaultAddress, err: errTransferSubAccountInvalid},
	} {
		user, err := ex.abstractionUser(t.Context(), credentials, tc.subAccount, tc.user)
		require.ErrorIsf(t, err, tc.err, "abstractionUser must return the expected error for subaccount %q and user %q", tc.subAccount, tc.user)
		assert.Equalf(t, tc.exp, user, "abstractionUser should resolve subaccount %q and user %q", tc.subAccount, tc.user)
	}
}

func TestResolveTransferToken(t *testing.T) {
	t.Parallel()
	_, _, err := e.resolveTransferToken(t.Context(), currency.EMPTYCODE)
	require.ErrorIs(t, err, errTransferTokenInvalid, "resolveTransferToken must reject an empty token")
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "resolveTransferToken must report the empty token")

	token, identifier, err := e.resolveTransferToken(t.Context(), currency.HYPE)
	require.NoError(t, err, "resolveTransferToken must not error")
	if mockTests {
		assert.Equal(t, uint64(150), token.Index, "resolveTransferToken should return the token's metadata")
		assert.Equal(t, "HYPE:0x0d01dc56dcaaca66ad901c959b4011ec", identifier, "resolveTransferToken should return the token's NAME:TOKEN_ID identifier")
	} else {
		assert.True(t, strings.HasPrefix(identifier, "HYPE:0x"), "resolveTransferToken should return the token's NAME:TOKEN_ID identifier")
	}
	_, _, err = e.resolveTransferToken(t.Context(), currency.NewCode("NOTATOKEN"))
	assert.ErrorIs(t, err, errTransferTokenInvalid, "resolveTransferToken should reject an unknown token")
	ambiguous := newInfoServerExchange(t, map[string]string{"spotMeta": `{"tokens":[{"name":"HYPE","index":150},{"name":"HYPE","index":151}]}`}, nil)
	_, _, err = ambiguous.resolveTransferToken(t.Context(), currency.HYPE)
	assert.ErrorIs(t, err, errTransferTokenInvalid, "resolveTransferToken should reject a name shared by two tokens")
	_, _, err = newUnavailableServerExchange(t).resolveTransferToken(t.Context(), currency.HYPE)
	assert.Error(t, err, "resolveTransferToken should return a metadata failure")
}

func TestResolveTransferDEX(t *testing.T) {
	t.Parallel()
	for dex, exp := range map[string]string{"": "", " ": "", "SPOT": "spot", "xyz": "xyz"} {
		resolved, err := e.resolveTransferDEX(t.Context(), dex)
		require.NoErrorf(t, err, "resolveTransferDEX must not error for %q", dex)
		assert.Equalf(t, exp, resolved, "resolveTransferDEX should resolve %q", dex)
	}
	_, err := e.resolveTransferDEX(t.Context(), "notadex")
	assert.ErrorIs(t, err, errTransferDEXInvalid, "resolveTransferDEX should reject an unregistered DEX")
	_, err = newUnavailableServerExchange(t).resolveTransferDEX(t.Context(), "xyz")
	assert.Error(t, err, "resolveTransferDEX should return a registry failure")
}

func TestValidateSendAssetRoute(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		source, destination string
		token               currency.Code
		exp                 [3]string
	}{
		{destination: "spot", token: currency.USDC, exp: [3]string{"", "spot", "USDC"}},
		{source: "spot", destination: "spot", token: currency.HYPE, exp: [3]string{"spot", "spot", "HYPE:0x0d01dc56dcaaca66ad901c959b4011ec"}},
		{source: "xyz", destination: "", token: currency.USDC, exp: [3]string{"xyz", "", "USDC"}},
	} {
		sourceDEX, destinationDEX, token, err := e.validateSendAssetRoute(t.Context(), tc.source, tc.destination, tc.token)
		require.NoErrorf(t, err, "validateSendAssetRoute must not error from %q to %q", tc.source, tc.destination)
		if mockTests {
			assert.Equalf(t, tc.exp, [3]string{sourceDEX, destinationDEX, token}, "validateSendAssetRoute should resolve the route from %q to %q", tc.source, tc.destination)
		}
	}
	for _, tc := range []struct {
		source, destination string
		token               currency.Code
		err                 error
	}{
		{source: "notadex", token: currency.USDC, err: errTransferDEXInvalid},
		{destination: "notadex", token: currency.USDC, err: errTransferDEXInvalid},
		{token: currency.NewCode("NOTATOKEN"), err: errTransferTokenInvalid},
		{destination: "xyz", token: currency.HYPE, err: errTransferTokenInvalid},
	} {
		sourceDEX, destinationDEX, token, err := e.validateSendAssetRoute(t.Context(), tc.source, tc.destination, tc.token)
		assert.ErrorIsf(t, err, tc.err, "validateSendAssetRoute should reject %s from %q to %q", tc.token, tc.source, tc.destination)
		assert.Equalf(t, [3]string{}, [3]string{sourceDEX, destinationDEX, token}, "validateSendAssetRoute should return no route for %s from %q to %q", tc.token, tc.source, tc.destination)
	}
	ex := newInfoServerExchange(t, map[string]string{"spotMeta": `{"tokens":[{"name":"USDC","index":0}]}`, "meta": `{`}, nil)
	sourceDEX, destinationDEX, token, err := ex.validateSendAssetRoute(t.Context(), "", "spot", currency.USDC)
	assert.Error(t, err, "validateSendAssetRoute should return a metadata failure")
	assert.Equal(t, [3]string{}, [3]string{sourceDEX, destinationDEX, token}, "validateSendAssetRoute should return no route after a metadata failure")
}
