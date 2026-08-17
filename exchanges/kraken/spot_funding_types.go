package kraken

import (
	"bytes"
	"errors"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var (
	errCursorConflict             = errors.New("cursor value and pagination toggle are mutually exclusive")
	errDepositResultInvalid       = errors.New("deposit result contains no deposit fields")
	errFromWalletInvalid          = errors.New("source wallet must be Spot Wallet")
	errKeyRequired                = errors.New("key is required")
	errMaximumFeeInvalid          = errors.New("maximum fee must not be negative")
	errMethodRequired             = errors.New("method is required")
	errPaginatedDepositInvalid    = errors.New("paginated deposit result contains no deposit fields")
	errPaginatedWithdrawalInvalid = errors.New("paginated withdrawal result contains no withdrawal fields")
	errReferenceIDRequired        = errors.New("reference identifier is required")
	errToWalletInvalid            = errors.New("destination wallet must be Futures Wallet")
	errWithdrawalResultInvalid    = errors.New("withdrawal result contains no withdrawal fields")
)

// Wallet identifies a Kraken wallet used in a wallet transfer.
type Wallet string

// Kraken wallet transfer endpoints.
const (
	WalletSpot    Wallet = "Spot Wallet"
	WalletFutures Wallet = "Futures Wallet"
)

// GetRecentDepositsStatusRequest defines filters for recent deposit status.
type GetRecentDepositsStatusRequest struct {
	Asset            string
	AssetClass       AssetClass
	Method           string
	Start            time.Time
	End              time.Time
	Cursor           string
	Paginate         *bool
	Limit            *uint64
	RebaseMultiplier RebaseMultiplier
}

// GetDepositMethodsRequest defines current deposit-method parameters.
type GetDepositMethodsRequest struct {
	Asset            string
	AssetClass       AssetClass
	RebaseMultiplier RebaseMultiplier
}

// DepositMethodResponse defines an available deposit method.
type DepositMethodResponse struct {
	Method           string       `json:"method"`
	Limit            any          `json:"limit"`
	Fee              types.Number `json:"fee"`
	FeePercent       types.Number `json:"fee-percentage"`
	AddressSetupFee  types.Number `json:"address-setup-fee"`
	GeneratesAddress bool         `json:"gen-address"`
	Minimum          types.Number `json:"minimum"`
}

// GetDepositAddressesRequest defines current deposit-address parameters.
type GetDepositAddressesRequest struct {
	Asset      string
	AssetClass AssetClass
	Method     string
	New        bool
	Amount     *float64
}

// DepositAddressResponse defines a current deposit address.
type DepositAddressResponse struct {
	Address    string `json:"address"`
	ExpireTime string `json:"expiretm"`
	New        bool   `json:"new"`
	Tag        string `json:"tag"`
	Memo       string `json:"memo"`
}

// GetWithdrawalInformationRequest defines withdrawal information parameters.
type GetWithdrawalInformationRequest struct {
	Asset  string
	Key    string
	Amount float64
}

// WithdrawalInformationResponse defines the calculated withdrawal amount and fees.
type WithdrawalInformationResponse struct {
	Method string       `json:"method"`
	Limit  types.Number `json:"limit"`
	Amount types.Number `json:"amount"`
	Fee    types.Number `json:"fee"`
}

// WithdrawFundsRequest defines current withdrawal parameters.
type WithdrawFundsRequest struct {
	Asset            string
	AssetClass       AssetClass
	Key              string
	Address          string
	Amount           float64
	MaximumFee       *float64
	RebaseMultiplier RebaseMultiplier
}

// WithdrawFundsResponse defines a withdrawal reference.
type WithdrawFundsResponse struct {
	ReferenceID string `json:"refid"`
}

// GetRecentWithdrawalsStatusRequest defines filters for recent withdrawal status.
type GetRecentWithdrawalsStatusRequest struct {
	Asset            string
	AssetClass       AssetClass
	Method           string
	Start            time.Time
	End              time.Time
	Cursor           string
	Paginate         *bool
	Limit            *uint64
	RebaseMultiplier RebaseMultiplier
}

// RecentWithdrawalsStatusResponse normalises Kraken's paginated and non-paginated result shapes.
type RecentWithdrawalsStatusResponse struct {
	Withdrawals []RecentWithdrawalStatus
	NextCursor  string
}

// UnmarshalJSON normalises Kraken's array, single-withdrawal, and paginated withdrawal results.
func (r *RecentWithdrawalsStatusResponse) UnmarshalJSON(data []byte) error {
	var withdrawals []RecentWithdrawalStatus
	if !bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		if err := json.Unmarshal(data, &withdrawals); err == nil {
			*r = RecentWithdrawalsStatusResponse{Withdrawals: withdrawals}
			return nil
		}
	}

	var paginated struct {
		Withdrawal json.RawMessage `json:"withdrawal"`
		NextCursor string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(data, &paginated); err == nil && len(paginated.Withdrawal) > 0 {
		response := RecentWithdrawalsStatusResponse{NextCursor: paginated.NextCursor}
		if !bytes.Equal(bytes.TrimSpace(paginated.Withdrawal), []byte("null")) {
			if err := json.Unmarshal(paginated.Withdrawal, &withdrawals); err == nil {
				response.Withdrawals = withdrawals
				*r = response
				return nil
			}
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(paginated.Withdrawal, &fields); err != nil {
			return err
		}
		if !containsWithdrawalField(fields) {
			return errPaginatedWithdrawalInvalid
		}
		var withdrawal RecentWithdrawalStatus
		if err := json.Unmarshal(paginated.Withdrawal, &withdrawal); err != nil {
			return err
		}
		response.Withdrawals = []RecentWithdrawalStatus{withdrawal}
		*r = response
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if !containsWithdrawalField(fields) {
		return errWithdrawalResultInvalid
	}
	var withdrawal RecentWithdrawalStatus
	if err := json.Unmarshal(data, &withdrawal); err != nil {
		return err
	}
	*r = RecentWithdrawalsStatusResponse{Withdrawals: []RecentWithdrawalStatus{withdrawal}}
	return nil
}

func containsWithdrawalField(fields map[string]json.RawMessage) bool {
	for key := range fields {
		switch key {
		case "method", "network", "aclass", "asset", "refid", "txid", "info", "amount", "fee", "time", "status", "status-prop", "key":
			return true
		}
	}
	return false
}

// RecentWithdrawalStatus defines one withdrawal status entry.
type RecentWithdrawalStatus struct {
	Method           string       `json:"method"`
	Network          string       `json:"network"`
	AssetClass       string       `json:"aclass"`
	Asset            string       `json:"asset"`
	ReferenceID      string       `json:"refid"`
	TransactionID    string       `json:"txid"`
	Information      string       `json:"info"`
	Amount           types.Number `json:"amount"`
	Fee              types.Number `json:"fee"`
	Time             types.Time   `json:"time"`
	Status           string       `json:"status"`
	StatusProperties string       `json:"status-prop"`
	Key              string       `json:"key"`
}

// RecentDepositsStatusResponse normalises Kraken's paginated and non-paginated result shapes.
type RecentDepositsStatusResponse struct {
	Deposits   []RecentDepositStatus
	NextCursor string
}

// UnmarshalJSON normalises Kraken's array, single-deposit, and paginated deposit results.
func (r *RecentDepositsStatusResponse) UnmarshalJSON(data []byte) error {
	var deposits []RecentDepositStatus
	if !bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		if err := json.Unmarshal(data, &deposits); err == nil {
			*r = RecentDepositsStatusResponse{Deposits: deposits}
			return nil
		}
	}

	var paginated struct {
		Deposit    json.RawMessage `json:"deposit"`
		NextCursor string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(data, &paginated); err == nil && len(paginated.Deposit) > 0 {
		response := RecentDepositsStatusResponse{NextCursor: paginated.NextCursor}
		if !bytes.Equal(bytes.TrimSpace(paginated.Deposit), []byte("null")) {
			if err := json.Unmarshal(paginated.Deposit, &deposits); err == nil {
				response.Deposits = deposits
				*r = response
				return nil
			}
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(paginated.Deposit, &fields); err != nil {
			return err
		}
		if !containsDepositField(fields) {
			return errPaginatedDepositInvalid
		}
		var deposit RecentDepositStatus
		if err := json.Unmarshal(paginated.Deposit, &deposit); err != nil {
			return err
		}
		response.Deposits = []RecentDepositStatus{deposit}
		*r = response
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if !containsDepositField(fields) {
		return errDepositResultInvalid
	}
	var deposit RecentDepositStatus
	if err := json.Unmarshal(data, &deposit); err != nil {
		return err
	}
	*r = RecentDepositsStatusResponse{Deposits: []RecentDepositStatus{deposit}}
	return nil
}

func containsDepositField(fields map[string]json.RawMessage) bool {
	for key := range fields {
		switch key {
		case "method", "aclass", "asset", "refid", "txid", "info", "amount", "fee", "time", "status", "status-prop", "originators":
			return true
		}
	}
	return false
}

// RecentDepositStatus defines one deposit status entry.
type RecentDepositStatus struct {
	Method           string       `json:"method"`
	AssetClass       string       `json:"aclass"`
	Asset            string       `json:"asset"`
	ReferenceID      string       `json:"refid"`
	TransactionID    string       `json:"txid"`
	Information      string       `json:"info"`
	Amount           types.Number `json:"amount"`
	Fee              types.Number `json:"fee"`
	Time             types.Time   `json:"time"`
	Status           string       `json:"status"`
	StatusProperties string       `json:"status-prop"`
	Originators      []string     `json:"originators"`
}

// GetWithdrawalMethodsRequest defines withdrawal method filters.
type GetWithdrawalMethodsRequest struct {
	Asset            string
	AssetClass       AssetClass
	Network          string
	RebaseMultiplier RebaseMultiplier
}

// WithdrawalMethodResponse defines an available withdrawal method.
type WithdrawalMethodResponse struct {
	Asset     string                  `json:"asset"`
	Method    string                  `json:"method"`
	MethodID  string                  `json:"method_id"`
	Network   string                  `json:"network"`
	NetworkID string                  `json:"network_id"`
	Minimum   types.Number            `json:"minimum"`
	Fee       WithdrawalMethodFee     `json:"fee"`
	Limits    []WithdrawalMethodLimit `json:"limits"`
}

// WithdrawalMethodFee defines flat or percentage withdrawal fees.
type WithdrawalMethodFee struct {
	AssetClass string       `json:"aclass"`
	Asset      string       `json:"asset"`
	Fee        types.Number `json:"fee"`
	FeePercent types.Number `json:"fee_percentage"`
}

// WithdrawalMethodLimit defines rate limits for a withdrawal method.
type WithdrawalMethodLimit struct {
	Description string                          `json:"description"`
	LimitType   string                          `json:"limit_type"`
	Limits      map[string]WithdrawalLimitUsage `json:"limits"`
}

// WithdrawalLimitUsage defines usage within one withdrawal rate-limit window.
type WithdrawalLimitUsage struct {
	Maximum   types.Number `json:"maximum"`
	Remaining types.Number `json:"remaining"`
	Used      types.Number `json:"used"`
}

// GetWithdrawalAddressesRequest defines withdrawal address filters.
type GetWithdrawalAddressesRequest struct {
	Asset      string
	AssetClass AssetClass
	Method     string
	Key        string
	Verified   *bool
}

// WithdrawalAddressResponse defines a configured withdrawal address.
type WithdrawalAddressResponse struct {
	Address  string `json:"address"`
	Asset    string `json:"asset"`
	Method   string `json:"method"`
	Key      string `json:"key"`
	Tag      string `json:"tag"`
	Verified bool   `json:"verified"`
}

// CancelWithdrawalRequest defines a withdrawal cancellation.
type CancelWithdrawalRequest struct {
	Asset       string
	ReferenceID string
}

// WalletTransferRequest defines a transfer between Spot and Futures wallets.
type WalletTransferRequest struct {
	Asset  string
	From   Wallet
	To     Wallet
	Amount float64
}

// WalletTransferResponse defines a wallet transfer reference.
type WalletTransferResponse struct {
	ReferenceID string `json:"refid"`
}

// DepositFees the large list of predefined deposit fees
// Prone to change
var DepositFees = map[currency.Code]float64{
	currency.XTZ: 0.05,
}

// WithdrawalFees the large list of predefined withdrawal fees
// Prone to change
var WithdrawalFees = map[currency.Code]float64{
	currency.ZUSD: 5,
	currency.ZEUR: 5,
	currency.USD:  5,
	currency.EUR:  5,
	currency.REP:  0.01,
	currency.XXBT: 0.0005,
	currency.BTC:  0.0005,
	currency.XBT:  0.0005,
	currency.BCH:  0.0001,
	currency.ADA:  0.3,
	currency.DASH: 0.005,
	currency.XDG:  2,
	currency.EOS:  0.05,
	currency.ETH:  0.005,
	currency.ETC:  0.005,
	currency.GNO:  0.005,
	currency.ICN:  0.2,
	currency.LTC:  0.001,
	currency.MLN:  0.003,
	currency.XMR:  0.05,
	currency.QTUM: 0.01,
	currency.XRP:  0.02,
	currency.XLM:  0.00002,
	currency.USDT: 5,
	currency.XTZ:  0.05,
	currency.ZEC:  0.0001,
}
