package hyperliquid

import (
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

func newTransferTestExchange(
	t *testing.T,
	credentials *accounts.Credentials,
	responses map[string]string,
	captured *signedActionRequest,
) *Exchange {
	t.Helper()
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			var request infoRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request), "Decode should not error for transfer validation request") {
				return
			}
			key := request.Type
			if request.DEX != "" {
				key += ":" + request.DEX
			}
			response, ok := responses[key]
			if request.Type == testUserRoleInfoType {
				if specific, exists := responses[key+":"+strings.ToLower(request.User)]; exists {
					response, ok = specific, true
				}
			}
			if !ok && request.Type == testUserRoleInfoType && credentials != nil && strings.EqualFold(request.User, credentials.Key) {
				response, ok = testUserRoleResponse, true
			}
			if !ok {
				http.Error(w, "unexpected info request", http.StatusBadRequest)
				return
			}
			_, err := w.Write([]byte(response))
			assert.NoError(t, err, "Write should not error for transfer validation response")
		case "/exchange":
			if captured != nil {
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(captured), "Decode should not error for signed transfer action") {
					return
				}
			}
			_, err := w.Write([]byte(`{"status":"ok","response":null}`))
			assert.NoError(t, err, "Write should not error for transfer action response")
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	if credentials != nil {
		setTestCredentials(ex, credentials)
	}
	return ex
}

func getCapturedAction(t *testing.T, captured *signedActionRequest) map[string]any {
	t.Helper()
	action, ok := captured.Action.(map[string]any)
	require.True(t, ok, "ok: captured action must decode as an object")
	return action
}

func TestGetBridgeChain(t *testing.T) {
	mainnet := new(Exchange)
	mainnet.Config = new(config.Exchange)
	assert.Equal(t, "Arbitrum", mainnet.getBridgeChain(), "getBridgeChain: mainnet bridge should use Arbitrum")
	sandbox := new(Exchange)
	sandbox.Config = &config.Exchange{UseSandbox: true}
	assert.Equal(t, "Arbitrum Sepolia", sandbox.getBridgeChain(), "getBridgeChain: sandbox bridge should use Arbitrum Sepolia")
}

func TestFormatTransferAmount(t *testing.T) {
	for _, tc := range []struct {
		name       string
		amount     float64
		expected   string
		expectedIs error
	}{
		{name: "integer", amount: 1, expected: "1"},
		{name: "fraction", amount: 1.23, expected: "1.23"},
		{name: "zero", expectedIs: errTransferAmountInvalid},
		{name: "negative", amount: -1, expectedIs: errTransferAmountInvalid},
		{name: "too precise", amount: 0.000000001, expectedIs: errTransferAmountInvalid},
		{name: "nan", amount: math.NaN(), expectedIs: errTransferAmountInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := formatTransferAmount(tc.amount)
			require.ErrorIs(t, err, tc.expectedIs, "formatTransferAmount must return the expected error for a transfer amount")
			assert.Equal(t, tc.expected, result, "result: formatted transfer amount should match")
		})
	}
}

func TestValidateUserSignedSubAccount(t *testing.T) {
	ex := new(Exchange)
	require.NoError(t, ex.validateUserSignedSubAccount(t.Context(), nil, ""), "validateUserSignedSubAccount must not require validation for empty subaccount")
	require.ErrorIs(t, ex.validateUserSignedSubAccount(t.Context(), nil, testVaultAddress), common.ErrNilPointer, "validateUserSignedSubAccount must return the expected error for nil credentials")

	failing := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	err := failing.validateUserSignedSubAccount(t.Context(), &accounts.Credentials{Key: officialSigningAddress}, testVaultAddress)
	require.Error(t, err, "validateUserSignedSubAccount must return subaccount role lookup failure")

	for _, tc := range []struct {
		name       string
		role       string
		account    string
		expectedIs error
	}{
		{name: "wrong role", role: `{"role":"vault"}`, account: officialSigningAddress, expectedIs: errTransferSubAccountInvalid},
		{name: "invalid account", role: `{"role":"subAccount","data":{"master":"` + officialSigningAddress + `"}}`, account: "invalid", expectedIs: errInvalidAddress},
		{name: "invalid master", role: `{"role":"subAccount","data":{"master":"invalid"}}`, account: officialSigningAddress, expectedIs: errTransferSubAccountInvalid},
		{name: "wrong master", role: `{"role":"subAccount","data":{"master":"` + testOtherAddress + `"}}`, account: officialSigningAddress, expectedIs: errTransferSubAccountInvalid},
		{name: "owned", role: `{"role":"subAccount","data":{"master":"` + officialSigningAddress + `"}}`, account: officialSigningAddress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			roleExchange := newStaticInfoExchange(t, map[string]string{testUserRoleInfoType: tc.role})
			err := roleExchange.validateUserSignedSubAccount(
				t.Context(),
				&accounts.Credentials{Key: tc.account},
				testVaultAddress,
			)
			require.ErrorIs(t, err, tc.expectedIs, "validateUserSignedSubAccount must return the expected error for subaccount validation")
		})
	}
}

