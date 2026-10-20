//go:build mysqlqa

package model

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Point MYSQL_RECONCILIATION_MIGRATION_DSN at an isolated server whose default
// row format is COMPACT. This test creates its own databases and removes only
// those databases; it never changes global server settings or existing data.
func TestReconciliationMySQLMigrationCompatibility(t *testing.T) {
	dsn := os.Getenv("MYSQL_RECONCILIATION_MIGRATION_DSN")
	if dsn == "" {
		t.Skip("MYSQL_RECONCILIATION_MIGRATION_DSN is not set")
	}
	root, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	connection, err := root.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	var defaultRowFormat string
	require.NoError(t, root.Raw("SELECT @@innodb_default_row_format").Scan(&defaultRowFormat).Error)
	require.Equal(t, "compact", defaultRowFormat, "use a dedicated COMPACT-default server")
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing_table_%t", existing), func(t *testing.T) {
			name := fmt.Sprintf("reconciliation_migration_test_%d", time.Now().UnixNano())
			require.NoError(t, root.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci").Error)
			t.Cleanup(func() { require.NoError(t, root.Exec("DROP DATABASE `"+name+"`").Error) })
			require.NoError(t, root.Exec("USE `"+name+"`").Error)
			db := root.Session(&gorm.Session{NewDB: true})
			original := LOG_DB
			LOG_DB = db
			t.Cleanup(func() { LOG_DB = original })
			require.NoError(t, db.Set("gorm:table_options", "ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin").AutoMigrate(&Log{}))
			// The two source fields need not have the same collation.
			require.NoError(t, db.Exec("ALTER TABLE logs MODIFY username varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT ''").Error)
			logDDL := func() string {
				var result struct {
					Table string
					DDL   string `gorm:"column:Create Table"`
				}
				require.NoError(t, db.Raw("SHOW CREATE TABLE logs").Scan(&result).Error)
				return result.DDL
			}
			before := logDDL()
			day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
			if existing {
				// Simulate an existing COMPACT summary with shorter names, old
				// default CI collation and the original 64-character username.
				require.NoError(t, db.Exec(`CREATE TABLE reconciliation_daily_summaries (
id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
day_start bigint NOT NULL, user_id bigint NOT NULL,
username varchar(64) NOT NULL DEFAULT '', model_name varchar(120) NOT NULL DEFAULT '',
log_count bigint NOT NULL DEFAULT 0, quota bigint NOT NULL DEFAULT 0,
UNIQUE KEY idx_reconciliation_daily_dimension (day_start, user_id, model_name),
KEY idx_reconciliation_daily_model (model_name)
) ENGINE=InnoDB ROW_FORMAT=COMPACT`).Error)
				require.NoError(t, db.Create(&ReconciliationDailySummary{DayStart: day.Unix(), UserID: 1, Username: "alice", ModelName: "model-a", LogCount: 1, Quota: 11}).Error)
			}
			require.NoError(t, migrateReconciliationTables(db))
			for _, column := range []string{"username", "model_name"} {
				var collations []string
				require.NoError(t, db.Raw("SELECT collation_name FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN ('logs','reconciliation_daily_summaries') AND column_name=?", column).Scan(&collations).Error)
				require.Len(t, collations, 2)
				require.Equal(t, collations[0], collations[1], column+" must inherit source comparison rules")
			}
			if existing {
				var summary ReconciliationDailySummary
				require.NoError(t, db.First(&summary).Error)
				require.EqualValues(t, 11, summary.Quota, "upgrade must preserve existing summary rows")
			}
			require.Equal(t, before, logDDL(), "migration must not change logs")
			var rowFormat string
			require.NoError(t, db.Raw("SELECT row_format FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='reconciliation_daily_summaries'").Scan(&rowFormat).Error)
			require.Equal(t, "Dynamic", rowFormat)
			logs := []Log{
				{UserId: 1, Username: "alice", ModelName: "model-a", Type: LogTypeConsume, CreatedAt: day.Unix() + 1, Quota: 11},
				{UserId: 1, Username: "alice", ModelName: "MODEL-A", Type: LogTypeConsume, CreatedAt: day.Unix() + 2, Quota: 22},
				{UserId: 1, Username: "alice", ModelName: "model-a", Type: LogTypeConsume, CreatedAt: day.AddDate(0, 0, 1).Unix() + 1, Quota: 33},
			}
			require.NoError(t, db.Create(&logs).Error)
			filter := dto.ReconciliationFilter{StartTimestamp: day.Unix(), EndTimestamp: day.AddDate(0, 0, 2).Unix(), Granularity: "month"}
			raw, total, quota, err := GetReconciliationBillsPage(context.Background(), filter, 0, 20)
			require.NoError(t, err)
			require.EqualValues(t, 2, total)
			require.EqualValues(t, 66, quota)
			count, err := RebuildReconciliationDailySummary(context.Background(), day.Unix())
			require.NoError(t, err, "case-sensitive model groups must both archive")
			require.Equal(t, 2, count)
			assertResults := func() {
				items, count, sum, err := GetReconciliationBillsPage(context.Background(), filter, 0, 20)
				require.NoError(t, err, "mixed sources must have compatible collations")
				require.Equal(t, raw, items)
				require.Equal(t, total, count)
				require.Equal(t, quota, sum)
			}
			assertResults()
			require.NoError(t, migrateReconciliationTables(db), "repeat startup must not shrink or reset the table")
			assertResults()
		})
	}
}
