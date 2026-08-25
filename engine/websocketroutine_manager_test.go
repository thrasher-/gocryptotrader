package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fill"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/protocol"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
)

type websocketDataHandlerSyncer struct {
	running   bool
	updateErr error
}

func (s *websocketDataHandlerSyncer) IsRunning() bool { return s.running }

func (*websocketDataHandlerSyncer) PrintTickerSummary(*ticker.Price, string, error) {}

func (*websocketDataHandlerSyncer) PrintOrderbookSummary(*orderbook.Book, string, error) {}

func (s *websocketDataHandlerSyncer) WebsocketUpdate(string, currency.Pair, asset.Item, syncItemType, error) error {
	return s.updateErr
}

type websocketDataHandlerOrderManager struct {
	running   bool
	exists    bool
	addErr    error
	getErr    error
	updateErr error
	order     *order.Detail
}

func (m *websocketDataHandlerOrderManager) IsRunning() bool { return m.running }

func (m *websocketDataHandlerOrderManager) Exists(*order.Detail) bool { return m.exists }

func (m *websocketDataHandlerOrderManager) Add(*order.Detail) error { return m.addErr }

func (*websocketDataHandlerOrderManager) Cancel(context.Context, *order.Cancel) error { return nil }

func (m *websocketDataHandlerOrderManager) GetByExchangeAndID(string, string) (*order.Detail, error) {
	return m.order, m.getErr
}

func (m *websocketDataHandlerOrderManager) UpdateExistingOrder(*order.Detail) error {
	return m.updateErr
}

type websocketRoutineExchangeManager struct {
	exchanges []exchange.IBotExchange
	err       error
}

func (m *websocketRoutineExchangeManager) GetExchanges() ([]exchange.IBotExchange, error) {
	return m.exchanges, m.err
}

func (*websocketRoutineExchangeManager) GetExchangeByName(string) (exchange.IBotExchange, error) {
	return nil, ErrExchangeNotFound
}

type websocketRoutineExchange struct {
	sharedtestvalues.CustomEx
	name              string
	supportsWebsocket bool
	manager           *websocket.Manager
	getWebsocketErr   error
}

func (e *websocketRoutineExchange) GetName() string { return e.name }

func (e *websocketRoutineExchange) SupportsWebsocket() bool { return e.supportsWebsocket }

func (e *websocketRoutineExchange) GetWebsocket() (*websocket.Manager, error) {
	return e.manager, e.getWebsocketErr
}

func TestWebsocketRoutineManagerSetup(t *testing.T) {
	_, err := setupWebsocketRoutineManager(nil, nil, nil, nil, false)
	assert.ErrorIs(t, err, errNilExchangeManager)

	_, err = setupWebsocketRoutineManager(NewExchangeManager(), (*OrderManager)(nil), nil, nil, false)
	assert.ErrorIs(t, err, errNilCurrencyPairSyncer)

	_, err = setupWebsocketRoutineManager(NewExchangeManager(), &OrderManager{}, &SyncManager{}, nil, false)
	assert.ErrorIs(t, err, errNilCurrencyConfig)

	_, err = setupWebsocketRoutineManager(NewExchangeManager(), &OrderManager{}, &SyncManager{}, &currency.Config{}, true)
	assert.ErrorIs(t, err, errNilCurrencyPairFormat)

	m, err := setupWebsocketRoutineManager(NewExchangeManager(), &OrderManager{}, &SyncManager{}, &currency.Config{CurrencyPairFormat: &currency.PairFormat{}}, false)
	assert.NoError(t, err)

	if m == nil {
		t.Error("expecting manager")
	}
}

func TestWebsocketRoutineManagerStart(t *testing.T) {
	var m *WebsocketRoutineManager
	err := m.Start(t.Context())
	assert.ErrorIs(t, err, ErrNilSubsystem)

	cfg := &currency.Config{CurrencyPairFormat: &currency.PairFormat{
		Uppercase: false,
		Delimiter: "-",
	}}
	m, err = setupWebsocketRoutineManager(NewExchangeManager(), &OrderManager{}, &SyncManager{}, cfg, true)
	assert.NoError(t, err)

	err = m.Start(t.Context())
	assert.NoError(t, err)

	err = m.Start(t.Context())
	assert.ErrorIs(t, err, ErrSubSystemAlreadyStarted)

	err = m.Stop()
	assert.NoError(t, err)
}

