package binance

import (
	"context"
	"fmt"
	"time"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

// PortfolioMarginStreamSetup can be registered with the WebSocket manager by
// portfolio-margin clients. Account eligibility cannot be inferred from enabled
// assets, so the ordinary futures connections do not automatically open it.
func (e *Exchange) PortfolioMarginStreamSetup() *websocket.ConnectionSetup {
	return &websocket.ConnectionSetup{
		URL:                      "wss://fstream.binance.com/pm",
		MessageFilter:            "portfolio-margin-private",
		Connector:                e.WsPortfolioMarginConnect,
		Handler:                  e.WsHandlePortfolioMarginData,
		SubscriptionsNotRequired: true,
		ResponseCheckTimeout:     e.Config.WebsocketResponseCheckTimeout,
		ResponseMaxLimit:         e.Config.WebsocketResponseMaxLimit,
		RateLimit:                request.NewWeightedRateLimitByDuration(250 * time.Millisecond),
	}
}

// WsPortfolioMarginConnect obtains a dedicated PAPI listen key and renews it
// until this connection is cancelled or a renewal is rejected.
func (e *Exchange) WsPortfolioMarginConnect(ctx context.Context, conn websocket.Connection) error {
	response, err := e.CreatePortfolioMarginListenKey(ctx)
	if err != nil {
		return err
	}
	return e.dialListenKeyStream(ctx, conn, response, "portfolio margin", func(ctx context.Context) error {
		_, err := e.KeepPortfolioMarginListenKeyAlive(ctx)
		return err
	})
}

// PortfolioMarginProStreamSetup registers the classic portfolio margin account
// stream separately from PAPI and ordinary futures streams.
func (e *Exchange) PortfolioMarginProStreamSetup() *websocket.ConnectionSetup {
	setup := e.PortfolioMarginStreamSetup()
	setup.URL = "wss://fstream.binance.com/pm-classic"
	setup.MessageFilter = "portfolio-margin-pro-private"
	setup.Connector = e.WsPortfolioMarginProConnect
	return setup
}

// WsPortfolioMarginProConnect uses the USD-M listen key required by PM Pro.
func (e *Exchange) WsPortfolioMarginProConnect(ctx context.Context, conn websocket.Connection) error {
	response, err := e.CreateUFuturesListenKey(ctx)
	if err != nil {
		return err
	}
	return e.dialListenKeyStream(ctx, conn, response, "portfolio margin pro", func(ctx context.Context) error {
		_, err := e.KeepUFuturesListenKeyAlive(ctx)
		return err
	})
}

// WsHandlePortfolioMarginData preserves every event field, including fs, which
// distinguishes UM and CM events sharing this authenticated connection.
func (e *Exchange) WsHandlePortfolioMarginData(ctx context.Context, conn websocket.Connection, raw []byte) error {
	var event struct {
		EventType string          `json:"e"`
		EventTime json.RawMessage `json:"E"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return err
	}
	var response any
	switch event.EventType {
	case "ACCOUNT_UPDATE":
		response = new(WSBalanceAndPositionUpdate)
	case "ACCOUNT_CONFIG_UPDATE":
		response = new(FuturesAccountConfigUpdate)
	case "ORDER_TRADE_UPDATE":
		response = new(FuturesOrderTradeUpdate)
	case "ALGO_UPDATE":
		response = new(PortfolioMarginAlgoUpdate)
	case "CONDITIONAL_ORDER_TRADE_UPDATE":
		response = new(PortfolioMarginConditionalOrderUpdate)
	case "balanceUpdate":
		response = new(PortfolioMarginBalanceUpdate)
	case "outboundAccountPosition":
		response = new(PortfolioMarginAccountPosition)
	case "executionReport":
		response = new(WsOrderUpdateData)
	case "liabilityChange":
		response = new(PortfolioMarginLiabilityChange)
	case "openOrderLoss":
		response = new(PortfolioMarginOpenOrderLoss)
	case "RISK_LEVEL_CHANGE", "riskLevelChange":
		response = new(PortfolioMarginRiskLevelChange)
	case "PM_PRO_ACCOUNT_UPDATE":
		response = new(PortfolioMarginProAccountUpdate)
	case listenKeyExpiredEvent:
		response = new(FuturesListenKeyExpired)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedChannel, event.EventType)
	}
	if err := json.Unmarshal(raw, response); err != nil {
		return err
	}
	if err := e.Websocket.DataHandler.Send(ctx, response); err != nil {
		return err
	}
	if event.EventType == listenKeyExpiredEvent && conn != nil {
		return conn.Shutdown()
	}
	return nil
}
