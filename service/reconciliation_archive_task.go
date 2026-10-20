package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	reconciliationArchiveFinalizeDelay = 10 * time.Minute
	reconciliationArchiveActiveDelay   = time.Minute
	reconciliationArchiveIdleDelay     = 5 * time.Minute
	reconciliationArchiveDayTimeout    = 10 * time.Minute
)

var (
	reconciliationArchiveOnce    sync.Once
	reconciliationArchiveRunning atomic.Bool
)

func reconciliationArchiveDayStart(value time.Time) time.Time {
	value = value.In(time.Local)
	return common.ReconciliationDayStart(value)
}

func reconciliationReadyThrough(now time.Time) time.Time {
	today := reconciliationArchiveDayStart(now)
	if now.Before(today.Add(reconciliationArchiveFinalizeDelay)) {
		return common.ReconciliationPreviousDay(common.ReconciliationPreviousDay(today))
	}
	return common.ReconciliationPreviousDay(today)
}

func nextReconciliationArchiveDay(
	now time.Time,
	state *model.ReconciliationArchiveState,
	earliestTimestamp int64,
) (time.Time, bool) {
	readyThrough := reconciliationReadyThrough(now)
	if state == nil {
		return readyThrough, true
	}

	through := reconciliationArchiveDayStart(time.Unix(state.ArchivedThrough, 0))
	if through.Before(readyThrough) {
		return common.ReconciliationNextDay(through), true
	}
	if earliestTimestamp <= 0 {
		return time.Time{}, false
	}

	earliest := reconciliationArchiveDayStart(time.Unix(earliestTimestamp, 0))
	start := reconciliationArchiveDayStart(time.Unix(state.SummaryStart, 0))
	if start.After(earliest) {
		return common.ReconciliationPreviousDay(start), true
	}
	return time.Time{}, false
}

func StartReconciliationArchiveTask() {
	reconciliationArchiveOnce.Do(func() {
		if !common.IsMasterNode || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), "reconciliation daily archive task started")
			for {
				worked := runReconciliationArchiveOnce()
				delay := reconciliationArchiveIdleDelay
				if worked {
					delay = reconciliationArchiveActiveDelay
				}
				timer := time.NewTimer(delay)
				<-timer.C
			}
		})
	})
}

func runReconciliationArchiveOnce() bool {
	if !reconciliationArchiveRunning.CompareAndSwap(false, true) {
		return false
	}
	defer reconciliationArchiveRunning.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), reconciliationArchiveDayTimeout)
	defer cancel()

	earliest, err := model.GetEarliestConsumeLogTimestamp(ctx)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("reconciliation archive: query earliest log failed: %v", err))
		return false
	}
	state, err := model.GetReconciliationArchiveState(ctx)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("reconciliation archive: query state failed: %v", err))
		return false
	}
	target, ok := nextReconciliationArchiveDay(time.Now(), state, earliest)
	if !ok {
		return false
	}

	rows, err := model.RebuildReconciliationDailySummary(ctx, target.Unix())
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("reconciliation archive: rebuild %s failed: %v", target.Format(time.DateOnly), err))
		return false
	}
	logger.LogInfo(ctx, fmt.Sprintf("reconciliation archive: completed %s, rows=%d", target.Format(time.DateOnly), rows))
	return true
}
