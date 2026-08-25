package binanceus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
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
	dialErr    error
	dialTarget string
	dialValues url.Values
	dialURL    string
	url        string
	dialCalls  atomic.Int32
	pingCalls  atomic.Int32
	readCalls  atomic.Int32
}

func (c *wsConnectTestConnection) Dial(_ context.Context, _ *gws.Dialer, _ http.Header, values url.Values) error {
	c.dialCalls.Add(1)
	c.dialTarget = c.url
	if len(values) != 0 {
		endpoint, err := url.Parse(c.url)
		if err != nil {
			return err
		}
		query, err := url.ParseQuery(endpoint.RawQuery)
		if err != nil {
			return err
		}
		for key, explicitValues := range values {
			query.Del(key)
			for _, value := range explicitValues {
				query.Add(key, value)
			}
		}
		endpoint.RawQuery = query.Encode()
		c.dialTarget = endpoint.String()
	}
	c.dialValues = values
	c.dialURL = c.url
	return c.dialErr
}

func (c *wsConnectTestConnection) ReadMessage() websocket.Response {
	c.readCalls.Add(1)
	return websocket.Response{}
}

func (c *wsConnectTestConnection) SetupPingHandler(request.EndpointLimit, websocket.PingHandler) {
	c.pingCalls.Add(1)
}

func (c *wsConnectTestConnection) SetURL(u string) {
	c.url = u
}

func (c *wsConnectTestConnection) GetURL() string {
	return c.url
}

func newWsConnectTestExchange(t *testing.T, conn *wsConnectTestConnection) *Exchange {
	t.Helper()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	conn.url = ex.Websocket.GetWebsocketURL()
	ex.Websocket.Conn = conn
	return ex
}

func stopWsConnectTestExchange(t *testing.T, ex *Exchange) {
	t.Helper()
	close(ex.Websocket.ShutdownC)
	done := make(chan struct{})
	go func() {
		ex.Websocket.Wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("websocket routines must stop")
	}
}

func cleanupWsConnectTestExchange(t *testing.T, ex *Exchange) {
	t.Helper()
	t.Cleanup(func() { stopWsConnectTestExchange(t, ex) })
}

func configureWsConnectAuth(t *testing.T, ex *Exchange, listenKeyValue string, subsequentListenKeyValues ...string) {
	t.Helper()
	listenKeyValues := append([]string{listenKeyValue}, subsequentListenKeyValues...)
	var requestCount atomic.Uint32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, userAccountStream, r.URL.Path, "auth stream request path should match")
		assert.Equal(t, "test-key", r.Header.Get("X-MBX-APIKEY"), "auth stream request key should match")
		switch r.Method {
		case http.MethodPost:
			index := min(int(requestCount.Add(1)-1), len(listenKeyValues)-1)
			_, err := w.Write([]byte(`{"listenKey":"` + listenKeyValues[index] + `"}`))
			assert.NoError(t, err, "auth stream response write should not error")
		case http.MethodDelete:
			assert.Equal(t, listenKeyValues[len(listenKeyValues)-1], r.URL.Query().Get("listenKey"), "auth stream close key should match")
		default:
			assert.Failf(t, "unexpected auth stream method", "method %q should not be used", r.Method)
		}
	}))
	t.Cleanup(server.Close)
	configureWsConnectAuthEndpoint(t, ex, server.URL)
}

func configureWsConnectAuthFailure(t *testing.T, ex *Exchange, responseBody string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(responseBody))
		assert.NoError(t, err, "auth stream response write should not error")
	}))
	t.Cleanup(server.Close)
	configureWsConnectAuthEndpoint(t, ex, server.URL)
}

func configureWsConnectAuthEndpoint(t *testing.T, ex *Exchange, endpoint string) {
	t.Helper()
	require.NoError(t, ex.API.Endpoints.SetRunningURL(exchange.RestSpotSupplementary.String(), endpoint), "REST endpoint must be set")
	ex.API.AuthenticatedSupport = true
	ex.API.AuthenticatedWebsocketSupport = true
	ex.API.CredentialsValidator.RequiresBase64DecodeSecret = false
	ex.SetCredentials(&accounts.Credentials{Key: "test-key", Secret: "test-secret"})
	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
}

