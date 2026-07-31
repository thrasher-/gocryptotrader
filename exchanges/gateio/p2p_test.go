package gateio

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
)

func newAuthenticatedP2PRouteTestExchange(t *testing.T, requestURI, expectedBody, response string) *Exchange {
	t.Helper()
	return newAuthenticatedHTTPTestExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method, "request method should match")
		assert.Equal(t, requestURI, r.URL.RequestURI(), "request URI should match")
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err, "reading request body should not error") {
			return
		}
		if expectedBody == "" {
			assert.Empty(t, body, "request body should be empty")
		} else {
			assert.JSONEq(t, expectedBody, string(body), "request body should match Gate's documented wire format")
		}
		_, err = fmt.Fprint(w, response)
		assert.NoError(t, err, "writing response should not error")
	}))
}

func TestGetP2PAccountInfo(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/account/get_user_info",
			"",
			`{"code":0,"data":{"user_timest":"2026-07-31 12:00:00","counterparties_num":2,"biz_uid":"business-1"}}`)
	}
	result, err := ex.GetP2PAccountInfo(t.Context())
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		assert.Equal(t, "2026-07-31 12:00:00", result.UserTimestamp)
		assert.Equal(t, uint64(2), result.CounterpartiesNumber)
		assert.Equal(t, "business-1", result.BusinessUID)
	}
}

func TestGetP2PCounterpartyInfo(t *testing.T) {
	t.Parallel()
	_, err := e.GetP2PCounterpartyInfo(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetP2PCounterpartyInfo(t.Context(), &GetCounterpartyInfoRequest{})
	require.ErrorIs(t, err, errBizUIDRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/account/get_counterparty_user_info",
			`{"biz_uid":"biz_uid_demo_0fbc1"}`,
			`{"code":0,"data":{"user_timest":"2026-07-31 12:00:00","biz_uid":"biz_uid_demo_0fbc1"}}`)
	}
	result, err := ex.GetP2PCounterpartyInfo(t.Context(), &GetCounterpartyInfoRequest{BusinessUID: "biz_uid_demo_0fbc1"})
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		assert.Equal(t, "2026-07-31 12:00:00", result.UserTimestamp)
		assert.Equal(t, "biz_uid_demo_0fbc1", result.BusinessUID)
	}
}

func TestGetP2PPaymentMethods(t *testing.T) {
	t.Parallel()

	_, err := e.GetP2PPaymentMethods(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/account/get_myself_payment",
			`{"fiat":"USD"}`,
			`{"code":0,"data":[]}`)
	}
	result, err := ex.GetP2PPaymentMethods(t.Context(), &GetP2PPaymentMethodsRequest{Fiat: "USD"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSetMerchantWorkingStatusAndCustomWorking(t *testing.T) {
	t.Parallel()

	_, err := e.SetMerchantWorkingStatusAndCustomWorking(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.SetMerchantWorkingStatusAndCustomWorking(t.Context(), &SetMerchantWorkHoursRequest{WorkStatus: 5})
	require.ErrorIs(t, err, errP2PWorkStatusInvalid)

	_, err = e.SetMerchantWorkingStatusAndCustomWorking(t.Context(), &SetMerchantWorkHoursRequest{WorkStatus: 2})
	require.ErrorIs(t, err, errNoValidParameterPassed)

	_, err = e.SetMerchantWorkingStatusAndCustomWorking(t.Context(), &SetMerchantWorkHoursRequest{WorkStatus: 2, CycleType: "Daily", TimeZone: "+15", StartTime: "09:00", EndTime: "18:00"})
	require.ErrorIs(t, err, errInvalidTimezone)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/account/set_merchant_work_hours",
			`{"work_status":1,"cycle_type":"","day_of_week":"","time_zone":"","start_time":"","end_time":""}`,
			`{"code":0,"data":{"work_status":1}}`)
	}
	result, err := ex.SetMerchantWorkingStatusAndCustomWorking(t.Context(), &SetMerchantWorkHoursRequest{WorkStatus: 1})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, uint64(1), result.WorkStatus)
}

