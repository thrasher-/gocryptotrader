package report

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/backtester/common"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// GenerateReport sends final data from statistics to a template
// to create a lovely final report for someone to view
func (d *Data) GenerateReport() error {
	log.Info(common.Report, "generating report")
	err := d.enhanceCandles()
	if err != nil {
		return err
	}
	for i := range d.OriginalCandles {
		for j := range d.OriginalCandles[i].Candles {
			if d.OriginalCandles[i].Candles[j].ValidationIssues == "" {
				continue
			}
			d.Warnings = append(d.Warnings, Warning{
				Exchange: d.OriginalCandles[i].Exchange,
				Asset:    d.OriginalCandles[i].Asset,
				Pair:     d.OriginalCandles[i].Pair,
				Message:  fmt.Sprintf("candle data %v", d.OriginalCandles[i].Candles[j].ValidationIssues),
			})
		}
	}
	for i := range d.EnhancedCandles {
		if len(d.EnhancedCandles[i].Candles) >= maxChartLimit {
			d.EnhancedCandles[i].IsOverLimit = true
			d.EnhancedCandles[i].Candles = d.EnhancedCandles[i].Candles[:maxChartLimit]
		}
	}

	if d.Statistics.FundingStatistics != nil {
		d.HoldingsOverTimeChart, err = createHoldingsOverTimeChart(d.Statistics.FundingStatistics.Items)
		if err != nil {
			return err
		}
		if !d.Statistics.FundingStatistics.Report.DisableUSDTracking {
			d.USDTotalsChart, err = createUSDTotalsChart(d.Statistics.FundingStatistics.TotalUSDStatistics.HoldingValues, d.Statistics.FundingStatistics.Items)
			if err != nil {
				return err
			}
		}
	}

	if d.Statistics.HasCollateral {
		d.PNLOverTimeChart, err = createPNLCharts(d.Statistics.ExchangeAssetPairStatistics)
		if err != nil {
			return err
		}
		d.FuturesSpotDiffChart, err = createFuturesSpotDiffChart(d.Statistics.ExchangeAssetPairStatistics)
		if err != nil {
			return err
		}
	}
	tmpl := template.Must(
		template.ParseFiles(d.TemplatePath),
	)
	fn := d.Config.Nickname
	if fn != "" {
		fn += "-"
	}
	fn += d.Statistics.StrategyName + "-"
	fn += time.Now().Format("2006-01-02-15-04-05")

	fileName, err := common.GenerateFileName(fn, "html")
	if err != nil {
		return err
	}
	var f *os.File
	f, err = os.Create(
		filepath.Join(d.OutputPath,
			fileName,
		),
	)
	if err != nil {
		return err
	}
	defer func() {
		err = f.Close()
		if err != nil {
			log.Error(common.Report, err)
		}
	}()

	err = tmpl.Execute(f, d)
	if err != nil {
		return err
	}
	log.Infof(common.Report, "successfully saved report to %v", filepath.Join(d.OutputPath, fileName))
	return nil
}

// AddKlineItem appends a SET of candles for the report to enhance upon
// generation
func (d *Data) AddKlineItem(k *kline.Item) {
	d.OriginalCandles = append(d.OriginalCandles, k)
}

// UpdateItem updates an existing kline item for LIVE data usage
func (d *Data) UpdateItem(k *kline.Item) {
	if len(d.OriginalCandles) == 0 {
		d.OriginalCandles = append(d.OriginalCandles, k)
	} else {
		d.OriginalCandles[0].Candles = append(d.OriginalCandles[0].Candles, k.Candles...)
		d.OriginalCandles[0].RemoveDuplicates()
	}
}

