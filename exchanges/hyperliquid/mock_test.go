//go:build !mock_test_off

// This will build unless build tag mock_test_off is parsed and will do mock testing
// using all tests in (exchange)_test.go
package hyperliquid

import (
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

var mockTests = true

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := setupMockExchange(e); err != nil {
		log.Fatal(err)
	}
	os.Exit(m.Run())
}

// setupMockExchange serves an exchange from the recorded responses, signing with the Python SDK's public test key so
// signed actions run against the recorded exchange endpoint responses
func setupMockExchange(ex *Exchange) error {
	if err := testexch.Setup(ex); err != nil {
		return err
	}
	ex.API.AuthenticatedSupport = true
	ex.API.AuthenticatedWebsocketSupport = true
	ex.SetCredentials(&accounts.Credentials{Key: testAccountAddress, Secret: testPrivateKey})
	if err := testexch.MockHTTPInstance(ex); err != nil {
		return err
	}
	// Mock responses are served locally, so throttling protects nothing
	return ex.DisableRateLimiter()
}

// newUserSignedTestExchange returns a mock exchange whose next nonce is the recorded user-signed actions' nonce
func newUserSignedTestExchange(t *testing.T) *Exchange {
	t.Helper()
	ex := new(Exchange)
	require.NoError(t, setupMockExchange(ex), "setupMockExchange must not error")
	ex.lastNonce.Store(mockUserSignedNonce - 1)
	return ex
}
