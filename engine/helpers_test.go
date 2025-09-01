package engine

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common/convert"
	"github.com/thrasher-corp/gocryptotrader/common/file"
	"github.com/thrasher-corp/gocryptotrader/communications"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/database"
	"github.com/thrasher-corp/gocryptotrader/dispatch"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/account"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/deposit"
	"github.com/thrasher-corp/gocryptotrader/exchanges/protocol"
	"github.com/thrasher-corp/gocryptotrader/exchanges/stats"
	"github.com/thrasher-corp/gocryptotrader/gctscript/vm"
	"github.com/thrasher-corp/gocryptotrader/log"
)

var testExchange = "Bitstamp"

func CreateTestBot(tb testing.TB) *Engine {
	tb.Helper()
	cFormat := &currency.PairFormat{Uppercase: true}
	cp1 := currency.NewBTCUSD()
	cp2 := currency.NewBTCUSDT()

	pairs1 := map[asset.Item]*currency.PairStore{
		asset.Spot: {
			AssetEnabled: true,
			Available:    currency.Pairs{cp1},
			Enabled:      currency.Pairs{cp1},
		},
	}
	pairs2 := map[asset.Item]*currency.PairStore{
		asset.Spot: {
			AssetEnabled: true,
			Available:    currency.Pairs{cp2},
			Enabled:      currency.Pairs{cp2},
		},
	}
	bot := &Engine{
		ExchangeManager: NewExchangeManager(),
		Config: &config.Config{Exchanges: []config.Exchange{
			{
				Name:                    testExchange,
				Enabled:                 true,
				WebsocketTrafficTimeout: time.Second,
				API: config.APIConfig{
					Credentials: config.APICredentialsConfig{},
				},
				CurrencyPairs: &currency.PairsManager{
					RequestFormat:   cFormat,
					ConfigFormat:    cFormat,
					UseGlobalFormat: true,
					Pairs:           pairs1,
				},
			},
			{
				Name:                    "binance",
				Enabled:                 true,
				WebsocketTrafficTimeout: time.Second,
				API: config.APIConfig{
					Credentials: config.APICredentialsConfig{},
				},
				CurrencyPairs: &currency.PairsManager{
					RequestFormat:   cFormat,
					ConfigFormat:    cFormat,
					UseGlobalFormat: true,
					Pairs:           pairs2,
				},
			},
		}},
	}
	err := bot.LoadExchange(testExchange)
	assert.NoError(tb, err, "LoadExchange should not error")

	return bot
}

func TestGetSubsystemsStatus(t *testing.T) {
	t.Parallel()
	m := (&Engine{}).GetSubsystemsStatus()
	assert.Len(t, m, 15, "subsystem count should be 15")
}

func TestGetRPCEndpoints(t *testing.T) {
	t.Parallel()
	_, err := (&Engine{}).GetRPCEndpoints()
	require.ErrorIs(t, err, errNilConfig, "GetRPCEndpoints must error on nil config")

	m, err := (&Engine{Config: &config.Config{}}).GetRPCEndpoints()
	require.NoError(t, err, "GetRPCEndpoints should not error on valid config")
	assert.Len(t, m, 4, "should return 4 RPC endpoints")
}

