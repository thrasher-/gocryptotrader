package hyperliquid

import (
	"fmt"
	"strconv"
	"strings"

	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

// Exchange status values for Hyperliquid order responses.
const (
	ExchangeStatusSuccess = "success"
	ExchangeStatusResting = "resting"
	ExchangeStatusError   = "error"
	ExchangeStatusFilled  = "filled"
)

// ExchangeResponse encapsulates the standard exchange endpoint response payload.
type ExchangeResponse struct {
	Status   string                `json:"status"`
	TxHash   string                `json:"txHash"`
	Response *ExchangeResponseBody `json:"response,omitempty"`
	Extras   map[string]any        `json:"-"`
}

// UnmarshalJSON decodes the response and preserves unknown fields.
func (r *ExchangeResponse) UnmarshalJSON(data []byte) error {
	type alias ExchangeResponse
	var base alias
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	delete(raw, "status")
	delete(raw, "txHash")
	delete(raw, "response")
	extra := make(map[string]any, len(raw))
	for key, value := range raw {
		if len(value) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(value, &v); err != nil {
			extra[key] = string(value)
			continue
		}
		extra[key] = v
	}
	*r = ExchangeResponse(base)
	r.Extras = extra
	return nil
}

// ExchangeResponseBody captures the nested response payload.
type ExchangeResponseBody struct {
	Type string               `json:"type"`
	Data ExchangeResponseData `json:"data"`
}

// ExchangeResponseData holds order statuses for order related responses.
type ExchangeResponseData struct {
	Statuses []ExchangeStatusEntry `json:"statuses"`
}

// ExchangeStatusEntry represents a single outcome entry.
type ExchangeStatusEntry struct {
	Kind    string              `json:"-"`
	Text    string              `json:"-"`
	Success bool                `json:"success,omitempty"`
	Resting *ExchangeOrderState `json:"resting,omitempty"`
	Error   string              `json:"error,omitempty"`
}

// UnmarshalJSON decodes flexible status payloads (string or object).
func (e *ExchangeStatusEntry) UnmarshalJSON(data []byte) error {
	strValue := string(data)
	if strValue == "" {
		return nil
	}
	if strValue[0] == '"' {
		var status string
		if err := json.Unmarshal(data, &status); err != nil {
			return err
		}
		e.Kind = strings.ToLower(status)
		e.Text = status
		e.Success = strings.EqualFold(status, ExchangeStatusSuccess)
		if e.Success {
			e.Kind = ExchangeStatusSuccess
		}
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, value := range raw {
		switch strings.ToLower(key) {
		case ExchangeStatusFilled:
			e.Kind = ExchangeStatusFilled
			e.Text = key
			e.Resting = new(ExchangeOrderState)
			if err := json.Unmarshal(value, e.Resting); err != nil {
				return err
			}
		case ExchangeStatusResting:
			e.Kind = ExchangeStatusResting
			e.Text = key
			e.Resting = new(ExchangeOrderState)
			if err := json.Unmarshal(value, e.Resting); err != nil {
				return err
			}
		case ExchangeStatusError:
			e.Kind = ExchangeStatusError
			e.Text = key
			if err := json.Unmarshal(value, &e.Error); err != nil {
				return err
			}
		case ExchangeStatusSuccess:
			e.Kind = ExchangeStatusSuccess
			e.Text = key
			if err := json.Unmarshal(value, &e.Success); err != nil {
				return err
			}
		default:
			var boolVal bool
			if err := json.Unmarshal(value, &boolVal); err == nil {
				e.Kind = strings.ToLower(key)
				e.Text = key
				e.Success = boolVal
				continue
			}
			var resting ExchangeOrderState
			if err := json.Unmarshal(value, &resting); err == nil && resting.OrderID != 0 {
				e.Kind = strings.ToLower(key)
				e.Text = key
				e.Resting = &resting
			}
		}
	}
	return nil
}

// ExchangeOrderState contains the resting order metadata.
type ExchangeOrderState struct {
	OrderID int64 `json:"oid"`
}

// ExtractOrderStatus extracts the primary order id and status from a response.
func (r *ExchangeResponse) ExtractOrderStatus() (string, order.Status, error, error) {
	if r == nil || r.Response == nil {
		return "", order.UnknownStatus, nil, errResponseMissing
	}
	statuses := r.Response.Data.Statuses
	if len(statuses) == 0 {
		return "", order.UnknownStatus, nil, errResponseStatusesEmpty
	}
	var (
		resting    *ExchangeOrderState
		hasSuccess bool
		subErr     error
	)
	for i := range statuses {
		entry := statuses[i]
		switch entry.Kind {
		case ExchangeStatusResting:
			resting = entry.Resting
		case ExchangeStatusSuccess:
			hasSuccess = hasSuccess || entry.Success || entry.Kind == ExchangeStatusSuccess
		case ExchangeStatusError:
			if entry.Error != "" {
				subErr = fmt.Errorf("%w: %s", errExchangeStatusEntryError, entry.Error)
			}
		}
	}
	var orderID string
	if resting != nil && resting.OrderID != 0 {
		orderID = strconv.FormatInt(resting.OrderID, 10)
	}
	status := order.UnknownStatus
	switch {
	case subErr != nil:
		status = order.UnknownStatus
	case resting != nil && hasSuccess:
		status = order.Active
	case hasSuccess:
		status = order.Filled
	}
	return orderID, status, subErr, nil
}

// SpotDeployAddressWei represents an address and balance tuple for spot deploy actions.
type SpotDeployAddressWei struct {
	Address string
	Wei     string
}

// MarshalJSON encodes the tuple representation expected by Hyperliquid.
func (s SpotDeployAddressWei) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{s.Address, s.Wei})
}

