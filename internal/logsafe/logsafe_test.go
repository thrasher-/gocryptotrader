package logsafe

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testNetError struct {
	timeout   bool
	temporary bool
}

func (*testNetError) Error() string { return "network-error-token" }

func (e *testNetError) Timeout() bool { return e.timeout }

func (e *testNetError) Temporary() bool { return e.temporary }

type nilUnsafeError struct {
	err error
}

func (*nilUnsafeError) Error() string { return "nil-unsafe-token" }

func (e *nilUnsafeError) Unwrap() error { return e.err }

type testAsError struct {
	value string
}

func (*testAsError) Error() string { return "as-target-token" }

type testChainError struct {
	isTarget error
	asTarget *testAsError
	err      error
}

func (*testChainError) Error() string { return "chain-error-token" }

func (e *testChainError) Is(target error) bool { return target == e.isTarget }

func (e *testChainError) As(target any) bool {
	asTarget, ok := target.(**testAsError)
	if !ok {
		return false
	}
	*asTarget = e.asTarget
	return true
}

func (e *testChainError) Unwrap() error { return e.err }

type panicMatcherError struct {
	err error
}

func (*panicMatcherError) Error() string { return "panic-matcher-token" }

func (*panicMatcherError) Is(error) bool { panic("panic Is token") }

func (*panicMatcherError) As(any) bool { panic("panic As token") }

func (e *panicMatcherError) Unwrap() error { return e.err }

type panicUnwrapError struct{}

func (*panicUnwrapError) Error() string { return "panic-unwrap-token" }

func (*panicUnwrapError) Unwrap() error { panic("panic Unwrap token") }

type panicJoinError struct{}

func (*panicJoinError) Error() string { return "panic-join-token" }

func (*panicJoinError) Unwrap() []error { panic("panic join token") }

type testJoinError []error

func (testJoinError) Error() string { return "join-error-token" }

func (e testJoinError) Unwrap() []error { return e }

type testSliceError []string

func (testSliceError) Error() string { return "slice-error-token" }

type cycleError struct{}

func (*cycleError) Error() string { return "cycle-secret-token" }

func (e *cycleError) Unwrap() error { return e }

type cycleJoinError struct{}

func (*cycleJoinError) Error() string { return "cycle-join-secret-token" }

func (e *cycleJoinError) Unwrap() []error { return []error{e} }

type sliceCycleError []string

func (sliceCycleError) Error() string { return "slice-cycle-secret-token" }

func (e sliceCycleError) Unwrap() error { return e }

type branchingSliceCycleError []string

func (branchingSliceCycleError) Error() string { return "branching-cycle-secret-token" }

func (branchingSliceCycleError) Is(target error) bool {
	_, ok := target.(branchingSliceCycleError)
	return ok
}

func (e branchingSliceCycleError) Unwrap() []error { return []error{e, e} }

type mutableCycleError struct {
	err error
}

func (*mutableCycleError) Error() string { return "mutable-cycle-secret-token" }

func (e *mutableCycleError) Unwrap() error { return e.err }

type dynamicallyUnhashableError struct {
	value   any
	cycle   bool
	matchID string
}

func (dynamicallyUnhashableError) Error() string { return "dynamically-unhashable-secret-token" }

func (e dynamicallyUnhashableError) Is(target error) bool {
	other, ok := target.(dynamicallyUnhashableError)
	return ok && e.matchID != "" && e.matchID == other.matchID
}

func (e dynamicallyUnhashableError) Unwrap() error {
	if e.cycle {
		return e
	}
	return nil
}

type timeoutOnlyError struct {
	timeout bool
}

func (*timeoutOnlyError) Error() string { return "timeout-only-secret-token" }

func (e *timeoutOnlyError) Timeout() bool { return e.timeout }

type temporaryOnlyError struct {
	temporary bool
}

func (*temporaryOnlyError) Error() string { return "temporary-only-secret-token" }

func (e *temporaryOnlyError) Temporary() bool { return e.temporary }

type panicNetError struct{}

func (*panicNetError) Error() string { return "panic-network-token" }

func (*panicNetError) Timeout() bool { panic("panic Timeout token") }

func (*panicNetError) Temporary() bool { panic("panic Temporary token") }

type reentrantMatcherError struct {
	safe error
}

func (*reentrantMatcherError) Error() string { return "reentrant-matcher-secret-token" }

func (e *reentrantMatcherError) Is(target error) bool { return errors.Is(e.safe, target) }

func (e *reentrantMatcherError) As(target any) bool { return errors.As(e.safe, target) }

type reentrantNetworkError struct {
	safe *safeURLError
}

func (*reentrantNetworkError) Error() string { return "reentrant-network-secret-token" }

func (e *reentrantNetworkError) Timeout() bool { return e.safe.Timeout() }

func (e *reentrantNetworkError) Temporary() bool { return e.safe.Temporary() }

type freshReentrantMatcherError struct{}

func (*freshReentrantMatcherError) Error() string { return "fresh-reentrant-matcher-secret-token" }

func (e *freshReentrantMatcherError) Is(target error) bool { return errors.Is(Error(e), target) }

func (e *freshReentrantMatcherError) As(target any) bool { return errors.As(Error(e), target) }

type freshReentrantNetworkError struct{}

func (*freshReentrantNetworkError) Error() string { return "fresh-reentrant-network-secret-token" }

func (e *freshReentrantNetworkError) Timeout() bool {
	safe, _ := URLRequestError(&url.Error{URL: "https://example.com", Err: e}).(*safeURLError)
	return safe.Timeout()
}

func (e *freshReentrantNetworkError) Temporary() bool {
	safe, _ := URLRequestError(&url.Error{URL: "https://example.com", Err: e}).(*safeURLError)
	return safe.Temporary()
}

type freshReentrantUnwrapError struct{}

func (*freshReentrantUnwrapError) Error() string { return "fresh-reentrant-unwrap-secret-token" }

func (e *freshReentrantUnwrapError) Unwrap() error { return URLRequestError(e) }

type freshReentrantJoinUnwrapError struct{}

func (*freshReentrantJoinUnwrapError) Error() string {
	return "fresh-reentrant-join-unwrap-secret-token"
}

func (e *freshReentrantJoinUnwrapError) Unwrap() []error { return []error{URLRequestError(e)} }

//go:noinline
func callErrorMethodAtDepth(depth int, callback func()) bool {
	if depth == 0 {
		return callErrorMethod(callback)
	}
	return callErrorMethodAtDepth(depth-1, callback)
}

//go:noinline
func urlRequestErrorAtDepth(depth int, err error) error {
	if depth == 0 {
		return URLRequestError(err)
	}
	return urlRequestErrorAtDepth(depth-1, err)
}

type testFormatState struct {
	bytes.Buffer
}

func (*testFormatState) Width() (int, bool) { return 12, true }

func (*testFormatState) Precision() (int, bool) { return 5, true }

func (*testFormatState) Flag(int) bool { return true }

func TestURL(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		url      string
		expected string
	}{
		{name: "origin", url: "https://example.com", expected: "https://example.com"},
		{name: "port", url: "wss://example.com:8443/socket", expected: "wss://example.com:8443"},
		{name: "credentials", url: "https://user:password@example.com/path-token?api_key=query-token#fragment-token", expected: "https://example.com"},
		{name: "forced query", url: "https://example.com?", expected: "https://example.com"},
		{name: "relative", url: "/path-token?api_key=query-token", expected: redactedURL},
		{name: "opaque", url: "secret:path-token", expected: redactedURL},
		{name: "malformed", url: "://path-token", expected: redactedURL},
		{name: "empty", expected: redactedURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, URL(tc.url), "URL should return safe endpoint metadata")
		})
	}
}

func TestCallErrorMethod(t *testing.T) {
	t.Parallel()

	called := false
	assert.True(t, callErrorMethod(func() {
		called = true
	}), "callErrorMethod should report an ordinary callback as complete")
	assert.True(t, called, "callErrorMethod should invoke an ordinary callback")

	assert.PanicsWithValue(t, "callback-panic-token", func() {
		callErrorMethod(func() { panic("callback-panic-token") })
	}, "callErrorMethod should preserve callback panics")

	innerCalled := false
	innerComplete := true
	assert.True(t, callErrorMethod(func() {
		innerComplete = callErrorMethod(func() { innerCalled = true })
	}), "callErrorMethod should complete the outer callback")
	assert.False(t, innerComplete, "callErrorMethod should reject synchronous re-entry")
	assert.False(t, innerCalled, "callErrorMethod should not invoke a re-entrant callback")

	callbackCalledAtDepth := false
	assert.False(t, callErrorMethodAtDepth(maxErrorMethodFrames+32, func() { callbackCalledAtDepth = true }), "callErrorMethod should fail closed when stack inspection is truncated")
	assert.False(t, callbackCalledAtDepth, "callErrorMethod should not invoke a callback after truncated stack inspection")

	value, incomplete := callErrorMethodValue(func() int { return 42 })
	assert.Equal(t, 42, value, "callErrorMethodValue should return an ordinary callback value")
	assert.False(t, incomplete, "callErrorMethodValue should report an ordinary callback as complete")
	assert.True(t, callErrorMethod(func() {
		value, incomplete = callErrorMethodValue(func() int { return 7 })
	}), "callErrorMethod should complete the outer value callback")
	assert.Zero(t, value, "callErrorMethodValue should return the zero value after re-entry")
	assert.True(t, incomplete, "callErrorMethodValue should report re-entry as incomplete")
	assert.PanicsWithValue(t, "value-callback-panic-token", func() {
		callErrorMethodValue(func() int { panic("value-callback-panic-token") })
	}, "callErrorMethodValue should preserve callback panics")
}

