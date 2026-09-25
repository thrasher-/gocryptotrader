package bitstamp

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// websocketEventRequest holds a subscribe or unsubscribe request
type websocketEventRequest struct {
	Event string        `json:"event"`
	Data  websocketData `json:"data"`
}

// websocketData holds the channel of a subscription request and the token required by private channels
type websocketData struct {
	Channel string `json:"channel"`
	Auth    string `json:"auth,omitempty"`
}

// websocketError holds an error reported by the websocket server
type websocketError struct {
	Code    uint64 `json:"code"`
	Message string `json:"message"`
}

// websocketOrderbook holds a snapshot of the top 100 orderbook levels
type websocketOrderbook struct {
	Asks           orderbook.LevelsArrayPriceAmount `json:"asks"`
	Bids           orderbook.LevelsArrayPriceAmount `json:"bids"`
	Timestamp      types.Time                       `json:"timestamp"`
	Microtimestamp types.Time                       `json:"microtimestamp"`
}

// websocketTrade holds a public trade
type websocketTrade struct {
	ID             uint64       `json:"id"`
	Amount         types.Number `json:"amount_str"`
	Price          types.Number `json:"price_str"`
	Side           orderSide    `json:"type"`
	Timestamp      types.Time   `json:"timestamp"`
	Microtimestamp types.Time   `json:"microtimestamp"`
	BuyOrderID     uint64       `json:"buy_order_id"`
	SellOrderID    uint64       `json:"sell_order_id"`
}

// websocketOrder holds an event for one of the account's orders
// Amount is the amount remaining, and AmountTraded is the amount executed by the event rather than in total
type websocketOrder struct {
	ID              uint64       `json:"id"`
	ClientOrderID   string       `json:"client_order_id"`
	Amount          types.Number `json:"amount_str"`
	AmountTraded    types.Number `json:"amount_traded"`
	AmountAtCreate  types.Number `json:"amount_at_create"`
	Price           types.Number `json:"price_str"`
	Side            orderSide    `json:"order_type"`
	OrderSubtype    uint8        `json:"order_subtype"`
	DateTime        types.Time   `json:"datetime"`
	Microtimestamp  types.Time   `json:"microtimestamp"`
	TradeAccountID  uint64       `json:"trade_account_id"`
	IsLiquidation   bool         `json:"is_liquidation"`
	ReduceOnly      bool         `json:"reduce_only"`
	StopPrice       types.Number `json:"stop_price"`
	ActivationPrice types.Number `json:"activation_price"`
	TrailingDelta   types.Number `json:"trailing_delta"`
	OriginalOrderID uint64       `json:"orig_order_id"`
}

// websocketMyTrade holds an execution of one of the account's orders
type websocketMyTrade struct {
	ID             uint64       `json:"id"`
	OrderID        uint64       `json:"order_id"`
	ClientOrderID  string       `json:"client_order_id"`
	Amount         types.Number `json:"amount"`
	Price          types.Number `json:"price"`
	Fee            types.Number `json:"fee"`
	Side           string       `json:"side"`
	Microtimestamp types.Time   `json:"microtimestamp"`
	TradeAccountID uint64       `json:"trade_account_id"`
}

// websocketFundingRate holds a funding rate update for a perpetual market
type websocketFundingRate struct {
	Market          string       `json:"market"`
	MarkPrice       types.Number `json:"mark_price"`
	IndexPrice      types.Number `json:"index_price"`
	FundingRate     types.Number `json:"funding_rate"`
	Timestamp       types.Time   `json:"timestamp"`
	NextFundingTime types.Time   `json:"next_funding_time"`
}

// WebsocketSettlement holds a position settlement or insurance fund transfer from the private settlements channel
// Insurance fund events only populate the position ID for isolated positions, and use Amount instead of the
// settlement amount fields
type WebsocketSettlement struct {
	Event                     string       `json:"-"`
	PositionID                string       `json:"id"`
	TransactionID             string       `json:"tnx_id"`
	Timestamp                 types.Time   `json:"ts"`
	Price                     types.Number `json:"price"`
	Currency                  string       `json:"ccy"`
	AmountSettled             types.Number `json:"as"`
	AmountFromPrice           types.Number `json:"ap"`
	AmountFromTradingFees     types.Number `json:"atf"`
	AmountFromLiquidationFees types.Number `json:"alf"`
	AmountFromFunding         types.Number `json:"afu"`
	AmountFromSocialisedLoss  types.Number `json:"asl"`
	StrikePrice               types.Number `json:"sp"`
	SettlementType            string       `json:"typ"`
	Amount                    types.Number `json:"amount"`
}

// WebsocketLiquidationAlert holds a margin or liquidation alert from the private liquidations channel
type WebsocketLiquidationAlert struct {
	PositionID             string       `json:"position_id"`
	TradeAccountID         uint64       `json:"trade_account_id"`
	Microtimestamp         types.Time   `json:"microtimestamp"`
	AlertType              string       `json:"alert_type"`
	MarginMode             string       `json:"margin_mode"`
	InitialMarginRatio     types.Number `json:"initial_margin_ratio"`
	MaintenanceMarginRatio types.Number `json:"maintenance_margin_ratio"`
}

// WebsocketTokenSettlement holds a notification that the tokens bought by a tokenised security order are withdrawable
type WebsocketTokenSettlement struct {
	Action                string       `json:"action"`
	OrderID               uint64       `json:"order_id"`
	Instrument            string       `json:"instrument"`
	TokenSecurityCurrency string       `json:"token_security_currency"`
	TokensQuantity        types.Number `json:"tokens_quantity"`
	DateTime              time.Time    `json:"datetime"`
}

// WebsocketAnnouncement holds a tokenised security lifecycle announcement such as a trading halt or corporate action
// The details are only populated for their announcement type; the corporate action and multiplier update details are
// left undecoded as their fields are not documented
type WebsocketAnnouncement struct {
	Event                   string              `json:"-"`
	AnnouncementID          uint64              `json:"announcement_id"`
	AnnouncementType        string              `json:"announcement_type"`
	Status                  string              `json:"status"`
	PublishedAt             time.Time           `json:"published_at"`
	UpdatedAt               time.Time           `json:"updated_at"`
	ProductType             string              `json:"product_type"`
	Product                 []string            `json:"product"`
	TradingHaltDetails      *TradingHaltDetails `json:"trading_halt_details"`
	CorporateActionDetails  json.RawMessage     `json:"corporate_action_details"`
	MultiplierUpdateDetails json.RawMessage     `json:"multiplier_update_details"`
	ChangedFields           []string            `json:"changed_fields"`
}

// TradingHaltDetails holds the reason and period of a trading halt; EndTime is zero while the halt is ongoing
type TradingHaltDetails struct {
	Reason    string    `json:"reason"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}