func TestGetPendingP2POrders(t *testing.T) {
	t.Parallel()

	_, err := e.GetPendingP2POrders(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetPendingP2POrders(t.Context(), &PendingP2POrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.GetPendingP2POrders(t.Context(), &PendingP2POrderRequest{CryptoCurrency: currency.USDT})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.GetPendingP2POrders(t.Context(), &PendingP2POrderRequest{
		CryptoCurrency: currency.USDT,
		FiatCurrency:   currency.USD,
		StartTime:      time.Unix(1767009884, 0),
		EndTime:        time.Unix(1767009000, 0),
	})
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	if mockTests {
		currenciesOnlyExchange := newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/transaction/get_pending_transaction_list",
			`{"crypto_currency":"USDT","fiat_currency":"USD"}`,
			`{"code":0,"data":{"list":[],"trans_time":[],"count":0,"exported_num":0}}`)
		result, err := currenciesOnlyExchange.GetPendingP2POrders(t.Context(), &PendingP2POrderRequest{CryptoCurrency: currency.USDT, FiatCurrency: currency.USD})
		require.NoError(t, err)
		require.NotNil(t, result)
	}
	ex := e
	arg := &PendingP2POrderRequest{CryptoCurrency: currency.USDT, FiatCurrency: currency.USD}
	if mockTests {
		arg.TransactionID = 40000001
		arg.StartTime = time.Unix(1767009000, 0)
		arg.EndTime = time.Unix(1767009884, 0)
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/transaction/get_pending_transaction_list",
			`{"crypto_currency":"USDT","fiat_currency":"USD","txid":40000001,"start_time":1767009000,"end_time":1767009884}`,
			`{"code":0,"data":{"list":[{"cd_time":300}],"trans_time":[{"od_time":600}],"count":1,"exported_num":0}}`)
	}
	result, err := ex.GetPendingP2POrders(t.Context(), arg)
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		require.Len(t, result.List, 1)
		assert.Equal(t, uint64(300), result.List[0].CountdownSeconds)
		require.Len(t, result.TransactionCountdowns, 1)
		assert.Equal(t, uint64(600), result.TransactionCountdowns[0].CountdownSeconds)
	}
}

func TestGetHistoricalP2POrders(t *testing.T) {
	t.Parallel()
	_, err := e.GetHistoricalP2POrders(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetHistoricalP2POrders(t.Context(), &P2PCompletedOrderRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.GetHistoricalP2POrders(t.Context(), &P2PCompletedOrderRequest{CryptoCurrency: currency.USDT})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.GetHistoricalP2POrders(t.Context(), &P2PCompletedOrderRequest{
		CryptoCurrency: currency.USDT,
		FiatCurrency:   currency.USD,
		StartTime:      time.Unix(1767009000, 0),
	})
	require.ErrorIs(t, err, common.ErrDateUnset)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	if mockTests {
		currenciesOnlyExchange := newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/transaction/get_completed_transaction_list",
			`{"crypto_currency":"USDT","fiat_currency":"USD"}`,
			`{"code":0,"data":{"list":[],"trans_time":[],"count":0,"exported_num":0}}`)
		result, err := currenciesOnlyExchange.GetHistoricalP2POrders(t.Context(), &P2PCompletedOrderRequest{CryptoCurrency: currency.USDT, FiatCurrency: currency.USD})
		require.NoError(t, err)
		require.NotNil(t, result)
	}
	ex := e
	arg := &P2PCompletedOrderRequest{CryptoCurrency: currency.USDT, FiatCurrency: currency.USD}
	if mockTests {
		arg.TransactionID = 40000001
		arg.StartTime = time.Unix(1767009000, 0)
		arg.EndTime = time.Unix(1767009884, 0)
		arg.Page = 1
		arg.PerPage = 20
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/transaction/get_completed_transaction_list",
			`{"crypto_currency":"USDT","fiat_currency":"USD","txid":40000001,"start_time":1767009000,"end_time":1767009884,"page":1,"per_page":20}`,
			`{"code":0,"data":{"list":[],"trans_time":[],"count":0,"exported_num":0}}`)
	}
	result, err := ex.GetHistoricalP2POrders(t.Context(), arg)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetP2POrderDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetP2POrderDetails(t.Context(), &GetP2POrderDetailsRequest{TransactionID: 0})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/transaction/get_transaction_details",
			`{"txid":40000001}`,
			`{"code":0,"data":{"txid":40000001,"orderid":50000001,"timest":1767009000,"remain_pay_time":-3}}`)
	}
	result, err := ex.GetP2POrderDetails(t.Context(), &GetP2POrderDetailsRequest{TransactionID: 40000001})
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		assert.Equal(t, uint64(40000001), result.TransactionID)
		assert.Equal(t, uint64(50000001), result.OrderID)
		assert.Equal(t, int64(-3), result.RemainingPaymentSeconds)
	}
}

func TestConfirmP2PPayment(t *testing.T) {
	t.Parallel()
	err := e.ConfirmP2PPayment(t.Context(), &ConfirmP2PPaymentRequest{})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/transaction/confirm-payment",
			`{"txid":"40000001","payment_method":"bank"}`,
			`{"code":0,"data":{}}`)
	}
	err = ex.ConfirmP2PPayment(t.Context(), &ConfirmP2PPaymentRequest{TransactionID: "40000001", PaymentMethod: "bank"})
	require.NoError(t, err)
}

