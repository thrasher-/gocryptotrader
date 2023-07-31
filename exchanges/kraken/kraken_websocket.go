package kraken

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/convert"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/stream"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// List of all websocket channels to subscribe to
const (
	krakenWSURL              = "wss://ws.kraken.com"
	krakenAuthWSURL          = "wss://ws-auth.kraken.com"
	krakenWSSandboxURL       = "wss://sandbox.kraken.com"
	krakenWSSupportedVersion = "1.4.0"
	// WS endpoints
	krakenWsHeartbeat            = "heartbeat"
	krakenWsSystemStatus         = "systemStatus"
	krakenWsSubscribe            = "subscribe"
	krakenWsSubscriptionStatus   = "subscriptionStatus"
	krakenWsUnsubscribe          = "unsubscribe"
	krakenWsTicker               = "ticker"
	krakenWsOHLC                 = "ohlc"
	krakenWsTrade                = "trade"
	krakenWsSpread               = "spread"
	krakenWsOrderbook            = "book"
	krakenWsOwnTrades            = "ownTrades"
	krakenWsOpenOrders           = "openOrders"
	krakenWsAddOrder             = "addOrder"
	krakenWsCancelOrder          = "cancelOrder"
	krakenWsCancelAll            = "cancelAll"
	krakenWsAddOrderStatus       = "addOrderStatus"
	krakenWsCancelOrderStatus    = "cancelOrderStatus"
	krakenWsCancelAllOrderStatus = "cancelAllStatus"
	krakenWsRateLimit            = 50
	krakenWsPingDelay            = time.Second * 27
	krakenWsOrderbookDepth       = 1000
)

// variables for websocket functionality
var (
	subscriptionChannelPair     = make(map[int64]*WebsocketChannelData)
	authToken                   string
	pingRequest                 = WebsocketBaseEventRequest{Event: stream.Ping}
	subscriptionChannelPairMtx  sync.RWMutex
	errNoWebsocketOrderbookData = errors.New("no websocket orderbook data")
)

// Channels require a topic and a currency
// Format [[ticker,but-t4u],[orderbook,nce-btt]]
var defaultSubscribedChannels = []string{
	krakenWsTicker,
	krakenWsTrade,
	krakenWsOrderbook,
	krakenWsOHLC,
	krakenWsSpread}
var authenticatedChannels = []string{krakenWsOwnTrades, krakenWsOpenOrders}

var cancelOrdersStatusMutex sync.Mutex
var cancelOrdersStatus = make(map[int64]*struct {
	Total        int    // total count of orders in wsCancelOrders request
	Successful   int    // numbers of Successfully canceled orders in wsCancelOrders request
	Unsuccessful int    // numbers of Unsuccessfully canceled orders in wsCancelOrders request
	Error        string // if at least one of requested order return fail, store error here
})

// WsConnect initiates a websocket connection
func (k *Kraken) WsConnect() error {
	if !k.Websocket.IsEnabled() || !k.IsEnabled() {
		return errors.New(stream.WebsocketNotEnabled)
	}

	var dialer websocket.Dialer
	err := k.Websocket.Conn.Dial(&dialer, http.Header{})
	if err != nil {
		return err
	}

	comms := make(chan stream.Response)
	subscriptionChannelPair = make(map[int64]*WebsocketChannelData)
	k.Websocket.Wg.Add(2)
	go k.wsReadData(comms)
	go k.wsFunnelConnectionData(k.Websocket.Conn, comms)

	if k.IsWebsocketAuthenticationSupported() {
		authToken, err = k.GetWebsocketToken(context.TODO())
		if err != nil {
			k.Websocket.SetCanUseAuthenticatedEndpoints(false)
			log.Errorf(log.ExchangeSys,
				"%v - authentication failed: %v\n",
				k.Name,
				err)
		} else {
			err = k.Websocket.AuthConn.Dial(&dialer, http.Header{})
			if err != nil {
				k.Websocket.SetCanUseAuthenticatedEndpoints(false)
				log.Errorf(log.ExchangeSys,
					"%v - failed to connect to authenticated endpoint: %v\n",
					k.Name,
					err)
			} else {
				k.Websocket.Wg.Add(1)
				go k.wsFunnelConnectionData(k.Websocket.AuthConn, comms)
				err = k.wsAuthPingHandler()
				if err != nil {
					log.Errorf(log.ExchangeSys,
						"%v - failed setup ping handler for auth connection. Websocket may disconnect unexpectedly. %v\n",
						k.Name,
						err)
				}
			}
		}
	}

	err = k.wsPingHandler()
	if err != nil {
		log.Errorf(log.ExchangeSys,
			"%v - failed setup ping handler. Websocket may disconnect unexpectedly. %v\n",
			k.Name,
			err)
	}
	return nil
}

// wsFunnelConnectionData funnels both auth and public ws data into one manageable place
func (k *Kraken) wsFunnelConnectionData(ws stream.Connection, comms chan stream.Response) {
	defer k.Websocket.Wg.Done()
	for {
		resp := ws.ReadMessage()
		if resp.Raw == nil {
			return
		}
		comms <- resp
	}
}

