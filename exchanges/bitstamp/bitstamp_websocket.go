package bitstamp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/buger/jsonparser"
	gws "github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fill"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	"github.com/thrasher-corp/gocryptotrader/log"
)

const (
	bitstampWSURL = "wss://ws.bitstamp.net" //nolint // gosec false positive
	hbInterval    = 8 * time.Second         // Connection monitor defaults to 10s inactivity

	privateChannelPrefix = "private-"
)

// Websocket channels
const (
	channelLiveTrades         = "live_trades"
	channelLiveOrders         = "live_orders"
	channelOrderbook          = "order_book"
	channelFundingRate        = "funding_rate"
	channelAnnouncements      = "announcements"
	channelMyOrders           = "my_orders"
	channelMyTrades           = "my_trades"
	channelMySettlements      = "my_settlements"
	channelMyLiquidations     = "my_liquidations"
	channelMyTokenSettlements = "my_token_settlements"
)

var (
	errParsingWSPair    = errors.New("unable to parse currency pair from channel")
	errMalformedChannel = errors.New("malformed channel name")
	errWebsocketError   = errors.New("websocket error")
	errOrderIDMissing   = errors.New("order event missing an order ID")

	hbMsg = []byte(`{"event":"bts:heartbeat"}`)
)

var defaultSubscriptions = subscription.List{
	{Enabled: true, Asset: asset.All, Channel: subscription.OrderbookChannel, Interval: kline.HundredMilliseconds},
	{Enabled: true, Asset: asset.All, Channel: subscription.AllTradesChannel},
	{Enabled: true, Asset: asset.PerpetualContract, Channel: channelFundingRate},
	{Enabled: true, Asset: asset.All, Channel: subscription.MyOrdersChannel, Authenticated: true},
	{Enabled: true, Asset: asset.All, Channel: subscription.MyTradesChannel, Authenticated: true},
}

var subscriptionNames = map[string]string{
	subscription.OrderbookChannel: channelOrderbook,
	subscription.AllTradesChannel: channelLiveTrades,
	subscription.MyOrdersChannel:  channelMyOrders,
	subscription.MyTradesChannel:  channelMyTrades,
	channelFundingRate:            channelFundingRate,
	channelAnnouncements:          channelAnnouncements,
	channelMySettlements:          channelMySettlements,
	channelMyLiquidations:         channelMyLiquidations,
	channelMyTokenSettlements:     channelMyTokenSettlements,
}

// accountChannels are channels which are not specific to a market; subscriptions to them should use an empty asset
var accountChannels = []string{channelAnnouncements, channelMySettlements, channelMyLiquidations, channelMyTokenSettlements}

// WsConnect connects to a websocket feed
func (e *Exchange) WsConnect() error {
	if !e.Websocket.IsEnabled() || !e.IsEnabled() {
		return websocket.ErrWebsocketNotEnabled
	}
	ctx := context.TODO()
	var dialer gws.Dialer
	err := e.Websocket.Conn.Dial(ctx, &dialer, http.Header{}, nil)
	if err != nil {
		return err
	}
	if e.Verbose {
		log.Debugf(log.ExchangeSys, "%s Connected to Websocket.\n", e.Name)
	}
	e.Websocket.Conn.SetupPingHandler(request.Unset, websocket.PingHandler{
		MessageType: gws.TextMessage,
		Message:     hbMsg,
		Delay:       hbInterval,
	})

	e.Websocket.Wg.Add(1)
	go e.wsReadData(ctx)

	return nil
}

// wsReadData receives and passes on websocket messages for processing
func (e *Exchange) wsReadData(ctx context.Context) {
	defer e.Websocket.Wg.Done()

	for {
		resp := e.Websocket.Conn.ReadMessage()
		if resp.Raw == nil {
			return
		}
		if err := e.wsHandleData(ctx, resp.Raw); err != nil {
			if errSend := e.Websocket.DataHandler.Send(ctx, err); errSend != nil {
				log.Errorf(log.WebsocketMgr, "%s %s: %s %s", e.Name, e.Websocket.Conn.GetURL(), errSend, err)
			}
		}
	}
}

