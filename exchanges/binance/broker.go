package binance

import (
	"context"
	"errors"
	"net/http"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/types"
)

var errInvalidTransferType = errors.New("invalid broker transfer type")

// BrokerAPIKeyOptionsRequest permits asymmetric keys when creating a broker API key.
type BrokerAPIKeyOptionsRequest struct {
	PublicKey  string `json:"publicKey,omitempty"`
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// BrokerSubAccountRequest identifies an account under the broker's master account.
type BrokerSubAccountRequest struct {
	SubAccountID string `json:"subAccountId"`
	RecvWindow   uint64 `json:"recvWindow,omitempty"`
}

// BrokerAPIKeysRequest selects keys belonging to one broker sub-account.
type BrokerAPIKeysRequest struct {
	SubAccountID     string `json:"subAccountId"`
	SubAccountAPIKey string `json:"subAccountApiKey,omitempty"`
	Page             uint64 `json:"page,omitempty"`
	Size             uint64 `json:"size,omitempty"`
	RecvWindow       uint64 `json:"recvWindow,omitempty"`
}

// BrokerIPRestrictionRequest identifies the key whose IP restrictions are queried.
type BrokerIPRestrictionRequest struct {
	SubAccountID     string `json:"subAccountId"`
	SubAccountAPIKey string `json:"subAccountApiKey"`
	RecvWindow       uint64 `json:"recvWindow,omitempty"`
}

// BrokerFuturesTransferRequest moves funds between a sub-account's Spot and Futures wallets.
type BrokerFuturesTransferRequest struct {
	SubAccountID string        `json:"subAccountId"`
	Asset        currency.Code `json:"asset"`
	Amount       float64       `json:"amount"`
	Type         uint64        `json:"type"`
	RecvWindow   uint64        `json:"recvWindow,omitempty"`
}

// BrokerDepositHistoryRequest queries a specific deposit without the v1 time window.
type BrokerDepositHistoryRequest struct {
	DepositID    string `json:"depositId"`
	SubAccountID string `json:"subAccountId"`
	Limit        uint64 `json:"limit,omitempty"`
	Offset       uint64 `json:"offset,omitempty"`
	RecvWindow   uint64 `json:"recvWindow,omitempty"`
}

// BrokerUniversalTransferResponse uses txnId, unlike the ordinary sub-account API's tranId.
type BrokerUniversalTransferResponse struct {
	TransactionID       types.PreciseNumber `json:"txnId"`
	ClientTransactionID string              `json:"clientTranId"`
}

// BrokerUniversalTransferRecord identifies the accounts and amount of a broker transfer.
type BrokerUniversalTransferRecord struct {
	FromID              string              `json:"fromId"`
	ToID                string              `json:"toId"`
	Asset               currency.Code       `json:"asset"`
	Quantity            types.Number        `json:"qty"`
	Time                types.Time          `json:"time"`
	Status              string              `json:"status"`
	TransactionID       types.PreciseNumber `json:"txnId"`
	ClientTransactionID string              `json:"clientTranId"`
	FromAccountType     string              `json:"fromAccountType"`
	ToAccountType       string              `json:"toAccountType"`
}

// DeleteBrokerSubAccount permanently deletes an empty sub-account after its API keys have been removed.
func (e *Exchange) DeleteBrokerSubAccount(ctx context.Context, arg *BrokerSubAccountRequest) error {
	if err := common.NilGuard(arg); err != nil {
		return err
	}
	if arg.SubAccountID == "" {
		return errSubAccountIDMissing
	}
	return e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodDelete, "/sapi/v1/broker/subAccount", nil, request.Auth, arg, nil)
}

// GetBrokerSubAccountAPIKeys queries a broker sub-account's API keys and permissions.
func (e *Exchange) GetBrokerSubAccountAPIKeys(ctx context.Context, arg *BrokerAPIKeysRequest) ([]*SubAccountAPIKey, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.SubAccountID == "" {
		return nil, errSubAccountIDMissing
	}
	var response []*SubAccountAPIKey
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/broker/subAccountApi", nil, request.Auth, arg, &response)
}

// GetBrokerSubAccountIPRestriction queries a key's trusted IP list.
func (e *Exchange) GetBrokerSubAccountIPRestriction(ctx context.Context, arg *BrokerIPRestrictionRequest) (*SubAccountIPRestrictioin, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.SubAccountID == "" {
		return nil, errSubAccountIDMissing
	}
	if arg.SubAccountAPIKey == "" {
		return nil, errEmptySubAccountAPIKey
	}
	var response *SubAccountIPRestrictioin
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/broker/subAccountApi/ipRestriction", nil, request.Auth, arg, &response)
}

// BrokerFuturesTransfer moves assets between one sub-account's Spot and Futures wallets.
func (e *Exchange) BrokerFuturesTransfer(ctx context.Context, arg *BrokerFuturesTransferRequest) (*BrokerSubAccountTransfer, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.SubAccountID == "" {
		return nil, errSubAccountIDMissing
	}
	if arg.Asset.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if arg.Amount <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.Type < 1 || arg.Type > 4 {
		return nil, errInvalidTransferType
	}
	var response *BrokerSubAccountTransfer
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/sapi/v1/broker/futures/accountTransfer", nil, request.Auth, arg, &response)
}

// GetBrokerDepositHistory queries the v2 deposit history using a deposit identifier.
func (e *Exchange) GetBrokerDepositHistory(ctx context.Context, arg *BrokerDepositHistoryRequest) ([]*SubAccountTransferWithBroker, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.SubAccountID == "" {
		return nil, errSubAccountIDMissing
	}
	if arg.DepositID == "" {
		return nil, errTransactionIDRequired
	}
	var response []*SubAccountTransferWithBroker
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v2/broker/subAccount/depositHist", nil, request.Auth, arg, &response)
}
