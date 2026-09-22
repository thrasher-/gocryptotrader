package binance

import "github.com/thrasher-corp/gocryptotrader/types"

// SpotBlockTradeStream retains the separate block-trade ID namespace.
type SpotBlockTradeStream struct {
	EventType    string       `json:"e"`
	EventTime    types.Time   `json:"E"`
	Symbol       string       `json:"s"`
	TradeID      uint64       `json:"t"`
	Price        types.Number `json:"p"`
	Quantity     types.Number `json:"q"`
	TradeTime    types.Time   `json:"T"`
	IsBuyerMaker bool         `json:"m"`
}

// SpotReferencePriceStream distinguishes an unavailable price from a zero price.
type SpotReferencePriceStream struct {
	EventType      string        `json:"e"`
	Symbol         string        `json:"s"`
	ReferencePrice *types.Number `json:"r"`
	Timestamp      types.Time    `json:"t"`
}

// FuturesRPIDepth contains changes including RPI orders. These updates have a
// different liquidity universe and must not modify the ordinary order book.
type FuturesRPIDepth FuturesDepthOrderbook
