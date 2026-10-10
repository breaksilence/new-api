package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestReconciliationMidnightDSTFilterAndScheduler(t *testing.T) {
	originalLocation := time.Local
	t.Cleanup(func() { time.Local = originalLocation })
	var err error
	time.Local, err = time.LoadLocation("America/Santiago")
	require.NoError(t, err)
	day5 := common.ReconciliationDateStart(2026, 9, 5, time.Local)
	day6 := common.ReconciliationDateStart(2026, 9, 6, time.Local)
	day7 := common.ReconciliationDateStart(2026, 9, 7, time.Local)
	filter, err := BuildReconciliationFilter("2026-09-06", "2026-09-06", "day", 0, "")
	require.NoError(t, err)
	require.Equal(t, day6.Unix(), filter.StartTimestamp)
	require.Equal(t, day7.Unix(), filter.EndTimestamp)
	require.EqualValues(t, 23*3600, filter.EndTimestamp-filter.StartTimestamp)
	state := &model.ReconciliationArchiveState{SummaryStart: day5.Unix(), ArchivedThrough: day5.Unix()}
	target, ok := nextReconciliationArchiveDay(day7.Add(time.Hour), state, day5.Unix())
	require.True(t, ok)
	require.Equal(t, day6, target, "must not repeatedly archive Sep 5")
	state.SummaryStart, state.ArchivedThrough = day6.Unix(), day6.Unix()
	target, ok = nextReconciliationArchiveDay(day7.Add(time.Hour), state, day5.Unix())
	require.True(t, ok)
	require.Equal(t, day5, target, "backfill must reset 01:00 DST start to preceding date's boundary")
	require.Equal(t, "2026-09-04", reconciliationReadyThrough(day6.Add(5*time.Minute)).Format(time.DateOnly))
	require.Equal(t, day5, reconciliationReadyThrough(day6.Add(15*time.Minute)))
}

func TestReconciliationReadyThroughWaitsForClosedDay(t *testing.T) {
	location := time.Local
	beforeDelay := time.Date(2026, 10, 9, 0, 5, 0, 0, location)
	afterDelay := time.Date(2026, 10, 9, 0, 15, 0, 0, location)

	require.Equal(t, "2026-10-07", reconciliationReadyThrough(beforeDelay).Format(time.DateOnly))
	require.Equal(t, "2026-10-08", reconciliationReadyThrough(afterDelay).Format(time.DateOnly))
}

func TestNextReconciliationArchiveDayPrioritizesRecentThenBackfills(t *testing.T) {
	location := time.Local
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, location)
	earliest := time.Date(2026, 9, 1, 8, 0, 0, 0, location)

	target, ok := nextReconciliationArchiveDay(now, nil, earliest.Unix())
	require.True(t, ok)
	require.Equal(t, "2026-10-08", target.Format(time.DateOnly))

	state := &model.ReconciliationArchiveState{
		SummaryStart:    time.Date(2026, 10, 8, 0, 0, 0, 0, location).Unix(),
		ArchivedThrough: time.Date(2026, 10, 8, 0, 0, 0, 0, location).Unix(),
	}
	target, ok = nextReconciliationArchiveDay(now, state, earliest.Unix())
	require.True(t, ok)
	require.Equal(t, "2026-10-07", target.Format(time.DateOnly))

	state.SummaryStart = time.Date(2026, 9, 1, 0, 0, 0, 0, location).Unix()
	target, ok = nextReconciliationArchiveDay(now, state, earliest.Unix())
	require.False(t, ok)
	require.True(t, target.IsZero())
}

func TestNextReconciliationArchiveDayCatchesUpForwardFirst(t *testing.T) {
	location := time.Local
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, location)
	state := &model.ReconciliationArchiveState{
		SummaryStart:    time.Date(2026, 10, 5, 0, 0, 0, 0, location).Unix(),
		ArchivedThrough: time.Date(2026, 10, 6, 0, 0, 0, 0, location).Unix(),
	}

	target, ok := nextReconciliationArchiveDay(
		now,
		state,
		time.Date(2026, 9, 1, 0, 0, 0, 0, location).Unix(),
	)
	require.True(t, ok)
	require.Equal(t, "2026-10-07", target.Format(time.DateOnly))
}
