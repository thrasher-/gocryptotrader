package buffer

import (
	"fmt"
	"time"

	"github.com/thrasher-corp/gocryptotrader/common/key"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-corp/gocryptotrader/log"
)

// ProcessType identifies the websocket orderbook processing path captured in a ProcessReport.
type ProcessType string

const (
	// SnapshotProcess reports timings for an orderbook snapshot load, publish and relay send.
	SnapshotProcess ProcessType = "snapshot"
	// UpdateProcess reports timings for an orderbook update apply, publish and relay send.
	UpdateProcess ProcessType = "update"

	slowProcessReportThreshold = 100 * time.Microsecond
)

// ProcessReport captures timings for the most recent applied snapshot or update for a shared websocket orderbook.
type ProcessReport struct {
	Operation             ProcessType
	UpdateID              int64
	AppliedUpdates        int
	BidCount              int
	AskCount              int
	Buffered              bool
	LastPushed            time.Time
	ReceivedAt            time.Time
	StartedAt             time.Time
	CompletedAt           time.Time
	ApplyDuration         time.Duration
	PublishDuration       time.Duration
	SendDuration          time.Duration
	TotalDuration         time.Duration
	ReceivedToStart       time.Duration
	ReceivedToFinish      time.Duration
	ExchangePushToReceive time.Duration
	ExchangePushToFinish  time.Duration
	SendFailed            bool
}

type processReportInput struct {
	Operation      ProcessType
	UpdateID       int64
	AppliedUpdates int
	BidCount       int
	AskCount       int
	Buffered       bool
	LastPushed     time.Time
	ReceivedAt     time.Time
	StartedAt      time.Time
	AppliedAt      time.Time
	PublishedAt    time.Time
	CompletedAt    time.Time
	SendErr        error
}

func buildProcessReport(input processReportInput) ProcessReport {
	return ProcessReport{
		Operation:             input.Operation,
		UpdateID:              input.UpdateID,
		AppliedUpdates:        input.AppliedUpdates,
		BidCount:              input.BidCount,
		AskCount:              input.AskCount,
		Buffered:              input.Buffered,
		LastPushed:            input.LastPushed,
		ReceivedAt:            input.ReceivedAt,
		StartedAt:             input.StartedAt,
		CompletedAt:           input.CompletedAt,
		ApplyDuration:         input.AppliedAt.Sub(input.StartedAt),
		PublishDuration:       input.PublishedAt.Sub(input.AppliedAt),
		SendDuration:          input.CompletedAt.Sub(input.PublishedAt),
		TotalDuration:         input.CompletedAt.Sub(input.StartedAt),
		ReceivedToStart:       durationBetween(input.ReceivedAt, input.StartedAt),
		ReceivedToFinish:      durationBetween(input.ReceivedAt, input.CompletedAt),
		ExchangePushToReceive: durationBetween(input.LastPushed, input.ReceivedAt),
		ExchangePushToFinish:  durationBetween(input.LastPushed, input.CompletedAt),
		SendFailed:            input.SendErr != nil,
	}
}

func durationBetween(start, end time.Time) time.Duration {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start)
}

func (h *orderbookHolder) storeProcessReport(report ProcessReport) {
	h.reportM.Lock()
	h.lastReport = report
	h.reportM.Unlock()
}

func (h *orderbookHolder) loadProcessReport() ProcessReport {
	h.reportM.RLock()
	report := h.lastReport
	h.reportM.RUnlock()
	return report
}

func (o *Orderbook) maybeLogProcessReport(report ProcessReport, p currency.Pair, a asset.Item) {
	if !o.verbose || report.TotalDuration < slowProcessReportThreshold {
		return
	}
	var pushedToStart time.Duration
	if !report.LastPushed.IsZero() {
		pushedToStart = report.StartedAt.Sub(report.LastPushed)
	}
	log.Debugf(log.WebsocketMgr,
		"%s websocket orderbook slow op=%s pair=%s asset=%s total=%s apply=%s publish=%s send=%s recv_to_start=%s recv_to_finish=%s push_to_receive=%s push_to_finish=%s bids=%d asks=%d buffered=%t applied_updates=%d send_failed=%t update_id=%d pushed_to_start=%s",
		o.exchangeName,
		report.Operation,
		p,
		a,
		report.TotalDuration,
		report.ApplyDuration,
		report.PublishDuration,
		report.SendDuration,
		report.ReceivedToStart,
		report.ReceivedToFinish,
		report.ExchangePushToReceive,
		report.ExchangePushToFinish,
		report.BidCount,
		report.AskCount,
		report.Buffered,
		report.AppliedUpdates,
		report.SendFailed,
		report.UpdateID,
		pushedToStart,
	)
}

// LastProcessReport returns the most recent applied snapshot or update timings for the specified orderbook.
func (o *Orderbook) LastProcessReport(p currency.Pair, a asset.Item) (ProcessReport, error) {
	if p.IsEmpty() {
		return ProcessReport{}, currency.ErrCurrencyPairEmpty
	}
	if !a.IsValid() {
		return ProcessReport{}, asset.ErrInvalidAsset
	}
	o.m.RLock()
	holder, ok := o.ob[key.PairAsset{Base: p.Base.Item, Quote: p.Quote.Item, Asset: a}]
	o.m.RUnlock()
	if !ok {
		return ProcessReport{}, fmt.Errorf("%s %w: %s.%s", o.exchangeName, orderbook.ErrDepthNotFound, a, p)
	}
	return holder.loadProcessReport(), nil
}
