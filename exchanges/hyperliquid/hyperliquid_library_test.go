package hyperliquid_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/hyperliquid"
)

func TestGetSpotMetadataLibrary(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/info", r.URL.Path, "GetSpotMetadata should query the info endpoint")
		_, err := w.Write([]byte(`{"universe":[{"name":"@107","tokens":[1,0],"index":107}],"tokens":[{"name":"kPEPE","tokenId":"0xAB","index":1}]}`))
		assert.NoError(t, err, "Write should return token metadata to the library client")
	}))
	t.Cleanup(server.Close)
	client := new(hyperliquid.Exchange)
	client.SetDefaults()
	t.Cleanup(func() {
		assert.NoError(t, client.Shutdown(), "Shutdown should close the library client")
	})
	require.NoError(t, client.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL), "SetRunningURL must select the mock endpoint")
	var (
		response *hyperliquid.SpotMetadataResponse
		token    hyperliquid.SpotTokenMetadata
		market   hyperliquid.SpotAssetMetadata
	)
	response, err := client.GetSpotMetadata(t.Context())
	require.NoError(t, err, "GetSpotMetadata must decode the exported response")
	require.NotNil(t, response, "response must contain spot metadata")
	require.Len(t, response.Tokens, 1, "response.Tokens must contain the requested token")
	token = response.Tokens[0]
	assert.True(t, token.Name.Equal(currency.NewCode("kPEPE")), "token.Name should be usable as a currency code")
	assert.Equal(t, "kPEPE:0xAB", token.TokenIdentifier, "token.TokenIdentifier should retain the exact transfer identifier")
	require.Len(t, response.Universe, 1, "response.Universe must contain the spot market")
	market = response.Universe[0]
	assert.Equal(t, "@107", market.Name, "market.Name should preserve the API market identifier")
}
