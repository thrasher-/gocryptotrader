package hyperliquid

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

var (
	errBridgeChainInvalid            = errors.New("invalid Hyperliquid bridge chain")
	errTransferAmountInvalid         = errors.New("transfer amount must be greater than zero")
	errTransferCurrencyInvalid       = errors.New("hyperliquid bridge transfers only support USDC")
	errTransferDEXInvalid            = errors.New("invalid transfer DEX")
	errTransferSubAccountInvalid     = errors.New("user-signed transfer requires an owned subaccount")
	errTransferSubAccountUnsupported = errors.New("this user-signed action does not support a configured subaccount")
	errTransferTokenInvalid          = errors.New("invalid transfer token")
	errWithdrawalAddressTag          = errors.New("hyperliquid bridge withdrawals do not support an address tag")
	errWithdrawalFeeInput            = errors.New("hyperliquid calculates the bridge withdrawal fee")
)

func (e *Exchange) getBridgeChain() string {
	if e.isMainnetEnvironment() {
		return "Arbitrum"
	}
	return "Arbitrum Sepolia"
}

func formatTransferAmount(amount float64) (string, error) {
	if amount <= 0 {
		return "", errTransferAmountInvalid
	}
	formatted, err := floatToWire(amount)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errTransferAmountInvalid, err)
	}
	return formatted, nil
}

func (e *Exchange) validateUserSignedSubAccount(
	ctx context.Context,
	credentials *accounts.Credentials,
	subAccount string,
) error {
	if subAccount == "" {
		return nil
	}
	if credentials == nil {
		return common.ErrNilPointer
	}
	role, err := e.GetUserRole(ctx, subAccount)
	if err != nil {
		return err
	}
	if role.Role != "subAccount" {
		return fmt.Errorf("%w: role is %q", errTransferSubAccountInvalid, role.Role)
	}
	accountAddress, _, err := normaliseAddress(credentials.Key)
	if err != nil {
		return err
	}
	masterAddress, _, err := normaliseAddress(role.Data.Master)
	if err != nil || masterAddress != accountAddress {
		return fmt.Errorf("%w: configured account does not control %s", errTransferSubAccountInvalid, subAccount)
	}
	return nil
}

func (e *Exchange) resolveTransferToken(ctx context.Context, token string) (SpotTokenMetadata, string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return SpotTokenMetadata{}, "", errTransferTokenInvalid
	}
	metadata, err := e.GetSpotMetadata(ctx)
	if err != nil {
		return SpotTokenMetadata{}, "", err
	}
	if token == perpetualQuoteCurrency {
		for i := range metadata.Tokens {
			if metadata.Tokens[i].Name.Equal(currency.USDC) && metadata.Tokens[i].TokenIdentifier != "" {
				return metadata.Tokens[i], metadata.Tokens[i].TokenIdentifier, nil
			}
		}
		return SpotTokenMetadata{}, "", fmt.Errorf("%w: %s metadata is missing", errTransferTokenInvalid, perpetualQuoteCurrency)
	}
	name, tokenID, ok := strings.Cut(token, ":")
	if !ok || name == "" || tokenID == "" {
		return SpotTokenMetadata{}, "", fmt.Errorf("%w: expected NAME:TOKEN_ID", errTransferTokenInvalid)
	}
	for i := range metadata.Tokens {
		metadataName, _, _ := strings.Cut(metadata.Tokens[i].TokenIdentifier, ":")
		if metadataName == name && strings.EqualFold(metadata.Tokens[i].TokenID, tokenID) {
			return metadata.Tokens[i], metadata.Tokens[i].TokenIdentifier, nil
		}
	}
	return SpotTokenMetadata{}, "", fmt.Errorf("%w: %s is not present in spot metadata", errTransferTokenInvalid, token)
}

func (e *Exchange) resolveTransferDEX(ctx context.Context, dex string) (string, error) {
	dex = strings.TrimSpace(dex)
	if dex == "" {
		return "", nil
	}
	if strings.EqualFold(dex, "spot") {
		return "spot", nil
	}
	dexes, err := e.GetPerpetualDEXs(ctx)
	if err != nil {
		return "", err
	}
	for i := 1; i < len(dexes); i++ {
		if dexes[i] != nil && dexes[i].Name == dex {
			return dex, nil
		}
	}
	return "", fmt.Errorf("%w: %q", errTransferDEXInvalid, dex)
}

