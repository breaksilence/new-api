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
	require.NoError(t, db.AutoMigrate(&Log{}))
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
	total, err := CountReconciliationBills(context.Background(), filter)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)

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

func TestReconciliationPeriodPlanAvoidsDatabaseDateFormatting(t *testing.T) {
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
	})

	start, err := time.ParseInLocation(time.DateOnly, "2026-09-01", time.Local)
	require.NoError(t, err)
	filter := dto.ReconciliationFilter{
		StartTimestamp: start.Unix(),
		EndTimestamp:   start.AddDate(0, 0, 30).Unix(),
		Granularity:    "day",
	}

	for _, database := range []string{"sqlite", "postgres", "mysql"} {
		common.UsingSQLite = database == "sqlite"
		common.UsingPostgreSQL = database == "postgres"
		common.UsingMySQL = database == "mysql"

		plan := buildReconciliationPeriodPlan(filter)
		require.Len(t, plan.labels, 30)
		require.Equal(t, "2026-09-01", plan.labels[0])
		require.Equal(t, "2026-09-30", plan.labels[29])
		require.NotContains(t, plan.expression, "DATE_FORMAT")
		require.NotContains(t, plan.expression, "FROM_UNIXTIME")
		require.NotContains(t, plan.expression, "strftime")
		require.NotContains(t, plan.expression, "TO_CHAR")
	}
}

func TestReconciliationBillsAggregateByMonthWithoutDateFormatting(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&Log{}))
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	start, err := time.ParseInLocation(time.DateOnly, "2026-09-15", time.Local)
	require.NoError(t, err)
	end, err := time.ParseInLocation(time.DateOnly, "2026-10-16", time.Local)
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, Username: "alice", CreatedAt: start.Add(time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-5", Quota: 100},
		{UserId: 1, Username: "alice", CreatedAt: start.AddDate(0, 1, 0).Unix(), Type: LogTypeConsume, ModelName: "gpt-5", Quota: 200},
	}).Error)

	items, err := GetReconciliationBills(context.Background(), dto.ReconciliationFilter{
		StartTimestamp: start.Unix(),
		EndTimestamp:   end.Unix(),
		Granularity:    "month",
	}, 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "2026-10", items[0].Period)
	require.EqualValues(t, 200, items[0].Quota)
	require.Equal(t, "2026-09", items[1].Period)
	require.EqualValues(t, 100, items[1].Quota)
}

func TestReconciliationOptionsUseConsumeLogs(t *testing.T) {
	originalLogDB := LOG_DB
	t.Cleanup(func() {
		LOG_DB = originalLogDB
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
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
