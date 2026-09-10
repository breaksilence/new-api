package dto

// ReconciliationFilter is the shared, validated filter used by both the
// reconciliation list and export paths.
type ReconciliationFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
	UserID         int
	ModelName      string
	Granularity    string
}

type ReconciliationBill struct {
	UserID    int     `json:"user_id"`
	Username  string  `json:"username"`
	Period    string  `json:"period"`
	ModelName string  `json:"model_name"`
	Quota     int64   `json:"quota"`
	AmountUSD float64 `json:"amount_usd"`
}

type ReconciliationBillsPage struct {
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Total    int64                `json:"total"`
	Items    []ReconciliationBill `json:"items"`
}

type ReconciliationUserOption struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
}

type ReconciliationUserOptionsPage struct {
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
	Total    int64                      `json:"total"`
	Items    []ReconciliationUserOption `json:"items"`
}

type ReconciliationModelOption struct {
	ModelName string `json:"model_name"`
}

type ReconciliationModelOptionsPage struct {
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
	Total    int64                       `json:"total"`
	Items    []ReconciliationModelOption `json:"items"`
}