func configureWsConnectMultiConnectionManager(t *testing.T, ex *Exchange, conn *wsConnectTestConnection, runningURL string) {
	t.Helper()
	configCopy := *ex.Config
	configCopy.WebsocketTrafficTimeout = time.Nanosecond
	manager := websocket.NewManager()
	setup := &websocket.ManagerSetup{
		ExchangeConfig:        &configCopy,
		DefaultURL:            binanceusDefaultWebsocketURL,
		RunningURL:            runningURL,
		Connector:             ex.WsConnect,
		Subscriber:            ex.Subscribe,
		Unsubscriber:          ex.Unsubscribe,
		GenerateSubscriptions: ex.GenerateSubscriptions,
		Features:              &ex.Features.Supports.WebsocketCapabilities,
	}
	require.Error(t, manager.Setup(setup), "Setup must reject the invalid traffic timeout")
	configCopy.WebsocketTrafficTimeout = time.Second
	setup.UseMultiConnectionManagement = true
	require.NoError(t, manager.Setup(setup), "Setup must enable multi-connection management")
	manager.Conn = conn
	conn.SetURL(runningURL)
	ex.Websocket = manager
}

func TestRemoveLegacyListenKey(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name              string
		endpoint          string
		previousListenKey string
		expected          string
		removed           bool
	}{
		{name: "empty listen key", endpoint: "wss://example.com/stream?streams=legacy", expected: "wss://example.com/stream?streams=legacy"},
		{name: "no query", endpoint: "wss://example.com/stream#fragment", previousListenKey: "legacy", expected: "wss://example.com/stream#fragment"},
		{name: "query marker in fragment", endpoint: "wss://example.com/stream#fragment?streams=legacy", previousListenKey: "legacy", expected: "wss://example.com/stream#fragment?streams=legacy"},
		{name: "no streams value", endpoint: "wss://example.com/stream?mode=combined#fragment", previousListenKey: "legacy", expected: "wss://example.com/stream?mode=combined#fragment"},
		{name: "different streams value", endpoint: "wss://example.com/stream?streams=btcusdt@trade", previousListenKey: "legacy", expected: "wss://example.com/stream?streams=btcusdt@trade"},
		{name: "matching only value", endpoint: "wss://example.com/stream?streams=legacy#fragment", previousListenKey: "legacy", expected: "wss://example.com/stream#fragment", removed: true},
		{name: "matching values among persistent parameters", endpoint: "wss://example.com/stream?mode=combined&streams=legacy&streams=btcusdt@trade&opaque=one%20two&streams=legacy#fragment", previousListenKey: "legacy", expected: "wss://example.com/stream?mode=combined&streams=btcusdt@trade&opaque=one%20two#fragment", removed: true},
		{name: "encoded matching value", endpoint: "wss://example.com/stream?str%65ams=legacy%2Dkey&mode=combined", previousListenKey: "legacy-key", expected: "wss://example.com/stream?mode=combined", removed: true},
		{name: "invalid encoded value", endpoint: "wss://example.com/stream?streams=legacy%zz&mode=combined", previousListenKey: "legacy", expected: "wss://example.com/stream?streams=legacy%zz&mode=combined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, removed := removeLegacyListenKey(tc.endpoint, tc.previousListenKey)
			assert.Equal(t, tc.expected, actual, "removeLegacyListenKey should preserve the expected endpoint")
			assert.Equal(t, tc.removed, removed, "removeLegacyListenKey should report removal correctly")
		})
	}
}

