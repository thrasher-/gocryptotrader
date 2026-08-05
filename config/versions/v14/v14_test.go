package v14_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/config/versions"
	v14 "github.com/thrasher-corp/gocryptotrader/config/versions/v14"
)

func TestExchanges(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"Kraken"}, new(v14.Version).Exchanges(), "Exchanges should return only Kraken")
}

func TestUpgradeExchange(t *testing.T) {
	t.Parallel()

	input := []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://ws.kraken.com","WebsocketSpotSupplementaryURL":"wss://ws-auth.kraken.com","RestSpotURL":"https://api.kraken.com"}},"features":{"subscriptions":[{"enabled":true,"channel":"ticker","asset":"spot"},{"enabled":true,"channel":"myOrders","authenticated":true},{"enabled":true,"channel":"myTrades","authenticated":true}]}}`)
	output, err := new(v14.Version).UpgradeExchange(t.Context(), input)
	require.NoError(t, err, "UpgradeExchange must not error for default v1 configuration")
	assert.JSONEq(t, `{"name":"Kraken","api":{"urlEndpoints":{"RestSpotURL":"https://api.kraken.com"}},"features":{"subscriptions":[{"enabled":true,"channel":"ticker","asset":"spot"},{"enabled":true,"channel":"myAccount","authenticated":true}]}}`, string(output), "UpgradeExchange should remove v1 URLs and combine private subscriptions")

	input = []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://ws.kraken.com/","WebsocketSpotSupplementaryURL":"wss://ws-auth.kraken.com/"}}}`)
	output, err = new(v14.Version).UpgradeExchange(t.Context(), input)
	require.NoError(t, err, "UpgradeExchange must accept default v1 URLs with trailing slashes")
	assert.JSONEq(t, `{"name":"Kraken","api":{"urlEndpoints":{}}}`, string(output), "UpgradeExchange should remove both normalized default v1 URLs")

	input = []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://proxy.example/v2","WebsocketSpotSupplementaryURL":"wss://auth-proxy.example/v2"}},"features":{"subscriptions":[{"channel":"myAccount","authenticated":true},{"channel":"openOrders","authenticated":true},{"channel":"ownTrades","authenticated":true}]}}`)
	output, err = new(v14.Version).UpgradeExchange(t.Context(), input)
	require.NoError(t, err, "UpgradeExchange must not error for custom v2 configuration")
	assert.JSONEq(t, `{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://proxy.example/v2","WebsocketSpotSupplementaryURL":"wss://auth-proxy.example/v2"}},"features":{"subscriptions":[{"channel":"myAccount","authenticated":true}]}}`, string(output), "UpgradeExchange should preserve custom URLs and remove duplicate legacy subscriptions")
}

func TestUpgradeExchangeMissingFields(t *testing.T) {
	t.Parallel()

	input := []byte(`{"name":"Kraken","api":{}}`)
	output, err := new(v14.Version).UpgradeExchange(t.Context(), input)
	require.NoError(t, err, "UpgradeExchange must not error when optional fields are missing")
	assert.Equal(t, string(input), string(output), "UpgradeExchange should leave missing optional fields unchanged")
}

func TestUpgradeExchangeInvalidFields(t *testing.T) {
	t.Parallel()

	_, err := new(v14.Version).UpgradeExchange(t.Context(), []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":42}}}`))
	require.ErrorContains(t, err, "Value is not a string", "UpgradeExchange must reject an invalid URL field")

	_, err = new(v14.Version).UpgradeExchange(t.Context(), []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotSupplementaryURL":42}}}`))
	require.ErrorContains(t, err, "Value is not a string", "UpgradeExchange must reject an invalid supplementary URL field")

	_, err = new(v14.Version).UpgradeExchange(t.Context(), []byte(`{"name":"Kraken","features":{"subscriptions":[{"channel":42}]}}`))
	require.ErrorContains(t, err, "Value is not a string", "UpgradeExchange must reject an invalid subscription channel")

	input := []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://proxy.example/kraken-v1"}}}`)
	output, err := new(v14.Version).UpgradeExchange(t.Context(), input)
	require.ErrorIs(t, err, v14.ErrCannotUpgrade, "UpgradeExchange must reject an ambiguous custom WebSocket endpoint")
	assert.Equal(t, string(input), string(output), "UpgradeExchange should leave an incompatible custom endpoint unchanged")

	input = []byte(`{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotSupplementaryURL":"wss://auth-proxy.example/kraken-v1"}}}`)
	output, err = new(v14.Version).UpgradeExchange(t.Context(), input)
	require.ErrorIs(t, err, v14.ErrCannotUpgrade, "UpgradeExchange must reject an ambiguous custom supplementary WebSocket endpoint")
	assert.Equal(t, string(input), string(output), "UpgradeExchange should leave an incompatible custom supplementary endpoint unchanged")
}

func TestDowngradeExchange(t *testing.T) {
	t.Parallel()

	input := []byte(`{"name":"Kraken"}`)
	output, err := new(v14.Version).DowngradeExchange(t.Context(), input)
	require.ErrorIs(t, err, v14.ErrCannotDowngrade, "DowngradeExchange must reject a v1-incompatible Kraken configuration")
	assert.Equal(t, string(input), string(output), "DowngradeExchange should leave the rejected v2 configuration unchanged")
}

func TestRegisteredUpgrade(t *testing.T) {
	t.Parallel()

	input := []byte(`{"version":13,"exchanges":[{"name":"Kraken","api":{"urlEndpoints":{"WebsocketSpotURL":"wss://ws.kraken.com"}},"features":{"subscriptions":[{"channel":"openOrders","authenticated":true}]}}]}`)
	output, err := versions.Manager.Deploy(t.Context(), input, 14)
	require.NoError(t, err, "Deploy must apply the registered v14 upgrade")
	assert.JSONEq(t, `{"version":14,"exchanges":[{"name":"Kraken","api":{"urlEndpoints":{}},"features":{"subscriptions":[{"channel":"myAccount","authenticated":true}]}}]}`, string(output), "Deploy should migrate Kraken and set version 14")
}

func TestRegisteredDowngrade(t *testing.T) {
	t.Parallel()

	input := []byte(`{"version":14,"exchanges":[{"name":"Kraken","features":{"subscriptions":[{"channel":"myAccount","authenticated":true}]}}]}`)
	output, err := versions.Manager.Deploy(t.Context(), input, 13)
	require.ErrorIs(t, err, v14.ErrCannotDowngrade, "Deploy must reject the incompatible Kraken v14 downgrade")
	assert.JSONEq(t, string(input), string(output), "Deploy should retain config version 14 after a rejected downgrade")
}
