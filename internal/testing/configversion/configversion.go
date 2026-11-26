// Package configversion provides test helpers for config version migrations
package configversion

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/config/versions"
)

// Number returns the number that v's type is registered under in versions.Manager
func Number(tb testing.TB, v any) uint16 {
	tb.Helper()
	latest, err := versions.Manager.Latest()
	require.NoError(tb, err, "Latest must not error")
	for i := range latest + 1 {
		if reflect.TypeOf(versions.Manager.Version(i)) == reflect.TypeOf(v) {
			return i
		}
	}
	require.FailNowf(tb, "version must be registered", "%T is not registered", v)
	return 0
}

// Migrate checks that the registered version v upgrades before into after, downgrades that result into downgraded and
// upgrades the downgraded config back into after
// Each document's version is set from v's registered number, so renumbering a version leaves its tests unchanged
func Migrate(tb testing.TB, v any, before, after, downgraded string) {
	tb.Helper()
	n := Number(tb, v)
	require.Positive(tb, n, "Migrate must be given a version with a previous version")
	upgraded, err := versions.Manager.Deploy(tb.Context(), withVersion(tb, before, n-1), n)
	require.NoErrorf(tb, err, "Deploy must upgrade to version %d", n)
	assert.JSONEqf(tb, string(withVersion(tb, after, n)), string(upgraded), "Deploy should upgrade to version %d", n)
	downgradedConfig, err := versions.Manager.Deploy(tb.Context(), upgraded, n-1)
	require.NoErrorf(tb, err, "Deploy must downgrade from version %d", n)
	assert.JSONEqf(tb, string(withVersion(tb, downgraded, n-1)), string(downgradedConfig), "Deploy should downgrade from version %d", n)
	upgradedAgain, err := versions.Manager.Deploy(tb.Context(), downgradedConfig, n)
	require.NoErrorf(tb, err, "Deploy must upgrade the downgraded config to version %d", n)
	assert.JSONEqf(tb, string(upgraded), string(upgradedAgain), "Deploy should upgrade the downgraded config to the same version %d config", n)
}

func withVersion(tb testing.TB, doc string, version uint16) []byte {
	tb.Helper()
	out, err := jsonparser.Set([]byte(doc), []byte(strconv.FormatUint(uint64(version), 10)), "version")
	require.NoError(tb, err, "jsonparser.Set must set the version")
	return out
}