func TestUserSignedTransfersRejectAgentAccount(t *testing.T) {
	agentSecret := "0x1123456789012345678901234567890123456789012345678901234567890123"
	agentKey, err := parsePrivateKey(agentSecret)
	require.NoError(t, err, "parsePrivateKey must not error for the agent test key")
	agentAddress := privateKeyAddress(agentKey)
	agentKey.Zero()

	for _, tc := range []struct {
		name string
		run  func(*Exchange) error
	}{
		{
			name: "class transfer",
			run: func(ex *Exchange) error {
				_, err := ex.TransferUSDCBetweenSpotAndPerp(t.Context(), 1, true)
				return err
			},
		},
		{
			name: "asset transfer",
			run: func(ex *Exchange) error {
				_, err := ex.SendAsset(t.Context(), &SendAssetRequest{
					Destination:    testOtherAddress,
					SourceDEX:      "spot",
					DestinationDEX: "spot",
					Token:          "USDC",
					Amount:         1,
				})
				return err
			},
		},
		{
			name: "Core USDC send",
			run: func(ex *Exchange) error {
				_, err := ex.SendCoreUSDC(t.Context(), testOtherAddress, 1)
				return err
			},
		},
		{
			name: "Core spot send",
			run: func(ex *Exchange) error {
				_, err := ex.SendCoreSpot(t.Context(), testOtherAddress, "USDC", 1)
				return err
			},
		},
		{
			name: "bridge withdrawal",
			run: func(ex *Exchange) error {
				_, err := ex.WithdrawFromBridge(t.Context(), testOtherAddress, 1)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var captured signedActionRequest
			ex := newTransferTestExchange(t, &accounts.Credentials{
				Key:    agentAddress,
				Secret: agentSecret,
			}, map[string]string{
				testUserRoleInfoType: `{"role":"agent","data":{"user":"` + officialSigningAddress + `"}}`,
				"spotMeta":           spotMetadataJSON,
			}, &captured)
			err := tc.run(ex)
			require.ErrorIs(t, err, errConfiguredAccountMissing, "tc.run must reject an API-wallet account")
			require.ErrorIs(t, err, request.ErrAuthRequestFailed, "tc.run must classify an API-wallet account as an authentication failure")
			assert.Zero(t, ex.lastNonce.Load(), "ex.lastNonce: a rejected API-wallet account should not consume a nonce")
			assert.Nil(t, captured.Action, "captured.Action: a rejected API-wallet account should not reach the exchange endpoint")
		})
	}
}

