package exchange

import (
	"context"
	"fmt"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/asset"
	"github.com/thrasher-corp/gocryptotrader/exchange/ticker"
)

// ValidateAPICredentials validates an exchange's API credentials by updating its account balances for
// an asset type. A network error does not fail them, since it says nothing about the credentials
func ValidateAPICredentials(ctx context.Context, e IBotExchange, a asset.Item) error {
	_, err := e.UpdateAccountBalances(ctx, a)
	return e.GetBase().CheckTransientError(err)
}

// UpdateTradablePairs stores the tradable pairs an exchange fetches for each of its asset types as that
// asset type's available pairs, then ensures at least one pair is enabled
func UpdateTradablePairs(ctx context.Context, e IBotExchange) error {
	b := e.GetBase()
	for _, a := range e.GetAssetTypes(false) {
		pairs, err := e.FetchTradablePairs(ctx, a)
		if err != nil {
			return err
		}
		if err := b.UpdatePairs(pairs, a, false); err != nil {
			return err
		}
	}
	return e.EnsureOnePairEnabled()
}

// UpdateTicker updates every ticker of an asset type with the exchange's UpdateTickers and returns the
// pair's, for exchanges that fetch all their tickers in one request
func UpdateTicker(ctx context.Context, e IBotExchange, p currency.Pair, a asset.Item) (*ticker.Price, error) {
	if err := e.UpdateTickers(ctx, a); err != nil {
		return nil, err
	}
	return ticker.GetTicker(e.GetName(), p, a)
}

// GetFeeByType returns the fee getFee calculates for a fee builder. A cryptocurrency trade fee is
// estimated as an offline trade fee instead when the exchange has no verified credentials to look up
// its account's rate
func GetFeeByType(ctx context.Context, e IBotExchange, f *FeeBuilder, getFee func(context.Context, *FeeBuilder) (float64, error)) (float64, error) {
	if f == nil {
		return 0, fmt.Errorf("%T %w", f, common.ErrNilPointer)
	}
	b := e.GetBase()
	if (b.SkipAuthCheck || !b.AreCredentialsValid(ctx)) && f.FeeType == CryptocurrencyTradeFee {
		f.FeeType = OfflineTradeFee
	}
	return getFee(ctx, f)
}
