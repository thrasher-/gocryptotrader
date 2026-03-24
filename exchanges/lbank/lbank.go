package lbank

import (
	"bytes"
	"context"
	"crypto"
	"crypto/md5" //nolint:gosec // Used for this exchange
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Exchange implements exchange.IBotExchange and contains additional specific api methods for interacting with Lbank
type Exchange struct {
	exchange.Base
	privateKey *rsa.PrivateKey
}

const (
	lbankAPIURL      = "https://api.lbkex.com"
	lbankAPIVersion1 = "1"
	lbankAPIVersion2 = "2"
	lbankFeeNotFound = 0.0
	tradeBaseURL     = "https://www.lbank.com/trade/"
	lbankTimeFormat  = "2006-01-02 15:04:05"

	// Public endpoints
	lbankTicker24hr     = "ticker/24hr.do"
	lbankCurrencyPairs  = "currencyPairs.do"
	lbankMarketDepths   = "depth.do"
	lbankTrades         = "trades.do"
	lbankKlines         = "kline.do"
	lbankPairInfo       = "accuracy.do"
	lbankUSD2CNYRate    = "usdToCny.do"
	lbankWithdrawConfig = "withdrawConfigs.do"

	// Authenticated endpoints
	lbankUserInfo                = "user_info.do"
	lbankPlaceOrder              = "create_order.do"
	lbankCancelOrder             = "cancel_order.do"
	lbankQueryOrder              = "orders_info.do"
	lbankQueryHistoryOrder       = "orders_info_history.do"
	lbankOrderTransactionDetails = "order_transaction_detail.do"
	lbankPastTransactions        = "transaction_history.do"
	lbankOpeningOrders           = "orders_info_no_deal.do"
	lbankWithdrawalRecords       = "withdraws.do"
	lbankWithdraw                = "withdraw.do"
	lbankRevokeWithdraw          = "withdrawCancel.do"
	lbankTimestamp               = "timestamp.do"
)

var (
	errPEMBlockIsNil           = errors.New("pem block is nil")
	errUnableToParsePrivateKey = errors.New("unable to parse private key")
	errPrivateKeyNotLoaded     = errors.New("private key not loaded")
	lbankTimeLocation          = time.FixedZone("UTC+8", 8*60*60)
)

// GetTicker returns a ticker for the specified symbol
// symbol: eth_btc
func (e *Exchange) GetTicker(ctx context.Context, symbol string) (*TickerResponse, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	path := common.EncodeURLValues("/v"+lbankAPIVersion2+"/"+lbankTicker24hr, params)
	resp, err := sendPublicV2Request[[]tickerV2Response](ctx, e, path)
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, nil
	}
	ticker := standardiseTickerResponse(&resp[0])
	return &ticker, nil
}

// GetTimestamp returns a timestamp
func (e *Exchange) GetTimestamp(ctx context.Context) (time.Time, error) {
	path := "/v" + lbankAPIVersion2 + "/" + lbankTimestamp
	resp, err := sendPublicV2Request[types.Time](ctx, e, path)
	if err != nil {
		return time.Time{}, err
	}
	return resp.Time(), nil
}

// GetTickers returns all tickers
func (e *Exchange) GetTickers(ctx context.Context) ([]TickerResponse, error) {
	params := url.Values{}
	params.Set("symbol", "all")
	path := common.EncodeURLValues("/v"+lbankAPIVersion2+"/"+lbankTicker24hr, params)
	resp, err := sendPublicV2Request[[]tickerV2Response](ctx, e, path)
	if err != nil {
		return nil, err
	}

	tickers := make([]TickerResponse, len(resp))
	for i := range resp {
		tickers[i] = standardiseTickerResponse(&resp[i])
	}
	return tickers, nil
}

// GetCurrencyPairs returns a list of supported currency pairs by the exchange
func (e *Exchange) GetCurrencyPairs(ctx context.Context) ([]string, error) {
	path := "/v" + lbankAPIVersion2 + "/" + lbankCurrencyPairs
	return sendPublicV2Request[[]string](ctx, e, path)
}

