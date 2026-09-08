package service

import (
	"context"
	"errors"
	"strconv"
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
)

func TestBuildReconciliationFilter(t *testing.T) {
	filter, err := BuildReconciliationFilter("2026-09-01", "2026-09-08", "day", 12, "gpt-5")
	require.NoError(t, err)
	require.Equal(t, 12, filter.UserID)
	require.Equal(t, "gpt-5", filter.ModelName)
	require.Equal(t, "day", filter.Granularity)
	require.Equal(t, int64(8*24*time.Hour/time.Second), filter.EndTimestamp-filter.StartTimestamp)
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
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	generalSetting := operation_setting.GetGeneralSetting()
	originalDisplayType := generalSetting.QuotaDisplayType
	t.Cleanup(func() {
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		generalSetting.QuotaDisplayType = originalDisplayType
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
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
