package binance

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func TestAdditionalMarketStreams(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	local.Websocket.DataHandler = stream.NewRelay(16)
	pairs := currency.Pairs{currency.NewBTCUSDT(), currency.NewPair(currency.BNB, currency.BTC), currency.NewPair(currency.NewCode("BAZ"), currency.USD)}
	require.NoError(t, local.CurrencyPairs.StorePairs(asset.Spot, pairs, false), "documented symbols must be available")
	require.NoError(t, local.CurrencyPairs.StorePairs(asset.Spot, pairs, true), "documented symbols must be enabled")
	handle := func(t *testing.T, raw []byte) {
		t.Helper()
		var envelope struct {
			Stream string          `json:"stream"`
			Data   json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(raw, &envelope), "stream envelope must decode")
		var header struct {
			Event     string          `json:"e"`
			EventTime json.RawMessage `json:"E"`
		}
		require.NoError(t, json.Unmarshal(envelope.Data, &header), "event type must decode")
		var model any
		switch header.Event {
		case "referencePrice":
			model = new(SpotReferencePriceStream)
		case "blockTrade":
			model = new(SpotBlockTradeStream)
		case "depthUpdate":
			model = new(FuturesRPIDepth)
		default:
			t.Fatalf("fixture must have a supported event: %s", header.Event)
		}
		require.NoError(t, json.Unmarshal(envelope.Data, model), "all event fields must decode")
		assertResponseFields(t, envelope.Data, reflect.TypeOf(model), envelope.Stream)
		if header.Event == "depthUpdate" {
			require.NoError(t, local.wsHandleFuturesData(t.Context(), nil, raw), "RPI stream must route through the futures handler")
			assert.Empty(t, local.derivativeBooks, "RPI updates should preserve the ordinary book universe")
		} else {
			require.NoError(t, local.wsHandleData(t.Context(), nil, raw), "Spot stream must route through the registered handler")
		}
		select {
		case value := <-local.Websocket.DataHandler.C:
			assert.Equal(t, reflect.ValueOf(model).Elem().Interface(), value.Data, "handler should preserve every event field")
		default:
			t.Fatal("handler must emit its typed event")
		}
	}
	data, err := os.ReadFile("testdata/documented_market_streams.json")
	require.NoError(t, err, "documented market fixtures must load")
	var fixtures []struct {
		Model string          `json:"model"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(data, &fixtures), "documented fixtures must decode")
	for _, fixture := range fixtures {
		t.Run(fixture.Model, func(t *testing.T) { handle(t, fixture.Data) })
	}
	t.Run("unavailable reference price", func(t *testing.T) {
		handle(t, []byte(`{"stream":"btcusdt@referencePrice","data":{"e":"referencePrice","s":"BTCUSDT","r":null,"t":1770313263917}}`))
	})
	for _, name := range []string{"spot-extra", "um-rpi"} {
		t.Run(name, func(t *testing.T) {
			testexch.FixtureToDataHandler(t, "testdata/public_api/"+name+"_streams.json", func(_ context.Context, frame []byte) error { handle(t, frame); return nil })
		})
	}
}

func TestAdditionalStreamSubscriptions(t *testing.T) {
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	require.NoError(t, local.CurrencyPairs.StorePairs(asset.Spot, currency.Pairs{currency.NewBTCUSDT()}, true), "Spot pair must enable")
	for _, channel := range []string{"blockTrade", "referencePrice"} {
		local.Features.Subscriptions = subscription.List{{Channel: channel, Asset: asset.Spot, Enabled: true}}
		subs, err := local.generateSubscriptions()
		require.NoError(t, err, "native Spot channel must expand")
		require.Len(t, subs, 1, "one enabled pair must yield one stream")
		assert.Equal(t, "btcusdt@"+channel, subs[0].QualifiedChannel, "native channel should retain its documented case")
	}
	for _, tc := range []struct {
		asset    asset.Item
		levels   int
		interval kline.Interval
		want     error
	}{
		{asset.USDTMarginedFutures, 0, 0, nil},
		{asset.USDTMarginedFutures, 0, kline.FiveHundredMilliseconds, nil},
		{asset.USDTMarginedFutures, 5, 0, subscription.ErrInvalidLevel},
		{asset.USDTMarginedFutures, 0, kline.HundredMilliseconds, subscription.ErrInvalidInterval},
		{asset.CoinMarginedFutures, 0, 0, subscription.ErrNotSupported},
	} {
		local.Features.Subscriptions = subscription.List{{Channel: "rpiDepth", Asset: tc.asset, Enabled: true, Pairs: currency.Pairs{currency.NewBTCUSDT()}, Levels: tc.levels, Interval: tc.interval}}
		subs, _, err := local.configuredDerivativeSubscriptions(tc.asset, usdtmPublicFilter)
		if tc.want != nil {
			assert.ErrorIs(t, err, tc.want, "invalid RPI subscription should retain its sentinel")
			continue
		}
		require.NoError(t, err, "RPI subscription must expand")
		require.Len(t, subs, 1, "RPI must produce one configured stream")
		assert.Equal(t, "btcusdt@rpiDepth@500ms", subs[0].QualifiedChannel, "RPI should use its documented update interval")
	}
}

func TestPortfolioMarginStreamFields(t *testing.T) {
	data, err := os.ReadFile("testdata/documented_portfolio_streams.json")
	require.NoError(t, err, "portfolio fixtures must load")
	var fixtures []struct {
		Model string          `json:"model"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(data, &fixtures), "portfolio fixtures must decode")
	local := new(Exchange)
	require.NoError(t, testexch.Setup(local), "test exchange must initialise")
	local.Websocket.DataHandler = stream.NewRelay(16)
	for _, fixture := range fixtures {
		t.Run(fixture.Model, func(t *testing.T) {
			require.NoError(t, local.WsHandlePortfolioMarginData(t.Context(), nil, fixture.Data), "documented portfolio event must route")
			select {
			case value := <-local.Websocket.DataHandler.C:
				assertResponseFields(t, fixture.Data, reflect.TypeOf(value.Data), fixture.Model)
			default:
				t.Fatal("portfolio handler must emit a typed event")
			}
		})
	}
	assert.ErrorIs(t, local.WsHandlePortfolioMarginData(t.Context(), nil, []byte(`{"e":"unknown"}`)), errUnsupportedChannel, "unknown portfolio event should retain its sentinel")
	for _, body := range []string{`{}`, `{"code":-1125,"msg":"invalid listen key"}`} {
		mock := mockBinanceHTTP(t, func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(body))
			assert.NoError(t, err, "mock response should write")
		})
		want := common.ErrNoResponse
		if body != `{}` {
			want = errAPIResponse
		}
		assert.ErrorIs(t, mock.WsPortfolioMarginConnect(t.Context(), nil), want, "failed listen-key acquisition should retain its sentinel before dialling")
	}
}
