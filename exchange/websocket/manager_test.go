package websocket

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchanges/protocol"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	mockws "github.com/thrasher-corp/gocryptotrader/internal/testing/websocket"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
)

const (
	Ping               = "ping"
	useProxyTests      = false                     // Disabled by default. Freely available proxy servers that work all the time are difficult to find
	proxyURL           = "http://212.186.171.4:80" // Replace with a usable proxy server
	testString         = "test"
	testTrafficTimeout = time.Second
)

var errDastardlyReason = errors.New("some dastardly reason")

func noopConnect() error { return nil }

func closeChanNoPanic(ch chan struct{}) {
	if ch == nil {
		return
	}
	defer func() { _ = recover() }()
	close(ch)
}

func restoreShutdownChannel(ws *Manager) {
	if ws == nil {
		return
	}
	ws.m.Lock()
	defer ws.m.Unlock()
	ws.ShutdownC = make(chan struct{})
	for i := range ws.connectionManager {
		for j := range ws.connectionManager[i].connections {
			if conn, ok := ws.connectionManager[i].connections[j].(*connection); ok {
				conn.shutdown = ws.ShutdownC
			}
		}
	}
}

// resetManagerForNextConnectAttempt waits for monitor goroutines to drain during test cleanup.
// It intentionally avoids mutating ShutdownC directly.
func resetManagerForNextConnectAttempt(t *testing.T, ws *Manager) {
	t.Helper()
	if ws == nil {
		return
	}
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		ws.Wg.Wait()
	}()
	require.Eventually(t, func() bool {
		select {
		case <-waitDone:
			return true
		default:
			return false
		}
	}, 5*time.Second, 20*time.Millisecond, "manager cleanup must wait for monitor goroutines")
}

func cleanupManagerMonitors(t *testing.T, ws *Manager) {
	t.Helper()
	if ws == nil {
		return
	}
	ws.setEnabled(false)
	err := ws.Shutdown()
	if err != nil {
		if errors.Is(err, ErrNotConnected) || errors.Is(err, errAlreadyReconnecting) {
			closeChanNoPanic(ws.ShutdownC)
			resetManagerForNextConnectAttempt(t, ws)
			require.Eventually(t, func() bool {
				return !ws.connectionMonitorRunning.Load()
			}, 5*time.Second, 20*time.Millisecond, "connection monitor must stop during cleanup")
			restoreShutdownChannel(ws)
			return
		}
		t.Fatalf("manager shutdown cleanup failed: %v", err)
	}
	resetManagerForNextConnectAttempt(t, ws)
	require.Eventually(t, func() bool {
		return !ws.connectionMonitorRunning.Load()
	}, 5*time.Second, 20*time.Millisecond, "connection monitor must stop during cleanup")
}

type testStruct struct {
	Error error
	WC    connection
}

type diagnosticConnection struct {
	Connection
	url           string
	proxy         string
	responses     []Response
	subscriptions *subscription.Store
	shutdownErr   error
}

func (d *diagnosticConnection) ReadMessage() Response {
	if len(d.responses) == 0 {
		return Response{}
	}
	response := d.responses[0]
	d.responses = d.responses[1:]
	return response
}

func (d *diagnosticConnection) GetURL() string {
	return d.url
}

func (d *diagnosticConnection) SetURL(u string) {
	d.url = u
}

func (d *diagnosticConnection) SetProxy(proxy string) {
	d.proxy = proxy
}

func (d *diagnosticConnection) Shutdown() error {
	return d.shutdownErr
}

func (d *diagnosticConnection) Subscriptions() *subscription.Store {
	return d.subscriptions
}

type testRequest struct {
	Event        string          `json:"event"`
	RequestID    int64           `json:"reqid,omitempty"`
	Pairs        []string        `json:"pair"`
	Subscription testRequestData `json:"subscription"`
}

// testRequestData contains details on WS channel
type testRequestData struct {
	Name     string `json:"name,omitempty"`
	Interval int64  `json:"interval,omitempty"`
	Depth    int64  `json:"depth,omitempty"`
}

type testResponse struct {
	RequestID int64 `json:"reqid,omitempty"`
}

type testSubKey struct {
	Mood string
}

func newDefaultSetup() *ManagerSetup {
	return &ManagerSetup{
		ExchangeConfig: &config.Exchange{
			Features: &config.FeaturesConfig{
				Enabled: config.FeaturesEnabledConfig{Websocket: true},
			},
			API: config.APIConfig{
				AuthenticatedWebsocketSupport: true,
			},
			WebsocketTrafficTimeout: testTrafficTimeout,
			Name:                    "GTX",
		},
		DefaultURL:   "testDefaultURL",
		RunningURL:   "wss://testRunningURL",
		Connector:    func() error { return nil },
		Subscriber:   func(subscription.List) error { return nil },
		Unsubscriber: func(subscription.List) error { return nil },
		GenerateSubscriptions: func() (subscription.List, error) {
			return subscription.List{
				{Channel: "TestSub"},
				{Channel: "TestSub2", Key: "purple"},
				{Channel: "TestSub3", Key: testSubKey{"mauve"}},
				{Channel: "TestSub4", Key: 42},
			}, nil
		},
		Features: &protocol.Features{Subscribe: true, Unsubscribe: true},
	}
}

func TestSetup(t *testing.T) {
	t.Parallel()
	var w *Manager
	err := w.Setup(nil)
	assert.ErrorContains(t, err, "nil pointer: *websocket.Manager")

	w = &Manager{DataHandler: stream.NewRelay(1)}
	err = w.Setup(nil)
	assert.ErrorContains(t, err, "nil pointer: *websocket.ManagerSetup")

	websocketSetup := &ManagerSetup{}
	err = w.Setup(websocketSetup)
	assert.ErrorContains(t, err, "nil pointer: ManagerSetup.Exchange")

	websocketSetup.ExchangeConfig = &config.Exchange{}
	err = w.Setup(websocketSetup)
	assert.ErrorContains(t, err, "nil pointer: ManagerSetup.ExchangeConfig.Features")

	websocketSetup.ExchangeConfig.Features = &config.FeaturesConfig{}
	err = w.Setup(websocketSetup)
	assert.ErrorContains(t, err, "nil pointer: ManagerSetup.Features")

	websocketSetup.Features = &protocol.Features{}
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errExchangeConfigNameEmpty)

	websocketSetup.ExchangeConfig.Name = "testname"
	websocketSetup.Subscriber = func(subscription.List) error { return nil } // kicks off the setup
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errWebsocketConnectorUnset)
	websocketSetup.Subscriber = nil

	websocketSetup.Connector = func() error { return nil }
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errWebsocketSubscriberUnset)

	websocketSetup.Subscriber = func(subscription.List) error { return nil }
	w.features.Unsubscribe = true
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errWebsocketUnsubscriberUnset)

	websocketSetup.Unsubscriber = func(subscription.List) error { return nil }
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errWebsocketSubscriptionsGeneratorUnset)

	websocketSetup.GenerateSubscriptions = func() (subscription.List, error) { return nil, nil }
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errDefaultURLIsEmpty)

	websocketSetup.DefaultURL = testString
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errRunningURLIsEmpty)

	websocketSetup.RunningURL = "http://www.google.com"
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errInvalidWebsocketURL)

	websocketSetup.RunningURL = "wss://www.google.com"
	websocketSetup.RunningURLAuth = "http://www.google.com"
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errInvalidWebsocketURL)

	websocketSetup.RunningURLAuth = "wss://www.google.com"
	err = w.Setup(websocketSetup)
	assert.ErrorIs(t, err, errInvalidTrafficTimeout)

	websocketSetup.ExchangeConfig.WebsocketTrafficTimeout = time.Minute
	err = w.Setup(websocketSetup)
	assert.NoError(t, err, "Setup should not error")
}

func TestConnectionMessageErrors(t *testing.T) { //nolint:tparallel // top-level parallel is safe; serial subtests limit websocket CI contention
	t.Parallel()

	newSingleManager := func(t *testing.T) *Manager {
		t.Helper()

		ws := NewManager()
		t.Cleanup(func() { cleanupManagerMonitors(t, ws) })
		return ws
	}

	newConfiguredSingleManager := func(t *testing.T) *Manager {
		t.Helper()

		ws := newSingleManager(t)
		err := ws.Setup(newDefaultSetup())
		require.NoError(t, err, "Setup must not error")
		ws.trafficTimeout = time.Minute
		return ws
	}

	newConfiguredMultiManager := func(t *testing.T, connSetup *ConnectionSetup) *Manager {
		t.Helper()

		ws := newSingleManager(t)
		setup := newDefaultSetup()
		setup.UseMultiConnectionManagement = true
		err := ws.Setup(setup)
		require.NoError(t, err, "Setup must not error")
		ws.SetCanUseAuthenticatedEndpoints(true)
		if connSetup != nil {
			ws.connectionManager = []*websocket{{setup: connSetup}}
		}
		return ws
	}

	t.Run("single connection preflight", func(t *testing.T) {
		t.Run("disabled websocket", func(t *testing.T) {
			ws := newSingleManager(t)
			ws.connector = noopConnect

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, ErrWebsocketNotEnabled, "Connect must error correctly")
		})

		t.Run("already reconnecting", func(t *testing.T) {
			ws := newSingleManager(t)
			ws.setEnabled(true)
			ws.setState(connectingState)

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errAlreadyReconnecting, "Connect must error correctly")
		})

		t.Run("nil subscriptions", func(t *testing.T) {
			ws := newSingleManager(t)
			ws.setEnabled(true)
			ws.setState(disconnectedState)
			ws.connector = noopConnect
			ws.subscriptions = nil

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, common.ErrNilPointer, "Connect must get a nil pointer error")
			require.ErrorContains(t, err, "subscriptions", "Connect must get a nil pointer error about subscriptions")
		})

		t.Run("connector error", func(t *testing.T) {
			ws := newSingleManager(t)
			ws.setEnabled(true)
			ws.setState(disconnectedState)
			ws.connector = func() error { return errDastardlyReason }

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errDastardlyReason, "Connect must error correctly")
		})
	})

	t.Run("single connection requires subscriptions", func(t *testing.T) {
		ws := newConfiguredSingleManager(t)

		require.ErrorIs(t, ws.Connect(t.Context()), ErrSubscriptionsNotAdded)
		require.NoError(t, ws.Shutdown())
	})

	t.Run("single connection forwards read errors to data handler", func(t *testing.T) {
		ws := newConfiguredSingleManager(t)
		ws.Subscriber = func(subs subscription.List) error {
			for _, sub := range subs {
				if err := ws.subscriptions.Add(sub); err != nil {
					return err
				}
			}
			return nil
		}

		require.NoError(t, ws.Connect(t.Context()), "Connect must not error")

		checkToRoutineResult := func(t *testing.T) {
			t.Helper()

			v, ok := <-ws.DataHandler.C
			require.True(t, ok, "ToRoutine must not be closed on us")

			switch err := v.Data.(type) {
			case *gws.CloseError:
				assert.Equal(t, "SpecialText", err.Text, "Should get correct Close Error")
			case error:
				assert.ErrorIs(t, err, errDastardlyReason, "Should get the correct error")
			default:
				assert.Failf(t, "Wrong data type sent to ToRoutine", "Got type: %T", err)
			}
		}

		ws.TrafficAlert <- struct{}{}
		ws.ReadMessageErrors <- errDastardlyReason
		checkToRoutineResult(t)

		ws.ReadMessageErrors <- &gws.CloseError{Code: 1006, Text: "SpecialText"}
		checkToRoutineResult(t)
	})

	t.Run("multi connection", func(t *testing.T) {
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler)
		}))
		t.Cleanup(mock.Close)

		mockURL := "ws" + mock.URL[len("http"):] + "/ws"
		dial := func(ctx context.Context, conn Connection) error {
			return conn.Dial(ctx, gws.DefaultDialer, nil, nil)
		}
		noopHandler := func(context.Context, Connection, []byte) error { return nil }
		testSubs := subscription.List{{Channel: testString}}

		t.Run("no pending connections", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, nil)

			err := ws.Connect(t.Context())
			assert.ErrorIs(t, err, errNoPendingConnections, "Connect should error correctly")
		})

		t.Run("missing generate subscriptions", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{URL: mockURL})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errWebsocketSubscriptionsGeneratorUnset)
		})

		t.Run("generate subscriptions error", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				GenerateSubscriptions: func() (subscription.List, error) {
					return nil, errDastardlyReason
				},
			})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errDastardlyReason)
		})

		t.Run("missing connector", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				GenerateSubscriptions: func() (subscription.List, error) {
					return testSubs, nil
				},
			})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errNoConnectFunc)
		})

		t.Run("missing handler", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				GenerateSubscriptions: func() (subscription.List, error) {
					return testSubs, nil
				},
				Connector: func(context.Context, Connection) error {
					return errDastardlyReason
				},
			})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errWebsocketDataHandlerUnset)
		})

		t.Run("missing subscriber", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				GenerateSubscriptions: func() (subscription.List, error) {
					return testSubs, nil
				},
				Connector: func(context.Context, Connection) error {
					return errDastardlyReason
				},
				Handler: noopHandler,
			})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errWebsocketSubscriberUnset)
		})

		t.Run("connector error", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				GenerateSubscriptions: func() (subscription.List, error) {
					return testSubs, nil
				},
				Connector: func(context.Context, Connection) error {
					return errDastardlyReason
				},
				Handler:    noopHandler,
				Subscriber: func(context.Context, Connection, subscription.List) error { return nil },
			})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errDastardlyReason)
		})

		t.Run("authenticate error", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				Authenticate: func(context.Context, Connection) error {
					return errDastardlyReason
				},
				GenerateSubscriptions: func() (subscription.List, error) {
					return testSubs, nil
				},
				Connector:  dial,
				Handler:    noopHandler,
				Subscriber: func(context.Context, Connection, subscription.List) error { return nil },
			})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errDastardlyReason)
		})

		t.Run("subscriber error", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				GenerateSubscriptions: func() (subscription.List, error) {
					return testSubs, nil
				},
				Connector: dial,
				Handler:   noopHandler,
				Subscriber: func(context.Context, Connection, subscription.List) error {
					return errDastardlyReason
				},
			})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, errDastardlyReason)
			require.NoError(t, ws.Shutdown())
		})

		t.Run("missing recorded subscriptions", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				GenerateSubscriptions: func() (subscription.List, error) {
					return testSubs, nil
				},
				Connector:  dial,
				Handler:    noopHandler,
				Subscriber: func(context.Context, Connection, subscription.List) error { return nil },
			})
			ws.connectionManager[0].subscriptions = subscription.NewStore()

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, ErrSubscriptionsNotAdded)
			require.NoError(t, ws.Shutdown())
		})

		t.Run("successful connect and send raw message", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL: mockURL,
				GenerateSubscriptions: func() (subscription.List, error) {
					return testSubs, nil
				},
				Connector: dial,
				Handler:   noopHandler,
			})
			ws.connectionManager[0].subscriptions = subscription.NewStore()
			ws.connectionManager[0].setup.Subscriber = func(context.Context, Connection, subscription.List) error {
				return ws.connectionManager[0].subscriptions.Add(&subscription.Subscription{Channel: testString})
			}

			err := ws.Connect(t.Context())
			require.NoError(t, err)

			err = ws.connectionManager[0].connections[0].SendRawMessage(t.Context(), request.Unset, gws.TextMessage, []byte(testString))
			require.NoError(t, err)
			require.NoError(t, ws.Shutdown())
		})

		t.Run("subscriptions not required", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL:                      mockURL,
				SubscriptionsNotRequired: true,
				Connector:                dial,
				Handler:                  noopHandler,
			})

			err := ws.Connect(t.Context())
			require.NoError(t, err, "must not error when connection when no subscriptions are required")
			require.NoError(t, ws.Shutdown())
		})

		t.Run("subscriptions not required connector failure", func(t *testing.T) {
			ws := newConfiguredMultiManager(t, &ConnectionSetup{
				URL:                      mockURL,
				SubscriptionsNotRequired: true,
				Connector: func(context.Context, Connection) error {
					return errors.New("no connect")
				},
				Handler: noopHandler,
			})

			err := ws.Connect(t.Context())
			require.ErrorIs(t, err, common.ErrFatal, "must error on connect when no subscriptions are required")
		})
	})
}

