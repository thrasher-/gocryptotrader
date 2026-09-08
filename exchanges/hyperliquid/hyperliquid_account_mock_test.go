package hyperliquid

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

func TestGetUserFees(t *testing.T) {
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for user-fees request") {
			return
		}
		_, err := w.Write([]byte(`{"userCrossRate":"0.0003","userAddRate":"0.0001","userSpotCrossRate":"0.0005","userSpotAddRate":"0.0002"}`))
		assert.NoError(t, err, "Write should not error for user-fees response")
	}))
	_, err := ex.GetUserFees(t.Context(), "invalid")
	require.ErrorIs(t, err, errInvalidAddress, "GetUserFees must return the expected error for invalid fee address")
	result, err := ex.GetUserFees(t.Context(), strings.ToUpper(officialSigningAddress))
	require.NoError(t, err, "GetUserFees must not error for valid user fees")
	assert.Equal(t, officialSigningAddress, got.User, "got.User: fee address should be normalised")
	assert.Equal(t, 0.0005, result.UserSpotCrossRate.Float64(), "Float64: spot taker rate should be decoded")

	nullExchange := newStaticInfoExchange(t, map[string]string{"userFees": "null"})
	_, err = nullExchange.GetUserFees(t.Context(), officialSigningAddress)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetUserFees must return the expected error for null user fees")
	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetUserFees(t.Context(), officialSigningAddress)
	require.Error(t, err, "GetUserFees must error for user fees from a failing server")
}

func TestGetActiveAssetData(t *testing.T) {
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for active-asset request") {
			return
		}
		_, err := w.Write([]byte(`{"user":"` + officialSigningAddress + `","coin":"BTC","leverage":{"type":"cross","value":20}}`))
		assert.NoError(t, err, "Write should not error for active-asset response")
	}))
	_, err := ex.GetActiveAssetData(t.Context(), "invalid", "BTC")
	require.ErrorIs(t, err, errInvalidAddress, "GetActiveAssetData must return the expected error for invalid active-asset address")
	_, err = ex.GetActiveAssetData(t.Context(), officialSigningAddress, " ")
	require.ErrorIs(t, err, errCoinRequired, "GetActiveAssetData must return the expected error for blank active-asset coin")
	result, err := ex.GetActiveAssetData(t.Context(), officialSigningAddress, " BTC ")
	require.NoError(t, err, "GetActiveAssetData must not error for valid active-asset data")
	assert.Equal(t, "BTC", got.Coin, "got.Coin: active-asset coin should be trimmed")
	assert.Equal(t, 20.0, result.Leverage.Value.Float64(), "Float64: active-asset leverage should be decoded")

	nullExchange := newStaticInfoExchange(t, map[string]string{"activeAssetData": "null"})
	_, err = nullExchange.GetActiveAssetData(t.Context(), officialSigningAddress, "BTC")
	require.ErrorIs(t, err, common.ErrNilPointer, "GetActiveAssetData must return the expected error for null active-asset data")
	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetActiveAssetData(t.Context(), officialSigningAddress, "BTC")
	require.Error(t, err, "GetActiveAssetData must error for active-asset data from a failing server")
}

