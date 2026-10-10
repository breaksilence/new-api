package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReconciliationCivilDayBoundaries(t *testing.T) {
	for _, tc := range []struct {
		zone, date, next, previous string
		hour, durationHours        int
	}{
		{"Asia/Shanghai", "2026-09-06", "2026-09-07", "2026-09-05", 0, 24},
		{"America/New_York", "2026-03-08", "2026-03-09", "2026-03-07", 0, 23},
		{"America/New_York", "2026-11-01", "2026-11-02", "2026-10-31", 0, 25},
		{"America/Santiago", "2026-09-05", "2026-09-06", "2026-09-04", 0, 24},
		{"America/Santiago", "2026-09-06", "2026-09-07", "2026-09-05", 1, 23},
		{"America/Havana", "2026-11-01", "2026-11-02", "2026-10-31", 0, 25},
		{"Pacific/Apia", "2011-12-29", "2011-12-31", "2011-12-28", 0, 24},
		{"Pacific/Apia", "2011-12-31", "2012-01-01", "2011-12-29", 0, 24},
	} {
		t.Run(tc.zone+tc.date, func(t *testing.T) {
			location, err := time.LoadLocation(tc.zone)
			require.NoError(t, err)
			civil, err := time.Parse(time.DateOnly, tc.date)
			require.NoError(t, err)
			start := ReconciliationDateStart(civil.Year(), civil.Month(), civil.Day(), location)
			require.Equal(t, tc.date, start.Format(time.DateOnly))
			require.Equal(t, tc.hour, start.Hour())
			require.NotEqual(t, tc.date, start.Add(-time.Second).Format(time.DateOnly))
			next := ReconciliationNextDay(start)
			require.Equal(t, tc.next, next.Format(time.DateOnly))
			require.Equal(t, time.Duration(tc.durationHours)*time.Hour, next.Sub(start))
			require.Equal(t, tc.previous, ReconciliationPreviousDay(start).Format(time.DateOnly))
			require.Equal(t, start, ReconciliationDayStart(next.Add(-time.Second)))
		})
	}
}
