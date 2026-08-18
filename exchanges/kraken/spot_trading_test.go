package kraken

import (
	"math"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

var spotTradingFixtures = spotFixtureSet{results: map[string]string{
	"/0/private/AmendOrder":           `{"amend_id":"AMEND"}`,
	"/0/private/AddOrder":             `{"descr":{"order":"buy 1 BTC/USD","close":"close @ 90"},"txid":["ORDER"]}`,
	"/0/private/CancelOrder":          `{"count":1,"pending":false}`,
	"/0/private/CancelAll":            `{"count":2,"pending":false}`,
	"/0/private/CancelAllOrdersAfter": `{"currentTime":"2026-08-02T00:00:00Z","triggerTime":"2026-08-02T00:00:00Z"}`,
	"/0/private/AddOrderBatch":        `{"orders":[{"descr":{"order":"buy 1 XBTUSD"},"txid":"ORDER"}]}`,
	"/0/private/CancelOrderBatch":     `{"count":3}`,
}}

func TestFormatOrderPrice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		price         *OrderPrice
		expected      string
		expectedError error
	}{
		{name: "omitted"},
		{name: "explicit zero", price: &OrderPrice{}, expected: "0"},
		{name: "absolute", price: &OrderPrice{Value: 100.25}, expected: "100.25"},
		{name: "small absolute", price: &OrderPrice{Value: 0.000000001}, expected: "0.000000001"},
		{name: "relative addition", price: &OrderPrice{Expression: "+5"}, expected: "+5"},
		{name: "relative subtraction", price: &OrderPrice{Expression: "-5"}, expected: "-5"},
		{name: "relative static", price: &OrderPrice{Expression: "#5"}, expected: "#5"},
		{name: "relative percentage", price: &OrderPrice{Expression: "+5%"}, expected: "+5%"},
		{name: "conflicting representations", price: &OrderPrice{Value: 1, Expression: "+1"}, expectedError: errOrderPriceConflict},
		{name: "negative absolute", price: &OrderPrice{Value: -1}, expectedError: errOrderPriceInvalid},
		{name: "NaN absolute", price: &OrderPrice{Value: math.NaN()}, expectedError: errOrderPriceInvalid},
		{name: "infinite absolute", price: &OrderPrice{Value: math.Inf(1)}, expectedError: errOrderPriceInvalid},
		{name: "missing prefix", price: &OrderPrice{Expression: "5"}, expectedError: errOrderPriceInvalid},
		{name: "missing relative value", price: &OrderPrice{Expression: "+"}, expectedError: errOrderPriceInvalid},
		{name: "negative relative magnitude", price: &OrderPrice{Expression: "+-5"}, expectedError: errOrderPriceInvalid},
		{name: "invalid relative value", price: &OrderPrice{Expression: "+price"}, expectedError: errOrderPriceInvalid},
		{name: "infinite relative value", price: &OrderPrice{Expression: "+Inf"}, expectedError: errOrderPriceInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatOrderPrice(tc.price)
			require.ErrorIs(t, err, tc.expectedError, "formatOrderPrice must return the expected error")
			assert.Equal(t, tc.expected, result, "formatOrderPrice should return the expected wire value")
		})
	}
}

func TestFormatScheduledTime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		absolute        time.Time
		relative        time.Duration
		minimumRelative time.Duration
		expected        string
		expectedError   error
	}{
		{name: "omitted", minimumRelative: time.Second},
		{name: "epoch", absolute: time.Unix(0, 0), minimumRelative: time.Second, expected: "0"},
		{name: "absolute", absolute: time.Unix(123, 0), minimumRelative: time.Second, expected: "123"},
		{name: "minimum relative", relative: 5 * time.Second, minimumRelative: 5 * time.Second, expected: "+5"},
		{name: "conflicting representations", absolute: time.Unix(123, 0), relative: time.Second, minimumRelative: time.Second, expectedError: errScheduledTimeConflict},
		{name: "pre-epoch absolute", absolute: time.Unix(-1, 0), minimumRelative: time.Second, expectedError: errScheduledTimeInvalid},
		{name: "negative relative", relative: -time.Second, minimumRelative: time.Second, expectedError: errScheduledTimeInvalid},
		{name: "below minimum relative", relative: 4 * time.Second, minimumRelative: 5 * time.Second, expectedError: errScheduledTimeInvalid},
		{name: "fractional relative", relative: 1500 * time.Millisecond, minimumRelative: time.Second, expectedError: errScheduledTimeInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatScheduledTime(tc.absolute, tc.relative, tc.minimumRelative)
			require.ErrorIs(t, err, tc.expectedError, "formatScheduledTime must return the expected error")
			assert.Equal(t, tc.expected, result, "formatScheduledTime should return the expected wire value")
		})
	}
}

func TestFormatOrderFlags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		flags         []OrderFlag
		expected      string
		expectedError error
	}{
		{name: "omitted"},
		{name: "documented flags", flags: []OrderFlag{OrderFlagPostOnly, OrderFlagFeeInBase, OrderFlagFeeInQuote, OrderFlagVolumeInQuote, OrderFlagNoMarketProtect}, expected: "post,fcib,fciq,viqc,nompp"},
		{name: "empty flag", flags: []OrderFlag{""}, expectedError: errOrderFlagInvalid},
		{name: "invalid flag", flags: []OrderFlag{"invalid"}, expectedError: errOrderFlagInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatOrderFlags(tc.flags)
			require.ErrorIs(t, err, tc.expectedError, "formatOrderFlags must return the expected error")
			assert.Equal(t, tc.expected, result, "formatOrderFlags should return the expected wire value")
		})
	}
}

func TestFormatDeadline(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	location := time.FixedZone("AEST", 10*60*60)
	for _, tc := range []struct {
		name          string
		deadline      time.Time
		expected      string
		expectedError error
	}{
		{name: "omitted"},
		{name: "too soon", deadline: now.Add(2*time.Second - time.Nanosecond), expectedError: errDeadlineInvalid},
		{name: "too late", deadline: now.Add(time.Minute + time.Nanosecond), expectedError: errDeadlineInvalid},
		{name: "minimum", deadline: now.Add(2 * time.Second), expected: "2026-08-05T00:00:02Z"},
		{name: "maximum in another location", deadline: now.Add(time.Minute).In(location), expected: "2026-08-05T00:01:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatDeadline(tc.deadline, now)
			require.ErrorIs(t, err, tc.expectedError, "formatDeadline must return the expected error")
			assert.Equal(t, tc.expected, result, "formatDeadline should return the expected UTC value")
		})
	}
}

