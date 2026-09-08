package service

import (
	"context"
	"errors"
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
	ErrReconciliationDateRequired    = errors.New("开始日期和结束日期不能为空")
	ErrReconciliationDateFormat      = errors.New("日期格式应为 YYYY-MM-DD")
	ErrReconciliationDateOrder       = errors.New("开始时间需早于结束时间")
	ErrReconciliationDateRange       = errors.New("查询日期范围不能超过 366 天")
	ErrReconciliationGranularity     = errors.New("时间粒度仅支持 day 或 month")
	ErrReconciliationNoExportData    = errors.New("当前筛选条件下没有可导出的数据")
	ErrReconciliationExportRowsLimit = errors.New("导出结果超过 100000 行，请缩小筛选范围后重试")
	ErrReconciliationInvalidUserID   = errors.New("用户编号必须为正整数")
	ErrReconciliationInvalidPage     = errors.New("页码必须为正整数")
	ErrReconciliationInvalidPageSize = errors.New("每页条数必须为 1 到 100 之间的整数")
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

	start, err := time.ParseInLocation(time.DateOnly, startDate, time.Local)
	if err != nil {
		return dto.ReconciliationFilter{}, ErrReconciliationDateFormat
	}
	end, err := time.ParseInLocation(time.DateOnly, endDate, time.Local)
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
		StartTimestamp: start.Unix(),
		EndTimestamp:   end.AddDate(0, 0, 1).Unix(),
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

	total, err := model.CountReconciliationBills(ctx, filter)
	if err != nil {
		return dto.ReconciliationBillsPage{}, err
	}
	items, err := model.GetReconciliationBills(ctx, filter, (page-1)*pageSize, pageSize)
	if err != nil {
		return dto.ReconciliationBillsPage{}, err
	}
	for i := range items {
		items[i].AmountUSD = float64(items[i].Quota) / common.QuotaPerUnit
	}

	return dto.ReconciliationBillsPage{
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
