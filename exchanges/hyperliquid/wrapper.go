package hyperliquid

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
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
)

const (
	// Builder-deployed perpetual DEXs number their asset IDs from 100000, in blocks of 10000 per DEX index
	builderPerpetualAssetIDBase    = 100000
	builderPerpetualDEXAssetStride = 10000
	// spotAssetIDBase offsets spot universe indices into the asset ID space
	spotAssetIDBase              = 10000
	pairMappingMissCacheDuration = time.Minute
	// Prices allow five significant figures and at most this many decimals less the market's size decimals
	marketPriceSignificantFigures = 5
	perpetualPriceDecimalBase     = 6
	spotPriceDecimalBase          = 8
	// defaultTriggerSlippage bounds the execution price of a market TP/SL child that sets no limit price
	defaultTriggerSlippage = 0.1
	// maximumFrontendOpenOrders is the most orders frontendOpenOrders returns for one perpetual DEX
	maximumFrontendOpenOrders = 100
	// Base tier fee rates for offline fee estimates
	perpetualMakerBaseFeeRate = 0.00015
	perpetualTakerBaseFeeRate = 0.00045
	spotMakerBaseFeeRate      = 0.0004
	spotTakerBaseFeeRate      = 0.0007
)

var (
	errAmbiguousCoinMapping       = errors.New("ambiguous coin mapping")
	errAssetContextNotFound       = errors.New("asset context not found")
	errBridgeChainInvalid         = errors.New("invalid Hyperliquid bridge chain")
	errCrossMarginUnavailable     = errors.New("cross margin is unavailable for an isolated-only market")
	errEndpointEnvironment        = errors.New("endpoint does not match the configured sandbox environment")
	errGroupedOrderChildFailure   = errors.New("one or more grouped TP/SL child orders were rejected")
	errInvalidFilledSize          = errors.New("invalid filled order size")
	errInvalidMarketPrice         = errors.New("invalid market price")
	errInvalidPerpetualDEX        = errors.New("invalid perpetual DEX registry or metadata")
	errInvalidSpotTokenCount      = errors.New("spot market must reference exactly two tokens")
	errMarketMidPriceNotFound     = errors.New("market mid price not found")
	errModifyOrderTypeUnsupported = errors.New("only resting limit orders can be modified")
	errOrderNotModifiable         = errors.New("order is not open for modification")
	errPairMappingNotFound        = errors.New("pair mapping not found")
	errPricePrecision             = errors.New("price exceeds market precision")
	errRiskManagementUnsupported  = errors.New("unsupported risk management configuration")
	errSizePrecision              = errors.New("size exceeds market precision")
	errSlippageTolerance          = errors.New("market order slippage tolerance must be greater than 0 and less than 1")
	errSpotTokenNotFound          = errors.New("spot token metadata not found")
	errTransferCurrencyInvalid    = errors.New("hyperliquid bridge transfers only support USDC")
	errTriggerOrderReduceOnly     = errors.New("trigger orders must be reduce-only perpetual orders")
	errUnsupportedOrderStatus     = errors.New("unsupported order status")
	errWithdrawalAddressTag       = errors.New("hyperliquid bridge withdrawals do not support an address tag")
	errWithdrawalFeeInput         = errors.New("hyperliquid calculates the bridge withdrawal fee")
)

// pairMapping links a currency pair to the Hyperliquid coin and asset ID it trades as
type pairMapping struct {
	pair         currency.Pair
	coin         string
	dex          string
	assetID      uint64
	sizeDecimals uint64
	maxLeverage  uint64
	onlyIsolated bool
}

func logDefaultError(err error) {
	if err != nil {
		log.Errorln(log.ExchangeSys, err)
	}
}

// SetDefaults sets the basic defaults for Hyperliquid
func (e *Exchange) SetDefaults() {
	e.Name = "Hyperliquid"
	e.Enabled = true
	e.Verbose = true
	e.BaseCurrencies = currency.Currencies{currency.USDC}
	e.API.CredentialsValidator.RequiresKey = true

	pairFormat := &currency.PairFormat{Uppercase: true, Delimiter: currency.DashDelimiter}
	for _, a := range []asset.Item{asset.Spot, asset.PerpetualContract} {
		logDefaultError(e.SetAssetPairStore(a, currency.PairStore{AssetEnabled: true, RequestFormat: pairFormat, ConfigFormat: pairFormat}))
	}

	e.Features = exchange.Features{
		Supports: exchange.FeaturesSupported{
			REST:      true,
			Websocket: true,
			RESTCapabilities: protocol.Features{
				TickerBatching:                 true,
				TickerFetching:                 true,
				KlineFetching:                  true,
				TradeFetching:                  true,
				OrderbookFetching:              true,
				AutoPairUpdates:                true,
				AccountBalance:                 true,
				CryptoWithdrawal:               true,
				DepositHistory:                 true,
				WithdrawalHistory:              true,
				GetOrder:                       true,
				GetOrders:                      true,
				CancelOrders:                   true,
				CancelOrder:                    true,
				SubmitOrder:                    true,
				ModifyOrder:                    true,
				TradeFee:                       true,
				AuthenticatedEndpoints:         true,
				HasAssetTypeAccountSegregation: true,
			},
			WebsocketCapabilities: protocol.Features{
				TickerFetching:         true,
				KlineFetching:          true,
				TradeFetching:          true,
				OrderbookFetching:      true,
				Subscribe:              true,
				Unsubscribe:            true,
				GetOrders:              true,
				AuthenticatedEndpoints: true,
			},
			WithdrawPermissions: exchange.AutoWithdrawCryptoWithSetup,
			Kline: kline.ExchangeCapabilitiesSupported{
				Intervals: true,
			},
			FuturesCapabilities: exchange.FuturesCapabilities{
				FundingRates: true,
				FundingRateBatching: map[asset.Item]bool{
					asset.PerpetualContract: true,
				},
				SupportedFundingRateFrequencies: map[kline.Interval]bool{
					kline.OneHour: true,
				},
				Leverage: true,
				OpenInterest: exchange.OpenInterestSupport{
					Supported:         true,
					SupportsRestBatch: true,
				},
			},
		},
		Enabled: exchange.FeaturesEnabled{
			AutoPairUpdates: true,
			Kline: kline.ExchangeCapabilitiesEnabled{
				Intervals: kline.DeployExchangeIntervals(
					kline.IntervalCapacity{Interval: kline.OneMin},
					kline.IntervalCapacity{Interval: kline.ThreeMin},
					kline.IntervalCapacity{Interval: kline.FiveMin},
					kline.IntervalCapacity{Interval: kline.FifteenMin},
					kline.IntervalCapacity{Interval: kline.ThirtyMin},
					kline.IntervalCapacity{Interval: kline.OneHour},
					kline.IntervalCapacity{Interval: kline.TwoHour},
					kline.IntervalCapacity{Interval: kline.FourHour},
					kline.IntervalCapacity{Interval: kline.EightHour},
					kline.IntervalCapacity{Interval: kline.TwelveHour},
					kline.IntervalCapacity{Interval: kline.OneDay},
					kline.IntervalCapacity{Interval: kline.ThreeDay},
					kline.IntervalCapacity{Interval: kline.OneWeek},
					kline.IntervalCapacity{Interval: kline.OneMonth},
				),
				GlobalResultLimit: maximumCandleCount,
			},
		},
		Subscriptions: defaultSubscriptions.Clone(),
	}

	var err error
	e.Requester, err = request.New(e.Name, common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout), request.WithLimiter(GetRateLimits()))
	logDefaultError(err)
	e.API.Endpoints = e.NewEndpoints()
	logDefaultError(e.API.Endpoints.SetDefaultEndpoints(map[exchange.URL]string{
		exchange.RestSpot:      apiURL,
		exchange.WebsocketSpot: websocketURL,
	}))
	e.Websocket = websocket.NewManager()
	e.WebsocketResponseMaxLimit = exchange.DefaultWebsocketResponseMaxLimit
	e.WebsocketResponseCheckTimeout = exchange.DefaultWebsocketResponseCheckTimeout
	e.pairMappingsMu.Lock()
	e.pairMappings = make(map[asset.Item][]pairMapping)
	e.pairMappingsMu.Unlock()
	e.pairMappingsFetchMu.Lock()
	e.pairMappingMisses = make(map[string]time.Time)
	e.pairMappingsFetchMu.Unlock()
	e.authorityValidationMu.Lock()
	e.authorityValidationKey = authorityValidationKey{}
	e.authorityValidated = false
	e.authorityValidationMu.Unlock()
	e.websocketPendingMu.Lock()
	e.websocketPending = make(map[websocketPendingKey]*websocketPendingOperation)
	e.websocketPendingMu.Unlock()
}

// Setup sets the exchange configuration profile
// With useSandbox set, endpoints still at their production defaults move to testnet; custom endpoints are kept
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
	for _, endpoint := range []struct {
		kind       exchange.URL
		production string
		sandbox    string
	}{
		{kind: exchange.RestSpot, production: apiURL, sandbox: testnetAPIURL},
		{kind: exchange.WebsocketSpot, production: websocketURL, sandbox: testnetWebsocketURL},
	} {
		runningURL, err := e.API.Endpoints.GetURL(endpoint.kind)
		if err != nil {
			return err
		}
		runningURL = strings.TrimRight(runningURL, "/")
		switch {
		case exch.UseSandbox && runningURL == endpoint.production:
			if err := e.API.Endpoints.SetRunningURL(endpoint.kind.String(), endpoint.sandbox); err != nil {
				return err
			}
		case !exch.UseSandbox && runningURL == endpoint.sandbox:
			return fmt.Errorf("%w: %s uses the testnet URL while useSandbox is false", errEndpointEnvironment, endpoint.kind)
		}
	}
	wsURL, err := e.API.Endpoints.GetURL(exchange.WebsocketSpot)
	if err != nil {
		return err
	}
	if err := e.Websocket.Setup(&websocket.ManagerSetup{
		ExchangeConfig:                         exch,
		DefaultURL:                             websocketURL,
		RunningURL:                             wsURL,
		RunningURLAuth:                         wsURL,
		Connector:                              e.WsConnect,
		Subscriber:                             e.Subscribe,
		Unsubscriber:                           e.Unsubscribe,
		GenerateSubscriptions:                  e.generateSubscriptions,
		Features:                               &e.Features.Supports.WebsocketCapabilities,
		TradeFeed:                              exch.Features.Enabled.TradeFeed,
		FillsFeed:                              exch.Features.Enabled.FillsFeed,
		MaxWebsocketSubscriptionsPerConnection: maximumWebsocketSubscriptions,
	}); err != nil {
		return err
	}
	connectionRateLimit := request.NewWeightedRateLimitByDuration(websocketMessageInterval)
	if err := e.Websocket.SetupNewConnection(&websocket.ConnectionSetup{
		RateLimit:            connectionRateLimit,
		ResponseCheckTimeout: exch.WebsocketResponseCheckTimeout,
		ResponseMaxLimit:     exch.WebsocketResponseMaxLimit,
	}); err != nil {
		return err
	}
	return e.Websocket.SetupNewConnection(&websocket.ConnectionSetup{
		RateLimit:            connectionRateLimit,
		ResponseCheckTimeout: exch.WebsocketResponseCheckTimeout,
		ResponseMaxLimit:     exch.WebsocketResponseMaxLimit,
		URL:                  wsURL,
		Authenticated:        true,
	})
}

