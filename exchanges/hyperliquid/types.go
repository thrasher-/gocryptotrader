package hyperliquid

import "github.com/thrasher-corp/gocryptotrader/types"

// MetaResponse defines the universe of perpetual markets returned by meta/info endpoints.
type MetaResponse struct {
	Universe []PerpetualMarket `json:"universe"`
}

// PerpetualMarket describes a single perpetual contract listing.
type PerpetualMarket struct {
	Name         string `json:"name"`
	SzDecimals   int64  `json:"szDecimals"`
	MaxLeverage  int64  `json:"maxLeverage"`
	MarginTable  int64  `json:"marginTableId"`
	OnlyIsolated bool   `json:"onlyIsolated"`
	IsDelisted   bool   `json:"isDelisted"`
}

// PerpetualAssetContext contains mark and volume information for a perpetual contract.
type PerpetualAssetContext struct {
	Funding        types.Number   `json:"funding"`
	OpenInterest   types.Number   `json:"openInterest"`
	PrevDayPrice   types.Number   `json:"prevDayPx"`
	DayNotionalVol types.Number   `json:"dayNtlVlm"`
	Premium        *types.Number  `json:"premium"`
	OraclePrice    types.Number   `json:"oraclePx"`
	MarkPrice      types.Number   `json:"markPx"`
	MidPrice       *types.Number  `json:"midPx"`
	ImpactPrices   []types.Number `json:"impactPxs"`
	DayBaseVolume  types.Number   `json:"dayBaseVlm"`
}

// OrderbookSnapshot represents the L2 orderbook state for a given instrument.
type OrderbookSnapshot struct {
	Coin   string        `json:"coin"`
	Time   types.Time    `json:"time"`
	Levels [][]BookLevel `json:"levels"`
}

// BookLevel represents a price level within an L2 snapshot.
type BookLevel struct {
	Price      types.Number `json:"px"`
	Size       types.Number `json:"sz"`
	OrderCount int64        `json:"n"`
}

// RecentTrade represents a single trade returned from the recentTrades endpoint.
type RecentTrade struct {
	Coin  string       `json:"coin"`
	Side  string       `json:"side"`
	Price types.Number `json:"px"`
	Size  types.Number `json:"sz"`
	Time  types.Time   `json:"time"`
	TID   int64        `json:"tid"`
	Hash  string       `json:"hash"`
	Users []string     `json:"users"`
}

// CandleSnapshot represents a single candle returned from the candleSnapshot endpoint.
type CandleSnapshot struct {
	OpenTime  types.Time   `json:"t"`
	CloseTime types.Time   `json:"T"`
	Symbol    string       `json:"s"`
	Interval  string       `json:"i"`
	Open      types.Number `json:"o"`
	Close     types.Number `json:"c"`
	High      types.Number `json:"h"`
	Low       types.Number `json:"l"`
	Volume    types.Number `json:"v"`
	Trades    int64        `json:"n"`
}

// SpotMetaResponse contains Hyperliquid spot universe and token metadata.
type SpotMetaResponse struct {
	Universe []SpotMarket `json:"universe"`
	Tokens   []SpotToken  `json:"tokens"`
}

// SpotMarket identifies a tradable spot market and its component tokens.
type SpotMarket struct {
	Tokens      []int  `json:"tokens"`
	Name        string `json:"name"`
	Index       int64  `json:"index"`
	IsCanonical bool   `json:"isCanonical"`
}

// SpotToken describes a single spot token listed on Hyperliquid.
type SpotToken struct {
	Name        string `json:"name"`
	SZDecimals  int64  `json:"szDecimals"`
	WeiDecimals int64  `json:"weiDecimals"`
	Index       int64  `json:"index"`
	TokenID     string `json:"tokenId"`
	IsCanonical bool   `json:"isCanonical"`
}
