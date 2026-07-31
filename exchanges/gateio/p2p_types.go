package gateio

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// P2PMerchantInfo holds P2P merchant account information.
type P2PMerchantInfo struct {
	IsSelf                    bool                         `json:"is_self"`
	UserTimestamp             string                       `json:"user_timest"`
	CounterpartiesNumber      uint64                       `json:"counterparties_num"`
	EmailVerified             string                       `json:"email_verified"`
	Verified                  string                       `json:"verified"`
	HasPhone                  string                       `json:"has_phone"`
	UserName                  string                       `json:"user_name"`
	UserNote                  string                       `json:"user_note"`
	BusinessUID               string                       `json:"biz_uid"`
	HaveTraded                uint64                       `json:"have_traded"`
	CompleteTransactions      string                       `json:"complete_transactions"`
	PaidTransactions          string                       `json:"paid_transactions"`
	AcceptedTransactions      string                       `json:"accepted_transactions"`
	TransactionsUsedTime      string                       `json:"transactions_used_time"`
	CancelledUsedTimeMonth    string                       `json:"cancelled_used_time_month"`
	CompleteTransactionsMonth string                       `json:"complete_transactions_month"`
	CompleteRateMonth         types.Number                 `json:"complete_rate_month"`
	OrdersBuyRateMonth        types.Number                 `json:"orders_buy_rate_month"`
	IsBlack                   uint64                       `json:"is_black"`
	IsFollow                  uint64                       `json:"is_follow"`
	BlueVIP                   uint64                       `json:"blue_vip"`
	WorkStatus                uint64                       `json:"work_status"`
	RegistrationDays          uint64                       `json:"registration_days"`
	FirstTradeDays            uint64                       `json:"first_trade_days"`
	NeedReplenish             uint64                       `json:"need_replenish"`
	MerchantInfo              *P2PMerchantMarketInfo       `json:"merchant_info"`
	OnlineStatus              uint64                       `json:"online_status"`
	WorkHours                 *SetMerchantWorkHoursRequest `json:"work_hours"`
	TransactionsMonth         types.Number                 `json:"transactions_month"`
	TransactionsAll           types.Number                 `json:"transactions_all"`
	TradeVersatile            bool                         `json:"trade_versatile"`
}

// P2PMerchantMarketInfo holds a market in which the merchant can place advertisements.
type P2PMerchantMarketInfo struct {
	Type   string `json:"type"`
	Market string `json:"market"`
}

// GetCounterpartyInfoRequest holds the request parameters for getting counterparty info.
type GetCounterpartyInfoRequest struct {
	BusinessUID string `json:"biz_uid"`
}

// P2PCounterpartyInfo holds P2P counterparty user information.
type P2PCounterpartyInfo struct {
	UserTimestamp             string       `json:"user_timest"`
	EmailVerified             string       `json:"email_verified"`
	Verified                  string       `json:"verified"`
	HasPhone                  string       `json:"has_phone"`
	UserName                  string       `json:"user_name"`
	UserNote                  string       `json:"user_note"`
	CompleteTransactions      string       `json:"complete_transactions"`
	PaidTransactions          string       `json:"paid_transactions"`
	AcceptedTransactions      string       `json:"accepted_transactions"`
	TransactionsUsedTime      string       `json:"transactions_used_time"`
	CancelledUsedTimeMonth    string       `json:"cancelled_used_time_month"`
	CompleteTransactionsMonth string       `json:"complete_transactions_month"`
	CompleteRateMonth         types.Number `json:"complete_rate_month"`
	IsFollow                  uint64       `json:"is_follow"`
	HaveTraded                uint64       `json:"have_traded"`
	BusinessUID               string       `json:"biz_uid"`
	RegistrationDays          uint64       `json:"registration_days"`
	FirstTradeDays            uint64       `json:"first_trade_days"`
	TradeVersatile            bool         `json:"trade_versatile"`
}

