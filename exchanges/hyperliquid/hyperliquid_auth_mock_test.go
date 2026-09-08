package hyperliquid

import (
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

const (
	officialSigningAddress = "0x14791697260e4c9a71f18484c9f997b308e59325"
	testVaultAddress       = "0x1719884eb866cb12b2287399b15f7db5e7d775ea"
	testOtherAddress       = "0x2222222222222222222222222222222222222222"
	validClientOrderID     = "0x00000000000000000000000000000001"
	testExchangeEndpoint   = "/exchange"
	testInfoEndpoint       = "/info"
	testUserRoleInfoType   = "userRole"
	testUserRoleResponse   = `{"role":"user"}`
)

func setTestCredentials(ex *Exchange, credentials *accounts.Credentials) {
	ex.API.AuthenticatedSupport = true
	ex.API.AuthenticatedWebsocketSupport = true
	ex.SetCredentials(credentials)
}

func setCachedTestAuthority(t *testing.T, ex *Exchange, credentials *accounts.Credentials) {
	t.Helper()
	setTestCredentials(ex, credentials)
	accountAddress, _, err := normaliseAddress(credentials.Key)
	require.NoError(t, err, "normaliseAddress must not error for the cached test account")
	vaultAddress := ""
	if credentials.SubAccount != "" {
		vaultAddress, _, err = normaliseAddress(credentials.SubAccount)
		require.NoError(t, err, "normaliseAddress must not error for the cached test subaccount")
	}
	signerAddress := ""
	if credentials.Secret != "" {
		privateKey, err := parsePrivateKey(credentials.Secret)
		require.NoError(t, err, "parsePrivateKey must not error for the cached test signer")
		signerAddress = privateKeyAddress(privateKey)
		privateKey.Zero()
	}
	ex.authorityValidationMu.Lock()
	ex.authorityValidationKey = authorityValidationKey{
		accountAddress: accountAddress,
		vaultAddress:   vaultAddress,
		signerAddress:  signerAddress,
		mainnet:        ex.isMainnetEnvironment(),
	}
	ex.authorityValidated = true
	ex.authorityValidationMu.Unlock()
}

func TestValidateClientOrderID(t *testing.T) {
	for _, clientOrderID := range []string{
		validClientOrderID,
		strings.ToUpper(validClientOrderID),
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		require.NoError(t, validateClientOrderID(clientOrderID), "validateClientOrderID must not error for valid client order ID")
	}
	for _, clientOrderID := range []string{"", "0x01", "0000000000000000000000000000000001", "0x0000000000000000000000000000000z"} {
		require.ErrorIs(t, validateClientOrderID(clientOrderID), errClientOrderIDInvalid, "validateClientOrderID must return the expected error for invalid client order ID")
	}
}

func TestGetWatchAddress(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()

	_, err := ex.getWatchAddress(t.Context())
	require.Error(t, err, "getWatchAddress must error without credentials")

	setTestCredentials(ex, &accounts.Credentials{Key: strings.ToUpper(officialSigningAddress)})
	address, err := ex.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error for a valid account watch address")
	assert.Equal(t, officialSigningAddress, address, "address: account watch address should be normalised")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, SubAccount: strings.ToUpper(testVaultAddress)})
	address, err = ex.getWatchAddress(t.Context())
	require.NoError(t, err, "getWatchAddress must not error for a valid subaccount watch address")
	assert.Equal(t, testVaultAddress, address, "address: subaccount watch address should be normalised")

	setTestCredentials(ex, &accounts.Credentials{Key: "invalid"})
	_, err = ex.getWatchAddress(t.Context())
	require.ErrorIs(t, err, errInvalidAddress, "getWatchAddress must return the expected error for invalid account address")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, SubAccount: "invalid"})
	_, err = ex.getWatchAddress(t.Context())
	require.ErrorIs(t, err, errInvalidAddress, "getWatchAddress must return the expected error for invalid subaccount address")
}

func TestGetSigningCredentials(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()

	_, _, err := ex.getSigningCredentials(t.Context())
	require.Error(t, err, "getSigningCredentials must error without credentials")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress})
	_, _, err = ex.getSigningCredentials(t.Context())
	require.ErrorIs(t, err, errPrivateKeyRequired, "getSigningCredentials must return the expected error for missing private key")
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "getSigningCredentials must classify missing private key as an authentication failure")

	setTestCredentials(ex, &accounts.Credentials{Key: "invalid", Secret: officialSigningTestKey})
	_, _, err = ex.getSigningCredentials(t.Context())
	require.ErrorIs(t, err, errInvalidAddress, "getSigningCredentials must return the expected error for invalid account address")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	credentials, vault, err := ex.getSigningCredentials(t.Context())
	require.NoError(t, err, "getSigningCredentials must not error for valid main-account signing credentials")
	assert.Equal(t, officialSigningTestKey, credentials.Secret, "credentials.Secret should contain the signing secret")
	assert.Empty(t, vault, "vault should be empty for main-account signing")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: strings.ToUpper(testVaultAddress)})
	_, vault, err = ex.getSigningCredentials(t.Context())
	require.NoError(t, err, "getSigningCredentials must not error for valid vault signing credentials")
	assert.Equal(t, testVaultAddress, vault, "vault should contain the normalised signing address")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: "invalid"})
	_, _, err = ex.getSigningCredentials(t.Context())
	require.ErrorIs(t, err, errInvalidAddress, "getSigningCredentials must return the expected error for invalid vault address")
}

