package hyperliquid

import (
	"fmt"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
)

func payloadKeyFromPayload(payload *wsSubscriptionPayload) string {
	if payload == nil {
		return ""
	}
	segments := []string{
		strings.ToLower(payload.Type),
		strings.ToLower(payload.Coin),
		strings.ToLower(payload.Interval),
		strings.ToLower(payload.User),
		strings.ToLower(payload.Dex),
	}
	return strings.Join(segments, "|")
}

func payloadDescription(payload *wsSubscriptionPayload) string {
	if payload == nil {
		return ""
	}
	parts := []string{"type=" + payload.Type}
	if payload.Coin != "" {
		parts = append(parts, "coin="+payload.Coin)
	}
	if payload.Interval != "" {
		parts = append(parts, "interval="+payload.Interval)
	}
	if payload.User != "" {
		parts = append(parts, "user="+payload.User)
	}
	if payload.Dex != "" {
		parts = append(parts, "dex="+payload.Dex)
	}
	return strings.Join(parts, ", ")
}

func ackSubscriptionMapToPayload(m map[string]any) (wsSubscriptionPayload, error) {
	if len(m) == 0 {
		return wsSubscriptionPayload{}, errSubscriptionAckMissingPayload
	}
	typeValue, ok := m["type"].(string)
	if !ok || typeValue == "" {
		return wsSubscriptionPayload{}, errSubscriptionAckMissingType
	}
	payload := wsSubscriptionPayload{Type: typeValue}
	if coin, ok := m["coin"].(string); ok {
		payload.Coin = coin
	}
	if interval, ok := m["interval"].(string); ok {
		payload.Interval = interval
	}
	if user, ok := m["user"].(string); ok {
		payload.User = user
	}
	if dex, ok := m["dex"].(string); ok {
		payload.Dex = dex
	}
	return payload, nil
}

