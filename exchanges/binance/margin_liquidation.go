package binance

import (
	"context"
	"net/http"
	"net/url"

	"github.com/thrasher-corp/gocryptotrader/currency"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// GetMarginLiquidationLoanResponse contains the documented QueryLiquidationLoanResponse fields.
type GetMarginLiquidationLoanResponse struct {
	Asset           currency.Code `json:"asset"`
	Amount          types.Number  `json:"amount"`
	RepaidAmount    types.Number  `json:"repaidAmount"`
	RemainingAmount types.Number  `json:"remainingAmount"`
}

// GetMarginLiquidationLoanRequest holds the documented parameters for GetMarginLiquidationLoan.
type GetMarginLiquidationLoanRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// GetMarginLiquidationLoan calls GET /sapi/v1/margin/liquidation-loan.
// See https://developers.binance.com/en/docs/catalog/core-trading-margin-trading/api/rest-api/trade#query-liquidation-loan.
func (e *Exchange) GetMarginLiquidationLoan(ctx context.Context, arg *GetMarginLiquidationLoanRequest) (*GetMarginLiquidationLoanResponse, error) {
	if arg == nil {
		arg = new(GetMarginLiquidationLoanRequest)
	}
	params := url.Values{}
	var response *GetMarginLiquidationLoanResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/margin/liquidation-loan", params, sapiGetMarginLiquidationLoanRate, arg, &response)
}
