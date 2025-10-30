package hyperliquid

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

const (
	infoRequestsPerSecond     = 10
	exchangeRequestsPerSecond = 10

	infoRateLimit request.EndpointLimit = iota
	exchangeRateLimit
)

// GetRateLimits returns the Hyperliquid REST rate limits.
func GetRateLimits() request.RateLimitDefinitions {
	return request.RateLimitDefinitions{
		infoRateLimit:     request.NewRateLimitWithWeight(time.Second, infoRequestsPerSecond, 1),
		exchangeRateLimit: request.NewRateLimitWithWeight(time.Second, exchangeRequestsPerSecond, 1),
	}
}
