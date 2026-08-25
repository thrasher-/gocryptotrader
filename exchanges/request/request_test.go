package request

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/exchanges/nonce"
	"github.com/thrasher-corp/gocryptotrader/internal/logsafe"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
)

const unexpected = "unexpected values"

var (
	testURL     string
	serverLimit *RateLimiterWithWeight
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (r roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

type trackedReadCloser struct {
	io.Reader
	closeErr   error
	closeCalls int
}

func (t *trackedReadCloser) Close() error {
	t.closeCalls++
	return t.closeErr
}

type countingReader struct {
	io.Reader
	readCalls int
}

type requestTestTimeoutOnlyError struct{}

func (*requestTestTimeoutOnlyError) Error() string { return "timeout-only-secret-token" }

func (*requestTestTimeoutOnlyError) Timeout() bool { return true }

type requestTestPanickingNetError struct{}

func (*requestTestPanickingNetError) Error() string { return "panicking-network-secret-token" }

func (*requestTestPanickingNetError) Timeout() bool { panic("panicking-network-timeout-token") }

func (*requestTestPanickingNetError) Temporary() bool { return false }

func (c *countingReader) Read(p []byte) (int, error) {
	c.readCalls++
	return c.Reader.Read(p)
}

func TestCountingReaderRead(t *testing.T) {
	t.Parallel()
	const payload = "counting-reader-payload"
	reader := &countingReader{Reader: strings.NewReader(payload)}
	buffer := make([]byte, len(payload))
	n, err := reader.Read(buffer)
	require.NoError(t, err, "Read must not error while consuming the counting reader payload")
	assert.Equal(t, len(payload), n, "Read should return the complete payload length")
	assert.Equal(t, payload, string(buffer), "Read should return the complete payload")
	assert.Equal(t, 1, reader.readCalls, "Read should increment readCalls exactly once")
}

type trackingReporter struct {
	calls  int
	name   string
	method string
	path   string
}

func (t *trackingReporter) Latency(name, method, path string, _ time.Duration) {
	t.calls++
	t.name = name
	t.method = method
	t.path = path
}

func TestRoundTripFuncRoundTrip(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("transport failure")
	for _, tc := range []struct {
		name             string
		expectedResponse *http.Response
		expectedErr      error
	}{
		{
			name:             "response",
			expectedResponse: &http.Response{StatusCode: http.StatusTeapot, Body: http.NoBody},
		},
		{
			name:        "error",
			expectedErr: expectedErr,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "https://example.com", http.NoBody)
			var receivedRequest *http.Request
			transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				receivedRequest = req
				return tc.expectedResponse, tc.expectedErr
			})

			response, err := transport.RoundTrip(req)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr, "RoundTrip must return the transport error")
			} else {
				require.NoError(t, err, "RoundTrip must not error for a transport response")
			}
			assert.Same(t, tc.expectedResponse, response, "RoundTrip should return the transport response")
			assert.Same(t, req, receivedRequest, "RoundTrip should pass the request to the transport")
			if response != nil {
				require.NoError(t, response.Body.Close(), "response.Body.Close must not error")
			}
		})
	}
}

func TestTrackedReadCloserClose(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("close failure")
	for _, tc := range []struct {
		name     string
		closeErr error
	}{
		{
			name: "success",
		},
		{
			name:     "error",
			closeErr: closeErr,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			closer := &trackedReadCloser{closeErr: tc.closeErr}
			err := closer.Close()
			if tc.closeErr != nil {
				require.ErrorIs(t, err, tc.closeErr, "Close must return the configured error")
			} else {
				require.NoError(t, err, "Close must not error without a configured error")
			}
			assert.Equal(t, 1, closer.closeCalls, "Close should increment closeCalls exactly once")
		})
	}
}

func TestTrackingReporterLatency(t *testing.T) {
	t.Parallel()
	reporter := new(trackingReporter)
	reporter.Latency("exchange", http.MethodGet, "/path?signature=raw-signed-path-canary", time.Second)
	assert.Equal(t, 1, reporter.calls, "Latency should increment calls exactly once")
	assert.Equal(t, "exchange", reporter.name, "Latency should retain the exchange name")
	assert.Equal(t, http.MethodGet, reporter.method, "Latency should retain the request method")
	assert.Equal(t, "/path?signature=raw-signed-path-canary", reporter.path, "Latency should retain the raw signed request path")
}

func TestExecuteRequestReporterRawPathCompatibility(t *testing.T) {
	t.Parallel()

	const rawSignedPath = "https://example.com/private/path?api_key=raw-key-canary&signature=raw-signature-canary"
	reporter := new(trackingReporter)
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})}
	r, err := New("reporter-compatibility", httpClient, WithReporter(reporter))
	require.NoError(t, err, "New must configure the reporter compatibility requester")
	t.Cleanup(func() {
		assert.NoError(t, r.Shutdown(), "Shutdown should close the reporter compatibility requester")
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawSignedPath, http.NoBody)
	require.NoError(t, err, "http.NewRequestWithContext must create the reporter compatibility request")

	retry, err := r.executeRequest(t.Context(), &Item{Method: http.MethodGet, Path: rawSignedPath}, req, 1, false)
	require.NoError(t, err, "executeRequest must complete the reporter compatibility request")
	assert.False(t, retry, "executeRequest should not retry the reporter compatibility request")
	assert.Equal(t, 1, reporter.calls, "executeRequest should report one latency observation")
	assert.Equal(t, "reporter-compatibility", reporter.name, "executeRequest should pass the requester name to the reporter")
	assert.Equal(t, http.MethodGet, reporter.method, "executeRequest should pass the method to the reporter")
	assert.Equal(t, rawSignedPath, reporter.path, "executeRequest should preserve the raw signed path compatibility boundary")
}

func TestMain(m *testing.M) {
	serverLimitInterval := time.Millisecond * 500
	serverLimit = NewWeightedRateLimitByDuration(serverLimitInterval)
	serverLimitRetry := NewWeightedRateLimitByDuration(serverLimitInterval)
	sm := http.NewServeMux()
	sm.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, `{"response":true}`)
		if err != nil {
			log.Fatal(err)
		}
	})
	sm.HandleFunc("/error", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, err := io.WriteString(w, `{"error":true}`)
		if err != nil {
			log.Fatal(err)
		}
	})
	sm.HandleFunc("/timeout", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Millisecond * 100)
		w.WriteHeader(http.StatusGatewayTimeout)
	})
	sm.HandleFunc("/rate", func(w http.ResponseWriter, _ *http.Request) {
		if !serverLimit.limiter.Allow() {
			http.Error(w,
				http.StatusText(http.StatusTooManyRequests),
				http.StatusTooManyRequests)
			_, err := io.WriteString(w, `{"response":false}`)
			if err != nil {
				log.Fatal(err)
			}
			return
		}
		_, err := io.WriteString(w, `{"response":true}`)
		if err != nil {
			log.Fatal(err)
		}
	})
	sm.HandleFunc("/rate-retry", func(w http.ResponseWriter, _ *http.Request) {
		if !serverLimitRetry.limiter.Allow() {
			w.Header().Add("Retry-After", strconv.Itoa(int(math.Round(serverLimitInterval.Seconds()))))
			http.Error(w,
				http.StatusText(http.StatusTooManyRequests),
				http.StatusTooManyRequests)
			_, err := io.WriteString(w, `{"response":false}`)
			if err != nil {
				log.Fatal(err)
			}
			return
		}
		_, err := io.WriteString(w, `{"response":true}`)
		if err != nil {
			log.Fatal(err)
		}
	})
	sm.HandleFunc("/always-retry", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Retry-After", time.Now().Format(time.RFC1123))
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := io.WriteString(w, `{"response":false}`)
		if err != nil {
			log.Fatal(err)
		}
	})
	sm.HandleFunc("/nocontent", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewServer(sm)
	testURL = server.URL
	issues := m.Run()
	server.Close()
	os.Exit(issues)
}

