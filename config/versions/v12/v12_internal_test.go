package v12

import (
	"testing"

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
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := upgradeSubscriptions([]byte(tc.input))
			require.NoError(t, err, "valid subscriptions must upgrade")
			assert.JSONEq(t, tc.expected, string(actual), "subscriptions should match the expected migration result")
		})
	}

	_, err := upgradeSubscriptions([]byte(`{"features":{"subscriptions":{}}}`))
	assert.Error(t, err, "non-array subscriptions should return an error")
}