func TestGetAccountInfoEndpoints(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		testUserRoleInfoType:     testUserRoleResponse,
		"vaultDetails":           `{"vaultAddress":"` + testVaultAddress + `","leader":"` + officialSigningAddress + `"}`,
		"spotClearinghouseState": `{"balances":[{"coin":"USDC","token":0,"total":"10","hold":"2","entryNtl":"1"}]}`,
		"clearinghouseState":     `{"marginSummary":{"accountValue":"12","totalMarginUsed":"3"},"withdrawable":"8","assetPositions":[]}`,
		"frontendOpenOrders":     `[{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"}]`,
		"historicalOrders":       `[{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"0","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"},"status":"filled","statusTimestamp":1700000001000}]`,
		"orderStatus":            `{"status":"order","order":{"order":{"coin":"BTC","side":"B","limitPx":"100","sz":"1","origSz":"2","oid":7,"timestamp":1700000000000,"isTrigger":false,"reduceOnly":false,"orderType":"Limit","tif":"Gtc"},"status":"open","statusTimestamp":1700000001000}}`,
	})

	role, err := ex.GetUserRole(t.Context(), strings.ToUpper(officialSigningAddress))
	require.NoError(t, err, "GetUserRole must not error for a valid user role")
	assert.Equal(t, "user", role.Role, "role.Role: user role should be decoded")

	vault, err := ex.GetVaultDetails(t.Context(), testVaultAddress, officialSigningAddress)
	require.NoError(t, err, "GetVaultDetails must not error for valid vault details")
	assert.Equal(t, officialSigningAddress, vault.Leader, "vault.Leader: vault leader should be decoded")
	_, err = ex.GetVaultDetails(t.Context(), testVaultAddress, "")
	require.NoError(t, err, "GetVaultDetails must not error for vault details without optional user")

	spotState, err := ex.GetSpotClearinghouseState(t.Context(), officialSigningAddress)
	require.NoError(t, err, "GetSpotClearinghouseState must not error for valid spot state")
	require.Len(t, spotState.Balances, 1, "spotState.Balances: spot state must contain one balance")
	assert.Equal(t, 10.0, spotState.Balances[0].Total.Float64(), "Float64: spot balance should be decoded")

	perpetualState, err := ex.GetClearinghouseState(t.Context(), officialSigningAddress)
	require.NoError(t, err, "GetClearinghouseState must not error for valid perpetual state")
	assert.Equal(t, 12.0, perpetualState.MarginSummary.AccountValue.Float64(), "Float64: perpetual account value should be decoded")
	_, err = ex.GetClearinghouseStateForDEX(t.Context(), officialSigningAddress, "xyz")
	require.NoError(t, err, "GetClearinghouseStateForDEX must not error for named DEX perpetual state")

	openOrders, err := ex.GetOpenOrdersForUser(t.Context(), officialSigningAddress)
	require.NoError(t, err, "GetOpenOrdersForUser must not error for valid open orders")
	require.Len(t, openOrders, 1, "openOrders: open order response must be decoded")
	assert.Equal(t, uint64(7), openOrders[0].OrderID, "openOrders[0].OrderID: open order ID should be decoded")
	openOrders, err = ex.GetOpenOrdersForUserForDEX(t.Context(), officialSigningAddress, "xyz")
	require.NoError(t, err, "GetOpenOrdersForUserForDEX must not error for named DEX open orders")
	require.Len(t, openOrders, 1, "GetOpenOrdersForUserForDEX must decode one order")

	history, err := ex.GetHistoricalOrdersForUser(t.Context(), officialSigningAddress)
	require.NoError(t, err, "GetHistoricalOrdersForUser must not error for valid order history")
	require.Len(t, history, 1, "history: order history response must be decoded")
	assert.Equal(t, "filled", history[0].Status, "history[0].Status: historical order status should be decoded")

	status, err := ex.GetOrderStatusForUser(t.Context(), officialSigningAddress, uint64(7))
	require.NoError(t, err, "GetOrderStatusForUser must not error for order status by numeric ID")
	assert.Equal(t, "order", status.Status, "status.Status: order status response should be decoded")
	_, err = ex.GetOrderStatusForUser(t.Context(), officialSigningAddress, validClientOrderID)
	require.NoError(t, err, "GetOrderStatusForUser must not error for order status by client ID")

	for _, call := range []func() error{
		func() error { _, err := ex.GetUserRole(t.Context(), "invalid"); return err },
		func() error { _, err := ex.GetVaultDetails(t.Context(), "invalid", ""); return err },
		func() error { _, err := ex.GetVaultDetails(t.Context(), testVaultAddress, "invalid"); return err },
		func() error { _, err := ex.GetSpotClearinghouseState(t.Context(), "invalid"); return err },
		func() error { _, err := ex.GetClearinghouseState(t.Context(), "invalid"); return err },
		func() error { _, err := ex.GetOpenOrdersForUser(t.Context(), "invalid"); return err },
		func() error { _, err := ex.GetHistoricalOrdersForUser(t.Context(), "invalid"); return err },
		func() error { _, err := ex.GetOrderStatusForUser(t.Context(), "invalid", uint64(7)); return err },
	} {
		require.ErrorIs(t, call(), errInvalidAddress, "call must return the expected error for invalid address")
	}
	_, err = ex.GetOrderStatusForUser(t.Context(), officialSigningAddress, uint64(0))
	require.ErrorIs(t, err, order.ErrOrderIDNotSet, "GetOrderStatusForUser must return the expected error for zero order ID")
	_, err = ex.GetOrderStatusForUser(t.Context(), officialSigningAddress, "invalid")
	require.ErrorIs(t, err, errClientOrderIDInvalid, "GetOrderStatusForUser must return the expected error for invalid client order ID")
	_, err = ex.GetOrderStatusForUser(t.Context(), officialSigningAddress, int64(7))
	require.ErrorIs(t, err, order.ErrOrderIDNotSet, "GetOrderStatusForUser must return the expected error for unsupported order ID type")

	nullExchange := newStaticInfoExchange(t, map[string]string{
		testUserRoleInfoType:     `null`,
		"vaultDetails":           `null`,
		"spotClearinghouseState": `null`,
		"clearinghouseState":     `null`,
		"frontendOpenOrders":     `[]`,
		"historicalOrders":       `[]`,
		"orderStatus":            `null`,
	})
	for _, call := range []func() error{
		func() error { _, err := nullExchange.GetUserRole(t.Context(), officialSigningAddress); return err },
		func() error { _, err := nullExchange.GetVaultDetails(t.Context(), testVaultAddress, ""); return err },
		func() error {
			_, err := nullExchange.GetSpotClearinghouseState(t.Context(), officialSigningAddress)
			return err
		},
		func() error {
			_, err := nullExchange.GetClearinghouseState(t.Context(), officialSigningAddress)
			return err
		},
		func() error {
			_, err := nullExchange.GetOrderStatusForUser(t.Context(), officialSigningAddress, uint64(7))
			return err
		},
	} {
		require.ErrorIs(t, call(), common.ErrNilPointer, "call must return the expected error for null account response")
	}
	openOrders, err = nullExchange.GetOpenOrdersForUser(t.Context(), officialSigningAddress)
	require.NoError(t, err, "GetOpenOrdersForUser must not error for empty open-order response")
	assert.Empty(t, openOrders, "openOrders: empty open-order response should be retained")
	history, err = nullExchange.GetHistoricalOrdersForUser(t.Context(), officialSigningAddress)
	require.NoError(t, err, "GetHistoricalOrdersForUser must not error for empty historical-order response")
	assert.Empty(t, history, "history: empty historical-order response should be retained")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	for _, call := range []func() error{
		func() error { _, err := errorExchange.GetUserRole(t.Context(), officialSigningAddress); return err },
		func() error { _, err := errorExchange.GetVaultDetails(t.Context(), testVaultAddress, ""); return err },
		func() error {
			_, err := errorExchange.GetSpotClearinghouseState(t.Context(), officialSigningAddress)
			return err
		},
		func() error {
			_, err := errorExchange.GetClearinghouseState(t.Context(), officialSigningAddress)
			return err
		},
		func() error {
			_, err := errorExchange.GetOpenOrdersForUser(t.Context(), officialSigningAddress)
			return err
		},
		func() error {
			_, err := errorExchange.GetHistoricalOrdersForUser(t.Context(), officialSigningAddress)
			return err
		},
		func() error {
			_, err := errorExchange.GetOrderStatusForUser(t.Context(), officialSigningAddress, uint64(7))
			return err
		},
	} {
		require.Error(t, call(), "call must return account endpoint HTTP failure")
	}
}

