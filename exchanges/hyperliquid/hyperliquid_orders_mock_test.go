package hyperliquid

import (
	"fmt"
	"math"
	"net/http"
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
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

func newTradingTestExchange(t *testing.T, infoResponses map[string]string, actionResponse func(string, map[string]any) string) *Exchange {
	t.Helper()
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			var infoPayload infoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&infoPayload), "Decode should not error for trading info request") {
				return
			}
			response, ok := infoResponses[infoPayload.Type]
			if !ok {
				switch infoPayload.Type {
				case infoTypePerpetualDEXs:
					response = `[null]`
				case infoTypeMetadata:
					response = perpetualMetadataJSON
				case "spotMeta":
					response = spotMetadataJSON
				case testUserRoleInfoType:
					response = testUserRoleResponse
				default:
					http.Error(w, "unexpected info request "+infoPayload.Type, http.StatusBadRequest)
					return
				}
			}
			_, err := w.Write([]byte(response))
			assert.NoError(t, err, "Write should not error for trading info response")
		case "/exchange":
			var actionPayload struct {
				Action map[string]any `json:"action"`
			}
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&actionPayload), "Decode should not error for signed trading request") {
				return
			}
			actionType, _ := actionPayload.Action["type"].(string)
			response := ""
			if actionResponse != nil {
				response = actionResponse(actionType, actionPayload.Action)
			}
			if response == "" {
				http.Error(w, "unexpected exchange action "+actionType, http.StatusBadRequest)
				return
			}
			_, err := w.Write([]byte(response))
			assert.NoError(t, err, "Write should not error for signed trading response")
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	ex.setPairMappings(asset.PerpetualContract, []pairMapping{{
		pair:         testPerpetualPair,
		coin:         "BTC",
		assetID:      0,
		sizeDecimals: 5,
		maxLeverage:  40,
	}})
	ex.setPairMappings(asset.Spot, []pairMapping{{
		pair:         testSpotPair,
		coin:         "@107",
		assetID:      10107,
		sizeDecimals: 2,
	}})
	return ex
}

func mustOpenOrder(t *testing.T, raw string) OpenOrder {
	t.Helper()
	var result OpenOrder
	require.NoError(t, json.Unmarshal([]byte(raw), &result), "Unmarshal must not error for test order")
	return result
}

func TestFormatOrderSize(t *testing.T) {
	for _, tc := range []struct {
		name       string
		size       float64
		decimals   uint64
		expected   string
		expectedIs error
	}{
		{name: "integer", size: 2, decimals: 0, expected: "2"},
		{name: "fraction", size: 1.23, decimals: 2, expected: "1.23"},
		{name: "zero", size: 0, decimals: 2, expectedIs: errSizePrecision},
		{name: "negative", size: -1, decimals: 2, expectedIs: errSizePrecision},
		{name: "metadata precision", size: 1, decimals: 9, expectedIs: errSizePrecision},
		{name: "excess precision", size: 1.234, decimals: 2, expectedIs: errSizePrecision},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := formatOrderSize(tc.size, tc.decimals)
			require.ErrorIs(t, err, tc.expectedIs, "formatOrderSize must return the expected error for order size")
			assert.Equal(t, tc.expected, result, "result: formatted order size should match")
		})
	}
}

func TestDeriveFilledOrderState(t *testing.T) {
	for _, tc := range []struct {
		name              string
		requested         float64
		filled            float64
		decimals          uint64
		timeInForce       string
		expectedStatus    order.Status
		expectedRemaining float64
		expectedIs        error
	}{
		{name: "filled", requested: 0.1, filled: 0.1, decimals: 5, timeInForce: wireTimeInForceGTC, expectedStatus: order.Filled},
		{name: "partially filled GTC", requested: 0.1, filled: 0.04, decimals: 5, timeInForce: wireTimeInForceGTC, expectedStatus: order.PartiallyFilled, expectedRemaining: 0.06},
		{name: "partially filled IOC", requested: 0.1, filled: 0.04, decimals: 5, timeInForce: wireTimeInForceIOC, expectedStatus: order.PartiallyFilledCancelled, expectedRemaining: 0.06},
		{name: "partially filled ALO", requested: 0.1, filled: 0.04, decimals: 5, timeInForce: wireTimeInForceALO, expectedIs: errActionStatusMalformed},
		{name: "partially filled trigger", requested: 0.1, filled: 0.04, decimals: 5, expectedIs: errActionStatusMalformed},
		{name: "invalid requested size", requested: 0, filled: 0.04, decimals: 5, timeInForce: wireTimeInForceGTC, expectedIs: errInvalidFilledSize},
		{name: "invalid reported precision", requested: 0.1, filled: 0.0400001, decimals: 5, timeInForce: wireTimeInForceGTC, expectedIs: errInvalidFilledSize},
		{name: "unsupported metadata precision", requested: 0.1, filled: 0.04, decimals: 9, timeInForce: wireTimeInForceGTC, expectedIs: errInvalidFilledSize},
		{name: "over-reported fill", requested: 0.1, filled: 0.11, decimals: 5, timeInForce: wireTimeInForceGTC, expectedIs: errInvalidFilledSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, remaining, err := deriveFilledOrderState(tc.requested, tc.filled, tc.decimals, tc.timeInForce)
			require.ErrorIs(t, err, tc.expectedIs, "deriveFilledOrderState must return the expected error for deriving filled order state")
			assert.Equal(t, tc.expectedStatus, status, "status: derived order status should match")
			assert.InDelta(t, tc.expectedRemaining, remaining, 1e-12, "remaining: derived remaining amount should match")
		})
	}
}

func TestValidateLimitPrice(t *testing.T) {
	for _, tc := range []struct {
		name       string
		price      float64
		asset      asset.Item
		decimals   uint64
		expectedIs error
	}{
		{name: "perpetual", price: 1234.5, asset: asset.PerpetualContract, decimals: 5},
		{name: "spot", price: 0.001234, asset: asset.Spot, decimals: 2},
		{name: "large integer", price: 123456789, asset: asset.PerpetualContract, decimals: 5},
		{name: "zero", asset: asset.Spot, expectedIs: errInvalidMarketPrice},
		{name: "nan", price: math.NaN(), asset: asset.Spot, expectedIs: errInvalidMarketPrice},
		{name: "infinity", price: math.Inf(1), asset: asset.Spot, expectedIs: errInvalidMarketPrice},
		{name: "unsupported asset", price: 1, asset: asset.Options, expectedIs: asset.ErrNotSupported},
		{name: "invalid metadata precision", price: 1, asset: asset.PerpetualContract, decimals: 7, expectedIs: errSizePrecision},
		{name: "too many price decimals", price: 100.55, asset: asset.PerpetualContract, decimals: 5, expectedIs: errPricePrecision},
		{name: "too many significant figures", price: 12.3456, asset: asset.Spot, decimals: 2, expectedIs: errPricePrecision},
		{name: "wire precision", price: 0.000000001, asset: asset.Spot, expectedIs: errWireNumberRounding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLimitPrice(tc.price, tc.asset, tc.decimals)
			require.ErrorIs(t, err, tc.expectedIs, "validateLimitPrice must return the expected error for a limit price")
		})
	}
}

func TestRoundMarketPrice(t *testing.T) {
	for _, tc := range []struct {
		name       string
		price      float64
		asset      asset.Item
		decimals   uint64
		expected   float64
		expectedIs error
	}{
		{name: "spot", price: 123.456789, asset: asset.Spot, decimals: 2, expected: 123.46},
		{name: "perpetual", price: 123.456789, asset: asset.PerpetualContract, decimals: 5, expected: 123.5},
		{name: "zero", price: 0, asset: asset.Spot, expectedIs: errInvalidMarketPrice},
		{name: "nan", price: math.NaN(), asset: asset.Spot, expectedIs: errInvalidMarketPrice},
		{name: "infinity", price: math.Inf(1), asset: asset.Spot, expectedIs: errInvalidMarketPrice},
		{name: "unsupported asset", price: 1, asset: asset.Options, expectedIs: asset.ErrNotSupported},
		{name: "excess perpetual size precision", price: 1, asset: asset.PerpetualContract, decimals: 7, expectedIs: errSizePrecision},
		{name: "rounds to zero", price: 0.4, asset: asset.PerpetualContract, decimals: 6, expectedIs: errInvalidMarketPrice},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := roundMarketPrice(tc.price, tc.asset, tc.decimals)
			require.ErrorIs(t, err, tc.expectedIs, "roundMarketPrice must return the expected error for rounding a market price")
			assert.InDelta(t, tc.expected, result, 1e-12, "result: rounded market price should match")
		})
	}
}

func TestFormatOrderTimeInForce(t *testing.T) {
	for _, tc := range []struct {
		timeInForce order.TimeInForce
		expected    string
		expectedIs  error
	}{
		{timeInForce: order.UnknownTIF, expected: "Gtc"},
		{timeInForce: order.GoodTillCancel, expected: "Gtc"},
		{timeInForce: order.PostOnly, expected: "Alo"},
		{timeInForce: order.GoodTillCancel | order.PostOnly, expected: "Alo"},
		{timeInForce: order.ImmediateOrCancel, expected: "Ioc"},
		{timeInForce: order.FillOrKill, expectedIs: errUnsupportedTimeInForce},
	} {
		result, err := formatOrderTimeInForce(tc.timeInForce)
		require.ErrorIs(t, err, tc.expectedIs, "formatOrderTimeInForce must return the expected error for time in force")
		assert.Equal(t, tc.expected, result, "result: formatted time in force should match")
	}
}