func TestGetUserSigningCredentials(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, _, err := ex.getUserSigningCredentials(t.Context())
	require.Error(t, err, "getUserSigningCredentials must error without credentials")

	setTestCredentials(ex, &accounts.Credentials{
		Key:        strings.ToUpper(officialSigningAddress),
		Secret:     officialSigningTestKey,
		SubAccount: strings.ToUpper(testVaultAddress),
	})
	credentials, subAccount, err := ex.getUserSigningCredentials(t.Context())
	require.NoError(t, err, "getUserSigningCredentials must not error for matching master-key credentials")
	assert.Equal(t, officialSigningTestKey, credentials.Secret, "credentials.Secret should contain the master private key")
	assert.Equal(t, testVaultAddress, subAccount, "subAccount should contain the normalised configured subaccount")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: "invalid"})
	_, _, err = ex.getUserSigningCredentials(t.Context())
	require.ErrorIs(t, err, errInvalidPrivateKey, "getUserSigningCredentials must return the expected error for invalid user-signing key")

	setTestCredentials(ex, &accounts.Credentials{
		Key:    officialSigningAddress,
		Secret: "0x1123456789012345678901234567890123456789012345678901234567890123",
	})
	_, _, err = ex.getUserSigningCredentials(t.Context())
	require.ErrorIs(t, err, errUserSignedMasterRequired, "getUserSigningCredentials must reject API-wallet signing key for user-signed actions")
}

func TestValidateCachedAuthority(t *testing.T) {
	_, err := (*Exchange)(nil).validateCachedAuthority(t.Context(), &accounts.Credentials{}, false)
	require.ErrorIs(t, err, common.ErrNilPointer, "validateCachedAuthority must return the expected error for nil exchange authority validation")

	ex := new(Exchange)
	ex.SetDefaults()
	_, err = ex.validateCachedAuthority(t.Context(), nil, false)
	require.ErrorIs(t, err, common.ErrNilPointer, "validateCachedAuthority must return the expected error for nil authority credentials")
	_, err = ex.validateCachedAuthority(t.Context(), &accounts.Credentials{Key: "invalid"}, false)
	require.ErrorIs(t, err, errInvalidAddress, "validateCachedAuthority must return the expected error for invalid cached account address")
	_, err = ex.validateCachedAuthority(t.Context(), &accounts.Credentials{Key: officialSigningAddress, SubAccount: "invalid"}, false)
	require.ErrorIs(t, err, errInvalidAddress, "validateCachedAuthority must return the expected error for invalid cached subaccount address")
	_, err = ex.validateCachedAuthority(t.Context(), &accounts.Credentials{Key: officialSigningAddress, Secret: "invalid"}, false)
	require.ErrorIs(t, err, errInvalidPrivateKey, "validateCachedAuthority must return the expected error for invalid cached signer")

	var roleCalls atomic.Int32
	valid := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload), "Decode should not error for a cached role request") {
			return
		}
		assert.Equal(t, testUserRoleInfoType, payload.Type, "payload.Type: cached authority validation should request the account role")
		assert.Equal(t, officialSigningAddress, payload.User, "payload.User: cached authority validation should request the configured account")
		roleCalls.Add(1)
		_, writeErr := w.Write([]byte(testUserRoleResponse))
		assert.NoError(t, writeErr, "Write should not error for a cached role response")
	}))
	credentials := &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey}
	setTestCredentials(valid, credentials)
	validationKey, err := valid.validateCachedAuthority(t.Context(), credentials, false)
	require.NoError(t, err, "validateCachedAuthority must not error for a fresh authority tuple")
	assert.Equal(t, officialSigningAddress, validationKey.accountAddress, "validationKey.accountAddress: cached authority account should match")
	assert.Equal(t, officialSigningAddress, validationKey.signerAddress, "validationKey.signerAddress: cached authority signer should match")
	assert.Empty(t, validationKey.vaultAddress, "validationKey.vaultAddress: main-account authority should not cache a vault")
	assert.True(t, validationKey.mainnet, "validationKey.mainnet: default authority should use the mainnet environment")
	assert.Equal(t, int32(1), roleCalls.Load(), "roleCalls: fresh authority validation should perform one role lookup")

	cachedKey, err := valid.validateCachedAuthority(t.Context(), credentials, false)
	require.NoError(t, err, "validateCachedAuthority must not error for an unchanged authority tuple")
	assert.Equal(t, validationKey, cachedKey, "cachedKey: cached authority tuple should remain unchanged")
	assert.Equal(t, int32(1), roleCalls.Load(), "roleCalls: cached authority validation should not repeat the role lookup")

	_, err = valid.validateCachedAuthority(t.Context(), credentials, true)
	require.NoError(t, err, "validateCachedAuthority must not error when forcing authority revalidation")
	assert.Equal(t, int32(2), roleCalls.Load(), "roleCalls: forced authority validation should repeat the role lookup")
}

