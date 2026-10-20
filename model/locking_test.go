package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

// lockForUpdate must emit FOR UPDATE on databases that support it and skip
// it on SQLite, where the syntax does not exist.
//
// The dummy dialector is used because SQLite drivers strip locking clauses
// from the generated SQL, which would mask what the helper itself does.
func TestLockForUpdateEmitsRowLock(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			db, err := gorm.Open(lockTestDialector{name: engine}, &gorm.Config{DryRun: true})
			require.NoError(t, err)
			var rows []Redemption
			query := lockForUpdate(db).Where("id = ?", 1).Find(&rows).Statement.SQL.String()
			if engine == "sqlite" {
				assert.NotContains(t, query, "FOR UPDATE")
			} else {
				assert.Contains(t, query, "FOR UPDATE")
			}
		})
	}
}

type lockTestDialector struct {
	tests.DummyDialector
	name string
}

func (d lockTestDialector) Name() string { return d.name }
