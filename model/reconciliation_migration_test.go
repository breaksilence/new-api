package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestReconciliationRelationalMigrationAndUpgrade(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var db *gorm.DB
			var err error
			if engine == "sqlite" {
				db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.db")), &gorm.Config{})
			} else {
				dsn := os.Getenv("POSTGRES_RECONCILIATION_MIGRATION_DSN")
				if dsn == "" {
					t.Skip("POSTGRES_RECONCILIATION_MIGRATION_DSN is not set")
				}
				db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
			}
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			if engine == "postgres" {
				schemaName := fmt.Sprintf("reconciliation_qa_%d", time.Now().UnixNano())
				require.NoError(t, db.Exec("CREATE SCHEMA "+schemaName).Error)
				require.NoError(t, db.Exec("SET search_path TO "+schemaName).Error)
				t.Cleanup(func() {
					require.NoError(t, db.Exec("SET search_path TO public").Error)
					require.NoError(t, db.Exec("DROP SCHEMA "+schemaName+" CASCADE").Error)
				})
			}
			originalLogDB := LOG_DB
			originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
			LOG_DB = db
			common.SetLogDatabaseType(common.DatabaseType(engine))
			t.Cleanup(func() {
				LOG_DB = originalLogDB
				common.SetDatabaseTypes(originalMainType, originalLogType)
			})
			// rc.45's existing log schema and data, before the custom tables exist.
			require.NoError(t, db.AutoMigrate(&Log{}))
			day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
			name := "model-" + strings.Repeat("a", 250)
			logs := []Log{
				{UserId: 1, Username: "alice", ModelName: name, Type: LogTypeConsume, CreatedAt: day.Unix() + 1, Quota: 11},
				{UserId: 1, Username: "alice", ModelName: name, Type: LogTypeConsume, CreatedAt: day.AddDate(0, 0, 1).Unix() + 1, Quota: 22},
			}
			require.NoError(t, db.Create(&logs).Error)
			require.False(t, db.Migrator().HasTable(&ReconciliationDailySummary{}))
			require.NoError(t, migrateLOGDB(), "startup creates custom tables alongside existing logs")
			filter := dto.ReconciliationFilter{StartTimestamp: day.Unix(), EndTimestamp: day.AddDate(0, 0, 2).Unix(), Granularity: "month"}
			raw, total, quota, err := GetReconciliationBillsPage(context.Background(), filter, 0, 20)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.EqualValues(t, 33, quota)
			rows, err := RebuildReconciliationDailySummary(context.Background(), day.Unix())
			require.NoError(t, err)
			require.Equal(t, 1, rows)
			for range 2 {
				require.NoError(t, migrateLOGDB(), "restarting must preserve archived data and indexes")
				items, count, sum, queryErr := GetReconciliationBillsPage(context.Background(), filter, 0, 20)
				require.NoError(t, queryErr)
				require.Equal(t, raw, items)
				require.Equal(t, total, count)
				require.Equal(t, quota, sum)
				var unchanged []Log
				require.NoError(t, db.Order("id").Find(&unchanged).Error)
				require.Equal(t, logs, unchanged, "upgrade and archive never modify source logs")
			}
			require.True(t, db.Migrator().HasIndex(&ReconciliationDailySummary{}, "idx_reconciliation_daily_dimension"))
			duplicate := ReconciliationDailySummary{DayStart: day.Unix(), UserID: 1, Username: "alice", ModelName: reconciliationText(name)}
			require.Error(t, db.Create(&duplicate).Error, "upgrades preserve dimension uniqueness")
		})
	}
}

func TestReconciliationPostgresSummaryPreservesLogTextCapacity(t *testing.T) {
	// No PostgreSQL server required: exercise the actual dialect/migrator DDL
	// type resolution, including the custom type used by AutoMigrate on upgrade.
	db, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"),
		&gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	for _, value := range []any{&Log{}, &ReconciliationDailySummary{}} {
		statement := &gorm.Statement{DB: db}
		require.NoError(t, statement.Parse(value))
		for _, name := range []string{"Username", "ModelName"} {
			field := statement.Schema.LookUpField(name)
			require.Equal(t, "text", db.Migrator().FullDataTypeOf(field).SQL[:4])
			require.NotContains(t, db.Migrator().FullDataTypeOf(field).SQL, "varchar")
		}
	}
}
