package hyperliquid

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/common"
)

var (
	errInvalidTokenID       = errors.New("token ID must be a 16-byte hexadecimal value")
	errVenueRequired        = errors.New("outcome deployer venue is required")
	errInvalidQuestionState = errors.New("invalid question state")
)

// GetSpotMetadata returns the spot markets and tokens
func (e *Exchange) GetSpotMetadata(ctx context.Context) (*SpotMetadata, error) {
	var resp *SpotMetadata
	if err := e.SendHTTPRequest(ctx, infoStandardEPL, &InfoRequest{Type: "spotMeta"}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// GetSpotMetadataAndAssetContexts returns spot metadata and the current context of each spot coin
func (e *Exchange) GetSpotMetadataAndAssetContexts(ctx context.Context) (*SpotMetadataAndAssetContextsResponse, error) {
	var resp *SpotMetadataAndAssetContextsResponse
	if err := e.SendHTTPRequest(ctx, infoStandardEPL, &InfoRequest{Type: "spotMetaAndAssetCtxs"}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// GetSpotClearinghouseState returns an account's spot token balances
func (e *Exchange) GetSpotClearinghouseState(ctx context.Context, user string) (*SpotClearinghouseStateResponse, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *SpotClearinghouseStateResponse
	if err := e.SendHTTPRequest(ctx, infoLightEPL, &InfoRequest{Type: "spotClearinghouseState", User: user}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// GetSpotDeployState returns a deployer's in-progress spot token deployments and the current spot deploy gas auction;
// completed deployments are not returned
func (e *Exchange) GetSpotDeployState(ctx context.Context, user string) (*SpotDeployStateResponse, error) {
	user, _, err := normaliseAddress(user)
	if err != nil {
		return nil, err
	}
	var resp *SpotDeployStateResponse
	if err := e.SendHTTPRequest(ctx, infoStandardEPL, &InfoRequest{Type: "spotDeployState", User: user}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// GetSpotPairDeployAuctionStatus returns the gas auction for deploying a spot pair between existing tokens
func (e *Exchange) GetSpotPairDeployAuctionStatus(ctx context.Context) (*GasAuction, error) {
	var resp *GasAuction
	if err := e.SendHTTPRequest(ctx, infoStandardEPL, &InfoRequest{Type: "spotPairDeployAuctionStatus"}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// GetTokenDetails returns a spot token's supply, prices, genesis distribution and deployment details by its token ID,
// as SpotTokenMetadata lists it; a token still being deployed is not available
func (e *Exchange) GetTokenDetails(ctx context.Context, tokenID string) (*TokenDetailsResponse, error) {
	if err := validateTokenID(tokenID); err != nil {
		return nil, err
	}
	var resp *TokenDetailsResponse
	if err := e.SendHTTPRequest(ctx, infoStandardEPL, &InfoRequest{Type: "tokenDetails", TokenID: strings.ToLower(tokenID)}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// GetOutcomeMetadata returns the active outcome markets, their questions and the outcome deployers
func (e *Exchange) GetOutcomeMetadata(ctx context.Context) (*OutcomeMetadataResponse, error) {
	var resp *OutcomeMetadataResponse
	if err := e.SendHTTPRequest(ctx, infoStandardEPL, &InfoRequest{Type: "outcomeMeta"}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// GetSettledOutcome returns a settled outcome's specification and settlement; active and unknown outcomes return
// common.ErrNoResponse
func (e *Exchange) GetSettledOutcome(ctx context.Context, outcome uint64) (*SettledOutcomeResponse, error) {
	var resp *SettledOutcomeResponse
	if err := e.SendHTTPRequest(ctx, infoStandardEPL, &InfoRequest{Type: "settledOutcome", Outcome: &outcome}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// GetOutcomeDeployerLimits returns how many more outcomes a venue's deployer may deploy today and have active at once
func (e *Exchange) GetOutcomeDeployerLimits(ctx context.Context, venue string) (*OutcomeDeployerLimitsResponse, error) {
	if strings.TrimSpace(venue) == "" {
		return nil, errVenueRequired
	}
	var resp *OutcomeDeployerLimitsResponse
	if err := e.SendHTTPRequest(ctx, infoStandardEPL, &InfoRequest{Type: "outcomeDeployerLimits", Venue: venue}, &resp); err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, common.ErrNoResponse
	}
	return resp, nil
}

// validateTokenID checks a token ID is a 0x-prefixed 128-bit hexadecimal value
func validateTokenID(tokenID string) error {
	if len(tokenID) != 34 || !strings.EqualFold(tokenID[:2], "0x") {
		return fmt.Errorf("%w: %q", errInvalidTokenID, tokenID)
	}
	if _, err := hex.DecodeString(tokenID[2:]); err != nil {
		return fmt.Errorf("%w: %q", errInvalidTokenID, tokenID)
	}
	return nil
}