func TestSetSubsystem(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Subsystem    string
		Engine       *Engine
		EnableError  error
		DisableError error
	}{
		{Subsystem: "sillyBilly", EnableError: errNilBot, DisableError: errNilBot},
		{Subsystem: "sillyBilly", Engine: &Engine{}, EnableError: errNilConfig, DisableError: errNilConfig},
		{Subsystem: "sillyBilly", Engine: &Engine{Config: &config.Config{}}, EnableError: errSubsystemNotFound, DisableError: errSubsystemNotFound},
		{
			Subsystem:    CommunicationsManagerName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  communications.ErrNoRelayersEnabled,
			DisableError: ErrNilSubsystem,
		},
		{
			Subsystem:    ConnectionManagerName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  nil,
			DisableError: nil,
		},
		{
			Subsystem:    OrderManagerName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  nil,
			DisableError: nil,
		},
		{
			Subsystem:    PortfolioManagerName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  errNilExchangeManager,
			DisableError: ErrNilSubsystem,
		},
		{
			Subsystem:    NTPManagerName,
			Engine:       &Engine{Config: &config.Config{Logging: log.Config{Enabled: convert.BoolPtr(false)}}},
			EnableError:  errNilNTPConfigValues,
			DisableError: ErrNilSubsystem,
		},
		{
			Subsystem:    DatabaseConnectionManagerName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  database.ErrDatabaseSupportDisabled,
			DisableError: ErrSubSystemNotStarted,
		},
		{
			Subsystem:    SyncManagerName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  errNoSyncItemsEnabled,
			DisableError: ErrNilSubsystem,
		},
		{
			Subsystem:    dispatch.Name,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  nil,
			DisableError: nil,
		},

		{
			Subsystem:    DeprecatedName,
			Engine:       &Engine{Config: &config.Config{}, Settings: Settings{ConfigFile: config.DefaultFilePath()}},
			EnableError:  errServerDisabled,
			DisableError: ErrSubSystemNotStarted,
		},
		{
			Subsystem:    WebsocketName,
			Engine:       &Engine{Config: &config.Config{}, Settings: Settings{ConfigFile: config.DefaultFilePath()}},
			EnableError:  errServerDisabled,
			DisableError: ErrSubSystemNotStarted,
		},
		{
			Subsystem:    grpcName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  errGRPCManagementFault,
			DisableError: errGRPCManagementFault,
		},
		{
			Subsystem:    grpcProxyName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  errGRPCManagementFault,
			DisableError: errGRPCManagementFault,
		},
		{
			Subsystem:    dataHistoryManagerName,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  database.ErrNilInstance,
			DisableError: ErrNilSubsystem,
		},
		{
			Subsystem:    vm.Name,
			Engine:       &Engine{Config: &config.Config{}},
			EnableError:  nil,
			DisableError: nil,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.Subsystem, func(t *testing.T) {
			t.Parallel()
			err := tt.Engine.SetSubsystem(tt.Subsystem, true)
			require.ErrorIs(t, err, tt.EnableError)

			err = tt.Engine.SetSubsystem(tt.Subsystem, false)
			require.ErrorIs(t, err, tt.DisableError)
		})
	}
}

func TestGetExchangeOTPs(t *testing.T) {
	t.Parallel()
	bot := CreateTestBot(t)
	_, err := bot.GetExchangeOTPs()
	assert.Error(t, err, "GetExchangeOTPs should error when no exchange OTP secrets set")

	bnCfg, err := bot.Config.GetExchangeConfig("binance")
	require.NoError(t, err, "GetExchangeConfig must not error for binance")
	bCfg, err := bot.Config.GetExchangeConfig(testExchange)
	require.NoError(t, err, "GetExchangeConfig must not error for testExchange")

	bnCfg.API.Credentials.OTPSecret = "JBSWY3DPEHPK3PXP"
	bCfg.API.Credentials.OTPSecret = "JBSWY3DPEHPK3PXP"
	result, err := bot.GetExchangeOTPs()
	assert.NoError(t, err, "GetExchangeOTPs should not error with valid secrets")
	assert.Len(t, result, 2, "GetExchangeOTPs should return 2 OTP results")

	bnCfg.API.Credentials.OTPSecret = "°"
	result, err = bot.GetExchangeOTPs()
	assert.NoError(t, err, "GetExchangeOTPs should not error with one invalid secret")
	assert.Len(t, result, 1, "GetExchangeOTPs should return 1 OTP code with one invalid OTP Secret")

	// Flush settings
	bnCfg.API.Credentials.OTPSecret = ""
	bCfg.API.Credentials.OTPSecret = ""
}

func TestGetExchangeoOTPByName(t *testing.T) {
	t.Parallel()
	bot := CreateTestBot(t)
	_, err := bot.GetExchangeOTPByName(testExchange)
	assert.Error(t, err, "GetExchangeOTPByName should error with no exchange OTP secrets set")

	bCfg, err := bot.Config.GetExchangeConfig(testExchange)
	require.NoError(t, err, "GetExchangeConfig must not error for testExchange")

	bCfg.API.Credentials.OTPSecret = "JBSWY3DPEHPK3PXP"
	result, err := bot.GetExchangeOTPByName(testExchange)
	assert.NoError(t, err, "GetExchangeOTPByName should not error with a valid secret")
	assert.NotEmpty(t, result, "GetExchangeOTPByName should return a valid OTP code")

	// Flush setting
	bCfg.API.Credentials.OTPSecret = ""
}

