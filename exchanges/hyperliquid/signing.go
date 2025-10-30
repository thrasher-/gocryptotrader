package hyperliquid

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	ethmath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/shopspring/decimal"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	hyperliquidMainnetChain = "Mainnet"
	hyperliquidTestnetChain = "Testnet"
	defaultSignatureChainID = "0x66eee"
)

type wallet struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
}

func newWalletFromHex(hexKey string) (*wallet, error) {
	key := strings.TrimPrefix(hexKey, "0x")
	if key == "" {
		return nil, errPrivateKeyNotProvided
	}
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("invalid private key length %d", len(keyBytes))
	}
	priv, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("construct private key: %w", err)
	}
	return &wallet{
		privateKey: priv,
		address:    crypto.PubkeyToAddress(priv.PublicKey),
	}, nil
}

func (w *wallet) hexAddress() string {
	return strings.ToLower(w.address.Hex())
}

func (w *wallet) signTypedData(td *apitypes.TypedData) (map[string]any, error) {
	if td == nil {
		return nil, errTypedDataPayloadMissing
	}
	hash, _, err := apitypes.TypedDataAndHash(*td)
	if err != nil {
		return nil, fmt.Errorf("typed data hash: %w", err)
	}
	sig, err := crypto.Sign(hash, w.privateKey)
	if err != nil {
		return nil, fmt.Errorf("sign typed data: %w", err)
	}
	v := int(sig[64]) + 27
	r := "0x" + hex.EncodeToString(sig[:32])
	s := "0x" + hex.EncodeToString(sig[32:64])
	return map[string]any{
		"r": r,
		"s": s,
		"v": v,
	}, nil
}

func floatToWire(x float64) (string, error) {
	rounded := fmt.Sprintf("%.8f", x)
	parsed, err := strconv.ParseFloat(rounded, 64)
	if err != nil {
		return "", err
	}
	if math.Abs(parsed-x) >= 1e-12 {
		return "", fmt.Errorf("float_to_wire causes rounding: %f", x)
	}
	if strings.HasPrefix(rounded, "-0") {
		parsed = 0
	}
	return decimal.NewFromFloat(parsed).String(), nil
}

func floatToInt(x float64, power int) (int64, error) {
	scale := math.Pow10(power)
	withDecimals := x * scale
	rounded := math.Round(withDecimals)
	if math.Abs(rounded-withDecimals) >= 1e-3 {
		return 0, fmt.Errorf("float_to_int causes rounding: %f", x)
	}
	return int64(rounded), nil
}

func floatToUSDInt(x float64) (int64, error) {
	return floatToInt(x, 6)
}

func orderTypeToWire(o OrderType) (map[string]any, error) {
	if o.Limit != nil {
		return map[string]any{"limit": map[string]any{"tif": o.Limit.TimeInForce}}, nil
	}
	if o.Trigger != nil {
		triggerPx, err := floatToWire(o.Trigger.TriggerPrice)
		if err != nil {
			return nil, err
		}
		return map[string]any{"trigger": map[string]any{
			"isMarket":  o.Trigger.IsMarket,
			"triggerPx": triggerPx,
			"tpsl":      o.Trigger.TPSL,
		}}, nil
	}
	return nil, errInvalidOrderType
}

func orderRequestToOrderWire(req *OrderRequest, asset int64) (map[string]any, error) {
	if req == nil {
		return nil, errOrderRequestMissing
	}
	limitPx, err := floatToWire(req.LimitPrice)
	if err != nil {
		return nil, err
	}
	size, err := floatToWire(req.Size)
	if err != nil {
		return nil, err
	}
	wireType, err := orderTypeToWire(req.OrderType)
	if err != nil {
		return nil, err
	}
	wire := map[string]any{
		"a": asset,
		"b": req.IsBuy,
		"p": limitPx,
		"s": size,
		"r": req.ReduceOnly,
		"t": wireType,
	}
	if req.ClientOrderID != "" {
		wire["c"] = req.ClientOrderID
	}
	return wire, nil
}

