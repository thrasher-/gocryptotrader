package kraken

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var assetTranslator assetTranslatorStore

var (
	errAssetVersionInvalid    = errors.New("asset version must be 1 when set")
	errDepthCountInvalid      = errors.New("order book count must be omitted or between 1 and 500")
	errExecutionVenueInvalid  = errors.New("execution venue must be international or bitnomial_exchange")
	errGroupedDepthInvalid    = errors.New("grouped order book depth must be omitted or 10, 25, 100, 250, or 1000")
	errGroupingInvalid        = errors.New("order book grouping must be omitted or 1, 5, 10, 25, 50, 100, 250, 500, or 1000")
	errInfoInvalid            = errors.New("asset pair info must be info, leverage, fees, or margin")
	errIntervalInvalid        = errors.New("OHLC interval is invalid")
	errLevel3DepthInvalid     = errors.New("level 3 order book depth must be 0, 10, 25, 100, 250, or 1000")
	errPostTradeCountTooLarge = errors.New("post-trade count cannot exceed 1000")
	errSinceCursorConflict    = errors.New("since timestamp and cursor are mutually exclusive")
	errSymbolLengthInvalid    = errors.New("symbol must contain between 3 and 32 characters")
	errSymbolRequired         = errors.New("symbol is required")
	errTradeCountInvalid      = errors.New("trade count must be omitted or between 1 and 1000")
)

type assetTranslatorStore struct {
	l      sync.RWMutex
	Assets map[string]string
}

// AssetVersion selects the naming convention used in Kraken responses.
type AssetVersion uint8

// AssetVersionDisplay requests canonical display names.
const AssetVersionDisplay AssetVersion = 1

// AssetPairInfo selects the tradable-pair information returned by Kraken.
type AssetPairInfo string

// Kraken tradable-pair information filters.
const (
	AssetPairInfoAll      AssetPairInfo = "info"
	AssetPairInfoLeverage AssetPairInfo = "leverage"
	AssetPairInfoFees     AssetPairInfo = "fees"
	AssetPairInfoMargin   AssetPairInfo = "margin"
)

// ExecutionVenue identifies a Kraken execution venue.
type ExecutionVenue string

// Kraken execution venues.
const (
	ExecutionVenueInternational ExecutionVenue = "international"
	ExecutionVenueBitnomial     ExecutionVenue = "bitnomial_exchange"
)

// GroupedOrderBookDepth identifies a supported grouped L2 order-book depth.
type GroupedOrderBookDepth uint64

// Kraken grouped L2 order-book depths.
const (
	GroupedOrderBookDepth10   GroupedOrderBookDepth = 10
	GroupedOrderBookDepth25   GroupedOrderBookDepth = 25
	GroupedOrderBookDepth100  GroupedOrderBookDepth = 100
	GroupedOrderBookDepth250  GroupedOrderBookDepth = 250
	GroupedOrderBookDepth1000 GroupedOrderBookDepth = 1000
)

// Level3OrderBookDepth identifies a supported authenticated L3 order-book depth.
type Level3OrderBookDepth uint64

// Kraken authenticated L3 order-book depths.
const (
	Level3OrderBookDepthFull Level3OrderBookDepth = 0
	Level3OrderBookDepth10   Level3OrderBookDepth = 10
	Level3OrderBookDepth25   Level3OrderBookDepth = 25
	Level3OrderBookDepth100  Level3OrderBookDepth = 100
	Level3OrderBookDepth250  Level3OrderBookDepth = 250
	Level3OrderBookDepth1000 Level3OrderBookDepth = 1000
)

// OrderBookGrouping identifies the number of ticks accumulated into a grouped price level.
type OrderBookGrouping uint64

// Kraken grouped-order-book tick groupings.
const (
	OrderBookGrouping1    OrderBookGrouping = 1
	OrderBookGrouping5    OrderBookGrouping = 5
	OrderBookGrouping10   OrderBookGrouping = 10
	OrderBookGrouping25   OrderBookGrouping = 25
	OrderBookGrouping50   OrderBookGrouping = 50
	OrderBookGrouping100  OrderBookGrouping = 100
	OrderBookGrouping250  OrderBookGrouping = 250
	OrderBookGrouping500  OrderBookGrouping = 500
	OrderBookGrouping1000 OrderBookGrouping = 1000
)

// SystemStatusResponse defines Kraken's current Spot system status.
type SystemStatusResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// GroupedOrderBookRequest defines request parameters for the grouped L2 order book.
type GroupedOrderBookRequest struct {
	Pair     currency.Pair
	Depth    GroupedOrderBookDepth
	Grouping OrderBookGrouping
}

