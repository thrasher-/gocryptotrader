package gateio

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
	"github.com/thrasher-corp/gocryptotrader/exchanges/sharedtestvalues"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func TestGetFlatStablecoinQuote(t *testing.T) {
	t.Parallel()
	_, err := e.GetFiatStablecoinQuote(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	_, err = e.GetFiatStablecoinQuote(t.Context(), &OTCQuoteRequest{})
	require.ErrorIs(t, err, errOTCSideRequired)

	_, err = e.GetFiatStablecoinQuote(t.Context(), &OTCQuoteRequest{Side: "PAY"})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.GetFiatStablecoinQuote(t.Context(), &OTCQuoteRequest{Side: "PAY", PayCoin: currency.USD})
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	_, err = e.GetFiatStablecoinQuote(t.Context(), &OTCQuoteRequest{Side: "invalid", PayCoin: currency.USD, GetCoin: currency.USDT})
	require.ErrorIs(t, err, order.ErrSideIsInvalid, "GetFiatStablecoinQuote must require a valid side")

	_, err = e.GetFiatStablecoinQuote(t.Context(), &OTCQuoteRequest{Side: "PAY", PayCoin: currency.USD, GetCoin: currency.USDT})
	require.ErrorIs(t, err, order.ErrAmountMustBeSet, "GetFiatStablecoinQuote must require a pay amount for PAY quotes")

	_, err = e.GetFiatStablecoinQuote(t.Context(), &OTCQuoteRequest{Side: "GET", PayCoin: currency.USD, GetCoin: currency.USDT})
	require.ErrorIs(t, err, order.ErrAmountMustBeSet, "GetFiatStablecoinQuote must require a receive amount for GET quotes")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err := e.GetFiatStablecoinQuote(t.Context(), &OTCQuoteRequest{
		Side:      "PAY",
		PayCoin:   currency.USD,
		GetCoin:   currency.USDT,
		PayAmount: 100,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateFlatOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CreateFiatOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OTCFiatOrderRequest{}
	_, err = e.CreateFiatOrder(t.Context(), arg)
	require.ErrorIs(t, err, errOTCOrderTypeRequired)

	arg.Type = "BUY"
	_, err = e.CreateFiatOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid)

	arg.Side = "FIAT"
	_, err = e.CreateFiatOrder(t.Context(), arg)
	require.ErrorIs(t, err, errOTCQuoteTokenRequired)

	arg.QuoteToken = "token"
	_, err = e.CreateFiatOrder(t.Context(), arg)
	require.ErrorIs(t, err, errOTCBankIDRequired)

	arg.BankID = "2"
	_, err = e.CreateFiatOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.CryptoCurrency = currency.USDT
	_, err = e.CreateFiatOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty)

	arg.FiatCurrency = currency.USD
	_, err = e.CreateFiatOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrAmountMustBeSet)

	arg.CryptoAmount = 1
	_, err = e.CreateFiatOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrAmountMustBeSet)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	_, err = e.CreateFiatOrder(t.Context(), &OTCFiatOrderRequest{
		Type:           "BUY",
		Side:           "FIAT",
		FiatCurrency:   currency.USD,
		CryptoCurrency: currency.USDT,
		CryptoAmount:   100,
		FiatAmount:     100,
		QuoteToken:     "some_token",
		BankID:         "72",
	})
	require.NoError(t, err)
}

func TestCreateStablecoinOrder(t *testing.T) {
	t.Parallel()
	_, err := e.CreateStablecoinOrder(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OTCStablecoinOrderRequest{}
	_, err = e.CreateStablecoinOrder(t.Context(), arg)
	require.ErrorIs(t, err, currency.ErrCurrencyCodeEmpty, "CreateStablecoinOrder must require currencies")
	arg.PayCoin = currency.USD
	arg.GetCoin = currency.USDT
	_, err = e.CreateStablecoinOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrAmountMustBeSet, "CreateStablecoinOrder must require a pay amount")
	arg.PayAmount = 100
	_, err = e.CreateStablecoinOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrAmountMustBeSet, "CreateStablecoinOrder must require a receive amount")
	arg.GetAmount = 100
	_, err = e.CreateStablecoinOrder(t.Context(), arg)
	require.ErrorIs(t, err, errOTCSideRequired, "CreateStablecoinOrder must require a side")
	arg.Side = "invalid"
	_, err = e.CreateStablecoinOrder(t.Context(), arg)
	require.ErrorIs(t, err, order.ErrSideIsInvalid, "CreateStablecoinOrder must require a valid side")
	arg.Side = "PAY"
	_, err = e.CreateStablecoinOrder(t.Context(), arg)
	require.ErrorIs(t, err, errOTCQuoteTokenRequired, "CreateStablecoinOrder must require a quote token")

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	arg.QuoteToken = "some_token"
	_, err = e.CreateStablecoinOrder(t.Context(), arg)
	require.NoError(t, err)
}

