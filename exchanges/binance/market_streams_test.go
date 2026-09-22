package binance

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
	testexch "github.com/thrasher-corp/gocryptotrader/internal/testing/exchange"
)

// These are real public frames, including the merged UM/CM universe after migration.
// Diff-depth joining is exercised separately against the mocktester REST snapshots.
func TestRecordedMarketStreams(t *testing.T) {
	for _, tc := range []struct {
		name  string
		asset asset.Item
	}{
		{"spot", asset.Spot}, {"um-public", asset.USDTMarginedFutures}, {"um-market", asset.USDTMarginedFutures}, {"coin", asset.CoinMarginedFutures},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testexch.FixtureToDataHandler(t, "testdata/public_api/"+tc.name+"_streams.json", func(_ context.Context, frame []byte) error {
				var envelope struct {
					Stream string          `json:"stream"`
					Data   json.RawMessage `json:"data"`
				}
				require.NoError(t, json.Unmarshal(frame, &envelope), "recorded envelope must decode")
				t.Run(envelope.Stream, func(t *testing.T) {
					var sample struct {
						Event      string          `json:"e"`
						EventTime  json.RawMessage `json:"E"`
						OpenTime   json.RawMessage `json:"O"`
						SymbolType uint64          `json:"st"`
						Order      json.RawMessage `json:"o"`
					}
					array := len(envelope.Data) > 0 && envelope.Data[0] == '['
					sampleRaw := envelope.Data
					if array {
						var rows []json.RawMessage
						require.NoError(t, json.Unmarshal(envelope.Data, &rows), "array must decode")
						require.NotEmpty(t, rows, "recorded array must contain events")
						sampleRaw = rows[0]
					}
					require.NoError(t, json.Unmarshal(sampleRaw, &sample), "event header must decode")
					var model any
					if tc.asset == asset.Spot {
						switch {
						case isPartialDepthStream(envelope.Stream):
							model = new(OrderBook)
						case sample.Event == "depthUpdate":
							model = new(WebsocketDepthStream)
						case sample.Event == "aggTrade":
							model = new(SpotAggregateTradeStream)
						case sample.Event == "trade":
							model = new(TradeStream)
						case sample.Event == "kline":
							model = new(KlineStream)
						case sample.Event == "avgPrice":
							model = new(AveragePriceStream)
						case strings.Contains(envelope.Stream, "bookTicker"):
							model = new(FuturesBookTicker)
						default:
							model = new(TickerStream)
						}
					} else {
						switch sample.Event {
						case "depthUpdate":
							model = new(FuturesDepthOrderbook)
						case "aggTrade":
							model = new(FuturesAggTrade)
						case "kline":
							model = new(KlineStream)
						case "continuous_kline":
							model = new(FutureContinuousKline)
						case "markPriceUpdate":
							model = new(FuturesMarkPrice)
						case "bookTicker":
							model = new(FuturesBookTicker)
						case "24hrTicker":
							if tc.asset == asset.CoinMarginedFutures {
								model = new(CFuturesMarketTicker)
							} else {
								model = new(UFutureMarketTicker)
							}
						case "24hrMiniTicker":
							model = new(FutureMiniTickerPrice)
						case "assetIndexUpdate":
							model = new(UFuturesAssetIndexUpdate)
						case "indexPrice_kline", "markPrice_kline":
							model = new(CFutureMarkOrIndexPriceKline)
						case "forceOrder":
							model = new(MarketLiquidationOrder)
						default:
							t.Fatalf("recorded event must have a typed model: %s", sample.Event)
						}
					}
					if array {
						model = reflect.New(reflect.SliceOf(reflect.TypeOf(model))).Interface()
					}
					require.NoError(t, json.Unmarshal(envelope.Data, model), "model must decode every live value")
					assertResponseFields(t, envelope.Data, reflect.TypeOf(model), envelope.Stream)
					if sample.Event == "depthUpdate" && !isPartialDepthStream(envelope.Stream) {
						return
					}
					local := new(Exchange)
					require.NoError(t, testexch.Setup(local), "test exchange must initialise")
					local.Websocket.DataHandler = stream.NewRelay(20)
					switch tc.asset {
					case asset.Spot:
						require.NoError(t, local.CurrencyPairs.StorePairs(asset.Spot, currency.Pairs{currency.NewBTCUSDT()}, true), "Spot pair must enable")
						require.NoError(t, local.wsHandleData(t.Context(), nil, frame), "Spot handler must accept its live frame")
					case asset.CoinMarginedFutures:
						require.NoError(t, local.wsHandleCFuturesData(t.Context(), nil, frame), "COIN-M handler must accept its live frame")
					default:
						require.NoError(t, local.wsHandleFuturesData(t.Context(), nil, frame), "USD-M handler must accept its live frame")
					}
					if sample.Event == "depthUpdate" || tc.asset == asset.Spot {
						return
					}
					symbolType := sample.SymbolType
					if sample.Event == "forceOrder" {
						var liquidation FuturesLiquidationOrder
						require.NoError(t, json.Unmarshal(sample.Order, &liquidation), "liquidation header must decode")
						symbolType = liquidation.SymbolType
					}
					expected := tc.asset
					switch symbolType {
					case 1:
						expected = asset.USDTMarginedFutures
					case 2:
						expected = asset.CoinMarginedFutures
					}
					select {
					case event := <-local.Websocket.DataHandler.C:
						switch value := event.Data.(type) {
						case *ticker.Price:
							assert.Equal(t, expected, value.AssetType, "live symbol type should determine the emitted asset")
						case []ticker.Price:
							require.NotEmpty(t, value, "ticker array must emit rows")
							assert.Equal(t, expected, value[0].AssetType, "each row should use its symbol type")
						case *order.Detail:
							assert.Equal(t, expected, value.AssetType, "liquidation should use the order's symbol type")
						}
					default:
						// Trades are delivered through the trade feed rather than DataHandler.
						assert.Equal(t, "aggTrade", sample.Event, "non-trade event should publish a result")
					}
				})
				return nil
			})
		})
	}
}
