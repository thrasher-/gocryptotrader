package binance

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/buger/jsonparser"
	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

const (
	binanceCFuturesWebsocketURL = "wss://dstream.binance.com"
)

var defaultCFuturesSubscriptions = []string{
	aggTradeChan,
	depthChan,
	tickerAllChan,
	klineChan,
	bookTickerAllChan,
}

// WsCFutureConnect initiates a websocket connection to coin margined futures websocket
func (e *Exchange) WsCFutureConnect(ctx context.Context, conn websocket.Connection) error {
	if err := e.CurrencyPairs.IsAssetEnabled(asset.CoinMarginedFutures); err != nil {
		return err
	}

	dialer := gws.Dialer{
		HandshakeTimeout: e.Config.HTTPTimeout,
		Proxy:            http.ProxyFromEnvironment,
	}
	wsURL := strings.TrimSuffix(conn.GetURL(), "/stream") + "/stream"
	conn.SetURL(wsURL)
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

// GenerateDefaultCFuturesSubscriptions generates a list of subscription instances.
func (e *Exchange) GenerateDefaultCFuturesSubscriptions() (subscription.List, error) {
	if !slices.Contains(e.GetAssetTypes(true), asset.CoinMarginedFutures) {
		return nil, nil
	}
	if configured, present, err := e.configuredDerivativeSubscriptions(asset.CoinMarginedFutures, ""); present || err != nil {
		return configured, err
	}
	var subscriptions subscription.List
	pairs, err := e.GetEnabledPairs(asset.CoinMarginedFutures)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	channels := defaultCFuturesSubscriptions
	for z := range channels {
		var chSubscription *subscription.Subscription
		switch channels[z] {
		case contractInfoAllChan, forceOrderAllChan,
			bookTickerAllChan, tickerAllChan, miniTickerAllChan:
			subscriptions = append(subscriptions, &subscription.Subscription{
				Channel:          channels[z],
				QualifiedChannel: channels[z],
				Asset:            asset.CoinMarginedFutures,
			})
		case aggTradeChan, depthChan, markPriceChan, tickerChan,
			klineChan, miniTickerChan, forceOrderChan,
			indexPriceCFuturesChan, bookTickerCFuturesChan,
			indexPriceKlineCFuturesChan, markPriceKlineCFuturesChan:
			for y := range pairs {
				lp := pairs[y].Lower()
				lp.Delimiter = "_"
				chSubscription = &subscription.Subscription{
					QualifiedChannel: lp.String() + channels[z],
					Asset:            asset.CoinMarginedFutures,
				}
				switch channels[z] {
				case depthChan:
					chSubscription.QualifiedChannel += "@100ms"
				case klineChan, indexPriceKlineCFuturesChan, markPriceKlineCFuturesChan:
					chSubscription.QualifiedChannel += "_" + getKlineIntervalString(kline.FiveMin)
				}
				chSubscription.Channel = chSubscription.QualifiedChannel
				subscriptions = append(subscriptions, chSubscription)
			}
		case continuousKline:
			for y := range pairs {
				underlying := pairs[y].Base.Lower().String()
				chSubscription = &subscription.Subscription{
					// Contract types: "perpetual", "current_quarter", "next_quarter"
					Asset:            asset.CoinMarginedFutures,
					QualifiedChannel: underlying + "_perpetual@" + channels[z] + "_" + getKlineIntervalString(kline.FiveMin),
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

func (e *Exchange) wsHandleCFuturesData(ctx context.Context, conn websocket.Connection, respRaw []byte) error {
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
		if eventType, err := jsonparser.GetString(respRaw, "e"); err == nil {
			return e.wsHandleDerivativeUserData(ctx, eventType, respRaw, asset.CoinMarginedFutures)
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
	case contractInfoAllChan:
		return e.processContractInfoStream(ctx, result.Data)
	case forceOrderAllChan, "forceOrder":
		return e.processCFuturesForceOrder(ctx, result.Data)
	case bookTickerAllChan, bookTickerStream:
		return e.processBookTicker(ctx, result.Data, asset.CoinMarginedFutures)
	case tickerAllChan:
		return e.processCFuturesMarketTicker(ctx, result.Data, true)
	case tickerStream:
		return e.processCFuturesMarketTicker(ctx, result.Data, false)
	case miniTickerAllChan:
		return e.processMiniTickers(ctx, result.Data, true, asset.CoinMarginedFutures)
	case "miniTicker":
		return e.processMiniTickers(ctx, result.Data, false, asset.CoinMarginedFutures)
	case aggTradeStream:
		return e.processAggregateTrade(ctx, result.Data, asset.CoinMarginedFutures)
	case "markPrice":
		return e.processMarkPriceUpdate(ctx, result.Data, len(result.Data) > 0 && result.Data[0] == '[')
	case cnlDepth:
		return e.processDerivativeDepth(ctx, result.Data, asset.CoinMarginedFutures, isPartialDepthStream(result.Stream))
	case continuousKline:
		return e.processContinuousKlineUpdate(ctx, result.Data, asset.CoinMarginedFutures)
	case cnlKline:
		return e.processFuturesKline(ctx, result.Data, asset.CoinMarginedFutures)
	case "indexPrice":
		return e.processIndexPrice(ctx, result.Data)
	case "indexPriceKline", "markPriceKline":
		return e.processMarkPriceKline(ctx, result.Data)
	}
	return fmt.Errorf("unhandled stream data %s", string(respRaw))
}

func (e *Exchange) processCFuturesMarketTicker(ctx context.Context, respRaw []byte, array bool) error {
	if array {
		var resp []CFuturesMarketTicker
		if err := json.Unmarshal(respRaw, &resp); err != nil {
			return err
		}
		tickerPrices, err := e.getCFuturesTickerInfos(resp)
		if err != nil {
			return err
		}
		return e.Websocket.DataHandler.Send(ctx, tickerPrices)
	}
	var resp CFuturesMarketTicker
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	eventAsset := futuresStreamAsset(resp.SymbolType, asset.CoinMarginedFutures)
	baseVolume, quoteVolume := resp.TotalTradedVolume.Float64(), resp.TotalTradedBaseAssetVolume.Float64()
	if eventAsset == asset.CoinMarginedFutures {
		baseVolume, quoteVolume = quoteVolume, 0
	}
	cp, err := e.derivativePair(resp.Symbol, eventAsset)
	if err != nil {
		return err
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
		AssetType:    eventAsset,
		LastUpdated:  resp.EventTime.Time(),
	})
}

func (e *Exchange) getCFuturesTickerInfos(marketTickers []CFuturesMarketTicker) ([]ticker.Price, error) {
	tickerPrices := make([]ticker.Price, len(marketTickers))
	for a := range marketTickers {
		eventAsset := futuresStreamAsset(marketTickers[a].SymbolType, asset.CoinMarginedFutures)
		baseVolume, quoteVolume := marketTickers[a].TotalTradedVolume.Float64(), marketTickers[a].TotalTradedBaseAssetVolume.Float64()
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

func (e *Exchange) processFuturesKline(ctx context.Context, respRaw []byte, assetType asset.Item) error {
	var resp KlineStream
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	pair, err := e.derivativePair(resp.Symbol, assetType)
	if err != nil {
		return err
	}
	interval, err := formatToInterval(resp.Kline.Interval)
	if err != nil {
		return err
	}
	var validationIssues string
	if !resp.Kline.KlineClosed {
		validationIssues = kline.PartialCandle
	}
	volume := resp.Kline.Volume.Float64()
	if assetType == asset.CoinMarginedFutures {
		volume = resp.Kline.Quote.Float64()
	}
	return e.Websocket.DataHandler.Send(ctx, &kline.Item{Pair: pair, Exchange: e.Name, Asset: assetType, Interval: interval, Candles: []kline.Candle{{Time: resp.Kline.StartTime.Time(), Open: resp.Kline.OpenPrice.Float64(), High: resp.Kline.HighPrice.Float64(), Low: resp.Kline.LowPrice.Float64(), Close: resp.Kline.ClosePrice.Float64(), Volume: volume, ValidationIssues: validationIssues}}})
}

func (e *Exchange) processIndexPrice(ctx context.Context, respRaw []byte) error {
	var resp CFutureIndexPriceStream
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	cp, err := e.derivativePair(resp.Pair, asset.CoinMarginedFutures)
	if err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, &ticker.Price{
		Pair:         cp,
		Last:         resp.IndexPrice.Float64(),
		ExchangeName: e.Name,
		AssetType:    asset.CoinMarginedFutures,
		LastUpdated:  resp.EventTime.Time(),
	})
}

func (e *Exchange) processCFuturesForceOrder(ctx context.Context, respRaw []byte) error {
	return e.processForceOrder(ctx, respRaw, asset.CoinMarginedFutures)
}

func (e *Exchange) processMarkPriceKline(ctx context.Context, respRaw []byte) error {
	var resp CFutureMarkOrIndexPriceKline
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return err
	}
	cp, err := e.derivativePair(resp.Pair, asset.CoinMarginedFutures)
	if err != nil {
		return err
	}
	interval, err := formatToInterval(resp.Kline.Interval)
	if err != nil {
		return err
	}
	var validationIssues string
	if !resp.Kline.IsKlineClosed {
		validationIssues = kline.PartialCandle
	}
	return e.Websocket.DataHandler.Send(ctx, &kline.Item{
		Pair:     cp,
		Exchange: e.Name,
		Asset:    asset.CoinMarginedFutures,
		Interval: interval,
		Candles: []kline.Candle{{
			Time:             resp.Kline.StartTime.Time(),
			ValidationIssues: validationIssues,
			Open:             resp.Kline.OpenPrice.Float64(),
			Close:            resp.Kline.ClosePrice.Float64(),
			High:             resp.Kline.HighPrice.Float64(),
			Low:              resp.Kline.LowPrice.Float64(),
		}},
	})
}