// GroupedOrderBookResponse defines grouped L2 order book data.
type GroupedOrderBookResponse struct {
	Pair     string                  `json:"pair"`
	Grouping uint64                  `json:"grouping"`
	Bids     []GroupedOrderBookEntry `json:"bids"`
	Asks     []GroupedOrderBookEntry `json:"asks"`
}

// GroupedOrderBookEntry defines a grouped price level.
type GroupedOrderBookEntry struct {
	Price    types.Number `json:"price"`
	Quantity types.Number `json:"qty"`
}

// QueryLevel3OrderBookRequest defines request parameters for the authenticated L3 order book.
type QueryLevel3OrderBookRequest struct {
	Pair  currency.Pair
	Depth *Level3OrderBookDepth
}

// GetAssetsRequest defines current asset-info query parameters.
type GetAssetsRequest struct {
	Assets       currency.Currencies
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// GetAssetPairsRequest defines current tradable-pair query parameters.
type GetAssetPairsRequest struct {
	Pairs          currency.Pairs
	AssetClassBase AssetClass
	Info           AssetPairInfo
	CountryCode    string
	ExecutionVenue ExecutionVenue
	AssetVersion   AssetVersion
}

// GetTickerRequest defines current ticker query parameters.
type GetTickerRequest struct {
	Pairs        currency.Pairs
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// GetOHLCRequest defines current OHLC query parameters.
type GetOHLCRequest struct {
	Pair         currency.Pair
	Interval     time.Duration
	Since        time.Time
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// OHLCResponse defines candles grouped by pair and Kraken's pagination marker.
type OHLCResponse struct {
	Candles map[string][]OHLCEntry
	Last    types.Time
}

// UnmarshalJSON decodes pair-keyed OHLC arrays and the last marker.
func (r *OHLCResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	response := OHLCResponse{Candles: make(map[string][]OHLCEntry, len(raw))}
	for key, value := range raw {
		if key == "last" {
			if err := json.Unmarshal(value, &response.Last); err != nil {
				return err
			}
			continue
		}
		var candles []OHLCEntry
		if err := json.Unmarshal(value, &candles); err != nil {
			return err
		}
		response.Candles[key] = candles
	}
	*r = response
	return nil
}

// OHLCEntry defines one current OHLC candle.
type OHLCEntry struct {
	Time                       types.Time
	Open                       types.Number
	High                       types.Number
	Low                        types.Number
	Close                      types.Number
	VolumeWeightedAveragePrice types.Number
	Volume                     types.Number
	Count                      uint64
}

// UnmarshalJSON decodes Kraken's positional OHLC entry.
func (o *OHLCEntry) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 8 {
		return fmt.Errorf("expected 8 OHLC fields, got %d", len(fields))
	}
	decoded := OHLCEntry{}
	if err := json.Unmarshal(data, &[8]any{
		&decoded.Time,
		&decoded.Open,
		&decoded.High,
		&decoded.Low,
		&decoded.Close,
		&decoded.VolumeWeightedAveragePrice,
		&decoded.Volume,
		&decoded.Count,
	}); err != nil {
		return err
	}
	*o = decoded
	return nil
}

// GetDepthRequest defines current L2 order-book query parameters.
type GetDepthRequest struct {
	Pair         currency.Pair
	Count        uint64
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// OrderbookResponse defines pair-keyed current L2 order books.
type OrderbookResponse map[string]Orderbook

// Orderbook defines current L2 bids and asks.
type Orderbook struct {
	Bids []OrderbookEntry `json:"bids"`
	Asks []OrderbookEntry `json:"asks"`
}

// OrderbookEntry defines one positional L2 price level.
type OrderbookEntry struct {
	Price     types.Number
	Quantity  types.Number
	Timestamp types.Time
}

// UnmarshalJSON decodes Kraken's positional L2 price level.
func (o *OrderbookEntry) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 3 {
		return fmt.Errorf("expected 3 order book fields, got %d", len(fields))
	}
	decoded := OrderbookEntry{}
	if err := json.Unmarshal(data, &[3]any{&decoded.Price, &decoded.Quantity, &decoded.Timestamp}); err != nil {
		return err
	}
	*o = decoded
	return nil
}

// GetTradesRequest defines current recent-trades query parameters.
type GetTradesRequest struct {
	Pair         currency.Pair
	Since        time.Time
	Cursor       string
	Count        uint64
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// GetSpreadRequest defines current recent-spread query parameters.
type GetSpreadRequest struct {
	Pair         currency.Pair
	Since        time.Time
	AssetClass   AssetClass
	AssetVersion AssetVersion
}

// QueryLevel3OrderBookResponse defines L3 order book data.
type QueryLevel3OrderBookResponse struct {
	Pair string                 `json:"pair"`
	Bids []Level3OrderBookEntry `json:"bids"`
	Asks []Level3OrderBookEntry `json:"asks"`
}

// Level3OrderBookEntry defines a single L3 order.
type Level3OrderBookEntry struct {
	Price     types.Number `json:"price"`
	Quantity  types.Number `json:"qty"`
	OrderID   string       `json:"order_id"`
	Timestamp types.Time   `json:"timestamp"`
}

// GetPreTradeDataRequest defines request parameters for pre-trade transparency data.
type GetPreTradeDataRequest struct {
	Pair currency.Pair
}

// GetPreTradeDataResponse defines pre-trade transparency data.
type GetPreTradeDataResponse struct {
	Symbol            string              `json:"symbol"`
	Description       string              `json:"description"`
	BaseAsset         string              `json:"base_asset"`
	BaseDTICode       string              `json:"base_dti_code"`
	BaseDTIShortName  string              `json:"base_dti_short_name"`
	BaseNotation      string              `json:"base_notation"`
	QuoteAsset        string              `json:"quote_asset"`
	QuoteDTICode      string              `json:"quote_dti_code"`
	QuoteDTIShortName string              `json:"quote_dti_short_name"`
	QuoteNotation     string              `json:"quote_notation"`
	Venue             string              `json:"venue"`
	System            string              `json:"system"`
	Bids              []PreTradeBookLevel `json:"bids"`
	Asks              []PreTradeBookLevel `json:"asks"`
}

// PreTradeBookLevel defines an aggregated transparency order book level.
type PreTradeBookLevel struct {
	Side                 string       `json:"side"`
	Price                types.Number `json:"price"`
	Quantity             types.Number `json:"qty"`
	Count                uint64       `json:"count"`
	SubmissionTimestamp  time.Time    `json:"submission_ts"`
	PublicationTimestamp time.Time    `json:"publication_ts"`
}

// GetPostTradeDataRequest defines filters for post-trade transparency data.
type GetPostTradeDataRequest struct {
	Pair          currency.Pair
	FromTimestamp time.Time
	ToTimestamp   time.Time
	Count         uint64
}

// GetPostTradeDataResponse defines post-trade transparency data.
type GetPostTradeDataResponse struct {
	LastTimestamp time.Time       `json:"last_ts"`
	Count         uint64          `json:"count"`
	Trades        []PostTradeData `json:"trades"`
}

// PostTradeData defines a published Spot trade.
type PostTradeData struct {
	TradeID              string       `json:"trade_id"`
	Price                types.Number `json:"price"`
	Quantity             types.Number `json:"quantity"`
	Symbol               string       `json:"symbol"`
	Description          string       `json:"description"`
	BaseAsset            string       `json:"base_asset"`
	BaseNotation         string       `json:"base_notation"`
	QuoteAsset           string       `json:"quote_asset"`
	QuoteNotation        string       `json:"quote_notation"`
	TradeVenue           string       `json:"trade_venue"`
	TradeTimestamp       time.Time    `json:"trade_ts"`
	PublicationVenue     string       `json:"publication_venue"`
	PublicationTimestamp time.Time    `json:"publication_ts"`
}

// Asset holds asset information
type Asset struct {
	Altname         string       `json:"altname"`
	AclassBase      string       `json:"aclass_base"`
	AssetClass      string       `json:"aclass"`
	Decimals        int          `json:"decimals"`
	DisplayDecimals int          `json:"display_decimals"`
	CollateralValue types.Number `json:"collateral_value"`
	MarginRate      types.Number `json:"margin_rate"`
	Status          string       `json:"status"`
}

// AssetPairs holds asset pair information
type AssetPairs struct {
	Altname            string       `json:"altname"`
	Wsname             string       `json:"wsname"`
	AclassBase         string       `json:"aclass_base"`
	Base               string       `json:"base"`
	AclassQuote        string       `json:"aclass_quote"`
	Quote              string       `json:"quote"`
	ExecutionVenue     string       `json:"execution_venue"`
	Lot                string       `json:"lot"`
	CostDecimals       int          `json:"cost_decimals"`
	PairDecimals       int          `json:"pair_decimals"`
	LotDecimals        int          `json:"lot_decimals"`
	LotMultiplier      int          `json:"lot_multiplier"`
	LeverageBuy        []int        `json:"leverage_buy"`
	LeverageSell       []int        `json:"leverage_sell"`
	Fees               [][]float64  `json:"fees"`
	FeesMaker          [][]float64  `json:"fees_maker"`
	FeeVolumeCurrency  string       `json:"fee_volume_currency"`
	MarginCall         int          `json:"margin_call"`
	MarginStop         int          `json:"margin_stop"`
	OrderMinimum       types.Number `json:"ordermin"`
	CostMinimum        types.Number `json:"costmin"`
	TickSize           types.Number `json:"tick_size"`
	Status             string       `json:"status"`
	LongPositionLimit  types.Number `json:"long_position_limit"`
	ShortPositionLimit types.Number `json:"short_position_limit"`
}

// TickerResponse holds Kraken Spot ticker information.
type TickerResponse struct {
	Ask                        [3]types.Number `json:"a"`
	Bid                        [3]types.Number `json:"b"`
	Last                       [2]types.Number `json:"c"`
	Volume                     [2]types.Number `json:"v"`
	VolumeWeightedAveragePrice [2]types.Number `json:"p"`
	Trades                     [2]int64        `json:"t"`
	Low                        [2]types.Number `json:"l"`
	High                       [2]types.Number `json:"h"`
	Open                       types.Number    `json:"o"`
}

// RecentTradesResponse holds recent trade data
type RecentTradesResponse struct {
	Trades map[string][]RecentTradeResponseItem
	Last   string
}

// UnmarshalJSON unmarshals the recent trades response
func (r *RecentTradesResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	response := RecentTradesResponse{Trades: make(map[string][]RecentTradeResponseItem, len(raw))}
	for key, value := range raw {
		if key == "last" {
			if err := json.Unmarshal(value, &response.Last); err != nil {
				return err
			}
		} else {
			var trades []RecentTradeResponseItem
			if err := json.Unmarshal(value, &trades); err != nil {
				return err
			}
			response.Trades[key] = trades
		}
	}
	*r = response
	return nil
}

// RecentTradeResponseItem holds a single recent trade response item
type RecentTradeResponseItem struct {
	Price         types.Number
	Volume        types.Number
	Time          types.Time
	BuyOrSell     string
	MarketOrLimit string
	Miscellaneous any
	TradeID       types.Number
}

// UnmarshalJSON unmarshals the recent trade response item
func (r *RecentTradeResponseItem) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 7 {
		return fmt.Errorf("expected 7 trade fields, got %d", len(fields))
	}
	decoded := RecentTradeResponseItem{}
	if err := json.Unmarshal(data, &[7]any{
		&decoded.Price,
		&decoded.Volume,
		&decoded.Time,
		&decoded.BuyOrSell,
		&decoded.MarketOrLimit,
		&decoded.Miscellaneous,
		&decoded.TradeID,
	}); err != nil {
		return err
	}
	*r = decoded
	return nil
}

// SpreadItem holds the spread between trades
type SpreadItem struct {
	Time types.Time
	Bid  types.Number
	Ask  types.Number
}

// UnmarshalJSON unmarshals the spread item
func (s *SpreadItem) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 3 {
		return fmt.Errorf("expected 3 spread fields, got %d", len(fields))
	}
	decoded := SpreadItem{}
	if err := json.Unmarshal(data, &[3]any{&decoded.Time, &decoded.Bid, &decoded.Ask}); err != nil {
		return err
	}
	*s = decoded
	return nil
}

// SpreadResponse holds the spread response data
type SpreadResponse struct {
	Spreads map[string][]SpreadItem
	Last    types.Time
}

// UnmarshalJSON unmarshals the spread response
func (s *SpreadResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	response := SpreadResponse{Spreads: make(map[string][]SpreadItem, len(raw))}
	for key, value := range raw {
		if key == "last" {
			if err := json.Unmarshal(value, &response.Last); err != nil {
				return err
			}
		} else {
			var spreads []SpreadItem
			if err := json.Unmarshal(value, &spreads); err != nil {
				return err
			}
			response.Spreads[key] = spreads
		}
	}
	*s = response
	return nil
}

// TimeResponse contains Kraken's current server time.
type TimeResponse struct {
	Unixtime types.Time `json:"unixtime"`
	Rfc1123  string     `json:"rfc1123"`
}