func orderWiresToOrderAction(orderWires []map[string]any, builder *BuilderInfo) map[string]any {
	action := map[string]any{
		"type":     "order",
		"orders":   orderWires,
		"grouping": "na",
	}
	if builder != nil {
		action["builder"] = map[string]any{
			"b": strings.ToLower(builder.Address),
			"f": builder.Fee,
		}
	}
	return action
}

func msgpackMarshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.SetSortMapKeys(true)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func addressToBytes(addr string) ([]byte, error) {
	trimmed := strings.TrimPrefix(strings.ToLower(addr), "0x")
	return hex.DecodeString(trimmed)
}

func actionHash(action map[string]any, vaultAddress *string, nonce uint64, expiresAfter *uint64) ([]byte, error) {
	encoded, err := msgpackMarshal(action)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(encoded)
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, nonce)
	buf.Write(nonceBytes)
	if vaultAddress == nil || *vaultAddress == "" {
		buf.WriteByte(0x00)
	} else {
		buf.WriteByte(0x01)
		addrBytes, err := addressToBytes(*vaultAddress)
		if err != nil {
			return nil, fmt.Errorf("vault address: %w", err)
		}
		buf.Write(addrBytes)
	}
	if expiresAfter != nil {
		buf.WriteByte(0x00)
		expBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(expBytes, *expiresAfter)
		buf.Write(expBytes)
	}
	return crypto.Keccak256(buf.Bytes()), nil
}

func constructPhantomAgent(hash []byte, isMainnet bool) map[string]any {
	source := "b"
	if isMainnet {
		source = "a"
	}
	return map[string]any{
		"source":       source,
		"connectionId": hash,
	}
}

func l1Payload(agent map[string]any) apitypes.TypedData {
	return apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Agent": {
				{Name: "source", Type: "string"},
				{Name: "connectionId", Type: "bytes32"},
			},
		},
		PrimaryType: "Agent",
		Domain: apitypes.TypedDataDomain{
			Name:              "Exchange",
			Version:           "1",
			ChainId:           ethmath.NewHexOrDecimal256(1337),
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
		Message: agent,
	}
}

func signL1Action(w *wallet, action map[string]any, vaultAddress *string, nonce uint64, expiresAfter *uint64, isMainnet bool) (map[string]any, error) {
	hash, err := actionHash(action, vaultAddress, nonce, expiresAfter)
	if err != nil {
		return nil, err
	}
	payload := l1Payload(constructPhantomAgent(hash, isMainnet))
	return w.signTypedData(&payload)
}

var (
	eip712Domain = apitypes.Types{
		"EIP712Domain": {
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
	}
	usdSendTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "destination", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "time", Type: "uint64"},
	}
	spotSendTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "destination", Type: "string"},
		{Name: "token", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "time", Type: "uint64"},
	}
	usdClassTransferTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "toPerp", Type: "bool"},
		{Name: "nonce", Type: "uint64"},
	}
	sendAssetTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "destination", Type: "string"},
		{Name: "sourceDex", Type: "string"},
		{Name: "destinationDex", Type: "string"},
		{Name: "token", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "fromSubAccount", Type: "string"},
		{Name: "nonce", Type: "uint64"},
	}
	tokenDelegateTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "validator", Type: "address"},
		{Name: "wei", Type: "uint64"},
		{Name: "isUndelegate", Type: "bool"},
		{Name: "nonce", Type: "uint64"},
	}
	withdrawTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "destination", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "time", Type: "uint64"},
	}
	approveAgentTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "agentAddress", Type: "address"},
		{Name: "agentName", Type: "string"},
		{Name: "nonce", Type: "uint64"},
	}
	approveBuilderFeeTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "maxFeeRate", Type: "string"},
		{Name: "builder", Type: "address"},
		{Name: "nonce", Type: "uint64"},
	}
	convertMultiSigTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "signers", Type: "string"},
		{Name: "nonce", Type: "uint64"},
	}
	multiSigEnvelopeTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "multiSigActionHash", Type: "bytes32"},
		{Name: "nonce", Type: "uint64"},
	}
	userDexAbstractionTypes = []apitypes.Type{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "user", Type: "address"},
		{Name: "enabled", Type: "bool"},
		{Name: "nonce", Type: "uint64"},
	}
)