// GetP2PPaymentMethodsRequest holds the request parameters for getting payment methods.
type GetP2PPaymentMethodsRequest struct {
	Fiat string `json:"fiat,omitempty"`
}

// P2PPaymentMethodGroup holds a group of payment methods of the same type.
type P2PPaymentMethodGroup struct {
	PaymentType string              `json:"pay_type"`
	PaymentName string              `json:"pay_name"`
	IDs         []uint64            `json:"ids"`
	List        []*P2PPaymentMethod `json:"list"`
}

// P2PPaymentMethod holds a single bound payment method account.
type P2PPaymentMethod struct {
	UID                   uint64 `json:"uid"`
	ID                    string `json:"id"`
	BankID                string `json:"bankid"`
	BankName              string `json:"bankname"`
	BankBranch            string `json:"bankbranch"`
	BankAddress           string `json:"bankaddr"`
	BankCity              string `json:"bankcity"`
	BankProvince          string `json:"bankprov"`
	BankDescription       string `json:"bankdesc"`
	RealName              string `json:"real_name"`
	AccountDescription    string `json:"account_des"`
	BankHolderUID         uint64 `json:"hold_uid"`
	BankHolderUsername    string `json:"hold_username"`
	PaymentMethodType     string `json:"pay_type"`
	PaymentMethodFileLink string `json:"file"`
	PaymentMethodFileKey  string `json:"file_key"`
	Account               string `json:"account"`
	Memo                  string `json:"memo"`
	Code                  string `json:"code"`
	MemoExtended          string `json:"memo_ext"`
	TradeTips             string `json:"trade_tips"`
	Version               string `json:"version"`
	Nickname              uint64 `json:"nickname"`
}

