package exchange

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
)

func TestGetCredentials(t *testing.T) {
	t.Parallel()
	var b Base
	_, err := b.GetCredentials(t.Context())
	require.ErrorIs(t, err, ErrCredentialsAreEmpty)

	b.API.CredentialsValidator.RequiresKey = true
	ctx := accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Secret: "wow"})
	_, err = b.GetCredentials(ctx)
	require.ErrorIs(t, err, errRequiresAPIKey)

	b.API.CredentialsValidator.RequiresSecret = true
	ctx = accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "wow"})
	_, err = b.GetCredentials(ctx)
	require.ErrorIs(t, err, errRequiresAPISecret)

	b.API.CredentialsValidator.RequiresBase64DecodeSecret = true
	ctx = accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{
		Key:    "meow",
		Secret: "invalidb64",
	})
	_, err = b.GetCredentials(ctx)
	require.ErrorIs(t, err, errBase64DecodeFailure)

	const expectedBase64DecodedOutput = "hello world"
	ctx = accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{
		Key:    "meow",
		Secret: "aGVsbG8gd29ybGQ=",
	})
	creds, err := b.GetCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, expectedBase64DecodedOutput, creds.Secret, "GetCredentials should base64 decode secret")

	ctx = context.WithValue(t.Context(), accounts.ContextCredentialsFlag, "pewpew")
	_, err = b.GetCredentials(ctx)
	require.ErrorIs(t, err, common.ErrTypeAssertFailure)

	b.API.CredentialsValidator.RequiresBase64DecodeSecret = false
	fullCred := &accounts.Credentials{
		Key:             "superkey",
		Secret:          "supersecret",
		SubAccount:      "supersub",
		ClientID:        "superclient",
		PEMKey:          "superpem",
		OneTimePassword: "superOneTimePasssssss",
	}

	ctx = accounts.DeployCredentialsToContext(t.Context(), fullCred)
	creds, err = b.GetCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, "superkey", creds.Key, "GetCredentials should return key from context")
	assert.Equal(t, "supersecret", creds.Secret, "GetCredentials should return secret from context")
	assert.Equal(t, "supersub", creds.SubAccount, "GetCredentials should return sub account from context")
	assert.Equal(t, "superclient", creds.ClientID, "GetCredentials should return client id from context")
	assert.Equal(t, "superpem", creds.PEMKey, "GetCredentials should return pem key from context")
	assert.Equal(t, "superOneTimePasssssss", creds.OneTimePassword, "GetCredentials should return otp from context")

	lonelyCred := &accounts.Credentials{
		Key:             "superkey",
		Secret:          "supersecret",
		SubAccount:      "supersub",
		PEMKey:          "superpem",
		OneTimePassword: "superOneTimePasssssss",
	}

	ctx = accounts.DeployCredentialsToContext(t.Context(), lonelyCred)
	b.API.CredentialsValidator.RequiresClientID = true
	_, err = b.GetCredentials(ctx)
	require.ErrorIs(t, err, errRequiresAPIClientID)

	b.API.SetKey("hello")
	b.API.SetSecret("sir")
	b.API.SetClientID("1337")

	ctx = context.WithValue(t.Context(), accounts.ContextSubAccountFlag, "superaccount")
	overridedSA, err := b.GetCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, "hello", overridedSA.Key, "GetCredentials should fall back to API key")
	assert.Equal(t, "sir", overridedSA.Secret, "GetCredentials should fall back to API secret")
	assert.Equal(t, "1337", overridedSA.ClientID, "GetCredentials should fall back to API client id")
	assert.Equal(t, "superaccount", overridedSA.SubAccount, "GetCredentials should override sub account from context")

	notOverrided, err := b.GetCredentials(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "hello", notOverrided.Key, "GetCredentials should retain API key")
	assert.Equal(t, "sir", notOverrided.Secret, "GetCredentials should retain API secret")
	assert.Equal(t, "1337", notOverrided.ClientID, "GetCredentials should retain API client id")
	assert.Empty(t, notOverrided.SubAccount, "GetCredentials should keep default sub account when not overridden")
}