func TestBuildOrderWire(t *testing.T) {
	ex := newTradingTestExchange(t, map[string]string{"allMids": `{"BTC":"100","@107":"10"}`}, nil)

	wire, mapping, err := ex.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.Limit, order.Buy, order.GoodTillCancel, 0.12345, 100.5, 0, 0, true, strings.ToUpper(validClientOrderID))
	require.NoError(t, err, "buildOrderWire must not error for a valid limit order")
	assert.Equal(t, uint64(5), mapping.sizeDecimals, "mapping.sizeDecimals: built order should retain its market precision")
	assert.Equal(t, uint64(0), wire.AssetID, "wire.AssetID: perpetual order should use its universe index")
	assert.True(t, wire.IsBuy, "wire.IsBuy: buy order should set the buy flag")
	assert.Equal(t, "100.5", wire.Price, "wire.Price: limit price should be formatted")
	assert.Equal(t, "0.12345", wire.Size, "wire.Size: limit size should be formatted")
	assert.True(t, wire.ReduceOnly, "wire.ReduceOnly: reduce-only flag should be retained")
	assert.Equal(t, "Gtc", wire.Type.Limit.TimeInForce, "wire.Type.Limit.TimeInForce: limit time in force should be formatted")
	assert.Equal(t, validClientOrderID, wire.ClientOrderID, "wire.ClientOrderID should be normalised")

	wire, _, err = ex.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 0.1, 0, 0, 0.01, false, "")
	require.NoError(t, err, "buildOrderWire must not error for a slippage-bounded market buy")
	assert.Equal(t, "101", wire.Price, "wire.Price: market buy should apply positive slippage to the midpoint")
	assert.Equal(t, "Ioc", wire.Type.Limit.TimeInForce, "wire.Type.Limit.TimeInForce: market order should use IOC")

	wire, _, err = ex.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.Market, order.Sell, order.UnknownTIF, 0.1, 0, 0, 0.01, false, "")
	require.NoError(t, err, "buildOrderWire must not error for a slippage-bounded market sell")
	assert.Equal(t, "99", wire.Price, "wire.Price: market sell should apply negative slippage to the midpoint")
	assert.False(t, wire.IsBuy, "wire.IsBuy: sell order should clear the buy flag")

	wire, _, err = ex.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.StopMarket, order.Sell, order.UnknownTIF, 0.1, 0, 90, 0.1, true, "")
	require.NoError(t, err, "buildOrderWire must not error for a stop-market order")
	require.NotNil(t, wire.Type.Trigger, "wire.Type.Trigger: stop-market order must use the trigger wire variant")
	assert.True(t, wire.Type.Trigger.IsMarket, "wire.Type.Trigger.IsMarket: stop-market order should set the market flag")
	assert.Equal(t, "90", wire.Type.Trigger.TriggerPrice, "wire.Type.Trigger.TriggerPrice: stop-market trigger price should be formatted")
	assert.Equal(t, "sl", wire.Type.Trigger.TakeProfitStopLoss, "wire.Type.Trigger.TakeProfitStopLoss: stop-market order should use stop-loss semantics")
	assert.Equal(t, "81", wire.Price, "wire.Price: stop-market sell should derive a slippage-bounded execution price")

	wire, _, err = ex.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.TakeProfit, order.Buy, order.UnknownTIF, 0.1, 111, 110, 0, true, "")
	require.NoError(t, err, "buildOrderWire must not error for a take-profit limit order")
	require.NotNil(t, wire.Type.Trigger, "wire.Type.Trigger: take-profit order must use the trigger wire variant")
	assert.False(t, wire.Type.Trigger.IsMarket, "wire.Type.Trigger.IsMarket: take-profit limit order should clear the market flag")
	assert.Equal(t, "tp", wire.Type.Trigger.TakeProfitStopLoss, "wire.Type.Trigger.TakeProfitStopLoss: take-profit order should use take-profit semantics")
	assert.Equal(t, "111", wire.Price, "wire.Price: take-profit limit price should be formatted")

	for _, tc := range []struct {
		name         string
		pair         currency.Pair
		asset        asset.Item
		orderType    order.Type
		timeInForce  order.TimeInForce
		amount       float64
		price        float64
		triggerPrice float64
		slippage     float64
		reduceOnly   bool
		clientID     string
		expectedIs   error
	}{
		{name: "missing mapping", pair: currency.NewPair(currency.ETH, currency.USDC), asset: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: 1, expectedIs: errPairMappingNotFound},
		{name: "invalid size", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Limit, amount: 0.000001, price: 1, expectedIs: errSizePrecision},
		{name: "invalid client ID", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: 1, clientID: "invalid", expectedIs: errClientOrderIDInvalid},
		{name: "invalid limit precision", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: 100.55, expectedIs: errPricePrecision},
		{name: "invalid time in force", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Limit, timeInForce: order.FillOrKill, amount: 1, price: 1, expectedIs: errUnsupportedTimeInForce},
		{name: "zero slippage", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Market, amount: 1, slippage: 0, expectedIs: errSlippageTolerance},
		{name: "excess slippage", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Market, amount: 1, slippage: 1, expectedIs: errSlippageTolerance},
		{name: "unsupported type", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.TrailingStop, amount: 1, price: 1, expectedIs: order.ErrTypeIsInvalid},
		{name: "invalid price", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: math.NaN(), expectedIs: errInvalidMarketPrice},
		{name: "limit with trigger price", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Limit, amount: 1, price: 1, triggerPrice: 2, expectedIs: errRiskManagementUnsupported},
		{name: "market with trigger price", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.Market, amount: 1, triggerPrice: 2, slippage: 0.1, expectedIs: errRiskManagementUnsupported},
		{name: "spot trigger", pair: testSpotPair, asset: asset.Spot, orderType: order.StopMarket, amount: 1, triggerPrice: 10, slippage: 0.1, reduceOnly: true, expectedIs: errTriggerOrderReduceOnly},
		{name: "non-reduce-only trigger", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, triggerPrice: 10, slippage: 0.1, expectedIs: errTriggerOrderReduceOnly},
		{name: "missing trigger price", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, slippage: 0.1, reduceOnly: true, expectedIs: errTriggerPriceRequired},
		{name: "invalid trigger precision", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, triggerPrice: 100.55, slippage: 0.1, reduceOnly: true, expectedIs: errPricePrecision},
		{name: "trigger market missing slippage", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, triggerPrice: 100, reduceOnly: true, expectedIs: errSlippageTolerance},
		{name: "trigger market derived price overflow", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.StopMarket, amount: 1, triggerPrice: math.MaxFloat64, slippage: 0.9, reduceOnly: true, expectedIs: errInvalidMarketPrice},
		{name: "trigger limit missing price", pair: testPerpetualPair, asset: asset.PerpetualContract, orderType: order.StopLimit, amount: 1, triggerPrice: 100, reduceOnly: true, expectedIs: errInvalidMarketPrice},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ex.buildOrderWire(t.Context(), tc.pair, tc.asset, tc.orderType, order.Buy, tc.timeInForce, tc.amount, tc.price, tc.triggerPrice, tc.slippage, tc.reduceOnly, tc.clientID)
			require.ErrorIs(t, err, tc.expectedIs, "buildOrderWire must return the expected error for invalid order")
		})
	}

	missingMid := newTradingTestExchange(t, map[string]string{"allMids": `{}`}, nil)
	_, _, err = missingMid.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 1, 0, 0, 0.01, false, "")
	require.ErrorIs(t, err, errMarketMidPriceNotFound, "buildOrderWire must return the expected error for missing market midpoint")

	zeroMid := newTradingTestExchange(t, map[string]string{"allMids": `{"BTC":"0"}`}, nil)
	_, _, err = zeroMid.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 1, 0, 0, 0.01, false, "")
	require.ErrorIs(t, err, errMarketMidPriceNotFound, "buildOrderWire must return the expected error for zero market midpoint")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	errorExchange.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: testPerpetualPair, coin: "BTC", sizeDecimals: 5}})
	_, _, err = errorExchange.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 1, 0, 0, 0.01, false, "")
	require.Error(t, err, "buildOrderWire must return market midpoint HTTP failure")

	invalidPricePrecision := newTradingTestExchange(t, map[string]string{"allMids": `{"BTC":"100"}`}, nil)
	invalidPricePrecision.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: testPerpetualPair, coin: "BTC", sizeDecimals: 7}})
	_, _, err = invalidPricePrecision.buildOrderWire(t.Context(), testPerpetualPair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 1, 0, 0, 0.01, false, "")
	require.ErrorIs(t, err, errSizePrecision, "buildOrderWire must return the expected error for market price precision incompatible with metadata")

	hip3Pair := currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.USDC)
	var hip3Request infoRequest
	hip3 := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&hip3Request), "Decode should not error for HIP-3 midpoint request") {
			return
		}
		_, err := w.Write([]byte(`{"xyz:XYZ100":"50"}`))
		assert.NoError(t, err, "Write should not error for HIP-3 midpoint response")
	}))
	hip3.setPairMappings(asset.PerpetualContract, []pairMapping{{
		pair: hip3Pair, coin: "xyz:XYZ100", dex: testBuilderDEXName, assetID: 110000, sizeDecimals: 2,
	}})
	wire, _, err = hip3.buildOrderWire(t.Context(), hip3Pair, asset.PerpetualContract, order.Market, order.Buy, order.UnknownTIF, 1, 0, 0, 0.01, false, "")
	require.NoError(t, err, "buildOrderWire must not error for a HIP-3 market order")
	assert.Equal(t, testBuilderDEXName, hip3Request.DEX, "hip3Request.DEX: HIP-3 market midpoint request should use its DEX")
	assert.Equal(t, uint64(110000), wire.AssetID, "wire.AssetID: HIP-3 order should use its builder asset ID")
}

func TestBuildOrderWires(t *testing.T) {
	ex := newTradingTestExchange(t, nil, nil)
	submit := &order.Submit{
		Exchange:      "Hyperliquid",
		Type:          order.Limit,
		Side:          order.Buy,
		Pair:          testPerpetualPair,
		AssetType:     asset.PerpetualContract,
		TimeInForce:   order.GoodTillCancel,
		Amount:        0.1,
		Price:         100,
		ClientOrderID: validClientOrderID,
	}

	wires, mapping, grouping, err := ex.buildOrderWires(t.Context(), submit)
	require.NoError(t, err, "buildOrderWires must not error for one ungrouped order")
	require.Len(t, wires, 1, "wires: ungrouped submission must contain one wire")
	assert.Equal(t, uint64(0), mapping.assetID, "mapping.assetID: ungrouped submission should return the parent mapping")
	assert.Equal(t, orderGroupingNone, grouping, "grouping: ungrouped submission should use no grouping")

	standaloneTrigger := *submit
	standaloneTrigger.Type = order.StopMarket
	standaloneTrigger.Side = order.Sell
	standaloneTrigger.TimeInForce = order.UnknownTIF
	standaloneTrigger.Price = 0
	standaloneTrigger.TriggerPrice = 90
	standaloneTrigger.TriggerPriceType = order.LastPrice
	standaloneTrigger.SlippageTolerance = 0.1
	standaloneTrigger.ReduceOnly = true
	wires, mapping, _, err = ex.buildOrderWires(t.Context(), &standaloneTrigger)
	require.ErrorIs(t, err, errRiskManagementUnsupported, "buildOrderWires must fail closed for standalone trigger using last price")
	assert.Empty(t, wires, "wires: rejected standalone trigger should not build a wire")
	assert.Equal(t, pairMapping{}, mapping, "mapping: rejected standalone trigger should not return a mapping")
	standaloneTrigger.TriggerPriceType = order.MarkPrice
	wires, _, grouping, err = ex.buildOrderWires(t.Context(), &standaloneTrigger)
	require.NoError(t, err, "buildOrderWires must not error for standalone mark-price trigger")
	require.Len(t, wires, 1, "wires: standalone trigger must contain one wire")
	assert.Equal(t, orderGroupingNone, grouping, "grouping: standalone trigger should not use grouped semantics")

	grouped := *submit
	grouped.RiskManagementModes = order.RiskManagementModes{
		Mode: orderGroupingNormalTPSL,
		TakeProfit: order.RiskManagement{
			Enabled:          true,
			TriggerPriceType: order.MarkPrice,
			Price:            110,
			OrderType:        order.Market,
		},
		StopLoss: order.RiskManagement{
			Enabled:          true,
			TriggerPriceType: order.MarkPrice,
			Price:            90,
			LimitPrice:       89,
			OrderType:        order.StopLimit,
		},
	}
	wires, _, grouping, err = ex.buildOrderWires(t.Context(), &grouped)
	require.NoError(t, err, "buildOrderWires must not error for grouped parent and TP/SL children")
	require.Len(t, wires, 3, "wires: grouped submission must contain the parent and both children")
	assert.Equal(t, orderGroupingNormalTPSL, grouping, "grouping: grouped submission should use normal TP/SL grouping")
	assert.Equal(t, validClientOrderID, wires[0].ClientOrderID, "wires[0].ClientOrderID: grouped parent should retain the client order ID")
	assert.Empty(t, wires[1].ClientOrderID, "wires[1].ClientOrderID: grouped take-profit child should not reuse the parent client order ID")
	assert.False(t, wires[1].IsBuy, "wires[1].IsBuy: long parent take-profit child should sell")
	assert.Equal(t, "99", wires[1].Price, "wires[1].Price: market take-profit child should use Hyperliquid's default ten-percent bound")
	assert.Equal(t, "tp", wires[1].Type.Trigger.TakeProfitStopLoss, "wires[1].Type.Trigger.TakeProfitStopLoss: take-profit child should use TP semantics")
	assert.False(t, wires[2].Type.Trigger.IsMarket, "wires[2].Type.Trigger.IsMarket: stop-limit child should use a limit trigger")
	assert.Equal(t, "89", wires[2].Price, "wires[2].Price: stop-limit child should retain its execution price")

	shortParent := grouped
	shortParent.Side = order.Sell
	shortParent.RiskManagementModes.TakeProfit.OrderType = order.TakeProfit
	shortParent.RiskManagementModes.TakeProfit.LimitPrice = 91
	shortParent.RiskManagementModes.StopLoss.OrderType = order.Stop
	shortParent.RiskManagementModes.StopLoss.LimitPrice = 0
	wires, _, _, err = ex.buildOrderWires(t.Context(), &shortParent)
	require.NoError(t, err, "buildOrderWires must not error for explicitly typed short-parent children")
	assert.True(t, wires[1].IsBuy, "wires[1].IsBuy: short parent take-profit child should buy")
	assert.False(t, wires[1].Type.Trigger.IsMarket, "wires[1].Type.Trigger.IsMarket: take-profit type should build a limit trigger")
	assert.True(t, wires[2].Type.Trigger.IsMarket, "wires[2].Type.Trigger.IsMarket: stop type should build a market trigger")
	assert.Equal(t, "99", wires[2].Price, "wires[2].Price: short-parent stop child should derive an upward buy bound")

	for _, tc := range []struct {
		name       string
		mutate     func(*order.Submit)
		expectedIs error
	}{
		{
			name: "invalid parent",
			mutate: func(s *order.Submit) {
				s.Amount = 0.000001
			},
			expectedIs: errSizePrecision,
		},
		{
			name: "spot grouping",
			mutate: func(s *order.Submit) {
				s.Pair = testSpotPair
				s.AssetType = asset.Spot
				s.RiskManagementModes.TakeProfit.Enabled = true
				s.RiskManagementModes.TakeProfit.Price = 11
			},
			expectedIs: errRiskManagementUnsupported,
		},
		{
			name: "stop entry",
			mutate: func(s *order.Submit) {
				s.RiskManagementModes.StopEntry.Enabled = true
			},
			expectedIs: errRiskManagementUnsupported,
		},
		{
			name: "unsupported mode",
			mutate: func(s *order.Submit) {
				s.RiskManagementModes.Mode = "position"
				s.RiskManagementModes.TakeProfit.Enabled = true
			},
			expectedIs: errRiskManagementUnsupported,
		},
		{
			name: "missing child trigger",
			mutate: func(s *order.Submit) {
				s.RiskManagementModes.TakeProfit.Enabled = true
			},
			expectedIs: errTriggerPriceRequired,
		},
		{
			name: "unsupported trigger source",
			mutate: func(s *order.Submit) {
				s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, Price: 110, TriggerPriceType: order.IndexPrice}
			},
			expectedIs: errRiskManagementUnsupported,
		},
		{
			name: "unsupported take-profit type",
			mutate: func(s *order.Submit) {
				s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110, OrderType: order.Stop}
			},
			expectedIs: errRiskManagementUnsupported,
		},
		{
			name: "unsupported stop-loss type",
			mutate: func(s *order.Submit) {
				s.RiskManagementModes.StopLoss = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 90, OrderType: order.TakeProfit}
			},
			expectedIs: errRiskManagementUnsupported,
		},
		{
			name: "invalid child execution price",
			mutate: func(s *order.Submit) {
				s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110, LimitPrice: 100.55, OrderType: order.Limit}
			},
			expectedIs: errPricePrecision,
		},
		{
			name: "invalid child slippage",
			mutate: func(s *order.Submit) {
				s.SlippageTolerance = 1
				s.RiskManagementModes.TakeProfit = order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110}
			},
			expectedIs: errSlippageTolerance,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := *submit
			tc.mutate(&invalid)
			_, _, _, err := ex.buildOrderWires(t.Context(), &invalid)
			require.ErrorIs(t, err, tc.expectedIs, "buildOrderWires must return the expected error for invalid grouped order")
		})
	}
}