func TestCheckRequest(t *testing.T) {
	t.Parallel()

	r, err := New("TestRequest",
		new(http.Client))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	var check *Item
	_, err = check.validateRequest(ctx, &Requester{})
	if err == nil {
		t.Fatal(unexpected)
	}

	_, err = check.validateRequest(ctx, nil)
	if err == nil {
		t.Fatal(unexpected)
	}

	_, err = check.validateRequest(ctx, r)
	if err == nil {
		t.Fatal(unexpected)
	}

	check = &Item{}
	_, err = check.validateRequest(ctx, r)
	if err == nil {
		t.Fatal(unexpected)
	}

	check.Path = testURL
	check.Method = " " // Forces method check; "" automatically converts to GET
	_, err = check.validateRequest(ctx, r)
	if err == nil {
		t.Fatal(unexpected)
	}

	check.Method = http.MethodPost
	_, err = check.validateRequest(ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	var passback http.Header
	check.HeaderResponse = &passback
	_, err = check.validateRequest(ctx, r)
	if err == nil {
		t.Fatal("expected error when underlying memory is not allocated")
	}
	passback = http.Header{}

	// Test setting headers
	check.Headers = map[string]string{
		"Content-Type": "Super awesome HTTP party experience",
		"preserved":    "true",
	}

	// Test user agent set
	r.userAgent = "r00t axxs"
	req, err := check.validateRequest(ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	if req.Header.Get("Content-Type") != "Super awesome HTTP party experience" {
		t.Fatal(unexpected)
	}

	if req.UserAgent() != "r00t axxs" {
		t.Fatal(unexpected)
	}

	ctx = WithHeaders(ctx, http.Header{
		"Content-Type": {"context override"},
		"User-Agent":   {"context agent"},
		"X-Values":     {"one", "two"},
	})
	req, err = check.validateRequest(ctx, r)
	require.NoError(t, err, "validateRequest must not error")
	assert.Equal(t, "context override", req.Header.Get("Content-Type"), "context header should override item header")
	assert.Equal(t, "context agent", req.UserAgent(), "context header should override requester user agent")
	assert.Equal(t, []string{"one", "two"}, req.Header.Values("X-Values"), "context header values should be preserved")
	assert.Equal(t, "true", req.Header.Get("preserved"), "item header should be preserved when not overridden by context")
}

var globalshell = RateLimitDefinitions{
	Auth:   NewWeightedRateLimitByDuration(time.Millisecond * 600),
	UnAuth: NewRateLimitWithWeight(time.Second*1, 100, 1),
}

func TestDoRequest(t *testing.T) {
	t.Parallel()
	r, err := New("test", new(http.Client), WithLimiter(globalshell))
	require.NoError(t, err, "New requester must not error")

	ctx := t.Context()
	err = (*Requester)(nil).SendPayload(ctx, Unset, nil, UnauthenticatedRequest)
	require.ErrorIs(t, err, ErrRequestSystemIsNil)

	err = r.SendPayload(ctx, Unset, nil, UnauthenticatedRequest)
	require.ErrorIs(t, err, errRequestFunctionIsNil)

	err = r.SendPayload(ctx, UnAuth, func() (*Item, error) { return nil, nil }, UnauthenticatedRequest)
	require.ErrorIs(t, err, errRequestItemNil)

	err = r.SendPayload(ctx, UnAuth, func() (*Item, error) { return &Item{}, nil }, UnauthenticatedRequest)
	require.ErrorIs(t, err, errInvalidPath)

	var nilHeader http.Header
	err = r.SendPayload(ctx, UnAuth, func() (*Item, error) {
		return &Item{
			Path:           testURL,
			HeaderResponse: &nilHeader,
		}, nil
	}, UnauthenticatedRequest)
	require.ErrorIs(t, err, errHeaderResponseMapIsNil)

	// Invalid/missing endpoint limit
	err = r.SendPayload(ctx, Unset, func() (*Item, error) { return &Item{Path: testURL}, nil }, UnauthenticatedRequest)
	require.ErrorIs(t, err, common.ErrNilPointer)

	// Force debug
	err = r.SendPayload(ctx, UnAuth, func() (*Item, error) {
		return &Item{
			Path: testURL,
			Headers: map[string]string{
				"test": "supertest",
			},
			Body:          strings.NewReader("test"),
			HTTPDebugging: true,
			Verbose:       true,
		}, nil
	}, UnauthenticatedRequest)
	require.NoError(t, err, "SendPayload must not error")

	// Fail new request call
	newError := errors.New("request item failure")
	err = r.SendPayload(ctx, UnAuth, func() (*Item, error) {
		return nil, newError
	}, UnauthenticatedRequest)
	require.ErrorIs(t, err, newError)

	r._HTTPClient, err = newProtectedClient(common.NewHTTPClientWithTimeout(0))
	require.NoError(t, err, "newProtectedClient must not error")

	// timeout checker
	err = r._HTTPClient.setHTTPClientTimeout(time.Millisecond * 50)
	require.NoError(t, err, "setHTTPClientTimeout must not error")
	err = r.SendPayload(ctx, UnAuth, func() (*Item, error) {
		return &Item{Path: testURL + "/timeout"}, nil
	}, UnauthenticatedRequest)
	require.ErrorIs(t, err, errFailedToRetryRequest)
	// reset timeout
	err = r._HTTPClient.setHTTPClientTimeout(0)
	require.NoError(t, err, "setHTTPClientTimeout must not error")

	// Check JSON
	var resp struct {
		Response bool `json:"response"`
	}

	// Check header contents
	passback := http.Header{}
	err = r.SendPayload(ctx, UnAuth, func() (*Item, error) {
		return &Item{
			Method:         http.MethodGet,
			Path:           testURL,
			Result:         &resp,
			HeaderResponse: &passback,
		}, nil
	}, UnauthenticatedRequest)
	require.NoError(t, err, "SendPayload must not error")

	require.Equal(t, "17", passback.Get("Content-Length"), "Content-Length must have the correct value")
	require.Equal(t, "application/json", passback.Get("Content-Type"), "Content-Type must have the correct value")

	// Check error
	var respErr struct {
		Error bool `json:"error"`
	}
	err = r.SendPayload(ctx, UnAuth, func() (*Item, error) {
		return &Item{
			Method: http.MethodGet,
			Path:   testURL,
			Result: &respErr,
		}, nil
	}, UnauthenticatedRequest)
	require.NoError(t, err, "SendPayload must not error")
	require.False(t, respErr.Error, "Error must be false")

	// Check client side rate limit
	var ec common.ErrorCollector
	for range 5 {
		ec.Go(func() error {
			var resp struct {
				Response bool `json:"response"`
			}
			if err := r.SendPayload(ctx, Auth, func() (*Item, error) {
				return &Item{
					Method: http.MethodGet,
					Path:   testURL + "/rate",
					Result: &resp,
				}, nil
			}, AuthenticatedRequest); err != nil {
				return fmt.Errorf("SendPayload error: %w", err)
			}
			if !resp.Response {
				return fmt.Errorf("unexpected response: %+v", resp)
			}
			return nil
		})
	}

	require.NoError(t, ec.Collect(), "Collect must return no errors")
}

func TestExecuteRequestClosesResponseBody(t *testing.T) {
	t.Parallel()

	readErr := errors.New("response read failure")
	closeErr := errors.New("response close failure")
	retryErr := errors.New("retry policy failure")

	for _, tc := range []struct {
		name          string
		statusCode    int
		body          io.Reader
		closeErr      error
		retryPolicy   RetryPolicy
		expectedErr   error
		errContains   string
		headers       http.Header
		trailer       http.Header
		transfer      []string
		record        bool
		debug         bool
		verbose       bool
		report        bool
		attempt       int
		expectedRetry bool
	}{
		{
			name:       "successful response",
			statusCode: http.StatusOK,
			body:       strings.NewReader(`{"response":true}`),
		},
		{
			name:        "unsuccessful response",
			statusCode:  http.StatusBadRequest,
			body:        strings.NewReader(`{"error":true}`),
			expectedErr: ErrBadStatus,
		},
		{
			name:        "response read failure",
			statusCode:  http.StatusOK,
			body:        iotest.ErrReader(readErr),
			expectedErr: readErr,
		},
		{
			name:       "response close failure",
			statusCode: http.StatusOK,
			body:       strings.NewReader(`{"response":true}`),
			closeErr:   closeErr,
		},
		{
			name:       "retry policy failure",
			statusCode: http.StatusOK,
			body:       strings.NewReader(`{"response":true}`),
			retryPolicy: func(*http.Response, error) (bool, error) {
				return false, retryErr
			},
			expectedErr: retryErr,
		},
		{
			name:        "mock recording failure",
			statusCode:  http.StatusOK,
			body:        strings.NewReader(`{"response":"recording-response-body-canary"}`),
			record:      true,
			errContains: "mock recording failure",
		},
		{
			name:       "debug response dump failure",
			statusCode: http.StatusOK,
			body:       strings.NewReader(`{"response":true}`),
			trailer: http.Header{
				"Content-Length": {"1"},
			},
			transfer: []string{"chunked"},
			debug:    true,
			verbose:  true,
		},
		{
			name:       "verbose response with latency reporter",
			statusCode: http.StatusOK,
			body:       strings.NewReader(`{"response":true}`),
			headers: http.Header{
				"X-Test": {"value"},
			},
			verbose: true,
			report:  true,
		},
		{
			name:          "retry response",
			statusCode:    http.StatusTooManyRequests,
			body:          strings.NewReader(`{"retry":true}`),
			expectedRetry: true,
		},
		{
			name:        "terminal retry response",
			statusCode:  http.StatusTooManyRequests,
			body:        strings.NewReader(`{"retry":true}`),
			attempt:     MaxRetryAttempts + 1,
			expectedErr: errFailedToRetryRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			status := fmt.Sprintf("%d %s", tc.statusCode, http.StatusText(tc.statusCode))
			headers := tc.headers.Clone()
			if tc.record {
				status = "200 recording-response-status-canary"
				if headers == nil {
					headers = make(http.Header)
				}
				headers.Set("X-Recording-Response-Header-Canary", "recording-response-header-canary")
			}
			body := &trackedReadCloser{
				Reader:   tc.body,
				closeErr: tc.closeErr,
			}
			httpClient := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						Status:           status,
						StatusCode:       tc.statusCode,
						ProtoMajor:       1,
						ProtoMinor:       1,
						Header:           headers,
						Trailer:          tc.trailer.Clone(),
						TransferEncoding: slices.Clone(tc.transfer),
						Body:             body,
						Request:          req,
					}, nil
				}),
			}
			options := []RequesterOption{WithBackoff(func(int) time.Duration { return 0 })}
			if tc.retryPolicy != nil {
				options = append(options, WithRetryPolicy(tc.retryPolicy))
			}
			reporter := &trackingReporter{}
			if tc.report {
				options = append(options, WithReporter(reporter))
			}
			requesterName := "test"
			if tc.record {
				requesterName = ""
			}
			r, err := New(requesterName, httpClient, options...)
			require.NoError(t, err, "New must not error")
			t.Cleanup(func() {
				assert.NoError(t, r.Shutdown(), "Shutdown should not error")
			})

			requestPath := "https://example.com"
			if tc.record {
				requestPath = "https://recording-user-canary:recording-password-canary@example.com/recording-request-path-canary?signature=recording-request-query-canary"
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, requestPath, http.NoBody)
			require.NoError(t, err, "http.NewRequestWithContext must not error")
			if tc.record {
				req.Header.Set("X-Recording-Request-Header-Canary", "recording-request-header-canary")
			}
			attempt := tc.attempt
			if attempt == 0 {
				attempt = 1
			}
			retry, err := r.executeRequest(t.Context(), &Item{
				Method:        http.MethodGet,
				Path:          requestPath,
				HTTPRecording: tc.record,
				HTTPDebugging: tc.debug,
			}, req, attempt, tc.verbose)
			require.Equal(t, tc.expectedRetry, retry, "executeRequest must return the expected retry decision")
			switch {
			case tc.expectedErr != nil:
				require.ErrorIs(t, err, tc.expectedErr, "executeRequest must return the expected error")
			case tc.errContains != "":
				require.ErrorContains(t, err, tc.errContains, "executeRequest must return the expected error")
			default:
				require.NoError(t, err, "executeRequest must not error")
			}
			if tc.record {
				for _, canary := range []string{"recording-user-canary", "recording-password-canary", "recording-request-path-canary", "recording-request-query-canary", "recording-request-header-canary", "recording-response-status-canary", "recording-response-header-canary", "recording-response-body-canary"} {
					assert.NotContains(t, err.Error(), canary, "executeRequest recording failures should omit request and response data")
				}
			}
			assert.Equal(t, 1, body.closeCalls, "executeRequest should close body exactly once")
			if tc.report {
				assert.Equal(t, 1, reporter.calls, "executeRequest should report one latency observation")
			}
		})
	}
}

