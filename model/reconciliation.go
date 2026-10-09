package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

type reconciliationPeriodPlan struct {
	expression string
	labels     []string
}

func reconciliationDayIndexExpression(startTimestamp int64) string {
	switch {
	case common.UsingPostgreSQL:
		return fmt.Sprintf("FLOOR((created_at - %d) / 86400.0)", startTimestamp)
	case common.UsingSQLite:
		return fmt.Sprintf("CAST((created_at - %d) / 86400 AS INTEGER)", startTimestamp)
	default:
		return fmt.Sprintf("((created_at - %d) DIV 86400)", startTimestamp)
	}
}

func reconciliationCasePeriodExpression(periodEnds []int64) string {
	if len(periodEnds) <= 1 {
		return "0"
	}

	var builder strings.Builder
	builder.WriteString("CASE")
	for index := 0; index < len(periodEnds)-1; index++ {
		fmt.Fprintf(&builder, " WHEN created_at < %d THEN %d", periodEnds[index], index)
	}
	fmt.Fprintf(&builder, " ELSE %d END", len(periodEnds)-1)
	return builder.String()
}

func buildReconciliationPeriodPlan(filter dto.ReconciliationFilter) reconciliationPeriodPlan {
	location := time.Local
	start := time.Unix(filter.StartTimestamp, 0).In(location)
	end := time.Unix(filter.EndTimestamp, 0).In(location)

	labels := make([]string, 0, 32)
	periodEnds := make([]int64, 0, 32)
	for cursor := start; cursor.Before(end); {
		labels = append(labels, cursor.Format(time.DateOnly))
		next := cursor.AddDate(0, 0, 1)
		if filter.Granularity == "month" {
			labels[len(labels)-1] = cursor.Format("2006-01")
			next = time.Date(cursor.Year(), cursor.Month()+1, 1, 0, 0, 0, 0, location)
		}
		if next.After(end) {
			next = end
		}
		periodEnds = append(periodEnds, next.Unix())
		cursor = next
	}

	if len(labels) == 0 {
		return reconciliationPeriodPlan{expression: "0", labels: labels}
	}

	if filter.Granularity == "day" {
		fixedLengthDays := true
		periodStart := filter.StartTimestamp
		for _, periodEnd := range periodEnds {
			if periodEnd-periodStart != int64((24 * time.Hour).Seconds()) {
				fixedLengthDays = false
				break
			}
			periodStart = periodEnd
		}
		if fixedLengthDays {
			return reconciliationPeriodPlan{
				expression: reconciliationDayIndexExpression(filter.StartTimestamp),
				labels:     labels,
			}
		}
	}

	return reconciliationPeriodPlan{
		expression: reconciliationCasePeriodExpression(periodEnds),
		labels:     labels,
	}
}

func buildReconciliationFilteredQuery(ctx context.Context, filter dto.ReconciliationFilter) *gorm.DB {

	query := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Where("type = ?", LogTypeConsume).
		Where("created_at >= ? AND created_at < ?", filter.StartTimestamp, filter.EndTimestamp)
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.ModelName != "" {
		query = query.Where("model_name = ?", filter.ModelName)
	}
	return query
}

func buildReconciliationGroupedQuery(
	ctx context.Context,
	filter dto.ReconciliationFilter,
	includeAggregates bool,
) (*gorm.DB, reconciliationPeriodPlan) {
	periodPlan := buildReconciliationPeriodPlan(filter)
	selectExpression := fmt.Sprintf(
		"user_id, model_name, %s AS period_index",
		periodPlan.expression,
	)
	if includeAggregates {
		selectExpression = fmt.Sprintf(
			"user_id, COALESCE(MAX(NULLIF(username, '')), '') AS username, model_name, %s AS period_index, SUM(quota) AS quota",
			periodPlan.expression,
		)
	}

	query := buildReconciliationFilteredQuery(ctx, filter).
		Select(selectExpression).
		Group("user_id").
		Group("model_name").
		Group(periodPlan.expression)
	return query, periodPlan
}

