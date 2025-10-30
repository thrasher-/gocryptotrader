package hyperliquid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/deposit"
	"github.com/thrasher-corp/gocryptotrader/exchanges/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchanges/futures"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/margin"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/exchanges/protocol"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	"github.com/thrasher-corp/gocryptotrader/log"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
	"github.com/thrasher-corp/gocryptotrader/types"
)

const (
	hyperliquidBridgeChain          = "arbitrum"
	hyperliquidBridgeMainnetAddress = "0x2df1c51e09aecf9cacb7bc98cb1742757f163df7"
	hyperliquidBridgeTestnetAddress = "0x08cfc1b6b2dcf36a1480b99353a354aa8ac56f89"
	fundingHistoryLookback          = 30 * 24 * time.Hour
)

// SetDefaults sets the basic defaults for Hyperliquid
func (e *Exchange) SetDefaults() {
	e.Name = "Hyperliquid"
	e.Enabled = true
	e.Verbose = true
	e.API.CredentialsValidator.RequiresKey = true
	e.API.CredentialsValidator.RequiresSecret = true

	requestFmt := &currency.PairFormat{Uppercase: true, Delimiter: currency.DashDelimiter}
	configFmt := &currency.PairFormat{Uppercase: true, Delimiter: currency.DashDelimiter}
	if err := e.SetGlobalPairsManager(requestFmt, configFmt); err != nil {
		log.Errorln(log.ExchangeSys, err)
	}

	perpFmt := currency.PairStore{
		AssetEnabled:  true,
		RequestFormat: requestFmt,
		ConfigFormat:  configFmt,
	}
	if err := e.SetAssetPairStore(asset.PerpetualContract, perpFmt); err != nil {
		log.Errorf(log.ExchangeSys, "%s error storing %q default asset formats: %s", e.Name, asset.PerpetualContract, err)
	}

	spotFmt := currency.PairStore{
		AssetEnabled:  true,
		RequestFormat: requestFmt,
		ConfigFormat:  configFmt,
	}
	if err := e.SetAssetPairStore(asset.Spot, spotFmt); err != nil {
		log.Errorf(log.ExchangeSys, "%s error storing %q default asset formats: %s", e.Name, asset.Spot, err)
	}

	// Fill out the capabilities/features that the exchange supports
	e.Features = exchange.Features{
		Supports: exchange.FeaturesSupported{
			REST:      true,
			Websocket: true,

			RESTCapabilities: protocol.Features{
				TickerFetching:         true,
				OrderbookFetching:      true,
				KlineFetching:          true,
				TradeFetching:          true,
				GetOrders:              true,
				AccountInfo:            true,
				AuthenticatedEndpoints: true,
			},

			WebsocketCapabilities: protocol.Features{
				TickerFetching:         true,
				OrderbookFetching:      true,
				KlineFetching:          true,
				TradeFetching:          true,
				Subscribe:              true,
				Unsubscribe:            true,
				AuthenticatedEndpoints: true,
			},

			WithdrawPermissions: exchange.AutoWithdrawCrypto |
				exchange.AutoWithdrawFiat,
			Kline: kline.ExchangeCapabilitiesSupported{
				Intervals: false,
			},
		},

		Enabled: exchange.FeaturesEnabled{
			AutoPairUpdates: true,
			Kline: kline.ExchangeCapabilitiesEnabled{
				Intervals: kline.DeployExchangeIntervals(
					kline.IntervalCapacity{Interval: kline.OneMin},
					kline.IntervalCapacity{Interval: kline.FiveMin},
					kline.IntervalCapacity{Interval: kline.FifteenMin},
					kline.IntervalCapacity{Interval: kline.OneHour},
					kline.IntervalCapacity{Interval: kline.FourHour},
					kline.IntervalCapacity{Interval: kline.OneDay},
				),
				GlobalResultLimit: 2000,
			},
		},
		Subscriptions: defaultSubscriptions,
	}
	// TODO: SET THE EXCHANGES RATE LIMIT HERE
	var err error
	e.Requester, err = request.New(e.Name, common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout))
	if err != nil {
		log.Errorln(log.ExchangeSys, err)
	}

	// TODO: SET THE URLs HERE
	e.API.Endpoints = e.NewEndpoints()
	if err = e.API.Endpoints.SetDefaultEndpoints(map[exchange.URL]string{
		exchange.RestSpot:      apiURL,
		exchange.WebsocketSpot: wsAPIURL,
	}); err != nil {
		log.Errorln(log.ExchangeSys, err)
	}
	e.Websocket = websocket.NewManager()
	e.WebsocketResponseMaxLimit = exchange.DefaultWebsocketResponseMaxLimit
	e.WebsocketResponseCheckTimeout = exchange.DefaultWebsocketResponseCheckTimeout
	e.WebsocketOrderbookBufferLimit = exchange.DefaultWebsocketOrderbookBufferLimit
}

func mapTimeInForce(tif order.TimeInForce) (string, error) {
	switch {
	case tif.Is(order.PostOnly):
		if tif.Is(order.ImmediateOrCancel) || tif.Is(order.FillOrKill) {
			return "", fmt.Errorf("hyperliquid: post-only cannot be combined with IOC or FOK")
		}
		return "Alo", nil
	case tif.Is(order.ImmediateOrCancel):
		return "Ioc", nil
	case tif.Is(order.FillOrKill):
		return "Fok", nil
	case tif == order.UnknownTIF || tif.Is(order.GoodTillCancel):
		return "Gtc", nil
	default:
		return "", fmt.Errorf("hyperliquid: unsupported time in force %s", tif.String())
	}
}

func parseTimeInForceHL(v string) order.TimeInForce {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ioc":
		return order.ImmediateOrCancel
	case "fok":
		return order.FillOrKill
	case "alo":
		return order.PostOnly
	case "gtc":
		return order.GoodTillCancel
	default:
		tif, err := order.StringToTimeInForce(strings.ToUpper(strings.TrimSpace(v)))
		if err != nil {
			return order.UnknownTIF
		}
		return tif
	}
}

func orderMarketIdentifier(pair currency.Pair, assetType asset.Item) (string, error) {
	switch assetType {
	case asset.PerpetualContract:
		return strings.ToUpper(pair.Base.String()), nil
	default:
		return "", fmt.Errorf("hyperliquid: asset %s not supported for trading operations", assetType)
	}
}

func pairFromCoin(coin string) currency.Pair {
	return currency.NewPair(currency.NewCode(strings.ToUpper(strings.TrimSpace(coin))), currency.USDC)
}

func parseOrderTypeHL(t string) order.Type {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "limit":
		return order.Limit
	case "market":
		return order.Market
	case "stop":
		return order.Stop
	case "takeprofit":
		return order.TakeProfit
	default:
		return order.UnknownType
	}
}

func mapOrderStatusFromString(status string) order.Status {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "", "open", "active", "resting", "new":
		return order.Active
	case "filled", "complete":
		return order.Filled
	case "cancelled", "canceled":
		return order.Cancelled
	case "partiallyfilled", "partialfilled", "partial_fill":
		return order.PartiallyFilled
	case "rejected", "ioccancelrejected":
		return order.Rejected
	default:
		if mapped, err := order.StringToOrderStatus(status); err == nil {
			return mapped
		}
		return order.UnknownStatus
	}
}