func TestWebsocketRoutineManagerIsRunning(t *testing.T) {
	var m *WebsocketRoutineManager
	if m.IsRunning() {
		t.Error("expected false")
	}

	m, err := setupWebsocketRoutineManager(NewExchangeManager(), &OrderManager{}, &SyncManager{}, &currency.Config{CurrencyPairFormat: &currency.PairFormat{}}, false)
	assert.NoError(t, err)

	if m.IsRunning() {
		t.Error("expected false")
	}

	err = m.Start(t.Context())
	assert.NoError(t, err)

	for m.state.Load() == startingState {
		<-time.After(time.Second / 100)
	}
	if !m.IsRunning() {
		t.Error("expected true")
	}
}

func TestWebsocketRoutineManagerStop(t *testing.T) {
	var m *WebsocketRoutineManager
	err := m.Stop()
	assert.ErrorIs(t, err, ErrNilSubsystem)

	m, err = setupWebsocketRoutineManager(NewExchangeManager(), &OrderManager{}, &SyncManager{}, &currency.Config{CurrencyPairFormat: &currency.PairFormat{}}, false)
	assert.NoError(t, err)

	err = m.Stop()
	assert.ErrorIs(t, err, ErrSubSystemNotStarted)

	err = m.Start(t.Context())
	assert.NoError(t, err)

	err = m.Stop()
	assert.NoError(t, err)
}

func TestWebsocketRoutineManagerConcurrentStartStop(t *testing.T) {
	cfg := &currency.Config{CurrencyPairFormat: &currency.PairFormat{}}
	for range 128 {
		m, err := setupWebsocketRoutineManager(NewExchangeManager(), &OrderManager{}, &SyncManager{}, cfg, false)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = m.Start(t.Context())
		}()
		go func() {
			defer wg.Done()
			_ = m.Stop()
		}()
		wg.Wait()

		if m.state.Load() != stoppedState {
			require.NoError(t, m.Stop())
		}
		assert.Nil(t, m.connectionCancel)
	}
}

