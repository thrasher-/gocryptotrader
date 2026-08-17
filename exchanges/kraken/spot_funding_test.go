package kraken

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

var spotFundingFixtures = spotFixtureSet{results: map[string]string{
	"/0/private/DepositMethods":    `[{"method":"Bitcoin","limit":false,"fee":"0.0001","fee-percentage":"0.1","address-setup-fee":"0","gen-address":true,"minimum":"0.001"},{"method":"SynapsePay (US Wire)","limit":false,"fee":"5","fee-percentage":"0","address-setup-fee":"0","gen-address":false,"minimum":"1"}]`,
	"/0/private/DepositAddresses":  `[{"address":"bc1q","expiretm":"0","new":true,"tag":"TAG","memo":"MEMO"}]`,
	"/0/private/DepositStatus":     `{"deposit":[{"method":"Bitcoin","aclass":"currency","asset":"XXBT","refid":"REF","txid":"TX","amount":"1","fee":"0","time":1695828271,"status":"Success"}],"next_cursor":"NEXT"}`,
	"/0/private/WithdrawInfo":      `{"method":"Bitcoin","limit":"10","amount":"0.9","fee":"0.1"}`,
	"/0/private/Withdraw":          `{"refid":"WITHDRAWAL"}`,
	"/0/private/WithdrawCancel":    `true`,
	"/0/private/WithdrawStatus":    `{"withdrawal":[{"method":"Bitcoin","network":"Bitcoin","aclass":"currency","asset":"XXBT","refid":"REF","txid":"TX","info":"bc1q","amount":"1","fee":"0.1","time":1695828271,"status":"Success","key":"wallet"}],"next_cursor":"NEXT"}`,
	"/0/private/WithdrawMethods":   `[{"asset":"XXBT","method":"Bitcoin","method_id":"METHOD","network":"Bitcoin","network_id":"NETWORK","minimum":"0.0004","fee":{"aclass":"currency","asset":"XXBT","fee":"0.00001","fee_percentage":"0.1"},"limits":[{"description":"daily","limit_type":"amount","limits":{"86400":{"maximum":"10","remaining":"8","used":"2"}}}]}]`,
	"/0/private/WithdrawAddresses": `[{"address":"bc1q","asset":"XBT","method":"Bitcoin","key":"wallet","tag":"","verified":true}]`,
	"/0/private/WalletTransfer":    `{"refid":"TRANSFER"}`,
}}

func TestSpotFundingEndpointErrors(t *testing.T) {
	ex := newSpotErrorExchange(t)
	ctx := t.Context()

	_, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{})
	require.Error(t, err, "GetRecentDepositsStatus must surface request errors")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT"})
	require.Error(t, err, "GetDepositMethods must surface request errors")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin"})
	require.Error(t, err, "GetDepositAddresses must surface request errors")
	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet", Amount: 1})
	require.Error(t, err, "GetWithdrawalInformation must surface request errors")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: 1})
	require.Error(t, err, "WithdrawFunds must surface request errors")
	_, err = ex.CancelWithdrawal(ctx, &CancelWithdrawalRequest{Asset: "BTC", ReferenceID: "REFERENCE"})
	require.Error(t, err, "CancelWithdrawal must surface request errors")
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{})
	require.Error(t, err, "GetRecentWithdrawalsStatus must surface request errors")
	_, err = ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{})
	require.Error(t, err, "GetWithdrawalMethods must surface request errors")
	_, err = ex.GetWithdrawalAddresses(ctx, &GetWithdrawalAddressesRequest{})
	require.Error(t, err, "GetWithdrawalAddresses must surface request errors")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures, Amount: 1})
	require.Error(t, err, "WalletTransfer must surface request errors")
}

