package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestBuildReconciliationFilter(t *testing.T) {
	filter, err := BuildReconciliationFilter("2026-09-01", "2026-09-08", "day", 12, "gpt-5")
	require.NoError(t, err)
	require.Equal(t, 12, filter.UserID)
	require.Equal(t, "gpt-5", filter.ModelName)
	require.Equal(t, "day", filter.Granularity)
	require.Equal(t, int64(8*24*time.Hour/time.Second), filter.EndTimestamp-filter.StartTimestamp)
}

func TestReconciliationSkippedCivilDateReturnsEmptyResults(t *testing.T) {
	originalDB, originalLocation := model.LOG_DB, time.Local
	t.Cleanup(func() { model.LOG_DB, time.Local = originalDB, originalLocation })
	var err error
	time.Local, err = time.LoadLocation("Pacific/Apia")
	require.NoError(t, err)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.ReconciliationDailySummary{}, &model.ReconciliationArchiveState{}))
	model.LOG_DB = db
	connection, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	// Data immediately after the skipped date must not leak into the result.
	boundary := common.ReconciliationDateStart(2011, 12, 31, time.Local)
	require.NoError(t, db.Create(&model.Log{UserId: 1, Username: "alice", Type: model.LogTypeConsume,
		ModelName: "model", CreatedAt: boundary.Unix(), Quota: 100}).Error)
	for _, granularity := range []string{"day", "month"} {
		filter, err := BuildReconciliationFilter("2011-12-30", "2011-12-30", granularity, 0, "")
		require.NoError(t, err)
		require.Equal(t, filter.StartTimestamp, filter.EndTimestamp)
		page, err := GetReconciliationBillsPage(context.Background(), filter, 1, 20)
		require.NoError(t, err)
		require.Empty(t, page.Items)
		require.Zero(t, page.Total)
		require.Zero(t, page.TotalQuota)
		require.Zero(t, page.TotalAmountUSD)
		count, err := model.CountReconciliationBills(context.Background(), filter)
		require.NoError(t, err)
		require.Zero(t, count)
		items, err := model.GetReconciliationBills(context.Background(), filter, 0, 20)
		require.NoError(t, err)
		require.Empty(t, items)
		_, err = GetReconciliationExportRows(context.Background(), filter)
		require.ErrorIs(t, err, ErrReconciliationNoExportData)
	}
}

func TestBuildReconciliationFilterValidation(t *testing.T) {
	testCases := []struct {
		name        string
		startDate   string
		endDate     string
		granularity string
		expected    error
	}{
		{name: "missing date", startDate: "", endDate: "2026-09-08", granularity: "day", expected: ErrReconciliationDateRequired},
		{name: "invalid date", startDate: "2026/09/01", endDate: "2026-09-08", granularity: "day", expected: ErrReconciliationDateFormat},
		{name: "reversed date", startDate: "2026-09-09", endDate: "2026-09-08", granularity: "day", expected: ErrReconciliationDateOrder},
		{name: "too large", startDate: "2025-09-08", endDate: "2026-09-09", granularity: "day", expected: ErrReconciliationDateRange},
		{name: "invalid granularity", startDate: "2026-09-01", endDate: "2026-09-08", granularity: "week", expected: ErrReconciliationGranularity},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := BuildReconciliationFilter(testCase.startDate, testCase.endDate, testCase.granularity, 0, "")
			require.True(t, errors.Is(err, testCase.expected))
		})
	}
}