func TestGetAuthAPISupportedExchanges(t *testing.T) {
	t.Parallel()
	e := CreateTestBot(t)
	assert.Empty(t, e.GetAuthAPISupportedExchanges(), "GetAuthAPISupportedExchanges should not return any exchanges initially")

	exch, err := e.ExchangeManager.GetExchangeByName(testExchange)
	require.NoError(t, err, "GetExchangeByName must not error")

	b := exch.GetBase()
	b.API.AuthenticatedWebsocketSupport = true
	b.SetCredentials("test", "test", "", "", "", "")
	assert.Len(t, e.GetAuthAPISupportedExchanges(), 1, "GetAuthAPISupportedExchanges should return one exchange")
}

func TestIsOnline(t *testing.T) {
	t.Parallel()
	e := CreateTestBot(t)
	var err error
	e.connectionManager, err = setupConnectionManager(&e.Config.ConnectionMonitor)
	require.NoError(t, err, "setupConnectionManager must not error")
	assert.False(t, e.IsOnline(), "IsOnline should be false initially")

	require.NoError(t, e.connectionManager.Start(), "connectionManager.Start must not error")
	t.Cleanup(func() {
		assert.NoError(t, e.connectionManager.Stop(), "connectionManager.Stop should not error")
	})

	assert.Eventually(t, e.IsOnline, 5*time.Second, 100*time.Millisecond, "IsOnline should become true")
}

func TestGetSpecificAvailablePairs(t *testing.T) {
	t.Parallel()
	e := CreateTestBot(t)
	c := currency.Code{
		Item: &currency.Item{
			Role:   currency.Cryptocurrency,
			Symbol: "usdt",
		},
	}
	e.Config = &config.Config{
		Exchanges: []config.Exchange{
			{
				Enabled: true,
				Name:    testExchange,
				CurrencyPairs: &currency.PairsManager{Pairs: map[asset.Item]*currency.PairStore{
					asset.Spot: {
						AssetEnabled: true,
						Enabled:      currency.Pairs{currency.NewBTCUSD(), currency.NewPair(currency.BTC, c)},
						Available:    currency.Pairs{currency.NewBTCUSD(), currency.NewPair(currency.BTC, c)},
						ConfigFormat: &currency.PairFormat{
							Uppercase: true,
						},
					},
				}},
			},
		},
	}
	assetType := asset.Spot

	result := e.GetSpecificAvailablePairs(true, true, true, true, assetType)
	btcUSD := currency.NewBTCUSD()
	assert.True(t, result.Contains(btcUSD, true), "result should contain BTC-USD")

	btcUSDT := currency.NewPair(currency.BTC, c)
	assert.True(t, result.Contains(btcUSDT, false), "result should contain BTC-USDT")

	result = e.GetSpecificAvailablePairs(true, true, false, false, assetType)

	assert.False(t, result.Contains(btcUSDT, false), "result should not contain BTC-USDT")

	ltcBTC := currency.NewPair(currency.LTC, currency.BTC)
	result = e.GetSpecificAvailablePairs(true, false, false, true, assetType)
	assert.False(t, result.Contains(ltcBTC, false), "result should not contain LTC-BTC")
}

func TestIsRelatablePairs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		p1       string
		p2       string
		usdt     bool
		expected bool
	}{
		{"similar names", "XBT-USD", "BTC-USD", false, true},
		{"similar names reversed", "BTC-USD", "XBT-USD", false, true},
		{"similar names with tether disabled", "XBT-USD", "BTC-USDT", false, false},
		{"similar names with tether enabled", "XBT-USDT", "BTC-USD", true, true},
		{"different ordering and delimiter with tether", "AE-USDT", "USDT-AE", true, true},
		{"different ordering and delimiter without tether", "AE-USDT", "USDT-AE", false, true},
		{"similar names different fiat", "XBT-EUR", "BTC-AUD", false, true},
		{"similar names different fiat and ordering", "USD-BTC", "BTC-EUR", false, true},
		{"similar names different fiat with tether", "USD-BTC", "BTC-USDT", true, true},
		{"similar crypto pairs", "LTC-BTC", "BTC-LTC", false, true},
		{"different crypto pairs", "LTC-ETH", "BTC-ETH", false, false},
		{"USDT-USD vs BTC-USD with tether", "USDT-USD", "BTC-USD", true, false},
		{"similar crypto names 2", "XBT-LTC", "BTC-LTC", false, true},
		{"similar crypto names different ordering", "LTC-XBT", "BTC-LTC", false, true},
		{"non-relational fiat", "EUR-USD", "BTC-USD", false, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p1, err := currency.NewPairFromString(tc.p1)
			require.NoError(t, err, "Must create pair from string")
			p2, err := currency.NewPairFromString(tc.p2)
			require.NoError(t, err, "Must create pair from string")

			result := IsRelatablePairs(p1, p2, tc.usdt)
			assert.Equal(t, tc.expected, result, "IsRelatablePairs result should be as expected")
		})
	}
}

