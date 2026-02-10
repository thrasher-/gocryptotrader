package kraken

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/crypto"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/nonce"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/log"
	"github.com/thrasher-corp/gocryptotrader/types"
)

const (
	krakenAPIURL                  = "https://api.kraken.com"
	krakenFuturesURL              = "https://futures.kraken.com/derivatives"
	krakenFuturesSupplementaryURL = "https://futures.kraken.com/api/"
	tradeBaseURL                  = "https://pro.kraken.com/app/trade/"
	tradeFuturesURL               = "https://futures.kraken.com/trade/futures/"
)

// Exchange implements exchange.IBotExchange and contains additional specific api methods for interacting with Kraken
type Exchange struct {
	exchange.Base
	wsAuthToken string
	wsAuthMtx   sync.RWMutex
}

// GetCurrentServerTime returns current server time
func (e *Exchange) GetCurrentServerTime(ctx context.Context) (*TimeResponse, error) {
	var result TimeResponse
	return &result, e.SendHTTPRequest(ctx, exchange.RestSpot, "/0/public/Time", &result)
}

// GetSystemStatus returns current Kraken system status
func (e *Exchange) GetSystemStatus(ctx context.Context) (*SystemStatusResponse, error) {
	var result SystemStatusResponse
	return &result, e.SendHTTPRequest(ctx, exchange.RestSpot, "/0/public/SystemStatus", &result)
}

// SeedAssets seeds Kraken's asset list and stores it in the
// asset translator
func (e *Exchange) SeedAssets(ctx context.Context) error {
	assets, err := e.GetAssets(ctx, &GetAssetsRequest{})
	if err != nil {
		return err
	}
	for orig, val := range assets {
		assetTranslator.Seed(orig, val.Altname)
	}

	assetPairs, err := e.GetAssetPairs(ctx, &GetAssetPairsRequest{})
	if err != nil {
		return err
	}
	for k, v := range assetPairs {
		assetTranslator.Seed(k, v.Altname)
	}
	return nil
}

// GetAssets returns a full asset list
func (e *Exchange) GetAssets(ctx context.Context, req *GetAssetsRequest) (map[string]*Asset, error) {
	params := url.Values{}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.Aclass != "" {
		params.Set("aclass", req.Aclass)
	}

	var result map[string]*Asset
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Assets", params), &result)
}

// GetAssetPairs returns a full asset pair list
// Parameter 'info' only supports 4 strings: "fees", "leverage", "margin", "info" <- (default)
func (e *Exchange) GetAssetPairs(ctx context.Context, req *GetAssetPairsRequest) (map[string]*AssetPairs, error) {
	params := url.Values{}
	if len(req.AssetPairs) != 0 {
		params.Set("pair", strings.Join(req.AssetPairs, ","))
	}

	if req.AssetClassBase != "" {
		params.Set("aclass_base", req.AssetClassBase)
	}
	if req.CountryCode != "" {
		params.Set("country_code", req.CountryCode)
	}

	var result map[string]*AssetPairs
	if req.Info != "" {
		if req.Info != "margin" && req.Info != "leverage" && req.Info != "fees" && req.Info != "info" {
			return nil, errInvalidAssetPairInfo
		}
		params.Set("info", req.Info)
	}
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/AssetPairs", params), &result)
}

// GetTicker returns ticker information from kraken
func (e *Exchange) GetTicker(ctx context.Context, req *GetTickerRequest) (*Ticker, error) {
	values := url.Values{}
	symbolValue, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}
	values.Set("pair", symbolValue)
	if req.AssetClass != "" {
		values.Set("asset_class", req.AssetClass)
	}

	var data map[string]*TickerResponse
	if err := e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Ticker", values), &data); err != nil {
		return nil, err
	}

	var tick Ticker
	for _, v := range data {
		tick.Ask = v.Ask[0].Float64()
		tick.AskSize = v.Ask[2].Float64()
		tick.Bid = v.Bid[0].Float64()
		tick.BidSize = v.Bid[2].Float64()
		tick.Last = v.Last[0].Float64()
		tick.Volume = v.Volume[1].Float64()
		tick.VolumeWeightedAveragePrice = v.VolumeWeightedAveragePrice[1].Float64()
		tick.Trades = v.Trades[1]
		tick.Low = v.Low[1].Float64()
		tick.High = v.High[1].Float64()
		tick.Open = v.Open.Float64()
	}
	return &tick, nil
}

// GetTickers supports fetching multiple tickers from Kraken
// pairList must be in the format pairs separated by commas
// ("LTCUSD,ETCUSD")
func (e *Exchange) GetTickers(ctx context.Context, req *GetTickersRequest) (map[string]Ticker, error) {
	values := url.Values{}
	if req.PairList != "" {
		values.Set("pair", req.PairList)
	}
	if req.AssetClass != "" {
		values.Set("asset_class", req.AssetClass)
	}

	var result map[string]*TickerResponse
	err := e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Ticker", values), &result)
	if err != nil {
		return nil, err
	}

	tickers := make(map[string]Ticker, len(result))
	for k, v := range result {
		tickers[k] = Ticker{
			Ask:                        v.Ask[0].Float64(),
			AskSize:                    v.Ask[2].Float64(),
			Bid:                        v.Bid[0].Float64(),
			BidSize:                    v.Bid[2].Float64(),
			Last:                       v.Last[0].Float64(),
			Volume:                     v.Volume[1].Float64(),
			VolumeWeightedAveragePrice: v.VolumeWeightedAveragePrice[1].Float64(),
			Trades:                     v.Trades[1],
			Low:                        v.Low[1].Float64(),
			High:                       v.High[1].Float64(),
			Open:                       v.Open.Float64(),
		}
	}
	return tickers, nil
}

