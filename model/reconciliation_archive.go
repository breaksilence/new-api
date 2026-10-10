package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const reconciliationArchiveStateID = 1

// ReconciliationDailySummary stores an exact, closed-day aggregation of
// consume logs. Raw logs remain the source of truth and are never deleted.
type ReconciliationDailySummary struct {
	ID        int64              `gorm:"primaryKey"`
	DayStart  int64              `gorm:"not null;uniqueIndex:idx_reconciliation_daily_dimension,priority:1;index:idx_reconciliation_daily_day"`
	UserID    int                `gorm:"not null;uniqueIndex:idx_reconciliation_daily_dimension,priority:2;index:idx_reconciliation_daily_user"`
	Username  reconciliationText `gorm:"size:191;not null;default:''"`
	ModelName reconciliationText `gorm:"size:256;not null;default:'';uniqueIndex:idx_reconciliation_daily_dimension,priority:3;index:idx_reconciliation_daily_model"`
	LogCount  int64              `gorm:"not null;default:0"`
	Quota     int64              `gorm:"not null;default:0"`
}

func (ReconciliationDailySummary) TableName() string {
	return "reconciliation_daily_summaries"
}

// ReconciliationArchiveState describes the single contiguous date interval
// that is safe to read from ReconciliationDailySummary.
type ReconciliationArchiveState struct {
	ID              int   `gorm:"primaryKey"`
	SummaryStart    int64 `gorm:"not null;default:0"`
	ArchivedThrough int64 `gorm:"not null;default:0"`
	UpdatedAt       int64 `gorm:"not null;default:0"`
	Revision        int64 `gorm:"not null;default:0"`
}

func (ReconciliationArchiveState) TableName() string {
	return "reconciliation_archive_states"
}

func GetReconciliationArchiveState(ctx context.Context) (*ReconciliationArchiveState, error) {
	var state ReconciliationArchiveState
	result := LOG_DB.WithContext(ctx).
		Where("id = ?", reconciliationArchiveStateID).
		Limit(1).
		Find(&state)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	if state.SummaryStart <= 0 || state.ArchivedThrough < state.SummaryStart {
		return nil, nil
	}
	return &state, nil
}

func GetEarliestConsumeLogTimestamp(ctx context.Context) (int64, error) {
	var earliest sql.NullInt64
	err := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Select("MIN(created_at)").
		Where("type = ?", LogTypeConsume).
		Scan(&earliest).Error
	if err != nil {
		return 0, err
	}
	if !earliest.Valid {
		return 0, nil
	}
	return earliest.Int64, nil
}

func reconciliationLocalDayStart(timestamp int64) time.Time {
	value := time.Unix(timestamp, 0).In(time.Local)
	return common.ReconciliationDayStart(value)
}

func updateReconciliationArchiveBounds(state *ReconciliationArchiveState, dayStart int64) error {
	if state.SummaryStart == 0 {
		state.SummaryStart = dayStart
		state.ArchivedThrough = dayStart
		return nil
	}

	start := reconciliationLocalDayStart(state.SummaryStart)
	through := reconciliationLocalDayStart(state.ArchivedThrough)
	target := reconciliationLocalDayStart(dayStart)
	switch {
	case !target.Before(start) && !target.After(through):
		return nil
	case target.Equal(common.ReconciliationPreviousDay(start)):
		state.SummaryStart = dayStart
		return nil
	case target.Equal(common.ReconciliationNextDay(through)):
		state.ArchivedThrough = dayStart
		return nil
	default:
		return fmt.Errorf("reconciliation archive day %s is not adjacent to completed interval %s..%s",
			target.Format(time.DateOnly), start.Format(time.DateOnly), through.Format(time.DateOnly))
	}
}

// RebuildReconciliationDailySummary replaces one closed day's summary in a
// transaction and advances the completed interval only after the replacement
// succeeds. It is safe to retry the same day.
func RebuildReconciliationDailySummary(ctx context.Context, dayStart int64) (int, error) {
	day := reconciliationLocalDayStart(dayStart)
	dayStart = day.Unix()
	dayEnd := common.ReconciliationNextDay(day).Unix()
	if LOG_DB.Dialector.Name() == "sqlite" {
		return rebuildSQLiteReconciliationDailySummary(ctx, dayStart, dayEnd)
	}

	rowCount := 0
	err := LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state := ReconciliationArchiveState{ID: reconciliationArchiveStateID}
		stateResult := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", reconciliationArchiveStateID).
			Limit(1).
			Find(&state)
		if stateResult.Error != nil {
			return stateResult.Error
		}
		if stateResult.RowsAffected == 0 {
			state = ReconciliationArchiveState{ID: reconciliationArchiveStateID}
			if err := tx.Create(&state).Error; err != nil {
				return err
			}
		}

		rows := make([]ReconciliationDailySummary, 0)
		if err := tx.Model(&Log{}).
			Select("user_id, COALESCE(MAX(NULLIF(username, '')), '') AS username, model_name, COUNT(*) AS log_count, SUM(quota) AS quota").
			Where("type = ?", LogTypeConsume).
			Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
			Group("user_id").
			Group("model_name").
			Scan(&rows).Error; err != nil {
			return err
		}
		for index := range rows {
			rows[index].DayStart = dayStart
		}

		if err := tx.Where("day_start = ?", dayStart).Delete(&ReconciliationDailySummary{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.CreateInBatches(&rows, 500).Error; err != nil {
				return err
			}
		}

		if err := updateReconciliationArchiveBounds(&state, dayStart); err != nil {
			return err
		}
		state.UpdatedAt = time.Now().Unix()
		state.Revision++
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		rowCount = len(rows)
		return nil
	})
	return rowCount, err
}