// wsReadData receives and passes on websocket messages for processing
func (k *Kraken) wsReadData(comms chan stream.Response) {
	defer k.Websocket.Wg.Done()

	for {
		select {
		case <-k.Websocket.ShutdownC:
			select {
			case resp := <-comms:
				err := k.wsHandleData(resp.Raw)
				if err != nil {
					select {
					case k.Websocket.DataHandler <- err:
					default:
						log.Errorf(log.WebsocketMgr,
							"%s websocket handle data error: %v",
							k.Name,
							err)
					}
				}
			default:
			}
			return
		case resp := <-comms:
			err := k.wsHandleData(resp.Raw)
			if err != nil {
				k.Websocket.DataHandler <- fmt.Errorf("%s - unhandled websocket data: %v",
					k.Name,
					err)
			}
		}
	}
}

// awaitForCancelOrderResponses used to wait until all responses will received for appropriate CancelOrder request
// success param = was the response from Kraken successful or not
func isAwaitingCancelOrderResponses(requestID int64, success bool) bool {
	cancelOrdersStatusMutex.Lock()
	if stat, ok := cancelOrdersStatus[requestID]; ok {
		if success {
			cancelOrdersStatus[requestID].Successful++
		} else {
			cancelOrdersStatus[requestID].Unsuccessful++
		}

		if stat.Successful+stat.Unsuccessful != stat.Total {
			cancelOrdersStatusMutex.Unlock()
			return true
		}
	}
	cancelOrdersStatusMutex.Unlock()
	return false
}

func (k *Kraken) wsHandleData(respRaw []byte) error {
	if len(respRaw) == 0 {
		return nil
	}

	if respRaw[0] == '[' {
		var dataResponse []json.RawMessage
		if err := json.Unmarshal(respRaw, &dataResponse); err != nil {
			return err
		}

		if dataResponse[0][0] != '[' {
			// data response
			return k.wsReadDataResponse(dataResponse)
		}

		var channel string
		if err := json.Unmarshal(dataResponse[1], &channel); err != nil {
			return err
		}

		return k.wsHandleAuthDataResponse(channel, dataResponse[0])
	}

	var eventResp wsEvent
	if err := json.Unmarshal(respRaw, &eventResp); err != nil {
		return fmt.Errorf("%s - err %s could not parse websocket data: %s",
			k.Name,
			err,
			respRaw)
	}
	switch eventResp.Event {
	case stream.Pong, krakenWsHeartbeat:
		return nil
	case krakenWsCancelOrderStatus:
		var status WsCancelOrderResponse
		err := json.Unmarshal(respRaw, &status)
		if err != nil {
			return fmt.Errorf("%s - err %s unable to parse WsCancelOrderResponse: %s",
				k.Name,
				err,
				respRaw)
		}

		success := true
		if status.Status == "error" {
			success = false
			cancelOrdersStatusMutex.Lock()
			if _, ok := cancelOrdersStatus[status.RequestID]; ok {
				if cancelOrdersStatus[status.RequestID].Error == "" { // save the first error, if any
					cancelOrdersStatus[status.RequestID].Error = status.ErrorMessage
				}
			}
			cancelOrdersStatusMutex.Unlock()
		}

		if isAwaitingCancelOrderResponses(status.RequestID, success) {
			return nil
		}

		// all responses handled, return results stored in cancelOrdersStatus
		if status.RequestID > 0 && !k.Websocket.Match.IncomingWithData(status.RequestID, respRaw) {
			return fmt.Errorf("can't send ws incoming data to Matched channel with RequestID: %d",
				status.RequestID)
		}
	case krakenWsCancelAllOrderStatus:
		var status WsCancelOrderResponse
		err := json.Unmarshal(respRaw, &status)
		if err != nil {
			return fmt.Errorf("%s - err %s unable to parse WsCancelOrderResponse: %s",
				k.Name,
				err,
				respRaw)
		}

		var isChannelExist bool
		if status.RequestID > 0 {
			isChannelExist = k.Websocket.Match.IncomingWithData(status.RequestID, respRaw)
		}

		if status.Status == "error" {
			return fmt.Errorf("%v Websocket status for RequestID %d: '%v'",
				k.Name,
				status.RequestID,
				status.ErrorMessage)
		}

		if !isChannelExist && status.RequestID > 0 {
			return fmt.Errorf("can't send ws incoming data to Matched channel with RequestID: %d",
				status.RequestID)
		}
	case krakenWsSystemStatus:
		var systemStatus wsSystemStatus
		err := json.Unmarshal(respRaw, &systemStatus)
		if err != nil {
			return fmt.Errorf("%s - err %s unable to parse system status response: %s",
				k.Name,
				err,
				respRaw)
		}
		if systemStatus.Status != "online" {
			k.Websocket.DataHandler <- fmt.Errorf("%v Websocket status '%v'",
				k.Name,
				systemStatus.Status)
		}
		if systemStatus.Version > krakenWSSupportedVersion {
			log.Warnf(log.ExchangeSys,
				"%v New version of Websocket API released. Was %v Now %v",
				k.Name,
				krakenWSSupportedVersion,
				systemStatus.Version)
		}
	case krakenWsAddOrderStatus:
		var status WsAddOrderResponse
		err := json.Unmarshal(respRaw, &status)
		if err != nil {
			return fmt.Errorf("%s - err %s unable to parse add order response: %s",
				k.Name,
				err,
				respRaw)
		}

		var isChannelExist bool
		if status.RequestID > 0 {
			isChannelExist = k.Websocket.Match.IncomingWithData(status.RequestID, respRaw)
		}

		if status.Status == "error" {
			return fmt.Errorf("%v Websocket status for RequestID %d: '%v'",
				k.Name,
				status.RequestID,
				status.ErrorMessage)
		}

		k.Websocket.DataHandler <- &order.Detail{
			Exchange: k.Name,
			OrderID:  status.TransactionID,
			Status:   order.New,
		}

		if !isChannelExist && status.RequestID > 0 {
			return fmt.Errorf("can't send ws incoming data to Matched channel with RequestID: %d",
				status.RequestID)
		}
	case krakenWsSubscriptionStatus:
		var sub wsSubscription
		err := json.Unmarshal(respRaw, &sub)
		if err != nil {
			return fmt.Errorf("%s - err %s unable to parse subscription response: %s",
				k.Name,
				err,
				respRaw)
		}
		if sub.Status != "subscribed" && sub.Status != "unsubscribed" {
			return fmt.Errorf("%v %v %v",
				k.Name,
				sub.RequestID,
				sub.ErrorMessage)
		}
		k.addNewSubscriptionChannelData(&sub)
		if sub.RequestID > 0 {
			k.Websocket.Match.IncomingWithData(sub.RequestID, respRaw)
		}
	default:
		k.Websocket.DataHandler <- stream.UnhandledMessageWarning{
			Message: k.Name + stream.UnhandledMessage + string(respRaw),
		}
	}
	return nil
}