// SendAssetAction represents the typed payload required for send asset user actions.
type SendAssetAction struct {
	Type           string `json:"type"`
	Destination    string `json:"destination"`
	SourceDEX      string `json:"sourceDex"`
	DestinationDEX string `json:"destinationDex"`
	Token          string `json:"token"`
	Amount         string `json:"amount"`
	FromSubAccount string `json:"fromSubAccount"`
}

// LimitOrderType describes limit order configuration details.
type LimitOrderType struct {
	TimeInForce string
}

// TriggerOrderType captures trigger order settings.
type TriggerOrderType struct {
	TriggerPrice float64
	IsMarket     bool
	TPSL         string
}

// OrderType specifies either limit or trigger order details.
type OrderType struct {
	Limit   *LimitOrderType
	Trigger *TriggerOrderType
}

// OrderRequest represents the payload for placing an order.
type OrderRequest struct {
	Coin          string
	IsBuy         bool
	Size          float64
	LimitPrice    float64
	OrderType     OrderType
	ReduceOnly    bool
	ClientOrderID string
}

// BuilderInfo carries optional builder address information.
type BuilderInfo struct {
	Address string
	Fee     int
}

// ModifyIdentifier identifies an order to amend.
type ModifyIdentifier struct {
	OrderID       *int64
	ClientOrderID string
}

// ModifyRequest describes an order modification payload.
type ModifyRequest struct {
	Identifier ModifyIdentifier
	Order      OrderRequest
}

// CancelRequest references an order to cancel by ID.
type CancelRequest struct {
	Coin    string
	OrderID *int64
}

// CancelByCloidRequest references an order to cancel by client ID.
type CancelByCloidRequest struct {
	Coin          string
	ClientOrderID string
}

// SetReferrerRequest sets a referral code on the account.
type SetReferrerRequest struct {
	Code string
}

// CreateSubAccountRequest creates a new sub-account.
type CreateSubAccountRequest struct {
	Name string
}

// USDClassTransferRequest transfers funds between perp and spot.
type USDClassTransferRequest struct {
	Amount float64
	ToPerp bool
}

// SendAssetRequest moves assets between chains/dexes.
type SendAssetRequest struct {
	Destination    string
	SourceDEX      string
	DestinationDEX string
	Token          string
	Amount         float64
}

// SubAccountTransferRequest transfers USDC to or from a sub-account.
type SubAccountTransferRequest struct {
	SubAccountUser string
	IsDeposit      bool
	USD            int64
}

// SubAccountSpotTransferRequest transfers spot tokens with a sub-account.
type SubAccountSpotTransferRequest struct {
	SubAccountUser string
	IsDeposit      bool
	Token          string
	Amount         float64
}

// VaultUSDTransferRequest records transfers with a vault address.
type VaultUSDTransferRequest struct {
	VaultAddress string
	IsDeposit    bool
	USD          int64
}

// USDTransferRequest sends USDC to another address.
type USDTransferRequest struct {
	Destination string
	Amount      float64
}

// SpotTransferRequest moves spot tokens to another address.
type SpotTransferRequest struct {
	Destination string
	Token       string
	Amount      float64
}

// TokenDelegateRequest delegates or undelegates validator staking.
type TokenDelegateRequest struct {
	Validator    string
	Wei          uint64
	IsUndelegate bool
}