func TestWebsocketRoutineManagerHandleData(t *testing.T) {
	exchName := "Bitstamp"
	var wg sync.WaitGroup
	em := NewExchangeManager()
	exch, err := em.NewExchangeByName(exchName)
	require.NoError(t, err)

	exch.SetDefaults()
	err = em.Add(exch)
	require.NoError(t, err)

	om, err := SetupOrderManager(em, &CommunicationManager{}, &wg, &config.OrderManager{})
	assert.NoError(t, err)

	err = om.Start(t.Context())
	assert.NoError(t, err)

	cfg := &currency.Config{CurrencyPairFormat: &currency.PairFormat{
		Uppercase: false,
		Delimiter: "-",
	}}
	m, err := setupWebsocketRoutineManager(em, om, &SyncManager{}, cfg, true)
	assert.NoError(t, err)

	err = m.Start(t.Context())
	assert.NoError(t, err)

	orderID := "1337"
	err = m.websocketDataHandler(exchName, errors.New("error"))
	if err == nil {
		t.Error("Error not handled correctly")
	}
	err = m.websocketDataHandler(exchName, websocket.FundingData{})
	if err != nil {
		t.Error(err)
	}
	err = m.websocketDataHandler(exchName, &ticker.Price{
		ExchangeName: exchName,
		Pair:         currency.NewPair(currency.BTC, currency.USDC),
		AssetType:    asset.Spot,
	})
	assert.NoError(t, err)

	err = m.websocketDataHandler(exchName, kline.Item{})
	require.NoError(t, err)
	origOrder := &order.Detail{
		Exchange: exchName,
		OrderID:  orderID,
		Amount:   1337,
		Price:    1337,
	}
	err = m.websocketDataHandler(exchName, origOrder)
	if err != nil {
		t.Error(err)
	}
	// Send it again since it exists now
	err = m.websocketDataHandler(exchName, &order.Detail{
		Exchange: exchName,
		OrderID:  orderID,
		Amount:   1338,
	})
	if err != nil {
		t.Error(err)
	}
	updated, err := m.orderManager.GetByExchangeAndID(origOrder.Exchange, origOrder.OrderID)
	if err != nil {
		t.Error(err)
	}
	if updated.Amount != 1338 {
		t.Error("Bad pipeline")
	}

	err = m.websocketDataHandler(exchName, &order.Detail{
		Exchange: "Bitstamp",
		OrderID:  orderID,
		Status:   order.Active,
	})
	if err != nil {
		t.Error(err)
	}
	updated, err = m.orderManager.GetByExchangeAndID(origOrder.Exchange, origOrder.OrderID)
	if err != nil {
		t.Error(err)
	}
	if updated.Status != order.Active {
		t.Error("Expected order to be modified to Active")
	}

	// Send some gibberish
	err = m.websocketDataHandler(exchName, order.Stop)
	if err != nil {
		t.Error(err)
	}

	err = m.websocketDataHandler(exchName, websocket.UnhandledMessageWarning{
		Message: "there's an issue here's a tissue",
	})
	if err != nil {
		t.Error(err)
	}

	classificationError := order.ClassificationError{
		Exchange: "test",
		OrderID:  "one",
		Err:      errors.New("lol"),
	}
	err = m.websocketDataHandler(exchName, classificationError)
	if err == nil {
		t.Error("Expected error")
	}
	assert.ErrorIs(t, err, classificationError.Err)

	err = m.websocketDataHandler(exchName, &orderbook.Book{
		Exchange: "Bitstamp",
		Pair:     currency.NewBTCUSD(),
	})
	if err != nil {
		t.Error(err)
	}
	err = m.websocketDataHandler(exchName, "this is a test string")
	if err != nil {
		t.Error(err)
	}
}