func TestCallErrorMethodConcurrent(t *testing.T) {
	t.Parallel()

	const callerCount = 128
	var entered atomic.Int32
	var panics atomic.Int32
	var rejected atomic.Int32
	allEntered := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	for range callerCount {
		wg.Go(func() {
			defer func() {
				if recover() != nil {
					panics.Add(1)
				}
			}()
			if !callErrorMethod(func() {
				if entered.Add(1) == callerCount {
					close(allEntered)
				}
				<-release
			}) {
				rejected.Add(1)
			}
		})
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	concurrent := false
	select {
	case <-allEntered:
		concurrent = true
	case <-timer.C:
	}
	close(release)
	wg.Wait()
	require.True(t, concurrent, "callErrorMethod must allow all independent goroutines to enter concurrently")
	assert.Equal(t, int32(callerCount), entered.Load(), "callErrorMethod should invoke every concurrent callback")
	assert.Zero(t, rejected.Load(), "callErrorMethod should accept every concurrent callback")
	assert.Zero(t, panics.Load(), "callErrorMethod should not treat concurrent goroutines as re-entry")
}

func TestURLRequestError(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-token")
	rawErr := &url.Error{Op: "GET", URL: "https://user:password@example.com/path-token?api_key=query-token", Err: cause}
	wrappedErr := fmt.Errorf("outer-token: %w", rawErr)
	wrappedSafeErr := fmt.Errorf("outer-token: %w", URLRequestError(rawErr))

	for _, tc := range []struct {
		name       string
		err        error
		expectSame bool
	}{
		{name: "nil"},
		{name: "plain", err: cause, expectSame: true},
		{name: "URL error", err: rawErr},
		{name: "wrapped URL error", err: wrappedErr},
		{name: "wrapped safe URL error", err: wrappedSafeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := URLRequestError(tc.err)
			if tc.err == nil {
				assert.NoError(t, result, "URLRequestError should preserve nil")
				return
			}
			if tc.expectSame {
				assert.Same(t, tc.err, result, "URLRequestError should preserve non-URL errors")
				return
			}
			require.Error(t, result, "URLRequestError must return a safe error")
			assert.Contains(t, result.Error(), "https://example.com", "URLRequestError should retain safe endpoint metadata")
			assert.NotContains(t, result.Error(), "password", "URLRequestError should omit userinfo")
			assert.NotContains(t, result.Error(), "path-token", "URLRequestError should omit paths")
			assert.NotContains(t, result.Error(), "query-token", "URLRequestError should omit queries")
			assert.NotContains(t, result.Error(), "outer-token", "URLRequestError should omit wrapper text")
			assert.NotContains(t, result.Error(), "cause-token", "URLRequestError should omit cause text")
			assert.ErrorIs(t, result, cause, "URLRequestError should preserve the underlying cause")
			var target *url.Error
			assert.ErrorAs(t, result, &target, "URLRequestError should preserve URL error inspection")
			assert.Same(t, result, URLRequestError(result), "URLRequestError should not wrap an already safe error")
		})
	}
}

func TestURLRequestErrorTypedNil(t *testing.T) {
	t.Parallel()

	var typedNil *url.Error
	source := error(typedNil)
	result := URLRequestError(source)
	require.Error(t, result, "URLRequestError must safely wrap a typed-nil URL error")

	for _, format := range []string{"%v", "%+v", "%#v"} {
		var formatted string
		assert.NotPanics(t, func() {
			formatted = fmt.Sprintf(format, result)
		}, "URLRequestError formatting should not panic for a typed-nil URL error")
		assert.Contains(t, formatted, redactedURL, "URLRequestError should fail closed for a typed-nil URL error")
	}

	assert.ErrorIs(t, result, source, "URLRequestError should preserve a typed-nil source error")
	var target *url.Error
	require.ErrorAs(t, result, &target, "URLRequestError must preserve typed-nil URL error inspection")
	assert.Nil(t, target, "URLRequestError should preserve the typed-nil URL error value")
	assert.NoError(t, errors.Unwrap(result), "errors.Unwrap should not expose the typed-nil URL error")
	var unrelatedIs bool
	assert.NotPanics(t, func() {
		unrelatedIs = errors.Is(result, errors.New("unrelated-token"))
	}, "errors.Is should not panic for an unrelated typed-nil URL target")
	assert.False(t, unrelatedIs, "errors.Is should reject an unrelated typed-nil URL target")
	var unrelatedAs *testNetError
	var unrelatedAsMatch bool
	assert.NotPanics(t, func() {
		unrelatedAsMatch = errors.As(result, &unrelatedAs)
	}, "errors.As should not panic for an unrelated typed-nil URL target")
	assert.False(t, unrelatedAsMatch, "errors.As should reject an unrelated typed-nil URL target")

	wrappedResult := fmt.Errorf("safe wrapper: %w", result)
	assert.ErrorIs(t, wrappedResult, source, "errors.Is should inspect a wrapped safe URL error")
	var wrappedTarget *url.Error
	require.ErrorAs(t, wrappedResult, &wrappedTarget, "errors.As must inspect a wrapped safe URL error")
	assert.Nil(t, wrappedTarget, "errors.As should preserve the wrapped typed-nil URL error value")

	netErr, ok := result.(net.Error)
	require.True(t, ok, "URLRequestError must preserve net.Error compatibility for a typed-nil URL error")
	assert.False(t, netErr.Timeout(), "URLRequestError should fail closed for typed-nil timeout classification")
	temporaryErr, ok := result.(temporaryError)
	require.True(t, ok, "URLRequestError must preserve temporary error compatibility for a typed-nil URL error")
	assert.False(t, temporaryErr.Temporary(), "URLRequestError should fail closed for typed-nil temporary classification")
}

func TestURLRequestErrorNestedTypedNil(t *testing.T) {
	t.Parallel()

	var typedNil *testNetError
	typedNilSource := error(typedNil)
	result := URLRequestError(&url.Error{
		Op:  "outer",
		URL: "https://outer-user:outer-password@example.com/outer-path?signature=outer-query",
		Err: &url.Error{
			Op:  "inner",
			URL: "https://inner-user:inner-password@example.net/inner-path?signature=inner-query",
			Err: typedNil,
		},
	})
	require.Error(t, result, "URLRequestError must safely wrap a nested typed-nil network error")

	for _, format := range []string{"%v", "%+v", "%#v"} {
		formatted := fmt.Sprintf(format, result)
		assert.Contains(t, formatted, "https://example.com", "URLRequestError should retain safe outer endpoint metadata")
		for _, secret := range []string{"outer-user", "outer-password", "outer-path", "outer-query", "inner-user", "inner-password", "inner-path", "inner-query"} {
			assert.NotContains(t, formatted, secret, "URLRequestError formatting should omit nested URL data")
		}
	}

	assert.ErrorIs(t, result, typedNilSource, "URLRequestError should preserve a nested typed-nil source error")
	var target *testNetError
	require.ErrorAs(t, result, &target, "URLRequestError must preserve nested typed-nil network error inspection")
	assert.Nil(t, target, "URLRequestError should preserve the nested typed-nil network error value")

	netErr, ok := result.(net.Error)
	require.True(t, ok, "URLRequestError must preserve net.Error compatibility for a nested typed-nil error")
	assert.NotPanics(t, func() {
		assert.False(t, netErr.Timeout(), "URLRequestError should fail closed for nested typed-nil timeout classification")
	}, "URLRequestError Timeout should not panic for a nested typed-nil network error")
	temporaryErr, ok := result.(temporaryError)
	require.True(t, ok, "URLRequestError must preserve temporary error compatibility for a nested typed-nil error")
	assert.NotPanics(t, func() {
		assert.False(t, temporaryErr.Temporary(), "URLRequestError should fail closed for nested typed-nil temporary classification")
	}, "URLRequestError Temporary should not panic for a nested typed-nil network error")
}

func TestURLRequestErrorPoisonedBranches(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-token")
	rawURL := &url.Error{Op: "GET", URL: "https://user-token:password-token@example.com/path-token?signature=query-token", Err: cause}
	var typedNil *nilUnsafeError
	typedNilSource := error(typedNil)
	for _, tc := range []struct {
		name   string
		poison error
	}{
		{name: "typed nil", poison: typedNilSource},
		{name: "panicking matcher", poison: &panicMatcherError{}},
		{name: "panicking unwrap", poison: &panicUnwrapError{}},
		{name: "panicking join", poison: &panicJoinError{}},
		{name: "branching cycle", poison: branchingSliceCycleError{"branching-cycle-secret-token"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := errors.Join(tc.poison, rawURL)
			var result error
			assert.NotPanics(t, func() {
				result = URLRequestError(source)
			}, "URLRequestError should not panic while discovering a URL error after a poisoned branch")
			var safeResult *safeURLError
			require.ErrorAs(t, result, &safeResult, "URLRequestError must wrap a URL error found after a poisoned branch")
			for _, format := range []string{"%v", "%+v", "%#v"} {
				formatted := fmt.Sprintf(format, result)
				for _, secret := range []string{"user-token", "password-token", "path-token", "query-token", "cause-token"} {
					assert.NotContains(t, formatted, secret, "URLRequestError formatting should redact a URL error found after a poisoned branch")
				}
			}
			assert.ErrorIs(t, result, cause, "errors.Is should inspect a valid sibling after a poisoned branch")
			var target *url.Error
			require.ErrorAs(t, result, &target, "errors.As must inspect a valid sibling after a poisoned branch")
			assert.Same(t, rawURL, target, "errors.As should return the URL error after a poisoned branch")
			assert.NoError(t, errors.Unwrap(result), "errors.Unwrap should not expose the poisoned source chain")
		})
	}
}