func TestResolveTransferToken(t *testing.T) {
	ex := new(Exchange)
	_, _, err := ex.resolveTransferToken(t.Context(), "")
	require.ErrorIs(t, err, errTransferTokenInvalid, "resolveTransferToken must return the expected error for blank token")

	failing := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, _, err = failing.resolveTransferToken(t.Context(), "USDC")
	require.Error(t, err, "resolveTransferToken must return spot metadata failure")

	valid := newStaticInfoExchange(t, map[string]string{"spotMeta": spotMetadataJSON})
	token, canonical, err := valid.resolveTransferToken(t.Context(), "USDC")
	require.NoError(t, err, "resolveTransferToken must not error for USDC")
	assert.Equal(t, uint64(0), token.Index, "token.Index: USDC should retain its token index")
	assert.Equal(t, "USDC:0x0", canonical, "canonical: USDC should use its full spot-transfer identifier")

	token, canonical, err = valid.resolveTransferToken(t.Context(), "HYPE:0X96")
	require.NoError(t, err, "resolveTransferToken must not error for an exact spot token")
	assert.Equal(t, uint64(150), token.Index, "token.Index: spot token should retain its token index")
	assert.Equal(t, "HYPE:0x96", canonical, "canonical: spot token identifier should use metadata casing")

	for _, invalid := range []string{"HYPE", ":0x96", "HYPE:", "MISSING:0x1"} {
		_, _, err := valid.resolveTransferToken(t.Context(), invalid)
		require.ErrorIs(t, err, errTransferTokenInvalid, "resolveTransferToken must return the expected error for invalid token identifier")
	}

	missingUSDC := newStaticInfoExchange(t, map[string]string{"spotMeta": `{"tokens":[{"name":"HYPE","index":150}]}`})
	_, _, err = missingUSDC.resolveTransferToken(t.Context(), "USDC")
	require.ErrorIs(t, err, errTransferTokenInvalid, "resolveTransferToken must return the expected error for missing USDC metadata")
}

func TestResolveTransferDEX(t *testing.T) {
	ex := new(Exchange)
	dex, err := ex.resolveTransferDEX(t.Context(), " ")
	require.NoError(t, err, "resolveTransferDEX must not error for the default DEX")
	assert.Empty(t, dex, "dex: default DEX should remain empty")
	dex, err = ex.resolveTransferDEX(t.Context(), " SPOT ")
	require.NoError(t, err, "resolveTransferDEX must not error for the spot balance")
	assert.Equal(t, "spot", dex, "dex: spot DEX should be canonicalised")

	failing := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = failing.resolveTransferDEX(t.Context(), "xyz")
	require.Error(t, err, "resolveTransferDEX must return DEX registry failure")

	valid := newStaticInfoExchange(t, map[string]string{infoTypePerpetualDEXs: `[null,null,{"name":"xyz"}]`})
	dex, err = valid.resolveTransferDEX(t.Context(), "xyz")
	require.NoError(t, err, "resolveTransferDEX must not error for a registered builder DEX")
	assert.Equal(t, "xyz", dex, "dex: builder DEX name should be retained")
	_, err = valid.resolveTransferDEX(t.Context(), "missing")
	require.ErrorIs(t, err, errTransferDEXInvalid, "resolveTransferDEX must return the expected error for unknown builder DEX")
}