func TestExecuteRequestResponseCloseDiagnostics(t *testing.T) {
	const (
		requesterName = "response-close-diagnostic-test"
		closeCanary   = "response-close-error-canary"
	)
	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable response close diagnostic capture")
	var logMu sync.Mutex
	var entries []string
	gctlog.SetCustomLogHook(func(_, _ string, a ...any) bool {
		entry := fmt.Sprint(a...)
		if !strings.Contains(entry, requesterName) {
			return true
		}
		logMu.Lock()
		entries = append(entries, entry)
		logMu.Unlock()
		return true
	})
	t.Cleanup(func() {
		gctlog.SetCustomLogHook(nil)
		assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable response close diagnostic capture")
	})

	body := &trackedReadCloser{Reader: strings.NewReader(`{"response":true}`), closeErr: errors.New(closeCanary)}
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
	})}
	r, err := New(requesterName, httpClient)
	require.NoError(t, err, "New must configure the response close diagnostic requester")
	t.Cleanup(func() {
		assert.NoError(t, r.Shutdown(), "Shutdown should close the response close diagnostic requester")
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", http.NoBody)
	require.NoError(t, err, "http.NewRequestWithContext must create the response close diagnostic request")

	retry, err := r.executeRequest(t.Context(), &Item{Method: http.MethodGet, Path: "https://example.com"}, req, 1, false)
	require.NoError(t, err, "executeRequest must ignore a response body close failure")
	assert.False(t, retry, "executeRequest should not retry after a response body close failure")
	assert.Equal(t, 1, body.closeCalls, "executeRequest should close the response body exactly once")
	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	logMu.Unlock()
	assert.Contains(t, diagnostics, requesterName+" failed to close response body (*errors.errorString)", "executeRequest should log response close error type metadata")
	assert.NotContains(t, diagnostics, closeCanary, "executeRequest response close diagnostics should omit error text")
}

func TestExecuteRequestSanitizesMockRecordingFailures(t *testing.T) {
	serviceDir, err := os.MkdirTemp("..", "request_test_http_recording_failure_") //nolint:usetesting // mock.HTTPRecord requires a sibling fixture directory.
	require.NoError(t, err, "os.MkdirTemp must create the recording service directory")
	recordingDir := filepath.Join(serviceDir, "testdata")
	recordingFile := filepath.Join(recordingDir, "http.json")
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(serviceDir), "os.RemoveAll must remove the recording service directory")
	})
	require.NoError(t, os.Mkdir(recordingDir, 0o755), "os.Mkdir must create the recording testdata directory")
	require.NoError(t, os.WriteFile(recordingFile, []byte(`{"routes":null}`), 0o600), "os.WriteFile must create the recording fixture")

	service := filepath.Base(serviceDir)
	requestBodyErr := &os.PathError{
		Op:   "record",
		Path: "recording-request-body-path-canary",
		Err:  errors.New("recording-request-body-error-canary"),
	}
	for _, tc := range []struct {
		name            string
		method          string
		response        string
		getBodyErr      error
		omittedCanaries []string
	}{
		{
			name:            "trailing response content",
			method:          http.MethodGet,
			response:        `{"response":true}{"recording-response-trailing-canary":true}`,
			omittedCanaries: []string{"recording-response-trailing-canary"},
		},
		{
			name:            "request body error",
			method:          http.MethodPost,
			response:        `{"response":true}`,
			getBodyErr:      requestBodyErr,
			omittedCanaries: []string{"recording-request-body-path-canary", "recording-request-body-error-canary"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedReadCloser{Reader: strings.NewReader(tc.response)}
			httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       body,
					Request:    req,
				}, nil
			})}
			r, err := New(service, httpClient)
			require.NoError(t, err, "New must configure the mock recording requester")
			t.Cleanup(func() {
				assert.NoError(t, r.Shutdown(), "Shutdown should close the mock recording requester")
			})

			req, err := http.NewRequestWithContext(t.Context(), tc.method, "https://example.com/record", http.NoBody)
			require.NoError(t, err, "http.NewRequestWithContext must create the mock recording request")
			if tc.getBodyErr != nil {
				req.GetBody = func() (io.ReadCloser, error) {
					return nil, tc.getBodyErr
				}
			}
			retry, err := r.executeRequest(t.Context(), &Item{
				Method:        tc.method,
				Path:          req.URL.String(),
				HTTPRecording: true,
			}, req, 1, false)
			require.Error(t, err, "executeRequest must return the mock recording failure")
			assert.False(t, retry, "executeRequest should not retry a mock recording failure")
			assert.Contains(t, err.Error(), "mock recording failure: operation failed (", "executeRequest error should retain safe mock recording context")
			for _, canary := range tc.omittedCanaries {
				assert.NotContains(t, err.Error(), canary, "executeRequest error should omit mock recording data")
			}
			if tc.getBodyErr != nil {
				require.ErrorIs(t, err, requestBodyErr, "executeRequest must preserve the mock recording error identity")
				var pathErr *os.PathError
				require.ErrorAs(t, err, &pathErr, "executeRequest must preserve mock recording error inspection")
				assert.Same(t, requestBodyErr, pathErr, "executeRequest should preserve the original mock recording error")
			}
			assert.Equal(t, 1, body.closeCalls, "executeRequest should close the response body exactly once")
		})
	}
}

