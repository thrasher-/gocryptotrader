package bitfinex

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
	// publicWebsocketURL serves the public channels; authenticated connections must use authenticatedWebsocketURL
	publicWebsocketURL        = "wss://api-pub.bitfinex.com/ws/2"
	authenticatedWebsocketURL = "wss://api.bitfinex.com/ws/2"

	// wrapperBookLength is how many orders, or funding offers, a side of the book the wrapper fetches holds
	wrapperBookLength = 100
	// tradesPageLimit is the most trades a page holds
	tradesPageLimit = 10000
	// ordersHistoryPageLimit and movementsPageLimit are the most orders and movements a page of their history holds
	ordersHistoryPageLimit = 2500
	movementsPageLimit     = 2500
	// candlesPageLimit is the most candles a page holds
	candlesPageLimit = 10000
	// fundingCandlePeriod is the funding period, in days, of the funding candles the wrapper fetches, as the default
	// websocket subscription's are
	fundingCandlePeriod = 30
	// testListingMarker marks the pairs and currencies Bitfinex lists for testing, which trade nothing real
	testListingMarker = "TEST"
)

var (
	errTransferMethodUnknown = errors.New("no transfer method for currency")
	errTransferMethodNeeded  = errors.New("transfer chain required, as the currency has several")
	errInvalidClientOrderID  = errors.New("client order ID must be an unsigned integer")
	errUnsupportedOrderType  = errors.New("order type not supported")
	errHistoryPageFull       = errors.New("history has more records from one millisecond than a page holds")
)

// SetDefaults sets the basic defaults for bitfinex
func (e *Exchange) SetDefaults() {
	e.Name = "Bitfinex"
	e.Enabled = true
	e.Verbose = true
	e.API.CredentialsValidator.RequiresKey = true
	e.API.CredentialsValidator.RequiresSecret = true

	for _, a := range []asset.Item{asset.Spot, asset.Margin, asset.MarginFunding} {
		ps := currency.PairStore{
			AssetEnabled:  true,
			RequestFormat: &currency.PairFormat{Uppercase: true},
			ConfigFormat:  &currency.PairFormat{Uppercase: true, Delimiter: currency.DashDelimiter},
		}
		if a == asset.Margin {
			ps.ConfigFormat.Delimiter = ":"
		}
		if err := e.SetAssetPairStore(a, ps); err != nil {
			log.Errorf(log.ExchangeSys, "%s error storing %q default asset formats: %s", e.Name, a, err)
		}
	}

	e.Features = exchange.Features{
		Supports: exchange.FeaturesSupported{
			REST:      true,
			Websocket: true,
			RESTCapabilities: protocol.Features{
				TickerBatching:      true,
				TickerFetching:      true,
				OrderbookFetching:   true,
				AutoPairUpdates:     true,
				AccountInfo:         true,
				CryptoDeposit:       true,
				CryptoWithdrawal:    true,
				FiatWithdraw:        true,
				GetOrder:            true,
				GetOrders:           true,
				CancelOrders:        true,
				CancelOrder:         true,
				SubmitOrder:         true,
				ModifyOrder:         true,
				DepositHistory:      true,
				WithdrawalHistory:   true,
				TradeFetching:       true,
				UserTradeHistory:    true,
				TradeFee:            true,
				CryptoDepositFee:    true,
				CryptoWithdrawalFee: true,
				FiatDepositFee:      true,
				FiatWithdrawalFee:   true,
				KlineFetching:       true,
				// A currency deposited and withdrawn over several methods needs one named
				MultiChainDeposits:                true,
				MultiChainWithdrawals:             true,
				MultiChainDepositRequiresChainSet: true,
			},
			WebsocketCapabilities: protocol.Features{
				AccountBalance:         true,
				CancelOrders:           true,
				CancelOrder:            true,
				SubmitOrder:            true,
				ModifyOrder:            true,
				TickerFetching:         true,
				KlineFetching:          true,
				TradeFetching:          true,
				OrderbookFetching:      true,
				AccountInfo:            true,
				Subscribe:              true,
				Unsubscribe:            true,
				AuthenticatedEndpoints: true,
				MessageCorrelation:     true,
				GetOrders:              true,
				GetOrder:               true,
			},
			WithdrawPermissions: exchange.AutoWithdrawCryptoWithAPIPermission | exchange.AutoWithdrawFiatWithAPIPermission,
			Kline: kline.ExchangeCapabilitiesSupported{
				DateRanges: true,
				Intervals:  true,
			},
		},
		Enabled: exchange.FeaturesEnabled{
			AutoPairUpdates: true,
			Kline: kline.ExchangeCapabilitiesEnabled{
				Intervals: kline.DeployExchangeIntervals(
					kline.IntervalCapacity{Interval: kline.OneMin},
					kline.IntervalCapacity{Interval: kline.FiveMin},
					kline.IntervalCapacity{Interval: kline.FifteenMin},
					kline.IntervalCapacity{Interval: kline.ThirtyMin},
					kline.IntervalCapacity{Interval: kline.OneHour},
					kline.IntervalCapacity{Interval: kline.ThreeHour},
					kline.IntervalCapacity{Interval: kline.SixHour},
					kline.IntervalCapacity{Interval: kline.TwelveHour},
					kline.IntervalCapacity{Interval: kline.OneDay},
					kline.IntervalCapacity{Interval: kline.OneWeek},
					kline.IntervalCapacity{Interval: kline.TwoWeek},
					kline.IntervalCapacity{Interval: kline.OneMonth},
				),
				GlobalResultLimit: candlesPageLimit,
			},
		},
		Subscriptions: defaultSubscriptions.Clone(),
	}

	var err error
	e.Requester, err = request.New(e.Name, common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout), request.WithLimiter(GetRateLimit()))
	if err != nil {
		log.Errorln(log.ExchangeSys, err)
	}
	e.API.Endpoints = e.NewEndpoints()
	if err := e.API.Endpoints.SetDefaultEndpoints(map[exchange.URL]string{
		exchange.RestSpot:         apiURL,
		exchange.WebsocketSpot:    publicWebsocketURL,
		exchange.WebsocketPrivate: authenticatedWebsocketURL,
	}); err != nil {
		log.Errorln(log.ExchangeSys, err)
	}
	e.Websocket = websocket.NewManager()
	e.WebsocketResponseMaxLimit = exchange.DefaultWebsocketResponseMaxLimit
	e.WebsocketResponseCheckTimeout = exchange.DefaultWebsocketResponseCheckTimeout
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
	return e.setupWebsocket(exch)
}

