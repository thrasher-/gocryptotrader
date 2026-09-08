package hyperliquid

import (
	"github.com/thrasher-corp/gocryptotrader/types"
)

// SendAssetRequest contains one generalised Hyperliquid Core asset transfer.
type SendAssetRequest struct {
	Destination    string
	SourceDEX      string
	DestinationDEX string
	Token          string
	Amount         float64
}

// UserLedgerUpdate contains one non-funding account ledger update.
type UserLedgerUpdate struct {
	Delta UserLedgerDelta `json:"delta"`
	Hash  string          `json:"hash"`
	Time  types.Time      `json:"time"`
}

// UserLedgerDelta contains common fields across Hyperliquid's account ledger
// update variants. Type identifies which fields are populated.
type UserLedgerDelta struct {
	Type            string       `json:"type"`
	USDC            types.Number `json:"usdc"`
	Fee             types.Number `json:"fee"`
	Amount          types.Number `json:"amount"`
	USDCValue       types.Number `json:"usdcValue"`
	NativeTokenFee  types.Number `json:"nativeTokenFee"`
	RequestedUSD    types.Number `json:"requestedUsd"`
	NetWithdrawnUSD types.Number `json:"netWithdrawnUsd"`
	Nonce           uint64       `json:"nonce"`
	User            string       `json:"user"`
	Destination     string       `json:"destination"`
	SourceDEX       string       `json:"sourceDex"`
	DestinationDEX  string       `json:"destinationDex"`
	Token           string       `json:"token"`
	FeeToken        string       `json:"feeToken"`
	Vault           string       `json:"vault"`
	ToPerp          bool         `json:"toPerp"`
}
