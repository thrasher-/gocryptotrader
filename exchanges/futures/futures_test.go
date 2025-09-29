package futures

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

const testExchange = "test"

// FakePNL implements PNL interface
type FakePNL struct {
	err    error
	result *PNLResult
}

// CalculatePNL overrides default pnl calculations
func (f *FakePNL) CalculatePNL(context.Context, *PNLCalculatorRequest) (*PNLResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// GetCurrencyForRealisedPNL  overrides default pnl calculations
func (f *FakePNL) GetCurrencyForRealisedPNL(realisedAsset asset.Item, realisedPair currency.Pair) (currency.Code, asset.Item, error) {
	if f.err != nil {
		return realisedPair.Base, asset.Empty, f.err
	}
	return realisedPair.Base, realisedAsset, nil
}

func TestUpsertPNLEntry(t *testing.T) {
	t.Parallel()
	var results []PNLResult
	result := &PNLResult{
		IsOrder: true,
	}
	_, err := upsertPNLEntry(results, result)
	assert.ErrorIs(t, err, errTimeUnset)

	tt := time.Now()
	result.Time = tt
	results, err = upsertPNLEntry(results, result)
	assert.NoError(t, err)

	assert.Len(t, results, 1, "upsertPNLEntry should append first result")
	result.Fee = decimal.NewFromInt(1337)
	results, err = upsertPNLEntry(results, result)
	assert.NoError(t, err)

	assert.Len(t, results, 1, "upsertPNLEntry should update existing entry")
	assert.Truef(t, results[0].Fee.Equal(result.Fee), "upsertPNLEntry should update fee to %v", result.Fee)
}

func TestTrackNewOrder(t *testing.T) {
	t.Parallel()
	exch := testExchange
	item := asset.Futures
	pair, err := currency.NewPairFromStrings("BTC", "1231")
	assert.NoError(t, err)

	setup := &PositionTrackerSetup{
		Exchange: exch,
		Asset:    item,
		Pair:     pair,
	}
	c, err := SetupPositionTracker(setup)
	assert.NoError(t, err)

	err = c.TrackNewOrder(nil, false)
	assert.ErrorIs(t, err, common.ErrNilPointer)

	err = c.TrackNewOrder(&order.Detail{}, false)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	od := &order.Detail{
		Exchange:  exch,
		AssetType: item,
		Pair:      pair,
		OrderID:   "1",
		Price:     1337,
	}
	err = c.TrackNewOrder(od, false)
	assert.ErrorIs(t, err, order.ErrSideIsInvalid)

	od.Side = order.Long
	od.Amount = 1
	od.OrderID = "2"
	err = c.TrackNewOrder(od, false)
	assert.ErrorIs(t, err, errTimeUnset)

	c.openingDirection = order.Long
	od.Date = time.Now()
	err = c.TrackNewOrder(od, false)
	assert.NoError(t, err)

	assert.True(t, c.openingPrice.Equal(decimal.NewFromInt(1337)), "TrackNewOrder should set openingPrice to 1337")
	assert.Len(t, c.longPositions, 1, "TrackNewOrder should store one long position")
	assert.Equal(t, order.Long, c.latestDirection, "TrackNewOrder should mark latestDirection long")
	assert.Equal(t, od.Amount, c.exposure.InexactFloat64(), "TrackNewOrder should track exposure amount")

	od.Date = od.Date.Add(1)
	od.Amount = 0.4
	od.Side = order.Short
	od.OrderID = "3"
	err = c.TrackNewOrder(od, false)
	assert.NoError(t, err)

	assert.Len(t, c.shortPositions, 1, "TrackNewOrder should store one short position")
	assert.Equal(t, order.Long, c.latestDirection, "TrackNewOrder should maintain latestDirection long")
	assert.Equal(t, 0.6, c.exposure.InexactFloat64(), "TrackNewOrder should reduce exposure to 0.6")

	od.Date = od.Date.Add(1)
	od.Amount = 0.8
	od.Side = order.Short
	od.OrderID = "4"
	od.Fee = 0.1
	err = c.TrackNewOrder(od, false)
	assert.NoError(t, err)

	assert.Equal(t, order.Short, c.latestDirection, "TrackNewOrder should mark latestDirection short")
	assert.Truef(t, c.exposure.Equal(decimal.NewFromFloat(0.2)), "TrackNewOrder should set exposure to 0.2, received %v", c.exposure)

	od.Date = od.Date.Add(1)
	od.OrderID = "5"
	od.Side = order.Long
	od.Amount = 0.2
	err = c.TrackNewOrder(od, false)
	assert.NoError(t, err)

	assert.Equal(t, order.ClosePosition, c.latestDirection, "TrackNewOrder should recognise closed position")
	assert.Equal(t, order.Closed, c.status, "TrackNewOrder should mark status closed")

	err = c.TrackNewOrder(od, false)
	assert.NoError(t, err)

	od.OrderID = "hellomoto"
	err = c.TrackNewOrder(od, false)
	assert.ErrorIs(t, err, ErrPositionClosed)

	assert.Equal(t, order.ClosePosition, c.latestDirection, "TrackNewOrder should remain closed after error")
	assert.Equal(t, order.Closed, c.status, "TrackNewOrder should keep status closed after error")

	err = c.TrackNewOrder(od, true)
	assert.ErrorIs(t, err, errCannotTrackInvalidParams)

	c, err = SetupPositionTracker(setup)
	assert.NoError(t, err)

	err = c.TrackNewOrder(od, true)
	assert.NoError(t, err)

	var ptp *PositionTracker
	err = ptp.TrackNewOrder(nil, false)
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestSetupMultiPositionTracker(t *testing.T) {
	t.Parallel()

	_, err := SetupMultiPositionTracker(nil)
	assert.ErrorIs(t, err, errNilSetup)

	setup := &MultiPositionTrackerSetup{}
	_, err = SetupMultiPositionTracker(setup)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	setup.Exchange = testExchange
	_, err = SetupMultiPositionTracker(setup)
	assert.ErrorIs(t, err, ErrNotFuturesAsset)

	setup.Asset = asset.Futures
	_, err = SetupMultiPositionTracker(setup)
	assert.ErrorIs(t, err, order.ErrPairIsEmpty)

	setup.Pair = currency.NewBTCUSDT()
	_, err = SetupMultiPositionTracker(setup)
	assert.ErrorIs(t, err, errEmptyUnderlying)

	setup.Underlying = currency.BTC
	_, err = SetupMultiPositionTracker(setup)
	assert.NoError(t, err)

	setup.UseExchangePNLCalculation = true
	_, err = SetupMultiPositionTracker(setup)
	assert.ErrorIs(t, err, errMissingPNLCalculationFunctions)

	setup.ExchangePNLCalculation = &FakePNL{}
	resp, err := SetupMultiPositionTracker(setup)
	assert.NoError(t, err)

	assert.Equal(t, testExchange, resp.exchange, "SetupMultiPositionTracker should set exchange")
}

func TestMultiPositionTrackerTrackNewOrder(t *testing.T) {
	t.Parallel()
	exch := testExchange
	item := asset.Futures
	pair := currency.NewBTCUSDT()
	setup := &MultiPositionTrackerSetup{
		Asset:                  item,
		Pair:                   pair,
		Underlying:             pair.Base,
		ExchangePNLCalculation: &FakePNL{},
	}
	_, err := SetupMultiPositionTracker(setup)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	setup.Exchange = testExchange
	resp, err := SetupMultiPositionTracker(setup)
	assert.NoError(t, err)

	tt := time.Now()
	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		AssetType: item,
		Pair:      pair,
		Side:      order.Short,
		OrderID:   "1",
		Amount:    1,
	})
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  exch,
		AssetType: item,
		Pair:      pair,
		Side:      order.Short,
		OrderID:   "1",
		Amount:    1,
	})
	assert.NoError(t, err)

	assert.Len(t, resp.positions, 1, "TrackNewOrder should add first position")

	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  exch,
		AssetType: item,
		Pair:      pair,
		Side:      order.Short,
		OrderID:   "2",
		Amount:    1,
	})
	assert.NoError(t, err)

	assert.Len(t, resp.positions, 1, "TrackNewOrder should not duplicate short position")

	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  exch,
		AssetType: item,
		Pair:      pair,
		Side:      order.Long,
		OrderID:   "3",
		Amount:    2,
	})
	assert.NoError(t, err)

	assert.Len(t, resp.positions, 1, "TrackNewOrder should close existing position")
	assert.Equal(t, order.Closed, resp.positions[0].status, "TrackNewOrder should mark position closed")
	resp.positions[0].status = order.Open
	resp.positions = append(resp.positions, resp.positions...)
	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  exch,
		AssetType: item,
		Pair:      pair,
		Side:      order.Long,
		OrderID:   "4",
		Amount:    2,
	})
	assert.ErrorIs(t, err, errPositionDiscrepancy)

	resp.positions = []*PositionTracker{resp.positions[0]}
	resp.positions[0].status = order.Closed
	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  exch,
		AssetType: item,
		Pair:      pair,
		Side:      order.Long,
		OrderID:   "4",
		Amount:    2,
	})
	assert.NoError(t, err)

	assert.Len(t, resp.positions, 2, "TrackNewOrder should append new position after close")

	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  exch,
		AssetType: item,
		Pair:      pair,
		Side:      order.Long,
		OrderID:   "4",
		Amount:    2,
	})
	assert.NoError(t, err)

	assert.Len(t, resp.positions, 2, "TrackNewOrder should ignore duplicate order ID when already tracked")

	resp.positions[0].status = order.Closed
	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  exch,
		Pair:      pair,
		AssetType: asset.USDTMarginedFutures,
		Side:      order.Long,
		OrderID:   "5",
		Amount:    2,
	})
	assert.ErrorIs(t, err, errAssetMismatch)

	err = resp.TrackNewOrder(nil)
	assert.ErrorIs(t, err, common.ErrNilPointer)

	resp = nil
	err = resp.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  exch,
		Pair:      pair,
		AssetType: asset.USDTMarginedFutures,
		Side:      order.Long,
		OrderID:   "5",
		Amount:    2,
	})
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestSetupPositionControllerReal(t *testing.T) {
	t.Parallel()
	pc := SetupPositionController()
	assert.NotNil(t, pc.multiPositionTrackers, "SetupPositionController should initialise tracker map")
}

