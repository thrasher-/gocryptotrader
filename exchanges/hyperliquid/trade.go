package hyperliquid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/log"
)

func (e *Exchange) ensureInitialised() {
	if e.now == nil {
		e.now = time.Now
	}
	if e.assetCache == nil {
		e.assetCache = make(map[string]int64)
	}
	if e.activeAssetData == nil {
		e.activeAssetData = make(map[string]ActiveAssetDataUpdate)
	}
}

func (e *Exchange) ensureWallet(ctx context.Context) error {
	e.ensureInitialised()
	e.walletMu.Lock()
	defer e.walletMu.Unlock()
	if e.wallet != nil {
		return nil
	}

	creds, err := e.GetCredentials(ctx)
	if err != nil {
		return err
	}
	if creds.Secret == "" {
		return errPrivateKeyRequiredForSignedAction
	}
	w, err := newWalletFromHex(creds.Secret)
	if err != nil {
		return err
	}
	key := strings.ToLower(creds.Key)
	if key != "" && key != w.hexAddress() {
		log.Warnf(log.ExchangeSys, "%s credential key does not match derived address; using derived address", e.Name)
	}
	e.wallet = w
	if creds.ClientID != "" {
		e.accountAddr = strings.ToLower(creds.ClientID)
	} else {
		e.accountAddr = w.hexAddress()
	}
	if creds.SubAccount != "" {
		e.vaultAddress = strings.ToLower(creds.SubAccount)
	}
	e.expiresAfter = nil
	e.assetCache = make(map[string]int64)
	return nil
}

func (e *Exchange) assetID(ctx context.Context, coin string) (int64, error) {
	e.ensureInitialised()
	coin = strings.ToUpper(coin)
	e.assetCacheMu.RLock()
	if id, ok := e.assetCache[coin]; ok {
		e.assetCacheMu.RUnlock()
		return id, nil
	}
	e.assetCacheMu.RUnlock()

	meta, err := e.GetMeta(ctx, "")
	if err != nil {
		return 0, err
	}
	if meta == nil {
		return 0, errPerpMetaNoMarkets
	}

	e.assetCacheMu.Lock()
	defer e.assetCacheMu.Unlock()
	e.assetCache = make(map[string]int64, len(meta.Universe))
	for idx, market := range meta.Universe {
		if market.IsDelisted {
			continue
		}
		e.assetCache[strings.ToUpper(market.Name)] = int64(idx)
	}
	id, ok := e.assetCache[coin]
	if !ok {
		return 0, fmt.Errorf("hyperliquid: unknown coin %s", coin)
	}
	return id, nil
}

func (e *Exchange) executeL1Action(ctx context.Context, action map[string]any, useVault bool) (*ExchangeResponse, error) {
	return e.executeL1ActionWithNonce(ctx, action, useVault, nil)
}

func ensureExchangeResponseOK(resp *ExchangeResponse) error {
	if resp == nil {
		return errResponseMissing
	}
	if !strings.EqualFold(resp.Status, "ok") {
		return errActionStatusNotOK
	}
	return nil
}

func (e *Exchange) nextNonce() (uint64, error) {
	millis := e.now().UnixMilli()
	if millis < 0 {
		return 0, errNegativeNonceTimestamp
	}
	return uint64(millis), nil
}

func (e *Exchange) executeL1ActionWithNonce(ctx context.Context, action map[string]any, useVault bool, overrideNonce *uint64) (*ExchangeResponse, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	nonce, err := e.nextNonce()
	if err != nil {
		return nil, err
	}
	if overrideNonce != nil {
		nonce = *overrideNonce
	}
	var vault *string
	if useVault && e.vaultAddress != "" {
		addr := e.vaultAddress
		vault = &addr
	}
	signature, err := signL1Action(e.wallet, action, vault, nonce, e.expiresAfter, e.isMainnetEndpoint())
	if err != nil {
		return nil, err
	}
	return e.postSignedAction(ctx, action, signature, nonce, vault)
}

