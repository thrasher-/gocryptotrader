package hyperliquid

import (
	"cmp"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

const (
	// testAgentPrivateKey signs as an API wallet in authority tests
	testAgentPrivateKey = "0x1123456789012345678901234567890123456789012345678901234567890123"
	// testSubAccountAddress is the vault address of the Python SDK's signing vectors, used as a subaccount or vault
	testSubAccountAddress = "0x1719884eb866cb12b2287399b15f7db5e7d775ea"
	testOtherAddress      = "0x2222222222222222222222222222222222222222"
)

// newRoleServerExchange serves userRole requests from roles and vaultDetails requests from leaders, both keyed by
// address; an exchange endpoint request fails the test unless actions handles it, and every role lookup is counted
func newRoleServerExchange(t *testing.T, roles, leaders map[string]string, actions http.HandlerFunc) (*Exchange, *atomic.Int32) {
	t.Helper()
	var roleLookups atomic.Int32
	return newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			var body InfoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
				return
			}
			var response string
			switch body.Type {
			case "userRole":
				roleLookups.Add(1)
				response = cmp.Or(roles[body.User], `{"role":"missing"}`)
			case "vaultDetails":
				response = `{"vaultAddress":"` + body.VaultAddress + `","leader":"` + leaders[body.VaultAddress] + `"}`
			default:
				http.Error(w, "unexpected info request "+body.Type, http.StatusBadRequest)
				return
			}
			_, err := w.Write([]byte(response))
			assert.NoError(t, err, "Writing the info response should not error")
		case "/exchange":
			if actions == nil {
				t.Error("The exchange endpoint should not be reached")
				http.Error(w, "unexpected exchange request", http.StatusInternalServerError)
				return
			}
			actions(w, r)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	})), &roleLookups
}

func testKeyAddress(t *testing.T, secret string) string {
	t.Helper()
	key, err := parsePrivateKey(secret)
	require.NoError(t, err, "parsePrivateKey must not error")
	defer key.Zero()
	return privateKeyAddress(key)
}

func TestGetWatchAddress(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	ex.SetDefaults()
	_, err := ex.getWatchAddress(t.Context())
	assert.Error(t, err, "getWatchAddress should error without credentials")

	for _, tc := range []struct {
		credentials *accounts.Credentials
		exp         string
		err         error
	}{
		{credentials: &accounts.Credentials{Key: strings.ToUpper(testAccountAddress)}, exp: testAccountAddress},
		{credentials: &accounts.Credentials{Key: testAccountAddress, SubAccount: strings.ToUpper(testSubAccountAddress)}, exp: testSubAccountAddress},
		{credentials: &accounts.Credentials{Key: "invalid"}, err: errInvalidAddress},
		{credentials: &accounts.Credentials{Key: testAccountAddress, SubAccount: "invalid"}, err: errInvalidAddress},
	} {
		setTestCredentials(ex, tc.credentials)
		address, err := ex.getWatchAddress(t.Context())
		require.ErrorIsf(t, err, tc.err, "getWatchAddress must return the expected error for %+v", tc.credentials)
		assert.Equalf(t, tc.exp, address, "getWatchAddress should return the watched address for %+v", tc.credentials)
	}
}

func TestGetSigningCredentials(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	ex.SetDefaults()
	_, _, err := ex.getSigningCredentials(t.Context())
	assert.Error(t, err, "getSigningCredentials should error without credentials")

	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress})
	_, _, err = ex.getSigningCredentials(t.Context())
	assert.ErrorIs(t, err, errPrivateKeyRequired, "getSigningCredentials should require a private key")
	assert.ErrorIs(t, err, request.ErrAuthRequestFailed, "getSigningCredentials should report a missing private key as an authentication failure")

	setTestCredentials(ex, &accounts.Credentials{Key: "invalid", Secret: testPrivateKey})
	_, _, err = ex.getSigningCredentials(t.Context())
	assert.ErrorIs(t, err, errInvalidAddress, "getSigningCredentials should reject an invalid account address")

	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey, SubAccount: "invalid"})
	_, _, err = ex.getSigningCredentials(t.Context())
	assert.ErrorIs(t, err, errInvalidAddress, "getSigningCredentials should reject an invalid subaccount address")

	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey})
	credentials, vaultAddress, err := ex.getSigningCredentials(t.Context())
	require.NoError(t, err, "getSigningCredentials must not error for an account")
	assert.Equal(t, testPrivateKey, credentials.Secret, "getSigningCredentials should return the private key")
	assert.Empty(t, vaultAddress, "getSigningCredentials should not set a vault for an account")

	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey, SubAccount: strings.ToUpper(testSubAccountAddress)})
	_, vaultAddress, err = ex.getSigningCredentials(t.Context())
	require.NoError(t, err, "getSigningCredentials must not error for a subaccount")
	assert.Equal(t, testSubAccountAddress, vaultAddress, "getSigningCredentials should normalise the subaccount address")
}

