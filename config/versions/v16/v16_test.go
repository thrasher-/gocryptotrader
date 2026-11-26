package v16_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/config/versions"
	v16 "github.com/thrasher-corp/gocryptotrader/config/versions/v16"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/configversion"
)

func TestUpgradeConfig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		input    string
		expected string
		exact    bool
	}{
		{name: "missing logging", input: `{"gctscript":{"enabled":true},"keep":true}`, expected: `{"keep":true}`},
		{name: "non-array subloggers", input: `{"logging":{"subloggers":null},"keep":true}`, expected: `{"logging":{"subloggers":null},"keep":true}`},
		{
			name:     "subloggers preserved",
			input:    `{"gctscript":{"enabled":true},"logging":{"subloggers":[{"name":"GCTSCRIPT","level":"DEBUG","output":"stdout"},{"name":"DATABASE","level":"INFO","output":"file"},{"name":"gctscript","level":"ERROR","output":"stderr"}]},"keep":true}`,
			expected: `{"logging":{"subloggers":[{"name":"GCTSCRIPT","level":"DEBUG","output":"stdout"},{"name":"DATABASE","level":"INFO","output":"file"},{"name":"gctscript","level":"ERROR","output":"stderr"}]},"keep":true}`,
		},
		{
			name:     "duplicate subloggers preserved",
			input:    `{"gctscript":{},"logging":{"subloggers":[{"name":"GCTSCRIPT","level":"ERROR","output":"console"},{"name":"DATABASE","level":"ERROR","output":"console"}],"subloggers":[{"name":"DATABASE"}]}}`,
			expected: `{"logging":{"subloggers":[{"name":"GCTSCRIPT","level":"ERROR","output":"console"},{"name":"DATABASE","level":"ERROR","output":"console"}],"subloggers":[{"name":"DATABASE"}]}}`,
			exact:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := new(v16.Version).UpgradeConfig(t.Context(), []byte(tc.input))
			require.NoError(t, err, "UpgradeConfig must not error")
			if tc.exact {
				assert.Equal(t, tc.expected, string(out), "UpgradeConfig should preserve duplicate sublogger arrays exactly")
			} else {
				assert.JSONEq(t, tc.expected, string(out), "UpgradeConfig should remove only the root GCTScript configuration")
			}
		})
	}
}

func TestDowngradeConfig(t *testing.T) {
	t.Parallel()

	out, err := new(v16.Version).DowngradeConfig(t.Context(), []byte(`{"keep":true}`))
	require.NoError(t, err, "DowngradeConfig must not error")
	assert.JSONEq(t, `{"keep":true,"gctscript":{"enabled":false,"timeout":30000000000,"max_virtual_machines":10,"allow_imports":false,"auto_load":null,"verbose":false}}`, string(out), "DowngradeConfig should restore the legacy GCTScript defaults")
}

func TestRegisteredMigration(t *testing.T) {
	t.Parallel()
	configversion.Migrate(t, new(v16.Version),
		`{"gctscript":{"enabled":true},"logging":{"subloggers":[{"name":"GCTSCRIPT"},{"name":"DATABASE"}]}}`,
		`{"logging":{"subloggers":[{"name":"GCTSCRIPT"},{"name":"DATABASE"}]}}`,
		`{"logging":{"subloggers":[{"name":"GCTSCRIPT"},{"name":"DATABASE"}]},"gctscript":{"enabled":false,"timeout":30000000000,"max_virtual_machines":10,"allow_imports":false,"auto_load":null,"verbose":false}}`)
}

func TestRegisteredMultiHopUpgrade(t *testing.T) {
	t.Parallel()

	input := []byte(`{"version":12,"gctscript":{"enabled":true},"currencyConfig":{"cryptocurrencyProvider":{"accountPlan":"hobbyist"}},"exchanges":[{"name":"Bitmex"},{"name":"Kraken"}],"logging":{"subloggers":[{"name":"GCTSCRIPT"},{"name":"DATABASE"}]}}`)
	out, err := versions.Manager.Deploy(t.Context(), input, 16)
	require.NoError(t, err, "Deploy must apply registered migrations from version 12 through version 16")
	assert.JSONEq(t, `{"version":16,"currencyConfig":{"cryptocurrencyProvider":{"accountPlan":"builder"}},"exchanges":[{"name":"Kraken"}],"logging":{"subloggers":[{"name":"GCTSCRIPT"},{"name":"DATABASE"}]}}`, string(out), "Deploy should preserve earlier migrations while removing the root GCTScript configuration")
}
