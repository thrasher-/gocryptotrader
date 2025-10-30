package hyperliquid

import (
	"context"
	"fmt"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
)

func (e *Exchange) generateSubscriptions() (subscription.List, error) {
	var subs subscription.List
	for _, template := range e.Features.Subscriptions {
		if template == nil || !template.Enabled {
			continue
		}
		switch template.Channel {
		case subscription.TickerChannel:
			sub := template.Clone()
			sub.Asset = asset.PerpetualContract
			qualified, err := formatQualifiedChannel(subscription.TickerChannel, asset.PerpetualContract, tickerIdentifierAll)
			if err != nil {
				return nil, err
			}
			sub.QualifiedChannel = qualified
			subs = append(subs, sub)
		case subscription.OrderbookChannel, subscription.AllTradesChannel, subscription.CandlesChannel, websocketChannelBbo, hyperliquidActiveAssetDataChannel:
			assets := e.assetsForSubscription(template)
			for _, a := range assets {
				if template.Channel == hyperliquidActiveAssetDataChannel && a != asset.PerpetualContract {
					continue
				}
				pairs, err := e.GetEnabledPairs(a)
				if err != nil || len(pairs) == 0 {
					if template.Asset == asset.All {
						continue
					}
					return nil, err
				}
				for _, pair := range pairs {
					identifier, err := marketIdentifier(pair, a)
					if err != nil {
						return nil, err
					}
					sub := template.Clone()
					sub.Asset = a
					sub.Pairs = currency.Pairs{pair}
					if template.Channel == hyperliquidActiveAssetDataChannel {
						addr, addrErr := e.accountAddressLower()
						if addrErr != nil {
							continue
						}
						sub.Authenticated = true
						sub.Params = map[string]any{"user": addr}
					}
					qualified, err := formatQualifiedChannel(sub.Channel, a, identifier)
					if err != nil {
						return nil, err
					}
					sub.QualifiedChannel = qualified
					subs = append(subs, sub)
				}
			}
		case websocketChannelActiveAssetCtx, websocketChannelActiveSpotAssetCtx:
			assets := e.assetsForSubscription(template)
			for _, a := range assets {
				pPairs, err := e.GetEnabledPairs(a)
				if err != nil || len(pPairs) == 0 {
					if template.Asset == asset.All {
						continue
					}
					return nil, err
				}
				for _, pair := range pPairs {
					identifier, err := marketIdentifier(pair, a)
					if err != nil {
						return nil, err
					}
					sub := template.Clone()
					sub.Asset = a
					sub.Pairs = currency.Pairs{pair}
					qualified, err := formatQualifiedChannel(sub.Channel, a, identifier)
					if err != nil {
						return nil, err
					}
					sub.QualifiedChannel = qualified
					subs = append(subs, sub)
				}
			}
		case websocketChannelUserEvents,
			websocketChannelUserFills,
			websocketChannelOrderUpdates,
			websocketChannelUserFundings,
			websocketChannelUserLedgerUpdates,
			websocketChannelWebData2:
			addr, err := e.accountAddressLower()
			if err != nil {
				if template.Enabled {
					continue
				}
				return nil, err
			}
			sub := template.Clone()
			sub.Asset = asset.PerpetualContract
			sub.Authenticated = true
			sub.Params = map[string]any{"user": addr}
			qualified, err := formatQualifiedChannel(sub.Channel, asset.PerpetualContract, addr)
			if err != nil {
				return nil, err
			}
			sub.QualifiedChannel = qualified
			subs = append(subs, sub)
		default:
			return nil, fmt.Errorf("hyperliquid: unsupported subscription channel %s", template.Channel)
		}
	}
	return subs, nil
}

func (e *Exchange) assetsForSubscription(sub *subscription.Subscription) asset.Items {
	if sub == nil {
		return nil
	}
	if sub.Asset == asset.All {
		return e.GetAssetTypes(false)
	}
	return asset.Items{sub.Asset}
}

func (e *Exchange) accountAddressLower() (string, error) {
	if addr := strings.ToLower(e.accountAddr); addr != "" {
		return addr, nil
	}
	creds, err := e.GetCredentials(context.Background())
	if err != nil {
		return "", err
	}
	if trimmed := strings.ToLower(creds.ClientID); trimmed != "" {
		return trimmed, nil
	}
	if trimmed := strings.ToLower(creds.Key); trimmed != "" {
		return trimmed, nil
	}
	if creds.Secret != "" {
		wallet, err := newWalletFromHex(creds.Secret)
		if err == nil {
			return strings.ToLower(wallet.hexAddress()), nil
		}
	}
	return "", errCredentialsMissingAccountAddress
}
