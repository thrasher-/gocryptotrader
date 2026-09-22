package binance

import (
	"fmt"

	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// A message alone is not an error: several SAPI endpoints return {"msg":"success"}.
// Code is optional and may be a JSON number or a numeric string.
func apiResponseError(data []byte) error {
	var response struct {
		Code             *types.Number `json:"code"`
		Success          *bool         `json:"success"`
		Message          string        `json:"msg"`
		AlternateMessage string        `json:"message"`
	}
	if json.Unmarshal(data, &response) != nil {
		return nil //nolint:nilerr // Non-envelope responses are decoded by the endpoint model.
	}
	failed := response.Success != nil && !*response.Success
	var code int64
	if response.Code != nil {
		code = response.Code.Int64()
		failed = failed || (code != 0 && code != 200)
	}
	if !failed {
		return nil
	}
	message := response.Message
	if message == "" {
		message = response.AlternateMessage
	}
	if mapped, ok := errorCodeToErrorMap[code]; ok {
		return fmt.Errorf("%w: %w: code %d: %s", errAPIResponse, mapped, code, message)
	}
	return fmt.Errorf("%w: code %d: %s", errAPIResponse, code, message)
}
