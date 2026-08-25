package gemini

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

type wsAuthTestConnection struct {
	websocket.Connection
	dialErr    error
	url        string
	dialHeader http.Header
	dialValues url.Values
	dialCalls  atomic.Int32
	readCalls  atomic.Int32
}

func (c *wsAuthTestConnection) Dial(_ context.Context, _ *gws.Dialer, header http.Header, values url.Values) error {
	c.dialCalls.Add(1)
	c.dialHeader = header.Clone()
	c.dialValues = values
	return c.dialErr
}

func (c *wsAuthTestConnection) ReadMessage() websocket.Response {
	c.readCalls.Add(1)
	return websocket.Response{}
}

func (c *wsAuthTestConnection) GetURL() string {
	return c.url
}

func newWsAuthTestExchange(t *testing.T, authURL string, conn *wsAuthTestConnection) *Exchange {
	t.Helper()
	ex := new(Exchange)
	require.NoError(t, testexch.Setup(ex), "Setup must not error")
	ex.API.AuthenticatedSupport = true
	ex.API.AuthenticatedWebsocketSupport = true
	ex.SetCredentials(&accounts.Credentials{Key: "test-key", Secret: "test-secret"})
	if authURL == "" {
		authURL = ex.Websocket.AuthConn.GetURL()
	}
	conn.url = authURL
	ex.Websocket.AuthConn = conn
	return ex
}

func TestWsAuthDirectPaths(t *testing.T) {
	t.Run("authentication unsupported", func(t *testing.T) {
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		ex.API.AuthenticatedWebsocketSupport = false
		err := ex.WsAuth(t.Context(), new(gws.Dialer))
		require.ErrorContains(t, err, "AuthenticatedWebsocketAPISupport not enabled", "WsAuth must reject unsupported authentication")
	})

	t.Run("credentials error", func(t *testing.T) {
		ex := new(Exchange)
		require.NoError(t, testexch.Setup(ex), "Setup must not error")
		ex.API.AuthenticatedWebsocketSupport = true
		err := ex.WsAuth(t.Context(), new(gws.Dialer))
		require.ErrorIs(t, err, exchange.ErrCredentialsAreEmpty, "WsAuth must return the credentials error")
	})

	t.Run("payload error", func(t *testing.T) {
		marshal := json.Marshal
		marshalErr := errors.New("marshal failure")
		json.Marshal = func(any) ([]byte, error) { return nil, marshalErr }
		t.Cleanup(func() { json.Marshal = marshal })

		conn := new(wsAuthTestConnection)
		ex := newWsAuthTestExchange(t, "wss://example.com/v1/", conn)
		err := ex.WsAuth(t.Context(), new(gws.Dialer))
		require.ErrorContains(t, err, "Unable to JSON request", "WsAuth must return the payload encoding error")
		assert.Zero(t, conn.dialCalls.Load(), "Dial should not run after a payload encoding error")
	})

	t.Run("HMAC error", func(t *testing.T) {
		original := getWebsocketHMAC
		hmacErr := errors.New("HMAC failure")
		getWebsocketHMAC = func(int, []byte, []byte) ([]byte, error) { return nil, hmacErr }
		t.Cleanup(func() { getWebsocketHMAC = original })

		conn := new(wsAuthTestConnection)
		ex := newWsAuthTestExchange(t, "wss://example.com/v1/", conn)
		err := ex.WsAuth(t.Context(), new(gws.Dialer))
		require.ErrorIs(t, err, hmacErr, "WsAuth must return HMAC failures")
		assert.Zero(t, conn.dialCalls.Load(), "Dial should not run after an HMAC failure")
	})

	t.Run("dial error diagnostics", func(t *testing.T) {
		const endpoint = "wss://user-token:password-token@example.com/path-token?signature=query-token#fragment-token"
		dialErr := errors.New("dial-cause-token")
		conn := &wsAuthTestConnection{dialErr: dialErr}
		ex := newWsAuthTestExchange(t, endpoint, conn)

		err := ex.WsAuth(t.Context(), new(gws.Dialer))
		require.ErrorIs(t, err, dialErr, "WsAuth must preserve the dial error chain")
		assert.Contains(t, err.Error(), "wss://example.com", "WsAuth should retain safe endpoint metadata")
		for _, secret := range []string{"user-token", "password-token", "path-token", "query-token", "fragment-token", "dial-cause-token"} {
			assert.NotContains(t, err.Error(), secret, "WsAuth diagnostics should omit endpoint and error secrets")
		}
		assert.Equal(t, int32(1), conn.dialCalls.Load(), "Dial should run once")
		assert.Zero(t, conn.readCalls.Load(), "ReadMessage should not run after a dial failure")
	})

	t.Run("default endpoint dial error diagnostics", func(t *testing.T) {
		dialErr := errors.New("default-dial-cause-token")
		conn := &wsAuthTestConnection{dialErr: dialErr}
		ex := newWsAuthTestExchange(t, "", conn)

		err := ex.WsAuth(t.Context(), new(gws.Dialer))
		require.ErrorIs(t, err, dialErr, "WsAuth must preserve the default endpoint dial error chain")
		assert.Contains(t, err.Error(), geminiWebsocketEndpoint, "WsAuth should retain the default authenticated endpoint origin")
		assert.NotContains(t, err.Error(), geminiWebsocketEndpoint+geminiWsOrderEvents, "WsAuth should not concatenate the authenticated path without a separator")
		assert.NotContains(t, err.Error(), "/v1/"+geminiWsOrderEvents, "WsAuth diagnostics should omit the authenticated endpoint path")
		assert.NotContains(t, err.Error(), "default-dial-cause-token", "WsAuth diagnostics should omit the dial error text")
		assert.Equal(t, int32(1), conn.dialCalls.Load(), "Dial should run once")
		assert.Zero(t, conn.readCalls.Load(), "ReadMessage should not run after a dial failure")
	})

	t.Run("success", func(t *testing.T) {
		conn := new(wsAuthTestConnection)
		ex := newWsAuthTestExchange(t, "wss://example.com/v1/", conn)
		require.NoError(t, ex.WsAuth(t.Context(), new(gws.Dialer)), "WsAuth must connect successfully")

		waitDone := make(chan struct{})
		go func() {
			ex.Websocket.Wg.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(time.Second):
			t.Fatal("WsAuth websocket reader must stop")
		}

		assert.Equal(t, int32(1), conn.dialCalls.Load(), "Dial should run once")
		assert.Equal(t, int32(1), conn.readCalls.Load(), "ReadMessage should run once")
		assert.Equal(t, "test-key", conn.dialHeader.Get("X-GEMINI-APIKEY"), "Dial should receive the API key header")
		assert.NotEmpty(t, conn.dialHeader.Get("X-GEMINI-PAYLOAD"), "Dial should receive the encoded payload header")
		assert.NotEmpty(t, conn.dialHeader.Get("X-GEMINI-SIGNATURE"), "Dial should receive the signature header")
		assert.Empty(t, conn.dialValues, "Dial query values should remain empty")
		assert.NotContains(t, conn.dialHeader.Get("X-GEMINI-PAYLOAD"), "test-secret", "Dial payload should not contain the raw secret")
	})
}
