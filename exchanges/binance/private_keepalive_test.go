package binance

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thrasher-corp/gocryptotrader/exchange/stream"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

type keepaliveConnection struct {
	websocket.Connection
	closed atomic.Int64
}

func (c *keepaliveConnection) Shutdown() error { c.closed.Add(1); return nil }

// Virtual time verifies the production 30-minute interval and cancellation without
// shortening the interval, polling wall time or requiring exchange credentials.
func TestPrivateStreamKeepaliveTiming(t *testing.T) {
	for _, a := range []asset.Item{asset.USDTMarginedFutures, asset.CoinMarginedFutures, asset.Options, asset.Margin} {
		for _, failure := range []bool{false, true} {
			t.Run(a.String()+"/"+map[bool]string{false: "cancel", true: "rejected"}[failure], func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					var calls atomic.Int64
					local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						assert.Equal(t, http.MethodPut, r.Method, "keepalive should renew with PUT")
						body := `{}`
						if failure {
							body = `{"code":-1125,"msg":"This listenKey does not exist."}`
						}
						_, err := w.Write([]byte(body))
						assert.NoError(t, err, "mock response should write")
					})
					local.Websocket.DataHandler = stream.NewRelay(2)
					conn := new(keepaliveConnection)
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if a == asset.Margin {
						local.maintainListenKey(ctx, conn, "portfolio margin", 30*time.Minute, func(ctx context.Context) error {
							_, err := local.KeepPortfolioMarginListenKeyAlive(ctx)
							return err
						})
					} else {
						local.maintainPrivateStream(ctx, conn, a, 30*time.Minute)
					}
					synctest.Wait()
					synctest.Sleep(29 * time.Minute)
					assert.Zero(t, calls.Load(), "keepalive should wait for the full interval")
					synctest.Sleep(time.Minute)
					assert.EqualValues(t, 1, calls.Load(), "keepalive should renew after thirty minutes")
					if failure {
						assert.EqualValues(t, 1, conn.closed.Load(), "renewal rejection should close the connection for recovery")
						event := <-local.Websocket.DataHandler.C
						err, ok := event.Data.(error)
						assert.True(t, ok, "renewal rejection should emit an error")
						assert.ErrorIs(t, err, errAPIResponse, "renewal rejection should retain the API error sentinel")
					} else {
						assert.Zero(t, conn.closed.Load(), "successful renewal should preserve the connection")
					}
					cancel()
					local.Websocket.Wg.Wait()
					synctest.Sleep(time.Hour)
					assert.EqualValues(t, 1, calls.Load(), "cancelled connection should stop renewing its old key")
				})
			})
		}
	}
}