func TestAmendOrder(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotTradingFixtures)
	ctx := t.Context()
	deadline := time.Now().Add(30 * time.Second).In(time.FixedZone("AEST", 10*60*60))
	orderQuantity := 2.0
	displayQuantity := 1.0

	_, err := ex.AmendOrder(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AmendOrder must reject a nil request")
	_, err = ex.AmendOrder(ctx, &AmendOrderRequest{})
	require.ErrorIs(t, err, errOrderIdentityRequired, "AmendOrder must require an order identifier")
	_, err = ex.AmendOrder(ctx, &AmendOrderRequest{TransactionID: "ORDER", ClientOrderID: "CLIENT"})
	require.ErrorIs(t, err, errOrderIdentityConflict, "AmendOrder must reject conflicting order identifiers")
	amended, err := ex.AmendOrder(ctx, &AmendOrderRequest{
		TransactionID:   "ORDER",
		OrderQuantity:   &orderQuantity,
		DisplayQuantity: &displayQuantity,
		LimitPrice:      &OrderPrice{Value: 100},
		TriggerPrice:    &OrderPrice{Value: 90},
		Pair:            spotTestPair,
		PostOnly:        true,
		Deadline:        deadline,
	})
	require.NoError(t, err, "AmendOrder must not error")
	require.NotNil(t, amended, "AmendOrder must return a response")
	assert.Equal(t, "AMEND", amended.AmendID, "AmendOrder should decode the amend identifier")
	responseJSON, err := json.Marshal(amended)
	require.NoError(t, err, "AmendOrder must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"amend_id":"AMEND"`, "AmendOrder should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/AmendOrder")
	assert.Equal(t, "ORDER", values.Get("txid"), "AmendOrder should encode the transaction identifier")
	assert.Equal(t, "2", values.Get("order_qty"), "AmendOrder should encode order quantity")
	assert.Equal(t, "1", values.Get("display_qty"), "AmendOrder should encode display quantity")
	assert.Equal(t, "100", values.Get("limit_price"), "AmendOrder should encode limit price")
	assert.Equal(t, "90", values.Get("trigger_price"), "AmendOrder should encode trigger price")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "AmendOrder should encode the formatted pair")
	assert.Equal(t, "true", values.Get("post_only"), "AmendOrder should encode post-only")
	assert.Equal(t, deadline.UTC().Format(time.RFC3339Nano), values.Get("deadline"), "AmendOrder should encode deadline in UTC")

	_, err = ex.AmendOrder(ctx, &AmendOrderRequest{ClientOrderID: "CLIENT"})
	require.NoError(t, err, "AmendOrder must accept a client order identifier")
	values = requireSpotRequest(t, requests, "/0/private/AmendOrder")
	assert.Empty(t, values.Get("txid"), "AmendOrder should omit an empty transaction identifier")
	negative := -1.0
	notANumber := math.NaN()
	invalidDeadline := time.Now().Add(time.Second)
	for _, tc := range []struct {
		name     string
		request  AmendOrderRequest
		expected error
	}{
		{name: "negative order quantity", request: AmendOrderRequest{TransactionID: "ORDER", OrderQuantity: &negative}, expected: errVolumeInvalid},
		{name: "non-finite order quantity", request: AmendOrderRequest{TransactionID: "ORDER", OrderQuantity: &notANumber}, expected: errNumericValueInvalid},
		{name: "negative display quantity", request: AmendOrderRequest{TransactionID: "ORDER", DisplayQuantity: &negative}, expected: errVolumeInvalid},
		{name: "non-finite display quantity", request: AmendOrderRequest{TransactionID: "ORDER", DisplayQuantity: &notANumber}, expected: errNumericValueInvalid},
		{name: "invalid limit price", request: AmendOrderRequest{TransactionID: "ORDER", LimitPrice: &OrderPrice{Expression: "invalid"}}, expected: errOrderPriceInvalid},
		{name: "unsupported relative limit price", request: AmendOrderRequest{TransactionID: "ORDER", LimitPrice: &OrderPrice{Expression: "#1"}}, expected: errOrderPriceInvalid},
		{name: "invalid trigger price", request: AmendOrderRequest{TransactionID: "ORDER", TriggerPrice: &OrderPrice{Expression: "invalid"}}, expected: errOrderPriceInvalid},
		{name: "unsupported relative trigger price", request: AmendOrderRequest{TransactionID: "ORDER", TriggerPrice: &OrderPrice{Expression: "#1"}}, expected: errOrderPriceInvalid},
		{name: "invalid deadline", request: AmendOrderRequest{TransactionID: "ORDER", Deadline: invalidDeadline}, expected: errDeadlineInvalid},
		{name: "partially populated pair", request: AmendOrderRequest{TransactionID: "ORDER", Pair: currency.Pair{Base: currency.BTC}}, expected: errPairRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.AmendOrder(ctx, &tc.request)
			require.ErrorIs(t, err, tc.expected, "AmendOrder must reject "+tc.name)
		})
	}
	_, err = new(Exchange).AmendOrder(ctx, &AmendOrderRequest{TransactionID: "ORDER", Pair: spotTestPair})
	require.Error(t, err, "AmendOrder must surface pair-format errors")

	amended, err = newSpotNullResultExchange(t).AmendOrder(ctx, &AmendOrderRequest{TransactionID: "ORDER"})
	require.NoError(t, err, "AmendOrder must accept a null result")
	assert.Nil(t, amended, "AmendOrder should return nil for a null result")
	amended, err = newSpotErrorExchange(t).AmendOrder(ctx, &AmendOrderRequest{TransactionID: "ORDER"})
	require.ErrorIs(t, err, errSpotTransport, "AmendOrder must surface request errors")
	assert.Nil(t, amended, "AmendOrder result should remain nil on request errors")
}