func (e *Exchange) validateSendAssetRoute(ctx context.Context, arg *SendAssetRequest) (sourceDEX, destinationDEX, resolvedToken string, err error) {
	if arg == nil {
		return "", "", "", common.ErrNilPointer
	}
	useUSDCCollateralIdentifier := strings.TrimSpace(arg.Token) == perpetualQuoteCurrency
	sourceDEX, err = e.resolveTransferDEX(ctx, arg.SourceDEX)
	if err != nil {
		return "", "", "", err
	}
	destinationDEX, err = e.resolveTransferDEX(ctx, arg.DestinationDEX)
	if err != nil {
		return "", "", "", err
	}
	tokenMetadata, resolvedToken, err := e.resolveTransferToken(ctx, arg.Token)
	if err != nil {
		return "", "", "", err
	}
	if useUSDCCollateralIdentifier {
		resolvedToken = perpetualQuoteCurrency
	}
	for _, dex := range []string{sourceDEX, destinationDEX} {
		if dex == "spot" {
			continue
		}
		metadata, err := e.GetPerpetualMetadataForDEX(ctx, dex)
		if err != nil {
			return "", "", "", err
		}
		if metadata.CollateralToken != tokenMetadata.Index {
			return "", "", "", fmt.Errorf(
				"%w: %s is not collateral for DEX %q",
				errTransferTokenInvalid,
				resolvedToken,
				dex,
			)
		}
	}
	return sourceDEX, destinationDEX, resolvedToken, nil
}

// TransferUSDCBetweenSpotAndPerp transfers USDC between spot and the default
// perpetual DEX for the configured account or owned subaccount.
func (e *Exchange) TransferUSDCBetweenSpotAndPerp(ctx context.Context, arg *ClassTransferRequest) (uint64, error) {
	if arg == nil {
		return 0, common.ErrNilPointer
	}
	amountText, err := formatTransferAmount(arg.Amount)
	if err != nil {
		return 0, err
	}
	credentials, subAccount, err := e.getUserSigningCredentials(ctx)
	if err != nil {
		return 0, err
	}
	if err := e.validateUserSignedSubAccount(ctx, credentials, subAccount); err != nil {
		return 0, err
	}
	if subAccount != "" {
		amountText += " subaccount:" + subAccount
	}
	response := new(exchangeActionResponse)
	return e.sendUserSignedAction(ctx, &userSignedActionRequest{
		Credentials: credentials,
		ActionType:  "usdClassTransfer",
		PrimaryType: "HyperliquidTransaction:UsdClassTransfer",
		NonceField:  "nonce",
		Fields: []eip712Field{
			{Name: "amount", Type: "string", Value: amountText},
			{Name: "toPerp", Type: "bool", Value: arg.ToPerpetual},
		},
	}, response)
}

// SendAsset transfers a validated spot or collateral token between Core DEX
// balances, users, and owned subaccounts.
func (e *Exchange) SendAsset(ctx context.Context, arg *SendAssetRequest) (uint64, error) {
	if arg == nil {
		return 0, common.ErrNilPointer
	}
	destination, _, err := normaliseAddress(arg.Destination)
	if err != nil {
		return 0, err
	}
	amountText, err := formatTransferAmount(arg.Amount)
	if err != nil {
		return 0, err
	}
	credentials, subAccount, err := e.getUserSigningCredentials(ctx)
	if err != nil {
		return 0, err
	}
	if err := e.validateUserSignedSubAccount(ctx, credentials, subAccount); err != nil {
		return 0, err
	}
	sourceDEX, destinationDEX, token, err := e.validateSendAssetRoute(ctx, arg)
	if err != nil {
		return 0, err
	}
	response := new(exchangeActionResponse)
	return e.sendUserSignedAction(ctx, &userSignedActionRequest{
		Credentials: credentials,
		ActionType:  "sendAsset",
		PrimaryType: "HyperliquidTransaction:SendAsset",
		NonceField:  "nonce",
		Fields: []eip712Field{
			{Name: "destination", Type: "string", Value: destination},
			{Name: "sourceDex", Type: "string", Value: sourceDEX},
			{Name: "destinationDex", Type: "string", Value: destinationDEX},
			{Name: "token", Type: "string", Value: token},
			{Name: "amount", Type: "string", Value: amountText},
			{Name: "fromSubAccount", Type: "string", Value: subAccount},
		},
	}, response)
}