func TestValidateSendAssetRoute(t *testing.T) {
	badRegistry := newStaticInfoExchange(t, map[string]string{infoTypePerpetualDEXs: `[null]`})
	sourceDEX, destinationDEX, token, err := badRegistry.validateSendAssetRoute(t.Context(), "missing", "spot", "USDC")
	require.ErrorIs(t, err, errTransferDEXInvalid, "validateSendAssetRoute must return the expected error for invalid source DEX")
	assert.Empty(t, sourceDEX, "sourceDEX: invalid source route should not return a source DEX")
	assert.Empty(t, destinationDEX, "destinationDEX: invalid source route should not return a destination DEX")
	assert.Empty(t, token, "token: invalid source route should not return a token")

	sourceDEX, destinationDEX, token, err = badRegistry.validateSendAssetRoute(t.Context(), "", "missing", "USDC")
	require.ErrorIs(t, err, errTransferDEXInvalid, "validateSendAssetRoute must return the expected error for invalid destination DEX")
	assert.Empty(t, sourceDEX, "sourceDEX: invalid destination route should not return a source DEX")
	assert.Empty(t, destinationDEX, "destinationDEX: invalid destination route should not return a destination DEX")
	assert.Empty(t, token, "token: invalid destination route should not return a token")

	missingToken := newStaticInfoExchange(t, map[string]string{"spotMeta": `{"tokens":[]}`})
	sourceDEX, destinationDEX, token, err = missingToken.validateSendAssetRoute(t.Context(), "spot", "spot", "USDC")
	require.ErrorIs(t, err, errTransferTokenInvalid, "validateSendAssetRoute must return the expected error for invalid transfer token")
	assert.Empty(t, sourceDEX, "sourceDEX: invalid-token route should not return a source DEX")
	assert.Empty(t, destinationDEX, "destinationDEX: invalid-token route should not return a destination DEX")
	assert.Empty(t, token, "token: invalid-token route should not return a token")

	metadataFailure := newTransferTestExchange(t, nil, map[string]string{
		"spotMeta": spotMetadataJSON,
		"meta":     `null`,
	}, nil)
	sourceDEX, destinationDEX, token, err = metadataFailure.validateSendAssetRoute(t.Context(), "", "spot", "USDC")
	require.ErrorIs(t, err, common.ErrNilPointer, "validateSendAssetRoute must return perpetual metadata failure")
	assert.Empty(t, sourceDEX, "sourceDEX: metadata-failure route should not return a source DEX")
	assert.Empty(t, destinationDEX, "destinationDEX: metadata-failure route should not return a destination DEX")
	assert.Empty(t, token, "token: metadata-failure route should not return a token")

	wrongCollateral := newTransferTestExchange(t, nil, map[string]string{
		"spotMeta": spotMetadataJSON,
		"meta":     `{"collateralToken":150}`,
	}, nil)
	sourceDEX, destinationDEX, token, err = wrongCollateral.validateSendAssetRoute(t.Context(), "spot", "", "USDC")
	require.ErrorIs(t, err, errTransferTokenInvalid, "validateSendAssetRoute must return the expected error for non-collateral token")
	assert.Empty(t, sourceDEX, "sourceDEX: wrong-collateral route should not return a source DEX")
	assert.Empty(t, destinationDEX, "destinationDEX: wrong-collateral route should not return a destination DEX")
	assert.Empty(t, token, "token: wrong-collateral route should not return a token")

	valid := newTransferTestExchange(t, nil, map[string]string{
		infoTypePerpetualDEXs: `[null,{"name":"xyz"}]`,
		"spotMeta":            spotMetadataJSON,
		"meta":                `{"collateralToken":0}`,
		"meta:xyz":            `{"collateralToken":0}`,
	}, nil)
	sourceDEX, destinationDEX, token, err = valid.validateSendAssetRoute(t.Context(), "", "xyz", "USDC")
	require.NoError(t, err, "validateSendAssetRoute must not error for valid default-to-builder collateral route")
	assert.Empty(t, sourceDEX, "sourceDEX: default DEX should remain empty")
	assert.Equal(t, "xyz", destinationDEX, "destinationDEX: destination builder DEX should be retained")
	assert.Equal(t, "USDC", token, "token: validated collateral token should be retained")

	sourceDEX, destinationDEX, token, err = valid.validateSendAssetRoute(t.Context(), "spot", "spot", "HYPE:0x96")
	require.NoError(t, err, "validateSendAssetRoute must not error for valid spot-to-spot route")
	assert.Equal(t, "spot", sourceDEX, "sourceDEX: spot source should be retained")
	assert.Equal(t, "spot", destinationDEX, "destinationDEX: spot destination should be retained")
	assert.Equal(t, "HYPE:0x96", token, "token: validated spot token should be retained")
}

func TestTransferUSDCBetweenSpotAndPerp(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, err := ex.TransferUSDCBetweenSpotAndPerp(t.Context(), 0, true)
	require.ErrorIs(t, err, errTransferAmountInvalid, "TransferUSDCBetweenSpotAndPerp must return the expected error for invalid class-transfer amount")
	_, err = ex.TransferUSDCBetweenSpotAndPerp(t.Context(), 1, true)
	require.Error(t, err, "TransferUSDCBetweenSpotAndPerp must error for class transfer without credentials")

	invalidSubAccount := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: testVaultAddress,
	}, map[string]string{"userRole:" + testVaultAddress: `{"role":"vault"}`}, nil)
	_, err = invalidSubAccount.TransferUSDCBetweenSpotAndPerp(t.Context(), 1, true)
	require.ErrorIs(t, err, errTransferSubAccountInvalid, "TransferUSDCBetweenSpotAndPerp must reject class transfer from a vault")

	var captured signedActionRequest
	success := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: testVaultAddress,
	}, map[string]string{
		"userRole:" + testVaultAddress: `{"role":"subAccount","data":{"master":"` + officialSigningAddress + `"}}`,
	}, &captured)
	nonce, err := success.TransferUSDCBetweenSpotAndPerp(t.Context(), 1.23, true)
	require.NoError(t, err, "TransferUSDCBetweenSpotAndPerp must not error for owned-subaccount class transfer")
	action := getCapturedAction(t, &captured)
	assert.Equal(t, "usdClassTransfer", action["type"], "action[\"type\"]: class-transfer action type should match")
	assert.Equal(t, "1.23 subaccount:"+testVaultAddress, action["amount"], "action[\"amount\"]: class-transfer amount should identify the subaccount")
	assert.Equal(t, true, action["toPerp"], "action[\"toPerp\"]: class-transfer direction should be retained")
	assert.Equal(t, float64(nonce), action["nonce"], "action[\"nonce\"]: class-transfer action nonce should match the outer nonce")
}

