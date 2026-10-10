package model

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func reconciliationReviewDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "review.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &ReconciliationDailySummary{}, &ReconciliationArchiveState{}))
	original := LOG_DB
	LOG_DB = db
	t.Cleanup(func() {
		LOG_DB = original
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	return db, dsn
}

func TestReconciliationArchiveMidnightDSTPreservesRawResults(t *testing.T) {
	db, _ := reconciliationReviewDB(t)
	originalLocation := time.Local
	t.Cleanup(func() { time.Local = originalLocation })
	var err error
	time.Local, err = time.LoadLocation("America/Santiago")
	require.NoError(t, err)
	first := common.ReconciliationDateStart(2026, 9, 5, time.Local)
	second := common.ReconciliationNextDay(first)
	end := common.ReconciliationNextDay(second)
	longModel := strings.Repeat("m", 250) + "-compact-suffix"
	var logs []Log
	for i, date := range []string{"2026-09-05 23:30:00", "2026-09-06 01:30:00", "2026-09-06 23:30:00"} {
		stamp, parseErr := time.ParseInLocation(time.DateTime, date, time.Local)
		require.NoError(t, parseErr)
		logs = append(logs, Log{UserId: 1, Username: "alice", ModelName: longModel, Type: LogTypeConsume, CreatedAt: stamp.Unix(), Quota: i + 1})
	}
	require.NoError(t, db.Create(&logs).Error)
	filter := dto.ReconciliationFilter{StartTimestamp: first.Unix(), EndTimestamp: end.Unix(), Granularity: "day"}
	raw, total, quota, err := GetReconciliationBillsPage(context.Background(), filter, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.EqualValues(t, 6, quota)
	require.Equal(t, "2026-09-06", raw[0].Period)
	require.EqualValues(t, 5, raw[0].Quota)
	require.Equal(t, "2026-09-05", raw[1].Period)
	require.EqualValues(t, 1, raw[1].Quota)
	for _, day := range []time.Time{first, second} {
		_, err = RebuildReconciliationDailySummary(context.Background(), day.Unix())
		require.NoError(t, err)
		items, count, sum, readErr := GetReconciliationBillsPage(context.Background(), filter, 0, 20)
		require.NoError(t, readErr)
		require.Equal(t, raw, items, "mixed and fully archived ranges must match raw results")
		require.Equal(t, total, count)
		require.Equal(t, quota, sum)
	}
	filter.Granularity = "month"
	items, err := GetReconciliationBills(context.Background(), filter, 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.EqualValues(t, 6, items[0].Quota)
	require.Equal(t, longModel, items[0].ModelName)
}

func TestReconciliationSQLiteArchiveDoesNotHoldWriterDuringRead(t *testing.T) {
	db, dsn := reconciliationReviewDB(t)
	writer, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlWriter, err := writer.DB()
	require.NoError(t, err)
	sqlWriter.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlWriter.Close()) })
	require.NoError(t, writer.Exec("PRAGMA busy_timeout = 200").Error)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	logs := make([]Log, reconciliationSQLiteReadBatchSize+3)
	for i := range logs {
		logs[i] = Log{UserId: 1, Username: "alice", ModelName: "model", Type: LogTypeConsume, CreatedAt: day.Unix() + 1, Quota: 3}
	}
	require.NoError(t, db.CreateInBatches(&logs, 100).Error)

	reading, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	pause := func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			once.Do(func() { close(reading); <-resume })
		}
	}
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("review:pause_query", pause))
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("review:pause_row", pause))
	done := make(chan error, 1)
	go func() {
		_, err := RebuildReconciliationDailySummary(context.Background(), day.Unix())
		done <- err
	}()
	select {
	case <-reading:
	case <-time.After(5 * time.Second):
		close(resume)
		t.Fatal("archive did not reach the log read")
	}
	// This write runs before the archive read resumes, including on the first
	// archive when its progress row did not previously exist.
	writeErr := writer.Create(&Log{UserId: 2, Username: "live", Type: LogTypeConsume, CreatedAt: day.AddDate(0, 0, 1).Unix(), Quota: 7}).Error
	close(resume)
	require.NoError(t, <-done)
	require.NoError(t, writeErr, "archiving must not hold SQLite's writer lock while reading")
	var summary ReconciliationDailySummary
	require.NoError(t, db.First(&summary).Error)
	require.EqualValues(t, len(logs), summary.LogCount)
	require.EqualValues(t, len(logs)*3, summary.Quota)
	var rawCount int64
	require.NoError(t, db.Model(&Log{}).Count(&rawCount).Error)
	require.EqualValues(t, len(logs)+1, rawCount)
}

func TestReconciliationSQLiteArchiveRejectsStalePublishAndRollsBack(t *testing.T) {
	db, _ := reconciliationReviewDB(t)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	require.NoError(t, db.Create(&Log{UserId: 1, Username: "alice", ModelName: "model", Type: LogTypeConsume, CreatedAt: day.Unix() + 1, Quota: 3}).Error)
	_, err := RebuildReconciliationDailySummary(context.Background(), day.Unix())
	require.NoError(t, err)

	advanced := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("review:advance_revision", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" && !advanced {
			advanced = true
			tx.AddError(db.Model(&ReconciliationArchiveState{}).Where("id = 1").Update("revision", gorm.Expr("revision + 1")).Error)
		}
	}))
	_, err = RebuildReconciliationDailySummary(context.Background(), day.Unix())
	require.ErrorIs(t, err, errReconciliationArchiveChanged)
	require.NoError(t, db.Callback().Query().Remove("review:advance_revision"))
	var before ReconciliationArchiveState
	require.NoError(t, db.First(&before).Error)
	require.EqualValues(t, 2, before.Revision)

	failure := errors.New("simulated summary insertion failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("review:fail_publish", func(tx *gorm.DB) {
		if tx.Statement.Table == "reconciliation_daily_summaries" {
			tx.AddError(failure)
		}
	}))
	_, err = RebuildReconciliationDailySummary(context.Background(), day.Unix())
	require.ErrorIs(t, err, failure)
	var after ReconciliationArchiveState
	require.NoError(t, db.First(&after).Error)
	require.Equal(t, before, after, "failed replacement must roll back progress too")
	var summary ReconciliationDailySummary
	require.NoError(t, db.First(&summary).Error)
	require.EqualValues(t, 3, summary.Quota, "failed replacement must retain the previous summary")
	require.NoError(t, db.Callback().Create().Remove("review:fail_publish"))
	_, err = RebuildReconciliationDailySummary(context.Background(), day.Unix())
	require.NoError(t, err)
}

