package hyperliquid

import (
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// AccountAbstraction identifies how Hyperliquid shares balances across spot
// and perpetual DEXes.
type AccountAbstraction string

// Hyperliquid account abstraction modes returned by the info API.
const (
	AccountAbstractionDefault   AccountAbstraction = "default"
	AccountAbstractionDisabled  AccountAbstraction = "disabled"
	AccountAbstractionDEX       AccountAbstraction = "dexAbstraction"
	AccountAbstractionUnified   AccountAbstraction = "unifiedAccount"
	AccountAbstractionPortfolio AccountAbstraction = "portfolioMargin"
)

// UserFeesResponse contains effective account-specific trade fee rates.
type UserFeesResponse struct {
	UserCrossRate     types.Number `json:"userCrossRate"`
	UserAddRate       types.Number `json:"userAddRate"`
	UserSpotCrossRate types.Number `json:"userSpotCrossRate"`
	UserSpotAddRate   types.Number `json:"userSpotAddRate"`
}

// ActiveAssetDataResponse contains account-specific settings for one perpetual market.
type ActiveAssetDataResponse struct {
	User     string          `json:"user"`
	Coin     string          `json:"coin"`
	Leverage AccountLeverage `json:"leverage"`
}

// AccountLeverage contains one cross or isolated leverage setting.
type AccountLeverage struct {
	Type   string       `json:"type"`
	Value  types.Number `json:"value"`
	RawUSD types.Number `json:"rawUsd"`
}

// UserRoleResponse contains the account role for an on-chain address.
type UserRoleResponse struct {
	Role string       `json:"role"`
	Data UserRoleData `json:"data"`
}

// UserRoleData contains a role's related master or user address.
type UserRoleData struct {
	User   string `json:"user"`
	Master string `json:"master"`
}

// VaultDetailsResponse contains the ownership fields needed to validate vault trading authority.
type VaultDetailsResponse struct {
	VaultAddress string `json:"vaultAddress"`
	Leader       string `json:"leader"`
}

// SpotClearinghouseStateResponse contains spot balances for one account.
type SpotClearinghouseStateResponse struct {
	Balances []SpotBalance `json:"balances"`
}

// SpotBalance contains one spot token balance.
type SpotBalance struct {
	Coin       currency.Code `json:"coin"`
	TokenIndex uint64        `json:"token"`
	Total      types.Number  `json:"total"`
	Hold       types.Number  `json:"hold"`
	EntryValue types.Number  `json:"entryNtl"`
}

// ClearinghouseStateResponse contains perpetual account balances and positions.
type ClearinghouseStateResponse struct {
	MarginSummary      MarginSummary   `json:"marginSummary"`
	CrossMarginSummary MarginSummary   `json:"crossMarginSummary"`
	Withdrawable       types.Number    `json:"withdrawable"`
	AssetPositions     []AssetPosition `json:"assetPositions"`
}

// MarginSummary contains aggregate perpetual margin values.
type MarginSummary struct {
	AccountValue    types.Number `json:"accountValue"`
	TotalMarginUsed types.Number `json:"totalMarginUsed"`
	TotalNotional   types.Number `json:"totalNtlPos"`
	TotalRawUSD     types.Number `json:"totalRawUsd"`
}

// AssetPosition contains one perpetual position.
type AssetPosition struct {
	Type     string   `json:"type"`
	Position Position `json:"position"`
}

// Position contains one perpetual market position.
type Position struct {
	Coin             string       `json:"coin"`
	EntryPrice       types.Number `json:"entryPx"`
	LiquidationPrice types.Number `json:"liquidationPx"`
	MarginUsed       types.Number `json:"marginUsed"`
	PositionValue    types.Number `json:"positionValue"`
	ReturnOnEquity   types.Number `json:"returnOnEquity"`
	Size             types.Number `json:"szi"`
	UnrealisedProfit types.Number `json:"unrealizedPnl"`
}
