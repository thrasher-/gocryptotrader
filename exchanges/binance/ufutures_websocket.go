package binance

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	"github.com/thrasher-corp/gocryptotrader/log"
)

const (
	fstreamPublicURL  = "wss://fstream.binance.com/public/stream"
	fstreamMarketURL  = "wss://fstream.binance.com/market/stream"
	fstreamPrivateURL = "wss://fstream.binance.com/private"

	usdtmPublicFilter  = "usd-m-public"
	usdtmMarketFilter  = "usd-m-market"
	usdtmPrivateFilter = "usd-m-private"
)

var defaultSubscriptions = []string{
	aggTradeChan,
	depthChan,
	tickerAllChan,
	klineChan,

	bookTickerAllChan, bookTickersChan,
}

// getKlineIntervalString returns a string representation of the kline interval.
func getKlineIntervalString(interval kline.Interval) string {
	klineMap := map[kline.Interval]string{
		kline.OneMin: "1m", kline.ThreeMin: "3m", kline.FiveMin: "5m", kline.FifteenMin: "15m", kline.ThirtyMin: "30m",
		kline.OneHour: "1h", kline.TwoHour: "2h", kline.FourHour: "4h", kline.SixHour: "6h", kline.EightHour: "8h", kline.TwelveHour: "12h",
		kline.OneDay: "1d", kline.ThreeDay: "3d", kline.OneWeek: "1w", kline.OneMonth: "1M",
	}
	intervalString, okay := klineMap[interval]
	if !okay {
		return ""
	}
	return intervalString
}

// WsUFuturesConnect initiates a websocket connection
func (e *Exchange) WsUFuturesConnect(ctx context.Context, conn websocket.Connection) error {
	err := e.CurrencyPairs.IsAssetEnabled(asset.USDTMarginedFutures)
	if err != nil {
		return err
	}

	dialer := gws.Dialer{
		HandshakeTimeout: e.Config.HTTPTimeout,
		Proxy:            http.ProxyFromEnvironment,
	}
	if err := conn.Dial(ctx, &dialer, http.Header{}, nil); err != nil {
		return fmt.Errorf("%s - unable to connect to websocket: %w", e.Name, err)
	}
	conn.SetupPingHandler(request.UnAuth, websocket.PingHandler{
		UseGorillaHandler: true,
		MessageType:       gws.PongMessage,
		Delay:             pingDelay,
	})
	return nil
}