// GetOHLC returns an array of open high low close values of a currency pair
func (e *Exchange) GetOHLC(ctx context.Context, req *GetOHLCRequest) ([]OpenHighLowClose, error) {
	values := url.Values{}
	symbolValue, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}
	translatedAsset := assetTranslator.LookupCurrency(symbolValue)
	if translatedAsset == "" {
		translatedAsset = symbolValue
	}
	values.Set("pair", translatedAsset)
	values.Set("interval", req.Interval)
	if !req.Since.IsZero() {
		values.Set("since", strconv.FormatInt(req.Since.Unix(), 10))
	}
	if req.AssetClass != "" {
		values.Set("asset_class", req.AssetClass)
	}

	var result OHLCResponse
	err = e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/OHLC", values), &result)
	if err != nil {
		return nil, err
	}

	ohlcData, ok := result.Data[translatedAsset]
	if !ok {
		return nil, errInvalidDataReturned
	}

	ohlc := make([]OpenHighLowClose, len(ohlcData))
	for x := range ohlcData {
		ohlc[x] = OpenHighLowClose{
			Time:                       ohlcData[x].Time.Time(),
			Open:                       ohlcData[x].Open.Float64(),
			High:                       ohlcData[x].High.Float64(),
			Low:                        ohlcData[x].Low.Float64(),
			Close:                      ohlcData[x].Close.Float64(),
			VolumeWeightedAveragePrice: ohlcData[x].VolumeWeightedAveragePrice.Float64(),
			Volume:                     ohlcData[x].Volume.Float64(),
			Count:                      ohlcData[x].Count,
		}
	}
	return ohlc, nil
}

// GetDepth returns the orderbook for a particular currency
func (e *Exchange) GetDepth(ctx context.Context, req *GetDepthRequest) (*Orderbook, error) {
	symbolValue, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("pair", symbolValue)
	if req.Count > 0 {
		values.Set("count", strconv.FormatUint(req.Count, 10))
	}
	if req.AssetClass != "" {
		values.Set("asset_class", req.AssetClass)
	}

	type orderbookStructure struct {
		Bids [][3]types.Number `json:"bids"`
		Asks [][3]types.Number `json:"asks"`
	}

	result := make(map[string]*orderbookStructure)
	if err := e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Depth", values), &result); err != nil {
		return nil, err
	}

	ob := new(Orderbook)
	for _, v := range result {
		ob.Asks = make([]OrderbookBase, len(v.Asks))
		ob.Bids = make([]OrderbookBase, len(v.Bids))

		for x := range v.Asks {
			ob.Asks[x].Price = v.Asks[x][0]
			ob.Asks[x].Amount = v.Asks[x][1]
			ob.Asks[x].Timestamp = time.Unix(v.Asks[x][2].Int64(), 0)
		}

		for x := range v.Bids {
			ob.Bids[x].Price = v.Bids[x][0]
			ob.Bids[x].Amount = v.Bids[x][1]
			ob.Bids[x].Timestamp = time.Unix(v.Bids[x][2].Int64(), 0)
		}
	}

	return ob, nil
}

// GetGroupedOrderBook returns grouped L2 orderbook data for a currency pair.
func (e *Exchange) GetGroupedOrderBook(ctx context.Context, req *GroupedOrderBookRequest) (*GroupedOrderBookResponse, error) {
	symbolValue, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("pair", symbolValue)
	if req.Depth > 0 {
		params.Set("depth", strconv.FormatUint(req.Depth, 10))
	}
	if req.Grouping > 0 {
		params.Set("grouping", strconv.FormatUint(req.Grouping, 10))
	}

	var result *GroupedOrderBookResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/GroupedBook", params), &result)
}

// QueryLevel3OrderBook returns L3 orderbook data for a currency pair.
func (e *Exchange) QueryLevel3OrderBook(ctx context.Context, req *QueryLevel3OrderBookRequest) (*QueryLevel3OrderBookResponse, error) {
	symbolValue, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("pair", symbolValue)
	if req.Depth > 0 {
		params.Set("depth", strconv.FormatUint(req.Depth, 10))
	}

	var result *QueryLevel3OrderBookResponse
	return result, e.sendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "Level3", params, &result)
}

// GetTrades returns current trades on Kraken
func (e *Exchange) GetTrades(ctx context.Context, req *GetTradesRequest) (*RecentTradesResponse, error) {
	symbolValue, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("pair", assetTranslator.LookupCurrency(symbolValue))
	if !req.Since.IsZero() {
		values.Set("since", strconv.FormatInt(req.Since.Unix(), 10))
	}
	if req.Count > 0 {
		values.Set("count", strconv.FormatUint(req.Count, 10))
	}
	if req.AssetClass != "" {
		values.Set("asset_class", req.AssetClass)
	}

	var resp *RecentTradesResponse
	return resp, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Trades", values), &resp)
}

// GetSpread returns the full spread on Kraken
func (e *Exchange) GetSpread(ctx context.Context, req *GetSpreadRequest) (*SpreadResponse, error) {
	symbolValue, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("pair", symbolValue)
	if !req.Since.IsZero() {
		values.Set("since", strconv.FormatInt(req.Since.Unix(), 10))
	}
	if req.AssetClass != "" {
		values.Set("asset_class", req.AssetClass)
	}
	var peanutButter *SpreadResponse
	return peanutButter, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Spread", values), &peanutButter)
}

// GetAccountBalance returns account balances by currency
func (e *Exchange) GetAccountBalance(ctx context.Context, req *GetAccountBalanceRequest) (map[string]types.Number, error) {
	params := url.Values{}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result map[string]types.Number
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Balance", params, &result)
}

// GetExtendedBalance returns account balances and held amounts by currency
func (e *Exchange) GetExtendedBalance(ctx context.Context, req *GetExtendedBalanceRequest) (map[string]Balance, error) {
	params := url.Values{}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result map[string]Balance
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "BalanceEx", params, &result)
}

// GetBalance returns your extended balance associated with your keys
func (e *Exchange) GetBalance(ctx context.Context) (map[string]Balance, error) {
	return e.GetExtendedBalance(ctx, &GetExtendedBalanceRequest{})
}

// GetWithdrawInfo gets withdrawal fees
func (e *Exchange) GetWithdrawInfo(ctx context.Context, withdrawalAsset, withdrawalKey string, amount float64) (*WithdrawInformation, error) {
	params := url.Values{}
	params.Set("asset", withdrawalAsset)
	params.Set("key", withdrawalKey)
	params.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))

	var result *WithdrawInformation
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawInfo", params, &result)
}