// wsPingHandler sends a message "ping" every 27 seconds to maintain the websocket connection
func (k *Kraken) wsPingHandler() error {
	message, err := json.Marshal(pingRequest)
	if err != nil {
		return err
	}
	k.Websocket.Conn.SetupPingHandler(stream.PingHandler{
		Message:     message,
		Delay:       krakenWsPingDelay,
		MessageType: websocket.TextMessage,
	})
	return nil
}

// wsAuthPingHandler sends a message "ping" every 27 seconds to maintain the websocket connection
func (k *Kraken) wsAuthPingHandler() error {
	message, err := json.Marshal(pingRequest)
	if err != nil {
		return err
	}
	k.Websocket.AuthConn.SetupPingHandler(stream.PingHandler{
		Message:     message,
		Delay:       krakenWsPingDelay,
		MessageType: websocket.TextMessage,
	})
	return nil
}

// wsReadDataResponse classifies the WS response and sends to appropriate handler
func (k *Kraken) wsReadDataResponse(response []json.RawMessage) error {
	channelID, err := strconv.ParseInt(string(response[0]), 10, 64)
	if err != nil {
		return err
	}

	channelData, err := getSubscriptionChannelData(channelID)
	if err != nil {
		return err
	}

	switch channelData.Subscription {
	case krakenWsTicker:
		var tempTicker wsTicker
		if err := json.Unmarshal(response[1], &tempTicker); err != nil {
			return err
		}
		return k.wsProcessTickers(channelData, &tempTicker)
	case krakenWsOHLC:
		var ohlcv [9]interface{}
		if err := json.Unmarshal(response[1], &ohlcv); err != nil {
			return err
		}
		return k.wsProcessCandles(channelData, &ohlcv)
	case krakenWsOrderbook:
		if isSnapshot := bytes.Contains(response[1], []byte("as")); isSnapshot {
			var snapshot wsOrderbookSnapshot
			if err := json.Unmarshal(response[1], &snapshot); err != nil {
				return err
			}
			return k.wsProcessOrderBookPartial(channelData, &snapshot)
		}
		var update wsOrderbookUpdate
		if err := json.Unmarshal(response[1], &update); err != nil {
			return err
		}
		if len(response) == 5 {
			var secondPart wsOrderbookUpdate
			if err := json.Unmarshal(response[2], &secondPart); err != nil {
				return err
			}
			update.Bids = make([][4]string, len(secondPart.Bids))
			copy(update.Bids, secondPart.Bids)
			update.Checksum = secondPart.Checksum
		}
		return k.wsProcessOrderBookUpdate(channelData, &update)
	case krakenWsSpread:
		var spread [5]string
		if err := json.Unmarshal(response[1], &spread); err != nil {
			return err
		}
		k.wsProcessSpread(channelData, &spread)
	case krakenWsTrade:
		var tempTrade [][5]string
		if err := json.Unmarshal(response[1], &tempTrade); err != nil {
			return err
		}
		return k.wsProcessTrades(channelData, tempTrade)
	default:
		return fmt.Errorf("%s received unidentified data for subscription %s: %+v",
			k.Name,
			channelData.Subscription,
			response)
	}

	return nil
}

func (k *Kraken) wsHandleAuthDataResponse(channel string, data json.RawMessage) error {
	switch channel {
	case krakenWsOwnTrades:
		return k.wsProcessOwnTrades(data)
	case krakenWsOpenOrders:
		return k.wsProcessOpenOrders(data)
	}
	return fmt.Errorf("%v websocket: Unidentified websocket data received: %v", k.Name, string(data))
}

func (k *Kraken) wsProcessOwnTrades(ownTrades json.RawMessage) error {
	var result []map[string]*WsOwnTrade
	if err := json.Unmarshal(ownTrades, &result); err != nil {
		return err
	}

	if len(result) == 0 {
		return nil
	}

	for key, val := range result[0] {
		oSide, err := order.StringToOrderSide(val.Type)
		if err != nil {
			k.Websocket.DataHandler <- order.ClassificationError{
				Exchange: k.Name,
				OrderID:  key,
				Err:      err,
			}
			continue
		}
		oType, err := order.StringToOrderType(val.OrderType)
		if err != nil {
			k.Websocket.DataHandler <- order.ClassificationError{
				Exchange: k.Name,
				OrderID:  key,
				Err:      err,
			}
			continue
		}
		trade := order.TradeHistory{
			Price:     val.Price,
			Amount:    val.Vol,
			Fee:       val.Fee,
			Exchange:  k.Name,
			TID:       key,
			Type:      oType,
			Side:      oSide,
			Timestamp: convert.TimeFromUnixTimestampDecimal(val.Time),
		}
		k.Websocket.DataHandler <- &order.Detail{
			Exchange: k.Name,
			OrderID:  val.OrderTransactionID,
			Trades:   []order.TradeHistory{trade},
		}
	}
	return nil
}