func TestExecuteRequestRecordsBadStatusResponse(t *testing.T) {
	const rawResponse = `{"compatibility_body":"raw-bad-status-body-canary"}`
	serviceDir, err := os.MkdirTemp("..", "request_test_http_recording_") //nolint:usetesting // mock.HTTPRecord requires a sibling fixture directory.
	require.NoError(t, err, "os.MkdirTemp must create the recording service directory")
	recordingDir := filepath.Join(serviceDir, "testdata")
	recordingFile := filepath.Join(recordingDir, "http.json")
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(serviceDir), "os.RemoveAll must remove the recording service directory")
	})
	require.NoError(t, os.Mkdir(recordingDir, 0o755), "os.Mkdir must create the recording testdata directory")
	require.NoError(t, os.WriteFile(recordingFile, []byte(`{"routes":null}`), 0o600), "os.WriteFile must create the recording fixture")

	service := filepath.Base(serviceDir)
	body := &trackedReadCloser{Reader: strings.NewReader(rawResponse)}
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "400 Bad Request",
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       body,
				Request:    req,
			}, nil
		}),
	}
	r, err := New(service, httpClient)
	require.NoError(t, err, "New must not error")
	t.Cleanup(func() {
		assert.NoError(t, r.Shutdown(), "Shutdown should not error")
	})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/record", http.NoBody)
	require.NoError(t, err, "http.NewRequestWithContext must not error")
	retry, err := r.executeRequest(t.Context(), &Item{
		Method:        http.MethodGet,
		Path:          "https://example.com/record",
		HTTPRecording: true,
	}, req, 1, false)
	require.ErrorIs(t, err, ErrBadStatus, "executeRequest must return ErrBadStatus after recording the response")
	assert.ErrorContains(t, err, "raw-bad-status-body-canary", "executeRequest should retain the raw response body in ErrBadStatus display text")
	assert.False(t, retry, "executeRequest should not retry the bad status response")
	assert.Equal(t, 1, body.closeCalls, "executeRequest should close body exactly once")
	recorded, err := os.ReadFile(recordingFile)
	require.NoError(t, err, "os.ReadFile must read the recording fixture")
	assert.Contains(t, string(recorded), `"/record"`, "HTTPRecording should retain the request path before ErrBadStatus is returned")
	assert.Contains(t, string(recorded), "raw-bad-status-body-canary", "HTTPRecording should retain the raw response before ErrBadStatus is returned")
}

func TestDoRequestExecutesRetryDecision(t *testing.T) {
	t.Parallel()

	attempts := 0
	bodies := make([]*trackedReadCloser, 0, 2)
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			statusCode := http.StatusOK
			if attempts == 1 {
				statusCode = http.StatusTooManyRequests
			}
			body := &trackedReadCloser{Reader: strings.NewReader(`{"response":true}`)}
			bodies = append(bodies, body)
			return &http.Response{
				Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
				StatusCode: statusCode,
				Header:     make(http.Header),
				Body:       body,
				Request:    req,
			}, nil
		}),
	}
	r, err := New("test", httpClient, WithBackoff(func(int) time.Duration { return 0 }))
	require.NoError(t, err, "New must not error")
	t.Cleanup(func() {
		assert.NoError(t, r.Shutdown(), "Shutdown should not error")
	})

	err = r.doRequest(t.Context(), Unset, func() (*Item, error) {
		return &Item{
			Method:       http.MethodGet,
			Path:         "https://example.com",
			NonceEnabled: true,
		}, nil
	})
	require.NoError(t, err, "doRequest must not error")
	require.Len(t, bodies, 2, "bodies must contain one entry per request attempt")
	for _, body := range bodies {
		assert.Equal(t, 1, body.closeCalls, "doRequest should close body exactly once")
	}
}

func TestDoRequestReturnsExecutionError(t *testing.T) {
	t.Parallel()

	retryErr := errors.New("retry policy failure")
	body := &trackedReadCloser{Reader: strings.NewReader(`{"response":true}`)}
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       body,
				Request:    req,
			}, nil
		}),
	}
	r, err := New("test", httpClient, WithRetryPolicy(func(*http.Response, error) (bool, error) {
		return false, retryErr
	}))
	require.NoError(t, err, "New must not error")
	t.Cleanup(func() {
		assert.NoError(t, r.Shutdown(), "Shutdown should not error")
	})

	err = r.doRequest(t.Context(), Unset, func() (*Item, error) {
		return &Item{
			Method:       http.MethodGet,
			Path:         "https://example.com",
			NonceEnabled: true,
		}, nil
	})
	require.ErrorIs(t, err, retryErr, "doRequest must return the execution error")
	assert.Equal(t, 1, body.closeCalls, "doRequest should close body exactly once")
}

func TestExecuteRequestProcessesResponse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		statusCode     int
		payload        string
		initialResult  bool
		expectedResult bool
		debug          bool
		expectError    bool
	}{
		{
			name:           "decoded response with headers and debugging",
			statusCode:     http.StatusOK,
			payload:        `{"response":true}`,
			expectedResult: true,
			debug:          true,
		},
		{
			name:           "no content preserves result",
			statusCode:     http.StatusNoContent,
			initialResult:  true,
			expectedResult: true,
		},
		{
			name:        "invalid response",
			statusCode:  http.StatusOK,
			payload:     `{`,
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := &trackedReadCloser{Reader: strings.NewReader(tc.payload)}
			httpClient := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     fmt.Sprintf("%d %s", tc.statusCode, http.StatusText(tc.statusCode)),
						StatusCode: tc.statusCode,
						Header: http.Header{
							"X-Test": {"value"},
						},
						Body:    body,
						Request: req,
					}, nil
				}),
			}
			r, err := New("test", httpClient)
			require.NoError(t, err, "New must not error")
			t.Cleanup(func() {
				assert.NoError(t, r.Shutdown(), "Shutdown should not error")
			})

			result := struct {
				Response bool `json:"response"`
			}{Response: tc.initialResult}
			headerResponse := make(http.Header)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", http.NoBody)
			require.NoError(t, err, "http.NewRequestWithContext must not error")
			retry, err := r.executeRequest(t.Context(), &Item{
				Method:         http.MethodGet,
				Path:           "https://example.com",
				Result:         &result,
				HeaderResponse: &headerResponse,
				HTTPDebugging:  tc.debug,
			}, req, 1, false)
			require.False(t, retry, "executeRequest must not retry")
			if tc.expectError {
				require.Error(t, err, "executeRequest must return the response decoding error")
			} else {
				require.NoError(t, err, "executeRequest must not error")
			}
			assert.Equal(t, tc.expectedResult, result.Response, "executeRequest should return the expected response result")
			assert.Equal(t, []string{"value"}, headerResponse.Values("X-Test"), "executeRequest should return the expected response headers")
			assert.Equal(t, 1, body.closeCalls, "executeRequest should close body exactly once")
		})
	}
}

func TestExecuteRequestTransportError(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("transport failure")
	for _, tc := range []struct {
		name          string
		retryPolicy   RetryPolicy
		expectedRetry bool
		expectedErr   error
	}{
		{
			name:        "default policy returns transport error",
			expectedErr: transportErr,
		},
		{
			name: "custom policy declines transport error",
			retryPolicy: func(*http.Response, error) (bool, error) {
				return false, nil
			},
			expectedErr: transportErr,
		},
		{
			name: "custom policy retries transport error",
			retryPolicy: func(*http.Response, error) (bool, error) {
				return true, nil
			},
			expectedRetry: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reporter := &trackingReporter{}
			httpClient := &http.Client{
				Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return nil, transportErr
				}),
			}
			options := []RequesterOption{
				WithBackoff(func(int) time.Duration { return 0 }),
				WithReporter(reporter),
			}
			if tc.retryPolicy != nil {
				options = append(options, WithRetryPolicy(tc.retryPolicy))
			}
			r, err := New("test", httpClient, options...)
			require.NoError(t, err, "New must not error")
			t.Cleanup(func() {
				assert.NoError(t, r.Shutdown(), "Shutdown should not error")
			})

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", http.NoBody)
			require.NoError(t, err, "http.NewRequestWithContext must not error")
			retry, err := r.executeRequest(t.Context(), &Item{Method: http.MethodGet, Path: "https://example.com"}, req, 1, false)
			require.Equal(t, tc.expectedRetry, retry, "executeRequest must return the correct retry decision")
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr, "executeRequest must return the transport error")
			} else {
				require.NoError(t, err, "executeRequest must not error")
			}
			assert.Zero(t, reporter.calls, "reporter.calls should remain zero")
		})
	}
}

func TestExecuteRequestRedirectError(t *testing.T) {
	t.Parallel()

	redirectErr := errors.New("redirect failure")
	body := &trackedReadCloser{Reader: strings.NewReader("redirect response")}
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "302 Found",
				StatusCode: http.StatusFound,
				Header: http.Header{
					"Location": {"https://example.net"},
				},
				Body:    body,
				Request: req,
			}, nil
		}),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return redirectErr
		},
	}
	r, err := New("test", httpClient, WithRetryPolicy(func(*http.Response, error) (bool, error) {
		return false, nil
	}))
	require.NoError(t, err, "New must not error")
	t.Cleanup(func() {
		assert.NoError(t, r.Shutdown(), "Shutdown should not error")
	})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", http.NoBody)
	require.NoError(t, err, "http.NewRequestWithContext must not error")
	retry, err := r.executeRequest(t.Context(), &Item{Method: http.MethodGet, Path: "https://example.com"}, req, 1, false)
	require.ErrorIs(t, err, redirectErr, "executeRequest must return the redirect error")
	require.False(t, retry, "executeRequest must not retry")
	assert.Equal(t, 1, body.closeCalls, "executeRequest should close body exactly once")
}