// Withdraw withdraws funds
func (e *Exchange) Withdraw(ctx context.Context, req *WithdrawRequest) (string, error) {
	params := url.Values{}
	params.Set("asset", req.Asset)
	params.Set("key", req.Key)
	params.Set("amount", strconv.FormatFloat(req.Amount, 'f', -1, 64))
	if req.AssetClass != "" {
		params.Set("aclass", req.AssetClass)
	}
	if req.Address != "" {
		params.Set("address", req.Address)
	}
	if req.MaxFee != "" {
		params.Set("max_fee", req.MaxFee)
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var response WithdrawResponse
	return response.ReferenceID, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Withdraw", params, &response)
}

// GetDepositMethods gets withdrawal fees for a specific asset
func (e *Exchange) GetDepositMethods(ctx context.Context, req *GetDepositMethodsRequest) ([]DepositMethods, error) {
	params := url.Values{}
	params.Set("asset", req.Asset)
	if req.AssetClass != "" {
		params.Set("aclass", req.AssetClass)
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result []DepositMethods
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "DepositMethods", params, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetTradeBalance returns full information about your trades on Kraken
func (e *Exchange) GetTradeBalance(ctx context.Context, req *TradeBalanceOptions) (*TradeBalanceInfo, error) {
	params := url.Values{}

	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}

	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result TradeBalanceInfo
	return &result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "TradeBalance", params, &result)
}

// GetOpenOrders returns all current open orders
func (e *Exchange) GetOpenOrders(ctx context.Context, args OrderInfoOptions) (*OpenOrders, error) {
	params := url.Values{}

	if args.Trades {
		params.Set("trades", "true")
	}

	if args.UserRef != 0 {
		params.Set("userref", strconv.FormatInt(int64(args.UserRef), 10))
	}

	if args.ClientOrderID != "" {
		params.Set("cl_ord_id", args.ClientOrderID)
	}

	if args.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", args.RebaseMultiplier)
	}

	var result OpenOrders
	return &result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "OpenOrders", params, &result)
}

// GetClosedOrders returns a list of closed orders
func (e *Exchange) GetClosedOrders(ctx context.Context, args *GetClosedOrdersOptions) (*ClosedOrders, error) {
	params := url.Values{}

	if args.Trades {
		params.Set("trades", "true")
	}

	if args.UserRef != 0 {
		params.Set("userref", strconv.FormatInt(int64(args.UserRef), 10))
	}

	if args.ClientOrderID != "" {
		params.Set("cl_ord_id", args.ClientOrderID)
	}

	if args.Start != "" {
		params.Set("start", args.Start)
	}

	if args.End != "" {
		params.Set("end", args.End)
	}

	if args.Ofs > 0 {
		params.Set("ofs", strconv.FormatInt(args.Ofs, 10))
	}

	if args.CloseTime != "" {
		params.Set("closetime", args.CloseTime)
	}

	if args.ConsolidateTaker {
		params.Set("consolidate_taker", strconv.FormatBool(args.ConsolidateTaker))
	}

	if args.WithoutCount {
		params.Set("without_count", strconv.FormatBool(args.WithoutCount))
	}

	if args.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", args.RebaseMultiplier)
	}

	var result ClosedOrders
	return &result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "ClosedOrders", params, &result)
}

// QueryOrdersInfo returns order information
func (e *Exchange) QueryOrdersInfo(ctx context.Context, args OrderInfoOptions, txid string, txids ...string) (map[string]OrderInfo, error) {
	params := url.Values{
		"txid": {txid},
	}

	if txids != nil {
		params.Set("txid", txid+","+strings.Join(txids, ","))
	}

	if args.Trades {
		params.Set("trades", "true")
	}

	if args.UserRef != 0 {
		params.Set("userref", strconv.FormatInt(int64(args.UserRef), 10))
	}

	if args.ConsolidateTaker {
		params.Set("consolidate_taker", strconv.FormatBool(args.ConsolidateTaker))
	}

	if args.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", args.RebaseMultiplier)
	}

	var result map[string]OrderInfo
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "QueryOrders", params, &result)
}

// GetTradesHistory returns trade history information
func (e *Exchange) GetTradesHistory(ctx context.Context, req *GetTradesHistoryOptions) (*TradesHistory, error) {
	params := url.Values{}

	if req.Type != "" {
		params.Set("type", req.Type)
	}

	if req.Trades {
		params.Set("trades", "true")
	}

	if req.Start != "" {
		params.Set("start", req.Start)
	}

	if req.End != "" {
		params.Set("end", req.End)
	}

	if req.Ofs > 0 {
		params.Set("ofs", strconv.FormatInt(req.Ofs, 10))
	}

	if req.WithoutCount {
		params.Set("without_count", strconv.FormatBool(req.WithoutCount))
	}

	if req.ConsolidateTaker {
		params.Set("consolidate_taker", strconv.FormatBool(req.ConsolidateTaker))
	}

	if req.Ledgers {
		params.Set("ledgers", strconv.FormatBool(req.Ledgers))
	}

	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result TradesHistory
	return &result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "TradesHistory", params, &result)
}

// QueryTrades returns information on a specific trade
func (e *Exchange) QueryTrades(ctx context.Context, req *QueryTradesRequest) (map[string]TradeInfo, error) {
	if req.TransactionID == "" {
		return nil, errTransactionIDRequired
	}

	params := url.Values{
		"txid": {req.TransactionID},
	}

	if req.Trades {
		params.Set("trades", "true")
	}

	if len(req.TransactionIDs) != 0 {
		params.Set("txid", req.TransactionID+","+strings.Join(req.TransactionIDs, ","))
	}

	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result map[string]TradeInfo
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "QueryTrades", params, &result)
}

// OpenPositions returns current open positions
func (e *Exchange) OpenPositions(ctx context.Context, req *OpenPositionsRequest) (map[string]Position, error) {
	params := url.Values{}

	if len(req.TransactionIDList) != 0 {
		params.Set("txid", strings.Join(req.TransactionIDList, ","))
	}

	if req.DoCalculations {
		params.Set("docalcs", "true")
	}

	if req.Consolidation != "" {
		params.Set("consolidation", req.Consolidation)
	}

	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result map[string]Position
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "OpenPositions", params, &result)
}

// GetLedgers returns current ledgers
func (e *Exchange) GetLedgers(ctx context.Context, req *GetLedgersOptions) (*Ledgers, error) {
	params := url.Values{}

	if req.Aclass != "" {
		params.Set("aclass", req.Aclass)
	}

	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}

	if req.Type != "" {
		params.Set("type", req.Type)
	}

	if req.Start != "" {
		params.Set("start", req.Start)
	}

	if req.End != "" {
		params.Set("end", req.End)
	}

	if req.Ofs != 0 {
		params.Set("ofs", strconv.FormatInt(req.Ofs, 10))
	}

	if req.WithoutCount {
		params.Set("without_count", strconv.FormatBool(req.WithoutCount))
	}

	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result Ledgers
	return &result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Ledgers", params, &result)
}