func (k *Kraken) wsProcessOpenOrders(ownOrders json.RawMessage) error {
	var result []map[string]*WsOpenOrder
	if err := json.Unmarshal(ownOrders, &result); err != nil {
		return err
	}

	if len(result) == 0 {
		return nil
	}

	for key, val := range result[0] {
		oStatus, err := order.StringToOrderStatus(val.Status)
		if err != nil {
			k.Websocket.DataHandler <- order.ClassificationError{
				Exchange: k.Name,
				OrderID:  key,
				Err:      err,
			}
			continue
		}

		if val.Description.Price == 0 {
			k.Websocket.DataHandler <- &order.Detail{
				Exchange: k.Name,
				OrderID:  key,
				Status:   oStatus,
			}
			continue
		}

		oSide, err := order.StringToOrderSide(val.Description.Type)
		if err != nil {
			k.Websocket.DataHandler <- order.ClassificationError{
				Exchange: k.Name,
				OrderID:  key,
				Err:      err,
			}
			continue
		}

		oType, err := order.StringToOrderType(val.Description.OrderType)
		if err != nil {
			k.Websocket.DataHandler <- order.ClassificationError{
				Exchange: k.Name,
				OrderID:  key,
				Err:      err,
			}
			continue
		}

		p, err := currency.NewPairFromString(val.Description.Pair)
		if err != nil {
			k.Websocket.DataHandler <- order.ClassificationError{
				Exchange: k.Name,
				OrderID:  key,
				Err:      err,
			}
			continue
		}

		assetItem, err := k.GetPairAssetType(p)
		if err != nil {
			k.Websocket.DataHandler <- order.ClassificationError{
				Exchange: k.Name,
				OrderID:  key,
				Err:      err,
			}
			continue
		}
		k.Websocket.DataHandler <- &order.Detail{
			Leverage:        0,
			Price:           val.Price,
			Amount:          val.Volume,
			LimitPriceUpper: val.LimitPrice,
			ExecutedAmount:  val.ExecutedVolume,
			RemainingAmount: val.Volume - val.ExecutedVolume,
			Fee:             val.Fee,
			Exchange:        k.Name,
			OrderID:         key,
			Type:            oType,
			Side:            oSide,
			Status:          oStatus,
			AssetType:       assetItem,
			Date:            convert.TimeFromUnixTimestampDecimal(val.OpenTime),
			Pair:            p,
		}
	}
	return nil
}

// addNewSubscriptionChannelData stores channel ids, pairs and subscription types to an array
// allowing correlation between subscriptions and returned data
func (k *Kraken) addNewSubscriptionChannelData(response *wsSubscription) {
	if response.Pair.IsEmpty() {
		k.Websocket.DataHandler <- fmt.Errorf("%s websocket error: pair not found in response", k.Name)
		return
	}

	fPair, err := k.FormatExchangeCurrency(response.Pair, asset.Spot)
	if err != nil {
		log.Errorf(log.ExchangeSys, "%s websocket error: %s", k.Name, err)
		return
	}

	maxDepth := 0
	if strings.HasPrefix(response.ChannelName, "book-") {
		maxDepthStr := response.ChannelName[5:] // get the string after "book-"
		var err error
		if maxDepth, err = strconv.Atoi(maxDepthStr); err != nil {
			log.Errorf(log.ExchangeSys, "%s websocket error: unable to get book depth: %s", k.Name, err)
		}
	}

	subscriptionChannelPairMtx.Lock()
	subscriptionChannelPair[response.ChannelID] = &WebsocketChannelData{
		Subscription: response.Subscription.Name,
		Pair:         fPair,
		ChannelID:    response.ChannelID,
		MaxDepth:     maxDepth,
	}
	subscriptionChannelPairMtx.Unlock()
}

// getSubscriptionChannelData retrieves WebsocketChannelData based on response ID
func getSubscriptionChannelData(id int64) (*WebsocketChannelData, error) {
	subscriptionChannelPairMtx.RLock()
	data, ok := subscriptionChannelPair[id]
	subscriptionChannelPairMtx.RUnlock()
	if !ok {
		return nil, fmt.Errorf("could not get subscription data for id %d", id)
	}
	return data, nil
}

// wsProcessTickers converts ticker data and sends it to the datahandler
func (k *Kraken) wsProcessTickers(channelData *WebsocketChannelData, data *wsTicker) error {
	closePrice, err := strconv.ParseFloat(data.Close[0], 64)
	if err != nil {
		return err
	}
	openPrice, err := strconv.ParseFloat(data.Open[0], 64)
	if err != nil {
		return err
	}
	highPrice, err := strconv.ParseFloat(data.High[0], 64)
	if err != nil {
		return err
	}
	lowPrice, err := strconv.ParseFloat(data.Low[0], 64)
	if err != nil {
		return err
	}
	quantity, err := strconv.ParseFloat(data.Volume[0], 64)
	if err != nil {
		return err
	}
	ask, err := strconv.ParseFloat(data.Ask[0].(string), 64)
	if err != nil {
		return err
	}
	bid, err := strconv.ParseFloat(data.Bid[0].(string), 64)
	if err != nil {
		return err
	}

	k.Websocket.DataHandler <- &ticker.Price{
		ExchangeName: k.Name,
		Open:         openPrice,
		Close:        closePrice,
		Volume:       quantity,
		High:         highPrice,
		Low:          lowPrice,
		Bid:          bid,
		Ask:          ask,
		AssetType:    asset.Spot,
		Pair:         channelData.Pair,
	}
	return nil
}

