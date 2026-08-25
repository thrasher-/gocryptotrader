// Package logsafe provides fail-closed formatting for values which may contain
// credentials and are intended for logs or other diagnostics.
package logsafe

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"runtime"
)

const (
	redactedURL          = "[redacted URL]"
	maxErrorTreeDepth    = 100
	maxErrorTreeNodes    = 4096
	maxErrorMethodFrames = 256
)

type walkResult uint8

const (
	walkMiss walkResult = iota
	walkMatch
	walkIncomplete
)

// callErrorMethod invokes an arbitrary error method while rejecting synchronous
// re-entry through logsafe on the same goroutine. The fixed capture bounds the
// stack inspection; a truncated stack fails closed.
//
//go:noinline
func callErrorMethod(callback func()) bool {
	var callers [maxErrorMethodFrames]uintptr
	count := runtime.Callers(1, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	current, more := frames.Next()
	for more {
		var frame runtime.Frame
		frame, more = frames.Next()
		if frame.Entry == current.Entry {
			return false
		}
	}
	if count == len(callers) {
		return false
	}
	callback()
	return true
}

func callErrorMethodValue[T any](callback func() T) (result T, incomplete bool) {
	completed := callErrorMethod(func() { result = callback() })
	return result, !completed
}

// URL returns only the scheme and host of an absolute URL. All other URL
// components are omitted because credentials can occur in userinfo, paths,
// queries, and fragments.
func URL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" {
		return redactedURL
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}

// URLRequestError returns an error with safe display text when err contains a
// *url.Error. The original error remains available through errors.Is and
// errors.As.
func URLRequestError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*safeURLError); ok {
		return err
	}
	var urlErr *url.Error
	switch errorAsResult(err, &urlErr) {
	case walkIncomplete:
		return Error(err)
	case walkMiss:
		return err
	}
	if urlErr == nil {
		return &safeURLError{
			url:       redactedURL,
			causeType: "<nil>",
			retained:  func() error { return err },
		}
	}
	return &safeURLError{
		url:       URL(urlErr.URL),
		causeType: fmt.Sprintf("%T", urlErr.Err),
		retained:  func() error { return err },
	}
}

// Error returns metadata-only display text while retaining err for
// errors.Is and errors.As.
func Error(err error) error {
	if err == nil {
		return nil
	}
	switch err.(type) {
	case *safeError, *safeURLError:
		return err
	}
	return &safeError{
		errorType: fmt.Sprintf("%T", err),
		retained:  func() error { return err },
	}
}

type safeURLError struct {
	url       string
	causeType string
	retained  func() error
}

type timeoutError interface {
	Timeout() bool
}

type temporaryError interface {
	Temporary() bool
}

// IsTimeout reports the top-level net.Error timeout classification. Typed-nil,
// cyclic, and panicking implementations fail closed.
func IsTimeout(err error) bool {
	return classifyNetworkError(err, true)
}

func (e *safeURLError) Error() string {
	return fmt.Sprintf("request to %s failed (%s)", e.url, e.causeType)
}

// GoString prevents Go-syntax formatting from reflecting retained error data.
func (e *safeURLError) GoString() string {
	return e.Error()
}

// Format prevents any formatting verb from reflecting retained error data.
func (e *safeURLError) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(e.Error()))
}

func (e *safeURLError) Is(target error) bool {
	return errorIs(e.retained(), target)
}

func (e *safeURLError) As(target any) bool {
	return errorAs(e.retained(), target)
}

// Timeout preserves net.Error timeout classification without exposing the
// original error text.
func (e *safeURLError) Timeout() bool {
	return IsTimeout(e.retained())
}

// Temporary preserves net.Error temporary classification without exposing the
// original error text.
func (e *safeURLError) Temporary() bool {
	return classifyNetworkError(e.retained(), false)
}

