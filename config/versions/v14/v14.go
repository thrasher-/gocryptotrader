package v14

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/buger/jsonparser"
)

const myAccountChannel = "myAccount"

var (
	// ErrCannotDowngrade prevents v2-only Kraken configuration from being loaded by a v1 WebSocket implementation.
	ErrCannotDowngrade = errors.New("kraken WebSocket v2 configuration cannot be downgraded")
	// ErrCannotUpgrade prevents an ambiguous custom WebSocket endpoint from being used with the v2 implementation.
	ErrCannotUpgrade = errors.New("kraken custom WebSocket endpoint does not identify v2")
)

var legacyPrivateChannels = map[string]bool{
	"myOrders":   true,
	"myTrades":   true,
	"openOrders": true,
	"ownTrades":  true,
}

// Version is an ExchangeVersion that migrates Kraken Spot WebSocket configuration to v2.
type Version struct{}

// Exchanges returns Kraken as the only exchange upgraded by this version.
func (*Version) Exchanges() []string { return []string{"Kraken"} }

// UpgradeExchange removes v1 default URLs and combines the old private channels into executions.
func (*Version) UpgradeExchange(_ context.Context, exchangeConfig []byte) ([]byte, error) {
	upgraded := bytes.Clone(exchangeConfig)
	for _, endpoint := range []struct {
		key   string
		v1URL string
	}{
		{key: "WebsocketSpotURL", v1URL: "wss://ws.kraken.com"},
		{key: "WebsocketSpotSupplementaryURL", v1URL: "wss://ws-auth.kraken.com"},
	} {
		endpointURL, err := jsonparser.GetString(upgraded, "api", "urlEndpoints", endpoint.key)
		switch {
		case errors.Is(err, jsonparser.KeyPathNotFoundError):
			continue
		case err != nil:
			return exchangeConfig, err
		case endpointURL == "", endpointURL == endpoint.v1URL, strings.TrimSuffix(endpointURL, "/") == endpoint.v1URL:
			upgraded = jsonparser.Delete(upgraded, "api", "urlEndpoints", endpoint.key)
			continue
		}

		parsed, err := url.Parse(endpointURL)
		if err != nil || !strings.HasSuffix(strings.TrimSuffix(parsed.Path, "/"), "/v2") {
			return exchangeConfig, fmt.Errorf("%s %q: %w", endpoint.key, endpointURL, ErrCannotUpgrade)
		}
	}

	upgraded, err := upgradeSubscriptions(upgraded)
	if err != nil {
		return exchangeConfig, err
	}
	return upgraded, nil
}

func upgradeSubscriptions(exchangeConfig []byte) ([]byte, error) {
	rawSubscriptions, valueType, _, err := jsonparser.Get(exchangeConfig, "features", "subscriptions")
	switch {
	case errors.Is(err, jsonparser.KeyPathNotFoundError):
		return exchangeConfig, nil
	case err != nil:
		return exchangeConfig, err
	case valueType != jsonparser.Array:
		return exchangeConfig, fmt.Errorf("subscriptions: %w (%q)", jsonparser.UnknownValueTypeError, valueType)
	}

	type entry struct {
		raw           []byte
		channel       string
		enabled       bool
		authenticated bool
	}
	entries := []entry{}
	var parseErr error
	_, arrayErr := jsonparser.ArrayEach(rawSubscriptions, func(value []byte, _ jsonparser.ValueType, _ int, _ error) {
		if parseErr != nil {
			return
		}
		channel, channelErr := jsonparser.GetString(value, "channel")
		if channelErr != nil && !errors.Is(channelErr, jsonparser.KeyPathNotFoundError) {
			parseErr = channelErr
			return
		}
		current := channel == myAccountChannel
		if !current && !legacyPrivateChannels[channel] {
			entries = append(entries, entry{raw: bytes.Clone(value), channel: channel})
			return
		}
		enabled, enabledErr := jsonparser.GetBoolean(value, "enabled")
		if enabledErr != nil && !errors.Is(enabledErr, jsonparser.KeyPathNotFoundError) {
			parseErr = enabledErr
			return
		}
		authenticated, authenticatedErr := jsonparser.GetBoolean(value, "authenticated")
		if authenticatedErr != nil && !errors.Is(authenticatedErr, jsonparser.KeyPathNotFoundError) {
			parseErr = authenticatedErr
			return
		}
		entries = append(entries, entry{
			raw:           bytes.Clone(value),
			channel:       channel,
			enabled:       enabled,
			authenticated: authenticated,
		})
	})
	if parseErr != nil {
		return exchangeConfig, parseErr
	}
	if arrayErr != nil {
		return exchangeConfig, arrayErr
	}

	hasLegacy := false
	currentIndex := -1
	firstLegacyIndex := -1
	enabled := false
	authenticated := false
	for i := range entries {
		switch {
		case entries[i].channel == myAccountChannel:
			if currentIndex == -1 {
				currentIndex = i
			}
		case legacyPrivateChannels[entries[i].channel]:
			hasLegacy = true
			if firstLegacyIndex == -1 {
				firstLegacyIndex = i
			}
		default:
			continue
		}
		enabled = enabled || entries[i].enabled
		authenticated = authenticated || entries[i].authenticated
	}
	if !hasLegacy {
		return exchangeConfig, nil
	}
	if currentIndex == -1 {
		currentIndex = firstLegacyIndex
	}

	upgraded := make([][]byte, 0, len(entries))
	for i := range entries {
		privateChannel := entries[i].channel == myAccountChannel || legacyPrivateChannels[entries[i].channel]
		if !privateChannel {
			upgraded = append(upgraded, entries[i].raw)
			continue
		}
		if i != currentIndex {
			continue
		}
		updated := entries[i].raw
		if entries[i].channel != myAccountChannel {
			// The channel path was successfully parsed above, so replacing it cannot fail.
			updated, _ = jsonparser.Set(updated, []byte(`"`+myAccountChannel+`"`), "channel")
		}
		if enabled {
			updated, _ = jsonparser.Set(updated, []byte("true"), "enabled")
		}
		if authenticated {
			updated, _ = jsonparser.Set(updated, []byte("true"), "authenticated")
		}
		upgraded = append(upgraded, updated)
	}

	// The subscriptions path was successfully parsed above, so replacing it cannot fail.
	updatedConfig, _ := jsonparser.Set(exchangeConfig, append(append([]byte{'['}, bytes.Join(upgraded, []byte{','})...), ']'), "features", "subscriptions")
	return updatedConfig, nil
}

// DowngradeExchange rejects an incompatible Kraken WebSocket v1 downgrade.
func (*Version) DowngradeExchange(_ context.Context, exchangeConfig []byte) ([]byte, error) {
	return exchangeConfig, ErrCannotDowngrade
}