func parseActionStatuses(raw json.RawMessage) (string, order.Status, error, error) {
	if len(raw) == 0 {
		return "", order.UnknownStatus, nil, nil
	}
	var payload struct {
		Status   string `json:"status"`
		Response struct {
			Type string `json:"type"`
			Data struct {
				Statuses []json.RawMessage `json:"statuses"`
			} `json:"data"`
		} `json:"response"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", order.UnknownStatus, nil, err
	}
	if payload.Status != "" && !strings.EqualFold(payload.Status, "ok") {
		return "", order.UnknownStatus, errors.New(payload.Status), nil
	}
	var (
		orderID        string
		statusFromResp = order.UnknownStatus
		submissionErr  error
	)
	for _, statusRaw := range payload.Response.Data.Statuses {
		var statusMap map[string]json.RawMessage
		if err := json.Unmarshal(statusRaw, &statusMap); err != nil {
			var statusText string
			if err := json.Unmarshal(statusRaw, &statusText); err == nil && !strings.EqualFold(statusText, "success") {
				submissionErr = errors.New(statusText)
			}
			continue
		}
		if errRaw, ok := statusMap["error"]; ok {
			var errMsg string
			if err := json.Unmarshal(errRaw, &errMsg); err == nil {
				submissionErr = errors.New(errMsg)
			}
		}
		if restingRaw, ok := statusMap["resting"]; ok {
			var resting struct {
				OrderID int64 `json:"oid"`
			}
			if err := json.Unmarshal(restingRaw, &resting); err == nil && resting.OrderID > 0 {
				orderID = strconv.FormatInt(resting.OrderID, 10)
				statusFromResp = order.Active
			}
		}
		if filledRaw, ok := statusMap["filled"]; ok {
			var filled struct {
				OrderID int64 `json:"oid"`
			}
			if err := json.Unmarshal(filledRaw, &filled); err == nil && filled.OrderID > 0 {
				orderID = strconv.FormatInt(filled.OrderID, 10)
				statusFromResp = order.Filled
			}
		}
	}
	return orderID, statusFromResp, submissionErr, nil
}

// Setup takes in the supplied exchange configuration details and sets params
func (e *Exchange) Setup(exch *config.Exchange) error {
	if err := exch.Validate(); err != nil {
		return err
	}
	if !exch.Enabled {
		e.SetEnabled(false)
		return nil
	}
	if err := e.SetupDefaults(exch); err != nil {
		return err
	}

	/*
		wsRunningEndpoint, err := e.API.Endpoints.GetURL(exchange.WebsocketSpot)
		if err != nil {
			return err
		}

		// If websocket is not supported, remove these websocket sections
		if err := e.Websocket.Setup(
			&websocket.ManagerSetup{
				ExchangeConfig:        exch,
				DefaultURL:            wsAPIURL,
				RunningURL:            wsRunningEndpoint,
				Connector:             e.WsConnect,
				Subscriber:            e.Subscribe,
				Unsubscriber:          e.Unsubscribe,
				GenerateSubscriptions: e.generateSubscriptions,
				Features:              &e.Features.Supports.WebsocketCapabilities,
			}); err != nil {
			return err
		}

		return e.Websocket.SetupNewConnection(&websocket.ConnectionSetup{
			URL:                  e.Websocket.GetWebsocketURL(),
			ResponseCheckTimeout: exch.WebsocketResponseCheckTimeout,
			ResponseMaxLimit:     exch.WebsocketResponseMaxLimit,
		})
	*/
	return nil
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (e *Exchange) FetchTradablePairs(ctx context.Context, a asset.Item) (currency.Pairs, error) {
	switch a {
	case asset.PerpetualContract:
		return e.fetchPerpPairs(ctx)
	case asset.Spot:
		return e.fetchSpotPairs(ctx)
	default:
		return nil, fmt.Errorf("hyperliquid: asset %s not supported", a)
	}
}

// UpdateTradablePairs updates the exchanges available pairs and stores them in the exchanges config
func (e *Exchange) UpdateTradablePairs(ctx context.Context) error {
	assetTypes := e.GetAssetTypes(false)
	for x := range assetTypes {
		pairs, err := e.FetchTradablePairs(ctx, assetTypes[x])
		if err != nil {
			return err
		}
		if err := e.UpdatePairs(pairs, assetTypes[x], false); err != nil {
			return err
		}
	}
	return nil
}

func (e *Exchange) fetchPerpPairs(ctx context.Context) (currency.Pairs, error) {
	resp, err := e.GetMeta(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("hyperliquid: fetch perp meta: %w", err)
	}
	var meta MetaResponse
	if err := json.Unmarshal(resp, &meta); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode perp meta: %w", err)
	}
	if len(meta.Universe) == 0 {
		return nil, fmt.Errorf("hyperliquid: perp meta returned no markets")
	}
	pairs := make(currency.Pairs, 0, len(meta.Universe))
	for _, market := range meta.Universe {
		if market.IsDelisted {
			continue
		}
		base := currency.NewCode(strings.ToUpper(market.Name))
		pair := currency.NewPair(base, currency.USDC)
		pairs = append(pairs, pair)
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("hyperliquid: no active perp markets available")
	}
	return pairs, nil
}

func (e *Exchange) fetchSpotPairs(ctx context.Context) (currency.Pairs, error) {
	resp, err := e.GetSpotMeta(ctx)
	if err != nil {
		return nil, fmt.Errorf("hyperliquid: fetch spot meta: %w", err)
	}
	var meta SpotMetaResponse
	if err := json.Unmarshal(resp, &meta); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode spot meta: %w", err)
	}
	if len(meta.Universe) == 0 || len(meta.Tokens) == 0 {
		return nil, fmt.Errorf("hyperliquid: spot meta returned no markets")
	}
	pairs := make(currency.Pairs, 0, len(meta.Universe))
	for _, market := range meta.Universe {
		if !market.IsCanonical {
			continue
		}
		if len(market.Tokens) != 2 {
			continue
		}
		baseIdx := market.Tokens[0]
		quoteIdx := market.Tokens[1]
		if baseIdx < 0 || baseIdx >= len(meta.Tokens) || quoteIdx < 0 || quoteIdx >= len(meta.Tokens) {
			continue
		}
		baseName := strings.ToUpper(meta.Tokens[baseIdx].Name)
		quoteName := strings.ToUpper(meta.Tokens[quoteIdx].Name)
		base := currency.NewCode(baseName)
		quote := currency.NewCode(quoteName)
		pairs = append(pairs, currency.NewPair(base, quote))
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("hyperliquid: no canonical spot markets available")
	}
	return pairs, nil
}

func marketIdentifier(p currency.Pair, a asset.Item) (string, error) {
	switch a {
	case asset.PerpetualContract:
		return strings.ToUpper(p.Base.String()), nil
	case asset.Spot:
		return strings.ToUpper(p.Base.String()) + "/" + strings.ToUpper(p.Quote.String()), nil
	default:
		return "", fmt.Errorf("hyperliquid: unsupported asset %s", a)
	}
}

func candleIntervalString(interval kline.Interval) (string, error) {
	switch interval {
	case kline.OneMin:
		return "1m", nil
	case kline.FiveMin:
		return "5m", nil
	case kline.FifteenMin:
		return "15m", nil
	case kline.OneHour:
		return "1h", nil
	case kline.FourHour:
		return "4h", nil
	case kline.OneDay:
		return "1d", nil
	default:
		return "", fmt.Errorf("hyperliquid: unsupported kline interval %s", interval.Short())
	}
}

func (e *Exchange) ensurePublicTickers(ctx context.Context, a asset.Item, pairs []currency.Pair) error {
	if len(pairs) == 0 {
		return fmt.Errorf("hyperliquid: no pairs supplied for ticker update")
	}
	mids, err := e.fetchAllMidsMap(ctx)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		identifier, err := marketIdentifier(pair, a)
		if err != nil {
			return err
		}
		price, ok := mids[identifier]
		if !ok {
			return fmt.Errorf("hyperliquid: mid price missing for %s", identifier)
		}
		if price <= 0 {
			return fmt.Errorf("hyperliquid: invalid mid price for %s", identifier)
		}
		if err := e.storeTicker(pair, a, price); err != nil {
			return err
		}
	}
	return nil
}

func (e *Exchange) currentTime() time.Time {
	if e.now != nil {
		return e.now()
	}
	return time.Now()
}

func (e *Exchange) fetchAllMidsMap(ctx context.Context) (map[string]float64, error) {
	resp, err := e.GetAllMids(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("hyperliquid: fetch mids: %w", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode mids: %w", err)
	}
	mids := make(map[string]float64, len(raw))
	for key, value := range raw {
		if value == "" {
			continue
		}
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		mids[strings.ToUpper(key)] = floatVal
	}
	if len(mids) == 0 {
		return nil, fmt.Errorf("hyperliquid: received empty mids response")
	}
	return mids, nil
}

func (e *Exchange) storeTicker(p currency.Pair, a asset.Item, price float64) error {
	return ticker.ProcessTicker(&ticker.Price{
		ExchangeName: e.Name,
		Pair:         p,
		AssetType:    a,
		Last:         price,
		Close:        price,
		LastUpdated:  time.Now().UTC(),
	})
}

const defaultPublicTradeLimit = 200

func (e *Exchange) fetchTrades(ctx context.Context, p currency.Pair, a asset.Item, start, end *time.Time) ([]trade.Data, error) {
	identifier, err := marketIdentifier(p, a)
	if err != nil {
		return nil, err
	}
	limit := defaultPublicTradeLimit
	var startMs, endMs *int64
	if start != nil && !start.IsZero() {
		t := start.UTC().UnixMilli()
		startMs = &t
	}
	if end != nil && !end.IsZero() {
		t := end.UTC().UnixMilli()
		endMs = &t
	}
	resp, err := e.GetRecentPublicTrades(ctx, identifier, &limit, startMs, endMs)
	if err != nil {
		return nil, fmt.Errorf("hyperliquid: fetch trades: %w", err)
	}
	var raw []RecentTrade
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode trades: %w", err)
	}
	result := make([]trade.Data, 0, len(raw))
	startFilter := time.Time{}
	if start != nil {
		startFilter = start.UTC()
	}
	endFilter := time.Time{}
	if end != nil {
		endFilter = end.UTC()
	}
	for _, tr := range raw {
		ts := time.UnixMilli(tr.Time).UTC()
		if !startFilter.IsZero() && ts.Before(startFilter) {
			continue
		}
		if !endFilter.IsZero() && ts.After(endFilter) {
			continue
		}
		price := tr.Price.Float64()
		amount := tr.Size.Float64()
		if price == 0 || amount == 0 {
			continue
		}
		result = append(result, trade.Data{
			Exchange:     e.Name,
			CurrencyPair: p,
			AssetType:    a,
			Price:        price,
			Amount:       amount,
			Timestamp:    ts,
			Side:         convertTradeSide(tr.Side),
			TID:          strconv.FormatInt(tr.TID, 10),
		})
	}
	if len(result) == 0 {
		return result, nil
	}
	if e.IsSaveTradeDataEnabled() {
		if err := trade.AddTradesToBuffer(result...); err != nil {
			return nil, err
		}
	}
	sort.Sort(trade.ByDate(result))
	return result, nil
}

func convertTradeSide(side string) order.Side {
	switch strings.ToUpper(side) {
	case "B":
		return order.Buy
	case "A":
		return order.Sell
	default:
		return order.UnknownSide
	}
}

type userStateResponse struct {
	Withdrawable  types.Number `json:"withdrawable"`
	MarginSummary struct {
		AccountValue    types.Number `json:"accountValue"`
		TotalMarginUsed types.Number `json:"totalMarginUsed"`
	} `json:"marginSummary"`
	AssetPositions []struct {
		Position struct {
			Coin       string       `json:"coin"`
			Szi        types.Number `json:"szi"`
			MarginUsed types.Number `json:"marginUsed"`
			Leverage   struct {
				Type   string       `json:"type"`
				Value  types.Number `json:"value"`
				RawUSD types.Number `json:"rawUsd"`
			} `json:"leverage"`
		} `json:"position"`
	} `json:"assetPositions"`
}

type spotUserStateResponse struct {
	Balances []struct {
		Coin   string       `json:"coin"`
		Token  int64        `json:"token"`
		Total  types.Number `json:"total"`
		Hold   types.Number `json:"hold"`
		Entry  types.Number `json:"entryNtl"`
		Symbol string       `json:"symbol"`
	} `json:"balances"`
}

type userFundingHistoryEntry struct {
	Delta struct {
		Coin        string       `json:"coin"`
		FundingRate string       `json:"fundingRate"`
		NSamples    int64        `json:"nSamples"`
		Szi         types.Number `json:"szi"`
		Type        string       `json:"type"`
		USDC        types.Number `json:"usdc"`
	} `json:"delta"`
	Hash string `json:"hash"`
	Time int64  `json:"time"`
}

type userNonFundingLedgerEntry struct {
	Time  int64  `json:"time"`
	Hash  string `json:"hash"`
	Delta struct {
		Type             string       `json:"type"`
		USDC             types.Number `json:"usdc"`
		Fee              types.Number `json:"fee"`
		Destination      string       `json:"destination"`
		User             string       `json:"user"`
		Vault            string       `json:"vault"`
		RequestedUSD     types.Number `json:"requestedUsd"`
		NetWithdrawnUSD  types.Number `json:"netWithdrawnUsd"`
		Commission       types.Number `json:"commission"`
		ClosingCost      types.Number `json:"closingCost"`
		Nonce            int64        `json:"nonce"`
		Chain            string       `json:"chain"`
		TransactionHash  string       `json:"transactionHash"`
		TransactionIndex string       `json:"transactionIndex"`
	} `json:"delta"`
}

type userFeesResponse struct {
	UserAddRate   types.Number `json:"userAddRate"`
	UserCrossRate types.Number `json:"userCrossRate"`
	FeeSchedule   struct {
		Add   types.Number `json:"add"`
		Cross types.Number `json:"cross"`
	} `json:"feeSchedule"`
}

type fundingHistoryEntry struct {
	Coin        string        `json:"coin"`
	FundingRate types.Number  `json:"fundingRate"`
	Premium     *types.Number `json:"premium"`
	Time        int64         `json:"time"`
}

type openOrderResponse struct {
	Coin        string       `json:"coin"`
	LimitPrice  types.Number `json:"limitPx"`
	OrderID     int64        `json:"oid"`
	Side        string       `json:"side"`
	Size        types.Number `json:"sz"`
	Timestamp   int64        `json:"timestamp"`
	ReduceOnly  bool         `json:"reduceOnly"`
	OrderType   string       `json:"orderType"`
	TimeInForce string       `json:"tif"`
	ClientOID   *string      `json:"cloid"`
}

type historicalOrderEntry struct {
	Order struct {
		Coin        string       `json:"coin"`
		Side        string       `json:"side"`
		LimitPrice  types.Number `json:"limitPx"`
		Size        types.Number `json:"sz"`
		OrigSize    types.Number `json:"origSz"`
		OrderID     int64        `json:"oid"`
		Timestamp   int64        `json:"timestamp"`
		TriggerPx   types.Number `json:"triggerPx"`
		OrderType   string       `json:"orderType"`
		TimeInForce string       `json:"tif"`
		ReduceOnly  bool         `json:"reduceOnly"`
		ClientOID   *string      `json:"cloid"`
	} `json:"order"`
	Status          string `json:"status"`
	StatusTimestamp int64  `json:"statusTimestamp"`
}

func (e *Exchange) updatePerpBalances(ctx context.Context, address string) (accounts.SubAccounts, error) {
	resp, err := e.GetUserState(ctx, address, "")
	if err != nil {
		return nil, err
	}
	var payload userStateResponse
	if err := json.Unmarshal(resp, &payload); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode user state: %w", err)
	}
	now := e.currentTime().UTC()
	sub := accounts.NewSubAccount(asset.PerpetualContract, "")
	total := payload.MarginSummary.AccountValue.Float64()
	withdrawable := payload.Withdrawable.Float64()
	if total == 0 {
		total = withdrawable
	}
	hold := payload.MarginSummary.TotalMarginUsed.Float64()
	if hold == 0 {
		hold = math.Max(total-withdrawable, 0)
	}
	sub.Balances.Set(currency.USDC, accounts.Balance{
		Total:     total,
		Hold:      hold,
		Free:      withdrawable,
		UpdatedAt: now,
	})
	for _, pos := range payload.AssetPositions {
		coin := strings.ToUpper(pos.Position.Coin)
		if coin == "" {
			continue
		}
		qty := pos.Position.Szi.Float64()
		if qty == 0 {
			continue
		}
		curr := currency.NewCode(coin)
		sub.Balances.Set(curr, accounts.Balance{
			Total:     qty,
			Hold:      math.Abs(qty),
			UpdatedAt: now,
		})
	}
	subAccts := accounts.SubAccounts{sub}
	return subAccts, e.Accounts.Save(ctx, subAccts, true)
}

func (e *Exchange) updateSpotBalances(ctx context.Context, address string) (accounts.SubAccounts, error) {
	resp, err := e.GetSpotUserState(ctx, address)
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		sub := accounts.NewSubAccount(asset.Spot, "")
		subAccts := accounts.SubAccounts{sub}
		return subAccts, e.Accounts.Save(ctx, subAccts, true)
	}
	var payload spotUserStateResponse
	if err := json.Unmarshal(resp, &payload); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode spot user state: %w", err)
	}
	now := time.Now().UTC()
	sub := accounts.NewSubAccount(asset.Spot, "")
	for _, bal := range payload.Balances {
		total := bal.Total.Float64()
		hold := bal.Hold.Float64()
		free := math.Max(total-hold, 0)
		if total == 0 && hold == 0 {
			continue
		}
		code := currency.NewCode(strings.ToUpper(bal.Coin))
		sub.Balances.Set(code, accounts.Balance{
			Total:     total,
			Hold:      hold,
			Free:      free,
			UpdatedAt: now,
		})
	}
	subAccts := accounts.SubAccounts{sub}
	return subAccts, e.Accounts.Save(ctx, subAccts, true)
}

func (e *Exchange) openOrderToDetail(entry openOrderResponse) (order.Detail, error) {
	price := entry.LimitPrice.Float64()
	size := entry.Size.Float64()
	pair := pairFromCoin(entry.Coin)
	side := convertTradeSide(entry.Side)
	if side == order.Buy {
		side = order.Bid
	} else if side == order.Sell {
		side = order.Ask
	}
	detail := order.Detail{
		Exchange:           e.Name,
		OrderID:            strconv.FormatInt(entry.OrderID, 10),
		Pair:               pair,
		Type:               parseOrderTypeHL(entry.OrderType),
		Side:               side,
		Status:             order.Active,
		AssetType:          asset.PerpetualContract,
		Date:               time.UnixMilli(entry.Timestamp).UTC(),
		LastUpdated:        time.UnixMilli(entry.Timestamp).UTC(),
		Price:              price,
		Amount:             size,
		RemainingAmount:    size,
		ReduceOnly:         entry.ReduceOnly,
		TimeInForce:        parseTimeInForceHL(entry.TimeInForce),
		Cost:               price * size,
		CostAsset:          currency.USDC,
		SettlementCurrency: currency.USDC,
	}
	if entry.ClientOID != nil {
		detail.ClientOrderID = *entry.ClientOID
	}
	return detail, nil
}

func (e *Exchange) historicalOrderToDetail(entry historicalOrderEntry) (order.Detail, error) {
	orderInfo := entry.Order
	price := orderInfo.LimitPrice.Float64()
	size := orderInfo.Size.Float64()
	orig := orderInfo.OrigSize.Float64()
	if orig == 0 {
		orig = size
	}
	pair := pairFromCoin(orderInfo.Coin)
	side := convertTradeSide(orderInfo.Side)
	remaining := size
	executed := math.Max(orig-remaining, 0)
	detail := order.Detail{
		Exchange:           e.Name,
		OrderID:            strconv.FormatInt(orderInfo.OrderID, 10),
		Pair:               pair,
		Type:               parseOrderTypeHL(orderInfo.OrderType),
		Side:               side,
		Status:             mapOrderStatusFromString(entry.Status),
		AssetType:          asset.PerpetualContract,
		Date:               time.UnixMilli(orderInfo.Timestamp).UTC(),
		LastUpdated:        time.UnixMilli(entry.StatusTimestamp).UTC(),
		Price:              price,
		Amount:             orig,
		ExecutedAmount:     executed,
		RemainingAmount:    remaining,
		ReduceOnly:         orderInfo.ReduceOnly,
		TimeInForce:        parseTimeInForceHL(orderInfo.TimeInForce),
		Cost:               price * orig,
		CostAsset:          currency.USDC,
		SettlementCurrency: currency.USDC,
	}
	if orderInfo.ClientOID != nil {
		detail.ClientOrderID = *orderInfo.ClientOID
	}
	return detail, nil
}

func (e *Exchange) fetchOpenOrders(ctx context.Context, address string) ([]order.Detail, error) {
	raw, err := e.GetOpenOrders(ctx, address, "")
	if err != nil {
		return nil, err
	}
	var entries []openOrderResponse
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode open orders: %w", err)
	}
	results := make([]order.Detail, 0, len(entries))
	for _, entry := range entries {
		detail, err := e.openOrderToDetail(entry)
		if err != nil {
			return nil, err
		}
		results = append(results, detail)
	}
	return results, nil
}

func (e *Exchange) fetchHistoricalOrders(ctx context.Context, address string) ([]order.Detail, error) {
	raw, err := e.GetHistoricalOrders(ctx, address)
	if err != nil {
		return nil, err
	}
	var entries []historicalOrderEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode historical orders: %w", err)
	}
	results := make([]order.Detail, 0, len(entries))
	for _, entry := range entries {
		detail, err := e.historicalOrderToDetail(entry)
		if err != nil {
			return nil, err
		}
		results = append(results, detail)
	}
	return results, nil
}

// UpdateTicker updates and returns the ticker for a currency pair
func (e *Exchange) UpdateTicker(ctx context.Context, p currency.Pair, assetType asset.Item) (*ticker.Price, error) {
	if err := e.ensurePublicTickers(ctx, assetType, []currency.Pair{p}); err != nil {
		return nil, err
	}
	return ticker.GetTicker(e.Name, p, assetType)
}

// UpdateTickers updates all currency pairs of a given asset type
func (e *Exchange) UpdateTickers(ctx context.Context, assetType asset.Item) error {
	pairs, err := e.GetEnabledPairs(assetType)
	if err != nil {
		return err
	}
	return e.ensurePublicTickers(ctx, assetType, pairs)
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (e *Exchange) UpdateOrderbook(ctx context.Context, pair currency.Pair, assetType asset.Item) (*orderbook.Book, error) {
	market, err := marketIdentifier(pair, assetType)
	if err != nil {
		return nil, err
	}
	resp, err := e.GetL2Snapshot(ctx, market)
	if err != nil {
		return nil, fmt.Errorf("hyperliquid: get l2 snapshot: %w", err)
	}
	var snapshot OrderbookSnapshot
	if err := json.Unmarshal(resp, &snapshot); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode orderbook: %w", err)
	}
	if len(snapshot.Levels) < 2 {
		return nil, fmt.Errorf("hyperliquid: orderbook levels incomplete")
	}
	book := &orderbook.Book{
		Exchange:          e.Name,
		Pair:              pair,
		Asset:             assetType,
		Bids:              make(orderbook.Levels, 0, len(snapshot.Levels[0])),
		Asks:              make(orderbook.Levels, 0, len(snapshot.Levels[1])),
		LastUpdated:       time.UnixMilli(snapshot.Time),
		ValidateOrderbook: e.ValidateOrderbook,
	}
	for _, lvl := range snapshot.Levels[0] {
		price := lvl.Price.Float64()
		size := lvl.Size.Float64()
		if price == 0 || size == 0 {
			continue
		}
		book.Bids = append(book.Bids, orderbook.Level{Price: price, Amount: size})
	}
	for _, lvl := range snapshot.Levels[1] {
		price := lvl.Price.Float64()
		size := lvl.Size.Float64()
		if price == 0 || size == 0 {
			continue
		}
		book.Asks = append(book.Asks, orderbook.Level{Price: price, Amount: size})
	}
	if err := book.Process(); err != nil {
		return nil, err
	}
	return orderbook.Get(e.Name, pair, assetType)
}

// UpdateAccountBalances retrieves currency balances
func (e *Exchange) UpdateAccountBalances(ctx context.Context, assetType asset.Item) (accounts.SubAccounts, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	address := e.accountAddr
	if address == "" && e.wallet != nil {
		address = e.wallet.hexAddress()
	}
	if address == "" {
		return nil, fmt.Errorf("hyperliquid: account address unavailable")
	}
	switch assetType {
	case asset.PerpetualContract:
		return e.updatePerpBalances(ctx, address)
	case asset.Spot:
		return e.updateSpotBalances(ctx, address)
	default:
		return nil, fmt.Errorf("%s: %w", assetType, asset.ErrNotSupported)
	}
}

// GetAccountFundingHistory returns funding history, deposits and withdrawals
func (e *Exchange) GetAccountFundingHistory(ctx context.Context) ([]exchange.FundingHistory, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	e.ensureInitialised()
	address := e.accountAddr
	if address == "" && e.wallet != nil {
		address = e.wallet.hexAddress()
	}
	if address == "" {
		return nil, fmt.Errorf("hyperliquid: account address unavailable")
	}
	start := e.now().Add(-fundingHistoryLookback).UnixMilli()
	raw, err := e.GetUserFundingHistory(ctx, address, start, nil)
	if err != nil {
		return nil, err
	}
	var entries []userFundingHistoryEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode funding history: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	result := make([]exchange.FundingHistory, 0, len(entries))
	for _, entry := range entries {
		amount := entry.Delta.USDC.Float64()
		result = append(result, exchange.FundingHistory{
			ExchangeName: e.Name,
			Status:       entry.Delta.Type,
			TransferID:   entry.Hash,
			Description:  fmt.Sprintf("%s funding rate %s (%d samples)", entry.Delta.Coin, entry.Delta.FundingRate, entry.Delta.NSamples),
			Timestamp:    time.UnixMilli(entry.Time).UTC(),
			Currency:     entry.Delta.Coin,
			Amount:       amount,
			Fee:          0,
			TransferType: entry.Delta.Type,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})
	return result, nil
}

// GetWithdrawalsHistory returns previous withdrawals data
func (e *Exchange) GetWithdrawalsHistory(ctx context.Context, c currency.Code, a asset.Item) ([]exchange.WithdrawalHistory, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	e.ensureInitialised()
	if !c.Equal(currency.USDC) {
		return nil, fmt.Errorf("hyperliquid: unsupported withdrawal currency %s", c)
	}
	if a != asset.PerpetualContract && a != asset.Spot {
		return nil, fmt.Errorf("hyperliquid: unsupported asset for withdrawals %s", a)
	}
	address := e.accountAddr
	if address == "" && e.wallet != nil {
		address = e.wallet.hexAddress()
	}
	if address == "" {
		return nil, fmt.Errorf("hyperliquid: account address unavailable")
	}
	start := e.now().Add(-fundingHistoryLookback).UnixMilli()
	raw, err := e.GetUserNonFundingLedgerUpdates(ctx, address, start, nil)
	if err != nil {
		return nil, err
	}
	var entries []userNonFundingLedgerEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode non-funding ledger: %w", err)
	}
	withdrawals := make([]exchange.WithdrawalHistory, 0, len(entries))
	for _, entry := range entries {
		if strings.ToLower(entry.Delta.Type) != "withdraw" {
			continue
		}
		amount := entry.Delta.USDC.Float64()
		if amount == 0 {
			amount = entry.Delta.NetWithdrawnUSD.Float64()
		}
		withdrawals = append(withdrawals, exchange.WithdrawalHistory{
			Status:       entry.Delta.Type,
			TransferID:   entry.Hash,
			Description:  fmt.Sprintf("withdraw nonce %d", entry.Delta.Nonce),
			Timestamp:    time.UnixMilli(entry.Time).UTC(),
			Currency:     c.String(),
			Amount:       amount,
			Fee:          entry.Delta.Fee.Float64(),
			TransferType: entry.Delta.Type,
			CryptoChain:  hyperliquidBridgeChain,
		})
	}
	if len(withdrawals) == 0 {
		return nil, nil
	}
	sort.Slice(withdrawals, func(i, j int) bool {
		return withdrawals[i].Timestamp.Before(withdrawals[j].Timestamp)
	})
	return withdrawals, nil
}

// GetRecentTrades returns the most recent trades for a currency and asset
func (e *Exchange) GetRecentTrades(ctx context.Context, p currency.Pair, assetType asset.Item) ([]trade.Data, error) {
	return e.fetchTrades(ctx, p, assetType, nil, nil)
}

// GetHistoricTrades returns historic trade data within the timeframe provided
func (e *Exchange) GetHistoricTrades(ctx context.Context, p currency.Pair, assetType asset.Item, timestampStart, timestampEnd time.Time) ([]trade.Data, error) {
	start := timestampStart
	end := timestampEnd
	return e.fetchTrades(ctx, p, assetType, &start, &end)
}

// GetServerTime returns the current exchange server time.
func (e *Exchange) GetServerTime(ctx context.Context, a asset.Item) (time.Time, error) {
	return time.Now().UTC(), nil
}

// SubmitOrder submits a new order
func (e *Exchange) SubmitOrder(ctx context.Context, s *order.Submit) (*order.SubmitResponse, error) {
	if err := s.Validate(e.GetTradingRequirements()); err != nil {
		return nil, err
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	if s.AssetType != asset.PerpetualContract {
		return nil, fmt.Errorf("hyperliquid: submit order unsupported asset %s", s.AssetType)
	}
	if !s.Type.Is(order.Limit) {
		return nil, fmt.Errorf("hyperliquid: only limit orders are supported")
	}
	if s.Price <= 0 {
		return nil, order.ErrPriceMustBeSetIfLimitOrder
	}
	if s.Amount <= 0 {
		return nil, order.ErrAmountIsInvalid
	}
	marketID, err := orderMarketIdentifier(s.Pair, s.AssetType)
	if err != nil {
		return nil, err
	}
	tif, err := mapTimeInForce(s.TimeInForce)
	if err != nil {
		return nil, err
	}
	req := OrderRequest{
		Coin:          marketID,
		IsBuy:         s.Side == order.Buy,
		Size:          s.Amount,
		LimitPrice:    s.Price,
		OrderType:     OrderType{Limit: &LimitOrderType{TimeInForce: tif}},
		ReduceOnly:    s.ReduceOnly,
		ClientOrderID: s.ClientOrderID,
	}
	raw, err := e.PlaceOrder(ctx, req, nil)
	if err != nil {
		return nil, err
	}
	orderID, statusFromResponse, submissionErr, statusParseErr := parseActionStatuses(raw)
	now := time.Now().UTC()
	resp := &order.SubmitResponse{
		Exchange:        e.Name,
		Type:            s.Type,
		Side:            s.Side,
		Pair:            s.Pair,
		AssetType:       s.AssetType,
		TimeInForce:     s.TimeInForce,
		ReduceOnly:      s.ReduceOnly,
		Price:           s.Price,
		Amount:          s.Amount,
		RemainingAmount: s.Amount,
		ClientID:        s.ClientID,
		ClientOrderID:   s.ClientOrderID,
		Date:            now,
		LastUpdated:     now,
		Status:          order.Open,
		Cost:            s.Price * s.Amount,
	}
	if statusParseErr == nil {
		if orderID != "" {
			resp.OrderID = orderID
		}
		if statusFromResponse != order.UnknownStatus {
			resp.Status = statusFromResponse
		}
		if submissionErr != nil {
			resp.SubmissionError = submissionErr
			return resp, submissionErr
		}
	}
	return resp, nil
}

// ModifyOrder modifies an existing order
func (e *Exchange) ModifyOrder(ctx context.Context, action *order.Modify) (*order.ModifyResponse, error) {
	if err := action.Validate(); err != nil {
		return nil, err
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	if action.AssetType != asset.PerpetualContract {
		return nil, fmt.Errorf("hyperliquid: modify order unsupported asset %s", action.AssetType)
	}
	if action.Price <= 0 {
		return nil, fmt.Errorf("hyperliquid: modify requires price")
	}
	if action.Amount <= 0 {
		return nil, fmt.Errorf("hyperliquid: modify requires amount")
	}
	marketID, err := orderMarketIdentifier(action.Pair, action.AssetType)
	if err != nil {
		return nil, err
	}
	tif, err := mapTimeInForce(action.TimeInForce)
	if err != nil {
		return nil, err
	}
	modReq := ModifyRequest{
		Order: OrderRequest{
			Coin:          marketID,
			IsBuy:         action.Side == order.Buy,
			Size:          action.Amount,
			LimitPrice:    action.Price,
			OrderType:     OrderType{Limit: &LimitOrderType{TimeInForce: tif}},
			ClientOrderID: action.ClientOrderID,
		},
	}
	if action.OrderID != "" {
		oid, err := strconv.ParseInt(action.OrderID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("hyperliquid: invalid order id %s: %w", action.OrderID, err)
		}
		modReq.Identifier.OrderID = &oid
	} else if action.ClientOrderID != "" {
		modReq.Identifier.ClientOrderID = action.ClientOrderID
	} else {
		return nil, order.ErrOrderIDNotSet
	}
	raw, err := e.AmendOrders(ctx, []ModifyRequest{modReq})
	if err != nil {
		return nil, err
	}
	if _, _, submissionErr, parseErr := parseActionStatuses(raw); parseErr == nil && submissionErr != nil {
		return nil, submissionErr
	}
	resp, err := action.DeriveModifyResponse()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	resp.Status = order.Open
	resp.Date = now
	resp.LastUpdated = now
	return resp, nil
}

// CancelOrder cancels an order by its corresponding ID number
func (e *Exchange) CancelOrder(ctx context.Context, ord *order.Cancel) error {
	if ord == nil {
		return order.ErrCancelOrderIsNil
	}
	if err := ord.Validate(ord.PairAssetRequired()); err != nil {
		return err
	}
	if err := e.ensureWallet(ctx); err != nil {
		return err
	}
	if ord.AssetType != asset.PerpetualContract {
		return fmt.Errorf("hyperliquid: cancel unsupported asset %s", ord.AssetType)
	}
	marketID, err := orderMarketIdentifier(ord.Pair, ord.AssetType)
	if err != nil {
		return err
	}
	switch {
	case ord.OrderID != "":
		oid, err := strconv.ParseInt(ord.OrderID, 10, 64)
		if err != nil {
			return fmt.Errorf("hyperliquid: invalid order id %s: %w", ord.OrderID, err)
		}
		raw, err := e.CancelOrdersByID(ctx, []CancelRequest{{
			Coin:    marketID,
			OrderID: &oid,
		}})
		if err != nil {
			return err
		}
		if _, _, submissionErr, parseErr := parseActionStatuses(raw); parseErr == nil && submissionErr != nil {
			return submissionErr
		}
	case ord.ClientOrderID != "":
		raw, err := e.CancelOrdersByClientID(ctx, []CancelByCloidRequest{{
			Coin:          marketID,
			ClientOrderID: ord.ClientOrderID,
		}})
		if err != nil {
			return err
		}
		if _, _, submissionErr, parseErr := parseActionStatuses(raw); parseErr == nil && submissionErr != nil {
			return submissionErr
		}
	default:
		return order.ErrOrderIDNotSet
	}
	return nil
}

// CancelBatchOrders cancels orders by their corresponding ID numbers
func (e *Exchange) CancelBatchOrders(ctx context.Context, orders []order.Cancel) (*order.CancelBatchResponse, error) {
	if len(orders) == 0 {
		return nil, fmt.Errorf("hyperliquid: no cancel requests supplied")
	}
	resp := &order.CancelBatchResponse{
		Status: make(map[string]string, len(orders)),
	}
	var errs error
	for i := range orders {
		key := orders[i].OrderID
		if key == "" {
			key = orders[i].ClientOrderID
		}
		if key == "" {
			key = fmt.Sprintf("index_%d", i)
		}
		err := e.CancelOrder(ctx, &orders[i])
		if err != nil {
			errs = common.AppendError(errs, err)
			resp.Status[key] = err.Error()
		} else {
			resp.Status[key] = "success"
		}
	}
	return resp, errs
}

// CancelAllOrders cancels all orders associated with a currency pair
func (e *Exchange) CancelAllOrders(ctx context.Context, orderCancellation *order.Cancel) (order.CancelAllResponse, error) {
	resp := order.CancelAllResponse{
		Status: make(map[string]string),
	}
	if orderCancellation == nil {
		return resp, order.ErrCancelOrderIsNil
	}
	assetType := orderCancellation.AssetType
	if assetType == asset.Empty {
		assetType = asset.PerpetualContract
	}
	if assetType != asset.PerpetualContract {
		return resp, fmt.Errorf("hyperliquid: cancel unsupported asset %s", assetType)
	}
	if err := e.ensureWallet(ctx); err != nil {
		return resp, err
	}
	req := &order.MultiOrderRequest{
		AssetType: asset.PerpetualContract,
		Type:      order.AnyType,
		Side:      order.AnySide,
	}
	if !orderCancellation.Pair.IsEmpty() {
		req.Pairs = append(req.Pairs, orderCancellation.Pair)
	}
	if orderCancellation.Type != order.UnknownType && orderCancellation.Type != order.AnyType {
		req.Type = orderCancellation.Type
	}
	if orderCancellation.Side != order.UnknownSide && orderCancellation.Side != order.AnySide {
		req.Side = orderCancellation.Side
	}
	active, err := e.GetActiveOrders(ctx, req)
	if err != nil {
		return resp, err
	}
	var errs error
	for i := range active {
		if orderCancellation.ClientOrderID != "" && !strings.EqualFold(active[i].ClientOrderID, orderCancellation.ClientOrderID) {
			continue
		}
		cancelReq := &order.Cancel{
			AssetType: asset.PerpetualContract,
			Pair:      active[i].Pair,
			OrderID:   active[i].OrderID,
		}
		if cancelReq.OrderID == "" {
			continue
		}
		if err := e.CancelOrder(ctx, cancelReq); err != nil {
			errs = common.AppendError(errs, err)
			resp.Status[cancelReq.OrderID] = err.Error()
		} else {
			resp.Status[cancelReq.OrderID] = "success"
		}
	}
	return resp, errs
}

// GetOrderInfo returns order information based on order ID
func (e *Exchange) GetOrderInfo(ctx context.Context, orderID string, pair currency.Pair, assetType asset.Item) (*order.Detail, error) {
	id := strings.TrimSpace(orderID)
	if id == "" {
		return nil, fmt.Errorf("hyperliquid: order id required")
	}
	if assetType == asset.Empty {
		assetType = asset.PerpetualContract
	}
	if assetType != asset.PerpetualContract {
		return nil, fmt.Errorf("hyperliquid: asset %s not supported for order info", assetType)
	}
	req := &order.MultiOrderRequest{
		AssetType: assetType,
		Type:      order.AnyType,
		Side:      order.AnySide,
	}
	if !pair.IsEmpty() {
		req.Pairs = append(req.Pairs, pair)
	}
	active, err := e.GetActiveOrders(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range active {
		if active[i].OrderID == id {
			result := active[i]
			return &result, nil
		}
	}
	history, err := e.GetOrderHistory(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range history {
		if history[i].OrderID == id {
			result := history[i]
			return &result, nil
		}
	}
	return nil, fmt.Errorf("hyperliquid: order %s not found", id)
}

// GetDepositAddress returns a deposit address for a specified currency
func (e *Exchange) GetDepositAddress(ctx context.Context, c currency.Code, accountID string, chain string) (*deposit.Address, error) {
	if !c.Equal(currency.USDC) {
		return nil, fmt.Errorf("hyperliquid: unsupported deposit currency %s", c)
	}
	normalized := strings.TrimSpace(strings.ToLower(chain))
	switch normalized {
	case "", hyperliquidBridgeChain, "arbitrum-one", "arbitrum one":
	default:
		return nil, fmt.Errorf("hyperliquid: unsupported deposit chain %s", chain)
	}
	addr := hyperliquidBridgeMainnetAddress
	if !e.isMainnetEndpoint() {
		addr = hyperliquidBridgeTestnetAddress
	}
	return &deposit.Address{
		Address: addr,
		Chain:   hyperliquidBridgeChain,
	}, nil
}

// GetAvailableTransferChains returns the available transfer blockchains for the specific cryptocurrency
func (e *Exchange) GetAvailableTransferChains(ctx context.Context, cryptocurrency currency.Code) ([]string, error) {
	if !cryptocurrency.Equal(currency.USDC) {
		return nil, fmt.Errorf("hyperliquid: unsupported transfer currency %s", cryptocurrency)
	}
	return []string{hyperliquidBridgeChain}, nil
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is submitted
func (e *Exchange) WithdrawCryptocurrencyFunds(ctx context.Context, withdrawRequest *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	if err := withdrawRequest.Validate(); err != nil {
		return nil, err
	}
	if withdrawRequest.Type != withdraw.Crypto {
		return nil, fmt.Errorf("hyperliquid: unsupported withdrawal type %v", withdrawRequest.Type)
	}
	if !withdrawRequest.Currency.Equal(currency.USDC) {
		return nil, fmt.Errorf("hyperliquid: only USDC withdrawals are supported")
	}
	chain := strings.TrimSpace(strings.ToLower(withdrawRequest.Crypto.Chain))
	switch chain {
	case "", hyperliquidBridgeChain, "arbitrum-one", "arbitrum one":
	default:
		return nil, fmt.Errorf("hyperliquid: unsupported withdrawal chain %s", withdrawRequest.Crypto.Chain)
	}
	resp, err := e.WithdrawFromBridge(ctx, &WithdrawFromBridgeRequest{
		Destination: strings.ToLower(withdrawRequest.Crypto.Address),
		Amount:      withdrawRequest.Amount,
	})
	if err != nil {
		return nil, err
	}
	status := struct {
		Status string `json:"status"`
		ID     string `json:"txHash"`
	}{}
	_ = json.Unmarshal(resp, &status)
	exchangeResp := &withdraw.ExchangeResponse{
		Name:   e.Name,
		ID:     status.ID,
		Status: status.Status,
	}
	if exchangeResp.Status == "" {
		exchangeResp.Status = string(resp)
	}
	return exchangeResp, nil
}

// WithdrawFiatFunds returns a withdrawal ID when a withdrawal is submitted
func (e *Exchange) WithdrawFiatFunds(_ context.Context, withdrawRequest *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	// if err := withdrawRequest.Validate(); err != nil {
	//	return nil, err
	// }
	return nil, common.ErrNotYetImplemented
}

// WithdrawFiatFundsToInternationalBank returns a withdrawal ID when a withdrawal is submitted
func (e *Exchange) WithdrawFiatFundsToInternationalBank(_ context.Context, withdrawRequest *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	// if err := withdrawRequest.Validate(); err != nil {
	//	return nil, err
	// }
	return nil, common.ErrNotYetImplemented
}

// GetActiveOrders retrieves any orders that are active/open
func (e *Exchange) GetActiveOrders(ctx context.Context, req *order.MultiOrderRequest) (order.FilteredOrders, error) {
	if req == nil {
		req = &order.MultiOrderRequest{
			AssetType: asset.PerpetualContract,
			Type:      order.AnyType,
			Side:      order.AnySide,
		}
	}
	if req.AssetType == asset.Empty {
		req.AssetType = asset.PerpetualContract
	}
	if req.Type == order.UnknownType {
		req.Type = order.AnyType
	}
	if req.Side == order.UnknownSide {
		req.Side = order.AnySide
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.AssetType != asset.PerpetualContract {
		return nil, fmt.Errorf("hyperliquid: asset %s not supported for active orders", req.AssetType)
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	address := e.accountAddr
	if address == "" && e.wallet != nil {
		address = e.wallet.hexAddress()
	}
	if address == "" {
		return nil, fmt.Errorf("hyperliquid: account address unavailable")
	}
	orders, err := e.fetchOpenOrders(ctx, address)
	if err != nil {
		return nil, err
	}
	return req.Filter(e.Name, orders), nil
}

// GetOrderHistory retrieves account order information
// Can Limit response to specific order status
func (e *Exchange) GetOrderHistory(ctx context.Context, req *order.MultiOrderRequest) (order.FilteredOrders, error) {
	if req == nil {
		req = &order.MultiOrderRequest{
			AssetType: asset.PerpetualContract,
			Type:      order.AnyType,
			Side:      order.AnySide,
		}
	}
	if req.AssetType == asset.Empty {
		req.AssetType = asset.PerpetualContract
	}
	if req.Type == order.UnknownType {
		req.Type = order.AnyType
	}
	if req.Side == order.UnknownSide {
		req.Side = order.AnySide
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.AssetType != asset.PerpetualContract {
		return nil, fmt.Errorf("hyperliquid: asset %s not supported for order history", req.AssetType)
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	address := e.accountAddr
	if address == "" && e.wallet != nil {
		address = e.wallet.hexAddress()
	}
	if address == "" {
		return nil, fmt.Errorf("hyperliquid: account address unavailable")
	}
	orders, err := e.fetchHistoricalOrders(ctx, address)
	if err != nil {
		return nil, err
	}
	return req.Filter(e.Name, orders), nil
}

func (e *Exchange) tradingFeeRate(ctx context.Context, isMaker bool) (float64, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return 0, err
	}
	address := e.accountAddr
	if address == "" && e.wallet != nil {
		address = e.wallet.hexAddress()
	}
	if address == "" {
		return 0, fmt.Errorf("hyperliquid: account address unavailable")
	}
	raw, err := e.GetUserFees(ctx, address)
	if err != nil {
		return 0, fmt.Errorf("hyperliquid: get user fees: %w", err)
	}
	var payload userFeesResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, fmt.Errorf("hyperliquid: decode user fees: %w", err)
	}
	var rate float64
	if isMaker {
		rate = payload.UserAddRate.Float64()
		if rate == 0 {
			rate = payload.FeeSchedule.Add.Float64()
		}
	} else {
		rate = payload.UserCrossRate.Float64()
		if rate == 0 {
			rate = payload.FeeSchedule.Cross.Float64()
		}
	}
	if rate < 0 {
		rate = 0
	}
	return rate, nil
}

// GetFeeByType returns an estimate of fee based on the type of transaction
func (e *Exchange) GetFeeByType(ctx context.Context, feeBuilder *exchange.FeeBuilder) (float64, error) {
	if feeBuilder == nil {
		return 0, common.ErrNilPointer
	}
	switch feeBuilder.FeeType {
	case exchange.CryptocurrencyTradeFee, exchange.OfflineTradeFee:
		if feeBuilder.Amount == 0 || feeBuilder.PurchasePrice == 0 {
			return 0, nil
		}
		rate, err := e.tradingFeeRate(ctx, feeBuilder.IsMaker)
		if err != nil {
			return 0, err
		}
		return rate * feeBuilder.PurchasePrice * feeBuilder.Amount, nil
	case exchange.CryptocurrencyWithdrawalFee:
		return 0, nil
	default:
		return 0, fmt.Errorf("hyperliquid: unsupported fee type %v", feeBuilder.FeeType)
	}
}

// ValidateAPICredentials validates current credentials used for wrapper
func (e *Exchange) ValidateAPICredentials(ctx context.Context, assetType asset.Item) error {
	_, err := e.UpdateAccountBalances(ctx, assetType)
	return e.CheckTransientError(err)
}

func (e *Exchange) fetchCandleSnapshots(ctx context.Context, market, interval string, start, end time.Time) ([]CandleSnapshot, error) {
	raw, err := e.GetCandleSnapshot(ctx, market, interval, start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("hyperliquid: get candle snapshot: %w", err)
	}
	var snapshots []CandleSnapshot
	if err := json.Unmarshal(raw, &snapshots); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode candle snapshot: %w", err)
	}
	return snapshots, nil
}

func candleSnapshotsToSeries(snapshots []CandleSnapshot) []kline.Candle {
	if len(snapshots) == 0 {
		return nil
	}
	candles := make([]kline.Candle, 0, len(snapshots))
	for _, snap := range snapshots {
		candles = append(candles, kline.Candle{
			Time:   time.UnixMilli(snap.OpenTime).UTC(),
			Open:   snap.Open.Float64(),
			High:   snap.High.Float64(),
			Low:    snap.Low.Float64(),
			Close:  snap.Close.Float64(),
			Volume: snap.Volume.Float64(),
		})
	}
	return candles
}

func matchesPairFilters(pair currency.Pair, assetType asset.Item, filters []key.PairAsset) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f.Asset == assetType && f.Pair().Equal(pair) {
			return true
		}
	}
	return false
}

// GetHistoricCandles returns candles between a time period for a set time interval
func (e *Exchange) GetHistoricCandles(ctx context.Context, pair currency.Pair, a asset.Item, interval kline.Interval, start, end time.Time) (*kline.Item, error) {
	req, err := e.GetKlineRequest(pair, a, interval, start, end, false)
	if err != nil {
		return nil, err
	}
	market, err := marketIdentifier(pair, a)
	if err != nil {
		return nil, err
	}
	intervalStr, err := candleIntervalString(req.ExchangeInterval)
	if err != nil {
		return nil, err
	}
	snapshots, err := e.fetchCandleSnapshots(ctx, market, intervalStr, req.Start, req.End)
	if err != nil {
		return nil, err
	}
	series := candleSnapshotsToSeries(snapshots)
	if len(series) == 0 {
		return nil, kline.ErrNoTimeSeriesDataToConvert
	}
	return req.ProcessResponse(series)
}

// GetHistoricCandlesExtended returns candles between a time period for a set time interval
func (e *Exchange) GetHistoricCandlesExtended(ctx context.Context, pair currency.Pair, a asset.Item, interval kline.Interval, start, end time.Time) (*kline.Item, error) {
	req, err := e.GetKlineExtendedRequest(pair, a, interval, start, end)
	if err != nil {
		return nil, err
	}
	market, err := marketIdentifier(pair, a)
	if err != nil {
		return nil, err
	}
	intervalStr, err := candleIntervalString(req.ExchangeInterval)
	if err != nil {
		return nil, err
	}
	timeSeries := make([]kline.Candle, 0, req.Size())
	if req.RangeHolder != nil && len(req.RangeHolder.Ranges) > 0 {
		for _, r := range req.RangeHolder.Ranges {
			snapshots, err := e.fetchCandleSnapshots(ctx, market, intervalStr, r.Start.Time, r.End.Time)
			if err != nil {
				return nil, err
			}
			timeSeries = append(timeSeries, candleSnapshotsToSeries(snapshots)...)
		}
	} else {
		snapshots, err := e.fetchCandleSnapshots(ctx, market, intervalStr, req.Start, req.End)
		if err != nil {
			return nil, err
		}
		timeSeries = append(timeSeries, candleSnapshotsToSeries(snapshots)...)
	}
	if len(timeSeries) == 0 {
		return nil, kline.ErrNoTimeSeriesDataToConvert
	}
	return req.ProcessResponse(timeSeries)
}

// GetLeverage gets the account's initial leverage for the asset type and pair
func (e *Exchange) GetLeverage(ctx context.Context, a asset.Item, pair currency.Pair, _ margin.Type, _ order.Side) (float64, error) {
	if a != asset.PerpetualContract {
		return 0, fmt.Errorf("hyperliquid: leverage unsupported for asset %s", a)
	}
	if pair.IsEmpty() {
		return 0, currency.ErrCurrencyPairEmpty
	}
	if err := e.ensureWallet(ctx); err != nil {
		return 0, err
	}
	address := e.accountAddr
	if address == "" && e.wallet != nil {
		address = e.wallet.hexAddress()
	}
	if address == "" {
		return 0, fmt.Errorf("hyperliquid: account address unavailable")
	}
	raw, err := e.GetUserState(ctx, address, "")
	if err != nil {
		return 0, err
	}
	var payload userStateResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, fmt.Errorf("hyperliquid: decode user state: %w", err)
	}
	target := strings.ToUpper(pair.Base.String())
	for _, pos := range payload.AssetPositions {
		if !strings.EqualFold(pos.Position.Coin, target) {
			continue
		}
		value := pos.Position.Leverage.Value.Float64()
		if value <= 0 {
			return 0, nil
		}
		return value, nil
	}
	return 0, nil
}

// GetFuturesContractDetails returns all contracts from the exchange by asset type
func (e *Exchange) GetFuturesContractDetails(ctx context.Context, item asset.Item) ([]futures.Contract, error) {
	if item != asset.PerpetualContract {
		return nil, fmt.Errorf("hyperliquid: futures unsupported for asset %s", item)
	}
	raw, err := e.GetMeta(ctx, "")
	if err != nil {
		return nil, err
	}
	var meta MetaResponse
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode meta: %w", err)
	}
	if len(meta.Universe) == 0 {
		return nil, fmt.Errorf("hyperliquid: no perpetual markets available")
	}
	contracts := make([]futures.Contract, 0, len(meta.Universe))
	for _, market := range meta.Universe {
		if market.IsDelisted {
			continue
		}
		pair := currency.NewPair(currency.NewCode(strings.ToUpper(market.Name)), currency.USDC)
		status := "active"
		contracts = append(contracts, futures.Contract{
			Exchange:             e.Name,
			Name:                 pair,
			Underlying:           pair,
			Asset:                asset.PerpetualContract,
			IsActive:             !market.IsDelisted,
			Status:               status,
			Type:                 futures.Perpetual,
			SettlementType:       futures.Linear,
			SettlementCurrencies: currency.Currencies{currency.USDC},
			MarginCurrency:       currency.USDC,
			Multiplier:           1,
			MaxLeverage:          float64(market.MaxLeverage),
		})
	}
	return contracts, nil
}

// GetLatestFundingRates returns the latest funding rates data
func (e *Exchange) GetLatestFundingRates(ctx context.Context, req *fundingrate.LatestRateRequest) ([]fundingrate.LatestRateResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Asset != asset.PerpetualContract {
		return nil, fmt.Errorf("hyperliquid: funding unsupported for asset %s", req.Asset)
	}
	if req.Pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	now := e.currentTime()
	start := now.Add(-fundingHistoryLookback)
	raw, err := e.GetFundingHistory(ctx, strings.ToUpper(req.Pair.Base.String()), start.UnixMilli(), nil)
	if err != nil {
		return nil, err
	}
	var entries []fundingHistoryEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode funding history: %w", err)
	}
	if len(entries) == 0 {
		return nil, fundingrate.ErrNoFundingRatesFound
	}
	latest := entries[len(entries)-1]
	rate := fundingrate.Rate{
		Time: time.UnixMilli(latest.Time).UTC(),
		Rate: latest.FundingRate.Decimal(),
	}
	response := fundingrate.LatestRateResponse{
		Exchange:       e.Name,
		Asset:          req.Asset,
		Pair:           req.Pair,
		LatestRate:     rate,
		TimeChecked:    now.UTC(),
		TimeOfNextRate: rate.Time.Add(time.Hour),
	}
	if req.IncludePredictedRate && latest.Premium != nil {
		response.PredictedUpcomingRate = fundingrate.Rate{
			Time: response.TimeOfNextRate,
			Rate: latest.Premium.Decimal(),
		}
	}
	return []fundingrate.LatestRateResponse{response}, nil
}

// GetHistoricalFundingRates returns funding rates for a given asset and currency for a time period
func (e *Exchange) GetHistoricalFundingRates(ctx context.Context, r *fundingrate.HistoricalRatesRequest) (*fundingrate.HistoricalRates, error) {
	if r == nil {
		return nil, common.ErrNilPointer
	}
	if r.Asset != asset.PerpetualContract {
		return nil, fmt.Errorf("hyperliquid: funding unsupported for asset %s", r.Asset)
	}
	if r.Pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	now := e.currentTime()
	start := r.StartDate
	if start.IsZero() {
		start = time.Now().Add(-fundingHistoryLookback)
	}
	end := r.EndDate
	if end.IsZero() {
		end = now
	}
	if r.RespectHistoryLimits {
		limit := now.Add(-fundingHistoryLookback)
		if start.Before(limit) {
			start = limit
		}
	}
	if end.Before(start) {
		return nil, fmt.Errorf("hyperliquid: end time %s before start %s", end, start)
	}
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()
	raw, err := e.GetFundingHistory(ctx, strings.ToUpper(r.Pair.Base.String()), startMs, &endMs)
	if err != nil {
		return nil, err
	}
	var entries []fundingHistoryEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode funding history: %w", err)
	}
	rates := make([]fundingrate.Rate, 0, len(entries))
	for i := range entries {
		rates = append(rates, fundingrate.Rate{
			Time: time.UnixMilli(entries[i].Time).UTC(),
			Rate: entries[i].FundingRate.Decimal(),
		})
	}
	result := &fundingrate.HistoricalRates{
		Exchange:        e.Name,
		Asset:           r.Asset,
		Pair:            r.Pair,
		StartDate:       start.UTC(),
		EndDate:         end.UTC(),
		FundingRates:    rates,
		PaymentCurrency: currency.USDC,
		TimeOfNextRate:  end.UTC().Add(time.Hour),
	}
	if len(rates) > 0 {
		result.LatestRate = rates[len(rates)-1]
		result.TimeOfNextRate = result.LatestRate.Time.Add(time.Hour)
		if r.IncludePredictedRate {
			last := entries[len(entries)-1]
			if last.Premium != nil {
				result.PredictedUpcomingRate = fundingrate.Rate{
					Time: result.TimeOfNextRate,
					Rate: last.Premium.Decimal(),
				}
			}
		}
	} else {
		result.TimeOfNextRate = end.UTC()
	}
	return result, nil
}

// GetOpenInterest returns the open interest rate for a given asset pair
func (e *Exchange) GetOpenInterest(ctx context.Context, filters ...key.PairAsset) ([]futures.OpenInterest, error) {
	raw, err := e.GetMetaAndAssetContexts(ctx)
	if err != nil {
		return nil, err
	}
	var payload []json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode metaAndAssetCtxs: %w", err)
	}
	if len(payload) < 2 {
		return nil, fmt.Errorf("hyperliquid: metaAndAssetCtxs response malformed")
	}
	var meta MetaResponse
	if err := json.Unmarshal(payload[0], &meta); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode meta: %w", err)
	}
	var contexts []PerpetualAssetContext
	if err := json.Unmarshal(payload[1], &contexts); err != nil {
		return nil, fmt.Errorf("hyperliquid: decode asset contexts: %w", err)
	}
	results := make([]futures.OpenInterest, 0, len(contexts))
	ctxIndex := 0
	for _, market := range meta.Universe {
		if market.IsDelisted {
			continue
		}
		if ctxIndex >= len(contexts) {
			break
		}
		pair := currency.NewPair(currency.NewCode(strings.ToUpper(market.Name)), currency.USDC)
		if !matchesPairFilters(pair, asset.PerpetualContract, filters) {
			ctxIndex++
			continue
		}
		results = append(results, futures.OpenInterest{
			Key:          key.NewExchangeAssetPair(e.Name, asset.PerpetualContract, pair),
			OpenInterest: contexts[ctxIndex].OpenInterest.Float64(),
		})
		ctxIndex++
	}
	if len(results) == 0 && len(filters) > 0 {
		return nil, fmt.Errorf("hyperliquid: open interest not found for requested pairs")
	}
	return results, nil
}

// GetCurrencyTradeURL returns the URL to the exchange's trade page for the given asset and currency pair
func (e *Exchange) GetCurrencyTradeURL(_ context.Context, a asset.Item, cp currency.Pair) (string, error) {
	_, err := e.CurrencyPairs.IsPairEnabled(cp, a)
	if err != nil {
		return "", err
	}
	switch a {
	case asset.PerpetualContract:
		return "https://app.hyperliquid.xyz/trade/" + strings.ToUpper(cp.Base.String()), nil
	case asset.Spot:
		return "https://app.hyperliquid.xyz/spot/" + strings.ToUpper(cp.Base.String()) + "-" + strings.ToUpper(cp.Quote.String()), nil
	default:
		return "", fmt.Errorf("hyperliquid: unsupported asset %s for trade URL", a)
	}
}

// UpdateOrderExecutionLimits updates order execution limits
func (e *Exchange) UpdateOrderExecutionLimits(ctx context.Context, a asset.Item) error {
	var levels []limits.MinMaxLevel
	switch a {
	case asset.PerpetualContract:
		raw, err := e.GetMeta(ctx, "")
		if err != nil {
			return err
		}
		var meta MetaResponse
		if err := json.Unmarshal(raw, &meta); err != nil {
			return fmt.Errorf("hyperliquid: decode meta: %w", err)
		}
		levels = make([]limits.MinMaxLevel, 0, len(meta.Universe))
		for _, market := range meta.Universe {
			if market.IsDelisted {
				continue
			}
			pair := currency.NewPair(currency.NewCode(strings.ToUpper(market.Name)), currency.USDC)
			step := math.Pow10(-int(market.SzDecimals))
			if step <= 0 {
				step = 0
			}
			levels = append(levels, limits.MinMaxLevel{
				Key:                     key.NewExchangeAssetPair(e.Name, asset.PerpetualContract, pair),
				AmountStepIncrementSize: step,
				MinimumBaseAmount:       step,
				PriceStepIncrementSize:  0.01,
				Listed:                  time.Now(),
			})
		}
	case asset.Spot:
		raw, err := e.GetSpotMeta(ctx)
		if err != nil {
			return err
		}
		var meta SpotMetaResponse
		if err := json.Unmarshal(raw, &meta); err != nil {
			return fmt.Errorf("hyperliquid: decode spot meta: %w", err)
		}
		levels = make([]limits.MinMaxLevel, 0, len(meta.Universe))
		for _, market := range meta.Universe {
			if len(market.Tokens) < 2 {
				continue
			}
			baseIdx := market.Tokens[0]
			quoteIdx := market.Tokens[1]
			if baseIdx < 0 || int(baseIdx) >= len(meta.Tokens) || quoteIdx < 0 || int(quoteIdx) >= len(meta.Tokens) {
				continue
			}
			baseToken := meta.Tokens[baseIdx]
			quoteToken := meta.Tokens[quoteIdx]
			pair := currency.NewPair(currency.NewCode(strings.ToUpper(baseToken.Name)), currency.NewCode(strings.ToUpper(quoteToken.Name)))
			step := math.Pow10(-int(baseToken.SZDecimals))
			if step <= 0 {
				step = 0
			}
			levels = append(levels, limits.MinMaxLevel{
				Key:                     key.NewExchangeAssetPair(e.Name, asset.Spot, pair),
				AmountStepIncrementSize: step,
				MinimumBaseAmount:       step,
				PriceStepIncrementSize:  0.0001,
				Listed:                  time.Now(),
			})
		}
	default:
		return fmt.Errorf("hyperliquid: update limits unsupported for asset %s", a)
	}
	if len(levels) == 0 {
		return fmt.Errorf("hyperliquid: no execution limits available for %s", a)
	}
	return limits.Load(levels)
}

// SetLeverage sets the account's initial leverage for the asset type and pair
func (e *Exchange) SetLeverage(ctx context.Context, a asset.Item, pair currency.Pair, m margin.Type, leverage float64, _ order.Side) error {
	if a != asset.PerpetualContract {
		return fmt.Errorf("hyperliquid: leverage unsupported for asset %s", a)
	}
	if pair.IsEmpty() {
		return currency.ErrCurrencyPairEmpty
	}
	if leverage <= 0 {
		return fmt.Errorf("hyperliquid: leverage must be positive")
	}
	var isCross bool
	switch m { //nolint:exhaustive // only care about relevant margin types
	case margin.Unset, margin.Multi, margin.NoMargin:
		isCross = true
	case margin.Isolated:
		isCross = false
	default:
		return margin.ErrMarginTypeUnsupported
	}
	amount := int64(math.Round(leverage))
	if amount <= 0 {
		return fmt.Errorf("hyperliquid: leverage must be positive")
	}
	_, err := e.UpdateLeverage(ctx, strings.ToUpper(pair.Base.String()), amount, isCross)
	return err
}