// SendCoreUSDC sends default-perpetual USDC to another Hyperliquid address.
func (e *Exchange) SendCoreUSDC(ctx context.Context, arg *USDCTransferRequest) (uint64, error) {
	if arg == nil {
		return 0, common.ErrNilPointer
	}
	destination, _, err := normaliseAddress(arg.Destination)
	if err != nil {
		return 0, err
	}
	amountText, err := formatTransferAmount(arg.Amount)
	if err != nil {
		return 0, err
	}
	credentials, subAccount, err := e.getUserSigningCredentials(ctx)
	if err != nil {
		return 0, err
	}
	if subAccount != "" {
		return 0, errTransferSubAccountUnsupported
	}
	response := new(exchangeActionResponse)
	return e.sendUserSignedAction(ctx, &userSignedActionRequest{
		Credentials: credentials,
		ActionType:  "usdSend",
		PrimaryType: "HyperliquidTransaction:UsdSend",
		NonceField:  "time",
		Fields: []eip712Field{
			{Name: "destination", Type: "string", Value: destination},
			{Name: "amount", Type: "string", Value: amountText},
		},
	}, response)
}

// SendCoreSpot sends one spot token to another Hyperliquid address.
func (e *Exchange) SendCoreSpot(ctx context.Context, arg *SpotTransferRequest) (uint64, error) {
	if arg == nil {
		return 0, common.ErrNilPointer
	}
	destination, _, err := normaliseAddress(arg.Destination)
	if err != nil {
		return 0, err
	}
	amountText, err := formatTransferAmount(arg.Amount)
	if err != nil {
		return 0, err
	}
	credentials, subAccount, err := e.getUserSigningCredentials(ctx)
	if err != nil {
		return 0, err
	}
	if subAccount != "" {
		return 0, errTransferSubAccountUnsupported
	}
	_, token, err := e.resolveTransferToken(ctx, arg.Token)
	if err != nil {
		return 0, err
	}
	response := new(exchangeActionResponse)
	return e.sendUserSignedAction(ctx, &userSignedActionRequest{
		Credentials: credentials,
		ActionType:  "spotSend",
		PrimaryType: "HyperliquidTransaction:SpotSend",
		NonceField:  "time",
		Fields: []eip712Field{
			{Name: "destination", Type: "string", Value: destination},
			{Name: "token", Type: "string", Value: token},
			{Name: "amount", Type: "string", Value: amountText},
		},
	}, response)
}

// WithdrawFromBridge requests a USDC withdrawal from HyperCore through the
// configured environment's Arbitrum bridge.
func (e *Exchange) WithdrawFromBridge(ctx context.Context, arg *USDCTransferRequest) (uint64, error) {
	if arg == nil {
		return 0, common.ErrNilPointer
	}
	destination, _, err := normaliseAddress(arg.Destination)
	if err != nil {
		return 0, err
	}
	amountText, err := formatTransferAmount(arg.Amount)
	if err != nil {
		return 0, err
	}
	credentials, subAccount, err := e.getUserSigningCredentials(ctx)
	if err != nil {
		return 0, err
	}
	if subAccount != "" {
		return 0, errTransferSubAccountUnsupported
	}
	response := new(exchangeActionResponse)
	return e.sendUserSignedAction(ctx, &userSignedActionRequest{
		Credentials: credentials,
		ActionType:  "withdraw3",
		PrimaryType: "HyperliquidTransaction:Withdraw",
		NonceField:  "time",
		Fields: []eip712Field{
			{Name: "destination", Type: "string", Value: destination},
			{Name: "amount", Type: "string", Value: amountText},
		},
	}, response)
}

// GetUserNonFundingLedgerUpdates returns up to 500 non-funding account ledger
// updates over an inclusive time range.
func (e *Exchange) GetUserNonFundingLedgerUpdates(ctx context.Context, arg *UserLedgerRequest) ([]UserLedgerUpdate, error) {
	if arg == nil {
		return nil, common.ErrNilPointer
	}
	user, _, err := normaliseAddress(arg.User)
	if err != nil {
		return nil, err
	}
	if err := common.StartEndTimeCheck(arg.StartTime, arg.EndTime); err != nil {
		return nil, err
	}
	var resp []UserLedgerUpdate
	err = e.SendHTTPRequest(ctx, &HTTPRequest{Endpoint: exchange.RestSpot, RateLimit: infoUserLedgerEPL, Payload: &infoRequest{
		Type:      "userNonFundingLedgerUpdates",
		User:      user,
		StartTime: arg.StartTime.UnixMilli(),
		EndTime:   arg.EndTime.UnixMilli(),
	}}, &resp)
	return resp, err
}
