package kraken

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

// GetCurrentServerTime returns current server time.
func (e *Exchange) GetCurrentServerTime(ctx context.Context) (*TimeResponse, error) {
	var result *TimeResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, "/0/public/Time", &result)
}

// GetSystemStatus returns Kraken's current Spot system status.
func (e *Exchange) GetSystemStatus(ctx context.Context) (*SystemStatusResponse, error) {
	var result *SystemStatusResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, "/0/public/SystemStatus", &result)
}

// GetAssets returns current asset metadata with all supported query parameters.
func (e *Exchange) GetAssets(ctx context.Context, req *GetAssetsRequest) (map[string]*Asset, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if req.AssetVersion != 0 && req.AssetVersion != AssetVersionDisplay {
		return nil, errAssetVersionInvalid
	}

	params := url.Values{}
	if len(req.Assets) > 0 {
		params.Set("asset", req.Assets.Join())
	}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if req.AssetVersion != 0 {
		params.Set("assetVersion", strconv.FormatUint(uint64(req.AssetVersion), 10))
	}

	var result map[string]*Asset
	err := e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Assets", params), &result)
	return result, err
}

// GetAssetPairs returns current tradable-pair metadata with all supported filters.
func (e *Exchange) GetAssetPairs(ctx context.Context, req *GetAssetPairsRequest) (map[string]*AssetPairs, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.AssetClassBase, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if !isValidSpotEnum(req.Info, "info", "leverage", "fees", "margin") {
		return nil, errInfoInvalid
	}
	if !isValidSpotEnum(req.ExecutionVenue, "international", "bitnomial_exchange") {
		return nil, errExecutionVenueInvalid
	}
	if req.AssetVersion != 0 && req.AssetVersion != AssetVersionDisplay {
		return nil, errAssetVersionInvalid
	}

	params := url.Values{}
	if len(req.Pairs) > 0 {
		for i := range req.Pairs {
			if !req.Pairs[i].IsPopulated() {
				return nil, errPairRequired
			}
		}
		pairs, err := e.FormatExchangeCurrencies(req.Pairs, asset.Spot)
		if err != nil {
			return nil, err
		}
		params.Set("pair", pairs)
	}
	if req.AssetClassBase != "" {
		params.Set("aclass_base", string(req.AssetClassBase))
	}
	if req.Info != "" {
		params.Set("info", string(req.Info))
	}
	if req.CountryCode != "" {
		params.Set("country_code", req.CountryCode)
	}
	if req.ExecutionVenue != "" {
		params.Set("execution_venue", string(req.ExecutionVenue))
	}
	if req.AssetVersion != 0 {
		params.Set("assetVersion", strconv.FormatUint(uint64(req.AssetVersion), 10))
	}

	var result map[string]*AssetPairs
	err := e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/AssetPairs", params), &result)
	return result, err
}

// GetTicker returns current ticker data with all supported query parameters.
func (e *Exchange) GetTicker(ctx context.Context, req *GetTickerRequest) (map[string]*TickerResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.AssetClass, "tokenized_asset", "forex") {
		return nil, errAssetClassInvalid
	}
	if req.AssetVersion != 0 && req.AssetVersion != AssetVersionDisplay {
		return nil, errAssetVersionInvalid
	}

	params := url.Values{}
	if len(req.Pairs) > 0 {
		for i := range req.Pairs {
			if !req.Pairs[i].IsPopulated() {
				return nil, errPairRequired
			}
		}
		pairs, err := e.FormatExchangeCurrencies(req.Pairs, asset.Spot)
		if err != nil {
			return nil, err
		}
		params.Set("pair", pairs)
	}
	if req.AssetClass != "" {
		params.Set("asset_class", string(req.AssetClass))
	}
	if req.AssetVersion != 0 {
		params.Set("assetVersion", strconv.FormatUint(uint64(req.AssetVersion), 10))
	}

	var result map[string]*TickerResponse
	err := e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Ticker", params), &result)
	return result, err
}

