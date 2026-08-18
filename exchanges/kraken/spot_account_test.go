package kraken

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

var spotAccountFixtures = spotFixtureSet{
	results: map[string]string{
		"/0/private/Balance":       `{"XXBT":"1.25"}`,
		"/0/private/BalanceEx":     `{"XXBT":{"balance":1.25,"credit":"0.50","credit_used":0.10,"hold_trade":"0.25"},"UNKNOWN":{"balance":"2","credit":"0","credit_used":"0","hold_trade":"0"}}`,
		"/0/private/CreditLines":   `{"asset_details":{"USD":{"balance":"100","hold_trade":"5","collateral_value":0.99,"credit_limit":"50","credit_used":"10","available_credit":"40"}},"limits_monitor":{"total_credit_usd":"50"}}`,
		"/0/private/TradeBalance":  `{"eb":"1101.3425","tb":"392.2264","m":"7.0354","n":"-10.0232","c":"21.1063","v":"31.1297","e":"382.2032","mf":"375.1678","mfo":"374","ml":"5432.57","uv":"2"}`,
		"/0/private/OpenOrders":    `{"open":{"ORDER":{"refid":"REF","userref":1,"cl_ord_id":"CLIENT","status":"open","opentm":1695828271,"starttm":0,"expiretm":0,"descr":{"pair":"BTC/USD","type":"buy","ordertype":"limit","price":"100","price2":"0","leverage":"none","order":"buy 1 BTC/USD","close":"","aclass":"currency"},"time_in_force":"gtc","vol":"1","vol_exec":"0","cost":"0","fee":"0","price":"0","stopprice":"0","limitprice":"0","trigger":"last","margin":false,"misc":"","oflags":"post","trades":[],"sender_sub_id":"SUB"}}}`,
		"/0/private/ClosedOrders":  `{"closed":{"ORDER":{"status":"closed","reason":"User requested","descr":{"pair":"BTC/USD","aclass":"currency"},"time_in_force":"fok","vol":"1","vol_exec":"1","cost":"100","fee":"0.2","price":"100","stopprice":"0","limitprice":"0","trigger":"last","margin":true,"sender_sub_id":"SUB"}},"count":1}`,
		"/0/private/QueryOrders":   `{"ORDER":{"status":"closed","descr":{"pair":"BTC/USD","aclass":"currency"},"time_in_force":"ioc","vol":"1","vol_exec":"1","cost":"100","fee":"0.2","price":"100","stopprice":"0","limitprice":"0","trigger":"last","margin":false,"sender_sub_id":"SUB"}}`,
		"/0/private/TradesHistory": `{"trades":{"TRADE":{"ordertxid":"ORDER","postxid":"POSITION","pair":"BTC/USD","time":1695828271,"type":"buy","ordertype":"limit","price":"100","cost":"100","fee":"0.2","vol":"1","margin":"0","leverage":"2","misc":"closing","cprice":101.5,"ccost":100.5,"cfee":0.1,"cvol":1,"cmargin":20,"net":1.5,"trades":["CLOSE"],"ledgers":["LEDGER"],"trade_id":42,"maker":true,"aclass":"currency","tradeordertype":"market","posstatus":"closed"}},"count":1}`,
		"/0/private/QueryTrades":   `{"TRADE":{"ordertxid":"ORDER","postxid":"POSITION","pair":"BTC/USD","time":1695828271,"type":"buy","ordertype":"limit","price":"100","cost":"100","fee":"0.2","vol":"1","margin":"0","leverage":"2","misc":"closing","cprice":"101.5","ccost":"100.5","cfee":"0.1","cvol":"1","cmargin":"20","net":"1.5","trades":["CLOSE"],"ledgers":["LEDGER"],"trade_id":42,"maker":true,"aclass":"currency","tradeordertype":"market","posstatus":"closed"}}`,
		"/0/private/OpenPositions": `{"POSITION":{"ordertxid":"ORDER","class":"currency","pair":"BTC/USD","time":1695828271,"type":"buy","ordertype":"limit","cost":"100","fee":"0.2","vol":"1","vol_closed":"0","margin":"20","value":120,"rollovertm":"0","misc":"","oflags":""}}`,
		"/0/private/QueryLedgers":  `{"LEDGER":{"refid":"REFERENCE","time":1695828271,"type":"trade","subtype":"spotfromfutures","aclass":"currency","asset":"BTC","amount":"1","fee":"0","balance":"1"}}`,
		"/0/private/TradeVolume":   `{"currency":"USD","asset_class":"currency","volume":"200","inputs":{"domain_spot_volume_30d":"200","domain_futures_volume_30d":"10","domain_assets_on_platform":"1000"},"fees":{"BTC/USD":{"fee":"0.2","minfee":"0.1","maxfee":"0.3","nextfee":null,"tiervolume":"100","tierfuturesvolume":null,"nextvolume":null,"nextfuturesvolume":null,"volumeoffset":"5"}},"fees_maker":{"BTC/USD":{"fee":"0.1","minfee":"0.05","maxfee":"0.2","nextfee":"0.08","tiervolume":"100","tierfuturesvolume":"50","nextvolume":"200","nextfuturesvolume":"100","volumeoffset":null}},"volume_subaccounts":[{"iiban":"SUB","volume":"50"}],"schedules":[{"pair":"BTC/USD","class":"forex","tiers":[{"maker_fee":"0.1","taker_fee":"0.2","min_spot_volume":"100","min_futures_volume":null,"min_assets_on_platform":"500","active":true}]}]}`,
		"/0/private/OrderAmends":   `{"count":1,"amends":[{"amend_id":"AMEND","amend_type":"original","order_qty":"1","timestamp":1724158070287558000}]}`,
		"/0/private/AddExport":     `{"id":"REPORT"}`,
		"/0/private/ExportStatus":  `[{"id":"REPORT","status":"Processed","error":"","asset_classes":["currency"],"endtm":"1688669085","delete":false}]`,
		"/0/private/RemoveExport":  `{"delete":true,"cancel":false}`,
		"/0/private/GetApiKeyInfo": `{"apiKeyName":"spot","apiKey":"key","nonce":"1","nonceWindow":0,"permissions":["earn-funds"],"validUntil":"0","queryFrom":"0","queryTo":"0","createdTime":"1772542900","modifiedTime":"1772543095","lastUsed":null}`,
		"/0/private/Ledgers":       `{"ledger":{"LEDGER":{"refid":"REFERENCE","time":1695828271,"type":"trade","subtype":"spotfromfutures","aclass":"currency","asset":"XXBT","amount":"1","fee":"0","balance":"1"}},"count":1}`,
	},
	responses: map[string]spotHTTPResponse{
		"/0/private/RetrieveExport": {contentType: "application/octet-stream", body: "PK\x03\x04export"},
	},
}

func TestFormatTimeOrTransactionID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		value         TimeOrTransactionID
		expected      string
		expectedError error
	}{
		{name: "omitted"},
		{name: "transaction ID", value: TimeOrTransactionID{TransactionID: "TX"}, expected: "TX"},
		{name: "epoch", value: TimeOrTransactionID{Time: time.Unix(0, 0)}, expected: "0"},
		{name: "timestamp", value: TimeOrTransactionID{Time: time.Unix(123, 0)}, expected: "123"},
		{name: "conflicting representations", value: TimeOrTransactionID{Time: time.Unix(123, 0), TransactionID: "TX"}, expectedError: errTimeOrIDConflict},
		{name: "pre-epoch timestamp", value: TimeOrTransactionID{Time: time.Unix(-1, 0)}, expectedError: errTimestampInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := formatTimeOrTransactionID(tc.value)
			require.ErrorIs(t, err, tc.expectedError, "formatTimeOrTransactionID must return the expected error")
			assert.Equal(t, tc.expected, result, "formatTimeOrTransactionID should return the expected wire value")
		})
	}
}

