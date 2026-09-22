package binance

import (
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// LeadTraderStatusResponse contains the copy trading status and API result.
type LeadTraderStatusResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Success bool   `json:"success"`

	Data *LeadTraderStatus `json:"data"`
}

// LeadTradingSymbolsResponse contains the symbols available to a lead trader.
type LeadTradingSymbolsResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`

	Data []*LeadTradingSymbolItem `json:"data"`
}

// AccountFiltersResponse contains the exchange, symbol and asset limits for an account.
type AccountFiltersResponse struct {
	ExchangeFilters []*AccountFiltersExchangeFiltersResponse `json:"exchangeFilters"`
	SymbolFilters   []*filterData                            `json:"symbolFilters"`
	AssetFilters    []*AccountFiltersAssetFiltersResponse    `json:"assetFilters"`
	RateLimits      []*RateLimitItem                         `json:"rateLimits"`
}

// AmendKeepPriorityResponse contains the amended order and its execution details.
type AmendKeepPriorityResponse struct {
	TransactTime types.Time                           `json:"transactTime"`
	ExecutionID  uint64                               `json:"executionId"`
	ListStatus   *AmendKeepPriorityListStatusResponse `json:"listStatus"`

	AmendedOrder *TradeOrder `json:"amendedOrder"`
}

// TestOrderResponse contains the commission estimates requested by a test order.
type TestOrderResponse struct {
	StandardCommissionForOrder *OrderCommissionResponse   `json:"standardCommissionForOrder"`
	SpecialCommissionForOrder  *OrderCommissionResponse   `json:"specialCommissionForOrder"`
	TaxCommissionForOrder      *OrderCommissionResponse   `json:"taxCommissionForOrder"`
	Discount                   *TestOrderDiscountResponse `json:"discount"`
}

// MarginAccountOrderFillsResponse contains the documented MarginAccountOrderFills fields.
type MarginAccountOrderFillsResponse struct {
	Price           types.Number  `json:"price"`
	Quantity        types.Number  `json:"qty"`
	Commission      types.Number  `json:"commission"`
	CommissionAsset currency.Code `json:"commissionAsset"`
	TradeID         uint64        `json:"tradeId"`
}

// AccountCommissionRateSpecialCommissionResponse contains the documented AccountCommissionRateSpecialCommission fields.
type AccountCommissionRateSpecialCommissionResponse struct {
	Maker  types.Number `json:"maker"`
	Taker  types.Number `json:"taker"`
	Buyer  types.Number `json:"buyer"`
	Seller types.Number `json:"seller"`
}

// AccountFiltersExchangeFiltersResponse contains the documented AccountFiltersExchangeFilters fields.
type AccountFiltersExchangeFiltersResponse struct {
	FilterType             string `json:"filterType"`
	MaxNumberOrders        uint64 `json:"maxNumOrders"`
	MaxNumberAlgoOrders    uint64 `json:"maxNumAlgoOrders"`
	MaxNumberIcebergOrders uint64 `json:"maxNumIcebergOrders"`
	MaxNumberOrderLists    uint64 `json:"maxNumOrderLists"`
}

// AccountFiltersAssetFiltersResponse contains the documented AccountFiltersAssetFilters fields.
type AccountFiltersAssetFiltersResponse struct {
	FilterType       string        `json:"filterType"`
	QuantityExponent int64         `json:"qtyExponent"`
	Limit            types.Number  `json:"limit"`
	Asset            currency.Code `json:"asset"`
}

// ExchangeInfoSorsResponse contains the documented ExchangeInfoSors fields.
type ExchangeInfoSorsResponse struct {
	BaseAsset currency.Code `json:"baseAsset"`
	Symbols   []string      `json:"symbols"`
}

// AmendKeepPriorityListStatusResponse contains the documented AmendKeepPriorityListStatus fields.
type AmendKeepPriorityListStatusResponse struct {
	OrderListID       int64                                        `json:"orderListId"`
	ContingencyType   string                                       `json:"contingencyType"`
	ListOrderStatus   string                                       `json:"listOrderStatus"`
	ListClientOrderID string                                       `json:"listClientOrderId"`
	Symbol            string                                       `json:"symbol"`
	Orders            []*AmendKeepPriorityListStatusOrdersResponse `json:"orders"`
}

// AmendKeepPriorityListStatusOrdersResponse contains the documented AmendKeepPriorityListStatusOrders fields.
type AmendKeepPriorityListStatusOrdersResponse struct {
	Symbol        string `json:"symbol"`
	OrderID       uint64 `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
}