func TestReconciliationSQLiteArchiveUsesDateIndexAndStableBatchCursor(t *testing.T) {
	db, _ := reconciliationReviewDB(t)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	logs := make([]Log, reconciliationSQLiteReadBatchSize*2+7)
	var expectedCount, expectedQuota int64
	for i := range logs {
		// IDs and dates intentionally have opposite order; many rows share
		// the same second, and non-consumption rows occur between them.
		logs[i] = Log{UserId: 1, Username: "alice", ModelName: "m", Type: LogTypeConsume,
			CreatedAt: day.Unix() + int64((len(logs)-i)/1200), Quota: i + 1}
		if i%7 == 0 {
			logs[i].Type = LogTypeRefund
		} else {
			expectedCount++
			expectedQuota += int64(logs[i].Quota)
		}
	}
	logs = append(logs,
		Log{UserId: 1, Type: LogTypeConsume, CreatedAt: day.Unix() - 1, Quota: 999999},
		Log{UserId: 1, Type: LogTypeConsume, CreatedAt: day.AddDate(0, 0, 1).Unix(), Quota: 999999})
	require.NoError(t, db.CreateInBatches(&logs, 100).Error)
	type statement struct {
		sql  string
		args []any
	}
	var statements []statement
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("review:record_batches", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			statements = append(statements, statement{tx.Statement.SQL.String(), append([]any(nil), tx.Statement.Vars...)})
		}
	}))
	_, err := RebuildReconciliationDailySummary(context.Background(), day.Unix())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(statements), 3)
	var summary ReconciliationDailySummary
	require.NoError(t, db.First(&summary).Error)
	require.Equal(t, expectedCount, summary.LogCount)
	require.Equal(t, expectedQuota, summary.Quota)
	for _, statement := range statements {
		var plan []struct{ Detail string }
		require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+statement.sql, statement.args...).Scan(&plan).Error)
		var details []string
		for _, row := range plan {
			details = append(details, row.Detail)
		}
		joined := strings.Join(details, "\n")
		require.Contains(t, joined, "idx_created_at_type", "batches must seek the target date, not scan older history")
		require.NotContains(t, joined, "TEMP B-TREE", "batching must not sort the whole day before LIMIT")
	}
}

func TestReconciliationArchivedUsernameAliasesRemainSearchable(t *testing.T) {
	db, _ := reconciliationReviewDB(t)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, Username: "old-alias", Type: LogTypeConsume, ModelName: "model", CreatedAt: day.Unix() + 1, Quota: 3},
		{UserId: 1, Username: "z-new-name", Type: LogTypeConsume, ModelName: "model", CreatedAt: day.Unix() + 2, Quota: 5},
	}).Error)
	before, count, err := GetReconciliationUserOptions(context.Background(), "old-alias", 0, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	_, err = RebuildReconciliationDailySummary(context.Background(), day.Unix())
	require.NoError(t, err)
	after, count, err := GetReconciliationUserOptions(context.Background(), "old-alias", 0, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Equal(t, before, after)
	all, count, err := GetReconciliationUserOptions(context.Background(), "", 0, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Equal(t, "z-new-name", all[0].Username)
}

func TestReconciliationPageTotalsSurvivePaginationAndEmptyPages(t *testing.T) {
	db, _ := reconciliationReviewDB(t)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, Username: "alice", Type: LogTypeConsume, ModelName: "a", CreatedAt: day.Unix() + 1, Quota: 3},
		{UserId: 2, Username: "bob", Type: LogTypeConsume, ModelName: "b", CreatedAt: day.Unix() + 2, Quota: 5},
	}).Error)
	filter := dto.ReconciliationFilter{StartTimestamp: day.Unix(), EndTimestamp: day.AddDate(0, 0, 1).Unix(), Granularity: "day"}
	for _, archived := range []bool{false, true} {
		if archived {
			_, err := RebuildReconciliationDailySummary(context.Background(), day.Unix())
			require.NoError(t, err)
		}
		for offset := 0; offset < 3; offset++ {
			items, total, quota, err := GetReconciliationBillsPage(context.Background(), filter, offset, 1)
			require.NoError(t, err)
			require.EqualValues(t, 2, total)
			require.EqualValues(t, 8, quota)
			if offset < 2 {
				require.Len(t, items, 1)
				require.Equal(t, offset+1, items[0].UserID)
			} else {
				require.Empty(t, items, "LEFT JOIN's empty page must not create a phantom bill")
			}
		}
	}
	filter.UserID = 99
	items, total, quota, err := GetReconciliationBillsPage(context.Background(), filter, 0, 20)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, total)
	require.Zero(t, quota)
}