func TestGetAccountBalance(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.GetAccountBalance(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetAccountBalance must reject a nil request")
	_, err = ex.GetAccountBalance(ctx, &GetAccountBalanceRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetAccountBalance must reject an invalid rebase multiplier")
	balance, err := ex.GetAccountBalance(ctx, &GetAccountBalanceRequest{RebaseMultiplier: "base"})
	require.NoError(t, err, "GetAccountBalance must not error")
	assert.Equal(t, 1.25, balance["XXBT"].Float64(), "GetAccountBalance should decode balances")
	values := requireSpotRequest(t, requests, "/0/private/Balance")
	assert.Equal(t, "base", values.Get("rebase_multiplier"), "GetAccountBalance should encode the rebase multiplier")

	balance, err = newSpotErrorExchange(t).GetAccountBalance(ctx, &GetAccountBalanceRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetAccountBalance must surface request errors")
	assert.Nil(t, balance, "GetAccountBalance result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetAccountBalance(t.Context(), new(GetAccountBalanceRequest))
		require.NoError(t, err, "GetAccountBalance must not error against the live API")
		require.NotNil(t, response, "GetAccountBalance must return a response from the live API")
	})
}

func TestGetExtendedBalance(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.GetExtendedBalance(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetExtendedBalance must reject a nil request")
	_, err = ex.GetExtendedBalance(ctx, &GetExtendedBalanceRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetExtendedBalance must reject an invalid rebase multiplier")
	extended, err := ex.GetExtendedBalance(ctx, &GetExtendedBalanceRequest{RebaseMultiplier: "rebased"})
	require.NoError(t, err, "GetExtendedBalance must not error")
	assert.Equal(t, 1.25, extended["XXBT"].Balance.Float64(), "GetExtendedBalance should decode numeric balances")
	assert.Equal(t, 0.5, extended["XXBT"].Credit.Float64(), "GetExtendedBalance should decode credit")
	assert.Equal(t, 0.1, extended["XXBT"].CreditUsed.Float64(), "GetExtendedBalance should decode used credit")
	assert.Equal(t, 0.25, extended["XXBT"].HoldTrade.Float64(), "GetExtendedBalance should decode held balances")
	values := requireSpotRequest(t, requests, "/0/private/BalanceEx")
	assert.Equal(t, "rebased", values.Get("rebase_multiplier"), "GetExtendedBalance should encode the rebase multiplier")

	extended, err = newSpotErrorExchange(t).GetExtendedBalance(ctx, &GetExtendedBalanceRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetExtendedBalance must surface request errors")
	assert.Nil(t, extended, "GetExtendedBalance result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetExtendedBalance(t.Context(), new(GetExtendedBalanceRequest))
		require.NoError(t, err, "GetExtendedBalance must not error against the live API")
		require.NotNil(t, response, "GetExtendedBalance must return a response from the live API")
	})
}

func TestGetCreditLines(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.GetCreditLines(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetCreditLines must reject a nil request")
	_, err = ex.GetCreditLines(ctx, &GetCreditLinesRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetCreditLines must reject an invalid rebase multiplier")
	credit, err := ex.GetCreditLines(ctx, &GetCreditLinesRequest{RebaseMultiplier: "base"})
	require.NoError(t, err, "GetCreditLines must not error")
	require.NotNil(t, credit, "GetCreditLines must return a response")
	assert.Equal(t, 40.0, credit.AssetDetails["USD"].AvailableCredit.Float64(), "GetCreditLines should decode available credit")
	assert.Equal(t, 5.0, credit.AssetDetails["USD"].HoldTrade.Float64(), "GetCreditLines should decode held balances")
	assert.Equal(t, 0.99, credit.AssetDetails["USD"].CollateralValue.Float64(), "GetCreditLines should decode collateral value")
	responseJSON, err := json.Marshal(credit)
	require.NoError(t, err, "GetCreditLines must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"asset_details":{"USD"`, "GetCreditLines should decode the response")
	requireSpotRequest(t, requests, "/0/private/CreditLines")

	credit, err = newSpotNullResultExchange(t).GetCreditLines(ctx, &GetCreditLinesRequest{})
	require.NoError(t, err, "GetCreditLines must accept a null result")
	assert.Nil(t, credit, "GetCreditLines should return nil for a null result")
	credit, err = newSpotErrorExchange(t).GetCreditLines(ctx, &GetCreditLinesRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetCreditLines must surface request errors")
	assert.Nil(t, credit, "GetCreditLines result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetCreditLines(t.Context(), new(GetCreditLinesRequest))
		require.NoError(t, err, "GetCreditLines must not error against the live API")
		require.NotNil(t, response, "GetCreditLines must return a response from the live API")
	})
}

func TestGetOrderAmends(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.GetOrderAmends(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetOrderAmends must reject a nil request")
	_, err = ex.GetOrderAmends(ctx, &GetOrderAmendsRequest{})
	require.ErrorIs(t, err, errOrderIDRequired, "GetOrderAmends must require an order identifier")
	_, err = ex.GetOrderAmends(ctx, &GetOrderAmendsRequest{OrderID: "ORDER", RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetOrderAmends must reject an invalid rebase multiplier")
	amends, err := ex.GetOrderAmends(ctx, &GetOrderAmendsRequest{OrderID: "ORDER", RebaseMultiplier: "base"})
	require.NoError(t, err, "GetOrderAmends must not error")
	require.NotNil(t, amends, "GetOrderAmends must return a response")
	assert.Equal(t, int64(1724158070287558000), amends.Amends[0].Timestamp.Time().UnixNano(), "GetOrderAmends should retain nanosecond timestamps")
	responseJSON, err := json.Marshal(amends)
	require.NoError(t, err, "GetOrderAmends must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"amend_id":"AMEND"`, "GetOrderAmends should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/OrderAmends")
	assert.Equal(t, "ORDER", values.Get("order_id"), "GetOrderAmends should encode the order identifier")

	amends, err = newSpotNullResultExchange(t).GetOrderAmends(ctx, &GetOrderAmendsRequest{OrderID: "ORDER"})
	require.NoError(t, err, "GetOrderAmends must accept a null result")
	assert.Nil(t, amends, "GetOrderAmends should return nil for a null result")
	amends, err = newSpotErrorExchange(t).GetOrderAmends(ctx, &GetOrderAmendsRequest{OrderID: "ORDER"})
	require.ErrorIs(t, err, errSpotTransport, "GetOrderAmends must surface request errors")
	assert.Nil(t, amends, "GetOrderAmends result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		orderID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_ORDER_ID")
		response, err := spotLiveExchange.GetOrderAmends(t.Context(), &GetOrderAmendsRequest{OrderID: orderID})
		require.NoError(t, err, "GetOrderAmends must not error against the live API")
		require.NotNil(t, response, "GetOrderAmends must return a response from the live API")
		require.NotNil(t, response.Amends, "GetOrderAmends live response must include amendments")
	})
}

func TestRequestExportReport(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.RequestExportReport(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "RequestExportReport must reject a nil request")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{})
	require.ErrorIs(t, err, errReportRequired, "RequestExportReport must require a report type")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: "invalid"})
	require.ErrorIs(t, err, errExportReportInvalid, "RequestExportReport must reject an invalid report type")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: "trades"})
	require.ErrorIs(t, err, errDescriptionRequired, "RequestExportReport must require a description")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: "trades", Description: "annual", Format: "JSON"})
	require.ErrorIs(t, err, errExportFormatInvalid, "RequestExportReport must reject an invalid format")
	start := time.Unix(1, 0)
	end := time.Unix(2, 0)
	preEpoch := time.Unix(-1, 0)
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Description: "annual", Fields: []ExportField{ExportFieldAmount}})
	require.ErrorIs(t, err, errExportFieldInvalid, "RequestExportReport must reject fields from a different report type")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Description: "annual", StartTime: preEpoch})
	require.ErrorIs(t, err, errTimestampInvalid, "RequestExportReport must reject a pre-epoch start time")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Description: "annual", EndTime: preEpoch})
	require.ErrorIs(t, err, errTimestampInvalid, "RequestExportReport must reject a pre-epoch end time")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Description: "annual", StartTime: end, EndTime: start})
	require.ErrorIs(t, err, errTimeRangeInvalid, "RequestExportReport must reject a reversed time range")
	report, err := ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportTrades, Format: ExportFormatTSV, Description: "annual", Fields: []ExportField{ExportFieldTime, ExportFieldPrice}, StartTime: start, EndTime: end})
	require.NoError(t, err, "RequestExportReport must not error")
	require.NotNil(t, report, "RequestExportReport must return a response")
	assert.Equal(t, "REPORT", report.ID, "RequestExportReport should decode the report identifier")
	responseJSON, err := json.Marshal(report)
	require.NoError(t, err, "RequestExportReport must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"id":"REPORT"`, "RequestExportReport should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/AddExport")
	assert.Equal(t, "annual", values.Get("description"), "RequestExportReport should encode description")
	assert.Equal(t, "TSV", values.Get("format"), "RequestExportReport should encode format")
	assert.Equal(t, "time,price", values.Get("fields"), "RequestExportReport should encode selected fields")
	assert.Equal(t, "1", values.Get("starttm"), "RequestExportReport should encode start time")
	assert.Equal(t, "2", values.Get("endtm"), "RequestExportReport should encode end time")
	_, err = ex.RequestExportReport(ctx, &RequestExportReportRequest{Report: ExportReportLedgers, Description: "ledger", Fields: []ExportField{ExportFieldAmount, ExportFieldWallet}})
	require.NoError(t, err, "RequestExportReport must accept ledger fields")
	values = requireSpotRequest(t, requests, "/0/private/AddExport")
	assert.Equal(t, "amount,wallet", values.Get("fields"), "RequestExportReport should encode ledger fields")

	report, err = newSpotNullResultExchange(t).RequestExportReport(ctx, &RequestExportReportRequest{Report: "trades", Description: "test"})
	require.NoError(t, err, "RequestExportReport must accept a null result")
	assert.Nil(t, report, "RequestExportReport should return nil for a null result")
	report, err = newSpotErrorExchange(t).RequestExportReport(ctx, &RequestExportReportRequest{Report: "trades", Description: "test"})
	require.ErrorIs(t, err, errSpotTransport, "RequestExportReport must surface request errors")
	assert.Nil(t, report, "RequestExportReport result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLiveExportRequest)
		response, err := spotLiveExchange.RequestExportReport(t.Context(), &RequestExportReportRequest{Report: ExportReportTrades, Description: "GoCryptoTrader live test"})
		require.NoError(t, err, "RequestExportReport must not error against the live API")
		require.NotNil(t, response, "RequestExportReport must return a response from the live API")
		require.NotEmpty(t, response.ID, "RequestExportReport live response must include the report identifier")
		removed, err := spotLiveExchange.DeleteExportReport(t.Context(), &DeleteExportReportRequest{ID: response.ID, Type: ExportRemovalCancel})
		require.NoError(t, err, "RequestExportReport live cleanup must cancel the export")
		require.NotNil(t, removed, "RequestExportReport live cleanup must return a response")
		require.True(t, removed.Cancel, "RequestExportReport live cleanup must confirm cancellation")
	})
}

