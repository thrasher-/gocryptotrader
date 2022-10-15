package ftx

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gorilla/websocket"
	easyjson "github.com/mailru/easyjson"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/crypto"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fill"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/stream"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	"github.com/thrasher-corp/gocryptotrader/log"
)

const (
	ftxWSURL          = "wss://ftx.com/ws/"
	ftxWebsocketTimer = 13 * time.Second
	wsTicker          = "ticker"
	wsTrades          = "trades"
	wsOrderbook       = "orderbook"
	wsMarkets         = "markets"
	wsFills           = "fills"
	wsOrders          = "orders"
	wsUpdate          = "update"
	wsPartial         = "partial"
	subscribe         = "subscribe"
	unsubscribe       = "unsubscribe"
)

var obSuccess = make(map[currency.Pair]bool)

// WsConnect connects to a websocket feed
func (f *FTX) WsConnect() error {
	if !f.Websocket.IsEnabled() || !f.IsEnabled() {
		return errors.New(stream.WebsocketNotEnabled)
	}
	var dialer websocket.Dialer
	err := f.Websocket.Conn.Dial(&dialer, http.Header{})
	if err != nil {
		return err
	}
	f.Websocket.Conn.SetupPingHandler(stream.PingHandler{
		MessageType: websocket.PingMessage,
		Delay:       ftxWebsocketTimer,
	})
	if f.Verbose {
		log.Debugf(log.ExchangeSys, "%s Connected to Websocket.\n", f.Name)
	}

	f.Websocket.Wg.Add(1)
	go f.wsReadData()

	if f.IsWebsocketAuthenticationSupported() {
		err = f.WsAuth(context.TODO())
		if err != nil {
			f.Websocket.DataHandler <- err
			f.Websocket.SetCanUseAuthenticatedEndpoints(false)
		}
	}

	return nil
}

// WsAuth sends an authentication message to receive auth data
func (f *FTX) WsAuth(ctx context.Context) error {
	creds, err := f.GetCredentials(ctx)
	if err != nil {
		return err
	}
	intNonce := time.Now().UnixMilli()
	strNonce := strconv.FormatInt(intNonce, 10)
	hmac, err := crypto.GetHMAC(
		crypto.HashSHA256,
		[]byte(strNonce+"websocket_login"),
		[]byte(creds.Secret),
	)
	if err != nil {
		return err
	}
	sign := crypto.HexEncodeToString(hmac)
	req := Authenticate{Operation: "login",
		Args: AuthenticationData{
			Key:        creds.Key,
			Sign:       sign,
			Time:       intNonce,
			SubAccount: creds.SubAccount,
		},
	}

	return f.Websocket.Conn.SendJSONMessage(req)
}

// Subscribe sends a websocket message to receive data from the channel
func (f *FTX) Subscribe(channelsToSubscribe []stream.ChannelSubscription) error {
	var errs common.Errors
channels:
	for i := range channelsToSubscribe {
		var sub WsSub
		sub.Channel = channelsToSubscribe[i].Channel
		sub.Operation = subscribe

		switch channelsToSubscribe[i].Channel {
		case wsFills, wsOrders, wsMarkets:
		default:
			a, err := f.GetPairAssetType(channelsToSubscribe[i].Currency)
			if err != nil {
				errs = append(errs, err)
				continue channels
			}

			formattedPair, err := f.FormatExchangeCurrency(channelsToSubscribe[i].Currency, a)
			if err != nil {
				errs = append(errs, err)
				continue channels
			}
			sub.Market = formattedPair.String()
		}
		err := f.Websocket.Conn.SendJSONMessage(sub)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f.Websocket.AddSuccessfulSubscriptions(channelsToSubscribe[i])
	}
	if errs != nil {
		return errs
	}
	return nil
}

