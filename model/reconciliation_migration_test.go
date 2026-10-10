package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

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
