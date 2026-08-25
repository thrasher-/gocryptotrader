package kucoin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
)

type wsConnectTestConnection struct {
	websocket.Connection
	dialErr     error
	dialValues  url.Values
	url         string
	dialCalls   int
	setURLCalls int
	pingCalls   int
	pingLimit   request.EndpointLimit
	pingHandler websocket.PingHandler
}

func (c *wsConnectTestConnection) Dial(_ context.Context, _ *gws.Dialer, _ http.Header, values url.Values) error {
	c.dialCalls++
	c.dialValues = values
	return c.dialErr
}

func (c *wsConnectTestConnection) SetupPingHandler(limit request.EndpointLimit, handler websocket.PingHandler) {
	c.pingCalls++
	c.pingLimit = limit
	c.pingHandler = handler
}

func (c *wsConnectTestConnection) SetURL(endpoint string) {
	c.setURLCalls++
	c.url = endpoint
}

func (c *wsConnectTestConnection) GetURL() string {
	return c.url
}

func newWsConnectTestExchange(t *testing.T, response string) (ex *Exchange, requestPaths <-chan string) {
	t.Helper()
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		_, err := w.Write([]byte(response))
		assert.NoError(t, err, "response write should not error")
	}))
	t.Cleanup(server.Close)

	ex = new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL must configure the test endpoint")
	return ex, requests
}

func TestWsConnect(t *testing.T) {
	const token = "instance-token-canary"

	t.Run("instance server error", func(t *testing.T) {
		ex, requests := newWsConnectTestExchange(t, `{`)
		conn := new(wsConnectTestConnection)
		require.Error(t, ex.WsConnect(t.Context(), conn), "WsConnect must return the instance server error")
		assert.Equal(t, publicBullets, <-requests, "GetInstanceServers should request the public endpoint")
		assert.Zero(t, conn.dialCalls, "Dial should not run after an instance server error")
	})

	t.Run("no instance servers", func(t *testing.T) {
		ex, _ := newWsConnectTestExchange(t, `{"code":"200000","data":{"token":"`+token+`","instanceServers":[]}}`)
		conn := new(wsConnectTestConnection)
		err := ex.WsConnect(t.Context(), conn)
		require.EqualError(t, err, "no websocket instance server found", "WsConnect must reject an empty instance server list")
		assert.Zero(t, conn.dialCalls, "Dial should not run without an instance server")
	})

	t.Run("endpoint override diagnostics", func(t *testing.T) {
		const (
			oldEndpoint = "wss://old-user-token:old-password-token@old.example.com/old-path-token?signature=old-query-token#old-fragment-token"
			newEndpoint = "wss://new-user-token:new-password-token@new.example.com/new-path-token?signature=new-query-token#new-fragment-token"
		)
		ex, _ := newWsConnectTestExchange(t, fmt.Sprintf(`{"code":"200000","data":{"token":%q,"instanceServers":[{"endpoint":%q,"pingInterval":250}]}}`, token, newEndpoint))
		ex.Name += "-ws-connect-diagnostic-canary"
		conn := &wsConnectTestConnection{url: oldEndpoint}

		require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable diagnostic capture")
		var logMu sync.Mutex
		var entries []string
		gctlog.SetCustomLogHook(func(_, _ string, args ...any) bool {
			entry := fmt.Sprint(args...)
			if !strings.Contains(entry, ex.Name) {
				return false
			}
			logMu.Lock()
			entries = append(entries, entry)
			logMu.Unlock()
			return true
		})
		t.Cleanup(func() {
			gctlog.SetCustomLogHook(nil)
			assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable diagnostic capture")
		})

		require.NoError(t, ex.WsConnect(t.Context(), conn), "WsConnect must connect using the replacement endpoint")
		assert.Equal(t, newEndpoint, conn.url, "WsConnect should preserve the replacement endpoint for connection use")
		assert.Equal(t, url.Values{"token": {token}}, conn.dialValues, "Dial should receive the instance token")
		assert.Equal(t, 1, conn.setURLCalls, "SetURL should run once for a changed endpoint")
		assert.Equal(t, 1, conn.pingCalls, "SetupPingHandler should run once")
		assert.Equal(t, request.Unset, conn.pingLimit, "SetupPingHandler should use the unset endpoint limit")
		assert.Equal(t, 250*time.Millisecond, conn.pingHandler.Delay, "SetupPingHandler should preserve the server interval")

		logMu.Lock()
		diagnostics := strings.Join(entries, "\n")
		logMu.Unlock()
		assert.Contains(t, diagnostics, "old: wss://old.example.com", "WsConnect diagnostics should include the old endpoint origin")
		assert.Contains(t, diagnostics, "new: wss://new.example.com", "WsConnect diagnostics should include the new endpoint origin")
		for _, secret := range []string{"old-user-token", "old-password-token", "old-path-token", "old-query-token", "old-fragment-token", "new-user-token", "new-password-token", "new-path-token", "new-query-token", "new-fragment-token", token} {
			assert.NotContains(t, diagnostics, secret, "WsConnect diagnostics should omit endpoint and token secrets")
		}
	})

	t.Run("unchanged endpoint and dial failure", func(t *testing.T) {
		const endpoint = "wss://example.com/socket"
		dialErr := errors.New("dial failure")
		ex, _ := newWsConnectTestExchange(t, fmt.Sprintf(`{"code":"200000","data":{"token":%q,"instanceServers":[{"endpoint":%q,"pingInterval":250}]}}`, token, endpoint))
		conn := &wsConnectTestConnection{url: endpoint, dialErr: dialErr}
		err := ex.WsConnect(t.Context(), conn)
		require.ErrorIs(t, err, dialErr, "WsConnect must return the dial failure")
		assert.Zero(t, conn.setURLCalls, "SetURL should not run for an unchanged endpoint")
		assert.Zero(t, conn.pingCalls, "SetupPingHandler should not run after a dial failure")
	})

	t.Run("authenticated instance server", func(t *testing.T) {
		const endpoint = "wss://example.com/private"
		ex, requests := newWsConnectTestExchange(t, fmt.Sprintf(`{"code":"200000","data":{"token":%q,"instanceServers":[{"endpoint":%q,"pingInterval":250}]}}`, token, endpoint))
		ex.API.AuthenticatedSupport = true
		ex.API.AuthenticatedWebsocketSupport = true
		ex.API.CredentialsValidator.RequiresBase64DecodeSecret = false
		ex.SetCredentials(&accounts.Credentials{Key: "key", Secret: "secret", ClientID: "client-id"})
		ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
		conn := &wsConnectTestConnection{url: endpoint}

		require.NoError(t, ex.WsConnect(t.Context(), conn), "WsConnect must retrieve an authenticated instance server")
		assert.Equal(t, privateBullets, <-requests, "GetAuthenticatedInstanceServers should request the private endpoint")
		assert.Equal(t, 1, conn.dialCalls, "Dial should run once")
		assert.Equal(t, url.Values{"token": {token}}, conn.dialValues, "Dial should receive the authenticated instance token")
		assert.Equal(t, 1, conn.pingCalls, "SetupPingHandler should run once")
	})
}
