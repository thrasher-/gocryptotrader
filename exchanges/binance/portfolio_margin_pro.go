package binance

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// DeletePortfolioMarginCallLevelResponse contains the documented DeleteMarginCallLevelResponse fields.
type DeletePortfolioMarginCallLevelResponse struct {
	Message string `json:"msg"`
}

// DeletePortfolioMarginCallLevelRequest holds the documented parameters for DeletePortfolioMarginCallLevel.
type DeletePortfolioMarginCallLevelRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// GetPortfolioDeltaModeResponse contains the documented GetDeltaModeStatusResponse fields.
type GetPortfolioDeltaModeResponse struct {
	DeltaEnabled bool `json:"deltaEnabled"`
}

// GetPortfolioDeltaModeRequest holds the documented parameters for GetPortfolioDeltaMode.
type GetPortfolioDeltaModeRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// GetPortfolioMarginCallLevelResponse contains the documented GetMarginCallLevelResponse fields.
type GetPortfolioMarginCallLevelResponse struct {
	MarginCallLevel types.Number `json:"marginCallLevel"`
}

// GetPortfolioMarginCallLevelRequest holds the documented parameters for GetPortfolioMarginCallLevel.
type GetPortfolioMarginCallLevelRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// GetPortfolioMarginProBalancesResponse contains the documented GetPortfolioMarginProAccountBalanceResponseInner fields.
type GetPortfolioMarginProBalancesResponse struct {
	Asset               currency.Code `json:"asset"`
	TotalWalletBalance  types.Number  `json:"totalWalletBalance"`
	CrossMarginAsset    types.Number  `json:"crossMarginAsset"`
	CrossMarginBorrowed types.Number  `json:"crossMarginBorrowed"`
	CrossMarginFree     types.Number  `json:"crossMarginFree"`
	CrossMarginInterest types.Number  `json:"crossMarginInterest"`
	CrossMarginLocked   types.Number  `json:"crossMarginLocked"`
	UMWalletBalance     types.Number  `json:"umWalletBalance"`
	UMUnrealisedPNL     types.Number  `json:"umUnrealizedPNL"`
	CMWalletBalance     types.Number  `json:"cmWalletBalance"`
	CMUnrealisedPNL     types.Number  `json:"cmUnrealizedPNL"`
	UpdateTime          types.Time    `json:"updateTime"`
	NegativeBalance     types.Number  `json:"negativeBalance"`
	OptionWalletBalance types.Number  `json:"optionWalletBalance"`
	OptionEquity        types.Number  `json:"optionEquity"`
}

// GetPortfolioMarginProBalancesRequest holds the documented parameters for GetPortfolioMarginProBalances.
type GetPortfolioMarginProBalancesRequest struct {
	Asset      currency.Code `json:"asset,omitzero"`
	RecvWindow uint64        `json:"recvWindow,omitempty"`
}

// GetPortfolioMarginProSPANAccountRiskUnitMMListResponse contains the documented GetPortfolioMarginProSpanAccountInfoResponseRiskUnitMMListInner fields.
type GetPortfolioMarginProSPANAccountRiskUnitMMListResponse struct {
	Asset          currency.Code `json:"asset"`
	UniMaintainUSD types.Number  `json:"uniMaintainUsd"`
}

// GetPortfolioMarginProSPANAccountResponse contains the documented GetPortfolioMarginProSpanAccountInfoResponse fields.
type GetPortfolioMarginProSPANAccountResponse struct {
	UniMMR             types.Number                                              `json:"uniMMR"`
	AccountEquity      types.Number                                              `json:"accountEquity"`
	ActualEquity       types.Number                                              `json:"actualEquity"`
	AccountMaintMargin types.Number                                              `json:"accountMaintMargin"`
	RiskUnitMMList     []*GetPortfolioMarginProSPANAccountRiskUnitMMListResponse `json:"riskUnitMMList"`
	MarginMM           types.Number                                              `json:"marginMM"`
	OtherMM            types.Number                                              `json:"otherMM"`
	AccountStatus      string                                                    `json:"accountStatus"`
	AccountType        string                                                    `json:"accountType"`
}