func TestConnectTrackOnExistingConnectionManagerRecordsTrackedSubscriptions(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	setup := newDefaultSetup()
	setup.UseMultiConnectionManagement = true
	require.NoError(t, mgr.Setup(setup))
	trackedSub := &subscription.Subscription{Channel: "tracked-only"}

	require.NoError(t, mgr.SetupNewConnection(&ConnectionSetup{
		URL: "wss://tracked-only.example/ws",
		Connector: func(context.Context, Connection) error {
			return errors.New("connector should not be called for tracked-only batch")
		},
		GenerateSubscriptions: func() (subscription.List, error) {
			return subscription.List{trackedSub}, nil
		},
		Subscriber:   func(context.Context, Connection, subscription.List) error { return nil },
		Unsubscriber: func(context.Context, Connection, subscription.List) error { return nil },
		Handler:      func(context.Context, Connection, []byte) error { return nil },
		TrackOnExistingConnection: func(context.Context, Connection, subscription.List) (subscription.List, subscription.List, error) {
			return nil, subscription.List{trackedSub}, nil
		},
	}))

	existingConn := &fakeConnection{subscriptions: subscription.NewStore()}
	mgr.trackConnection(existingConn, mgr.connectionManager[0])

	require.NoError(t, mgr.Connect(t.Context()))
	require.NotNil(t, mgr.connectionManager[0].subscriptions.Get(trackedSub), "tracked subscriptions must be recorded by manager")

	mgr.setEnabled(false)
	require.NoError(t, mgr.Shutdown())
}

func TestCreateConnectAndSubscribe(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.MaxSubscriptionsPerConnection = 1

	ws := &websocket{subscriptions: subscription.NewStore(), setup: &ConnectionSetup{}}
	subs := subscription.List{{Channel: "one"}, {Channel: "two"}}
	err := mgr.createConnectAndSubscribe(t.Context(), ws, subs)
	require.ErrorIs(t, err, common.ErrFatal, "must return fatal error when exceeding max subscriptions")
	assert.ErrorIs(t, err, errSubscriptionsExceedsLimit, "should return the subscriptions exceeds limit error")

	mgr.MaxSubscriptionsPerConnection = 0
	ws.setup.Connector = func(context.Context, Connection) error { return errConnectionFault }
	err = mgr.createConnectAndSubscribe(t.Context(), ws, subs)
	require.ErrorIs(t, err, common.ErrFatal, "must return fatal error when calling ws.setup.Connector")
	assert.ErrorIs(t, err, errConnectionFault, "should return the correct error when calling ws.setup.Connector")

	ws.setup.Connector = func(context.Context, Connection) error { return nil }
	err = mgr.createConnectAndSubscribe(t.Context(), ws, subs)
	require.ErrorIs(t, err, common.ErrFatal, "must return fatal error when not connected after a potential failed ws.setup.Connector call")
	assert.ErrorIs(t, err, ErrNotConnected, "should signal connection not established")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler)
	}))
	t.Cleanup(server.Close)

	ws.setup.URL = "ws" + server.URL[len("http"):] + "/ws"
	ws.setup.Handler = func(context.Context, Connection, []byte) error { return nil }
	ws.setup.Connector = func(ctx context.Context, conn Connection) error {
		return conn.Dial(ctx, gws.DefaultDialer, nil, nil)
	}
	ws.setup.Authenticate = func(context.Context, Connection) error { return errConnectionFault }
	mgr.SetCanUseAuthenticatedEndpoints(true)

	err = mgr.createConnectAndSubscribe(t.Context(), ws, subs)
	require.ErrorIs(t, err, common.ErrFatal, "authenticate failure must be fatal")
	assert.ErrorIs(t, err, errConnectionFault, "should wrap authentication failure reason")
	assert.ErrorIs(t, err, errFailedToAuthenticate, "should wrap authentication failure")
	require.Len(t, ws.connections, 1, "connection must be tracked by websocket")
	require.Len(t, mgr.connections, 1, "websocket connection association must be tracked by manager")
	require.Equal(t, mgr.connections[ws.connections[0]], ws, "manager connections map must track the websocket owner")
	require.NoError(t, ws.connections[0].Shutdown())
	delete(mgr.connections, ws.connections[0])
	ws.connections = nil
	mgr.Wg.Wait()

	ws.setup.Authenticate = func(context.Context, Connection) error { return nil }
	ws.setup.SubscriptionsNotRequired = true
	err = mgr.createConnectAndSubscribe(t.Context(), ws, subs)
	require.ErrorIs(t, err, common.ErrFatal, "subscriptions not required must error when subscriptions are provided")
	require.ErrorIs(t, err, ErrSubscriptionFailure, "subscriptions not required must error when subscriptions are provided")
	require.Len(t, ws.connections, 1, "connection must be tracked by websocket")
	require.Len(t, mgr.connections, 1, "websocket connection association must be tracked by manager")
	require.Equal(t, mgr.connections[ws.connections[0]], ws, "manager connections map must track the websocket owner")
	require.NoError(t, ws.connections[0].Shutdown())
	delete(mgr.connections, ws.connections[0])
	ws.connections = nil
	mgr.Wg.Wait()

	err = mgr.createConnectAndSubscribe(t.Context(), ws, nil)
	require.NoError(t, err, "subscriptions not required with no subscriptions must not error")
	require.Len(t, ws.connections, 1, "connection must be tracked by websocket")
	require.Len(t, mgr.connections, 1, "websocket connection association must be tracked by manager")
	require.Equal(t, mgr.connections[ws.connections[0]], ws, "manager connections map must track the websocket owner")
	require.NoError(t, ws.connections[0].Shutdown())
	delete(mgr.connections, ws.connections[0])
	ws.connections = nil
	mgr.Wg.Wait()

	ws.setup.SubscriptionsNotRequired = false
	ws.setup.Subscriber = func(context.Context, Connection, subscription.List) error {
		return errConnectionFault
	}
	err = mgr.createConnectAndSubscribe(t.Context(), ws, subs)
	require.ErrorIs(t, err, ErrSubscriptionFailure, "subscriber error must bubble as subscription failure")
	assert.ErrorIs(t, err, errConnectionFault, "should include wrapped error")
	require.Len(t, ws.connections, 1, "connection must be tracked by websocket")
	require.Len(t, mgr.connections, 1, "websocket connection association must be tracked by manager")
	require.Equal(t, mgr.connections[ws.connections[0]], ws, "manager connections map must track the websocket owner")
	require.NoError(t, ws.connections[0].Shutdown())
	delete(mgr.connections, ws.connections[0])
	ws.connections = nil
	mgr.Wg.Wait()

	ws.setup.Subscriber = func(context.Context, Connection, subscription.List) error {
		return nil
	}
	err = mgr.createConnectAndSubscribe(t.Context(), ws, subs)
	require.ErrorIs(t, err, ErrSubscriptionFailure, "missing added subscriptions must return subscription failure error")
	require.ErrorIs(t, err, ErrSubscriptionsNotAdded, "missing added subscriptions must return subs not added error")
	require.Len(t, ws.connections, 1, "connection must be tracked by websocket")
	require.Len(t, mgr.connections, 1, "websocket connection association must be tracked by manager")
	require.Equal(t, mgr.connections[ws.connections[0]], ws, "manager connections map must track the websocket owner")
	require.NoError(t, ws.connections[0].Shutdown())
	delete(mgr.connections, ws.connections[0])
	ws.connections = nil
	mgr.Wg.Wait()

	ws.setup.Subscriber = func(context.Context, Connection, subscription.List) error {
		for _, sub := range subs {
			if err := ws.subscriptions.Add(sub); err != nil {
				return err
			}
		}
		return nil
	}
	err = mgr.createConnectAndSubscribe(t.Context(), ws, subs)
	require.NoError(t, err, "createConnectAndSubscribe must succeed")
	require.Len(t, ws.connections, 1, "connection must be tracked by websocket")
	require.Len(t, mgr.connections, 1, "websocket connection association must be tracked by manager")
	require.Equal(t, mgr.connections[ws.connections[0]], ws, "manager connections map must track the websocket owner")
	require.Len(t, ws.connections[0].Subscriptions().List(), len(subs), "connection subscription store must mirror websocket store")
	require.NoError(t, ws.connections[0].Shutdown())
	delete(mgr.connections, ws.connections[0])
	ws.connections = nil
	mgr.Wg.Wait()
}

func TestTrackConnection(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	conn := &connection{}
	first := &websocket{}
	second := &websocket{}

	mgr.trackConnection(conn, first)
	mgr.trackConnection(conn, first)

	require.Len(t, mgr.connections, 1, "manager connection association must stay deduplicated")
	require.Len(t, first.connections, 1, "websocket connection list must not append duplicates")
	assert.Same(t, first, mgr.connections[conn], "manager connection association should stay with the original websocket")
	assert.Same(t, conn, first.connections[0], "websocket should retain the tracked connection")

	assert.PanicsWithValue(t,
		"trackConnection called with connection already associated with a different websocket",
		func() { mgr.trackConnection(conn, second) },
		"trackConnection should panic when the same connection is associated with a different websocket")
	assert.Same(t, first, mgr.connections[conn], "manager connection association should remain unchanged after panic")
	require.Len(t, first.connections, 1, "original websocket must retain the tracked connection after panic")
	assert.Same(t, conn, first.connections[0], "original websocket should still retain the tracked connection")
	assert.Empty(t, second.connections, "new websocket should not gain the tracked connection after panic")
}

