package binance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/buger/jsonparser"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var (
	errInvalidListenTokenValidity = errors.New("listen token validity must be a whole number of milliseconds within 24 hours")
	errListenTokenRequired        = errors.New("listen token is required")
)

// MarginListenTokenRequest selects a cross or isolated margin account. Zero
// Validity uses Binance's 24-hour default; IsIsolated preserves explicit false.
type MarginListenTokenRequest struct {
	Symbol     currency.Pair
	IsIsolated *bool
	Validity   time.Duration
}

// MarginListenTokenSubscriptionResponse contains the subscription lifetime.
type MarginListenTokenSubscriptionResponse struct {
	SubscriptionID uint64     `json:"subscriptionId"`
	ExpirationTime types.Time `json:"expirationTime"`
}

// WsSubscribeMarginListenToken subscribes or extends a margin user stream.
// Binance accepts this method on unauthenticated WebSocket API sessions.
func (e *Exchange) WsSubscribeMarginListenToken(token string) (*MarginListenTokenSubscriptionResponse, error) {
	if token == "" {
		return nil, errListenTokenRequired
	}
	var response *MarginListenTokenSubscriptionResponse
	return response, e.SendWsRequest("userDataStream.subscribe.listenToken", map[string]any{"listenToken": token}, &response)
}

// MarginLevelStatusChange reports the cross-margin account's margin-call state.
type MarginLevelStatusChange struct {
	UserDataEvent
	MarginLevel types.Number `json:"l"`
	Status      string       `json:"s"`
}

// MarginLiabilityChange reports borrowing, repayment and interest changes.
type MarginLiabilityChange struct {
	UserDataEvent
	Asset     currency.Code `json:"a"`
	Type      string        `json:"t"`
	Principal types.Number  `json:"p"`
	Interest  types.Number  `json:"i"`
}

// MarginRiskStreamSetup provides the separate cross-margin risk stream for
// explicit registration with the WebSocket manager.
func (e *Exchange) MarginRiskStreamSetup() *websocket.ConnectionSetup {
	return &websocket.ConnectionSetup{
		URL:                      "wss://margin-stream.binance.com",
		MessageFilter:            "margin-risk-private",
		Connector:                e.WsMarginRiskConnect,
		Handler:                  e.WsHandleMarginRiskData,
		SubscriptionsNotRequired: true,
		ResponseCheckTimeout:     e.Config.WebsocketResponseCheckTimeout,
		ResponseMaxLimit:         e.Config.WebsocketResponseMaxLimit,
		RateLimit:                request.NewWeightedRateLimitByDuration(250 * time.Millisecond),
	}
}

// WsMarginRiskConnect obtains and renews the dedicated margin listen key.
func (e *Exchange) WsMarginRiskConnect(ctx context.Context, conn websocket.Connection) error {
	response, err := e.CreateMarginListenKey(ctx)
	if err != nil {
		return err
	}
	return e.dialListenKeyStream(ctx, conn, response, "margin risk", func(ctx context.Context) error {
		return e.KeepMarginListenKeyAlive(ctx, response.ListenKey)
	})
}

// WsHandleMarginRiskData preserves the documented risk fields and closes an
// expired connection so the manager can acquire a new key.
func (e *Exchange) WsHandleMarginRiskData(ctx context.Context, conn websocket.Connection, raw []byte) error {
	event, err := jsonparser.GetString(raw, "e")
	if err != nil {
		return err
	}
	var response any
	switch event {
	case "MARGIN_LEVEL_STATUS_CHANGE":
		response = new(MarginLevelStatusChange)
	case "USER_LIABILITY_CHANGE":
		response = new(MarginLiabilityChange)
	case listenKeyExpiredEvent:
		response = new(FuturesListenKeyExpired)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedChannel, event)
	}
	if err := json.Unmarshal(raw, response); err != nil {
		return err
	}
	if err := e.Websocket.DataHandler.Send(ctx, response); err != nil {
		return err
	}
	if event == listenKeyExpiredEvent && conn != nil {
		return conn.Shutdown()
	}
	return nil
}
