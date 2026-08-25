package websocket

import (
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"net"
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
	gctjson "github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
)

var errConnectionTest = errors.New("connection test error")

type connectionTestMarshalError struct {
	message string
}

func (e *connectionTestMarshalError) Error() string { return e.message }

type connectionTestNetError struct {
	timeout   bool
	temporary bool
}

func (e connectionTestNetError) Error() string { return errConnectionTest.Error() }

func (e connectionTestNetError) Timeout() bool { return e.timeout }

func (e connectionTestNetError) Temporary() bool { return e.temporary }

type connectionTestReporter struct {
	message []byte
}

type connectionTestMarshalFailure struct {
	err error
}

func (e connectionTestMarshalFailure) MarshalJSON() ([]byte, error) {
	return nil, e.err
}

type connectionTestPanickingNetError struct{}

func (connectionTestPanickingNetError) Error() string { return errConnectionTest.Error() }

func (connectionTestPanickingNetError) Timeout() bool { panic("timeout-panic-token") }

func (r *connectionTestReporter) Latency(_ string, message []byte, _ time.Duration) {
	r.message = message
}

func newConnectionTestServer(t *testing.T, handler func(*gws.Conn)) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}))
	t.Cleanup(server.Close)
	return "ws" + server.URL[len("http"):] + "/path-token?signature=query-token"
}

func deflateConnectionTestPayload(t *testing.T, payload []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer, err := flate.NewWriter(&compressed, flate.BestSpeed)
	require.NoError(t, err, "NewWriter must create the test compressor")
	_, err = writer.Write(payload)
	require.NoError(t, err, "Write must compress the test payload")
	require.NoError(t, writer.Close(), "Close must finish the test payload")
	return compressed.Bytes()
}