var errReconciliationArchiveChanged = errors.New("reconciliation archive changed during aggregation; retry required")

const reconciliationSQLiteReadBatchSize = 1000

// SQLite allows only one writer. Read closed-day logs in bounded statements
// without a surrounding transaction, then atomically publish in a short write
// transaction. Even in rollback-journal mode, readers release their locks
// between batches. Historical edits during a rebuild still require a rerun.
func rebuildSQLiteReconciliationDailySummary(ctx context.Context, dayStart, dayEnd int64) (int, error) {
	db := LOG_DB.WithContext(ctx)
	state := ReconciliationArchiveState{ID: reconciliationArchiveStateID}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return 0, err
	}
	if err := db.Where("id = ?", reconciliationArchiveStateID).First(&state).Error; err != nil {
		return 0, err
	}
	expectedRevision := state.Revision
	if err := updateReconciliationArchiveBounds(&state, dayStart); err != nil {
		return 0, err
	}

	// Fix the upper bound so concurrent live inserts cannot extend this scan.
	var upperID int
	if err := db.Model(&Log{}).Select("COALESCE(MAX(id), 0)").Scan(&upperID).Error; err != nil {
		return 0, err
	}
	type dimension struct {
		userID    int
		modelName string
	}
	groups := make(map[dimension]*ReconciliationDailySummary)
	var cursor Log
	for {
		var batch []Log
		// Follow the existing (created_at, type, rowid) index order. Filtering
		// type here makes SQLite sort equal timestamps; ordering only by id
		// scans older history before the first matching day. Read all types in
		// this day and discard non-consumption rows after each bounded read.
		readBatch := func(query *gorm.DB, order string) error {
			return query.Select("id, created_at, type, user_id, username, model_name, quota").
				Order(order).Limit(reconciliationSQLiteReadBatchSize).Find(&batch).Error
		}
		if cursor.Id == 0 {
			if err := readBatch(db.Where("created_at >= ? AND created_at < ? AND id <= ?", dayStart, dayEnd, upperID),
				"created_at ASC, type ASC, id ASC"); err != nil {
				return 0, err
			}
		} else {
			// SQLite does not fully seek the implicit rowid in a three-part
			// tuple comparison. Fix the first two index columns to seek within
			// the same second/type, then skip that bucket when it is exhausted.
			if err := readBatch(db.Where("created_at = ? AND type = ? AND id > ? AND id <= ?",
				cursor.CreatedAt, cursor.Type, cursor.Id, upperID), "id ASC"); err != nil {
				return 0, err
			}
			if len(batch) == 0 {
				if err := readBatch(db.Where("(created_at, type) > (?, ?) AND created_at < ? AND id <= ?",
					cursor.CreatedAt, cursor.Type, dayEnd, upperID), "created_at ASC, type ASC, id ASC"); err != nil {
					return 0, err
				}
			}
		}
		if len(batch) == 0 {
			break
		}
		for _, log := range batch {
			if log.Type != LogTypeConsume {
				continue
			}
			key := dimension{log.UserId, log.ModelName}
			row := groups[key]
			if row == nil {
				row = &ReconciliationDailySummary{DayStart: dayStart, UserID: log.UserId, ModelName: reconciliationText(log.ModelName)}
				groups[key] = row
			}
			if log.Username > string(row.Username) {
				row.Username = reconciliationText(log.Username)
			}
			row.LogCount++
			row.Quota += int64(log.Quota)
		}
		cursor = batch[len(batch)-1]
	}
	rows := make([]ReconciliationDailySummary, 0, len(groups))
	for _, row := range groups {
		rows = append(rows, *row)
	}
	state.UpdatedAt = time.Now().Unix()
	state.Revision++
	err := db.Transaction(func(tx *gorm.DB) error {
		// Acquire the write lock first, without a read-to-write lock upgrade.
		// Reject an older computation if another worker published meanwhile.
		result := tx.Model(&ReconciliationArchiveState{}).
			Where("id = ? AND revision = ?", state.ID, expectedRevision).
			Updates(map[string]any{
				"summary_start": state.SummaryStart, "archived_through": state.ArchivedThrough,
				"updated_at": state.UpdatedAt, "revision": state.Revision,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errReconciliationArchiveChanged
		}
		if err := tx.Where("day_start = ?", dayStart).Delete(&ReconciliationDailySummary{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			return tx.CreateInBatches(&rows, 500).Error
		}
		return nil
	})
	return len(rows), err
}