// GetMarketDepths returns arrays of asks, bids and timestamp
func (e *Exchange) GetMarketDepths(ctx context.Context, symbol string, size uint64) (*MarketDepthResponse, error) {
	var m MarketDepthResponse
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("size", strconv.FormatUint(size, 10))
	path := common.EncodeURLValues("/v"+lbankAPIVersion2+"/"+lbankMarketDepths, params)
	return &m, e.SendHTTPRequest(ctx, exchange.RestSpot, path, &m)
}

// GetTrades returns an array of available trades regarding a particular exchange
// The time parameter is optional, if provided it will return trades after the given time
func (e *Exchange) GetTrades(ctx context.Context, symbol string, limit uint64, tm time.Time) ([]TradeResponse, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	if limit > 0 {
		params.Set("size", strconv.FormatUint(limit, 10))
	}
	if !tm.IsZero() {
		params.Set("time", strconv.FormatInt(tm.UnixMilli(), 10))
	}
	path := common.EncodeURLValues("/v"+lbankAPIVersion2+"/supplement/"+lbankTrades, params)
	resp, err := sendPublicV2Request[[]tradeV2Response](ctx, e, path)
	if err != nil {
		return nil, err
	}
	return standardiseTradeResponses(resp), nil
}

// GetKlines returns kline data
func (e *Exchange) GetKlines(ctx context.Context, symbol, size, klineType string, tm time.Time) ([]KlineResponse, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("size", size)
	params.Set("type", klineType)
	params.Set("time", strconv.FormatInt(tm.Unix(), 10))
	path := common.EncodeURLValues("/v"+lbankAPIVersion2+"/"+lbankKlines, params)
	klineTemp, err := sendPublicV2Request[[][]float64](ctx, e, path)
	if err != nil {
		return nil, err
	}

	k := make([]KlineResponse, len(klineTemp))
	for x := range klineTemp {
		if len(klineTemp[x]) < 6 {
			return nil, errors.New("unexpected kline data length")
		}
		k[x] = KlineResponse{
			TimeStamp:     time.Unix(int64(klineTemp[x][0]), 0).UTC(),
			OpenPrice:     klineTemp[x][1],
			HighestPrice:  klineTemp[x][2],
			LowestPrice:   klineTemp[x][3],
			ClosePrice:    klineTemp[x][4],
			TradingVolume: klineTemp[x][5],
		}
	}
	return k, nil
}

// GetUserInfo gets users account info
func (e *Exchange) GetUserInfo(ctx context.Context) (InfoFinalResponse, error) {
	var resp InfoFinalResponse
	path := "/v" + lbankAPIVersion2 + "/supplement/" + lbankUserInfo
	info, err := sendAuthV2Request[[]userInfoV2Response](ctx, e, path, nil)
	if err != nil {
		return resp, err
	}

	resp.Info.Asset = make(map[string]types.Number, len(info))
	resp.Info.Freeze = make(map[string]types.Number, len(info))
	resp.Info.Free = make(map[string]types.Number, len(info))
	for i := range info {
		resp.Info.Asset[info[i].Coin] = info[i].AssetAmount
		resp.Info.Freeze[info[i].Coin] = info[i].FreezeAmount
		resp.Info.Free[info[i].Coin] = info[i].UsableAmount
	}
	return resp, nil
}

// CreateOrder creates an order
func (e *Exchange) CreateOrder(ctx context.Context, pair, side string, amount, price float64) (CreateOrderResponse, error) {
	var resp CreateOrderResponse
	if !strings.EqualFold(side, order.Buy.String()) && !strings.EqualFold(side, order.Sell.String()) {
		return resp, order.ErrSideIsInvalid
	}
	if amount <= 0 {
		return resp, limits.ErrAmountBelowMin
	}
	if price <= 0 {
		return resp, limits.ErrPriceBelowMin
	}

	params := url.Values{}
	params.Set("symbol", pair)
	params.Set("type", strings.ToLower(side))
	params.Set("price", strconv.FormatFloat(price, 'f', -1, 64))
	params.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	path := "/v" + lbankAPIVersion2 + "/supplement/" + lbankPlaceOrder
	orderResp, err := sendAuthV2Request[createOrderV2Response](ctx, e, path, params)
	if err != nil {
		return resp, err
	}

	resp.OrderID = orderResp.OrderID
	return resp, nil
}