func TestCancelAllOpenOrders(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotTradingFixtures)
	ctx := t.Context()

	cancelled, err := ex.CancelAllOpenOrders(ctx)
	require.NoError(t, err, "CancelAllOpenOrders must not error")
	require.NotNil(t, cancelled, "CancelAllOpenOrders must return a response")
	assert.Equal(t, int64(2), cancelled.Count, "CancelAllOpenOrders should decode the cancellation count")
	responseJSON, err := json.Marshal(cancelled)
	require.NoError(t, err, "CancelAllOpenOrders must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"count":2`, "CancelAllOpenOrders should decode the response")
	requireSpotRequest(t, requests, "/0/private/CancelAll")

	cancelled, err = newSpotNullResultExchange(t).CancelAllOpenOrders(ctx)
	require.NoError(t, err, "CancelAllOpenOrders must accept a null result")
	assert.Nil(t, cancelled, "CancelAllOpenOrders should return nil for a null result")
	cancelled, err = newSpotErrorExchange(t).CancelAllOpenOrders(ctx)
	require.ErrorIs(t, err, errSpotTransport, "CancelAllOpenOrders must surface request errors")
	assert.Nil(t, cancelled, "CancelAllOpenOrders result should remain nil on request errors")
}

func TestCancelAllOrdersAfter(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotTradingFixtures)
	ctx := t.Context()

	_, err := ex.CancelAllOrdersAfter(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CancelAllOrdersAfter must reject a nil request")
	_, err = ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{Timeout: 24 * time.Hour})
	require.ErrorIs(t, err, errTimeoutTooLarge, "CancelAllOrdersAfter must enforce Kraken's timeout limit")
	_, err = ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{Timeout: -time.Second})
	require.ErrorIs(t, err, errTimeoutInvalid, "CancelAllOrdersAfter must reject negative timeouts")
	_, err = ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{Timeout: time.Millisecond})
	require.ErrorIs(t, err, errTimeoutInvalid, "CancelAllOrdersAfter must reject fractional-second timeouts")
	deadMan, err := ex.CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{})
	require.NoError(t, err, "CancelAllOrdersAfter must accept zero to disable the timer")
	require.NotNil(t, deadMan, "CancelAllOrdersAfter must return a response")
	assert.False(t, deadMan.CurrentTime.IsZero(), "CancelAllOrdersAfter should decode the current time")
	responseJSON, err := json.Marshal(deadMan)
	require.NoError(t, err, "CancelAllOrdersAfter must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"currentTime":"2026-08-02T00:00:00Z"`, "CancelAllOrdersAfter should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/CancelAllOrdersAfter")
	assert.Equal(t, "0", values.Get("timeout"), "CancelAllOrdersAfter should encode a zero timeout")

	deadMan, err = newSpotNullResultExchange(t).CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{})
	require.NoError(t, err, "CancelAllOrdersAfter must accept a null result")
	assert.Nil(t, deadMan, "CancelAllOrdersAfter should return nil for a null result")
	deadMan, err = newSpotErrorExchange(t).CancelAllOrdersAfter(ctx, &CancelAllOrdersAfterRequest{})
	require.ErrorIs(t, err, errSpotTransport, "CancelAllOrdersAfter must surface request errors")
	assert.Nil(t, deadMan, "CancelAllOrdersAfter result should remain nil on request errors")
}

