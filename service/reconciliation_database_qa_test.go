//go:build mysqlqa || dbqa

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// TestReconciliationDatabaseEndToEndQA uses disposable databases only. Run with:
//
//	MYSQL_QA_DSN='user:pass@tcp(127.0.0.1:33306)/db?parseTime=true&loc=Local' \
//	go test -tags dbqa ./service -run TestReconciliationDatabaseEndToEndQA -v
//
// POSTGRES_QA_DSN enables PostgreSQL; RECONCILIATION_SPLIT_QA_DSN and
// RECONCILIATION_SPLIT_LOG_QA_DSN enable a separate log DB. These databases
// must be dedicated to this test; log/summary fixtures are deleted.
func TestReconciliationDatabaseEndToEndQA(t *testing.T) {
	for _, tc := range []struct {
		name, dsn, logDSN string
	}{
		{"sqlite", "", ""},
		{"mysql", os.Getenv("MYSQL_QA_DSN"), ""},
		{"postgres", os.Getenv("POSTGRES_QA_DSN"), ""},
		{"split", os.Getenv("RECONCILIATION_SPLIT_QA_DSN"), os.Getenv("RECONCILIATION_SPLIT_LOG_QA_DSN")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name != "sqlite" && tc.dsn == "" {
				t.Skip("dedicated QA DSN is not set")
			}
			if tc.name == "sqlite" {
				originalPath := common.SQLitePath
				common.SQLitePath = filepath.Join(t.TempDir(), "qa.db") + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"
				t.Cleanup(func() { common.SQLitePath = originalPath })
			}
			runReconciliationDatabaseQA(t, tc.dsn, tc.logDSN)
		})
	}
}