func (e *Exchange) wsHandleData(ctx context.Context, respRaw []byte) error {
	event, err := jsonparser.GetUnsafeString(respRaw, "event")
	if err != nil {
		return fmt.Errorf("%w `event`: %w", common.ErrParsingWSField, err)
	}

	switch event {
	case "bts:heartbeat":
		return nil
	case "bts:subscription_succeeded", "bts:unsubscription_succeeded":
		return e.handleWSSubscription(event, respRaw)
	case "bts:error":
		return handleWSError(respRaw)
	case "bts:request_reconnect":
		go func() {
			if err := e.Websocket.Shutdown(); err != nil { // Connection monitor will reconnect
				log.Errorf(log.WebsocketMgr, "%s failed to shutdown websocket: %v", e.Name, err)
			}
		}()
		return nil
	}

	channel, err := jsonparser.GetUnsafeString(respRaw, "channel")
	if err != nil {
		return fmt.Errorf("%w `channel`: %w", common.ErrParsingWSField, err)
	}
	name, market, private, err := splitChannel(channel)
	if err != nil {
		return err
	}
	data, _, _, err := jsonparser.Get(respRaw, "data")
	if err != nil {
		return fmt.Errorf("%w `data`: %w", common.ErrParsingWSField, err)
	}

	switch {
	case name == channelAnnouncements && !private:
		return e.handleWSAnnouncement(ctx, event, data)
	case name == channelMySettlements && private:
		return e.handleWSSettlement(ctx, event, data)
	case name == channelMyLiquidations && private:
		var alert WebsocketLiquidationAlert
		if err := json.Unmarshal(data, &alert); err != nil {
			return err
		}
		return e.Websocket.DataHandler.Send(ctx, &alert)
	case name == channelMyTokenSettlements && private:
		var settlement WebsocketTokenSettlement
		if err := json.Unmarshal(data, &settlement); err != nil {
			return err
		}
		return e.Websocket.DataHandler.Send(ctx, &settlement)
	case market == "":
		return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{Message: e.Name + websocket.UnhandledMessage + string(respRaw)})
	}

	pair, a, err := e.marketPair(market)
	if err != nil {
		return err
	}
	switch {
	case name == channelOrderbook && !private:
		return e.handleWSOrderbook(data, pair, a)
	case name == channelLiveTrades && !private:
		return e.handleWSTrade(data, pair, a)
	case name == channelFundingRate && !private:
		return e.handleWSFundingRate(ctx, data, pair, a)
	case name == channelLiveOrders && !private:
		return nil // Public order events are not processed
	case name == channelMyOrders && private:
		return e.handleWSOrder(ctx, event, data, pair, a)
	case name == channelMyTrades && private:
		return e.handleWSMyTrade(data, pair, a)
	}
	return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{Message: e.Name + websocket.UnhandledMessage + string(respRaw)})
}

func (e *Exchange) handleWSSubscription(event string, respRaw []byte) error {
	channel, err := jsonparser.GetUnsafeString(respRaw, "channel")
	if err != nil {
		return fmt.Errorf("%w `channel`: %w", common.ErrParsingWSField, err)
	}
	op := strings.TrimSuffix(strings.TrimPrefix(event, "bts:"), "scription_succeeded")
	return e.Websocket.Match.RequireMatchWithData(op+":"+channel, respRaw)
}

// handleWSError returns the error reported by the server
// Errors do not identify the request or channel which caused them, so failed subscriptions time out
func handleWSError(respRaw []byte) error {
	data, _, _, err := jsonparser.Get(respRaw, "data")
	if err != nil {
		return fmt.Errorf("%w `data`: %w", common.ErrParsingWSField, err)
	}
	var wsErr websocketError
	if err := json.Unmarshal(data, &wsErr); err != nil {
		return err
	}
	if wsErr.Code != 0 {
		return fmt.Errorf("%w %d: %s", errWebsocketError, wsErr.Code, wsErr.Message)
	}
	return fmt.Errorf("%w: %s", errWebsocketError, wsErr.Message)
}

