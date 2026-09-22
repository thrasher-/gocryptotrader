package binance

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/order/limits"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// GetSpotAssetTagsResponse contains the documented GetSpotAssetTagsResponseInner fields.
type GetSpotAssetTagsResponse struct {
	AssetCode currency.Code `json:"assetCode"`
	AssetName string        `json:"assetName"`
	Trading   bool          `json:"trading"`
	Tags      []string      `json:"tags"`
}

// GetSpotAssetTagsRequest holds the documented parameters for GetSpotAssetTags.
type GetSpotAssetTagsRequest struct {
	Tag string `json:"tag,omitempty"`
}

// BrokerTravelRuleWithdrawResponse contains the documented BrokerWithdrawResponse fields.
type BrokerTravelRuleWithdrawResponse struct {
	TravelRuleID uint64 `json:"trId"`
	Accepted     bool   `json:"accepted"`
	Info         string `json:"info"`
}

// BrokerTravelRuleWithdrawRequest holds the documented parameters for BrokerTravelRuleWithdraw.
type BrokerTravelRuleWithdrawRequest struct {
	Address            string          `json:"address"`
	Coin               currency.Code   `json:"coin"`
	Amount             float64         `json:"amount"`
	WithdrawOrderID    string          `json:"withdrawOrderId"`
	Questionnaire      json.RawMessage `json:"questionnaire"`
	OriginatorPII      json.RawMessage `json:"originatorPii"`
	AddressTag         string          `json:"addressTag,omitempty"`
	Network            string          `json:"network,omitempty"`
	AddressName        string          `json:"addressName,omitempty"`
	TransactionFeeFlag *bool           `json:"transactionFeeFlag,omitempty"`
	WalletType         *uint64         `json:"walletType,omitempty"`
}

// GetTravelRuleDepositHistoryV2Response contains the documented DepositHistoryV2ResponseInner fields.
type GetTravelRuleDepositHistoryV2Response struct {
	DepositID            types.PreciseNumber `json:"depositId"`
	Amount               types.Number        `json:"amount"`
	Network              string              `json:"network"`
	Coin                 currency.Code       `json:"coin"`
	DepositStatus        uint64              `json:"depositStatus"`
	TravelRuleReqStatus  uint64              `json:"travelRuleReqStatus"`
	Address              string              `json:"address"`
	AddressTag           string              `json:"addressTag"`
	TxID                 string              `json:"txId"`
	TransferType         uint64              `json:"transferType"`
	ConfirmTimes         string              `json:"confirmTimes"`
	RequireQuestionnaire bool                `json:"requireQuestionnaire"`
	Questionnaire        json.RawMessage     `json:"questionnaire"`
	InsertTime           types.Time          `json:"insertTime"`
}

// GetTravelRuleDepositHistoryV2Request holds the documented parameters for GetTravelRuleDepositHistoryV2.
type GetTravelRuleDepositHistoryV2Request struct {
	DepositID             string        `json:"depositId,omitempty"`
	TxID                  string        `json:"txId,omitempty"`
	Network               string        `json:"network,omitempty"`
	Coin                  currency.Code `json:"coin,omitzero"`
	RetrieveQuestionnaire *bool         `json:"retrieveQuestionnaire,omitempty"`
	StartTime             time.Time     `json:"-"`
	EndTime               time.Time     `json:"-"`
	Offset                uint64        `json:"offset,omitempty"`
	Limit                 uint64        `json:"limit,omitempty"`
}

// GetAddressVerificationsResponse contains the documented FetchAddressVerificationListResponseInner fields.
type GetAddressVerificationsResponse struct {
	Status               string          `json:"status"`
	Token                currency.Code   `json:"token"`
	Network              string          `json:"network"`
	WalletAddress        string          `json:"walletAddress"`
	AddressQuestionnaire json.RawMessage `json:"addressQuestionnaire"`
}

// GetAddressVerificationsRequest holds the documented parameters for GetAddressVerifications.
type GetAddressVerificationsRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// GetTravelRuleCountriesCountriesResponse contains the documented GetCountryListResponseCountriesInner fields.
type GetTravelRuleCountriesCountriesResponse struct {
	CountryCode           string `json:"countryCode"`
	CountryName           string `json:"countryName"`
	BlockType             string `json:"blockType"`
	DepositAllowed        bool   `json:"depositAllowed"`
	WithdrawalAllowed     bool   `json:"withdrawalAllowed"`
	HasRegionRestrictions bool   `json:"hasRegionRestrictions"`
}

// GetTravelRuleCountriesResponse contains the documented GetCountryListResponse fields.
type GetTravelRuleCountriesResponse struct {
	Countries   []*GetTravelRuleCountriesCountriesResponse `json:"countries"`
	LastUpdated types.Time                                 `json:"lastUpdated"`
}

