package kraken

import (
	"context"
	"encoding/base64"
	"slices"
	"sync"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fill"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	testsubs "github.com/thrasher-corp/gocryptotrader/internal/testing/subscriptions"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// futuresTestChallenge is the challenge the mock futures server issues
const futuresTestChallenge = "226aee50-88fc-4618-a42a-34f7709570b2"

// mockFuturesServer answers futures requests as Kraken does: a challenge with futuresTestChallenge, and a subscription
// or unsubscription with one reply for each product, or one for a feed without products, in the order requested. It
// records each request
type mockFuturesServer struct {
	m        sync.Mutex
	requests []string
	// alerts maps a product, or a feed without products, to the alert its request is answered with
	alerts map[string]string
	// challengeReply replaces the reply to a challenge request
	challengeReply string
	// wrongProduct acknowledges a product that was not requested
	wrongProduct bool
}

func (s *mockFuturesServer) handle(_ testing.TB, msg []byte, w *gws.Conn) error {
	var req wsFuturesRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		return err
	}
	s.m.Lock()
	s.requests = append(s.requests, string(msg))
	alerts, challengeReply, wrongProduct := s.alerts, s.challengeReply, s.wrongProduct
	s.m.Unlock()
	reply := func(v map[string]any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return w.WriteMessage(gws.TextMessage, b)
	}
	if req.Event == wsFuturesEventChallenge {
		if challengeReply != "" {
			return w.WriteMessage(gws.TextMessage, []byte(challengeReply))
		}
		return reply(map[string]any{"event": wsFuturesEventChallenge, "message": futuresTestChallenge})
	}
	targets := req.ProductIDs
	if len(targets) == 0 {
		targets = []string{req.Feed}
	}
	for _, target := range targets {
		if alert, ok := alerts[target]; ok {
			if err := reply(map[string]any{"event": wsFuturesEventAlert, "message": alert}); err != nil {
				return err
			}
			continue
		}
		ack := map[string]any{"event": req.Event + "d", "feed": req.Feed}
		if len(req.ProductIDs) != 0 {
			ack["product_ids"] = []string{target}
			if wrongProduct {
				ack["product_ids"] = []string{"PF_MOONUSD"}
			}
		}
		if req.SignedChallenge != "" {
			ack["api_key"], ack["original_challenge"], ack["signed_challenge"] = req.APIKey, req.OriginalChallenge, req.SignedChallenge
		}
		if err := reply(ack); err != nil {
			return err
		}
	}
	return nil
}

// takeRequests returns and forgets the requests received
func (s *mockFuturesServer) takeRequests() []string {
	s.m.Lock()
	defer s.m.Unlock()
	r := s.requests
	s.requests = nil
	return r
}

// newFuturesTestExchange returns an exchange with test credentials whose futures connection, connected without
// subscriptions, is served by server
func newFuturesTestExchange(t *testing.T, server *mockFuturesServer) (*Exchange, websocket.Connection) {
	t.Helper()
	ex := newTestExchange(t)
	useTestCredentials(ex)
	ex.Features.Subscriptions = subscription.List{}
	useTestWebsocket(t, ex, newMockWebsocketURL(t, server.handle), wsFuturesConnection)
	ex.Websocket.SetSubscriptionsNotRequired()
	require.NoError(t, ex.Websocket.Connect(t.Context()), "Connect must not error")
	conn, err := ex.Websocket.GetConnection(wsFuturesConnection)
	require.NoError(t, err, "GetConnection must not error")
	return ex, conn
}

// futuresTestSignedChallenge returns futuresTestChallenge signed with the test credentials' secret
func futuresTestSignedChallenge(t *testing.T) string {
	t.Helper()
	signed, err := futuresChallengeSignature(decodedTestSecret(t), futuresTestChallenge)
	require.NoError(t, err, "futuresChallengeSignature must not error")
	return signed
}

// futuresSubscription returns a futures subscription to a feed, as template expansion gives it
func futuresSubscription(channel, feed string, pairs ...currency.Pair) *subscription.Subscription {
	return &subscription.Subscription{Enabled: true, Asset: asset.Futures, Channel: channel, QualifiedChannel: feed, Pairs: pairs, Authenticated: slices.Contains(futuresAccountFeeds, feed)}
}

func TestFuturesChallengeSignature(t *testing.T) {
	t.Parallel()
	// Kraken's documented example
	secret, err := base64.StdEncoding.DecodeString("7zxMEF5p/Z8l2p2U7Ghv6x14Af+Fx+92tPgUdVQ748FOIrEoT9bgT+bTRfXc5pz8na+hL/QdrCVG7bh9KpT0eMTm")
	require.NoError(t, err, "DecodeString must not error")
	signed, err := futuresChallengeSignature(string(secret), "c100b894-1729-464d-ace1-52dbce11db42")
	require.NoError(t, err, "futuresChallengeSignature must not error")
	assert.Equal(t, "4JEpF3ix66GA2B+ooK128Ift4XQVtc137N9yeg4Kqsn9PI0Kpzbysl9M1IeCEdjg0zl00wkVqcsnG4bmnlMb3A==", signed, "futuresChallengeSignature should match Kraken's documented example")
}

