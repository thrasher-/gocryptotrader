package hyperliquid

import (
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
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