func TestSetSubscriptionsNotRequired(t *testing.T) {
	t.Parallel()

	singleConn := NewManager()
	singleConn.GenerateSubs = func() (subscription.List, error) {
		return subscription.List{{Channel: "single"}}, nil
	}

	singleConn.SetSubscriptionsNotRequired()

	subs, err := singleConn.GenerateSubs()
	require.NoError(t, err, "GenerateSubs must not error after subscriptions are disabled")
	assert.Empty(t, subs, "GenerateSubs should return no subscriptions after subscriptions are disabled")

	multiConn := NewManager()
	multiConn.useMultiConnectionManagement = true
	multiConn.connectionManager = []*websocket{
		{setup: nil},
		{setup: &ConnectionSetup{}},
		{setup: &ConnectionSetup{SubscriptionsNotRequired: true}},
	}

	multiConn.SetSubscriptionsNotRequired()

	for i := range multiConn.connectionManager {
		require.NotNil(t,
			multiConn.connectionManager[i].setup,
			"connection setup must be initialised when missing")
		assert.True(t,
			multiConn.connectionManager[i].setup.SubscriptionsNotRequired,
			"connection setup should not require subscriptions after override")
	}
}

func TestSetAllConnectionURLs(t *testing.T) {
	t.Parallel()

	singleConn := NewManager()
	singleConn.Conn = &connection{URL: "ws://old-public.example.com"}
	singleConn.AuthConn = &connection{URL: "ws://old-auth.example.com"}

	err := singleConn.SetAllConnectionURLs("ws://mock.example.com/ws")
	require.NoError(t, err, "SetAllConnectionURLs must not error for single-connection managers")
	assert.Equal(t, "ws://mock.example.com/ws", singleConn.runningURL, "runningURL should be updated for single-connection managers")
	assert.Equal(t, "ws://mock.example.com/ws", singleConn.runningURLAuth, "runningURLAuth should be updated for single-connection managers")
	assert.Equal(t, "ws://mock.example.com/ws", singleConn.Conn.GetURL(), "Conn URL should be updated for single-connection managers")
	assert.Equal(t, "ws://mock.example.com/ws", singleConn.AuthConn.GetURL(), "AuthConn URL should be updated for single-connection managers")

	multiConn := NewManager()
	multiConn.useMultiConnectionManagement = true
	multiConn.connectionManager = []*websocket{
		{setup: nil},
		{setup: &ConnectionSetup{URL: "ws://first.example.com"}},
		{setup: &ConnectionSetup{URL: "ws://second.example.com"}, connections: []Connection{&connection{URL: "ws://live.example.com"}}},
	}

	err = multiConn.SetAllConnectionURLs("ws://mock.example.com/ws")
	require.NoError(t, err, "SetAllConnectionURLs must not error for multi-connection managers")

	for i := range multiConn.connectionManager {
		require.NotNil(t,
			multiConn.connectionManager[i].setup,
			"connection setup must be initialised when missing")
		assert.Equal(t,
			"ws://mock.example.com/ws",
			multiConn.connectionManager[i].setup.URL,
			"connection setup URL should be updated for each multi-connection setup")
	}
	assert.Equal(t,
		"ws://live.example.com",
		multiConn.connectionManager[2].connections[0].GetURL(),
		"existing live connection URL should not be mutated by the pre-connect helper")
}

func TestSetAllConnectionURLsErrorsAfterConnect(t *testing.T) {
	t.Parallel()

	ws := NewManager()

	err := ws.SetAllConnectionURLs("ws://mock.example.com/ws")
	require.NoError(t, err, "SetAllConnectionURLs must allow pre-connect configuration")

	ws.setState(connectingState)
	err = ws.SetAllConnectionURLs("ws://mock.example.com/ws")
	require.ErrorIs(t, err, errAlreadyReconnecting, "SetAllConnectionURLs must error once Connect has started")
	require.ErrorContains(t, err, "SetAllConnectionURLs must be called before Connect")

	ws.setState(connectedState)
	err = ws.SetAllConnectionURLs("ws://mock.example.com/ws")
	require.ErrorIs(t, err, errAlreadyConnected, "SetAllConnectionURLs must error after connect")
	require.ErrorContains(t, err, "SetAllConnectionURLs must be called before Connect")
}

func TestManager(t *testing.T) {
	t.Parallel()

	ws := NewManager()

	err := ws.SetProxyAddress(t.Context(), "garbagio")
	assert.Error(t, err, "SetProxyAddress should error correctly")
	assert.NotContains(t, err.Error(), "garbagio", "SetProxyAddress should omit the invalid proxy address")

	ws.setEnabled(true)
	defaultSetup := newDefaultSetup()
	err = ws.Setup(defaultSetup) // Sets to enabled again
	require.NoError(t, err, "Setup may not error")

	err = ws.Setup(defaultSetup)
	assert.ErrorIs(t, err, errWebsocketAlreadyInitialised, "Setup should error correctly if called twice")

	assert.Equal(t, "GTX", ws.GetName(), "GetName should return correctly")
	assert.True(t, ws.IsEnabled(), "Websocket should be enabled by Setup")

	ws.setEnabled(false)
	assert.False(t, ws.IsEnabled(), "Websocket should be disabled by setEnabled(false)")

	ws.setEnabled(true)
	assert.True(t, ws.IsEnabled(), "Websocket should be enabled by setEnabled(true)")

	err = ws.SetProxyAddress(t.Context(), "https://192.168.0.1:1337")
	assert.NoError(t, err, "SetProxyAddress should not error when not yet connected")

	ws.setState(connectedState)

	ws.connector = func() error { return errDastardlyReason }
	err = ws.SetProxyAddress(t.Context(), "https://192.168.0.1:1336")
	assert.ErrorIs(t, err, errDastardlyReason, "SetProxyAddress should call Connect and error from there")

	err = ws.SetProxyAddress(t.Context(), "https://192.168.0.1:1336")
	assert.ErrorIs(t, err, errSameProxyAddress, "SetProxyAddress should error correctly")

	// removing proxy
	assert.NoError(t, ws.SetProxyAddress(t.Context(), ""))

	ws.setEnabled(true)
	// reinstate proxy
	err = ws.SetProxyAddress(t.Context(), "http://localhost:1337")
	assert.NoError(t, err, "SetProxyAddress should not error")
	assert.Equal(t, "http://localhost:1337", ws.GetProxyAddress(), "GetProxyAddress should return correctly")
	assert.Equal(t, "wss://testRunningURL", ws.GetWebsocketURL(), "GetWebsocketURL should return correctly")
	assert.Equal(t, testTrafficTimeout, ws.trafficTimeout, "trafficTimeout should default correctly")

	assert.ErrorIs(t, ws.Shutdown(), ErrNotConnected)
	ws.setState(connectedState)
	assert.NoError(t, ws.Shutdown())

	ws.connector = func() error { return nil }

	require.ErrorIs(t, ws.Connect(t.Context()), ErrSubscriptionsNotAdded)
	require.NoError(t, ws.Shutdown())

	ws.Subscriber = func(subs subscription.List) error {
		for _, sub := range subs {
			if err := ws.subscriptions.Add(sub); err != nil {
				return err
			}
		}
		return nil
	}
	assert.NoError(t, ws.Connect(t.Context()), "Connect should not error")

	ws.defaultURL = "ws://demos.kaazing.com/echo"
	ws.defaultURLAuth = "ws://demos.kaazing.com/echo"

	err = ws.SetWebsocketURL("", false, false)
	assert.NoError(t, err, "SetWebsocketURL should not error")

	err = ws.SetWebsocketURL("ws://demos.kaazing.com/echo", false, false)
	assert.NoError(t, err, "SetWebsocketURL should not error")

	err = ws.SetWebsocketURL("", true, false)
	assert.NoError(t, err, "SetWebsocketURL should not error")

	err = ws.SetWebsocketURL("ws://demos.kaazing.com/echo", true, false)
	assert.NoError(t, err, "SetWebsocketURL should not error")

	err = ws.SetWebsocketURL("ws://demos.kaazing.com/echo", true, true)
	assert.NoError(t, err, "SetWebsocketURL should not error on reconnect")

	// -- initiate the reconnect which is usually handled by connection monitor
	err = ws.Connect(t.Context())
	assert.NoError(t, err, "ReConnect called manually should not error")

	err = ws.Connect(t.Context())
	assert.ErrorIs(t, err, errAlreadyConnected, "ReConnect should error when already connected")

	err = ws.Shutdown()
	assert.NoError(t, err, "Shutdown should not error")
	ws.Wg.Wait()

	ws.useMultiConnectionManagement = true

	ws.connectionManager = []*websocket{{setup: &ConnectionSetup{URL: "ws://demos.kaazing.com/echo"}, connections: []Connection{&connection{subscriptions: subscription.NewStore()}}}}
	err = ws.SetProxyAddress(t.Context(), "https://192.168.0.1:1337")
	require.NoError(t, err)
}

func TestSetWebsocketURLAllPaths(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	manager.exchangeName = testString
	manager.defaultURL = "wss://default.example.com/path"
	manager.defaultURLAuth = "wss://auth.example.com/path"
	manager.verbose = true
	manager.Conn = &diagnosticConnection{}
	manager.AuthConn = &diagnosticConnection{}

	require.NoError(t, manager.SetWebsocketURL(config.WebsocketURLNonDefaultMessage, false, false), "SetWebsocketURL must accept the default unauthenticated URL")
	require.Equal(t, manager.defaultURL, manager.Conn.GetURL(), "SetWebsocketURL must update the unauthenticated connection")
	require.NoError(t, manager.SetWebsocketURL(config.WebsocketURLNonDefaultMessage, true, false), "SetWebsocketURL must accept the default authenticated URL")
	require.Equal(t, manager.defaultURLAuth, manager.AuthConn.GetURL(), "SetWebsocketURL must update the authenticated connection")

	manager.useMultiConnectionManagement = true
	require.ErrorIs(t, manager.SetWebsocketURL("wss://example.com", false, false), errCannotChangeConnectionURL, "SetWebsocketURL must reject managed connections")
}

func TestSetProxyAddressAllPaths(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	manager.exchangeName = testString
	publicConn := &diagnosticConnection{subscriptions: subscription.NewStore()}
	authConn := &diagnosticConnection{subscriptions: subscription.NewStore()}
	managedConn := &diagnosticConnection{subscriptions: subscription.NewStore()}
	managed := &websocket{setup: &ConnectionSetup{}, subscriptions: subscription.NewStore(), connections: []Connection{managedConn}}
	manager.Conn = publicConn
	manager.AuthConn = authConn
	manager.connectionManager = []*websocket{managed}

	const firstProxy = "http://127.0.0.1:8080"
	require.NoError(t, manager.SetProxyAddress(t.Context(), firstProxy), "SetProxyAddress must update disconnected connections")
	require.Equal(t, firstProxy, publicConn.proxy, "SetProxyAddress must update the public connection")
	require.Equal(t, firstProxy, authConn.proxy, "SetProxyAddress must update the authenticated connection")
	require.Equal(t, firstProxy, managedConn.proxy, "SetProxyAddress must update managed connections")

	manager.setState(connectedState)
	err := manager.SetProxyAddress(t.Context(), "http://127.0.0.1:8081")
	require.ErrorIs(t, err, common.ErrTypeAssertFailure, "SetProxyAddress must return shutdown failures")
}

