package binance

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/kline"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

func TestOptionsSubscriptionParams(t *testing.T) {
	t.Parallel()
	pair := currency.NewPair(currency.BTC, currency.NewCode("260921-81000-C"))
	pair.Delimiter = currency.DashDelimiter
	for _, tc := range []struct {
		channel, expected string
		interval          kline.Interval
		params            map[string]any
	}{
		{channel: cnlTrade, expected: "btc-260921-81000-c@optionTrade"},
		{channel: "trade", expected: "btc-260921-81000-c@optionTrade"},
		{channel: cnlTradeWithUnderlyingAsset, expected: "btcusdt@optionTrade"},
		{channel: cnlTicker, expected: "btc-260921-81000-c@optionTicker"},
		{channel: "ticker", expected: "btc-260921-81000-c@optionTicker"},
		{channel: cnlTickerWithExpiration, expected: "btcusdt@optionTicker@260921"},
		{channel: cnlOpenInterest, expected: "btcusdt@openInterest@260921"},
		{channel: cnlMarkPrice, expected: "btcusdt@optionMarkPrice"},
		{channel: cnlIndex, expected: "!index@arr"},
		{channel: cnlOptionSymbol, expected: "!optionSymbol"},
		{channel: cnlKline, expected: "btc-260921-81000-c@kline_1m", interval: kline.OneMin},
		{channel: cnlDepth, expected: "btc-260921-81000-c@depth10@100ms"},
		{channel: cnlDepth, expected: "btc-260921-81000-c@depth50@500ms", interval: kline.FiveHundredMilliseconds, params: map[string]any{"level": 50}},
		{channel: cnlDepth, expected: "btc-260921-81000-c@depth50@500ms", interval: kline.FiveHundredMilliseconds, params: map[string]any{"level": "50"}},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()
			got, err := optionsSubscriptionParams(subscription.List{{Channel: tc.channel, Pairs: currency.Pairs{pair, pair}, Interval: tc.interval, Params: tc.params}})
			require.NoError(t, err, "valid subscriptions must format")
			assert.Equal(t, []string{tc.expected}, got, "subscription should match the live channel name without duplicates")
		})
	}
	for _, tc := range []struct {
		name          string
		subscriptions subscription.List
		expected      error
	}{
		{name: "empty", expected: common.ErrEmptyParams},
		{name: "unsupported", subscriptions: subscription.List{{Channel: "unsupported", Pairs: currency.Pairs{pair}}}, expected: errUnsupportedChannel},
		{name: "missing pairs", subscriptions: subscription.List{{Channel: cnlTrade}}, expected: currency.ErrCurrencyPairsEmpty},
		{name: "empty pair", subscriptions: subscription.List{{Channel: cnlTrade, Pairs: currency.Pairs{currency.EMPTYPAIR}}}, expected: currency.ErrCurrencyPairEmpty},
		{name: "depth limit", subscriptions: subscription.List{{Channel: cnlDepth, Pairs: currency.Pairs{pair}, Params: map[string]any{"level": 5}}}, expected: errLimitNumberRequired},
		{name: "depth interval", subscriptions: subscription.List{{Channel: cnlDepth, Pairs: currency.Pairs{pair}, Interval: kline.OneMin}}, expected: kline.ErrInvalidInterval},
		{name: "candle interval", subscriptions: subscription.List{{Channel: cnlKline, Pairs: currency.Pairs{pair}, Interval: kline.HundredMilliseconds}}, expected: kline.ErrInvalidInterval},
		{name: "expiration", subscriptions: subscription.List{{Channel: cnlOpenInterest, Pairs: currency.Pairs{currency.NewBTCUSDT()}}}, expected: errExpirationTimeRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := optionsSubscriptionParams(tc.subscriptions)
			assert.ErrorIs(t, err, tc.expected, "invalid subscription should return the expected sentinel")
		})
	}
}