func TestSpotFundingResponseObjectContract(t *testing.T) {
	successEx, _ := newSpotEndpointExchange(t, spotFundingFixtures)
	nilResultEx := newSpotNullResultExchange(t)
	errorEx := newSpotErrorExchange(t)
	ctx := t.Context()

	for _, tc := range []struct {
		name         string
		call         func(*Exchange) (any, error)
		expectedJSON string
	}{
		{
			name: "GetWithdrawalInformation",
			call: func(ex *Exchange) (any, error) {
				return ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet", Amount: 1})
			},
			expectedJSON: `"method":"Bitcoin"`,
		},
		{
			name: "WithdrawFunds",
			call: func(ex *Exchange) (any, error) {
				return ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: 1})
			},
			expectedJSON: `"refid":"WITHDRAWAL"`,
		},
		{
			name: "GetRecentDepositsStatus",
			call: func(ex *Exchange) (any, error) {
				return ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{})
			},
			expectedJSON: `"NextCursor":"NEXT"`,
		},
		{
			name: "GetRecentWithdrawalsStatus",
			call: func(ex *Exchange) (any, error) {
				return ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{})
			},
			expectedJSON: `"NextCursor":"NEXT"`,
		},
		{
			name: "WalletTransfer",
			call: func(ex *Exchange) (any, error) {
				return ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures, Amount: 1})
			},
			expectedJSON: `"refid":"TRANSFER"`,
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

