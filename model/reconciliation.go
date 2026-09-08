package model

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

func reconciliationPeriodExpression(granularity string) string {
	format := "%Y-%m-%d"
	postgresFormat := "YYYY-MM-DD"
	if granularity == "month" {
		format = "%Y-%m"
		postgresFormat = "YYYY-MM"
	}

	switch {
	case common.UsingPostgreSQL:
		return fmt.Sprintf("TO_CHAR(TO_TIMESTAMP(created_at), '%s')", postgresFormat)
	case common.UsingSQLite:
		return fmt.Sprintf("strftime('%s', created_at, 'unixepoch', 'localtime')", format)
	default:
		return fmt.Sprintf("DATE_FORMAT(FROM_UNIXTIME(created_at), '%s')", format)
	}
}

func buildReconciliationGroupedQuery(ctx context.Context, filter dto.ReconciliationFilter) *gorm.DB {
	periodExpression := reconciliationPeriodExpression(filter.Granularity)
	selectExpression := fmt.Sprintf(
		"user_id, COALESCE(MAX(NULLIF(username, '')), '') AS username, model_name, %s AS period, SUM(quota) AS quota",
		periodExpression,
	)

	query := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Select(selectExpression).
		Where("type = ?", LogTypeConsume).
		Where("created_at >= ? AND created_at < ?", filter.StartTimestamp, filter.EndTimestamp)
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.ModelName != "" {
		query = query.Where("model_name = ?", filter.ModelName)
	}

	return query.
		Group("user_id").
		Group("model_name").
		Group(periodExpression)
}

func CountReconciliationBills(ctx context.Context, filter dto.ReconciliationFilter) (int64, error) {
	groupedQuery := buildReconciliationGroupedQuery(ctx, filter)
	var total int64
	err := LOG_DB.WithContext(ctx).
		Table("(?) AS reconciliation_groups", groupedQuery).
		Count(&total).Error
	return total, err
}

func GetReconciliationBills(
	ctx context.Context,
	filter dto.ReconciliationFilter,
	offset int,
	limit int,
) ([]dto.ReconciliationBill, error) {
	query := buildReconciliationGroupedQuery(ctx, filter).
		Order("period DESC").
		Order("username ASC").
		Order("model_name ASC").
		Order("user_id ASC")
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	items := make([]dto.ReconciliationBill, 0)
	err := query.Scan(&items).Error
	return items, err
}
