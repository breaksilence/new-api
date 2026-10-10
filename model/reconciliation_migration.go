package model

import (
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// PostgreSQL logs store these values as text, including model suffixes added
// by relay adapters. Do not introduce a shorter limit in their summaries.
type reconciliationText string

func (reconciliationText) GormDataType() string { return "string" }

func (reconciliationText) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	if db.Dialector.Name() == "mysql" {
		return fmt.Sprintf("varchar(%d)", field.Size)
	}
	return "text"
}

// Migrate only reconciliation-owned tables; never alter the source logs.
// Explicit row format is required even when all existing tables are DYNAMIC:
// the server's default for NEW tables may still be COMPACT (767-byte indexes).
func migrateReconciliationTables(db *gorm.DB) error {
	if db.Dialector.Name() != "mysql" {
		return db.AutoMigrate(&ReconciliationDailySummary{}, &ReconciliationArchiveState{})
	}
	var rowFormat string
	if err := db.Raw("SELECT row_format FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
		(ReconciliationDailySummary{}).TableName()).Scan(&rowFormat).Error; err != nil {
		return err
	}
	if strings.EqualFold(rowFormat, "Compact") || strings.EqualFold(rowFormat, "Redundant") {
		if err := db.Exec("ALTER TABLE reconciliation_daily_summaries ROW_FORMAT=DYNAMIC").Error; err != nil {
			return err
		}
	}
	if err := db.Set("gorm:table_options", "ENGINE=InnoDB ROW_FORMAT=DYNAMIC").
		AutoMigrate(&ReconciliationDailySummary{}, &ReconciliationArchiveState{}); err != nil {
		return err
	}
	return alignReconciliationMySQLCollations(db)
}

type reconciliationMySQLColumn struct {
	ColumnName       string
	ColumnType       string
	CharacterSetName string
	CollationName    string
}

var reconciliationSQLIdentifier = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
var reconciliationSQLVarchar = regexp.MustCompile(`^varchar\([0-9]+\)$`)

func alignReconciliationMySQLCollations(db *gorm.DB) error {
	columns := func(table string) ([]reconciliationMySQLColumn, error) {
		var result []reconciliationMySQLColumn
		err := db.Raw(`SELECT column_name AS column_name, column_type AS column_type,
character_set_name AS character_set_name, collation_name AS collation_name
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name IN ('username', 'model_name')`, table).Scan(&result).Error
		return result, err
	}
	source, err := columns("logs")
	if err != nil {
		return err
	}
	target, err := columns((ReconciliationDailySummary{}).TableName())
	if err != nil {
		return err
	}
	if len(source) != 2 || len(target) != 2 {
		return fmt.Errorf("reconciliation migration: missing log or summary text columns")
	}
	for _, column := range source {
		for _, current := range target {
			if column.ColumnName != current.ColumnName ||
				(column.CharacterSetName == current.CharacterSetName && column.CollationName == current.CollationName) {
				continue
			}
			// Metadata is trusted DB output, but identifiers still cannot be bound
			// as SQL parameters. Validate them before building this narrow DDL.
			if !reconciliationSQLIdentifier.MatchString(column.CharacterSetName) ||
				!reconciliationSQLIdentifier.MatchString(column.CollationName) ||
				!reconciliationSQLVarchar.MatchString(current.ColumnType) {
				return fmt.Errorf("reconciliation migration: unsupported text column metadata for %s", column.ColumnName)
			}
			statement := fmt.Sprintf("ALTER TABLE reconciliation_daily_summaries MODIFY COLUMN `%s` %s CHARACTER SET %s COLLATE %s NOT NULL DEFAULT ''",
				column.ColumnName, current.ColumnType, column.CharacterSetName, column.CollationName)
			if err := db.Exec(statement).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