func TestConnectionDialAllPaths(t *testing.T) {
	t.Parallel()

	t.Run("query merge", func(t *testing.T) {
		t.Parallel()
		requestURL := make(chan *url.URL, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedURL := *r.URL
			requestURL <- &receivedURL
			conn, err := (&gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_, _, _ = conn.ReadMessage()
		}))
		t.Cleanup(server.Close)

		endpoint := "ws" + server.URL[len("http"):] + "/path?existing=first&existing=second&override=stale&remove=stale#fragment-token"
		conn := connection{ExchangeName: "test", URL: endpoint}
		explicit := url.Values{
			"added":    {"third", "fourth"},
			"override": {"replacement-one", "replacement-two"},
			"remove":   nil,
		}
		require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, explicit), "Dial must merge endpoint query values")
		t.Cleanup(func() { assert.NoError(t, conn.Shutdown(), "Shutdown should close the merged-query connection") })

		dialedURL := <-requestURL
		assert.Equal(t, []string{"first", "second"}, dialedURL.Query()["existing"], "Dial should preserve existing query multi-values")
		assert.Equal(t, []string{"third", "fourth"}, dialedURL.Query()["added"], "Dial should preserve explicit query multi-values")
		assert.Equal(t, []string{"replacement-one", "replacement-two"}, dialedURL.Query()["override"], "Dial should replace existing values for an explicit query key")
		assert.NotContains(t, dialedURL.Query(), "remove", "Dial should remove existing values for an explicitly empty query key")
		assert.Empty(t, dialedURL.Fragment, "Dial should keep endpoint fragments out of the handshake request")
	})

	t.Run("empty values preserve query bytes", func(t *testing.T) {
		t.Parallel()
		requestQuery := make(chan string, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestQuery <- r.URL.RawQuery
			conn, err := (&gws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_, _, _ = conn.ReadMessage()
		}))
		t.Cleanup(server.Close)

		const rawQuery = "z=last&flag&a=hello%20world&escaped=literal%2fslash&plus=literal+plus"
		endpoint := "ws" + server.URL[len("http"):] + "/path?" + rawQuery + "#fragment-token"
		conn := connection{ExchangeName: "test", URL: endpoint}
		require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must preserve an endpoint query when no values are supplied")
		t.Cleanup(func() { assert.NoError(t, conn.Shutdown(), "Shutdown should close the preserved-query connection") })

		assert.Equal(t, rawQuery, <-requestQuery, "Dial should preserve opaque endpoint query bytes when no values are supplied")
	})

	t.Run("invalid endpoint", func(t *testing.T) {
		t.Parallel()
		conn := connection{ExchangeName: "test", URL: "ws://example.com/path-token/%zz?signature=query-token#fragment-token"}
		err := conn.Dial(t.Context(), &gws.Dialer{}, nil, url.Values{"listen-key": {"value-token"}})
		require.Error(t, err, "Dial must reject an invalid endpoint URL")
		var urlErr *url.Error
		require.ErrorAs(t, err, &urlErr, "Dial must preserve URL parse failures")
		expectedErr := url.EscapeError("%zz")
		assert.ErrorIs(t, err, expectedErr, "Dial should preserve invalid escape failures")
		for _, secret := range []string{"path-token", "query-token", "fragment-token", "value-token"} {
			assert.NotContains(t, err.Error(), secret, "Dial errors should omit endpoint and explicit query data")
		}
	})

	t.Run("invalid endpoint query", func(t *testing.T) {
		t.Parallel()
		conn := connection{ExchangeName: "test", URL: "ws://example.com/path-token?signature=query-token;invalid#fragment-token"}
		err := conn.Dial(t.Context(), &gws.Dialer{}, nil, url.Values{"listen-key": {"value-token"}})
		require.Error(t, err, "Dial must reject an invalid endpoint query")
		for _, secret := range []string{"path-token", "query-token", "fragment-token", "value-token"} {
			assert.NotContains(t, err.Error(), secret, "Dial errors should omit endpoint and explicit query data")
		}
	})

	t.Run("invalid proxy", func(t *testing.T) {
		t.Parallel()
		conn := connection{ExchangeName: "test", ProxyURL: "http://proxy-path-token/%zz"}
		err := conn.Dial(t.Context(), &gws.Dialer{}, nil, nil)
		require.Error(t, err, "Dial must reject an invalid proxy URL")
		assert.NotContains(t, err.Error(), "proxy-path-token", "Dial errors should omit proxy URL data")
	})

	t.Run("valid proxy", func(t *testing.T) {
		t.Parallel()
		conn := connection{ExchangeName: "test", ProxyURL: "http://example.com/proxy-path-token", URL: "ws://example.invalid/path-token?signature=query-token"}
		dialer := &gws.Dialer{NetDialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, errConnectionTest
		}}
		err := conn.Dial(t.Context(), dialer, nil, nil)
		require.Error(t, err, "Dial must return proxy connection failures")
		for _, secret := range []string{"proxy-path-token", "path-token", "query-token"} {
			assert.NotContains(t, err.Error(), secret, "Dial errors should omit endpoint and proxy URL data")
		}
	})

	t.Run("error compatibility", func(t *testing.T) {
		t.Parallel()
		sourceErr := &connectionTestNetError{timeout: true, temporary: true}
		dialer := &gws.Dialer{Proxy: func(*http.Request) (*url.URL, error) {
			return nil, &url.Error{
				Op:  "proxy",
				URL: "https://user-token:password-token@example.com/path-token?signature=query-token",
				Err: sourceErr,
			}
		}}
		conn := connection{ExchangeName: "test", URL: "ws://example.invalid/socket-token"}
		err := conn.Dial(t.Context(), dialer, nil, nil)
		require.Error(t, err, "Dial must return the source connection error")
		assert.ErrorIs(t, err, sourceErr, "Dial should preserve the source connection error")
		var target *connectionTestNetError
		require.ErrorAs(t, err, &target, "Dial must preserve source error inspection")
		assert.Same(t, sourceErr, target, "Dial should preserve the source error value")
		var netErr net.Error
		require.ErrorAs(t, err, &netErr, "Dial must preserve net.Error compatibility")
		assert.True(t, netErr.Timeout(), "Dial should preserve timeout classification")
		var temporaryErr interface{ Temporary() bool }
		require.ErrorAs(t, err, &temporaryErr, "Dial must preserve temporary error compatibility")
		assert.True(t, temporaryErr.Temporary(), "Dial should preserve temporary classification")
		for _, secret := range []string{"user-token", "password-token", "path-token", "query-token", errConnectionTest.Error()} {
			assert.NotContains(t, err.Error(), secret, "Dial errors should omit URL and source error data")
		}
	})

	t.Run("traffic signal", func(t *testing.T) {
		t.Parallel()
		endpoint := newConnectionTestServer(t, func(conn *gws.Conn) {
			_, _, _ = conn.ReadMessage()
		})
		traffic := make(chan struct{}, 1)
		conn := connection{ExchangeName: "test", URL: endpoint, Traffic: traffic}
		require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the test server")
		t.Cleanup(func() { assert.NoError(t, conn.Shutdown(), "Shutdown should close the test connection") })
		select {
		case <-traffic:
		default:
			t.Fatal("Dial must signal websocket traffic")
		}
		assert.True(t, conn.IsConnected(), "Dial should mark the websocket connected")
	})
}