// wsProcessSpread converts spread/orderbook data and sends it to the datahandler
func (k *Kraken) wsProcessSpread(channelData *WebsocketChannelData, data *[5]string) {
	if len(data) < 5 {
		k.Websocket.DataHandler <- fmt.Errorf("%s unexpected wsProcessSpread data length", k.Name)
		return
	}

	timeData, err := strconv.ParseFloat(data[2], 64)
	if err != nil {
		k.Websocket.DataHandler <- fmt.Errorf("%s wsProcessSpread: unable to parse timeData. Error: %s",
			k.Name,
			err)
		return
	}

	if k.Verbose {
		log.Debugf(log.ExchangeSys,
			"%v websocket spread data for '%v' received. Best bid: '%v' Best ask: '%v' Time: '%v', Bid volume '%v', Ask volume '%v'",
			k.Name,
			channelData.Pair,
			data[0],
			data[1],
			convert.TimeFromUnixTimestampDecimal(timeData),
			data[3],
			data[4])
	}
}

// wsProcessTrades converts trade data and sends it to the datahandler
func (k *Kraken) wsProcessTrades(channelData *WebsocketChannelData, data [][5]string) error {
	saveTradeData := k.IsSaveTradeDataEnabled()
	if !saveTradeData && !k.IsTradeFeedEnabled() {
		return nil
	}

	trades := make([]trade.Data, len(data))
	for i := range data {
		price, err := strconv.ParseFloat(data[i][0], 64)
		if err != nil {
			return err
		}

		amount, err := strconv.ParseFloat(data[i][1], 64)
		if err != nil {
			return err
		}

		timeData, err := strconv.ParseFloat(data[i][2], 64)
		if err != nil {
			return err
		}

		var tSide = order.Buy
		if data[i][3] == "s" {
			tSide = order.Sell
		}

		trades[i] = trade.Data{
			AssetType:    asset.Spot,
			CurrencyPair: channelData.Pair,
			Exchange:     k.Name,
			Price:        price,
			Amount:       amount,
			Timestamp:    convert.TimeFromUnixTimestampDecimal(timeData),
			Side:         tSide,
		}
	}

	return k.Websocket.Trade.Update(saveTradeData, trades...)
}

// wsProcessOrderBookUpdate processes orderbook updates
func (k *Kraken) wsProcessOrderBookUpdate(channelData *WebsocketChannelData, data *wsOrderbookUpdate) error {
	if len(data.Asks) == 0 && len(data.Bids) == 0 {
		return fmt.Errorf("%w for %v %v", errNoWebsocketOrderbookData, channelData.Pair, asset.Spot)
	}

	k.wsRequestMtx.Lock()
	defer k.wsRequestMtx.Unlock()
	err := k.wsProcessOrderBookUpdateStep2(channelData, data)
	if err != nil {
		outbound := channelData.Pair // Format required "XBT/USD"
		outbound.Delimiter = "/"
		go func(resub *stream.ChannelSubscription) {
			// This was locking the main websocket reader routine and a
			// backlog occurred. So put this into it's own go routine.
			errResub := k.Websocket.ResubscribeToChannel(resub)
			if errResub != nil {
				log.Errorf(log.WebsocketMgr,
					"%s resubscription failure for %v: %v",
					k.Name,
					resub,
					errResub)
			}
		}(&stream.ChannelSubscription{
			Channel:  krakenWsOrderbook,
			Currency: outbound,
			Asset:    asset.Spot,
		})
		return err
	}
	return nil
}

// wsProcessOrderBookPartial creates a new orderbook entry for a given currency pair
func (k *Kraken) wsProcessOrderBookPartial(channelData *WebsocketChannelData, snapshot *wsOrderbookSnapshot) error {
	base := orderbook.Base{
		Exchange:        k.Name,
		Pair:            channelData.Pair,
		Asset:           asset.Spot,
		VerifyOrderbook: k.CanVerifyOrderbook,
		Bids:            make(orderbook.Items, len(snapshot.Bids)),
		Asks:            make(orderbook.Items, len(snapshot.Asks)),
		MaxDepth:        channelData.MaxDepth,
	}
	// Kraken ob data is timestamped per price, GCT orderbook data is
	// timestamped per entry using the highest last update time, we can attempt
	// to respect both within a reasonable degree
	var highestLastUpdate time.Time
	for i := range snapshot.Asks {
		if len(snapshot.Asks[i]) < 3 {
			return errors.New("unexpected asks length")
		}

		var err error
		if base.Asks[i].Price, err = strconv.ParseFloat(snapshot.Asks[i][0], 64); err != nil {
			return err
		}
		if base.Asks[i].Amount, err = strconv.ParseFloat(snapshot.Asks[i][1], 64); err != nil {
			return err
		}

		timeData, err := strconv.ParseFloat(snapshot.Asks[i][2], 64)
		if err != nil {
			return err
		}

		askUpdatedTime := convert.TimeFromUnixTimestampDecimal(timeData)
		if highestLastUpdate.Before(askUpdatedTime) {
			highestLastUpdate = askUpdatedTime
		}
	}

	for i := range snapshot.Bids {
		if len(snapshot.Bids[i]) < 3 {
			return errors.New("unexpected bids length")
		}
		var err error
		if base.Bids[i].Price, err = strconv.ParseFloat(snapshot.Bids[i][0], 64); err != nil {
			return err
		}
		if base.Bids[i].Amount, err = strconv.ParseFloat(snapshot.Bids[i][1], 64); err != nil {
			return err
		}

		timeData, err := strconv.ParseFloat(snapshot.Bids[i][2], 64)
		if err != nil {
			return err
		}

		bidUpdateTime := convert.TimeFromUnixTimestampDecimal(timeData)
		if highestLastUpdate.Before(bidUpdateTime) {
			highestLastUpdate = bidUpdateTime
		}
	}
	base.LastUpdated = highestLastUpdate
	return k.Websocket.Orderbook.LoadSnapshot(&base)
}

