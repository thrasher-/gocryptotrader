package hyperliquid

import (
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
)

type authorityValidationKey struct {
	accountAddress string
	vaultAddress   string
	signerAddress  string
	mainnet        bool
}

type exchangeActionResponse struct {
	Status   string          `json:"status"`
	Response json.RawMessage `json:"response"`
}

type userSignedActionRequest struct {
	Credentials *accounts.Credentials
	ActionType  string
	PrimaryType string
	NonceField  string
	Fields      []eip712Field
}