func actionResponse(raw string) *exchangeActionResponse {
	return &exchangeActionResponse{Status: "ok", Response: json.RawMessage(raw)}
}

func TestParseOrderActionStatuses(t *testing.T) {
	_, err := parseOrderActionStatuses(nil, 1)
	require.ErrorIs(t, err, common.ErrNilPointer, "parseOrderActionStatuses must return the expected error for nil order action response")

	_, err = parseOrderActionStatuses(actionResponse(`invalid`), 1)
	require.Error(t, err, "parseOrderActionStatuses must error for invalid order action response")

	_, err = parseOrderActionStatuses(actionResponse(`{"data":{"statuses":[]}}`), 1)
	require.ErrorIs(t, err, errActionStatusCount, "parseOrderActionStatuses must return the expected error for unexpected order status count")

	statuses, err := parseOrderActionStatuses(actionResponse(`{"data":{"statuses":[{"resting":{"oid":7}},{"filled":{"oid":8,"totalSz":"1","avgPx":"2"}},{"error":"bad"},"waitingForFill","waitingForTrigger"]}}`), 5)
	require.NoError(t, err, "parseOrderActionStatuses must not error for valid order status variants")
	require.Len(t, statuses, 5, "statuses: all order status variants must be returned")
	assert.Equal(t, uint64(7), statuses[0].Resting.OrderID, "statuses[0].Resting.OrderID: resting order ID should be decoded")
	assert.Equal(t, uint64(8), statuses[1].Filled.OrderID, "statuses[1].Filled.OrderID: filled order ID should be decoded")
	assert.Equal(t, "bad", statuses[2].Error, "statuses[2].Error: order error should be decoded")
	assert.Equal(t, orderStatusWaitingForFill, statuses[3].Deferred, "statuses[3].Deferred: waiting-for-fill status should be decoded")
	assert.Equal(t, orderStatusWaitingForTrigger, statuses[4].Deferred, "statuses[4].Deferred: waiting-for-trigger status should be decoded")

	statuses, err = parseOrderActionStatuses(actionResponse(`{"data":{"statuses":[{"error":"batch rejected"}]}}`), 3)
	require.NoError(t, err, "parseOrderActionStatuses must not error for one deterministic batch error")
	require.Len(t, statuses, 3, "statuses: one deterministic batch error must be expanded to every requested order")
	assert.Equal(t, "batch rejected", statuses[2].Error, "statuses[2].Error: expanded batch error should retain the exchange message")

	for _, raw := range []string{
		`{"data":{"statuses":[invalid]}}`,
		`{"data":{"statuses":[1]}}`,
		`{"data":{"statuses":["invalid"]}}`,
		`{"data":{"statuses":[""]}}`,
		`{"data":{"statuses":[{}]}}`,
		`{"data":{"statuses":[{"resting":{"oid":7},"error":"bad"}]}}`,
		`{"data":{"statuses":[{"resting":{"oid":0}}]}}`,
		`{"data":{"statuses":[{"filled":{"oid":0}}]}}`,
	} {
		_, err = parseOrderActionStatuses(actionResponse(raw), 1)
		require.Error(t, err, "parseOrderActionStatuses must error for malformed order status")
	}
}

func TestParseCancelActionStatuses(t *testing.T) {
	_, err := parseCancelActionStatuses(nil, []string{"1"})
	require.ErrorIs(t, err, common.ErrNilPointer, "parseCancelActionStatuses must return the expected error for nil cancel action response")
	_, err = parseCancelActionStatuses(actionResponse(`invalid`), []string{"1"})
	require.Error(t, err, "parseCancelActionStatuses must error for invalid cancel action response")
	_, err = parseCancelActionStatuses(actionResponse(`{"data":{"statuses":[]}}`), []string{"1"})
	require.ErrorIs(t, err, errActionStatusCount, "parseCancelActionStatuses must return the expected error for unexpected cancel status count")

	statuses, err := parseCancelActionStatuses(actionResponse(`{"data":{"statuses":["success",{"error":"already closed"}]}}`), []string{"1", validClientOrderID})
	require.Error(t, err, "parseCancelActionStatuses must return per-order cancellation failure")
	require.ErrorIs(t, err, errActionResponse, "parseCancelActionStatuses must retain the action-response classification for per-order cancellation failure")
	assert.Equal(t, "success", statuses["1"], "statuses[\"1\"]: successful cancellation should be retained")
	assert.Equal(t, "already closed", statuses[validClientOrderID], "statuses[validClientOrderID]: failed cancellation message should be retained")

	_, err = parseCancelActionStatuses(actionResponse(`{"data":{"statuses":[invalid]}}`), []string{"1"})
	require.Error(t, err, "parseCancelActionStatuses must error for invalid cancellation status")
	_, err = parseCancelActionStatuses(actionResponse(`{"data":{"statuses":[1]}}`), []string{"1"})
	require.Error(t, err, "parseCancelActionStatuses must error for cancellation status with an invalid variant type")
	_, err = parseCancelActionStatuses(actionResponse(`{"data":{"statuses":[{}]}}`), []string{"1"})
	require.ErrorIs(t, err, errActionStatusMalformed, "parseCancelActionStatuses must return the expected error for empty cancellation status")
}

func TestClassifyHyperliquidOrderStatus(t *testing.T) {
	for _, tc := range []struct {
		status     string
		expected   order.Status
		expectedIs error
	}{
		{status: "open", expected: order.Open},
		{status: "filled", expected: order.Filled},
		{status: "triggered", expected: order.Closed},
		{status: "canceled", expected: order.Cancelled},
		{status: "scheduledCancel", expected: order.Cancelled},
		{status: "marginCanceled", expected: order.Cancelled},
		{status: "liquidatedCanceled", expected: order.Cancelled},
		{status: "rejected", expected: order.Rejected},
		{status: "perpMaxPositionRejected", expected: order.Rejected},
		{status: "unknown", expected: order.UnknownStatus, expectedIs: errUnsupportedOrderStatus},
	} {
		result, err := classifyHyperliquidOrderStatus(tc.status)
		require.ErrorIs(t, err, tc.expectedIs, "classifyHyperliquidOrderStatus must return the expected error for classifying order status")
		assert.Equal(t, tc.expected, result, "result: classified order status should match")
	}
}

func TestClassifyHyperliquidOrderType(t *testing.T) {
	for _, tc := range []struct {
		orderType  string
		isTrigger  bool
		expected   order.Type
		expectedIs error
	}{
		{orderType: "Limit", expected: order.Limit},
		{orderType: "Market", expected: order.Market},
		{orderType: "Stop Limit", isTrigger: true, expected: order.StopLimit},
		{orderType: "Stop Market", isTrigger: true, expected: order.StopMarket},
		{orderType: "Stop", isTrigger: true, expected: order.Stop},
		{orderType: "Take Profit Limit", isTrigger: true, expected: order.TakeProfit},
		{orderType: "Take Profit Market", isTrigger: true, expected: order.TakeProfitMarket},
		{orderType: "unknown", expected: order.UnknownType, expectedIs: order.ErrTypeIsInvalid},
	} {
		result, err := classifyHyperliquidOrderType(tc.orderType, tc.isTrigger)
		require.ErrorIs(t, err, tc.expectedIs, "classifyHyperliquidOrderType must return the expected error for classifying order type")
		assert.Equal(t, tc.expected, result, "result: classified order type should match")
	}
}

func TestClassifyHyperliquidTimeInForce(t *testing.T) {
	for _, tc := range []struct {
		timeInForce string
		expected    order.TimeInForce
		expectedIs  error
	}{
		{timeInForce: "", expected: order.GoodTillCancel},
		{timeInForce: "GTC", expected: order.GoodTillCancel},
		{timeInForce: "Alo", expected: order.PostOnly},
		{timeInForce: "Ioc", expected: order.ImmediateOrCancel},
		{timeInForce: "FrontendMarket", expected: order.ImmediateOrCancel},
		{timeInForce: "bad", expected: order.UnknownTIF, expectedIs: order.ErrInvalidTimeInForce},
	} {
		result, err := classifyHyperliquidTimeInForce(tc.timeInForce)
		require.ErrorIs(t, err, tc.expectedIs, "classifyHyperliquidTimeInForce must return the expected error for classifying time in force")
		assert.Equal(t, tc.expected, result, "result: classified time in force should match")
	}
}

func TestConvertOrder(t *testing.T) {
	ex := newTradingTestExchange(t, nil, nil)
	_, err := ex.convertOrder(t.Context(), nil, "open", time.Time{})
	require.ErrorIs(t, err, common.ErrNilPointer, "convertOrder must return the expected error for nil source order")
	source := mustOpenOrder(t, `{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1700000000000,"triggerPx":"0","isTrigger":false,"reduceOnly":true,"orderType":"Limit","tif":"Gtc","cloid":"`+validClientOrderID+`"}`)
	statusTime := time.UnixMilli(1700000001000)
	detail, err := ex.convertOrder(t.Context(), &source, "open", statusTime)
	require.NoError(t, err, "convertOrder must not error for a valid order")
	assert.Equal(t, order.Buy, detail.Side, "detail.Side: order side should be converted")
	assert.Equal(t, order.Limit, detail.Type, "detail.Type: order type should be converted")
	assert.Equal(t, order.Open, detail.Status, "detail.Status: order status should be converted")
	assert.Equal(t, 2.0, detail.Amount, "detail.Amount: original order size should be used")
	assert.Equal(t, 1.0, detail.ExecutedAmount, "detail.ExecutedAmount: executed size should be derived")
	assert.Equal(t, validClientOrderID, detail.ClientOrderID, "detail.ClientOrderID should be retained")
	assert.Equal(t, statusTime.UTC(), detail.LastUpdated, "detail.LastUpdated: status timestamp should be used")
	assert.True(t, detail.ReduceOnly, "detail.ReduceOnly: reduce-only flag should be retained")

	source.Side = "A"
	source.OriginalSize = 0
	source.ClientOrderID = nil
	detail, err = ex.convertOrder(t.Context(), &source, "filled", time.Time{})
	require.NoError(t, err, "convertOrder must not error for an order with fallback fields")
	assert.Equal(t, order.Sell, detail.Side, "detail.Side: sell side should be converted")
	assert.Equal(t, 1.0, detail.Amount, "detail.Amount: remaining size should be used when original size is absent")
	assert.Empty(t, detail.ClientOrderID, "detail.ClientOrderID: missing client order ID should remain empty")
	assert.Equal(t, source.Timestamp.Time().UTC(), detail.LastUpdated, "detail.LastUpdated: order timestamp should be the last-updated fallback")

	for _, tc := range []struct {
		name       string
		mutate     func(*OpenOrder)
		status     string
		expectedIs error
	}{
		{name: "missing mapping", mutate: func(o *OpenOrder) { o.Coin = "MISSING" }, status: "open", expectedIs: errPairMappingNotFound},
		{name: "invalid side", mutate: func(o *OpenOrder) { o.Side = "X" }, status: "open", expectedIs: order.ErrSideIsInvalid},
		{name: "invalid type", mutate: func(o *OpenOrder) { o.OrderType = "unknown" }, status: "open", expectedIs: order.ErrTypeIsInvalid},
		{name: "invalid time in force", mutate: func(o *OpenOrder) { o.TimeInForce = "bad" }, status: "open", expectedIs: order.ErrInvalidTimeInForce},
		{name: "invalid status", mutate: func(*OpenOrder) {}, status: "unknown", expectedIs: errUnsupportedOrderStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := source
			invalid.Side = "B"
			invalid.OrderType = "Limit"
			invalid.TimeInForce = "Gtc"
			invalid.Coin = "BTC"
			tc.mutate(&invalid)
			_, err := ex.convertOrder(t.Context(), &invalid, tc.status, time.Time{})
			require.ErrorIs(t, err, tc.expectedIs, "convertOrder must return the expected error for invalid order")
		})
	}
}

