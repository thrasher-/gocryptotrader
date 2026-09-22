package binance

import (
	"context"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
)

type derivativeBookState struct {
	lastUpdateID int64
	synchronised bool
}

func isPartialDepthStream(stream string) bool {
	_, channel, _ := strings.Cut(stream, "@")
	return strings.HasPrefix(channel, "depth") && len(channel) > len("depth") && channel[len("depth")] >= '0' && channel[len("depth")] <= '9'
}

// Futures and options diffs must be joined to a REST snapshot using U/u and then
// chained with pu. A first diff alone cannot represent the unchanged book levels.
func (e *Exchange) processDerivativeDepth(ctx context.Context, data []byte, assetType asset.Item, partial bool) error {
	var update FuturesDepthOrderbook
	if err := json.Unmarshal(data, &update); err != nil {
		return err
	}
	assetType = futuresStreamAsset(update.SymbolType, assetType)
	pair, err := e.derivativePair(update.Symbol, assetType)
	if err != nil {
		return err
	}
	e.derivativeBooksMu.Lock()
	defer e.derivativeBooksMu.Unlock()
	if e.derivativeBooks == nil {
		e.derivativeBooks = make(map[key.PairAsset]*derivativeBookState)
	}
	bookKey := key.PairAsset{Base: pair.Base.Item, Quote: pair.Quote.Item, Asset: assetType}
	if partial {
		delete(e.derivativeBooks, bookKey)
		return e.Websocket.Orderbook.LoadSnapshot(&orderbook.Book{Exchange: e.Name, Pair: pair, Asset: assetType, Bids: update.Bids.Levels(), Asks: update.Asks.Levels(), LastUpdateID: update.LastUpdateID, LastUpdated: update.TransactionTime.Time()})
	}
	state := e.derivativeBooks[bookKey]
	if state != nil && update.LastUpdateID <= state.lastUpdateID {
		return nil
	}
	if state != nil && state.synchronised && update.FinalUpdateIDLastStream != state.lastUpdateID {
		if err := e.Websocket.Orderbook.InvalidateOrderbook(pair, assetType); err != nil {
			return err
		}
		delete(e.derivativeBooks, bookKey)
		state = nil
	}
	if state == nil {
		snapshot, err := e.derivativeSnapshot(ctx, pair, assetType)
		if err != nil {
			return err
		}
		if err := e.Websocket.Orderbook.LoadSnapshot(snapshot); err != nil {
			return err
		}
		state = &derivativeBookState{lastUpdateID: snapshot.LastUpdateID}
		e.derivativeBooks[bookKey] = state
	}
	if update.LastUpdateID < state.lastUpdateID {
		return nil
	}
	if !state.synchronised && update.FirstUpdateID > state.lastUpdateID && update.FinalUpdateIDLastStream != state.lastUpdateID {
		// The snapshot was older than the buffered diff. Discard it so the next
		// message obtains a fresh snapshot instead of publishing a book with a gap.
		delete(e.derivativeBooks, bookKey)
		return e.Websocket.Orderbook.InvalidateOrderbook(pair, assetType)
	}
	if err := e.Websocket.Orderbook.Update(&orderbook.Update{Pair: pair, Asset: assetType, UpdateID: update.LastUpdateID, UpdateTime: update.TransactionTime.Time(), Bids: update.Bids.Levels(), Asks: update.Asks.Levels(), Action: orderbook.UpdateAction}); err != nil {
		delete(e.derivativeBooks, bookKey)
		return err
	}
	state.lastUpdateID = update.LastUpdateID
	state.synchronised = true
	return nil
}

func (e *Exchange) derivativeSnapshot(ctx context.Context, pair currency.Pair, assetType asset.Item) (*orderbook.Book, error) {
	snapshot := &orderbook.Book{Exchange: e.Name, Pair: pair, Asset: assetType}
	symbol, err := e.FormatExchangeCurrency(pair, assetType)
	if err != nil {
		return nil, err
	}
	switch assetType {
	case asset.CoinMarginedFutures, asset.USDTMarginedFutures:
		var response *OrderBook
		var err error
		if assetType == asset.CoinMarginedFutures {
			response, err = e.GetFuturesOrderbook(ctx, symbol, 1000)
		} else {
			response, err = e.UFuturesOrderbook(ctx, symbol, 1000)
		}
		if err != nil {
			return nil, err
		}
		snapshot.Bids = response.Bids.Levels()
		snapshot.Asks = response.Asks.Levels()
		snapshot.LastUpdateID = response.LastUpdateID
		snapshot.LastUpdated = response.TransactionTime.Time()
	case asset.Options:
		response, err := e.GetEOptionsOrderbook(ctx, symbol, 1000)
		if err != nil {
			return nil, err
		}
		snapshot.Bids = response.Bids.Levels()
		snapshot.Asks = response.Asks.Levels()
		snapshot.LastUpdateID = response.LastUpdateID
		if snapshot.LastUpdateID == 0 {
			snapshot.LastUpdateID = response.UpdateID
		}
		snapshot.LastUpdated = response.TransactionTime.Time()
	default:
		return nil, asset.ErrNotSupported
	}
	return snapshot, nil
}

func (e *Exchange) derivativePair(symbol string, assetType asset.Item) (currency.Pair, error) {
	if pair, err := e.MatchSymbolWithAvailablePairs(symbol, assetType, assetType == asset.CoinMarginedFutures || assetType == asset.Options); err == nil {
		return pair, nil
	}
	if assetType == asset.Options || assetType == asset.CoinMarginedFutures {
		return currency.NewPairFromString(symbol)
	}
	underlying, contract, hasContract := strings.Cut(strings.ToUpper(symbol), "_")
	for _, quote := range []string{"USDT", "USDC", "BUSD", "USD"} {
		if strings.HasSuffix(underlying, quote) && len(underlying) > len(quote) {
			base := strings.TrimSuffix(underlying, quote)
			if hasContract {
				quote += "_" + contract
			}
			return currency.NewPairFromStrings(base, quote)
		}
	}
	return currency.EMPTYPAIR, currency.ErrPairNotFound
}

// After the CM migration both hosts carry a merged universe. Older payloads lack
// st, so the connection's asset remains the fallback for those messages.
func futuresStreamAsset(symbolType uint64, fallback asset.Item) asset.Item {
	switch symbolType {
	case 1:
		return asset.USDTMarginedFutures
	case 2:
		return asset.CoinMarginedFutures
	default:
		return fallback
	}
}