func TestConnectDirectAllPaths(t *testing.T) {
	const (
		setupURL     = "wss://setup-user-token:setup-password-token@setup.example.com/setup-path-token?signature=setup-query-token#setup-fragment-token"
		safeSetupURL = "wss://setup.example.com"
	)
	newManager := func() *Manager {
		manager := NewManager()
		manager.exchangeName = testString
		manager.setEnabled(true)
		manager.connectionMonitorRunning.Store(true)
		manager.trafficTimeout = time.Hour
		manager.verbose = true
		close(manager.ShutdownC)
		return manager
	}
	waitForMonitor := func(t *testing.T, manager *Manager) {
		t.Helper()
		manager.Wg.Wait()
	}
	configuredSetup := func(generate func() (subscription.List, error)) *ConnectionSetup {
		return &ConnectionSetup{
			URL:                   setupURL,
			Connector:             func(context.Context, Connection) error { return nil },
			GenerateSubscriptions: generate,
			Subscriber:            func(context.Context, Connection, subscription.List) error { return nil },
			Handler:               func(context.Context, Connection, []byte) error { return nil },
		}
	}
	assertSafeSetupURL := func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err, "connect must return the managed setup failure")
		assert.Contains(t, err.Error(), safeSetupURL, "connect errors should retain the safe setup origin")
		for _, secret := range []string{"setup-user-token", "setup-password-token", "setup-path-token", "setup-query-token", "setup-fragment-token"} {
			assert.NotContains(t, err.Error(), secret, "connect errors should omit setup URL secrets")
		}
	}

	t.Run("missing connector", func(t *testing.T) {
		manager := newManager()
		require.ErrorIs(t, manager.connect(t.Context()), errNoConnectFunc, "connect must reject a missing connector")
		waitForMonitor(t, manager)
	})

	t.Run("subscription generator failure", func(t *testing.T) {
		manager := newManager()
		manager.connector = noopConnect
		manager.GenerateSubs = func() (subscription.List, error) { return nil, errDastardlyReason }
		require.ErrorIs(t, manager.connect(t.Context()), errDastardlyReason, "connect must return subscription generator failures")
		waitForMonitor(t, manager)
	})

	t.Run("subscriber failure", func(t *testing.T) {
		manager := newManager()
		manager.connector = noopConnect
		manager.GenerateSubs = func() (subscription.List, error) { return subscription.List{{Channel: "A"}}, nil }
		manager.Subscriber = func(subscription.List) error { return errDastardlyReason }
		require.ErrorIs(t, manager.connect(t.Context()), errDastardlyReason, "connect must return subscriber failures")
		waitForMonitor(t, manager)
	})

	t.Run("managed setup failures redact URL", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			setup       func() *ConnectionSetup
			expectedErr error
		}{
			{
				name: "missing generator",
				setup: func() *ConnectionSetup {
					return configuredSetup(nil)
				},
				expectedErr: errWebsocketSubscriptionsGeneratorUnset,
			},
			{
				name: "missing connector",
				setup: func() *ConnectionSetup {
					setup := configuredSetup(func() (subscription.List, error) { return subscription.List{{Channel: "A"}}, nil })
					setup.Connector = nil
					return setup
				},
				expectedErr: errNoConnectFunc,
			},
			{
				name: "missing handler",
				setup: func() *ConnectionSetup {
					setup := configuredSetup(func() (subscription.List, error) { return subscription.List{{Channel: "A"}}, nil })
					setup.Handler = nil
					return setup
				},
				expectedErr: errWebsocketDataHandlerUnset,
			},
			{
				name: "missing subscriber",
				setup: func() *ConnectionSetup {
					setup := configuredSetup(func() (subscription.List, error) { return subscription.List{{Channel: "A"}}, nil })
					setup.Subscriber = nil
					return setup
				},
				expectedErr: errWebsocketSubscriberUnset,
			},
			{
				name: "subscription-free connector failure",
				setup: func() *ConnectionSetup {
					setup := configuredSetup(nil)
					setup.SubscriptionsNotRequired = true
					setup.Connector = func(context.Context, Connection) error { return errDastardlyReason }
					return setup
				},
				expectedErr: errDastardlyReason,
			},
			{
				name: "subscription connector failure",
				setup: func() *ConnectionSetup {
					setup := configuredSetup(func() (subscription.List, error) { return subscription.List{{Channel: "A"}}, nil })
					setup.Connector = func(context.Context, Connection) error { return errDastardlyReason }
					return setup
				},
				expectedErr: errDastardlyReason,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				manager := newManager()
				manager.useMultiConnectionManagement = true
				manager.connectionManager = []*websocket{{setup: tc.setup(), subscriptions: subscription.NewStore()}}

				err := manager.connect(t.Context())
				require.ErrorIs(t, err, tc.expectedErr, "connect must retain the managed setup failure")
				assertSafeSetupURL(t, err)
				waitForMonitor(t, manager)
			})
		}
	})

	t.Run("empty generated subscriptions", func(t *testing.T) {
		manager := newManager()
		manager.useMultiConnectionManagement = true
		manager.connectionManager = []*websocket{{setup: configuredSetup(func() (subscription.List, error) { return nil, nil }), subscriptions: subscription.NewStore()}}
		require.NoError(t, manager.connect(t.Context()), "connect must skip empty generated subscriptions")
		waitForMonitor(t, manager)
	})

	t.Run("initial tracking failure", func(t *testing.T) {
		manager := newManager()
		manager.useMultiConnectionManagement = true
		sub := &subscription.Subscription{Channel: "A"}
		setup := configuredSetup(func() (subscription.List, error) { return subscription.List{sub}, nil })
		setup.TrackOnExistingConnection = func(context.Context, Connection, subscription.List) (subscription.List, subscription.List, error) {
			return nil, nil, errDastardlyReason
		}
		ws := &websocket{setup: setup, subscriptions: subscription.NewStore()}
		manager.trackConnection(&diagnosticConnection{subscriptions: subscription.NewStore()}, ws)
		manager.connectionManager = []*websocket{ws}
		err := manager.connect(t.Context())
		require.ErrorIs(t, err, errDastardlyReason, "connect must return initial tracking failures")
		assertSafeSetupURL(t, err)
		waitForMonitor(t, manager)
	})

	t.Run("initial tracking absorbs subscriptions", func(t *testing.T) {
		manager := newManager()
		manager.useMultiConnectionManagement = true
		sub := &subscription.Subscription{Channel: "A"}
		setup := configuredSetup(func() (subscription.List, error) { return subscription.List{sub}, nil })
		setup.TrackOnExistingConnection = func(_ context.Context, _ Connection, subs subscription.List) (subscription.List, subscription.List, error) {
			return nil, subs, nil
		}
		ws := &websocket{setup: setup, subscriptions: subscription.NewStore()}
		manager.trackConnection(&diagnosticConnection{subscriptions: subscription.NewStore()}, ws)
		manager.connectionManager = []*websocket{ws}
		require.NoError(t, manager.connect(t.Context()), "connect must accept subscriptions tracked on an existing connection")
		waitForMonitor(t, manager)
	})

	t.Run("batch tracking failure", func(t *testing.T) {
		manager := newManager()
		manager.useMultiConnectionManagement = true
		manager.MaxSubscriptionsPerConnection = 1
		sub := &subscription.Subscription{Channel: "A"}
		setup := configuredSetup(func() (subscription.List, error) { return subscription.List{sub}, nil })
		calls := 0
		setup.TrackOnExistingConnection = func(_ context.Context, _ Connection, subs subscription.List) (subscription.List, subscription.List, error) {
			calls++
			if calls == 2 {
				return nil, nil, errDastardlyReason
			}
			return subs, nil, nil
		}
		store, err := subscription.NewStoreFromList(subscription.List{{Channel: "occupied"}})
		require.NoError(t, err, "NewStoreFromList must create the full connection store")
		ws := &websocket{setup: setup, subscriptions: subscription.NewStore()}
		manager.trackConnection(&diagnosticConnection{subscriptions: store}, ws)
		manager.connectionManager = []*websocket{ws}
		err = manager.connect(t.Context())
		require.ErrorIs(t, err, errDastardlyReason, "connect must return batch tracking failures")
		assertSafeSetupURL(t, err)
		waitForMonitor(t, manager)
	})

	t.Run("batch tracking absorbs subscriptions", func(t *testing.T) {
		manager := newManager()
		manager.useMultiConnectionManagement = true
		manager.MaxSubscriptionsPerConnection = 1
		sub := &subscription.Subscription{Channel: "A"}
		setup := configuredSetup(func() (subscription.List, error) { return subscription.List{sub}, nil })
		calls := 0
		setup.TrackOnExistingConnection = func(_ context.Context, _ Connection, subs subscription.List) (subscription.List, subscription.List, error) {
			calls++
			if calls == 2 {
				return nil, subs, nil
			}
			return subs, nil, nil
		}
		store, err := subscription.NewStoreFromList(subscription.List{{Channel: "occupied"}})
		require.NoError(t, err, "NewStoreFromList must create the full connection store")
		ws := &websocket{setup: setup, subscriptions: subscription.NewStore()}
		manager.trackConnection(&diagnosticConnection{subscriptions: store}, ws)
		manager.connectionManager = []*websocket{ws}
		require.NoError(t, manager.connect(t.Context()), "connect must accept subscriptions tracked during batching")
		waitForMonitor(t, manager)
	})

	t.Run("successful managed connections", func(t *testing.T) {
		echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler)
		}))
		t.Cleanup(echoServer.Close)
		endpoint := "ws" + echoServer.URL[len("http"):] + "/ws"
		manager := newManager()
		manager.useMultiConnectionManagement = true
		noSubscriptions := &websocket{
			setup: &ConnectionSetup{
				URL:                      endpoint,
				SubscriptionsNotRequired: true,
				Connector:                func(ctx context.Context, conn Connection) error { return conn.Dial(ctx, gws.DefaultDialer, nil, nil) },
				Handler:                  func(context.Context, Connection, []byte) error { return nil },
			},
			subscriptions: subscription.NewStore(),
		}
		sub := &subscription.Subscription{Channel: "A"}
		withSubscriptions := &websocket{
			setup: &ConnectionSetup{
				URL:                   endpoint,
				Connector:             func(ctx context.Context, conn Connection) error { return conn.Dial(ctx, gws.DefaultDialer, nil, nil) },
				GenerateSubscriptions: func() (subscription.List, error) { return subscription.List{sub}, nil },
				Subscriber: func(_ context.Context, conn Connection, subs subscription.List) error {
					return manager.AddSuccessfulSubscriptions(conn, subs...)
				},
				Handler: func(context.Context, Connection, []byte) error { return nil },
			},
			subscriptions: subscription.NewStore(),
		}
		manager.connectionManager = []*websocket{noSubscriptions, withSubscriptions}
		require.NoError(t, manager.connect(t.Context()), "connect must establish managed connections")
		for _, ws := range manager.connectionManager {
			for _, conn := range ws.connections {
				require.NoError(t, conn.Shutdown(), "Shutdown must close managed test connections")
			}
		}
		waitForMonitor(t, manager)
	})

	t.Run("rollback shutdown failure", func(t *testing.T) {
		manager := newManager()
		manager.useMultiConnectionManagement = true
		ws := &websocket{
			setup:         &ConnectionSetup{URL: setupURL},
			subscriptions: subscription.NewStore(),
			connections:   []Connection{&diagnosticConnection{subscriptions: subscription.NewStore(), shutdownErr: errDastardlyReason}},
		}
		manager.connectionManager = []*websocket{ws}
		err := manager.connect(t.Context())
		require.ErrorIs(t, err, errWebsocketSubscriptionsGeneratorUnset, "connect must return fatal setup failures")
		assertSafeSetupURL(t, err)
		waitForMonitor(t, manager)
	})
}

// TestSetCanUseAuthenticatedEndpoints logic test
func TestSetCanUseAuthenticatedEndpoints(t *testing.T) {
	t.Parallel()
	ws := NewManager()
	assert.False(t, ws.CanUseAuthenticatedEndpoints(), "CanUseAuthenticatedEndpoints should return false")
	ws.SetCanUseAuthenticatedEndpoints(true)
	assert.True(t, ws.CanUseAuthenticatedEndpoints(), "CanUseAuthenticatedEndpoints should return true")
}

// TestDial logic test
func TestDial(t *testing.T) {
	t.Parallel()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler) }))
	defer mock.Close()

	// Mock server rejects parallel connections
	for _, tc := range []*testStruct{
		{
			WC: connection{
				ExchangeName:     "test1",
				Verbose:          true,
				URL:              "ws" + mock.URL[len("http"):] + "/ws",
				RateLimit:        request.NewWeightedRateLimitByDuration(10 * time.Millisecond),
				ResponseMaxLimit: 7000000000,
			},
		},
		{
			Error: errors.New("websocket connection to [redacted URL] failed"),
			WC: connection{
				ExchangeName:     "test2",
				Verbose:          true,
				URL:              "",
				ResponseMaxLimit: 7000000000,
			},
		},
		{
			WC: connection{
				ExchangeName:     "test3",
				Verbose:          true,
				URL:              "ws" + mock.URL[len("http"):] + "/ws",
				ProxyURL:         proxyURL,
				ResponseMaxLimit: 7000000000,
			},
		},
	} {
		if tc.WC.ProxyURL != "" && !useProxyTests {
			t.Log("Proxy testing not enabled, skipping")
			continue
		}
		err := tc.WC.Dial(t.Context(), &gws.Dialer{}, http.Header{}, nil)
		if err != nil {
			if tc.Error != nil && strings.Contains(err.Error(), tc.Error.Error()) {
				continue
			}
			t.Fatal(err)
		}
	}
}

// TestSendMessage logic test
func TestSendMessage(t *testing.T) {
	t.Parallel()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler) }))
	defer mock.Close()

	// Mock server rejects parallel connections
	for _, tc := range []*testStruct{
		{
			WC: connection{
				ExchangeName:     "test1",
				Verbose:          true,
				URL:              "ws" + mock.URL[len("http"):] + "/ws",
				RateLimit:        request.NewWeightedRateLimitByDuration(10 * time.Millisecond),
				ResponseMaxLimit: 7000000000,
			},
		},
		{
			Error: errors.New("websocket connection to [redacted URL] failed"),
			WC: connection{
				ExchangeName:     "test2",
				Verbose:          true,
				URL:              "",
				ResponseMaxLimit: 7000000000,
			},
		},
		{
			WC: connection{
				ExchangeName:     "test3",
				Verbose:          true,
				URL:              "ws" + mock.URL[len("http"):] + "/ws",
				ProxyURL:         proxyURL,
				ResponseMaxLimit: 7000000000,
			},
		},
	} {
		if tc.WC.ProxyURL != "" && !useProxyTests {
			t.Log("Proxy testing not enabled, skipping")
			continue
		}
		err := tc.WC.Dial(t.Context(), &gws.Dialer{}, http.Header{}, nil)
		if err != nil {
			if tc.Error != nil && strings.Contains(err.Error(), tc.Error.Error()) {
				continue
			}
			t.Fatal(err)
		}
		err = tc.WC.SendJSONMessage(t.Context(), request.Unset, Ping)
		require.NoError(t, err)
		err = tc.WC.SendRawMessage(t.Context(), request.Unset, gws.TextMessage, []byte(Ping))
		require.NoError(t, err)
	}
}