func TestGenerateFuturesSubscriptions(t *testing.T) {
	t.Parallel()
	ex := newTestExchange(t)
	require.NoError(t, ex.CurrencyPairs.EnablePair(asset.Futures, piXBTUSDPair), "EnablePair must not error")
	ex.Features.Subscriptions = append(defaultSubscriptions.Clone(), &subscription.Subscription{Enabled: true, Asset: asset.Futures, Channel: subscription.HeartbeatChannel})
	public := subscription.List{
		futuresSubscription(subscription.TickerChannel, wsFuturesFeedTicker, futuresTestPair),
		futuresSubscription(subscription.TickerChannel, wsFuturesFeedTicker, piXBTUSDPair),
		futuresSubscription(subscription.AllTradesChannel, wsFuturesFeedTrade, futuresTestPair),
		futuresSubscription(subscription.AllTradesChannel, wsFuturesFeedTrade, piXBTUSDPair),
		futuresSubscription(subscription.OrderbookChannel, wsFuturesFeedBook, futuresTestPair),
		futuresSubscription(subscription.OrderbookChannel, wsFuturesFeedBook, piXBTUSDPair),
		// A feed without products is subscribed to once
		futuresSubscription(subscription.HeartbeatChannel, wsFuturesFeedHeartbeat),
	}
	ex.Websocket.SetCanUseAuthenticatedEndpoints(false)
	got, err := ex.generateFuturesSubscriptions()
	require.NoError(t, err, "generateFuturesSubscriptions must not error")
	testsubs.EqualLists(t, public, got)

	useTestCredentials(ex)
	ex.Websocket.SetCanUseAuthenticatedEndpoints(true)
	got, err = ex.generateFuturesSubscriptions()
	require.NoError(t, err, "generateFuturesSubscriptions must not error with credentials")
	exp := append(public.Clone(),
		futuresSubscription(subscription.MyOrdersChannel, wsFuturesFeedOpenOrders),
		futuresSubscription(subscription.MyTradesChannel, wsFuturesFeedFills),
	)
	testsubs.EqualLists(t, exp, got)
	for _, s := range ex.Features.Subscriptions {
		assert.Emptyf(t, s.Pairs, "generateFuturesSubscriptions should not change the configured %s subscription", s.Channel)
	}
}

func TestValidateFuturesSubscriptions(t *testing.T) {
	t.Parallel()
	valid := subscription.List{
		futuresSubscription(subscription.TickerChannel, wsFuturesFeedTicker, futuresTestPair),
		futuresSubscription(subscription.OrderbookChannel, wsFuturesFeedBook, futuresTestPair),
		futuresSubscription(subscription.AllTradesChannel, wsFuturesFeedTrade, futuresTestPair),
		futuresSubscription(subscription.MyOrdersChannel, wsFuturesFeedOpenOrders),
		futuresSubscription(subscription.MyTradesChannel, wsFuturesFeedFills),
		futuresSubscription(subscription.MyWalletChannel, wsFuturesFeedBalances),
		futuresSubscription(wsFuturesFeedOpenPositions, wsFuturesFeedOpenPositions),
		futuresSubscription(wsFuturesFeedNotifications, wsFuturesFeedNotifications),
		futuresSubscription(subscription.HeartbeatChannel, wsFuturesFeedHeartbeat),
	}
	assert.NoError(t, e.ValidateSubscriptions(valid), "ValidateSubscriptions should accept every feed Kraken streams")
	for _, s := range valid {
		assert.NoErrorf(t, validateFuturesSubscription(s), "validateFuturesSubscription should accept %s", s)
	}
	for _, tc := range []struct {
		s   *subscription.Subscription
		err error
	}{
		{futuresSubscription(subscription.CandlesChannel, subscription.CandlesChannel, futuresTestPair), errFuturesUnsupportedFeed},
		{futuresSubscription(subscription.AllOrdersChannel, subscription.AllOrdersChannel, futuresTestPair), errFuturesUnsupportedFeed},
		{futuresSubscription("ticker_lite", "ticker_lite", futuresTestPair), errFuturesUnsupportedFeed},
		{&subscription.Subscription{Asset: asset.Futures, Channel: subscription.OrderbookChannel, QualifiedChannel: wsFuturesFeedBook, Levels: 10, Pairs: currency.Pairs{futuresTestPair}}, errFuturesOrderbookLevels},
	} {
		assert.ErrorIsf(t, e.ValidateSubscriptions(subscription.List{tc.s}), tc.err, "ValidateSubscriptions should reject %s", tc.s)
		assert.ErrorIsf(t, validateFuturesSubscription(tc.s), tc.err, "validateFuturesSubscription should reject %s", tc.s)
	}
}

