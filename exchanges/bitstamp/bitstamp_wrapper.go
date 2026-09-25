package bitstamp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
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
)

const (
	perpetualMarketSuffix = "-PERP"
	tradingEnabled        = "Enabled"

	// Order history, user transactions and settlement transactions are only retained for 30 days
	maxHistoryAge = 30 * 24 * time.Hour
	// historyAgeBuffer keeps requested start times inside the retention window despite clock differences
	historyAgeBuffer = time.Minute
)

var errCancelAllOrdersFailed = errors.New("not all orders were cancelled; check order status to verify")

// SetDefaults sets default for Bitstamp
func (e *Exchange) SetDefaults() {
	e.Name = "Bitstamp"
	e.Enabled = true
	e.Verbose = true
	e.API.CredentialsValidator.RequiresKey = true
	e.API.CredentialsValidator.RequiresSecret = true
	requestFmt := &currency.PairFormat{}
	configFmt := &currency.PairFormat{
		Uppercase: true,
		Delimiter: currency.ForwardSlashDelimiter,
	}
	err := e.SetGlobalPairsManager(requestFmt, configFmt, asset.Spot, asset.PerpetualContract)
	if err != nil {
		log.Errorln(log.ExchangeSys, err)
	}

	e.Features = exchange.Features{
		Supports: exchange.FeaturesSupported{
			REST:      true,
			Websocket: true,
			RESTCapabilities: protocol.Features{
				TickerBatching:        true,
				TickerFetching:        true,
				KlineFetching:         true,
				TradeFetching:         true,
				OrderbookFetching:     true,
				AutoPairUpdates:       true,
				AccountBalance:        true,
				GetOrder:              true,
				GetOrders:             true,
				CancelOrders:          true,
				CancelOrder:           true,
				SubmitOrder:           true,
				ModifyOrder:           true,
				DepositHistory:        true,
				WithdrawalHistory:     true,
				UserTradeHistory:      true,
				CryptoDeposit:         true,
				CryptoWithdrawal:      true,
				FiatDeposit:           true,
				FiatWithdraw:          true,
				TradeFee:              true,
				FiatDepositFee:        true,
				FiatWithdrawalFee:     true,
				CryptoDepositFee:      true,
				CryptoWithdrawalFee:   true,
				FundingRateFetching:   true,
				MultiChainDeposits:    true,
				MultiChainWithdrawals: true,
			},
			WebsocketCapabilities: protocol.Features{
				TradeFetching:          true,
				OrderbookFetching:      true,
				FundingRateFetching:    true,
				Subscribe:              true,
				Unsubscribe:            true,
				AuthenticatedEndpoints: true,
				GetOrders:              true,
			},
			WithdrawPermissions: exchange.AutoWithdrawCrypto |
				exchange.AutoWithdrawFiat,
			Kline: kline.ExchangeCapabilitiesSupported{
				Intervals:  true,
				DateRanges: true,
			},
			MaximumOrderHistory: maxHistoryAge,
			FuturesCapabilities: exchange.FuturesCapabilities{
				FundingRates:              true,
				MaximumFundingRateHistory: maxHistoryAge,
				SupportedFundingRateFrequencies: map[kline.Interval]bool{
					kline.EightHour: true,
				},
				Positions: true,
				Leverage:  true,
				OpenInterest: exchange.OpenInterestSupport{
					Supported:          true,
					SupportedViaTicker: true,
					SupportsRestBatch:  true,
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
					kline.IntervalCapacity{Interval: kline.SixHour},
					kline.IntervalCapacity{Interval: kline.TwelveHour},
					kline.IntervalCapacity{Interval: kline.OneDay},
					kline.IntervalCapacity{Interval: kline.ThreeDay},
				),
				GlobalResultLimit: maxResultLimit,
			},
		},
		Subscriptions: defaultSubscriptions.Clone(),
	}

	e.Requester, err = request.New(e.Name,
		common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout),
		request.WithLimiter(request.NewBasicRateLimit(bitstampRateInterval, bitstampRequestRate, 1)))
	if err != nil {
		log.Errorln(log.ExchangeSys, err)
	}
	e.API.Endpoints = e.NewEndpoints()
	err = e.API.Endpoints.SetDefaultEndpoints(map[exchange.URL]string{
		exchange.RestSpot:      bitstampAPIURL,
		exchange.WebsocketSpot: bitstampWSURL,
	})
	if err != nil {
		log.Errorln(log.ExchangeSys, err)
	}
	e.Websocket = websocket.NewManager()
	e.WebsocketResponseMaxLimit = exchange.DefaultWebsocketResponseMaxLimit
	e.WebsocketResponseCheckTimeout = exchange.DefaultWebsocketResponseCheckTimeout
	e.WebsocketOrderbookBufferLimit = exchange.DefaultWebsocketOrderbookBufferLimit
}

