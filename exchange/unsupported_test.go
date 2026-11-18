package exchange

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/asset"
	"github.com/thrasher-corp/gocryptotrader/exchange/kline"
	"github.com/thrasher-corp/gocryptotrader/exchange/protocol"
)

func errOnly[T any](_ T, err error) error { return err }

// claimAll claims every boolean capability in a feature set
func claimAll(f *protocol.Features) {
	for _, field := range reflect.ValueOf(f).Elem().Fields() {
		if field.Kind() == reflect.Bool {
			field.SetBool(true)
		}
	}
}

func TestUnsupportedErr(t *testing.T) {
	t.Parallel()
	btcusd, start := currency.NewBTCUSD(), time.Now().Add(-time.Hour)
	type claimCase struct {
		name  string
		offer func(*Base)
		err   error
	}
	for _, tc := range []struct {
		name string
		call func(context.Context, *Base) error
		// claim sets the capability that decides the error; nil when no capability does
		claim func(f *protocol.Features, claimed bool)
		// alsoOffered offers the function through a capability outside the REST and websocket sets
		alsoOffered func(*Base)
		err         error
	}{
		{
			name:  "ModifyOrder",
			call:  func(ctx context.Context, b *Base) error { return errOnly(b.ModifyOrder(ctx, nil)) },
			claim: func(f *protocol.Features, claimed bool) { f.ModifyOrder = claimed },
			err:   common.ErrFunctionNotSupported,
		},
		{
			name: "CancelBatchOrders",
			call: func(ctx context.Context, b *Base) error { return errOnly(b.CancelBatchOrders(ctx, nil)) },
			err:  common.ErrFunctionNotSupported,
		},
		{
			name:  "WithdrawFiatFunds",
			call:  func(ctx context.Context, b *Base) error { return errOnly(b.WithdrawFiatFunds(ctx, nil)) },
			claim: func(f *protocol.Features, claimed bool) { f.FiatWithdraw = claimed },
			err:   common.ErrFunctionNotSupported,
		},
		{
			name: "WithdrawFiatFundsToInternationalBank",
			call: func(ctx context.Context, b *Base) error {
				return errOnly(b.WithdrawFiatFundsToInternationalBank(ctx, nil))
			},
			err: common.ErrFunctionNotSupported,
		},
		{
			name: "GetAccountFundingHistory",
			call: func(ctx context.Context, b *Base) error { return errOnly(b.GetAccountFundingHistory(ctx)) },
			err:  common.ErrFunctionNotSupported,
		},
		{
			name: "GetWithdrawalsHistory",
			call: func(ctx context.Context, b *Base) error {
				return errOnly(b.GetWithdrawalsHistory(ctx, currency.BTC, asset.Spot))
			},
			claim: func(f *protocol.Features, claimed bool) { f.WithdrawalHistory = claimed },
			err:   common.ErrFunctionNotSupported,
		},
		{
			name: "GetDepositAddress",
			call: func(ctx context.Context, b *Base) error {
				return errOnly(b.GetDepositAddress(ctx, currency.BTC, "", ""))
			},
			claim: func(f *protocol.Features, claimed bool) { f.CryptoDeposit = claimed },
			err:   common.ErrFunctionNotSupported,
		},
		{
			name:  "UpdateTickers",
			call:  func(ctx context.Context, b *Base) error { return b.UpdateTickers(ctx, asset.Spot) },
			claim: func(f *protocol.Features, claimed bool) { f.TickerBatching = claimed },
			err:   common.ErrFunctionNotSupported,
		},
		{
			name: "GetHistoricTrades",
			call: func(ctx context.Context, b *Base) error {
				return errOnly(b.GetHistoricTrades(ctx, btcusd, asset.Spot, start, start.Add(time.Minute)))
			},
			claim: func(f *protocol.Features, claimed bool) { f.TradeHistory = claimed },
			err:   common.ErrFunctionNotSupported,
		},
		{
			name: "GetHistoricCandles",
			call: func(ctx context.Context, b *Base) error {
				return errOnly(b.GetHistoricCandles(ctx, btcusd, asset.Spot, kline.OneMin, start, start.Add(time.Minute)))
			},
			claim: func(f *protocol.Features, claimed bool) { f.KlineFetching = claimed },
			err:   common.ErrFunctionNotSupported,
		},
		{
			name: "GetHistoricCandlesExtended",
			call: func(ctx context.Context, b *Base) error {
				return errOnly(b.GetHistoricCandlesExtended(ctx, btcusd, asset.Spot, kline.OneMin, start, start.Add(time.Minute)))
			},
			err: common.ErrFunctionNotSupported,
		},
		{
			name: "GetServerTime",
			call: func(ctx context.Context, b *Base) error { return errOnly(b.GetServerTime(ctx, asset.Spot)) },
			err:  common.ErrFunctionNotSupported,
		},
		{
			name: "UpdateOrderExecutionLimits",
			call: func(ctx context.Context, b *Base) error { return b.UpdateOrderExecutionLimits(ctx, asset.Spot) },
			err:  common.ErrNotYetImplemented,
		},
		{
			name:        "GetLatestFundingRates",
			call:        func(ctx context.Context, b *Base) error { return errOnly(b.GetLatestFundingRates(ctx, nil)) },
			claim:       func(f *protocol.Features, claimed bool) { f.FundingRateFetching = claimed },
			alsoOffered: func(b *Base) { b.Features.Supports.FuturesCapabilities.FundingRates = true },
			err:         common.ErrFunctionNotSupported,
		},
		{
			name: "GetFuturesContractDetails",
			call: func(ctx context.Context, b *Base) error {
				return errOnly(b.GetFuturesContractDetails(ctx, asset.Futures))
			},
			err: common.ErrFunctionNotSupported,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cases := []claimCase{
				{name: "no capability claimed", offer: func(*Base) {}, err: tc.err},
				{
					name: "every other capability claimed",
					offer: func(b *Base) {
						for _, f := range []*protocol.Features{&b.Features.Supports.RESTCapabilities, &b.Features.Supports.WebsocketCapabilities} {
							claimAll(f)
							if tc.claim != nil {
								tc.claim(f, false)
							}
						}
					},
					err: tc.err,
				},
			}
			if tc.claim != nil {
				cases = append(cases,
					claimCase{name: "claimed by REST", offer: func(b *Base) { tc.claim(&b.Features.Supports.RESTCapabilities, true) }, err: common.ErrNotYetImplemented},
					claimCase{name: "claimed by websocket", offer: func(b *Base) { tc.claim(&b.Features.Supports.WebsocketCapabilities, true) }, err: common.ErrNotYetImplemented},
				)
			}
			if tc.alsoOffered != nil {
				cases = append(cases, claimCase{name: "offered by another capability", offer: tc.alsoOffered, err: common.ErrNotYetImplemented})
			}
			t.Run("matches the interface method", func(t *testing.T) {
				t.Parallel()
				want, ok := reflect.TypeFor[IBotExchange]().MethodByName(tc.name)
				require.True(t, ok, "IBotExchange must declare the method")
				got := reflect.ValueOf(new(Base)).MethodByName(tc.name)
				require.True(t, got.IsValid(), "Base must define the method")
				assert.Equal(t, want.Type, got.Type(), "Base's default should have the IBotExchange method's signature")
			})
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					t.Parallel()
					b := new(Base)
					c.offer(b)
					assert.ErrorIs(t, tc.call(t.Context(), b), c.err, "should return the error for what the exchange claims")
				})
			}
		})
	}
}