func TestFuturesSubscribe(t *testing.T) {
	t.Parallel()
	ex := newTestExchange(t)
	server := new(mockFuturesServer)
	var url string
	if mockTests {
		url = newMockWebsocketURL(t, server.handle)
		useTestCredentials(ex)
	}
	useTestWebsocket(t, ex, url, wsFuturesConnection)
	// Feeds relay to the data handler, which is drained as the engine drains it, keeping what was not handled
	var errs []error
	var unhandled []websocket.UnhandledMessageWarning
	stop, drained := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case <-stop:
				return
			case p := <-ex.Websocket.DataHandler.C:
				switch data := p.Data.(type) {
				case error:
					errs = append(errs, data)
				case websocket.UnhandledMessageWarning:
					unhandled = append(unhandled, data)
				}
			}
		}
	}()
	stopDraining := sync.OnceFunc(func() {
		close(stop)
		<-drained
	})
	t.Cleanup(stopDraining)
	require.NoError(t, ex.Websocket.Connect(t.Context()), "Connect must subscribe without error")
	exp := subscription.List{
		futuresSubscription(subscription.TickerChannel, wsFuturesFeedTicker, futuresTestPair),
		futuresSubscription(subscription.AllTradesChannel, wsFuturesFeedTrade, futuresTestPair),
		futuresSubscription(subscription.OrderbookChannel, wsFuturesFeedBook, futuresTestPair),
	}
	// A live run subscribes to the account feeds only when the test config enables authenticated websocket use and holds
	// credentials
	if ex.Websocket.CanUseAuthenticatedEndpoints() && ex.AreCredentialsValid(t.Context()) {
		exp = append(exp, futuresSubscription(subscription.MyOrdersChannel, wsFuturesFeedOpenOrders), futuresSubscription(subscription.MyTradesChannel, wsFuturesFeedFills))
	}
	subs := ex.Websocket.GetSubscriptions()
	testsubs.EqualLists(t, exp, subs.Clone())
	for _, s := range subs {
		assert.Equalf(t, subscription.SubscribedState, s.State(), "%s should be subscribed", s)
	}
	if !mockTests {
		require.Eventually(t, func() bool {
			_, err := ticker.GetTicker(ex.Name, futuresTestPair, asset.Futures)
			return err == nil
		}, 30*time.Second, 100*time.Millisecond, "the ticker feed must store the futures ticker")
		require.Eventually(t, func() bool {
			_, err := ex.Websocket.Orderbook.GetOrderbook(futuresTestPair, asset.Futures)
			return err == nil
		}, 30*time.Second, 100*time.Millisecond, "the book feed must store the futures order book")
	}
	signed := futuresTestSignedChallenge(t)
	if mockTests {
		assertRequests(t, []string{
			`{"event":"subscribe","feed":"ticker","product_ids":["PF_XBTUSD"]}`,
			`{"event":"subscribe","feed":"trade","product_ids":["PF_XBTUSD"]}`,
			`{"event":"subscribe","feed":"book","product_ids":["PF_XBTUSD"]}`,
			`{"event":"challenge","api_key":"test-key"}`,
			`{"event":"subscribe","feed":"open_orders","api_key":"test-key","original_challenge":"` + futuresTestChallenge + `","signed_challenge":"` + signed + `"}`,
			`{"event":"subscribe","feed":"fills","api_key":"test-key","original_challenge":"` + futuresTestChallenge + `","signed_challenge":"` + signed + `"}`,
		}, server.takeRequests())
	}

	conn, err := ex.Websocket.GetConnection(wsFuturesConnection)
	require.NoError(t, err, "GetConnection must not error")
	require.NoError(t, ex.Websocket.UnsubscribeChannels(t.Context(), conn, subs), "UnsubscribeChannels must not error")
	assert.Empty(t, ex.Websocket.GetSubscriptions(), "UnsubscribeChannels should remove every subscription")
	stopDraining()
	assert.Empty(t, errs, "futures messages should be handled without error")
	assert.Empty(t, unhandled, "futures messages should all be handled")
	if mockTests {
		// The challenge issued on the connection is reused
		assertRequests(t, []string{
			`{"event":"unsubscribe","feed":"ticker","product_ids":["PF_XBTUSD"]}`,
			`{"event":"unsubscribe","feed":"trade","product_ids":["PF_XBTUSD"]}`,
			`{"event":"unsubscribe","feed":"book","product_ids":["PF_XBTUSD"]}`,
			`{"event":"unsubscribe","feed":"open_orders","api_key":"test-key","original_challenge":"` + futuresTestChallenge + `","signed_challenge":"` + signed + `"}`,
			`{"event":"unsubscribe","feed":"fills","api_key":"test-key","original_challenge":"` + futuresTestChallenge + `","signed_challenge":"` + signed + `"}`,
		}, server.takeRequests())
	}
}

func TestFuturesSubscribeAlerts(t *testing.T) {
	t.Parallel()
	piETHUSD := currency.NewPairWithDelimiter("PI", "ETHUSD", currency.UnderscoreDelimiter)
	server := &mockFuturesServer{alerts: map[string]string{"PI_XBTUSD": "Couldn't subscribe to invalid product `PI_XBTUSD`"}}
	ex, conn := newFuturesTestExchange(t, server)
	accepted := futuresSubscription(subscription.TickerChannel, wsFuturesFeedTicker, futuresTestPair)
	rejected := futuresSubscription(subscription.TickerChannel, wsFuturesFeedTicker, piXBTUSDPair)
	alsoAccepted := futuresSubscription(subscription.TickerChannel, wsFuturesFeedTicker, piETHUSD)
	err := ex.futuresSubscribe(t.Context(), conn, subscription.List{accepted, rejected, alsoAccepted})
	require.ErrorIs(t, err, errFuturesSubscriptionFailed, "a subscription Kraken alerts must error")
	assert.ErrorContains(t, err, "Couldn't subscribe to invalid product `PI_XBTUSD`", "the error should carry Kraken's alert")
	assertRequests(t, []string{`{"event":"subscribe","feed":"ticker","product_ids":["PF_XBTUSD","PI_XBTUSD","PI_ETHUSD"]}`}, server.takeRequests())
	assert.NotNil(t, ex.Websocket.GetSubscription(accepted), "the subscription acknowledged before the alert should be stored")
	assert.Nil(t, ex.Websocket.GetSubscription(rejected), "the alerted subscription should not be stored")
	assert.NotNil(t, ex.Websocket.GetSubscription(alsoAccepted), "the subscription acknowledged after the alert should be stored")

	// A request whose subscription is already as asked changes nothing, so it succeeds
	server.m.Lock()
	server.alerts = map[string]string{"PI_XBTUSD": wsFuturesAlreadySubscribed}
	server.m.Unlock()
	require.NoError(t, ex.futuresSubscribe(t.Context(), conn, subscription.List{rejected}), "futuresSubscribe must not error for a subscription Kraken already holds")
	assert.NotNil(t, ex.Websocket.GetSubscription(rejected), "a subscription Kraken already holds should be stored")
	server.m.Lock()
	server.alerts = map[string]string{"PI_XBTUSD": wsFuturesNotSubscribed}
	server.m.Unlock()
	require.NoError(t, ex.futuresUnsubscribe(t.Context(), conn, subscription.List{rejected}), "futuresUnsubscribe must not error for a subscription Kraken does not hold")
	assert.Nil(t, ex.Websocket.GetSubscription(rejected), "a subscription Kraken does not hold should be removed")

	server.m.Lock()
	server.alerts = map[string]string{"PF_XBTUSD": "Unsubscribe failed"}
	server.m.Unlock()
	err = ex.futuresUnsubscribe(t.Context(), conn, subscription.List{accepted})
	assert.ErrorIs(t, err, errFuturesSubscriptionFailed, "an unsubscription Kraken alerts should error")
	assert.NotNil(t, ex.Websocket.GetSubscription(accepted), "a subscription Kraken did not unsubscribe should be kept")
}

