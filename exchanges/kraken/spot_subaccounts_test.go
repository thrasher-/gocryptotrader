package kraken

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

var spotSubaccountsFixtures = spotFixtureSet{results: map[string]string{
	"/0/private/CreateSubaccount": `true`,
	"/0/private/AccountTransfer":  `{"transfer_id":"TRANSFER","status":"complete"}`,
}}

func TestSpotSubaccountsEndpointErrors(t *testing.T) {
	ex := newSpotErrorExchange(t)
	ctx := t.Context()

	_, err := ex.CreateSubaccount(ctx, &CreateSubaccountRequest{Username: "subaccount", Email: "subaccount@example.com"})
	require.Error(t, err, "CreateSubaccount must surface request errors")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: 1, From: "PRIMARY", To: "SUB"})
	require.Error(t, err, "AccountTransfer must surface request errors")
}

func TestSpotSubaccountsResponseObjectContract(t *testing.T) {
	successEx, _ := newSpotEndpointExchange(t, spotSubaccountsFixtures)
	nilResultEx := newSpotNullResultExchange(t)
	errorEx := newSpotErrorExchange(t)
	ctx := t.Context()

	for _, tc := range []struct {
		name         string
		call         func(*Exchange) (any, error)
		expectedJSON string
	}{
		{
			name: "AccountTransfer",
			call: func(ex *Exchange) (any, error) {
				return ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: 1, From: "PRIMARY", To: "SUB"})
			},
			expectedJSON: `"transfer_id":"TRANSFER"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call(successEx)
			require.NoError(t, err, tc.name+" must not error")
			require.NotNil(t, result, tc.name+" must return a response")
			responseJSON, err := json.Marshal(result)
			require.NoError(t, err, tc.name+" must encode the decoded response")
			assert.Contains(t, string(responseJSON), tc.expectedJSON, tc.name+" should decode the response")

			result, err = tc.call(nilResultEx)
			require.NoError(t, err, tc.name+" must accept a null result")
			assert.Nil(t, result, tc.name+" should return nil for a null result")

			result, err = tc.call(errorEx)
			require.ErrorIs(t, err, errSpotTransport, tc.name+" must surface request errors")
			assert.Nil(t, result, tc.name+" result should remain nil on request errors")
		})
	}
}

func TestSpotSubaccountsEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotSubaccountsFixtures)
	ctx := t.Context()
	amount := 1.0

	_, err := ex.CreateSubaccount(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CreateSubaccount must reject a nil request")
	_, err = ex.CreateSubaccount(ctx, &CreateSubaccountRequest{})
	require.ErrorIs(t, err, errUsernameRequired, "CreateSubaccount must require a username")
	_, err = ex.CreateSubaccount(ctx, &CreateSubaccountRequest{Username: "subaccount"})
	require.ErrorIs(t, err, errEmailRequired, "CreateSubaccount must require an email address")
	created, err := ex.CreateSubaccount(ctx, &CreateSubaccountRequest{Username: "subaccount", Email: "subaccount@example.com"})
	require.NoError(t, err, "CreateSubaccount must not error")
	assert.True(t, created, "CreateSubaccount should decode success")
	requireSpotRequest(t, requests, "/0/private/CreateSubaccount")

	_, err = ex.AccountTransfer(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "AccountTransfer must reject a nil request")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{})
	require.ErrorIs(t, err, errAssetRequired, "AccountTransfer must require an asset")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errAmountInvalid, "AccountTransfer must require a positive amount")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: amount})
	require.ErrorIs(t, err, errFromRequired, "AccountTransfer must require a source account")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: amount, From: "PRIMARY"})
	require.ErrorIs(t, err, errToRequired, "AccountTransfer must require a destination account")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", AssetClass: "invalid", Amount: amount, From: "PRIMARY", To: "SUB"})
	require.ErrorIs(t, err, errAssetClassInvalid, "AccountTransfer must reject an invalid asset class")
	_, err = ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: math.NaN(), From: "PRIMARY", To: "SUB"})
	require.ErrorIs(t, err, errNumericValueInvalid, "AccountTransfer must reject a non-finite amount")
	accountTransfer, err := ex.AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", AssetClass: AssetClassCurrency, Amount: amount, From: "PRIMARY", To: "SUB"})
	require.NoError(t, err, "AccountTransfer must not error")
	assert.Equal(t, "complete", accountTransfer.Status, "AccountTransfer should decode status")
	values := requireSpotRequest(t, requests, "/0/private/AccountTransfer")
	assert.Equal(t, "currency", values.Get("asset_class"), "AccountTransfer should encode asset class")
	assert.Equal(t, "1", values.Get("amount"), "AccountTransfer should encode amount")
}