func TestSpotFundingEndpoints(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotFundingFixtures)
	ctx := t.Context()
	amount := 1.0
	maximumFee := 0.1
	start := time.Unix(1, 0)
	end := time.Unix(2, 0)

	_, err := ex.GetDepositMethods(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetDepositMethods must reject a nil request")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{})
	require.ErrorIs(t, err, errAssetRequired, "GetDepositMethods must require an asset")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT", AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetDepositMethods must reject an invalid asset class")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT", RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetDepositMethods must reject an invalid rebase multiplier")
	depositMethods, err := ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT", AssetClass: "tokenized_asset", RebaseMultiplier: "base"})
	require.NoError(t, err, "GetDepositMethods must not error")
	assert.True(t, depositMethods[0].GeneratesAddress, "GetDepositMethods should decode address generation support")
	assert.Equal(t, 0.1, depositMethods[0].FeePercent.Float64(), "GetDepositMethods should decode percentage fees")
	values := requireSpotRequest(t, requests, "/0/private/DepositMethods")
	assert.Equal(t, "tokenized_asset", values.Get("aclass"), "GetDepositMethods should encode asset class")
	assert.Equal(t, "base", values.Get("rebase_multiplier"), "GetDepositMethods should encode rebase multiplier")
	_, err = ex.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: "XBT"})
	require.NoError(t, err, "GetDepositMethods must allow omitted optional parameters")
	requireSpotRequest(t, requests, "/0/private/DepositMethods")

	_, err = ex.GetDepositAddresses(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetDepositAddresses must reject a nil request")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{})
	require.ErrorIs(t, err, errAssetRequired, "GetDepositAddresses must require an asset")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errMethodRequired, "GetDepositAddresses must require a method")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin", AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetDepositAddresses must reject an invalid asset class")
	depositAddresses, err := ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", AssetClass: AssetClassCurrency, Method: "Bitcoin Lightning", New: true, Amount: &amount})
	require.NoError(t, err, "GetDepositAddresses must not error")
	assert.Equal(t, "MEMO", depositAddresses[0].Memo, "GetDepositAddresses should decode deposit memos")
	values = requireSpotRequest(t, requests, "/0/private/DepositAddresses")
	assert.Equal(t, "true", values.Get("new"), "GetDepositAddresses should encode address generation")
	assert.Equal(t, "1", values.Get("amount"), "GetDepositAddresses should encode Lightning deposit amounts")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin"})
	require.NoError(t, err, "GetDepositAddresses must allow omitted optional parameters")
	requireSpotRequest(t, requests, "/0/private/DepositAddresses")

	_, err = ex.GetWithdrawalInformation(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetWithdrawalInformation must reject a nil request")
	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{})
	require.ErrorIs(t, err, errAssetRequired, "GetWithdrawalInformation must require an asset")
	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errKeyRequired, "GetWithdrawalInformation must require a key")
	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet"})
	require.ErrorIs(t, err, errAmountInvalid, "GetWithdrawalInformation must require a positive amount")
	withdrawalInfo, err := ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet", Amount: amount})
	require.NoError(t, err, "GetWithdrawalInformation must not error")
	assert.Equal(t, 0.9, withdrawalInfo.Amount.Float64(), "GetWithdrawalInformation should decode the net amount")
	requireSpotRequest(t, requests, "/0/private/WithdrawInfo")

	_, err = ex.WithdrawFunds(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "WithdrawFunds must reject a nil request")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{})
	require.ErrorIs(t, err, errAssetRequired, "WithdrawFunds must require an asset")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errKeyRequired, "WithdrawFunds must require a key")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet"})
	require.ErrorIs(t, err, errAmountInvalid, "WithdrawFunds must require a positive amount")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: amount, AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "WithdrawFunds must reject an invalid asset class")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: amount, RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "WithdrawFunds must reject an invalid rebase multiplier")
	withdrawal, err := ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", AssetClass: AssetClassCurrency, Key: "wallet", Address: "bc1q", Amount: amount, MaximumFee: &maximumFee, RebaseMultiplier: RebaseMultiplierBase})
	require.NoError(t, err, "WithdrawFunds must not error")
	assert.Equal(t, "WITHDRAWAL", withdrawal.ReferenceID, "WithdrawFunds should decode the withdrawal reference")
	values = requireSpotRequest(t, requests, "/0/private/Withdraw")
	assert.Equal(t, "bc1q", values.Get("address"), "WithdrawFunds should encode a confirmation address")
	assert.Equal(t, "0.1", values.Get("max_fee"), "WithdrawFunds should encode a maximum fee")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: amount})
	require.NoError(t, err, "WithdrawFunds must allow omitted optional parameters")
	requireSpotRequest(t, requests, "/0/private/Withdraw")

	_, err = ex.GetRecentDepositsStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetRecentDepositsStatus must reject a nil request")
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetRecentDepositsStatus must reject an invalid asset class")
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetRecentDepositsStatus must reject an invalid rebase multiplier")
	depositLimit := uint64(25)
	deposits, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{
		Asset:            "XBT",
		AssetClass:       AssetClassCurrency,
		Method:           "Bitcoin",
		Start:            start,
		End:              end,
		Cursor:           "CURSOR",
		Limit:            &depositLimit,
		RebaseMultiplier: RebaseMultiplierBase,
	})
	require.NoError(t, err, "GetRecentDepositsStatus must not error")
	assert.Equal(t, "NEXT", deposits.NextCursor, "GetRecentDepositsStatus should decode pagination")
	assert.Equal(t, "REF", deposits.Deposits[0].ReferenceID, "GetRecentDepositsStatus should decode deposits")
	values = requireSpotRequest(t, requests, "/0/private/DepositStatus")
	assert.Equal(t, "CURSOR", values.Get("cursor"), "GetRecentDepositsStatus should encode a cursor")
	assert.Equal(t, "1", values.Get("start"), "GetRecentDepositsStatus should encode start time")
	assert.Equal(t, "2", values.Get("end"), "GetRecentDepositsStatus should encode end time")
	assert.Equal(t, "25", values.Get("limit"), "GetRecentDepositsStatus should encode a limit")
	paginate := true
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Paginate: &paginate})
	require.NoError(t, err, "GetRecentDepositsStatus must accept a pagination flag")
	values = requireSpotRequest(t, requests, "/0/private/DepositStatus")
	assert.Equal(t, "true", values.Get("cursor"), "GetRecentDepositsStatus should encode a pagination flag")
	depositLimit = 0
	_, err = ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Limit: &depositLimit})
	require.NoError(t, err, "GetRecentDepositsStatus must accept an explicit zero limit")
	values = requireSpotRequest(t, requests, "/0/private/DepositStatus")
	assert.Equal(t, "0", values.Get("limit"), "GetRecentDepositsStatus should encode an explicit zero limit")

	_, err = ex.GetRecentWithdrawalsStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetRecentWithdrawalsStatus must reject a nil request")
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetRecentWithdrawalsStatus must reject an invalid asset class")
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetRecentWithdrawalsStatus must reject an invalid rebase multiplier")
	withdrawalLimit := uint64(500)
	withdrawals, err := ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{
		Asset:            "XBT",
		AssetClass:       AssetClassCurrency,
		Method:           "Bitcoin",
		Start:            start,
		End:              end,
		Cursor:           "CURSOR",
		Limit:            &withdrawalLimit,
		RebaseMultiplier: RebaseMultiplierBase,
	})
	require.NoError(t, err, "GetRecentWithdrawalsStatus must not error")
	assert.Equal(t, "NEXT", withdrawals.NextCursor, "GetRecentWithdrawalsStatus should decode pagination")
	assert.Equal(t, "REF", withdrawals.Withdrawals[0].ReferenceID, "GetRecentWithdrawalsStatus should decode withdrawals")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawStatus")
	assert.Equal(t, "CURSOR", values.Get("cursor"), "GetRecentWithdrawalsStatus should encode a cursor")
	assert.Equal(t, "1", values.Get("start"), "GetRecentWithdrawalsStatus should encode start time")
	assert.Equal(t, "2", values.Get("end"), "GetRecentWithdrawalsStatus should encode end time")
	assert.Equal(t, "500", values.Get("limit"), "GetRecentWithdrawalsStatus should encode a limit")
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Paginate: &paginate})
	require.NoError(t, err, "GetRecentWithdrawalsStatus must accept a pagination flag")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawStatus")
	assert.Equal(t, "true", values.Get("cursor"), "GetRecentWithdrawalsStatus should encode a pagination flag")
	withdrawalLimit = 0
	_, err = ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Limit: &withdrawalLimit})
	require.NoError(t, err, "GetRecentWithdrawalsStatus must accept an explicit zero limit")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawStatus")
	assert.Equal(t, "0", values.Get("limit"), "GetRecentWithdrawalsStatus should encode an explicit zero limit")

	_, err = ex.GetWithdrawalMethods(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetWithdrawalMethods must reject a nil request")
	_, err = ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetWithdrawalMethods must reject an invalid asset class")
	_, err = ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetWithdrawalMethods must reject an invalid rebase multiplier")
	methods, err := ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{Asset: "XBT", AssetClass: "currency", Network: "Bitcoin", RebaseMultiplier: "base"})
	require.NoError(t, err, "GetWithdrawalMethods must not error")
	assert.Equal(t, "METHOD", methods[0].MethodID, "GetWithdrawalMethods should decode method identifiers")
	assert.Equal(t, 8.0, methods[0].Limits[0].Limits["86400"].Remaining.Float64(), "GetWithdrawalMethods should decode current rate limits")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawMethods")
	assert.Equal(t, "Bitcoin", values.Get("network"), "GetWithdrawalMethods should encode network")
	_, err = ex.GetWithdrawalMethods(ctx, &GetWithdrawalMethodsRequest{})
	require.NoError(t, err, "GetWithdrawalMethods must allow unfiltered requests")
	requireSpotRequest(t, requests, "/0/private/WithdrawMethods")

	_, err = ex.GetWithdrawalAddresses(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetWithdrawalAddresses must reject a nil request")
	_, err = ex.GetWithdrawalAddresses(ctx, &GetWithdrawalAddressesRequest{AssetClass: "invalid"})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetWithdrawalAddresses must reject an invalid asset class")
	verified := false
	addresses, err := ex.GetWithdrawalAddresses(ctx, &GetWithdrawalAddressesRequest{Asset: "XBT", AssetClass: "currency", Method: "Bitcoin", Key: "wallet", Verified: &verified})
	require.NoError(t, err, "GetWithdrawalAddresses must not error")
	assert.True(t, addresses[0].Verified, "GetWithdrawalAddresses should decode verification status")
	values = requireSpotRequest(t, requests, "/0/private/WithdrawAddresses")
	assert.Equal(t, "false", values.Get("verified"), "GetWithdrawalAddresses should encode a false verification filter")
	_, err = ex.GetWithdrawalAddresses(ctx, &GetWithdrawalAddressesRequest{})
	require.NoError(t, err, "GetWithdrawalAddresses must allow unfiltered requests")
	requireSpotRequest(t, requests, "/0/private/WithdrawAddresses")

	_, err = ex.WalletTransfer(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "WalletTransfer must reject a nil request")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{})
	require.ErrorIs(t, err, errAssetRequired, "WalletTransfer must require an asset")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT"})
	require.ErrorIs(t, err, errFromRequired, "WalletTransfer must require a source wallet")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletFutures})
	require.ErrorIs(t, err, errFromWalletInvalid, "WalletTransfer must reject an invalid source wallet")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot})
	require.ErrorIs(t, err, errToRequired, "WalletTransfer must require a destination wallet")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletSpot})
	require.ErrorIs(t, err, errToWalletInvalid, "WalletTransfer must reject an invalid destination wallet")
	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures})
	require.ErrorIs(t, err, errAmountInvalid, "WalletTransfer must require a positive amount")
	transfer, err := ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures, Amount: amount})
	require.NoError(t, err, "WalletTransfer must not error")
	assert.Equal(t, "TRANSFER", transfer.ReferenceID, "WalletTransfer should decode the transfer reference")
	values = requireSpotRequest(t, requests, "/0/private/WalletTransfer")
	assert.Equal(t, "1", values.Get("amount"), "WalletTransfer should encode amount")
}

func TestRecentDepositsStatusResponseUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name          string
		payload       string
		expectedCount int
		expectedNext  string
		errExpected   bool
		expectedErr   error
	}{
		{name: "array", payload: `[{"refid":"ARRAY"}]`, expectedCount: 1},
		{name: "empty array", payload: `[]`},
		{name: "single", payload: `{"refid":"SINGLE"}`, expectedCount: 1},
		{name: "paginated array", payload: `{"deposit":[{"refid":"PAGE"}],"next_cursor":"NEXT"}`, expectedCount: 1, expectedNext: "NEXT"},
		{name: "paginated single", payload: `{"deposit":{"refid":"PAGE"},"next_cursor":"NEXT"}`, expectedCount: 1, expectedNext: "NEXT"},
		{name: "invalid paginated deposit", payload: `{"deposit":"invalid"}`, errExpected: true},
		{name: "invalid paginated deposit field", payload: `{"deposit":{"amount":{}}}`, errExpected: true},
		{name: "empty paginated deposit", payload: `{"deposit":{}}`, errExpected: true, expectedErr: errPaginatedDepositInvalid},
		{name: "null paginated deposit", payload: `{"deposit":null}`, errExpected: true, expectedErr: errPaginatedDepositInvalid},
		{name: "empty object", payload: `{}`, errExpected: true, expectedErr: errDepositResultInvalid},
		{name: "invalid deposit field", payload: `{"amount":{}}`, errExpected: true},
		{name: "unrelated object", payload: `{"unexpected":true}`, errExpected: true, expectedErr: errDepositResultInvalid},
		{name: "null", payload: `null`, errExpected: true, expectedErr: errDepositResultInvalid},
		{name: "invalid JSON", payload: `{`, errExpected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response RecentDepositsStatusResponse
			err := response.UnmarshalJSON([]byte(tc.payload))
			if tc.errExpected {
				if tc.expectedErr != nil {
					require.ErrorIs(t, err, tc.expectedErr, "UnmarshalJSON must return the expected deposit shape error")
					return
				}
				require.Error(t, err, "UnmarshalJSON must reject invalid deposit data")
				return
			}
			require.NoError(t, err, "UnmarshalJSON must not error")
			assert.Len(t, response.Deposits, tc.expectedCount, "UnmarshalJSON should normalise deposit results")
			assert.Equal(t, tc.expectedNext, response.NextCursor, "UnmarshalJSON should retain the next cursor")
		})
	}

	var reused RecentDepositsStatusResponse
	require.NoError(t, reused.UnmarshalJSON([]byte(`{"deposit":{"refid":"PAGE"},"next_cursor":"NEXT"}`)), "UnmarshalJSON must decode a reusable response")
	beforeError := reused
	err := reused.UnmarshalJSON([]byte(`{"deposit":{"amount":{}}}`))
	require.Error(t, err, "UnmarshalJSON must reject invalid data when reusing a response")
	assert.Equal(t, beforeError, reused, "UnmarshalJSON should not partially mutate the receiver on error")
	require.NoError(t, reused.UnmarshalJSON([]byte(`[{"refid":"ARRAY"}]`)), "UnmarshalJSON must replace a reusable response")
	assert.Empty(t, reused.NextCursor, "UnmarshalJSON should clear a stale cursor on a non-paginated response")
	assert.Equal(t, "ARRAY", reused.Deposits[0].ReferenceID, "UnmarshalJSON should replace stale deposit data")
}

func TestRecentWithdrawalsStatusResponseUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name          string
		payload       string
		expectedCount int
		expectedNext  string
		errExpected   bool
		expectedErr   error
	}{
		{name: "array", payload: `[{"refid":"ARRAY"}]`, expectedCount: 1},
		{name: "empty array", payload: `[]`},
		{name: "single", payload: `{"refid":"SINGLE"}`, expectedCount: 1},
		{name: "paginated array", payload: `{"withdrawal":[{"refid":"PAGE"}],"next_cursor":"NEXT"}`, expectedCount: 1, expectedNext: "NEXT"},
		{name: "paginated single", payload: `{"withdrawal":{"refid":"PAGE"},"next_cursor":"NEXT"}`, expectedCount: 1, expectedNext: "NEXT"},
		{name: "invalid paginated withdrawal", payload: `{"withdrawal":"invalid"}`, errExpected: true},
		{name: "invalid paginated withdrawal field", payload: `{"withdrawal":{"amount":{}}}`, errExpected: true},
		{name: "empty paginated withdrawal", payload: `{"withdrawal":{}}`, errExpected: true, expectedErr: errPaginatedWithdrawalInvalid},
		{name: "null paginated withdrawal", payload: `{"withdrawal":null}`, errExpected: true, expectedErr: errPaginatedWithdrawalInvalid},
		{name: "empty object", payload: `{}`, errExpected: true, expectedErr: errWithdrawalResultInvalid},
		{name: "invalid withdrawal field", payload: `{"amount":{}}`, errExpected: true},
		{name: "unrelated object", payload: `{"unexpected":true}`, errExpected: true, expectedErr: errWithdrawalResultInvalid},
		{name: "null", payload: `null`, errExpected: true, expectedErr: errWithdrawalResultInvalid},
		{name: "invalid JSON", payload: `{`, errExpected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response RecentWithdrawalsStatusResponse
			err := response.UnmarshalJSON([]byte(tc.payload))
			if tc.errExpected {
				if tc.expectedErr != nil {
					require.ErrorIs(t, err, tc.expectedErr, "UnmarshalJSON must return the expected withdrawal shape error")
					return
				}
				require.Error(t, err, "UnmarshalJSON must reject invalid withdrawal data")
				return
			}
			require.NoError(t, err, "UnmarshalJSON must not error")
			assert.Len(t, response.Withdrawals, tc.expectedCount, "UnmarshalJSON should normalise withdrawal results")
			assert.Equal(t, tc.expectedNext, response.NextCursor, "UnmarshalJSON should retain the next cursor")
		})
	}

	var reused RecentWithdrawalsStatusResponse
	require.NoError(t, reused.UnmarshalJSON([]byte(`{"withdrawal":{"refid":"PAGE"},"next_cursor":"NEXT"}`)), "UnmarshalJSON must decode a reusable response")
	beforeError := reused
	err := reused.UnmarshalJSON([]byte(`{"withdrawal":{"amount":{}}}`))
	require.Error(t, err, "UnmarshalJSON must reject invalid data when reusing a response")
	assert.Equal(t, beforeError, reused, "UnmarshalJSON should not partially mutate the receiver on error")
	require.NoError(t, reused.UnmarshalJSON([]byte(`[{"refid":"ARRAY"}]`)), "UnmarshalJSON must replace a reusable response")
	assert.Empty(t, reused.NextCursor, "UnmarshalJSON should clear a stale cursor on a non-paginated response")
	assert.Equal(t, "ARRAY", reused.Withdrawals[0].ReferenceID, "UnmarshalJSON should replace stale withdrawal data")
}