func (e *Exchange) handleWSOrderbook(data []byte, pair currency.Pair, a asset.Item) error {
	var ob websocketOrderbook
	if err := json.Unmarshal(data, &ob); err != nil {
		return err
	}
	book := &orderbook.Book{
		Bids:              ob.Bids.Levels(),
		Asks:              ob.Asks.Levels(),
		Pair:              pair,
		LastUpdated:       ob.Microtimestamp.Time(),
		Asset:             a,
		Exchange:          e.Name,
		ValidateOrderbook: e.ValidateOrderbook,
	}
	filterOrderbookZeroBidPrice(book)
	return e.Websocket.Orderbook.LoadSnapshot(book)
}

func (e *Exchange) handleWSTrade(data []byte, pair currency.Pair, a asset.Item) error {
	saveTradeData := e.IsSaveTradeDataEnabled()
	if !saveTradeData && !e.IsTradeFeedEnabled() {
		return nil
	}
	var t websocketTrade
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	return e.Websocket.Trade.Update(saveTradeData, trade.Data{
		Timestamp:    t.Microtimestamp.Time(),
		CurrencyPair: pair,
		AssetType:    a,
		Exchange:     e.Name,
		Price:        t.Price.Float64(),
		Amount:       t.Amount.Float64(),
		Side:         t.Side.Side(),
		TID:          strconv.FormatUint(t.ID, 10),
	})
}

func (e *Exchange) handleWSFundingRate(ctx context.Context, data []byte, pair currency.Pair, a asset.Item) error {
	var rate websocketFundingRate
	if err := json.Unmarshal(data, &rate); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, websocket.FundingData{
		Timestamp:    rate.Timestamp.Time(),
		CurrencyPair: pair,
		AssetType:    a,
		Exchange:     e.Name,
		Rate:         rate.FundingRate.Float64(),
	})
}

func (e *Exchange) handleWSOrder(ctx context.Context, event string, data []byte, pair currency.Pair, a asset.Item) error {
	var o websocketOrder
	if err := json.Unmarshal(data, &o); err != nil {
		return err
	}
	if o.ID == 0 && o.ClientOrderID == "" {
		return fmt.Errorf("%w: %s", errOrderIDMissing, data)
	}

	amount := o.AmountAtCreate.Float64()
	remaining := o.Amount.Float64()
	// AmountTraded only holds the amount executed by this event, so the total executed amount is derived instead
	executed := amount - remaining

	var status order.Status
	switch event {
	case "order_created", "order_replaced":
		status = order.New
	case "order_changed":
		status = order.Open
		if executed > 0 {
			status = order.PartiallyFilled
		}
	case "order_deleted":
		status = order.Cancelled
		if remaining == 0 && amount > 0 {
			status = order.Filled
		}
	case "stop_active":
		status = order.Active
	case "stop_inactive":
		status = order.Cancelled
	default:
		return e.Websocket.DataHandler.Send(ctx, websocket.UnhandledMessageWarning{Message: e.Name + websocket.UnhandledMessage + event + " " + string(data)})
	}

	orderType, tif := orderTypeFromSubtypeID(o.OrderSubtype)
	details := []order.Detail{{
		Price:           o.Price.Float64(),
		Amount:          amount,
		RemainingAmount: remaining,
		ExecutedAmount:  executed,
		Exchange:        e.Name,
		OrderID:         strconv.FormatUint(o.ID, 10),
		ClientOrderID:   o.ClientOrderID,
		Type:            orderType,
		TimeInForce:     tif,
		Side:            o.Side.Side(),
		Status:          status,
		AssetType:       a,
		Date:            o.Microtimestamp.Time(),
		Pair:            pair,
		ReduceOnly:      o.ReduceOnly,
		TriggerPrice:    o.StopPrice.Float64(),
	}}
	if event == "order_replaced" && o.OriginalOrderID != 0 {
		// A replacement order has a new ID, so the replaced order is reported as cancelled
		details = append(details, order.Detail{
			Exchange:    e.Name,
			OrderID:     strconv.FormatUint(o.OriginalOrderID, 10),
			Status:      order.Cancelled,
			AssetType:   a,
			Pair:        pair,
			LastUpdated: o.Microtimestamp.Time(),
		})
	}
	for i := range details {
		if err := e.Websocket.DataHandler.Send(ctx, &details[i]); err != nil {
			return err
		}
	}
	return nil
}

