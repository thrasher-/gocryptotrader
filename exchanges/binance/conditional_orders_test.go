package binance

import (
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

func TestPortfolioMarginConditionalSubmission(t *testing.T) {
	raw, err := os.ReadFile("testdata/documented_responses.json")
	require.NoError(t, err, "documented responses must load")
	var fixtures map[string]*documentedResponse
	require.NoError(t, json.Unmarshal(raw, &fixtures), "documented responses must decode")
	fixture := fixtures["derivatives_trading_portfolio_margin/new_um_algo_order"]
	require.NotNil(t, fixture, "current algo order fixture must exist")
	for _, tc := range []struct {
		orderType order.Type
		wireType  string
	}{
		{order.Stop, "STOP"},
		{order.StopLimit, "STOP"},
		{order.StopMarket, "STOP_MARKET"},
		{order.TakeProfit, "TAKE_PROFIT"},
		{order.TakeProfitLimit, "TAKE_PROFIT"},
		{order.TakeProfitMarket, "TAKE_PROFIT_MARKET"},
		{order.TrailingStop, "TRAILING_STOP_MARKET"},
	} {
		t.Run(tc.orderType.String(), func(t *testing.T) {
			want := url.Values{"symbol": {"BNBUSDT"}, "algoType": {"CONDITIONAL"}, "side": {"SELL"}, "type": {tc.wireType}, "quantity": {"0.01"}, "clientAlgoId": {"6B2I9XVcJpCjqPAJ4YoFX7"}, "triggerPrice": {"750"}, "reduceOnly": {"true"}}
			submission := &order.Submit{
				Exchange: "Binance", Pair: currency.NewPairWithDelimiter("BNB", "USDT", "-"),
				AssetType: asset.USDTMarginedFutures, Type: tc.orderType, Side: order.Sell,
				Amount: 0.01, TriggerPrice: 750, ReduceOnly: true, ClientOrderID: "6B2I9XVcJpCjqPAJ4YoFX7",
			}
			switch tc.wireType {
			case "STOP", "TAKE_PROFIT":
				submission.Price = 749
				submission.TimeInForce = order.GoodTillCancel
				want.Set("price", "749")
				want.Set("timeInForce", "GTC")
			case "TRAILING_STOP_MARKET":
				submission.TrackingMode = order.Percentage
				submission.TrackingValue = 0.3
				want.Del("triggerPrice")
				want.Set("activatePrice", "750")
				want.Set("callbackRate", "0.3")
			}
			local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, fixture.Method, r.Method, "submission should use the documented method")
				assert.Equal(t, fixture.Path, r.URL.Path, "submission should use the current algo endpoint")
				params := r.URL.Query()
				assert.NotEmpty(t, params.Get("signature"), "submission should be signed")
				params.Del("signature")
				params.Del("timestamp")
				params.Del("recvWindow")
				assert.Equal(t, want, params, "conditional parameters should follow the current contract")
				_, writeErr := w.Write(fixture.Data)
				assert.NoError(t, writeErr, "documented response should write")
			})
			local.accountTypeIsUnified = new(true)
			response, err := local.SubmitOrder(t.Context(), submission)
			require.NoError(t, err, "conditional submission must succeed offline")
			require.NotNil(t, response, "conditional submission must return a result")
			assert.Equal(t, "2146760", response.OrderID, "wrapper should return the algo ID for cancellation")
		})
	}
}

func TestUMConditionalCancellation(t *testing.T) {
	for _, unified := range []bool{false, true} {
		for _, tc := range []struct {
			name, orderID, clientID string
			want                    url.Values
		}{
			{"exchange ID", "9007199254740993", "", url.Values{"algoId": {"9007199254740993"}}},
			{"client ID", "", "client-algo", url.Values{"clientAlgoId": {"client-algo"}}},
			{"both IDs", "9007199254740993", "client-algo", url.Values{"algoId": {"9007199254740993"}, "clientAlgoId": {"client-algo"}}},
		} {
			t.Run(strconv.FormatBool(unified)+"/"+tc.name, func(t *testing.T) {
				local := mockBinanceHTTP(t, func(w http.ResponseWriter, r *http.Request) {
					wantPath := "/fapi/v1/algoOrder"
					response := `{"algoId":9007199254740993,"clientAlgoId":"client-algo","code":"200","msg":"success"}`
					if unified {
						wantPath = "/papi/v1/um/algo/order"
						response = `{"complete":true}`
					}
					assert.Equal(t, http.MethodDelete, r.Method, "cancellation should use DELETE")
					assert.Equal(t, wantPath, r.URL.Path, "cancellation should select the account's algo endpoint")
					params := r.URL.Query()
					params.Del("signature")
					params.Del("timestamp")
					params.Del("recvWindow")
					assert.Equal(t, tc.want, params, "cancellation should preserve exact identifiers")
					_, err := w.Write([]byte(response))
					assert.NoError(t, err, "documented cancellation response should write")
				})
				local.accountTypeIsUnified = new(unified)
				for _, orderType := range []order.Type{order.Stop, order.StopLimit, order.StopMarket, order.TakeProfit, order.TakeProfitLimit, order.TakeProfitMarket, order.TrailingStop} {
					err := local.CancelOrder(t.Context(), &order.Cancel{AssetType: asset.USDTMarginedFutures, Type: orderType, Pair: currency.NewBTCUSDT(), OrderID: tc.orderID, ClientOrderID: tc.clientID})
					assert.NoError(t, err, "every conditional alias should cancel offline")
				}
			})
		}
	}
}

func TestUMConditionalCancellationValidation(t *testing.T) {
	local := mockBinanceHTTP(t, func(http.ResponseWriter, *http.Request) {
		t.Error("invalid algo IDs should fail before sending a request")
	})
	local.accountTypeIsUnified = new(true)
	for _, tc := range []struct {
		id   string
		want error
	}{
		{"", order.ErrOrderIDNotSet},
		{"0", order.ErrOrderIDNotSet},
		{"invalid", strconv.ErrSyntax},
		{"-1", strconv.ErrSyntax},
		{"18446744073709551616", strconv.ErrRange},
	} {
		err := local.CancelOrder(t.Context(), &order.Cancel{AssetType: asset.USDTMarginedFutures, Type: order.StopMarket, Pair: currency.NewBTCUSDT(), OrderID: tc.id})
		assert.ErrorIs(t, err, tc.want, "invalid algo identifier should preserve its sentinel")
	}
}
