package hyperliquid

import (
	"maps"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

// UserFillEvent represents a single fill update from authenticated websocket channels.
type UserFillEvent struct {
	User          string
	Pair          currency.Pair
	Asset         asset.Item
	Price         float64
	Amount        float64
	Side          order.Side
	Fee           float64
	FeeAsset      *currency.Code
	Hash          string
	OrderID       int64
	TradeID       *int64
	Crossed       bool
	Direction     string
	StartPosition float64
	ClosedPnl     float64
	Timestamp     time.Time
	Raw           UserFill
}

// UserEventsUpdate aggregates user event notifications such as fills.
type UserEventsUpdate struct {
	User  string
	Fills []UserFillEvent
}

// UserFillsUpdate captures user fill snapshots or incremental updates.
type UserFillsUpdate struct {
	User       string
	IsSnapshot bool
	Fills      []UserFillEvent
}

// OrderUpdateEvent represents an authenticated order status update.
type OrderUpdateEvent struct {
	User   string
	Detail order.Detail
	Raw    OrderStatusEntry
}

// UserFundingEvent conveys funding ledger updates for a user.
type UserFundingEvent struct {
	User      string
	Entry     UserFundingHistoryEntry
	Timestamp time.Time
}

// UserLedgerUpdateEvent describes non-funding ledger updates for a user.
type UserLedgerUpdateEvent struct {
	User      string
	Entry     UserNonFundingLedgerEntry
	Timestamp time.Time
}

// WebData2Event exposes raw payloads from the webData2 websocket channel.
type WebData2Event struct {
	User string
	Data map[string]any
}

func (e *Exchange) handleUserEvents(payload *wsUserEventsData) error {
	if payload == nil {
		return errUserEventsPayloadMissing
	}
	if len(payload.Fills) == 0 {
		return nil
	}
	user := e.defaultUserAddress()
	events := make([]UserFillEvent, 0, len(payload.Fills))
	for i := range payload.Fills {
		fill := payload.Fills[i]
		evt, err := e.convertUserFill(user, &fill)
		if err != nil {
			return err
		}
		events = append(events, evt)
	}
	if len(events) == 0 {
		return nil
	}
	e.Websocket.DataHandler <- UserEventsUpdate{User: user, Fills: events}
	return nil
}

func (e *Exchange) handleUserFills(payload *wsUserFillsPayload) error {
	if payload == nil {
		return errUserFillsPayloadMissing
	}
	user := strings.ToLower(payload.User)
	if user == "" {
		user = e.defaultUserAddress()
	}
	events := make([]UserFillEvent, 0, len(payload.Fills))
	for i := range payload.Fills {
		fill := payload.Fills[i]
		evt, err := e.convertUserFill(user, &fill)
		if err != nil {
			return err
		}
		events = append(events, evt)
	}
	e.Websocket.DataHandler <- UserFillsUpdate{User: user, IsSnapshot: payload.IsSnapshot, Fills: events}
	return nil
}

func (e *Exchange) handleOrderUpdates(payload *wsOrderUpdatesPayload) error {
	if payload == nil {
		return errOrderUpdatesPayloadMissing
	}
	user := strings.ToLower(payload.User)
	if user == "" {
		user = e.defaultUserAddress()
	}
	for i := range payload.Statuses {
		entry := payload.Statuses[i]
		detail, err := e.orderStatusToDetail(&entry)
		if err != nil {
			return err
		}
		detail.Exchange = e.Name
		e.Websocket.DataHandler <- OrderUpdateEvent{User: user, Detail: detail, Raw: entry}
	}
	return nil
}

func (e *Exchange) handleUserFunding(payload *wsUserFundingPayload) error {
	if payload == nil {
		return errUserFundingPayloadMissing
	}
	user := strings.ToLower(payload.User)
	if user == "" {
		user = e.defaultUserAddress()
	}
	event := UserFundingEvent{
		User:      user,
		Entry:     payload.UserFundingHistoryEntry,
		Timestamp: payload.Time.Time().UTC(),
	}
	e.Websocket.DataHandler <- event
	return nil
}

func (e *Exchange) handleUserLedgerUpdates(payload *wsUserNonFundingLedgerPayload) error {
	if payload == nil {
		return errUserLedgerPayloadMissing
	}
	user := strings.ToLower(payload.User)
	if user == "" {
		user = e.defaultUserAddress()
	}
	event := UserLedgerUpdateEvent{
		User:      user,
		Entry:     payload.UserNonFundingLedgerEntry,
		Timestamp: payload.Time.Time().UTC(),
	}
	e.Websocket.DataHandler <- event
	return nil
}

func (e *Exchange) handleWebData2(payload *wsWebData2Payload) error {
	if payload == nil {
		return errWebDataPayloadMissing
	}
	user := strings.ToLower(payload.User)
	if user == "" {
		user = e.defaultUserAddress()
	}
	data := make(map[string]any, len(payload.Data))
	maps.Copy(data, payload.Data)
	e.Websocket.DataHandler <- WebData2Event{User: user, Data: data}
	return nil
}

func (e *Exchange) convertUserFill(user string, fill *UserFill) (UserFillEvent, error) {
	if fill == nil {
		return UserFillEvent{}, errUserFillMissing
	}
	assetType, pair, err := parseMarketString(fill.Coin)
	if err != nil {
		return UserFillEvent{}, err
	}
	price := fill.Price.Float64()
	amount := fill.Size.Float64()
	fee := fill.Fee.Float64()
	var feeAsset *currency.Code
	if fill.FeeToken != nil && *fill.FeeToken != "" {
		code := currency.NewCode(strings.ToUpper(*fill.FeeToken))
		feeAsset = &code
	}
	event := UserFillEvent{
		User:          user,
		Pair:          pair,
		Asset:         assetType,
		Price:         price,
		Amount:        amount,
		Side:          convertTradeSideToOrder(fill.Side),
		Fee:           fee,
		FeeAsset:      feeAsset,
		Hash:          fill.Hash,
		OrderID:       fill.OrderID,
		TradeID:       fill.TradeID,
		Crossed:       fill.Crossed,
		Direction:     strings.ToLower(fill.Direction),
		StartPosition: fill.StartPosition.Float64(),
		ClosedPnl:     fill.ClosedPnl.Float64(),
		Timestamp:     fill.Time.Time().UTC(),
		Raw:           *fill,
	}
	return event, nil
}

func (e *Exchange) orderStatusToDetail(entry *OrderStatusEntry) (order.Detail, error) {
	if entry == nil {
		return order.Detail{}, errOrderStatusEntryMissing
	}
	if entry.Order == nil {
		return order.Detail{}, errOrderStatusMissingOrderData
	}
	orderInfo := entry.Order
	assetType, pair, err := parseMarketString(orderInfo.Coin)
	if err != nil {
		return order.Detail{}, err
	}
	price := orderInfo.LimitPrice.Float64()
	size := orderInfo.Size.Float64()
	orig := orderInfo.OrigSize.Float64()
	if orig == 0 {
		orig = size
	}
	executed := math.Max(orig-size, 0)
	remaining := math.Max(size, 0)
	detail := order.Detail{
		Exchange:           e.Name,
		OrderID:            strconv.FormatInt(orderInfo.OrderID, 10),
		ClientOrderID:      derefString(orderInfo.ClientOID),
		Pair:               pair,
		AssetType:          assetType,
		Type:               parseOrderTypeHL(orderInfo.OrderType),
		Side:               convertTradeSideToOrder(orderInfo.Side),
		Status:             mapOrderStatusFromString(entry.Status),
		Date:               orderInfo.Timestamp.Time().UTC(),
		LastUpdated:        entry.StatusTimestamp.Time().UTC(),
		Price:              price,
		Amount:             orig,
		ExecutedAmount:     executed,
		RemainingAmount:    remaining,
		ReduceOnly:         orderInfo.ReduceOnly,
		TimeInForce:        parseTimeInForceHL(orderInfo.TimeInForce),
		Cost:               price * executed,
		CostAsset:          pair.Quote,
		SettlementCurrency: pair.Quote,
	}
	if orderInfo.TriggerPx != nil {
		detail.TriggerPrice = orderInfo.TriggerPx.Float64()
	}
	return detail, nil
}

func (e *Exchange) defaultUserAddress() string {
	if addr := strings.ToLower(e.accountAddr); addr != "" {
		return addr
	}
	addr, err := e.accountAddressLower()
	if err != nil {
		return ""
	}
	return addr
}

func derefString(val *string) string {
	if val == nil {
		return ""
	}
	return *val
}
