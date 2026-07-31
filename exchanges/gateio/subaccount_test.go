package gateio

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
)

func skipTestIfSubAccountUserIDUnset(t *testing.T) {
	t.Helper()
	if !mockTests && subAccountUserID == 0 {
		t.Skip("live sub-account user ID is not configured")
	}
}

func skipTestIfSubAccountAPIKeyUnset(t *testing.T) {
	t.Helper()
	if !mockTests && subAccountKeyIdentifier == "" {
		t.Skip("live sub-account API key is not configured")
	}
}

func TestListSubAccounts(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	_, err := e.ListSubAccounts(t.Context(), 0)
	require.NoError(t, err)

	_, err = e.ListSubAccounts(t.Context(), 1)
	require.NoError(t, err)
}

func TestCreateSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.CreateSubAccount(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.CreateSubAccount(t.Context(), &CreateSubAccountRequest{})
	require.ErrorIs(t, err, errInvalidSubAccount)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
		if subAccountLoginName == "" {
			t.Skip("live sub-account login name is not configured")
		}
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedJSONRouteTestExchange(t, http.MethodPost, "/api/v4/sub_accounts", `{"login_name":"test_sub_account_001"}`, `{"login_name":"test_sub_account_001","user_id":12345678}`)
	}
	result, err := ex.CreateSubAccount(t.Context(), &CreateSubAccountRequest{
		LoginName: subAccountLoginName,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestGetSubAccount(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccount(t.Context(), 0)
	require.ErrorIs(t, err, errInvalidSubAccountUserID)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	skipTestIfSubAccountUserIDUnset(t)
	_, err = e.GetSubAccount(t.Context(), subAccountUserID)
	require.NoError(t, err)
}

func TestListSubAccountAPIKeys(t *testing.T) {
	t.Parallel()
	_, err := e.ListSubAccountAPIKeys(t.Context(), 0)
	require.ErrorIs(t, err, errInvalidSubAccountUserID)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	skipTestIfSubAccountUserIDUnset(t)
	result, err := e.ListSubAccountAPIKeys(t.Context(), subAccountUserID)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestCreateSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.CreateSubAccountAPIKey(t.Context(), 0, &SubAccountKeyRequest{})
	require.ErrorIs(t, err, errInvalidSubAccountUserID)

	_, err = e.CreateSubAccountAPIKey(t.Context(), 12345678, nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
		if subAccountAPIKeyName == "" {
			t.Skip("live sub-account API key name is not configured")
		}
	}
	skipTestIfSubAccountUserIDUnset(t)
	ex := e
	if mockTests {
		ex = newAuthenticatedJSONRouteTestExchange(t, http.MethodPost, "/api/v4/sub_accounts/12345678/keys", `{"name":"test_key"}`, `{"user_id":12345678,"name":"test_key","key":"mock-subaccount-key-id"}`)
	}
	result, err := ex.CreateSubAccountAPIKey(t.Context(), subAccountUserID, &SubAccountKeyRequest{
		Name: subAccountAPIKeyName,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestGetSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	_, err := e.GetSubAccountAPIKey(t.Context(), 0, "mock-subaccount-key-id")
	require.ErrorIs(t, err, errInvalidSubAccountUserID)

	_, err = e.GetSubAccountAPIKey(t.Context(), 12345678, "")
	require.ErrorIs(t, err, errMissingAPIKey)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	skipTestIfSubAccountUserIDUnset(t)
	skipTestIfSubAccountAPIKeyUnset(t)
	_, err = e.GetSubAccountAPIKey(t.Context(), subAccountUserID, subAccountKeyIdentifier)
	require.NoError(t, err)
}

func TestUpdateSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	err := e.UpdateSubAccountAPIKey(t.Context(), 0, "mock-subaccount-key-id", &SubAccountKeyRequest{})
	require.ErrorIs(t, err, errInvalidSubAccountUserID)

	err = e.UpdateSubAccountAPIKey(t.Context(), 12345678, "", &SubAccountKeyRequest{})
	require.ErrorIs(t, err, errMissingAPIKey)

	err = e.UpdateSubAccountAPIKey(t.Context(), 12345678, "mock-subaccount-key-id", nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
		if subAccountUpdatedAPIKeyName == "" {
			t.Skip("updated live sub-account API key name is not configured")
		}
	}
	skipTestIfSubAccountUserIDUnset(t)
	skipTestIfSubAccountAPIKeyUnset(t)
	ex := e
	if mockTests {
		ex = newAuthenticatedJSONRouteTestExchange(t, http.MethodPut, "/api/v4/sub_accounts/12345678/keys/mock-subaccount-key-id", `{"name":"updated_key","perms":[{"name":"wallet","read_only":true}]}`, `{}`)
	}
	err = ex.UpdateSubAccountAPIKey(t.Context(), subAccountUserID, subAccountKeyIdentifier, &SubAccountKeyRequest{
		Name: subAccountUpdatedAPIKeyName,
		Permissions: []*SubAccountKeyPerm{
			{Name: "wallet", ReadOnly: true},
		},
	})
	require.NoError(t, err)
}

func TestDeleteSubAccountAPIKey(t *testing.T) {
	t.Parallel()
	err := e.DeleteSubAccountAPIKey(t.Context(), 0, "mock-subaccount-key-id")
	require.ErrorIs(t, err, errInvalidSubAccountUserID)

	err = e.DeleteSubAccountAPIKey(t.Context(), 12345678, "")
	require.ErrorIs(t, err, errMissingAPIKey)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	skipTestIfSubAccountUserIDUnset(t)
	skipTestIfSubAccountAPIKeyUnset(t)
	err = e.DeleteSubAccountAPIKey(t.Context(), subAccountUserID, subAccountKeyIdentifier)
	require.NoError(t, err)
}

func TestLockSubAccount(t *testing.T) {
	t.Parallel()
	err := e.LockSubAccount(t.Context(), 0)
	require.ErrorIs(t, err, errInvalidSubAccountUserID)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	skipTestIfSubAccountUserIDUnset(t)
	err = e.LockSubAccount(t.Context(), subAccountUserID)
	require.NoError(t, err)
}

func TestUnlockSubAccount(t *testing.T) {
	t.Parallel()
	err := e.UnlockSubAccount(t.Context(), 0)
	require.ErrorIs(t, err, errInvalidSubAccountUserID)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	skipTestIfSubAccountUserIDUnset(t)
	err = e.UnlockSubAccount(t.Context(), subAccountUserID)
	require.NoError(t, err)
}

func TestGetSubAccountMode(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err := e.GetSubAccountMode(t.Context())
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}
