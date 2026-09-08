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

func TestReconciliationPeriodExpression(t *testing.T) {
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
	})

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.Equal(t, "strftime('%Y-%m', created_at, 'unixepoch', 'localtime')", reconciliationPeriodExpression("month"))

	common.UsingSQLite = false
	common.UsingPostgreSQL = true
	require.Equal(t, "TO_CHAR(TO_TIMESTAMP(created_at), 'YYYY-MM-DD')", reconciliationPeriodExpression("day"))

	common.UsingPostgreSQL = false
	common.UsingMySQL = true
	require.Equal(t, "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d')", reconciliationPeriodExpression("day"))
}