func TestSendAsset(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, err := ex.SendAsset(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "SendAsset must return the expected error for nil asset transfer")
	_, err = ex.SendAsset(t.Context(), &SendAssetRequest{Destination: "invalid", Amount: 1})
	require.ErrorIs(t, err, errInvalidAddress, "SendAsset must return the expected error for invalid asset-transfer destination")
	_, err = ex.SendAsset(t.Context(), &SendAssetRequest{Destination: testOtherAddress})
	require.ErrorIs(t, err, errTransferAmountInvalid, "SendAsset must return the expected error for invalid asset-transfer amount")
	_, err = ex.SendAsset(t.Context(), &SendAssetRequest{Destination: testOtherAddress, Amount: 1})
	require.Error(t, err, "SendAsset must error for asset transfer without credentials")

	invalidSubAccount := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: testVaultAddress,
	}, map[string]string{"userRole:" + testVaultAddress: `{"role":"vault"}`}, nil)
	_, err = invalidSubAccount.SendAsset(t.Context(), &SendAssetRequest{
		Destination: testOtherAddress, SourceDEX: "spot", DestinationDEX: "spot", Token: "USDC", Amount: 1,
	})
	require.ErrorIs(t, err, errTransferSubAccountInvalid, "SendAsset must reject asset transfer from a vault")

	invalidRoute := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey,
	}, map[string]string{infoTypePerpetualDEXs: `[null]`}, nil)
	_, err = invalidRoute.SendAsset(t.Context(), &SendAssetRequest{
		Destination: testOtherAddress, SourceDEX: "missing", DestinationDEX: "spot", Token: "USDC", Amount: 1,
	})
	require.ErrorIs(t, err, errTransferDEXInvalid, "SendAsset must return the expected error for invalid asset-transfer route")

	var captured signedActionRequest
	success := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: testVaultAddress,
	}, map[string]string{
		"userRole:" + testVaultAddress: `{"role":"subAccount","data":{"master":"` + officialSigningAddress + `"}}`,
		"spotMeta":                     spotMetadataJSON,
	}, &captured)
	nonce, err := success.SendAsset(t.Context(), &SendAssetRequest{
		Destination: strings.ToUpper(testOtherAddress),
		SourceDEX:   "SPOT", DestinationDEX: "spot",
		Token: "HYPE:0X96", Amount: 1.25,
	})
	require.NoError(t, err, "SendAsset must not error for validated asset transfer")
	action := getCapturedAction(t, &captured)
	assert.Equal(t, "sendAsset", action["type"], "action[\"type\"]: asset-transfer action type should match")
	assert.Equal(t, testOtherAddress, action["destination"], "action[\"destination\"]: asset-transfer destination should be normalised")
	assert.Equal(t, "spot", action["sourceDex"], "action[\"sourceDex\"]: asset-transfer source should be canonicalised")
	assert.Equal(t, "spot", action["destinationDex"], "action[\"destinationDex\"]: asset-transfer destination DEX should be canonicalised")
	assert.Equal(t, "HYPE:0x96", action["token"], "action[\"token\"]: asset-transfer token should use metadata casing")
	assert.Equal(t, "1.25", action["amount"], "action[\"amount\"]: asset-transfer amount should use wire formatting")
	assert.Equal(t, testVaultAddress, action["fromSubAccount"], "action[\"fromSubAccount\"]: asset transfer should identify its owned subaccount source")
	assert.Equal(t, float64(nonce), action["nonce"], "action[\"nonce\"]: asset-transfer action nonce should match the outer nonce")
}

