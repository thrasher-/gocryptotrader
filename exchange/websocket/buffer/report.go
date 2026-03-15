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
	Operation       ProcessType
	UpdateID        int64
	AppliedUpdates  int
	Buffered        bool
	LastPushed      time.Time
	StartedAt       time.Time
	CompletedAt     time.Time
	ApplyDuration   time.Duration
	PublishDuration time.Duration
	SendDuration    time.Duration
	TotalDuration   time.Duration
	SendFailed      bool
}

func buildProcessReport(operation ProcessType, updateID int64, appliedUpdates int, buffered bool, lastPushed time.Time, startedAt, appliedAt, publishedAt, completedAt time.Time, sendErr error) ProcessReport {
	return ProcessReport{
		Operation:       operation,
		UpdateID:        updateID,
		AppliedUpdates:  appliedUpdates,
		Buffered:        buffered,
		LastPushed:      lastPushed,
		StartedAt:       startedAt,
		CompletedAt:     completedAt,
		ApplyDuration:   appliedAt.Sub(startedAt),
		PublishDuration: publishedAt.Sub(appliedAt),
		SendDuration:    completedAt.Sub(publishedAt),
		TotalDuration:   completedAt.Sub(startedAt),
		SendFailed:      sendErr != nil,
	}
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
		"%s websocket orderbook slow op=%s pair=%s asset=%s total=%s apply=%s publish=%s send=%s buffered=%t applied_updates=%d send_failed=%t update_id=%d pushed_to_start=%s",
		o.exchangeName,
		report.Operation,
		p,
		a,
		report.TotalDuration,
		report.ApplyDuration,
		report.PublishDuration,
		report.SendDuration,
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