func CountReconciliationBills(ctx context.Context, filter dto.ReconciliationFilter) (int64, error) {
	// The count only needs the grouping keys. Avoid calculating MAX(username)
	// and SUM(quota) during the first of the two paginated queries.
	groupedQuery, _ := buildReconciliationGroupedQuery(ctx, filter, false)
	var total int64
	err := LOG_DB.WithContext(ctx).
		Table("(?) AS reconciliation_groups", groupedQuery).
		Count(&total).Error
	return total, err
}

type reconciliationBillRow struct {
	UserID      int
	Username    string
	PeriodIndex int64
	ModelName   string
	Quota       int64
}

func GetReconciliationBills(
	ctx context.Context,
	filter dto.ReconciliationFilter,
	offset int,
	limit int,
) ([]dto.ReconciliationBill, error) {
	query, periodPlan := buildReconciliationGroupedQuery(ctx, filter, true)
	query = query.
		Order("period_index DESC").
		Order("username ASC").
		Order("model_name ASC").
		Order("user_id ASC")
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	rows := make([]reconciliationBillRow, 0)
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]dto.ReconciliationBill, 0, len(rows))
	for _, row := range rows {
		if row.PeriodIndex < 0 || row.PeriodIndex >= int64(len(periodPlan.labels)) {
			return nil, fmt.Errorf("invalid reconciliation period index: %d", row.PeriodIndex)
		}
		items = append(items, dto.ReconciliationBill{
			UserID:    row.UserID,
			Username:  row.Username,
			Period:    periodPlan.labels[row.PeriodIndex],
			ModelName: row.ModelName,
			Quota:     row.Quota,
		})
	}
	return items, nil
}

func reconciliationOptionSearchPattern(keyword string) string {
	escaped := strings.NewReplacer(
		"!", "!!",
		"%", "!%",
		"_", "!_",
	).Replace(strings.TrimSpace(keyword))
	return "%" + escaped + "%"
}

func buildReconciliationUserOptionsQuery(ctx context.Context, keyword string, userID int) *gorm.DB {
	query := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Where("type = ?", LogTypeConsume).
		Where("user_id > ?", 0)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		pattern := reconciliationOptionSearchPattern(keyword)
		if numericID, err := strconv.Atoi(keyword); err == nil && numericID > 0 {
			query = query.Where("(username LIKE ? ESCAPE '!' OR user_id = ?)", pattern, numericID)
		} else {
			query = query.Where("username LIKE ? ESCAPE '!'", pattern)
		}
	}
	return query
}

func GetReconciliationUserOptions(
	ctx context.Context,
	keyword string,
	userID int,
	offset int,
	limit int,
) ([]dto.ReconciliationUserOption, int64, error) {
	query := buildReconciliationUserOptionsQuery(ctx, keyword, userID)
	var total int64
	if err := query.Distinct("user_id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]dto.ReconciliationUserOption, 0)
	err := query.
		Select("user_id, COALESCE(MAX(NULLIF(username, '')), '') AS username").
		Group("user_id").
		Order("username ASC").
		Order("user_id ASC").
		Offset(offset).
		Limit(limit).
		Scan(&items).Error
	return items, total, err
}

func buildReconciliationModelOptionsQuery(ctx context.Context, keyword string) *gorm.DB {
	query := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Where("type = ?", LogTypeConsume).
		Where("model_name <> ?", "")
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		query = query.Where("model_name LIKE ? ESCAPE '!'", reconciliationOptionSearchPattern(keyword))
	}
	return query
}

func GetReconciliationModelOptions(
	ctx context.Context,
	keyword string,
	offset int,
	limit int,
) ([]dto.ReconciliationModelOption, int64, error) {
	query := buildReconciliationModelOptionsQuery(ctx, keyword)
	var total int64
	if err := query.Distinct("model_name").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]dto.ReconciliationModelOption, 0)
	err := query.
		Select("model_name").
		Distinct().
		Order("model_name ASC").
		Offset(offset).
		Limit(limit).
		Scan(&items).Error
	return items, total, err
}