// GetPortfolioMarginProSPANAccountRequest holds the documented parameters for GetPortfolioMarginProSPANAccount.
type GetPortfolioMarginProSPANAccountRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// GetPortfolioTransferableEarnBalanceResponse contains the documented GetTransferableEarnAssetBalanceForPortfolioMarginResponse fields.
type GetPortfolioTransferableEarnBalanceResponse struct {
	Asset  currency.Code `json:"asset"`
	Amount types.Number  `json:"amount"`
}

// GetPortfolioTransferableEarnBalanceRequest holds the documented parameters for GetPortfolioTransferableEarnBalance.
type GetPortfolioTransferableEarnBalanceRequest struct {
	Asset        currency.Code `json:"asset"`
	TransferType string        `json:"transferType"`
	RecvWindow   uint64        `json:"recvWindow,omitempty"`
}

// GetPortfolioMarginProLoanRepaymentsRowsResponse contains the documented QueryPortfolioMarginProBankruptcyLoanRepayHistoryResponseRowsInner fields.
type GetPortfolioMarginProLoanRepaymentsRowsResponse struct {
	Asset     currency.Code `json:"asset"`
	Amount    types.Number  `json:"amount"`
	RepayTime types.Time    `json:"repayTime"`
}

// GetPortfolioMarginProLoanRepaymentsResponse contains the documented QueryPortfolioMarginProBankruptcyLoanRepayHistoryResponse fields.
type GetPortfolioMarginProLoanRepaymentsResponse struct {
	Total uint64                                             `json:"total"`
	Rows  []*GetPortfolioMarginProLoanRepaymentsRowsResponse `json:"rows"`
}

// GetPortfolioMarginProLoanRepaymentsRequest holds the documented parameters for GetPortfolioMarginProLoanRepayments.
type GetPortfolioMarginProLoanRepaymentsRequest struct {
	StartTime  time.Time `json:"-"`
	EndTime    time.Time `json:"-"`
	Size       uint64    `json:"size,omitempty"`
	Current    uint64    `json:"current,omitempty"`
	RecvWindow uint64    `json:"recvWindow,omitempty"`
}

// SetPortfolioMarginCallLevelResponse contains the documented SetMarginCallLevelResponse fields.
type SetPortfolioMarginCallLevelResponse struct {
	MarginCallLevel types.Number `json:"marginCallLevel"`
}

// SetPortfolioMarginCallLevelRequest holds the documented parameters for SetPortfolioMarginCallLevel.
type SetPortfolioMarginCallLevelRequest struct {
	MarginCallLevel float64 `json:"marginCallLevel"`
	RecvWindow      uint64  `json:"recvWindow,omitempty"`
}

// SetPortfolioDeltaModeResponse contains the documented SwitchDeltaModeResponse fields.
type SetPortfolioDeltaModeResponse struct {
	Message string `json:"msg"`
}

// SetPortfolioDeltaModeRequest holds the documented parameters for SetPortfolioDeltaMode.
type SetPortfolioDeltaModeRequest struct {
	DeltaEnabled bool   `json:"deltaEnabled"`
	RecvWindow   uint64 `json:"recvWindow,omitempty"`
}

// TransferPortfolioEarnAssetsResponse contains the documented TransferLdusdtRwusdForPortfolioMarginResponse fields.
type TransferPortfolioEarnAssetsResponse struct {
	Message string `json:"msg"`
}

// TransferPortfolioEarnAssetsRequest holds the documented parameters for TransferPortfolioEarnAssets.
type TransferPortfolioEarnAssetsRequest struct {
	Asset        currency.Code `json:"asset"`
	TransferType string        `json:"transferType"`
	Amount       float64       `json:"amount"`
	RecvWindow   uint64        `json:"recvWindow,omitempty"`
}

// GetPortfolioTieredCollateralRatesCollateralInfoResponse contains the documented PortfolioMarginProTieredCollateralRateResponseInnerCollateralInfoInner fields.
type GetPortfolioTieredCollateralRatesCollateralInfoResponse struct {
	TierFloor        types.Number `json:"tierFloor"`
	TierCap          types.Number `json:"tierCap"`
	CollateralRate   types.Number `json:"collateralRate"`
	CumulativeEquity types.Number `json:"cum"`
}