// WithdrawFromBridgeRequest withdraws funds from the Hyperliquid bridge.
type WithdrawFromBridgeRequest struct {
	Destination string
	Amount      float64
}

// ApproveAgentRequest describes the desired agent to approve.
type ApproveAgentRequest struct {
	AgentName string
}

// ApproveBuilderFeeRequest sets a builder's maximum fee rate.
type ApproveBuilderFeeRequest struct {
	Builder    string
	MaxFeeRate string
}

// ConvertToMultiSigUserRequest converts a user to multi-signature control.
type ConvertToMultiSigUserRequest struct {
	AuthorizedUsers []string
	Threshold       int
}

// SpotDeployRegisterTokenRequest registers a new spot token.
type SpotDeployRegisterTokenRequest struct {
	TokenName    string
	SizeDecimals int
	WeiDecimals  int
	MaxGas       int
	FullName     string
}

// SpotDeployUserGenesisEntry maps a user to an initial balance.
type SpotDeployUserGenesisEntry struct {
	User string
	Wei  string
}

// SpotDeployExistingTokenWeiEntry references existing balances for a token.
type SpotDeployExistingTokenWeiEntry struct {
	Token int
	Wei   string
}

// SpotDeployUserGenesisRequest seeds balances for a new spot token.
type SpotDeployUserGenesisRequest struct {
	Token               int
	UserAndWei          []SpotDeployUserGenesisEntry
	ExistingTokenAndWei []SpotDeployExistingTokenWeiEntry
}

// SpotDeployFreezeUserRequest toggles freeze privileges for a user.
type SpotDeployFreezeUserRequest struct {
	Token  int
	User   string
	Freeze bool
}

// SpotDeployGenesisRequest configures token supply parameters.
type SpotDeployGenesisRequest struct {
	Token            int
	MaxSupply        string
	NoHyperliquidity bool
}

// SpotDeployRegisterSpotRequest registers a new spot trading pair.
type SpotDeployRegisterSpotRequest struct {
	BaseToken  int
	QuoteToken int
}

// SpotDeployRegisterHyperliquidityRequest defines hyperliquidity parameters.
type SpotDeployRegisterHyperliquidityRequest struct {
	Spot         int
	StartPrice   float64
	OrderSize    float64
	Orders       int
	SeededLevels *int
}

// SpotDeploySetDeployerTradingFeeShareRequest sets deployer fee share.
type SpotDeploySetDeployerTradingFeeShareRequest struct {
	Token int
	Share string
}

// PerpDeploySchema defines metadata for perp assets registered by builders.
type PerpDeploySchema struct {
	FullName        string
	CollateralToken string
	OracleUpdater   *string
}

// PerpDeployRegisterAssetRequest registers a new perpetual asset.
type PerpDeployRegisterAssetRequest struct {
	Dex           string
	MaxGas        *int
	Coin          string
	SizeDecimals  int
	OraclePrice   string
	MarginTableID int
	OnlyIsolated  bool
	Schema        *PerpDeploySchema
}

// PerpDeploySetOracleRequest updates oracle pricing configuration.
type PerpDeploySetOracleRequest struct {
	Dex                string
	OraclePrices       map[string]string
	MarkPrices         []map[string]string
	ExternalPerpPrices map[string]string
}

// CValidatorRegisterRequest registers validator metadata.
type CValidatorRegisterRequest struct {
	NodeIP              string
	Name                string
	Description         string
	DelegationsDisabled bool
	CommissionBPS       int
	Signer              string
	Unjailed            bool
	InitialWei          uint64
}

// CValidatorChangeProfileRequest updates validator profile fields.
type CValidatorChangeProfileRequest struct {
	NodeIP             *string
	Name               *string
	Description        *string
	Unjailed           bool
	DisableDelegations *bool
	CommissionBPS      *int
	Signer             *string
}

// MultiSigSignature captures an individual signature tuple.
type MultiSigSignature struct {
	R string
	S string
	V int
}

// MultiSigRequest encapsulates a multi-sig action submission.
type MultiSigRequest struct {
	MultiSigUser string
	Action       map[string]any
	Signatures   []MultiSigSignature
	Nonce        uint64
	VaultAddress *string
}

// UseBigBlocksRequest toggles large block usage configuration.
type UseBigBlocksRequest struct {
	Enable bool
}

// UserDexAbstractionRequest enables DEX abstraction for a user.
type UserDexAbstractionRequest struct {
	User    string
	Enabled bool
}

// NoopRequest sends a no-op signed message with a nonce.
type NoopRequest struct {
	Nonce uint64
}