func TestWebsocketDiagnostics(t *testing.T) {
	const diagnosticName = "diagnostic-test"

	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable diagnostic capture")
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
		assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable diagnostic capture")
	})

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler)
	}))
	t.Cleanup(echoServer.Close)
	endpoint := "ws" + echoServer.URL[len("http"):] + "/path-token?signature=query-token"
	wc := &connection{ExchangeName: diagnosticName, Verbose: true, URL: endpoint, ResponseMaxLimit: time.Second, Match: NewMatch()}
	require.NoError(t, wc.Dial(t.Context(), &gws.Dialer{}, http.Header{"X-Handshake-Header-Token": {"handshake-header-value-token"}}, nil), "Dial must connect to the diagnostic server")
	t.Cleanup(func() { assert.NoError(t, wc.Shutdown(), "Shutdown should close the diagnostic connection") })

	const jsonPayload = "{\"credential\":\"json-payload-token\"}\n"
	require.NoError(t, wc.SendJSONMessage(t.Context(), request.Unset, map[string]string{"credential": "json-payload-token"}), "SendJSONMessage must send the diagnostic frame")
	response := wc.ReadMessage()
	require.True(t, bytes.Equal([]byte(jsonPayload), response.Raw), "SendJSONMessage must emit the exact JSON payload")
	require.NoError(t, wc.SendRawMessage(t.Context(), request.Unset, gws.TextMessage, []byte("raw-payload-token")), "SendRawMessage must send the diagnostic frame")
	response = wc.ReadMessage()
	require.Equal(t, "raw-payload-token", string(response.Raw), "ReadMessage must preserve the raw frame")

	matched := make(chan []byte, 1)
	matched <- []byte("matched-response-token")
	responses, err := wc.waitForResponses(request.WithVerbose(t.Context()), "signature-value-token", matched, 1, nil)
	require.NoError(t, err, "waitForResponses must receive the diagnostic response")
	require.Equal(t, "matched-response-token", string(responses[0]), "waitForResponses must preserve matched data")

	handshakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Response-Header-Token", "response-header-value-token")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "handshake-body-token")
	}))
	t.Cleanup(handshakeServer.Close)
	failedEndpoint := "ws" + handshakeServer.URL[len("http"):] + "/handshake-path-token"
	failedConn := &connection{ExchangeName: diagnosticName, URL: failedEndpoint}
	err = failedConn.Dial(t.Context(), &gws.Dialer{}, http.Header{"X-Request-Header-Token": {"request-header-value-token"}}, url.Values{"signature": {"handshake-query-token"}})
	require.Error(t, err, "Dial must return the failed handshake")
	assert.Contains(t, err.Error(), "status 401", "Dial errors should retain the handshake status code")
	for _, secret := range []string{"handshake-path-token", "handshake-query-token", "Request-Header-Token", "request-header-value-token", "Response-Header-Token", "response-header-value-token", "handshake-body-token"} {
		assert.NotContains(t, err.Error(), secret, "Dial errors should omit handshake wire data")
	}

	pingConn := &connection{ExchangeName: diagnosticName, URL: endpoint, Wg: &sync.WaitGroup{}, shutdown: make(chan struct{})}
	pingConn.SetupPingHandler(request.Unset, PingHandler{MessageType: gws.TextMessage, Message: []byte("ping-payload-token"), Delay: time.Millisecond})
	pingConn.Wg.Wait()

	manager := NewManager()
	manager.exchangeName = diagnosticName
	manager.verbose = true
	require.NoError(t, manager.SetWebsocketURL("wss://url-user-token:url-password-token@example.com/manager-path-token?signature=manager-query-token#manager-fragment-token", false, false), "SetWebsocketURL must preserve the configured URL")
	require.NoError(t, manager.SetProxyAddress(t.Context(), "http://proxy-user-token:proxy-password-token@example.com:8080/proxy-path-token?key=proxy-query-token"), "SetProxyAddress must preserve the configured proxy")
	err = manager.SetProxyAddress(t.Context(), "http://proxy-user-token:proxy-password-token@example.com:8080/proxy-path-token?key=proxy-query-token")
	require.ErrorIs(t, err, errSameProxyAddress, "SetProxyAddress must reject a duplicate proxy")
	assert.NotContains(t, err.Error(), "proxy-user-token", "SetProxyAddress errors should omit proxy credentials")
	require.NoError(t, manager.SetWebsocketURL("wss://auth-user-token:auth-password-token@auth.example.com/auth-path-token?signature=auth-query-token#auth-fragment-token", true, false), "SetWebsocketURL must preserve the authenticated URL")
	manager.setState(connectedState)
	require.NoError(t, manager.SetWebsocketURL("wss://reconnect-user-token:reconnect-password-token@reconnect.example.com/reconnect-path-token?signature=reconnect-query-token#reconnect-fragment-token", false, true), "SetWebsocketURL must reconnect after updating the public URL")

	reconnectErrorCanary := "reconnect-error-secret-token"
	reconnectManager := NewManager()
	reconnectManager.exchangeName = diagnosticName
	reconnectManager.trafficTimeout = time.Hour
	reconnectManager.connectionMonitorRunning.Store(true)
	reconnectManager.connector = func() error { return errors.New(reconnectErrorCanary) }
	reconnectManager.setEnabled(true)
	close(reconnectManager.ShutdownC)
	reconnectManager.ReadMessageErrors <- errDastardlyReason
	reconnectTimer := time.NewTimer(time.Hour)
	require.False(t, reconnectManager.observeConnection(t.Context(), reconnectTimer), "observeConnection must continue after a reconnect failure")
	require.True(t, reconnectTimer.Stop(), "reconnectTimer must remain active after reconnect-error handling")
	reconnectManager.Wg.Wait()

	setupURL := "wss://setup-user-token:setup-password-token@setup.example.com/setup-path-token?signature=setup-query-token#setup-fragment-token"
	setupManager := NewManager()
	setupManager.exchangeName = diagnosticName
	setupManager.verbose = true
	setupManager.useMultiConnectionManagement = true
	setupManager.connectionMonitorRunning.Store(true)
	setupManager.trafficTimeout = time.Hour
	setupManager.setEnabled(true)
	close(setupManager.ShutdownC)
	setupManager.connectionManager = []*websocket{{
		setup: &ConnectionSetup{
			URL:                   setupURL,
			Connector:             func(context.Context, Connection) error { return nil },
			GenerateSubscriptions: func() (subscription.List, error) { return nil, nil },
			Subscriber:            func(context.Context, Connection, subscription.List) error { return nil },
			Handler:               func(context.Context, Connection, []byte) error { return nil },
		},
		subscriptions: subscription.NewStore(),
	}}
	require.NoError(t, setupManager.connect(t.Context()), "connect must skip a managed setup without generated subscriptions")
	setupManager.Wg.Wait()

	rollbackErrorCanary := "rollback-shutdown-error-secret-token"
	rollbackManager := NewManager()
	rollbackManager.exchangeName = diagnosticName
	rollbackManager.useMultiConnectionManagement = true
	rollbackManager.connectionMonitorRunning.Store(true)
	rollbackManager.trafficTimeout = time.Hour
	rollbackManager.setEnabled(true)
	close(rollbackManager.ShutdownC)
	rollbackManager.connectionManager = []*websocket{{
		setup:         &ConnectionSetup{URL: setupURL},
		subscriptions: subscription.NewStore(),
		connections: []Connection{
			&diagnosticConnection{
				url:           "wss://rollback-user-token:rollback-password-token@rollback.example.com/rollback-path-token?signature=rollback-query-token#rollback-fragment-token",
				subscriptions: subscription.NewStore(),
				shutdownErr:   errors.New(rollbackErrorCanary),
			},
		},
	}}
	err = rollbackManager.connect(t.Context())
	require.ErrorIs(t, err, errWebsocketSubscriptionsGeneratorUnset, "connect must return the managed setup failure")
	assert.Contains(t, err.Error(), "wss://setup.example.com", "connect errors should retain the safe setup origin")
	for _, secret := range []string{"setup-user-token", "setup-password-token", "setup-path-token", "setup-query-token", "setup-fragment-token"} {
		assert.NotContains(t, err.Error(), secret, "connect errors should omit setup URL secrets")
	}
	rollbackManager.Wg.Wait()

	handlerConn := &diagnosticConnection{
		url:       "wss://reader-user-token:reader-password-token@example.com/reader-path-token?signature=reader-query-token",
		responses: []Response{{Type: gws.TextMessage, Raw: []byte("reader-frame-token")}, {}},
	}
	manager.Wg.Add(1)
	handlerSourceErr := &connectionTestNetError{timeout: true, temporary: true}
	manager.Reader(t.Context(), handlerConn, func(context.Context, Connection, []byte) error {
		return fmt.Errorf("handler-payload-token: %w", handlerSourceErr)
	})
	relayed := <-manager.DataHandler.C
	relayedErr, ok := relayed.Data.(error)
	require.True(t, ok, "Reader must relay handler failures as errors")
	require.Error(t, relayedErr, "Reader must preserve the handler failure")
	assert.Contains(t, relayedErr.Error(), "endpoint=[wss://example.com]", "Reader errors should include the safe endpoint")
	assert.ErrorIs(t, relayedErr, handlerSourceErr, "Reader should preserve the handler source error")
	var handlerTarget *connectionTestNetError
	require.ErrorAs(t, relayedErr, &handlerTarget, "Reader must preserve handler source error inspection")
	assert.Same(t, handlerSourceErr, handlerTarget, "Reader should preserve the handler source error value")
	var handlerNetErr net.Error
	require.ErrorAs(t, relayedErr, &handlerNetErr, "Reader must preserve net.Error compatibility")
	assert.True(t, handlerNetErr.Timeout(), "Reader should preserve handler timeout classification")
	var handlerTemporaryErr interface{ Temporary() bool }
	require.ErrorAs(t, relayedErr, &handlerTemporaryErr, "Reader must preserve temporary error compatibility")
	assert.True(t, handlerTemporaryErr.Temporary(), "Reader should preserve handler temporary classification")
	for _, secret := range []string{"reader-user-token", "reader-password-token", "reader-path-token", "reader-query-token", "reader-frame-token", "handler-payload-token"} {
		assert.NotContains(t, relayedErr.Error(), secret, "Reader errors should omit endpoint and handler data")
	}

	fullRelay := stream.NewRelay(1)
	require.NoError(t, fullRelay.Send(t.Context(), "occupied"), "Send must fill the diagnostic relay")
	fullManager := NewManager()
	fullManager.exchangeName = diagnosticName
	fullManager.DataHandler = fullRelay
	fullManager.Wg.Add(1)
	fullManager.Reader(t.Context(), &diagnosticConnection{responses: []Response{{Type: gws.TextMessage, Raw: []byte("reader-frame-token")}, {}}}, func(context.Context, Connection, []byte) error {
		return errors.New("handler-payload-token")
	})

	readErrorCanary := "read-error-secret-token"
	readError := fmt.Errorf("%s: %w", readErrorCanary, errConnectionFault)
	readManager := NewManager()
	readManager.exchangeName = diagnosticName
	readManager.ReadMessageErrors <- readError
	readTimer := time.NewTimer(time.Hour)
	require.False(t, readManager.observeConnection(t.Context(), readTimer), "observeConnection must continue after a sanitized read error")
	require.True(t, readTimer.Stop(), "readTimer must remain active after read-error handling")

	fullReadRelay := stream.NewRelay(1)
	relayBufferCanary := "relay-buffer-secret-token"
	require.NoError(t, fullReadRelay.Send(t.Context(), relayBufferCanary), "Send must fill the read-error diagnostic relay")
	fullReadManager := NewManager()
	fullReadManager.exchangeName = diagnosticName
	fullReadManager.DataHandler = fullReadRelay
	fullReadManager.ReadMessageErrors <- readError
	fullReadTimer := time.NewTimer(time.Hour)
	require.False(t, fullReadManager.observeConnection(t.Context(), fullReadTimer), "observeConnection must continue after a read-error relay failure")
	require.True(t, fullReadTimer.Stop(), "fullReadTimer must remain active after read-error relay handling")

	shutdownCanary := "shutdown-close-secret-token"
	shutdownManager := NewManager()
	shutdownManager.exchangeName = diagnosticName
	shutdownManager.connectionManager = []*websocket{{
		subscriptions: subscription.NewStore(),
		connections: []Connection{
			&diagnosticConnection{
				url:           "wss://managed-one-user-token:managed-one-password-token@managed-one.example.com/managed-one-path-token?signature=managed-one-query-token",
				subscriptions: subscription.NewStore(),
				shutdownErr:   errors.New(shutdownCanary),
			},
			&diagnosticConnection{
				url:           "wss://managed-two-user-token:managed-two-password-token@managed-two.example.com/managed-two-path-token?signature=managed-two-query-token",
				subscriptions: subscription.NewStore(),
				shutdownErr:   errors.New(shutdownCanary),
			},
		},
	}}
	shutdownManager.setState(connectedState)
	require.NoError(t, shutdownManager.shutdown(), "shutdown must treat diagnostic connection close failures as non-fatal")

	publicShutdownCanary := "public-shutdown-close-secret-token"
	authenticatedShutdownCanary := "authenticated-shutdown-close-secret-token"
	roleShutdownManager := NewManager()
	roleShutdownManager.exchangeName = diagnosticName
	roleShutdownManager.Conn = &diagnosticConnection{
		url:         "wss://public-user-token:public-password-token@public-shutdown.example.com/public-path-token?signature=public-query-token",
		shutdownErr: errors.New(publicShutdownCanary),
	}
	roleShutdownManager.AuthConn = &diagnosticConnection{
		url:         "wss://authenticated-user-token:authenticated-password-token@authenticated-shutdown.example.com/authenticated-path-token?signature=authenticated-query-token",
		shutdownErr: errors.New(authenticatedShutdownCanary),
	}
	roleShutdownManager.setState(connectedState)
	require.Error(t, roleShutdownManager.shutdown(), "shutdown must report unsupported custom connection reset after logging close failures")

	trafficManager := NewManager()
	trafficManager.exchangeName = diagnosticName
	trafficManager.trafficTimeout = 0
	trafficManager.setState(connectedState)
	trafficManager.m.Lock()
	monitorExited := trafficManager.monitorTraffic(t.Context())()
	trafficManager.setState(connectingState)
	trafficManager.m.Unlock()
	require.True(t, monitorExited, "monitorTraffic must exit after a traffic timeout")
	require.Eventually(t, func() bool {
		logMu.Lock()
		defer logMu.Unlock()
		return slices.ContainsFunc(entries, func(entry string) bool {
			return strings.Contains(entry, "traffic monitor shutdown failed")
		})
	}, time.Second, time.Millisecond, "monitorTraffic must report shutdown failures")

	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	logMu.Unlock()
	for _, metadata := range []string{
		"websocket connected to ws://",
		"outbound JSON frame type=map[string]string",
		"outbound frame opcode=1 bytes=17",
		"inbound frame opcode=1",
		"matched response [1/1] bytes=22",
		"ping handler failed opcode=1 bytes=18",
		"setting unauthenticated websocket URL: wss://example.com",
		"setting authenticated websocket URL: wss://auth.example.com",
		"flushing websocket connection to wss://reconnect.example.com",
		"setting websocket proxy: http://example.com:8080",
		"websocket reconnect failed error=operation failed (",
		"no subscriptions generated for [conn:1] [URL:wss://setup.example.com], skipping",
		"managed connection rollback shutdown failed setup=1 connection=1 endpoint=wss://rollback.example.com error=operation failed (",
		"websocket handler error relay failed handler-error=connection endpoint=[[redacted URL]] error: operation failed (",
		"websocket has been disconnected read-error=operation failed (",
		"connection monitor error relay failed read-error=operation failed (",
		"managed connection shutdown failed setup=1 connection=1 endpoint=wss://managed-one.example.com error=operation failed (",
		"managed connection shutdown failed setup=1 connection=2 endpoint=wss://managed-two.example.com error=operation failed (",
		"connection shutdown failed role=public endpoint=wss://public-shutdown.example.com error=operation failed (",
		"connection shutdown failed role=authenticated endpoint=wss://authenticated-shutdown.example.com error=operation failed (",
		"websocket: shutdown encountered connection close failures count=2",
	} {
		assert.Contains(t, diagnostics, metadata, "websocket diagnostics should include safe metadata")
	}
	assert.Equal(t, 2, strings.Count(diagnostics, "websocket: shutdown encountered connection close failures count=2"), "shutdown diagnostics should summarise managed and role-specific connection failures")
	for _, secret := range []string{
		"path-token", "query-token", "fragment-token", "Handshake-Header-Token", "handshake-header-value-token",
		"json-payload-token", "raw-payload-token", "matched-response-token", "signature-value-token", "ping-payload-token",
		"url-user-token", "url-password-token", "manager-path-token", "manager-query-token", "proxy-user-token",
		"proxy-password-token", "proxy-path-token", "proxy-query-token", "handler-payload-token",
		"auth-user-token", "auth-password-token", "auth-path-token", "auth-query-token", "auth-fragment-token",
		"reconnect-user-token", "reconnect-password-token", "reconnect-path-token", "reconnect-query-token", "reconnect-fragment-token",
		"setup-user-token", "setup-password-token", "setup-path-token", "setup-query-token", "setup-fragment-token",
		"rollback-user-token", "rollback-password-token", "rollback-path-token", "rollback-query-token", "rollback-fragment-token",
		"managed-one-user-token", "managed-one-password-token", "managed-one-path-token", "managed-one-query-token",
		"managed-two-user-token", "managed-two-password-token", "managed-two-path-token", "managed-two-query-token",
		"public-user-token", "public-password-token", "public-path-token", "public-query-token",
		"authenticated-user-token", "authenticated-password-token", "authenticated-path-token", "authenticated-query-token",
		readErrorCanary, relayBufferCanary, shutdownCanary, publicShutdownCanary, authenticatedShutdownCanary, reconnectErrorCanary, rollbackErrorCanary,
	} {
		assert.NotContains(t, diagnostics, secret, "websocket diagnostics should omit wire and credential data")
	}
}

