package request_test

import (
	"net"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

func TestDefaultRetryPolicy(t *testing.T) {
	t.Parallel()
	type args struct {
		Error    error
		Response *http.Response
	}
	type want struct {
		Error error
		Retry bool
	}
	testTable := map[string]struct {
		Args args
		Want want
	}{
		"DNS Error": {
			Args: args{Error: &net.DNSError{Err: "fake"}},
			Want: want{Error: &net.DNSError{Err: "fake"}},
		},
		"DNS Timeout": {
			Args: args{Error: &net.DNSError{Err: "fake", IsTimeout: true}},
			Want: want{Retry: true},
		},
		"Too Many Requests": {
			Args: args{Response: &http.Response{StatusCode: http.StatusTooManyRequests}},
			Want: want{Retry: true},
		},
		"Not Found": {
			Args: args{Response: &http.Response{StatusCode: http.StatusNotFound}},
		},
		"Retry After": {
			Args: args{Response: &http.Response{StatusCode: http.StatusTeapot, Header: http.Header{"Retry-After": []string{"0.5"}}}},
			Want: want{Retry: true},
		},
	}

	for name, tt := range testTable {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			retry, err := request.DefaultRetryPolicy(tt.Args.Response, tt.Args.Error)

			if exp := tt.Want.Error; exp != nil {
				require.Truef(t, reflect.DeepEqual(err, exp), "DefaultRetryPolicy must return expected error; got %v want %v", err, exp)
				return
			}

			require.NoErrorf(t, err, "DefaultRetryPolicy must not return error for test %s", name)
			assert.Equalf(t, tt.Want.Retry, retry, "DefaultRetryPolicy should set retry flag for test %s", name)
		})
	}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Date(2020, time.April, 20, 13, 31, 13, 0, time.UTC)

	type args struct {
		Now      time.Time
		Response *http.Response
	}
	type want struct {
		Delay time.Duration
	}
	testTable := map[string]struct {
		Args args
		Want want
	}{
		"No Response": {},
		"Empty Header": {
			Args: args{Response: &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{""}}}},
		},
		"Partial Seconds": {
			Args: args{Response: &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{"0.5"}}}},
		},
		"Delay Seconds": {
			Args: args{Response: &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{"3"}}}},
			Want: want{Delay: 3 * time.Second},
		},
		"Invalid HTTP Date RFC3339": {
			Args: args{
				Now:      now,
				Response: &http.Response{StatusCode: http.StatusTeapot, Header: http.Header{"Retry-After": []string{"2020-04-02T13:31:18Z"}}},
			},
		},
		"Valid HTTP Date": {
			Args: args{
				Now:      now,
				Response: &http.Response{StatusCode: http.StatusTeapot, Header: http.Header{"Retry-After": []string{"Mon, 20 Apr 2020 13:31:18 GMT"}}},
			},
			Want: want{Delay: 5 * time.Second},
		},
	}

	for name, tt := range testTable {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			delay := request.RetryAfter(tt.Args.Response, tt.Args.Now)
			assert.Equalf(t, tt.Want.Delay, delay, "RetryAfter should return expected delay for test %s", name)
		})
	}
}