func TestWsConnect(t *testing.T) {
	originalListenKey := listenKey
	t.Cleanup(func() { listenKey = originalListenKey })

	t.Run("websocket disabled", func(t *testing.T) {
		listenKey = ""
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		require.NoError(t, ex.Websocket.Disable(), "Disable must not error")
		require.ErrorIs(t, ex.WsConnect(), websocket.ErrWebsocketNotEnabled, "WsConnect must reject a disabled websocket")
		assert.Zero(t, conn.dialCalls.Load(), "Dial should not be called")
	})

	t.Run("exchange disabled", func(t *testing.T) {
		listenKey = ""
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		ex.SetEnabled(false)
		require.ErrorIs(t, ex.WsConnect(), websocket.ErrWebsocketNotEnabled, "WsConnect must reject a disabled exchange")
		assert.Zero(t, conn.dialCalls.Load(), "Dial should not be called")
	})

	t.Run("dial failure", func(t *testing.T) {
		listenKey = ""
		dialCause := errors.New("dial-cause-secret-canary")
		dialErr := &url.Error{Op: "dial-operation-secret-canary", URL: "wss://user-secret:password-secret@example.com/path-secret?signature=query-secret", Err: dialCause}
		conn := &wsConnectTestConnection{dialErr: dialErr}
		ex := newWsConnectTestExchange(t, conn)
		err := ex.WsConnect()
		require.Error(t, err, "WsConnect must return the dial failure")
		assert.ErrorIs(t, err, dialCause, "WsConnect should preserve the dial failure chain")
		var target *url.Error
		assert.ErrorAs(t, err, &target, "WsConnect should preserve dial failure inspection")
		for _, secret := range []string{"dial-cause-secret-canary", "dial-operation-secret-canary", "user-secret", "password-secret", "path-secret", "query-secret"} {
			assert.NotContains(t, err.Error(), secret, "WsConnect dial failure should omit secret details")
		}
		assert.Equal(t, int32(1), conn.dialCalls.Load(), "Dial should be called once")
		assert.Nil(t, conn.dialValues, "Dial values should be empty for a public connection")
		assert.Zero(t, conn.pingCalls.Load(), "Ping setup should not run after a dial failure")
	})

	t.Run("authenticated dial failure keeps listen key ephemeral", func(t *testing.T) {
		const (
			listenKeyCanary = "authenticated-listen-key-canary"
			persistentURL   = binanceusDefaultWebsocketURL + "?streams=btcusdt@trade/ethusdt@trade&timeUnit=MICROSECOND"
		)
		listenKey = ""
		dialCause := errors.New("authenticated-dial-cause-" + listenKeyCanary)
		conn := &wsConnectTestConnection{dialErr: dialCause}
		ex := newWsConnectTestExchange(t, conn)
		require.NoError(t, ex.Websocket.SetWebsocketURL(persistentURL, false, false), "SetWebsocketURL must set the public combined stream URL")
		configureWsConnectAuth(t, ex, listenKeyCanary)

		err := ex.WsConnect()
		require.Error(t, err, "WsConnect must return the authenticated dial failure")
		assert.ErrorIs(t, err, dialCause, "WsConnect should preserve the authenticated dial failure chain")
		assert.NotContains(t, err.Error(), listenKeyCanary, "WsConnect authenticated dial failure should omit the listen key")
		assert.Equal(t, url.Values{"streams": {listenKeyCanary}}, conn.dialValues, "Dial values should contain only the authenticated listen key")
		assert.Contains(t, conn.dialTarget, listenKeyCanary, "Dial target should use the authenticated listen key")
		assert.Equal(t, persistentURL, conn.GetURL(), "connection URL should retain the configured public URL")
		assert.Equal(t, persistentURL, ex.Websocket.GetWebsocketURL(), "manager URL should retain the configured public URL")
		assert.Zero(t, conn.pingCalls.Load(), "Ping setup should not run after an authenticated dial failure")
		assert.Zero(t, conn.readCalls.Load(), "ReadMessage should not run after an authenticated dial failure")
	})

	t.Run("stale URL reset failure", func(t *testing.T) {
		const staleListenKeyCanary = "stale-listen-key-canary"
		listenKey = staleListenKeyCanary
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		configureWsConnectMultiConnectionManager(t, ex, conn, binanceusDefaultWebsocketURL+"?streams="+staleListenKeyCanary)
		configureWsConnectAuth(t, ex, "new-listen-key-canary")

		require.ErrorContains(t, ex.WsConnect(), "cannot change connection URL", "WsConnect must return the stale URL reset failure")
		assert.Zero(t, conn.dialCalls.Load(), "Dial should not be called after a stale URL reset failure")
	})

	t.Run("authenticated public URL survives managed connection", func(t *testing.T) {
		const persistentURL = binanceusDefaultWebsocketURL + "?streams=btcusdt@trade/ethusdt@trade&mode=combined#public-fragment"
		listenKey = ""
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		configureWsConnectMultiConnectionManager(t, ex, conn, persistentURL)
		configureWsConnectAuth(t, ex, "new-listen-key-canary")

		require.NoError(t, ex.WsConnect(), "WsConnect must preserve a managed public URL")
		cleanupWsConnectTestExchange(t, ex)
		assert.Equal(t, persistentURL, conn.dialURL, "Dial should use the managed public URL")
		assert.Equal(t, persistentURL, conn.GetURL(), "connection URL should preserve the managed public URL")
		assert.Equal(t, persistentURL, ex.Websocket.GetWebsocketURL(), "manager URL should preserve the managed public URL")
		assert.Equal(t, url.Values{"streams": {"new-listen-key-canary"}}, conn.dialValues, "Dial values should contain only the authenticated listen key")
	})

	t.Run("auth key failure falls back to public", func(t *testing.T) {
		const (
			staleListenKeyCanary = "stale-listen-key-canary"
			responseBodyCanary   = "auth-response-body-secret-canary"
		)
		listenKey = staleListenKeyCanary
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		baseURL := conn.GetURL()
		require.NoError(t, ex.Websocket.SetWebsocketURL(baseURL+"?streams="+staleListenKeyCanary, false, false), "SetWebsocketURL must set the stale authenticated URL")
		configureWsConnectAuthFailure(t, ex, responseBodyCanary)
		ex.Name += "-auth-error-diagnostic-canary"

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

		require.NoError(t, ex.WsConnect(), "WsConnect must fall back to a public connection")
		cleanupWsConnectTestExchange(t, ex)
		assert.False(t, ex.Websocket.CanUseAuthenticatedEndpoints(), "authenticated websocket support should be disabled")
		assert.Nil(t, conn.dialValues, "Dial values should be empty after auth fallback")
		assert.Equal(t, baseURL, conn.dialURL, "Dial should omit the stale authenticated listen key after auth fallback")
		assert.Equal(t, baseURL, ex.Websocket.GetWebsocketURL(), "manager URL should omit the stale authenticated listen key after auth fallback")
		logMu.Lock()
		diagnostics := strings.Join(entries, "\n")
		logMu.Unlock()
		assert.Contains(t, diagnostics, ex.Name, "WsConnect diagnostics should identify the exchange")
		for _, secret := range []string{responseBodyCanary, staleListenKeyCanary, "test-key", "test-secret"} {
			assert.NotContains(t, diagnostics, secret, "WsConnect diagnostics should omit authentication secrets")
		}
	})

	t.Run("auth key failure preserves public combined streams", func(t *testing.T) {
		const (
			previousListenKey = "previous-listen-key-canary"
			combinedStreamURL = binanceusDefaultWebsocketURL + "?streams=btcusdt@trade/ethusdt@trade&mode=combined&opaque=one%20two#public-fragment"
		)
		listenKey = previousListenKey
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		require.NoError(t, ex.Websocket.SetWebsocketURL(combinedStreamURL, false, false), "SetWebsocketURL must set the public combined stream URL")
		configureWsConnectAuthFailure(t, ex, "auth-response-body-secret-canary")

		require.NoError(t, ex.WsConnect(), "WsConnect must fall back without replacing public combined streams")
		cleanupWsConnectTestExchange(t, ex)
		assert.Equal(t, combinedStreamURL, conn.dialURL, "Dial should preserve public combined streams after auth failure")
		assert.Equal(t, combinedStreamURL, conn.GetURL(), "connection URL should preserve public combined streams after auth failure")
		assert.Equal(t, combinedStreamURL, ex.Websocket.GetWebsocketURL(), "manager URL should preserve public combined streams after auth failure")
		assert.Nil(t, conn.dialValues, "Dial values should be empty after auth failure")
	})

	t.Run("authenticated dial keeps the listen key ephemeral", func(t *testing.T) {
		const listenKeyCanary = "listen-key-canary"
		listenKey = ""
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		baseURL := conn.GetURL()
		configureWsConnectAuth(t, ex, listenKeyCanary)
		require.NoError(t, ex.WsConnect(), "WsConnect must connect with authentication")
		cleanupWsConnectTestExchange(t, ex)
		assert.Equal(t, url.Values{"streams": {listenKeyCanary}}, conn.dialValues, "Dial values should contain the listen key")
		assert.Equal(t, baseURL, conn.GetURL(), "connection URL should remain at the configured base URL")
		assert.Equal(t, baseURL, ex.Websocket.GetWebsocketURL(), "manager URL should remain at the configured base URL")
		assert.Equal(t, int32(1), conn.pingCalls.Load(), "Ping setup should run once")
	})

	t.Run("authenticated reconnect keeps stored public URL while wire streams use fresh keys", func(t *testing.T) {
		const (
			firstListenKey   = "first-listen-key-canary"
			secondListenKey  = "second-listen-key-canary"
			publicStreamsURL = binanceusDefaultWebsocketURL + "?streams=btcusdt@trade/ethusdt@trade&mode=combined&opaque=one%20two#public-fragment"
		)
		listenKey = ""
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		require.NoError(t, ex.Websocket.SetWebsocketURL(publicStreamsURL, false, false), "SetWebsocketURL must set the public combined stream URL")
		configureWsConnectAuth(t, ex, firstListenKey, secondListenKey)

		require.NoError(t, ex.WsConnect(), "WsConnect must connect while retaining the configured public URL")
		cleanupWsConnectTestExchange(t, ex)
		assert.Equal(t, int32(1), conn.dialCalls.Load(), "Dial should be called once before reconnect")
		assert.Equal(t, publicStreamsURL, conn.dialURL, "Dial URL should preserve configured public combined streams")
		assert.Equal(t, url.Values{"streams": {firstListenKey}}, conn.dialValues, "Dial values should contain only the first authenticated listen key")
		target, err := url.Parse(conn.dialTarget)
		require.NoError(t, err, "Dial target must parse")
		assert.Equal(t, []string{firstListenKey}, target.Query()["streams"], "Dial target should override public streams with only the first listen key")
		assert.Equal(t, "combined", target.Query().Get("mode"), "Dial target should preserve the configured mode")
		assert.Equal(t, "one two", target.Query().Get("opaque"), "Dial target should preserve unrelated query data")
		assert.Equal(t, "public-fragment", target.Fragment, "Dial target should preserve the configured fragment")
		assert.Equal(t, publicStreamsURL, conn.GetURL(), "connection URL should preserve public combined streams")
		assert.Equal(t, publicStreamsURL, ex.Websocket.GetWebsocketURL(), "manager URL should preserve public combined streams")
		assert.NotContains(t, conn.GetURL(), firstListenKey, "connection URL should not retain the first authenticated listen key")

		require.NoError(t, ex.WsConnect(), "WsConnect must reconnect while retaining the configured public URL")
		assert.Equal(t, int32(2), conn.dialCalls.Load(), "Dial should be called twice after reconnect")
		assert.Equal(t, publicStreamsURL, conn.dialURL, "reconnect Dial URL should preserve configured public combined streams")
		assert.Equal(t, url.Values{"streams": {secondListenKey}}, conn.dialValues, "Dial values should contain only the second authenticated listen key")
		target, err = url.Parse(conn.dialTarget)
		require.NoError(t, err, "reconnect Dial target must parse")
		assert.Equal(t, []string{secondListenKey}, target.Query()["streams"], "reconnect Dial target should override public streams with only the second listen key")
		assert.Equal(t, publicStreamsURL, conn.GetURL(), "connection URL should survive authenticated reconnect")
		assert.Equal(t, publicStreamsURL, ex.Websocket.GetWebsocketURL(), "manager URL should survive authenticated reconnect")
		for _, key := range []string{firstListenKey, secondListenKey} {
			assert.NotContains(t, conn.GetURL(), key, "connection URL should not retain authenticated listen keys")
			assert.NotContains(t, ex.Websocket.GetWebsocketURL(), key, "manager URL should not retain authenticated listen keys")
		}
	})

	t.Run("authenticated dial replaces a stale listen key ephemerally", func(t *testing.T) {
		const (
			staleListenKeyCanary = "stale-listen-key-canary"
			newListenKeyCanary   = "new-listen-key-canary"
		)
		listenKey = staleListenKeyCanary
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		baseURL := conn.GetURL()
		legacyURL := baseURL + "?mode=combined&streams=" + staleListenKeyCanary + "&streams=btcusdt@trade&opaque=one%20two#public-fragment"
		persistentURL := baseURL + "?mode=combined&streams=btcusdt@trade&opaque=one%20two#public-fragment"
		require.NoError(t, ex.Websocket.SetWebsocketURL(legacyURL, false, false), "SetWebsocketURL must set the stale authenticated URL")
		configureWsConnectAuth(t, ex, newListenKeyCanary)

		require.NoError(t, ex.WsConnect(), "WsConnect must replace a stale listen key")
		cleanupWsConnectTestExchange(t, ex)
		assert.Equal(t, int32(1), conn.dialCalls.Load(), "Dial should be called once")
		assert.Equal(t, persistentURL, conn.dialURL, "Dial URL should remove only the stale listen key")
		assert.Equal(t, url.Values{"streams": {newListenKeyCanary}}, conn.dialValues, "Dial values should contain only the new listen key")
		assert.Equal(t, persistentURL, conn.GetURL(), "connection URL should preserve public query and fragment data")
		assert.NotContains(t, conn.GetURL(), staleListenKeyCanary, "connection URL should not retain the stale listen key")
		assert.NotContains(t, conn.GetURL(), newListenKeyCanary, "connection URL should not retain the new listen key")
		assert.Equal(t, persistentURL, ex.Websocket.GetWebsocketURL(), "manager URL should preserve public query and fragment data")
		assert.NotContains(t, ex.Websocket.GetWebsocketURL(), staleListenKeyCanary, "manager URL should not retain the stale listen key")
		assert.NotContains(t, ex.Websocket.GetWebsocketURL(), newListenKeyCanary, "manager URL should not retain the new listen key")
	})

	t.Run("public connection", func(t *testing.T) {
		listenKey = ""
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		require.NoError(t, ex.WsConnect(), "WsConnect must connect publicly")
		cleanupWsConnectTestExchange(t, ex)
		assert.Nil(t, conn.dialValues, "Dial values should be empty for a public connection")
		assert.Equal(t, int32(1), conn.pingCalls.Load(), "Ping setup should run once")
	})

	t.Run("public combined stream URL is preserved", func(t *testing.T) {
		const combinedStreamURL = binanceusDefaultWebsocketURL + "?streams=btcusdt@trade/ethusdt@trade"
		listenKey = "previous-listen-key-canary"
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		require.NoError(t, ex.Websocket.SetWebsocketURL(combinedStreamURL, false, false), "SetWebsocketURL must set the public combined stream URL")

		require.NoError(t, ex.WsConnect(), "WsConnect must preserve public combined streams")
		cleanupWsConnectTestExchange(t, ex)
		assert.Equal(t, combinedStreamURL, conn.dialURL, "Dial should use the complete public combined stream URL")
		assert.Equal(t, combinedStreamURL, conn.GetURL(), "connection URL should retain public combined streams")
		assert.Equal(t, combinedStreamURL, ex.Websocket.GetWebsocketURL(), "manager URL should retain public combined streams")
		assert.Nil(t, conn.dialValues, "Dial values should be empty for public combined streams")
	})

	t.Run("auth-disabled stale listen key is removed", func(t *testing.T) {
		const staleListenKeyCanary = "stale-listen-key-canary"
		listenKey = staleListenKeyCanary
		conn := new(wsConnectTestConnection)
		ex := newWsConnectTestExchange(t, conn)
		baseURL := conn.GetURL()
		legacyURL := baseURL + "?mode=combined&streams=" + staleListenKeyCanary + "&streams=btcusdt@trade&opaque=one%20two#public-fragment"
		persistentURL := baseURL + "?mode=combined&streams=btcusdt@trade&opaque=one%20two#public-fragment"
		require.NoError(t, ex.Websocket.SetWebsocketURL(legacyURL, false, false), "SetWebsocketURL must set the stale authenticated URL")

		require.NoError(t, ex.WsConnect(), "WsConnect must remove a known stale listen key")
		cleanupWsConnectTestExchange(t, ex)
		assert.Equal(t, persistentURL, conn.dialURL, "Dial should remove only the known stale listen key when authentication is disabled")
		assert.Equal(t, persistentURL, conn.GetURL(), "connection URL should preserve public query and fragment data when authentication is disabled")
		assert.Equal(t, persistentURL, ex.Websocket.GetWebsocketURL(), "manager URL should preserve public query and fragment data when authentication is disabled")
		assert.Nil(t, conn.dialValues, "Dial values should be empty when authentication is disabled")
	})
}
