package binance

import (
	"bytes"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

// timeString gets the time as Binance timestamp
func timeString(t time.Time) string {
	return strconv.FormatInt(t.UnixMilli(), 10)
}

// UnmarshalJSON deserialises either a single PriceChangeStats object or an array of them.
func (a *PriceChanges) UnmarshalJSON(data []byte) error {
	var resp []*PriceChangeStats
	err := json.Unmarshal(data, &resp)
	if err != nil {
		var singleResp *PriceChangeStats
		err = json.Unmarshal(data, &singleResp)
		if err != nil {
			return err
		}
		*a = []*PriceChangeStats{singleResp}
	} else {
		*a = resp
	}
	return nil
}

// UnmarshalJSON deserialises the data to unmarshal into SymbolTickerItem or []SymbolTickerItem
func (a *SymbolTickers) UnmarshalJSON(data []byte) error {
	var resp []*SymbolTickerItem
	err := json.Unmarshal(data, &resp)
	if err != nil {
		var singleResp *SymbolTickerItem
		err = json.Unmarshal(data, &singleResp)
		if err != nil {
			return err
		}
		*a = []*SymbolTickerItem{singleResp}
	} else {
		*a = resp
	}
	return nil
}

// UnmarshalJSON deserialises the data to unmarshal into WsOrderbookTicker or []WsOrderbookTicker
func (a *WsOrderbookTickers) UnmarshalJSON(data []byte) error {
	var resp []*WsOrderbookTicker
	err := json.Unmarshal(data, &resp)
	if err != nil {
		var singleResp *WsOrderbookTicker
		err = json.Unmarshal(data, &singleResp)
		if err != nil {
			return err
		}
		*a = []*WsOrderbookTicker{singleResp}
	} else {
		*a = resp
	}
	return nil
}

// UnmarshalJSON deserialises incoming object or slice into WsOptionIncomingResponses([]WsOptionIncomingResponse) instance.
func (a *WsOptionIncomingResponses) UnmarshalJSON(data []byte) error {
	var resp []*WsOptionIncomingResponse
	isSlice := true
	if err := json.Unmarshal(data, &resp); err != nil {
		isSlice = false
		var newResp *WsOptionIncomingResponse
		err = json.Unmarshal(data, &newResp)
		if err != nil {
			return err
		}
		resp = append(resp, newResp)
	}
	a.Instances = resp
	a.IsSlice = isSlice
	return nil
}

// UnmarshalJSON unmarshals a []byte data in an object or array form to AssetIndexResponse([]AssetIndex) instance.
func (a *AssetIndexResponse) UnmarshalJSON(data []byte) error {
	var resp []*AssetIndex
	if err := json.Unmarshal(data, &resp); err != nil {
		resp = make([]*AssetIndex, 1)
		err = json.Unmarshal(data, &resp[0])
		if err != nil {
			return err
		}
	}
	*a = resp
	return nil
}

// UnmarshalJSON unmarshals a []byte data in an object or array form to AccountBalanceResponse([]AccountBalance) instance.
func (a *AccountBalanceResponse) UnmarshalJSON(data []byte) error {
	var resp []AccountBalance
	err := json.Unmarshal(data, &resp)
	if err != nil {
		resp = make([]AccountBalance, 1)
		err := json.Unmarshal(data, &resp[0])
		if err != nil {
			return err
		}
	}
	*a = resp
	return nil
}

// UnmarshalJSON deserialises either a single insurance fund object, which is what the endpoint
// returns when a symbol is supplied, or an array of them when it is not.
func (a *UFuturesInsuranceBalances) UnmarshalJSON(data []byte) error {
	var resp []*UFuturesInsuranceBalance
	if err := json.Unmarshal(data, &resp); err != nil {
		var single *UFuturesInsuranceBalance
		if err := json.Unmarshal(data, &single); err != nil {
			return err
		}
		*a = []*UFuturesInsuranceBalance{single}
		return nil
	}
	*a = resp
	return nil
}

// UnmarshalJSON decodes single-symbol and multiple-symbol price responses.
func (a *SymbolPrices) UnmarshalJSON(data []byte) error {
	return unmarshalObjectOrArray(data, (*[]*SymbolPrice)(a))
}

// UnmarshalJSON decodes single-symbol and multiple-symbol best price responses.
func (a *BestPrices) UnmarshalJSON(data []byte) error {
	return unmarshalObjectOrArray(data, (*[]*BestPrice)(a))
}

func unmarshalObjectOrArray[T any](data []byte, target *[]*T) error {
	data = bytes.TrimSpace(data)
	if len(data) != 0 && data[0] == '{' {
		var item *T
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		*target = []*T{item}
		return nil
	}
	return json.Unmarshal(data, target)
}

// UnmarshalJSON decodes one symbol or all symbols' ADL risks.
func (a *UFuturesSymbolADLRisks) UnmarshalJSON(data []byte) error {
	return unmarshalObjectOrArray(data, (*[]*UFuturesSymbolADLRisk)(a))
}