// RemoveOrder cancels a given order
func (e *Exchange) RemoveOrder(ctx context.Context, pair, orderID string) (RemoveOrderResponse, error) {
	var resp RemoveOrderResponse
	orderIDs := splitCommaSeparatedValues(orderID)
	if len(orderIDs) > 1 {
		successes := make([]string, 0, len(orderIDs))
		for i := range orderIDs {
			cancelResp, err := e.RemoveOrder(ctx, pair, orderIDs[i])
			if err != nil {
				resp.Success = strings.Join(successes, ",")
				return resp, err
			}
			successes = append(successes, cancelResp.Success)
		}
		resp.Success = strings.Join(successes, ",")
		return resp, nil
	}

	params := url.Values{}
	params.Set("symbol", pair)
	params.Set("orderId", orderIDs[0])
	path := "/v" + lbankAPIVersion2 + "/supplement/" + lbankCancelOrder
	_, err := sendAuthV2Request[struct{}](ctx, e, path, params)
	if err != nil {
		return resp, err
	}
	resp.OrderID = orderIDs[0]
	resp.Success = orderIDs[0]
	return resp, nil
}

// QueryOrder finds out information about orders.
func (e *Exchange) QueryOrder(ctx context.Context, pair, orderIDs string) (QueryOrderFinalResponse, error) {
	var resp QueryOrderFinalResponse
	path := "/v" + lbankAPIVersion2 + "/spot/trade/" + lbankQueryOrder
	for _, orderID := range splitCommaSeparatedValues(orderIDs) {
		params := url.Values{}
		params.Set("symbol", pair)
		params.Set("orderId", orderID)
		orderResp, err := sendAuthV2Request[orderV2Response](ctx, e, path, params)
		if err != nil {
			return resp, err
		}
		resp.Orders = append(resp.Orders, standardiseOrderResponse(&orderResp))
	}
	return resp, nil
}

// QueryOrderHistory finds order info in the past 2 days.
func (e *Exchange) QueryOrderHistory(ctx context.Context, pair, pageNumber, pageLength string) (OrderHistoryFinalResponse, error) {
	var resp OrderHistoryFinalResponse
	params := url.Values{}
	params.Set("symbol", pair)
	params.Set("current_page", pageNumber)
	params.Set("page_length", pageLength)
	path := "/v" + lbankAPIVersion2 + "/spot/trade/" + lbankQueryHistoryOrder
	orderResp, err := sendAuthV2Request[pagedOrdersV2Response](ctx, e, path, params)
	if err != nil {
		return resp, err
	}

	resp.PageLength = orderResp.PageLength
	resp.Orders = standardiseOrderResponses(orderResp.Orders)
	resp.CurrentPage = orderResp.CurrentPage
	return resp, nil
}

// GetPairInfo finds information about all trading pairs
func (e *Exchange) GetPairInfo(ctx context.Context) ([]PairInfoResponse, error) {
	path := "/v" + lbankAPIVersion2 + "/" + lbankPairInfo
	return sendPublicV2Request[[]PairInfoResponse](ctx, e, path)
}

// OrderTransactionDetails gets info about transactions.
// LBank only references `/v2/order_transaction_detail.do` in the changelog and does not publish
// a stable request/response contract for it, so this wrapper intentionally stays on the v1 path.
func (e *Exchange) OrderTransactionDetails(ctx context.Context, symbol, orderID string) (TransactionHistoryResp, error) {
	var resp TransactionHistoryResp
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("order_id", orderID)
	path := "/v" + lbankAPIVersion1 + "/" + lbankOrderTransactionDetails
	return resp, e.SendAuthHTTPRequest(ctx, http.MethodPost, path, params, &resp)
}

// TransactionHistory stores info about transactions using the documented v2 request shape.
func (e *Exchange) TransactionHistory(ctx context.Context, req *TransactionHistoryRequest) (TransactionHistoryResp, error) {
	var resp TransactionHistoryResp
	if err := req.validate(); err != nil {
		return resp, err
	}

	params := url.Values{}
	params.Set("symbol", req.Symbol)
	if !req.StartTime.IsZero() {
		params.Set("startTime", formatLBankTime(req.StartTime))
	}
	if !req.EndTime.IsZero() {
		params.Set("endTime", formatLBankTime(req.EndTime))
	}
	if req.FromID != "" {
		params.Set("fromId", req.FromID)
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.FormatUint(req.Limit, 10))
	}
	path := "/v" + lbankAPIVersion2 + "/supplement/" + lbankPastTransactions
	transactionResp, err := sendAuthV2Request[[]transactionV2Response](ctx, e, path, params)
	if err != nil {
		return resp, err
	}
	resp.Transaction = standardiseTransactionResponses(transactionResp)
	return resp, nil
}

