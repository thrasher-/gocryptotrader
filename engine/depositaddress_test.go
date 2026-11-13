package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/deposit"
)

const (
	address  = "1F1tAaz5x1HUXrCNLbtMDqcw6o5GNn4xqX"
	bitStamp = "BITSTAMP"
	btc      = "BTC"
)

func TestIsSynced(t *testing.T) {
	t.Parallel()
	var d DepositAddressManager
	if d.IsSynced() {
		t.Error("should be false")
	}
	m := SetupDepositAddressManager()
	err := m.Sync(map[string]ExchangeDepositAddresses{
		bitStamp: {
			btc: []deposit.Address{
				{
					Address: address,
				},
			},
		},
	})
	if err != nil {
		t.Error(err)
	}
	if !m.IsSynced() {
		t.Error("should be synced")
	}
}

func TestSetupDepositAddressManager(t *testing.T) {
	t.Parallel()
	m := SetupDepositAddressManager()
	if m.store == nil {
		t.Fatal("expected store")
	}
}

func TestSync(t *testing.T) {
	t.Parallel()
	m := SetupDepositAddressManager()
	err := m.Sync(map[string]ExchangeDepositAddresses{
		bitStamp: {
			btc: []deposit.Address{
				{
					Address: address,
				},
			},
		},
	})
	if err != nil {
		t.Error(err)
	}
	r, err := m.GetDepositAddressByExchangeAndCurrency(bitStamp, "", currency.BTC)
	if err != nil {
		t.Error("unexpected result")
	}
	if r.Address != address {
		t.Error("unexpected result")
	}

	m.store = nil
	err = m.Sync(map[string]ExchangeDepositAddresses{
		bitStamp: {
			btc: []deposit.Address{
				{
					Address: address,
				},
			},
		},
	})
	assert.ErrorIs(t, err, ErrDepositAddressStoreIsNil)

	m = nil
	err = m.Sync(map[string]ExchangeDepositAddresses{
		bitStamp: {
			btc: []deposit.Address{
				{
					Address: address,
				},
			},
		},
	})
	assert.ErrorIs(t, err, ErrNilSubsystem)
}

func TestGetDepositAddressByExchangeAndCurrency(t *testing.T) {
	t.Parallel()
	m := SetupDepositAddressManager()
	_, err := m.GetDepositAddressByExchangeAndCurrency("", "", currency.BTC)
	assert.ErrorIs(t, err, ErrDepositAddressStoreIsNil)

	m.store = map[string]ExchangeDepositAddresses{
		bitStamp: {
			btc: []deposit.Address{
				{
					Address: address,
				},
			},
			"USDT": []deposit.Address{
				{
					Address: "ABsdZ",
					Chain:   "SOL",
				},
				{
					Address: "0x1b",
					Chain:   "ERC20",
				},
				{
					Address: "1asdad",
					Chain:   "USDT",
				},
			},
			"BNB": nil,
		},
	}
	_, err = m.GetDepositAddressByExchangeAndCurrency("asdf", "", currency.BTC)
	assert.ErrorIs(t, err, ErrExchangeNotFound)

	_, err = m.GetDepositAddressByExchangeAndCurrency(bitStamp, "", currency.LTC)
	assert.ErrorIs(t, err, ErrDepositAddressNotFound)

	_, err = m.GetDepositAddressByExchangeAndCurrency(bitStamp, "", currency.BNB)
	assert.ErrorIs(t, err, errNoDepositAddressesRetrieved)

	_, err = m.GetDepositAddressByExchangeAndCurrency(bitStamp, "NON-EXISTENT-CHAIN", currency.USDT)
	assert.ErrorIs(t, err, errDepositAddressChainNotFound)

	for _, tc := range []struct {
		name            string
		chain           string
		code            currency.Code
		expectedAddress string
		expectedChain   string
	}{
		{name: "mixed-case ERC20 chain", chain: "ErC20", code: currency.USDT, expectedAddress: "0x1b", expectedChain: "ERC20"},
		{name: "mixed-case SOL chain", chain: "sOl", code: currency.USDT, expectedAddress: "ABsdZ", expectedChain: "SOL"},
		{name: "no chain defaults to currency chain", code: currency.USDT, expectedAddress: "1asdad", expectedChain: "USDT"},
		{name: "no chain defaults to first address", code: currency.BTC, expectedAddress: address},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, err := m.GetDepositAddressByExchangeAndCurrency(bitStamp, tc.chain, tc.code)
			require.NoError(t, err, "GetDepositAddressByExchangeAndCurrency must not error")
			assert.Equal(t, tc.expectedAddress, r.Address, "GetDepositAddressByExchangeAndCurrency should return the expected address")
			assert.Equal(t, tc.expectedChain, r.Chain, "GetDepositAddressByExchangeAndCurrency should return the expected chain")
		})
	}
}

func TestGetDepositAddressesByExchange(t *testing.T) {
	t.Parallel()
	m := SetupDepositAddressManager()
	_, err := m.GetDepositAddressesByExchange("")
	assert.ErrorIs(t, err, ErrDepositAddressStoreIsNil)

	m.store = map[string]ExchangeDepositAddresses{
		bitStamp: {
			btc: []deposit.Address{
				{
					Address: address,
				},
			},
		},
	}
	_, err = m.GetDepositAddressesByExchange("non-existent")
	assert.ErrorIs(t, err, ErrDepositAddressNotFound)

	_, err = m.GetDepositAddressesByExchange(bitStamp)
	assert.NoError(t, err)
}
