//go:build !mock_test_off

// This will build if build tag mock_test_off is not parsed and will try to mock
// all tests in _test.go
package gemini

import (
	"log"
	"os"
	"testing"

	"github.com/thrasher-corp/gocryptotrader/exchanges/mock"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

const mockFile = "../../testdata/http_mock/gemini/gemini.json"

var mockTests = true

func TestMain(m *testing.M) {
	g = new(Gemini)
	if err := testexch.Setup(g); err != nil {
		log.Fatalf("Gemini Setup error: %s", err)
	}

	g.SkipAuthCheck = true
	if apiKey != "" && apiSecret != "" {
		g.API.AuthenticatedSupport = true
		g.SetCredentials(apiKey, apiSecret, "", "", "", "")
	}
	if err := testexch.MockHTTPInstance(g); err != nil {
		log.Fatal(err)
	}
	os.Exit(m.Run())
}