func TestSendSignedAction(t *testing.T) {
	action := cancelAction{Type: "cancel", Cancels: []cancelWire{{AssetID: 1, OrderID: 2}}}
	var response exchangeActionResponse

	require.ErrorIs(t, (*Exchange)(nil).sendSignedAction(t.Context(), action, 1, &response), common.ErrNilPointer, "sendSignedAction must return the expected error for nil exchange")
	ex := new(Exchange)
	ex.SetDefaults()
	require.ErrorIs(t, ex.sendSignedAction(t.Context(), action, 1, nil), common.ErrNilPointer, "sendSignedAction must return the expected error for nil response")
	ex.Requester = nil
	require.ErrorIs(t, ex.sendSignedAction(t.Context(), action, 1, &response), common.ErrNilPointer, "sendSignedAction must return the expected error for nil requester")

	ex.SetDefaults()
	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	err := ex.sendSignedAction(t.Context(), action, maximumActionBatchSize+1, &response)
	require.ErrorIs(t, err, errActionBatchTooLarge, "sendSignedAction must return the expected error for oversized action batch")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress})
	err = ex.sendSignedAction(t.Context(), action, 1, &response)
	require.ErrorIs(t, err, errPrivateKeyRequired, "sendSignedAction must return the expected error for action without a private key")
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "sendSignedAction must classify action without a private key as an authentication failure")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: "invalid"})
	err = ex.sendSignedAction(t.Context(), action, 1, &response)
	require.ErrorIs(t, err, errInvalidPrivateKey, "sendSignedAction must return the expected error for action with an invalid private key")

	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	err = ex.sendSignedAction(t.Context(), make(chan int), 1, &response)
	require.Error(t, err, "sendSignedAction must error for action with an unsupported signing payload")

	ex.API.Endpoints = ex.NewEndpoints()
	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	err = ex.sendSignedAction(t.Context(), action, 1, &response)
	require.Error(t, err, "sendSignedAction must error for action without a configured endpoint")

	var captured signedActionRequest
	successExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testInfoEndpoint:
			var request infoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for a signed-action authority request") {
				return
			}
			response := testUserRoleResponse
			if request.User == testVaultAddress {
				response = `{"role":"subAccount","data":{"master":"` + officialSigningAddress + `"}}`
			}
			_, writeErr := w.Write([]byte(response))
			assert.NoError(t, writeErr, "Write should not error for a signed-action authority response")
		case testExchangeEndpoint:
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&captured), "Decode should not error for a signed action request") {
				return
			}
			_, writeErr := w.Write([]byte(`{"status":"ok","response":{"type":"cancel","data":{"statuses":["success"]}}}`))
			assert.NoError(t, writeErr, "Write should not error for a signed action response")
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	setTestCredentials(successExchange, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: testVaultAddress})
	err = successExchange.sendSignedAction(t.Context(), action, 1, &response)
	require.NoError(t, err, "sendSignedAction must not error for a successful signed action")
	assert.Equal(t, testVaultAddress, captured.VaultAddress, "captured.VaultAddress should contain the configured vault")
	assert.NotZero(t, captured.Nonce, "captured.Nonce should be set for the signed action")
	assert.NotEmpty(t, captured.Signature.R, "captured.Signature.R should be set for the signed action")
	assert.NotEmpty(t, captured.Signature.S, "captured.Signature.S should be set for the signed action")
	assert.Contains(t, []uint8{27, 28}, captured.Signature.V, "captured.Signature.V should contain a valid recovery ID")

	agentSecret := "0x1123456789012345678901234567890123456789012345678901234567890123"
	agentKey, err := parsePrivateKey(agentSecret)
	require.NoError(t, err, "parsePrivateKey must not error for unauthorised API-wallet test key")
	agentAddress := privateKeyAddress(agentKey)
	agentKey.Zero()
	var unauthorisedExchangeCalls atomic.Int32
	unauthorisedSigner := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testInfoEndpoint:
			var request infoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for an unauthorised signer role request") {
				return
			}
			response := testUserRoleResponse
			if request.User == agentAddress {
				response = `{"role":"agent","data":{"user":"` + testOtherAddress + `"}}`
			}
			_, writeErr := w.Write([]byte(response))
			assert.NoError(t, writeErr, "Write should not error for an unauthorised signer role response")
		case testExchangeEndpoint:
			unauthorisedExchangeCalls.Add(1)
			http.Error(w, "must not be reached", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	setTestCredentials(unauthorisedSigner, &accounts.Credentials{Key: officialSigningAddress, Secret: agentSecret})
	err = unauthorisedSigner.sendSignedAction(t.Context(), action, 1, &response)
	require.ErrorIs(t, err, errSignerNotAuthorised, "sendSignedAction must fail authority validation for unapproved API-wallet action")
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "sendSignedAction must classify unapproved API-wallet action as an authentication failure")
	assert.Zero(t, unauthorisedExchangeCalls.Load(), "unauthorisedExchangeCalls: unapproved API-wallet action should not reach the exchange endpoint")
	assert.Zero(t, unauthorisedSigner.lastNonce.Load(), "unauthorisedSigner.lastNonce: unapproved API-wallet action should not consume a nonce")

	var unauthorisedVaultExchangeCalls atomic.Int32
	unauthorisedVault := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testInfoEndpoint:
			var request infoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for an unauthorised vault role request") {
				return
			}
			response := testUserRoleResponse
			if request.User == testVaultAddress {
				response = `{"role":"subAccount","data":{"master":"` + testOtherAddress + `"}}`
			}
			_, writeErr := w.Write([]byte(response))
			assert.NoError(t, writeErr, "Write should not error for an unauthorised vault role response")
		case testExchangeEndpoint:
			unauthorisedVaultExchangeCalls.Add(1)
			http.Error(w, "must not be reached", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	setTestCredentials(unauthorisedVault, &accounts.Credentials{
		Key:        officialSigningAddress,
		Secret:     officialSigningTestKey,
		SubAccount: testVaultAddress,
	})
	err = unauthorisedVault.sendSignedAction(t.Context(), action, 1, &response)
	require.ErrorIs(t, err, errVaultNotAuthorised, "sendSignedAction must fail authority validation for unowned vault action")
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "sendSignedAction must classify unowned vault action as an authentication failure")
	assert.Zero(t, unauthorisedVaultExchangeCalls.Load(), "unauthorisedVaultExchangeCalls: unowned vault action should not reach the exchange endpoint")
	assert.Zero(t, unauthorisedVault.lastNonce.Load(), "unauthorisedVault.lastNonce: unowned vault action should not consume a nonce")

	jsonFailureExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testInfoEndpoint {
			t.Error("sendSignedAction should fail JSON encoding before an exchange request")
			return
		}
		_, writeErr := w.Write([]byte(testUserRoleResponse))
		assert.NoError(t, writeErr, "Write should not error for a JSON-failure authority response")
	}))
	setTestCredentials(jsonFailureExchange, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	err = jsonFailureExchange.sendSignedAction(t.Context(), struct {
		Type  string  `json:"type"  msgpack:"type"`
		Value float64 `json:"value" msgpack:"value"`
	}{Type: "test", Value: math.Inf(1)}, 1, &response)
	require.Error(t, err, "sendSignedAction must error for JSON-unsupported signed action")
	err = jsonFailureExchange.sendSignedAction(t.Context(), make(chan int), 1, &response)
	require.Error(t, err, "sendSignedAction must error after authority validation for msgpack-unsupported signed action")

	for _, tc := range []struct {
		name        string
		replacement *accounts.Credentials
		expectedIs  error
	}{
		{name: "credentials removed", expectedIs: request.ErrAuthRequestFailed},
		{
			name:        "credentials changed",
			replacement: &accounts.Credentials{Key: testOtherAddress, Secret: officialSigningTestKey},
			expectedIs:  errCredentialsChanged,
		},
	} {
		t.Run(tc.name+" during authority validation", func(t *testing.T) {
			var (
				mutatingExchange *Exchange
				exchangeCalls    atomic.Int32
			)
			mutatingExchange = newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case testInfoEndpoint:
					setTestCredentials(mutatingExchange, tc.replacement)
					_, writeErr := w.Write([]byte(testUserRoleResponse))
					assert.NoError(t, writeErr, "Write should not error for a mutating authority response")
				case testExchangeEndpoint:
					exchangeCalls.Add(1)
					http.Error(w, "must not be reached", http.StatusInternalServerError)
				default:
					http.Error(w, "unexpected path", http.StatusNotFound)
				}
			}))
			setTestCredentials(mutatingExchange, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
			var result exchangeActionResponse
			err := mutatingExchange.sendSignedAction(t.Context(), action, 1, &result)
			require.ErrorIs(t, err, tc.expectedIs, "sendSignedAction must return the expected error for credentials mutated during validation")
			require.ErrorIs(t, err, request.ErrAuthRequestFailed, "sendSignedAction must classify credentials mutated during validation as an authentication failure")
			assert.Zero(t, exchangeCalls.Load(), "exchangeCalls: credentials mutated during validation should not reach the exchange endpoint")
			assert.Zero(t, mutatingExchange.lastNonce.Load(), "mutatingExchange.lastNonce: credentials mutated during validation should not consume a nonce")
		})
	}

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(errorExchange, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
	err = errorExchange.sendSignedAction(t.Context(), action, 1, &response)
	require.Error(t, err, "sendSignedAction must return HTTP failure")

	for _, tc := range []struct {
		name     string
		response string
		contains string
	}{
		{name: "message", response: `{"status":"err","response":"bad action"}`, contains: "bad action"},
		{name: "escaped message", response: `{"status":"err","response":"bad \"size\""}`, contains: `bad "size"`},
		{name: "structured response", response: `{"status":"err","response":{"error":"bad action"}}`, contains: `"error":"bad action"`},
		{name: "null response", response: `{"status":"err","response":null}`, contains: "err"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actionExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == testInfoEndpoint {
					_, writeErr := w.Write([]byte(testUserRoleResponse))
					assert.NoError(t, writeErr, "Write should not error for an action-error authority response")
					return
				}
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "Write should not error for an action error response")
			}))
			setTestCredentials(actionExchange, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey})
			var result exchangeActionResponse
			err := actionExchange.sendSignedAction(t.Context(), action, 1, &result)
			require.ErrorIs(t, err, errActionResponse, "sendSignedAction must return the expected error for failed action")
			assert.ErrorContains(t, err, tc.contains, "sendSignedAction should include the server message for action error")
		})
	}
}

