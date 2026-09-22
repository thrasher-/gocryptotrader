package binance

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
)

const (
	fstreamOptionsPublicURL  = "wss://fstream.binance.com/public/stream"
	fstreamOptionsMarketURL  = "wss://fstream.binance.com/market/stream"
	fstreamOptionsPrivateURL = "wss://fstream.binance.com/private"

	optionsPublicFilter  = "options-public"
	optionsMarketFilter  = "options-market"
	optionsPrivateFilter = "options-private"

	cnlTrade                    = "optionTrade"
	cnlTradeWithUnderlyingAsset = "@optionTrade"
	cnlIndex                    = "!index@arr"
	cnlMarkPrice                = "optionMarkPrice"
	cnlKline                    = "kline"
	cnlTicker                   = "optionTicker"
	cnlTickerWithExpiration     = "@optionTicker@"
	cnlOpenInterest             = "@openInterest@"
	cnlDepth                    = "depth"
	cnlOptionPair               = "option_pair"
	cnlOptionSymbol             = "!optionSymbol" // mar
)

// defaultEOptionsSubscriptions list of default subscription channels
var defaultEOptionsSubscriptions = []string{
	cnlTrade,
	cnlTicker,
	cnlKline,
	cnlDepth,
}

// WsOptionsConnect initiates a websocket connection to coin margined futures websocket
func (e *Exchange) WsOptionsConnect(ctx context.Context, conn websocket.Connection) error {
	err := e.CurrencyPairs.IsAssetEnabled(asset.Options)
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

func (e *Exchange) handleEOptionsSubscriptions(ctx context.Context, conn websocket.Connection, operation string, subscs subscription.List) error {
	if len(subscs) == 0 {
		return common.ErrEmptyParams
	}
	params := &EOptionSubscriptionRequest{
		Method: operation,
		Params: make([]string, 0, len(subscs)),
		ID:     e.MessageSequence(),
	}
	var err error
	params.Params, err = optionsSubscriptionParams(subscs)
	if err != nil {
		return err
	}

	response, err := conn.SendMessageReturnResponse(ctx, request.UnAuth, params.ID, params)
	if err != nil {
		return err
	}
	var resp EOptionsOperationResponse
	if err := json.Unmarshal(response, &resp); err != nil {
		return err
	} else if resp.Error.Code != 0 {
		return fmt.Errorf("err: code: %d, msg: %s", resp.Error.Code, resp.Error.Message)
	}
	if operation == "SUBSCRIBE" {
		return e.Websocket.AddSuccessfulSubscriptions(conn, subscs...)
	}
	return e.Websocket.RemoveSubscriptions(conn, subscs...)
}

// OptionSubscribe sends an european option subscription messages.
func (e *Exchange) OptionSubscribe(ctx context.Context, conn websocket.Connection, subscs subscription.List) error {
	return e.handleEOptionsSubscriptions(ctx, conn, "SUBSCRIBE", subscs)
}

// OptionUnsubscribe unsubscribes an option un-subscription messages.
func (e *Exchange) OptionUnsubscribe(ctx context.Context, conn websocket.Connection, subscs subscription.List) error {
	return e.handleEOptionsSubscriptions(ctx, conn, "UNSUBSCRIBE", subscs)
}

// GenerateEOptionsDefaultSubscriptions generates the default subscription set
func (e *Exchange) GenerateEOptionsDefaultSubscriptions(filter string) (subscription.List, error) {
	switch filter {
	case optionsPublicFilter, optionsMarketFilter:
	case optionsPrivateFilter:
		return subscription.List{}, nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedSubscription, filter)
	}
	if !slices.Contains(e.GetAssetTypes(true), asset.Options) {
		return nil, nil
	}
	if configured, present, err := e.configuredDerivativeSubscriptions(asset.Options, filter); present || err != nil {
		return configured, err
	}
	var subscriptions subscription.List
	pairs, err := e.GetEnabledPairs(asset.Options)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	var channels []string
	for _, ch := range defaultEOptionsSubscriptions {
		switch filter {
		case optionsPublicFilter:
			switch ch {
			case cnlTrade, cnlTradeWithUnderlyingAsset, cnlDepth, cnlTicker, cnlTickerWithExpiration:
				channels = append(channels, ch)
			}
		case optionsMarketFilter:
			switch ch {
			case cnlOptionSymbol, cnlMarkPrice, cnlIndex, cnlOpenInterest, cnlKline:
				channels = append(channels, ch)
			}
		case optionsPrivateFilter:
			// The private connection carries user data events only; it takes no subscriptions.
			return subscription.List{}, nil
		}
	}

	for z := range channels {
		switch channels[z] {
		case cnlTrade, cnlMarkPrice, cnlIndex, cnlTicker, cnlTradeWithUnderlyingAsset:
			subscriptions = append(subscriptions, &subscription.Subscription{
				Channel: channels[z],
				Pairs:   pairs,
				Asset:   asset.Options,
			})
		case cnlKline:
			subscriptions = append(subscriptions, &subscription.Subscription{
				Channel:  cnlKline,
				Pairs:    pairs,
				Asset:    asset.Options,
				Interval: kline.FiveMin,
			})
		case cnlTickerWithExpiration, cnlOpenInterest:
			subscriptions = append(subscriptions, &subscription.Subscription{
				Channel: channels[z],
				Pairs:   pairs,
				Asset:   asset.Options,
				Params: map[string]any{
					"expiration": time.Now().Add(time.Hour * 24 * 5),
				},
			})
		case cnlDepth:
			subscriptions = append(subscriptions, &subscription.Subscription{
				Channel:  cnlDepth,
				Pairs:    pairs,
				Asset:    asset.Options,
				Interval: kline.FiveHundredMilliseconds,
				Params: map[string]any{
					"level": 50, // Valid levels are 10, 20, 50, 100.
				},
			})
		case cnlOptionPair, cnlOptionSymbol:
			subscriptions = append(subscriptions, &subscription.Subscription{
				Channel: cnlOptionPair,
			})
		default:
			return nil, errUnsupportedSubscription
		}
	}
	return subscriptions, nil
}