func TestAddOrderBatch(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotTradingFixtures)
	ctx := t.Context()
	deadline := time.Now().Add(30 * time.Second).In(time.FixedZone("AEST", 10*60*60))

	_, err := ex.AddOrderBatch(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AddOrderBatch must reject a nil request")
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{})
	require.ErrorIs(t, err, errBatchOrderCount, "AddOrderBatch must require at least two orders")
	tooManyOrders := make([]AddOrderBatchOrderRequest, 16)
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: tooManyOrders})
	require.ErrorIs(t, err, errBatchOrderCount, "AddOrderBatch must reject more than fifteen orders")
	validOrders := []AddOrderBatchOrderRequest{
		{OrderType: OrderTypeLimit, OrderSide: OrderSideBuy, Volume: 1, Price: &OrderPrice{Value: 100}, Close: &AddOrderBatchCloseRequest{OrderType: OrderTypeStopLoss, Price: &OrderPrice{Value: 90}}},
		{OrderType: OrderTypeLimit, OrderSide: OrderSideSell, Volume: 1, Price: &OrderPrice{Value: 110}},
	}
	zeroUserReference := int32(0)
	validOrders[0].UserReference = &zeroUserReference
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: validOrders})
	require.ErrorIs(t, err, errPairRequired, "AddOrderBatch must require a pair")
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: validOrders, Pair: spotTestPair, AssetClass: AssetClassCurrency})
	require.ErrorIs(t, err, errAssetClassInvalid, "AddOrderBatch must reject an invalid asset class")
	invalidOrders := slices.Clone(validOrders)
	invalidOrders[1].OrderType = ""
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchOrderFields, "AddOrderBatch must require fields on every order")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].OrderType = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchOrderTypeInvalid, "AddOrderBatch must reject an invalid order type")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].OrderSide = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchSideInvalid, "AddOrderBatch must reject an invalid order side")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].Trigger = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchTriggerInvalid, "AddOrderBatch must reject an invalid trigger")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].SelfTradePolicy = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchSelfTradeInvalid, "AddOrderBatch must reject an invalid self-trade policy")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[1].TimeInForce = "invalid"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchTimeInForceInvalid, "AddOrderBatch must reject an invalid time-in-force")
	invalidOrders = slices.Clone(validOrders)
	conflictingUserReference := int32(1)
	invalidOrders[1].UserReference = &conflictingUserReference
	invalidOrders[1].ClientOrderID = "CLIENT"
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchIdentityConflict, "AddOrderBatch must reject conflicting client identifiers")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[0].Close = new(AddOrderBatchCloseRequest)
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchCloseTypeInvalid, "AddOrderBatch must require a conditional close order type")
	invalidOrders = slices.Clone(validOrders)
	invalidOrders[0].Close = &AddOrderBatchCloseRequest{OrderType: "invalid"}
	_, err = ex.AddOrderBatch(ctx, &AddOrderBatchRequest{Orders: invalidOrders, Pair: spotTestPair})
	require.ErrorIs(t, err, errBatchCloseTypeInvalid, "AddOrderBatch must reject an invalid conditional close order type")
	batch, err := ex.AddOrderBatch(ctx, &AddOrderBatchRequest{
		Orders:     validOrders,
		Pair:       spotTestPair,
		AssetClass: AssetClassTokenizedAsset,
		Deadline:   deadline,
		Validate:   true,
		Broker:     "BROKER",
	})
	require.NoError(t, err, "AddOrderBatch must not error")
	require.NotNil(t, batch, "AddOrderBatch must return a response")
	assert.Equal(t, "ORDER", batch.Orders[0].Transaction, "AddOrderBatch should decode order identifiers")
	responseJSON, err := json.Marshal(batch)
	require.NoError(t, err, "AddOrderBatch must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"txid":"ORDER"`, "AddOrderBatch should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/AddOrderBatch")
	assert.Contains(t, values.Get("orders"), `"close"`, "AddOrderBatch should encode conditional close orders")
	assert.Contains(t, values.Get("orders"), `"userref":0`, "AddOrderBatch should encode an explicit zero user reference")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "AddOrderBatch should encode the formatted pair")
	assert.Equal(t, "BROKER", values.Get("broker"), "AddOrderBatch should encode broker")
	assert.Equal(t, "true", values.Get("validate"), "AddOrderBatch should encode validation mode")
	assert.Equal(t, deadline.UTC().Format(time.RFC3339Nano), values.Get("deadline"), "AddOrderBatch should encode deadline in UTC")

	negative := -1.0
	notANumber := math.NaN()
	invalidDeadline := time.Now().Add(time.Second)
	validRequest := func() *AddOrderBatchRequest {
		return &AddOrderBatchRequest{
			Orders: []AddOrderBatchOrderRequest{
				{OrderType: OrderTypeLimit, OrderSide: OrderSideBuy, Volume: 1},
				{OrderType: OrderTypeLimit, OrderSide: OrderSideSell, Volume: 1},
			},
			Pair: spotTestPair,
		}
	}
	for _, tc := range []struct {
		name     string
		mutate   func(*AddOrderBatchOrderRequest)
		expected error
	}{
		{name: "negative volume", mutate: func(req *AddOrderBatchOrderRequest) { req.Volume = negative }, expected: errVolumeInvalid},
		{name: "non-finite volume", mutate: func(req *AddOrderBatchOrderRequest) { req.Volume = notANumber }, expected: errNumericValueInvalid},
		{name: "negative display volume", mutate: func(req *AddOrderBatchOrderRequest) { req.DisplayVolume = &negative }, expected: errVolumeInvalid},
		{name: "non-finite display volume", mutate: func(req *AddOrderBatchOrderRequest) { req.DisplayVolume = &notANumber }, expected: errNumericValueInvalid},
		{name: "invalid price", mutate: func(req *AddOrderBatchOrderRequest) { req.Price = &OrderPrice{Expression: "invalid"} }, expected: errOrderPriceInvalid},
		{name: "invalid secondary price", mutate: func(req *AddOrderBatchOrderRequest) { req.SecondaryPrice = &OrderPrice{Expression: "invalid"} }, expected: errOrderPriceInvalid},
		{name: "invalid order flag", mutate: func(req *AddOrderBatchOrderRequest) { req.OrderFlags = []OrderFlag{"invalid"} }, expected: errOrderFlagInvalid},
		{name: "invalid start delay", mutate: func(req *AddOrderBatchOrderRequest) { req.StartDelay = time.Millisecond }, expected: errScheduledTimeInvalid},
		{name: "invalid expiry delay", mutate: func(req *AddOrderBatchOrderRequest) { req.ExpireAfter = time.Second }, expected: errScheduledTimeInvalid},
		{name: "invalid close price", mutate: func(req *AddOrderBatchOrderRequest) {
			req.Close = &AddOrderBatchCloseRequest{OrderType: OrderTypeLimit, Price: &OrderPrice{Expression: "invalid"}}
		}, expected: errOrderPriceInvalid},
		{name: "invalid close secondary price", mutate: func(req *AddOrderBatchOrderRequest) {
			req.Close = &AddOrderBatchCloseRequest{OrderType: OrderTypeLimit, SecondaryPrice: &OrderPrice{Expression: "invalid"}}
		}, expected: errOrderPriceInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest()
			tc.mutate(&req.Orders[0])
			_, err := ex.AddOrderBatch(ctx, req)
			require.ErrorIs(t, err, tc.expected, "AddOrderBatch must reject "+tc.name)
		})
	}
	invalidBatchDeadline := validRequest()
	invalidBatchDeadline.Deadline = invalidDeadline
	_, err = ex.AddOrderBatch(ctx, invalidBatchDeadline)
	require.ErrorIs(t, err, errDeadlineInvalid, "AddOrderBatch must reject an invalid deadline")

	displayVolume := 0.5
	richBatch := validRequest()
	richBatch.Orders[0].DisplayVolume = &displayVolume
	richBatch.Orders[0].SecondaryPrice = &OrderPrice{Value: 90}
	richBatch.Orders[0].Leverage = 2
	richBatch.Orders[0].OrderFlags = []OrderFlag{OrderFlagPostOnly}
	richBatch.Orders[0].StartDelay = time.Second
	richBatch.Orders[0].ExpireAfter = 5 * time.Second
	richBatch.Orders[0].Close = &AddOrderBatchCloseRequest{OrderType: OrderTypeLimit, Price: &OrderPrice{Value: 80}, SecondaryPrice: &OrderPrice{Value: 70}}
	beforeOrders := slices.Clone(richBatch.Orders)
	_, err = ex.AddOrderBatch(ctx, richBatch)
	require.NoError(t, err, "AddOrderBatch must accept typed optional fields")
	assert.Equal(t, beforeOrders, richBatch.Orders, "AddOrderBatch should not mutate caller orders")
	values = requireSpotRequest(t, requests, "/0/private/AddOrderBatch")
	assert.Contains(t, values.Get("orders"), `"displayvol":"0.5"`, "AddOrderBatch should encode display volume")
	assert.Contains(t, values.Get("orders"), `"leverage":"2"`, "AddOrderBatch should encode leverage")
	assert.Contains(t, values.Get("orders"), `"price2":"90"`, "AddOrderBatch should encode secondary price")
	assert.Contains(t, values.Get("orders"), `"starttm":"+1"`, "AddOrderBatch should encode start delay")
	assert.Contains(t, values.Get("orders"), `"expiretm":"+5"`, "AddOrderBatch should encode expiry delay")
	assert.Contains(t, values.Get("orders"), `"oflags":"post"`, "AddOrderBatch should encode order flags")
	assert.Contains(t, values.Get("orders"), `"price2":"70"`, "AddOrderBatch should encode the close secondary price")
	_, err = new(Exchange).AddOrderBatch(ctx, validRequest())
	require.Error(t, err, "AddOrderBatch must surface pair-format errors")

	testValue := func(t *testing.T, mutate func(*AddOrderBatchRequest)) (addOrderBatchWireOrder, url.Values) {
		t.Helper()
		req := validRequest()
		mutate(req)
		_, err := ex.AddOrderBatch(t.Context(), req)
		require.NoError(t, err, "AddOrderBatch must accept the documented enum value")
		values := requireSpotRequest(t, requests, "/0/private/AddOrderBatch")
		var orders []addOrderBatchWireOrder
		require.NoError(t, json.Unmarshal([]byte(values.Get("orders")), &orders), "AddOrderBatch must encode valid orders JSON")
		require.Len(t, orders, len(req.Orders), "AddOrderBatch must encode every order")
		return orders[0], values
	}
	for _, value := range []OrderType{OrderTypeMarket, OrderTypeLimit, OrderTypeIceberg, OrderTypeStopLoss, OrderTypeTakeProfit, OrderTypeStopLossLimit, OrderTypeTakeProfitLimit, OrderTypeTrailingStop, OrderTypeTrailingStopLimit, OrderTypeSettlePosition} {
		t.Run("order type "+string(value), func(t *testing.T) {
			order, _ := testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].OrderType = value })
			assert.Equal(t, string(value), order.OrderType, "AddOrderBatch should encode the documented order type")
		})
	}
	for _, value := range []OrderSide{OrderSideBuy, OrderSideSell} {
		t.Run("side "+string(value), func(t *testing.T) {
			order, _ := testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].OrderSide = value })
			assert.Equal(t, string(value), order.OrderSide, "AddOrderBatch should encode the documented order side")
		})
	}
	for _, value := range []OrderTrigger{OrderTriggerIndex, OrderTriggerLast} {
		t.Run("trigger "+string(value), func(t *testing.T) {
			order, _ := testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].Trigger = value })
			assert.Equal(t, string(value), order.Trigger, "AddOrderBatch should encode the documented trigger")
		})
	}
	for _, value := range []SelfTradePolicy{SelfTradePolicyCancelNewest, SelfTradePolicyCancelOldest, SelfTradePolicyCancelBoth} {
		t.Run("self trade "+string(value), func(t *testing.T) {
			order, _ := testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].SelfTradePolicy = value })
			assert.Equal(t, string(value), order.SelfTradePolicy, "AddOrderBatch should encode the documented self-trade policy")
		})
	}
	for _, value := range []OrderTimeInForce{OrderTimeInForceGTC, OrderTimeInForceIOC, OrderTimeInForceGTD} {
		t.Run("time in force "+string(value), func(t *testing.T) {
			order, _ := testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].TimeInForce = value })
			assert.Equal(t, string(value), order.TimeInForce, "AddOrderBatch should encode the documented time in force")
		})
	}
	for _, value := range []OrderType{OrderTypeLimit, OrderTypeIceberg, OrderTypeStopLoss, OrderTypeTakeProfit, OrderTypeStopLossLimit, OrderTypeTakeProfitLimit, OrderTypeTrailingStop, OrderTypeTrailingStopLimit} {
		t.Run("close order type "+string(value), func(t *testing.T) {
			order, _ := testValue(t, func(req *AddOrderBatchRequest) { req.Orders[0].Close = &AddOrderBatchCloseRequest{OrderType: value} })
			require.NotNil(t, order.Close, "AddOrderBatch must encode the close order")
			assert.Equal(t, string(value), order.Close.OrderType, "AddOrderBatch should encode the documented close order type")
		})
	}
	t.Run("asset class tokenized asset", func(t *testing.T) {
		_, values := testValue(t, func(req *AddOrderBatchRequest) { req.AssetClass = AssetClassTokenizedAsset })
		assert.Equal(t, string(AssetClassTokenizedAsset), values.Get("asset_class"), "AddOrderBatch should encode the documented asset class")
	})

	batch, err = newSpotNullResultExchange(t).AddOrderBatch(ctx, validRequest())
	require.NoError(t, err, "AddOrderBatch must accept a null result")
	assert.Nil(t, batch, "AddOrderBatch should return nil for a null result")
	batch, err = newSpotErrorExchange(t).AddOrderBatch(ctx, validRequest())
	require.ErrorIs(t, err, errSpotTransport, "AddOrderBatch must surface request errors")
	assert.Nil(t, batch, "AddOrderBatch result should remain nil on request errors")
}