func TestSendSignedActionAuthorityCache(t *testing.T) {
	action := cancelAction{Type: "cancel", Cancels: []cancelWire{{AssetID: 1, OrderID: 2}}}
	agentSecret := "0x1123456789012345678901234567890123456789012345678901234567890123"
	agentKey, err := parsePrivateKey(agentSecret)
	require.NoError(t, err, "parsePrivateKey must not error for API-wallet test key")
	agentAddress := privateKeyAddress(agentKey)
	agentKey.Zero()

	var (
		infoCalls     atomic.Int32
		exchangeCalls atomic.Int32
		failAction    atomic.Bool
	)
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testInfoEndpoint:
			infoCalls.Add(1)
			var request infoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for an authority request") {
				return
			}
			response := testUserRoleResponse
			if request.User == agentAddress {
				response = `{"role":"agent","data":{"user":"` + officialSigningAddress + `"}}`
			}
			_, writeErr := w.Write([]byte(response))
			assert.NoError(t, writeErr, "Write should not error for an authority response")
		case testExchangeEndpoint:
			exchangeCalls.Add(1)
			response := `{"status":"ok","response":{"type":"cancel","data":{"statuses":["success"]}}}`
			if failAction.Swap(false) {
				response = `{"status":"err","response":"signer is no longer authorised"}`
			}
			_, writeErr := w.Write([]byte(response))
			assert.NoError(t, writeErr, "Write should not error for a signed-action response")
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	setTestCredentials(ex, &accounts.Credentials{Key: officialSigningAddress, Secret: agentSecret})

	var response exchangeActionResponse
	require.NoError(t, ex.sendSignedAction(t.Context(), action, 1, &response), "sendSignedAction must validate and succeed for the first API-wallet action")
	assert.Equal(t, int32(2), infoCalls.Load(), "infoCalls: the first API-wallet action should validate the account and signer")
	require.NoError(t, ex.sendSignedAction(t.Context(), action, 1, &response), "sendSignedAction must succeed for a cached API-wallet action")
	assert.Equal(t, int32(2), infoCalls.Load(), "infoCalls: an unchanged authority tuple should not be revalidated")

	require.NoError(t, ex.ValidateAPICredentials(t.Context(), asset.Empty), "ValidateAPICredentials must succeed for explicit credential validation")
	assert.Equal(t, int32(4), infoCalls.Load(), "infoCalls: explicit credential validation should force a fresh account and signer check")
	require.NoError(t, ex.sendSignedAction(t.Context(), action, 1, &response), "sendSignedAction must succeed for an action after explicit validation")
	assert.Equal(t, int32(4), infoCalls.Load(), "infoCalls: a successful explicit validation should refresh the cached authority tuple")

	failAction.Store(true)
	require.ErrorIs(t, ex.sendSignedAction(t.Context(), action, 1, &response), errActionResponse, "sendSignedAction must return an exchange action failure")
	assert.Equal(t, int32(4), infoCalls.Load(), "infoCalls: the failed action should use the previously validated authority tuple")
	require.NoError(t, ex.sendSignedAction(t.Context(), action, 1, &response), "sendSignedAction must revalidate and succeed for an action after a server rejection")
	assert.Equal(t, int32(6), infoCalls.Load(), "infoCalls: a server rejection should invalidate the cached authority tuple")
	assert.Equal(t, int32(5), exchangeCalls.Load(), "exchangeCalls: every locally authorised action should reach the exchange endpoint")
}

