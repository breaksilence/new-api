package service

import (
	"context"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const (
	ReconciliationDefaultPageSize = 20
	ReconciliationMaxPageSize     = 100
	ReconciliationMaxExportRows   = 100000
)

var (
	ErrReconciliationDateRequired    = common.NewMessage("Start and end dates are required")
	ErrReconciliationDateFormat      = common.NewMessage("Dates must use YYYY-MM-DD format")
	ErrReconciliationDateOrder       = common.NewMessage("Start date must not be later than end date")
	ErrReconciliationDateRange       = common.NewMessage("The date range cannot exceed 366 days")
	ErrReconciliationGranularity     = common.NewMessage("Time granularity must be day or month")
	ErrReconciliationNoExportData    = common.NewMessage("No data to export for the current filters")
	ErrReconciliationExportRowsLimit = common.NewMessage("Export exceeds 100000 rows. Narrow the filters and try again.")
	ErrReconciliationInvalidUserID   = common.NewMessage("User ID must be a non-negative integer")
	ErrReconciliationInvalidPage     = common.NewMessage("Page number must be a positive integer")
	ErrReconciliationInvalidPageSize = common.NewMessage("Page size must be an integer between 1 and 100")
)

func BuildReconciliationFilter(
	startDate string,
	endDate string,
	granularity string,
	userID int,
	modelName string,
) (dto.ReconciliationFilter, error) {
	startDate = strings.TrimSpace(startDate)
	endDate = strings.TrimSpace(endDate)
	granularity = strings.TrimSpace(granularity)
	if startDate == "" || endDate == "" {
		return dto.ReconciliationFilter{}, ErrReconciliationDateRequired
	}
	if granularity != "day" && granularity != "month" {
		return dto.ReconciliationFilter{}, ErrReconciliationGranularity
	}
	if userID < 0 {
		return dto.ReconciliationFilter{}, ErrReconciliationInvalidUserID
	}

	// Validate civil dates in UTC before resolving their local day boundaries.
	// ParseInLocation can silently shift nonexistent midnight to the prior day.
	start, err := time.Parse(time.DateOnly, startDate)
	if err != nil {
		return dto.ReconciliationFilter{}, ErrReconciliationDateFormat
	}
	end, err := time.Parse(time.DateOnly, endDate)
	if err != nil {
		return dto.ReconciliationFilter{}, ErrReconciliationDateFormat
	}
	if start.After(end) {
		return dto.ReconciliationFilter{}, ErrReconciliationDateOrder
	}
	if end.After(start.AddDate(0, 0, 365)) {
		return dto.ReconciliationFilter{}, ErrReconciliationDateRange
	}

	return dto.ReconciliationFilter{
		StartTimestamp: common.ReconciliationDateStart(start.Year(), start.Month(), start.Day(), time.Local).Unix(),
		EndTimestamp:   common.ReconciliationDateStart(end.Year(), end.Month(), end.Day()+1, time.Local).Unix(),
		UserID:         userID,
		ModelName:      strings.TrimSpace(modelName),
		Granularity:    granularity,
	}, nil
}

func GetReconciliationBillsPage(
	ctx context.Context,
	filter dto.ReconciliationFilter,
	page int,
	pageSize int,
) (dto.ReconciliationBillsPage, error) {
	if page < 1 {
		return dto.ReconciliationBillsPage{}, ErrReconciliationInvalidPage
	}
	if pageSize < 1 || pageSize > ReconciliationMaxPageSize {
		return dto.ReconciliationBillsPage{}, ErrReconciliationInvalidPageSize
	}

	items, total, totalQuota, err := model.GetReconciliationBillsPage(ctx, filter, (page-1)*pageSize, pageSize)
	if err != nil {
		return dto.ReconciliationBillsPage{}, err
	}
	for i := range items {
		items[i].AmountUSD = float64(items[i].Quota) / common.QuotaPerUnit
	}

	return dto.ReconciliationBillsPage{
		Page:           page,
		PageSize:       pageSize,
		Total:          total,
		TotalQuota:     totalQuota,
		TotalAmountUSD: float64(totalQuota) / common.QuotaPerUnit,
		Items:          items,
	}, nil
}

func GetReconciliationUserOptionsPage(
	ctx context.Context,
	keyword string,
	userID int,
	page int,
	pageSize int,
) (dto.ReconciliationUserOptionsPage, error) {
	if page < 1 {
		return dto.ReconciliationUserOptionsPage{}, ErrReconciliationInvalidPage
	}
	if pageSize < 1 || pageSize > ReconciliationMaxPageSize {
		return dto.ReconciliationUserOptionsPage{}, ErrReconciliationInvalidPageSize
	}
	if userID < 0 {
		return dto.ReconciliationUserOptionsPage{}, ErrReconciliationInvalidUserID
	}

	items, total, err := model.GetReconciliationUserOptions(
		ctx,
		strings.TrimSpace(keyword),
		userID,
		(page-1)*pageSize,
		pageSize,
	)
	if err != nil {
		return dto.ReconciliationUserOptionsPage{}, err
	}
	return dto.ReconciliationUserOptionsPage{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	}, nil
}

func GetReconciliationModelOptionsPage(
	ctx context.Context,
	keyword string,
	page int,
	pageSize int,
) (dto.ReconciliationModelOptionsPage, error) {
	if page < 1 {
		return dto.ReconciliationModelOptionsPage{}, ErrReconciliationInvalidPage
	}
	if pageSize < 1 || pageSize > ReconciliationMaxPageSize {
		return dto.ReconciliationModelOptionsPage{}, ErrReconciliationInvalidPageSize
	}

	items, total, err := model.GetReconciliationModelOptions(
		ctx,
		strings.TrimSpace(keyword),
		(page-1)*pageSize,
		pageSize,
	)
	if err != nil {
		return dto.ReconciliationModelOptionsPage{}, err
	}
	return dto.ReconciliationModelOptionsPage{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	}, nil
}

func GetReconciliationExportRows(
	ctx context.Context,
	filter dto.ReconciliationFilter,
) ([]dto.ReconciliationBill, error) {
	items, err := model.GetReconciliationBills(ctx, filter, 0, ReconciliationMaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrReconciliationNoExportData
	}
	if len(items) > ReconciliationMaxExportRows {
		return nil, ErrReconciliationExportRowsLimit
	}
	for i := range items {
		items[i].AmountUSD = float64(items[i].Quota) / common.QuotaPerUnit
	}
	return items, nil
}