func TestCancelOrderBatch(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotTradingFixtures)
	ctx := t.Context()

	_, err := ex.CancelOrderBatch(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CancelOrderBatch must reject a nil request")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{})
	require.ErrorIs(t, err, errBatchCancelOrderCount, "CancelOrderBatch must require an identifier")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: make([]string, 51)})
	require.ErrorIs(t, err, errBatchCancelOrderCount, "CancelOrderBatch must reject more than fifty identifiers")
	batchCancelled, err := ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{
		TransactionIDs: []string{"ORDER"},
		UserReferences: []int32{42},
		ClientOrderIDs: []string{"CLIENT"},
	})
	require.NoError(t, err, "CancelOrderBatch must not error")
	require.NotNil(t, batchCancelled, "CancelOrderBatch must return a response")
	assert.Equal(t, uint64(3), batchCancelled.Count, "CancelOrderBatch should decode the cancellation count")
	responseJSON, err := json.Marshal(batchCancelled)
	require.NoError(t, err, "CancelOrderBatch must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"count":3`, "CancelOrderBatch should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/CancelOrderBatch")
	assert.Equal(t, `["ORDER",42]`, values.Get("orders"), "CancelOrderBatch should encode primitive transaction and user identifiers")
	assert.Equal(t, `["CLIENT"]`, values.Get("cl_ord_ids"), "CancelOrderBatch should encode client order identifiers")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{ClientOrderIDs: []string{"CLIENT"}})
	require.NoError(t, err, "CancelOrderBatch must accept only client order identifiers")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrderBatch")
	assert.Empty(t, values.Get("orders"), "CancelOrderBatch should omit an empty orders list")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: []string{"ORDER"}})
	require.NoError(t, err, "CancelOrderBatch must accept only transaction identifiers")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrderBatch")
	assert.Empty(t, values.Get("cl_ord_ids"), "CancelOrderBatch should omit an empty client order identifier list")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{UserReferences: []int32{42}})
	require.NoError(t, err, "CancelOrderBatch must accept only user references")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrderBatch")
	assert.Equal(t, `[42]`, values.Get("orders"), "CancelOrderBatch should encode user references without transaction identifiers")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: []string{""}})
	require.ErrorIs(t, err, errOrderIDRequired, "CancelOrderBatch must reject an empty transaction identifier")
	_, err = ex.CancelOrderBatch(ctx, &CancelOrderBatchRequest{ClientOrderIDs: []string{""}})
	require.ErrorIs(t, err, errOrderIDRequired, "CancelOrderBatch must reject an empty client order identifier")

	batchCancelled, err = newSpotNullResultExchange(t).CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: []string{"ORDER"}})
	require.NoError(t, err, "CancelOrderBatch must accept a null result")
	assert.Nil(t, batchCancelled, "CancelOrderBatch should return nil for a null result")
	batchCancelled, err = newSpotErrorExchange(t).CancelOrderBatch(ctx, &CancelOrderBatchRequest{TransactionIDs: []string{"ORDER"}})
	require.ErrorIs(t, err, errSpotTransport, "CancelOrderBatch must surface request errors")
	assert.Nil(t, batchCancelled, "CancelOrderBatch result should remain nil on request errors")
}

