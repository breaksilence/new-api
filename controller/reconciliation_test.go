package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestReconciliationErrorsCarryTranslationKeys(t *testing.T) {
	const validDates = "start_date=2026-09-01&end_date=2026-09-02"
	tests := []struct {
		name    string
		query   string
		handler gin.HandlerFunc
		key     string
	}{
		{"missing dates", "", GetReconciliationBills, "Start and end dates are required"},
		{"invalid date", "start_date=invalid&end_date=2026-09-02", GetReconciliationBills, "Dates must use YYYY-MM-DD format"},
		{"reversed dates", "start_date=2026-09-03&end_date=2026-09-02", GetReconciliationBills, "Start date must not be later than end date"},
		{"date range", "start_date=2025-01-01&end_date=2026-09-02", GetReconciliationBills, "The date range cannot exceed 366 days"},
		{"granularity", validDates + "&granularity=week", GetReconciliationBills, "Time granularity must be day or month"},
		{"invalid user", validDates + "&user_id=-1", GetReconciliationBills, "User ID must be a non-negative integer"},
		{"invalid page", validDates + "&p=0", GetReconciliationBills, "Page number must be a positive integer"},
		{"invalid page size", validDates + "&page_size=101", GetReconciliationBills, "Page size must be an integer between 1 and 100"},
		{"export row limit", "", func(c *gin.Context) {
			common.ApiError(c, service.ErrReconciliationExportRowsLimit)
		}, "Export exceeds 100000 rows. Narrow the filters and try again."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			query := c.Request.URL.Query()
			if !query.Has("granularity") {
				query.Set("granularity", "day")
				c.Request.URL.RawQuery = query.Encode()
			}
			tt.handler(c)
			var body map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, false, body["success"])
			assert.Equal(t, tt.key, body["message_key"])
			assert.Equal(t, tt.key, body["message"])
		})
	}
}

func TestReconciliationExportAndQueryErrorsAreTranslatable(t *testing.T) {
	tests := []struct {
		name    string
		handler gin.HandlerFunc
		key     string
		closed  bool
	}{
		{"empty export", ExportReconciliationBills, "No data to export for the current filters", false},
		{"failed list", GetReconciliationBills, "Failed to query reconciliation bills. Please try again later.", true},
		{"failed users", GetReconciliationUserOptions, "Failed to query reconciliation users. Please try again later.", true},
		{"failed models", GetReconciliationModelOptions, "Failed to query reconciliation models. Please try again later.", true},
		{"failed export", ExportReconciliationBills, "Failed to export reconciliation bills. Please try again later.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalDB := model.LOG_DB
			originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
			t.Cleanup(func() {
				model.LOG_DB = originalDB
				common.SetDatabaseTypes(originalMainType, originalLogType)
			})
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Log{}, &model.ReconciliationDailySummary{}, &model.ReconciliationArchiveState{}))
			model.LOG_DB = db
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			conn, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			if tt.closed {
				require.NoError(t, conn.Close())
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/?start_date=2026-09-01&end_date=2026-09-02&granularity=day", nil)
			tt.handler(c)
			var body map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, false, body["success"])
			assert.Equal(t, tt.key, body["message_key"])
			assert.Equal(t, tt.key, body["message"], "do not expose database error details")
		})
	}
}