func TestConvertOrderFromMapping(t *testing.T) {
	ex := newTradingTestExchange(t, nil, nil)
	mapping, a, err := ex.lookupPairMappingByCoin("BTC")
	require.NoError(t, err, "lookupPairMappingByCoin must not error for the mapped-order fixture")
	_, err = ex.convertOrderFromMapping(nil, "open", time.Time{}, &mapping, a)
	require.ErrorIs(t, err, common.ErrNilPointer, "convertOrderFromMapping must return the expected error for nil mapped source order")
	_, err = ex.convertOrderFromMapping(&OpenOrder{}, "open", time.Time{}, nil, a)
	require.ErrorIs(t, err, common.ErrNilPointer, "convertOrderFromMapping must return the expected error for nil pair mapping")

	source := mustOpenOrder(t, `{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1700000000000,"triggerPx":"0","isTrigger":false,"reduceOnly":true,"orderType":"Limit","tif":"Gtc","cloid":"`+validClientOrderID+`"}`)
	statusTime := time.UnixMilli(1700000001000)
	detail, err := ex.convertOrderFromMapping(&source, "open", statusTime, &mapping, a)
	require.NoError(t, err, "convertOrderFromMapping must not error for a valid mapped order")
	assert.Equal(t, order.Buy, detail.Side, "detail.Side: mapped order side should be converted")
	assert.Equal(t, testPerpetualPair, detail.Pair, "detail.Pair: mapped order pair should be retained")
	assert.Equal(t, statusTime.UTC(), detail.LastUpdated, "detail.LastUpdated: mapped order status timestamp should be used")

	source.Side = "A"
	source.OriginalSize = 0
	source.ClientOrderID = nil
	detail, err = ex.convertOrderFromMapping(&source, "filled", time.Time{}, &mapping, a)
	require.NoError(t, err, "convertOrderFromMapping must not error for a mapped order with fallback fields")
	assert.Equal(t, order.Sell, detail.Side, "detail.Side: mapped sell side should be converted")
	assert.Equal(t, 1.0, detail.Amount, "detail.Amount: mapped order should use remaining size when original size is absent")
	assert.Empty(t, detail.ClientOrderID, "detail.ClientOrderID: mapped order without a client ID should remain empty")
	assert.Equal(t, source.Timestamp.Time().UTC(), detail.LastUpdated, "detail.LastUpdated: mapped order timestamp should be the last-updated fallback")

	for _, tc := range []struct {
		name       string
		mutate     func(*OpenOrder)
		status     string
		expectedIs error
	}{
		{name: "invalid side", mutate: func(o *OpenOrder) { o.Side = "X" }, status: "open", expectedIs: order.ErrSideIsInvalid},
		{name: "invalid type", mutate: func(o *OpenOrder) { o.OrderType = "unknown" }, status: "open", expectedIs: order.ErrTypeIsInvalid},
		{name: "invalid time in force", mutate: func(o *OpenOrder) { o.TimeInForce = "bad" }, status: "open", expectedIs: order.ErrInvalidTimeInForce},
		{name: "invalid status", mutate: func(*OpenOrder) {}, status: "unknown", expectedIs: errUnsupportedOrderStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := source
			invalid.Side = "B"
			invalid.OrderType = "Limit"
			invalid.TimeInForce = "Gtc"
			tc.mutate(&invalid)
			_, err := ex.convertOrderFromMapping(&invalid, tc.status, time.Time{}, &mapping, a)
			require.ErrorIs(t, err, tc.expectedIs, "convertOrderFromMapping must return the expected error for an invalid mapped order")
		})
	}
}

func TestExchangeActionEndpointLimit(t *testing.T) {
	for _, tc := range []struct {
		batchLength int
		offset      request.EndpointLimit
	}{
		{batchLength: -1, offset: 0},
		{batchLength: 0, offset: 0},
		{batchLength: 1, offset: 0},
		{batchLength: 39, offset: 0},
		{batchLength: 40, offset: 1},
		{batchLength: 79, offset: 1},
		{batchLength: 80, offset: 2},
		{batchLength: maximumActionBatchSize + 1, offset: maximumActionBatchSize / actionBatchWeightSize},
	} {
		assert.Equal(t, exchangeActionEPLBase+tc.offset, exchangeActionEndpointLimit(tc.batchLength), "exchangeActionEndpointLimit: action endpoint limit should match its batch weight")
	}
}

func TestSubmitOrder(t *testing.T) {
	_, err := new(Exchange).SubmitOrder(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrSubmissionIsNil, "SubmitOrder must return the expected error for nil order submission")

	submit := &order.Submit{
		Exchange:      "Hyperliquid",
		Type:          order.Limit,
		Side:          order.Buy,
		Pair:          testPerpetualPair,
		AssetType:     asset.PerpetualContract,
		TimeInForce:   order.GoodTillCancel,
		Amount:        0.1,
		Price:         100,
		ClientOrderID: validClientOrderID,
	}
	missingSigner := newTradingTestExchange(t, nil, nil)
	setTestCredentials(missingSigner, &accounts.Credentials{Key: officialSigningAddress})
	_, err = missingSigner.SubmitOrder(t.Context(), submit)
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "SubmitOrder must fail before constructing an action for missing signing key")

	invalidBuild := newTradingTestExchange(t, nil, nil)
	invalid := *submit
	invalid.Amount = 0.000001
	_, err = invalidBuild.SubmitOrder(t.Context(), &invalid)
	require.ErrorIs(t, err, errSizePrecision, "SubmitOrder must return order wire construction failure")
	trigger := *submit
	trigger.TriggerPrice = 90
	_, err = invalidBuild.SubmitOrder(t.Context(), &trigger)
	require.ErrorIs(t, err, errRiskManagementUnsupported, "SubmitOrder must fail closed for trigger price on a non-trigger order")

	resting := newTradingTestExchange(t, nil, func(actionType string, _ map[string]any) string {
		assert.Equal(t, "order", actionType, "actionType: submit should send an order action")
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":7}}]}}}`
	})
	result, err := resting.submitOrder(t.Context(), submit)
	require.NoError(t, err, "submitOrder must not error for a resting limit order")
	assert.Equal(t, "7", result.OrderID, "result.OrderID: submitted order ID should be returned")
	assert.Equal(t, order.New, result.Status, "result.Status: resting order should be new")
	assert.Equal(t, 100.0, result.Price, "result.Price: submitted wire price should be returned")
	assert.Equal(t, submit.Amount, result.RemainingAmount, "result.RemainingAmount: a resting order should retain its full open quantity")

	partiallyFilledGTC := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":7,"totalSz":"0.04","avgPx":"100"}}]}}}`
	})
	result, err = partiallyFilledGTC.SubmitOrder(t.Context(), submit)
	require.NoError(t, err, "SubmitOrder must not error for a partially filled GTC order")
	assert.Equal(t, order.PartiallyFilled, result.Status, "result.Status: partial GTC submission should remain active")
	assert.InDelta(t, 0.06, result.RemainingAmount, 1e-12, "result.RemainingAmount: partial GTC submission should return the open remainder")

	postOnly := *submit
	postOnly.TimeInForce = order.PostOnly
	partiallyFilledPostOnly := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":7,"totalSz":"0.04","avgPx":"100"}}]}}}`
	})
	_, err = partiallyFilledPostOnly.SubmitOrder(t.Context(), &postOnly)
	require.ErrorIs(t, err, errActionStatusMalformed, "SubmitOrder must fail closed for a partial post-only fill")

	stopMarket := *submit
	stopMarket.Type = order.StopMarket
	stopMarket.Side = order.Sell
	stopMarket.Price = 80
	stopMarket.TriggerPrice = 90
	stopMarket.TriggerPriceType = order.MarkPrice
	stopMarket.ReduceOnly = true
	stopMarket.TimeInForce = order.UnknownTIF
	triggerResting := newTradingTestExchange(t, nil, func(actionType string, action map[string]any) string {
		assert.Equal(t, "order", actionType, "actionType: trigger submit should send an order action")
		assert.Equal(t, orderGroupingNone, action["grouping"], "action[\"grouping\"]: standalone trigger should not use grouped semantics")
		orders, _ := action["orders"].([]any)
		require.Len(t, orders, 1, "orders: standalone trigger action must contain one order")
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":9}}]}}}`
	})
	result, err = triggerResting.SubmitOrder(t.Context(), &stopMarket)
	require.NoError(t, err, "SubmitOrder must not error for a resting stop-market order")
	assert.Equal(t, "9", result.OrderID, "result.OrderID: submitted trigger order ID should be returned")
	assert.Equal(t, order.StopMarket, result.Type, "result.Type: submitted trigger type should be retained")
	assert.Equal(t, 90.0, result.TriggerPrice, "result.TriggerPrice: submitted trigger price should be retained")
	assert.Equal(t, order.UnknownTIF, result.TimeInForce, "result.TimeInForce: trigger submission should not report a limit time in force")

	partiallyFilledTrigger := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":9,"totalSz":"0.04","avgPx":"90"}}]}}}`
	})
	_, err = partiallyFilledTrigger.SubmitOrder(t.Context(), &stopMarket)
	require.ErrorIs(t, err, errActionStatusMalformed, "SubmitOrder must fail closed for a partial trigger fill")

	bracket := *submit
	bracket.RiskManagementModes = order.RiskManagementModes{
		TakeProfit: order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 110},
		StopLoss:   order.RiskManagement{Enabled: true, TriggerPriceType: order.MarkPrice, Price: 90},
	}
	grouped := newTradingTestExchange(t, nil, func(actionType string, action map[string]any) string {
		assert.Equal(t, "order", actionType, "actionType: bracket submit should send an order action")
		assert.Equal(t, orderGroupingNormalTPSL, action["grouping"], "action[\"grouping\"]: bracket submit should use normal TP/SL grouping")
		orders, _ := action["orders"].([]any)
		require.Len(t, orders, 3, "orders: bracket action must contain parent, take-profit, and stop-loss orders")
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":10}},"waitingForFill","waitingForFill"]}}}`
	})
	result, err = grouped.SubmitOrder(t.Context(), &bracket)
	require.NoError(t, err, "SubmitOrder must not error for a bracket order with deferred children")
	assert.Equal(t, "10", result.OrderID, "result.OrderID: bracket submission should return the parent order ID")
	assert.NoError(t, result.SubmissionError, "result.SubmissionError: accepted bracket children should not set a submission error")

	groupedChildFailure := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":11}},{"error":"bad TP"},"waitingForFill"]}}}`
	})
	result, err = groupedChildFailure.SubmitOrder(t.Context(), &bracket)
	require.NoError(t, err, "SubmitOrder must return the parent without encouraging a duplicate retry for a placed parent with a rejected child")
	require.ErrorIs(t, result.SubmissionError, errGroupedOrderChildFailure, "result.SubmissionError: rejected grouped child must be retained on the parent response")
	assert.Equal(t, "11", result.OrderID, "result.OrderID: partial grouped response should retain the placed parent order ID")

	groupedParentFailure := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"error":"batch rejected"}]}}}`
	})
	_, err = groupedParentFailure.SubmitOrder(t.Context(), &bracket)
	require.ErrorIs(t, err, order.ErrUnableToPlaceOrder, "SubmitOrder must fail the parent submission for deterministic grouped action rejection")

	deferredParent := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":["waitingForFill","waitingForFill","waitingForFill"]}}}`
	})
	_, err = deferredParent.SubmitOrder(t.Context(), &bracket)
	require.ErrorIs(t, err, errActionStatusMalformed, "SubmitOrder must fail closed for deferred grouped parent status")

	market := *submit
	market.Type = order.Market
	market.Price = 0
	market.SlippageTolerance = 0.01
	filled := newTradingTestExchange(t, map[string]string{"allMids": `{"BTC":"100"}`}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":8,"totalSz":"0.04","avgPx":"101"}}]}}}`
	})
	result, err = filled.SubmitOrder(t.Context(), &market)
	require.NoError(t, err, "SubmitOrder must not error for a filled market order")
	assert.Equal(t, order.PartiallyFilledCancelled, result.Status, "result.Status: partial IOC execution should be marked partially filled and cancelled")
	assert.Equal(t, 101.0, result.AverageExecutedPrice, "result.AverageExecutedPrice: average execution price should be returned")
	assert.InDelta(t, 0.06, result.RemainingAmount, 1e-12, "result.RemainingAmount should be derived")
	assert.Equal(t, order.ImmediateOrCancel, result.TimeInForce, "result.TimeInForce: market submission should report its wire time in force")

	overfilled := newTradingTestExchange(t, map[string]string{"allMids": `{"BTC":"100"}`}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":8,"totalSz":"2","avgPx":"101"}}]}}}`
	})
	_, err = overfilled.SubmitOrder(t.Context(), &market)
	require.ErrorIs(t, err, errInvalidFilledSize, "SubmitOrder must fail closed for over-reported fill")

	invalidFill := newTradingTestExchange(t, map[string]string{"allMids": `{"BTC":"100"}`}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":8,"totalSz":"0.0400001","avgPx":"101"}}]}}}`
	})
	_, err = invalidFill.SubmitOrder(t.Context(), &market)
	require.ErrorIs(t, err, errInvalidFilledSize, "SubmitOrder must return the expected error for invalid reported fill precision")

	rejected := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"error":"bad order"}]}}}`
	})
	_, err = rejected.SubmitOrder(t.Context(), submit)
	require.ErrorIs(t, err, order.ErrUnableToPlaceOrder, "SubmitOrder must return the expected error for rejected order")

	malformed := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[]}}}`
	})
	_, err = malformed.SubmitOrder(t.Context(), submit)
	require.ErrorIs(t, err, errActionStatusCount, "SubmitOrder must return the expected error for malformed order action response")

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failed, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: testPerpetualPair, coin: "BTC", sizeDecimals: 5}})
	_, err = failed.SubmitOrder(t.Context(), submit)
	require.Error(t, err, "SubmitOrder must return signed order HTTP failure")
}