func (e *Exchange) postSignedAction(ctx context.Context, action, signature map[string]any, nonce uint64, vaultAddress *string) (*ExchangeResponse, error) {
	payload := map[string]any{
		"action":       action,
		"nonce":        nonce,
		"signature":    signature,
		"expiresAfter": e.expiresAfter,
	}
	if vaultAddress != nil && *vaultAddress != "" {
		payload["vaultAddress"] = strings.ToLower(*vaultAddress)
	} else {
		payload["vaultAddress"] = nil
	}
	var resp ExchangeResponse
	if err := e.sendPOST(ctx, "/exchange", payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (e *Exchange) executeUserSignedAction(ctx context.Context, action map[string]any, signer func(*wallet, map[string]any, bool) (map[string]any, error), nonceApplier func(uint64), vaultOverride *string) (*ExchangeResponse, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	if e.expiresAfter != nil {
		return nil, errExpiresAfterUnsupported
	}
	nonce, err := e.nextNonce()
	if err != nil {
		return nil, err
	}
	if nonceApplier != nil {
		nonceApplier(nonce)
	}
	signature, err := signer(e.wallet, action, e.isMainnetEndpoint())
	if err != nil {
		return nil, err
	}
	var vault *string
	switch {
	case vaultOverride != nil:
		if *vaultOverride != "" {
			addr := strings.ToLower(*vaultOverride)
			vault = &addr
		}
	case e.vaultAddress != "":
		addr := e.vaultAddress
		vault = &addr
	}
	if vaultOverride != nil && *vaultOverride == "" {
		vault = nil
	}
	return e.postSignedAction(ctx, action, signature, nonce, vault)
}

func formatAmountString(amount float64) string {
	return strconv.FormatFloat(amount, 'f', -1, 64)
}

func (e *Exchange) executeSpotDeploy(ctx context.Context, body map[string]any) (*ExchangeResponse, error) {
	action := map[string]any{"type": "spotDeploy"}
	maps.Copy(action, body)
	resp, err := e.executeL1Action(ctx, action, false)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

func (e *Exchange) executePerpDeploy(ctx context.Context, body map[string]any) (*ExchangeResponse, error) {
	action := map[string]any{"type": "perpDeploy"}
	maps.Copy(action, body)
	resp, err := e.executeL1Action(ctx, action, false)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

func (e *Exchange) executeCValidatorAction(ctx context.Context, body map[string]any) (*ExchangeResponse, error) {
	action := map[string]any{"type": "CValidatorAction"}
	maps.Copy(action, body)
	resp, err := e.executeL1Action(ctx, action, false)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

func (e *Exchange) executeCSignerAction(ctx context.Context, variant string) (*ExchangeResponse, error) {
	if variant == "" {
		return nil, errSignerVariantRequired
	}
	action := map[string]any{"type": "CSignerAction"}
	action[variant] = nil
	resp, err := e.executeL1Action(ctx, action, false)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

func (e *Exchange) spotDeployTokenAction(ctx context.Context, variant string, token int) (*ExchangeResponse, error) {
	if variant == "" {
		return nil, errSpotVariantRequired
	}
	action := map[string]any{
		variant: map[string]any{
			"token": token,
		},
	}
	return e.executeSpotDeploy(ctx, action)
}

func mapToPairSlice(input map[string]string) [][]any {
	if len(input) == 0 {
		return [][]any{}
	}
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([][]any, len(keys))
	for i, k := range keys {
		pairs[i] = []any{k, input[k]}
	}
	return pairs
}

// SetVaultAddress overrides the vault address used when signing actions.
func (e *Exchange) SetVaultAddress(address string) {
	e.walletMu.Lock()
	e.vaultAddress = strings.ToLower(address)
	e.walletMu.Unlock()
}

// SetAccountAddress overrides the account address derived from the private key.
func (e *Exchange) SetAccountAddress(address string) {
	e.walletMu.Lock()
	e.accountAddr = strings.ToLower(address)
	e.walletMu.Unlock()
}

// SetExpiresAfter configures an optional expiresAfter timestamp in milliseconds since epoch for future actions.
func (e *Exchange) SetExpiresAfter(expires *uint64) {
	e.walletMu.Lock()
	e.expiresAfter = expires
	e.walletMu.Unlock()
}

// PlaceOrders submits one or more orders.
func (e *Exchange) PlaceOrders(ctx context.Context, orders []OrderRequest, builder *BuilderInfo) (*ExchangeResponse, error) {
	if len(orders) == 0 {
		return nil, errNoOrdersSupplied
	}
	orderWires := make([]map[string]any, len(orders))
	for i := range orders {
		asset, err := e.assetID(ctx, orders[i].Coin)
		if err != nil {
			return nil, err
		}
		wire, err := orderRequestToOrderWire(&orders[i], asset)
		if err != nil {
			return nil, err
		}
		orderWires[i] = wire
	}
	action := orderWiresToOrderAction(orderWires, builder)
	return e.executeL1Action(ctx, action, true)
}

// PlaceOrder submits a single order.
//
//nolint:gocritic // order passed by value to maintain external API compatibility
func (e *Exchange) PlaceOrder(ctx context.Context, order OrderRequest, builder *BuilderInfo) (*ExchangeResponse, error) {
	return e.PlaceOrders(ctx, []OrderRequest{order}, builder)
}

// AmendOrders amends existing orders.
func (e *Exchange) AmendOrders(ctx context.Context, requests []ModifyRequest) (*ExchangeResponse, error) {
	if len(requests) == 0 {
		return nil, errNoModifyRequests
	}
	modifies := make([]map[string]any, len(requests))
	for i := range requests {
		var identifier any
		switch {
		case requests[i].Identifier.OrderID != nil:
			identifier = *requests[i].Identifier.OrderID
		case requests[i].Identifier.ClientOrderID != "":
			identifier = requests[i].Identifier.ClientOrderID
		default:
			return nil, errModifyRequestMissingIdentifier
		}
		asset, err := e.assetID(ctx, requests[i].Order.Coin)
		if err != nil {
			return nil, err
		}
		wire, err := orderRequestToOrderWire(&requests[i].Order, asset)
		if err != nil {
			return nil, err
		}
		modifies[i] = map[string]any{
			"oid":   identifier,
			"order": wire,
		}
	}
	action := map[string]any{
		"type":     "batchModify",
		"modifies": modifies,
	}
	return e.executeL1Action(ctx, action, true)
}

// CancelOrdersByID cancels orders by order ID.
func (e *Exchange) CancelOrdersByID(ctx context.Context, requests []CancelRequest) (*ExchangeResponse, error) {
	if len(requests) == 0 {
		return nil, errCancelBatchNoRequests
	}
	cancels := make([]map[string]any, len(requests))
	for i := range requests {
		if requests[i].OrderID == nil {
			return nil, errCancelRequestMissingOrderID
		}
		asset, err := e.assetID(ctx, requests[i].Coin)
		if err != nil {
			return nil, err
		}
		cancels[i] = map[string]any{
			"a": asset,
			"o": *requests[i].OrderID,
		}
	}
	action := map[string]any{
		"type":    "cancel",
		"cancels": cancels,
	}
	return e.executeL1Action(ctx, action, true)
}

// CancelOrdersByClientID cancels orders by client order ID.
func (e *Exchange) CancelOrdersByClientID(ctx context.Context, requests []CancelByCloidRequest) (*ExchangeResponse, error) {
	if len(requests) == 0 {
		return nil, errCancelBatchNoRequests
	}
	cancels := make([]map[string]any, len(requests))
	for i := range requests {
		if requests[i].ClientOrderID == "" {
			return nil, errCancelRequestMissingClientOrderID
		}
		asset, err := e.assetID(ctx, requests[i].Coin)
		if err != nil {
			return nil, err
		}
		cancels[i] = map[string]any{
			"asset": asset,
			"cloid": requests[i].ClientOrderID,
		}
	}
	action := map[string]any{
		"type":    "cancelByCloid",
		"cancels": cancels,
	}
	return e.executeL1Action(ctx, action, true)
}

// ScheduleCancel schedules (or clears) a mass cancel at the provided Unix millisecond timestamp.
func (e *Exchange) ScheduleCancel(ctx context.Context, scheduledTime *uint64) (*ExchangeResponse, error) {
	action := map[string]any{
		"type": "scheduleCancel",
	}
	if scheduledTime != nil {
		action["time"] = *scheduledTime
	}
	return e.executeL1Action(ctx, action, true)
}

// UpdateLeverage updates leverage for the given asset.
func (e *Exchange) UpdateLeverage(ctx context.Context, coin string, leverage int64, isCross bool) (*ExchangeResponse, error) {
	asset, err := e.assetID(ctx, coin)
	if err != nil {
		return nil, err
	}
	action := map[string]any{
		"type":     "updateLeverage",
		"asset":    asset,
		"isCross":  isCross,
		"leverage": leverage,
	}
	return e.executeL1Action(ctx, action, true)
}

// UpdateIsolatedMargin updates isolated margin for the given asset.
func (e *Exchange) UpdateIsolatedMargin(ctx context.Context, coin string, amount float64, isBuy bool) (*ExchangeResponse, error) {
	asset, err := e.assetID(ctx, coin)
	if err != nil {
		return nil, err
	}
	ntli, err := floatToUSDInt(amount)
	if err != nil {
		return nil, err
	}
	action := map[string]any{
		"type":  "updateIsolatedMargin",
		"asset": asset,
		"isBuy": isBuy,
		"ntli":  ntli,
	}
	return e.executeL1Action(ctx, action, true)
}

// SetReferrer assigns a referral code to the authenticated user.
func (e *Exchange) SetReferrer(ctx context.Context, req *SetReferrerRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSetReferrerRequestNil
	}
	code := req.Code
	if code == "" {
		return nil, errReferrerCodeRequired
	}
	action := map[string]any{
		"type": "setReferrer",
		"code": code,
	}
	resp, err := e.executeL1Action(ctx, action, false)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// CreateSubAccount creates a named sub-account for the authenticated user.
func (e *Exchange) CreateSubAccount(ctx context.Context, req *CreateSubAccountRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errCreateSubAccountRequestNil
	}
	name := req.Name
	if name == "" {
		return nil, errSubAccountNameRequired
	}
	action := map[string]any{
		"type": "createSubAccount",
		"name": name,
	}
	resp, err := e.executeL1Action(ctx, action, false)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// USDClassTransfer moves USD collateral between perp and spot contexts.
func (e *Exchange) USDClassTransfer(ctx context.Context, req *USDClassTransferRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errUSDClassTransferRequestNil
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	amount := formatAmountString(req.Amount)
	if e.vaultAddress != "" {
		amount += " subaccount:" + e.vaultAddress
	}
	action := map[string]any{
		"type":   "usdClassTransfer",
		"amount": amount,
		"toPerp": req.ToPerp,
	}
	empty := ""
	resp, err := e.executeUserSignedAction(ctx, action, signUSDClassTransferAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, &empty)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// SendAsset transfers assets between DEX contexts or sub-accounts.
func (e *Exchange) SendAsset(ctx context.Context, req *SendAssetRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSendAssetRequestNil
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	destination := strings.ToLower(req.Destination)
	if destination == "" {
		return nil, errDestinationRequired
	}
	source := req.SourceDEX
	if source == "" {
		return nil, errSourceDexRequired
	}
	destinationDex := req.DestinationDEX
	if destinationDex == "" {
		return nil, errDestinationDexRequired
	}
	token := req.Token
	if token == "" {
		return nil, errTokenRequired
	}
	action := map[string]any{
		"type":           "sendAsset",
		"destination":    destination,
		"sourceDex":      source,
		"destinationDex": destinationDex,
		"token":          token,
		"amount":         formatAmountString(req.Amount),
		"fromSubAccount": "",
	}
	if e.vaultAddress != "" {
		action["fromSubAccount"] = e.vaultAddress
	}
	empty := ""
	resp, err := e.executeUserSignedAction(ctx, action, signSendAssetAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, &empty)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// SubAccountTransfer moves USD between a sub-account and the primary account.
func (e *Exchange) SubAccountTransfer(ctx context.Context, req *SubAccountTransferRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSubAccountTransferRequestNil
	}
	user := strings.ToLower(req.SubAccountUser)
	if user == "" {
		return nil, errSubAccountUserRequired
	}
	action := map[string]any{
		"type":           "subAccountTransfer",
		"subAccountUser": user,
		"isDeposit":      req.IsDeposit,
		"usd":            req.USD,
	}
	return e.executeL1Action(ctx, action, false)
}

// SubAccountSpotTransfer moves spot tokens between a sub-account and the primary account.
func (e *Exchange) SubAccountSpotTransfer(ctx context.Context, req *SubAccountSpotTransferRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSubAccountSpotTransferRequestNil
	}
	user := strings.ToLower(req.SubAccountUser)
	if user == "" {
		return nil, errSubAccountUserRequired
	}
	token := req.Token
	if token == "" {
		return nil, errTokenRequired
	}
	action := map[string]any{
		"type":           "subAccountSpotTransfer",
		"subAccountUser": user,
		"isDeposit":      req.IsDeposit,
		"token":          token,
		"amount":         formatAmountString(req.Amount),
	}
	return e.executeL1Action(ctx, action, false)
}

// VaultUSDTransfer moves USD between the primary account and an external vault.
func (e *Exchange) VaultUSDTransfer(ctx context.Context, req *VaultUSDTransferRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errVaultTransferRequestNil
	}
	vaultAddress := strings.ToLower(req.VaultAddress)
	if vaultAddress == "" {
		return nil, errVaultAddressRequired
	}
	action := map[string]any{
		"type":         "vaultTransfer",
		"vaultAddress": vaultAddress,
		"isDeposit":    req.IsDeposit,
		"usd":          req.USD,
	}
	resp, err := e.executeL1Action(ctx, action, false)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// USDTransfer sends USD to another address.
func (e *Exchange) USDTransfer(ctx context.Context, req *USDTransferRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errUSDTransferRequestNil
	}
	destination := strings.ToLower(req.Destination)
	if destination == "" {
		return nil, errDestinationRequired
	}
	action := map[string]any{
		"type":        "usdSend",
		"destination": destination,
		"amount":      formatAmountString(req.Amount),
	}
	resp, err := e.executeUserSignedAction(ctx, action, signUSDTransferAction, func(nonce uint64) {
		action["time"] = hexOrDecimalFromUint64(nonce)
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// SpotTransfer sends spot tokens to another address.
func (e *Exchange) SpotTransfer(ctx context.Context, req *SpotTransferRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSpotTransferRequestNil
	}
	destination := strings.ToLower(req.Destination)
	if destination == "" {
		return nil, errDestinationRequired
	}
	token := req.Token
	if token == "" {
		return nil, errTokenRequired
	}
	action := map[string]any{
		"type":        "spotSend",
		"destination": destination,
		"token":       token,
		"amount":      formatAmountString(req.Amount),
	}
	resp, err := e.executeUserSignedAction(ctx, action, signSpotTransferAction, func(nonce uint64) {
		action["time"] = hexOrDecimalFromUint64(nonce)
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// TokenDelegate delegates or undelegates HL tokens to a validator.
func (e *Exchange) TokenDelegate(ctx context.Context, req *TokenDelegateRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errTokenDelegateRequestNil
	}
	validator := strings.ToLower(req.Validator)
	if validator == "" {
		return nil, errValidatorAddressRequired
	}
	action := map[string]any{
		"type":         "tokenDelegate",
		"validator":    validator,
		"wei":          hexOrDecimalFromUint64(req.Wei),
		"isUndelegate": req.IsUndelegate,
	}
	resp, err := e.executeUserSignedAction(ctx, action, signTokenDelegateAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// WithdrawFromBridge withdraws USD from the Hyperliquid bridge.
func (e *Exchange) WithdrawFromBridge(ctx context.Context, req *WithdrawFromBridgeRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errWithdrawRequestNil
	}
	destination := strings.ToLower(req.Destination)
	if destination == "" {
		return nil, errDestinationRequired
	}
	action := map[string]any{
		"type":        "withdraw3",
		"destination": destination,
		"amount":      formatAmountString(req.Amount),
	}
	resp, err := e.executeUserSignedAction(ctx, action, signWithdrawFromBridgeAction, func(nonce uint64) {
		action["time"] = hexOrDecimalFromUint64(nonce)
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// ApproveAgent authorises an agent key to act on behalf of the user.
func (e *Exchange) ApproveAgent(ctx context.Context, req *ApproveAgentRequest) (resp *ExchangeResponse, agentKey string, err error) {
	if walletErr := e.ensureWallet(ctx); walletErr != nil {
		return nil, "", walletErr
	}
	if e.expiresAfter != nil {
		return nil, "", errExpiresAfterUnsupported
	}
	keyBytes := make([]byte, 32)
	if _, err = rand.Read(keyBytes); err != nil {
		return nil, "", fmt.Errorf("hyperliquid: generate agent key: %w", err)
	}
	priv, privErr := ethcrypto.ToECDSA(keyBytes)
	if privErr != nil {
		return nil, "", fmt.Errorf("hyperliquid: construct agent key: %w", privErr)
	}
	agentKey = "0x" + hex.EncodeToString(keyBytes)
	agentAddress := strings.ToLower(ethcrypto.PubkeyToAddress(priv.PublicKey).Hex())
	name := ""
	if req != nil {
		name = req.AgentName
	}
	action := map[string]any{
		"type":         "approveAgent",
		"agentAddress": agentAddress,
		"agentName":    name,
	}
	nonce, nonceErr := e.nextNonce()
	if nonceErr != nil {
		return nil, "", nonceErr
	}
	action["nonce"] = hexOrDecimalFromUint64(nonce)
	signature, signErr := signAgentAction(e.wallet, action, e.isMainnetEndpoint())
	if signErr != nil {
		return nil, "", signErr
	}
	if name == "" {
		delete(action, "agentName")
	}
	var vault *string
	if e.vaultAddress != "" {
		addr := e.vaultAddress
		vault = &addr
	}
	resp, err = e.postSignedAction(ctx, action, signature, nonce, vault)
	if err != nil {
		return nil, "", err
	}
	if statusErr := ensureExchangeResponseOK(resp); statusErr != nil {
		return resp, "", statusErr
	}
	return resp, agentKey, nil
}

// ApproveBuilderFee sets the maximum fee rate for a builder.
func (e *Exchange) ApproveBuilderFee(ctx context.Context, req *ApproveBuilderFeeRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errApproveBuilderFeeRequestNil
	}
	builder := strings.ToLower(req.Builder)
	if builder == "" {
		return nil, errBuilderAddressRequired
	}
	maxFee := req.MaxFeeRate
	if maxFee == "" {
		return nil, errMaxFeeRateRequired
	}
	action := map[string]any{
		"type":       "approveBuilderFee",
		"builder":    builder,
		"maxFeeRate": maxFee,
	}
	resp, err := e.executeUserSignedAction(ctx, action, signApproveBuilderFeeAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// ConvertToMultiSigUser converts the account to a multi-sig controlled account.
func (e *Exchange) ConvertToMultiSigUser(ctx context.Context, req *ConvertToMultiSigUserRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errConvertMultiSigRequestNil
	}
	if req.Threshold <= 0 {
		return nil, errThresholdMustBePositive
	}
	if len(req.AuthorizedUsers) == 0 {
		return nil, errAtLeastOneAuthorisedUserRequired
	}
	authorised := make([]string, len(req.AuthorizedUsers))
	for i := range req.AuthorizedUsers {
		user := strings.ToLower(req.AuthorizedUsers[i])
		if user == "" {
			return nil, errAuthorisedUserMissing
		}
		authorised[i] = user
	}
	sort.Strings(authorised)
	signersPayload, err := json.Marshal(map[string]any{
		"authorizedUsers": authorised,
		"threshold":       req.Threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("hyperliquid: marshal multi-sig signers: %w", err)
	}
	action := map[string]any{
		"type":    "convertToMultiSigUser",
		"signers": string(signersPayload),
	}
	resp, err := e.executeUserSignedAction(ctx, action, signConvertToMultiSigUserAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// SpotDeployRegisterToken registers a new spot token specification.
func (e *Exchange) SpotDeployRegisterToken(ctx context.Context, req *SpotDeployRegisterTokenRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSpotRegisterTokenRequestNil
	}
	name := req.TokenName
	if name == "" {
		return nil, errTokenNameRequired
	}
	fullName := req.FullName
	if fullName == "" {
		return nil, errTokenFullNameRequired
	}
	body := map[string]any{
		"registerToken2": map[string]any{
			"spec": map[string]any{
				"name":        name,
				"szDecimals":  req.SizeDecimals,
				"weiDecimals": req.WeiDecimals,
			},
			"maxGas":   req.MaxGas,
			"fullName": fullName,
		},
	}
	return e.executeSpotDeploy(ctx, body)
}

// SpotDeployUserGenesis seeds user balances for a new spot token.
func (e *Exchange) SpotDeployUserGenesis(ctx context.Context, req *SpotDeployUserGenesisRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSpotUserGenesisRequestNil
	}
	users := make([][]any, len(req.UserAndWei))
	for i := range req.UserAndWei {
		user := strings.ToLower(req.UserAndWei[i].User)
		if user == "" {
			return nil, errUserGenesisEntryMissingUser
		}
		users[i] = []any{user, req.UserAndWei[i].Wei}
	}
	existing := make([][]any, len(req.ExistingTokenAndWei))
	for i := range req.ExistingTokenAndWei {
		existing[i] = []any{req.ExistingTokenAndWei[i].Token, req.ExistingTokenAndWei[i].Wei}
	}
	body := map[string]any{
		"userGenesis": map[string]any{
			"token":               req.Token,
			"userAndWei":          users,
			"existingTokenAndWei": existing,
		},
	}
	return e.executeSpotDeploy(ctx, body)
}

// SpotDeployEnableFreezePrivilege enables the freeze privilege for a token.
func (e *Exchange) SpotDeployEnableFreezePrivilege(ctx context.Context, token int) (*ExchangeResponse, error) {
	return e.spotDeployTokenAction(ctx, "enableFreezePrivilege", token)
}

// SpotDeployFreezeUser freezes or unfreezes a specific user for a spot token.
func (e *Exchange) SpotDeployFreezeUser(ctx context.Context, req *SpotDeployFreezeUserRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSpotFreezeUserRequestNil
	}
	user := strings.ToLower(req.User)
	if user == "" {
		return nil, errUserAddressRequired
	}
	body := map[string]any{
		"freezeUser": map[string]any{
			"token":  req.Token,
			"user":   user,
			"freeze": req.Freeze,
		},
	}
	return e.executeSpotDeploy(ctx, body)
}

// SpotDeployRevokeFreezePrivilege revokes the freeze privilege for a token.
func (e *Exchange) SpotDeployRevokeFreezePrivilege(ctx context.Context, token int) (*ExchangeResponse, error) {
	return e.spotDeployTokenAction(ctx, "revokeFreezePrivilege", token)
}

// SpotDeployEnableQuoteToken marks a token as a valid quote token.
func (e *Exchange) SpotDeployEnableQuoteToken(ctx context.Context, token int) (*ExchangeResponse, error) {
	return e.spotDeployTokenAction(ctx, "enableQuoteToken", token)
}

// SpotDeployGenesis finalises the genesis configuration for a spot token.
func (e *Exchange) SpotDeployGenesis(ctx context.Context, req *SpotDeployGenesisRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSpotGenesisRequestNil
	}
	maxSupply := req.MaxSupply
	if maxSupply == "" {
		return nil, errMaxSupplyRequired
	}
	body := map[string]any{
		"genesis": map[string]any{
			"token":            req.Token,
			"maxSupply":        maxSupply,
			"noHyperliquidity": req.NoHyperliquidity,
		},
	}
	return e.executeSpotDeploy(ctx, body)
}

// SpotDeployRegisterSpot registers a new spot market pair.
func (e *Exchange) SpotDeployRegisterSpot(ctx context.Context, req *SpotDeployRegisterSpotRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSpotRegisterSpotRequestNil
	}
	body := map[string]any{
		"registerSpot": map[string]any{
			"baseToken":  req.BaseToken,
			"quoteToken": req.QuoteToken,
		},
	}
	return e.executeSpotDeploy(ctx, body)
}

// SpotDeployRegisterHyperliquidity configures Hyperliquidity parameters for a spot market.
func (e *Exchange) SpotDeployRegisterHyperliquidity(ctx context.Context, req *SpotDeployRegisterHyperliquidityRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSpotHyperliquidityRequestNil
	}
	body := map[string]any{
		"registerHyperliquidity": map[string]any{
			"spot":    req.Spot,
			"startPx": formatAmountString(req.StartPrice),
			"orderSz": formatAmountString(req.OrderSize),
			"nOrders": req.Orders,
		},
	}
	if req.SeededLevels != nil {
		if register, ok := body["registerHyperliquidity"].(map[string]any); ok {
			register["nSeededLevels"] = *req.SeededLevels
		} else {
			return nil, errRegisterHyperliquidityMalformed
		}
	}
	return e.executeSpotDeploy(ctx, body)
}

// SpotDeploySetDeployerTradingFeeShare adjusts the deployer's fee share for a token.
func (e *Exchange) SpotDeploySetDeployerTradingFeeShare(ctx context.Context, req *SpotDeploySetDeployerTradingFeeShareRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errSpotTradingFeeShareRequestNil
	}
	share := req.Share
	if share == "" {
		return nil, errTradingFeeShareRequired
	}
	body := map[string]any{
		"setDeployerTradingFeeShare": map[string]any{
			"token": req.Token,
			"share": share,
		},
	}
	return e.executeSpotDeploy(ctx, body)
}

// PerpDeployRegisterAsset registers a new perpetual asset configuration.
func (e *Exchange) PerpDeployRegisterAsset(ctx context.Context, req *PerpDeployRegisterAssetRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errPerpRegisterAssetRequestNil
	}
	dex := req.Dex
	if dex == "" {
		return nil, errDexIdentifierRequired
	}
	coin := req.Coin
	if coin == "" {
		return nil, errCoinRequired
	}
	oraclePx := req.OraclePrice
	if oraclePx == "" {
		return nil, errOraclePriceRequired
	}
	assetRequest := map[string]any{
		"coin":          coin,
		"szDecimals":    req.SizeDecimals,
		"oraclePx":      oraclePx,
		"marginTableId": req.MarginTableID,
		"onlyIsolated":  req.OnlyIsolated,
	}
	register := map[string]any{
		"dex":          dex,
		"assetRequest": assetRequest,
		"maxGas":       req.MaxGas,
	}
	if req.Schema != nil {
		schema := map[string]any{
			"fullName":        req.Schema.FullName,
			"collateralToken": req.Schema.CollateralToken,
		}
		if req.Schema.OracleUpdater != nil {
			schema["oracleUpdater"] = strings.ToLower(*req.Schema.OracleUpdater)
		} else {
			schema["oracleUpdater"] = nil
		}
		register["schema"] = schema
	}
	body := map[string]any{
		"registerAsset": register,
	}
	return e.executePerpDeploy(ctx, body)
}

// PerpDeploySetOracle updates oracle pricing for a perpetual DEX.
func (e *Exchange) PerpDeploySetOracle(ctx context.Context, req *PerpDeploySetOracleRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errPerpSetOracleRequestNil
	}
	dex := req.Dex
	if dex == "" {
		return nil, errDexIdentifierRequired
	}
	oracleWire := mapToPairSlice(req.OraclePrices)
	markWire := make([]any, len(req.MarkPrices))
	for i := range req.MarkPrices {
		markWire[i] = mapToPairSlice(req.MarkPrices[i])
	}
	externalWire := mapToPairSlice(req.ExternalPerpPrices)
	body := map[string]any{
		"setOracle": map[string]any{
			"dex":             dex,
			"oraclePxs":       oracleWire,
			"markPxs":         markWire,
			"externalPerpPxs": externalWire,
		},
	}
	return e.executePerpDeploy(ctx, body)
}

// CSignerUnjailSelf requests to unjail the connected signer.
func (e *Exchange) CSignerUnjailSelf(ctx context.Context) (*ExchangeResponse, error) {
	return e.executeCSignerAction(ctx, "unjailSelf")
}

// CSignerJailSelf requests to jail the connected signer.
func (e *Exchange) CSignerJailSelf(ctx context.Context) (*ExchangeResponse, error) {
	return e.executeCSignerAction(ctx, "jailSelf")
}

// CValidatorRegister registers a validator profile.
func (e *Exchange) CValidatorRegister(ctx context.Context, req *CValidatorRegisterRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errValidatorRegisterRequestNil
	}
	nodeIP := req.NodeIP
	if nodeIP == "" {
		return nil, errNodeIPRequired
	}
	name := req.Name
	if name == "" {
		return nil, errValidatorNameRequired
	}
	description := req.Description
	signer := strings.ToLower(req.Signer)
	if signer == "" {
		return nil, errSignerAddressRequired
	}
	body := map[string]any{
		"register": map[string]any{
			"profile": map[string]any{
				"node_ip":              map[string]any{"Ip": nodeIP},
				"name":                 name,
				"description":          description,
				"delegations_disabled": req.DelegationsDisabled,
				"commission_bps":       req.CommissionBPS,
				"signer":               signer,
			},
			"unjailed":    req.Unjailed,
			"initial_wei": req.InitialWei,
		},
	}
	return e.executeCValidatorAction(ctx, body)
}

// CValidatorChangeProfile updates validator profile details.
func (e *Exchange) CValidatorChangeProfile(ctx context.Context, req *CValidatorChangeProfileRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errValidatorChangeProfileRequestNil
	}
	profile := map[string]any{
		"node_ip":             nil,
		"name":                nil,
		"description":         nil,
		"disable_delegations": req.DisableDelegations,
		"commission_bps":      req.CommissionBPS,
		"signer":              nil,
	}
	if req.NodeIP != nil {
		node := *req.NodeIP
		if node != "" {
			profile["node_ip"] = map[string]any{"Ip": node}
		}
	}
	if req.Name != nil {
		profile["name"] = *req.Name
	}
	if req.Description != nil {
		profile["description"] = *req.Description
	}
	if req.Signer != nil {
		signer := strings.ToLower(*req.Signer)
		if signer != "" {
			profile["signer"] = signer
		}
	}
	body := map[string]any{
		"changeProfile": map[string]any{
			"node_ip":             profile["node_ip"],
			"name":                profile["name"],
			"description":         profile["description"],
			"unjailed":            req.Unjailed,
			"disable_delegations": profile["disable_delegations"],
			"commission_bps":      profile["commission_bps"],
			"signer":              profile["signer"],
		},
	}
	return e.executeCValidatorAction(ctx, body)
}

// CValidatorUnregister unregisters the connected validator.
func (e *Exchange) CValidatorUnregister(ctx context.Context) (*ExchangeResponse, error) {
	body := map[string]any{
		"unregister": nil,
	}
	return e.executeCValidatorAction(ctx, body)
}

// MultiSig submits an action signed by multiple parties.
func (e *Exchange) MultiSig(ctx context.Context, req *MultiSigRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errMultiSigRequestNil
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	if req.Action == nil {
		return nil, errMultiSigInnerActionRequired
	}
	if len(req.Signatures) == 0 {
		return nil, errAtLeastOneSignatureRequired
	}
	if req.Nonce == 0 {
		return nil, errNonceRequired
	}
	multiSigUser := strings.ToLower(req.MultiSigUser)
	if multiSigUser == "" {
		return nil, errMultiSigUserRequired
	}
	signatures := make([]map[string]any, len(req.Signatures))
	for i := range req.Signatures {
		signatures[i] = map[string]any{
			"r": req.Signatures[i].R,
			"s": req.Signatures[i].S,
			"v": req.Signatures[i].V,
		}
	}
	outerSigner := e.accountAddr
	if outerSigner == "" {
		outerSigner = e.wallet.hexAddress()
	}
	action := map[string]any{
		"type":             "multiSig",
		"signatureChainId": defaultSignatureChainID,
		"signatures":       signatures,
		"payload": map[string]any{
			"multiSigUser": multiSigUser,
			"outerSigner":  outerSigner,
			"action":       req.Action,
		},
	}
	var vault *string
	if req.VaultAddress != nil {
		addr := strings.ToLower(*req.VaultAddress)
		if addr != "" {
			vault = &addr
		}
	} else if e.vaultAddress != "" {
		addr := e.vaultAddress
		vault = &addr
	}
	signature, err := signMultiSigAction(e.wallet, action, e.isMainnetEndpoint(), vault, req.Nonce, e.expiresAfter)
	if err != nil {
		return nil, err
	}
	resp, err := e.postSignedAction(ctx, action, signature, req.Nonce, vault)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// UseBigBlocks toggles execution against big blocks.
func (e *Exchange) UseBigBlocks(ctx context.Context, req *UseBigBlocksRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errUseBigBlocksRequestNil
	}
	action := map[string]any{
		"type":           "evmUserModify",
		"usingBigBlocks": req.Enable,
	}
	resp, err := e.executeL1Action(ctx, action, false)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// AgentEnableDexAbstraction enables DEX abstraction for the current agent.
func (e *Exchange) AgentEnableDexAbstraction(ctx context.Context) (*ExchangeResponse, error) {
	action := map[string]any{
		"type": "agentEnableDexAbstraction",
	}
	resp, err := e.executeL1Action(ctx, action, true)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// UserDexAbstraction toggles DEX abstraction for a user.
func (e *Exchange) UserDexAbstraction(ctx context.Context, req *UserDexAbstractionRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errUserDexAbstractionRequestNil
	}
	user := strings.ToLower(req.User)
	if user == "" {
		return nil, errUserAddressRequired
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	if e.expiresAfter != nil {
		return nil, errExpiresAfterUnsupported
	}
	nonce, err := e.nextNonce()
	if err != nil {
		return nil, err
	}
	message := map[string]any{
		"user":    user,
		"enabled": req.Enabled,
		"nonce":   hexOrDecimalFromUint64(nonce),
	}
	signature, err := signUserDexAbstractionAction(e.wallet, message, e.isMainnetEndpoint())
	if err != nil {
		return nil, err
	}
	message["type"] = "userDexAbstraction"
	var vault *string
	if e.vaultAddress != "" {
		addr := e.vaultAddress
		vault = &addr
	}
	resp, err := e.postSignedAction(ctx, message, signature, nonce, vault)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// Noop submits a no-op action with the provided nonce.
func (e *Exchange) Noop(ctx context.Context, req *NoopRequest) (*ExchangeResponse, error) {
	if req == nil {
		return nil, errNoopRequestNil
	}
	if req.Nonce == 0 {
		return nil, errNonceRequired
	}
	action := map[string]any{
		"type": "noop",
	}
	resp, err := e.executeL1ActionWithNonce(ctx, action, true, &req.Nonce)
	if err != nil {
		return nil, err
	}
	return resp, ensureExchangeResponseOK(resp)
}

// SubmitOrderSubmit integrates with the existing exchange wrapper submit pathway.