func TestSendSignedActionRejectsAPIWalletAccount(t *testing.T) {
	action := cancelAction{Type: "cancel", Cancels: []cancelWire{{AssetID: 1, OrderID: 2}}}
	agentSecret := "0x1123456789012345678901234567890123456789012345678901234567890123"
	agentKey, err := parsePrivateKey(agentSecret)
	require.NoError(t, err, "parsePrivateKey must not error for API-wallet test key")
	agentAddress := privateKeyAddress(agentKey)
	agentKey.Zero()

	var exchangeCalls atomic.Int32
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testInfoEndpoint:
			_, writeErr := w.Write([]byte(`{"role":"agent","data":{"user":"` + officialSigningAddress + `"}}`))
			assert.NoError(t, writeErr, "Write should not error for an API-wallet role response")
		case testExchangeEndpoint:
			exchangeCalls.Add(1)
			_, writeErr := w.Write([]byte(`{"status":"ok","response":{"type":"cancel","data":{"statuses":["success"]}}}`))
			assert.NoError(t, writeErr, "Write should not error for a signed-action response")
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	setTestCredentials(ex, &accounts.Credentials{Key: agentAddress, Secret: agentSecret})

	var response exchangeActionResponse
	err = ex.sendSignedAction(t.Context(), action, 1, &response)
	require.ErrorIs(t, err, errConfiguredAccountMissing, "sendSignedAction must fail master-account validation for an API wallet configured as the account")
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "sendSignedAction must classify an API wallet configured as the account as an authentication failure")
	assert.Zero(t, exchangeCalls.Load(), "exchangeCalls: a misconfigured API-wallet account should not reach the exchange endpoint")
	assert.Zero(t, ex.lastNonce.Load(), "ex.lastNonce: a misconfigured API-wallet account should not consume a nonce")
}

