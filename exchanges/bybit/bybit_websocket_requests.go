package bybit

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

// Websocket request operation types
const (
	OutboundTradeConnection  = "PRIVATE_TRADE"
	InboundPrivateConnection = "PRIVATE"
)

// WSCreateOrder creates an order through the websocket connection
func (e *Exchange) WSCreateOrder(ctx context.Context, r *PlaceOrderRequest) (*WebsocketOrderDetails, error) {
	wire, limit, err := prepareWSCreateOrder(r)
	if err != nil {
		return nil, err
	}
	outbound, inbound, err := getWebsocketTradeConnections(e.Websocket.GetConnection, true)
	if err != nil {
		return nil, err
	}
	return e.sendWebsocketTradeRequest(ctx, outbound, inbound, "order.create", wire.OrderLinkID, wire, limit)
}

func prepareWSCreateOrder(r *PlaceOrderRequest) (*PlaceOrderRequest, request.EndpointLimit, error) {
	if r == nil {
		return nil, request.Unset, errNilArgument
	}
	wire := *r
	if err := wire.Validate(); err != nil {
		return nil, request.Unset, err
	}
	// Validate and getWSRateLimitEPLByCategory share the canonical category set.
	limit, _ := getWSRateLimitEPLByCategory(wire.Category)
	if wire.OrderLinkID == "" {
		wire.OrderLinkID = uuid.Must(uuid.NewV7()).String()
	}
	return &wire, limit, nil
}

// WSAmendOrder amends an order through the websocket connection.
func (e *Exchange) WSAmendOrder(ctx context.Context, r *AmendOrderRequest) (*WebsocketOrderDetails, error) {
	wire, limit, err := prepareWSAmendOrder(r)
	if err != nil {
		return nil, err
	}
	outbound, _, err := getWebsocketTradeConnections(e.Websocket.GetConnection, true)
	if err != nil {
		return nil, err
	}
	matchID := wire.OrderLinkID
	if matchID == "" {
		matchID = wire.OrderID
	}
	return e.sendWebsocketTradeRequestUntil(ctx, outbound, "order.amend", matchID, wire, limit, func(update *WebsocketOrderDetails) (bool, error) {
		return websocketAmendOrderConfirmed(wire, update)
	})
}

func prepareWSAmendOrder(r *AmendOrderRequest) (*AmendOrderRequest, request.EndpointLimit, error) {
	if r == nil {
		return nil, request.Unset, errNilArgument
	}
	wire := *r
	if err := wire.Validate(); err != nil {
		return nil, request.Unset, err
	}
	// Validate and getWSRateLimitEPLByCategory share the canonical category set.
	limit, _ := getWSRateLimitEPLByCategory(wire.Category)
	return &wire, limit, nil
}

// WSCancelOrder cancels an order through the websocket connection.
func (e *Exchange) WSCancelOrder(ctx context.Context, r *CancelOrderRequest) (*WebsocketOrderDetails, error) {
	wire, limit, err := prepareWSCancelOrder(r)
	if err != nil {
		return nil, err
	}
	outbound, _, err := getWebsocketTradeConnections(e.Websocket.GetConnection, true)
	if err != nil {
		return nil, err
	}
	matchID := wire.OrderLinkID
	if matchID == "" {
		matchID = wire.OrderID
	}
	return e.sendWebsocketTradeRequestUntil(ctx, outbound, "order.cancel", matchID, wire, limit, websocketCancelOrderConfirmed)
}

func prepareWSCancelOrder(r *CancelOrderRequest) (*CancelOrderRequest, request.EndpointLimit, error) {
	if r == nil {
		return nil, request.Unset, errNilArgument
	}
	wire := *r
	if err := wire.Validate(); err != nil {
		return nil, request.Unset, err
	}
	// Validate and getWSRateLimitEPLByCategory share the canonical category set.
	limit, _ := getWSRateLimitEPLByCategory(wire.Category)
	return &wire, limit, nil
}

func getWebsocketTradeConnections(connectionGetter func(any) (websocket.Connection, error), requireInbound bool) (outbound, inbound websocket.Connection, err error) {
	outbound, err = connectionGetter(OutboundTradeConnection)
	if err != nil {
		return nil, nil, err
	}
	if !requireInbound {
		return outbound, nil, nil
	}
	inbound, err = connectionGetter(InboundPrivateConnection)
	if err != nil {
		return nil, nil, err
	}
	return outbound, inbound, nil
}

type websocketOrderUpdateStore struct {
	mu        sync.Mutex
	listeners map[string]*websocketOrderUpdateListener
}

type websocketOrderUpdateListener struct {
	mu      sync.Mutex
	updates []WebsocketOrderDetails
	notify  chan struct{}
	closed  bool
}

