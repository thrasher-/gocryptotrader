package hyperliquid

import (
	"fmt"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// ActiveLeverage captures leverage information for active asset data updates.
type ActiveLeverage struct {
	Type   string
	Value  float64
	RawUSD *float64
}

// ActiveAssetDataUpdate represents an active asset data websocket update.
type ActiveAssetDataUpdate struct {
	Address          string
	Pair             currency.Pair
	Asset            asset.Item
	Leverage         ActiveLeverage
	MaxTradeSizes    [2]float64
	AvailableToTrade [2]float64
	MarkPrice        float64
	Timestamp        time.Time
}

// ActiveAssetContextUpdate encapsulates perp asset context websocket updates.
type ActiveAssetContextUpdate struct {
	Pair      currency.Pair
	Asset     asset.Item
	Context   PerpetualAssetContext
	Timestamp time.Time
}

// ActiveSpotAssetContextUpdate encapsulates spot asset context websocket updates.
type ActiveSpotAssetContextUpdate struct {
	Pair      currency.Pair
	Context   SpotAssetContext
	Timestamp time.Time
}

func (e *Exchange) handleActiveAssetData(payload *wsActiveAssetDataPayload) error {
	if payload == nil {
		return errActiveAssetDataPayloadMissing
	}
	if payload.Coin == "" {
		return errActiveAssetDataCoinMissing
	}
	if payload.User == "" {
		return errActiveAssetDataUserMissing
	}
	assetType, pair, err := parseMarketString(payload.Coin)
	if err != nil {
		return err
	}
	markPrice := payload.MarkPrice.Float64()
	maxTrade, err := parseSizedTuple(payload.MaxTradeSizes)
	if err != nil {
		return fmt.Errorf("hyperliquid: parse active asset max trade sizes: %w", err)
	}
	avail, err := parseSizedTuple(payload.AvailableToTrade)
	if err != nil {
		return fmt.Errorf("hyperliquid: parse active asset available to trade: %w", err)
	}
	leverage := ActiveLeverage{}
	if payload.Leverage != nil {
		leverage.Type = strings.ToLower(payload.Leverage.Type)
		leverage.Value = payload.Leverage.Value.Float64()
		if payload.Leverage.RawUSD != nil {
			raw := payload.Leverage.RawUSD.Float64()
			leverage.RawUSD = &raw
		}
	}
	update := ActiveAssetDataUpdate{
		Address:          strings.ToLower(payload.User),
		Pair:             pair,
		Asset:            assetType,
		Leverage:         leverage,
		MaxTradeSizes:    maxTrade,
		AvailableToTrade: avail,
		MarkPrice:        markPrice,
		Timestamp:        e.currentTime(),
	}
	e.storeActiveAssetData(&update)
	e.Websocket.DataHandler <- update
	return nil
}

func (e *Exchange) handleActiveAssetContext(payload *wsActiveAssetContextPayload) error {
	if payload == nil {
		return errActiveAssetContextPayloadMissing
	}
	if payload.Coin == "" {
		return errActiveAssetContextCoinMissing
	}
	assetType, pair, err := parseMarketString(payload.Coin)
	if err != nil {
		return err
	}
	if assetType != asset.PerpetualContract {
		return fmt.Errorf("hyperliquid: unexpected asset for active asset context %s", assetType)
	}
	update := ActiveAssetContextUpdate{
		Pair:      pair,
		Asset:     assetType,
		Context:   payload.Ctx,
		Timestamp: e.currentTime(),
	}
	e.Websocket.DataHandler <- update
	return nil
}

func (e *Exchange) handleActiveSpotAssetContext(payload *wsActiveSpotAssetContextPayload) error {
	if payload == nil {
		return errActiveSpotAssetContextPayloadMissing
	}
	if payload.Coin == "" {
		return errActiveSpotAssetContextCoinMissing
	}
	assetType, pair, err := parseMarketString(payload.Coin)
	if err != nil {
		return err
	}
	if assetType != asset.Spot {
		return fmt.Errorf("hyperliquid: unexpected asset for active spot asset context %s", assetType)
	}
	update := ActiveSpotAssetContextUpdate{
		Pair:      pair,
		Context:   payload.Ctx,
		Timestamp: e.currentTime(),
	}
	e.Websocket.DataHandler <- update
	return nil
}

func parseSizedTuple(values []types.Number) ([2]float64, error) {
	if len(values) != 2 {
		return [2]float64{}, fmt.Errorf("expected sized tuple length 2 got %d", len(values))
	}
	return [2]float64{values[0].Float64(), values[1].Float64()}, nil
}

func (e *Exchange) storeActiveAssetData(update *ActiveAssetDataUpdate) {
	if update == nil || update.Pair.IsEmpty() {
		return
	}
	key := activeAssetDataKey(update.Asset, update.Pair)
	e.activeAssetDataMu.Lock()
	if e.activeAssetData == nil {
		e.activeAssetData = make(map[string]ActiveAssetDataUpdate)
	}
	e.activeAssetData[key] = *update
	e.activeAssetDataMu.Unlock()
}

func activeAssetDataKey(a asset.Item, pair currency.Pair) string {
	return strings.ToLower(a.String()) + ":" + strings.ToUpper(pair.String())
}

// GetLatestActiveAssetData returns the last cached active asset data update for a pair.
func (e *Exchange) GetLatestActiveAssetData(pair currency.Pair, a asset.Item) (*ActiveAssetDataUpdate, error) {
	if pair.IsEmpty() {
		return nil, errPairRequired
	}
	key := activeAssetDataKey(a, pair)
	e.activeAssetDataMu.RLock()
	update, ok := e.activeAssetData[key]
	e.activeAssetDataMu.RUnlock()
	if !ok {
		return nil, errActiveAssetDataNotFound
	}
	clone := update
	return &clone, nil
}
