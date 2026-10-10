package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReconciliationBillsAggregateConsumeLogs(t *testing.T) {
	originalLogDB := LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &ReconciliationDailySummary{}, &ReconciliationArchiveState{}))
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dayOne, err := time.ParseInLocation(time.DateOnly, "2026-09-01", time.Local)
	require.NoError(t, err)
	dayTwo := dayOne.AddDate(0, 0, 1)
	logs := []Log{
		{UserId: 1, Username: "alice", CreatedAt: dayOne.Add(time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-5", Quota: 100},
		{UserId: 1, Username: "", CreatedAt: dayOne.Add(2 * time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-5", Quota: 250},
		{UserId: 2, Username: "bob", CreatedAt: dayTwo.Add(time.Hour).Unix(), Type: LogTypeConsume, ModelName: "claude", Quota: 400},
		{UserId: 1, Username: "alice", CreatedAt: dayOne.Add(3 * time.Hour).Unix(), Type: LogTypeRefund, ModelName: "gpt-5", Quota: 999},
	}
	require.NoError(t, db.Create(&logs).Error)

	filter := dto.ReconciliationFilter{
		StartTimestamp: dayOne.Unix(),
		EndTimestamp:   dayTwo.AddDate(0, 0, 1).Unix(),
		Granularity:    "day",
	}
	total, totalQuota, err := GetReconciliationBillsSummary(context.Background(), filter)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.EqualValues(t, 750, totalQuota)

	userFilter := filter
	userFilter.UserID = 1
	filteredTotal, filteredQuota, err := GetReconciliationBillsSummary(context.Background(), userFilter)
	require.NoError(t, err)
	require.EqualValues(t, 1, filteredTotal)
	require.EqualValues(t, 350, filteredQuota)

	items, err := GetReconciliationBills(context.Background(), filter, 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "2026-09-02", items[0].Period)
	require.Equal(t, "bob", items[0].Username)
	require.EqualValues(t, 400, items[0].Quota)
	require.Equal(t, "2026-09-01", items[1].Period)
	require.Equal(t, "alice", items[1].Username)
	require.EqualValues(t, 350, items[1].Quota)
}

func TestReconciliationPeriodExpression(t *testing.T) {
	originalLocation := time.Local
	t.Cleanup(func() {
		time.Local = originalLocation
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, testCase := range []struct {
		name, zone, start, end, timestamp, granularity, expected string
	}{
		{"Shanghai midnight", "Asia/Shanghai", "2026-09-30", "2026-10-02", "2026-10-01 00:00:00", "day", "2026-10-01"},
		{"Shanghai month end", "Asia/Shanghai", "2026-09-30", "2026-10-02", "2026-09-30 23:59:59", "month", "2026-09"},
		{"Shanghai month start", "Asia/Shanghai", "2026-09-30", "2026-10-02", "2026-10-01 00:00:00", "month", "2026-10"},
		{"DST short day end", "America/New_York", "2026-03-08", "2026-03-10", "2026-03-08 23:59:59", "day", "2026-03-08"},
		{"DST short day next midnight", "America/New_York", "2026-03-08", "2026-03-10", "2026-03-09 00:00:00", "day", "2026-03-09"},
		{"DST long day end", "America/New_York", "2026-11-01", "2026-11-03", "2026-11-01 23:59:59", "day", "2026-11-01"},
		{"DST long day next midnight", "America/New_York", "2026-11-01", "2026-11-03", "2026-11-02 00:00:00", "day", "2026-11-02"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			time.Local, err = time.LoadLocation(testCase.zone)
			require.NoError(t, err)
			start, parseErr := time.ParseInLocation(time.DateOnly, testCase.start, time.Local)
			require.NoError(t, parseErr)
			end, parseErr := time.ParseInLocation(time.DateOnly, testCase.end, time.Local)
			require.NoError(t, parseErr)
			value, parseErr := time.ParseInLocation(time.DateTime, testCase.timestamp, time.Local)
			require.NoError(t, parseErr)
			expression := reconciliationPeriodExpression(testCase.granularity, "created_at", reconciliationTimeRange{start.Unix(), end.Unix()})
			var period string
			require.NoError(t, db.Raw("SELECT "+expression+" AS period FROM (SELECT ? AS created_at) AS sample", value.Unix()).Scan(&period).Error)
			require.Equal(t, testCase.expected, period)
		})
	}
}

func TestReconciliationOptionsUseConsumeLogs(t *testing.T) {
	originalLogDB := LOG_DB
	t.Cleanup(func() {
		LOG_DB = originalLogDB
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &ReconciliationDailySummary{}, &ReconciliationArchiveState{}))
	LOG_DB = db

	logs := []Log{
		{UserId: 1, Username: "alice", Type: LogTypeConsume, ModelName: "deepseek-v4-flash"},
		{UserId: 1, Username: "", Type: LogTypeConsume, ModelName: "deepseek-reasoner"},
		{UserId: 2, Username: "bob", Type: LogTypeConsume, ModelName: "deepseek-v4-flash"},
		{UserId: 3, Username: "refund-user", Type: LogTypeRefund, ModelName: "refund-model"},
		{UserId: 4, Username: "empty-model-user", Type: LogTypeConsume, ModelName: ""},
	}
	require.NoError(t, db.Create(&logs).Error)

	users, total, err := GetReconciliationUserOptions(context.Background(), "ali", 0, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, []dto.ReconciliationUserOption{{UserID: 1, Username: "alice"}}, users)

	users, total, err = GetReconciliationUserOptions(context.Background(), "", 2, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, []dto.ReconciliationUserOption{{UserID: 2, Username: "bob"}}, users)

	models, total, err := GetReconciliationModelOptions(context.Background(), "v4", 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, []dto.ReconciliationModelOption{{ModelName: "deepseek-v4-flash"}}, models)

	models, total, err = GetReconciliationModelOptions(context.Background(), "", 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Equal(t, []dto.ReconciliationModelOption{
		{ModelName: "deepseek-reasoner"},
		{ModelName: "deepseek-v4-flash"},
	}, models)
}

func TestReconciliationArchiveCombinesSummaryAndRawExactlyOnce(t *testing.T) {
	originalLogDB := LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &ReconciliationDailySummary{}, &ReconciliationArchiveState{}))
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dayOne, err := time.ParseInLocation(time.DateOnly, "2026-09-01", time.Local)
	require.NoError(t, err)
	dayTwo := dayOne.AddDate(0, 0, 1)
	logs := []Log{
		{UserId: 1, Username: "alice", CreatedAt: dayOne.Add(time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-5", Quota: 100},
		{UserId: 1, Username: "alice", CreatedAt: dayOne.Add(2 * time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-5", Quota: 250},
		{UserId: 1, Username: "alice", CreatedAt: dayTwo.Add(time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-5", Quota: 400},
		{UserId: 2, Username: "bob", CreatedAt: dayTwo.Add(2 * time.Hour).Unix(), Type: LogTypeConsume, ModelName: "claude", Quota: 500},
	}
	require.NoError(t, db.Create(&logs).Error)

	rows, err := RebuildReconciliationDailySummary(context.Background(), dayOne.Unix())
	require.NoError(t, err)
	require.Equal(t, 1, rows)

	filter := dto.ReconciliationFilter{
		StartTimestamp: dayOne.Unix(),
		EndTimestamp:   dayTwo.AddDate(0, 0, 1).Unix(),
		Granularity:    "day",
	}
	items, err := GetReconciliationBills(context.Background(), filter, 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.EqualValues(t, 350, items[2].Quota, "archived raw logs must not be counted again")
	total, totalQuota, err := GetReconciliationBillsSummary(context.Background(), filter)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.EqualValues(t, 1250, totalQuota, "summary and raw tail totals must not overlap")

	monthFilter := filter
	monthFilter.Granularity = "month"
	monthFilter.UserID = 1
	monthItems, err := GetReconciliationBills(context.Background(), monthFilter, 0, 20)
	require.NoError(t, err)
	require.Equal(t, []dto.ReconciliationBill{{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-5",
		Period:    "2026-09",
		Quota:     750,
	}}, monthItems, "daily summary and raw tail must merge into one monthly row")

	rows, err = RebuildReconciliationDailySummary(context.Background(), dayOne.Unix())
	require.NoError(t, err)
	require.Equal(t, 1, rows)
	var summaryRows int64
	require.NoError(t, db.Model(&ReconciliationDailySummary{}).Count(&summaryRows).Error)
	require.EqualValues(t, 1, summaryRows, "retrying an archived day must replace rather than append")

	users, total, err := GetReconciliationUserOptions(context.Background(), "", 0, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Equal(t, []dto.ReconciliationUserOption{
		{UserID: 1, Username: "alice"},
		{UserID: 2, Username: "bob"},
	}, users)
	models, total, err := GetReconciliationModelOptions(context.Background(), "", 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Equal(t, []dto.ReconciliationModelOption{
		{ModelName: "claude"},
		{ModelName: "gpt-5"},
	}, models)
}

func TestReconciliationArchiveExtendsOnlyAdjacentDays(t *testing.T) {
	state := &ReconciliationArchiveState{}
	day, err := time.ParseInLocation(time.DateOnly, "2026-09-10", time.Local)
	require.NoError(t, err)

	require.NoError(t, updateReconciliationArchiveBounds(state, day.Unix()))
	require.Equal(t, day.Unix(), state.SummaryStart)
	require.Equal(t, day.Unix(), state.ArchivedThrough)

	next := day.AddDate(0, 0, 1)
	require.NoError(t, updateReconciliationArchiveBounds(state, next.Unix()))
	require.Equal(t, next.Unix(), state.ArchivedThrough)

	previous := day.AddDate(0, 0, -1)
	require.NoError(t, updateReconciliationArchiveBounds(state, previous.Unix()))
	require.Equal(t, previous.Unix(), state.SummaryStart)

	err = updateReconciliationArchiveBounds(state, day.AddDate(0, 0, 3).Unix())
	require.Error(t, err)
}
