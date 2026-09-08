package hyperliquid

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
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

func TestGetUserRole(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{testUserRoleInfoType: testUserRoleResponse})
	role, err := ex.GetUserRole(t.Context(), strings.ToUpper(officialSigningAddress))
	require.NoError(t, err, "GetUserRole must not error for a valid user role")
	assert.Equal(t, "user", role.Role, "role.Role should contain the user role")

	_, err = ex.GetUserRole(t.Context(), "invalid")
	require.ErrorIs(t, err, errInvalidAddress, "GetUserRole must return the expected error for an invalid address")

	nullExchange := newStaticInfoExchange(t, map[string]string{testUserRoleInfoType: `null`})
	_, err = nullExchange.GetUserRole(t.Context(), officialSigningAddress)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetUserRole must return the expected error for a null response")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetUserRole(t.Context(), officialSigningAddress)
	require.Error(t, err, "GetUserRole must return an HTTP failure")
}

func TestGetVaultDetails(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		"vaultDetails": `{"vaultAddress":"` + testVaultAddress + `","leader":"` + officialSigningAddress + `"}`,
	})
	vault, err := ex.GetVaultDetails(t.Context(), testVaultAddress, officialSigningAddress)
	require.NoError(t, err, "GetVaultDetails must not error for valid vault details")
	assert.Equal(t, officialSigningAddress, vault.Leader, "vault.Leader should contain the vault leader")
	_, err = ex.GetVaultDetails(t.Context(), testVaultAddress, "")
	require.NoError(t, err, "GetVaultDetails must not error for vault details without optional user")

	for _, tc := range []struct {
		name  string
		vault string
		user  string
	}{
		{name: "invalid vault", vault: "invalid"},
		{name: "invalid user", vault: testVaultAddress, user: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.GetVaultDetails(t.Context(), tc.vault, tc.user)
			require.ErrorIs(t, err, errInvalidAddress, "GetVaultDetails must return the expected error for an invalid address")
		})
	}

	nullExchange := newStaticInfoExchange(t, map[string]string{"vaultDetails": `null`})
	_, err = nullExchange.GetVaultDetails(t.Context(), testVaultAddress, "")
	require.ErrorIs(t, err, common.ErrNilPointer, "GetVaultDetails must return the expected error for a null response")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetVaultDetails(t.Context(), testVaultAddress, "")
	require.Error(t, err, "GetVaultDetails must return an HTTP failure")
}

func TestGetSpotClearinghouseState(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		"spotClearinghouseState": `{"balances":[{"coin":"USDC","token":0,"total":"10","hold":"2","entryNtl":"1"}]}`,
	})
	spotState, err := ex.GetSpotClearinghouseState(t.Context(), officialSigningAddress)
	require.NoError(t, err, "GetSpotClearinghouseState must not error for valid spot state")
	require.Len(t, spotState.Balances, 1, "spotState.Balances must contain one balance")
	assert.Equal(t, 10.0, spotState.Balances[0].Total.Float64(), "spotState.Balances[0].Total should contain the spot balance")

	_, err = ex.GetSpotClearinghouseState(t.Context(), "invalid")
	require.ErrorIs(t, err, errInvalidAddress, "GetSpotClearinghouseState must return the expected error for an invalid address")

	nullExchange := newStaticInfoExchange(t, map[string]string{"spotClearinghouseState": `null`})
	_, err = nullExchange.GetSpotClearinghouseState(t.Context(), officialSigningAddress)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetSpotClearinghouseState must return the expected error for a null response")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetSpotClearinghouseState(t.Context(), officialSigningAddress)
	require.Error(t, err, "GetSpotClearinghouseState must return an HTTP failure")
}

func TestGetClearinghouseState(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		"clearinghouseState": `{"marginSummary":{"accountValue":"12","totalMarginUsed":"3"},"withdrawable":"8","assetPositions":[]}`,
	})
	perpetualState, err := ex.GetClearinghouseState(t.Context(), officialSigningAddress)
	require.NoError(t, err, "GetClearinghouseState must not error for valid perpetual state")
	assert.Equal(t, 12.0, perpetualState.MarginSummary.AccountValue.Float64(), "perpetualState.MarginSummary.AccountValue should contain the perpetual account value")

	_, err = ex.GetClearinghouseState(t.Context(), "invalid")
	require.ErrorIs(t, err, errInvalidAddress, "GetClearinghouseState must return the expected error for an invalid address")

	nullExchange := newStaticInfoExchange(t, map[string]string{"clearinghouseState": `null`})
	_, err = nullExchange.GetClearinghouseState(t.Context(), officialSigningAddress)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetClearinghouseState must return the expected error for a null response")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetClearinghouseState(t.Context(), officialSigningAddress)
	require.Error(t, err, "GetClearinghouseState must return an HTTP failure")
}

func TestGetClearinghouseStateForDEX(t *testing.T) {
	ex := newStaticInfoExchange(t, map[string]string{
		"clearinghouseState": `{"marginSummary":{"accountValue":"12","totalMarginUsed":"3"},"withdrawable":"8","assetPositions":[]}`,
	})
	perpetualState, err := ex.GetClearinghouseStateForDEX(t.Context(), officialSigningAddress, "xyz")
	require.NoError(t, err, "GetClearinghouseStateForDEX must not error for named DEX perpetual state")
	assert.Equal(t, 12.0, perpetualState.MarginSummary.AccountValue.Float64(), "perpetualState.MarginSummary.AccountValue should contain the perpetual account value")

	var requests []infoRequest
	scoped := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for the DEX clearinghouse request") {
			return
		}
		requests = append(requests, request)
		_, err := w.Write([]byte(`{"assetPositions":[]}`))
		assert.NoError(t, err, "Write should not error for the DEX clearinghouse response")
	}))
	_, err = scoped.GetClearinghouseStateForDEX(t.Context(), officialSigningAddress, "xyz")
	require.NoError(t, err, "GetClearinghouseStateForDEX must not error for DEX-scoped clearinghouse state")
	require.Len(t, requests, 1, "requests must contain one DEX clearinghouse request")
	assert.Equal(t, "xyz", requests[0].DEX, "requests[0].DEX should contain the clearinghouse DEX")

	_, err = ex.GetClearinghouseStateForDEX(t.Context(), "invalid", "xyz")
	require.ErrorIs(t, err, errInvalidAddress, "GetClearinghouseStateForDEX must reject an invalid account address")
	nullExchange := newStaticInfoExchange(t, map[string]string{"clearinghouseState": `null`})
	_, err = nullExchange.GetClearinghouseStateForDEX(t.Context(), officialSigningAddress, "xyz")
	require.ErrorIs(t, err, common.ErrNilPointer, "GetClearinghouseStateForDEX must reject a null response")
	failed := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = failed.GetClearinghouseStateForDEX(t.Context(), officialSigningAddress, "xyz")
	require.Error(t, err, "GetClearinghouseStateForDEX must return an HTTP failure")
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