func TestWebsocketRoutineDiagnostics(t *testing.T) {
	getExchangesCanary := "raw-get-exchanges-secret-token"
	getWebsocketCanary := "raw-get-websocket-secret-token"
	connectCanary := "raw-connect-secret-token"
	cancelledConnectCanary := "raw-cancelled-connect-secret-token"

	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable websocketRoutine diagnostic capture")
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
		assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable websocketRoutine diagnostic capture")
	})

	newWebsocketManager := func(name string, connector func() error) *websocket.Manager {
		ws := websocket.NewManager()
		t.Cleanup(func() {
			select {
			case <-ws.ShutdownC:
			default:
				close(ws.ShutdownC)
			}
			ws.Wg.Wait()
		})
		require.NoError(t, ws.Setup(&websocket.ManagerSetup{
			ExchangeConfig: &config.Exchange{
				Name:                    name,
				WebsocketTrafficTimeout: time.Minute,
				ConnectionMonitorDelay:  time.Millisecond,
				Features: &config.FeaturesConfig{
					Enabled: config.FeaturesEnabledConfig{Websocket: true},
				},
			},
			DefaultURL: "wss://example.com",
			RunningURL: "wss://example.com",
			Connector:  connector,
			Subscriber: func(subscription.List) error { return nil },
			GenerateSubscriptions: func() (subscription.List, error) {
				return nil, nil
			},
			Features: &protocol.Features{},
		}), "Setup must configure the websocketRoutine test manager")
		return ws
	}

	unsupported := &websocketRoutineExchange{name: "unsupported-no-verbose"}
	(&WebsocketRoutineManager{
		exchangeManager: &websocketRoutineExchangeManager{
			exchanges: []exchange.IBotExchange{unsupported},
			err:       errors.New(getExchangesCanary),
		},
	}).websocketRoutine(t.Context())

	receiverFailureWS := newWebsocketManager("receiver-failure", func() error { return errors.New(connectCanary) })
	receiverFailureCtx, cancelReceiverFailure := context.WithCancel(t.Context())
	(&WebsocketRoutineManager{exchangeManager: &websocketRoutineExchangeManager{exchanges: []exchange.IBotExchange{
		&websocketRoutineExchange{name: "receiver-failure", supportsWebsocket: true, manager: receiverFailureWS},
	}}}).websocketRoutine(receiverFailureCtx)
	cancelReceiverFailure()

	failedConnectWS := newWebsocketManager("connect-failure", func() error { return errors.New(connectCanary) })
	successfulConnectWS := newWebsocketManager("connect-success", func() error { return nil })
	manager := &WebsocketRoutineManager{
		verbose: true,
		exchangeManager: &websocketRoutineExchangeManager{exchanges: []exchange.IBotExchange{
			&websocketRoutineExchange{name: "unsupported", supportsWebsocket: false},
			&websocketRoutineExchange{name: "websocket-lookup-failure", supportsWebsocket: true, getWebsocketErr: errors.New(getWebsocketCanary)},
			&websocketRoutineExchange{name: "disabled", supportsWebsocket: true, manager: websocket.NewManager()},
			&websocketRoutineExchange{name: "connect-failure", supportsWebsocket: true, manager: failedConnectWS},
			&websocketRoutineExchange{name: "connect-success", supportsWebsocket: true, manager: successfulConnectWS},
		}},
		shutdown: make(chan struct{}),
	}
	manager.state.Store(readyState)
	ctx, cancel := context.WithCancel(t.Context())
	manager.websocketRoutine(ctx)
	close(manager.shutdown)
	manager.wg.Wait()
	require.True(t, successfulConnectWS.IsConnected(), "websocketRoutine must establish the successful test connection")
	successfulShutdown := successfulConnectWS.ShutdownC
	require.NoError(t, successfulConnectWS.Disable(), "Disable must disable the successful websocketRoutine connection")
	monitorTimeout := time.NewTimer(time.Second)
	defer monitorTimeout.Stop()
	select {
	case <-successfulShutdown:
	case <-monitorTimeout.C:
		require.FailNow(t, "websocketRoutine must let the connection monitor shut down the disabled connection")
	}
	require.ErrorIs(t, successfulConnectWS.Shutdown(), websocket.ErrNotConnected, "Shutdown must confirm the connection monitor completed shutdown")
	cancel()

	var receiverCancelledConnectorCalls int
	cancelledReceiverWS := newWebsocketManager("receiver-cancelled", func() error {
		receiverCancelledConnectorCalls++
		return nil
	})
	cancelledCtx, cancelReceiver := context.WithCancel(t.Context())
	cancelReceiver()
	(&WebsocketRoutineManager{exchangeManager: &websocketRoutineExchangeManager{exchanges: []exchange.IBotExchange{
		&websocketRoutineExchange{name: "receiver-cancelled", supportsWebsocket: true, manager: cancelledReceiverWS},
	}}}).websocketRoutine(cancelledCtx)
	assert.Zero(t, receiverCancelledConnectorCalls, "websocketRoutine should not connect after receiver shutdown cancellation")

	connectCtx, cancelConnect := context.WithCancel(t.Context())
	cancelledConnectWS := newWebsocketManager("connect-cancelled", func() error {
		cancelConnect()
		return errors.New(cancelledConnectCanary)
	})
	cancelledConnectManager := &WebsocketRoutineManager{
		exchangeManager: &websocketRoutineExchangeManager{exchanges: []exchange.IBotExchange{
			&websocketRoutineExchange{name: "connect-cancelled", supportsWebsocket: true, manager: cancelledConnectWS},
		}},
		shutdown: make(chan struct{}),
	}
	cancelledConnectManager.state.Store(readyState)
	cancelledConnectManager.websocketRoutine(connectCtx)
	close(cancelledConnectManager.shutdown)
	cancelledConnectManager.wg.Wait()

	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	logMu.Unlock()
	assert.Contains(t, diagnostics, "websocket routine manager cannot get exchanges error=operation failed (", "websocketRoutine should log GetExchanges error metadata")
	assert.Contains(t, diagnostics, "exchange websocket-lookup-failure GetWebsocket failed error=operation failed (", "websocketRoutine should log GetWebsocket error metadata")
	assert.Contains(t, diagnostics, "exchange receiver-failure websocket data receiver failed error=operation failed (", "websocketRoutine should log receiver error metadata")
	assert.Contains(t, diagnostics, "exchange connect-failure websocket connect failed error=operation failed (", "websocketRoutine should log connection error metadata")
	for _, canary := range []string{getExchangesCanary, getWebsocketCanary, connectCanary, cancelledConnectCanary, errRoutineManagerNotStarted.Error()} {
		assert.NotContains(t, diagnostics, canary, "websocketRoutine should omit raw error data")
	}
}