func TestGetUserBankCardList(t *testing.T) {
	t.Parallel()
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err := e.GetUserBankCardList(t.Context())
	require.NoError(t, err, "GetUserBankCardList must not error")
	if mockTests {
		require.Len(t, result, 1, "GetUserBankCardList must return the mocked bank card")
		assert.Equal(t, uint64(1), result[0].IsDefault, "default bank indicator should decode from an integer")
		assert.Equal(t, time.Date(2025, time.September, 9, 10, 0, 0, 0, time.UTC), result[0].SubmitTime.Time(), "submit time should decode the API timestamp")
	}
}

func TestCreateBankCard(t *testing.T) {
	t.Parallel()
	_, err := e.CreateBankCard(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OTCBankCreateMultipartRequest{}
	_, err = e.CreateBankCard(t.Context(), arg)
	require.ErrorIs(t, err, errBankAccountNameRequired)

	arg.BankAccountName = "John Doe"
	_, err = e.CreateBankCard(t.Context(), arg)
	require.ErrorIs(t, err, errBankNameRequired)

	arg.BankName = "Bank of Test"
	_, err = e.CreateBankCard(t.Context(), arg)
	require.ErrorIs(t, err, errBankCountryRequired)

	arg.BankCountry = "US"
	_, err = e.CreateBankCard(t.Context(), arg)
	require.ErrorIs(t, err, errBankAddressRequired)

	arg.BankAddress = "123 Test Street"
	_, err = e.CreateBankCard(t.Context(), arg)
	require.ErrorIs(t, err, errIBANAddressRequired)

	arg.IBAN = "GB33BUKB20201555555555"
	_, err = e.CreateBankCard(t.Context(), arg)
	require.ErrorIs(t, err, errSWIFTAddressRequired)

	arg.SWIFT = "BUKBGB22"
	_, err = e.CreateBankCard(t.Context(), arg)
	require.ErrorIs(t, err, errDocumentationFileRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	arg.DocumentationFile = "base64encodeddocument"
	result, err := e.CreateBankCard(t.Context(), arg)
	require.NoError(t, err)
	assert.NotNil(t, result, "result should not be nil")
}

func TestDeleteBankCard(t *testing.T) {
	t.Parallel()
	err := e.DeleteBankCard(t.Context(), "")
	require.ErrorIs(t, err, errOTCBankIDRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	err = e.DeleteBankCard(t.Context(), "123")
	require.NoError(t, err)
}

func TestSubmitBankCardSupplementMaterials(t *testing.T) {
	t.Parallel()
	err := e.SubmitBankCardSupplementMaterials(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OTCBankPersonalSupplementMultipartRequest{}
	err = e.SubmitBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errOTCBankIDRequired)

	arg.BankID = "123"
	err = e.SubmitBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errDocumentationFileRequired)

	arg.IDDocumentFront = "base64frontdocument"
	err = e.SubmitBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errDocumentationFileRequired)

	arg.IDDocumentBack = "base64backdocument"
	err = e.SubmitBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errBankAddressRequired)

	arg.AddressProof = "base64addressproof"
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	err = e.SubmitBankCardSupplementMaterials(t.Context(), arg)
	require.NoError(t, err)
}

func TestSubmitEnterpriseBankCardSupplementMaterials(t *testing.T) {
	t.Parallel()
	err := e.SubmitEnterpriseBankCardSupplementMaterials(t.Context(), nil)
	require.ErrorIs(t, err, common.ErrNilPointer)

	arg := &OTCBankEnterpriseSupplementMultipartRequest{}
	err = e.SubmitEnterpriseBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errOTCBankIDRequired)

	arg.BankID = "123"
	err = e.SubmitEnterpriseBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errBusinessLicenseCertificateRequired)

	arg.Certificate = "base64certificate"
	err = e.SubmitEnterpriseBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errShareholdersRequired)

	arg.ShareHolders = "base64shareholders"
	err = e.SubmitEnterpriseBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errPassportRequired)

	arg.Passport = "base64passport"
	err = e.SubmitEnterpriseBankCardSupplementMaterials(t.Context(), arg)
	require.ErrorIs(t, err, errShareholdersRequired)

	arg.ShareHoldingStructure = "base64structure"
	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	err = e.SubmitEnterpriseBankCardSupplementMaterials(t.Context(), arg)
	require.NoError(t, err)
}

