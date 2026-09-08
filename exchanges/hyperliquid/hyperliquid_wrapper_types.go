package hyperliquid

import (
	"github.com/thrasher-corp/gocryptotrader/currency"
)

type pairMapping struct {
	pair         currency.Pair
	coin         string
	dex          string
	assetID      uint64
	sizeDecimals uint64
	maxLeverage  uint64
	onlyIsolated bool
}
