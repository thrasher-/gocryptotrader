package kraken

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/convert"
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
	krakenSpotVersion             = "0"
	krakenFuturesVersion          = "3"
)

// Kraken is the overarching type across the kraken package
type Kraken struct {
	exchange.Base
}

// GetCurrentServerTime returns current server time
func (k *Kraken) GetCurrentServerTime(ctx context.Context) (*TimeResponse, error) {
	const method = "Time"
	path := "/" + krakenAPIVersion + "/public/" + method

	var result TimeResponse
	if err := k.SendHTTPRequest(ctx, exchange.RestSpot, path, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// RequestExportReport requests a new trades or ledgers data export.
func (k *Kraken) RequestExportReport(ctx context.Context, opts RequestExportReportOptions) (*RequestExportReportResponse, error) {
	const methodSpecificPath = "AddExport"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath

	if opts.Report == "" || opts.Description == "" {
		return nil, errors.New("report type and description are required parameters")
	}
	params := url.Values{}
	params.Set("report", string(opts.Report))
	params.Set("descr", opts.Description) // API calls it description, struct has Description
	if opts.Format != "" {
		params.Set("format", string(opts.Format))
	}
	if opts.Fields != "" {
		params.Set("fields", opts.Fields)
	}
	if opts.StartTm != 0 {
		params.Set("starttm", strconv.FormatInt(opts.StartTm, 10))
	}
	if opts.EndTm != 0 {
		params.Set("endtm", strconv.FormatInt(opts.EndTm, 10))
	}
	if opts.Asset != "" {
		 params.Set("asset", opts.Asset)
	}

	var result RequestExportReportResponse
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SeedAssets seeds Kraken's asset list and stores it in the
// asset translator
func (k *Kraken) SeedAssets(ctx context.Context) error {
	assets, err := k.GetAssets(ctx)
	if err != nil {
		return err
	}
	for orig, val := range assets {
		assetTranslator.Seed(orig, val.Altname)
	}

	assetPairs, err := k.GetAssetPairs(ctx, []string{}, "")
	if err != nil {
		return err
	}
	for k, v := range assetPairs {
		assetTranslator.Seed(k, v.Altname)
	}
	return nil
}

// GetAssets returns a full asset list
func (k *Kraken) GetAssets(ctx context.Context) (map[string]*Asset, error) {
	const method = "Assets"
	path := "/" + krakenAPIVersion + "/public/" + method
	var result map[string]*Asset
	if err := k.SendHTTPRequest(ctx, exchange.RestSpot, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetAccountBalance retrieves all cash balances, net of pending withdrawals.
// It calls the /private/Balance endpoint.
func (k *Kraken) GetAccountBalance(ctx context.Context) (map[string]string, error) {
	const methodSpecificPath = "Balance"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result map[string]string
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, url.Values{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetAssetPairs returns a full asset pair list
// Parameter 'info' only supports 4 strings: "fees", "leverage", "margin", "info" <- (default)
func (k *Kraken) GetAssetPairs(ctx context.Context, assetPairs []string, info string) (map[string]*AssetPairs, error) {
	const method = "AssetPairs"
	basePath := "/" + krakenAPIVersion + "/public/" + method
	params := url.Values{}
	if len(assetPairs) != 0 {
		assets := strings.Join(assetPairs, ",")
		params.Set("pair", assets)
	}

	if info != "" {
		if info != "margin" && info != "leverage" && info != "fees" && info != "info" {
			return nil, errors.New("parameter info can only be 'asset', 'margin', 'fees' or 'leverage'")
		}
		params.Set("info", info)
	}

	requestPath := basePath
	if len(params) > 0 {
		requestPath = basePath + "?" + params.Encode()
	}

	var result map[string]*AssetPairs
	if err := k.SendHTTPRequest(ctx, exchange.RestSpot, requestPath, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetTicker returns ticker information from kraken
func (k *Kraken) GetTicker(ctx context.Context, symbol currency.Pair) (*Ticker, error) {
	const method = "Ticker"
	values := url.Values{}
	symbolValue, err := k.FormatSymbol(symbol, asset.Spot)
	if err != nil {
		return nil, err
	}
	values.Set("pair", symbolValue)

	var data map[string]*TickerResponse
	path := "/" + krakenAPIVersion + "/public/" + method + "?" + values.Encode()
	if err := k.SendHTTPRequest(ctx, exchange.RestSpot, path, &data); err != nil {
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
	tick.Open = v.Open[0].Float64()
	}
	return &tick, nil
}

// GetTickers supports fetching multiple tickers from Kraken
// pairList must be in the format pairs separated by commas
// ("LTCUSD,ETCUSD")
func (k *Kraken) GetTickers(ctx context.Context, pairList string) (map[string]Ticker, error) {
	const method = "Ticker"
	basePath := "/" + krakenAPIVersion + "/public/" + method
	params := url.Values{}
	if pairList != "" {
		params.Set("pair", pairList)
	}

	requestPath := basePath
	if len(params) > 0 {
		requestPath = basePath + "?" + params.Encode()
	}

	var result map[string]*TickerResponse
	err := k.SendHTTPRequest(ctx, exchange.RestSpot, requestPath, &result)
	if err != nil {
		return nil, err
	}

	tickers := make(map[string]Ticker, len(result))
	for pairName, v := range result {
		tickers[pairName] = Ticker{
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
			Open:                       v.Open[0].Float64(), // Adjusted
		}
	}
	return tickers, nil
}

// GetOHLC returns an array of open high low close values of a currency pair
func (k *Kraken) GetOHLC(ctx context.Context, symbol currency.Pair, interval string) ([]OpenHighLowClose, error) {
	const method = "OHLC"
	values := url.Values{}
	symbolValue, err := k.FormatSymbol(symbol, asset.Spot)
	if err != nil {
		return nil, err
	}
	translatedAsset := assetTranslator.LookupCurrency(symbolValue)
	if translatedAsset == "" {
		translatedAsset = symbolValue
	}
	values.Set("pair", translatedAsset)
	values.Set("interval", interval)

	path := "/" + krakenAPIVersion + "/public/" + method + "?" + values.Encode()

	result := make(map[string]any)
	err = k.SendHTTPRequest(ctx, exchange.RestSpot, path, &result)
	if err != nil {
		return nil, err
	}

	ohlcData, ok := result[translatedAsset].([]any)
	if !ok {
		return nil, errors.New("invalid data returned")
	}

	OHLC := make([]OpenHighLowClose, len(ohlcData))
	for x := range ohlcData {
		subData, ok := ohlcData[x].([]any)
		if !ok {
			return nil, errors.New("unable to type assert subData")
		}

		if len(subData) < 8 {
			return nil, errors.New("unexpected data length returned")
		}

		var o OpenHighLowClose

		tmData, ok := subData[0].(float64)
		if !ok {
			return nil, errors.New("unable to type assert time")
		}
		o.Time = time.Unix(int64(tmData), 0)
		if o.Open, err = convert.FloatFromString(subData[1]); err != nil {
			return nil, err
		}
		if o.High, err = convert.FloatFromString(subData[2]); err != nil {
			return nil, err
		}
		if o.Low, err = convert.FloatFromString(subData[3]); err != nil {
			return nil, err
		}
		if o.Close, err = convert.FloatFromString(subData[4]); err != nil {
			return nil, err
		}
		if o.VolumeWeightedAveragePrice, err = convert.FloatFromString(subData[5]); err != nil {
			return nil, err
		}
		if o.Volume, err = convert.FloatFromString(subData[6]); err != nil {
			return nil, err
		}
		countFloat, ok := subData[7].(float64)
		if !ok {
			return nil, fmt.Errorf("unable to type assert OHLC count data (index 7) to float64: got %T", subData[7])
		}
		o.Count = int64(countFloat)
		OHLC[x] = o
	}
	return OHLC, nil
}

// GetDepth returns the orderbook for a particular currency
func (k *Kraken) GetDepth(ctx context.Context, symbol currency.Pair) (*Orderbook, error) {
	const method = "Depth"
	symbolValue, err := k.FormatSymbol(symbol, asset.Spot)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("pair", symbolValue)
	path := "/" + krakenAPIVersion + "/public/" + method + "?" + values.Encode()

	type orderbookStructure struct {
		Bids [][3]types.Number `json:"bids"`
		Asks [][3]types.Number `json:"asks"`
	}

	result := make(map[string]*orderbookStructure)
	if err := k.SendHTTPRequest(ctx, exchange.RestSpot, path, &result); err != nil {
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

// GetTrades returns current trades on Kraken
func (k *Kraken) GetTrades(ctx context.Context, symbol currency.Pair) ([]RecentTrades, error) {
	const method = "Trades"
	values := url.Values{}
	symbolValue, err := k.FormatSymbol(symbol, asset.Spot)
	if err != nil {
		return nil, err
	}
	translatedAsset := assetTranslator.LookupCurrency(symbolValue)
	values.Set("pair", translatedAsset)

	path := "/" + krakenAPIVersion + "/public/" + method + "?" + values.Encode()

	data := make(map[string]any)
	err = k.SendHTTPRequest(ctx, exchange.RestSpot, path, &data)
	if err != nil {
		return nil, err
	}

	trades, ok := data[translatedAsset].([]any)
	if !ok {
		return nil, fmt.Errorf("no data returned for symbol %v", symbol)
	}

	var individualTrade []any
	recentTrades := make([]RecentTrades, len(trades))
	for x := range trades {
		individualTrade, ok = trades[x].([]any)
		if !ok {
			return nil, errors.New("unable to parse individual trade data")
		}
		if len(individualTrade) != 7 {
			return nil, errors.New("unrecognised trade data received")
		}
		var r RecentTrades

		price, ok := individualTrade[0].(string)
		if !ok {
			return nil, common.GetTypeAssertError("string", individualTrade[0], "price")
		}
		r.Price, err = strconv.ParseFloat(price, 64)
		if err != nil {
			return nil, err
		}

		volume, ok := individualTrade[1].(string)
		if !ok {
			return nil, common.GetTypeAssertError("string", individualTrade[1], "volume")
		}
		r.Volume, err = strconv.ParseFloat(volume, 64)
		if err != nil {
			return nil, err
		}
		r.Time, ok = individualTrade[2].(float64)
		if !ok {
			return nil, common.GetTypeAssertError("float64", individualTrade[2], "time")
		}
		r.BuyOrSell, ok = individualTrade[3].(string)
		if !ok {
			return nil, common.GetTypeAssertError("string", individualTrade[3], "buyOrSell")
		}
		r.MarketOrLimit, ok = individualTrade[4].(string)
		if !ok {
			return nil, common.GetTypeAssertError("string", individualTrade[4], "marketOrLimit")
		}
		r.Miscellaneous, ok = individualTrade[5].(string)
		if !ok {
			return nil, common.GetTypeAssertError("string", individualTrade[5], "miscellaneous")
		}
		tradeID, ok := individualTrade[6].(float64)
		if !ok {
			return nil, common.GetTypeAssertError("float64", individualTrade[6], "tradeID")
		}
		r.TradeID = int64(tradeID)
		recentTrades[x] = r
	}
	return recentTrades, nil
}

// GetSpread returns the full spread on Kraken
func (k *Kraken) GetSpread(ctx context.Context, symbol currency.Pair) ([]Spread, error) {
	const method = "Spread"
	values := url.Values{}
	symbolValue, err := k.FormatSymbol(symbol, asset.Spot)
	if err != nil {
		return nil, err
	}
	values.Set("pair", symbolValue)

	result := make(map[string]any)
	path := "/" + krakenAPIVersion + "/public/" + method + "?" + values.Encode()
	err = k.SendHTTPRequest(ctx, exchange.RestSpot, path, &result)
	if err != nil {
		return nil, err
	}

	data, ok := result[symbolValue]
	if !ok {
		return nil, fmt.Errorf("unable to find %s in spread data", symbolValue)
	}

	spreadData, ok := data.([]any)
	if !ok {
		return nil, errors.New("unable to type assert spreadData")
	}

	peanutButter := make([]Spread, len(spreadData))
	for x := range spreadData {
		subData, ok := spreadData[x].([]any)
		if !ok {
			return nil, errors.New("unable to type assert subData")
		}

		if len(subData) < 3 {
			return nil, errors.New("unexpected data length")
		}

		var s Spread
		timeData, ok := subData[0].(float64)
		if !ok {
			return nil, common.GetTypeAssertError("float64", subData[0], "timeData")
		}
		s.Time = time.Unix(int64(timeData), 0)

		if s.Bid, err = convert.FloatFromString(subData[1]); err != nil {
			return nil, err
		}
		if s.Ask, err = convert.FloatFromString(subData[2]); err != nil {
			return nil, err
		}
		peanutButter[x] = s
	}
	return peanutButter, nil
}

// GetExtendedBalance returns your balance associated with your keys (formerly GetBalance)
func (k *Kraken) GetExtendedBalance(ctx context.Context) (map[string]Balance, error) {
	const methodSpecificPath = "BalanceEx"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath

	var result map[string]Balance
	// The 'method' argument to SendAuthenticatedHTTPRequest is now the full partial path
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, url.Values{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetWithdrawInfo gets withdrawal fees
func (k *Kraken) GetWithdrawInfo(ctx context.Context, currency string, amount float64) (*WithdrawInformation, error) {
	params := url.Values{}
	params.Set("asset", currency)
	params.Set("key", "")
	params.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))

	const methodSpecificPath = "WithdrawInfo"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result WithdrawInformation
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Withdraw withdraws funds
func (k *Kraken) Withdraw(ctx context.Context, asset, key string, amount float64) (string, error) {
	params := url.Values{}
	params.Set("asset", asset)
	params.Set("key", key)
	params.Set("amount", fmt.Sprintf("%f", amount))

	const methodSpecificPath = "Withdraw"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var referenceID string
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &referenceID); err != nil {
		return referenceID, err
	}

	return referenceID, nil
}

// GetSystemStatus returns the current system status or trading mode.
func (k *Kraken) GetSystemStatus(ctx context.Context) (*SystemStatusResponse, error) {
	const method = "SystemStatus"
	path := "/" + krakenAPIVersion + "/public/" + method
	var result SystemStatusResponse
	if err := k.SendHTTPRequest(ctx, exchange.RestSpot, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDepositMethods gets withdrawal fees
func (k *Kraken) GetDepositMethods(ctx context.Context, currency string) ([]DepositMethods, error) {
	params := url.Values{}
	params.Set("asset", currency)

	const methodSpecificPath = "DepositMethods"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result []DepositMethods
	err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetTradeBalance returns full information about your trades on Kraken
func (k *Kraken) GetTradeBalance(ctx context.Context, args ...TradeBalanceOptions) (*TradeBalanceInfo, error) {
	params := url.Values{}
	if len(args) > 0 { // Check if args are provided
		if args[0].Asset != "" {
			params.Set("asset", args[0].Asset)
		}
	}

	const methodSpecificPath = "TradeBalance"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result TradeBalanceInfo
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetOpenOrders returns all current open orders
func (k *Kraken) GetOpenOrders(ctx context.Context, args OrderInfoOptions) (*OpenOrders, error) {
	params := url.Values{}

	if args.Trades {
		params.Set("trades", "true")
	}

	if args.UserRef != 0 {
		params.Set("userref", strconv.FormatInt(int64(args.UserRef), 10))
	}

	const methodSpecificPath = "OpenOrders"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result OpenOrders
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetClosedOrders returns a list of closed orders
func (k *Kraken) GetClosedOrders(ctx context.Context, args GetClosedOrdersOptions) (*ClosedOrders, error) {
	params := url.Values{}

	if args.Trades {
		params.Set("trades", "true")
	}

	if args.UserRef != 0 {
		params.Set("userref", strconv.FormatInt(int64(args.UserRef), 10))
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

	const methodSpecificPath = "ClosedOrders"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result ClosedOrders
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QueryOrdersInfo returns order information
func (k *Kraken) QueryOrdersInfo(ctx context.Context, args OrderInfoOptions, txid string, txids ...string) (map[string]OrderInfo, error) {
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

	const methodSpecificPath = "QueryOrders"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result map[string]OrderInfo
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err // Corrected this line
	}

	return result, nil
}

// GetTradesHistory returns trade history information
func (k *Kraken) GetTradesHistory(ctx context.Context, args ...GetTradesHistoryOptions) (*TradesHistory, error) {
	params := url.Values{}
	if len(args) > 0 {
		options := args[0] // Use a local variable for clarity
		if options.Type != "" {
			params.Set("type", options.Type)
		}
		if options.Trades { // API default is false, so only send if true
			params.Set("trades", "true")
		}
		if options.Start != "" {
			params.Set("start", options.Start)
		}
		if options.End != "" {
			params.Set("end", options.End)
		}
		if options.Ofs > 0 {
			params.Set("ofs", strconv.FormatInt(options.Ofs, 10))
		}
		if options.ConsolidateTaker != nil { // If pointer is not nil, user explicitly set it
			params.Set("consolidate_taker", strconv.FormatBool(*options.ConsolidateTaker))
		}
	}

	const methodSpecificPath = "TradesHistory"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result TradesHistory
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QueryTrades returns information on a specific trade
func (k *Kraken) QueryTrades(ctx context.Context, trades bool, txid string, txids ...string) (map[string]TradeInfo, error) {
	params := url.Values{
		"txid": {txid},
	}

	if trades {
		params.Set("trades", "true")
	}

	if txids != nil {
		params.Set("txid", txid+","+strings.Join(txids, ","))
	}

	const methodSpecificPath = "QueryTrades"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result map[string]TradeInfo
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// OpenPositions returns current open positions
func (k *Kraken) OpenPositions(ctx context.Context, txids []string, docalcs bool, consolidation string) (map[string]Position, error) {
	const methodSpecificPath = "OpenPositions"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath

	params := url.Values{}
	if len(txids) > 0 {
		params.Set("txid", strings.Join(txids, ","))
	}
	if docalcs {
		params.Set("docalcs", "true")
	}
	if consolidation != "" { // Assuming "market" is the only valid non-empty value as per doc
		params.Set("consolidation", consolidation)
	}

	var result map[string]Position
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetLedgers returns current ledgers
func (k *Kraken) GetLedgers(ctx context.Context, args ...GetLedgersOptions) (*Ledgers, error) {
	params := url.Values{}

	if len(args) > 0 {
		options := args[0]
		if options.Aclass != "" { // Corrected condition
			params.Set("aclass", options.Aclass)
		}
		if options.Asset != "" { // Corrected condition (still single asset, API allows comma-delimited)
			params.Set("asset", options.Asset)
		}
		if options.Type != "" { // Corrected condition
			params.Set("type", options.Type)
		}
		if options.Start != "" { // Corrected condition
			params.Set("start", options.Start)
		}
		if options.End != "" { // Corrected condition
			params.Set("end", options.End)
		}
		if options.Ofs != 0 {
			params.Set("ofs", strconv.FormatInt(options.Ofs, 10))
		}
	}

	const methodSpecificPath = "Ledgers"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result Ledgers
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QueryLedgers queries an individual ledger by ID
func (k *Kraken) QueryLedgers(ctx context.Context, id string, ids ...string) (map[string]LedgerInfo, error) {
	params := url.Values{
		"id": {id},
	}

	if ids != nil {
		params.Set("id", id+","+strings.Join(ids, ","))
	}

	const methodSpecificPath = "QueryLedgers"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result map[string]LedgerInfo
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetTradeVolume returns your trade volume by currency
func (k *Kraken) GetTradeVolume(ctx context.Context, feeinfo bool, symbol ...currency.Pair) (*TradeVolumeResponse, error) {
	params := url.Values{}
	formattedPairs := make([]string, len(symbol))
	for x := range symbol {
		symbolValue, err := k.FormatSymbol(symbol[x], asset.Spot)
		if err != nil {
			return nil, err
		}
		formattedPairs[x] = symbolValue
	}
	if symbol != nil {
		params.Set("pair", strings.Join(formattedPairs, ","))
	}

	if feeinfo {
		params.Set("fee-info", "true")
	}

	const methodSpecificPath = "TradeVolume"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result *TradeVolumeResponse
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// AddOrder adds a new order for Kraken exchange
func (k *Kraken) AddOrder(ctx context.Context, symbol currency.Pair, side, orderType string, volume, price, price2, leverage float64, args *AddOrderOptions) (*AddOrderResponse, error) {
	symbolValue, err := k.FormatSymbol(symbol, asset.Spot)
	if err != nil {
		return nil, err
	}
	params := url.Values{
		"pair":      {symbolValue},
		"type":      {strings.ToLower(side)},
		"ordertype": {strings.ToLower(orderType)},
		"volume":    {strconv.FormatFloat(volume, 'f', -1, 64)},
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
		params.Set("close[ordertype]", args.ExpireTm)
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

	const methodSpecificPath = "AddOrder"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result AddOrderResponse
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CancelExistingOrder cancels order by orderID
func (k *Kraken) CancelExistingOrder(ctx context.Context, txid string) (*CancelOrderResponse, error) {
	values := url.Values{
		"txid": {txid},
	}

	const methodSpecificPath = "CancelOrder"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result CancelOrderResponse
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, values, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SendHTTPRequest sends an unauthenticated HTTP requests
func (k *Kraken) SendHTTPRequest(ctx context.Context, ep exchange.URL, path string, result any) error {
	endpoint, err := k.API.Endpoints.GetURL(ep)
	if err != nil {
		return err
	}

	var rawMessage json.RawMessage
	item := &request.Item{
		Method:        http.MethodGet,
		Path:          endpoint + path,
		Result:        &rawMessage,
		Verbose:       k.Verbose,
		HTTPDebugging: k.HTTPDebugging,
		HTTPRecording: k.HTTPRecording,
	}

	err = k.SendPayload(ctx, request.Unset, func() (*request.Item, error) {
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
			log.Warnf(log.ExchangeSys, "%v: REST request warning: %v", k.Name, genResponse.Error.Warnings())
		}

		return genResponse.Error.Errors()
	}

	if err := getFuturesErr(rawMessage); err != nil {
		return err
	}

	return json.Unmarshal(rawMessage, result)
}

// SendAuthenticatedHTTPRequest sends an authenticated HTTP request
func (k *Kraken) SendAuthenticatedHTTPRequest(ctx context.Context, ep exchange.URL, requestPathPart string, params url.Values, result any) error { // method changed to requestPathPart
	creds, err := k.GetCredentials(ctx)
	if err != nil {
		return err
	}
	endpoint, err := k.API.Endpoints.GetURL(ep)
	if err != nil {
		return err
	}
	// path := fmt.Sprintf("/%s/private/%s", krakenAPIVersion, method) // REMOVE THIS LINE

	interim := json.RawMessage{}
	err = k.SendPayload(ctx, request.Unset, func() (*request.Item, error) {
		nonce := k.Requester.GetNonce(nonce.UnixNano).String()
		params.Set("nonce", nonce)
		encoded := params.Encode()
		var shasum []byte
		shasum, err = crypto.GetSHA256([]byte(nonce + encoded))
		if err != nil {
			return nil, err
		}

		var hmac []byte
		// Use requestPathPart here
		hmac, err = crypto.GetHMAC(crypto.HashSHA512,
			append([]byte(requestPathPart), shasum...),
			[]byte(creds.Secret))
		if err != nil {
			return nil, err
		}

		signature := crypto.Base64Encode(hmac)

		headers := make(map[string]string)
		headers["API-Key"] = creds.Key
		headers["API-Sign"] = signature

		return &request.Item{
			Method:        http.MethodPost,
			Path:          endpoint + requestPathPart, // Use requestPathPart here
			Headers:       headers,
			Body:          strings.NewReader(encoded),
			Result:        &interim,
			NonceEnabled:  true,
			Verbose:       k.Verbose,
			HTTPDebugging: k.HTTPDebugging,
			HTTPRecording: k.HTTPRecording,
		}, nil
	}, request.AuthenticatedRequest)
	if err != nil {
		return err
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
		log.Warnf(log.ExchangeSys, "%v: AUTH REST request warning: %v", k.Name, genResponse.Error.Warnings())
	}

	return nil
}

// GetOrderAmends retrieves an audit trail of amend transactions on the specified order.
func (k *Kraken) GetOrderAmends(ctx context.Context, opts GetOrderAmendsOptions) (*GetOrderAmendsResponse, error) {
	const methodSpecificPath = "OrderAmends"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath

	if opts.OrderID == "" {
		return nil, errors.New("order_id is a required parameter")
	}

	params := url.Values{}
	params.Set("order_id", opts.OrderID)

	if opts.UserRef != 0 {
		params.Set("userref", strconv.FormatInt(int64(opts.UserRef), 10))
	}

	// The 'trades' parameter is not clearly documented for this endpoint's request,
	// unlike QueryOrders. So, it's omitted here.

	var result GetOrderAmendsResponse
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetExportReportStatus retrieves status of previously requested reports.
func (k *Kraken) GetExportReportStatus(ctx context.Context, opts ExportStatusOptions) ([]ExportReportInfo, error) {
	const methodSpecificPath = "ExportStatus"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	if opts.Report == "" {
		return nil, errors.New("report type is a required parameter")
	}
	params := url.Values{}
	params.Set("report", string(opts.Report))

	var result []ExportReportInfo // API returns an array directly
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RetrieveExportReport retrieves a processed data export.
// Note: This returns raw []byte as the report data can be CSV/TSV.
func (k *Kraken) RetrieveExportReport(ctx context.Context, reportID string) ([]byte, error) {
	const methodSpecificPath = "RetrieveExport"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	if reportID == "" {
		return nil, errors.New("reportID is a required parameter")
	}
	params := url.Values{}
	params.Set("id", reportID)

	rawData, err := k.sendAuthenticatedHTTPRequestRaw(ctx, exchange.RestSpot, requestPath, params)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve export report raw data: %w", err)
	}
	return rawData, nil
}

// DeleteExportReport deletes or cancels a data export report.
func (k *Kraken) DeleteExportReport(ctx context.Context, opts DeleteExportOptions) (*DeleteExportResponse, error) {
	const methodSpecificPath = "RemoveExport" // API endpoint is RemoveExport
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath

	if opts.ID == "" || opts.Type == "" {
		return nil, errors.New("report ID and type (delete/cancel) are required parameters")
	}
	params := url.Values{}
	params.Set("id", opts.ID)
	params.Set("type", string(opts.Type))

	var result DeleteExportResponse
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// sendAuthenticatedHTTPRequestRaw sends an authenticated HTTP request and returns the raw response body.
func (k *Kraken) sendAuthenticatedHTTPRequestRaw(ctx context.Context, ep exchange.URL, requestPathPart string, params url.Values) ([]byte, error) {
	creds, err := k.GetCredentials(ctx)
	if err != nil {
		return nil, err
	}
	endpoint, err := k.API.Endpoints.GetURL(ep)
	if err != nil {
		return nil, err
	}

	var rawResponse []byte // To store the raw response

	// Note: SendPayload might need internal adjustments to handle rawResponse if item.Result is *[]byte
	// and the response Content-Type is not application/json.
	// For Kraken, error responses are JSON, but success for RetrieveExport is raw.
	// This function assumes SendPayload can populate rawResponse if no JSON unmarshal error occurs,
	// or that SendPayload is enhanced to check for a special flag or type for rawResponse.
	err = k.SendPayload(ctx, request.Unset, func() (*request.Item, error) {
		nonce := k.Requester.GetNonce(nonce.UnixNano).String()

		// Create a temporary URL values for signature generation, excluding nonce initially
		postDataForSig := url.Values{}
		if params != nil {
			for key, values := range params {
				for _, value := range values {
					postDataForSig.Add(key, value)
				}
			}
		}
		// Nonce is part of the POST data for Kraken
		postDataForSig.Set("nonce", nonce)
		encodedPostDataForSig := postDataForSig.Encode()

		var shasum []byte
		// Signature is path + sha256(nonce + POST data)
		// Kraken's documentation implies the 'nonce' value itself is prepended to the already encoded POST data string for the SHA256 hash.
		// However, standard practice and other parts of this driver (SendAuthenticatedHTTPRequest)
		// add nonce to url.Values and then encode. Let's stick to what SendAuthenticatedHTTPRequest implies:
		// The nonce is part of the encoded POST data string.
		// So, shasum is over (nonce_value + encoded_post_data_without_nonce)
		// OR shasum is over (encoded_post_data_with_nonce), and path + that.
		// The existing SendAuthenticatedHTTPRequest does: sha256([]byte(nonce + encoded)) where encoded does NOT have nonce yet.
		// Let's replicate that for consistency.

		// Data for SHA256 hash: nonce + (form-encoded POST data without nonce)
		originalPostDataWithoutNonce := ""
		if params != nil {
			 originalPostDataWithoutNonce = params.Encode() // params should not have nonce at this stage
		}
		stringToHash := nonce + originalPostDataWithoutNonce

		shasum, err = crypto.GetSHA256([]byte(stringToHash))
		if err != nil {
			return nil, fmt.Errorf("error generating SHA256 hash: %w", err)
		}

		var hmac []byte
		hmac, err = crypto.GetHMAC(crypto.HashSHA512,
			append([]byte(requestPathPart), shasum...),
			[]byte(creds.Secret))
		if err != nil {
			return nil, fmt.Errorf("error generating HMAC: %w", err)
		}
		signature := crypto.Base64Encode(hmac)

		headers := make(map[string]string)
		headers["API-Key"] = creds.Key
		headers["API-Sign"] = signature

		// The actual POST body sent to the server must include the nonce.
		// Use the `postDataForSig` which already includes the nonce.
		finalEncodedBody := encodedPostDataForSig

		return &request.Item{
			Method:        http.MethodPost,
			Path:          endpoint + requestPathPart,
			Headers:       headers,
			Body:          strings.NewReader(finalEncodedBody),
			Result:        &rawResponse, // Point to our byte slice
			NonceEnabled:  true, // Kraken uses nonce
			Verbose:       k.Verbose,
			HTTPDebugging: k.HTTPDebugging,
			HTTPRecording: k.HTTPRecording,
			// IsRaw:         true, // Hypothetical field, not in current request.Item
								 // If SendPayload uses Result type, *[]byte might signal raw.
		}, nil
	}, request.AuthenticatedRequest)

	if err != nil {
		// This error could be a connection error, an HTTP status error,
		// or a JSON unmarshalling error if SendPayload tries to parse a Kraken JSON error.
		// If Kraken returns JSON error for this raw endpoint, SendAuthenticatedHTTPRequest might be better,
		// but then we can't get raw bytes on success. This is the dilemma.
		return nil, fmt.Errorf("SendPayload failed: %w", err)
	}
	return rawResponse, nil
}

// GetFee returns an estimate of fee based on type of transaction
func (k *Kraken) GetFee(ctx context.Context, feeBuilder *exchange.FeeBuilder) (float64, error) {
	var fee float64
	switch feeBuilder.FeeType {
	case exchange.CryptocurrencyTradeFee:
		feePair, err := k.GetTradeVolume(ctx, true, feeBuilder.Pair)
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
		depositMethods, err := k.GetDepositMethods(ctx,
			feeBuilder.FiatCurrency.String())
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

func calculateTradingFee(currency string, feePair map[string]TradeVolumeFee, purchasePrice, amount float64) float64 {
	return (feePair[currency].Fee / 100) * purchasePrice * amount
}

// GetCryptoDepositAddress returns a deposit address for a cryptocurrency
func (k *Kraken) GetCryptoDepositAddress(ctx context.Context, method, code string, createNew bool) ([]DepositAddress, error) {
	values := url.Values{}
	values.Set("asset", code)
	values.Set("method", method)

	if createNew {
		values.Set("new", "true")
	}

	const methodSpecificPath = "DepositAddresses"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result []DepositAddress
	err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, values, &result)
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, errors.New("no addresses returned")
	}
	return result, nil
}

// WithdrawStatus gets the status of recent withdrawals
func (k *Kraken) WithdrawStatus(ctx context.Context, c currency.Code, method string) ([]WithdrawStatusResponse, error) {
	params := url.Values{}
	params.Set("asset", c.String())
	if method != "" {
		params.Set("method", method)
	}

	const methodSpecificPath = "WithdrawStatus"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result []WithdrawStatusResponse
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// WithdrawCancel sends a withdrawal cancellation request
func (k *Kraken) WithdrawCancel(ctx context.Context, c currency.Code, refID string) (bool, error) {
	params := url.Values{}
	params.Set("asset", c.String())
	params.Set("refid", refID)

	const methodSpecificPath = "WithdrawCancel"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var result bool
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, params, &result); err != nil {
		return result, err
	}

	return result, nil
}

// GetWebsocketToken returns a websocket token
func (k *Kraken) GetWebsocketToken(ctx context.Context) (string, error) {
	const methodSpecificPath = "GetWebSocketsToken"
	requestPath := "/" + krakenAPIVersion + "/private/" + methodSpecificPath
	var response WsTokenResponse
	if err := k.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, requestPath, url.Values{}, &response); err != nil {
		return "", err
	}
	return response.Token, nil
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
