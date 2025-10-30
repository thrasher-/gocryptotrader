package hyperliquid

import (
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
)

var defaultSubscriptions = subscription.List{
	{Enabled: true, Asset: asset.All, Channel: subscription.TickerChannel},
	{Enabled: true, Asset: asset.All, Channel: subscription.OrderbookChannel, Interval: kline.HundredMilliseconds},
	{Enabled: true, Asset: asset.All, Channel: subscription.AllTradesChannel},
	{Enabled: true, Asset: asset.All, Channel: subscription.CandlesChannel, Interval: kline.OneMin},
	{Enabled: true, Asset: asset.All, Channel: websocketChannelBbo},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: hyperliquidActiveAssetDataChannel},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: websocketChannelActiveAssetCtx},
	{Enabled: true, Asset: asset.Spot, Channel: websocketChannelActiveSpotAssetCtx},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: websocketChannelUserEvents, Authenticated: true},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: websocketChannelUserFills, Authenticated: true},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: websocketChannelOrderUpdates, Authenticated: true},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: websocketChannelUserFundings, Authenticated: true},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: websocketChannelUserLedgerUpdates, Authenticated: true},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: websocketChannelWebData2, Authenticated: true},
}