func classifyNetworkError(err error, timeout bool) bool {
	if isNilValue(err) {
		return false
	}

	insideURLError := false
	for range maxErrorTreeDepth {
		if isNilValue(err) {
			return false
		}
		if safeErr, ok := err.(*safeURLError); ok {
			err = safeErr.retained()
			insideURLError = false
			continue
		}
		if urlErr, ok := err.(*url.Error); ok {
			insideURLError = true
			err = urlErr.Err
			continue
		}

		var classification bool
		classified := func() (classified bool) {
			defer func() {
				if recover() != nil {
					classified = false
				}
			}()
			if !insideURLError {
				if _, ok := err.(net.Error); !ok {
					return false
				}
			}
			if timeout {
				timeoutErr, ok := err.(timeoutError)
				if !ok {
					return false
				}
				value, incomplete := callErrorMethodValue(timeoutErr.Timeout)
				classification = value
				return !incomplete
			}
			temporaryErr, ok := err.(temporaryError)
			if !ok {
				return false
			}
			value, incomplete := callErrorMethodValue(temporaryErr.Temporary)
			classification = value
			return !incomplete
		}()
		return classified && classification
	}
	return false
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	switch reflect.ValueOf(value).Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(value).IsNil()
	default:
		return false
	}
}

type safeError struct {
	errorType string
	retained  func() error
}

func (e *safeError) Error() string {
	return fmt.Sprintf("operation failed (%s)", e.errorType)
}

// GoString prevents Go-syntax formatting from reflecting retained error data.
func (e *safeError) GoString() string {
	return e.Error()
}

// Format prevents any formatting verb from reflecting retained error data.
func (e *safeError) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(e.Error()))
}

func (e *safeError) Is(target error) bool {
	return errorIs(e.retained(), target)
}

func (e *safeError) As(target any) bool {
	return errorAs(e.retained(), target)
}

func errorIs(err, target error) bool {
	if err == nil || target == nil {
		return err == target
	}
	targetComparable := reflect.ValueOf(target).Comparable()
	return walkErrorTree(err, func(current error) bool {
		if targetComparable && reflect.ValueOf(current).Comparable() && current == target {
			return true
		}
		if isNilValue(current) {
			return false
		}
		switch current.(type) {
		case *safeError, *safeURLError:
			return false
		}
		matcher, ok := current.(interface{ Is(error) bool })
		if !ok {
			return false
		}
		matched, incomplete := callErrorMethodValue(func() bool { return matcher.Is(target) })
		return !incomplete && matched
	}) == walkMatch
}

func errorAs(err error, target any) bool {
	return errorAsResult(err, target) == walkMatch
}

func errorAsResult(err error, target any) walkResult {
	if err == nil {
		return walkMiss
	}
	if target == nil {
		panic("errors: target cannot be nil")
	}
	targetValue := reflect.ValueOf(target)
	targetType := targetValue.Type()
	if targetType.Kind() != reflect.Pointer || targetValue.IsNil() {
		panic("errors: target must be a non-nil pointer")
	}
	targetType = targetType.Elem()
	if targetType.Kind() != reflect.Interface && !targetType.Implements(reflect.TypeFor[error]()) {
		panic("errors: *target must be interface or implement error")
	}
	customChecked := false
	result := walkErrorTree(err, func(current error) bool {
		if reflect.TypeOf(current).AssignableTo(targetType) {
			targetValue.Elem().Set(reflect.ValueOf(current))
			return true
		}
		if isNilValue(current) {
			return false
		}
		switch current.(type) {
		case *safeError, *safeURLError:
			return false
		}
		matcher, ok := current.(interface{ As(any) bool })
		if !ok {
			return false
		}
		customChecked = true
		matched, incomplete := callErrorMethodValue(func() bool { return matcher.As(target) })
		return !incomplete && matched
	})
	if result == walkMiss && customChecked {
		return walkIncomplete
	}
	return result
}

