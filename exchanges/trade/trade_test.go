package trade

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/database"
	"github.com/thrasher-corp/gocryptotrader/database/drivers"
	sqltrade "github.com/thrasher-corp/gocryptotrader/database/repository/trade"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

func TestAddTradesToBuffer(t *testing.T) {
	t.Parallel()
	processor.mutex.Lock()
	processor.bufferProcessorInterval = BufferProcessorIntervalTime
	processor.mutex.Unlock()
	dbConf := database.Config{
		Enabled: true,
		Driver:  database.DBSQLite3,
		ConnectionDetails: drivers.ConnectionDetails{
			Host:     "localhost",
			Database: "./rpctestdb",
		},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	processor.setup(&wg)
	wg.Wait()
	require.NoError(t, database.DB.SetConfig(&dbConf), "SetConfig must not error")
	require.NoError(t, AddTradesToBuffer([]Data{
		{
			Timestamp:    time.Now(),
			Exchange:     "test!",
			CurrencyPair: currency.NewBTCUSD(),
			AssetType:    asset.Spot,
			Price:        1337,
			Amount:       1337,
			Side:         order.Buy,
		},
	}...), "AddTradesToBuffer must not error for valid trade")
	assert.NotZero(t, atomic.AddInt32(&processor.started, 0), "AddTradesToBuffer should start processor")

	err := AddTradesToBuffer([]Data{
		{
			Timestamp:    time.Now(),
			Exchange:     "test!",
			CurrencyPair: currency.NewBTCUSD(),
			AssetType:    asset.Spot,
			Price:        0,
			Amount:       0,
			Side:         order.Buy,
		},
	}...)
	assert.Error(t, err, "AddTradesToBuffer should error for zero price and amount")
	processor.mutex.Lock()
	processor.buffer = nil
	processor.mutex.Unlock()

	require.NoError(t, AddTradesToBuffer([]Data{
		{
			Timestamp:    time.Now(),
			Exchange:     "test!",
			CurrencyPair: currency.NewBTCUSD(),
			AssetType:    asset.Spot,
			Price:        -1,
			Amount:       -1,
		},
	}...), "AddTradesToBuffer must normalise negative values")
	processor.mutex.Lock()
	assert.Equal(t, float64(1), processor.buffer[0].Amount, "AddTradesToBuffer should convert negative amount to positive")
	assert.Equal(t, order.Sell, processor.buffer[0].Side, "AddTradesToBuffer should flip side when amount negative")
	processor.mutex.Unlock()
}

func TestSqlDataToTrade(t *testing.T) {
	t.Parallel()
	uuiderino, _ := uuid.NewV4()
	data, err := SQLDataToTrade(sqltrade.Data{
		ID:        uuiderino.String(),
		Timestamp: time.Time{},
		Exchange:  "hello",
		Base:      currency.BTC.String(),
		Quote:     currency.USD.String(),
		AssetType: "spot",
		Price:     1337,
		Amount:    1337,
		Side:      "buy",
	})
	require.NoError(t, err, "SQLDataToTrade must not error")
	require.Len(t, data, 1, "SQLDataToTrade must return single trade")
	assert.Equal(t, order.Buy, data[0].Side, "SQLDataToTrade should map side to buy")
	assert.Equal(t, "BTCUSD", data[0].CurrencyPair.String(), "SQLDataToTrade should map pair")
	assert.Equal(t, asset.Spot, data[0].AssetType, "SQLDataToTrade should map asset type to spot")
}

func TestTradeToSQLData(t *testing.T) {
	t.Parallel()
	cp := currency.NewBTCUSD()
	sqlData, err := tradeToSQLData(Data{
		Timestamp:    time.Now(),
		Exchange:     "test!",
		CurrencyPair: cp,
		AssetType:    asset.Spot,
		Price:        1337,
		Amount:       1337,
		Side:         order.Buy,
	})
	require.NoError(t, err, "tradeToSQLData must not error")
	require.Len(t, sqlData, 1, "tradeToSQLData must return single entry")
	assert.Equal(t, cp.Base.String(), sqlData[0].Base, "tradeToSQLData should map base")
	assert.Equal(t, asset.Spot.String(), sqlData[0].AssetType, "tradeToSQLData should map asset type")
}

func TestConvertTradesToCandles(t *testing.T) {
	t.Parallel()
	cp := currency.NewBTCUSD()
	startDate := time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC)
	candles, err := ConvertTradesToCandles(kline.FifteenSecond, []Data{
		{
			Timestamp:    startDate,
			Exchange:     "test!",
			CurrencyPair: cp,
			AssetType:    asset.Spot,
			Price:        1337,
			Amount:       1337,
			Side:         order.Buy,
		},
		{
			Timestamp:    startDate.Add(time.Second),
			Exchange:     "test!",
			CurrencyPair: cp,
			AssetType:    asset.Spot,
			Price:        1337,
			Amount:       1337,
			Side:         order.Buy,
		},
		{
			Timestamp:    startDate.Add(time.Minute),
			Exchange:     "test!",
			CurrencyPair: cp,
			AssetType:    asset.Spot,
			Price:        -1337,
			Amount:       -1337,
			Side:         order.Buy,
		},
	}...)
	require.NoError(t, err, "ConvertTradesToCandles must not error")
	require.Len(t, candles.Candles, 2, "ConvertTradesToCandles must produce two candles")
	assert.Equal(t, kline.FifteenSecond, candles.Interval, "ConvertTradesToCandles should preserve interval")
}

func TestShutdown(t *testing.T) {
	t.Parallel()
	var p Processor
	p.mutex.Lock()
	p.bufferProcessorInterval = time.Millisecond
	p.mutex.Unlock()
	var wg sync.WaitGroup
	wg.Add(1)
	go p.Run(&wg)
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&p.started), "Run should set started flag")
	time.Sleep(time.Millisecond * 20)
	assert.Equal(t, int32(0), atomic.LoadInt32(&p.started), "Run should reset started flag after shutdown")
}

func TestFilterTradesByTime(t *testing.T) {
	t.Parallel()
	trades := []Data{
		{
			Exchange:  "test",
			Timestamp: time.Now().Add(-time.Second),
		},
	}
	trades = FilterTradesByTime(trades, time.Now().Add(-time.Minute), time.Now())
	assert.Len(t, trades, 1, "FilterTradesByTime should keep trade within range")
	trades = FilterTradesByTime(trades, time.Now().Add(-time.Millisecond), time.Now())
	assert.Empty(t, trades, "FilterTradesByTime should remove trade outside range")
}

func TestSaveTradesToDatabase(t *testing.T) {
	t.Parallel()
	err := SaveTradesToDatabase(Data{})
	if err != nil {
		assert.Equal(t, "exchange name/uuid not set, cannot insert", err.Error(), "SaveTradesToDatabase should require exchange details")
	}
}

func TestGetTradesInRange(t *testing.T) {
	t.Parallel()
	_, err := GetTradesInRange("", "", "", "", time.Time{}, time.Time{})
	if err != nil {
		assert.Equal(t, "invalid arguments received", err.Error(), "GetTradesInRange should validate arguments")
	}
}