// GetOHLC returns current OHLC data with pagination metadata.
func (e *Exchange) GetOHLC(ctx context.Context, req *GetOHLCRequest) (*OHLCResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !req.Pair.IsPopulated() {
		return nil, errPairRequired
	}
	switch req.Interval {
	case 0, time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 4 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour, 15 * 24 * time.Hour:
	default:
		return nil, errIntervalInvalid
	}
	if !isValidSpotEnum(req.AssetClass, "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if req.AssetVersion != 0 && req.AssetVersion != AssetVersionDisplay {
		return nil, errAssetVersionInvalid
	}
	symbol, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}

	params := url.Values{"pair": {symbol}}
	if req.Interval != 0 {
		params.Set("interval", strconv.FormatInt(int64(req.Interval/time.Minute), 10))
	}
	if !req.Since.IsZero() {
		if req.Since.Unix() < 0 {
			return nil, errTimestampInvalid
		}
		params.Set("since", strconv.FormatInt(req.Since.Unix(), 10))
	}
	if req.AssetClass != "" {
		params.Set("asset_class", string(req.AssetClass))
	}
	if req.AssetVersion != 0 {
		params.Set("assetVersion", strconv.FormatUint(uint64(req.AssetVersion), 10))
	}

	var result *OHLCResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/OHLC", params), &result)
}

// GetDepth returns the current L2 order book with all supported query parameters.
func (e *Exchange) GetDepth(ctx context.Context, req *GetDepthRequest) (OrderbookResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !req.Pair.IsPopulated() {
		return nil, errPairRequired
	}
	if req.Count > 500 {
		return nil, errDepthCountInvalid
	}
	if !isValidSpotEnum(req.AssetClass, "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if req.AssetVersion != 0 && req.AssetVersion != AssetVersionDisplay {
		return nil, errAssetVersionInvalid
	}
	symbol, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}

	params := url.Values{"pair": {symbol}}
	if req.Count != 0 {
		params.Set("count", strconv.FormatUint(req.Count, 10))
	}
	if req.AssetClass != "" {
		params.Set("asset_class", string(req.AssetClass))
	}
	if req.AssetVersion != 0 {
		params.Set("assetVersion", strconv.FormatUint(uint64(req.AssetVersion), 10))
	}

	var result OrderbookResponse
	err = e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Depth", params), &result)
	return result, err
}

// GetTrades returns current recent-trade data with all supported query parameters.
func (e *Exchange) GetTrades(ctx context.Context, req *GetTradesRequest) (*RecentTradesResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !req.Pair.IsPopulated() {
		return nil, errPairRequired
	}
	if req.Count > 1000 {
		return nil, errTradeCountInvalid
	}
	if !isValidSpotEnum(req.AssetClass, "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if req.AssetVersion != 0 && req.AssetVersion != AssetVersionDisplay {
		return nil, errAssetVersionInvalid
	}

	if !req.Since.IsZero() && req.Cursor != "" {
		return nil, errSinceCursorConflict
	}
	symbol, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}

	params := url.Values{"pair": {symbol}}
	if !req.Since.IsZero() {
		if req.Since.Unix() < 0 {
			return nil, errTimestampInvalid
		}
		params.Set("since", strconv.FormatInt(req.Since.Unix(), 10))
	} else if req.Cursor != "" {
		params.Set("since", req.Cursor)
	}
	if req.Count != 0 {
		params.Set("count", strconv.FormatUint(req.Count, 10))
	}
	if req.AssetClass != "" {
		params.Set("asset_class", string(req.AssetClass))
	}
	if req.AssetVersion != 0 {
		params.Set("assetVersion", strconv.FormatUint(uint64(req.AssetVersion), 10))
	}

	var result *RecentTradesResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Trades", params), &result)
}

// GetSpread returns current spread data with all supported query parameters.
func (e *Exchange) GetSpread(ctx context.Context, req *GetSpreadRequest) (*SpreadResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !req.Pair.IsPopulated() {
		return nil, errPairRequired
	}
	if !isValidSpotEnum(req.AssetClass, "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if req.AssetVersion != 0 && req.AssetVersion != AssetVersionDisplay {
		return nil, errAssetVersionInvalid
	}
	symbol, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}

	params := url.Values{"pair": {symbol}}
	if !req.Since.IsZero() {
		if req.Since.Unix() < 0 {
			return nil, errTimestampInvalid
		}
		params.Set("since", strconv.FormatInt(req.Since.Unix(), 10))
	}
	if req.AssetClass != "" {
		params.Set("asset_class", string(req.AssetClass))
	}
	if req.AssetVersion != 0 {
		params.Set("assetVersion", strconv.FormatUint(uint64(req.AssetVersion), 10))
	}

	var result *SpreadResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/Spread", params), &result)
}