// QueryLedgers queries an individual ledger by ID
func (e *Exchange) QueryLedgers(ctx context.Context, req *QueryLedgersRequest) (map[string]LedgerInfo, error) {
	if req.ID == "" {
		return nil, errIDRequired
	}

	params := url.Values{
		"id": {req.ID},
	}

	if len(req.IDs) != 0 {
		params.Set("id", req.ID+","+strings.Join(req.IDs, ","))
	}

	if req.Trades {
		params.Set("trades", strconv.FormatBool(req.Trades))
	}

	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result map[string]LedgerInfo
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "QueryLedgers", params, &result)
}

// GetTradeVolume returns your trade volume by currency
func (e *Exchange) GetTradeVolume(ctx context.Context, req *GetTradeVolumeRequest) (*TradeVolumeResponse, error) {
	params := url.Values{}
	formattedPairs := make([]string, len(req.Pairs))
	for x := range req.Pairs {
		symbolValue, err := e.FormatSymbol(req.Pairs[x], asset.Spot)
		if err != nil {
			return nil, err
		}
		formattedPairs[x] = symbolValue
	}
	if len(req.Pairs) != 0 {
		params.Set("pair", strings.Join(formattedPairs, ","))
	}

	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result *TradeVolumeResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "TradeVolume", params, &result)
}

// AddOrder adds a new order for Kraken exchange
func (e *Exchange) AddOrder(ctx context.Context, symbol currency.Pair, side, orderType string, volume, price, price2, leverage float64, args *AddOrderOptions) (*AddOrderResponse, error) {
	symbolValue, err := e.FormatSymbol(symbol, asset.Spot)
	if err != nil {
		return nil, err
	}

	if args == nil {
		args = &AddOrderOptions{}
	}
	params := url.Values{
		"pair":      {symbolValue},
		"type":      {strings.ToLower(side)},
		"ordertype": {strings.ToLower(orderType)},
		"volume":    {strconv.FormatFloat(volume, 'f', -1, 64)},
	}

	if args.UserRef != 0 {
		params.Set("userref", strconv.FormatInt(int64(args.UserRef), 10))
	}

	if args.ClientOrderID != "" {
		params.Set("cl_ord_id", args.ClientOrderID)
	}

	if args.AssetClass != "" {
		params.Set("asset_class", args.AssetClass)
	}

	if orderType == order.Limit.Lower() || price > 0 {
		params.Set("price", strconv.FormatFloat(price, 'f', -1, 64))
	}

	if price2 != 0 {
		params.Set("price2", strconv.FormatFloat(price2, 'f', -1, 64))
	}

	if leverage != 0 {
		params.Set("leverage", strconv.FormatFloat(leverage, 'f', -1, 64))
	}

	if args.DisplayVolume != 0 {
		params.Set("displayvol", strconv.FormatFloat(args.DisplayVolume, 'f', -1, 64))
	}

	if args.Trigger != "" {
		params.Set("trigger", args.Trigger)
	}

	if args.ReduceOnly {
		params.Set("reduce_only", strconv.FormatBool(args.ReduceOnly))
	}

	if args.SelfTradePolicy != "" {
		params.Set("stptype", args.SelfTradePolicy)
	}

	if args.OrderFlags != "" {
		params.Set("oflags", args.OrderFlags)
	}

	if args.StartTm != "" {
		params.Set("starttm", args.StartTm)
	}

	if args.ExpireTm != "" {
		params.Set("expiretm", args.ExpireTm)
	}

	if args.CloseOrderType != "" {
		params.Set("close[ordertype]", args.CloseOrderType)
	}

	if args.ClosePrice != 0 {
		params.Set("close[price]", strconv.FormatFloat(args.ClosePrice, 'f', -1, 64))
	}

	if args.ClosePrice2 != 0 {
		params.Set("close[price2]", strconv.FormatFloat(args.ClosePrice2, 'f', -1, 64))
	}

	if args.Validate {
		params.Set("validate", "true")
	}

	if args.TimeInForce != "" {
		params.Set("timeinforce", args.TimeInForce)
	}

	if args.Deadline != "" {
		params.Set("deadline", args.Deadline)
	}

	var result AddOrderResponse
	return &result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AddOrder", params, &result)
}

// CancelExistingOrder cancels order by orderID
func (e *Exchange) CancelExistingOrder(ctx context.Context, txid string, clientOrderID ...string) (*CancelOrderResponse, error) {
	values := url.Values{
		"txid": {txid},
	}
	if len(clientOrderID) > 0 && clientOrderID[0] != "" {
		values.Set("cl_ord_id", clientOrderID[0])
	}

	var result CancelOrderResponse
	return &result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CancelOrder", values, &result)
}

