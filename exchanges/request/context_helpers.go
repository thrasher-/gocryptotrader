package request

import (
	"context"
	"net/http"
	"sync"

	"github.com/thrasher-corp/gocryptotrader/common"
)

const contextVerboseFlag verbosity = "verbose"

type verbosity string

type headersKey struct{}

// Header overrides are optional, so register their key on first use while still guaranteeing registration before context freezing.
var registerHeadersContextKey = sync.OnceFunc(func() {
	common.RegisterContextKey(headersKey{})
})

// WithVerbose adds verbosity to a request context so that specific requests
// can have distinct verbosity without impacting all requests.
func WithVerbose(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextVerboseFlag, true)
}

// IsVerbose checks main verbosity first then checks context verbose values
// for specific request verbosity.
func IsVerbose(ctx context.Context, verbose bool) bool {
	if !verbose {
		verbose, _ = ctx.Value(contextVerboseFlag).(bool)
	}
	return verbose
}

// WithHeaders adds outbound HTTP header overrides to the context. These values
// replace matching generated headers, including authentication headers.
func WithHeaders(ctx context.Context, headers http.Header) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	registerHeadersContextKey()
	return context.WithValue(ctx, headersKey{}, headers.Clone())
}

func headersFromContext(ctx context.Context) http.Header {
	headers, _ := ctx.Value(headersKey{}).(http.Header)
	return headers
}

type delayNotAllowedKey struct{}

// WithDelayNotAllowed adds a value to the context that indicates that no delay is allowed for rate limiting.
func WithDelayNotAllowed(ctx context.Context) context.Context {
	return context.WithValue(ctx, delayNotAllowedKey{}, struct{}{})
}

func hasDelayNotAllowed(ctx context.Context) bool {
	_, ok := ctx.Value(delayNotAllowedKey{}).(struct{})
	return ok
}

type retryNotAllowedKey struct{}

// WithRetryNotAllowed adds a value to the context that indicates that no retries are allowed for requests.
func WithRetryNotAllowed(ctx context.Context) context.Context {
	return context.WithValue(ctx, retryNotAllowedKey{}, struct{}{})
}

func hasRetryNotAllowed(ctx context.Context) bool {
	_, ok := ctx.Value(retryNotAllowedKey{}).(struct{})
	return ok
}