// TestOrderDiscountResponse contains the documented TestOrderDiscount fields.
type TestOrderDiscountResponse struct {
	EnabledForAccount bool          `json:"enabledForAccount"`
	EnabledForSymbol  bool          `json:"enabledForSymbol"`
	DiscountAsset     currency.Code `json:"discountAsset"`
	Discount          types.Number  `json:"discount"`
}

// BNSOLRewardItemBoostRewardsResponse contains the documented BNSOLRewardItemBoostRewards fields.
type BNSOLRewardItemBoostRewardsResponse struct {
	BoostAPR     types.Number  `json:"boostAPR"`
	RewardsAsset currency.Code `json:"rewardsAsset"`
}

// SubAccountFuturesPositionRiskFuturePositionRiskVOsResponse contains the documented SubAccountFuturesPositionRiskFuturePositionRiskVOs fields.
type SubAccountFuturesPositionRiskFuturePositionRiskVOsResponse struct {
	EntryPrice       types.Number `json:"entryPrice"`
	Leverage         types.Number `json:"leverage"`
	MaxNotional      types.Number `json:"maxNotional"`
	LiquidationPrice types.Number `json:"liquidationPrice"`
	MarkPrice        types.Number `json:"markPrice"`
	PositionAmount   types.Number `json:"positionAmount"`
	Symbol           string       `json:"symbol"`
	UnrealisedProfit types.Number `json:"unrealizedProfit"`
}

// SubAccountFuturesPositionRiskDeliveryPositionRiskVOsResponse contains the documented SubAccountFuturesPositionRiskDeliveryPositionRiskVOs fields.
type SubAccountFuturesPositionRiskDeliveryPositionRiskVOsResponse struct {
	EntryPrice       types.Number  `json:"entryPrice"`
	MarkPrice        types.Number  `json:"markPrice"`
	Leverage         types.Number  `json:"leverage"`
	Isolated         string        `json:"isolated"`
	IsolatedWallet   types.Number  `json:"isolatedWallet"`
	IsolatedMargin   types.Number  `json:"isolatedMargin"`
	IsAutoAddMargin  types.Boolean `json:"isAutoAddMargin"`
	PositionSide     string        `json:"positionSide"`
	PositionAmount   types.Number  `json:"positionAmount"`
	Symbol           string        `json:"symbol"`
	UnrealisedProfit types.Number  `json:"unrealizedProfit"`
}

// MarginedFuturesAccountDeliveryAccountRespResponse contains the documented MarginedFuturesAccountDeliveryAccountResp fields.
type MarginedFuturesAccountDeliveryAccountRespResponse struct {
	Email       string                                                     `json:"email"`
	Assets      []*MarginedFuturesAccountDeliveryAccountRespAssetsResponse `json:"assets"`
	CanDeposit  bool                                                       `json:"canDeposit"`
	CanTrade    bool                                                       `json:"canTrade"`
	CanWithdraw bool                                                       `json:"canWithdraw"`
	FeeTier     uint64                                                     `json:"feeTier"`
	UpdateTime  types.Time                                                 `json:"updateTime"`
}

// MarginedFuturesAccountDeliveryAccountRespAssetsResponse contains the documented MarginedFuturesAccountDeliveryAccountRespAssets fields.
type MarginedFuturesAccountDeliveryAccountRespAssetsResponse struct {
	Asset                  currency.Code `json:"asset"`
	InitialMargin          types.Number  `json:"initialMargin"`
	MaintenanceMargin      types.Number  `json:"maintenanceMargin"`
	MarginBalance          types.Number  `json:"marginBalance"`
	MaxWithdrawAmount      types.Number  `json:"maxWithdrawAmount"`
	OpenOrderInitialMargin types.Number  `json:"openOrderInitialMargin"`
	PositionInitialMargin  types.Number  `json:"positionInitialMargin"`
	UnrealisedProfit       types.Number  `json:"unrealizedProfit"`
	WalletBalance          types.Number  `json:"walletBalance"`
}

// SubAccountAssetsSnapshotSnapshotVosDataUserAssetsResponse contains the documented SubAccountAssetsSnapshotSnapshotVosDataUserAssets fields.
type SubAccountAssetsSnapshotSnapshotVosDataUserAssetsResponse struct {
	Asset    currency.Code `json:"asset"`
	Borrowed types.Number  `json:"borrowed"`
	Free     types.Number  `json:"free"`
	Interest types.Number  `json:"interest"`
	Locked   types.Number  `json:"locked"`
	NetAsset types.Number  `json:"netAsset"`
}

