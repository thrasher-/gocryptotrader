package kraken

import (
	"context"
	"net/http"
	"sync"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
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
		if feePair == nil {
			return 0, common.ErrNoResponse
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