func TestGetRelatableCryptocurrencies(t *testing.T) {
	t.Parallel()
	btcltc, err := currency.NewPairFromStrings("BTC", "LTC")
	require.NoError(t, err, "must create pair from string")

	btcbtc, err := currency.NewPairFromStrings("BTC", "BTC")
	require.NoError(t, err, "must create pair from string")

	ltcltc, err := currency.NewPairFromStrings("LTC", "LTC")
	require.NoError(t, err, "must create pair from string")

	btceth, err := currency.NewPairFromStrings("BTC", "ETH")
	require.NoError(t, err, "must create pair from string")

	p := GetRelatableCryptocurrencies(btcltc)
	assert.False(t, p.Contains(btcltc, true), "result should not contain BTCLTC")
	assert.False(t, p.Contains(btcbtc, true), "result should not contain BTCBTC")
	assert.False(t, p.Contains(ltcltc, true), "result should not contain LTCLTC")
	assert.True(t, p.Contains(btceth, true), "result should contain BTCETH")
}

func TestGetRelatableFiatCurrencies(t *testing.T) {
	t.Parallel()
	btcUSD, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "must create pair from string")

	btcEUR, err := currency.NewPairFromStrings("BTC", "EUR")
	require.NoError(t, err, "must create pair from string")

	p := GetRelatableFiatCurrencies(btcUSD)
	assert.True(t, p.Contains(btcEUR, true), "result should contain BTCEUR")

	assert.False(t, p.Contains(currency.NewPair(currency.DOGE, currency.XRP), true), "result should not contain DOGEXRP")
}

func TestMapCurrenciesByExchange(t *testing.T) {
	t.Parallel()
	e := CreateTestBot(t)

	pairs := []currency.Pair{
		currency.NewBTCUSD(),
		currency.NewPair(currency.BTC, currency.EUR),
	}

	result := e.MapCurrenciesByExchange(pairs, true, asset.Spot)
	pairs, ok := result[testExchange]
	require.True(t, ok, "result must contain the test exchange")
	assert.Len(t, pairs, 2, "pairs length should be 2")
}

func TestGetExchangeNamesByCurrency(t *testing.T) {
	t.Parallel()
	btsusd, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "must create pair from string")

	btcjpy, err := currency.NewPairFromStrings("BTC", "JPY")
	require.NoError(t, err, "must create pair from string")

	blahjpy, err := currency.NewPairFromStrings("blah", "JPY")
	require.NoError(t, err, "must create pair from string")

	e := CreateTestBot(t)
	bf := "Bitflyer"
	e.Config.Exchanges = append(e.Config.Exchanges, config.Exchange{
		Enabled: true,
		Name:    bf,
		CurrencyPairs: &currency.PairsManager{Pairs: map[asset.Item]*currency.PairStore{
			asset.Spot: {
				AssetEnabled: true,
				Enabled:      currency.Pairs{btcjpy},
				Available:    currency.Pairs{btcjpy},
				ConfigFormat: &currency.PairFormat{
					Uppercase: true,
				},
			},
		}},
	})
	assetType := asset.Spot

	result := e.GetExchangeNamesByCurrency(btsusd,
		true,
		assetType)
	assert.Contains(t, result, testExchange, "result should contain test exchange")

	result = e.GetExchangeNamesByCurrency(btcjpy,
		true,
		assetType)
	assert.Contains(t, result, bf, "result should contain bitflyer")

	result = e.GetExchangeNamesByCurrency(blahjpy,
		true,
		assetType)
	assert.Empty(t, result, "result should be empty")
}