func TestCancelOrders(t *testing.T) {
	ex := newTradingTestExchange(t, nil, func(actionType string, action map[string]any) string {
		cancels, _ := action["cancels"].([]any)
		statuses := make([]string, len(cancels))
		for i := range statuses {
			statuses[i] = `"success"`
		}
		return `{"status":"ok","response":{"type":"` + actionType + `","data":{"statuses":[` + strings.Join(statuses, ",") + `]}}}`
	})

	statuses, err := ex.cancelOrders(t.Context(), nil)
	require.NoError(t, err, "cancelOrders must not error for empty cancellation batch")
	assert.Empty(t, statuses, "statuses: empty cancellation batch should return empty status")

	_, err = ex.cancelOrders(t.Context(), make([]order.Cancel, maximumActionBatchSize+1))
	require.ErrorIs(t, err, errActionBatchTooLarge, "cancelOrders must return the expected error for oversized cancellation batch")

	for _, tc := range []struct {
		name       string
		cancel     order.Cancel
		expectedIs error
	}{
		{name: "missing pair", cancel: order.Cancel{OrderID: "1", AssetType: asset.PerpetualContract}, expectedIs: order.ErrPairIsEmpty},
		{name: "missing asset", cancel: order.Cancel{OrderID: "1", Pair: testPerpetualPair}, expectedIs: order.ErrAssetNotSet},
		{name: "missing mapping", cancel: order.Cancel{OrderID: "1", Pair: currency.NewPair(currency.ETH, currency.USDC), AssetType: asset.PerpetualContract}, expectedIs: errPairMappingNotFound},
		{name: "invalid numeric ID", cancel: order.Cancel{OrderID: "bad", Pair: testPerpetualPair, AssetType: asset.PerpetualContract}, expectedIs: order.ErrOrderIDNotSet},
		{name: "zero numeric ID", cancel: order.Cancel{OrderID: "0", Pair: testPerpetualPair, AssetType: asset.PerpetualContract}, expectedIs: order.ErrOrderIDNotSet},
		{name: "invalid client ID", cancel: order.Cancel{ClientOrderID: "invalid", Pair: testPerpetualPair, AssetType: asset.PerpetualContract}, expectedIs: errClientOrderIDInvalid},
		{name: "missing identifier", cancel: order.Cancel{Pair: testPerpetualPair, AssetType: asset.PerpetualContract}, expectedIs: order.ErrOrderIDNotSet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.cancelOrders(t.Context(), []order.Cancel{tc.cancel})
			require.ErrorIs(t, err, tc.expectedIs, "cancelOrders must return the expected error for invalid cancellation")
		})
	}

	uppercaseClientOrderID := strings.ToUpper(validClientOrderID)
	statuses, err = ex.cancelOrders(t.Context(), []order.Cancel{
		{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract},
		{ClientOrderID: uppercaseClientOrderID, Pair: testSpotPair, AssetType: asset.Spot},
	})
	require.NoError(t, err, "cancelOrders must not error for mixed numeric and client-ID cancellation")
	assert.Equal(t, "success", statuses["7"], "statuses[\"7\"]: numeric cancellation status should be returned")
	assert.Equal(t, "success", statuses[uppercaseClientOrderID], "statuses[uppercaseClientOrderID]: Client-ID cancellation status should retain the caller's identifier")
	assert.NotContains(t, statuses, validClientOrderID, "statuses: Client-ID cancellation status should not silently normalise the caller's key")

	require.ErrorIs(t, ex.CancelOrder(t.Context(), nil), order.ErrCancelOrderIsNil, "CancelOrder must return the expected error for nil single cancellation")
	require.NoError(t, ex.CancelOrder(t.Context(), &order.Cancel{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract}), "CancelOrder must not error for valid single cancellation")

	batch, err := ex.CancelBatchOrders(t.Context(), []order.Cancel{{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract}})
	require.NoError(t, err, "CancelBatchOrders must not error for valid batch cancellation")
	assert.Equal(t, "success", batch.Status["7"], "batch.Status[\"7\"]: batch cancellation status should be returned")

	failed := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"err","response":"cancel failed"}`
	})
	statuses, err = failed.cancelOrders(t.Context(), []order.Cancel{
		{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract},
		{ClientOrderID: validClientOrderID, Pair: testPerpetualPair, AssetType: asset.PerpetualContract},
	})
	require.ErrorIs(t, err, errActionResponse, "cancelOrders must return action failures from cancellation groups")
	assert.Empty(t, statuses, "statuses: failed cancellation groups should not report success")

	malformed := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"ok","response":{"data":{"statuses":[]}}}`
	})
	_, err = malformed.cancelOrders(t.Context(), []order.Cancel{{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract}})
	require.ErrorIs(t, err, errActionStatusCount, "cancelOrders must return the expected error for malformed cancellation response")
}

func TestSetLeverage(t *testing.T) {
	ex := newTradingTestExchange(t, nil, func(actionType string, action map[string]any) string {
		assert.Equal(t, "updateLeverage", actionType, "actionType: leverage change should use the expected action")
		assert.Equal(t, float64(0), action["asset"], "action[\"asset\"]: leverage change should use the perpetual universe index")
		assert.Contains(t, []any{true, false}, action["isCross"], "action[\"isCross\"]: leverage change should include the margin mode")
		assert.Contains(t, []any{float64(10), float64(20), float64(30)}, action["leverage"], "action[\"leverage\"]: leverage change should include the requested amount")
		return `{"status":"ok","response":{"type":"default"}}`
	})
	require.NoError(t, ex.SetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Unset, 10, order.UnknownSide), "SetLeverage must use cross margin for unset margin")
	require.NoError(t, ex.SetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Multi, 20, order.UnknownSide), "SetLeverage must use cross margin for multi margin")
	require.NoError(t, ex.SetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Isolated, 30, order.UnknownSide), "SetLeverage must use isolated margin for isolated margin")

	for _, tc := range []struct {
		name       string
		asset      asset.Item
		pair       currency.Pair
		marginType margin.Type
		amount     float64
		expectedIs error
	}{
		{name: "unsupported asset", asset: asset.Spot, pair: testSpotPair, marginType: margin.Multi, amount: 1, expectedIs: asset.ErrNotSupported},
		{name: "missing pair", asset: asset.PerpetualContract, pair: currency.NewPair(currency.ETH, currency.USDC), marginType: margin.Multi, amount: 1, expectedIs: errPairMappingNotFound},
		{name: "zero", asset: asset.PerpetualContract, pair: testPerpetualPair, marginType: margin.Multi, amount: 0, expectedIs: errInvalidLeverage},
		{name: "negative", asset: asset.PerpetualContract, pair: testPerpetualPair, marginType: margin.Multi, amount: -1, expectedIs: errInvalidLeverage},
		{name: "fraction", asset: asset.PerpetualContract, pair: testPerpetualPair, marginType: margin.Multi, amount: 1.5, expectedIs: errInvalidLeverage},
		{name: "nan", asset: asset.PerpetualContract, pair: testPerpetualPair, marginType: margin.Multi, amount: math.NaN(), expectedIs: errInvalidLeverage},
		{name: "infinity", asset: asset.PerpetualContract, pair: testPerpetualPair, marginType: margin.Multi, amount: math.Inf(1), expectedIs: errInvalidLeverage},
		{name: "above maximum", asset: asset.PerpetualContract, pair: testPerpetualPair, marginType: margin.Multi, amount: 41, expectedIs: errInvalidLeverage},
		{name: "unsupported margin", asset: asset.PerpetualContract, pair: testPerpetualPair, marginType: margin.NoMargin, amount: 1, expectedIs: margin.ErrMarginTypeUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ex.SetLeverage(t.Context(), tc.asset, tc.pair, tc.marginType, tc.amount, order.UnknownSide)
			require.ErrorIs(t, err, tc.expectedIs, "SetLeverage must return the expected error for invalid leverage")
		})
	}

	missingMaximum := newTradingTestExchange(t, nil, nil)
	missingMaximum.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: testPerpetualPair, coin: "BTC"}})
	require.ErrorIs(t, missingMaximum.SetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Multi, 1, order.UnknownSide), errInvalidLeverage, "SetLeverage must fail closed for missing leverage metadata")

	isolatedOnly := newTradingTestExchange(t, nil, func(actionType string, action map[string]any) string {
		assert.Equal(t, "updateLeverage", actionType, "actionType: isolated leverage should use the expected action")
		assert.Equal(t, false, action["isCross"], "action[\"isCross\"]: an isolated-only market should use isolated margin")
		return `{"status":"ok","response":{"type":"default"}}`
	})
	isolatedOnly.setPairMappings(asset.PerpetualContract, []pairMapping{{
		pair:         testPerpetualPair,
		coin:         "BTC",
		maxLeverage:  40,
		onlyIsolated: true,
	}})
	err := isolatedOnly.SetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Multi, 1, order.UnknownSide)
	require.ErrorIs(t, err, margin.ErrMarginTypeUnsupported, "SetLeverage must return the expected margin error for cross leverage on an isolated-only market")
	require.ErrorIs(t, err, errCrossMarginUnavailable, "SetLeverage must return the specific restriction for cross leverage on an isolated-only market")
	require.NoError(t, isolatedOnly.SetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Isolated, 1, order.UnknownSide), "SetLeverage must not error for isolated leverage on an isolated-only market")

	hip3Pair := currency.NewPair(currency.NewCode("xyz:XYZ100"), currency.USDC)
	hip3 := newTradingTestExchange(t, nil, func(actionType string, action map[string]any) string {
		assert.Equal(t, "updateLeverage", actionType, "actionType: HIP-3 leverage should use the expected action")
		assert.Equal(t, float64(110000), action["asset"], "action[\"asset\"]: HIP-3 leverage should use the builder asset ID")
		return `{"status":"ok","response":{"type":"default"}}`
	})
	hip3.setPairMappings(asset.PerpetualContract, []pairMapping{{
		pair: hip3Pair, coin: "xyz:XYZ100", dex: testBuilderDEXName, assetID: 110000, maxLeverage: 20,
	}})
	require.NoError(t, hip3.SetLeverage(t.Context(), asset.PerpetualContract, hip3Pair, margin.Multi, 10, order.UnknownSide), "SetLeverage must not error for HIP-3 leverage")

	failed := newTradingTestExchange(t, nil, func(string, map[string]any) string {
		return `{"status":"err","response":"leverage update failed"}`
	})
	require.ErrorIs(t, failed.SetLeverage(t.Context(), asset.PerpetualContract, testPerpetualPair, margin.Multi, 1, order.UnknownSide), errActionResponse, "SetLeverage must return exchange leverage failure")
}

