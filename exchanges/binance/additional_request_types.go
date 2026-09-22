package binance

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
)

// FuturesForceOrdersRequest supplies optional parameters accepted by FuturesForceOrders.
type FuturesForceOrdersRequest struct {
	Limit uint64 `json:"limit,omitempty"`
}

// PMCancelMarginAccountOrderRequest supplies optional parameters accepted by PMCancelMarginAccountOrder.
type PMCancelMarginAccountOrderRequest struct {
	NewClientOrderID string `json:"newClientOrderId,omitempty"`
}

// CancelUMAlgoOrderRequest supplies optional parameters accepted by CancelUMAlgoOrder.
type CancelUMAlgoOrderRequest struct {
	ClientAlgoID string `json:"clientAlgoId,omitempty"`
}

// GetAllUMOpenAlgoOrdersRequest supplies optional filters for GetAllUMOpenAlgoOrders.
type GetAllUMOpenAlgoOrdersRequest struct {
	AlgoID   uint64 `json:"algoId,omitempty"`
	AlgoType string `json:"algoType,omitempty"`
}

// GetUMOpenAlgoOrderRequest identifies a single algo order.
type GetUMOpenAlgoOrderRequest struct {
	AlgoID       uint64 `json:"algoId,omitempty"`
	ClientAlgoID string `json:"clientAlgoId,omitempty"`
}

// GetUMAlgoOrderHistoryRequest supplies optional filters for GetUMAlgoOrderHistory.
type GetUMAlgoOrderHistoryRequest struct {
	AlgoID    uint64    `json:"algoId,omitempty"`
	EndTime   time.Time `json:"endTime,omitzero"`
	Limit     uint64    `json:"limit,omitempty"`
	StartTime time.Time `json:"startTime,omitzero"`
}

// RepayFuturesNegativeBalanceClassicRequest supplies optional parameters accepted by RepayFuturesNegativeBalanceClassic.
type RepayFuturesNegativeBalanceClassicRequest struct {
	From string `json:"from,omitempty"`
}

// UCompositeIndexesInfoRequest supplies optional parameters accepted by UCompositeIndexesInfo.
type UCompositeIndexesInfoRequest struct {
	Symbol currency.Pair `json:"symbol,omitzero"`
}

// UFuturesOpenAlgoOrdersRequest supplies optional parameters accepted by UFuturesOpenAlgoOrders.
type UFuturesOpenAlgoOrdersRequest struct {
	AlgoID   uint64 `json:"algoId,omitempty"`
	AlgoType string `json:"algoType,omitempty"`
}

// UFuturesAllAlgoOrdersRequest supplies optional parameters accepted by UFuturesAllAlgoOrders.
type UFuturesAllAlgoOrdersRequest struct {
	AlgoID uint64 `json:"algoId,omitempty"`
}

// GetExchangeInfoRequest supplies optional parameters accepted by GetExchangeInfo.
type GetExchangeInfoRequest struct {
	Permissions        []string       `json:"permissions,omitempty"`
	ShowPermissionSets *bool          `json:"showPermissionSets,omitempty"`
	Symbol             currency.Pair  `json:"symbol,omitzero"`
	SymbolStatus       string         `json:"symbolStatus,omitempty"`
	Symbols            currency.Pairs `json:"symbols,omitempty"`
}

// GetTickerDataRequest supplies optional parameters accepted by GetTickerData.
type GetTickerDataRequest struct {
	SymbolStatus string `json:"symbolStatus,omitempty"`
}

// GetPriceChangeStatsRequest supplies optional parameters accepted by GetPriceChangeStats.
type GetPriceChangeStatsRequest struct {
	SymbolStatus string `json:"symbolStatus,omitempty"`
	Type         string `json:"type,omitempty"`
}

// GetBestPriceRequest supplies optional parameters accepted by GetBestPrice.
type GetBestPriceRequest struct {
	SymbolStatus string `json:"symbolStatus,omitempty"`
}

// GetLatestSpotPriceRequest supplies optional parameters accepted by GetLatestSpotPrice.
type GetLatestSpotPriceRequest struct {
	SymbolStatus string `json:"symbolStatus,omitempty"`
}

// GetTradingDayTickerRequest supplies optional parameters accepted by GetTradingDayTicker.
type GetTradingDayTickerRequest struct {
	SymbolStatus string `json:"symbolStatus,omitempty"`
}

// CancelExistingOrderRequest supplies optional parameters accepted by CancelExistingOrder.
type CancelExistingOrderRequest struct {
	CancelRestrictions string `json:"cancelRestrictions,omitempty"`
	NewClientOrderID   string `json:"newClientOrderId,omitempty"`
}

// GetETHRedemptionHistoryRequest supplies optional parameters accepted by GetETHRedemptionHistory.
type GetETHRedemptionHistoryRequest struct {
	RedeemID uint64 `json:"redeemId,omitempty"`
}

// GetETHStakingHistoryRequest supplies optional parameters accepted by GetETHStakingHistory.
type GetETHStakingHistoryRequest struct {
	PurchaseID uint64 `json:"purchaseId,omitempty"`
}

// GetSOLRedemptionHistoryRequest supplies optional parameters accepted by GetSOLRedemptionHistory.
type GetSOLRedemptionHistoryRequest struct {
	RedeemID uint64 `json:"redeemId,omitempty"`
}

// GetSOLStakingHistoryRequest supplies optional parameters accepted by GetSOLStakingHistory.
type GetSOLStakingHistoryRequest struct {
	PurchaseID uint64 `json:"purchaseId,omitempty"`
}

// GetManagedSubAccountDepositAddressRequest supplies optional parameters accepted by GetManagedSubAccountDepositAddress.
type GetManagedSubAccountDepositAddressRequest struct {
	Amount float64 `json:"amount,omitempty"`
}

// GetManagedSubAccountFutureesAssetDetailsRequest supplies optional parameters accepted by GetManagedSubAccountFutureesAssetDetails.
type GetManagedSubAccountFutureesAssetDetailsRequest struct {
	AccountType string `json:"accountType,omitempty"`
}

// GetManagedSubAccountMarginAssetDetailsRequest supplies optional parameters accepted by GetManagedSubAccountMarginAssetDetails.
type GetManagedSubAccountMarginAssetDetailsRequest struct {
	AccountType string `json:"accountType,omitempty"`
}

// GetAssetDetailRequest supplies optional parameters accepted by GetAssetDetail.
type GetAssetDetailRequest struct {
	Asset currency.Code `json:"asset,omitzero"`
}

// GetUserWalletBalanceRequest supplies optional parameters accepted by GetUserWalletBalance.
type GetUserWalletBalanceRequest struct {
	NeedBalanceDetail *bool `json:"needBalanceDetail,omitempty"`
}

// GetDepositAddressForCurrencyRequest supplies optional parameters accepted by GetDepositAddressForCurrency.
type GetDepositAddressForCurrencyRequest struct {
	Amount float64 `json:"amount,omitempty"`
}

// CancelAlgoOrderRequest allows cancellation by the client's algo order ID.
type CancelAlgoOrderRequest struct {
	ClientAlgoID string `json:"clientAlgoId,omitempty"`
}

// SubscribeToLockedProductsRequest selects the account receiving redemptions.
type SubscribeToLockedProductsRequest struct {
	RedeemTo string `json:"redeemTo,omitempty"`
}
