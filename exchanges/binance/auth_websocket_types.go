package binance

import (
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// WsAPIRequest is a JSON WebSocket API request.
type WsAPIRequest struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// WsAPIResponse is a JSON WebSocket API response envelope.
type WsAPIResponse struct {
	ID         json.RawMessage `json:"id"`
	Status     int64           `json:"status"`
	Result     json.RawMessage `json:"result"`
	Error      json.RawMessage `json:"error"`
	RateLimits []RateLimitItem `json:"rateLimits,omitempty"`
}

// WsEventStreamTerminated identifies the user-data subscription that ended.
type WsEventStreamTerminated struct {
	SubscriptionID int64      `json:"-"`
	EventType      string     `json:"e"`
	EventTime      types.Time `json:"E"`
}

// WsExternalLockUpdate reports an external change to locked Spot funds.
type WsExternalLockUpdate struct {
	EventType       string        `json:"e"`
	EventTime       types.Time    `json:"E"`
	Asset           currency.Code `json:"a"`
	Delta           types.Number  `json:"d"`
	TransactionTime types.Time    `json:"T"`
}