func TestUpdateAccountBalances(t *testing.T) {
	ex := newTradingTestExchange(t, map[string]string{
		"spotClearinghouseState": `{"balances":[{"coin":"USDC","total":"10","hold":"2"}]}`,
		"clearinghouseState":     `{"marginSummary":{"accountValue":"20","totalMarginUsed":"3"},"withdrawable":"16"}`,
		"userAbstraction":        `"default"`,
	}, nil)

	spotAccounts, err := ex.UpdateAccountBalances(t.Context(), asset.Spot)
	require.NoError(t, err, "UpdateAccountBalances must not error for spot balances")
	require.Len(t, spotAccounts, 1, "spotAccounts: spot balances must return one subaccount")
	spotBalance := spotAccounts[0].Balances[currency.USDC]
	assert.Equal(t, 10.0, spotBalance.Total, "spotBalance.Total: spot total should be decoded")
	assert.Equal(t, 2.0, spotBalance.Hold, "spotBalance.Hold: spot hold should be decoded")
	assert.Equal(t, 8.0, spotBalance.Free, "spotBalance.Free: spot free balance should be derived")

	perpetualAccounts, err := ex.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "UpdateAccountBalances must not error for perpetual balances")
	require.Len(t, perpetualAccounts, 1, "perpetualAccounts: perpetual balances must return one subaccount")
	perpetualBalance := perpetualAccounts[0].Balances[currency.USDC]
	assert.Equal(t, 20.0, perpetualBalance.Total, "perpetualBalance.Total: perpetual total should be decoded")
	assert.Equal(t, 3.0, perpetualBalance.Hold, "perpetualBalance.Hold: perpetual hold should be decoded")
	assert.Equal(t, 16.0, perpetualBalance.Free, "perpetualBalance.Free: perpetual withdrawable balance should be used")
	defaultBalances, err := ex.Accounts.CurrencyBalances(nil, asset.All)
	require.NoError(t, err, "CurrencyBalances must not error for aggregating separate default account balances")
	assert.Equal(t, 30.0, defaultBalances[currency.USDC].Total, "defaultBalances[currency.USDC].Total: separate spot and perpetual USDC pools should both be counted")

	for _, tc := range []struct {
		name string
		mode AccountAbstraction
	}{
		{name: "unified account", mode: AccountAbstractionUnified},
		{name: "portfolio margin", mode: AccountAbstractionPortfolio},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unified := newTradingTestExchange(t, map[string]string{
				"spotClearinghouseState": `{"balances":[{"coin":"USDC","total":"30","hold":"4"},{"coin":"HYPE","total":"5","hold":"1"}]}`,
				"userAbstraction":        `"` + string(tc.mode) + `"`,
				infoTypePerpetualDEXs:    `[null,{"name":"` + testBuilderDEXName + `"}]`,
			}, nil)
			staleDefault := accounts.NewSubAccount(asset.PerpetualContract, officialSigningAddress)
			staleDefault.Balances.Set(currency.USDC, accounts.Balance{Total: 30})
			staleHIP3 := accounts.NewSubAccount(asset.PerpetualContract, officialSigningAddress+":"+testBuilderDEXName)
			staleHIP3.Balances.Set(currency.NewCode("HYPE"), accounts.Balance{Total: 5})
			require.NoError(t, unified.Accounts.Save(t.Context(), accounts.SubAccounts{staleDefault, staleHIP3}, true), "Save must not error for saving stale separate perpetual balances")
			spotResult, err := unified.UpdateAccountBalances(t.Context(), asset.Spot)
			require.NoError(t, err, "UpdateAccountBalances must not error for unified spot balances")
			require.Len(t, spotResult, 1, "spotResult: unified spot balances must return one subaccount")
			result, err := unified.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
			require.NoError(t, err, "UpdateAccountBalances must not error for unified perpetual balances")
			require.Len(t, result, 2, "result: unified perpetual refresh must clear every registered DEX subaccount")
			assert.Equal(t, officialSigningAddress, result[0].ID, "result[0].ID: unified default DEX cleanup should use the account address")
			assert.Empty(t, result[0].Balances, "result[0].Balances: unified default DEX balances should be cleared")
			assert.Equal(t, officialSigningAddress+":"+testBuilderDEXName, result[1].ID, "result[1].ID: unified HIP-3 cleanup should use the scoped account ID")
			assert.Empty(t, result[1].Balances, "result[1].Balances: unified HIP-3 balances should be cleared")
			balances, err := unified.Accounts.CurrencyBalances(nil, asset.All)
			require.NoError(t, err, "CurrencyBalances must not error for aggregating unified balances")
			assert.Equal(t, 30.0, balances[currency.USDC].Total, "balances[currency.USDC].Total: unified USDC should be counted once")
			assert.Equal(t, 5.0, balances[currency.NewCode("HYPE")].Total, "balances[currency.NewCode(\"HYPE\")].Total: unified collateral should be counted once")
		})
	}

	hip3 := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for HIP-3 balance request") {
			return
		}
		var response string
		switch request.Type {
		case "userAbstraction":
			response = `"disabled"`
		case infoTypePerpetualDEXs:
			response = `[null,{"name":"` + testBuilderDEXName + `"}]`
		case "spotMeta":
			response = `{"tokens":[{"name":"USDC","index":0},{"name":"HYPE","index":150}]}`
		case infoTypeMetadata:
			if request.DEX == testBuilderDEXName {
				response = `{"collateralToken":150}`
			} else {
				response = `{"collateralToken":0}`
			}
		case "clearinghouseState":
			if request.DEX == testBuilderDEXName {
				response = `{"marginSummary":{"accountValue":"7","totalMarginUsed":"2"},"withdrawable":"4"}`
			} else {
				response = `{"marginSummary":{"accountValue":"20","totalMarginUsed":"3"},"withdrawable":"16"}`
			}
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Write should not error for HIP-3 balance response")
	}))
	setTestCredentials(hip3, &accounts.Credentials{Key: officialSigningAddress})
	hip3Accounts, err := hip3.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "UpdateAccountBalances must not error for cold standard HIP-3 balances")
	require.Len(t, hip3Accounts, 2, "hip3Accounts: standard mode must preserve separate balances even without active market mappings")
	assert.Equal(t, officialSigningAddress, hip3Accounts[0].ID, "hip3Accounts[0].ID: default DEX should use the account address")
	assert.Equal(t, 20.0, hip3Accounts[0].Balances[currency.USDC].Total, "hip3Accounts[0].Balances[currency.USDC].Total: default DEX collateral should be retained")
	assert.Equal(t, officialSigningAddress+":xyz", hip3Accounts[1].ID, "hip3Accounts[1].ID: HIP-3 DEX should have a scoped subaccount ID")
	assert.Equal(t, 7.0, hip3Accounts[1].Balances[currency.NewCode("HYPE")].Total, "hip3Accounts[1].Balances[currency.NewCode(\"HYPE\")].Total: HIP-3 collateral token should be retained")

	_, err = ex.UpdateAccountBalances(t.Context(), asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "UpdateAccountBalances must return the expected error for unsupported balance asset")

	missingCredentials := new(Exchange)
	missingCredentials.SetDefaults()
	_, err = missingCredentials.UpdateAccountBalances(t.Context(), asset.Spot)
	require.Error(t, err, "UpdateAccountBalances must error for balances without credentials")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(errorExchange, &accounts.Credentials{Key: officialSigningAddress})
	_, err = errorExchange.UpdateAccountBalances(t.Context(), asset.Spot)
	require.Error(t, err, "UpdateAccountBalances must return spot balance HTTP failure")
	_, err = errorExchange.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
	require.Error(t, err, "UpdateAccountBalances must return perpetual balance HTTP failure")

	unifiedStateFailure := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for unified balance failure request") {
			return
		}
		switch request.Type {
		case "userAbstraction":
			_, writeErr := w.Write([]byte(`"unifiedAccount"`))
			assert.NoError(t, writeErr, "Write should not error for abstraction mode")
			return
		case infoTypePerpetualDEXs:
			_, writeErr := w.Write([]byte(`[null]`))
			assert.NoError(t, writeErr, "Write should not error for perpetual DEX registry")
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(unifiedStateFailure, &accounts.Credentials{Key: officialSigningAddress})
	result, err := unifiedStateFailure.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
	require.NoError(t, err, "UpdateAccountBalances must not query a second balance source for unified perpetual refresh")
	require.Len(t, result, 1, "result: unified perpetual refresh must return one clearing subaccount")
	assert.Empty(t, result[0].Balances, "result[0].Balances: unified perpetual refresh should clear the separate balance bucket")

	for _, tc := range []struct {
		name      string
		responses map[string]string
		expected  error
	}{
		{
			name: "spot metadata failure",
			responses: map[string]string{
				"userAbstraction": `"default"`,
				"spotMeta":        `{`,
			},
		},
		{
			name: "perpetual DEX registry failure",
			responses: map[string]string{
				"userAbstraction":     `"default"`,
				infoTypePerpetualDEXs: `{`,
			},
		},
		{
			name:     "duplicate collateral token",
			expected: errUnexpectedResponseLength,
			responses: map[string]string{
				"userAbstraction": `"default"`,
				"spotMeta":        `{"tokens":[{"name":"USDC","index":0},{"name":"USDC2","index":0}]}`,
			},
		},
		{
			name: "metadata failure",
			responses: map[string]string{
				"userAbstraction": `"default"`,
				"spotMeta":        spotMetadataJSON,
				infoTypeMetadata:  `{`,
			},
		},
		{
			name:     "missing collateral token",
			expected: errSpotTokenNotFound,
			responses: map[string]string{
				"userAbstraction": `"default"`,
				"spotMeta":        `{"tokens":[{"name":"USDC","index":0}]}`,
				infoTypeMetadata:  `{"collateralToken":150}`,
			},
		},
		{
			name:     "blank collateral token",
			expected: errSpotTokenNotFound,
			responses: map[string]string{
				"userAbstraction": `"default"`,
				"spotMeta":        `{"tokens":[{"name":" ","index":0}]}`,
				infoTypeMetadata:  `{"collateralToken":0}`,
			},
		},
		{
			name: "clearinghouse failure",
			responses: map[string]string{
				"userAbstraction":    `"default"`,
				"spotMeta":           spotMetadataJSON,
				"clearinghouseState": `{`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failed := newTradingTestExchange(t, tc.responses, nil)
			_, err := failed.UpdateAccountBalances(t.Context(), asset.PerpetualContract)
			require.Error(t, err, "UpdateAccountBalances must return an error for invalid account balance response")
			if tc.expected != nil {
				require.ErrorIs(t, err, tc.expected, "UpdateAccountBalances must return the expected error for invalid account balance response")
			}
		})
	}
}