func TestGetExportReportStatus(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.GetExportReportStatus(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetExportReportStatus must reject a nil request")
	_, err = ex.GetExportReportStatus(ctx, &GetExportReportStatusRequest{})
	require.ErrorIs(t, err, errReportRequired, "GetExportReportStatus must require a report type")
	_, err = ex.GetExportReportStatus(ctx, &GetExportReportStatusRequest{Report: "invalid"})
	require.ErrorIs(t, err, errExportReportInvalid, "GetExportReportStatus must reject an invalid report type")
	reports, err := ex.GetExportReportStatus(ctx, &GetExportReportStatusRequest{Report: "trades"})
	require.NoError(t, err, "GetExportReportStatus must not error")
	assert.Equal(t, []string{"currency"}, reports[0].AssetClasses, "GetExportReportStatus should decode current asset classes")
	assert.Equal(t, int64(1688669085), reports[0].EndTime.Time().Unix(), "GetExportReportStatus should decode report timestamps")
	requireSpotRequest(t, requests, "/0/private/ExportStatus")

	reports, err = newSpotErrorExchange(t).GetExportReportStatus(ctx, &GetExportReportStatusRequest{Report: "trades"})
	require.ErrorIs(t, err, errSpotTransport, "GetExportReportStatus must surface request errors")
	assert.Nil(t, reports, "GetExportReportStatus result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetExportReportStatus(t.Context(), &GetExportReportStatusRequest{Report: ExportReportTrades})
		require.NoError(t, err, "GetExportReportStatus must not error against the live API")
		require.NotNil(t, response, "GetExportReportStatus must return a response from the live API")
	})
}

func TestRetrieveDataExport(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.RetrieveDataExport(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "RetrieveDataExport must reject a nil request")
	_, err = ex.RetrieveDataExport(ctx, &RetrieveDataExportRequest{})
	require.ErrorIs(t, err, errIDRequired, "RetrieveDataExport must require an identifier")
	archive, err := ex.RetrieveDataExport(ctx, &RetrieveDataExportRequest{ID: "REPORT"})
	require.NoError(t, err, "RetrieveDataExport must not error")
	assert.Equal(t, "PK\x03\x04export", string(archive), "RetrieveDataExport should preserve binary response bytes")
	requireSpotRequest(t, requests, "/0/private/RetrieveExport")

	archive, err = newSpotErrorExchange(t).RetrieveDataExport(ctx, &RetrieveDataExportRequest{ID: "REPORT"})
	require.ErrorIs(t, err, errSpotTransport, "RetrieveDataExport must surface request errors")
	assert.Nil(t, archive, "RetrieveDataExport result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		reportID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_EXPORT_ID")
		response, err := spotLiveExchange.RetrieveDataExport(t.Context(), &RetrieveDataExportRequest{ID: reportID})
		require.NoError(t, err, "RetrieveDataExport must not error against the live API")
		require.NotEmpty(t, response, "RetrieveDataExport must return report data from the live API")
	})
}

func TestDeleteExportReport(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.DeleteExportReport(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "DeleteExportReport must reject a nil request")
	_, err = ex.DeleteExportReport(ctx, &DeleteExportReportRequest{})
	require.ErrorIs(t, err, errIDRequired, "DeleteExportReport must require an identifier")
	_, err = ex.DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT"})
	require.ErrorIs(t, err, errTypeRequired, "DeleteExportReport must require a removal type")
	_, err = ex.DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT", Type: "invalid"})
	require.ErrorIs(t, err, errExportRemovalInvalid, "DeleteExportReport must reject an invalid removal type")
	deleted, err := ex.DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT", Type: "delete"})
	require.NoError(t, err, "DeleteExportReport must not error")
	require.NotNil(t, deleted, "DeleteExportReport must return a response")
	assert.True(t, deleted.Delete, "DeleteExportReport should decode deletion status")
	responseJSON, err := json.Marshal(deleted)
	require.NoError(t, err, "DeleteExportReport must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"delete":true`, "DeleteExportReport should decode the response")
	requireSpotRequest(t, requests, "/0/private/RemoveExport")

	deleted, err = newSpotNullResultExchange(t).DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT", Type: "delete"})
	require.NoError(t, err, "DeleteExportReport must accept a null result")
	assert.Nil(t, deleted, "DeleteExportReport should return nil for a null result")
	deleted, err = newSpotErrorExchange(t).DeleteExportReport(ctx, &DeleteExportReportRequest{ID: "REPORT", Type: "delete"})
	require.ErrorIs(t, err, errSpotTransport, "DeleteExportReport must surface request errors")
	assert.Nil(t, deleted, "DeleteExportReport result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLiveExportDeletion)
		reportID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_DELETE_EXPORT_ID")
		response, err := spotLiveExchange.DeleteExportReport(t.Context(), &DeleteExportReportRequest{ID: reportID, Type: ExportRemovalDelete})
		require.NoError(t, err, "DeleteExportReport must not error against the live API")
		require.NotNil(t, response, "DeleteExportReport must return a response from the live API")
		require.True(t, response.Delete, "DeleteExportReport live response must confirm deletion")
	})
}