// Unsubscribe sends a websocket message to stop receiving data from the channel
func (f *FTX) Unsubscribe(channelsToUnsubscribe []stream.ChannelSubscription) error {
	var errs common.Errors
channels:
	for i := range channelsToUnsubscribe {
		var unSub WsSub
		unSub.Operation = unsubscribe
		unSub.Channel = channelsToUnsubscribe[i].Channel
		switch channelsToUnsubscribe[i].Channel {
		case wsFills, wsOrders, wsMarkets:
		default:
			a, err := f.GetPairAssetType(channelsToUnsubscribe[i].Currency)
			if err != nil {
				errs = append(errs, err)
				continue channels
			}

			formattedPair, err := f.FormatExchangeCurrency(channelsToUnsubscribe[i].Currency, a)
			if err != nil {
				errs = append(errs, err)
				continue channels
			}
			unSub.Market = formattedPair.String()
		}
		err := f.Websocket.Conn.SendJSONMessage(unSub)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f.Websocket.RemoveSuccessfulUnsubscriptions(channelsToUnsubscribe[i])
	}
	if errs != nil {
		return errs
	}
	return nil
}

// GenerateDefaultSubscriptions generates default subscription
func (f *FTX) GenerateDefaultSubscriptions() ([]stream.ChannelSubscription, error) {
	var subscriptions []stream.ChannelSubscription
	subscriptions = append(subscriptions, stream.ChannelSubscription{
		Channel: wsMarkets,
	})
	var channels = []string{wsTicker, wsTrades, wsOrderbook}
	assets := f.GetAssetTypes(true)
	for a := range assets {
		pairs, err := f.GetEnabledPairs(assets[a])
		if err != nil {
			return nil, err
		}
		for z := range pairs {
			newPair := currency.NewPairWithDelimiter(pairs[z].Base.String(),
				pairs[z].Quote.String(),
				"-")
			for x := range channels {
				subscriptions = append(subscriptions,
					stream.ChannelSubscription{
						Channel:  channels[x],
						Currency: newPair,
						Asset:    assets[a],
					})
			}
		}
	}
	if f.IsWebsocketAuthenticationSupported() {
		var authchan = []string{wsOrders, wsFills}
		for x := range authchan {
			subscriptions = append(subscriptions, stream.ChannelSubscription{
				Channel: authchan[x],
			})
		}
	}
	return subscriptions, nil
}

// wsReadData gets and passes on websocket messages for processing
func (f *FTX) wsReadData() {
	defer f.Websocket.Wg.Done()

	for {
		select {
		case <-f.Websocket.ShutdownC:
			return
		default:
			resp := f.Websocket.Conn.ReadMessage()
			if resp.Raw == nil {
				return
			}

			err := f.wsHandleData(resp.Raw)
			if err != nil {
				f.Websocket.DataHandler <- err
			}
		}
	}
}

func timestampFromFloat64(ts float64) time.Time {
	secs := int64(ts)
	nsecs := int64((ts - float64(secs)) * 1e9)
	return time.Unix(secs, nsecs).UTC()
}