// SendHTTPRequest sends an unauthenticated HTTP requests
func (e *Exchange) SendHTTPRequest(ctx context.Context, ep exchange.URL, path string, result any) error {
	endpoint, err := e.API.Endpoints.GetURL(ep)
	if err != nil {
		return err
	}

	var rawMessage json.RawMessage
	item := &request.Item{
		Method:                 http.MethodGet,
		Path:                   endpoint + path,
		Result:                 &rawMessage,
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

	isSpot := ep == exchange.RestSpot
	if isSpot {
		genResponse := genericRESTResponse{
			Result: result,
		}

		if err := json.Unmarshal(rawMessage, &genResponse); err != nil {
			return err
		}

		if genResponse.Error.Warnings() != "" {
			log.Warnf(log.ExchangeSys, "%v: REST request warning: %v", e.Name, genResponse.Error.Warnings())
		}

		return genResponse.Error.Errors()
	}

	if err := getFuturesErr(rawMessage); err != nil {
		return err
	}

	return json.Unmarshal(rawMessage, result)
}

// SendAuthenticatedHTTPRequest sends an authenticated HTTP request
func (e *Exchange) SendAuthenticatedHTTPRequest(ctx context.Context, ep exchange.URL, method string, params url.Values, result any) error {
	return e.sendAuthenticatedHTTPRequest(ctx, ep, http.MethodPost, method, params, result)
}

func (e *Exchange) sendAuthenticatedHTTPRequest(ctx context.Context, ep exchange.URL, reqMethod, method string, params url.Values, result any) error {
	creds, err := e.GetCredentials(ctx)
	if err != nil {
		return err
	}
	endpoint, err := e.API.Endpoints.GetURL(ep)
	if err != nil {
		return err
	}

	if params == nil {
		params = url.Values{}
	}

	interim := json.RawMessage{}
	requestResult := any(&interim)
	_, isRawBytesResponse := result.(*[]byte)
	if isRawBytesResponse {
		requestResult = result
	}

	err = e.SendPayload(ctx, request.Unset, func() (*request.Item, error) {
		nonce := e.Requester.GetNonce(nonce.UnixNano).String()
		params.Set("nonce", nonce)
		encoded := params.Encode()

		shasum := sha256.Sum256([]byte(nonce + encoded))
		hmac, err := crypto.GetHMAC(crypto.HashSHA512, append([]byte("/0/private/"+method), shasum[:]...), []byte(creds.Secret))
		if err != nil {
			return nil, err
		}

		headers := make(map[string]string)
		headers["API-Key"] = creds.Key
		headers["API-Sign"] = base64.StdEncoding.EncodeToString(hmac)

		item := &request.Item{
			Method:                 reqMethod,
			Path:                   common.EncodeURLValues(endpoint+"/0/private/"+method, params),
			Headers:                headers,
			Result:                 requestResult,
			NonceEnabled:           true,
			Verbose:                e.Verbose,
			HTTPDebugging:          e.HTTPDebugging,
			HTTPRecording:          e.HTTPRecording,
			HTTPMockDataSliceLimit: e.HTTPMockDataSliceLimit,
		}
		if reqMethod == http.MethodPost {
			item.Path = endpoint + "/0/private/" + method
			item.Body = strings.NewReader(encoded)
		}
		return item, nil
	}, request.AuthenticatedRequest)
	if err != nil {
		return err
	}
	if isRawBytesResponse {
		return nil
	}

	genResponse := genericRESTResponse{
		Result: result,
	}

	if err := json.Unmarshal(interim, &genResponse); err != nil {
		return fmt.Errorf("%w %w", request.ErrAuthRequestFailed, err)
	}

	if err := genResponse.Error.Errors(); err != nil {
		return fmt.Errorf("%w %w", request.ErrAuthRequestFailed, err)
	}

	if genResponse.Error.Warnings() != "" {
		log.Warnf(log.ExchangeSys, "%v: AUTH REST request warning: %v", e.Name, genResponse.Error.Warnings())
	}

	return nil
}

// GetFee returns an estimate of fee based on type of transaction
func (e *Exchange) GetFee(ctx context.Context, feeBuilder *exchange.FeeBuilder) (float64, error) {
	var fee float64
	switch feeBuilder.FeeType {
	case exchange.CryptocurrencyTradeFee:
		feePair, err := e.GetTradeVolume(ctx, &GetTradeVolumeRequest{
			Pairs: []currency.Pair{feeBuilder.Pair},
		})
		if err != nil {
			return 0, err
		}
		if feeBuilder.IsMaker {
			fee = calculateTradingFee(feePair.Currency,
				feePair.FeesMaker,
				feeBuilder.PurchasePrice,
				feeBuilder.Amount)
		} else {
			fee = calculateTradingFee(feePair.Currency,
				feePair.Fees,
				feeBuilder.PurchasePrice,
				feeBuilder.Amount)
		}
	case exchange.CryptocurrencyWithdrawalFee:
		fee = getWithdrawalFee(feeBuilder.Pair.Base)
	case exchange.InternationalBankDepositFee:
		depositMethods, err := e.GetDepositMethods(ctx,
			&GetDepositMethodsRequest{
				Asset: feeBuilder.FiatCurrency.String(),
			})
		if err != nil {
			return 0, err
		}

		for _, i := range depositMethods {
			if feeBuilder.BankTransactionType == exchange.WireTransfer {
				if i.Method == "SynapsePay (US Wire)" {
					fee = i.Fee
					return fee, nil
				}
			}
		}
	case exchange.CryptocurrencyDepositFee:
		fee = getCryptocurrencyDepositFee(feeBuilder.Pair.Base)

	case exchange.InternationalBankWithdrawalFee:
		fee = getWithdrawalFee(feeBuilder.FiatCurrency)
	case exchange.OfflineTradeFee:
		fee = getOfflineTradeFee(feeBuilder.PurchasePrice, feeBuilder.Amount)
	}
	if fee < 0 {
		fee = 0
	}

	return fee, nil
}

// getOfflineTradeFee calculates the worst case-scenario trading fee
func getOfflineTradeFee(price, amount float64) float64 {
	return 0.0016 * price * amount
}

func getWithdrawalFee(c currency.Code) float64 {
	return WithdrawalFees[c]
}

func getCryptocurrencyDepositFee(c currency.Code) float64 {
	return DepositFees[c]
}

func calculateTradingFee(ccy string, feePair map[string]TradeVolumeFee, purchasePrice, amount float64) float64 {
	return (feePair[ccy].Fee / 100) * purchasePrice * amount
}

// GetCryptoDepositAddress returns a deposit address for a cryptocurrency
func (e *Exchange) GetCryptoDepositAddress(ctx context.Context, req *GetCryptoDepositAddressRequest) ([]DepositAddress, error) {
	values := url.Values{}
	values.Set("asset", req.Asset)
	values.Set("method", req.Method)

	if req.CreateNew {
		values.Set("new", "true")
	}
	if req.AssetClass != "" {
		values.Set("aclass", req.AssetClass)
	}
	if req.Amount != "" {
		values.Set("amount", req.Amount)
	}

	var result []DepositAddress
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "DepositAddresses", values, &result)
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, errNoAddressesReturned
	}
	return result, nil
}

// WithdrawStatus gets the status of recent withdrawals
func (e *Exchange) WithdrawStatus(ctx context.Context, req *WithdrawStatusRequest) ([]WithdrawStatusResponse, error) {
	params := url.Values{}
	params.Set("asset", req.Asset.String())
	if req.Method != "" {
		params.Set("method", req.Method)
	}
	if req.AssetClass != "" {
		params.Set("aclass", req.AssetClass)
	}
	if req.Start != "" {
		params.Set("start", req.Start)
	}
	if req.End != "" {
		params.Set("end", req.End)
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.FormatUint(req.Limit, 10))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result []WithdrawStatusResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawStatus", params, &result)
}

