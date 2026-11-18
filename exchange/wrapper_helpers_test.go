package exchange_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchange/asset"
	shared "github.com/thrasher-corp/gocryptotrader/exchange/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/exchange/ticker"
)

var errHelper = errors.New("helper test error")

// helperEx serves the exchange methods the wrapper helpers call from its Base and fields, and records
// the asset types it is asked for
type helperEx struct {
	shared.CustomEx
	balancesErr error
	pairs       map[asset.Item]currency.Pairs
	pairsErr    error
	tickers     []*ticker.Price
	tickersErr  error
	assets      []asset.Item
}

func (h *helperEx) GetBase() *exchange.Base { return &h.Base }

func (h *helperEx) GetName() string { return h.Name }

func (h *helperEx) GetAssetTypes(enabled bool) asset.Items { return h.Base.GetAssetTypes(enabled) }

func (h *helperEx) UpdateAccountBalances(_ context.Context, a asset.Item) (accounts.SubAccounts, error) {
	h.assets = append(h.assets, a)
	return nil, h.balancesErr
}

func (h *helperEx) FetchTradablePairs(_ context.Context, a asset.Item) (currency.Pairs, error) {
	h.assets = append(h.assets, a)
	return h.pairs[a], h.pairsErr
}

func (h *helperEx) UpdateTickers(_ context.Context, a asset.Item) error {
	h.assets = append(h.assets, a)
	if h.tickersErr != nil {
		return h.tickersErr
	}
	for _, p := range h.tickers {
		if err := ticker.ProcessTicker(p); err != nil {
			return err
		}
	}
	return nil
}

func TestValidateAPICredentials(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		balancesErr error
		err         error
	}{
		{name: "balances updated"},
		{name: "credentials rejected", balancesErr: errHelper, err: errHelper},
		{name: "network error", balancesErr: &net.DNSError{Err: "i/o timeout", IsTimeout: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &helperEx{balancesErr: tc.balancesErr}
			assert.ErrorIs(t, exchange.ValidateAPICredentials(t.Context(), e, asset.Futures), tc.err, "ValidateAPICredentials should fail only for a non-network error")
			assert.Equal(t, []asset.Item{asset.Futures}, e.assets, "ValidateAPICredentials should update the asset type's balances")
		})
	}
}

func TestUpdateTradablePairs(t *testing.T) {
	t.Parallel()
	pFmt := currency.PairFormat{Uppercase: true, Delimiter: currency.DashDelimiter}
	spot := currency.Pairs{currency.NewBTCUSD().Format(pFmt), currency.NewPair(currency.ETH, currency.USD).Format(pFmt)}
	futures := currency.Pairs{currency.NewBTCUSDT().Format(pFmt)}
	for _, tc := range []struct {
		name     string
		pairs    map[asset.Item]currency.Pairs
		pairsErr error
		err      error
	}{
		{name: "stores every asset type's pairs", pairs: map[asset.Item]currency.Pairs{asset.Spot: spot, asset.Futures: futures}},
		{name: "fetch error", pairsErr: errHelper, err: errHelper},
		{name: "no pairs fetched", pairs: map[asset.Item]currency.Pairs{asset.Spot: spot}, err: currency.ErrCurrencyPairsEmpty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &helperEx{
				Name: "UpdateTradablePairs",
				CurrencyPairs: currency.PairsManager{
					Pairs:           map[asset.Item]*currency.PairStore{asset.Spot: {AssetEnabled: true}, asset.Futures: {}},
					ConfigFormat:    &pFmt,
					UseGlobalFormat: true,
				},
				Config:   &config.Exchange{CurrencyPairs: &currency.PairsManager{}},
				pairs:    tc.pairs,
				pairsErr: tc.pairsErr,
			}
			err := exchange.UpdateTradablePairs(t.Context(), e)
			require.ErrorIs(t, err, tc.err, "UpdateTradablePairs must return the error that stopped it")
			if tc.err != nil {
				return
			}
			assert.ElementsMatch(t, []asset.Item{asset.Spot, asset.Futures}, e.assets, "UpdateTradablePairs should fetch every asset type's pairs")
			var enabled int
			for a, want := range tc.pairs {
				available, err := e.CurrencyPairs.GetPairs(a, false)
				require.NoErrorf(t, err, "GetPairs must not error for %s", a)
				assert.Equalf(t, want, available, "UpdateTradablePairs should store the %s pairs fetched", a)
				enabledPairs, err := e.CurrencyPairs.GetPairs(a, true)
				require.NoErrorf(t, err, "GetPairs must not error for enabled %s pairs", a)
				enabled += len(enabledPairs)
			}
			assert.Equal(t, 1, enabled, "UpdateTradablePairs should enable one pair when none are enabled")
		})
	}
}

