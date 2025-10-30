package hyperliquid

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

// Exchange implements exchange.IBotExchange and contains additional specific API methods for interacting with Hyperliquid
type Exchange struct {
	exchange.Base
	walletMu          sync.Mutex
	wallet            *wallet
	vaultAddress      string
	accountAddr       string
	now               func() time.Time
	expiresAfter      *uint64
	assetCache        map[string]int64
	assetCacheMu      sync.RWMutex
	wsPendingMu       sync.Mutex
	wsPending         map[string][]pendingWSRequest
	activeAssetDataMu sync.RWMutex
	activeAssetData   map[string]ActiveAssetDataUpdate
}

const (
	apiURL   = "https://api.hyperliquid.xyz"
	infoPath = "/info"
	wsAPIURL = "wss://api.hyperliquid.xyz/ws"
)

func (e *Exchange) sendInfo(ctx context.Context, payload, result any) error {
	return e.sendPOSTWithLimit(ctx, infoPath, payload, result, infoRateLimit)
}

func (e *Exchange) sendPOST(ctx context.Context, path string, payload, result any) error {
	return e.sendPOSTWithLimit(ctx, path, payload, result, exchangeRateLimit)
}

func (e *Exchange) sendPOSTWithLimit(ctx context.Context, path string, payload, result any, limit request.EndpointLimit) error {
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

	return e.SendPayload(ctx, limit, generate, request.UnauthenticatedRequest)
}

func (e *Exchange) isMainnetEndpoint() bool {
	endpoint, err := e.API.Endpoints.GetURL(exchange.RestSpot)
	if err != nil {
		return true
	}
	return !strings.Contains(strings.ToLower(endpoint), "testnet")
}