// GetTravelRuleCountriesRequest holds the documented parameters for GetTravelRuleCountries.
type GetTravelRuleCountriesRequest struct {
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// GetTravelRuleRegionsRegionsResponse contains the documented GetRegionListResponseRegionsInner fields.
type GetTravelRuleRegionsRegionsResponse struct {
	RegionName        string `json:"regionName"`
	BlockType         string `json:"blockType"`
	DepositAllowed    bool   `json:"depositAllowed"`
	WithdrawalAllowed bool   `json:"withdrawalAllowed"`
}

// GetTravelRuleRegionsResponse contains the documented GetRegionListResponse fields.
type GetTravelRuleRegionsResponse struct {
	CountryCode string                                 `json:"countryCode"`
	Regions     []*GetTravelRuleRegionsRegionsResponse `json:"regions"`
	LastUpdated types.Time                             `json:"lastUpdated"`
}

// GetTravelRuleRegionsRequest holds the documented parameters for GetTravelRuleRegions.
type GetTravelRuleRegionsRequest struct {
	CountryCode string `json:"countryCode"`
	RecvWindow  uint64 `json:"recvWindow,omitempty"`
}

// SubmitBrokerDepositQuestionnaireResponse contains the documented SubmitDepositQuestionnaireResponse fields.
type SubmitBrokerDepositQuestionnaireResponse struct {
	TravelRuleID uint64 `json:"trId"`
	Accepted     bool   `json:"accepted"`
	Info         string `json:"info"`
}

// SubmitBrokerDepositQuestionnaireRequest holds the documented parameters for SubmitBrokerDepositQuestionnaire.
type SubmitBrokerDepositQuestionnaireRequest struct {
	SubAccountID   string          `json:"subAccountId"`
	DepositID      uint64          `json:"depositId"`
	Questionnaire  json.RawMessage `json:"questionnaire"`
	BeneficiaryPII json.RawMessage `json:"beneficiaryPii"`
	Network        string          `json:"network,omitempty"`
	Coin           currency.Code   `json:"coin,omitzero"`
	Amount         float64         `json:"amount,omitempty"`
	Address        string          `json:"address,omitempty"`
	AddressTag     string          `json:"addressTag,omitempty"`
}

// SubmitDepositQuestionnaireV2Response contains the documented SubmitDepositQuestionnaireV2Response fields.
type SubmitDepositQuestionnaireV2Response struct {
	TravelRuleID uint64 `json:"trId"`
	Accepted     bool   `json:"accepted"`
	Info         string `json:"info"`
}

// SubmitDepositQuestionnaireV2Request holds the documented parameters for SubmitDepositQuestionnaireV2.
type SubmitDepositQuestionnaireV2Request struct {
	DepositID     uint64          `json:"depositId"`
	Questionnaire json.RawMessage `json:"questionnaire"`
}

// GetTravelRuleWithdrawalHistoryResponse contains the documented WithdrawHistoryV1ResponseInner fields.
type GetTravelRuleWithdrawalHistoryResponse struct {
	ID               string        `json:"id"`
	TravelRuleID     uint64        `json:"trId"`
	Amount           types.Number  `json:"amount"`
	TransactionFee   types.Number  `json:"transactionFee"`
	Coin             currency.Code `json:"coin"`
	WithdrawalStatus uint64        `json:"withdrawalStatus"`
	TravelRuleStatus uint64        `json:"travelRuleStatus"`
	Address          string        `json:"address"`
	TxID             string        `json:"txId"`
	ApplyTime        Timestamp     `json:"applyTime"`
	Network          string        `json:"network"`
	TransferType     uint64        `json:"transferType"`
	WithdrawOrderID  string        `json:"withdrawOrderId"`
	Info             string        `json:"info"`
	ConfirmNo        uint64        `json:"confirmNo"`
	WalletType       uint64        `json:"walletType"`
	TxKey            string        `json:"txKey"`
	Questionnaire    string        `json:"questionnaire"`
	CompleteTime     Timestamp     `json:"completeTime"`
}

// GetTravelRuleWithdrawalHistoryRequest holds the documented parameters for GetTravelRuleWithdrawalHistory.
type GetTravelRuleWithdrawalHistoryRequest struct {
	TravelRuleID     string        `json:"trId,omitempty"`
	TxID             string        `json:"txId,omitempty"`
	WithdrawOrderID  string        `json:"withdrawOrderId,omitempty"`
	Network          string        `json:"network,omitempty"`
	Coin             currency.Code `json:"coin,omitzero"`
	TravelRuleStatus *uint64       `json:"travelRuleStatus,omitempty"`
	Offset           uint64        `json:"offset,omitempty"`
	Limit            uint64        `json:"limit,omitempty"`
	StartTime        time.Time     `json:"-"`
	EndTime          time.Time     `json:"-"`
	RecvWindow       uint64        `json:"recvWindow,omitempty"`
}

// TravelRuleWithdrawResponse contains the documented WithdrawTravelRuleResponse fields.
type TravelRuleWithdrawResponse struct {
	TravelRuleID uint64 `json:"trId"`
	Accepted     bool   `json:"accepted"`
	Info         string `json:"info"`
}

// TravelRuleWithdrawRequest holds the documented parameters for TravelRuleWithdraw.
type TravelRuleWithdrawRequest struct {
	Coin               currency.Code   `json:"coin"`
	Address            string          `json:"address"`
	Amount             float64         `json:"amount"`
	Questionnaire      json.RawMessage `json:"questionnaire"`
	WithdrawOrderID    string          `json:"withdrawOrderId,omitempty"`
	Network            string          `json:"network,omitempty"`
	AddressTag         string          `json:"addressTag,omitempty"`
	TransactionFeeFlag *bool           `json:"transactionFeeFlag,omitempty"`
	Name               string          `json:"name,omitempty"`
	WalletType         *uint64         `json:"walletType,omitempty"`
	RecvWindow         uint64          `json:"recvWindow,omitempty"`
}

// GetSpotAssetTags calls GET /sapi/v1/spot/asset/tags.
// Binance requires an API key for this MARKET_DATA endpoint (live rejection -2014 without one).
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/asset#get-spot-asset-tags.
func (e *Exchange) GetSpotAssetTags(ctx context.Context, arg *GetSpotAssetTagsRequest) ([]*GetSpotAssetTagsResponse, error) {
	if arg == nil {
		arg = new(GetSpotAssetTagsRequest)
	}
	params := url.Values{}
	if arg != nil {
		var err error
		params, err = interfaceToParams(arg)
		if err != nil {
			return nil, err
		}
	}
	var response []*GetSpotAssetTagsResponse
	return response, e.SendAPIKeyHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, common.EncodeURLValues("/sapi/v1/spot/asset/tags", params), sapiGetSpotAssetTagsRate, &response)
}