func TestSendUserSignedAction(t *testing.T) {
	credentials := &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey}
	fields := []eip712Field{
		{Name: "destination", Type: "string", Value: testOtherAddress},
		{Name: "amount", Type: "string", Value: "1"},
	}
	var response exchangeActionResponse

	_, err := (*Exchange)(nil).sendUserSignedAction(
		t.Context(), credentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, &response,
	)
	require.ErrorIs(t, err,
		common.ErrNilPointer,
		"sendUserSignedAction must return the expected error for nil exchange")
	ex := new(Exchange)
	ex.SetDefaults()
	_, err = ex.sendUserSignedAction(
		t.Context(), credentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, nil,
	)
	require.ErrorIs(t, err,
		common.ErrNilPointer,
		"sendUserSignedAction must return the expected error for nil response")
	_, err = ex.sendUserSignedAction(
		t.Context(), nil, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, &response,
	)
	require.ErrorIs(t, err,
		common.ErrNilPointer,
		"sendUserSignedAction must return the expected error for nil credentials")
	ex.Requester = nil
	_, err = ex.sendUserSignedAction(
		t.Context(), credentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, &response,
	)
	require.ErrorIs(t, err,
		common.ErrNilPointer,
		"sendUserSignedAction must return the expected error for nil requester")

	ex.SetDefaults()
	for _, tc := range []struct {
		name        string
		actionType  string
		primaryType string
		nonceField  string
	}{
		{name: "blank action type", primaryType: "HyperliquidTransaction:UsdSend", nonceField: "time"},
		{name: "blank primary type", actionType: "usdSend", nonceField: "time"},
		{name: "invalid nonce field", actionType: "usdSend", primaryType: "HyperliquidTransaction:UsdSend", nonceField: "id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ex.sendUserSignedAction(
				t.Context(), credentials, tc.actionType, tc.primaryType, tc.nonceField, fields, &response,
			)
			require.ErrorIs(t, err, errUserSignedActionInvalid, "sendUserSignedAction must return the expected error for malformed action metadata")
		})
	}

	ex.API.Endpoints = ex.NewEndpoints()
	_, err = ex.sendUserSignedAction(
		t.Context(), credentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, &response,
	)
	require.Error(t, err, "sendUserSignedAction must error for user-signed action without a configured endpoint")

	ex.SetDefaults()
	setCachedTestAuthority(t, ex, credentials)
	staleCredentials := *credentials
	staleCredentials.ClientID = "changed"
	nonceBeforeStaleCredentials := ex.lastNonce.Load()
	_, err = ex.sendUserSignedAction(
		t.Context(), &staleCredentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, &response,
	)
	require.ErrorIs(t, err, errCredentialsChanged, "sendUserSignedAction must return the expected error for stale user-signing credentials")
	assert.Equal(t, nonceBeforeStaleCredentials, ex.lastNonce.Load(), "ex.lastNonce: stale user-signing credentials should not consume a nonce")

	for _, invalidFields := range [][]eip712Field{
		{{Name: "", Type: "string", Value: "value"}},
		{{Name: "type", Type: "string", Value: "value"}},
		{{Name: "amount", Type: "string", Value: "1"}, {Name: "amount", Type: "string", Value: "2"}},
	} {
		_, err := ex.sendUserSignedAction(
			t.Context(), credentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", invalidFields, &response,
		)
		require.ErrorIs(t, err, errUserSignedActionInvalid, "sendUserSignedAction must return the expected error for reserved or duplicate action fields")
	}
	_, err = ex.sendUserSignedAction(
		t.Context(),
		credentials,
		"usdSend",
		"HyperliquidTransaction:UsdSend",
		"time",
		[]eip712Field{{Name: "destination", Type: "address", Value: testOtherAddress}},
		&response,
	)
	require.ErrorIs(t, err, errEIP712Field, "sendUserSignedAction must return the expected error for unsupported EIP-712 field type")
	invalidCredentials := &accounts.Credentials{Key: officialSigningAddress, Secret: "invalid"}
	setTestCredentials(ex, invalidCredentials)
	ex.authorityValidationMu.Lock()
	ex.authorityValidated = false
	ex.authorityValidationMu.Unlock()
	_, err = ex.sendUserSignedAction(
		t.Context(),
		invalidCredentials,
		"usdSend",
		"HyperliquidTransaction:UsdSend",
		"time",
		fields,
		&response,
	)
	require.ErrorIs(t, err, errInvalidPrivateKey, "sendUserSignedAction must return the expected error for invalid user-signing key")

	var captured struct {
		Action struct {
			Type             string `json:"type"`
			SignatureChainID string `json:"signatureChainId"`
			HyperliquidChain string `json:"hyperliquidChain"`
			Destination      string `json:"destination"`
			Amount           string `json:"amount"`
			Time             uint64 `json:"time"`
		} `json:"action"`
		Nonce        uint64      `json:"nonce"`
		Signature    l1Signature `json:"signature"`
		VaultAddress string      `json:"vaultAddress"`
	}
	successExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, testExchangeEndpoint, r.URL.Path, "r.URL.Path: user-signed action should use the exchange endpoint")
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&captured), "Decode should not error for a user-signed action request") {
			return
		}
		_, writeErr := w.Write([]byte(`{"status":"ok","response":null}`))
		assert.NoError(t, writeErr, "Write should not error for a successful user action response")
	}))
	successExchange.Config.UseSandbox = true
	setCachedTestAuthority(t, successExchange, credentials)
	nonce, err := successExchange.sendUserSignedAction(
		t.Context(), credentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, &response,
	)
	require.NoError(t, err, "sendUserSignedAction must not error for a successful user-signed action")
	assert.True(t, successExchange.authorityValidated, "successExchange.authorityValidated: successful user-signed action should retain cached authority")
	assert.Equal(t, nonce, captured.Nonce, "captured.Nonce should match the returned nonce")
	assert.Equal(t, nonce, captured.Action.Time, "captured.Action.Time should match the outer request nonce")
	assert.Equal(t, "usdSend", captured.Action.Type, "captured.Action.Type should retain the action type")
	assert.Equal(t, userSignedChainIDHex, captured.Action.SignatureChainID, "captured.Action.SignatureChainID should use the SDK value")
	assert.Equal(t, "Testnet", captured.Action.HyperliquidChain, "captured.Action.HyperliquidChain should use the testnet domain for the sandbox action")
	assert.Equal(t, testOtherAddress, captured.Action.Destination, "captured.Action.Destination should retain the destination")
	assert.Equal(t, "1", captured.Action.Amount, "captured.Action.Amount should retain the amount")
	assert.Empty(t, captured.VaultAddress, "captured.VaultAddress should be omitted from a user-signed action")
	assert.NotEmpty(t, captured.Signature.R, "captured.Signature.R should be set for the user-signed action")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setCachedTestAuthority(t, errorExchange, credentials)
	_, err = errorExchange.sendUserSignedAction(
		t.Context(), credentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, &response,
	)
	require.Error(t, err, "sendUserSignedAction must return user-signed action HTTP failure")
	assert.False(t, errorExchange.authorityValidated, "errorExchange.authorityValidated: user-signed HTTP failure should invalidate cached authority")

	for _, tc := range []struct {
		name     string
		response string
		contains string
	}{
		{name: "message", response: `{"status":"err","response":"bad action"}`, contains: "bad action"},
		{name: "structured response", response: `{"status":"err","response":{"error":"bad action"}}`, contains: `"error":"bad action"`},
		{name: "null response", response: `{"status":"err","response":null}`, contains: "err"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actionExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, writeErr := w.Write([]byte(tc.response))
				assert.NoError(t, writeErr, "Write should not error for a user action error response")
			}))
			setCachedTestAuthority(t, actionExchange, credentials)
			_, err := actionExchange.sendUserSignedAction(
				t.Context(), credentials, "usdSend", "HyperliquidTransaction:UsdSend", "time", fields, &response,
			)
			require.ErrorIs(t, err, errActionResponse, "sendUserSignedAction must return the expected error for failed user action")
			assert.ErrorContains(t, err, tc.contains, "sendUserSignedAction should include the server message for user action error")
			assert.False(t, actionExchange.authorityValidated, "actionExchange.authorityValidated: rejected user action should invalidate cached authority")
		})
	}
}

func TestIsMainnetEnvironment(t *testing.T) {
	mainnet := new(Exchange)
	mainnet.Config = new(config.Exchange)
	sandbox := new(Exchange)
	sandbox.Config = &config.Exchange{UseSandbox: true}
	for _, tc := range []struct {
		name     string
		exchange *Exchange
		expected bool
	}{
		{name: "nil exchange", expected: true},
		{name: "without config", exchange: new(Exchange), expected: true},
		{name: "mainnet", exchange: mainnet, expected: true},
		{name: "sandbox", exchange: sandbox},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.exchange.isMainnetEnvironment(), "isMainnetEnvironment: signing environment should match explicit sandbox configuration")
		})
	}
}

func newRoleTestExchange(t *testing.T, credentials *accounts.Credentials, roles map[string]string, vaultDetails string) *Exchange {
	t.Helper()
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request infoRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for role request") {
			return
		}
		var response string
		switch request.Type {
		case testUserRoleInfoType:
			response = roles[request.User]
		case "vaultDetails":
			response = vaultDetails
		default:
			t.Errorf("request.Type should identify an expected role validation request: %q", request.Type)
		}
		if response == "" {
			response = `{"role":"missing"}`
		}
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "Write should not error for role response")
	}))
	setTestCredentials(ex, credentials)
	return ex
}