// enhanceCandles will enhance candle data with order information allowing
// report charts to have annotations to highlight buy and sell events
func (d *Data) enhanceCandles() error {
	if len(d.OriginalCandles) == 0 {
		return errNoCandles
	}
	if d.Statistics == nil {
		return errStatisticsUnset
	}
	d.Statistics.RiskFreeRate = d.Statistics.RiskFreeRate.Mul(decimal.NewFromInt(100))

	for intVal := range d.OriginalCandles {
		lookup := d.OriginalCandles[intVal]
		enhancedKline := EnhancedKline{
			Exchange:  lookup.Exchange,
			Asset:     lookup.Asset,
			Pair:      lookup.Pair,
			Interval:  lookup.Interval,
			Watermark: fmt.Sprintf("%s - %s - %s", strings.Title(lookup.Exchange), lookup.Asset.String(), lookup.Pair.Upper()), //nolint // Title usage
		}

		statsForCandles :=
			d.Statistics.ExchangeAssetPairStatistics[lookup.Exchange][lookup.Asset][lookup.Pair]
		if statsForCandles == nil {
			continue
		}

		requiresIteration := false
		if len(statsForCandles.Events) != len(d.OriginalCandles[intVal].Candles) {
			requiresIteration = true
		}
		for j := range d.OriginalCandles[intVal].Candles {
			_, offset := time.Now().Zone()
			tt := d.OriginalCandles[intVal].Candles[j].Time.Add(time.Duration(offset) * time.Second)
			enhancedCandle := DetailedCandle{
				UnixMilli:    tt.UTC().UnixMilli(),
				Open:         d.OriginalCandles[intVal].Candles[j].Open,
				High:         d.OriginalCandles[intVal].Candles[j].High,
				Low:          d.OriginalCandles[intVal].Candles[j].Low,
				Close:        d.OriginalCandles[intVal].Candles[j].Close,
				Volume:       d.OriginalCandles[intVal].Candles[j].Volume,
				VolumeColour: "rgba(50, 204, 30, 0.5)",
			}
			if j != 0 {
				if d.OriginalCandles[intVal].Candles[j].Close < d.OriginalCandles[intVal].Candles[j-1].Close {
					enhancedCandle.VolumeColour = "rgba(232, 3, 3, 0.5)"
				}
			}
			if !requiresIteration {
				if statsForCandles.Events[intVal].Time.Equal(d.OriginalCandles[intVal].Candles[j].Time) &&
					(statsForCandles.Events[intVal].SignalEvent == nil || statsForCandles.Events[intVal].SignalEvent.GetDirection() == order.MissingData) &&
					len(enhancedKline.Candles) > 0 {
					enhancedCandle.copyCloseFromPreviousEvent(&enhancedKline)
				}
			} else {
				for k := range statsForCandles.Events {
					if statsForCandles.Events[k].SignalEvent.GetTime().Equal(d.OriginalCandles[intVal].Candles[j].Time) &&
						statsForCandles.Events[k].SignalEvent.GetDirection() == order.MissingData &&
						len(enhancedKline.Candles) > 0 {
						enhancedCandle.copyCloseFromPreviousEvent(&enhancedKline)
					}
				}
			}
			for k := range statsForCandles.FinalOrders.Orders {
				if statsForCandles.FinalOrders.Orders[k].Order == nil ||
					!statsForCandles.FinalOrders.Orders[k].Order.Date.Equal(d.OriginalCandles[intVal].Candles[j].Time) {
					continue
				}
				// an order was placed here, can enhance chart!
				enhancedCandle.MadeOrder = true
				enhancedCandle.OrderAmount = decimal.NewFromFloat(statsForCandles.FinalOrders.Orders[k].Order.Amount)
				enhancedCandle.PurchasePrice = statsForCandles.FinalOrders.Orders[k].Order.Price
				enhancedCandle.OrderDirection = statsForCandles.FinalOrders.Orders[k].Order.Side
				if enhancedCandle.OrderDirection == order.Buy {
					enhancedCandle.Colour = "green"
					enhancedCandle.Position = "aboveBar"
					enhancedCandle.Shape = "arrowDown"
				} else if enhancedCandle.OrderDirection == order.Sell {
					enhancedCandle.Colour = "red"
					enhancedCandle.Position = "belowBar"
					enhancedCandle.Shape = "arrowUp"
				}
				enhancedCandle.Text = enhancedCandle.OrderDirection.String()
				break
			}
			enhancedKline.Candles = append(enhancedKline.Candles, enhancedCandle)
		}
		d.EnhancedCandles = append(d.EnhancedCandles, enhancedKline)
	}

	return nil
}

func (d *DetailedCandle) copyCloseFromPreviousEvent(ek *EnhancedKline) {
	// if the data is missing, ensure that all values just continue the previous candle's close price visually
	d.Open = ek.Candles[len(ek.Candles)-1].Close
	d.High = ek.Candles[len(ek.Candles)-1].Close
	d.Low = ek.Candles[len(ek.Candles)-1].Close
	d.Close = ek.Candles[len(ek.Candles)-1].Close
	d.Colour = "white"
	d.Position = "aboveBar"
	d.Shape = "arrowDown"
	d.Text = order.MissingData.String()
}

// UseDarkMode sets whether to use a dark theme by default
// for the html generated report
func (d *Data) UseDarkMode(use bool) {
	d.UseDarkTheme = use
}
