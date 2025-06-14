package binance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

func TestFuturesNewOrderRequest_Unmarshal(t *testing.T) {
	const inp = `
{
  "orderId": 18662274680,
  "symbol": "ETHUSD_PERP",
  "pair": "ETHUSD",
  "status": "NEW",
  "clientOrderId": "customID",
  "price": "4096",
  "avgPrice": "2.00",
  "origQty": "8",
  "executedQty": "4",
  "cumQty": "32",
  "cumBase": "16",
  "timeInForce": "GTC",
  "type": "LIMIT",
  "reduceOnly": true,
  "closePosition": true,
  "side": "BUY",
  "positionSide": "BOTH",
  "stopPrice": "2048",
  "workingType": "CONTRACT_PRICE",
  "priceProtect": true,
  "origType": "MARKET",
  "updateTime": 1635931801320,
  "activatePrice": "64",
  "priceRate": "32"
}
`

	var x FuturesOrderPlaceData

	err := json.Unmarshal([]byte(inp), &x)
	require.NoError(t, err, "Unmarshal must not error")

	assert.Equal(t, int64(18662274680), x.OrderID, "OrderID should be 18662274680")
	assert.Equal(t, "ETHUSD_PERP", x.Symbol, "Symbol should be ETHUSD_PERP")
	assert.Equal(t, "ETHUSD", x.Pair, "Pair should be ETHUSD")
	assert.Equal(t, "NEW", x.Status, "Status should be NEW")
	assert.Equal(t, "customID", x.ClientOrderID, "ClientOrderID should be customID")
	assert.Equal(t, float64(4096), x.Price, "Price should be 4096")
	assert.Equal(t, float64(2), x.AvgPrice, "AvgPrice should be 2")
	assert.Equal(t, float64(8), x.OrigQty, "OrigQty should be 8")
	assert.Equal(t, float64(4), x.ExecuteQty, "ExecuteQty should be 4")
	assert.Equal(t, float64(32), x.CumQty, "CumQty should be 32")
	assert.Equal(t, float64(16), x.CumBase, "CumBase should be 16")
	assert.Equal(t, "GTC", x.TimeInForce, "TimeInForce should be GTC")
	assert.Equal(t, cfuturesLimit, x.OrderType, "OrderType should be cfuturesLimit")
	assert.True(t, x.ReduceOnly, "ReduceOnly should be true")
	assert.True(t, x.ClosePosition, "ClosePosition should be true")
	assert.Equal(t, float64(2048), x.StopPrice, "StopPrice should be 2048")
	assert.Equal(t, "CONTRACT_PRICE", x.WorkingType, "WorkingType should be CONTRACT_PRICE")
	assert.True(t, x.PriceProtect, "PriceProtect should be true")
	assert.Equal(t, cfuturesMarket, x.OrigType, "OrigType should be cfuturesMarket")
	assert.Equal(t, int64(1635931801320), x.UpdateTime, "UpdateTime should be 1635931801320")
	assert.Equal(t, float64(64), x.ActivatePrice, "ActivatePrice should be 64")
	assert.Equal(t, float64(32), x.PriceRate, "PriceRate should be 32")
}