func TestModifyOrder(t *testing.T) {
	openOrderResponse := `{"status":"order","order":{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":true,"orderType":"Limit","tif":"Gtc","cloid":"` + validClientOrderID + `"},"status":"open","statusTimestamp":1700000001000}}`
	ex := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, func(actionType string, action map[string]any) string {
		assert.Equal(t, "batchModify", actionType, "actionType: modification should send a batchModify action")
		assert.NotEmpty(t, action["modifies"], "action[\"modifies\"]: modification action should include one order")
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":7}}]}}}`
	})

	_, err := ex.ModifyOrder(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrModifyOrderIsNil, "ModifyOrder must return the expected error for nil modification")

	missingSigner := newTradingTestExchange(t, nil, nil)
	setTestCredentials(missingSigner, &accounts.Credentials{Key: officialSigningAddress})
	_, err = missingSigner.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "ModifyOrder must fail before querying the existing order for missing signing key")

	_, err = ex.ModifyOrder(t.Context(), &order.Modify{OrderID: "bad", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet, "ModifyOrder must return the expected error for non-numeric order ID")
	_, err = ex.ModifyOrder(t.Context(), &order.Modify{OrderID: "0", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet, "ModifyOrder must return the expected error for zero order ID")
	_, err = ex.ModifyOrder(t.Context(), &order.Modify{ClientOrderID: "invalid", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, errClientOrderIDInvalid, "ModifyOrder must return the expected error for invalid client order ID")
	_, err = ex.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract, TriggerPrice: 90})
	require.ErrorIs(t, err, errRiskManagementUnsupported, "ModifyOrder must fail closed for trigger price on a non-trigger modification")

	result, err := ex.ModifyOrder(t.Context(), &order.Modify{
		OrderID:          "7",
		Pair:             testPerpetualPair,
		AssetType:        asset.PerpetualContract,
		Price:            101,
		Amount:           1.5,
		Side:             order.Sell,
		Type:             order.Limit,
		TimeInForce:      order.PostOnly,
		NewClientOrderID: "0x00000000000000000000000000000002",
	})
	require.NoError(t, err, "ModifyOrder must not error for an order with explicit fields")
	assert.Equal(t, order.Sell, result.Side, "result.Side: modified side should be returned")
	assert.Equal(t, 101.0, result.Price, "result.Price: modified price should be returned")
	assert.Equal(t, 1.5, result.Amount, "result.Amount: modified amount should be returned")
	assert.Equal(t, 1.5, result.RemainingAmount, "result.RemainingAmount: resting modified amount should remain open")
	assert.Equal(t, order.PostOnly, result.TimeInForce, "result.TimeInForce: modified time in force should be returned")
	assert.Equal(t, "0x00000000000000000000000000000002", result.ClientOrderID, "result.ClientOrderID: new client order ID should be returned")

	result, err = ex.ModifyOrder(t.Context(), &order.Modify{ClientOrderID: strings.ToUpper(validClientOrderID), Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.NoError(t, err, "ModifyOrder must not error for by client ID with inherited fields")
	assert.Equal(t, order.Buy, result.Side, "result.Side: existing side should be inherited")
	assert.Equal(t, order.Limit, result.Type, "result.Type: existing type should be inherited")
	assert.Equal(t, order.GoodTillCancel, result.TimeInForce, "result.TimeInForce: existing time in force should be inherited")
	assert.Equal(t, 1.0, result.Amount, "result.Amount: only the existing remaining amount should be inherited")
	assert.Equal(t, 100.0, result.Price, "result.Price: existing price should be inherited")
	assert.Equal(t, "7", result.OrderID, "result.OrderID: modification by client ID should return the resolved numeric order ID")

	filled := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":7,"totalSz":"1","avgPx":"100"}}]}}}`
	})
	result, err = filled.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.NoError(t, err, "ModifyOrder must not error for immediately filled modification")
	assert.Equal(t, order.Filled, result.Status, "result.Status: immediately filled modification should be marked filled")
	assert.Zero(t, result.RemainingAmount, "result.RemainingAmount: filled modification should have no remaining amount")

	partiallyFilled := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":7,"totalSz":"0.4","avgPx":"100"}}]}}}`
	})
	result, err = partiallyFilled.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.NoError(t, err, "ModifyOrder must not error for partially filled GTC modification")
	assert.Equal(t, order.PartiallyFilled, result.Status, "result.Status: partial GTC modification should remain active")
	assert.Equal(t, 0.6, result.RemainingAmount, "result.RemainingAmount: partial GTC modification should return the open remainder")

	marketModification := newTradingTestExchange(t, map[string]string{
		"orderStatus": openOrderResponse,
		"allMids":     `{"BTC":"100"}`,
	}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":7,"totalSz":"1","avgPx":"101"}}]}}}`
	})
	result, err = marketModification.ModifyOrder(t.Context(), &order.Modify{
		OrderID:           "7",
		Pair:              testPerpetualPair,
		AssetType:         asset.PerpetualContract,
		Type:              order.Market,
		SlippageTolerance: 0.01,
	})
	require.NoError(t, err, "ModifyOrder must not error for a limit order to market")
	assert.Equal(t, 101.0, result.Price, "result.Price: market modification should report the submitted wire price")
	assert.Equal(t, order.ImmediateOrCancel, result.TimeInForce, "result.TimeInForce: market modification should report its wire time in force")

	partiallyFilledIOC := newTradingTestExchange(t, map[string]string{
		"orderStatus": openOrderResponse,
		"allMids":     `{"BTC":"100"}`,
	}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":7,"totalSz":"0.4","avgPx":"101"}}]}}}`
	})
	result, err = partiallyFilledIOC.ModifyOrder(t.Context(), &order.Modify{
		OrderID:           "7",
		Pair:              testPerpetualPair,
		AssetType:         asset.PerpetualContract,
		Type:              order.Market,
		SlippageTolerance: 0.01,
	})
	require.NoError(t, err, "ModifyOrder must not error for partially filled IOC modification")
	assert.Equal(t, order.PartiallyFilledCancelled, result.Status, "result.Status: partial IOC modification should be marked partially filled and cancelled")
	assert.Equal(t, 0.6, result.RemainingAmount, "result.RemainingAmount: partial IOC modification should return the unexecuted amount")

	triggerOrderResponse := `{"status":"order","order":{"order":{"coin":"BTC","side":"A","limitPx":"80","sz":"1","origSz":"1","oid":12,"timestamp":1700000000000,"isTrigger":true,"triggerPx":"90","reduceOnly":true,"orderType":"Stop Market","tif":""},"status":"open","statusTimestamp":1700000001000}}`
	triggerModification := newTradingTestExchange(t, map[string]string{"orderStatus": triggerOrderResponse}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":["waitingForTrigger"]}}}`
	})
	_, err = triggerModification.ModifyOrder(t.Context(), &order.Modify{
		OrderID:   "12",
		Pair:      testPerpetualPair,
		AssetType: asset.PerpetualContract,
	})
	require.ErrorIs(t, err, errRiskManagementUnsupported, "ModifyOrder must fail closed for trigger modification without an explicit mark-price source")
	result, err = triggerModification.ModifyOrder(t.Context(), &order.Modify{
		OrderID:          "12",
		Pair:             testPerpetualPair,
		AssetType:        asset.PerpetualContract,
		TriggerPriceType: order.MarkPrice,
	})
	require.NoError(t, err, "ModifyOrder must not error for a deferred trigger order with inherited fields")
	assert.Equal(t, "12", result.OrderID, "result.OrderID: deferred trigger modification should retain the existing order ID")
	assert.Equal(t, order.StopMarket, result.Type, "result.Type: deferred trigger modification should retain the existing type")
	assert.Equal(t, 90.0, result.TriggerPrice, "result.TriggerPrice: deferred trigger modification should inherit the trigger price")
	assert.Equal(t, order.UnknownTIF, result.TimeInForce, "result.TimeInForce: trigger modification should not report a limit time in force")

	overfilled := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":7,"totalSz":"2","avgPx":"100"}}]}}}`
	})
	_, err = overfilled.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, errInvalidFilledSize, "ModifyOrder must fail closed for over-reported modification fill")

	invalidFill := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"oid":7,"totalSz":"0.400001","avgPx":"100"}}]}}}`
	})
	_, err = invalidFill.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, errInvalidFilledSize, "ModifyOrder must return the expected error for invalid modification fill precision")

	rejected := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"type":"order","data":{"statuses":[{"error":"bad modify"}]}}}`
	})
	_, err = rejected.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, order.ErrUnableToPlaceOrder, "ModifyOrder must return the expected error for rejected modification")

	notFound := newTradingTestExchange(t, map[string]string{"orderStatus": `{"status":"unknownOid"}`}, nil)
	_, err = notFound.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, order.ErrOrderNotFound, "ModifyOrder must return the expected error for missing existing order")

	filledOrderResponse := `{"status":"order","order":{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"0","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":true,"orderType":"Limit","tif":"Gtc"},"status":"filled","statusTimestamp":1700000001000}}`
	terminal := newTradingTestExchange(t, map[string]string{"orderStatus": filledOrderResponse}, nil)
	_, err = terminal.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, errOrderNotModifiable, "ModifyOrder must not be submitted for modification for terminal existing order")

	invalidBuild := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, nil)
	_, err = invalidBuild.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract, Amount: 0.000001})
	require.ErrorIs(t, err, errSizePrecision, "ModifyOrder must return the expected error for invalid modified size")

	malformed := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"data":{"statuses":[]}}}`
	})
	_, err = malformed.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, errActionStatusCount, "ModifyOrder must return the expected error for malformed modification response")

	actionFailure := newTradingTestExchange(t, map[string]string{"orderStatus": openOrderResponse}, func(string, map[string]any) string {
		return `{"status":"err","response":"modify failed"}`
	})
	_, err = actionFailure.ModifyOrder(t.Context(), &order.Modify{OrderID: "7", Pair: testPerpetualPair, AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, errActionResponse, "ModifyOrder must return signed modification failure")
}

func TestGetOrderInfo(t *testing.T) {
	response := `{"status":"order","order":{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000}}`
	ex := newTradingTestExchange(t, map[string]string{"orderStatus": response}, nil)

	_, err := ex.GetOrderInfo(t.Context(), "", testPerpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, order.ErrOrderIDNotSet, "GetOrderInfo must return the expected error for empty order ID")
	_, err = ex.GetOrderInfo(t.Context(), "7", testPerpetualPair, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetOrderInfo must return the expected error for unsupported order asset")
	_, err = ex.GetOrderInfo(t.Context(), "invalid", testPerpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, errClientOrderIDInvalid, "GetOrderInfo must return the expected error for invalid client order ID")

	detail, err := ex.GetOrderInfo(t.Context(), "7", testPerpetualPair, asset.PerpetualContract)
	require.NoError(t, err, "GetOrderInfo must not error for order by numeric ID")
	assert.Equal(t, "7", detail.OrderID, "detail.OrderID: order detail should be returned")
	_, err = ex.GetOrderInfo(t.Context(), validClientOrderID, currency.EMPTYPAIR, asset.Empty)
	require.NoError(t, err, "GetOrderInfo must not error for order by client ID without filters")

	_, err = ex.GetOrderInfo(t.Context(), "7", testSpotPair, asset.PerpetualContract)
	require.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo must return not found for mismatched pair filter")
	_, err = ex.GetOrderInfo(t.Context(), "7", testPerpetualPair, asset.Spot)
	require.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo must return not found for mismatched asset filter")

	unknown := newTradingTestExchange(t, map[string]string{"orderStatus": `{"status":"unknownOid"}`}, nil)
	_, err = unknown.GetOrderInfo(t.Context(), "7", testPerpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo must return not found for unknown order response")

	nilOrder := newTradingTestExchange(t, map[string]string{"orderStatus": `{"status":"order","order":null}`}, nil)
	_, err = nilOrder.GetOrderInfo(t.Context(), "7", testPerpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, order.ErrOrderNotFound, "GetOrderInfo must return not found for order response without order data")

	missingCredentials := new(Exchange)
	missingCredentials.SetDefaults()
	_, err = missingCredentials.GetOrderInfo(t.Context(), "7", testPerpetualPair, asset.PerpetualContract)
	require.Error(t, err, "GetOrderInfo must error for order without credentials")

	badOrder := newTradingTestExchange(t, map[string]string{"orderStatus": `{"status":"order","order":{"order":{"coin":"MISSING","side":"B","limitPx":"1","sz":"1","origSz":"1","oid":7,"timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000}}`}, nil)
	_, err = badOrder.GetOrderInfo(t.Context(), "7", testPerpetualPair, asset.PerpetualContract)
	require.ErrorIs(t, err, errPairMappingNotFound, "GetOrderInfo must return its conversion error for invalid returned order")

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failed, &accounts.Credentials{Key: officialSigningAddress})
	failed.setPairMappings(asset.PerpetualContract, []pairMapping{{pair: testPerpetualPair, coin: "BTC"}})
	_, err = failed.GetOrderInfo(t.Context(), "7", testPerpetualPair, asset.PerpetualContract)
	require.Error(t, err, "GetOrderInfo must return order-status HTTP failure")
}

func TestGetOpenOrdersForAsset(t *testing.T) {
	var requestedDEXes []string
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for scoped open-orders request") {
			return
		}
		if request.Type == infoTypePerpetualDEXs {
			_, err := w.Write([]byte(`[null,{"name":"` + testBuilderDEXName + `"}]`))
			assert.NoError(t, err, "Write should not error for the perpetual DEX registry")
			return
		}
		requestedDEXes = append(requestedDEXes, request.DEX)
		coin := "BTC"
		orderID := 7
		if request.DEX == testBuilderDEXName {
			coin = "xyz:XYZ100"
			orderID = 8
		}
		_, err := fmt.Fprintf(w, `[{"coin":%q,"oid":%d}]`, coin, orderID)
		assert.NoError(t, err, "Fprintf should not error for scoped open-orders response")
	}))
	orders, err := ex.getOpenOrdersForAsset(t.Context(), officialSigningAddress, asset.PerpetualContract)
	require.NoError(t, err, "getOpenOrdersForAsset must not error for cold open orders across registered perpetual DEXes")
	require.Len(t, orders, 2, "getOpenOrdersForAsset must combine one response per DEX")
	assert.Equal(t, []string{"", testBuilderDEXName}, requestedDEXes, "requestedDEXes: perpetual open orders should query each DEX once")

	requestedDEXes = nil
	orders, err = ex.getOpenOrdersForAsset(t.Context(), officialSigningAddress, asset.Spot)
	require.NoError(t, err, "getOpenOrdersForAsset must not error for spot open orders")
	require.Len(t, orders, 1, "getOpenOrdersForAsset must use the default DEX response for spot")
	assert.Equal(t, []string{""}, requestedDEXes, "requestedDEXes: spot open orders should query only the default DEX")
	_, err = ex.getOpenOrdersForAsset(t.Context(), officialSigningAddress, asset.Options)
	require.ErrorIs(t, err, asset.ErrNotSupported, "getOpenOrdersForAsset must return the expected error for unsupported open-order asset")

	noMappings := newStaticInfoExchange(t, map[string]string{"frontendOpenOrders": `[]`})
	orders, err = noMappings.getOpenOrdersForAsset(t.Context(), officialSigningAddress, asset.PerpetualContract)
	require.NoError(t, err, "getOpenOrdersForAsset must not error for perpetual open orders without cached mappings")
	assert.Empty(t, orders, "orders: default DEX open-order fallback should retain an empty response")

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for failed open-orders request") {
			return
		}
		if request.Type == infoTypePerpetualDEXs {
			_, err := w.Write([]byte(`[null]`))
			assert.NoError(t, err, "Write should not error for the failed open-orders registry")
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = failed.getOpenOrdersForAsset(t.Context(), officialSigningAddress, asset.PerpetualContract)
	require.Error(t, err, "getOpenOrdersForAsset must return scoped open-order HTTP failure")
}

