package configversion

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v0 "github.com/thrasher-corp/gocryptotrader/config/versions/v0"
	v16 "github.com/thrasher-corp/gocryptotrader/config/versions/v16"
)

const (
	v16Before     = `{"gctscript":{"enabled":true},"logging":{"subloggers":[{"name":"GCTSCRIPT"}]}}`
	v16After      = `{"logging":{"subloggers":[{"name":"GCTSCRIPT"}]}}`
	v16Downgraded = `{"logging":{"subloggers":[{"name":"GCTSCRIPT"}]},"gctscript":{"enabled":false,"timeout":30000000000,"max_virtual_machines":10,"allow_imports":false,"auto_load":null,"verbose":false}}`
)

// failureRecorder records the failures reported through it instead of failing its test
type failureRecorder struct {
	testing.TB
	failures []string
}

func (r *failureRecorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

// FailNow ends the calling goroutine, as testing.T's FailNow does, without failing the embedded test
func (r *failureRecorder) FailNow() {
	runtime.Goexit()
}

// recordFailures runs fn on its own goroutine, which FailNow may end, and returns the failures fn reported
func recordFailures(t *testing.T, fn func(testing.TB)) []string {
	t.Helper()
	r := &failureRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(r)
	}()
	<-done
	return r.failures
}

func TestNumber(t *testing.T) {
	t.Parallel()

	t.Run("registered version", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, uint16(16), Number(t, new(v16.Version)), "Number should return the registered number")
	})

	t.Run("unregistered type", func(t *testing.T) {
		t.Parallel()
		failures := recordFailures(t, func(tb testing.TB) {
			tb.Helper()
			Number(tb, new(struct{}))
		})
		require.Len(t, failures, 1, "Number must report one failure")
		assert.Contains(t, failures[0], "*struct {} is not registered", "Number should name the unregistered type")
	})
}

func TestMigrate(t *testing.T) {
	t.Parallel()

	t.Run("matching documents", func(t *testing.T) {
		t.Parallel()
		Migrate(t, new(v16.Version), v16Before, v16After, v16Downgraded)
	})

	for _, tc := range []struct {
		name       string
		after      string
		downgraded string
		message    string
	}{
		{name: "unexpected upgrade", after: v16Before, downgraded: v16Downgraded, message: "Deploy should upgrade to version 16"},
		{name: "unexpected downgrade", after: v16After, downgraded: v16After, message: "Deploy should downgrade from version 16"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failures := recordFailures(t, func(tb testing.TB) {
				tb.Helper()
				Migrate(tb, new(v16.Version), v16Before, tc.after, tc.downgraded)
			})
			require.Len(t, failures, 1, "Migrate must report one failure")
			assert.Contains(t, failures[0], tc.message, "Migrate should name the step that failed")
		})
	}

	t.Run("version without a previous version", func(t *testing.T) {
		t.Parallel()
		failures := recordFailures(t, func(tb testing.TB) {
			tb.Helper()
			Migrate(tb, new(v0.Version), `{}`, `{}`, `{}`)
		})
		require.Len(t, failures, 1, "Migrate must report one failure")
		assert.Contains(t, failures[0], "given a version with a previous version", "Migrate should refuse version 0")
	})
}

func TestWithVersion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		doc  string
		exp  string
	}{
		{name: "adds a version", doc: `{"name":"test"}`, exp: `{"name":"test","version":7}`},
		{name: "replaces a version", doc: `{"name":"test","version":1}`, exp: `{"name":"test","version":7}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.JSONEq(t, tc.exp, string(withVersion(t, tc.doc, 7)), "withVersion should set the version")
		})
	}

	t.Run("non-object document", func(t *testing.T) {
		t.Parallel()
		failures := recordFailures(t, func(tb testing.TB) {
			tb.Helper()
			withVersion(tb, `"text"`, 7)
		})
		require.Len(t, failures, 1, "withVersion must report one failure")
		assert.Contains(t, failures[0], "set the version", "withVersion should report the failed edit")
	})
}