func subscriptionFromPayload(payload *wsSubscriptionPayload) (*subscription.Subscription, error) {
	if payload == nil {
		return nil, errSubscriptionPayloadMissing
	}
	typ := strings.ToLower(payload.Type)
	switch typ {
	case strings.ToLower(websocketChannelAllMids):
		sub := &subscription.Subscription{
			Channel: subscription.TickerChannel,
			Asset:   asset.PerpetualContract,
		}
		qualified, err := formatQualifiedChannel(subscription.TickerChannel, asset.PerpetualContract, tickerIdentifierAll)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelL2Book):
		assetType, pair, err := parseMarketString(payload.Coin)
		if err != nil {
			return nil, err
		}
		identifier, err := marketIdentifier(pair, assetType)
		if err != nil {
			return nil, err
		}
		sub := &subscription.Subscription{
			Channel: subscription.OrderbookChannel,
			Asset:   assetType,
			Pairs:   currency.Pairs{pair},
		}
		qualified, err := formatQualifiedChannel(subscription.OrderbookChannel, assetType, identifier)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelTrades):
		assetType, pair, err := parseMarketString(payload.Coin)
		if err != nil {
			return nil, err
		}
		identifier, err := marketIdentifier(pair, assetType)
		if err != nil {
			return nil, err
		}
		sub := &subscription.Subscription{
			Channel: subscription.AllTradesChannel,
			Asset:   assetType,
			Pairs:   currency.Pairs{pair},
		}
		qualified, err := formatQualifiedChannel(subscription.AllTradesChannel, assetType, identifier)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelCandle):
		assetType, pair, err := parseMarketString(payload.Coin)
		if err != nil {
			return nil, err
		}
		interval, err := candleIntervalFromString(payload.Interval)
		if err != nil {
			return nil, err
		}
		identifier, err := marketIdentifier(pair, assetType)
		if err != nil {
			return nil, err
		}
		sub := &subscription.Subscription{
			Channel:  subscription.CandlesChannel,
			Asset:    assetType,
			Pairs:    currency.Pairs{pair},
			Interval: interval,
		}
		qualified, err := formatQualifiedChannel(subscription.CandlesChannel, assetType, identifier)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelBbo):
		assetType, pair, err := parseMarketString(payload.Coin)
		if err != nil {
			return nil, err
		}
		identifier, err := marketIdentifier(pair, assetType)
		if err != nil {
			return nil, err
		}
		sub := &subscription.Subscription{
			Channel: websocketChannelBbo,
			Asset:   assetType,
			Pairs:   currency.Pairs{pair},
		}
		qualified, err := formatQualifiedChannel(websocketChannelBbo, assetType, identifier)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelActiveAssetData):
		assetType, pair, err := parseMarketString(payload.Coin)
		if err != nil {
			return nil, err
		}
		identifier, err := marketIdentifier(pair, assetType)
		if err != nil {
			return nil, err
		}
		if payload.User == "" {
			return nil, errActiveAssetDataSubMissingUser
		}
		user := strings.ToLower(payload.User)
		sub := &subscription.Subscription{
			Channel:       hyperliquidActiveAssetDataChannel,
			Asset:         assetType,
			Pairs:         currency.Pairs{pair},
			Authenticated: true,
			Params:        map[string]any{"user": user},
		}
		qualified, err := formatQualifiedChannel(hyperliquidActiveAssetDataChannel, assetType, identifier)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelActiveAssetCtx):
		assetType, pair, err := parseMarketString(payload.Coin)
		if err != nil {
			return nil, err
		}
		identifier, err := marketIdentifier(pair, assetType)
		if err != nil {
			return nil, err
		}
		sub := &subscription.Subscription{
			Channel: websocketChannelActiveAssetCtx,
			Asset:   assetType,
			Pairs:   currency.Pairs{pair},
		}
		qualified, err := formatQualifiedChannel(websocketChannelActiveAssetCtx, assetType, identifier)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelActiveSpotAssetCtx):
		assetType, pair, err := parseMarketString(payload.Coin)
		if err != nil {
			return nil, err
		}
		identifier, err := marketIdentifier(pair, assetType)
		if err != nil {
			return nil, err
		}
		sub := &subscription.Subscription{
			Channel: websocketChannelActiveSpotAssetCtx,
			Asset:   assetType,
			Pairs:   currency.Pairs{pair},
		}
		qualified, err := formatQualifiedChannel(websocketChannelActiveSpotAssetCtx, assetType, identifier)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelUserEvents):
		if payload.User == "" {
			return nil, errUserEventsSubscriptionMissingUser
		}
		user := strings.ToLower(payload.User)
		sub := &subscription.Subscription{
			Channel:       websocketChannelUserEvents,
			Asset:         asset.PerpetualContract,
			Authenticated: true,
			Params:        map[string]any{"user": user},
		}
		qualified, err := formatQualifiedChannel(websocketChannelUserEvents, asset.PerpetualContract, user)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelUserFills):
		if payload.User == "" {
			return nil, errUserFillsSubscriptionMissingUser
		}
		user := strings.ToLower(payload.User)
		sub := &subscription.Subscription{
			Channel:       websocketChannelUserFills,
			Asset:         asset.PerpetualContract,
			Authenticated: true,
			Params:        map[string]any{"user": user},
		}
		qualified, err := formatQualifiedChannel(websocketChannelUserFills, asset.PerpetualContract, user)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelOrderUpdates):
		if payload.User == "" {
			return nil, errOrderUpdatesSubscriptionMissingUser
		}
		user := strings.ToLower(payload.User)
		sub := &subscription.Subscription{
			Channel:       websocketChannelOrderUpdates,
			Asset:         asset.PerpetualContract,
			Authenticated: true,
			Params:        map[string]any{"user": user},
		}
		qualified, err := formatQualifiedChannel(websocketChannelOrderUpdates, asset.PerpetualContract, user)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelUserFundings):
		if payload.User == "" {
			return nil, errUserFundingsSubscriptionMissingUser
		}
		user := strings.ToLower(payload.User)
		sub := &subscription.Subscription{
			Channel:       websocketChannelUserFundings,
			Asset:         asset.PerpetualContract,
			Authenticated: true,
			Params:        map[string]any{"user": user},
		}
		qualified, err := formatQualifiedChannel(websocketChannelUserFundings, asset.PerpetualContract, user)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelUserLedgerUpdates):
		if payload.User == "" {
			return nil, errUserLedgerSubscriptionMissingUser
		}
		user := strings.ToLower(payload.User)
		sub := &subscription.Subscription{
			Channel:       websocketChannelUserLedgerUpdates,
			Asset:         asset.PerpetualContract,
			Authenticated: true,
			Params:        map[string]any{"user": user},
		}
		qualified, err := formatQualifiedChannel(websocketChannelUserLedgerUpdates, asset.PerpetualContract, user)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	case strings.ToLower(websocketChannelWebData2):
		if payload.User == "" {
			return nil, errWebDataSubscriptionMissingUser
		}
		user := strings.ToLower(payload.User)
		sub := &subscription.Subscription{
			Channel:       websocketChannelWebData2,
			Asset:         asset.PerpetualContract,
			Authenticated: true,
			Params:        map[string]any{"user": user},
		}
		qualified, err := formatQualifiedChannel(websocketChannelWebData2, asset.PerpetualContract, user)
		if err != nil {
			return nil, err
		}
		sub.QualifiedChannel = qualified
		return sub, nil
	default:
		return nil, fmt.Errorf("hyperliquid: unsupported subscription payload %s", payload.Type)
	}
}

func (e *Exchange) findSubscriptionByPayload(payload *wsSubscriptionPayload) *subscription.Subscription {
	key := payloadKeyFromPayload(payload)
	if key == "" {
		return nil
	}
	subs := e.Websocket.GetSubscriptions()
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		channel, identifier, err := parseQualifiedChannel(sub.QualifiedChannel)
		if err != nil {
			continue
		}
		candidate, err := e.buildSubscriptionPayload(sub, channel, identifier)
		if err != nil {
			continue
		}
		if payloadKeyFromPayload(&candidate) == key {
			return sub
		}
	}
	return nil
}

func candleIntervalFromString(interval string) (kline.Interval, error) {
	switch strings.ToLower(interval) {
	case "1m":
		return kline.OneMin, nil
	case "5m":
		return kline.FiveMin, nil
	case "15m":
		return kline.FifteenMin, nil
	case "1h":
		return kline.OneHour, nil
	case "4h":
		return kline.FourHour, nil
	case "1d":
		return kline.OneDay, nil
	default:
		return kline.Interval(0), fmt.Errorf("hyperliquid: unsupported kline interval %s", interval)
	}
}
