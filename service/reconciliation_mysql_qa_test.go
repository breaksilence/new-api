//go:build mysqlqa

package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// TestMySQLReconciliationEndToEndQA is an opt-in QA test for a disposable
// MySQL database. Run with:
//
//	MYSQL_QA_DSN='user:pass@tcp(127.0.0.1:33306)/db?parseTime=true&loc=Local' \
//	go test -tags mysqlqa ./service -run TestMySQLReconciliationEndToEndQA -v
//
// The database must be dedicated to this test because it deletes reconciliation
// logs and summaries before inserting deterministic fixtures.
func TestMySQLReconciliationEndToEndQA(t *testing.T) {
	dsn := os.Getenv("MYSQL_QA_DSN")
	if dsn == "" {
		t.Skip("MYSQL_QA_DSN is not set")
	}

	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	originalLocation := time.Local
	originalMaster := common.IsMasterNode
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	originalLogSQLDSN, hadLogSQLDSN := os.LookupEnv("LOG_SQL_DSN")
	t.Cleanup(func() {
		time.Local = originalLocation
		common.IsMasterNode = originalMaster
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
	common.UsingSQLite = false
	common.UsingPostgreSQL = false
	common.UsingMySQL = false
	require.NoError(t, os.Setenv("SQL_DSN", dsn))
	require.NoError(t, os.Unsetenv("LOG_SQL_DSN"))

	t.Run("automatic migration and repeated startup", func(t *testing.T) {
		require.NoError(t, model.InitDB())
		require.NoError(t, model.InitLogDB())
		require.True(t, model.LOG_DB.Migrator().HasTable(&model.ReconciliationDailySummary{}))
		require.True(t, model.LOG_DB.Migrator().HasTable(&model.ReconciliationArchiveState{}))
		require.True(t, model.LOG_DB.Migrator().HasIndex(&model.ReconciliationDailySummary{}, "idx_reconciliation_daily_dimension"))

		// A second startup must leave the schema usable and unchanged.
		// Reset process-global dialect flags to simulate a fresh process.
		common.UsingSQLite = false
		common.UsingPostgreSQL = false
		common.UsingMySQL = false
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
		for rebuildErr := range errors {
			require.NoError(t, rebuildErr)
		}

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