func TestExecuteRequestVerboseRequestBody(t *testing.T) {
	t.Parallel()

	getBodyErr := errors.New("get request body failure")
	responseBody := &trackedReadCloser{Reader: strings.NewReader(`{"response":true}`)}
	transportCalls := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		transportCalls++
		return &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: make(http.Header), Body: responseBody, Request: req}, nil
	})}
	r, err := New("test", httpClient)
	require.NoError(t, err, "New must not error")
	t.Cleanup(func() { assert.NoError(t, r.Shutdown(), "Shutdown should not error") })

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com", http.NoBody)
	require.NoError(t, err, "http.NewRequestWithContext must not error")
	getBodyCalls := 0
	req.GetBody = func() (io.ReadCloser, error) {
		getBodyCalls++
		return nil, getBodyErr
	}
	retry, err := r.executeRequest(t.Context(), &Item{Method: http.MethodPost, Path: "https://example.com"}, req, 1, true)
	require.NoError(t, err, "executeRequest must not inspect the diagnostic body copy")
	assert.False(t, retry, "executeRequest should not retry a successful response")
	assert.Zero(t, getBodyCalls, "executeRequest should not call GetBody for diagnostics")
	assert.Equal(t, 1, transportCalls, "executeRequest should execute the transport once")
	assert.Equal(t, 1, responseBody.closeCalls, "executeRequest should close the response body once")
}

func TestRequesterDiagnostics(t *testing.T) {
	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable diagnostic capture")
	var logMu sync.Mutex
	var entries []string
	gctlog.SetCustomLogHook(func(_, _ string, a ...any) bool {
		entry := fmt.Sprint(a...)
		if !strings.Contains(entry, "diagnostic-test") {
			return true
		}
		logMu.Lock()
		entries = append(entries, entry)
		logMu.Unlock()
		return true
	})
	t.Cleanup(func() {
		gctlog.SetCustomLogHook(nil)
		assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable diagnostic capture")
	})

	for _, tc := range []struct {
		name          string
		verbose       bool
		httpDebugging bool
	}{
		{name: "verbose", verbose: true},
		{name: "HTTP debugging", httpDebugging: true},
		{name: "verbose and HTTP debugging", verbose: true, httpDebugging: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logMu.Lock()
			entries = nil
			logMu.Unlock()
			transportCalls := 0
			httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				transportCalls++
				requestBody, err := io.ReadAll(req.Body)
				require.NoError(t, err, "transport must read the request body")
				assert.Equal(t, "request-body-token", string(requestBody), "transport should receive the request body unchanged")
				assert.Equal(t, "header-value-token", req.Header.Get("X-Header-Name-Token"), "transport should receive request headers unchanged")
				return &http.Response{
					Status:     "200 status-reason-token",
					StatusCode: http.StatusOK,
					Header:     http.Header{"X-Response-Header-Token": {"response-header-value-token"}},
					Body:       io.NopCloser(strings.NewReader("response-body-token")),
					Request:    req,
				}, nil
			})}
			r, err := New("diagnostic-test", httpClient)
			require.NoError(t, err, "New must configure the diagnostic requester")
			t.Cleanup(func() { assert.NoError(t, r.Shutdown(), "Shutdown should not error") })

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://user-token:password-token@example.com/path-token?signature=query-token", strings.NewReader("request-body-token"))
			require.NoError(t, err, "http.NewRequestWithContext must create the diagnostic request")
			req.Header.Set("X-Header-Name-Token", "header-value-token")
			getBodyCalls := 0
			req.GetBody = func() (io.ReadCloser, error) {
				getBodyCalls++
				return nil, errors.New("get-body-token")
			}
			retry, err := r.executeRequest(t.Context(), &Item{
				Method:        http.MethodPost,
				Path:          req.URL.String(),
				HTTPDebugging: tc.httpDebugging,
			}, req, 2, tc.verbose)
			require.NoError(t, err, "executeRequest must complete the diagnostic request")
			assert.False(t, retry, "executeRequest should not retry a successful response")
			assert.Zero(t, getBodyCalls, "executeRequest should not call GetBody for diagnostics")
			assert.Equal(t, 1, transportCalls, "executeRequest should execute the transport once")

			logMu.Lock()
			diagnostics := strings.Join(entries, "\n")
			entryCount := len(entries)
			logMu.Unlock()
			assert.Equal(t, 2, entryCount, "executeRequest should emit one request and one response diagnostic")
			assert.Contains(t, diagnostics, "attempt=2", "request diagnostics should include the attempt")
			assert.Contains(t, diagnostics, "method=POST", "request diagnostics should include the method")
			assert.Contains(t, diagnostics, "endpoint=https://example.com", "request diagnostics should include the safe endpoint")
			assert.Contains(t, diagnostics, "status=200", "response diagnostics should include the status code")
			assert.Contains(t, diagnostics, "body-bytes=19", "response diagnostics should include the body length")
			for _, secret := range []string{"user-token", "password-token", "path-token", "query-token", "Header-Name-Token", "header-value-token", "request-body-token", "status-reason-token", "Response-Header-Token", "response-header-value-token", "response-body-token", "get-body-token"} {
				assert.NotContains(t, diagnostics, secret, "request diagnostics should omit wire data")
			}
		})
	}

	logMu.Lock()
	entries = nil
	logMu.Unlock()
	r := Requester{name: "diagnostic-test", retryPolicy: func(*http.Response, error) (bool, error) { return true, nil }, backoff: func(int) time.Duration { return 0 }, maxRetries: 2}
	retry, err := r.evaluateRetry(t.Context(), nil, errors.New("retry-cause-token"), 1, true)
	require.NoError(t, err, "evaluateRetry must allow the configured retry")
	assert.True(t, retry, "evaluateRetry should retry the request")
	r.drainBody(&trackedReadCloser{Reader: iotest.ErrReader(errors.New("drain-cause-token")), closeErr: errors.New("close-cause-token")})
	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	logMu.Unlock()
	assert.Contains(t, diagnostics, "cause=operation failed (*errors.errorString)", "retry diagnostics should include safe error type metadata")
	for _, secret := range []string{"retry-cause-token", "drain-cause-token", "close-cause-token"} {
		assert.NotContains(t, diagnostics, secret, "retry and cleanup diagnostics should omit error text")
	}
}

func TestExecuteRequestHTTPDebuggingRetryDiagnostics(t *testing.T) {
	retryCanary := "retry-transport-secret-token"
	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable retry diagnostic capture")
	var logMu sync.Mutex
	var entries []string
	gctlog.SetCustomLogHook(func(_, _ string, a ...any) bool {
		entry := fmt.Sprint(a...)
		if !strings.Contains(entry, "diagnostic-retry-test") {
			return true
		}
		logMu.Lock()
		entries = append(entries, entry)
		logMu.Unlock()
		return true
	})
	t.Cleanup(func() {
		gctlog.SetCustomLogHook(nil)
		assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable retry diagnostic capture")
	})

	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New(retryCanary)
	})}
	r, err := New("diagnostic-retry-test", httpClient,
		WithRetryPolicy(func(*http.Response, error) (bool, error) { return true, nil }),
		WithBackoff(func(int) time.Duration { return 0 }))
	require.NoError(t, err, "New must configure the retry diagnostic requester")
	t.Cleanup(func() { assert.NoError(t, r.Shutdown(), "Shutdown should close the retry diagnostic requester") })

	endpoint := "https://retry-user-token:retry-password-token@example.com/retry-path-token?signature=retry-query-token"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, http.NoBody)
	require.NoError(t, err, "http.NewRequestWithContext must create the retry diagnostic request")
	retry, err := r.executeRequest(t.Context(), &Item{Method: http.MethodGet, Path: endpoint, HTTPDebugging: true}, req, 1, false)
	require.NoError(t, err, "executeRequest must allow the HTTP debugging retry")
	assert.True(t, retry, "executeRequest should retry when HTTP debugging is enabled")

	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	logMu.Unlock()
	assert.Contains(t, diagnostics, "HTTP request attempt=1 method=GET endpoint=https://example.com", "executeRequest should log HTTP debugging request metadata")
	assert.Contains(t, diagnostics, "HTTP request failed; retrying in 0s, attempt 1, cause=request to https://example.com failed", "executeRequest should log HTTP debugging retry metadata")
	for _, secret := range []string{retryCanary, "retry-user-token", "retry-password-token", "retry-path-token", "retry-query-token"} {
		assert.NotContains(t, diagnostics, secret, "executeRequest retry diagnostics should omit transport and URL data")
	}
}

func TestDefaultRetryPolicyPoisonedErrors(t *testing.T) {
	t.Parallel()

	var typedNil *net.DNSError
	urlCycle := new(url.Error)
	urlCycle.Err = urlCycle
	safeURLTimeout := logsafe.URLRequestError(&url.Error{URL: "https://example.com/path-token", Err: &net.DNSError{IsTimeout: true}})
	for _, tc := range []struct {
		name          string
		err           error
		expectedRetry bool
	}{
		{name: "typed nil", err: typedNil},
		{name: "URL cycle", err: urlCycle},
		{name: "timeout only", err: new(requestTestTimeoutOnlyError)},
		{name: "wrapped timeout", err: fmt.Errorf("wrapped timeout: %w", &net.DNSError{IsTimeout: true})},
		{name: "joined timeout", err: errors.Join(errors.New("joined-error-token"), &net.DNSError{IsTimeout: true})},
		{name: "panicking timeout", err: new(requestTestPanickingNetError)},
		{name: "safe URL timeout", err: safeURLTimeout, expectedRetry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var (
				retry       bool
				returnedErr error
			)
			assert.NotPanics(t, func() {
				retry, returnedErr = DefaultRetryPolicy(nil, tc.err)
			}, "DefaultRetryPolicy should not panic while classifying a transport error")
			assert.Equal(t, tc.expectedRetry, retry, "DefaultRetryPolicy should return the correct retry decision")
			if tc.expectedRetry {
				assert.NoError(t, returnedErr, "DefaultRetryPolicy should not error for a timeout")
			} else {
				assert.ErrorIs(t, returnedErr, tc.err, "DefaultRetryPolicy should return the original non-timeout error")
			}
		})
	}
}