func TestSendCoreUSDC(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, err := ex.SendCoreUSDC(t.Context(), "invalid", 1)
	require.ErrorIs(t, err, errInvalidAddress, "SendCoreUSDC must return the expected error for invalid USDC-send destination")
	_, err = ex.SendCoreUSDC(t.Context(), testOtherAddress, 0)
	require.ErrorIs(t, err, errTransferAmountInvalid, "SendCoreUSDC must return the expected error for invalid USDC-send amount")
	_, err = ex.SendCoreUSDC(t.Context(), testOtherAddress, 1)
	require.Error(t, err, "SendCoreUSDC must error for USDC send without credentials")

	subAccount := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: testVaultAddress,
	}, nil, nil)
	_, err = subAccount.SendCoreUSDC(t.Context(), testOtherAddress, 1)
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "SendCoreUSDC must reject USDC send with a configured subaccount")

	var captured signedActionRequest
	success := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey,
	}, nil, &captured)
	nonce, err := success.SendCoreUSDC(t.Context(), strings.ToUpper(testOtherAddress), 1.5)
	require.NoError(t, err, "SendCoreUSDC must not error for valid Core USDC send")
	action := getCapturedAction(t, &captured)
	assert.Equal(t, "usdSend", action["type"], "action[\"type\"]: USDC-send action type should match")
	assert.Equal(t, testOtherAddress, action["destination"], "action[\"destination\"]: USDC-send destination should be normalised")
	assert.Equal(t, "1.5", action["amount"], "action[\"amount\"]: USDC-send amount should use wire formatting")
	assert.Equal(t, float64(nonce), action["time"], "action[\"time\"]: USDC-send time should match the outer nonce")
}

func TestSendCoreSpot(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, err := ex.SendCoreSpot(t.Context(), "invalid", "HYPE:0x96", 1)
	require.ErrorIs(t, err, errInvalidAddress, "SendCoreSpot must return the expected error for invalid spot-send destination")
	_, err = ex.SendCoreSpot(t.Context(), testOtherAddress, "HYPE:0x96", 0)
	require.ErrorIs(t, err, errTransferAmountInvalid, "SendCoreSpot must return the expected error for invalid spot-send amount")
	_, err = ex.SendCoreSpot(t.Context(), testOtherAddress, "HYPE:0x96", 1)
	require.Error(t, err, "SendCoreSpot must error for spot send without credentials")

	subAccount := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: testVaultAddress,
	}, nil, nil)
	_, err = subAccount.SendCoreSpot(t.Context(), testOtherAddress, "HYPE:0x96", 1)
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "SendCoreSpot must reject spot send with a configured subaccount")

	missingToken := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey,
	}, map[string]string{"spotMeta": `{"tokens":[]}`}, nil)
	_, err = missingToken.SendCoreSpot(t.Context(), testOtherAddress, "HYPE:0x96", 1)
	require.ErrorIs(t, err, errTransferTokenInvalid, "SendCoreSpot must return the expected error for unknown spot-send token")

	var captured signedActionRequest
	success := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey,
	}, map[string]string{"spotMeta": spotMetadataJSON}, &captured)
	nonce, err := success.SendCoreSpot(t.Context(), testOtherAddress, "HYPE:0X96", 2)
	require.NoError(t, err, "SendCoreSpot must not error for valid Core spot send")
	action := getCapturedAction(t, &captured)
	assert.Equal(t, "spotSend", action["type"], "action[\"type\"]: spot-send action type should match")
	assert.Equal(t, "HYPE:0x96", action["token"], "action[\"token\"]: spot-send token should use metadata casing")
	assert.Equal(t, "2", action["amount"], "action[\"amount\"]: spot-send amount should use wire formatting")
	assert.Equal(t, float64(nonce), action["time"], "action[\"time\"]: spot-send time should match the outer nonce")

	_, err = success.SendCoreSpot(t.Context(), testOtherAddress, "USDC", 1)
	require.NoError(t, err, "SendCoreSpot must not error for valid Core spot USDC send")
	assert.Equal(t, "USDC:0x0", getCapturedAction(t, &captured)["token"], "captured: core spot USDC should use its full token identifier")
}