func TestFuturesSubscribeUnexpectedReply(t *testing.T) {
	t.Parallel()
	ex, conn := newFuturesTestExchange(t, &mockFuturesServer{wrongProduct: true})
	s := futuresSubscription(subscription.TickerChannel, wsFuturesFeedTicker, futuresTestPair)
	err := ex.futuresSubscribe(t.Context(), conn, subscription.List{s})
	assert.ErrorIs(t, err, errFuturesUnexpectedReply, "an acknowledgement of a product not requested should error")
	assert.Nil(t, ex.Websocket.GetSubscription(s), "the subscription should not be stored")
}

func TestFuturesSubscribeChallenge(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, reply string
		err         error
	}{
		{"refused", `{"event":"error","message":"Json Error"}`, errFuturesSubscriptionFailed},
		{"empty", `{"event":"challenge","message":""}`, errFuturesChallengeEmpty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := &mockFuturesServer{challengeReply: tc.reply}
			ex, conn := newFuturesTestExchange(t, server)
			s := futuresSubscription(subscription.MyOrdersChannel, wsFuturesFeedOpenOrders)
			err := ex.futuresSubscribe(t.Context(), conn, subscription.List{s})
			assert.ErrorIs(t, err, tc.err, "a challenge Kraken does not issue should fail the subscription")
			assertRequests(t, []string{`{"event":"challenge","api_key":"test-key"}`}, server.takeRequests())
			assert.Nil(t, ex.Websocket.GetSubscription(s), "the subscription should not be stored")
		})
	}
}

