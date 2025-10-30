package hyperliquid

import (
	"fmt"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/types"
)

const (
	hyperliquidQualifier                 = "hyperliquid"
	channelSeparator                     = "::"
	tickerIdentifierAll                  = "ALL"
	assetKeyPerpetualContract            = "perp"
	assetKeySpot                         = "spot"
	websocketChannelAllMids              = "allMids"
	websocketChannelL2Book               = "l2Book"
	websocketChannelTrades               = "trades"
	websocketChannelCandle               = "candle"
	websocketChannelSubscriptionResponse = "subscriptionResponse"
	websocketChannelError                = "error"
	websocketChannelBbo                  = "bbo"
	websocketChannelActiveAssetCtx       = "activeAssetCtx"
	websocketChannelActiveSpotAssetCtx   = "activeSpotAssetCtx"
	websocketChannelActiveAssetData      = "activeAssetData"
	websocketChannelUserEvents           = "user"
	websocketChannelUserFills            = "userFills"
	websocketChannelOrderUpdates         = "orderUpdates"
	websocketChannelUserFundings         = "userFundings"
	websocketChannelUserLedgerUpdates    = "userNonFundingLedgerUpdates"
	websocketChannelWebData2             = "webData2"

	hyperliquidActiveAssetDataChannel = "hyperliquidActiveAssetData"
)

type wsMessage struct {
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

type wsAllMidsData struct {
	Mids map[string]types.Number `json:"mids"`
}

type wsL2Level struct {
	Price types.Number `json:"px"`
	Size  types.Number `json:"sz"`
	Count int64        `json:"n"`
}

type wsL2BookData struct {
	Coin   string        `json:"coin"`
	Levels [][]wsL2Level `json:"levels"`
	Time   types.Time    `json:"time"`
}

type wsBBOData struct {
	Coin string        `json:"coin"`
	Time types.Time    `json:"time"`
	BBO  [2]*wsL2Level `json:"bbo"`
}

type wsTrade struct {
	Coin  string       `json:"coin"`
	Side  string       `json:"side"`
	Price types.Number `json:"px"`
	Size  types.Number `json:"sz"`
	Time  types.Time   `json:"time"`
	Hash  string       `json:"hash"`
	TID   string       `json:"tid"`
	Users []string     `json:"users"`
}

type wsTradesData []wsTrade

type wsCandleData struct {
	OpenTime   types.Time   `json:"t"`
	CloseTime  types.Time   `json:"T"`
	Symbol     string       `json:"s"`
	Interval   string       `json:"i"`
	Open       types.Number `json:"o"`
	Close      types.Number `json:"c"`
	High       types.Number `json:"h"`
	Low        types.Number `json:"l"`
	Volume     types.Number `json:"v"`
	TradeCount int64        `json:"n"`
}

type wsLeverage struct {
	Type   string        `json:"type"`
	Value  types.Number  `json:"value"`
	RawUSD *types.Number `json:"rawUsd,omitempty"`
}

type wsActiveAssetDataPayload struct {
	User             string         `json:"user"`
	Coin             string         `json:"coin"`
	Leverage         *wsLeverage    `json:"leverage"`
	MaxTradeSizes    []types.Number `json:"maxTradeSzs"`
	AvailableToTrade []types.Number `json:"availableToTrade"`
	MarkPrice        types.Number   `json:"markPx"`
}

type wsActiveAssetContextPayload struct {
	Coin string                `json:"coin"`
	Ctx  PerpetualAssetContext `json:"ctx"`
}

type wsActiveSpotAssetContextPayload struct {
	Coin string           `json:"coin"`
	Ctx  SpotAssetContext `json:"ctx"`
}

type wsUserEventsData struct {
	Fills []UserFill `json:"fills,omitempty"`
}

type wsUserFillsPayload struct {
	User       string     `json:"user"`
	IsSnapshot bool       `json:"isSnapshot"`
	Fills      []UserFill `json:"fills"`
}

type wsOrderUpdatesPayload struct {
	User     string             `json:"user"`
	Statuses []OrderStatusEntry `json:"statuses"`
}

type wsUserFundingPayload struct {
	User string `json:"user"`
	UserFundingHistoryEntry
}

type wsUserNonFundingLedgerPayload struct {
	User string `json:"user"`
	UserNonFundingLedgerEntry
}

type wsWebData2Payload struct {
	User string         `json:"user"`
	Data map[string]any `json:"data"`
}

type wsSubscriptionAck struct {
	Method       string         `json:"method"`
	Subscription map[string]any `json:"subscription"`
	Error        *string        `json:"error,omitempty"`
}

type wsSubscriptionPayload struct {
	Type     string `json:"type"`
	Coin     string `json:"coin,omitempty"`
	Dex      string `json:"dex,omitempty"`
	Interval string `json:"interval,omitempty"`
	User     string `json:"user,omitempty"`
}

type wsSubscriptionRequest struct {
	Method       string                `json:"method"`
	Subscription wsSubscriptionPayload `json:"subscription"`
}

func formatQualifiedChannel(channel string, a asset.Item, identifier string) (string, error) {
	key, err := assetToKey(a)
	if err != nil {
		return "", err
	}
	if identifier == "" {
		identifier = "GLOBAL"
	}
	return strings.Join([]string{hyperliquidQualifier, channel, key, strings.ToUpper(identifier)}, channelSeparator), nil
}

func parseQualifiedChannel(qualified string) (channel, identifier string, err error) {
	parts := strings.Split(qualified, channelSeparator)
	if len(parts) != 4 || parts[0] != hyperliquidQualifier {
		return "", "", fmt.Errorf("hyperliquid: invalid qualified channel %s", qualified)
	}
	channel = parts[1]
	if _, err = assetFromKey(parts[2]); err != nil {
		return "", "", err
	}
	identifier = parts[3]
	return channel, identifier, nil
}

func assetToKey(a asset.Item) (string, error) {
	switch a {
	case asset.PerpetualContract:
		return assetKeyPerpetualContract, nil
	case asset.Spot:
		return assetKeySpot, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnsupportedAsset, a)
	}
}

func assetFromKey(k string) (asset.Item, error) {
	switch strings.ToLower(k) {
	case assetKeyPerpetualContract:
		return asset.PerpetualContract, nil
	case assetKeySpot:
		return asset.Spot, nil
	default:
		return asset.Item(0), fmt.Errorf("%w key %s", errUnsupportedAsset, k)
	}
}

func parseMarketString(market string) (asset.Item, currency.Pair, error) {
	if market == "" {
		return asset.Item(0), currency.Pair{}, errEmptyMarketIdentifier
	}
	if strings.Contains(market, "/") {
		pair, err := currency.NewPairFromString(market)
		if err != nil {
			return asset.Item(0), currency.Pair{}, fmt.Errorf("hyperliquid: parse market %s: %w", market, err)
		}
		return asset.Spot, pair, nil
	}
	base := currency.NewCode(strings.ToUpper(market))
	return asset.PerpetualContract, currency.NewPair(base, currency.USDC), nil
}