func TestGetCollatedExchangeAccountInfoByCoin(t *testing.T) {
	t.Parallel()

	var exchangeInfo []account.Holdings

	var bitfinexHoldings account.Holdings
	bitfinexHoldings.Exchange = "Bitfinex"
	bitfinexHoldings.Accounts = append(bitfinexHoldings.Accounts,
		account.SubAccount{
			Currencies: []account.Balance{
				{
					Currency: currency.BTC,
					Total:    100,
					Hold:     0,
				},
			},
		})

	exchangeInfo = append(exchangeInfo, bitfinexHoldings)

	var bitstampHoldings account.Holdings
	bitstampHoldings.Exchange = testExchange
	bitstampHoldings.Accounts = append(bitstampHoldings.Accounts,
		account.SubAccount{
			Currencies: []account.Balance{
				{
					Currency: currency.LTC,
					Total:    100,
					Hold:     0,
				},
				{
					Currency: currency.BTC,
					Total:    100,
					Hold:     0,
				},
			},
		})

	exchangeInfo = append(exchangeInfo, bitstampHoldings)

	result := GetCollatedExchangeAccountInfoByCoin(exchangeInfo)
	require.NotEmpty(t, result, "result must not be empty")

	amount, ok := result[currency.BTC]
	require.True(t, ok, "currency must be found in result map")
	assert.Equal(t, float64(200), amount.Total, "total should be 200")

	_, ok = result[currency.ETH]
	assert.False(t, ok, "currency should not be found in result map")
}

func TestGetExchangePriceByCurrencyPair(t *testing.T) {
	t.Parallel()
	stats.StatMutex.Lock()
	stats.Items = stats.Items[:0]
	stats.StatMutex.Unlock()
	p, err := currency.NewPairFromStrings("BTC", "USD")
	require.NoError(t, err, "must create pair from string")

	err = stats.Add("Bitfinex", p, asset.Spot, 1000, 10000)
	require.NoError(t, err, "stats.Add must not error")
	err = stats.Add(testExchange, p, asset.Spot, 1337, 10000)
	require.NoError(t, err, "stats.Add must not error")

	btcaud, err := currency.NewPairFromStrings("BTC", "AUD")
	require.NoError(t, err, "must create pair from string")

	testCases := []struct {
		name         string
		pair         currency.Pair
		asset        asset.Item
		function     func(currency.Pair, asset.Item) (string, error)
		expectedExch string
		expectedErr  bool
	}{
		{
			name:         "Highest Price",
			pair:         p,
			asset:        asset.Spot,
			function:     GetExchangeHighestPriceByCurrencyPair,
			expectedExch: testExchange,
		},
		{
			name:         "Lowest Price",
			pair:         p,
			asset:        asset.Spot,
			function:     GetExchangeLowestPriceByCurrencyPair,
			expectedExch: "Bitfinex",
		},
		{
			name:        "Highest Price - no stats",
			pair:        btcaud,
			asset:       asset.Spot,
			function:    GetExchangeHighestPriceByCurrencyPair,
			expectedErr: true,
		},
		{
			name:        "Lowest Price - no stats",
			pair:        btcaud,
			asset:       asset.Spot,
			function:    GetExchangeLowestPriceByCurrencyPair,
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exchangeName, err := tc.function(tc.pair, tc.asset)
			if tc.expectedErr {
				assert.Error(t, err, "function should error")
			} else {
				assert.NoError(t, err, "function should not error")
				assert.Equal(t, tc.expectedExch, exchangeName, "should return correct exchange name")
			}
		})
	}
}

func TestGetCryptocurrenciesByExchange(t *testing.T) {
	t.Parallel()
	e := CreateTestBot(t)
	_, err := e.GetCryptocurrenciesByExchange("Bitfinex", false, false, asset.Spot)
	assert.NoError(t, err, "GetCryptocurrenciesByExchange should not error")
}

type fakeDepositExchangeOpts struct {
	SupportsAuth             bool
	SupportsMultiChain       bool
	RequiresChainSet         bool
	ReturnMultipleChains     bool
	ThrowPairError           bool
	ThrowTransferChainError  bool
	ThrowDepositAddressError bool
}

type fakeDepositExchange struct {
	exchange.IBotExchange
	*fakeDepositExchangeOpts
}