// wsProcessOrderBookUpdate updates an orderbook entry for a given currency pair
func (k *Kraken) wsProcessOrderBookUpdateStep2(channelData *WebsocketChannelData, data *wsOrderbookUpdate) error {
	update := orderbook.Update{
		Asset: asset.Spot,
		Pair:  channelData.Pair,
		Bids:  make([]orderbook.Item, len(data.Bids)),
		Asks:  make([]orderbook.Item, len(data.Asks)),
	}

	// Calculating checksum requires incoming decimal place checks for both
	// price and amount as there is no set standard between currency pairs. This
	// is calculated per update as opposed to snapshot because changes to
	// decimal amounts could occur at any time.
	var priceDP, amtDP int
	var highestLastUpdate time.Time
	// Ask data is not always sent
	for i := range data.Asks {
		var err error
		if update.Asks[i].Price, err = strconv.ParseFloat(data.Asks[i][0], 64); err != nil {
			return err
		}
		if update.Asks[i].Amount, err = strconv.ParseFloat(data.Asks[i][1], 64); err != nil {
			return err
		}

		timeData, err := strconv.ParseFloat(data.Asks[i][2], 64)
		if err != nil {
			return err
		}

		askUpdatedTime := convert.TimeFromUnixTimestampDecimal(timeData)
		if highestLastUpdate.Before(askUpdatedTime) {
			highestLastUpdate = askUpdatedTime
		}

		if i == len(data.Asks)-1 {
			pIdx := strings.Index(data.Asks[i][0], ".")
			if pIdx == -1 {
				return errors.New("incorrect decimal data returned for price")
			}
			priceDP = len(data.Asks[i][0]) - pIdx - 1

			aIdx := strings.Index(data.Asks[i][1], ".")
			if aIdx == -1 {
				return errors.New("incorrect decimal data returned for amount")
			}
			amtDP = len(data.Asks[i][1]) - aIdx - 1
		}
	}

	// Bid data is not always sent
	for i := range data.Bids {
		var err error
		if update.Bids[i].Price, err = strconv.ParseFloat(data.Bids[i][0], 64); err != nil {
			return err
		}
		if update.Bids[i].Amount, err = strconv.ParseFloat(data.Bids[i][1], 64); err != nil {
			return err
		}

		timeData, err := strconv.ParseFloat(data.Bids[i][2], 64)
		if err != nil {
			return err
		}

		bidUpdatedTime := convert.TimeFromUnixTimestampDecimal(timeData)
		if highestLastUpdate.Before(bidUpdatedTime) {
			highestLastUpdate = bidUpdatedTime
		}

		if i == len(data.Bids)-1 {
			pIdx := strings.Index(data.Bids[i][0], ".")
			if pIdx == -1 {
				return errors.New("incorrect decimal data returned for price")
			}
			priceDP = len(data.Bids[i][0]) - pIdx - 1

			aIdx := strings.Index(data.Bids[i][1], ".")
			if aIdx == -1 {
				return errors.New("incorrect decimal data returned for amount")
			}
			amtDP = len(data.Bids[i][1]) - aIdx - 1
		}
	}
	update.UpdateTime = highestLastUpdate

	err := k.Websocket.Orderbook.Update(&update)
	if err != nil {
		return err
	}

	book, err := k.Websocket.Orderbook.GetOrderbook(channelData.Pair, asset.Spot)
	if err != nil {
		return fmt.Errorf("cannot calculate websocket checksum: book not found for %s %s %w",
			channelData.Pair,
			asset.Spot,
			err)
	}

	return validateCRC32(book, data.Checksum, priceDP, amtDP)
}

func validateCRC32(b *orderbook.Base, token uint32, decPrice, decAmount int) error {
	if decPrice == 0 || decAmount == 0 {
		return fmt.Errorf("%s %s trailing decimal count not calculated",
			b.Pair,
			b.Asset)
	}

	var checkStr strings.Builder
	for i := 0; i < 10 && i < len(b.Asks); i++ {
		priceStr := trim(strconv.FormatFloat(b.Asks[i].Price, 'f', decPrice, 64))
		checkStr.WriteString(priceStr)
		amountStr := trim(strconv.FormatFloat(b.Asks[i].Amount, 'f', decAmount, 64))
		checkStr.WriteString(amountStr)
	}

	for i := 0; i < 10 && i < len(b.Bids); i++ {
		priceStr := trim(strconv.FormatFloat(b.Bids[i].Price, 'f', decPrice, 64))
		checkStr.WriteString(priceStr)
		amountStr := trim(strconv.FormatFloat(b.Bids[i].Amount, 'f', decAmount, 64))
		checkStr.WriteString(amountStr)
	}

	if check := crc32.ChecksumIEEE([]byte(checkStr.String())); check != token {
		return fmt.Errorf("%s %s invalid checksum %d, expected %d",
			b.Pair,
			b.Asset,
			check,
			token)
	}
	return nil
}