func TestWithdrawFromBridge(t *testing.T) {
	ex := new(Exchange)
	ex.SetDefaults()
	_, err := ex.WithdrawFromBridge(t.Context(), "invalid", 1)
	require.ErrorIs(t, err, errInvalidAddress, "WithdrawFromBridge must return the expected error for invalid bridge-withdrawal destination")
	_, err = ex.WithdrawFromBridge(t.Context(), testOtherAddress, 0)
	require.ErrorIs(t, err, errTransferAmountInvalid, "WithdrawFromBridge must return the expected error for invalid bridge-withdrawal amount")
	_, err = ex.WithdrawFromBridge(t.Context(), testOtherAddress, 1)
	require.Error(t, err, "WithdrawFromBridge must error for bridge withdrawal without credentials")

	subAccount := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey, SubAccount: testVaultAddress,
	}, nil, nil)
	_, err = subAccount.WithdrawFromBridge(t.Context(), testOtherAddress, 1)
	require.ErrorIs(t, err, errTransferSubAccountUnsupported, "WithdrawFromBridge must reject bridge withdrawal with a configured subaccount")

	var captured signedActionRequest
	success := newTransferTestExchange(t, &accounts.Credentials{
		Key: officialSigningAddress, Secret: officialSigningTestKey,
	}, nil, &captured)
	nonce, err := success.WithdrawFromBridge(t.Context(), strings.ToUpper(testOtherAddress), 2)
	require.NoError(t, err, "WithdrawFromBridge must not error for valid bridge withdrawal")
	action := getCapturedAction(t, &captured)
	assert.Equal(t, "withdraw3", action["type"], "action[\"type\"]: bridge-withdrawal action type should match")
	assert.Equal(t, testOtherAddress, action["destination"], "action[\"destination\"]: bridge-withdrawal destination should be normalised")
	assert.Equal(t, "2", action["amount"], "action[\"amount\"]: bridge-withdrawal amount should use wire formatting")
	assert.Equal(t, float64(nonce), action["time"], "action[\"time\"]: bridge-withdrawal time should match the outer nonce")
}

func TestGetUserNonFundingLedgerUpdates(t *testing.T) {
	start := time.UnixMilli(1700000000000).UTC()
	end := start.Add(time.Hour)
	var got infoRequest
	ex := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&got), "Decode should not error for ledger-history request") {
			return
		}
		_, err := w.Write([]byte(`[{"time":1700000000000,"hash":"0x1","delta":{"type":"withdraw","usdc":"2","fee":"1","nonce":7}}]`))
		assert.NoError(t, err, "Write should not error for ledger-history response")
	}))
	_, err := ex.GetUserNonFundingLedgerUpdates(t.Context(), "invalid", start, end)
	require.ErrorIs(t, err, errInvalidAddress, "GetUserNonFundingLedgerUpdates must return the expected error for invalid ledger address")
	_, err = ex.GetUserNonFundingLedgerUpdates(t.Context(), officialSigningAddress, end, start)
	require.ErrorIs(t, err, common.ErrStartAfterEnd, "GetUserNonFundingLedgerUpdates must return the expected error for invalid ledger range")

	result, err := ex.GetUserNonFundingLedgerUpdates(t.Context(), strings.ToUpper(officialSigningAddress), start, end)
	require.NoError(t, err, "GetUserNonFundingLedgerUpdates must not error for valid ledger history")
	require.Len(t, result, 1, "GetUserNonFundingLedgerUpdates must decode one record")
	assert.Equal(t, "userNonFundingLedgerUpdates", got.Type, "got.Type: ledger request type should match")
	assert.Equal(t, officialSigningAddress, got.User, "got.User: ledger address should be normalised")
	assert.Equal(t, start.UnixMilli(), got.StartTime, "got.StartTime: ledger start time should be serialised")
	assert.Equal(t, end.UnixMilli(), got.EndTime, "got.EndTime: ledger end time should be serialised")
	assert.Equal(t, "withdraw", result[0].Delta.Type, "result[0].Delta.Type: ledger delta type should be decoded")
	assert.Equal(t, 2.0, result[0].Delta.USDC.Float64(), "Float64: ledger amount should be decoded")
	assert.Equal(t, uint64(7), result[0].Delta.Nonce, "result[0].Delta.Nonce: ledger nonce should be decoded")

	errorExchange := newHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	_, err = errorExchange.GetUserNonFundingLedgerUpdates(t.Context(), officialSigningAddress, start, end)
	require.Error(t, err, "GetUserNonFundingLedgerUpdates must error for ledger history from a failing server")
}