func TestURLRequestErrorPanickingMatcherChild(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-secret-token")
	rawURL := &url.Error{Op: "GET-secret-token", URL: "https://user-secret:password-secret@example.com/path-secret?signature=query-secret", Err: cause}
	source := &panicMatcherError{err: rawURL}
	result := URLRequestError(source)
	var safeResult *safeURLError
	require.ErrorAs(t, result, &safeResult, "URLRequestError must find a URL error after a panicking matcher")
	assert.ErrorIs(t, result, cause, "URLRequestError should preserve the URL error cause")
	for _, format := range []string{"%v", "%#v", "%c", "%f", "%U"} {
		formatted := fmt.Sprintf(format, result)
		for _, secret := range []string{"cause-secret-token", "GET-secret-token", "user-secret", "password-secret", "path-secret", "query-secret"} {
			assert.NotContains(t, formatted, secret, "URLRequestError should redact a child found after a panicking matcher")
		}
	}
}

func TestURLRequestErrorIncompleteTraversal(t *testing.T) {
	t.Parallel()

	cycle := new(cycleError)
	joinCycle := new(cycleJoinError)
	for _, tc := range []struct {
		name     string
		err      error
		expectIs bool
		secret   string
	}{
		{name: "panicking unwrap", err: &panicUnwrapError{}, expectIs: true, secret: "panic-unwrap-token"},
		{name: "single cycle", err: cycle, expectIs: true, secret: "cycle-secret-token"},
		{name: "join cycle", err: joinCycle, expectIs: true, secret: "cycle-join-secret-token"},
		{name: "non-comparable cycle", err: sliceCycleError{"slice-cycle-secret-token"}, secret: "slice-cycle-secret-token"},
		{name: "branching non-comparable cycle", err: branchingSliceCycleError{"branching-cycle-secret-token"}, expectIs: true, secret: "branching-cycle-secret-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := URLRequestError(tc.err)
			var safeResult *safeError
			require.ErrorAs(t, result, &safeResult, "URLRequestError must fail closed when traversal is incomplete")
			assert.NotContains(t, fmt.Sprintf("%c", result), tc.secret, "URLRequestError should hide incomplete traversal details")
			assert.Equal(t, tc.expectIs, errors.Is(result, tc.err), "URLRequestError should preserve comparable source matching")
			assert.NoError(t, errors.Unwrap(result), "URLRequestError should not expose an incomplete source chain")
		})
	}
}

func TestSafeErrorCycle(t *testing.T) {
	t.Parallel()

	cycle := new(mutableCycleError)
	safeErr := Error(cycle)
	cycle.err = safeErr

	assert.ErrorIs(t, safeErr, cycle, "safeError should preserve direct matching inside a cycle")
	var cycleTarget *mutableCycleError
	require.ErrorAs(t, safeErr, &cycleTarget, "safeError must preserve direct type inspection inside a cycle")
	assert.Same(t, cycle, cycleTarget, "safeError should return the retained cycle node")

	var unrelatedIs bool
	assert.NotPanics(t, func() {
		unrelatedIs = errors.Is(safeErr, errors.New("unrelated-token"))
	}, "safeError errors.Is should terminate for a cycle which re-enters the wrapper")
	assert.False(t, unrelatedIs, "safeError errors.Is should reject an unrelated cycle target")
	var unrelatedAs *url.Error
	var unrelatedAsMatch bool
	assert.NotPanics(t, func() {
		unrelatedAsMatch = errors.As(safeErr, &unrelatedAs)
	}, "safeError errors.As should terminate for a cycle which re-enters the wrapper")
	assert.False(t, unrelatedAsMatch, "safeError errors.As should reject an unrelated cycle target")

	var result error
	assert.NotPanics(t, func() {
		result = URLRequestError(cycle)
	}, "URLRequestError should terminate for a cycle which re-enters a safeError")
	var fallback *safeError
	require.ErrorAs(t, result, &fallback, "URLRequestError must fail closed for a safeError cycle")
	for _, format := range []string{"%v", "%#v", "%c", "%f", "%U"} {
		assert.NotContains(t, fmt.Sprintf(format, result), "mutable-cycle-secret-token", "URLRequestError should redact safeError cycle details")
	}
	assert.NoError(t, errors.Unwrap(result), "URLRequestError should not expose a safeError cycle")
}

func TestSafeURLErrorCycle(t *testing.T) {
	t.Parallel()

	cycle := new(mutableCycleError)
	rawURL := &url.Error{
		Op:  "GET-secret-token",
		URL: "https://user-secret:password-secret@example.com/path-secret?signature=query-secret",
		Err: cycle,
	}
	safeErr, ok := URLRequestError(rawURL).(*safeURLError)
	require.True(t, ok, "URLRequestError must return a safe URL error")
	cycle.err = safeErr

	assert.ErrorIs(t, safeErr, rawURL, "safeURLError should preserve direct matching inside a cycle")
	var rawTarget *url.Error
	require.ErrorAs(t, safeErr, &rawTarget, "safeURLError must preserve direct URL error inspection inside a cycle")
	assert.Same(t, rawURL, rawTarget, "safeURLError should return the retained URL error")

	var unrelatedIs bool
	assert.NotPanics(t, func() {
		unrelatedIs = errors.Is(safeErr, errors.New("unrelated-token"))
	}, "safeURLError errors.Is should terminate for a cycle which re-enters the wrapper")
	assert.False(t, unrelatedIs, "safeURLError errors.Is should reject an unrelated cycle target")
	var unrelatedAs *testNetError
	var unrelatedAsMatch bool
	assert.NotPanics(t, func() {
		unrelatedAsMatch = errors.As(safeErr, &unrelatedAs)
	}, "safeURLError errors.As should terminate for a cycle which re-enters the wrapper")
	assert.False(t, unrelatedAsMatch, "safeURLError errors.As should reject an unrelated cycle target")
	assert.NotPanics(t, func() {
		assert.False(t, safeErr.Timeout(), "safeURLError should fail closed for a cyclic timeout classification")
		assert.False(t, safeErr.Temporary(), "safeURLError should fail closed for a cyclic temporary classification")
	}, "safeURLError network classification should terminate when a cycle re-enters the wrapper")

	nested := URLRequestError(cycle)
	var nestedSafe *safeURLError
	require.ErrorAs(t, nested, &nestedSafe, "URLRequestError must find a URL error through a safe wrapper cycle")
	for _, format := range []string{"%v", "%#v", "%c", "%f", "%U"} {
		formatted := fmt.Sprintf(format, nested)
		for _, secret := range []string{"GET-secret-token", "user-secret", "password-secret", "path-secret", "query-secret", "mutable-cycle-secret-token"} {
			assert.NotContains(t, formatted, secret, "URLRequestError should redact safeURLError cycle details")
		}
	}
}

func TestDynamicallyUnhashableError(t *testing.T) {
	t.Parallel()

	cleanMiss := dynamicallyUnhashableError{value: []string{"clean-secret-token"}}
	var result error
	assert.NotPanics(t, func() {
		result = URLRequestError(cleanMiss)
	}, "URLRequestError should not hash a dynamically unhashable error value")
	assert.Equal(t, cleanMiss, result, "URLRequestError should preserve a complete non-URL traversal")

	safeErr := Error(cleanMiss)
	var unrelatedIs bool
	assert.NotPanics(t, func() {
		unrelatedIs = errors.Is(safeErr, errors.New("unrelated-token"))
	}, "safeError errors.Is should not hash a dynamically unhashable error value")
	assert.False(t, unrelatedIs, "safeError errors.Is should reject an unrelated dynamically unhashable error")
	var unrelatedAs *url.Error
	var unrelatedAsMatch bool
	assert.NotPanics(t, func() {
		unrelatedAsMatch = errors.As(safeErr, &unrelatedAs)
	}, "safeError errors.As should not hash a dynamically unhashable error value")
	assert.False(t, unrelatedAsMatch, "safeError errors.As should reject an unrelated dynamically unhashable error")
	matchTarget := dynamicallyUnhashableError{value: []string{"target-secret-token"}, matchID: "match"}
	matchSource := dynamicallyUnhashableError{value: []string{"source-secret-token"}, matchID: "match"}
	assert.NotPanics(t, func() {
		assert.ErrorIs(t, Error(matchSource), matchTarget, "safeError should preserve custom matching for dynamically unhashable errors")
	}, "safeError errors.Is should not compare dynamically unhashable error values directly")

	cycle := dynamicallyUnhashableError{value: []string{"cycle-secret-token"}, cycle: true}
	assert.NotPanics(t, func() {
		result = URLRequestError(cycle)
	}, "URLRequestError should bound a dynamically unhashable self-cycle")
	var fallback *safeError
	require.ErrorAs(t, result, &fallback, "URLRequestError must fail closed for a dynamically unhashable self-cycle")
	assert.NotContains(t, fmt.Sprintf("%#v", result), "cycle-secret-token", "URLRequestError should redact a dynamically unhashable self-cycle")
	assert.NoError(t, errors.Unwrap(result), "URLRequestError should not expose a dynamically unhashable self-cycle")
}

