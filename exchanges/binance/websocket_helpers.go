package binance

import (
	"strconv"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
)

// Binance echoes both string and numeric subscription IDs without changing their type.
func matchSubscriptionResponse(conn websocket.Connection, rawID, data json.RawMessage) error {
	if len(rawID) == 0 {
		return common.ErrMalformedData
	}
	if rawID[0] == '"' {
		var id string
		if err := json.Unmarshal(rawID, &id); err != nil {
			return err
		}
		return conn.RequireMatchWithData(id, data)
	}
	id, err := strconv.ParseInt(string(rawID), 10, 64)
	if err != nil {
		return err
	}
	return conn.RequireMatchWithData(id, data)
}

const (
	symbolParam      = "symbol"
	tickerStream     = "ticker"
	bookTickerStream = "bookTicker"
	aggTradeStream   = "aggTrade"
)

const listenKeyExpiredEvent = "listenKeyExpired"