func walkErrorTree(err error, match func(error) bool) walkResult {
	seen := make(map[error]struct{})
	type walkItem struct {
		err      error
		children []error
		depth    int
	}
	fallback := make([]walkItem, 0, maxErrorTreeDepth)
	remaining := maxErrorTreeNodes
	var walk func(error, int) walkResult
	walk = func(current error, depth int) walkResult {
		if remaining <= 0 {
			fallback = append(fallback, walkItem{err: current, depth: depth})
			return walkIncomplete
		}
		remaining--
		if current == nil {
			return walkMiss
		}
		if isNilValue(current) {
			matched, _ := func() (matched, panicked bool) {
				defer func() {
					panicked = recover() != nil
				}()
				return match(current), false
			}()
			if matched {
				return walkMatch
			}
			return walkIncomplete
		}
		if depth >= maxErrorTreeDepth {
			return walkIncomplete
		}
		if reflect.ValueOf(current).Comparable() {
			if _, ok := seen[current]; ok {
				return walkIncomplete
			}
			seen[current] = struct{}{}
			defer delete(seen, current)
		}
		matched, matchPanicked := func() (matched, panicked bool) {
			defer func() {
				panicked = recover() != nil
			}()
			return match(current), false
		}()
		if matched {
			return walkMatch
		}
		incomplete := matchPanicked
		switch unwrapping := current.(type) {
		case *safeError:
			result := walk(unwrapping.retained(), depth+1)
			if result == walkMatch {
				return walkMatch
			}
			if incomplete || result == walkIncomplete {
				return walkIncomplete
			}
			return walkMiss
		case *safeURLError:
			result := walk(unwrapping.retained(), depth+1)
			if result == walkMatch {
				return walkMatch
			}
			if incomplete || result == walkIncomplete {
				return walkIncomplete
			}
			return walkMiss
		case interface{ Unwrap() error }:
			child, unwrapIncomplete := func() (unwrapped error, incomplete bool) {
				defer func() {
					if recover() != nil {
						incomplete = true
					}
				}()
				return callErrorMethodValue(unwrapping.Unwrap)
			}()
			if unwrapIncomplete {
				return walkIncomplete
			}
			result := walk(child, depth+1)
			if result == walkMatch {
				return walkMatch
			}
			if incomplete || result == walkIncomplete {
				return walkIncomplete
			}
			return walkMiss
		case interface{ Unwrap() []error }:
			children, unwrapIncomplete := func() (children []error, incomplete bool) {
				defer func() {
					if recover() != nil {
						incomplete = true
					}
				}()
				return callErrorMethodValue(unwrapping.Unwrap)
			}()
			if unwrapIncomplete {
				return walkIncomplete
			}
			for i, child := range children {
				if remaining <= 0 {
					fallback = append(fallback, walkItem{children: children[i:], depth: depth + 1})
					incomplete = true
					break
				}
				result := walk(child, depth+1)
				if result == walkMatch {
					return walkMatch
				}
				if result == walkIncomplete {
					incomplete = true
				}
			}
			if incomplete {
				return walkIncomplete
			}
			return walkMiss
		default:
			if incomplete {
				return walkIncomplete
			}
			return walkMiss
		}
	}
	result := walk(err, 0)
	if result == walkMatch || len(fallback) == 0 {
		return result
	}

	// Once the depth-first budget is exhausted, inspect skipped branches
	// breadth-first so another hostile branch cannot starve later siblings.
	processed := 0
	for i := 0; i < len(fallback) && processed < maxErrorTreeNodes; i++ {
		item := fallback[i]
		if len(item.children) > 0 {
			fallback = append(fallback, walkItem{err: item.children[0], depth: item.depth})
			if len(item.children) > 1 {
				fallback = append(fallback, walkItem{children: item.children[1:], depth: item.depth})
			}
			continue
		}
		processed++
		current := item.err
		depth := item.depth
		if current == nil {
			continue
		}
		if isNilValue(current) {
			matched, _ := func() (matched, panicked bool) {
				defer func() {
					panicked = recover() != nil
				}()
				return match(current), false
			}()
			if matched {
				return walkMatch
			}
			continue
		}
		if depth >= maxErrorTreeDepth {
			continue
		}
		matched, _ := func() (matched, panicked bool) {
			defer func() {
				panicked = recover() != nil
			}()
			return match(current), false
		}()
		if matched {
			return walkMatch
		}
		switch unwrapping := current.(type) {
		case *safeError:
			fallback = append(fallback, walkItem{err: unwrapping.retained(), depth: depth + 1})
		case *safeURLError:
			fallback = append(fallback, walkItem{err: unwrapping.retained(), depth: depth + 1})
		case interface{ Unwrap() error }:
			child, incomplete := func() (unwrapped error, incomplete bool) {
				defer func() {
					if recover() != nil {
						incomplete = true
					}
				}()
				return callErrorMethodValue(unwrapping.Unwrap)
			}()
			if !incomplete {
				fallback = append(fallback, walkItem{err: child, depth: depth + 1})
			}
		case interface{ Unwrap() []error }:
			children, incomplete := func() (children []error, incomplete bool) {
				defer func() {
					if recover() != nil {
						incomplete = true
					}
				}()
				return callErrorMethodValue(unwrapping.Unwrap)
			}()
			if !incomplete && len(children) > 0 {
				fallback = append(fallback, walkItem{children: children, depth: depth + 1})
			}
		}
	}
	return walkIncomplete
}
