package kraken

import (
	"context"
	"net/url"

	"github.com/thrasher-corp/gocryptotrader/common"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

// CreateSubaccount creates a Kraken subaccount.
func (e *Exchange) CreateSubaccount(ctx context.Context, req *CreateSubaccountRequest) (bool, error) {
	if req == nil {
		return false, common.ErrNilPointer
	}
	if req.Username == "" {
		return false, errUsernameRequired
	}
	if req.Email == "" {
		return false, errEmailRequired
	}

	params := url.Values{"username": {req.Username}, "email": {req.Email}}
	var result bool
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CreateSubaccount", params, &result)
	return result, err
}

// AccountTransfer transfers funds between a primary Kraken account and subaccount.
func (e *Exchange) AccountTransfer(ctx context.Context, req *AccountTransferRequest) (*AccountTransferResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Asset == "" {
		return nil, errAssetRequired
	}
	if req.Amount <= 0 {
		return nil, errAmountInvalid
	}
	if req.From == "" {
		return nil, errFromRequired
	}
	if req.To == "" {
		return nil, errToRequired
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	amount, err := formatSpotFloat(req.Amount)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"asset":  {req.Asset},
		"amount": {amount},
		"from":   {req.From},
		"to":     {req.To},
	}
	if req.AssetClass != "" {
		params.Set("asset_class", string(req.AssetClass))
	}
	var result *AccountTransferResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AccountTransfer", params, &result)
}