func TestError(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-token")
	safeErr := Error(cause)
	safeURLErr := URLRequestError(&url.Error{URL: "https://example.com", Err: cause})
	for _, tc := range []struct {
		name       string
		err        error
		expectSame bool
		expectType string
	}{
		{name: "nil"},
		{name: "error", err: cause, expectType: "*errors.errorString"},
		{name: "already safe", err: safeErr, expectSame: true},
		{name: "already safe URL error", err: safeURLErr, expectSame: true},
		{name: "wrapped safe error", err: fmt.Errorf("outer-token: %w", safeErr), expectType: "*fmt.wrapError"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := Error(tc.err)
			if tc.err == nil {
				assert.NoError(t, result, "Error should preserve nil")
				return
			}
			if tc.expectSame {
				assert.Same(t, tc.err, result, "Error should preserve safe display errors")
			} else {
				assert.NotContains(t, result.Error(), "cause-token", "Error should omit cause text")
				assert.NotContains(t, result.Error(), "outer-token", "Error should omit wrapper text")
				assert.Contains(t, result.Error(), tc.expectType, "Error should retain cause type metadata")
			}
			assert.ErrorIs(t, result, cause, "Error should preserve the underlying cause")
		})
	}
}

func TestErrorTypedNilChains(t *testing.T) {
	t.Parallel()

	var typedNil *nilUnsafeError
	typedNilSource := error(typedNil)
	for _, tc := range []struct {
		name   string
		source error
	}{
		{name: "direct", source: typedNilSource},
		{name: "nested", source: fmt.Errorf("nested-secret-token: %w", typedNilSource)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := Error(tc.source)
			require.Error(t, result, "Error must safely wrap a typed-nil chain")
			for _, format := range []string{"%v", "%+v", "%#v"} {
				formatted := fmt.Sprintf(format, result)
				assert.NotContains(t, formatted, "nil-unsafe-token", "Error formatting should omit typed-nil source text")
				assert.NotContains(t, formatted, "nested-secret-token", "Error formatting should omit nested wrapper text")
			}
			assert.ErrorIs(t, result, typedNilSource, "errors.Is should match a typed-nil node")
			var target *nilUnsafeError
			require.ErrorAs(t, result, &target, "errors.As must match a typed-nil node")
			assert.Nil(t, target, "errors.As should preserve the typed-nil node value")
			var unrelatedIs bool
			assert.NotPanics(t, func() {
				unrelatedIs = errors.Is(result, errors.New("unrelated-token"))
			}, "errors.Is should not panic while rejecting an unrelated typed-nil chain target")
			assert.False(t, unrelatedIs, "errors.Is should reject an unrelated typed-nil chain target")
			var unrelatedAs *url.Error
			var unrelatedAsMatch bool
			assert.NotPanics(t, func() {
				unrelatedAsMatch = errors.As(result, &unrelatedAs)
			}, "errors.As should not panic while rejecting an unrelated typed-nil chain target")
			assert.False(t, unrelatedAsMatch, "errors.As should reject an unrelated typed-nil chain target")
			wrappedResult := fmt.Errorf("safe wrapper: %w", result)
			assert.ErrorIs(t, wrappedResult, typedNilSource, "errors.Is should inspect a wrapped safe error")
			var wrappedTarget *nilUnsafeError
			require.ErrorAs(t, wrappedResult, &wrappedTarget, "errors.As must inspect a wrapped safe error")
			assert.Nil(t, wrappedTarget, "errors.As should preserve the wrapped typed-nil node value")
			assert.NoError(t, errors.Unwrap(result), "errors.Unwrap should not expose the typed-nil source chain")
		})
	}
}

func TestErrorPoisonedBranches(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-token")
	validAs := &testNetError{timeout: true}
	var typedNil *nilUnsafeError
	result := Error(errors.Join(typedNil, validAs, cause))
	require.Error(t, result, "Error must wrap a joined error tree")
	var isMatch bool
	assert.NotPanics(t, func() {
		isMatch = errors.Is(result, cause)
	}, "errors.Is should not panic before a valid sibling")
	assert.True(t, isMatch, "errors.Is should match a valid sibling after a typed-nil branch")
	var asTarget *testNetError
	var asMatch bool
	assert.NotPanics(t, func() {
		asMatch = errors.As(result, &asTarget)
	}, "errors.As should not panic before a valid sibling")
	assert.True(t, asMatch, "errors.As should match a valid sibling after a typed-nil branch")
	assert.Same(t, validAs, asTarget, "errors.As should return the valid sibling after a typed-nil branch")
}

func TestSafeErrorIs(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-token")
	safeErr, ok := Error(cause).(*safeError)
	require.True(t, ok, "Error must return a safeError")
	assert.True(t, safeErr.Is(cause), "safeError.Is should match the retained error")
	assert.False(t, safeErr.Is(errors.New("unrelated")), "safeError.Is should reject an unrelated error")
}

func TestSafeErrorAs(t *testing.T) {
	t.Parallel()

	cause := &testNetError{timeout: true}
	safeErr, ok := Error(cause).(*safeError)
	require.True(t, ok, "Error must return a safeError")
	var target *testNetError
	assert.True(t, safeErr.As(&target), "safeError.As should match the retained error")
	assert.Same(t, cause, target, "safeError.As should return the retained error")
	var unrelated *url.Error
	assert.False(t, safeErr.As(&unrelated), "safeError.As should reject an unrelated error type")
}

func TestSafeURLErrorIs(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-token")
	safeErr, ok := URLRequestError(&url.Error{URL: "https://example.com", Err: cause}).(*safeURLError)
	require.True(t, ok, "URLRequestError must return a safeURLError")
	assert.True(t, safeErr.Is(cause), "safeURLError.Is should match the retained cause")
	assert.False(t, safeErr.Is(errors.New("unrelated")), "safeURLError.Is should reject an unrelated error")
}

func TestSafeURLErrorAs(t *testing.T) {
	t.Parallel()

	cause := &url.Error{URL: "https://example.com", Err: errors.New("cause-token")}
	safeErr, ok := URLRequestError(cause).(*safeURLError)
	require.True(t, ok, "URLRequestError must return a safeURLError")
	var target *url.Error
	assert.True(t, safeErr.As(&target), "safeURLError.As should match the retained URL error")
	assert.Same(t, cause, target, "safeURLError.As should return the retained URL error")
	var unrelated *testNetError
	assert.False(t, safeErr.As(&unrelated), "safeURLError.As should reject an unrelated error type")
}

func TestSafeErrorReentrantMethods(t *testing.T) {
	t.Parallel()

	source := new(reentrantMatcherError)
	safeErr, ok := Error(source).(*safeError)
	require.True(t, ok, "Error must return a safeError")
	source.safe = safeErr

	assert.NotPanics(t, func() {
		assert.False(t, safeErr.Is(errors.New("unrelated-token")), "safeError.Is should fail closed when a matcher re-enters the wrapper")
	}, "safeError.Is should bound re-entrant matchers")
	var target *url.Error
	assert.NotPanics(t, func() {
		assert.False(t, safeErr.As(&target), "safeError.As should fail closed when a matcher re-enters the wrapper")
	}, "safeError.As should bound re-entrant matchers")
}

func TestSafeURLErrorReentrantMethods(t *testing.T) {
	t.Parallel()

	matcher := new(reentrantMatcherError)
	safeErr, ok := URLRequestError(&url.Error{URL: "https://example.com", Err: matcher}).(*safeURLError)
	require.True(t, ok, "URLRequestError must return a safeURLError")
	matcher.safe = safeErr

	assert.NotPanics(t, func() {
		assert.False(t, safeErr.Is(errors.New("unrelated-token")), "safeURLError.Is should fail closed when a matcher re-enters the wrapper")
	}, "safeURLError.Is should bound re-entrant matchers")
	var target *testAsError
	assert.NotPanics(t, func() {
		assert.False(t, safeErr.As(&target), "safeURLError.As should fail closed when a matcher re-enters the wrapper")
	}, "safeURLError.As should bound re-entrant matchers")

	networkErr := new(reentrantNetworkError)
	safeNetworkErr, ok := URLRequestError(&url.Error{URL: "https://example.com", Err: networkErr}).(*safeURLError)
	require.True(t, ok, "URLRequestError must return a safeURLError for network classification")
	networkErr.safe = safeNetworkErr
	assert.NotPanics(t, func() {
		assert.False(t, safeNetworkErr.Timeout(), "safeURLError.Timeout should fail closed when a classifier re-enters the wrapper")
	}, "safeURLError.Timeout should bound re-entrant classifiers")
	assert.NotPanics(t, func() {
		assert.False(t, safeNetworkErr.Temporary(), "safeURLError.Temporary should fail closed when a classifier re-enters the wrapper")
	}, "safeURLError.Temporary should bound re-entrant classifiers")
}