func TestWsFuturesHandleData(t *testing.T) {
	t.Parallel()
	ex := newTestExchange(t)
	seedTestAssetNames(ex)
	useTestCredentials(ex)
	ex.Features.Enabled.TradeFeed = true
	ex.Websocket.Trade.Setup(true, ex.Websocket.DataHandler)
	ex.Features.Enabled.FillsFeed = true
	ex.Websocket.Fills.Setup(true, ex.Websocket.DataHandler)
	conn, err := ex.Websocket.CreateTestConnection(wsFuturesConnection)
	require.NoError(t, err, "CreateTestConnection must not error")
	testexch.FixtureToDataHandler(t, "testdata/wsFutures.json", func(ctx context.Context, b []byte) error {
		return ex.wsFuturesHandleData(ctx, conn, b)
	})
	got := relayed(ex)
	require.Len(t, got, 24, "wsFuturesHandleData must relay every payload")

	expTicker := &ticker.Price{
		Last:                82382,
		Bid:                 82380,
		BidSize:             0.0003,
		Ask:                 82381,
		AskSize:             0.4671,
		BaseVolume:          7779.8361,
		QuoteVolume:         636625710.8256,
		Open:                82634,
		High:                83262,
		Low:                 80320,
		PercentChange24Hour: -0.3,
		OpenInterest:        2370.562,
		MarkPrice:           82380.4640336367,
		IndexPrice:          82367.95,
		Pair:                futuresTestPair,
		ExchangeName:        t.Name(),
		AssetType:           asset.Futures,
		LastUpdated:         time.UnixMilli(1791518715420),
	}
	assert.Equal(t, expTicker, got[0], "the ticker should be relayed")
	stored, err := ticker.GetTicker(t.Name(), futuresTestPair, asset.Futures)
	require.NoError(t, err, "GetTicker must not error")
	assert.Equal(t, expTicker, stored, "the ticker should be stored as it was relayed")

	for i := 1; i < 5; i++ {
		assert.IsTypef(t, &orderbook.Depth{}, got[i], "payload %d should be the order book", i)
	}
	book, err := ex.Websocket.Orderbook.GetOrderbook(futuresTestPair, asset.Futures)
	require.NoError(t, err, "GetOrderbook must not error")
	// The updates add bids below the snapshot's
	assert.Equal(t, orderbook.Levels{{Price: 82380, Amount: 0.0003}, {Price: 82379, Amount: 0.0012}, {Price: 82378, Amount: 0.0003}, {Price: 82343, Amount: 0.1228}, {Price: 81529, Amount: 1.5296}, {Price: 81235, Amount: 0.0031}}, book.Bids, "the bids should have the updates applied")
	assert.Equal(t, orderbook.Levels{{Price: 82381, Amount: 0.6051}, {Price: 82382, Amount: 0.4345}, {Price: 82383, Amount: 0.0243}}, book.Asks, "the asks should be the snapshot's")
	assert.Equal(t, time.UnixMilli(1791518718754), book.LastUpdated, "the book should be as of the last update")
	lastUpdateID, err := ex.Websocket.Orderbook.LastUpdateID(futuresTestPair, asset.Futures)
	require.NoError(t, err, "LastUpdateID must not error")
	assert.Equal(t, int64(173877394), lastUpdateID, "the book should follow the last update's sequence number")

	futuresTrade := func(uid string, price, amount float64, ms int64) trade.Data {
		return trade.Data{Exchange: t.Name(), CurrencyPair: futuresTestPair, AssetType: asset.Futures, TID: uid, Side: order.Sell, Price: price, Amount: amount, Timestamp: time.UnixMilli(ms)}
	}
	assert.Equal(t, []trade.Data{
		futuresTrade("8fc9d2b3-f541-4e2b-8e0c-11559e0cd62b", 82382, 0.0002, 1791518714883),
		futuresTrade("d4d20435-6841-4f2d-8df9-0cd6a4198172", 82382, 0.0001, 1791518714883),
	}, got[5], "the trade snapshot should be relayed")
	assert.Equal(t, []trade.Data{futuresTrade("4fba15f2-e9e2-4811-8016-2c8b5d1e4c6e", 82380, 0.0002, 1791518719257)}, got[6], "the trade should be relayed")

	snapshotTime := time.UnixMilli(1612275024153)
	expOrders := []any{
		&order.Detail{
			Exchange:        t.Name(),
			AssetType:       asset.Futures,
			OrderID:         "723ba95f-13b7-418b-8fcf-ab7ba6620555",
			Pair:            futuresTestPair,
			Side:            order.Sell,
			Type:            order.StopLimit,
			Status:          order.Open,
			Price:           34900,
			TriggerPrice:    13789,
			Amount:          1000,
			RemainingAmount: 1000,
			Date:            snapshotTime,
			LastUpdated:     snapshotTime,
		},
		&order.Detail{
			Exchange:        t.Name(),
			AssetType:       asset.Futures,
			OrderID:         "59302619-41d2-4f0b-941f-7e7914760ad3",
			Pair:            futuresTestPair,
			Side:            order.Sell,
			Type:            order.TrailingStop,
			Status:          order.Open,
			TriggerPrice:    3200.1,
			Amount:          12,
			RemainingAmount: 12,
			Date:            snapshotTime,
			LastUpdated:     time.UnixMilli(1612275178153),
		},
		&order.Detail{
			Exchange:        t.Name(),
			AssetType:       asset.Futures,
			OrderID:         "7a2f793e-26f3-4987-a938-56d296a11560",
			Pair:            futuresTestPair,
			Side:            order.Sell,
			Type:            order.Limit,
			Status:          order.Open,
			Price:           35058,
			Amount:          1000,
			RemainingAmount: 1000,
			Date:            time.UnixMilli(1612275209430),
			LastUpdated:     time.UnixMilli(1612275209430),
		},
		&order.Detail{
			Exchange:        t.Name(),
			AssetType:       asset.Futures,
			OrderID:         "59302619-41d2-4f0b-941f-7e7914760ad3",
			Pair:            futuresTestPair,
			Side:            order.Sell,
			Type:            order.Limit,
			Status:          order.Open,
			ReduceOnly:      true,
			Price:           10640,
			Amount:          304,
			RemainingAmount: 304,
			Date:            time.UnixMilli(1567702877410),
			LastUpdated:     time.UnixMilli(1567702877410),
		},
		// A cancelled order is named by its ID alone
		&order.Detail{Exchange: t.Name(), AssetType: asset.Futures, OrderID: "660c6b23-8007-48c1-a7c9-4893f4572e8c", Status: order.Cancelled},
	}
	for i := range expOrders {
		assert.Equalf(t, expOrders[i], got[7+i], "open orders payload %d should be relayed", i)
	}

	piETHUSD := currency.NewPairWithDelimiter("PI", "ETHUSD", currency.UnderscoreDelimiter)
	makerTime, takerTime := time.UnixMilli(1600256910739), time.UnixMilli(1600256945531)
	expFills := []any{
		&order.Detail{
			Exchange:    t.Name(),
			AssetType:   asset.Futures,
			OrderID:     "9e30258b-5a98-4002-968a-5b0e149bcfbf",
			Pair:        futuresTestPair,
			Side:        order.Buy,
			Type:        order.Limit,
			LastUpdated: makerTime,
			Trades: []order.TradeHistory{{
				TID:       "cad76f07-814e-4dc6-8478-7867407b6bff",
				Price:     10937.5,
				Amount:    5000,
				Fee:       -0.00009142857,
				FeeAsset:  "XBT",
				Exchange:  t.Name(),
				Type:      order.Limit,
				Side:      order.Buy,
				Timestamp: makerTime,
				IsMaker:   true,
			}},
		},
		&order.Detail{
			Exchange:    t.Name(),
			AssetType:   asset.Futures,
			OrderID:     "7e60b6e8-e4c2-4ce8-bbd0-ef81e18b65bb",
			Pair:        piETHUSD,
			Side:        order.Buy,
			Type:        order.Limit,
			LastUpdated: takerTime,
			Trades: []order.TradeHistory{{
				TID:       "b1aa44b2-4f2a-4031-999c-ae1175c91580",
				Price:     364.65,
				Amount:    5000,
				Fee:       0.00685588921,
				FeeAsset:  "ETH",
				Exchange:  t.Name(),
				Type:      order.Limit,
				Side:      order.Buy,
				Timestamp: takerTime,
			}},
		},
		[]fill.Data{
			{
				ID:           "cad76f07-814e-4dc6-8478-7867407b6bff",
				Timestamp:    makerTime,
				Exchange:     t.Name(),
				AssetType:    asset.Futures,
				CurrencyPair: futuresTestPair,
				Side:         order.Buy,
				OrderID:      "9e30258b-5a98-4002-968a-5b0e149bcfbf",
				TradeID:      "cad76f07-814e-4dc6-8478-7867407b6bff",
				Price:        10937.5,
				Amount:       5000,
			},
			{
				ID:           "b1aa44b2-4f2a-4031-999c-ae1175c91580",
				Timestamp:    takerTime,
				Exchange:     t.Name(),
				AssetType:    asset.Futures,
				CurrencyPair: piETHUSD,
				Side:         order.Buy,
				OrderID:      "7e60b6e8-e4c2-4ce8-bbd0-ef81e18b65bb",
				TradeID:      "b1aa44b2-4f2a-4031-999c-ae1175c91580",
				Price:        364.65,
				Amount:       5000,
			},
		},
	}
	for i := range expFills {
		assert.Equalf(t, expFills[i], got[12+i], "fills payload %d should be relayed", i)
	}

	// The balances snapshot stores and relays the cash and multi-collateral wallets, then relays the message
	cash := accounts.CurrencyBalances{}
	for code, amount := range map[string]float64{
		"USDT": 4997.5012493753, "XBT": 0.1285407184, "ETH": 1.8714395862, "LTC": 47.6462740614, "GBP": 3733.488646461,
		"USDC": 5001.00020004, "USD": 5000, "BCH": 16.8924625832, "EUR": 4459.070194683, "XRP": 7065.5399485629,
	} {
		c := currency.NewCode(code)
		cash[c] = accounts.Balance{Currency: c, Total: amount, Free: amount}
	}
	flex := accounts.CurrencyBalances{
		currency.USDT: {Currency: currency.USDT},
		currency.USD:  {Currency: currency.USD},
	}
	assert.Equal(t, accounts.SubAccounts{
		{ID: "cash", AssetType: asset.Futures, Balances: cash},
		{ID: "flex", AssetType: asset.Futures, Balances: flex},
	}, futuresTestSubAccounts(t, got[15]), "the balances snapshot's wallets should be relayed")
	assert.IsType(t, &WsFuturesBalances{}, got[16], "the balances snapshot should be relayed")
	usd := accounts.CurrencyBalances{currency.USD: {Currency: currency.USD, Total: 5000, Free: 5000}}
	assert.Equal(t, accounts.SubAccounts{{ID: "cash", AssetType: asset.Futures, Balances: usd}}, futuresTestSubAccounts(t, got[17]), "the cash wallet update should be relayed")
	assert.IsType(t, &WsFuturesBalances{}, got[18], "the cash wallet update should be relayed")
	assert.Equal(t, accounts.SubAccounts{{ID: "flex", AssetType: asset.Futures, Balances: usd}}, futuresTestSubAccounts(t, got[19]), "the multi-collateral wallet update should be relayed")
	assert.IsType(t, &WsFuturesBalances{}, got[20], "the multi-collateral wallet update should be relayed")
	margin, ok := got[21].(*WsFuturesBalances)
	require.True(t, ok, "a margin account update must be relayed as the message alone")
	assert.Equal(t, map[string]WsFuturesMarginAccount{"F-XBT:USD": {
		Name:           "F-XBT:USD",
		Pair:           "XBT/USD",
		Unit:           "XBT",
		PortfolioValue: 0.1219368845,
		Balance:        0.1219368845,
		Available:      0.1219368845,
	}}, margin.Futures, "the margin account should be relayed")
	for _, tc := range []struct {
		subAccount string
		c          currency.Code
		exp        accounts.Balance
	}{
		{"cash", currency.USD, accounts.Balance{Currency: currency.USD, Total: 5000, Free: 5000}},
		{"cash", currency.XBT, accounts.Balance{Currency: currency.XBT, Total: 0.1285407184, Free: 0.1285407184}},
		{"flex", currency.USD, accounts.Balance{Currency: currency.USD, Total: 5000, Free: 5000}},
	} {
		b, err := ex.Accounts.GetBalance(tc.subAccount, ex.GetDefaultCredentials(), asset.Futures, tc.c)
		require.NoErrorf(t, err, "GetBalance must not error for %s %s", tc.subAccount, tc.c)
		b.UpdatedAt = time.Time{}
		assert.Equalf(t, tc.exp, b, "the %s %s balance should be stored", tc.subAccount, tc.c)
	}

	positions, ok := got[22].(*WsFuturesPositions)
	require.True(t, ok, "the open positions must be relayed")
	assert.Len(t, positions.Positions, 3, "every open position should be relayed")
	notifications, ok := got[23].(*WsFuturesNotifications)
	require.True(t, ok, "the notifications must be relayed")
	assert.Equal(t, []WsFuturesNotification{{ID: 5, Type: "market", Priority: "low", Note: "A note describing the notification.", EffectiveTime: types.Time(time.UnixMilli(1520288300000))}}, notifications.Notifications, "the notification should be relayed")
}

