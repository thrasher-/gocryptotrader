package v23_test

import (
	"testing"

	migration "github.com/thrasher-corp/gocryptotrader/config/versions/v23"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/configversion"
)

func TestDeploy(t *testing.T) {
	t.Parallel()
	before := `{"exchanges":[{"name":"GateIO","api":{"urlEndpoints":{"RestFuturesURL":"https://proxy.example/gate-fx"}},"bankAccounts":[{"supportedExchanges":"GateIO"}]},{"name":"Kraken"}],"bankAccounts":[{"supportedExchanges":"Kraken,GateIO"}],"portfolioAddresses":{"addresses":[{"Address":"GateIO","CoinType":"BTC","Balance":1,"Description":"Exchange"},{"Address":"1abc","CoinType":"BTC","Description":"Personal","SupportedExchanges":"GateIO,Kraken"}]}}`
	configversion.Migrate(t, new(migration.Version), before, `{"exchanges":[{"name":"Gate","api":{"urlEndpoints":{"RestFuturesURL":"https://proxy.example/gate-fx"}},"bankAccounts":[{"supportedExchanges":"Gate"}]},{"name":"Kraken"}],"bankAccounts":[{"supportedExchanges":"Kraken,Gate"}],"portfolioAddresses":{"addresses":[{"Address":"Gate","CoinType":"BTC","Balance":1,"Description":"Exchange"},{"Address":"1abc","CoinType":"BTC","Description":"Personal","SupportedExchanges":"Gate,Kraken"}]}}`, before)
}
