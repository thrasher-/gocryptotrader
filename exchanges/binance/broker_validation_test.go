package binance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
)

func TestBrokerRequestValidation(t *testing.T) {
	local := new(Exchange)
	t.Run("DeleteBrokerSubAccount", func(t *testing.T) {
		err := local.DeleteBrokerSubAccount(t.Context(), nil)
		assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail offline")
		for _, tc := range []struct {
			field string
			want  error
		}{{"subAccountId", errSubAccountIDMissing}} {
			t.Run(tc.field, func(t *testing.T) {
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(`{"subAccountId":"1"}`), &fields), "valid input must decode")
				delete(fields, tc.field)
				raw, err := json.Marshal(fields)
				require.NoError(t, err, "invalid request must serialise")
				arg := new(BrokerSubAccountRequest)
				require.NoError(t, json.Unmarshal(raw, arg), "request must decode")
				err = local.DeleteBrokerSubAccount(t.Context(), arg)
				assert.ErrorIs(t, err, tc.want, "missing field should retain its sentinel")
			})
		}
	})
	t.Run("GetBrokerSubAccountAPIKeys", func(t *testing.T) {
		_, err := local.GetBrokerSubAccountAPIKeys(t.Context(), nil)
		assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail offline")
		for _, tc := range []struct {
			field string
			want  error
		}{{"subAccountId", errSubAccountIDMissing}} {
			t.Run(tc.field, func(t *testing.T) {
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(`{"subAccountId":"1"}`), &fields), "valid input must decode")
				delete(fields, tc.field)
				raw, err := json.Marshal(fields)
				require.NoError(t, err, "invalid request must serialise")
				arg := new(BrokerAPIKeysRequest)
				require.NoError(t, json.Unmarshal(raw, arg), "request must decode")
				_, err = local.GetBrokerSubAccountAPIKeys(t.Context(), arg)
				assert.ErrorIs(t, err, tc.want, "missing field should retain its sentinel")
			})
		}
	})
	t.Run("GetBrokerSubAccountIPRestriction", func(t *testing.T) {
		_, err := local.GetBrokerSubAccountIPRestriction(t.Context(), nil)
		assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail offline")
		for _, tc := range []struct {
			field string
			want  error
		}{{"subAccountId", errSubAccountIDMissing}, {"subAccountApiKey", errEmptySubAccountAPIKey}} {
			t.Run(tc.field, func(t *testing.T) {
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(`{"subAccountId":"1","subAccountApiKey":"key"}`), &fields), "valid input must decode")
				delete(fields, tc.field)
				raw, err := json.Marshal(fields)
				require.NoError(t, err, "invalid request must serialise")
				arg := new(BrokerIPRestrictionRequest)
				require.NoError(t, json.Unmarshal(raw, arg), "request must decode")
				_, err = local.GetBrokerSubAccountIPRestriction(t.Context(), arg)
				assert.ErrorIs(t, err, tc.want, "missing field should retain its sentinel")
			})
		}
	})
	t.Run("BrokerFuturesTransfer", func(t *testing.T) {
		_, err := local.BrokerFuturesTransfer(t.Context(), nil)
		assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail offline")
		for _, tc := range []struct {
			field string
			want  error
		}{{"subAccountId", errSubAccountIDMissing}, {"asset", currency.ErrCurrencyCodeEmpty}, {"amount", limits.ErrAmountBelowMin}, {"type", errInvalidTransferType}} {
			t.Run(tc.field, func(t *testing.T) {
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(`{"subAccountId":"1","asset":"BTC","amount":1,"type":1}`), &fields), "valid input must decode")
				delete(fields, tc.field)
				raw, err := json.Marshal(fields)
				require.NoError(t, err, "invalid request must serialise")
				arg := new(BrokerFuturesTransferRequest)
				require.NoError(t, json.Unmarshal(raw, arg), "request must decode")
				_, err = local.BrokerFuturesTransfer(t.Context(), arg)
				assert.ErrorIs(t, err, tc.want, "missing field should retain its sentinel")
			})
		}
	})
	t.Run("GetBrokerDepositHistory", func(t *testing.T) {
		_, err := local.GetBrokerDepositHistory(t.Context(), nil)
		assert.ErrorIs(t, err, common.ErrNilPointer, "nil request should fail offline")
		for _, tc := range []struct {
			field string
			want  error
		}{{"subAccountId", errSubAccountIDMissing}, {"depositId", errTransactionIDRequired}} {
			t.Run(tc.field, func(t *testing.T) {
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(`{"subAccountId":"1","depositId":"2"}`), &fields), "valid input must decode")
				delete(fields, tc.field)
				raw, err := json.Marshal(fields)
				require.NoError(t, err, "invalid request must serialise")
				arg := new(BrokerDepositHistoryRequest)
				require.NoError(t, json.Unmarshal(raw, arg), "request must decode")
				_, err = local.GetBrokerDepositHistory(t.Context(), arg)
				assert.ErrorIs(t, err, tc.want, "missing field should retain its sentinel")
			})
		}
	})
	for _, transferType := range []uint64{0, 5} {
		_, err := local.BrokerFuturesTransfer(t.Context(), &BrokerFuturesTransferRequest{SubAccountID: "1", Asset: currency.BTC, Amount: 1, Type: transferType})
		assert.ErrorIs(t, err, errInvalidTransferType, "both transfer type bounds should retain their sentinel")
	}
}