// futuresTestSubAccounts asserts a payload is relayed sub-accounts whose balances are stamped, and clears the stamps so
// they compare with an expected value
func futuresTestSubAccounts(t *testing.T, payload any) accounts.SubAccounts {
	t.Helper()
	subAccts, ok := payload.(accounts.SubAccounts)
	require.True(t, ok, "the payload must be sub-accounts")
	for _, s := range subAccts {
		for c, b := range s.Balances {
			assert.NotZerof(t, b.UpdatedAt, "the %s %s balance should be stamped", s.ID, c)
			b.UpdatedAt = time.Time{}
			s.Balances[c] = b
		}
	}
	return subAccts
}

func TestWsFuturesBookSequenceGap(t *testing.T) {
	t.Parallel()
	server := new(mockFuturesServer)
	ex, conn := newFuturesTestExchange(t, server)
	s := futuresSubscription(subscription.OrderbookChannel, wsFuturesFeedBook, futuresTestPair)
	require.NoError(t, ex.futuresSubscribe(t.Context(), conn, subscription.List{s}), "futuresSubscribe must not error")
	server.takeRequests()
	snapshot := `{"feed":"book_snapshot","product_id":"PF_XBTUSD","timestamp":1791518718655,"seq":173877391,"tickSize":null,"bids":[{"price":82380.0,"qty":0.0003}],"asks":[{"price":82381.0,"qty":0.6051}]}`
	require.NoError(t, ex.wsFuturesHandleData(t.Context(), conn, []byte(snapshot)), "wsFuturesHandleData must load the snapshot")
	gap := `{"feed":"book","product_id":"PF_XBTUSD","side":"buy","seq":173877393,"price":81529.0,"qty":1.5296,"timestamp":1791518718725}`
	require.ErrorIs(t, ex.wsFuturesHandleData(t.Context(), conn, []byte(gap)), errFuturesSequenceGap, "an update out of sequence must error")
	_, err := ex.Websocket.Orderbook.GetOrderbook(futuresTestPair, asset.Futures)
	assert.ErrorIs(t, err, orderbook.ErrOrderbookInvalid, "an update out of sequence should invalidate the book")
	next := `{"feed":"book","product_id":"PF_XBTUSD","side":"buy","seq":173877394,"price":82343.0,"qty":0.1228,"timestamp":1791518718754}`
	assert.NoError(t, ex.wsFuturesHandleData(t.Context(), conn, []byte(next)), "updates to an invalid book should be dropped until a new snapshot")

	exp := []string{
		`{"event":"unsubscribe","feed":"book","product_ids":["PF_XBTUSD"]}`,
		`{"event":"subscribe","feed":"book","product_ids":["PF_XBTUSD"]}`,
	}
	var got []string
	require.Eventually(t, func() bool {
		got = append(got, server.takeRequests()...)
		return len(got) >= len(exp)
	}, 5*time.Second, 10*time.Millisecond, "an update out of sequence must resubscribe the book")
	assert.Equal(t, exp, got, "an update out of sequence should resubscribe the book, unsubscribing first")
	require.Eventually(t, func() bool {
		stored := ex.Websocket.GetSubscription(s)
		return stored != nil && stored.State() == subscription.SubscribedState
	}, 5*time.Second, 10*time.Millisecond, "the book must be subscribed again")
	require.NoError(t, ex.wsFuturesHandleData(t.Context(), conn, []byte(snapshot)), "wsFuturesHandleData must load the new snapshot")
	_, err = ex.Websocket.Orderbook.GetOrderbook(futuresTestPair, asset.Futures)
	assert.NoError(t, err, "a new snapshot should restore the book")
}

