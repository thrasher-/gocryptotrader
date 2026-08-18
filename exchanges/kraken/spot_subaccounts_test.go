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

func TestCreateSubaccount(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotSubaccountsFixtures)
	ctx := t.Context()

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

	created, err = newSpotErrorExchange(t).CreateSubaccount(ctx, &CreateSubaccountRequest{Username: "subaccount", Email: "subaccount@example.com"})
	require.ErrorIs(t, err, errSpotTransport, "CreateSubaccount must surface request errors")
	assert.False(t, created, "CreateSubaccount result should remain false on request errors")
}

func TestAccountTransfer(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotSubaccountsFixtures)
	ctx := t.Context()
	amount := 1.0

	_, err := ex.AccountTransfer(ctx, nil)
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
	require.NotNil(t, accountTransfer, "AccountTransfer must decode a non-null result")
	assert.Equal(t, "complete", accountTransfer.Status, "AccountTransfer should decode status")
	responseJSON, err := json.Marshal(accountTransfer)
	require.NoError(t, err, "AccountTransfer must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"transfer_id":"TRANSFER"`, "AccountTransfer should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/AccountTransfer")
	assert.Equal(t, "currency", values.Get("asset_class"), "AccountTransfer should encode asset class")
	assert.Equal(t, "1", values.Get("amount"), "AccountTransfer should encode amount")

	accountTransfer, err = newSpotNullResultExchange(t).AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: 1, From: "PRIMARY", To: "SUB"})
	require.NoError(t, err, "AccountTransfer must accept a null result")
	assert.Nil(t, accountTransfer, "AccountTransfer should return nil for a null result")
	accountTransfer, err = newSpotErrorExchange(t).AccountTransfer(ctx, &AccountTransferRequest{Asset: "XBT", Amount: 1, From: "PRIMARY", To: "SUB"})
	require.ErrorIs(t, err, errSpotTransport, "AccountTransfer must surface request errors")
	assert.Nil(t, accountTransfer, "AccountTransfer result should remain nil on request errors")
}
