package kraken

import (
	"context"
	"net/url"
	"strconv"

	"github.com/thrasher-corp/gocryptotrader/common"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

// GetDepositMethods returns available deposit methods for an asset.
func (e *Exchange) GetDepositMethods(ctx context.Context, req *GetDepositMethodsRequest) ([]DepositMethodResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Asset == "" {
		return nil, errAssetRequired
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}

	params := url.Values{"asset": {req.Asset}}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result []DepositMethodResponse
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "DepositMethods", params, &result)
	return result, err
}

// GetDepositAddresses returns current deposit addresses with all documented request controls.
func (e *Exchange) GetDepositAddresses(ctx context.Context, req *GetDepositAddressesRequest) ([]DepositAddressResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Asset == "" {
		return nil, errAssetRequired
	}
	if req.Method == "" {
		return nil, errMethodRequired
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}

	params := url.Values{"asset": {req.Asset}, "method": {req.Method}}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if req.New {
		params.Set("new", "true")
	}
	if req.Amount != nil {
		if *req.Amount <= 0 {
			return nil, errAmountInvalid
		}
		amount, err := formatSpotFloat(*req.Amount)
		if err != nil {
			return nil, err
		}
		params.Set("amount", amount)
	}

	var result []DepositAddressResponse
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "DepositAddresses", params, &result)
	return result, err
}

// GetWithdrawalInformation returns calculated withdrawal limits, net amount, and fees.
func (e *Exchange) GetWithdrawalInformation(ctx context.Context, req *GetWithdrawalInformationRequest) (*WithdrawalInformationResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Asset == "" {
		return nil, errAssetRequired
	}
	if req.Key == "" {
		return nil, errKeyRequired
	}
	if req.Amount <= 0 {
		return nil, errAmountInvalid
	}
	amount, err := formatSpotFloat(req.Amount)
	if err != nil {
		return nil, err
	}

	params := url.Values{"asset": {req.Asset}, "key": {req.Key}, "amount": {amount}}
	var result *WithdrawalInformationResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawInfo", params, &result)
}