func TestSendMessageReturnResponse(t *testing.T) {
	t.Parallel()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler) }))
	defer mock.Close()

	wc := &connection{
		Verbose:          true,
		URL:              "ws" + mock.URL[len("http"):] + "/ws",
		ResponseMaxLimit: time.Second * 5,
		Match:            NewMatch(),
	}
	if wc.ProxyURL != "" && !useProxyTests {
		t.Skip("Proxy testing not enabled, skipping")
	}

	err := wc.Dial(t.Context(), &gws.Dialer{}, http.Header{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	go readMessages(t, wc)

	req := testRequest{
		Event: "subscribe",
		Pairs: []string{currency.NewPairWithDelimiter("XBT", "USD", "/").String()},
		Subscription: testRequestData{
			Name: "ticker",
		},
		RequestID: 12345,
	}

	_, err = wc.SendMessageReturnResponse(t.Context(), request.Unset, req.RequestID, req)
	if err != nil {
		t.Error(err)
	}

	cancelledCtx, fn := context.WithDeadline(t.Context(), time.Now())
	fn()
	_, err = wc.SendMessageReturnResponse(cancelledCtx, request.Unset, "123", req)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// with timeout
	wc.ResponseMaxLimit = 1
	_, err = wc.SendMessageReturnResponse(t.Context(), request.Unset, "123", req)
	assert.ErrorIs(t, err, ErrSignatureTimeout, "SendMessageReturnResponse should error when request ID not found")

	_, err = wc.SendMessageReturnResponsesWithInspector(t.Context(), request.Unset, "123", req, 1, inspection{})
	assert.ErrorIs(t, err, ErrSignatureTimeout, "SendMessageReturnResponse should error when request ID not found")
}

func TestWaitForResponses(t *testing.T) {
	t.Parallel()
	dummy := &connection{
		ResponseMaxLimit: time.Nanosecond,
		Match:            NewMatch(),
	}
	_, err := dummy.waitForResponses(t.Context(), "signature-value-token", nil, 1, inspection{})
	require.ErrorIs(t, err, ErrSignatureTimeout)
	assert.NotContains(t, err.Error(), "signature-value-token", "waitForResponses should omit the signature value")

	dummy.ResponseMaxLimit = time.Second
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = dummy.waitForResponses(ctx, "silly", nil, 1, inspection{})
	require.ErrorIs(t, err, context.Canceled)

	// test break early and hit verbose path
	ch := make(chan []byte, 1)
	ch <- []byte("hello")
	ctx = request.WithVerbose(t.Context())

	got, err := dummy.waitForResponses(ctx, "silly", ch, 2, inspection{breakEarly: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "hello", string(got[0]))
}

func TestSendMessageReturnResponseMarshalError(t *testing.T) {
	t.Parallel()

	wc := &connection{}
	_, err := wc.SendMessageReturnResponse(t.Context(), request.Unset, "signature-value-token", make(chan int))
	require.Error(t, err, "SendMessageReturnResponse must return marshal errors")
	assert.NotContains(t, err.Error(), "signature-value-token", "SendMessageReturnResponse should omit the signature value")
	assert.NotContains(t, err.Error(), "unsupported type", "SendMessageReturnResponse should omit marshal error text")
}

type inspection struct {
	breakEarly bool
}

func (i inspection) IsFinal([]byte) bool { return i.breakEarly }

type reporter struct {
	name string
	msg  []byte
	t    time.Duration
}

func (r *reporter) Latency(name string, payload []byte, t time.Duration) {
	r.name = name
	r.msg = payload
	r.t = t
}

// readMessages helper func
func readMessages(t *testing.T, wc *connection) {
	t.Helper()
	timer := time.NewTimer(20 * time.Second)
	for {
		select {
		case <-timer.C:
			return
		default:
			resp := wc.ReadMessage()
			if resp.Raw == nil {
				t.Error("connection has closed")
				return
			}
			var incoming testResponse
			err := json.Unmarshal(resp.Raw, &incoming)
			if err != nil {
				t.Error(err)
				return
			}
			if incoming.RequestID > 0 {
				wc.Match.IncomingWithData(incoming.RequestID, resp.Raw)
				return
			}
		}
	}
}

// TestSetupPingHandler logic test
func TestSetupPingHandler(t *testing.T) {
	t.Parallel()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler) }))
	defer mock.Close()

	wc := &connection{
		URL:              "ws" + mock.URL[len("http"):] + "/ws",
		ResponseMaxLimit: time.Second * 5,
		Match:            NewMatch(),
		Wg:               &sync.WaitGroup{},
	}

	if wc.ProxyURL != "" && !useProxyTests {
		t.Skip("Proxy testing not enabled, skipping")
	}
	wc.shutdown = make(chan struct{})
	err := wc.Dial(t.Context(), &gws.Dialer{}, http.Header{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	wc.SetupPingHandler(request.Unset, PingHandler{
		UseGorillaHandler: true,
		MessageType:       gws.PingMessage,
		Delay:             100,
	})

	err = wc.Connection.Close()
	if err != nil {
		t.Error(err)
	}

	err = wc.Dial(t.Context(), &gws.Dialer{}, http.Header{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wc.SetupPingHandler(request.Unset, PingHandler{
		MessageType: gws.TextMessage,
		Message:     []byte(Ping),
		Delay:       200,
	})
	time.Sleep(time.Millisecond * 201)
	close(wc.shutdown)
	wc.Wg.Wait()
}

// TestParseBinaryResponse logic test
func TestParseBinaryResponse(t *testing.T) {
	t.Parallel()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler) }))
	defer mock.Close()

	wc := &connection{
		URL:              "ws" + mock.URL[len("http"):] + "/ws",
		ResponseMaxLimit: time.Second * 5,
		Match:            NewMatch(),
	}

	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	_, err := g.Write([]byte("hello"))
	require.NoError(t, err, "gzip.Write must not error")
	assert.NoError(t, g.Close(), "Close should not error")

	resp, err := wc.parseBinaryResponse(b.Bytes())
	assert.NoError(t, err, "parseBinaryResponse should not error parsing gzip")
	assert.EqualValues(t, "hello", resp, "parseBinaryResponse should decode gzip")

	b.Reset()
	f, err := flate.NewWriter(&b, 1)
	require.NoError(t, err, "flate.NewWriter must not error")
	_, err = f.Write([]byte("goodbye"))
	require.NoError(t, err, "flate.Write must not error")
	assert.NoError(t, f.Close(), "Close should not error")

	resp, err = wc.parseBinaryResponse(b.Bytes())
	assert.NoError(t, err, "parseBinaryResponse should not error parsing inflate")
	assert.EqualValues(t, "goodbye", resp, "parseBinaryResponse should deflate")

	_, err = wc.parseBinaryResponse([]byte{})
	assert.ErrorContains(t, err, "unexpected EOF", "parseBinaryResponse should error on empty input")
}

