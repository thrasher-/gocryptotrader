// Package coinmarketcap connects to a suite of high-performance RESTful JSON
// endpoints that are specifically designed to meet the mission-critical demands
// of application developers, data scientists, and enterprise business
// platforms. Please see https://coinmarketcap.com/api/documentation/v1/# for
// API documentation
package coinmarketcap

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// NewFromSettings returns a new coin market cap instance with supplied settings
func NewFromSettings(cfg Settings) (*Coinmarketcap, error) {
	c := &Coinmarketcap{}
	c.SetDefaults()
	if err := c.Setup(cfg); err != nil {
		return nil, err
	}
	return c, nil
}

// SetDefaults sets default values for the exchange
func (c *Coinmarketcap) SetDefaults() {
	c.Name = "CoinMarketCap"
	c.Enabled = false
	c.Verbose = false
	c.APIUrl = baseURL
	c.APIVersion = version
	var err error
	c.Requester, err = request.New(c.Name,
		common.NewHTTPClientWithTimeout(defaultTimeOut),
		request.WithLimiter(request.NewBasicRateLimit(RateInterval, BasicRequestRate, 1)),
	)
	if err != nil {
		log.Errorln(log.Global, err)
	}
}

// Setup sets user configuration
func (c *Coinmarketcap) Setup(conf Settings) error {
	if !conf.Enabled {
		c.Enabled = false
		return nil
	}

	c.Enabled = true
	c.Verbose = conf.Verbose
	c.APIkey = conf.APIKey
	return c.SetAccountPlan(conf.AccountPlan)
}

