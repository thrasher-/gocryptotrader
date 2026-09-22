package binance

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
)

// Explicit product and wildcard subscriptions replace that product's defaults,
// including an explicitly disabled list. Clone entries so routing never changes
// the saved configuration or another connection's subscription set.
func (e *Exchange) configuredDerivativeSubscriptions(a asset.Item, filter string) (subscription.List, bool, error) {
	var configured bool
	var result subscription.List
	for _, saved := range e.Features.Subscriptions {
		if saved.Asset != a && saved.Asset != asset.All {
			continue
		}
		configured = true
		if !saved.Enabled || saved.Authenticated {
			continue
		}
		sub := saved.Clone()
		sub.Asset = a
		if len(sub.Pairs) == 0 {
			var err error
			sub.Pairs, err = e.GetEnabledPairs(a)
			if err != nil {
				return nil, true, err
			}
		}
		if len(sub.Pairs) == 0 {
			continue
		}
		channel := strings.TrimPrefix(sub.Channel, "@")
		switch sub.Channel {
		case subscription.AllTradesChannel:
			channel = aggTradeStream
		case subscription.CandlesChannel:
			channel = cnlKline
		case subscription.OrderbookChannel:
			channel = cnlDepth
		}
		if a == asset.Options {
			switch saved.Channel {
			case subscription.AllTradesChannel, "trade":
				sub.Channel = cnlTrade
			case subscription.TickerChannel:
				sub.Channel = cnlTicker
			case subscription.CandlesChannel:
				sub.Channel = cnlKline
			case subscription.OrderbookChannel:
				sub.Channel = cnlDepth
			}
			public := sub.Channel == cnlTrade || sub.Channel == cnlTradeWithUnderlyingAsset || sub.Channel == cnlDepth || sub.Channel == cnlTicker || sub.Channel == cnlTickerWithExpiration || sub.Channel == bookTickerStream
			if (filter == optionsPublicFilter) != public {
				continue
			}
			if sub.Channel == cnlKline && sub.Interval == 0 {
				sub.Interval = kline.OneMin
			}
			if sub.Channel == cnlDepth && sub.Levels != 0 {
				if sub.Params == nil {
					sub.Params = make(map[string]any)
				}
				sub.Params["level"] = sub.Levels
			}
			if _, err := optionsSubscriptionParams(subscription.List{sub}); err != nil {
				return nil, true, err
			}
			result = append(result, sub)
			continue
		}
		kind, _, _ := strings.Cut(channel, "@")
		public := strings.HasPrefix(kind, "depth") || kind == "rpiDepth" || kind == "bookTicker" || kind == "!bookTicker"
		if a == asset.USDTMarginedFutures && ((filter == usdtmPublicFilter) != public) {
			continue
		}
		if strings.HasPrefix(channel, "!") {
			switch kind {
			case "!bookTicker", "!ticker", "!miniTicker", "!markPrice", "!forceOrder", "!contractInfo", "!assetIndex":
			default:
				return nil, true, fmt.Errorf("%w: %s", subscription.ErrNotSupported, sub.Channel)
			}
			sub.QualifiedChannel = channel
			result = append(result, sub)
			continue
		}
		switch kind {
		case "rpiDepth":
			if a != asset.USDTMarginedFutures {
				return nil, true, subscription.ErrNotSupported
			}
			if sub.Levels != 0 {
				return nil, true, subscription.ErrInvalidLevel
			}
			if sub.Interval != 0 && sub.Interval != kline.FiveHundredMilliseconds {
				return nil, true, subscription.ErrInvalidInterval
			}
			channel = "rpiDepth@500ms"
		case aggTradeStream, tickerStream, "miniTicker", bookTickerStream, "markPrice", "forceOrder", "compositeIndex", "assetIndex", "indexPrice":
		case "depth":
			if sub.Levels != 0 && sub.Levels != 5 && sub.Levels != 10 && sub.Levels != 20 {
				return nil, true, subscription.ErrInvalidLevel
			}
			if sub.Levels != 0 {
				channel += strconv.Itoa(sub.Levels)
			}
			if sub.Interval == 0 {
				sub.Interval = kline.HundredMilliseconds
			}
			if sub.Interval != kline.HundredMilliseconds && sub.Interval != kline.TwoHundredAndFiftyMilliseconds && sub.Interval != kline.FiveHundredMilliseconds {
				return nil, true, subscription.ErrInvalidInterval
			}
			if !strings.Contains(channel, "@") {
				channel += "@" + sub.Interval.Short()
			}
		case "kline", "continuousKline", "indexPriceKline", "markPriceKline":
			if sub.Interval == 0 {
				sub.Interval = kline.OneMin
			}
			interval := getKlineIntervalString(sub.Interval)
			if interval == "" {
				return nil, true, kline.ErrInvalidInterval
			}
			channel += "_" + interval
		default:
			return nil, true, fmt.Errorf("%w: %s", subscription.ErrNotSupported, sub.Channel)
		}
		for _, pair := range sub.Pairs {
			if pair.IsEmpty() {
				return nil, true, currency.ErrCurrencyPairEmpty
			}
			symbol, err := e.FormatSymbol(pair, a)
			if err != nil {
				return nil, true, err
			}
			symbol = strings.ToLower(symbol)
			if kind == "continuousKline" {
				underlying, contract, _ := strings.Cut(symbol, "_")
				contractType, _ := sub.Params["contractType"].(string)
				if contractType == "" {
					if contract != "" && contract != "perp" {
						return nil, true, errContractTypeIsRequired
					}
					contractType = "perpetual"
				}
				symbol = underlying + "_" + strings.ToLower(contractType)
			} else if a == asset.CoinMarginedFutures && (kind == "indexPrice" || kind == "indexPriceKline") {
				symbol, _, _ = strings.Cut(symbol, "_")
			}
			item := sub.Clone()
			item.Pairs = currency.Pairs{pair}
			item.QualifiedChannel = symbol + "@" + channel
			result = append(result, item)
		}
	}
	return result, configured, nil
}