func (f *FTX) wsHandleData(respRaw []byte) error {
	typeMsg, err := jsonparser.GetString(respRaw, "type")
	if err != nil {
		return err
	}

	switch typeMsg {
	case wsUpdate:
		var p currency.Pair
		var a asset.Item
		market, err := jsonparser.GetString(respRaw, "market")
		if err == nil {
			p, err = currency.NewPairFromString(market)
			if err != nil {
				return err
			}
			a, err = f.GetPairAssetType(p)
			if err != nil {
				return err
			}
		}
		channel, err := jsonparser.GetString(respRaw, "channel")
		if err != nil {
			return err
		}
		switch channel {
		case wsTicker:
			var resultData WsTickerDataStore
			err = easyjson.Unmarshal(respRaw, &resultData)
			if err != nil {
				return err
			}
			f.Websocket.DataHandler <- &ticker.Price{
				ExchangeName: f.Name,
				Bid:          resultData.Ticker.Bid,
				BidSize:      resultData.Ticker.BidSize,
				Ask:          resultData.Ticker.Ask,
				AskSize:      resultData.Ticker.AskSize,
				Last:         resultData.Ticker.Last,
				LastUpdated:  timestampFromFloat64(resultData.Ticker.Time),
				Pair:         p,
				AssetType:    a,
			}
		case wsOrderbook:
			var resultData WsOrderbookDataStore
			err = easyjson.Unmarshal(respRaw, &resultData)
			if err != nil {
				return err
			}
			if len(resultData.OBData.Asks) == 0 && len(resultData.OBData.Bids) == 0 {
				return nil
			}
			err = f.WsProcessUpdateOB(&resultData.OBData, p, a)
			if err != nil {
				err2 := f.wsResubToOB(p)
				if err2 != nil {
					f.Websocket.DataHandler <- err2
				}
				return err
			}
		case wsTrades:
			saveTradeData := f.IsSaveTradeDataEnabled()

			if !saveTradeData &&
				!f.IsTradeFeedEnabled() {
				return nil
			}
			var resultData WsTradeDataStore
			err = easyjson.Unmarshal(respRaw, &resultData)
			if err != nil {
				return err
			}
			var trades []trade.Data
			for z := range resultData.TradeData {
				var oSide order.Side
				oSide, err = order.StringToOrderSide(resultData.TradeData[z].Side)
				if err != nil {
					f.Websocket.DataHandler <- order.ClassificationError{
						Exchange: f.Name,
						Err:      err,
					}
				}
				trades = append(trades, trade.Data{
					Timestamp:    resultData.TradeData[z].Time,
					CurrencyPair: p,
					AssetType:    a,
					Exchange:     f.Name,
					Price:        resultData.TradeData[z].Price,
					Amount:       resultData.TradeData[z].Size,
					Side:         oSide,
					TID:          strconv.FormatInt(resultData.TradeData[z].ID, 10),
				})
			}
			return f.Websocket.Trade.Update(saveTradeData, trades...)
		case wsOrders:
			var resultData WsOrderDataStore
			err = easyjson.Unmarshal(respRaw, &resultData)
			if err != nil {
				return err
			}
			var pair currency.Pair
			pair, err = currency.NewPairFromString(resultData.OrderData.Market)
			if err != nil {
				return err
			}
			var assetType asset.Item
			assetType, err = f.GetPairAssetType(pair)
			if err != nil {
				return err
			}
			var orderVars OrderVars
			orderVars, err = f.compatibleOrderVars(context.TODO(),
				resultData.OrderData.Side,
				resultData.OrderData.Status,
				resultData.OrderData.OrderType,
				resultData.OrderData.Size,
				resultData.OrderData.FilledSize,
				resultData.OrderData.AvgFillPrice)
			if err != nil {
				return err
			}
			var resp order.Detail
			resp.PostOnly = resultData.OrderData.PostOnly
			resp.Price = resultData.OrderData.Price
			resp.Amount = resultData.OrderData.Size
			resp.AverageExecutedPrice = resultData.OrderData.AvgFillPrice
			resp.ExecutedAmount = resultData.OrderData.FilledSize
			resp.RemainingAmount = resultData.OrderData.Size - resultData.OrderData.FilledSize
			resp.Cost = resp.AverageExecutedPrice * resultData.OrderData.FilledSize
			// Fee: orderVars.Fee is incorrect.
			resp.Exchange = f.Name
			resp.OrderID = strconv.FormatInt(resultData.OrderData.ID, 10)
			resp.ClientOrderID = resultData.OrderData.ClientID
			resp.Type = orderVars.OrderType
			resp.Side = orderVars.Side
			resp.Status = orderVars.Status
			resp.AssetType = assetType
			resp.Date = resultData.OrderData.CreatedAt
			// There's no current timestamp, so this is the best we can get.
			resp.LastUpdated = resultData.OrderData.CreatedAt
			resp.Pair = pair
			f.Websocket.DataHandler <- &resp
		case wsFills:
			if !f.IsFillsFeedEnabled() {
				return nil
			}

			var resultData WsFillsDataStore
			err = easyjson.Unmarshal(respRaw, &resultData)
			if err != nil {
				return err
			}

			var side order.Side
			side, err = order.StringToOrderSide(resultData.FillsData.Side)
			if err != nil {
				f.Websocket.DataHandler <- order.ClassificationError{
					Exchange: f.Name,
					Err:      err,
				}
			}

			p, err = currency.NewPairFromString(resultData.FillsData.Market)
			if err != nil {
				return err
			}
			a, err = f.GetPairAssetType(p)
			if err != nil {
				return err
			}

			return f.Websocket.Fills.Update(fill.Data{
				ID:           strconv.FormatInt(resultData.FillsData.ID, 10),
				Timestamp:    resultData.FillsData.Time,
				Exchange:     f.Name,
				AssetType:    a,
				CurrencyPair: p,
				Side:         side,
				OrderID:      strconv.FormatInt(resultData.FillsData.OrderID, 10),
				TradeID:      strconv.FormatInt(resultData.FillsData.TradeID, 10),
				Price:        resultData.FillsData.Price,
				Amount:       resultData.FillsData.Size,
			})
		default:
			f.Websocket.DataHandler <- stream.UnhandledMessageWarning{Message: f.Name + stream.UnhandledMessage + string(respRaw)}
		}
	case wsPartial:
		channel, err := jsonparser.GetString(respRaw, "channel")
		if err != nil {
			return err
		}

		switch channel {
		case "orderbook":
			var p currency.Pair
			var a asset.Item

			market, err := jsonparser.GetString(respRaw, "market")
			if err != nil {
				return err
			}

			p, err = currency.NewPairFromString(market)
			if err != nil {
				return err
			}
			a, err = f.GetPairAssetType(p)
			if err != nil {
				return err
			}

			var resultData WsOrderbookDataStore
			err = easyjson.Unmarshal(respRaw, &resultData)
			if err != nil {
				return err
			}
			err = f.WsProcessPartialOB(&resultData.OBData, p, a)
			if err != nil {
				err2 := f.wsResubToOB(p)
				if err2 != nil {
					f.Websocket.DataHandler <- err2
				}
				return err
			}
			// reset obchecksum failure blockage for pair
			delete(obSuccess, p)
		case wsMarkets:
			var resultData WSMarkets
			err = easyjson.Unmarshal(respRaw, &resultData)
			if err != nil {
				return err
			}
			f.Websocket.DataHandler <- resultData.Data
		}
	case "error":
		f.Websocket.DataHandler <- stream.UnhandledMessageWarning{
			Message: f.Name + stream.UnhandledMessage + string(respRaw),
		}
	}
	return nil
}