func TestValidateCredentials(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	require.Error(t, ex.validateCredentials(t.Context()), "validateCredentials must error for absent credentials")

	setTestCredentials(ex, &accounts.Credentials{Key: "invalid"})
	require.ErrorIs(t, ex.validateCredentials(t.Context()), errInvalidAddress, "validateCredentials must return the expected error for invalid account address")

	missing := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress}, nil, "")
	require.ErrorIs(t, missing.validateCredentials(t.Context()), errConfiguredAccountMissing, "validateCredentials must return the expected error for missing account")

	agentAccount := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress}, map[string]string{
		officialSigningAddress: `{"role":"agent","data":{"user":"` + testOtherAddress + `"}}`,
	}, "")
	require.ErrorIs(t, agentAccount.validateCredentials(t.Context()), errConfiguredAccountMissing, "validateCredentials must return the expected error for non-user account role")

	userRoles := map[string]string{officialSigningAddress: testUserRoleResponse}
	keyOnly := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress}, userRoles, "")
	require.NoError(t, keyOnly.validateCredentials(t.Context()), "validateCredentials must accept valid key-only monitoring credentials")

	masterSigner := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, Secret: officialSigningTestKey}, userRoles, "")
	require.NoError(t, masterSigner.validateCredentials(t.Context()), "validateCredentials must accept matching master signer")

	invalidSecret := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, Secret: "invalid"}, userRoles, "")
	require.ErrorIs(t, invalidSecret.validateCredentials(t.Context()), errInvalidPrivateKey, "validateCredentials must return the expected error for invalid signer key")

	agentSecret := "0x1123456789012345678901234567890123456789012345678901234567890123"
	agentKey, err := parsePrivateKey(agentSecret)
	require.NoError(t, err, "parsePrivateKey must not error for agent test key")
	agentAddress := privateKeyAddress(agentKey)
	agentKey.Zero()
	agentRoles := map[string]string{
		officialSigningAddress: testUserRoleResponse,
		agentAddress:           `{"role":"agent","data":{"user":"` + officialSigningAddress + `"}}`,
	}
	agentSigner := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, Secret: agentSecret}, agentRoles, "")
	require.NoError(t, agentSigner.validateCredentials(t.Context()), "validateCredentials must accept approved API-wallet signer")

	unauthorisedRoles := map[string]string{
		officialSigningAddress: testUserRoleResponse,
		agentAddress:           `{"role":"agent","data":{"user":"` + testOtherAddress + `"}}`,
	}
	unauthorisedSigner := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, Secret: agentSecret}, unauthorisedRoles, "")
	require.ErrorIs(t, unauthorisedSigner.validateCredentials(t.Context()), errSignerNotAuthorised, "validateCredentials must return the expected error for unapproved API-wallet signer")

	invalidAgentUserRoles := map[string]string{
		officialSigningAddress: testUserRoleResponse,
		agentAddress:           `{"role":"agent","data":{"user":"invalid"}}`,
	}
	invalidAgentUser := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, Secret: agentSecret}, invalidAgentUserRoles, "")
	require.ErrorIs(t, invalidAgentUser.validateCredentials(t.Context()), errSignerNotAuthorised, "validateCredentials must return the expected error for agent with malformed user")

	invalidVault := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, SubAccount: "invalid"}, userRoles, "")
	require.ErrorIs(t, invalidVault.validateCredentials(t.Context()), errInvalidAddress, "validateCredentials must return the expected error for invalid vault address")

	subaccountRoles := map[string]string{
		officialSigningAddress: testUserRoleResponse,
		testVaultAddress:       `{"role":"subAccount","data":{"master":"` + officialSigningAddress + `"}}`,
	}
	subaccount := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress}, subaccountRoles, "")
	require.NoError(t, subaccount.validateCredentials(t.Context()), "validateCredentials must accept owned subaccount")

	unownedSubaccountRoles := map[string]string{
		officialSigningAddress: testUserRoleResponse,
		testVaultAddress:       `{"role":"subAccount","data":{"master":"` + testOtherAddress + `"}}`,
	}
	unownedSubaccount := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress}, unownedSubaccountRoles, "")
	require.ErrorIs(t, unownedSubaccount.validateCredentials(t.Context()), errVaultNotAuthorised, "validateCredentials must return the expected error for unowned subaccount")

	invalidSubaccountMasterRoles := map[string]string{
		officialSigningAddress: testUserRoleResponse,
		testVaultAddress:       `{"role":"subAccount","data":{"master":"invalid"}}`,
	}
	invalidSubaccountMaster := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress}, invalidSubaccountMasterRoles, "")
	require.ErrorIs(t, invalidSubaccountMaster.validateCredentials(t.Context()), errVaultNotAuthorised, "validateCredentials must return the expected error for malformed subaccount master")

	vaultRoles := map[string]string{
		officialSigningAddress: testUserRoleResponse,
		testVaultAddress:       `{"role":"vault"}`,
	}
	vault := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress}, vaultRoles,
		`{"vaultAddress":"`+testVaultAddress+`","leader":"`+officialSigningAddress+`"}`)
	require.NoError(t, vault.validateCredentials(t.Context()), "validateCredentials must accept led vault")

	unownedVault := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress}, vaultRoles,
		`{"vaultAddress":"`+testVaultAddress+`","leader":"`+testOtherAddress+`"}`)
	require.ErrorIs(t, unownedVault.validateCredentials(t.Context()), errVaultNotAuthorised, "validateCredentials must return the expected error for vault led by another account")

	invalidVaultLeader := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress}, vaultRoles,
		`{"vaultAddress":"`+testVaultAddress+`","leader":"invalid"}`)
	require.ErrorIs(t, invalidVaultLeader.validateCredentials(t.Context()), errVaultNotAuthorised, "validateCredentials must return the expected error for vault with malformed leader")

	unknownVaultRole := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress}, map[string]string{
		officialSigningAddress: testUserRoleResponse,
		testVaultAddress:       `{"role":"missing"}`,
	}, "")
	require.ErrorIs(t, unknownVaultRole.validateCredentials(t.Context()), errVaultNotAuthorised, "validateCredentials must return the expected error for unsupported vault role")

	failing := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	setTestCredentials(failing, &accounts.Credentials{Key: officialSigningAddress})
	require.Error(t, failing.validateCredentials(t.Context()), "validateCredentials must return role lookup failure")

	for _, tc := range []struct {
		name        string
		credentials *accounts.Credentials
		failureType string
		failureUser string
	}{
		{
			name:        "vault role lookup",
			credentials: &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress},
			failureType: testUserRoleInfoType,
			failureUser: testVaultAddress,
		},
		{
			name:        "vault details lookup",
			credentials: &accounts.Credentials{Key: officialSigningAddress, SubAccount: testVaultAddress},
			failureType: "vaultDetails",
		},
		{
			name:        "signer role lookup",
			credentials: &accounts.Credentials{Key: officialSigningAddress, Secret: agentSecret},
			failureType: testUserRoleInfoType,
			failureUser: agentAddress,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failedLookup := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload infoRequest
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload), "Decode should not error for failed-role fixture request") {
					return
				}
				if payload.Type == tc.failureType && (tc.failureUser == "" || payload.User == tc.failureUser) {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				switch {
				case payload.Type == testUserRoleInfoType && payload.User == officialSigningAddress:
					_, err := w.Write([]byte(testUserRoleResponse))
					assert.NoError(t, err, "Write should not error for the account role")
				case payload.Type == testUserRoleInfoType && payload.User == testVaultAddress:
					_, err := w.Write([]byte(`{"role":"vault"}`))
					assert.NoError(t, err, "Write should not error for the vault role")
				default:
					t.Errorf("payload.Type and payload.User should match an expected failed-role fixture request: type=%q user=%q", payload.Type, payload.User)
				}
			}))
			setTestCredentials(failedLookup, tc.credentials)
			require.Error(t, failedLookup.validateCredentials(t.Context()), "validateCredentials must return the nested API failure")
		})
	}
}