// orderTypeFromSubtypeID returns the order type and time in force of a websocket order subtype
func orderTypeFromSubtypeID(subtype uint8) (order.Type, order.TimeInForce) {
	switch subtype {
	case 0:
		return order.Limit, order.GoodTillCancel
	case 1, 2, 7:
		return order.Market, order.UnknownTIF
	case 3:
		return order.Limit, order.GoodTillDay
	case 4:
		return order.Limit, order.ImmediateOrCancel
	case 5:
		return order.Limit, order.PostOnly
	case 6:
		return order.Limit, order.FillOrKill
	case 8:
		return order.Limit, order.GoodTillTime
	case 20:
		return order.StopMarket, order.UnknownTIF
	case 21:
		return order.TakeProfitMarket, order.UnknownTIF
	case 22:
		return order.StopLimit, order.UnknownTIF
	case 23:
		return order.TakeProfit | order.Limit, order.UnknownTIF
	case 24, 25:
		return order.TrailingStop, order.UnknownTIF
	case 26, 27:
		return order.TrailingStopLimit, order.UnknownTIF
	default:
		return order.UnknownType, order.UnknownTIF
	}
}

func (e *Exchange) handleWSMyTrade(data []byte, pair currency.Pair, a asset.Item) error {
	if !e.IsFillsFeedEnabled() {
		return nil
	}
	var t websocketMyTrade
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	side, err := order.StringToOrderSide(t.Side)
	if err != nil {
		return err
	}
	return e.Websocket.Fills.Update(fill.Data{
		Timestamp:     t.Microtimestamp.Time(),
		Exchange:      e.Name,
		AssetType:     a,
		CurrencyPair:  pair,
		Side:          side,
		OrderID:       strconv.FormatUint(t.OrderID, 10),
		ClientOrderID: t.ClientOrderID,
		TradeID:       strconv.FormatUint(t.ID, 10),
		Price:         t.Price.Float64(),
		Amount:        t.Amount.Float64(),
	})
}

func (e *Exchange) handleWSSettlement(ctx context.Context, event string, data []byte) error {
	settlement := &WebsocketSettlement{Event: event}
	if err := json.Unmarshal(data, settlement); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, settlement)
}

func (e *Exchange) handleWSAnnouncement(ctx context.Context, event string, data []byte) error {
	announcement := &WebsocketAnnouncement{Event: event}
	if err := json.Unmarshal(data, announcement); err != nil {
		return err
	}
	return e.Websocket.DataHandler.Send(ctx, announcement)
}

func (e *Exchange) generateSubscriptions() (subscription.List, error) {
	return e.Features.Subscriptions.ExpandTemplates(e)
}

// GetSubscriptionTemplate returns a subscription channel template
func (e *Exchange) GetSubscriptionTemplate(_ *subscription.Subscription) (*template.Template, error) {
	return template.New("master.tmpl").Funcs(template.FuncMap{
		"channelName":      channelName,
		"isAccountChannel": isAccountChannel,
	}).Parse(subTplText)
}

// Subscribe sends a websocket message to receive data from a list of channels
func (e *Exchange) Subscribe(subs subscription.List) error {
	ctx := context.TODO()
	return e.manageSubsWithCreds(ctx, subs, "sub")
}

// Unsubscribe sends a websocket message to stop receiving data from a list of channels
func (e *Exchange) Unsubscribe(subs subscription.List) error {
	ctx := context.TODO()
	return e.manageSubsWithCreds(ctx, subs, "unsub")
}

func (e *Exchange) manageSubsWithCreds(ctx context.Context, subs subscription.List, op string) error {
	var errs error
	var creds *WebsocketTokenResponse
	if authed := subs.Private(); len(authed) > 0 {
		creds, errs = e.GetWebsocketToken(ctx)
	}
	return common.AppendError(errs, e.ParallelChanOp(ctx, subs, func(ctx context.Context, s subscription.List) error { return e.manageSubs(ctx, s, op, creds) }, 1))
}