func TestContainsDepositField(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		fields   map[string]json.RawMessage
		expected bool
	}{
		{name: "nil fields"},
		{name: "unrelated field", fields: map[string]json.RawMessage{"unexpected": nil}},
		{name: "deposit field", fields: map[string]json.RawMessage{"refid": nil}, expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, containsDepositField(tc.fields), "containsDepositField should identify known deposit fields")
		})
	}
}

func TestContainsWithdrawalField(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		fields   map[string]json.RawMessage
		expected bool
	}{
		{name: "nil fields"},
		{name: "unrelated field", fields: map[string]json.RawMessage{"unexpected": nil}},
		{name: "withdrawal field", fields: map[string]json.RawMessage{"refid": nil}, expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, containsWithdrawalField(tc.fields), "containsWithdrawalField should identify known withdrawal fields")
		})
	}
}

func TestSpotFundingTypedRequestValidation(t *testing.T) {
	ex, _ := newSpotEndpointExchange(t, spotFundingFixtures)
	ctx := t.Context()
	zero := 0.0
	negative := -1.0
	notANumber := math.NaN()
	preEpoch := time.Unix(-1, 0)
	start := time.Unix(2, 0)
	end := time.Unix(1, 0)
	paginate := true

	_, err := ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin Lightning", Amount: &zero})
	require.ErrorIs(t, err, errAmountInvalid, "GetDepositAddresses must reject an explicit zero amount")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin Lightning", Amount: &negative})
	require.ErrorIs(t, err, errAmountInvalid, "GetDepositAddresses must reject a negative amount")
	_, err = ex.GetDepositAddresses(ctx, &GetDepositAddressesRequest{Asset: "XBT", Method: "Bitcoin Lightning", Amount: &notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "GetDepositAddresses must reject a non-finite amount")

	_, err = ex.GetWithdrawalInformation(ctx, &GetWithdrawalInformationRequest{Asset: "XBT", Key: "wallet", Amount: notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "GetWithdrawalInformation must reject a non-finite amount")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "WithdrawFunds must reject a non-finite amount")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: 1, MaximumFee: &negative})
	require.ErrorIs(t, err, errMaximumFeeInvalid, "WithdrawFunds must reject a negative maximum fee")
	_, err = ex.WithdrawFunds(ctx, &WithdrawFundsRequest{Asset: "XBT", Key: "wallet", Amount: 1, MaximumFee: &notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "WithdrawFunds must reject a non-finite maximum fee")

	for _, tc := range []struct {
		name     string
		call     func() error
		expected error
	}{
		{name: "deposit cursor conflict", call: func() error {
			_, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Cursor: "CURSOR", Paginate: &paginate})
			return err
		}, expected: errCursorConflict},
		{name: "deposit pre-epoch timestamp", call: func() error {
			_, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Start: preEpoch})
			return err
		}, expected: errTimestampInvalid},
		{name: "deposit reversed range", call: func() error {
			_, err := ex.GetRecentDepositsStatus(ctx, &GetRecentDepositsStatusRequest{Start: start, End: end})
			return err
		}, expected: errTimeRangeInvalid},
		{name: "withdrawal cursor conflict", call: func() error {
			_, err := ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Cursor: "CURSOR", Paginate: &paginate})
			return err
		}, expected: errCursorConflict},
		{name: "withdrawal pre-epoch timestamp", call: func() error {
			_, err := ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Start: preEpoch})
			return err
		}, expected: errTimestampInvalid},
		{name: "withdrawal reversed range", call: func() error {
			_, err := ex.GetRecentWithdrawalsStatus(ctx, &GetRecentWithdrawalsStatusRequest{Start: start, End: end})
			return err
		}, expected: errTimeRangeInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			require.ErrorIs(t, err, tc.expected, tc.name+" must be rejected")
		})
	}

	_, err = ex.WalletTransfer(ctx, &WalletTransferRequest{Asset: "XBT", From: WalletSpot, To: WalletFutures, Amount: notANumber})
	require.ErrorIs(t, err, errNumericValueInvalid, "WalletTransfer must reject a non-finite amount")
}