// GetOpenOrders gets opening orders.
func (e *Exchange) GetOpenOrders(ctx context.Context, pair, pageNumber, pageLength string) (OpenOrderFinalResponse, error) {
	var resp OpenOrderFinalResponse
	params := url.Values{}
	params.Set("symbol", pair)
	params.Set("current_page", pageNumber)
	params.Set("page_length", pageLength)
	path := "/v" + lbankAPIVersion2 + "/supplement/" + lbankOpeningOrders
	orderResp, err := sendAuthV2Request[pagedOrdersV2Response](ctx, e, path, params)
	if err != nil {
		return resp, err
	}

	resp.PageLength = orderResp.PageLength
	resp.PageNumber = orderResp.CurrentPage
	resp.Total = orderResp.Total.String()
	resp.Orders = standardiseOrderResponses(orderResp.Orders)
	return resp, nil
}

// USD2RMBRate finds USD-CNY Rate
func (e *Exchange) USD2RMBRate(ctx context.Context) (ExchangeRateResponse, error) {
	path := "/v" + lbankAPIVersion2 + "/" + lbankUSD2CNYRate
	resp, err := sendPublicV2Request[string](ctx, e, path)
	if err != nil {
		return ExchangeRateResponse{}, err
	}
	return ExchangeRateResponse{USD2CNY: resp}, nil
}

// GetWithdrawConfig gets information about withdrawals
func (e *Exchange) GetWithdrawConfig(ctx context.Context, c currency.Code) ([]WithdrawConfigResponse, error) {
	params := url.Values{}
	params.Set("assetCode", c.Lower().String())
	path := common.EncodeURLValues("/v"+lbankAPIVersion2+"/"+lbankWithdrawConfig, params)
	return sendPublicV2Request[[]WithdrawConfigResponse](ctx, e, path)
}

// Withdraw sends a withdrawal request using the documented v2 wallet contract.
func (e *Exchange) Withdraw(ctx context.Context, req *WithdrawRequest) (WithdrawResponse, error) {
	var resp WithdrawResponse
	if err := req.validate(); err != nil {
		return resp, err
	}

	params := url.Values{}
	params.Set("address", req.Address)
	params.Set("coin", req.Coin.Lower().String())
	params.Set("amount", strconv.FormatFloat(req.Amount, 'f', -1, 64))
	params.Set("fee", strconv.FormatFloat(req.Fee, 'f', -1, 64))
	if req.NetworkName != "" {
		params.Set("networkName", req.NetworkName)
	}
	if req.Memo != "" {
		params.Set("memo", req.Memo)
	}
	if req.Mark != "" {
		params.Set("mark", req.Mark)
	}
	if req.Name != "" {
		params.Set("name", req.Name)
	}
	if req.WithdrawOrderID != "" {
		params.Set("withdrawOrderId", req.WithdrawOrderID)
	}
	if req.Type != "" {
		params.Set("type", req.Type)
	}
	path := "/v" + lbankAPIVersion2 + "/spot/wallet/" + lbankWithdraw
	withdrawResp, err := sendAuthV2Request[withdrawV2Response](ctx, e, path, params)
	if err != nil {
		return resp, err
	}
	resp.WithdrawID = withdrawResp.WithdrawID.String()
	resp.Fee = withdrawResp.Fee.Float64()
	return resp, nil
}

// RevokeWithdraw cancels the withdrawal given the withdrawalID.
// LBank's current v2 documentation does not publish a replacement cancel-withdraw endpoint.
func (e *Exchange) RevokeWithdraw(ctx context.Context, withdrawID string) (RevokeWithdrawResponse, error) {
	var resp RevokeWithdrawResponse
	params := url.Values{}
	if withdrawID != "" {
		params.Set("withdrawId", withdrawID)
	}
	path := "/v" + lbankAPIVersion1 + "/" + lbankRevokeWithdraw
	return resp, e.SendAuthHTTPRequest(ctx, http.MethodPost, path, params, &resp)
}