func (s *websocketOrderUpdateStore) subscribe(matchID string) (listener *websocketOrderUpdateListener, unsubscribe func(), err error) {
	if matchID == "" {
		return nil, nil, errOrderLinkIDMissing
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listeners == nil {
		s.listeners = make(map[string]*websocketOrderUpdateListener)
	}
	if _, exists := s.listeners[matchID]; exists {
		return nil, nil, fmt.Errorf("%w for %q", errWebsocketOrderUpdateListenerExists, matchID)
	}
	listener = &websocketOrderUpdateListener{notify: make(chan struct{}, 1)}
	s.listeners[matchID] = listener
	return listener, func() {
		s.mu.Lock()
		if s.listeners[matchID] == listener {
			delete(s.listeners, matchID)
		}
		s.mu.Unlock()
		listener.mu.Lock()
		listener.closed = true
		listener.updates = nil
		listener.mu.Unlock()
	}, nil
}

func (s *websocketOrderUpdateStore) publish(update *WebsocketOrderDetails) {
	if update == nil {
		return
	}
	s.mu.Lock()
	var listener *websocketOrderUpdateListener
	if update.OrderLinkID != "" {
		listener = s.listeners[update.OrderLinkID]
	}
	if listener == nil && update.OrderID != "" {
		listener = s.listeners[update.OrderID]
	}
	s.mu.Unlock()
	if listener == nil {
		return
	}
	listener.mu.Lock()
	defer listener.mu.Unlock()
	if listener.closed {
		return
	}
	listener.updates = append(listener.updates, *update)
	select {
	case listener.notify <- struct{}{}:
	default:
	}
}

func websocketAmendOrderConfirmed(amendRequest *AmendOrderRequest, update *WebsocketOrderDetails) (bool, error) {
	switch update.OrderStatus {
	case "Filled", "Cancelled", "Rejected", "Deactivated", "PartiallyFilledCanceled":
		return false, fmt.Errorf("%w: amendment ended with %s", errWebsocketOrderTerminalState, update.OrderStatus)
	}
	if amendRequest.OrderImpliedVolatility != "" {
		requested, err := strconv.ParseFloat(amendRequest.OrderImpliedVolatility, 64)
		if err != nil {
			return false, fmt.Errorf("%w: %w", errInvalidOrderImpliedVolatility, err)
		}
		if requested != update.OrderImpliedVolatility.Float64() {
			return false, nil
		}
	}
	return (amendRequest.OrderQuantity == 0 || amendRequest.OrderQuantity == update.Quantity.Float64()) &&
		(amendRequest.Price == 0 || amendRequest.Price == update.Price.Float64()) &&
		(amendRequest.TriggerPrice == 0 || amendRequest.TriggerPrice == update.TriggerPrice.Float64()) &&
		(amendRequest.TakeProfitPrice == nil || *amendRequest.TakeProfitPrice == update.TakeProfit.Float64()) &&
		(amendRequest.StopLossPrice == nil || *amendRequest.StopLossPrice == update.StopLoss.Float64()) &&
		(amendRequest.TakeProfitLimitPrice == 0 || amendRequest.TakeProfitLimitPrice == update.TakeProfitLimitPrice.Float64()) &&
		(amendRequest.StopLossLimitPrice == 0 || amendRequest.StopLossLimitPrice == update.StopLossLimitPrice.Float64()) &&
		websocketAmendStringConfirmed(amendRequest.TakeProfitTriggerBy, update.TakeProfitTriggerBy) &&
		websocketAmendStringConfirmed(amendRequest.StopLossTriggerBy, update.StopLossTriggerBy) &&
		websocketAmendStringConfirmed(amendRequest.TriggerPriceType, update.TriggerBy) &&
		(amendRequest.TPSLMode == "" || amendRequest.TPSLMode == update.TakeProfitStopLossMode), nil
}

func websocketAmendStringConfirmed(requested, observed string) bool {
	return requested == "" || requested == observed
}

func websocketOrderRejection(update *WebsocketOrderDetails) error {
	if update.OrderStatus == "Rejected" {
		return fmt.Errorf("%w: status %s", errWebsocketOrderRejected, update.OrderStatus)
	}
	switch update.RejectReason {
	case "", "EC_NoError", "EC_PerCancelRequest":
		return nil
	default:
		return fmt.Errorf("%w: %s", errWebsocketOrderRejected, update.RejectReason)
	}
}

func websocketCancelOrderConfirmed(update *WebsocketOrderDetails) (bool, error) {
	switch update.OrderStatus {
	case "Cancelled", "Deactivated", "PartiallyFilledCanceled":
		return true, nil
	case "Filled", "Rejected":
		return false, fmt.Errorf("%w: cancellation ended with %s", errWebsocketOrderTerminalState, update.OrderStatus)
	default:
		return false, nil
	}
}

// sendWebsocketTradeRequestUntil submits an asynchronous websocket trade request and waits for an operation-specific private order update.
func (e *Exchange) sendWebsocketTradeRequestUntil(ctx context.Context, outbound websocket.Connection, op, matchID string, payload any, limit request.EndpointLimit, confirmed func(*WebsocketOrderDetails) (bool, error)) (*WebsocketOrderDetails, error) {
	if confirmed == nil {
		return nil, errNilArgument
	}
	listener, unsubscribe, err := e.websocketOrderUpdates.subscribe(matchID)
	if err != nil {
		return nil, err
	}
	defer unsubscribe()

	if _, err := e.sendWebsocketTradeAcknowledgement(ctx, outbound, op, payload, limit); err != nil {
		return nil, err
	}

	wait := e.WebsocketResponseMaxLimit
	if wait <= 0 {
		wait = exchange.DefaultWebsocketResponseMaxLimit
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-listener.notify:
			listener.mu.Lock()
			updates := slices.Clone(listener.updates)
			listener.updates = listener.updates[:0]
			listener.mu.Unlock()
			for i := range updates {
				if err := websocketOrderRejection(&updates[i]); err != nil {
					return nil, err
				}
				isConfirmed, err := confirmed(&updates[i])
				if err != nil {
					return nil, err
				}
				if isConfirmed {
					return &updates[i], nil
				}
			}
		case <-timer.C:
			return nil, fmt.Errorf("%s %w %v", e.Name, websocket.ErrSignatureTimeout, matchID)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// sendWebsocketTradeRequest submits a websocket trade request and waits for its uniquely correlated private order update.
func (e *Exchange) sendWebsocketTradeRequest(ctx context.Context, outbound, inbound websocket.Connection, op, matchID string, payload any, limit request.EndpointLimit) (*WebsocketOrderDetails, error) {
	// Set up a listener to wait for the response to come back from the inbound connection. The request is sent through
	// the outbound trade connection, the response can come back through the inbound private connection before the
	// outbound connection sends its acknowledgement.
	matchCtx, cancelMatch := context.WithCancel(ctx)
	ch, err := inbound.MatchReturnResponses(matchCtx, matchID, 1)
	if err != nil {
		cancelMatch()
		return nil, err
	}
	defer func() {
		cancelMatch()
		for range ch {
			continue
		}
	}()

	_, err = e.sendWebsocketTradeAcknowledgement(ctx, outbound, op, payload, limit)
	if err != nil {
		return nil, err
	}

	inResp := <-ch // Blocking read is acceptable; channel has a built in timeout already
	if inResp.Err != nil {
		return nil, inResp.Err
	}

	if len(inResp.Responses) != 1 {
		return nil, fmt.Errorf("expected 1 matched response, received %d", len(inResp.Responses))
	}

	var ret WebsocketOrderResponse
	if err := json.Unmarshal(inResp.Responses[0], &ret); err != nil {
		return nil, err
	}

	if len(ret.OrderDetails) != 1 {
		return nil, fmt.Errorf("expected 1 order detail, received %d", len(ret.OrderDetails))
	}

	if err := websocketOrderRejection(&ret.OrderDetails[0]); err != nil {
		return nil, err
	}

	return &ret.OrderDetails[0], nil
}

// sendWebsocketTradeAcknowledgement submits a websocket trade request and validates its request-scoped acknowledgement.
func (e *Exchange) sendWebsocketTradeAcknowledgement(ctx context.Context, outbound websocket.Connection, op string, payload any, limit request.EndpointLimit) (*WebsocketConfirmation, error) {
	requestID := e.MessageID()
	outResp, err := outbound.SendMessageReturnResponse(ctx, limit, requestID, WebsocketGeneralPayload{
		RequestID: requestID,
		Header:    map[string]string{"X-BAPI-TIMESTAMP": strconv.FormatInt(time.Now().UnixMilli(), 10)},
		Operation: op,
		Arguments: []any{payload},
	})
	if err != nil {
		return nil, err
	}

	var confirmation WebsocketConfirmation
	if err := json.Unmarshal(outResp, &confirmation); err != nil {
		return nil, err
	}
	if confirmation.RetCode != 0 {
		return nil, fmt.Errorf("code:%d, info:%v message:%s", confirmation.RetCode, retCode[confirmation.RetCode], confirmation.RetMsg)
	}
	return &confirmation, nil
}

var retCode = map[int64]string{
	10404: "either op type is not found or category is not correct/supported",
	10429: "request exceeds rate limit",
	20006: "reqId is duplicated",
	10016: "internal server error",
	10019: "ws trade service is restarting, please reconnect",
	20003: "too frequent requests under the same session",
	10403: "exceed IP rate limit. 3000 requests per second per IP",
}
