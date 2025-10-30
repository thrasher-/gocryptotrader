package hyperliquid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/thrasher-corp/gocryptotrader/log"
)

var errPrivateKeyMissing = errors.New("hyperliquid: private key required for signed action")

func (e *Exchange) ensureInitialised() {
	if e.now == nil {
		e.now = time.Now
	}
	if e.assetCache == nil {
		e.assetCache = make(map[string]int64)
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
		return errPrivateKeyMissing
	}
	w, err := newWalletFromHex(creds.Secret)
	if err != nil {
		return err
	}
	key := strings.ToLower(strings.TrimSpace(creds.Key))
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

	resp, err := e.GetMeta(ctx, "")
	if err != nil {
		return 0, err
	}
	var meta MetaResponse
	if err := json.Unmarshal(resp, &meta); err != nil {
		return 0, err
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

func (e *Exchange) executeL1Action(ctx context.Context, action map[string]any, useVault bool) (json.RawMessage, error) {
	return e.executeL1ActionWithNonce(ctx, action, useVault, nil)
}

func (e *Exchange) executeL1ActionWithNonce(ctx context.Context, action map[string]any, useVault bool, overrideNonce *uint64) (json.RawMessage, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	nonce := uint64(e.now().UnixMilli())
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
	return e.postSignedAction(ctx, action, signature, nonce, true, vault)
}

func (e *Exchange) postSignedAction(ctx context.Context, action map[string]any, signature map[string]any, nonce uint64, includeVault bool, vaultAddress *string) (json.RawMessage, error) {
	payload := map[string]any{
		"action":       action,
		"nonce":        nonce,
		"signature":    signature,
		"expiresAfter": e.expiresAfter,
	}
	if includeVault {
		if vaultAddress != nil && *vaultAddress != "" {
			payload["vaultAddress"] = strings.ToLower(*vaultAddress)
		} else {
			payload["vaultAddress"] = nil
		}
	}
	var resp json.RawMessage
	if err := e.sendPOST(ctx, "/exchange", payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (e *Exchange) executeUserSignedAction(ctx context.Context, action map[string]any, signer func(*wallet, map[string]any, bool) (map[string]any, error), nonceApplier func(uint64), overrideNonce *uint64, vaultOverride *string, includeVault bool) (json.RawMessage, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	if e.expiresAfter != nil {
		return nil, fmt.Errorf("hyperliquid: expiresAfter not supported for this action")
	}
	nonce := uint64(e.now().UnixMilli())
	if overrideNonce != nil {
		nonce = *overrideNonce
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
	return e.postSignedAction(ctx, action, signature, nonce, includeVault, vault)
}

func formatAmountString(amount float64) string {
	return strconv.FormatFloat(amount, 'f', -1, 64)
}

func (e *Exchange) executeSpotDeploy(ctx context.Context, body map[string]any) (json.RawMessage, error) {
	action := map[string]any{"type": "spotDeploy"}
	for k, v := range body {
		action[k] = v
	}
	return e.executeL1Action(ctx, action, false)
}

func (e *Exchange) executePerpDeploy(ctx context.Context, body map[string]any) (json.RawMessage, error) {
	action := map[string]any{"type": "perpDeploy"}
	for k, v := range body {
		action[k] = v
	}
	return e.executeL1Action(ctx, action, false)
}

func (e *Exchange) executeCValidatorAction(ctx context.Context, body map[string]any) (json.RawMessage, error) {
	action := map[string]any{"type": "CValidatorAction"}
	for k, v := range body {
		action[k] = v
	}
	return e.executeL1Action(ctx, action, false)
}

func (e *Exchange) executeCSignerAction(ctx context.Context, variant string) (json.RawMessage, error) {
	if variant == "" {
		return nil, fmt.Errorf("hyperliquid: signer variant required")
	}
	action := map[string]any{"type": "CSignerAction"}
	action[variant] = nil
	return e.executeL1Action(ctx, action, false)
}

func (e *Exchange) spotDeployTokenAction(ctx context.Context, variant string, token int) (json.RawMessage, error) {
	if variant == "" {
		return nil, fmt.Errorf("hyperliquid: spot deploy variant required")
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
func (e *Exchange) PlaceOrders(ctx context.Context, orders []OrderRequest, builder *BuilderInfo) (json.RawMessage, error) {
	if len(orders) == 0 {
		return nil, fmt.Errorf("hyperliquid: no orders supplied")
	}
	orderWires := make([]map[string]any, len(orders))
	for i := range orders {
		asset, err := e.assetID(ctx, orders[i].Coin)
		if err != nil {
			return nil, err
		}
		wire, err := orderRequestToOrderWire(orders[i], asset)
		if err != nil {
			return nil, err
		}
		orderWires[i] = wire
	}
	action := orderWiresToOrderAction(orderWires, builder)
	return e.executeL1Action(ctx, action, true)
}

// PlaceOrder submits a single order.
func (e *Exchange) PlaceOrder(ctx context.Context, order OrderRequest, builder *BuilderInfo) (json.RawMessage, error) {
	return e.PlaceOrders(ctx, []OrderRequest{order}, builder)
}

// AmendOrders amends existing orders.
func (e *Exchange) AmendOrders(ctx context.Context, requests []ModifyRequest) (json.RawMessage, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("hyperliquid: no modify requests supplied")
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
			return nil, fmt.Errorf("hyperliquid: modify request missing identifier")
		}
		asset, err := e.assetID(ctx, requests[i].Order.Coin)
		if err != nil {
			return nil, err
		}
		wire, err := orderRequestToOrderWire(requests[i].Order, asset)
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
func (e *Exchange) CancelOrdersByID(ctx context.Context, requests []CancelRequest) (json.RawMessage, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("hyperliquid: no cancel requests supplied")
	}
	cancels := make([]map[string]any, len(requests))
	for i := range requests {
		if requests[i].OrderID == nil {
			return nil, fmt.Errorf("hyperliquid: cancel request missing order ID")
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
func (e *Exchange) CancelOrdersByClientID(ctx context.Context, requests []CancelByCloidRequest) (json.RawMessage, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("hyperliquid: no cancel requests supplied")
	}
	cancels := make([]map[string]any, len(requests))
	for i := range requests {
		if requests[i].ClientOrderID == "" {
			return nil, fmt.Errorf("hyperliquid: cancel request missing client order ID")
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
func (e *Exchange) ScheduleCancel(ctx context.Context, scheduledTime *uint64) (json.RawMessage, error) {
	action := map[string]any{
		"type": "scheduleCancel",
	}
	if scheduledTime != nil {
		action["time"] = *scheduledTime
	}
	return e.executeL1Action(ctx, action, true)
}

// UpdateLeverage updates leverage for the given asset.
func (e *Exchange) UpdateLeverage(ctx context.Context, coin string, leverage int64, isCross bool) (json.RawMessage, error) {
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
func (e *Exchange) UpdateIsolatedMargin(ctx context.Context, coin string, amount float64, isBuy bool) (json.RawMessage, error) {
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
func (e *Exchange) SetReferrer(ctx context.Context, req *SetReferrerRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: set referrer request must not be nil")
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, fmt.Errorf("hyperliquid: referrer code required")
	}
	action := map[string]any{
		"type": "setReferrer",
		"code": code,
	}
	return e.executeL1Action(ctx, action, false)
}

// CreateSubAccount creates a named sub-account for the authenticated user.
func (e *Exchange) CreateSubAccount(ctx context.Context, req *CreateSubAccountRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: create sub-account request must not be nil")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("hyperliquid: sub-account name required")
	}
	action := map[string]any{
		"type": "createSubAccount",
		"name": name,
	}
	return e.executeL1Action(ctx, action, false)
}

// USDClassTransfer moves USD collateral between perp and spot contexts.
func (e *Exchange) USDClassTransfer(ctx context.Context, req *USDClassTransferRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: usd class transfer request must not be nil")
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
	return e.executeUserSignedAction(ctx, action, signUSDClassTransferAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, nil, &empty, true)
}

// SendAsset transfers assets between DEX contexts or sub-accounts.
func (e *Exchange) SendAsset(ctx context.Context, req *SendAssetRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: send asset request must not be nil")
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	destination := strings.ToLower(strings.TrimSpace(req.Destination))
	if destination == "" {
		return nil, fmt.Errorf("hyperliquid: destination required")
	}
	source := strings.TrimSpace(req.SourceDEX)
	if source == "" {
		return nil, fmt.Errorf("hyperliquid: source dex required")
	}
	destinationDex := strings.TrimSpace(req.DestinationDEX)
	if destinationDex == "" {
		return nil, fmt.Errorf("hyperliquid: destination dex required")
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return nil, fmt.Errorf("hyperliquid: token required")
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
	return e.executeUserSignedAction(ctx, action, signSendAssetAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, nil, &empty, true)
}

// SubAccountTransfer moves USD between a sub-account and the primary account.
func (e *Exchange) SubAccountTransfer(ctx context.Context, req *SubAccountTransferRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: sub-account transfer request must not be nil")
	}
	user := strings.ToLower(strings.TrimSpace(req.SubAccountUser))
	if user == "" {
		return nil, fmt.Errorf("hyperliquid: sub-account user required")
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
func (e *Exchange) SubAccountSpotTransfer(ctx context.Context, req *SubAccountSpotTransferRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: sub-account spot transfer request must not be nil")
	}
	user := strings.ToLower(strings.TrimSpace(req.SubAccountUser))
	if user == "" {
		return nil, fmt.Errorf("hyperliquid: sub-account user required")
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return nil, fmt.Errorf("hyperliquid: token required")
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
func (e *Exchange) VaultUSDTransfer(ctx context.Context, req *VaultUSDTransferRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: vault transfer request must not be nil")
	}
	vaultAddress := strings.ToLower(strings.TrimSpace(req.VaultAddress))
	if vaultAddress == "" {
		return nil, fmt.Errorf("hyperliquid: vault address required")
	}
	action := map[string]any{
		"type":         "vaultTransfer",
		"vaultAddress": vaultAddress,
		"isDeposit":    req.IsDeposit,
		"usd":          req.USD,
	}
	return e.executeL1Action(ctx, action, false)
}

// USDTransfer sends USD to another address.
func (e *Exchange) USDTransfer(ctx context.Context, req *USDTransferRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: usd transfer request must not be nil")
	}
	destination := strings.ToLower(strings.TrimSpace(req.Destination))
	if destination == "" {
		return nil, fmt.Errorf("hyperliquid: destination required")
	}
	action := map[string]any{
		"type":        "usdSend",
		"destination": destination,
		"amount":      formatAmountString(req.Amount),
	}
	return e.executeUserSignedAction(ctx, action, signUSDTransferAction, func(nonce uint64) {
		action["time"] = hexOrDecimalFromUint64(nonce)
	}, nil, nil, true)
}

// SpotTransfer sends spot tokens to another address.
func (e *Exchange) SpotTransfer(ctx context.Context, req *SpotTransferRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: spot transfer request must not be nil")
	}
	destination := strings.ToLower(strings.TrimSpace(req.Destination))
	if destination == "" {
		return nil, fmt.Errorf("hyperliquid: destination required")
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return nil, fmt.Errorf("hyperliquid: token required")
	}
	action := map[string]any{
		"type":        "spotSend",
		"destination": destination,
		"token":       token,
		"amount":      formatAmountString(req.Amount),
	}
	return e.executeUserSignedAction(ctx, action, signSpotTransferAction, func(nonce uint64) {
		action["time"] = hexOrDecimalFromUint64(nonce)
	}, nil, nil, true)
}

// TokenDelegate delegates or undelegates HL tokens to a validator.
func (e *Exchange) TokenDelegate(ctx context.Context, req *TokenDelegateRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: token delegate request must not be nil")
	}
	validator := strings.ToLower(strings.TrimSpace(req.Validator))
	if validator == "" {
		return nil, fmt.Errorf("hyperliquid: validator address required")
	}
	action := map[string]any{
		"type":         "tokenDelegate",
		"validator":    validator,
		"wei":          hexOrDecimalFromUint64(req.Wei),
		"isUndelegate": req.IsUndelegate,
	}
	return e.executeUserSignedAction(ctx, action, signTokenDelegateAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, nil, nil, true)
}

// WithdrawFromBridge withdraws USD from the Hyperliquid bridge.
func (e *Exchange) WithdrawFromBridge(ctx context.Context, req *WithdrawFromBridgeRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: withdraw request must not be nil")
	}
	destination := strings.ToLower(strings.TrimSpace(req.Destination))
	if destination == "" {
		return nil, fmt.Errorf("hyperliquid: destination required")
	}
	action := map[string]any{
		"type":        "withdraw3",
		"destination": destination,
		"amount":      formatAmountString(req.Amount),
	}
	return e.executeUserSignedAction(ctx, action, signWithdrawFromBridgeAction, func(nonce uint64) {
		action["time"] = hexOrDecimalFromUint64(nonce)
	}, nil, nil, true)
}

// ApproveAgent authorises an agent key to act on behalf of the user.
func (e *Exchange) ApproveAgent(ctx context.Context, req *ApproveAgentRequest) (json.RawMessage, string, error) {
	if err := e.ensureWallet(ctx); err != nil {
		return nil, "", err
	}
	if e.expiresAfter != nil {
		return nil, "", fmt.Errorf("hyperliquid: expiresAfter not supported for this action")
	}
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, "", fmt.Errorf("hyperliquid: generate agent key: %w", err)
	}
	priv, err := ethcrypto.ToECDSA(keyBytes)
	if err != nil {
		return nil, "", fmt.Errorf("hyperliquid: construct agent key: %w", err)
	}
	agentKey := "0x" + hex.EncodeToString(keyBytes)
	agentAddress := strings.ToLower(ethcrypto.PubkeyToAddress(priv.PublicKey).Hex())
	name := ""
	if req != nil {
		name = strings.TrimSpace(req.AgentName)
	}
	action := map[string]any{
		"type":         "approveAgent",
		"agentAddress": agentAddress,
		"agentName":    name,
	}
	nonce := uint64(e.now().UnixMilli())
	action["nonce"] = hexOrDecimalFromUint64(nonce)
	signature, err := signAgentAction(e.wallet, action, e.isMainnetEndpoint())
	if err != nil {
		return nil, "", err
	}
	if name == "" {
		delete(action, "agentName")
	}
	var vault *string
	if e.vaultAddress != "" {
		addr := e.vaultAddress
		vault = &addr
	}
	resp, err := e.postSignedAction(ctx, action, signature, nonce, true, vault)
	if err != nil {
		return nil, "", err
	}
	return resp, agentKey, nil
}

// ApproveBuilderFee sets the maximum fee rate for a builder.
func (e *Exchange) ApproveBuilderFee(ctx context.Context, req *ApproveBuilderFeeRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: approve builder fee request must not be nil")
	}
	builder := strings.ToLower(strings.TrimSpace(req.Builder))
	if builder == "" {
		return nil, fmt.Errorf("hyperliquid: builder address required")
	}
	maxFee := strings.TrimSpace(req.MaxFeeRate)
	if maxFee == "" {
		return nil, fmt.Errorf("hyperliquid: max fee rate required")
	}
	action := map[string]any{
		"type":       "approveBuilderFee",
		"builder":    builder,
		"maxFeeRate": maxFee,
	}
	return e.executeUserSignedAction(ctx, action, signApproveBuilderFeeAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, nil, nil, true)
}

// ConvertToMultiSigUser converts the account to a multi-sig controlled account.
func (e *Exchange) ConvertToMultiSigUser(ctx context.Context, req *ConvertToMultiSigUserRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: convert to multi-sig request must not be nil")
	}
	if req.Threshold <= 0 {
		return nil, fmt.Errorf("hyperliquid: threshold must be positive")
	}
	if len(req.AuthorizedUsers) == 0 {
		return nil, fmt.Errorf("hyperliquid: at least one authorised user required")
	}
	authorised := make([]string, len(req.AuthorizedUsers))
	for i := range req.AuthorizedUsers {
		user := strings.ToLower(strings.TrimSpace(req.AuthorizedUsers[i]))
		if user == "" {
			return nil, fmt.Errorf("hyperliquid: authorised user entries must not be empty")
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
	return e.executeUserSignedAction(ctx, action, signConvertToMultiSigUserAction, func(nonce uint64) {
		action["nonce"] = hexOrDecimalFromUint64(nonce)
	}, nil, nil, true)
}

// SpotDeployRegisterToken registers a new spot token specification.
func (e *Exchange) SpotDeployRegisterToken(ctx context.Context, req *SpotDeployRegisterTokenRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: spot deploy register token request must not be nil")
	}
	name := strings.TrimSpace(req.TokenName)
	if name == "" {
		return nil, fmt.Errorf("hyperliquid: token name required")
	}
	fullName := strings.TrimSpace(req.FullName)
	if fullName == "" {
		return nil, fmt.Errorf("hyperliquid: token full name required")
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
func (e *Exchange) SpotDeployUserGenesis(ctx context.Context, req *SpotDeployUserGenesisRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: spot deploy user genesis request must not be nil")
	}
	users := make([][]any, len(req.UserAndWei))
	for i := range req.UserAndWei {
		user := strings.ToLower(strings.TrimSpace(req.UserAndWei[i].User))
		if user == "" {
			return nil, fmt.Errorf("hyperliquid: user genesis entry must include user")
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
func (e *Exchange) SpotDeployEnableFreezePrivilege(ctx context.Context, token int) (json.RawMessage, error) {
	return e.spotDeployTokenAction(ctx, "enableFreezePrivilege", token)
}

// SpotDeployFreezeUser freezes or unfreezes a specific user for a spot token.
func (e *Exchange) SpotDeployFreezeUser(ctx context.Context, req *SpotDeployFreezeUserRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: spot deploy freeze user request must not be nil")
	}
	user := strings.ToLower(strings.TrimSpace(req.User))
	if user == "" {
		return nil, fmt.Errorf("hyperliquid: user address required")
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
func (e *Exchange) SpotDeployRevokeFreezePrivilege(ctx context.Context, token int) (json.RawMessage, error) {
	return e.spotDeployTokenAction(ctx, "revokeFreezePrivilege", token)
}

// SpotDeployEnableQuoteToken marks a token as a valid quote token.
func (e *Exchange) SpotDeployEnableQuoteToken(ctx context.Context, token int) (json.RawMessage, error) {
	return e.spotDeployTokenAction(ctx, "enableQuoteToken", token)
}

// SpotDeployGenesis finalises the genesis configuration for a spot token.
func (e *Exchange) SpotDeployGenesis(ctx context.Context, req *SpotDeployGenesisRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: spot deploy genesis request must not be nil")
	}
	maxSupply := strings.TrimSpace(req.MaxSupply)
	if maxSupply == "" {
		return nil, fmt.Errorf("hyperliquid: max supply required")
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
func (e *Exchange) SpotDeployRegisterSpot(ctx context.Context, req *SpotDeployRegisterSpotRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: spot deploy register spot request must not be nil")
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
func (e *Exchange) SpotDeployRegisterHyperliquidity(ctx context.Context, req *SpotDeployRegisterHyperliquidityRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: spot deploy hyperliquidity request must not be nil")
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
		body["registerHyperliquidity"].(map[string]any)["nSeededLevels"] = *req.SeededLevels
	}
	return e.executeSpotDeploy(ctx, body)
}

// SpotDeploySetDeployerTradingFeeShare adjusts the deployer's fee share for a token.
func (e *Exchange) SpotDeploySetDeployerTradingFeeShare(ctx context.Context, req *SpotDeploySetDeployerTradingFeeShareRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: spot deploy trading fee share request must not be nil")
	}
	share := strings.TrimSpace(req.Share)
	if share == "" {
		return nil, fmt.Errorf("hyperliquid: trading fee share required")
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
func (e *Exchange) PerpDeployRegisterAsset(ctx context.Context, req *PerpDeployRegisterAssetRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: perp deploy register asset request must not be nil")
	}
	dex := strings.TrimSpace(req.Dex)
	if dex == "" {
		return nil, fmt.Errorf("hyperliquid: dex identifier required")
	}
	coin := strings.TrimSpace(req.Coin)
	if coin == "" {
		return nil, fmt.Errorf("hyperliquid: coin required")
	}
	oraclePx := strings.TrimSpace(req.OraclePrice)
	if oraclePx == "" {
		return nil, fmt.Errorf("hyperliquid: oracle price required")
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
			"fullName":        strings.TrimSpace(req.Schema.FullName),
			"collateralToken": strings.TrimSpace(req.Schema.CollateralToken),
		}
		if req.Schema.OracleUpdater != nil {
			schema["oracleUpdater"] = strings.ToLower(strings.TrimSpace(*req.Schema.OracleUpdater))
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
func (e *Exchange) PerpDeploySetOracle(ctx context.Context, req *PerpDeploySetOracleRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: perp deploy set oracle request must not be nil")
	}
	dex := strings.TrimSpace(req.Dex)
	if dex == "" {
		return nil, fmt.Errorf("hyperliquid: dex identifier required")
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
func (e *Exchange) CSignerUnjailSelf(ctx context.Context) (json.RawMessage, error) {
	return e.executeCSignerAction(ctx, "unjailSelf")
}

// CSignerJailSelf requests to jail the connected signer.
func (e *Exchange) CSignerJailSelf(ctx context.Context) (json.RawMessage, error) {
	return e.executeCSignerAction(ctx, "jailSelf")
}

// CValidatorRegister registers a validator profile.
func (e *Exchange) CValidatorRegister(ctx context.Context, req *CValidatorRegisterRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: validator register request must not be nil")
	}
	nodeIP := strings.TrimSpace(req.NodeIP)
	if nodeIP == "" {
		return nil, fmt.Errorf("hyperliquid: node IP required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("hyperliquid: validator name required")
	}
	description := strings.TrimSpace(req.Description)
	signer := strings.ToLower(strings.TrimSpace(req.Signer))
	if signer == "" {
		return nil, fmt.Errorf("hyperliquid: signer address required")
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
func (e *Exchange) CValidatorChangeProfile(ctx context.Context, req *CValidatorChangeProfileRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: validator change profile request must not be nil")
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
		node := strings.TrimSpace(*req.NodeIP)
		if node != "" {
			profile["node_ip"] = map[string]any{"Ip": node}
		}
	}
	if req.Name != nil {
		profile["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		profile["description"] = strings.TrimSpace(*req.Description)
	}
	if req.Signer != nil {
		signer := strings.ToLower(strings.TrimSpace(*req.Signer))
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
func (e *Exchange) CValidatorUnregister(ctx context.Context) (json.RawMessage, error) {
	body := map[string]any{
		"unregister": nil,
	}
	return e.executeCValidatorAction(ctx, body)
}

// MultiSig submits an action signed by multiple parties.
func (e *Exchange) MultiSig(ctx context.Context, req *MultiSigRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: multi-sig request must not be nil")
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	if req.Action == nil {
		return nil, fmt.Errorf("hyperliquid: multi-sig inner action required")
	}
	if len(req.Signatures) == 0 {
		return nil, fmt.Errorf("hyperliquid: at least one signature required")
	}
	if req.Nonce == 0 {
		return nil, fmt.Errorf("hyperliquid: nonce required")
	}
	multiSigUser := strings.ToLower(strings.TrimSpace(req.MultiSigUser))
	if multiSigUser == "" {
		return nil, fmt.Errorf("hyperliquid: multi-sig user required")
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
		addr := strings.ToLower(strings.TrimSpace(*req.VaultAddress))
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
	return e.postSignedAction(ctx, action, signature, req.Nonce, true, vault)
}

// UseBigBlocks toggles execution against big blocks.
func (e *Exchange) UseBigBlocks(ctx context.Context, req *UseBigBlocksRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: use big blocks request must not be nil")
	}
	action := map[string]any{
		"type":           "evmUserModify",
		"usingBigBlocks": req.Enable,
	}
	return e.executeL1Action(ctx, action, false)
}

// AgentEnableDexAbstraction enables DEX abstraction for the current agent.
func (e *Exchange) AgentEnableDexAbstraction(ctx context.Context) (json.RawMessage, error) {
	action := map[string]any{
		"type": "agentEnableDexAbstraction",
	}
	return e.executeL1Action(ctx, action, true)
}

// UserDexAbstraction toggles DEX abstraction for a user.
func (e *Exchange) UserDexAbstraction(ctx context.Context, req *UserDexAbstractionRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: user dex abstraction request must not be nil")
	}
	user := strings.ToLower(strings.TrimSpace(req.User))
	if user == "" {
		return nil, fmt.Errorf("hyperliquid: user address required")
	}
	if err := e.ensureWallet(ctx); err != nil {
		return nil, err
	}
	if e.expiresAfter != nil {
		return nil, fmt.Errorf("hyperliquid: expiresAfter not supported for this action")
	}
	nonce := uint64(e.now().UnixMilli())
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
	return e.postSignedAction(ctx, message, signature, nonce, true, vault)
}

// Noop submits a no-op action with the provided nonce.
func (e *Exchange) Noop(ctx context.Context, req *NoopRequest) (json.RawMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperliquid: noop request must not be nil")
	}
	if req.Nonce == 0 {
		return nil, fmt.Errorf("hyperliquid: nonce required")
	}
	action := map[string]any{
		"type": "noop",
	}
	return e.executeL1ActionWithNonce(ctx, action, true, &req.Nonce)
}

// SubmitOrderSubmit integrates with the existing exchange wrapper submit pathway.