func runReconciliationDatabaseQA(t *testing.T, dsn, logDSN string) {
	t.Helper()
	originalDB, originalLogDB := model.DB, model.LOG_DB
	t.Cleanup(func() {
		if model.LOG_DB != model.DB {
			sqlDB, err := model.LOG_DB.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
		}
		sqlDB, err := model.DB.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		model.DB, model.LOG_DB = originalDB, originalLogDB
	})
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	originalLocation := time.Local
	originalMaster := common.IsMasterNode
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	originalLogSQLDSN, hadLogSQLDSN := os.LookupEnv("LOG_SQL_DSN")
	t.Cleanup(func() {
		time.Local = originalLocation
		common.IsMasterNode = originalMaster
		common.SetDatabaseTypes(originalMainType, originalLogType)
		if hadSQLDSN {
			_ = os.Setenv("SQL_DSN", originalSQLDSN)
		} else {
			_ = os.Unsetenv("SQL_DSN")
		}
		if hadLogSQLDSN {
			_ = os.Setenv("LOG_SQL_DSN", originalLogSQLDSN)
		} else {
			_ = os.Unsetenv("LOG_SQL_DSN")
		}
	})
	time.Local = location
	common.IsMasterNode = true
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", dsn))
	require.NoError(t, os.Setenv("LOG_SQL_DSN", logDSN))

	t.Run("automatic migration and repeated startup", func(t *testing.T) {
		require.NoError(t, model.InitDB())
		require.NoError(t, model.InitLogDB())
		var version string
		if common.UsingLogDatabase(common.DatabaseTypeSQLite) {
			require.NoError(t, model.LOG_DB.Raw("SELECT sqlite_version()").Scan(&version).Error)
		} else {
			require.NoError(t, model.LOG_DB.Raw("SELECT version()").Scan(&version).Error)
		}
		t.Logf("log database version: %s", version)
		require.True(t, model.LOG_DB.Migrator().HasTable(&model.ReconciliationDailySummary{}))
		require.True(t, model.LOG_DB.Migrator().HasTable(&model.ReconciliationArchiveState{}))
		require.True(t, model.LOG_DB.Migrator().HasIndex(&model.ReconciliationDailySummary{}, "idx_reconciliation_daily_dimension"))
		if logDSN != "" {
			require.False(t, model.DB.Migrator().HasTable(&model.ReconciliationDailySummary{}))
		}

		// A second startup must leave the schema usable and unchanged.
		// Reset process-global dialect flags to simulate a fresh process.
		if model.LOG_DB != model.DB {
			sqlDB, err := model.LOG_DB.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
		}
		sqlDB, err := model.DB.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
		require.NoError(t, model.InitDB())
		require.NoError(t, model.InitLogDB())
		require.True(t, model.LOG_DB.Migrator().HasTable(&model.ReconciliationDailySummary{}))
		require.True(t, model.LOG_DB.Migrator().HasTable(&model.ReconciliationArchiveState{}))
	})

	require.NoError(t, model.LOG_DB.Exec("DELETE FROM reconciliation_archive_states").Error)
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM reconciliation_daily_summaries").Error)
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)

	today := localDayStart(time.Now(), location)
	dayMinus3 := today.AddDate(0, 0, -3)
	dayMinus2 := today.AddDate(0, 0, -2)
	yesterday := today.AddDate(0, 0, -1)

	logs := []model.Log{
		{UserId: 101, Username: "alice", CreatedAt: dayMinus3.Add(time.Second).Unix(), Type: model.LogTypeConsume, ModelName: "model-a", Quota: 100},
		{UserId: 101, Username: "alice", CreatedAt: dayMinus3.Add(23*time.Hour + 59*time.Minute + 59*time.Second).Unix(), Type: model.LogTypeConsume, ModelName: "model-a", Quota: 200},
		{UserId: 202, Username: "bob", CreatedAt: dayMinus2.Add(time.Hour).Unix(), Type: model.LogTypeConsume, ModelName: "model-b", Quota: 300},
		{UserId: 101, Username: "alice", CreatedAt: yesterday.Add(time.Hour).Unix(), Type: model.LogTypeConsume, ModelName: "model-a", Quota: 400},
		{UserId: 202, Username: "bob", CreatedAt: yesterday.Add(2 * time.Hour).Unix(), Type: model.LogTypeConsume, ModelName: "model-b", Quota: 500},
		{UserId: 101, Username: "alice", CreatedAt: today.Unix(), Type: model.LogTypeConsume, ModelName: "model-a", Quota: 600},
		{UserId: 303, Username: "carol", CreatedAt: today.Add(time.Hour).Unix(), Type: model.LogTypeConsume, ModelName: "model-c", Quota: 700},
		{UserId: 303, Username: "carol", CreatedAt: today.Add(2 * time.Hour).Unix(), Type: model.LogTypeManage, ModelName: "ignored", Quota: 999999},
	}
	// Ensure the result spans multiple pages without changing the archived-day
	// fixtures used by the scheduler checks.
	for index := 0; index < 25; index++ {
		logs = append(logs, model.Log{
			UserId:    404,
			Username:  "pagination",
			CreatedAt: today.Add(time.Duration(index+3) * time.Minute).Unix(),
			Type:      model.LogTypeConsume,
			ModelName: fmt.Sprintf("page-model-%02d", index),
			Quota:     index + 1,
		})
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)

	fullFilter := dto.ReconciliationFilter{
		StartTimestamp: dayMinus3.Unix(),
		EndTimestamp:   today.AddDate(0, 0, 1).Unix(),
		Granularity:    "day",
	}
	expectedQuota := int64(100 + 200 + 300 + 400 + 500 + 600 + 700 + (25*26)/2)

	t.Run("gradual scheduler backfill", func(t *testing.T) {
		reconciliationArchiveRunning.Store(false)
		require.True(t, runReconciliationArchiveOnce(), "first run should archive yesterday")
		state, stateErr := model.GetReconciliationArchiveState(context.Background())
		require.NoError(t, stateErr)
		require.NotNil(t, state)
		require.Equal(t, yesterday.Unix(), state.SummaryStart)
		require.Equal(t, yesterday.Unix(), state.ArchivedThrough)

		// Even during gradual backfill, archived and raw ranges must form one
		// complete result without overlap.
		assertPageTotals(t, fullFilter, expectedQuota, 10)

		require.True(t, runReconciliationArchiveOnce(), "second run should backfill the adjacent previous day")
		state, stateErr = model.GetReconciliationArchiveState(context.Background())
		require.NoError(t, stateErr)
		require.Equal(t, dayMinus2.Unix(), state.SummaryStart)
		require.Equal(t, yesterday.Unix(), state.ArchivedThrough)
		assertPageTotals(t, fullFilter, expectedQuota, 10)

		require.True(t, runReconciliationArchiveOnce(), "third run should reach the earliest consume day")
		state, stateErr = model.GetReconciliationArchiveState(context.Background())
		require.NoError(t, stateErr)
		require.Equal(t, dayMinus3.Unix(), state.SummaryStart)
		require.Equal(t, yesterday.Unix(), state.ArchivedThrough)
		assertPageTotals(t, fullFilter, expectedQuota, 10)

		require.False(t, runReconciliationArchiveOnce(), "backfill should become idle after reaching the earliest log")
	})

	t.Run("day month filters pagination and empty totals", func(t *testing.T) {
		dayPage := assertPageTotals(t, fullFilter, expectedQuota, 7)
		require.Greater(t, dayPage.Total, int64(20))
		require.Len(t, dayPage.Items, 7)

		secondPage, pageErr := GetReconciliationBillsPage(context.Background(), fullFilter, 2, 7)
		require.NoError(t, pageErr)
		require.Equal(t, dayPage.Total, secondPage.Total)
		require.Equal(t, dayPage.TotalQuota, secondPage.TotalQuota)
		require.Equal(t, dayPage.TotalAmountUSD, secondPage.TotalAmountUSD)

		monthFilter := fullFilter
		monthFilter.Granularity = "month"
		monthPage := assertPageTotals(t, monthFilter, expectedQuota, 100)
		require.Equal(t, dayPage.TotalQuota, monthPage.TotalQuota)
		require.Equal(t, dayPage.TotalAmountUSD, monthPage.TotalAmountUSD)

		aliceFilter := fullFilter
		aliceFilter.UserID = 101
		alicePage := assertPageTotals(t, aliceFilter, 1300, 100)
		require.NotEmpty(t, alicePage.Items)
		for _, item := range alicePage.Items {
			require.Equal(t, 101, item.UserID)
		}

		modelFilter := fullFilter
		modelFilter.ModelName = "model-b"
		modelPage := assertPageTotals(t, modelFilter, 800, 100)
		for _, item := range modelPage.Items {
			require.Equal(t, "model-b", item.ModelName)
		}

		boundaryFilter := dto.ReconciliationFilter{
			StartTimestamp: dayMinus3.Unix(),
			EndTimestamp:   dayMinus2.Unix(),
			Granularity:    "day",
		}
		boundaryPage := assertPageTotals(t, boundaryFilter, 300, 100)
		require.Len(t, boundaryPage.Items, 1)
		require.Equal(t, dayMinus3.Format(time.DateOnly), boundaryPage.Items[0].Period)

		emptyFilter := fullFilter
		emptyFilter.UserID = 999999
		emptyPage := assertPageTotals(t, emptyFilter, 0, 100)
		require.Zero(t, emptyPage.Total)
		require.Empty(t, emptyPage.Items)
		require.Zero(t, emptyPage.TotalAmountUSD)
	})

	t.Run("late log requires idempotent day rebuild", func(t *testing.T) {
		lateQuota := 77
		require.NoError(t, model.LOG_DB.Create(&model.Log{
			UserId:    202,
			Username:  "bob",
			CreatedAt: dayMinus2.Add(3 * time.Hour).Unix(),
			Type:      model.LogTypeConsume,
			ModelName: "model-b",
			Quota:     lateQuota,
		}).Error)

		// Closed archived ranges intentionally do not read raw logs again. The
		// documented repair path is an explicit idempotent rebuild of that day.
		stalePage := assertPageTotals(t, fullFilter, expectedQuota, 100)
		require.EqualValues(t, expectedQuota, stalePage.TotalQuota)

		rows, rebuildErr := model.RebuildReconciliationDailySummary(context.Background(), dayMinus2.Unix())
		require.NoError(t, rebuildErr)
		require.Equal(t, 1, rows)
		expectedQuota += int64(lateQuota)
		assertPageTotals(t, fullFilter, expectedQuota, 100)

		rows, rebuildErr = model.RebuildReconciliationDailySummary(context.Background(), dayMinus2.Unix())
		require.NoError(t, rebuildErr)
		require.Equal(t, 1, rows)
		assertPageTotals(t, fullFilter, expectedQuota, 100)
	})

	t.Run("concurrent same-day master rebuild is data-safe", func(t *testing.T) {
		const workers = 4
		var waitGroup sync.WaitGroup
		errors := make(chan error, workers)
		for index := 0; index < workers; index++ {
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				_, rebuildErr := model.RebuildReconciliationDailySummary(context.Background(), yesterday.Unix())
				errors <- rebuildErr
			}()
		}
		waitGroup.Wait()
		close(errors)
		successes := 0
		for rebuildErr := range errors {
			if rebuildErr == nil {
				successes++
				continue
			}
			if common.UsingLogDatabase(common.DatabaseTypeSQLite) {
				require.ErrorContains(t, rebuildErr, "reconciliation archive changed during aggregation; retry required")
				_, retryErr := model.RebuildReconciliationDailySummary(context.Background(), yesterday.Unix())
				require.NoError(t, retryErr, "an optimistic-lock loser can retry without duplicating summaries")
			} else {
				require.NoError(t, rebuildErr)
			}
		}
		require.Positive(t, successes)

		var rows int64
		require.NoError(t, model.LOG_DB.Model(&model.ReconciliationDailySummary{}).
			Where("day_start = ?", yesterday.Unix()).Count(&rows).Error)
		require.EqualValues(t, 2, rows)
		assertPageTotals(t, fullFilter, expectedQuota, 100)
	})
}

func assertPageTotals(
	t *testing.T,
	filter dto.ReconciliationFilter,
	expectedQuota int64,
	pageSize int,
) dto.ReconciliationBillsPage {
	t.Helper()
	page, err := GetReconciliationBillsPage(context.Background(), filter, 1, pageSize)
	require.NoError(t, err)
	require.Equal(t, expectedQuota, page.TotalQuota)
	require.InDelta(t, float64(expectedQuota)/common.QuotaPerUnit, page.TotalAmountUSD, 1e-12)
	return page
}

func localDayStart(value time.Time, location *time.Location) time.Time {
	value = value.In(location)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}