func TestWsFuturesUpdateBookResubscribe(t *testing.T) {
	t.Parallel()
	snapshot := `{"feed":"book_snapshot","product_id":"PF_XBTUSD","timestamp":1791518718655,"seq":173877391,"tickSize":null,"bids":[{"price":82380.0,"qty":0.0003}],"asks":[{"price":82381.0,"qty":0.6051}]}`
	setup := func(t *testing.T) (*Exchange, websocket.Connection, *mockFuturesServer, *subscription.Subscription) {
		t.Helper()
		server := new(mockFuturesServer)
		ex, conn := newFuturesTestExchange(t, server)
		s := futuresSubscription(subscription.OrderbookChannel, wsFuturesFeedBook, futuresTestPair)
		require.NoError(t, ex.futuresSubscribe(t.Context(), conn, subscription.List{s}), "futuresSubscribe must not error")
		server.takeRequests()
		require.NoError(t, ex.wsFuturesHandleData(t.Context(), conn, []byte(snapshot)), "wsFuturesHandleData must load the snapshot")
		return ex, conn, server, s
	}

	t.Run("rejected update", func(t *testing.T) {
		t.Parallel()
		ex, conn, server, s := setup(t)
		// The book cannot apply an update without a timestamp
		rejected := `{"feed":"book","product_id":"PF_XBTUSD","side":"buy","seq":173877392,"price":81529.0,"qty":1.5296}`
		err := ex.wsFuturesHandleData(t.Context(), conn, []byte(rejected))
		require.ErrorIs(t, err, orderbook.ErrLastUpdatedNotSet, "an update the book rejects must error")
		exp := []string{
			`{"event":"unsubscribe","feed":"book","product_ids":["PF_XBTUSD"]}`,
			`{"event":"subscribe","feed":"book","product_ids":["PF_XBTUSD"]}`,
		}
		var got []string
		require.Eventually(t, func() bool {
			got = append(got, server.takeRequests()...)
			return len(got) >= len(exp)
		}, 5*time.Second, 10*time.Millisecond, "an update the book rejects must resubscribe the book")
		assert.Equal(t, exp, got, "an update the book rejects should resubscribe the book, unsubscribing first")
		require.Eventually(t, func() bool {
			stored := ex.Websocket.GetSubscription(s)
			return stored != nil && stored.State() == subscription.SubscribedState
		}, 5*time.Second, 10*time.Millisecond, "the book must be subscribed again")
	})

	t.Run("unrelayed update", func(t *testing.T) {
		t.Parallel()
		ex, conn, server, _ := setup(t)
		var err error
		for err == nil {
			err = ex.Websocket.DataHandler.Send(t.Context(), nil)
		}
		update := `{"feed":"book","product_id":"PF_XBTUSD","side":"buy","seq":173877392,"price":81529.0,"qty":1.5296,"timestamp":1791518718725}`
		err = ex.wsFuturesHandleData(t.Context(), conn, []byte(update))
		require.Error(t, err, "an update the data handler cannot relay must error")
		assert.NotErrorIs(t, err, orderbook.ErrOrderbookInvalid, "an update the data handler cannot relay should leave the book valid")
		id, err := ex.Websocket.Orderbook.LastUpdateID(futuresTestPair, asset.Futures)
		require.NoError(t, err, "LastUpdateID must not error for a valid book")
		assert.Equal(t, int64(173877392), id, "the update should be applied although it was not relayed")
		var got []string
		assert.Never(t, func() bool {
			got = append(got, server.takeRequests()...)
			return len(got) != 0
		}, 100*time.Millisecond, 5*time.Millisecond, "an update the data handler cannot relay should not resubscribe the book")
	})
}