// BrokerTravelRuleWithdraw calls POST /sapi/v1/localentity/broker/withdraw/apply.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#broker-withdraw.
func (e *Exchange) BrokerTravelRuleWithdraw(ctx context.Context, arg *BrokerTravelRuleWithdrawRequest) (*BrokerTravelRuleWithdrawResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Address == "" {
		return nil, errAddressRequired
	}
	if arg.Coin.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if arg.Amount <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if arg.WithdrawOrderID == "" {
		return nil, order.ErrOrderIDNotSet
	}
	if len(arg.Questionnaire) == 0 {
		return nil, errQuestionnaireRequired
	}
	if len(arg.OriginatorPII) == 0 {
		return nil, errAccountRequired
	}
	params := url.Values{}
	var response *BrokerTravelRuleWithdrawResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/sapi/v1/localentity/broker/withdraw/apply", params, sapiBrokerTravelRuleWithdrawRate, arg, &response)
}

// GetTravelRuleDepositHistoryV2 calls GET /sapi/v2/localentity/deposit/history.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#deposit-history-v2.
func (e *Exchange) GetTravelRuleDepositHistoryV2(ctx context.Context, arg *GetTravelRuleDepositHistoryV2Request) ([]*GetTravelRuleDepositHistoryV2Response, error) {
	if arg == nil {
		arg = new(GetTravelRuleDepositHistoryV2Request)
	}
	if !arg.StartTime.IsZero() && !arg.EndTime.IsZero() {
		if err := common.StartEndTimeCheck(arg.StartTime, arg.EndTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !arg.StartTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(arg.StartTime.UTC().UnixMilli(), 10))
	}
	if !arg.EndTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(arg.EndTime.UTC().UnixMilli(), 10))
	}
	var response []*GetTravelRuleDepositHistoryV2Response
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v2/localentity/deposit/history", params, sapiGetTravelRuleDepositHistoryV2Rate, arg, &response)
}

// GetAddressVerifications calls GET /sapi/v1/addressVerify/list.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#fetch-address-verification-list.
func (e *Exchange) GetAddressVerifications(ctx context.Context, arg *GetAddressVerificationsRequest) ([]*GetAddressVerificationsResponse, error) {
	if arg == nil {
		arg = new(GetAddressVerificationsRequest)
	}
	params := url.Values{}
	var response []*GetAddressVerificationsResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/addressVerify/list", params, sapiGetAddressVerificationsRate, arg, &response)
}

// GetTravelRuleCountries calls GET /sapi/v1/localentity/country/list.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#get-country-list.
func (e *Exchange) GetTravelRuleCountries(ctx context.Context, arg *GetTravelRuleCountriesRequest) (*GetTravelRuleCountriesResponse, error) {
	if arg == nil {
		arg = new(GetTravelRuleCountriesRequest)
	}
	params := url.Values{}
	var response *GetTravelRuleCountriesResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/localentity/country/list", params, sapiGetTravelRuleCountriesRate, arg, &response)
}

