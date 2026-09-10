package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	reconciliationListTimeout   = 30 * time.Second
	reconciliationExportTimeout = 2 * time.Minute
)

func parseReconciliationIntQuery(c *gin.Context, key string, defaultValue int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(raw)
}

func buildReconciliationFilterFromRequest(c *gin.Context) (dto.ReconciliationFilter, error) {
	userID, parseErr := parseReconciliationIntQuery(c, "user_id", 0)
	if parseErr != nil || userID < 0 {
		return dto.ReconciliationFilter{}, service.ErrReconciliationInvalidUserID
	}
	return service.BuildReconciliationFilter(
		c.Query("start_date"),
		c.Query("end_date"),
		c.Query("granularity"),
		userID,
		c.Query("model_name"),
	)
}

func GetReconciliationBills(c *gin.Context) {
	filter, err := buildReconciliationFilterFromRequest(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page, err := parseReconciliationIntQuery(c, "p", 1)
	if err != nil || page < 1 {
		common.ApiError(c, service.ErrReconciliationInvalidPage)
		return
	}
	pageSize, err := parseReconciliationIntQuery(c, "page_size", service.ReconciliationDefaultPageSize)
	if err != nil || pageSize < 1 || pageSize > service.ReconciliationMaxPageSize {
		common.ApiError(c, service.ErrReconciliationInvalidPageSize)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), reconciliationListTimeout)
	defer cancel()
	result, err := service.GetReconciliationBillsPage(ctx, filter, page, pageSize)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("reconciliation list query failed: %v", err))
		common.ApiErrorMsg(c, "对账单查询失败，请稍后重试")
		return
	}
	common.ApiSuccess(c, result)
}

func GetReconciliationUserOptions(c *gin.Context) {
	page, err := parseReconciliationIntQuery(c, "p", 1)
	if err != nil || page < 1 {
		common.ApiError(c, service.ErrReconciliationInvalidPage)
		return
	}
	pageSize, err := parseReconciliationIntQuery(c, "page_size", service.ReconciliationDefaultPageSize)
	if err != nil || pageSize < 1 || pageSize > service.ReconciliationMaxPageSize {
		common.ApiError(c, service.ErrReconciliationInvalidPageSize)
		return
	}
	userID, err := parseReconciliationIntQuery(c, "user_id", 0)
	if err != nil || userID < 0 {
		common.ApiError(c, service.ErrReconciliationInvalidUserID)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), reconciliationListTimeout)
	defer cancel()
	result, err := service.GetReconciliationUserOptionsPage(ctx, c.Query("keyword"), userID, page, pageSize)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("reconciliation user options query failed: %v", err))
		common.ApiErrorMsg(c, "对账单用户选项查询失败，请稍后重试")
		return
	}
	common.ApiSuccess(c, result)
}

func GetReconciliationModelOptions(c *gin.Context) {
	page, err := parseReconciliationIntQuery(c, "p", 1)
	if err != nil || page < 1 {
		common.ApiError(c, service.ErrReconciliationInvalidPage)
		return
	}
	pageSize, err := parseReconciliationIntQuery(c, "page_size", service.ReconciliationDefaultPageSize)
	if err != nil || pageSize < 1 || pageSize > service.ReconciliationMaxPageSize {
		common.ApiError(c, service.ErrReconciliationInvalidPageSize)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), reconciliationListTimeout)
	defer cancel()
	result, err := service.GetReconciliationModelOptionsPage(ctx, c.Query("keyword"), page, pageSize)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("reconciliation model options query failed: %v", err))
		common.ApiErrorMsg(c, "对账单模型选项查询失败，请稍后重试")
		return
	}
	common.ApiSuccess(c, result)
}

func ExportReconciliationBills(c *gin.Context) {
	filter, err := buildReconciliationFilterFromRequest(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), reconciliationExportTimeout)
	defer cancel()
	file, err := service.BuildReconciliationWorkbook(ctx, filter)
	if err != nil {
		if err != service.ErrReconciliationNoExportData && err != service.ErrReconciliationExportRowsLimit {
			logger.LogError(c.Request.Context(), fmt.Sprintf("reconciliation export failed: %v", err))
			common.ApiErrorMsg(c, "对账单导出失败，请稍后重试")
			return
		}
		common.ApiError(c, err)
		return
	}
	defer file.Close()

	filename := fmt.Sprintf("对账单_%s.xlsx", time.Now().In(time.Local).Format(time.DateOnly))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	if err = file.Write(c.Writer); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("reconciliation response write failed: %v", err))
	}
}
