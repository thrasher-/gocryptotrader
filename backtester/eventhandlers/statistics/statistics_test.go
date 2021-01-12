package statistics

import (
	"strings"
	"testing"
	"time"

	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/portfolio/compliance"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/portfolio/holdings"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/statistics/currencystatstics"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/event"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/fill"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/kline"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/order"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/signal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	gctkline "github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	gctorder "github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

func TestReset(t *testing.T) {
	s := Statistic{
		TotalOrders: 1,
	}
	s.Reset()
	if s.TotalOrders != 0 {
		t.Error("expected 0")
	}
}

func TestAddDataEventForTime(t *testing.T) {
	tt := time.Now()
	exch := "binance"
	a := asset.Spot
	p := currency.NewPair(currency.BTC, currency.USDT)
	s := Statistic{}
	err := s.AddDataEventForTime(nil)
	if err != nil && err.Error() != "nil data event received" {
		t.Error(err)
	}
	err = s.AddDataEventForTime(&kline.Kline{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		Open:   1337,
		Close:  1337,
		Low:    1337,
		High:   1337,
		Volume: 1337,
	})
	if err != nil {
		t.Error(err)
	}
	if s.ExchangeAssetPairStatistics == nil {
		t.Error("expected not nil")
	}
	if len(s.ExchangeAssetPairStatistics[exch][a][p].Events) != 1 {
		t.Error("expected 1 event")
	}
}

func TestAddSignalEventForTime(t *testing.T) {
	tt := time.Now()
	exch := "binance"
	a := asset.Spot
	p := currency.NewPair(currency.BTC, currency.USDT)
	s := Statistic{}
	err := s.AddSignalEventForTime(nil)
	if err != nil && err.Error() != "nil signal event received" {
		t.Error(err)
	}
	err = s.AddSignalEventForTime(&signal.Signal{})
	if err != nil && err.Error() != "ExchangeAssetPairStatistics not setup" {
		t.Error(err)
	}
	s.ExchangeAssetPairStatistics = make(map[string]map[asset.Item]map[currency.Pair]*currencystatstics.CurrencyStatistic)
	err = s.AddSignalEventForTime(&signal.Signal{})
	if err != nil && !strings.Contains(err.Error(), "no data for") {
		t.Error(err)
	}

	err = s.AddDataEventForTime(&kline.Kline{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		Open:   1337,
		Close:  1337,
		Low:    1337,
		High:   1337,
		Volume: 1337,
	})
	if err != nil {
		t.Error(err)
	}
	err = s.AddSignalEventForTime(&signal.Signal{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		Amount:    1337,
		Price:     1337,
		Direction: gctorder.Buy,
	})
	if err != nil {
		t.Error(err)
	}
}

func TestAddExchangeEventForTime(t *testing.T) {
	tt := time.Now()
	exch := "binance"
	a := asset.Spot
	p := currency.NewPair(currency.BTC, currency.USDT)
	s := Statistic{}
	err := s.AddOrderEventForTime(nil)
	if err != nil && err.Error() != "nil order event received" {
		t.Error(err)
	}
	err = s.AddOrderEventForTime(&order.Order{})
	if err != nil && err.Error() != "ExchangeAssetPairStatistics not setup" {
		t.Error(err)
	}
	s.ExchangeAssetPairStatistics = make(map[string]map[asset.Item]map[currency.Pair]*currencystatstics.CurrencyStatistic)
	err = s.AddOrderEventForTime(&order.Order{})
	if err != nil && !strings.Contains(err.Error(), "no data for") {
		t.Error(err)
	}

	err = s.AddDataEventForTime(&kline.Kline{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		Open:   1337,
		Close:  1337,
		Low:    1337,
		High:   1337,
		Volume: 1337,
	})
	if err != nil {
		t.Error(err)
	}
	err = s.AddOrderEventForTime(&order.Order{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		ID:        "1337",
		Direction: gctorder.Buy,
		Status:    gctorder.New,
		Price:     1337,
		Amount:    1337,
		OrderType: gctorder.Stop,
		Limit:     1337,
		Leverage:  1337,
	})
	if err != nil {
		t.Error(err)
	}
}

func TestAddFillEventForTime(t *testing.T) {
	tt := time.Now()
	exch := "binance"
	a := asset.Spot
	p := currency.NewPair(currency.BTC, currency.USDT)
	s := Statistic{}
	err := s.AddFillEventForTime(nil)
	if err != nil && err.Error() != "nil fill event received" {
		t.Error(err)
	}
	err = s.AddFillEventForTime(&fill.Fill{})
	if err != nil && err.Error() != "ExchangeAssetPairStatistics not setup" {
		t.Error(err)
	}
	s.ExchangeAssetPairStatistics = make(map[string]map[asset.Item]map[currency.Pair]*currencystatstics.CurrencyStatistic)
	err = s.AddFillEventForTime(&fill.Fill{})
	if err != nil && !strings.Contains(err.Error(), "no data for") {
		t.Error(err)
	}

	err = s.AddDataEventForTime(&kline.Kline{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		Open:   1337,
		Close:  1337,
		Low:    1337,
		High:   1337,
		Volume: 1337,
	})
	if err != nil {
		t.Error(err)
	}
	err = s.AddFillEventForTime(&fill.Fill{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		Direction:           gctorder.Buy,
		Amount:              1337,
		ClosePrice:          1337,
		VolumeAdjustedPrice: 1337,
		PurchasePrice:       1337,
		ExchangeFee:         1337,
		Slippage:            1337,
	})
	if err != nil {
		t.Error(err)
	}
}

func TestAddHoldingsForTime(t *testing.T) {
	tt := time.Now()
	exch := "binance"
	a := asset.Spot
	p := currency.NewPair(currency.BTC, currency.USDT)
	s := Statistic{}

	err := s.AddHoldingsForTime(holdings.Holding{})
	if err != nil && err.Error() != "ExchangeAssetPairStatistics not setup" {
		t.Error(err)
	}
	s.ExchangeAssetPairStatistics = make(map[string]map[asset.Item]map[currency.Pair]*currencystatstics.CurrencyStatistic)
	err = s.AddHoldingsForTime(holdings.Holding{})
	if err != nil && !strings.Contains(err.Error(), "no data for") {
		t.Error(err)
	}

	err = s.AddDataEventForTime(&kline.Kline{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		Open:   1337,
		Close:  1337,
		Low:    1337,
		High:   1337,
		Volume: 1337,
	})
	if err != nil {
		t.Error(err)
	}
	err = s.AddHoldingsForTime(holdings.Holding{
		Pair:                         p,
		Asset:                        a,
		Exchange:                     exch,
		Timestamp:                    tt,
		InitialFunds:                 1337,
		PositionsSize:                1337,
		PositionsValue:               1337,
		SoldAmount:                   1337,
		SoldValue:                    1337,
		BoughtAmount:                 1337,
		BoughtValue:                  1337,
		RemainingFunds:               1337,
		TotalValueDifference:         1337,
		ChangeInTotalValuePercent:    1337,
		ExcessReturnPercent:          1337,
		BoughtValueDifference:        1337,
		SoldValueDifference:          1337,
		PositionsValueDifference:     1337,
		TotalValue:                   1337,
		TotalFees:                    1337,
		TotalValueLostToVolumeSizing: 1337,
		TotalValueLostToSlippage:     1337,
		TotalValueLost:               1337,
		RiskFreeRate:                 1337,
	})
	if err != nil {
		t.Error(err)
	}
}

func TestAddComplianceSnapshotForTime(t *testing.T) {
	tt := time.Now()
	exch := "binance"
	a := asset.Spot
	p := currency.NewPair(currency.BTC, currency.USDT)
	s := Statistic{}

	err := s.AddComplianceSnapshotForTime(compliance.Snapshot{}, nil)
	if err != nil && err.Error() != "nil fill event received" {
		t.Error(err)
	}
	err = s.AddComplianceSnapshotForTime(compliance.Snapshot{}, &fill.Fill{})
	if err != nil && err.Error() != "ExchangeAssetPairStatistics not setup" {
		t.Error(err)
	}
	s.ExchangeAssetPairStatistics = make(map[string]map[asset.Item]map[currency.Pair]*currencystatstics.CurrencyStatistic)
	err = s.AddComplianceSnapshotForTime(compliance.Snapshot{}, &fill.Fill{})
	if err != nil && !strings.Contains(err.Error(), "no data for") {
		t.Error(err)
	}

	err = s.AddDataEventForTime(&kline.Kline{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
		Open:   1337,
		Close:  1337,
		Low:    1337,
		High:   1337,
		Volume: 1337,
	})
	if err != nil {
		t.Error(err)
	}
	err = s.AddComplianceSnapshotForTime(compliance.Snapshot{
		Timestamp: tt,
	}, &fill.Fill{
		Event: event.Event{
			Exchange:     exch,
			Time:         tt,
			Interval:     gctkline.OneDay,
			CurrencyPair: p,
			AssetType:    a,
		},
	})
	if err != nil {
		t.Error(err)
	}
}

func TestSerialise(t *testing.T) {
	s := Statistic{}
	_, err := s.Serialise()
	if err != nil {
		t.Error(err)
	}
}

func TestSetStrategyName(t *testing.T) {
	s := Statistic{}
	s.SetStrategyName("test")
	if s.StrategyName != "test" {
		t.Error("expected test")
	}
}