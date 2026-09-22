package binance

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
)

var errAccessTokenRequired = errors.New("OAuth access token required")

// FastAPIRequest uses a user-authorised OAuth token, independently of an exchange API key.
type FastAPIRequest struct{ AccessToken string }

// ReferralCustomersRequest supplies the optional Futures referral pagination.
type ReferralCustomersRequest struct {
	Page       uint64 `json:"page,omitempty"`
	Limit      uint64 `json:"limit,omitempty"`
	RecvWindow uint64 `json:"recvWindow,omitempty"`
}

// SendOAuthHTTPRequest sends the Fast API's bearer token and form body without
// attaching the unrelated exchange API key, timestamp or HMAC signature.
func (e *Exchange) SendOAuthHTTPRequest(ctx context.Context, method, path, accessToken string, params url.Values, result any) error {
	if accessToken == "" {
		return errAccessTokenRequired
	}
	endpoint, err := e.API.Endpoints.GetURL(exchange.RestSpot)
	if err != nil {
		return err
	}
	headers := map[string]string{"Authorization": "Bearer " + accessToken}
	if method == http.MethodPost {
		headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	var response json.RawMessage
	requestErr := e.SendPayload(ctx, request.Auth, func() (*request.Item, error) {
		item := &request.Item{Method: method, Path: endpoint + path, Headers: headers, Result: &response, Verbose: e.Verbose, HTTPDebugging: e.HTTPDebugging, HTTPRecording: e.HTTPRecording, HTTPMockDataSliceLimit: e.HTTPMockDataSliceLimit}
		if method == http.MethodPost {
			item.Body = strings.NewReader(params.Encode())
		}
		return item, nil
	}, request.AuthenticatedRequest)
	if err := apiResponseError(response); err != nil {
		return err
	}
	if requestErr != nil {
		return requestErr
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(response, result)
}
