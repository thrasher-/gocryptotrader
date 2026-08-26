package orderbook

import (
	"fmt"
	"math"

	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// Side ordering for alignment checks. A bool rather than a comparison function keeps the price
// check a direct branch; a function value costs an indirect call for every adjacent pair.
const (
	descending = false
	ascending  = true
)

// Validate ensures that the orderbook items are correctly sorted and all fields are valid
// Bids should always go from a high price to a low price and Asks should always go from a low price to a higher price
func (b *Book) Validate() error {
	if err := common.NilGuard(b); err != nil {
		return err
	}
	if !b.ValidateOrderbook {
		return nil
	}
	return validate(b)
}

func validate(b *Book) error {
	// Some exchanges may return empty sides, but it's not an error
	// Options have empty sides too frequently for this warning to be useful
	if (len(b.Asks) == 0 || len(b.Bids) == 0) && !b.Asset.IsOptions() {
		log.Warnf(log.OrderBook, bookLengthIssue, b.Exchange, b.Pair, b.Asset, len(b.Bids), len(b.Asks))
	}
	err := checkAlignment(b.Bids, b.IsFundingRate, b.PriceDuplication, b.IDAlignment, b.ChecksumStringRequired, descending, b.Exchange)
	if err != nil {
		return fmt.Errorf(bidLoadBookFailure, b.Exchange, b.Pair, b.Asset, err)
	}
	err = checkAlignment(b.Asks, b.IsFundingRate, b.PriceDuplication, b.IDAlignment, b.ChecksumStringRequired, ascending, b.Exchange)
	if err != nil {
		return fmt.Errorf(askLoadBookFailure, b.Exchange, b.Pair, b.Asset, err)
	}
	return nil
}

// validateAround is validate restricted to the levels bids and asks touched. The book must have been
// valid beforehand and both sides must satisfy isOrdered.
func validateAround(b *Book, bids, asks Levels) error {
	// Some exchanges may return empty sides, but it's not an error
	// Options have empty sides too frequently for this warning to be useful
	if (len(b.Asks) == 0 || len(b.Bids) == 0) && !b.Asset.IsOptions() {
		log.Warnf(log.OrderBook, bookLengthIssue, b.Exchange, b.Pair, b.Asset, len(b.Bids), len(b.Asks))
	}
	err := checkAlignmentAround(b.Bids, bids, b.IsFundingRate, b.PriceDuplication, b.IDAlignment, b.ChecksumStringRequired, descending, b.Exchange)
	if err != nil {
		return fmt.Errorf(bidLoadBookFailure, b.Exchange, b.Pair, b.Asset, err)
	}
	err = checkAlignmentAround(b.Asks, asks, b.IsFundingRate, b.PriceDuplication, b.IDAlignment, b.ChecksumStringRequired, ascending, b.Exchange)
	if err != nil {
		return fmt.Errorf(askLoadBookFailure, b.Exchange, b.Pair, b.Asset, err)
	}
	return nil
}

// checkAlignmentAround runs checkAlignment over a three level window centred on each level touched,
// which is equivalent to walking an already aligned side.
//
// checkAlignment only ever compares a level with the one before it, so the only pairs an update can
// invalidate are those bordering a level it touched, and depth[i-1:i+2] covers both.
func checkAlignmentAround(depth, touched Levels, fundingRate, priceDuplication, isIDAligned, requiresChecksumString, isAscending bool, exch string) error {
	for x := range touched {
		// For a deleted level locate returns the index that closed the gap, covering the pair
		// that became adjacent
		i := depth.locate(touched[x].Price, isAscending)
		lo, hi := max(i-1, 0), min(i+2, len(depth))
		if err := checkAlignment(depth[lo:hi], fundingRate, priceDuplication, isIDAligned, requiresChecksumString, isAscending, exch); err != nil {
			return err
		}
	}
	return nil
}

// checkAlignment validates an orderbook side is sequential and does not contain any invalid data
func checkAlignment(depth Levels, fundingRate, priceDuplication, isIDAligned, requiresChecksumString, isAscending bool, exch string) error {
	for i := range depth {
		if depth[i].Price == 0 {
			switch {
			case exch == "Bitfinex" && fundingRate: /* funding rate can be 0 it seems on Bitfinex */
			default:
				return ErrPriceZero
			}
		} else if math.IsNaN(depth[i].Price) || math.IsInf(depth[i].Price, 0) {
			return errPriceInvalid
		}

		// Written as a positive test so that a NaN falls into it. NaN <= 0 is false, so an amount
		// that arrived as a quoted NaN was stored and then carried into every liquidity and
		// slippage figure derived from the book.
		if !(depth[i].Amount > 0 && depth[i].Amount < math.Inf(1)) {
			return errAmountInvalid
		}
		if fundingRate && depth[i].Period == 0 {
			return errPeriodUnset
		}
		if requiresChecksumString && (depth[i].StrAmount == "" || depth[i].StrPrice == "") {
			return errChecksumStringNotSet
		}

		if i != 0 {
			prev := i - 1
			if isAscending {
				if depth[i].Price < depth[prev].Price {
					return errPriceOutOfOrder
				}
			} else if depth[i].Price > depth[prev].Price {
				return errPriceOutOfOrder
			}
			if isIDAligned && depth[i].ID < depth[prev].ID {
				return errIDOutOfOrder
			}
			if !priceDuplication && depth[i].Price == depth[prev].Price {
				return errDuplication
			}
			if depth[i].ID != 0 && depth[i].ID == depth[prev].ID {
				return errIDDuplication
			}
		}
	}
	return nil
}