// SetMerchantWorkHoursRequest represents request parameters to sent merchant working hour
type SetMerchantWorkHoursRequest struct {
	WorkStatus uint64 `json:"work_status"`
	CycleType  string `json:"cycle_type"`
	DayOfWeek  string `json:"day_of_week"`
	TimeZone   string `json:"time_zone"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
}

// WorkStatusResponse represents a response payload after setting working time
type WorkStatusResponse struct {
	WorkStatus uint64 `json:"work_status"`
}

// PendingP2POrderRequest represents a p2p order request parameter
type PendingP2POrderRequest struct {
	CryptoCurrency currency.Code `json:"crypto_currency"`
	FiatCurrency   currency.Code `json:"fiat_currency"`
	OrderTab       string        `json:"order_tab,omitempty"`
	SelectType     string        `json:"select_type,omitempty"`
	Status         string        `json:"status,omitempty"`
	TransactionID  uint64        `json:"txid,omitempty"`
	StartTime      time.Time     `json:"-"`
	EndTime        time.Time     `json:"-"`
}

type pendingP2POrderRequestPayload struct {
	CryptoCurrency currency.Code `json:"crypto_currency"`
	FiatCurrency   currency.Code `json:"fiat_currency"`
	OrderTab       string        `json:"order_tab,omitempty"`
	SelectType     string        `json:"select_type,omitempty"`
	Status         string        `json:"status,omitempty"`
	TransactionID  uint64        `json:"txid,omitempty"`
	StartTime      uint64        `json:"start_time,omitempty"`
	EndTime        uint64        `json:"end_time,omitempty"`
}

func (p *PendingP2POrderRequest) payload() *pendingP2POrderRequestPayload {
	return &pendingP2POrderRequestPayload{
		CryptoCurrency: p.CryptoCurrency,
		FiatCurrency:   p.FiatCurrency,
		OrderTab:       p.OrderTab,
		SelectType:     p.SelectType,
		Status:         p.Status,
		TransactionID:  p.TransactionID,
		StartTime:      p2pUnixSeconds(p.StartTime),
		EndTime:        p2pUnixSeconds(p.EndTime),
	}
}

// P2POrderList represents a P2P transactions detail list
type P2POrderList struct {
	List                  []*P2POrderInfo            `json:"list"`
	TransactionCountdowns []*P2PTransactionCountdown `json:"trans_time"`
	Count                 uint64                     `json:"count"`
	ExportedNumber        uint64                     `json:"exported_num"`
}

// P2PTransactionCountdown holds the number of seconds remaining for a P2P transaction.
type P2PTransactionCountdown struct {
	CountdownSeconds uint64 `json:"od_time"`
}

// P2POrderInfo represents a P2P order detail instance
type P2POrderInfo struct {
	TypeBuy                      uint64               `json:"type_buy"`
	FormattedTimestamp           string               `json:"timest"`
	FormattedExpirationTimestamp string               `json:"timest_expire"`
	Timestamp                    types.Time           `json:"timestamp"`
	Rate                         types.Number         `json:"rate"`
	Amount                       types.Number         `json:"amount"`
	Total                        types.Number         `json:"total"`
	TransactionID                uint64               `json:"txid"`
	Status                       string               `json:"status"`
	CounterpartyRealName         string               `json:"its_realname"`
	CounterpartyUID              string               `json:"its_uid"`
	CounterpartyNickname         string               `json:"its_nick"`
	SellerRealName               string               `json:"seller_realname"`
	BuyerRealName                string               `json:"buyer_realname"`
	Cancelable                   uint64               `json:"cancelable"`
	CurrencyType                 string               `json:"currency_type"`
	WantType                     string               `json:"want_type"`
	HidePayment                  uint64               `json:"hide_payment"`
	SelectedPaymentType          string               `json:"sel_paytype"`
	CountdownSeconds             uint64               `json:"cd_time"`
	OrderType                    uint64               `json:"order_type"`
	OrderTag                     []string             `json:"order_tag"`
	ConvertInfo                  *P2PConvertInfo      `json:"convert_info"`
	OtherPaymentMethods          []OtherPaymentMethod `json:"pay_others"`
	IsSelf                       uint64               `json:"is_self"`
	BusinessID                   uint64               `json:"bizid"`
	LastPayTime                  types.Time           `json:"last_pay_time"`
	Type                         string               `json:"type"`
	TotalFiat                    string               `json:"totalfat"`
	DisputeTime                  types.Time           `json:"dispute_time"`
	TradeType                    string               `json:"trade_type"`
	TradeNote                    string               `json:"trade_note"`
	BankName                     string               `json:"bankname"`
	BankBranch                   string               `json:"bankbranch"`
}

// P2POrderDetail represents a P2P order detail
type P2POrderDetail struct {
	IsSell                  uint64               `json:"is_sell"`
	TransactionID           uint64               `json:"txid"`
	OrderID                 uint64               `json:"orderid"`
	Timestamp               types.Time           `json:"timest"`
	LastPayTime             types.Time           `json:"last_pay_time"`
	RemainingPaymentSeconds int64                `json:"remain_pay_time"` // Values at or below zero indicate that payment is overdue.
	CurrencyType            string               `json:"currency_type"`
	WantType                string               `json:"want_type"`
	Symbol                  currency.Pair        `json:"symbol"`
	Rate                    types.Number         `json:"rate"`
	Amount                  types.Number         `json:"amount"`
	Total                   types.Number         `json:"total"`
	Status                  string               `json:"status"`
	ReasonID                string               `json:"reason_id"`
	ReasonDescription       string               `json:"reason_desc"`
	CancelTime              types.Time           `json:"cancel_time"`
	InAppeal                uint64               `json:"in_appeal"`
	DisputeTime             types.Time           `json:"dispute_time"`
	Cancelable              uint64               `json:"cancelable"`
	HidePayment             uint64               `json:"hide_payment"`
	TradeTips               string               `json:"trade_tips"`
	ShowBank                string               `json:"show_bank"`
	BankName                string               `json:"bankname"`
	BankBranch              string               `json:"bankbranch"`
	BankID                  string               `json:"bankid"`
	BankHolderRealName      string               `json:"bank_holder_realname"`
	ShowAlipayDetail        string               `json:"show_ali"`
	AlipayName              string               `json:"aliname"`
	IsAlipayCode            uint64               `json:"is_alicode"`
	ShowWechat              string               `json:"show_wechat"`
	WeChatName              string               `json:"wename"`
	ShowOthers              string               `json:"show_others"`
	OtherPaymentMethods     []OtherPaymentMethod `json:"pay_others"`
	SelectedPaymentType     string               `json:"sel_paytype"`
	CounterpartyUID         string               `json:"its_uid"`
	CounterpartyNickname    string               `json:"its_nickname"`
	CounterpartyRealName    string               `json:"its_realname"`
	HaveTraded              uint64               `json:"have_traded"`
	AppealAllowCancel       uint64               `json:"appeal_allow_cancel"`
	AppealVerdictHasOpen    string               `json:"appeal_verdict_has_open"`
	IMUnread                uint64               `json:"im_unread"`
	PaymentVoucherURL       []string             `json:"payment_voucher_url"`
	PaidTimestamp           types.Time           `json:"timest_paid"`
	OwnRealName             string               `json:"own_realname"`
	OrderType               uint64               `json:"order_type"`
	IsShowReceive           uint64               `json:"is_show_receive"`
	ShowSellerContactInfo   bool                 `json:"show_seller_contact_info"`
	SupportedPayTypes       []string             `json:"supported_pay_types"`
}

// OtherPaymentMethod represents other payment methods detail
type OtherPaymentMethod struct {
	ID                         string `json:"id"`
	PaymentAccountDescriptions string `json:"account_des"`
	PaymentType                string `json:"pay_type"`
	PaymentName                string `json:"pay_name"`
	Account                    string `json:"account"`
	Memo                       string `json:"memo"`
	TradeTips                  string `json:"trade_tips"`
}

// P2PConvertInfo represents a P2P order transaction convert info
type P2PConvertInfo struct {
	ConvertType         string       `json:"convert_type"`
	ConvertStatus       string       `json:"convert_status"`
	ExpectedPriceRate   types.Number `json:"pre_rate"`
	ExecutionRate       types.Number `json:"rate"`
	ExpectedFiatPrice   types.Number `json:"pre_fiat_rate"`
	FiatRate            types.Number `json:"fiat_rate"`
	Amount              types.Number `json:"amount"`
	SwapAmount          types.Number `json:"convert_amount"`
	SlippageCalculation types.Number `json:"slippage"`
	Status              string       `json:"status"`
}

// P2PCompletedOrderRequest holds request parameters to retrieve completed p2p orders
type P2PCompletedOrderRequest struct {
	CryptoCurrency currency.Code `json:"crypto_currency"`
	FiatCurrency   currency.Code `json:"fiat_currency"`
	SelectType     string        `json:"select_type,omitempty"`
	Status         string        `json:"status,omitempty"`
	TransactionID  uint64        `json:"txid,omitempty"`
	StartTime      time.Time     `json:"-"`
	EndTime        time.Time     `json:"-"`
	QueryDispute   uint64        `json:"query_dispute,omitempty"`
	Page           uint64        `json:"page,omitempty"`
	PerPage        uint64        `json:"per_page,omitempty"`
}

type p2pCompletedOrderRequestPayload struct {
	CryptoCurrency currency.Code `json:"crypto_currency"`
	FiatCurrency   currency.Code `json:"fiat_currency"`
	SelectType     string        `json:"select_type,omitempty"`
	Status         string        `json:"status,omitempty"`
	TransactionID  uint64        `json:"txid,omitempty"`
	StartTime      uint64        `json:"start_time,omitempty"`
	EndTime        uint64        `json:"end_time,omitempty"`
	QueryDispute   uint64        `json:"query_dispute,omitempty"`
	Page           uint64        `json:"page,omitempty"`
	PerPage        uint64        `json:"per_page,omitempty"`
}

func (p *P2PCompletedOrderRequest) payload() *p2pCompletedOrderRequestPayload {
	return &p2pCompletedOrderRequestPayload{
		CryptoCurrency: p.CryptoCurrency,
		FiatCurrency:   p.FiatCurrency,
		SelectType:     p.SelectType,
		Status:         p.Status,
		TransactionID:  p.TransactionID,
		StartTime:      p2pUnixSeconds(p.StartTime),
		EndTime:        p2pUnixSeconds(p.EndTime),
		QueryDispute:   p.QueryDispute,
		Page:           p.Page,
		PerPage:        p.PerPage,
	}
}

func p2pUnixSeconds(timestamp time.Time) uint64 {
	if timestamp.IsZero() {
		return 0
	}
	seconds := timestamp.Unix()
	if seconds <= 0 {
		return 0
	}
	return uint64(seconds)
}

// GetP2POrderDetailsRequest holds request parameters for querying P2P order details.
type GetP2POrderDetailsRequest struct {
	TransactionID uint64 `json:"txid"`
	Channel       string `json:"channel,omitempty"`
}

// ConfirmP2PPaymentRequest holds request parameters for confirming P2P payment.
type ConfirmP2PPaymentRequest struct {
	TransactionID string `json:"txid"`
	PaymentMethod string `json:"payment_method,omitempty"`
}

// ConfirmP2PReceiptRequest holds request parameters for confirming P2P receipt.
type ConfirmP2PReceiptRequest struct {
	TransactionID string `json:"txid"`
}

// CancelP2POrderRequest holds request parameters for cancelling a P2P order.
// ReasonID values: 1=no longer want to buy, 2=cannot reach seller, 3=will not pay,
// 4=seller account not real, 5=seller payout account issue, 6=price mismatch,
// 7=mutually agreed cancel, 8=poor communication, 9=other,
// 10=seller cannot release or refund, 11=terms not met, 12=seller payout risk-controlled.
type CancelP2POrderRequest struct {
	TransactionID string `json:"txid"`
	ReasonID      string `json:"reason_id,omitempty"`
	ReasonMemo    string `json:"reason_memo,omitempty"`
}

// PublishP2PAdRequest holds request parameters for publishing a P2P advertisement.
type PublishP2PAdRequest struct {
	CurrencyType          currency.Code `json:"currencyType"`
	ExchangeType          string        `json:"exchangeType"`
	Type                  string        `json:"type"`
	UnitPrice             types.Number  `json:"unitPrice"`
	Number                types.Number  `json:"number"`
	PaymentType           string        `json:"payType"`
	PaymentTypeJSON       string        `json:"pay_type_json,omitempty"`
	FixedRate             string        `json:"rateFixed,omitempty"`
	OrderID               string        `json:"oid,omitempty"`
	MinAmount             types.Number  `json:"minAmount,omitempty"`
	MaxAmount             types.Number  `json:"maxAmount,omitempty"`
	LimitBasis            uint64        `json:"limitBasis,omitempty"`
	FiatMinAmount         types.Number  `json:"fiatMinAmount,omitempty"`
	FiatMaxAmount         types.Number  `json:"fiatMaxAmount,omitempty"`
	TierLimit             string        `json:"tierLimit,omitempty"`
	VerifiedLimit         string        `json:"verifiedLimit,omitempty"`
	RegistrationTimeLimit string        `json:"regTimeLimit,omitempty"`
	AdvertisersLimit      string        `json:"advertisersLimit,omitempty"`
	PolymarketLimit       uint64        `json:"polymarket_limit,omitempty"`
	ExpireMinutes         string        `json:"expire_min,omitempty"`
	TradeTips             string        `json:"trade_tips,omitempty"`
	AutoReply             string        `json:"auto_reply,omitempty"`
	MinCompletedLimit     string        `json:"min_completed_limit,omitempty"`
	MaxCompletedLimit     string        `json:"max_completed_limit,omitempty"`
	CompletedRateLimit    string        `json:"completed_rate_limit,omitempty"`
	UserCountryLimit      string        `json:"user_country_limit,omitempty"`
	UserOrderLimit        string        `json:"user_order_limit,omitempty"`
	RateReferenceID       string        `json:"rateReferenceId,omitempty"`
	RateOffset            string        `json:"rateOffset,omitempty"`
	FloatTrend            string        `json:"float_trend,omitempty"`
	TeamPaymentUID        string        `json:"team_payment_uid,omitempty"`
}

// UpdateP2PAdStatusRequest holds request parameters for updating P2P ad status.
// AdvertisementStatus: 1=listed, 3=delisted, 4=closed.
type UpdateP2PAdStatusRequest struct {
	AdvertisementNumber uint64 `json:"adv_no"`
	AdvertisementStatus uint64 `json:"adv_status"`
}

// P2PUpdateAdStatusResult holds the result of updating a P2P ad status.
type P2PUpdateAdStatusResult struct {
	Status uint64 `json:"status"`
}

// GetP2PAdDetailsRequest holds request parameters for querying P2P ad details.
type GetP2PAdDetailsRequest struct {
	AdvertisementNumber string `json:"adv_no"`
}

// P2PAdDetail holds detailed P2P advertisement information.
type P2PAdDetail struct {
	Rate                  types.Number  `json:"rate"`
	Type                  string        `json:"type"`
	Amount                types.Number  `json:"amount"`
	MinAmount             types.Number  `json:"min_amount"`
	MaxAmount             types.Number  `json:"max_amount"`
	FiatMinAmount         types.Number  `json:"fiat_min_amount"`
	FiatMaxAmount         types.Number  `json:"fiat_max_amount"`
	LimitBasis            uint64        `json:"limit_basis"`
	LimitBasisText        string        `json:"limit_basis_text"`
	Total                 types.Number  `json:"total"`
	AlipaySupported       uint64        `json:"pay_ali"`
	BankSupported         uint64        `json:"pay_bank"`
	PayPalSupported       uint64        `json:"pay_paypal"`
	WeChatSupported       uint64        `json:"pay_wechat"`
	PaymentTypeNumber     string        `json:"pay_type_num"`
	PaymentTypeJSON       string        `json:"pay_type_json"`
	LockedAmount          types.Number  `json:"locked_amount"`
	OrderID               uint64        `json:"orderid"`
	Timestamp             types.Time    `json:"timestamp"`
	CurrencyType          currency.Code `json:"currency_type"`
	WantType              string        `json:"want_type"`
	HideRate              types.Number  `json:"hide_rate"`
	TradeTips             string        `json:"trade_tips"`
	AutoReply             string        `json:"auto_reply"`
	RateReferenceID       int64         `json:"rate_ref_id"`
	RateOffset            types.Number  `json:"rate_offset"`
	Status                string        `json:"status"`
	FixedRate             uint64        `json:"rate_fixed"`
	FloatTrend            uint64        `json:"float_trend"`
	ExpireMinutes         uint64        `json:"expire_min"`
	TierLimit             uint64        `json:"tier_limit"`
	RegistrationTimeLimit uint64        `json:"reg_time_limit"`
	AdvertisersLimit      uint64        `json:"advertisers_limit"`
	PolymarketLimit       uint64        `json:"polymarket_limit"`
	MinCompletedLimit     int64         `json:"min_completed_limit"`
	MaxCompletedLimit     int64         `json:"max_completed_limit"`
	UserOrdersLimit       int64         `json:"user_orders_limit"`
	CompletedRateLimit    types.Number  `json:"completed_rate_limit"`
	LimitCountryCN        string        `json:"limit_country_cn"`
	LimitCountryEN        string        `json:"limit_country_en"`
	IsHedge               uint64        `json:"is_hedge"`
	HidePayment           uint64        `json:"hide_payment"`
}

// GetMyP2PAdsRequest holds request parameters for getting the current user's P2P ads.
type GetMyP2PAdsRequest struct {
	Asset     currency.Code `json:"asset"`
	FiatUnit  string        `json:"fiat_unit,omitempty"`
	TradeType string        `json:"trade_type,omitempty"`
}

// P2PMyAdsData wraps the list of the user's own P2P ads.
type P2PMyAdsData struct {
	Lists []*P2PMyAdItem `json:"lists"`
}

// P2PMyAdItem holds a single item from the user's P2P ad list.
type P2PMyAdItem struct {
	Type                  string        `json:"type"`
	Rate                  types.Number  `json:"rate"`
	OriginalRate          types.Number  `json:"original_rate"`
	Amount                types.Number  `json:"amount"`
	Total                 types.Number  `json:"total"`
	LimitTotal            string        `json:"limit_total"`
	LimitFiat             string        `json:"limit_fiat"`
	MinAmount             types.Number  `json:"min_amount"`
	MaxAmount             types.Number  `json:"max_amount"`
	PaymentTypeNumber     string        `json:"pay_type_num"`
	PaymentTypeJSON       string        `json:"pay_type_json"`
	ExpireMinutes         string        `json:"expire_min"`
	TierLimit             string        `json:"tier_limit"`
	AdvertisersLimit      uint64        `json:"advertisers_limit"`
	RegistrationTimeLimit uint64        `json:"reg_time_limit"`
	VerifiedLimit         uint64        `json:"verified_limit"`
	MinCompletedLimit     int64         `json:"min_completed_limit"`
	MaxCompletedLimit     int64         `json:"max_completed_limit"`
	UserCountryLimit      int64         `json:"user_country_limit"`
	CompletedRateLimit    types.Number  `json:"completed_rate_limit"`
	UserOrdersLimit       int64         `json:"user_orders_limit"`
	HidePayment           string        `json:"hide_payment"`
	CurrencyType          currency.Code `json:"currencyType"`
	WantType              string        `json:"want_type"`
	TradeTips             string        `json:"trade_tips"`
	NewHand               uint64        `json:"new_hand"`
	ID                    string        `json:"id"`
	Status                string        `json:"status"`
	LockedAmount          types.Number  `json:"locked_amount"`
	HideRate              types.Number  `json:"hide_rate"`
	IsOutTime             uint64        `json:"is_out_time"`
	RateReferenceID       int64         `json:"rate_ref_id"`
	RateOffset            types.Number  `json:"rate_offset"`
	FixedRate             uint64        `json:"rate_fixed"`
	FloatTrend            uint64        `json:"float_trend"`
	InDispute             uint64        `json:"in_dispute"`
	AutoReply             string        `json:"auto_reply"`
	Timestamp             types.Time    `json:"timestamp"`
	IsHedge               uint64        `json:"is_hedge"`
}

// GetP2PAdsListRequest holds request parameters for getting the public P2P ads list.
type GetP2PAdsListRequest struct {
	Asset     currency.Code `json:"asset"`
	FiatUnit  string        `json:"fiat_unit"`
	TradeType string        `json:"trade_type"`
}

// P2PAdListItem holds a single item from the public P2P ads list.
type P2PAdListItem struct {
	Index                          uint64              `json:"index"`
	Asset                          currency.Code       `json:"asset"`
	FiatUnit                       string              `json:"fiat_unit"`
	Price                          types.Number        `json:"price"`
	SurplusAmount                  types.Number        `json:"surplus_amount"`
	MaximumSingleTransactionAmount types.Number        `json:"max_single_trans_amount"`
	MinimumSingleTransactionAmount types.Number        `json:"min_single_trans_amount"`
	FiatMinAmount                  types.Number        `json:"fiat_min_amount"`
	FiatMaxAmount                  types.Number        `json:"fiat_max_amount"`
	LimitBasis                     uint64              `json:"limit_basis"`
	LimitBasisText                 string              `json:"limit_basis_text"`
	TradeMethods                   []*P2PAdTradeMethod `json:"trade_methods"`
	Nickname                       string              `json:"nick_name"`
	AdvertisementNumber            uint64              `json:"adv_no"`
}

// P2PAdTradeMethod holds a payment method accepted by a P2P advertisement.
type P2PAdTradeMethod struct {
	IconURLColor    string `json:"icon_url_color"`
	Identifier      string `json:"identifier"`
	PaymentID       string `json:"pay_id"`
	PaymentType     string `json:"pay_type"`
	TradeMethodName string `json:"trade_method_name"`
}

// P2PChatMessagesResponse holds a single P2P chat message.
type P2PChatMessagesResponse struct {
	Messages      []*P2PMessageDetail `json:"messages"`
	Memo          string              `json:"memo"`
	HasHistory    bool                `json:"has_history"`
	TransactionID uint64              `json:"txid"`
	ServerTime    uint64              `json:"SRVTM"`
	OrderStatus   string              `json:"order_status"`
}

// P2PMessageDetail represents a P2P conversation message detail
type P2PMessageDetail struct {
	IsSell        uint64         `json:"is_sell,omitempty"`
	MessageType   uint64         `json:"msg_type,omitempty"`
	Message       string         `json:"msg"`
	Username      string         `json:"username"`
	Timestamp     types.Time     `json:"timest"`
	RiskType      uint64         `json:"risk_type,omitempty"`
	ToastMessage  string         `json:"toast_msg,omitempty"`
	UID           string         `json:"uid,omitempty"`
	Type          uint64         `json:"type,omitempty"`
	MessageObject *MessageObject `json:"msg_obj,omitempty"`
	Picture       string         `json:"pic,omitempty"`
	FileKey       string         `json:"file_key,omitempty"`
	FileType      string         `json:"file_type,omitempty"`
}

// MessageObject represents
type MessageObject struct {
	ID                 string     `json:"id"`
	Status             string     `json:"status"`
	Text               string     `json:"text"`
	ReasonID           uint64     `json:"reason_id"`
	ToastID            uint64     `json:"toast_id"`
	ReasonMemo         string     `json:"reason_memo"`
	CancelTime         types.Time `json:"cancel_time"`
	SellerConfirm      uint64     `json:"seller_confirm"`
	PaymentVoucher     []any      `json:"payment_voucher"`
	AccountDescription string     `json:"account_des"`
	PaymentType        string     `json:"pay_type"`
	File               string     `json:"file"`
	FileKey            string     `json:"file_key"`
	Account            string     `json:"account"`
	Memo               string     `json:"memo"`
	Code               string     `json:"code"`
	MemoExtended       string     `json:"memo_ext"`
	TradeTips          string     `json:"trade_tips"`
	RealName           string     `json:"real_name"`
	IsDelete           uint64     `json:"is_delete"`
	PaymentName        string     `json:"pay_name"`
}

// P2PChatSenderInfo holds basic info about the sender of a P2P chat message.
type P2PChatSenderInfo struct {
	UserName    string `json:"user_name"`
	BusinessUID string `json:"biz_uid"`
}

// SendP2PChatMessageRequest holds request parameters for sending a P2P chat message.
// Type: 0=text (default), 1=file (image or video).
type SendP2PChatMessageRequest struct {
	TransactionID uint64 `json:"txid"`
	Type          uint64 `json:"type,omitempty"`
	Message       string `json:"message"`
}

// P2PSendMessageResult holds the result of sending a P2P chat message.
type P2PSendMessageResult struct {
	ServerTime     types.Time `json:"SRVTM"`
	TransactionID  uint64     `json:"txid"`
	ConversationID string     `json:"conversation_id"`
	MessageType    uint64     `json:"msg_type"`
	RiskType       uint64     `json:"risk_type"`
	ToastMessage   string     `json:"toast_msg"`
}

// UploadP2PChatFileRequest holds request parameters for uploading a P2P chat file.
type UploadP2PChatFileRequest struct {
	ImageContentType string `json:"image_content_type"`
	Base64Image      string `json:"base64_img"`
}

// P2PUploadFileResult holds the result of uploading a P2P chat file.
type P2PUploadFileResult struct {
	FileKey string `json:"file_key"`
}