func TestAddOrder(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotTradingFixtures)
	ctx := t.Context()
	userReference := int32(0)
	deadline := time.Now().Add(30 * time.Second).In(time.FixedZone("AEST", 10*60*60))
	displayVolume := 0.5
	validRequest := &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Volume: 1, Pair: spotTestPair}

	_, err := ex.AddOrder(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AddOrder must reject a nil request")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{})
	require.ErrorIs(t, err, errOrderTypeInvalid, "AddOrder must require an order type")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: invalidSpotValue})
	require.ErrorIs(t, err, errOrderTypeInvalid, "AddOrder must reject an invalid order type")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: "limit"})
	require.ErrorIs(t, err, errOrderSideInvalid, "AddOrder must require an order side")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: "limit", Side: invalidSpotValue})
	require.ErrorIs(t, err, errOrderSideInvalid, "AddOrder must reject an invalid order side")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy})
	require.ErrorIs(t, err, errPairRequired, "AddOrder must require a pair")
	_, err = ex.AddOrder(ctx, &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Pair: spotTestPair})
	require.NoError(t, err, "AddOrder must accept an explicit zero volume")
	values := requireSpotRequest(t, requests, "/0/private/AddOrder")
	assert.Equal(t, "0", values.Get("volume"), "AddOrder should encode an explicit zero volume")
	conflictingIdentity := *validRequest
	conflictingIdentity.UserReference = &userReference
	conflictingIdentity.ClientOrderID = "CLIENT"
	_, err = ex.AddOrder(ctx, &conflictingIdentity)
	require.ErrorIs(t, err, errOrderIdentityConflict, "AddOrder must reject conflicting client identifiers")
	invalidRequest := *validRequest
	invalidRequest.AssetClass = "currency"
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errAssetClassInvalid, "AddOrder must reject an invalid asset class")
	invalidRequest = *validRequest
	invalidRequest.Trigger = invalidSpotValue
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errTriggerInvalid, "AddOrder must reject an invalid trigger")
	invalidRequest = *validRequest
	invalidRequest.SelfTradePolicy = invalidSpotValue
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errSelfTradeInvalid, "AddOrder must reject an invalid self-trade policy")
	invalidRequest = *validRequest
	invalidRequest.TimeInForce = invalidSpotValue
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errTimeInForceInvalid, "AddOrder must reject an invalid time-in-force")
	invalidRequest = *validRequest
	invalidRequest.Close = new(AddOrderCloseRequest)
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errBatchCloseTypeInvalid, "AddOrder must require a conditional close order type")
	invalidRequest = *validRequest
	invalidRequest.Close = &AddOrderCloseRequest{OrderType: invalidSpotValue}
	_, err = ex.AddOrder(ctx, &invalidRequest)
	require.ErrorIs(t, err, errBatchCloseTypeInvalid, "AddOrder must reject an invalid conditional close order type")

	added, err := ex.AddOrder(ctx, &AddOrderRequest{
		UserReference:   &userReference,
		OrderType:       OrderTypeLimit,
		Side:            OrderSideBuy,
		Volume:          1,
		DisplayVolume:   &displayVolume,
		Pair:            spotTestPair,
		AssetClass:      AssetClassTokenizedAsset,
		Price:           &OrderPrice{Value: 100},
		SecondaryPrice:  &OrderPrice{Value: 90},
		Trigger:         OrderTriggerLast,
		Leverage:        2,
		ReduceOnly:      true,
		SelfTradePolicy: SelfTradePolicyCancelBoth,
		OrderFlags:      []OrderFlag{OrderFlagPostOnly},
		TimeInForce:     OrderTimeInForceFOK,
		StartDelay:      5 * time.Second,
		ExpireAfter:     time.Minute,
		Close:           &AddOrderCloseRequest{OrderType: OrderTypeStopLossLimit, Price: &OrderPrice{Value: 80}, SecondaryPrice: &OrderPrice{Value: 75}},
		Deadline:        deadline,
		Validate:        true,
		Broker:          "BROKER",
	})
	require.NoError(t, err, "AddOrder must not error")
	require.NotNil(t, added, "AddOrder must return a response")
	assert.Equal(t, []string{"ORDER"}, added.TransactionIDs, "AddOrder should decode transaction IDs")
	responseJSON, err := json.Marshal(added)
	require.NoError(t, err, "AddOrder must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"txid":["ORDER"]`, "AddOrder should decode the response")
	values = requireSpotRequest(t, requests, "/0/private/AddOrder")
	assert.Equal(t, "0", values.Get("userref"), "AddOrder should encode an explicit zero user reference")
	assert.Equal(t, "0.5", values.Get("displayvol"), "AddOrder should encode display volume")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "AddOrder should encode the formatted pair")
	assert.Equal(t, "tokenized_asset", values.Get("asset_class"), "AddOrder should encode asset class")
	assert.Equal(t, "90", values.Get("price2"), "AddOrder should encode secondary price")
	assert.Equal(t, "last", values.Get("trigger"), "AddOrder should encode trigger")
	assert.Equal(t, "2", values.Get("leverage"), "AddOrder should encode leverage")
	assert.Equal(t, "true", values.Get("reduce_only"), "AddOrder should encode reduce-only")
	assert.Equal(t, "cancel-both", values.Get("stptype"), "AddOrder should encode self-trade policy")
	assert.Equal(t, "FOK", values.Get("timeinforce"), "AddOrder should encode time-in-force")
	assert.Equal(t, "+5", values.Get("starttm"), "AddOrder should preserve relative start time")
	assert.Equal(t, "+60", values.Get("expiretm"), "AddOrder should preserve relative expiry time")
	assert.Equal(t, "stop-loss-limit", values.Get("close[ordertype]"), "AddOrder should encode conditional close type")
	assert.Equal(t, "80", values.Get("close[price]"), "AddOrder should encode conditional close price")
	assert.Equal(t, "75", values.Get("close[price2]"), "AddOrder should encode conditional close secondary price")
	assert.Equal(t, deadline.UTC().Format(time.RFC3339Nano), values.Get("deadline"), "AddOrder should encode deadline in UTC")
	assert.Equal(t, "BROKER", values.Get("broker"), "AddOrder should encode broker")

	clientRequest := *validRequest
	clientRequest.ClientOrderID = "CLIENT"
	_, err = ex.AddOrder(ctx, &clientRequest)
	require.NoError(t, err, "AddOrder must accept a client order ID")
	values = requireSpotRequest(t, requests, "/0/private/AddOrder")
	assert.Equal(t, "CLIENT", values.Get("cl_ord_id"), "AddOrder should encode client order ID")
	assert.Empty(t, values.Get("userref"), "AddOrder should omit user reference when unset")
	assert.Empty(t, values.Get("close[price]"), "AddOrder should omit conditional close fields when unset")
	negative := -1.0
	notANumber := math.NaN()
	invalidDeadline := time.Now().Add(time.Second)
	for _, tc := range []struct {
		name     string
		mutate   func(*AddOrderRequest)
		expected error
	}{
		{name: "negative volume", mutate: func(req *AddOrderRequest) { req.Volume = negative }, expected: errVolumeInvalid},
		{name: "non-finite volume", mutate: func(req *AddOrderRequest) { req.Volume = notANumber }, expected: errNumericValueInvalid},
		{name: "negative display volume", mutate: func(req *AddOrderRequest) { req.DisplayVolume = &negative }, expected: errVolumeInvalid},
		{name: "non-finite display volume", mutate: func(req *AddOrderRequest) { req.DisplayVolume = &notANumber }, expected: errNumericValueInvalid},
		{name: "invalid price", mutate: func(req *AddOrderRequest) { req.Price = &OrderPrice{Expression: "invalid"} }, expected: errOrderPriceInvalid},
		{name: "invalid secondary price", mutate: func(req *AddOrderRequest) { req.SecondaryPrice = &OrderPrice{Expression: "invalid"} }, expected: errOrderPriceInvalid},
		{name: "invalid start delay", mutate: func(req *AddOrderRequest) { req.StartDelay = time.Millisecond }, expected: errScheduledTimeInvalid},
		{name: "invalid expiry delay", mutate: func(req *AddOrderRequest) { req.ExpireAfter = time.Second }, expected: errScheduledTimeInvalid},
		{name: "invalid deadline", mutate: func(req *AddOrderRequest) { req.Deadline = invalidDeadline }, expected: errDeadlineInvalid},
		{name: "invalid order flag", mutate: func(req *AddOrderRequest) { req.OrderFlags = []OrderFlag{"invalid"} }, expected: errOrderFlagInvalid},
		{name: "invalid close price", mutate: func(req *AddOrderRequest) {
			req.Close = &AddOrderCloseRequest{OrderType: OrderTypeLimit, Price: &OrderPrice{Expression: "invalid"}}
		}, expected: errOrderPriceInvalid},
		{name: "invalid close secondary price", mutate: func(req *AddOrderRequest) {
			req.Close = &AddOrderCloseRequest{OrderType: OrderTypeLimit, SecondaryPrice: &OrderPrice{Expression: "invalid"}}
		}, expected: errOrderPriceInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := *validRequest
			tc.mutate(&req)
			_, err := ex.AddOrder(ctx, &req)
			require.ErrorIs(t, err, tc.expected, "AddOrder must reject "+tc.name)
		})
	}
	_, err = new(Exchange).AddOrder(ctx, validRequest)
	require.Error(t, err, "AddOrder must surface pair-format errors")

	testValue := func(t *testing.T, mutate func(*AddOrderRequest)) url.Values {
		t.Helper()
		req := &AddOrderRequest{OrderType: OrderTypeLimit, Side: OrderSideBuy, Volume: 1, Pair: spotTestPair}
		mutate(req)
		_, err := ex.AddOrder(t.Context(), req)
		require.NoError(t, err, "AddOrder must accept the documented enum value")
		return requireSpotRequest(t, requests, "/0/private/AddOrder")
	}
	for _, value := range []OrderType{OrderTypeMarket, OrderTypeLimit, OrderTypeIceberg, OrderTypeStopLoss, OrderTypeTakeProfit, OrderTypeStopLossLimit, OrderTypeTakeProfitLimit, OrderTypeTrailingStop, OrderTypeTrailingStopLimit, OrderTypeSettlePosition} {
		t.Run("order type "+string(value), func(t *testing.T) {
			values := testValue(t, func(req *AddOrderRequest) { req.OrderType = value })
			assert.Equal(t, string(value), values.Get("ordertype"), "AddOrder should encode the documented order type")
		})
	}
	for _, value := range []OrderSide{OrderSideBuy, OrderSideSell} {
		t.Run("side "+string(value), func(t *testing.T) {
			values := testValue(t, func(req *AddOrderRequest) { req.Side = value })
			assert.Equal(t, string(value), values.Get("type"), "AddOrder should encode the documented order side")
		})
	}
	for _, value := range []OrderTrigger{OrderTriggerIndex, OrderTriggerLast} {
		t.Run("trigger "+string(value), func(t *testing.T) {
			values := testValue(t, func(req *AddOrderRequest) { req.Trigger = value })
			assert.Equal(t, string(value), values.Get("trigger"), "AddOrder should encode the documented trigger")
		})
	}
	for _, value := range []SelfTradePolicy{SelfTradePolicyCancelNewest, SelfTradePolicyCancelOldest, SelfTradePolicyCancelBoth} {
		t.Run("self trade "+string(value), func(t *testing.T) {
			values := testValue(t, func(req *AddOrderRequest) { req.SelfTradePolicy = value })
			assert.Equal(t, string(value), values.Get("stptype"), "AddOrder should encode the documented self-trade policy")
		})
	}
	for _, value := range []OrderTimeInForce{OrderTimeInForceGTC, OrderTimeInForceIOC, OrderTimeInForceGTD, OrderTimeInForceFOK} {
		t.Run("time in force "+string(value), func(t *testing.T) {
			values := testValue(t, func(req *AddOrderRequest) { req.TimeInForce = value })
			assert.Equal(t, string(value), values.Get("timeinforce"), "AddOrder should encode the documented time in force")
		})
	}
	for _, value := range []OrderType{OrderTypeLimit, OrderTypeIceberg, OrderTypeStopLoss, OrderTypeTakeProfit, OrderTypeStopLossLimit, OrderTypeTakeProfitLimit, OrderTypeTrailingStop, OrderTypeTrailingStopLimit} {
		t.Run("close order type "+string(value), func(t *testing.T) {
			values := testValue(t, func(req *AddOrderRequest) { req.Close = &AddOrderCloseRequest{OrderType: value} })
			assert.Equal(t, string(value), values.Get("close[ordertype]"), "AddOrder should encode the documented close order type")
		})
	}

	added, err = newSpotNullResultExchange(t).AddOrder(ctx, validRequest)
	require.NoError(t, err, "AddOrder must accept a null result")
	assert.Nil(t, added, "AddOrder should return nil for a null result")
	added, err = newSpotErrorExchange(t).AddOrder(ctx, validRequest)
	require.ErrorIs(t, err, errSpotTransport, "AddOrder must surface request errors")
	assert.Nil(t, added, "AddOrder result should remain nil on request errors")
}

