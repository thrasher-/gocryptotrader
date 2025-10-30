package hyperliquid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

// Exchange implements exchange.IBotExchange and contains additional specific API methods for interacting with Hyperliquid
type Exchange struct {
	exchange.Base
	walletMu     sync.Mutex
	wallet       *wallet
	vaultAddress string
	accountAddr  string
	now          func() time.Time
	expiresAfter *uint64
	assetCache   map[string]int64
	assetCacheMu sync.RWMutex
}

const (
	apiURL   = "https://api.hyperliquid.xyz"
	infoPath = "/info"
	wsAPIURL = "wss://api.hyperliquid.xyz/ws"
)

func (e *Exchange) sendInfo(ctx context.Context, payload any, result any) error {
	return e.sendPOST(ctx, infoPath, payload, result)
}

func (e *Exchange) sendPOST(ctx context.Context, path string, payload any, result any) error {
	endpoint, err := e.API.Endpoints.GetURL(exchange.RestSpot)
	if err != nil {
		return fmt.Errorf("get REST endpoint: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request payload: %w", err)
	}

	item := &request.Item{
		Method:                 http.MethodPost,
		Path:                   endpoint + path,
		Headers:                map[string]string{"Content-Type": "application/json"},
		Body:                   bytes.NewReader(data),
		Result:                 result,
		Verbose:                e.Verbose,
		HTTPDebugging:          e.HTTPDebugging,
		HTTPRecording:          e.HTTPRecording,
		HTTPMockDataSliceLimit: e.HTTPMockDataSliceLimit,
	}

	generate := func() (*request.Item, error) {
		return item, nil
	}

	return e.SendPayload(ctx, request.Unset, generate, request.UnauthenticatedRequest)
}

// Start implementing public and private exchange API funcs below
// Private endpoints can be implemented in a separate file with a _private.go suffix for ease of access and simplicity.

func (e *Exchange) isMainnetEndpoint() bool {
	endpoint, err := e.API.Endpoints.GetURL(exchange.RestSpot)
	if err != nil {
		return true
	}
	return !strings.Contains(strings.ToLower(endpoint), "testnet")
}