func TestGetAPIKeyInfo(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.GetAPIKeyInfo(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetAPIKeyInfo must reject a nil request")
	keyInfo, err := ex.GetAPIKeyInfo(ctx, &GetAPIKeyInfoRequest{OTP: "123456"})
	require.NoError(t, err, "GetAPIKeyInfo must not error")
	require.NotNil(t, keyInfo, "GetAPIKeyInfo must return a response")
	assert.Equal(t, "spot", keyInfo.APIKeyName, "GetAPIKeyInfo should decode the key name")
	assert.Nil(t, keyInfo.LastUsed, "GetAPIKeyInfo should decode a null last-used timestamp")
	assert.Equal(t, int64(1772542900), keyInfo.CreatedTime.Time().Unix(), "GetAPIKeyInfo should decode creation time")
	responseJSON, err := json.Marshal(keyInfo)
	require.NoError(t, err, "GetAPIKeyInfo must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"apiKeyName":"spot"`, "GetAPIKeyInfo should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/GetApiKeyInfo")
	assert.Equal(t, "123456", values.Get("otp"), "GetAPIKeyInfo should encode OTP")

	keyInfo, err = newSpotNullResultExchange(t).GetAPIKeyInfo(ctx, &GetAPIKeyInfoRequest{})
	require.NoError(t, err, "GetAPIKeyInfo must accept a null result")
	assert.Nil(t, keyInfo, "GetAPIKeyInfo should return nil for a null result")
	keyInfo, err = newSpotErrorExchange(t).GetAPIKeyInfo(ctx, &GetAPIKeyInfoRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetAPIKeyInfo must surface request errors")
	assert.Nil(t, keyInfo, "GetAPIKeyInfo result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetAPIKeyInfo(t.Context(), new(GetAPIKeyInfoRequest))
		require.NoError(t, err, "GetAPIKeyInfo must not error against the live API")
		require.NotNil(t, response, "GetAPIKeyInfo must return a response from the live API")
		require.NotEmpty(t, response.APIKey, "GetAPIKeyInfo live response must include the API key identifier")
	})
}

func TestGetLedgers(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.GetLedgers(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetLedgers must reject a nil request")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Type: "invalid"})
	require.ErrorIs(t, err, errLedgerTypeInvalid, "GetLedgers must reject an invalid ledger type")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{RebaseMultiplier: "invalid"})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetLedgers must reject an invalid rebase multiplier")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Assets: []string{""}})
	require.ErrorIs(t, err, errAssetRequired, "GetLedgers must reject an empty asset")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Start: TimeOrTransactionID{Time: time.Unix(1, 0), TransactionID: "LEDGER"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetLedgers must reject conflicting start values")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{End: TimeOrTransactionID{Time: time.Unix(1, 0), TransactionID: "LEDGER"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetLedgers must reject conflicting end values")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Start: TimeOrTransactionID{Time: time.Unix(2, 0)}, End: TimeOrTransactionID{Time: time.Unix(1, 0)}})
	require.ErrorIs(t, err, errTimeRangeInvalid, "GetLedgers must reject a reversed time range")
	ledgers, err := ex.GetLedgers(ctx, new(GetLedgersRequest))
	require.NoError(t, err, "GetLedgers must allow omitted options")
	require.NotNil(t, ledgers, "GetLedgers must return a response")
	assert.Equal(t, int64(1), ledgers.Count, "GetLedgers should decode the ledger count")
	assert.Equal(t, "spotfromfutures", ledgers.Ledger["LEDGER"].Subtype, "GetLedgers should decode ledger subtype")
	responseJSON, err := json.Marshal(ledgers)
	require.NoError(t, err, "GetLedgers must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"ledger":{"LEDGER"`, "GetLedgers should decode the response")
	requireSpotRequest(t, requests, "/0/private/Ledgers")
	ledgers, err = ex.GetLedgers(ctx, &GetLedgersRequest{
		AssetClass:       AssetClassCurrency,
		Assets:           []string{"XBT", "USD"},
		Type:             LedgerTypeTrade,
		Start:            TimeOrTransactionID{TransactionID: "1"},
		End:              TimeOrTransactionID{TransactionID: "2"},
		Offset:           3,
		WithoutCount:     true,
		RebaseMultiplier: RebaseMultiplierBase,
	})
	require.NoError(t, err, "GetLedgers must not error")
	assert.Equal(t, "REFERENCE", ledgers.Ledger["LEDGER"].Refid, "GetLedgers should decode ledger entries")
	values := requireSpotRequest(t, requests, "/0/private/Ledgers")
	assert.Equal(t, "currency", values.Get("aclass"), "GetLedgers should encode asset class")
	assert.Equal(t, "XBT,USD", values.Get("asset"), "GetLedgers should encode assets")
	assert.Equal(t, "trade", values.Get("type"), "GetLedgers should encode ledger type")
	assert.Equal(t, "1", values.Get("start"), "GetLedgers should encode start")
	assert.Equal(t, "2", values.Get("end"), "GetLedgers should encode end")
	assert.Equal(t, "3", values.Get("ofs"), "GetLedgers should encode offset")
	assert.Equal(t, "true", values.Get("without_count"), "GetLedgers should encode count suppression")
	assert.Equal(t, "base", values.Get("rebase_multiplier"), "GetLedgers should encode rebase multiplier")
	_, err = ex.GetLedgers(ctx, &GetLedgersRequest{Start: TimeOrTransactionID{Time: time.Unix(1, 0)}, End: TimeOrTransactionID{Time: time.Unix(2, 0)}})
	require.NoError(t, err, "GetLedgers must accept timestamp bounds")
	values = requireSpotRequest(t, requests, "/0/private/Ledgers")
	assert.Equal(t, "1", values.Get("start"), "GetLedgers should encode the start timestamp")
	assert.Equal(t, "2", values.Get("end"), "GetLedgers should encode the end timestamp")
	for _, ledgerType := range []LedgerType{LedgerTypeAll, LedgerTypeTrade, LedgerTypeDeposit, LedgerTypeWithdrawal, LedgerTypeTransfer, LedgerTypeMargin, LedgerTypeAdjustment, LedgerTypeRollover, LedgerTypeCredit, LedgerTypeSettled, LedgerTypeStaking, LedgerTypeDividend, LedgerTypeSale, LedgerTypeNFTRebate} {
		t.Run(string(ledgerType), func(t *testing.T) {
			_, err := ex.GetLedgers(t.Context(), &GetLedgersRequest{Type: ledgerType})
			require.NoError(t, err, "GetLedgers must accept the documented ledger type")
			values := requireSpotRequest(t, requests, "/0/private/Ledgers")
			assert.Equal(t, string(ledgerType), values.Get("type"), "GetLedgers should encode the documented ledger type")
		})
	}
	ledgers, err = newSpotNullResultExchange(t).GetLedgers(ctx, &GetLedgersRequest{})
	require.NoError(t, err, "GetLedgers must accept a null result")
	assert.Nil(t, ledgers, "GetLedgers should return nil for a null result")
	ledgers, err = newSpotErrorExchange(t).GetLedgers(ctx, &GetLedgersRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetLedgers must surface request errors")
	assert.Nil(t, ledgers, "GetLedgers result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetLedgers(t.Context(), new(GetLedgersRequest))
		require.NoError(t, err, "GetLedgers must not error against the live API")
		require.NotNil(t, response, "GetLedgers must return a response from the live API")
		require.NotNil(t, response.Ledger, "GetLedgers live response must include a ledger map")
	})
}

func TestTradeInfoJSONUnmarshal(t *testing.T) {
	var trade TradeInfo
	require.Error(t, json.Unmarshal([]byte(`{`), &trade), "json.Unmarshal must reject malformed trade info JSON")
	require.Error(t, json.Unmarshal([]byte(`{"cprice":{}}`), &trade), "json.Unmarshal must reject an invalid closed-position value")
	require.NoError(t, json.Unmarshal([]byte(`{"price":"100","cost":"100","fee":"0.2","vol":"1","margin":"0","leverage":"2","cprice":101.5,"ccost":100.5,"cfee":0.1,"cvol":1,"cmargin":20,"net":1.5}`), &trade), "json.Unmarshal must decode numeric closed-position values")
	assert.Equal(t, 101.5, trade.ClosedPositionAveragePrice.Float64(), "TradeInfo.ClosedPositionAveragePrice should decode correctly")
	assert.Equal(t, 100.5, trade.ClosedPositionCost.Float64(), "TradeInfo.ClosedPositionCost should decode correctly")
	require.NoError(t, json.Unmarshal([]byte(`{"price":"100","cost":"100","fee":"0.2","vol":"1","margin":"0","cprice":"101.5","cfee":"0.1","cvol":"1","cmargin":"20"}`), &trade), "json.Unmarshal must decode quoted closed-position values")
	assert.Equal(t, 20.0, trade.ClosedPositionMargin.Float64(), "TradeInfo.ClosedPositionMargin should decode correctly")
}

func TestTradeVolumeFeeJSONUnmarshal(t *testing.T) {
	var fee TradeVolumeFee
	require.Error(t, json.Unmarshal([]byte(`{`), &fee), "json.Unmarshal must reject malformed fee info JSON")
	require.Error(t, json.Unmarshal([]byte(`{"fee":"0.2","volumeoffset":{}}`), &fee), "json.Unmarshal must reject an invalid fee-tier value")
	require.NoError(t, json.Unmarshal([]byte(`{"fee":"0.2","minfee":"0.1","maxfee":"0.3","nextfee":null,"tiervolume":"100","tierfuturesvolume":null,"nextvolume":null,"nextfuturesvolume":null,"volumeoffset":"5"}`), &fee), "json.Unmarshal must decode nullable fee-tier values")
	assert.Nil(t, fee.NextFee, "TradeVolumeFee.NextFee should preserve a null value")
	assert.Nil(t, fee.NextVolume, "TradeVolumeFee.NextVolume should preserve a null value")
	require.NotNil(t, fee.VolumeOffset, "TradeVolumeFee.VolumeOffset must decode a non-null value")
	assert.Equal(t, 5.0, fee.VolumeOffset.Float64(), "TradeVolumeFee.VolumeOffset should decode correctly")
	require.NoError(t, json.Unmarshal([]byte(`{"fee":"0.2","minfee":"0.1","maxfee":"0.3","nextfee":"0.15","tiervolume":"100","tierfuturesvolume":"50","nextvolume":"200","nextfuturesvolume":"100","volumeoffset":null}`), &fee), "json.Unmarshal must decode non-null fee-tier values")
	require.NotNil(t, fee.NextFee, "TradeVolumeFee.NextFee must decode a non-null value")
	assert.Equal(t, 0.15, fee.NextFee.Float64(), "TradeVolumeFee.NextFee should decode correctly")
	require.NotNil(t, fee.NextVolume, "TradeVolumeFee.NextVolume must decode a non-null value")
	assert.Equal(t, 200.0, fee.NextVolume.Float64(), "TradeVolumeFee.NextVolume should decode correctly")
}

func TestGetTradeBalance(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.GetTradeBalance(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetTradeBalance must reject a nil request")
	_, err = ex.GetTradeBalance(ctx, &GetTradeBalanceRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetTradeBalance must reject an invalid rebase multiplier")
	balance, err := ex.GetTradeBalance(ctx, &GetTradeBalanceRequest{Asset: "USD", RebaseMultiplier: "base"})
	require.NoError(t, err, "GetTradeBalance must not error")
	require.NotNil(t, balance, "GetTradeBalance must return a response")
	assert.Equal(t, 21.1063, balance.CostBasis.Float64(), "GetTradeBalance should decode cost basis")
	assert.Equal(t, 31.1297, balance.CurrentValuation.Float64(), "GetTradeBalance should decode current valuation")
	assert.Equal(t, 374.0, balance.FreeMarginOrders.Float64(), "GetTradeBalance should decode free margin for orders")
	assert.Equal(t, 2.0, balance.UnexecutedValue.Float64(), "GetTradeBalance should decode unexecuted value")
	responseJSON, err := json.Marshal(balance)
	require.NoError(t, err, "GetTradeBalance must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"eb":"1101.3425"`, "GetTradeBalance should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/TradeBalance")
	assert.Equal(t, "USD", values.Get("asset"), "GetTradeBalance should encode asset")
	assert.Equal(t, "base", values.Get("rebase_multiplier"), "GetTradeBalance should encode rebase multiplier")
	_, err = ex.GetTradeBalance(ctx, &GetTradeBalanceRequest{})
	require.NoError(t, err, "GetTradeBalance must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/TradeBalance")
	for _, value := range []RebaseMultiplier{RebaseMultiplierRebased, RebaseMultiplierBase} {
		t.Run("rebase multiplier "+string(value), func(t *testing.T) {
			_, err := ex.GetTradeBalance(t.Context(), &GetTradeBalanceRequest{RebaseMultiplier: value})
			require.NoError(t, err, "GetTradeBalance must accept the documented rebase multiplier")
			values := requireSpotRequest(t, requests, "/0/private/TradeBalance")
			assert.Equal(t, string(value), values.Get("rebase_multiplier"), "GetTradeBalance should encode the documented rebase multiplier")
		})
	}

	balance, err = newSpotNullResultExchange(t).GetTradeBalance(ctx, &GetTradeBalanceRequest{})
	require.NoError(t, err, "GetTradeBalance must accept a null result")
	assert.Nil(t, balance, "GetTradeBalance should return nil for a null result")
	balance, err = newSpotErrorExchange(t).GetTradeBalance(ctx, &GetTradeBalanceRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetTradeBalance must surface request errors")
	assert.Nil(t, balance, "GetTradeBalance result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetTradeBalance(t.Context(), new(GetTradeBalanceRequest))
		require.NoError(t, err, "GetTradeBalance must not error against the live API")
		require.NotNil(t, response, "GetTradeBalance must return a response from the live API")
	})
}

func TestGetOpenOrders(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()
	zeroUserReference := int32(0)

	_, err := ex.GetOpenOrders(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetOpenOrders must reject a nil request")
	_, err = ex.GetOpenOrders(ctx, &GetOpenOrdersRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetOpenOrders must reject an invalid rebase multiplier")
	openOrders, err := ex.GetOpenOrders(ctx, &GetOpenOrdersRequest{Trades: true, UserReference: &zeroUserReference, ClientOrderID: "CLIENT", RebaseMultiplier: "rebased"})
	require.NoError(t, err, "GetOpenOrders must not error")
	require.NotNil(t, openOrders, "GetOpenOrders must return a response")
	assert.Equal(t, "CLIENT", openOrders.Open["ORDER"].ClientOrderID, "GetOpenOrders should decode client order ID")
	assert.Equal(t, "currency", openOrders.Open["ORDER"].Description.AssetClass, "GetOpenOrders should decode description asset class")
	assert.Equal(t, "gtc", openOrders.Open["ORDER"].TimeInForce, "GetOpenOrders should decode time-in-force")
	assert.Equal(t, "last", openOrders.Open["ORDER"].Trigger, "GetOpenOrders should decode trigger")
	assert.Equal(t, "SUB", openOrders.Open["ORDER"].SenderSubID, "GetOpenOrders should decode sender subaccount")
	responseJSON, err := json.Marshal(openOrders)
	require.NoError(t, err, "GetOpenOrders must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"open":{"ORDER"`, "GetOpenOrders should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/OpenOrders")
	assert.Equal(t, "true", values.Get("trades"), "GetOpenOrders should encode trades")
	assert.Equal(t, "0", values.Get("userref"), "GetOpenOrders should encode an explicit zero user reference")
	assert.Equal(t, "CLIENT", values.Get("cl_ord_id"), "GetOpenOrders should encode client order ID")
	assert.Equal(t, "rebased", values.Get("rebase_multiplier"), "GetOpenOrders should encode rebase multiplier")
	_, err = ex.GetOpenOrders(ctx, &GetOpenOrdersRequest{})
	require.NoError(t, err, "GetOpenOrders must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/OpenOrders")

	openOrders, err = newSpotNullResultExchange(t).GetOpenOrders(ctx, &GetOpenOrdersRequest{})
	require.NoError(t, err, "GetOpenOrders must accept a null result")
	assert.Nil(t, openOrders, "GetOpenOrders should return nil for a null result")
	openOrders, err = newSpotErrorExchange(t).GetOpenOrders(ctx, &GetOpenOrdersRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetOpenOrders must surface request errors")
	assert.Nil(t, openOrders, "GetOpenOrders result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetOpenOrders(t.Context(), new(GetOpenOrdersRequest))
		require.NoError(t, err, "GetOpenOrders must not error against the live API")
		require.NotNil(t, response, "GetOpenOrders must return a response from the live API")
		require.NotNil(t, response.Open, "GetOpenOrders live response must include an open-order map")
	})
}

func TestGetClosedOrders(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()
	zeroUserReference := int32(0)
	falseValue := false
	startTime := time.Unix(2, 0)
	endTime := time.Unix(1, 0)

	_, err := ex.GetClosedOrders(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetClosedOrders must reject a nil request")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{CloseTime: invalidSpotValue})
	require.ErrorIs(t, err, errCloseTimeInvalid, "GetClosedOrders must reject an invalid close-time selector")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetClosedOrders must reject an invalid rebase multiplier")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{Start: TimeOrTransactionID{Time: startTime, TransactionID: "START"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetClosedOrders must reject conflicting start boundaries")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{End: TimeOrTransactionID{Time: endTime, TransactionID: "END"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetClosedOrders must reject conflicting end boundaries")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{Start: TimeOrTransactionID{Time: startTime}, End: TimeOrTransactionID{Time: endTime}})
	require.ErrorIs(t, err, errTimeRangeInvalid, "GetClosedOrders must reject a reversed time range")
	closedOrders, err := ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{
		Trades:           true,
		UserReference:    &zeroUserReference,
		ClientOrderID:    "CLIENT",
		Start:            TimeOrTransactionID{TransactionID: "START"},
		End:              TimeOrTransactionID{TransactionID: "END"},
		Offset:           1,
		CloseTime:        CloseTimeBoth,
		ConsolidateTaker: &falseValue,
		WithoutCount:     true,
		RebaseMultiplier: RebaseMultiplierBase,
	})
	require.NoError(t, err, "GetClosedOrders must not error")
	require.NotNil(t, closedOrders, "GetClosedOrders must return a response")
	assert.Equal(t, int64(1), closedOrders.Count, "GetClosedOrders should decode count")
	assert.True(t, closedOrders.Closed["ORDER"].Margin, "GetClosedOrders should decode margin status")
	assert.Equal(t, "User requested", closedOrders.Closed["ORDER"].Reason, "GetClosedOrders should decode status reason")
	responseJSON, err := json.Marshal(closedOrders)
	require.NoError(t, err, "GetClosedOrders must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"closed":{"ORDER"`, "GetClosedOrders should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/ClosedOrders")
	assert.Equal(t, "0", values.Get("userref"), "GetClosedOrders should encode an explicit zero user reference")
	assert.Equal(t, "START", values.Get("start"), "GetClosedOrders should encode start")
	assert.Equal(t, "END", values.Get("end"), "GetClosedOrders should encode end")
	assert.Equal(t, "1", values.Get("ofs"), "GetClosedOrders should encode offset")
	assert.Equal(t, "both", values.Get("closetime"), "GetClosedOrders should encode close-time selector")
	assert.Equal(t, "false", values.Get("consolidate_taker"), "GetClosedOrders should encode explicit false consolidation")
	assert.Equal(t, "true", values.Get("without_count"), "GetClosedOrders should encode count suppression")
	_, err = ex.GetClosedOrders(ctx, &GetClosedOrdersRequest{})
	require.NoError(t, err, "GetClosedOrders must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/ClosedOrders")
	for _, value := range []CloseTime{CloseTimeOpen, CloseTimeClose, CloseTimeBoth} {
		t.Run("close time "+string(value), func(t *testing.T) {
			_, err := ex.GetClosedOrders(t.Context(), &GetClosedOrdersRequest{CloseTime: value})
			require.NoError(t, err, "GetClosedOrders must accept the documented close-time selector")
			values := requireSpotRequest(t, requests, "/0/private/ClosedOrders")
			assert.Equal(t, string(value), values.Get("closetime"), "GetClosedOrders should encode the documented close-time selector")
		})
	}

	closedOrders, err = newSpotNullResultExchange(t).GetClosedOrders(ctx, &GetClosedOrdersRequest{})
	require.NoError(t, err, "GetClosedOrders must accept a null result")
	assert.Nil(t, closedOrders, "GetClosedOrders should return nil for a null result")
	closedOrders, err = newSpotErrorExchange(t).GetClosedOrders(ctx, &GetClosedOrdersRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetClosedOrders must surface request errors")
	assert.Nil(t, closedOrders, "GetClosedOrders result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetClosedOrders(t.Context(), new(GetClosedOrdersRequest))
		require.NoError(t, err, "GetClosedOrders must not error against the live API")
		require.NotNil(t, response, "GetClosedOrders must return a response from the live API")
		require.NotNil(t, response.Closed, "GetClosedOrders live response must include a closed-order map")
	})
}

func TestQueryOrdersInfo(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()
	zeroUserReference := int32(0)
	falseValue := false

	_, err := ex.QueryOrdersInfo(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "QueryOrdersInfo must reject a nil request")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{})
	require.ErrorIs(t, err, errOrderIdentifierCount, "QueryOrdersInfo must require an order identifier")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: make([]string, 51)})
	require.ErrorIs(t, err, errOrderIdentifierCount, "QueryOrdersInfo must reject more than fifty identifiers")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{""}})
	require.ErrorIs(t, err, errOrderIDRequired, "QueryOrdersInfo must reject an empty order identifier")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{"ORDER"}, RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "QueryOrdersInfo must reject an invalid rebase multiplier")
	orders, err := ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{"ORDER", "ORDER2"}, Trades: true, UserReference: &zeroUserReference, ConsolidateTaker: &falseValue, RebaseMultiplier: "base"})
	require.NoError(t, err, "QueryOrdersInfo must not error")
	assert.Equal(t, "ioc", orders["ORDER"].TimeInForce, "QueryOrdersInfo should decode time-in-force")
	values := requireSpotRequest(t, requests, "/0/private/QueryOrders")
	assert.Equal(t, "ORDER,ORDER2", values.Get("txid"), "QueryOrdersInfo should encode order identifiers")
	assert.Equal(t, "false", values.Get("consolidate_taker"), "QueryOrdersInfo should encode explicit false consolidation")
	_, err = ex.QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{"ORDER"}})
	require.NoError(t, err, "QueryOrdersInfo must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/QueryOrders")

	orders, err = newSpotErrorExchange(t).QueryOrdersInfo(ctx, &QueryOrdersInfoRequest{TransactionIDs: []string{"ORDER"}})
	require.ErrorIs(t, err, errSpotTransport, "QueryOrdersInfo must surface request errors")
	assert.Nil(t, orders, "QueryOrdersInfo result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		orderID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_ORDER_ID")
		response, err := spotLiveExchange.QueryOrdersInfo(t.Context(), &QueryOrdersInfoRequest{TransactionIDs: []string{orderID}})
		require.NoError(t, err, "QueryOrdersInfo must not error against the live API")
		require.NotNil(t, response, "QueryOrdersInfo must return a response from the live API")
		require.Contains(t, response, orderID, "QueryOrdersInfo live response must include the requested order")
	})
}

