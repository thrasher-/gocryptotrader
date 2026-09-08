package hyperliquid

import (
	"context"
	"fmt"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/common"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

// GetUserFees returns the effective maker and taker rates for an address.
func (e *Exchange) GetUserFees(ctx context.Context, user string) (*UserFees, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *UserFees
	if err := e.SendHTTPRequest(ctx, exchange.RestSpot, infoStandardEPL, &infoRequest{Type: "userFees", User: user}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetActiveAssetData returns account-specific leverage and trading limits for a
// perpetual market.
func (e *Exchange) GetActiveAssetData(ctx context.Context, user, coin string) (*ActiveAssetData, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	coin = strings.TrimSpace(coin)
	if coin == "" {
		return nil, errCoinRequired
	}
	var resp *ActiveAssetData
	if err := e.SendHTTPRequest(ctx, exchange.RestFutures, infoStandardEPL, &infoRequest{Type: "activeAssetData", User: user, Coin: coin}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetUserRole returns the on-chain role for an address.
func (e *Exchange) GetUserRole(ctx context.Context, user string) (*UserRole, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *UserRole
	if err := e.SendHTTPRequest(ctx, exchange.RestSpot, infoUserRoleEPL, &infoRequest{Type: "userRole", User: user}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetVaultDetails returns ownership details for a vault address.
func (e *Exchange) GetVaultDetails(ctx context.Context, vault, user string) (*VaultDetails, error) {
	vault, _, err := normaliseAddress(vault)
	if err != nil {
		return nil, err
	}
	if user != "" {
		user, _, err = normaliseAddress(user)
		if err != nil {
			return nil, err
		}
	}
	var resp *VaultDetails
	if err := e.SendHTTPRequest(ctx, exchange.RestSpot, infoStandardEPL, &infoRequest{Type: "vaultDetails", Vault: vault, User: user}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetSpotClearinghouseState returns spot balances for an address.
func (e *Exchange) GetSpotClearinghouseState(ctx context.Context, user string) (*SpotClearinghouseState, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *SpotClearinghouseState
	if err := e.SendHTTPRequest(ctx, exchange.RestSpot, infoLightEPL, &infoRequest{Type: "spotClearinghouseState", User: user}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetUserAbstraction returns the account abstraction mode for an address.
func (e *Exchange) GetUserAbstraction(ctx context.Context, user string) (AccountAbstraction, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return "", err
	}
	var resp AccountAbstraction
	if err := e.SendHTTPRequest(ctx, exchange.RestSpot, infoStandardEPL, &infoRequest{Type: "userAbstraction", User: user}, &resp); err != nil {
		return "", err
	}
	switch resp {
	case AccountAbstractionDefault,
		AccountAbstractionDisabled,
		AccountAbstractionDEX,
		AccountAbstractionUnified,
		AccountAbstractionPortfolio:
		return resp, nil
	default:
		return "", fmt.Errorf("%w: %q", errAccountAbstractionInvalid, resp)
	}
}

// GetClearinghouseState returns default-DEX perpetual account state for an address.
func (e *Exchange) GetClearinghouseState(ctx context.Context, user string) (*ClearinghouseState, error) {
	return e.GetClearinghouseStateForDEX(ctx, user, "")
}

// GetClearinghouseStateForDEX returns perpetual account state for one DEX.
func (e *Exchange) GetClearinghouseStateForDEX(ctx context.Context, user, dex string) (*ClearinghouseState, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *ClearinghouseState
	if err := e.SendHTTPRequest(ctx, exchange.RestFutures, infoLightEPL, &infoRequest{Type: "clearinghouseState", User: user, DEX: dex}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}
