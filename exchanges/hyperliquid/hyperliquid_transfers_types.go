package hyperliquid

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// ClassTransferRequest selects the destination balance for a USDC transfer.
type ClassTransferRequest struct {
	Amount      float64
	ToPerpetual bool
}

// USDCTransferRequest specifies a USDC recipient and amount for Core or bridge transfers.
type USDCTransferRequest struct {
	Destination string
	Amount      float64
}

// SpotTransferRequest identifies a spot token by its exact NAME:TOKEN_ID identifier.
type SpotTransferRequest struct {
	Destination string
	Token       string
	Amount      float64
}

// UserLedgerRequest selects an account and an inclusive non-funding ledger range.
type UserLedgerRequest struct {
	User      string
	StartTime time.Time
	EndTime   time.Time
}

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
	Type            string        `json:"type"`
	USDC            types.Number  `json:"usdc"`
	Fee             types.Number  `json:"fee"`
	Amount          types.Number  `json:"amount"`
	USDCValue       types.Number  `json:"usdcValue"`
	NativeTokenFee  types.Number  `json:"nativeTokenFee"`
	RequestedUSD    types.Number  `json:"requestedUsd"`
	NetWithdrawnUSD types.Number  `json:"netWithdrawnUsd"`
	Nonce           uint64        `json:"nonce"`
	User            string        `json:"user"`
	Destination     string        `json:"destination"`
	SourceDEX       string        `json:"sourceDex"`
	DestinationDEX  string        `json:"destinationDex"`
	Token           string        `json:"token"`
	FeeToken        currency.Code `json:"feeToken"`
	Vault           string        `json:"vault"`
	ToPerp          bool          `json:"toPerp"`
}