func (f fakeDepositExchange) GetName() string {
	return "fake"
}

func (f fakeDepositExchange) GetBase() *exchange.Base {
	return &exchange.Base{
		Features: exchange.Features{Supports: exchange.FeaturesSupported{
			RESTCapabilities: protocol.Features{
				MultiChainDeposits:                f.SupportsMultiChain,
				MultiChainDepositRequiresChainSet: f.RequiresChainSet,
			},
		}},
	}
}

func (f fakeDepositExchange) IsRESTAuthenticationSupported() bool {
	return f.SupportsAuth
}

func (f fakeDepositExchange) GetAvailableTransferChains(_ context.Context, c currency.Code) ([]string, error) {
	if f.ThrowTransferChainError {
		return nil, errors.New("unable to get available transfer chains")
	}
	if c.Equal(currency.XRP) {
		return nil, nil
	}
	if c.Equal(currency.USDT) {
		return []string{"sol", "btc", "usdt", ""}, nil
	}
	return []string{"BITCOIN"}, nil
}

func (f fakeDepositExchange) GetDepositAddress(_ context.Context, _ currency.Code, _, _ string) (*deposit.Address, error) {
	if f.ThrowDepositAddressError {
		return nil, errors.New("unable to get deposit address")
	}
	return &deposit.Address{Address: "fakeaddr"}, nil
}

func createDepositEngine(opts *fakeDepositExchangeOpts) *Engine {
	ps := currency.PairStore{
		AssetEnabled: true,
		Enabled: currency.Pairs{
			currency.NewBTCUSDT(),
			currency.NewPair(currency.XRP, currency.USDT),
		},
		Available: currency.Pairs{
			currency.NewBTCUSDT(),
			currency.NewPair(currency.XRP, currency.USDT),
		},
	}
	if opts.ThrowPairError {
		ps.Available = nil
	}
	return &Engine{
		Settings: Settings{CoreSettings: CoreSettings{Verbose: true}},
		Config: &config.Config{
			Exchanges: []config.Exchange{
				{
					Name:    "fake",
					Enabled: true,
					CurrencyPairs: &currency.PairsManager{
						UseGlobalFormat: true,
						ConfigFormat:    &currency.EMPTYFORMAT,
						Pairs: map[asset.Item]*currency.PairStore{
							asset.Spot: &ps,
						},
					},
				},
			},
		},
		ExchangeManager: &ExchangeManager{
			exchanges: map[string]exchange.IBotExchange{
				"fake": fakeDepositExchange{
					fakeDepositExchangeOpts: opts,
				},
			},
		},
	}
}

func TestGetCryptocurrencyDepositAddressesByExchange(t *testing.T) {
	t.Parallel()
	const exchName = "fake"
	e := createDepositEngine(&fakeDepositExchangeOpts{SupportsAuth: true, SupportsMultiChain: true})
	_, err := e.GetCryptocurrencyDepositAddressesByExchange(exchName)
	assert.NoError(t, err, "GetCryptocurrencyDepositAddressesByExchange should not error")
	_, err = e.GetCryptocurrencyDepositAddressesByExchange("non-existent")
	assert.ErrorIs(t, err, ErrExchangeNotFound, "GetCryptocurrencyDepositAddressesByExchange must error on non-existent exchange")

	e.DepositAddressManager = SetupDepositAddressManager()
	_, err = e.GetCryptocurrencyDepositAddressesByExchange(exchName)
	assert.Error(t, err, "GetCryptocurrencyDepositAddressesByExchange should error")
	err = e.DepositAddressManager.Sync(e.GetAllExchangeCryptocurrencyDepositAddresses())
	require.NoError(t, err, "Sync must not error")
	_, err = e.GetCryptocurrencyDepositAddressesByExchange(exchName)
	assert.NoError(t, err, "GetCryptocurrencyDepositAddressesByExchange should not error")
}

