package v17_test

import (
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v17 "github.com/thrasher-corp/gocryptotrader/config/versions/v17"
	"github.com/thrasher-corp/gocryptotrader/internal/testing/configversion"
)

func TestUpgradeExchange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "both settings with explicit choices",
			input:    `{"name":"Kraken","enabled":true,"orderbook":{"verificationBypass":true,"websocketBufferEnabled":true,"websocketBufferLimit":0},"custom":{"keep":"me"}}`,
			expected: `{"name":"Kraken","enabled":true,"orderbook":{"verificationBypass":true},"custom":{"keep":"me"}}`,
		},
		{
			name:     "only limit",
			input:    `{"name":"Binance","orderbook":{"websocketBufferLimit":5,"verificationBypass":false}}`,
			expected: `{"name":"Binance","orderbook":{"verificationBypass":false}}`,
		},
		{
			name:     "only enabled",
			input:    `{"name":"Gemini","orderbook":{"websocketBufferEnabled":false}}`,
			expected: `{"name":"Gemini","orderbook":{}}`,
		},
		{
			name:     "no orderbook",
			input:    `{"name":"Coinbase","enabled":false}`,
			expected: `{"name":"Coinbase","enabled":false}`,
		},
		{
			name:     "null orderbook",
			input:    `{"name":"Bitstamp","orderbook":null}`,
			expected: `{"name":"Bitstamp","orderbook":null}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := new(v17.Version).UpgradeExchange(t.Context(), []byte(tc.input))
			require.NoError(t, err, "UpgradeExchange must not error")
			assert.JSONEq(t, tc.expected, string(out), "UpgradeExchange should remove only obsolete buffer settings")
		})
	}
}

func TestDowngradeExchange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "restore defaults",
			input:    `{"name":"Kraken","orderbook":{"verificationBypass":true}}`,
			expected: `{"name":"Kraken","orderbook":{"verificationBypass":true,"websocketBufferLimit":5,"websocketBufferEnabled":false}}`,
		},
		{
			name:     "preserve explicit settings",
			input:    `{"name":"Kraken","orderbook":{"websocketBufferLimit":0,"websocketBufferEnabled":true}}`,
			expected: `{"name":"Kraken","orderbook":{"websocketBufferLimit":0,"websocketBufferEnabled":true}}`,
		},
		{
			name:     "restore only missing setting",
			input:    `{"name":"Kraken","orderbook":{"websocketBufferEnabled":true}}`,
			expected: `{"name":"Kraken","orderbook":{"websocketBufferLimit":5,"websocketBufferEnabled":true}}`,
		},
		{
			name:     "no orderbook",
			input:    `{"name":"Coinbase"}`,
			expected: `{"name":"Coinbase"}`,
		},
		{
			name:     "null orderbook",
			input:    `{"name":"Bitstamp","orderbook":null}`,
			expected: `{"name":"Bitstamp","orderbook":null}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := new(v17.Version).DowngradeExchange(t.Context(), []byte(tc.input))
			require.NoError(t, err, "DowngradeExchange must not error")
			assert.JSONEq(t, tc.expected, string(out), "DowngradeExchange should restore only missing legacy buffer defaults")
		})
	}
}

func TestDowngradeExchangeErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "orderbook without a value", input: `{"name":"Kraken","orderbook":}`},
		{name: "setting without a value", input: `{"name":"Kraken","orderbook":{"websocketBufferLimit":}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := new(v17.Version).DowngradeExchange(t.Context(), []byte(tc.input))
			require.ErrorIs(t, err, jsonparser.UnknownValueTypeError, "DowngradeExchange must return the parser error")
		})
	}
}

func TestRegisteredMigration(t *testing.T) {
	t.Parallel()
	configversion.Migrate(t, new(v17.Version),
		`{"exchanges":[{"name":"Kraken","orderbook":{"verificationBypass":true,"websocketBufferEnabled":true,"websocketBufferLimit":0}},{"name":"Gemini","orderbook":{"websocketBufferEnabled":false,"websocketBufferLimit":5}}]}`,
		`{"exchanges":[{"name":"Kraken","orderbook":{"verificationBypass":true}},{"name":"Gemini","orderbook":{}}]}`,
		`{"exchanges":[{"name":"Kraken","orderbook":{"verificationBypass":true,"websocketBufferLimit":5,"websocketBufferEnabled":false}},{"name":"Gemini","orderbook":{"websocketBufferLimit":5,"websocketBufferEnabled":false}}]}`)
}
