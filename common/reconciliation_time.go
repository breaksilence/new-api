package common

import "time"

// ReconciliationDateStart returns the first valid instant of a civil date.
// Midnight can be missing or repeated (e.g. Santiago/Havana DST). time.Date
// alone may normalize a missing midnight into the PREVIOUS date. Resolve the
// boundary by calendar date instead, including dates skipped by timezone rules.
// A wholly skipped date has the same boundary as the next existing date.
func ReconciliationDateStart(year int, month time.Month, day int, location *time.Location) time.Time {
	civil := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	dateKey := func(value time.Time) int64 {
		y, m, d := value.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix()
	}
	target := civil.Unix()
	// All supported timezone offsets fit comfortably inside this bracket.
	low, high := target-48*3600, target+48*3600
	for low < high {
		middle := low + (high-low)/2
		if dateKey(time.Unix(middle, 0).In(location)) < target {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return time.Unix(low, 0).In(location)
}

func ReconciliationDayStart(value time.Time) time.Time {
	return ReconciliationDateStart(value.Year(), value.Month(), value.Day(), value.Location())
}

func reconciliationAdjacentDay(value time.Time, direction int) time.Time {
	current := ReconciliationDayStart(value)
	civil := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC)
	for {
		civil = civil.AddDate(0, 0, direction)
		next := ReconciliationDateStart(civil.Year(), civil.Month(), civil.Day(), current.Location())
		if (direction > 0 && next.After(current)) || (direction < 0 && next.Before(current)) {
			return next
		}
	}
}

func ReconciliationNextDay(value time.Time) time.Time {
	return reconciliationAdjacentDay(value, 1)
}

func ReconciliationPreviousDay(value time.Time) time.Time {
	return reconciliationAdjacentDay(value, -1)
}