func TestEvaluateRetrySanitizesURLErrors(t *testing.T) {
	t.Parallel()

	assertSafeError := func(t *testing.T, err error, original *url.Error, cause error) {
		t.Helper()
		require.ErrorIs(t, err, original, "evaluateRetry must preserve URL error identity")
		require.ErrorIs(t, err, cause, "evaluateRetry must preserve the URL error cause")
		var target *url.Error
		require.ErrorAs(t, err, &target, "evaluateRetry must preserve URL error inspection")
		assert.Same(t, original, target, "evaluateRetry should preserve the original URL error through errors.As")
		assert.Contains(t, err.Error(), "https://example.com", "evaluateRetry should retain safe endpoint metadata")
		for _, secret := range []string{"user-token", "password-token", "path-token", "query-token", cause.Error()} {
			assert.NotContains(t, err.Error(), secret, "evaluateRetry should omit URL and cause text")
		}
	}

	t.Run("retry declined", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("transport-cause-token")
		original := &url.Error{Op: http.MethodGet, URL: "https://user-token:password-token@example.com/path-token?signature=query-token", Err: cause}
		r := Requester{retryPolicy: DefaultRetryPolicy}
		retry, err := r.evaluateRetry(t.Context(), nil, original, 1, false)
		assert.False(t, retry, "evaluateRetry should not retry a non-timeout transport error")
		assertSafeError(t, err, original, cause)
	})

	t.Run("retry forbidden", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("retry-forbidden-cause-token")
		original := &url.Error{Op: http.MethodGet, URL: "https://user-token:password-token@example.com/path-token?signature=query-token", Err: cause}
		retryPolicyCalled := false
		r := Requester{retryPolicy: func(*http.Response, error) (bool, error) {
			retryPolicyCalled = true
			return true, nil
		}}
		retry, err := r.evaluateRetry(WithRetryNotAllowed(t.Context()), nil, original, 1, false)
		assert.False(t, retry, "evaluateRetry should not retry when retry is forbidden")
		assert.False(t, retryPolicyCalled, "evaluateRetry should not call retryPolicy when retry is forbidden")
		assertSafeError(t, err, original, cause)
	})

	t.Run("retry policy error", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("retry-policy-cause-token")
		original := &url.Error{Op: http.MethodGet, URL: "https://user-token:password-token@example.com/path-token?signature=query-token", Err: cause}
		body := &trackedReadCloser{Reader: strings.NewReader("response")}
		r := Requester{retryPolicy: func(*http.Response, error) (bool, error) { return false, original }}
		retry, err := r.evaluateRetry(t.Context(), &http.Response{Body: body}, nil, 1, false)
		assert.False(t, retry, "evaluateRetry should not retry after a retry policy error")
		assert.Equal(t, 1, body.closeCalls, "evaluateRetry should close the response body after a retry policy error")
		assertSafeError(t, err, original, cause)
	})
}

func TestValidateRequestSanitizesURL(t *testing.T) {
	t.Parallel()

	const malformedURL = "https://malformed-user-token:malformed-password-token@example.com/malformed-path-token%zz?signature=malformed-query-token"
	item := &Item{Method: http.MethodGet, Path: malformedURL, NonceEnabled: true}
	_, err := item.validateRequest(t.Context(), &Requester{})
	malformedCause := url.EscapeError("%zz")
	require.ErrorIs(t, err, malformedCause, "validateRequest must preserve the malformed URL cause")
	var malformedURLError *url.Error
	require.ErrorAs(t, err, &malformedURLError, "validateRequest must preserve malformed URL error inspection")
	assert.Equal(t, malformedURL, malformedURLError.URL, "validateRequest should preserve the malformed URL through errors.As")
	assert.Contains(t, err.Error(), "[redacted URL]", "validateRequest should fail closed when the endpoint cannot be parsed safely")
	for _, secret := range []string{"malformed-user-token", "malformed-password-token", "malformed-path-token", "malformed-query-token", "%zz", "invalid URL escape"} {
		assert.NotContains(t, err.Error(), secret, "validateRequest should omit malformed URL and cause text")
	}
}

func TestValidateRequestHTTPDebuggingDoesNotReadBody(t *testing.T) {
	require.NoError(t, gctlog.SetGlobalLogConfig(gctlog.GenDefaultSettings()), "SetGlobalLogConfig must enable validation diagnostic capture")
	var logMu sync.Mutex
	var entries []string
	gctlog.SetCustomLogHook(func(_, _ string, a ...any) bool {
		logMu.Lock()
		entries = append(entries, fmt.Sprint(a...))
		logMu.Unlock()
		return true
	})
	t.Cleanup(func() {
		gctlog.SetCustomLogHook(nil)
		assert.NoError(t, gctlog.SetGlobalLogConfig(&gctlog.Config{}), "SetGlobalLogConfig should disable validation diagnostic capture")
	})

	const bodyCanary = "validation-request-body-canary"
	body := &countingReader{Reader: strings.NewReader(bodyCanary)}
	item := &Item{
		Method:        http.MethodPost,
		Path:          "https://validation-user-canary:validation-password-canary@example.com/validation-path-canary?signature=validation-query-canary",
		Headers:       map[string]string{"X-Validation-Header-Canary": "validation-header-canary"},
		Body:          body,
		NonceEnabled:  true,
		HTTPDebugging: true,
	}
	req, err := item.validateRequest(t.Context(), &Requester{})
	require.NoError(t, err, "validateRequest must accept the HTTP debugging request")
	assert.Zero(t, body.readCalls, "validateRequest should not read the request body for HTTP debugging")
	logMu.Lock()
	diagnostics := strings.Join(entries, "\n")
	entryCount := len(entries)
	logMu.Unlock()
	assert.Zero(t, entryCount, "validateRequest should not emit HTTP debugging diagnostics")
	for _, canary := range []string{"validation-user-canary", "validation-password-canary", "validation-path-canary", "validation-query-canary", "Validation-Header-Canary", "validation-header-canary", bodyCanary} {
		assert.NotContains(t, diagnostics, canary, "validateRequest should not dump request wire data")
	}
	payload, err := io.ReadAll(req.Body)
	require.NoError(t, err, "io.ReadAll must not error when reading req.Body after validateRequest")
	assert.Equal(t, bodyCanary, string(payload), "req.Body should retain the complete request payload")
	assert.Positive(t, body.readCalls, "body.readCalls should increase when req.Body is consumed")
	require.NoError(t, req.Body.Close(), "req.Body.Close must not error")
}

func TestDoRequest_NoContent(t *testing.T) {
	t.Parallel()
	newRequester := func(t *testing.T) *Requester {
		t.Helper()
		r, err := New("test", common.NewHTTPClientWithTimeout(0), WithLimiter(globalshell))
		require.NoError(t, err, "New requester must not error")
		t.Cleanup(func() { assert.NoError(t, r.Shutdown(), "Shutdown should not error") })
		return r
	}

	scenarioTests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "no content zero value result remains unchanged",
			run: func(t *testing.T) {
				t.Helper()
				r := newRequester(t)
				var resp struct {
					Response bool `json:"response"`
				}
				err := r.SendPayload(t.Context(), UnAuth, func() (*Item, error) {
					return &Item{
						Method: http.MethodPost,
						Path:   testURL + "/nocontent",
						Result: &resp,
					}, nil
				}, UnauthenticatedRequest)
				require.NoError(t, err, "SendPayload must not error on 204 No Content")
				assert.False(t, resp.Response, "result should remain zero value on 204 No Content")
			},
		},
		{
			name: "no content pre populated result remains unchanged",
			run: func(t *testing.T) {
				t.Helper()
				r := newRequester(t)
				resp := struct {
					Response bool `json:"response"`
				}{
					Response: true,
				}
				err := r.SendPayload(t.Context(), UnAuth, func() (*Item, error) {
					return &Item{
						Method: http.MethodPost,
						Path:   testURL + "/nocontent",
						Result: &resp,
					}, nil
				}, UnauthenticatedRequest)
				require.NoError(t, err, "SendPayload must not error on 204 No Content")
				assert.True(t, resp.Response, "result should remain unchanged when unmarshalling is skipped on 204 No Content")
			},
		},
		{
			name: "no content nil result must not error",
			run: func(t *testing.T) {
				t.Helper()
				r := newRequester(t)
				err := r.SendPayload(t.Context(), UnAuth, func() (*Item, error) {
					return &Item{
						Method: http.MethodPost,
						Path:   testURL + "/nocontent",
					}, nil
				}, UnauthenticatedRequest)
				require.NoError(t, err, "SendPayload must not error on 204 No Content with nil Result")
			},
		},
		{
			name: "no content header response must be copied",
			run: func(t *testing.T) {
				t.Helper()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-No-Content", "true")
					w.WriteHeader(http.StatusNoContent)
				}))
				t.Cleanup(server.Close)

				r := newRequester(t)
				headers := http.Header{}
				var resp struct {
					Response bool `json:"response"`
				}
				err := r.SendPayload(t.Context(), UnAuth, func() (*Item, error) {
					return &Item{
						Method:         http.MethodPost,
						Path:           server.URL,
						Result:         &resp,
						HeaderResponse: &headers,
					}, nil
				}, UnauthenticatedRequest)
				require.NoError(t, err, "SendPayload must not error on 204 No Content with headers")
				assert.Equal(t, "true", headers.Get("X-No-Content"), "HeaderResponse should contain custom headers for 204 No Content")
				assert.Equal(t, "application/json", headers.Get("Content-Type"), "HeaderResponse should contain Content-Type for 204 No Content")
				assert.False(t, resp.Response, "result should remain zero value when 204 No Content has headers")
			},
		},
	}
	for _, tc := range scenarioTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func TestDoRequest_Retries(t *testing.T) {
	t.Parallel()

	r, err := New("test", new(http.Client), WithBackoff(func(int) time.Duration { return 0 }))
	require.NoError(t, err, "New requester must not error")

	var ec common.ErrorCollector
	for range 4 {
		ec.Go(func() error {
			var resp struct {
				Response bool `json:"response"`
			}
			itemFn := func() (*Item, error) {
				return &Item{
					Method: http.MethodGet,
					Path:   testURL + "/rate-retry",
					Result: &resp,
				}, nil
			}

			if err := r.SendPayload(t.Context(), Auth, itemFn, AuthenticatedRequest); err != nil {
				return fmt.Errorf("SendPayload error: %w", err)
			}
			if !resp.Response {
				return fmt.Errorf("unexpected response: %+v", resp)
			}
			return nil
		})
	}

	require.NoError(t, ec.Collect(), "Collect must return no errors")
}

