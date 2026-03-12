package request

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"time"
)

const headerRetryAfter = "Retry-After"

type retryNotAllowedKey struct{}

// DefaultRetryPolicy determines whether the request should be retried, implemented with a default strategy.
func DefaultRetryPolicy(resp *http.Response, err error) (bool, error) {
	if err != nil {
		if timeoutErr, ok := err.(net.Error); ok && timeoutErr.Timeout() {
			return true, nil
		}
		return false, err
	}

	return resp.StatusCode == http.StatusTooManyRequests || resp.Header.Get(headerRetryAfter) != "", nil
}

// RetryAfter parses the Retry-After header in the response to determine the minimum
// duration needed to wait before retrying.
func RetryAfter(resp *http.Response, now time.Time) time.Duration {
	if resp == nil {
		return 0
	}

	after := resp.Header.Get(headerRetryAfter)
	if after == "" {
		return 0
	}

	if sec, err := strconv.ParseInt(after, 10, 32); err == nil {
		return time.Duration(sec) * time.Second
	}

	if when, err := time.Parse(time.RFC1123, after); err == nil {
		return when.Sub(now)
	}

	return 0
}

// WithRetryNotAllowed adds a value to the context that indicates that no retries are allowed for requests.
func WithRetryNotAllowed(ctx context.Context) context.Context {
	return context.WithValue(ctx, retryNotAllowedKey{}, struct{}{})
}

func hasRetryNotAllowed(ctx context.Context) bool {
	_, ok := ctx.Value(retryNotAllowedKey{}).(struct{})
	return ok
}