func TestGetExchangeCryptocurrencyDepositAddress(t *testing.T) {
	t.Parallel()
	e := createDepositEngine(&fakeDepositExchangeOpts{SupportsAuth: true, SupportsMultiChain: true})
	_, err := e.GetExchangeCryptocurrencyDepositAddress(t.Context(), "non-existent", "", "", currency.BTC, false)
	assert.ErrorIs(t, err, ErrExchangeNotFound)

	const exchName = "fake"
	r, err := e.GetExchangeCryptocurrencyDepositAddress(t.Context(), exchName, "", "", currency.BTC, false)
	require.NoError(t, err, "GetExchangeCryptocurrencyDepositAddress must not error")
	assert.Equal(t, "fakeaddr", r.Address, "Should return the correct r.Address")
	e.DepositAddressManager = SetupDepositAddressManager()
	err = e.DepositAddressManager.Sync(e.GetAllExchangeCryptocurrencyDepositAddresses())
	assert.NoError(t, err, "Sync should not error")
	_, err = e.GetExchangeCryptocurrencyDepositAddress(t.Context(), "meow", "", "", currency.BTC, false)
	assert.ErrorIs(t, err, ErrExchangeNotFound)
	_, err = e.GetExchangeCryptocurrencyDepositAddress(t.Context(), exchName, "", "", currency.BTC, false)
	assert.NoError(t, err, "GetExchangeCryptocurrencyDepositAddress should not error")
}

func TestGetAllExchangeCryptocurrencyDepositAddresses(t *testing.T) {
	t.Parallel()
	e := createDepositEngine(&fakeDepositExchangeOpts{})
	assert.Empty(t, e.GetAllExchangeCryptocurrencyDepositAddresses(), "should have no addresses returned for an unauthenticated exchange")

	e = createDepositEngine(&fakeDepositExchangeOpts{SupportsAuth: true, ThrowPairError: true})
	assert.Empty(t, e.GetAllExchangeCryptocurrencyDepositAddresses(), "should have no cryptos returned for no enabled pairs")

	e = createDepositEngine(&fakeDepositExchangeOpts{SupportsAuth: true, SupportsMultiChain: true, ThrowTransferChainError: true})
	assert.Empty(t, e.GetAllExchangeCryptocurrencyDepositAddresses()["fake"], "should have returned no deposit addresses for a fake exchange with transfer error")

	e = createDepositEngine(&fakeDepositExchangeOpts{SupportsAuth: true, SupportsMultiChain: true, ThrowDepositAddressError: true})
	assert.Empty(t, e.GetAllExchangeCryptocurrencyDepositAddresses()["fake"]["btc"], "should have returned no deposit addresses for fake exchange with deposit error, with multichain support enabled")

	e = createDepositEngine(&fakeDepositExchangeOpts{SupportsAuth: true, SupportsMultiChain: true, RequiresChainSet: true})
	assert.NotEmpty(t, e.GetAllExchangeCryptocurrencyDepositAddresses()["fake"]["btc"], "should of returned a BTC address")

	e = createDepositEngine(&fakeDepositExchangeOpts{SupportsAuth: true, SupportsMultiChain: true})
	assert.NotEmpty(t, e.GetAllExchangeCryptocurrencyDepositAddresses()["fake"]["btc"], "should of returned a BTC address")

	e = createDepositEngine(&fakeDepositExchangeOpts{SupportsAuth: true})
	assert.NotEmpty(t, e.GetAllExchangeCryptocurrencyDepositAddresses()["fake"]["xrp"], "should have returned a XRP address")
}

func TestGetExchangeNames(t *testing.T) {
	t.Parallel()
	bot := CreateTestBot(t)
	assert.NotEmpty(t, bot.GetExchangeNames(true), "exchange names should be populated")

	require.NoError(t, bot.UnloadExchange(testExchange), "UnloadExchange must not error")
	assert.NotContains(t, bot.GetExchangeNames(true), testExchange, "Bitstamp should be missing")
	assert.Empty(t, bot.GetExchangeNames(false), "should not have any inactive exchanges")

	for i := range bot.Config.Exchanges {
		exch, err := bot.ExchangeManager.NewExchangeByName(bot.Config.Exchanges[i].Name)
		require.Truef(t, err == nil || errors.Is(err, ErrExchangeAlreadyLoaded),
			"%s NewExchangeByName must not error: %s", bot.Config.Exchanges[i].Name, err)
		if exch != nil {
			exch.SetDefaults()
			err = bot.ExchangeManager.Add(exch)
			require.NoError(t, err)
		}
	}
	assert.Len(t, bot.GetExchangeNames(false), len(bot.Config.Exchanges), "should have all exchanges loaded")
}