// FetchTradablePairs returns the pairs, or for margin funding the currencies, an asset trades, leaving out the ones
// Bitfinex lists for testing
func (e *Exchange) FetchTradablePairs(ctx context.Context, a asset.Item) (currency.Pairs, error) {
	var symbols []string
	switch a {
	case asset.Spot:
		spot, err := e.GetSpotPairs(ctx)
		if err != nil {
			return nil, err
		}
		// Bitfinex lists securities among its exchange pairs, which only its securities accounts trade
		securities, err := e.GetSecuritiesPairs(ctx)
		if err != nil {
			return nil, err
		}
		symbols = slices.DeleteFunc(spot, func(s string) bool { return slices.Contains(securities, s) })
	case asset.Margin:
		margin, err := e.GetMarginPairs(ctx)
		if err != nil {
			return nil, err
		}
		symbols = margin
	case asset.MarginFunding:
		// The currencies funding trades in are those with funding tickers
		tickers, err := e.GetTickers(ctx, nil)
		if err != nil {
			return nil, err
		}
		for i := range tickers.Funding {
			symbols = append(symbols, tickers.Funding[i].Symbol)
		}
	default:
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	pairs := make(currency.Pairs, 0, len(symbols))
	for _, symbol := range symbols {
		if strings.Contains(symbol, testListingMarker) {
			continue
		}
		p, err := pairFromSymbol(symbol)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

// UpdateTradablePairs updates the exchanges available pairs and stores them in the exchanges config
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

// UpdateOrderExecutionLimits stores the smallest and largest order amounts of an asset's pairs; spot and margin trade
// the same markets, so share them
func (e *Exchange) UpdateOrderExecutionLimits(ctx context.Context, a asset.Item) error {
	switch a {
	case asset.Spot, asset.Margin:
	case asset.MarginFunding:
		// Bootstrap skips this error for every enabled asset, where asset.ErrNotSupported fails the load
		return common.ErrNotYetImplemented
	default:
		return fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	info, err := e.GetPairInfo(ctx)
	if err != nil {
		return err
	}
	l := make([]limits.MinMaxLevel, 0, len(info))
	for i := range info {
		if strings.Contains(info[i].Pair, testListingMarker) {
			continue
		}
		p, err := pairFromSymbol(info[i].Pair)
		if err != nil {
			return err
		}
		l = append(l, limits.MinMaxLevel{
			Key:               key.NewExchangeAssetPair(e.Name, a, p),
			MinimumBaseAmount: info[i].Info.MinimumOrderSize.Float64(),
			MaximumBaseAmount: info[i].Info.MaximumOrderSize.Float64(),
		})
	}
	return limits.Load(l)
}

// UpdateTickers updates the tickers of an asset's enabled pairs from one request for every ticker
func (e *Exchange) UpdateTickers(ctx context.Context, a asset.Item) error {
	switch a {
	case asset.Spot, asset.Margin, asset.MarginFunding:
	default:
		return fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	tickers, err := e.GetTickers(ctx, nil)
	if err != nil {
		return err
	}
	var errs error
	process := func(symbol string, price func(currency.Pair) *ticker.Price) {
		p, err := pairFromSymbol(symbol)
		if err != nil {
			// A symbol GCT cannot pair is not one of the enabled pairs
			return
		}
		if enabled, err := e.IsPairEnabled(p, a); err != nil || !enabled {
			return
		}
		if err := ticker.ProcessTicker(price(p)); err != nil {
			errs = common.AppendError(errs, err)
		}
	}
	if a == asset.MarginFunding {
		for i := range tickers.Funding {
			process(tickers.Funding[i].Symbol, func(p currency.Pair) *ticker.Price {
				return e.fundingTickerPrice(&tickers.Funding[i].Ticker, p, a)
			})
		}
		return errs
	}
	for i := range tickers.Trading {
		process(tickers.Trading[i].Symbol, func(p currency.Pair) *ticker.Price {
			return e.tickerPrice(&tickers.Trading[i].Ticker, p, a)
		})
	}
	return errs
}

// UpdateTicker updates and returns the ticker for a currency pair
func (e *Exchange) UpdateTicker(ctx context.Context, p currency.Pair, a asset.Item) (*ticker.Price, error) {
	if p.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	symbol, err := symbolFor(a, p)
	if err != nil {
		return nil, err
	}
	var price *ticker.Price
	if a == asset.MarginFunding {
		t, err := e.GetFundingTicker(ctx, symbol)
		if err != nil {
			return nil, err
		}
		price = e.fundingTickerPrice(t, p, a)
	} else {
		t, err := e.GetTicker(ctx, symbol)
		if err != nil {
			return nil, err
		}
		price = e.tickerPrice(t, p, a)
	}
	if err := ticker.ProcessTicker(price); err != nil {
		return nil, err
	}
	return ticker.GetTicker(e.Name, p, a)
}

// tickerPrice returns a trading pair's ticker as GCT records it. Bitfinex's REST tickers carry no time; their first
// trade time is not one
func (e *Exchange) tickerPrice(t *Ticker, p currency.Pair, a asset.Item) *ticker.Price {
	return &ticker.Price{
		ExchangeName:        e.Name,
		Pair:                p,
		AssetType:           a,
		Bid:                 t.Bid,
		BidSize:             t.BidSize,
		Ask:                 t.Ask,
		AskSize:             t.AskSize,
		Last:                t.LastPrice,
		BaseVolume:          t.Volume,
		High:                t.High,
		Low:                 t.Low,
		Open24Hour:          t.LastPrice - t.DailyChange,
		PercentChange24Hour: t.DailyChangeRelative * 100,
	}
}

// fundingTickerPrice returns a funding currency's ticker as GCT records it, its rates as prices
func (e *Exchange) fundingTickerPrice(t *FundingTicker, p currency.Pair, a asset.Item) *ticker.Price {
	return &ticker.Price{
		ExchangeName:          e.Name,
		Pair:                  p,
		AssetType:             a,
		FlashReturnRate:       t.FlashReturnRate,
		Bid:                   t.Bid,
		BidPeriod:             float64(t.BidPeriod),
		BidSize:               t.BidSize,
		Ask:                   t.Ask,
		AskPeriod:             float64(t.AskPeriod),
		AskSize:               t.AskSize,
		Last:                  t.LastRate,
		BaseVolume:            t.Volume,
		High:                  t.High,
		Low:                   t.Low,
		FlashReturnRateAmount: t.FlashReturnRateAmountAvailable,
	}
}

// UpdateOrderbook updates and returns a pair's book of orders, or a funding currency's of offers, as the default
// websocket subscription keeps it
func (e *Exchange) UpdateOrderbook(ctx context.Context, p currency.Pair, a asset.Item) (*orderbook.Book, error) {
	if p.IsEmpty() {
		return nil, currency.ErrCurrencyPairEmpty
	}
	if err := e.CurrencyPairs.IsAssetEnabled(a); err != nil {
		return nil, err
	}
	symbol, err := symbolFor(a, p)
	if err != nil {
		return nil, err
	}
	book := &orderbook.Book{
		Exchange:          e.Name,
		Pair:              p,
		Asset:             a,
		PriceDuplication:  true,
		IsFundingRate:     a == asset.MarginFunding,
		ValidateOrderbook: e.ValidateOrderbook,
		LastUpdated:       time.Now(),
	}
	req := &RawBookRequest{Symbol: symbol, Length: wrapperBookLength}
	if a == asset.MarginFunding {
		levels, err := e.GetRawFundingBook(ctx, req)
		if err != nil {
			return nil, err
		}
		for i := range levels {
			addBookLevel(book, &levels[i])
		}
	} else {
		levels, err := e.GetRawBook(ctx, req)
		if err != nil {
			return nil, err
		}
		for i := range levels {
			addBookLevel(book, &levels[i])
		}
	}
	if err := book.Process(); err != nil {
		return nil, err
	}
	return orderbook.Get(e.Name, p, a)
}

// addBookLevel adds an order, offer or level of Bitfinex's to the side of a book holding it
func addBookLevel(book *orderbook.Book, level wsBookLevel) {
	l, bid, _ := level.level()
	if bid {
		book.Bids = append(book.Bids, l)
		return
	}
	book.Asks = append(book.Asks, l)
}

// UpdateAccountBalances stores and returns the balances of the wallet an asset trades from: exchange for spot, margin
// for margin and funding for margin funding
func (e *Exchange) UpdateAccountBalances(ctx context.Context, a asset.Item) (accounts.SubAccounts, error) {
	switch a {
	case asset.Spot, asset.Margin, asset.MarginFunding:
	default:
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
	}
	wallets, err := e.GetWallets(ctx)
	if err != nil {
		return nil, err
	}
	subAcct := accounts.NewSubAccount(a, "")
	for i := range wallets {
		if wa, err := walletAsset(wallets[i].Type); err != nil || wa != a {
			continue
		}
		// Bitfinex sends no available balance until it has calculated it, so none of the balance is free until then
		b := accounts.Balance{Total: wallets[i].Balance, Hold: wallets[i].Balance}
		if available := wallets[i].AvailableBalance; available != nil {
			b.Free = *available
			b.Hold = b.Total - b.Free
		}
		subAcct.Balances.Set(wallets[i].Currency, b)
	}
	subAccts := accounts.SubAccounts{subAcct}
	return subAccts, e.Accounts.Save(ctx, subAccts, true)
}

// GetAccountFundingHistory returns the account's deposits and withdrawals in every currency
func (e *Exchange) GetAccountFundingHistory(ctx context.Context) ([]exchange.FundingHistory, error) {
	movements, err := e.getMovementsHistory(ctx, currency.EMPTYCODE)
	if err != nil {
		return nil, err
	}
	resp := make([]exchange.FundingHistory, len(movements))
	for i := range movements {
		m := &movements[i]
		resp[i] = exchange.FundingHistory{
			ExchangeName:    e.Name,
			Status:          m.Status,
			TransferID:      strconv.FormatUint(m.ID, 10),
			Description:     m.Note,
			Timestamp:       m.StartedAt.Time(),
			Currency:        m.Currency.String(),
			Amount:          math.Abs(m.Amount.Float64()),
			Fee:             math.Abs(m.Fees.Float64()),
			TransferType:    movementType(m),
			CryptoToAddress: m.DestinationAddress,
			CryptoTxID:      m.TransactionID,
			CryptoChain:     movementChain(m),
		}
	}
	return resp, nil
}

// getMovementsHistory returns the account's deposits and withdrawals of a currency, or of every currency when it is
// empty, page by page
func (e *Exchange) getMovementsHistory(ctx context.Context, c currency.Code) ([]Movement, error) {
	return pageBack(time.Time{}, time.Time{}, movementsPageLimit, func(end time.Time) ([]Movement, error) {
		return e.GetMovements(ctx, &MovementsRequest{Currency: c, End: end, Limit: movementsPageLimit})
	}, func(m *Movement) uint64 { return m.ID }, func(m *Movement) time.Time { return m.UpdatedAt.Time() })
}

// movementChain returns the method a movement went over, its chain, in the form GetAvailableTransferChains returns
func movementChain(m *Movement) string {
	return strings.ToLower(m.Method)
}

// movementType returns whether a movement is a deposit, whose amount is positive, or a withdrawal
func movementType(m *Movement) string {
	if m.Amount.Float64() < 0 {
		return "withdrawal"
	}
	return "deposit"
}

// GetWithdrawalsHistory returns the account's withdrawals of a currency, or of every currency when it is empty
func (e *Exchange) GetWithdrawalsHistory(ctx context.Context, c currency.Code, _ asset.Item) ([]exchange.WithdrawalHistory, error) {
	movements, err := e.getMovementsHistory(ctx, bitfinexCurrency(c))
	if err != nil {
		return nil, err
	}
	resp := make([]exchange.WithdrawalHistory, 0, len(movements))
	for i := range movements {
		m := &movements[i]
		if m.Amount.Float64() >= 0 {
			continue
		}
		resp = append(resp, exchange.WithdrawalHistory{
			Status:          m.Status,
			TransferID:      strconv.FormatUint(m.ID, 10),
			Description:     m.Note,
			Timestamp:       m.StartedAt.Time(),
			Currency:        m.Currency.String(),
			Amount:          -m.Amount.Float64(),
			Fee:             math.Abs(m.Fees.Float64()),
			TransferType:    movementType(m),
			CryptoToAddress: m.DestinationAddress,
			CryptoTxID:      m.TransactionID,
			CryptoChain:     movementChain(m),
		})
	}
	return resp, nil
}

// GetRecentTrades returns a pair's, or a funding currency's, most recent trades
func (e *Exchange) GetRecentTrades(ctx context.Context, p currency.Pair, a asset.Item) ([]trade.Data, error) {
	resp, err := e.getTrades(ctx, p, a, &PublicTradesRequest{Limit: tradesPageLimit})
	if err != nil {
		return nil, err
	}
	if err := e.AddTradesToBuffer(resp...); err != nil {
		return nil, err
	}
	trade.SortByDate(resp)
	return resp, nil
}

// GetHistoricTrades returns the trades of a pair, or of a funding currency, between two times, paging back from the
// end
func (e *Exchange) GetHistoricTrades(ctx context.Context, p currency.Pair, a asset.Item, start, end time.Time) ([]trade.Data, error) {
	if err := common.StartEndTimeCheck(start, end); err != nil {
		return nil, err
	}
	resp, err := pageBack(start, end, tradesPageLimit, func(pageEnd time.Time) ([]trade.Data, error) {
		return e.getTrades(ctx, p, a, &PublicTradesRequest{Start: start, End: pageEnd, Limit: tradesPageLimit, Sort: -1})
	}, func(t *trade.Data) string { return t.TID }, func(t *trade.Data) time.Time { return t.Timestamp })
	if err != nil {
		return nil, err
	}
	if err := e.AddTradesToBuffer(resp...); err != nil {
		return nil, err
	}
	trade.SortByDate(resp)
	return trade.FilterTradesByTime(resp, start, end), nil
}

// pageBack returns a history Bitfinex sends newest first, a page of at most limit records at a time, paging back from
// end to start. Each page ends at its predecessor's oldest record, which both hold and which is kept once. A full page
// of records all from the millisecond it ends at errors, as no page can reach any more from that millisecond, and so
// does a server ignoring the end
func pageBack[T any, K comparable](start, end time.Time, limit int, fetch func(end time.Time) ([]T, error), id func(*T) K, at func(*T) time.Time) ([]T, error) {
	var resp []T
	seen := make(map[K]struct{})
	for {
		page, err := fetch(end)
		if err != nil {
			return nil, err
		}
		oldest := end
		for i := range page {
			if t := at(&page[i]); oldest.IsZero() || t.Before(oldest) {
				oldest = t
			}
			k := id(&page[i])
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			resp = append(resp, page[i])
		}
		if len(page) < limit {
			return resp, nil
		}
		if !end.IsZero() && !oldest.Before(end) {
			return nil, fmt.Errorf("%w: %d or more from %s", errHistoryPageFull, limit, end.UTC().Format(time.RFC3339Nano))
		}
		if !start.IsZero() && oldest.Before(start) {
			return resp, nil
		}
		end = oldest
	}
}

// getTrades returns a page of a pair's, or a funding currency's, trades
func (e *Exchange) getTrades(ctx context.Context, p currency.Pair, a asset.Item, req *PublicTradesRequest) ([]trade.Data, error) {
	symbol, err := symbolFor(a, p)
	if err != nil {
		return nil, err
	}
	req.Symbol = symbol
	if a == asset.MarginFunding {
		trades, err := e.GetPublicFundingTrades(ctx, req)
		if err != nil {
			return nil, err
		}
		resp := make([]trade.Data, len(trades))
		for i := range trades {
			resp[i] = e.fundingTradeData(p, a, &trades[i])
		}
		return resp, nil
	}
	trades, err := e.GetPublicTrades(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := make([]trade.Data, len(trades))
	for i := range trades {
		resp[i] = e.tradeData(p, a, &trades[i])
	}
	return resp, nil
}

// SubmitOrder places an order through the authenticated websocket, or over REST without it
func (e *Exchange) SubmitOrder(ctx context.Context, s *order.Submit) (*order.SubmitResponse, error) {
	if err := s.Validate(e.GetTradingRequirements()); err != nil {
		return nil, err
	}
	req, err := placeOrderRequest(s)
	if err != nil {
		return nil, err
	}
	var placed *Order
	if e.Websocket.CanUseAuthenticatedWebsocketForWrapper() {
		if placed, err = e.WebsocketPlaceOrder(ctx, req); err != nil {
			return nil, err
		}
	} else {
		n, err := e.PlaceOrder(ctx, req)
		if err != nil {
			return nil, err
		}
		if len(n.Data) == 0 {
			return nil, common.ErrNoResponse
		}
		placed = &n.Data[0]
	}
	resp, err := s.DeriveSubmitResponse(strconv.FormatUint(placed.ID, 10))
	if err != nil {
		return nil, err
	}
	if status := orderStatusFromBitfinex(placed.Status); status != order.UnknownStatus {
		resp.Status = status
	}
	return resp, nil
}

// placeOrderRequest returns the request placing an order: an EXCHANGE order for spot, from the exchange wallet, or a
// margin order from the margin wallet, its amount signed by its side
func placeOrderRequest(s *order.Submit) (*PlaceOrderRequest, error) {
	if s.AssetType != asset.Spot && s.AssetType != asset.Margin {
		return nil, fmt.Errorf("%w: %s", asset.ErrNotSupported, s.AssetType)
	}
	symbol, err := orderSymbol(s.AssetType, s.Pair)
	if err != nil {
		return nil, err
	}
	req := &PlaceOrderRequest{Symbol: symbol, Amount: s.Amount, Price: s.Price}
	if s.Side.IsShort() {
		req.Amount = -s.Amount
	}
	switch s.Type {
	case order.Limit:
		req.Type = OrderTypeLimit
		switch {
		case s.TimeInForce.Is(order.FillOrKill):
			req.Type = OrderTypeFillOrKill
		case s.TimeInForce.Is(order.ImmediateOrCancel):
			req.Type = OrderTypeImmediateOrCancel
		}
	case order.Market:
		req.Type, req.Price = OrderTypeMarket, 0
	case order.Stop:
		req.Type, req.Price = OrderTypeStop, s.TriggerPrice
	case order.StopLimit:
		req.Type, req.Price, req.AuxiliaryLimitPrice = OrderTypeStopLimit, s.TriggerPrice, s.Price
	case order.TrailingStop:
		req.Type, req.Price, req.TrailingPrice = OrderTypeTrailingStop, 0, s.TrackingValue
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedOrderType, s.Type)
	}
	if s.AssetType == asset.Spot {
		req.Type = ExchangeOrderTypePrefix + req.Type
	}
	if s.TimeInForce.Is(order.PostOnly) {
		req.Flags |= OrderFlagPostOnly
	}
	if s.ReduceOnly {
		req.Flags |= OrderFlagReduceOnly
	}
	if s.TimeInForce.Is(order.GoodTillTime) {
		req.TimeInForce = s.EndTime
	}
	if s.ClientOrderID != "" {
		if req.ClientOrderID, err = strconv.ParseUint(s.ClientOrderID, 10, 64); err != nil {
			return nil, fmt.Errorf("%w: %q", errInvalidClientOrderID, s.ClientOrderID)
		}
	}
	return req, nil
}

// ModifyOrder updates an order's amount and price through the authenticated websocket, or over REST without it
func (e *Exchange) ModifyOrder(ctx context.Context, m *order.Modify) (*order.ModifyResponse, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	id, err := parseOrderID(m.OrderID)
	if err != nil {
		return nil, err
	}
	req := &UpdateOrderRequest{ID: id, Amount: m.Amount, Price: m.Price}
	// Bitfinex signs an order's amount by its side, buying positive and selling negative
	switch {
	case m.Amount == 0:
	case m.Side.IsShort():
		req.Amount = -m.Amount
	case !m.Side.IsLong():
		return nil, fmt.Errorf("%w: %s, and an order's amount is signed by its side", order.ErrSideIsInvalid, m.Side)
	}
	if e.Websocket.CanUseAuthenticatedWebsocketForWrapper() {
		if _, err := e.WebsocketUpdateOrder(ctx, req); err != nil {
			return nil, err
		}
	} else if _, err := e.UpdateOrder(ctx, req); err != nil {
		return nil, err
	}
	return m.DeriveModifyResponse()
}

// CancelOrder submits an order for cancellation through the authenticated websocket, or over REST without it
func (e *Exchange) CancelOrder(ctx context.Context, c *order.Cancel) error {
	if err := c.Validate(c.StandardCancel()); err != nil {
		return err
	}
	id, err := parseOrderID(c.OrderID)
	if err != nil {
		return err
	}
	if e.Websocket.CanUseAuthenticatedWebsocketForWrapper() {
		_, err = e.WebsocketCancelExistingOrder(ctx, id)
		return err
	}
	_, err = e.CancelExistingOrder(ctx, &CancelExistingOrderRequest{ID: id})
	return err
}

// parseOrderID returns an order ID as Bitfinex numbers it
func parseOrderID(orderID string) (uint64, error) {
	id, err := strconv.ParseUint(orderID, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("%w: %q", errOrderIDRequired, orderID)
	}
	return id, nil
}

// CancelBatchOrders submits orders for cancellation in one request, reporting the orders Bitfinex accepted for
// cancellation as cancelled
func (e *Exchange) CancelBatchOrders(ctx context.Context, cancels []order.Cancel) (*order.CancelBatchResponse, error) {
	if len(cancels) == 0 {
		return nil, order.ErrCancelOrderIsNil
	}
	ids := make([]uint64, len(cancels))
	for i := range cancels {
		id, err := parseOrderID(cancels[i].OrderID)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	statuses, err := e.cancelOrdersByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	return &order.CancelBatchResponse{Status: statuses}, nil
}

// CancelAllOrders submits the account's open orders of an asset, on a pair when one is given, for cancellation
func (e *Exchange) CancelAllOrders(ctx context.Context, c *order.Cancel) (order.CancelAllResponse, error) {
	if err := common.NilGuard(c); err != nil {
		return order.CancelAllResponse{}, err
	}
	open, err := e.openOrderDetails(ctx, c.Pair, c.AssetType)
	if err != nil {
		return order.CancelAllResponse{}, err
	}
	if len(open) == 0 {
		return order.CancelAllResponse{Status: map[string]string{}}, nil
	}
	ids := make([]uint64, len(open))
	for i := range open {
		if ids[i], err = parseOrderID(open[i].OrderID); err != nil {
			return order.CancelAllResponse{}, err
		}
	}
	statuses, err := e.cancelOrdersByID(ctx, ids)
	return order.CancelAllResponse{Status: statuses}, err
}

// cancelOrdersByID submits orders for cancellation, through the authenticated websocket or over REST, reporting each
// order's status: over REST, cancelled for the orders Bitfinex reports cancelled; over the websocket, pending
// cancellation, as its notification names none of the orders and each order's close event confirms its cancellation
func (e *Exchange) cancelOrdersByID(ctx context.Context, ids []uint64) (map[string]string, error) {
	statuses := make(map[string]string, len(ids))
	if e.Websocket.CanUseAuthenticatedWebsocketForWrapper() {
		if err := e.WebsocketCancelOrders(ctx, &CancelOrdersRequest{IDs: ids}); err != nil {
			return nil, err
		}
		for _, id := range ids {
			statuses[strconv.FormatUint(id, 10)] = order.PendingCancel.String()
		}
		return statuses, nil
	}
	n, err := e.CancelOrders(ctx, &CancelOrdersRequest{IDs: ids})
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		statuses[strconv.FormatUint(id, 10)] = order.UnknownStatus.String()
	}
	for i := range n.Data {
		statuses[strconv.FormatUint(n.Data[i].ID, 10)] = order.Cancelled.String()
	}
	return statuses, nil
}

// openOrderDetails returns the account's open orders of an asset, on a pair when one is given
func (e *Exchange) openOrderDetails(ctx context.Context, p currency.Pair, a asset.Item) ([]order.Detail, error) {
	var orders []Order
	var err error
	if p.IsEmpty() {
		orders, err = e.GetOpenOrders(ctx, &OpenOrdersRequest{})
	} else {
		var symbol string
		if symbol, err = orderSymbol(a, p); err != nil {
			return nil, err
		}
		orders, err = e.GetOpenOrdersBySymbol(ctx, &OpenOrdersBySymbolRequest{Symbol: symbol})
	}
	if err != nil {
		return nil, err
	}
	return e.orderDetails(orders, a)
}

// orderDetails returns orders as GCT records them, leaving out those of other assets when an asset is given
func (e *Exchange) orderDetails(orders []Order, a asset.Item) ([]order.Detail, error) {
	details := make([]order.Detail, 0, len(orders))
	for i := range orders {
		d, err := e.orderDetail(&orders[i])
		if err != nil {
			return nil, fmt.Errorf("order %d: %w", orders[i].ID, err)
		}
		if a.IsValid() && d.AssetType != a {
			continue
		}
		details = append(details, *d)
	}
	return details, nil
}

// GetOrderInfo returns an order, open or from the last two weeks' history, with the trades it made
func (e *Exchange) GetOrderInfo(ctx context.Context, orderID string, p currency.Pair, a asset.Item) (*order.Detail, error) {
	id, err := parseOrderID(orderID)
	if err != nil {
		return nil, err
	}
	orders, err := e.GetOpenOrders(ctx, &OpenOrdersRequest{IDs: []uint64{id}})
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		if orders, err = e.GetOrdersHistory(ctx, &OrdersHistoryRequest{IDs: []uint64{id}}); err != nil {
			return nil, err
		}
	}
	i := slices.IndexFunc(orders, func(o Order) bool { return o.ID == id })
	if i == -1 {
		return nil, fmt.Errorf("%w: %s", order.ErrOrderNotFound, orderID)
	}
	d, err := e.orderDetail(&orders[i])
	if err != nil {
		return nil, err
	}
	if (!p.IsEmpty() && !d.Pair.Equal(p)) || (a.IsValid() && d.AssetType != a) {
		return nil, fmt.Errorf("%w: %s is a %s %s order", order.ErrOrderNotFound, orderID, d.AssetType, d.Pair)
	}
	trades, err := e.GetOrderTrades(ctx, &OrderTradesRequest{Symbol: orders[i].Symbol, OrderID: id})
	if err != nil {
		return nil, err
	}
	for j := range trades {
		t := &trades[j]
		side, err := orderSideFromAmount(t.ExecutedAmount)
		if err != nil {
			return nil, err
		}
		// Bitfinex sends fees as negative, as charged
		d.Trades = append(d.Trades, order.TradeHistory{
			Price:     t.ExecutedPrice,
			Amount:    math.Abs(t.ExecutedAmount),
			Fee:       -t.Fee,
			FeeAsset:  t.FeeCurrency.String(),
			Exchange:  e.Name,
			TID:       strconv.FormatUint(t.ID, 10),
			Type:      d.Type,
			Side:      side,
			Timestamp: t.ExecutedAt.Time(),
			IsMaker:   t.Maker == 1,
		})
		d.Fee -= t.Fee
	}
	return d, nil
}

// GetActiveOrders returns the account's open orders of an asset, on the request's pairs, or every pair when it names
// none
func (e *Exchange) GetActiveOrders(ctx context.Context, req *order.MultiOrderRequest) (order.FilteredOrders, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	orders, err := e.GetOpenOrders(ctx, &OpenOrdersRequest{})
	if err != nil {
		return nil, err
	}
	details, err := e.orderDetails(orders, req.AssetType)
	if err != nil {
		return nil, err
	}
	return req.Filter(e.Name, details), nil
}

// GetOrderHistory returns the account's closed and cancelled orders from up to two weeks ago, of an asset, on the
// request's pairs, or every pair when it names none
func (e *Exchange) GetOrderHistory(ctx context.Context, req *order.MultiOrderRequest) (order.FilteredOrders, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	orderID := func(o *Order) uint64 { return o.ID }
	createdAt := func(o *Order) time.Time { return o.CreatedAt.Time() }
	var orders []Order
	if len(req.Pairs) == 0 {
		var err error
		orders, err = pageBack(req.StartTime, req.EndTime, ordersHistoryPageLimit, func(end time.Time) ([]Order, error) {
			return e.GetOrdersHistory(ctx, &OrdersHistoryRequest{Start: req.StartTime, End: end, Limit: ordersHistoryPageLimit})
		}, orderID, createdAt)
		if err != nil {
			return nil, err
		}
	}
	for _, p := range req.Pairs {
		symbol, err := orderSymbol(req.AssetType, p)
		if err != nil {
			return nil, err
		}
		pairOrders, err := pageBack(req.StartTime, req.EndTime, ordersHistoryPageLimit, func(end time.Time) ([]Order, error) {
			return e.GetOrdersHistoryBySymbol(ctx, &OrdersHistoryBySymbolRequest{Symbol: symbol, Start: req.StartTime, End: end, Limit: ordersHistoryPageLimit})
		}, orderID, createdAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, pairOrders...)
	}
	details, err := e.orderDetails(orders, req.AssetType)
	if err != nil {
		return nil, err
	}
	return req.Filter(e.Name, details), nil
}

// GetDepositAddress returns an address depositing a currency, over a chain when it has several, into a wallet: the
// account ID names exchange, margin or funding, and funding takes deposits when it names none
func (e *Exchange) GetDepositAddress(ctx context.Context, c currency.Code, accountID, chain string) (*deposit.Address, error) {
	if accountID == "" {
		accountID = WalletTypeFunding
	}
	method, err := e.transferMethod(ctx, c, chain)
	if err != nil {
		return nil, err
	}
	n, err := e.RequestDepositAddress(ctx, &DepositAddressRequest{Wallet: accountID, Method: method})
	if err != nil {
		return nil, err
	}
	// A currency needing a tag, memo or payment ID has its address in the pool address, and the tag in its place
	if n.Data.PoolAddress != "" {
		return &deposit.Address{Address: n.Data.PoolAddress, Tag: n.Data.Address, Chain: method}, nil
	}
	return &deposit.Address{Address: n.Data.Address, Chain: method}, nil
}

// WithdrawCryptocurrencyFunds withdraws a cryptocurrency, over a chain when it has several, from the exchange wallet,
// or the wallet the request's wallet ID names
func (e *Exchange) WithdrawCryptocurrencyFunds(ctx context.Context, w *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	if err := w.Validate(); err != nil {
		return nil, err
	}
	method, err := e.transferMethod(ctx, w.Currency, w.Crypto.Chain)
	if err != nil {
		return nil, err
	}
	wallet := w.WalletID
	if wallet == "" {
		wallet = WalletTypeExchange
	}
	travel, err := withdrawalTravelRule(&w.Travel)
	if err != nil {
		return nil, err
	}
	req := &WithdrawalRequest{Wallet: wallet, Method: method, Amount: w.Amount, Address: w.Crypto.Address, PaymentID: w.Crypto.AddressTag, Note: w.Description, TravelRule: travel}
	n, err := e.RequestWithdrawal(ctx, req)
	if err != nil {
		return nil, err
	}
	return &withdraw.ExchangeResponse{ID: strconv.FormatUint(n.Data.ID, 10), Status: n.Status}, nil
}

// withdrawalTravelRule returns the travel rule details of a withdrawal, or none when it names no beneficiary and no
// provider: the account itself as the beneficiary, or the person it names, whose name Bitfinex takes as first names
// and a last name, split at its last space, and the provider the withdrawal goes to
func withdrawalTravelRule(t *withdraw.TravelRule) (*TravelRuleDetails, error) {
	name := strings.Fields(t.BeneficiaryName)
	if !t.IsSelf && len(name) == 0 && t.BeneficiaryFinancialInstitution == "" {
		return nil, nil
	}
	details := &TravelRuleDetails{BeneficiarySelf: t.IsSelf, VASPName: t.BeneficiaryFinancialInstitution}
	// The account's verification data names it, so a name given with it is not sent
	if t.IsSelf || len(name) == 0 {
		return details, nil
	}
	if len(name) == 1 {
		return nil, fmt.Errorf("%w: %q", errTravelRuleNameIncomplete, t.BeneficiaryName)
	}
	details.FirstName, details.LastName = strings.Join(name[:len(name)-1], " "), name[len(name)-1]
	return details, nil
}

// WithdrawFiatFunds withdraws fiat by wire from the exchange wallet, or the wallet the request's wallet ID names,
// through Bitfinex's v1 API, as v2 has no equivalent
func (e *Exchange) WithdrawFiatFunds(ctx context.Context, w *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	if err := w.Validate(); err != nil {
		return nil, err
	}
	wallet := w.WalletID
	if wallet == "" {
		wallet = WalletTypeExchange
	}
	resp, err := e.RequestFiatWithdrawal(ctx, &FiatWithdrawalRequest{Wallet: wallet, Withdrawal: w})
	if err != nil {
		return nil, err
	}
	return &withdraw.ExchangeResponse{ID: strconv.FormatUint(resp.WithdrawalID, 10), Status: resp.Status}, nil
}

// WithdrawFiatFundsToInternationalBank withdraws fiat by wire, as WithdrawFiatFunds does
func (e *Exchange) WithdrawFiatFundsToInternationalBank(ctx context.Context, w *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	return e.WithdrawFiatFunds(ctx, w)
}

// GetFeeByType returns an estimate of a fee: trading fees from the account's summary, and withdrawal fees from
// Bitfinex's configuration
func (e *Exchange) GetFeeByType(ctx context.Context, f *exchange.FeeBuilder) (float64, error) {
	if err := common.NilGuard(f); err != nil {
		return 0, err
	}
	if !e.AreCredentialsValid(ctx) && f.FeeType == exchange.CryptocurrencyTradeFee {
		f.FeeType = exchange.OfflineTradeFee
	}
	var fee float64
	switch f.FeeType {
	case exchange.CryptocurrencyTradeFee:
		summary, err := e.GetAccountSummary(ctx)
		if err != nil {
			return 0, err
		}
		fee = tradingFeeRate(&summary.FeeRates, f.Pair, f.IsMaker) * f.PurchasePrice * f.Amount
	case exchange.CryptocurrencyWithdrawalFee:
		fees, err := e.GetCurrencyWithdrawalFees(ctx)
		if err != nil {
			return 0, err
		}
		c := bitfinexCurrency(f.Pair.Base)
		i := slices.IndexFunc(fees, func(w CurrencyWithdrawalFee) bool { return w.Currency.Equal(c) })
		if i == -1 {
			return 0, fmt.Errorf("%w: no withdrawal fee for %s", currency.ErrCurrencyNotSupported, c)
		}
		fee = fees[i].Fee.Amount
	case exchange.InternationalBankDepositFee:
		// Bitfinex charges 0.1% of a wire deposit, and at least 60 dollars or euros
		fee = max(0.001*f.Amount, wireFeeMinimum(f.FiatCurrency, 60))
	case exchange.InternationalBankWithdrawalFee:
		// Bitfinex charges 0.1% of a wire withdrawal, and at least 100 dollars or euros
		fee = max(0.001*f.Amount, wireFeeMinimum(f.FiatCurrency, 100))
	case exchange.OfflineTradeFee:
		// Bitfinex charges no trading fees
	}
	// A maker rebate is no fee
	return max(fee, 0), nil
}

// wireFeeMinimum returns the least Bitfinex charges for a wire in a currency, which it sets for dollars and euros
func wireFeeMinimum(c currency.Code, minimum float64) float64 {
	if c.Equal(currency.USD) || c.Equal(currency.EUR) {
		return minimum
	}
	return 0
}

// tradingFeeRate returns the account's fee rate trading a pair, which Bitfinex sets by what the pair is quoted in:
// fiat, a stablecoin or another cryptocurrency
func tradingFeeRate(rates *SummaryFeeRates, p currency.Pair, maker bool) float64 {
	quote := bitfinexCurrency(p.Quote)
	switch {
	case maker && quote.IsFiatCurrency():
		return rates.Maker.ToFiat
	case maker && (quote.IsStableCurrency() || quote.Equal(currency.UST)):
		return rates.Maker.ToStable
	case maker:
		return rates.Maker.ToCrypto
	case quote.IsFiatCurrency():
		return rates.Taker.ToFiat
	case quote.IsStableCurrency() || quote.Equal(currency.UST):
		return rates.Taker.ToStable
	}
	return rates.Taker.ToCrypto
}

// bitfinexCurrency returns the code Bitfinex names a currency by: UST for Tether's USDT
func bitfinexCurrency(c currency.Code) currency.Code {
	if c.Equal(currency.USDT) {
		return currency.UST
	}
	return c
}

// transferMethod returns the method a currency is deposited and withdrawn over: the chain given, which may be
// required when the currency has several
func (e *Exchange) transferMethod(ctx context.Context, c currency.Code, chain string) (string, error) {
	methods, err := e.GetAvailableTransferChains(ctx, c)
	if err != nil {
		return "", err
	}
	switch {
	case chain != "":
		if i := slices.IndexFunc(methods, func(m string) bool { return strings.EqualFold(m, chain) }); i != -1 {
			return methods[i], nil
		}
		return "", fmt.Errorf("%w %s over %q, which has %s", errTransferMethodUnknown, c, chain, methods)
	case len(methods) > 1:
		return "", fmt.Errorf("%w: %s has %s", errTransferMethodNeeded, c, methods)
	}
	return methods[0], nil
}

// GetAvailableTransferChains returns the methods a currency is deposited and withdrawn over, in effect its chains, such
// as bitcoin, or tetheruse and tetherusx for USDT on Ethereum and Tron
func (e *Exchange) GetAvailableTransferChains(ctx context.Context, c currency.Code) ([]string, error) {
	if c.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	methods, err := e.GetTransferMethods(ctx)
	if err != nil {
		return nil, err
	}
	c = bitfinexCurrency(c)
	var chains []string
	for i := range methods {
		if slices.ContainsFunc(methods[i].Currencies, c.Equal) {
			chains = append(chains, strings.ToLower(methods[i].Method))
		}
	}
	if len(chains) == 0 {
		return nil, fmt.Errorf("%w %s", errTransferMethodUnknown, c)
	}
	return chains, nil
}

// AuthenticateWebsocket authenticates the account connection
func (e *Exchange) AuthenticateWebsocket(ctx context.Context) error {
	conn, err := e.Websocket.GetConnection(wsAccountConnection)
	if err != nil {
		return err
	}
	return e.wsSendAuth(ctx, conn)
}

// ValidateAPICredentials validates current credentials used for wrapper functionality
func (e *Exchange) ValidateAPICredentials(ctx context.Context, a asset.Item) error {
	_, err := e.UpdateAccountBalances(ctx, a)
	return e.CheckTransientError(err)
}

// GetHistoricCandles returns candles between a time period for a set time interval
func (e *Exchange) GetHistoricCandles(ctx context.Context, p currency.Pair, a asset.Item, interval kline.Interval, start, end time.Time) (*kline.Item, error) {
	req, err := e.GetKlineRequest(p, a, interval, start, end, false)
	if err != nil {
		return nil, err
	}
	candles, err := e.getCandles(ctx, req.Pair, req.Asset, req.ExchangeInterval, req.Start, req.End, req.RequestLimit)
	if err != nil {
		return nil, err
	}
	return req.ProcessResponse(candles)
}

// GetHistoricCandlesExtended returns candles between a time period for a set time interval, over several requests
func (e *Exchange) GetHistoricCandlesExtended(ctx context.Context, p currency.Pair, a asset.Item, interval kline.Interval, start, end time.Time) (*kline.Item, error) {
	req, err := e.GetKlineExtendedRequest(p, a, interval, start, end)
	if err != nil {
		return nil, err
	}
	timeSeries := make([]kline.Candle, 0, req.Size())
	for i := range req.RangeHolder.Ranges {
		candles, err := e.getCandles(ctx, req.Pair, req.Asset, req.ExchangeInterval, req.RangeHolder.Ranges[i].Start.Time, req.RangeHolder.Ranges[i].End.Time, req.RequestLimit)
		if err != nil {
			return nil, err
		}
		timeSeries = append(timeSeries, candles...)
	}
	return req.ProcessResponse(timeSeries)
}

// getCandles returns a pair's, or a funding currency's, candles oldest first
func (e *Exchange) getCandles(ctx context.Context, p currency.Pair, a asset.Item, interval kline.Interval, start, end time.Time, limit uint64) ([]kline.Candle, error) {
	symbol, err := symbolFor(a, p)
	if err != nil {
		return nil, err
	}
	req := &CandlesHistoryRequest{Symbol: symbol, Interval: interval, Start: start, End: end, Limit: limit, Sort: 1}
	if a == asset.MarginFunding {
		req.Period = fundingCandlePeriod
	}
	candles, err := e.GetCandlesHistory(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := make([]kline.Candle, len(candles))
	for i := range candles {
		resp[i] = kline.Candle{
			Time:   candles[i].Time.Time(),
			Open:   candles[i].Open,
			High:   candles[i].High,
			Low:    candles[i].Low,
			Close:  candles[i].Close,
			Volume: candles[i].Volume,
		}
	}
	return resp, nil
}

// GetServerTime returns the current exchange server time.
func (e *Exchange) GetServerTime(context.Context, asset.Item) (time.Time, error) {
	return time.Time{}, common.ErrFunctionNotSupported
}

// GetFuturesContractDetails returns all contracts from the exchange by asset type
func (e *Exchange) GetFuturesContractDetails(context.Context, asset.Item) ([]futures.Contract, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetLatestFundingRates returns the latest funding rates data
func (e *Exchange) GetLatestFundingRates(context.Context, *fundingrate.LatestRateRequest) ([]fundingrate.LatestRateResponse, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetOpenInterest returns the open interest rate for a given asset pair
func (e *Exchange) GetOpenInterest(context.Context, ...key.PairAsset) ([]futures.OpenInterest, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetCurrencyTradeURL returns the URL to the exchange's trade page for the given asset and currency pair
func (e *Exchange) GetCurrencyTradeURL(_ context.Context, a asset.Item, p currency.Pair) (string, error) {
	if _, err := e.CurrencyPairs.IsPairEnabled(p, a); err != nil {
		return "", err
	}
	base, quote := p.Base.Upper().String(), p.Quote.Upper().String()
	switch a {
	case asset.Spot:
		return tradeBaseURL + "/t/" + base + ":" + quote + "?type=exchange", nil
	case asset.Margin:
		return tradeBaseURL + "/t/" + base + ":" + quote + "?type=margin", nil
	case asset.MarginFunding:
		return tradeBaseURL + "/f/" + base, nil
	}
	return "", fmt.Errorf("%w: %s", asset.ErrNotSupported, a)
}