func TestDoRequest_RetryNonRecoverable(t *testing.T) {
	t.Parallel()

	backoff := func(int) time.Duration {
		return 0
	}
	r, err := New("test", new(http.Client), WithBackoff(backoff))
	if err != nil {
		t.Fatal(err)
	}
	err = r.SendPayload(t.Context(), Unset, func() (*Item, error) {
		return &Item{
			Method: http.MethodGet,
			Path:   testURL + "/always-retry",
		}, nil
	}, UnauthenticatedRequest)
	require.ErrorIs(t, err, errFailedToRetryRequest)
}

func TestDoRequest_NotRetryable(t *testing.T) {
	t.Parallel()

	notRetryErr := errors.New("not retryable")
	retry := func(*http.Response, error) (bool, error) {
		return false, notRetryErr
	}
	backoff := func(n int) time.Duration {
		return time.Duration(n) * time.Millisecond
	}
	r, err := New("test", new(http.Client), WithRetryPolicy(retry), WithBackoff(backoff))
	if err != nil {
		t.Fatal(err)
	}
	err = r.SendPayload(t.Context(), Unset, func() (*Item, error) {
		return &Item{
			Method: http.MethodGet,
			Path:   testURL + "/always-retry",
		}, nil
	}, UnauthenticatedRequest)
	require.ErrorIs(t, err, notRetryErr)
}

func TestEvaluateRetry(t *testing.T) {
	t.Parallel()

	r := Requester{}
	retry, err := r.evaluateRetry(WithRetryNotAllowed(t.Context()), nil, errInvalidPath, 1, false)
	require.ErrorIs(t, err, errInvalidPath, "must return incoming error when retry not allowed")
	require.False(t, retry, "must not retry when retry not allowed")

	retryPolicyErr := errors.New("retry policy failure")
	responseBody := &trackedReadCloser{Reader: strings.NewReader("response")}
	r.retryPolicy = func(*http.Response, error) (bool, error) {
		return false, retryPolicyErr
	}
	retry, err = r.evaluateRetry(t.Context(), &http.Response{Body: responseBody}, nil, 1, false)
	require.ErrorIs(t, err, retryPolicyErr, "evaluateRetry must return the retry policy error")
	require.False(t, retry, "evaluateRetry must not retry after a retry policy error")
	assert.Equal(t, 1, responseBody.closeCalls, "evaluateRetry should close responseBody exactly once after a retry policy error")

	retry, err = r.evaluateRetry(t.Context(), nil, nil, 1, false)
	require.ErrorIs(t, err, retryPolicyErr, "evaluateRetry must return the retry policy error without a response")
	require.False(t, retry, "evaluateRetry must not retry after a retry policy error without a response")

	transportErr := errors.New("transport failure")
	responseBody = &trackedReadCloser{Reader: strings.NewReader("response")}
	retry, err = r.evaluateRetry(t.Context(), &http.Response{Body: responseBody}, transportErr, 1, false)
	require.ErrorIs(t, err, retryPolicyErr, "evaluateRetry must return the retry policy error after a transport error")
	require.False(t, retry, "evaluateRetry must not retry after a retry policy and transport error")
	assert.Zero(t, responseBody.closeCalls, "evaluateRetry should leave responseBody open after a transport error")
	require.NoError(t, responseBody.Close(), "responseBody.Close must not error")

	drainErr := errors.New("response drain failure")
	drainCloseErr := errors.New("response close failure")
	responseBody = &trackedReadCloser{Reader: iotest.ErrReader(drainErr), closeErr: drainCloseErr}
	retry, err = r.evaluateRetry(t.Context(), &http.Response{Body: responseBody}, nil, 1, false)
	require.ErrorIs(t, err, retryPolicyErr, "evaluateRetry must return the retry policy error after drain and close failures")
	require.False(t, retry, "evaluateRetry must not retry after drain and close failures")
	assert.Equal(t, 1, responseBody.closeCalls, "evaluateRetry should close responseBody exactly once after drain and close failures")

	r.retryPolicy = func(*http.Response, error) (bool, error) {
		return false, nil
	}
	retry, err = r.evaluateRetry(t.Context(), nil, transportErr, 1, false)
	require.ErrorIs(t, err, transportErr, "evaluateRetry must return the transport error when retrying is declined")
	require.False(t, retry, "evaluateRetry must not retry when the retry policy declines")

	r.retryPolicy = DefaultRetryPolicy
	retry, err = r.evaluateRetry(t.Context(), nil, errInvalidPath, 1, false)
	require.ErrorIs(t, err, errInvalidPath, "must return incoming error when using default retry policy")
	require.False(t, retry, "must not retry when using default retry policy and the error is non-timeout")

	retry, err = r.evaluateRetry(t.Context(), &http.Response{StatusCode: http.StatusOK}, nil, 1, false)
	require.NoError(t, err, "must not error when response is OK")
	require.False(t, retry, "must not retry on 200 status")

	errTimeout := &net.DNSError{IsTimeout: true}
	retry, err = r.evaluateRetry(t.Context(), nil, errTimeout, 1, false)
	require.ErrorIs(t, err, errFailedToRetryRequest, "must return error when attempt is higher than max retries")
	require.ErrorIs(t, err, errExceedsMaxRetries, "must return error when attempt is higher than max retries")
	require.ErrorIs(t, err, errTimeout, "must wrap original error")
	require.False(t, retry, "must not retry when max attempts exceeded")

	maxRetryBody := &trackedReadCloser{Reader: strings.NewReader("")}
	retry, err = r.evaluateRetry(t.Context(), &http.Response{StatusCode: http.StatusTooManyRequests, Status: "429 status-reason-token", Body: maxRetryBody}, nil, 1, false)
	require.ErrorContains(t, err, "failed to retry request exceeds maximum retry attempts: status 429", "evaluateRetry must return the status code when maximum retries are exceeded")
	assert.NotContains(t, err.Error(), "status-reason-token", "evaluateRetry should omit response status text")
	require.False(t, retry, "must not retry when max attempts exceeded")
	assert.Equal(t, 1, maxRetryBody.closeCalls, "evaluateRetry should close maxRetryBody exactly once after maximum retries")

	r.maxRetries = 1
	r.backoff = func(int) time.Duration { return time.Millisecond * 10 }
	ctx, cancel := context.WithDeadline(t.Context(), time.Now())
	defer cancel()
	retry, err = r.evaluateRetry(ctx, nil, errTimeout, 1, false)
	require.ErrorIs(t, err, errFailedToRetryRequest, "must return error when deadline would be exceeded")
	require.ErrorIs(t, err, context.DeadlineExceeded, "must return error when deadline would be exceeded")
	require.ErrorIs(t, err, errTimeout, "must wrap original error")
	require.False(t, retry, "must not retry when deadline would be exceeded")

	deadlineBody := &trackedReadCloser{Reader: strings.NewReader("")}
	retry, err = r.evaluateRetry(ctx, &http.Response{StatusCode: http.StatusTooManyRequests, Status: "429 status-reason-token", Body: deadlineBody}, nil, 1, false)
	require.ErrorContains(t, err, "failed to retry request context deadline exceeded: status 429", "evaluateRetry must return the status code when the retry delay exceeds the context deadline")
	assert.NotContains(t, err.Error(), "status-reason-token", "evaluateRetry should omit response status text")
	require.False(t, retry, "must not retry when deadline would be exceeded")
	assert.Equal(t, 1, deadlineBody.closeCalls, "evaluateRetry should close deadlineBody exactly once when the deadline would be exceeded")

	ctx, cancel = context.WithCancel(t.Context())
	cancel()
	cancelledBody := &trackedReadCloser{Reader: strings.NewReader("")}
	retry, err = r.evaluateRetry(ctx, &http.Response{StatusCode: http.StatusTooManyRequests, Status: "429", Body: cancelledBody}, nil, 1, true)
	require.ErrorIs(t, err, errFailedToRetryRequest, "must return error when context is cancelled")
	require.ErrorIs(t, err, context.Canceled, "must return error when context is cancelled")
	require.False(t, retry, "must not retry when context is cancelled")
	assert.Equal(t, 1, cancelledBody.closeCalls, "evaluateRetry should close cancelledBody exactly once when the context is cancelled")

	retryBody := &trackedReadCloser{Reader: strings.NewReader("")}
	retry, err = r.evaluateRetry(t.Context(), &http.Response{StatusCode: http.StatusTooManyRequests, Status: "429", Body: retryBody}, nil, 1, true)
	require.NoError(t, err, "must not error")
	require.True(t, retry, "must retry on 429 response")
	assert.Equal(t, 1, retryBody.closeCalls, "evaluateRetry should close retryBody exactly once before retrying")

	r.backoff = func(int) time.Duration { return 0 }
	retryBody = &trackedReadCloser{Reader: iotest.ErrReader(drainErr), closeErr: drainCloseErr}
	retry, err = r.evaluateRetry(t.Context(), &http.Response{StatusCode: http.StatusTooManyRequests, Status: "429", Body: retryBody}, nil, 1, true)
	require.NoError(t, err, "evaluateRetry must not error after drain and close failures")
	require.True(t, retry, "evaluateRetry must retry after drain and close failures")
	assert.Equal(t, 1, retryBody.closeCalls, "evaluateRetry should close retryBody exactly once after drain and close failures")

	retry, err = r.evaluateRetry(t.Context(), nil, errTimeout, 1, true)
	require.NoError(t, err, "must not error")
	require.True(t, retry, "must retry on timeout error")
}