// WithdrawCancel sends a withdrawal cancellation request
func (e *Exchange) WithdrawCancel(ctx context.Context, c currency.Code, refID string) (bool, error) {
	params := url.Values{}
	params.Set("asset", c.String())
	params.Set("refid", refID)

	var result bool
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawCancel", params, &result)
}

// GetWebsocketToken returns a websocket token
func (e *Exchange) GetWebsocketToken(ctx context.Context) (string, error) {
	var response WsTokenResponse
	return response.Token, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "GetWebSocketsToken", nil, &response)
}

// GetCreditLines returns the account credit line data.
func (e *Exchange) GetCreditLines(ctx context.Context, req *GetCreditLinesRequest) (*GetCreditLinesResponse, error) {
	params := url.Values{}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result *GetCreditLinesResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CreditLines", params, &result)
}

// GetOrderAmends returns order amendment history.
func (e *Exchange) GetOrderAmends(ctx context.Context, req *GetOrderAmendsRequest) (*GetOrderAmendsResponse, error) {
	if req.OrderID == "" {
		return nil, errOrderIDRequired
	}

	params := url.Values{}
	params.Set("order_id", req.OrderID)
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result *GetOrderAmendsResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "OrderAmends", params, &result)
}

// RequestExportReport creates a new export report request.
func (e *Exchange) RequestExportReport(ctx context.Context, req *RequestExportReportRequest) (*RequestExportReportResponse, error) {
	if req.Report == "" {
		return nil, errReportRequired
	}
	if req.Format == "" {
		return nil, errFormatRequired
	}

	params := url.Values{}
	params.Set("report", req.Report)
	params.Set("format", req.Format)
	if req.Description != "" {
		params.Set("description", req.Description)
	}
	if req.Fields != "" {
		params.Set("fields", req.Fields)
	}
	if req.StartTime > 0 {
		params.Set("starttm", strconv.FormatInt(req.StartTime, 10))
	}
	if req.EndTime > 0 {
		params.Set("endtm", strconv.FormatInt(req.EndTime, 10))
	}

	var result *RequestExportReportResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AddExport", params, &result)
}

// GetExportReportStatus returns status for one or more export reports.
func (e *Exchange) GetExportReportStatus(ctx context.Context, req *GetExportReportStatusRequest) ([]ExportReportStatusResponse, error) {
	if req.Report == "" {
		return nil, errReportRequired
	}

	params := url.Values{}
	params.Set("report", req.Report)

	var result []ExportReportStatusResponse
	return result, e.sendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "ExportStatus", params, &result)
}

// RetrieveDataExport retrieves an export report file by report ID.
func (e *Exchange) RetrieveDataExport(ctx context.Context, req *RetrieveDataExportRequest) ([]byte, error) {
	if req.ID == "" {
		return nil, errIDRequired
	}

	params := url.Values{}
	params.Set("id", req.ID)

	var result []byte
	return result, e.sendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "RetrieveExport", params, &result)
}

// DeleteExportReport removes an export report by report ID.
func (e *Exchange) DeleteExportReport(ctx context.Context, req *DeleteExportReportRequest) (*DeleteExportReportResponse, error) {
	if req.ID == "" {
		return nil, errIDRequired
	}
	if req.Type == "" {
		return nil, errTypeRequired
	}

	params := url.Values{}
	params.Set("id", req.ID)
	params.Set("type", req.Type)

	var result *DeleteExportReportResponse
	return result, e.sendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "RemoveExport", params, &result)
}

// AmendOrder amends an open order.
func (e *Exchange) AmendOrder(ctx context.Context, req *AmendOrderRequest) (*AmendOrderResponse, error) {
	params := url.Values{}
	if req.TransactionID != "" {
		params.Set("txid", req.TransactionID)
	}
	if req.ClientOrderID != "" {
		params.Set("cl_ord_id", req.ClientOrderID)
	}
	if req.OrderQuantity != "" {
		params.Set("order_qty", req.OrderQuantity)
	}
	if req.DisplayQuantity != "" {
		params.Set("display_qty", req.DisplayQuantity)
	}
	if req.LimitPrice != "" {
		params.Set("limit_price", req.LimitPrice)
	}
	if req.TriggerPrice != "" {
		params.Set("trigger_price", req.TriggerPrice)
	}
	if req.Pair != "" {
		params.Set("pair", req.Pair)
	}
	if req.PostOnly {
		params.Set("post_only", strconv.FormatBool(req.PostOnly))
	}
	if req.Deadline != "" {
		params.Set("deadline", req.Deadline)
	}

	var result *AmendOrderResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AmendOrder", params, &result)
}

// CancelAllOrdersREST cancels all open orders via Kraken's spot REST endpoint.
func (e *Exchange) CancelAllOrdersREST(ctx context.Context) (*CancelOrderResponse, error) {
	var result *CancelOrderResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CancelAll", nil, &result)
}

// CancelAllOrdersAfter cancels all open orders after timeout seconds.
func (e *Exchange) CancelAllOrdersAfter(ctx context.Context, req *CancelAllOrdersAfterRequest) (*CancelAllOrdersAfterResponse, error) {
	if req.Timeout == 0 {
		return nil, errTimeoutMustBeGreaterThanZero
	}

	params := url.Values{}
	params.Set("timeout", strconv.FormatUint(req.Timeout, 10))

	var result *CancelAllOrdersAfterResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CancelAllOrdersAfter", params, &result)
}

// AddOrderBatch places multiple orders in a single request.
func (e *Exchange) AddOrderBatch(ctx context.Context, req *AddOrderBatchRequest) (*AddOrderBatchResponse, error) {
	if len(req.Orders) == 0 {
		return nil, errOrdersRequired
	}

	encodedOrders, err := json.Marshal(req.Orders)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal add order batch request: %w", err)
	}

	params := url.Values{}
	params.Set("orders", string(encodedOrders))
	if req.Pair != "" {
		params.Set("pair", req.Pair)
	}
	if req.AssetClass != "" {
		params.Set("asset_class", req.AssetClass)
	}
	if req.Deadline != "" {
		params.Set("deadline", req.Deadline)
	}
	if req.Validate {
		params.Set("validate", strconv.FormatBool(req.Validate))
	}

	var result *AddOrderBatchResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AddOrderBatch", params, &result)
}