func TestFreshWrapperReentrantMethods(t *testing.T) {
	t.Parallel()

	matcher := new(freshReentrantMatcherError)
	safeErr, ok := Error(matcher).(*safeError)
	require.True(t, ok, "Error must return a safeError")
	assert.NotPanics(t, func() {
		assert.False(t, safeErr.Is(errors.New("unrelated-token")), "safeError.Is should fail closed when a matcher creates a fresh wrapper")
	}, "safeError.Is should bound fresh-wrapper re-entry")
	var target *testAsError
	assert.NotPanics(t, func() {
		assert.False(t, safeErr.As(&target), "safeError.As should fail closed when a matcher creates a fresh wrapper")
	}, "safeError.As should bound fresh-wrapper re-entry")

	constructorResult := URLRequestError(matcher)
	require.Error(t, constructorResult, "URLRequestError must fail closed when custom As creates a fresh wrapper")
	assert.NotSame(t, matcher, constructorResult, "URLRequestError should not return a re-entrant custom matcher directly")
	assert.NotContains(t, constructorResult.Error(), "secret-token", "URLRequestError should redact a re-entrant custom matcher")

	networkErr := new(freshReentrantNetworkError)
	safeNetworkErr, ok := URLRequestError(&url.Error{URL: "https://example.com", Err: networkErr}).(*safeURLError)
	require.True(t, ok, "URLRequestError must return a safeURLError for fresh-wrapper network classification")
	assert.NotPanics(t, func() {
		assert.False(t, safeNetworkErr.Timeout(), "safeURLError.Timeout should fail closed when a classifier creates a fresh wrapper")
	}, "safeURLError.Timeout should bound fresh-wrapper re-entry")
	assert.NotPanics(t, func() {
		assert.False(t, safeNetworkErr.Temporary(), "safeURLError.Temporary should fail closed when a classifier creates a fresh wrapper")
	}, "safeURLError.Temporary should bound fresh-wrapper re-entry")

	unwrapErr := new(freshReentrantUnwrapError)
	unwrapResult := URLRequestError(unwrapErr)
	require.Error(t, unwrapResult, "URLRequestError must fail closed when Unwrap creates a fresh wrapper")
	assert.NotSame(t, unwrapErr, unwrapResult, "URLRequestError should not return a re-entrant unwrapper directly")
	assert.NotContains(t, unwrapResult.Error(), "secret-token", "URLRequestError should redact a re-entrant unwrapper")

	joinUnwrapErr := new(freshReentrantJoinUnwrapError)
	joinUnwrapResult := URLRequestError(joinUnwrapErr)
	require.Error(t, joinUnwrapResult, "URLRequestError must fail closed when join Unwrap creates a fresh wrapper")
	assert.NotSame(t, joinUnwrapErr, joinUnwrapResult, "URLRequestError should not return a re-entrant join unwrapper directly")
	assert.NotContains(t, joinUnwrapResult.Error(), "secret-token", "URLRequestError should redact a re-entrant join unwrapper")
}

func TestURLRequestErrorStackInspectionLimit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		err    error
		secret string
	}{
		{name: "single unwrap", err: &nilUnsafeError{err: errors.New("single-child-token")}, secret: "nil-unsafe-token"},
		{name: "join unwrap", err: testJoinError{errors.New("join-child-token")}, secret: "join-error-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := urlRequestErrorAtDepth(maxErrorMethodFrames+32, tc.err)
			require.Error(t, result, "URLRequestError must fail closed when stack inspection is truncated")
			_, isSafeError := result.(*safeError)
			assert.True(t, isSafeError, "URLRequestError should return a metadata-only error after truncated stack inspection")
			assert.NotContains(t, result.Error(), tc.secret, "URLRequestError should redact an error after truncated stack inspection")
		})
	}
}

func TestErrorIs(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-token")
	unrelated := errors.New("unrelated-token")
	var typedNil *nilUnsafeError
	typedNilSource := error(typedNil)
	customMatch := &testChainError{isTarget: cause}
	customUnwrap := &testChainError{isTarget: unrelated, err: cause}
	branchingCycle := branchingSliceCycleError{"branching-cycle-secret-token"}
	for _, tc := range []struct {
		name     string
		err      error
		target   error
		expected bool
	}{
		{name: "nil matches nil", expected: true},
		{name: "nil source", target: cause},
		{name: "nil target", err: cause},
		{name: "direct", err: cause, target: cause, expected: true},
		{name: "unrelated", err: cause, target: unrelated},
		{name: "nested", err: fmt.Errorf("nested-token: %w", cause), target: cause, expected: true},
		{name: "typed nil", err: typedNilSource, target: typedNilSource, expected: true},
		{name: "nested typed nil", err: fmt.Errorf("nested-token: %w", typedNilSource), target: typedNilSource, expected: true},
		{name: "unrelated typed nil", err: typedNilSource, target: unrelated},
		{name: "typed nil sibling", err: errors.Join(typedNilSource, cause), target: cause, expected: true},
		{name: "custom match", err: customMatch, target: cause, expected: true},
		{name: "custom miss unwraps", err: customUnwrap, target: cause, expected: true},
		{name: "panicking matcher unwraps", err: &panicMatcherError{err: cause}, target: cause, expected: true},
		{name: "panicking matcher sibling", err: errors.Join(&panicMatcherError{}, cause), target: cause, expected: true},
		{name: "panicking unwrap sibling", err: errors.Join(&panicUnwrapError{}, cause), target: cause, expected: true},
		{name: "panicking join sibling", err: errors.Join(&panicJoinError{}, cause), target: cause, expected: true},
		{name: "branching cycle sibling", err: errors.Join(branchingCycle, cause), target: cause, expected: true},
		{name: "non-comparable target", err: testSliceError{"source"}, target: testSliceError{"target"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var matched bool
			assert.NotPanicsf(t, func() {
				matched = errorIs(tc.err, tc.target)
			}, "errorIs should not panic for %s", tc.name)
			assert.Equalf(t, tc.expected, matched, "errorIs should return the correct match for %s", tc.name)
		})
	}
}

func TestErrorAs(t *testing.T) {
	t.Parallel()

	assert.False(t, errorAs(nil, nil), "errorAs should preserve the nil-source short circuit")

	rawURL := &url.Error{URL: "https://example.com", Err: errors.New("cause-token")}
	var directTarget *url.Error
	assert.True(t, errorAs(rawURL, &directTarget), "errorAs should match a directly assignable error")
	assert.Same(t, rawURL, directTarget, "errorAs should return the directly assignable error")

	var typedNil *nilUnsafeError
	typedNilSource := error(typedNil)
	var typedNilTarget *nilUnsafeError
	assert.True(t, errorAs(typedNilSource, &typedNilTarget), "errorAs should match a typed-nil node")
	assert.Nil(t, typedNilTarget, "errorAs should preserve a typed-nil node value")
	var unrelated *url.Error
	assert.False(t, errorAs(typedNilSource, &unrelated), "errorAs should reject an unrelated target after a typed-nil node")

	customTarget := &testAsError{value: "matched"}
	var matchedCustom *testAsError
	assert.True(t, errorAs(&testChainError{asTarget: customTarget}, &matchedCustom), "errorAs should preserve custom As matching")
	assert.Same(t, customTarget, matchedCustom, "errorAs should preserve the custom As result")

	interfaceSource := &testNetError{timeout: true}
	var interfaceTarget timeoutError
	assert.True(t, errorAs(interfaceSource, &interfaceTarget), "errorAs should match a pointer-to-interface target")
	assert.Same(t, interfaceSource, interfaceTarget, "errorAs should return the interface target")
	var unrelatedInterface interface{ Retryable() bool }
	assert.False(t, errorAs(interfaceSource, &unrelatedInterface), "errorAs should reject an unrelated pointer-to-interface target")

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "custom miss unwraps", err: &testChainError{err: rawURL}},
		{name: "panicking matcher unwraps", err: &panicMatcherError{err: rawURL}},
		{name: "typed nil sibling", err: errors.Join(typedNilSource, rawURL)},
		{name: "panicking matcher sibling", err: errors.Join(&panicMatcherError{}, rawURL)},
		{name: "panicking unwrap sibling", err: errors.Join(&panicUnwrapError{}, rawURL)},
		{name: "panicking join sibling", err: errors.Join(&panicJoinError{}, rawURL)},
		{name: "branching cycle sibling", err: errors.Join(branchingSliceCycleError{"branching-cycle-secret-token"}, rawURL)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var target *url.Error
			var matched bool
			assert.NotPanicsf(t, func() {
				matched = errorAs(tc.err, &target)
			}, "errorAs should not panic for %s", tc.name)
			assert.Truef(t, matched, "errorAs should match a later sibling for %s", tc.name)
			assert.Samef(t, rawURL, target, "errorAs should return the later sibling for %s", tc.name)
		})
	}

	earlierURL := &url.Error{URL: "https://earlier.example", Err: errors.New("earlier-token")}
	laterURL := &url.Error{URL: "https://later.example", Err: errors.New("later-token")}
	earlierBranch := error(earlierURL)
	for range maxErrorTreeDepth - 5 {
		earlierBranch = &testChainError{err: earlierBranch}
	}
	orderedJoin := make(testJoinError, 41)
	orderedJoin[0] = earlierBranch
	for i := 1; i < len(orderedJoin)-1; i++ {
		orderedJoin[i] = errors.New("shallow-token")
	}
	orderedJoin[len(orderedJoin)-1] = laterURL
	var orderedTarget *url.Error
	require.True(t, errorAs(orderedJoin, &orderedTarget), "errorAs must preserve a match in an earlier in-budget branch")
	assert.Same(t, earlierURL, orderedTarget, "errorAs should preserve depth-first ordering for in-budget trees")

	assert.Panics(t, func() { errorAs(rawURL, nil) }, "errorAs should panic for a nil target")
	assert.Panics(t, func() { errorAs(rawURL, 1) }, "errorAs should panic for a non-pointer target")
	var nilTarget *url.Error
	assert.Panics(t, func() { errorAs(rawURL, nilTarget) }, "errorAs should panic for a nil target pointer")
	assert.Panics(t, func() { errorAs(rawURL, new(string)) }, "errorAs should panic for a pointer to a non-error type")
}