func TestGetNonce(t *testing.T) {
	t.Parallel()
	r, err := New("test", new(http.Client), WithLimiter(globalshell))
	require.NoError(t, err)
	n1 := r.GetNonce(nonce.Unix)
	assert.NotZero(t, n1)
	n2 := r.GetNonce(nonce.Unix)
	assert.NotZero(t, n2)
	assert.NotEqual(t, n1, n2)

	r2, err := New("test", new(http.Client), WithLimiter(globalshell))
	require.NoError(t, err)
	n3 := r2.GetNonce(nonce.UnixNano)
	assert.NotZero(t, n3)
	n4 := r2.GetNonce(nonce.UnixNano)
	assert.NotZero(t, n4)
	assert.NotEqual(t, n3, n4)

	assert.NotEqual(t, n1, n3)
	assert.NotEqual(t, n2, n4)
}

// 40532461	       30.29 ns/op	       0 B/op	       0 allocs/op (prev)
// 45329203	       26.53 ns/op	       0 B/op	       0 allocs/op
func BenchmarkGetNonce(b *testing.B) {
	r, err := New("test", new(http.Client), WithLimiter(globalshell))
	require.NoError(b, err)
	for b.Loop() {
		r.GetNonce(nonce.UnixNano)
		r.timedLock.UnlockIfLocked()
	}
}

func TestSetProxy(t *testing.T) {
	t.Parallel()
	var r *Requester
	err := r.SetProxy(nil)
	require.ErrorIs(t, err, ErrRequestSystemIsNil)

	r, err = New("test", &http.Client{Transport: new(http.Transport)}, WithLimiter(globalshell))
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse("http://www.google.com")
	if err != nil {
		t.Fatal(err)
	}
	err = r.SetProxy(u)
	if err != nil {
		t.Fatal(err)
	}
	u, err = url.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	err = r.SetProxy(u)
	if err == nil {
		t.Fatal("error cannot be nil")
	}
}

func TestBasicLimiter(t *testing.T) {
	r, err := New("test", new(http.Client), WithLimiter(NewBasicRateLimit(time.Second, 1, 1)))
	if err != nil {
		t.Fatal(err)
	}
	i := Item{Path: "http://www.google.com", Method: http.MethodGet}
	ctx := t.Context()

	tn := time.Now()
	err = r.SendPayload(ctx, Unset, func() (*Item, error) { return &i, nil }, UnauthenticatedRequest)
	if err != nil {
		t.Fatal(err)
	}
	err = r.SendPayload(ctx, Unset, func() (*Item, error) { return &i, nil }, UnauthenticatedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(tn) < time.Second {
		t.Error("rate limit issues")
	}

	ctx, cancel := context.WithDeadline(ctx, tn.Add(time.Nanosecond))
	defer cancel()
	err = r.SendPayload(ctx, Unset, func() (*Item, error) { return &i, nil }, UnauthenticatedRequest)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestEnableDisableRateLimit(t *testing.T) {
	r, err := New("TestRequest", new(http.Client), WithLimiter(NewBasicRateLimit(50*time.Millisecond, 1, 1)))
	require.NoError(t, err, "New requester must not error")

	sendIt := func() error {
		return r.SendPayload(t.Context(), Auth, func() (*Item, error) {
			return &Item{
				Method: http.MethodGet,
				Path:   testURL,
				Result: new(any),
			}, nil
		}, AuthenticatedRequest)
	}

	// allow initial request
	require.NoError(t, sendIt(), "sendIt must not error")

	// error on redundant enable
	assert.ErrorIs(t, r.EnableRateLimiter(), ErrRateLimiterAlreadyEnabled)

	// error on redundant disable
	require.NoError(t, r.DisableRateLimiter(), "DisableRateLimiter must not error")
	assert.ErrorIs(t, r.DisableRateLimiter(), ErrRateLimiterAlreadyDisabled)

	// allow requests when disabled
	require.NoError(t, sendIt(), "sendIt must not error")

	// allow when re-enabled
	require.NoError(t, r.EnableRateLimiter(), "EnableRateLimiter must succeed")
	require.NoError(t, sendIt(), "sendIt must not error")

	// block excess requests
	require.NoError(t, sendIt(), "sendIt must not error") // consume the one token
	start := time.Now()
	err = sendIt() // this should block until a token is refilled
	require.NoError(t, err, "sendIt must not error")
	elapsed := time.Since(start)
	assert.GreaterOrEqualf(t, elapsed.Milliseconds(), int64(20), "Expected sendIt to block for at least 20ms, but it returned after %dms", elapsed.Milliseconds())
}

func TestSetHTTPClient(t *testing.T) {
	var r *Requester
	err := r.SetHTTPClient(nil)
	require.ErrorIs(t, err, ErrRequestSystemIsNil)

	client := new(http.Client)
	r = new(Requester)
	err = r.SetHTTPClient(client)
	require.NoError(t, err)

	err = r.SetHTTPClient(client)
	require.ErrorIs(t, err, errCannotReuseHTTPClient)
}

func TestSetHTTPClientTimeout(t *testing.T) {
	var r *Requester
	err := r.SetHTTPClientTimeout(0)
	require.ErrorIs(t, err, ErrRequestSystemIsNil)

	r = new(Requester)
	err = r.SetHTTPClient(common.NewHTTPClientWithTimeout(2))
	if err != nil {
		t.Fatal(err)
	}
	err = r.SetHTTPClientTimeout(time.Second)
	require.NoError(t, err)
}

func TestSetHTTPClientUserAgent(t *testing.T) {
	var r *Requester
	err := r.SetHTTPClientUserAgent("")
	require.ErrorIs(t, err, ErrRequestSystemIsNil)

	r = new(Requester)
	err = r.SetHTTPClientUserAgent("")
	require.NoError(t, err)
}

func TestGetHTTPClientUserAgent(t *testing.T) {
	var r *Requester
	_, err := r.GetHTTPClientUserAgent()
	require.ErrorIs(t, err, ErrRequestSystemIsNil)

	r = new(Requester)
	err = r.SetHTTPClientUserAgent("sillyness")
	require.NoError(t, err)

	ua, err := r.GetHTTPClientUserAgent()
	require.NoError(t, err)

	if ua != "sillyness" {
		t.Fatal("unexpected value")
	}
}

func TestGetRateLimiterDefinitions(t *testing.T) {
	t.Parallel()
	require.Equal(t, RateLimitDefinitions(nil), (*Requester)(nil).GetRateLimiterDefinitions())
	r, err := New("test", new(http.Client), WithLimiter(globalshell))
	require.NoError(t, err)
	require.NotEmpty(t, r.GetRateLimiterDefinitions())
	assert.Equal(t, globalshell, r.GetRateLimiterDefinitions())
}
