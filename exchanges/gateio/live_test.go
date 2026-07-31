//go:build mock_test_off

// This will build if build tag mock_test_off is parsed and will do live testing
// using all tests in (exchange)_test.go
package gateio

import (
	"log"
	"os"
	"testing"

	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/livetest"
)

var (
	mockTests = false
	// apiCredentials holds the credentials used for due diligence testing; please supply your own.
	apiCredentials = &accounts.Credentials{
		Key:    "",
		Secret: "",
	}
	// Supply existing test-only sub-account resources to exercise the corresponding live tests.
	subAccountUserID            uint64
	subAccountKeyIdentifier     string
	subAccountLoginName         string
	subAccountAPIKeyName        string
	subAccountUpdatedAPIKeyName string
)

const tradFiOrderLogID uint64 = 0

func TestMain(m *testing.M) {
	if livetest.ShouldSkip() {
		log.Printf(livetest.LiveTestingSkipped, "GateIO")
		os.Exit(0)
	}

	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatalf("Gateio Setup error: %s", err)
	}

	if apiCredentials.Key != "" && apiCredentials.Secret != "" {
		e.API.AuthenticatedSupport = true
		e.API.AuthenticatedWebsocketSupport = true
		e.SetCredentials(apiCredentials)
	}
	log.Printf(sharedtestvalues.LiveTesting, e.Name)
	os.Exit(m.Run())
}