// GetCryptocurrencyInfo returns all static metadata for one or more
// cryptocurrencies including name, symbol, logo, and its various registered
// URLs
//
// currencyID = digit code generated by coinmarketcap
func (c *Coinmarketcap) GetCryptocurrencyInfo(currencyID ...int64) (CryptoCurrencyInfo, error) {
	resp := struct {
		Data   CryptoCurrencyInfo `json:"data"`
		Status Status             `json:"status"`
	}{}

	err := c.CheckAccountPlan(Basic)
	if err != nil {
		return resp.Data, err
	}

	currStr := make([]string, len(currencyID))
	for i := range currencyID {
		currStr[i] = strconv.FormatInt(currencyID[i], 10)
	}

	val := url.Values{}
	val.Set("id", strings.Join(currStr, ","))

	err = c.SendHTTPRequest(http.MethodGet, endpointCryptocurrencyInfo, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetCryptocurrencyIDMap returns a paginated list of all cryptocurrencies by
// CoinMarketCap ID.
func (c *Coinmarketcap) GetCryptocurrencyIDMap() ([]CryptoCurrencyMap, error) {
	resp := struct {
		Data   []CryptoCurrencyMap `json:"data"`
		Status Status              `json:"status"`
	}{}

	err := c.CheckAccountPlan(Basic)
	if err != nil {
		return resp.Data, err
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointCryptocurrencyMap, nil, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetCryptocurrencyHistoricalListings returns a paginated list of all
// cryptocurrencies with market data for a given historical time.
func (c *Coinmarketcap) GetCryptocurrencyHistoricalListings() ([]CryptocurrencyHistoricalListings, error) {
	return nil, common.ErrNotYetImplemented
	// NOTE unreachable code but will be utilised at a later date
	// resp := struct {
	// 	Data   []CryptocurrencyHistoricalListings `json:"data"`
	// 	ServerStatus Status                       `json:"status"`
	// }{}

	//nolint:gocritic // unused code, used as example
	// err := c.CheckAccountPlan(0)
	// if err != nil {
	// 	return resp.Data, err
	// }

	//nolint:gocritic // unused code, used as example
	// err = c.SendHTTPRequest(http.MethodGet, endpointCryptocurrencyHistoricalListings, nil, &resp)
	// if err != nil {
	// 	return resp.Data, err
	// }

	//nolint:gocritic // unused code, used as example
	// if resp.ServerStatus.ErrorCode != 0 {
	// 	return resp.Data, errors.New(resp.ServerStatus.ErrorMessage)
	// }

	//nolint:gocritic // unused code, used as example
	// return resp.Data, nil
}

// GetCryptocurrencyLatestListing returns a paginated list of all
// cryptocurrencies with latest market data.
//
// Start - optionally offsets the paginated items
// limit - optionally sets return limit on items [1..5000]
func (c *Coinmarketcap) GetCryptocurrencyLatestListing(start, limit int64) ([]CryptocurrencyLatestListings, error) {
	resp := struct {
		Data   []CryptocurrencyLatestListings `json:"data"`
		Status Status                         `json:"status"`
	}{}

	err := c.CheckAccountPlan(Basic)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	if start >= 1 {
		val.Set("start", strconv.FormatInt(start, 10))
	}

	if limit > 0 {
		val.Set("limit", strconv.FormatInt(limit, 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointCryptocurrencyLatestListings, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetCryptocurrencyLatestMarketPairs returns all market pairs across all
// exchanges for the specified cryptocurrency with associated stats.
//
// currencyID - refers to the coinmarketcap currency id
// Start - optionally offsets the paginated items
// limit - optionally sets return limit on items [1..5000]
func (c *Coinmarketcap) GetCryptocurrencyLatestMarketPairs(currencyID, start, limit int64) (CryptocurrencyLatestMarketPairs, error) {
	resp := struct {
		Data   CryptocurrencyLatestMarketPairs `json:"data"`
		Status Status                          `json:"status"`
	}{}

	err := c.CheckAccountPlan(Standard)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	val.Set("id", strconv.FormatInt(currencyID, 10))

	if start >= 1 {
		val.Set("start", strconv.FormatInt(start, 10))
	}

	if limit > 0 {
		val.Set("limit", strconv.FormatInt(limit, 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointCryptocurrencyMarketPairs, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetCryptocurrencyOHLCHistorical return an interval of historic OHLCV
// (Open, High, Low, Close, Volume) market quotes for a cryptocurrency.
// Currently daily and hourly OHLCV periods are supported.
//
// currencyID - refers to the coinmarketcap currency id
// tStart - refers to the start time of historic value
// tEnd - refers to the end of the time block if zero will default to time.Now()
func (c *Coinmarketcap) GetCryptocurrencyOHLCHistorical(currencyID int64, tStart, tEnd time.Time) (CryptocurrencyOHLCHistorical, error) {
	resp := struct {
		Data   CryptocurrencyOHLCHistorical `json:"data"`
		Status Status                       `json:"status"`
	}{}

	err := c.CheckAccountPlan(Standard)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	val.Set("id", strconv.FormatInt(currencyID, 10))
	val.Set("time_start", strconv.FormatInt(tStart.Unix(), 10))

	if !tEnd.IsZero() {
		val.Set("time_end", strconv.FormatInt(tEnd.Unix(), 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointOHLCVHistorical, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetCryptocurrencyOHLCLatest return the latest OHLCV
// (Open, High, Low, Close, Volume) market values for one or more
// cryptocurrencies in the currently UTC day. Since the current UTC day is still
// active these values are updated frequently. You can find the final calculated
// OHLCV values for the last completed UTC day along with all historic days
// using /cryptocurrency/ohlcv/historical.
//
// currencyID - refers to the coinmarketcap currency id
func (c *Coinmarketcap) GetCryptocurrencyOHLCLatest(currencyID int64) (CryptocurrencyOHLCLatest, error) {
	resp := struct {
		Data   CryptocurrencyOHLCLatest `json:"data"`
		Status Status                   `json:"status"`
	}{}

	err := c.CheckAccountPlan(Startup)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	val.Set("id", strconv.FormatInt(currencyID, 10))

	err = c.SendHTTPRequest(http.MethodGet, endpointOHLCVLatest, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetCryptocurrencyLatestQuotes returns  the latest market quote for 1 or more
// cryptocurrencies.
//
// currencyID - refers to the coinmarketcap currency id
func (c *Coinmarketcap) GetCryptocurrencyLatestQuotes(currencyID ...int64) (CryptocurrencyLatestQuotes, error) {
	resp := struct {
		Data   CryptocurrencyLatestQuotes `json:"data"`
		Status Status                     `json:"status"`
	}{}

	err := c.CheckAccountPlan(Basic)
	if err != nil {
		return resp.Data, err
	}

	currStr := make([]string, len(currencyID))
	for i := range currencyID {
		currStr[i] = strconv.FormatInt(currencyID[i], 10)
	}

	val := url.Values{}
	val.Set("id", strings.Join(currStr, ","))

	err = c.SendHTTPRequest(http.MethodGet, endpointGetMarketQuotesLatest, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetCryptocurrencyHistoricalQuotes returns an interval of historic market
// quotes for any cryptocurrency based on time and interval parameters.
//
// currencyID - refers to the coinmarketcap currency id
// tStart - refers to the start time of historic value
// tEnd - refers to the end of the time block if zero will default to time.Now()
func (c *Coinmarketcap) GetCryptocurrencyHistoricalQuotes(currencyID int64, tStart, tEnd time.Time) (CryptocurrencyHistoricalQuotes, error) {
	resp := struct {
		Data   CryptocurrencyHistoricalQuotes `json:"data"`
		Status Status                         `json:"status"`
	}{}

	err := c.CheckAccountPlan(Standard)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	val.Set("id", strconv.FormatInt(currencyID, 10))
	val.Set("time_start", strconv.FormatInt(tStart.Unix(), 10))

	if !tEnd.IsZero() {
		val.Set("time_end", strconv.FormatInt(tEnd.Unix(), 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointGetMarketQuotesHistorical, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetExchangeInfo returns all static metadata for one or more exchanges
// including logo and homepage URL.
//
// exchangeID - refers to coinmarketcap exchange id
func (c *Coinmarketcap) GetExchangeInfo(exchangeID ...int64) (ExchangeInfo, error) {
	resp := struct {
		Data   ExchangeInfo `json:"data"`
		Status Status       `json:"status"`
	}{}

	err := c.CheckAccountPlan(Startup)
	if err != nil {
		return resp.Data, err
	}

	exchStr := make([]string, len(exchangeID))
	for x := range exchangeID {
		exchStr[x] = strconv.FormatInt(exchangeID[x], 10)
	}

	val := url.Values{}
	val.Set("id", strings.Join(exchStr, ","))

	err = c.SendHTTPRequest(http.MethodGet, endpointExchangeInfo, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetExchangeMap returns a paginated list of all cryptocurrency exchanges by
// CoinMarketCap ID. Recommend using this convenience endpoint to lookup and
// utilize the unique exchange id across all endpoints as typical exchange
// identifiers may change over time. ie huobi -> hadax -> global -> who knows
// what else
//
// Start - optionally offsets the paginated items
// limit - optionally sets return limit on items [1..5000]
func (c *Coinmarketcap) GetExchangeMap(start, limit int64) ([]ExchangeMap, error) {
	resp := struct {
		Data   []ExchangeMap `json:"data"`
		Status Status        `json:"status"`
	}{}

	err := c.CheckAccountPlan(Startup)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	if start >= 1 {
		val.Set("start", strconv.FormatInt(start, 10))
	}

	if limit != 0 {
		val.Set("limit", strconv.FormatInt(start, 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointExchangeMap, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetExchangeHistoricalListings returns a paginated list of all cryptocurrency
// exchanges with historical market data for a given point in time.
func (c *Coinmarketcap) GetExchangeHistoricalListings() ([]ExchangeHistoricalListings, error) {
	resp := struct {
		Data   []ExchangeHistoricalListings `json:"data"`
		Status Status                       `json:"status"`
	}{}

	return resp.Data, errors.New("this endpoint is not yet available")
}

// GetExchangeLatestListings returns a paginated list of all cryptocurrency
// exchanges with historical market data for a given point in time.
func (c *Coinmarketcap) GetExchangeLatestListings() ([]ExchangeLatestListings, error) {
	resp := struct {
		Data   []ExchangeLatestListings `json:"data"`
		Status Status                   `json:"status"`
	}{}

	return resp.Data, errors.New("this endpoint is not yet available")
}

// GetExchangeLatestMarketPairs returns a list of active market pairs for an
// exchange. Active means the market pair is open for trading.
//
// exchangeID - refers to coinmarketcap exchange id
// Start - optionally offsets the paginated items
// limit - optionally sets return limit on items [1..5000]
func (c *Coinmarketcap) GetExchangeLatestMarketPairs(exchangeID, start, limit int64) (ExchangeLatestMarketPairs, error) {
	resp := struct {
		Data   ExchangeLatestMarketPairs `json:"data"`
		Status Status                    `json:"status"`
	}{}

	err := c.CheckAccountPlan(Standard)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	val.Set("id", strconv.FormatInt(exchangeID, 10))

	if start >= 1 {
		val.Set("start", strconv.FormatInt(start, 10))
	}

	if limit != 0 {
		val.Set("limit", strconv.FormatInt(start, 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointExchangeMarketPairsLatest, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetExchangeLatestQuotes returns the latest aggregate market data for 1 or
// more exchanges.
//
// exchangeID - refers to coinmarketcap exchange id
func (c *Coinmarketcap) GetExchangeLatestQuotes(exchangeID ...int64) (ExchangeLatestQuotes, error) {
	resp := struct {
		Data   ExchangeLatestQuotes `json:"data"`
		Status Status               `json:"status"`
	}{}

	err := c.CheckAccountPlan(Standard)
	if err != nil {
		return resp.Data, err
	}

	exchStr := make([]string, len(exchangeID))
	for x := range exchangeID {
		exchStr[x] = strconv.FormatInt(exchangeID[x], 10)
	}

	val := url.Values{}
	val.Set("id", strings.Join(exchStr, ","))

	err = c.SendHTTPRequest(http.MethodGet, endpointExchangeMarketQuoteLatest, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetExchangeHistoricalQuotes returns an interval of historic quotes for any
// exchange based on time and interval parameters.
//
// exchangeID - refers to coinmarketcap exchange id
// tStart - refers to the start time of historic value
// tEnd - refers to the end of the time block if zero will default to time.Now()
func (c *Coinmarketcap) GetExchangeHistoricalQuotes(exchangeID int64, tStart, tEnd time.Time) (ExchangeHistoricalQuotes, error) {
	resp := struct {
		Data   ExchangeHistoricalQuotes `json:"data"`
		Status Status                   `json:"status"`
	}{}

	err := c.CheckAccountPlan(Standard)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	val.Set("id", strconv.FormatInt(exchangeID, 10))
	val.Set("time_start", strconv.FormatInt(tStart.Unix(), 10))

	if !tEnd.IsZero() {
		val.Set("time_end", strconv.FormatInt(tEnd.Unix(), 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointExchangeMarketQuoteHistorical, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetGlobalMeticLatestQuotes returns the latest quote of aggregate market
// metrics.
func (c *Coinmarketcap) GetGlobalMeticLatestQuotes() (GlobalMeticLatestQuotes, error) {
	resp := struct {
		Data   GlobalMeticLatestQuotes `json:"data"`
		Status Status                  `json:"status"`
	}{}

	err := c.CheckAccountPlan(Basic)
	if err != nil {
		return resp.Data, err
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointGlobalQuoteLatest, nil, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetGlobalMeticHistoricalQuotes returns an interval of aggregate 24 hour
// volume and market cap data globally based on time and interval parameters.
//
// tStart - refers to the start time of historic value
// tEnd - refers to the end of the time block if zero will default to time.Now()
func (c *Coinmarketcap) GetGlobalMeticHistoricalQuotes(tStart, tEnd time.Time) (GlobalMeticHistoricalQuotes, error) {
	resp := struct {
		Data   GlobalMeticHistoricalQuotes `json:"data"`
		Status Status                      `json:"status"`
	}{}

	err := c.CheckAccountPlan(Standard)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	val.Set("time_start", strconv.FormatInt(tStart.Unix(), 10))

	if !tEnd.IsZero() {
		val.Set("time_end", strconv.FormatInt(tEnd.Unix(), 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointGlobalQuoteHistorical, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// GetPriceConversion converts an amount of one currency into multiple
// cryptocurrencies or fiat currencies at the same time using the latest market
// averages. Optionally pass a historical timestamp to convert values based on
// historic averages.
//
// amount - An amount of currency to convert. Example: 10.43
// currencyID - refers to the coinmarketcap currency id
// atHistoricTime - [Optional] timestamp to reference historical pricing during
// conversion.
func (c *Coinmarketcap) GetPriceConversion(amount float64, currencyID int64, atHistoricTime time.Time) (PriceConversion, error) {
	resp := struct {
		Data PriceConversion `json:"data"`
		Status
	}{}

	err := c.CheckAccountPlan(Hobbyist)
	if err != nil {
		return resp.Data, err
	}

	val := url.Values{}
	val.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	val.Set("id", strconv.FormatInt(currencyID, 10))

	if !atHistoricTime.IsZero() {
		val.Set("time", strconv.FormatInt(atHistoricTime.Unix(), 10))
	}

	err = c.SendHTTPRequest(http.MethodGet, endpointPriceConversion, val, &resp)
	if err != nil {
		return resp.Data, err
	}

	if resp.Status.ErrorCode != 0 {
		return resp.Data, errors.New(resp.Status.ErrorMessage)
	}

	return resp.Data, nil
}

// SendHTTPRequest sends a valid HTTP request
func (c *Coinmarketcap) SendHTTPRequest(method, endpoint string, v url.Values, result any) error {
	headers := make(map[string]string)
	headers["Accept"] = "application/json"
	headers["X-CMC_PRO_API_KEY"] = c.APIkey

	path := c.APIUrl + c.APIVersion + endpoint
	if v != nil {
		path = path + "?" + v.Encode()
	}
	item := &request.Item{
		Method:  method,
		Path:    path,
		Headers: headers,
		Result:  result,
		Verbose: c.Verbose,
	}
	return c.Requester.SendPayload(context.TODO(), request.Unset, func() (*request.Item, error) {
		return item, nil
	}, request.AuthenticatedRequest)
}

// CheckAccountPlan checks your current account plan to the minimal account
// needed to send http request, this is used to minimize requests for lower
// account privileges
func (c *Coinmarketcap) CheckAccountPlan(minAllowable uint8) error {
	if c.Plan < minAllowable {
		return errors.New("function use not allowed, higher plan needed")
	}
	return nil
}

// SetAccountPlan sets account plan
func (c *Coinmarketcap) SetAccountPlan(s string) error {
	switch s {
	case "basic":
		c.Plan = Basic
	case "hobbyist":
		c.Plan = Hobbyist
	case "startup":
		c.Plan = Startup
	case "standard":
		c.Plan = Standard
	case "professional":
		c.Plan = Professional
	case "enterprise":
		c.Plan = Enterprise
	default:
		log.Warnf(log.Currency, "account plan %s not found, defaulting to basic", s)
		c.Plan = Basic
	}
	return nil
}