// GetWithdrawalRecords gets withdrawal records using the documented v2 wallet history endpoint.
func (e *Exchange) GetWithdrawalRecords(ctx context.Context, req *WithdrawalRecordsRequest) ([]WithdrawalRecord, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}

	params := url.Values{}
	if !req.Coin.IsEmpty() {
		params.Set("coin", req.Coin.Lower().String())
	}
	if req.Status != "" {
		params.Set("status", req.Status)
	}
	if req.WithdrawOrderID != "" {
		params.Set("withdrawOrderId", req.WithdrawOrderID)
	}
	if !req.StartTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(req.StartTime.UnixMilli(), 10))
	}
	if !req.EndTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(req.EndTime.UnixMilli(), 10))
	}
	path := "/v" + lbankAPIVersion2 + "/spot/wallet/" + lbankWithdrawalRecords
	recordsResp, err := sendAuthV2Request[[]withdrawalRecordV2Response](ctx, e, path, params)
	if err != nil {
		return nil, err
	}
	return standardiseWithdrawalRecordResponses(recordsResp), nil
}

// ErrorCapture captures errors
func ErrorCapture(code int64) error {
	msg, ok := errorCodes[code]
	if !ok {
		return fmt.Errorf("undefined code please check api docs for error code definition: %v", code)
	}
	return errors.New(msg)
}

func (e ErrCapture) responseErrorCode() int64 {
	if e.Error != 0 {
		return e.Error
	}
	if e.Code != 0 {
		return e.Code
	}
	return e.Error
}

func (e ErrCapture) responseErrorMessage() string {
	return e.Message
}

type responseError interface {
	responseErrorCode() int64
	responseErrorMessage() string
}