func TestErrorAsResult(t *testing.T) {
	t.Parallel()

	assert.Equal(t, walkMiss, errorAsResult(nil, nil), "errorAsResult should preserve the nil-source short circuit")
	rawURL := &url.Error{URL: "https://example.com", Err: errors.New("cause-token")}
	var target *url.Error
	assert.Equal(t, walkMatch, errorAsResult(rawURL, &target), "errorAsResult should report a direct match")
	assert.Same(t, rawURL, target, "errorAsResult should return the direct match")
	target = nil
	assert.Equal(t, walkMiss, errorAsResult(errors.New("plain"), &target), "errorAsResult should report a clean miss")
	assert.Equal(t, walkIncomplete, errorAsResult(&testChainError{}, &target), "errorAsResult should fail closed after a custom As miss")
	assert.Equal(t, walkIncomplete, errorAsResult(&panicUnwrapError{}, &target), "errorAsResult should report an incomplete traversal")
	assert.Equal(t, walkMatch, errorAsResult(&panicMatcherError{err: rawURL}, &target), "errorAsResult should inspect a child after a panicking matcher")
	assert.Same(t, rawURL, target, "errorAsResult should return the child after a panicking matcher")
	interfaceSource := &testNetError{timeout: true}
	var interfaceTarget timeoutError
	assert.Equal(t, walkMatch, errorAsResult(interfaceSource, &interfaceTarget), "errorAsResult should match a pointer-to-interface target")
	assert.Same(t, interfaceSource, interfaceTarget, "errorAsResult should return the interface target")
	var unrelatedInterface interface{ Retryable() bool }
	assert.Equal(t, walkMiss, errorAsResult(interfaceSource, &unrelatedInterface), "errorAsResult should reject an unrelated pointer-to-interface target")

	assert.Panics(t, func() { errorAsResult(rawURL, nil) }, "errorAsResult should panic for a nil target")
	assert.Panics(t, func() { errorAsResult(rawURL, 1) }, "errorAsResult should panic for a non-pointer target")
	var nilTarget *url.Error
	assert.Panics(t, func() { errorAsResult(rawURL, nilTarget) }, "errorAsResult should panic for a nil target pointer")
	assert.Panics(t, func() { errorAsResult(rawURL, new(string)) }, "errorAsResult should panic for a pointer to a non-error type")
}

func TestWalkErrorTree(t *testing.T) {
	t.Parallel()

	target := errors.New("target-token")
	unrelated := errors.New("unrelated-token")
	panicMarker := errors.New("panic-marker-token")
	var typedNil *nilUnsafeError
	typedNilSource := error(typedNil)
	cycle := new(cycleError)
	joinCycle := new(cycleJoinError)
	branchingCycle := branchingSliceCycleError{"branching-cycle-secret-token"}
	dynamicallyUnhashable := dynamicallyUnhashableError{value: []string{"secret-token"}}
	dynamicallyUnhashableCycle := dynamicallyUnhashableError{value: []string{"secret-token"}, cycle: true}
	wrapperCycle := new(mutableCycleError)
	safeWrapperCycle := Error(wrapperCycle)
	wrapperCycle.err = safeWrapperCycle
	safeMiss := Error(unrelated)
	safeURLMiss := URLRequestError(&url.Error{URL: "https://example.com", Err: unrelated})
	budgetExhaustion := make(testJoinError, maxErrorTreeNodes+1)
	budgetExhaustion[0] = safeMiss
	budgetBoundary := make(testJoinError, maxErrorTreeNodes-1)
	for i := range len(budgetBoundary) - 1 {
		budgetBoundary[i] = unrelated
	}
	budgetBoundary[len(budgetBoundary)-1] = fmt.Errorf("boundary: %w", target)
	fallbackExhaustor := make(testJoinError, maxErrorTreeNodes-1)
	for i := range fallbackExhaustor {
		fallbackExhaustor[i] = unrelated
	}
	hostileFallback := make(testJoinError, maxErrorTreeNodes*2)
	for i := range hostileFallback {
		hostileFallback[i] = unrelated
	}
	forceFallback := func(branches ...error) error {
		root := make(testJoinError, 1, len(branches)+1)
		root[0] = fallbackExhaustor
		return append(root, branches...)
	}
	fallbackDuplicate := new(testNetError)
	fallbackDuplicateCalls := 0
	fallbackDuplicateMatch := func(current error) bool {
		candidate, ok := current.(*testNetError)
		if !ok || candidate != fallbackDuplicate {
			return false
		}
		fallbackDuplicateCalls++
		return fallbackDuplicateCalls == 2
	}
	fallbackAtLimit := make([]error, maxErrorTreeNodes-1)
	for i := range len(fallbackAtLimit) - 1 {
		fallbackAtLimit[i] = unrelated
	}
	fallbackAtLimit[len(fallbackAtLimit)-1] = target
	fallbackAfterLimit := make([]error, maxErrorTreeNodes)
	for i := range len(fallbackAfterLimit) - 1 {
		fallbackAfterLimit[i] = unrelated
	}
	fallbackAfterLimit[len(fallbackAfterLimit)-1] = target
	workConserving := make(testJoinError, 41)
	earlyTarget := target
	for range maxErrorTreeDepth - 5 {
		earlyTarget = &testChainError{err: earlyTarget}
	}
	workConserving[0] = earlyTarget
	for i := 1; i < len(workConserving); i++ {
		workConserving[i] = unrelated
	}
	matchTarget := func(current error) bool { return current == target }
	for _, tc := range []struct {
		name     string
		err      error
		match    func(error) bool
		expected walkResult
	}{
		{name: "nil", match: matchTarget},
		{name: "direct match", err: target, match: matchTarget, expected: walkMatch},
		{name: "leaf miss", err: unrelated, match: matchTarget},
		{name: "single unwrap match", err: fmt.Errorf("outer: %w", target), match: matchTarget, expected: walkMatch},
		{name: "single unwrap nil", err: &testChainError{}, match: matchTarget},
		{name: "join child match", err: testJoinError{nil, unrelated, target}, match: matchTarget, expected: walkMatch},
		{name: "join children miss", err: testJoinError{nil, unrelated}, match: matchTarget},
		{name: "typed nil branch", err: typedNilSource, match: matchTarget, expected: walkIncomplete},
		{name: "matching panic", err: unrelated, match: func(error) bool { panic("match panic token") }, expected: walkIncomplete},
		{name: "matching panic unwrap match", err: &panicMatcherError{err: target}, match: matchTarget, expected: walkMatch},
		{name: "matching panic sibling", err: testJoinError{panicMarker, target}, match: func(current error) bool {
			if current == panicMarker {
				panic("match panic token")
			}
			return current == target
		}, expected: walkMatch},
		{name: "single unwrap panic", err: &panicUnwrapError{}, match: matchTarget, expected: walkIncomplete},
		{name: "single unwrap panic sibling", err: testJoinError{&panicUnwrapError{}, target}, match: matchTarget, expected: walkMatch},
		{name: "join unwrap panic", err: &panicJoinError{}, match: matchTarget, expected: walkIncomplete},
		{name: "join unwrap panic sibling", err: testJoinError{&panicJoinError{}, target}, match: matchTarget, expected: walkMatch},
		{name: "single cycle", err: cycle, match: matchTarget, expected: walkIncomplete},
		{name: "single cycle sibling", err: testJoinError{cycle, target}, match: matchTarget, expected: walkMatch},
		{name: "join cycle", err: joinCycle, match: matchTarget, expected: walkIncomplete},
		{name: "join cycle sibling", err: testJoinError{joinCycle, target}, match: matchTarget, expected: walkMatch},
		{name: "branching cycle", err: branchingCycle, match: matchTarget, expected: walkIncomplete},
		{name: "branching cycle sibling", err: testJoinError{branchingCycle, target}, match: matchTarget, expected: walkMatch},
		{name: "non-comparable cycle depth", err: sliceCycleError{"cycle"}, match: matchTarget, expected: walkIncomplete},
		{name: "dynamically unhashable miss", err: dynamicallyUnhashable, match: matchTarget},
		{name: "dynamically unhashable cycle", err: dynamicallyUnhashableCycle, match: matchTarget, expected: walkIncomplete},
		{name: "dynamically unhashable cycle sibling", err: testJoinError{dynamicallyUnhashableCycle, target}, match: matchTarget, expected: walkMatch},
		{name: "safe error miss", err: safeMiss, match: matchTarget},
		{name: "safe URL error miss", err: safeURLMiss, match: matchTarget},
		{name: "node budget exhaustion", err: budgetExhaustion, match: matchTarget, expected: walkIncomplete},
		{name: "single unwrap at node budget", err: budgetBoundary, match: matchTarget, expected: walkMatch},
		{name: "fallback typed nil miss", err: forceFallback(typedNilSource), match: matchTarget, expected: walkIncomplete},
		{name: "fallback typed nil match", err: forceFallback(typedNilSource), match: func(current error) bool { return current == typedNilSource }, expected: walkMatch},
		{name: "fallback typed nil match panic", err: forceFallback(typedNilSource), match: func(current error) bool {
			if isNilValue(current) {
				panic("fallback typed nil match panic token")
			}
			return false
		}, expected: walkIncomplete},
		{name: "fallback matching panic sibling", err: forceFallback(panicMarker, target), match: func(current error) bool {
			if current == panicMarker {
				panic("fallback match panic token")
			}
			return current == target
		}, expected: walkMatch},
		{name: "fallback duplicate occurrence", err: forceFallback(fallbackDuplicate, fallbackDuplicate), match: fallbackDuplicateMatch, expected: walkMatch},
		{name: "fallback safe error", err: forceFallback(Error(target)), match: matchTarget, expected: walkMatch},
		{name: "fallback safe URL error", err: forceFallback(URLRequestError(&url.Error{URL: "https://example.com", Err: target})), match: matchTarget, expected: walkMatch},
		{name: "fallback single unwrap panic sibling", err: forceFallback(&panicUnwrapError{}, target), match: matchTarget, expected: walkMatch},
		{name: "fallback join unwrap match", err: forceFallback(testJoinError{target}), match: matchTarget, expected: walkMatch},
		{name: "fallback join unwrap panic sibling", err: forceFallback(&panicJoinError{}, target), match: matchTarget, expected: walkMatch},
		{name: "fallback empty join sibling", err: forceFallback(testJoinError{}, target), match: matchTarget, expected: walkMatch},
		{name: "fallback node limit inclusive", err: forceFallback(fallbackAtLimit...), match: matchTarget, expected: walkMatch},
		{name: "fallback node limit exclusive", err: forceFallback(fallbackAfterLimit...), match: matchTarget, expected: walkIncomplete},
		{name: "fallback preserves wrapped parent sibling", err: testJoinError{hostileFallback, fmt.Errorf("later sibling: %w", target)}, match: matchTarget, expected: walkMatch},
		{name: "work-conserving siblings", err: workConserving, match: matchTarget, expected: walkMatch},
		{name: "safe wrapper cycle", err: safeWrapperCycle, match: matchTarget, expected: walkIncomplete},
		{name: "safe wrapper cycle sibling", err: testJoinError{safeWrapperCycle, target}, match: matchTarget, expected: walkMatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var result walkResult
			assert.NotPanicsf(t, func() {
				result = walkErrorTree(tc.err, tc.match)
			}, "walkErrorTree should not panic for %s", tc.name)
			assert.Equalf(t, tc.expected, result, "walkErrorTree should return the correct result for %s", tc.name)
		})
	}
}

func TestSafeErrorsDoNotUnwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause-token")
	assert.NoError(t, errors.Unwrap(Error(cause)), "errors.Unwrap should not expose the safeError source")
	assert.NoError(t, errors.Unwrap(URLRequestError(&url.Error{URL: "https://example.com", Err: cause})), "errors.Unwrap should not expose the safeURLError source")
}

func TestSafeErrorsAsInvalidTarget(t *testing.T) {
	t.Parallel()

	safeErr := Error(errors.New("cause-token"))
	as := errors.As
	assert.Panics(t, func() { as(safeErr, nil) }, "errors.As should preserve its nil-target panic")
	assert.Panics(t, func() { as(safeErr, 1) }, "errors.As should preserve its non-pointer-target panic")
	var nilTarget *url.Error
	assert.Panics(t, func() { as(safeErr, nilTarget) }, "errors.As should preserve its nil-pointer-target panic")
	assert.Panics(t, func() { as(safeErr, new(string)) }, "errors.As should preserve its non-error-target panic")
}

func TestSafeErrorsGoString(t *testing.T) {
	t.Parallel()

	retained := dynamicallyUnhashableError{value: []string{"retained-secret-token"}}
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "error", err: Error(retained)},
		{name: "URL error", err: URLRequestError(&url.Error{Op: "GET", URL: "https://user-token:password-token@example.com/path-token?signature=query-token", Err: retained})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			goStringer, ok := tc.err.(fmt.GoStringer)
			require.True(t, ok, "safe errors must implement fmt.GoStringer")
			assert.Equal(t, tc.err.Error(), goStringer.GoString(), "GoString should return safe display text")
			for _, err := range []error{tc.err, fmt.Errorf("outer: %w", tc.err)} {
				for _, format := range []string{"%#v", "%#s", "%#q", "%#x", "%+v", "%v", "%s", "%q", "%x", "%X", "%d", "%o", "%b", "%c", "%U", "%e", "%E", "%f", "%F", "%g", "%G", "%p", "%T", "%#12.5f", "%w", "%+w", "%#w", "%12w"} {
					formatted := fmt.Sprintf(format, err)
					for _, secret := range []string{"retained-secret-token", "user-token", "password-token", "path-token", "query-token"} {
						assert.NotContains(t, formatted, secret, "safe error formatting should omit retained error data")
					}
				}
			}
		})
	}
}

func TestSafeErrorFormat(t *testing.T) {
	t.Parallel()

	safeErr, ok := Error(errors.New("cause-secret-token")).(*safeError)
	require.True(t, ok, "Error must return a safeError")
	state := new(testFormatState)
	safeErr.Format(state, 'f')
	assert.Equal(t, safeErr.Error(), state.String(), "safeError.Format should emit only safe display text")
	assert.NotContains(t, state.String(), "cause-secret-token", "safeError.Format should omit retained error text")
}

func TestSafeURLErrorFormat(t *testing.T) {
	t.Parallel()

	safeErr, ok := URLRequestError(&url.Error{
		URL: "https://example.com",
		Err: errors.New("cause-secret-token"),
	}).(*safeURLError)
	require.True(t, ok, "URLRequestError must return a safeURLError")
	state := new(testFormatState)
	safeErr.Format(state, 'c')
	assert.Equal(t, safeErr.Error(), state.String(), "safeURLError.Format should emit only safe display text")
	assert.NotContains(t, state.String(), "secret-token", "safeURLError.Format should omit retained error text")
}

func TestSafeURLErrorNetError(t *testing.T) {
	t.Parallel()

	var typedNil *testNetError
	var typedNilUnwrapper *nilUnsafeError
	cycle := new(cycleError)
	joinCycle := new(cycleJoinError)
	branchingCycle := branchingSliceCycleError{"branching-cycle-secret-token"}
	urlCycle := new(url.Error)
	urlCycle.Err = urlCycle
	for _, tc := range []struct {
		name          string
		cause         error
		wantTimeout   bool
		wantTemporary bool
	}{
		{name: "plain error", cause: errors.New("plain-error-token")},
		{name: "timeout", cause: &testNetError{timeout: true}, wantTimeout: true},
		{name: "timeout only", cause: &timeoutOnlyError{timeout: true}, wantTimeout: true},
		{name: "temporary", cause: &testNetError{temporary: true}, wantTemporary: true},
		{name: "timeout and temporary", cause: &testNetError{timeout: true, temporary: true}, wantTimeout: true, wantTemporary: true},
		{name: "nested timeout", cause: &url.Error{Op: "nested", URL: "https://nested-user:nested-password@example.net/nested-path?signature=nested-query", Err: &testNetError{timeout: true}}, wantTimeout: true},
		{name: "nested temporary", cause: &url.Error{Op: "nested", URL: "https://nested-user:nested-password@example.net/nested-path?signature=nested-query", Err: &testNetError{temporary: true}}, wantTemporary: true},
		{name: "nested typed nil", cause: &url.Error{Op: "nested", URL: "https://nested-user:nested-password@example.net/nested-path?signature=nested-query", Err: typedNil}},
		{name: "typed nil sibling", cause: errors.Join(typedNilUnwrapper, &testNetError{timeout: true, temporary: true})},
		{name: "panicking matcher sibling", cause: errors.Join(&panicMatcherError{}, &testNetError{timeout: true, temporary: true})},
		{name: "panicking unwrap sibling", cause: errors.Join(&panicUnwrapError{}, &testNetError{timeout: true, temporary: true})},
		{name: "panicking join sibling", cause: errors.Join(&panicJoinError{}, &testNetError{timeout: true, temporary: true})},
		{name: "panicking network classification", cause: &panicNetError{}},
		{name: "panicking network sibling", cause: errors.Join(&panicNetError{}, &testNetError{timeout: true, temporary: true})},
		{name: "single cycle", cause: cycle},
		{name: "join cycle", cause: joinCycle},
		{name: "branching cycle", cause: branchingCycle},
		{name: "URL cycle", cause: urlCycle},
		{name: "cycle sibling", cause: errors.Join(cycle, &testNetError{timeout: true, temporary: true})},
		{name: "branching cycle sibling", cause: errors.Join(branchingCycle, &testNetError{timeout: true, temporary: true})},
		{name: "URL cycle sibling", cause: errors.Join(urlCycle, &testNetError{timeout: true, temporary: true})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := URLRequestError(&url.Error{Op: "GET", URL: "https://example.com/path-token", Err: tc.cause})
			netErr, ok := err.(net.Error)
			require.True(t, ok, "URLRequestError must preserve net.Error compatibility")
			var timeout bool
			assert.NotPanics(t, func() {
				timeout = netErr.Timeout()
			}, "Timeout should not panic while classifying network errors")
			assert.Equal(t, tc.wantTimeout, timeout, "Timeout should preserve the original classification")
			safeErr, ok := err.(*safeURLError)
			require.True(t, ok, "URLRequestError must return a safe URL error")
			var temporary bool
			assert.NotPanics(t, func() {
				temporary = safeErr.Temporary()
			}, "Temporary should not panic while classifying network errors")
			assert.Equal(t, tc.wantTemporary, temporary, "Temporary should preserve the original classification")
			assert.NotContains(t, err.Error(), "token", "net.Error compatibility should not expose error details")
			assert.NotContains(t, fmt.Sprintf("%+v", err), "nested-password", "URLRequestError formatting should omit nested userinfo")
			assert.NotContains(t, fmt.Sprintf("%#v", err), "nested-query", "URLRequestError formatting should omit nested queries")
			assert.ErrorIs(t, err, tc.cause, "URLRequestError should preserve the nested cause")
			var target *url.Error
			assert.ErrorAs(t, err, &target, "URLRequestError should preserve nested URL error inspection")
		})
	}
}

func TestSafeURLErrorTopLevelNetworkClassification(t *testing.T) {
	t.Parallel()

	rawURL := &url.Error{URL: "https://example.com/path-token", Err: &testNetError{timeout: true, temporary: true}}
	nestedSafeURL := &url.Error{Err: URLRequestError(rawURL)}
	for _, tc := range []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "URL error", err: rawURL, expected: true},
		{name: "URL error containing safe URL error", err: nestedSafeURL, expected: true},
		{name: "wrapped URL error", err: fmt.Errorf("wrapped-error-token: %w", rawURL)},
		{name: "joined URL error", err: errors.Join(errors.New("joined-error-token"), rawURL)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			safeErr, ok := URLRequestError(tc.err).(*safeURLError)
			require.True(t, ok, "URLRequestError must return a safeURLError")
			assert.Equal(t, tc.expected, safeErr.Timeout(), "safeURLError.Timeout should preserve top-level classification")
			assert.Equal(t, tc.expected, safeErr.Temporary(), "safeURLError.Temporary should preserve top-level classification")
		})
	}
}