func TestGetTradesHistory(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()
	falseValue := false
	tradeLimit := uint64(100)
	startTime := time.Unix(2, 0)
	endTime := time.Unix(1, 0)

	_, err := ex.GetTradesHistory(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetTradesHistory must reject a nil request")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Type: invalidSpotValue})
	require.ErrorIs(t, err, errTradeTypeInvalid, "GetTradesHistory must reject an invalid trade type")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetTradesHistory must reject an invalid rebase multiplier")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{AssetClass: AssetClassCurrency})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetTradesHistory must reject an invalid asset class")
	invalidTradeLimit := uint64(0)
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Limit: &invalidTradeLimit})
	require.ErrorIs(t, err, errTradeLimitInvalid, "GetTradesHistory must reject an invalid result limit")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Start: TimeOrTransactionID{Time: startTime, TransactionID: "START"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetTradesHistory must reject conflicting start boundaries")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{End: TimeOrTransactionID{Time: endTime, TransactionID: "END"}})
	require.ErrorIs(t, err, errTimeOrIDConflict, "GetTradesHistory must reject conflicting end boundaries")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Start: TimeOrTransactionID{Time: startTime}, End: TimeOrTransactionID{Time: endTime}})
	require.ErrorIs(t, err, errTimeRangeInvalid, "GetTradesHistory must reject a reversed time range")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{Pair: currency.Pair{Base: currency.BTC}})
	require.ErrorIs(t, err, errPairRequired, "GetTradesHistory must reject a partially populated pair")
	_, err = new(Exchange).GetTradesHistory(ctx, &GetTradesHistoryRequest{Pair: spotTestPair})
	require.Error(t, err, "GetTradesHistory must surface pair-format errors")
	history, err := ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{
		Type:             TradeHistoryClosedPosition,
		Trades:           true,
		Start:            TimeOrTransactionID{TransactionID: "START"},
		End:              TimeOrTransactionID{TransactionID: "END"},
		Offset:           1,
		WithoutCount:     true,
		ConsolidateTaker: &falseValue,
		Ledgers:          true,
		RebaseMultiplier: RebaseMultiplierRebased,
		AssetClass:       AssetClassForex,
		Pair:             spotTestPair,
		Limit:            &tradeLimit,
	})
	require.NoError(t, err, "GetTradesHistory must not error")
	require.NotNil(t, history, "GetTradesHistory must return a response")
	assert.Equal(t, int64(1), history.Count, "GetTradesHistory should decode count")
	trade := history.Trades["TRADE"]
	assert.Equal(t, "POSITION", trade.PosTxID, "GetTradesHistory should decode position identifier")
	assert.Equal(t, "2", trade.Leverage, "GetTradesHistory should decode leverage")
	assert.Equal(t, 101.5, trade.ClosedPositionAveragePrice.Float64(), "GetTradesHistory should decode numeric closed-position price")
	assert.Equal(t, 100.5, trade.ClosedPositionCost.Float64(), "GetTradesHistory should decode closed-position cost")
	assert.Equal(t, 1.5, trade.Net.Float64(), "GetTradesHistory should decode net result")
	assert.Equal(t, []string{"LEDGER"}, trade.Ledgers, "GetTradesHistory should decode ledger identifiers")
	assert.Equal(t, uint64(42), trade.TradeID, "GetTradesHistory should decode trade identifier")
	assert.True(t, trade.Maker, "GetTradesHistory should decode maker status")
	assert.Equal(t, "currency", trade.AssetClass, "GetTradesHistory should decode asset class")
	assert.Equal(t, "market", trade.TradeOrderType, "GetTradesHistory should decode execution order type")
	responseJSON, err := json.Marshal(history)
	require.NoError(t, err, "GetTradesHistory must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"trades":{"TRADE"`, "GetTradesHistory should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/TradesHistory")
	assert.Equal(t, "closed position", values.Get("type"), "GetTradesHistory should encode trade type")
	assert.Equal(t, "1", values.Get("ofs"), "GetTradesHistory should encode offset")
	assert.Equal(t, "true", values.Get("without_count"), "GetTradesHistory should encode count suppression")
	assert.Equal(t, "false", values.Get("consolidate_taker"), "GetTradesHistory should encode explicit false consolidation")
	assert.Equal(t, "true", values.Get("ledgers"), "GetTradesHistory should encode ledger inclusion")
	assert.Equal(t, "forex", values.Get("aclass"), "GetTradesHistory should encode asset class")
	assert.Equal(t, "XBTUSD", values.Get("pair"), "GetTradesHistory should encode the formatted pair")
	assert.Equal(t, "100", values.Get("limit"), "GetTradesHistory should encode the result limit")
	_, err = ex.GetTradesHistory(ctx, &GetTradesHistoryRequest{})
	require.NoError(t, err, "GetTradesHistory must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/TradesHistory")
	for _, value := range []TradeHistoryType{TradeHistoryAll, TradeHistoryAnyPosition, TradeHistoryClosedPosition, TradeHistoryClosingPosition, TradeHistoryNoPosition} {
		t.Run("trade type "+string(value), func(t *testing.T) {
			_, err := ex.GetTradesHistory(t.Context(), &GetTradesHistoryRequest{Type: value})
			require.NoError(t, err, "GetTradesHistory must accept the documented trade type")
			values := requireSpotRequest(t, requests, "/0/private/TradesHistory")
			assert.Equal(t, string(value), values.Get("type"), "GetTradesHistory should encode the documented trade type")
		})
	}

	history, err = newSpotNullResultExchange(t).GetTradesHistory(ctx, &GetTradesHistoryRequest{})
	require.NoError(t, err, "GetTradesHistory must accept a null result")
	assert.Nil(t, history, "GetTradesHistory should return nil for a null result")
	history, err = newSpotErrorExchange(t).GetTradesHistory(ctx, &GetTradesHistoryRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetTradesHistory must surface request errors")
	assert.Nil(t, history, "GetTradesHistory result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetTradesHistory(t.Context(), new(GetTradesHistoryRequest))
		require.NoError(t, err, "GetTradesHistory must not error against the live API")
		require.NotNil(t, response, "GetTradesHistory must return a response from the live API")
		require.NotNil(t, response.Trades, "GetTradesHistory live response must include a trade map")
	})
}

