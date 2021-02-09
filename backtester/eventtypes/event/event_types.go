package event

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
)

// Event is the underlying event across all actions that occur for the backtester
// Data, fill, order events all contain the base event and store important and
// consistent information
type Event struct {
	Exchange     string         `json:"exchange"`
	Time         time.Time      `json:"timestamp"`
	Interval     kline.Interval `json:"interval-size"`
	CurrencyPair currency.Pair  `json:"pair"`
	AssetType    asset.Item     `json:"asset"`
	Why          string         `json:"why"`
}