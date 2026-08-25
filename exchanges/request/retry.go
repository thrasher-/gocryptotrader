package request

import (
	"net/http"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/internal/logsafe"
)

const headerRetryAfter = "Retry-After"

// DefaultRetryPolicy determines whether the request should be retried, implemented with a default strategy.
func DefaultRetryPolicy(resp *http.Response, err error) (bool, error) {
	if err != nil {
		if logsafe.IsTimeout(err) {
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