func TestWsFuturesHandleDataErrors(t *testing.T) {
	t.Parallel()
	ex := newTestExchange(t)
	conn, err := ex.Websocket.CreateTestConnection(wsFuturesConnection)
	require.NoError(t, err, "CreateTestConnection must not error")
	for _, tc := range []struct {
		name string
		msg  string
		err  error
	}{
		{"unmatched reply", `{"event":"subscribed","feed":"ticker","product_ids":["PF_XBTUSD"]}`, errFuturesUnexpectedReply},
		{"unsolicited alert", `{"event":"alert","message":"Bad request"}`, errFuturesUnexpectedReply},
		{"neither event nor feed", `{"product_id":"PF_XBTUSD"}`, websocket.ErrSignatureNotMatched},
		{"ticker product", `{"feed":"ticker","product_id":"PF_MOONUSD"}`, currency.ErrPairNotFound},
		{"book snapshot product", `{"feed":"book_snapshot","product_id":"PF_MOONUSD","bids":[],"asks":[]}`, currency.ErrPairNotFound},
		{"book update product", `{"feed":"book","product_id":"PF_MOONUSD","side":"buy","seq":1}`, currency.ErrPairNotFound},
		{"book update without a snapshot", `{"feed":"book","product_id":"PF_ETHUSD","side":"buy","seq":1,"price":2500,"qty":1,"timestamp":1791518718707}`, orderbook.ErrDepthNotFound},
		{"trade product", `{"feed":"trade","product_id":"PF_MOONUSD","side":"buy"}`, currency.ErrPairNotFound},
		{"trade side", `{"feed":"trade","product_id":"PF_XBTUSD","side":"up"}`, order.ErrSideIsInvalid},
		{"open order instrument", `{"feed":"open_orders_snapshot","orders":[{"instrument":"PF_MOONUSD","type":"limit"}]}`, currency.ErrPairNotFound},
		{"open order type", `{"feed":"open_orders","order":{"instrument":"PF_XBTUSD","type":"moon"}}`, errUnknownOrderType},
		{"fill instrument", `{"feed":"fills","fills":[{"instrument":"PF_MOONUSD","order_type":"lmt"}]}`, currency.ErrPairNotFound},
		{"fill order type", `{"feed":"fills","fills":[{"instrument":"PF_XBTUSD","order_type":"moon"}]}`, errUnknownOrderType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tcEx := newTestExchange(t)
			tcEx.Features.Enabled.TradeFeed = true
			assert.ErrorIs(t, tcEx.wsFuturesHandleData(t.Context(), conn, []byte(tc.msg)), tc.err, "wsFuturesHandleData should return the expected error")
		})
	}
	ex.Features.Enabled.TradeFeed = true
	for _, feed := range []string{
		wsFuturesFeedTicker, wsFuturesFeedBookSnapshot, wsFuturesFeedBook, wsFuturesFeedTradeSnapshot, wsFuturesFeedTrade,
		wsFuturesFeedOpenOrdersSnapshot, wsFuturesFeedOpenOrders, wsFuturesFeedFillsSnapshot, wsFuturesFeedFills,
		wsFuturesFeedBalancesSnapshot, wsFuturesFeedBalances, wsFuturesFeedOpenPositions, wsFuturesFeedNotifications,
	} {
		msg := `{"feed":"` + feed + `","product_id":7,"order":7,"orders":7,"fills":7,"holding":7,"positions":7,"notifications":7,"trades":7}`
		assert.Errorf(t, ex.wsFuturesHandleData(t.Context(), conn, []byte(msg)), "wsFuturesHandleData should reject a %s message it cannot decode", feed)
	}
}

func TestWsFuturesHandleDataUnhandled(t *testing.T) {
	t.Parallel()
	ex := newTestExchange(t)
	conn, err := ex.Websocket.CreateTestConnection(wsFuturesConnection)
	require.NoError(t, err, "CreateTestConnection must not error")
	for _, msg := range []string{`{"event":"info","version":1}`, `{"feed":"heartbeat","time":1791518718452}`} {
		require.NoErrorf(t, ex.wsFuturesHandleData(t.Context(), conn, []byte(msg)), "wsFuturesHandleData must not error for %s", msg)
	}
	assert.Empty(t, ex.Websocket.DataHandler.C, "the info event and heartbeats should not be relayed")
	require.NoError(t, ex.wsFuturesHandleData(t.Context(), conn, []byte(`{"feed":"account_log","logs":[]}`)), "wsFuturesHandleData must not error for an unhandled feed")
	require.Len(t, ex.Websocket.DataHandler.C, 1, "an unhandled feed's message must be relayed")
	warning, ok := (<-ex.Websocket.DataHandler.C).Data.(websocket.UnhandledMessageWarning)
	require.True(t, ok, "an unhandled feed's message must be relayed as a warning")
	assert.Contains(t, warning.Message, "account_log", "the warning should carry the message")
}

func TestWsFuturesHandleDataTradesDisabled(t *testing.T) {
	t.Parallel()
	ex := newTestExchange(t)
	ex.Features.Enabled.TradeFeed = false
	ex.Features.Enabled.SaveTradeData = false
	require.NoError(t, ex.wsFuturesHandleData(t.Context(), nil, []byte(`{"feed":"trade","product_id":7}`)), "wsFuturesHandleData must not decode trades nobody uses")
	assert.Empty(t, ex.Websocket.DataHandler.C, "trades should not be relayed when the trade feed is disabled")
}

func TestFuturesOrderUpdateStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		reason   string
		isCancel bool
		filled   float64
		exp      order.Status
	}{
		{"new_placed_order_by_user", false, 0, order.Open},
		{"limit_order_from_stop", false, 0, order.Open},
		{"partial_fill", false, 0.25, order.PartiallyFilled},
		{"full_fill", true, 0, order.Filled},
		{"stop_order_triggered", true, 0, order.Closed},
		{"contract_expired", true, 0, order.Expired},
		{"cancelled_by_user", true, 0, order.Cancelled},
		{"cancelled_by_user", true, 0.25, order.PartiallyFilledCancelled},
		{"cancelled_by_admin", true, 0, order.Cancelled},
		{"dead_man_switch", true, 0, order.Cancelled},
		{"liquidation", true, 0, order.Cancelled},
		{"not_enough_margin", true, 0, order.Cancelled},
		{"market_inactive", true, 0, order.Cancelled},
		{"post_order_failed_because_it_would_filled", true, 0, order.Rejected},
		{"ioc_order_failed_because_it_would_not_be_executed", true, 0, order.Rejected},
		{"would_execute_self", true, 0, order.Rejected},
		{"would_not_reduce_position", true, 0, order.Rejected},
		{"order_for_edit_not_found", true, 0, order.Rejected},
	} {
		assert.Equalf(t, tc.exp, futuresOrderUpdateStatus(tc.reason, tc.isCancel, tc.filled), "futuresOrderUpdateStatus should convert %s with %v filled", tc.reason, tc.filled)
	}
}