func TestAreCredentialsValid(t *testing.T) {
	t.Parallel()
	var b Base
	assert.False(t, b.AreCredentialsValid(t.Context()), "AreCredentialsValid should return false when no credentials provided")
	ctx := accounts.DeployCredentialsToContext(t.Context(), &accounts.Credentials{Key: "hello"})
	assert.True(t, b.AreCredentialsValid(ctx), "AreCredentialsValid should return true when key provided")
}

func TestVerifyAPICredentials(t *testing.T) {
	t.Parallel()

	type tester struct {
		Key                        string
		Secret                     string
		ClientID                   string
		PEMKey                     string
		RequiresPEM                bool
		RequiresKey                bool
		RequiresSecret             bool
		RequiresClientID           bool
		RequiresBase64DecodeSecret bool
		UseSetCredentials          bool
		CheckBase64DecodedOutput   bool
		Expected                   error
	}

	const expectedBase64DecodedOutput = "hello world"

	testCases := []tester{
		// Empty credentials
		{Expected: ErrCredentialsAreEmpty},
		// test key
		{RequiresKey: true, Expected: errRequiresAPIKey, Secret: "bruh"},
		{RequiresKey: true, Key: "k3y"},
		// test secret
		{RequiresSecret: true, Expected: errRequiresAPISecret, Key: "bruh"},
		{RequiresSecret: true, Secret: "s3cr3t"},
		// test pem
		{RequiresPEM: true, Expected: errRequiresAPIPEMKey, Key: "bruh"},
		{RequiresPEM: true, PEMKey: "p3mK3y"},
		// test clientID
		{RequiresClientID: true, Expected: errRequiresAPIClientID, Key: "bruh"},
		{RequiresClientID: true, ClientID: "cli3nt1D"},
		// test requires base64 decode secret
		{RequiresBase64DecodeSecret: true, RequiresSecret: true, Expected: errRequiresAPISecret, Key: "bruh"},
		{RequiresBase64DecodeSecret: true, Secret: "%%", Expected: errBase64DecodeFailure},
		{RequiresBase64DecodeSecret: true, Secret: "aGVsbG8gd29ybGQ=", CheckBase64DecodedOutput: true},
		{RequiresBase64DecodeSecret: true, Secret: "aGVsbG8gd29ybGQ=", UseSetCredentials: true, CheckBase64DecodedOutput: true},
	}

	setupBase := func(tData *tester) *Base {
		b := &Base{
			API: API{
				CredentialsValidator: config.APICredentialsValidatorConfig{
					RequiresKey:                tData.RequiresKey,
					RequiresSecret:             tData.RequiresSecret,
					RequiresClientID:           tData.RequiresClientID,
					RequiresPEM:                tData.RequiresPEM,
					RequiresBase64DecodeSecret: tData.RequiresBase64DecodeSecret,
				},
			},
		}
		if tData.UseSetCredentials {
			b.SetCredentials(tData.Key, tData.Secret, tData.ClientID, "", tData.PEMKey, "")
		} else {
			b.API.SetKey(tData.Key)
			b.API.SetSecret(tData.Secret)
			b.API.SetClientID(tData.ClientID)
			b.API.SetPEMKey(tData.PEMKey)
		}
		return b
	}

	for x, tc := range testCases {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			b := setupBase(&tc)
			if tc.Expected != nil {
				assert.ErrorIs(t, b.VerifyAPICredentials(&b.API.credentials), tc.Expected, "VerifyAPICredentials should return expected error")
			} else {
				assert.NoError(t, b.VerifyAPICredentials(&b.API.credentials), "VerifyAPICredentials should not return error")
			}

			if tc.CheckBase64DecodedOutput {
				assert.Equalf(t, expectedBase64DecodedOutput, b.API.credentials.Secret, "Test %d secret should be base64 decoded", x+1)
			}
		})
	}
}