func TestGetUserSigningCredentials(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	ex.SetDefaults()
	_, _, err := ex.getUserSigningCredentials(t.Context())
	assert.Error(t, err, "getUserSigningCredentials should error without credentials")

	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress, Secret: "invalid"})
	_, _, err = ex.getUserSigningCredentials(t.Context())
	assert.ErrorIs(t, err, errInvalidPrivateKey, "getUserSigningCredentials should reject an invalid private key")

	setTestCredentials(ex, &accounts.Credentials{Key: testAccountAddress, Secret: testAgentPrivateKey})
	_, _, err = ex.getUserSigningCredentials(t.Context())
	assert.ErrorIs(t, err, errUserSignedMasterRequired, "getUserSigningCredentials should reject an API wallet key")

	setTestCredentials(ex, &accounts.Credentials{Key: strings.ToUpper(testAccountAddress), Secret: testPrivateKey, SubAccount: strings.ToUpper(testSubAccountAddress)})
	credentials, subAccount, err := ex.getUserSigningCredentials(t.Context())
	require.NoError(t, err, "getUserSigningCredentials must not error for the account's own key")
	assert.Equal(t, testPrivateKey, credentials.Secret, "getUserSigningCredentials should return the private key")
	assert.Equal(t, testSubAccountAddress, subAccount, "getUserSigningCredentials should normalise the subaccount address")
}

func TestValidateCachedAuthority(t *testing.T) {
	t.Parallel()
	ex, roleLookups := newRoleServerExchange(t, map[string]string{testAccountAddress: `{"role":"user"}`}, nil, nil)
	for _, tc := range []struct {
		credentials *accounts.Credentials
		err         error
	}{
		{credentials: &accounts.Credentials{Key: "invalid"}, err: errInvalidAddress},
		{credentials: &accounts.Credentials{Key: testAccountAddress, SubAccount: "invalid"}, err: errInvalidAddress},
		{credentials: &accounts.Credentials{Key: testAccountAddress, Secret: "invalid"}, err: errInvalidPrivateKey},
	} {
		_, err := ex.validateCachedAuthority(t.Context(), tc.credentials, false)
		assert.ErrorIsf(t, err, tc.err, "validateCachedAuthority should reject %+v", tc.credentials)
	}
	assert.Zero(t, roleLookups.Load(), "validateCachedAuthority should not look up roles for invalid credentials")

	credentials := &accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey}
	validationKey, err := ex.validateCachedAuthority(t.Context(), credentials, false)
	require.NoError(t, err, "validateCachedAuthority must not error for the account's own key")
	exp := authorityValidationKey{accountAddress: testAccountAddress, signerAddress: testAccountAddress, mainnet: true}
	assert.Equal(t, exp, validationKey, "validateCachedAuthority should key the account, signer and environment")
	assert.Equal(t, int32(1), roleLookups.Load(), "validateCachedAuthority should look up the account role once")

	cachedKey, err := ex.validateCachedAuthority(t.Context(), credentials, false)
	require.NoError(t, err, "validateCachedAuthority must not error for cached authority")
	assert.Equal(t, validationKey, cachedKey, "validateCachedAuthority should return the cached key")
	assert.Equal(t, int32(1), roleLookups.Load(), "validateCachedAuthority should not repeat a cached validation")

	_, err = ex.validateCachedAuthority(t.Context(), credentials, true)
	require.NoError(t, err, "validateCachedAuthority must not error when forced")
	assert.Equal(t, int32(2), roleLookups.Load(), "validateCachedAuthority should revalidate when forced")

	_, err = ex.validateCachedAuthority(t.Context(), &accounts.Credentials{Key: testAccountAddress}, false)
	require.NoError(t, err, "validateCachedAuthority must not error for a watch-only account")
	assert.Equal(t, int32(3), roleLookups.Load(), "validateCachedAuthority should revalidate a different signer")

	ex.invalidateAuthority(&validationKey)
	ex.authorityValidationMu.Lock()
	validated := ex.authorityValidated
	ex.authorityValidationMu.Unlock()
	assert.True(t, validated, "invalidateAuthority should keep a validation it does not match")
}

func TestInvalidateAuthority(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	validationKey := authorityValidationKey{accountAddress: testAccountAddress, mainnet: true}
	ex.authorityValidationKey = validationKey
	ex.authorityValidated = true
	ex.invalidateAuthority(&authorityValidationKey{accountAddress: testOtherAddress})
	assert.True(t, ex.authorityValidated, "invalidateAuthority should keep another key's validation")
	ex.invalidateAuthority(&validationKey)
	assert.False(t, ex.authorityValidated, "invalidateAuthority should clear a matching validation")
}