func captureResponseError(result any) error {
	resp, ok := result.(responseError)
	if !ok || resp.responseErrorCode() == 0 {
		return nil
	}

	msg := strings.TrimSpace(resp.responseErrorMessage())
	err := ErrorCapture(resp.responseErrorCode())
	if msg == "" || strings.EqualFold(msg, "success") || strings.EqualFold(msg, err.Error()) {
		return err
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func standardiseTickerResponse(resp *tickerV2Response) TickerResponse {
	return TickerResponse{
		Symbol:    resp.Symbol,
		Timestamp: resp.Timestamp,
		Ticker: Ticker{
			Change:   resp.Ticker.Change.Float64(),
			High:     resp.Ticker.High.Float64(),
			Latest:   resp.Ticker.Latest.Float64(),
			Low:      resp.Ticker.Low.Float64(),
			Turnover: resp.Ticker.Turnover.Float64(),
			Volume:   resp.Ticker.Volume.Float64(),
		},
	}
}

func standardiseTradeResponse(resp tradeV2Response) TradeResponse {
	tradeType := strings.ToLower(order.Buy.String())
	if resp.IsBuyerMaker {
		tradeType = strings.ToLower(order.Sell.String())
	}
	return TradeResponse{
		DateMS: resp.Time,
		Amount: resp.Quantity.Float64(),
		Price:  resp.Price.Float64(),
		Type:   tradeType,
		TID:    resp.ID,
	}
}

func standardiseTradeResponses(resp []tradeV2Response) []TradeResponse {
	trades := make([]TradeResponse, len(resp))
	for i := range resp {
		trades[i] = standardiseTradeResponse(resp[i])
	}
	return trades
}

func standardiseOrderResponse(resp *orderV2Response) OrderResponse {
	executedQuantity := resp.ExecutedQuantity.Float64()
	averagePrice := 0.0
	if executedQuantity > 0 {
		averagePrice = resp.CumulativeQuoteQuantity.Float64() / executedQuantity
	}

	return OrderResponse{
		Symbol:     resp.Symbol,
		Amount:     resp.OriginalQuantity.Float64(),
		CreateTime: resp.Time,
		Price:      resp.Price.Float64(),
		AvgPrice:   averagePrice,
		Type:       resp.Type,
		OrderID:    resp.OrderID,
		DealAmount: executedQuantity,
		Status:     resp.Status,
	}
}

func standardiseOrderResponses(resp []orderV2Response) []OrderResponse {
	orders := make([]OrderResponse, len(resp))
	for i := range resp {
		orders[i] = standardiseOrderResponse(&resp[i])
	}
	return orders
}

func standardiseTransactionResponses(resp []transactionV2Response) []TransactionTemp {
	transactions := make([]TransactionTemp, len(resp))
	for i := range resp {
		tradeType := strings.ToLower(order.Sell.String())
		if resp[i].IsBuyer {
			tradeType = strings.ToLower(order.Buy.String())
		}
		transactions[i] = TransactionTemp{
			TxUUID:       resp[i].ID,
			OrderUUID:    resp[i].OrderID,
			TradeType:    tradeType,
			DealTime:     resp[i].Time,
			DealPrice:    resp[i].Price.Float64(),
			DealQuantity: resp[i].Quantity.Float64(),
			DealVolPrice: resp[i].QuoteQty.Float64(),
			TradeFee:     resp[i].Commission.Float64(),
		}
	}
	return transactions
}

func standardiseWithdrawalRecordResponse(resp *withdrawalRecordV2Response) WithdrawalRecord {
	coin := resp.Coin
	if coin.IsEmpty() {
		coin = resp.CoID
	}
	return WithdrawalRecord{
		Amount:          resp.Amount.Float64(),
		Coin:            coin,
		Address:         resp.Address,
		WithdrawOrderID: resp.WithdrawOrderID,
		Fee:             resp.Fee.Float64(),
		NetworkName:     resp.NetworkName,
		TransferType:    resp.TransferType,
		TransactionID:   resp.TransactionID,
		FeeAssetCode:    resp.FeeAssetCode,
		ID:              resp.ID,
		ApplyTime:       resp.ApplyTime.Time(),
		Status:          resp.Status,
	}
}

func standardiseWithdrawalRecordResponses(resp []withdrawalRecordV2Response) []WithdrawalRecord {
	records := make([]WithdrawalRecord, len(resp))
	for i := range resp {
		records[i] = standardiseWithdrawalRecordResponse(&resp[i])
	}
	return records
}

func sendPublicV2Request[T any](ctx context.Context, e *Exchange, path string) (T, error) {
	var resp dataResponse[T]
	err := e.SendHTTPRequest(ctx, exchange.RestSpot, path, &resp)
	if err != nil {
		var zero T
		return zero, err
	}
	return resp.Data, nil
}

func sendAuthV2Request[T any](ctx context.Context, e *Exchange, path string, params url.Values) (T, error) {
	var resp dataResponse[T]
	err := e.SendAuthHTTPRequest(ctx, http.MethodPost, path, params, &resp)
	if err != nil {
		var zero T
		return zero, err
	}
	return resp.Data, nil
}

func splitCommaSeparatedValues(input string) []string {
	parts := strings.Split(input, ",")
	values := make([]string, 0, len(parts))
	for i := range parts {
		part := strings.TrimSpace(parts[i])
		if part != "" {
			values = append(values, part)
		}
	}
	if len(values) == 0 {
		return []string{""}
	}
	return values
}

func formatLBankTime(input time.Time) string {
	return input.In(lbankTimeLocation).Format(lbankTimeFormat)
}

func (r *TransactionHistoryRequest) validate() error {
	if r == nil {
		return common.ErrNilPointer
	}
	if strings.TrimSpace(r.Symbol) == "" {
		return errors.New("symbol cannot be empty")
	}
	if !r.StartTime.IsZero() && !r.EndTime.IsZero() && r.EndTime.Before(r.StartTime) {
		return errors.New("end time cannot be before start time")
	}
	return nil
}

func (r *WithdrawRequest) validate() error {
	if r == nil {
		return common.ErrNilPointer
	}
	if strings.TrimSpace(r.Address) == "" {
		return errors.New("address cannot be empty")
	}
	if r.Coin.IsEmpty() {
		return errors.New("coin cannot be empty")
	}
	if r.Amount <= 0 {
		return errors.New("amount must be greater than zero")
	}
	if r.Fee < 0 {
		return errors.New("fee cannot be negative")
	}
	return nil
}

func (r *WithdrawalRecordsRequest) validate() error {
	if r == nil {
		return common.ErrNilPointer
	}
	if !r.StartTime.IsZero() && !r.EndTime.IsZero() && r.EndTime.Before(r.StartTime) {
		return errors.New("end time cannot be before start time")
	}
	return nil
}

// SendHTTPRequest sends an unauthenticated HTTP request
func (e *Exchange) SendHTTPRequest(ctx context.Context, ep exchange.URL, path string, result any) error {
	endpoint, err := e.API.Endpoints.GetURL(ep)
	if err != nil {
		return err
	}

	item := &request.Item{
		Method:                 http.MethodGet,
		Path:                   endpoint + path,
		Result:                 result,
		Verbose:                e.Verbose,
		HTTPDebugging:          e.HTTPDebugging,
		HTTPRecording:          e.HTTPRecording,
		HTTPMockDataSliceLimit: e.HTTPMockDataSliceLimit,
	}

	err = e.SendPayload(ctx, request.Unset, func() (*request.Item, error) {
		return item, nil
	}, request.UnauthenticatedRequest)
	if err != nil {
		return err
	}
	return captureResponseError(result)
}

func (e *Exchange) loadPrivKey(ctx context.Context) error {
	creds, err := e.GetCredentials(ctx)
	if err != nil {
		return err
	}
	key := strings.Join([]string{
		"-----BEGIN RSA PRIVATE KEY-----",
		creds.Secret,
		"-----END RSA PRIVATE KEY-----",
	}, "\n")

	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return errPEMBlockIsNil
	}

	p, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("%w: %w", errUnableToParsePrivateKey, err)
	}

	var ok bool
	e.privateKey, ok = p.(*rsa.PrivateKey)
	if !ok {
		return common.GetTypeAssertError("*rsa.PrivateKey", p)
	}
	return nil
}