func mockCert(derType string, notAfter time.Time) ([]byte, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	dnsNames := []string{host}
	if host != "localhost" {
		dnsNames = append(dnsNames, "localhost")
	}

	if notAfter.IsZero() {
		notAfter = time.Now().Add(time.Hour * 24 * 365)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"gocryptotrader"},
			CommonName:   host,
		},
		NotBefore:             time.Now(),
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
		DNSNames: dnsNames,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, err
	}

	if derType == "" {
		derType = "CERTIFICATE"
	}

	certData := pem.EncodeToMemory(&pem.Block{Type: derType, Bytes: derBytes})
	if certData == nil {
		return nil, err
	}

	return certData, nil
}

func TestVerifyCert(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		pemType       string
		createBypass  bool
		notAfter      time.Time
		errorExpected error
	}{
		{
			name:          "valid cert",
			errorExpected: nil,
		},
		{
			name:          "nil cert data",
			createBypass:  true,
			errorExpected: errCertDataIsNil,
		},
		{
			name:          "invalid pem type",
			pemType:       "MEOW",
			errorExpected: errCertTypeInvalid,
		},
		{
			name:          "expired cert",
			notAfter:      time.Now().Add(-time.Hour),
			errorExpected: errCertExpired,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cert []byte
			var err error
			if !tc.createBypass {
				cert, err = mockCert(tc.pemType, tc.notAfter)
				require.NoError(t, err, "mockCert should not error")
			}
			err = verifyCert(cert)
			assert.ErrorIs(t, err, tc.errorExpected, "verifyCert should return the expected error")
		})
	}
}

func TestCheckAndGenCerts(t *testing.T) {
	t.Parallel()

	tempDir := filepath.Join(os.TempDir(), "gct-temp-tls")
	t.Cleanup(func() {
		assert.NoError(t, os.RemoveAll(tempDir), "cleanup should not error")
	})

	require.NoError(t, genCert(tempDir), "genCert must not error")
	require.NoError(t, CheckCerts(tempDir), "CheckCerts must not error on valid certs")

	// Now delete cert.pem and test regeneration of cert/key files
	certFile := filepath.Join(tempDir, "cert.pem")
	require.NoError(t, os.Remove(certFile), "must be able to remove cert file")
	require.NoError(t, CheckCerts(tempDir), "CheckCerts must not error when regenerating certs")

	// Now call CheckCerts to test an expired cert
	certData, err := mockCert("", time.Now().Add(-time.Hour))
	require.NoError(t, err, "mockCert must not error")
	require.NoError(t, file.Write(certFile, certData), "must be able to write expired cert")
	require.NoError(t, CheckCerts(tempDir), "CheckCerts must not error when regenerating expired cert")
}

func TestNewSupportedExchangeByName(t *testing.T) {
	t.Parallel()

	for _, exchName := range exchange.Exchanges {
		t.Run(exchName, func(t *testing.T) {
			t.Parallel()
			exch, err := NewSupportedExchangeByName(exchName)
			require.NoError(t, err, "NewSupportedExchangeByName must not error for supported exchange")
			require.NotNil(t, exch, "NewSupportedExchangeByName must not return a nil exchange")
		})
	}

	_, err := NewSupportedExchangeByName("")
	assert.ErrorIs(t, err, ErrExchangeNotFound, "NewSupportedExchangeByName should error for empty exchange name")
}

func TestNewExchangeByNameWithDefaults(t *testing.T) {
	t.Parallel()

	_, err := NewExchangeByNameWithDefaults(t.Context(), "moarunlikelymeow")
	assert.ErrorIs(t, err, ErrExchangeNotFound, "Invalid exchange name should error")
	for x := range exchange.Exchanges {
		name := exchange.Exchanges[x]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if isCITest() && slices.Contains(blockedCIExchanges, name) {
				t.Skipf("skipping %s due to CI test restrictions", name)
			}
			if slices.Contains(unsupportedDefaultConfigExchanges, name) {
				t.Skipf("skipping %s unsupported", name)
			}
			exch, err := NewExchangeByNameWithDefaults(t.Context(), name)
			if assert.NoError(t, err, "NewExchangeByNameWithDefaults should not error") {
				assert.Equal(t, name, strings.ToLower(exch.GetName()), "Should get correct exchange name")
			}
		})
	}
}