// WsProcessUpdateOB processes an update on the orderbook
func (f *FTX) WsProcessUpdateOB(data *WsOrderbookData, p currency.Pair, a asset.Item) error {
	update := orderbook.Update{
		Asset:      a,
		Pair:       p,
		Bids:       make([]orderbook.Item, len(data.Bids)),
		Asks:       make([]orderbook.Item, len(data.Asks)),
		UpdateTime: timestampFromFloat64(data.Time),
	}

	for x := range data.Bids {
		update.Bids[x] = orderbook.Item{
			Price:  data.Bids[x][0],
			Amount: data.Bids[x][1],
		}
	}
	for x := range data.Asks {
		update.Asks[x] = orderbook.Item{
			Price:  data.Asks[x][0],
			Amount: data.Asks[x][1],
		}
	}

	err := f.Websocket.Orderbook.Update(&update)
	if err != nil {
		return err
	}

	updatedOb, err := f.Websocket.Orderbook.GetOrderbook(p, a)
	if err != nil {
		return err
	}
	checksum := f.CalcUpdateOBChecksum(updatedOb)

	if checksum != data.Checksum {
		log.Warnf(log.ExchangeSys, "%s checksum failure for item %s",
			f.Name,
			p)
		return errors.New("checksum failed")
	}
	return nil
}