func (e *Exchange) sign(data string) (string, error) {
	if e.privateKey == nil {
		return "", errPrivateKeyNotLoaded
	}
	md5sum := md5.Sum([]byte(data)) //nolint:gosec // Used for this exchange
	shasum := sha256.Sum256([]byte(strings.ToUpper(hex.EncodeToString(md5sum[:]))))
	r, err := rsa.SignPKCS1v15(rand.Reader, e.privateKey, crypto.SHA256, shasum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(r), nil
}

// SendAuthHTTPRequest sends an authenticated request
func (e *Exchange) SendAuthHTTPRequest(ctx context.Context, method, endpoint string, vals url.Values, result any) error {
	creds, err := e.GetCredentials(ctx)
	if err != nil {
		return err
	}
	baseURL, err := e.API.Endpoints.GetURL(exchange.RestSpot)
	if err != nil {
		return err
	}

	if vals == nil {
		vals = url.Values{}
	}

	vals.Set("api_key", creds.Key)
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}
	if strings.HasPrefix(endpoint, "/v"+lbankAPIVersion2+"/") {
		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
		echostr, err := common.GenerateRandomString(32, common.SmallLetters, common.CapitalLetters, common.NumberCharacters)
		if err != nil {
			return err
		}
		vals.Set("signature_method", "RSA")
		vals.Set("timestamp", timestamp)
		vals.Set("echostr", echostr)
		headers["signature_method"] = "RSA"
		headers["timestamp"] = timestamp
		headers["echostr"] = echostr
	}
	sig, err := e.sign(vals.Encode())
	if err != nil {
		return err
	}

	vals.Set("sign", sig)
	payload := vals.Encode()
	requestPath := endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		requestPath = baseURL + endpoint
	}

	item := &request.Item{
		Method:                 method,
		Path:                   requestPath,
		Headers:                headers,
		Result:                 result,
		Verbose:                e.Verbose,
		HTTPDebugging:          e.HTTPDebugging,
		HTTPRecording:          e.HTTPRecording,
		HTTPMockDataSliceLimit: e.HTTPMockDataSliceLimit,
	}

	err = e.SendPayload(ctx, request.Unset, func() (*request.Item, error) {
		item.Body = bytes.NewBufferString(payload)
		return item, nil
	}, request.AuthenticatedRequest)
	if err != nil {
		return err
	}
	return captureResponseError(result)
}