func TestGetUserAbstraction(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode AccountAbstraction
	}{
		{name: "default", mode: AccountAbstractionDefault},
		{name: "disabled", mode: AccountAbstractionDisabled},
		{name: "legacy DEX abstraction", mode: AccountAbstractionDEX},
		{name: "unified", mode: AccountAbstractionUnified},
		{name: "portfolio margin", mode: AccountAbstractionPortfolio},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := newStaticInfoExchange(t, map[string]string{"userAbstraction": `"` + string(tc.mode) + `"`})
			result, err := ex.GetUserAbstraction(t.Context(), officialSigningAddress)
			require.NoError(t, err, "GetUserAbstraction must not error for a supported account abstraction mode")
			assert.Equal(t, tc.mode, result, "result: account abstraction mode should be decoded")
		})
	}

	ex := newStaticInfoExchange(t, map[string]string{"userAbstraction": `"futureMode"`})
	_, err := ex.GetUserAbstraction(t.Context(), "invalid")
	require.ErrorIs(t, err, errInvalidAddress, "GetUserAbstraction must return the expected error for invalid abstraction address")
	_, err = ex.GetUserAbstraction(t.Context(), officialSigningAddress)
	require.ErrorIs(t, err, errAccountAbstractionInvalid, "GetUserAbstraction must fail closed for unknown abstraction mode")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetUserAbstraction(t.Context(), officialSigningAddress)
	require.Error(t, err, "GetUserAbstraction must return account abstraction HTTP failure")
}

func TestDEXScopedAccountInfoRequests(t *testing.T) {
	var requests []infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for DEX-scoped account request") {
			return
		}
		requests = append(requests, request)
		response := `[]`
		if request.Type == "clearinghouseState" {
			response = `{"assetPositions":[]}`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Write should not error for DEX-scoped account response")
	}))
	_, err := ex.GetClearinghouseStateForDEX(t.Context(), officialSigningAddress, "xyz")
	require.NoError(t, err, "GetClearinghouseStateForDEX must not error for DEX-scoped clearinghouse state")
	_, err = ex.GetOpenOrdersForUserForDEX(t.Context(), officialSigningAddress, "xyz")
	require.NoError(t, err, "GetOpenOrdersForUserForDEX must not error for DEX-scoped open orders")
	require.Len(t, requests, 2, "requests: DEX-scoped account requests must contain two entries")
	assert.Equal(t, "xyz", requests[0].DEX, "requests[0].DEX: clearinghouse state should include its DEX")
	assert.Equal(t, "xyz", requests[1].DEX, "requests[1].DEX: open orders should include their DEX")
}