// GetPortfolioTieredCollateralRatesResponse contains the documented PortfolioMarginProTieredCollateralRateResponseInner fields.
type GetPortfolioTieredCollateralRatesResponse struct {
	Asset          currency.Code                                              `json:"asset"`
	CollateralInfo []*GetPortfolioTieredCollateralRatesCollateralInfoResponse `json:"collateralInfo"`
}

// GetPortfolioTieredCollateralRatesRequest holds the documented parameters for GetPortfolioTieredCollateralRates.
type GetPortfolioTieredCollateralRatesRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// DeletePortfolioMarginCallLevel calls DELETE /sapi/v1/portfolio/margin-call-level.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#delete-margin-call-level.
func (e *Exchange) DeletePortfolioMarginCallLevel(ctx context.Context, arg *DeletePortfolioMarginCallLevelRequest) (*DeletePortfolioMarginCallLevelResponse, error) {
	if arg == nil {
		arg = new(DeletePortfolioMarginCallLevelRequest)
	}
	params := url.Values{}
	var response *DeletePortfolioMarginCallLevelResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodDelete, "/sapi/v1/portfolio/margin-call-level", params, sapiDeletePortfolioMarginCallLevelRate, arg, &response)
}

// GetPortfolioDeltaMode calls GET /sapi/v1/portfolio/delta-mode.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#get-delta-mode-status.
func (e *Exchange) GetPortfolioDeltaMode(ctx context.Context, arg *GetPortfolioDeltaModeRequest) (*GetPortfolioDeltaModeResponse, error) {
	if arg == nil {
		arg = new(GetPortfolioDeltaModeRequest)
	}
	params := url.Values{}
	var response *GetPortfolioDeltaModeResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/portfolio/delta-mode", params, sapiGetPortfolioDeltaModeRate, arg, &response)
}

// GetPortfolioMarginCallLevel calls GET /sapi/v1/portfolio/margin-call-level.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#get-margin-call-level.
func (e *Exchange) GetPortfolioMarginCallLevel(ctx context.Context, arg *GetPortfolioMarginCallLevelRequest) (*GetPortfolioMarginCallLevelResponse, error) {
	if arg == nil {
		arg = new(GetPortfolioMarginCallLevelRequest)
	}
	params := url.Values{}
	var response *GetPortfolioMarginCallLevelResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/portfolio/margin-call-level", params, sapiGetPortfolioMarginCallLevelRate, arg, &response)
}

// GetPortfolioMarginProBalances calls GET /sapi/v1/portfolio/balance.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#get-portfolio-margin-pro-account-balance.
func (e *Exchange) GetPortfolioMarginProBalances(ctx context.Context, arg *GetPortfolioMarginProBalancesRequest) ([]*GetPortfolioMarginProBalancesResponse, error) {
	if arg == nil {
		arg = new(GetPortfolioMarginProBalancesRequest)
	}
	params := url.Values{}
	var response []*GetPortfolioMarginProBalancesResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/portfolio/balance", params, sapiGetPortfolioMarginProBalancesRate, arg, &response)
}

// GetPortfolioMarginProSPANAccount calls GET /sapi/v2/portfolio/account.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#get-portfolio-margin-pro-span-account-info.
func (e *Exchange) GetPortfolioMarginProSPANAccount(ctx context.Context, arg *GetPortfolioMarginProSPANAccountRequest) (*GetPortfolioMarginProSPANAccountResponse, error) {
	if arg == nil {
		arg = new(GetPortfolioMarginProSPANAccountRequest)
	}
	params := url.Values{}
	var response *GetPortfolioMarginProSPANAccountResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v2/portfolio/account", params, sapiGetPortfolioMarginProSPANAccountRate, arg, &response)
}