func TestBuildReconciliationWorkbook(t *testing.T) {
	originalLogDB := model.LOG_DB
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	generalSetting := operation_setting.GetGeneralSetting()
	originalDisplayType := generalSetting.QuotaDisplayType
	t.Cleanup(func() {
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		generalSetting.QuotaDisplayType = originalDisplayType
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Log{},
		&model.ReconciliationDailySummary{},
		&model.ReconciliationArchiveState{},
	))
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	generalSetting.QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD

	day, err := time.ParseInLocation(time.DateOnly, "2026-09-08", time.Local)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Log{
		UserId:    7,
		Username:  "alice",
		CreatedAt: day.Add(time.Hour).Unix(),
		Type:      model.LogTypeConsume,
		ModelName: "gpt-5",
		Quota:     int(common.QuotaPerUnit * 2),
	}).Error)

	file, err := BuildReconciliationWorkbook(context.Background(), dto.ReconciliationFilter{
		StartTimestamp: day.Unix(),
		EndTimestamp:   day.AddDate(0, 0, 1).Unix(),
		Granularity:    "day",
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	require.Equal(t, []string{reconciliationSheetName}, file.GetSheetList())
	header, err := file.GetRows(reconciliationSheetName)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(header), 2)
	require.Equal(t, []string{"用户", "时间", "模型", "费用"}, header[0])
	require.Equal(t, []string{"alice", "2026-09-08", "gpt-5"}, header[1][:3])
	rawAmount, err := file.GetCellValue(reconciliationSheetName, "D2", excelize.Options{RawCellValue: true})
	require.NoError(t, err)
	amount, err := strconv.ParseFloat(rawAmount, 64)
	require.NoError(t, err)
	require.Equal(t, 2.0, amount)
}

func TestGetReconciliationBillsPageIncludesFullFilteredTotal(t *testing.T) {
	originalLogDB := model.LOG_DB
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Log{},
		&model.ReconciliationDailySummary{},
		&model.ReconciliationArchiveState{},
	))
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	day, err := time.ParseInLocation(time.DateOnly, "2026-09-08", time.Local)
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]model.Log{
		{
			UserId:    7,
			Username:  "alice",
			CreatedAt: day.Add(time.Hour).Unix(),
			Type:      model.LogTypeConsume,
			ModelName: "gpt-5",
			Quota:     int(common.QuotaPerUnit),
		},
		{
			UserId:    8,
			Username:  "bob",
			CreatedAt: day.Add(2 * time.Hour).Unix(),
			Type:      model.LogTypeConsume,
			ModelName: "claude",
			Quota:     int(common.QuotaPerUnit * 2),
		},
	}).Error)

	page, err := GetReconciliationBillsPage(context.Background(), dto.ReconciliationFilter{
		StartTimestamp: day.Unix(),
		EndTimestamp:   day.AddDate(0, 0, 1).Unix(),
		Granularity:    "day",
	}, 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, page.Total)
	require.EqualValues(t, common.QuotaPerUnit*3, page.TotalQuota)
	require.Equal(t, 3.0, page.TotalAmountUSD)
	require.Len(t, page.Items, 1, "the total must cover all filtered rows, not only the current page")

	filter := dto.ReconciliationFilter{
		StartTimestamp: day.Unix(), EndTimestamp: day.AddDate(0, 0, 1).Unix(), Granularity: "day", UserID: 7,
	}
	inserted := false
	var insertErr error
	model.LOG_DB = db.Session(&gorm.Session{Logger: reconciliationAfterQueryLogger{
		Interface: db.Logger,
		afterQuery: func(query string) {
			// Insert immediately after the first aggregate statement completes.
			// A separate summary/list implementation would now return mismatched
			// amounts, whereas one statement sees a single consistent snapshot.
			if !inserted && strings.HasPrefix(query, "SELECT") && strings.Contains(query, "SUM(") {
				inserted = true
				insertErr = db.Create(&model.Log{UserId: 7, Username: "alice", Type: model.LogTypeConsume,
					CreatedAt: day.Add(3 * time.Hour).Unix(), ModelName: "gpt-5", Quota: int(common.QuotaPerUnit)}).Error
			}
		},
	}})
	page, err = GetReconciliationBillsPage(context.Background(), filter, 1, 20)
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, insertErr)
	require.Len(t, page.Items, 1)
	require.Equal(t, page.Items[0].Quota, page.TotalQuota)
	require.Equal(t, page.Items[0].AmountUSD, page.TotalAmountUSD)
	page, err = GetReconciliationBillsPage(context.Background(), filter, 1, 20)
	require.NoError(t, err)
	require.Equal(t, 2.0, page.TotalAmountUSD, "the next request must include the newly inserted consumption")
}

type reconciliationAfterQueryLogger struct {
	gormlogger.Interface
	afterQuery func(string)
}

func (l reconciliationAfterQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	query, rows := fc()
	l.Interface.Trace(ctx, begin, func() (string, int64) { return query, rows }, err)
	if err == nil {
		l.afterQuery(query)
	}
}
