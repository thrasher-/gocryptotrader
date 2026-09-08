package hyperliquid

import (
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/types"
)

type websocketRequest struct {
	Method       string                `json:"method"`
	Subscription websocketSubscription `json:"subscription"`
}

type websocketSubscription struct {
	Type            string `json:"type"`
	Coin            string `json:"coin,omitempty"`
	Interval        string `json:"interval,omitempty"`
	User            string `json:"user,omitempty"`
	AggregateByTime bool   `json:"aggregateByTime,omitempty"`
}

type websocketSubscriptionResponse struct {
	Method       string                `json:"method"`
	Subscription websocketSubscription `json:"subscription"`
}

type websocketPendingKey struct {
	authenticated bool
	subscription  websocketSubscription
}

type websocketPendingOperation struct {
	method        string
	connection    websocket.Connection
	subscription  *subscription.Subscription
	previousState subscription.State
	done          chan error
}

type websocketEnvelope struct {
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

type websocketAssetContext struct {
	Coin    string          `json:"coin"`
	Context json.RawMessage `json:"ctx"`
}

type websocketUserFills struct {
	IsSnapshot bool            `json:"isSnapshot"`
	User       string          `json:"user"`
	Fills      []websocketFill `json:"fills"`
}

type websocketFill struct {
	Coin          string       `json:"coin"`
	Price         types.Number `json:"px"`
	Size          types.Number `json:"sz"`
	Side          string       `json:"side"`
	Time          types.Time   `json:"time"`
	Hash          string       `json:"hash"`
	OrderID       uint64       `json:"oid"`
	TradeID       uint64       `json:"tid"`
	ClientOrderID *string      `json:"cloid"`
}