func TestWebsocketDataReceiverDiagnostics(t *testing.T) {
	var manager *WebsocketRoutineManager
	ws := websocket.NewManager()
	assert.ErrorIs(t, manager.websocketDataReceiver(ws), ErrNilSubsystem, "websocketDataReceiver should reject a nil manager")

	manager = new(WebsocketRoutineManager)
	assert.ErrorIs(t, manager.websocketDataReceiver(nil), errNilWebsocket, "websocketDataReceiver should reject a nil websocket")
	assert.ErrorIs(t, manager.websocketDataReceiver(ws), errRoutineManagerNotStarted, "websocketDataReceiver should reject a stopped manager")

	manager.state.Store(readyState)
	assert.ErrorIs(t, manager.websocketDataReceiver(ws), errRoutineManagerNotStarted, "websocketDataReceiver should reject a manager without a shutdown channel")

	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable websocketDataReceiver diagnostic capture")
	var logMu sync.Mutex
	var entries []string
	gctlog.SetCustomLogHook(func(_, _ string, a ...any) bool {
		entry := fmt.Sprint(a...)
		if !strings.Contains(entry, "receiver-diagnostic-test") {
			return true
		}
		logMu.Lock()
		entries = append(entries, entry)
		logMu.Unlock()
		return true
	})
	t.Cleanup(func() {
		gctlog.SetCustomLogHook(nil)
		assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable websocketDataReceiver diagnostic capture")
	})

	ws = websocket.NewManager()
	require.NoError(t, ws.Setup(&websocket.ManagerSetup{
		ExchangeConfig: &config.Exchange{
			Name:                    "receiver-diagnostic-test",
			WebsocketTrafficTimeout: time.Second,
			Features:                &config.FeaturesConfig{},
		},
		DefaultURL: "wss://example.com",
		RunningURL: "wss://example.com",
		Connector:  func() error { return nil },
		Subscriber: func(subscription.List) error { return nil },
		GenerateSubscriptions: func() (subscription.List, error) {
			return nil, nil
		},
		Features: &protocol.Features{},
	}), "Setup must configure the websocketDataReceiver test manager")

	handlerCanary := "raw-handler-secret-token"
	handled := make(chan struct{}, 4)
	manager = &WebsocketRoutineManager{
		shutdown: make(chan struct{}),
		dataHandlers: []WebsocketDataHandler{
			func(string, any) error {
				handled <- struct{}{}
				return errors.New(handlerCanary)
			},
			func(string, any) error {
				handled <- struct{}{}
				return nil
			},
		},
	}
	manager.state.Store(readyState)
	require.NoError(t, manager.websocketDataReceiver(ws), "websocketDataReceiver must start for a ready manager")
	require.NoError(t, ws.DataHandler.Send(t.Context(), nil), "DataHandler.Send must relay nil test data")
	require.NoError(t, ws.DataHandler.Send(t.Context(), "safe-payload"), "DataHandler.Send must relay non-nil test data")
	for range 4 {
		<-handled
	}
	close(manager.shutdown)
	manager.wg.Wait()

	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	logMu.Unlock()
	assert.Contains(t, diagnostics, "exchange receiver-diagnostic-test nil data sent to websocket", "websocketDataReceiver should log nil-data metadata")
	assert.Equal(t, 2, strings.Count(diagnostics, "exchange receiver-diagnostic-test websocket data handler failed error=operation failed ("), "websocketDataReceiver should log handler error metadata for each payload")
	assert.NotContains(t, diagnostics, handlerCanary, "websocketDataReceiver should omit raw handler error data")
}