func TestConfirmP2PReceipt(t *testing.T) {
	t.Parallel()
	err := e.ConfirmP2PReceipt(t.Context(), &ConfirmP2PReceiptRequest{})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/transaction/confirm-receipt",
			`{"txid":"40000001"}`,
			`{"code":0,"data":{}}`)
	}
	err = ex.ConfirmP2PReceipt(t.Context(), &ConfirmP2PReceiptRequest{TransactionID: "40000001"})
	require.NoError(t, err)
}

func TestCancelP2POrder(t *testing.T) {
	t.Parallel()
	err := e.CancelP2POrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	err = e.CancelP2POrder(t.Context(), &CancelP2POrderRequest{})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/transaction/cancel",
			`{"txid":"100000","reason_id":"7","reason_memo":"Cancelled after agreement with the counterparty"}`,
			`{"code":0,"data":{}}`)
	}
	err = ex.CancelP2POrder(t.Context(), &CancelP2POrderRequest{TransactionID: "100000", ReasonID: "7", ReasonMemo: "Cancelled after agreement with the counterparty"})
	require.NoError(t, err)
}

func TestPublishP2PAdOrder(t *testing.T) {
	t.Parallel()
	err := e.PublishP2PAdOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	err = e.PublishP2PAdOrder(t.Context(), &PublishP2PAdRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg := &PublishP2PAdRequest{CurrencyType: currency.USDT}
	err = e.PublishP2PAdOrder(t.Context(), arg)
	require.ErrorIs(t, err, errP2PFiatUnitRequired)

	arg.ExchangeType = "USD"
	err = e.PublishP2PAdOrder(t.Context(), arg)
	require.ErrorIs(t, err, errP2PTradeTypeRequired)

	arg.Type = "0"
	err = e.PublishP2PAdOrder(t.Context(), arg)
	require.ErrorIs(t, err, errP2PUnitPriceRequired)

	arg.UnitPrice = 1.05
	err = e.PublishP2PAdOrder(t.Context(), arg)
	require.ErrorIs(t, err, errP2PAdAmountRequired)

	arg.Number = 100
	err = e.PublishP2PAdOrder(t.Context(), arg)
	require.ErrorIs(t, err, errP2PPayTypeRequired)

	arg.PaymentType = "bank"
	err = e.PublishP2PAdOrder(t.Context(), arg)
	require.ErrorIs(t, err, errP2PMinAmountRequired)

	arg.MinAmount = 10
	err = e.PublishP2PAdOrder(t.Context(), arg)
	require.ErrorIs(t, err, errP2PMaxAmountRequired)

	arg.MaxAmount = 500
	arg.FixedRate = "1"
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/books/place_biz_push_order",
			`{"currencyType":"USDT","exchangeType":"USD","type":"0","unitPrice":"1.05","number":"100","payType":"bank","rateFixed":"1","minAmount":"10","maxAmount":"500"}`,
			`{"code":0,"data":{}}`)
	}
	err = ex.PublishP2PAdOrder(t.Context(), arg)
	require.NoError(t, err)
}

func TestUpdateP2PAdStatus(t *testing.T) {
	t.Parallel()
	_, err := e.UpdateP2PAdStatus(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.UpdateP2PAdStatus(t.Context(), &UpdateP2PAdStatusRequest{AdvertisementNumber: 0, AdvertisementStatus: 1})
	require.ErrorIs(t, err, errP2PAdIDRequired)

	_, err = e.UpdateP2PAdStatus(t.Context(), &UpdateP2PAdStatusRequest{AdvertisementNumber: 2124000001, AdvertisementStatus: 2})
	require.ErrorIs(t, err, errP2PAdStatusInvalid)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/books/ads_update_status",
			`{"adv_no":2124000001,"adv_status":3}`,
			`{"code":0,"data":{"status":3}}`)
	}
	result, err := ex.UpdateP2PAdStatus(t.Context(), &UpdateP2PAdStatusRequest{AdvertisementNumber: 2124000001, AdvertisementStatus: 3})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetP2PAdDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetP2PAdDetails(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetP2PAdDetails(t.Context(), &GetP2PAdDetailsRequest{})
	require.ErrorIs(t, err, errP2PAdIDRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/books/ads_detail",
			`{"adv_no":"2124000001"}`,
			`{"code":0,"data":{"rate":"1.01","rate_ref_id":-1,"min_completed_limit":-1,"max_completed_limit":-1,"user_orders_limit":-1}}`)
	}
	result, err := ex.GetP2PAdDetails(t.Context(), &GetP2PAdDetailsRequest{AdvertisementNumber: "2124000001"})
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		assert.Equal(t, int64(-1), result.RateReferenceID)
		assert.Equal(t, int64(-1), result.MinCompletedLimit)
		assert.Equal(t, int64(-1), result.MaxCompletedLimit)
		assert.Equal(t, int64(-1), result.UserOrdersLimit)
	}
}