func TestQueryTrades(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.QueryTrades(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "QueryTrades must reject a nil request")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{})
	require.ErrorIs(t, err, errTradeIdentifierCount, "QueryTrades must require a trade identifier")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: make([]string, 21)})
	require.ErrorIs(t, err, errTradeIdentifierCount, "QueryTrades must reject more than twenty identifiers")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{""}})
	require.ErrorIs(t, err, errTransactionIDRequired, "QueryTrades must reject an empty trade identifier")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{"TRADE"}, RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "QueryTrades must reject an invalid rebase multiplier")
	queriedTrades, err := ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{"TRADE", "TRADE2"}, Trades: true, RebaseMultiplier: "base"})
	require.NoError(t, err, "QueryTrades must not error")
	assert.Equal(t, "ORDER", queriedTrades["TRADE"].OrderTxID, "QueryTrades should decode order identifier")
	assert.Equal(t, 101.5, queriedTrades["TRADE"].ClosedPositionAveragePrice.Float64(), "QueryTrades should decode quoted closed-position values")
	values := requireSpotRequest(t, requests, "/0/private/QueryTrades")
	assert.Equal(t, "TRADE,TRADE2", values.Get("txid"), "QueryTrades should encode trade identifiers")
	assert.Equal(t, "true", values.Get("trades"), "QueryTrades should encode related trades")
	_, err = ex.QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{"TRADE"}})
	require.NoError(t, err, "QueryTrades must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/QueryTrades")

	queriedTrades, err = newSpotErrorExchange(t).QueryTrades(ctx, &QueryTradesRequest{TransactionIDs: []string{"TRADE"}})
	require.ErrorIs(t, err, errSpotTransport, "QueryTrades must surface request errors")
	assert.Nil(t, queriedTrades, "QueryTrades result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		tradeID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_TRADE_ID")
		response, err := spotLiveExchange.QueryTrades(t.Context(), &QueryTradesRequest{TransactionIDs: []string{tradeID}})
		require.NoError(t, err, "QueryTrades must not error against the live API")
		require.NotNil(t, response, "QueryTrades must return a response from the live API")
		require.Contains(t, response, tradeID, "QueryTrades live response must include the requested trade")
	})
}

