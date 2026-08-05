package v14

import (
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpgradeSubscriptions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "missing subscriptions",
			input:    `{"name":"Kraken"}`,
			expected: `{"name":"Kraken"}`,
		},
		{
			name:     "legacy subscriptions",
			input:    `{"features":{"subscriptions":[{"channel":"ticker"},{"channel":"myOrders","authenticated":true},{"channel":"ownTrades","authenticated":true}]}}`,
			expected: `{"features":{"subscriptions":[{"channel":"ticker"},{"channel":"myAccount","authenticated":true}]}}`,
		},
		{
			name:     "current subscription",
			input:    `{"features":{"subscriptions":[{"channel":"myAccount","authenticated":true},{"channel":"openOrders","authenticated":true}]}}`,
			expected: `{"features":{"subscriptions":[{"channel":"myAccount","authenticated":true}]}}`,
		},
		{
			name:     "conflicting legacy state",
			input:    `{"features":{"subscriptions":[{"channel":"myOrders","enabled":false,"authenticated":true},{"channel":"ownTrades","enabled":true,"authenticated":false}]}}`,
			expected: `{"features":{"subscriptions":[{"channel":"myAccount","enabled":true,"authenticated":true}]}}`,
		},
		{
			name:     "current state merged with legacy",
			input:    `{"features":{"subscriptions":[{"channel":"myAccount","enabled":false,"params":{"snapshot":true}},{"channel":"openOrders","enabled":true,"authenticated":true}]}}`,
			expected: `{"features":{"subscriptions":[{"channel":"myAccount","enabled":true,"authenticated":true,"params":{"snapshot":true}}]}}`,
		},
		{
			name:     "duplicate current subscriptions",
			input:    `{"features":{"subscriptions":[{"channel":"myAccount","enabled":false},{"channel":"myAccount","enabled":true,"authenticated":true},{"channel":"myTrades"}]}}`,
			expected: `{"features":{"subscriptions":[{"channel":"myAccount","enabled":true,"authenticated":true}]}}`,
		},
		{
			name:     "no legacy subscriptions",
			input:    `{"features":{"subscriptions":[{"channel":"ticker"}]}}`,
			expected: `{"features":{"subscriptions":[{"channel":"ticker"}]}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := upgradeSubscriptions([]byte(tc.input))
			require.NoError(t, err, "upgradeSubscriptions must not error for valid subscriptions")
			assert.JSONEq(t, tc.expected, string(actual), "upgradeSubscriptions should return the migrated subscriptions")
		})
	}

	_, err := upgradeSubscriptions([]byte(`{"features":{"subscriptions":{}}}`))
	assert.Error(t, err, "upgradeSubscriptions should reject non-array subscriptions")

	_, err = upgradeSubscriptions([]byte(`{"features":{"subscriptions":[`))
	assert.Error(t, err, "upgradeSubscriptions should reject malformed subscriptions")

	malformed := []byte(`{"features":{"subscriptions":[{"channel":"myOrders"} {"channel":"ticker"}]}}`)
	output, err := upgradeSubscriptions(malformed)
	require.ErrorIs(t, err, jsonparser.MalformedArrayError, "upgradeSubscriptions must reject a missing array separator")
	assert.Equal(t, malformed, output, "upgradeSubscriptions should leave malformed subscriptions unchanged")

	_, err = upgradeSubscriptions([]byte(`{"features":{"subscriptions":[{"channel":42},{"channel":"ticker"}]}}`))
	assert.Error(t, err, "upgradeSubscriptions should retain the first subscription parse error")

	_, err = upgradeSubscriptions([]byte(`{"features":{"subscriptions":[{"channel":"myOrders","enabled":"yes"}]}}`))
	assert.Error(t, err, "upgradeSubscriptions should reject a non-boolean enabled state")

	_, err = upgradeSubscriptions([]byte(`{"features":{"subscriptions":[{"channel":"myAccount","authenticated":"yes"},{"channel":"myTrades"}]}}`))
	assert.Error(t, err, "upgradeSubscriptions should reject a non-boolean authenticated state")
}