// trim removes '.' and prefixed '0' from subsequent string
func trim(s string) string {
	s = strings.Replace(s, ".", "", 1)
	s = strings.TrimLeft(s, "0")
	return s
}

// wsProcessCandles converts candle data and sends it to the data handler
func (k *Kraken) wsProcessCandles(channelData *WebsocketChannelData, data *[9]interface{}) error {
	startTime, err := strconv.ParseFloat(data[0].(string), 64)
	if err != nil {
		return err
	}

	endTime, err := strconv.ParseFloat(data[1].(string), 64)
	if err != nil {
		return err
	}

	openPrice, err := strconv.ParseFloat(data[2].(string), 64)
	if err != nil {
		return err
	}

	highPrice, err := strconv.ParseFloat(data[3].(string), 64)
	if err != nil {
		return err
	}

	lowPrice, err := strconv.ParseFloat(data[4].(string), 64)
	if err != nil {
		return err
	}

	closePrice, err := strconv.ParseFloat(data[5].(string), 64)
	if err != nil {
		return err
	}

	volume, err := strconv.ParseFloat(data[7].(string), 64)
	if err != nil {
		return err
	}

	k.Websocket.DataHandler <- stream.KlineData{
		AssetType: asset.Spot,
		Pair:      channelData.Pair,
		Timestamp: time.Now(),
		Exchange:  k.Name,
		StartTime: convert.TimeFromUnixTimestampDecimal(startTime),
		CloseTime: convert.TimeFromUnixTimestampDecimal(endTime),
		// Candles are sent every 60 seconds
		Interval:   "60",
		HighPrice:  highPrice,
		LowPrice:   lowPrice,
		OpenPrice:  openPrice,
		ClosePrice: closePrice,
		Volume:     volume,
	}
	return nil
}

// GenerateDefaultSubscriptions Adds default subscriptions to websocket to be handled by ManageSubscriptions()
func (k *Kraken) GenerateDefaultSubscriptions() ([]stream.ChannelSubscription, error) {
	enabledCurrencies, err := k.GetEnabledPairs(asset.Spot)
	if err != nil {
		return nil, err
	}
	var subscriptions []stream.ChannelSubscription
	for i := range defaultSubscribedChannels {
		for j := range enabledCurrencies {
			enabledCurrencies[j].Delimiter = "/"
			subscriptions = append(subscriptions, stream.ChannelSubscription{
				Channel:  defaultSubscribedChannels[i],
				Currency: enabledCurrencies[j],
				Asset:    asset.Spot,
			})
		}
	}
	if k.Websocket.CanUseAuthenticatedEndpoints() {
		for i := range authenticatedChannels {
			subscriptions = append(subscriptions, stream.ChannelSubscription{
				Channel: authenticatedChannels[i],
			})
		}
	}
	return subscriptions, nil
}

// Subscribe sends a websocket message to receive data from the channel
func (k *Kraken) Subscribe(channelsToSubscribe []stream.ChannelSubscription) error {
	var subscriptions = make(map[string]*[]WebsocketSubscriptionEventRequest)
channels:
	for i := range channelsToSubscribe {
		s, ok := subscriptions[channelsToSubscribe[i].Channel]
		if !ok {
			s = &[]WebsocketSubscriptionEventRequest{}
			subscriptions[channelsToSubscribe[i].Channel] = s
		}

		for j := range *s {
			(*s)[j].Pairs = append((*s)[j].Pairs, channelsToSubscribe[i].Currency.String())
			(*s)[j].Channels = append((*s)[j].Channels, channelsToSubscribe[i])
			continue channels
		}

		id := k.Websocket.Conn.GenerateMessageID(false)
		outbound := WebsocketSubscriptionEventRequest{
			Event:     krakenWsSubscribe,
			RequestID: id,
			Subscription: WebsocketSubscriptionData{
				Name: channelsToSubscribe[i].Channel,
			},
		}
		if channelsToSubscribe[i].Channel == "book" {
			outbound.Subscription.Depth = krakenWsOrderbookDepth
		}
		if !channelsToSubscribe[i].Currency.IsEmpty() {
			outbound.Pairs = []string{channelsToSubscribe[i].Currency.String()}
		}
		if common.StringDataContains(authenticatedChannels, channelsToSubscribe[i].Channel) {
			outbound.Subscription.Token = authToken
		}

		outbound.Channels = append(outbound.Channels, channelsToSubscribe[i])
		*s = append(*s, outbound)
	}

	var errs error
	for _, subs := range subscriptions {
		for i := range *subs {
			if common.StringDataContains(authenticatedChannels, (*subs)[i].Subscription.Name) {
				_, err := k.Websocket.AuthConn.SendMessageReturnResponse((*subs)[i].RequestID, (*subs)[i])
				if err != nil {
					errs = common.AppendError(errs, err)
					continue
				}
				k.Websocket.AddSuccessfulSubscriptions((*subs)[i].Channels...)
				continue
			}
			_, err := k.Websocket.Conn.SendMessageReturnResponse((*subs)[i].RequestID, (*subs)[i])
			if err != nil {
				errs = common.AppendError(errs, err)
				continue
			}
			k.Websocket.AddSuccessfulSubscriptions((*subs)[i].Channels...)
		}
	}
	return errs
}