func TestValidateAuthority(t *testing.T) {
	t.Parallel()
	agentAddress := testKeyAddress(t, testAgentPrivateKey)
	user := `{"role":"user"}`
	for _, tc := range []struct {
		name    string
		key     authorityValidationKey
		roles   map[string]string
		leaders map[string]string
		err     error
	}{
		{name: "account", key: authorityValidationKey{accountAddress: testAccountAddress}, roles: map[string]string{testAccountAddress: user}},
		{name: "missing account", key: authorityValidationKey{accountAddress: testAccountAddress}, err: errConfiguredAccountMissing},
		{name: "account is an agent", key: authorityValidationKey{accountAddress: testAccountAddress}, roles: map[string]string{testAccountAddress: `{"role":"agent","data":{"user":"` + testOtherAddress + `"}}`}, err: errConfiguredAccountMissing},
		{
			name:  "owned subaccount",
			key:   authorityValidationKey{accountAddress: testAccountAddress, vaultAddress: testSubAccountAddress},
			roles: map[string]string{testAccountAddress: user, testSubAccountAddress: `{"role":"subAccount","data":{"master":"` + testAccountAddress + `"}}`},
		},
		{
			name:  "foreign subaccount",
			key:   authorityValidationKey{accountAddress: testAccountAddress, vaultAddress: testSubAccountAddress},
			roles: map[string]string{testAccountAddress: user, testSubAccountAddress: `{"role":"subAccount","data":{"master":"` + testOtherAddress + `"}}`},
			err:   errVaultNotAuthorised,
		},
		{
			name:    "led vault",
			key:     authorityValidationKey{accountAddress: testAccountAddress, vaultAddress: testSubAccountAddress},
			roles:   map[string]string{testAccountAddress: user, testSubAccountAddress: `{"role":"vault"}`},
			leaders: map[string]string{testSubAccountAddress: strings.ToUpper(testAccountAddress)},
		},
		{
			name:    "foreign vault",
			key:     authorityValidationKey{accountAddress: testAccountAddress, vaultAddress: testSubAccountAddress},
			roles:   map[string]string{testAccountAddress: user, testSubAccountAddress: `{"role":"vault"}`},
			leaders: map[string]string{testSubAccountAddress: testOtherAddress},
			err:     errVaultNotAuthorised,
		},
		{
			name:  "vault address is a user",
			key:   authorityValidationKey{accountAddress: testAccountAddress, vaultAddress: testSubAccountAddress},
			roles: map[string]string{testAccountAddress: user, testSubAccountAddress: user},
			err:   errVaultNotAuthorised,
		},
		{
			name:  "account signs",
			key:   authorityValidationKey{accountAddress: testAccountAddress, signerAddress: testAccountAddress},
			roles: map[string]string{testAccountAddress: user},
		},
		{
			name:  "approved API wallet",
			key:   authorityValidationKey{accountAddress: testAccountAddress, signerAddress: agentAddress},
			roles: map[string]string{testAccountAddress: user, agentAddress: `{"role":"agent","data":{"user":"` + testAccountAddress + `"}}`},
		},
		{
			name:  "another account's API wallet",
			key:   authorityValidationKey{accountAddress: testAccountAddress, signerAddress: agentAddress},
			roles: map[string]string{testAccountAddress: user, agentAddress: `{"role":"agent","data":{"user":"` + testOtherAddress + `"}}`},
			err:   errSignerNotAuthorised,
		},
		{
			name:  "unapproved signer",
			key:   authorityValidationKey{accountAddress: testAccountAddress, signerAddress: agentAddress},
			roles: map[string]string{testAccountAddress: user, agentAddress: user},
			err:   errSignerNotAuthorised,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex, _ := newRoleServerExchange(t, tc.roles, tc.leaders, nil)
			assert.ErrorIs(t, ex.validateAuthority(t.Context(), &tc.key), tc.err, "validateAuthority should return the expected error")
		})
	}

	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	assert.Error(t, ex.validateAuthority(t.Context(), &authorityValidationKey{accountAddress: testAccountAddress}), "validateAuthority should return a role lookup failure")
}

func TestIsMainnetEnvironment(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	assert.True(t, ex.isMainnetEnvironment(), "isMainnetEnvironment should default to mainnet without a config")
	ex.Config = &config.Exchange{}
	assert.True(t, ex.isMainnetEnvironment(), "isMainnetEnvironment should be mainnet when sandbox is off")
	ex.Config.UseSandbox = true
	assert.False(t, ex.isMainnetEnvironment(), "isMainnetEnvironment should be testnet when sandbox is on")
}

func TestNextNonce(t *testing.T) {
	t.Parallel()
	ex := new(Exchange)
	first := ex.nextNonce()
	assert.Greater(t, ex.nextNonce(), first, "nextNonce should strictly increase")

	ex.lastNonce.Store(1 << 62)
	assert.Equal(t, uint64(1<<62+1), ex.nextNonce(), "nextNonce should step past a nonce ahead of the clock")

	const goroutines = 32
	var mu sync.Mutex
	seen := make(map[uint64]struct{}, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			nonce := ex.nextNonce()
			mu.Lock()
			seen[nonce] = struct{}{}
			mu.Unlock()
		})
	}
	wg.Wait()
	assert.Len(t, seen, goroutines, "nextNonce should return a unique nonce to every concurrent caller")
}
