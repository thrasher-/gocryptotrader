package kraken

import "errors"

var (
	errEmailRequired    = errors.New("email is required")
	errUsernameRequired = errors.New("username is required")
)

// CreateSubaccountRequest defines subaccount creation parameters.
type CreateSubaccountRequest struct {
	Username string
	Email    string
}

// AccountTransferRequest defines a transfer between a primary account and subaccount.
type AccountTransferRequest struct {
	Asset      string
	AssetClass AssetClass
	Amount     float64
	From       string
	To         string
}

// AccountTransferResponse defines an account transfer result.
type AccountTransferResponse struct {
	TransferID string `json:"transfer_id"`
	Status     string `json:"status"`
}