func TestCancelWithdrawal(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotFundingFixtures)
	_, err := ex.CancelWithdrawal(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "CancelWithdrawal must reject a nil request")
	_, err = ex.CancelWithdrawal(t.Context(), &CancelWithdrawalRequest{ReferenceID: "REFERENCE"})
	require.ErrorIs(t, err, errAssetRequired, "CancelWithdrawal must require an asset")
	_, err = ex.CancelWithdrawal(t.Context(), &CancelWithdrawalRequest{Asset: "BTC"})
	require.ErrorIs(t, err, errReferenceIDRequired, "CancelWithdrawal must require a reference identifier")
	cancelled, err := ex.CancelWithdrawal(t.Context(), &CancelWithdrawalRequest{Asset: "BTC", ReferenceID: "REFERENCE"})
	require.NoError(t, err, "CancelWithdrawal must not error")
	assert.True(t, cancelled, "CancelWithdrawal should decode the cancellation result")
	values := requireSpotRequest(t, requests, "/0/private/WithdrawCancel")
	assert.Equal(t, "BTC", values.Get("asset"), "CancelWithdrawal should encode the asset")
	assert.Equal(t, "REFERENCE", values.Get("refid"), "CancelWithdrawal should encode the reference identifier")
	_, err = newSpotErrorExchange(t).CancelWithdrawal(t.Context(), &CancelWithdrawalRequest{Asset: "BTC", ReferenceID: "REFERENCE"})
	require.ErrorIs(t, err, errSpotTransport, "CancelWithdrawal must surface request errors")
}