func TestIsTimeout(t *testing.T) {
	t.Parallel()

	var typedNil *testNetError
	urlCycle := new(url.Error)
	urlCycle.Err = urlCycle
	rawURLTimeout := &url.Error{Err: &testNetError{timeout: true}}
	nestedSafeURLTimeout := &url.Error{Err: URLRequestError(rawURLTimeout)}
	for _, tc := range []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil"},
		{name: "plain error", err: errors.New("plain-error-token")},
		{name: "timeout", err: &testNetError{timeout: true}, expected: true},
		{name: "non-timeout network error", err: &testNetError{}},
		{name: "timeout only", err: &timeoutOnlyError{timeout: true}},
		{name: "URL timeout only", err: &url.Error{Err: &timeoutOnlyError{timeout: true}}, expected: true},
		{name: "safe URL timeout", err: URLRequestError(rawURLTimeout), expected: true},
		{name: "URL containing safe URL timeout", err: nestedSafeURLTimeout, expected: true},
		{name: "safe URL containing safe URL timeout", err: URLRequestError(nestedSafeURLTimeout), expected: true},
		{name: "safe wrapped URL timeout", err: URLRequestError(fmt.Errorf("wrapped-error-token: %w", rawURLTimeout))},
		{name: "safe joined URL timeout", err: URLRequestError(errors.Join(errors.New("joined-error-token"), rawURLTimeout))},
		{name: "wrapped timeout", err: fmt.Errorf("wrapped-error-token: %w", &testNetError{timeout: true})},
		{name: "joined timeout", err: errors.Join(errors.New("joined-error-token"), &testNetError{timeout: true})},
		{name: "typed nil", err: typedNil},
		{name: "panicking timeout", err: &panicNetError{}},
		{name: "URL cycle", err: urlCycle},
		{name: "poisoned sibling before timeout", err: errors.Join(&panicNetError{}, &testNetError{timeout: true})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var timeout bool
			assert.NotPanics(t, func() {
				timeout = IsTimeout(tc.err)
			}, "IsTimeout should not panic while classifying a transport error")
			assert.Equal(t, tc.expected, timeout, "IsTimeout should return the correct timeout classification")
		})
	}
}

func TestClassifyNetworkError(t *testing.T) {
	t.Parallel()

	var typedNil *testNetError
	var typedNilUnwrapper *nilUnsafeError
	cycle := new(cycleError)
	joinCycle := new(cycleJoinError)
	branchingCycle := branchingSliceCycleError{"branching-cycle-secret-token"}
	orderedClassifiers := make(testJoinError, 41)
	earlyClassifier := error(&timeoutOnlyError{timeout: true})
	for range maxErrorTreeDepth - 5 {
		earlyClassifier = &testChainError{err: earlyClassifier}
	}
	orderedClassifiers[0] = earlyClassifier
	for i := 1; i < len(orderedClassifiers); i++ {
		orderedClassifiers[i] = &timeoutOnlyError{}
	}
	urlCycle := new(url.Error)
	urlCycle.Err = urlCycle
	urlDepthBoundary := error(&testNetError{timeout: true, temporary: true})
	for range maxErrorTreeDepth - 1 {
		urlDepthBoundary = &url.Error{Err: urlDepthBoundary}
	}
	urlDepthExceeded := &url.Error{Err: urlDepthBoundary}
	rawURLNetworkError := &url.Error{Err: &testNetError{timeout: true, temporary: true}}
	safeURLNetworkError := URLRequestError(rawURLNetworkError)
	urlSafeNetworkError := &url.Error{Err: safeURLNetworkError}
	sanitizedURLSafeNetworkError := URLRequestError(urlSafeNetworkError)
	urlSafeCycle := new(url.Error)
	safeURLCycle := &safeURLError{retained: func() error { return urlSafeCycle }}
	urlSafeCycle.Err = safeURLCycle
	for _, tc := range []struct {
		name     string
		err      error
		timeout  bool
		expected bool
	}{
		{name: "nil", timeout: true},
		{name: "plain", err: errors.New("plain")},
		{name: "timeout", err: &testNetError{timeout: true}, timeout: true, expected: true},
		{name: "non-timeout network error", err: &testNetError{}, timeout: true},
		{name: "timeout only", err: &timeoutOnlyError{timeout: true}, timeout: true},
		{name: "URL timeout only", err: &url.Error{Err: &timeoutOnlyError{timeout: true}}, timeout: true, expected: true},
		{name: "URL without timeout", err: &url.Error{Err: errors.New("plain")}, timeout: true},
		{name: "temporary", err: &testNetError{temporary: true}, expected: true},
		{name: "non-temporary network error", err: &testNetError{}},
		{name: "temporary only", err: &temporaryOnlyError{temporary: true}},
		{name: "URL temporary only", err: &url.Error{Err: &temporaryOnlyError{temporary: true}}, expected: true},
		{name: "URL without temporary", err: &url.Error{Err: errors.New("plain")}},
		{name: "safe URL timeout", err: URLRequestError(rawURLNetworkError), timeout: true, expected: true},
		{name: "safe URL temporary", err: URLRequestError(rawURLNetworkError), expected: true},
		{name: "URL containing safe URL timeout", err: urlSafeNetworkError, timeout: true, expected: true},
		{name: "URL containing safe URL temporary", err: urlSafeNetworkError, expected: true},
		{name: "safe URL containing safe URL timeout", err: sanitizedURLSafeNetworkError, timeout: true, expected: true},
		{name: "safe URL containing safe URL temporary", err: sanitizedURLSafeNetworkError, expected: true},
		{name: "safe wrapped URL timeout", err: URLRequestError(fmt.Errorf("wrapped-error-token: %w", rawURLNetworkError)), timeout: true},
		{name: "safe wrapped URL temporary", err: URLRequestError(fmt.Errorf("wrapped-error-token: %w", rawURLNetworkError))},
		{name: "typed nil timeout", err: typedNil, timeout: true},
		{name: "typed nil temporary", err: typedNil},
		{name: "nested timeout", err: &url.Error{Err: &url.Error{Err: &testNetError{timeout: true}}}, timeout: true, expected: true},
		{name: "nested temporary", err: &url.Error{Err: &url.Error{Err: &testNetError{temporary: true}}}, expected: true},
		{name: "nested typed nil timeout", err: &url.Error{Err: &url.Error{Err: typedNil}}, timeout: true},
		{name: "nested typed nil temporary", err: &url.Error{Err: &url.Error{Err: typedNil}}},
		{name: "typed nil timeout sibling", err: errors.Join(typedNilUnwrapper, &testNetError{timeout: true}), timeout: true},
		{name: "typed nil temporary sibling", err: errors.Join(typedNilUnwrapper, &testNetError{temporary: true})},
		{name: "panicking matcher sibling", err: errors.Join(&panicMatcherError{}, &testNetError{timeout: true}), timeout: true},
		{name: "panicking unwrap sibling", err: errors.Join(&panicUnwrapError{}, &testNetError{timeout: true}), timeout: true},
		{name: "panicking join sibling", err: errors.Join(&panicJoinError{}, &testNetError{temporary: true})},
		{name: "panicking timeout classification", err: &panicNetError{}, timeout: true},
		{name: "panicking temporary classification", err: &panicNetError{}},
		{name: "panicking timeout sibling", err: errors.Join(&panicNetError{}, &testNetError{timeout: true}), timeout: true},
		{name: "panicking temporary sibling", err: errors.Join(&panicNetError{}, &testNetError{temporary: true})},
		{name: "first timeout false is terminal", err: errors.Join(&testNetError{}, &testNetError{timeout: true}), timeout: true},
		{name: "joined classifier order", err: orderedClassifiers, timeout: true},
		{name: "first temporary false is terminal", err: errors.Join(&testNetError{}, &testNetError{temporary: true})},
		{name: "single cycle", err: cycle, timeout: true},
		{name: "join cycle", err: joinCycle},
		{name: "branching cycle", err: branchingCycle},
		{name: "URL cycle", err: urlCycle, timeout: true},
		{name: "URL safe cycle timeout", err: urlSafeCycle, timeout: true},
		{name: "URL safe cycle temporary", err: urlSafeCycle},
		{name: "URL timeout depth boundary", err: urlDepthBoundary, timeout: true, expected: true},
		{name: "URL temporary depth boundary", err: urlDepthBoundary, expected: true},
		{name: "URL timeout depth exceeded", err: urlDepthExceeded, timeout: true},
		{name: "URL temporary depth exceeded", err: urlDepthExceeded},
		{name: "cycle timeout sibling", err: errors.Join(cycle, &testNetError{timeout: true}), timeout: true},
		{name: "branching cycle timeout sibling", err: errors.Join(branchingCycle, &testNetError{timeout: true}), timeout: true},
		{name: "URL cycle temporary sibling", err: errors.Join(urlCycle, &testNetError{temporary: true})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.NotPanics(t, func() {
				assert.Equal(t, tc.expected, classifyNetworkError(tc.err, tc.timeout), "classifyNetworkError should return the correct classification")
			}, "classifyNetworkError should not panic")
		})
	}
}

func TestIsNilValue(t *testing.T) {
	t.Parallel()

	var typedNil *testNetError
	assert.True(t, isNilValue(nil), "isNilValue should identify nil")
	assert.True(t, isNilValue(typedNil), "isNilValue should identify a typed-nil pointer")
	assert.False(t, isNilValue(errors.New("not nil")), "isNilValue should reject non-nil values")
	assert.False(t, isNilValue(1), "isNilValue should reject values which cannot be nil")
}