func TestCancelExistingOrder(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotTradingFixtures)
	ctx := t.Context()
	cancelUserReference := int32(-42)

	_, err := ex.CancelExistingOrder(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CancelExistingOrder must reject a nil request")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{})
	require.ErrorIs(t, err, errOrderIdentityRequired, "CancelExistingOrder must require an order identifier")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER", UserReference: &cancelUserReference})
	require.ErrorIs(t, err, errOrderIdentityConflict, "CancelExistingOrder must reject multiple order identifiers")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER", ClientOrderID: "CLIENT"})
	require.ErrorIs(t, err, errOrderIdentityConflict, "CancelExistingOrder must reject transaction and client identifiers together")
	cancelled, err := ex.CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER"})
	require.NoError(t, err, "CancelExistingOrder must accept a transaction identifier")
	require.NotNil(t, cancelled, "CancelExistingOrder must return a response")
	assert.Equal(t, int64(1), cancelled.Count, "CancelExistingOrder should decode cancellation count")
	responseJSON, err := json.Marshal(cancelled)
	require.NoError(t, err, "CancelExistingOrder must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"count":1`, "CancelExistingOrder should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/CancelOrder")
	assert.Equal(t, "ORDER", values.Get("txid"), "CancelExistingOrder should encode transaction identifier")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{UserReference: &cancelUserReference})
	require.NoError(t, err, "CancelExistingOrder must accept a user reference")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrder")
	assert.Equal(t, "-42", values.Get("txid"), "CancelExistingOrder should encode signed user reference")
	_, err = ex.CancelExistingOrder(ctx, &CancelOrderRequest{ClientOrderID: "CLIENT"})
	require.NoError(t, err, "CancelExistingOrder must accept a client order ID")
	values = requireSpotRequest(t, requests, "/0/private/CancelOrder")
	assert.Equal(t, "CLIENT", values.Get("cl_ord_id"), "CancelExistingOrder should encode client order ID")

	cancelled, err = newSpotNullResultExchange(t).CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER"})
	require.NoError(t, err, "CancelExistingOrder must accept a null result")
	assert.Nil(t, cancelled, "CancelExistingOrder should return nil for a null result")
	cancelled, err = newSpotErrorExchange(t).CancelExistingOrder(ctx, &CancelOrderRequest{TransactionID: "ORDER"})
	require.ErrorIs(t, err, errSpotTransport, "CancelExistingOrder must surface request errors")
	assert.Nil(t, cancelled, "CancelExistingOrder result should remain nil on request errors")
}