func TestConnectionReadMessageAllPaths(t *testing.T) {
	t.Parallel()

	t.Run("text with traffic signal", func(t *testing.T) {
		t.Parallel()
		endpoint := newConnectionTestServer(t, func(conn *gws.Conn) {
			_ = conn.WriteMessage(gws.TextMessage, []byte("text-response"))
		})
		traffic := make(chan struct{}, 1)
		conn := connection{ExchangeName: "test", URL: endpoint, Traffic: traffic, Verbose: true}
		require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the text server")
		<-traffic
		response := conn.ReadMessage()
		require.Equal(t, gws.TextMessage, response.Type, "ReadMessage must preserve the message type")
		assert.Equal(t, "text-response", string(response.Raw), "ReadMessage should preserve text data")
		select {
		case <-traffic:
		default:
			t.Fatal("ReadMessage must signal websocket traffic")
		}
	})

	t.Run("compressed binary", func(t *testing.T) {
		t.Parallel()
		payload := []byte("binary-response")
		compressed := deflateConnectionTestPayload(t, payload)
		endpoint := newConnectionTestServer(t, func(conn *gws.Conn) {
			_ = conn.WriteMessage(gws.BinaryMessage, compressed)
		})
		conn := connection{ExchangeName: "test", URL: endpoint}
		require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the binary server")
		response := conn.ReadMessage()
		require.Equal(t, gws.BinaryMessage, response.Type, "ReadMessage must preserve the message type")
		assert.Equal(t, payload, response.Raw, "ReadMessage should decompress binary data")
	})

	t.Run("invalid binary", func(t *testing.T) {
		t.Parallel()
		endpoint := newConnectionTestServer(t, func(conn *gws.Conn) {
			_ = conn.WriteMessage(gws.BinaryMessage, []byte("binary-payload-token"))
		})
		conn := connection{ExchangeName: "test", URL: endpoint}
		require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the invalid binary server")
		response := conn.ReadMessage()
		require.NotNil(t, response.Raw, "ReadMessage must return a non-nil invalid binary response")
		assert.Empty(t, response.Raw, "ReadMessage should omit invalid binary data")
	})

	t.Run("read failures", func(t *testing.T) {
		t.Parallel()
		endpoint := newConnectionTestServer(t, func(*gws.Conn) {})
		readErrors := make(chan error, 1)
		conn := connection{ExchangeName: "test", URL: endpoint, readMessageErrors: readErrors}
		require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the closing server")
		assert.Empty(t, conn.ReadMessage(), "ReadMessage should return an empty response after a read failure")
		select {
		case err := <-readErrors:
			require.ErrorIs(t, err, errConnectionFault, "ReadMessage must relay connection faults")
			assert.NotContains(t, err.Error(), "path-token", "ReadMessage errors should omit endpoint paths")
			assert.NotContains(t, err.Error(), "query-token", "ReadMessage errors should omit endpoint queries")
		default:
			t.Fatal("ReadMessage must relay the connection fault")
		}

		conn.readMessageErrors = nil
		conn.setConnectedStatus(true)
		assert.Empty(t, conn.ReadMessage(), "ReadMessage should return an empty response when error relay is unavailable")
		assert.Empty(t, conn.ReadMessage(), "ReadMessage should skip error relay after disconnection")
	})
}