func TestPositionControllerTestTrackNewOrder(t *testing.T) {
	t.Parallel()
	pc := SetupPositionController()
	err := pc.TrackNewOrder(nil)
	assert.ErrorIs(t, err, errNilOrder)

	err = pc.TrackNewOrder(&order.Detail{
		Date:      time.Now(),
		Exchange:  "hi",
		Pair:      currency.NewBTCUSDT(),
		AssetType: asset.Spot,
		Side:      order.Long,
		OrderID:   "lol",
	})
	assert.ErrorIs(t, err, ErrNotFuturesAsset)

	err = pc.TrackNewOrder(&order.Detail{
		Date:      time.Now(),
		Pair:      currency.NewBTCUSDT(),
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
	})
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	err = pc.TrackNewOrder(&order.Detail{
		Exchange:  testExchange,
		Date:      time.Now(),
		Pair:      currency.NewBTCUSDT(),
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
	})
	assert.NoError(t, err)

	var pcp *PositionController
	err = pcp.TrackNewOrder(nil)
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestGetLatestPNLSnapshot(t *testing.T) {
	t.Parallel()
	pt := PositionTracker{}
	_, err := pt.GetLatestPNLSnapshot()
	assert.ErrorIs(t, err, errNoPNLHistory)

	pnl := PNLResult{
		Time:                  time.Now(),
		UnrealisedPNL:         decimal.NewFromInt(1337),
		RealisedPNLBeforeFees: decimal.NewFromInt(1337),
	}
	pt.pnlHistory = append(pt.pnlHistory, pnl)

	result, err := pt.GetLatestPNLSnapshot()
	assert.NoError(t, err)

	assert.Equal(t, pt.pnlHistory[0], result, "GetLatestPNLSnapshot should return most recent entry")
}

func TestGetRealisedPNL(t *testing.T) {
	t.Parallel()
	p := PositionTracker{}
	result := p.GetRealisedPNL()
	assert.True(t, result.IsZero(), "GetRealisedPNL should return zero for empty tracker")
}

func TestGetStats(t *testing.T) {
	t.Parallel()

	p := &PositionTracker{}
	stats := p.GetStats()
	assert.Empty(t, stats.Orders, "GetStats should return empty orders for new tracker")

	p.exchange = testExchange
	p.fundingRateDetails = &fundingrate.HistoricalRates{
		FundingRates: []fundingrate.Rate{
			{},
		},
	}

	stats = p.GetStats()
	assert.Equal(t, p.exchange, stats.Exchange, "GetStats should include exchange")

	p = nil
	stats = p.GetStats()
	assert.Nil(t, stats, "GetStats should return nil for nil tracker")
}

func TestGetPositions(t *testing.T) {
	t.Parallel()
	p := &MultiPositionTracker{}
	positions := p.GetPositions()
	assert.Empty(t, positions, "GetPositions should return empty slice when uninitialised")

	p.positions = append(p.positions, &PositionTracker{
		exchange: testExchange,
	})
	positions = p.GetPositions()
	assert.Len(t, positions, 1, "GetPositions should return one entry after append")
	assert.Equal(t, testExchange, positions[0].Exchange, "GetPositions should retain exchange name")

	p = nil
	positions = p.GetPositions()
	assert.Empty(t, positions, "GetPositions should return empty slice for nil tracker")
}

func TestGetPositionsForExchange(t *testing.T) {
	t.Parallel()
	c := &PositionController{}
	p := currency.NewBTCUSDT()

	_, err := c.GetPositionsForExchange("", asset.Futures, p)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	pos, err := c.GetPositionsForExchange(testExchange, asset.Futures, p)
	assert.ErrorIs(t, err, ErrPositionNotFound)

	assert.Empty(t, pos, "GetPositionsForExchange should return empty slice when tracker missing")
	c.multiPositionTrackers = make(map[key.ExchangeAssetPair]*MultiPositionTracker)
	c.multiPositionTrackers[key.NewExchangeAssetPair(testExchange, asset.Futures, p)] = nil
	_, err = c.GetPositionsForExchange(testExchange, asset.Futures, p)
	require.ErrorIs(t, err, ErrPositionNotFound, "GetPositionsForExchange must return ErrPositionNotFound")

	c.multiPositionTrackers[key.NewExchangeAssetPair(testExchange, asset.Futures, p)] = nil
	_, err = c.GetPositionsForExchange(testExchange, asset.Futures, p)
	assert.ErrorIs(t, err, ErrPositionNotFound)

	_, err = c.GetPositionsForExchange(testExchange, asset.Spot, p)
	assert.ErrorIs(t, err, ErrNotFuturesAsset)

	c.multiPositionTrackers[key.NewExchangeAssetPair(testExchange, asset.Futures, p)] = &MultiPositionTracker{
		exchange: testExchange,
	}

	pos, err = c.GetPositionsForExchange(testExchange, asset.Futures, p)
	assert.NoError(t, err)
	assert.Empty(t, pos, "GetPositionsForExchange should return empty slice when tracker has no positions")
	c.multiPositionTrackers[key.NewExchangeAssetPair(testExchange, asset.Futures, p)] = &MultiPositionTracker{
		exchange: testExchange,
		positions: []*PositionTracker{
			{
				exchange: testExchange,
			},
		},
	}
	pos, err = c.GetPositionsForExchange(testExchange, asset.Futures, p)
	assert.NoError(t, err)
	assert.Len(t, pos, 1, "GetPositionsForExchange should return single position when present")
	assert.Equal(t, testExchange, pos[0].Exchange, "GetPositionsForExchange should preserve exchange name")
	c = nil
	_, err = c.GetPositionsForExchange(testExchange, asset.Futures, p)
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestClearPositionsForExchange(t *testing.T) {
	t.Parallel()
	c := &PositionController{}
	p := currency.NewBTCUSDT()
	err := c.ClearPositionsForExchange("", asset.Futures, p)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	err = c.ClearPositionsForExchange(testExchange, asset.Futures, p)
	assert.ErrorIs(t, err, ErrPositionNotFound)

	c.multiPositionTrackers = make(map[key.ExchangeAssetPair]*MultiPositionTracker)
	err = c.ClearPositionsForExchange(testExchange, asset.Futures, p)
	assert.ErrorIs(t, err, ErrPositionNotFound)

	err = c.ClearPositionsForExchange(testExchange, asset.Spot, p)
	assert.ErrorIs(t, err, ErrNotFuturesAsset)

	c.multiPositionTrackers[key.NewExchangeAssetPair(testExchange, asset.Futures, p)] = &MultiPositionTracker{
		exchange:   testExchange,
		underlying: currency.DOGE,
		positions: []*PositionTracker{
			{
				exchange: testExchange,
			},
		},
	}
	err = c.ClearPositionsForExchange(testExchange, asset.Futures, p)
	require.NoError(t, err, "ClearPositionsForExchange must not error")
	assert.Empty(t, c.multiPositionTrackers[key.NewExchangeAssetPair(testExchange, asset.Futures, p)].positions, "ClearPositionsForExchange should remove positions")
	c = nil
	_, err = c.GetPositionsForExchange(testExchange, asset.Futures, p)
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestCalculateRealisedPNL(t *testing.T) {
	t.Parallel()
	result := calculateRealisedPNL(nil)
	assert.True(t, result.IsZero(), "calculateRealisedPNL should return zero for nil results")
	result = calculateRealisedPNL([]PNLResult{
		{
			IsOrder:               true,
			RealisedPNLBeforeFees: decimal.NewFromInt(1337),
		},
	})
	assert.True(t, result.Equal(decimal.NewFromInt(1337)), "calculateRealisedPNL should equal 1337 when fees absent")

	result = calculateRealisedPNL([]PNLResult{
		{
			IsOrder:               true,
			RealisedPNLBeforeFees: decimal.NewFromInt(1339),
			Fee:                   decimal.NewFromInt(2),
		},
		{
			IsOrder:               true,
			RealisedPNLBeforeFees: decimal.NewFromInt(2),
			Fee:                   decimal.NewFromInt(2),
		},
	})
	assert.True(t, result.Equal(decimal.NewFromInt(1337)), "calculateRealisedPNL should subtract fees to 1337 total")
}

func TestSetupPositionTracker(t *testing.T) {
	t.Parallel()
	p, err := SetupPositionTracker(nil)
	assert.ErrorIs(t, err, errNilSetup)
	assert.Nil(t, p, "SetupPositionTracker should return nil tracker for nil setup")
	p, err = SetupPositionTracker(&PositionTrackerSetup{
		Asset: asset.Spot,
	})
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	assert.Nil(t, p, "SetupPositionTracker should return nil tracker when exchange missing")

	p, err = SetupPositionTracker(&PositionTrackerSetup{
		Exchange: testExchange,
		Asset:    asset.Spot,
	})
	assert.ErrorIs(t, err, ErrNotFuturesAsset)

	assert.Nil(t, p, "SetupPositionTracker should return nil tracker when asset not futures")

	p, err = SetupPositionTracker(&PositionTrackerSetup{
		Exchange: testExchange,
		Asset:    asset.Futures,
	})
	assert.ErrorIs(t, err, order.ErrPairIsEmpty)

	assert.Nil(t, p, "SetupPositionTracker should return nil tracker when pair missing")

	cp := currency.NewBTCUSDT()
	p, err = SetupPositionTracker(&PositionTrackerSetup{
		Exchange: testExchange,
		Asset:    asset.Futures,
		Pair:     cp,
	})
	require.NoError(t, err)

	require.NotNil(t, p, "SetupPositionTracker must return tracker when setup valid")
	assert.Equal(t, testExchange, p.exchange, "SetupPositionTracker should set exchange")

	_, err = SetupPositionTracker(&PositionTrackerSetup{
		Exchange:                  testExchange,
		Asset:                     asset.Futures,
		Pair:                      cp,
		UseExchangePNLCalculation: true,
	})
	assert.ErrorIs(t, err, ErrNilPNLCalculator)

	p, err = SetupPositionTracker(&PositionTrackerSetup{
		Exchange:                  testExchange,
		Asset:                     asset.Futures,
		Pair:                      cp,
		UseExchangePNLCalculation: true,
		PNLCalculator:             &PNLCalculator{},
	})
	assert.NoError(t, err)

	assert.True(t, p.useExchangePNLCalculation, "SetupPositionTracker should enable exchange PNL when configured")
}

func TestCalculatePNL(t *testing.T) {
	t.Parallel()
	p := &PNLCalculator{}
	_, err := p.CalculatePNL(t.Context(), nil)
	assert.ErrorIs(t, err, ErrNilPNLCalculator)

	_, err = p.CalculatePNL(t.Context(), &PNLCalculatorRequest{})
	assert.ErrorIs(t, err, errCannotCalculateUnrealisedPNL)

	_, err = p.CalculatePNL(t.Context(),
		&PNLCalculatorRequest{
			OrderDirection:   order.Short,
			CurrentDirection: order.Long,
		})
	assert.ErrorIs(t, err, errCannotCalculateUnrealisedPNL)
}

func TestTrackPNLByTime(t *testing.T) {
	t.Parallel()
	p := &PositionTracker{}
	err := p.TrackPNLByTime(time.Now(), 1)
	assert.NoError(t, err)

	err = p.TrackPNLByTime(time.Now(), 2)
	assert.NoError(t, err)

	assert.True(t, p.latestPrice.Equal(decimal.NewFromInt(2)), "TrackPNLByTime should update latestPrice to 2")
	p = nil
	err = p.TrackPNLByTime(time.Now(), 2)
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestUpdateOpenPositionUnrealisedPNL(t *testing.T) {
	t.Parallel()
	pc := SetupPositionController()

	_, err := pc.UpdateOpenPositionUnrealisedPNL("", asset.Futures, currency.NewBTCUSDT(), 2, time.Now())
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	_, err = pc.UpdateOpenPositionUnrealisedPNL("hi", asset.Futures, currency.NewBTCUSDT(), 2, time.Now())
	assert.ErrorIs(t, err, ErrPositionNotFound)

	_, err = pc.UpdateOpenPositionUnrealisedPNL("hi", asset.Spot, currency.NewBTCUSDT(), 2, time.Now())
	assert.ErrorIs(t, err, ErrNotFuturesAsset)

	err = pc.TrackNewOrder(&order.Detail{
		Date:      time.Now(),
		Exchange:  "hi",
		Pair:      currency.NewBTCUSDT(),
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1,
		Amount:    1,
	})
	assert.NoError(t, err)

	_, err = pc.UpdateOpenPositionUnrealisedPNL("hi2", asset.Futures, currency.NewBTCUSDT(), 2, time.Now())
	assert.ErrorIs(t, err, ErrPositionNotFound)

	_, err = pc.UpdateOpenPositionUnrealisedPNL("hi", asset.PerpetualSwap, currency.NewBTCUSDT(), 2, time.Now())
	assert.ErrorIs(t, err, ErrPositionNotFound)

	_, err = pc.UpdateOpenPositionUnrealisedPNL("hi", asset.Futures, currency.NewPair(currency.BTC, currency.DOGE), 2, time.Now())
	assert.ErrorIs(t, err, ErrPositionNotFound)

	pnl, err := pc.UpdateOpenPositionUnrealisedPNL("hi", asset.Futures, currency.NewBTCUSDT(), 2, time.Now())
	assert.NoError(t, err)

	assert.True(t, pnl.Equal(decimal.NewFromInt(1)), "UpdateOpenPositionUnrealisedPNL should return 1 unrealised pnl")

	var nilPC *PositionController
	_, err = nilPC.UpdateOpenPositionUnrealisedPNL("hi", asset.Futures, currency.NewBTCUSDT(), 2, time.Now())
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestSetCollateralCurrency(t *testing.T) {
	t.Parallel()
	pc := SetupPositionController()
	err := pc.SetCollateralCurrency("", asset.Spot, currency.EMPTYPAIR, currency.Code{})
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	err = pc.SetCollateralCurrency("hi", asset.Spot, currency.EMPTYPAIR, currency.Code{})
	assert.ErrorIs(t, err, ErrNotFuturesAsset)

	p := currency.NewBTCUSDT()
	pc.multiPositionTrackers = make(map[key.ExchangeAssetPair]*MultiPositionTracker)
	err = pc.SetCollateralCurrency("hi", asset.Futures, p, currency.DOGE)
	require.ErrorIs(t, err, ErrPositionNotFound)

	err = pc.SetCollateralCurrency("hi", asset.Futures, p, currency.DOGE)
	require.ErrorIs(t, err, ErrPositionNotFound)

	mapKey := key.NewExchangeAssetPair("hi", asset.Futures, p)
	pc.multiPositionTrackers[mapKey] = &MultiPositionTracker{
		exchange:       "hi",
		asset:          asset.Futures,
		pair:           p,
		orderPositions: make(map[string]*PositionTracker),
	}
	err = pc.TrackNewOrder(&order.Detail{
		Date:      time.Now(),
		Exchange:  "hi",
		Pair:      p,
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1,
		Amount:    1,
	})
	require.NoError(t, err)

	err = pc.SetCollateralCurrency("hi", asset.Futures, p, currency.DOGE)
	require.NoError(t, err)

	assert.True(t, pc.multiPositionTrackers[mapKey].collateralCurrency.Equal(currency.DOGE), "SetCollateralCurrency should update tracker collateral")
	assert.True(t, pc.multiPositionTrackers[mapKey].positions[0].collateralCurrency.Equal(currency.DOGE), "SetCollateralCurrency should update position collateral")

	var nilPC *PositionController
	err = nilPC.SetCollateralCurrency("hi", asset.Spot, currency.EMPTYPAIR, currency.Code{})
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestMPTUpdateOpenPositionUnrealisedPNL(t *testing.T) {
	t.Parallel()
	p := currency.NewBTCUSDT()
	pc := SetupPositionController()
	err := pc.TrackNewOrder(&order.Detail{
		Date:      time.Now(),
		Exchange:  "hi",
		Pair:      p,
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1,
		Amount:    1,
	})
	require.NoError(t, err)

	mapKey := key.NewExchangeAssetPair("hi", asset.Futures, p)
	result, err := pc.multiPositionTrackers[mapKey].UpdateOpenPositionUnrealisedPNL(1337, time.Now())
	require.NoError(t, err)

	assert.False(t, result.Equal(decimal.NewFromInt(1337)), "UpdateOpenPositionUnrealisedPNL should adjust unrealised value")

	pc.multiPositionTrackers[mapKey].positions[0].status = order.Closed
	_, err = pc.multiPositionTrackers[mapKey].UpdateOpenPositionUnrealisedPNL(1337, time.Now())
	require.ErrorIs(t, err, ErrPositionClosed)

	pc.multiPositionTrackers[mapKey].positions = nil
	_, err = pc.multiPositionTrackers[mapKey].UpdateOpenPositionUnrealisedPNL(1337, time.Now())
	require.ErrorIs(t, err, ErrPositionNotFound)
}

func TestMPTLiquidate(t *testing.T) {
	t.Parallel()
	item := asset.Futures
	pair, err := currency.NewPairFromStrings("BTC", "1231")
	assert.NoError(t, err)

	e := &MultiPositionTracker{
		exchange:               testExchange,
		exchangePNLCalculation: &FakePNL{},
		asset:                  item,
		orderPositions:         make(map[string]*PositionTracker),
	}

	err = e.Liquidate(decimal.Zero, time.Time{})
	assert.ErrorIs(t, err, ErrPositionNotFound)

	setup := &PositionTrackerSetup{
		Pair:  pair,
		Asset: item,
	}
	_, err = SetupPositionTracker(setup)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	setup.Exchange = "exch"
	_, err = SetupPositionTracker(setup)
	assert.NoError(t, err)

	tt := time.Now()
	err = e.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  testExchange,
		Pair:      pair,
		AssetType: item,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1,
		Amount:    1,
	})
	assert.NoError(t, err)

	err = e.Liquidate(decimal.Zero, time.Time{})
	assert.ErrorIs(t, err, order.ErrCannotLiquidate)

	err = e.Liquidate(decimal.Zero, tt)
	assert.NoError(t, err)

	assert.Equal(t, order.Liquidated, e.positions[0].status, "Liquidate should mark multi position tracker status")
	assert.True(t, e.positions[0].exposure.IsZero(), "Liquidate should zero multi position tracker exposure")

	e = nil
	err = e.Liquidate(decimal.Zero, tt)
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestPositionLiquidate(t *testing.T) {
	t.Parallel()
	item := asset.Futures
	pair, err := currency.NewPairFromStrings("BTC", "1231")
	assert.NoError(t, err)

	p := &PositionTracker{
		contractPair:     pair,
		asset:            item,
		exchange:         testExchange,
		PNLCalculation:   &PNLCalculator{},
		status:           order.Open,
		openingDirection: order.Long,
	}

	tt := time.Now()
	err = p.TrackNewOrder(&order.Detail{
		Date:      tt,
		Exchange:  testExchange,
		Pair:      pair,
		AssetType: item,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1,
		Amount:    1,
	}, false)
	assert.NoError(t, err)

	err = p.Liquidate(decimal.Zero, time.Time{})
	assert.ErrorIs(t, err, order.ErrCannotLiquidate)

	err = p.Liquidate(decimal.Zero, tt)
	assert.NoError(t, err)

	assert.Equal(t, order.Liquidated, p.status, "Liquidate should set position status")
	assert.True(t, p.exposure.IsZero(), "Liquidate should zero exposure")

	p = nil
	err = p.Liquidate(decimal.Zero, tt)
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestGetOpenPosition(t *testing.T) {
	t.Parallel()
	pc := SetupPositionController()
	cp := currency.NewPair(currency.BTC, currency.PERP)
	tn := time.Now()

	_, err := pc.GetOpenPosition("", asset.Futures, cp)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	_, err = pc.GetOpenPosition(testExchange, asset.Futures, cp)
	assert.ErrorIs(t, err, ErrPositionNotFound)

	err = pc.TrackNewOrder(&order.Detail{
		Date:      tn,
		Exchange:  testExchange,
		Pair:      cp,
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1337,
		Amount:    1337,
	})
	assert.NoError(t, err)

	_, err = pc.GetOpenPosition(testExchange, asset.Futures, cp)
	assert.NoError(t, err)
}

func TestGetAllOpenPositions(t *testing.T) {
	t.Parallel()
	pc := SetupPositionController()

	_, err := pc.GetAllOpenPositions()
	assert.ErrorIs(t, err, ErrNoPositionsFound)

	cp := currency.NewPair(currency.BTC, currency.PERP)
	tn := time.Now()
	err = pc.TrackNewOrder(&order.Detail{
		Date:      tn,
		Exchange:  testExchange,
		Pair:      cp,
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1337,
		Amount:    1337,
	})
	assert.NoError(t, err)

	_, err = pc.GetAllOpenPositions()
	assert.NoError(t, err)
}

func TestPCTrackFundingDetails(t *testing.T) {
	t.Parallel()
	pc := SetupPositionController()
	err := pc.TrackFundingDetails(nil)
	assert.ErrorIs(t, err, common.ErrNilPointer)

	p := currency.NewPair(currency.BTC, currency.PERP)
	rates := &fundingrate.HistoricalRates{
		Asset: asset.Futures,
		Pair:  p,
	}
	err = pc.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	rates.Exchange = testExchange
	err = pc.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, ErrPositionNotFound)

	tn := time.Now()
	err = pc.TrackNewOrder(&order.Detail{
		Date:      tn,
		Exchange:  testExchange,
		Pair:      p,
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1337,
		Amount:    1337,
	})
	assert.NoError(t, err)

	rates.StartDate = tn.Add(-time.Hour)
	rates.EndDate = tn
	rates.FundingRates = []fundingrate.Rate{
		{
			Time:    tn,
			Rate:    decimal.NewFromInt(1337),
			Payment: decimal.NewFromInt(1337),
		},
	}

	mapKey := key.NewExchangeAssetPair(testExchange, asset.Futures, p)
	pc.multiPositionTrackers[mapKey].orderPositions["lol"].openingDate = tn.Add(-time.Hour)
	pc.multiPositionTrackers[mapKey].orderPositions["lol"].lastUpdated = tn
	err = pc.TrackFundingDetails(rates)
	assert.NoError(t, err)
}

func TestMPTTrackFundingDetails(t *testing.T) {
	t.Parallel()
	mpt := &MultiPositionTracker{
		orderPositions: make(map[string]*PositionTracker),
	}

	err := mpt.TrackFundingDetails(nil)
	assert.ErrorIs(t, err, common.ErrNilPointer)

	cp := currency.NewPair(currency.BTC, currency.PERP)
	rates := &fundingrate.HistoricalRates{
		Asset: asset.Futures,
		Pair:  cp,
	}
	err = mpt.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	mpt.exchange = testExchange
	rates = &fundingrate.HistoricalRates{
		Exchange: testExchange,
		Asset:    asset.Futures,
		Pair:     cp,
	}
	err = mpt.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, errAssetMismatch)

	mpt.asset = rates.Asset
	mpt.pair = cp
	err = mpt.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, ErrPositionNotFound)

	tn := time.Now()
	err = mpt.TrackNewOrder(&order.Detail{
		Date:      tn,
		Exchange:  testExchange,
		Pair:      cp,
		AssetType: asset.Futures,
		Side:      order.Long,
		OrderID:   "lol",
		Price:     1337,
		Amount:    1337,
	})
	assert.NoError(t, err)

	rates.StartDate = tn.Add(-time.Hour)
	rates.EndDate = tn
	rates.FundingRates = []fundingrate.Rate{
		{
			Time:    tn,
			Rate:    decimal.NewFromInt(1337),
			Payment: decimal.NewFromInt(1337),
		},
	}
	mpt.orderPositions["lol"].openingDate = tn.Add(-time.Hour)
	mpt.orderPositions["lol"].lastUpdated = tn
	rates.Exchange = "lol"
	err = mpt.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, errExchangeNameMismatch)
}

func TestPTTrackFundingDetails(t *testing.T) {
	t.Parallel()
	p := &PositionTracker{}
	err := p.TrackFundingDetails(nil)
	assert.ErrorIs(t, err, common.ErrNilPointer)

	cp := currency.NewPair(currency.BTC, currency.PERP)
	rates := &fundingrate.HistoricalRates{
		Exchange: testExchange,
		Asset:    asset.Futures,
		Pair:     cp,
	}
	err = p.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, errDoesntMatch)

	p.exchange = testExchange
	p.asset = asset.Futures
	p.contractPair = cp
	err = p.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, common.ErrDateUnset)

	rates.StartDate = time.Now().Add(-time.Hour)
	rates.EndDate = time.Now()
	p.openingDate = rates.StartDate
	err = p.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, ErrNoPositionsFound)

	p.pnlHistory = append(p.pnlHistory, PNLResult{
		Time:                  rates.EndDate,
		UnrealisedPNL:         decimal.NewFromInt(1337),
		RealisedPNLBeforeFees: decimal.NewFromInt(1337),
		Price:                 decimal.NewFromInt(1337),
		Exposure:              decimal.NewFromInt(1337),
		Fee:                   decimal.NewFromInt(1337),
	})
	err = p.TrackFundingDetails(rates)
	assert.NoError(t, err)

	rates.FundingRates = []fundingrate.Rate{
		{
			Time:    rates.StartDate,
			Rate:    decimal.NewFromInt(1337),
			Payment: decimal.NewFromInt(1337),
		},
	}
	err = p.TrackFundingDetails(rates)
	assert.NoError(t, err)

	err = p.TrackFundingDetails(rates)
	assert.NoError(t, err)

	rates.StartDate = rates.StartDate.Add(-time.Hour)
	err = p.TrackFundingDetails(rates)
	assert.NoError(t, err)

	rates.Exchange = ""
	err = p.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	p = nil
	err = p.TrackFundingDetails(rates)
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestAreFundingRatePrerequisitesMet(t *testing.T) {
	t.Parallel()
	err := CheckFundingRatePrerequisites(false, false, false)
	assert.NoError(t, err)

	err = CheckFundingRatePrerequisites(true, false, false)
	assert.NoError(t, err)

	err = CheckFundingRatePrerequisites(true, true, false)
	assert.NoError(t, err)

	err = CheckFundingRatePrerequisites(true, true, true)
	assert.NoError(t, err)

	err = CheckFundingRatePrerequisites(true, false, true)
	assert.NoError(t, err)

	err = CheckFundingRatePrerequisites(false, false, true)
	assert.ErrorIs(t, err, ErrGetFundingDataRequired)

	err = CheckFundingRatePrerequisites(false, true, true)
	assert.ErrorIs(t, err, ErrGetFundingDataRequired)

	err = CheckFundingRatePrerequisites(false, true, false)
	assert.ErrorIs(t, err, ErrGetFundingDataRequired)
}