func TestCheckCredentials(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		base              *Base
		checkBase64Output bool
		expectedErr       error
	}{
		{
			name: "Test SkipAuthCheck",
			base: &Base{
				SkipAuthCheck: true,
			},
			expectedErr: nil,
		},
		{
			name: "Test credentials failure",
			base: &Base{
				API: API{
					CredentialsValidator: config.APICredentialsValidatorConfig{RequiresKey: true},
					credentials:          accounts.Credentials{OneTimePassword: "wow"},
				},
			},
			expectedErr: errRequiresAPIKey,
		},
		{
			name: "Test exchange usage with authenticated API support disabled, but with valid credentials",
			base: &Base{
				LoadedByConfig: true,
				API: API{
					CredentialsValidator: config.APICredentialsValidatorConfig{RequiresKey: true},
					credentials:          accounts.Credentials{Key: "k3y"},
				},
			},
			expectedErr: ErrAuthenticationSupportNotEnabled,
		},
		{
			name: "Test enabled authenticated API support and loaded by config but invalid credentials",
			base: &Base{
				LoadedByConfig: true,
				API: API{
					AuthenticatedSupport: true,
					CredentialsValidator: config.APICredentialsValidatorConfig{RequiresKey: true},
					credentials:          accounts.Credentials{},
				},
			},
			expectedErr: ErrCredentialsAreEmpty,
		},
		{
			name: "Test base64 decoded invalid credentials",
			base: &Base{
				API: API{
					CredentialsValidator: config.APICredentialsValidatorConfig{RequiresBase64DecodeSecret: true},
					credentials:          accounts.Credentials{Secret: "invalid"},
				},
			},
			expectedErr: errBase64DecodeFailure,
		},
		{
			name: "Test base64 decoded valid credentials",
			base: &Base{
				API: API{
					CredentialsValidator: config.APICredentialsValidatorConfig{RequiresBase64DecodeSecret: true},
					credentials:          accounts.Credentials{Secret: "aGVsbG8gd29ybGQ="},
				},
			},
			checkBase64Output: true,
			expectedErr:       nil,
		},
		{
			name: "Test valid credentials",
			base: &Base{
				API: API{
					AuthenticatedSupport: true,
					CredentialsValidator: config.APICredentialsValidatorConfig{RequiresKey: true},
					credentials:          accounts.Credentials{Key: "k3y"},
				},
			},
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.expectedErr != nil {
				assert.ErrorIsf(t, tc.base.CheckCredentials(&tc.base.API.credentials, false), tc.expectedErr, "%s should return expected error", tc.name)
			} else {
				assert.NoErrorf(t, tc.base.CheckCredentials(&tc.base.API.credentials, false), "%s should not return error", tc.name)
			}

			if tc.checkBase64Output {
				assert.Truef(t, tc.base.API.credentials.SecretBase64Decoded, "%s should mark secret as base64 decoded", tc.name)
				assert.Equalf(t, "hello world", tc.base.API.credentials.Secret, "%s should decode base64 secret", tc.name)
			}
		})
	}
}

func TestAPISetters(t *testing.T) {
	t.Parallel()
	api := API{}
	api.SetKey(accounts.Key)
	assert.Equal(t, accounts.Key, api.credentials.Key, "SetKey should set API key")

	api = API{}
	api.SetSecret(accounts.Secret)
	assert.Equal(t, accounts.Secret, api.credentials.Secret, "SetSecret should set secret")

	api = API{}
	api.SetClientID(accounts.ClientID)
	assert.Equal(t, accounts.ClientID, api.credentials.ClientID, "SetClientID should set client id")

	api = API{}
	api.SetPEMKey(accounts.PEMKey)
	assert.Equal(t, accounts.PEMKey, api.credentials.PEMKey, "SetPEMKey should set pem key")

	api = API{}
	api.SetSubAccount(accounts.SubAccountSTR)
	assert.Equal(t, accounts.SubAccountSTR, api.credentials.SubAccount, "SetSubAccount should set sub account")
}