// TestCanUseAuthenticatedWebsocketForWrapper logic test
func TestCanUseAuthenticatedWebsocketForWrapper(t *testing.T) {
	t.Parallel()
	ws := &Manager{}
	assert.False(t, ws.CanUseAuthenticatedWebsocketForWrapper(), "CanUseAuthenticatedWebsocketForWrapper should return false")

	ws.setState(connectedState)
	require.True(t, ws.IsConnected(), "IsConnected must return true")
	assert.False(t, ws.CanUseAuthenticatedWebsocketForWrapper(), "CanUseAuthenticatedWebsocketForWrapper should return false")

	ws.SetCanUseAuthenticatedEndpoints(true)
	assert.True(t, ws.CanUseAuthenticatedWebsocketForWrapper(), "CanUseAuthenticatedWebsocketForWrapper should return true")
}

func TestCheckWebsocketURL(t *testing.T) {
	err := checkWebsocketURL("")
	assert.ErrorIs(t, err, errInvalidWebsocketURL, "checkWebsocketURL should error correctly on empty string")

	err = checkWebsocketURL("wowowow:wowowowo")
	assert.ErrorIs(t, err, errInvalidWebsocketURL, "checkWebsocketURL should error correctly on bad format")

	err = checkWebsocketURL("://")
	assert.ErrorIs(t, err, errInvalidWebsocketURL, "checkWebsocketURL should error correctly on bad proto")
	assert.NotContains(t, err.Error(), "://", "checkWebsocketURL should omit the invalid URL")
	assert.Contains(t, err.Error(), "[redacted URL]", "checkWebsocketURL should identify a redacted malformed URL")

	err = checkWebsocketURL("http://www.google.com")
	assert.ErrorIs(t, err, errInvalidWebsocketURL, "checkWebsocketURL should error correctly on wrong proto")
	assert.Contains(t, err.Error(), "http://www.google.com", "checkWebsocketURL should retain the rejected endpoint origin")

	err = checkWebsocketURL("wss://websocketconnection.place")
	assert.NoError(t, err, "checkWebsocketURL should not error")

	err = checkWebsocketURL("ws://websocketconnection.place")
	assert.NoError(t, err, "checkWebsocketURL should not error")
}

// GenSubs defines a theoretical exchange with pair management
type GenSubs struct {
	EnabledPairs currency.Pairs
	subscribos   subscription.List
	unsubscribos subscription.List
}

// generateSubs default subs created from the enabled pairs list
func (g *GenSubs) generateSubs() (subscription.List, error) {
	superduperchannelsubs := make(subscription.List, len(g.EnabledPairs))
	for i := range g.EnabledPairs {
		superduperchannelsubs[i] = &subscription.Subscription{
			Channel: "TEST:" + strconv.FormatInt(int64(i), 10),
			Pairs:   currency.Pairs{g.EnabledPairs[i]},
		}
	}
	return superduperchannelsubs, nil
}

func (g *GenSubs) SUBME(subs subscription.List) error {
	if len(subs) == 0 {
		return errors.New("WOW")
	}
	g.subscribos = subs
	return nil
}

func (g *GenSubs) UNSUBME(unsubs subscription.List) error {
	if len(unsubs) == 0 {
		return errors.New("WOW")
	}
	g.unsubscribos = unsubs
	return nil
}

func TestDisable(t *testing.T) {
	t.Parallel()
	w := NewManager()
	w.setEnabled(true)
	w.setState(connectedState)
	require.NoError(t, w.Disable(), "Disable must not error")
	assert.ErrorIs(t, w.Disable(), ErrAlreadyDisabled, "Disable should error correctly")
}

func TestEnable(t *testing.T) {
	t.Parallel()
	w := NewManager()
	w.connector = noopConnect
	w.Subscriber = func(subscription.List) error { return nil }
	w.Unsubscriber = func(subscription.List) error { return nil }
	w.GenerateSubs = func() (subscription.List, error) { return nil, nil }
	require.NoError(t, w.Enable(t.Context()), "Enable must not error")
	assert.ErrorIs(t, w.Enable(t.Context()), ErrWebsocketAlreadyEnabled, "Enable should error correctly")
}

func TestSetupNewConnection(t *testing.T) {
	t.Parallel()
	var nonsenseWebsock *Manager
	err := nonsenseWebsock.SetupNewConnection(&ConnectionSetup{URL: "urlstring"})
	assert.ErrorContains(t, err, "nil pointer: *websocket.Manager")

	nonsenseWebsock = &Manager{}
	err = nonsenseWebsock.SetupNewConnection(&ConnectionSetup{URL: "urlstring"})
	assert.ErrorIs(t, err, errExchangeConfigNameEmpty, "SetupNewConnection should error correctly")

	nonsenseWebsock = &Manager{exchangeName: testString}
	err = nonsenseWebsock.SetupNewConnection(&ConnectionSetup{URL: "urlstring"})
	assert.ErrorIs(t, err, errTrafficAlertNil, "SetupNewConnection should error correctly")

	nonsenseWebsock.TrafficAlert = make(chan struct{}, 1)
	err = nonsenseWebsock.SetupNewConnection(&ConnectionSetup{URL: "urlstring"})
	assert.ErrorIs(t, err, errReadMessageErrorsNil, "SetupNewConnection should error correctly")

	web := NewManager()

	err = web.Setup(newDefaultSetup())
	assert.NoError(t, err, "Setup should not error")

	err = web.SetupNewConnection(&ConnectionSetup{URL: "urlstring"})
	assert.NoError(t, err, "SetupNewConnection should not error")

	err = web.SetupNewConnection(&ConnectionSetup{URL: "urlstring", Authenticated: true})
	assert.NoError(t, err, "SetupNewConnection should not error")

	// Test connection candidates for multi connection tracking.
	multi := NewManager()
	set := newDefaultSetup()
	set.UseMultiConnectionManagement = true
	require.NoError(t, multi.Setup(set))

	err = multi.SetupNewConnection(nil)
	assert.ErrorContains(t, err, "nil pointer: *websocket.ConnectionSetup")

	connSetup := &ConnectionSetup{ResponseCheckTimeout: time.Millisecond}
	err = multi.SetupNewConnection(connSetup)
	require.ErrorIs(t, err, errDefaultURLIsEmpty)

	connSetup.URL = "urlstring"
	err = multi.SetupNewConnection(connSetup)
	require.ErrorIs(t, err, errWebsocketConnectorUnset)

	connSetup.Connector = func(context.Context, Connection) error { return nil }
	err = multi.SetupNewConnection(connSetup)
	require.ErrorIs(t, err, errWebsocketSubscriptionsGeneratorUnset)

	connSetup.GenerateSubscriptions = func() (subscription.List, error) { return nil, nil }
	err = multi.SetupNewConnection(connSetup)
	require.ErrorIs(t, err, errWebsocketSubscriberUnset)

	connSetup.Subscriber = func(context.Context, Connection, subscription.List) error { return nil }
	err = multi.SetupNewConnection(connSetup)
	require.ErrorIs(t, err, errWebsocketUnsubscriberUnset)

	connSetup.Unsubscriber = func(context.Context, Connection, subscription.List) error { return nil }
	err = multi.SetupNewConnection(connSetup)
	require.ErrorIs(t, err, errWebsocketDataHandlerUnset)

	connSetup.Handler = func(context.Context, Connection, []byte) error { return nil }
	connSetup.MessageFilter = []string{"slices are super naughty and not comparable"}
	err = multi.SetupNewConnection(connSetup)
	require.ErrorIs(t, err, errMessageFilterNotComparable)

	connSetup.MessageFilter = "comparable string signature"
	err = multi.SetupNewConnection(connSetup)
	require.NoError(t, err)

	require.Len(t, multi.connectionManager, 1)

	require.Nil(t, multi.AuthConn)
	require.Nil(t, multi.Conn)

	err = multi.SetupNewConnection(connSetup)
	require.ErrorIs(t, err, errDuplicateConnectionSetup)
}

func TestGetConfiguredWebsocketURLs(t *testing.T) {
	t.Parallel()

	var nilManager *Manager
	urls, err := nilManager.GetConfiguredWebsocketURLs()
	assert.ErrorIs(t, err, common.ErrNilPointer)
	assert.Nil(t, urls)

	single := NewManager()
	require.NoError(t, single.Setup(newDefaultSetup()))
	single.runningURL = "wss://single-running"
	urls, err = single.GetConfiguredWebsocketURLs()
	require.NoError(t, err)
	assert.Equal(t, []string{"wss://single-running"}, urls)

	single.runningURL = ""
	urls, err = single.GetConfiguredWebsocketURLs()
	require.NoError(t, err)
	assert.Equal(t, []string{single.defaultURL}, urls)

	single.defaultURL = ""
	urls, err = single.GetConfiguredWebsocketURLs()
	require.NoError(t, err)
	assert.Nil(t, urls, "Configured websocket URLs should be nil when no URLs are set")

	multi := NewManager()
	setup := newDefaultSetup()
	setup.UseMultiConnectionManagement = true
	require.NoError(t, multi.Setup(setup))

	connSetupOne := &ConnectionSetup{
		URL:                   "wss://one.example/ws",
		Connector:             func(context.Context, Connection) error { return nil },
		GenerateSubscriptions: func() (subscription.List, error) { return nil, nil },
		Subscriber:            func(context.Context, Connection, subscription.List) error { return nil },
		Unsubscriber:          func(context.Context, Connection, subscription.List) error { return nil },
		Handler:               func(context.Context, Connection, []byte) error { return nil },
		MessageFilter:         "one",
	}
	require.NoError(t, multi.SetupNewConnection(connSetupOne))

	connSetupTwo := &ConnectionSetup{
		URL:                   "wss://two.example/ws",
		Connector:             func(context.Context, Connection) error { return nil },
		GenerateSubscriptions: func() (subscription.List, error) { return nil, nil },
		Subscriber:            func(context.Context, Connection, subscription.List) error { return nil },
		Unsubscriber:          func(context.Context, Connection, subscription.List) error { return nil },
		Handler:               func(context.Context, Connection, []byte) error { return nil },
		MessageFilter:         "two",
	}
	require.NoError(t, multi.SetupNewConnection(connSetupTwo))

	urls, err = multi.GetConfiguredWebsocketURLs()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"wss://one.example/ws", "wss://two.example/ws"}, urls)
}

func TestConnectionShutdown(t *testing.T) {
	t.Parallel()
	wc := connection{shutdown: make(chan struct{})}
	err := wc.Shutdown()
	assert.NoError(t, err, "Shutdown should not error when connection.Connection is nil")

	err = wc.Dial(t.Context(), &gws.Dialer{}, nil, nil)
	assert.Error(t, err, "Dial should error correctly")
	assert.NotContains(t, err.Error(), "malformed ws or wss URL", "Dial should omit transport error text")

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler) }))
	defer mock.Close()

	wc.URL = "ws" + mock.URL[len("http"):] + "/ws"

	err = wc.Dial(t.Context(), &gws.Dialer{}, nil, nil)
	require.NoError(t, err, "Dial must not error")

	err = wc.Shutdown()
	require.NoError(t, err, "Shutdown must not error")
}

// TestLatency logic test
func TestLatency(t *testing.T) {
	t.Parallel()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler) }))
	defer mock.Close()

	r := &reporter{}
	exch := "Kraken"
	wc := &connection{
		ExchangeName:     exch,
		Verbose:          true,
		URL:              "ws" + mock.URL[len("http"):] + "/ws",
		ResponseMaxLimit: time.Second * 1,
		Match:            NewMatch(),
		Reporter:         r,
	}
	if wc.ProxyURL != "" && !useProxyTests {
		t.Skip("Proxy testing not enabled, skipping")
	}

	err := wc.Dial(t.Context(), &gws.Dialer{}, http.Header{}, nil)
	require.NoError(t, err)

	go readMessages(t, wc)

	req := testRequest{
		Event:        "subscribe",
		Pairs:        []string{currency.NewPairWithDelimiter("XBT", "USD", "/").String()},
		Subscription: testRequestData{Name: "ticker"},
		RequestID:    12346,
	}

	_, err = wc.SendMessageReturnResponse(t.Context(), request.Unset, req.RequestID, req)
	require.NoError(t, err)
	require.NotEmpty(t, r.t, "Latency must have a duration")
	require.Equal(t, exch, r.name, "Latency must have the correct exchange name")
}

func TestWriteToConn(t *testing.T) {
	t.Parallel()
	wc := connection{}
	require.ErrorIs(t, wc.writeToConn(t.Context(), request.Unset, func() error { return nil }), errWebsocketIsDisconnected)
	wc.setConnectedStatus(true)
	// No rate limits set
	require.NoError(t, wc.writeToConn(t.Context(), request.Unset, func() error { return nil }))
	// connection rate limit set
	// Use a longer interval so the second call always requires delay and hits ctx deadline checks deterministically.
	wc.RateLimit = request.NewWeightedRateLimitByDuration(time.Second)
	require.NoError(t, wc.writeToConn(t.Context(), request.Unset, func() error { return nil }))
	ctx, cancel := context.WithTimeout(t.Context(), 0) // deadline exceeded
	cancel()
	require.ErrorIs(t, wc.writeToConn(ctx, request.Unset, func() error { return nil }), context.DeadlineExceeded)
	wc.RateLimit = request.NewWeightedRateLimitByDuration(time.Millisecond)
	// definitions set but with fallover
	wc.RateLimitDefinitions = request.RateLimitDefinitions{
		request.Auth: request.NewWeightedRateLimitByDuration(time.Millisecond),
	}
	require.NoError(t, wc.writeToConn(t.Context(), request.Unset, func() error { return nil }))
	// match with global rate limit
	require.NoError(t, wc.writeToConn(t.Context(), request.Auth, func() error { return nil }))
	// definitions set but connection rate limiter not set
	wc.RateLimit = nil
	require.ErrorIs(t, wc.writeToConn(ctx, request.Unset, func() error { return nil }), errRateLimitNotFound)
}

