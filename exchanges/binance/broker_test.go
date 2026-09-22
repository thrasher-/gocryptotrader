package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

// TestDocumentedBrokerRequests uses the CAAS parameter tables and response examples.
// Input values are synthetic; no credentialed success is inferred from these mocks.
func TestDocumentedBrokerRequests(t *testing.T) {
	raw, err := os.ReadFile("testdata/documented_broker.json")
	require.NoError(t, err, "broker examples must load")
	var fixtures map[string]*documentedResponse
	require.NoError(t, json.Unmarshal(raw, &fixtures), "broker examples must decode")
	start, end := time.UnixMilli(1750500000000), time.UnixMilli(1750500060000)
	for _, tc := range []struct {
		name   string
		params url.Values
		call   func(*Exchange) (any, error)
	}{
		{"POST /sapi/v1/broker/subAccountApi/permission", url.Values{"subAccountId": {"1"}, "subAccountApiKey": {"key"}, "canTrade": {"false"}, "marginTrade": {"false"}, "futuresTrade": {"false"}}, func(e *Exchange) (any, error) {
			return e.ChangeSubAccountAPIPermission(t.Context(), &ChangeSubAccountAPIPermissionRequest{SubAccountID: "1", SubAccountAPIKey: "key"})
		}},
		{"GET /sapi/v1/broker/subAccount", url.Values{"subAccountId": {"1"}, "page": {"2"}, "size": {"5"}}, func(e *Exchange) (any, error) { return e.GetSubAccounts(t.Context(), "1", 2, 5) }},
		{"POST /sapi/v1/broker/subAccount", url.Values{"tag": {"tag"}}, func(e *Exchange) (any, error) { return e.CreateSubAccount(t.Context(), "tag") }},
		{"DELETE /sapi/v1/broker/subAccount", url.Values{"subAccountId": {"1"}}, func(e *Exchange) (any, error) {
			return nil, e.DeleteBrokerSubAccount(t.Context(), &BrokerSubAccountRequest{SubAccountID: "1"})
		}},
		{"GET /sapi/v1/broker/subAccountApi", url.Values{"subAccountId": {"1"}, "subAccountApiKey": {"key"}, "page": {"2"}, "size": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetBrokerSubAccountAPIKeys(t.Context(), &BrokerAPIKeysRequest{SubAccountID: "1", SubAccountAPIKey: "key", Page: 2, Size: 5})
		}},
		{"POST /sapi/v1/broker/subAccountApi", url.Values{"subAccountId": {"1"}, "canTrade": {"false"}, "marginTrade": {"false"}, "futuresTrade": {"false"}, "publicKey": {"documented-public-key"}}, func(e *Exchange) (any, error) {
			return e.CreateAPIKeyForSubAccount(t.Context(), "1", false, false, false, &BrokerAPIKeyOptionsRequest{PublicKey: "documented-public-key"})
		}},
		{"DELETE /sapi/v1/broker/subAccountApi", url.Values{"subAccountId": {"1"}, "subAccountApiKey": {"key"}}, func(e *Exchange) (any, error) { return e.DeleteBrokerSubAccountAPIKey(t.Context(), "1", "key") }},
		{"DELETE /sapi/v1/broker/subAccountApi/ipRestriction/ipList", url.Values{"subAccountId": {"1"}, "subAccountApiKey": {"key"}, "ipAddress": {"192.0.2.1"}}, func(e *Exchange) (any, error) {
			return e.DeleteIPRestrictionForSubAccountAPIKey(t.Context(), "1", "key", "192.0.2.1")
		}},
		{"POST /sapi/v1/broker/subAccount/futures", url.Values{"subAccountId": {"1"}, "futures": {"true"}}, func(e *Exchange) (any, error) { return e.EnableFuturesForSubAccount(t.Context(), "1", true) }},
		{"POST /sapi/v1/broker/subAccount/bnbBurn/marginInterest", url.Values{"subAccountId": {"1"}, "interestBNBBurn": {"false"}}, func(e *Exchange) (any, error) {
			return e.EnableOrDisableBNBBurnForSubAccountMarginInterest(t.Context(), "1", false)
		}},
		{"POST /sapi/v1/broker/subAccount/bnbBurn/spot", url.Values{"subAccountId": {"1"}, "spotBNBBurn": {"false"}}, func(e *Exchange) (any, error) {
			return e.EnableOrDisableBNBBurnForSubAccountSpotAndMargin(t.Context(), "1", false)
		}},
		{"POST /sapi/v1/broker/subAccountApi/permission/universalTransfer", url.Values{"subAccountId": {"1"}, "subAccountApiKey": {"key"}, "canUniversalTransfer": {"false"}}, func(e *Exchange) (any, error) {
			return e.EnableUniversalTransferPermissionForSubAccountAPIKey(t.Context(), "1", "key", false)
		}},
		{"GET /sapi/v1/broker/subAccount/bnbBurn/status", url.Values{"subAccountId": {"1"}}, func(e *Exchange) (any, error) { return e.GetBNBBurnStatusForSubAccount(t.Context(), "1") }},
		{"GET /sapi/v1/broker/subAccountApi/ipRestriction", url.Values{"subAccountId": {"1"}, "subAccountApiKey": {"key"}}, func(e *Exchange) (any, error) {
			return e.GetBrokerSubAccountIPRestriction(t.Context(), &BrokerIPRestrictionRequest{SubAccountID: "1", SubAccountAPIKey: "key"})
		}},
		{"GET /sapi/v1/broker/info", url.Values{}, func(e *Exchange) (any, error) { return e.LinkAccountInformation(t.Context()) }},
		{"POST /sapi/v2/broker/subAccountApi/ipRestriction", url.Values{"subAccountId": {"1"}, "subAccountApiKey": {"key"}, "status": {"2"}, "ipAddress": {"192.0.2.1"}}, func(e *Exchange) (any, error) {
			return e.UpdateIPRestrictionForSubAccountAPIKey(t.Context(), "1", "key", "2", "192.0.2.1")
		}},
		{"POST /sapi/v1/broker/futures/accountTransfer", url.Values{"subAccountId": {"1"}, "asset": {"BTC"}, "amount": {"0.123"}, "type": {"1"}}, func(e *Exchange) (any, error) {
			return e.BrokerFuturesTransfer(t.Context(), &BrokerFuturesTransferRequest{SubAccountID: "1", Asset: currency.BTC, Amount: 0.123, Type: 1})
		}},
		{"GET /sapi/v1/broker/subAccount/depositHist", url.Values{"subAccountId": {"1"}, "coin": {"BTC"}, "status": {"0"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "limit": {"5"}, "offset": {"2"}}, func(e *Exchange) (any, error) {
			return e.GetSubAccountDepositHistoryWithBroker(t.Context(), &GetSubAccountDepositHistoryWithBrokerRequest{SubAccountID: "1", Coin: currency.BTC, StartTime: start, EndTime: end, Limit: 5, Offset: 2})
		}},
		{"GET /sapi/v2/broker/subAccount/depositHist", url.Values{"depositId": {"123"}, "subAccountId": {"1"}, "limit": {"5"}, "offset": {"2"}}, func(e *Exchange) (any, error) {
			return e.GetBrokerDepositHistory(t.Context(), &BrokerDepositHistoryRequest{DepositID: "123", SubAccountID: "1", Limit: 5, Offset: 2})
		}},
		{"GET /sapi/v3/broker/subAccount/futuresSummary", url.Values{"subAccountId": {"1"}, "page": {"2"}, "size": {"5"}, "futuresType": {"2"}}, func(e *Exchange) (any, error) { return e.GetSubAccountFuturesAssetInfo(t.Context(), "1", true, 2, 5) }},
		{"GET /sapi/v1/broker/subAccount/marginSummary", url.Values{"subAccountId": {"1"}, "page": {"2"}, "size": {"5"}}, func(e *Exchange) (any, error) { return e.GetSubAccountMarginAssetInfo(t.Context(), "1", 2, 5) }},
		{"GET /sapi/v1/broker/subAccount/spotSummary", url.Values{"subAccountId": {"1"}, "page": {"2"}, "size": {"5"}}, func(e *Exchange) (any, error) { return e.GetSubAccountSpotAssetInfo(t.Context(), "1", 2, 5) }},
		{"GET /sapi/v1/broker/transfer/futures", url.Values{"subAccountId": {"1"}, "futuresType": {"2"}, "clientTranId": {"test"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "page": {"2"}, "limit": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetFuturesBrokerSubAccountTransferHistory(t.Context(), &GetFuturesBrokerSubAccountTransferHistoryRequest{SubAccountID: "1", CoinMargined: true, ClientTransferID: "test", StartTime: start, EndTime: end, Page: 2, Limit: 5})
		}},
		{"POST /sapi/v1/broker/transfer/futures", url.Values{"futuresType": {"2"}, "asset": {"BTC"}, "amount": {"0.123"}, "fromId": {"1"}, "toId": {"2"}, "clientTranId": {"test"}}, func(e *Exchange) (any, error) {
			return e.SubAccountTransferWithFuturesBroker(t.Context(), &SubAccountTransferWithFuturesBrokerRequest{FuturesType: 2, Currency: currency.BTC, Amount: 0.123, FromID: "1", ToID: "2", ClientTransferID: "test"})
		}},
		{"GET /sapi/v1/broker/transfer", url.Values{"fromId": {"1"}, "toId": {"2"}, "clientTranId": {"test"}, "showAllStatus": {"true"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "page": {"2"}, "limit": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetSpotBrokerSubAccountTransferHistory(t.Context(), &BrokerSubAccountTransferHistoryRequest{FromID: "1", ToID: "2", ClientTransferID: "test", ShowAllStatus: true, StartTime: start, EndTime: end, Page: 2, Limit: 5})
		}},
		{"POST /sapi/v1/broker/transfer", url.Values{"asset": {"BTC"}, "amount": {"0.123"}, "fromId": {"1"}, "toId": {"2"}, "clientTranId": {"test"}}, func(e *Exchange) (any, error) {
			return e.SubAccountTransferWithSpotBroker(t.Context(), &SubAccountTransferWithSpotBrokerRequest{Currency: currency.BTC, Amount: 0.123, FromID: "1", ToID: "2", ClientTransferID: "test"})
		}},
		{"GET /sapi/v1/broker/universalTransfer", url.Values{"fromId": {"1"}, "toId": {"2"}, "clientTranId": {"test"}, "showAllStatus": {"false"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "page": {"2"}, "limit": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetUniversalTransferHistoryThroughBroker(t.Context(), &GetUniversalTransferHistoryThroughBrokerRequest{FromID: "1", ToID: "2", ClientTransferID: "test", ShowAllStatus: new(false), StartTime: start, EndTime: end, Page: 2, Limit: 5})
		}},
		{"POST /sapi/v1/broker/universalTransfer", url.Values{"fromAccountType": {"SPOT"}, "toAccountType": {"COIN_FUTURE"}, "asset": {"BTC"}, "amount": {"0.123"}, "fromId": {"1"}, "toId": {"2"}, "clientTranId": {"test"}}, func(e *Exchange) (any, error) {
			return e.UniversalTransferWithBroker(t.Context(), &UniversalTransferWithBrokerRequest{FromAccountType: "SPOT", ToAccountType: "COIN_FUTURE", Currency: currency.BTC, Amount: 0.123, FromID: "1", ToID: "2", ClientTransferID: "test"})
		}},
		{"GET /sapi/v1/broker/subAccountApi/commission/coinFutures", url.Values{"subAccountId": {"1"}, "pair": {"BTCUSD"}}, func(e *Exchange) (any, error) {
			return e.GetSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "1", currency.NewBTCUSD())
		}},
		{"POST /sapi/v1/broker/subAccountApi/commission/coinFutures", url.Values{"subAccountId": {"1"}, "makerAdjustment": {"0"}, "takerAdjustment": {"0"}, "pair": {"BTCUSD"}}, func(e *Exchange) (any, error) {
			return e.ChangeSubAccountCoinMarginedFuturesCommissionAdjustment(t.Context(), "1", currency.NewBTCUSD(), 0, 0)
		}},
		{"GET /sapi/v1/broker/subAccountApi/commission/futures", url.Values{"subAccountId": {"1"}, "symbol": {"BTCUSDT"}}, func(e *Exchange) (any, error) {
			return e.GetSubAccountUSDMarginedFuturesCommissionAdjustment(t.Context(), "1", currency.NewBTCUSDT())
		}},
		{"POST /sapi/v1/broker/subAccountApi/commission/futures", url.Values{"subAccountId": {"1"}, "makerAdjustment": {"0"}, "takerAdjustment": {"0"}, "symbol": {"BTCUSDT"}}, func(e *Exchange) (any, error) {
			return e.ChangeSubAccountUSDTMarginedFuturesCommissionAdjustment(t.Context(), "1", currency.NewBTCUSDT(), 0, 0)
		}},
		{"POST /sapi/v1/broker/subAccountApi/commission", url.Values{"subAccountId": {"1"}, "makerCommission": {"0.001"}, "takerCommission": {"0.002"}, "marginMakerCommission": {"0.001"}, "marginTakerCommission": {"0.002"}}, func(e *Exchange) (any, error) {
			return e.ChangeSubAccountCommission(t.Context(), &ChangeSubAccountCommissionRequest{SubAccountID: "1", MakerCommission: 0.001, TakerCommission: 0.002, MarginMakerCommission: 0.001, MarginTakerCommission: 0.002})
		}},
		{"GET /sapi/v1/broker/rebate/recentRecord", url.Values{"subAccountId": {"1"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "page": {"2"}, "size": {"5"}}, func(e *Exchange) (any, error) {
			return e.GetSpotBrokerCommissionRebateRecentRecord(t.Context(), &GetSpotBrokerCommissionRebateRecentRecordRequest{SubAccountID: "1", StartTime: start, EndTime: end, Page: 2, Limit: 5})
		}},
		{"GET /sapi/v1/broker/rebate/futures/recentRecord", url.Values{"futuresType": {"2"}, "startTime": {"1750500000000"}, "endTime": {"1750500060000"}, "page": {"2"}, "size": {"5"}, "filterResult": {"true"}}, func(e *Exchange) (any, error) {
			return e.GetFuturesBrokerCommissionRebateRecentRecord(t.Context(), &GetFuturesBrokerCommissionRebateRecentRecordRequest{CoinMargined: true, StartTime: start, EndTime: end, Page: 2, Size: 5, FilterResult: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := fixtures[tc.name]
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, fixture.Method, r.Method, "request should use its documented HTTP verb")
				assert.Equal(t, fixture.Path, r.URL.Path, "request should use its documented route")
				assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "request should use the test key")
				params := r.URL.Query()
				signature := params.Get("signature")
				params.Del("signature")
				mac := hmac.New(sha256.New, []byte("test-secret"))
				_, _ = mac.Write([]byte(params.Encode()))
				assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), signature, "signature should cover every parameter")
				assert.NotEmpty(t, params.Get("timestamp"), "request should include a timestamp")
				assert.NotEmpty(t, params.Get("recvWindow"), "request should include its validity window")
				params.Del("timestamp")
				params.Del("recvWindow")
				assert.Equal(t, tc.params, params, "request should transmit all documented business parameters")
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write(fixture.Data)
				assert.NoError(t, err, "documented response should write")
			})
			result, err := tc.call(local)
			require.NoError(t, err, "documented response must decode through the endpoint")
			if result != nil && string(fixture.Data) != "null" {
				assertResponseFields(t, fixture.Data, reflect.TypeOf(result), tc.name)
			}
			switch response := result.(type) {
			case *SubAccountAPIKey:
				assert.Equal(t, "documented-key", response.APIKey, "either documented API key spelling should decode")
			case *SubAccountIPRestrictioin:
				assert.Equal(t, "documented-key", response.APIKey, "either documented API key spelling should decode")
			case *BrokerUniversalTransferResponse:
				assert.Equal(t, "12831061179", response.TransactionID.String(), "transfer identifier should retain its exact value")
			}
		})
	}
}