func TestValidateAPICredentials(t *testing.T) {
	missing := new(Exchange)
	missing.SetDefaults()
	require.ErrorIs(t, missing.ValidateAPICredentials(t.Context(), asset.Empty), request.ErrAuthRequestFailed, "ValidateAPICredentials must classify missing credentials as an authentication failure")

	ex := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress}, map[string]string{
		officialSigningAddress: testUserRoleResponse,
	}, "")
	require.NoError(t, ex.ValidateAPICredentials(t.Context(), asset.Empty), "ValidateAPICredentials must accept valid credentials with an empty asset filter")
	require.NoError(t, ex.ValidateAPICredentials(t.Context(), asset.Spot), "ValidateAPICredentials must accept valid credentials with a supported asset")
	require.ErrorIs(t, ex.ValidateAPICredentials(t.Context(), asset.Options), asset.ErrNotSupported, "ValidateAPICredentials must return the expected error for unsupported validation asset")

	invalid := newRoleTestExchange(t, &accounts.Credentials{Key: officialSigningAddress}, map[string]string{
		officialSigningAddress: `{"role":"missing"}`,
	}, "")
	err := invalid.ValidateAPICredentials(t.Context(), asset.Empty)
	require.ErrorIs(t, err, errConfiguredAccountMissing, "ValidateAPICredentials must retain the credential validation error for missing account")
	require.ErrorIs(t, err, request.ErrAuthRequestFailed, "ValidateAPICredentials must classify invalid credentials as an authentication failure")

	for _, tc := range []struct {
		name        string
		credentials *accounts.Credentials
		expectedIs  error
	}{
		{name: "invalid account address", credentials: &accounts.Credentials{Key: "invalid"}, expectedIs: errInvalidAddress},
		{
			name:        "invalid vault address",
			credentials: &accounts.Credentials{Key: officialSigningAddress, SubAccount: "invalid"},
			expectedIs:  errInvalidAddress,
		},
		{
			name:        "invalid signer key",
			credentials: &accounts.Credentials{Key: officialSigningAddress, Secret: "invalid"},
			expectedIs:  errInvalidPrivateKey,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := newRoleTestExchange(t, tc.credentials, map[string]string{
				officialSigningAddress: testUserRoleResponse,
			}, "")
			err := invalid.ValidateAPICredentials(t.Context(), asset.Empty)
			require.ErrorIs(t, err, tc.expectedIs, "ValidateAPICredentials must return the expected error for invalid credential material")
			require.ErrorIs(t, err, request.ErrAuthRequestFailed, "ValidateAPICredentials must classify invalid credential material as an authentication failure")
		})
	}

	for _, tc := range []struct {
		name        string
		replacement *accounts.Credentials
		expectedIs  error
	}{
		{name: "credentials removed", expectedIs: request.ErrAuthRequestFailed},
		{
			name:        "credentials changed",
			replacement: &accounts.Credentials{Key: testOtherAddress},
			expectedIs:  errCredentialsChanged,
		},
	} {
		t.Run(tc.name+" during explicit validation", func(t *testing.T) {
			var mutatingExchange *Exchange
			mutatingExchange = newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				setTestCredentials(mutatingExchange, tc.replacement)
				_, writeErr := w.Write([]byte(testUserRoleResponse))
				assert.NoError(t, writeErr, "Write should not error for a mutating credential-validation response")
			}))
			setTestCredentials(mutatingExchange, &accounts.Credentials{Key: officialSigningAddress})
			err := mutatingExchange.ValidateAPICredentials(t.Context(), asset.Empty)
			require.ErrorIs(t, err, tc.expectedIs, "ValidateAPICredentials must return the expected error for credentials mutated during explicit validation")
			require.ErrorIs(t, err, request.ErrAuthRequestFailed, "ValidateAPICredentials must classify credentials mutated during explicit validation as an authentication failure")
		})
	}
}