func TestDrain(t *testing.T) {
	t.Parallel()
	drain(nil)
	ch := make(chan error)
	drain(ch)
	require.Empty(t, ch, "Drain must empty the channel")
	ch = make(chan error, 10)
	for range 10 {
		ch <- errors.New(testString)
	}
	drain(ch)
	require.Empty(t, ch, "Drain must empty the channel")
}

func TestMonitorFrame(t *testing.T) {
	t.Parallel()
	ws := Manager{}
	require.Panics(t, func() { ws.monitorFrame(t.Context(), nil, nil) }, "monitorFrame must panic on nil frame")
	require.Panics(t, func() { ws.monitorFrame(t.Context(), nil, func(context.Context) func() bool { return nil }) }, "monitorFrame must panic on nil function")
	ws.Wg.Add(1)
	ws.monitorFrame(t.Context(), &ws.Wg, func(context.Context) func() bool { return func() bool { return true } })
	ws.Wg.Wait()
}

func TestMonitorConnection(t *testing.T) {
	t.Parallel()
	ws := Manager{verbose: true, ReadMessageErrors: make(chan error, 1), ShutdownC: make(chan struct{})}
	// Handle timer expired and websocket disabled, shutdown everything.
	timer := time.NewTimer(0)
	ws.setState(connectedState)
	ws.connectionMonitorRunning.Store(true)
	require.True(t, ws.observeConnection(t.Context(), timer))
	require.False(t, ws.connectionMonitorRunning.Load())
	require.Equal(t, disconnectedState, ws.state.Load())
	// Handle timer expired and everything is great, reset the timer.
	ws.setEnabled(true)
	ws.setState(connectedState)
	ws.connectionMonitorRunning.Store(true)
	timer = time.NewTimer(0)
	require.False(t, ws.observeConnection(t.Context(), timer)) // Not shutting down
	// Handle timer expired and for reason its not connected, so lets happily connect again.
	ws.setState(disconnectedState)
	require.False(t, ws.observeConnection(t.Context(), timer)) // Connect is intentionally erroring
	// Handle error from a connection which will then trigger a reconnect
	ws.setState(connectedState)
	ws.DataHandler = stream.NewRelay(1)
	ws.ReadMessageErrors <- errConnectionFault
	timer = time.NewTimer(time.Second)
	require.False(t, ws.observeConnection(t.Context(), timer))
	payload := <-ws.DataHandler.C
	err, ok := payload.Data.(error)
	require.True(t, ok)
	require.ErrorIs(t, err, errConnectionFault)

	// Handle error while still in connecting state; state should be reset so reconnect can proceed.
	ws.setState(connectingState)
	ws.ReadMessageErrors <- errConnectionFault
	timer = time.NewTimer(time.Second)
	require.False(t, ws.observeConnection(t.Context(), timer))
	require.Equal(t, disconnectedState, ws.state.Load())

	shutdownFailureManager := NewManager()
	shutdownFailureManager.Conn = &diagnosticConnection{subscriptions: subscription.NewStore()}
	shutdownFailureManager.setState(connectedState)
	shutdownFailureManager.ReadMessageErrors <- errConnectionFault
	require.False(t, shutdownFailureManager.observeConnection(t.Context(), time.NewTimer(time.Second)), "observeConnection must continue after shutdown failures")
	require.Equal(t, disconnectedState, shutdownFailureManager.state.Load(), "observeConnection must leave a failed shutdown disconnected")

	disabledShutdownFailure := NewManager()
	disabledShutdownFailure.Conn = &diagnosticConnection{subscriptions: subscription.NewStore()}
	disabledShutdownFailure.verbose = true
	disabledShutdownFailure.connectionMonitorRunning.Store(true)
	disabledShutdownFailure.setState(connectedState)
	require.True(t, disabledShutdownFailure.observeConnection(t.Context(), time.NewTimer(0)), "observeConnection must exit after a disabled websocket shutdown failure")
	require.Equal(t, disconnectedState, disabledShutdownFailure.state.Load(), "observeConnection must leave a disabled failed shutdown disconnected")

	fullRelayManager := NewManager()
	fullRelayManager.DataHandler = stream.NewRelay(1)
	require.NoError(t, fullRelayManager.DataHandler.Send(t.Context(), "occupied"), "Send must fill the connection monitor relay")
	fullRelayManager.ReadMessageErrors <- errDastardlyReason
	require.False(t, fullRelayManager.observeConnection(t.Context(), time.NewTimer(time.Second)), "observeConnection must continue after relay failures")

	// Handle outta closure shell
	innerShell := ws.monitorConnection(t.Context())
	ws.setState(connectedState)
	ws.ReadMessageErrors <- errConnectionFault
	require.False(t, innerShell())
}

func TestMonitorTraffic(t *testing.T) { //nolint:tparallel // top-level parallel is safe; serial subtests limit websocket CI contention
	t.Parallel()

	newTimeoutSignal := func() <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}

	newManager := func() *Manager {
		return &Manager{
			verbose:      true,
			ShutdownC:    make(chan struct{}),
			TrafficAlert: make(chan struct{}, 1),
		}
	}

	t.Run("shutdown signal exits", func(t *testing.T) {
		ws := newManager()
		close(ws.ShutdownC)

		require.True(t, ws.observeTraffic(make(chan time.Time), nil))
	})

	t.Run("connecting keeps monitor alive", func(t *testing.T) {
		ws := newManager()
		ws.setState(connectingState)

		require.False(t, ws.observeTraffic(newTimeoutSignal(), nil))
	})

	t.Run("traffic keeps monitor alive", func(t *testing.T) {
		ws := newManager()
		ws.setState(connectedState)
		ws.TrafficAlert <- struct{}{}

		require.False(t, ws.observeTraffic(newTimeoutSignal(), nil))
	})

	t.Run("timeout invokes shutdown handler", func(t *testing.T) {
		ws := newManager()
		ws.setState(connectedState)

		shutdownCalled := false
		require.True(t, ws.observeTraffic(newTimeoutSignal(), func() {
			shutdownCalled = true
			ws.setState(disconnectedState)
		}))
		require.True(t, shutdownCalled, "timeout handler must be called when traffic is missing")
		require.Equal(t, disconnectedState, ws.state.Load())
	})

	t.Run("monitor traffic shell", func(t *testing.T) {
		ws := newManager()
		ws.trafficTimeout = time.Hour
		close(ws.ShutdownC)

		innerShell := ws.monitorTraffic(t.Context())
		require.True(t, innerShell())
	})
}

func TestGetConnection(t *testing.T) {
	t.Parallel()
	var ws *Manager
	_, err := ws.GetConnection(nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetConnection must reject a nil manager")
	require.ErrorContains(t, err, fmt.Sprintf("%T", ws), "GetConnection must retain nil manager type metadata")

	ws = &Manager{}

	_, err = ws.GetConnection(nil)
	require.ErrorIs(t, err, common.ErrNilPointer, "GetConnection must reject a nil message filter")
	require.ErrorContains(t, err, "messageFilter", "GetConnection must identify a nil message filter")

	_, err = ws.GetConnection("testURL")
	require.ErrorIs(t, err, errCannotObtainOutboundConnection, "GetConnection must reject disabled multi-connection management")

	ws.useMultiConnectionManagement = true

	_, err = ws.GetConnection("testURL")
	require.ErrorIs(t, err, ErrNotConnected, "GetConnection must reject a disconnected manager")

	ws.setState(connectedState)

	const messageFilterCanary = "message-filter-secret-token"
	_, err = ws.GetConnection(messageFilterCanary)
	require.ErrorIs(t, err, ErrRequestRouteNotFound, "GetConnection must reject an unmatched message filter")
	assert.Contains(t, err.Error(), "message filter type string", "GetConnection errors should retain message-filter type metadata")
	assert.NotContains(t, err.Error(), messageFilterCanary, "GetConnection errors should omit message-filter values")

	ws.connectionManager = []*websocket{
		{setup: &ConnectionSetup{MessageFilter: "differentURL", URL: "differentURL"}},
		{setup: &ConnectionSetup{
			MessageFilter: messageFilterCanary,
			URL:           "wss://filter-user-token:filter-password-token@filter.example.com/filter-path-token?signature=filter-query-token#filter-fragment-token",
		}},
	}

	_, err = ws.GetConnection(messageFilterCanary)
	require.ErrorIs(t, err, ErrNotConnected, "GetConnection must reject a matched setup without a connection")
	assert.Contains(t, err.Error(), "wss://filter.example.com", "GetConnection errors should retain the safe setup origin")
	assert.Contains(t, err.Error(), "message filter type string", "GetConnection errors should retain message-filter type metadata")
	for _, secret := range []string{messageFilterCanary, "filter-user-token", "filter-password-token", "filter-path-token", "filter-query-token", "filter-fragment-token"} {
		assert.NotContains(t, err.Error(), secret, "GetConnection errors should omit filter and setup values")
	}

	expected := &connection{subscriptions: subscription.NewStore()}
	ws.connectionManager[1].connections = []Connection{expected}

	conn, err := ws.GetConnection(messageFilterCanary)
	require.NoError(t, err, "GetConnection must return a matching connection")
	assert.Same(t, expected, conn, "GetConnection should return the matching connection")
}

func TestShutdown(t *testing.T) {
	t.Parallel()
	m := Manager{}
	m.setState(connectingState)
	require.ErrorIs(t, m.shutdown(), errAlreadyReconnecting, "shutdown must error correctly")
	m.setState(disconnectedState)
	require.ErrorIs(t, m.shutdown(), ErrNotConnected, "shutdown must error correctly")
	m.setState(connectedState)
	require.Panics(t, func() { _ = m.Shutdown() }, "Shutdown must panic on nil shutdown channel")
	m.ShutdownC = make(chan struct{})
	require.NoError(t, m.Shutdown(), "Shutdown must not error with no connections")
	m.setState(connectedState)
	m.Conn = &struct{ *connection }{&connection{}}
	m.AuthConn = &struct{ *connection }{&connection{}}
	require.ErrorIs(t, m.Shutdown(), common.ErrTypeAssertFailure, "Shutdown must error with unhandled connection type")

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mockws.WsMockUpgrader(t, w, r, mockws.EchoHandler) }))
	defer mock.Close()

	wsURL := "ws" + mock.URL[len("http"):] + "/ws"
	conn, resp, err := gws.DefaultDialer.DialContext(t.Context(), wsURL, nil)
	require.NoError(t, err, "DialContext must not error")
	defer resp.Body.Close()

	m.AuthConn = nil
	m.Conn = nil
	m.connectionManager = []*websocket{
		{connections: []Connection{&connection{Connection: nil, subscriptions: subscription.NewStore()}}},
		{connections: []Connection{&connection{Connection: conn, subscriptions: subscription.NewStore()}}},
	}
	m.setState(connectedState)
	require.NoError(t, m.Shutdown(), "Shutdown must not error with faulty connection in connectionManager")

	gwsConnAuth, respAuth, err := gws.DefaultDialer.DialContext(t.Context(), wsURL, nil)
	require.NoError(t, err, "DialContext must not error")
	defer respAuth.Body.Close()

	gwsConnUnAuth, respUnAuth, err := gws.DefaultDialer.DialContext(t.Context(), wsURL, nil)
	require.NoError(t, err, "DialContext must not error")
	defer respUnAuth.Body.Close()

	m.connectionManager = nil
	authConn := &connection{Connection: gwsConnAuth, shutdown: m.ShutdownC}
	m.AuthConn = authConn
	unauthConn := &connection{Connection: gwsConnUnAuth, shutdown: m.ShutdownC}
	m.Conn = unauthConn

	m.setState(connectedState)
	require.NoError(t, m.Shutdown(), "Shutdown must not error with good connections")

	require.Equal(t, m.ShutdownC, authConn.shutdown, "shutdown channels must be the same after original shutdown channel is closed")
	require.Equal(t, m.ShutdownC, unauthConn.shutdown, "shutdown channels must be the same after original shutdown channel is closed")

	m.connectionManager = []*websocket{{
		setup:         &ConnectionSetup{},
		subscriptions: subscription.NewStore(),
		connections:   []Connection{&diagnosticConnection{subscriptions: subscription.NewStore(), shutdownErr: errDastardlyReason}},
	}}
	m.setState(connectedState)
	require.NoError(t, m.shutdown(), "shutdown must treat connection close failures as non-fatal")
}
