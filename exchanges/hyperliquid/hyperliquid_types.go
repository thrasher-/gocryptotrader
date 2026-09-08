package hyperliquid

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// Exchange implements exchange.IBotExchange and provides Hyperliquid API access.
type Exchange struct {
	exchange.Base
	pairMappingsMu         sync.RWMutex
	pairMappingsFetchMu    sync.Mutex
	pairMappings           map[asset.Item][]pairMapping
	pairMappingMisses      map[string]time.Time
	authorityValidationMu  sync.Mutex
	authorityValidationKey authorityValidationKey
	authorityValidated     bool
	websocketPendingMu     sync.Mutex
	websocketPending       map[websocketPendingKey]*websocketPendingOperation
	lastNonce              atomic.Uint64
}

// PerpetualMetadata contains metadata for Hyperliquid perpetual markets.
type PerpetualMetadata struct {
	Universe        []PerpetualAssetMetadata `json:"universe"`
	CollateralToken uint64                   `json:"collateralToken"`
	MarginTables    []json.RawMessage        `json:"marginTables"`
}

// PerpetualDEX describes one builder-deployed perpetual DEX.
type PerpetualDEX struct {
	Name         string `json:"name"`
	FullName     string `json:"fullName"`
	Deployer     string `json:"deployer"`
	FeeRecipient string `json:"feeRecipient"`
}

// PerpetualAssetMetadata contains metadata for one perpetual market.
type PerpetualAssetMetadata struct {
	Name          string `json:"name"`
	SizeDecimals  uint64 `json:"szDecimals"`
	MaxLeverage   uint64 `json:"maxLeverage"`
	MarginTableID uint64 `json:"marginTableId"`
	OnlyIsolated  bool   `json:"onlyIsolated"`
	IsDelisted    bool   `json:"isDelisted"`
}

// SpotMetadata contains metadata for Hyperliquid spot markets and tokens.
type SpotMetadata struct {
	Universe []SpotAssetMetadata `json:"universe"`
	Tokens   []SpotTokenMetadata `json:"tokens"`
}

// SpotAssetMetadata contains metadata for one spot market.
type SpotAssetMetadata struct {
	Tokens      []uint64 `json:"tokens"`
	Name        string   `json:"name"`
	Index       uint64   `json:"index"`
	IsCanonical bool     `json:"isCanonical"`
}

// SpotTokenMetadata contains metadata for one spot token.
type SpotTokenMetadata struct {
	Name                    string          `json:"name"`
	SizeDecimals            uint64          `json:"szDecimals"`
	WeiDecimals             uint64          `json:"weiDecimals"`
	Index                   uint64          `json:"index"`
	TokenID                 string          `json:"tokenId"`
	IsCanonical             bool            `json:"isCanonical"`
	EVMContract             json.RawMessage `json:"evmContract"`
	FullName                *string         `json:"fullName"`
	DeployerTradingFeeShare types.Number    `json:"deployerTradingFeeShare"`
}

// PerpetualAssetContext contains current market data for one perpetual market.
type PerpetualAssetContext struct {
	Funding           types.Number   `json:"funding"`
	OpenInterest      types.Number   `json:"openInterest"`
	PreviousDayPrice  types.Number   `json:"prevDayPx"`
	DayNotionalVolume types.Number   `json:"dayNtlVlm"`
	Premium           types.Number   `json:"premium"`
	OraclePrice       types.Number   `json:"oraclePx"`
	MarkPrice         types.Number   `json:"markPx"`
	MidPrice          types.Number   `json:"midPx"`
	ImpactPrices      []types.Number `json:"impactPxs"`
	DayBaseVolume     types.Number   `json:"dayBaseVlm"`
}

// SpotAssetContext contains current market data for one spot market.
type SpotAssetContext struct {
	PreviousDayPrice  types.Number `json:"prevDayPx"`
	DayNotionalVolume types.Number `json:"dayNtlVlm"`
	MarkPrice         types.Number `json:"markPx"`
	MidPrice          types.Number `json:"midPx"`
	CirculatingSupply types.Number `json:"circulatingSupply"`
	TotalSupply       types.Number `json:"totalSupply"`
	Coin              string       `json:"coin"`
	DayBaseVolume     types.Number `json:"dayBaseVlm"`
}

// PerpetualMetadataAndAssetContexts contains perpetual metadata and aligned market contexts.
type PerpetualMetadataAndAssetContexts struct {
	Metadata      PerpetualMetadata
	AssetContexts []PerpetualAssetContext
}

// FundingRateRecord contains one hourly perpetual funding rate.
type FundingRateRecord struct {
	Coin        string       `json:"coin"`
	FundingRate types.Number `json:"fundingRate"`
	Premium     types.Number `json:"premium"`
	Time        types.Time   `json:"time"`
}

// SpotMetadataAndAssetContexts contains spot metadata and current market contexts.
type SpotMetadataAndAssetContexts struct {
	Metadata      SpotMetadata
	AssetContexts []SpotAssetContext
}

// L2BookRequest contains optional aggregation controls for an L2 book request.
type L2BookRequest struct {
	Coin               string
	SignificantFigures *uint64
	Mantissa           *uint64
}

// L2Book contains a complete Hyperliquid L2 book snapshot.
type L2Book struct {
	Coin   string      `json:"coin"`
	Levels [][]L2Level `json:"levels"`
	Time   types.Time  `json:"time"`
}

// L2Level contains one aggregated orderbook price level.
type L2Level struct {
	Price      types.Number `json:"px"`
	Size       types.Number `json:"sz"`
	OrderCount uint64       `json:"n"`
}

// RecentTrade contains one public trade reported by Hyperliquid.
type RecentTrade struct {
	Coin    string       `json:"coin"`
	Side    string       `json:"side"`
	Price   types.Number `json:"px"`
	Size    types.Number `json:"sz"`
	Time    types.Time   `json:"time"`
	Hash    string       `json:"hash"`
	TradeID uint64       `json:"tid"`
	Users   []string     `json:"users"`
}

// CandleRequest contains parameters for a candle snapshot request.
type CandleRequest struct {
	Coin      string
	Interval  kline.Interval
	StartTime time.Time
	EndTime   time.Time
}

type candleSnapshotRequest struct {
	Coin      string `json:"coin"`
	Interval  string `json:"interval"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
}

// Candle contains one Hyperliquid OHLCV interval.
type Candle struct {
	OpenTime   types.Time   `json:"t"`
	CloseTime  types.Time   `json:"T"`
	Symbol     string       `json:"s"`
	Interval   string       `json:"i"`
	Open       types.Number `json:"o"`
	Close      types.Number `json:"c"`
	High       types.Number `json:"h"`
	Low        types.Number `json:"l"`
	Volume     types.Number `json:"v"`
	TradeCount uint64       `json:"n"`
}

type infoRequest struct {
	Type      string  `json:"type"`
	DEX       string  `json:"dex,omitempty"`
	Coin      string  `json:"coin,omitempty"`
	User      string  `json:"user,omitempty"`
	Vault     string  `json:"vaultAddress,omitempty"`
	OrderID   any     `json:"oid,omitempty"`
	StartTime int64   `json:"startTime,omitempty"`
	EndTime   int64   `json:"endTime,omitempty"`
	NSigFigs  *uint64 `json:"nSigFigs,omitempty"`
	Mantissa  *uint64 `json:"mantissa,omitempty"`
	Request   any     `json:"req,omitempty"`
}
