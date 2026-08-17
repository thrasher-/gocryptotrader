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

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/common/crypto"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/nonce"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/log"
)

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

// GetWebsocketToken returns the websocket token and its expiry.
func (e *Exchange) GetWebsocketToken(ctx context.Context) (*WsTokenResponse, error) {
	var response *WsTokenResponse
	return response, e.SendAuthenticatedHTTPRequest(ctx, exchange.RestSpot, "GetWebSocketsToken", url.Values{}, &response)
}