func TestGetOrders(t *testing.T) {
	openOrders := `[{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"},{"coin":"@107","side":"A","limitPx":"10","sz":"2","origSz":"2","oid":8,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"}]`
	history := `[{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"0","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"},"status":"filled","statusTimestamp":1700000001000},{"order":{"coin":"@107","side":"A","limitPx":"10","sz":"0","origSz":"2","oid":8,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"},"status":"filled","statusTimestamp":1700000001000}]`
	ex := newTradingTestExchange(t, map[string]string{"frontendOpenOrders": openOrders, "historicalOrders": history}, nil)

	_, err := ex.GetActiveOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrGetOrdersRequestIsNil, "GetActiveOrders must return the expected error for nil active-order request")
	_, err = ex.GetOrderHistory(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrGetOrdersRequestIsNil, "GetOrderHistory must return the expected error for nil order-history request")

	unsupported := &order.MultiOrderRequest{AssetType: asset.Options, Side: order.AnySide, Type: order.AnyType}
	_, err = ex.GetActiveOrders(t.Context(), unsupported)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetActiveOrders must return the expected error for unsupported active-order asset")
	_, err = ex.GetOrderHistory(t.Context(), unsupported)
	require.ErrorIs(t, err, asset.ErrNotSupported, "GetOrderHistory must return the expected error for unsupported history asset")

	orderRequest := &order.MultiOrderRequest{AssetType: asset.PerpetualContract, Side: order.AnySide, Type: order.AnyType}
	missingCredentials := new(Exchange)
	missingCredentials.SetDefaults()
	_, err = missingCredentials.GetActiveOrders(t.Context(), orderRequest)
	require.Error(t, err, "GetActiveOrders must error for active orders without credentials")
	_, err = missingCredentials.GetOrderHistory(t.Context(), orderRequest)
	require.Error(t, err, "GetOrderHistory must error for order history without credentials")

	active, err := ex.GetActiveOrders(t.Context(), orderRequest)
	require.NoError(t, err, "GetActiveOrders must not error for active perpetual orders")
	require.Len(t, active, 1, "active: active orders must filter out other assets")
	assert.Equal(t, "7", active[0].OrderID, "active[0].OrderID: active perpetual order should be returned")

	historical, err := ex.GetOrderHistory(t.Context(), orderRequest)
	require.NoError(t, err, "GetOrderHistory must not error for perpetual order history")
	require.Len(t, historical, 1, "historical: order history must filter out other assets")
	assert.Equal(t, order.Filled, historical[0].Status, "historical[0].Status: historical order status should be converted")

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failed, &accounts.Credentials{Key: officialSigningAddress})
	_, err = failed.GetActiveOrders(t.Context(), orderRequest)
	require.Error(t, err, "GetActiveOrders must return active-order HTTP failure")
	_, err = failed.GetOrderHistory(t.Context(), orderRequest)
	require.Error(t, err, "GetOrderHistory must return order-history HTTP failure")

	badOrders := newTradingTestExchange(t, map[string]string{
		"frontendOpenOrders": `[{"coin":"MISSING","side":"B","limitPx":"1","sz":"1","origSz":"1","oid":7,"timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"}]`,
		"historicalOrders":   `[{"order":{"coin":"MISSING","side":"B","limitPx":"1","sz":"1","origSz":"1","oid":7,"timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000}]`,
	}, nil)
	_, err = badOrders.GetActiveOrders(t.Context(), orderRequest)
	require.ErrorIs(t, err, errPairMappingNotFound, "GetActiveOrders must return active-order conversion failure")
	_, err = badOrders.GetOrderHistory(t.Context(), orderRequest)
	require.ErrorIs(t, err, errPairMappingNotFound, "GetOrderHistory must return order-history conversion failure")

	mixedOrders := newTradingTestExchange(t, map[string]string{
		"frontendOpenOrders": `[{"coin":"MISSING","side":"B","limitPx":"1","sz":"1","origSz":"1","oid":7,"timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"},{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":8,"timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"}]`,
		"historicalOrders":   `[{"order":{"coin":"MISSING","side":"B","limitPx":"1","sz":"1","origSz":"1","oid":7,"timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000},{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"0","origSz":"2","oid":8,"timestamp":1700000000000,"orderType":"Limit","tif":"Gtc"},"status":"filled","statusTimestamp":1700000001000}]`,
	}, nil)
	active, err = mixedOrders.GetActiveOrders(t.Context(), orderRequest)
	require.ErrorIs(t, err, errPairMappingNotFound, "GetActiveOrders must report the skipped conversion for mixed active orders")
	require.Len(t, active, 1, "active: mixed active orders must retain convertible orders")
	assert.Equal(t, "8", active[0].OrderID, "active[0].OrderID: convertible active order should be returned")
	historical, err = mixedOrders.GetOrderHistory(t.Context(), orderRequest)
	require.ErrorIs(t, err, errPairMappingNotFound, "GetOrderHistory must report the skipped conversion for mixed order history")
	require.Len(t, historical, 1, "historical: mixed order history must retain convertible orders")
	assert.Equal(t, "8", historical[0].OrderID, "historical[0].OrderID: convertible historical order should be returned")
}

func TestCancelAllOrders(t *testing.T) {
	openOrders := `[{"coin":"BTC","oid":7},{"coin":"@107","oid":8}]`
	ex := newTradingTestExchange(t, map[string]string{"frontendOpenOrders": openOrders}, func(actionType string, action map[string]any) string {
		cancels, _ := action["cancels"].([]any)
		statuses := make([]string, len(cancels))
		for i := range statuses {
			statuses[i] = `"success"`
		}
		return `{"status":"ok","response":{"type":"` + actionType + `","data":{"statuses":[` + strings.Join(statuses, ",") + `]}}}`
	})

	_, err := ex.CancelAllOrders(t.Context(), nil)
	require.ErrorIs(t, err, order.ErrCancelOrderIsNil, "CancelAllOrders must return the expected error for nil cancel-all request")
	_, err = ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.Options})
	require.ErrorIs(t, err, asset.ErrNotSupported, "CancelAllOrders must return the expected error for unsupported cancel-all asset")

	missingCredentials := new(Exchange)
	missingCredentials.SetDefaults()
	_, err = missingCredentials.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.Error(t, err, "CancelAllOrders must error for cancel-all without credentials")

	result, err := ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.NoError(t, err, "CancelAllOrders must not error for canceling all perpetual orders")
	assert.Equal(t, "success", result.Status["7"], "result.Status[\"7\"]: matching perpetual order should be canceled")
	assert.NotContains(t, result.Status, "8", "result.Status: other asset order should be excluded")

	result, err = ex.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract, Pair: currency.NewPair(currency.ETH, currency.USDC)})
	require.NoError(t, err, "CancelAllOrders must not error for canceling unmatched pair")
	assert.Empty(t, result.Status, "result.Status: unmatched pair should produce no cancellations")

	badOrders := newTradingTestExchange(t, map[string]string{"frontendOpenOrders": `[{"coin":"MISSING","oid":7}]`}, nil)
	_, err = badOrders.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, errPairMappingNotFound, "CancelAllOrders must return cancel-all mapping failure")

	mixedOrders := newTradingTestExchange(t, map[string]string{"frontendOpenOrders": `[{"coin":"MISSING","oid":6},{"coin":"BTC","oid":7}]`}, func(actionType string, _ map[string]any) string {
		return `{"status":"ok","response":{"type":"` + actionType + `","data":{"statuses":["success"]}}}`
	})
	result, err = mixedOrders.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.ErrorIs(t, err, errPairMappingNotFound, "CancelAllOrders must report an unmappable skipped order for cancel-all")
	assert.Equal(t, "success", result.Status["7"], "result.Status[\"7\"]: cancel-all should still cancel mappable orders")

	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failed, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	_, err = failed.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.Error(t, err, "CancelAllOrders must return cancel-all order lookup failure")

	orders := make([]string, maximumActionBatchSize+1)
	for i := range orders {
		orders[i] = `{"coin":"BTC","oid":` + strconv.Itoa(i+1) + `}`
	}
	chunked := newTradingTestExchange(t, map[string]string{"frontendOpenOrders": `[` + strings.Join(orders, ",") + `]`}, func(actionType string, action map[string]any) string {
		cancels, _ := action["cancels"].([]any)
		statuses := make([]string, len(cancels))
		for i := range statuses {
			statuses[i] = `"success"`
		}
		return `{"status":"ok","response":{"type":"` + actionType + `","data":{"statuses":[` + strings.Join(statuses, ",") + `]}}}`
	})
	result, err = chunked.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.NoError(t, err, "CancelAllOrders must chunk orders beyond one action batch for cancel-all")
	assert.Len(t, result.Status, maximumActionBatchSize+1, "result.Status: every chunked order should be canceled")

	partial := newTradingTestExchange(t, map[string]string{"frontendOpenOrders": `[{"coin":"BTC","oid":7}]`}, func(string, map[string]any) string {
		return `{"status":"ok","response":{"data":{"statuses":[{"error":"already closed"}]}}}`
	})
	result, err = partial.CancelAllOrders(t.Context(), &order.Cancel{AssetType: asset.PerpetualContract})
	require.Error(t, err, "CancelAllOrders must return cancel-all per-order failure")
	assert.Equal(t, "already closed", result.Status["7"], "result.Status[\"7\"]: cancel-all failure status should be retained")
}
