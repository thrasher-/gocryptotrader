package kline

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/database/repository/candle"
	"github.com/thrasher-corp/gocryptotrader/database/repository/exchange"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// LoadFromDatabase returns Item from database seeded data
func LoadFromDatabase(exchange string, pair currency.Pair, interval Interval, start, end time.Time) (Item, error) {
	retCandle, err := candle.Series(exchange,
		pair.Base.String(), pair.Quote.String(),
		interval.Short(), start, end)
	if err != nil {
		return Item{}, err
	}

	ret := Item{
		Exchange: exchange,
		Pair:     pair,
		Interval: interval,
	}

	for x := range retCandle.Tick {
		ret.Candles = append(ret.Candles, Candle{
			Time:   retCandle.Tick[x].Timestamp,
			Open:   retCandle.Tick[x].Open,
			High:   retCandle.Tick[x].High,
			Low:    retCandle.Tick[x].Low,
			Close:  retCandle.Tick[x].Close,
			Volume: retCandle.Tick[x].Volume,
		})
	}
	return ret, nil
}

// StoreInDatabase returns Item from database seeded data
func StoreInDatabase(in *Item) error {
	if in.Exchange == "" {
		return errors.New("name cannot be blank")
	}

	exchangeUUID, err := exchange.UUIDByName(in.Exchange)
	if err != nil {
		return err
	}

	databaseCandles := candle.Candle{
		ExchangeID: exchangeUUID.String(),
		Base:       in.Pair.Base.Upper().String(),
		Quote:      in.Pair.Quote.Upper().String(),
		Interval:   in.Interval.Short(),
		Asset:      in.Asset.String(),
	}

	for x := range in.Candles {
		databaseCandles.Tick = append(databaseCandles.Tick, candle.Tick{
			Timestamp: in.Candles[x].Time,
			Open:      in.Candles[x].Open,
			High:      in.Candles[x].High,
			Low:       in.Candles[x].Low,
			Close:     in.Candles[x].Close,
			Volume:    in.Candles[x].Volume,
		})
	}
	return candle.Insert(&databaseCandles)
}

// LoadFromGCTScriptCSV loads kline data from a CSV file
func LoadFromGCTScriptCSV(file string) (out []Candle, errRet error) {
	csvFile, err := os.Open(file)
	if err != nil {
		return out, err
	}

	defer func() {
		err = csvFile.Close()
		if err != nil {
			log.Errorln(log.Global, err)
		}
	}()

	csvData := csv.NewReader(csvFile)

	for {
		row, errCSV := csvData.Read()
		if errCSV != nil {
			if errCSV == io.EOF {
				errCSV = nil
			}
			return out, errCSV
		}

		tempCandle := Candle{}
		v, errParse := strconv.ParseInt(row[0], 10, 32)
		if errParse != nil {
			return out, errParse
		}
		tempCandle.Time = time.Unix(v, 0).UTC()
		if tempCandle.Time.IsZero() {
			err = fmt.Errorf("invalid timestamp received on row %v", row)
			break
		}

		tempCandle.Volume, err = strconv.ParseFloat(row[1], 64)
		if err != nil {
			break
		}

		tempCandle.Open, err = strconv.ParseFloat(row[2], 64)
		if err != nil {
			break
		}

		tempCandle.High, err = strconv.ParseFloat(row[3], 64)
		if err != nil {
			break
		}

		tempCandle.Low, err = strconv.ParseFloat(row[4], 64)
		if err != nil {
			break
		}

		tempCandle.Close, err = strconv.ParseFloat(row[5], 64)
		if err != nil {
			break
		}
		out = append(out, tempCandle)
	}
	return out, err
}