func hexOrDecimalFromUint64(v uint64) *ethmath.HexOrDecimal256 {
	if v > math.MaxInt64 {
		return ethmath.NewHexOrDecimal256(math.MaxInt64)
	}
	return ethmath.NewHexOrDecimal256(int64(v))
}

func signUserAction(w *wallet, action map[string]any, fields []apitypes.Type, primaryType string, isMainnet bool) (map[string]any, error) {
	chain := hyperliquidTestnetChain
	if isMainnet {
		chain = hyperliquidMainnetChain
	}
	message := make(map[string]any, len(action)+1)
	maps.Copy(message, action)
	delete(message, "type")
	delete(message, "signatureChainId")
	message["hyperliquidChain"] = chain

	types := apitypes.Types{
		primaryType: fields,
	}
	maps.Copy(types, eip712Domain)

	td := apitypes.TypedData{
		Types:       types,
		PrimaryType: primaryType,
		Domain: apitypes.TypedDataDomain{
			Name:              "HyperliquidSignTransaction",
			Version:           "1",
			ChainId:           hexOrDecimalFromUint64(0x66eee),
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
		Message: message,
	}
	sig, err := w.signTypedData(&td)
	if err != nil {
		return nil, err
	}
	action["hyperliquidChain"] = chain
	action["signatureChainId"] = defaultSignatureChainID
	return sig, nil
}

func signUSDTransferAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, usdSendTypes, "HyperliquidTransaction:UsdSend", isMainnet)
}

func signSpotTransferAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, spotSendTypes, "HyperliquidTransaction:SpotSend", isMainnet)
}

func signUSDClassTransferAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, usdClassTransferTypes, "HyperliquidTransaction:UsdClassTransfer", isMainnet)
}

func signSendAssetAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, sendAssetTypes, "HyperliquidTransaction:SendAsset", isMainnet)
}

func signTokenDelegateAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, tokenDelegateTypes, "HyperliquidTransaction:TokenDelegate", isMainnet)
}

func signWithdrawFromBridgeAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, withdrawTypes, "HyperliquidTransaction:Withdraw", isMainnet)
}

func signAgentAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, approveAgentTypes, "HyperliquidTransaction:ApproveAgent", isMainnet)
}

func signApproveBuilderFeeAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, approveBuilderFeeTypes, "HyperliquidTransaction:ApproveBuilderFee", isMainnet)
}

func signConvertToMultiSigUserAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, convertMultiSigTypes, "HyperliquidTransaction:ConvertToMultiSigUser", isMainnet)
}

func signUserDexAbstractionAction(w *wallet, action map[string]any, isMainnet bool) (map[string]any, error) {
	return signUserAction(w, action, userDexAbstractionTypes, "HyperliquidTransaction:UserDexAbstraction", isMainnet)
}

func signMultiSigAction(w *wallet, action map[string]any, isMainnet bool, vaultAddress *string, nonce uint64, expiresAfter *uint64) (map[string]any, error) {
	actionCopy := make(map[string]any, len(action))
	for k, v := range action {
		if k == "type" {
			continue
		}
		actionCopy[k] = v
	}
	hash, err := actionHash(actionCopy, vaultAddress, nonce, expiresAfter)
	if err != nil {
		return nil, err
	}
	envelope := map[string]any{
		"multiSigActionHash": hash,
		"nonce":              hexOrDecimalFromUint64(nonce),
	}
	return signUserAction(w, envelope, multiSigEnvelopeTypes, "HyperliquidTransaction:SendMultiSig", isMainnet)
}
