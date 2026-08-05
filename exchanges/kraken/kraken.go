package kraken

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/crypto"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/nonce"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/log"
)

const (
	krakenAPIURL                  = "https://api.kraken.com"
	krakenFuturesURL              = "https://futures.kraken.com/derivatives"
	krakenFuturesSupplementaryURL = "https://futures.kraken.com/api/"
	tradeBaseURL                  = "https://pro.kraken.com/app/trade/"
	tradeFuturesURL               = "https://futures.kraken.com/trade/futures/"
	krakenSpotVersion             = "0"
	krakenFuturesVersion          = "3"
)

// Exchange implements exchange.IBotExchange and contains additional specific api methods for interacting with Kraken
type Exchange struct {
	exchange.Base
	wsAuthToken           string
	wsAuthMtx             sync.RWMutex
	executionSequence     uint64
	executionResubPending bool
	executionSequenceMtx  sync.Mutex
}

// GetCurrentServerTime returns current server time
func (e *Exchange) GetCurrentServerTime(ctx context.Context) (*TimeResponse, error) {
	var result TimeResponse
	if err := e.SendHTTPRequest(ctx, exchange.RestSpot, "/0/public/Time", &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SeedAssets seeds Kraken's asset list and stores it in the
// asset translator
func (e *Exchange) SeedAssets(ctx context.Context) error {
	assets, err := e.GetAssets(ctx, new(GetAssetsRequest))
	if err != nil {
		return err
	}
	for orig, val := range assets {
		assetTranslator.Seed(orig, val.Altname)
	}

	assetPairs, err := e.GetAssetPairs(ctx, new(GetAssetPairsRequest))
	if err != nil {
		return err
	}
	for k, v := range assetPairs {
		assetTranslator.Seed(k, v.Altname)
	}
	return nil
}

// SendHTTPRequest sends an unauthenticated HTTP request.
func (e *Exchange) SendHTTPRequest(ctx context.Context, ep exchange.URL, path string, result any) error {
	endpoint, err := e.API.Endpoints.GetURL(ep)
	if err != nil {
		return err
	}

	var rawMessage json.RawMessage
	item := &request.Item{
		Method:                 http.MethodGet,
		Path:                   endpoint + path,
		Result:                 &rawMessage,
		Verbose:                e.Verbose,
		HTTPDebugging:          e.HTTPDebugging,
		HTTPRecording:          e.HTTPRecording,
		HTTPMockDataSliceLimit: e.HTTPMockDataSliceLimit,
	}

	err = e.SendPayload(ctx, request.Unset, func() (*request.Item, error) {
		return item, nil
	}, request.UnauthenticatedRequest)
	if err != nil {
		return err
	}

	isSpot := ep == exchange.RestSpot
	if isSpot {
		genResponse := genericRESTResponse{
			Result: result,
		}

		if err := json.Unmarshal(rawMessage, &genResponse); err != nil {
			return err
		}

		if genResponse.Error.Warnings() != "" {
			log.Warnf(log.ExchangeSys, "%v: REST request warning: %v", e.Name, genResponse.Error.Warnings())
		}

		return genResponse.Error.Errors()
	}

	if err := getFuturesErr(rawMessage); err != nil {
		return err
	}

	return json.Unmarshal(rawMessage, result)
}

// SendAuthenticatedHTTPRequest sends an authenticated HTTP request
func (e *Exchange) SendAuthenticatedHTTPRequest(ctx context.Context, ep exchange.URL, method string, params url.Values, result any) error {
	creds, err := e.GetCredentials(ctx)
	if err != nil {
		return err
	}
	endpoint, err := e.API.Endpoints.GetURL(ep)
	if err != nil {
		return err
	}

	interim := json.RawMessage{}
	requestResult := any(&interim)
	rawResponse, rawResult := result.(*request.RawResponse)
	if rawResult {
		if rawResponse == nil {
			return common.ErrNilPointer
		}
		requestResult = rawResponse
	}
	err = e.SendPayload(ctx, request.Unset, func() (*request.Item, error) {
		nonce := e.Requester.GetNonce(nonce.UnixNano).String()
		params.Set("nonce", nonce)
		encoded := params.Encode()

		shasum := sha256.Sum256([]byte(nonce + encoded))
		path := "/0/private/" + method
		hmac, _ := crypto.GetHMAC(crypto.HashSHA512, append([]byte(path), shasum[:]...), []byte(creds.Secret)) // SHA-512 writes cannot fail.

		headers := make(map[string]string)
		headers["API-Key"] = creds.Key
		headers["API-Sign"] = base64.StdEncoding.EncodeToString(hmac)

		return &request.Item{
			Method:                 http.MethodPost,
			Path:                   endpoint + path,
			Headers:                headers,
			Body:                   strings.NewReader(encoded),
			Result:                 requestResult,
			NonceEnabled:           true,
			Verbose:                e.Verbose,
			HTTPDebugging:          e.HTTPDebugging,
			HTTPRecording:          e.HTTPRecording,
			HTTPMockDataSliceLimit: e.HTTPMockDataSliceLimit,
		}, nil
	}, request.AuthenticatedRequest)
	if err != nil {
		return err
	}
	if rawResult {
		if !json.Valid(*rawResponse) {
			return nil
		}
		trimmedResponse := bytes.TrimSpace(*rawResponse)
		if trimmedResponse[0] != '{' {
			return nil
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(trimmedResponse, &fields) // A valid JSON object always decodes into raw-message fields.
		if _, ok := fields["error"]; !ok {
			return nil
		}
		genResponse := genericRESTResponse{}
		if err := json.Unmarshal(*rawResponse, &genResponse); err != nil {
			return fmt.Errorf("%w %w", request.ErrAuthRequestFailed, err)
		}
		if err := genResponse.Error.Errors(); err != nil {
			return fmt.Errorf("%w %w", request.ErrAuthRequestFailed, err)
		}
		if genResponse.Error.Warnings() != "" {
			log.Warnf(log.ExchangeSys, "%v: AUTH REST request warning: %v", e.Name, genResponse.Error.Warnings())
		}
		return nil
	}

	genResponse := genericRESTResponse{
		Result: result,
	}

	if err := json.Unmarshal(interim, &genResponse); err != nil {
		return fmt.Errorf("%w %w", request.ErrAuthRequestFailed, err)
	}

	if err := genResponse.Error.Errors(); err != nil {
		return fmt.Errorf("%w %w", request.ErrAuthRequestFailed, err)
	}

	if genResponse.Error.Warnings() != "" {
		log.Warnf(log.ExchangeSys, "%v: AUTH REST request warning: %v", e.Name, genResponse.Error.Warnings())
	}

	return nil
}

// GetFee returns an estimate of fee based on type of transaction
func (e *Exchange) GetFee(ctx context.Context, feeBuilder *exchange.FeeBuilder) (float64, error) {
	var fee float64
	switch feeBuilder.FeeType {
	case exchange.CryptocurrencyTradeFee:
		pair, err := e.FormatSymbol(feeBuilder.Pair, asset.Spot)
		if err != nil {
			return 0, err
		}
		includeFeeInfo := true
		feePair, err := e.GetTradeVolume(ctx, &GetTradeVolumeRequest{
			Pairs:   []TradeVolumePairRequest{{Asset: pair, AssetClass: AssetClassCurrency}},
			FeeInfo: &includeFeeInfo,
		})
		if err != nil {
			return 0, err
		}
		if feeBuilder.IsMaker {
			fee = calculateTradingFee(feePair.Currency,
				feePair.FeesMaker,
				feeBuilder.PurchasePrice,
				feeBuilder.Amount)
		} else {
			fee = calculateTradingFee(feePair.Currency,
				feePair.Fees,
				feeBuilder.PurchasePrice,
				feeBuilder.Amount)
		}
	case exchange.CryptocurrencyWithdrawalFee:
		fee = getWithdrawalFee(feeBuilder.Pair.Base)
	case exchange.InternationalBankDepositFee:
		depositMethods, err := e.GetDepositMethods(ctx, &GetDepositMethodsRequest{Asset: feeBuilder.FiatCurrency.String()})
		if err != nil {
			return 0, err
		}

		for _, i := range depositMethods {
			if feeBuilder.BankTransactionType == exchange.WireTransfer {
				if i.Method == "SynapsePay (US Wire)" {
					fee = i.Fee.Float64()
					return fee, nil
				}
			}
		}
	case exchange.CryptocurrencyDepositFee:
		fee = getCryptocurrencyDepositFee(feeBuilder.Pair.Base)

	case exchange.InternationalBankWithdrawalFee:
		fee = getWithdrawalFee(feeBuilder.FiatCurrency)
	case exchange.OfflineTradeFee:
		fee = getOfflineTradeFee(feeBuilder.PurchasePrice, feeBuilder.Amount)
	}
	if fee < 0 {
		fee = 0
	}

	return fee, nil
}

// getOfflineTradeFee calculates the worst case-scenario trading fee
func getOfflineTradeFee(price, amount float64) float64 {
	return 0.0016 * price * amount
}

func getWithdrawalFee(c currency.Code) float64 {
	return WithdrawalFees[c]
}

func getCryptocurrencyDepositFee(c currency.Code) float64 {
	return DepositFees[c]
}

func calculateTradingFee(ccy string, feePair map[string]TradeVolumeFee, purchasePrice, amount float64) float64 {
	return (feePair[ccy].Fee.Float64() / 100) * purchasePrice * amount
}

// GetWebsocketToken returns the websocket token and its expiry.
func (e *Exchange) GetWebsocketToken(ctx context.Context) (*WsTokenResponse, error) {
	var response WsTokenResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "GetWebSocketsToken", url.Values{}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// LookupAltName converts a currency into its altName (ZUSD -> USD)
func (a *assetTranslatorStore) LookupAltName(target string) string {
	a.l.RLock()
	alt, ok := a.Assets[target]
	if !ok {
		a.l.RUnlock()
		return ""
	}
	a.l.RUnlock()
	return alt
}

// LookupCurrency converts an altName to its original type (USD -> ZUSD)
func (a *assetTranslatorStore) LookupCurrency(target string) string {
	a.l.RLock()
	for k, v := range a.Assets {
		if v == target {
			a.l.RUnlock()
			return k
		}
	}
	a.l.RUnlock()
	return ""
}

// Seed seeds a currency translation pair
func (a *assetTranslatorStore) Seed(orig, alt string) {
	a.l.Lock()
	if a.Assets == nil {
		a.Assets = make(map[string]string)
	}

	if _, ok := a.Assets[orig]; ok {
		a.l.Unlock()
		return
	}

	a.Assets[orig] = alt
	a.l.Unlock()
}

// Seeded checks if assets have been seeded
func (a *assetTranslatorStore) Seeded() bool {
	a.l.RLock()
	isSeeded := len(a.Assets) > 0
	a.l.RUnlock()
	return isSeeded
}
