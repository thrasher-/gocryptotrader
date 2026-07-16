package v12_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v12 "github.com/thrasher-corp/gocryptotrader/config/versions/v12"
)

func TestExchanges(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"Kraken"}, new(v12.Version).Exchanges(), "v12 should only upgrade Kraken")
}

func TestUpgradeExchange(t *testing.T) {
	t.Parallel()

	in := []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://ws.kraken.com","WebsocketSpotSupplementaryURL":"wss://ws-auth.kraken.com","RestSpotURL":"https://api.kraken.com"}},"features":{"subscriptions":[{"enabled":true,"channel":"ticker","asset":"spot"},{"enabled":true,"channel":"myOrders","authenticated":true},{"enabled":true,"channel":"myTrades","authenticated":true}]}}`)
	out, err := new(v12.Version).UpgradeExchange(t.Context(), in)
	require.NoError(t, err, "default v1 configuration must upgrade")
	assert.JSONEq(t, `{"name":"Kraken","api":{"urlEndpoints":{"RestSpotURL":"https://api.kraken.com"}},"features":{"subscriptions":[{"enabled":true,"channel":"ticker","asset":"spot"},{"enabled":true,"channel":"myAccount","authenticated":true}]}}`, string(out), "v1 URLs should be removed and private subscriptions should be combined")

	in = []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://proxy.example/v2","WebsocketSpotSupplementaryURL":"wss://auth-proxy.example/v2"}},"features":{"subscriptions":[{"channel":"myAccount","authenticated":true},{"channel":"openOrders","authenticated":true},{"channel":"ownTrades","authenticated":true}]}}`)
	out, err = new(v12.Version).UpgradeExchange(t.Context(), in)
	require.NoError(t, err, "custom v2 configuration must upgrade")
	assert.JSONEq(t, `{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://proxy.example/v2","WebsocketSpotSupplementaryURL":"wss://auth-proxy.example/v2"}},"features":{"subscriptions":[{"channel":"myAccount","authenticated":true}]}}`, string(out), "custom URLs should remain and duplicate legacy private subscriptions should be removed")
}

func TestUpgradeExchangeMissingFields(t *testing.T) {
	t.Parallel()

	in := []byte(`{"name":"Kraken","api":{}}`)
	out, err := new(v12.Version).UpgradeExchange(t.Context(), in)
	require.NoError(t, err, "missing optional fields must not prevent upgrade")
	assert.Equal(t, string(in), string(out), "missing optional fields should leave the exchange unchanged")
}

func TestUpgradeExchangeInvalidFields(t *testing.T) {
	t.Parallel()

	_, err := new(v12.Version).UpgradeExchange(t.Context(), []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":42}}}`))
	require.ErrorContains(t, err, "Value is not a string", "invalid URL field must error")

	_, err = new(v12.Version).UpgradeExchange(t.Context(), []byte(`{"name":"Kraken","features":{"subscriptions":[{"channel":42}]}}`))
	require.ErrorContains(t, err, "Value is not a string", "invalid subscription channel must error")
}

func TestDowngradeExchange(t *testing.T) {
	t.Parallel()
	in := []byte(`{"name":"Kraken"}`)
	out, err := new(v12.Version).DowngradeExchange(t.Context(), in)
	require.NoError(t, err, "downgrade must not error")
	assert.Equal(t, string(in), string(out), "downgrade should leave v2 configuration unchanged")
}