// GetEOptionsWsAuthStreamKey retrieves the options user-data listen key.
func (e *Exchange) GetEOptionsWsAuthStreamKey(ctx context.Context) (string, error) {
	response, err := e.CreateOptionsListenKey(ctx)
	if err != nil {
		return "", err
	}
	if response == nil {
		return "", common.ErrNoResponse
	}
	return response.ListenKey, nil
}

func (e *Exchange) wsHandleEOptionsData(ctx context.Context, conn websocket.Connection, respRaw []byte) error {
	var envelope WsOptionIncomingResponse
	data := respRaw
	if len(data) == 0 {
		return errEmptyWebsocketResponse
	}
	if data[0] == '{' {
		if err := json.Unmarshal(data, &envelope); err != nil {
			return err
		}
		if len(envelope.Data) != 0 {
			data = envelope.Data
		} else if envelope.ID != 0 {
			return conn.RequireMatchWithData(envelope.ID, respRaw)
		}
	}
	var result WsOptionIncomingResponses
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	if len(result.Instances) == 0 || result.Instances[0] == nil {
		return errEmptyWebsocketResponse
	}
	switch result.Instances[0].EventType {
	case "ACCOUNT_UPDATE", "BALANCE_POSITION_UPDATE", "ORDER_TRADE_UPDATE", "GREEK_UPDATE", "RISK_LEVEL_CHANGE", listenKeyExpiredEvent:
		return e.wsHandleOptionsUserData(ctx, result.Instances[0].EventType, data)
	case "trade":
		return e.processOptionsTradeStream(ctx, data)
	case "indexPrice":
		return e.processOptionsIndexPrice(ctx, data)
	case "24hrTicker":
		return e.processOptionsTicker(ctx, data, result.IsSlice)
	case "markPrice":
		return e.processOptionsMarkPrices(ctx, data)
	case cnlKline:
		return e.processOptionsKline(ctx, data)
	case "openInterest":
		return e.processOptionsOpenInterest(ctx, data)
	case "option_pair":
		return e.processOptionsPair(ctx, data)
	case "depthUpdate":
		return e.processDerivativeDepth(ctx, data, asset.Options, isPartialDepthStream(envelope.Stream))
	case bookTickerStream:
		return e.processOptionsBookTicker(ctx, data)
	case "optionSymbol", "!optionSymbol":
		return e.processOptionsSymbol(ctx, data)
	default:
		return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{Message: string(respRaw)})
	}
}