func TestOpenPositions(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.OpenPositions(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "OpenPositions must reject a nil request")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{Consolidation: invalidSpotValue})
	require.ErrorIs(t, err, errConsolidationInvalid, "OpenPositions must reject an invalid consolidation")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "OpenPositions must reject an invalid rebase multiplier")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{TransactionIDs: []string{""}})
	require.ErrorIs(t, err, errTransactionIDRequired, "OpenPositions must reject an empty transaction identifier")
	positions, err := ex.OpenPositions(ctx, &OpenPositionsRequest{TransactionIDs: []string{"POSITION", "POSITION2"}, DoCalculations: true, Consolidation: "market", RebaseMultiplier: "base"})
	require.NoError(t, err, "OpenPositions must not error")
	assert.Equal(t, "ORDER", positions["POSITION"].Ordertxid, "OpenPositions should decode order identifier")
	assert.Equal(t, "currency", positions["POSITION"].AssetClass, "OpenPositions should decode asset class")
	assert.Equal(t, 120.0, positions["POSITION"].Value.Float64(), "OpenPositions should decode position value")
	values := requireSpotRequest(t, requests, "/0/private/OpenPositions")
	assert.Equal(t, "POSITION,POSITION2", values.Get("txid"), "OpenPositions should encode position identifiers")
	assert.Equal(t, "true", values.Get("docalcs"), "OpenPositions should encode calculations")
	assert.Equal(t, "market", values.Get("consolidation"), "OpenPositions should encode consolidation")
	_, err = ex.OpenPositions(ctx, &OpenPositionsRequest{})
	require.NoError(t, err, "OpenPositions must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/OpenPositions")

	positions, err = newSpotErrorExchange(t).OpenPositions(ctx, &OpenPositionsRequest{})
	require.ErrorIs(t, err, errSpotTransport, "OpenPositions must surface request errors")
	assert.Nil(t, positions, "OpenPositions result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.OpenPositions(t.Context(), new(OpenPositionsRequest))
		require.NoError(t, err, "OpenPositions must not error against the live API")
		require.NotNil(t, response, "OpenPositions must return a response from the live API")
	})
}

func TestQueryLedgers(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()

	_, err := ex.QueryLedgers(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "QueryLedgers must reject a nil request")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{})
	require.ErrorIs(t, err, errLedgerIdentifierCount, "QueryLedgers must require a ledger identifier")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: make([]string, 21)})
	require.ErrorIs(t, err, errLedgerIdentifierCount, "QueryLedgers must reject more than twenty identifiers")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{""}})
	require.ErrorIs(t, err, errIDRequired, "QueryLedgers must reject an empty ledger identifier")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{"LEDGER"}, RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "QueryLedgers must reject an invalid rebase multiplier")
	queriedLedgers, err := ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{"LEDGER", "LEDGER2"}, Trades: true, RebaseMultiplier: "base"})
	require.NoError(t, err, "QueryLedgers must not error")
	assert.Equal(t, "REFERENCE", queriedLedgers["LEDGER"].Refid, "QueryLedgers should decode reference identifier")
	assert.Equal(t, "spotfromfutures", queriedLedgers["LEDGER"].Subtype, "QueryLedgers should decode ledger subtype")
	values := requireSpotRequest(t, requests, "/0/private/QueryLedgers")
	assert.Equal(t, "LEDGER,LEDGER2", values.Get("id"), "QueryLedgers should encode ledger identifiers")
	assert.Equal(t, "true", values.Get("trades"), "QueryLedgers should encode related trades")
	_, err = ex.QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{"LEDGER"}})
	require.NoError(t, err, "QueryLedgers must allow optional parameters to be omitted")
	requireSpotRequest(t, requests, "/0/private/QueryLedgers")

	queriedLedgers, err = newSpotErrorExchange(t).QueryLedgers(ctx, &QueryLedgersRequest{IDs: []string{"LEDGER"}})
	require.ErrorIs(t, err, errSpotTransport, "QueryLedgers must surface request errors")
	assert.Nil(t, queriedLedgers, "QueryLedgers result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		ledgerID := spotLiveTestValue(t, "GCT_KRAKEN_SPOT_LIVE_LEDGER_ID")
		response, err := spotLiveExchange.QueryLedgers(t.Context(), &QueryLedgersRequest{IDs: []string{ledgerID}})
		require.NoError(t, err, "QueryLedgers must not error against the live API")
		require.NotNil(t, response, "QueryLedgers must return a response from the live API")
		require.Contains(t, response, ledgerID, "QueryLedgers live response must include the requested ledger entry")
	})
}