// Setup sets configuration values to bitstamp
func (e *Exchange) Setup(exch *config.Exchange) error {
	err := exch.Validate()
	if err != nil {
		return err
	}
	if !exch.Enabled {
		e.SetEnabled(false)
		return nil
	}
	err = e.SetupDefaults(exch)
	if err != nil {
		return err
	}

	wsURL, err := e.API.Endpoints.GetURL(exchange.WebsocketSpot)
	if err != nil {
		return err
	}

	err = e.Websocket.Setup(&websocket.ManagerSetup{
		ExchangeConfig:        exch,
		DefaultURL:            bitstampWSURL,
		RunningURL:            wsURL,
		Connector:             e.WsConnect,
		Subscriber:            e.Subscribe,
		Unsubscriber:          e.Unsubscribe,
		GenerateSubscriptions: e.generateSubscriptions,
		Features:              &e.Features.Supports.WebsocketCapabilities,
	})
	if err != nil {
		return err
	}

	return e.Websocket.SetupNewConnection(&websocket.ConnectionSetup{
		URL:                  e.Websocket.GetWebsocketURL(),
		ResponseCheckTimeout: exch.WebsocketResponseCheckTimeout,
		ResponseMaxLimit:     exch.WebsocketResponseMaxLimit,
	})
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (e *Exchange) FetchTradablePairs(ctx context.Context, a asset.Item) (currency.Pairs, error) {
	marketType, err := marketTypeForAsset(a)
	if err != nil {
		return nil, err
	}
	markets, err := e.GetMarkets(ctx, "")
	if err != nil {
		return nil, err
	}
	return tradablePairs(markets, marketType)
}

// UpdateTradablePairs updates the exchanges available pairs and stores
// them in the exchanges config
func (e *Exchange) UpdateTradablePairs(ctx context.Context) error {
	markets, err := e.GetMarkets(ctx, "")
	if err != nil {
		return err
	}
	for _, a := range e.GetAssetTypes(false) {
		marketType, err := marketTypeForAsset(a)
		if err != nil {
			return err
		}
		pairs, err := tradablePairs(markets, marketType)
		if err != nil {
			return err
		}
		if err := e.UpdatePairs(pairs, a, false); err != nil {
			return err
		}
	}
	return e.EnsureOnePairEnabled()
}

// tradablePairs returns the pairs of the markets of a market type which have trading enabled
func tradablePairs(markets []MarketResponse, marketType string) (currency.Pairs, error) {
	pairs := make(currency.Pairs, 0, len(markets))
	for i := range markets {
		if markets[i].MarketType != marketType || markets[i].Trading != tradingEnabled {
			continue
		}
		pair, err := currency.NewPairDelimiter(markets[i].Name, currency.ForwardSlashDelimiter)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}
	return pairs, nil
}

// UpdateOrderExecutionLimits sets exchange execution order limits for an asset type
func (e *Exchange) UpdateOrderExecutionLimits(ctx context.Context, a asset.Item) error {
	marketType, err := marketTypeForAsset(a)
	if err != nil {
		return err
	}
	markets, err := e.GetMarkets(ctx, "")
	if err != nil {
		return err
	}
	l := make([]limits.MinMaxLevel, 0, len(markets))
	for i := range markets {
		m := &markets[i]
		if m.MarketType != marketType || m.Trading != tradingEnabled {
			continue
		}
		pair, err := currency.NewPairDelimiter(m.Name, currency.ForwardSlashDelimiter)
		if err != nil {
			return err
		}
		priceStep := math.Pow10(-int(m.CounterDecimals))
		if m.TickSize > 0 {
			priceStep = m.TickSize.Float64()
		}
		l = append(l, limits.MinMaxLevel{
			Key:                     key.NewExchangeAssetPair(e.Name, a, pair),
			PriceStepIncrementSize:  priceStep,
			AmountStepIncrementSize: math.Pow10(-int(m.BaseDecimals)),
			MinimumBaseAmount:       m.MinimumOrderAmount.Float64(),
			MaximumBaseAmount:       m.MaximumOrderAmount.Float64(),
			MinimumQuoteAmount:      m.MinimumOrderValue.Float64(),
			MaximumQuoteAmount:      m.MaximumOrderValue.Float64(),
		})
	}
	return limits.Load(l)
}

// UpdateTickers updates the ticker for all currency pairs of a given asset type
func (e *Exchange) UpdateTickers(ctx context.Context, a asset.Item) error {
	marketType, err := marketTypeForAsset(a)
	if err != nil {
		return err
	}
	tickers, err := e.GetTickers(ctx)
	if err != nil {
		return err
	}
	for i := range tickers {
		if tickers[i].MarketType != marketType {
			continue
		}
		pair, err := currency.NewPairDelimiter(tickers[i].Market, currency.ForwardSlashDelimiter)
		if err != nil {
			return err
		}
		if err := ticker.ProcessTicker(e.tickerPrice(&tickers[i], pair, a)); err != nil {
			return err
		}
	}
	return nil
}

// UpdateTicker updates and returns the ticker for a currency pair
func (e *Exchange) UpdateTicker(ctx context.Context, p currency.Pair, a asset.Item) (*ticker.Price, error) {
	if p.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if _, err := marketTypeForAsset(a); err != nil {
		return nil, err
	}
	fPair, err := e.FormatExchangeCurrency(p, a)
	if err != nil {
		return nil, err
	}
	tick, err := e.GetTicker(ctx, fPair)
	if err != nil {
		return nil, err
	}
	if err := ticker.ProcessTicker(e.tickerPrice(tick, fPair, a)); err != nil {
		return nil, err
	}
	return ticker.GetTicker(e.Name, fPair, a)
}

func (e *Exchange) tickerPrice(t *TickerResponse, pair currency.Pair, a asset.Item) *ticker.Price {
	return &ticker.Price{
		Last:         t.Last.Float64(),
		High:         t.High.Float64(),
		Low:          t.Low.Float64(),
		Bid:          t.Bid.Float64(),
		Ask:          t.Ask.Float64(),
		BaseVolume:   t.Volume.Float64(),
		Open:         t.Open.Float64(),
		OpenInterest: t.OpenInterest.Float64(),
		MarkPrice:    t.MarkPrice.Float64(),
		IndexPrice:   t.IndexPrice.Float64(),
		Pair:         pair,
		ExchangeName: e.Name,
		AssetType:    a,
		LastUpdated:  t.Timestamp.Time(),
	}
}

// GetFeeByType returns an estimate of fee based on type of transaction
func (e *Exchange) GetFeeByType(ctx context.Context, feeBuilder *exchange.FeeBuilder) (float64, error) {
	if err := common.NilGuard(feeBuilder); err != nil {
		return 0, err
	}
	if (!e.AreCredentialsValid(ctx) || e.SkipAuthCheck) && // Todo check connection status
		feeBuilder.FeeType == exchange.CryptocurrencyTradeFee {
		feeBuilder.FeeType = exchange.OfflineTradeFee
	}
	return e.GetFee(ctx, feeBuilder)
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (e *Exchange) UpdateOrderbook(ctx context.Context, p currency.Pair, assetType asset.Item) (*orderbook.Book, error) {
	if p.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if err := e.CurrencyPairs.IsAssetEnabled(assetType); err != nil {
		return nil, err
	}
	fPair, err := e.FormatExchangeCurrency(p, assetType)
	if err != nil {
		return nil, err
	}
	ob, err := e.GetOrderbook(ctx, fPair, OrderbookGroupedByPrice)
	if err != nil {
		return nil, err
	}
	book := &orderbook.Book{
		Exchange:          e.Name,
		Pair:              p,
		Asset:             assetType,
		ValidateOrderbook: e.ValidateOrderbook,
		LastUpdated:       ob.Microtimestamp.Time(),
		Bids:              orderbookLevels(ob.Bids),
		Asks:              orderbookLevels(ob.Asks),
	}
	filterOrderbookZeroBidPrice(book)
	if err := book.Process(); err != nil {
		return book, err
	}
	return orderbook.Get(e.Name, p, assetType)
}

func orderbookLevels(levels []OrderbookLevel) orderbook.Levels {
	l := make(orderbook.Levels, len(levels))
	for i := range levels {
		l[i] = orderbook.Level{Price: levels[i].Price, Amount: levels[i].Amount}
	}
	return l
}

// filterOrderbookZeroBidPrice removes the trailing zero priced bid Bitstamp includes in some orderbooks
func filterOrderbookZeroBidPrice(ob *orderbook.Book) {
	if len(ob.Bids) == 0 || ob.Bids[len(ob.Bids)-1].Price != 0 {
		return
	}
	ob.Bids = ob.Bids[0 : len(ob.Bids)-1]
}

// UpdateAccountBalances retrieves currency balances
// Perpetual contract balances are the collateral balances of the derivatives trade account
func (e *Exchange) UpdateAccountBalances(ctx context.Context, assetType asset.Item) (accounts.SubAccounts, error) {
	subAccts := accounts.SubAccounts{accounts.NewSubAccount(assetType, "")}
	switch assetType {
	case asset.Spot:
		balances, err := e.GetAccountBalances(ctx)
		if err != nil {
			return nil, err
		}
		for i := range balances {
			subAccts[0].Balances.Set(balances[i].Currency.Upper(), accounts.Balance{
				Total: balances[i].Total.Float64(),
				Hold:  balances[i].Reserved.Float64(),
				Free:  balances[i].Available.Float64(),
			})
		}
	case asset.PerpetualContract:
		info, err := e.GetMarginInfo(ctx)
		if err != nil {
			return nil, err
		}
		for i := range info.Assets {
			subAccts[0].Balances.Set(info.Assets[i].Asset.Upper(), accounts.Balance{
				Total: info.Assets[i].TotalAmount.Float64(),
				Hold:  info.Assets[i].Reserved.Float64(),
				Free:  info.Assets[i].Available.Float64(),
			})
		}
	default:
		return nil, fmt.Errorf("%w %q", asset.ErrNotSupported, assetType)
	}
	return subAccts, e.Accounts.Save(ctx, subAccts, true)
}

// GetAccountFundingHistory returns cryptocurrency deposits and withdrawals from the last 30 days
func (e *Exchange) GetAccountFundingHistory(ctx context.Context) ([]exchange.FundingHistory, error) {
	txs, err := e.GetCryptoTransactions(ctx, &CryptoTransactionsRequest{Limit: maxResultLimit})
	if err != nil {
		return nil, err
	}
	resp := make([]exchange.FundingHistory, 0, len(txs.Deposits)+len(txs.Withdrawals))
	for i := range txs.Deposits {
		d := &txs.Deposits[i]
		resp = append(resp, exchange.FundingHistory{
			ExchangeName:      e.Name,
			Status:            d.Status,
			TransferID:        strconv.FormatUint(d.ID, 10),
			Timestamp:         d.DateTime.Time(),
			Currency:          d.Currency.Upper().String(),
			Amount:            d.Amount.Float64(),
			TransferType:      "deposit",
			CryptoToAddress:   d.DestinationAddress,
			CryptoFromAddress: d.OriginatorAddress,
			CryptoTxID:        d.TxID,
			CryptoChain:       d.Network,
		})
	}
	for i := range txs.Withdrawals {
		w := &txs.Withdrawals[i]
		resp = append(resp, exchange.FundingHistory{
			ExchangeName:    e.Name,
			Timestamp:       w.DateTime.Time(),
			Currency:        w.Currency.Upper().String(),
			Amount:          w.Amount.Float64(),
			TransferType:    "withdrawal",
			CryptoToAddress: w.DestinationAddress,
			CryptoTxID:      w.TxID,
			CryptoChain:     w.Network,
		})
	}
	return resp, nil
}

// GetWithdrawalsHistory returns previous withdrawals data
func (e *Exchange) GetWithdrawalsHistory(ctx context.Context, c currency.Code, _ asset.Item) ([]exchange.WithdrawalHistory, error) {
	withdrawals, err := e.GetWithdrawalRequests(ctx, &WithdrawalRequestsRequest{TimeDelta: maxWithdrawalRequestsTimeDelta})
	if err != nil {
		return nil, err
	}
	resp := make([]exchange.WithdrawalHistory, 0, len(withdrawals))
	for i := range withdrawals {
		w := &withdrawals[i]
		if !c.IsEmpty() && !c.Equal(w.Currency) {
			continue
		}
		resp = append(resp, exchange.WithdrawalHistory{
			Status:          withdrawalStatus(w.Status),
			TransferID:      strconv.FormatUint(w.ID, 10),
			Timestamp:       w.DateTime.Time(),
			Currency:        w.Currency.Upper().String(),
			Amount:          w.Amount.Float64(),
			TransferType:    withdrawalType(w.Type),
			CryptoToAddress: w.Address,
			CryptoTxID:      w.TransactionID,
			CryptoChain:     w.Network,
		})
	}
	return resp, nil
}

// withdrawalStatus returns the description of a withdrawal request status
func withdrawalStatus(status uint8) string {
	switch status {
	case 0:
		return "Open"
	case 1:
		return "In process"
	case 2:
		return "Finished"
	case 3:
		return "Canceled"
	case 4:
		return "Failed"
	case 11:
		return "Reversed"
	default:
		return strconv.FormatUint(uint64(status), 10)
	}
}

// withdrawalType returns the description of a withdrawal request type; bank transfers have dedicated types and every
// other type identifies the cryptocurrency withdrawn
func withdrawalType(withdrawalType uint64) string {
	switch withdrawalType {
	case 0:
		return "SEPA"
	case 2:
		return "Wire transfer"
	default:
		return "Cryptocurrency"
	}
}

// GetRecentTrades returns the most recent trades for a currency and asset
func (e *Exchange) GetRecentTrades(ctx context.Context, p currency.Pair, assetType asset.Item) ([]trade.Data, error) {
	if p.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if _, err := marketTypeForAsset(assetType); err != nil {
		return nil, err
	}
	p, err := e.FormatExchangeCurrency(p, assetType)
	if err != nil {
		return nil, err
	}

	tradeData, err := e.GetTransactions(ctx, p, "")
	if err != nil {
		return nil, err
	}

	resp := make([]trade.Data, len(tradeData))
	for i := range tradeData {
		resp[i] = trade.Data{
			Exchange:     e.Name,
			TID:          strconv.FormatUint(tradeData[i].TradeID, 10),
			CurrencyPair: p,
			AssetType:    assetType,
			Side:         tradeData[i].Side.Side(),
			Price:        tradeData[i].Price.Float64(),
			Amount:       tradeData[i].Amount.Float64(),
			Timestamp:    tradeData[i].Date.Time(),
		}
	}

	if err := e.AddTradesToBuffer(resp...); err != nil {
		return nil, err
	}

	trade.SortByDate(resp)
	return resp, nil
}

// GetHistoricTrades returns historic trade data within the timeframe provided
func (e *Exchange) GetHistoricTrades(_ context.Context, _ currency.Pair, _ asset.Item, _, _ time.Time) ([]trade.Data, error) {
	return nil, common.ErrFunctionNotSupported
}

// SubmitOrder submits a new order
// Market orders with a QuoteAmount are placed as instant orders. Perpetual contract orders use cross margin unless
// isolated margin is requested and use the current leverage setting when Leverage is not set
func (e *Exchange) SubmitOrder(ctx context.Context, s *order.Submit) (*order.SubmitResponse, error) {
	if err := s.Validate(e.GetTradingRequirements()); err != nil {
		return nil, err
	}
	if _, err := marketTypeForAsset(s.AssetType); err != nil {
		return nil, err
	}
	fPair, err := e.FormatExchangeCurrency(s.Pair, s.AssetType)
	if err != nil {
		return nil, err
	}

	var p derivativesOrderParams
	if s.AssetType == asset.PerpetualContract {
		if p.MarginMode, err = formatMarginMode(s.MarginType); err != nil {
			return nil, err
		}
		p.Leverage = s.Leverage
		if p.Leverage == 0 {
			if p.Leverage, err = e.GetLeverage(ctx, s.AssetType, fPair, s.MarginType, s.Side); err != nil {
				return nil, err
			}
		}
		p.ReduceOnly = s.ReduceOnly
		p.Trigger = formatTrigger(s.TriggerPriceType)
	}
	p.ClientOrderID = s.ClientOrderID

	var resp *OrderResponse
	switch {
	case s.Type == order.Limit:
		req := &LimitOrderRequest{
			Pair:          fPair,
			Side:          s.Side,
			Amount:        s.Amount,
			Price:         s.Price,
			ClientOrderID: p.ClientOrderID,
			MarginMode:    p.MarginMode,
			Leverage:      p.Leverage,
			ReduceOnly:    p.ReduceOnly,
		}
		if err := setLimitOrderTimeInForce(req, s.TimeInForce, s.EndTime); err != nil {
			return nil, err
		}
		resp, err = e.PlaceLimitOrder(ctx, req)
	case s.Type == order.Market && s.QuoteAmount > 0:
		resp, err = e.PlaceInstantOrder(ctx, &InstantOrderRequest{
			Pair:            fPair,
			Side:            s.Side,
			Amount:          s.QuoteAmount,
			AmountInCounter: true,
			ClientOrderID:   p.ClientOrderID,
			MarginMode:      p.MarginMode,
			Leverage:        p.Leverage,
			ReduceOnly:      p.ReduceOnly,
		})
	case s.Type == order.Market:
		resp, err = e.PlaceMarketOrder(ctx, &MarketOrderRequest{
			Pair:          fPair,
			Side:          s.Side,
			Amount:        s.Amount,
			ClientOrderID: p.ClientOrderID,
			MarginMode:    p.MarginMode,
			Leverage:      p.Leverage,
			ReduceOnly:    p.ReduceOnly,
		})
	case s.AssetType == asset.PerpetualContract:
		resp, err = e.submitConditionalOrder(ctx, s, fPair, &p)
	default:
		return nil, fmt.Errorf("%w %s for %s", order.ErrUnsupportedOrderType, s.Type, s.AssetType)
	}
	if err != nil {
		return nil, err
	}
	return s.DeriveSubmitResponse(strconv.FormatUint(resp.ID, 10))
}

// submitConditionalOrder places a stop loss or take profit order on a derivatives market
func (e *Exchange) submitConditionalOrder(ctx context.Context, s *order.Submit, fPair currency.Pair, p *derivativesOrderParams) (*OrderResponse, error) {
	if s.TriggerPrice <= 0 {
		return nil, fmt.Errorf("%w: trigger price must be set for %s orders", order.ErrPriceMustBeSetIfLimitOrder, s.Type)
	}
	switch s.Type {
	case order.Stop, order.StopMarket, order.TakeProfit, order.TakeProfitMarket:
		subtype := OrderSubtypeStopLoss
		if s.Type.Is(order.TakeProfit) {
			subtype = OrderSubtypeTakeProfit
		}
		return e.PlaceMarketOrder(ctx, &MarketOrderRequest{
			Pair:          fPair,
			Side:          s.Side,
			Amount:        s.Amount,
			ClientOrderID: p.ClientOrderID,
			Subtype:       subtype,
			MarginMode:    p.MarginMode,
			Leverage:      p.Leverage,
			StopPrice:     s.TriggerPrice,
			Trigger:       p.Trigger,
			ReduceOnly:    p.ReduceOnly,
		})
	case order.StopLimit, order.TakeProfit | order.Limit:
		subtype := OrderSubtypeStopLossLimit
		if s.Type.Is(order.TakeProfit) {
			subtype = OrderSubtypeTakeProfitLimit
		}
		req := &LimitOrderRequest{
			Pair:          fPair,
			Side:          s.Side,
			Amount:        s.Amount,
			Price:         s.Price,
			ClientOrderID: p.ClientOrderID,
			Subtype:       subtype,
			MarginMode:    p.MarginMode,
			Leverage:      p.Leverage,
			StopPrice:     s.TriggerPrice,
			Trigger:       p.Trigger,
			ReduceOnly:    p.ReduceOnly,
		}
		if err := setLimitOrderTimeInForce(req, s.TimeInForce, s.EndTime); err != nil {
			return nil, err
		}
		return e.PlaceLimitOrder(ctx, req)
	default:
		return nil, fmt.Errorf("%w %s for %s", order.ErrUnsupportedOrderType, s.Type, s.AssetType)
	}
}

// setLimitOrderTimeInForce sets the limit order flags which match a time in force
func setLimitOrderTimeInForce(req *LimitOrderRequest, tif order.TimeInForce, endTime time.Time) error {
	if tif.Is(order.GoodTillCrossing) || tif.Is(order.StopOrReduce) {
		return fmt.Errorf("%w: %s", order.ErrUnsupportedTimeInForce, tif)
	}
	req.ImmediateOrCancel = tif.Is(order.ImmediateOrCancel)
	req.FillOrKill = tif.Is(order.FillOrKill)
	req.MakerOrCancel = tif.Is(order.PostOnly)
	req.DailyOrder = tif.Is(order.GoodTillDay)
	if tif.Is(order.GoodTillTime) {
		if endTime.IsZero() {
			return fmt.Errorf("%w: end time is required for %s orders", common.ErrDateUnset, tif)
		}
		req.GoodTillDate = true
		req.ExpireTime = endTime
	}
	return nil
}

// ModifyOrder modifies an existing order by atomically replacing it with a new order at the supplied amount and price
// The replacement order has a new order ID
func (e *Exchange) ModifyOrder(ctx context.Context, action *order.Modify) (*order.ModifyResponse, error) {
	if err := action.Validate(); err != nil {
		return nil, err
	}
	if _, err := marketTypeForAsset(action.AssetType); err != nil {
		return nil, err
	}
	req := &ReplaceOrderRequest{
		ClientOrderID: action.NewClientOrderID,
		Amount:        action.Amount,
		Price:         action.Price,
	}
	if action.OrderID != "" {
		orderID, err := strconv.ParseUint(action.OrderID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w %q: %w", order.ErrOrderIDNotSet, action.OrderID, err)
		}
		req.OrderID = orderID
	} else {
		req.OriginalClientOrderID = action.ClientOrderID
	}
	replaced, err := e.ReplaceOrder(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := action.DeriveModifyResponse()
	if err != nil {
		return nil, err
	}
	resp.OrderID = strconv.FormatUint(replaced.OrderID, 10)
	resp.ClientOrderID = action.NewClientOrderID
	return resp, nil
}

// CancelOrder cancels an order by its order ID, or by its client order ID when the order ID is not set
func (e *Exchange) CancelOrder(ctx context.Context, o *order.Cancel) error {
	if err := o.Validate(); err != nil {
		return err
	}
	req := &CancelOrderRequest{ClientOrderID: o.ClientOrderID}
	if o.OrderID != "" {
		orderID, err := strconv.ParseUint(o.OrderID, 10, 64)
		if err != nil {
			return fmt.Errorf("%w %q: %w", order.ErrOrderIDNotSet, o.OrderID, err)
		}
		// The API does not define which ID takes precedence when both are sent
		req = &CancelOrderRequest{OrderID: orderID}
	}
	_, err := e.CancelExistingOrder(ctx, req)
	return err
}

// CancelBatchOrders cancels an orders by their corresponding ID numbers
func (e *Exchange) CancelBatchOrders(_ context.Context, _ []order.Cancel) (*order.CancelBatchResponse, error) {
	return nil, common.ErrFunctionNotSupported
}

// CancelAllOrders cancels all orders for a currency pair, or for every market of the asset type when no pair is set
// Status holds the cancelled orders keyed by order ID
func (e *Exchange) CancelAllOrders(ctx context.Context, req *order.Cancel) (order.CancelAllResponse, error) {
	if err := req.Validate(); err != nil {
		return order.CancelAllResponse{}, err
	}
	if _, err := marketTypeForAsset(req.AssetType); err != nil {
		return order.CancelAllResponse{}, err
	}
	var markets currency.Pairs
	if !req.Pair.IsEmpty() {
		fPair, err := e.FormatExchangeCurrency(req.Pair, req.AssetType)
		if err != nil {
			return order.CancelAllResponse{}, err
		}
		markets = currency.Pairs{fPair}
	} else {
		// Cancelling all orders without a market would also cancel the orders of every other asset type
		openOrders, err := e.GetOpenOrders(ctx, currency.EMPTYPAIR)
		if err != nil {
			return order.CancelAllResponse{}, err
		}
		for i := range openOrders {
			pair, a, err := marketPairAsset(openOrders[i].Market)
			if err != nil {
				return order.CancelAllResponse{}, err
			}
			if a == req.AssetType && !markets.Contains(pair, true) {
				markets = append(markets, pair)
			}
		}
	}

	resp := order.CancelAllResponse{Status: make(map[string]string)}
	var errs error
	for _, market := range markets {
		cancelled, err := e.CancelAllExistingOrders(ctx, market)
		if err != nil {
			errs = common.AppendError(errs, fmt.Errorf("%s: %w", market, err))
			continue
		}
		for i := range cancelled.Canceled {
			resp.Status[strconv.FormatUint(cancelled.Canceled[i].ID, 10)] = order.Cancelled.String()
		}
		if !cancelled.Success {
			errs = common.AppendError(errs, fmt.Errorf("%s: %w", market, errCancelAllOrdersFailed))
		}
	}
	return resp, errs
}

// GetOrderInfo returns order information based on order ID
// Orders closed more than 30 days ago are not found
func (e *Exchange) GetOrderInfo(ctx context.Context, orderID string, _ currency.Pair, _ asset.Item) (*order.Detail, error) {
	id, err := strconv.ParseUint(orderID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", order.ErrOrderIDNotSet, orderID, err)
	}
	status, err := e.GetOrderStatus(ctx, &OrderStatusRequest{OrderID: id})
	if err != nil {
		return nil, err
	}
	pair, a, err := marketPairAsset(status.Market)
	if err != nil {
		return nil, err
	}

	d := &order.Detail{
		Exchange:        e.Name,
		OrderID:         strconv.FormatUint(status.ID, 10),
		ClientOrderID:   status.ClientOrderID,
		Type:            orderTypeFromSubtype(status.Subtype),
		Side:            status.Side.Side(),
		AssetType:       a,
		Pair:            pair,
		Date:            status.DateTime.Time(),
		RemainingAmount: status.AmountRemaining.Float64(),
		Leverage:        status.Leverage.Float64(),
		MarginType:      marginTypeFromMode(status.MarginMode),
		TriggerPrice:    status.StopPrice.Float64(),
		Trades:          make([]order.TradeHistory, len(status.Transactions)),
	}
	for i := range status.Transactions {
		tx := &status.Transactions[i]
		amount := math.Abs(tx.Amounts[pair.Base.Upper()])
		d.Trades[i] = order.TradeHistory{
			TID:       strconv.FormatUint(tx.TradeID, 10),
			Price:     tx.Price,
			Amount:    amount,
			Fee:       tx.Fee,
			Exchange:  e.Name,
			Side:      d.Side,
			Timestamp: tx.DateTime,
		}
		d.ExecutedAmount += amount
		d.Cost += amount * tx.Price
		d.Fee += tx.Fee
		if tx.DateTime.After(d.LastUpdated) {
			d.LastUpdated = tx.DateTime
		}
	}
	d.Amount = d.ExecutedAmount + d.RemainingAmount
	if d.ExecutedAmount > 0 {
		d.AverageExecutedPrice = d.Cost / d.ExecutedAmount
	}
	d.Status = orderStatus(status.Status, d.ExecutedAmount > 0)
	return d, nil
}

// orderStatus returns the order status of an order status description
func orderStatus(status string, partiallyExecuted bool) order.Status {
	switch status {
	case "Open":
		if partiallyExecuted {
			return order.PartiallyFilled
		}
		return order.Open
	case "Finished":
		return order.Filled
	case "Canceled":
		if partiallyExecuted {
			return order.PartiallyCancelled
		}
		return order.Cancelled
	case "Expired":
		return order.Expired
	default:
		return order.UnknownStatus
	}
}

// GetDepositAddress returns a deposit address for a specified currency
func (e *Exchange) GetDepositAddress(ctx context.Context, cryptocurrency currency.Code, _, chain string) (*deposit.Address, error) {
	addr, err := e.GetCryptoDepositAddress(ctx, cryptocurrency, chain)
	if err != nil {
		return nil, err
	}
	tag := addr.MemoID
	if addr.DestinationTag != 0 {
		tag = strconv.FormatUint(addr.DestinationTag, 10)
	}
	return &deposit.Address{
		Address: addr.Address,
		Tag:     tag,
		Chain:   chain,
	}, nil
}

// GetAvailableTransferChains returns the networks which support deposits or withdrawals of a currency
func (e *Exchange) GetAvailableTransferChains(ctx context.Context, cryptocurrency currency.Code) ([]string, error) {
	if cryptocurrency.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	currencies, err := e.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	for i := range currencies {
		if !currencies[i].Currency.Equal(cryptocurrency) {
			continue
		}
		chains := make([]string, 0, len(currencies[i].Networks))
		for j := range currencies[i].Networks {
			n := &currencies[i].Networks[j]
			if n.Deposit == tradingEnabled || n.Withdrawal == tradingEnabled {
				chains = append(chains, n.Network)
			}
		}
		return chains, nil
	}
	return nil, fmt.Errorf("%w: %s", currency.ErrCurrencyNotFound, cryptocurrency)
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is
// submitted
func (e *Exchange) WithdrawCryptocurrencyFunds(ctx context.Context, withdrawRequest *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	if err := withdrawRequest.Validate(); err != nil {
		return nil, err
	}
	req := &CryptoWithdrawalRequest{
		Currency: withdrawRequest.Currency,
		Network:  withdrawRequest.Crypto.Chain,
		Amount:   withdrawRequest.Amount,
		Address:  withdrawRequest.Crypto.Address,
	}
	if withdrawRequest.Crypto.AddressTag != "" {
		// Ripple uses destination tags whereas other tagged currencies such as XLM, HBAR and ATOM use memo IDs
		if withdrawRequest.Currency.Equal(currency.XRP) {
			req.DestinationTag = withdrawRequest.Crypto.AddressTag
		} else {
			req.MemoID = withdrawRequest.Crypto.AddressTag
		}
	}
	resp, err := e.CryptoWithdrawal(ctx, req)
	if err != nil {
		return nil, err
	}
	return &withdraw.ExchangeResponse{
		ID: strconv.FormatUint(resp.ID, 10),
	}, nil
}

// WithdrawFiatFunds returns a withdrawal ID when a
// withdrawal is submitted
func (e *Exchange) WithdrawFiatFunds(ctx context.Context, withdrawRequest *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	if err := withdrawRequest.Validate(); err != nil {
		return nil, err
	}
	resp, err := e.OpenBankWithdrawal(ctx, &BankWithdrawalRequest{
		Amount:          withdrawRequest.Amount,
		AccountCurrency: withdrawRequest.Currency,
		Name:            withdrawRequest.Fiat.Bank.AccountName,
		IBAN:            withdrawRequest.Fiat.Bank.IBAN,
		BIC:             withdrawRequest.Fiat.Bank.SWIFTCode,
		Address:         withdrawRequest.Fiat.Bank.BankAddress,
		PostalCode:      withdrawRequest.Fiat.Bank.BankPostalCode,
		City:            withdrawRequest.Fiat.Bank.BankPostalCity,
		Country:         withdrawRequest.Fiat.Bank.BankCountry,
		Comment:         withdrawRequest.Description,
		Type:            BankWithdrawalTypeSEPA,
	})
	if err != nil {
		return nil, err
	}
	return &withdraw.ExchangeResponse{
		ID: strconv.FormatUint(resp.WithdrawalID, 10),
	}, nil
}

// WithdrawFiatFundsToInternationalBank returns a withdrawal ID when a
// withdrawal is submitted
func (e *Exchange) WithdrawFiatFundsToInternationalBank(ctx context.Context, withdrawRequest *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	if err := withdrawRequest.Validate(); err != nil {
		return nil, err
	}
	resp, err := e.OpenBankWithdrawal(ctx, &BankWithdrawalRequest{
		Amount:          withdrawRequest.Amount,
		AccountCurrency: withdrawRequest.Currency,
		Name:            withdrawRequest.Fiat.Bank.AccountName,
		IBAN:            withdrawRequest.Fiat.Bank.IBAN,
		BIC:             withdrawRequest.Fiat.Bank.SWIFTCode,
		Address:         withdrawRequest.Fiat.Bank.BankAddress,
		PostalCode:      withdrawRequest.Fiat.Bank.BankPostalCode,
		City:            withdrawRequest.Fiat.Bank.BankPostalCity,
		Country:         withdrawRequest.Fiat.Bank.BankCountry,
		Comment:         withdrawRequest.Description,
		Type:            BankWithdrawalTypeInternational,
		BankName:        withdrawRequest.Fiat.IntermediaryBankName,
		BankAddress:     withdrawRequest.Fiat.IntermediaryBankAddress,
		BankPostalCode:  withdrawRequest.Fiat.IntermediaryBankPostalCode,
		BankCity:        withdrawRequest.Fiat.IntermediaryBankCity,
		BankCountry:     withdrawRequest.Fiat.IntermediaryBankCountry,
		Currency:        currency.NewCode(withdrawRequest.Fiat.WireCurrency),
	})
	if err != nil {
		return nil, err
	}
	return &withdraw.ExchangeResponse{
		ID: strconv.FormatUint(resp.WithdrawalID, 10),
	}, nil
}

// GetActiveOrders retrieves any orders that are active/open
func (e *Exchange) GetActiveOrders(ctx context.Context, req *order.MultiOrderRequest) (order.FilteredOrders, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, err := marketTypeForAsset(req.AssetType); err != nil {
		return nil, err
	}
	var pair currency.Pair
	if len(req.Pairs) == 1 {
		var err error
		if pair, err = e.FormatExchangeCurrency(req.Pairs[0], req.AssetType); err != nil {
			return nil, err
		}
	}
	openOrders, err := e.GetOpenOrders(ctx, pair)
	if err != nil {
		return nil, err
	}
	orders := make([]order.Detail, 0, len(openOrders))
	for i := range openOrders {
		o := &openOrders[i]
		p, a, err := marketPairAsset(o.Market)
		if err != nil {
			return nil, err
		}
		if a != req.AssetType {
			continue
		}
		amount := o.AmountAtCreate.Float64()
		if amount == 0 {
			amount = o.Amount.Float64()
		}
		executed := amount - o.Amount.Float64()
		status := order.Open
		if executed > 0 {
			status = order.PartiallyFilled
		}
		orders = append(orders, order.Detail{
			Exchange:        e.Name,
			OrderID:         strconv.FormatUint(o.ID, 10),
			ClientOrderID:   o.ClientOrderID,
			Type:            orderTypeFromSubtype(o.Subtype),
			Side:            o.Side.Side(),
			Status:          status,
			AssetType:       a,
			Pair:            p,
			Date:            o.DateTime.Time(),
			Price:           o.Price.Float64(),
			Amount:          amount,
			ExecutedAmount:  executed,
			RemainingAmount: o.Amount.Float64(),
			Leverage:        o.Leverage.Float64(),
			MarginType:      marginTypeFromMode(o.MarginMode),
			TriggerPrice:    o.StopPrice.Float64(),
			ReduceOnly:      o.ReduceOnly,
		})
	}
	return req.Filter(e.Name, orders), nil
}

// GetOrderHistory retrieves the executed orders of the last 30 days, aggregated from their trades
func (e *Exchange) GetOrderHistory(ctx context.Context, req *order.MultiOrderRequest) (order.FilteredOrders, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var orders []order.Detail
	var err error
	switch req.AssetType {
	case asset.Spot:
		orders, err = e.spotOrderHistory(ctx, req)
	case asset.PerpetualContract:
		pairs := req.Pairs
		if len(pairs) == 0 {
			if pairs, err = e.GetEnabledPairs(req.AssetType); err != nil {
				return nil, err
			}
		}
		orders, err = e.derivativesOrderHistory(ctx, pairs, req.StartTime, req.EndTime)
	default:
		return nil, fmt.Errorf("%w %q", asset.ErrNotSupported, req.AssetType)
	}
	if err != nil {
		return nil, err
	}
	return req.Filter(e.Name, orders), nil
}

// spotOrderHistory returns spot orders aggregated from the account's market trade transactions
func (e *Exchange) spotOrderHistory(ctx context.Context, req *order.MultiOrderRequest) ([]order.Detail, error) {
	params := &UserTransactionsRequest{
		Limit: maxResultLimit,
		Since: withinHistoryLimit(req.StartTime),
		Until: withinHistoryLimit(req.EndTime),
	}
	if len(req.Pairs) == 1 {
		fPair, err := e.FormatExchangeCurrency(req.Pairs[0], asset.Spot)
		if err != nil {
			return nil, err
		}
		params.Pair = fPair
	}
	txs, err := e.GetUserTransactions(ctx, params)
	if err != nil {
		return nil, err
	}
	aggregated := make(map[uint64]*order.Detail)
	for i := range txs {
		tx := &txs[i]
		if tx.Type != TransactionTypeMarketTrade || tx.OrderID == 0 {
			continue
		}
		pair, price, ok := tradedPair(tx)
		if !ok {
			continue
		}
		baseAmount := tx.Amounts[pair.Base]
		side := order.Buy
		if baseAmount < 0 {
			side = order.Sell
		}
		d, ok := aggregated[tx.OrderID]
		if !ok {
			d = &order.Detail{
				Exchange:  e.Name,
				OrderID:   strconv.FormatUint(tx.OrderID, 10),
				Side:      side,
				AssetType: asset.Spot,
				Pair:      pair,
				FeeAsset:  pair.Quote,
				CostAsset: pair.Quote,
				Date:      tx.DateTime,
			}
			aggregated[tx.OrderID] = d
		}
		addTrade(d, &order.TradeHistory{
			TID:       strconv.FormatUint(tx.ID, 10),
			Price:     price,
			Amount:    math.Abs(baseAmount),
			Fee:       tx.Fee,
			Exchange:  e.Name,
			Side:      side,
			Timestamp: tx.DateTime,
			FeeAsset:  pair.Quote.String(),
		})
	}
	return aggregatedOrders(aggregated), nil
}

// tradedPair returns the market and execution price of a trade transaction from its exchange rates
func tradedPair(tx *UserTransactionResponse) (currency.Pair, float64, bool) {
	for pair, rate := range tx.ExchangeRates {
		if rate != 0 && tx.Amounts[pair.Base] != 0 {
			return pair, rate, true
		}
	}
	return currency.EMPTYPAIR, 0, false
}

// derivativesOrderHistory returns derivatives orders aggregated from the account's trades on each market
func (e *Exchange) derivativesOrderHistory(ctx context.Context, pairs currency.Pairs, start, end time.Time) ([]order.Detail, error) {
	var orders []order.Detail
	for _, p := range pairs {
		fPair, err := e.FormatExchangeCurrency(p, asset.PerpetualContract)
		if err != nil {
			return nil, err
		}
		trades, err := e.GetDerivativesTradeHistory(ctx, &DerivativesTradeHistoryRequest{
			Pair:  fPair,
			Limit: maxResultLimit,
			Since: start,
			Until: end,
		})
		if err != nil {
			return nil, err
		}
		aggregated := make(map[uint64]*order.Detail)
		for i := range trades {
			t := &trades[i]
			side, err := order.StringToOrderSide(t.Side)
			if err != nil {
				return nil, err
			}
			d, ok := aggregated[t.OrderID]
			if !ok {
				d = &order.Detail{
					Exchange:   e.Name,
					OrderID:    strconv.FormatUint(t.OrderID, 10),
					Side:       side,
					AssetType:  asset.PerpetualContract,
					Pair:       fPair,
					FeeAsset:   t.FeeCurrency.Upper(),
					Leverage:   t.Leverage.Float64(),
					MarginType: marginTypeFromMode(t.MarginMode),
					Date:       t.DateTime.Time(),
				}
				aggregated[t.OrderID] = d
			}
			addTrade(d, &order.TradeHistory{
				TID:       strconv.FormatUint(t.TradeID, 10),
				Price:     t.Price.Float64(),
				Amount:    t.Amount.Float64(),
				Fee:       t.Fee.Float64() + t.LiquidationFee.Float64(),
				Exchange:  e.Name,
				Side:      side,
				Timestamp: t.DateTime.Time(),
				FeeAsset:  t.FeeCurrency.Upper().String(),
			})
		}
		orders = append(orders, aggregatedOrders(aggregated)...)
	}
	return orders, nil
}

// addTrade adds an executed trade to an order
func addTrade(d *order.Detail, t *order.TradeHistory) {
	d.Trades = append(d.Trades, *t)
	d.Amount += t.Amount
	d.ExecutedAmount += t.Amount
	d.Cost += t.Amount * t.Price
	d.Fee += t.Fee
	if t.Timestamp.Before(d.Date) {
		d.Date = t.Timestamp
	}
	if t.Timestamp.After(d.LastUpdated) {
		d.LastUpdated = t.Timestamp
	}
}

// aggregatedOrders returns orders aggregated from their trades sorted by order date
func aggregatedOrders(aggregated map[uint64]*order.Detail) []order.Detail {
	orders := make([]order.Detail, 0, len(aggregated))
	for _, d := range aggregated {
		if d.ExecutedAmount > 0 {
			d.AverageExecutedPrice = d.Cost / d.ExecutedAmount
			d.Price = d.AverageExecutedPrice
		}
		orders = append(orders, *d)
	}
	slices.SortFunc(orders, func(a, b order.Detail) int {
		return a.Date.Compare(b.Date)
	})
	return orders
}

// withinHistoryLimit returns the supplied time if it is within the history retention period, otherwise zero so the
// request uses the API's default range
func withinHistoryLimit(t time.Time) time.Time {
	if t.IsZero() || t.Before(time.Now().Add(-maxHistoryAge+historyAgeBuffer)) {
		return time.Time{}
	}
	return t
}

// ValidateAPICredentials validates current credentials used for wrapper functionality
func (e *Exchange) ValidateAPICredentials(ctx context.Context, assetType asset.Item) error {
	_, err := e.UpdateAccountBalances(ctx, assetType)
	return e.CheckTransientError(err)
}

// GetHistoricCandles returns candles between a time period for a set time interval
func (e *Exchange) GetHistoricCandles(ctx context.Context, pair currency.Pair, a asset.Item, interval kline.Interval, start, end time.Time) (*kline.Item, error) {
	req, err := e.GetKlineRequest(pair, a, interval, start, end, false)
	if err != nil {
		return nil, err
	}
	candles, err := e.candlesInRange(ctx, req.RequestFormatted, req.ExchangeInterval, req.Start, req.End)
	if err != nil {
		return nil, err
	}
	return req.ProcessResponse(candles)
}

// GetHistoricCandlesExtended returns candles between a time period for a set time interval
func (e *Exchange) GetHistoricCandlesExtended(ctx context.Context, pair currency.Pair, a asset.Item, interval kline.Interval, start, end time.Time) (*kline.Item, error) {
	req, err := e.GetKlineExtendedRequest(pair, a, interval, start, end)
	if err != nil {
		return nil, err
	}
	timeSeries := make([]kline.Candle, 0, req.Size())
	for x := range req.RangeHolder.Ranges {
		candles, err := e.candlesInRange(ctx, req.RequestFormatted, req.ExchangeInterval, req.RangeHolder.Ranges[x].Start.Time, req.RangeHolder.Ranges[x].End.Time)
		if err != nil {
			return nil, err
		}
		timeSeries = append(timeSeries, candles...)
	}
	return req.ProcessResponse(timeSeries)
}

// candlesInRange returns the candles starting within a time range
// The API ignores the start time when an end time is also sent and returns the limit of candles before the end time,
// so only the start time and the number of candles in the range are sent
func (e *Exchange) candlesInRange(ctx context.Context, pair currency.Pair, interval kline.Interval, start, end time.Time) ([]kline.Candle, error) {
	resp, err := e.GetOHLC(ctx, &OHLCRequest{
		Pair:  pair,
		Step:  interval,
		Limit: kline.TotalCandlesPerInterval(start, end, interval),
		Start: start,
	})
	if err != nil {
		return nil, err
	}
	candles := make([]kline.Candle, len(resp.Data.OHLC))
	for i := range resp.Data.OHLC {
		c := &resp.Data.OHLC[i]
		candles[i] = kline.Candle{
			Time:   c.Timestamp.Time(),
			Open:   c.Open.Float64(),
			High:   c.High.Float64(),
			Low:    c.Low.Float64(),
			Close:  c.Close.Float64(),
			Volume: c.Volume.Float64(),
		}
	}
	return candles, nil
}

// GetServerTime returns the current exchange server time.
func (e *Exchange) GetServerTime(_ context.Context, _ asset.Item) (time.Time, error) {
	return time.Time{}, common.ErrFunctionNotSupported
}

// GetFuturesContractDetails returns all contracts from the exchange by asset type
func (e *Exchange) GetFuturesContractDetails(ctx context.Context, a asset.Item) ([]futures.Contract, error) {
	if !a.IsFutures() {
		return nil, futures.ErrNotFuturesAsset
	}
	if a != asset.PerpetualContract {
		return nil, fmt.Errorf("%w %q", asset.ErrNotSupported, a)
	}
	markets, err := e.GetMarkets(ctx, "")
	if err != nil {
		return nil, err
	}
	contracts := make([]futures.Contract, 0, len(markets))
	for i := range markets {
		m := &markets[i]
		if m.MarketType != MarketTypePerpetual {
			continue
		}
		pair, err := currency.NewPairDelimiter(m.Name, currency.ForwardSlashDelimiter)
		if err != nil {
			return nil, err
		}
		settlementType := futures.UnsetSettlementType
		if strings.EqualFold(m.PayoffType, "Linear") {
			settlementType = futures.Linear
		}
		contracts = append(contracts, futures.Contract{
			Exchange:           e.Name,
			Name:               pair,
			Underlying:         currency.NewPair(m.BaseCurrency.Upper(), m.CounterCurrency.Upper()),
			Asset:              a,
			IsActive:           m.Trading == tradingEnabled,
			Status:             m.Trading,
			Type:               futures.Perpetual,
			SettlementType:     settlementType,
			SettlementCurrency: m.CounterCurrency.Upper(),
			MarginCurrency:     m.CounterCurrency.Upper(),
			Multiplier:         m.ContractSize.Float64(),
			MaxLeverage:        m.MaxLeverage.Float64(),
		})
	}
	return contracts, nil
}

// GetLatestFundingRates returns the current funding rates of perpetual markets
// When no pair is set the rates of all enabled pairs are returned. The current rate is settled at the next funding
// time, so it is also returned as the predicted rate when requested
func (e *Exchange) GetLatestFundingRates(ctx context.Context, r *fundingrate.LatestRateRequest) ([]fundingrate.LatestRateResponse, error) {
	if err := common.NilGuard(r); err != nil {
		return nil, err
	}
	if isPerp, err := e.IsPerpetualFutureCurrency(r.Asset, r.Pair); err != nil {
		return nil, err
	} else if !isPerp {
		return nil, fmt.Errorf("%w %s %s", futures.ErrNotPerpetualFuture, r.Asset, r.Pair)
	}
	pairs := currency.Pairs{r.Pair}
	if r.Pair.IsEmpty() {
		var err error
		if pairs, err = e.GetEnabledPairs(r.Asset); err != nil {
			return nil, err
		}
	}
	resp := make([]fundingrate.LatestRateResponse, 0, len(pairs))
	for _, p := range pairs {
		fPair, err := e.FormatExchangeCurrency(p, r.Asset)
		if err != nil {
			return nil, err
		}
		rate, err := e.GetFundingRate(ctx, fPair)
		if err != nil {
			return nil, err
		}
		latest := fundingrate.LatestRateResponse{
			Exchange: e.Name,
			Asset:    r.Asset,
			Pair:     fPair,
			LatestRate: fundingrate.Rate{
				Time: rate.Timestamp.Time(),
				Rate: rate.FundingRate.Decimal(),
			},
			TimeOfNextRate: rate.NextFundingTime.Time(),
			TimeChecked:    time.Now(),
		}
		if r.IncludePredictedRate {
			latest.PredictedUpcomingRate = fundingrate.Rate{
				Time: rate.NextFundingTime.Time(),
				Rate: rate.FundingRate.Decimal(),
			}
		}
		resp = append(resp, latest)
	}
	return resp, nil
}

// GetHistoricalFundingRates returns funding rates for a perpetual market from up to 30 days ago
func (e *Exchange) GetHistoricalFundingRates(ctx context.Context, r *fundingrate.HistoricalRatesRequest) (*fundingrate.HistoricalRates, error) {
	if err := common.NilGuard(r); err != nil {
		return nil, err
	}
	if r.Asset != asset.PerpetualContract {
		return nil, fmt.Errorf("%w %s %s", futures.ErrNotPerpetualFuture, r.Asset, r.Pair)
	}
	if r.Pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if r.IncludePayments {
		return nil, fmt.Errorf("%w: funding payments", common.ErrFunctionNotSupported)
	}
	if !r.StartDate.IsZero() && !r.EndDate.IsZero() {
		if err := common.StartEndTimeCheck(r.StartDate, r.EndDate); err != nil {
			return nil, err
		}
	}
	if earliest := time.Now().Add(-maxHistoryAge + historyAgeBuffer); !r.StartDate.IsZero() && r.StartDate.Before(earliest) {
		if !r.RespectHistoryLimits {
			return nil, fmt.Errorf("%w: start date %s is more than %s ago", fundingrate.ErrFundingRateOutsideLimits, r.StartDate, maxHistoryAge)
		}
		r.StartDate = earliest
	}
	fPair, err := e.FormatExchangeCurrency(r.Pair, r.Asset)
	if err != nil {
		return nil, err
	}
	history, err := e.GetFundingRateHistory(ctx, &FundingRateHistoryRequest{
		Pair:  fPair,
		Limit: maxFundingRateHistoryLimit,
		Since: r.StartDate,
		Until: r.EndDate,
	})
	if err != nil {
		return nil, err
	}
	rates := make([]fundingrate.Rate, 0, len(history.FundingRateHistory))
	for i := range history.FundingRateHistory {
		rates = append(rates, fundingrate.Rate{
			Time: history.FundingRateHistory[i].Timestamp.Time(),
			Rate: history.FundingRateHistory[i].FundingRate.Decimal(),
		})
	}
	if len(rates) == 0 {
		return nil, fundingrate.ErrNoFundingRatesFound
	}
	slices.SortFunc(rates, func(a, b fundingrate.Rate) int {
		return a.Time.Compare(b.Time)
	})
	return &fundingrate.HistoricalRates{
		Exchange:     e.Name,
		Asset:        r.Asset,
		Pair:         fPair,
		StartDate:    rates[0].Time,
		EndDate:      rates[len(rates)-1].Time,
		LatestRate:   rates[len(rates)-1],
		FundingRates: rates,
	}, nil
}

// IsPerpetualFutureCurrency ensures a given asset and currency is a perpetual future
func (e *Exchange) IsPerpetualFutureCurrency(a asset.Item, _ currency.Pair) (bool, error) {
	return a == asset.PerpetualContract, nil
}

// GetCollateralCurrencyForContract returns the settlement currency of a perpetual contract
func (e *Exchange) GetCollateralCurrencyForContract(a asset.Item, cp currency.Pair) (currency.Code, asset.Item, error) {
	if a != asset.PerpetualContract {
		return currency.EMPTYCODE, asset.Empty, fmt.Errorf("%w %q", futures.ErrNotPerpetualFuture, a)
	}
	if cp.IsEmpty() {
		return currency.EMPTYCODE, asset.Empty, currency.ErrCurrencyPairEmpty
	}
	return currency.NewCode(strings.TrimSuffix(cp.Quote.Upper().String(), perpetualMarketSuffix)).Upper(), a, nil
}

// GetOpenInterest returns the open interest of perpetual markets in their base currency
func (e *Exchange) GetOpenInterest(ctx context.Context, keys ...key.PairAsset) ([]futures.OpenInterest, error) {
	for i := range keys {
		if keys[i].Asset != asset.PerpetualContract {
			// Avoid API calls or returning errors after a successful retrieval
			return nil, fmt.Errorf("%w %v %v", asset.ErrNotSupported, keys[i].Asset, keys[i].Pair())
		}
	}
	tickers, err := e.GetTickers(ctx)
	if err != nil {
		return nil, err
	}
	resp := make([]futures.OpenInterest, 0, len(tickers))
	for i := range tickers {
		if tickers[i].MarketType != MarketTypePerpetual {
			continue
		}
		pair, err := currency.NewPairDelimiter(tickers[i].Market, currency.ForwardSlashDelimiter)
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 && !slices.ContainsFunc(keys, func(k key.PairAsset) bool { return k.Pair().Equal(pair) }) {
			continue
		}
		resp = append(resp, futures.OpenInterest{
			Key:          key.NewExchangeAssetPair(e.Name, asset.PerpetualContract, pair),
			OpenInterest: tickers[i].OpenInterest.Float64(),
		})
	}
	return resp, nil
}

// SetLeverage sets the leverage used for a perpetual market and margin type
func (e *Exchange) SetLeverage(ctx context.Context, a asset.Item, pair currency.Pair, marginType margin.Type, amount float64, _ order.Side) error {
	if a != asset.PerpetualContract {
		return fmt.Errorf("%w %q", futures.ErrNotPerpetualFuture, a)
	}
	marginMode, err := formatMarginMode(marginType)
	if err != nil {
		return err
	}
	fPair, err := e.FormatExchangeCurrency(pair, a)
	if err != nil {
		return err
	}
	_, err = e.UpdateLeverageSetting(ctx, &LeverageSettingRequest{
		Pair:       fPair,
		MarginMode: marginMode,
		Leverage:   amount,
	})
	return err
}

// GetLeverage returns the leverage used for a perpetual market and margin type
func (e *Exchange) GetLeverage(ctx context.Context, a asset.Item, pair currency.Pair, marginType margin.Type, _ order.Side) (float64, error) {
	if a != asset.PerpetualContract {
		return 0, fmt.Errorf("%w %q", futures.ErrNotPerpetualFuture, a)
	}
	if pair.IsEmpty() {
		return 0, currency.ErrCurrencyPairEmpty
	}
	marginMode, err := formatMarginMode(marginType)
	if err != nil {
		return 0, err
	}
	fPair, err := e.FormatExchangeCurrency(pair, a)
	if err != nil {
		return 0, err
	}
	settings, err := e.GetLeverageSettings(ctx, marginMode, fPair)
	if err != nil {
		return 0, err
	}
	market := formatMarketName(fPair)
	for i := range settings {
		if settings[i].Market == market && settings[i].MarginMode == marginMode {
			return settings[i].LeverageCurrent.Float64(), nil
		}
	}
	return 0, fmt.Errorf("%w: no %s leverage setting for %s", errLeverageRequired, marginMode, market)
}

// GetFuturesPositionSummary returns a summary of the open position of a perpetual market
// Isolated positions are preferred over cross margin positions when both are open for the market
func (e *Exchange) GetFuturesPositionSummary(ctx context.Context, r *futures.PositionSummaryRequest) (*futures.PositionSummary, error) {
	if err := common.NilGuard(r); err != nil {
		return nil, err
	}
	if r.Asset != asset.PerpetualContract {
		return nil, fmt.Errorf("%w %q", futures.ErrNotPerpetualFuture, r.Asset)
	}
	if r.Pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	fPair, err := e.FormatExchangeCurrency(r.Pair, r.Asset)
	if err != nil {
		return nil, err
	}
	positions, err := e.GetOpenPositions(ctx, fPair)
	if err != nil {
		return nil, err
	}
	pos, err := selectPosition(positions, formatMarketName(fPair))
	if err != nil {
		return nil, err
	}
	size := pos.Size.Decimal()
	if pos.Side == PositionSideShort {
		size = size.Abs().Neg()
	}
	return &futures.PositionSummary{
		Pair:                         r.Pair,
		Asset:                        r.Asset,
		MarginType:                   marginTypeFromMode(pos.MarginMode),
		Currency:                     pos.SettlementCurrency.Upper(),
		IsolatedMargin:               pos.CollateralReserved.Decimal(),
		NotionalSize:                 pos.CurrentValue.Decimal(),
		Leverage:                     pos.Leverage.Decimal(),
		MaintenanceMarginRequirement: pos.MaintenanceMargin.Decimal(),
		InitialMarginRequirement:     pos.InitialMargin.Decimal(),
		EstimatedLiquidationPrice:    pos.EstimatedLiquidationPrice.Decimal(),
		CollateralUsed:               pos.CurrentMargin.Decimal(),
		MarkPrice:                    pos.MarkPrice.Decimal(),
		CurrentSize:                  size,
		ContractSettlementType:       futures.Linear,
		AverageOpenPrice:             pos.EntryPrice.Decimal(),
		UnrealisedPNL:                pos.PNLUnrealised.Decimal(),
		RealisedPNL:                  pos.PNLRealised.Decimal(),
		MaintenanceMarginFraction:    pos.MaintenanceMarginRatio.Decimal(),
	}, nil
}

// selectPosition returns the open position of a market, preferring isolated margin positions
func selectPosition(positions []PositionResponse, market string) (*PositionResponse, error) {
	var selected *PositionResponse
	for i := range positions {
		if positions[i].Market != market {
			continue
		}
		if selected == nil || positions[i].MarginMode == MarginModeIsolated {
			selected = &positions[i]
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("%w for %s", futures.ErrNoPositionsFound, market)
	}
	return selected, nil
}

// GetFuturesPositionOrders returns the orders of perpetual market positions, aggregated from their trades
func (e *Exchange) GetFuturesPositionOrders(ctx context.Context, r *futures.PositionsRequest) ([]futures.PositionResponse, error) {
	if err := common.NilGuard(r); err != nil {
		return nil, err
	}
	if r.Asset != asset.PerpetualContract {
		return nil, fmt.Errorf("%w %q", futures.ErrNotPerpetualFuture, r.Asset)
	}
	if len(r.Pairs) == 0 {
		return nil, currency.ErrCurrencyPairsEmpty
	}
	if err := common.StartEndTimeCheck(r.StartDate, r.EndDate); err != nil {
		return nil, err
	}
	if earliest := time.Now().Add(-maxHistoryAge + historyAgeBuffer); r.StartDate.Before(earliest) {
		if !r.RespectOrderHistoryLimits {
			return nil, fmt.Errorf("%w max lookup %v", futures.ErrOrderHistoryTooLarge, earliest)
		}
		r.StartDate = earliest
	}
	resp := make([]futures.PositionResponse, len(r.Pairs))
	for i, p := range r.Pairs {
		orders, err := e.derivativesOrderHistory(ctx, currency.Pairs{p}, r.StartDate, r.EndDate)
		if err != nil {
			return nil, err
		}
		resp[i] = futures.PositionResponse{
			Pair:                   p,
			Asset:                  r.Asset,
			ContractSettlementType: futures.Linear,
			Orders:                 orders,
		}
	}
	return resp, nil
}

// ChangePositionMargin sets the collateral of an isolated margin position
func (e *Exchange) ChangePositionMargin(ctx context.Context, r *margin.PositionChangeRequest) (*margin.PositionChangeResponse, error) {
	if err := common.NilGuard(r); err != nil {
		return nil, err
	}
	if r.Asset != asset.PerpetualContract {
		return nil, fmt.Errorf("%w %q", futures.ErrNotPerpetualFuture, r.Asset)
	}
	if r.Pair.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if r.MarginType != margin.Isolated {
		return nil, fmt.Errorf("%w %v", margin.ErrMarginTypeUnsupported, r.MarginType)
	}
	if r.NewAllocatedMargin <= 0 {
		return nil, margin.ErrNewAllocatedMarginRequired
	}
	fPair, err := e.FormatExchangeCurrency(r.Pair, r.Asset)
	if err != nil {
		return nil, err
	}
	positions, err := e.GetOpenPositions(ctx, fPair)
	if err != nil {
		return nil, err
	}
	market := formatMarketName(fPair)
	var positionID string
	for i := range positions {
		if positions[i].Market == market && positions[i].MarginMode == MarginModeIsolated {
			positionID = positions[i].ID
			break
		}
	}
	if positionID == "" {
		return nil, fmt.Errorf("%w for isolated %s", futures.ErrNoPositionsFound, market)
	}
	if err := e.AdjustPositionCollateral(ctx, positionID, r.NewAllocatedMargin); err != nil {
		return nil, err
	}
	return &margin.PositionChangeResponse{
		Exchange:        e.Name,
		Pair:            r.Pair,
		Asset:           r.Asset,
		MarginType:      r.MarginType,
		AllocatedMargin: r.NewAllocatedMargin,
	}, nil
}

// GetCurrencyTradeURL returns the URL to the exchange's trade page for the given asset and currency pair
func (e *Exchange) GetCurrencyTradeURL(_ context.Context, a asset.Item, cp currency.Pair) (string, error) {
	_, err := e.CurrencyPairs.IsPairEnabled(cp, a)
	if err != nil {
		return "", err
	}
	return tradeBaseURL + formatMarketSymbol(cp) + "/", nil
}

// marketTypeForAsset returns the market type of an asset type
func marketTypeForAsset(a asset.Item) (string, error) {
	switch a {
	case asset.Spot:
		return MarketTypeSpot, nil
	case asset.PerpetualContract:
		return MarketTypePerpetual, nil
	default:
		return "", fmt.Errorf("%w %q", asset.ErrNotSupported, a)
	}
}

// marketPairAsset returns the currency pair and asset type of a market name, e.g. BTC/USD or BTC/USD-PERP
func marketPairAsset(market string) (currency.Pair, asset.Item, error) {
	pair, err := currency.NewPairDelimiter(market, currency.ForwardSlashDelimiter)
	if err != nil {
		return currency.EMPTYPAIR, asset.Empty, err
	}
	if strings.HasSuffix(market, perpetualMarketSuffix) {
		return pair, asset.PerpetualContract, nil
	}
	return pair, asset.Spot, nil
}

// formatMarginMode returns the margin mode of a margin type; cross margin is used when the margin type is unset
func formatMarginMode(t margin.Type) (string, error) {
	switch t {
	case margin.Unset, margin.Multi:
		return MarginModeCross, nil
	case margin.Isolated:
		return MarginModeIsolated, nil
	default:
		return "", fmt.Errorf("%w %v", margin.ErrMarginTypeUnsupported, t)
	}
}

// marginTypeFromMode returns the margin type of a margin mode
func marginTypeFromMode(mode string) margin.Type {
	switch mode {
	case MarginModeCross:
		return margin.Multi
	case MarginModeIsolated:
		return margin.Isolated
	default:
		return margin.Unset
	}
}

// formatTrigger returns the trigger of a stop order price type
func formatTrigger(t order.PriceType) string {
	switch t {
	case order.IndexPrice:
		return TriggerIndexPrice
	case order.MarkPrice:
		return TriggerMarkPrice
	default:
		return TriggerLastTradedPrice
	}
}

// orderTypeFromSubtype returns the order type of a derivatives order subtype; orders without a subtype are limit orders
func orderTypeFromSubtype(subtype string) order.Type {
	switch subtype {
	case "", OrderSubtypeLimit:
		return order.Limit
	case OrderSubtypeMarket, OrderSubtypeInstant, OrderSubtypeCash:
		return order.Market
	case OrderSubtypeStopMarket, OrderSubtypeStopLoss:
		return order.StopMarket
	case OrderSubtypeStopLimit, OrderSubtypeStopLossLimit:
		return order.StopLimit
	case OrderSubtypeTakeProfit:
		return order.TakeProfitMarket
	case OrderSubtypeTakeProfitLimit:
		return order.TakeProfit | order.Limit
	case OrderSubtypeTrailingStopLoss, OrderSubtypeTrailingTakeProfit:
		return order.TrailingStop
	case OrderSubtypeTrailingStopLossLimit, OrderSubtypeTrailingTakeProfitLimit:
		return order.TrailingStopLimit
	default:
		return order.UnknownType
	}
}