// CancelOrderBatch cancels multiple open orders.
func (e *Exchange) CancelOrderBatch(ctx context.Context, req *CancelOrderBatchRequest) (*CancelOrderBatchResponse, error) {
	if len(req.Orders) == 0 && len(req.ClientOrder) == 0 {
		return nil, errOrdersOrClientOrdersRequired
	}

	params := url.Values{}
	if len(req.Orders) > 0 {
		encodedOrders, err := json.Marshal(req.Orders)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal cancel order batch orders: %w", err)
		}
		params.Set("orders", string(encodedOrders))
	}
	if len(req.ClientOrder) > 0 {
		encodedClientOrders, err := json.Marshal(req.ClientOrder)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal cancel order batch client orders: %w", err)
		}
		params.Set("cl_ord_ids", string(encodedClientOrders))
	}

	var result *CancelOrderBatchResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CancelOrderBatch", params, &result)
}

// EditOrder edits an open order.
func (e *Exchange) EditOrder(ctx context.Context, req *EditOrderRequest) (*EditOrderResponse, error) {
	if req.TransactionID == "" {
		return nil, errTransactionIDRequired
	}

	params := url.Values{}
	params.Set("txid", req.TransactionID)
	if req.UserReference > 0 {
		params.Set("userref", strconv.FormatInt(int64(req.UserReference), 10))
	}
	if req.Volume != "" {
		params.Set("volume", req.Volume)
	}
	if req.DisplayVolume != "" {
		params.Set("displayvol", req.DisplayVolume)
	}
	if req.Pair != "" {
		params.Set("pair", req.Pair)
	}
	if req.AssetClass != "" {
		params.Set("asset_class", req.AssetClass)
	}
	if req.Price != "" {
		params.Set("price", req.Price)
	}
	if req.SecondaryPrice != "" {
		params.Set("price2", req.SecondaryPrice)
	}
	if req.OrderFlags != "" {
		params.Set("oflags", req.OrderFlags)
	}
	if req.Deadline != "" {
		params.Set("deadline", req.Deadline)
	}
	if req.CancelResponse {
		params.Set("cancel_response", strconv.FormatBool(req.CancelResponse))
	}
	if req.Validate {
		params.Set("validate", strconv.FormatBool(req.Validate))
	}

	var result *EditOrderResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "EditOrder", params, &result)
}

// GetRecentDepositsStatus returns status of recent deposits.
func (e *Exchange) GetRecentDepositsStatus(ctx context.Context, req *GetRecentDepositsStatusRequest) (*RecentDepositsStatusResponse, error) {
	params := url.Values{}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.AssetClass != "" {
		params.Set("aclass", req.AssetClass)
	}
	if req.Method != "" {
		params.Set("method", req.Method)
	}
	if req.Start != "" {
		params.Set("start", req.Start)
	}
	if req.End != "" {
		params.Set("end", req.End)
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.FormatUint(req.Limit, 10))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result *RecentDepositsStatusResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "DepositStatus", params, &result)
}

// GetWithdrawalMethods returns available withdrawal methods for an asset.
func (e *Exchange) GetWithdrawalMethods(ctx context.Context, req *GetWithdrawalMethodsRequest) ([]WithdrawalMethodResponse, error) {
	if req.Asset == "" {
		return nil, errAssetRequired
	}

	params := url.Values{}
	params.Set("asset", req.Asset)
	if req.AssetClass != "" {
		params.Set("aclass", req.AssetClass)
	}
	if req.Network != "" {
		params.Set("network", req.Network)
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", req.RebaseMultiplier)
	}

	var result []WithdrawalMethodResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawMethods", params, &result)
}

// GetWithdrawalAddresses returns withdrawal addresses for an asset.
func (e *Exchange) GetWithdrawalAddresses(ctx context.Context, req *GetWithdrawalAddressesRequest) ([]WithdrawalAddressResponse, error) {
	if req.Asset == "" {
		return nil, errAssetRequired
	}

	params := url.Values{}
	params.Set("asset", req.Asset)
	if req.AssetClass != "" {
		params.Set("aclass", req.AssetClass)
	}
	if req.Method != "" {
		params.Set("method", req.Method)
	}
	if req.Key != "" {
		params.Set("key", req.Key)
	}
	if req.Verified != nil {
		params.Set("verified", strconv.FormatBool(*req.Verified))
	}

	var result []WithdrawalAddressResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawAddresses", params, &result)
}

// WalletTransfer transfers funds between Kraken account wallets.
func (e *Exchange) WalletTransfer(ctx context.Context, req *WalletTransferRequest) (*WalletTransferResponse, error) {
	if req.Asset == "" {
		return nil, errAssetRequired
	}
	if req.From == "" {
		return nil, errFromRequired
	}
	if req.To == "" {
		return nil, errToRequired
	}
	if req.Amount == "" {
		return nil, errAmountRequired
	}

	params := url.Values{}
	params.Set("asset", req.Asset)
	params.Set("from", req.From)
	params.Set("to", req.To)
	params.Set("amount", req.Amount)

	var result *WalletTransferResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WalletTransfer", params, &result)
}

// CreateSubaccount creates a new subaccount.
func (e *Exchange) CreateSubaccount(ctx context.Context, req *CreateSubaccountRequest) (*CreateSubaccountResponse, error) {
	if req.Username == "" {
		return nil, errUsernameRequired
	}
	if req.Email == "" {
		return nil, errEmailRequired
	}

	params := url.Values{}
	params.Set("username", req.Username)
	params.Set("email", req.Email)

	var result *CreateSubaccountResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CreateSubaccount", params, &result)
}

// AccountTransfer transfers funds between account and subaccounts.
func (e *Exchange) AccountTransfer(ctx context.Context, req *AccountTransferRequest) (*AccountTransferResponse, error) {
	if req.Asset == "" {
		return nil, errAssetRequired
	}
	if req.Amount == "" {
		return nil, errAmountRequired
	}
	if req.From == "" {
		return nil, errFromRequired
	}
	if req.To == "" {
		return nil, errToRequired
	}

	params := url.Values{}
	params.Set("asset", req.Asset)
	if req.AssetClass != "" {
		params.Set("asset_class", req.AssetClass)
	}
	params.Set("amount", req.Amount)
	params.Set("from", req.From)
	params.Set("to", req.To)

	var result *AccountTransferResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AccountTransfer", params, &result)
}