// GetGroupedOrderBook returns grouped L2 order book data for a currency pair.
func (e *Exchange) GetGroupedOrderBook(ctx context.Context, req *GroupedOrderBookRequest) (*GroupedOrderBookResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !req.Pair.IsPopulated() {
		return nil, errPairRequired
	}
	switch req.Depth {
	case 0, 10, 25, 100, 250, 1000:
	default:
		return nil, errGroupedDepthInvalid
	}
	switch req.Grouping {
	case 0, 1, 5, 10, 25, 50, 100, 250, 500, 1000:
	default:
		return nil, errGroupingInvalid
	}
	symbol, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}

	params := url.Values{"pair": {symbol}}
	if req.Depth > 0 {
		params.Set("depth", strconv.FormatUint(uint64(req.Depth), 10))
	}
	if req.Grouping > 0 {
		params.Set("grouping", strconv.FormatUint(uint64(req.Grouping), 10))
	}

	var result *GroupedOrderBookResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/GroupedBook", params), &result)
}

// QueryLevel3OrderBook returns authenticated L3 order book data for a currency pair.
func (e *Exchange) QueryLevel3OrderBook(ctx context.Context, req *QueryLevel3OrderBookRequest) (*QueryLevel3OrderBookResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !req.Pair.IsPopulated() {
		return nil, errPairRequired
	}
	if req.Depth != nil {
		switch *req.Depth {
		case 0, 10, 25, 100, 250, 1000:
		default:
			return nil, errLevel3DepthInvalid
		}
	}
	symbol, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}

	params := url.Values{"pair": {symbol}}
	if req.Depth != nil {
		params.Set("depth", strconv.FormatUint(uint64(*req.Depth), 10))
	}

	var result *QueryLevel3OrderBookResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Level3", params, &result)
}

// GetPreTradeData returns pre-trade transparency data for a symbol.
func (e *Exchange) GetPreTradeData(ctx context.Context, req *GetPreTradeDataRequest) (*GetPreTradeDataResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !req.Pair.IsPopulated() {
		return nil, errSymbolRequired
	}
	symbol, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}
	if len(symbol) < 3 || len(symbol) > 32 {
		return nil, errSymbolLengthInvalid
	}

	params := url.Values{"symbol": {symbol}}
	var result *GetPreTradeDataResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/PreTrade", params), &result)
}

// GetPostTradeData returns post-trade transparency data.
func (e *Exchange) GetPostTradeData(ctx context.Context, req *GetPostTradeDataRequest) (*GetPostTradeDataResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Count > 1000 {
		return nil, errPostTradeCountTooLarge
	}
	if !req.FromTimestamp.IsZero() && !req.ToTimestamp.IsZero() && req.ToTimestamp.Before(req.FromTimestamp) {
		return nil, errTimeRangeInvalid
	}

	params := url.Values{}
	if !req.Pair.IsEmpty() {
		if !req.Pair.IsPopulated() {
			return nil, errPairRequired
		}
		symbol, err := e.FormatSymbol(req.Pair, asset.Spot)
		if err != nil {
			return nil, err
		}
		params.Set("symbol", symbol)
	}
	if !req.FromTimestamp.IsZero() {
		params.Set("from_ts", req.FromTimestamp.UTC().Format(time.RFC3339Nano))
	}
	if !req.ToTimestamp.IsZero() {
		params.Set("to_ts", req.ToTimestamp.UTC().Format(time.RFC3339Nano))
	}
	if req.Count > 0 {
		params.Set("count", strconv.FormatUint(req.Count, 10))
	}

	var result *GetPostTradeDataResponse
	return result, e.SendHTTPRequest(ctx, exchange.RestSpot, common.EncodeURLValues("/0/public/PostTrade", params), &result)
}

// SeedAssets seeds Kraken's asset list and stores it in the asset translator.
func (e *Exchange) SeedAssets(ctx context.Context) error {
	assets, err := e.GetAssets(ctx, new(GetAssetsRequest))
	if err != nil {
		return err
	}
	for orig, val := range assets {
		assetTranslator.Seed(orig, val.Altname)
	}

	assetPairs, err := e.GetAssetPairs(ctx, new(GetAssetPairsRequest))
	if err != nil {
		return err
	}
	for k, v := range assetPairs {
		assetTranslator.Seed(k, v.Altname)
	}
	return nil
}

// LookupAltName converts a currency into its alternate name (ZUSD -> USD).
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

// LookupCurrency converts an alternate name to its original type (USD -> ZUSD).
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

// Seed stores a currency translation pair.
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

// Seeded reports whether assets have been seeded.
func (a *assetTranslatorStore) Seeded() bool {
	a.l.RLock()
	isSeeded := len(a.Assets) > 0
	a.l.RUnlock()
	return isSeeded
}