func TestLastUpdated(t *testing.T) {
	t.Parallel()
	p := &PositionController{}
	tm, err := p.LastUpdated()
	assert.NoError(t, err)

	assert.True(t, tm.IsZero(), "LastUpdated should be zero for new controller")
	p.updated = time.Now()
	tm, err = p.LastUpdated()
	assert.NoError(t, err)

	assert.Equal(t, p.updated, tm, "LastUpdated should return stored timestamp")
	p = nil
	_, err = p.LastUpdated()
	assert.ErrorIs(t, err, common.ErrNilPointer)
}

func TestGetCurrencyForRealisedPNL(t *testing.T) {
	p := PNLCalculator{}
	code, a, err := p.GetCurrencyForRealisedPNL(asset.Spot, currency.NewPair(currency.DOGE, currency.XRP))
	assert.NoError(t, err)
	assert.True(t, code.Equal(currency.DOGE), "GetCurrencyForRealisedPNL should return base currency")
	assert.Equal(t, asset.Spot, a, "GetCurrencyForRealisedPNL should return realised asset")
}

func TestCheckTrackerPrerequisitesLowerExchange(t *testing.T) {
	t.Parallel()
	_, err := checkTrackerPrerequisitesLowerExchange("", asset.Spot, currency.EMPTYPAIR)
	assert.ErrorIs(t, err, common.ErrExchangeNameNotSet)

	upperExch := "IM UPPERCASE"
	_, err = checkTrackerPrerequisitesLowerExchange(upperExch, asset.Spot, currency.EMPTYPAIR)
	assert.ErrorIs(t, err, ErrNotFuturesAsset)

	_, err = checkTrackerPrerequisitesLowerExchange(upperExch, asset.Futures, currency.EMPTYPAIR)
	assert.ErrorIs(t, err, order.ErrPairIsEmpty)

	lowerExch, err := checkTrackerPrerequisitesLowerExchange(upperExch, asset.Futures, currency.NewBTCUSDT())
	assert.NoError(t, err)

	assert.Equal(t, "im uppercase", lowerExch, "checkTrackerPrerequisitesLowerExchange should lowercase exchange")
}
