package bitstamp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

// UnmarshalJSON deserialises JSON and parses the minimum order value
func (p *TradingPair) UnmarshalJSON(data []byte) error {
	type Alias TradingPair
	t := &struct {
		*Alias
		MinimumOrder string `json:"minimum_order"`
	}{
		Alias: (*Alias)(p),
	}

	err := json.Unmarshal(data, t)
	if err != nil {
		return err
	}
	minOrderStr := t.MinimumOrder
	if prefix, _, found := strings.Cut(t.MinimumOrder, " "); found {
		minOrderStr = prefix
	}
	p.MinimumOrder, err = strconv.ParseFloat(minOrderStr, 64)
	return err
}

type orderSide order.Side

func (s *orderSide) UnmarshalJSON(data []byte) error {
	// Bitstamp quotes the side on the ticker and sends it bare over the websocket. A ,string tag
	// cannot reconcile the two, as encoding/json does not apply it to a type that unmarshals itself.
	if len(data) > 1 && data[0] == '"' && data[len(data)-1] == '"' {
		data = data[1 : len(data)-1]
	}
	var i int64
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	switch i {
	case 0:
		*s = orderSide(order.Buy)
	case 1:
		*s = orderSide(order.Sell)
	default:
		return fmt.Errorf("invalid value for order side: %v", i)
	}

	return nil
}

func (s *orderSide) Side() order.Side {
	return order.Side(*s)
}
