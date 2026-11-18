package exchange

import (
	"context"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/asset"
	"github.com/thrasher-corp/gocryptotrader/exchange/deposit"
	"github.com/thrasher-corp/gocryptotrader/exchange/fundingrate"
	"github.com/thrasher-corp/gocryptotrader/exchange/futures"
	"github.com/thrasher-corp/gocryptotrader/exchange/kline"
	"github.com/thrasher-corp/gocryptotrader/exchange/order"
	"github.com/thrasher-corp/gocryptotrader/exchange/protocol"
	"github.com/thrasher-corp/gocryptotrader/exchange/trade"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
)

// unsupportedErr returns the error for a wrapper function an exchange does not implement:
// common.ErrNotYetImplemented when the exchange offers the function, since only the implementation is
// missing, and common.ErrFunctionNotSupported when it does not
func unsupportedErr(offered bool) error {
	if offered {
		return common.ErrNotYetImplemented
	}
	return common.ErrFunctionNotSupported
}

// claims reports whether the exchange's REST or websocket capabilities include a function
func (b *Base) claims(capability func(protocol.Features) bool) bool {
	return capability(b.Features.Supports.RESTCapabilities) || capability(b.Features.Supports.WebsocketCapabilities)
}

// ModifyOrder returns common.ErrFunctionNotSupported, or common.ErrNotYetImplemented when the
// exchange's capabilities claim order modification
func (b *Base) ModifyOrder(context.Context, *order.Modify) (*order.ModifyResponse, error) {
	return nil, unsupportedErr(b.claims(func(f protocol.Features) bool { return f.ModifyOrder }))
}

// CancelBatchOrders returns common.ErrFunctionNotSupported; no capability covers batch cancellation
func (b *Base) CancelBatchOrders(context.Context, []order.Cancel) (*order.CancelBatchResponse, error) {
	return nil, common.ErrFunctionNotSupported
}

// WithdrawFiatFunds returns common.ErrFunctionNotSupported, or common.ErrNotYetImplemented when the
// exchange's capabilities claim fiat withdrawals
func (b *Base) WithdrawFiatFunds(context.Context, *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	return nil, unsupportedErr(b.claims(func(f protocol.Features) bool { return f.FiatWithdraw }))
}

// WithdrawFiatFundsToInternationalBank returns common.ErrFunctionNotSupported; the fiat withdrawal
// capability does not say whether an exchange withdraws to international banks
func (b *Base) WithdrawFiatFundsToInternationalBank(context.Context, *withdraw.Request) (*withdraw.ExchangeResponse, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetAccountFundingHistory returns common.ErrFunctionNotSupported; the deposit and withdrawal history
// capabilities do not imply a combined funding history
func (b *Base) GetAccountFundingHistory(context.Context) ([]FundingHistory, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetWithdrawalsHistory returns common.ErrFunctionNotSupported, or common.ErrNotYetImplemented when the
// exchange's capabilities claim withdrawal history
func (b *Base) GetWithdrawalsHistory(context.Context, currency.Code, asset.Item) ([]WithdrawalHistory, error) {
	return nil, unsupportedErr(b.claims(func(f protocol.Features) bool { return f.WithdrawalHistory }))
}

// GetDepositAddress returns common.ErrFunctionNotSupported, or common.ErrNotYetImplemented when the
// exchange's capabilities claim cryptocurrency deposits
func (b *Base) GetDepositAddress(context.Context, currency.Code, string, string) (*deposit.Address, error) {
	return nil, unsupportedErr(b.claims(func(f protocol.Features) bool { return f.CryptoDeposit }))
}

// UpdateTickers returns common.ErrFunctionNotSupported, or common.ErrNotYetImplemented when the
// exchange's capabilities claim ticker batching
func (b *Base) UpdateTickers(context.Context, asset.Item) error {
	return unsupportedErr(b.claims(func(f protocol.Features) bool { return f.TickerBatching }))
}

// GetHistoricTrades returns common.ErrFunctionNotSupported, or common.ErrNotYetImplemented when the
// exchange's capabilities claim trade history
func (b *Base) GetHistoricTrades(context.Context, currency.Pair, asset.Item, time.Time, time.Time) ([]trade.Data, error) {
	return nil, unsupportedErr(b.claims(func(f protocol.Features) bool { return f.TradeHistory }))
}

// GetHistoricCandles returns common.ErrFunctionNotSupported, or common.ErrNotYetImplemented when the
// exchange's capabilities claim kline fetching
func (b *Base) GetHistoricCandles(context.Context, currency.Pair, asset.Item, kline.Interval, time.Time, time.Time) (*kline.Item, error) {
	return nil, unsupportedErr(b.claims(func(f protocol.Features) bool { return f.KlineFetching }))
}

// GetHistoricCandlesExtended returns common.ErrFunctionNotSupported; kline fetching does not imply
// that an exchange serves candles from before its most recent page
func (b *Base) GetHistoricCandlesExtended(context.Context, currency.Pair, asset.Item, kline.Interval, time.Time, time.Time) (*kline.Item, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetServerTime returns common.ErrFunctionNotSupported; no capability covers the server time
func (b *Base) GetServerTime(context.Context, asset.Item) (time.Time, error) {
	return time.Time{}, common.ErrFunctionNotSupported
}

// UpdateOrderExecutionLimits returns common.ErrNotYetImplemented, which Bootstrap and the backtester
// treat as having no limits to load
func (b *Base) UpdateOrderExecutionLimits(context.Context, asset.Item) error {
	return common.ErrNotYetImplemented
}

// GetLatestFundingRates returns common.ErrFunctionNotSupported, or common.ErrNotYetImplemented when the
// exchange's capabilities claim funding rates
func (b *Base) GetLatestFundingRates(context.Context, *fundingrate.LatestRateRequest) ([]fundingrate.LatestRateResponse, error) {
	offered := b.Features.Supports.FuturesCapabilities.FundingRates || b.claims(func(f protocol.Features) bool { return f.FundingRateFetching })
	return nil, unsupportedErr(offered)
}

// GetFuturesContractDetails returns common.ErrFunctionNotSupported; no capability covers contract
// details
func (b *Base) GetFuturesContractDetails(context.Context, asset.Item) ([]futures.Contract, error) {
	return nil, common.ErrFunctionNotSupported
}
