package portfolio

import (
	"errors"
	"fmt"
	"time"

	"github.com/thrasher-corp/gocryptotrader/backtester/common"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/exchange"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/portfolio/compliance"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/portfolio/holdings"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/portfolio/risk"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventhandlers/portfolio/settings"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/event"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/fill"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/order"
	"github.com/thrasher-corp/gocryptotrader/backtester/eventtypes/signal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	gctorder "github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// Setup creates a portfolio manager instance and sets private fields
func Setup(sh SizeHandler, r risk.Handler, riskFreeRate float64) (*Portfolio, error) {
	if sh == nil {
		return nil, errors.New("received nil sizeHandler")
	}
	if riskFreeRate < 0 {
		return nil, errors.New("received negative riskFreeRate")
	}
	if r == nil {
		return nil, errors.New("received nil risk handler")
	}
	p := &Portfolio{}
	p.sizeManager = sh
	p.riskManager = r
	p.riskFreeRate = riskFreeRate

	return p, nil
}

func (p *Portfolio) Reset() {
	p.exchangeAssetPairSettings = nil
}

// OnSignal receives the event from the strategy on whether it has signalled to buy, do nothing or sell
// on buy/sell, the portfolio manager will size the order and assess the risk of the order
// if successful, it will pass on an order.Order to be used by the exchange event handler to place an order based on
// the portfolio manager's recommendations
func (p *Portfolio) OnSignal(signal signal.SignalEvent, cs *exchange.Settings) (*order.Order, error) {
	if signal == nil || cs == nil {
		return nil, errors.New("received nil arguments, cannot process OnSignal")
	}
	if p.sizeManager == nil {
		return nil, errors.New("no size manager set")
	}
	if p.riskManager == nil {
		return nil, errors.New("no risk manager set")
	}

	o := &order.Order{
		Event: event.Event{
			Exchange:     signal.GetExchange(),
			Time:         signal.GetTime(),
			CurrencyPair: signal.Pair(),
			AssetType:    signal.GetAssetType(),
			Interval:     signal.GetInterval(),
			Why:          signal.GetWhy(),
		},
		Direction: signal.GetDirection(),
	}
	if signal.GetDirection() == "" {
		return o, errors.New("invalid Direction")
	}

	lookup := p.exchangeAssetPairSettings[signal.GetExchange()][signal.GetAssetType()][signal.Pair()]
	if lookup == nil {
		return nil, fmt.Errorf("no portfolio settings for %v %v %v", signal.GetExchange(), signal.GetAssetType(), signal.Pair())
	}
	prevHolding := lookup.HoldingsSnapshots.GetLatestSnapshot()
	if p.iteration == 0 {
		prevHolding.InitialFunds = lookup.InitialFunds
		prevHolding.RemainingFunds = lookup.InitialFunds
		prevHolding.Exchange = signal.GetExchange()
		prevHolding.Pair = signal.Pair()
		prevHolding.Asset = signal.GetAssetType()
		prevHolding.Timestamp = signal.GetTime()
	}
	p.iteration++

	if signal.GetDirection() == common.DoNothing || signal.GetDirection() == common.MissingData || signal.GetDirection() == "" {
		return o, nil
	}

	if signal.GetDirection() == gctorder.Sell && prevHolding.PositionsSize == 0 {
		o.AppendWhy("no holdings to sell")
		o.SetDirection(common.CouldNotSell)
		signal.SetDirection(o.Direction)
		return o, nil
	}

	if signal.GetDirection() == gctorder.Buy && prevHolding.RemainingFunds <= 0 {
		o.AppendWhy("not enough funds to buy")
		o.SetDirection(common.CouldNotBuy)
		signal.SetDirection(o.Direction)
		return o, nil
	}

	o.Price = signal.GetPrice()
	o.Amount = signal.GetAmount()
	o.OrderType = gctorder.Market
	sizingFunds := prevHolding.RemainingFunds
	if signal.GetDirection() == gctorder.Sell {
		sizingFunds = prevHolding.PositionsSize
	}

	sizedOrder := p.sizeOrder(signal, cs, o, sizingFunds)

	return p.evaluateOrder(signal, o, sizedOrder)
}

func (p *Portfolio) evaluateOrder(signal signal.SignalEvent, originalOrderSignal *order.Order, sizedOrder *order.Order) (*order.Order, error) {
	var evaluatedOrder *order.Order
	cm, err := p.GetComplianceManager(originalOrderSignal.GetExchange(), originalOrderSignal.GetAssetType(), originalOrderSignal.Pair())
	if err != nil {
		return nil, err
	}

	evaluatedOrder, err = p.riskManager.EvaluateOrder(sizedOrder, p.GetLatestHoldingsForAllCurrencies(), cm.GetLatestSnapshot())
	if err != nil {
		originalOrderSignal.AppendWhy(err.Error())
		if signal.GetDirection() == gctorder.Buy {
			originalOrderSignal.Direction = common.CouldNotBuy
		} else if signal.GetDirection() == gctorder.Sell {
			originalOrderSignal.Direction = common.CouldNotSell
		} else {
			originalOrderSignal.Direction = common.DoNothing
		}
		signal.SetDirection(originalOrderSignal.Direction)
		return originalOrderSignal, nil
	}

	return evaluatedOrder, nil
}

func (p *Portfolio) sizeOrder(signal signal.SignalEvent, cs *exchange.Settings, originalOrderSignal *order.Order, sizingFunds float64) *order.Order {
	sizedOrder, err := p.sizeManager.SizeOrder(originalOrderSignal, sizingFunds, cs)
	if err != nil {
		originalOrderSignal.AppendWhy(err.Error())
		if originalOrderSignal.Direction == gctorder.Buy {
			originalOrderSignal.Direction = common.CouldNotBuy
		} else if originalOrderSignal.Direction == gctorder.Sell {
			originalOrderSignal.Direction = common.CouldNotSell
		} else {
			originalOrderSignal.Direction = common.DoNothing
		}
		signal.SetDirection(originalOrderSignal.Direction)
		return originalOrderSignal
	}

	if sizedOrder.Amount == 0 {
		if originalOrderSignal.Direction == gctorder.Buy {
			originalOrderSignal.Direction = common.CouldNotBuy
		} else if originalOrderSignal.Direction == gctorder.Sell {
			originalOrderSignal.Direction = common.CouldNotSell
		} else {
			originalOrderSignal.Direction = common.DoNothing
		}
		signal.SetDirection(originalOrderSignal.Direction)
		originalOrderSignal.AppendWhy("sized order to 0")
	}

	return sizedOrder
}

// OnFill processes the event after an order has been placed by the exchange. Its purpose is to track holdings for future portfolio decisions
func (p *Portfolio) OnFill(fillEvent fill.FillEvent) (*fill.Fill, error) {
	if fillEvent == nil {
		return nil, errors.New("nil fill event received, cannot process OnFill")
	}
	lookup := p.exchangeAssetPairSettings[fillEvent.GetExchange()][fillEvent.GetAssetType()][fillEvent.Pair()]
	if lookup == nil {
		return nil, fmt.Errorf("no currency settings found for %v %v %v", fillEvent.GetExchange(), fillEvent.GetAssetType(), fillEvent.Pair())
	}
	var err error
	// Get the holding from the previous iteration, create it if it doesn't yet have a timestamp
	h := lookup.GetLatestHoldings()
	if !h.Timestamp.Equal(fillEvent.GetTime().Add(-fillEvent.GetInterval().Duration())) && !h.Timestamp.IsZero() {
		log.Warnf(log.BackTester, "hey, there isn't a matching event. Expected %v, Received %v, please ensure data is correct. %v", fillEvent.GetTime().Add(-fillEvent.GetInterval().Duration()), h.Timestamp, fillEvent.GetTime().Add(-fillEvent.GetInterval().Duration()).Unix())
	}

	if !h.Timestamp.IsZero() {
		h.Update(fillEvent)
	} else {
		h, err = holdings.Create(fillEvent, lookup.InitialFunds, p.riskFreeRate)
		if err != nil {
			return nil, err
		}
	}
	err = p.setHoldings(fillEvent.GetExchange(), fillEvent.GetAssetType(), fillEvent.Pair(), h, true)
	if err != nil {
		log.Error(log.BackTester, err)
	}

	err = p.addComplianceSnapshot(fillEvent)
	if err != nil {
		log.Error(log.BackTester, err)
	}

	direction := fillEvent.GetDirection()
	if direction == common.DoNothing ||
		direction == common.CouldNotBuy ||
		direction == common.CouldNotSell ||
		direction == common.MissingData ||
		direction == "" {
		fe := fillEvent.(*fill.Fill)
		fe.ExchangeFee = 0
		return fe, nil
	}

	return fillEvent.(*fill.Fill), nil
}

// addComplianceSnapshot gets the previous snapshot of compliance events, updates with the latest fillevent
// then saves the snapshot to the c
func (p *Portfolio) addComplianceSnapshot(fillEvent fill.FillEvent) error {
	complianceManager, err := p.GetComplianceManager(fillEvent.GetExchange(), fillEvent.GetAssetType(), fillEvent.Pair())
	if err != nil {
		return err
	}
	if complianceManager.Interval == 0 {
		complianceManager.SetInterval(fillEvent.GetInterval())
	}
	prevSnap := complianceManager.GetLatestSnapshot()
	fo := fillEvent.GetOrder()
	if fo != nil {
		snapOrder := compliance.SnapshotOrder{
			ClosePrice:          fillEvent.GetClosePrice(),
			VolumeAdjustedPrice: fillEvent.GetVolumeAdjustedPrice(),
			SlippageRate:        fillEvent.GetSlippageRate(),
			Detail:              fo,
			CostBasis:           fo.Price + fo.Fee,
		}
		prevSnap.Orders = append(prevSnap.Orders, snapOrder)
	}
	err = complianceManager.AddSnapshot(prevSnap.Orders, fillEvent.GetTime(), true)
	if err != nil {
		return err
	}
	return nil
}

func (p *Portfolio) GetComplianceManager(exchangeName string, a asset.Item, cp currency.Pair) (*compliance.Manager, error) {
	lookup := p.exchangeAssetPairSettings[exchangeName][a][cp]
	if lookup == nil {
		return nil, fmt.Errorf("no exchange settings found for %v %v %v could not retrieve compliance manager", exchangeName, a, cp)
	}
	return &lookup.ComplianceManager, nil
}

func (p *Portfolio) SetFee(exch string, a asset.Item, cp currency.Pair, fee float64) {
	lookup := p.exchangeAssetPairSettings[exch][a][cp]
	lookup.Fee = fee
}

// GetFee can panic for bad requests, but why are you getting things that don't exist?
func (p *Portfolio) GetFee(exchangeName string, a asset.Item, cp currency.Pair) float64 {
	if p.exchangeAssetPairSettings == nil {
		return 0
	}
	lookup := p.exchangeAssetPairSettings[exchangeName][a][cp]
	if lookup == nil {
		return 0
	}
	return lookup.Fee
}

func (p *Portfolio) IsInvested(exchangeName string, a asset.Item, cp currency.Pair) (holdings.Holding, bool) {
	s := p.exchangeAssetPairSettings[exchangeName][a][cp]
	if s == nil {
		return holdings.Holding{}, false
	}
	h := s.GetLatestHoldings()
	if h.PositionsSize > 0 {
		return h, true
	}
	return h, false
}

func (p *Portfolio) Update(d common.DataEventHandler) error {
	if d == nil {
		return errors.New("received nil data event")
	}
	h, ok := p.IsInvested(d.GetExchange(), d.GetAssetType(), d.Pair())
	if !ok {
		return nil
	}
	h.UpdateValue(d)
	err := p.setHoldings(d.GetExchange(), d.GetAssetType(), d.Pair(), h, true)
	if err != nil {
		return err
	}
	return nil
}

func (p *Portfolio) SetInitialFunds(exch string, a asset.Item, cp currency.Pair, funds float64) error {
	lookup, ok := p.exchangeAssetPairSettings[exch][a][cp]
	if !ok {
		var err error
		lookup, err = p.SetupCurrencySettingsMap(exch, a, cp)
		if err != nil {
			return err
		}
	}
	lookup.InitialFunds = funds

	return nil
}

func (p *Portfolio) GetInitialFunds(exch string, a asset.Item, cp currency.Pair) float64 {
	lookup, ok := p.exchangeAssetPairSettings[exch][a][cp]
	if !ok {
		return 0
	}
	return lookup.InitialFunds
}

// GetLatestHoldingsForAllCurrencies will return the current holdings for all loaded currencies
// this is useful to assess the position of your entire portfolio in order to help with risk decisions
func (p *Portfolio) GetLatestHoldingsForAllCurrencies() []holdings.Holding {
	var resp []holdings.Holding
	for _, x := range p.exchangeAssetPairSettings {
		for _, y := range x {
			for _, z := range y {
				resp = append(resp, z.HoldingsSnapshots.GetLatestSnapshot())
			}
		}
	}
	return resp
}

func (p *Portfolio) setHoldings(exch string, a asset.Item, cp currency.Pair, h holdings.Holding, force bool) error {
	if h.Timestamp.IsZero() {
		return errors.New("holding with unset timestamp received")
	}
	lookup := p.exchangeAssetPairSettings[exch][a][cp]
	if lookup == nil {
		var err error
		lookup, err = p.SetupCurrencySettingsMap(exch, a, cp)
		if err != nil {
			return err
		}
	}
	for i := range lookup.HoldingsSnapshots.Holdings {
		if lookup.HoldingsSnapshots.Holdings[i].Timestamp.Equal(h.Timestamp) {
			if !force {
				return fmt.Errorf("holdings for %v %v %v at %v already set", exch, a, cp, h.Timestamp)
			}
			lookup.HoldingsSnapshots.Holdings[i] = h
			return nil
		}
	}
	lookup.HoldingsSnapshots.Holdings = append(lookup.HoldingsSnapshots.Holdings, h)
	p.exchangeAssetPairSettings[exch][a][cp] = lookup
	return nil
}

// ViewHoldingAtTimePeriod retrieves a snapshot of holdings at a specific time period,
// returning empty when not found
func (p *Portfolio) ViewHoldingAtTimePeriod(exch string, a asset.Item, cp currency.Pair, t time.Time) (holdings.Holding, error) {
	exchangeAssetPairSettings := p.exchangeAssetPairSettings[exch][a][cp]
	if exchangeAssetPairSettings == nil {
		return holdings.Holding{}, fmt.Errorf("no holdings found for %v %v %v", exch, a, cp)
	}
	for i := range exchangeAssetPairSettings.HoldingsSnapshots.Holdings {
		if t.Equal(exchangeAssetPairSettings.HoldingsSnapshots.Holdings[i].Timestamp) {
			return exchangeAssetPairSettings.HoldingsSnapshots.Holdings[i], nil
		}
	}

	return holdings.Holding{}, fmt.Errorf("no holdings found for %v %v %v at %v", exch, a, cp, t)
}

func (p *Portfolio) SetupCurrencySettingsMap(exch string, a asset.Item, cp currency.Pair) (*settings.Settings, error) {
	if exch == "" {
		return nil, errors.New("received empty exchange name")
	}
	if a == "" {
		return nil, errors.New("received empty asset")
	}
	if cp.IsEmpty() {
		return nil, errors.New("received unset currency pair")
	}
	if p.exchangeAssetPairSettings == nil {
		p.exchangeAssetPairSettings = make(map[string]map[asset.Item]map[currency.Pair]*settings.Settings)
	}
	if p.exchangeAssetPairSettings[exch] == nil {
		p.exchangeAssetPairSettings[exch] = make(map[asset.Item]map[currency.Pair]*settings.Settings)
	}
	if p.exchangeAssetPairSettings[exch][a] == nil {
		p.exchangeAssetPairSettings[exch][a] = make(map[currency.Pair]*settings.Settings)
	}
	if _, ok := p.exchangeAssetPairSettings[exch][a][cp]; !ok {
		p.exchangeAssetPairSettings[exch][a][cp] = &settings.Settings{}
	}

	return p.exchangeAssetPairSettings[exch][a][cp], nil
}