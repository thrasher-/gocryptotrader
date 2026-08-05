package kraken

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

// AddOrder places a standard Spot order with all current request parameters.
func (e *Exchange) AddOrder(ctx context.Context, req *AddOrderRequest) (*AddOrderResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.OrderType == "" {
		return nil, errOrderTypeInvalid
	}
	if !isValidSpotEnum(req.OrderType, "market", "limit", "iceberg", "stop-loss", "take-profit", "stop-loss-limit", "take-profit-limit", "trailing-stop", "trailing-stop-limit", "settle-position") {
		return nil, errOrderTypeInvalid
	}
	if req.Side == "" || !isValidSpotEnum(req.Side, "buy", "sell") {
		return nil, errOrderSideInvalid
	}
	if !req.Pair.IsPopulated() {
		return nil, errPairRequired
	}
	if req.UserReference != nil && req.ClientOrderID != "" {
		return nil, errOrderIdentityConflict
	}
	if !isValidSpotEnum(req.AssetClass, "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	if !isValidSpotEnum(req.Trigger, "index", "last") {
		return nil, errTriggerInvalid
	}
	if !isValidSpotEnum(req.SelfTradePolicy, "cancel-newest", "cancel-oldest", "cancel-both") {
		return nil, errSelfTradeInvalid
	}
	if !isValidSpotEnum(req.TimeInForce, "GTC", "IOC", "GTD", "FOK") {
		return nil, errTimeInForceInvalid
	}
	if req.Close != nil && (req.Close.OrderType == "" || !isValidSpotEnum(req.Close.OrderType, "limit", "iceberg", "stop-loss", "take-profit", "stop-loss-limit", "take-profit-limit", "trailing-stop", "trailing-stop-limit")) {
		return nil, errBatchCloseTypeInvalid
	}
	deadline, err := formatDeadline(req.Deadline, time.Now())
	if err != nil {
		return nil, err
	}
	if req.Volume < 0 {
		return nil, errVolumeInvalid
	}
	volume, err := formatSpotFloat(req.Volume)
	if err != nil {
		return nil, err
	}
	displayVolume := ""
	if req.DisplayVolume != nil {
		if *req.DisplayVolume < 0 {
			return nil, errVolumeInvalid
		}
		displayVolume, err = formatSpotFloat(*req.DisplayVolume)
		if err != nil {
			return nil, err
		}
	}
	price, err := formatOrderPrice(req.Price)
	if err != nil {
		return nil, err
	}
	secondaryPrice, err := formatOrderPrice(req.SecondaryPrice)
	if err != nil {
		return nil, err
	}
	startTime, err := formatScheduledTime(req.StartTime, req.StartDelay, time.Second)
	if err != nil {
		return nil, err
	}
	expireTime, err := formatScheduledTime(req.ExpireTime, req.ExpireAfter, 5*time.Second)
	if err != nil {
		return nil, err
	}
	orderFlags, err := formatOrderFlags(req.OrderFlags)
	if err != nil {
		return nil, err
	}
	pair, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}
	closePrice, closeSecondaryPrice := "", ""
	if req.Close != nil {
		closePrice, err = formatOrderPrice(req.Close.Price)
		if err != nil {
			return nil, err
		}
		closeSecondaryPrice, err = formatOrderPrice(req.Close.SecondaryPrice)
		if err != nil {
			return nil, err
		}
	}

	params := url.Values{
		"ordertype": {string(req.OrderType)},
		"type":      {string(req.Side)},
		"volume":    {volume},
		"pair":      {pair},
	}
	if req.UserReference != nil {
		params.Set("userref", strconv.FormatInt(int64(*req.UserReference), 10))
	}
	if req.ClientOrderID != "" {
		params.Set("cl_ord_id", req.ClientOrderID)
	}
	if req.DisplayVolume != nil {
		params.Set("displayvol", displayVolume)
	}
	if req.AssetClass != "" {
		params.Set("asset_class", string(req.AssetClass))
	}
	if price != "" {
		params.Set("price", price)
	}
	if secondaryPrice != "" {
		params.Set("price2", secondaryPrice)
	}
	if req.Trigger != "" {
		params.Set("trigger", string(req.Trigger))
	}
	if req.Leverage != 0 {
		params.Set("leverage", strconv.FormatUint(uint64(req.Leverage), 10))
	}
	if req.ReduceOnly {
		params.Set("reduce_only", strconv.FormatBool(req.ReduceOnly))
	}
	if req.SelfTradePolicy != "" {
		params.Set("stptype", string(req.SelfTradePolicy))
	}
	if orderFlags != "" {
		params.Set("oflags", orderFlags)
	}
	if req.TimeInForce != "" {
		params.Set("timeinforce", string(req.TimeInForce))
	}
	if startTime != "" {
		params.Set("starttm", startTime)
	}
	if expireTime != "" {
		params.Set("expiretm", expireTime)
	}
	if req.Close != nil {
		params.Set("close[ordertype]", string(req.Close.OrderType))
		if closePrice != "" {
			params.Set("close[price]", closePrice)
		}
		if closeSecondaryPrice != "" {
			params.Set("close[price2]", closeSecondaryPrice)
		}
	}
	if deadline != "" {
		params.Set("deadline", deadline)
	}
	if req.Validate {
		params.Set("validate", strconv.FormatBool(req.Validate))
	}
	if req.Broker != "" {
		params.Set("broker", req.Broker)
	}

	var result AddOrderResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AddOrder", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CancelExistingOrder cancels an order by Kraken ID, user reference, or client order ID.