func TestConnectionReadMessageDiagnosticRedaction(t *testing.T) {
	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable connection diagnostic capture")
	var logMu sync.Mutex
	var entries []string
	gctlog.SetCustomLogHook(func(_, _ string, a ...any) bool {
		logMu.Lock()
		entries = append(entries, fmt.Sprint(a...))
		logMu.Unlock()
		return true
	})
	t.Cleanup(func() {
		gctlog.SetCustomLogHook(nil)
		assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable connection diagnostic capture")
	})

	const closeReason = "close-reason-token"
	closeWrites := make(chan error, 2)
	closeEndpoint := newConnectionTestServer(t, func(conn *gws.Conn) {
		closeWrites <- conn.WriteMessage(gws.CloseMessage, gws.FormatCloseMessage(gws.ClosePolicyViolation, closeReason))
	})
	parsedCloseEndpoint, err := url.Parse(closeEndpoint)
	require.NoError(t, err, "Parse must parse the close-reason test endpoint")
	readErrors := make(chan error, 1)
	closeConn := connection{ExchangeName: "close-diagnostic-test", URL: closeEndpoint, readMessageErrors: readErrors}
	require.NoError(t, closeConn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the close-reason server")
	t.Cleanup(func() { assert.NoError(t, closeConn.Shutdown(), "Shutdown should close the close-reason connection") })
	assert.Empty(t, closeConn.ReadMessage(), "ReadMessage should return an empty response for a close frame")
	require.NoError(t, <-closeWrites, "WriteMessage must send the close-reason frame")
	select {
	case err := <-readErrors:
		require.ErrorIs(t, err, errConnectionFault, "ReadMessage must relay close-frame connection faults")
		var closeErr *gws.CloseError
		require.ErrorAs(t, err, &closeErr, "ReadMessage must preserve close-error inspection")
		assert.Equal(t, gws.ClosePolicyViolation, closeErr.Code, "ReadMessage should preserve the close code")
		assert.Equal(t, closeReason, closeErr.Text, "ReadMessage should preserve the close reason for explicit error inspection")
		assert.Contains(t, err.Error(), "endpoint="+parsedCloseEndpoint.Scheme+"://"+parsedCloseEndpoint.Host, "ReadMessage errors should retain safe endpoint metadata")
		for _, secret := range []string{closeReason, "path-token", "query-token"} {
			assert.NotContains(t, err.Error(), secret, "ReadMessage errors should omit close-reason and endpoint data")
		}
	default:
		t.Fatal("ReadMessage must relay the close-frame connection fault")
	}

	unrelayedCloseConn := connection{ExchangeName: "close-log-diagnostic-test", URL: closeEndpoint}
	require.NoError(t, unrelayedCloseConn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the unrelayed close-reason server")
	t.Cleanup(func() {
		assert.NoError(t, unrelayedCloseConn.Shutdown(), "Shutdown should close the unrelayed close-reason connection")
	})
	assert.Empty(t, unrelayedCloseConn.ReadMessage(), "ReadMessage should return an empty response when close-error relay is unavailable")
	require.NoError(t, <-closeWrites, "WriteMessage must send the unrelayed close-reason frame")

	invalidBinary := []byte("binary-frame-token")
	_, parseErr := (&connection{}).parseBinaryResponse(invalidBinary)
	require.Error(t, parseErr, "parseBinaryResponse must reject the invalid diagnostic frame")
	invalidEndpoint := newConnectionTestServer(t, func(conn *gws.Conn) {
		_ = conn.WriteMessage(gws.BinaryMessage, invalidBinary)
	})
	invalidConn := connection{ExchangeName: "binary-diagnostic-test", URL: invalidEndpoint}
	require.NoError(t, invalidConn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the invalid-binary diagnostic server")
	t.Cleanup(func() {
		assert.NoError(t, invalidConn.Shutdown(), "Shutdown should close the invalid-binary diagnostic connection")
	})
	response := invalidConn.ReadMessage()
	require.NotNil(t, response.Raw, "ReadMessage must return a non-nil invalid binary response")
	assert.Empty(t, response.Raw, "ReadMessage should omit invalid binary data")

	parsedEndpoint, err := url.Parse(invalidEndpoint)
	require.NoError(t, err, "Parse must parse the invalid-binary test endpoint")
	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	logMu.Unlock()
	for _, metadata := range []string{
		"close-log-diagnostic-test failed to relay websocket read error cause-type=*websocket.CloseError",
		fmt.Sprintf("binary frame parse failure wire-bytes=%d cause-type=%T", len(invalidBinary), parseErr),
		parsedEndpoint.Scheme + "://" + parsedEndpoint.Host,
	} {
		assert.Contains(t, diagnostics, metadata, "ReadMessage diagnostics should include safe connection metadata")
	}
	for _, secret := range []string{
		closeReason,
		invalidEndpoint,
		"path-token",
		"signature",
		"query-token",
		string(invalidBinary),
		parseErr.Error(),
	} {
		assert.NotContains(t, diagnostics, secret, "ReadMessage diagnostics should omit close, endpoint, frame, and error data")
	}
}

func TestNewGorillaPingHandler(t *testing.T) {
	t.Parallel()

	var typedNil *connectionTestNetError
	panickingErr := connectionTestPanickingNetError{}
	assert.Equal(t, errConnectionTest.Error(), panickingErr.Error(), "connectionTestPanickingNetError should return the test error text")
	assert.PanicsWithValue(t, "timeout-panic-token", func() {
		panickingErr.Timeout()
	}, "connectionTestPanickingNetError should panic when Timeout is called directly")
	wrappedTimeout := fmt.Errorf("wrapped timeout: %w", connectionTestNetError{timeout: true})
	joinedTimeout := errors.Join(panickingErr, connectionTestNetError{timeout: true})

	for _, tc := range []struct {
		name       string
		writeError error
		wantError  error
	}{
		{name: "success"},
		{name: "close sent", writeError: gws.ErrCloseSent},
		{name: "timeout", writeError: connectionTestNetError{timeout: true}},
		{name: "wrapped timeout", writeError: wrappedTimeout, wantError: wrappedTimeout},
		{name: "timeout after panicking sibling", writeError: joinedTimeout, wantError: joinedTimeout},
		{name: "typed nil network failure", writeError: typedNil, wantError: typedNil},
		{name: "panicking network failure", writeError: panickingErr, wantError: panickingErr},
		{name: "network failure", writeError: connectionTestNetError{}, wantError: connectionTestNetError{}},
		{name: "write failure", writeError: errConnectionTest, wantError: errConnectionTest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var messageType int
			var message []byte
			var deadline time.Time
			before := time.Now()
			handler := newGorillaPingHandler(func(mt int, payload []byte, dl time.Time) error {
				messageType = mt
				message = payload
				deadline = dl
				return tc.writeError
			}, PingHandler{MessageType: gws.PongMessage, Delay: time.Second})
			var err error
			assert.NotPanics(t, func() {
				err = handler("ping-payload-token")
			}, "newGorillaPingHandler should not panic while classifying write errors")
			after := time.Now()
			if tc.wantError == nil {
				require.NoError(t, err, "newGorillaPingHandler must suppress expected write results")
			} else {
				require.ErrorIs(t, err, tc.wantError, "newGorillaPingHandler must preserve unexpected write failures")
			}
			assert.Equal(t, gws.PongMessage, messageType, "newGorillaPingHandler should preserve the configured opcode")
			assert.Equal(t, []byte("ping-payload-token"), message, "newGorillaPingHandler should preserve the ping payload")
			assert.False(t, deadline.Before(before.Add(time.Second)), "newGorillaPingHandler should not set an early deadline")
			assert.False(t, deadline.After(after.Add(time.Second)), "newGorillaPingHandler should not set a late deadline")
		})
	}
}

func TestSendJSONMessageMarshalError(t *testing.T) {
	t.Parallel()

	endpoint := newConnectionTestServer(t, func(conn *gws.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	conn := connection{ExchangeName: "test", URL: endpoint}
	require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the marshal-error test server")
	t.Cleanup(func() {
		assert.NoError(t, conn.Shutdown(), "Shutdown should close the marshal-error test connection")
	})

	marshalErr := &connectionTestMarshalError{message: "marshal-cause-secret-token"}
	assert.Equal(t, "marshal-cause-secret-token", marshalErr.Error(), "connectionTestMarshalError should return the test canary")
	payload := connectionTestMarshalFailure{err: marshalErr}
	_, cause := payload.MarshalJSON()
	require.Error(t, cause, "MarshalJSON must return the test source error")
	assert.Same(t, marshalErr, cause, "MarshalJSON should return the exact test marshal error")
	err := conn.SendJSONMessage(t.Context(), request.Unset, payload)
	require.Error(t, err, "SendJSONMessage must return marshal failures")
	assert.ErrorIs(t, err, marshalErr, "SendJSONMessage should preserve the marshal source error")
	var target *connectionTestMarshalError
	require.ErrorAs(t, err, &target, "SendJSONMessage must preserve marshal source error inspection")
	assert.Same(t, marshalErr, target, "SendJSONMessage should preserve the exact marshal source error")
	assert.Contains(t, err.Error(), "websocket JSON write failed", "SendJSONMessage errors should retain safe operation context")
	assert.Contains(t, err.Error(), "*json.MarshalerError", "SendJSONMessage errors should retain marshal-error type metadata")
	assert.NotContains(t, err.Error(), marshalErr.Error(), "SendJSONMessage errors should omit marshal-cause text")
}

func TestSendMessageReturnResponsesWithInspectorAllPaths(t *testing.T) {
	t.Parallel()

	t.Run("marshal error", func(t *testing.T) {
		t.Parallel()
		conn := connection{Match: NewMatch()}
		sourceErr := &connectionTestMarshalError{message: "marshal-cause-secret-token"}
		payload := connectionTestMarshalFailure{err: sourceErr}
		_, directCause := payload.MarshalJSON()
		require.Error(t, directCause, "MarshalJSON must return the test source error")
		assert.Same(t, sourceErr, directCause, "MarshalJSON should return the exact test source error")
		_, marshalCause := gctjson.Marshal(payload)
		require.Error(t, marshalCause, "Marshal must reject the unsupported test payload")
		_, err := conn.SendMessageReturnResponsesWithInspector(t.Context(), request.Unset, "signature-token", payload, 1, nil)
		require.Error(t, err, "SendMessageReturnResponsesWithInspector must return marshal failures")
		assert.ErrorIs(t, err, sourceErr, "SendMessageReturnResponsesWithInspector should preserve the marshal source error")
		var target *connectionTestMarshalError
		require.ErrorAs(t, err, &target, "SendMessageReturnResponsesWithInspector must preserve marshal source error inspection")
		assert.Same(t, sourceErr, target, "SendMessageReturnResponsesWithInspector should preserve the exact marshal source error")
		assert.Contains(t, err.Error(), fmt.Sprintf("%T", marshalCause), "SendMessageReturnResponsesWithInspector errors should preserve marshal-error type metadata")
		assert.NotContains(t, err.Error(), "signature-token", "SendMessageReturnResponsesWithInspector errors should omit signature values")
		assert.NotContains(t, err.Error(), marshalCause.Error(), "SendMessageReturnResponsesWithInspector errors should omit marshal-cause text")
	})

	t.Run("match setup error", func(t *testing.T) {
		t.Parallel()
		conn := connection{Match: NewMatch()}
		_, err := conn.SendMessageReturnResponsesWithInspector(t.Context(), request.Unset, "signature-token", struct{}{}, 0, nil)
		require.ErrorIs(t, err, errInvalidBufferSize, "SendMessageReturnResponsesWithInspector must return match setup failures")
	})

	t.Run("send error", func(t *testing.T) {
		t.Parallel()
		conn := connection{Match: NewMatch()}
		_, err := conn.SendMessageReturnResponsesWithInspector(t.Context(), request.Unset, "signature-token", struct{}{}, 1, nil)
		require.ErrorIs(t, err, errWebsocketIsDisconnected, "SendMessageReturnResponsesWithInspector must return send failures")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		endpoint := newConnectionTestServer(t, func(conn *gws.Conn) {
			_, payload, err := conn.ReadMessage()
			if err == nil {
				_ = conn.WriteMessage(gws.TextMessage, payload)
			}
		})
		reporter := &connectionTestReporter{}
		conn := connection{ExchangeName: "test", URL: endpoint, ResponseMaxLimit: time.Second, Match: NewMatch(), Reporter: reporter}
		require.NoError(t, conn.Dial(t.Context(), &gws.Dialer{}, nil, nil), "Dial must connect to the echo server")
		matched := make(chan bool, 1)
		go func() {
			response := conn.ReadMessage()
			matched <- conn.Match.IncomingWithData("signature-token", response.Raw)
		}()
		responses, err := conn.SendMessageReturnResponsesWithInspector(t.Context(), request.Unset, "signature-token", map[string]string{"secret": "payload-token"}, 1, nil)
		require.NoError(t, err, "SendMessageReturnResponsesWithInspector must receive the matched response")
		require.Len(t, responses, 1, "SendMessageReturnResponsesWithInspector must return one response")
		assert.True(t, <-matched, "SendMessageReturnResponsesWithInspector should route the response")
		assert.JSONEq(t, `{"secret":"payload-token"}`, string(reporter.message), "SendMessageReturnResponsesWithInspector should preserve reporter payloads")
	})
}

func TestMatchReturnResponses(t *testing.T) {
	t.Parallel()

	conn := connection{Match: NewMatch()}
	_, err := conn.MatchReturnResponses(t.Context(), nil, 0)
	require.ErrorIs(t, err, errInvalidBufferSize)

	ch, err := conn.MatchReturnResponses(t.Context(), nil, 1)
	require.NoError(t, err)

	require.ErrorIs(t, (<-ch).Err, ErrSignatureTimeout)
	conn.ResponseMaxLimit = time.Second

	ch, err = conn.MatchReturnResponses(t.Context(), nil, 1)
	require.NoError(t, err)

	exp := []byte("test")
	require.True(t, conn.Match.IncomingWithData(nil, exp))
	resp := <-ch
	require.NoError(t, resp.Err)
	require.NotEmpty(t, resp.Responses, "must have response data")
	assert.Equal(t, exp, resp.Responses[0])
}

func TestWebsocketConnectionRequireMatchWithData(t *testing.T) {
	t.Parallel()
	ws := connection{Match: NewMatch()}
	err := ws.RequireMatchWithData(0, nil)
	require.ErrorIs(t, err, ErrSignatureNotMatched)

	ch, err := ws.Match.Set(0, 1)
	require.NoError(t, err)

	err = ws.RequireMatchWithData(0, []byte("test"))
	require.NoError(t, err)
	require.Len(t, ch, 1, "must have one item in channel")
	assert.Equal(t, []byte("test"), <-ch)
}

func TestIncomingWithData(t *testing.T) {
	t.Parallel()
	ws := connection{Match: NewMatch()}
	require.False(t, ws.IncomingWithData(0, nil))

	ch, err := ws.Match.Set(0, 1)
	require.NoError(t, err)

	require.True(t, ws.IncomingWithData(0, []byte("test")))
	require.Len(t, ch, 1, "must have one item in channel")
	assert.Equal(t, []byte("test"), <-ch)
}

func TestConnectionSubscriptions(t *testing.T) {
	t.Parallel()
	ws := &connection{}
	require.Nil(t, ws.Subscriptions())
	ws.subscriptions = subscription.NewStore()
	require.NotNil(t, ws.Subscriptions())
	testsubs.EqualLists(t, ws.subscriptions.List(), ws.Subscriptions().List())
}