// AllocateEarnFunds allocates funds to an earn strategy.
func (e *Exchange) AllocateEarnFunds(ctx context.Context, req *AllocateEarnFundsRequest) (*AllocateEarnFundsResponse, error) {
	if req.Amount == "" {
		return nil, errAmountRequired
	}
	if req.StrategyID == "" {
		return nil, errStrategyIDRequired
	}

	params := url.Values{}
	params.Set("amount", req.Amount)
	params.Set("strategy_id", req.StrategyID)

	var result *AllocateEarnFundsResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Earn/Allocate", params, &result)
}

// DeallocateEarnFunds deallocates funds from an earn strategy.
func (e *Exchange) DeallocateEarnFunds(ctx context.Context, req *DeallocateEarnFundsRequest) (*DeallocateEarnFundsResponse, error) {
	if req.Amount == "" {
		return nil, errAmountRequired
	}
	if req.StrategyID == "" {
		return nil, errStrategyIDRequired
	}

	params := url.Values{}
	params.Set("amount", req.Amount)
	params.Set("strategy_id", req.StrategyID)

	var result *DeallocateEarnFundsResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Earn/Deallocate", params, &result)
}

// GetEarnAllocationStatus returns status of pending earn allocations.
func (e *Exchange) GetEarnAllocationStatus(ctx context.Context, req *EarnOperationStatusRequest) (*EarnOperationStatusResponse, error) {
	if req.StrategyID == "" {
		return nil, errStrategyIDRequired
	}

	params := url.Values{}
	params.Set("strategy_id", req.StrategyID)

	var result *EarnOperationStatusResponse
	return result, e.sendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "Earn/AllocateStatus", params, &result)
}

// GetEarnDeallocationStatus returns status of pending earn deallocations.
func (e *Exchange) GetEarnDeallocationStatus(ctx context.Context, req *EarnOperationStatusRequest) (*EarnOperationStatusResponse, error) {
	if req.StrategyID == "" {
		return nil, errStrategyIDRequired
	}

	params := url.Values{}
	params.Set("strategy_id", req.StrategyID)

	var result *EarnOperationStatusResponse
	return result, e.sendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "Earn/DeallocateStatus", params, &result)
}

// ListEarnStrategies returns all available earn strategies.
func (e *Exchange) ListEarnStrategies(ctx context.Context, req *ListEarnStrategiesRequest) (*ListEarnStrategiesResponse, error) {
	params := url.Values{}
	if req.Ascending != nil {
		params.Set("ascending", strconv.FormatBool(*req.Ascending))
	}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.FormatUint(req.Limit, 10))
	}
	if len(req.LockType) > 0 {
		encodedLockType, err := json.Marshal(req.LockType)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal lock types: %w", err)
		}
		params.Set("lock_type", string(encodedLockType))
	}

	var result *ListEarnStrategiesResponse
	return result, e.sendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "Earn/Strategies", params, &result)
}

// ListEarnAllocations returns all active earn allocations.
func (e *Exchange) ListEarnAllocations(ctx context.Context, req *ListEarnAllocationsRequest) (*ListEarnAllocationsResponse, error) {
	params := url.Values{}
	if req.Ascending != nil {
		params.Set("ascending", strconv.FormatBool(*req.Ascending))
	}
	if req.ConvertedAsset != "" {
		params.Set("converted_asset", req.ConvertedAsset)
	}
	if req.HideZeroAllocations != nil {
		params.Set("hide_zero_allocations", strconv.FormatBool(*req.HideZeroAllocations))
	}

	var result *ListEarnAllocationsResponse
	return result, e.sendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "Earn/Allocations", params, &result)
}

// GetPreTradeData returns transparency metadata for pre-trade market data.
func (e *Exchange) GetPreTradeData(ctx context.Context, req *GetPreTradeDataRequest) (*GetPreTradeDataResponse, error) {
	if req.Symbol == "" {
		return nil, errSymbolRequired
	}

	params := url.Values{}
	params.Set("symbol", req.Symbol)

	var result *GetPreTradeDataResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/PreTrade", params), &result)
}

// GetPostTradeData returns transparency entries for post-trade records.
func (e *Exchange) GetPostTradeData(ctx context.Context, req *GetPostTradeDataRequest) (*GetPostTradeDataResponse, error) {
	params := url.Values{}
	if req.Symbol != "" {
		params.Set("symbol", req.Symbol)
	}
	if !req.FromTimestamp.IsZero() {
		params.Set("from_ts", req.FromTimestamp.UTC().Format(time.RFC3339))
	}
	if !req.ToTimestamp.IsZero() {
		params.Set("to_ts", req.ToTimestamp.UTC().Format(time.RFC3339))
	}
	if req.Count > 0 {
		params.Set("count", strconv.FormatUint(req.Count, 10))
	}

	var result *GetPostTradeDataResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/PostTrade", params), &result)
}

// LookupAltName converts a currency into its altName (ZUSD -> USD)
func (a *assetTranslatorStore) LookupAltName(target string) string {
	a.l.RLock()
	alt, ok := a.Assets[target]
	if !ok {
		a.l.RUnlock()
		return ""
	}
	a.l.RUnlock()
	return alt
}

// LookupCurrency converts an altName to its original type (USD -> ZUSD)
func (a *assetTranslatorStore) LookupCurrency(target string) string {
	a.l.RLock()
	for k, v := range a.Assets {
		if v == target {
			a.l.RUnlock()
			return k
		}
	}
	a.l.RUnlock()
	return ""
}

// Seed seeds a currency translation pair
func (a *assetTranslatorStore) Seed(orig, alt string) {
	a.l.Lock()
	if a.Assets == nil {
		a.Assets = make(map[string]string)
	}

	if _, ok := a.Assets[orig]; ok {
		a.l.Unlock()
		return
	}

	a.Assets[orig] = alt
	a.l.Unlock()
}

// Seeded checks if assets have been seeded
func (a *assetTranslatorStore) Seeded() bool {
	a.l.RLock()
	isSeeded := len(a.Assets) > 0
	a.l.RUnlock()
	return isSeeded
}
