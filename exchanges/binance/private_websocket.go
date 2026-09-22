package binance

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

const coinPrivateFilter = "coin-private"

func (e *Exchange) privateStreamSetup(a asset.Item) *websocket.ConnectionSetup {
	setup := &websocket.ConnectionSetup{
		ResponseCheckTimeout:     e.Config.WebsocketResponseCheckTimeout,
		ResponseMaxLimit:         e.Config.WebsocketResponseMaxLimit,
		RateLimit:                request.NewWeightedRateLimitByDuration(250 * time.Millisecond),
		SubscriptionsNotRequired: true,
	}
	switch a {
	case asset.USDTMarginedFutures:
		setup.URL, setup.MessageFilter, setup.Handler = fstreamPrivateURL, usdtmPrivateFilter, e.wsHandleFuturesData
	case asset.CoinMarginedFutures:
		setup.URL, setup.MessageFilter, setup.Handler = binanceCFuturesWebsocketURL, coinPrivateFilter, e.wsHandleCFuturesData
	case asset.Options:
		setup.URL, setup.MessageFilter, setup.Handler = fstreamOptionsPrivateURL, optionsPrivateFilter, e.wsHandleEOptionsData
	}
	setup.Connector = func(ctx context.Context, conn websocket.Connection) error {
		return e.connectPrivateStream(ctx, conn, a)
	}
	handler := setup.Handler
	setup.Handler = func(ctx context.Context, conn websocket.Connection, data []byte) error {
		if err := handler(ctx, conn, data); err != nil {
			return err
		}
		if event, _ := jsonparser.GetString(data, "e"); event == listenKeyExpiredEvent && conn != nil {
			// An expired listen key stops delivering events even if the socket remains open.
			return conn.Shutdown()
		}
		return nil
	}
	return setup
}

func (e *Exchange) connectPrivateStream(ctx context.Context, conn websocket.Connection, a asset.Item) error {
	if err := e.CurrencyPairs.IsAssetEnabled(a); err != nil {
		return err
	}
	var response *ListenKeyResponse
	var err error
	switch a {
	case asset.USDTMarginedFutures:
		response, err = e.CreateUFuturesListenKey(ctx)
	case asset.CoinMarginedFutures:
		response, err = e.CreateCFuturesListenKey(ctx)
	case asset.Options:
		response, err = e.CreateOptionsListenKey(ctx)
	default:
		return fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	if err != nil {
		return err
	}
	if response == nil || response.ListenKey == "" {
		return common.ErrNoResponse
	}
	e.setListenKey(a, response.ListenKey)
	conn.SetURL(strings.TrimSuffix(conn.GetURL(), "/") + "/ws/" + response.ListenKey)
	dialer := &gws.Dialer{HandshakeTimeout: e.Config.HTTPTimeout, Proxy: http.ProxyFromEnvironment}
	if err := conn.Dial(ctx, dialer, http.Header{}, nil); err != nil {
		return err
	}
	conn.SetupPingHandler(request.UnAuth, websocket.PingHandler{UseGorillaHandler: true, MessageType: gws.PongMessage, Delay: pingDelay})
	e.maintainPrivateStream(ctx, conn, a, 30*time.Minute)
	return nil
}

func (e *Exchange) maintainPrivateStream(ctx context.Context, conn websocket.Connection, a asset.Item, interval time.Duration) {
	e.maintainListenKey(ctx, conn, a.String(), interval, func(ctx context.Context) error {
		var err error
		switch a {
		case asset.USDTMarginedFutures:
			_, err = e.KeepUFuturesListenKeyAlive(ctx)
		case asset.CoinMarginedFutures:
			_, err = e.KeepCFuturesListenKeyAlive(ctx)
		case asset.Options:
			err = e.KeepOptionsListenKeyAlive(ctx)
		}
		return err
	})
}

func (e *Exchange) dialListenKeyStream(ctx context.Context, conn websocket.Connection, response *ListenKeyResponse, product string, renew func(context.Context) error) error {
	if response == nil || response.ListenKey == "" {
		return common.ErrNoResponse
	}
	conn.SetURL(strings.TrimSuffix(conn.GetURL(), "/") + "/ws/" + response.ListenKey)
	dialer := &gws.Dialer{HandshakeTimeout: e.Config.HTTPTimeout, Proxy: http.ProxyFromEnvironment}
	if err := conn.Dial(ctx, dialer, http.Header{}, nil); err != nil {
		return err
	}
	conn.SetupPingHandler(request.UnAuth, websocket.PingHandler{UseGorillaHandler: true, MessageType: gws.PongMessage, Delay: pingDelay})
	e.maintainListenKey(ctx, conn, product, 30*time.Minute, renew)
	return nil
}

func (e *Exchange) maintainListenKey(ctx context.Context, conn websocket.Connection, product string, interval time.Duration, renew func(context.Context) error) {
	shutdown := e.Websocket.ShutdownC
	e.Websocket.Wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-shutdown:
				return
			case <-ticker.C:
				if err := renew(ctx); err != nil {
					_ = e.Websocket.DataHandler.Send(ctx, fmt.Errorf("%s %s user-data keepalive: %w", e.Name, product, err))
					// Let the manager reconnect and obtain a fresh key after a renewal failure.
					_ = conn.Shutdown()
					return
				}
			}
		}
	})
}