func TestGetTradeVolume(t *testing.T) {
	ex, requests := newSpotEndpointExchange(t, spotAccountFixtures)
	ctx := t.Context()
	trueValue := true

	_, err := ex.GetTradeVolume(ctx, nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetTradeVolume must reject a nil request")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{RebaseMultiplier: invalidSpotValue})
	require.ErrorIs(t, err, errRebaseMultiplierInvalid, "GetTradeVolume must reject an invalid rebase multiplier")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{AssetClass: AssetClassForex}}})
	require.ErrorIs(t, err, errAssetRequired, "GetTradeVolume must require an asset for class-qualified pairs")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{Asset: "XBTUSD"}}})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetTradeVolume must require an asset class for class-qualified pairs")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{Asset: "XBTUSD", AssetClass: invalidSpotValue}}})
	require.ErrorIs(t, err, errAssetClassInvalid, "GetTradeVolume must reject an invalid class-qualified pair")
	feeInfo := false
	volume, err := ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{Asset: "XBTUSD", AssetClass: AssetClassForex}}, FeeInfo: &feeInfo, FeeSchedule: &trueValue, RebaseMultiplier: RebaseMultiplierRebased})
	require.NoError(t, err, "GetTradeVolume must not error")
	require.NotNil(t, volume, "GetTradeVolume must return a response")
	assert.Equal(t, "currency", volume.AssetClass, "GetTradeVolume should decode asset class")
	assert.Equal(t, 200.0, volume.Inputs.SpotVolume30D.Float64(), "GetTradeVolume should decode spot volume input")
	assert.Equal(t, "SUB", volume.VolumeSubaccounts[0].IIBAN, "GetTradeVolume should decode subaccount volume")
	assert.Equal(t, "BTC/USD", volume.Schedules[0].Pair, "GetTradeVolume should decode fee schedule")
	assert.True(t, *volume.Schedules[0].Tiers[0].Active, "GetTradeVolume should decode active fee tier")
	assert.Nil(t, volume.Fees["BTC/USD"].NextFee, "GetTradeVolume should preserve a null next fee")
	assert.Nil(t, volume.Fees["BTC/USD"].NextVolume, "GetTradeVolume should preserve a null next volume")
	assert.Equal(t, 5.0, volume.Fees["BTC/USD"].VolumeOffset.Float64(), "GetTradeVolume should decode a volume offset")
	require.NotNil(t, volume.FeesMaker["BTC/USD"].NextFee, "GetTradeVolume must decode a non-null next fee")
	assert.Equal(t, 0.08, volume.FeesMaker["BTC/USD"].NextFee.Float64(), "GetTradeVolume should decode the next fee")
	assert.Equal(t, 50.0, volume.FeesMaker["BTC/USD"].TierFuturesVolume.Float64(), "GetTradeVolume should decode the futures-volume tier")
	assert.Equal(t, 100.0, volume.FeesMaker["BTC/USD"].NextFuturesVolume.Float64(), "GetTradeVolume should decode the next futures-volume tier")
	responseJSON, err := json.Marshal(volume)
	require.NoError(t, err, "GetTradeVolume must encode the decoded response")
	assert.Contains(t, string(responseJSON), `"currency":"USD"`, "GetTradeVolume should decode the response")
	values := requireSpotRequest(t, requests, "/0/private/TradeVolume")
	assert.JSONEq(t, `[{"asset":"XBTUSD","aclass":"forex"}]`, values.Get("pair"), "GetTradeVolume should encode class-qualified pairs")
	assert.Equal(t, "false", values.Get("fee-info"), "GetTradeVolume should encode explicit false fee info")
	assert.Equal(t, "true", values.Get("fee_schedule"), "GetTradeVolume should encode fee schedule selection")
	_, err = ex.GetTradeVolume(ctx, &GetTradeVolumeRequest{})
	require.NoError(t, err, "GetTradeVolume must allow optional parameters to be omitted")
	values = requireSpotRequest(t, requests, "/0/private/TradeVolume")
	values.Del("nonce")
	assert.Empty(t, values, "GetTradeVolume should omit unset parameters")
	for _, value := range []AssetClass{AssetClassCurrency, AssetClassForex, AssetClassEquity, AssetClassEquityPair, AssetClassNFT, AssetClassDerivatives, AssetClassTokenizedAsset, AssetClassFuturesContract} {
		t.Run(string(value), func(t *testing.T) {
			_, err := ex.GetTradeVolume(t.Context(), &GetTradeVolumeRequest{Pairs: []TradeVolumePairRequest{{Asset: "XBTUSD", AssetClass: value}}})
			require.NoError(t, err, "GetTradeVolume must accept the documented asset class")
			values := requireSpotRequest(t, requests, "/0/private/TradeVolume")
			assert.JSONEq(t, `[{"asset":"XBTUSD","aclass":"`+string(value)+`"}]`, values.Get("pair"), "GetTradeVolume should encode the documented asset class")
		})
	}

	volume, err = newSpotNullResultExchange(t).GetTradeVolume(ctx, &GetTradeVolumeRequest{})
	require.NoError(t, err, "GetTradeVolume must accept a null result")
	assert.Nil(t, volume, "GetTradeVolume should return nil for a null result")
	volume, err = newSpotErrorExchange(t).GetTradeVolume(ctx, &GetTradeVolumeRequest{})
	require.ErrorIs(t, err, errSpotTransport, "GetTradeVolume must surface request errors")
	assert.Nil(t, volume, "GetTradeVolume result should remain nil on request errors")

	t.Run("live", func(t *testing.T) {
		skipSpotLiveTest(t, spotLivePrivate)
		response, err := spotLiveExchange.GetTradeVolume(t.Context(), new(GetTradeVolumeRequest))
		require.NoError(t, err, "GetTradeVolume must not error against the live API")
		require.NotNil(t, response, "GetTradeVolume must return a response from the live API")
		require.NotEmpty(t, response.Currency, "GetTradeVolume live response must include a currency")
		require.NotEmpty(t, response.AssetClass, "GetTradeVolume live response must include an asset class")
	})
}

func TestOrderInfoUnmarshalJSON(t *testing.T) {
	var order OrderInfo
	require.NoError(t, json.Unmarshal([]byte(`{"cl_ord_id":"CLIENT","descr":{"aclass":"tokenized_asset"},"time_in_force":"fok","trigger":"index","margin":true,"sender_sub_id":"SUB"}`), &order), "OrderInfo must decode current fields")
	assert.Equal(t, "CLIENT", order.ClientOrderID, "OrderInfo should decode client order ID")
	assert.Equal(t, "tokenized_asset", order.Description.AssetClass, "OrderInfo should decode description asset class")
	assert.Equal(t, "fok", order.TimeInForce, "OrderInfo should decode time-in-force")
	assert.Equal(t, "index", order.Trigger, "OrderInfo should decode trigger")
	assert.True(t, order.Margin, "OrderInfo should decode margin")
	assert.Equal(t, "SUB", order.SenderSubID, "OrderInfo should decode sender subaccount")
}

func TestTradeVolumeResponseUnmarshalJSON(t *testing.T) {
	var volume TradeVolumeResponse
	require.NoError(t, json.Unmarshal([]byte(`{"asset_class":"currency","inputs":{"domain_spot_volume_30d":"1","domain_futures_volume_30d":"2","domain_assets_on_platform":"3"},"volume_subaccounts":[{"iiban":"SUB","volume":"4"}],"schedules":[{"pair":"BTC/USD","class":"forex","tiers":[{"maker_fee":"0.1","taker_fee":"0.2","min_spot_volume":null,"min_futures_volume":"5","min_assets_on_platform":null,"active":false}]}]}`), &volume), "TradeVolumeResponse must decode current fields")
	assert.Equal(t, 2.0, volume.Inputs.FuturesVolume30D.Float64(), "TradeVolumeResponse should decode futures volume input")
	assert.Nil(t, volume.Schedules[0].Tiers[0].MinimumSpotVolume, "TradeVolumeResponse should decode a null spot threshold")
	assert.Equal(t, 5.0, volume.Schedules[0].Tiers[0].MinimumFuturesVolume.Float64(), "TradeVolumeResponse should decode futures threshold")
	assert.False(t, *volume.Schedules[0].Tiers[0].Active, "TradeVolumeResponse should decode an explicit false active flag")
}
