package strategies

import (
	"github.com/thrasher-corp/gocryptotrader/backtester/data"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/portfolio"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/signal"
)

type Handler interface {
	Name() string
	OnSignal(data.Handler, portfolio.Handler) (signal.SignalEvent, error)
	SetCustomSettings(map[string]interface{}) error
	SetDefaults()
}

const errNotFound = "strategy %v not found"