func TestOptionsSubscriptionFilterValidation(t *testing.T) {
	t.Parallel()
	local := new(Exchange)
	_, err := local.GenerateEOptionsDefaultSubscriptions("invalid")
	assert.ErrorIs(t, err, errUnsupportedSubscription, "unsupported filter should fail before fetching pairs")
	subscriptions, err := local.GenerateEOptionsDefaultSubscriptions(optionsPrivateFilter)
	require.NoError(t, err, "private stream must not require market subscriptions")
	assert.Empty(t, subscriptions, "private stream should use its authenticated connection")
}

func TestOptionsLiveStreamResponses(t *testing.T) {
	testexch.FixtureToDataHandler(t, "testdata/public_api/options_streams.json", func(_ context.Context, frame []byte) error {
		var captured struct {
			Stream string `json:"stream"`
		}
		require.NoError(t, json.Unmarshal(frame, &captured), "captured envelope must decode")
		t.Run(captured.Stream, func(t *testing.T) {
			t.Parallel()
			local := new(Exchange)
			require.NoError(t, testexch.Setup(local), "test exchange must initialise")
			local.Websocket.DataHandler = stream.NewRelay(10)
			var envelope WsOptionIncomingResponse
			require.NoError(t, json.Unmarshal(frame, &envelope), "live envelope must decode")
			var events WsOptionIncomingResponses
			require.NoError(t, json.Unmarshal(envelope.Data, &events), "live events must decode")
			var model any
			switch events.Instances[0].EventType {
			case "trade":
				model = new(EOptionsWsTrade)
			case "kline":
				model = new(WsOptionsKlineData)
			case "indexPrice":
				model = new([]*OptionsIndexInfo)
			case "markPrice":
				model = new([]*WsOptionsMarkPrice)
			case "24hrTicker":
				model = new(OptionsTicker24Hr)
			case "depthUpdate":
				model = new(WsOptionsOrderbook)
			case "bookTicker":
				model = new(FuturesBookTicker)
			default:
				t.Fatalf("captured event must have a response model: %s", events.Instances[0].EventType)
			}
			require.NoError(t, json.Unmarshal(envelope.Data, model), "model must decode live event")
			assertResponseFields(t, envelope.Data, reflect.TypeOf(model), captured.Stream)
			require.NoError(t, local.wsHandleEOptionsData(t.Context(), nil, frame), "handler must process live envelope")
			if events.Instances[0].EventType == "depthUpdate" {
				return
			}
			select {
			case message := <-local.Websocket.DataHandler.C:
				_, warning := message.Data.(websocket.UnhandledMessageWarning)
				assert.False(t, warning, "supported live event should be handled")
				switch value := message.Data.(type) {
				case trade.Data:
					assert.Equal(t, order.Sell, value.Side, "live SELL trade should retain its side")
					assert.Positive(t, value.Price, "live trade should retain its price")
					assert.Equal(t, asset.Options, value.AssetType, "trade should retain its asset")
				case *ticker.Price:
					assert.Equal(t, asset.Options, value.AssetType, "ticker should retain its asset")
					if events.Instances[0].EventType == "24hrTicker" {
						assert.Positive(t, value.BaseVolume, "ticker should retain volume from lowercase v")
						assert.Positive(t, value.Last, "ticker should retain its last price")
					}
				case kline.Item:
					assert.Equal(t, kline.OneMin, value.Interval, "candle should use k.i")
					assert.Equal(t, time.UnixMilli(1789953360000), value.Candles[0].Time, "candle should use k.t rather than event time")
				}
			default:
				t.Fatal("handler must publish the live event")
			}
		})
		return nil
	})
}

func TestEmptyOptionsStreamResponse(t *testing.T) {
	t.Parallel()
	local := new(Exchange)
	for _, raw := range []string{"", "[]", "null", `{"stream":"btcusdt@optionMarkPrice","data":[]}`} {
		assert.ErrorIs(t, local.wsHandleEOptionsData(t.Context(), nil, []byte(raw)), errEmptyWebsocketResponse, "empty stream should return its sentinel")
	}
}
