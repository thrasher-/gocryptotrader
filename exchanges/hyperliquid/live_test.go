//go:build mock_test_off

// This will build if build tag mock_test_off is parsed and will do live testing
// using all tests in (exchange)_test.go
package hyperliquid

import (
	"log"
	"os"
	"testing"

	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

var (
	mockTests = false
	// apiCredentials holds the account address (Key), an optional private key of the account or of one of its approved
	// API wallets (Secret) and an optional subaccount or vault (SubAccount); please supply your own for live testing
	apiCredentials = &accounts.Credentials{}
)

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Hyperliquid Setup error: %s", err)
	}
	if apiCredentials.Key != "" {
		e.API.AuthenticatedSupport = true
		e.API.AuthenticatedWebsocketSupport = true
		e.SetCredentials(apiCredentials)
	}
	log.Printf(sharedtestvalues.LiveTesting, e.Name)
	os.Exit(m.Run())
}

// newUserSignedTestExchange returns the live exchange, whose nonces follow the clock
func newUserSignedTestExchange(*testing.T) *Exchange {
	return e
}