// GetTravelRuleRegions calls GET /sapi/v1/localentity/region/list.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#get-region-list.
func (e *Exchange) GetTravelRuleRegions(ctx context.Context, arg *GetTravelRuleRegionsRequest) (*GetTravelRuleRegionsResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.CountryCode == "" {
		return nil, errCodeRequired
	}
	params := url.Values{}
	var response *GetTravelRuleRegionsResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/localentity/region/list", params, sapiGetTravelRuleRegionsRate, arg, &response)
}

// SubmitBrokerDepositQuestionnaire calls PUT /sapi/v1/localentity/broker/deposit/provide-info.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#submit-deposit-questionnaire.
func (e *Exchange) SubmitBrokerDepositQuestionnaire(ctx context.Context, arg *SubmitBrokerDepositQuestionnaireRequest) (*SubmitBrokerDepositQuestionnaireResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.SubAccountID == "" {
		return nil, errSubAccountIDMissing
	}
	if arg.DepositID == 0 {
		return nil, errTransactionIDRequired
	}
	if len(arg.Questionnaire) == 0 {
		return nil, errQuestionnaireRequired
	}
	if len(arg.BeneficiaryPII) == 0 {
		return nil, errAccountRequired
	}
	params := url.Values{}
	var response *SubmitBrokerDepositQuestionnaireResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPut, "/sapi/v1/localentity/broker/deposit/provide-info", params, sapiSubmitBrokerDepositQuestionnaireRate, arg, &response)
}

// SubmitDepositQuestionnaireV2 calls PUT /sapi/v2/localentity/deposit/provide-info.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#submit-deposit-questionnaire-v2.
func (e *Exchange) SubmitDepositQuestionnaireV2(ctx context.Context, arg *SubmitDepositQuestionnaireV2Request) (*SubmitDepositQuestionnaireV2Response, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.DepositID == 0 {
		return nil, errTransactionIDRequired
	}
	if len(arg.Questionnaire) == 0 {
		return nil, errQuestionnaireRequired
	}
	params := url.Values{}
	var response *SubmitDepositQuestionnaireV2Response
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPut, "/sapi/v2/localentity/deposit/provide-info", params, sapiSubmitDepositQuestionnaireV2Rate, arg, &response)
}

// GetTravelRuleWithdrawalHistory calls GET /sapi/v1/localentity/withdraw/history.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#withdraw-history-v1.
func (e *Exchange) GetTravelRuleWithdrawalHistory(ctx context.Context, arg *GetTravelRuleWithdrawalHistoryRequest) ([]*GetTravelRuleWithdrawalHistoryResponse, error) {
	if arg == nil {
		arg = new(GetTravelRuleWithdrawalHistoryRequest)
	}
	if !arg.StartTime.IsZero() && !arg.EndTime.IsZero() {
		if err := common.StartEndTimeCheck(arg.StartTime, arg.EndTime); err != nil {
			return nil, err
		}
	}
	params := url.Values{}
	if !arg.StartTime.IsZero() {
		params.Set("startTime", strconv.FormatInt(arg.StartTime.UTC().UnixMilli(), 10))
	}
	if !arg.EndTime.IsZero() {
		params.Set("endTime", strconv.FormatInt(arg.EndTime.UTC().UnixMilli(), 10))
	}
	var response []*GetTravelRuleWithdrawalHistoryResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodGet, "/sapi/v1/localentity/withdraw/history", params, sapiGetTravelRuleWithdrawalHistoryRate, arg, &response)
}

// TravelRuleWithdraw calls POST /sapi/v1/localentity/withdraw/apply.
// See https://developers.binance.com/en/docs/catalog/core-trading-wallet/api/rest-api/travel-rule#withdraw-travel-rule.
func (e *Exchange) TravelRuleWithdraw(ctx context.Context, arg *TravelRuleWithdrawRequest) (*TravelRuleWithdrawResponse, error) {
	if err := common.NilGuard(arg); err != nil {
		return nil, err
	}
	if arg.Coin.IsEmpty() {
		return nil, currency.ErrCurrencyCodeEmpty
	}
	if arg.Address == "" {
		return nil, errAddressRequired
	}
	if arg.Amount <= 0 {
		return nil, limits.ErrAmountBelowMin
	}
	if len(arg.Questionnaire) == 0 {
		return nil, errQuestionnaireRequired
	}
	params := url.Values{}
	var response *TravelRuleWithdrawResponse
	return response, e.SendAuthHTTPRequest(ctx, exchange.RestSpot, http.MethodPost, "/sapi/v1/localentity/withdraw/apply", params, sapiTravelRuleWithdrawRate, arg, &response)
}
