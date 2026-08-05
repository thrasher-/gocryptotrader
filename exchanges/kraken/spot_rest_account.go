package kraken

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// GetAccountBalance returns account balances by asset.
func (e *Exchange) GetAccountBalance(ctx context.Context, req *GetAccountBalanceRequest) (map[string]types.Number, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	params := url.Values{}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result map[string]types.Number
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Balance", params, &result)
	return result, err
}

// GetExtendedBalance returns account balances and held amounts by asset with optional rebase control.
func (e *Exchange) GetExtendedBalance(ctx context.Context, req *GetExtendedBalanceRequest) (map[string]ExtendedBalanceResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	params := url.Values{}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result map[string]ExtendedBalanceResponse
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "BalanceEx", params, &result)
	return result, err
}

// GetCreditLines returns account credit-line data.
func (e *Exchange) GetCreditLines(ctx context.Context, req *GetCreditLinesRequest) (*GetCreditLinesResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	params := url.Values{}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result GetCreditLinesResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CreditLines", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTradeBalance returns current collateral and margin balances with rebase control.
func (e *Exchange) GetTradeBalance(ctx context.Context, req *GetTradeBalanceRequest) (*TradeBalanceInfo, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}

	params := url.Values{}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result TradeBalanceInfo
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "TradeBalance", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOpenOrders returns open orders with all current filters.
func (e *Exchange) GetOpenOrders(ctx context.Context, req *GetOpenOrdersRequest) (*OpenOrders, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}

	params := url.Values{}
	if req.Trades {
		params.Set("trades", strconv.FormatBool(req.Trades))
	}
	if req.UserReference != nil {
		params.Set("userref", strconv.FormatInt(int64(*req.UserReference), 10))
	}
	if req.ClientOrderID != "" {
		params.Set("cl_ord_id", req.ClientOrderID)
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result OpenOrders
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "OpenOrders", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetClosedOrders returns closed orders with all current filters.
func (e *Exchange) GetClosedOrders(ctx context.Context, req *GetClosedOrdersRequest) (*ClosedOrders, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.CloseTime, "open", "close", "both") {
		return nil, errCloseTimeInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	start, err := formatTimeOrTransactionID(req.Start)
	if err != nil {
		return nil, err
	}
	end, err := formatTimeOrTransactionID(req.End)
	if err != nil {
		return nil, err
	}
	if !req.Start.Time.IsZero() && !req.End.Time.IsZero() && req.End.Time.Before(req.Start.Time) {
		return nil, errTimeRangeInvalid
	}

	params := url.Values{}
	if req.Trades {
		params.Set("trades", strconv.FormatBool(req.Trades))
	}
	if req.UserReference != nil {
		params.Set("userref", strconv.FormatInt(int64(*req.UserReference), 10))
	}
	if req.ClientOrderID != "" {
		params.Set("cl_ord_id", req.ClientOrderID)
	}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if req.Offset != 0 {
		params.Set("ofs", strconv.FormatUint(req.Offset, 10))
	}
	if req.CloseTime != "" {
		params.Set("closetime", string(req.CloseTime))
	}
	if req.ConsolidateTaker != nil {
		params.Set("consolidate_taker", strconv.FormatBool(*req.ConsolidateTaker))
	}
	if req.WithoutCount {
		params.Set("without_count", strconv.FormatBool(req.WithoutCount))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result ClosedOrders
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "ClosedOrders", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// QueryOrdersInfo returns information for up to fifty orders with current options.
func (e *Exchange) QueryOrdersInfo(ctx context.Context, req *QueryOrdersInfoRequest) (map[string]OrderInfo, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if len(req.TransactionIDs) == 0 || len(req.TransactionIDs) > 50 {
		return nil, errOrderIdentifierCount
	}
	if slices.Contains(req.TransactionIDs, "") {
		return nil, errOrderIDRequired
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}

	params := url.Values{"txid": {strings.Join(req.TransactionIDs, ",")}}
	if req.Trades {
		params.Set("trades", strconv.FormatBool(req.Trades))
	}
	if req.UserReference != nil {
		params.Set("userref", strconv.FormatInt(int64(*req.UserReference), 10))
	}
	if req.ConsolidateTaker != nil {
		params.Set("consolidate_taker", strconv.FormatBool(*req.ConsolidateTaker))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result map[string]OrderInfo
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "QueryOrders", params, &result)
	return result, err
}

// GetTradesHistory returns trade history with all current filters.
func (e *Exchange) GetTradesHistory(ctx context.Context, req *GetTradesHistoryRequest) (*TradesHistory, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.Type, "all", "any position", "closed position", "closing position", "no position") {
		return nil, errTradeTypeInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	if !isValidSpotEnum(req.AssetClass, "forex", "equity_pair", "futures_contract", "synthetic_pair", "external_pair") {
		return nil, errAssetClassInvalid
	}
	if req.Limit != nil && (*req.Limit == 0 || *req.Limit > 100) {
		return nil, errTradeLimitInvalid
	}
	start, err := formatTimeOrTransactionID(req.Start)
	if err != nil {
		return nil, err
	}
	end, err := formatTimeOrTransactionID(req.End)
	if err != nil {
		return nil, err
	}
	if !req.Start.Time.IsZero() && !req.End.Time.IsZero() && req.End.Time.Before(req.Start.Time) {
		return nil, errTimeRangeInvalid
	}

	params := url.Values{}
	if req.Type != "" {
		params.Set("type", string(req.Type))
	}
	if req.Trades {
		params.Set("trades", strconv.FormatBool(req.Trades))
	}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if req.Offset != 0 {
		params.Set("ofs", strconv.FormatUint(req.Offset, 10))
	}
	if req.WithoutCount {
		params.Set("without_count", strconv.FormatBool(req.WithoutCount))
	}
	if req.ConsolidateTaker != nil {
		params.Set("consolidate_taker", strconv.FormatBool(*req.ConsolidateTaker))
	}
	if req.Ledgers {
		params.Set("ledgers", strconv.FormatBool(req.Ledgers))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if !req.Pair.IsEmpty() {
		if !req.Pair.IsPopulated() {
			return nil, errPairRequired
		}
		pair, err := e.FormatSymbol(req.Pair, asset.Spot)
		if err != nil {
			return nil, err
		}
		params.Set("pair", pair)
	}
	if req.Limit != nil {
		params.Set("limit", strconv.FormatUint(*req.Limit, 10))
	}

	var result TradesHistory
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "TradesHistory", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// QueryTrades returns information for up to twenty trades with current options.
func (e *Exchange) QueryTrades(ctx context.Context, req *QueryTradesRequest) (map[string]TradeInfo, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if len(req.TransactionIDs) == 0 || len(req.TransactionIDs) > 20 {
		return nil, errTradeIdentifierCount
	}
	if slices.Contains(req.TransactionIDs, "") {
		return nil, errTransactionIDRequired
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}

	params := url.Values{"txid": {strings.Join(req.TransactionIDs, ",")}}
	if req.Trades {
		params.Set("trades", strconv.FormatBool(req.Trades))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result map[string]TradeInfo
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "QueryTrades", params, &result)
	return result, err
}

// OpenPositions returns current open positions with all supported options.
func (e *Exchange) OpenPositions(ctx context.Context, req *OpenPositionsRequest) (map[string]Position, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.Consolidation, "market") {
		return nil, errConsolidationInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	if slices.Contains(req.TransactionIDs, "") {
		return nil, errTransactionIDRequired
	}

	params := url.Values{}
	if len(req.TransactionIDs) != 0 {
		params.Set("txid", strings.Join(req.TransactionIDs, ","))
	}
	if req.DoCalculations {
		params.Set("docalcs", strconv.FormatBool(req.DoCalculations))
	}
	if req.Consolidation != "" {
		params.Set("consolidation", string(req.Consolidation))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result map[string]Position
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "OpenPositions", params, &result)
	return result, err
}

// GetLedgers returns current ledger entries.
func (e *Exchange) GetLedgers(ctx context.Context, req *GetLedgersRequest) (*Ledgers, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.Type,
		"all", "trade", "deposit", "withdrawal", "transfer", "margin", "adjustment", "rollover", "credit", "settled", "staking", "dividend", "sale", "nft_rebate") {
		return nil, errLedgerTypeInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	if slices.Contains(req.Assets, "") {
		return nil, errAssetRequired
	}
	start, err := formatTimeOrTransactionID(req.Start)
	if err != nil {
		return nil, err
	}
	end, err := formatTimeOrTransactionID(req.End)
	if err != nil {
		return nil, err
	}
	if !req.Start.Time.IsZero() && !req.End.Time.IsZero() && req.End.Time.Before(req.Start.Time) {
		return nil, errTimeRangeInvalid
	}

	params := url.Values{}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if len(req.Assets) != 0 {
		params.Set("asset", strings.Join(req.Assets, ","))
	}
	if req.Type != "" {
		params.Set("type", string(req.Type))
	}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if req.Offset != 0 {
		params.Set("ofs", strconv.FormatUint(req.Offset, 10))
	}
	if req.WithoutCount {
		params.Set("without_count", strconv.FormatBool(req.WithoutCount))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result Ledgers
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Ledgers", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// QueryLedgers returns information for up to twenty ledger entries with current options.
func (e *Exchange) QueryLedgers(ctx context.Context, req *QueryLedgersRequest) (map[string]LedgerInfo, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if len(req.IDs) == 0 || len(req.IDs) > 20 {
		return nil, errLedgerIdentifierCount
	}
	if slices.Contains(req.IDs, "") {
		return nil, errIDRequired
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}

	params := url.Values{"id": {strings.Join(req.IDs, ",")}}
	if req.Trades {
		params.Set("trades", strconv.FormatBool(req.Trades))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result map[string]LedgerInfo
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "QueryLedgers", params, &result)
	return result, err
}

type tradeVolumePairWire struct {
	Asset      string     `json:"asset"`
	AssetClass AssetClass `json:"aclass"`
}

// GetTradeVolume returns current trade-volume and fee-schedule data.
func (e *Exchange) GetTradeVolume(ctx context.Context, req *GetTradeVolumeRequest) (*TradeVolumeResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}

	params := url.Values{}
	if len(req.Pairs) != 0 {
		pairs := make([]tradeVolumePairWire, len(req.Pairs))
		for i := range req.Pairs {
			if req.Pairs[i].Asset == "" {
				return nil, errAssetRequired
			}
			if !isValidSpotEnum(req.Pairs[i].AssetClass, "currency", "forex", "equity", "equity_pair", "nft", "derivatives", "tokenized_asset", "futures_contract") || req.Pairs[i].AssetClass == "" {
				return nil, errAssetClassInvalid
			}
			pairs[i] = tradeVolumePairWire{Asset: req.Pairs[i].Asset, AssetClass: req.Pairs[i].AssetClass}
		}
		encodedPairs, _ := json.Marshal(pairs) // The wire model contains only JSON-supported string fields.
		params.Set("pair", string(encodedPairs))
	}
	if req.FeeInfo != nil {
		params.Set("fee-info", strconv.FormatBool(*req.FeeInfo))
	}
	if req.FeeSchedule != nil {
		params.Set("fee_schedule", strconv.FormatBool(*req.FeeSchedule))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result TradeVolumeResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "TradeVolume", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOrderAmends returns amendment history for an order.
func (e *Exchange) GetOrderAmends(ctx context.Context, req *GetOrderAmendsRequest) (*GetOrderAmendsResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.OrderID == "" {
		return nil, errOrderIDRequired
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}

	params := url.Values{"order_id": {req.OrderID}}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result GetOrderAmendsResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "OrderAmends", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RequestExportReport creates an account data export.
func (e *Exchange) RequestExportReport(ctx context.Context, req *RequestExportReportRequest) (*RequestExportReportResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Report == "" {
		return nil, errReportRequired
	}
	if !isValidSpotEnum(req.Report, "trades", "ledgers") {
		return nil, errExportReportInvalid
	}
	if req.Description == "" {
		return nil, errDescriptionRequired
	}
	if !isValidSpotEnum(req.Format, "CSV", "TSV") {
		return nil, errExportFormatInvalid
	}

	params := url.Values{
		"report":      {string(req.Report)},
		"description": {req.Description},
	}
	if req.Format != "" {
		params.Set("format", string(req.Format))
	}
	if len(req.Fields) > 0 {
		fields := make([]string, len(req.Fields))
		for i := range req.Fields {
			valid := false
			switch req.Report {
			case ExportReportTrades:
				valid = isValidSpotEnum(req.Fields[i], "ordertxid", "time", "ordertype", "price", "cost", "fee", "vol", "margin", "misc", "ledgers") && req.Fields[i] != ""
			case ExportReportLedgers:
				valid = isValidSpotEnum(req.Fields[i], "refid", "time", "type", "subtype", "aclass", "asset", "amount", "fee", "balance", "wallet") && req.Fields[i] != ""
			}
			if !valid {
				return nil, errExportFieldInvalid
			}
			fields[i] = string(req.Fields[i])
		}
		params.Set("fields", strings.Join(fields, ","))
	}
	if !req.StartTime.IsZero() {
		if req.StartTime.Unix() < 0 {
			return nil, errTimestampInvalid
		}
		params.Set("starttm", strconv.FormatInt(req.StartTime.Unix(), 10))
	}
	if !req.EndTime.IsZero() {
		if req.EndTime.Unix() < 0 {
			return nil, errTimestampInvalid
		}
		params.Set("endtm", strconv.FormatInt(req.EndTime.Unix(), 10))
	}
	if !req.StartTime.IsZero() && !req.EndTime.IsZero() && req.EndTime.Before(req.StartTime) {
		return nil, errTimeRangeInvalid
	}

	var result RequestExportReportResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AddExport", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetExportReportStatus returns status for account data exports.
func (e *Exchange) GetExportReportStatus(ctx context.Context, req *GetExportReportStatusRequest) ([]ExportReportStatusResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Report == "" {
		return nil, errReportRequired
	}
	if !isValidSpotEnum(req.Report, "trades", "ledgers") {
		return nil, errExportReportInvalid
	}

	var result []ExportReportStatusResponse
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "ExportStatus", url.Values{"report": {string(req.Report)}}, &result)
	return result, err
}

// RetrieveDataExport retrieves a completed account data export archive.
func (e *Exchange) RetrieveDataExport(ctx context.Context, req *RetrieveDataExportRequest) ([]byte, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.ID == "" {
		return nil, errIDRequired
	}

	var result request.RawResponse
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "RetrieveExport", url.Values{"id": {req.ID}}, &result)
	return result, err
}

// DeleteExportReport cancels or deletes an account data export.
func (e *Exchange) DeleteExportReport(ctx context.Context, req *DeleteExportReportRequest) (*DeleteExportReportResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.ID == "" {
		return nil, errIDRequired
	}
	if req.Type == "" {
		return nil, errTypeRequired
	}
	if !isValidSpotEnum(req.Type, "cancel", "delete") {
		return nil, errExportRemovalInvalid
	}

	params := url.Values{"id": {req.ID}, "type": {string(req.Type)}}
	var result DeleteExportReportResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "RemoveExport", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAPIKeyInfo returns metadata for the authenticated API key.
func (e *Exchange) GetAPIKeyInfo(ctx context.Context, req *GetAPIKeyInfoRequest) (*GetAPIKeyInfoResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	params := url.Values{}
	if req.OTP != "" {
		params.Set("otp", req.OTP)
	}

	var result GetAPIKeyInfoResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "GetApiKeyInfo", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