func TestUpdateTicker(t *testing.T) {
	t.Parallel()
	btcusd, ethusd := currency.NewBTCUSD(), currency.NewPair(currency.ETH, currency.USD)
	for _, tc := range []struct {
		name       string
		stored     currency.Pairs
		tickersErr error
		err        error
	}{
		{name: "returns the pair's ticker", stored: currency.Pairs{btcusd, ethusd}},
		{name: "update error", tickersErr: errHelper, err: errHelper},
		{name: "pair not updated", stored: currency.Pairs{btcusd}, err: ticker.ErrTickerNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &helperEx{tickersErr: tc.tickersErr}
			e.Name = t.Name()
			for i, p := range tc.stored {
				e.tickers = append(e.tickers, &ticker.Price{ExchangeName: e.Name, Pair: p, AssetType: asset.Spot, Last: float64(i + 1)})
			}
			got, err := exchange.UpdateTicker(t.Context(), e, ethusd, asset.Spot)
			require.ErrorIs(t, err, tc.err, "UpdateTicker must return the error that stopped it")
			assert.Equal(t, []asset.Item{asset.Spot}, e.assets, "UpdateTicker should update the asset type's tickers")
			if tc.err != nil {
				assert.Nil(t, got, "UpdateTicker should not return a ticker with an error")
				return
			}
			require.NotNil(t, got, "UpdateTicker must return a ticker")
			assert.Equal(t, ethusd, got.Pair, "UpdateTicker should return the requested pair's ticker")
			assert.Equal(t, 2.0, got.Last, "UpdateTicker should return the requested pair's stored price")
		})
	}
}

func TestGetFeeByType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		credentials   bool
		skipAuthCheck bool
		feeType       exchange.FeeType
		calculated    exchange.FeeType
	}{
		{name: "trade fee without credentials", feeType: exchange.CryptocurrencyTradeFee, calculated: exchange.OfflineTradeFee},
		{name: "trade fee with credentials", credentials: true, feeType: exchange.CryptocurrencyTradeFee, calculated: exchange.CryptocurrencyTradeFee},
		{name: "trade fee with auth checks skipped", credentials: true, skipAuthCheck: true, feeType: exchange.CryptocurrencyTradeFee, calculated: exchange.OfflineTradeFee},
		{name: "withdrawal fee without credentials", feeType: exchange.CryptocurrencyWithdrawalFee, calculated: exchange.CryptocurrencyWithdrawalFee},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := new(helperEx)
			e.SkipAuthCheck = tc.skipAuthCheck
			if tc.credentials {
				e.GetBase().SetCredentials(&accounts.Credentials{Key: "key", Secret: "secret"})
			}
			var calculated []exchange.FeeType
			fee, err := exchange.GetFeeByType(t.Context(), e, &exchange.FeeBuilder{FeeType: tc.feeType}, func(_ context.Context, f *exchange.FeeBuilder) (float64, error) {
				calculated = append(calculated, f.FeeType)
				return 0.25, nil
			})
			require.NoError(t, err, "GetFeeByType must not error")
			assert.Equal(t, 0.25, fee, "GetFeeByType should return the calculated fee")
			assert.Equal(t, []exchange.FeeType{tc.calculated}, calculated, "GetFeeByType should calculate the fee type once")
		})
	}
	t.Run("nil fee builder", func(t *testing.T) {
		t.Parallel()
		_, err := exchange.GetFeeByType(t.Context(), new(helperEx), nil, func(context.Context, *exchange.FeeBuilder) (float64, error) {
			require.Fail(t, "GetFeeByType must not calculate a fee without a fee builder")
			return 0, nil
		})
		assert.ErrorIs(t, err, common.ErrNilPointer, "GetFeeByType should reject a nil fee builder")
	})
	t.Run("calculation error", func(t *testing.T) {
		t.Parallel()
		_, err := exchange.GetFeeByType(t.Context(), new(helperEx), &exchange.FeeBuilder{FeeType: exchange.CryptocurrencyTradeFee}, func(context.Context, *exchange.FeeBuilder) (float64, error) {
			return 0, errHelper
		})
		assert.ErrorIs(t, err, errHelper, "GetFeeByType should return the calculation's error")
	})
}