func TestWebsocketDataHandlerDiagnostics(t *testing.T) {
	exchName := "websocket-handler-diagnostic-test"
	stringCanary := "raw-string-secret-token"
	errorCanary := "raw-error-secret-token"
	unhandledCanary := "raw-unhandled-secret-token"
	unknownCanary := "raw-unknown-secret-token"

	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable diagnostic capture")
	var logMu sync.Mutex
	var entries []string
	gctlog.SetCustomLogHook(func(_, _ string, a ...any) bool {
		entry := fmt.Sprint(a...)
		if !strings.Contains(entry, exchName) {
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

	m := &WebsocketRoutineManager{verbose: true}
	require.NoError(t, m.websocketDataHandler(exchName, stringCanary), "websocketDataHandler must handle string diagnostics")

	sourceErr := errors.New(errorCanary)
	handlerErr := m.websocketDataHandler(exchName, sourceErr)
	require.Error(t, handlerErr, "websocketDataHandler must return websocket errors")
	assert.Contains(t, handlerErr.Error(), fmt.Sprintf("exchange %s websocket error: operation failed (", exchName), "websocketDataHandler errors should retain safe exchange and operation context")
	assert.ErrorIs(t, handlerErr, sourceErr, "websocketDataHandler should preserve the underlying websocket error")

	require.NoError(t, m.websocketDataHandler(exchName, websocket.UnhandledMessageWarning{Message: unhandledCanary}), "websocketDataHandler must handle unhandled-message diagnostics")
	unknownPayload := struct{ Canary string }{Canary: unknownCanary}
	require.NoError(t, m.websocketDataHandler(exchName, unknownPayload), "websocketDataHandler must handle unknown-message diagnostics")

	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	entryCount := len(entries)
	logMu.Unlock()
	require.Equal(t, 3, entryCount, "websocketDataHandler must emit three diagnostic entries")
	assert.Contains(t, diagnostics, fmt.Sprintf("%s websocket message bytes=%d", exchName, len(stringCanary)), "websocketDataHandler should log string size metadata")
	assert.Contains(t, diagnostics, fmt.Sprintf("%s unhandled websocket message bytes=%d", exchName, len(unhandledCanary)), "websocketDataHandler should log unhandled-message size metadata")
	assert.Contains(t, diagnostics, fmt.Sprintf("%s websocket unknown type=%T", exchName, unknownPayload), "websocketDataHandler should log unknown-message type metadata")

	diagnosticsAndError := diagnostics + "\n" + handlerErr.Error()
	for _, canary := range []string{stringCanary, errorCanary, unhandledCanary, unknownCanary} {
		assert.NotContains(t, diagnosticsAndError, canary, "websocketDataHandler should omit raw websocket data")
	}
}

func TestWebsocketDataHandlerAllPaths(t *testing.T) {
	exchName := "websocket-handler-all-paths-test"
	pair := currency.NewBTCUSD()
	syncErr := errors.New("sync failure")
	addErr := errors.New("add failure")
	getErr := errors.New("get failure")
	updateErr := errors.New("update failure")
	depthErr := errors.New("depth failure")
	classificationErr := errors.New("classification failure")

	newManager := func(syncer *websocketDataHandlerSyncer, orderManager *websocketDataHandlerOrderManager) *WebsocketRoutineManager {
		return &WebsocketRoutineManager{verbose: true, syncer: syncer, orderManager: orderManager}
	}
	newTicker := func(name string) ticker.Price {
		return ticker.Price{ExchangeName: name, Pair: pair, AssetType: asset.Spot}
	}
	newDepth := func() *orderbook.Depth {
		depth := orderbook.NewDepth(uuid.Nil)
		depth.AssignOptions(&orderbook.Book{Exchange: exchName, Pair: pair, Asset: asset.Spot})
		return depth
	}
	newOrder := func() *order.Detail {
		return &order.Detail{Exchange: exchName, OrderID: "order-id", Pair: pair, AssetType: asset.Spot}
	}

	invalidDepth := newDepth()
	require.ErrorIs(t, invalidDepth.Invalidate(depthErr), depthErr, "Invalidate must retain the test error")
	validDepth := newDepth()

	for _, tc := range []struct {
		name    string
		manager *WebsocketRoutineManager
		data    any
		wantErr error
	}{
		{name: "funding", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: websocket.FundingData{}},
		{name: "ticker sync error", manager: newManager(&websocketDataHandlerSyncer{running: true, updateErr: syncErr}, &websocketDataHandlerOrderManager{}), data: func() *ticker.Price { value := newTicker("ticker-sync-error"); return &value }(), wantErr: syncErr},
		{name: "ticker sync success", manager: newManager(&websocketDataHandlerSyncer{running: true}, &websocketDataHandlerOrderManager{}), data: func() *ticker.Price { value := newTicker("ticker-sync-success"); return &value }()},
		{name: "ticker processing error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: &ticker.Price{}, wantErr: common.ErrExchangeNameNotSet},
		{name: "empty ticker slice", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: []ticker.Price{}},
		{name: "ticker slice sync error", manager: newManager(&websocketDataHandlerSyncer{running: true, updateErr: syncErr}, &websocketDataHandlerOrderManager{}), data: []ticker.Price{newTicker("ticker-slice-sync-error")}, wantErr: syncErr},
		{name: "ticker slice sync success", manager: newManager(&websocketDataHandlerSyncer{running: true}, &websocketDataHandlerOrderManager{}), data: []ticker.Price{newTicker("ticker-slice-sync-success")}},
		{name: "ticker slice processing error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: []ticker.Price{{}}, wantErr: common.ErrExchangeNameNotSet},
		{name: "value requires pointer", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: order.Detail{}, wantErr: errUseAPointer},
		{name: "kline", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: kline.Item{Pair: pair, Asset: asset.Spot}},
		{name: "empty kline slice", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: []kline.Item{}},
		{name: "kline slice", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: []kline.Item{{Pair: pair, Asset: asset.Spot}}},
		{name: "orderbook retrieval error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: invalidDepth, wantErr: depthErr},
		{name: "orderbook sync error", manager: newManager(&websocketDataHandlerSyncer{running: true, updateErr: syncErr}, &websocketDataHandlerOrderManager{}), data: validDepth, wantErr: syncErr},
		{name: "orderbook sync success", manager: newManager(&websocketDataHandlerSyncer{running: true}, &websocketDataHandlerOrderManager{}), data: validDepth},
		{name: "orderbook without sync", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: validDepth},
		{name: "order manager stopped", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: newOrder()},
		{name: "order add error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, addErr: addErr}), data: newOrder(), wantErr: addErr},
		{name: "order add success", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true}), data: newOrder()},
		{name: "order get error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, exists: true, getErr: getErr}), data: newOrder(), wantErr: getErr},
		{name: "order detail update error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, exists: true}), data: newOrder(), wantErr: order.ErrOrderDetailIsNil},
		{name: "order store update error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, exists: true, order: newOrder(), updateErr: updateErr}), data: newOrder(), wantErr: updateErr},
		{name: "order update success", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, exists: true, order: newOrder()}), data: newOrder()},
		{name: "order slice manager stopped", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: []order.Detail{*newOrder()}},
		{name: "empty order slice", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true}), data: []order.Detail{}},
		{name: "order slice add error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, addErr: addErr}), data: []order.Detail{*newOrder()}, wantErr: addErr},
		{name: "order slice add success", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true}), data: []order.Detail{*newOrder()}},
		{name: "order slice get error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, exists: true, getErr: getErr}), data: []order.Detail{*newOrder()}, wantErr: getErr},
		{name: "order slice detail update error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, exists: true}), data: []order.Detail{*newOrder()}, wantErr: order.ErrOrderDetailIsNil},
		{name: "order slice store update error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, exists: true, order: newOrder(), updateErr: updateErr}), data: []order.Detail{*newOrder()}, wantErr: updateErr},
		{name: "order slice update success", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{running: true, exists: true, order: newOrder()}), data: []order.Detail{*newOrder()}},
		{name: "classification error", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: order.ClassificationError{Exchange: exchName, Err: classificationErr}, wantErr: classificationErr},
		{name: "account change", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: accounts.Change{}},
		{name: "account changes", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: []accounts.Change{{}}},
		{name: "trade", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: trade.Data{}},
		{name: "trades", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: []trade.Data{{}}},
		{name: "fills", manager: newManager(&websocketDataHandlerSyncer{}, &websocketDataHandlerOrderManager{}), data: []fill.Data{{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.manager.websocketDataHandler(exchName, tc.data)
			if tc.wantErr != nil {
				require.ErrorIsf(t, err, tc.wantErr, "websocketDataHandler must return the expected error for %s", tc.name)
				return
			}
			require.NoErrorf(t, err, "websocketDataHandler must not error for %s", tc.name)
		})
	}
}

func TestRegisterWebsocketDataHandlerWithFunctionality(t *testing.T) {
	t.Parallel()
	var m *WebsocketRoutineManager
	err := m.registerWebsocketDataHandler(nil, false)
	require.ErrorIs(t, err, ErrNilSubsystem)

	m = new(WebsocketRoutineManager)
	m.shutdown = make(chan struct{})

	err = m.registerWebsocketDataHandler(nil, false)
	require.ErrorIs(t, err, errNilWebsocketDataHandlerFunction)

	// externally defined capture device
	dataChan := make(chan any)
	fn := func(_ string, data any) error {
		switch data.(type) {
		case string:
			dataChan <- data
		default:
		}
		return nil
	}

	err = m.registerWebsocketDataHandler(fn, true)
	require.NoError(t, err)

	if len(m.dataHandlers) != 1 {
		t.Fatal("unexpected data handlers registered")
	}

	mock := websocket.NewManager()
	m.state.Store(readyState)
	err = m.websocketDataReceiver(mock)
	if err != nil {
		t.Fatal(err)
	}

	err = mock.DataHandler.Send(t.Context(), nil)
	require.NoError(t, err)
	err = mock.DataHandler.Send(t.Context(), 1336)
	require.NoError(t, err)
	err = mock.DataHandler.Send(t.Context(), "intercepted")
	require.NoError(t, err)

	if r := <-dataChan; r != "intercepted" {
		t.Fatal("unexpected value received")
	}

	close(m.shutdown)
	m.wg.Wait()
}

func TestSetWebsocketDataHandler(t *testing.T) {
	t.Parallel()
	var m *WebsocketRoutineManager
	err := m.setWebsocketDataHandler(nil)
	require.ErrorIs(t, err, ErrNilSubsystem)

	m = new(WebsocketRoutineManager)
	m.shutdown = make(chan struct{})

	err = m.setWebsocketDataHandler(nil)
	require.ErrorIs(t, err, errNilWebsocketDataHandlerFunction)

	err = m.registerWebsocketDataHandler(m.websocketDataHandler, false)
	require.NoError(t, err)

	err = m.registerWebsocketDataHandler(m.websocketDataHandler, false)
	require.NoError(t, err)

	err = m.registerWebsocketDataHandler(m.websocketDataHandler, false)
	require.NoError(t, err)

	if len(m.dataHandlers) != 3 {
		t.Fatal("unexpected data handler count")
	}

	err = m.setWebsocketDataHandler(m.websocketDataHandler)
	require.NoError(t, err)

	if len(m.dataHandlers) != 1 {
		t.Fatal("unexpected data handler count")
	}
}
