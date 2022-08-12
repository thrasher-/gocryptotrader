package asset

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNotSupported is an error for an unsupported asset type
	ErrNotSupported = errors.New("unsupported asset type")
)

// Item stores the asset type
type Item uint16

// Items stores a list of assets types
type Items []Item

// Const vars for asset package
const (
	Empty Item = 0
	Spot  Item = 1 << iota
	Margin
	MarginFunding
	Index
	Binary
	PerpetualContract
	PerpetualSwap
	Futures
	UpsideProfitContract
	DownsideProfitContract
	CoinMarginedFutures
	USDTMarginedFutures
	USDCMarginedFutures

	futuresFlag   = PerpetualContract | PerpetualSwap | Futures | UpsideProfitContract | DownsideProfitContract | CoinMarginedFutures | USDTMarginedFutures | USDCMarginedFutures
	supportedFlag = Spot | Margin | MarginFunding | Index | Binary | PerpetualContract | PerpetualSwap | Futures | UpsideProfitContract | DownsideProfitContract | CoinMarginedFutures | USDTMarginedFutures | USDCMarginedFutures

	spot                   = "spot"
	margin                 = "margin"
	marginFunding          = "marginfunding"
	index                  = "index"
	binary                 = "binary"
	perpetualContract      = "perpetualcontract"
	perpetualSwap          = "perpetualswap"
	futures                = "futures"
	upsideProfitContract   = "upsideprofitcontract"
	downsideProfitContract = "downsideprofitcontract"
	coinMarginedFutures    = "coinmarginedfutures"
	usdtMarginedFutures    = "usdtmarginedfutures"
	usdcMarginedFutures    = "usdcmarginedfutures"
)

var (
	supportedList = Items{Spot, Margin, MarginFunding, Index, Binary, PerpetualContract, PerpetualSwap, Futures, UpsideProfitContract, DownsideProfitContract, CoinMarginedFutures, USDTMarginedFutures, USDCMarginedFutures}
)

// Supported returns a list of supported asset types
func Supported() Items {
	return supportedList
}

// String converts an Item its string representation
func (a Item) String() string {
	switch a {
	case Spot:
		return spot
	case Margin:
		return margin
	case MarginFunding:
		return marginFunding
	case Index:
		return index
	case Binary:
		return binary
	case PerpetualContract:
		return perpetualContract
	case PerpetualSwap:
		return perpetualSwap
	case Futures:
		return futures
	case UpsideProfitContract:
		return upsideProfitContract
	case DownsideProfitContract:
		return downsideProfitContract
	case CoinMarginedFutures:
		return coinMarginedFutures
	case USDTMarginedFutures:
		return usdtMarginedFutures
	case USDCMarginedFutures:
		return usdcMarginedFutures
	default:
		return ""
	}
}

// Strings converts an asset type array to a string array
func (a Items) Strings() []string {
	assets := make([]string, len(a))
	for x := range a {
		assets[x] = a[x].String()
	}
	return assets
}

// Contains returns whether or not the supplied asset exists
// in the list of Items
func (a Items) Contains(i Item) bool {
	if i.IsValid() {
		for x := range a {
			if a[x] == i {
				return true
			}
		}
	}
	return false
}

// JoinToString joins an asset type array and converts it to a string
// with the supplied separator
func (a Items) JoinToString(separator string) string {
	return strings.Join(a.Strings(), separator)
}

// IsValid returns whether or not the supplied asset type is valid or
// not
func (a Item) IsValid() bool {
	return a != Empty && supportedFlag&a == a
}

// UnmarshalJSON comforms type to the umarshaler interface
func (a *Item) UnmarshalJSON(d []byte) error {
	var assetString string
	err := json.Unmarshal(d, &assetString)
	if err != nil {
		return err
	}

	if assetString == "" {
		return nil
	}

	ai, err := New(assetString)
	if err != nil {
		return err
	}

	*a = ai
	return nil
}

// MarshalJSON comforms type to the marshaller interface
func (a Item) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// New takes an input matches to relevant package assets
func New(input string) (Item, error) {
	input = strings.ToLower(input)
	switch input {
	case spot:
		return Spot, nil
	case margin:
		return Margin, nil
	case marginFunding:
		return MarginFunding, nil
	case index:
		return Index, nil
	case binary:
		return Binary, nil
	case perpetualContract:
		return PerpetualContract, nil
	case perpetualSwap:
		return PerpetualSwap, nil
	case futures:
		return Futures, nil
	case upsideProfitContract:
		return UpsideProfitContract, nil
	case downsideProfitContract:
		return DownsideProfitContract, nil
	case coinMarginedFutures:
		return CoinMarginedFutures, nil
	case usdtMarginedFutures:
		return USDTMarginedFutures, nil
	case usdcMarginedFutures:
		return USDCMarginedFutures, nil
	default:
		return 0, fmt.Errorf("%w '%v', only supports %s",
			ErrNotSupported,
			input,
			supportedList)
	}
}

// UseDefault returns default asset type
func UseDefault() Item {
	return Spot
}

// IsFutures checks if the asset type is a futures contract based asset
func (a Item) IsFutures() bool {
	return a != Empty && futuresFlag&a == a
}