func (e *Exchange) wsHandleFuturesData(ctx context.Context, conn websocket.Connection, respRaw []byte) error {
	var result struct {
		Result json.RawMessage `json:"result"`
		ID     json.RawMessage `json:"id"`
		Stream string          `json:"stream"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respRaw, &result); err != nil {
		return err
	}
	if result.Stream == "" || (len(result.ID) != 0 && result.Result != nil) {
		// The user data stream is served at /ws/<listenKey> and delivers the event object
		// directly, with no "stream" wrapper; only market streams carry one. Route on the
		// event type field so these are not mistaken for unmatched request responses.
		if eventType, err := jsonparser.GetUnsafeString(respRaw, "e"); err == nil {
			return e.wsHandleDerivativeUserData(ctx, eventType, respRaw, asset.USDTMarginedFutures)
		}
		return matchSubscriptionResponse(conn, result.ID, respRaw)
	}
	var stream string
	switch result.Stream {
	case assetIndexAllChan, forceOrderAllChan, bookTickerAllChan, tickerAllChan, miniTickerAllChan:
		stream = result.Stream
	default:
		stream = extractStreamInfo(result.Stream)
	}
	switch stream {
	case assetIndexAllChan:
		return e.processMultiAssetModeAssetIndexes(ctx, result.Data, true)
	case "assetIndex":
		return e.processMultiAssetModeAssetIndexes(ctx, result.Data, false)
	case contractInfoAllChan:
		return e.processContractInfoStream(ctx, result.Data)
	case forceOrderAllChan, "forceOrder":
		return e.processForceOrder(ctx, result.Data, asset.USDTMarginedFutures)
	case bookTickerAllChan, bookTickerStream:
		return e.processBookTicker(ctx, result.Data, asset.USDTMarginedFutures)
	case tickerAllChan:
		return e.processMarketTicker(ctx, result.Data, true, asset.USDTMarginedFutures)
	case tickerStream:
		return e.processMarketTicker(ctx, result.Data, false, asset.USDTMarginedFutures)
	case miniTickerAllChan:
		return e.processMiniTickers(ctx, result.Data, true, asset.USDTMarginedFutures)
	case "miniTicker":
		return e.processMiniTickers(ctx, result.Data, false, asset.USDTMarginedFutures)
	case aggTradeStream:
		return e.processAggregateTrade(ctx, result.Data, asset.USDTMarginedFutures)
	case "markPrice":
		return e.processMarkPriceUpdate(ctx, result.Data, false)
	case "!markPrice@arr":
		return e.processMarkPriceUpdate(ctx, result.Data, true)
	case cnlDepth:
		return e.processDerivativeDepth(ctx, result.Data, asset.USDTMarginedFutures, isPartialDepthStream(result.Stream))
	case "rpiDepth":
		var data FuturesRPIDepth
		if err := json.Unmarshal(result.Data, &data); err != nil {
			return err
		}
		return e.Websocket.DataHandler.Send(ctx, data)
	case "compositeIndex":
		return e.processCompositeIndex(ctx, result.Data)
	case cnlKline:
		return e.processFuturesKline(ctx, result.Data, asset.USDTMarginedFutures)
	case continuousKline:
		return e.processContinuousKlineUpdate(ctx, result.Data, asset.USDTMarginedFutures)
	}
	return fmt.Errorf("unhandled stream data %s", string(respRaw))
}

// wsHandleFuturesUserData routes USD-M user data stream events. These arrive on the
// authenticated /ws/<listenKey> connection as bare event objects.
func (e *Exchange) wsHandleFuturesUserData(ctx context.Context, eventType string, data []byte) error {
	return e.wsHandleDerivativeUserData(ctx, eventType, data, asset.USDTMarginedFutures)
}

func (e *Exchange) wsHandleDerivativeUserData(ctx context.Context, eventType string, data []byte, assetType asset.Item) error {
	var response any
	switch eventType {
	case "ACCOUNT_UPDATE":
		return e.processBalanceAndPositionUpdate(ctx, data, assetType)
	case "ORDER_TRADE_UPDATE":
		return e.processFuturesOrderTradeUpdate(ctx, data, assetType)
	case "ACCOUNT_CONFIG_UPDATE":
		response = new(FuturesAccountConfigUpdate)
	case listenKeyExpiredEvent:
		response = new(FuturesListenKeyExpired)
	case "MARGIN_CALL":
		response = new(FuturesMarginCall)
	case "TRADE_LITE":
		response = new(FuturesTradeLite)
	case "STRATEGY_UPDATE":
		response = new(FuturesStrategyUpdate)
	case "GRID_UPDATE":
		response = new(FuturesGridUpdate)
	case "ALGO_UPDATE":
		response = new(FuturesAlgoUpdate)
	case "CONDITIONAL_ORDER_TRIGGER_REJECT":
		response = new(FuturesConditionalOrderReject)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedChannel, eventType)
	}
	if err := json.Unmarshal(data, response); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, response)
}

// processFuturesOrderTradeUpdate converts an ORDER_TRADE_UPDATE event into an order detail.
func (e *Exchange) processFuturesOrderTradeUpdate(ctx context.Context, respRaw []byte, assetType asset.Item) error {
	var resp *FuturesOrderTradeUpdate
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	if resp.Order == nil {
		return nil
	}
	o := resp.Order
	status, err := stringToOrderStatus(o.OrderStatus)
	if err != nil {
		return err
	}
	side, err := order.StringToOrderSide(o.Side)
	if err != nil {
		return e.Websocket.DataHandler.Send(ctx, order.ClassificationError{
			Exchange: e.Name,
			OrderID:  strconv.FormatUint(o.OrderID, 10),
			Err:      err,
		})
	}
	oType, err := order.StringToOrderType(o.OrderType)
	if err != nil {
		return err
	}
	// Binance sends delimiter-less symbols, which currency.Pair cannot parse unaided.
	pair, err := e.derivativePair(o.Symbol, assetType)
	if err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, &order.Detail{
		Exchange:             e.Name,
		OrderID:              strconv.FormatUint(o.OrderID, 10),
		ClientOrderID:        o.ClientOrderID,
		Pair:                 pair,
		AssetType:            assetType,
		Side:                 side,
		Type:                 oType,
		Status:               status,
		TimeInForce:          o.TimeInForce,
		Price:                o.OriginalPrice.Float64(),
		Amount:               o.OriginalQuantity.Float64(),
		AverageExecutedPrice: o.AveragePrice.Float64(),
		ExecutedAmount:       o.FilledAccumulatedQuantity.Float64(),
		RemainingAmount:      o.OriginalQuantity.Float64() - o.FilledAccumulatedQuantity.Float64(),
		Fee:                  o.Commission.Float64(),
		FeeAsset:             o.CommissionAsset,
		TriggerPrice:         o.StopPrice.Float64(),
		ReduceOnly:           o.IsReduceOnly,
		Date:                 resp.EventTime.Time(),
		LastUpdated:          o.OrderTradeTime.Time(),
	})
}

func (e *Exchange) processBalanceAndPositionUpdate(ctx context.Context, respRaw []byte, assetType asset.Item) error {
	var resp *WSBalanceAndPositionUpdate
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	if resp.UpdateData == nil {
		return nil
	}
	log.Debugf(log.ExchangeSys, "%v ACCOUNT_UPDATE reason: %s eventTime: %v", e.Name, resp.UpdateData.ReasonType, resp.EventTime.Time())
	subAccts := accounts.SubAccounts{}
	for _, b := range resp.UpdateData.Balances {
		subAcct := accounts.NewSubAccount(assetType, "")
		walletBalance := b.WalletBalance.Float64()
		crossWallet := b.CrossWallet.Float64()
		subAcct.Balances.Set(b.Asset, accounts.Balance{
			Currency:  b.Asset,
			Total:     walletBalance,
			Hold:      walletBalance - crossWallet,
			Free:      crossWallet,
			UpdatedAt: resp.Transaction.Time(),
		})
		subAccts = subAccts.Merge(subAcct)
	}
	for _, p := range resp.UpdateData.Positions {
		if p.PositionAmount.Float64() == 0 {
			log.Debugf(log.ExchangeSys, "%v position closed: %v side: %s", e.Name, p.Symbol, p.PositionSide)
		}
	}
	if len(resp.UpdateData.Positions) > 0 {
		if err := e.Websocket.DataHandler.Send(ctx, resp.UpdateData.Positions); err != nil {
			return err
		}
	}
	if len(subAccts) == 0 {
		return nil
	}
	if err := e.Accounts.Save(ctx, subAccts, true); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, subAccts)
}

func (e *Exchange) processContinuousKlineUpdate(ctx context.Context, respRaw []byte, assetType asset.Item) error {
	var resp FutureContinuousKline
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	cp, err := e.derivativePair(resp.Pair, assetType)
	if err != nil {
		return err
	}
	interval, err := formatToInterval(resp.KlineData.Interval)
	if err != nil {
		return err
	}
	volume := resp.KlineData.Volume.Float64()
	if assetType == asset.CoinMarginedFutures {
		volume = resp.KlineData.QuoteAssetVolume.Float64()
	}
	var validationIssues string
	if !resp.KlineData.IsKlineClosed {
		validationIssues = kline.PartialCandle
	}
	return e.Websocket.DataHandler.Send(ctx, kline.Item{
		Pair:     cp,
		Exchange: e.Name,
		Interval: interval,
		Asset:    assetType,
		Candles: []kline.Candle{
			{
				Time:             resp.KlineData.StartTime.Time(),
				ValidationIssues: validationIssues,
				Open:             resp.KlineData.OpenPrice.Float64(),
				Close:            resp.KlineData.ClosePrice.Float64(),
				High:             resp.KlineData.HighPrice.Float64(),
				Low:              resp.KlineData.LowPrice.Float64(),
				Volume:           volume,
			},
		},
	})
}

func (e *Exchange) processCompositeIndex(ctx context.Context, respRaw []byte) error {
	var resp UFutureCompositeIndex
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

func (e *Exchange) processAggregateTrade(ctx context.Context, respRaw []byte, assetType asset.Item) error {
	var resp FuturesAggTrade
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	assetType = futuresStreamAsset(resp.SymbolType, assetType)
	cp, err := e.derivativePair(resp.Symbol, assetType)
	if err != nil {
		return err
	}
	side := order.Buy
	if resp.IsMaker {
		side = order.Sell
	}
	return e.Websocket.DataHandler.Send(ctx, []trade.Data{
		{
			TID:          strconv.FormatUint(resp.AggregateTradeID, 10),
			Side:         side,
			Exchange:     e.Name,
			CurrencyPair: cp,
			AssetType:    assetType,
			Price:        resp.Price.Float64(),
			Amount:       resp.Quantity.Float64(),
			Timestamp:    resp.TradeTime.Time(),
		},
	})
}

func extractStreamInfo(resultStream string) string {
	splitStream := strings.Split(resultStream, "@")
	if len(splitStream) < 2 {
		return resultStream
	}
	switch splitStream[1] {
	case "aggTrade", "markPrice", "ticker", "bookTicker", "forceOrder", "depth",
		"compositeIndex", "assetIndex", "miniTicker", "indexPrice", "rpiDepth":
		return splitStream[1]
	default:
		switch {
		case strings.HasPrefix(splitStream[1], "depth"):
			return "depth"
		case strings.HasPrefix(splitStream[1], "continuousKline"):
			return "continuousKline"
		// The interval-suffixed kline streams normalise to their base name so that
		// handlers can match them; without this they fall through unrecognised.
		case strings.HasPrefix(splitStream[1], "indexPriceKline"):
			return "indexPriceKline"
		case strings.HasPrefix(splitStream[1], "markPriceKline"):
			return "markPriceKline"
		case strings.HasPrefix(splitStream[1], "kline"):
			return "kline"
		case strings.HasPrefix(splitStream[0], "!markPrice"):
			return "!markPrice@arr"
		}
	}
	return resultStream
}

func (e *Exchange) processMiniTickers(ctx context.Context, respRaw []byte, array bool, assetType asset.Item) error {
	if array {
		var resp []FutureMiniTickerPrice
		if err := json.Unmarshal(respRaw, &resp); err != nil {
			return err
		}
		tickerPrices, err := e.getMiniTickers(resp, assetType)
		if err != nil {
			return err
		}
		return e.Websocket.DataHandler.Send(ctx, tickerPrices)
	}
	var resp FutureMiniTickerPrice
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	assetType = futuresStreamAsset(resp.SymbolType, assetType)
	cp, err := e.derivativePair(resp.Symbol, assetType)
	if err != nil {
		return err
	}
	baseVolume, quoteVolume := resp.Volume.Float64(), resp.QuoteVolume.Float64()
	if assetType == asset.CoinMarginedFutures {
		baseVolume, quoteVolume = quoteVolume, 0
	}
	return e.Websocket.DataHandler.Send(ctx, &ticker.Price{
		Pair:         cp,
		High:         resp.HighPrice.Float64(),
		Low:          resp.LowPrice.Float64(),
		BaseVolume:   baseVolume,
		QuoteVolume:  quoteVolume,
		Last:         resp.ClosePrice.Float64(),
		Open:         resp.OpenPrice.Float64(),
		ExchangeName: e.Name,
		AssetType:    assetType,
		LastUpdated:  resp.EventTime.Time(),
	})
}

func (e *Exchange) getMiniTickers(miniTickers []FutureMiniTickerPrice, assetType asset.Item) ([]ticker.Price, error) {
	tickerPrices := make([]ticker.Price, len(miniTickers))
	for i := range miniTickers {
		eventAsset := futuresStreamAsset(miniTickers[i].SymbolType, assetType)
		cp, err := e.derivativePair(miniTickers[i].Symbol, eventAsset)
		if err != nil {
			return nil, err
		}
		baseVolume, quoteVolume := miniTickers[i].Volume.Float64(), miniTickers[i].QuoteVolume.Float64()
		if eventAsset == asset.CoinMarginedFutures {
			baseVolume, quoteVolume = quoteVolume, 0
		}
		tickerPrices[i] = ticker.Price{
			Pair:         cp,
			High:         miniTickers[i].HighPrice.Float64(),
			Low:          miniTickers[i].LowPrice.Float64(),
			BaseVolume:   baseVolume,
			Last:         miniTickers[i].ClosePrice.Float64(),
			QuoteVolume:  quoteVolume,
			Open:         miniTickers[i].OpenPrice.Float64(),
			ExchangeName: e.Name,
			AssetType:    eventAsset,
			LastUpdated:  miniTickers[i].EventTime.Time(),
		}
	}
	return tickerPrices, nil
}

func (e *Exchange) processMarketTicker(ctx context.Context, respRaw []byte, array bool, assetType asset.Item) error {
	if array {
		var resp []UFutureMarketTicker
		if err := json.Unmarshal(respRaw, &resp); err != nil {
			return err
		}
		tickerPrices, err := e.getTickerInfos(resp)
		if err != nil {
			return err
		}
		return e.Websocket.DataHandler.Send(ctx, tickerPrices)
	}
	var resp UFutureMarketTicker
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	assetType = futuresStreamAsset(resp.SymbolType, assetType)
	cp, err := e.derivativePair(resp.Symbol, assetType)
	if err != nil {
		return err
	}
	baseVolume, quoteVolume := resp.TotalTradeBaseVolume.Float64(), resp.TotalQuoteAssetVolume.Float64()
	if assetType == asset.CoinMarginedFutures {
		baseVolume, quoteVolume = quoteVolume, 0
	}
	return e.Websocket.DataHandler.Send(ctx, &ticker.Price{
		Pair:         cp,
		Last:         resp.LastPrice.Float64(),
		High:         resp.HighPrice.Float64(),
		Low:          resp.LowPrice.Float64(),
		BaseVolume:   baseVolume,
		QuoteVolume:  quoteVolume,
		Open:         resp.OpenPrice.Float64(),
		ExchangeName: e.Name,
		AssetType:    assetType,
		LastUpdated:  resp.EventTime.Time(),
	})
}

func (e *Exchange) getTickerInfos(marketTickers []UFutureMarketTicker) ([]ticker.Price, error) {
	tickerPrices := make([]ticker.Price, len(marketTickers))
	for a := range marketTickers {
		eventAsset := futuresStreamAsset(marketTickers[a].SymbolType, asset.USDTMarginedFutures)
		baseVolume, quoteVolume := marketTickers[a].TotalTradeBaseVolume.Float64(), marketTickers[a].TotalQuoteAssetVolume.Float64()
		if eventAsset == asset.CoinMarginedFutures {
			baseVolume, quoteVolume = quoteVolume, 0
		}
		cp, err := e.derivativePair(marketTickers[a].Symbol, eventAsset)
		if err != nil {
			return nil, err
		}
		tickerPrices[a] = ticker.Price{
			Pair:         cp,
			Last:         marketTickers[a].LastPrice.Float64(),
			High:         marketTickers[a].HighPrice.Float64(),
			Low:          marketTickers[a].LowPrice.Float64(),
			BaseVolume:   baseVolume,
			QuoteVolume:  quoteVolume,
			Open:         marketTickers[a].OpenPrice.Float64(),
			ExchangeName: e.Name,
			AssetType:    eventAsset,
			LastUpdated:  marketTickers[a].EventTime.Time(),
		}
	}
	return tickerPrices, nil
}

// A best bid/ask update is a ticker, not a depth delta. Publishing it separately
// avoids leaving stale price levels in the book or replacing a synchronised book.
func (e *Exchange) processBookTicker(ctx context.Context, respRaw []byte, assetType asset.Item) error {
	var resp FuturesBookTicker
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	assetType = futuresStreamAsset(resp.SymbolType, assetType)
	pair, err := e.derivativePair(resp.Symbol, assetType)
	if err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, &ticker.Price{Pair: pair, ExchangeName: e.Name, AssetType: assetType, Bid: resp.BestBidPrice.Float64(), Ask: resp.BestAskPrice.Float64(), LastUpdated: resp.TransactionTime.Time()})
}

func (e *Exchange) processForceOrder(ctx context.Context, respRaw []byte, assetType asset.Item) error {
	var resp MarketLiquidationOrder
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	oType, err := order.StringToOrderType(resp.Order.OrderType)
	if err != nil {
		return err
	}
	oSide, err := order.StringToOrderSide(resp.Order.Side)
	if err != nil {
		return err
	}
	oStatus, err := order.StringToOrderStatus(resp.Order.OrderStatus)
	if err != nil {
		return err
	}
	assetType = futuresStreamAsset(resp.Order.SymbolType, futuresStreamAsset(resp.SymbolType, assetType))
	cp, err := e.derivativePair(resp.Order.Symbol, assetType)
	if err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, &order.Detail{
		Price:                resp.Order.Price.Float64(),
		Amount:               resp.Order.OriginalQuantity.Float64(),
		AverageExecutedPrice: resp.Order.AveragePrice.Float64(),
		ExecutedAmount:       resp.Order.OrderFilledAccumulatedQuantity.Float64(),
		RemainingAmount:      resp.Order.OriginalQuantity.Float64() - resp.Order.OrderFilledAccumulatedQuantity.Float64(),
		Exchange:             e.Name,
		Type:                 oType,
		Side:                 oSide,
		Status:               oStatus,
		AssetType:            assetType,
		LastUpdated:          resp.Order.OrderTradeTime.Time(),
		Pair:                 cp,
		TimeInForce:          resp.Order.TimeInForce,
	})
}

func (e *Exchange) processContractInfoStream(ctx context.Context, respRaw []byte) error {
	var resp FuturesContractInfo
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

func (e *Exchange) processMultiAssetModeAssetIndexes(ctx context.Context, respRaw []byte, array bool) error {
	if array {
		var resp []UFuturesAssetIndexUpdate
		if err := json.Unmarshal(respRaw, &resp); err != nil {
			return err
		}
		return e.Websocket.DataHandler.Send(ctx, resp)
	}
	var resp UFuturesAssetIndexUpdate
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

func (e *Exchange) processMarkPriceUpdate(ctx context.Context, respRaw []byte, array bool) error {
	if array {
		var resp []FuturesMarkPrice
		if err := json.Unmarshal(respRaw, &resp); err != nil {
			return err
		}
		if err := e.Websocket.DataHandler.Send(ctx, resp); err != nil {
			return err
		}
		return nil
	}
	var resp FuturesMarkPrice
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

// SubscribeFutures subscribes to a set of channels
func (e *Exchange) SubscribeFutures(ctx context.Context, conn websocket.Connection, channelsToSubscribe subscription.List) error {
	return e.handleSubscriptions(ctx, conn, "SUBSCRIBE", channelsToSubscribe)
}

// UnsubscribeFutures unsubscribes from a set of channels
func (e *Exchange) UnsubscribeFutures(ctx context.Context, conn websocket.Connection, channelsToUnsubscribe subscription.List) error {
	return e.handleSubscriptions(ctx, conn, "UNSUBSCRIBE", channelsToUnsubscribe)
}

func (e *Exchange) handleSubscriptions(ctx context.Context, conn websocket.Connection, operation string, channels subscription.List) error {
	if len(channels) == 0 {
		return common.ErrEmptyParams
	}
	for _, sub := range channels {
		if sub.QualifiedChannel == "" {
			sub.QualifiedChannel = sub.Channel
		}
	}
	for start := 0; start < len(channels); start += 50 {
		end := min(start+50, len(channels))
		if err := e.manageSubs(ctx, conn, operation, channels[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// GenerateUFuturesDefaultSubscriptions generates the default subscription set
func (e *Exchange) GenerateUFuturesDefaultSubscriptions(messageFilter string) (subscription.List, error) {
	if messageFilter == usdtmPrivateFilter {
		return nil, nil
	}
	if messageFilter != usdtmPublicFilter && messageFilter != usdtmMarketFilter {
		return nil, errUnsupportedSubscription
	}
	if !slices.Contains(e.GetAssetTypes(true), asset.USDTMarginedFutures) {
		return nil, nil
	}
	if configured, present, err := e.configuredDerivativeSubscriptions(asset.USDTMarginedFutures, messageFilter); present || err != nil {
		return configured, err
	}
	var subscriptions subscription.List
	pairs, err := e.GetEnabledPairs(asset.USDTMarginedFutures)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	var channels []string
	for _, ch := range defaultSubscriptions {
		switch messageFilter {
		case usdtmPublicFilter:
			switch ch {
			case bookTickerAllChan, bookTickersChan, depthChan:
				channels = append(channels, ch)
			}
		case usdtmMarketFilter:
			switch ch {
			case aggTradeChan, markPriceChan, markPriceAllChan, klineChan, continuousKline, miniTickerChan, miniTickerAllChan, tickerChan, tickerAllChan, forceOrderChan, forceOrderAllChan, compositeIndexChan, contractInfoAllChan, assetIndexChan, assetIndexAllChan:
				channels = append(channels, ch)
			}
		case usdtmPrivateFilter:
			return subscription.List{}, nil
		}
	}
	for z := range channels {
		var chSubscription *subscription.Subscription
		switch channels[z] {
		case assetIndexAllChan, contractInfoAllChan, forceOrderAllChan,
			bookTickerAllChan, tickerAllChan, miniTickerAllChan, markPriceAllChan:
			if channels[z] == markPriceAllChan {
				channels[z] += "@1s"
			}
			subscriptions = append(subscriptions, &subscription.Subscription{
				Asset:            asset.USDTMarginedFutures,
				Channel:          channels[z],
				QualifiedChannel: channels[z],
			})
		case aggTradeChan, depthChan, markPriceChan, tickerChan, klineChan,
			miniTickerChan, bookTickersChan, forceOrderChan, compositeIndexChan, assetIndexChan:
			for y := range pairs {
				lp := pairs[y].Lower()
				lp.Delimiter = ""
				chSubscription = &subscription.Subscription{
					Asset:            asset.USDTMarginedFutures,
					QualifiedChannel: lp.String() + channels[z],
				}
				switch channels[z] {
				case depthChan:
					chSubscription.QualifiedChannel += "@100ms"
				case klineChan:
					chSubscription.QualifiedChannel += "_" + getKlineIntervalString(kline.FiveMin)
				}
				chSubscription.Channel = chSubscription.QualifiedChannel
				subscriptions = append(subscriptions, chSubscription)
			}
		case continuousKline:
			for y := range pairs {
				lp := pairs[y].Lower()
				lp.Delimiter = ""
				chSubscription = &subscription.Subscription{
					// Contract types:"PERPETUAL", "CURRENT_MONTH", "NEXT_MONTH", "CURRENT_QUARTER", "NEXT_QUARTER"
					// by default we are subscribing to PERPETUAL contract types
					Asset:            asset.USDTMarginedFutures,
					QualifiedChannel: lp.String() + "_perpetual@" + channels[z] + "_" + getKlineIntervalString(kline.FifteenMin),
				}
				chSubscription.Channel = chSubscription.QualifiedChannel
				subscriptions = append(subscriptions, chSubscription)
			}
		default:
			return nil, fmt.Errorf("%w: channel %s", subscription.ErrNotSupported, channels[z])
		}
	}
	return subscriptions, nil
}

// ListSubscriptions retrieves list of subscriptions
func (e *Exchange) ListSubscriptions(ctx context.Context, conn websocket.Connection) ([]string, error) {
	req := &WsPayload{
		ID:     e.MessageID(),
		Method: "LIST_SUBSCRIPTIONS",
	}
	var resp WebsocketActionResponse
	respRaw, err := conn.SendMessageReturnResponse(ctx, request.UnAuth, req.ID, &req)
	if err != nil {
		return nil, err
	}
	return resp.Result, json.Unmarshal(respRaw, &resp)
}

// SetProperty to set a property for the websocket connection you are using.
func (e *Exchange) SetProperty(ctx context.Context, conn websocket.Connection, property string, value any) error {
	// Currently, the only property can be set is to set whether "combined" stream payloads are enabled are not.
	req := &struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params []any  `json:"params"`
	}{
		ID:     e.MessageSequence(),
		Method: "SET_PROPERTY",
		Params: []any{
			property,
			value,
		},
	}
	var resp WebsocketActionResponse
	respRaw, err := conn.SendMessageReturnResponse(ctx, request.UnAuth, req.ID, &req)
	if err != nil {
		return err
	}
	return json.Unmarshal(respRaw, &resp)
}