// SubAccountAssetsSnapshotSnapshotVosDataAssetsResponse contains the documented SubAccountAssetsSnapshotSnapshotVosDataAssets fields.
type SubAccountAssetsSnapshotSnapshotVosDataAssetsResponse struct {
	Asset         currency.Code `json:"asset"`
	MarginBalance types.Number  `json:"marginBalance"`
	WalletBalance types.Number  `json:"walletBalance"`
}

// SubAccountAssetsSnapshotSnapshotVosDataPositionResponse contains the documented SubAccountAssetsSnapshotSnapshotVosDataPosition fields.
type SubAccountAssetsSnapshotSnapshotVosDataPositionResponse struct {
	EntryPrice       types.Number `json:"entryPrice"`
	MarkPrice        types.Number `json:"markPrice"`
	PositionAmount   types.Number `json:"positionAmt"`
	Symbol           string       `json:"symbol"`
	UnrealisedProfit types.Number `json:"unRealizedProfit"`
}

// DailyAccountSnapshotSnapshotVosDataUserAssetsResponse contains the documented DailyAccountSnapshotSnapshotVosDataUserAssets fields.
type DailyAccountSnapshotSnapshotVosDataUserAssetsResponse struct {
	Asset    currency.Code `json:"asset"`
	Borrowed types.Number  `json:"borrowed"`
	Free     types.Number  `json:"free"`
	Interest types.Number  `json:"interest"`
	Locked   types.Number  `json:"locked"`
	NetAsset types.Number  `json:"netAsset"`
}

// DailyAccountSnapshotSnapshotVosDataAssetsResponse contains the documented DailyAccountSnapshotSnapshotVosDataAssets fields.
type DailyAccountSnapshotSnapshotVosDataAssetsResponse struct {
	Asset         currency.Code `json:"asset"`
	MarginBalance types.Number  `json:"marginBalance"`
	WalletBalance types.Number  `json:"walletBalance"`
}

// DailyAccountSnapshotSnapshotVosDataPositionResponse contains the documented DailyAccountSnapshotSnapshotVosDataPosition fields.
type DailyAccountSnapshotSnapshotVosDataPositionResponse struct {
	EntryPrice       types.Number `json:"entryPrice"`
	MarkPrice        types.Number `json:"markPrice"`
	PositionAmount   types.Number `json:"positionAmt"`
	Symbol           string       `json:"symbol"`
	UnrealisedProfit types.Number `json:"unRealizedProfit"`
}

// UserWalletBalanceAssetBalancesResponse contains the documented UserWalletBalanceAssetBalances fields.
type UserWalletBalanceAssetBalancesResponse struct {
	Asset        currency.Code `json:"asset"`
	AssetName    string        `json:"assetName"`
	Free         types.Number  `json:"free"`
	Locked       types.Number  `json:"locked"`
	Freeze       types.Number  `json:"freeze"`
	Withdrawing  types.Number  `json:"withdrawing"`
	BTCValuation types.Number  `json:"btcValuation"`
}

// OrderCommissionResponse contains the maker and taker commission rates for an order.
type OrderCommissionResponse struct {
	Maker types.Number `json:"maker"`
	Taker types.Number `json:"taker"`
}

// DepositCreditApplyResponse reports whether an expired-address deposit was credited.
type DepositCreditApplyResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    bool   `json:"data"`
	Success bool   `json:"success"`
}

// MarginRestrictedAssetsResponse identifies assets restricted for new positions or collateral.
type MarginRestrictedAssetsResponse struct {
	OpenLongRestrictedAsset    []string `json:"openLongRestrictedAsset"`
	MaxCollateralExceededAsset []string `json:"maxCollateralExceededAsset"`
}

// HashrateRescaleCancelResponse reports whether a mining rescale configuration was cancelled.
type HashrateRescaleCancelResponse struct {
	Code    int64  `json:"code"`
	Message string `json:"msg"`
	Data    bool   `json:"data"`
}

// OrderFillResponse describes an individual fill in a FULL order response.
type OrderFillResponse struct {
	Price           types.Number  `json:"price"`
	Quantity        types.Number  `json:"qty"`
	Commission      types.Number  `json:"commission"`
	CommissionAsset currency.Code `json:"commissionAsset"`
	TradeID         uint64        `json:"tradeId"`
}

// OrderListEntryResponse identifies an order belonging to an order list.
type OrderListEntryResponse struct {
	Symbol        string `json:"symbol"`
	OrderID       uint64 `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
}
