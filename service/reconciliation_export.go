package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/xuri/excelize/v2"
)

const reconciliationSheetName = "对账单"

func BuildReconciliationWorkbook(
	ctx context.Context,
	filter dto.ReconciliationFilter,
) (*excelize.File, error) {
	items, err := GetReconciliationExportRows(ctx, filter)
	if err != nil {
		return nil, err
	}

	file := excelize.NewFile()
	if err = file.SetSheetName("Sheet1", reconciliationSheetName); err != nil {
		_ = file.Close()
		return nil, err
	}
	headerStyle, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"D9EAF7"}, Pattern: 1},
	})
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	currencySymbol := operation_setting.GetCurrencySymbol()
	exchangeRate := operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
	// Billing displays intentionally stay monetary even when the site's quota
	// display is configured as USD or tokens. Keep the spreadsheet aligned with
	// the current frontend billing formatter.
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeUSD ||
		operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		currencySymbol = "¥"
		exchangeRate = 1
	}
	numberFormat := "#,##0.00"
	if currencySymbol != "" {
		numberFormat = fmt.Sprintf(`"%s"#,##0.00`, strings.ReplaceAll(currencySymbol, `"`, `""`))
	}
	amountStyle, err := file.NewStyle(&excelize.Style{CustomNumFmt: &numberFormat})
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	stream, err := file.NewStreamWriter(reconciliationSheetName)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if err = stream.SetColWidth(1, 1, 24); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err = stream.SetColWidth(2, 3, 20); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err = stream.SetColWidth(4, 4, 18); err != nil {
		_ = file.Close()
		return nil, err
	}
	header := []interface{}{
		excelize.Cell{StyleID: headerStyle, Value: "用户"},
		excelize.Cell{StyleID: headerStyle, Value: "时间"},
		excelize.Cell{StyleID: headerStyle, Value: "模型"},
		excelize.Cell{StyleID: headerStyle, Value: "费用"},
	}
	if err = stream.SetRow("A1", header); err != nil {
		_ = file.Close()
		return nil, err
	}

	for index, item := range items {
		if err = ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		cell, coordinateErr := excelize.CoordinatesToCellName(1, index+2)
		if coordinateErr != nil {
			_ = file.Close()
			return nil, coordinateErr
		}
		row := []interface{}{
			item.Username,
			item.Period,
			item.ModelName,
			excelize.Cell{StyleID: amountStyle, Value: item.AmountUSD * exchangeRate},
		}
		if err = stream.SetRow(cell, row); err != nil {
			_ = file.Close()
			return nil, err
		}
	}
	if err = stream.Flush(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
