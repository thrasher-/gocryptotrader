//go:build !mock_test_off

// This will build unless build tag mock_test_off is parsed and will do mock testing
// using all tests in (exchange)_test.go
package gateio

import (
	"log"
	"os"
	"testing"

	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

var mockTests = true

const (
	tradFiOrderLogID            uint64 = 1223
	subAccountUserID            uint64 = 12345678
	subAccountKeyIdentifier            = "mock-subaccount-key-id"
	subAccountLoginName                = "test_sub_account_001"
	subAccountAPIKeyName               = "test_key"
	subAccountUpdatedAPIKeyName        = "updated_key"
)

func TestMain(m *testing.M) {
	e = new(Exchange)
	if err := testexch.Setup(e); err != nil {
		log.Fatal(err)
	}
	if err := testexch.MockHTTPInstance(e, ""); err != nil {
		log.Fatalf("MockHTTPInstance error: %s", err)
	}
	if err := e.enablePairs(); err != nil {
		log.Fatal(err)
	}
	os.Exit(m.Run())
}
