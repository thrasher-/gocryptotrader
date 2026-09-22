package binance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

func TestOrderListValidationSentinels(t *testing.T) {
	local := new(Exchange)
	assert.ErrorIs(t, validateOPOOrder(nil), common.ErrNilPointer, "nil OPO request should retain its sentinel")
	for _, tc := range []struct {
		name   string
		change func(*OPOOrderRequest)
		want   error
	}{
		{"OPO/Symbol", func(a *OPOOrderRequest) { a.Symbol = currency.EMPTYPAIR }, currency.ErrCurrencyPairEmpty},
		{"OPO/WorkingQuantity", func(a *OPOOrderRequest) { a.WorkingQuantity = 0 }, limits.ErrAmountBelowMin},
		{"OPO/WorkingQuantity", func(a *OPOOrderRequest) { a.WorkingQuantity = -1 }, limits.ErrAmountBelowMin},
		{"OPO/WorkingSide", func(a *OPOOrderRequest) { a.WorkingSide = "" }, order.ErrSideIsInvalid},
		{"OPO/PendingSide", func(a *OPOOrderRequest) { a.PendingSide = "" }, order.ErrSideIsInvalid},
		{"OPO/WorkingType", func(a *OPOOrderRequest) { a.WorkingType = "" }, order.ErrTypeIsInvalid},
		{"OPO/PendingType", func(a *OPOOrderRequest) { a.PendingType = "" }, order.ErrTypeIsInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arg := &OPOOrderRequest{Symbol: currency.NewBTCUSDT(), WorkingQuantity: 1, WorkingSide: "BUY", PendingSide: "SELL", WorkingType: "LIMIT", PendingType: "LIMIT"}
			tc.change(arg)
			assert.ErrorIs(t, validateOPOOrder(arg), tc.want, "helper should return its exact validation sentinel")
			_, err := local.NewOPOOrderList(t.Context(), arg)
			assert.ErrorIs(t, err, tc.want, "REST builder should validate before transport")
			_, err = local.WsNewOPOOrderList(arg)
			assert.ErrorIs(t, err, tc.want, "WebSocket builder should validate before transport")
		})
	}
	assert.ErrorIs(t, validateOPOCOOrder(nil), common.ErrNilPointer, "nil OPOCO request should retain its sentinel")
	for _, tc := range []struct {
		name   string
		change func(*OPOCOOrderRequest)
		want   error
	}{
		{"OPOCO/Symbol", func(a *OPOCOOrderRequest) { a.Symbol = currency.EMPTYPAIR }, currency.ErrCurrencyPairEmpty},
		{"OPOCO/WorkingQuantity", func(a *OPOCOOrderRequest) { a.WorkingQuantity = 0 }, limits.ErrAmountBelowMin},
		{"OPOCO/WorkingQuantity", func(a *OPOCOOrderRequest) { a.WorkingQuantity = -1 }, limits.ErrAmountBelowMin},
		{"OPOCO/WorkingSide", func(a *OPOCOOrderRequest) { a.WorkingSide = "" }, order.ErrSideIsInvalid},
		{"OPOCO/PendingSide", func(a *OPOCOOrderRequest) { a.PendingSide = "" }, order.ErrSideIsInvalid},
		{"OPOCO/WorkingType", func(a *OPOCOOrderRequest) { a.WorkingType = "" }, order.ErrTypeIsInvalid},
		{"OPOCO/PendingAboveType", func(a *OPOCOOrderRequest) { a.PendingAboveType = "" }, order.ErrTypeIsInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arg := &OPOCOOrderRequest{Symbol: currency.NewBTCUSDT(), WorkingQuantity: 1, WorkingSide: "BUY", PendingSide: "SELL", WorkingType: "LIMIT", PendingAboveType: "LIMIT"}
			tc.change(arg)
			assert.ErrorIs(t, validateOPOCOOrder(arg), tc.want, "helper should return its exact validation sentinel")
			_, err := local.NewOPOCOOrderList(t.Context(), arg)
			assert.ErrorIs(t, err, tc.want, "REST builder should validate before transport")
			_, err = local.WsNewOPOCOOrderList(arg)
			assert.ErrorIs(t, err, tc.want, "WebSocket builder should validate before transport")
		})
	}
}

func TestAdditionalRequestValidation(t *testing.T) {
	local := new(Exchange)
	local.SetValues()
	for _, limit := range []uint64{3, 5000} {
		_, err := local.WsUFuturesGetOrderbook(t.Context(), &WsUFuturesGetOrderbookRequest{Symbol: currency.NewBTCUSDT(), Limit: limit})
		assert.ErrorIs(t, err, errLimitNumberRequired, "both invalid depth limit paths should fail offline")
	}
	assert.ErrorIs(t, local.sendFuturesWSRequest(t.Context(), asset.Spot, "depth", nil, nil), asset.ErrNotSupported, "unsupported futures API asset should fail offline")
	start := time.UnixMilli(1750500060000)
	for _, tc := range []struct {
		end  time.Time
		want error
	}{{start.Add(-time.Minute), common.ErrStartAfterEnd}, {start, common.ErrStartEqualsEnd}} {
		_, err := local.GetPortfolioMarginProLoanRepayments(t.Context(), &GetPortfolioMarginProLoanRepaymentsRequest{StartTime: start, EndTime: tc.end})
		assert.ErrorIs(t, err, tc.want, "portfolio history should retain time validation errors")
		_, err = local.GetTravelRuleDepositHistoryV2(t.Context(), &GetTravelRuleDepositHistoryV2Request{StartTime: start, EndTime: tc.end})
		assert.ErrorIs(t, err, tc.want, "deposit history should retain time validation errors")
		_, err = local.GetTravelRuleWithdrawalHistory(t.Context(), &GetTravelRuleWithdrawalHistoryRequest{StartTime: start, EndTime: tc.end})
		assert.ErrorIs(t, err, tc.want, "withdrawal history should retain time validation errors")
	}
}