// WithdrawFunds submits a withdrawal with all current Spot REST controls.
func (e *Exchange) WithdrawFunds(ctx context.Context, req *WithdrawFundsRequest) (*WithdrawFundsResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Asset == "" {
		return nil, errAssetRequired
	}
	if req.Key == "" {
		return nil, errKeyRequired
	}
	if req.Amount <= 0 {
		return nil, errAmountInvalid
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	amount, err := formatSpotFloat(req.Amount)
	if err != nil {
		return nil, err
	}
	maximumFee := ""
	if req.MaximumFee != nil {
		if *req.MaximumFee < 0 {
			return nil, errMaximumFeeInvalid
		}
		maximumFee, err = formatSpotFloat(*req.MaximumFee)
		if err != nil {
			return nil, err
		}
	}

	params := url.Values{"asset": {req.Asset}, "key": {req.Key}, "amount": {amount}}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if req.Address != "" {
		params.Set("address", req.Address)
	}
	if req.MaximumFee != nil {
		params.Set("max_fee", maximumFee)
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result *WithdrawFundsResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "Withdraw", params, &result)
}

// GetRecentDepositsStatus returns recent Spot deposit statuses.
func (e *Exchange) GetRecentDepositsStatus(ctx context.Context, req *GetRecentDepositsStatusRequest) (*RecentDepositsStatusResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	if req.Cursor != "" && req.Paginate != nil {
		return nil, errCursorConflict
	}
	if (!req.Start.IsZero() && req.Start.Unix() < 0) || (!req.End.IsZero() && req.End.Unix() < 0) {
		return nil, errTimestampInvalid
	}
	if !req.Start.IsZero() && !req.End.IsZero() && req.End.Before(req.Start) {
		return nil, errTimeRangeInvalid
	}
	params := url.Values{}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if req.Method != "" {
		params.Set("method", req.Method)
	}
	if !req.Start.IsZero() {
		params.Set("start", strconv.FormatInt(req.Start.Unix(), 10))
	}
	if !req.End.IsZero() {
		params.Set("end", strconv.FormatInt(req.End.Unix(), 10))
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	} else if req.Paginate != nil {
		params.Set("cursor", strconv.FormatBool(*req.Paginate))
	}
	if req.Limit != nil {
		params.Set("limit", strconv.FormatUint(*req.Limit, 10))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result *RecentDepositsStatusResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "DepositStatus", params, &result)
}

// GetRecentWithdrawalsStatus returns recent Spot withdrawal statuses.
func (e *Exchange) GetRecentWithdrawalsStatus(ctx context.Context, req *GetRecentWithdrawalsStatusRequest) (*RecentWithdrawalsStatusResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	if req.Cursor != "" && req.Paginate != nil {
		return nil, errCursorConflict
	}
	if (!req.Start.IsZero() && req.Start.Unix() < 0) || (!req.End.IsZero() && req.End.Unix() < 0) {
		return nil, errTimestampInvalid
	}
	if !req.Start.IsZero() && !req.End.IsZero() && req.End.Before(req.Start) {
		return nil, errTimeRangeInvalid
	}
	params := url.Values{}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if req.Method != "" {
		params.Set("method", req.Method)
	}
	if !req.Start.IsZero() {
		params.Set("start", strconv.FormatInt(req.Start.Unix(), 10))
	}
	if !req.End.IsZero() {
		params.Set("end", strconv.FormatInt(req.End.Unix(), 10))
	}
	if req.Cursor != "" {
		params.Set("cursor", req.Cursor)
	} else if req.Paginate != nil {
		params.Set("cursor", strconv.FormatBool(*req.Paginate))
	}
	if req.Limit != nil {
		params.Set("limit", strconv.FormatUint(*req.Limit, 10))
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result *RecentWithdrawalsStatusResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawStatus", params, &result)
}

// GetWithdrawalMethods returns available withdrawal methods.
func (e *Exchange) GetWithdrawalMethods(ctx context.Context, req *GetWithdrawalMethodsRequest) ([]WithdrawalMethodResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if !isValidSpotEnum(req.RebaseMultiplier, "rebased", "base") {
		return nil, errRebaseMultiplierInvalid
	}
	params := url.Values{}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if req.Network != "" {
		params.Set("network", req.Network)
	}
	if req.RebaseMultiplier != "" {
		params.Set("rebase_multiplier", string(req.RebaseMultiplier))
	}

	var result []WithdrawalMethodResponse
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawMethods", params, &result)
	return result, err
}

// GetWithdrawalAddresses returns configured withdrawal addresses.
func (e *Exchange) GetWithdrawalAddresses(ctx context.Context, req *GetWithdrawalAddressesRequest) ([]WithdrawalAddressResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if !isValidSpotEnum(req.AssetClass, "currency", "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	params := url.Values{}
	if req.Asset != "" {
		params.Set("asset", req.Asset)
	}
	if req.AssetClass != "" {
		params.Set("aclass", string(req.AssetClass))
	}
	if req.Method != "" {
		params.Set("method", req.Method)
	}
	if req.Key != "" {
		params.Set("key", req.Key)
	}
	if req.Verified != nil {
		params.Set("verified", strconv.FormatBool(*req.Verified))
	}

	var result []WithdrawalAddressResponse
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawAddresses", params, &result)
	return result, err
}

// CancelWithdrawal requests cancellation of a recent withdrawal.
func (e *Exchange) CancelWithdrawal(ctx context.Context, req *CancelWithdrawalRequest) (bool, error) {
	if req == nil {
		return false, common.ErrNilPointer
	}
	if req.Asset == "" {
		return false, errAssetRequired
	}
	if req.ReferenceID == "" {
		return false, errReferenceIDRequired
	}

	params := url.Values{"asset": {req.Asset}, "refid": {req.ReferenceID}}
	var result bool
	err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WithdrawCancel", params, &result)
	return result, err
}

// WalletTransfer transfers funds between Kraken Spot and Futures wallets.
func (e *Exchange) WalletTransfer(ctx context.Context, req *WalletTransferRequest) (*WalletTransferResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Asset == "" {
		return nil, errAssetRequired
	}
	if req.From == "" {
		return nil, errFromRequired
	}
	if req.From != WalletSpot {
		return nil, errFromWalletInvalid
	}
	if req.To == "" {
		return nil, errToRequired
	}
	if req.To != WalletFutures {
		return nil, errToWalletInvalid
	}
	if req.Amount <= 0 {
		return nil, errAmountInvalid
	}
	amount, err := formatSpotFloat(req.Amount)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"asset":  {req.Asset},
		"from":   {string(req.From)},
		"to":     {string(req.To)},
		"amount": {amount},
	}
	var result *WalletTransferResponse
	return result, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "WalletTransfer", params, &result)
}

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