func (f *FTX) wsResubToOB(p currency.Pair) error {
	if ok := obSuccess[p]; ok {
		return nil
	}

	obSuccess[p] = true

	channelToResubscribe := &stream.ChannelSubscription{
		Channel:  wsOrderbook,
		Currency: p,
	}
	err := f.Websocket.ResubscribeToChannel(channelToResubscribe)
	if err != nil {
		return fmt.Errorf("%s resubscribe to orderbook failure %s", f.Name, err)
	}
	return nil
}

// WsProcessPartialOB creates an OB from websocket data
func (f *FTX) WsProcessPartialOB(data *WsOrderbookData, p currency.Pair, a asset.Item) error {
	signedChecksum := f.CalcPartialOBChecksum(data)
	if signedChecksum != data.Checksum {
		return fmt.Errorf("%s channel: %s. Orderbook partial for %v checksum invalid",
			f.Name,
			a,
			p)
	}
	bids := make(orderbook.Items, len(data.Bids))
	asks := make(orderbook.Items, len(data.Asks))
	for x := range data.Bids {
		bids[x] = orderbook.Item{
			Price:  data.Bids[x][0],
			Amount: data.Bids[x][1],
		}
	}
	for x := range data.Asks {
		asks[x] = orderbook.Item{
			Price:  data.Asks[x][0],
			Amount: data.Asks[x][1],
		}
	}

	newOrderBook := orderbook.Base{
		Asks:            asks,
		Bids:            bids,
		Asset:           a,
		LastUpdated:     timestampFromFloat64(data.Time),
		Pair:            p,
		Exchange:        f.Name,
		VerifyOrderbook: f.CanVerifyOrderbook,
	}

	return f.Websocket.Orderbook.LoadSnapshot(&newOrderBook)
}

// CalcPartialOBChecksum calculates checksum of partial OB data received from WS
func (f *FTX) CalcPartialOBChecksum(data *WsOrderbookData) int64 {
	var checksum strings.Builder
	var price, amount string
	for i := 0; i < 100; i++ {
		if len(data.Bids)-1 >= i {
			price = checksumParseNumber(data.Bids[i][0])
			amount = checksumParseNumber(data.Bids[i][1])
			checksum.WriteString(price + ":" + amount + ":")
		}
		if len(data.Asks)-1 >= i {
			price = checksumParseNumber(data.Asks[i][0])
			amount = checksumParseNumber(data.Asks[i][1])
			checksum.WriteString(price + ":" + amount + ":")
		}
	}
	checksumStr := strings.TrimSuffix(checksum.String(), ":")
	return int64(crc32.ChecksumIEEE([]byte(checksumStr)))
}

// CalcUpdateOBChecksum calculates checksum of update OB data received from WS
func (f *FTX) CalcUpdateOBChecksum(data *orderbook.Base) int64 {
	var checksum strings.Builder
	var price, amount string
	for i := 0; i < 100; i++ {
		if len(data.Bids)-1 >= i {
			price = checksumParseNumber(data.Bids[i].Price)
			amount = checksumParseNumber(data.Bids[i].Amount)
			checksum.WriteString(price + ":" + amount + ":")
		}
		if len(data.Asks)-1 >= i {
			price = checksumParseNumber(data.Asks[i].Price)
			amount = checksumParseNumber(data.Asks[i].Amount)
			checksum.WriteString(price + ":" + amount + ":")
		}
	}
	checksumStr := strings.TrimSuffix(checksum.String(), ":")
	return int64(crc32.ChecksumIEEE([]byte(checksumStr)))
}

func checksumParseNumber(num float64) string {
	modifier := byte('f')
	if num < 0.0001 {
		modifier = 'e'
	}
	r := strconv.FormatFloat(num, modifier, -1, 64)
	if strings.IndexByte(r, '.') == -1 && modifier != 'e' {
		r += ".0"
	}
	return r
}
