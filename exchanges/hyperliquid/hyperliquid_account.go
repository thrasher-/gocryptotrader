package hyperliquid

import (
	"context"
	"fmt"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/common"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

// GetUserFees returns the effective maker and taker rates for an address.
func (e *Exchange) GetUserFees(ctx context.Context, user string) (*UserFeesResponse, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *UserFeesResponse
	if err := e.SendHTTPRequest(ctx, &HTTPRequest{Endpoint: exchange.RestSpot, RateLimit: infoStandardEPL, Payload: &infoRequest{Type: "userFees", User: user}}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetActiveAssetData returns account-specific leverage and trading limits for a
// perpetual market.
func (e *Exchange) GetActiveAssetData(ctx context.Context, user, coin string) (*ActiveAssetDataResponse, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	coin = strings.TrimSpace(coin)
	if coin == "" {
		return nil, errCoinRequired
	}
	var resp *ActiveAssetDataResponse
	if err := e.SendHTTPRequest(ctx, &HTTPRequest{Endpoint: exchange.RestFutures, RateLimit: infoStandardEPL, Payload: &infoRequest{Type: "activeAssetData", User: user, Coin: coin}}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetUserRole returns the on-chain role for an address.
func (e *Exchange) GetUserRole(ctx context.Context, user string) (*UserRoleResponse, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *UserRoleResponse
	if err := e.SendHTTPRequest(ctx, &HTTPRequest{Endpoint: exchange.RestSpot, RateLimit: infoUserRoleEPL, Payload: &infoRequest{Type: "userRole", User: user}}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetVaultDetails returns ownership details for a vault address.
func (e *Exchange) GetVaultDetails(ctx context.Context, vault, user string) (*VaultDetailsResponse, error) {
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
	var resp *VaultDetailsResponse
	if err := e.SendHTTPRequest(ctx, &HTTPRequest{Endpoint: exchange.RestSpot, RateLimit: infoStandardEPL, Payload: &infoRequest{Type: "vaultDetails", Vault: vault, User: user}}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}

// GetSpotClearinghouseState returns spot balances for an address.
func (e *Exchange) GetSpotClearinghouseState(ctx context.Context, user string) (*SpotClearinghouseStateResponse, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *SpotClearinghouseStateResponse
	if err := e.SendHTTPRequest(ctx, &HTTPRequest{Endpoint: exchange.RestSpot, RateLimit: infoLightEPL, Payload: &infoRequest{Type: "spotClearinghouseState", User: user}}, &resp); err != nil {
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
	if err := e.SendHTTPRequest(ctx, &HTTPRequest{Endpoint: exchange.RestSpot, RateLimit: infoStandardEPL, Payload: &infoRequest{Type: "userAbstraction", User: user}}, &resp); err != nil {
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
func (e *Exchange) GetClearinghouseState(ctx context.Context, user string) (*ClearinghouseStateResponse, error) {
	return e.GetClearinghouseStateForDEX(ctx, user, "")
}

// GetClearinghouseStateForDEX returns perpetual account state for one DEX.
func (e *Exchange) GetClearinghouseStateForDEX(ctx context.Context, user, dex string) (*ClearinghouseStateResponse, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *ClearinghouseStateResponse
	if err := e.SendHTTPRequest(ctx, &HTTPRequest{Endpoint: exchange.RestFutures, RateLimit: infoLightEPL, Payload: &infoRequest{Type: "clearinghouseState", User: user, DEX: dex}}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNilPointer
	}
	return resp, nil
}