func TestSetDefaultBankCard(t *testing.T) {
	t.Parallel()
	err := e.SetDefaultBankCard(t.Context(), "")
	require.ErrorIs(t, err, errOTCBankIDRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	err = e.SetDefaultBankCard(t.Context(), "123")
	require.NoError(t, err)
}

func TestGetChecklistOfMaterialsToSupplementForBankCard(t *testing.T) {
	t.Parallel()
	_, err := e.GetCheckListOfMaterialsToSupplementForBankCard(t.Context(), "")
	require.ErrorIs(t, err, errOTCBankIDRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err := e.GetCheckListOfMaterialsToSupplementForBankCard(t.Context(), "123")
	require.NoError(t, err, "GetCheckListOfMaterialsToSupplementForBankCard must not error")
	if mockTests {
		require.Len(t, result.Items, 1, "supplement checklist must contain the mocked item")
		assert.Equal(t, "Proof of address", result.Items[0].Description, "supplement description should match")
	}
}

func TestMarkFlatOrderAsPaid(t *testing.T) {
	t.Parallel()
	err := e.MarkFiatOrderAsPaid(t.Context(), "", "client-order-id-here", "", "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	err = e.MarkFiatOrderAsPaid(t.Context(), "203", "client-order-id-here", "", "")
	require.ErrorIs(t, err, errPaymentReceiptFileKeyRequired)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	err = e.MarkFiatOrderAsPaid(t.Context(), "203", "client-order-id-here", "payment-receipt-file-key", "payment-receipt")
	require.NoError(t, err)
}

func TestCancelFlatOrder(t *testing.T) {
	t.Parallel()
	err := e.CancelFiatOrder(t.Context(), "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e, canManipulateRealOrders)
	}
	err = e.CancelFiatOrder(t.Context(), "203")
	require.NoError(t, err)
}

func TestGetFlatOrderList(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetFiatOrderList(t.Context(), "", "", currency.EMPTYCODE, currency.EMPTYCODE, endTime, startTime, 0, 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err := e.GetFiatOrderList(t.Context(), "BUY", "", currency.EMPTYCODE, currency.USDT, startTime, endTime, 1, 10)
	require.NoError(t, err, "GetFiatOrderList must not error")
	if mockTests {
		require.NotEmpty(t, result.List, "GetFiatOrderList must return the mocked order")
		assert.Equal(t, time.Date(2025, time.February, 11, 7, 45, 6, 0, time.UTC), result.List[0].Time.Time(), "order time should decode the API timestamp")
		assert.Equal(t, "US Dollar", result.List[0].FiatCurrencyInfo.Name, "fiat currency information should decode")
		assert.Equal(t, "Tether", result.List[0].CryptoCurrencyInfo.Name, "crypto currency information should decode")
		assert.Equal(t, "promo-203", result.List[0].PromotionCode, "promotion code should decode")
	}
}

func TestGetStablecoinOrderList(t *testing.T) {
	t.Parallel()
	startTime, endTime := getTime()
	_, err := e.GetStablecoinOrderList(t.Context(), currency.EMPTYCODE, "", endTime, startTime, 0, 0)
	require.ErrorIs(t, err, common.ErrStartAfterEnd)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err := e.GetStablecoinOrderList(t.Context(), currency.USDT, "", startTime, endTime, 1, 10)
	require.NoError(t, err, "GetStablecoinOrderList must not error")
	if mockTests {
		require.NotEmpty(t, result.List, "GetStablecoinOrderList must return the mocked order")
		assert.Equal(t, time.Date(2025, time.September, 9, 10, 0, 0, 0, time.UTC), result.List[0].CreateTime.Time(), "create time should decode the API timestamp")
		assert.Equal(t, int64(1757392800), result.List[0].CreateTimestamp.Time().Unix(), "creation timestamp should decode from Unix seconds")
		assert.Equal(t, types.Number(0.999), result.List[0].ReciprocalRate, "reciprocal rate should match")
	}
}

func TestGetFlatOrderDetail(t *testing.T) {
	t.Parallel()
	_, err := e.GetFiatOrderDetail(t.Context(), "")
	require.ErrorIs(t, err, order.ErrOrderIDNotSet)

	if !mockTests {
		sharedtestvalues.SkipTestIfCredentialsUnset(t, e)
	}
	result, err := e.GetFiatOrderDetail(t.Context(), "203")
	require.NoError(t, err, "GetFiatOrderDetail must not error")
	if mockTests {
		assert.Equal(t, time.Date(2025, time.September, 9, 10, 0, 0, 0, time.UTC), result.CreateTime.Time(), "create time should decode the API timestamp")
		assert.Equal(t, "Gate Bank", result.GateBankName, "Gate bank name should match")
		assert.Equal(t, "remark-203", result.TransferRemark, "transfer remark should match")
		assert.Equal(t, "reference-203", result.ReferenceCode, "reference code should match")
		assert.Equal(t, "reference-203", result.GateReferenceCode, "Gate reference code should match")
	}
}
