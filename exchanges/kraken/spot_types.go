package kraken

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

var (
	errAmountInvalid           = errors.New("amount must be greater than zero")
	errAssetClassInvalid       = errors.New("asset class is not supported by this endpoint")
	errAssetRequired           = errors.New("asset is required")
	errFromRequired            = errors.New("source account is required")
	errNumericValueInvalid     = errors.New("numeric value must be finite")
	errOrderIDRequired         = errors.New("order identifier is required")
	errPairRequired            = errors.New("pair is required")
	errRebaseMultiplierInvalid = errors.New("rebase multiplier must be rebased or base")
	errTimeRangeInvalid        = errors.New("end time must not precede start time")
	errTimestampInvalid        = errors.New("timestamp must not precede the Unix epoch")
	errToRequired              = errors.New("destination account is required")
)

// AssetClass identifies a Kraken asset or pair class.
type AssetClass string

// Kraken asset classes.
const (
	AssetClassCurrency        AssetClass = "currency"
	AssetClassForex           AssetClass = "forex"
	AssetClassEquity          AssetClass = "equity"
	AssetClassEquityPair      AssetClass = "equity_pair"
	AssetClassNFT             AssetClass = "nft"
	AssetClassDerivatives     AssetClass = "derivatives"
	AssetClassTokenizedAsset  AssetClass = "tokenized_asset"
	AssetClassFuturesContract AssetClass = "futures_contract"
	AssetClassSyntheticPair   AssetClass = "synthetic_pair"
	AssetClassExternalPair    AssetClass = "external_pair"
)

// RebaseMultiplier controls how tokenized-asset values are displayed.
type RebaseMultiplier string

// Kraken rebase multiplier modes.
const (
	RebaseMultiplierRebased RebaseMultiplier = "rebased"
	RebaseMultiplierBase    RebaseMultiplier = "base"
)

// WsTokenResponse holds the Spot WebSocket authentication token.
type WsTokenResponse struct {
	Expires int64  `json:"expires"`
	Token   string `json:"token"`
}

type genericRESTResponse struct {
	Error  errorResponse `json:"error"`
	Result any           `json:"result"`
}

type errorResponse struct {
	warnings []string
	errors   error
}

func (e *errorResponse) UnmarshalJSON(data []byte) error {
	var errInterface any
	if err := json.Unmarshal(data, &errInterface); err != nil {
		return err
	}

	switch d := errInterface.(type) {
	case string:
		if d[0] == 'E' {
			e.errors = common.AppendError(e.errors, errors.New(d))
		} else {
			e.warnings = append(e.warnings, d)
		}
	case []any:
		for x := range d {
			errStr, ok := d[x].(string)
			if !ok {
				return fmt.Errorf("unable to convert %v to string", d[x])
			}
			if errStr[0] == 'E' {
				e.errors = common.AppendError(e.errors, errors.New(errStr))
			} else {
				e.warnings = append(e.warnings, errStr)
			}
		}
	default:
		return fmt.Errorf("unhandled error response type %T", errInterface)
	}
	return nil
}

// Errors returns one or many errors as an error.
func (e errorResponse) Errors() error {
	return e.errors
}

// Warnings returns a string of warnings.
func (e errorResponse) Warnings() string {
	return strings.Join(e.warnings, ", ")
}

func isValidSpotEnum[T ~string](value T, allowed ...string) bool {
	return value == "" || slices.Contains(allowed, string(value))
}

func formatSpotFloat(value float64) (string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", errNumericValueInvalid
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
}