// Unsubscribe sends a websocket message to stop receiving data from the channel
func (k *Kraken) Unsubscribe(channelsToUnsubscribe []stream.ChannelSubscription) error {
	var unsubs []WebsocketSubscriptionEventRequest
channels:
	for x := range channelsToUnsubscribe {
		for y := range unsubs {
			if unsubs[y].Subscription.Name == channelsToUnsubscribe[x].Channel {
				unsubs[y].Pairs = append(unsubs[y].Pairs,
					channelsToUnsubscribe[x].Currency.String())
				unsubs[y].Channels = append(unsubs[y].Channels,
					channelsToUnsubscribe[x])
				continue channels
			}
		}
		var depth int64
		if channelsToUnsubscribe[x].Channel == "book" {
			depth = krakenWsOrderbookDepth
		}

		var id int64
		if common.StringDataContains(authenticatedChannels, channelsToUnsubscribe[x].Channel) {
			id = k.Websocket.AuthConn.GenerateMessageID(false)
		} else {
			id = k.Websocket.Conn.GenerateMessageID(false)
		}

		unsub := WebsocketSubscriptionEventRequest{
			Event: krakenWsUnsubscribe,
			Pairs: []string{channelsToUnsubscribe[x].Currency.String()},
			Subscription: WebsocketSubscriptionData{
				Name:  channelsToUnsubscribe[x].Channel,
				Depth: depth,
			},
			RequestID: id,
		}
		if common.StringDataContains(authenticatedChannels, channelsToUnsubscribe[x].Channel) {
			unsub.Subscription.Token = authToken
		}
		unsub.Channels = append(unsub.Channels, channelsToUnsubscribe[x])
		unsubs = append(unsubs, unsub)
	}

	var errs error
	for i := range unsubs {
		if common.StringDataContains(authenticatedChannels, unsubs[i].Subscription.Name) {
			_, err := k.Websocket.AuthConn.SendMessageReturnResponse(unsubs[i].RequestID, unsubs[i])
			if err != nil {
				errs = common.AppendError(errs, err)
				continue
			}
			k.Websocket.RemoveSuccessfulUnsubscriptions(unsubs[i].Channels...)
			continue
		}

		_, err := k.Websocket.Conn.SendMessageReturnResponse(unsubs[i].RequestID, unsubs[i])
		if err != nil {
			errs = common.AppendError(errs, err)
			continue
		}
		k.Websocket.RemoveSuccessfulUnsubscriptions(unsubs[i].Channels...)
	}
	return errs
}

// wsAddOrder creates an order, returned order ID if success
func (k *Kraken) wsAddOrder(request *WsAddOrderRequest) (string, error) {
	id := k.Websocket.AuthConn.GenerateMessageID(false)
	request.RequestID = id
	request.Event = krakenWsAddOrder
	request.Token = authToken
	jsonResp, err := k.Websocket.AuthConn.SendMessageReturnResponse(id, request)
	if err != nil {
		return "", err
	}
	var resp WsAddOrderResponse
	err = json.Unmarshal(jsonResp, &resp)
	if err != nil {
		return "", err
	}
	if resp.ErrorMessage != "" {
		return "", errors.New(k.Name + " - " + resp.ErrorMessage)
	}
	return resp.TransactionID, nil
}

// wsCancelOrders cancels one or more open orders passed in orderIDs param
func (k *Kraken) wsCancelOrders(orderIDs []string) error {
	id := k.Websocket.AuthConn.GenerateMessageID(false)
	request := WsCancelOrderRequest{
		Event:          krakenWsCancelOrder,
		Token:          authToken,
		TransactionIDs: orderIDs,
		RequestID:      id,
	}

	cancelOrdersStatus[id] = &struct {
		Total        int
		Successful   int
		Unsuccessful int
		Error        string
	}{
		Total: len(orderIDs),
	}

	defer delete(cancelOrdersStatus, id)

	_, err := k.Websocket.AuthConn.SendMessageReturnResponse(id, request)
	if err != nil {
		return err
	}

	successful := cancelOrdersStatus[id].Successful

	if cancelOrdersStatus[id].Error != "" || len(orderIDs) != successful { // strange Kraken logic ...
		var reason string
		if cancelOrdersStatus[id].Error != "" {
			reason = fmt.Sprintf(" Reason: %s", cancelOrdersStatus[id].Error)
		}
		return fmt.Errorf("%s cancelled %d out of %d orders.%s",
			k.Name, successful, len(orderIDs), reason)
	}
	return nil
}

// wsCancelAllOrders cancels all opened orders
// Returns number (count param) of affected orders or 0 if no open orders found
func (k *Kraken) wsCancelAllOrders() (*WsCancelOrderResponse, error) {
	id := k.Websocket.AuthConn.GenerateMessageID(false)
	request := WsCancelOrderRequest{
		Event:     krakenWsCancelAll,
		Token:     authToken,
		RequestID: id,
	}

	jsonResp, err := k.Websocket.AuthConn.SendMessageReturnResponse(id, request)
	if err != nil {
		return nil, err
	}
	var resp WsCancelOrderResponse
	err = json.Unmarshal(jsonResp, &resp)
	if err != nil {
		return nil, err
	}
	if resp.ErrorMessage != "" {
		return nil, errors.New(k.Name + " - " + resp.ErrorMessage)
	}
	return &resp, nil
}