// processOptionsSymbol represents a new symbol listing stream
func (e *Exchange) processOptionsSymbol(ctx context.Context, data []byte) error {
	var resp WsOptionSymbol
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

// processOptionsPair new symbol listing stream
func (e *Exchange) processOptionsPair(ctx context.Context, data []byte) error {
	var resp WsOptionsNewPair
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

func (e *Exchange) processOptionsOpenInterest(ctx context.Context, data []byte) error {
	var resp []WsOpenInterest
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

func (e *Exchange) processOptionsKline(ctx context.Context, data []byte) error {
	var resp WsOptionsKlineData
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	pair, err := currency.NewPairFromString(resp.KlineData.Symbol)
	if err != nil {
		return err
	}
	interval, err := formatToInterval(resp.KlineData.CandlePeriod)
	if err != nil {
		return err
	}
	var validationIssues string
	if !resp.KlineData.ContractCompleted {
		validationIssues = kline.PartialCandle
	}
	return e.Websocket.DataHandler.Send(ctx, kline.Item{
		Pair:     pair,
		Exchange: e.Name,
		Asset:    asset.Options,
		Interval: interval,
		Candles: []kline.Candle{{
			Time:             resp.KlineData.StartTime.Time(),
			Open:             resp.KlineData.Open.Float64(),
			Close:            resp.KlineData.Close.Float64(),
			High:             resp.KlineData.High.Float64(),
			Low:              resp.KlineData.Low.Float64(),
			Volume:           resp.KlineData.ContractVolume.Float64(),
			ValidationIssues: validationIssues,
		}},
	})
}

func (e *Exchange) processOptionsMarkPrices(ctx context.Context, data []byte) error {
	var resp []WsOptionsMarkPrice
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

func (e *Exchange) processOptionsIndexPrice(ctx context.Context, data []byte) error {
	var resp []*OptionsIndexInfo
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, resp)
}

func (e *Exchange) processOptionsTicker(ctx context.Context, data []byte, isSlice bool) error {
	var resp []OptionsTicker24Hr
	if isSlice {
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
	} else {
		respSingle := OptionsTicker24Hr{}
		if err := json.Unmarshal(data, &respSingle); err != nil {
			return err
		}
		resp = append(resp, respSingle)
	}
	for a := range resp {
		pair, err := currency.NewPairFromString(resp[a].Symbol)
		if err != nil {
			return err
		}
		if err := e.Websocket.DataHandler.Send(ctx, &ticker.Price{
			High:         resp[a].HighPrice.Float64(),
			Low:          resp[a].LowPrice.Float64(),
			BaseVolume:   resp[a].TradingVolume.Float64(),
			QuoteVolume:  resp[a].TradingAmount.Float64(),
			Open:         resp[a].OpeningPrice.Float64(),
			Close:        resp[a].ClosingPrice.Float64(),
			Last:         resp[a].ClosingPrice.Float64(),
			Pair:         pair,
			ExchangeName: e.Name,
			AssetType:    asset.Options,
			LastUpdated:  resp[a].EventTime.Time(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Exchange) processOptionsTradeStream(ctx context.Context, data []byte) error {
	var resp *EOptionsWsTrade
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	pair, err := currency.NewPairFromString(resp.Symbol)
	if err != nil {
		return err
	}
	side := order.Buy
	if resp.Direction == "SELL" || resp.Direction == "-1" {
		side = order.Sell
	}
	return e.Websocket.DataHandler.Send(ctx, trade.Data{
		Exchange:     e.Name,
		CurrencyPair: pair,
		AssetType:    asset.Options,
		Side:         side,
		Price:        resp.Price.Float64(),
		Amount:       resp.Quantity.Float64(),
		Timestamp:    resp.TradeCompletedTime.Time(),
		TID:          strconv.FormatInt(resp.TradeID, 10),
	})
}

var intervalsMap = map[kline.Interval]string{
	// Intervals used by the orderbook depth
	kline.HundredMilliseconds: "100ms", kline.FiveHundredMilliseconds: "500ms",
	kline.ThousandMilliseconds: "1000ms",

	// other intervals
	kline.OneMin: "1m", kline.ThreeMin: "3m", kline.FiveMin: "5m", kline.FifteenMin: "15m",
	kline.ThirtyMin: "30m", kline.OneHour: "1h", kline.TwoHour: "2h", kline.FourHour: "4h",
	kline.SixHour: "6h", kline.TwelveHour: "12h", kline.OneDay: "1d", kline.ThreeDay: "3d", kline.OneWeek: "1w",
}

// optionsSubscriptionParams preserves the case of channel names while lowercasing symbols.
// Expiry tickers use an @ separator even though the SDK's placeholder omits it.
func optionsSubscriptionParams(subscriptions subscription.List) ([]string, error) {
	if len(subscriptions) == 0 {
		return nil, common.ErrEmptyParams
	}
	var streams []string
	for _, sub := range subscriptions {
		switch sub.Channel {
		case cnlIndex, "index", cnlOptionSymbol:
			channel := sub.Channel
			if channel == "index" {
				channel = cnlIndex
			}
			streams = append(streams, channel)
			continue
		}
		if len(sub.Pairs) == 0 {
			return nil, currency.ErrCurrencyPairsEmpty
		}
		for _, pair := range sub.Pairs {
			if pair.IsEmpty() {
				return nil, currency.ErrCurrencyPairEmpty
			}
			symbol := strings.ToLower(pair.String())
			underlying := strings.ToLower(pair.Base.String() + "USDT")
			if value, ok := sub.Params["underlying"].(string); ok && value != "" {
				underlying = strings.ToLower(value)
			}
			switch sub.Channel {
			case cnlTrade, "trade", cnlTicker, tickerStream, bookTickerStream:
				channel := sub.Channel
				if channel == "trade" {
					channel = cnlTrade
				}
				if channel == "ticker" {
					channel = cnlTicker
				}
				streams = append(streams, symbol+"@"+channel)
			case cnlTradeWithUnderlyingAsset, "@trade":
				streams = append(streams, underlying+"@"+cnlTrade)
			case cnlMarkPrice, "@markPrice":
				streams = append(streams, underlying+"@"+cnlMarkPrice)
			case cnlKline:
				interval := sub.Interval
				if interval == 0 {
					interval = kline.FiveMin
				}
				value := getKlineIntervalString(interval)
				if value == "" {
					return nil, kline.ErrInvalidInterval
				}
				streams = append(streams, symbol+"@kline_"+value)
			case cnlTickerWithExpiration, "@ticker@", cnlOpenInterest:
				expiry, ok := sub.Params["expiration"].(time.Time)
				if !ok || expiry.IsZero() {
					parts := strings.Split(pair.Quote.String(), "-")
					if len(parts) < 3 {
						return nil, errExpirationTimeRequired
					}
					var err error
					expiry, err = time.Parse("060102", parts[0])
					if err != nil {
						return nil, fmt.Errorf("%w: %w", errExpirationTimeRequired, err)
					}
				}
				channel := sub.Channel
				if channel == "@ticker@" {
					channel = cnlTickerWithExpiration
				}
				streams = append(streams, underlying+channel+expiry.UTC().Format("060102"))
			case cnlDepth:
				level := "10"
				if value, ok := sub.Params["level"]; ok {
					level = fmt.Sprint(value)
				}
				switch level {
				case "10", "20", "50", "100":
				default:
					return nil, fmt.Errorf("%w: %s", errLimitNumberRequired, level)
				}
				interval := sub.Interval
				if interval == 0 {
					interval = kline.HundredMilliseconds
				}
				switch interval {
				case kline.HundredMilliseconds, kline.FiveHundredMilliseconds, kline.ThousandMilliseconds:
				default:
					return nil, kline.ErrInvalidInterval
				}
				streams = append(streams, symbol+"@depth"+level+"@"+intervalsMap[interval])
			default:
				return nil, fmt.Errorf("%w: %s", errUnsupportedChannel, sub.Channel)
			}
		}
	}
	seen := make(map[string]bool, len(streams))
	unique := streams[:0]
	for _, stream := range streams {
		if !seen[stream] {
			seen[stream] = true
			unique = append(unique, stream)
		}
	}
	return unique, nil
}

func (e *Exchange) processOptionsBookTicker(ctx context.Context, data []byte) error {
	var resp FuturesBookTicker
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	pair, err := currency.NewPairFromString(resp.Symbol)
	if err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, &ticker.Price{ExchangeName: e.Name, AssetType: asset.Options, Pair: pair, Bid: resp.BestBidPrice.Float64(), Ask: resp.BestAskPrice.Float64(), LastUpdated: resp.TransactionTime.Time()})
}

func (e *Exchange) wsHandleOptionsUserData(ctx context.Context, eventType string, data []byte) error {
	var response any
	switch eventType {
	case "ACCOUNT_UPDATE":
		response = new(OptionsAccountUpdate)
	case "BALANCE_POSITION_UPDATE":
		response = new(OptionsBalancePositionUpdate)
	case "ORDER_TRADE_UPDATE":
		return e.processFuturesOrderTradeUpdate(ctx, data, asset.Options)
	case "GREEK_UPDATE":
		response = new(OptionsGreekUpdate)
	case "RISK_LEVEL_CHANGE":
		response = new(OptionsRiskLevelChange)
	case listenKeyExpiredEvent:
		response = new(FuturesListenKeyExpired)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedChannel, eventType)
	}
	if err := json.Unmarshal(data, response); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, response)
}