func (e *Exchange) CancelExistingOrder(ctx context.Context, req *CancelOrderRequest) (*CancelOrderResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	identifierCount := 0
	if req.TransactionID != "" {
		identifierCount++
	}
	if req.UserReference != nil {
		identifierCount++
	}
	if req.ClientOrderID != "" {
		identifierCount++
	}
	if identifierCount == 0 {
		return nil, errOrderIdentityRequired
	}
	if identifierCount > 1 {
		return nil, errOrderIdentityConflict
	}

	params := url.Values{}
	if req.TransactionID != "" {
		params.Set("txid", req.TransactionID)
	}
	if req.UserReference != nil {
		params.Set("txid", strconv.FormatInt(int64(*req.UserReference), 10))
	}
	if req.ClientOrderID != "" {
		params.Set("cl_ord_id", req.ClientOrderID)
	}

	var result CancelOrderResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CancelOrder", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AmendOrder amends an open order while retaining its queue priority where possible.
func (e *Exchange) AmendOrder(ctx context.Context, req *AmendOrderRequest) (*AmendOrderResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.TransactionID == "" && req.ClientOrderID == "" {
		return nil, errOrderIdentityRequired
	}
	if req.TransactionID != "" && req.ClientOrderID != "" {
		return nil, errOrderIdentityConflict
	}
	deadline, err := formatDeadline(req.Deadline, time.Now())
	if err != nil {
		return nil, err
	}
	orderQuantity := ""
	if req.OrderQuantity != nil {
		if *req.OrderQuantity < 0 {
			return nil, errVolumeInvalid
		}
		var err error
		orderQuantity, err = formatSpotFloat(*req.OrderQuantity)
		if err != nil {
			return nil, err
		}
	}
	displayQuantity := ""
	if req.DisplayQuantity != nil {
		if *req.DisplayQuantity < 0 {
			return nil, errVolumeInvalid
		}
		var err error
		displayQuantity, err = formatSpotFloat(*req.DisplayQuantity)
		if err != nil {
			return nil, err
		}
	}
	if req.LimitPrice != nil && req.LimitPrice.Expression != "" && req.LimitPrice.Expression[0] == '#' {
		return nil, errOrderPriceInvalid
	}
	limitPrice, err := formatOrderPrice(req.LimitPrice)
	if err != nil {
		return nil, err
	}
	if req.TriggerPrice != nil && req.TriggerPrice.Expression != "" && req.TriggerPrice.Expression[0] == '#' {
		return nil, errOrderPriceInvalid
	}
	triggerPrice, err := formatOrderPrice(req.TriggerPrice)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	if req.TransactionID != "" {
		params.Set("txid", req.TransactionID)
	}
	if req.ClientOrderID != "" {
		params.Set("cl_ord_id", req.ClientOrderID)
	}
	if req.OrderQuantity != nil {
		params.Set("order_qty", orderQuantity)
	}
	if req.DisplayQuantity != nil {
		params.Set("display_qty", displayQuantity)
	}
	if limitPrice != "" {
		params.Set("limit_price", limitPrice)
	}
	if triggerPrice != "" {
		params.Set("trigger_price", triggerPrice)
	}
	if !req.Pair.IsEmpty() {
		if !req.Pair.IsPopulated() {
			return nil, errPairRequired
		}
		pair, err := e.FormatSymbol(req.Pair, asset.Spot)
		if err != nil {
			return nil, err
		}
		params.Set("pair", pair)
	}
	if req.PostOnly {
		params.Set("post_only", strconv.FormatBool(req.PostOnly))
	}
	if deadline != "" {
		params.Set("deadline", deadline)
	}

	var result AmendOrderResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AmendOrder", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CancelAllOpenOrders cancels all open Spot orders.
func (e *Exchange) CancelAllOpenOrders(ctx context.Context) (*CancelOrderResponse, error) {
	var result CancelOrderResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CancelAll", url.Values{}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CancelAllOrdersAfter configures Kraken's Spot dead-man switch. A timeout of zero disables it.
func (e *Exchange) CancelAllOrdersAfter(ctx context.Context, req *CancelAllOrdersAfterRequest) (*CancelAllOrdersAfterResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if req.Timeout < 0 || req.Timeout%time.Second != 0 {
		return nil, errTimeoutInvalid
	}
	if req.Timeout >= 24*time.Hour {
		return nil, errTimeoutTooLarge
	}

	params := url.Values{"timeout": {strconv.FormatInt(int64(req.Timeout/time.Second), 10)}}
	var result CancelAllOrdersAfterResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CancelAllOrdersAfter", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type addOrderBatchWireClose struct {
	OrderType      string `json:"ordertype"`
	Price          string `json:"price,omitempty"`
	SecondaryPrice string `json:"price2,omitempty"`
}

type addOrderBatchWireOrder struct {
	UserReference   *int32                  `json:"userref,omitempty"`
	ClientOrderID   string                  `json:"cl_ord_id,omitempty"`
	OrderType       string                  `json:"ordertype"`
	OrderSide       string                  `json:"type"`
	Volume          string                  `json:"volume"`
	DisplayVolume   string                  `json:"displayvol,omitempty"`
	Price           string                  `json:"price,omitempty"`
	SecondaryPrice  string                  `json:"price2,omitempty"`
	Trigger         string                  `json:"trigger,omitempty"`
	Leverage        string                  `json:"leverage,omitempty"`
	ReduceOnly      bool                    `json:"reduce_only,omitempty"`
	SelfTradePolicy string                  `json:"stptype,omitempty"`
	OrderFlags      string                  `json:"oflags,omitempty"`
	TimeInForce     string                  `json:"timeinforce,omitempty"`
	StartTime       string                  `json:"starttm,omitempty"`
	ExpireTime      string                  `json:"expiretm,omitempty"`
	Close           *addOrderBatchWireClose `json:"close,omitempty"`
}

// AddOrderBatch places between two and fifteen orders for one Spot pair.
func (e *Exchange) AddOrderBatch(ctx context.Context, req *AddOrderBatchRequest) (*AddOrderBatchResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	if len(req.Orders) < 2 || len(req.Orders) > 15 {
		return nil, errBatchOrderCount
	}
	if !req.Pair.IsPopulated() {
		return nil, errPairRequired
	}
	if !isValidSpotEnum(req.AssetClass, "tokenized_asset") {
		return nil, errAssetClassInvalid
	}
	deadline, err := formatDeadline(req.Deadline, time.Now())
	if err != nil {
		return nil, err
	}
	wireOrders := make([]addOrderBatchWireOrder, len(req.Orders))
	for i := range req.Orders {
		order := &req.Orders[i]
		if order.OrderType == "" || order.OrderSide == "" {
			return nil, fmt.Errorf("order %d: %w", i, errBatchOrderFields)
		}
		if !isValidSpotEnum(order.OrderType, "market", "limit", "iceberg", "stop-loss", "take-profit", "stop-loss-limit", "take-profit-limit", "trailing-stop", "trailing-stop-limit", "settle-position") {
			return nil, fmt.Errorf("order %d: %w", i, errBatchOrderTypeInvalid)
		}
		if !isValidSpotEnum(order.OrderSide, "buy", "sell") {
			return nil, fmt.Errorf("order %d: %w", i, errBatchSideInvalid)
		}
		if !isValidSpotEnum(order.Trigger, "index", "last") {
			return nil, fmt.Errorf("order %d: %w", i, errBatchTriggerInvalid)
		}
		if !isValidSpotEnum(order.SelfTradePolicy, "cancel-newest", "cancel-oldest", "cancel-both") {
			return nil, fmt.Errorf("order %d: %w", i, errBatchSelfTradeInvalid)
		}
		if !isValidSpotEnum(order.TimeInForce, "GTC", "IOC", "GTD") {
			return nil, fmt.Errorf("order %d: %w", i, errBatchTimeInForceInvalid)
		}
		if order.UserReference != nil && order.ClientOrderID != "" {
			return nil, fmt.Errorf("order %d: %w", i, errBatchIdentityConflict)
		}
		if order.Close != nil && (order.Close.OrderType == "" || !isValidSpotEnum(order.Close.OrderType, "limit", "iceberg", "stop-loss", "take-profit", "stop-loss-limit", "take-profit-limit", "trailing-stop", "trailing-stop-limit")) {
			return nil, fmt.Errorf("order %d: %w", i, errBatchCloseTypeInvalid)
		}
		if order.Volume < 0 {
			return nil, fmt.Errorf("order %d: %w", i, errVolumeInvalid)
		}
		volume, err := formatSpotFloat(order.Volume)
		if err != nil {
			return nil, fmt.Errorf("order %d: %w", i, err)
		}
		displayVolume := ""
		if order.DisplayVolume != nil {
			if *order.DisplayVolume < 0 {
				return nil, fmt.Errorf("order %d: %w", i, errVolumeInvalid)
			}
			displayVolume, err = formatSpotFloat(*order.DisplayVolume)
			if err != nil {
				return nil, fmt.Errorf("order %d: %w", i, err)
			}
		}
		price, err := formatOrderPrice(order.Price)
		if err != nil {
			return nil, fmt.Errorf("order %d: %w", i, err)
		}
		secondaryPrice, err := formatOrderPrice(order.SecondaryPrice)
		if err != nil {
			return nil, fmt.Errorf("order %d: %w", i, err)
		}
		orderFlags, err := formatOrderFlags(order.OrderFlags)
		if err != nil {
			return nil, fmt.Errorf("order %d: %w", i, err)
		}
		startTime, err := formatScheduledTime(order.StartTime, order.StartDelay, time.Second)
		if err != nil {
			return nil, fmt.Errorf("order %d: %w", i, err)
		}
		expireTime, err := formatScheduledTime(order.ExpireTime, order.ExpireAfter, 5*time.Second)
		if err != nil {
			return nil, fmt.Errorf("order %d: %w", i, err)
		}
		wireOrder := addOrderBatchWireOrder{
			UserReference:   order.UserReference,
			ClientOrderID:   order.ClientOrderID,
			OrderType:       string(order.OrderType),
			OrderSide:       string(order.OrderSide),
			Volume:          volume,
			DisplayVolume:   displayVolume,
			Price:           price,
			SecondaryPrice:  secondaryPrice,
			Trigger:         string(order.Trigger),
			ReduceOnly:      order.ReduceOnly,
			SelfTradePolicy: string(order.SelfTradePolicy),
			OrderFlags:      orderFlags,
			TimeInForce:     string(order.TimeInForce),
			StartTime:       startTime,
			ExpireTime:      expireTime,
		}
		if order.Leverage != 0 {
			wireOrder.Leverage = strconv.FormatUint(uint64(order.Leverage), 10)
		}
		if order.Close != nil {
			closePrice, err := formatOrderPrice(order.Close.Price)
			if err != nil {
				return nil, fmt.Errorf("order %d: %w", i, err)
			}
			closeSecondaryPrice, err := formatOrderPrice(order.Close.SecondaryPrice)
			if err != nil {
				return nil, fmt.Errorf("order %d: %w", i, err)
			}
			wireOrder.Close = &addOrderBatchWireClose{OrderType: string(order.Close.OrderType), Price: closePrice, SecondaryPrice: closeSecondaryPrice}
		}
		wireOrders[i] = wireOrder
	}

	pair, err := e.FormatSymbol(req.Pair, asset.Spot)
	if err != nil {
		return nil, err
	}
	encodedOrders, _ := json.Marshal(wireOrders) // The wire request contains only JSON-supported primitive and struct fields.
	params := url.Values{
		"orders": {string(encodedOrders)},
		"pair":   {pair},
	}
	if req.AssetClass != "" {
		params.Set("asset_class", string(req.AssetClass))
	}
	if deadline != "" {
		params.Set("deadline", deadline)
	}
	if req.Validate {
		params.Set("validate", strconv.FormatBool(req.Validate))
	}
	if req.Broker != "" {
		params.Set("broker", req.Broker)
	}

	var result AddOrderBatchResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "AddOrderBatch", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CancelOrderBatch cancels up to fifty Spot orders by transaction ID, user reference, or client order ID.
func (e *Exchange) CancelOrderBatch(ctx context.Context, req *CancelOrderBatchRequest) (*CancelOrderBatchResponse, error) {
	if req == nil {
		return nil, common.ErrNilPointer
	}
	identifierCount := len(req.TransactionIDs) + len(req.UserReferences) + len(req.ClientOrderIDs)
	if identifierCount == 0 || identifierCount > 50 {
		return nil, errBatchCancelOrderCount
	}
	if slices.Contains(req.TransactionIDs, "") {
		return nil, errOrderIDRequired
	}
	if slices.Contains(req.ClientOrderIDs, "") {
		return nil, errOrderIDRequired
	}

	params := url.Values{}
	if len(req.TransactionIDs)+len(req.UserReferences) > 0 {
		orders := make([]any, 0, len(req.TransactionIDs)+len(req.UserReferences))
		for i := range req.TransactionIDs {
			orders = append(orders, req.TransactionIDs[i])
		}
		for i := range req.UserReferences {
			orders = append(orders, req.UserReferences[i])
		}
		encodedOrders, _ := json.Marshal(orders) // Identifiers are restricted to strings and integers.
		params.Set("orders", string(encodedOrders))
	}
	if len(req.ClientOrderIDs) > 0 {
		encodedClientOrderIDs, _ := json.Marshal(req.ClientOrderIDs) // Client order identifiers are strings.
		params.Set("cl_ord_ids", string(encodedClientOrderIDs))
	}

	var result CancelOrderBatchResponse
	if err := e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "CancelOrderBatch", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