// FetchTradablePairs returns the active spot markets, or the active perpetual markets of every perpetual DEX
// A display pair listed by more than one market is ambiguous, so it is skipped rather than mapped to either
func (e *Exchange) FetchTradablePairs(ctx context.Context, a asset.Item) (currency.Pairs, error) {
	if !e.SupportsAsset(a) {
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	var candidates []pairMapping
	var err error
	switch a {
	case asset.PerpetualContract:
		candidates, err = e.fetchPerpetualPairMappings(ctx)
	case asset.Spot:
		candidates, err = e.fetchSpotPairMappings(ctx)
	default:
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	if err != nil {
		return nil, err
	}
	counts := make(map[currency.Pair]int, len(candidates))
	for i := range candidates {
		counts[candidates[i].pair]++
	}
	pairs := make(currency.Pairs, 0, len(candidates))
	mappings := make([]pairMapping, 0, len(candidates))
	reported := make(map[currency.Pair]struct{})
	for i := range candidates {
		if counts[candidates[i].pair] > 1 {
			if _, ok := reported[candidates[i].pair]; !ok {
				log.Warnf(log.ExchangeSys, "%s skipping ambiguous %s display pair %s", e.Name, a, candidates[i].pair)
				reported[candidates[i].pair] = struct{}{}
			}
			continue
		}
		pairs = append(pairs, candidates[i].pair)
		mappings = append(mappings, candidates[i])
	}
	e.setPairMappings(a, mappings)
	return pairs, nil
}

// fetchPerpetualPairMappings maps every active perpetual market, quoted in its DEX's collateral token
func (e *Exchange) fetchPerpetualPairMappings(ctx context.Context) ([]pairMapping, error) {
	dexes, err := e.getPerpetualDEXNames(ctx)
	if err != nil {
		return nil, err
	}
	collateralNames := map[uint64]currency.Code{0: currency.USDC}
	var candidates []pairMapping
	for dexIndex, dex := range dexes {
		metadata, err := e.GetPerpetualMetadata(ctx, dex)
		if err != nil {
			return nil, err
		}
		collateralName, ok := collateralNames[metadata.CollateralToken]
		if !ok {
			if collateralNames, err = e.getSpotTokenNames(ctx); err != nil {
				return nil, err
			}
			if collateralName, ok = collateralNames[metadata.CollateralToken]; !ok {
				return nil, fmt.Errorf("%w: collateral token index %d", errSpotTokenNotFound, metadata.CollateralToken)
			}
		}
		if dexIndex != 0 && len(metadata.Universe) > builderPerpetualDEXAssetStride {
			return nil, fmt.Errorf("%w: DEX %q has %d markets, maximum %d", errInvalidPerpetualDEX, dex, len(metadata.Universe), builderPerpetualDEXAssetStride)
		}
		var offset uint64
		if dexIndex != 0 {
			offset = builderPerpetualAssetIDBase + uint64(dexIndex)*builderPerpetualDEXAssetStride
		}
		for marketIndex := range metadata.Universe {
			market := &metadata.Universe[marketIndex]
			if market.IsDelisted {
				continue
			}
			if dex != "" && !strings.HasPrefix(market.Name, dex+":") {
				return nil, fmt.Errorf("%w: market %q is not scoped to DEX %q", errInvalidPerpetualDEX, market.Name, dex)
			}
			pair, err := currency.NewPairFromStrings(market.Name, collateralName.String())
			if err != nil {
				log.Warnf(log.ExchangeSys, "%s skipping perpetual market %q: %s", e.Name, market.Name, err)
				continue
			}
			candidates = append(candidates, pairMapping{
				pair:         pair,
				coin:         market.Name,
				dex:          dex,
				assetID:      offset + uint64(marketIndex),
				sizeDecimals: market.SizeDecimals,
				maxLeverage:  market.MaxLeverage,
				onlyIsolated: market.OnlyIsolated || market.MarginMode != "",
			})
		}
	}
	return candidates, nil
}

// getSpotTokenNames maps spot token indices to names, which builder DEXs use for their collateral token
func (e *Exchange) getSpotTokenNames(ctx context.Context) (map[uint64]currency.Code, error) {
	metadata, err := e.GetSpotMetadata(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[uint64]currency.Code, len(metadata.Tokens))
	for i := range metadata.Tokens {
		index := metadata.Tokens[i].Index
		if _, exists := names[index]; exists {
			return nil, fmt.Errorf("%w: duplicate spot token index %d", errUnexpectedResponseLength, index)
		}
		name := metadata.Tokens[i].Name
		if name.IsEmpty() {
			return nil, fmt.Errorf("%w: token index %d has no name", errSpotTokenNotFound, index)
		}
		if index == 0 && !name.Equal(currency.USDC) {
			return nil, fmt.Errorf("%w: token index 0 is %s", errSpotTokenNotFound, name)
		}
		names[index] = name
	}
	return names, nil
}

// fetchSpotPairMappings maps every spot market to its base and quote token names
func (e *Exchange) fetchSpotPairMappings(ctx context.Context) ([]pairMapping, error) {
	metadata, err := e.GetSpotMetadata(ctx)
	if err != nil {
		return nil, err
	}
	tokens := make(map[uint64]*SpotTokenMetadata, len(metadata.Tokens))
	for i := range metadata.Tokens {
		if _, exists := tokens[metadata.Tokens[i].Index]; exists {
			return nil, fmt.Errorf("%w: duplicate spot token index %d", errUnexpectedResponseLength, metadata.Tokens[i].Index)
		}
		tokens[metadata.Tokens[i].Index] = &metadata.Tokens[i]
	}
	candidates := make([]pairMapping, 0, len(metadata.Universe))
	marketIndices := make(map[uint64]struct{}, len(metadata.Universe))
	marketNames := make(map[string]struct{}, len(metadata.Universe))
	for _, market := range metadata.Universe {
		if _, exists := marketIndices[market.Index]; exists {
			return nil, fmt.Errorf("%w: duplicate spot market index %d", errUnexpectedResponseLength, market.Index)
		}
		marketIndices[market.Index] = struct{}{}
		if _, exists := marketNames[market.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate spot market name %q", errUnexpectedResponseLength, market.Name)
		}
		marketNames[market.Name] = struct{}{}
		if len(market.Tokens) != 2 {
			log.Warnf(log.ExchangeSys, "%s skipping spot market %q: %s; got %d tokens", e.Name, market.Name, errInvalidSpotTokenCount, len(market.Tokens))
			continue
		}
		baseToken, ok := tokens[market.Tokens[0]]
		if !ok {
			log.Warnf(log.ExchangeSys, "%s skipping spot market %q: %s for base index %d", e.Name, market.Name, errSpotTokenNotFound, market.Tokens[0])
			continue
		}
		quoteToken, ok := tokens[market.Tokens[1]]
		if !ok {
			log.Warnf(log.ExchangeSys, "%s skipping spot market %q: %s for quote index %d", e.Name, market.Name, errSpotTokenNotFound, market.Tokens[1])
			continue
		}
		if baseToken.Name.IsEmpty() || quoteToken.Name.IsEmpty() {
			log.Warnf(log.ExchangeSys, "%s skipping spot market %q with base %q and quote %q", e.Name, market.Name, baseToken.Name, quoteToken.Name)
			continue
		}
		pair := currency.NewPair(baseToken.Name, quoteToken.Name)
		candidates = append(candidates, pairMapping{
			pair:         pair,
			coin:         market.Name,
			assetID:      spotAssetIDBase + market.Index,
			sizeDecimals: baseToken.SizeDecimals,
		})
	}
	return candidates, nil
}

// UpdateTradablePairs updates all available Hyperliquid markets
func (e *Exchange) UpdateTradablePairs(ctx context.Context) error {
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.FetchTradablePairs(ctx, a)
		if err != nil {
			return err
		}
		if err := e.UpdatePairs(pairs, a, false); err != nil {
			return err
		}
	}
	return e.EnsureOnePairEnabled()
}

// UpdateTickers updates the tickers of an asset type's enabled pairs from the batch market contexts
// Contexts carry no last trade price, so the ticker's last price is the mid price, falling back to the mark price
func (e *Exchange) UpdateTickers(ctx context.Context, a asset.Item) error {
	if !e.SupportsAsset(a) {
		return fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	pairs, err := e.GetEnabledPairs(a)
	if err != nil {
		return err
	}
	var prices []ticker.Price
	var errs error
	switch a {
	case asset.PerpetualContract:
		contexts := make(map[string]map[string]*PerpetualAssetContext)
		dexErrors := make(map[string]error)
		for _, pair := range pairs {
			mapping, err := e.getPairMapping(ctx, pair, a)
			if err != nil {
				errs = common.AppendError(errs, fmt.Errorf("%s: %w", pair, err))
				continue
			}
			dexContexts, loaded := contexts[mapping.dex]
			if !loaded && dexErrors[mapping.dex] == nil {
				resp, err := e.GetPerpetualMetadataAndAssetContexts(ctx, mapping.dex)
				if err != nil {
					dexErrors[mapping.dex] = err
				} else {
					dexContexts = make(map[string]*PerpetualAssetContext, len(resp.AssetContexts))
					for i := range resp.Metadata.Universe {
						dexContexts[resp.Metadata.Universe[i].Name] = &resp.AssetContexts[i]
					}
					contexts[mapping.dex] = dexContexts
				}
			}
			if dexErrors[mapping.dex] != nil {
				errs = common.AppendError(errs, fmt.Errorf("%s: %w", pair, dexErrors[mapping.dex]))
				continue
			}
			market, ok := dexContexts[mapping.coin]
			if !ok {
				errs = common.AppendError(errs, fmt.Errorf("%w: %s", errAssetContextNotFound, mapping.coin))
				continue
			}
			prices = append(prices, *e.perpetualTickerPrice(pair, market))
		}
	case asset.Spot:
		resp, err := e.GetSpotMetadataAndAssetContexts(ctx)
		if err != nil {
			return err
		}
		contexts := make(map[string]*SpotAssetContext, len(resp.AssetContexts))
		for i := range resp.AssetContexts {
			contexts[resp.AssetContexts[i].Coin] = &resp.AssetContexts[i]
		}
		for _, pair := range pairs {
			coin, err := e.getCoin(ctx, pair, a)
			if err != nil {
				errs = common.AppendError(errs, fmt.Errorf("%s: %w", pair, err))
				continue
			}
			market, ok := contexts[coin]
			if !ok {
				errs = common.AppendError(errs, fmt.Errorf("%w: %s", errAssetContextNotFound, coin))
				continue
			}
			prices = append(prices, *e.spotTickerPrice(pair, market))
		}
	default:
		return fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	if len(prices) == 0 {
		return errs
	}
	_, err = ticker.ProcessBatch(prices)
	return common.AppendError(errs, err)
}

// UpdateTicker updates and returns one ticker
func (e *Exchange) UpdateTicker(ctx context.Context, p currency.Pair, a asset.Item) (*ticker.Price, error) {
	if err := e.UpdateTickers(ctx, a); err != nil {
		return nil, err
	}
	return ticker.GetTicker(e.Name, p, a)
}

// UpdateOrderbook updates and returns one L2 orderbook snapshot
func (e *Exchange) UpdateOrderbook(ctx context.Context, p currency.Pair, a asset.Item) (*orderbook.Book, error) {
	coin, err := e.getCoin(ctx, p, a)
	if err != nil {
		return nil, err
	}
	resp, err := e.GetL2Book(ctx, &L2BookRequest{Coin: coin})
	if err != nil {
		return nil, err
	}
	book, err := e.convertL2Book(resp, p, a)
	if err != nil {
		return nil, err
	}
	if err := book.Process(); err != nil {
		return nil, err
	}
	return orderbook.Get(e.Name, p, a)
}

// GetRecentTrades returns recent public trades for a pair and asset type
func (e *Exchange) GetRecentTrades(ctx context.Context, p currency.Pair, a asset.Item) ([]trade.Data, error) {
	coin, err := e.getCoin(ctx, p, a)
	if err != nil {
		return nil, err
	}
	resp, err := e.GetRecentTradesForCoin(ctx, coin)
	if err != nil {
		return nil, err
	}
	trades := make([]trade.Data, len(resp))
	for i := range resp {
		if trades[i], err = e.convertTrade(&resp[i], p, a); err != nil {
			return nil, err
		}
	}
	trade.SortByDate(trades)
	return trades, e.AddTradesToBuffer(trades...)
}

// GetHistoricTrades is not supported by Hyperliquid's recent trades endpoint
func (e *Exchange) GetHistoricTrades(context.Context, currency.Pair, asset.Item, time.Time, time.Time) ([]trade.Data, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetHistoricCandles returns candles within the requested time range
func (e *Exchange) GetHistoricCandles(ctx context.Context, p currency.Pair, a asset.Item, interval kline.Interval, start, end time.Time) (*kline.Item, error) {
	req, err := e.GetKlineRequest(p, a, interval, start, end, true)
	if err != nil {
		return nil, err
	}
	coin, err := e.getCoin(ctx, p, a)
	if err != nil {
		return nil, err
	}
	resp, err := e.GetCandleSnapshot(ctx, &CandleSnapshotRequest{
		Coin:      coin,
		Interval:  req.ExchangeInterval,
		StartTime: req.Start,
		// Hyperliquid's end time is inclusive, but kline request ranges exclude their end
		EndTime: req.End.Add(-time.Millisecond),
	})
	if err != nil {
		return nil, err
	}
	candles := make([]kline.Candle, len(resp))
	for i := range resp {
		candles[i] = convertCandle(&resp[i])
	}
	return req.ProcessResponse(candles)
}

// GetHistoricCandlesExtended is not supported because Hyperliquid retains only the latest 5000 candles
func (e *Exchange) GetHistoricCandlesExtended(context.Context, currency.Pair, asset.Item, kline.Interval, time.Time, time.Time) (*kline.Item, error) {
	return nil, common.ErrFunctionNotSupported
}

// UpdateAccountBalances retrieves balances for the configured account, vault or subaccount
// Unified and portfolio margin accounts hold one balance across spot and perpetuals, which is stored under spot; their
// perpetual DEX balances are saved empty so stale separate balances are cleared without double counting
func (e *Exchange) UpdateAccountBalances(ctx context.Context, a asset.Item) (accounts.SubAccounts, error) {
	if a != asset.Spot && a != asset.PerpetualContract {
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	address, err := e.getWatchAddress(ctx)
	if err != nil {
		return nil, err
	}
	var subAccounts accounts.SubAccounts
	switch a {
	case asset.Spot:
		state, err := e.GetSpotClearinghouseState(ctx, address)
		if err != nil {
			return nil, err
		}
		subAccount := accounts.NewSubAccount(a, address)
		for i := range state.Balances {
			subAccount.Balances.Set(state.Balances[i].Coin, accounts.Balance{
				Total: state.Balances[i].Total.Float64(),
				Hold:  state.Balances[i].Hold.Float64(),
				Free:  state.Balances[i].Total.Float64() - state.Balances[i].Hold.Float64(),
			})
		}
		subAccounts = accounts.SubAccounts{subAccount}
	case asset.PerpetualContract:
		mode, err := e.GetUserAbstraction(ctx, address)
		if err != nil {
			return nil, err
		}
		dexes, err := e.getPerpetualDEXNames(ctx)
		if err != nil {
			return nil, err
		}
		subAccounts = make(accounts.SubAccounts, 0, len(dexes))
		if mode == AccountAbstractionUnified || mode == AccountAbstractionPortfolio {
			for i := range dexes {
				subAccounts = append(subAccounts, accounts.NewSubAccount(a, dexSubAccountID(address, dexes[i])))
			}
			break
		}
		tokenNames, err := e.getSpotTokenNames(ctx)
		if err != nil {
			return nil, err
		}
		for i := range dexes {
			metadata, err := e.GetPerpetualMetadata(ctx, dexes[i])
			if err != nil {
				return nil, err
			}
			collateral, ok := tokenNames[metadata.CollateralToken]
			if !ok {
				return nil, fmt.Errorf("%w: collateral token index %d", errSpotTokenNotFound, metadata.CollateralToken)
			}
			state, err := e.GetClearinghouseState(ctx, address, dexes[i])
			if err != nil {
				return nil, err
			}
			subAccount := accounts.NewSubAccount(a, dexSubAccountID(address, dexes[i]))
			subAccount.Balances.Set(collateral, accounts.Balance{
				Total: state.MarginSummary.AccountValue.Float64(),
				Hold:  state.MarginSummary.TotalMarginUsed.Float64(),
				Free:  state.Withdrawable.Float64(),
			})
			subAccounts = append(subAccounts, subAccount)
		}
	}
	return subAccounts, e.Accounts.Save(ctx, subAccounts, true)
}

// GetAccountFundingHistory returns the configured account's deposits, withdrawals, transfers, rewards and other
// non-funding ledger movements
func (e *Exchange) GetAccountFundingHistory(ctx context.Context) ([]exchange.FundingHistory, error) {
	user, err := e.getWatchAddress(ctx)
	if err != nil {
		return nil, err
	}
	updates, err := e.getUserNonFundingLedgerUpdatesPaginated(ctx, user, time.UnixMilli(1).UTC())
	if err != nil {
		return nil, err
	}
	result := make([]exchange.FundingHistory, len(updates))
	for i := range updates {
		if result[i], err = e.convertUserLedgerUpdate(&updates[i]); err != nil {
			return nil, err
		}
	}
	slices.SortStableFunc(result, func(a, b exchange.FundingHistory) int { return a.Timestamp.Compare(b.Timestamp) })
	return result, nil
}

// GetWithdrawalsHistory returns processed bridge withdrawals
func (e *Exchange) GetWithdrawalsHistory(ctx context.Context, c currency.Code, _ asset.Item) ([]exchange.WithdrawalHistory, error) {
	if !c.IsEmpty() && !c.Equal(currency.USDC) {
		return []exchange.WithdrawalHistory{}, nil
	}
	user, err := e.getWatchAddress(ctx)
	if err != nil {
		return nil, err
	}
	updates, err := e.getUserNonFundingLedgerUpdatesPaginated(ctx, user, time.UnixMilli(1).UTC())
	if err != nil {
		return nil, err
	}
	result := make([]exchange.WithdrawalHistory, 0)
	for i := range updates {
		if updates[i].Delta.Type != "withdraw" {
			continue
		}
		result = append(result, exchange.WithdrawalHistory{
			Status:       "processed",
			TransferID:   updates[i].Hash,
			Description:  "Hyperliquid bridge withdrawal",
			Timestamp:    updates[i].Time.Time().UTC(),
			Currency:     currency.USDC.String(),
			Amount:       math.Abs(updates[i].Delta.USDC.Float64()),
			Fee:          math.Abs(updates[i].Delta.Fee.Float64()),
			TransferType: "withdrawal",
			CryptoTxID:   updates[i].Hash,
			CryptoChain:  e.getBridgeChain(),
		})
	}
	return result, nil
}

// GetServerTime is not supported by Hyperliquid
func (e *Exchange) GetServerTime(context.Context, asset.Item) (time.Time, error) {
	return time.Time{}, common.ErrFunctionNotSupported
}

// SubmitOrder submits a limit, slippage-bounded market or trigger order, with any grouped TP/SL children
func (e *Exchange) SubmitOrder(ctx context.Context, submit *order.Submit) (*order.SubmitResponse, error) {
	if err := submit.Validate(protocol.TradingRequirements{}); err != nil {
		return nil, err
	}
	if _, _, err := e.getSigningCredentials(ctx); err != nil {
		return nil, err
	}
	orders, mapping, grouping, err := e.buildOrderRequests(ctx, submit)
	if err != nil {
		return nil, err
	}
	statuses, err := e.PlaceOrders(ctx, &PlaceOrdersRequest{Orders: orders, Grouping: grouping})
	if err != nil {
		return nil, err
	}
	parent := statuses[0]
	if parent.Error != "" {
		return nil, fmt.Errorf("%w: %s", order.ErrUnableToPlaceOrder, parent.Error)
	}
	if parent.Waiting != "" {
		return nil, fmt.Errorf("%w: parent order cannot be %s", errActionStatusMalformed, parent.Waiting)
	}
	var timeInForce string
	if orders[0].Limit != nil {
		timeInForce = orders[0].Limit.TimeInForce
	}
	var orderID uint64
	status := order.New
	remainingAmount := submit.Amount
	switch {
	case parent.Resting != nil:
		orderID = parent.Resting.OrderID
	case parent.Filled != nil:
		orderID = parent.Filled.OrderID
		if status, remainingAmount, err = deriveFilledOrderState(submit.Amount, parent.Filled.TotalSize.Float64(), mapping.sizeDecimals, timeInForce); err != nil {
			return nil, err
		}
	}
	result, _ := submit.DeriveSubmitResponse(strconv.FormatUint(orderID, 10)) // An accepted order always has a non-zero order ID
	result.Status = status
	result.Price = orders[0].Price
	result.TimeInForce = order.UnknownTIF
	if timeInForce != "" {
		result.TimeInForce, _ = parseTimeInForce(timeInForce) // buildOrderRequest only sets time in force values it can parse
	}
	if parent.Filled != nil {
		result.AverageExecutedPrice = parent.Filled.AveragePrice.Float64()
	}
	result.RemainingAmount = remainingAmount
	var childErrors error
	for i := 1; i < len(statuses); i++ {
		if statuses[i].Error != "" {
			childErrors = common.AppendError(childErrors, fmt.Errorf("child %d: %s", i, statuses[i].Error))
		}
	}
	if childErrors != nil {
		result.SubmissionError = fmt.Errorf("%w: %w", errGroupedOrderChildFailure, childErrors)
	}
	return result, nil
}

// ModifyOrder replaces a resting limit order, keeping any fields the modification omits
// Without always place, Hyperliquid only accepts a replacement that rests: it must not be a trigger order and must be
// ALO or a GTC that would not execute, which then rests as ALO
func (e *Exchange) ModifyOrder(ctx context.Context, modify *order.Modify) (*order.ModifyResponse, error) {
	if err := modify.Validate(); err != nil {
		return nil, err
	}
	if _, _, err := e.getSigningCredentials(ctx); err != nil {
		return nil, err
	}
	modifyRequest := OrderModification{ClientOrderID: modify.ClientOrderID}
	identifier := modify.OrderID
	if modify.OrderID != "" {
		id, err := strconv.ParseUint(modify.OrderID, 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("%w: %q", order.ErrOrderIDNotSet, modify.OrderID)
		}
		modifyRequest = OrderModification{OrderID: id}
	} else {
		if err := validateClientOrderID(modify.ClientOrderID); err != nil {
			return nil, err
		}
		identifier = strings.ToLower(modify.ClientOrderID)
	}
	existing, err := e.GetOrderInfo(ctx, identifier, modify.Pair, modify.AssetType)
	if err != nil {
		return nil, err
	}
	if existing.IsInactive() || existing.RemainingAmount <= 0 {
		return nil, fmt.Errorf("%w: %s", errOrderNotModifiable, identifier)
	}
	orderType := cmp.Or(modify.Type, existing.Type)
	if orderType != order.Limit {
		return nil, fmt.Errorf("%w: %w: %s", order.ErrUnsupportedOrderType, errModifyOrderTypeUnsupported, orderType)
	}
	timeInForce := cmp.Or(modify.TimeInForce, existing.TimeInForce)
	if timeInForce == order.ImmediateOrCancel {
		return nil, fmt.Errorf("%w: %w: %s", order.ErrUnsupportedTimeInForce, errModifyOrderTypeUnsupported, timeInForce)
	}
	side := cmp.Or(modify.Side, existing.Side)
	amount := cmp.Or(modify.Amount, existing.RemainingAmount)
	price := cmp.Or(modify.Price, existing.Price)
	clientOrderID := cmp.Or(modify.NewClientOrderID, existing.ClientOrderID)
	replacement, mapping, err := e.buildOrderRequest(ctx, modify.Pair, modify.AssetType, orderType, side, timeInForce, amount, price, modify.TriggerPrice, modify.SlippageTolerance, existing.ReduceOnly, clientOrderID)
	if err != nil {
		return nil, err
	}
	modifyRequest.Order = replacement
	statuses, err := e.ModifyOrders(ctx, &ModifyOrdersRequest{Modifies: []OrderModification{modifyRequest}})
	if err != nil {
		return nil, err
	}
	if statuses[0].Error != "" {
		return nil, fmt.Errorf("%w: %s", order.ErrUnableToPlaceOrder, statuses[0].Error)
	}
	status := order.New
	remainingAmount := amount
	orderID, _ := strconv.ParseUint(existing.OrderID, 10, 64) // GetOrderInfo formats a parsed numeric order ID
	switch {
	case statuses[0].Resting != nil:
		orderID = statuses[0].Resting.OrderID
	case statuses[0].Filled != nil:
		orderID = statuses[0].Filled.OrderID
		if status, remainingAmount, err = deriveFilledOrderState(amount, statuses[0].Filled.TotalSize.Float64(), mapping.sizeDecimals, replacement.Limit.TimeInForce); err != nil {
			return nil, err
		}
	}
	return &order.ModifyResponse{
		Exchange:        e.Name,
		OrderID:         strconv.FormatUint(orderID, 10),
		ClientOrderID:   clientOrderID,
		Pair:            modify.Pair,
		Type:            orderType,
		Side:            side,
		Status:          status,
		AssetType:       modify.AssetType,
		TimeInForce:     order.PostOnly,
		Price:           replacement.Price,
		Amount:          amount,
		RemainingAmount: remainingAmount,
		Date:            existing.Date,
		LastUpdated:     time.Now().UTC(),
	}, nil
}

// CancelOrder cancels one order by order ID or client order ID
func (e *Exchange) CancelOrder(ctx context.Context, cancel *order.Cancel) error {
	if cancel == nil {
		return order.ErrCancelOrderIsNil
	}
	_, err := e.cancelOrders(ctx, []order.Cancel{*cancel})
	return err
}

// CancelBatchOrders cancels a batch of orders by order ID or client order ID
func (e *Exchange) CancelBatchOrders(ctx context.Context, cancels []order.Cancel) (*order.CancelBatchResponse, error) {
	statuses, err := e.cancelOrders(ctx, cancels)
	return &order.CancelBatchResponse{Status: statuses}, err
}

// CancelAllOrders cancels every open order of an asset type, optionally limited to one pair
func (e *Exchange) CancelAllOrders(ctx context.Context, cancel *order.Cancel) (order.CancelAllResponse, error) {
	if cancel == nil {
		return order.CancelAllResponse{}, order.ErrCancelOrderIsNil
	}
	if !cancel.AssetType.IsValid() || !e.SupportsAsset(cancel.AssetType) {
		return order.CancelAllResponse{}, fmt.Errorf("%w: %s", asset.ErrNotSupported, cancel.AssetType)
	}
	address, err := e.getWatchAddress(ctx)
	if err != nil {
		return order.CancelAllResponse{}, err
	}
	dexes, err := e.openOrderDEXes(ctx, cancel.AssetType)
	if err != nil {
		return order.CancelAllResponse{}, err
	}
	var openOrders []BasicOrder
	for _, dex := range dexes {
		orders, err := e.GetOpenOrders(ctx, address, dex)
		if err != nil {
			return order.CancelAllResponse{}, err
		}
		openOrders = append(openOrders, orders...)
	}
	cancels := make([]order.Cancel, 0, len(openOrders))
	var errs error
	for i := range openOrders {
		mapping, a, err := e.getPairMappingByCoin(ctx, openOrders[i].Coin)
		if err != nil {
			errs = common.AppendError(errs, fmt.Errorf("order %d: %w", openOrders[i].OrderID, err))
			continue
		}
		if a != cancel.AssetType || (!cancel.Pair.IsEmpty() && !mapping.pair.Equal(cancel.Pair)) {
			continue
		}
		cancels = append(cancels, order.Cancel{
			OrderID:   strconv.FormatUint(openOrders[i].OrderID, 10),
			AssetType: a,
			Pair:      mapping.pair,
		})
	}
	statuses := make(map[string]string, len(cancels))
	for chunk := range slices.Chunk(cancels, maximumActionBatchSize) {
		batchStatuses, err := e.cancelOrders(ctx, chunk)
		maps.Copy(statuses, batchStatuses)
		errs = common.AppendError(errs, err)
	}
	return order.CancelAllResponse{Status: statuses}, errs
}

// GetOrderInfo returns one order by order ID or client order ID
func (e *Exchange) GetOrderInfo(ctx context.Context, orderID string, pair currency.Pair, a asset.Item) (*order.Detail, error) {
	if orderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	if a != asset.Empty && !e.SupportsAsset(a) {
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	address, err := e.getWatchAddress(ctx)
	if err != nil {
		return nil, err
	}
	statusRequest := &OrderStatusRequest{User: address}
	if statusRequest.OrderID, err = strconv.ParseUint(orderID, 10, 64); err != nil {
		statusRequest.ClientOrderID = orderID
	}
	response, err := e.GetOrderStatus(ctx, statusRequest)
	if err != nil {
		return nil, err
	}
	if response.Status != "order" || response.Order == nil {
		return nil, order.ErrOrderNotFound
	}
	converted, err := e.convertOrder(ctx, &response.Order.Order, response.Order.Status, response.Order.StatusTimestamp.Time())
	if err != nil {
		return nil, err
	}
	if (!pair.IsEmpty() && !converted.Pair.Equal(pair)) || (a != asset.Empty && converted.AssetType != a) {
		return nil, order.ErrOrderNotFound
	}
	return &converted, nil
}

// GetDepositAddress is not supported: Hyperliquid credits bridge deposits to the address that sent them, so there is no
// deposit address to return
func (e *Exchange) GetDepositAddress(context.Context, currency.Code, string, string) (*deposit.Address, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetAvailableTransferChains returns the configured environment's Arbitrum network, which USDC withdrawals use
func (e *Exchange) GetAvailableTransferChains(_ context.Context, c currency.Code) ([]string, error) {
	if c.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if !c.Equal(currency.USDC) {
		return nil, fmt.Errorf("%w: %s", errTransferCurrencyInvalid, c)
	}
	return []string{e.getBridgeChain()}, nil
}

// WithdrawCryptocurrencyFunds submits a USDC bridge withdrawal, or a Core USDC send when InternalTransfer is set
func (e *Exchange) WithdrawCryptocurrencyFunds(ctx context.Context, withdrawRequest *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	if err := withdrawRequest.Validate(); err != nil {
		return nil, err
	}
	if !withdrawRequest.Currency.Equal(currency.USDC) {
		return nil, fmt.Errorf("%w: %s", errTransferCurrencyInvalid, withdrawRequest.Currency)
	}
	if strings.TrimSpace(withdrawRequest.Crypto.AddressTag) != "" {
		return nil, errWithdrawalAddressTag
	}
	if withdrawRequest.Crypto.FeeAmount != 0 {
		return nil, errWithdrawalFeeInput
	}
	if chain := strings.TrimSpace(withdrawRequest.Crypto.Chain); chain != "" && !strings.EqualFold(chain, e.getBridgeChain()) {
		return nil, fmt.Errorf("%w: expected %s", errBridgeChainInvalid, e.getBridgeChain())
	}
	var nonce uint64
	var err error
	if withdrawRequest.InternalTransfer {
		nonce, err = e.SendCoreUSDC(ctx, withdrawRequest.Crypto.Address, withdrawRequest.Amount)
	} else {
		nonce, err = e.WithdrawFromBridge(ctx, withdrawRequest.Crypto.Address, withdrawRequest.Amount)
	}
	if err != nil {
		return nil, err
	}
	return &withdraw.ExchangeResponse{
		Name:   e.Name,
		ID:     strconv.FormatUint(nonce, 10),
		Status: "submitted",
	}, nil
}

// WithdrawFiatFunds is not supported by Hyperliquid
func (e *Exchange) WithdrawFiatFunds(context.Context, *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	return nil, common.ErrFunctionNotSupported
}

// WithdrawFiatFundsToInternationalBank is not supported by Hyperliquid
func (e *Exchange) WithdrawFiatFundsToInternationalBank(context.Context, *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetActiveOrders returns the configured account's open orders of an asset type, filtered by the request
// frontendOpenOrders returns only a DEX's 100 newest orders, so a DEX at that limit has its older orders added from
// openOrders, which omits their order type, time in force and trigger price
func (e *Exchange) GetActiveOrders(ctx context.Context, filter *order.MultiOrderRequest) (order.FilteredOrders, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	if !e.SupportsAsset(filter.AssetType) {
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, filter.AssetType)
	}
	address, err := e.getWatchAddress(ctx)
	if err != nil {
		return nil, err
	}
	dexes, err := e.openOrderDEXes(ctx, filter.AssetType)
	if err != nil {
		return nil, err
	}
	var converted []order.Detail
	var errs error
	for _, dex := range dexes {
		frontendOrders, err := e.GetFrontendOpenOrders(ctx, address, dex)
		if err != nil {
			return nil, err
		}
		listed := make(map[uint64]struct{}, len(frontendOrders))
		for i := range frontendOrders {
			listed[frontendOrders[i].OrderID] = struct{}{}
			detail, err := e.convertOrder(ctx, &frontendOrders[i], "open", time.Time{})
			if err != nil {
				errs = common.AppendError(errs, fmt.Errorf("order %d: %w", frontendOrders[i].OrderID, err))
				continue
			}
			if detail.AssetType == filter.AssetType {
				converted = append(converted, detail)
			}
		}
		if len(frontendOrders) < maximumFrontendOpenOrders {
			continue
		}
		basicOrders, err := e.GetOpenOrders(ctx, address, dex)
		if err != nil {
			return nil, err
		}
		for i := range basicOrders {
			if _, ok := listed[basicOrders[i].OrderID]; ok {
				continue
			}
			detail, err := e.convertOpenOrder(ctx, &basicOrders[i])
			if err != nil {
				errs = common.AppendError(errs, fmt.Errorf("order %d: %w", basicOrders[i].OrderID, err))
				continue
			}
			if detail.AssetType == filter.AssetType {
				converted = append(converted, detail)
			}
		}
	}
	return filter.Filter(e.Name, converted), errs
}

// convertOpenOrder converts an open order from openOrders, whose order type and time in force are unknown
func (e *Exchange) convertOpenOrder(ctx context.Context, source *BasicOrder) (order.Detail, error) {
	mapping, a, err := e.getPairMappingByCoin(ctx, source.Coin)
	if err != nil {
		return order.Detail{}, err
	}
	return e.convertBasicOrder(source, "open", time.Time{}, mapping.pair, a)
}

// GetOrderHistory returns the configured account's 2000 most recent orders of an asset type, filtered by the request
func (e *Exchange) GetOrderHistory(ctx context.Context, filter *order.MultiOrderRequest) (order.FilteredOrders, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	if !e.SupportsAsset(filter.AssetType) {
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, filter.AssetType)
	}
	address, err := e.getWatchAddress(ctx)
	if err != nil {
		return nil, err
	}
	history, err := e.GetHistoricalOrders(ctx, address)
	if err != nil {
		return nil, err
	}
	converted := make([]order.Detail, 0, len(history))
	var errs error
	for i := range history {
		detail, err := e.convertOrder(ctx, &history[i].Order, history[i].Status, history[i].StatusTimestamp.Time())
		if err != nil {
			errs = common.AppendError(errs, fmt.Errorf("order %d: %w", history[i].Order.OrderID, err))
			continue
		}
		if detail.AssetType == filter.AssetType {
			converted = append(converted, detail)
		}
	}
	return filter.Filter(e.Name, converted), errs
}

// GetFeeByType returns a maker or taker trade fee estimate, using the account's effective rates when credentials are
// valid and the base tier rates otherwise
// A pair enabled for both spot and perpetuals cannot identify its fee schedule, so it fails rather than guessing
func (e *Exchange) GetFeeByType(ctx context.Context, feeBuilder *exchange.FeeBuilder) (float64, error) {
	if feeBuilder == nil {
		return 0, common.ErrNilPointer
	}
	switch feeBuilder.FeeType {
	case exchange.CryptocurrencyTradeFee, exchange.OfflineTradeFee:
	default:
		return 0, common.ErrFunctionNotSupported
	}
	if feeBuilder.Pair.IsEmpty() {
		return 0, currency.ErrCurrencyPairEmpty
	}
	if feeBuilder.PurchasePrice < 0 || feeBuilder.Amount < 0 ||
		math.IsNaN(feeBuilder.PurchasePrice) || math.IsNaN(feeBuilder.Amount) ||
		math.IsInf(feeBuilder.PurchasePrice, 0) || math.IsInf(feeBuilder.Amount, 0) {
		return 0, fmt.Errorf("%w: invalid fee notional", order.ErrAmountIsInvalid)
	}
	var spot, perpetual bool
	if pairs, err := e.GetEnabledPairs(asset.Spot); err == nil {
		spot = pairs.Contains(feeBuilder.Pair, true)
	}
	if pairs, err := e.GetEnabledPairs(asset.PerpetualContract); err == nil {
		perpetual = pairs.Contains(feeBuilder.Pair, true)
	}
	if !spot && !perpetual {
		_, spot = e.lookupPairMapping(feeBuilder.Pair, asset.Spot)
		_, perpetual = e.lookupPairMapping(feeBuilder.Pair, asset.PerpetualContract)
	}
	if spot == perpetual {
		if spot {
			return 0, fmt.Errorf("%w: fee request does not identify spot or perpetual asset", errAmbiguousCoinMapping)
		}
		return 0, fmt.Errorf("%w: %s", errPairMappingNotFound, feeBuilder.Pair)
	}
	if (!e.AreCredentialsValid(ctx) || e.SkipAuthCheck) && feeBuilder.FeeType == exchange.CryptocurrencyTradeFee {
		feeBuilder.FeeType = exchange.OfflineTradeFee
	}
	var rate float64
	if feeBuilder.FeeType == exchange.OfflineTradeFee {
		switch {
		case spot && feeBuilder.IsMaker:
			rate = spotMakerBaseFeeRate
		case spot:
			rate = spotTakerBaseFeeRate
		case feeBuilder.IsMaker:
			rate = perpetualMakerBaseFeeRate
		default:
			rate = perpetualTakerBaseFeeRate
		}
	} else {
		address, err := e.getWatchAddress(ctx)
		if err != nil {
			return 0, err
		}
		fees, err := e.GetUserFees(ctx, address)
		if err != nil {
			return 0, err
		}
		switch {
		case spot && feeBuilder.IsMaker:
			rate = fees.UserSpotAddRate.Float64()
		case spot:
			rate = fees.UserSpotCrossRate.Float64()
		case feeBuilder.IsMaker:
			rate = fees.UserAddRate.Float64()
		default:
			rate = fees.UserCrossRate.Float64()
		}
	}
	return feeBuilder.PurchasePrice * feeBuilder.Amount * rate, nil
}

// ValidateAPICredentials validates the configured account, its vault or subaccount authority and any signing key
func (e *Exchange) ValidateAPICredentials(ctx context.Context, a asset.Item) error {
	if a != asset.Empty && !e.SupportsAsset(a) {
		return fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	credentials, err := e.GetCredentials(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", request.ErrAuthRequestFailed, err)
	}
	if _, err := e.validateCachedAuthority(ctx, credentials, true); err != nil {
		return fmt.Errorf("%w: %w", request.ErrAuthRequestFailed, err)
	}
	return nil
}

// SetLeverage sets a perpetual market's margin mode and whole-number leverage, on any perpetual DEX
func (e *Exchange) SetLeverage(ctx context.Context, a asset.Item, p currency.Pair, marginType margin.Type, amount float64, _ order.Side) error {
	if a != asset.PerpetualContract {
		return fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	mapping, err := e.getPairMapping(ctx, p, a)
	if err != nil {
		return err
	}
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || math.Trunc(amount) != amount ||
		mapping.maxLeverage == 0 || amount > float64(mapping.maxLeverage) {
		return fmt.Errorf("%w: requested %v, maximum %d", errInvalidLeverage, amount, mapping.maxLeverage)
	}
	var isCross bool
	switch marginType {
	case margin.Unset, margin.Multi:
		if mapping.onlyIsolated {
			return fmt.Errorf("%w: %w", margin.ErrMarginTypeUnsupported, errCrossMarginUnavailable)
		}
		isCross = true
	case margin.Isolated:
	default:
		return fmt.Errorf("%w: %s", margin.ErrMarginTypeUnsupported, marginType)
	}
	return e.UpdateLeverage(ctx, &UpdateLeverageRequest{Asset: mapping.assetID, IsCross: isCross, Leverage: uint64(amount)})
}

// GetLeverage returns the configured cross or isolated leverage of a perpetual market
func (e *Exchange) GetLeverage(ctx context.Context, a asset.Item, p currency.Pair, marginType margin.Type, _ order.Side) (float64, error) {
	if a != asset.PerpetualContract {
		return 0, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	mapping, err := e.getPairMapping(ctx, p, a)
	if err != nil {
		return 0, err
	}
	address, err := e.getWatchAddress(ctx)
	if err != nil {
		return 0, err
	}
	data, err := e.GetActiveAssetData(ctx, address, mapping.coin)
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(data.Leverage.Type) {
	case "cross":
		if marginType != margin.Unset && marginType != margin.Multi {
			return 0, fmt.Errorf("%w: requested %s, account uses cross", margin.ErrMarginTypeUnsupported, marginType)
		}
	case "isolated":
		if marginType != margin.Unset && marginType != margin.Isolated {
			return 0, fmt.Errorf("%w: requested %s, account uses isolated", margin.ErrMarginTypeUnsupported, marginType)
		}
	default:
		return 0, fmt.Errorf("%w: %q", margin.ErrMarginTypeUnsupported, data.Leverage.Type)
	}
	value := data.Leverage.Value.Float64()
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errInvalidLeverage
	}
	return value, nil
}

// GetFuturesContractDetails is not supported by this integration
func (e *Exchange) GetFuturesContractDetails(context.Context, asset.Item) ([]futures.Contract, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetLatestFundingRates returns the current hourly rates of one or all active perpetual markets
func (e *Exchange) GetLatestFundingRates(ctx context.Context, arg *fundingrate.LatestRateRequest) ([]fundingrate.LatestRateResponse, error) {
	if arg == nil {
		return nil, common.ErrNilPointer
	}
	if arg.Asset != asset.PerpetualContract {
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, arg.Asset)
	}
	if arg.IncludePredictedRate {
		return nil, fmt.Errorf("%w: predicted funding rates", common.ErrFunctionNotSupported)
	}
	var mappings []pairMapping
	if arg.Pair.IsEmpty() {
		var err error
		if mappings, err = e.getPerpetualPairMappings(ctx); err != nil {
			return nil, err
		}
	} else {
		mapping, err := e.getPairMapping(ctx, arg.Pair, arg.Asset)
		if err != nil {
			return nil, err
		}
		mappings = []pairMapping{mapping}
	}
	contexts, err := e.getPerpetualAssetContexts(ctx, mappings)
	if err != nil {
		return nil, err
	}
	checked := time.Now().UTC()
	responses := make([]fundingrate.LatestRateResponse, len(mappings))
	for i := range mappings {
		responses[i] = fundingrate.LatestRateResponse{
			Exchange:    e.Name,
			Asset:       asset.PerpetualContract,
			Pair:        mappings[i].pair,
			TimeChecked: checked,
			LatestRate: fundingrate.Rate{
				Time: checked.Truncate(time.Hour),
				Rate: contexts[i].Funding.Decimal(),
			},
			TimeOfNextRate: checked.Truncate(time.Hour).Add(time.Hour),
		}
	}
	if len(responses) == 0 {
		return nil, fundingrate.ErrNoFundingRatesFound
	}
	return responses, nil
}

// GetHistoricalFundingRates returns the hourly funding rates of one perpetual market, paging through the range
func (e *Exchange) GetHistoricalFundingRates(ctx context.Context, arg *fundingrate.HistoricalRatesRequest) (*fundingrate.HistoricalRates, error) {
	if arg == nil {
		return nil, common.ErrNilPointer
	}
	if arg.Asset != asset.PerpetualContract {
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, arg.Asset)
	}
	if arg.Pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if arg.IncludePredictedRate || arg.IncludePayments {
		return nil, fmt.Errorf("%w: predicted rates and account payments", common.ErrFunctionNotSupported)
	}
	if err := common.StartEndTimeCheck(arg.StartDate, arg.EndDate); err != nil {
		return nil, err
	}
	mapping, err := e.getPairMapping(ctx, arg.Pair, arg.Asset)
	if err != nil {
		return nil, err
	}
	if !arg.PaymentCurrency.IsEmpty() && !arg.PaymentCurrency.Equal(mapping.pair.Quote) {
		return nil, fmt.Errorf("%w: funding payment currency %s", asset.ErrNotSupported, arg.PaymentCurrency)
	}
	result := &fundingrate.HistoricalRates{
		Exchange:        e.Name,
		Asset:           arg.Asset,
		Pair:            mapping.pair,
		StartDate:       arg.StartDate,
		EndDate:         arg.EndDate,
		PaymentCurrency: mapping.pair.Quote,
	}
	cursor := arg.StartDate
	for !cursor.After(arg.EndDate) {
		records, err := e.GetFundingHistory(ctx, mapping.coin, cursor, arg.EndDate)
		if err != nil {
			return nil, err
		}
		if len(records) > maximumFundingHistoryCount {
			return nil, fmt.Errorf("%w: funding page has %d entries, maximum %d", errUnexpectedResponseLength, len(records), maximumFundingHistoryCount)
		}
		for i := range records {
			recordTime := records[i].Time.Time().UTC()
			if records[i].Coin != mapping.coin || recordTime.Before(cursor) || recordTime.After(arg.EndDate) ||
				(i > 0 && !recordTime.After(records[i-1].Time.Time())) {
				return nil, fmt.Errorf("%w: malformed funding history page", errUnexpectedResponseLength)
			}
			result.FundingRates = append(result.FundingRates, fundingrate.Rate{
				Time: recordTime,
				Rate: records[i].FundingRate.Decimal(),
			})
		}
		if len(records) < maximumFundingHistoryCount {
			break
		}
		cursor = records[len(records)-1].Time.Time().UTC().Add(time.Millisecond)
	}
	if len(result.FundingRates) == 0 {
		return nil, fundingrate.ErrNoFundingRatesFound
	}
	result.LatestRate = result.FundingRates[len(result.FundingRates)-1]
	return result, nil
}

// GetOpenInterest returns the current base unit open interest of selected or all active perpetual markets
func (e *Exchange) GetOpenInterest(ctx context.Context, requested ...key.PairAsset) ([]futures.OpenInterest, error) {
	var mappings []pairMapping
	if len(requested) == 0 {
		var err error
		if mappings, err = e.getPerpetualPairMappings(ctx); err != nil {
			return nil, err
		}
	} else {
		mappings = make([]pairMapping, len(requested))
		for i := range requested {
			if requested[i].Asset != asset.PerpetualContract {
				return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, requested[i].Asset)
			}
			var err error
			if mappings[i], err = e.getPairMapping(ctx, requested[i].Pair(), requested[i].Asset); err != nil {
				return nil, err
			}
		}
	}
	contexts, err := e.getPerpetualAssetContexts(ctx, mappings)
	if err != nil {
		return nil, err
	}
	result := make([]futures.OpenInterest, len(mappings))
	for i := range mappings {
		result[i] = futures.OpenInterest{
			Key:          key.NewExchangeAssetPair(e.Name, asset.PerpetualContract, mappings[i].pair),
			OpenInterest: contexts[i].OpenInterest.Float64(),
		}
	}
	return result, nil
}

// GetCurrencyTradeURL is not supported by this integration
func (e *Exchange) GetCurrencyTradeURL(context.Context, asset.Item, currency.Pair) (string, error) {
	return "", common.ErrFunctionNotSupported
}

// UpdateOrderExecutionLimits is not yet implemented by this integration
func (e *Exchange) UpdateOrderExecutionLimits(context.Context, asset.Item) error {
	return common.ErrNotYetImplemented
}

func (e *Exchange) setPairMappings(a asset.Item, mappings []pairMapping) {
	e.pairMappingsMu.Lock()
	if e.pairMappings == nil {
		e.pairMappings = make(map[asset.Item][]pairMapping)
	}
	e.pairMappings[a] = slices.Clone(mappings)
	e.pairMappingsMu.Unlock()
}

func (e *Exchange) getCoin(ctx context.Context, p currency.Pair, a asset.Item) (string, error) {
	mapping, err := e.getPairMapping(ctx, p, a)
	if err != nil {
		return "", err
	}
	return mapping.coin, nil
}

// getPairMapping returns a pair's mapping, fetching the asset's markets when it is not cached
func (e *Exchange) getPairMapping(ctx context.Context, p currency.Pair, a asset.Item) (pairMapping, error) {
	if !e.SupportsAsset(a) {
		return pairMapping{}, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	if p.IsEmpty() {
		return pairMapping{}, currency.ErrCurrencyPairEmpty
	}
	if mapping, ok := e.lookupPairMapping(p, a); ok {
		return mapping, nil
	}
	return e.fetchPairMapping(ctx, p, a)
}

func (e *Exchange) lookupPairMapping(p currency.Pair, a asset.Item) (pairMapping, bool) {
	e.pairMappingsMu.RLock()
	defer e.pairMappingsMu.RUnlock()
	for _, mapping := range e.pairMappings[a] {
		if mapping.pair.Equal(p) {
			return mapping, true
		}
	}
	return pairMapping{}, false
}

// fetchPairMapping refreshes an asset's mappings for a missing pair; a pair still missing is cached as a miss for a
// minute, so repeated lookups of an unknown pair do not refetch every market
func (e *Exchange) fetchPairMapping(ctx context.Context, p currency.Pair, a asset.Item) (pairMapping, error) {
	e.pairMappingsFetchMu.Lock()
	defer e.pairMappingsFetchMu.Unlock()
	if mapping, ok := e.lookupPairMapping(p, a); ok {
		return mapping, nil
	}
	cacheKey := "pair:" + a.String() + ":" + strings.ToLower(p.String())
	if e.isCachedPairMappingMiss(cacheKey) {
		return pairMapping{}, fmt.Errorf("%w: %s %s", errPairMappingNotFound, a, p)
	}
	if _, err := e.FetchTradablePairs(ctx, a); err != nil {
		return pairMapping{}, err
	}
	if mapping, ok := e.lookupPairMapping(p, a); ok {
		return mapping, nil
	}
	e.cachePairMappingMiss(cacheKey)
	return pairMapping{}, fmt.Errorf("%w: %s %s", errPairMappingNotFound, a, p)
}

// getPairMappingByCoin returns the mapping and asset type of a coin, fetching all markets when it is not cached
func (e *Exchange) getPairMappingByCoin(ctx context.Context, coin string) (pairMapping, asset.Item, error) {
	if strings.TrimSpace(coin) == "" {
		return pairMapping{}, asset.Empty, errCoinRequired
	}
	mapping, a, err := e.lookupPairMappingByCoin(coin)
	if err == nil || errors.Is(err, errAmbiguousCoinMapping) {
		return mapping, a, err
	}
	return e.fetchPairMappingByCoin(ctx, coin)
}

func (e *Exchange) lookupPairMappingByCoin(coin string) (pairMapping, asset.Item, error) {
	e.pairMappingsMu.RLock()
	defer e.pairMappingsMu.RUnlock()
	var found pairMapping
	var foundAsset asset.Item
	var count int
	for _, a := range []asset.Item{asset.Spot, asset.PerpetualContract} {
		for _, mapping := range e.pairMappings[a] {
			if mapping.coin == coin {
				found = mapping
				foundAsset = a
				count++
			}
		}
	}
	switch count {
	case 0:
		return pairMapping{}, asset.Empty, fmt.Errorf("%w: %s", errPairMappingNotFound, coin)
	case 1:
		return found, foundAsset, nil
	default:
		return pairMapping{}, asset.Empty, fmt.Errorf("%w: %s", errAmbiguousCoinMapping, coin)
	}
}

func (e *Exchange) fetchPairMappingByCoin(ctx context.Context, coin string) (pairMapping, asset.Item, error) {
	e.pairMappingsFetchMu.Lock()
	defer e.pairMappingsFetchMu.Unlock()
	mapping, a, err := e.lookupPairMappingByCoin(coin)
	if err == nil || errors.Is(err, errAmbiguousCoinMapping) {
		return mapping, a, err
	}
	cacheKey := "coin:" + strings.ToLower(coin)
	if e.isCachedPairMappingMiss(cacheKey) {
		return pairMapping{}, asset.Empty, fmt.Errorf("%w: %s", errPairMappingNotFound, coin)
	}
	for _, item := range []asset.Item{asset.Spot, asset.PerpetualContract} {
		if !e.SupportsAsset(item) {
			continue
		}
		if _, err := e.FetchTradablePairs(ctx, item); err != nil {
			return pairMapping{}, asset.Empty, err
		}
	}
	mapping, a, err = e.lookupPairMappingByCoin(coin)
	if errors.Is(err, errPairMappingNotFound) {
		e.cachePairMappingMiss(cacheKey)
	}
	return mapping, a, err
}

// isCachedPairMappingMiss reports whether a lookup missed within the last minute; callers hold pairMappingsFetchMu
func (e *Exchange) isCachedPairMappingMiss(cacheKey string) bool {
	expiry, ok := e.pairMappingMisses[cacheKey]
	if !ok {
		return false
	}
	if time.Now().Before(expiry) {
		return true
	}
	delete(e.pairMappingMisses, cacheKey)
	return false
}

// cachePairMappingMiss records a lookup miss; callers hold pairMappingsFetchMu
func (e *Exchange) cachePairMappingMiss(cacheKey string) {
	if e.pairMappingMisses == nil {
		e.pairMappingMisses = make(map[string]time.Time)
	}
	e.pairMappingMisses[cacheKey] = time.Now().Add(pairMappingMissCacheDuration)
}

// getPerpetualPairMappings returns every cached perpetual mapping, fetching the markets when none are cached
func (e *Exchange) getPerpetualPairMappings(ctx context.Context) ([]pairMapping, error) {
	e.pairMappingsMu.RLock()
	mappings := slices.Clone(e.pairMappings[asset.PerpetualContract])
	e.pairMappingsMu.RUnlock()
	if len(mappings) != 0 {
		return mappings, nil
	}
	if _, err := e.FetchTradablePairs(ctx, asset.PerpetualContract); err != nil {
		return nil, err
	}
	e.pairMappingsMu.RLock()
	defer e.pairMappingsMu.RUnlock()
	return slices.Clone(e.pairMappings[asset.PerpetualContract]), nil
}

// getPerpetualAssetContexts returns the current context of each mapped market, fetching each perpetual DEX once
func (e *Exchange) getPerpetualAssetContexts(ctx context.Context, mappings []pairMapping) ([]PerpetualAssetContext, error) {
	dexContexts := make(map[string]*PerpetualMetadataAndAssetContextsResponse)
	contexts := make([]PerpetualAssetContext, len(mappings))
	for i := range mappings {
		resp, ok := dexContexts[mappings[i].dex]
		if !ok {
			var err error
			if resp, err = e.GetPerpetualMetadataAndAssetContexts(ctx, mappings[i].dex); err != nil {
				return nil, err
			}
			dexContexts[mappings[i].dex] = resp
		}
		index := slices.IndexFunc(resp.Metadata.Universe, func(market PerpetualAssetMetadata) bool {
			return market.Name == mappings[i].coin && !market.IsDelisted
		})
		if index == -1 {
			return nil, fmt.Errorf("%w: %s", errAssetContextNotFound, mappings[i].coin)
		}
		contexts[i] = resp.AssetContexts[index]
	}
	return contexts, nil
}

// getPerpetualDEXNames returns the name of each perpetual DEX by index, with an empty name for the default DEX
func (e *Exchange) getPerpetualDEXNames(ctx context.Context) ([]string, error) {
	dexes, err := e.GetPerpetualDEXs(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(dexes))
	seen := make(map[string]struct{}, len(dexes)-1)
	for i := 1; i < len(dexes); i++ {
		if dexes[i] == nil || strings.TrimSpace(dexes[i].Name) == "" {
			return nil, fmt.Errorf("%w: entry %d has no name", errInvalidPerpetualDEX, i)
		}
		names[i] = strings.TrimSpace(dexes[i].Name)
		if _, exists := seen[names[i]]; exists {
			return nil, fmt.Errorf("%w: duplicate DEX %q", errInvalidPerpetualDEX, names[i])
		}
		seen[names[i]] = struct{}{}
	}
	return names, nil
}

// openOrderDEXes returns the perpetual DEXs whose open orders include an asset type's orders: the first DEX, which also
// lists spot orders, for spot, and every DEX for perpetuals
func (e *Exchange) openOrderDEXes(ctx context.Context, a asset.Item) ([]string, error) {
	switch a {
	case asset.Spot:
		return []string{""}, nil
	case asset.PerpetualContract:
		return e.getPerpetualDEXNames(ctx)
	default:
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
}

// getUserNonFundingLedgerUpdatesPaginated pages through every ledger update from start until now, using each full
// page's last timestamp as the next start; records at that boundary repeat on the next page, so identical records are
// dropped
func (e *Exchange) getUserNonFundingLedgerUpdatesPaginated(ctx context.Context, user string, start time.Time) ([]UserLedgerUpdate, error) {
	cursor := start
	var result []UserLedgerUpdate
	seen := make(map[string]struct{})
	for {
		page, err := e.GetUserNonFundingLedgerUpdates(ctx, user, cursor, time.Time{})
		if err != nil {
			return nil, err
		}
		if len(page) > maximumUserLedgerHistoryCount {
			return nil, fmt.Errorf("%w: ledger page has %d entries, maximum %d", errUnexpectedResponseLength, len(page), maximumUserLedgerHistoryCount)
		}
		for i := range page {
			recordTime := page[i].Time.Time()
			if recordTime.Before(cursor) || (i > 0 && recordTime.Before(page[i-1].Time.Time())) {
				return nil, fmt.Errorf("%w: malformed user ledger page", errUnexpectedResponseLength)
			}
			encoded, _ := json.Marshal(&page[i]) // Ledger updates hold only strings, numbers, booleans and timestamps
			if _, exists := seen[string(encoded)]; !exists {
				seen[string(encoded)] = struct{}{}
				result = append(result, page[i])
			}
		}
		if len(page) < maximumUserLedgerHistoryCount {
			return result, nil
		}
		last := page[len(page)-1].Time.Time()
		if !last.After(cursor) {
			return nil, fmt.Errorf("%w: user ledger cursor did not advance", errUnexpectedResponseLength)
		}
		cursor = last
	}
}

// convertUserLedgerUpdate converts a ledger update into funding history, reading the amount and currency from the
// fields its delta type populates
func (e *Exchange) convertUserLedgerUpdate(update *UserLedgerUpdate) (exchange.FundingHistory, error) {
	deltaType := strings.TrimSpace(update.Delta.Type)
	if deltaType == "" {
		return exchange.FundingHistory{}, fmt.Errorf("%w: ledger update type is empty", errUnexpectedResponseLength)
	}
	ccy := currency.USDC
	amount := update.Delta.USDC.Float64()
	switch deltaType {
	case "spotTransfer", "spotGenesis", "send":
		if !update.Delta.Token.IsEmpty() {
			ccy = update.Delta.Token
		}
		amount = update.Delta.Amount.Float64()
	case "rewardsClaim":
		amount = update.Delta.Amount.Float64()
	case "vaultWithdraw":
		amount = update.Delta.NetWithdrawnUSD.Float64()
	}
	description := deltaType
	switch {
	case deltaType == "accountClassTransfer" && update.Delta.ToPerp:
		description = "spot to perpetual"
	case deltaType == "accountClassTransfer":
		description = "perpetual to spot"
	case update.Delta.SourceDEX != "" || update.Delta.DestinationDEX != "":
		description = deltaType + ": " + update.Delta.SourceDEX + " to " + update.Delta.DestinationDEX
	}
	return exchange.FundingHistory{
		ExchangeName:      e.Name,
		Status:            "processed",
		TransferID:        update.Hash,
		Description:       description,
		Timestamp:         update.Time.Time().UTC(),
		Currency:          ccy.String(),
		Amount:            amount,
		Fee:               update.Delta.Fee.Float64(),
		TransferType:      deltaType,
		CryptoToAddress:   update.Delta.Destination,
		CryptoFromAddress: update.Delta.User,
		CryptoTxID:        update.Hash,
	}, nil
}

// buildOrderRequest validates an order against its market's precision and builds the order to sign
// Market orders become IOC limit orders bounded by the slippage tolerance from the mid price
func (e *Exchange) buildOrderRequest(ctx context.Context, p currency.Pair, a asset.Item, orderType order.Type, side order.Side, timeInForce order.TimeInForce, amount, price, triggerPrice, slippage float64, reduceOnly bool, clientOrderID string) (OrderRequest, pairMapping, error) {
	mapping, err := e.getPairMapping(ctx, p, a)
	if err != nil {
		return OrderRequest{}, pairMapping{}, err
	}
	if _, err := formatOrderSize(amount, mapping.sizeDecimals); err != nil {
		return OrderRequest{}, mapping, err
	}
	if clientOrderID != "" {
		if err := validateClientOrderID(clientOrderID); err != nil {
			return OrderRequest{}, mapping, err
		}
	}
	orderRequest := OrderRequest{
		Asset:         mapping.assetID,
		IsBuy:         side.IsLong(),
		Size:          amount,
		ReduceOnly:    reduceOnly,
		ClientOrderID: strings.ToLower(clientOrderID),
	}
	switch orderType {
	case order.Limit:
		if triggerPrice != 0 {
			return OrderRequest{}, mapping, fmt.Errorf("%w: trigger price requires a trigger order type", errRiskManagementUnsupported)
		}
		if err := validateLimitPrice(price, a, mapping.sizeDecimals); err != nil {
			return OrderRequest{}, mapping, err
		}
		tif, err := formatOrderTimeInForce(timeInForce)
		if err != nil {
			return OrderRequest{}, mapping, err
		}
		orderRequest.Price = price
		orderRequest.Limit = &LimitOrderType{TimeInForce: tif}
	case order.Market:
		if triggerPrice != 0 {
			return OrderRequest{}, mapping, fmt.Errorf("%w: trigger price requires a trigger order type", errRiskManagementUnsupported)
		}
		if slippage <= 0 || slippage >= 1 {
			return OrderRequest{}, mapping, errSlippageTolerance
		}
		mids, err := e.GetAllMids(ctx, mapping.dex)
		if err != nil {
			return OrderRequest{}, mapping, err
		}
		mid, ok := mids[mapping.coin]
		if !ok || mid.Float64() <= 0 {
			return OrderRequest{}, mapping, fmt.Errorf("%w: %s", errMarketMidPriceNotFound, mapping.coin)
		}
		if orderRequest.Price, err = roundMarketPrice(slippagePrice(mid.Float64(), slippage, side), a, mapping.sizeDecimals); err != nil {
			return OrderRequest{}, mapping, err
		}
		orderRequest.Limit = &LimitOrderType{TimeInForce: TimeInForceIOC}
	case order.Stop, order.StopLimit, order.StopMarket, order.TakeProfit, order.TakeProfitMarket:
		if a != asset.PerpetualContract || !reduceOnly {
			return OrderRequest{}, mapping, errTriggerOrderReduceOnly
		}
		if triggerPrice <= 0 {
			return OrderRequest{}, mapping, errTriggerPriceRequired
		}
		if err := validateLimitPrice(triggerPrice, a, mapping.sizeDecimals); err != nil {
			return OrderRequest{}, mapping, fmt.Errorf("invalid trigger price: %w", err)
		}
		isMarket := orderType == order.Stop || orderType == order.StopMarket || orderType == order.TakeProfitMarket
		if isMarket && price == 0 {
			if slippage <= 0 || slippage >= 1 {
				return OrderRequest{}, mapping, errSlippageTolerance
			}
			if price, err = roundMarketPrice(slippagePrice(triggerPrice, slippage, side), a, mapping.sizeDecimals); err != nil {
				return OrderRequest{}, mapping, err
			}
		} else if err := validateLimitPrice(price, a, mapping.sizeDecimals); err != nil {
			return OrderRequest{}, mapping, err
		}
		kind := TriggerStopLoss
		if orderType == order.TakeProfit || orderType == order.TakeProfitMarket {
			kind = TriggerTakeProfit
		}
		orderRequest.Price = price
		orderRequest.Trigger = &TriggerOrderType{IsMarket: isMarket, TriggerPrice: triggerPrice, TakeProfitStopLoss: kind}
	default:
		return OrderRequest{}, mapping, fmt.Errorf("%w: %s", order.ErrTypeIsInvalid, orderType)
	}
	return orderRequest, mapping, nil
}

// buildOrderRequests builds a submission's order and any TP/SL children, which Hyperliquid triggers on the mark price
func (e *Exchange) buildOrderRequests(ctx context.Context, submit *order.Submit) ([]OrderRequest, pairMapping, string, error) {
	switch submit.Type {
	case order.Stop, order.StopLimit, order.StopMarket, order.TakeProfit, order.TakeProfitMarket:
		if submit.TriggerPriceType != order.MarkPrice {
			return nil, pairMapping{}, "", fmt.Errorf("%w: Hyperliquid triggers use mark price", errRiskManagementUnsupported)
		}
	}
	parent, mapping, err := e.buildOrderRequest(ctx, submit.Pair, submit.AssetType, submit.Type, submit.Side, submit.TimeInForce, submit.Amount, submit.Price, submit.TriggerPrice, submit.SlippageTolerance, submit.ReduceOnly, submit.ClientOrderID)
	if err != nil {
		return nil, mapping, "", err
	}
	risk := submit.RiskManagementModes
	if !risk.TakeProfit.Enabled && !risk.StopLoss.Enabled && !risk.StopEntry.Enabled {
		return []OrderRequest{parent}, mapping, GroupingNone, nil
	}
	if submit.AssetType != asset.PerpetualContract || risk.StopEntry.Enabled || (risk.Mode != "" && risk.Mode != GroupingNormalTPSL) {
		return nil, mapping, "", errRiskManagementUnsupported
	}
	orders := []OrderRequest{parent}
	childSide := order.Buy
	if submit.Side.IsLong() {
		childSide = order.Sell
	}
	for _, child := range []struct {
		riskManagement order.RiskManagement
		takeProfit     bool
	}{
		{riskManagement: risk.TakeProfit, takeProfit: true},
		{riskManagement: risk.StopLoss},
	} {
		if !child.riskManagement.Enabled {
			continue
		}
		if child.riskManagement.Price <= 0 {
			return nil, mapping, "", errTriggerPriceRequired
		}
		if child.riskManagement.TriggerPriceType != order.MarkPrice {
			return nil, mapping, "", fmt.Errorf("%w: Hyperliquid TP/SL triggers use mark price", errRiskManagementUnsupported)
		}
		childType, err := riskManagementOrderType(child.riskManagement.OrderType, child.takeProfit)
		if err != nil {
			return nil, mapping, "", err
		}
		slippage := submit.SlippageTolerance
		if (childType == order.StopMarket || childType == order.TakeProfitMarket) && child.riskManagement.LimitPrice == 0 && slippage == 0 {
			slippage = defaultTriggerSlippage
		}
		childOrder, _, err := e.buildOrderRequest(ctx, submit.Pair, submit.AssetType, childType, childSide, order.UnknownTIF, submit.Amount, child.riskManagement.LimitPrice, child.riskManagement.Price, slippage, true, "")
		if err != nil {
			return nil, mapping, "", err
		}
		orders = append(orders, childOrder)
	}
	return orders, mapping, GroupingNormalTPSL, nil
}

// riskManagementOrderType maps a TP/SL child's requested order type to a take-profit or stop-loss trigger type
func riskManagementOrderType(orderType order.Type, takeProfit bool) (order.Type, error) {
	switch {
	case takeProfit && (orderType == order.UnknownType || orderType == order.Market || orderType == order.TakeProfitMarket):
		return order.TakeProfitMarket, nil
	case takeProfit && (orderType == order.Limit || orderType == order.TakeProfit):
		return order.TakeProfit, nil
	case !takeProfit && (orderType == order.UnknownType || orderType == order.Market || orderType == order.Stop || orderType == order.StopMarket):
		return order.StopMarket, nil
	case !takeProfit && (orderType == order.Limit || orderType == order.StopLimit):
		return order.StopLimit, nil
	case takeProfit:
		return order.UnknownType, fmt.Errorf("%w: take-profit child type %s", errRiskManagementUnsupported, orderType)
	default:
		return order.UnknownType, fmt.Errorf("%w: stop-loss child type %s", errRiskManagementUnsupported, orderType)
	}
}

// cancelOrders cancels orders by order ID and by client order ID, and maps each identifier to its outcome
func (e *Exchange) cancelOrders(ctx context.Context, cancels []order.Cancel) (map[string]string, error) {
	if len(cancels) == 0 {
		return map[string]string{}, nil
	}
	if len(cancels) > maximumActionBatchSize {
		return nil, fmt.Errorf("%w: maximum %d, got %d", errActionBatchTooLarge, maximumActionBatchSize, len(cancels))
	}
	var byOrderID []CancelRequest
	var orderIDs []string
	var byClientOrderID []CancelByClientOrderIDRequest
	var clientOrderIDs []string
	for i := range cancels {
		if err := cancels[i].Validate(cancels[i].PairAssetRequired()); err != nil {
			return nil, err
		}
		mapping, err := e.getPairMapping(ctx, cancels[i].Pair, cancels[i].AssetType)
		if err != nil {
			return nil, err
		}
		switch {
		case cancels[i].OrderID != "":
			id, err := strconv.ParseUint(cancels[i].OrderID, 10, 64)
			if err != nil || id == 0 {
				return nil, fmt.Errorf("%w: %q", order.ErrOrderIDNotSet, cancels[i].OrderID)
			}
			byOrderID = append(byOrderID, CancelRequest{Asset: mapping.assetID, OrderID: id})
			orderIDs = append(orderIDs, cancels[i].OrderID)
		case cancels[i].ClientOrderID != "":
			if err := validateClientOrderID(cancels[i].ClientOrderID); err != nil {
				return nil, err
			}
			byClientOrderID = append(byClientOrderID, CancelByClientOrderIDRequest{Asset: mapping.assetID, ClientOrderID: cancels[i].ClientOrderID})
			clientOrderIDs = append(clientOrderIDs, cancels[i].ClientOrderID)
		default:
			return nil, order.ErrOrderIDNotSet
		}
	}
	statuses := make(map[string]string, len(cancels))
	var errs error
	if len(byOrderID) != 0 {
		results, err := e.CancelOrders(ctx, &CancelOrdersRequest{Cancels: byOrderID})
		errs = common.AppendError(errs, common.AppendError(err, recordCancelStatuses(statuses, orderIDs, results)))
	}
	if len(byClientOrderID) != 0 {
		results, err := e.CancelOrdersByClientOrderID(ctx, &CancelOrdersByClientOrderIDRequest{Cancels: byClientOrderID})
		errs = common.AppendError(errs, common.AppendError(err, recordCancelStatuses(statuses, clientOrderIDs, results)))
	}
	return statuses, errs
}

// recordCancelStatuses maps each identifier to success or its error, and returns the failed cancels as errors
func recordCancelStatuses(statuses map[string]string, identifiers []string, results []CancelActionStatus) error {
	var errs error
	for i := range results {
		if results[i].Error == "" {
			statuses[identifiers[i]] = "success"
			continue
		}
		statuses[identifiers[i]] = results[i].Error
		errs = common.AppendError(errs, fmt.Errorf("%s: %w: %s", identifiers[i], errActionResponse, results[i].Error))
	}
	return errs
}

// convertOrder converts an order from its coin, fetching market mappings when the coin is not cached
func (e *Exchange) convertOrder(ctx context.Context, source *FrontendOpenOrder, status string, statusTimestamp time.Time) (order.Detail, error) {
	mapping, a, err := e.getPairMappingByCoin(ctx, source.Coin)
	if err != nil {
		return order.Detail{}, err
	}
	return e.convertOrderFromMapping(source, status, statusTimestamp, &mapping, a)
}

func (e *Exchange) convertOrderFromMapping(source *FrontendOpenOrder, status string, statusTimestamp time.Time, mapping *pairMapping, a asset.Item) (order.Detail, error) {
	orderType, err := parseOrderType(source.OrderType, source.IsTrigger)
	if err != nil {
		return order.Detail{}, err
	}
	timeInForce, err := parseTimeInForce(source.TimeInForce)
	if err != nil {
		return order.Detail{}, err
	}
	detail, err := e.convertBasicOrder(&source.BasicOrder, status, statusTimestamp, mapping.pair, a)
	if err != nil {
		return order.Detail{}, err
	}
	detail.Type = orderType
	detail.TimeInForce = timeInForce
	detail.ReduceOnly = source.ReduceOnly
	detail.TriggerPrice = source.TriggerPrice.Float64()
	return detail, nil
}

// convertWsOrder converts an order update, whose feed omits the order type and time in force
func (e *Exchange) convertWsOrder(update *WsOrder) (order.Detail, error) {
	mapping, a, err := e.lookupPairMappingByCoin(update.Order.Coin)
	if err != nil {
		return order.Detail{}, err
	}
	return e.convertBasicOrder(&update.Order, update.Status, update.StatusTimestamp.Time(), mapping.pair, a)
}

// convertBasicOrder converts the order fields every order source provides; its size is the order's remaining size
func (e *Exchange) convertBasicOrder(source *BasicOrder, status string, statusTimestamp time.Time, p currency.Pair, a asset.Item) (order.Detail, error) {
	side, err := parseSide(source.Side)
	if err != nil {
		return order.Detail{}, err
	}
	orderStatus, err := parseOrderStatus(status)
	if err != nil {
		return order.Detail{}, err
	}
	amount := cmp.Or(source.OriginalSize.Float64(), source.Size.Float64())
	placed := source.Timestamp.Time().UTC()
	lastUpdated := statusTimestamp.UTC()
	if lastUpdated.IsZero() {
		lastUpdated = placed
	}
	return order.Detail{
		Price:           source.LimitPrice.Float64(),
		Amount:          amount,
		ExecutedAmount:  max(0, amount-source.Size.Float64()),
		RemainingAmount: source.Size.Float64(),
		Exchange:        e.Name,
		OrderID:         strconv.FormatUint(source.OrderID, 10),
		ClientOrderID:   source.ClientOrderID,
		Side:            side,
		Status:          orderStatus,
		AssetType:       a,
		Date:            placed,
		LastUpdated:     lastUpdated,
		Pair:            p,
	}, nil
}

// convertL2Book converts an L2 book snapshot into an orderbook
func (e *Exchange) convertL2Book(book *L2Book, p currency.Pair, a asset.Item) (*orderbook.Book, error) {
	if len(book.Levels) != 2 {
		return nil, fmt.Errorf("%w: expected 2 sides, got %d", errInvalidBookLevelCount, len(book.Levels))
	}
	result := &orderbook.Book{
		Exchange:          e.Name,
		Pair:              p,
		Asset:             a,
		LastUpdated:       book.Time.Time().UTC(),
		ValidateOrderbook: e.ValidateOrderbook,
		Bids:              make(orderbook.Levels, len(book.Levels[0])),
		Asks:              make(orderbook.Levels, len(book.Levels[1])),
	}
	for i := range book.Levels[0] {
		result.Bids[i] = convertL2Level(&book.Levels[0][i])
	}
	for i := range book.Levels[1] {
		result.Asks[i] = convertL2Level(&book.Levels[1][i])
	}
	return result, nil
}

func convertL2Level(level *L2Level) orderbook.Level {
	return orderbook.Level{
		Price:      level.Price.Float64(),
		Amount:     level.Size.Float64(),
		OrderCount: int64(level.OrderCount), //nolint:gosec // A level's order count is far below the int64 maximum
	}
}

// convertTrade converts a public trade; its side is the aggressor's
func (e *Exchange) convertTrade(t *RecentTrade, p currency.Pair, a asset.Item) (trade.Data, error) {
	side, err := parseSide(t.Side)
	if err != nil {
		return trade.Data{}, err
	}
	return trade.Data{
		TID:          strconv.FormatUint(t.TradeID, 10),
		Exchange:     e.Name,
		CurrencyPair: p,
		AssetType:    a,
		Side:         side,
		Price:        t.Price.Float64(),
		Amount:       t.Size.Float64(),
		Timestamp:    t.Time.Time().UTC(),
	}, nil
}

func convertCandle(c *Candle) kline.Candle {
	return kline.Candle{
		Time:   c.OpenTime.Time().UTC(),
		Open:   c.Open.Float64(),
		High:   c.High.Float64(),
		Low:    c.Low.Float64(),
		Close:  c.Close.Float64(),
		Volume: c.Volume.Float64(),
	}
}

// perpetualTickerPrice builds a ticker from a perpetual market context, whose oracle price is the index price
func (e *Exchange) perpetualTickerPrice(p currency.Pair, market *PerpetualAssetContext) *ticker.Price {
	return &ticker.Price{
		Last:         cmp.Or(market.MidPrice.Float64(), market.MarkPrice.Float64()),
		Open:         market.PreviousDayPrice.Float64(),
		BaseVolume:   market.DayBaseVolume.Float64(),
		QuoteVolume:  market.DayNotionalVolume.Float64(),
		OpenInterest: market.OpenInterest.Float64(),
		MarkPrice:    market.MarkPrice.Float64(),
		IndexPrice:   market.OraclePrice.Float64(),
		Pair:         p,
		ExchangeName: e.Name,
		AssetType:    asset.PerpetualContract,
		LastUpdated:  time.Now().UTC(),
	}
}

// spotTickerPrice builds a ticker from a spot market context
func (e *Exchange) spotTickerPrice(p currency.Pair, market *SpotAssetContext) *ticker.Price {
	return &ticker.Price{
		Last:         cmp.Or(market.MidPrice.Float64(), market.MarkPrice.Float64()),
		Open:         market.PreviousDayPrice.Float64(),
		BaseVolume:   market.DayBaseVolume.Float64(),
		QuoteVolume:  market.DayNotionalVolume.Float64(),
		MarkPrice:    market.MarkPrice.Float64(),
		Pair:         p,
		ExchangeName: e.Name,
		AssetType:    asset.Spot,
		LastUpdated:  time.Now().UTC(),
	}
}

// dexSubAccountID scopes a balance subaccount to a perpetual DEX, leaving the default DEX unscoped
func dexSubAccountID(address, dex string) string {
	if dex == "" {
		return address
	}
	return address + ":" + dex
}

// parseSide converts a side, which is A for ask and B for bid
func parseSide(side string) (order.Side, error) {
	switch side {
	case "A":
		return order.Sell, nil
	case "B":
		return order.Buy, nil
	default:
		return order.UnknownSide, fmt.Errorf("%w: %q", order.ErrSideIsInvalid, side)
	}
}

func parseOrderStatus(status string) (order.Status, error) {
	switch status {
	case "open":
		return order.Open, nil
	case "filled":
		return order.Filled, nil
	case "triggered":
		return order.Closed, nil
	case "canceled", "scheduledCancel", "marginCanceled", "vaultWithdrawalCanceled", "openInterestCapCanceled", "selfTradeCanceled", "reduceOnlyCanceled", "siblingFilledCanceled", "delistedCanceled", "liquidatedCanceled":
		return order.Cancelled, nil
	case "rejected", "tickRejected", "minTradeNtlRejected", "perpMarginRejected", "reduceOnlyRejected", "badAloPxRejected", "iocCancelRejected", "badTriggerPxRejected", "marketOrderNoLiquidityRejected", "positionIncreaseAtOpenInterestCapRejected", "positionFlipAtOpenInterestCapRejected", "tooAggressiveAtOpenInterestCapRejected", "openInterestIncreaseRejected", "insufficientSpotBalanceRejected", "oracleRejected", "perpMaxPositionRejected":
		return order.Rejected, nil
	default:
		return order.UnknownStatus, fmt.Errorf("%w: %s", errUnsupportedOrderStatus, status)
	}
}

// parseOrderType converts the order types the frontend displays, such as Limit, Stop Market or Take Profit Limit
func parseOrderType(orderType string, isTrigger bool) (order.Type, error) {
	lowerOrderType := strings.ToLower(orderType)
	switch {
	case isTrigger && strings.Contains(lowerOrderType, "take profit") && strings.Contains(lowerOrderType, "market"):
		return order.TakeProfitMarket, nil
	case isTrigger && strings.Contains(lowerOrderType, "take profit"):
		return order.TakeProfit, nil
	case isTrigger && strings.Contains(lowerOrderType, "market"):
		return order.StopMarket, nil
	case isTrigger && strings.Contains(lowerOrderType, "limit"):
		return order.StopLimit, nil
	case isTrigger:
		return order.Stop, nil
	case strings.Contains(lowerOrderType, "market"):
		return order.Market, nil
	case strings.Contains(lowerOrderType, "limit"):
		return order.Limit, nil
	default:
		return order.UnknownType, fmt.Errorf("%w: %s", order.ErrTypeIsInvalid, orderType)
	}
}

// parseTimeInForce converts a time in force; FrontendMarket marks the IOC orders the frontend places as market orders
func parseTimeInForce(timeInForce string) (order.TimeInForce, error) {
	switch strings.ToLower(timeInForce) {
	case "", "gtc":
		return order.GoodTillCancel, nil
	case "alo":
		return order.PostOnly, nil
	case "ioc", "frontendmarket":
		return order.ImmediateOrCancel, nil
	default:
		return order.UnknownTIF, fmt.Errorf("%w: %s", order.ErrInvalidTimeInForce, timeInForce)
	}
}

func formatOrderTimeInForce(timeInForce order.TimeInForce) (string, error) {
	switch timeInForce {
	case order.UnknownTIF, order.GoodTillCancel:
		return TimeInForceGTC, nil
	case order.PostOnly, order.GoodTillCancel | order.PostOnly:
		return TimeInForceALO, nil
	case order.ImmediateOrCancel:
		return TimeInForceIOC, nil
	default:
		return "", fmt.Errorf("%w: %s", order.ErrUnsupportedTimeInForce, timeInForce)
	}
}

// formatOrderSize checks a size is positive and fits the market's size decimals
func formatOrderSize(size float64, sizeDecimals uint64) (string, error) {
	if size <= 0 || sizeDecimals > 8 {
		return "", fmt.Errorf("%w: size %v with %d decimals", errSizePrecision, size, sizeDecimals)
	}
	scale := math.Pow10(int(sizeDecimals))
	if math.Abs(math.RoundToEven(size*scale)/scale-size) >= 1e-12 {
		return "", fmt.Errorf("%w: %v allows %d decimals", errSizePrecision, size, sizeDecimals)
	}
	return floatToWire(size)
}

// deriveFilledOrderState derives the status and remaining size of an order that filled on placement; only a GTC or IOC
// order can fill partially
func deriveFilledOrderState(requested, filled float64, sizeDecimals uint64, timeInForce string) (order.Status, float64, error) {
	if _, err := formatOrderSize(requested, sizeDecimals); err != nil {
		return order.UnknownStatus, 0, fmt.Errorf("%w: invalid requested size: %w", errInvalidFilledSize, err)
	}
	if _, err := formatOrderSize(filled, sizeDecimals); err != nil {
		return order.UnknownStatus, 0, fmt.Errorf("%w: invalid reported size: %w", errInvalidFilledSize, err)
	}
	scale := math.Pow10(int(sizeDecimals)) //nolint:gosec // formatOrderSize limits sizeDecimals to 8
	requestedUnits := math.Round(requested * scale)
	filledUnits := math.Round(filled * scale)
	switch {
	case filledUnits > requestedUnits:
		return order.UnknownStatus, 0, fmt.Errorf("%w: reported %v exceeds requested %v", errInvalidFilledSize, filled, requested)
	case filledUnits == requestedUnits:
		return order.Filled, 0, nil
	}
	remaining := (requestedUnits - filledUnits) / scale
	switch timeInForce {
	case TimeInForceGTC:
		return order.PartiallyFilled, remaining, nil
	case TimeInForceIOC:
		return order.PartiallyFilledCancelled, remaining, nil
	default:
		return order.UnknownStatus, 0, fmt.Errorf("%w: partial fill for %q time in force", errActionStatusMalformed, timeInForce)
	}
}

// priceDecimalBase returns the maximum price decimals of an asset type before subtracting the size decimals
func priceDecimalBase(a asset.Item, sizeDecimals uint64) (uint64, error) {
	var decimalBase uint64
	switch a {
	case asset.Spot:
		decimalBase = spotPriceDecimalBase
	case asset.PerpetualContract:
		decimalBase = perpetualPriceDecimalBase
	default:
		return 0, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	if sizeDecimals > decimalBase {
		return 0, fmt.Errorf("%w: %s prices allow at most %d size decimals, got %d", errSizePrecision, a, decimalBase, sizeDecimals)
	}
	return decimalBase, nil
}

// validateLimitPrice checks a price has at most five significant figures and fits the market's price decimals
func validateLimitPrice(price float64, a asset.Item, sizeDecimals uint64) error {
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return errInvalidMarketPrice
	}
	decimalBase, err := priceDecimalBase(a, sizeDecimals)
	if err != nil {
		return err
	}
	wire, err := floatToWire(price)
	if err != nil {
		return err
	}
	whole, fractional, hasFraction := strings.Cut(wire, ".")
	if !hasFraction {
		return nil // Integer prices are valid regardless of their significant figures
	}
	if uint64(len(fractional)) > decimalBase-sizeDecimals {
		return fmt.Errorf("%w: %s allows at most %d price decimals", errPricePrecision, a, decimalBase-sizeDecimals)
	}
	if significant := strings.TrimLeft(whole+fractional, "0"); len(significant) > marketPriceSignificantFigures {
		return fmt.Errorf("%w: got %d significant figures", errPricePrecision, len(significant))
	}
	return nil
}

// roundMarketPrice rounds a price to five significant figures and the market's price decimals
func roundMarketPrice(price float64, a asset.Item, sizeDecimals uint64) (float64, error) {
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, errInvalidMarketPrice
	}
	decimalBase, err := priceDecimalBase(a, sizeDecimals)
	if err != nil {
		return 0, err
	}
	significantPrice, _ := strconv.ParseFloat(strconv.FormatFloat(price, 'g', marketPriceSignificantFigures, 64), 64) // FormatFloat of a finite value always parses
	scale := math.Pow10(int(decimalBase - sizeDecimals))                                                              //nolint:gosec // priceDecimalBase bounds the difference to 8
	rounded := math.RoundToEven(significantPrice*scale) / scale
	if rounded <= 0 {
		return 0, errInvalidMarketPrice
	}
	return rounded, nil
}

// slippagePrice moves a reference price against the order by the slippage tolerance
func slippagePrice(reference, slippage float64, side order.Side) float64 {
	if side.IsLong() {
		return reference * (1 + slippage)
	}
	return reference * (1 - slippage)
}