func (e *Exchange) manageSubs(ctx context.Context, subs subscription.List, op string, creds *WebsocketTokenResponse) error {
	subs, errs := subs.ExpandTemplates(e)
	for _, s := range subs {
		req := websocketEventRequest{
			Event: "bts:" + op + "scribe",
			Data: websocketData{
				Channel: s.QualifiedChannel,
			},
		}
		if s.Authenticated {
			if creds == nil {
				return request.ErrAuthRequestFailed
			}
			req.Data.Channel = privateChannelPrefix + req.Data.Channel + "-" + strconv.FormatUint(creds.UserID, 10)
			req.Data.Auth = creds.Token
		}
		_, err := e.Websocket.Conn.SendMessageReturnResponse(ctx, request.Unset, op+":"+req.Data.Channel, req)
		if err == nil {
			if op == "sub" {
				err = e.Websocket.AddSuccessfulSubscriptions(e.Websocket.Conn, s)
			} else {
				err = e.Websocket.RemoveSubscriptions(e.Websocket.Conn, s)
			}
		}
		if err != nil {
			errs = common.AppendError(errs, err)
		}
	}

	return errs
}

// splitChannel returns the name and market of a channel, and whether it is private
// Private channels are suffixed with the user ID, and market symbols never contain an underscore
func splitChannel(channel string) (name, market string, private bool, err error) {
	if after, ok := strings.CutPrefix(channel, privateChannelPrefix); ok {
		idx := strings.LastIndexByte(after, '-')
		if idx <= 0 {
			return "", "", false, fmt.Errorf("%w: %q", errMalformedChannel, channel)
		}
		if _, err := strconv.ParseUint(after[idx+1:], 10, 64); err != nil {
			return "", "", false, fmt.Errorf("%w: %q", errMalformedChannel, channel)
		}
		channel, private = after[:idx], true
	}
	if isAccountChannelName(channel) {
		return channel, "", private, nil
	}
	idx := strings.LastIndexByte(channel, '_')
	if idx <= 0 || idx == len(channel)-1 {
		return "", "", false, fmt.Errorf("%w: %q", errMalformedChannel, channel)
	}
	return channel[:idx], channel[idx+1:], private, nil
}

// marketPair returns the currency pair and asset type of a channel market symbol
func (e *Exchange) marketPair(market string) (currency.Pair, asset.Item, error) {
	a := asset.Spot
	if strings.HasSuffix(market, strings.ToLower(perpetualMarketSuffix)) {
		a = asset.PerpetualContract
	}
	pair, err := e.MatchSymbolWithAvailablePairs(market, a, false)
	if err != nil {
		return currency.EMPTYPAIR, asset.Empty, fmt.Errorf("%w %q: %w", errParsingWSPair, market, err)
	}
	return pair, a, nil
}

// channelName converts global channel names to exchange specific ones
// panics if name is not supported, so should be called within a recover chain
func channelName(s *subscription.Subscription) string {
	if name, ok := subscriptionNames[s.Channel]; ok {
		return name
	}
	panic(fmt.Errorf("%w: %s", subscription.ErrNotSupported, s.Channel))
}

// isAccountChannel returns whether a subscription is to a channel which is not specific to a market
func isAccountChannel(s *subscription.Subscription) bool {
	return isAccountChannelName(subscriptionNames[s.Channel])
}

func isAccountChannelName(name string) bool {
	return slices.Contains(accountChannels, name)
}

const subTplText = `
{{- if isAccountChannel $.S }}
	{{- range $asset, $pairs := $.AssetPairs }}
		{{- channelName $.S }}
		{{- $.AssetSeparator }}
	{{- end }}
{{- else }}
	{{- range $asset, $pairs := $.AssetPairs }}
		{{- with $name := channelName $.S }}
			{{- range $p := $pairs -}}
				{{- $name -}} _ {{- $p -}}
				{{ $.PairSeparator }}
			{{- end -}}
		{{- end }}
		{{ $.AssetSeparator }}
	{{- end }}
{{- end }}
`