func TestGetMyP2PAds(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/books/my_ads_list",
			`{"asset":"USDT","fiat_unit":"USD","trade_type":"sell"}`,
			`{"code":0,"data":{"lists":[{"id":"2124000001","currencyType":"USDT","rate_ref_id":-1,"min_completed_limit":-1,"max_completed_limit":-1,"user_country_limit":-1,"user_orders_limit":-1}]}}`)
	}
	result, err := ex.GetMyP2PAds(t.Context(), &GetMyP2PAdsRequest{Asset: currency.USDT, FiatUnit: "USD", TradeType: "sell"})
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		require.Len(t, result.Lists, 1)
		assert.Equal(t, int64(-1), result.Lists[0].RateReferenceID)
		assert.Equal(t, int64(-1), result.Lists[0].UserCountryLimit)
	}
}

func TestGetP2PAdList(t *testing.T) {
	t.Parallel()
	_, err := e.GetP2PAdList(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetP2PAdList(t.Context(), &GetP2PAdsListRequest{})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.GetP2PAdList(t.Context(), &GetP2PAdsListRequest{Asset: currency.USDT})
	require.ErrorIs(t, err, errP2PFiatUnitRequired)

	_, err = e.GetP2PAdList(t.Context(), &GetP2PAdsListRequest{Asset: currency.USDT, FiatUnit: "USD"})
	require.ErrorIs(t, err, errP2PTradeTypeRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/books/ads_list",
			`{"asset":"USDT","fiat_unit":"USD","trade_type":"sell"}`,
			`{"code":0,"data":[{"index":1,"asset":"USDT","fiat_unit":"USD","adv_no":2124000001,"surplus_amount":"100","trade_methods":[]}]}`)
	}
	result, err := ex.GetP2PAdList(t.Context(), &GetP2PAdsListRequest{Asset: currency.USDT, FiatUnit: "USD", TradeType: "sell"})
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		require.Len(t, result, 1)
		assert.Equal(t, uint64(2124000001), result[0].AdvertisementNumber)
	}
}

func TestGetP2PChatHistory(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/chat/get_chats_list",
			`{"txid":40000001,"lastreceived":1767009884,"firstreceived":1767009000}`,
			`{"code":0,"data":{"messages":[],"memo":"","has_history":false,"txid":40000001,"SRVTM":1767009884,"order_status":"pending"}}`)
	}
	result, err := ex.GetP2PChatHistory(t.Context(), 40000001, 1767009884, 1767009000)
	require.NoError(t, err)
	require.NotNil(t, result)
	if mockTests {
		assert.Equal(t, uint64(40000001), result.TransactionID)
	}
}

func TestSendP2PChatMessage(t *testing.T) {
	t.Parallel()
	_, err := e.SendP2PChatMessage(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.SendP2PChatMessage(t.Context(), &SendP2PChatMessageRequest{})
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	_, err = e.SendP2PChatMessage(t.Context(), &SendP2PChatMessageRequest{TransactionID: 40000001})
	require.ErrorIs(t, err, errP2PMessageRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/chat/send_chat_message",
			`{"txid":40000001,"message":"Payment completed, please check"}`,
			`{"code":0,"data":{"SRVTM":1767009884,"txid":40000001,"conversation_id":"conversation-1","msg_type":0,"risk_type":0,"toast_msg":""}}`)
	}
	result, err := ex.SendP2PChatMessage(t.Context(), &SendP2PChatMessageRequest{TransactionID: 40000001, Message: "Payment completed, please check"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestUploadP2PChatFile(t *testing.T) {
	t.Parallel()
	_, err := e.UploadP2PChatFile(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.UploadP2PChatFile(t.Context(), &UploadP2PChatFileRequest{})
	require.ErrorIs(t, err, errP2PImageTypeRequired)

	_, err = e.UploadP2PChatFile(t.Context(), &UploadP2PChatFileRequest{ImageContentType: "image/png"})
	require.ErrorIs(t, err, errP2PImageDataRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	ex := e
	if mockTests {
		ex = newAuthenticatedP2PRouteTestExchange(t,
			"/api/v4/p2p/merchant/chat/upload_chat_file",
			`{"image_content_type":"image/png","base64_img":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}`,
			`{"code":0,"data":{"file_key":"p2p-file-1"}}`)
	}
	result, err := ex.UploadP2PChatFile(t.Context(), &UploadP2PChatFileRequest{
		ImageContentType: "image/png",
		Base64Image:      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}