func TestSetCredentials(t *testing.T) {
	t.Parallel()

	b := Base{
		Name:    "TESTNAME",
		Enabled: false,
		API: API{
			AuthenticatedSupport:          false,
			AuthenticatedWebsocketSupport: false,
		},
	}

	b.SetCredentials("RocketMan", "Digereedoo", "007", "", "", "")
	assert.Equal(t, "RocketMan", b.API.credentials.Key, "SetCredentials should set key")
	assert.Equal(t, "Digereedoo", b.API.credentials.Secret, "SetCredentials should set secret")
	assert.Equal(t, "007", b.API.credentials.ClientID, "SetCredentials should set client id")

	// Invalid secret
	b.API.CredentialsValidator.RequiresBase64DecodeSecret = true
	b.API.AuthenticatedSupport = true
	b.SetCredentials("RocketMan", "%%", "007", "", "", "")
	assert.False(t, b.API.AuthenticatedSupport, "SetCredentials should disable REST auth when base64 decode fails")
	assert.False(t, b.API.AuthenticatedWebsocketSupport, "SetCredentials should disable websocket auth when base64 decode fails")

	// valid secret
	b.API.CredentialsValidator.RequiresBase64DecodeSecret = true
	b.API.AuthenticatedSupport = true
	b.SetCredentials("RocketMan", "aGVsbG8gd29ybGQ=", "007", "", "", "")
	assert.True(t, b.API.AuthenticatedSupport, "SetCredentials should keep REST auth enabled for valid secret")
	assert.Equal(t, "hello world", b.API.credentials.Secret, "SetCredentials should decode base64 secret")
}

func TestGetDefaultCredentials(t *testing.T) {
	var b Base
	assert.Nil(t, b.GetDefaultCredentials(), "GetDefaultCredentials should return nil when not configured")
	b.SetCredentials("test", "", "", "", "", "")
	assert.NotNil(t, b.GetDefaultCredentials(), "GetDefaultCredentials should return credentials when configured")
}

func TestSetAPICredentialDefaults(t *testing.T) {
	t.Parallel()

	b := Base{
		Config: &config.Exchange{},
	}
	b.API.CredentialsValidator.RequiresKey = true
	b.API.CredentialsValidator.RequiresSecret = true
	b.API.CredentialsValidator.RequiresBase64DecodeSecret = true
	b.API.CredentialsValidator.RequiresClientID = true
	b.API.CredentialsValidator.RequiresPEM = true
	b.SetAPICredentialDefaults()

	assert.True(t, b.Config.API.CredentialsValidator.RequiresKey, "SetAPICredentialDefaults should propagate requires key")
	assert.True(t, b.Config.API.CredentialsValidator.RequiresSecret, "SetAPICredentialDefaults should propagate requires secret")
	assert.True(t, b.Config.API.CredentialsValidator.RequiresBase64DecodeSecret, "SetAPICredentialDefaults should propagate base64 decode requirement")
	assert.True(t, b.Config.API.CredentialsValidator.RequiresClientID, "SetAPICredentialDefaults should propagate client id requirement")
	assert.True(t, b.Config.API.CredentialsValidator.RequiresPEM, "SetAPICredentialDefaults should propagate pem requirement")
}

func TestGetAuthenticatedAPISupport(t *testing.T) {
	t.Parallel()

	base := Base{
		API: API{
			AuthenticatedSupport:          true,
			AuthenticatedWebsocketSupport: false,
		},
	}

	assert.True(t, base.IsRESTAuthenticationSupported(), "IsRESTAuthenticationSupported should return true when enabled")
	base.API.AuthenticatedSupport = false
	assert.False(t, base.IsRESTAuthenticationSupported(), "IsRESTAuthenticationSupported should return false when disabled")
	assert.False(t, base.IsWebsocketAuthenticationSupported(), "IsWebsocketAuthenticationSupported should return false when disabled")
	base.API.AuthenticatedWebsocketSupport = true
	assert.True(t, base.IsWebsocketAuthenticationSupported(), "IsWebsocketAuthenticationSupported should return true when enabled")
}