// GetPortfolioTransferableEarnBalance calls GET /sapi/v1/portfolio/earn-asset-balance.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#get-transferable-earn-asset-balance-for-portfolio-margin.
func (e *Exchange) GetPortfolioTransferableEarnBalance(ctx context.Context, arg *GetPortfolioTransferableEarnBalanceRequest) (*GetPortfolioTransferableEarnBalanceResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Asset.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if arg.TransferType == "" {
		return nil, errTransferTypeRequired
	}
	params := url.Values{}
	var response *GetPortfolioTransferableEarnBalanceResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/portfolio/earn-asset-balance", params, sapiGetPortfolioTransferableEarnBalanceRate, arg, &response)
}

// GetPortfolioMarginProLoanRepayments calls GET /sapi/v1/portfolio/pmloan-history.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#query-portfolio-margin-pro-bankruptcy-loan-repay-history.
func (e *Exchange) GetPortfolioMarginProLoanRepayments(ctx context.Context, arg *GetPortfolioMarginProLoanRepaymentsRequest) (*GetPortfolioMarginProLoanRepaymentsResponse, error) {
	if arg == nil {
		arg = new(GetPortfolioMarginProLoanRepaymentsRequest)
	}
	if !arg.StartTime.IsZero() && !arg.EndTime.IsZero() {
		if err := common.StartEndTimeCheck(arg.StartTime, arg.EndTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !arg.StartTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(arg.StartTime.UTC().UnixMilli(), 10))
	}
	if !arg.EndTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(arg.EndTime.UTC().UnixMilli(), 10))
	}
	var response *GetPortfolioMarginProLoanRepaymentsResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/portfolio/pmloan-history", params, sapiGetPortfolioMarginProLoanRepaymentsRate, arg, &response)
}

// SetPortfolioMarginCallLevel calls POST /sapi/v1/portfolio/margin-call-level.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#set-margin-call-level.
func (e *Exchange) SetPortfolioMarginCallLevel(ctx context.Context, arg *SetPortfolioMarginCallLevelRequest) (*SetPortfolioMarginCallLevelResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.MarginCallLevel < 1.1 || arg.MarginCallLevel > 2 {
		return nil, errMarginCallValueRequired
	}
	params := url.Values{}
	var response *SetPortfolioMarginCallLevelResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/sapi/v1/portfolio/margin-call-level", params, sapiSetPortfolioMarginCallLevelRate, arg, &response)
}

// SetPortfolioDeltaMode calls POST /sapi/v1/portfolio/delta-mode.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#switch-delta-mode.
func (e *Exchange) SetPortfolioDeltaMode(ctx context.Context, arg *SetPortfolioDeltaModeRequest) (*SetPortfolioDeltaModeResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	params := url.Values{}
	var response *SetPortfolioDeltaModeResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/sapi/v1/portfolio/delta-mode", params, sapiSetPortfolioDeltaModeRate, arg, &response)
}

// TransferPortfolioEarnAssets calls POST /sapi/v1/portfolio/earn-asset-transfer.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/account#transfer-ldusdt-rwusd-for-portfolio-margin.
func (e *Exchange) TransferPortfolioEarnAssets(ctx context.Context, arg *TransferPortfolioEarnAssetsRequest) (*TransferPortfolioEarnAssetsResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Asset.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if arg.TransferType == "" {
		return nil, errTransferTypeRequired
	}
	if arg.Amount <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	params := url.Values{}
	var response *TransferPortfolioEarnAssetsResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/sapi/v1/portfolio/earn-asset-transfer", params, sapiTransferPortfolioEarnAssetsRate, arg, &response)
}

// GetPortfolioTieredCollateralRates calls GET /sapi/v2/portfolio/collateralRate.
// See https://developers.binance.com/en/docs/catalog/advanced-trading-derivatives-trading-portfolio-margin-pro/api/rest-api/market-data#portfolio-margin-pro-tiered-collateral-rate.
func (e *Exchange) GetPortfolioTieredCollateralRates(ctx context.Context, arg *GetPortfolioTieredCollateralRatesRequest) ([]*GetPortfolioTieredCollateralRatesResponse, error) {
	if arg == nil {
		arg = new(GetPortfolioTieredCollateralRatesRequest)
	}
	params := url.Values{}
	var response []*GetPortfolioTieredCollateralRatesResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v2/portfolio/collateralRate", params, sapiGetPortfolioTieredCollateralRatesRate, arg, &response)
}
