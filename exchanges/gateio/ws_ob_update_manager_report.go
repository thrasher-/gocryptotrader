package gateio

import (
	"time"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

type syncTrigger string

const (
	syncTriggerInitialise        syncTrigger = "initialise"
	syncTriggerLastUpdateIDError syncTrigger = "last_update_id_error"
	syncTriggerDesync            syncTrigger = "desync"
	syncTriggerApplyUpdateError  syncTrigger = "apply_update_error"
)

type syncReport struct {
	Trigger               syncTrigger
	QueuedUpdateCount     int
	FirstPendingID        int64
	LastPendingID         int64
	StartedAt             time.Time
	CompletedAt           time.Time
	DelayWaitDuration     time.Duration
	RESTFetchDuration     time.Duration
	WaitForUpdateDuration time.Duration
	ApplyPendingDuration  time.Duration
	TotalDuration         time.Duration
	Success               bool
	FinalError            string
}

type syncReportInput struct {
	Trigger               syncTrigger
	QueuedUpdateCount     int
	FirstPendingID        int64
	LastPendingID         int64
	StartedAt             time.Time
	CompletedAt           time.Time
	DelayWaitDuration     time.Duration
	RESTFetchDuration     time.Duration
	WaitForUpdateDuration time.Duration
	ApplyPendingDuration  time.Duration
	FinalErr              error
}

func buildSyncReport(input syncReportInput) syncReport {
	report := syncReport{
		Trigger:               input.Trigger,
		QueuedUpdateCount:     input.QueuedUpdateCount,
		FirstPendingID:        input.FirstPendingID,
		LastPendingID:         input.LastPendingID,
		StartedAt:             input.StartedAt,
		CompletedAt:           input.CompletedAt,
		DelayWaitDuration:     input.DelayWaitDuration,
		RESTFetchDuration:     input.RESTFetchDuration,
		WaitForUpdateDuration: input.WaitForUpdateDuration,
		ApplyPendingDuration:  input.ApplyPendingDuration,
		TotalDuration:         input.CompletedAt.Sub(input.StartedAt),
		Success:               input.FinalErr == nil,
	}
	if input.FinalErr != nil {
		report.FinalError = input.FinalErr.Error()
	}
	return report
}

func (c *updateCache) storeSyncReport(report syncReport) {
	c.reportM.Lock()
	c.lastReport = report
	c.reportM.Unlock()
}

func (c *updateCache) loadSyncReport() syncReport {
	c.reportM.RLock()
	report := c.lastReport
	c.reportM.RUnlock()
	return report
}

func (m *wsOBUpdateManager) LastSyncReport(p currency.Pair, a asset.Item) (syncReport, error) {
	cache, err := m.LoadCache(p, a)
	if err != nil {
		return syncReport{}, err
	}
	return cache.loadSyncReport(), nil
}
