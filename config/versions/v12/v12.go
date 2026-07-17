package v12

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/buger/jsonparser"
)

const myAccountChannel = "myAccount"

var legacyPrivateChannels = map[string]bool{
	"myOrders":   true,
	"myTrades":   true,
	"openOrders": true,
	"ownTrades":  true,
}

// Version is an ExchangeVersion to migrate Kraken Spot WebSocket configuration to v2.
type Version struct{}

// Exchanges returns just Kraken.
func (*Version) Exchanges() []string { return []string{"Kraken"} }

// UpgradeExchange removes v1 default URLs and combines the old private channels into executions.
func (*Version) UpgradeExchange(_ context.Context, e []byte) ([]byte, error) {
	for key, oldURL := range map[string]string{
		"WebsocketSpotURL":              "wss://ws.kraken.com",
		"WebsocketSpotSupplementaryURL": "wss://ws-auth.kraken.com",
	} {
		url, err := jsonparser.GetString(e, "api", "urlEndpoints", key)
		if err != nil && !errors.Is(err, jsonparser.KeyPathNotFoundError) {
			return e, err
		}
		if url == oldURL {
			e = jsonparser.Delete(e, "api", "urlEndpoints", key)
		}
	}

	return upgradeSubscriptions(e)
}

func upgradeSubscriptions(e []byte) ([]byte, error) {
	rawSubscriptions, valueType, _, err := jsonparser.Get(e, "features", "subscriptions")
	switch {
	case errors.Is(err, jsonparser.KeyPathNotFoundError):
		return e, nil
	case err != nil:
		return e, err
	case valueType != jsonparser.Array:
		return e, fmt.Errorf("subscriptions: %w (%q)", jsonparser.UnknownValueTypeError, valueType)
	}

	type entry struct {
		raw     []byte
		channel string
	}
	entries := []entry{}
	var parseErr error
	_, err = jsonparser.ArrayEach(rawSubscriptions, func(value []byte, _ jsonparser.ValueType, _ int, callbackErr error) {
		if parseErr != nil {
			return
		}
		if callbackErr != nil {
			parseErr = callbackErr
			return
		}
		channel, channelErr := jsonparser.GetString(value, "channel")
		if channelErr != nil && !errors.Is(channelErr, jsonparser.KeyPathNotFoundError) {
			parseErr = channelErr
			return
		}
		entries = append(entries, entry{raw: bytes.Clone(value), channel: channel})
	})
	if err != nil {
		return e, err
	}
	if parseErr != nil {
		return e, parseErr
	}

	hasCurrent := false
	hasLegacy := false
	for i := range entries {
		hasCurrent = hasCurrent || entries[i].channel == myAccountChannel
		hasLegacy = hasLegacy || legacyPrivateChannels[entries[i].channel]
	}
	if !hasLegacy {
		return e, nil
	}

	upgraded := make([][]byte, 0, len(entries))
	insertedCurrent := hasCurrent
	for i := range entries {
		if !legacyPrivateChannels[entries[i].channel] {
			upgraded = append(upgraded, entries[i].raw)
			continue
		}
		if insertedCurrent {
			continue
		}
		updated, setErr := jsonparser.Set(entries[i].raw, []byte(`"`+myAccountChannel+`"`), "channel")
		if setErr != nil {
			return e, setErr
		}
		upgraded = append(upgraded, updated)
		insertedCurrent = true
	}

	return jsonparser.Set(e, append(append([]byte{'['}, bytes.Join(upgraded, []byte{','})...), ']'), "features", "subscriptions")
}

// DowngradeExchange is a no-op because Kraken v1 is superseded.
func (*Version) DowngradeExchange(_ context.Context, e []byte) ([]byte, error) {
	return e, nil
}
